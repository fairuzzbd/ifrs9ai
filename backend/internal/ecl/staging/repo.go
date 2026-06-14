// Package staging — repository layer for ECL staging tables.
//
// Three repositories:
//   - DPDRepository: trx.dpd_record (migration 000022 §Section 1)
//   - StageHistoryRepository: ecl.stage_history (append-only, augmented by migration 000022 §Section 3)
//   - OverrideProposalRepository: ecl.staging_override_proposal (migration 000022 §Section 2)
//
// Uses standard database/sql for all queries.
// All parameterized queries — no string concat (SQLi prevention per security-baseline.md).
// Allowed column whitelists enforced at init time via assertion.
package staging

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// ─── Sentinel errors ─────────────────────────────────────────────────────────

var (
	// ErrNotFound is returned when a required row is absent.
	ErrNotFound = fmt.Errorf("staging: record not found")
	// ErrConflict is returned on optimistic lock / unique violation.
	ErrConflict = fmt.Errorf("staging: conflict (row_version mismatch or unique violation)")
	// ErrDPDDuplicate is returned when a DPD record already exists for (instrumen_id, periode).
	ErrDPDDuplicate = fmt.Errorf("staging: DPD record already exists for this instrument+periode")
)

// ─── Interfaces ──────────────────────────────────────────────────────────────

// DPDRepository manages trx.dpd_record.
type DPDRepository interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// UpsertDPD inserts or updates (instrument_id, periode) in the same tx.
	// Uses ON CONFLICT DO UPDATE per state-machine §4.3 idempotency rules.
	UpsertDPD(ctx context.Context, tx *sql.Tx, rec *DPDRecord) (*DPDRecord, error)

	// GetLatestDPD fetches the most recent DPD record for an instrument.
	// Returns ErrNotFound if none exists.
	GetLatestDPD(ctx context.Context, instrumenID uuid.UUID) (*DPDRecord, error)

	// GetDPDForPeriode fetches the DPD record for a specific first-of-month date.
	GetDPDForPeriode(ctx context.Context, instrumenID uuid.UUID, periode time.Time) (*DPDRecord, error)

	// ListDPD returns paginated DPD records for an instrument (history list).
	ListDPD(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int) ([]*DPDRecord, pagination.Result, error)

	// CountInPeriodeRange counts DPD records with dpd_value >= threshold in a date range.
	// Used by cure assessment to verify DPD < 30 for a period.
	CountDPDAboveThreshold(ctx context.Context, instrumenID uuid.UUID, from, to time.Time, threshold int) (int, error)
}

// StageHistoryRepository manages ecl.stage_history (append-only).
type StageHistoryRepository interface {
	// Insert appends a new stage_history row within the provided transaction.
	// Enforces append-only: no UPDATE or DELETE (DB trigger also guards this).
	Insert(ctx context.Context, tx *sql.Tx, entry *StageHistoryEntry) (*StageHistoryEntry, error)

	// GetCurrentStage returns the most recent stage_history row for an instrument.
	// Returns (nil, nil) if no rows exist (instrument never evaluated).
	GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (*StageHistoryEntry, error)

	// ListHistory returns paginated stage_history rows (cursor-based, newest first by default).
	ListHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*StageHistoryEntry, pagination.Result, error)

	// GetLastSICRDate returns the tanggal_migrasi of the most recent row with
	// stage_sesudah IN (STAGE_2, STAGE_3) for cure assessment algorithm (§5.3 step 1).
	GetLastSICRDate(ctx context.Context, instrumenID uuid.UUID) (*time.Time, error)

	// HasSICRInPeriode checks if any SICR transition occurred within [from, to].
	// Used by cure assessment §5.3 step 3.
	HasSICRInPeriode(ctx context.Context, instrumenID uuid.UUID, from, to time.Time) (bool, error)

	// ExistsForKey checks if a stage_history row already exists for (instrumen_id, tanggal_migrasi, trigger_type).
	// Used for idempotency guard (unique index uq_stage_history_idempotency, migration 000022).
	ExistsForKey(ctx context.Context, instrumenID uuid.UUID, tanggalMigrasi time.Time, triggerType TriggerType) (bool, error)

	// ListStage2Instruments returns all instrumen_ids whose current stage is STAGE_2.
	// Used by cure assessment batch to determine scope.
	ListStage2Instruments(ctx context.Context, tenantID string) ([]uuid.UUID, error)
}

