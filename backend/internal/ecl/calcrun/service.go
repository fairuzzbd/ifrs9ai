package calcrun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	eclcore "blips-ifrs9.tugu-re.com/internal/ecl/core"
)

// service.go — Service: business logic for ECL Calc Run lifecycle (P4-M8).
//
// Panics on nil auditWriter (DEC-018 compliance — audit must be guaranteed).
// All state transitions write to aud.audit_log IN THE SAME TRANSACTION.
//
// Seal workflow (4-eyes, per state machine §4):
//   ROLE-RISK  request → ROLE-ALCO/CFO approve (step-up MFA, DEC-027)
//
// SoD:
//   seal_requested_by ≠ seal_approved_by (server-side, DEC-017)
//
// Immutability:
//   DB trigger fn_ecl_calc_run_no_modify_when_sealed blocks any UPDATE on SEALED rows.
//   IsSealedCalcRun() implements core.CalcRunSealChecker for M7 integration.

// ─── AsynqEnqueuer ────────────────────────────────────────────────────────────

// AsynqEnqueuer is the minimal interface for dispatching Asynq tasks.
// Implemented by *asynq.Client in production; stub in tests.
type AsynqEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ─── JobProgressUpdater ────────────────────────────────────────────────────────

// JobProgressUpdater is the interface for updating sys.job status/progress.
// Implemented by the jobs package in production; stub in tests.
type JobProgressUpdater interface {
	// UpdateStatus sets sys.job.status and progress fields.
	UpdateStatus(ctx context.Context, jobID string, status string, progress int, step string) error
	// MarkCompleted sets sys.job.status=completed + result_jsonb.
	MarkCompleted(ctx context.Context, jobID string, result map[string]any) error
	// MarkFailed sets sys.job.status=failed + error_jsonb.
	MarkFailed(ctx context.Context, jobID string, errCode, errMsg string) error
}

// ─── noop implementations for optional dependencies ──────────────────────────

// noopJobUpdater is a no-op JobProgressUpdater used when not wired.
type noopJobUpdater struct{}

