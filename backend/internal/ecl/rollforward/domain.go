// Package rollforward implements the P4-M11 Roll-Forward CKPN service.
//
// Replaces the PARTIAL_PHASE_5_DEFER stub from M7 with a complete implementation.
//
// Formula (formulas.md §Roll-forward, FSD-APP-C §5, SoW §4):
//
//	ECL_closing = ECL_opening
//	            + Σ stage_transfers (6 directional buckets, signed)
//	            + new_originations
//	            − derecognitions
//	            ± remeasurements
//
// Reconcile invariant: |closing − Σ ecl.calc_result_line.ecl_weighted_idr| < IDR 1.0000
//
// Sign convention (OQ-M11-002-A locked):
//
//	Positive = INCREASE in ECL allowance (loss booked) — e.g. stage1To2, originations
//	Negative = DECREASE in ECL allowance (cure/release) — e.g. stage2To1, stage3To2
//
// Detection method: BASIC_STATUS_DIFF (Phase 4).
// Stage 3→1: only via management override (OQ-M11-002-B locked).
// FVTPL→AC reclassification: treated as origination (OQ-M11-003-B locked).
// Reconcile tolerance: IDR 1.0000 absolute (OQ-M11-001-C locked, DEC-016).
//
// No float64 — shopspring/decimal throughout (DEC-016).
// Stories: APP-C-M11-001..006
// State machine: docs/state-machines/p4-m11-roll-forward.md
// OpenAPI: api/openapi/app-c-roll-forward.yaml
package rollforward

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Permission constants ─────────────────────────────────────────────────────

const (
	// PermRollForwardRead allows reading roll-forward reports.
	// Roles: ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-ALCO.
	PermRollForwardRead = "ecl.roll_forward.read"

	// PermRollForwardCompute allows computing roll-forward reports.
	// Roles: ROLE-RISK, ROLE-AKUN-CTL.
	PermRollForwardCompute = "ecl.roll_forward.compute"

	// PermRollForwardExport allows exporting disclosure XLSX/CSV.
	// Roles: ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT.
	PermRollForwardExport = "ecl.roll_forward.export"

	// PermPortfolioAggregateRead allows reading portfolio aggregate data.
	// Roles: ROLE-RISK, ROLE-CFO.
	PermPortfolioAggregateRead = "ecl.portfolio_aggregate.read"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodeRollForwardPriorNotFound — 404 priorCalcRunId not in ecl.calc_run.
	CodeRollForwardPriorNotFound = "ROLL_FORWARD_PRIOR_NOT_FOUND"

	// CodeRollForwardPriorNotSealed — 422 prior not SEALED (allow with warning).
	CodeRollForwardPriorNotSealed = "ROLL_FORWARD_PRIOR_NOT_SEALED"

	// CodeRollForwardCurrentInvalidState — 422 current must be COMPLETED or SEALED.
	CodeRollForwardCurrentInvalidState = "ROLL_FORWARD_CURRENT_INVALID_STATE"

	// CodeRollForwardPeriodeMismatch — 422 current periode must be after prior periode.
	CodeRollForwardPeriodeMismatch = "ROLL_FORWARD_PERIODE_MISMATCH"

	// CodeRollForwardDetectionMethodInvalid — 422 detectionMethod not in valid enum.
	CodeRollForwardDetectionMethodInvalid = "ROLL_FORWARD_DETECTION_METHOD_INVALID"

	// CodeRollForwardExportMismatchForbidden — 422 export blocked when MISMATCH.
	CodeRollForwardExportMismatchForbidden = "ROLL_FORWARD_EXPORT_MISMATCH_FORBIDDEN"

	// CodeRollForwardPortfolioNotFound — 404 portofolioId not in mst.portofolio.
	CodeRollForwardPortfolioNotFound = "ROLL_FORWARD_PORTFOLIO_NOT_FOUND"

	// CodeRollForwardTrendInsufficientData — 422 need ≥ 2 SEALED calc runs.
	CodeRollForwardTrendInsufficientData = "ROLL_FORWARD_TREND_INSUFFICIENT_DATA"

	// CodeRollForwardScopeMismatch — 422 scope of current and prior run incompatible.
	CodeRollForwardScopeMismatch = "ROLL_FORWARD_SCOPE_MISMATCH"

	// CodeRollForwardInvalidCalcRunStatus — 422 alias for CURRENT_INVALID_STATE (UI).
	CodeRollForwardInvalidCalcRunStatus = "ROLL_FORWARD_INVALID_CALC_RUN_STATUS"

	// CodeRollForwardInvalidPriorPeriod — 422 alias for PERIODE_MISMATCH (UI).
	CodeRollForwardInvalidPriorPeriod = "ROLL_FORWARD_INVALID_PRIOR_PERIOD"
)