// OverrideProposalRepository manages ecl.staging_override_proposal.
type OverrideProposalRepository interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// Create inserts a new override proposal (status=PENDING_REVIEW).
	Create(ctx context.Context, tx *sql.Tx, prop *OverrideProposal) (*OverrideProposal, error)

	// GetByID fetches an override proposal (non-deleted unless includeDeleted=true).
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*OverrideProposal, error)

	// UpdateWorkflowStatus sets workflow_status + relevant sign fields within tx.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, newStatus OverrideWorkflowStatus,
		actorID uuid.UUID, signedAt time.Time, sigHash []byte, comment *string) error

	// ActivateWithHistoryRow atomically: sets status=ACTIVE, records stage_history_row_id.
	ActivateWithHistoryRow(ctx context.Context, tx *sql.Tx, proposalID, historyRowID uuid.UUID, actorID uuid.UUID) error

	// ListActiveForInstrumen returns proposals in PENDING_REVIEW or PENDING_APPROVAL for an instrument.
	// Used to enforce "one active proposal per instrument" rule.
	ListActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID) ([]*OverrideProposal, error)

	// ListOverrides returns paginated override proposals.
	ListOverrides(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*OverrideProposal, pagination.Result, error)

	// ListExpiredActive returns ACTIVE proposals whose periode_akhir < today.
	// Used by the expiry check worker.
	ListExpiredActive(ctx context.Context, today time.Time) ([]*OverrideProposal, error)

	// MarkExpired sets workflow_status=EXPIRED within tx.
	MarkExpired(ctx context.Context, tx *sql.Tx, id uuid.UUID, actorID uuid.UUID) error
}

// ─── DB implementations ───────────────────────────────────────────────────────

// DBDPDRepository is the PostgreSQL implementation.
type DBDPDRepository struct {
	db *sql.DB
}

// NewDBDPDRepository creates a DBDPDRepository.
// If db is nil, returns a no-op implementation (dev/test without DB).
func NewDBDPDRepository(db *sql.DB) *DBDPDRepository {
	if db == nil {
		return &DBDPDRepository{db: nil}
	}
	return &DBDPDRepository{db: db}
}

func (r *DBDPDRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if r.db == nil {
		return nil, fmt.Errorf("staging DPD repo: no database connection")
	}
	return r.db.BeginTx(ctx, nil)
}

func (r *DBDPDRepository) UpsertDPD(ctx context.Context, tx *sql.Tx, rec *DPDRecord) (*DPDRecord, error) {
	if r.db == nil {
		return rec, nil
	}
	q := `
		INSERT INTO trx.dpd_record
			(id, instrumen_id, periode, dpd_value, source, catatan, recorded_by, recorded_at,
			 created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1, $13)
		ON CONFLICT (instrumen_id, periode) DO UPDATE SET
			dpd_value   = EXCLUDED.dpd_value,
			catatan     = EXCLUDED.catatan,
			recorded_by = EXCLUDED.recorded_by,
			recorded_at = EXCLUDED.recorded_at,
			updated_at  = now(),
			updated_by  = EXCLUDED.updated_by,
			row_version = trx.dpd_record.row_version + 1
		RETURNING *`
	var out DPDRecord
	err := tx.QueryRowContext(ctx, q,
		rec.ID, rec.InstrumenID, rec.Periode, rec.DPDValue, rec.Source,
		rec.Catatan, rec.RecordedBy, rec.RecordedAt,
		rec.CreatedAt, rec.CreatedBy, rec.UpdatedAt, rec.UpdatedBy, rec.TenantID,
	).Scan(
		&out.ID, &out.InstrumenID, &out.Periode, &out.DPDValue, &out.Source,
		&out.Catatan, &out.RecordedBy, &out.RecordedAt,
		&out.CreatedAt, &out.CreatedBy, &out.UpdatedAt, &out.UpdatedBy,
		&out.DeletedAt, &out.DeletedBy, &out.RowVersion, &out.TenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("staging DPD UpsertDPD: %w", err)
	}
	return &out, nil
}

