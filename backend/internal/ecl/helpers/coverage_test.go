// Package helpers — additional coverage tests targeting specific uncovered paths.
//
// Covers:
//   - processOne (bulk worker) via direct BatchParams construction
//   - EclStage.IsValid / EclScenario.IsValid
//   - domainErrMsg (handler helper)
//   - GetPreview via stub previewRepo
//   - ComputeEADFromBatchParams FCY path
//   - GetPDFromBatchParams Stage2 / missing rating edge cases
//   - GetLGDFromBatchParams missing mapping
package helpers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── EclStage.IsValid / EclScenario.IsValid ──────────────────────────────────

func TestEclStage_IsValid(t *testing.T) {
	cases := []struct {
		stage EclStage
		want  bool
	}{
		{Stage1, true},
		{Stage2, true},
		{Stage3, true},
		{EclStage("STAGE_4"), false},
		{EclStage(""), false},
	}
	for _, tc := range cases {
		if got := tc.stage.IsValid(); got != tc.want {
			t.Errorf("EclStage(%q).IsValid() = %v, want %v", tc.stage, got, tc.want)
		}
	}
}

func TestEclScenario_IsValid(t *testing.T) {
	cases := []struct {
		scenario EclScenario
		want     bool
	}{
		{ScenarioGood, true},
		{ScenarioNormal, true},
		{ScenarioBad, true},
		{EclScenario("PESSIMISTIC"), false},
		{EclScenario(""), false},
	}
	for _, tc := range cases {
		if got := tc.scenario.IsValid(); got != tc.want {
			t.Errorf("EclScenario(%q).IsValid() = %v, want %v", tc.scenario, got, tc.want)
		}
	}
}

// ─── domainErrMsg ─────────────────────────────────────────────────────────────

func TestDomainErrMsg_DomainError(t *testing.T) {
	err := domainerrors.New(domainerrors.CodePDLookupRatingMissing, "rating tidak ditemukan")
	msg := domainErrMsg(err)
	if msg == "" {
		t.Error("Expected non-empty message for domain error")
	}
}

func TestDomainErrMsg_GenericError(t *testing.T) {
	err := fmt.Errorf("db connection lost")
	msg := domainErrMsg(err)
	if msg != err.Error() {
		t.Errorf("Expected original error string, got %q", msg)
	}
}

// ─── processOne via bulkHelperService ────────────────────────────────────────

// buildTestBulkService creates a bulkHelperService with stub repos suitable for processOne tests.
func buildTestBulkService(instrType, klasifikasi, tipe string, stage EclStage) *bulkHelperService {
	instrID := uuid.New()
	cpID := uuid.New()

	jt := testEvalDate.AddDate(2, 0, 0)
	inst := &InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: klasifikasi,
		TipeInstrumen:     instrType,
		MatauangKode:      "IDR",
		Nominal:           d("500000000.0000"),
		Status:            "AKTIF",
		TanggalJatuhTempo: &jt,
	}

	pdRepo := &stubPDRepo{
		curve:    &PDCurveRow{Rating: "idAA", PD12Month: d("0.00350000")},
		impactPD: &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		rating:   "idAA",
	}
	lgdRepo := &stubLGDRepo{
		pool:    &LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")},
		mapping: map[string]string{tipe: "BANK"},
	}
	instrRepo := &stubInstrRepo{inst: inst, stage: stage}
	cpRepo := &stubCPRepo{tipe: tipe}
	kursRepo := &stubKursRepo{}
	ccfRepo := &stubCCFRepo{
		table: map[string]decimal.Decimal{instrType: decimal.Zero},
	}

	return &bulkHelperService{
		pdRepo:      pdRepo,
		lgdRepo:     lgdRepo,
		instrRepo:   instrRepo,
		cpRepo:      cpRepo,
		kursRepo:    kursRepo,
		ccfRepo:     ccfRepo,
		auditWriter: nil,
		db:          nil,
	}
}

