// Package lps — Asynq batch expiry worker for LPS exclusion overrides.
//
// Issue #47: APPROVED_ACTIVE rows in ecl.lps_exclusion_override accumulate after
// their valid_to_periode_id's tanggal_akhir passes. The aggregator query-time filter
// correctly skips them, but ListOverrides UI shows stale APPROVED_ACTIVE entries.
//
// Fix: ExpireLPSOverridesJob runs daily (01:00 UTC+7 = 18:00 UTC prev day) via Asynq
// periodic scheduler. For each row WHERE workflow_status='APPROVED_ACTIVE' AND
// valid_to_periode.tanggal_akhir < CURRENT_DATE:
//   - Open individual DB transaction per row.
//   - UPDATE ecl.lps_exclusion_override → workflow_status='EXPIRED'.
//   - Write aud.audit_log with Action="LPS_OVERRIDE.EXPIRED_AUTO" IN SAME TX (DEC-018).
//   - Commit. On audit failure, rollback (override stays APPROVED_ACTIVE, retried next run).
//
// Idempotency: rows already-EXPIRED are excluded by WHERE clause (idempotent re-run).
// Uses partial index idx_lps_override_valid_to_periode (migration 000023) for efficient scan.
//
// References:
//   - FSD-APP-C §3.3 (LPS Aggregator state machine)
//   - DEC-017 (4-eyes workflow), DEC-018 (audit-in-tx)
//   - migration 000023 (ecl.lps_exclusion_override schema)
package lps

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// ─── Task type + payload ──────────────────────────────────────────────────────

// TaskTypeLPSExpiryCheck is the Asynq task type for the LPS override expiry job.
// Scheduled daily at 01:00 WIB (18:00 UTC previous day).
const TaskTypeLPSExpiryCheck = "lps:expiry-check"

// LPSExpiryCheckPayload is the payload for TaskTypeLPSExpiryCheck.
//
//nolint:revive // name intentionally prefixed with LPS for package cross-reference clarity
type LPSExpiryCheckPayload struct {
	TenantID string     `json:"tenant_id"`
	JobID    *uuid.UUID `json:"job_id,omitempty"`
}

// NewLPSExpiryCheckTask creates an Asynq task for the expiry check job.
func NewLPSExpiryCheckTask(p LPSExpiryCheckPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("lps expiry: marshal payload: %w", err)
	}
	return asynq.NewTask(TaskTypeLPSExpiryCheck, b), nil
}

// ─── ExpiryRepo ──────────────────────────────────────────────────────────────

// ExpiryRepoIface defines the minimal DB operations needed by ExpiryWorker.
// Separate from OverrideRepoIface to keep the expiry path narrow and testable.
type ExpiryRepoIface interface {
	// ListExpiredApprovedActive returns IDs (and instrumen_id for audit) of all
	// APPROVED_ACTIVE overrides whose valid_to_periode.tanggal_akhir < refDate.
	// Uses partial index idx_lps_override_valid_to_periode (migration 000023).
	ListExpiredApprovedActive(ctx context.Context, refDate time.Time, tenantID string) ([]ExpiryCandidate, error)

	// MarkExpiredInTx transitions one override to EXPIRED inside the given tx.
	// Returns sql.ErrNoRows if the row is no longer APPROVED_ACTIVE (idempotent).
	MarkExpiredInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, systemUserID uuid.UUID) error

	// BeginTx starts a DB transaction with default options.
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// ExpiryCandidate holds the minimal fields needed for the expiry loop.
type ExpiryCandidate struct {
	ID               uuid.UUID
	InstrumenID      uuid.UUID
	ValidToPeriodeID uuid.UUID
}

// DBExpiryRepo implements ExpiryRepoIface against ecl.lps_exclusion_override.
type DBExpiryRepo struct {
	db *sql.DB
}

// NewDBExpiryRepo creates a DBExpiryRepo.
func NewDBExpiryRepo(db *sql.DB) *DBExpiryRepo {
	return &DBExpiryRepo{db: db}
}