// ─── Warning codes (non-error, returned in Report.Warnings) ─────────────────

const (
	// WarnFirstPeriodOpeningZero — priorCalcRunId = nil, opening = 0.
	WarnFirstPeriodOpeningZero = "ROLL_FORWARD_FIRST_PERIOD_OPENING_ZERO"

	// WarnMismatchDetected — reconcileStatus = MISMATCH.
	WarnMismatchDetected = "ROLL_FORWARD_MISMATCH_DETECTED"

	// WarnPriorNotSealedPreview — prior not SEALED (preview mode).
	WarnPriorNotSealedPreview = "ROLL_FORWARD_PRIOR_NOT_SEALED_PREVIEW"

	// WarnHasDataQualityWarnings — per-instrument data quality issues exist.
	WarnHasDataQualityWarnings = "ROLL_FORWARD_HAS_DATA_QUALITY_WARNINGS"
)

// ─── Data quality warning codes ─────────────────────────────────────────────

const (
	// DQWarnStageHistoryMissingFallback — stage transition from calc_header (no stage_history).
	DQWarnStageHistoryMissingFallback = "STAGE_HISTORY_MISSING_FALLBACK_CALC_HEADER"

	// DQWarnInstrumenAktifNotInCurrentRun — active instrument absent from current run.
	DQWarnInstrumenAktifNotInCurrentRun = "INSTRUMEN_AKTIF_NOT_IN_CURRENT_RUN"

	// DQWarnDerecognitionReasonUnknown — cannot determine derecognition reason.
	DQWarnDerecognitionReasonUnknown = "DERECOGNITION_REASON_UNKNOWN"
)

// ─── Reconcile status ────────────────────────────────────────────────────────

// ReconcileStatus is the result of the reconcile invariant check.
type ReconcileStatus string

const (
	// ReconcileStatusReconciled means |delta| < IDR 1.0000.
	ReconcileStatusReconciled ReconcileStatus = "RECONCILED"

	// ReconcileStatusMismatch means |delta| >= IDR 1.0000 — indicates detection logic bug.
	ReconcileStatusMismatch ReconcileStatus = "MISMATCH"
)

// ─── Detection method ────────────────────────────────────────────────────────

// DetectionMethod identifies the origination/derecognition detection algorithm.
type DetectionMethod string

const (
	// DetectionMethodBasicStatusDiff is the Phase 4 method:
	// compare ecl.calc_header presence + mst.instrumen.status.
	DetectionMethodBasicStatusDiff DetectionMethod = "BASIC_STATUS_DIFF"
)

// Phase5LimitationNote is the constant text shown in UI and export Sign-Off.
// Describes the Phase 4 detection limitation per story APP-C-M11-003 AC Skenario 4.
const Phase5LimitationNote = "Deteksi origination/derecognition menggunakan perubahan status instrumen " +
	"dan kehadiran di calc_run result. Untuk deteksi berbasis transaction lifecycle events " +
	"(penempatan, penjualan, jatuh tempo), update ke Phase 5 (APP-B integration)."

// ─── Reconcile tolerance ─────────────────────────────────────────────────────

// ReconcileTolerance is IDR 1.0000 absolute (OQ-M11-001-C locked, DEC-016).
var ReconcileTolerance = decimal.RequireFromString("1.0000")

// ─── TransferBucket ──────────────────────────────────────────────────────────

