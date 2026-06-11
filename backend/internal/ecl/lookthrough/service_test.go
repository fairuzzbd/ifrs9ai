package lookthrough

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
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
	supersedeCalled    bool
	supersedeErr       error
	supersedeDateCalled time.Time // captures the supersedeDate argument for assertion
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

func (m *mockFundCompositionRepo) SupersedeOld(_ context.Context, _ *sql.Tx, _ uuid.UUID, d time.Time, _ uuid.UUID) error {
	m.supersedeCalled = true
	m.supersedeDateCalled = d
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

func (m *mockResultRepo) UpsertResult(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ Result, _ uuid.UUID, _ uuid.UUID, _ time.Time, _ uuid.UUID, _ string) error {
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
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})

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
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})

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
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})

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
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})

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
		instrumenID, uuid.UUID{}, uuid.New(), time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), uuid.UUID{})

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

	_, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now(), uuid.UUID{})
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

	results, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now(), uuid.UUID{})
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

// ─── Service tx-path tests using sqlmock ─────────────────────────────────────

// TestCompositionService_Submit_TransactionPath verifies Submit goes through the tx path.
// Uses sqlmock to allow s.db.BeginTx to succeed; repo is mocked to succeed.
func TestCompositionService_Submit_TransactionPath(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// mockFundCompositionRepo + mockAuditWriter do not execute SQL against the DB.
	// Only sqlmock.ExpectBegin + ExpectCommit needed.
	mock.ExpectBegin()
	mock.ExpectCommit()

	compRepo := &mockFundCompositionRepo{tipe: "REKSADANA"}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	result, err := svc.Submit(context.Background(), SubmitCompositionRequest{
		InstrumenID:   uuid.New(),
		EffectiveFrom: time.Now(),
		Lines: []CompositionLineInput{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(100)},
		},
	}, uuid.New(), "ROLE-AKUN")
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Header.WorkflowStatus != WorkflowStatusPendingReview {
		t.Errorf("WorkflowStatus: got %s", result.Header.WorkflowStatus)
	}
}

// TestCompositionService_Review_TransactionPath verifies Review transitions to PENDING_APPROVAL.
func TestCompositionService_Review_TransactionPath(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	reviewerID := uuid.New()
	compositionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectCommit()

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			WorkflowStatus: WorkflowStatusPendingReview,
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	result, err := svc.Review(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       reviewerID,
		ActorRole:     "ROLE-RISK",
		Comment:       "Looks good",
	})
	if err != nil {
		t.Fatalf("Review error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil composition")
	}
	if result.WorkflowStatus != WorkflowStatusPendingApproval {
		t.Errorf("WorkflowStatus: got %s", result.WorkflowStatus)
	}
}

// TestCompositionService_Approve_TransactionPath verifies Approve transitions to APPROVED_ACTIVE.
func TestCompositionService_Approve_TransactionPath(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	compositionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectCommit()

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	result, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       approverID,
		ActorRole:     "ROLE-ALCO",
		Comment:       "ALCO approves",
	}, nil)
	if err != nil {
		t.Fatalf("Approve error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil composition")
	}
	if result.WorkflowStatus != WorkflowStatusApprovedActive {
		t.Errorf("WorkflowStatus: got %s", result.WorkflowStatus)
	}
}

// TestCompositionService_Reject_TransactionPath verifies Reject transitions to REJECTED.
func TestCompositionService_Reject_TransactionPath(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	rejectorID := uuid.New()
	compositionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectCommit()

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			WorkflowStatus: WorkflowStatusPendingReview,
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	result, err := svc.Reject(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       rejectorID,
		ActorRole:     "ROLE-RISK",
		Comment:       "Needs more data.",
	})
	if err != nil {
		t.Fatalf("Reject error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil composition")
	}
	if result.WorkflowStatus != WorkflowStatusRejected {
		t.Errorf("WorkflowStatus: got %s", result.WorkflowStatus)
	}
}

