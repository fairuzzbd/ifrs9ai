// Package helpers implements the P4-M2 PD/LGD/EAD/CCF lookup helpers for BLIPS IFRS9.
//
// This package is READ-ONLY — it provides pure-read services for the ECL engine
// (P4-M7) and the ROLE-RISK UI preview (APP-C-PAR-001..006).
//
// IFRS9 / PSAK 71 formula reference (formulas.md, SoW §4):
//
//	ECL_skenario    = EAD_IDR × PD_skenario × LGD
//	ECL_FL_skenario = ECL_skenario × Impact_PD_multiplier_skenario
//	ECL_weighted    = Σ (ECL_FL_skenario × bobot_skenario)
//
// PD per stage (FSD-APP-C §3):
//   - Stage 1 : PD_12M × impact_pd × impact_mev_pd(skenario)
//   - Stage 2 : PD_Lifetime (linear interpolation over tenor buckets) × same FL
//   - Stage 3 : PD = 1.00000000 (fixed, FL not applied), per DEC-010
//
// EAD (FSD-APP-C §4):
//
//	EAD_FCY = Outstanding_Principal + Accrued_Interest + (Undrawn × CCF)
//	EAD_IDR = EAD_FCY × kurs_BI_JISDOR(evaluationDate)
//
// Decimal precision (DEC-016):
//   - PD / LGD / EIR : NUMERIC(10,8)   → .StringFixed(8)
//   - IDR amounts    : NUMERIC(20,4)   → .StringFixed(4)
//   - FX rates       : NUMERIC(20,8)   → .StringFixed(8)
//   - CCF            : NUMERIC(7,4)    → .StringFixed(4)
//
// No float64 anywhere in this package.
//
// Decision log:
//   - DEC-010 — ECL formula 3-stage × 3-skenario × dual FL; Stage 3 PD = 1.0
//   - DEC-011 — SICR triggers (context; staging from P4-M1)
//   - DEC-016 — NUMERIC precision
//   - DEC-017 — no workflow in M2 (read-only)
//
// See: docs/stories/phase-4/M2-pd-lgd-ead-helpers.md
// See: docs/state-machines/p4-m2-helpers.md
// See: api/openapi/app-c-helpers.yaml
package helpers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	// PermHelpersRead allows single and bulk PD/LGD/EAD/CCF lookups.
	// Roles: ROLE-RISK, ROLE-AUDIT, ROLE-ALCO.
	PermHelpersRead = "ecl_helpers.read"

	// PermHelpersPreview allows the GET /ecl/helpers/preview endpoint.
	// Roles: ROLE-RISK, ROLE-AUDIT, ROLE-ALCO.
	PermHelpersPreview = "ecl_helpers.preview"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

// EclStage mirrors the three IFRS9 stages. Values match ecl.stage_history CHECK constraint.
type EclStage string

const (
	Stage1 EclStage = "STAGE_1"
	Stage2 EclStage = "STAGE_2"
	Stage3 EclStage = "STAGE_3"
)

// IsValid returns true if s is a known stage value.
func (s EclStage) IsValid() bool {
	return s == Stage1 || s == Stage2 || s == Stage3
}

// String returns the string form of EclStage (same as its value).
func (s EclStage) String() string { return string(s) }

// EclStageFromString converts a string to EclStage. ok=false if unknown.
func EclStageFromString(s string) (EclStage, bool) {
	switch EclStage(s) {
	case Stage1, Stage2, Stage3:
		return EclStage(s), true
	}
	return "", false
}

// EclScenario is the forward-looking scenario enum.
// Default weights (DEC-010): GOOD=0.25, NORMAL=0.50, BAD=0.25.
type EclScenario string

const (
	ScenarioGood   EclScenario = "GOOD"
	ScenarioNormal EclScenario = "NORMAL"
	ScenarioBad    EclScenario = "BAD"
)

// IsValid returns true if s is a known scenario value.
func (s EclScenario) IsValid() bool {
	return s == ScenarioGood || s == ScenarioNormal || s == ScenarioBad
}