func TestProcessOne_HappyPath_Stage1(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	jt := testEvalDate.AddDate(2, 0, 0)
	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "DEPOSITO",
		MatauangKode:      "IDR",
		Nominal:           d("500000000.0000"),
		TanggalJatuhTempo: &jt,
	}

	params := &BatchParams{
		Instruments:   map[uuid.UUID]InstrumenRow{instrID: inst},
		CurrentStages: map[uuid.UUID]EclStage{instrID: Stage1},
		Ratings:       map[uuid.UUID]string{cpID: "idAA"},
		PDCurves:      map[string]PDCurveRow{"idAA": {Rating: "idAA", PD12Month: d("0.00350000")}},
		ImpactPD:      &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		ImpactMevPD: map[string]ImpactMevPDRow{
			"GOOD": {Scenario: "GOOD", PeriodeID: testPeriode, ImpactMultiplier: d("0.85000000")},
			"BAD":  {Scenario: "BAD", PeriodeID: testPeriode, ImpactMultiplier: d("1.25000000")},
		},
		Counterparties:    map[uuid.UUID]CounterpartyRow{cpID: {ID: cpID, TipeCounterparty: "BANK"}},
		LGDPools:          map[string]LGDBaselRow{"BANK": {TipeEksposur: "BANK", LGD: d("0.45000000")}},
		LGDMapping:        map[string]string{"BANK": "BANK"},
		FXRates:           map[string]KursRow{},
		EIRSchedules:      map[uuid.UUID]EIRScheduleRow{},
		CCFTable:          map[string]decimal.Decimal{"DEPOSITO": decimal.Zero},
		CollateralHaircut: map[string]decimal.Decimal{},
	}

	svc := buildTestBulkService("DEPOSITO", "AC", "BANK", Stage1)

	req := BulkRequest{InstrumenID: instrID}
	result, bulkErr, skipped := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)

	if bulkErr != nil {
		t.Fatalf("processOne returned error: %+v", bulkErr)
	}
	if skipped != nil {
		t.Fatalf("processOne returned skipped: %+v", skipped)
	}
	if result == nil {
		t.Fatal("processOne returned nil result")
	}

	// PDNormal = 0.0035 × 1.05 × 1.0 = 0.003675
	expectedPD := d("0.00367500")
	if !result.PDNormal.Equal(expectedPD) {
		t.Errorf("PDNormal want %s got %s", expectedPD, result.PDNormal)
	}
	if !result.LGD.Equal(d("0.45000000")) {
		t.Errorf("LGD want 0.45 got %s", result.LGD)
	}
	if !result.EADIDR.Equal(d("500000000.0000")) {
		t.Errorf("EADIDR want 500000000 got %s", result.EADIDR)
	}
}

func TestProcessOne_InstrumentNotInBatch_Error(t *testing.T) {
	instrID := uuid.New()
	params := &BatchParams{
		Instruments:    map[uuid.UUID]InstrumenRow{}, // empty
		CurrentStages:  map[uuid.UUID]EclStage{},
		Ratings:        map[uuid.UUID]string{},
		PDCurves:       map[string]PDCurveRow{},
		ImpactMevPD:    map[string]ImpactMevPDRow{},
		Counterparties: map[uuid.UUID]CounterpartyRow{},
		LGDPools:       map[string]LGDBaselRow{},
		LGDMapping:     map[string]string{},
		FXRates:        map[string]KursRow{},
		EIRSchedules:   map[uuid.UUID]EIRScheduleRow{},
		CCFTable:       map[string]decimal.Decimal{},
	}

	svc := buildTestBulkService("DEPOSITO", "AC", "BANK", Stage1)

	req := BulkRequest{InstrumenID: instrID}
	result, bulkErr, skipped := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)

	if result != nil {
		t.Error("Expected nil result for missing instrument")
	}
	if skipped != nil {
		t.Error("Expected nil skipped")
	}
	if bulkErr == nil {
		t.Fatal("Expected error for missing instrument")
	}
}

func TestProcessOne_FVTPL_Skipped(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "FVTPL",
		TipeInstrumen:     "SAHAM",
		MatauangKode:      "IDR",
		Nominal:           d("100000000.0000"),
	}

	params := &BatchParams{
		Instruments:   map[uuid.UUID]InstrumenRow{instrID: inst},
		CurrentStages: map[uuid.UUID]EclStage{},
		Ratings:       map[uuid.UUID]string{},
		PDCurves:      map[string]PDCurveRow{},
		ImpactMevPD:   map[string]ImpactMevPDRow{},
	}

	svc := buildTestBulkService("SAHAM", "FVTPL", "BANK", Stage1)

	req := BulkRequest{InstrumenID: instrID}
	result, bulkErr, skipped := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)

	if result != nil || bulkErr != nil {
		t.Error("Expected nil result and nil error for FVTPL skip")
	}
	if skipped == nil {
		t.Fatal("Expected skipped for FVTPL instrument")
	}
	if skipped.KlasifikasiPsak71 != "FVTPL" {
		t.Errorf("KlasifikasiPsak71 want FVTPL got %s", skipped.KlasifikasiPsak71)
	}
}