// TestNewCompositionService_Success verifies constructor returns non-nil service.
func TestNewCompositionService_Success(t *testing.T) {
	t.Parallel()
	db, _, _ := sqlmock.New()
	defer db.Close()
	svc := NewCompositionService(db, &mockFundCompositionRepo{}, &mockAuditWriter{}, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// TestNewLookthroughService_Success verifies constructor returns non-nil service.
func TestNewLookthroughService_Success(t *testing.T) {
	t.Parallel()
	db, _, _ := sqlmock.New()
	defer db.Close()
	svc := NewLookthroughService(db, &mockReksadanaRepo{}, &mockFundCompositionRepo{},
		&mockPDLGDRepo{}, &mockScenarioParamRepo{}, &mockResultRepo{}, &mockAuditWriter{}, nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// TestNewCompositionService_NilDBPanics verifies nil db panics.
func TestNewCompositionService_NilDBPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewCompositionService(nil, &mockFundCompositionRepo{}, &mockAuditWriter{}, nil)
}

// TestNewCompositionService_NilRepoPanics verifies nil repo panics.
func TestNewCompositionService_NilRepoPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil repo")
		}
	}()
	db, _, _ := sqlmock.New()
	defer db.Close()
	_ = NewCompositionService(db, nil, &mockAuditWriter{}, nil)
}

// TestNewLookthroughService_NilDBPanics verifies nil db panics.
func TestNewLookthroughService_NilDBPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewLookthroughService(nil, &mockReksadanaRepo{}, &mockFundCompositionRepo{},
		&mockPDLGDRepo{}, &mockScenarioParamRepo{}, &mockResultRepo{}, &mockAuditWriter{}, nil, nil)
}

// TestCompositionService_Approve_WithSupersedes verifies amendment path calls SupersedeOld.
func TestCompositionService_Approve_WithSupersedes(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	compositionID := uuid.New()
	oldCompositionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectCommit()

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	result, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       approverID,
		ActorRole:     "ROLE-ALCO",
		Comment:       "Amendment approved",
	}, &oldCompositionID) // non-nil supersedesID triggers SupersedeOld
	if err != nil {
		t.Fatalf("Approve with supersedes error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil composition")
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

// ─── GetCompositionGroup tests ────────────────────────────────────────────────

// TestCompositionService_GetCompositionGroup_NotFound returns error when header not found.
func TestCompositionService_GetCompositionGroup_NotFound(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{composition: nil, getByIDErr: nil}
	svc := newTestCompositionService(compRepo)

	_, err := svc.GetCompositionGroup(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not-found error when composition is nil")
	}
}

// TestCompositionService_GetCompositionGroup_RepoError propagates repo error.
func TestCompositionService_GetCompositionGroup_RepoError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db error")
	compRepo := &mockFundCompositionRepo{getByIDErr: sentinel}
	svc := newTestCompositionService(compRepo)

	_, err := svc.GetCompositionGroup(context.Background(), uuid.New())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

// TestCompositionService_GetCompositionGroup_Success returns group with computed total weight.
func TestCompositionService_GetCompositionGroup_Success(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{ID: id, WorkflowStatus: WorkflowStatusApprovedActive},
		details: []FundCompositionDetail{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(60)},
			{AssetClass: AssetClassCorpBond, WeightPct: decimal.NewFromFloat(40)},
		},
	}
	svc := newTestCompositionService(compRepo)

	group, err := svc.GetCompositionGroup(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Header.ID != id {
		t.Errorf("header ID mismatch: %v", group.Header.ID)
	}
	if len(group.Details) != 2 {
		t.Errorf("expected 2 details, got %d", len(group.Details))
	}
	expected := decimal.NewFromFloat(100)
	if !group.TotalWeightPct.Equal(expected) {
		t.Errorf("TotalWeightPct: expected 100, got %s", group.TotalWeightPct)
	}
}

// ─── ListCompositions tests ───────────────────────────────────────────────────

// TestCompositionService_ListCompositions_DelegatesToRepo verifies pass-through to repo.
func TestCompositionService_ListCompositions_DelegatesToRepo(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	compRepo := &mockFundCompositionRepo{
		compositions: []FundComposition{
			{ID: id, WorkflowStatus: WorkflowStatusApprovedActive},
		},
	}
	svc := newTestCompositionService(compRepo)

	comps, _, _, err := svc.ListCompositions(context.Background(), uuid.New(),
		"APPROVED_ACTIVE", "", 50, "created_at", "desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comps) != 1 {
		t.Errorf("expected 1 composition, got %d", len(comps))
	}
}

// ─── Review pre-tx validation tests ──────────────────────────────────────────

