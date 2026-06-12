package core

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
	"blips-ifrs9.tugu-re.com/internal/ecl/lookthrough"
	"blips-ifrs9.tugu-re.com/internal/ecl/lps"
)

// service.go — ECLOrchestrator: the canonical ECL compute orchestrator (P4-M7).
//
// This service coordinates M1-M5 (staging, PD/LGD/EAD, LPS, look-through, EIR)
// and applies the canonical formula. It does NOT re-implement any lookup logic.
//
// Formula (SoW §4, FSD-APP-C §3, DEC-010):
//
//	ECL_skenario     = EAD_IDR × PD_skenario × LGD
//	ECL_FL_skenario  = ECL_skenario × impact_mev_pd[skenario]
//	                   (Stage 3: FL NOT applied, PD = 1.0 fixed)
//	ECL_weighted     = Σ(ECL_FL_skenario × bobot_skenario)
//
// Anti-double-multiply (OQ-M7-4):
//   M2 PDLookupService.GetPD() returns PDDetail.PD (post-FL) and PDDetail.PDBase (pre-FL).
//   M7 uses PDDetail.PDBase for ECL_skenario = EAD × PD_base × LGD,
//   then applies PDDetail.ImpactMevPDMultiplier as the FL multiplier explicitly.
//   This makes M7 the sole owner of the FL multiplication step.
//
// Constructor rule (per M3/M4 pattern): panic on nil auditWriter.

// ─── External service interfaces (injected) ───────────────────────────────────

// LPSAggregatorIface is the M3 LPS service interface used by M7.
type LPSAggregatorIface interface {
	Aggregate(ctx context.Context, nasabahID, bankID uuid.UUID, evalDate time.Time) (*lps.PairAggregation, error)
}

// LookthroughServiceIface is the M4 look-through service interface used by M7.
type LookthroughServiceIface interface {
	Compute(ctx context.Context, instrumenID, runID, periodeID uuid.UUID, evaluationDate time.Time, actorID uuid.UUID) (*lookthrough.Result, error)
}

// BobotRepo is the interface for fetching active bobot_skenario for a periodeID.
type BobotRepo interface {
	GetActiveBobot(ctx context.Context, periodeID string) (BobotSnapshot, error)
}

// ─── ECLOrchestrator ──────────────────────────────────────────────────────────

// ECLOrchestrator is the main ECL compute service.
// It delegates PD/LGD/EAD to M2, LPS cap to M3, look-through to M4,
// and applies the canonical ECL formula in M7.
//
// Constructor: NewOrchestrator — panics if auditWriter is nil (DEC-018).
type ECLOrchestrator struct {
	db          *sql.DB
	auditWriter *audit.Writer
	helpers     *helpers.Services
	lpsAgg      LPSAggregatorIface
	lookthrough LookthroughServiceIface
	instrReader InstrumenReaderIface
	bobotRepo   BobotRepo
	resultRepo  *CalcResultLineRepo
	logger      *slog.Logger
	// F3 fix: M1 staging service injected directly — eliminates probe-via-M2 pattern.
	stagingSvc StagingServiceIface
	// F4 fix: CalcRunSealChecker injected — guards ComputeBulk/ComputeSingle.
	sealChecker CalcRunSealChecker
}

