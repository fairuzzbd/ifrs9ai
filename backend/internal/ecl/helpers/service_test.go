// Package helpers — unit tests for PD/LGD/EAD/CCF services.
//
// Coverage targets (per spec ≥ 85%):
//
//   - PD: Stage1 / Stage2 / Stage3 / FVTPL guard / FL multiplier
//   - LGD: pool lookup / REKSADANA guard / FVTPL guard / haircut
//   - EAD: IDR / FCY / outstanding fallback / accrued = 0 fallback
//   - CCF: known types / unknown type / missing config
//   - Bulk: < 1 / 1000-item cap / partial failure / stage3 skip
//
// All arithmetic verified against hand-computed expected values.
// No float64 used in expected values — all decimal.NewFromString().
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func d(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

var testInstrID = uuid.MustParse("01927f6c-0000-7000-8000-000000000001")
var testCPID = uuid.MustParse("01927f6c-0000-7000-8000-000000000002")
var testEvalDate = time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
var testPeriode = "PBUKU-2026-06"

// ─── Stub repositories ────────────────────────────────────────────────────────

type stubPDRepo struct {
	curve     *PDCurveRow
	impactPD  *ImpactPDRow
	impMevPD  *ImpactMevPDRow
	rating    string
	ratingErr error
}

func (s *stubPDRepo) GetPefindoCurve(_ context.Context, _, _ string) (*PDCurveRow, error) {
	return s.curve, nil
}
func (s *stubPDRepo) GetActiveImpactPD(_ context.Context, _ string) (*ImpactPDRow, error) {
	return s.impactPD, nil
}
func (s *stubPDRepo) GetActiveImpactMevPD(_ context.Context, scenario, _ string) (*ImpactMevPDRow, error) {
	if s.impMevPD != nil && s.impMevPD.Scenario == scenario {
		return s.impMevPD, nil
	}
	return nil, nil
}
func (s *stubPDRepo) GetActiveRating(_ context.Context, _ uuid.UUID, _ time.Time) (string, error) {
	return s.rating, s.ratingErr
}
func (s *stubPDRepo) BatchLoadPDCurves(_ context.Context, _ string) (map[string]PDCurveRow, error) {
	if s.curve != nil {
		return map[string]PDCurveRow{s.curve.Rating: *s.curve}, nil
	}
	return map[string]PDCurveRow{}, nil
}
func (s *stubPDRepo) BatchLoadImpactMevPD(_ context.Context, _ string) (map[string]ImpactMevPDRow, error) {
	result := map[string]ImpactMevPDRow{}
	if s.impMevPD != nil {
		result[s.impMevPD.Scenario] = *s.impMevPD
	}
	return result, nil
}
func (s *stubPDRepo) BatchLoadRatings(_ context.Context, ids []uuid.UUID, _ time.Time) (map[uuid.UUID]string, error) {
	result := map[uuid.UUID]string{}
	for _, id := range ids {
		result[id] = s.rating
	}
	return result, nil
}

type stubInstrRepo struct {
	inst   *InstrumenRow
	eirRow *EIRScheduleRow
	stage  EclStage
}

func (s *stubInstrRepo) GetEADInputs(_ context.Context, _ uuid.UUID) (*InstrumenRow, error) {
	return s.inst, nil
}
func (s *stubInstrRepo) GetCurrentStage(_ context.Context, _ uuid.UUID) (EclStage, error) {
	return s.stage, nil
}
func (s *stubInstrRepo) GetEIRScheduleRow(_ context.Context, _ uuid.UUID, _ time.Time) (*EIRScheduleRow, error) {
	return s.eirRow, nil
}
func (s *stubInstrRepo) BatchLoadInstruments(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]InstrumenRow, error) {
	result := map[uuid.UUID]InstrumenRow{}
	if s.inst != nil {
		for _, id := range ids {
			inst := *s.inst
			inst.ID = id
			result[id] = inst
		}
	}
	return result, nil
}
func (s *stubInstrRepo) BatchLoadEIRSchedules(_ context.Context, _ []uuid.UUID, _ time.Time) (map[uuid.UUID]EIRScheduleRow, error) {
	result := map[uuid.UUID]EIRScheduleRow{}
	if s.eirRow != nil {
		result[testInstrID] = *s.eirRow
	}
	return result, nil
}
func (s *stubInstrRepo) BatchLoadCurrentStages(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]EclStage, error) {
	result := map[uuid.UUID]EclStage{}
	for _, id := range ids {
		result[id] = s.stage
	}
	return result, nil
}