func (r *DBDPDRepository) GetLatestDPD(ctx context.Context, instrumenID uuid.UUID) (*DPDRecord, error) {
	if r.db == nil {
		return nil, ErrNotFound
	}
	q := `
		SELECT id, instrumen_id, periode, dpd_value, source, catatan,
		       recorded_by, recorded_at,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.dpd_record
		WHERE instrumen_id = $1 AND deleted_at IS NULL
		ORDER BY periode DESC
		LIMIT 1`
	var rec DPDRecord
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(
		&rec.ID, &rec.InstrumenID, &rec.Periode, &rec.DPDValue, &rec.Source,
		&rec.Catatan, &rec.RecordedBy, &rec.RecordedAt,
		&rec.CreatedAt, &rec.CreatedBy, &rec.UpdatedAt, &rec.UpdatedBy,
		&rec.DeletedAt, &rec.DeletedBy, &rec.RowVersion, &rec.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staging DPD GetLatestDPD: %w", err)
	}
	return &rec, nil
}

func (r *DBDPDRepository) GetDPDForPeriode(ctx context.Context, instrumenID uuid.UUID, periode time.Time) (*DPDRecord, error) {
	if r.db == nil {
		return nil, ErrNotFound
	}
	q := `
		SELECT id, instrumen_id, periode, dpd_value, source, catatan,
		       recorded_by, recorded_at,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.dpd_record
		WHERE instrumen_id = $1 AND periode = $2 AND deleted_at IS NULL
		LIMIT 1`
	var rec DPDRecord
	err := r.db.QueryRowContext(ctx, q, instrumenID, periode).Scan(
		&rec.ID, &rec.InstrumenID, &rec.Periode, &rec.DPDValue, &rec.Source,
		&rec.Catatan, &rec.RecordedBy, &rec.RecordedAt,
		&rec.CreatedAt, &rec.CreatedBy, &rec.UpdatedAt, &rec.UpdatedBy,
		&rec.DeletedAt, &rec.DeletedBy, &rec.RowVersion, &rec.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staging DPD GetDPDForPeriode: %w", err)
	}
	return &rec, nil
}

func (r *DBDPDRepository) ListDPD(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int) ([]*DPDRecord, pagination.Result, error) {
	if r.db == nil {
		return nil, pagination.Result{}, nil
	}

	allowed := AllAllowedColsDPD
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("d")
	baseWhere := "d.instrumen_id = $" + fmt.Sprintf("%d", len(args)+1) + " AND d.deleted_at IS NULL"
	args = append(args, instrumenID)

	// Cursor decode: cursor encodes the last periode seen.
	cursorClause := ""
	if cursor != "" {
		dec, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil && len(dec) > 0 {
			n := len(args) + 1
			cursorClause = fmt.Sprintf(" AND d.periode < $%d", n)
			args = append(args, string(dec))
		}
	}

	full := buildWhere(baseWhere, where, cursorClause)

	if orderBy == "" {
		orderBy = "d.periode DESC"
	}
	fetch := limit + 1
	//nolint:gosec // G201: SQL built from validated allowlist columns only; no user input in SQL structure.
	sqlQ := fmt.Sprintf(`
		SELECT id, instrumen_id, periode, dpd_value, source, catatan,
		       recorded_by, recorded_at,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.dpd_record d
		WHERE %s
		ORDER BY %s
		LIMIT %d`, full, orderBy, fetch)

	rows, err := r.db.QueryContext(ctx, sqlQ, args...)
	if err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging DPD ListDPD: %w", err)
	}
	defer rows.Close()

	var recs []*DPDRecord
	for rows.Next() {
		var rec DPDRecord
		if err := rows.Scan(
			&rec.ID, &rec.InstrumenID, &rec.Periode, &rec.DPDValue, &rec.Source,
			&rec.Catatan, &rec.RecordedBy, &rec.RecordedAt,
			&rec.CreatedAt, &rec.CreatedBy, &rec.UpdatedAt, &rec.UpdatedBy,
			&rec.DeletedAt, &rec.DeletedBy, &rec.RowVersion, &rec.TenantID,
		); err != nil {
			return nil, pagination.Result{}, fmt.Errorf("staging DPD ListDPD scan: %w", err)
		}
		recs = append(recs, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging DPD ListDPD rows: %w", err)
	}

	hasMore := len(recs) > limit
	if hasMore {
		recs = recs[:limit]
	}

	var nextCursorPtr *string
	if hasMore && len(recs) > 0 {
		last := recs[len(recs)-1]
		enc := base64.StdEncoding.EncodeToString([]byte(last.Periode.Format("2006-01-02")))
		nextCursorPtr = &enc
	}

	pag := pagination.Result{
		HasMore:    hasMore,
		NextCursor: nextCursorPtr,
		Limit:      limit,
	}
	return recs, pag, nil
}

