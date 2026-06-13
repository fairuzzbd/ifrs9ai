// Package core implements the ECL Core Calculation orchestrator (P4-M7).
//
// This package is the ORCHESTRATOR — it delegates formula inputs to M1-M5 services and
// applies the canonical ECL formula. It does NOT re-implement PD/LGD/EAD lookups.
//
// Formula (SoW §4, FSD-APP-C §3, DEC-010):
//
//	ECL_skenario     = EAD × PD_skenario × LGD
//	ECL_FL_skenario  = ECL_skenario × impact_mev_pd[skenario].impact_multiplier
//	                   (Stage 3: FL multiplier NOT applied — PD fixed 1.0)
//	ECL_weighted     = Σ(ECL_FL_skenario × bobot_skenario)
//
// FL multiplier source: mst.impact_mev_pd[skenario] ONLY. No double-multiply.
// Stage 3 net carrying: gross_ead − prior_sealed_ecl (0 if first run).
// POCI: ecl_weighted_idr = NULL (not 0 — semantics differ, Phase 5 defer).
// FVTPL: skip entirely, no ecl.calc_result_line row written.
//
// Decimal precision (DEC-016):
//   - IDR: NUMERIC(20,4). PD/LGD/FL: NUMERIC(10,8). Bobot: NUMERIC(7,4).
//   - No float64 anywhere.
//
// Stories: APP-C-ECL-M7-001..006
// State machine: docs/state-machines/p4-m7-ecl-core.md
// OpenAPI: api/openapi/app-c-ecl-core.yaml
// Migration: db/migrations/000029_ecl_core_tables.up.sql
package core

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Permission constants ─────────────────────────────────────────────────────

const (
	// PermECLCompute allows single and bulk ECL compute.
	// Roles: ROLE-RISK (preview/ad-hoc), internal Asynq worker.
	PermECLCompute = "ecl.compute"

	// PermECLResultRead allows reading ECL results per instrument.
	// Roles: ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO.
	PermECLResultRead = "ecl.result.read"

	// PermECLResultExport allows exporting ECL results CSV/XLSX.
	// Roles: ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT.
	PermECLResultExport = "ecl.result.export"

	// PermECLPortfolioAggregateRead allows reading portfolio aggregation.
	// Roles: ROLE-RISK, ROLE-CFO, ROLE-AUDIT.
	PermECLPortfolioAggregateRead = "ecl.portfolio_aggregate.read"

	// PermECLRecomputeAdHoc allows ad-hoc recompute for debugging.
	// Roles: ROLE-RISK only.
	PermECLRecomputeAdHoc = "ecl.recompute_adhoc"

	// PermECLBulkCompute allows submitting bulk compute jobs.
	// Roles: ROLE-RISK, internal Asynq worker.
	PermECLBulkCompute = "ecl.bulk_compute"
)

// ─── Routing path enum ────────────────────────────────────────────────────────

// RoutingPath indicates which ECL computation path was taken for an instrument.
type RoutingPath string

const (
	// RoutingStandard is the default path: M2 PD/LGD/EAD helpers.
	// For AC/FVOCI: OBLIGASI, SAHAM (AC rare), etc.
	RoutingStandard RoutingPath = "STANDARD"

	// RoutingLPS delegates to M3 LPS aggregator. Used for CASH and DEPOSITO.
	// ECL is computed only on the excess over the LPS cap (IDR 2B per nasabah per bank).
	RoutingLPS RoutingPath = "LPS"

	// RoutingLookthrough delegates to M4 look-through service. Used for REKSADANA.
	// ECL is computed per underlying asset class, then weighted.
	RoutingLookthrough RoutingPath = "LOOKTHROUGH"

	// RoutingSkipFVTPL indicates the instrument is classified FVTPL or FVOCI_ELECTION.
	// No ECL is required. ecl_weighted_idr = 0.0000. No ecl.calc_result_line row written.
	RoutingSkipFVTPL RoutingPath = "SKIP_FVTPL"

	// RoutingPOCIDeferred indicates a POCI instrument without a CA-EIR schedule yet.
	// Used when flag_poci=true but no credit-adjusted EIR schedule exists.
	// ecl_weighted_idr = NULL (not 0). No ecl.calc_result_line row written.
	// Superseded by RoutingPOCIComputed once CA-EIR is available (DEC-POCI-001).
	RoutingPOCIDeferred RoutingPath = "POCI_DEFERRED"

	// RoutingPOCIComputed indicates a POCI instrument that has been fully processed
	// with credit-adjusted EIR per PSAK 71 §5.5.13 (Phase 4.5).
	//
	// ECL is computed via the STANDARD formula path using PD/LGD/EAD helpers, but
	// the EIR used for the amortization schedule was produced by Solver.SolveCreditAdjusted
	// (PD-adjusted cashflows at origination). ECL result represents the initial baseline
	// lifetime ECL (not a delta from origination). Delta computation is deferred to Phase 5.
	//
	// ecl_weighted_idr = computed non-nil value.
	// ecl.calc_result_line row IS written (unlike POCI_DEFERRED).
	// DEC-POCI-001.
	RoutingPOCIComputed RoutingPath = "POCI_COMPUTED"
)

