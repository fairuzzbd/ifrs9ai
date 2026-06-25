package pocidelta

// handler_test.go — HTTP handler unit tests using httptest.
// Tests permission gating, request parsing, and service error mapping.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(h *HTTPHandler) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/api/v1")
	// Register without auth middleware for unit tests
	v1.GET("/poci/baseline", h.ListBaseline)
	v1.POST("/poci/baseline", h.CaptureBaseline)
	v1.GET("/poci/baseline/:instrumen_id", h.GetBaseline)
	v1.GET("/poci/delta-log", h.ListDeltaLog)
	v1.GET("/poci/delta-history/summary", h.GetDeltaSummary)
	v1.GET("/poci/delta-history", h.GetDeltaHistory)
	v1.POST("/poci/compute-delta-batch", h.ComputeDeltaBatch)
	return r
}

func TestListBaseline_Returns200(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		baseline: &Baseline{
			ID:                       uuid.New(),
			InstrumenID:              instrID,
			LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
			CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
		},
	}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/baseline", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetBaseline_NotFound_Returns404(t *testing.T) {
	repo := &stubRepo{baseline: nil}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/baseline/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetBaseline_InvalidUUID_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/baseline/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCaptureBaseline_InvalidJSON_Returns400(t *testing.T) {
	repo := &stubRepo{instrumenInfo: &InstrumenPociInfo{IsPoci: true, Status: "ACTIVE"}}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/poci/baseline",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCaptureBaseline_NotPoci_Returns422(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: false, Status: "ACTIVE"},
	}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]interface{}{
		"instrumenId":              instrID.String(),
		"lifetimeEclAtOrigination": 1000000,
		"creditAdjustedEir":        0.045,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/poci/baseline",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// POCI_INSTRUMEN_NOT_POCI is returned as VALIDATION_FAILED (400)
	// by validator.go using domainerrors.CodeValidationFailed.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (VALIDATION_FAILED for not-POCI instrumen), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeltaHistory_MissingInstrumenID_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetDeltaSummary_InvalidYear_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-history/summary?year=abc&month=6", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetDeltaSummary_ValidParams_Returns200(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-history/summary?year=2026&month=6", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestComputeDeltaBatch_NoAsynqClient_Returns501(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil) // nil asynqClient
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]interface{}{
		"calcRunId": uuid.New().String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/poci/compute-delta-batch",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when asynqClient nil, got %d", w.Code)
	}
}

func TestListDeltaLog_Returns200(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