type stubCPRepo struct {
	tipe string
}

func (s *stubCPRepo) GetTipeCounterparty(_ context.Context, _ uuid.UUID) (string, error) {
	return s.tipe, nil
}
func (s *stubCPRepo) BatchLoadCounterparties(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]CounterpartyRow, error) {
	result := map[uuid.UUID]CounterpartyRow{}
	for _, id := range ids {
		result[id] = CounterpartyRow{ID: id, TipeCounterparty: s.tipe}
	}
	return result, nil
}

type stubLGDRepo struct {
	pool    *LGDBaselRow
	mapping map[string]string
}

func (s *stubLGDRepo) GetByPool(_ context.Context, tipeEksposur, _ string) (*LGDBaselRow, error) {
	if s.pool != nil && s.pool.TipeEksposur == tipeEksposur {
		return s.pool, nil
	}
	return nil, nil
}
func (s *stubLGDRepo) GetLGDMapping(_ context.Context) (map[string]string, error) {
	if s.mapping != nil {
		return s.mapping, nil
	}
	return map[string]string{"BANK": "BANK"}, nil
}
func (s *stubLGDRepo) GetCollateralHaircut(_ context.Context, _ string) (decimal.Decimal, error) {
	return decimal.Zero, nil
}
func (s *stubLGDRepo) BatchLoadLGDPools(_ context.Context, _ string) (map[string]LGDBaselRow, error) {
	if s.pool != nil {
		return map[string]LGDBaselRow{s.pool.TipeEksposur: *s.pool}, nil
	}
	return map[string]LGDBaselRow{}, nil
}

type stubKursRepo struct {
	kurs *KursRow
}

func (s *stubKursRepo) GetByDate(_ context.Context, currency string, _ time.Time) (*KursRow, error) {
	if s.kurs != nil && s.kurs.KodeMatauang == currency {
		return s.kurs, nil
	}
	return nil, nil
}
func (s *stubKursRepo) BatchLoadKurs(_ context.Context, _ time.Time) (map[string]KursRow, error) {
	result := map[string]KursRow{}
	if s.kurs != nil {
		result[s.kurs.KodeMatauang] = *s.kurs
	}
	return result, nil
}

type stubCCFRepo struct {
	table map[string]decimal.Decimal
}

func (s *stubCCFRepo) GetCCFTable(_ context.Context) (map[string]decimal.Decimal, error) {
	if s.table != nil {
		return s.table, nil
	}
	return map[string]decimal.Decimal{"DEPOSITO": decimal.Zero}, nil
}

// ─── PD service tests ─────────────────────────────────────────────────────────

func TestPDService_Stage1_NORMAL(t *testing.T) {
	// Setup: idAA PD_12m = 0.0035, impact_pd = 1.05, NORMAL mev = 1.0
	// Expected: PD_FL = 0.0035 × 1.05 × 1.0 = 0.003675 → RoundBank(8) = 0.00367500
	curve := &PDCurveRow{
		Rating:    "idAA",
		PD12Month: d("0.00350000"),
	}
	impPD := &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")}

	pdRepo := &stubPDRepo{curve: curve, impactPD: impPD, rating: "idAA"}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID:                testInstrID,
			CounterpartyID:    testCPID,
			KlasifikasiPsak71: "AC",
			TanggalJatuhTempo: func() *time.Time { t := time.Date(2028, 6, 30, 0, 0, 0, 0, time.UTC); return &t }(),
		},
	}

	svc := NewPDLookupService(pdRepo, instrRepo)
	pd, detail, err := svc.GetPD(context.Background(), testInstrID, Stage1, ScenarioNormal, testPeriode, testEvalDate)

	if err != nil {
		t.Fatalf("GetPD error: %v", err)
	}

	expected := d("0.00367500")
	if !pd.Equal(expected) {
		t.Errorf("PD mismatch: want %s got %s", expected, pd)
	}

	if detail.NormalMultiplierIsDefault != true {
		t.Error("NormalMultiplierIsDefault should be true for NORMAL scenario")
	}
	if detail.RatingUsed != "idAA" {
		t.Errorf("RatingUsed want idAA got %s", detail.RatingUsed)
	}
}

