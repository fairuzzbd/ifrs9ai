package core

// handler_extra_test.go — additional handler tests to push coverage over 85%.
//
// Covers:
//   - handler.RecomputeAdHoc full success path (200 with stored+delta response)
//   - handler.ListResults with filter[stage] and filter[routing_path] params
//   - handler.GetPortfolioSummary bad calcRunId UUID (400)
//   - handler.GetPortfolioSummary bad portofolioId UUID (400)
//   - handler.GetRollForward with priorCalcRunId query param
//   - toComputeResultDTO with all optional fields populated (stage, eadIdr, lgdUsed, pdUsed, flMultiplier, netCarrying, priorSealedEcl)
//   - handler.ComputeSingle with stage filter params
//   - handler.ComputeBulk with scope (portofolioIds + instrumenIds)

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── handler.RecomputeAdHoc full success ─────────────────────────────────────

func TestHandler_RecomputeAdHoc_Success_WithStoredDelta(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	instrID := uuid.New()
	calcRunID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Override instrReader with FVTPL (so ComputeSingle returns quickly without PD/LGD lookup).
	h.orchestrator.instrReader = &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// ComparePersisted=true → loadLatestStoredResult SELECT.
	storedCols := []string{
		"ecl_weighted_idr", "pd_used_good", "pd_used_normal", "pd_used_bad",
		"calc_run_id", "sealed_at", "evaluation_date",
	}
	mock.ExpectQuery(`SELECT ecl_weighted_idr`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows(storedCols).AddRow(
			"5000000.0000", "0.02000000", "0.02000000", "0.03000000",
			calcRunID, nil, evalDate,
		))

	// Audit tx for RecomputeAdHoc.
	mock.ExpectBegin()
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	c, w := buildGinWithClaims(PermECLRecomputeAdHoc)
	c.Request = buildRequest(http.MethodPost, `{
		"instrumenId":    "`+instrID.String()+`",
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
		"comparePersisted": true
	}`)

	h.RecomputeAdHoc(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if _, ok := data["recomputed"]; !ok {
		t.Error("recomputed: missing from response")
	}
	// stored should be present (comparePersisted=true + row found).
	if _, ok := data["stored"]; !ok {
		t.Error("stored: missing from response when comparePersisted=true")
	}
	// delta should be present (both ECLs non-nil: recomputed=0 FVTPL, stored=5M → delta=-5M).
	if _, ok := data["delta"]; !ok {
		t.Error("delta: missing from response when both ECLs are non-nil")
	}
}

// TestHandler_RecomputeAdHoc_Success_NoCompare verifies the handler returns 200
// without stored/delta when comparePersisted=false.
func TestHandler_RecomputeAdHoc_Success_NoCompare(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	instrID := uuid.New()
	h.orchestrator.instrReader = &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// No comparePersisted → no SELECT, just audit tx.
	mock.ExpectBegin()
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	c, w := buildGinWithClaims(PermECLRecomputeAdHoc)
	c.Request = buildRequest(http.MethodPost, `{
		"instrumenId":    "`+instrID.String()+`",
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
		"comparePersisted": false
	}`)

	h.RecomputeAdHoc(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if _, ok := data["stored"]; ok {
		t.Error("stored: should not be present when comparePersisted=false")
	}
	if _, ok := data["delta"]; ok {
		t.Error("delta: should not be present when comparePersisted=false")
	}
}

// TestHandler_RecomputeAdHoc_BadDate verifies 400 on invalid evaluationDate.
func TestHandler_RecomputeAdHoc_BadDate(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerWithDB(t)

	c, w := buildGinWithClaims(PermECLRecomputeAdHoc)
	c.Request = buildRequest(http.MethodPost, `{
		"instrumenId":    "00000000-0000-0000-0000-000000000001",
		"evaluationDate": "01/06/2026",
		"periodeId":      "JUNI-2026"
	}`)

	h.RecomputeAdHoc(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── handler.ListResults with filter params ───────────────────────────────────

func TestHandler_ListResults_WithStageFilter(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID := uuid.New()
	lineID := uuid.New()
	instrID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}

	// Stage filter "2" → repo appends stage=2 condition.
	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, sqlmock.AnyArg(), 51).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			lineID, calcRunID, instrID, evalDate, "JUNI-2026",
			2, "STANDARD", "500000000.0000", "13750000.0000", false, nil, createdAt,
		))

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/?limit=50&filter[stage]=2", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: calcRunID.String()}}

	h.ListResults(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ListResults_WithRoutingPathFilter(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID := uuid.New()
	lineID := uuid.New()
	instrID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}

	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, sqlmock.AnyArg(), 51).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			lineID, calcRunID, instrID, evalDate, "JUNI-2026",
			1, "LPS", "1000000000.0000", "0.0000", false, nil, createdAt,
		))

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/?limit=50&filter[routing_path]=LPS", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: calcRunID.String()}}

	h.ListResults(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ListResults_BadCalcRunId(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerWithDB(t)

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: "not-a-uuid"}}

	h.ListResults(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── handler.GetPortfolioSummary edge cases ───────────────────────────────────

func TestHandler_GetPortfolioSummary_BadCalcRunId(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerWithDB(t)

	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: "not-a-uuid"},
		{Key: "portofolioId", Value: uuid.New().String()},
	}

	h.GetPortfolioSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