// ─── Warning codes ────────────────────────────────────────────────────────────

const (
	// WarnPOCIRequiresFullCAEIR is emitted for POCI instruments where full CA-EIR
	// integration (jurnal P&L direct booking) is deferred to Phase 5.
	// Still emitted in Phase 4.5 for RoutingPOCIComputed. DEC-POCI-002.
	WarnPOCIRequiresFullCAEIR = "ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR"

	// WarnPOCIECLRepresentsInitialBaseline is emitted when ECL for a POCI instrument
	// represents the full initial-baseline lifetime ECL, NOT the change-in-ECL-since-origination.
	// Per PSAK 71 §5.5.13, the correct POCI ECL delta is computed starting Phase 5.
	// DEC-POCI-001: Phase 4.5 limitation — baseline persisted; delta deferred.
	WarnPOCIECLRepresentsInitialBaseline = "POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA"

	// WarnFVTPLSkip is emitted for FVTPL / FVOCI_ELECTION instruments.
	WarnFVTPLSkip = "FVTPL_SKIP"

	// WarnStage3NetCarryingFirstRun is emitted when there is no prior sealed ECL.
	// net_carrying_idr = ead_idr (assumes ECL allowance = 0 for first run).
	WarnStage3NetCarryingFirstRun = "STAGE_3_NET_CARRYING_FIRST_RUN"

	// WarnPriorSealedECLNotFound is equivalent to WarnStage3NetCarryingFirstRun for Stage 3.
	WarnPriorSealedECLNotFound = "PRIOR_SEALED_ECL_NOT_FOUND"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeECLInstrumenNotFound is returned when the instrument does not exist. HTTP 404.
	CodeECLInstrumenNotFound = "ECL_INSTRUMEN_NOT_FOUND"

	// CodeECLInstrumenNotEligible is returned for FVTPL/POCI without valid path. HTTP 422.
	CodeECLInstrumenNotEligible = "ECL_INSTRUMEN_NOT_ELIGIBLE"

	// CodeECLStagingNotFound is returned when no stage record exists in ecl.stage_history. HTTP 422.
	CodeECLStagingNotFound = "ECL_STAGING_NOT_FOUND"

	// CodeECLParameterInactive is returned when bobot/pd/lgd/FL not APPROVED for periodeId. HTTP 422.
	CodeECLParameterInactive = "ECL_PARAMETER_INACTIVE"

	// CodeECLParamNotFound is returned when bobot_skenario not found at all. HTTP 422.
	CodeECLParamNotFound = "ECL_PARAM_NOT_FOUND"

	// CodeECLPOCIFullCAEIRDeferred is the warning code for POCI instruments. HTTP 200 + warning.
	CodeECLPOCIFullCAEIRDeferred = "ECL_POCI_FULL_CAEIR_DEFERRED"

	// CodeECLBulkTooLarge is returned when scope > 10,000 instruments. HTTP 413.
	CodeECLBulkTooLarge = "ECL_BULK_TOO_LARGE"

	// CodeECLCalcRunSealed is returned when the calc run is sealed. HTTP 423.
	CodeECLCalcRunSealed = "ECL_CALC_RUN_SEALED"

	// CodeECLPriorRunNotFound is returned when prior calc run for roll-forward is missing. HTTP 422.
	CodeECLPriorRunNotFound = "ECL_PRIOR_RUN_NOT_FOUND"

	// CodeECLBulkRunning is returned when a bulk job is already running for calcRunId. HTTP 409.
	CodeECLBulkRunning = "ECL_BULK_RUNNING"

	// CodeECLStagingLookupError is returned when the staging service returns an unexpected error. HTTP 500.
	CodeECLStagingLookupError = "ECL_STAGING_LOOKUP_ERROR"
)

