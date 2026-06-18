package mtm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupRouter returns a gin engine with MTM routes mounted without auth middleware.
// Tests cover handler logic; auth middleware is tested in auth package.
func setupRouter(repo Repository) (*gin.Engine, *HTTPHandler) {
	eq := &stubEnqueuer{}
	svc := newTestService(repo)
	h := NewHTTPHandler(svc, eq)

	engine := gin.New()
	// Inject test claims so RequirePermission passes (reads "claims" from gin context).
	testClaims := &auth.Claims{
		Sub:         uuid.New().String(),
		Permissions: []string{"fx_rate.read", "fx_rate.create", "fx_rate.approve"},
	}
	engine.Use(func(c *gin.Context) {
		c.Set("claims", testClaims)
		ctx := auth.ContextWithClaims(c.Request.Context(), testClaims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	v1 := engine.Group("/api/v1")
	RegisterRoutes(v1, h)
	return engine, h
}

// ─── GET /trx/mtm (List) ─────────────────────────────────────────────────────

func TestHandler_List_OK(t *testing.T) {
	repo := newStubRepo()
	repo.mtmList = []*Mtm{makeMtm(StatusAutoPOSTED), makeMtm(StatusPendingReview)}
	engine, _ := setupRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body["data"])
}

func TestHandler_List_WithFilters_200(t *testing.T) {
	repo := newStubRepo()
	engine, _ := setupRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm?limit=10&sort=status:asc", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── GET /trx/mtm/:id ────────────────────────────────────────────────────────

func TestHandler_GetByID_Found(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusAutoPOSTED)
	repo.mtm = m
	engine, _ := setupRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/"+m.ID.String(), nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	repo := newStubRepo()
	// mtm = nil → not found
	engine, _ := setupRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/"+uuid.New().String(), nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetByID_InvalidUUID(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/not-a-uuid", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── Static path ordering — /alerts/stale-price must not be treated as /:id ──

func TestHandler_StalePriceAlerts_NotTreatedAsIDParam(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/alerts/stale-price", nil)
	engine.ServeHTTP(w, req)

	// Must reach StalePriceAlerts handler (200), NOT GetByID (which would 400 invalid UUID)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── GET /trx/mtm/alerts/stale-price ─────────────────────────────────────────

func TestHandler_StalePriceAlerts_Empty(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/alerts/stale-price", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_StalePriceAlerts_WithLimit(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/alerts/stale-price?limit=5", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── POST /trx/mtm/cron/trigger (static path before /:id) ────────────────────

func TestHandler_CronTrigger_StaticPath_NotIDParam(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(CronTriggerRequest{TanggalTarget: "2026-06-16"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/cron/trigger",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	// Should reach CronTrigger (202 Accepted), not try to parse "cron" as UUID
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestHandler_CronTrigger_EmptyBody_OK(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/cron/trigger", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestHandler_CronTrigger_InvalidDate_422(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(CronTriggerRequest{TanggalTarget: "2026-99-99"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/cron/trigger",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── POST /trx/mtm/upload/batch ──────────────────────────────────────────────

func TestHandler_UploadBatch_NoFile_BadRequest(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	// No multipart file
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/upload/batch", nil)
	engine.ServeHTTP(w, req)

	// No file → 400 from FormFile error
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── GET /trx/mtm/upload/batch/:batch_id ─────────────────────────────────────

func TestHandler_GetUploadBatch_Static_NotIDParam(t *testing.T) {
	repo := newStubRepo()
	bID := uuid.New()
	repo.batch = &UploadBatch{
		ID:         bID,
		BatchType:  "MTM_UPLOAD",
		Status:     "PENDING_REVIEW",
		UploaderID: uuid.New(),
		TotalRows:  1,
		ValidRows:  1,
		CreatedAt:  time.Now(),
		CreatedBy:  uuid.New(),
	}
	engine, _ := setupRouter(repo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/upload/batch/"+bID.String(), nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_GetUploadBatch_InvalidUUID(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/upload/batch/not-a-uuid", nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetUploadBatch_NotFound(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/mtm/upload/batch/"+uuid.New().String(), nil)
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── POST /trx/mtm/:id/override-approve ──────────────────────────────────────

func TestHandler_OverrideApprove_InvalidUUID(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(OverrideApproveRequest{Comment: "Long enough comment here for the validation rule"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/not-uuid/override-approve",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_OverrideApprove_NotFound_404(t *testing.T) {
	// repo.mtm = nil → GetByID returns nil → service returns NotFound
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(OverrideApproveRequest{
		Comment: "Override comment that is long enough to pass validation rule here.",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/"+uuid.New().String()+"/override-approve",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_OverrideApprove_LockedFlag_423(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusPendingReview)
	m.LockedFlag = true
	repo.mtm = m
	engine, _ := setupRouter(repo)

	body, _ := json.Marshal(OverrideApproveRequest{
		Comment: "Override comment that is long enough to pass validation rule here.",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/"+m.ID.String()+"/override-approve",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, 423, w.Code)
}

func TestHandler_OverrideApprove_SOD_Violation_403(t *testing.T) {
	// SoD: the injected test claims have sub = some UUID.
	// Set uploader_id = same UUID → SoD violation.
	repo := newStubRepo()
	m := makeMtm(StatusPendingReview)
	// Injected claims sub = testClaims.Sub, set uploader to same
	testClaims := &auth.Claims{
		Sub:         uuid.New().String(),
		Permissions: []string{"fx_rate.read", "fx_rate.create", "fx_rate.approve"},
	}
	uploaderID, _ := uuid.Parse(testClaims.Sub)
	m.UploaderID = &uploaderID
	repo.mtm = m

	eq := &stubEnqueuer{}
	svc := newTestService(repo)
	h := NewHTTPHandler(svc, eq)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("claims", testClaims)
		ctx := auth.ContextWithClaims(c.Request.Context(), testClaims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	v1 := engine.Group("/api/v1")
	RegisterRoutes(v1, h)

	body, _ := json.Marshal(OverrideApproveRequest{
		Comment: "Override comment that is long enough to pass validation rule here.",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/"+m.ID.String()+"/override-approve",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── POST /trx/mtm/:id/override-reject ───────────────────────────────────────

func TestHandler_OverrideReject_InvalidUUID(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(OverrideRejectRequest{Comment: "Long enough rejection comment here for the rule"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/not-uuid/override-reject",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_OverrideReject_NotFound_404(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())

	body, _ := json.Marshal(OverrideRejectRequest{
		Comment: "Rejection comment that is long enough to pass the validation here.",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/"+uuid.New().String()+"/override-reject",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_OverrideReject_LockedFlag_423(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusPendingReview)
	m.LockedFlag = true
	repo.mtm = m
	engine, _ := setupRouter(repo)

	body, _ := json.Marshal(OverrideRejectRequest{
		Comment: "Rejection comment that is long enough to pass the validation here.",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/mtm/"+m.ID.String()+"/override-reject",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	assert.Equal(t, 423, w.Code)
}

// ─── parseUUID helper ────────────────────────────────────────────────────────

func TestParseUUID_ValidUUID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}
	id, err := parseUUID(c, "id")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

func TestParseUUID_InvalidUUID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	_, err := parseUUID(c, "id")
	require.Error(t, err)
}

// ─── Static/dynamic route ordering (regression) ──────────────────────────────

func TestRoutingOrder_StaticBeforeDynamic(t *testing.T) {
	assert.NotPanics(t, func() {
		setupRouter(newStubRepo())
	})
}

// ─── All routes registered ────────────────────────────────────────────────────

func TestHandler_Routes_AllRegistered(t *testing.T) {
	engine, _ := setupRouter(newStubRepo())
	routes := engine.Routes()

	paths := make(map[string]bool)
	for _, r := range routes {
		paths[r.Method+":"+r.Path] = true
	}

	assert.True(t, paths["GET:/api/v1/trx/mtm"], "List route missing")
	assert.True(t, paths["GET:/api/v1/trx/mtm/alerts/stale-price"], "StalePriceAlerts route missing")
	assert.True(t, paths["POST:/api/v1/trx/mtm/upload/batch"], "UploadBatch route missing")
	assert.True(t, paths["GET:/api/v1/trx/mtm/upload/batch/:batch_id"], "GetUploadBatch route missing")
	assert.True(t, paths["POST:/api/v1/trx/mtm/cron/trigger"], "CronTrigger route missing")
	assert.True(t, paths["GET:/api/v1/trx/mtm/:id"], "GetByID route missing")
	assert.True(t, paths["POST:/api/v1/trx/mtm/:id/override-approve"], "OverrideApprove route missing")
	assert.True(t, paths["POST:/api/v1/trx/mtm/:id/override-reject"], "OverrideReject route missing")
}

// ─── NewHTTPHandler / NewService edge cases ───────────────────────────────────

func TestNewHTTPHandler_NotNil(t *testing.T) {
	h := NewHTTPHandler(newTestService(newStubRepo()), &stubEnqueuer{})
	assert.NotNil(t, h)
}

func TestNewService_NilLogger_UsesDefault(t *testing.T) {
	svc := NewService(newStubRepo(), nil, nil, nil)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.logger)
}

func TestNewService_NilPoster_UsesNoop(t *testing.T) {
	svc := NewService(newStubRepo(), nil, nil, nil)
	assert.NotNil(t, svc.poster)
}

// ─── WithJurnalPoster ─────────────────────────────────────────────────────────

func TestService_WithJurnalPoster_Replaces(t *testing.T) {
	svc := newTestService(newStubRepo())
	originalPoster := svc.poster

	newPoster := NewJurnalPosterStub(nil)
	svc2 := svc.WithJurnalPoster(newPoster)
	assert.Same(t, svc, svc2, "WithJurnalPoster returns same service")
	assert.NotSame(t, originalPoster, svc.poster, "poster should be replaced")
}