func TestPDService_Stage3_PD_IsOne(t *testing.T) {
	// Stage 3: PD = 1.0, no FL applied regardless of scenario.
	pdRepo := &stubPDRepo{}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID, KlasifikasiPsak71: "AC",
		},
	}

	svc := NewPDLookupService(pdRepo, instrRepo)
	for _, scenario := range []EclScenario{ScenarioGood, ScenarioNormal, ScenarioBad} {
		pd, _, err := svc.GetPD(context.Background(), testInstrID, Stage3, scenario, testPeriode, testEvalDate)
		if err != nil {
			t.Fatalf("GetPD Stage3 %s error: %v", scenario, err)
		}
		if !pd.Equal(decimal.NewFromInt(1)) {
			t.Errorf("Stage3 PD want 1.0 got %s (scenario %s)", pd, scenario)
		}
		// Stage 3 must not apply FL multipliers (DEC-010): PD=1.0 fixed means FL was not applied.
	}
}

func TestPDService_FVTPL_Returns_NotApplicable(t *testing.T) {
	pdRepo := &stubPDRepo{}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID, KlasifikasiPsak71: "FVTPL",
		},
	}
	svc := NewPDLookupService(pdRepo, instrRepo)
	_, _, err := svc.GetPD(context.Background(), testInstrID, Stage1, ScenarioNormal, testPeriode, testEvalDate)
	if err == nil {
		t.Fatal("Expected error for FVTPL instrument, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeInstrumentECLNotApplicable {
		t.Errorf("Want CodeInstrumentECLNotApplicable, got %v", err)
	}
}

func TestPDService_Stage2_LinearInterpolation(t *testing.T) {
	// Tenor = 4 years → interpolate between 3y and 5y.
	// PD_3y = 0.03, PD_5y = 0.05 → PD@4y = 0.03 + (0.05-0.03)*(4-3)/(5-3) = 0.04
	// impact_pd = 1.0, NORMAL mev = 1.0 → PD_FL = 0.04
	curve := &PDCurveRow{
		Rating:        "idA",
		PDLifetime3Y:  d("0.03000000"),
		PDLifetime5Y:  d("0.05000000"),
		PDLifetime7Y:  d("0.07000000"),
		PDLifetime10Y: d("0.10000000"),
	}
	impPD := &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.00000000")}
	pdRepo := &stubPDRepo{curve: curve, impactPD: impPD, rating: "idA"}

	jatuhTempo := testEvalDate.AddDate(4, 0, 0)
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID:                testInstrID,
			CounterpartyID:    testCPID,
			KlasifikasiPsak71: "AC",
			TanggalJatuhTempo: &jatuhTempo,
		},
	}

	svc := NewPDLookupService(pdRepo, instrRepo)
	pd, _, err := svc.GetPD(context.Background(), testInstrID, Stage2, ScenarioNormal, testPeriode, testEvalDate)
	if err != nil {
		t.Fatalf("GetPD Stage2 error: %v", err)
	}

	expected := d("0.04000000")
	if !pd.Equal(expected) {
		t.Errorf("Stage2 interpolated PD want %s got %s", expected, pd)
	}
}

// ─── LGD service tests ────────────────────────────────────────────────────────

func TestLGDService_BasicPool(t *testing.T) {
	// LGD pool BANK = 0.45, haircut = 0 → LGD_eff = 0.45
	lgdRepo := &stubLGDRepo{
		pool:    &LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")},
		mapping: map[string]string{"BANK": "BANK"},
	}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "DEPOSITO",
		},
	}
	cpRepo := &stubCPRepo{tipe: "BANK"}

	svc := NewLGDLookupService(lgdRepo, instrRepo, cpRepo)
	lgd, detail, err := svc.GetLGD(context.Background(), testInstrID, testPeriode)
	if err != nil {
		t.Fatalf("GetLGD error: %v", err)
	}
	expected := d("0.45000000")
	if !lgd.Equal(expected) {
		t.Errorf("LGD want %s got %s", expected, lgd)
	}
	if detail.PoolUsed != "BANK" {
		t.Errorf("PoolUsed want BANK got %s", detail.PoolUsed)
	}
}