// NewOrchestrator creates ECLOrchestrator.
// Panics if auditWriter is nil (per M3/M4 constructor pattern, DEC-018).
// stagingSvc and sealChecker may be nil (optional — nil stagingSvc falls back to M2-probe legacy;
// nil sealChecker disables sealed-run guard, suitable for tests without M8 wired).
func NewOrchestrator(
	db *sql.DB,
	auditWriter *audit.Writer,
	helperSvcs *helpers.Services,
	lpsAgg LPSAggregatorIface,
	lookthroughSvc LookthroughServiceIface,
	instrReader InstrumenReaderIface,
	bobotRepo BobotRepo,
	logger *slog.Logger,
) *ECLOrchestrator {
	if db == nil {
		panic("core.NewOrchestrator: db must not be nil")
	}
	if auditWriter == nil {
		panic("core.NewOrchestrator: auditWriter must not be nil (DEC-018)")
	}
	if helperSvcs == nil {
		panic("core.NewOrchestrator: helperSvcs must not be nil")
	}
	if instrReader == nil {
		panic("core.NewOrchestrator: instrReader must not be nil")
	}
	if bobotRepo == nil {
		panic("core.NewOrchestrator: bobotRepo must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ECLOrchestrator{
		db:          db,
		auditWriter: auditWriter,
		helpers:     helperSvcs,
		lpsAgg:      lpsAgg,
		lookthrough: lookthroughSvc,
		instrReader: instrReader,
		bobotRepo:   bobotRepo,
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      logger,
	}
}

// WithStagingService injects the M1 StagingServiceIface (F3 fix).
// Call after NewOrchestrator when wiring production dependencies.
func (o *ECLOrchestrator) WithStagingService(svc StagingServiceIface) *ECLOrchestrator {
	o.stagingSvc = svc
	return o
}

// WithSealChecker injects the CalcRunSealChecker (F4 fix).
// Call after NewOrchestrator when wiring M8 seal checker.
func (o *ECLOrchestrator) WithSealChecker(checker CalcRunSealChecker) *ECLOrchestrator {
	o.sealChecker = checker
	return o
}

// ─── ComputeSingle ────────────────────────────────────────────────────────────

// ComputeSingle computes ECL for one instrument.
//
// Routing:
//   - FVTPL / FVOCI_ELECTION → RoutingSkipFVTPL, ecl_weighted = 0, no persist, audit FVTPL_SKIPPED.
//   - flag_poci = true       → RoutingPOCIDeferred, computed via STANDARD + warning. F2 fix.
//   - REKSADANA              → RoutingLookthrough, delegates to M4.
//   - CASH / DEPOSITO        → RoutingLPS, delegates to M3, ECL on excess only.
//   - default                → RoutingStandard, M2 PD+LGD+EAD.
//
// Stage 3 override: PD = 1.0 (fixed), FL not applied.
// If req.Persist=true + req.CalcRunID != nil: writes to ecl.calc_result_line in a transaction
// and writes audit ECL.COMPUTE to aud.audit_log IN THE SAME TRANSACTION.
// F4 fix: if CalcRunID is set and sealChecker is injected, returns ECL_CALC_RUN_SEALED (423) if sealed.
func (o *ECLOrchestrator) ComputeSingle(ctx context.Context, req ComputeRequest) (*ComputeResult, error) {
	// F4 fix: sealed-run guard when CalcRunID is provided.
	if req.CalcRunID != nil && o.sealChecker != nil {
		sealed, err := o.sealChecker.IsSealedCalcRun(ctx, *req.CalcRunID)
		if err != nil {
			return nil, fmt.Errorf("core.ComputeSingle: seal check: %w", err)
		}
		if sealed {
			return nil, errDomain(CodeECLCalcRunSealed, "calc run is sealed — no recompute allowed")
		}
	}

	// 1. Load instrument snapshot.
	inst, err := o.instrReader.GetByID(ctx, req.InstrumenID)
	if err != nil {
		return nil, err
	}

	// 2. Determine routing.
	routing := DetermineRouting(inst)

	// 3. Handle skipped paths immediately (no staging/PD lookup needed).
	switch routing {
	case RoutingSkipFVTPL:
		return o.handleSkipFVTPL(ctx, req, inst)
	case RoutingPOCIDeferred:
		return o.handlePOCI(ctx, req, inst)
	case RoutingLookthrough:
		return o.handleLookthrough(ctx, req, inst)
	}

	// 4. Get current stage from M1 staging service.
	stage, err := o.resolveStage(ctx, req.InstrumenID)
	if err != nil {
		return nil, err
	}

	// 5. Load active bobot snapshot for periodeID.
	bobot, err := o.bobotRepo.GetActiveBobot(ctx, req.PeriodeID)
	if err != nil {
		return nil, err
	}
	if err := bobot.Validate(); err != nil {
		return nil, err
	}

	// 6. Dispatch to LPS or STANDARD.
	switch routing {
	case RoutingLPS:
		return o.handleLPS(ctx, req, inst, stage, bobot)
	default:
		return o.handleStandard(ctx, req, inst, stage, bobot)
	}
}

// ─── FVTPL skip ──────────────────────────────────────────────────────────────

func (o *ECLOrchestrator) handleSkipFVTPL(ctx context.Context, req ComputeRequest, inst *InstrumenSnapshot) (*ComputeResult, error) {
	zero := decimal.Zero
	result := &ComputeResult{
		InstrumenID:    req.InstrumenID,
		CalcRunID:      req.CalcRunID,
		EvaluationDate: req.EvaluationDate,
		PeriodeID:      req.PeriodeID,
		RoutingPath:    RoutingSkipFVTPL,
		FlagPOCI:       inst.FlagPOCI,
		ECLWeightedIDR: &zero,
		Warnings:       []string{WarnFVTPLSkip},
	}

	// Audit FVTPL_SKIPPED even though no row is persisted (OQ-M7-5).
	if req.Persist && req.CalcRunID != nil {
		tx, err := o.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("core.ComputeSingle FVTPL: begin tx: %w", err)
		}
		defer rollbackTx(ctx, tx, o.logger)

		txWriter := o.auditWriter.WithTx(tx)
		if err := txWriter.Write(ctx, audit.Event{
			Action:     "ECL.FVTPL_SKIPPED",
			EntityType: "mst.instrumen",
			EntityID:   req.InstrumenID,
			After: map[string]any{
				"routing":         "SKIP_FVTPL",
				"calc_run_id":     req.CalcRunID,
				"evaluation_date": req.EvaluationDate.Format("2006-01-02"),
			},
			ActorUserID: req.ActorID.String(),
		}); err != nil {
			return nil, fmt.Errorf("core.ComputeSingle FVTPL: audit: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("core.ComputeSingle FVTPL: commit: %w", err)
		}
	}
	return result, nil
}

// ─── POCI path ────────────────────────────────────────────────────────────────

// handlePOCI computes ECL via the STANDARD path for POCI instruments and appends
// a warning that credit-adjusted EIR is required (Phase 5 defer).
//
// F2 fix (MAJOR): Previously returned ECLWeightedIDR=nil with no persist.
// Scope spec: "ECL still computed via STANDARD path with flag + warning".
// POCI routing_path column is set to 'POCI_DEFERRED' for audit trail.
// ECLWeightedIDR is non-nil (computed via STANDARD formula).
// Credit-adjusted EIR adjustment is deferred to Phase 5 (FSD-APP-C §4.3).
func (o *ECLOrchestrator) handlePOCI(ctx context.Context, req ComputeRequest, inst *InstrumenSnapshot) (*ComputeResult, error) {
	// Resolve stage via standard path.
	stage, err := o.resolveStage(ctx, req.InstrumenID)
	if err != nil {
		return nil, err
	}

	bobot, err := o.bobotRepo.GetActiveBobot(ctx, req.PeriodeID)
	if err != nil {
		return nil, err
	}
	if err := bobot.Validate(); err != nil {
		return nil, err
	}

	// Compute via STANDARD formula — inst already loaded.
	result, err := o.handleStandard(ctx, req, inst, stage, bobot)
	if err != nil {
		return nil, fmt.Errorf("core.handlePOCI STANDARD compute: %w", err)
	}

	// Override routing_path to POCI_DEFERRED for audit column.
	result.RoutingPath = RoutingPOCIDeferred
	result.FlagPOCI = true
	// Append POCI warning — ECL is computed but credit-adjusted EIR not yet applied.
	result.Warnings = append(result.Warnings, WarnPOCIRequiresFullCAEIR)

	return result, nil
}

// ─── Look-through (M4) ────────────────────────────────────────────────────────

func (o *ECLOrchestrator) handleLookthrough(ctx context.Context, req ComputeRequest, inst *InstrumenSnapshot) (*ComputeResult, error) {
	if o.lookthrough == nil {
		return nil, fmt.Errorf("core.ComputeSingle LOOKTHROUGH: lookthrough service not wired")
	}

	runID := uuid.Nil
	if req.CalcRunID != nil {
		runID = *req.CalcRunID
	}
	periodeUUID, err := uuid.Parse(req.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("core.ComputeSingle LOOKTHROUGH: invalid periodeID %q: %w", req.PeriodeID, err)
	}

	ltResult, err := o.lookthrough.Compute(ctx, req.InstrumenID, runID, periodeUUID, req.EvaluationDate, req.ActorID)
	if err != nil {
		return nil, fmt.Errorf("core.ComputeSingle LOOKTHROUGH: %w", err)
	}

	ecl := ltResult.TotalECLIDR.RoundBank(4)
	result := &ComputeResult{
		InstrumenID:    req.InstrumenID,
		CalcRunID:      req.CalcRunID,
		EvaluationDate: req.EvaluationDate,
		PeriodeID:      req.PeriodeID,
		RoutingPath:    RoutingLookthrough,
		FlagPOCI:       inst.FlagPOCI,
		ECLWeightedIDR: &ecl,
	}

	return result, nil
}

// ─── Standard path (M2 PD+LGD+EAD) ─────────────────────────────────────────

func (o *ECLOrchestrator) handleStandard(
	ctx context.Context,
	req ComputeRequest,
	inst *InstrumenSnapshot,
	stage Stage,
	bobot BobotSnapshot,
) (*ComputeResult, error) {
	// Convert to M2 stage type.
	m2Stage := stageToM2(stage)
	periodeID := req.PeriodeID

	// Get PD for all three scenarios.
	// M7 uses PDDetail.PDBase (pre-FL) to avoid double-multiply (OQ-M7-4).
	_, pdGoodDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioGood, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleStandard PD-GOOD: %w", err)
	}
	_, pdNormalDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioNormal, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleStandard PD-NORMAL: %w", err)
	}
	_, pdBadDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioBad, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleStandard PD-BAD: %w", err)
	}

	// Get LGD.
	_, lgdDetail, err := o.helpers.LGD.GetLGD(ctx, req.InstrumenID, periodeID)
	if err != nil {
		return nil, fmt.Errorf("core.handleStandard LGD: %w", err)
	}

	// Get EAD.
	eadIDR, _, err := o.helpers.EAD.ComputeEAD(ctx, req.InstrumenID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleStandard EAD: %w", err)
	}

	return o.applyFormulaAndPersist(ctx, req, inst, stage, bobot,
		pdGoodDetail, pdNormalDetail, pdBadDetail,
		lgdDetail, eadIDR,
	)
}