// String returns the string form of EclScenario (same as its value).
func (s EclScenario) String() string { return string(s) }

// EclScenarioFromString converts a string to EclScenario. ok=false if unknown.
func EclScenarioFromString(s string) (EclScenario, bool) {
	switch EclScenario(s) {
	case ScenarioGood, ScenarioNormal, ScenarioBad:
		return EclScenario(s), true
	}
	return "", false
}

// PermECLHelpersRead is an alias for PermHelpersRead (used in handler).
const PermECLHelpersRead = PermHelpersRead

// PermECLHelpersPreview is an alias for PermHelpersPreview (used in handler).
const PermECLHelpersPreview = PermHelpersPreview

// ─── Warning codes ────────────────────────────────────────────────────────────

// Warning code constants — non-fatal flags that appear in []HelperWarning.
// These do NOT stop computation; they are informational per OQ-M2-3, OQ-A.
const (
	// WarnAccruedZeroEIRScheduleMissing: accrued_interest = 0 because P4-M5 EIR
	// schedule not yet available. EAD is underestimated until M5 delivers.
	WarnAccruedZeroEIRScheduleMissing = "ACCRUED_INTEREST_ZERO_EIR_SCHEDULE_MISSING"

	// WarnCCFTypeNotInConfig: tipe_instrumen not in CCF_TABLE config; CCF = 0 default.
	WarnCCFTypeNotInConfig = "CCF_TYPE_NOT_IN_CONFIG_USING_DEFAULT"

	// WarnPDInterpolationNonMonotone: PD lifetime buckets in mst.pd_pefindo are not
	// monotonically increasing (data quality issue). Interpolation proceeds.
	// See: OQ-PAR-1a.
	WarnPDInterpolationNonMonotone = "PD_INTERPOLATION_NON_MONOTONE"

	// WarnOutstandingFallbackToNominal: EIR schedule not available for outstanding
	// principal; fell back to mst.instrumen.nominal. See OQ-M2-3.
	WarnOutstandingFallbackToNominal = "OUTSTANDING_FALLBACK_TO_NOMINAL"

	// WarnNormalFLMultiplierDefault: NORMAL scenario impact_mev_pd = 1.0 (default
	// assumption per OQ-A; no row in mst.impact_mev_pd for NORMAL).
	WarnNormalFLMultiplierDefault = "WARNING_NORMAL_FL_MULTIPLIER_DEFAULT"

	// WarnFXRateStale: FX rate sourced from a date more than 1 business day before evaluationDate.
	WarnFXRateStale = "WARNING_FX_RATE_STALE"
)

// HelperWarning is a non-fatal flag returned alongside a successful lookup result.
// It signals that a placeholder value or assumption was used.
type HelperWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ─── PD types ─────────────────────────────────────────────────────────────────

// PDDetail contains the full audit trace of a PD lookup.
// All decimal fields use NUMERIC(10,8) precision per DEC-016.
type PDDetail struct {
	Stage   EclStage
	Scenario EclScenario

	// PD is the final PD after FL multipliers.
	// Stage 3: always 1.00000000. FL not applied.
	PD decimal.Decimal // NUMERIC(10,8)

	// PDBase is PD before FL multipliers.
	PDBase decimal.Decimal // NUMERIC(10,8)

	// RatingUsed is the Pefindo rating of the counterparty at evaluationDate.
	// Empty for Stage 3 (PD is fixed).
	RatingUsed string

	// TenorMonthsUsed is set for Stage 2 (lifetime PD interpolation) — nil for Stage 1/3.
	TenorMonthsUsed *int

	// ImpactPDMultiplier is impact_pd.impact_multiplier applied.
	// Zero for Stage 3.
	ImpactPDMultiplier decimal.Decimal // NUMERIC(10,8)

	// ImpactMevPDMultiplier is impact_mev_pd.impact_multiplier for this scenario.
	// 1.0 for NORMAL (OQ-A: no row in mst.impact_mev_pd for NORMAL).
	// Zero for Stage 3.
	ImpactMevPDMultiplier decimal.Decimal // NUMERIC(10,8)

	// NormalMultiplierIsDefault is true when NORMAL scenario uses 1.0 because
	// no row exists in mst.impact_mev_pd for NORMAL (OQ-A flag).
	// Must be confirmed by ALCO before P4-M7 merge.
	NormalMultiplierIsDefault bool

	// SourcePD12M is pd_12month from mst.pd_pefindo, nil for Stage 2/3.
	SourcePD12M *decimal.Decimal

	// SourcePDLifetime is the interpolated lifetime PD, nil for Stage 1/3.
	SourcePDLifetime *decimal.Decimal

	Warnings []HelperWarning
}

