// Repository layer for the look-through ECL engine.
//
// Interfaces:
//   - FundCompositionRepo: CRUD + GetActive + List for mst.fund_composition + detail.
//   - ReksadanaInstrumenRepo: reads mst.instrumen filtered to REKSADANA type.
//   - PDLGDClassRepo: resolves PD + LGD parameters per asset class per evaluation period.
//   - ScenarioParamRepo: resolves ALCO-approved scenario weights + FL multipliers.
//   - LookthroughResultRepo: inserts / retrieves ecl.lookthrough_underlying rows.
//
// Decimal precision (DEC-016):
//   - NUMERIC(20,4) IDR: scanned via ::text → decimal.NewFromString.
//   - NUMERIC(10,8) PD/LGD/weight: scanned via ::text → decimal.NewFromString.
//   - NUMERIC(7,4) weight_pct: scanned via ::text → decimal.NewFromString.
//   - NEVER float64 for money or rates.
//
// Hard-delete rules:
//   - ecl.lookthrough_underlying: soft-delete only (fn_ecl_no_hard_delete trigger in migration).
//   - mst.fund_composition: soft-delete only (fn_mst_fc_no_hard_delete trigger in migration 000024).
//
// All mutations operate inside an open *sql.Tx for audit-in-transaction (DEC-018).
package lookthrough

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

// ─── FundCompositionRepo ──────────────────────────────────────────────────────

// FundCompositionRepo defines all persistence operations for mst.fund_composition
// and mst.fund_composition_detail.
type FundCompositionRepo interface {
	// Create inserts a new fund_composition header in PENDING_REVIEW state
	// plus its detail lines. Atomic — must be called within a *sql.Tx.
	Create(ctx context.Context, tx *sql.Tx, header *FundComposition, details []FundCompositionDetail) error

	// GetByID loads a single fund_composition header by PK.
	// Returns (nil, nil) when not found or soft-deleted.
	GetByID(ctx context.Context, id uuid.UUID) (*FundComposition, error)

	// GetDetailsForComposition loads all mst.fund_composition_detail rows for compositionID,
	// ordered by position ASC. Excludes soft-deleted detail rows.
	GetDetailsForComposition(ctx context.Context, compositionID uuid.UUID) ([]FundCompositionDetail, error)

	// GetActiveForInstrumen returns the APPROVED_ACTIVE composition for instrumenID
	// whose effective range covers evaluationDate
	// (effective_from <= evalDate AND effective_to >= evalDate).
	// Returns (nil, nil) when none found.
	GetActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time) (*FundComposition, error)

	// ListByInstrumen lists ALL compositions for instrumenID (any status), ordered by created_at DESC.
	// Used by the composition history endpoint and DataTable.
	// cursor is opaque; returns (rows, nextCursor, hasMore, error).
	ListByInstrumen(ctx context.Context, instrumenID uuid.UUID,
		filterStatus string, cursor string, limit int,
		sortCol, sortDir string,
	) ([]FundComposition, string, bool, error)

	// UpdateWorkflowStatus updates workflow_status + related fields atomically within a tx.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx,
		id uuid.UUID, newStatus WorkflowStatus,
		reviewerID *uuid.UUID, signedAtReview *time.Time, sigHashReview []byte, commentReview *string,
		approverID *uuid.UUID, signedAtApprove *time.Time, sigHashApprove []byte, commentApprove *string,
		rejectReason *string,
		updatedBy uuid.UUID,
	) error

	// SupersedeOld atomically sets status = SUPERSEDED + effective_to = supersedeDate
	// on oldCompositionID (the prior APPROVED_ACTIVE row). Called in amendment approve tx.
	SupersedeOld(ctx context.Context, tx *sql.Tx, oldCompositionID uuid.UUID, supersedeDate time.Time, updatedBy uuid.UUID) error

	// GetInstrumenTipeAndKlasifikasi returns (tipe_instrumen, klasifikasi_psak71, poci_flag) for instrumenID.
	// Returns ("","","",false, ErrNotFound) if soft-deleted or not found.
	GetInstrumenTipeAndKlasifikasi(ctx context.Context, instrumenID uuid.UUID) (tipe, klasifikasi string, pociFlag bool, err error)
}

// DBFundCompositionRepo implements FundCompositionRepo against mst.fund_composition.
type DBFundCompositionRepo struct {
	db *sql.DB
}

// NewDBFundCompositionRepo creates a DBFundCompositionRepo. db must not be nil.
func NewDBFundCompositionRepo(db *sql.DB) *DBFundCompositionRepo {
	if db == nil {
		panic("lookthrough: DBFundCompositionRepo requires non-nil db")
	}
	return &DBFundCompositionRepo{db: db}
}

const createFundCompositionSQL = `
INSERT INTO mst.fund_composition
  (id, instrumen_id, effective_from, effective_to, workflow_status,
   maker_id, source_doc_id,
   created_at, created_by, updated_at, updated_by, row_version, tenant_id)
VALUES
  ($1, $2, $3, $4, $5,
   $6, $7,
   now(), $8, now(), $8, 1, $9)`

const createFundCompositionDetailSQL = `
INSERT INTO mst.fund_composition_detail
  (id, fund_composition_id, asset_class, weight_pct, position,
   created_at, created_by, updated_at, updated_by, row_version, tenant_id)
VALUES
  ($1, $2, $3, $4::numeric, $5,
   now(), $6, now(), $6, 1, $7)`