// ─── Scenario values ─────────────────────────────────────────────────────────

// Scenario is the ECL forward-looking scenario enum.
type Scenario string

const (
	ScenarioGood   Scenario = "GOOD"
	ScenarioNormal Scenario = "NORMAL"
	ScenarioBad    Scenario = "BAD"
)

// AllScenarios is the ordered list of all three scenarios.
var AllScenarios = []Scenario{ScenarioGood, ScenarioNormal, ScenarioBad}

// ─── ScenarioValues — per-scenario triple ────────────────────────────────────

// ScenarioValues holds a decimal value per scenario.
// All values use NUMERIC(10,8) precision for PD/LGD/FL, NUMERIC(20,4) for IDR.
type ScenarioValues struct {
	Good   decimal.Decimal
	Normal decimal.Decimal
	Bad    decimal.Decimal
}

// ─── Stage constants ──────────────────────────────────────────────────────────

// Stage is the IFRS9 ECL stage (1, 2, 3).
type Stage int

const (
	Stage1 Stage = 1
	Stage2 Stage = 2
	Stage3 Stage = 3
)

// StageFromInt converts an int to Stage. ok=false if not 1, 2, or 3.
func StageFromInt(v int) (Stage, bool) {
	switch Stage(v) {
	case Stage1, Stage2, Stage3:
		return Stage(v), true
	}
	return 0, false
}

// ─── ComputeRequest ───────────────────────────────────────────────────────────

// ComputeRequest is the input for ECLOrchestrator.ComputeSingle.
type ComputeRequest struct {
	// InstrumenID is the UUID of the instrument from mst.instrumen.
	InstrumenID uuid.UUID

	// EvaluationDate is the assessment date for ECL.
	EvaluationDate time.Time

	// PeriodeID is the active bookkeeping period identifier (e.g. "JUNI-2026").
	PeriodeID string

	// CalcRunID is optional. When set + Persist=true, results are persisted to ecl.calc_result_line.
	// When nil, the compute runs in preview mode (no persist).
	CalcRunID *uuid.UUID

	// Persist controls whether results are written to ecl.calc_result_line.
	// Requires CalcRunID to be set.
	Persist bool

	// ActorID is the user ID triggering the compute (for audit trail).
	ActorID uuid.UUID
}

// ─── ComputeResult ────────────────────────────────────────────────────────────