// PDLookupService provides stage-aware PD lookups with dual forward-looking multiplier.
//
// Compliance ref: FSD-APP-C §3, formulas.md, DEC-010.
type PDLookupService interface {
	// GetPD returns PD for one instrument, stage, scenario, and evaluation period.
	//
	// Stage 1: PD_base = pd_12month; PD_FL = PD_base × impact_pd × impact_mev_pd.
	// Stage 2: PD_base = linear interpolation over lifetime tenor buckets;
	//          apply same FL multipliers.
	// Stage 3: PD = 1.00000000 (fixed, no FL applied per DEC-010).
	// FVTPL / FVOCI_ELECTION: returns INSTRUMENT_ECL_NOT_APPLICABLE.
	// No float64. All decimal.Decimal.
	GetPD(ctx context.Context, instrumenID uuid.UUID, stage EclStage, scenario EclScenario,
		periodeID string, evaluationDate time.Time) (decimal.Decimal, PDDetail, error)
}

// ─── LGD types ────────────────────────────────────────────────────────────────

// LGDDetail contains the full audit trace of an LGD lookup.
type LGDDetail struct {
	// LGD is the effective LGD after collateral haircut.
	LGD decimal.Decimal // NUMERIC(10,8)

	// PoolUsed is the mst.lgd_basel.tipe_eksposur used.
	PoolUsed string

	// BaseLGD is the pool LGD before haircut.
	BaseLGD decimal.Decimal // NUMERIC(10,8)

	// CollateralHaircut is the haircut rate applied.
	// Phase 1: always 0.00000000.
	CollateralHaircut decimal.Decimal // NUMERIC(10,8)

	// LGDEffective = BaseLGD × (1 − CollateralHaircut).
	LGDEffective decimal.Decimal // NUMERIC(10,8)

	// TipeCounterparty is mst.counterparty.tipe_counterparty used to derive pool.
	TipeCounterparty string

	Warnings []HelperWarning
}

// LGDLookupService provides pool-based LGD lookups.
//
// Compliance ref: FSD-APP-C §3, formulas.md, DEC-010.
type LGDLookupService interface {
	// GetLGD returns the effective LGD for an instrument.
	// REKSADANA returns LGD_LOOKUP_USE_LOOKTHROUGH (use P4-M4 instead).
	// FVTPL / FVOCI_ELECTION returns INSTRUMENT_ECL_NOT_APPLICABLE.
	GetLGD(ctx context.Context, instrumenID uuid.UUID, periodeID string) (decimal.Decimal, LGDDetail, error)
}

// ─── EAD types ────────────────────────────────────────────────────────────────

// EADBreakdown contains the full audit trace of an EAD computation.
// Formula (FSD-APP-C §4):
//
//	EAD_FCY = Outstanding_Principal_FCY + Accrued_Interest_FCY + (Undrawn × CCF)
//	EAD_IDR = EAD_FCY × kurs_BI_JISDOR(evaluationDate)
//
// Phase 1: Undrawn = 0, CCF = 0 per OQ-E resolution.
type EADBreakdown struct {
	OutstandingPrincipalFCY decimal.Decimal  // NUMERIC(20,4)
	AccruedInterestFCY      decimal.Decimal  // NUMERIC(20,4)
	CommittedUndrawnFCY     decimal.Decimal  // NUMERIC(20,4); Phase 1 always 0
	CCF                     decimal.Decimal  // NUMERIC(7,4); Phase 1 always 0
	EADFCY                  decimal.Decimal  // NUMERIC(20,4)
	EADIDR                  decimal.Decimal  // NUMERIC(20,4)
	FXRate                  *decimal.Decimal // NUMERIC(20,8); nil if currency = IDR
	FXSource                string           // "BI_JISDOR" or ""
	AccruedInterestSource   string           // "EIR_SCHEDULE" or "ZERO_FALLBACK"
	Currency                string           // ISO 4217 code
	Warnings                []HelperWarning
}

