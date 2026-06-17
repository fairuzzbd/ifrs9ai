package closeflow

// service.go — Business logic for P5-M4 Periode Buku Close Workflow.
//
// Service methods:
//   - RequestSoftClose   — runs checklist, persists snapshot, writes audit in-tx.
//   - ApproveSoftClose   — SoD check, stale check, OPEN→SOFT_CLOSED.
//   - RequestHardClose   — fresh checklist, SOFT_CLOSED→HARD_CLOSE_PENDING.
//   - ApproveHardClose   — CFO+step-up, HARD_CLOSE_PENDING→CLOSED, lock kurs, enqueue MV refresh.
//   - RejectHardClose    — CFO no-step-up, HARD_CLOSE_PENDING→SOFT_CLOSED.
//   - RequestReopen      — grace check for CLOSED, persists snapshot.
//   - ApproveReopen      — step-up if CLOSED→SOFT_CLOSED, unlock kurs, writes audit.
//   - GetChecklist       — real-time eval + MANUAL_CHECK snapshot.
//   - ListStatusPeriode  — cursor-paged, sort+filter.
//
// Compliance:
//   - DEC-017: SoD: approver_id ≠ requester_id enforced.
//   - DEC-018: Audit written in-tx with state mutation. Advisory audit via Writer.Write() (best-effort).
//   - DEC-021: Idempotency-Key mandatory — checked by handler, not re-checked here.
//   - DEC-022: Cursor pagination via repo.
//   - DEC-027: Step-up MFA freshness < 5 min for hard-close-approve + CLOSED→SOFT_CLOSED reopen.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// AsynqEnqueuer is the minimal interface for dispatching Asynq tasks.
type AsynqEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Actor carries the identity of the request actor.
type Actor struct {
	UserID uuid.UUID
	Role   string
}

// taskReportingMVRefresh is the Asynq task type for MV refresh after hard-close.
const taskReportingMVRefresh = "reporting:mv_refresh"

// ─── Service ─────────────────────────────────────────────────────────────────

// Service orchestrates close-workflow state transitions for mst.periode_buku.
type Service struct {
	repo      *Repo
	checklist *ChecklistService
	audit     *audit.Writer
	enqueuer  AsynqEnqueuer
	cfg       Config
	logger    *slog.Logger
}

// NewService creates a Service. Panics on nil mandatory deps (DEC-018 audit required).
func NewService(
	repo *Repo,
	checklist *ChecklistService,
	auditWriter *audit.Writer,
	enqueuer AsynqEnqueuer,
	cfg Config,
	logger *slog.Logger,
) *Service {
	if repo == nil {
		panic("closeflow.NewService: repo must not be nil")
	}
	if checklist == nil {
		panic("closeflow.NewService: checklist must not be nil")
	}
	if auditWriter == nil {
		panic("closeflow.NewService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:      repo,
		checklist: checklist,
		audit:     auditWriter,
		enqueuer:  enqueuer,
		cfg:       cfg,
		logger:    logger,
	}
}

// ─── RequestSoftClose ─────────────────────────────────────────────────────────

// RequestSoftClose runs the 4-item checklist, records the request fields on the
// periode row, and persists an append-only SOFT_CLOSE_REQUEST snapshot.
// Does NOT change status_periode (that happens at ApproveSoftClose).
func (s *Service) RequestSoftClose(
	ctx context.Context,
	periodeID uuid.UUID,
	catatan *string,
	rowVersion int64,
	actor Actor,
) (*SoftCloseRequestResponse, error) {
	// --- Load current state (no FOR SHARE needed — not a transition yet) ---
	periode, err := s.repo.GetByID(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: load periode: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	// Validate transition.
	if ok, transErr := CanTransition(periode.StatusPeriode, "soft-close-request", false, false); !ok {
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// Guard: reject if another soft-close request already pending.
	if periode.HasPendingSoftCloseRequest() {
		return nil, ErrSoftClosePendingExists(periode.PeriodeIDKode)
	}

	// Evaluate checklist in real-time.
	evalResult, err := s.checklist.Evaluate(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: checklist eval: %w", err)
	}

	// Record snapshot + update soft_close_requested_* fields in one tx.
	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	// Write request fields with optimistic lock.
	if err := s.repo.SetSoftCloseRequested(ctx, tx, periodeID, actor.UserID, catatan, rowVersion); err != nil {
		if isDomainConflict(err) {
			return nil, err
		}
		return nil, fmt.Errorf("closeflow.RequestSoftClose: set requested: %w", err)
	}

	// Build + insert checklist snapshot.
	snapshotID := uuid.New()
	now := time.Now()
	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionSoftCloseRequest,
		TriggerAction:    SnapshotTransitionSoftCloseRequest,
		EvaluatedAt:      evalResult.EvaluatedAt,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        evalResult.AllPassed,
		TransitionStatus: SnapshotTransitionStatusApproved, // this marks the REQUEST itself was recorded
		ChecklistItems:   evalResult.Items,
		CreatedAt:        now,
		CreatedBy:        actor.UserID,
		TenantID:         periode.TenantID,
	}
	if !evalResult.AllPassed {
		// Transition rejected because checklist failed — override status.
		snap.TransitionStatus = SnapshotTransitionStatusRejected
	}

	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: insert snapshot: %w", err)
	}

	// Audit in same tx (DEC-018).
	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.SOFT_CLOSE_REQUESTED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(periode.StatusPeriode)},
		After: map[string]any{
			"status_periode":          string(periode.StatusPeriode),
			"soft_close_requested_by": actor.UserID,
			"checklist_all_passed":    evalResult.AllPassed,
			"checklist_snapshot_id":   snapshotID,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.RequestSoftClose: commit: %w", err)
	}

	// If checklist failed: return 422 with details AFTER commit (snapshot persisted for audit).
	if !evalResult.AllPassed {
		details := BuildChecklistDetails(evalResult)
		return nil, ErrChecklistFailed(
			fmt.Sprintf("%d checklist item(s) tidak lulus. Soft-close tidak dapat dilanjutkan sampai semua item PASSED.", len(details)),
			details...,
		)
	}

	return &SoftCloseRequestResponse{
		PeriodeID:           periodeID,
		PeriodeKode:         periode.PeriodeIDKode,
		Transition:          SnapshotTransitionSoftCloseRequest,
		ChecklistSnapshotID: snapshotID,
		Checklist:           evalResult,
		AllPassed:           evalResult.AllPassed,
		NextStep:            "Kirimkan ke approver via POST /soft-close-approve dengan snapshot_id ini.",
	}, nil
}