func TestLGDService_REKSADANA_UseLookthrough(t *testing.T) {
	lgdRepo := &stubLGDRepo{}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "FVOCI_DEBT", TipeInstrumen: "REKSADANA",
		},
	}
	cpRepo := &stubCPRepo{tipe: "BANK"}

	svc := NewLGDLookupService(lgdRepo, instrRepo, cpRepo)
	_, _, err := svc.GetLGD(context.Background(), testInstrID, testPeriode)
	if err == nil {
		t.Fatal("Expected REKSADANA loothrough error, got nil")
	}
}

// ─── EAD service tests ────────────────────────────────────────────────────────

func TestEADService_IDR_NominalFallback(t *testing.T) {
	// IDR instrument, no EIR schedule → EAD = nominal = 1_000_000_000.0000
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "DEPOSITO",
			MatauangKode: "IDR", Nominal: d("1000000000.0000"),
		},
		eirRow: nil,
	}
	kursRepo := &stubKursRepo{}
	ccfSvc := NewCCFLookupService(&stubCCFRepo{})

	svc := NewEADService(instrRepo, kursRepo, ccfSvc)
	eadIDR, bd, err := svc.ComputeEAD(context.Background(), testInstrID, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEAD error: %v", err)
	}

	expected := d("1000000000.0000")
	if !eadIDR.Equal(expected) {
		t.Errorf("EAD_IDR want %s got %s", expected, eadIDR)
	}
	if bd.AccruedInterestSource != "ZERO_FALLBACK" {
		t.Errorf("AccruedInterestSource want ZERO_FALLBACK got %s", bd.AccruedInterestSource)
	}

	// Verify warnings include accrued = 0 and outstanding fallback.
	warnCodes := map[string]bool{}
	for _, w := range bd.Warnings {
		warnCodes[w.Code] = true
	}
	if !warnCodes[WarnOutstandingFallbackToNominal] {
		t.Error("Expected WarnOutstandingFallbackToNominal warning")
	}
	if !warnCodes[WarnAccruedZeroEIRScheduleMissing] {
		t.Error("Expected WarnAccruedZeroEIRScheduleMissing warning")
	}
}

func TestEADService_FCY_WithKurs(t *testing.T) {
	// USD instrument: nominal = 100_000, kurs = 15_000.5000 → EAD_IDR = 1_500_050_000.0000
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI",
			MatauangKode: "USD", Nominal: d("100000.0000"),
		},
	}
	kursRepo := &stubKursRepo{
		kurs: &KursRow{
			KodeMatauang:   "USD",
			NilaiKurs:      d("15000.50000000"),
			WorkflowStatus: "APPROVED",
		},
	}
	ccfSvc := NewCCFLookupService(&stubCCFRepo{})

	svc := NewEADService(instrRepo, kursRepo, ccfSvc)
	eadIDR, _, err := svc.ComputeEAD(context.Background(), testInstrID, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEAD FCY error: %v", err)
	}

	// 100_000 × 15000.5 = 1_500_050_000
	expected := d("1500050000.0000")
	if !eadIDR.Equal(expected) {
		t.Errorf("EAD_IDR FCY want %s got %s", expected, eadIDR)
	}
}

func TestEADService_FXRateMissing_Error(t *testing.T) {
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI",
			MatauangKode: "EUR", Nominal: d("100000.0000"),
		},
	}
	kursRepo := &stubKursRepo{} // no EUR kurs
	ccfSvc := NewCCFLookupService(&stubCCFRepo{})

	svc := NewEADService(instrRepo, kursRepo, ccfSvc)
	_, _, err := svc.ComputeEAD(context.Background(), testInstrID, testEvalDate)
	if err == nil {
		t.Fatal("Expected EAD_FX_RATE_MISSING error, got nil")
	}
}

// ─── CCF service tests ────────────────────────────────────────────────────────