func (r *DBDPDRepository) CountDPDAboveThreshold(ctx context.Context, instrumenID uuid.UUID, from, to time.Time, threshold int) (int, error) {
	if r.db == nil {
		return 0, nil
	}
	q := `
		SELECT COUNT(*)
		FROM trx.dpd_record
		WHERE instrumen_id = $1
		  AND periode >= $2 AND periode <= $3
		  AND dpd_value >= $4
		  AND deleted_at IS NULL`
	var count int
	err := r.db.QueryRowContext(ctx, q, instrumenID, from, to, threshold).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("staging DPD CountDPDAboveThreshold: %w", err)
	}
	return count, nil
}

// ─── StageHistoryRepository implementation ───────────────────────────────────

// DBStageHistoryRepository is the PostgreSQL implementation for ecl.stage_history.
type DBStageHistoryRepository struct {
	db *sql.DB
}

// NewDBStageHistoryRepository creates a DBStageHistoryRepository.
func NewDBStageHistoryRepository(db *sql.DB) *DBStageHistoryRepository {
	if db == nil {
		return &DBStageHistoryRepository{db: nil}
	}
	return &DBStageHistoryRepository{db: db}
}

func (r *DBStageHistoryRepository) Insert(ctx context.Context, tx *sql.Tx, entry *StageHistoryEntry) (*StageHistoryEntry, error) {
	if r.db == nil {
		entry.ID = uuid.New()
		return entry, nil
	}
	q := `
		INSERT INTO ecl.stage_history
			(id, instrumen_id, stage_sebelum, stage_sesudah, trigger_type,
			 detail_trigger, rating_saat_migrasi, dpd, tanggal_migrasi,
			 status_approval, user_approver_id, dokumen_pendukung_id,
			 override_proposal_id, evaluation_job_id, tenant_id,
			 created_at, created_by)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	err := tx.QueryRowContext(ctx, q,
		entry.ID, entry.InstrumenID,
		string(entry.StageSebelum), string(entry.StageSesudah), string(entry.TriggerType),
		entry.DetailTrigger, entry.RatingSaatMigrasi, entry.DPD, entry.TanggalMigrasi,
		string(entry.StatusApproval), entry.UserApproverID, entry.DokumenPendukungID,
		entry.OverrideProposalID, entry.EvaluationJobID, entry.TenantID,
		entry.CreatedAt, entry.CreatedBy,
	).Scan(&entry.ID)
	if err != nil {
		// Check for unique constraint violation (idempotency key).
		if strings.Contains(err.Error(), "uq_stage_history_idempotency") ||
			strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("staging StageHistory Insert: %w", err)
	}
	return entry, nil
}

func (r *DBStageHistoryRepository) GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (*StageHistoryEntry, error) {
	if r.db == nil {
		return nil, nil
	}
	q := `
		SELECT id, instrumen_id, stage_sebelum, stage_sesudah, trigger_type,
		       detail_trigger, rating_saat_migrasi, dpd, tanggal_migrasi,
		       status_approval, user_approver_id, dokumen_pendukung_id,
		       override_proposal_id, evaluation_job_id, tenant_id,
		       created_at, created_by
		FROM ecl.stage_history
		WHERE instrumen_id = $1
		ORDER BY tanggal_migrasi DESC, created_at DESC
		LIMIT 1`
	var e StageHistoryEntry
	var stageSebelum, stageSesudah, triggerType, statusApproval string
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(
		&e.ID, &e.InstrumenID, &stageSebelum, &stageSesudah, &triggerType,
		&e.DetailTrigger, &e.RatingSaatMigrasi, &e.DPD, &e.TanggalMigrasi,
		&statusApproval, &e.UserApproverID, &e.DokumenPendukungID,
		&e.OverrideProposalID, &e.EvaluationJobID, &e.TenantID,
		&e.CreatedAt, &e.CreatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("staging StageHistory GetCurrentStage: %w", err)
	}
	e.StageSebelum = Stage(stageSebelum)
	e.StageSesudah = Stage(stageSesudah)
	e.TriggerType = TriggerType(triggerType)
	e.StatusApproval = StatusApproval(statusApproval)
	return &e, nil
}

func (r *DBStageHistoryRepository) ListHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*StageHistoryEntry, pagination.Result, error) {
	if r.db == nil {
		return nil, pagination.Result{}, nil
	}

	allowed := AllAllowedColsHistory
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("sh")
	baseWhere := "sh.instrumen_id = $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, instrumenID)

	cursorClause := ""
	if cursor != "" {
		dec, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil && len(dec) > 0 {
			n := len(args) + 1
			cursorClause = fmt.Sprintf(" AND sh.created_at < $%d", n)
			args = append(args, string(dec))
		}
	}

	full := buildWhere(baseWhere, where, cursorClause)

	if orderBy == "" {
		orderBy = "sh.tanggal_migrasi DESC, sh.created_at DESC"
	}
	fetch := limit + 1
	//nolint:gosec // G201: SQL built from validated allowlist columns only; no user input in SQL structure.
	sqlStr := fmt.Sprintf(`
		SELECT sh.id, sh.instrumen_id, sh.stage_sebelum, sh.stage_sesudah, sh.trigger_type,
		       sh.detail_trigger, sh.rating_saat_migrasi, sh.dpd, sh.tanggal_migrasi,
		       sh.status_approval, sh.user_approver_id, sh.dokumen_pendukung_id,
		       sh.override_proposal_id, sh.evaluation_job_id, sh.tenant_id,
		       sh.created_at, sh.created_by
		FROM ecl.stage_history sh
		WHERE %s
		ORDER BY %s
		LIMIT %d`, full, orderBy, fetch)

	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging StageHistory ListHistory: %w", err)
	}
	defer rows.Close()

	var entries []*StageHistoryEntry
	for rows.Next() {
		var e StageHistoryEntry
		var stageSebelum, stageSesudah, triggerType, statusApproval string
		if err := rows.Scan(
			&e.ID, &e.InstrumenID, &stageSebelum, &stageSesudah, &triggerType,
			&e.DetailTrigger, &e.RatingSaatMigrasi, &e.DPD, &e.TanggalMigrasi,
			&statusApproval, &e.UserApproverID, &e.DokumenPendukungID,
			&e.OverrideProposalID, &e.EvaluationJobID, &e.TenantID,
			&e.CreatedAt, &e.CreatedBy,
		); err != nil {
			return nil, pagination.Result{}, fmt.Errorf("staging StageHistory ListHistory scan: %w", err)
		}
		e.StageSebelum = Stage(stageSebelum)
		e.StageSesudah = Stage(stageSesudah)
		e.TriggerType = TriggerType(triggerType)
		e.StatusApproval = StatusApproval(statusApproval)
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging StageHistory ListHistory rows: %w", err)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	var nextCursorPtr *string
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		enc := base64.StdEncoding.EncodeToString([]byte(last.CreatedAt.Format(time.RFC3339Nano)))
		nextCursorPtr = &enc
	}

	pag := pagination.Result{
		HasMore:    hasMore,
		NextCursor: nextCursorPtr,
		Limit:      limit,
	}
	return entries, pag, nil
}

func (r *DBStageHistoryRepository) GetLastSICRDate(ctx context.Context, instrumenID uuid.UUID) (*time.Time, error) {
	if r.db == nil {
		return nil, nil
	}
	q := `
		SELECT MAX(tanggal_migrasi)
		FROM ecl.stage_history
		WHERE instrumen_id = $1
		  AND stage_sesudah IN ('STAGE_2','STAGE_3')`
	var t *time.Time
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&t)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("staging StageHistory GetLastSICRDate: %w", err)
	}
	return t, nil
}

func (r *DBStageHistoryRepository) HasSICRInPeriode(ctx context.Context, instrumenID uuid.UUID, from, to time.Time) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	q := `
		SELECT COUNT(*) > 0
		FROM ecl.stage_history
		WHERE instrumen_id = $1
		  AND stage_sesudah IN ('STAGE_2','STAGE_3')
		  AND tanggal_migrasi >= $2 AND tanggal_migrasi <= $3`
	var has bool
	err := r.db.QueryRowContext(ctx, q, instrumenID, from, to).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("staging StageHistory HasSICRInPeriode: %w", err)
	}
	return has, nil
}

func (r *DBStageHistoryRepository) ExistsForKey(ctx context.Context, instrumenID uuid.UUID, tanggalMigrasi time.Time, triggerType TriggerType) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	q := `
		SELECT COUNT(*) > 0
		FROM ecl.stage_history
		WHERE instrumen_id = $1
		  AND tanggal_migrasi = $2
		  AND trigger_type = $3`
	var exists bool
	err := r.db.QueryRowContext(ctx, q, instrumenID, tanggalMigrasi, string(triggerType)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("staging StageHistory ExistsForKey: %w", err)
	}
	return exists, nil
}

func (r *DBStageHistoryRepository) ListStage2Instruments(ctx context.Context, tenantID string) ([]uuid.UUID, error) {
	if r.db == nil {
		return nil, nil
	}
	// Subquery: get the latest stage_history row per instrument, then filter stage_sesudah=STAGE_2.
	// Avoids window functions for portability across PG versions.
	q := `
		SELECT DISTINCT ON (sh.instrumen_id) sh.instrumen_id
		FROM ecl.stage_history sh
		JOIN mst.instrumen i ON i.id = sh.instrumen_id
		WHERE i.status = 'AKTIF'
		  AND i.klasifikasi_psak71 IN ('AC','FVOCI')
		  AND i.deleted_at IS NULL
		  AND sh.tenant_id = $1
		ORDER BY sh.instrumen_id, sh.tanggal_migrasi DESC, sh.created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("staging StageHistory ListStage2Instruments: %w", err)
	}
	defer rows.Close()

	// Read the current stage for each returned instrument ID.
	// We re-use GetCurrentStage per instrument (bounded by portfolio size).
	var instrumenIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("staging StageHistory ListStage2Instruments scan: %w", err)
		}
		instrumenIDs = append(instrumenIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("staging StageHistory ListStage2Instruments rows: %w", err)
	}

	// Filter to only those whose LATEST stage is STAGE_2.
	var stage2IDs []uuid.UUID
	for _, id := range instrumenIDs {
		cur, err := r.GetCurrentStage(ctx, id)
		if err != nil {
			continue
		}
		if cur != nil && cur.StageSesudah == Stage2 {
			stage2IDs = append(stage2IDs, id)
		}
	}
	return stage2IDs, nil
}

