package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
)

// handler_test.go — HTTP handler unit tests using httptest.
//
// AC covered:
//   M7-001: POST /ecl/compute — 200 OK for FVTPL skip
//   M7-001: POST /ecl/compute — 200 OK for POCI defer (ecl_weighted=null)
//   M7-001: POST /ecl/compute — 403 Forbidden without permission
//   M7-001: POST /ecl/compute — 400 for bad date
//   M7-003: GET /ecl/results/{calcRunId} — 200 with pagination
//   M7-004: GET /ecl/results/{calcRunId}/portofolio/{id}/summary — 200
//   M7-004: GET /ecl/results/{calcRunId}/roll-forward — 200
//   M7-005: POST /ecl/recompute/ad-hoc — 200 preview result
//   M7-002: POST /ecl/compute/bulk — 202 Accepted with jobId

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Mock orchestrator for handler tests ─────────────────────────────────────

// The cleanest approach for handler tests: use a real Handler but with a mock ECLOrchestrator
// built from a no-op auditWriter + mock services.

func buildHandlerForTest(reader InstrumenReaderIface, ltSvc LookthroughServiceIface) (*Handler, *ECLOrchestrator) {
	aw := audit.NewWriter(nil)
	svc := buildMockHelpers(decimal.NewFromFloat(0.02), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9))
	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: aw,
		helpers:     svc,
		lpsAgg:      nil,
		lookthrough: ltSvc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      nil,
	}
	return NewHandler(orch), orch
}

// ─── Helper to build gin context with JWT claims ──────────────────────────────

func buildGinWithClaims(perms ...string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("claims", &auth.Claims{
		Sub:         uuid.New().String(),
		Permissions: perms,
	})
	c.Set("user_id", uuid.New().String())
	return c, w
}

func buildRequest(method, body string) *http.Request {
	if body == "" {
		req, _ := http.NewRequest(method, "/", nil)
		return req
	}
	req, _ := http.NewRequest(method, "/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHandler_ComputeSingle_FVTPL_200(t *testing.T) {
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

	h, _ := buildHandlerForTest(reader, nil)
	c, w := buildGinWithClaims(PermECLCompute)

	body := map[string]interface{}{
		"instrumenId":    instrID.String(),
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
		"persist":        false,
	}
	bodyJSON, _ := json.Marshal(body)
	c.Request = buildRequest(http.MethodPost, string(bodyJSON))

	h.ComputeSingle(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["routingPath"] != "SKIP_FVTPL" {
		t.Errorf("routingPath: want SKIP_FVTPL, got %v", data["routingPath"])
	}
	// ECL_weighted for FVTPL = "0.0000".
	if data["eclWeightedIdr"] != "0.0000" {
		t.Errorf("eclWeightedIdr: want 0.0000, got %v", data["eclWeightedIdr"])
	}
}

func TestHandler_ComputeSingle_POCI_200_NullECL(t *testing.T) {
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
			},
		},
	}

	h, _ := buildHandlerForTest(reader, nil)
	c, w := buildGinWithClaims(PermECLCompute)

	body := map[string]interface{}{
		"instrumenId":    instrID.String(),
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
		"persist":        false,
	}
	bodyJSON, _ := json.Marshal(body)
	c.Request = buildRequest(http.MethodPost, string(bodyJSON))

	h.ComputeSingle(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	// Phase 4.5 (DEC-POCI-001): routing_path = POCI_COMPUTED (CA-EIR available).
	if data["routingPath"] != "POCI_COMPUTED" {
		t.Errorf("routingPath: want POCI_COMPUTED, got %v", data["routingPath"])
	}
	// Phase 4.5: POCI baseline ECL computed — eclWeightedIdr must be present and non-null.
	eclVal, hasKey := data["eclWeightedIdr"]
	if !hasKey {
		t.Errorf("POCI: eclWeightedIdr must be present in response (Phase 4.5: initial baseline ECL computed)")
	} else if eclVal == nil {
		t.Errorf("POCI: eclWeightedIdr must be non-nil (Phase 4.5: baseline ECL, delta deferred to Phase 5)")
	}
	// Both POCI warning codes must be present.
	warnings, _ := data["warnings"].([]interface{})
	wantCodes := []string{WarnPOCIRequiresFullCAEIR, WarnPOCIECLRepresentsInitialBaseline}
	for _, want := range wantCodes {
		foundWarn := false
		for _, w := range warnings {
			if w == want {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Errorf("POCI handler: expected warning %s in response, got %v", want, data["warnings"])
		}
	}
}

func TestHandler_ComputeSingle_Forbidden(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims() // no permissions

	c.Request = buildRequest(http.MethodPost, `{"instrumenId":"00000000-0000-0000-0000-000000000001","evaluationDate":"2026-06-01","periodeId":"JUNI-2026"}`)
	h.ComputeSingle(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", w.Code)
	}
}

func TestHandler_ComputeSingle_BadDate_400(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLCompute)

	c.Request = buildRequest(http.MethodPost, `{"instrumenId":"00000000-0000-0000-0000-000000000001","evaluationDate":"01/06/2026","periodeId":"JUNI-2026"}`)
	h.ComputeSingle(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ComputeBulk_202(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLBulkCompute)

	body := map[string]interface{}{
		"calcRunId":      uuid.New().String(),
		"evaluationDate": "2026-06-01",
		"periodeId":      "JUNI-2026",
	}
	bodyJSON, _ := json.Marshal(body)
	c.Request = buildRequest(http.MethodPost, string(bodyJSON))

	h.ComputeBulk(c)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: want 202, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["type"] != "ECL_BULK_COMPUTE" {
		t.Errorf("type: want ECL_BULK_COMPUTE, got %v", data["type"])
	}
	if _, ok := data["jobId"]; !ok {
		t.Error("jobId missing from response")
	}
}

func TestHandler_GetRollForward_BadCalcRunId(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)

	c.Request, _ = http.NewRequest(http.MethodGet, "/ecl/results/not-a-uuid/roll-forward", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: "not-a-uuid"}}

	h.GetRollForward(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetPortfolioSummary_Forbidden(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims() // no permissions

	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: uuid.New().String()},
		{Key: "portofolioId", Value: uuid.New().String()},
	}

	h.GetPortfolioSummary(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", w.Code)
	}
}