// listExpiredApprovedActiveQuery selects APPROVED_ACTIVE overrides whose
// valid_to_periode.tanggal_akhir is strictly before refDate.
// The WHERE clause hits partial index idx_lps_override_valid_to_periode
// (created in migration 000023 on workflow_status='APPROVED_ACTIVE').
const listExpiredApprovedActiveQuery = `
SELECT ov.id, ov.instrumen_id, ov.valid_to_periode_id
FROM ecl.lps_exclusion_override ov
JOIN mst.periode_buku pb_to ON pb_to.id = ov.valid_to_periode_id
WHERE ov.workflow_status = 'APPROVED_ACTIVE'
  AND ov.deleted_at IS NULL
  AND ov.tenant_id = $1
  AND pb_to.tanggal_akhir < $2
ORDER BY ov.id`

// ListExpiredApprovedActive returns expiry candidates.
func (r *DBExpiryRepo) ListExpiredApprovedActive(ctx context.Context, refDate time.Time, tenantID string) ([]ExpiryCandidate, error) {
	if r.db == nil {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, listExpiredApprovedActiveQuery, tenantID, refDate)
	if err != nil {
		return nil, fmt.Errorf("lps expiry repo: list expired: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []ExpiryCandidate
	for rows.Next() {
		var c ExpiryCandidate
		if err := rows.Scan(&c.ID, &c.InstrumenID, &c.ValidToPeriodeID); err != nil {
			return nil, fmt.Errorf("lps expiry repo: scan candidate: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// markExpiredQuery atomically transitions one APPROVED_ACTIVE override to EXPIRED.
// The WHERE clause guards against double-expiry (idempotent).
const markExpiredQuery = `
UPDATE ecl.lps_exclusion_override
SET workflow_status = 'EXPIRED',
    updated_at      = now(),
    updated_by      = $2,
    row_version     = row_version + 1
WHERE id = $1
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL`

// MarkExpiredInTx executes the expiry UPDATE inside the provided transaction.
// Returns nil if the row was updated, sql.ErrNoRows if it was already not APPROVED_ACTIVE.
func (r *DBExpiryRepo) MarkExpiredInTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, systemUserID uuid.UUID) error {
	if r.db == nil {
		return fmt.Errorf("lps expiry repo: db not initialized")
	}
	res, err := tx.ExecContext(ctx, markExpiredQuery, id, systemUserID)
	if err != nil {
		return fmt.Errorf("lps expiry repo: mark expired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("lps expiry repo: rows affected: %w", err)
	}
	if n == 0 {
		// Row already EXPIRED or deleted — treat as idempotent success.
		return sql.ErrNoRows
	}
	return nil
}

// BeginTx starts a database transaction.
func (r *DBExpiryRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// ─── ExpiryWorker ─────────────────────────────────────────────────────────────

// ExpiryWorker is the Asynq handler that transitions stale APPROVED_ACTIVE
// LPS exclusion overrides to EXPIRED.
//
// Constructor pattern mirrors staging.TaskWorker:
//   - db and auditWriter are mandatory (panic on nil auditWriter per M3 pattern).
//   - systemUserID is the actor UUID stamped on updated_by + audit rows.
//   - logger falls back to slog.Default() if nil.
type ExpiryWorker struct {
	expiryRepo   ExpiryRepoIface
	auditWriter  AuditWriterIface
	logger       *slog.Logger
	systemUserID uuid.UUID
}

// NewExpiryWorker creates an ExpiryWorker.
// Panics if auditWriter is nil — a nil audit writer silently skips the legally-required
// audit trail (DEC-018), which is a compliance violation.
func NewExpiryWorker(
	expiryRepo ExpiryRepoIface,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
	systemUserID uuid.UUID,
) *ExpiryWorker {
	if auditWriter == nil {
		panic("lps ExpiryWorker: auditWriter must not be nil — audit trail is mandatory (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ExpiryWorker{
		expiryRepo:   expiryRepo,
		auditWriter:  auditWriter,
		logger:       logger,
		systemUserID: systemUserID,
	}
}

// HandleExpiryCheck is the Asynq handler for TaskTypeLPSExpiryCheck.
//
// Algorithm per issue #47:
//  1. Unmarshal payload (non-retryable parse errors return nil).
//  2. Query ListExpiredApprovedActive with CURRENT_DATE as refDate.
//  3. For each candidate:
//     a. BEGIN tx.
//     b. UPDATE workflow_status='EXPIRED' (guarded by WHERE workflow_status='APPROVED_ACTIVE').
//     c. Write aud.audit_log Action="LPS_OVERRIDE.EXPIRED_AUTO" IN SAME TX (DEC-018).
//     d. COMMIT. On any error → ROLLBACK, log warning, increment failed counter.
//  4. If any rows failed → return error (Asynq will retry).
func (w *ExpiryWorker) HandleExpiryCheck(ctx context.Context, t *asynq.Task) error {
	var p LPSExpiryCheckPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		w.logger.ErrorContext(ctx, "lps expiry worker: unmarshal payload failed",
			"error", err, "payload", string(t.Payload()))
		// Non-retryable parse error.
		return nil
	}

	tenantID := p.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	// refDate = today (UTC truncated to midnight).
	refDate := time.Now().UTC().Truncate(24 * time.Hour)

	candidates, err := w.expiryRepo.ListExpiredApprovedActive(ctx, refDate, tenantID)
	if err != nil {
		return fmt.Errorf("lps expiry worker: list candidates: %w", err)
	}

	w.logger.InfoContext(ctx, "lps expiry worker: start",
		"candidate_count", len(candidates),
		"ref_date", refDate.Format("2006-01-02"),
		"tenant", tenantID,
	)

	failed := 0
	for _, cand := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := w.expireOne(ctx, cand, tenantID); err != nil {
			w.logger.WarnContext(ctx, "lps expiry worker: expireOne failed",
				"override_id", cand.ID,
				"instrumen_id", cand.InstrumenID,
				"error", err,
			)
			failed++
		}
	}

	w.logger.InfoContext(ctx, "lps expiry worker: done",
		"total", len(candidates),
		"failed", failed,
	)

	if failed > 0 {
		return fmt.Errorf("lps expiry worker: %d overrides failed to expire (see logs)", failed)
	}
	return nil
}

// expireOne performs the per-row transaction: UPDATE + audit + commit.
// On any error the transaction is rolled back and the error is returned.
func (w *ExpiryWorker) expireOne(ctx context.Context, cand ExpiryCandidate, tenantID string) error {
	tx, err := w.expiryRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Deferred rollback is a no-op if Commit already succeeded (sql.ErrTxDone).
	defer rollbackTx(ctx, tx, w.logger)

	// Step 1: UPDATE workflow_status → EXPIRED.
	if err := w.expiryRepo.MarkExpiredInTx(ctx, tx, cand.ID, w.systemUserID); err != nil {
		if err == sql.ErrNoRows {
			// Already EXPIRED or deleted — idempotent, skip silently.
			return nil
		}
		return fmt.Errorf("mark expired id=%s: %w", cand.ID, err)
	}

	// Step 2: Write audit log IN SAME TX (DEC-018).
	if auditErr := w.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: w.systemUserID,
		ActorRole:   "SYSTEM",
		Action:      "LPS_OVERRIDE.EXPIRED_AUTO",
		EntityType:  "ecl.lps_exclusion_override",
		EntityID:    cand.ID,
		BeforeJSON:  map[string]any{"workflow_status": string(WorkflowStatusApprovedActive)},
		AfterJSON: map[string]any{
			"workflow_status":     string(WorkflowStatusExpired),
			"valid_to_periode_id": cand.ValidToPeriodeID.String(),
			"instrumen_id":        cand.InstrumenID.String(),
			"expired_auto_at":     time.Now().UTC().Format(time.RFC3339),
		},
		TenantID: tenantID,
	}); auditErr != nil {
		return fmt.Errorf("audit write id=%s: %w", cand.ID, auditErr)
	}

	// Step 3: Commit.
	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit id=%s: %w", cand.ID, commitErr)
	}

	w.logger.InfoContext(ctx, "lps expiry worker: expired", "override_id", cand.ID)
	return nil
}