// ─── ApproveSoftClose ─────────────────────────────────────────────────────────

// ApproveSoftClose transitions OPEN → SOFT_CLOSED after 4-eyes SoD + stale check.
func (s *Service) ApproveSoftClose(
	ctx context.Context,
	periodeID uuid.UUID,
	comment *string,
	actor Actor,
) (*SoftCloseApproveResponse, error) {
	// BEGIN tx for serializable read + write.
	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	// SELECT FOR SHARE — prevent concurrent state changes.
	periode, err := s.repo.GetByIDForShare(ctx, tx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: load for share: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	// Validate state machine.
	if ok, transErr := CanTransition(periode.StatusPeriode, "soft-close-approve", false, false); !ok {
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// SoD: approver must NOT be the requester.
	if periode.SoftCloseRequestedBy != nil && *periode.SoftCloseRequestedBy == actor.UserID {
		_ = s.audit.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
			Action:      "PERIODE.SOFT_CLOSE_SOD_VIOLATION",
			EntityType:  "mst.periode_buku",
			EntityID:    periodeID,
			ActorUserID: actor.UserID.String(),
			ActorRole:   actor.Role,
			After: map[string]any{
				"violation": "approver == requester",
				"requester": periode.SoftCloseRequestedBy,
			},
		}))
		return nil, ErrSoDViolation("soft-close approver tidak boleh sama dengan requester (SoD DEC-017)")
	}

	// Stale check: was the checklist snapshot recorded within the stale window?
	stale, err := s.checklist.IsChecklistStale(ctx, periodeID, s.cfg.SoftCloseChecklistStaleHours)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: stale check: %w", err)
	}
	if stale {
		// Write advisory audit (out-of-tx; snapshot committed separately if needed).
		_ = s.audit.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
			Action:      "PERIODE.SOFT_CLOSE_APPROVE_STALE",
			EntityType:  "mst.periode_buku",
			EntityID:    periodeID,
			ActorUserID: actor.UserID.String(),
			ActorRole:   actor.Role,
			After:       map[string]any{"stale_hours": s.cfg.SoftCloseChecklistStaleHours},
		}))
		return nil, ErrChecklistStale()
	}

	// Perform state transition.
	if err := s.repo.SetSoftCloseApproved(ctx, tx, periodeID, actor.UserID, comment); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: set approved: %w", err)
	}

	// Persist approve snapshot.
	snapshotID := uuid.New()
	now := time.Now()
	// Use cached checklist from last SOFT_CLOSE_REQUEST snapshot items (stale check passed).
	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionSoftCloseApprove,
		TriggerAction:    SnapshotTransitionSoftCloseApprove,
		EvaluatedAt:      now,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        true,
		TransitionStatus: SnapshotTransitionStatusApproved,
		ChecklistItems:   []ChecklistItem{},
		OutcomeJSON:      map[string]any{"new_status": string(PeriodeStatusSoftClosed)},
		CreatedAt:        now,
		CreatedBy:        actor.UserID,
		TenantID:         periode.TenantID,
	}
	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: insert snapshot: %w", err)
	}

	// Audit in same tx.
	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.SOFT_CLOSE_APPROVED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(PeriodeStatusOpen)},
		After: map[string]any{
			"status_periode":         string(PeriodeStatusSoftClosed),
			"soft_close_approved_by": actor.UserID,
			"checklist_snapshot_id":  snapshotID,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveSoftClose: commit: %w", err)
	}

	return &SoftCloseApproveResponse{
		PeriodeID:           periodeID,
		PeriodeKode:         periode.PeriodeIDKode,
		StatusPeriode:       PeriodeStatusSoftClosed,
		TanggalSoftClose:    now,
		ApprovedBy:          actor.UserID,
		ChecklistSnapshotID: snapshotID,
		Message:             "Periode berhasil soft-closed. Semua mutasi (kecuali allowlist) sekarang diblokir.",
	}, nil
}

