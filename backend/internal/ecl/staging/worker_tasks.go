// Package staging — Asynq worker task handlers for staging engine background jobs.
//
// Three tasks (per state-machine §7):
//  1. TaskEvaluateStaging     — ECL_STAGING_EVALUATE: evaluate one instrument per periode.
//  2. TaskCureAssessmentBatch — ECL_CURE_ASSESSMENT: batch cure check for all Stage 2 instruments.
//  3. TaskOverrideExpiryCheck — ECL_OVERRIDE_EXPIRY_CHECK: mark expired ACTIVE overrides.
//
// All tasks are registered on the Asynq ServeMux in cmd/api/main.go.
// Progress is reported via sys.job table (UX pattern §3 — long-running process).
package staging

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── Task type constants (Asynq queue names) ─────────────────────────────────

const (
	// TaskTypeEvaluateStaging evaluates SICR for one instrument.
	TaskTypeEvaluateStaging = "ecl:staging:evaluate"

	// TaskTypeCureAssessmentBatch runs cure assessment for all Stage 2 instruments.
	TaskTypeCureAssessmentBatch = "ecl:staging:cure_batch"

	// TaskTypeOverrideExpiryCheck marks expired ACTIVE override proposals.
	TaskTypeOverrideExpiryCheck = "ecl:staging:override_expiry"
)

// ─── Payload types ────────────────────────────────────────────────────────────

// EvaluateStagingPayload is the payload for TaskTypeEvaluateStaging.
type EvaluateStagingPayload struct {
	InstrumenID       uuid.UUID  `json:"instrumen_id"`
	TanggalAssessment time.Time  `json:"tanggal_assessment"`
	TenantID          string     `json:"tenant_id"`
	JobID             *uuid.UUID `json:"job_id,omitempty"`
	ActorSub          string     `json:"actor_sub,omitempty"` // JWT sub of the triggering user
	ActorRole         string     `json:"actor_role,omitempty"`
}

// CureAssessmentBatchPayload is the payload for TaskTypeCureAssessmentBatch.
type CureAssessmentBatchPayload struct {
	TenantID string     `json:"tenant_id"`
	JobID    *uuid.UUID `json:"job_id,omitempty"`
	ActorSub string     `json:"actor_sub,omitempty"`
}

// OverrideExpiryCheckPayload is the payload for TaskTypeOverrideExpiryCheck.
type OverrideExpiryCheckPayload struct {
	TenantID string     `json:"tenant_id"`
	JobID    *uuid.UUID `json:"job_id,omitempty"`
	ActorSub string     `json:"actor_sub,omitempty"`
}

// ─── Task constructors ────────────────────────────────────────────────────────

// NewEvaluateStagingTask creates an Asynq task for evaluating one instrument.
func NewEvaluateStagingTask(p EvaluateStagingPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("staging: marshal EvaluateStagingPayload: %w", err)
	}
	return asynq.NewTask(TaskTypeEvaluateStaging, b), nil
}

// NewCureAssessmentBatchTask creates an Asynq task for batch cure assessment.
func NewCureAssessmentBatchTask(p CureAssessmentBatchPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("staging: marshal CureAssessmentBatchPayload: %w", err)
	}
	return asynq.NewTask(TaskTypeCureAssessmentBatch, b), nil
}

// NewOverrideExpiryCheckTask creates an Asynq task for override expiry check.
func NewOverrideExpiryCheckTask(p OverrideExpiryCheckPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("staging: marshal OverrideExpiryCheckPayload: %w", err)
	}
	return asynq.NewTask(TaskTypeOverrideExpiryCheck, b), nil
}

// ─── Worker ──────────────────────────────────────────────────────────────────

// TaskWorker handles all staging background tasks.
type TaskWorker struct {
	svc          *Service
	histRepo     StageHistoryRepository
	overrideRepo OverrideProposalRepository
	auditWriter  *audit.Writer
	logger       *slog.Logger
}

// NewTaskWorker creates a TaskWorker.
// auditWriter must be non-nil in production; pass audit.NewWriter(nil) for tests.
func NewTaskWorker(svc *Service, histRepo StageHistoryRepository, overrideRepo OverrideProposalRepository, auditWriter *audit.Writer, logger *slog.Logger) *TaskWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if auditWriter == nil {
		auditWriter = audit.NewWriter(nil)
	}
	return &TaskWorker{
		svc:          svc,
		histRepo:     histRepo,
		overrideRepo: overrideRepo,
		auditWriter:  auditWriter,
		logger:       logger,
	}
}

// HandleEvaluateStaging handles TaskTypeEvaluateStaging.
//
// Injects synthetic JWT claims from payload actor fields so the service layer
// can extract actorID and tenantID from context (same as HTTP handlers).
func (w *TaskWorker) HandleEvaluateStaging(ctx context.Context, t *asynq.Task) error {
	var p EvaluateStagingPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		w.logger.ErrorContext(ctx, "staging worker: unmarshal EvaluateStagingPayload failed",
			"error", err, "payload", string(t.Payload()))
		// Non-retryable parse error — return nil to avoid infinite retry.
		return nil
	}

	ctx = injectWorkerClaims(ctx, p.ActorSub, p.ActorRole, p.TenantID)

	tanggal := p.TanggalAssessment
	if tanggal.IsZero() {
		tanggal = time.Now().UTC()
	}

	result, err := w.svc.EvaluateSingleInstrumen(ctx, p.InstrumenID, tanggal, p.JobID)
	if err != nil {
		w.logger.WarnContext(ctx, "staging worker: EvaluateInstrumen failed, will retry",
			"instrumen_id", p.InstrumenID, "error", err)
		return fmt.Errorf("staging worker: EvaluateInstrumen: %w", err)
	}

	w.logger.InfoContext(ctx, "staging worker: EvaluateStaging complete",
		"instrumen_id", p.InstrumenID,
		"skipped", result.Skipped,
		"new_stage", result.NewStage,
	)
	return nil
}