func TestProcessOne_Stage3_PD_One(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "DEPOSITO",
		MatauangKode:      "IDR",
		Nominal:           d("500000000.0000"),
	}

	params := &BatchParams{
		Instruments:   map[uuid.UUID]InstrumenRow{instrID: inst},
		CurrentStages: map[uuid.UUID]EclStage{instrID: Stage3},
		Ratings:       map[uuid.UUID]string{},
		PDCurves:      map[string]PDCurveRow{},
		// Stage3 ignores ImpactMevPD — no entries needed
		ImpactMevPD:       map[string]ImpactMevPDRow{},
		Counterparties:    map[uuid.UUID]CounterpartyRow{cpID: {ID: cpID, TipeCounterparty: "BANK"}},
		LGDPools:          map[string]LGDBaselRow{"BANK": {TipeEksposur: "BANK", LGD: d("0.45000000")}},
		LGDMapping:        map[string]string{"BANK": "BANK"},
		FXRates:           map[string]KursRow{},
		EIRSchedules:      map[uuid.UUID]EIRScheduleRow{},
		CCFTable:          map[string]decimal.Decimal{"DEPOSITO": decimal.Zero},
		CollateralHaircut: map[string]decimal.Decimal{},
	}

	svc := buildTestBulkService("DEPOSITO", "AC", "BANK", Stage3)

	req := BulkRequest{InstrumenID: instrID}
	result, bulkErr, skipped := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)

	if bulkErr != nil {
		t.Fatalf("processOne Stage3 error: %+v", bulkErr)
	}
	if skipped != nil {
		t.Fatalf("unexpected skip")
	}
	if result == nil {
		t.Fatal("nil result")
	}

	// Stage 3: all PD scenarios = 1.0
	if !result.PDGood.Equal(d("1.00000000")) {
		t.Errorf("PDGood Stage3 want 1.0 got %s", result.PDGood)
	}
	if !result.PDBad.Equal(d("1.00000000")) {
		t.Errorf("PDBad Stage3 want 1.0 got %s", result.PDBad)
	}
}

func TestProcessOne_LGD_MappingError_ReturnsBulkErr(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "OBLIGASI",
		MatauangKode:      "IDR",
		Nominal:           d("200000000.0000"),
	}

	params := &BatchParams{
		Instruments:   map[uuid.UUID]InstrumenRow{instrID: inst},
		CurrentStages: map[uuid.UUID]EclStage{instrID: Stage1},
		Ratings:       map[uuid.UUID]string{cpID: "idAA"},
		PDCurves:      map[string]PDCurveRow{"idAA": {Rating: "idAA", PD12Month: d("0.00350000")}},
		ImpactPD:      &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		ImpactMevPD: map[string]ImpactMevPDRow{
			"GOOD": {Scenario: "GOOD", PeriodeID: testPeriode, ImpactMultiplier: d("0.85000000")},
			"BAD":  {Scenario: "BAD", PeriodeID: testPeriode, ImpactMultiplier: d("1.25000000")},
		},
		Counterparties:    map[uuid.UUID]CounterpartyRow{cpID: {ID: cpID, TipeCounterparty: "UNKNOWN_TYPE"}},
		LGDPools:          map[string]LGDBaselRow{},
		LGDMapping:        map[string]string{}, // mapping missing → error
		FXRates:           map[string]KursRow{},
		EIRSchedules:      map[uuid.UUID]EIRScheduleRow{},
		CCFTable:          map[string]decimal.Decimal{"OBLIGASI": decimal.Zero},
		CollateralHaircut: map[string]decimal.Decimal{},
	}

	svc := buildTestBulkService("OBLIGASI", "AC", "UNKNOWN_TYPE", Stage1)

	req := BulkRequest{InstrumenID: instrID}
	_, bulkErr, _ := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)
	if bulkErr == nil {
		t.Error("Expected bulkErr for missing LGD mapping")
	}
}

