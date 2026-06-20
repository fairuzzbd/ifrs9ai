// Package akrualmaturity implements trx.pendapatan_akrual, trx.jatuh_tempo, and trx.dividen
// for P5-M9: Jatuh Tempo + Pendapatan Akrual Harian.
//
// Domain rules (DEC-010/013/016/017/018/021):
//   - Stage 3 akrual = (Gross - ECL_sealed) × EIR / 365 per PSAK 71 §5.4.1(b)
//   - ECL must come from SEALED calc run (never draft)
//   - POCI: credit-adjusted EIR from ecl.amortisasi_schedule is_poci=TRUE version
//   - All amounts: shopspring/decimal (never float64) — DEC-016
//   - EIR immutability: NEVER UPDATE ecl.amortisasi_schedule rows — DEC-013
//   - Audit in-tx for all mutations — DEC-018
//   - Idempotency via unique(instrumen_id, tanggal_akrual, jenis) — DEC-021
//   - SoD dividen: approver_id ≠ maker_id — DEC-017
//   - Cron skip on holiday (sys.holiday_calendar)
//   - Periode lock: cannot post to CLOSED periode
package akrualmaturity

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes (P5-M9 new codes) ───────────────────────────────────────────

const (
	// CodeMaturityInstrumenNotActive — instrumen.status ≠ 'ACTIVE' during maturity cron. HTTP 422 (DLQ).
	CodeMaturityInstrumenNotActive = "MATURITY_INSTRUMEN_NOT_ACTIVE"

	// CodeAkrualStagingStale — ECL sealed run > AKRUAL_STAGING_STALE_DAYS old for Stage 3. HTTP 200 (warning).
	CodeAkrualStagingStale = "AKRUAL_STAGING_STALE"

	// CodeAkrualFXRateMissing — FX rate APPROVED not available for mata_uang + tanggal_akrual. HTTP 422 (DLQ).
	CodeAkrualFXRateMissing = "AKRUAL_FX_RATE_MISSING"

	// CodeAkrualPeriodeLocked — mst.periode_buku.status_periode = 'CLOSED'. HTTP 423 (DLQ).
	CodeAkrualPeriodeLocked = "AKRUAL_PERIODE_LOCKED"

	// CodeAkrualDuplicate — unique constraint (instrumen_id, tanggal_akrual, jenis) violation. HTTP 409 (DLQ).
	CodeAkrualDuplicate = "AKRUAL_DUPLICATE"

	// CodeAkrualEIRNotFound — no active ecl.amortisasi_schedule for instrumen. HTTP 422 (DLQ).
	CodeAkrualEIRNotFound = "AKRUAL_EIR_NOT_FOUND"

	// CodeDividenValidationFailed — gross_dividen ≤ 0 or required field missing. HTTP 422.
	CodeDividenValidationFailed = "DIVIDEN_VALIDATION_FAILED"
)

// ─── Akrual status enum ───────────────────────────────────────────────────────

// AkrualStatus represents trx.pendapatan_akrual.status.
type AkrualStatus string

const (
	AkrualAutoPosted        AkrualStatus = "AUTO_POSTED"
	AkrualPendingStaleReview AkrualStatus = "PENDING_STALE_REVIEW"
	AkrualOverrideApproved  AkrualStatus = "OVERRIDE_APPROVED"
	AkrualPosted            AkrualStatus = "POSTED"
	AkrualSkipped           AkrualStatus = "SKIPPED"
)

// CanOverride returns true if an override is allowed from this status.
func (s AkrualStatus) CanOverride() bool { return s == AkrualPendingStaleReview }

// ─── Jatuh tempo status enum ──────────────────────────────────────────────────

// JatuhTempoStatus represents trx.jatuh_tempo.status.
type JatuhTempoStatus string

const (
	JatuhTempoPending  JatuhTempoStatus = "PENDING"
	JatuhTempoSettled  JatuhTempoStatus = "SETTLED"
	JatuhTempoFailed   JatuhTempoStatus = "FAILED"
	JatuhTempoSkipped  JatuhTempoStatus = "SKIPPED"
)