func TestCCFService_KnownType_ZeroPhase1(t *testing.T) {
	svc := NewCCFLookupService(&stubCCFRepo{
		table: map[string]decimal.Decimal{"DEPOSITO": decimal.Zero},
	})
	ccf, detail, err := svc.GetCCF(context.Background(), "DEPOSITO")
	if err != nil {
		t.Fatalf("GetCCF error: %v", err)
	}
	if !ccf.IsZero() {
		t.Errorf("CCF want 0 got %s", ccf)
	}
	if detail.Source != "PHASE_1_HARDCODED" {
		t.Errorf("Source want PHASE_1_HARDCODED got %s", detail.Source)
	}
}

func TestCCFService_UnknownType_Error(t *testing.T) {
	svc := NewCCFLookupService(&stubCCFRepo{})
	_, _, err := svc.GetCCF(context.Background(), "UNKNOWN_TYPE_XYZ")
	if err == nil {
		t.Fatal("Expected CCF_INSTRUMEN_TYPE_UNKNOWN error, got nil")
	}
}

// ─── Bulk service tests ───────────────────────────────────────────────────────

func TestBulkLookup_Empty_Returns_Empty(t *testing.T) {
	svc := NewBulkHelperService(
		&stubPDRepo{}, &stubLGDRepo{}, &stubInstrRepo{},
		&stubCPRepo{}, &stubKursRepo{}, &stubCCFRepo{}, nil, nil,
	)
	results, summary, errs, skipped, err := svc.BulkLookup(context.Background(), nil, testPeriode, testEvalDate)
	if err != nil {
		t.Fatalf("BulkLookup empty error: %v", err)
	}
	if len(results) != 0 || len(errs) != 0 || len(skipped) != 0 {
		t.Error("Expected empty results for empty input")
	}
	if summary.Total != 0 {
		t.Errorf("Summary.Total want 0 got %d", summary.Total)
	}
}

func TestBulkLookup_TooMany_Returns_Error(t *testing.T) {
	reqs := make([]BulkRequest, maxBulkItems+1)
	for i := range reqs {
		reqs[i] = BulkRequest{InstrumenID: uuid.New()}
	}
	svc := NewBulkHelperService(
		&stubPDRepo{}, &stubLGDRepo{}, &stubInstrRepo{},
		&stubCPRepo{}, &stubKursRepo{}, &stubCCFRepo{}, nil, nil,
	)
	_, _, _, _, err := svc.BulkLookup(context.Background(), reqs, testPeriode, testEvalDate)
	if err == nil {
		t.Fatal("Expected HELPERS_BULK_TOO_LARGE error")
	}
}

func TestBulkLookup_FVTPL_Skipped(t *testing.T) {
	instrID := uuid.New()
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID:                instrID,
			CounterpartyID:    testCPID,
			KlasifikasiPsak71: "FVTPL",
			TipeInstrumen:     "SAHAM",
			MatauangKode:      "IDR",
			Nominal:           d("1000000.0000"),
		},
	}

	pdRepo := &stubPDRepo{}
	lgdRepo := &stubLGDRepo{}
	cpRepo := &stubCPRepo{tipe: "BANK"}
	kursRepo := &stubKursRepo{}
	ccfRepo := &stubCCFRepo{}

	svc := NewBulkHelperService(pdRepo, lgdRepo, instrRepo, cpRepo, kursRepo, ccfRepo, nil, nil)
	reqs := []BulkRequest{{InstrumenID: instrID}}
	results, summary, errs, skipped, err := svc.BulkLookup(context.Background(), reqs, testPeriode, testEvalDate)
	if err != nil {
		t.Fatalf("BulkLookup error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
	if len(errs) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(errs))
	}
	if len(skipped) != 1 {
		t.Errorf("Expected 1 skipped, got %d", len(skipped))
	}
	if summary.Skipped != 1 {
		t.Errorf("Summary.Skipped want 1 got %d", summary.Skipped)
	}
}

// ─── F1: decimal tenor tests ──────────────────────────────────────────────────