// EADService computes EAD per instrument per evaluation date.
type EADService interface {
	// ComputeEAD returns EAD_IDR and full breakdown for an instrument.
	// Multi-currency: uses kurs BI JISDOR per evaluationDate.
	// FVTPL / FVOCI_ELECTION returns INSTRUMENT_ECL_NOT_APPLICABLE.
	// Missing kurs returns EAD_FX_RATE_MISSING (not a warning — computation stops).
	ComputeEAD(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time) (decimal.Decimal, EADBreakdown, error)
}

// ─── CCF types ────────────────────────────────────────────────────────────────

// CCFDetail contains the source and value of a CCF lookup.
type CCFDetail struct {
	// CCF is the credit conversion factor. Phase 1: always 0.0000.
	CCF decimal.Decimal // NUMERIC(7,4)

	// Source indicates where CCF came from.
	// "PHASE_1_HARDCODED" — CCF read from sys.config CCF_TABLE, Phase 1 all = 0.
	// "SYS_CONFIG" — CCF read from sys.config and > 0 (future use).
	Source string

	Warnings []HelperWarning
}

// CCFLookupService provides CCF lookup by tipe_instrumen.
//
// Phase 1: all instruments return CCF = 0.0000 per OQ-E resolution.
// Future: COMMITMENT → 0.7500 via sys.config CCF_TABLE.
type CCFLookupService interface {
	// GetCCF returns CCF for a given tipe_instrumen from sys.config.
	// Unknown tipe returns CCF_INSTRUMEN_TYPE_UNKNOWN.
	// Missing sys.config returns CCF_CONFIG_MISSING.
	GetCCF(ctx context.Context, instrumenType string) (decimal.Decimal, CCFDetail, error)
}

// ─── Bulk types ───────────────────────────────────────────────────────────────

// BulkRequest is one item in a bulk lookup request.
type BulkRequest struct {
	InstrumenID uuid.UUID
	// Stage and Scenario are optional for bulk-lookup (engine resolves from stage_history).
	// Required for bulk-PD-only endpoint.
	Stage    *EclStage
	Scenario *EclScenario
}

// BulkResult contains all PD+LGD+EAD+CCF values for one instrument.
type BulkResult struct {
	InstrumenID uuid.UUID
	PDGood      decimal.Decimal // NUMERIC(10,8)
	PDNormal    decimal.Decimal // NUMERIC(10,8)
	PDBad       decimal.Decimal // NUMERIC(10,8)
	LGD         decimal.Decimal // NUMERIC(10,8)
	EADIDR      decimal.Decimal // NUMERIC(20,4)
	EADBreakdown EADBreakdown
	CCF         decimal.Decimal // NUMERIC(7,4)
	Warnings    []HelperWarning
}

// BulkSummary aggregates the final metrics of a bulk lookup call.
type BulkSummary struct {
	Total       int
	Success     int   // instruments fully computed without error
	Warning     int   // instruments computed but with warnings (e.g. accrued=0)
	Skipped     int   // FVTPL/FVOCI_ELECTION skipped
	ExecutionMs int64 // wall-clock ms; SLA ≤ 500ms for 1000 instruments cold cache
}

// BulkError records one instrument-level error in a bulk operation.
type BulkError struct {
	InstrumenID uuid.UUID
	ErrorCode   string
	Message     string
}