// ─── Dividen status enum ──────────────────────────────────────────────────────

// DividenStatus represents trx.dividen.status.
type DividenStatus string

const (
	DividenPendingApproval DividenStatus = "PENDING_APPROVAL"
	DividenApproved        DividenStatus = "APPROVED"
	DividenPosted          DividenStatus = "POSTED"
	DividenRejected        DividenStatus = "REJECTED"
)

// CanApprove returns true if approve is allowed from this status.
func (s DividenStatus) CanApprove() bool { return s == DividenPendingApproval }

// ─── Jenis akrual enum ────────────────────────────────────────────────────────

// AkrualJenis represents the type of accrual.
type AkrualJenis string

const (
	JenisBunga             AkrualJenis = "BUNGA"
	JenisDividen           AkrualJenis = "DIVIDEN"
	JenisAmortisasiPremium AkrualJenis = "AMORTISASI_PREMIUM"
	JenisAmortisasiDiskon  AkrualJenis = "AMORTISASI_DISKON"
	JenisDistribusiRD      AkrualJenis = "DISTRIBUSI_REKSADANA"
)

// ─── Carrying basis ───────────────────────────────────────────────────────────

// CarryingBasis indicates whether carrying is gross or net.
type CarryingBasis string

const (
	BasisGross       CarryingBasis = "GROSS"
	BasisNetCarrying CarryingBasis = "NET_CARRYING"
)

// ─── Domain entities ──────────────────────────────────────────────────────────