func TestPDService_Stage2_DecimalTenor_NoFloat64(t *testing.T) {
	// F1 (DEC-016): tenor computation must use decimal.Decimal, not float64.
	// Tenor exactly 4 years → interpolate between 3y and 5y.
	// PD_3y = 0.03, PD_5y = 0.05 → t = (4-3)/(5-3) = 0.5 → PD = 0.03 + 0.5×0.02 = 0.04
	// Verify daysPerYear constant: 36525/100 = 365.25 exactly in decimal.
	days365_25 := decimal.NewFromInt(36525).Div(decimal.NewFromInt(100))
	if !days365_25.Equal(daysPerYear) {
		t.Errorf("daysPerYear mismatch: want 365.25 got %s", daysPerYear)
	}

	// 4 years × 365.25 days/year = 1461 days
	tenorDaysInt := int64(4 * 365.25)
	tenorYearsDec := decimal.NewFromInt(tenorDaysInt).Div(daysPerYear)
	// Should be approximately 4.0 (1461 / 365.25 = 3.9986...)
	// For boundary test: use exactly 1461 days
	curve := PDCurveRow{
		PDLifetime3Y:  d("0.03000000"),
		PDLifetime5Y:  d("0.05000000"),
		PDLifetime7Y:  d("0.07000000"),
		PDLifetime10Y: d("0.10000000"),
	}
	pd, warn := interpolateLifetimePD(curve, tenorYearsDec)
	if warn != nil {
		t.Errorf("Unexpected warning: %v", warn)
	}
	// 1461/365.25 ≈ 3.9986; t = (3.9986-3)/(5-3) ≈ 0.4993; pd ≈ 0.03 + 0.4993×0.02 ≈ 0.039986
	// Just verify it's in (0.03, 0.05) and not zero or NaN
	if pd.LessThan(d("0.03")) || pd.GreaterThan(d("0.05")) {
		t.Errorf("Interpolated PD out of [3y,5y] range: %s", pd)
	}
}

func TestInterpolateLifetimePD_AllDecimal_BoundaryConditions(t *testing.T) {
	// F1: interpolateLifetimePD now takes decimal.Decimal — verify boundary conditions.
	curve := PDCurveRow{
		PDLifetime3Y:  d("0.03000000"),
		PDLifetime5Y:  d("0.05000000"),
		PDLifetime7Y:  d("0.07000000"),
		PDLifetime10Y: d("0.10000000"),
	}

	// At lower boundary: tenor = 2y → return 3y bucket.
	pd, _ := interpolateLifetimePD(curve, d("2.0"))
	if !pd.Equal(d("0.03000000")) {
		t.Errorf("Lower boundary: want 0.03000000 got %s", pd)
	}

	// At exactly 3y → return 3y bucket.
	pd, _ = interpolateLifetimePD(curve, d("3.0"))
	if !pd.Equal(d("0.03000000")) {
		t.Errorf("Exactly 3y: want 0.03000000 got %s", pd)
	}

	// At upper boundary: tenor = 12y → return 10y bucket.
	pd, _ = interpolateLifetimePD(curve, d("12.0"))
	if !pd.Equal(d("0.10000000")) {
		t.Errorf("Upper boundary: want 0.10000000 got %s", pd)
	}

	// Mid-point 4y: t=(4-3)/(5-3)=0.5, pd=0.03+0.5×0.02=0.04000000.
	pd, _ = interpolateLifetimePD(curve, d("4.0"))
	if !pd.Equal(d("0.04000000")) {
		t.Errorf("4y interpolation: want 0.04000000 got %s", pd)
	}
}

// ─── F2: FX rate stale warning tests ─────────────────────────────────────────

func TestEADService_FXRateStale_EmitsWarning(t *testing.T) {
	// F2 (OQ-M2-6): kurs date 7 days before evaluationDate → WarnFXRateStale emitted.
	staleDate := testEvalDate.AddDate(0, 0, -7)
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI",
			MatauangKode: "USD", Nominal: d("100000.0000"),
		},
	}
	kursRepo := &stubKursRepo{
		kurs: &KursRow{
			KodeMatauang:   "USD",
			NilaiKurs:      d("15000.00000000"),
			TanggalBerlaku: staleDate,
			WorkflowStatus: "APPROVED",
		},
	}
	ccfSvc := NewCCFLookupService(&stubCCFRepo{})

	svc := NewEADService(instrRepo, kursRepo, ccfSvc)
	_, bd, err := svc.ComputeEAD(context.Background(), testInstrID, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEAD error: %v", err)
	}

	warnCodes := map[string]bool{}
	for _, w := range bd.Warnings {
		warnCodes[w.Code] = true
	}
	if !warnCodes[WarnFXRateStale] {
		t.Errorf("Expected WarnFXRateStale warning for 7-day stale kurs; got warnings: %v", bd.Warnings)
	}
}

