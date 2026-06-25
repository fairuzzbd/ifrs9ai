package core

// service_standard_test.go — tests for STANDARD and LPS routing paths
// without requiring a real database (Persist=false).
//
// These tests drive coverage of:
//   - handleStandard → applyFormulaAndPersist (Stage 1, Stage 2)
//   - handleLPS fully-covered case (ExcessIDR = 0, ECL = 0)
//   - handleLPS partial excess case
//   - resolveStage helper
//   - stageToM2 helper
//   - ComputeBulk with zero instruments
//   - NewBulkWorker panic test
//   - reportProgress

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
	"blips-ifrs9.tugu-re.com/internal/ecl/lps"
)

// TestComputeSingle_Standard_Stage1_NoPersist verifies STANDARD path Stage1
// computes ECL without touching the database (Persist=false).
//
// Setup: EAD=1B, PD=0.01, FL=1.0, LGD=0.4, bobot default (0.25/0.50/0.25).
// ECL_skenario = 1_000_000_000 × 0.01 × 0.4 = 4_000_000
// ECL_FL_skenario = 4_000_000 × 1.0 = 4_000_000 (all scenarios same)
// ECL_weighted = 4_000_000 × (0.25+0.50+0.25) = 4_000_000.0000
func TestComputeSingle_Standard_Stage1_NoPersist(t *testing.T) {
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

	// PD=0.01, FL multiplier=1.0, LGD=0.40, EAD=1B.
	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil, // persist=false → no audit write
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.40), decimal.NewFromInt(1_000_000_000)),
		lpsAgg:      nil,
		lookthrough: nil,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("STANDARD Stage1: unexpected error: %v", err)
	}

	if result.RoutingPath != RoutingStandard {
		t.Errorf("routingPath: want STANDARD, got %s", result.RoutingPath)
	}
	if result.Stage != Stage1 {
		t.Errorf("stage: want 1, got %d", result.Stage)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR: must not be nil for STANDARD")
	}

	// ECL_weighted = EAD × PD × LGD × 1.0 = 1_000_000_000 × 0.01 × 0.40 = 4_000_000.0000
	want := decimal.NewFromFloat(4_000_000.0)
	if !result.ECLWeightedIDR.Equal(want) {
		t.Errorf("ECLWeightedIDR: want %s, got %s", want, result.ECLWeightedIDR)
	}

	// Verify no persistence (ResultLineID nil when Persist=false).
	if result.ResultLineID != nil {
		t.Error("ResultLineID must be nil when Persist=false")
	}

	// Verify FL multipliers populated (Stage1 non-nil).
	if result.FLMultiplierPerScenario == nil {
		t.Error("FLMultiplierPerScenario: must be non-nil for Stage 1")
	}
}

// TestComputeSingle_Standard_Stage2_NoPersist verifies STANDARD path Stage2.
// Stage2: lifetime PD (mock returns same PD as Stage1 for this test).
func TestComputeSingle_Standard_Stage2_NoPersist(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVOCI",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// Use a PD service that returns Stage2.
	pdSvc := &mockPDServiceStage{
		pdBase: decimal.NewFromFloat(0.05),
		flMult: decimal.NewFromFloat(1.10),
		stage:  helpers.Stage2,
	}
	svc := &helpers.Services{
		PD:   pdSvc,
		LGD:  &mockLGDService{lgd: decimal.NewFromFloat(0.50)},
		EAD:  &mockEADService{eadIDR: decimal.NewFromInt(500_000_000)},
		CCF:  &mockCCFService{},
		Bulk: &mockBulkHelperService{},
	}

	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("STANDARD Stage2: unexpected error: %v", err)
	}

	if result.Stage != Stage2 {
		t.Errorf("stage: want 2, got %d", result.Stage)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR: must not be nil for STANDARD Stage2")
	}

	// ECL_skenario = 500_000_000 × 0.05 × 0.50 = 12_500_000
	// ECL_FL = 12_500_000 × 1.10 = 13_750_000
	// ECL_weighted = 13_750_000 × (0.25 + 0.50 + 0.25) = 13_750_000
	want := decimal.NewFromFloat(13_750_000.0)
	if !result.ECLWeightedIDR.Equal(want) {
		t.Errorf("ECLWeightedIDR: want %s, got %s", want, result.ECLWeightedIDR)
	}
}

// mockPDServiceStage is a PD service that always returns the specified stage.
type mockPDServiceStage struct {
	pdBase decimal.Decimal
	flMult decimal.Decimal
	stage  helpers.EclStage
}

