package gldelivery_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newHandlerTestRouter returns a Gin router with all gldelivery routes registered and
// optional auth claims injected as middleware.
func newHandlerTestRouter(t *testing.T, perms ...string) *gin.Engine {
	t.Helper()
	_, delivery, dlqSvc, _, recon, _ := newTestDelivery(t)
	h := NewHandler(delivery, dlqSvc, recon)

	router := gin.New()

	if len(perms) > 0 {
		claims := &auth.Claims{
			Sub:         uuid.New().String(),
			Roles:       []string{"ROLE-IT-ADMIN"},
			Permissions: perms,
			TenantID:    "TUGURE",
		}
		router.Use(func(c *gin.Context) {
			ctx := auth.ContextWithClaims(c.Request.Context(), claims)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
	}

	v1 := router.Group("/api/v1")
	v1.GET("/jurnal/header/:id/gl-delivery-status", h.GetDeliveryStatus)
	v1.POST("/jurnal/header/:id/retry-gl-delivery", h.RetryGLDelivery)
	v1.GET("/jurnal/gl-delivery-dlq", h.ListDLQ)
	v1.GET("/jurnal/gl-delivery-dlq/:id", h.GetDLQEntry)
	v1.POST("/jurnal/gl-delivery-dlq/:id/replay", h.ReplayDLQEntry)
	v1.POST("/jurnal/gl-delivery-dlq/:id/discard", h.DiscardDLQEntry)
	v1.POST("/jurnal/reconciliation/run", h.RunReconciliation)
	v1.GET("/jurnal/reconciliation/history", h.ListReconciliationHistory)
	v1.GET("/jurnal/reconciliation/:date", h.GetReconciliationReport)

	return router
}

// ─── NewHandler panics ────────────────────────────────────────────────────────

func TestNewHandler_NilDelivery_Panics(t *testing.T) {
	_, _, dlqSvc, _, recon, _ := newTestDelivery(t) //nolint:dogsled
	assert.Panics(t, func() { NewHandler(nil, dlqSvc, recon) })
}

func TestNewHandler_NilDLQService_Panics(t *testing.T) {
	_, delivery, _, _, recon, _ := newTestDelivery(t) //nolint:dogsled
	assert.Panics(t, func() { NewHandler(delivery, nil, recon) })
}

func TestNewHandler_NilRecon_Panics(t *testing.T) {
	_, delivery, dlqSvc, _, _, _ := newTestDelivery(t) //nolint:dogsled
	assert.Panics(t, func() { NewHandler(delivery, dlqSvc, nil) })
}

// ─── 1. GET /jurnal/header/:id/gl-delivery-status ────────────────────────────

func TestGetDeliveryStatus_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetDeliveryStatus_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRetry) // not .read
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetDeliveryStatus_InvalidHeaderID_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRead)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/not-a-uuid/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetDeliveryStatus_ErrorEnvelopeShape(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRead)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/bad-id/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, hasError := body["error"]
	assert.True(t, hasError, "response must have 'error' envelope key")
}

// ─── 2. POST /jurnal/header/:id/retry-gl-delivery ────────────────────────────

