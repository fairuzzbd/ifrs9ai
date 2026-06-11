package lookthrough

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Mock implementations ─────────────────────────────────────────────────────

// mockAuditWriter implements AuditWriterIface. Captures calls for assertion.
type mockAuditWriter struct {
	events []AuditEvent
	err    error // if set, returns this on Write
}

func (m *mockAuditWriter) Write(_ context.Context, _ *sql.Tx, evt AuditEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, evt)
	return nil
}

// mockFundCompositionRepo implements FundCompositionRepo.
type mockFundCompositionRepo struct {
	// GetByID response
	composition *FundComposition
	getByIDErr  error
	// GetActive response
	activeComp   *FundComposition
	getActiveErr error
	// GetDetails response
	details    []FundCompositionDetail
	detailsErr error
	// Create call tracking
	createCalled bool
	createErr    error
	// UpdateWorkflowStatus tracking
	updateCalled bool
	updateErr    error
	// SupersedeOld tracking
	supersedeCalled bool
	supersedeErr    error
	// GetInstrumenTipeAndKlasifikasi response
	tipe        string
	klasifikasi string
	pociFlag    bool
	tipeErr     error
	// ListByInstrumen response
	compositions []FundComposition
	listErr      error
}

func (m *mockFundCompositionRepo) Create(_ context.Context, _ *sql.Tx, _ *FundComposition, _ []FundCompositionDetail) error {
	m.createCalled = true
	return m.createErr
}

func (m *mockFundCompositionRepo) GetByID(_ context.Context, _ uuid.UUID) (*FundComposition, error) {
	return m.composition, m.getByIDErr
}

func (m *mockFundCompositionRepo) GetDetailsForComposition(_ context.Context, _ uuid.UUID) ([]FundCompositionDetail, error) {
	return m.details, m.detailsErr
}

func (m *mockFundCompositionRepo) GetActiveForInstrumen(_ context.Context, _ uuid.UUID, _ time.Time) (*FundComposition, error) {
	return m.activeComp, m.getActiveErr
}

func (m *mockFundCompositionRepo) ListByInstrumen(_ context.Context, _ uuid.UUID, _ string, _ string, _ int, _ string, _ string) ([]FundComposition, string, bool, error) {
	return m.compositions, "", false, m.listErr
}

func (m *mockFundCompositionRepo) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx,
	_ uuid.UUID, _ WorkflowStatus,
	_ *uuid.UUID, _ *time.Time, _ []byte, _ *string,
	_ *uuid.UUID, _ *time.Time, _ []byte, _ *string,
	_ *string, _ uuid.UUID,
) error {
	m.updateCalled = true
	return m.updateErr
}

func (m *mockFundCompositionRepo) SupersedeOld(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ time.Time, _ uuid.UUID) error {
	m.supersedeCalled = true
	return m.supersedeErr
}

func (m *mockFundCompositionRepo) GetInstrumenTipeAndKlasifikasi(_ context.Context, _ uuid.UUID) (string, string, bool, error) {
	return m.tipe, m.klasifikasi, m.pociFlag, m.tipeErr
}

// mockReksadanaRepo implements ReksadanaInstrumenRepo.
type mockReksadanaRepo struct {
	inst    *InstrumenReksadanaRow
	getErr  error
	bulk    []InstrumenReksadanaRow
	bulkErr error
}

func (m *mockReksadanaRepo) GetByID(_ context.Context, _ uuid.UUID) (*InstrumenReksadanaRow, error) {
	return m.inst, m.getErr
}

func (m *mockReksadanaRepo) BulkListReksadanaForECL(_ context.Context, _ string) ([]InstrumenReksadanaRow, error) {
	return m.bulk, m.bulkErr
}

// mockPDLGDRepo implements PDLGDClassRepo.
type mockPDLGDRepo struct {
	params map[AssetClass]PDLGDParams
	err    error
}

