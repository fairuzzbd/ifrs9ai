// Package pocidelta implements PSAK 71 §5.5.13-14 POCI Delta ECL computation
// for P5-M10: Purchased or Originated Credit Impaired instruments.
//
// POCI treatment summary:
//   - stage_marker = 'POCI' — staging engine NEVER called for POCI instruments
//   - P&L books only delta_ecl = current_lifetime_ecl − baseline_lifetime_ecl
//   - ecl.poci_baseline is WORM (immutable since origination, DEC-018)
//   - Idempotency: unique (calc_run_id, instrumen_id) in ecl.poci_delta_log
//   - All amounts: shopspring/decimal (DEC-016 — never float64)
//   - Audit in-tx for all mutations (DEC-018)
//
// References: FSD-APP-C-ECL-EIR-v1.0 §5-6, SoW_v1.4 §4, DEC-010/013/016/017/018.
package pocidelta

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Error codes (P5-M10) ─────────────────────────────────────────────────────

const (
	// CodePociBaselineMissing — ecl.poci_baseline not found when computing delta.
	// Per-instrument error (does not halt entire calc run). HTTP 422 to error_log.
	CodePociBaselineMissing = "POCI_BASELINE_MISSING"

	// CodePociBaselineImmutableViolation — attempt to insert/update a second baseline
	// for an instrument that already has one. WORM enforcement per DEC-018. HTTP 422.
	CodePociBaselineImmutableViolation = "POCI_BASELINE_IMMUTABLE_VIOLATION"

	// CodePociDeltaDuplicate — unique constraint (calc_run_id, instrumen_id) violation
	// in ecl.poci_delta_log. Idempotency guard on job retry. HTTP 409.
	CodePociDeltaDuplicate = "POCI_DELTA_DUPLICATE"

	// CodePociInstrumenNotPoci — POCI-specific endpoint called for instrumen with
	// is_poci = FALSE. HTTP 422.
	CodePociInstrumenNotPoci = "POCI_INSTRUMEN_NOT_POCI"

	// CodePociPeriodeLocked — mst.periode_buku.status_periode = 'CLOSED' when posting
	// jurnal POCI delta. Blocking. HTTP 423.
	CodePociPeriodeLocked = "POCI_PERIODE_LOCKED"

	// CodePociJurnalDirectionMismatch — delta_ecl > 0 but direction = 'DECREASE' or
	// vice versa. Bug guard — reject posting + alert IT-ADMIN + RISK. HTTP 422.
	CodePociJurnalDirectionMismatch = "POCI_JURNAL_DIRECTION_MISMATCH"
)

// Sentinel errors used internally by service.
var (
	ErrBaselineMissing           = fmt.Errorf("%s: baseline tidak ditemukan untuk instrumen POCI ini", CodePociBaselineMissing)
	ErrBaselineImmutable         = fmt.Errorf("%s: baseline POCI immutable (DEC-018) — tidak dapat di-overwrite", CodePociBaselineImmutableViolation)
	ErrDeltaDuplicate            = fmt.Errorf("%s: (calc_run_id, instrumen_id) sudah ada di poci_delta_log", CodePociDeltaDuplicate)
	ErrInstrumenNotPoci          = fmt.Errorf("%s: instrumen.is_poci = FALSE", CodePociInstrumenNotPoci)
	ErrPeriodeLocked             = fmt.Errorf("%s: periode_buku CLOSED — posting POCI delta tidak diperbolehkan", CodePociPeriodeLocked)
	ErrJurnalDirectionMismatch   = fmt.Errorf("%s: sign delta_ecl tidak sesuai direction enum — bug data", CodePociJurnalDirectionMismatch)
	ErrSkipZero                  = fmt.Errorf("delta_ecl = 0 (ZERO direction) — skip jurnal posting")
)

// IsCodeErr checks whether err carries the given POCI error code.
// Handles both *DomainError (via Message() content) and fmt.Errorf string prefix convention.
// The POCI error constants (CodePoci*) are embedded in the DomainError message by validators,
// so we check err.Error() for the code prefix in both cases.
func IsCodeErr(err error, code string) bool {
	if err == nil {
		return false
	}
	// err.Error() returns the message for DomainError and for plain fmt.Errorf.
	// Both validator paths embed the POCI code constant in the message:
	//   DomainError message: "POCI_INSTRUMEN_NOT_POCI: instrumen ... is_poci = FALSE"
	//   fmt.Errorf:          "POCI_BASELINE_MISSING: instrumen ... tidak memiliki POCI baseline"
	s := err.Error()
	return len(s) >= len(code) && s[:len(code)] == code
}

// ─── Direction enum ───────────────────────────────────────────────────────────

// Direction represents the sign of delta_ecl per PSAK 71 §5.5.14 sign convention.
type Direction string