// ─── RequestHardClose ─────────────────────────────────────────────────────────

// RequestHardClose transitions SOFT_CLOSED → HARD_CLOSE_PENDING.
// Runs a fresh checklist and persists HARD_CLOSE_REQUEST snapshot.
func (s *Service) RequestHardClose(
	ctx context.Context,
	periodeID uuid.UUID,
	catatan *string,
	rowVersion int64,
	actor Actor,
) (*HardCloseRequestResponse, error) {
	// Load current state.
	periode, err := s.repo.GetByID(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: load: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	if ok, transErr := CanTransition(periode.StatusPeriode, "hard-close-request", false, false); !ok {
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// F-04/C1: Evaluate checklist BEFORE beginning any tx.
	// If checklist fails, return 422 immediately — mst.periode_buku.status_periode
	// MUST NOT be mutated to HARD_CLOSE_PENDING on a failed checklist.
	evalResult, err := s.checklist.Evaluate(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: checklist eval: %w", err)
	}

	if !evalResult.AllPassed {
		// Persist REJECTED snapshot in a best-effort separate tx (for audit trail)
		// WITHOUT touching status_periode.
		go func() {
			bgCtx := context.Background()
			bgTx, txErr := s.repo.db.BeginTx(bgCtx, nil)
			if txErr != nil {
				s.logger.Warn("closeflow.RequestHardClose: rejected snapshot begin tx", "error", txErr)
				return
			}
			defer func() { _ = bgTx.Rollback() }() //nolint:errcheck
			rejSnap := ChecklistSnapshot{
				ID:               uuid.New(),
				PeriodeBukuID:    periodeID,
				Transition:       SnapshotTransitionHardCloseRequest,
				TriggerAction:    SnapshotTransitionHardCloseRequest,
				EvaluatedAt:      evalResult.EvaluatedAt,
				EvaluatedBy:      actor.UserID,
				ActorRole:        actor.Role,
				AllPassed:        false,
				TransitionStatus: SnapshotTransitionStatusRejected,
				ChecklistItems:   evalResult.Items,
				CreatedAt:        time.Now(),
				CreatedBy:        actor.UserID,
				TenantID:         periode.TenantID,
			}
			if insErr := s.repo.InsertChecklistSnapshot(bgCtx, bgTx, rejSnap); insErr != nil {
				s.logger.Warn("closeflow.RequestHardClose: rejected snapshot insert", "error", insErr)
				return
			}
			if commitErr := bgTx.Commit(); commitErr != nil {
				s.logger.Warn("closeflow.RequestHardClose: rejected snapshot commit", "error", commitErr)
			}
		}()

		details := BuildChecklistDetails(evalResult)
		return nil, ErrChecklistFailed(
			fmt.Sprintf("%d checklist item(s) tidak lulus. Hard-close request ditolak. Perbaiki semua item sebelum mencoba lagi.", len(details)),
			details...,
		)
	}

	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	// Transition to HARD_CLOSE_PENDING with optimistic lock.
	if err := s.repo.SetHardCloseRequested(ctx, tx, periodeID, actor.UserID, catatan, rowVersion); err != nil {
		if isDomainConflict(err) {
			return nil, err
		}
		return nil, fmt.Errorf("closeflow.RequestHardClose: set requested: %w", err)
	}

	// Persist HARD_CLOSE_REQUEST snapshot.
	snapshotID := uuid.New()
	now := time.Now()

	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionHardCloseRequest,
		TriggerAction:    SnapshotTransitionHardCloseRequest,
		EvaluatedAt:      evalResult.EvaluatedAt,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        true,
		TransitionStatus: SnapshotTransitionStatusApproved,
		ChecklistItems:   evalResult.Items,
		CreatedAt:        now,
		CreatedBy:        actor.UserID,
		TenantID:         periode.TenantID,
	}

	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: insert snapshot: %w", err)
	}

	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.HARD_CLOSE_REQUESTED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(PeriodeStatusSoftClosed)},
		After: map[string]any{
			"status_periode":        string(PeriodeStatusHardClosePending),
			"checklist_all_passed":  true,
			"checklist_snapshot_id": snapshotID,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.RequestHardClose: commit: %w", err)
	}

	return &HardCloseRequestResponse{
		PeriodeID:           periodeID,
		PeriodeKode:         periode.PeriodeIDKode,
		Transition:          SnapshotTransitionHardCloseRequest,
		StatusPeriode:       PeriodeStatusHardClosePending,
		ChecklistSnapshotID: snapshotID,
		Checklist:           evalResult,
		NextStep:            "Menunggu persetujuan CFO via POST /hard-close-approve dengan step-up MFA.",
	}, nil
}