// ─── LPS path (M3) ────────────────────────────────────────────────────────────

func (o *ECLOrchestrator) handleLPS(
	ctx context.Context,
	req ComputeRequest,
	inst *InstrumenSnapshot,
	stage Stage,
	bobot BobotSnapshot,
) (*ComputeResult, error) {
	if o.lpsAgg == nil {
		// Fall back to standard if LPS not wired (should not happen in production).
		return o.handleStandard(ctx, req, inst, stage, bobot)
	}

	// Get LPS aggregation: covered + excess per (nasabah, bank).
	agg, err := o.lpsAgg.Aggregate(ctx, inst.NasabahID, inst.CounterpartyID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleLPS Aggregate: %w", err)
	}

	// ECL computed only on excess (EAD above LPS cap).
	// agg.ExcessIDR is the amount NOT covered by LPS guarantee.
	if agg.ExcessIDR.IsZero() || agg.ExcessIDR.IsNegative() {
		// Fully covered by LPS — ECL = 0 for this instrument.
		zero := decimal.Zero
		return &ComputeResult{
			InstrumenID:    req.InstrumenID,
			CalcRunID:      req.CalcRunID,
			EvaluationDate: req.EvaluationDate,
			PeriodeID:      req.PeriodeID,
			RoutingPath:    RoutingLPS,
			Stage:          stage,
			FlagPOCI:       inst.FlagPOCI,
			EADIDR:         &zero,
			ECLWeightedIDR: &zero,
		}, nil
	}

	// Get PD/LGD from M2 using excess EAD.
	m2Stage := stageToM2(stage)
	periodeID := req.PeriodeID

	_, pdGoodDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioGood, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleLPS PD-GOOD: %w", err)
	}
	_, pdNormalDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioNormal, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleLPS PD-NORMAL: %w", err)
	}
	_, pdBadDetail, err := o.helpers.PD.GetPD(ctx, req.InstrumenID, m2Stage, helpers.ScenarioBad, periodeID, req.EvaluationDate)
	if err != nil {
		return nil, fmt.Errorf("core.handleLPS PD-BAD: %w", err)
	}
	_, lgdDetail, err := o.helpers.LGD.GetLGD(ctx, req.InstrumenID, periodeID)
	if err != nil {
		return nil, fmt.Errorf("core.handleLPS LGD: %w", err)
	}

	// Override EAD with excess only.
	excessEAD := agg.ExcessIDR.RoundBank(4)
	return o.applyFormulaAndPersist(ctx, req, inst, stage, bobot,
		pdGoodDetail, pdNormalDetail, pdBadDetail,
		lgdDetail, excessEAD,
	)
}