// TransferBucket holds aggregate metrics for one directional stage transfer bucket.
// ECL movement is signed per OQ-M11-002-A:
//
//	Positive = increase in allowance (stage upgrades: 1→2, 2→3, 1→3)
//	Negative = decrease in allowance (cures: 2→1, 3→2, 3→1)
//
// All IDR amounts: NUMERIC(20,4) — stored as decimal.Decimal, serialized via .StringFixed(4).
type TransferBucket struct {
	// Count is the number of instruments that transitioned in this bucket.
	Count int `json:"count"`

	// EclMovementIdr is Σ(ecl_current − ecl_prior) for instruments in this bucket. Signed.
	// Formula: ecl_current_stage_Y − ecl_prior_stage_X per instrument.
	EclMovementIdr decimal.Decimal `json:"eclMovementIdr"`

	// CountOverride is the number of instruments whose transfer was due to management override
	// (trigger_type = "MANAGEMENT_OVERRIDE" in ecl.stage_history).
	// Stage 3→1 ALWAYS has CountOverride = Count (no auto-cure from Stage 3, OQ-M11-002-B).
	CountOverride int `json:"countOverride"`
}

// ─── Transfers ───────────────────────────────────────────────────────────────

// Transfers holds all 6 directional transfer buckets.
type Transfers struct {
	// Stage1To2: SICR. ECL movement positive (credit risk increased).
	Stage1To2 TransferBucket `json:"stage1To2"`

	// Stage2To1: Cure. ECL movement negative (credit risk decreased).
	Stage2To1 TransferBucket `json:"stage2To1"`

	// Stage2To3: Default. ECL movement positive.
	Stage2To3 TransferBucket `json:"stage2To3"`

	// Stage1To3: Rare direct default. ECL movement positive.
	Stage1To3 TransferBucket `json:"stage1To3"`

	// Stage3To2: Management override reverse. ECL movement negative.
	// CountOverride ≈ Count (almost always override-driven).
	Stage3To2 TransferBucket `json:"stage3To2"`

	// Stage3To1: Management override only. ECL movement negative.
	// CountOverride MUST = Count per OQ-M11-002-B (no auto-cure from Stage 3).
	Stage3To1 TransferBucket `json:"stage3To1"`
}

// SumMovement returns the sum of all 6 bucket EclMovementIdr values (signed).
// Used in remeasurement residual calculation: §4 state machine doc.
func (t Transfers) SumMovement() decimal.Decimal {
	return t.Stage1To2.EclMovementIdr.
		Add(t.Stage2To1.EclMovementIdr).
		Add(t.Stage2To3.EclMovementIdr).
		Add(t.Stage1To3.EclMovementIdr).
		Add(t.Stage3To2.EclMovementIdr).
		Add(t.Stage3To1.EclMovementIdr)
}

// ─── Originations ────────────────────────────────────────────────────────────

// Originations holds aggregate metrics for new instruments (in current, not in prior).
type Originations struct {
	// Count is the number of new instruments.
	Count int `json:"count"`

	// EclIdr is Σ(ecl_current) for all origination instruments. Always positive.
	// Includes FVTPL→AC reclassification (OQ-M11-003-B locked).
	EclIdr decimal.Decimal `json:"eclIdr"`
}

// ─── Derecognitions ──────────────────────────────────────────────────────────

// Derecognitions holds aggregate metrics for instruments removed (in prior, not in current).
type Derecognitions struct {
	// Count is the number of derecognized instruments.
	Count int `json:"count"`

	// PriorEclIdr is Σ(ecl_prior) for all derecognized instruments. Always positive.
	// ECL is reversed from the portfolio (closing = opening − derecognitions, other things equal).
	PriorEclIdr decimal.Decimal `json:"priorEclIdr"`
}

// ─── DataQualityWarning ──────────────────────────────────────────────────────

// DataQualityWarning records a per-instrument data quality issue found during computation.
type DataQualityWarning struct {
	// InstrumenID is the UUID of the instrument with the issue.
	InstrumenID uuid.UUID `json:"instrumenId"`

	// InstrumenKode is the human-readable code (may be empty if not available).
	InstrumenKode string `json:"instrumenKode,omitempty"`

	// WarningCode is the stable code identifying the issue type.
	WarningCode string `json:"warningCode"`

	// Message is a human-readable description for auditor verification.
	Message string `json:"message"`
}

// ─── Report ──────────────────────────────────────────────────────────────────