func (n *noopJobUpdater) UpdateStatus(ctx context.Context, jobID, status string, progress int, step string) error {
	return nil
}
func (n *noopJobUpdater) MarkCompleted(ctx context.Context, jobID string, result map[string]any) error {
	return nil
}
func (n *noopJobUpdater) MarkFailed(ctx context.Context, jobID, errCode, errMsg string) error {
	return nil
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service implements the ECL calc run lifecycle and seal workflow.
type Service struct {
	repo        *Repo
	snapshot    *ParameterSnapshotService
	auditWriter *audit.Writer
	asynqClient AsynqEnqueuer  // nil = sync mode (dev/test)
	jobUpdater  JobProgressUpdater
	logger      *slog.Logger
}

// NewService creates a Service. Panics if repo or auditWriter is nil (DEC-018).
// asynqClient may be nil (dev mode; no Asynq dispatch).
func NewService(
	repo *Repo,
	snapshot *ParameterSnapshotService,
	auditWriter *audit.Writer,
	asynqClient AsynqEnqueuer,
	jobUpdater JobProgressUpdater,
	logger *slog.Logger,
) *Service {
	if repo == nil {
		panic("calcrun.NewService: repo must not be nil")
	}
	if auditWriter == nil {
		panic("calcrun.NewService: auditWriter must not be nil (DEC-018)")
	}
	if snapshot == nil {
		panic("calcrun.NewService: snapshot must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	jpu := jobUpdater
	if jpu == nil {
		jpu = &noopJobUpdater{}
	}
	return &Service{
		repo:        repo,
		snapshot:    snapshot,
		auditWriter: auditWriter,
		asynqClient: asynqClient,
		jobUpdater:  jpu,
		logger:      logger,
	}
}

// ─── IsSealedCalcRun (implements core.CalcRunSealChecker) ────────────────────

// IsSealedCalcRun returns true if the calc_run has status = 'SEALED'.
// Implements core.CalcRunSealChecker injected into M7 ECLOrchestrator.
func (s *Service) IsSealedCalcRun(ctx context.Context, calcRunID uuid.UUID) (bool, error) {
	return s.repo.IsSealedCalcRun(ctx, calcRunID)
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create inserts a new DRAFT calc_run.
//
// Guards (state machine §create):
//   - periode_buku must NOT be HARD_CLOSED.
//   - No IN_PROGRESS calc_run for same periodeID (CALC_RUN_DUPLICATE_IN_PROGRESS 409).
//   - No SEALED calc_run for same periodeID (CALC_RUN_PERIODE_ALREADY_SEALED 409).
//
// Audit: CALC_RUN.CREATED written in-transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest, actorID uuid.UUID) (CalcRun, error) {
	// Parse evaluation_date.
	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: invalid evaluation_date %q: %w", req.EvaluationDate, err)
	}

	// 1. Check periode not HARD_CLOSED.
	if err := s.checkPeriodeNotHardClosed(ctx, req.PeriodeID); err != nil {
		return CalcRun{}, err
	}

	// 2. Check no IN_PROGRESS for same periode.
	existingInProgress, err := s.repo.CheckExistingInProgress(ctx, req.PeriodeID)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: check in-progress: %w", err)
	}
	if existingInProgress != "" {
		return CalcRun{}, ErrCalcRunDuplicateInProgress(req.PeriodeID, existingInProgress)
	}

	// 3. Check no SEALED for same periode.
	existingSealed, err := s.repo.CheckExistingSealed(ctx, req.PeriodeID)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: check sealed: %w", err)
	}
	if existingSealed != "" {
		return CalcRun{}, ErrCalcRunPeriodeAlreadySealed(req.PeriodeID, existingSealed)
	}

	// 4. Insert in transaction.
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	run := CalcRun{
		ID:             uuid.New(),
		PeriodeID:      req.PeriodeID,
		EvaluationDate: evalDate,
		Scope:          req.Scope,
		Status:         StatusDraft,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
	}

	created, err := s.repo.Create(ctx, tx, run)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: insert: %w", err)
	}

	// 5. Audit CALC_RUN.CREATED in-transaction.
	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.CREATED",
		EntityType: "ecl.calc_run",
		EntityID:   created.ID,
		After: map[string]any{
			"id":              created.ID,
			"periode_id":      created.PeriodeID,
			"evaluation_date": created.EvaluationDate.Format("2006-01-02"),
			"scope":           created.Scope,
			"status":          string(created.Status),
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Create: commit: %w", err)
	}

	s.logger.InfoContext(ctx, "calcrun.Create: created", "calc_run_id", created.ID, "periode_id", created.PeriodeID)
	return created, nil
}

// ─── Get / List ───────────────────────────────────────────────────────────────

// Get returns a single CalcRun by ID.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (CalcRun, error) {
	return s.repo.Get(ctx, id)
}

// List returns paginated CalcRun summaries for a periode.
func (s *Service) List(ctx context.Context, periodeID string, limit int, cursor string) ([]Summary, string, bool, error) {
	return s.repo.List(ctx, periodeID, limit, cursor)
}

// ─── Start ────────────────────────────────────────────────────────────────────