func (m *mockPDServiceStage) GetPD(_ context.Context, _ uuid.UUID, _ helpers.EclStage, _ helpers.EclScenario, _ string, _ time.Time, _ ...bool) (decimal.Decimal, helpers.PDDetail, error) {
	// ImpactPDMultiplier defaults to 1.0 so combined FL = 1.0 × flMult = flMult.
	// F1 fix: combined FL = ImpactPDMultiplier × ImpactMevPDMultiplier.
	return m.pdBase.Mul(m.flMult), helpers.PDDetail{
		Stage:                 m.stage,
		PD:                    m.pdBase.Mul(m.flMult),
		PDBase:                m.pdBase,
		ImpactPDMultiplier:    decimal.NewFromInt(1),
		ImpactMevPDMultiplier: m.flMult,
	}, nil
}

// TestComputeSingle_LPS_FullyCovered_ECLZero verifies LPS path where entire
// exposure is within the IDR 2B cap → ECL = 0.
func TestComputeSingle_LPS_FullyCovered_ECLZero(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	nasabahID := uuid.New()
	counterpartyID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "DEPOSITO",
				NasabahID:         nasabahID,
				CounterpartyID:    counterpartyID,
			},
		},
	}

	// ExcessIDR = 0 → fully covered → ECL = 0.
	lpsAgg := &mockLPSAggregator{
		result: &lps.PairAggregation{
			TotalExposureIDR: decimal.NewFromInt(1_000_000_000), // below cap
			CoveredIDR:       decimal.NewFromInt(1_000_000_000),
			ExcessIDR:        decimal.Zero,
		},
	}

	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1_000_000_000)),
		lpsAgg:      lpsAgg,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("LPS fully covered: unexpected error: %v", err)
	}

	if result.RoutingPath != RoutingLPS {
		t.Errorf("routingPath: want LPS, got %s", result.RoutingPath)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR: must not be nil")
	}
	if !result.ECLWeightedIDR.IsZero() {
		t.Errorf("ECLWeightedIDR: want 0 (fully covered), got %s", result.ECLWeightedIDR)
	}
}

// TestComputeSingle_LPS_WithExcess verifies LPS path where excess > 0
// applies standard formula on excess portion only.
func TestComputeSingle_LPS_WithExcess(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	nasabahID := uuid.New()
	cpID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "DEPOSITO",
				NasabahID:         nasabahID,
				CounterpartyID:    cpID,
			},
		},
	}

	// ExcessIDR = 500_000_000 (above LPS cap).
	excess := decimal.NewFromInt(500_000_000)
	lpsAgg := &mockLPSAggregator{
		result: &lps.PairAggregation{
			TotalExposureIDR: decimal.NewFromInt(2_500_000_000),
			CoveredIDR:       decimal.NewFromInt(2_000_000_000),
			ExcessIDR:        excess,
		},
	}

	// PD=0.02, FL=1.0, LGD=0.4.
	// ECL = 500_000_000 × 0.02 × 0.4 = 4_000_000
	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), excess),
		lpsAgg:      lpsAgg,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("LPS excess: unexpected error: %v", err)
	}

	if result.RoutingPath != RoutingLPS {
		t.Errorf("routingPath: want LPS, got %s", result.RoutingPath)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR: must not be nil for LPS excess")
	}

	// ECL = 500_000_000 × 0.02 × 0.40 × 1.0 = 4_000_000.0000
	want := decimal.NewFromFloat(4_000_000.0)
	if !result.ECLWeightedIDR.Equal(want) {
		t.Errorf("ECLWeightedIDR: want %s, got %s", want, result.ECLWeightedIDR)
	}
}

// TestComputeSingle_LPS_NilAgg_FallbackStandard verifies LPS falls back to STANDARD
// when lpsAgg is nil (e.g., not wired in test environment).
func TestComputeSingle_LPS_NilAgg_FallbackStandard(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "DEPOSITO",
			},
		},
	}

	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1_000_000_000)),
		lpsAgg:      nil, // no LPS service → fallback to STANDARD
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("LPS nil agg fallback: unexpected error: %v", err)
	}

	// Falls back to STANDARD compute, ECL non-nil.
	if result.ECLWeightedIDR == nil {
		t.Error("ECLWeightedIDR: must not be nil when LPS falls back to STANDARD")
	}
}

// TestStageToM2 verifies the stageToM2 helper covers all three stage values.
func TestStageToM2(t *testing.T) {
	t.Parallel()

	cases := []struct {
		stage Stage
		want  helpers.EclStage
	}{
		{Stage1, helpers.Stage1},
		{Stage2, helpers.Stage2},
		{Stage3, helpers.Stage3},
		{Stage(99), helpers.Stage1}, // unknown → default Stage1
	}
	for _, tc := range cases {
		got := stageToM2(tc.stage)
		if got != tc.want {
			t.Errorf("stageToM2(%d): want %v, got %v", tc.stage, tc.want, got)
		}
	}
}