// Create inserts fund_composition header + detail lines within the provided tx.
func (r *DBFundCompositionRepo) Create(ctx context.Context, tx *sql.Tx, header *FundComposition, details []FundCompositionDetail) error {
	_, err := tx.ExecContext(ctx, createFundCompositionSQL,
		header.ID, header.InstrumenID,
		header.EffectiveFrom, header.EffectiveTo,
		string(header.WorkflowStatus),
		header.MakerID, header.SourceDocID,
		header.CreatedBy, header.TenantID,
	)
	if err != nil {
		return fmt.Errorf("fund_composition insert: %w", err)
	}
	for i := range details {
		d := &details[i]
		_, err := tx.ExecContext(ctx, createFundCompositionDetailSQL,
			d.ID, d.FundCompositionID,
			string(d.AssetClass), d.WeightPct.StringFixed(4), d.Position,
			d.CreatedBy, d.TenantID,
		)
		if err != nil {
			return fmt.Errorf("fund_composition_detail insert asset_class=%s: %w", d.AssetClass, err)
		}
	}
	return nil
}

const fundCompositionByIDSQL = `
SELECT fc.id, fc.instrumen_id, fc.effective_from, fc.effective_to, fc.workflow_status,
       fc.maker_id, fc.reviewer_id, fc.approver_id,
       fc.signed_at_review, fc.signature_hash_review, fc.comment_review,
       fc.signed_at_approve, fc.signature_hash_approve, fc.comment_approve,
       fc.reject_reason, fc.source_doc_id,
       fc.created_at, fc.created_by, fc.updated_at, fc.updated_by,
       fc.deleted_at, fc.deleted_by, fc.row_version, fc.tenant_id
FROM mst.fund_composition fc
WHERE fc.id = $1
  AND fc.deleted_at IS NULL`

// GetByID loads a single fund_composition by PK.
func (r *DBFundCompositionRepo) GetByID(ctx context.Context, id uuid.UUID) (*FundComposition, error) {
	return scanFundCompositionRow(r.db.QueryRowContext(ctx, fundCompositionByIDSQL, id))
}

const fundCompositionDetailsSQL = `
SELECT id, fund_composition_id, asset_class, weight_pct::text, position,
       created_at, created_by, updated_at, updated_by,
       deleted_at, deleted_by, row_version, tenant_id
FROM mst.fund_composition_detail
WHERE fund_composition_id = $1
  AND deleted_at IS NULL
ORDER BY position ASC, id ASC`

// GetDetailsForComposition loads detail rows for compositionID.
func (r *DBFundCompositionRepo) GetDetailsForComposition(ctx context.Context, compositionID uuid.UUID) ([]FundCompositionDetail, error) {
	rows, err := r.db.QueryContext(ctx, fundCompositionDetailsSQL, compositionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanFundCompositionDetailRows(rows)
}

const activeCompositionSQL = `
SELECT fc.id, fc.instrumen_id, fc.effective_from, fc.effective_to, fc.workflow_status,
       fc.maker_id, fc.reviewer_id, fc.approver_id,
       fc.signed_at_review, fc.signature_hash_review, fc.comment_review,
       fc.signed_at_approve, fc.signature_hash_approve, fc.comment_approve,
       fc.reject_reason, fc.source_doc_id,
       fc.created_at, fc.created_by, fc.updated_at, fc.updated_by,
       fc.deleted_at, fc.deleted_by, fc.row_version, fc.tenant_id
FROM mst.fund_composition fc
WHERE fc.instrumen_id = $1
  AND fc.workflow_status = 'APPROVED_ACTIVE'
  AND fc.effective_from <= $2
  AND fc.effective_to >= $2
  AND fc.deleted_at IS NULL
ORDER BY fc.effective_from DESC
LIMIT 1`

// GetActiveForInstrumen returns the APPROVED_ACTIVE composition effective on evaluationDate.
func (r *DBFundCompositionRepo) GetActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time) (*FundComposition, error) {
	return scanFundCompositionRow(r.db.QueryRowContext(ctx, activeCompositionSQL, instrumenID, evaluationDate))
}