// ─── ComputeEADFromBatchParams FCY path ───────────────────────────────────────

func TestComputeEADFromBatchParams_FCY(t *testing.T) {
	instrID := uuid.New()
	inst := InstrumenRow{
		ID:            instrID,
		MatauangKode:  "USD",
		Nominal:       d("100000.0000"),
		TipeInstrumen: "OBLIGASI",
	}
	params := &BatchParams{
		FXRates: map[string]KursRow{
			"USD": {KodeMatauang: "USD", NilaiKurs: d("15432.12345678"), WorkflowStatus: "APPROVED"},
		},
		EIRSchedules: map[uuid.UUID]EIRScheduleRow{},
		CCFTable:     map[string]decimal.Decimal{"OBLIGASI": decimal.Zero},
	}

	eadIDR, bd, err := ComputeEADFromBatchParams(instrID, inst, params, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEADFromBatchParams FCY error: %v", err)
	}

	// EAD_IDR = 100000 × 15432.12345678 = 1543212345.6780
	expected := d("100000.0000").Mul(d("15432.12345678"))
	if !eadIDR.Equal(expected.RoundBank(4)) {
		t.Errorf("EAD_IDR want %s got %s", expected.RoundBank(4), eadIDR)
	}
	if bd.Currency != "USD" {
		t.Errorf("Currency want USD got %s", bd.Currency)
	}
	if bd.FXRate == nil || !bd.FXRate.Equal(d("15432.12345678")) {
		t.Errorf("FXRate mismatch")
	}
}

func TestComputeEADFromBatchParams_FCY_MissingKurs(t *testing.T) {
	instrID := uuid.New()
	inst := InstrumenRow{
		ID: instrID, MatauangKode: "EUR", Nominal: d("50000.0000"), TipeInstrumen: "OBLIGASI",
	}
	params := &BatchParams{
		FXRates:      map[string]KursRow{}, // EUR not available
		EIRSchedules: map[uuid.UUID]EIRScheduleRow{},
		CCFTable:     map[string]decimal.Decimal{"OBLIGASI": decimal.Zero},
	}

	_, _, err := ComputeEADFromBatchParams(instrID, inst, params, testEvalDate)
	if err == nil {
		t.Error("Expected error for missing FX rate")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Errorf("Expected domain error, got %T", err)
	}
	if de.Code() != domainerrors.CodeEADFXRateMissing {
		t.Errorf("Code want EAD_FX_RATE_MISSING got %s", de.Code())
	}
}

// ─── GetPreview with stub previewRepo ─────────────────────────────────────────

// stubPreviewRepo implements previewInstrumentLister.
type stubPreviewRepo struct {
	rows      []InstrumenRow
	cursor    string
	hasMore   bool
}

func (s *stubPreviewRepo) ListECLApplicableInstruments(
	_ context.Context, _, _, _, _, _ string, _ *bool, _, _, _, _ string, _ int,
) ([]InstrumenRow, string, bool, error) {
	return s.rows, s.cursor, s.hasMore, nil
}