// ─── applyFormulaAndPersist ──────────────────────────────────────────────────

// applyFormulaAndPersist applies the canonical ECL formula and optionally persists.
// Called from handleStandard and handleLPS after PD/LGD/EAD are resolved.
// OQ-M7-4: uses PDDetail.PDBase for formula, PDDetail.ImpactMevPDMultiplier for FL.
func (o *ECLOrchestrator) applyFormulaAndPersist(
	ctx context.Context,
	req ComputeRequest,
	inst *InstrumenSnapshot,
	stage Stage,
	bobot BobotSnapshot,
	pdGoodDetail helpers.PDDetail,
	pdNormalDetail helpers.PDDetail,
	pdBadDetail helpers.PDDetail,
	lgdDetail helpers.LGDDetail,
	eadIDR decimal.Decimal,
) (*ComputeResult, error) {
	// Collect warnings from M2 helpers.
	warnings := make([]string, 0, len(pdGoodDetail.Warnings)+len(pdNormalDetail.Warnings)+len(lgdDetail.Warnings))
	for _, w := range pdGoodDetail.Warnings {
		warnings = append(warnings, w.Code)
	}
	for _, w := range pdNormalDetail.Warnings {
		warnings = append(warnings, w.Code)
	}
	for _, w := range lgdDetail.Warnings {
		warnings = append(warnings, w.Code)
	}

	// Get prior sealed ECL for Stage 3 net carrying.
	var priorSealedECL *decimal.Decimal
	if stage == Stage3 {
		var err error
		priorSealedECL, err = o.resultRepo.GetPriorSealedECL(ctx, req.InstrumenID)
		if err != nil {
			return nil, fmt.Errorf("core.applyFormula GetPriorSealedECL: %w", err)
		}
		if priorSealedECL == nil {
			// First run for this Stage 3 instrument.
			warnings = append(warnings, WarnStage3NetCarryingFirstRun)
		}
	}

	// F1 fix (DEC-010): combined FL multiplier = impact_pd × impact_mev_pd.
	// Prior bug: only ImpactMevPDMultiplier was used, silently dropping impact_pd factor.
	// Pattern B (audit decomposition preserved): both multiplier components stored in
	// fl_multiplier_* columns as the combined product for full audit trail.
	// Stage 3: FL multipliers are nil (formula.go will skip FL application).
	var flGood, flNormal, flBad *decimal.Decimal
	if stage != Stage3 {
		fg := pdGoodDetail.ImpactPDMultiplier.Mul(pdGoodDetail.ImpactMevPDMultiplier).RoundBank(8)
		fn := pdNormalDetail.ImpactPDMultiplier.Mul(pdNormalDetail.ImpactMevPDMultiplier).RoundBank(8)
		fb := pdBadDetail.ImpactPDMultiplier.Mul(pdBadDetail.ImpactMevPDMultiplier).RoundBank(8)
		flGood = &fg
		flNormal = &fn
		flBad = &fb
	}

	fr := ComputeFormula(
		eadIDR,
		pdGoodDetail.PDBase,
		pdNormalDetail.PDBase,
		pdBadDetail.PDBase,
		lgdDetail.LGD,
		flGood, flNormal, flBad,
		bobot,
		priorSealedECL,
		stage,
	)

	// Build result.
	pdScenarios := &ScenarioValues{Good: fr.PDGood, Normal: fr.PDNormal, Bad: fr.PDBad}
	eclScenarios := &ScenarioValues{Good: fr.ECLGoodIDR, Normal: fr.ECLNormalIDR, Bad: fr.ECLBadIDR}
	eclFLScenarios := &ScenarioValues{Good: fr.ECLFLGoodIDR, Normal: fr.ECLFLNormalIDR, Bad: fr.ECLFLBadIDR}
	bobotScenarios := &ScenarioValues{Good: bobot.Good, Normal: bobot.Normal, Bad: bobot.Bad}
	lgdUsed := lgdDetail.LGD
	eadResult := eadIDR.RoundBank(4)
	ecl := fr.ECLWeightedIDR

	var flScenarios *ScenarioValues
	if stage != Stage3 && flGood != nil {
		flScenarios = &ScenarioValues{Good: *flGood, Normal: *flNormal, Bad: *flBad}
	}

	result := &ComputeResult{
		InstrumenID:             req.InstrumenID,
		CalcRunID:               req.CalcRunID,
		EvaluationDate:          req.EvaluationDate,
		PeriodeID:               req.PeriodeID,
		Stage:                   stage,
		RoutingPath:             DetermineRouting(inst),
		FlagPOCI:                inst.FlagPOCI,
		EADIDR:                  &eadResult,
		PDUsedPerScenario:       pdScenarios,
		LGDUsed:                 &lgdUsed,
		FLMultiplierPerScenario: flScenarios,
		BobotSnapshot:           bobotScenarios,
		ECLPerScenarioIDR:       eclScenarios,
		ECLFLPerScenarioIDR:     eclFLScenarios,
		ECLWeightedIDR:          &ecl,
		NetCarryingIDR:          fr.NetCarryingIDR,
		PriorSealedECLIDR:       fr.PriorSealedECLIDR,
		Warnings:                warnings,
	}

	// Persist if requested.
	if req.Persist && req.CalcRunID != nil {
		lineID := uuid.New()
		result.ResultLineID = &lineID

		row := ResultLineRow{
			ID:                lineID,
			CalcRunID:         *req.CalcRunID,
			InstrumenID:       req.InstrumenID,
			EvaluationDate:    req.EvaluationDate,
			PeriodeID:         req.PeriodeID,
			Stage:             stage,
			RoutingPath:       DetermineRouting(inst),
			EADIDR:            eadIDR,
			PDGood:            &fr.PDGood,
			PDNormal:          &fr.PDNormal,
			PDBad:             &fr.PDBad,
			LGDUsed:           &lgdUsed,
			FLGood:            flGood,
			FLNormal:          flNormal,
			FLBad:             flBad,
			ECLGoodIDR:        fr.ECLGoodIDR,
			ECLNormalIDR:      fr.ECLNormalIDR,
			ECLBadIDR:         fr.ECLBadIDR,
			ECLFLGoodIDR:      fr.ECLFLGoodIDR,
			ECLFLNormalIDR:    fr.ECLFLNormalIDR,
			ECLFLBadIDR:       fr.ECLFLBadIDR,
			ECLWeightedIDR:    &ecl,
			BobotGood:         bobot.Good,
			BobotNormal:       bobot.Normal,
			BobotBad:          bobot.Bad,
			NetCarryingIDR:    fr.NetCarryingIDR,
			PriorSealedECLIDR: fr.PriorSealedECLIDR,
			FlagPOCI:          inst.FlagPOCI,
			Warnings:          warnings,
			ActorID:           req.ActorID,
			FormulaVersion:    FormulaVersionM7, // F8 fix: stamp formula version
		}

		tx, err := o.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("core.applyFormula persist: begin tx: %w", err)
		}
		defer rollbackTx(ctx, tx, o.logger)

		if err := o.resultRepo.InsertResultLine(ctx, tx, row); err != nil {
			return nil, fmt.Errorf("core.applyFormula persist: insert: %w", err)
		}

		txWriter := o.auditWriter.WithTx(tx)
		if err := txWriter.Write(ctx, audit.Event{
			Action:     "ECL.COMPUTE",
			EntityType: "ecl.calc_result_line",
			EntityID:   lineID,
			After: map[string]any{
				"instrumen_id":     req.InstrumenID,
				"calc_run_id":      *req.CalcRunID,
				"stage":            int(stage),
				"routing_path":     string(result.RoutingPath),
				"ecl_weighted_idr": ecl.StringFixed(4),
				"evaluation_date":  req.EvaluationDate.Format("2006-01-02"),
				"warnings":         warnings,
			},
			ActorUserID: req.ActorID.String(),
		}); err != nil {
			return nil, fmt.Errorf("core.applyFormula persist: audit: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("core.applyFormula persist: commit: %w", err)
		}
	}

	return result, nil
}