func TestRetryGLDelivery_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	body := `{"reason":"valid reason over 30 chars"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/retry-gl-delivery",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRetryGLDelivery_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRead) // not .retry
	body := `{"reason":"valid reason over 30 chars"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/retry-gl-delivery",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRetryGLDelivery_InvalidHeaderID_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRetry)
	body := `{"reason":"valid reason over 30 chars"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/not-uuid/retry-gl-delivery",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRetryGLDelivery_InvalidBody_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRetry)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/retry-gl-delivery",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── 3. GET /jurnal/gl-delivery-dlq ──────────────────────────────────────────

func TestListDLQ_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/gl-delivery-dlq", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListDLQ_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRetry) // not .read
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/gl-delivery-dlq", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── 4. GET /jurnal/gl-delivery-dlq/:id ──────────────────────────────────────

func TestGetDLQEntry_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetDLQEntry_InvalidID_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRead)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/gl-delivery-dlq/bad-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── 5. POST /jurnal/gl-delivery-dlq/:id/replay ──────────────────────────────

func TestReplayDLQEntry_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	body := `{"reason":"valid reason that is more than thirty characters"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/replay",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReplayDLQEntry_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryRead) // not .replay
	body := `{"reason":"valid reason"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/replay",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReplayDLQEntry_InvalidID_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryReplay)
	body := `{"reason":"valid reason"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/not-uuid/replay",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplayDLQEntry_InvalidBody_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryReplay)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/replay",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── 6. POST /jurnal/gl-delivery-dlq/:id/discard ─────────────────────────────

func TestDiscardDLQEntry_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	body := `{"reason":"valid discard reason over 30 chars"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/discard",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDiscardDLQEntry_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryReplay) // not .discard
	body := `{"reason":"valid discard reason"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/discard",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDiscardDLQEntry_InvalidID_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryDiscard)
	body := `{"reason":"valid reason"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/bad-uuid/discard",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDiscardDLQEntry_InvalidBody_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermGlDeliveryDiscard)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/discard",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── 7. POST /jurnal/reconciliation/run ──────────────────────────────────────

func TestRunReconciliation_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	body := `{"date":"2026-06-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRunReconciliation_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRead) // not .run
	body := `{"date":"2026-06-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRunReconciliation_InvalidDate_422(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRun)
	body := `{"date":"not-a-date"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestRunReconciliation_MissingBody_400(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRun)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// Nil body or EOF on ShouldBindJSON → 400.
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── 8. GET /jurnal/reconciliation/:date ─────────────────────────────────────

func TestGetReconciliationReport_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/2026-06-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetReconciliationReport_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRun) // not .read
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/2026-06-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetReconciliationReport_InvalidDate_422(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRead)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/not-a-date", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// ─── 9. GET /jurnal/reconciliation/history ───────────────────────────────────

func TestListReconciliationHistory_NoAuth_403(t *testing.T) {
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListReconciliationHistory_WrongPerm_403(t *testing.T) {
	router := newHandlerTestRouter(t, PermReconciliationRun) // not .read
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── domain error code → HTTP status mapping ─────────────────────────────────

func TestDomainErrorCodes_HTTPStatus(t *testing.T) {
	cases := []struct {
		code   domainerrors.Code
		expect int
	}{
		{domainerrors.CodeGLDeliveryJurnalNotFound, http.StatusNotFound},
		{domainerrors.CodeGLDeliveryPermissionDenied, http.StatusForbidden},
		{domainerrors.CodeGLReconciliationInProgress, http.StatusConflict},
		{domainerrors.CodeGLStatusTerminalImmutable, 423},
		{domainerrors.CodeGLDeliveryInvalidTransition, http.StatusUnprocessableEntity},
		{domainerrors.CodeGLDeliveryReasonTooShort, http.StatusBadRequest},
		{domainerrors.CodeGLReconciliationDateInvalid, http.StatusUnprocessableEntity},
		{domainerrors.CodeGLReconciliationReportNotFound, http.StatusNotFound},
		{domainerrors.CodeGLDeliveryMaxAttemptsExceeded, http.StatusUnprocessableEntity},
		{domainerrors.CodeGLDLQReplayInvalidState, http.StatusUnprocessableEntity},
		{domainerrors.CodeValidationFailed, http.StatusBadRequest},
		{domainerrors.CodeUnauthorized, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			err := domainerrors.New(tc.code, "test")
			de, ok := domainerrors.IsDomainError(err)
			require.True(t, ok)
			assert.Equal(t, tc.expect, de.HTTPStatus())
		})
	}
}

// ─── error envelope key/shape ─────────────────────────────────────────────────

func TestForbiddenResponse_HasErrorAndTraceID(t *testing.T) {
	// No claims → 403 response must have error envelope with traceId.
	router := newHandlerTestRouter(t /* no perms */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, hasError := body["error"]
	assert.True(t, hasError, "response must have 'error' key in envelope")

	errMap, _ := body["error"].(map[string]any)
	require.NotNil(t, errMap, "'error' must be an object")
	// traceId key present even when empty string.
	_, hasTrace := errMap["traceId"]
	assert.True(t, hasTrace, "error envelope must include traceId field")
}