func (m *mockPDLGDRepo) GetPDLGDForAssetClass(_ context.Context, ac AssetClass, _ time.Time, _ string) (PDLGDParams, error) {
	if m.err != nil {
		return PDLGDParams{}, m.err
	}
	if p, ok := m.params[ac]; ok {
		return p, nil
	}
	return PDLGDParams{}, ErrPDLGDClassMissing(string(ac), "2026-06-11")
}

func (m *mockPDLGDRepo) BulkGetPDLGDForAssetClasses(_ context.Context, acs []AssetClass, _ time.Time, _ string) (map[AssetClass]PDLGDParams, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[AssetClass]PDLGDParams)
	for _, ac := range acs {
		if p, ok := m.params[ac]; ok {
			result[ac] = p
		}
	}
	return result, nil
}

// mockScenarioParamRepo implements ScenarioParamRepo.
type mockScenarioParamRepo struct {
	weights ScenarioWeights
	fl      FLMultipliers
	wErr    error
	fErr    error
}

func (m *mockScenarioParamRepo) GetScenarioWeights(_ context.Context, _ uuid.UUID, _ string) (ScenarioWeights, error) {
	if m.wErr != nil {
		return ScenarioWeights{}, m.wErr
	}
	if m.weights == (ScenarioWeights{}) {
		return defaultWeights, nil
	}
	return m.weights, nil
}

func (m *mockScenarioParamRepo) GetFLMultipliers(_ context.Context, _ uuid.UUID, _ string) (FLMultipliers, error) {
	if m.fErr != nil {
		return FLMultipliers{}, m.fErr
	}
	if m.fl == (FLMultipliers{}) {
		return defaultFL, nil
	}
	return m.fl, nil
}

// mockResultRepo implements LookthroughResultRepo.
type mockResultRepo struct {
	upsertErr error
	stored    *StoredLookthroughResult
	getErr    error
}

func (m *mockResultRepo) UpsertResult(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ Result, _ uuid.UUID, _ uuid.UUID, _ time.Time, _ string) error {
	return m.upsertErr
}

func (m *mockResultRepo) GetByInstrumenAndRun(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*StoredLookthroughResult, error) {
	return m.stored, m.getErr
}

// ─── CompositionService tests ─────────────────────────────────────────────────

// TestCompositionService_Submit_ValidatesReksadana ensures non-REKSADANA is rejected.
// AC: APP-C-LKT-002-AC02.
func TestCompositionService_Submit_ValidatesReksadana(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{tipe: "DEPOSITO"}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Submit(context.Background(), SubmitCompositionRequest{
		InstrumenID:   uuid.New(),
		EffectiveFrom: time.Now(),
		Lines: []CompositionLineInput{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(100)},
		},
	}, uuid.New(), "ROLE-AKUN")

	if err == nil {
		t.Fatal("expected error for non-REKSADANA instrument")
	}
	checkDomainCode(t, err, CodeLookthroughInstrumenNotReksadana)
}

// TestCompositionService_Submit_ValidatesAssetClass ensures unknown asset class returns error.
// AC: APP-C-LKT-002-AC04.
func TestCompositionService_Submit_ValidatesAssetClass(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{tipe: "REKSADANA"}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Submit(context.Background(), SubmitCompositionRequest{
		InstrumenID:   uuid.New(),
		EffectiveFrom: time.Now(),
		Lines: []CompositionLineInput{
			{AssetClass: AssetClass("INVALID"), WeightPct: decimal.NewFromFloat(100)},
		},
	}, uuid.New(), "ROLE-AKUN")

	if err == nil {
		t.Fatal("expected error for invalid asset class")
	}
	checkDomainCode(t, err, CodeLookthroughAssetClassUnknown)
}

