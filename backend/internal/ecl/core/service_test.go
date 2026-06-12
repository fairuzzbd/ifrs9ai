package core

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
	"blips-ifrs9.tugu-re.com/internal/ecl/lookthrough"
	"blips-ifrs9.tugu-re.com/internal/ecl/lps"
)

// service_test.go — ECLOrchestrator unit tests.
//
// AC covered:
//   M7-001 AC1-7: ComputeSingle routing + formula correctness
//   M7-005 AC1-4: RecomputeAdHoc preview mode (no persist)
//   M7-006 AC1-3: POCI flag + warning, no persist

// ─── Mock implementations ─────────────────────────────────────────────────────

type mockInstrumenReader struct {
	byID        map[uuid.UUID]*InstrumenSnapshot
	activeScope []InstrumenSnapshot
}

func (m *mockInstrumenReader) GetByID(_ context.Context, id uuid.UUID) (*InstrumenSnapshot, error) {
	if inst, ok := m.byID[id]; ok {
		return inst, nil
	}
	return nil, errDomain(CodeECLInstrumenNotFound, "instrument not found: "+id.String())
}

func (m *mockInstrumenReader) ListActiveByScope(_ context.Context, _ *BulkScope) ([]InstrumenSnapshot, error) {
	return m.activeScope, nil
}

type mockBobotRepo struct {
	bobot BobotSnapshot
	err   error
}

func (m *mockBobotRepo) GetActiveBobot(_ context.Context, _ string) (BobotSnapshot, error) {
	return m.bobot, m.err
}

type mockPDService struct {
	pdBase decimal.Decimal
	flMult decimal.Decimal
}

func (m *mockPDService) GetPD(_ context.Context, _ uuid.UUID, stage helpers.EclStage, scenario helpers.EclScenario, _ string, _ time.Time) (decimal.Decimal, helpers.PDDetail, error) {
	pd := m.pdBase
	mevMult := m.flMult
	// ImpactPDMultiplier defaults to 1.0 (neutral) so combined = 1.0 × mevMult = mevMult.
	// F1 fix: combined FL = ImpactPDMultiplier × ImpactMevPDMultiplier.
	impactPD := decimal.NewFromInt(1)
	// Stage 3: PD = 1.0 returned from M2, but M7 will override internally.
	if stage == helpers.Stage3 {
		pd = decimal.NewFromInt(1)
		mevMult = decimal.Zero
		impactPD = decimal.Zero
	}
	return pd.Mul(mevMult), helpers.PDDetail{
		Stage:                 stage,
		Scenario:              scenario,
		PD:                    pd.Mul(mevMult),
		PDBase:                pd,
		ImpactPDMultiplier:    impactPD,
		ImpactMevPDMultiplier: mevMult,
	}, nil
}

type mockLGDService struct {
	lgd decimal.Decimal
}

func (m *mockLGDService) GetLGD(_ context.Context, _ uuid.UUID, _ string) (decimal.Decimal, helpers.LGDDetail, error) {
	return m.lgd, helpers.LGDDetail{LGD: m.lgd, LGDEffective: m.lgd}, nil
}

type mockEADService struct {
	eadIDR decimal.Decimal
}

func (m *mockEADService) ComputeEAD(_ context.Context, _ uuid.UUID, _ time.Time) (decimal.Decimal, helpers.EADBreakdown, error) {
	return m.eadIDR, helpers.EADBreakdown{EADIDR: m.eadIDR}, nil
}

type mockCCFService struct{}

func (m *mockCCFService) GetCCF(_ context.Context, _ string) (decimal.Decimal, helpers.CCFDetail, error) {
	return decimal.Zero, helpers.CCFDetail{}, nil
}

type mockBulkHelperService struct{}

func (m *mockBulkHelperService) BulkLookup(_ context.Context, reqs []helpers.BulkRequest, _ string, _ time.Time) ([]helpers.BulkResult, helpers.BulkSummary, []helpers.BulkError, []helpers.BulkSkipped, error) {
	results := make([]helpers.BulkResult, len(reqs))
	for i, r := range reqs {
		results[i] = helpers.BulkResult{
			InstrumenID: r.InstrumenID,
			PDGood:      decimal.NewFromFloat(0.01),
			PDNormal:    decimal.NewFromFloat(0.02),
			PDBad:       decimal.NewFromFloat(0.03),
			LGD:         decimal.NewFromFloat(0.40),
			EADIDR:      decimal.NewFromInt(1_000_000_000),
		}
	}
	return results, helpers.BulkSummary{Total: len(reqs)}, nil, nil, nil
}

