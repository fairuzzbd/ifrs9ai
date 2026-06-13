package calcrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// repo.go — Repo: persistence for ecl.calc_run (migration 000031).
//
// Rules (db-conventions.md, DEC-018):
//   - No hard delete: ecl.calc_run is append-only; DB trigger blocks hard deletes.
//   - Sealed rows: DB trigger fn_ecl_calc_run_no_modify_when_sealed prevents UPDATE
//     on sealed rows. Service guard calls IsSealedCalcRun before any mutation.
//   - No float64: all numeric values use decimal.Decimal or int.
//   - Audit cols: created_by / updated_by wired from actorID.

// Repo handles ecl.calc_run persistence.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo. Panics if db is nil.
func NewRepo(db *sql.DB) *Repo {
	if db == nil {
		panic("calcrun.NewRepo: db must not be nil")
	}
	return &Repo{db: db}
}

// IsSealedCalcRun implements the core.CalcRunSealChecker interface (F4 fix in M7).
// Returns true if the calc_run row has status = 'SEALED'.
// Returns false (not sealed) when the row does not exist.
func (r *Repo) IsSealedCalcRun(ctx context.Context, calcRunID uuid.UUID) (bool, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`,
		calcRunID).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil // not found → not sealed
	}
	if err != nil {
		return false, fmt.Errorf("calcrun.repo.IsSealedCalcRun: %w", err)
	}
	return status == string(StatusSealed), nil
}

// Create inserts a new DRAFT calc_run within the provided transaction.
// Returns the inserted CalcRun.
func (r *Repo) Create(ctx context.Context, tx *sql.Tx, run CalcRun) (CalcRun, error) {
	run.Status = StatusDraft
	run.ProcessedCount = 0
	run.ErrorCount = 0
	if run.Scope == "" {
		run.Scope = "ALL_ACTIVE"
	}

	_, err := tx.ExecContext(ctx, `
INSERT INTO ecl.calc_run (
    id, periode_id, evaluation_date, scope, status,
    processed_count, error_count,
    created_by, updated_by, tenant_id
) VALUES (
    $1, $2, $3, $4, $5,
    0, 0,
    $6, $6, 'TUGURE'
)`,
		run.ID, run.PeriodeID, run.EvaluationDate.Format("2006-01-02"),
		run.Scope, string(run.Status),
		run.CreatedBy,
	)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.Create: %w", err)
	}
	return r.getByIDTx(ctx, tx, run.ID)
}

// Get returns a CalcRun by ID. Returns ErrCalcRunNotFound if not found.
func (r *Repo) Get(ctx context.Context, id uuid.UUID) (CalcRun, error) {
	return r.getByID(ctx, r.db, id)
}

func (r *Repo) getByID(ctx context.Context, q queryer, id uuid.UUID) (CalcRun, error) {
	row := q.QueryRowContext(ctx, calcRunSelectSQL+` WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanCalcRun(row)
}

func (r *Repo) getByIDTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (CalcRun, error) {
	return r.getByID(ctx, tx, id)
}