// TestComputeBulk_ZeroInstruments verifies bulk with empty scope returns completed status.
func TestComputeBulk_ZeroInstruments(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{},
		activeScope: []InstrumenSnapshot{}, // empty list
	}

	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		ActorID:        uuid.New(),
	}

	var progressCalls []int
	progressFn := func(processed, total int, _ string) {
		progressCalls = append(progressCalls, processed)
	}

	result, err := orch.ComputeBulk(context.Background(), req, progressFn)
	if err != nil {
		t.Fatalf("ComputeBulk zero instruments: unexpected error: %v", err)
	}
	if result.TotalScanned != 0 {
		t.Errorf("TotalScanned: want 0, got %d", result.TotalScanned)
	}
	if result.Status != "completed" {
		t.Errorf("status: want completed, got %s", result.Status)
	}
	// Progress called at start (progress=0) and completion.
	if len(progressCalls) == 0 {
		t.Error("progressFn should be called at least once (start + end)")
	}
}

// TestComputeBulk_TooMany verifies ECL_BULK_TOO_LARGE returned when scope > 10_000.
func TestComputeBulk_TooMany(t *testing.T) {
	t.Parallel()

	// Build a reader that returns 10_001 instruments.
	instruments := make([]InstrumenSnapshot, bulkMaxInstruments+1)
	for i := range instruments {
		instruments[i] = InstrumenSnapshot{
			ID:                uuid.New(),
			KlasifikasiPsak71: "FVTPL",
			TipeInstrumen:     "OBLIGASI",
		}
	}
	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{},
		activeScope: instruments,
	}

	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
	}

	_, err := orch.ComputeBulk(context.Background(), req, nil)
	if err == nil {
		t.Fatal("ComputeBulk TooMany: expected ECL_BULK_TOO_LARGE error, got nil")
	}

	ce, ok := err.(*coreError)
	if !ok {
		t.Fatalf("ComputeBulk TooMany: want *coreError, got %T", err)
	}
	if ce.code != CodeECLBulkTooLarge {
		t.Errorf("error code: want %s, got %s", CodeECLBulkTooLarge, ce.code)
	}
}

// TestNewBulkWorker_PanicsOnNilOrchestrator verifies constructor panic.
func TestNewBulkWorker_PanicsOnNilOrchestrator(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil orchestrator")
		}
	}()
	_ = NewBulkWorker(nil, nil, nil)
}

// TestNewBulkWorker_OK verifies non-nil creation.
func TestNewBulkWorker_OK(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	orch := buildOrchestratorForTest(reader, nil, nil)

	var progressCalledWith []int
	pFn := func(processed, total int, _ string) {
		progressCalledWith = append(progressCalledWith, processed)
	}

	w := NewBulkWorker(orch, pFn, nil)
	if w == nil {
		t.Error("NewBulkWorker: must not return nil")
	}
}

// TestReportProgress verifies reportProgress calls progressFn at boundaries.
func TestReportProgress(t *testing.T) {
	t.Parallel()

	calls := 0
	pFn := func(_, _ int, _ string) { calls++ }

	// Exactly at progressReportEvery (50).
	reportProgress(pFn, 50, 100)
	if calls != 1 {
		t.Errorf("want 1 call at 50, got %d", calls)
	}

	// Not at boundary (49 is not divisible by 50).
	reportProgress(pFn, 49, 100)
	if calls != 1 {
		t.Errorf("want 1 call (49 not at boundary), got %d", calls)
	}

	// At total (processed == total).
	reportProgress(pFn, 100, 100)
	if calls != 2 {
		t.Errorf("want 2 calls (at total), got %d", calls)
	}

	// nil progressFn: no-op.
	reportProgress(nil, 50, 100)
}

// TestComputeSingle_InstrumenNotFound verifies error propagation from instrReader.
func TestComputeSingle_InstrumenNotFound_Standard(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	orch := buildOrchestratorForTest(reader, nil, nil)

	req := ComputeRequest{
		InstrumenID:    uuid.New(), // not in reader
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
	}

	_, err := orch.ComputeSingle(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unknown instrumenID")
	}
	ce, ok := err.(*coreError)
	if !ok {
		t.Fatalf("want *coreError, got %T: %v", err, err)
	}
	if ce.code != CodeECLInstrumenNotFound {
		t.Errorf("error code: want %s, got %s", CodeECLInstrumenNotFound, ce.code)
	}
}