// ComputeResult is the full output of a single-instrument ECL computation.
// All IDR amounts are NUMERIC(20,4). All PD/LGD/FL are NUMERIC(10,8). Bobot NUMERIC(7,4).
// No float64 anywhere.
type ComputeResult struct {
	// InstrumenID is the instrument UUID.
	InstrumenID uuid.UUID

	// CalcRunID is nil in preview mode; populated when Persist=true.
	CalcRunID *uuid.UUID

	// EvaluationDate is the date the ECL was assessed.
	EvaluationDate time.Time

	// PeriodeID is the bookkeeping period.
	PeriodeID string

	// Stage is the IFRS9 staging result (1, 2, or 3).
	Stage Stage

	// RoutingPath indicates which computation path was taken.
	RoutingPath RoutingPath

	// FlagPOCI is true when the instrument is POCI (Phase 5 defer).
	FlagPOCI bool

	// EADIDR is the Exposure at Default in IDR.
	// For LPS routing: this is the excess EAD only.
	// Nil for SKIP_FVTPL and POCI_DEFERRED.
	EADIDR *decimal.Decimal

	// PDUsedPerScenario is the PD applied per scenario.
	// Stage 3: all = decimal.NewFromString("1.00000000").
	// Nil for SKIP_FVTPL and POCI_DEFERRED.
	PDUsedPerScenario *ScenarioValues

	// LGDUsed is the pool-based LGD applied.
	// Nil for SKIP_FVTPL and POCI_DEFERRED.
	LGDUsed *decimal.Decimal

	// FLMultiplierPerScenario is the impact_mev_pd multiplier per scenario.
	// Stage 3: nil (FL not applied).
	// SKIP_FVTPL / POCI_DEFERRED: nil.
	FLMultiplierPerScenario *ScenarioValues

	// BobotSnapshot is the bobot_skenario at the time of compute.
	// Snapshot preserved for ALCO audit trail.
	BobotSnapshot *ScenarioValues

	// ECLPerScenarioIDR is ECL before FL multiplier: EAD × PD × LGD.
	// Nil for SKIP_FVTPL and POCI_DEFERRED.
	ECLPerScenarioIDR *ScenarioValues

	// ECLFLPerScenarioIDR is ECL after FL multiplier: ECL_skenario × FL_multiplier.
	// Stage 3: same as ECLPerScenarioIDR (FL not applied).
	// Nil for SKIP_FVTPL and POCI_DEFERRED.
	ECLFLPerScenarioIDR *ScenarioValues

	// ECLWeightedIDR is Σ(ECL_FL × bobot). NUMERIC(20,4).
	// POCI_DEFERRED: nil (NOT 0 — different semantics).
	// SKIP_FVTPL: zero decimal.
	ECLWeightedIDR *decimal.Decimal

	// NetCarryingIDR is gross_carrying − prior_sealed_ecl, for Stage 3 only.
	// Nil for Stage 1 and 2.
	NetCarryingIDR *decimal.Decimal

	// PriorSealedECLIDR is the prior sealed ECL used for Stage 3 net carrying base.
	// Nil on first run.
	PriorSealedECLIDR *decimal.Decimal

	// ParameterSnapshotID is the reference to the frozen parameter snapshot.
	ParameterSnapshotID *uuid.UUID

	// Warnings is a list of non-fatal warning codes.
	Warnings []string

	// ResultLineID is the UUID of the ecl.calc_result_line row written (persisted mode only).
	ResultLineID *uuid.UUID
}

// ─── BulkComputeRequest ───────────────────────────────────────────────────────

// BulkComputeRequest is the input for ECLOrchestrator.ComputeBulk.
type BulkComputeRequest struct {
	// CalcRunID identifies the calc run (managed by M8).
	CalcRunID uuid.UUID

	// EvaluationDate is the ECL assessment date.
	EvaluationDate time.Time

	// PeriodeID is the bookkeeping period.
	PeriodeID string

	// Scope optionally restricts computation to a subset of instruments.
	// nil = all active non-FVTPL/non-POCI instruments.
	Scope *BulkScope

	// ActorID is the initiating user (for audit trail).
	ActorID uuid.UUID
}

// BulkScope restricts the bulk compute to a subset.
type BulkScope struct {
	// PortofolioIDs restricts to specific portfolios. nil = all.
	PortofolioIDs []uuid.UUID

	// InstrumenIDs restricts to specific instruments (max 10,000).
	InstrumenIDs []uuid.UUID
}

// ─── BulkComputeProgress ─────────────────────────────────────────────────────

// BulkComputeProgress is the progress report for bulk compute jobs.
type BulkComputeProgress struct {
	Total            int
	Processed        int
	Errors           int
	SkippedFVTPL     int
	SkippedPOCI      int
	SkippedDuplicate int
	CurrentStep      string
}

// ─── BulkComputeResult ───────────────────────────────────────────────────────

// BulkComputeResult is the summary returned after a bulk compute completes.
type BulkComputeResult struct {
	CalcRunID             uuid.UUID
	TotalScanned          int
	TotalComputed         int
	TotalSkippedFVTPL     int
	TotalPOCIDeferred     int
	TotalSkippedDuplicate int
	ECLWeightedIDRTotal   decimal.Decimal
	Errors                []BulkComputeError
	Status                string // "completed" or "completed_with_errors" or "cancelled"
}

