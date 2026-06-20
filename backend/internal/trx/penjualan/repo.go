package penjualan

// repo.go — sqlx repository for trx.penjualan.
// No business logic. Only SQL + mapping.
// Service owns transactions; repo never opens tx.
// Cursor pagination uses (created_at DESC, id DESC) keyset — same M7 F2 pattern.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Repository defines the data access interface for trx.penjualan.
type Repository interface {
	// GetByID fetches a penjualan by UUID. Returns (nil, nil) if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*Penjualan, error)

	// GetInstrumenInfo fetches minimal instrumen data for eligibility checks.
	GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenInfo, error)

	// HasActivePenjualan returns true if instrumen_id already has an active disposal.
	HasActivePenjualan(ctx context.Context, instrumenID uuid.UUID) (bool, error)

	// GetOCICumulativeByInstrumen fetches the current OCI cumulative for an instrumen
	// from the latest trx.mtm record. Returns (zero, nil) if no MTM record exists.
	GetOCICumulativeByInstrumen(ctx context.Context, instrumenID uuid.UUID) (decimal.Decimal, error)

	// GetAmortizedCarryingByInstrumen returns the amortized carrying amount from
	// ecl.amortisasi_schedule for the given instrumen and date.
	GetAmortizedCarryingByInstrumen(ctx context.Context, instrumenID uuid.UUID, tanggal time.Time) (decimal.Decimal, error)

	// GetRolling12mDisposalIDR returns the total proceed_idr for POSTED disposals
	// in the same portofolio within the last 12 months (excluding current).
	GetRolling12mDisposalIDR(ctx context.Context, portofolioID uuid.UUID) (decimal.Decimal, error)

	// GetPortofolioNilai returns the total nilai instrumen in a portfolio.
	GetPortofolioNilai(ctx context.Context, portofolioID uuid.UUID) (decimal.Decimal, error)

	// GetBMConfigThresholds reads warn and block thresholds from sys.config_param.
	GetBMConfigThresholds(ctx context.Context) (warn, block decimal.Decimal, err error)

	// Insert inserts a new penjualan row inside the given tx.
	Insert(ctx context.Context, tx *sql.Tx, p *Penjualan) error

	// UpdateStatus updates workflow status fields inside the given tx.
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, u StatusUpdate) error

	// List returns paginated penjualan rows.
	List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Penjualan, bool, int, error)

	// ListBMAlerts returns instrumen with bm_violation_risk=TRUE.
	ListBMAlerts(ctx context.Context) ([]*BMAlertItem, error)

	// GetPeriodeByTanggal looks up mst.periode_buku. Returns nil if not found.
	GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBuku, error)

	// BeginTx starts a new DB transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// PeriodeBuku holds minimal period data.
type PeriodeBuku struct {
	ID            uuid.UUID `db:"id"`
	StatusPeriode string    `db:"status_periode"`
	TanggalMulai  time.Time `db:"tanggal_mulai"`
	TanggalAkhir  time.Time `db:"tanggal_akhir"`
}

// Repo is the concrete database/sql implementation of Repository.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// BeginTx starts a database transaction.
func (r *Repo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx: %w", err)
	}
	return tx, nil
}

