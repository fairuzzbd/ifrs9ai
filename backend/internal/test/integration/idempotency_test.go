//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// TestIdempotency_Replay verifies that sending the same Idempotency-Key
// with the same payload returns the original response (200 IDEMPOTENCY_REPLAY)
// and does not create a duplicate side-effect.
//
// Covers: regression §8 — Idempotency replay.
func TestIdempotency_Replay(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Idempotency(infra.DB))

	callCount := 0
	router.POST("/api/v1/test/resource", func(c *gin.Context) {
		callCount++
		c.JSON(http.StatusCreated, map[string]any{
			"data": map[string]any{"id": "item-001", "created": true},
			"meta": map[string]any{"traceId": "test-trace-001"},
		})
	})

	key := uuid.New().String()
	body := `{"name":"deposito-test","amount":1000000}`

	// First request — should reach handler.
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/v1/test/resource", strings.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}

	// Second request — same key, same payload. Must NOT call handler again.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/test/resource", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w2, req2)

	// Still 201 (original status replayed).
	if w2.Code != http.StatusCreated {
		t.Errorf("replay: expected 201 (original status), got %d body=%s", w2.Code, w2.Body.String())
	}
	if callCount != 1 {
		t.Errorf("handler must NOT be called on replay, but call count is %d", callCount)
	}

	// Responses must be identical.
	var r1, r2 map[string]any
	if err := json.Unmarshal(w1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("parse r1: %v", err)
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("parse r2: %v", err)
	}
	d1, _ := r1["data"].(map[string]any)
	d2, _ := r2["data"].(map[string]any)
	if d1["id"] != d2["id"] {
		t.Errorf("replay response data mismatch: %v vs %v", d1, d2)
	}
	t.Logf("idempotency replay: OK (handler called %d time)", callCount)
}

// TestIdempotency_Mismatch verifies that sending the same Idempotency-Key
// with a DIFFERENT payload returns 422 IDEMPOTENCY_MISMATCH.
//
// Covers: api-conventions.md §Idempotency mismatch path.
func TestIdempotency_Mismatch(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Idempotency(infra.DB))

	router.POST("/api/v1/test/mismatch", func(c *gin.Context) {
		c.JSON(http.StatusCreated, map[string]any{"data": map[string]any{"ok": true}})
	})

	key := uuid.New().String()
	body1 := `{"amount":100}`
	body2 := `{"amount":999}` // different

	// First request.
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/v1/test/mismatch", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", w1.Code)
	}

	// Second request with same key but different payload.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/test/mismatch", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("mismatch: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}

	var errResp map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("parse mismatch response: %v", err)
	}
	errObj, _ := errResp["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("expected error code IDEMPOTENCY_MISMATCH, got %q", code)
	}
	t.Logf("idempotency mismatch: correctly returned 422 IDEMPOTENCY_MISMATCH")
}