// ─── ApproveHardClose ─────────────────────────────────────────────────────────

// ApproveHardClose transitions HARD_CLOSE_PENDING → CLOSED.
// Requires CFO step-up MFA (DEC-027). Locks kurs. Enqueues MV refresh.
func (s *Service) ApproveHardClose(
	ctx context.Context,
	periodeID uuid.UUID,
	comment *string,
	stepUpTokenRef string,
	actor Actor,
) (*HardCloseApproveResponse, error) {
	// Step-up MFA validation handled by handler before calling service.
	// stepUpTokenRef is the pre-hashed token reference (SHA-256 of token ID).

	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	// SELECT FOR SHARE.
	periode, err := s.repo.GetByIDForShare(ctx, tx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: load for share: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	if ok, transErr := CanTransition(periode.StatusPeriode, "hard-close-approve", true, false); !ok {
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// SoD: CFO approver must NOT be the hard-close requester.
	if periode.HardCloseRequestedBy != nil && *periode.HardCloseRequestedBy == actor.UserID {
		_ = s.audit.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
			Action:      "PERIODE.HARD_CLOSE_SOD_VIOLATION",
			EntityType:  "mst.periode_buku",
			EntityID:    periodeID,
			ActorUserID: actor.UserID.String(),
			ActorRole:   actor.Role,
			After:       map[string]any{"violation": "CFO approver == hard-close requester"},
		}))
		return nil, ErrSoDViolation("CFO hard-close approver tidak boleh sama dengan requester (SoD DEC-017)")
	}

	// Transition to CLOSED with grace window.
	if err := s.repo.SetHardCloseApproved(ctx, tx, periodeID, actor.UserID, comment,
		stepUpTokenRef, s.cfg.HardCloseGraceWindowHours); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: set approved: %w", err)
	}

	// Lock FX rates for the period (kurs cannot be changed after hard-close).
	if err := s.repo.LockKursForPeriode(ctx, tx, periodeID); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: lock kurs: %w", err)
	}

	// Persist HARD_CLOSE_APPROVE snapshot.
	snapshotID := uuid.New()
	now := time.Now()
	graceExpiry := now.Add(time.Duration(s.cfg.HardCloseGraceWindowHours) * time.Hour)

	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionHardCloseApprove,
		TriggerAction:    SnapshotTransitionHardCloseApprove,
		EvaluatedAt:      now,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        true,
		TransitionStatus: SnapshotTransitionStatusApproved,
		ChecklistItems:   []ChecklistItem{},
		OutcomeJSON: map[string]any{
			// F-07: step_up_token_ref removed from outcome_jsonb.
			// The canonical reference is mst.periode_buku.step_up_token_ref only.
			"new_status":       string(PeriodeStatusClosed),
			"grace_expires_at": graceExpiry.Format(time.RFC3339),
		},
		CreatedAt: now,
		CreatedBy: actor.UserID,
		TenantID:  periode.TenantID,
	}
	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: insert snapshot: %w", err)
	}

	// Audit in same tx (DEC-018).
	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.HARD_CLOSE_APPROVED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(PeriodeStatusHardClosePending)},
		After: map[string]any{
			"status_periode":         string(PeriodeStatusClosed),
			"hard_close_approved_by": actor.UserID,
			"grace_expires_at":       graceExpiry.Format(time.RFC3339),
			"kurs_locked":            true,
			"checklist_snapshot_id":  snapshotID,
			// Never log stepUpTokenRef — only the hash ref stored in DB.
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: audit: %w", err)
	}

	// C4: Insert a durable sys.job row WITHIN the tx so that if Asynq enqueue
	// fails after commit, the reconciliation worker can pick it up from sys.job.
	mvJobID := uuid.New().String()
	mvJobPayload, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"periode_id": periodeID.String(),
		"trigger":    "HARD_CLOSE_APPROVE",
	})
	if err := s.repo.InsertJobRow(ctx, tx, JobRow{
		ID:          mvJobID,
		Type:        taskReportingMVRefresh,
		Status:      "queued",
		PayloadJSON: mvJobPayload,
		CreatedBy:   actor.UserID,
	}); err != nil {
		// Non-fatal: log + continue. The tx commits the hard-close state which is primary.
		s.logger.Warn("closeflow.ApproveHardClose: insert sys.job row failed (non-fatal)",
			"periodeID", periodeID, "error", err)
		mvJobID = "" // reset so we don't expose a ghost job ID
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveHardClose: commit: %w", err)
	}

	// Enqueue MV refresh Asynq job AFTER commit (async, does not block response).
	// C4: The durable sys.job row above ensures a reconciliation worker can pick up
	// any job that failed to enqueue here.
	var mvJobIDPtr *string
	var mvStatusURL *string
	if s.enqueuer != nil && mvJobID != "" {
		payload, _ := json.Marshal(map[string]string{ //nolint:errcheck
			"periode_id": periodeID.String(),
			"trigger":    "HARD_CLOSE_APPROVE",
			"job_id":     mvJobID, // Asynq task ID = sys.job.id for dedup
		})
		task := asynq.NewTask(taskReportingMVRefresh, payload,
			asynq.TaskID(mvJobID)) // unique task ID = sys.job.id
		if _, enqErr := s.enqueuer.EnqueueContext(ctx, task); enqErr != nil {
			s.logger.Warn("closeflow.ApproveHardClose: mv_refresh enqueue failed — sys.job row persisted for reconciliation",
				"periodeID", periodeID, "jobID", mvJobID, "error", enqErr)
		} else {
			url := fmt.Sprintf("/api/v1/jobs/%s", mvJobID)
			mvJobIDPtr = &mvJobID
			mvStatusURL = &url
		}
	} else if mvJobID != "" {
		// enqueuer nil (dev mode) but job row was inserted — expose ID anyway.
		url := fmt.Sprintf("/api/v1/jobs/%s", mvJobID)
		mvJobIDPtr = &mvJobID
		mvStatusURL = &url
	}

	return &HardCloseApproveResponse{
		PeriodeID:           periodeID,
		PeriodeKode:         periode.PeriodeIDKode,
		StatusPeriode:       PeriodeStatusClosed,
		TanggalHardClose:    now,
		GraceExpiresAt:      graceExpiry,
		ApprovedBy:          actor.UserID,
		ChecklistSnapshotID: snapshotID,
		MvRefreshJobID:      mvJobIDPtr,
		MvRefreshStatusURL:  mvStatusURL,
		Message:             "Periode berhasil hard-closed. Kurs dikunci. MV refresh dijadwalkan.",
	}, nil
}

