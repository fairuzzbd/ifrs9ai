// Package lps implements the LPS (Lembaga Penjamin Simpanan) Aggregator
// for ECL computation (APP-C-LPS-001..005).
//
// Formula (SoW §4, FSD-APP-C §3):
//
//	For each (nasabah, bank) pair, DEPOSITO instruments sorted FIFO
//	(tanggal_penempatan ASC, instrumen_id ASC):
//	  1. Convert each EAD to IDR via BI JISDOR kurs on evaluationDate.
//	  2. Instruments with APPROVED_ACTIVE exclusion override → lps_excluded=true,
//	     full EAD goes to excess (no LPS benefit).
//	  3. Walk FIFO: covered = min(EAD_IDR, remaining_cap); excess = EAD_IDR - covered.
//	  4. ECL is computed only on excess portion (DEC-010, DEC-014).
//
// INVARIANT: AllocatedToCovered + AllocatedToExcess == EAD_IDR (per instrument).
// INVARIANT: sum(Breakdown.EAD_IDR) == TotalExposureIDR (per PairAggregation).
// Cap: IDR 2 miliar per nasabah per bank (DEC-014). ALCO can override via mst.lps_coverage.
// Precision: NUMERIC(20,4), shopspring/decimal — never float64 (DEC-016).
//
// Decisions: DEC-010, DEC-014, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022.
// State machine: docs/state-machines/p4-m3-lps.md §2.
// Stories: docs/stories/phase-4/M3-lps-aggregator.md.
// Migration: db/migrations/000023_lps_exclusion_override.up.sql.
package lps

import (
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	// PermLPSCompute is the permission for internal engine compute (not exposed as HTTP perm).
	PermLPSCompute = "lps_aggregator.compute"
	// PermLPSPreview is the permission for ROLE-RISK / ROLE-AUDIT to view preview.
	PermLPSPreview = "lps_aggregator.preview"
	// PermLPSOverride is the permission for ROLE-RISK to propose exclusion overrides.
	PermLPSOverride = "lps_aggregator.override"
	// PermLPSOverrideApprove is the permission for ROLE-ALCO to approve exclusion overrides.
	PermLPSOverrideApprove = "lps_aggregator.override.approve"
	// PermLPSOverrideReject is the permission for ROLE-ALCO or ROLE-RISK to reject/recall overrides.
	PermLPSOverrideReject = "lps_aggregator.override.reject"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeLPSCoverageNoActiveParam is returned when no APPROVED mst.lps_coverage exists for evaluationDate. HTTP 422.
	CodeLPSCoverageNoActiveParam = "LPS_COVERAGE_NO_ACTIVE_PARAM"
	// CodeLPSOverrideInstrumenNotFound is returned when instrumenId is not found in mst.instrumen. HTTP 404.
	CodeLPSOverrideInstrumenNotFound = "LPS_OVERRIDE_INSTRUMEN_NOT_FOUND"
	// CodeLPSOverrideReasonTooShort is returned when alasan is shorter than 30 chars. HTTP 422.
	CodeLPSOverrideReasonTooShort = "LPS_OVERRIDE_REASON_TOO_SHORT"
	// CodeLPSOverrideInvalidTransition is returned for an invalid workflow state transition. HTTP 422.
	CodeLPSOverrideInvalidTransition = "LPS_OVERRIDE_INVALID_TRANSITION"
	// CodeLPSOverrideExpired is returned when override effectiveTo has already passed. HTTP 410.
	CodeLPSOverrideExpired = "LPS_OVERRIDE_EXPIRED"
	// CodeLPSOverrideSoDViolation is returned when approver equals maker in LPS exclusion override. HTTP 403.
	CodeLPSOverrideSoDViolation = "LPS_OVERRIDE_SOD_VIOLATION"
	// CodeLPSOverridePeriodeInvalid is returned when effectiveFrom > effectiveTo. HTTP 422.
	CodeLPSOverridePeriodeInvalid = "LPS_OVERRIDE_PERIODE_INVALID"
	// CodeLPSAggregateInstrumenNotDeposito is returned when an instrument is not of DEPOSITO type. HTTP 422.
	CodeLPSAggregateInstrumenNotDeposito = "LPS_AGGREGATE_INSTRUMEN_NOT_DEPOSITO"
	// CodeLPSAggregateBulkTooLarge is returned when instrument scope exceeds 50000. HTTP 413.
	CodeLPSAggregateBulkTooLarge = "LPS_AGGREGATE_BULK_TOO_LARGE"
)