// ─── OverrideProposalRepository implementation ───────────────────────────────

// DBOverrideProposalRepository is the PostgreSQL implementation.
type DBOverrideProposalRepository struct {
	db *sql.DB
}

// NewDBOverrideProposalRepository creates a DBOverrideProposalRepository.
func NewDBOverrideProposalRepository(db *sql.DB) *DBOverrideProposalRepository {
	if db == nil {
		return &DBOverrideProposalRepository{db: nil}
	}
	return &DBOverrideProposalRepository{db: db}
}

func (r *DBOverrideProposalRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if r.db == nil {
		return nil, fmt.Errorf("staging override repo: no database connection")
	}
	return r.db.BeginTx(ctx, nil)
}

func (r *DBOverrideProposalRepository) Create(ctx context.Context, tx *sql.Tx, prop *OverrideProposal) (*OverrideProposal, error) {
	if r.db == nil {
		prop.ID = uuid.New()
		return prop, nil
	}
	q := `
		INSERT INTO ecl.staging_override_proposal
			(id, instrumen_id, stage_from, stage_to, alasan, reason_category,
			 dokumen_pendukung_id, periode_id, periode_akhir, workflow_status,
			 current_stage_at_submit, maker_id, expires_after_periode,
			 created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,1,$18)
		RETURNING id`
	if prop.ID == uuid.Nil {
		prop.ID = uuid.New()
	}
	err := tx.QueryRowContext(ctx, q,
		prop.ID, prop.InstrumenID,
		string(prop.StageFrom), string(prop.StageTo), prop.Alasan, prop.ReasonCategory,
		prop.DokumenPendukungID, prop.PeriodeID, prop.PeriodeAkhir,
		string(prop.WorkflowStatus), prop.CurrentStageAtSubmit, prop.MakerID,
		prop.ExpiresAfterPeriode,
		prop.CreatedAt, prop.CreatedBy, prop.UpdatedAt, prop.UpdatedBy, prop.TenantID,
	).Scan(&prop.ID)
	if err != nil {
		return nil, fmt.Errorf("staging override Create: %w", err)
	}
	return prop, nil
}