func TestHandler_GetPreview_200_WithStubRepo(t *testing.T) {
	instrID := uuid.New()
	rows := []InstrumenRow{{
		ID:                instrID,
		KodeInstrumen:     "INST-001",
		TipeInstrumen:     "DEPOSITO",
		KlasifikasiPsak71: "AC",
		MatauangKode:      "IDR",
	}}

	bulkSvc := &stubBulkSvc{
		results: []BulkResult{{InstrumenID: instrID}},
		summary: BulkSummary{Total: 1, Success: 1},
	}

	h := &Handler{
		svc:         &Services{Bulk: bulkSvc},
		previewRepo: &stubPreviewRepo{rows: rows, cursor: "", hasMore: false},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermECLHelpersPreview})
		c.Next()
	})
	RegisterRoutes(r.Group("/api/v1/ecl"), h)

	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/preview?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30", nil)
	w := doRequest(r, req)
	if w.Code != http.StatusOK {
		t.Errorf("GetPreview stub status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// ─── BulkLookup summary Success/Warning counters ─────────────────────────────

// pdRepoWithImpactMev extends stubPDRepo to return both GOOD+BAD ImpactMevPD rows.
type pdRepoWithImpactMev struct {
	*stubPDRepo
	mevMap map[string]ImpactMevPDRow
}

func (p *pdRepoWithImpactMev) BatchLoadImpactMevPD(_ context.Context, _ string) (map[string]ImpactMevPDRow, error) {
	return p.mevMap, nil
}
func (p *pdRepoWithImpactMev) GetActiveImpactMevPD(_ context.Context, scenario, _ string) (*ImpactMevPDRow, error) {
	if row, ok := p.mevMap[scenario]; ok {
		return &row, nil
	}
	return nil, nil
}

func TestBulkLookup_SummaryCounters(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	jt := testEvalDate.AddDate(2, 0, 0)
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID:                instrID,
			CounterpartyID:    cpID,
			KlasifikasiPsak71: "AC",
			TipeInstrumen:     "DEPOSITO",
			MatauangKode:      "IDR",
			Nominal:           d("500000000.0000"),
			Status:            "AKTIF",
			TanggalJatuhTempo: &jt,
		},
		stage: Stage1,
	}
	base := &stubPDRepo{
		curve:    &PDCurveRow{Rating: "idAA", PD12Month: d("0.00350000")},
		impactPD: &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		rating:   "idAA",
	}
	pdRepo := &pdRepoWithImpactMev{
		stubPDRepo: base,
		mevMap: map[string]ImpactMevPDRow{
			"GOOD": {Scenario: "GOOD", PeriodeID: testPeriode, ImpactMultiplier: d("0.85000000")},
			"BAD":  {Scenario: "BAD", PeriodeID: testPeriode, ImpactMultiplier: d("1.25000000")},
		},
	}
	lgdRepo := &stubLGDRepo{
		pool:    &LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")},
		mapping: map[string]string{"BANK": "BANK"},
	}
	cpRepo := &stubCPRepo{tipe: "BANK"}
	kursRepo := &stubKursRepo{}
	ccfRepo := &stubCCFRepo{table: map[string]decimal.Decimal{"DEPOSITO": decimal.Zero}}

	svc := NewBulkHelperService(pdRepo, lgdRepo, instrRepo, cpRepo, kursRepo, ccfRepo, nil, nil)

	reqs := []BulkRequest{{InstrumenID: instrID}}
	results, summary, errs, skipped, err := svc.BulkLookup(
		context.Background(), reqs, testPeriode, testEvalDate,
	)
	if err != nil {
		t.Fatalf("BulkLookup error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if summary.Total != 1 {
		t.Errorf("summary.Total want 1 got %d", summary.Total)
	}
	if summary.Success != 1 {
		t.Errorf("summary.Success want 1 got %d", summary.Success)
	}
	if len(errs) != 0 {
		t.Errorf("Expected 0 errors, got %d: %+v", len(errs), errs)
	}
	_ = skipped
}

// ─── Error-returning stubs for loadBatchParams branches ──────────────────────

// errInstrRepo returns an error from BatchLoadInstruments.
type errInstrRepo struct {
	*stubInstrRepo
}

func (e *errInstrRepo) BatchLoadInstruments(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]InstrumenRow, error) {
	return nil, fmt.Errorf("db down")
}

// errCPRepo returns an error from BatchLoadCounterparties.
type errCPRepo struct {
	*stubCPRepo
}

func (e *errCPRepo) BatchLoadCounterparties(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]CounterpartyRow, error) {
	return nil, fmt.Errorf("db down")
}

// errLGDRepoPool returns an error from BatchLoadLGDPools.
type errLGDRepoPool struct {
	*stubLGDRepo
}

func (e *errLGDRepoPool) BatchLoadLGDPools(ctx context.Context, _ string) (map[string]LGDBaselRow, error) {
	return nil, fmt.Errorf("db down")
}