// ListByInstrumen returns paginated composition headers for instrumenID.
// sortCol and sortDir are validated against AllowedSortColsComposition before use.
func (r *DBFundCompositionRepo) ListByInstrumen(ctx context.Context, instrumenID uuid.UUID,
	filterStatus string, cursor string, limit int,
	sortCol, sortDir string,
) ([]FundComposition, string, bool, error) {
	// Validate sort column against whitelist.
	if !isAllowedSortCol(sortCol, AllowedSortColsComposition) {
		sortCol = "created_at"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("fc.instrumen_id = $%d", argIdx))
	args = append(args, instrumenID)
	argIdx++

	conditions = append(conditions, "fc.deleted_at IS NULL")

	if filterStatus != "" {
		conditions = append(conditions, fmt.Sprintf("fc.workflow_status = $%d", argIdx))
		args = append(args, filterStatus)
		argIdx++
	}

	if cursor != "" {
		decodedCursor, err := decodeCursor(cursor)
		if err == nil {
			op := "<"
			if sortDir == "asc" {
				op = ">"
			}
			conditions = append(conditions, fmt.Sprintf("(fc.%s, fc.id) %s ($%d, $%d)",
				sortCol, op, argIdx, argIdx+1))
			args = append(args, decodedCursor.SortValue, decodedCursor.ID)
			argIdx += 2
		}
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	//nolint:gosec // sortCol and sortDir are validated against whitelist above
	q := fmt.Sprintf(`
SELECT fc.id, fc.instrumen_id, fc.effective_from, fc.effective_to, fc.workflow_status,
       fc.maker_id, fc.reviewer_id, fc.approver_id,
       fc.signed_at_review, fc.signature_hash_review, fc.comment_review,
       fc.signed_at_approve, fc.signature_hash_approve, fc.comment_approve,
       fc.reject_reason, fc.source_doc_id,
       fc.created_at, fc.created_by, fc.updated_at, fc.updated_by,
       fc.deleted_at, fc.deleted_by, fc.row_version, fc.tenant_id
FROM mst.fund_composition fc
%s
ORDER BY fc.%s %s, fc.id %s
LIMIT $%d`, whereClause, sortCol, sortDir, sortDir, argIdx)

	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close() //nolint:errcheck

	compositions, err := scanFundCompositionRows(rows)
	if err != nil {
		return nil, "", false, err
	}

	hasMore := len(compositions) > limit
	if hasMore {
		compositions = compositions[:limit]
	}
	var nextCursor string
	if hasMore && len(compositions) > 0 {
		last := compositions[len(compositions)-1]
		nextCursor = encodeCursor(last.CreatedAt.Format(time.RFC3339Nano), last.ID.String())
	}
	return compositions, nextCursor, hasMore, nil
}

const updateWorkflowStatusSQL = `
UPDATE mst.fund_composition
SET workflow_status = $1,
    reviewer_id = COALESCE($2, reviewer_id),
    signed_at_review = COALESCE($3, signed_at_review),
    signature_hash_review = COALESCE($4, signature_hash_review),
    comment_review = COALESCE($5, comment_review),
    approver_id = COALESCE($6, approver_id),
    signed_at_approve = COALESCE($7, signed_at_approve),
    signature_hash_approve = COALESCE($8, signature_hash_approve),
    comment_approve = COALESCE($9, comment_approve),
    reject_reason = COALESCE($10, reject_reason),
    updated_by = $11,
    updated_at = now(),
    row_version = row_version + 1
WHERE id = $12
  AND deleted_at IS NULL`

// UpdateWorkflowStatus updates workflow state fields within a tx.
func (r *DBFundCompositionRepo) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx,
	id uuid.UUID, newStatus WorkflowStatus,
	reviewerID *uuid.UUID, signedAtReview *time.Time, sigHashReview []byte, commentReview *string,
	approverID *uuid.UUID, signedAtApprove *time.Time, sigHashApprove []byte, commentApprove *string,
	rejectReason *string,
	updatedBy uuid.UUID,
) error {
	_, err := tx.ExecContext(ctx, updateWorkflowStatusSQL,
		string(newStatus),
		reviewerID, signedAtReview, sigHashReview, commentReview,
		approverID, signedAtApprove, sigHashApprove, commentApprove,
		rejectReason,
		updatedBy, id,
	)
	if err != nil {
		return fmt.Errorf("update fund_composition workflow_status id=%s: %w", id, err)
	}
	return nil
}

const supersedeOldSQL = `
UPDATE mst.fund_composition
SET workflow_status = 'SUPERSEDED',
    effective_to = $1,
    updated_by = $2,
    updated_at = now(),
    row_version = row_version + 1
WHERE id = $3
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL`