// Report is the full CKPN roll-forward report with all components.
//
// Formula (SoW §4, formulas.md §Roll-forward):
//
//	closing = opening + Σtransfers + originations − derecognitions ± remeasurements
//
// Reconcile invariant: |closing − Σcalc_header.ecl_fl_idr| < IDR 1.0000
//
// All IDR amounts: NUMERIC(20,4) — decimal.Decimal, serialize via .StringFixed(4).
type Report struct {
	// ReportID is the stable identifier used for export endpoint routing.
	// Format: "rf-{currentCalcRunID}".
	ReportID string `json:"reportId"`

	// CurrentCalcRunID is the calc run being reported on.
	CurrentCalcRunID uuid.UUID `json:"currentCalcRunId"`

	// PriorCalcRunID is the prior SEALED calc run. nil = first period (opening = 0).
	PriorCalcRunID *uuid.UUID `json:"priorCalcRunId"`

	// CurrentPeriodeID is the periode_id of the current calc run.
	CurrentPeriodeID string `json:"currentPeriodeId"`

	// PriorPeriodeID is the periode_id of the prior calc run. Empty if first period.
	PriorPeriodeID string `json:"priorPeriodeId,omitempty"`

	// OpeningEclIdr is Σ(ecl_weighted_idr) from prior calc run. 0.0000 for first period.
	// NUMERIC(20,4).
	OpeningEclIdr decimal.Decimal `json:"openingEclIdr"`

	// Transfers holds all 6 stage transfer buckets.
	Transfers Transfers `json:"transfers"`

	// NewOriginations holds aggregate for new instruments (in current, not in prior).
	NewOriginations Originations `json:"newOriginations"`

	// Derecognitions holds aggregate for removed instruments (in prior, not in current).
	Derecognitions Derecognitions `json:"derecognitions"`

	// RemeasurementsIdr is the residual ECL movement after transfers, originations, derecognitions.
	// Formula: closing − opening − Σtransfers − originations + derecognitions.
	// Can be negative (net release). NUMERIC(20,4).
	RemeasurementsIdr decimal.Decimal `json:"remeasurementsIdr"`

	// ClosingEclIdr is Σ(ecl_weighted_idr) from current calc run.
	// By definition = sum_ecl_fl_idr from ecl.calc_result_line (non-POCI rows).
	// NUMERIC(20,4).
	ClosingEclIdr decimal.Decimal `json:"closingEclIdr"`

	// ReconcileStatus is RECONCILED or MISMATCH per reconcile invariant check.
	ReconcileStatus ReconcileStatus `json:"reconcileStatus"`

	// ReconcileDeltaIdr is closing − (opening + Σtransfers + originations − derecognitions + remeasurements).
	// By construction should be 0 (remeasurements absorbs residual) but floating-point rounding
	// can produce small delta. Exposed for transparency.
	// NUMERIC(20,4).
	ReconcileDeltaIdr decimal.Decimal `json:"reconcileDeltaIdr"`

	// ReconcileTolerance is always IDR 1.0000 per OQ-M11-001-C.
	// Exposed in response for UI transparency.
	ReconcileTolerance decimal.Decimal `json:"reconcileTolerance"`

	// DetectionMethod identifies the origination/derecognition algorithm used.
	// Phase 4: always BASIC_STATUS_DIFF.
	DetectionMethod DetectionMethod `json:"detectionMethod"`

	// Phase5LimitationNote is the constant amber-banner text for origination/derecognition sections.
	Phase5LimitationNote string `json:"phase5LimitationNote"`

	// ComputedAt is the timestamp when computation completed.
	ComputedAt time.Time `json:"computedAt"`

	// Warnings holds report-level warning codes (non-error flags).
	Warnings []string `json:"warnings"`

	// DataQualityWarnings holds per-instrument data quality issues.
	// nil if no issues found.
	DataQualityWarnings []DataQualityWarning `json:"dataQualityWarnings,omitempty"`
}

// ─── ComputeRequest ──────────────────────────────────────────────────────────