// BulkComputeError records a per-instrument error during bulk compute.
type BulkComputeError struct {
	InstrumenID  uuid.UUID
	ErrorCode    string
	ErrorMessage string
}

// ─── PortfolioSummary ────────────────────────────────────────────────────────

// PortfolioSummary is the aggregated ECL per stage for a portfolio + calc run.
type PortfolioSummary struct {
	PortofolioID        uuid.UUID
	CalcRunID           uuid.UUID
	PriorCalcRunID      *uuid.UUID
	EvaluationDate      time.Time
	SummaryByStage      []StageSummaryRow
	ECLWeightedIDRTotal decimal.Decimal
	Notes               *string
}

// StageSummaryRow is one row in the portfolio summary (per stage).
type StageSummaryRow struct {
	Stage               string // "STAGE_1", "STAGE_2", "STAGE_3", "TOTAL"
	Count               int
	EADTotalIDR         decimal.Decimal
	ECLWeightedTotalIDR decimal.Decimal
	DeltaVsPriorIDR     *decimal.Decimal // nil when no prior run
}

// ─── RollForwardReport ───────────────────────────────────────────────────────

// RollForwardReport is the CKPN roll-forward reconciliation.
// Formula (formulas.md): opening + originations − derecognitions ± transfers ± remeasurements = closing.
//
// F5 fix: Phase 5 deferred components (NewOriginations, Derecognitions, Transfers) are *decimal.Decimal
// (nullable). When Status = PARTIAL_PHASE_5_DEFER, these fields are nil and IsReconciled = false.
// Full decomposition is deferred to Phase 5 (dedicated roll-forward report service).
type RollForwardReport struct {
	CalcRunID              uuid.UUID
	PriorCalcRunID         *uuid.UUID
	PortofolioID           *uuid.UUID
	OpeningECLIDR          decimal.Decimal
	NewOriginationsIDR     *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	DerecognitionsIDR      *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	TransfersToStage2IDR   *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	TransfersToStage3IDR   *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	TransfersFromStage2IDR *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	TransfersFromStage3IDR *decimal.Decimal // nil when Status = PARTIAL_PHASE_5_DEFER
	RemeasurementsIDR      decimal.Decimal  // opening → closing delta; always populated
	ClosingECLIDR          decimal.Decimal
	ReconcileCheck         RollForwardReconcile
	Status                 RollForwardStatus // FULL | PARTIAL_PHASE_5_DEFER
	Notes                  *string
}

// RollForwardReconcile checks that closing = Σ(ecl.calc_result_line.ecl_weighted_idr).
type RollForwardReconcile struct {
	SumCalcResultECL decimal.Decimal
	ClosingECL       decimal.Decimal
	DifferenceIDR    decimal.Decimal // abs(closing − Σ); should be < 1.0000
	IsReconciled     bool            // differenceIDR < IDR 1.0000
}

// ─── PortfolioSummaryRequest ─────────────────────────────────────────────────

// PortfolioSummaryRequest is the input for GetPortfolioSummary.
type PortfolioSummaryRequest struct {
	PortofolioID   uuid.UUID
	CalcRunID      uuid.UUID
	PriorCalcRunID *uuid.UUID
	ActorID        uuid.UUID
}

// ─── RollForwardRequest ──────────────────────────────────────────────────────

// RollForwardRequest is the input for GetRollForward.
type RollForwardRequest struct {
	CalcRunID      uuid.UUID
	PriorCalcRunID *uuid.UUID
	PortofolioID   *uuid.UUID
	ActorID        uuid.UUID
}

// ─── RecomputeAdHocRequest ───────────────────────────────────────────────────

// RecomputeAdHocRequest is the input for RecomputeAdHoc.
type RecomputeAdHocRequest struct {
	InstrumenID      uuid.UUID
	EvaluationDate   time.Time
	PeriodeID        string
	ComparePersisted bool
	ActorID          uuid.UUID
}

// RecomputeAdHocResult is the output of RecomputeAdHoc.
type RecomputeAdHocResult struct {
	InstrumenID uuid.UUID
	Recomputed  ComputeResult
	Stored      *StoredECLResult
	Delta       *ECLDelta
}