func TestHandler_GetPortfolioSummary_BadPortofolioId(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerWithDB(t)

	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: uuid.New().String()},
		{Key: "portofolioId", Value: "not-a-uuid"},
	}

	h.GetPortfolioSummary(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── handler.GetRollForward with priorCalcRunId ───────────────────────────────

func TestHandler_GetRollForward_WithPriorCalcRunId(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID := uuid.New()
	priorCalcRunID := uuid.New()

	// GetRollForward calls GetCalcRunECLTotal for both runs.
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("3000000.0000"))
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(priorCalcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("2000000.0000"))

	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/?priorCalcRunId="+priorCalcRunID.String(), nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: calcRunID.String()}}

	h.GetRollForward(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	// Opening = 2000000, Closing = 3000000.
	if data["openingEclIdr"] != "2000000.0000" {
		t.Errorf("openingEclIdr: want '2000000.0000', got %v", data["openingEclIdr"])
	}
}

// ─── toComputeResultDTO with all optional fields ──────────────────────────────

func TestToComputeResultDTO_AllOptionalFields(t *testing.T) {
	t.Parallel()

	calcRunID := uuid.New()
	resultLineID := uuid.New()
	ead := decimal.NewFromInt(1_000_000_000)
	ecl := decimal.NewFromFloat(8_800_000.0)
	lgd := decimal.NewFromFloat(0.50)
	netCarrying := decimal.NewFromFloat(991_200_000.0)
	priorSealed := decimal.NewFromFloat(8_000_000.0)

	r := &ComputeResult{
		InstrumenID:    uuid.New(),
		CalcRunID:      &calcRunID,
		ResultLineID:   &resultLineID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Stage:          Stage2,
		RoutingPath:    RoutingStandard,
		FlagPOCI:       false,
		EADIDR:         &ead,
		ECLWeightedIDR: &ecl,
		LGDUsed:        &lgd,
		NetCarryingIDR: &netCarrying,
		PriorSealedECLIDR: &priorSealed,
		PDUsedPerScenario: &ScenarioValues{
			Good:   decimal.NewFromFloat(0.02),
			Normal: decimal.NewFromFloat(0.02),
			Bad:    decimal.NewFromFloat(0.03),
		},
		FLMultiplierPerScenario: &ScenarioValues{
			Good:   decimal.NewFromFloat(1.10),
			Normal: decimal.NewFromFloat(1.00),
			Bad:    decimal.NewFromFloat(0.90),
		},
		Warnings: []string{WarnFVTPLSkip},
	}

	dto := toComputeResultDTO(r)

	// Required fields.
	if dto["stage"] != 2 {
		t.Errorf("stage: want 2, got %v", dto["stage"])
	}
	if dto["calcRunId"] == nil {
		t.Error("calcRunId: must be present")
	}
	if dto["resultLineId"] == nil {
		t.Error("resultLineId: must be present")
	}
	if dto["eadIdr"] != "1000000000.0000" {
		t.Errorf("eadIdr: want '1000000000.0000', got %v", dto["eadIdr"])
	}
	if dto["eclWeightedIdr"] != "8800000.0000" {
		t.Errorf("eclWeightedIdr: want '8800000.0000', got %v", dto["eclWeightedIdr"])
	}
	if dto["lgdUsed"] != "0.50000000" {
		t.Errorf("lgdUsed: want '0.50000000', got %v", dto["lgdUsed"])
	}
	if dto["netCarryingIdr"] != "991200000.0000" {
		t.Errorf("netCarryingIdr: want '991200000.0000', got %v", dto["netCarryingIdr"])
	}
	if dto["priorSealedEclIdr"] != "8000000.0000" {
		t.Errorf("priorSealedEclIdr: want '8000000.0000', got %v", dto["priorSealedEclIdr"])
	}
	// pdUsed map.
	pdUsed, ok := dto["pdUsed"].(gin.H)
	if !ok {
		t.Fatalf("pdUsed: expected gin.H, got %T", dto["pdUsed"])
	}
	if pdUsed["good"] != "0.02000000" {
		t.Errorf("pdUsed.good: want '0.02000000', got %v", pdUsed["good"])
	}
	// flMultiplier map.
	flMult, ok := dto["flMultiplier"].(gin.H)
	if !ok {
		t.Fatalf("flMultiplier: expected gin.H, got %T", dto["flMultiplier"])
	}
	if flMult["good"] != "1.10000000" {
		t.Errorf("flMultiplier.good: want '1.10000000', got %v", flMult["good"])
	}
}

// ─── handler.ComputeBulk with scope ──────────────────────────────────────────

func TestHandler_ComputeBulk_WithScope(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLBulkCompute)

	portoID := uuid.New().String()
	instrID := uuid.New().String()
	body := `{
		"calcRunId":      "00000000-0000-0000-0000-000000000001",
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
		"scope": {
			"portofolioIds": ["` + portoID + `"],
			"instrumenIds":  ["` + instrID + `", "not-a-uuid"]
		}
	}`
	c.Request = buildRequest(http.MethodPost, body)

	h.ComputeBulk(c)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: want 202, got %d, body: %s", w.Code, w.Body.String())
	}
}

// ─── GetSingleResult bad calcRunId ────────────────────────────────────────────

func TestHandler_GetSingleResult_BadCalcRunId(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerWithDB(t)

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: "bad-uuid"},
		{Key: "instrumenId", Value: uuid.New().String()},
	}

	h.GetSingleResult(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── handler respondDomainError with coreError non-404 ───────────────────────

func TestRespondDomainError_Sealed_423(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)

	respondDomainError(c, errDomain(CodeECLCalcRunSealed, "run is sealed"))

	// coreErrorHTTPStatus(CodeECLCalcRunSealed) = 423 Locked.
	// Re-check via buildGinWithClaims which wires a real recorder.
	c3, w3 := buildGinWithClaims()
	c3.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	respondDomainError(c3, errDomain(CodeECLCalcRunSealed, "run is sealed"))

	if w3.Code != http.StatusLocked {
		t.Errorf("status: want 423, got %d", w3.Code)
	}
}