// ComputeRequest is the input for Service.ComputeRollForward.
type ComputeRequest struct {
	// CurrentCalcRunID is the calc run being analyzed.
	// Must have status COMPLETED, COMPLETED_WITH_ERRORS, SEALED, or SEAL_REQUESTED.
	CurrentCalcRunID uuid.UUID

	// PriorCalcRunID is the prior SEALED run. nil = first period (opening = 0).
	PriorCalcRunID *uuid.UUID

	// DetectionMethod specifies the origination/derecognition detection algorithm.
	// Defaults to BASIC_STATUS_DIFF.
	DetectionMethod DetectionMethod

	// AllowNonSealedPrior allows prior runs that are not SEALED (preview mode, warning added).
	// Default false (production); true = add WarnPriorNotSealedPreview warning.
	AllowNonSealedPrior bool

	// ForceMismatchExport skips the MISMATCH export block for analysis-only exports.
	ForceMismatchExport bool

	// ActorID is the user triggering the compute (for audit trail).
	ActorID uuid.UUID
}

// ─── CKPNTrendPoint ──────────────────────────────────────────────────────────

// CKPNTrendPoint is one data point in the multi-period CKPN trend (M11-006).
// Source: all SEALED calc runs, ordered chronologically.
type CKPNTrendPoint struct {
	// CalcRunID is the SEALED calc run for this period.
	CalcRunID uuid.UUID `json:"calcRunId"`

	// PriorCalcRunID is the SEALED calc run for the immediately preceding period.
	// nil for the first available data point.
	PriorCalcRunID *uuid.UUID `json:"priorCalcRunId"`

	// PeriodeID is the period identifier (e.g. "JUNI-2026").
	PeriodeID string `json:"periodeId"`

	// SealedAt is when this calc run was sealed.
	SealedAt time.Time `json:"sealedAt"`

	// TotalEclIdr is Σ(ecl_weighted_idr) for this SEALED calc run.
	TotalEclIdr decimal.Decimal `json:"eclTotalIdr"`

	// EclByStage holds ECL broken down by Stage 1/2/3 for stacked bar chart.
	EclByStage EclByStage `json:"eclByStage"`

	// DeltaFromPrev is TotalEclIdr − prior point TotalEclIdr. nil for first point.
	DeltaFromPrev *decimal.Decimal `json:"deltaVsPriorIdr"`

	// DeltaPct is DeltaFromPrev / prior TotalEclIdr * 100. nil for first point.
	// Stored as string with 3 decimal places (e.g. "5.634").
	DeltaPct *string `json:"deltaPct"`
}

// EclByStage holds ECL amounts per IFRS 9 stage for one period.
type EclByStage struct {
	Stage1 decimal.Decimal `json:"stage1"`
	Stage2 decimal.Decimal `json:"stage2"`
	Stage3 decimal.Decimal `json:"stage3"`
}

// ─── ResultLineHeader ────────────────────────────────────────────────────────

// ResultLineHeader is the minimal projection from ecl.calc_result_line
// needed by the roll-forward computation.
// Columns: instrumen_id, stage, ecl_weighted_idr, ead_idr.
// Source: M7 result lines (read-only by M11).
type ResultLineHeader struct {
	// InstrumenID is the instrument UUID.
	InstrumenID uuid.UUID

	// Stage is the IFRS 9 stage (1, 2, or 3).
	Stage int

	// EclWeightedIdr is the weighted ECL for this instrument.
	// nil for POCI_DEFERRED instruments (excluded from roll-forward totals).
	EclWeightedIdr *decimal.Decimal

	// EadIdr is the EAD used as gross_carrying proxy (OQ-M11-005-A, Phase 4).
	EadIdr decimal.Decimal
}

// ─── InstrumenStatusSnapshot ─────────────────────────────────────────────────

// InstrumenStatusSnapshot is the minimal projection from mst.instrumen
// needed for derecognition reason classification (BASIC_STATUS_DIFF).
type InstrumenStatusSnapshot struct {
	// ID is the instrument UUID.
	ID uuid.UUID

	// Kode is the human-readable instrument code (for warnings).
	Kode string

	// Status is the lifecycle status: AKTIF | JATUH_TEMPO | DIJUAL | etc.
	Status string

	// TanggalJatuhTempo is the maturity date (may be zero if not set).
	TanggalJatuhTempo *time.Time
}