// BulkSkipped records one instrument skipped (ECL not applicable).
type BulkSkipped struct {
	InstrumenID       uuid.UUID
	Reason            string
	KlasifikasiPsak71 string
}

// BulkHelperService provides combined PD+LGD+EAD+CCF for a list of instruments.
//
// Anti-N+1 design (FSD-APP-C §4, performance SLA ≤ 500ms):
//   - All master data (PD curves, LGD pools, FL multipliers, FX rates, EIR schedules,
//     instruments, counterparties, ratings) loaded in ≤ 10 DB round-trips.
//   - O(1) in-memory lookups during the per-instrument loop.
//
// Partial failure: one instrument error does not abort the batch.
// Errors and skipped entries are collected separately.
//
// Audit: ECL_PARAM.BULK_LOOKUP_COMPLETE written once per call.
//
// Redis cache: ecl:params:bulk:{periode_id}:{evaluation_date} TTL 2h.
type BulkHelperService interface {
	// BulkLookup returns PD+LGD+EAD+CCF for the whole request list.
	// Max 1000 instruments per call; HELPERS_BULK_TOO_LARGE (413) otherwise.
	// Empty list returns ([], summary{total:0}, nil, nil, nil).
	BulkLookup(ctx context.Context, requests []BulkRequest, periodeID string,
		evaluationDate time.Time) ([]BulkResult, BulkSummary, []BulkError, []BulkSkipped, error)
}

// ─── Preview types ────────────────────────────────────────────────────────────

// PreviewItem is one row in the GET /ecl/helpers/preview response.
type PreviewItem struct {
	InstrumenID       uuid.UUID
	KodeInstrumen     string
	NamaInstrumen     string
	NamaCounterparty  *string
	KlasifikasiPsak71 string
	Stage             EclStage
	TipeInstrumen     string
	TipeEksposur      *string
	Matauang          string
	RatingAktif       *string

	// PD per scenario; nil if PD lookup failed.
	PDFlGood   *decimal.Decimal // NUMERIC(10,8)
	PDFlNormal *decimal.Decimal // NUMERIC(10,8)
	PDFlBad    *decimal.Decimal // NUMERIC(10,8)

	// LGD; nil if lookup failed.
	LGD *decimal.Decimal // NUMERIC(10,8)

	// EAD in IDR; nil if kurs missing.
	EADIDR *decimal.Decimal // NUMERIC(20,4)

	// CCF; Phase 1 always 0.0000.
	CCF *decimal.Decimal // NUMERIC(7,4)

	NormalMultiplierIsDefault bool

	Warnings []HelperWarning
}

// AllowedSortColsPreview is the whitelist for sort columns in the preview endpoint.
// Validated at init time by the repository.
var AllowedSortColsPreview = []string{
	"kode_instrumen", "stage", "lgd", "ead_idr", "pd_fl_normal", "ccf",
}

// AllowedFilterColsPreview is the whitelist for filter columns in the preview endpoint.
var AllowedFilterColsPreview = []string{
	"stage", "tipe_instrumen", "klasifikasi_psak71", "mata_uang", "has_warning",
}

// AllAllowedColsPreview is the union used by listquery.ParseFromRequest.
var AllAllowedColsPreview = append(append([]string{}, AllowedSortColsPreview...), AllowedFilterColsPreview...)

// ─── Batch parameter cache types (anti-N+1 design) ───────────────────────────

// PDCurveRow is one row from mst.pd_pefindo.
// All decimal fields use NUMERIC(10,8) per DEC-016.
type PDCurveRow struct {
	Rating         string
	PD12Month      decimal.Decimal // pd_12month
	PDLifetime3Y   decimal.Decimal // pd_lifetime_3y
	PDLifetime5Y   decimal.Decimal // pd_lifetime_5y
	PDLifetime7Y   decimal.Decimal // pd_lifetime_7y
	PDLifetime10Y  decimal.Decimal // pd_lifetime_10y
}

// ImpactPDRow is one row from mst.impact_pd.
type ImpactPDRow struct {
	PeriodeID       string
	ImpactMultiplier decimal.Decimal // NUMERIC(10,8)
}