// ─── RejectHardClose ─────────────────────────────────────────────────────────

// RejectHardClose transitions HARD_CLOSE_PENDING → SOFT_CLOSED.
// CFO only, no step-up MFA required for reject.
func (s *Service) RejectHardClose(
	ctx context.Context,
	periodeID uuid.UUID,
	reason string,
	actor Actor,
) (*PeriodeStateTransitionResponse, error) {
	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RejectHardClose: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	periode, err := s.repo.GetByIDForShare(ctx, tx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RejectHardClose: load for share: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	if ok, transErr := CanTransition(periode.StatusPeriode, "hard-close-reject", false, false); !ok {
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// C9: SoD — hard-close reject actor must NOT be the same user who requested it.
	// In normal role usage ROLE-AKUN-CTL requests and ROLE-CFO rejects, but defense
	// in depth enforces this at user level too.
	if periode.HardCloseRequestedBy != nil && *periode.HardCloseRequestedBy == actor.UserID {
		_ = s.audit.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
			Action:      "PERIODE.HARD_CLOSE_REJECT_SOD_VIOLATION",
			EntityType:  "mst.periode_buku",
			EntityID:    periodeID,
			ActorUserID: actor.UserID.String(),
			ActorRole:   actor.Role,
			After:       map[string]any{"violation": "hard-close reject actor == requester"},
		}))
		return nil, ErrSoDViolation("hard-close reject actor tidak boleh sama dengan requester (SoD DEC-017)")
	}

	if err := s.repo.SetHardCloseRejected(ctx, tx, periodeID, actor.UserID); err != nil {
		return nil, fmt.Errorf("closeflow.RejectHardClose: set rejected: %w", err)
	}

	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.HARD_CLOSE_REJECTED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(PeriodeStatusHardClosePending)},
		After: map[string]any{
			"status_periode": string(PeriodeStatusSoftClosed),
			"rejected_by":    actor.UserID,
			"reason":         reason,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.RejectHardClose: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.RejectHardClose: commit: %w", err)
	}

	return &PeriodeStateTransitionResponse{
		PeriodeID:      periodeID,
		PeriodeKode:    periode.PeriodeIDKode,
		PreviousStatus: PeriodeStatusHardClosePending,
		NewStatus:      PeriodeStatusSoftClosed,
		Transition:     "hard-close-reject",
		Reason:         reason,
		ActorID:        actor.UserID,
		TransitionedAt: time.Now(),
	}, nil
}

// ─── RequestReopen ────────────────────────────────────────────────────────────