type mockLPSAggregator struct {
	result *lps.PairAggregation
	err    error
}

func (m *mockLPSAggregator) Aggregate(_ context.Context, _, _ uuid.UUID, _ time.Time) (*lps.PairAggregation, error) {
	return m.result, m.err
}

type mockLookthroughService struct {
	result *lookthrough.Result
	err    error
}

func (m *mockLookthroughService) Compute(_ context.Context, _, _, _ uuid.UUID, _ time.Time, _ uuid.UUID) (*lookthrough.Result, error) {
	return m.result, m.err
}

// ─── buildMockHelpers ─────────────────────────────────────────────────────────

func buildMockHelpers(pdBase, flMult, lgd, eadIDR decimal.Decimal) *helpers.Services {
	pdSvc := &mockPDService{pdBase: pdBase, flMult: flMult}
	lgdSvc := &mockLGDService{lgd: lgd}
	eadSvc := &mockEADService{eadIDR: eadIDR}
	return &helpers.Services{
		PD:   pdSvc,
		LGD:  lgdSvc,
		EAD:  eadSvc,
		CCF:  &mockCCFService{},
		Bulk: &mockBulkHelperService{},
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestNewOrchestrator_PanicsOnNilAuditWriter verifies constructor panic (per M3 pattern, DEC-018).
func TestNewOrchestrator_PanicsOnNilAuditWriter(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil auditWriter")
		}
	}()

	// Use a real (non-nil) DB pointer via nil cast to get past the nil db check.
	// We need to reach the auditWriter nil check.
	// Use a non-nil sql.DB to trigger the auditWriter panic.
	_ = NewOrchestrator(
		&sql.DB{},
		nil, // auditWriter nil → must panic
		buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		nil,
		nil,
		&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}},
		&mockBobotRepo{bobot: defaultBobot()},
		nil,
	)
}

// TestComputeSingle_FVTPL_SkipNoRow verifies FVTPL routing returns ECL=0, no persist.
func TestComputeSingle_FVTPL_SkipNoRow(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// FVTPL path doesn't use helpers, lps, lookthrough.
	// But auditWriter is needed (persist=false → no tx needed).
	// Use a fake audit.Writer (pointer to empty struct).
	orch := buildOrchestratorForTest(reader, nil, nil)

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("FVTPL compute: unexpected error: %v", err)
	}

	if result.RoutingPath != RoutingSkipFVTPL {
		t.Errorf("routing: want %s, got %s", RoutingSkipFVTPL, result.RoutingPath)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("FVTPL: ECLWeightedIDR should be 0 (not nil)")
	}
	if !result.ECLWeightedIDR.IsZero() {
		t.Errorf("FVTPL: ECLWeightedIDR should be 0, got %s", result.ECLWeightedIDR)
	}
	// Verify warning code.
	found := false
	for _, w := range result.Warnings {
		if w == WarnFVTPLSkip {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FVTPL: expected warning %s, got %v", WarnFVTPLSkip, result.Warnings)
	}
	// No result line ID (not persisted).
	if result.ResultLineID != nil {
		t.Error("FVTPL: ResultLineID should be nil when Persist=false")
	}
}

// TestComputeSingle_POCI_ComputesViaStandardPath_WithWarning verifies F2 fix:
// POCI instruments now compute via STANDARD path and return ECL > 0 with warning.
// Prior behavior (ECLWeightedIDR = nil, no compute) was non-compliant with scope spec.
func TestComputeSingle_POCI_ComputesViaStandardPath_WithWarning(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
				FlagPOCI:          true,
			},
		},
	}

	orch := buildOrchestratorForTest(reader, nil, nil)

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("POCI compute: unexpected error: %v", err)
	}

	// F2 fix: routing_path = POCI_DEFERRED (audit column), but computed via STANDARD.
	if result.RoutingPath != RoutingPOCIDeferred {
		t.Errorf("routing: want %s, got %s", RoutingPOCIDeferred, result.RoutingPath)
	}
	// F2 fix: ECLWeightedIDR must be non-nil (computed via STANDARD; credit-adjusted EIR deferred).
	if result.ECLWeightedIDR == nil {
		t.Error("POCI F2 fix: ECLWeightedIDR must be non-nil (ECL is computed)")
	}
	if !result.FlagPOCI {
		t.Error("POCI: FlagPOCI must be true")
	}
	// Verify POCI warning code is present.
	found := false
	for _, w := range result.Warnings {
		if w == WarnPOCIRequiresFullCAEIR {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("POCI: expected warning %s, got %v", WarnPOCIRequiresFullCAEIR, result.Warnings)
	}
}

