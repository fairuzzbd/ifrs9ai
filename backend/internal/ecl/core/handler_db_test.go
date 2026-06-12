package core

// handler_db_test.go — Handler tests that require a DB-backed orchestrator
// via sqlmock (for ListResults, GetSingleResult, GetPortfolioSummary, GetRollForward).
//
// These tests use buildOrchestratorWithDB defined in service_repo_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// buildHandlerWithDB creates a Handler backed by a sqlmock DB orchestrator.
func buildHandlerWithDB(t *testing.T) (*Handler, sqlmock.Sqlmock) {
	t.Helper()
	orch, mock := buildOrchestratorWithDB(t)
	return NewHandler(orch), mock
}

// ─── ListResults ─────────────────────────────────────────────────────────────

func TestHandler_ListResults_OK(t *testing.T) {
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
		WithArgs(calcRunID, 51).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			lineID, calcRunID, instrID, evalDate, "JUNI-2026",
			1, "STANDARD", "1000000000.0000", "4000000.0000", false, nil, createdAt,
		))

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/?limit=50", nil)
	c.Params = gin.Params{{Key: "calcRunId", Value: calcRunID.String()}}

	h.ListResults(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("items: want 1, got %d", len(data))
	}
}

// ─── GetSingleResult ─────────────────────────────────────────────────────────

func TestHandler_GetSingleResult_Found(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID := uuid.New()
	instrID := uuid.New()
	lineID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := resultLineRowCols()
	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			lineID, calcRunID, instrID, evalDate, "JUNI-2026", 1, "STANDARD",
			"1000000000.0000", "0.02000000", "0.02000000", "0.02000000", "0.40000000",
			"1.10000000", "1.00000000", "0.90000000",
			"8000000.0000", "8000000.0000", "8000000.0000",
			"8800000.0000", "8000000.0000", "7200000.0000",
			"8200000.0000", "0.2500", "0.5000", "0.2500",
			nil, nil, false, nil,
			"[]", nil, createdAt,
		))

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: calcRunID.String()},
		{Key: "instrumenId", Value: instrID.String()},
	}

	h.GetSingleResult(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetSingleResult_NotFound(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID, instrID := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows(resultLineRowCols())) // empty

	c, w := buildGinWithClaims(PermECLResultRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: calcRunID.String()},
		{Key: "instrumenId", Value: instrID.String()},
	}

	h.GetSingleResult(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

// ─── GetPortfolioSummary ──────────────────────────────────────────────────────

func TestHandler_GetPortfolioSummary_OK(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID, portfolioID := uuid.New(), uuid.New()
	cols := []string{"stage_label", "cnt", "ead_total", "ecl_total"}
	mock.ExpectQuery(`SELECT`).
		WithArgs(calcRunID, portfolioID).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("STAGE_1", 3, "300000000.0000", "1200000.0000"))

	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "calcRunId", Value: calcRunID.String()},
		{Key: "portofolioId", Value: portfolioID.String()},
	}

	h.GetPortfolioSummary(c)

	if w.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

// ─── GetRollForward ───────────────────────────────────────────────────────────

func TestHandler_GetRollForward_OK(t *testing.T) {
	t.Parallel()
	h, mock := buildHandlerWithDB(t)

	calcRunID := uuid.New()
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("3000000.0000"))

	c, w := buildGinWithClaims(PermECLPortfolioAggregateRead)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
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
	if data["closingEclIdr"] != "3000000.0000" {
		t.Errorf("closingEclIdr: want '3000000.0000', got %v", data["closingEclIdr"])
	}
}

// ─── RecomputeAdHoc (service) ─────────────────────────────────────────────────

func TestOrchestrator_RecomputeAdHoc_FVTPL_NoCompare(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	instrID := uuid.New()
	orch.instrReader = &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// RecomputeAdHoc always audits → expects BeginTx + audit write + Commit.
	mock.ExpectBegin()
	// Audit write (INSERT INTO aud.audit_log ...) — allow any query.
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := RecomputeAdHocRequest{
		InstrumenID:      instrID,
		EvaluationDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:        "JUNI-2026",
		ComparePersisted: false,
		ActorID:          uuid.New(),
	}

	result, err := orch.RecomputeAdHoc(context.Background(), req)
	if err != nil {
		t.Fatalf("RecomputeAdHoc FVTPL: unexpected error: %v", err)
	}
	if result.Recomputed.RoutingPath != RoutingSkipFVTPL {
		t.Errorf("routing: want SKIP_FVTPL, got %s", result.Recomputed.RoutingPath)
	}
	if result.Recomputed.ECLWeightedIDR == nil || !result.Recomputed.ECLWeightedIDR.IsZero() {
		t.Error("FVTPL: ECLWeightedIDR must be 0")
	}
}

func TestOrchestrator_RecomputeAdHoc_WithCompare_FoundResult(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	instrID := uuid.New()
	orch.instrReader = &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// ComparePersisted=true → loadLatestStoredResult query.
	// loadLatestStoredResult SELECT: ecl_weighted_idr, pd_used_good, pd_used_normal, pd_used_bad,
	//                                calc_run_id, sealed_at, evaluation_date
	calcRunID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	storedCols := []string{"ecl_weighted_idr", "pd_used_good", "pd_used_normal", "pd_used_bad",
		"calc_run_id", "sealed_at", "evaluation_date"}
	mock.ExpectQuery(`SELECT ecl_weighted_idr`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows(storedCols).AddRow(
			"9000000.0000", "0.02000000", "0.02000000", "0.03000000",
			calcRunID, nil, evalDate,
		))

	// Audit BeginTx + write + commit.
	mock.ExpectBegin()
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := RecomputeAdHocRequest{
		InstrumenID:      instrID,
		EvaluationDate:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:        "JUNI-2026",
		ComparePersisted: true,
		ActorID:          uuid.New(),
	}

	result, err := orch.RecomputeAdHoc(context.Background(), req)
	if err != nil {
		t.Fatalf("RecomputeAdHoc compare: %v", err)
	}
	// FVTPL: recomputed ECL = 0, stored ECL = 9000000.
	// Delta = 0 - 9000000 = -9000000.
	if result.Stored == nil {
		t.Error("Stored: expected non-nil for compare mode")
	}
	if result.Delta == nil {
		t.Error("Delta: expected non-nil when compare mode + stored row found")
	}
}

// ─── NewHandler nil panic ──────────────────────────────────────────────────────

func TestNewHandler_PanicsOnNilOrchestrator(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil orchestrator")
		}
	}()
	_ = NewHandler(nil)
}

// ─── ComputeBulk handler bad date ────────────────────────────────────────────

func TestHandler_ComputeBulk_BadDate(t *testing.T) {
	t.Parallel()
	h, _ := buildHandlerForTest(&mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}, nil)
	c, w := buildGinWithClaims(PermECLBulkCompute)

	c.Request = buildRequest(http.MethodPost, `{
		"calcRunId": "00000000-0000-0000-0000-000000000001",
		"evaluationDate": "01-06-2026",
		"periodeId": "JUNI-2026"
	}`)
	h.ComputeBulk(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", w.Code)
	}
}

// ─── respondDomainError with domain error ────────────────────────────────────

func TestRespondDomainError_UnknownError_500(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)

	// Unknown non-domain, non-coreError.
	respondDomainError(c, errFoo("unknown error"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", w.Code)
	}
}

// errFoo is a plain error (not coreError or domainerrors.DomainError).
type plainErr struct{ msg string }

func (e *plainErr) Error() string { return e.msg }

func errFoo(msg string) error { return &plainErr{msg: msg} }
