package akrualmaturity

// repo.go — sqlx/database/sql repository for akrualmaturity package.
// No business logic. Only SQL + mapping.
// Service owns transactions; repo never opens tx.
// Cursor pagination uses (created_at DESC, id DESC) keyset — M7 F2 pattern.

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

// PeriodeBuku holds minimal periode buku data for checks.
type PeriodeBuku struct {
	ID            uuid.UUID `db:"id"`
	StatusPeriode string    `db:"status_periode"`
	TanggalMulai  time.Time `db:"tanggal_mulai"`
	TanggalAkhir  time.Time `db:"tanggal_akhir"`
}

// Repository defines the data access interface for akrualmaturity.
type Repository interface {
	// ── Instrumen lookups ─────────────────────────────────────────────────────

	// GetActiveAccruingInstrumens returns ACTIVE instrumen eligible for daily accrual.
	GetActiveAccruingInstrumens(ctx context.Context) ([]*InstrumenAkrualInfo, error)

	// GetActiveMaturityInstrumens returns ACTIVE instrumen with tanggal_jatuh_tempo = tanggal.
	GetActiveMaturityInstrumens(ctx context.Context, tanggal time.Time) ([]*InstrumenAkrualInfo, error)

	// GetInstrumenInfo fetches minimal instrumen info by ID.
	GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenAkrualInfo, error)

	// ── ECL & EIR lookups ────────────────────────────────────────────────────

	// GetSealedECLForInstrumen returns latest sealed ECL result for instrumen (M8 B1 pattern).
	// Returns (nil, nil) if no sealed run exists.
	GetSealedECLForInstrumen(ctx context.Context, instrumenID uuid.UUID) (*ECLSealedResult, error)

	// GetAmortisasiSchedule returns active amortisasi schedule for instrumen at given date.
	// Returns (nil, nil) if no schedule found.
	GetAmortisasiSchedule(ctx context.Context, instrumenID uuid.UUID, tanggal time.Time) (*AmortisasiScheduleRow, error)

	// ── FX rate ──────────────────────────────────────────────────────────────

	// GetFXRateApproved returns FX rate with status APPROVED for mata_uang on tanggal.
	// Returns (nil, nil) if not found.
	GetFXRateApproved(ctx context.Context, mataUang string, tanggal time.Time) (*FXRateApproved, error)

	// ── Periode buku ─────────────────────────────────────────────────────────

	// GetPeriodeByTanggal returns active periode_buku for the given date.
	GetPeriodeByTanggal(ctx context.Context, tanggal time.Time) (*PeriodeBuku, error)

	// ── Holiday calendar ─────────────────────────────────────────────────────

	// IsHoliday returns true if the given date is a holiday.
	IsHoliday(ctx context.Context, tanggal time.Time) (bool, error)

	// ── Config ───────────────────────────────────────────────────────────────

	// GetStaleDaysConfig returns AKRUAL_STAGING_STALE_DAYS from sys.config_param.
	GetStaleDaysConfig(ctx context.Context) (int, error)

	// ── akrual mutations ─────────────────────────────────────────────────────

	// InsertAkrual inserts a new pendapatan_akrual row inside the given tx.
	InsertAkrual(ctx context.Context, tx *sql.Tx, a *PendapatanAkrual) error

	// GetAkrualByID fetches a pendapatan_akrual by ID.
	GetAkrualByID(ctx context.Context, id uuid.UUID) (*PendapatanAkrual, error)

	// UpdateAkrualStatus updates workflow status fields inside the given tx.
	UpdateAkrualStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status AkrualStatus, jurnalHeaderID *uuid.UUID, overrideUserID *uuid.UUID, overrideComment *string, rowVersion int64, updatedBy uuid.UUID) error

	// IsDuplicateAkrual returns true if (instrumen_id, tanggal_akrual, jenis) already exists.
	IsDuplicateAkrual(ctx context.Context, instrumenID uuid.UUID, tanggalAkrual time.Time, jenis AkrualJenis) (bool, error)

	// List returns paginated pendapatan_akrual rows.
	ListAkrual(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*PendapatanAkrual, bool, int, error)

	// GetMTDYTDSummary returns MTD/YTD akrual aggregates.
	GetMTDYTDSummary(ctx context.Context, instrumenID *uuid.UUID, portofolioID *uuid.UUID, year, month int) (*AkrualDashboard, error)

	// ── Jatuh tempo mutations ─────────────────────────────────────────────────

	// InsertJatuhTempo inserts a new trx.jatuh_tempo row inside the given tx.
	InsertJatuhTempo(ctx context.Context, tx *sql.Tx, jt *JatuhTempo) error

	// UpdateJatuhTempoStatus updates trx.jatuh_tempo status inside the given tx.
	UpdateJatuhTempoStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggal time.Time, status JatuhTempoStatus, jurnalHeaderID *uuid.UUID, errorMessage *string, rowVersion int64, updatedBy uuid.UUID) error

	// ListJatuhTempo returns paginated jatuh_tempo rows.
	ListJatuhTempo(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*JatuhTempo, bool, int, error)

	// ── Dividen mutations ────────────────────────────────────────────────────

	// InsertDividen inserts a new trx.dividen row inside the given tx.
	InsertDividen(ctx context.Context, tx *sql.Tx, d *Dividen) error

	// GetDividenByID fetches a dividen by ID.
	GetDividenByID(ctx context.Context, id uuid.UUID) (*Dividen, error)

	// UpdateDividenStatus updates dividen status inside the given tx.
	UpdateDividenStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggal time.Time, status DividenStatus, approverID *uuid.UUID, comment *string, rejectReason *string, sigMethod *string, approvedAt *time.Time, jurnalHeaderID *uuid.UUID, rowVersion int64, updatedBy uuid.UUID) error

	// ── DLQ ──────────────────────────────────────────────────────────────────

	// InsertDLQ inserts a DLQ entry for a failed cron item.
	InsertDLQ(ctx context.Context, jobType string, instrumenID uuid.UUID, errorCode string, errorDetail string) error

	// ── TX ───────────────────────────────────────────────────────────────────

	// BeginTx starts a new DB transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// GetLastAkrualForInstrumen returns the most recent bunga akrual for an instrument.
	// Used to get bunga_last for maturity settlement.
	GetLastAkrualForInstrumen(ctx context.Context, instrumenID uuid.UUID) (*PendapatanAkrual, error)
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
		return nil, fmt.Errorf("akrualmaturity.repo.BeginTx: %w", err)
	}
	return tx, nil
}

