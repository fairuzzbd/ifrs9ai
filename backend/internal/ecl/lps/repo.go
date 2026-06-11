// Package lps — repository layer (read-only queries + override CRUD).
//
// All queries use parameterized SQL (no string concat).
// Allowed-column whitelists validated at init time via assertion.
// Uses database/sql with lib/pq driver (same as rest of codebase).
//
// Design:
//   - LPSCoverageRepo: reads mst.lps_coverage (APPROVED param rows).
//   - DepositoInstrumenRepo: reads mst.instrumen + counterparty for DEPOSITO aggregation.
//   - OverrideRepo: CRUD on ecl.lps_exclusion_override (no hard-delete — ecl schema rule).
//
// Decimal precision: NUMERIC columns read via ::text cast to avoid float64 (DEC-016).
// No float64 at any point in money/rate handling.
package lps

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── LPSCoverageRepo ──────────────────────────────────────────────────────────

// LPSCoverageRepoIface defines the read interface for mst.lps_coverage.
//
//nolint:revive // name intentionally prefixed with LPS for package cross-reference clarity
type LPSCoverageRepoIface interface {
	// GetActiveByEvaluationDate returns the APPROVED lps_coverage record
	// valid on evaluationDate (periode_berlaku_dari <= date <= periode_berlaku_sampai or NULL).
	// Returns (nil, nil) if no record found (caller returns ErrLPSCoverageNoActiveParam).
	GetActiveByEvaluationDate(ctx context.Context, evalDate time.Time) (*LPSCoverageRow, error)
}

// DBLPSCoverageRepo implements LPSCoverageRepoIface against mst.lps_coverage.
type DBLPSCoverageRepo struct {
	db *sql.DB
}

// NewDBLPSCoverageRepo creates a DBLPSCoverageRepo.
func NewDBLPSCoverageRepo(db *sql.DB) *DBLPSCoverageRepo {
	return &DBLPSCoverageRepo{db: db}
}

const lpsCoverageActiveQuery = `
SELECT id, coverage_amount::text, mata_uang,
       periode_berlaku_dari, periode_berlaku_sampai, workflow_status
FROM mst.lps_coverage
WHERE workflow_status = 'APPROVED'
  AND periode_berlaku_dari <= $1
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)
ORDER BY periode_berlaku_dari DESC
LIMIT 1`