// StoredECLResult is the latest persisted ECL result for an instrument.
type StoredECLResult struct {
	ECLWeightedIDR *decimal.Decimal
	PDUsed         *ScenarioValues
	CalcRunID      uuid.UUID
	SealedAt       *time.Time
	EvaluationDate time.Time
}

// ECLDelta is the difference between recomputed and stored ECL.
type ECLDelta struct {
	ECLWeightedDeltaIDR decimal.Decimal
	IsSealedComparison  bool
}

// ─── ProgressFn ──────────────────────────────────────────────────────────────

// ProgressFn is a callback for bulk compute progress reporting (M7-002).
// Implemented by M8 bulk job wrapper to update sys.job + Redis pub/sub.
type ProgressFn func(processed, total int, currentStep string)

// ─── InstrumenSnapshot ───────────────────────────────────────────────────────

// InstrumenSnapshot is a minimal read-only view from mst.instrumen needed by M7.
type InstrumenSnapshot struct {
	ID                uuid.UUID
	KlasifikasiPsak71 string // AC | FVOCI | FVTPL | FVOCI_ELECTION
	TipeInstrumen     string // OBLIGASI | DEPOSITO | CASH | REKSADANA | SAHAM | etc.
	Status            string // AKTIF | MATURED | etc.
	WorkflowStatus    string // APPROVED | PENDING | etc.
	FlagPOCI          bool
	CounterpartyID    uuid.UUID
	NasabahID         uuid.UUID // filled for DEPOSITO/CASH (counterparty as nasabah)
	PortofolioID      *uuid.UUID
	TenantID          string
}

// IsFVTPL returns true if the instrument should be skipped (no ECL).
func (i *InstrumenSnapshot) IsFVTPL() bool {
	return i.KlasifikasiPsak71 == "FVTPL" || i.KlasifikasiPsak71 == "FVOCI_ELECTION"
}

// IsReksadana returns true for REKSADANA tipe (look-through routing).
func (i *InstrumenSnapshot) IsReksadana() bool {
	return i.TipeInstrumen == "REKSADANA"
}

// IsLPS returns true for CASH or DEPOSITO (LPS routing).
func (i *InstrumenSnapshot) IsLPS() bool {
	return i.TipeInstrumen == "CASH" || i.TipeInstrumen == "DEPOSITO"
}

// ─── BobotSnapshot ───────────────────────────────────────────────────────────

// BobotSnapshot holds the scenario weights at the time of compute.
// Preserved for ALCO audit trail in case ALCO overrides later.
// Default (DEC-010): Good=0.25, Normal=0.50, Bad=0.25.
type BobotSnapshot struct {
	Good   decimal.Decimal // NUMERIC(7,4) — default 0.2500
	Normal decimal.Decimal // NUMERIC(7,4) — default 0.5000
	Bad    decimal.Decimal // NUMERIC(7,4) — default 0.2500
}

// Sum returns Good + Normal + Bad. Must equal 1.0 (tolerance 1e-8).
func (b BobotSnapshot) Sum() decimal.Decimal {
	return b.Good.Add(b.Normal).Add(b.Bad)
}

// Validate returns an error if sum deviates from 1.0 by more than 1e-8.
func (b BobotSnapshot) Validate() error {
	tolerance := decimal.NewFromFloat(1e-8)
	one := decimal.NewFromInt(1)
	diff := b.Sum().Sub(one).Abs()
	if diff.GreaterThan(tolerance) {
		return errDomain(CodeECLParameterInactive,
			"bobot_skenario sum invalid: "+b.Sum().StringFixed(8)+" (expected 1.0 ± 1e-8)")
	}
	return nil
}

// ─── ListResultsRequest ──────────────────────────────────────────────────────

// ListResultsRequest is the paginated filter for GetResults.
type ListResultsRequest struct {
	CalcRunID    uuid.UUID
	InstrumenID  *uuid.UUID
	PortofolioID *uuid.UUID
	Stage        *Stage
	RoutingPath  *RoutingPath
	FlagPOCI     *bool
	Cursor       string
	Limit        int
	Sort         []SortSpec
	SearchQ      string
	ExportFormat string // "csv" | "xlsx" | ""
}