func newBaseStubs() (*stubPDRepo, *stubLGDRepo, *stubInstrRepo, *stubCPRepo, *stubKursRepo, *stubCCFRepo) {
	instrID := uuid.New()
	cpID := uuid.New()
	jt := testEvalDate.AddDate(2, 0, 0)
	pd := &stubPDRepo{
		curve:    &PDCurveRow{Rating: "idAA", PD12Month: d("0.00350000")},
		impactPD: &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		rating:   "idAA",
	}
	lgd := &stubLGDRepo{
		pool:    &LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")},
		mapping: map[string]string{"BANK": "BANK"},
	}
	instr := &stubInstrRepo{
		inst: &InstrumenRow{
			ID: instrID, CounterpartyID: cpID,
			KlasifikasiPsak71: "AC", TipeInstrumen: "DEPOSITO",
			MatauangKode: "IDR", Nominal: d("500000000.0000"),
			Status: "AKTIF", TanggalJatuhTempo: &jt,
		},
		stage: Stage1,
	}
	cp := &stubCPRepo{tipe: "BANK"}
	kurs := &stubKursRepo{}
	ccf := &stubCCFRepo{table: map[string]decimal.Decimal{"DEPOSITO": decimal.Zero}}
	return pd, lgd, instr, cp, kurs, ccf
}

func TestBulkLookup_LoadBatchParams_InstrumentError(t *testing.T) {
	pd, lgd, instr, cp, kurs, ccf := newBaseStubs()
	svc := NewBulkHelperService(pd, lgd, &errInstrRepo{instr}, cp, kurs, ccf, nil, nil)

	_, _, _, _, err := svc.BulkLookup(context.Background(),
		[]BulkRequest{{InstrumenID: uuid.New()}},
		testPeriode, testEvalDate)
	if err == nil {
		t.Error("Expected error when BatchLoadInstruments fails")
	}
}

func TestBulkLookup_LoadBatchParams_CounterpartyError(t *testing.T) {
	pd, lgd, instr, _, kurs, ccf := newBaseStubs()
	svc := NewBulkHelperService(pd, lgd, instr, &errCPRepo{&stubCPRepo{tipe: "BANK"}}, kurs, ccf, nil, nil)

	_, _, _, _, err := svc.BulkLookup(context.Background(),
		[]BulkRequest{{InstrumenID: uuid.New()}},
		testPeriode, testEvalDate)
	if err == nil {
		t.Error("Expected error when BatchLoadCounterparties fails")
	}
}

func TestBulkLookup_LoadBatchParams_LGDPoolError(t *testing.T) {
	pd, lgd, instr, cp, kurs, ccf := newBaseStubs()
	svc := NewBulkHelperService(pd, &errLGDRepoPool{lgd}, instr, cp, kurs, ccf, nil, nil)

	_, _, _, _, err := svc.BulkLookup(context.Background(),
		[]BulkRequest{{InstrumenID: uuid.New()}},
		testPeriode, testEvalDate)
	if err == nil {
		t.Error("Expected error when BatchLoadLGDPools fails")
	}
}

func TestBulkLookup_EmptyRequests_FastPath(t *testing.T) {
	pd, lgd, instr, cp, kurs, ccf := newBaseStubs()
	svc := NewBulkHelperService(pd, lgd, instr, cp, kurs, ccf, nil, nil)

	results, summary, errs, skipped, err := svc.BulkLookup(
		context.Background(), []BulkRequest{}, testPeriode, testEvalDate)
	if err != nil {
		t.Fatalf("Empty request error: %v", err)
	}
	if results != nil || errs != nil || skipped != nil {
		t.Error("Expected all nil for empty request")
	}
	_ = summary
}

// ─── processOne EAD FCY error ─────────────────────────────────────────────────