// GetByID fetches one penjualan by UUID.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*Penjualan, error) {
	const q = `
		SELECT id, instrumen_id, jenis_disposal, qty_terjual::text, qty_holding_pre::text,
		       COALESCE(qty_holding_post::text, '') AS qty_holding_post_str,
		       harga_jual_per_unit::text, proceed::text, cost_basis::text, realized_gl::text,
		       COALESCE(oci_recycled::text, '') AS oci_recycled_str,
		       COALESCE(oci_cumulative_total::text, '') AS oci_cumulative_str,
		       klasifikasi_snapshot, jurnal_event_code, tanggal_eksekusi,
		       bm_violation_risk,
		       COALESCE(bm_violation_pct::text, '') AS bm_violation_pct_str,
		       status, maker_id, approver_id,
		       approve_comment, reject_reason, signature_method,
		       approved_at, jurnal_header_id, periode_bulanan_id, instrumen_status_after,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.penjualan
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var (
		p                   Penjualan
		qtyTerjualStr       string
		qtyHoldingPreStr    string
		qtyHoldingPostStr   string
		hargaStr            string
		proceedStr          string
		costBasisStr        string
		realizedGLStr       string
		ociRecycledStr      string
		ociCumulativeStr    string
		bmPctStr            string
	)

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.InstrumenID, &p.JenisDisposal,
		&qtyTerjualStr, &qtyHoldingPreStr, &qtyHoldingPostStr,
		&hargaStr, &proceedStr, &costBasisStr, &realizedGLStr,
		&ociRecycledStr, &ociCumulativeStr,
		&p.KlasifikasiSnapshot, &p.JurnalEventCode, &p.TanggalEksekusi,
		&p.BMViolationRisk, &bmPctStr,
		&p.Status, &p.MakerID, &p.ApproverID,
		&p.ApproveComment, &p.RejectReason, &p.SignatureMethod,
		&p.ApprovedAt, &p.JurnalHeaderID, &p.PeriodeBulananID, &p.InstrumenStatusAfter,
		&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.UpdatedBy,
		&p.DeletedAt, &p.DeletedBy, &p.RowVersion, &p.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetByID: %w", err)
	}

	parseDecField := func(s string, target *decimal.Decimal) {
		if s != "" {
			if v, e := decimal.NewFromString(s); e == nil {
				*target = v
			}
		}
	}
	parseDecPtr := func(s string) *decimal.Decimal {
		if s == "" {
			return nil
		}
		if v, e := decimal.NewFromString(s); e == nil {
			return &v
		}
		return nil
	}

	parseDecField(qtyTerjualStr, &p.QtyTerjual)
	parseDecField(qtyHoldingPreStr, &p.QtyHoldingPre)
	p.QtyHoldingPost = parseDecPtr(qtyHoldingPostStr)
	parseDecField(hargaStr, &p.HargaJualPerUnit)
	parseDecField(proceedStr, &p.Proceed)
	parseDecField(costBasisStr, &p.CostBasis)
	parseDecField(realizedGLStr, &p.RealizedGL)
	p.OCIRecycled = parseDecPtr(ociRecycledStr)
	p.OCICumulativeTotal = parseDecPtr(ociCumulativeStr)
	p.BMViolationPct = parseDecPtr(bmPctStr)

	return &p, nil
}

// GetInstrumenInfo fetches minimal instrumen data from mst.instrumen.
func (r *Repo) GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenInfo, error) {
	const q = `
		SELECT i.id, i.kode_instrumen, i.nama_instrumen,
		       COALESCE(i.status, '') AS status,
		       COALESCE(i.klasifikasi_psak71, '') AS klasifikasi_psak71,
		       i.klasifikasi_locked,
		       COALESCE(i.qty_holding::text, '0') AS qty_holding_str,
		       COALESCE(i.harga_perolehan::text, '0') AS harga_perolehan_str,
		       i.portofolio_id,
		       COALESCE(p.business_model, '') AS business_model,
		       COALESCE(i.mata_uang, 'IDR') AS mata_uang,
		       i.counterparty_id,
		       i.sppi_test_run_id, i.bm_assessment_id
		FROM mst.instrumen i
		LEFT JOIN mst.portofolio p ON p.id = i.portofolio_id
		WHERE i.id = $1 AND i.deleted_at IS NULL
		LIMIT 1`

	var (
		inst           InstrumenInfo
		qtyHoldingStr  string
		hargaPerolehan string
	)

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&inst.ID, &inst.KodeInstrumen, &inst.NamaInstrumen,
		&inst.Status, &inst.KlasifikasiPSAK71, &inst.KlasifikasiLocked,
		&qtyHoldingStr, &hargaPerolehan,
		&inst.PortofolioID, &inst.BusinessModel, &inst.MataUang,
		&inst.CounterpartyID,
		&inst.SppiTestRunID, &inst.BmAssessmentID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: %w", err)
	}

	if v, e := decimal.NewFromString(qtyHoldingStr); e == nil {
		inst.QtyHolding = v
	}
	if v, e := decimal.NewFromString(hargaPerolehan); e == nil {
		inst.HargaPerolehan = v
	}
	return &inst, nil
}

// HasActivePenjualan checks for existing active disposal on instrumen_id.
func (r *Repo) HasActivePenjualan(ctx context.Context, instrumenID uuid.UUID) (bool, error) {
	const q = `
		SELECT COUNT(*)
		FROM trx.penjualan
		WHERE instrumen_id = $1
		  AND status IN ('PENDING_APPROVAL', 'APPROVED', 'PENDING_BM_REVIEW')
		  AND deleted_at IS NULL`

	var count int
	if err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&count); err != nil {
		return false, fmt.Errorf("repo.HasActivePenjualan: %w", err)
	}
	return count > 0, nil
}

// GetOCICumulativeByInstrumen fetches cumulative OCI from latest trx.mtm.
func (r *Repo) GetOCICumulativeByInstrumen(ctx context.Context, instrumenID uuid.UUID) (decimal.Decimal, error) {
	const q = `
		SELECT COALESCE(oci_cumulative::text, '0')
		FROM trx.mtm
		WHERE instrumen_id = $1 AND deleted_at IS NULL
		ORDER BY tanggal_penilaian DESC
		LIMIT 1`

	var s string
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&s)
	if err == sql.ErrNoRows {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repo.GetOCICumulativeByInstrumen: %w", err)
	}
	v, e := decimal.NewFromString(s)
	if e != nil {
		return decimal.Zero, fmt.Errorf("repo.GetOCICumulativeByInstrumen: parse '%s': %w", s, e)
	}
	return v, nil
}

// GetAmortizedCarryingByInstrumen returns amortized carrying from ecl.amortisasi_schedule.
func (r *Repo) GetAmortizedCarryingByInstrumen(ctx context.Context, instrumenID uuid.UUID, tanggal time.Time) (decimal.Decimal, error) {
	const q = `
		SELECT COALESCE(carrying_amount::text, '0')
		FROM ecl.amortisasi_schedule
		WHERE instrumen_id = $1
		  AND $2 BETWEEN effective_from AND effective_to
		  AND deleted_at IS NULL
		ORDER BY schedule_version DESC
		LIMIT 1`

	var s string
	err := r.db.QueryRowContext(ctx, q, instrumenID, tanggal).Scan(&s)
	if err == sql.ErrNoRows {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repo.GetAmortizedCarryingByInstrumen: %w", err)
	}
	v, e := decimal.NewFromString(s)
	if e != nil {
		return decimal.Zero, fmt.Errorf("repo.GetAmortizedCarryingByInstrumen: parse '%s': %w", s, e)
	}
	return v, nil
}

// GetRolling12mDisposalIDR returns cumulative posted disposal proceeds for a portofolio in last 12 months.
func (r *Repo) GetRolling12mDisposalIDR(ctx context.Context, portofolioID uuid.UUID) (decimal.Decimal, error) {
	const q = `
		SELECT COALESCE(SUM(p.proceed), 0)::text
		FROM trx.penjualan p
		JOIN mst.instrumen i ON i.id = p.instrumen_id
		WHERE i.portofolio_id = $1
		  AND p.status = 'POSTED'
		  AND p.tanggal_eksekusi >= (CURRENT_DATE - INTERVAL '12 months')
		  AND p.deleted_at IS NULL`

	var s string
	if err := r.db.QueryRowContext(ctx, q, portofolioID).Scan(&s); err != nil {
		return decimal.Zero, fmt.Errorf("repo.GetRolling12mDisposalIDR: %w", err)
	}
	v, e := decimal.NewFromString(s)
	if e != nil {
		return decimal.Zero, fmt.Errorf("repo.GetRolling12mDisposalIDR: parse '%s': %w", s, e)
	}
	return v, nil
}

// GetPortofolioNilai returns total nilai instrumen in a portfolio.
func (r *Repo) GetPortofolioNilai(ctx context.Context, portofolioID uuid.UUID) (decimal.Decimal, error) {
	const q = `
		SELECT COALESCE(SUM(qty_holding * harga_perolehan), 0)::text
		FROM mst.instrumen
		WHERE portofolio_id = $1
		  AND status = 'ACTIVE'
		  AND deleted_at IS NULL`

	var s string
	if err := r.db.QueryRowContext(ctx, q, portofolioID).Scan(&s); err != nil {
		return decimal.Zero, fmt.Errorf("repo.GetPortofolioNilai: %w", err)
	}
	v, e := decimal.NewFromString(s)
	if e != nil {
		return decimal.Zero, fmt.Errorf("repo.GetPortofolioNilai: parse '%s': %w", s, e)
	}
	return v, nil
}

// GetBMConfigThresholds reads BM thresholds from sys.config_param.
func (r *Repo) GetBMConfigThresholds(ctx context.Context) (warn, block decimal.Decimal, err error) {
	const q = `
		SELECT key, value
		FROM sys.config_param
		WHERE key IN ('PENJUALAN_BM_WARN_THRESHOLD_PCT', 'PENJUALAN_BM_BLOCK_THRESHOLD_PCT')
		  AND deleted_at IS NULL`

	rows, e := r.db.QueryContext(ctx, q)
	if e != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("repo.GetBMConfigThresholds: %w", e)
	}
	defer rows.Close() //nolint:errcheck

	// defaults
	warn = decimal.NewFromInt(5)
	block = decimal.NewFromInt(10)

	for rows.Next() {
		var key, val string
		if e2 := rows.Scan(&key, &val); e2 != nil {
			continue
		}
		v, e3 := decimal.NewFromString(val)
		if e3 != nil {
			continue
		}
		switch key {
		case "PENJUALAN_BM_WARN_THRESHOLD_PCT":
			warn = v
		case "PENJUALAN_BM_BLOCK_THRESHOLD_PCT":
			block = v
		}
	}
	return warn, block, rows.Err()
}

// Insert inserts a new penjualan row.
func (r *Repo) Insert(ctx context.Context, tx *sql.Tx, p *Penjualan) error {
	const q = `
		INSERT INTO trx.penjualan (
			id, instrumen_id, jenis_disposal, qty_terjual, qty_holding_pre,
			qty_holding_post, harga_jual_per_unit, proceed, cost_basis, realized_gl,
			oci_recycled, oci_cumulative_total,
			klasifikasi_snapshot, jurnal_event_code, tanggal_eksekusi,
			bm_violation_risk, bm_violation_pct,
			status, maker_id, approver_id,
			approve_comment, reject_reason, signature_method, signature_hash_meta,
			approved_at, jurnal_header_id, periode_bulanan_id, instrumen_status_after,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12,
			$13, $14, $15,
			$16, $17,
			$18, $19, $20,
			$21, $22, $23, $24,
			$25, $26, $27, $28,
			$29, $30, $31, $32, $33, $34
		)`

	_, err := tx.ExecContext(ctx, q,
		p.ID, p.InstrumenID, string(p.JenisDisposal),
		p.QtyTerjual.StringFixed(8), p.QtyHoldingPre.StringFixed(8),
		decimalPtrToStr(p.QtyHoldingPost, 8),
		p.HargaJualPerUnit.StringFixed(4), p.Proceed.StringFixed(4),
		p.CostBasis.StringFixed(4), p.RealizedGL.StringFixed(4),
		decimalPtrToStr(p.OCIRecycled, 4), decimalPtrToStr(p.OCICumulativeTotal, 4),
		string(p.KlasifikasiSnapshot), p.JurnalEventCode, p.TanggalEksekusi,
		p.BMViolationRisk, decimalPtrToStr(p.BMViolationPct, 4),
		string(p.Status), p.MakerID, p.ApproverID,
		p.ApproveComment, p.RejectReason, p.SignatureMethod, p.SignatureHashMeta,
		p.ApprovedAt, p.JurnalHeaderID, p.PeriodeBulananID, p.InstrumenStatusAfter,
		p.CreatedAt, p.CreatedBy, p.UpdatedAt, p.UpdatedBy, p.RowVersion, p.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.Insert: %w", err)
	}
	return nil
}

// UpdateStatus updates workflow status and optional related fields with optimistic lock.
func (r *Repo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, u StatusUpdate) error {
	const q = `
		UPDATE trx.penjualan SET
			status                = $1,
			approver_id           = $2,
			approve_comment       = $3,
			reject_reason         = $4,
			signature_method      = $5,
			signature_hash_meta   = $6,
			approved_at           = $7,
			jurnal_header_id      = $8,
			qty_holding_post      = $9::NUMERIC,
			oci_recycled          = $10::NUMERIC,
			bm_violation_risk     = $11,
			bm_violation_pct      = $12::NUMERIC,
			jurnal_event_code     = $13,
			instrumen_status_after = $14,
			updated_by            = $15,
			updated_at            = NOW()
		WHERE id = $16
		  AND row_version = $17
		  AND deleted_at IS NULL`

	res, err := tx.ExecContext(ctx, q,
		string(u.Status),
		u.ApproverID,
		u.ApproveComment,
		u.RejectReason,
		u.SignatureMethod,
		u.SignatureHashMeta,
		u.ApprovedAt,
		u.JurnalHeaderID,
		decimalPtrToStr(u.QtyHoldingPost, 8),
		decimalPtrToStr(u.OCIRecycled, 4),
		u.BMViolationRisk,
		decimalPtrToStr(u.BMViolationPct, 4),
		u.JurnalEventCode,
		u.InstrumenStatusAfter,
		u.UpdatedBy,
		id,
		u.RowVersion,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repo.UpdateStatus: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("repo.UpdateStatus: optimistic lock conflict for penjualan %s (row_version %d)", id, u.RowVersion)
	}
	return nil
}

// List returns paginated penjualan rows filtered by tenant_id.
// Cursor format: base64(created_at_rfc3339nano + "|" + id_uuid).
func (r *Repo) List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Penjualan, bool, int, error) {
	var cursorTime time.Time
	var cursorID uuid.UUID
	if cursor != "" {
		ct, cid, err := decodeCursor(cursor)
		if err != nil {
			return nil, false, 0, fmt.Errorf("repo.List: invalid cursor: %w", err)
		}
		cursorTime, cursorID = ct, cid
	}

	const baseQ = `
		SELECT id, instrumen_id, jenis_disposal,
		       qty_terjual::text, qty_holding_pre::text,
		       COALESCE(qty_holding_post::text,'') AS qty_holding_post_str,
		       harga_jual_per_unit::text, proceed::text,
		       cost_basis::text, realized_gl::text,
		       COALESCE(oci_recycled::text,'') AS oci_recycled_str,
		       COALESCE(oci_cumulative_total::text,'') AS oci_cumulative_str,
		       klasifikasi_snapshot, jurnal_event_code, tanggal_eksekusi,
		       bm_violation_risk,
		       COALESCE(bm_violation_pct::text,'') AS bm_pct_str,
		       status, maker_id, approver_id,
		       approve_comment, reject_reason, jurnal_header_id, periode_bulanan_id,
		       instrumen_status_after,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM trx.penjualan
		WHERE deleted_at IS NULL
		  AND tenant_id = $1`

	const noCursorSuffix = `
		ORDER BY created_at DESC, id DESC
		LIMIT $2`

	const cursorSuffix = `
		  AND (created_at, id) < ($3, $4)
		ORDER BY created_at DESC, id DESC
		LIMIT $2`

	tenantID := "TUGURE"

	var (
		sqlRows *sql.Rows
		err     error
	)
	if cursor == "" {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+noCursorSuffix, tenantID, limit+1)
	} else {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+cursorSuffix, tenantID, limit+1, cursorTime, cursorID)
	}
	if err != nil {
		return nil, false, 0, fmt.Errorf("repo.List: %w", err)
	}
	defer sqlRows.Close() //nolint:errcheck

	parseDP := func(s string) *decimal.Decimal {
		if s == "" {
			return nil
		}
		v, e := decimal.NewFromString(s)
		if e != nil {
			return nil
		}
		return &v
	}

	var result []*Penjualan
	for sqlRows.Next() {
		var (
			p                  Penjualan
			qtyTStr            string
			qtyHPreStr         string
			qtyHPostStr        string
			hargaStr           string
			proceedStr         string
			costStr            string
			realizedStr        string
			ociRecStr          string
			ociCumStr          string
			bmPctStr           string
		)
		if err := sqlRows.Scan(
			&p.ID, &p.InstrumenID, &p.JenisDisposal,
			&qtyTStr, &qtyHPreStr, &qtyHPostStr,
			&hargaStr, &proceedStr, &costStr, &realizedStr,
			&ociRecStr, &ociCumStr,
			&p.KlasifikasiSnapshot, &p.JurnalEventCode, &p.TanggalEksekusi,
			&p.BMViolationRisk, &bmPctStr,
			&p.Status, &p.MakerID, &p.ApproverID,
			&p.ApproveComment, &p.RejectReason, &p.JurnalHeaderID, &p.PeriodeBulananID,
			&p.InstrumenStatusAfter,
			&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.UpdatedBy,
			&p.RowVersion, &p.TenantID,
		); err != nil {
			return nil, false, 0, fmt.Errorf("repo.List: scan: %w", err)
		}
		if v, e := decimal.NewFromString(qtyTStr); e == nil {
			p.QtyTerjual = v
		}
		if v, e := decimal.NewFromString(qtyHPreStr); e == nil {
			p.QtyHoldingPre = v
		}
		p.QtyHoldingPost = parseDP(qtyHPostStr)
		if v, e := decimal.NewFromString(hargaStr); e == nil {
			p.HargaJualPerUnit = v
		}
		if v, e := decimal.NewFromString(proceedStr); e == nil {
			p.Proceed = v
		}
		if v, e := decimal.NewFromString(costStr); e == nil {
			p.CostBasis = v
		}
		if v, e := decimal.NewFromString(realizedStr); e == nil {
			p.RealizedGL = v
		}
		p.OCIRecycled = parseDP(ociRecStr)
		p.OCICumulativeTotal = parseDP(ociCumStr)
		p.BMViolationPct = parseDP(bmPctStr)
		result = append(result, &p)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, false, 0, fmt.Errorf("repo.List: iterate: %w", err)
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, len(result), nil
}

// ListBMAlerts returns BMAlertItem rows for instrumen with bm_violation_risk=TRUE.
func (r *Repo) ListBMAlerts(ctx context.Context) ([]*BMAlertItem, error) {
	const q = `
		SELECT p.instrumen_id::text, i.kode_instrumen,
		       i.portofolio_id::text, COALESCE(pt.nama_portofolio, '') AS portofolio_nama,
		       COALESCE(p.bm_violation_pct::text, '0') AS pct,
		       '5.0' AS warn_threshold,
		       '10.0' AS block_threshold,
		       CASE WHEN p.bm_violation_pct > 10 THEN 'BM_VIOLATION_BLOCK' ELSE 'BM_VIOLATION_RISK' END AS flag_status,
		       p.updated_at
		FROM trx.penjualan p
		JOIN mst.instrumen i ON i.id = p.instrumen_id
		LEFT JOIN mst.portofolio pt ON pt.id = i.portofolio_id
		WHERE p.bm_violation_risk = TRUE
		  AND p.deleted_at IS NULL
		  AND p.tenant_id = 'TUGURE'
		GROUP BY p.instrumen_id, i.kode_instrumen, i.portofolio_id, pt.nama_portofolio,
		         p.bm_violation_pct, p.updated_at
		ORDER BY p.bm_violation_pct DESC NULLS LAST`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("repo.ListBMAlerts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*BMAlertItem
	for rows.Next() {
		var item BMAlertItem
		var updatedAt time.Time
		if e := rows.Scan(
			&item.InstrumenID, &item.InstrumenKode,
			&item.PortofolioID, &item.PortofolioNama,
			&item.CumulativeSold12mPct,
			&item.WarnThresholdPct, &item.BlockThresholdPct,
			&item.FlagStatus, &updatedAt,
		); e != nil {
			return nil, fmt.Errorf("repo.ListBMAlerts: scan: %w", e)
		}
		item.LastUpdated = updatedAt.Format(time.RFC3339)
		result = append(result, &item)
	}
	return result, rows.Err()
}

// GetPeriodeByTanggal looks up mst.periode_buku for a given date.
func (r *Repo) GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBuku, error) {
	const q = `
		SELECT id, status_periode, tanggal_mulai, tanggal_akhir
		FROM mst.periode_buku
		WHERE $1 BETWEEN tanggal_mulai AND tanggal_akhir
		  AND deleted_at IS NULL
		LIMIT 1`

	var row PeriodeBuku
	err := r.db.QueryRowContext(ctx, q, tanggal).Scan(
		&row.ID, &row.StatusPeriode, &row.TanggalMulai, &row.TanggalAkhir,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetPeriodeByTanggal: %w", err)
	}
	return &row, nil
}

// ─── cursor helpers ───────────────────────────────────────────────────────────

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor format invalid (expected 'timestamp|uuid')")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor time parse: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor uuid parse: %w", err)
	}
	return t, id, nil
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decimalPtrToStr returns nil if d is nil, else StringFixed(scale).
func decimalPtrToStr(d *decimal.Decimal, scale int32) interface{} {
	if d == nil {
		return nil
	}
	return d.StringFixed(scale)
}

// keep encodeCursor referenced to avoid unused warning in non-handler code
var _ = encodeCursor