// HandleCureAssessmentBatch handles TaskTypeCureAssessmentBatch.
//
// Iterates all Stage 2 instruments for the tenant and runs AssessCure per instrument.
// Individual instrument errors are logged but do not abort the batch.
func (w *TaskWorker) HandleCureAssessmentBatch(ctx context.Context, t *asynq.Task) error {
	var p CureAssessmentBatchPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		w.logger.ErrorContext(ctx, "staging worker: unmarshal CureAssessmentBatchPayload failed",
			"error", err)
		return nil
	}

	ctx = injectWorkerClaims(ctx, p.ActorSub, "SYSTEM", p.TenantID)

	ids, err := w.histRepo.ListStage2Instruments(ctx, p.TenantID)
	if err != nil {
		return fmt.Errorf("staging cure batch: ListStage2Instruments: %w", err)
	}

	w.logger.InfoContext(ctx, "staging cure batch: start", "count", len(ids), "tenant", p.TenantID)

	succeeded, failed := 0, 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cured, err := w.svc.AssessCure(ctx, id)
		if err != nil {
			w.logger.WarnContext(ctx, "staging cure batch: AssessCure error",
				"instrumen_id", id, "error", err)
			failed++
			continue
		}
		if cured {
			w.logger.InfoContext(ctx, "staging cure batch: instrument cured", "instrumen_id", id)
		}
		succeeded++
	}

	w.logger.InfoContext(ctx, "staging cure batch: done",
		"total", len(ids), "succeeded", succeeded, "failed", failed)

	if failed > 0 {
		return fmt.Errorf("staging cure batch: %d instruments failed (see logs)", failed)
	}
	return nil
}

// HandleOverrideExpiryCheck handles TaskTypeOverrideExpiryCheck.
//
// Finds ACTIVE override proposals whose periode_akhir < today and marks them EXPIRED.
// Per state-machine §2.6 (expiry logic).
func (w *TaskWorker) HandleOverrideExpiryCheck(ctx context.Context, t *asynq.Task) error {
	var p OverrideExpiryCheckPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		w.logger.ErrorContext(ctx, "staging worker: unmarshal OverrideExpiryCheckPayload failed",
			"error", err)
		return nil
	}

	ctx = injectWorkerClaims(ctx, p.ActorSub, "SYSTEM", p.TenantID)

	today := time.Now().UTC().Truncate(24 * time.Hour)

	expired, err := w.overrideRepo.ListExpiredActive(ctx, today)
	if err != nil {
		return fmt.Errorf("staging expiry check: ListExpiredActive: %w", err)
	}

	w.logger.InfoContext(ctx, "staging expiry check: start", "count", len(expired))

	// Use a system actor UUID for the expiry update.
	systemActor := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	expiredAt := time.Now().UTC()
	failed := 0
	for _, prop := range expired {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		tx, err := w.overrideRepo.BeginTx(ctx)
		if err != nil {
			w.logger.WarnContext(ctx, "staging expiry: BeginTx failed", "id", prop.ID, "error", err)
			failed++
			continue
		}
		if err := w.overrideRepo.MarkExpired(ctx, tx, prop.ID, systemActor); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
				w.logger.WarnContext(ctx, "staging expiry: rollback failed", "id", prop.ID, "error", rbErr)
			}
			w.logger.WarnContext(ctx, "staging expiry: MarkExpired failed", "id", prop.ID, "error", err)
			failed++
			continue
		}
		// Write audit event IN THE SAME TRANSACTION as MarkExpired (F1 fix).
		// Per security-baseline: mutation must write aud.audit_log in same tx.
		if err := w.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "STAGING.OVERRIDE_EXPIRED",
			EntityType:  "ecl.staging_override_proposal",
			EntityID:    prop.ID,
			Before:      map[string]any{"workflow_status": string(prop.WorkflowStatus)},
			After:       map[string]any{"workflow_status": "EXPIRED", "expired_at": expiredAt},
			ActorUserID: systemActor.String(),
			ActorRole:   "SYSTEM",
		}); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
				w.logger.WarnContext(ctx, "staging expiry: rollback (post-audit) failed", "id", prop.ID, "error", rbErr)
			}
			w.logger.WarnContext(ctx, "staging expiry: audit write failed", "id", prop.ID, "error", err)
			failed++
			continue
		}
		if err := tx.Commit(); err != nil {
			w.logger.WarnContext(ctx, "staging expiry: commit failed", "id", prop.ID, "error", err)
			failed++
			continue
		}
		w.logger.InfoContext(ctx, "staging expiry: marked EXPIRED", "id", prop.ID)
	}

	if failed > 0 {
		return fmt.Errorf("staging expiry check: %d proposals failed to expire (see logs)", failed)
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// injectWorkerClaims creates a synthetic auth.Claims and injects it into ctx.
// Workers do not have a JWT; they use a service identity constructed from payload fields.
func injectWorkerClaims(ctx context.Context, sub, role, tenantID string) context.Context {
	if sub == "" {
		sub = "00000000-0000-0000-0000-000000000001" // system actor UUID
	}
	if tenantID == "" {
		tenantID = "TUGURE"
	}
	claims := &auth.Claims{
		Sub:      sub,
		Roles:    []string{role},
		TenantID: tenantID,
	}
	return auth.ContextWithClaims(ctx, claims)
}
