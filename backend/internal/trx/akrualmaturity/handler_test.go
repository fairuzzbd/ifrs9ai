package akrualmaturity

// handler_test.go — HTTP handler tests using httptest.
// Tests thin-handler behaviour: routing, query param parsing, error mapping.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// buildTestRouter returns a Gin engine with akrualmaturity routes for testing.
// Injects auth.Claims into both Gin context (c.Set "claims") and request context
// so auth.RequirePermission and auth.ClaimsFromContext both pass.
func buildTestRouter(svc *Service) *gin.Engine {
	r := gin.New()
	// Inject claims before route handlers.
	// RequirePermission reads c.Get("claims") — must use c.Set, not only context.WithValue.
	r.Use(func(c *gin.Context) {
		userSub := c.GetHeader("X-Test-Sub")
		if userSub == "" {
			userSub = "00000000-0000-0000-0000-000000000099"
		}
		claims := &auth.Claims{
			Sub:      userSub,
			TenantID: "TUGURE",
			Roles:    []string{"ROLE-MAKER-TR", "ROLE-APPR-TR", "ROLE-IT-ADMIN"},
			Permissions: []string{
				"akrual.read", "akrual.override_stale",
				"sys.cron.trigger", "maturity.read",
			},
		}
		// Set on Gin context (for RequirePermission)
		c.Set("claims", claims)
		// Set on request context (for auth.ClaimsFromContext called in service)
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewHTTPHandler(svc, nil) // nil asynqClient: cron-trigger returns 501 in test
	v1 := r.Group("/api/v1")
	RegisterRoutes(v1, h) // no redis in test
	return r
}

func buildTestSvc(repo Repository) *Service {
	return NewService(repo, NewJurnalPosterStub(nil), NewInstrumenStatusUpdaterStub(), nil, nil)
}

// ─── GET /api/v1/transaksi/akrual ─────────────────────────────────────────────

func TestHandlerListAkrual_OK(t *testing.T) {
	repo := &stubRepo{
		isHoliday:      false,
		periode:        openPeriode(),
		listAkrualRows: []*PendapatanAkrual{},
	}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual", nil)
	req.Header.Set("Authorization", "Bearer test")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body, "data")
	assert.Contains(t, body, "pagination")
}

// ─── GET /api/v1/transaksi/akrual/dashboard (static route) ───────────────────

func TestHandlerGetDashboard_StaticRouteBeforeID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/dashboard", nil)
	router.ServeHTTP(w, req)

	// Should hit dashboard handler (not /:id with "dashboard" as UUID)
	// Dashboard returns 200 with an AkrualDashboard
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerGetDashboard_InvalidInstrumenID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/dashboard?instrumen_id=not-a-uuid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── POST /api/v1/transaksi/akrual/cron-trigger ───────────────────────────────

// B2 fix: handler returns 501 when asynqClient is nil (no Redis in test env).
// In production with Redis configured, real Asynq task is enqueued and 202 returned.
func TestHandlerTriggerAkrualCron_Returns501WhenNoAsynqClient(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo)) // nil asynqClient

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/akrual/cron-trigger?tanggal=2026-06-20", nil)
	req.Header.Set("Authorization", "Bearer test")
	router.ServeHTTP(w, req)

	// B2 fix: 501 Not Implemented when no Asynq client (dev/test mode without Redis).
	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errBlock, ok := body["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ASYNQ_NOT_CONFIGURED", errBlock["code"])
}

// ─── GET /api/v1/transaksi/akrual/:id ────────────────────────────────────────

func TestHandlerGetByID_NotFound(t *testing.T) {
	repo := &stubRepo{akrualByIDErr: errors.New("not found")}
	router := buildTestRouter(buildTestSvc(repo))

	id := uuid.New()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/"+id.String(), nil)
	router.ServeHTTP(w, req)

	// Service returns error → handler returns 4xx/5xx
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestHandlerGetByID_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/not-a-uuid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerGetByID_OK(t *testing.T) {
	id := uuid.New()
	eir := decimal.NewFromFloat(0.075)
	stage := 1
	repo := &stubRepo{
		akrualByID: &PendapatanAkrual{
			ID:           id,
			InstrumenID:  uuid.New(),
			TanggalAkrual: time.Now(),
			Jenis:        JenisBunga,
			Stage:        &stage,
			EIRPersen:    &eir,
			BungaKotor:   decimal.NewFromInt(100),
			BungaBersih:  decimal.NewFromInt(80),
			Status:       AkrualAutoPosted,
		},
	}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/"+id.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── POST /api/v1/transaksi/akrual/:id/override-stale ────────────────────────

func TestHandlerOverrideStale_MissingIdempotencyKey(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	id := uuid.New()
	body := map[string]interface{}{
		"reason":          "Override karena ECL sealed run telah diperbarui oleh tim Risk.",
		"signatureMethod": "JWT_STEP_UP",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/akrual/"+id.String()+"/override-stale",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// No Idempotency-Key
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerOverrideStale_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/akrual/bad-id/override-stale", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerOverrideStale_InvalidBody(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	id := uuid.New()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/akrual/"+id.String()+"/override-stale",
		bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── GET /api/v1/transaksi/jatuh-tempo ───────────────────────────────────────

func TestHandlerListJatuhTempo_OK(t *testing.T) {
	repo := &stubRepo{listJatuhTempoRows: []*JatuhTempo{}}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/jatuh-tempo", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── POST /api/v1/transaksi/jatuh-tempo/cron-trigger ─────────────────────────

// B2 fix: returns 501 when asynqClient is nil (no Redis in test env).
func TestHandlerTriggerMaturityCron_Returns501WhenNoAsynqClient(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo)) // nil asynqClient

	body := map[string]string{"tanggal": "2026-06-20"}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/jatuh-tempo/cron-trigger",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errBlock, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ASYNQ_NOT_CONFIGURED", errBlock["code"])
}

func TestHandlerTriggerMaturityCron_NoBody_Returns501(t *testing.T) {
	// No tanggal in body + no asynqClient → 501 (asynq check fires first)
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/jatuh-tempo/cron-trigger", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

// ─── Route isolation test: "dashboard" not treated as /:id ───────────────────

func TestRouteIsolation_DashboardNotTreatedAsID(t *testing.T) {
	// Ensure GET /transaksi/akrual/dashboard hits GetDashboard not GetByID
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual/dashboard", nil)
	router.ServeHTTP(w, req)

	// GetDashboard returns 200 with dashboard data
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	// Not a "not valid UUID" error
	_, hasError := body["error"]
	assert.False(t, hasError, "dashboard route should not return UUID parse error")
}

// ─── stub satisfying *Service interfaces for handler test ─────────────────────

// stubRepo already implements Repository via service_test.go.
// ListAkrual/ListJatuhTempo need specific signatures. In the handler tests we
// wire the same concrete Service so we get coverage of handler→service→stub.
// The below extend stubRepo to satisfy any remaining methods called in handler paths.

func (r *stubRepo) GetAkrualByIDHanlder(ctx context.Context, id uuid.UUID) (*PendapatanAkrual, error) {
	return r.akrualByID, r.akrualByIDErr
}

// auth.ContextWithClaims helper reference for tests
var _ = auth.ContextWithClaims
var _ = context.Background
var _ = sql.Tx{}