const (
	// DirectionIncrease — delta_ecl > 0 (credit quality deteriorated).
	// Jurnal: D Beban Penurunan Nilai ECL POCI / K Cadangan ECL POCI.
	DirectionIncrease Direction = "INCREASE"

	// DirectionDecrease — delta_ecl < 0 (credit quality improved).
	// Jurnal: D Cadangan ECL POCI / K Pendapatan Pemulihan ECL POCI.
	DirectionDecrease Direction = "DECREASE"

	// DirectionZero — delta_ecl = 0 (no change). No jurnal posted.
	DirectionZero Direction = "ZERO"
)

// Valid returns true if the direction value is one of the three valid values.
func (d Direction) Valid() bool {
	return d == DirectionIncrease || d == DirectionDecrease || d == DirectionZero
}

// ─── DeltaStatus enum ─────────────────────────────────────────────────────────

// DeltaStatus represents ecl.poci_delta_log.status.
type DeltaStatus string

const (
	StatusComputed     DeltaStatus = "COMPUTED"
	StatusPosted       DeltaStatus = "POSTED"
	StatusSkippedZero  DeltaStatus = "SKIPPED_ZERO"
)

// ─── Domain entities ──────────────────────────────────────────────────────────

// Baseline is the domain entity for ecl.poci_baseline.
// WORM — never UPDATE or DELETE (DEC-018).
type Baseline struct {
	ID                        uuid.UUID        `db:"id"`
	InstrumenID               uuid.UUID        `db:"instrumen_id"`
	TanggalBaseline           time.Time        `db:"tanggal_baseline"` // DATE
	LifetimeECLAtOrigination  decimal.Decimal  `db:"lifetime_ecl_at_origination"` // NUMERIC(20,4)
	CashflowExpektasiJsonb    *json.RawMessage `db:"cashflow_expectasi_jsonb"`
	CreditAdjustedEIR         decimal.Decimal  `db:"credit_adjusted_eir"` // NUMERIC(10,8)
	OriginationDate           time.Time        `db:"origination_date"` // DATE
	// Audit — no row_version (append-only)
	CreatedAt  time.Time `db:"created_at"`
	CreatedBy  uuid.UUID `db:"created_by"`
	TenantID   string    `db:"tenant_id"`
}