// Start transitions DRAFT → IN_PROGRESS, freezes the parameter snapshot,
// creates a sys.job row, and dispatches an Asynq ECL_CALC_RUN task.
//
// Guards: status must be DRAFT; periode must not be HARD_CLOSED; all params APPROVED; kurs available.
// Audit: CALC_RUN.STARTED written in-transaction.
func (s *Service) Start(ctx context.Context, id uuid.UUID, actorID uuid.UUID) (StartResponse, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return StartResponse{}, err
	}

	if !run.Status.CanStart() {
		return StartResponse{}, ErrCalcRunInvalidTransition(
			string(run.Status), "start",
			"Hanya DRAFT yang bisa di-start.")
	}

	// Check periode not HARD_CLOSED.
	if err := s.checkPeriodeNotHardClosed(ctx, run.PeriodeID); err != nil {
		return StartResponse{}, err
	}

	// Freeze parameter snapshot (validates all APPROVED params exist).
	snap, err := s.snapshot.SnapshotAll(ctx, run.PeriodeID, run.EvaluationDate)
	if err != nil {
		return StartResponse{}, err
	}

	// Count total instruments in scope (needed for progress tracking).
	totalCount, err := s.countActiveInstruments(ctx)
	if err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: count instruments: %w", err)
	}

	jobID := uuid.New().String() // ULID-style unique job ID

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	// Update status IN_PROGRESS + snapshot + job_id + started_at.
	updated, err := s.repo.UpdateStartFields(ctx, tx, id, snap, jobID, totalCount, actorID)
	if err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: update: %w", err)
	}

	// Insert sys.job row (non-blocking — failure here fails the start).
	if err := s.insertSysJob(ctx, tx, jobID, id, actorID); err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: sys.job: %w", err)
	}

	// Audit CALC_RUN.STARTED in-transaction.
	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.STARTED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":       id,
			"status":   string(StatusInProgress),
			"job_id":   jobID,
			"started_at": time.Now().Format(time.RFC3339),
			"parameter_snapshot_hash": snapHash(snap),
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return StartResponse{}, fmt.Errorf("calcrun.Start: commit: %w", err)
	}

	// Dispatch Asynq task (after commit, so if dispatch fails, the run stays IN_PROGRESS
	// and can be retried / cancelled).
	if s.asynqClient != nil {
		payload := eclcore.TaskECLBulkComputePayload{
			JobID:          jobID,
			CalcRunID:      updated.ID,
			EvaluationDate: updated.EvaluationDate,
			PeriodeID:      updated.PeriodeID,
			ActorID:        actorID,
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return StartResponse{}, fmt.Errorf("calcrun.Start: marshal asynq payload: %w", err)
		}
		task := asynq.NewTask(eclcore.TaskNameECLBulkCompute, b)
		if _, err := s.asynqClient.EnqueueContext(ctx, task); err != nil {
			s.logger.WarnContext(ctx, "calcrun.Start: asynq enqueue failed (run stays IN_PROGRESS)", "error", err)
		}
	} else {
		s.logger.WarnContext(ctx, "calcrun.Start: asynqClient nil — task NOT dispatched (dev mode)")
	}

	return StartResponse{
		JobID:     jobID,
		StatusURL: "/api/v1/jobs/" + jobID,
		StreamURL: "/api/v1/jobs/" + jobID + "/stream",
	}, nil
}

// ─── Cancel ───────────────────────────────────────────────────────────────────

// Cancel transitions DRAFT/IN_PROGRESS → CANCELLED.
//
// Guards: status IN (DRAFT, IN_PROGRESS); actorID = created_by (maker-only); cancel_reason ≥ 30 chars.
// Audit: CALC_RUN.CANCELLED written in-transaction.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, req CancelRequest, actorID uuid.UUID) (CalcRun, error) {
	if len(req.CancelReason) < 30 {
		return CalcRun{}, ErrCalcRunCancelReasonTooShort()
	}

	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return CalcRun{}, err
	}

	if !run.Status.CanCancel() {
		return CalcRun{}, ErrCalcRunCancelAfterCompleted(string(run.Status))
	}

	// Maker-only check.
	if run.CreatedBy != actorID {
		return CalcRun{}, ErrCalcRunForbiddenNotMaker(run.CreatedBy.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Cancel: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	cancelled, err := s.repo.UpdateCancel(ctx, tx, id, actorID, req.CancelReason)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Cancel: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.CANCELLED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":            id,
			"status":        string(StatusCancelled),
			"cancelled_by":  actorID,
			"cancel_reason": req.CancelReason,
			"partial_count": run.ProcessedCount,
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Cancel: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.Cancel: commit: %w", err)
	}

	// For IN_PROGRESS: the Asynq worker monitors ctx.Done() for graceful stop.
	// Partial calc_result_line rows committed by the worker are preserved (immutable audit).
	s.logger.InfoContext(ctx, "calcrun.Cancel: cancelled", "calc_run_id", id, "prior_status", run.Status)
	return cancelled, nil
}