// TestCompositionService_Submit_ValidatesWeightSum ensures weight ≠ 100% fails.
// AC: APP-C-LKT-002-AC05.
func TestCompositionService_Submit_ValidatesWeightSum(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{tipe: "REKSADANA"}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Submit(context.Background(), SubmitCompositionRequest{
		InstrumenID:   uuid.New(),
		EffectiveFrom: time.Now(),
		Lines: []CompositionLineInput{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(60)},
			{AssetClass: AssetClassCorpBond, WeightPct: decimal.NewFromFloat(30)},
			// Total = 90%, not 100%
		},
	}, uuid.New(), "ROLE-AKUN")

	if err == nil {
		t.Fatal("expected error for weight sum ≠ 100%")
	}
	checkDomainCode(t, err, CodeLookthroughWeightInvalid)
}

// TestCompositionService_Submit_DuplicateAssetClass ensures duplicate asset classes fail.
func TestCompositionService_Submit_DuplicateAssetClass(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{tipe: "REKSADANA"}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Submit(context.Background(), SubmitCompositionRequest{
		InstrumenID:   uuid.New(),
		EffectiveFrom: time.Now(),
		Lines: []CompositionLineInput{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(50)},
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(50)}, // duplicate
		},
	}, uuid.New(), "ROLE-AKUN")

	if err == nil {
		t.Fatal("expected error for duplicate asset class")
	}
	checkDomainCode(t, err, CodeLookthroughAssetClassUnknown)
}

// TestCompositionService_Review_SoDViolation ensures maker cannot be reviewer.
// AC: APP-C-LKT-002-AC10.
func TestCompositionService_Review_SoDViolation(t *testing.T) {
	t.Parallel()
	makerID := uuid.New()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        makerID,
			WorkflowStatus: WorkflowStatusPendingReview,
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Review(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       makerID, // same as maker — SoD violation
		ActorRole:     "ROLE-RISK",
	})

	if err == nil {
		t.Fatal("expected SoD violation error")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionSoDViolation)
}

// TestCompositionService_Review_InvalidTransition ensures reviewing a REJECTED composition fails.
// AC: APP-C-LKT-002-AC09.
func TestCompositionService_Review_InvalidTransition(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        uuid.New(),
			WorkflowStatus: WorkflowStatusRejected, // terminal
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Review(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-RISK",
	})

	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionReviewInvalidTransition)
}

// TestCompositionService_Approve_SoDViolation_ApproverIsMaker ensures SoD.
// AC: APP-C-LKT-002-AC15.
func TestCompositionService_Approve_SoDViolation_ApproverIsMaker(t *testing.T) {
	t.Parallel()
	makerID := uuid.New()
	reviewerID := uuid.New()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        makerID,
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       makerID, // approver == maker — SoD violation
		ActorRole:     "ROLE-ALCO",
	}, nil)

	if err == nil {
		t.Fatal("expected SoD violation")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionSoDViolation)
}

// TestCompositionService_Approve_SoDViolation_ApproverIsReviewer ensures SoD.
func TestCompositionService_Approve_SoDViolation_ApproverIsReviewer(t *testing.T) {
	t.Parallel()
	reviewerID := uuid.New()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        uuid.New(),
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       reviewerID, // approver == reviewer — SoD violation
		ActorRole:     "ROLE-ALCO",
	}, nil)

	if err == nil {
		t.Fatal("expected SoD violation")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionSoDViolation)
}

// TestCompositionService_Reject_SoDViolation ensures maker cannot reject.
func TestCompositionService_Reject_SoDViolation(t *testing.T) {
	t.Parallel()
	makerID := uuid.New()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        makerID,
			WorkflowStatus: WorkflowStatusPendingReview,
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Reject(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       makerID, // maker = rejector — SoD violation
		ActorRole:     "ROLE-RISK",
		Comment:       "rejecting my own submission",
	})

	if err == nil {
		t.Fatal("expected SoD violation")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionSoDViolation)
}

// ─── LookthroughService tests ─────────────────────────────────────────────────