// ─── CalcRunSummary ──────────────────────────────────────────────────────────

// CalcRunSummary is a minimal projection from ecl.calc_run for trend dashboard.
type CalcRunSummary struct {
	// ID is the calc run UUID.
	ID uuid.UUID

	// PeriodeID is the bookkeeping period.
	PeriodeID string

	// Status is the calc run status (must be SEALED for trend).
	Status string

	// SealedAt is the timestamp when the run was sealed.
	SealedAt *time.Time

	// TenantID is the tenant for multi-tenant isolation.
	TenantID string
}

// ─── StageHistoryRow ─────────────────────────────────────────────────────────

// StageHistoryRow is a minimal projection from ecl.stage_history.
// Used to detect management override trigger type per instrument.
type StageHistoryRow struct {
	// InstrumenID is the instrument UUID.
	InstrumenID uuid.UUID

	// CalcRunID is the calc run context for this history entry.
	CalcRunID uuid.UUID

	// TriggerType identifies what caused the stage transition.
	// Examples: "SICR_RATING", "DPD", "MANAGEMENT_OVERRIDE".
	TriggerType string

	// CreatedAt is the timestamp of this history entry.
	CreatedAt time.Time
}

// ─── InstrumentBucket enum ───────────────────────────────────────────────────

// InstrumentBucket identifies which roll-forward category an instrument belongs to.
type InstrumentBucket string

// Stage transition buckets used in roll-forward movement categorization.
const (
	BucketStage1To2      InstrumentBucket = "stage_1_to_2"
	BucketStage2To1      InstrumentBucket = "stage_2_to_1"
	BucketStage2To3      InstrumentBucket = "stage_2_to_3"
	BucketStage1To3      InstrumentBucket = "stage_1_to_3"
	BucketStage3To2      InstrumentBucket = "stage_3_to_2"
	BucketStage3To1      InstrumentBucket = "stage_3_to_1"
	BucketNewOrigination InstrumentBucket = "new_origination"
	BucketDerecognition  InstrumentBucket = "derecognition"
	BucketStageSame      InstrumentBucket = "stage_same"
)

// ─── InstrumentLine ──────────────────────────────────────────────────────────

// InstrumentLine is one instrument with its roll-forward context.
// Used for drill-down DataTable (DataTable UX §1 sort+filter+cursor+export).
type InstrumentLine struct {
	InstrumenID         uuid.UUID
	InstrumenKode       string
	InstrumenNama       string
	PortofolioID        *uuid.UUID
	StagePrior          *int // nil = origination (not in prior)
	StageCurrent        *int // nil = derecognition (not in current)
	EclPriorIdr         *decimal.Decimal
	EclCurrentIdr       *decimal.Decimal
	EclMovementIdr      decimal.Decimal // signed per sign convention
	OverrideFlag        bool
	TriggerType         *string // from ecl.stage_history
	DerecognitionReason *string // MATURED | SOLD | UNKNOWN (only for derecognition bucket)
	Bucket              InstrumentBucket
}

// ─── PortfolioRollForward ────────────────────────────────────────────────────

// PortfolioRollForward is the roll-forward breakdown for one portfolio.
// Same components as Report but scoped to instruments in portofolioID.
type PortfolioRollForward struct {
	PortofolioID        uuid.UUID
	PortofolioNama      string
	CurrentCalcRunID    uuid.UUID
	PriorCalcRunID      *uuid.UUID
	InstrumentCount     int
	OpeningEclIdr       decimal.Decimal
	Transfers           Transfers
	NewOriginations     Originations
	Derecognitions      Derecognitions
	RemeasurementsIdr   decimal.Decimal
	ClosingEclIdr       decimal.Decimal
	DetectionMethod     DetectionMethod
	DataQualityWarnings []DataQualityWarning
}

// ─── domainError ─────────────────────────────────────────────────────────────

// domainError is a simple domain error for the rollforward package.
type domainError struct {
	code    string
	message string
}

func (e *domainError) Error() string { return e.code + ": " + e.message }
func (e *domainError) Code() string  { return e.code }

// errDomain creates a domain error for this package.
func errDomain(code, message string) *domainError {
	return &domainError{code: code, message: message}
}