// ─── resolveStage ─────────────────────────────────────────────────────────────

// resolveStage returns the current IFRS9 stage for an instrument.
//
// F3 fix (MAJOR): Previously probed M2 GetPD with dummy periodeID="PROBE" and silently
// defaulted to Stage 1 on ALL errors — hiding infrastructure failures as staging issues.
// This fix injects M1 StagingServiceIface directly:
//   - ErrNotFound (no staging history) → Stage 1 (correct default for new instruments)
//   - Any other error → propagate as ECL_STAGING_LOOKUP_ERROR (500)
//
// Legacy fallback: if stagingSvc not injected (nil), falls back to M2-probe pattern
// for backward compat during incremental wiring. Production must inject stagingSvc.
func (o *ECLOrchestrator) resolveStage(ctx context.Context, instrumenID uuid.UUID) (Stage, error) {
	// F3 fix: use injected M1 staging service when available.
	if o.stagingSvc != nil {
		stage, err := o.stagingSvc.GetCurrentStage(ctx, instrumenID)
		if err != nil {
			// Check if it is the ErrStagingNotFound sentinel (no history = new instrument).
			if ce, ok := err.(*coreError); ok && ce.Code() == CodeECLStagingNotFound {
				return Stage1, nil
			}
			// Any other error: propagate — do NOT silently default (F3 requirement).
			return 0, fmt.Errorf("core.resolveStage: %w: %s",
				errDomain(CodeECLStagingLookupError, "staging lookup failed"), err.Error())
		}
		return stage, nil
	}

	// Legacy fallback: probe via M2 PDLookupService when stagingSvc not yet wired.
	// Production deployments MUST inject stagingSvc via WithStagingService().
	pdVal, pdDetail, err := o.helpers.PD.GetPD(ctx, instrumenID,
		helpers.Stage1, helpers.ScenarioNormal, "PROBE", time.Now())
	if err != nil {
		// Legacy: not-found or probe error → Stage 1 default (new instrument).
		_ = pdVal
		return Stage1, nil
	}
	_ = pdVal

	switch pdDetail.Stage {
	case helpers.Stage2:
		return Stage2, nil
	case helpers.Stage3:
		return Stage3, nil
	default:
		return Stage1, nil
	}
}