// errLPS builds a DomainError with an LPS-specific code.
func errLPS(code string, msg string, details ...domainerrors.Detail) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.Code(code), msg, details...)
}

// ErrLPSCoverageNoActiveParam returns 422 LPS_COVERAGE_NO_ACTIVE_PARAM.
func ErrLPSCoverageNoActiveParam(date string) *domainerrors.DomainError {
	return errLPS(CodeLPSCoverageNoActiveParam,
		"Tidak ditemukan LPS coverage parameter yang APPROVED untuk tanggal "+date,
		domainerrors.Detail{
			Field:   "evaluationDate",
			Rule:    "lps_coverage_active_required",
			Message: "mst.lps_coverage tidak punya record APPROVED berlaku pada tanggal ini",
		})
}

// ErrLPSOverrideInstrumenNotFound returns 404 LPS_OVERRIDE_INSTRUMEN_NOT_FOUND.
func ErrLPSOverrideInstrumenNotFound(instrumenID string) *domainerrors.DomainError {
	return errLPS(CodeLPSOverrideInstrumenNotFound,
		"Instrumen tidak ditemukan atau sudah dihapus",
		domainerrors.Detail{
			Field:   "body.instrumenId",
			Rule:    "exists",
			Message: "instrumenId tidak ada di mst.instrumen: " + instrumenID,
		})
}

// ErrLPSOverrideReasonTooShort returns 422 LPS_OVERRIDE_REASON_TOO_SHORT.
func ErrLPSOverrideReasonTooShort(length int) *domainerrors.DomainError {
	return errLPS(CodeLPSOverrideReasonTooShort,
		"Alasan exclusion minimal 30 karakter (saat ini "+itoa(length)+" karakter)",
		domainerrors.Detail{
			Field:   "body.alasan",
			Rule:    "min_length_30",
			Message: "Alasan exclusion minimal 30 karakter",
		})
}

// ErrLPSOverrideInvalidTransition returns 422 LPS_OVERRIDE_INVALID_TRANSITION.
func ErrLPSOverrideInvalidTransition(from, to string) *domainerrors.DomainError {
	return errLPS(CodeLPSOverrideInvalidTransition,
		"Tidak bisa "+to+" dari state "+from+". Override tidak dalam status yang valid untuk transisi ini.",
		domainerrors.Detail{
			Field:   "workflowStatus",
			Rule:    "invalid_transition",
			Message: "Transition " + from + " → " + to + " tidak valid",
		})
}

// ErrLPSOverrideExpired returns 410 LPS_OVERRIDE_EXPIRED.
func ErrLPSOverrideExpired() *domainerrors.DomainError {
	return errLPS(CodeLPSOverrideExpired,
		"Proposal override ini sudah kadaluarsa. Periode effectiveTo sudah terlewati.")
}

// ErrLPSOverrideSoDViolation returns 403 LPS_OVERRIDE_SOD_VIOLATION.
func ErrLPSOverrideSoDViolation() *domainerrors.DomainError {
	return errLPS(CodeLPSOverrideSoDViolation,
		"Maker tidak dapat menjadi approver untuk proposal exclusion yang sama",
		domainerrors.Detail{
			Field:   "approver",
			Rule:    "approver_not_maker",
			Message: "SoD: approver_id ≠ maker_id",
		})
}

// ErrLPSOverridePeriodeInvalid returns 422 LPS_OVERRIDE_PERIODE_INVALID.
func ErrLPSOverridePeriodeInvalid() *domainerrors.DomainError {
	return errLPS(CodeLPSOverridePeriodeInvalid,
		"effectiveTo tidak boleh lebih awal dari effectiveFrom",
		domainerrors.Detail{
			Field:   "body.effectiveTo",
			Rule:    "gte_effective_from",
			Message: "effectiveTo harus >= effectiveFrom",
		})
}