// IsHoliday checks sys.holiday_calendar for a given date.
func (r *Repo) IsHoliday(ctx context.Context, tanggal time.Time) (bool, error) {
	const q = `
		SELECT COUNT(*) FROM sys.holiday_calendar
		WHERE tanggal = $1 AND deleted_at IS NULL`

	var count int
	if err := r.db.QueryRowContext(ctx, q, tanggal.Format("2006-01-02")).Scan(&count); err != nil {
		return false, fmt.Errorf("repo.IsHoliday: %w", err)
	}
	return count > 0, nil
}

// GetPeriodeByTanggal returns active periode_buku for a date.
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

// GetStaleDaysConfig reads AKRUAL_STAGING_STALE_DAYS from sys.config_param.
func (r *Repo) GetStaleDaysConfig(ctx context.Context) (int, error) {
	const q = `
		SELECT value FROM sys.config_param
		WHERE key = 'AKRUAL_STAGING_STALE_DAYS' AND deleted_at IS NULL
		LIMIT 1`

	var s string
	if err := r.db.QueryRowContext(ctx, q).Scan(&s); err != nil {
		if err == sql.ErrNoRows {
			return 30, nil // default
		}
		return 30, fmt.Errorf("repo.GetStaleDaysConfig: %w", err)
	}
	var n int
	if _, err := fmt.Sscan(s, &n); err != nil || n <= 0 {
		return 30, nil
	}
	return n, nil
}

// GetActiveAccruingInstrumens returns ACTIVE instrumen eligible for daily accrual (non-FVTPL).
func (r *Repo) GetActiveAccruingInstrumens(ctx context.Context) ([]*InstrumenAkrualInfo, error) {
	const q = `
		SELECT i.id, i.kode_instrumen, i.nama_instrumen,
		       COALESCE(i.status,'') AS status,
		       COALESCE(i.klasifikasi_psak71,'') AS klasifikasi_psak71,
		       i.klasifikasi_locked,
		       COALESCE(i.mata_uang,'IDR') AS mata_uang,
		       COALESCE(i.gross_carrying::text,'0') AS gross_carrying_str,
		       i.portofolio_id,
		       COALESCE(i.is_poci, FALSE) AS is_poci,
		       i.tanggal_jatuh_tempo
		FROM mst.instrumen i
		WHERE i.status = 'ACTIVE'
		  AND i.klasifikasi_psak71 != 'FVTPL'
		  AND i.deleted_at IS NULL
		  AND i.tenant_id = 'TUGURE'`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("repo.GetActiveAccruingInstrumens: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*InstrumenAkrualInfo
	for rows.Next() {
		inst, err := scanInstrumenAkrualInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.GetActiveAccruingInstrumens: scan: %w", err)
		}
		result = append(result, inst)
	}
	return result, rows.Err()
}