// DeltaLog is the domain entity for ecl.poci_delta_log.
type DeltaLog struct {
	ID                    uuid.UUID       `db:"id"`
	CalcRunID             uuid.UUID       `db:"calc_run_id"`
	InstrumenID           uuid.UUID       `db:"instrumen_id"`
	TanggalCompute        time.Time       `db:"tanggal_compute"` // DATE — partition key
	BaselineECL           decimal.Decimal `db:"baseline_ecl"`    // snapshot from Baseline
	CurrentECL            decimal.Decimal `db:"current_ecl"`     // from this calc run
	DeltaECL              decimal.Decimal `db:"delta_ecl"`       // signed: current - baseline
	Direction             Direction       `db:"direction"`
	PriorDeltaCumulative  *decimal.Decimal `db:"prior_delta_cumulative"`
	JurnalHeaderID        *uuid.UUID      `db:"jurnal_header_id"`
	PeriodeBulananID      *uuid.UUID      `db:"periode_bulanan_id"`
	Status                DeltaStatus     `db:"status"`
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

// ─── Request/Response types ───────────────────────────────────────────────────

// CaptureBaselineRequest is the body for POST /poci/baseline.
type CaptureBaselineRequest struct {
	InstrumenID               uuid.UUID        `json:"instrumenId"                binding:"required"`
	LifetimeECLAtOrigination  decimal.Decimal  `json:"lifetimeEclAtOrigination"   binding:"required"`
	CashflowExpektasiJsonb    *json.RawMessage `json:"cashflowExpektasiJsonb"`
	CreditAdjustedEIR         decimal.Decimal  `json:"creditAdjustedEir"          binding:"required"`
	TanggalBaseline           *string          `json:"tanggalBaseline"` // defaults to today
}

// ComputeDeltaBatchRequest is the body for POST /poci/compute-delta-batch.
type ComputeDeltaBatchRequest struct {
	CalcRunID uuid.UUID `json:"calcRunId" binding:"required"`
}

// BaselineListItem is one row in GET /poci/baseline list response.
type BaselineListItem struct {
	ID                       string  `json:"id"`
	InstrumenID              string  `json:"instrumenId"`
	InstrumenKode            string  `json:"instrumenKode"`
	TanggalBaseline          string  `json:"tanggalBaseline"`
	LifetimeEclAtOrigination string  `json:"lifetimeEclAtOrigination"`
	CreditAdjustedEir        string  `json:"creditAdjustedEir"`
	CreatedAt                string  `json:"createdAt"`
}

// DeltaLogItem is one row in GET /poci/delta-log list response.
type DeltaLogItem struct {
	ID                   string  `json:"id"`
	CalcRunID            string  `json:"calcRunId"`
	InstrumenID          string  `json:"instrumenId"`
	InstrumenKode        string  `json:"instrumenKode"`
	TanggalCompute       string  `json:"tanggalCompute"`
	BaselineEcl          string  `json:"baselineEcl"`
	CurrentEcl           string  `json:"currentEcl"`
	DeltaEcl             string  `json:"deltaEcl"`
	Direction            string  `json:"direction"`
	PriorDeltaCumulative *string `json:"priorDeltaCumulative"`
	JurnalHeaderId       *string `json:"jurnalHeaderId"`
	Status               string  `json:"status"`
	LargeDeltaFlag       bool    `json:"largeDeltaFlag"`
	CreatedAt            string  `json:"createdAt"`
}

// DeltaSummary is the response for GET /poci/delta-history/summary.
type DeltaSummary struct {
	PortofolioID            *string                  `json:"portofolioId"`
	Year                    int                      `json:"year"`
	Month                   int                      `json:"month"`
	InstrumenCount          int                      `json:"instrumenCount"`
	DeltaEclMtdIdr          string                   `json:"deltaEclMtdIdr"`
	DeltaEclYtdIdr          string                   `json:"deltaEclYtdIdr"`
	NetCumulativeDeltaIdr   string                   `json:"netCumulativeDeltaIdr"`
	DirectionBreakdown      DeltaDirectionBreakdown  `json:"directionBreakdown"`
	LargeDeltaCount         int                      `json:"largeDeltaCount"`
}

// DeltaDirectionBreakdown is one part of DeltaSummary.
type DeltaDirectionBreakdown struct {
	Increase DeltaDirectionEntry `json:"increase"`
	Decrease DeltaDirectionEntry `json:"decrease"`
	Zero     DeltaZeroEntry      `json:"zero"`
}

// DeltaDirectionEntry holds count + amount for INCREASE or DECREASE.
type DeltaDirectionEntry struct {
	Count     int    `json:"count"`
	AmountIdr string `json:"amountIdr"`
}

// DeltaZeroEntry holds count for ZERO direction (no amount).
type DeltaZeroEntry struct {
	Count int `json:"count"`
}

// ─── InstrumenPociInfo — minimal fields needed from mst.instrumen ─────────────

// InstrumenPociInfo holds fields from mst.instrumen relevant for POCI delta.
type InstrumenPociInfo struct {
	ID            uuid.UUID
	KodeInstrumen string
	IsPoci        bool
	Status        string // 'ACTIVE', 'MATURED', etc.
	PortofolioID  uuid.UUID
}

// ─── Pagination ───────────────────────────────────────────────────────────────

// Pagination is the cursor-based pagination result returned by list repo methods.
// Mirrors the shape consumed by response.List (api-conventions.md DEC-022).
type Pagination struct {
	NextCursor    *string
	HasMore       bool
	TotalEstimate *int64
	Limit         int
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// ToBaselineListItem converts Baseline to list response.
func ToBaselineListItem(b *Baseline, instrumenKode string) BaselineListItem {
	return BaselineListItem{
		ID:                       b.ID.String(),
		InstrumenID:              b.InstrumenID.String(),
		InstrumenKode:            instrumenKode,
		TanggalBaseline:          b.TanggalBaseline.Format("2006-01-02"),
		LifetimeEclAtOrigination: b.LifetimeECLAtOrigination.StringFixed(4),
		CreditAdjustedEir:        b.CreditAdjustedEIR.StringFixed(8),
		CreatedAt:                b.CreatedAt.Format(time.RFC3339),
	}
}

// ToDeltaLogItem converts DeltaLog to list response.
func ToDeltaLogItem(d *DeltaLog, instrumenKode string, largeDeltaThreshold decimal.Decimal) DeltaLogItem {
	item := DeltaLogItem{
		ID:             d.ID.String(),
		CalcRunID:      d.CalcRunID.String(),
		InstrumenID:    d.InstrumenID.String(),
		InstrumenKode:  instrumenKode,
		TanggalCompute: d.TanggalCompute.Format("2006-01-02"),
		BaselineEcl:    d.BaselineECL.StringFixed(4),
		CurrentEcl:     d.CurrentECL.StringFixed(4),
		DeltaEcl:       d.DeltaECL.StringFixed(4),
		Direction:      string(d.Direction),
		Status:         string(d.Status),
		LargeDeltaFlag: d.DeltaECL.Abs().GreaterThan(largeDeltaThreshold),
		CreatedAt:      d.CreatedAt.Format(time.RFC3339),
	}
	if d.PriorDeltaCumulative != nil {
		s := d.PriorDeltaCumulative.StringFixed(4)
		item.PriorDeltaCumulative = &s
	}
	if d.JurnalHeaderID != nil {
		s := d.JurnalHeaderID.String()
		item.JurnalHeaderId = &s
	}
	return item
}