// ─── GetResult ────────────────────────────────────────────────────────────────

// GetResult returns one result line by (calcRunID, instrumenID).
func (o *ECLOrchestrator) GetResult(ctx context.Context, calcRunID, instrumenID uuid.UUID) (*ResultLineRow, error) {
	return o.resultRepo.GetResultLine(ctx, calcRunID, instrumenID)
}

// ListResults returns paginated result lines for a calc run.
func (o *ECLOrchestrator) ListResults(ctx context.Context, req ListResultsRequest) (*ListResultsResponse, error) {
	return o.resultRepo.ListResultLines(ctx, req)
}

// ─── GetPortfolioSummary ──────────────────────────────────────────────────────

// GetPortfolioSummary returns aggregated ECL per stage for a portfolio.
func (o *ECLOrchestrator) GetPortfolioSummary(ctx context.Context, req PortfolioSummaryRequest) (*PortfolioSummary, error) {
	rows, err := o.resultRepo.GetPortfolioAggregate(ctx, req.CalcRunID, req.PortofolioID)
	if err != nil {
		return nil, err
	}

	var total decimal.Decimal
	for _, r := range rows {
		if r.Stage == "TOTAL" {
			total = r.ECLWeightedTotalIDR
		}
	}

	summary := &PortfolioSummary{
		PortofolioID:        req.PortofolioID,
		CalcRunID:           req.CalcRunID,
		PriorCalcRunID:      req.PriorCalcRunID,
		SummaryByStage:      rows,
		ECLWeightedIDRTotal: total,
	}
	return summary, nil
}