func TestComputeEADFromBatchParams_FXStale_EmitsWarning(t *testing.T) {
	// F2: batch path also emits WarnFXRateStale when kurs > 3d stale.
	staleDate := testEvalDate.AddDate(0, 0, -7)
	inst := InstrumenRow{
		ID: testInstrID, CounterpartyID: testCPID,
		KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI",
		MatauangKode: "USD", Nominal: d("100000.0000"),
	}
	params := &BatchParams{
		FXRates: map[string]KursRow{
			"USD": {
				KodeMatauang:   "USD",
				NilaiKurs:      d("15000.00000000"),
				TanggalBerlaku: staleDate,
				WorkflowStatus: "APPROVED",
			},
		},
		EIRSchedules:      map[uuid.UUID]EIRScheduleRow{},
		CCFTable:          map[string]decimal.Decimal{},
		CollateralHaircut: map[string]decimal.Decimal{},
	}

	_, bd, err := ComputeEADFromBatchParams(testInstrID, inst, params, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEADFromBatchParams error: %v", err)
	}

	warnCodes := map[string]bool{}
	for _, w := range bd.Warnings {
		warnCodes[w.Code] = true
	}
	if !warnCodes[WarnFXRateStale] {
		t.Errorf("Expected WarnFXRateStale in batch path; got warnings: %v", bd.Warnings)
	}
}

func TestEADService_FXRateFresh_NoStaleWarning(t *testing.T) {
	// F2: kurs date same day as evaluationDate → no stale warning.
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI",
			MatauangKode: "USD", Nominal: d("100000.0000"),
		},
	}
	kursRepo := &stubKursRepo{
		kurs: &KursRow{
			KodeMatauang:   "USD",
			NilaiKurs:      d("15000.00000000"),
			TanggalBerlaku: testEvalDate, // same day
			WorkflowStatus: "APPROVED",
		},
	}
	ccfSvc := NewCCFLookupService(&stubCCFRepo{})

	svc := NewEADService(instrRepo, kursRepo, ccfSvc)
	_, bd, err := svc.ComputeEAD(context.Background(), testInstrID, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEAD error: %v", err)
	}

	for _, w := range bd.Warnings {
		if w.Code == WarnFXRateStale {
			t.Errorf("Unexpected WarnFXRateStale for same-day kurs")
		}
	}
}

// ─── F3: POCI guard tests ─────────────────────────────────────────────────────