// ErrLPSAggregateInstrumenNotDeposito returns 422 LPS_AGGREGATE_INSTRUMEN_NOT_DEPOSITO.
func ErrLPSAggregateInstrumenNotDeposito(tipe string) *domainerrors.DomainError {
	return errLPS(CodeLPSAggregateInstrumenNotDeposito,
		"Instrumen bukan DEPOSITO — LPS Aggregator hanya berlaku untuk tipe DEPOSITO. Tipe aktual: "+tipe,
		domainerrors.Detail{
			Field:   "body.instrumenId",
			Rule:    "deposito_scope_only",
			Message: "tipe_instrumen harus DEPOSITO",
		})
}

// ErrLPSAggregateBulkTooLarge returns 413 LPS_AGGREGATE_BULK_TOO_LARGE.
func ErrLPSAggregateBulkTooLarge(count int) *domainerrors.DomainError {
	return errLPS(CodeLPSAggregateBulkTooLarge,
		"Jumlah instrumen DEPOSITO dalam scope melebihi 50.000 ("+itoa(count)+
			"). Hubungi IT-Admin untuk mempartisi.")
}

// ErrFXRateNotFound returns the common FX_RATE_NOT_FOUND code reused from M2.
// Per state-machine doc §4 note, M3 uses FX_RATE_NOT_FOUND as stable code.
func ErrFXRateNotFound(currency, date string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeEADFXRateMissing,
		"Kurs BI JISDOR untuk "+currency+" tidak tersedia pada "+date+
			". Gunakan tanggal hari kerja terakhir yang tersedia.",
		domainerrors.Detail{
			Field:   "evaluationDate",
			Rule:    "fx_rate_required_for_fcy_instruments",
			Message: "Instrumen FCY ditemukan namun kurs tidak tersedia",
		})
}

// ─── WorkflowStatus ───────────────────────────────────────────────────────────

// WorkflowStatus is the state machine for ecl.lps_exclusion_override.
// Values match the CHECK constraint in migration 000023.
// Per state machine doc §2: PENDING_APPROVAL → APPROVED_ACTIVE | REJECTED → terminal.
type WorkflowStatus string

const (
	// WorkflowStatusPendingApproval is the initial state after maker submits override.
	WorkflowStatusPendingApproval WorkflowStatus = "PENDING_APPROVAL"
	// WorkflowStatusApprovedActive is the state after ALCO approves; override is in effect.
	WorkflowStatusApprovedActive WorkflowStatus = "APPROVED_ACTIVE"
	// WorkflowStatusRejected is a terminal state — override was rejected.
	WorkflowStatusRejected WorkflowStatus = "REJECTED"
	// WorkflowStatusExpired is a terminal state — override effectiveTo period has passed.
	WorkflowStatusExpired WorkflowStatus = "EXPIRED"
)

// IsTerminal returns true if the status is a final state (no further transitions).
func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusRejected || s == WorkflowStatusExpired
}

// IsActive returns true if the override is currently active (APPROVED_ACTIVE).
func (s WorkflowStatus) IsActive() bool {
	return s == WorkflowStatusApprovedActive
}

// String implements fmt.Stringer.
func (s WorkflowStatus) String() string { return string(s) }

// ─── Domain types ─────────────────────────────────────────────────────────────