// List returns paginated CalcRun summaries, ordered by created_at DESC.
// Simple implementation: cursor is an encoded created_at/id pair (offset-based fallback).
func (r *Repo) List(ctx context.Context, periodeID string, limit int, cursor string) ([]Summary, string, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows *sql.Rows
	var err error
	query := `
SELECT id, periode_id, evaluation_date, scope, status,
       processed_count, error_count, total_instrumen,
       started_at, completed_at, sealed_at,
       created_at, created_by
FROM ecl.calc_run
WHERE deleted_at IS NULL`

	var args []any
	argIdx := 1

	if periodeID != "" {
		query += fmt.Sprintf(" AND periode_id = $%d", argIdx)
		args = append(args, periodeID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argIdx)
	args = append(args, limit+1)

	rows, err = r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("calcrun.repo.List: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []Summary
	for rows.Next() {
		var s Summary
		var evalDate time.Time
		var total sql.NullInt64
		var startedAt, completedAt, sealedAt sql.NullTime

		if err := rows.Scan(
			&s.ID, &s.PeriodeID, &evalDate, &s.Scope, &s.Status,
			&s.ProcessedCount, &s.ErrorCount, &total,
			&startedAt, &completedAt, &sealedAt,
			&s.CreatedAt, &s.CreatedBy,
		); err != nil {
			return nil, "", false, fmt.Errorf("calcrun.repo.List scan: %w", err)
		}
		s.EvaluationDate = evalDate
		if total.Valid {
			n := int(total.Int64)
			s.TotalInstrumen = &n
		}
		if startedAt.Valid {
			t := startedAt.Time
			s.StartedAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			s.CompletedAt = &t
		}
		if sealedAt.Valid {
			t := sealedAt.Time
			s.SealedAt = &t
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("calcrun.repo.List rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor string
	if hasMore && len(items) > 0 {
		nextCursor = items[len(items)-1].ID.String()
	}
	return items, nextCursor, hasMore, nil
}

// UpdateStatus transitions the status in the same transaction.
// Returns the updated CalcRun.
func (r *Repo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, newStatus Status, updatedBy uuid.UUID) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1, updated_by = $2, updated_at = now()
WHERE id = $3 AND deleted_at IS NULL`,
		string(newStatus), updatedBy, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateStatus: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateStartFields sets status=IN_PROGRESS, parameter_snapshot_jsonb, job_id, started_at, total_instrumen.
func (r *Repo) UpdateStartFields(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	snapshot json.RawMessage, jobID string, totalCount int, updatedBy uuid.UUID) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    parameter_snapshot_jsonb = $2,
    job_id = $3,
    started_at = now(),
    total_instrumen = $4,
    updated_by = $5,
    updated_at = now()
WHERE id = $6 AND deleted_at IS NULL`,
		string(StatusInProgress), []byte(snapshot), jobID, totalCount, updatedBy, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateStartFields: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateProgress increments processed_count and error_count (non-transactional for perf).
func (r *Repo) UpdateProgress(ctx context.Context, id uuid.UUID, processed, errors int, updatedBy uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE ecl.calc_run
SET processed_count = $1, error_count = $2, updated_by = $3, updated_at = now()
WHERE id = $4 AND deleted_at IS NULL`,
		processed, errors, updatedBy, id)
	if err != nil {
		return fmt.Errorf("calcrun.repo.UpdateProgress: %w", err)
	}
	return nil
}

// UpdateCompletion sets status=COMPLETED or COMPLETED_WITH_ERRORS + completed_at.
func (r *Repo) UpdateCompletion(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	finalStatus Status, processed, errors int, updatedBy uuid.UUID) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    processed_count = $2,
    error_count = $3,
    completed_at = now(),
    updated_by = $4,
    updated_at = now()
WHERE id = $5 AND deleted_at IS NULL`,
		string(finalStatus), processed, errors, updatedBy, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateCompletion: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateSealRequest sets status=SEAL_REQUESTED + seal_requested_by/at/comment.
func (r *Repo) UpdateSealRequest(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	requestedBy uuid.UUID, comment string) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    seal_requested_by = $2,
    seal_requested_at = now(),
    updated_by = $2,
    updated_at = now()
WHERE id = $3 AND deleted_at IS NULL`,
		string(StatusSealRequested), requestedBy, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateSealRequest: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateSealApprove sets status=SEALED + seal_approved_by/at + sealed_at + signature.
func (r *Repo) UpdateSealApprove(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	approverID uuid.UUID, signatureHash []byte) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    seal_approved_by = $2,
    seal_approved_at = now(),
    sealed_at = now(),
    signature_hash_seal = $3,
    updated_by = $2,
    updated_at = now()
WHERE id = $4 AND deleted_at IS NULL`,
		string(StatusSealed), approverID, signatureHash, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateSealApprove: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateSealReject sets status=COMPLETED + seal_rejected_by/at/reason.
// Clears seal_requested_by/at so a re-request is possible.
func (r *Repo) UpdateSealReject(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	rejectedBy uuid.UUID, reason string) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    seal_rejected_by = $2,
    seal_rejected_at = now(),
    reject_reason = $3,
    seal_requested_by = NULL,
    seal_requested_at = NULL,
    updated_by = $2,
    updated_at = now()
WHERE id = $4 AND deleted_at IS NULL`,
		string(StatusCompleted), rejectedBy, reason, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateSealReject: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// UpdateCancel sets status=CANCELLED + cancelled_by/at/reason.
func (r *Repo) UpdateCancel(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	cancelledBy uuid.UUID, reason string) (CalcRun, error) {
	_, err := tx.ExecContext(ctx, `
UPDATE ecl.calc_run
SET status = $1,
    cancelled_by = $2,
    cancelled_at = now(),
    cancel_reason = $3,
    updated_by = $2,
    updated_at = now()
WHERE id = $4 AND deleted_at IS NULL`,
		string(StatusCancelled), cancelledBy, reason, id)
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.UpdateCancel: %w", err)
	}
	return r.getByIDTx(ctx, tx, id)
}

// CheckExistingInProgress returns id of an IN_PROGRESS calc_run for the given periodeID
// (empty string if none).
func (r *Repo) CheckExistingInProgress(ctx context.Context, periodeID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM ecl.calc_run
         WHERE periode_id = $1 AND status = 'IN_PROGRESS' AND deleted_at IS NULL
         LIMIT 1`,
		periodeID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("calcrun.repo.CheckExistingInProgress: %w", err)
	}
	return id, nil
}

// CheckExistingSealed returns id of a SEALED calc_run for the given periodeID
// (empty string if none).
func (r *Repo) CheckExistingSealed(ctx context.Context, periodeID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM ecl.calc_run
         WHERE periode_id = $1 AND status = 'SEALED' AND deleted_at IS NULL
         LIMIT 1`,
		periodeID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("calcrun.repo.CheckExistingSealed: %w", err)
	}
	return id, nil
}

// BeginTx starts a new database transaction.
func (r *Repo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

const calcRunSelectSQL = `
SELECT id, periode_id, evaluation_date, scope, status,
       job_id,
       total_instrumen, processed_count, error_count,
       started_at, completed_at,
       parameter_snapshot_jsonb,
       seal_requested_by, seal_requested_at,
       seal_approved_by, seal_approved_at,
       sealed_at, signature_hash_seal,
       seal_rejected_by, seal_rejected_at, reject_reason,
       cancelled_by, cancelled_at, cancel_reason,
       superseded_by_run_id,
       created_at, created_by, updated_at, updated_by, row_version, tenant_id
FROM ecl.calc_run`

func scanCalcRun(row *sql.Row) (CalcRun, error) {
	var c CalcRun
	var evalDate time.Time
	var jobID sql.NullString
	var totalInstrumen sql.NullInt64
	var startedAt, completedAt sql.NullTime
	var snapshot []byte
	var sealReqBy, sealApprBy, sealRejBy, cancelledBy sql.NullString
	var sealReqAt, sealApprAt, sealedAt, sealRejAt, cancelledAt sql.NullTime
	var sigHash []byte
	var rejectReason, cancelReason sql.NullString
	var supersededByRunID sql.NullString
	var status string

	err := row.Scan(
		&c.ID, &c.PeriodeID, &evalDate, &c.Scope, &status,
		&jobID,
		&totalInstrumen, &c.ProcessedCount, &c.ErrorCount,
		&startedAt, &completedAt,
		&snapshot,
		&sealReqBy, &sealReqAt,
		&sealApprBy, &sealApprAt,
		&sealedAt, &sigHash,
		&sealRejBy, &sealRejAt, &rejectReason,
		&cancelledBy, &cancelledAt, &cancelReason,
		&supersededByRunID,
		&c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy, &c.RowVersion, &c.TenantID,
	)
	if err == sql.ErrNoRows {
		return CalcRun{}, ErrCalcRunNotFound("(unknown)")
	}
	if err != nil {
		return CalcRun{}, fmt.Errorf("calcrun.repo.scan: %w", err)
	}

	c.EvaluationDate = evalDate
	c.Status = Status(status)
	if jobID.Valid {
		c.JobID = &jobID.String
	}
	if totalInstrumen.Valid {
		n := int(totalInstrumen.Int64)
		c.TotalInstrumen = &n
	}
	if startedAt.Valid {
		t := startedAt.Time
		c.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		c.CompletedAt = &t
	}
	c.ParameterSnapshotJSONB = snapshot
	if sealReqBy.Valid {
		u, err := uuid.Parse(sealReqBy.String)
		if err != nil {
			return CalcRun{}, fmt.Errorf("calcrun.repo.scan: seal_requested_by UUID: %w", err)
		}
		c.SealRequestedBy = &u
	}
	if sealReqAt.Valid {
		t := sealReqAt.Time
		c.SealRequestedAt = &t
	}
	if sealApprBy.Valid {
		u, err := uuid.Parse(sealApprBy.String)
		if err != nil {
			return CalcRun{}, fmt.Errorf("calcrun.repo.scan: seal_approved_by UUID: %w", err)
		}
		c.SealApprovedBy = &u
	}
	if sealApprAt.Valid {
		t := sealApprAt.Time
		c.SealApprovedAt = &t
	}
	if sealedAt.Valid {
		t := sealedAt.Time
		c.SealedAt = &t
	}
	c.SignatureHashSeal = sigHash
	if sealRejBy.Valid {
		u, err := uuid.Parse(sealRejBy.String)
		if err != nil {
			return CalcRun{}, fmt.Errorf("calcrun.repo.scan: seal_rejected_by UUID: %w", err)
		}
		c.SealRejectedBy = &u
	}
	if sealRejAt.Valid {
		t := sealRejAt.Time
		c.SealRejectedAt = &t
	}
	if rejectReason.Valid {
		c.RejectReason = &rejectReason.String
	}
	if cancelledBy.Valid {
		u, err := uuid.Parse(cancelledBy.String)
		if err != nil {
			return CalcRun{}, fmt.Errorf("calcrun.repo.scan: cancelled_by UUID: %w", err)
		}
		c.CancelledBy = &u
	}
	if cancelledAt.Valid {
		t := cancelledAt.Time
		c.CancelledAt = &t
	}
	if cancelReason.Valid {
		c.CancelReason = &cancelReason.String
	}
	if supersededByRunID.Valid {
		u, err := uuid.Parse(supersededByRunID.String)
		if err != nil {
			return CalcRun{}, fmt.Errorf("calcrun.repo.scan: superseded_by_run_id UUID: %w", err)
		}
		c.SupersededByRunID = &u
	}
	return c, nil
}

// queryer is the common interface for *sql.DB and *sql.Tx row queries.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