// TestIdempotency_MissingKey verifies that mutating endpoints without
// Idempotency-Key header return 400 VALIDATION_FAILED.
func TestIdempotency_MissingKey(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Idempotency(infra.DB))

	router.POST("/api/v1/test/nokey", func(c *gin.Context) {
		c.JSON(http.StatusCreated, map[string]any{"data": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/test/nokey",
		bytes.NewBufferString(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	// No Idempotency-Key
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing key: expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	var errResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("parse 400: %v", err)
	}
	errObj, _ := errResp["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
}

// TestIdempotency_GetSkipped verifies GET requests bypass idempotency check.
func TestIdempotency_GetSkipped(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Idempotency(infra.DB))

	router.GET("/api/v1/test/read", func(c *gin.Context) {
		c.JSON(http.StatusOK, map[string]any{"data": "read-ok"})
	})

	// No Idempotency-Key — should pass through for GET.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/test/read", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET should not require Idempotency-Key, got %d", w.Code)
	}
}

// TestIdempotency_CalcRunCreate_HandlerLevel verifies handler-level idempotency
// semantics for the POST /api/v1/ecl/calc-runs endpoint (F5, DEC-021).
//
// Sequence:
//  1. POST with Idempotency-Key=X + body A → 201 Created (first call through).
//  2. POST with Idempotency-Key=X + body A → 200 (IDEMPOTENCY_REPLAY, no new row).
//  3. POST with Idempotency-Key=X + body B (different payload) → 422 IDEMPOTENCY_MISMATCH.
//
// Uses the generic Idempotency middleware wired to the real DB, mirroring the
// production route wiring in calcrun/routes.go RegisterRoutes.
// The handler is a stub that mimics the 201 response shape of CreateCalcRun —
// full handler wiring would require testcontainers; this test validates the
// middleware contract (DEC-021) at the HTTP boundary.
func TestIdempotency_CalcRunCreate_HandlerLevel(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.Idempotency(infra.DB))

	handlerCallCount := 0
	// Stub handler that mimics calcrun.Handler.CreateCalcRun 201 response shape.
	router.POST("/api/v1/ecl/calc-runs", func(c *gin.Context) {
		handlerCallCount++
		c.JSON(http.StatusCreated, map[string]any{
			"data": map[string]any{
				"id":       "00000000-0000-0000-0000-000000000099",
				"status":   "DRAFT",
				"periodeId": "PBUKU-2026-06",
			},
			"meta": map[string]any{"traceId": "trace-calcrun-001"},
		})
	})

	key := uuid.New().String()
	bodyA := `{"periodeId":"PBUKU-2026-06","evaluationDate":"2026-06-30","scope":"ALL_ACTIVE","comment":"ECL run Juni 2026"}`
	bodyB := `{"periodeId":"PBUKU-2026-07","evaluationDate":"2026-07-31","scope":"ALL_ACTIVE","comment":"ECL run Juli 2026"}` // different

	// ── Call 1: fresh key + body A → 201 Created ─────────────────────────────
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/v1/ecl/calc-runs", strings.NewReader(bodyA))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("CalcRunCreate F5: call 1 expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	if handlerCallCount != 1 {
		t.Errorf("CalcRunCreate F5: expected handler called once, got %d", handlerCallCount)
	}

	// ── Call 2: same key + same body A → IDEMPOTENCY_REPLAY ─────────────────
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/ecl/calc-runs", strings.NewReader(bodyA))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w2, req2)

	// Middleware must return the original 201 response without calling handler again.
	if w2.Code != http.StatusCreated {
		t.Errorf("CalcRunCreate F5: replay expected 201 (original status), got %d body=%s",
			w2.Code, w2.Body.String())
	}
	if handlerCallCount != 1 {
		t.Errorf("CalcRunCreate F5: handler must NOT be called on replay, call count=%d", handlerCallCount)
	}
	// Responses must be byte-identical (DEC-021 contract).
	if w1.Body.String() != w2.Body.String() {
		t.Errorf("CalcRunCreate F5: replay body differs from original:\n  original=%s\n  replay=%s",
			w1.Body.String(), w2.Body.String())
	}

	// ── Call 3: same key + body B (different) → 422 IDEMPOTENCY_MISMATCH ───
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/api/v1/ecl/calc-runs", strings.NewReader(bodyB))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusUnprocessableEntity {
		t.Errorf("CalcRunCreate F5: mismatch expected 422, got %d body=%s", w3.Code, w3.Body.String())
	}
	var mismatchResp map[string]any
	if err := json.Unmarshal(w3.Body.Bytes(), &mismatchResp); err != nil {
		t.Fatalf("CalcRunCreate F5: parse mismatch response: %v", err)
	}
	errObj, _ := mismatchResp["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("CalcRunCreate F5: expected IDEMPOTENCY_MISMATCH code, got %q", code)
	}
	t.Logf("CalcRunCreate F5: handler calls=%d (want 1), mismatch code=%s", handlerCallCount, errObj["code"])
}