// GetActiveByEvaluationDate fetches the active APPROVED LPS coverage parameter.
func (r *DBLPSCoverageRepo) GetActiveByEvaluationDate(ctx context.Context, evalDate time.Time) (*LPSCoverageRow, error) {
	if r.db == nil {
		return nil, nil
	}
	var row LPSCoverageRow
	var amountStr string
	var sampai *time.Time
	err := r.db.QueryRowContext(ctx, lpsCoverageActiveQuery, evalDate).Scan(
		&row.ID, &amountStr, &row.MataUang,
		&row.PeriodeBerlakuDari, &sampai, &row.WorkflowStatus,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.PeriodeBerlakuSampai = sampai
	if row.CoverageAmountIDR, err = decimal.NewFromString(amountStr); err != nil {
		return nil, fmt.Errorf("lps_coverage.coverage_amount parse: %w", err)
	}
	return &row, nil
}

// ─── DepositoInstrumenRepo ────────────────────────────────────────────────────

// DepositoInstrumenRepoIface defines queries for DEPOSITO instrument data.
type DepositoInstrumenRepoIface interface {
	// ListByNasabahBank returns DEPOSITO instruments for a (nasabah, bank) pair,
	// ordered by tanggal_penempatan ASC, id ASC (FIFO). Filters: klasifikasi IN (AC, FVOCI_DEBT).
	ListByNasabahBank(ctx context.Context, nasabahID, bankID uuid.UUID, evalDate time.Time) ([]InstrumenDepositoRow, error)

	// ListAllActivePairs returns all unique (nasabah, bank) pairs with active DEPOSITO instruments
	// on or before evalDate. Used by BulkAggregate to enumerate all pairs.
	ListAllActivePairs(ctx context.Context, evalDate time.Time) ([]NasabahBankPair, error)

	// BulkListDepositoForAggregate is the N+1-free batch query used by BulkAggregate.
	// Returns all DEPOSITO instruments ordered by (counterparty_id, bank_counterparty_id,
	// tanggal_penempatan ASC, id ASC). Includes FX rate and override id (LATERAL joins).
	// Returns rows with kurs pre-joined (NULL if IDR or kurs missing).
	BulkListDepositoForAggregate(ctx context.Context, evalDate time.Time) ([]BulkDepositoRow, error)
}

// BulkDepositoRow holds one instrument row from the batch JOIN query (doc §5 pseudo-SQL).
// FX rate is pre-joined (nil means IDR or kurs not found — caller must fail-fast if FCY+nil).
type BulkDepositoRow struct {
	InstrumenID        uuid.UUID
	KodeInstrumen      string
	TanggalPenempatan  time.Time
	NasabahID          uuid.UUID // counterparty_id
	BankID             uuid.UUID // bank_counterparty_id
	Nominal            decimal.Decimal
	MataUang           string
	KlasifikasiPsak71  string
	FXRate             *decimal.Decimal // nil = IDR or kurs missing
	LPSCapIDR          decimal.Decimal  // from CROSS JOIN LATERAL lps_coverage
	LPSCoverageParamID uuid.UUID
	OverrideID         *uuid.UUID // nil = no active exclusion override
	ExclusionReason    *string    // populated if OverrideID != nil
	NasabahNama        string
	BankNama           string
	TenantID           string
}

// DBDepositoInstrumenRepo implements DepositoInstrumenRepoIface.
type DBDepositoInstrumenRepo struct {
	db *sql.DB
}

// NewDBDepositoInstrumenRepo creates a DBDepositoInstrumenRepo.
func NewDBDepositoInstrumenRepo(db *sql.DB) *DBDepositoInstrumenRepo {
	return &DBDepositoInstrumenRepo{db: db}
}

const depositoByNasabahBankQuery = `
SELECT i.id, i.kode_instrumen, i.nama_instrumen,
       i.counterparty_id, i.bank_counterparty_id,
       i.nominal::text, i.mata_uang,
       i.tanggal_penempatan, i.klasifikasi_psak71,
       i.status, i.tenant_id
FROM mst.instrumen i
JOIN mst.counterparty cp_bank ON cp_bank.id = i.bank_counterparty_id
                              AND cp_bank.tipe_counterparty = 'BANK'
                              AND cp_bank.eligible_lps_flag = TRUE
WHERE i.tipe_instrumen = 'DEPOSITO'
  AND i.status = 'AKTIF'
  AND i.deleted_at IS NULL
  AND i.workflow_status = 'APPROVED'
  AND i.klasifikasi_psak71 IN ('AC', 'FVOCI_DEBT')
  AND i.counterparty_id = $1
  AND i.bank_counterparty_id = $2
  AND i.tenant_id = $3
ORDER BY i.tanggal_penempatan ASC, i.id ASC`

// ListByNasabahBank returns DEPOSITO instruments for a (nasabah, bank) pair in FIFO order.
func (r *DBDepositoInstrumenRepo) ListByNasabahBank(ctx context.Context, nasabahID, bankID uuid.UUID, evalDate time.Time) ([]InstrumenDepositoRow, error) {
	if r.db == nil {
		return nil, nil
	}
	// evalDate used in caller for FX conversion — query doesn't need it for filtering.
	_ = evalDate
	rows, err := r.db.QueryContext(ctx, depositoByNasabahBankQuery, nasabahID, bankID, "TUGURE")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanInstrumenDepositoRows(rows)
}

const activePairsQuery = `
SELECT DISTINCT i.counterparty_id AS nasabah_id, i.bank_counterparty_id AS bank_id
FROM mst.instrumen i
JOIN mst.counterparty cp_bank ON cp_bank.id = i.bank_counterparty_id
                              AND cp_bank.tipe_counterparty = 'BANK'
                              AND cp_bank.eligible_lps_flag = TRUE
WHERE i.tipe_instrumen = 'DEPOSITO'
  AND i.status = 'AKTIF'
  AND i.deleted_at IS NULL
  AND i.workflow_status = 'APPROVED'
  AND i.klasifikasi_psak71 IN ('AC', 'FVOCI_DEBT')
  AND i.tenant_id = $1`

// ListAllActivePairs returns all unique (nasabah, bank) pairs with active DEPOSITO.
func (r *DBDepositoInstrumenRepo) ListAllActivePairs(ctx context.Context, evalDate time.Time) ([]NasabahBankPair, error) {
	if r.db == nil {
		return nil, nil
	}
	_ = evalDate
	rows, err := r.db.QueryContext(ctx, activePairsQuery, "TUGURE")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var pairs []NasabahBankPair
	for rows.Next() {
		var p NasabahBankPair
		if err := rows.Scan(&p.NasabahID, &p.BankID); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// BulkListDepositoForAggregate executes the batch JOIN query from state-machine doc §5.
// Single query (no N+1) for all DEPOSITO instruments with pre-joined FX, LPS cap, and overrides.
// Ref: p4-m3-lps.md §5 pseudo-SQL.
const bulkDepositoQuery = `
SELECT
    i.id                          AS instrumen_id,
    i.kode_instrumen,
    i.tanggal_penempatan,
    i.counterparty_id             AS nasabah_id,
    i.bank_counterparty_id        AS bank_id,
    i.nominal::text               AS nominal,
    i.mata_uang,
    i.klasifikasi_psak71,
    CASE WHEN i.mata_uang = 'IDR' THEN NULL ELSE k.nilai_kurs::text END AS fx_rate,
    l.id                          AS lps_coverage_param_id,
    l.coverage_amount::text       AS lps_cap_idr,
    ov.id                         AS override_id,
    ov.exclusion_reason           AS exclusion_reason,
    cp_nasabah.nama_counterparty  AS nasabah_nama,
    cp_bank.nama_counterparty     AS bank_nama,
    i.tenant_id
FROM mst.instrumen i
JOIN mst.counterparty cp_nasabah  ON cp_nasabah.id = i.counterparty_id
                                  AND cp_nasabah.deleted_at IS NULL
JOIN mst.counterparty cp_bank     ON cp_bank.id = i.bank_counterparty_id
                                  AND cp_bank.tipe_counterparty = 'BANK'
                                  AND cp_bank.eligible_lps_flag = TRUE
                                  AND cp_bank.deleted_at IS NULL
LEFT JOIN mst.kurs k              ON k.kode_mata_uang = i.mata_uang
                                  AND k.sumber_kurs = 'BI_JISDOR'
                                  AND k.tanggal_berlaku = $1
CROSS JOIN LATERAL (
    SELECT id, coverage_amount
    FROM mst.lps_coverage
    WHERE workflow_status = 'APPROVED'
      AND periode_berlaku_dari <= $1
      AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)
    ORDER BY periode_berlaku_dari DESC
    LIMIT 1
) l
LEFT JOIN LATERAL (
    SELECT ov2.id, ov2.exclusion_reason
    FROM ecl.lps_exclusion_override ov2
    WHERE ov2.instrumen_id = i.id
      AND ov2.workflow_status = 'APPROVED_ACTIVE'
      AND ov2.deleted_at IS NULL
    LIMIT 1
) ov ON TRUE
WHERE i.tipe_instrumen = 'DEPOSITO'
  AND i.status = 'AKTIF'
  AND i.deleted_at IS NULL
  AND i.workflow_status = 'APPROVED'
  AND i.klasifikasi_psak71 IN ('AC', 'FVOCI_DEBT')
  AND i.tenant_id = $2
ORDER BY i.counterparty_id ASC, i.bank_counterparty_id ASC,
         i.tanggal_penempatan ASC, i.id ASC`

// BulkListDepositoForAggregate executes the batch N+1-free query.
func (r *DBDepositoInstrumenRepo) BulkListDepositoForAggregate(ctx context.Context, evalDate time.Time) ([]BulkDepositoRow, error) {
	if r.db == nil {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, bulkDepositoQuery, evalDate, "TUGURE")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []BulkDepositoRow
	for rows.Next() {
		var row BulkDepositoRow
		var nominalStr string
		var fxStr *string
		var capStr string
		if err := rows.Scan(
			&row.InstrumenID,
			&row.KodeInstrumen,
			&row.TanggalPenempatan,
			&row.NasabahID,
			&row.BankID,
			&nominalStr,
			&row.MataUang,
			&row.KlasifikasiPsak71,
			&fxStr,
			&row.LPSCoverageParamID,
			&capStr,
			&row.OverrideID,
			&row.ExclusionReason,
			&row.NasabahNama,
			&row.BankNama,
			&row.TenantID,
		); err != nil {
			return nil, err
		}
		var e error
		if row.Nominal, e = decimal.NewFromString(nominalStr); e != nil {
			return nil, fmt.Errorf("instrumen %s nominal parse: %w", row.InstrumenID, e)
		}
		if row.LPSCapIDR, e = decimal.NewFromString(capStr); e != nil {
			return nil, fmt.Errorf("lps_cap_idr parse: %w", e)
		}
		if fxStr != nil {
			rate, fe := decimal.NewFromString(*fxStr)
			if fe != nil {
				return nil, fmt.Errorf("fx_rate parse for instrumen %s: %w", row.InstrumenID, fe)
			}
			row.FXRate = &rate
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// scanInstrumenDepositoRows is a helper that scans sql.Rows into []InstrumenDepositoRow.
func scanInstrumenDepositoRows(rows *sql.Rows) ([]InstrumenDepositoRow, error) {
	var result []InstrumenDepositoRow
	for rows.Next() {
		var row InstrumenDepositoRow
		var nominalStr string
		var tp time.Time
		if err := rows.Scan(
			&row.ID, &row.KodeInstrumen, &row.NamaInstrumen,
			&row.CounterpartyID, &row.BankCounterpartyID,
			&nominalStr, &row.MataUang,
			&tp, &row.KlasifikasiPsak71,
			&row.Status, &row.TenantID,
		); err != nil {
			return nil, err
		}
		row.TanggalPenempatan = tp
		var e error
		if row.Nominal, e = decimal.NewFromString(nominalStr); e != nil {
			return nil, fmt.Errorf("instrumen nominal parse: %w", e)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ─── OverrideRepo ─────────────────────────────────────────────────────────────

// OverrideRepoIface defines CRUD operations on ecl.lps_exclusion_override.
// No hard delete (ecl schema rule — migration 000005 fn_ecl_no_hard_delete trigger).
type OverrideRepoIface interface {
	// GetByID loads one override by primary key.
	// Returns (nil, nil) if not found (soft-deleted counts as not found).
	GetByID(ctx context.Context, id uuid.UUID) (*LPSExclusionOverride, error)

	// GetActiveForInstrumen returns the APPROVED_ACTIVE override for instrumenID
	// covering evalDate (valid_from <= date <= valid_to or no valid_to).
	// Returns (nil, nil) if none.
	GetActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID, evalDate time.Time) (*LPSExclusionOverride, error)

	// GetActiveSetForInstrumens bulk-loads active overrides indexed by instrumenID.
	GetActiveSetForInstrumens(ctx context.Context, instrumenIDs []uuid.UUID, evalDate time.Time) (map[uuid.UUID]*LPSExclusionOverride, error)

	// HasActiveOrPendingForInstrumen returns true if instrumenID has an APPROVED_ACTIVE
	// or PENDING_APPROVAL override (for duplicate-submit prevention).
	HasActiveOrPendingForInstrumen(ctx context.Context, instrumenID uuid.UUID) (bool, string, error)

	// Create inserts a new override in PENDING_APPROVAL state.
	// tx must be an open *sql.Tx (audit-in-tx rule DEC-018).
	Create(ctx context.Context, tx *sql.Tx, o *LPSExclusionOverride) error

	// Approve updates status to APPROVED_ACTIVE and sets approval fields atomically.
	Approve(ctx context.Context, tx *sql.Tx, id uuid.UUID, approverID uuid.UUID,
		signedAt time.Time, sigHash []byte, comment string, updatedBy uuid.UUID) error

	// Reject updates status to REJECTED and sets rejection reason.
	Reject(ctx context.Context, tx *sql.Tx, id uuid.UUID, actorID uuid.UUID,
		rejectReason string, updatedBy uuid.UUID) error

	// List returns a paginated list of overrides for the DataTable endpoint.
	// Cursor is the last seen created_at||id (opaque, encoded/decoded internally).
	List(ctx context.Context,
		filterWorkflowStatus, filterInstrumenID, filterMakerID string,
		search string,
		sortCol, sortDir string,
		cursor string, limit int,
	) ([]LPSExclusionOverride, string, bool, error)
}

// DBOverrideRepo implements OverrideRepoIface against ecl.lps_exclusion_override.
type DBOverrideRepo struct {
	db *sql.DB
}

// NewDBOverrideRepo creates a DBOverrideRepo. Panics if db is nil to prevent silent failures.
func NewDBOverrideRepo(db *sql.DB) *DBOverrideRepo {
	return &DBOverrideRepo{db: db}
}

// allowedOverrideSortCols is the init-time assertion whitelist for override list queries.
// Referenced in List() to prevent SQL injection from sort parameters.
var allowedOverrideSortCols = map[string]bool{
	"created_at": true, "valid_from_periode_id": true, "valid_to_periode_id": true,
	"workflow_status": true, "instrumen_id": true,
}

func init() {
	// init-time assertion: ensure AllowedSortColsOverride matches allowedOverrideSortCols.
	for _, col := range AllowedSortColsOverride {
		if !allowedOverrideSortCols[col] {
			panic("lps: AllowedSortColsOverride contains column not in allowedOverrideSortCols: " + col)
		}
	}
}

const overrideByIDQuery = `
SELECT id, instrumen_id, exclusion_reason, valid_from_periode_id, valid_to_periode_id,
       workflow_status, maker_id, approver_id, signed_at_approve, signature_hash_approve,
       comment_approve, reject_reason,
       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by,
       row_version, tenant_id
FROM ecl.lps_exclusion_override
WHERE id = $1 AND deleted_at IS NULL`

// GetByID loads one override by primary key.
func (r *DBOverrideRepo) GetByID(ctx context.Context, id uuid.UUID) (*LPSExclusionOverride, error) {
	if r.db == nil {
		return nil, nil
	}
	return scanOverrideRow(r.db.QueryRowContext(ctx, overrideByIDQuery, id))
}

// GetActiveForInstrumen returns an APPROVED_ACTIVE override for instrumenID covering evalDate.
// The override is active if the valid_from_periode.tanggal_mulai <= evalDate
// AND (valid_to_periode.tanggal_akhir IS NULL OR >= evalDate).
// For simplicity and correctness, we check periode FK IDs against mst.periode_buku dates.
// Per migration 000023: valid_to_periode_id is required (not nullable), so we JOIN both.
const activeOverrideQuery = `
SELECT ov.id, ov.instrumen_id, ov.exclusion_reason, ov.valid_from_periode_id, ov.valid_to_periode_id,
       ov.workflow_status, ov.maker_id, ov.approver_id, ov.signed_at_approve, ov.signature_hash_approve,
       ov.comment_approve, ov.reject_reason,
       ov.created_at, ov.created_by, ov.updated_at, ov.updated_by, ov.deleted_at, ov.deleted_by,
       ov.row_version, ov.tenant_id
FROM ecl.lps_exclusion_override ov
JOIN mst.periode_buku pb_from ON pb_from.id = ov.valid_from_periode_id
JOIN mst.periode_buku pb_to   ON pb_to.id = ov.valid_to_periode_id
WHERE ov.instrumen_id = $1
  AND ov.workflow_status = 'APPROVED_ACTIVE'
  AND ov.deleted_at IS NULL
  AND pb_from.tanggal_mulai <= $2
  AND pb_to.tanggal_akhir >= $2
ORDER BY pb_from.tanggal_mulai DESC
LIMIT 1`

// GetActiveForInstrumen fetches the active APPROVED_ACTIVE override for instrumenID on evalDate.
func (r *DBOverrideRepo) GetActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID, evalDate time.Time) (*LPSExclusionOverride, error) {
	if r.db == nil {
		return nil, nil
	}
	ov, err := scanOverrideRow(r.db.QueryRowContext(ctx, activeOverrideQuery, instrumenID, evalDate))
	if err != nil {
		return nil, err
	}
	return ov, nil
}

// GetActiveSetForInstrumens bulk-loads APPROVED_ACTIVE overrides for a set of instrumenIDs.
func (r *DBOverrideRepo) GetActiveSetForInstrumens(ctx context.Context, instrumenIDs []uuid.UUID, evalDate time.Time) (map[uuid.UUID]*LPSExclusionOverride, error) {
	if r.db == nil || len(instrumenIDs) == 0 {
		return map[uuid.UUID]*LPSExclusionOverride{}, nil
	}
	placeholders := make([]string, len(instrumenIDs))
	args := make([]interface{}, 0, len(instrumenIDs)+1)
	args = append(args, evalDate)
	for i, id := range instrumenIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	// Placeholders ($2, $3, ...) are parameterized — no injection risk.
	//nolint:gosec // placeholders generated via fmt.Sprintf("$%d") with parameterized args
	q := fmt.Sprintf(`
SELECT ov.id, ov.instrumen_id, ov.exclusion_reason, ov.valid_from_periode_id, ov.valid_to_periode_id,
       ov.workflow_status, ov.maker_id, ov.approver_id, ov.signed_at_approve, ov.signature_hash_approve,
       ov.comment_approve, ov.reject_reason,
       ov.created_at, ov.created_by, ov.updated_at, ov.updated_by, ov.deleted_at, ov.deleted_by,
       ov.row_version, ov.tenant_id
FROM ecl.lps_exclusion_override ov
JOIN mst.periode_buku pb_from ON pb_from.id = ov.valid_from_periode_id
JOIN mst.periode_buku pb_to   ON pb_to.id = ov.valid_to_periode_id
WHERE ov.instrumen_id IN (%s)
  AND ov.workflow_status = 'APPROVED_ACTIVE'
  AND ov.deleted_at IS NULL
  AND pb_from.tanggal_mulai <= $1
  AND pb_to.tanggal_akhir >= $1`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[uuid.UUID]*LPSExclusionOverride, len(instrumenIDs))
	for rows.Next() {
		ov, err := scanOverrideRowFromRows(rows)
		if err != nil {
			return nil, err
		}
		result[ov.InstrumenID] = ov
	}
	return result, rows.Err()
}

const hasActiveOrPendingQuery = `
SELECT id::text, workflow_status
FROM ecl.lps_exclusion_override
WHERE instrumen_id = $1
  AND workflow_status IN ('APPROVED_ACTIVE', 'PENDING_APPROVAL')
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1`

// HasActiveOrPendingForInstrumen returns (true, conflicting_id, nil) if there is an active or pending override.
func (r *DBOverrideRepo) HasActiveOrPendingForInstrumen(ctx context.Context, instrumenID uuid.UUID) (bool, string, error) {
	if r.db == nil {
		return false, "", nil
	}
	var existingID, status string
	err := r.db.QueryRowContext(ctx, hasActiveOrPendingQuery, instrumenID).Scan(&existingID, &status)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, existingID, nil
}

const overrideInsertQuery = `
INSERT INTO ecl.lps_exclusion_override (
    instrumen_id, exclusion_reason, valid_from_periode_id, valid_to_periode_id,
    workflow_status, maker_id,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3, $4,
    'PENDING_APPROVAL', $5,
    now(), $6, now(), $6, 1, $7
) RETURNING id, created_at, updated_at, row_version`

// Create inserts a new LPS exclusion override in PENDING_APPROVAL state.
func (r *DBOverrideRepo) Create(ctx context.Context, tx *sql.Tx, o *LPSExclusionOverride) error {
	if r.db == nil {
		return fmt.Errorf("lps override repo: db not initialized")
	}
	var exec interface {
		QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	} = r.db
	if tx != nil {
		exec = tx
	}
	return exec.QueryRowContext(ctx, overrideInsertQuery,
		o.InstrumenID, o.ExclusionReason, o.ValidFromPeriodeID, o.ValidToPeriodeID,
		o.MakerID, o.CreatedBy, o.TenantID,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt, &o.RowVersion)
}

const overrideApproveQuery = `
UPDATE ecl.lps_exclusion_override
SET workflow_status = 'APPROVED_ACTIVE',
    approver_id = $2,
    signed_at_approve = $3,
    signature_hash_approve = $4,
    comment_approve = $5,
    updated_by = $6,
    updated_at = now(),
    row_version = row_version + 1
WHERE id = $1
  AND workflow_status = 'PENDING_APPROVAL'
  AND deleted_at IS NULL`

// Approve transitions an override from PENDING_APPROVAL → APPROVED_ACTIVE.
func (r *DBOverrideRepo) Approve(ctx context.Context, tx *sql.Tx, id uuid.UUID, approverID uuid.UUID,
	signedAt time.Time, sigHash []byte, comment string, updatedBy uuid.UUID) error {
	if r.db == nil {
		return fmt.Errorf("lps override repo: db not initialized")
	}
	var exec interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	} = r.db
	if tx != nil {
		exec = tx
	}
	res, err := exec.ExecContext(ctx, overrideApproveQuery,
		id, approverID, signedAt, sigHash, comment, updatedBy)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domainerrors.New(domainerrors.Code(CodeLPSOverrideInvalidTransition),
			"Override tidak ditemukan atau sudah dalam status final")
	}
	return nil
}

const overrideRejectQuery = `
UPDATE ecl.lps_exclusion_override
SET workflow_status = 'REJECTED',
    reject_reason = $2,
    updated_by = $3,
    updated_at = now(),
    row_version = row_version + 1
WHERE id = $1
  AND workflow_status = 'PENDING_APPROVAL'
  AND deleted_at IS NULL`

// Reject transitions an override from PENDING_APPROVAL → REJECTED.
func (r *DBOverrideRepo) Reject(ctx context.Context, tx *sql.Tx, id uuid.UUID, _ uuid.UUID,
	rejectReason string, updatedBy uuid.UUID) error {
	if r.db == nil {
		return fmt.Errorf("lps override repo: db not initialized")
	}
	var exec interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	} = r.db
	if tx != nil {
		exec = tx
	}
	res, err := exec.ExecContext(ctx, overrideRejectQuery, id, rejectReason, updatedBy)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domainerrors.New(domainerrors.Code(CodeLPSOverrideInvalidTransition),
			"Override tidak ditemukan atau sudah dalam status final")
	}
	return nil
}

// List returns a paginated list of overrides for DataTable (sort+filter+cursor).
// Cursor is the last seen created_at||id pair (base64-encoded; for simplicity we use id as cursor here).
func (r *DBOverrideRepo) List(ctx context.Context,
	filterWorkflowStatus, filterInstrumenID, filterMakerID string,
	search string,
	sortCol, sortDir string,
	cursor string, limit int,
) ([]LPSExclusionOverride, string, bool, error) {
	if r.db == nil {
		return nil, "", false, nil
	}

	// Validate sort column against allowlist.
	if !allowedOverrideSortCols[sortCol] {
		sortCol = "created_at"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}

	args := []interface{}{}
	conds := []string{"deleted_at IS NULL"}
	argIdx := 1

	if filterWorkflowStatus != "" {
		conds = append(conds, fmt.Sprintf("workflow_status = $%d", argIdx))
		args = append(args, filterWorkflowStatus)
		argIdx++
	}
	if filterInstrumenID != "" {
		conds = append(conds, fmt.Sprintf("instrumen_id = $%d", argIdx))
		args = append(args, filterInstrumenID)
		argIdx++
	}
	if filterMakerID != "" {
		conds = append(conds, fmt.Sprintf("maker_id = $%d", argIdx))
		args = append(args, filterMakerID)
		argIdx++
	}
	if search != "" {
		conds = append(conds, fmt.Sprintf("exclusion_reason ILIKE $%d", argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if cursor != "" {
		op := ">"
		if sortDir == "desc" {
			op = "<"
		}
		conds = append(conds, fmt.Sprintf("id %s $%d", op, argIdx))
		args = append(args, cursor)
		argIdx++
	}

	where := "WHERE " + strings.Join(conds, " AND ")
	args = append(args, limit+1) // limit+1 trick to detect hasMore
	// sortCol is validated against allowlist above; sortDir validated; where uses parameterized args.
	//nolint:gosec // sortCol from allowlist, sortDir validated, where uses $N placeholders
	q := fmt.Sprintf(`
SELECT id, instrumen_id, exclusion_reason, valid_from_periode_id, valid_to_periode_id,
       workflow_status, maker_id, approver_id, signed_at_approve, signature_hash_approve,
       comment_approve, reject_reason,
       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by,
       row_version, tenant_id
FROM ecl.lps_exclusion_override
%s
ORDER BY %s %s
LIMIT $%d`, where, sortCol, strings.ToUpper(sortDir), argIdx)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close() //nolint:errcheck

	var result []LPSExclusionOverride
	for rows.Next() {
		ov, err := scanOverrideRowFromRows(rows)
		if err != nil {
			return nil, "", false, err
		}
		result = append(result, *ov)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	nextCursor := ""
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].ID.String()
	}
	return result, nextCursor, hasMore, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanOverrideRow(row *sql.Row) (*LPSExclusionOverride, error) {
	var ov LPSExclusionOverride
	err := row.Scan(
		&ov.ID, &ov.InstrumenID, &ov.ExclusionReason,
		&ov.ValidFromPeriodeID, &ov.ValidToPeriodeID,
		&ov.WorkflowStatus, &ov.MakerID, &ov.ApproverID,
		&ov.SignedAtApprove, &ov.SignatureHashApprove,
		&ov.CommentApprove, &ov.RejectReason,
		&ov.CreatedAt, &ov.CreatedBy, &ov.UpdatedAt, &ov.UpdatedBy,
		&ov.DeletedAt, &ov.DeletedBy, &ov.RowVersion, &ov.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ov, nil
}

func scanOverrideRowFromRows(rows *sql.Rows) (*LPSExclusionOverride, error) {
	var ov LPSExclusionOverride
	err := rows.Scan(
		&ov.ID, &ov.InstrumenID, &ov.ExclusionReason,
		&ov.ValidFromPeriodeID, &ov.ValidToPeriodeID,
		&ov.WorkflowStatus, &ov.MakerID, &ov.ApproverID,
		&ov.SignedAtApprove, &ov.SignatureHashApprove,
		&ov.CommentApprove, &ov.RejectReason,
		&ov.CreatedAt, &ov.CreatedBy, &ov.UpdatedAt, &ov.UpdatedBy,
		&ov.DeletedAt, &ov.DeletedBy, &ov.RowVersion, &ov.TenantID,
	)
	if err != nil {
		return nil, err
	}
	return &ov, nil
}