// ─── GetRollForward ───────────────────────────────────────────────────────────

// GetRollForward returns the CKPN roll-forward report.
// Formula: opening + originations − derecognitions ± transfers ± remeasurements = closing.
//
// F5 fix (MAJOR): transfer decomposition (NewOriginations, Derecognitions, Stage transfers)
// requires a dedicated Phase 5 report service with per-instrument delta analysis.
// This method returns Status = PARTIAL_PHASE_5_DEFER with those components set to nil.
// RemeasurementsIDR = closing − opening (delta) is always populated.
// IsReconciled = false when any component is nil (PSAK 71 §5.5 disclosure compliance).
func (o *ECLOrchestrator) GetRollForward(ctx context.Context, req RollForwardRequest) (*RollForwardReport, error) {
	closing, err := o.resultRepo.GetCalcRunECLTotal(ctx, req.CalcRunID)
	if err != nil {
		return nil, err
	}

	var opening decimal.Decimal
	if req.PriorCalcRunID != nil {
		opening, err = o.resultRepo.GetCalcRunECLTotal(ctx, *req.PriorCalcRunID)
		if err != nil {
			return nil, err
		}
	}

	// F5 fix: delta = closing − opening (only guaranteed component in Phase 4).
	// Transfer decomposition (originations, derecognitions, stage transfers) deferred to Phase 5.
	// IsReconciled = false — null components prevent reconciliation assertion.
	delta := closing.Sub(opening)
	reconcile := RollForwardReconcile{
		SumCalcResultECL: closing,
		ClosingECL:       closing,
		DifferenceIDR:    decimal.Zero,
		IsReconciled:     false, // F5: cannot reconcile without all transfer components
	}

	return &RollForwardReport{
		CalcRunID:      req.CalcRunID,
		PriorCalcRunID: req.PriorCalcRunID,
		PortofolioID:   req.PortofolioID,
		OpeningECLIDR:  opening,
		// F5: nil components — transfer decomposition deferred to Phase 5
		NewOriginationsIDR:     nil,
		DerecognitionsIDR:      nil,
		TransfersToStage2IDR:   nil,
		TransfersToStage3IDR:   nil,
		TransfersFromStage2IDR: nil,
		TransfersFromStage3IDR: nil,
		RemeasurementsIDR:      delta,
		ClosingECLIDR:          closing,
		ReconcileCheck:         reconcile,
		Status:                 RollForwardStatusPartialPhase5Defer,
	}, nil
}

// ─── RecomputeAdHoc ───────────────────────────────────────────────────────────