// SortSpec is one sort column.
type SortSpec struct {
	Col string
	Dir string // "asc" | "desc"
}

// ResultLine is one row from ecl.calc_result_line (summary for DataTable).
type ResultLine struct {
	ID             uuid.UUID
	InstrumenID    uuid.UUID
	CalcRunID      uuid.UUID
	EvaluationDate time.Time
	PeriodeID      string
	Stage          Stage
	RoutingPath    RoutingPath
	EADIDR         *decimal.Decimal
	ECLWeightedIDR *decimal.Decimal
	FlagPOCI       bool
	SealedAt       *time.Time
	CreatedAt      time.Time
}

// ListResultsResponse is the paginated response for list results.
type ListResultsResponse struct {
	Items         []ResultLine
	NextCursor    string
	HasMore       bool
	TotalEstimate int
	AppliedSort   []SortSpec
	AppliedFilter map[string]any
}

// ─── TaskPayload ──────────────────────────────────────────────────────────────

// TaskECLBulkComputePayload is the Asynq task payload for bulk ECL compute.
// Task type: "ecl:bulk_compute".
type TaskECLBulkComputePayload struct {
	JobID          string    `json:"job_id"`
	CalcRunID      uuid.UUID `json:"calc_run_id"`
	EvaluationDate time.Time `json:"evaluation_date"`
	PeriodeID      string    `json:"periode_id"`
	ActorID        uuid.UUID `json:"actor_id"`
	// Scope — nil means all active instruments.
	PortofolioIDs []uuid.UUID `json:"portofolio_ids,omitempty"`
	InstrumenIDs  []uuid.UUID `json:"instrumen_ids,omitempty"`
}

// TaskNameECLBulkCompute is the Asynq task type name for bulk ECL.
const TaskNameECLBulkCompute = "ecl:bulk_compute"

// ─── Internal helpers ─────────────────────────────────────────────────────────

// errDomain creates a domain error for the core package.
// Uses a simple struct satisfying the error interface.
func errDomain(code, message string) *coreError {
	return &coreError{code: code, message: message}
}

// coreError is a simple error type for M7.
type coreError struct {
	code    string
	message string
}

func (e *coreError) Error() string { return e.code + ": " + e.message }
func (e *coreError) Code() string  { return e.code }

// InstrumenReaderIface is the minimal interface M7 needs for mst.instrumen.
// Avoids circular imports with master packages.
type InstrumenReaderIface interface {
	// GetByID returns the instrument snapshot. Returns CodeECLInstrumenNotFound if missing.
	GetByID(ctx context.Context, id uuid.UUID) (*InstrumenSnapshot, error)

	// ListActiveByScope returns active instruments in scope.
	// If scope is nil, returns all active non-deleted instruments.
	ListActiveByScope(ctx context.Context, scope *BulkScope) ([]InstrumenSnapshot, error)
}

// StagingServiceIface is the M1 staging service interface used by M7.
// F3 fix: M7 now calls M1 directly instead of probing via M2 GetPD with a dummy periodeID.
// This eliminates the silent Stage 1 default-on-error architectural fragility.
type StagingServiceIface interface {
	// GetCurrentStage returns the current IFRS9 stage for an instrument.
	// Returns (Stage1, nil) for new instruments with no staging history (ErrNotFound semantics).
	// Returns (0, error) for unexpected database/infrastructure errors — M7 propagates these.
	GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (Stage, error)
}

// ErrStagingNotFound is the sentinel returned by StagingServiceIface when no history exists.
// M7 treats this as Stage 1 (new instrument default).
var ErrStagingNotFound = errDomain(CodeECLStagingNotFound, "no staging history for instrument")

// RollForwardStatus indicates whether roll-forward components are fully populated.
type RollForwardStatus string

const (
	// RollForwardStatusFull indicates all components are populated (Phase 5+).
	RollForwardStatusFull RollForwardStatus = "FULL"
	// RollForwardStatusPartialPhase5Defer indicates transfer components are nil (Phase 5 deferred).
	RollForwardStatusPartialPhase5Defer RollForwardStatus = "PARTIAL_PHASE_5_DEFER"
)