// GetActiveMaturityInstrumens returns ACTIVE instrumen with tanggal_jatuh_tempo = tanggal.
func (r *Repo) GetActiveMaturityInstrumens(ctx context.Context, tanggal time.Time) ([]*InstrumenAkrualInfo, error) {
	const q = `
		SELECT i.id, i.kode_instrumen, i.nama_instrumen,
		       COALESCE(i.status,'') AS status,
		       COALESCE(i.klasifikasi_psak71,'') AS klasifikasi_psak71,
		       i.klasifikasi_locked,
		       COALESCE(i.mata_uang,'IDR') AS mata_uang,
		       COALESCE(i.gross_carrying::text,'0') AS gross_carrying_str,
		       i.portofolio_id,
		       COALESCE(i.is_poci, FALSE) AS is_poci,
		       i.tanggal_jatuh_tempo
		FROM mst.instrumen i
		WHERE i.status = 'ACTIVE'
		  AND i.tanggal_jatuh_tempo = $1
		  AND i.deleted_at IS NULL
		  AND i.tenant_id = 'TUGURE'`

	rows, err := r.db.QueryContext(ctx, q, tanggal.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("repo.GetActiveMaturityInstrumens: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []*InstrumenAkrualInfo
	for rows.Next() {
		inst, err := scanInstrumenAkrualInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.GetActiveMaturityInstrumens: scan: %w", err)
		}
		result = append(result, inst)
	}
	return result, rows.Err()
}

// GetInstrumenInfo fetches minimal instrumen info by ID.
func (r *Repo) GetInstrumenInfo(ctx context.Context, id uuid.UUID) (*InstrumenAkrualInfo, error) {
	const q = `
		SELECT i.id, i.kode_instrumen, i.nama_instrumen,
		       COALESCE(i.status,'') AS status,
		       COALESCE(i.klasifikasi_psak71,'') AS klasifikasi_psak71,
		       i.klasifikasi_locked,
		       COALESCE(i.mata_uang,'IDR') AS mata_uang,
		       COALESCE(i.gross_carrying::text,'0') AS gross_carrying_str,
		       i.portofolio_id,
		       COALESCE(i.is_poci, FALSE) AS is_poci,
		       i.tanggal_jatuh_tempo
		FROM mst.instrumen i
		WHERE i.id = $1 AND i.deleted_at IS NULL
		LIMIT 1`

	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	if !rows.Next() {
		return nil, nil
	}
	inst, err := scanInstrumenAkrualInfo(rows)
	if err != nil {
		return nil, fmt.Errorf("repo.GetInstrumenInfo: scan: %w", err)
	}
	return inst, rows.Err()
}

// GetSealedECLForInstrumen returns latest sealed ECL result (M8 B1 pattern).
func (r *Repo) GetSealedECLForInstrumen(ctx context.Context, instrumenID uuid.UUID) (*ECLSealedResult, error) {
	const q = `
		SELECT crl.ecl_calc_run_id, crl.ecl_stage, COALESCE(crl.ecl_allowance::text,'0'), run.sealed_at
		FROM ecl.calc_result_line crl
		JOIN ecl.ecl_calc_run run ON run.id = crl.ecl_calc_run_id
		WHERE crl.instrumen_id = $1
		  AND run.sealed_at IS NOT NULL
		  AND run.deleted_at IS NULL
		  AND crl.deleted_at IS NULL
		ORDER BY run.sealed_at DESC, run.created_at DESC
		LIMIT 1`

	var runID uuid.UUID
	var stage int
	var eclStr string
	var sealedAt time.Time

	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&runID, &stage, &eclStr, &sealedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetSealedECLForInstrumen: %w", err)
	}

	ecl, e := decimal.NewFromString(eclStr)
	if e != nil {
		return nil, fmt.Errorf("repo.GetSealedECLForInstrumen: parse ecl '%s': %w", eclStr, e)
	}
	return &ECLSealedResult{
		ECLCalcRunID: runID,
		Stage:        stage,
		ECLAllowance: ecl,
		SealedAt:     sealedAt,
	}, nil
}

// GetAmortisasiSchedule returns active amortisasi schedule for instrumen at given date.
func (r *Repo) GetAmortisasiSchedule(ctx context.Context, instrumenID uuid.UUID, tanggal time.Time) (*AmortisasiScheduleRow, error) {
	const q = `
		SELECT schedule_version,
		       effective_from, effective_to,
		       COALESCE(eir_persen::text,'0') AS eir_str,
		       COALESCE(credit_adjusted_eir::text,'') AS ca_eir_str,
		       COALESCE(kupon_rate::text,'') AS kupon_str,
		       COALESCE(carrying_amount::text,'0') AS carrying_str,
		       COALESCE(premium_sisa::text,'0') AS premium_str,
		       COALESCE(diskon_sisa::text,'0') AS diskon_str,
		       COALESCE(amortisasi_harian::text,'0') AS amort_str,
		       COALESCE(is_poci, FALSE) AS is_poci
		FROM ecl.amortisasi_schedule
		WHERE instrumen_id = $1
		  AND $2 BETWEEN effective_from AND effective_to
		  AND deleted_at IS NULL
		ORDER BY schedule_version DESC
		LIMIT 1`

	var row AmortisasiScheduleRow
	row.InstrumenID = instrumenID

	var (
		eirStr, caEIRStr, kuponStr string
		carryingStr, premiumStr, diskonStr, amortStr string
		effectiveFrom, effectiveTo time.Time
	)

	err := r.db.QueryRowContext(ctx, q, instrumenID, tanggal.Format("2006-01-02")).Scan(
		&row.ScheduleVersion,
		&effectiveFrom, &effectiveTo,
		&eirStr, &caEIRStr, &kuponStr,
		&carryingStr, &premiumStr, &diskonStr, &amortStr,
		&row.IsPOCI,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetAmortisasiSchedule: %w", err)
	}

	row.EffectiveFrom = effectiveFrom
	row.EffectiveTo = effectiveTo

	parseD := func(s string) decimal.Decimal {
		if s == "" {
			return decimal.Zero
		}
		v, _ := decimal.NewFromString(s)
		return v
	}
	parseDPtr := func(s string) *decimal.Decimal {
		if s == "" {
			return nil
		}
		v, _ := decimal.NewFromString(s)
		return &v
	}

	row.EIRPersen = parseD(eirStr)
	row.CreditAdjustedEIR = parseDPtr(caEIRStr)
	row.KuponRate = parseDPtr(kuponStr)
	row.CarryingAmountAwal = parseD(carryingStr)
	row.PremiumSisa = parseD(premiumStr)
	row.DiskonSisa = parseD(diskonStr)
	row.AmortisasiHarian = parseD(amortStr)

	return &row, nil
}