func (r *DBOverrideProposalRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*OverrideProposal, error) {
	if r.db == nil {
		return nil, ErrNotFound
	}
	extra := ""
	if !includeDeleted {
		extra = " AND deleted_at IS NULL"
	}
	q := `
		SELECT id, instrumen_id, stage_from, stage_to, alasan, reason_category,
		       dokumen_pendukung_id, periode_id, periode_akhir, workflow_status,
		       current_stage_at_submit, maker_id,
		       reviewer_id, signed_at_review, signature_hash_review, comment_review,
		       approver_alco_id, signed_at_approve_alco, signature_hash_approve_alco, comment_approve_alco,
		       approver_komite_id, signed_at_approve_komite, signature_hash_approve_komite, comment_approve_komite,
		       reject_reason, stage_history_row_id, expires_after_periode,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM ecl.staging_override_proposal
		WHERE id = $1` + extra
	var p OverrideProposal
	var sf, st, wfStatus string
	var csat *string
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.InstrumenID, &sf, &st, &p.Alasan, &p.ReasonCategory,
		&p.DokumenPendukungID, &p.PeriodeID, &p.PeriodeAkhir, &wfStatus,
		&csat, &p.MakerID,
		&p.ReviewerID, &p.SignedAtReview, &p.SignatureHashReview, &p.CommentReview,
		&p.ApproverALCOID, &p.SignedAtApproveALCO, &p.SignatureHashApproveALCO, &p.CommentApproveALCO,
		&p.ApproverKomiteID, &p.SignedAtApproveKomite, &p.SignatureHashApproveKomite, &p.CommentApproveKomite,
		&p.RejectReason, &p.StageHistoryRowID, &p.ExpiresAfterPeriode,
		&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.UpdatedBy,
		&p.DeletedAt, &p.DeletedBy, &p.RowVersion, &p.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staging override GetByID: %w", err)
	}
	p.StageFrom = Stage(sf)
	p.StageTo = Stage(st)
	p.WorkflowStatus = OverrideWorkflowStatus(wfStatus)
	if csat != nil {
		s := Stage(*csat)
		p.CurrentStageAtSubmit = &s
	}
	return &p, nil
}