// RequestReopen records a reopen request for SOFT_CLOSED→OPEN or CLOSED→SOFT_CLOSED.
// For CLOSED periods: validates grace window (before CLOSED, no step-up needed here —
// step-up is required at ApproveReopen).
func (s *Service) RequestReopen(
	ctx context.Context,
	periodeID uuid.UUID,
	targetStatus PeriodeStatus,
	reason string,
	rowVersion int64,
	actor Actor,
) (*ReopenRequestResponse, error) {
	periode, err := s.repo.GetByID(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestReopen: load: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	// Validate source state and target state combination.
	switch periode.StatusPeriode {
	case PeriodeStatusSoftClosed:
		if targetStatus != PeriodeStatusOpen {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"targetStatus harus OPEN untuk reopen dari SOFT_CLOSED")
		}

	case PeriodeStatusClosed:
		if targetStatus != PeriodeStatusSoftClosed {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"targetStatus harus SOFT_CLOSED untuk reopen dari CLOSED")
		}
		withinGrace := periode.IsWithinGraceWindow()
		if !withinGrace && periode.HardCloseGraceExpiresAt != nil {
			return nil, ErrGraceExpired(
				periode.PeriodeIDKode,
				periode.HardCloseGraceExpiresAt.Format(time.RFC3339),
			)
		}
		// Note: step-up MFA is deferred to ApproveReopen, not required here.

	default:
		return nil, ErrInvalidTransition(
			fmt.Sprintf("reopen tidak dapat dilakukan dari status %s", periode.StatusPeriode))
	}

	// Transition reopen request (write reason, increment row_version).
	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.RequestReopen: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	if err := s.repo.SetReopenRequested(ctx, tx, periodeID, actor.UserID, reason, targetStatus, rowVersion); err != nil {
		if isDomainConflict(err) {
			return nil, err
		}
		return nil, fmt.Errorf("closeflow.RequestReopen: set requested: %w", err)
	}

	// Persist REOPEN_REQUEST snapshot.
	snapshotID := uuid.New()
	now := time.Now()
	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionReopenRequest,
		TriggerAction:    SnapshotTransitionReopenRequest,
		EvaluatedAt:      now,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        true,
		TransitionStatus: SnapshotTransitionStatusApproved,
		ChecklistItems:   []ChecklistItem{},
		OutcomeJSON: map[string]any{
			"target_status":  string(targetStatus),
			"current_status": string(periode.StatusPeriode),
			"reason":         reason,
		},
		CreatedAt: now,
		CreatedBy: actor.UserID,
		TenantID:  periode.TenantID,
	}
	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.RequestReopen: insert snapshot: %w", err)
	}

	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.REOPEN_REQUESTED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(periode.StatusPeriode)},
		After: map[string]any{
			"target_status":         string(targetStatus),
			"reason":                reason,
			"checklist_snapshot_id": snapshotID,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.RequestReopen: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.RequestReopen: commit: %w", err)
	}

	return &ReopenRequestResponse{
		PeriodeID:           periodeID,
		PeriodeKode:         periode.PeriodeIDKode,
		CurrentStatus:       periode.StatusPeriode,
		TargetStatus:        targetStatus,
		ChecklistSnapshotID: snapshotID,
		StepUpMFARequired:   targetStatus == PeriodeStatusSoftClosed,
		NextStep:            "Kirimkan ke approver via POST /reopen-approve (step-up MFA diperlukan jika dari CLOSED).",
	}, nil
}

// ─── ApproveReopen ────────────────────────────────────────────────────────────