// TestLookthroughService_Compute_FVTPLSkip ensures FVTPL instruments return ECL=0 without error.
// AC: APP-C-LKT-001-AC07 (FVTPL skip).
func TestLookthroughService_Compute_FVTPLSkip(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(1_000_000)
	instRepo := &mockReksadanaRepo{
		inst: &InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "FVTPL",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		},
	}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	result, err := svc.Compute(context.Background(),
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now())

	if err != nil {
		t.Fatalf("expected nil error for FVTPL skip, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.FVTPLSkipped {
		t.Error("FVTPLSkipped should be true")
	}
	if !result.TotalECLIDR.Equal(decimal.Zero) {
		t.Errorf("TotalECLIDR should be 0 for FVTPL, got %s", result.TotalECLIDR)
	}
	if result.Warning == "" {
		t.Error("Warning should be set for FVTPL skip")
	}
}

// TestLookthroughService_Compute_POCIDeferred ensures POCI returns LOOKTHROUGH_POCI_DEFERRED error.
// AC: APP-C-LKT-001-AC08.
func TestLookthroughService_Compute_POCIDeferred(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(1_000_000)
	instRepo := &mockReksadanaRepo{
		inst: &InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "AC",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
			POCIFlag:          true, // POCI
		},
	}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.Compute(context.Background(),
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now())

	if err == nil {
		t.Fatal("expected POCI deferred error")
	}
	checkDomainCode(t, err, CodeLookthroughPOCIDeferred)
}

// TestLookthroughService_Compute_NABMissing ensures NAB=nil returns error.
// AC: APP-C-LKT-001-AC03.
func TestLookthroughService_Compute_NABMissing(t *testing.T) {
	t.Parallel()
	instRepo := &mockReksadanaRepo{
		inst: &InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "AC",
			NominalNABIDR:     nil, // NAB missing
			TipeInstrumen:     "REKSADANA",
		},
	}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.Compute(context.Background(),
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now())

	if err == nil {
		t.Fatal("expected NAB missing error")
	}
	checkDomainCode(t, err, CodeLookthroughNABMissing)
}

// TestLookthroughService_Compute_FundCompositionMissing ensures missing composition returns error.
// AC: APP-C-LKT-001-AC04.
func TestLookthroughService_Compute_FundCompositionMissing(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(5_000_000)
	instRepo := &mockReksadanaRepo{
		inst: &InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "AC",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		},
	}
	// compRepo.activeComp = nil → composition missing
	compRepo := &mockFundCompositionRepo{activeComp: nil}
	svc := newTestLookthroughService(instRepo, compRepo, nil, nil, nil)

	_, err := svc.Compute(context.Background(),
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now())

	if err == nil {
		t.Fatal("expected fund composition missing error")
	}
	checkDomainCode(t, err, CodeLookthroughFundCompositionMissing)
}