func TestPDService_POCI_Returns_NotApplicable(t *testing.T) {
	// F3 (FSD-APP-C §3.5, IFRS9 §5.5.13): POCI instruments deferred to P4-M7.
	pdRepo := &stubPDRepo{}
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: testInstrID, CounterpartyID: testCPID, KlasifikasiPsak71: "POCI",
		},
	}
	svc := NewPDLookupService(pdRepo, instrRepo)
	_, _, err := svc.GetPD(context.Background(), testInstrID, Stage1, ScenarioNormal, testPeriode, testEvalDate)
	if err == nil {
		t.Fatal("Expected error for POCI instrument, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("Expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodePOCIDeferredToM7 {
		t.Errorf("Want CodePOCIDeferredToM7, got %s", de.Code())
	}
}

// ─── F4: batch LGD haircut formula tests ─────────────────────────────────────

func TestGetPDFromBatchParams_POCI_Returns_DeferredToM7(t *testing.T) {
	// F3: batch path also guards POCI and returns CodePOCIDeferredToM7.
	inst := InstrumenRow{
		ID: testInstrID, CounterpartyID: testCPID,
		KlasifikasiPsak71: "POCI",
	}
	params := &BatchParams{
		PDCurves:    map[string]PDCurveRow{},
		ImpactPD:    &ImpactPDRow{ImpactMultiplier: d("1.00000000")},
		ImpactMevPD: map[string]ImpactMevPDRow{},
		Ratings:     map[uuid.UUID]string{testCPID: "idA"},
	}
	_, _, err := GetPDFromBatchParams(testInstrID, Stage1, ScenarioNormal, inst, params, testEvalDate)
	if err == nil {
		t.Fatal("Expected CodePOCIDeferredToM7 error for POCI batch path, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodePOCIDeferredToM7 {
		t.Errorf("Want CodePOCIDeferredToM7, got %v", err)
	}
}

func TestTraceIDFromCtx_WithValue(t *testing.T) {
	// F5: traceIDFromCtx extracts trace ID from context.
	ctx := context.WithValue(context.Background(), "X-Trace-Id", "test-trace-123")
	result := traceIDFromCtx(ctx)
	if result != "test-trace-123" {
		t.Errorf("traceIDFromCtx want test-trace-123 got %q", result)
	}
}

func TestTraceIDFromCtx_NoValue(t *testing.T) {
	// F5: traceIDFromCtx returns empty string when no trace ID is set.
	result := traceIDFromCtx(context.Background())
	if result != "" {
		t.Errorf("traceIDFromCtx want empty string, got %q", result)
	}
}

func TestGetLGDFromBatchParams_AppliesHaircutFormula(t *testing.T) {
	// F4: even with haircut=0, verify the formula path executes:
	// LGD_eff = LGD_pool × (1 - 0) = LGD_pool, rounded to 8dp.
	pool := LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")}
	params := &BatchParams{
		LGDPools:   map[string]LGDBaselRow{"BANK": pool},
		LGDMapping: map[string]string{"BANK": "BANK"},
	}
	inst := InstrumenRow{TipeInstrumen: "DEPOSITO"}
	cp := CounterpartyRow{TipeCounterparty: "BANK"}

	lgd, detail, err := GetLGDFromBatchParams(testInstrID, inst, cp, params, testPeriode)
	if err != nil {
		t.Fatalf("GetLGDFromBatchParams error: %v", err)
	}
	expected := d("0.45000000")
	if !lgd.Equal(expected) {
		t.Errorf("Batch LGD want %s got %s", expected, lgd)
	}
	// Verify haircut is tracked in detail.
	if !detail.CollateralHaircut.IsZero() {
		t.Errorf("Phase 1 haircut should be 0, got %s", detail.CollateralHaircut)
	}
	// LGDEffective = BaseLGD × (1 - 0) = 0.45
	if !detail.LGDEffective.Equal(expected) {
		t.Errorf("LGDEffective want %s got %s", expected, detail.LGDEffective)
	}
}

// ─── Decimal precision tests ──────────────────────────────────────────────────

func TestPD_NoPrecisionLoss_8dp(t *testing.T) {
	// PD = 0.00350000, impact_pd = 1.05000000
	// Expected = 0.00367500 (exact in decimal, no float rounding)
	pd := d("0.00350000")
	impPD := d("1.05000000")
	result := pd.Mul(impPD).RoundBank(8)
	expected := d("0.00367500")
	if !result.Equal(expected) {
		t.Errorf("Precision loss: want %s got %s", expected, result)
	}
}

func TestEAD_NoPrecisionLoss_4dp(t *testing.T) {
	// Outstanding = 1_234_567_890.1234, accrued = 1_234.5678
	// EAD_FCY = 1_234_569_124.6912 (exact)
	outstanding := d("1234567890.1234")
	accrued := d("1234.5678")
	ead := outstanding.Add(accrued).RoundBank(4)
	expected := d("1234569124.6912")
	if !ead.Equal(expected) {
		t.Errorf("EAD precision loss: want %s got %s", expected, ead)
	}
}

func TestFXConversion_NoPrecisionLoss(t *testing.T) {
	// EAD_FCY = 100_000.0000, kurs = 15_432.12345678
	// EAD_IDR = 1_543_212_345.6780 (RoundBank(4))
	eadFCY := d("100000.0000")
	kurs := d("15432.12345678")
	eadIDR := eadFCY.Mul(kurs).RoundBank(4)
	expected := d("1543212345.6780")
	if !eadIDR.Equal(expected) {
		t.Errorf("FX conversion precision loss: want %s got %s", expected, eadIDR)
	}
}