// PendapatanAkrual is the domain entity for trx.pendapatan_akrual.
type PendapatanAkrual struct {
	ID                 uuid.UUID       `db:"id"`
	InstrumenID        uuid.UUID       `db:"instrumen_id"`
	TanggalAkrual      time.Time       `db:"tanggal_akrual"` // DATE
	Jenis              AkrualJenis     `db:"jenis"`
	Stage              *int            `db:"stage"`
	CarryingBasisIDR   decimal.Decimal `db:"carrying_basis"`
	EIRPersen          *decimal.Decimal `db:"eir_persen"`
	BungaKotor         decimal.Decimal `db:"bunga_kotor"`
	PPh                decimal.Decimal `db:"pph"`
	BungaBersih        decimal.Decimal `db:"bunga_bersih"`
	FXRateID           *uuid.UUID      `db:"fx_rate_id"`
	MataUang           string          `db:"mata_uang"`
	KlasifikasiSnapshot string         `db:"klasifikasi_snapshot"`
	ECLRunIDUsed       *uuid.UUID      `db:"ecl_run_id_used"`
	StaleStagingFlag   bool            `db:"stale_staging_flag"`
	OverrideUserID     *uuid.UUID      `db:"override_user_id"`
	OverrideComment    *string         `db:"override_comment"`
	JurnalHeaderID     *uuid.UUID      `db:"jurnal_header_id"`
	Status             AkrualStatus    `db:"status"`
	PeriodeBulananID   *uuid.UUID      `db:"periode_bulanan_id"`
	// Audit
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// JatuhTempo is the domain entity for trx.jatuh_tempo.
type JatuhTempo struct {
	ID                  uuid.UUID        `db:"id"`
	InstrumenID         uuid.UUID        `db:"instrumen_id"`
	TanggalJatuhTempo   time.Time        `db:"tanggal_jatuh_tempo"` // DATE
	Jenis               string           `db:"jenis"`
	PokokReturned       decimal.Decimal  `db:"pokok_returned"`
	BungaReturned       decimal.Decimal  `db:"bunga_returned"`
	PPh                 decimal.Decimal  `db:"pph"`
	Proceeds            decimal.Decimal  `db:"proceeds"`
	FXRateID            *uuid.UUID       `db:"fx_rate_id"`
	KlasifikasiSnapshot string           `db:"klasifikasi_snapshot"`
	JurnalHeaderID      *uuid.UUID       `db:"jurnal_header_id"`
	Status              JatuhTempoStatus `db:"status"`
	ErrorMessage        *string          `db:"error_message"`
	// Audit
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// Dividen is the domain entity for trx.dividen.
type Dividen struct {
	ID                  uuid.UUID     `db:"id"`
	InstrumenID         uuid.UUID     `db:"instrumen_id"`
	TanggalTerima       time.Time     `db:"tanggal_terima"` // DATE
	TanggalCumDate      *time.Time    `db:"tanggal_cum_date"`
	JumlahKotor         decimal.Decimal `db:"jumlah_kotor"`
	PPHDividen          decimal.Decimal `db:"pph_dividen"`
	JumlahBersih        decimal.Decimal `db:"jumlah_bersih"`
	KlasifikasiSnapshot string        `db:"klasifikasi_snapshot"`
	Treatment           string        `db:"treatment"`
	IsReksadana         bool          `db:"is_reksadana"`
	Status              DividenStatus `db:"status"`
	MakerID             uuid.UUID     `db:"maker_id"`
	ApproverID          *uuid.UUID    `db:"approver_id"`
	ApproveComment      *string       `db:"approve_comment"`
	RejectReason        *string       `db:"reject_reason"`
	SignatureMethod      *string       `db:"signature_method"`
	SignatureHashMeta   *json.RawMessage `db:"signature_hash_meta"`
	ApprovedAt          *time.Time    `db:"approved_at"`
	JurnalHeaderID      *uuid.UUID    `db:"jurnal_header_id"`
	// Audit
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// ─── InstrumenInfo ────────────────────────────────────────────────────────────

// InstrumenAkrualInfo holds fields needed by akrualmaturity service from mst.instrumen.
type InstrumenAkrualInfo struct {
	ID                  uuid.UUID
	KodeInstrumen       string
	NamaInstrumen       string
	Status              string
	KlasifikasiPSAK71   string
	KlasifikasiLocked   bool
	MataUang            string
	GrossCarryingIDR    decimal.Decimal
	EIRPersen           decimal.Decimal // annual EIR from amortisasi_schedule
	Stage               int             // current stage from ecl.staging_history
	IsPOCI              bool
	TanggalJatuhTempo   *time.Time
	PortofolioID        uuid.UUID
}

// ─── Schedule row ────────────────────────────────────────────────────────────

// AmortisasiScheduleRow holds the data for one amortisasi entry.
type AmortisasiScheduleRow struct {
	InstrumenID         uuid.UUID
	ScheduleVersion     int
	EffectiveFrom       time.Time
	EffectiveTo         time.Time
	EIRPersen           decimal.Decimal
	CreditAdjustedEIR   *decimal.Decimal // non-nil for POCI
	KuponRate           *decimal.Decimal
	CarryingAmountAwal  decimal.Decimal
	PremiumSisa         decimal.Decimal
	DiskonSisa          decimal.Decimal
	AmortisasiHarian    decimal.Decimal
	IsPOCI              bool
}

// ─── ECL sealed result ────────────────────────────────────────────────────────

// ECLSealedResult holds ECL from sealed calc run for a given instrument.
type ECLSealedResult struct {
	ECLCalcRunID uuid.UUID
	Stage        int
	ECLAllowance decimal.Decimal
	SealedAt     time.Time
}

// ─── FX rate ─────────────────────────────────────────────────────────────────

// FXRateApproved holds FX rate for a currency on a given date.
type FXRateApproved struct {
	ID       uuid.UUID
	MataUang string
	Tanggal  time.Time
	RateIDR  decimal.Decimal
}

// ─── Request / Response types ─────────────────────────────────────────────────

// OverrideStaleRequest is the body for POST /transaksi/akrual/{id}/override-stale.
type OverrideStaleRequest struct {
	Reason          string `json:"reason"          binding:"required,min=30"`
	SignatureMethod  string `json:"signatureMethod" binding:"required"`
}

// CreateDividenRequest is the body for POST /transaksi/dividen.
type CreateDividenRequest struct {
	InstrumenID     uuid.UUID       `json:"instrumenId"     binding:"required"`
	JumlahKotor     decimal.Decimal `json:"jumlahKotor"`
	TanggalTerima   string          `json:"tanggalTerima"   binding:"required"`
	TanggalCumDate  *string         `json:"tanggalCumDate"`
	IsReksadana     bool            `json:"isReksadana"`
}

// ApproveDividenRequest is the body for POST /transaksi/dividen/{id}/approve.
type ApproveDividenRequest struct {
	Comment         string `json:"comment"         binding:"required"`
	SignatureMethod  string `json:"signatureMethod" binding:"required"`
}

// RejectDividenRequest is the body for POST /transaksi/dividen/{id}/reject.
type RejectDividenRequest struct {
	Reason         string `json:"reason"          binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// ─── Response types ───────────────────────────────────────────────────────────

// AkrualListItem is one row in GET /transaksi/akrual list response.
type AkrualListItem struct {
	ID                  string  `json:"id"`
	InstrumenID         string  `json:"instrumenId"`
	InstrumenKode       string  `json:"instrumenKode"`
	KlasifikasiSnapshot string  `json:"klasifikasiSnapshot"`
	TanggalAkrual       string  `json:"tanggalAkrual"`
	Jenis               string  `json:"jenis"`
	Stage               *int    `json:"stage"`
	CarryingBasis       string  `json:"carryingBasis"` // "GROSS" or "NET_CARRYING"
	CarryingIdr         string  `json:"carryingIdr"`
	EirPersen           *string `json:"eirPersen"`
	BungaKotor          string  `json:"bungaKotor"`
	Pph                 string  `json:"pph"`
	BungaBersih         string  `json:"bungaBersih"`
	MataUang            string  `json:"mataUang"`
	FxRateId            *string `json:"fxRateId"`
	StaleStagingFlag    bool    `json:"staleStagingFlag"`
	EclRunIdUsed        *string `json:"eclRunIdUsed"`
	Status              string  `json:"status"`
	JurnalHeaderId      *string `json:"jurnalHeaderId"`
	CreatedAt           string  `json:"createdAt"`
}

// AkrualDashboard is the response for GET /transaksi/akrual/dashboard.
type AkrualDashboard struct {
	InstrumenID     *string              `json:"instrumenId"`
	PortofolioID    *string              `json:"portofolioId"`
	Year            int                  `json:"year"`
	Month           int                  `json:"month"`
	AkrualMtdIdr    string               `json:"akrualMtdIdr"`
	AkrualYtdIdr    string               `json:"akrualYtdIdr"`
	StageSaatIni    *int                 `json:"stageSaatIni"`
	StagingSource   *string              `json:"stagingSource"`
	EclRunSealedAt  *string              `json:"eclRunSealedAt"`
	StaleCount      int                  `json:"staleCount"`
	Breakdown       []AkrualBreakdown    `json:"breakdown"`
}

// AkrualBreakdown is one jenis row in dashboard.
type AkrualBreakdown struct {
	Jenis  string `json:"jenis"`
	MtdIdr string `json:"mtdIdr"`
	YtdIdr string `json:"ytdIdr"`
}

// JatuhTempoListItem is one row in GET /transaksi/jatuh-tempo list response.
type JatuhTempoListItem struct {
	ID                  string  `json:"id"`
	InstrumenID         string  `json:"instrumenId"`
	InstrumenKode       string  `json:"instrumenKode"`
	TanggalJatuhTempo   string  `json:"tanggalJatuhTempo"`
	Jenis               string  `json:"jenis"`
	PokokIdr            string  `json:"pokokIdr"`
	BungaLastIdr        string  `json:"bungaLastIdr"`
	PphIdr              string  `json:"pphIdr"`
	NetKasIdr           string  `json:"netKasIdr"`
	KlasifikasiSnapshot string  `json:"klasifikasiSnapshot"`
	Status              string  `json:"status"`
	ErrorMessage        *string `json:"errorMessage"`
	JurnalHeaderId      *string `json:"jurnalHeaderId"`
	CreatedAt           string  `json:"createdAt"`
}

// ─── Cron result ─────────────────────────────────────────────────────────────

// CronBatchResult summarizes results of a cron batch run.
type CronBatchResult struct {
	Tanggal       time.Time
	TotalProcessed int
	TotalSuccess  int
	TotalSkipped  int
	TotalFailed   int
	DLQCount      int
	Errors        []CronItemError
}

// CronItemError captures a per-instrument error in a cron batch.
type CronItemError struct {
	InstrumenID uuid.UUID
	ErrorCode   string
	ErrorDetail string
}

// ─── Helper functions ─────────────────────────────────────────────────────────

// ParseDateStrict parses "YYYY-MM-DD" strictly.
func ParseDateStrict(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal format tidak valid '%s': wajib YYYY-MM-DD", s)
	}
	return t, nil
}

// ToAkrualListItem converts PendapatanAkrual to list response.
func ToAkrualListItem(a *PendapatanAkrual, instrumenKode string) AkrualListItem {
	basis := string(BasisGross)
	if a.Stage != nil && *a.Stage == 3 {
		basis = string(BasisNetCarrying)
	}

	item := AkrualListItem{
		ID:                  a.ID.String(),
		InstrumenID:         a.InstrumenID.String(),
		InstrumenKode:       instrumenKode,
		KlasifikasiSnapshot: a.KlasifikasiSnapshot,
		TanggalAkrual:       a.TanggalAkrual.Format("2006-01-02"),
		Jenis:               string(a.Jenis),
		Stage:               a.Stage,
		CarryingBasis:       basis,
		CarryingIdr:         a.CarryingBasisIDR.StringFixed(4),
		BungaKotor:          a.BungaKotor.StringFixed(4),
		Pph:                 a.PPh.StringFixed(4),
		BungaBersih:         a.BungaBersih.StringFixed(4),
		MataUang:            a.MataUang,
		StaleStagingFlag:    a.StaleStagingFlag,
		Status:              string(a.Status),
		CreatedAt:           a.CreatedAt.Format(time.RFC3339),
	}
	if a.EIRPersen != nil {
		s := a.EIRPersen.StringFixed(8)
		item.EirPersen = &s
	}
	if a.FXRateID != nil {
		s := a.FXRateID.String()
		item.FxRateId = &s
	}
	if a.ECLRunIDUsed != nil {
		s := a.ECLRunIDUsed.String()
		item.EclRunIdUsed = &s
	}
	if a.JurnalHeaderID != nil {
		s := a.JurnalHeaderID.String()
		item.JurnalHeaderId = &s
	}
	return item
}

// ToJatuhTempoListItem converts JatuhTempo to list response.
func ToJatuhTempoListItem(jt *JatuhTempo, instrumenKode string) JatuhTempoListItem {
	item := JatuhTempoListItem{
		ID:                  jt.ID.String(),
		InstrumenID:         jt.InstrumenID.String(),
		InstrumenKode:       instrumenKode,
		TanggalJatuhTempo:   jt.TanggalJatuhTempo.Format("2006-01-02"),
		Jenis:               jt.Jenis,
		PokokIdr:            jt.PokokReturned.StringFixed(4),
		BungaLastIdr:        jt.BungaReturned.StringFixed(4),
		PphIdr:              jt.PPh.StringFixed(4),
		NetKasIdr:           jt.Proceeds.StringFixed(4),
		KlasifikasiSnapshot: jt.KlasifikasiSnapshot,
		Status:              string(jt.Status),
		ErrorMessage:        jt.ErrorMessage,
		CreatedAt:           jt.CreatedAt.Format(time.RFC3339),
	}
	if jt.JurnalHeaderID != nil {
		s := jt.JurnalHeaderID.String()
		item.JurnalHeaderId = &s
	}
	return item
}

// ErrAkrualStagingStale is the sentinel for stale staging.
var ErrAkrualStagingStale = domainerrors.New(domainerrors.CodeValidationFailed,
	"ECL sealed run untuk instrumen melebihi batas staleness. Akrual Stage 3 membutuhkan ECL terkini.")