// GetFXRateApproved returns approved FX rate for mata_uang on tanggal.
func (r *Repo) GetFXRateApproved(ctx context.Context, mataUang string, tanggal time.Time) (*FXRateApproved, error) {
	const q = `
		SELECT id, mata_uang, tanggal, COALESCE(rate_idr::text,'0')
		FROM sys.fx_rate
		WHERE mata_uang = $1
		  AND tanggal = $2
		  AND status = 'APPROVED'
		  AND deleted_at IS NULL
		LIMIT 1`

	var fx FXRateApproved
	var rateStr string
	err := r.db.QueryRowContext(ctx, q, mataUang, tanggal.Format("2006-01-02")).Scan(
		&fx.ID, &fx.MataUang, &fx.Tanggal, &rateStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetFXRateApproved: %w", err)
	}
	rate, e := decimal.NewFromString(rateStr)
	if e != nil {
		return nil, fmt.Errorf("repo.GetFXRateApproved: parse rate '%s': %w", rateStr, e)
	}
	fx.RateIDR = rate
	return &fx, nil
}

// IsDuplicateAkrual checks unique(instrumen_id, tanggal_akrual, jenis).
func (r *Repo) IsDuplicateAkrual(ctx context.Context, instrumenID uuid.UUID, tanggalAkrual time.Time, jenis AkrualJenis) (bool, error) {
	const q = `
		SELECT COUNT(*) FROM trx.pendapatan_akrual
		WHERE instrumen_id = $1
		  AND tanggal_akrual = $2
		  AND jenis = $3
		  AND deleted_at IS NULL`

	var count int
	if err := r.db.QueryRowContext(ctx, q, instrumenID, tanggalAkrual.Format("2006-01-02"), string(jenis)).Scan(&count); err != nil {
		return false, fmt.Errorf("repo.IsDuplicateAkrual: %w", err)
	}
	return count > 0, nil
}

// InsertAkrual inserts a pendapatan_akrual row in the given tx.
func (r *Repo) InsertAkrual(ctx context.Context, tx *sql.Tx, a *PendapatanAkrual) error {
	const q = `
		INSERT INTO trx.pendapatan_akrual (
			id, instrumen_id, tanggal_akrual, jenis, stage,
			carrying_basis, eir_persen, bunga_kotor, pph, bunga_bersih,
			fx_rate_id, mata_uang, klasifikasi_snapshot,
			ecl_run_id_used, stale_staging_flag,
			override_user_id, override_comment,
			jurnal_header_id, status, periode_bulanan_id,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19, $20,
			$21, $22, $23, $24,
			$25, $26
		)`

	_, err := tx.ExecContext(ctx, q,
		a.ID, a.InstrumenID, a.TanggalAkrual.Format("2006-01-02"), string(a.Jenis), a.Stage,
		a.CarryingBasisIDR.StringFixed(4), decPtrStr(a.EIRPersen, 8),
		a.BungaKotor.StringFixed(4), a.PPh.StringFixed(4), a.BungaBersih.StringFixed(4),
		a.FXRateID, a.MataUang, a.KlasifikasiSnapshot,
		a.ECLRunIDUsed, a.StaleStagingFlag,
		a.OverrideUserID, a.OverrideComment,
		a.JurnalHeaderID, string(a.Status), a.PeriodeBulananID,
		a.CreatedAt, a.CreatedBy, a.UpdatedAt, a.UpdatedBy,
		a.RowVersion, a.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertAkrual: %w", err)
	}
	return nil
}