// ─── MarkCompleted (called by Asynq worker) ───────────────────────────────────

// MarkCompleted transitions IN_PROGRESS → COMPLETED or COMPLETED_WITH_ERRORS.
// Called by the Asynq worker after BulkCompute finishes.
// Audit: CALC_RUN.COMPLETED / CALC_RUN.COMPLETED_WITH_ERRORS in-transaction.
func (s *Service) MarkCompleted(ctx context.Context, id uuid.UUID, processed, errors int, actorID uuid.UUID) (CalcRun, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return CalcRun{}, err
	}

	if run.Status == StatusCancelled {
		// Race: worker finished after cancel was acknowledged. Return gracefully.
		s.logger.InfoContext(ctx, "calcrun.MarkCompleted: run was cancelled before completion", "calc_run_id", id)
		return run, nil
	}

	finalStatus := StatusCompleted
	if errors > 0 {
		finalStatus = StatusCompletedWithErrors
	}
	auditAction := "CALC_RUN.COMPLETED"
	if errors > 0 {
		auditAction = "CALC_RUN.COMPLETED_WITH_ERRORS"
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.MarkCompleted: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	completed, err := s.repo.UpdateCompletion(ctx, tx, id, finalStatus, processed, errors, actorID)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.MarkCompleted: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":              id,
			"status":          string(finalStatus),
			"processed_count": processed,
			"error_count":     errors,
			"completed_at":    time.Now().Format(time.RFC3339),
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.MarkCompleted: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.MarkCompleted: commit: %w", err)
	}
	return completed, nil
}

// ─── Seal: RequestSeal ────────────────────────────────────────────────────────

// RequestSeal transitions COMPLETED → SEAL_REQUESTED.
//
// Guards: status = COMPLETED; error_count = 0; no other SEALED run for periode.
// Audit: CALC_RUN.SEAL_REQUESTED in-transaction.
func (s *Service) RequestSeal(ctx context.Context, id uuid.UUID, req SealRequestBody, actorID uuid.UUID) (CalcRun, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return CalcRun{}, err
	}

	if !run.Status.CanRequestSeal() {
		return CalcRun{}, ErrCalcRunSealRequiresCompleted(string(run.Status))
	}
	if run.ErrorCount > 0 {
		return CalcRun{}, ErrCalcRunHasErrors(run.ErrorCount)
	}

	// Check no SEALED run for same periode.
	existingSealed, err := s.repo.CheckExistingSealed(ctx, run.PeriodeID)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RequestSeal: check sealed: %w", err)
	}
	if existingSealed != "" {
		return CalcRun{}, ErrCalcRunPeriodeAlreadySealed(run.PeriodeID, existingSealed)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RequestSeal: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	updated, err := s.repo.UpdateSealRequest(ctx, tx, id, actorID, req.Comment)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RequestSeal: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.SEAL_REQUESTED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":                 id,
			"status":             string(StatusSealRequested),
			"seal_requested_by":  actorID,
			"seal_requested_at":  time.Now().Format(time.RFC3339),
			"seal_comment":       req.Comment,
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RequestSeal: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RequestSeal: commit: %w", err)
	}
	return updated, nil
}

// ─── Seal: ApproveSeal ────────────────────────────────────────────────────────