// LPSExclusionOverride mirrors ecl.lps_exclusion_override (migration 000023).
// Workflow 4-eyes: ROLE-RISK (maker) → ROLE-ALCO (approver).
// SoD: maker_id ≠ approver_id (DB CHECK + service layer).
// Immutable after APPROVED_ACTIVE: signature_hash_approve set atomically on approve.
//
//nolint:revive // name intentionally prefixed with LPS for package cross-reference clarity
type LPSExclusionOverride struct {
	ID                   uuid.UUID      `db:"id"`
	InstrumenID          uuid.UUID      `db:"instrumen_id"`
	ExclusionReason      string         `db:"exclusion_reason"`
	ValidFromPeriodeID   uuid.UUID      `db:"valid_from_periode_id"`
	ValidToPeriodeID     uuid.UUID      `db:"valid_to_periode_id"`
	WorkflowStatus       WorkflowStatus `db:"workflow_status"`
	MakerID              uuid.UUID      `db:"maker_id"`
	ApproverID           *uuid.UUID     `db:"approver_id"`
	SignedAtApprove      *time.Time     `db:"signed_at_approve"`
	SignatureHashApprove []byte         `db:"signature_hash_approve"`
	CommentApprove       *string        `db:"comment_approve"`
	RejectReason         *string        `db:"reject_reason"`
	// Standard audit columns:
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// InstrumenDepositoRow holds the fields from mst.instrumen needed for LPS aggregation.
// Read-only projection — never mutated by lps package.
type InstrumenDepositoRow struct {
	ID                 uuid.UUID
	KodeInstrumen      string
	NamaInstrumen      string
	CounterpartyID     uuid.UUID // nasabah
	BankCounterpartyID uuid.UUID // bank (FK to mst.counterparty tipe='BANK')
	Nominal            decimal.Decimal
	MataUang           string // ISO 4217 currency code
	TanggalPenempatan  time.Time
	KlasifikasiPsak71  string // AC | FVOCI_DEBT
	Status             string
	TenantID           string
}

// LPSCoverageRow holds one record from mst.lps_coverage.
//
//nolint:revive // name intentionally prefixed with LPS for package cross-reference clarity
type LPSCoverageRow struct {
	ID                   uuid.UUID
	CoverageAmountIDR    decimal.Decimal // NUMERIC(20,4)
	MataUang             string          // should be 'IDR'
	PeriodeBerlakuDari   time.Time
	PeriodeBerlakuSampai *time.Time // NULL = no end date
	WorkflowStatus       string
}

// NasabahBankPair identifies a unique (nasabah, bank) pair for aggregation.
type NasabahBankPair struct {
	NasabahID uuid.UUID
	BankID    uuid.UUID
}

// AggregationWarning is a non-blocking warning emitted during aggregation.
// Example: instrumen kurs pending confirmation, override expiring soon.
type AggregationWarning struct {
	Code        string
	Message     string
	InstrumenID uuid.UUID
}

// InstrumenBreakdown holds the FIFO allocation result for one instrument.
// INVARIANT: AllocatedToCovered.Add(AllocatedToExcess).Equal(EAD_IDR).
type InstrumenBreakdown struct {
	InstrumenID         uuid.UUID
	KodeInstrumen       string
	EAD_IDR             decimal.Decimal //nolint:revive,staticcheck // NUMERIC(20,4) after FX conversion; EAD_IDR is a standard financial term
	FIFORank            int
	TanggalPenempatan   time.Time
	AllocatedToCovered  decimal.Decimal // NUMERIC(20,4)
	AllocatedToExcess   decimal.Decimal // NUMERIC(20,4)
	LPSExcluded         bool
	ExclusionReason     string // empty if not excluded
	LPSFullCovered      bool   // AllocatedToExcess.IsZero() && !LPSExcluded
	ExclusionOverrideID *uuid.UUID
}

// PairAggregation is the result for one (nasabah, bank) pair.
// INVARIANT: sum(Breakdown.EAD_IDR) == TotalExposureIDR.
// INVARIANT: CoveredIDR.Add(ExcessIDR) == sum of non-excluded EAD_IDR
//
//	TotalExposureIDR includes excluded instruments at full EAD.
type PairAggregation struct {
	CounterpartyID     uuid.UUID
	BankID             uuid.UUID
	TotalExposureIDR   decimal.Decimal // NUMERIC(20,4)
	CoveredIDR         decimal.Decimal // NUMERIC(20,4) = min(non-excluded total, cap)
	ExcessIDR          decimal.Decimal // NUMERIC(20,4) = TotalExposureIDR - CoveredIDR (excl. excluded)
	LPSCapIDR          decimal.Decimal // NUMERIC(20,4) from mst.lps_coverage
	LPSCoverageParamID uuid.UUID
	JumlahInstrumen    int
	JumlahExcluded     int
	Breakdown          []InstrumenBreakdown
	Warnings           []AggregationWarning
}

// LPSBreakdown is the flat per-instrument result used by M7 ECL engine.
// Returned from BulkAggregate keyed by instrumenID.
//
//nolint:revive // name intentionally prefixed with LPS for package cross-reference clarity
type LPSBreakdown struct {
	EAD_IDR        decimal.Decimal //nolint:revive,staticcheck // NUMERIC(20,4); EAD_IDR is a standard financial term
	CoveredIDR     decimal.Decimal // NUMERIC(20,4)
	ExcessIDR      decimal.Decimal // NUMERIC(20,4)
	LPSExcluded    bool
	LPSFullCovered bool
}

// SubmitOverrideRequest is the input for OverrideService.Submit.
type SubmitOverrideRequest struct {
	InstrumenID        uuid.UUID
	ExclusionReason    string
	ValidFromPeriodeID uuid.UUID
	ValidToPeriodeID   uuid.UUID
}

// PreviewRow is one row in the coverage utilization preview DataTable.
// Keyed on (NasabahID, BankID).
type PreviewRow struct {
	NasabahID        uuid.UUID
	NasabahNama      string
	BankID           uuid.UUID
	BankNama         string
	LPSCapIDR        decimal.Decimal
	TotalExposureIDR decimal.Decimal
	CoveredIDR       decimal.Decimal
	ExcessIDR        decimal.Decimal
	CoveredPct       decimal.Decimal // (CoveredIDR / TotalExposureIDR) × 100; 2 decimal
	JumlahInstrumen  int
	JumlahExcluded   int
	EvaluationDate   time.Time
}

// AllowedSortColsPreview is the whitelist of sortable columns for the preview DataTable.
var AllowedSortColsPreview = []string{
	"nasabah_nama", "bank_nama", "total_exposure_idr",
	"covered_idr", "excess_idr", "covered_pct", "jumlah_instrumen",
}

// AllowedFilterColsPreview is the whitelist of filterable columns for the preview DataTable.
var AllowedFilterColsPreview = []string{
	"bank_id", "nasabah_id", "excess_idr", "covered_pct",
}

// AllowedColsPreview is the union for listquery.ParseFromRequest.
var AllowedColsPreview = append(append([]string{}, AllowedSortColsPreview...), AllowedFilterColsPreview...)

// AllowedSortColsOverride is the whitelist for override list DataTable.
var AllowedSortColsOverride = []string{
	"created_at", "valid_from_periode_id", "valid_to_periode_id",
	"workflow_status", "instrumen_id",
}

// AllowedFilterColsOverride is the whitelist of filterable columns for override list.
var AllowedFilterColsOverride = []string{
	"workflow_status", "instrumen_id", "maker_id", "valid_from_periode_id",
}

// AllowedColsOverride is the union for listquery.ParseFromRequest.
var AllowedColsOverride = append(append([]string{}, AllowedSortColsOverride...), AllowedFilterColsOverride...)

// ─── Signature hash ───────────────────────────────────────────────────────────

// ComputeApproveSignatureHash computes SHA-256(approverID || "APPROVE" || overrideID || signedAt || comment).
// Returns raw 32 bytes stored as BYTEA in DB.
// Per migration 000023 comment: SHA-256(approver_id || 'APPROVE' || id || signed_at_approve || comment_approve).
func ComputeApproveSignatureHash(approverID uuid.UUID, overrideID uuid.UUID, signedAt time.Time, comment string) []byte {
	payload := approverID.String() + "APPROVE" + overrideID.String() +
		signedAt.UTC().Format(time.RFC3339Nano) + comment
	sum := sha256.Sum256([]byte(payload))
	return sum[:]
}

// ─── Internal utilities ───────────────────────────────────────────────────────

// itoa converts int to string without importing strconv at domain layer.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }
