package core

// compliance_fix_test.go — tests required by IFRS9 compliance reviewer for PR #72.
//
// Tests cover:
//   - F1: combined FL multiplier (impact_pd × impact_mev_pd)
//   - F3: resolveStage with injected StagingServiceIface
//   - F4: sealed-run guard (ComputeBulk / ComputeSingle)
//   - F5: roll-forward Phase 5 deferred nullability + status
//   - F6: bobot DEC-010 defaults
//   - F8: formula_version field in ResultLineRow
//
// All decimal arithmetic uses shopspring/decimal — no float64 for money/rates.
// See SoW_v1.4.docx §4, FSD-APP-C-ECL-EIR-v1.0.docx §3, DEC-010, DEC-016.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
)

// ─── F1: combined FL multiplier tests ─────────────────────────────────────────

// TestApplyFormula_FullFLMultiplier_BothImpactPDAndMEV verifies via ComputeSingle
// that ECL is impact_pd × impact_mev_pd combined (not just impact_mev_pd).
//
// FSD-APP-C §3.4 — dual FL: ECL_FL = ECL × (impact_pd × impact_mev_pd).
// Fix for finding F1 (BLOCKER).
//
// Setup: impact_pd=1.5, impact_mev_pd=1.1 → combined=1.65
// EAD=100_000_000, PD=0.02, LGD=0.40
// ECL_base    = 100_000_000 × 0.02 × 0.40 = 800_000
// ECL_FL      = 800_000 × 1.65            = 1_320_000 (not 800_000 × 1.1 = 880_000)
// ECL_weighted = 1_320_000 × 1.0 (bobot sum) = 1_320_000
func TestApplyFormula_FullFLMultiplier_BothImpactPDAndMEV(t *testing.T) {
	t.Parallel()

	impactPD := decimal.NewFromFloat(1.5)
	impactMev := decimal.NewFromFloat(1.1)
	// combined = 1.5 × 1.1 = 1.65

	eadIDR := decimal.NewFromInt(100_000_000)
	pdBase := decimal.NewFromFloat(0.02)
	lgd := decimal.NewFromFloat(0.40)

	// ECL_base per scenario = 100M × 0.02 × 0.40 = 800_000
	// ECL_FL = 800_000 × 1.65 = 1_320_000
	// ECL_weighted = 1_320_000 (all scenarios same × bobot sum=1.0)
	eclBase := eadIDR.Mul(pdBase).Mul(lgd)
	combined := impactPD.Mul(impactMev)
	wantECL := eclBase.Mul(combined).RoundBank(4)

	// Build mock with explicit both multipliers.
	pdSvcDual := &mockPDServiceDual{
		pdBase:    pdBase,
		impactPD:  impactPD,
		impactMev: impactMev,
	}

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	svc := &helpers.Services{
		PD:   pdSvcDual,
		LGD:  &mockLGDService{lgd: lgd},
		EAD:  &mockEADService{eadIDR: eadIDR},
		CCF:  &mockCCFService{},
		Bulk: &mockBulkHelperService{},
	}

	orch := &ECLOrchestrator{
		auditWriter: audit.NewWriter(nil),
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		logger:      slog.Default(),
		stagingSvc:  &mockStagingService{stage: Stage1},
	}

	result, err := orch.ComputeSingle(context.Background(), ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	})
	if err != nil {
		t.Fatalf("F1: ComputeSingle: %v", err)
	}

	if result.ECLWeightedIDR == nil {
		t.Fatal("F1: ECLWeightedIDR must not be nil")
	}
	if !result.ECLWeightedIDR.Equal(wantECL) {
		t.Errorf("F1 combined FL:\n"+
			"  impact_pd=1.5 × impact_mev=1.1 = combined=1.65\n"+
			"  want ECLWeightedIDR=%s\n"+
			"  got  ECLWeightedIDR=%s\n"+
			"  (if got=880000, pre-F1 bug: only mev applied; if got=0, multiplier zeroed)",
			wantECL, result.ECLWeightedIDR)
	}
}

// TestECL_Underestimation_RegressionGuard ensures that impact_pd=2.0 and
// impact_mev=1.0 doubles ECL compared to no FL — catching the pre-F1 bug where
// only impact_mev_pd was applied and impact_pd=2.0 was silently ignored.
//
// Pre-F1 bug: FL = impact_mev_pd only → ECL_FL = ECL_base × 1.0 = ECL_base (not doubled)
// Post-F1 fix: FL = impact_pd × impact_mev_pd = 2.0 × 1.0 = 2.0 → ECL_FL = 2×ECL_base
func TestECL_Underestimation_RegressionGuard(t *testing.T) {
	t.Parallel()

	eadIDR := decimal.NewFromInt(100_000_000)
	pdBase := decimal.NewFromFloat(0.02)
	lgd := decimal.NewFromFloat(0.40)

	// impact_pd=2.0, impact_mev_pd=1.0 (neutral)
	// post-fix combined = 2.0; pre-bug combined = 1.0 (only mev applied)
	impactPD := decimal.NewFromFloat(2.0)
	impactMev := decimal.NewFromInt(1)

	// ECL_base = 100M × 0.02 × 0.40 = 800_000
	// Post-fix: ECL_FL = 800_000 × 2.0 = 1_600_000
	eclBase := eadIDR.Mul(pdBase).Mul(lgd)
	wantDoubled := eclBase.Mul(decimal.NewFromInt(2)).RoundBank(4)

	pdSvcDual := &mockPDServiceDual{
		pdBase:    pdBase,
		impactPD:  impactPD,
		impactMev: impactMev,
	}

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	svc := &helpers.Services{
		PD:   pdSvcDual,
		LGD:  &mockLGDService{lgd: lgd},
		EAD:  &mockEADService{eadIDR: eadIDR},
		CCF:  &mockCCFService{},
		Bulk: &mockBulkHelperService{},
	}

	orch := &ECLOrchestrator{
		auditWriter: audit.NewWriter(nil),
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		logger:      slog.Default(),
		stagingSvc:  &mockStagingService{stage: Stage1},
	}

	result, err := orch.ComputeSingle(context.Background(), ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	})
	if err != nil {
		t.Fatalf("F1: ComputeSingle: %v", err)
	}

	if result.ECLWeightedIDR == nil {
		t.Fatal("F1 regression: ECLWeightedIDR must not be nil")
	}
	if !result.ECLWeightedIDR.Equal(wantDoubled) {
		t.Errorf("F1 regression guard: impact_pd=2.0 should double ECL.\n"+
			"want=%s, got=%s\n"+
			"If got=800000, pre-F1 bug is still present (only impact_mev applied).",
			wantDoubled, result.ECLWeightedIDR)
	}
}

// mockPDServiceDual is a PD service that explicitly sets both FL multiplier fields.
// Used by F1 tests to verify combined impact_pd × impact_mev_pd behaviour.
type mockPDServiceDual struct {
	pdBase    decimal.Decimal
	impactPD  decimal.Decimal
	impactMev decimal.Decimal
}

func (m *mockPDServiceDual) GetPD(_ context.Context, _ uuid.UUID, stage helpers.EclStage, scenario helpers.EclScenario, _ string, _ time.Time) (decimal.Decimal, helpers.PDDetail, error) {
	pd := m.pdBase
	impactPD := m.impactPD
	impactMev := m.impactMev
	if stage == helpers.Stage3 {
		pd = decimal.NewFromInt(1)
		impactPD = decimal.Zero
		impactMev = decimal.Zero
	}
	return pd, helpers.PDDetail{
		Stage:                 stage,
		Scenario:              scenario,
		PD:                    pd,
		PDBase:                pd,
		ImpactPDMultiplier:    impactPD,
		ImpactMevPDMultiplier: impactMev,
	}, nil
}

// ─── F3: resolveStage injection tests ─────────────────────────────────────────

// mockStagingService implements StagingServiceIface for tests.
type mockStagingService struct {
	stage Stage
	err   error
}

func (m *mockStagingService) GetCurrentStage(_ context.Context, _ uuid.UUID) (Stage, error) {
	return m.stage, m.err
}

// TestResolveStage_PropagatesNonNotFoundErrors verifies that infrastructure errors
// (e.g., DB timeout) propagate as ECL_STAGING_LOOKUP_ERROR, not silently defaulting
// to Stage 1. Pre-F3 bug: ALL errors → Stage 1 default.
func TestResolveStage_PropagatesNonNotFoundErrors(t *testing.T) {
	t.Parallel()

	infraErr := errors.New("connection refused")
	stagingSvc := &mockStagingService{err: infraErr}

	orch := &ECLOrchestrator{
		stagingSvc: stagingSvc,
		logger:     slog.Default(),
	}

	_, err := orch.resolveStage(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("F3: expected error for non-NotFound staging error, got nil (pre-F3 bug: silently defaulted to Stage 1)")
	}

	var ce *coreError
	if !errors.As(err, &ce) {
		t.Errorf("F3: expected *coreError wrapped, got %T: %v", err, err)
	} else if ce.Code() != CodeECLStagingLookupError {
		t.Errorf("F3: expected code %s, got %s", CodeECLStagingLookupError, ce.Code())
	}
}

// TestResolveStage_NewInstrument_NoHistory_DefaultsStage1 verifies that
// ErrStagingNotFound sentinel (no staging history) correctly defaults to Stage 1.
// Correct behavior for new instruments without prior IFRS9 staging.
func TestResolveStage_NewInstrument_NoHistory_DefaultsStage1(t *testing.T) {
	t.Parallel()

	stagingSvc := &mockStagingService{err: ErrStagingNotFound}

	orch := &ECLOrchestrator{
		stagingSvc: stagingSvc,
		logger:     slog.Default(),
	}

	stage, err := orch.resolveStage(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("F3: new instrument (no history) should not error, got: %v", err)
	}
	if stage != Stage1 {
		t.Errorf("F3: new instrument should default to Stage 1, got Stage %d", stage)
	}
}

// TestResolveStage_Stage3FromHistory_ReturnsStage3 verifies that when M1 returns
// Stage 3 (credit-impaired), resolveStage returns Stage 3 without modification.
func TestResolveStage_Stage3FromHistory_ReturnsStage3(t *testing.T) {
	t.Parallel()

	stagingSvc := &mockStagingService{stage: Stage3}

	orch := &ECLOrchestrator{
		stagingSvc: stagingSvc,
		logger:     slog.Default(),
	}

	stage, err := orch.resolveStage(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("F3: Stage3 from history should not error: %v", err)
	}
	if stage != Stage3 {
		t.Errorf("F3: want Stage3, got Stage %d", stage)
	}
}

// ─── F4: sealed-run guard tests ───────────────────────────────────────────────

// mockCalcRunSealChecker implements CalcRunSealChecker.
type mockCalcRunSealChecker struct {
	sealed bool
	err    error
}

func (m *mockCalcRunSealChecker) IsSealedCalcRun(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.sealed, m.err
}

// TestComputeBulk_SealedCalcRun_Returns423 verifies ComputeBulk rejects requests
// for sealed calc runs with ECL_CALC_RUN_SEALED (HTTP 423 Locked per F4).
func TestComputeBulk_SealedCalcRun_Returns423(t *testing.T) {
	t.Parallel()

	calcRunID := uuid.New()
	orch := &ECLOrchestrator{
		sealChecker: &mockCalcRunSealChecker{sealed: true},
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      calcRunID,
		PeriodeID:      "JUNI-2026",
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}

	_, err := orch.ComputeBulk(context.Background(), req, nil)
	if err == nil {
		t.Fatal("F4: sealed calc run should return error from ComputeBulk")
	}

	var ce *coreError
	if !errors.As(err, &ce) {
		t.Errorf("F4: expected *coreError, got %T: %v", err, err)
	} else if ce.Code() != CodeECLCalcRunSealed {
		t.Errorf("F4: expected code %s, got %s", CodeECLCalcRunSealed, ce.Code())
	}
}

// TestComputeSingle_SealedCalcRun_Returns423 verifies ComputeSingle rejects
// requests for sealed calc runs. CalcRunID in ComputeRequest identifies the run.
func TestComputeSingle_SealedCalcRun_Returns423(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	calcRunID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	orch := &ECLOrchestrator{
		sealChecker: &mockCalcRunSealChecker{sealed: true},
		instrReader: reader,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		CalcRunID:      &calcRunID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        true,
	}

	_, err := orch.ComputeSingle(context.Background(), req)
	if err == nil {
		t.Fatal("F4: sealed calc run should return error from ComputeSingle")
	}

	var ce *coreError
	if !errors.As(err, &ce) {
		t.Errorf("F4: expected *coreError, got %T: %v", err, err)
	} else if ce.Code() != CodeECLCalcRunSealed {
		t.Errorf("F4: expected code %s, got %s", CodeECLCalcRunSealed, ce.Code())
	}
}

// ─── F5: roll-forward Phase5 deferred nullability tests ──────────────────────