// RecomputeAdHoc performs a preview recompute (no persist) and compares with stored.
// Used by ROLE-RISK for debugging and validation.
func (o *ECLOrchestrator) RecomputeAdHoc(ctx context.Context, req RecomputeAdHocRequest) (*RecomputeAdHocResult, error) {
	// Run preview (no persist).
	computeReq := ComputeRequest{
		InstrumenID:    req.InstrumenID,
		EvaluationDate: req.EvaluationDate,
		PeriodeID:      req.PeriodeID,
		Persist:        false,
		ActorID:        req.ActorID,
	}
	recomputed, err := o.ComputeSingle(ctx, computeReq)
	if err != nil {
		return nil, fmt.Errorf("core.RecomputeAdHoc: %w", err)
	}

	result := &RecomputeAdHocResult{
		InstrumenID: req.InstrumenID,
		Recomputed:  *recomputed,
	}

	// Optionally load stored result for comparison.
	if req.ComparePersisted {
		// Look for any existing result line for this instrument (latest).
		stored, err := o.loadLatestStoredResult(ctx, req.InstrumenID)
		if err != nil {
			o.logger.WarnContext(ctx, "core.RecomputeAdHoc: load stored result failed", "error", err)
		} else if stored != nil {
			result.Stored = stored
			if recomputed.ECLWeightedIDR != nil && stored.ECLWeightedIDR != nil {
				delta := recomputed.ECLWeightedIDR.Sub(*stored.ECLWeightedIDR).RoundBank(4)
				result.Delta = &ECLDelta{
					ECLWeightedDeltaIDR: delta,
					IsSealedComparison:  stored.SealedAt != nil,
				}
			}
		}
	}

	// Audit ad-hoc recompute.
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("core.RecomputeAdHoc: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, o.logger)

	txWriter := o.auditWriter.WithTx(tx)
	var eclStr *string
	if recomputed.ECLWeightedIDR != nil {
		s := recomputed.ECLWeightedIDR.StringFixed(4)
		eclStr = &s
	}
	if err := txWriter.Write(ctx, audit.Event{
		Action:     "ECL.RECOMPUTE_ADHOC",
		EntityType: "mst.instrumen",
		EntityID:   req.InstrumenID,
		After: map[string]any{
			"routing":           string(recomputed.RoutingPath),
			"ecl_weighted_idr":  eclStr,
			"evaluation_date":   req.EvaluationDate.Format("2006-01-02"),
			"periode_id":        req.PeriodeID,
			"compare_persisted": req.ComparePersisted,
		},
		ActorUserID: req.ActorID.String(),
	}); err != nil {
		return nil, fmt.Errorf("core.RecomputeAdHoc: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("core.RecomputeAdHoc: commit: %w", err)
	}

	return result, nil
}

func (o *ECLOrchestrator) loadLatestStoredResult(ctx context.Context, instrumenID uuid.UUID) (*StoredECLResult, error) {
	q := `
SELECT ecl_weighted_idr, pd_used_good, pd_used_normal, pd_used_bad,
       calc_run_id, sealed_at, evaluation_date
FROM ecl.calc_result_line
WHERE instrumen_id = $1 AND deleted_at IS NULL
ORDER BY evaluation_date DESC, created_at DESC
LIMIT 1`
	var (
		eclRaw, pdGRaw, pdNRaw, pdBRaw sql.NullString
		calcRunID                      uuid.UUID
		sealedAt                       sql.NullTime
		evalDate                       time.Time
	)
	err := o.db.QueryRowContext(ctx, q, instrumenID).Scan(
		&eclRaw, &pdGRaw, &pdNRaw, &pdBRaw, &calcRunID, &sealedAt, &evalDate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	stored := &StoredECLResult{
		CalcRunID:      calcRunID,
		EvaluationDate: evalDate,
	}
	if eclRaw.Valid {
		d, err := decimal.NewFromString(eclRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.loadLatestStoredResult: parse ecl_weighted_idr %q: %w", eclRaw.String, err)
		}
		stored.ECLWeightedIDR = &d
	}
	if pdGRaw.Valid && pdNRaw.Valid && pdBRaw.Valid {
		g, err := decimal.NewFromString(pdGRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.loadLatestStoredResult: parse pd_used_good %q: %w", pdGRaw.String, err)
		}
		n, err := decimal.NewFromString(pdNRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.loadLatestStoredResult: parse pd_used_normal %q: %w", pdNRaw.String, err)
		}
		b, err := decimal.NewFromString(pdBRaw.String)
		if err != nil {
			return nil, fmt.Errorf("core.loadLatestStoredResult: parse pd_used_bad %q: %w", pdBRaw.String, err)
		}
		stored.PDUsed = &ScenarioValues{Good: g, Normal: n, Bad: b}
	}
	if sealedAt.Valid {
		t := sealedAt.Time
		stored.SealedAt = &t
	}
	return stored, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// rollbackTx rolls back a transaction and logs any rollback error at WARN level.
// Used in defer to satisfy errcheck without silently discarding rollback failures.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(ctx, "core: tx rollback error", "error", err)
	}
}

// stageToM2 converts core.Stage to helpers.EclStage.
func stageToM2(s Stage) helpers.EclStage {
	switch s {
	case Stage2:
		return helpers.Stage2
	case Stage3:
		return helpers.Stage3
	default:
		return helpers.Stage1
	}
}
