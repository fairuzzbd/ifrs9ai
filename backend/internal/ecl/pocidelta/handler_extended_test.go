package pocidelta

// handler_extended_test.go — Extended handler coverage for uncovered branches.
// Covers: GetDeltaHistory success + svcErr, GetDeltaSummary month/portofolio_id errors,
// ListDeltaLog threshold branch, ComputeDeltaBatch body parse, parseIntDefault.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
)

// newAsynqTask is a helper to build an *asynq.Task for worker tests.
func newAsynqTask(payload []byte) *asynq.Task {
	return asynq.NewTask(TaskComputeDeltaBatch, payload)
}

// ─── parseIntDefault coverage ─────────────────────────────────────────────────

func TestParseIntDefault_EmptyString(t *testing.T) {
	got := parseIntDefault("", 42)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestParseIntDefault_InvalidString(t *testing.T) {
	got := parseIntDefault("abc", 99)
	if got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
}

func TestParseIntDefault_ValidString(t *testing.T) {
	got := parseIntDefault("2026", 0)
	if got != 2026 {
		t.Fatalf("expected 2026, got %d", got)
	}
}

// ─── GetDeltaHistory — success path ──────────────────────────────────────────

func TestGetDeltaHistory_Success_Returns200(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		baseline: &Baseline{InstrumenID: instrID, LifetimeECLAtOrigination: decimal.NewFromFloat(1000000)},
		deltaLog: &DeltaLog{
			InstrumenID: instrID,
			DeltaECL:    decimal.NewFromFloat(50000),
			Direction:   DirectionIncrease,
			Status:      StatusPosted,
			CreatedAt:   time.Now(),
		},
	}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history?instrumen_id="+instrID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeltaHistory_BaselineMissing_Returns404(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{baseline: nil} // no baseline → POCI_BASELINE_MISSING
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history?instrumen_id="+instrID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GetDeltaSummary — error paths ───────────────────────────────────────────

func TestGetDeltaSummary_InvalidMonth_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history/summary?year=2026&month=13", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetDeltaSummary_ZeroMonth_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history/summary?year=2026&month=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetDeltaSummary_InvalidPortofolioID_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history/summary?year=2026&month=6&portofolio_id=not-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeltaSummary_WithValidPortofolioID_Returns200(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history/summary?year=2026&month=6&portofolio_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeltaSummary_YearTooLow_Returns400(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history/summary?year=1999&month=6", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── CaptureBaseline — happy path via handler ─────────────────────────────────

func TestCaptureBaseline_HappyPath_Returns201(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: true, Status: "ACTIVE"},
		baseline:      nil, // no existing baseline
	}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]interface{}{
		"instrumenId":              instrID.String(),
		"lifetimeEclAtOrigination": 1250000000,
		"creditAdjustedEir":        0.045,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/poci/baseline", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ListDeltaLog — returns items with threshold branches ────────────────────

func TestListDeltaLog_ZeroThreshold_UsesDefault(t *testing.T) {
	// repo.GetLargeDeltaThreshold returns 0 → handler uses 500M default
	repo := &stubRepo{threshold: decimal.Zero}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeltaLog_PositiveThreshold_Returns200(t *testing.T) {
	repo := &stubRepo{threshold: decimal.NewFromFloat(1000000000)}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/poci/delta-log", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── worker — valid actor UUID path ──────────────────────────────────────────

func TestHandleComputeDeltaBatch_ValidActor_CalcRunNotSealed(t *testing.T) {
	repo := &stubRepo{calcRunStatus: "RUNNING"}
	svc := makeService(repo)
	wk := NewWorker(svc, nil, nil)

	payload, _ := json.Marshal(ComputeDeltaPayload{
		CalcRunID: uuid.New().String(),
		ActorID:   uuid.New().String(), // valid UUID
		TenantID:  "TUGURE",
		JobID:     uuid.New().String(),
	})
	task := newAsynqTask(payload)
	err := wk.HandleComputeDeltaBatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for non-sealed calc run")
	}
}

func TestHandleComputeDeltaBatch_InvalidActorFallsback_CalcRunNotSealed(t *testing.T) {
	// invalid actor UUID → fallback to systemActorID, still fails on calc run status
	repo := &stubRepo{calcRunStatus: "DRAFT"}
	svc := makeService(repo)
	wk := NewWorker(svc, nil, nil)

	payload, _ := json.Marshal(ComputeDeltaPayload{
		CalcRunID: uuid.New().String(),
		ActorID:   "not-a-uuid",
		TenantID:  "TUGURE",
		JobID:     uuid.New().String(),
	})
	task := newAsynqTask(payload)
	err := wk.HandleComputeDeltaBatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error (non-sealed calc run)")
	}
}

func TestHandleComputeDeltaBatch_EmptyPociList_Returns200(t *testing.T) {
	// SEALED calc run with empty POCI list → no per-instrument errors → success
	repo := &stubRepo{
		calcRunStatus: "SEALED",
		pociList:      []InstrumenPociInfo{},
	}
	svc := makeService(repo)
	wk := NewWorker(svc, nil, nil)

	payload, _ := json.Marshal(ComputeDeltaPayload{
		CalcRunID: uuid.New().String(),
		ActorID:   uuid.New().String(),
		TenantID:  "TUGURE",
		JobID:     uuid.New().String(),
	})
	task := newAsynqTask(payload)
	err := wk.HandleComputeDeltaBatch(context.Background(), task)
	if err != nil {
		t.Fatalf("expected nil error for empty POCI list, got: %v", err)
	}
}