// TestLookthroughService_Compute_Success verifies a successful full computation.
// AC: APP-C-LKT-001-AC01..AC06.
func TestLookthroughService_Compute_Success(t *testing.T) {
	t.Parallel()
	instrumenID := uuid.New()
	compositionID := uuid.New()
	nab := decimal.NewFromFloat(10_000_000) // IDR 10M

	instRepo := &mockReksadanaRepo{
		inst: &InstrumenReksadanaRow{
			ID:                instrumenID,
			KlasifikasiPsak71: "AC",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		},
	}
	compRepo := &mockFundCompositionRepo{
		activeComp: &FundComposition{
			ID:             compositionID,
			WorkflowStatus: WorkflowStatusApprovedActive,
			EffectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		details: []FundCompositionDetail{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(50)},
			{AssetClass: AssetClassCorpBond, WeightPct: decimal.NewFromFloat(50)},
		},
	}
	pdlgdRepo := &mockPDLGDRepo{
		params: map[AssetClass]PDLGDParams{
			AssetClassGovtBond: {
				AssetClass: AssetClassGovtBond,
				PDGood:     decimal.Zero,
				PDNormal:   decimal.Zero,
				PDBad:      decimal.Zero,
				LGD:        decimal.NewFromFloat(0.10),
			},
			AssetClassCorpBond: {
				AssetClass: AssetClassCorpBond,
				PDGood:     decimal.NewFromFloat(0.02),
				PDNormal:   decimal.NewFromFloat(0.03),
				PDBad:      decimal.NewFromFloat(0.06),
				LGD:        decimal.NewFromFloat(0.45),
			},
		},
	}

	svc := newTestLookthroughService(instRepo, compRepo, pdlgdRepo, nil, nil)

	result, err := svc.Compute(context.Background(),
		instrumenID, uuid.UUID{}, uuid.New(), time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.FVTPLSkipped {
		t.Error("FVTPLSkipped should be false for AC instrument")
	}
	if len(result.Breakdown) != 2 {
		t.Errorf("expected 2 breakdown lines, got %d", len(result.Breakdown))
	}
	// GOVT_BOND: PD=0 → ECL=0
	if !result.Breakdown[0].ECLWeightedIDR.Equal(decimal.Zero) {
		t.Errorf("GOVT_BOND ECL should be 0, got %s", result.Breakdown[0].ECLWeightedIDR)
	}
	// CORP_BOND: NABPortion = 5M; PD_Good=0.02, PD_Normal=0.03, PD_Bad=0.06; LGD=0.45; FL=1; bobot=0.25/0.50/0.25
	// ECL_Good = 5M × 0.02 × 0.45 = 45_000
	// ECL_Normal = 5M × 0.03 × 0.45 = 67_500
	// ECL_Bad = 5M × 0.06 × 0.45 = 135_000
	// ECL_w = 45_000×0.25 + 67_500×0.50 + 135_000×0.25 = 11_250 + 33_750 + 33_750 = 78_750
	expectedCorpBondECL := decimal.NewFromFloat(78_750)
	if !result.Breakdown[1].ECLWeightedIDR.Equal(expectedCorpBondECL) {
		t.Errorf("CorpBond ECLWeightedIDR: expected %s, got %s", expectedCorpBondECL, result.Breakdown[1].ECLWeightedIDR)
	}
	// Total = 0 + 78_750 = 78_750
	if !result.TotalECLIDR.Equal(decimal.NewFromFloat(78_750)) {
		t.Errorf("TotalECLIDR: expected 78_750, got %s", result.TotalECLIDR)
	}
}

// TestLookthroughService_BulkCompute_TooLarge returns ErrBulkTooLarge if > 10_000 instruments.
// AC: APP-C-LKT-001-AC15.
func TestLookthroughService_BulkCompute_TooLarge(t *testing.T) {
	t.Parallel()
	// Build 10_001 instruments.
	instruments := make([]InstrumenReksadanaRow, 10_001)
	for i := range instruments {
		instruments[i] = InstrumenReksadanaRow{ID: uuid.New(), KlasifikasiPsak71: "AC", TipeInstrumen: "REKSADANA"}
	}
	instRepo := &mockReksadanaRepo{bulk: instruments}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now())
	if err == nil {
		t.Fatal("expected bulk too large error")
	}
	checkDomainCode(t, err, CodeLookthroughBulkTooLarge)
}

// TestLookthroughService_BulkCompute_POCIIsNonFatal ensures POCI errors are captured not propagated.
// AC: APP-C-LKT-001-AC13.
func TestLookthroughService_BulkCompute_POCIIsNonFatal(t *testing.T) {
	t.Parallel()
	pociNAB := decimal.NewFromFloat(1_000_000)
	pociInst := InstrumenReksadanaRow{
		ID:                uuid.New(),
		KlasifikasiPsak71: "AC",
		NominalNABIDR:     &pociNAB,
		TipeInstrumen:     "REKSADANA",
		POCIFlag:          true, // POCI — should be non-fatal skip
	}
	instruments := []InstrumenReksadanaRow{pociInst}
	// inst is returned by GetByID (called per-instrument inside Compute).
	instRepo := &mockReksadanaRepo{bulk: instruments, inst: &pociInst}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	results, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("bulk compute should not return top-level error for POCI: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsPOCI {
		t.Error("expected IsPOCI=true for POCI instrument")
	}
}