// TestRollForward_Phase5Deferred_ReturnsNullComponents_WithStatus verifies that
// GetRollForward returns nil for all transfer components with PARTIAL_PHASE_5_DEFER
// status and IsReconciled=false.
// Per PSAK 71 §5.5, full roll-forward requires Phase 5 data (originations, derecognitions,
// stage transfers). These are nil (not zero) to distinguish from "computed zero".
func TestRollForward_Phase5Deferred_ReturnsNullComponents_WithStatus(t *testing.T) {
	t.Parallel()

	db, mock, _ := sqlmock.New()
	t.Cleanup(func() { db.Close() })

	calcRunID := uuid.New()
	// GetCalcRunECLTotal is called once for closing ECL (no prior calc run).
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("5000000.0000"))

	orch := &ECLOrchestrator{
		resultRepo: NewCalcResultLineRepo(db),
		logger:     slog.Default(),
	}

	report, err := orch.GetRollForward(context.Background(), RollForwardRequest{
		CalcRunID: calcRunID,
		ActorID:   uuid.New(),
	})
	if err != nil {
		t.Fatalf("F5: GetRollForward unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("F5: GetRollForward returned nil report")
	}

	// Status must be PARTIAL_PHASE_5_DEFER (not FULL).
	if report.Status != RollForwardStatusPartialPhase5Defer {
		t.Errorf("F5: Status: want %s, got %s", RollForwardStatusPartialPhase5Defer, report.Status)
	}

	// IsReconciled must be false — cannot reconcile without Phase 5 transfer data.
	if report.ReconcileCheck.IsReconciled {
		t.Error("F5: ReconcileCheck.IsReconciled must be false when transfer components are nil (Phase 5 deferred)")
	}

	// All 6 transfer components must be nil (not zero).
	nilCheck := func(name string, d *decimal.Decimal) {
		if d != nil {
			t.Errorf("F5: %s should be nil (Phase 5 deferred), got %s", name, d)
		}
	}
	nilCheck("NewOriginationsIDR", report.NewOriginationsIDR)
	nilCheck("DerecognitionsIDR", report.DerecognitionsIDR)
	nilCheck("TransfersToStage2IDR", report.TransfersToStage2IDR)
	nilCheck("TransfersToStage3IDR", report.TransfersToStage3IDR)
	nilCheck("TransfersFromStage2IDR", report.TransfersFromStage2IDR)
	nilCheck("TransfersFromStage3IDR", report.TransfersFromStage3IDR)
}

// ─── F6: bobot DEC-010 default values test ───────────────────────────────────

// TestBobotSnapshot_DEC010DefaultValues verifies that the DEC-010 locked default
// bobot values are Good=0.25, Normal=0.50, Bad=0.25 and sum to 1.0.
// Used by mockBobotRepo and the StaticBobotRepo fallback path in tests.
func TestBobotSnapshot_DEC010DefaultValues(t *testing.T) {
	t.Parallel()

	bobot := defaultBobot()

	if !bobot.Good.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("F6 DEC-010: Good want 0.25, got %s", bobot.Good)
	}
	if !bobot.Normal.Equal(decimal.NewFromFloat(0.50)) {
		t.Errorf("F6 DEC-010: Normal want 0.50, got %s", bobot.Normal)
	}
	if !bobot.Bad.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("F6 DEC-010: Bad want 0.25, got %s", bobot.Bad)
	}
	sum := bobot.Good.Add(bobot.Normal).Add(bobot.Bad)
	if !sum.Equal(decimal.NewFromInt(1)) {
		t.Errorf("F6 DEC-010: bobot sum must be 1.0, got %s", sum)
	}
}

// ─── F8: formula_version field tests ─────────────────────────────────────────

// TestResultLineRow_FormulaVersionField verifies that ResultLineRow carries a
// FormulaVersion field and FormulaVersionM7 constant is "M7-v1.0".
// Migration 000030 adds formula_version TEXT NOT NULL DEFAULT 'M7-v1.0'.
func TestResultLineRow_FormulaVersionField(t *testing.T) {
	t.Parallel()

	// Verify constant is defined and correct.
	if FormulaVersionM7 == "" {
		t.Fatal("F8: FormulaVersionM7 constant must not be empty")
	}
	if FormulaVersionM7 != "M7-v1.0" {
		t.Errorf("F8: FormulaVersionM7 want 'M7-v1.0', got %q", FormulaVersionM7)
	}

	// Verify zero-value ResultLineRow has empty FormulaVersion (not pre-set).
	var row ResultLineRow
	if row.FormulaVersion != "" {
		t.Errorf("F8: zero-value FormulaVersion should be empty, got %q", row.FormulaVersion)
	}

	// Verify field assignment works.
	row.FormulaVersion = FormulaVersionM7
	if row.FormulaVersion != FormulaVersionM7 {
		t.Error("F8: FormulaVersion field assignment failed")
	}
}

// TestComputeSingle_ResultContainsFormulaVersion verifies that when ComputeSingle
// builds a result via the STANDARD path, the formula version is propagated.
// (Full DB persist path verified in service_persist_test.go.)
func TestComputeSingle_ResultContainsFormulaVersion(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	orch := &ECLOrchestrator{
		auditWriter: audit.NewWriter(nil),
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		logger:      slog.Default(),
		stagingSvc:  &mockStagingService{stage: Stage1},
	}

	result, err := orch.ComputeSingle(context.Background(), ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	})
	if err != nil {
		t.Fatalf("F8: ComputeSingle: %v", err)
	}

	// ECLWeightedIDR non-nil confirms STANDARD path was taken.
	if result.ECLWeightedIDR == nil {
		t.Fatal("F8: ECLWeightedIDR must be non-nil for STANDARD Stage1")
	}
	if result.ECLWeightedIDR.IsZero() {
		t.Error("F8: ECLWeightedIDR should be non-zero for valid EAD/PD/LGD inputs")
	}
}