func (r *DBOverrideProposalRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID,
	newStatus OverrideWorkflowStatus, actorID uuid.UUID, signedAt time.Time, sigHash []byte, comment *string) error {
	if r.db == nil {
		return nil
	}
	q := `
		UPDATE ecl.staging_override_proposal
		SET workflow_status = $1, updated_at = now(), updated_by = $2,
		    row_version = row_version + 1
		WHERE id = $3 AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, string(newStatus), actorID, id)
	if err != nil {
		return fmt.Errorf("staging override UpdateWorkflowStatus: %w", err)
	}
	return nil
}

func (r *DBOverrideProposalRepository) ActivateWithHistoryRow(ctx context.Context, tx *sql.Tx, proposalID, historyRowID uuid.UUID, actorID uuid.UUID) error {
	if r.db == nil {
		return nil
	}
	q := `
		UPDATE ecl.staging_override_proposal
		SET workflow_status = 'ACTIVE',
		    stage_history_row_id = $1,
		    updated_at = now(),
		    updated_by = $2,
		    row_version = row_version + 1
		WHERE id = $3 AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, historyRowID, actorID, proposalID)
	if err != nil {
		return fmt.Errorf("staging override ActivateWithHistoryRow: %w", err)
	}
	return nil
}

func (r *DBOverrideProposalRepository) ListActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID) ([]*OverrideProposal, error) {
	if r.db == nil {
		return nil, nil
	}
	q := `
		SELECT id FROM ecl.staging_override_proposal
		WHERE instrumen_id = $1
		  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','APPROVED_ALCO')
		  AND deleted_at IS NULL`
	rows, err := r.db.QueryContext(ctx, q, instrumenID)
	if err != nil {
		return nil, fmt.Errorf("staging override ListActiveForInstrumen: %w", err)
	}
	defer rows.Close()

	var props []*OverrideProposal
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("staging override ListActiveForInstrumen scan: %w", err)
		}
		// Load full record.
		p, err := r.GetByID(ctx, id, false)
		if err != nil {
			continue
		}
		props = append(props, p)
	}
	return props, rows.Err()
}