// ApproveReopen transitions SOFT_CLOSED→OPEN or CLOSED→SOFT_CLOSED.
// CLOSED→SOFT_CLOSED requires step-up MFA (hasStepUp must be true from handler).
func (s *Service) ApproveReopen(
	ctx context.Context,
	periodeID uuid.UUID,
	comment string,
	stepUpTokenRef string,
	hasStepUp bool,
	actor Actor,
) (*ReopenApproveResponse, error) {
	tx, err := s.repo.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: begin tx: %w", err)
	}
	defer rollbackTx(tx) //nolint:errcheck

	periode, err := s.repo.GetByIDForShare(ctx, tx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: load for share: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	fromClosed := periode.StatusPeriode == PeriodeStatusClosed
	withinGrace := periode.IsWithinGraceWindow()

	// Determine action.
	var action string
	var targetStatus PeriodeStatus
	switch periode.StatusPeriode {
	case PeriodeStatusSoftClosed:
		action = "reopen-soft-closed-to-open"
		targetStatus = PeriodeStatusOpen
	case PeriodeStatusClosed:
		action = "reopen-closed-to-soft-closed"
		targetStatus = PeriodeStatusSoftClosed
	default:
		return nil, ErrInvalidTransition(fmt.Sprintf("reopen approve não pode ser feito de %s", periode.StatusPeriode))
	}

	if ok, transErr := CanTransition(periode.StatusPeriode, action, hasStepUp, withinGrace); !ok {
		if fromClosed && !hasStepUp {
			return nil, ErrMFAStepUpRequired("reopen-approve (CLOSED → SOFT_CLOSED)")
		}
		if fromClosed && !withinGrace {
			var graceStr string
			if periode.HardCloseGraceExpiresAt != nil {
				graceStr = periode.HardCloseGraceExpiresAt.Format(time.RFC3339)
			}
			return nil, ErrGraceExpired(periode.PeriodeIDKode, graceStr)
		}
		return nil, ErrInvalidTransition(transErr.Error())
	}

	// SoD: approver must NOT be the reopen requester.
	if periode.ReopenedBy != nil && *periode.ReopenedBy == actor.UserID {
		_ = s.audit.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
			Action:      "PERIODE.REOPEN_SOD_VIOLATION",
			EntityType:  "mst.periode_buku",
			EntityID:    periodeID,
			ActorUserID: actor.UserID.String(),
			ActorRole:   actor.Role,
			After:       map[string]any{"violation": "reopen approver == requester"},
		}))
		return nil, ErrSoDViolation("reopen approver tidak boleh sama dengan requester (SoD DEC-017)")
	}

	// Apply transition.
	if err := s.repo.SetReopenApproved(ctx, tx, periodeID, actor.UserID, targetStatus, stepUpTokenRef, fromClosed); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: set approved: %w", err)
	}

	// If CLOSED→SOFT_CLOSED: unlock kurs.
	fxUnlocked := false
	if fromClosed {
		if err := s.repo.UnlockKursForPeriode(ctx, tx, periodeID); err != nil {
			return nil, fmt.Errorf("closeflow.ApproveReopen: unlock kurs: %w", err)
		}
		fxUnlocked = true
	}

	// Persist REOPEN_APPROVE snapshot.
	snapshotID := uuid.New()
	now := time.Now()
	snap := ChecklistSnapshot{
		ID:               snapshotID,
		PeriodeBukuID:    periodeID,
		Transition:       SnapshotTransitionReopenApprove,
		TriggerAction:    SnapshotTransitionReopenApprove,
		EvaluatedAt:      now,
		EvaluatedBy:      actor.UserID,
		ActorRole:        actor.Role,
		AllPassed:        true,
		TransitionStatus: SnapshotTransitionStatusApproved,
		ChecklistItems:   []ChecklistItem{},
		OutcomeJSON: map[string]any{
			"previous_status": string(periode.StatusPeriode),
			"new_status":      string(targetStatus),
			"fx_unlocked":     fxUnlocked,
		},
		CreatedAt: now,
		CreatedBy: actor.UserID,
		TenantID:  periode.TenantID,
	}
	if err := s.repo.InsertChecklistSnapshot(ctx, tx, snap); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: insert snapshot: %w", err)
	}

	if err := s.audit.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:      "PERIODE.REOPEN_APPROVED",
		EntityType:  "mst.periode_buku",
		EntityID:    periodeID,
		ActorUserID: actor.UserID.String(),
		ActorRole:   actor.Role,
		Before:      map[string]any{"status_periode": string(periode.StatusPeriode)},
		After: map[string]any{
			"status_periode":        string(targetStatus),
			"reopened_by":           actor.UserID,
			"fx_unlocked":           fxUnlocked,
			"checklist_snapshot_id": snapshotID,
		},
	})); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("closeflow.ApproveReopen: commit: %w", err)
	}

	return &ReopenApproveResponse{
		PeriodeID:      periodeID,
		PeriodeKode:    periode.PeriodeIDKode,
		PreviousStatus: periode.StatusPeriode,
		NewStatus:      targetStatus,
		ReopenedAt:     now,
		ReopenedBy:     actor.UserID,
		FXRateUnlocked: fxUnlocked,
		Message:        fmt.Sprintf("Periode berhasil di-reopen ke %s. %s", targetStatus, reopenMessage(fromClosed, fxUnlocked)),
	}, nil
}

// ─── GetChecklist ─────────────────────────────────────────────────────────────