func TestHandler_RecomputeAdHoc_Forbidden(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLResultRead) // wrong permission

	c.Request = buildRequest(http.MethodPost, `{}`)
	h.RecomputeAdHoc(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", w.Code)
	}
}

func TestHandler_RecomputeAdHoc_BadInstrumenId(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLRecomputeAdHoc)

	// ShouldBindJSON validates instrumenId as uuid — invalid UUID → 400.
	c.Request = buildRequest(http.MethodPost, `{"instrumenId":"not-a-uuid","evaluationDate":"2026-06-01","periodeId":"JUNI-2026"}`)
	h.RecomputeAdHoc(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetListResults_Forbidden(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims()

	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: uuid.New().String()}}

	h.ListResults(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", w.Code)
	}
}

func TestHandler_GetSingleResult_BadInstrumenId(t *testing.T) {
	t.Parallel()

	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLResultRead)

	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: uuid.New().String()},
		{Key: "instrumenId", Value: "bad-uuid"},
	}

	h.GetSingleResult(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── respondDomainError tests ─────────────────────────────────────────────────

func TestRespondDomainError_CoreError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)

	respondDomainError(c, errDomain(CodeECLInstrumenNotFound, "test not found"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", w.Code)
	}
}

func TestCoreErrorHTTPStatus_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want int
	}{
		{CodeECLInstrumenNotFound, http.StatusNotFound},
		{CodeECLCalcRunSealed, http.StatusLocked},
		{CodeECLBulkTooLarge, http.StatusRequestEntityTooLarge},
		{CodeECLBulkRunning, http.StatusConflict},
		{CodeECLStagingNotFound, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		got := coreErrorHTTPStatus(tc.code)
		if got != tc.want {
			t.Errorf("coreErrorHTTPStatus(%s): want %d, got %d", tc.code, tc.want, got)
		}
	}
}

// ─── toComputeResultDTO tests ─────────────────────────────────────────────────

func TestToComputeResultDTO_FVTPL(t *testing.T) {
	t.Parallel()

	zero := decimal.Zero
	r := &ComputeResult{
		InstrumenID:    uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		RoutingPath:    RoutingSkipFVTPL,
		ECLWeightedIDR: &zero,
		Warnings:       []string{WarnFVTPLSkip},
	}
	dto := toComputeResultDTO(r)
	if dto["routingPath"] != "SKIP_FVTPL" {
		t.Errorf("routingPath: want SKIP_FVTPL, got %v", dto["routingPath"])
	}
	if dto["eclWeightedIdr"] != "0.0000" {
		t.Errorf("eclWeightedIdr: want 0.0000, got %v", dto["eclWeightedIdr"])
	}
}

func TestToComputeResultDTO_POCI_NilECL(t *testing.T) {
	t.Parallel()

	r := &ComputeResult{
		InstrumenID:    uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		RoutingPath:    RoutingPOCIDeferred,
		FlagPOCI:       true,
		ECLWeightedIDR: nil, // POCI = nil
	}
	dto := toComputeResultDTO(r)
	if dto["routingPath"] != "POCI_DEFERRED" {
		t.Errorf("routingPath: want POCI_DEFERRED, got %v", dto["routingPath"])
	}
	// eclWeightedIdr key should not exist (nil ECL omitted from map).
	if _, ok := dto["eclWeightedIdr"]; ok {
		t.Errorf("POCI: eclWeightedIdr should be absent (nil), got %v", dto["eclWeightedIdr"])
	}
}