// TestCompositionService_Review_NotFound returns error when GetByID returns nil.
func TestCompositionService_Review_NotFound(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{composition: nil}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Review(context.Background(), WorkflowActionRequest{
		CompositionID: uuid.New(),
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-RISK",
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

// TestCompositionService_Review_RepoError propagates repo GetByID error.
func TestCompositionService_Review_RepoError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db down")
	compRepo := &mockFundCompositionRepo{getByIDErr: sentinel}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Review(context.Background(), WorkflowActionRequest{
		CompositionID: uuid.New(),
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-RISK",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

// ─── Approve pre-tx validation tests ─────────────────────────────────────────

// TestCompositionService_Approve_NotFound returns error when composition is nil.
func TestCompositionService_Approve_NotFound(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{composition: nil}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: uuid.New(),
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-ALCO",
	}, nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

// TestCompositionService_Approve_InvalidTransition rejects wrong source state.
func TestCompositionService_Approve_InvalidTransition(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        uuid.New(),
			WorkflowStatus: WorkflowStatusPendingReview, // must be PENDING_APPROVAL first
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-ALCO",
	}, nil)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionReviewInvalidTransition)
}

// ─── Reject pre-tx validation tests ──────────────────────────────────────────

// TestCompositionService_Reject_NotFound returns error when composition is nil.
func TestCompositionService_Reject_NotFound(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{composition: nil}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Reject(context.Background(), WorkflowActionRequest{
		CompositionID: uuid.New(),
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-RISK",
		Comment:       "rejected",
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

// TestCompositionService_Reject_InvalidTransition rejects wrong source state.
func TestCompositionService_Reject_InvalidTransition(t *testing.T) {
	t.Parallel()
	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             uuid.New(),
			MakerID:        uuid.New(),
			WorkflowStatus: WorkflowStatusApprovedActive, // terminal — cannot reject
		},
	}
	svc := newTestCompositionService(compRepo)

	_, err := svc.Reject(context.Background(), WorkflowActionRequest{
		CompositionID: compRepo.composition.ID,
		ActorID:       uuid.New(),
		ActorRole:     "ROLE-RISK",
		Comment:       "try to reject",
	})
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	checkDomainCode(t, err, CodeLookthroughCompositionReviewInvalidTransition)
}

// ─── LookthroughService Compute additional coverage ───────────────────────────

// TestLookthroughService_Compute_InstrumenNotFound verifies error when GetByID returns nil.
func TestLookthroughService_Compute_InstrumenNotFound(t *testing.T) {
	t.Parallel()
	instRepo := &mockReksadanaRepo{inst: nil, getErr: nil}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.Compute(context.Background(), uuid.New(), uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})
	if err == nil {
		t.Fatal("expected error when instrumen not found")
	}
}

// TestLookthroughService_Compute_InstrumenRepoError propagates repo error.
func TestLookthroughService_Compute_InstrumenRepoError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db error")
	instRepo := &mockReksadanaRepo{getErr: sentinel}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.Compute(context.Background(), uuid.New(), uuid.UUID{}, uuid.New(), time.Now(), uuid.UUID{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

// TestLookthroughService_Compute_PDLGDMissing verifies error when PD/LGD params missing.
func TestLookthroughService_Compute_PDLGDMissing(t *testing.T) {
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
	compRepo := &mockFundCompositionRepo{
		activeComp: &FundComposition{
			ID:             uuid.New(),
			WorkflowStatus: WorkflowStatusApprovedActive,
		},
		details: []FundCompositionDetail{
			{AssetClass: AssetClassCorpBond, WeightPct: decimal.NewFromFloat(100)},
		},
	}
	// pdlgdRepo has no params — will return ErrPDLGDClassMissing.
	pdlgdRepo := &mockPDLGDRepo{params: map[AssetClass]PDLGDParams{}}

	svc := newTestLookthroughService(instRepo, compRepo, pdlgdRepo, nil, nil)

	_, err := svc.Compute(context.Background(),
		instRepo.inst.ID, uuid.UUID{}, uuid.New(), time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), uuid.UUID{})
	if err == nil {
		t.Fatal("expected PDLGD missing error")
	}
	checkDomainCode(t, err, CodeLookthroughPDLGDClassMissing)
}

// ─── Preview coverage tests ───────────────────────────────────────────────────

// TestLookthroughService_Preview_NoInstruments returns empty slice without error.
func TestLookthroughService_Preview_NoInstruments(t *testing.T) {
	t.Parallel()
	instRepo := &mockReksadanaRepo{bulk: []InstrumenReksadanaRow{}}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	rows, cursor, hasMore, err := svc.Preview(context.Background(), uuid.New(), time.Now(), "", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
}

// TestLookthroughService_Preview_MissingCompositionWarning verifies warning row for missing composition.
func TestLookthroughService_Preview_MissingCompositionWarning(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(10_000_000)
	instRepo := &mockReksadanaRepo{
		bulk: []InstrumenReksadanaRow{
			{
				ID:                uuid.New(),
				KlasifikasiPsak71: "AC",
				NominalNABIDR:     &nab,
				TipeInstrumen:     "REKSADANA",
			},
		},
	}
	// activeComp = nil → composition missing warning.
	compRepo := &mockFundCompositionRepo{activeComp: nil}
	svc := newTestLookthroughService(instRepo, compRepo, nil, nil, nil)

	rows, _, _, err := svc.Preview(context.Background(), uuid.New(), time.Now(), "", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HasComposition {
		t.Error("HasComposition should be false when composition missing")
	}
	if len(rows[0].Warnings) == 0 {
		t.Error("expected warning in row for missing composition")
	}
}

// TestLookthroughService_Preview_FVTPLSkipInPreview returns ECL=0 row for FVTPL.
func TestLookthroughService_Preview_FVTPLSkipInPreview(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(10_000_000)
	instRepo := &mockReksadanaRepo{
		bulk: []InstrumenReksadanaRow{
			{
				ID:                uuid.New(),
				KlasifikasiPsak71: "FVTPL",
				NominalNABIDR:     &nab,
				TipeInstrumen:     "REKSADANA",
			},
		},
	}
	compRepo := &mockFundCompositionRepo{
		activeComp: &FundComposition{
			ID:             uuid.New(),
			WorkflowStatus: WorkflowStatusApprovedActive,
		},
	}
	svc := newTestLookthroughService(instRepo, compRepo, nil, nil, nil)

	rows, _, _, err := svc.Preview(context.Background(), uuid.New(), time.Now(), "", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].TotalECLEstimateIDRStr == nil {
		t.Fatal("expected ECL estimate for FVTPL skip")
	}
	if *rows[0].TotalECLEstimateIDRStr != "0.0000" {
		t.Errorf("expected ECL=0 for FVTPL, got %s", *rows[0].TotalECLEstimateIDRStr)
	}
}

// TestLookthroughService_Preview_Cursor verifies cursor-based pagination.
func TestLookthroughService_Preview_Cursor(t *testing.T) {
	t.Parallel()
	// Build 5 instruments, request limit=2 with no cursor.
	nab := decimal.NewFromFloat(1_000_000)
	bulk := make([]InstrumenReksadanaRow, 5)
	for i := range bulk {
		bulk[i] = InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "FVTPL",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		}
	}
	instRepo := &mockReksadanaRepo{bulk: bulk}
	compRepo := &mockFundCompositionRepo{
		activeComp: &FundComposition{ID: uuid.New(), WorkflowStatus: WorkflowStatusApprovedActive},
	}
	svc := newTestLookthroughService(instRepo, compRepo, nil, nil, nil)

	rows, cursor, hasMore, err := svc.Preview(context.Background(), uuid.New(), time.Now(), "", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows from page 1, got %d", len(rows))
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if cursor == "" {
		t.Error("expected non-empty cursor for page 2")
	}

	// Fetch page 2 using cursor.
	rows2, _, _, err := svc.Preview(context.Background(), uuid.New(), time.Now(), cursor, 2)
	if err != nil {
		t.Fatalf("page 2 error: %v", err)
	}
	if len(rows2) != 2 {
		t.Errorf("expected 2 rows from page 2, got %d", len(rows2))
	}
}

// ─── BulkCompute additional tests ────────────────────────────────────────────

// TestLookthroughService_BulkCompute_InstrumenRepoError propagates BulkListReksadanaForECL error.
func TestLookthroughService_BulkCompute_InstrumenRepoError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db error")
	instRepo := &mockReksadanaRepo{bulkErr: sentinel}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	_, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now(), uuid.UUID{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

// TestLookthroughService_BulkCompute_AllFVTPL returns results with FVTPLSkipped=true.
func TestLookthroughService_BulkCompute_AllFVTPL(t *testing.T) {
	t.Parallel()
	nab := decimal.NewFromFloat(1_000_000)
	bulk := []InstrumenReksadanaRow{
		{
			ID:                uuid.New(),
			KlasifikasiPsak71: "FVTPL",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		},
	}
	instRepo := &mockReksadanaRepo{bulk: bulk, inst: &bulk[0]}
	svc := newTestLookthroughService(instRepo, &mockFundCompositionRepo{}, nil, nil, nil)

	results, err := svc.BulkCompute(context.Background(), uuid.New(), uuid.New(), time.Now(), uuid.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected no error for FVTPL skip, got: %v", results[0].Err)
	}
	if results[0].Result == nil {
		t.Fatal("expected non-nil result for FVTPL skip")
	}
	if !results[0].Result.FVTPLSkipped {
		t.Error("expected FVTPLSkipped=true for FVTPL instrument")
	}
}

// ─── Issue #54: SupersedeOld effective_to = new.effective_from - 1 day ────────

// TestApproveComposition_SupersedeOld_EffectiveToIsNewMinus1Day verifies that when
// Approve is called with a non-nil supersedesID, the date passed to SupersedeOld equals
// comp.EffectiveFrom - 1 day (not comp.EffectiveFrom itself).
//
// Regression guard for the 1-day overlap bug fixed in Issue #54:
// Previously: supersedeDate == comp.EffectiveFrom (both old SUPERSEDED and new APPROVED_ACTIVE
// shared the same effective_from date → overlap).
// Fixed:      supersedeDate == comp.EffectiveFrom.AddDate(0, 0, -1) → non-overlapping.
func TestApproveComposition_SupersedeOld_EffectiveToIsNewMinus1Day(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	compositionID := uuid.New()
	oldCompositionID := uuid.New()

	// new composition has effective_from = 2026-07-01
	newEffectiveFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	expectedSupersedeDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC) // new_from - 1 day

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
			EffectiveFrom:  newEffectiveFrom,
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	_, err = svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       approverID,
		ActorRole:     "ROLE-ALCO",
		Comment:       "Amendment approved",
	}, &oldCompositionID)
	if err != nil {
		t.Fatalf("Approve error: %v", err)
	}

	if !compRepo.supersedeCalled {
		t.Fatal("expected SupersedeOld to be called for amendment path")
	}
	if !compRepo.supersedeDateCalled.Equal(expectedSupersedeDate) {
		t.Errorf(
			"SupersedeOld date: expected %s (new_from - 1 day), got %s — overlap bug #54",
			expectedSupersedeDate.Format("2006-01-02"),
			compRepo.supersedeDateCalled.Format("2006-01-02"),
		)
	}
}

// TestComposition_NoEffectiveDateOverlap_AfterSupersede verifies that after Approve
// with an amendment, the date ranges [old_from, supersedeDate] and [new_from, infinity]
// are non-overlapping and contiguous (supersedeDate + 1 day == new_from).
//
// Property: effective_to_old + 1 day == effective_from_new
func TestComposition_NoEffectiveDateOverlap_AfterSupersede(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	compositionID := uuid.New()
	oldCompositionID := uuid.New()

	// Simulate: old version valid from 2026-01-01.
	// New version starts 2026-04-01.
	// Old version should end 2026-03-31 (= 2026-04-01 - 1 day).
	newEffectiveFrom := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	expectedOldEffectiveTo := newEffectiveFrom.AddDate(0, 0, -1) // 2026-03-31

	compRepo := &mockFundCompositionRepo{
		composition: &FundComposition{
			ID:             compositionID,
			MakerID:        makerID,
			ReviewerID:     &reviewerID,
			WorkflowStatus: WorkflowStatusPendingApproval,
			EffectiveFrom:  newEffectiveFrom,
			EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		},
	}
	svc := &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: &mockAuditWriter{},
		logger:      nil,
	}

	_, err = svc.Approve(context.Background(), WorkflowActionRequest{
		CompositionID: compositionID,
		ActorID:       approverID,
		ActorRole:     "ROLE-ALCO",
		Comment:       "Amendment",
	}, &oldCompositionID)
	if err != nil {
		t.Fatalf("Approve error: %v", err)
	}

	gotSupersedeDate := compRepo.supersedeDateCalled

	// Range check: [old_from, supersedeDate] and [new_from, ∞] must not overlap.
	// Overlap condition: supersedeDate >= newEffectiveFrom.
	if !gotSupersedeDate.Before(newEffectiveFrom) {
		t.Errorf(
			"date overlap detected: old effective_to (%s) is not strictly before new effective_from (%s)",
			gotSupersedeDate.Format("2006-01-02"),
			newEffectiveFrom.Format("2006-01-02"),
		)
	}
	// Contiguity check: supersedeDate + 1 day == newEffectiveFrom (no gap).
	if !gotSupersedeDate.AddDate(0, 0, 1).Equal(newEffectiveFrom) {
		t.Errorf(
			"date gap detected: old effective_to (%s) + 1 day != new effective_from (%s)",
			gotSupersedeDate.Format("2006-01-02"),
			newEffectiveFrom.Format("2006-01-02"),
		)
	}
	// Explicit equality check against expected value.
	if !gotSupersedeDate.Equal(expectedOldEffectiveTo) {
		t.Errorf(
			"supersedeDate: expected %s, got %s",
			expectedOldEffectiveTo.Format("2006-01-02"),
			gotSupersedeDate.Format("2006-01-02"),
		)
	}
}