// ImpactMevPDRow is one row from mst.impact_mev_pd.
type ImpactMevPDRow struct {
	PeriodeID        string
	Scenario         string          // "GOOD" or "BAD"
	ImpactMultiplier decimal.Decimal // NUMERIC(10,8)
}

// LGDBaselRow is one row from mst.lgd_basel.
type LGDBaselRow struct {
	TipeEksposur string
	LGD          decimal.Decimal // NUMERIC(10,8)
}

// KursRow is one row from mst.kurs.
type KursRow struct {
	KodeMatauang  string
	NilaiKurs     decimal.Decimal // NUMERIC(20,8)
	TanggalBerlaku time.Time
	WorkflowStatus string
}

// RatingHistoryRow is one row from mst.rating_history_counterparty.
type RatingHistoryRow struct {
	CounterpartyID uuid.UUID
	RatingPefindo  string
	TanggalBerlaku time.Time
}

// EIRScheduleRow is one row from ecl.eir_amortization_schedule.
type EIRScheduleRow struct {
	InstrumenID         uuid.UUID
	TanggalCicilan      time.Time
	PrincipalOutstanding decimal.Decimal // NUMERIC(20,4)
	BungaAkrual         decimal.Decimal // NUMERIC(20,4)
	ScheduleVersion     int
}

// InstrumenRow is the minimal columns from mst.instrumen needed for helpers.
type InstrumenRow struct {
	ID                uuid.UUID
	KodeInstrumen     string
	NamaInstrumen     string
	TipeInstrumen     string
	MatauangKode      string
	Nominal           decimal.Decimal // NUMERIC(20,4) — fallback outstanding
	KlasifikasiPsak71 string
	TanggalJatuhTempo *time.Time
	CounterpartyID    uuid.UUID
	Status            string
}

// CounterpartyRow is the minimal columns from mst.counterparty needed for helpers.
type CounterpartyRow struct {
	ID              uuid.UUID
	NamaCounterparty string
	TipeCounterparty string
}

// BatchParams holds all data pre-loaded for a bulk lookup run.
// This avoids N+1 queries: all data loaded once in ≤ 10 round-trips,
// then O(1) in-memory lookups per instrument.
type BatchParams struct {
	// PDCurves maps rating → PDCurveRow.
	PDCurves map[string]PDCurveRow

	// ImpactPD is the active impact_pd for the periodeID.
	ImpactPD *ImpactPDRow

	// ImpactMevPD maps scenario → ImpactMevPDRow for GOOD and BAD.
	// NORMAL is not stored (always 1.0 per OQ-A).
	ImpactMevPD map[string]ImpactMevPDRow

	// LGDPools maps tipe_eksposur → LGDBaselRow.
	LGDPools map[string]LGDBaselRow

	// FXRates maps ISO 4217 currency code → KursRow.
	FXRates map[string]KursRow

	// Instruments maps instrumenID → InstrumenRow.
	Instruments map[uuid.UUID]InstrumenRow

	// Counterparties maps counterpartyID → CounterpartyRow.
	Counterparties map[uuid.UUID]CounterpartyRow

	// Ratings maps counterpartyID → current approved RatingPefindo string.
	Ratings map[uuid.UUID]string

	// EIRSchedules maps instrumenID → latest EIRScheduleRow on or before evalDate.
	EIRSchedules map[uuid.UUID]EIRScheduleRow

	// LGDMapping maps tipe_counterparty → tipe_eksposur (from sys.config).
	LGDMapping map[string]string

	// CCFTable maps tipe_instrumen → CCF value (from sys.config).
	CCFTable map[string]decimal.Decimal

	// CollateralHaircut maps tipe_kolateral → haircut rate (from sys.config).
	// Phase 1: all 0.
	CollateralHaircut map[string]decimal.Decimal

	// CurrentStages maps instrumenID → EclStage (from ecl.stage_history).
	CurrentStages map[uuid.UUID]EclStage
}