// TestCoreError_ErrorAndCode cover the coreError type methods.
func TestCoreError_ErrorAndCode(t *testing.T) {
	t.Parallel()

	ce := errDomain(CodeECLCalcRunSealed, "sealed")
	// Error() = code + ": " + message.
	if ce.Error() != CodeECLCalcRunSealed+": sealed" {
		t.Errorf("Error(): want %q, got %q", CodeECLCalcRunSealed+": sealed", ce.Error())
	}
	if ce.Code() != CodeECLCalcRunSealed {
		t.Errorf("Code(): want %s, got %s", CodeECLCalcRunSealed, ce.Code())
	}
}

// TestBobotSnapshot_Sum_Validate cover the helpers.
func TestBobotSnapshot_Sum_Validate_Extended(t *testing.T) {
	t.Parallel()

	// Valid bobot.
	b := defaultBobot()
	sum := b.Sum()
	if !sum.Equal(decimal.NewFromInt(1)) {
		t.Errorf("Sum: want 1, got %s", sum)
	}
	if err := b.Validate(); err != nil {
		t.Errorf("Validate valid: unexpected error: %v", err)
	}

	// Invalid: sum ≠ 1.
	bad := BobotSnapshot{
		Good:   decimal.NewFromFloat(0.50),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.50),
	}
	if err := bad.Validate(); err == nil {
		t.Error("Validate invalid: expected error for sum ≠ 1")
	}
}

// ─── Phase 4.5 POCI ECL compute tests — DEC-POCI-001..003 ───────────────────

// TestECLCompute_POCI_ComputesViaStandardWithBaseline verifies that a POCI instrument
// with flag_poci=true is routed via RoutingPOCIComputed (Phase 4.5), computes non-nil
// ECL (initial baseline), and carries both POCI warning codes.
//
// Per DEC-POCI-001: CA-EIR is computed; ECL = initial baseline lifetime ECL.
// Per DEC-POCI-002: jurnal P&L booking deferred to Phase 5.
func TestECLCompute_POCI_ComputesViaStandardWithBaseline(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
				FlagPOCI:          true,
				HasCAEIRSchedule:  true, // CA-EIR present → POCI_COMPUTED routing (F2 fix)
				TenantID:          "TUGURE",
			},
		},
	}

	orch := buildOrchestratorForTest(reader, nil, nil)

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "PBUKU-2026-06",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("POCI ComputeSingle: unexpected error: %v", err)
	}

	// RoutingPath must be POCI_COMPUTED (Phase 4.5 — CA-EIR available).
	if result.RoutingPath != RoutingPOCIComputed {
		t.Errorf("RoutingPath: want %s, got %s", RoutingPOCIComputed, result.RoutingPath)
	}

	// FlagPOCI must be true.
	if !result.FlagPOCI {
		t.Error("FlagPOCI must be true for POCI instrument")
	}

	// ECLWeightedIDR must be non-nil and > 0 (initial baseline ECL computed via STANDARD).
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR must be non-nil for POCI Phase 4.5 (initial baseline)")
	}
	if result.ECLWeightedIDR.IsZero() {
		t.Error("ECLWeightedIDR must be > 0 for POCI with non-zero PD (mock returns 0.02)")
	}

	// P5-M10: WarnPOCIRequiresFullCAEIR must be present.
	// WarnPOCIECLRepresentsInitialBaseline must NOT be present (removed in P5-M10).
	foundCAEIR := false
	for _, w := range result.Warnings {
		if w == WarnPOCIRequiresFullCAEIR {
			foundCAEIR = true
		}
		if w == WarnPOCIECLRepresentsInitialBaseline {
			t.Errorf("POCI: WarnPOCIECLRepresentsInitialBaseline must NOT be emitted in P5-M10. Got warnings: %v", result.Warnings)
		}
	}
	if !foundCAEIR {
		t.Errorf("POCI: expected warning %s, got %v", WarnPOCIRequiresFullCAEIR, result.Warnings)
	}

	t.Logf("POCI ECL baseline: %s, routing: %s, warnings: %v",
		result.ECLWeightedIDR.StringFixed(4), result.RoutingPath, result.Warnings)
}

// TestECLCompute_POCI_DetermineRouting_StillReturnsPOCIDeferred verifies that
// DetermineRouting still maps flag_poci=true → RoutingPOCIDeferred.
// The POCI_COMPUTED override is applied at the service layer (handlePOCI), not in routing.
// This preserves backward compat for instruments without a CA-EIR schedule.
func TestECLCompute_POCI_DetermineRouting_StillReturnsPOCIDeferred(t *testing.T) {
	t.Parallel()

	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "OBLIGASI",
		FlagPOCI:          true,
	}
	routing := DetermineRouting(inst)
	if routing != RoutingPOCIDeferred {
		t.Errorf("DetermineRouting: want %s, got %s", RoutingPOCIDeferred, routing)
	}
}