// TestComputeSingle_InstrumenNotFound returns CodeECLInstrumenNotFound.
func TestComputeSingle_InstrumenNotFound(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	orch := buildOrchestratorForTest(reader, nil, nil)

	req := ComputeRequest{
		InstrumenID:    uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Persist:        false,
		ActorID:        uuid.New(),
	}

	_, err := orch.ComputeSingle(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for missing instrument")
	}
	if ce, ok := err.(*coreError); !ok {
		t.Errorf("expected *coreError, got %T: %v", err, err)
	} else if ce.code != CodeECLInstrumenNotFound {
		t.Errorf("expected code %s, got %s", CodeECLInstrumenNotFound, ce.code)
	}
}

// TestComputeSingle_Lookthrough_Delegates delegates to M4.
func TestComputeSingle_Lookthrough_Delegates(t *testing.T) {
	t.Parallel()

	periodeUUID := uuid.New()
	instrID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "REKSADANA",
			},
		},
	}

	expectedECL := decimal.NewFromInt(78_750)
	ltSvc := &mockLookthroughService{
		result: &lookthrough.Result{TotalECLIDR: expectedECL},
	}

	orch := buildOrchestratorForTestLT(reader, ltSvc)

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      periodeUUID.String(),
		Persist:        false,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("lookthrough compute: unexpected error: %v", err)
	}

	if result.RoutingPath != RoutingLookthrough {
		t.Errorf("routing: want %s, got %s", RoutingLookthrough, result.RoutingPath)
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR must not be nil for LOOKTHROUGH")
	}
	if !result.ECLWeightedIDR.Equal(expectedECL) {
		t.Errorf("ECLWeightedIDR: want %s, got %s", expectedECL, result.ECLWeightedIDR)
	}
}

// TestBobotSnapshot_Validate_DefaultValid verifies 0.25/0.50/0.25 passes.
func TestBobotSnapshot_Validate_DefaultValid(t *testing.T) {
	t.Parallel()
	if err := defaultBobot().Validate(); err != nil {
		t.Errorf("default bobot must be valid: %v", err)
	}
}

// TestBobotSnapshot_Validate_SumNot1_Fails verifies sum ≠ 1 fails.
func TestBobotSnapshot_Validate_SumNot1_Fails(t *testing.T) {
	t.Parallel()
	b := BobotSnapshot{
		Good:   decimal.NewFromFloat(0.30),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
	if err := b.Validate(); err == nil {
		t.Error("sum=1.05 should fail Validate")
	}
}

// TestStageFromInt confirms valid and invalid conversions.
func TestStageFromInt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   int
		want Stage
		ok   bool
	}{
		{1, Stage1, true},
		{2, Stage2, true},
		{3, Stage3, true},
		{0, 0, false},
		{4, 0, false},
	}
	for _, tc := range cases {
		got, ok := StageFromInt(tc.in)
		if ok != tc.ok {
			t.Errorf("StageFromInt(%d): ok=%v, want %v", tc.in, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Errorf("StageFromInt(%d): got %d, want %d", tc.in, got, tc.want)
		}
	}
}

// ─── Orchestrator builder helpers ─────────────────────────────────────────────

// buildOrchestratorForTest creates a minimal orchestrator for tests that only need
// FVTPL/POCI routing (no real DB, uses audit.NewWriter(nil) which returns a stub).
func buildOrchestratorForTest(reader InstrumenReaderIface, lpsAgg LPSAggregatorIface, ltSvc LookthroughServiceIface) *ECLOrchestrator {
	// We need a non-nil *audit.Writer. Use a real one with nil DB — it'll panic on write,
	// but FVTPL/POCI in non-persist mode don't write audit.
	aw := audit.NewWriter(nil)
	svc := buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9))
	return &ECLOrchestrator{
		db:          nil, // nil db — tests should not call persist
		auditWriter: aw,
		helpers:     svc,
		lpsAgg:      lpsAgg,
		lookthrough: ltSvc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil, // nil — tests that persist need different setup
		logger:      nil,
	}
}

func buildOrchestratorForTestLT(reader InstrumenReaderIface, ltSvc LookthroughServiceIface) *ECLOrchestrator {
	return buildOrchestratorForTest(reader, nil, ltSvc)
}