// ApproveSeal transitions SEAL_REQUESTED → SEALED.
//
// Guards: status = SEAL_REQUESTED; approver ≠ requester (4-eyes SoD, DEC-017);
//
//	step-up MFA valid ≤ 5 min (DEC-027); permission calc_run.seal_approve.
//
// Signature: SHA-256(approver_id|calc_run_id|sealed_at_rfc3339|comment).
// Audit: CALC_RUN.SEAL_APPROVED + CALC_RUN.SEALED in same transaction.
func (s *Service) ApproveSeal(ctx context.Context, id uuid.UUID, req SealApproveBody, actorID uuid.UUID, stepUpFresh bool) (CalcRun, error) {
	if !stepUpFresh {
		return CalcRun{}, ErrCalcRunSealStepUpRequired()
	}

	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return CalcRun{}, err
	}

	if !run.Status.CanApproveSeal() {
		return CalcRun{}, ErrCalcRunSealNotRequested(string(run.Status))
	}

	// 4-eyes SoD: approver ≠ requester (server-side enforcement, DEC-017).
	if run.SealRequestedBy != nil && *run.SealRequestedBy == actorID {
		// Write SoD violation audit event before returning error.
		// Failure to record this audit event is itself an error — abort and return.
		sodTx, txErr := s.repo.BeginTx(ctx)
		if txErr == nil {
			txWriter := s.auditWriter.WithTx(sodTx)
			if writeErr := txWriter.Write(ctx, audit.Event{
				Action:     "CALC_RUN.SOD_VIOLATION_ATTEMPT",
				EntityType: "ecl.calc_run",
				EntityID:   id,
				After: map[string]any{
					"id":                id,
					"attempted_by":      actorID,
					"seal_requested_by": run.SealRequestedBy,
					"violation_type":    "SEAL_APPROVE_BY_REQUESTER",
				},
				ActorUserID: actorID.String(),
			}); writeErr != nil {
				// Audit write failed — rollback and surface as internal error (DEC-018).
				if rbErr := sodTx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
					s.logger.WarnContext(ctx, "calcrun.ApproveSeal: SoD audit rollback error", "error", rbErr)
				}
				return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: SoD audit write: %w", writeErr)
			}
			if commitErr := sodTx.Commit(); commitErr != nil {
				return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: SoD audit commit: %w", commitErr)
			}
		}
		return CalcRun{}, ErrCalcRunSealSoDViolation(actorID.String())
	}

	sealedAt := time.Now().UTC()

	// Compute signature hash: SHA-256(approver_id|calc_run_id|sealed_at|comment).
	sigInput := fmt.Sprintf("%s|%s|%s|%s", actorID, id, sealedAt.Format(time.RFC3339Nano), req.Comment)
	sigHash := sha256.Sum256([]byte(sigInput))
	sigHashBytes := []byte(hex.EncodeToString(sigHash[:]))

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	sealed, err := s.repo.UpdateSealApprove(ctx, tx, id, actorID, sigHashBytes)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: update: %w", err)
	}

	// Write two audit events in same tx: SEAL_APPROVED + SEALED.
	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.SEAL_APPROVED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":              id,
			"seal_approved_by": actorID,
			"step_up_method":  "JWT_STEP_UP",
			"comment":         req.Comment,
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: audit approved: %w", err)
	}
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.SEALED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":                    id,
			"status":                string(StatusSealed),
			"sealed_at":             sealedAt.Format(time.RFC3339),
			"sealed_by":             actorID,
			"seal_requested_by":     run.SealRequestedBy,
			"seal_requested_at":     run.SealRequestedAt,
			"signature_hash_seal":   hex.EncodeToString(sigHash[:]),
			"signature_method":      "JWT_STEP_UP",
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: audit sealed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.ApproveSeal: commit: %w", err)
	}

	s.logger.InfoContext(ctx, "calcrun.ApproveSeal: sealed", "calc_run_id", id, "approver", actorID)
	return sealed, nil
}

// ─── Seal: RejectSeal ─────────────────────────────────────────────────────────