// GetAkrualByID fetches a pendapatan_akrual row by ID.
func (r *Repo) GetAkrualByID(ctx context.Context, id uuid.UUID) (*PendapatanAkrual, error) {
	const q = `
		SELECT id, instrumen_id, tanggal_akrual, jenis, stage,
		       COALESCE(carrying_basis::text,'0'), COALESCE(eir_persen::text,''),
		       COALESCE(bunga_kotor::text,'0'), COALESCE(pph::text,'0'), COALESCE(bunga_bersih::text,'0'),
		       fx_rate_id, mata_uang, COALESCE(klasifikasi_snapshot,''),
		       ecl_run_id_used, stale_staging_flag,
		       override_user_id, override_comment,
		       jurnal_header_id, status, periode_bulanan_id,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.pendapatan_akrual
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var (
		a            PendapatanAkrual
		carryingStr  string
		eirStr       string
		kotorStr     string
		pphStr       string
		bersihStr    string
	)

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&a.ID, &a.InstrumenID, &a.TanggalAkrual, &a.Jenis, &a.Stage,
		&carryingStr, &eirStr,
		&kotorStr, &pphStr, &bersihStr,
		&a.FXRateID, &a.MataUang, &a.KlasifikasiSnapshot,
		&a.ECLRunIDUsed, &a.StaleStagingFlag,
		&a.OverrideUserID, &a.OverrideComment,
		&a.JurnalHeaderID, &a.Status, &a.PeriodeBulananID,
		&a.CreatedAt, &a.CreatedBy, &a.UpdatedAt, &a.UpdatedBy,
		&a.DeletedAt, &a.DeletedBy, &a.RowVersion, &a.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetAkrualByID: %w", err)
	}

	if v, e := decimal.NewFromString(carryingStr); e == nil {
		a.CarryingBasisIDR = v
	}
	if eirStr != "" {
		if v, e := decimal.NewFromString(eirStr); e == nil {
			a.EIRPersen = &v
		}
	}
	if v, e := decimal.NewFromString(kotorStr); e == nil {
		a.BungaKotor = v
	}
	if v, e := decimal.NewFromString(pphStr); e == nil {
		a.PPh = v
	}
	if v, e := decimal.NewFromString(bersihStr); e == nil {
		a.BungaBersih = v
	}
	return &a, nil
}

// UpdateAkrualStatus updates akrual status + jurnal + override fields.
func (r *Repo) UpdateAkrualStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status AkrualStatus, jurnalHeaderID *uuid.UUID, overrideUserID *uuid.UUID, overrideComment *string, rowVersion int64, updatedBy uuid.UUID) error {
	const q = `
		UPDATE trx.pendapatan_akrual SET
			status            = $1,
			jurnal_header_id  = $2,
			override_user_id  = $3,
			override_comment  = $4,
			updated_by        = $5,
			updated_at        = NOW()
		WHERE id = $6
		  AND row_version = $7
		  AND deleted_at IS NULL`

	res, err := tx.ExecContext(ctx, q,
		string(status), jurnalHeaderID, overrideUserID, overrideComment,
		updatedBy, id, rowVersion,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateAkrualStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repo.UpdateAkrualStatus: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("repo.UpdateAkrualStatus: optimistic lock conflict for akrual %s", id)
	}
	return nil
}

// ListAkrual returns paginated pendapatan_akrual rows.
func (r *Repo) ListAkrual(ctx context.Context, _ listquery.Query, cursor string, limit int) ([]*PendapatanAkrual, bool, int, error) {
	var cursorTime time.Time
	var cursorID uuid.UUID
	if cursor != "" {
		ct, cid, err := decodeCursor(cursor)
		if err != nil {
			return nil, false, 0, fmt.Errorf("repo.ListAkrual: invalid cursor: %w", err)
		}
		cursorTime, cursorID = ct, cid
	}

	const baseQ = `
		SELECT id, instrumen_id, tanggal_akrual, jenis, stage,
		       COALESCE(carrying_basis::text,'0'), COALESCE(eir_persen::text,''),
		       COALESCE(bunga_kotor::text,'0'), COALESCE(pph::text,'0'), COALESCE(bunga_bersih::text,'0'),
		       fx_rate_id, mata_uang, COALESCE(klasifikasi_snapshot,''),
		       ecl_run_id_used, stale_staging_flag,
		       jurnal_header_id, status,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM trx.pendapatan_akrual
		WHERE deleted_at IS NULL AND tenant_id = $1`

	const noCursorSuffix = ` ORDER BY created_at DESC, id DESC LIMIT $2`
	const cursorSuffix = ` AND (created_at, id) < ($3, $4) ORDER BY created_at DESC, id DESC LIMIT $2`

	var (
		sqlRows *sql.Rows
		err     error
	)
	if cursor == "" {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+noCursorSuffix, "TUGURE", limit+1)
	} else {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+cursorSuffix, "TUGURE", limit+1, cursorTime, cursorID)
	}
	if err != nil {
		return nil, false, 0, fmt.Errorf("repo.ListAkrual: %w", err)
	}
	defer sqlRows.Close() //nolint:errcheck

	var result []*PendapatanAkrual
	for sqlRows.Next() {
		var (
			a            PendapatanAkrual
			carryingStr  string
			eirStr       string
			kotorStr, pphStr, bersihStr string
		)
		if err := sqlRows.Scan(
			&a.ID, &a.InstrumenID, &a.TanggalAkrual, &a.Jenis, &a.Stage,
			&carryingStr, &eirStr,
			&kotorStr, &pphStr, &bersihStr,
			&a.FXRateID, &a.MataUang, &a.KlasifikasiSnapshot,
			&a.ECLRunIDUsed, &a.StaleStagingFlag,
			&a.JurnalHeaderID, &a.Status,
			&a.CreatedAt, &a.CreatedBy, &a.UpdatedAt, &a.UpdatedBy,
			&a.RowVersion, &a.TenantID,
		); err != nil {
			return nil, false, 0, fmt.Errorf("repo.ListAkrual: scan: %w", err)
		}
		if v, e := decimal.NewFromString(carryingStr); e == nil {
			a.CarryingBasisIDR = v
		}
		if eirStr != "" {
			if v, e := decimal.NewFromString(eirStr); e == nil {
				a.EIRPersen = &v
			}
		}
		if v, e := decimal.NewFromString(kotorStr); e == nil {
			a.BungaKotor = v
		}
		if v, e := decimal.NewFromString(pphStr); e == nil {
			a.PPh = v
		}
		if v, e := decimal.NewFromString(bersihStr); e == nil {
			a.BungaBersih = v
		}
		result = append(result, &a)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, false, 0, fmt.Errorf("repo.ListAkrual: iterate: %w", err)
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, len(result), nil
}

// GetMTDYTDSummary returns MTD/YTD aggregate.
func (r *Repo) GetMTDYTDSummary(ctx context.Context, instrumenID *uuid.UUID, portofolioID *uuid.UUID, year, month int) (*AkrualDashboard, error) {
	const q = `
		SELECT
			COALESCE(SUM(CASE WHEN EXTRACT(MONTH FROM tanggal_akrual) = $3 AND EXTRACT(YEAR FROM tanggal_akrual) = $2 THEN bunga_bersih ELSE 0 END)::text, '0') AS mtd,
			COALESCE(SUM(CASE WHEN EXTRACT(YEAR FROM tanggal_akrual) = $2 THEN bunga_bersih ELSE 0 END)::text, '0') AS ytd
		FROM trx.pendapatan_akrual
		WHERE deleted_at IS NULL
		  AND tenant_id = 'TUGURE'
		  AND ($4::uuid IS NULL OR instrumen_id = $4)
		  AND status IN ('AUTO_POSTED', 'POSTED')`

	var mtdStr, ytdStr string
	if err := r.db.QueryRowContext(ctx, q, year, year, month, instrumenID).Scan(&mtdStr, &ytdStr); err != nil {
		return nil, fmt.Errorf("repo.GetMTDYTDSummary: %w", err)
	}

	mtd, _ := decimal.NewFromString(mtdStr)
	ytd, _ := decimal.NewFromString(ytdStr)

	dash := &AkrualDashboard{
		Year:         year,
		Month:        month,
		AkrualMtdIdr: mtd.StringFixed(4),
		AkrualYtdIdr: ytd.StringFixed(4),
	}
	if instrumenID != nil {
		s := instrumenID.String()
		dash.InstrumenID = &s
	}
	if portofolioID != nil {
		s := portofolioID.String()
		dash.PortofolioID = &s
	}
	return dash, nil
}

// InsertJatuhTempo inserts a jatuh_tempo row in the given tx.
func (r *Repo) InsertJatuhTempo(ctx context.Context, tx *sql.Tx, jt *JatuhTempo) error {
	const q = `
		INSERT INTO trx.jatuh_tempo (
			id, instrumen_id, tanggal_jatuh_tempo, jenis,
			pokok_returned, bunga_returned, pph, proceeds,
			fx_rate_id, klasifikasi_snapshot, jurnal_header_id, status, error_message,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19
		)`

	_, err := tx.ExecContext(ctx, q,
		jt.ID, jt.InstrumenID, jt.TanggalJatuhTempo.Format("2006-01-02"), jt.Jenis,
		jt.PokokReturned.StringFixed(4), jt.BungaReturned.StringFixed(4),
		jt.PPh.StringFixed(4), jt.Proceeds.StringFixed(4),
		jt.FXRateID, jt.KlasifikasiSnapshot, jt.JurnalHeaderID, string(jt.Status), jt.ErrorMessage,
		jt.CreatedAt, jt.CreatedBy, jt.UpdatedAt, jt.UpdatedBy, jt.RowVersion, jt.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertJatuhTempo: %w", err)
	}
	return nil
}

// UpdateJatuhTempoStatus updates trx.jatuh_tempo status.
func (r *Repo) UpdateJatuhTempoStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggal time.Time, status JatuhTempoStatus, jurnalHeaderID *uuid.UUID, errorMessage *string, rowVersion int64, updatedBy uuid.UUID) error {
	// Note: partitioned by tanggal_jatuh_tempo — must include partition key in WHERE for efficiency.
	const q = `
		UPDATE trx.jatuh_tempo SET
			status           = $1,
			jurnal_header_id = $2,
			error_message    = $3,
			updated_by       = $4,
			updated_at       = NOW()
		WHERE id = $5
		  AND tanggal_jatuh_tempo = $6
		  AND row_version = $7
		  AND deleted_at IS NULL`

	res, err := tx.ExecContext(ctx, q,
		string(status), jurnalHeaderID, errorMessage,
		updatedBy, id, tanggal.Format("2006-01-02"), rowVersion,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateJatuhTempoStatus: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("repo.UpdateJatuhTempoStatus: optimistic lock conflict for jatuh_tempo %s", id)
	}
	return nil
}

// ListJatuhTempo returns paginated jatuh_tempo rows.
func (r *Repo) ListJatuhTempo(ctx context.Context, _ listquery.Query, cursor string, limit int) ([]*JatuhTempo, bool, int, error) {
	var cursorTime time.Time
	var cursorID uuid.UUID
	if cursor != "" {
		ct, cid, err := decodeCursor(cursor)
		if err != nil {
			return nil, false, 0, fmt.Errorf("repo.ListJatuhTempo: cursor: %w", err)
		}
		cursorTime, cursorID = ct, cid
	}

	const baseQ = `
		SELECT id, instrumen_id, tanggal_jatuh_tempo, jenis,
		       COALESCE(pokok_returned::text,'0'), COALESCE(bunga_returned::text,'0'),
		       COALESCE(pph::text,'0'), COALESCE(proceeds::text,'0'),
		       fx_rate_id, COALESCE(klasifikasi_snapshot,''), jurnal_header_id, status, error_message,
		       created_at, created_by, updated_at, updated_by, row_version, tenant_id
		FROM trx.jatuh_tempo
		WHERE deleted_at IS NULL AND tenant_id = $1`

	const noCursor = ` ORDER BY created_at DESC, id DESC LIMIT $2`
	const withCursor = ` AND (created_at, id) < ($3, $4) ORDER BY created_at DESC, id DESC LIMIT $2`

	var (
		sqlRows *sql.Rows
		err     error
	)
	if cursor == "" {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+noCursor, "TUGURE", limit+1)
	} else {
		sqlRows, err = r.db.QueryContext(ctx, baseQ+withCursor, "TUGURE", limit+1, cursorTime, cursorID)
	}
	if err != nil {
		return nil, false, 0, fmt.Errorf("repo.ListJatuhTempo: %w", err)
	}
	defer sqlRows.Close() //nolint:errcheck

	var result []*JatuhTempo
	for sqlRows.Next() {
		var (
			jt                  JatuhTempo
			pokokStr, bungaStr  string
			pphStr, proceedsStr string
		)
		if err := sqlRows.Scan(
			&jt.ID, &jt.InstrumenID, &jt.TanggalJatuhTempo, &jt.Jenis,
			&pokokStr, &bungaStr, &pphStr, &proceedsStr,
			&jt.FXRateID, &jt.KlasifikasiSnapshot, &jt.JurnalHeaderID, &jt.Status, &jt.ErrorMessage,
			&jt.CreatedAt, &jt.CreatedBy, &jt.UpdatedAt, &jt.UpdatedBy, &jt.RowVersion, &jt.TenantID,
		); err != nil {
			return nil, false, 0, fmt.Errorf("repo.ListJatuhTempo: scan: %w", err)
		}
		if v, e := decimal.NewFromString(pokokStr); e == nil {
			jt.PokokReturned = v
		}
		if v, e := decimal.NewFromString(bungaStr); e == nil {
			jt.BungaReturned = v
		}
		if v, e := decimal.NewFromString(pphStr); e == nil {
			jt.PPh = v
		}
		if v, e := decimal.NewFromString(proceedsStr); e == nil {
			jt.Proceeds = v
		}
		result = append(result, &jt)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, false, 0, fmt.Errorf("repo.ListJatuhTempo: iterate: %w", err)
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, len(result), nil
}

// InsertDividen inserts a dividen row in the given tx.
func (r *Repo) InsertDividen(ctx context.Context, tx *sql.Tx, d *Dividen) error {
	const q = `
		INSERT INTO trx.dividen (
			id, instrumen_id, tanggal_terima, tanggal_cum_date,
			jumlah_kotor, pph_dividen, jumlah_bersih,
			klasifikasi_snapshot, treatment, is_reksadana,
			status, maker_id, approver_id,
			approve_comment, reject_reason, signature_method, approved_at,
			jurnal_header_id,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13,
			$14, $15, $16, $17,
			$18,
			$19, $20, $21, $22, $23, $24
		)`

	_, err := tx.ExecContext(ctx, q,
		d.ID, d.InstrumenID, d.TanggalTerima.Format("2006-01-02"), d.TanggalCumDate,
		d.JumlahKotor.StringFixed(4), d.PPHDividen.StringFixed(4), d.JumlahBersih.StringFixed(4),
		d.KlasifikasiSnapshot, d.Treatment, d.IsReksadana,
		string(d.Status), d.MakerID, d.ApproverID,
		d.ApproveComment, d.RejectReason, d.SignatureMethod, d.ApprovedAt,
		d.JurnalHeaderID,
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy, d.RowVersion, d.TenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertDividen: %w", err)
	}
	return nil
}

// GetDividenByID fetches a dividen row by ID.
func (r *Repo) GetDividenByID(ctx context.Context, id uuid.UUID) (*Dividen, error) {
	const q = `
		SELECT id, instrumen_id, tanggal_terima, tanggal_cum_date,
		       COALESCE(jumlah_kotor::text,'0'), COALESCE(pph_dividen::text,'0'), COALESCE(jumlah_bersih::text,'0'),
		       COALESCE(klasifikasi_snapshot,''), treatment, is_reksadana,
		       status, maker_id, approver_id,
		       approve_comment, reject_reason, signature_method, approved_at,
		       jurnal_header_id,
		       created_at, created_by, updated_at, updated_by,
		       deleted_at, deleted_by, row_version, tenant_id
		FROM trx.dividen
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var (
		d          Dividen
		kotorStr, pphStr, bersihStr string
	)
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&d.ID, &d.InstrumenID, &d.TanggalTerima, &d.TanggalCumDate,
		&kotorStr, &pphStr, &bersihStr,
		&d.KlasifikasiSnapshot, &d.Treatment, &d.IsReksadana,
		&d.Status, &d.MakerID, &d.ApproverID,
		&d.ApproveComment, &d.RejectReason, &d.SignatureMethod, &d.ApprovedAt,
		&d.JurnalHeaderID,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
		&d.DeletedAt, &d.DeletedBy, &d.RowVersion, &d.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetDividenByID: %w", err)
	}
	if v, e := decimal.NewFromString(kotorStr); e == nil {
		d.JumlahKotor = v
	}
	if v, e := decimal.NewFromString(pphStr); e == nil {
		d.PPHDividen = v
	}
	if v, e := decimal.NewFromString(bersihStr); e == nil {
		d.JumlahBersih = v
	}
	return &d, nil
}