func TestProcessOne_EAD_FCY_MissingKurs_BulkErr(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "OBLIGASI",
		MatauangKode:      "USD",   // FCY — needs kurs
		Nominal:           d("100000.0000"),
	}

	params := &BatchParams{
		Instruments:   map[uuid.UUID]InstrumenRow{instrID: inst},
		CurrentStages: map[uuid.UUID]EclStage{instrID: Stage1},
		Ratings:       map[uuid.UUID]string{cpID: "idAA"},
		PDCurves:      map[string]PDCurveRow{"idAA": {Rating: "idAA", PD12Month: d("0.00350000")}},
		ImpactPD:      &ImpactPDRow{PeriodeID: testPeriode, ImpactMultiplier: d("1.05000000")},
		ImpactMevPD: map[string]ImpactMevPDRow{
			"GOOD": {Scenario: "GOOD", PeriodeID: testPeriode, ImpactMultiplier: d("0.85000000")},
			"BAD":  {Scenario: "BAD", PeriodeID: testPeriode, ImpactMultiplier: d("1.25000000")},
		},
		Counterparties:    map[uuid.UUID]CounterpartyRow{cpID: {ID: cpID, TipeCounterparty: "BANK"}},
		LGDPools:          map[string]LGDBaselRow{"BANK": {TipeEksposur: "BANK", LGD: d("0.45000000")}},
		LGDMapping:        map[string]string{"BANK": "BANK"},
		FXRates:           map[string]KursRow{}, // USD missing → EAD error
		EIRSchedules:      map[uuid.UUID]EIRScheduleRow{},
		CCFTable:          map[string]decimal.Decimal{"OBLIGASI": decimal.Zero},
		CollateralHaircut: map[string]decimal.Decimal{},
	}

	svc := buildTestBulkService("OBLIGASI", "AC", "BANK", Stage1)
	req := BulkRequest{InstrumenID: instrID}
	result, bulkErr, _ := svc.processOne(context.Background(), req, params, testPeriode, testEvalDate)

	if result != nil {
		t.Error("Expected nil result for EAD error")
	}
	if bulkErr == nil {
		t.Fatal("Expected bulkErr for missing USD FX rate")
	}
}

// ─── BulkGetPD invalid scenario branch ───────────────────────────────────────

func TestBulkGetPD_InvalidScenario_InResult(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)

	body := `{
		"periodeId": "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items": [{"instrumenId": "01927f6c-0000-7000-8000-000000000001", "stage": "STAGE_1", "scenario": "PESSIMISTIC"}]
	}`
	req, _ := http.NewRequest("POST", "/api/v1/ecl/helpers/pd/bulk", strings.NewReader(body))
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for per-item invalid scenario, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Handler parse helpers via HTTP routes ────────────────────────────────────

func TestHandler_ParseDateParam_BadFormat(t *testing.T) {
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)

	// GetEAD requires instrumenId (UUID) and evaluationDate — pass bad date
	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/ead?instrumenId=01927f6c-0000-7000-8000-000000000001&evaluationDate=30-06-2026", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for bad evaluationDate format, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ParseStage_Empty(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)

	// GetPD requires stage param — pass empty stage (missing entirely)
	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&scenario=NORMAL", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing stage, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ParseStage_InvalidValue(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)

	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&stage=STAGE_9&scenario=NORMAL", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid stage value, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ParseScenario_Empty(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)

	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&stage=STAGE_1", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing scenario, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ParseScenario_InvalidValue(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)

	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&stage=STAGE_1&scenario=PESSIMISTIC", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid scenario, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ParseUUIDParam_BadUUID(t *testing.T) {
	h := newTestHandler(nil, &stubLGDSvc{}, nil, nil, nil)
	r := newRouter(h)

	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/lgd?instrumenId=not-a-uuid&periodeId=PBUKU-2026-06", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersRead)
	w := doRequest(r, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for bad UUID instrumenId, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_TraceID_NoKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest("GET", "/", nil)
	// No trace key set — should return empty string (falls back to header)
	result := traceID(c)
	if result != "" {
		t.Errorf("Expected empty traceID, got %q", result)
	}
}

func TestHandler_TraceID_WithKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest("GET", "/", nil)
	// response.TraceIDKey = "X-Trace-Id"
	c.Set("X-Trace-Id", "abc-123-xyz")
	result := traceID(c)
	if result != "abc-123-xyz" {
		t.Errorf("Expected trace ID 'abc-123-xyz', got %q", result)
	}
}

func TestHandler_ParseLimitParam_Invalid(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{
		results: []BulkResult{},
		summary: BulkSummary{},
	})
	r := newRouter(h)

	// limit=abc should be clamped to default (50) — no 400, preview returns 200
	req, _ := http.NewRequest("GET", "/api/v1/ecl/helpers/preview?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&limit=abc", nil)
	req.Header.Set("X-Test-Perms", PermECLHelpersPreview)
	w := doRequest(r, req)
	// preview with nil previewRepo returns 200 (nil-DB dev fallback)
	if w.Code >= 500 {
		t.Errorf("Expected non-500 for invalid limit, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── helper: doRequest ───────────────────────────────────────────────────────

func doRequest(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