// RejectSeal transitions SEAL_REQUESTED → COMPLETED (re-requestable).
//
// Guards: status = SEAL_REQUESTED; approver ≠ requester (SoD).
// seal_requested_by/at are cleared so a re-request is possible.
// Audit: CALC_RUN.SEAL_REJECTED in-transaction.
func (s *Service) RejectSeal(ctx context.Context, id uuid.UUID, req SealRejectBody, actorID uuid.UUID) (CalcRun, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return CalcRun{}, err
	}

	if !run.Status.CanApproveSeal() {
		return CalcRun{}, ErrCalcRunSealNotRequested(string(run.Status))
	}

	// SoD: rejector ≠ requester.
	if run.SealRequestedBy != nil && *run.SealRequestedBy == actorID {
		return CalcRun{}, ErrCalcRunSealSoDViolation(actorID.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RejectSeal: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	rejected, err := s.repo.UpdateSealReject(ctx, tx, id, actorID, req.RejectReason)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RejectSeal: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "CALC_RUN.SEAL_REJECTED",
		EntityType: "ecl.calc_run",
		EntityID:   id,
		After: map[string]any{
			"id":               id,
			"status":           string(StatusCompleted),
			"seal_rejected_by": actorID,
			"reject_reason":    req.RejectReason,
		},
		ActorUserID: actorID.String(),
	}); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RejectSeal: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.RejectSeal: commit: %w", err)
	}
	return rejected, nil
}

// ─── UpdateProgress (called by worker) ───────────────────────────────────────

// UpdateProgress updates processed_count and error_count non-transactionally.
// Called periodically by the Asynq worker for progress reporting.
func (s *Service) UpdateProgress(ctx context.Context, id uuid.UUID, processed, errors int, actorID uuid.UUID) error {
	return s.repo.UpdateProgress(ctx, id, processed, errors, actorID)
}

// ─── GetParameterSnapshot ─────────────────────────────────────────────────────

// GetParameterSnapshot returns the frozen parameter snapshot for an existing calc run.
func (s *Service) GetParameterSnapshot(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return run.ParameterSnapshotJSONB, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// checkPeriodeNotHardClosed verifies the periode_buku is not in HARD_CLOSED state.
func (s *Service) checkPeriodeNotHardClosed(ctx context.Context, periodeID string) error {
	var status string
	err := s.repo.db.QueryRowContext(ctx,
		`SELECT status FROM mst.periode_buku WHERE id = $1 AND deleted_at IS NULL`,
		periodeID).Scan(&status)
	if err == sql.ErrNoRows {
		// Periode not found — not a hard-close block, but validation will catch this.
		return nil
	}
	if err != nil {
		return fmt.Errorf("calcrun.checkPeriodeNotHardClosed: %w", err)
	}
	if status == "HARD_CLOSED" {
		return ErrCalcRunPeriodeHardClosed(periodeID)
	}
	return nil
}

// countActiveInstruments returns the count of active APPROVED instruments.
func (s *Service) countActiveInstruments(ctx context.Context) (int, error) {
	var count int
	err := s.repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mst.instrumen
         WHERE deleted_at IS NULL AND workflow_status = 'APPROVED' AND status = 'AKTIF'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("calcrun.countActiveInstruments: %w", err)
	}
	return count, nil
}

// insertSysJob creates a sys.job row for the Asynq job.
func (s *Service) insertSysJob(ctx context.Context, tx *sql.Tx, jobID string, calcRunID uuid.UUID, actorID uuid.UUID) error {
	payload := map[string]any{"calc_run_id": calcRunID.String()}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("calcrun.insertSysJob: marshal payload: %w", err)
	}

	var execErr error
	_, execErr = tx.ExecContext(ctx, `
INSERT INTO sys.job (
    id, type, status, progress, current_step,
    payload_jsonb, can_cancel,
    created_by, updated_by, tenant_id
) VALUES (
    $1, 'ECL_CALC_RUN', 'queued', 0, 'Menunggu eksekusi...',
    $2, true,
    $3, $3, 'TUGURE'
) ON CONFLICT (id) DO NOTHING`,
		jobID, payloadJSON, actorID)
	if execErr != nil {
		return fmt.Errorf("calcrun.insertSysJob: %w", execErr)
	}
	return nil
}

// snapHash returns a short SHA-256 hex of the snapshot for the audit event (non-secret).
func snapHash(snap json.RawMessage) string {
	h := sha256.Sum256(snap)
	return hex.EncodeToString(h[:8]) // first 8 bytes as hex prefix for audit readability
}

// rollbackTx rolls back a transaction, logging any error at WARN level.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(ctx, "calcrun: tx rollback error", "error", err)
	}
}