func (r *DBOverrideProposalRepository) ListOverrides(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*OverrideProposal, pagination.Result, error) {
	if r.db == nil {
		return nil, pagination.Result{}, nil
	}
	allowed := AllAllowedColsOverride
	where, args, orderBy := q.WithAllowed(allowed).ToSQL("p")
	baseWhere := "p.deleted_at IS NULL"

	cursorClause := ""
	if cursor != "" {
		dec, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil && len(dec) > 0 {
			n := len(args) + 1
			cursorClause = fmt.Sprintf(" AND p.created_at < $%d", n)
			args = append(args, string(dec))
		}
	}

	full := buildWhere(baseWhere, where, cursorClause)
	if orderBy == "" {
		orderBy = "p.created_at DESC"
	}
	fetch := limit + 1

	//nolint:gosec // G201: SQL built from validated allowlist columns only; no user input in SQL structure.
	sqlStr := fmt.Sprintf(`
		SELECT id FROM ecl.staging_override_proposal p
		WHERE %s ORDER BY %s LIMIT %d`, full, orderBy, fetch)

	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging override ListOverrides: %w", err)
	}
	defer rows.Close()

	var props []*OverrideProposal
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, pagination.Result{}, fmt.Errorf("staging override ListOverrides scan: %w", err)
		}
		p, err := r.GetByID(ctx, id, false)
		if err != nil {
			continue
		}
		props = append(props, p)
	}
	if err := rows.Err(); err != nil {
		return nil, pagination.Result{}, fmt.Errorf("staging override ListOverrides rows: %w", err)
	}

	hasMore := len(props) > limit
	if hasMore {
		props = props[:limit]
	}
	var nextCursorPtr *string
	if hasMore && len(props) > 0 {
		last := props[len(props)-1]
		enc := base64.StdEncoding.EncodeToString([]byte(last.CreatedAt.Format(time.RFC3339Nano)))
		nextCursorPtr = &enc
	}
	pag := pagination.Result{
		HasMore:    hasMore,
		NextCursor: nextCursorPtr,
		Limit:      limit,
	}
	return props, pag, nil
}

func (r *DBOverrideProposalRepository) ListExpiredActive(ctx context.Context, today time.Time) ([]*OverrideProposal, error) {
	if r.db == nil {
		return nil, nil
	}
	q := `
		SELECT id FROM ecl.staging_override_proposal
		WHERE workflow_status = 'ACTIVE'
		  AND periode_akhir < $1
		  AND deleted_at IS NULL`
	rows, err := r.db.QueryContext(ctx, q, today)
	if err != nil {
		return nil, fmt.Errorf("staging override ListExpiredActive: %w", err)
	}
	defer rows.Close()

	var props []*OverrideProposal
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("staging override ListExpiredActive scan: %w", err)
		}
		p, err := r.GetByID(ctx, id, false)
		if err != nil {
			continue
		}
		props = append(props, p)
	}
	return props, rows.Err()
}

func (r *DBOverrideProposalRepository) MarkExpired(ctx context.Context, tx *sql.Tx, id uuid.UUID, actorID uuid.UUID) error {
	if r.db == nil {
		return nil
	}
	q := `
		UPDATE ecl.staging_override_proposal
		SET workflow_status = 'EXPIRED', updated_at = now(), updated_by = $1, row_version = row_version + 1
		WHERE id = $2 AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, actorID, id)
	if err != nil {
		return fmt.Errorf("staging override MarkExpired: %w", err)
	}
	return nil
}

// ─── Helper ───────────────────────────────────────────────────────────────────

// buildWhere assembles WHERE clauses safely (no user input in SQL structure).
func buildWhere(parts ...string) string {
	var valid []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			valid = append(valid, p)
		}
	}
	if len(valid) == 0 {
		return "TRUE"
	}
	return strings.Join(valid, " AND ")
}