// SupersedeOld sets the old composition to SUPERSEDED and closes its effective_to.
func (r *DBFundCompositionRepo) SupersedeOld(ctx context.Context, tx *sql.Tx, oldCompositionID uuid.UUID, supersedeDate time.Time, updatedBy uuid.UUID) error {
	res, err := tx.ExecContext(ctx, supersedeOldSQL, supersedeDate, updatedBy, oldCompositionID)
	if err != nil {
		return fmt.Errorf("supersede fund_composition id=%s: %w", oldCompositionID, err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("supersede fund_composition id=%s rows affected: %w", oldCompositionID, raErr)
	}
	if n == 0 {
		return fmt.Errorf("supersede fund_composition id=%s: 0 rows affected — composition may not be APPROVED_ACTIVE", oldCompositionID)
	}
	return nil
}

const instrumenTipeKlasifikasiSQL = `
SELECT tipe_instrumen, COALESCE(klasifikasi_psak71,''), COALESCE(poci_flag, false)
FROM mst.instrumen
WHERE id = $1
  AND deleted_at IS NULL`

// GetInstrumenTipeAndKlasifikasi returns instrument classification data.
func (r *DBFundCompositionRepo) GetInstrumenTipeAndKlasifikasi(ctx context.Context, instrumenID uuid.UUID) (string, string, bool, error) {
	var tipe, klasifikasi string
	var poci bool
	err := r.db.QueryRowContext(ctx, instrumenTipeKlasifikasiSQL, instrumenID).Scan(&tipe, &klasifikasi, &poci)
	if err == sql.ErrNoRows {
		return "", "", false, domainerrors.ErrNotFound("instrumen")
	}
	return tipe, klasifikasi, poci, err
}

// ─── ReksadanaInstrumenRepo ───────────────────────────────────────────────────

// ReksadanaInstrumenRepo defines queries for REKSADANA instruments.
type ReksadanaInstrumenRepo interface {
	// GetByID returns a single REKSADANA instrument row.
	// Returns (nil, nil) if not found, soft-deleted, or tipe ≠ REKSADANA.
	GetByID(ctx context.Context, id uuid.UUID) (*InstrumenReksadanaRow, error)

	// BulkListReksadanaForECL returns ALL REKSADANA instruments that should participate
	// in bulk ECL computation for periodeID. Filters:
	//   - tipe_instrumen = 'REKSADANA'
	//   - status = 'AKTIF'
	//   - workflow_status = 'APPROVED'
	//   - deleted_at IS NULL
	// Returns max 10_001 rows (caller checks > 10_000 and returns ErrBulkTooLarge).
	BulkListReksadanaForECL(ctx context.Context, tenantID string) ([]InstrumenReksadanaRow, error)
}

// DBReksadanaInstrumenRepo implements ReksadanaInstrumenRepo.
type DBReksadanaInstrumenRepo struct {
	db *sql.DB
}

// NewDBReksadanaInstrumenRepo creates a DBReksadanaInstrumenRepo.
func NewDBReksadanaInstrumenRepo(db *sql.DB) *DBReksadanaInstrumenRepo {
	if db == nil {
		panic("lookthrough: DBReksadanaInstrumenRepo requires non-nil db")
	}
	return &DBReksadanaInstrumenRepo{db: db}
}

const reksadanaByIDSQL = `
SELECT i.id, i.kode_instrumen, i.nama_instrumen, i.tipe_instrumen,
       COALESCE(i.klasifikasi_psak71, '') AS klasifikasi_psak71,
       i.nominal_nab_idr::text,
       COALESCE(i.poci_flag, false) AS poci_flag,
       i.status, i.workflow_status, i.tenant_id
FROM mst.instrumen i
WHERE i.id = $1
  AND i.tipe_instrumen = 'REKSADANA'
  AND i.deleted_at IS NULL`

// GetByID returns a REKSADANA instrument row.
func (r *DBReksadanaInstrumenRepo) GetByID(ctx context.Context, id uuid.UUID) (*InstrumenReksadanaRow, error) {
	var row InstrumenReksadanaRow
	var nabStr *string
	err := r.db.QueryRowContext(ctx, reksadanaByIDSQL, id).Scan(
		&row.ID, &row.KodeInstrumen, &row.NamaInstrumen, &row.TipeInstrumen,
		&row.KlasifikasiPsak71,
		&nabStr,
		&row.POCIFlag,
		&row.Status, &row.WorkflowStatus, &row.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if nabStr != nil {
		d, e := decimal.NewFromString(*nabStr)
		if e != nil {
			return nil, fmt.Errorf("instrumen %s nominal_nab_idr parse: %w", id, e)
		}
		row.NominalNABIDR = &d
	}
	return &row, nil
}

const bulkReksadanaSQL = `
SELECT i.id, i.kode_instrumen, i.nama_instrumen, i.tipe_instrumen,
       COALESCE(i.klasifikasi_psak71, '') AS klasifikasi_psak71,
       i.nominal_nab_idr::text,
       COALESCE(i.poci_flag, false) AS poci_flag,
       i.status, i.workflow_status, i.tenant_id
FROM mst.instrumen i
WHERE i.tipe_instrumen = 'REKSADANA'
  AND i.status = 'AKTIF'
  AND i.workflow_status = 'APPROVED'
  AND i.deleted_at IS NULL
  AND i.tenant_id = $1
ORDER BY i.id
LIMIT 10001`

// BulkListReksadanaForECL loads up to 10_001 REKSADANA instruments.
func (r *DBReksadanaInstrumenRepo) BulkListReksadanaForECL(ctx context.Context, tenantID string) ([]InstrumenReksadanaRow, error) {
	rows, err := r.db.QueryContext(ctx, bulkReksadanaSQL, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []InstrumenReksadanaRow
	for rows.Next() {
		var row InstrumenReksadanaRow
		var nabStr *string
		if err := rows.Scan(
			&row.ID, &row.KodeInstrumen, &row.NamaInstrumen, &row.TipeInstrumen,
			&row.KlasifikasiPsak71, &nabStr,
			&row.POCIFlag,
			&row.Status, &row.WorkflowStatus, &row.TenantID,
		); err != nil {
			return nil, err
		}
		if nabStr != nil {
			d, e := decimal.NewFromString(*nabStr)
			if e != nil {
				return nil, fmt.Errorf("instrumen %s nominal_nab_idr parse: %w", row.ID, e)
			}
			row.NominalNABIDR = &d
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ─── PDLGDClassRepo ───────────────────────────────────────────────────────────

// PDLGDClassRepo resolves PD per scenario + LGD per asset class for a given period.
// PD source: mst.pd_pefindo (APPROVED, latest valid on evaluationDate).
// LGD source: mst.lgd_basel (APPROVED, per tipe_eksposur).
// GOVT_BOND: PD = 0.00000000 (hardcoded — IsSovereignZeroPD = true, per OQ-M4-4).
type PDLGDClassRepo interface {
	// GetPDLGDForAssetClass returns PD (Good/Normal/Bad) and LGD for the given asset class.
	// evaluationDate is used to select the active parameter version.
	// Returns ErrPDLGDClassMissing if no APPROVED parameter found.
	GetPDLGDForAssetClass(ctx context.Context, assetClass AssetClass, evaluationDate time.Time, tenantID string) (PDLGDParams, error)

	// BulkGetPDLGDForAssetClasses loads all asset classes in one shot.
	// Returns a map[AssetClass]PDLGDParams.
	BulkGetPDLGDForAssetClasses(ctx context.Context, assetClasses []AssetClass, evaluationDate time.Time, tenantID string) (map[AssetClass]PDLGDParams, error)
}

// DBPDLGDClassRepo implements PDLGDClassRepo.
type DBPDLGDClassRepo struct {
	db *sql.DB
}

// NewDBPDLGDClassRepo creates a DBPDLGDClassRepo.
func NewDBPDLGDClassRepo(db *sql.DB) *DBPDLGDClassRepo {
	if db == nil {
		panic("lookthrough: DBPDLGDClassRepo requires non-nil db")
	}
	return &DBPDLGDClassRepo{db: db}
}

// GetPDLGDForAssetClass resolves PD/LGD for one asset class.
// GOVT_BOND: PD = 0 (hardcoded). Others: look up mst.pd_pefindo + mst.lgd_basel.
func (r *DBPDLGDClassRepo) GetPDLGDForAssetClass(ctx context.Context, assetClass AssetClass, evaluationDate time.Time, tenantID string) (PDLGDParams, error) {
	result, err := r.BulkGetPDLGDForAssetClasses(ctx, []AssetClass{assetClass}, evaluationDate, tenantID)
	if err != nil {
		return PDLGDParams{}, err
	}
	p, ok := result[assetClass]
	if !ok {
		return PDLGDParams{}, ErrPDLGDClassMissing(string(assetClass), evaluationDate.Format("2006-01-02"))
	}
	return p, nil
}

// pdLGDBulkSQL is the batch PD/LGD query for multiple asset classes.
// Joins mst.pd_pefindo (for PD Good/Normal/Bad by rating_grade_lookthrough)
// with mst.lgd_basel (for LGD by tipe_eksposur).
// GOVT_BOND special case: PD = 0 injected by Go logic after query.
//
// The query uses `asset_class_lookthrough` column on pd_pefindo (added migration 000024)
// to map asset class → PD parameters.
// For asset classes without a specific rating (CASH, EQUITY, OTHER):
//   - uses the 'CONSERVATIVE' grade bucket or sector-average per state-machine §2.3.
const pdLGDBulkSQL = `
SELECT
    pd.asset_class_lookthrough      AS asset_class,
    pd.pd_good::text                AS pd_good,
    pd.pd_normal::text              AS pd_normal,
    pd.pd_bad::text                 AS pd_bad,
    lgd.lgd_pct::text               AS lgd
FROM mst.pd_pefindo pd
JOIN mst.lgd_basel lgd
  ON lgd.tipe_eksposur = pd.lgd_tipe_eksposur
  AND lgd.workflow_status = 'APPROVED'
  AND lgd.tenant_id = pd.tenant_id
WHERE pd.workflow_status = 'APPROVED'
  AND pd.asset_class_lookthrough = ANY($1::text[])
  AND pd.effective_from <= $2
  AND (pd.effective_to IS NULL OR pd.effective_to >= $2)
  AND pd.tenant_id = $3
ORDER BY pd.effective_from DESC`

// BulkGetPDLGDForAssetClasses loads PD/LGD for multiple asset classes in one query.
func (r *DBPDLGDClassRepo) BulkGetPDLGDForAssetClasses(ctx context.Context, assetClasses []AssetClass, evaluationDate time.Time, tenantID string) (map[AssetClass]PDLGDParams, error) {
	result := make(map[AssetClass]PDLGDParams)

	// GOVT_BOND: sovereign IDR → PD = 0 hardcoded, no DB lookup needed.
	nonSovereign := make([]string, 0, len(assetClasses))
	for _, ac := range assetClasses {
		if ac.IsSovereignZeroPD() {
			// Build GOVT_BOND LGD from mst.lgd_basel only.
			lgd, err := r.getSovereignLGD(ctx, evaluationDate, tenantID)
			if err != nil {
				return nil, err
			}
			result[AssetClassGovtBond] = PDLGDParams{
				AssetClass: AssetClassGovtBond,
				PDGood:     decimal.Zero,
				PDNormal:   decimal.Zero,
				PDBad:      decimal.Zero,
				LGD:        lgd,
			}
		} else {
			nonSovereign = append(nonSovereign, string(ac))
		}
	}

	if len(nonSovereign) == 0 {
		return result, nil
	}

	// lib/pq supports string arrays as $1::text[].
	rows, err := r.db.QueryContext(ctx, pdLGDBulkSQL,
		"{"+strings.Join(nonSovereign, ",")+"}",
		evaluationDate, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pd_lgd bulk query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	seen := make(map[string]bool)
	for rows.Next() {
		var acStr, pdGoodStr, pdNormalStr, pdBadStr, lgdStr string
		if err := rows.Scan(&acStr, &pdGoodStr, &pdNormalStr, &pdBadStr, &lgdStr); err != nil {
			return nil, fmt.Errorf("pd_lgd row scan: %w", err)
		}
		if seen[acStr] {
			continue // take first (latest effective_from due to ORDER BY)
		}
		seen[acStr] = true
		pdGood, e1 := decimal.NewFromString(pdGoodStr)
		pdNormal, e2 := decimal.NewFromString(pdNormalStr)
		pdBad, e3 := decimal.NewFromString(pdBadStr)
		lgd, e4 := decimal.NewFromString(lgdStr)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return nil, fmt.Errorf("pd_lgd decimal parse for %s: %v %v %v %v", acStr, e1, e2, e3, e4)
		}
		result[AssetClass(acStr)] = PDLGDParams{
			AssetClass: AssetClass(acStr),
			PDGood:     pdGood,
			PDNormal:   pdNormal,
			PDBad:      pdBad,
			LGD:        lgd,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

const sovereignLGDSQL = `
SELECT lgd_pct::text
FROM mst.lgd_basel
WHERE tipe_eksposur = 'SOVEREIGN'
  AND workflow_status = 'APPROVED'
  AND tenant_id = $1
  AND effective_from <= $2
  AND (effective_to IS NULL OR effective_to >= $2)
ORDER BY effective_from DESC
LIMIT 1`

// getSovereignLGD looks up LGD for GOVT_BOND (tipe_eksposur='SOVEREIGN').
func (r *DBPDLGDClassRepo) getSovereignLGD(ctx context.Context, evaluationDate time.Time, tenantID string) (decimal.Decimal, error) {
	var lgdStr string
	err := r.db.QueryRowContext(ctx, sovereignLGDSQL, tenantID, evaluationDate).Scan(&lgdStr)
	if err == sql.ErrNoRows {
		// Conservative fallback: 0 LGD for sovereign — per DEC-015 GOVT_BOND note.
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("sovereign lgd lookup: %w", err)
	}
	return decimal.NewFromString(lgdStr)
}

// ─── ScenarioParamRepo ────────────────────────────────────────────────────────

// ScenarioParamRepo loads ALCO-approved scenario weights + FL multipliers.
type ScenarioParamRepo interface {
	// GetScenarioWeights returns ALCO-approved bobot per scenario for the given period.
	// Returns default (0.25/0.50/0.25) if no override found (DEC-010).
	GetScenarioWeights(ctx context.Context, periodeID uuid.UUID, tenantID string) (ScenarioWeights, error)

	// GetFLMultipliers returns ALCO-approved FL multipliers per scenario for the given period.
	// Returns (1,1,1) for all if no multiplier found (no dual-FL applied — multiplier neutral).
	GetFLMultipliers(ctx context.Context, periodeID uuid.UUID, tenantID string) (FLMultipliers, error)
}

// DBScenarioParamRepo implements ScenarioParamRepo.
type DBScenarioParamRepo struct {
	db *sql.DB
}

// NewDBScenarioParamRepo creates a DBScenarioParamRepo.
func NewDBScenarioParamRepo(db *sql.DB) *DBScenarioParamRepo {
	if db == nil {
		panic("lookthrough: DBScenarioParamRepo requires non-nil db")
	}
	return &DBScenarioParamRepo{db: db}
}

// Default scenario weights per DEC-010.
var defaultWeights = ScenarioWeights{
	Good:   decimal.NewFromFloat(0.25),
	Normal: decimal.NewFromFloat(0.50),
	Bad:    decimal.NewFromFloat(0.25),
}

// defaultFL is the neutral FL multiplier (no adjustment — multiply by 1).
var defaultFL = FLMultipliers{
	Good:   decimal.NewFromInt(1),
	Normal: decimal.NewFromInt(1),
	Bad:    decimal.NewFromInt(1),
}

const scenarioWeightsSQL = `
SELECT bobot_good::text, bobot_normal::text, bobot_bad::text
FROM mst.ecl_scenario_weight
WHERE workflow_status = 'APPROVED'
  AND periode_id = $1
  AND tenant_id = $2
ORDER BY created_at DESC
LIMIT 1`

// GetScenarioWeights returns ALCO-approved weights or defaults.
func (r *DBScenarioParamRepo) GetScenarioWeights(ctx context.Context, periodeID uuid.UUID, tenantID string) (ScenarioWeights, error) {
	var gStr, nStr, bStr string
	err := r.db.QueryRowContext(ctx, scenarioWeightsSQL, periodeID, tenantID).Scan(&gStr, &nStr, &bStr)
	if err == sql.ErrNoRows {
		return defaultWeights, nil
	}
	if err != nil {
		return ScenarioWeights{}, fmt.Errorf("scenario weights lookup: %w", err)
	}
	g, e1 := decimal.NewFromString(gStr)
	n, e2 := decimal.NewFromString(nStr)
	b, e3 := decimal.NewFromString(bStr)
	if e1 != nil || e2 != nil || e3 != nil {
		return ScenarioWeights{}, fmt.Errorf("scenario weights parse: %v %v %v", e1, e2, e3)
	}
	return ScenarioWeights{Good: g, Normal: n, Bad: b}, nil
}

const flMultipliersSQL = `
SELECT multiplier_good::text, multiplier_normal::text, multiplier_bad::text
FROM mst.impact_mev_pd
WHERE workflow_status = 'APPROVED'
  AND periode_id = $1
  AND tenant_id = $2
ORDER BY created_at DESC
LIMIT 1`

// GetFLMultipliers returns ALCO-approved FL multipliers or neutral defaults.
func (r *DBScenarioParamRepo) GetFLMultipliers(ctx context.Context, periodeID uuid.UUID, tenantID string) (FLMultipliers, error) {
	var gStr, nStr, bStr string
	err := r.db.QueryRowContext(ctx, flMultipliersSQL, periodeID, tenantID).Scan(&gStr, &nStr, &bStr)
	if err == sql.ErrNoRows {
		return defaultFL, nil
	}
	if err != nil {
		return FLMultipliers{}, fmt.Errorf("fl multipliers lookup: %w", err)
	}
	g, e1 := decimal.NewFromString(gStr)
	n, e2 := decimal.NewFromString(nStr)
	b, e3 := decimal.NewFromString(bStr)
	if e1 != nil || e2 != nil || e3 != nil {
		return FLMultipliers{}, fmt.Errorf("fl multipliers parse: %v %v %v", e1, e2, e3)
	}
	return FLMultipliers{Good: g, Normal: n, Bad: b}, nil
}

// ─── LookthroughResultRepo ────────────────────────────────────────────────────

// ResultRepo persists look-through ECL results to ecl.lookthrough_underlying.
// No hard delete (ecl schema rule DEC-018).
type ResultRepo interface {
	// UpsertResult inserts or updates the ecl.lookthrough_underlying row for (instrumen_id, run_id).
	// tx must be an open transaction (audit-in-tx DEC-018).
	// actorID is the user UUID from JWT claims (created_by / updated_by).
	UpsertResult(ctx context.Context, tx *sql.Tx, instrumenID, runID uuid.UUID,
		result Result, compositionID uuid.UUID, periodeID uuid.UUID,
		evaluationDate time.Time, actorID uuid.UUID, tenantID string) error

	// GetByInstrumenAndRun fetches the stored result for (instrumen_id, run_id).
	GetByInstrumenAndRun(ctx context.Context, instrumenID, runID uuid.UUID) (*StoredLookthroughResult, error)
}

// StoredLookthroughResult is the projection of ecl.lookthrough_underlying for read endpoints.
type StoredLookthroughResult struct {
	ID             uuid.UUID
	InstrumenID    uuid.UUID
	RunID          uuid.UUID
	CompositionID  uuid.UUID
	PeriodeID      uuid.UUID
	EvaluationDate time.Time
	NABIDR         decimal.Decimal
	TotalECLIDR    decimal.Decimal
	BreakdownJSONB []byte // raw JSONB from DB — decoded by caller if needed
	FVTPLSkipped   bool
	Warning        string
	CreatedAt      time.Time
	TenantID       string
}

// DBLookthroughResultRepo implements LookthroughResultRepo.
type DBLookthroughResultRepo struct {
	db *sql.DB
}

// NewDBLookthroughResultRepo creates a DBLookthroughResultRepo.
func NewDBLookthroughResultRepo(db *sql.DB) *DBLookthroughResultRepo {
	if db == nil {
		panic("lookthrough: DBLookthroughResultRepo requires non-nil db")
	}
	return &DBLookthroughResultRepo{db: db}
}

const upsertLookthroughResultSQL = `
INSERT INTO ecl.lookthrough_underlying
  (id, instrumen_id, run_id, fund_composition_id, periode_id,
   evaluation_date, nab_idr, total_ecl_idr, breakdown_jsonb,
   fvtpl_skipped, warning,
   created_at, created_by, updated_at, updated_by, row_version, tenant_id)
VALUES
  ($1, $2, $3, $4, $5,
   $6, $7::numeric, $8::numeric, $9::jsonb,
   $10, $11,
   now(), $12, now(), $12, 1, $13)
ON CONFLICT (instrumen_id, run_id) DO UPDATE
SET fund_composition_id = EXCLUDED.fund_composition_id,
    nab_idr = EXCLUDED.nab_idr,
    total_ecl_idr = EXCLUDED.total_ecl_idr,
    breakdown_jsonb = EXCLUDED.breakdown_jsonb,
    fvtpl_skipped = EXCLUDED.fvtpl_skipped,
    warning = EXCLUDED.warning,
    updated_at = now(),
    updated_by = EXCLUDED.updated_by,
    row_version = ecl.lookthrough_underlying.row_version + 1`

// UpsertResult inserts or updates the look-through result row.
// actorID must be the authenticated user UUID from JWT claims (F4 — no placeholder).
func (r *DBLookthroughResultRepo) UpsertResult(ctx context.Context, tx *sql.Tx,
	instrumenID, runID uuid.UUID,
	result Result, compositionID uuid.UUID, periodeID uuid.UUID,
	evaluationDate time.Time, actorID uuid.UUID, tenantID string,
) error {
	id := uuid.New()
	breakdownJSON := marshalBreakdown(result.Breakdown)
	_, err := tx.ExecContext(ctx, upsertLookthroughResultSQL,
		id, instrumenID, runID, compositionID, periodeID,
		evaluationDate,
		result.NABIDR.StringFixed(4),
		result.TotalECLIDR.StringFixed(4),
		breakdownJSON,
		result.FVTPLSkipped, result.Warning,
		actorID, // created_by / updated_by from JWT actor (DEC-018)
		tenantID,
	)
	return err
}

const getLookthroughResultSQL = `
SELECT id, instrumen_id, run_id, fund_composition_id, periode_id,
       evaluation_date, nab_idr::text, total_ecl_idr::text,
       breakdown_jsonb, fvtpl_skipped, COALESCE(warning,''),
       created_at, tenant_id
FROM ecl.lookthrough_underlying
WHERE instrumen_id = $1 AND run_id = $2
  AND deleted_at IS NULL`

// GetByInstrumenAndRun fetches a stored result.
func (r *DBLookthroughResultRepo) GetByInstrumenAndRun(ctx context.Context, instrumenID, runID uuid.UUID) (*StoredLookthroughResult, error) {
	var res StoredLookthroughResult
	var nabStr, eclStr string
	err := r.db.QueryRowContext(ctx, getLookthroughResultSQL, instrumenID, runID).Scan(
		&res.ID, &res.InstrumenID, &res.RunID, &res.CompositionID, &res.PeriodeID,
		&res.EvaluationDate, &nabStr, &eclStr,
		&res.BreakdownJSONB, &res.FVTPLSkipped, &res.Warning,
		&res.CreatedAt, &res.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e1, e2 error
	if res.NABIDR, e1 = decimal.NewFromString(nabStr); e1 != nil {
		return nil, fmt.Errorf("nab_idr parse: %w", e1)
	}
	if res.TotalECLIDR, e2 = decimal.NewFromString(eclStr); e2 != nil {
		return nil, fmt.Errorf("total_ecl_idr parse: %w", e2)
	}
	return &res, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

// scanFundCompositionRow scans a *sql.Row into FundComposition.
// Returns (nil, nil) on sql.ErrNoRows.
func scanFundCompositionRow(row *sql.Row) (*FundComposition, error) {
	var fc FundComposition
	err := row.Scan(
		&fc.ID, &fc.InstrumenID,
		&fc.EffectiveFrom, &fc.EffectiveTo,
		&fc.WorkflowStatus,
		&fc.MakerID, &fc.ReviewerID, &fc.ApproverID,
		&fc.SignedAtReview, &fc.SignatureHashReview, &fc.CommentReview,
		&fc.SignedAtApprove, &fc.SignatureHashApprove, &fc.CommentApprove,
		&fc.RejectReason, &fc.SourceDocID,
		&fc.CreatedAt, &fc.CreatedBy, &fc.UpdatedAt, &fc.UpdatedBy,
		&fc.DeletedAt, &fc.DeletedBy, &fc.RowVersion, &fc.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &fc, err
}

// scanFundCompositionRows scans multiple rows into []FundComposition.
func scanFundCompositionRows(rows *sql.Rows) ([]FundComposition, error) {
	var result []FundComposition
	for rows.Next() {
		var fc FundComposition
		err := rows.Scan(
			&fc.ID, &fc.InstrumenID,
			&fc.EffectiveFrom, &fc.EffectiveTo,
			&fc.WorkflowStatus,
			&fc.MakerID, &fc.ReviewerID, &fc.ApproverID,
			&fc.SignedAtReview, &fc.SignatureHashReview, &fc.CommentReview,
			&fc.SignedAtApprove, &fc.SignatureHashApprove, &fc.CommentApprove,
			&fc.RejectReason, &fc.SourceDocID,
			&fc.CreatedAt, &fc.CreatedBy, &fc.UpdatedAt, &fc.UpdatedBy,
			&fc.DeletedAt, &fc.DeletedBy, &fc.RowVersion, &fc.TenantID,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, fc)
	}
	return result, rows.Err()
}

// scanFundCompositionDetailRows scans multiple rows into []FundCompositionDetail.
func scanFundCompositionDetailRows(rows *sql.Rows) ([]FundCompositionDetail, error) {
	var result []FundCompositionDetail
	for rows.Next() {
		var d FundCompositionDetail
		var weightStr string
		err := rows.Scan(
			&d.ID, &d.FundCompositionID,
			&d.AssetClass, &weightStr, &d.Position,
			&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
			&d.DeletedAt, &d.DeletedBy, &d.RowVersion, &d.TenantID,
		)
		if err != nil {
			return nil, err
		}
		w, e := decimal.NewFromString(weightStr)
		if e != nil {
			return nil, fmt.Errorf("weight_pct parse for detail %s: %w", d.ID, e)
		}
		d.WeightPct = w
		result = append(result, d)
	}
	return result, rows.Err()
}

// ─── Cursor helpers ───────────────────────────────────────────────────────────

type cursorData struct {
	SortValue string
	ID        string
}

// encodeCursor creates an opaque base64-like cursor string.
func encodeCursor(sortValue, id string) string {
	return sortValue + "|" + id
}

// decodeCursor splits the cursor string.
func decodeCursor(cursor string) (cursorData, error) {
	idx := strings.LastIndex(cursor, "|")
	if idx < 0 {
		return cursorData{}, fmt.Errorf("invalid cursor")
	}
	return cursorData{SortValue: cursor[:idx], ID: cursor[idx+1:]}, nil
}

// isAllowedSortCol checks if col is in the whitelist slice.
func isAllowedSortCol(col string, allowed []string) bool {
	for _, a := range allowed {
		if a == col {
			return true
		}
	}
	return false
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// marshalBreakdown serializes []BreakdownLine to JSON for JSONB storage.
// Uses string representation of Decimal to preserve precision (not float64).
func marshalBreakdown(lines []BreakdownLine) []byte {
	if len(lines) == 0 {
		return []byte("[]")
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i := range lines {
		l := &lines[i]
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(
			`{"asset_class":%q,"weight_pct":%q,"nab_portion_idr":%q,`+
				`"pd_good":%q,"pd_normal":%q,"pd_bad":%q,"lgd":%q,`+
				`"ecl_good_idr":%q,"ecl_normal_idr":%q,"ecl_bad_idr":%q,`+
				`"ecl_fl_good_idr":%q,"ecl_fl_normal_idr":%q,"ecl_fl_bad_idr":%q,`+
				`"ecl_weighted_idr":%q}`,
			string(l.AssetClass),
			l.WeightPct.StringFixed(4),
			l.NABPortionIDR.StringFixed(4),
			l.PDGood.StringFixed(8),
			l.PDNormal.StringFixed(8),
			l.PDBad.StringFixed(8),
			l.LGD.StringFixed(8),
			l.ECLSkenariosGoodIDR.StringFixed(4),
			l.ECLSkenariosNormalIDR.StringFixed(4),
			l.ECLSkenariosBadIDR.StringFixed(4),
			l.ECLFLGoodIDR.StringFixed(4),
			l.ECLFLNormalIDR.StringFixed(4),
			l.ECLFLBadIDR.StringFixed(4),
			l.ECLWeightedIDR.StringFixed(4),
		))
	}
	sb.WriteString("]")
	return []byte(sb.String())
}