// UpdateDividenStatus updates dividen status.
func (r *Repo) UpdateDividenStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, tanggal time.Time, status DividenStatus, approverID *uuid.UUID, comment *string, rejectReason *string, sigMethod *string, approvedAt *time.Time, jurnalHeaderID *uuid.UUID, rowVersion int64, updatedBy uuid.UUID) error {
	const q = `
		UPDATE trx.dividen SET
			status           = $1,
			approver_id      = $2,
			approve_comment  = $3,
			reject_reason    = $4,
			signature_method = $5,
			approved_at      = $6,
			jurnal_header_id = $7,
			updated_by       = $8,
			updated_at       = NOW()
		WHERE id = $9
		  AND tanggal_terima = $10
		  AND row_version = $11
		  AND deleted_at IS NULL`

	res, err := tx.ExecContext(ctx, q,
		string(status), approverID, comment, rejectReason, sigMethod, approvedAt, jurnalHeaderID,
		updatedBy, id, tanggal.Format("2006-01-02"), rowVersion,
	)
	if err != nil {
		return fmt.Errorf("repo.UpdateDividenStatus: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("repo.UpdateDividenStatus: optimistic lock conflict for dividen %s", id)
	}
	return nil
}

// InsertDLQ inserts a DLQ entry for a failed cron item.
func (r *Repo) InsertDLQ(ctx context.Context, jobType string, instrumenID uuid.UUID, errorCode string, errorDetail string) error {
	const q = `
		INSERT INTO sys.dlq (job_type, instrumen_id, error_code, error_detail, retry_count, max_retry, created_at)
		VALUES ($1, $2, $3, $4, 0, 3, NOW())
		ON CONFLICT DO NOTHING`

	_, err := r.db.ExecContext(ctx, q, jobType, instrumenID, errorCode, errorDetail)
	if err != nil {
		return fmt.Errorf("repo.InsertDLQ: %w", err)
	}
	return nil
}

// GetLastAkrualForInstrumen returns the most recent bunga akrual for instrument (for maturity bunga_last).
func (r *Repo) GetLastAkrualForInstrumen(ctx context.Context, instrumenID uuid.UUID) (*PendapatanAkrual, error) {
	const q = `
		SELECT id, instrumen_id, tanggal_akrual,
		       COALESCE(bunga_kotor::text,'0'), COALESCE(pph::text,'0'), COALESCE(bunga_bersih::text,'0'),
		       status, created_at, row_version, tenant_id
		FROM trx.pendapatan_akrual
		WHERE instrumen_id = $1
		  AND jenis = 'BUNGA'
		  AND status IN ('AUTO_POSTED','POSTED')
		  AND deleted_at IS NULL
		ORDER BY tanggal_akrual DESC, created_at DESC
		LIMIT 1`

	var (
		a                        PendapatanAkrual
		kotorStr, pphStr, bersihStr string
	)
	err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(
		&a.ID, &a.InstrumenID, &a.TanggalAkrual,
		&kotorStr, &pphStr, &bersihStr,
		&a.Status, &a.CreatedAt, &a.RowVersion, &a.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetLastAkrualForInstrumen: %w", err)
	}
	if v, e := decimal.NewFromString(kotorStr); e == nil {
		a.BungaKotor = v
	}
	if v, e := decimal.NewFromString(pphStr); e == nil {
		a.PPh = v
	}
	if v, e := decimal.NewFromString(bersihStr); e == nil {
		a.BungaBersih = v
	}
	return &a, nil
}

// ─── cursor helpers ───────────────────────────────────────────────────────────

func decodeCursor(cursor string) (time.Time, uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("cursor format invalid")
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

var _ = encodeCursor

// ─── scan helpers ─────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstrumenAkrualInfo(row rowScanner) (*InstrumenAkrualInfo, error) {
	var (
		inst           InstrumenAkrualInfo
		grossCarryStr  string
	)
	if err := row.Scan(
		&inst.ID, &inst.KodeInstrumen, &inst.NamaInstrumen,
		&inst.Status, &inst.KlasifikasiPSAK71, &inst.KlasifikasiLocked,
		&inst.MataUang, &grossCarryStr,
		&inst.PortofolioID, &inst.IsPOCI, &inst.TanggalJatuhTempo,
	); err != nil {
		return nil, err
	}
	if v, e := decimal.NewFromString(grossCarryStr); e == nil {
		inst.GrossCarryingIDR = v
	}
	return &inst, nil
}

// decPtrStr returns nil if d is nil, else StringFixed(scale).
func decPtrStr(d *decimal.Decimal, scale int32) interface{} {
	if d == nil {
		return nil
	}
	return d.StringFixed(scale)
}