// TestAuditWriterIface_NilPanics verifies constructor panics if auditWriter is nil.
// DEC-018 enforcement — no nil-guard pattern.
func TestAuditWriterIface_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter in CompositionService")
		}
	}()
	_ = NewCompositionService(
		&sql.DB{},
		&mockFundCompositionRepo{},
		nil, // nil auditWriter — must panic
		nil,
	)
}

// TestAuditWriterIface_NilPanics_Lookthrough verifies LookthroughService panics on nil auditWriter.
func TestAuditWriterIface_NilPanics_Lookthrough(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter in LookthroughService")
		}
	}()
	_ = NewLookthroughService(
		&sql.DB{},
		&mockReksadanaRepo{},
		&mockFundCompositionRepo{},
		&mockPDLGDRepo{},
		&mockScenarioParamRepo{},
		&mockResultRepo{},
		nil, // nil auditWriter — must panic
		NoopMetrics(),
		nil,
	)
}

// TestIsDomainCode verifies the isDomainCode helper.
func TestIsDomainCode(t *testing.T) {
	t.Parallel()
	err := ErrNABMissing("INST-001", "2026-06-11")
	if !isDomainCode(err, CodeLookthroughNABMissing) {
		t.Errorf("expected isDomainCode to return true for %s", CodeLookthroughNABMissing)
	}
	if isDomainCode(err, CodeLookthroughFundCompositionMissing) {
		t.Error("should not match wrong code")
	}
	if isDomainCode(nil, CodeLookthroughNABMissing) {
		t.Error("should return false for nil error")
	}
	if isDomainCode(errors.New("generic"), CodeLookthroughNABMissing) {
		t.Error("should return false for non-domain error")
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newTestCompositionService creates a CompositionService with a nil DB.
// Transaction operations won't work — this is for service-level validation tests only.
func newTestCompositionService(compRepo FundCompositionRepo) *CompositionService {
	// Use nil db — service logic tested before any tx operation.
	// compRepo injection only — avoid panic in constructors.
	return &CompositionService{
		db:          nil,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}
}

// newTestLookthroughService creates a LookthroughService with stub repos.
// runID=uuid.UUID{} in Compute tests skips DB upsert (tx branch).
func newTestLookthroughService(
	instRepo ReksadanaInstrumenRepo,
	compRepo FundCompositionRepo,
	pdlgdRepo PDLGDClassRepo,
	paramRepo ScenarioParamRepo,
	resultRepo ResultRepo,
) *Service {
	if pdlgdRepo == nil {
		pdlgdRepo = &mockPDLGDRepo{params: map[AssetClass]PDLGDParams{}}
	}
	if paramRepo == nil {
		paramRepo = &mockScenarioParamRepo{}
	}
	if resultRepo == nil {
		resultRepo = &mockResultRepo{}
	}
	return &Service{
		db:          nil,
		instRepo:    instRepo,
		compRepo:    compRepo,
		pdlgdRepo:   pdlgdRepo,
		paramRepo:   paramRepo,
		resultRepo:  resultRepo,
		auditWriter: &mockAuditWriter{},
		metrics:     NoopMetrics(),
		logger:      nil,
	}
}

// checkDomainCode asserts the error carries the expected domain code.
// Uses the domainerrors.Code type (underlying string) for comparison.
func checkDomainCode(t *testing.T, err error, expectedCode string) {
	t.Helper()
	type coder interface {
		Code() domainerrors.Code
	}
	c, ok := err.(coder)
	if !ok {
		t.Fatalf("error %v does not implement Code() domainerrors.Code", err)
	}
	if string(c.Code()) != expectedCode {
		t.Errorf("expected error code %s, got %s (error: %v)", expectedCode, c.Code(), err)
	}
}