// GetChecklist returns a real-time checklist evaluation for the period.
// For CLOSED periods, returns the last snapshot (no re-evaluation per S5-AC4).
// A MANUAL_CHECK snapshot is recorded on every call for audit trail.
func (s *Service) GetChecklist(ctx context.Context, periodeID uuid.UUID, actor Actor) (*ClosingChecklistResponse, error) {
	periode, err := s.repo.GetByID(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.GetChecklist: load: %w", err)
	}
	if periode == nil {
		return nil, ErrPeriodeNotFound(periodeID.String())
	}

	// For CLOSED periods: return last snapshot, not a fresh eval (S5-AC4).
	if periode.StatusPeriode == PeriodeStatusClosed {
		lastSnap, err := s.repo.GetLatestSnapshot(ctx, periodeID, nil)
		if err != nil {
			return nil, fmt.Errorf("closeflow.GetChecklist: get last snapshot: %w", err)
		}
		return &ClosingChecklistResponse{
			PeriodeID:      periodeID,
			PeriodeKode:    periode.PeriodeIDKode,
			StatusPeriode:  periode.StatusPeriode,
			EvaluatedAt:    time.Now(),
			AllPassed:      true,
			IsRealTimeEval: false,
			Items:          []ChecklistItem{},
			LastSnapshot:   lastSnap,
		}, nil
	}

	// Real-time evaluation.
	evalResult, err := s.checklist.Evaluate(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("closeflow.GetChecklist: eval: %w", err)
	}

	// Record MANUAL_CHECK snapshot (best-effort; use a new tx separate from response).
	// C8: capture trace + actor from request context BEFORE spawning goroutine.
	// C8: wrap goroutine in recover to prevent silent crash; write audit in same tx.
	bgTraceID := traceIDFromCtx(ctx)
	bgActorID := actor.UserID
	bgActorRole := actor.Role
	bgPeriodeID := periodeID
	bgTenantID := periode.TenantID
	bgEvalResult := evalResult

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("closeflow.GetChecklist: manual check goroutine panic",
					"recover", r,
					"stack", string(debug.Stack()),
					"traceId", bgTraceID,
					"actorUserId", bgActorID,
				)
			}
		}()

		bgCtx := context.Background()
		bgTx, err := s.repo.db.BeginTx(bgCtx, nil)
		if err != nil {
			s.logger.Warn("closeflow.GetChecklist: manual check snapshot begin tx", "error", err, "traceId", bgTraceID)
			return
		}
		defer func() { _ = bgTx.Rollback() }() //nolint:errcheck

		snapID := uuid.New()
		now := time.Now()
		snap := ChecklistSnapshot{
			ID:               snapID,
			PeriodeBukuID:    bgPeriodeID,
			Transition:       SnapshotTransitionManualCheck,
			TriggerAction:    SnapshotTransitionManualCheck,
			EvaluatedAt:      bgEvalResult.EvaluatedAt,
			EvaluatedBy:      bgActorID,
			ActorRole:        bgActorRole,
			AllPassed:        bgEvalResult.AllPassed,
			TransitionStatus: SnapshotTransitionStatusApproved,
			ChecklistItems:   bgEvalResult.Items,
			CreatedAt:        now,
			CreatedBy:        bgActorID,
			TenantID:         bgTenantID,
		}
		if err := s.repo.InsertChecklistSnapshot(bgCtx, bgTx, snap); err != nil {
			s.logger.Warn("closeflow.GetChecklist: manual check snapshot insert", "error", err, "traceId", bgTraceID)
			return
		}

		// C8: write audit log in the same tx.
		if err := s.audit.WithTx(bgTx).Write(bgCtx, audit.Event{
			Action:      "PERIODE.CHECKLIST.MANUAL_CHECK",
			EntityType:  "mst.periode_buku",
			EntityID:    bgPeriodeID,
			ActorUserID: bgActorID.String(),
			ActorRole:   bgActorRole,
			After: map[string]any{
				"checklist_snapshot_id": snapID,
				"all_passed":            bgEvalResult.AllPassed,
				"trace_id":              bgTraceID,
			},
		}); err != nil {
			s.logger.Warn("closeflow.GetChecklist: manual check audit write", "error", err, "traceId", bgTraceID)
			return
		}

		if err := bgTx.Commit(); err != nil {
			s.logger.Warn("closeflow.GetChecklist: manual check snapshot commit", "error", err, "traceId", bgTraceID)
		}
	}()

	// Fetch last snapshot for context.
	lastSnap, _ := s.repo.GetLatestSnapshot(ctx, periodeID, nil) //nolint:errcheck

	return &ClosingChecklistResponse{
		PeriodeID:      periodeID,
		PeriodeKode:    periode.PeriodeIDKode,
		StatusPeriode:  periode.StatusPeriode,
		EvaluatedAt:    evalResult.EvaluatedAt,
		AllPassed:      evalResult.AllPassed,
		IsRealTimeEval: true,
		Items:          evalResult.Items,
		LastSnapshot:   lastSnap,
	}, nil
}

// ─── ListStatusPeriode ────────────────────────────────────────────────────────

// ListStatusPeriode returns a cursor-paged list of periode buku with close-workflow status.
// cursor and limit are passed separately (not part of listquery.Query).
func (s *Service) ListStatusPeriode(ctx context.Context, q listquery.Query, cursor string, limit int) (
	[]StatusPeriodeListItem, interface{}, []interface{}, map[string]any, error,
) {
	items, pagination, sorts, filters, err := s.repo.ListStatusPeriode(ctx, q, cursor, limit)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("closeflow.ListStatusPeriode: %w", err)
	}
	return items, pagination, toInterfaceSlice(sorts), filters, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func rollbackTx(tx interface{ Rollback() error }) {
	_ = tx.Rollback() //nolint:errcheck
}

// isDomainConflict returns true if the error is a row_version CONFLICT domain error.
func isDomainConflict(err error) bool {
	if de, ok := domainerrors.IsDomainError(err); ok {
		return de.Code() == domainerrors.CodeConflict
	}
	return false
}

func reopenMessage(fromClosed, fxUnlocked bool) string {
	if fromClosed && fxUnlocked {
		return "Kurs FX dikembalikan ke unlocked. Mutasi kembali diizinkan."
	}
	if fromClosed {
		return "Kurs FX status tetap (tidak ada kurs yang dikunci sebelumnya)."
	}
	return "Mutasi kembali diizinkan."
}

func toInterfaceSlice[T any](s []T) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// traceIDFromCtx extracts the trace ID from context using the middleware package helper.
func traceIDFromCtx(ctx context.Context) string {
	return middleware.TraceIDFromContext(ctx)
}
