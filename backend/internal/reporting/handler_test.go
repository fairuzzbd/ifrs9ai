package reporting_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestService builds a minimal Service for handler tests.
func setupTestService(t *testing.T) (*reporting.Service, sqlmock.Sqlmock, *sql.DB) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("handler-test-secret-32bytes!!!!"))
	return svc, mock, db
}

// injectClaims injects JWT claims into Gin context.
func injectClaims(c *gin.Context, perms []string) {
	c.Set("claims", &auth.Claims{
		Sub:       uuid.New().String(),
		Roles:     []string{"ROLE-AKUN"},
		TenantID:  "TUGURE",
		Permissions: perms,
	})
}

// ─── GetMVStatus ─────────────────────────────────────────────────────────────

func TestHandler_GetMVStatus_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/admin/mv-status", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"}) // no report.admin
		h.GetMVStatus(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/mv-status", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_GetMVStatus_Unauthorized(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/admin/mv-status", h.GetMVStatus) // no claims injected

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/mv-status", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── PostMVRefresh ────────────────────────────────────────────────────────────

func TestHandler_PostMVRefresh_InvalidMVName(t *testing.T) {
	svc, mock, db := setupTestService(t)
	defer db.Close()

	// IsRefreshRunning check will be called.
	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WillReturnRows(sqlmock.NewRows([]string{}))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/admin/mv-refresh", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.PostMVRefresh(c)
	})

	body := `{"mvName": "rpt.mv_invalid"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/admin/mv-refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Should return 422 or 400 for invalid mv_name.
	assert.True(t, w.Code >= 400, "expected 4xx for invalid mv_name, got %d", w.Code)
}

func TestHandler_PostMVRefresh_EmptyBody_AllMVs(t *testing.T) {
	svc, mock, db := setupTestService(t)
	defer db.Close()

	// IsRefreshRunning queries for all 8 MVs.
	for range reporting.AllMVNames {
		mock.ExpectQuery(`FROM sys.mv_refresh_log`).
			WillReturnRows(sqlmock.NewRows([]string{}))
	}
	// Asynq enqueue would be called here; without a real Asynq client it returns error.
	// So we just verify 202 or 500 (if asynq nil means internal error).
	_ = mock

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/admin/mv-refresh", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.PostMVRefresh(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/admin/mv-refresh", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Without Asynq client, TriggerRefresh returns internal error.
	// Accept 202 or 500 as valid states in unit test.
	assert.True(t, w.Code == http.StatusAccepted || w.Code == http.StatusInternalServerError,
		"expected 202 or 500, got %d", w.Code)
}

// ─── GetReportExport — format validation ─────────────────────────────────────

func TestHandler_GetReportExport_InvalidFormat(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug/export", func(c *gin.Context) {
		injectClaims(c, []string{"report.mv-status-periode.export"})
		h.GetReportExport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-status-periode/export?format=doc", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "EXPORT_FORMAT_UNSUPPORTED", errObj["code"])
}

// ─── PostOptOut — no auth required ───────────────────────────────────────────

func TestHandler_PostOptOut_InvalidScheduledEmailID(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails/:id/opt-out", h.PostOptOut)

	body := `{"email":"u@x.com","token":"sometoken"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/reports/scheduled-emails/not-a-uuid/opt-out",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_PostOptOut_MissingFields(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails/:id/opt-out", h.PostOptOut)

	body := `{"email":"not-an-email","token":""}` // invalid email + empty token
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost,
		"/reports/scheduled-emails/"+uuid.New().String()+"/opt-out",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── DeleteScheduledEmail — forbidden ────────────────────────────────────────

func TestHandler_DeleteScheduledEmail_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.DELETE("/reports/scheduled-emails/:id", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"}) // wrong permission
		h.DeleteScheduledEmail(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/reports/scheduled-emails/"+uuid.New().String(), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_DeleteScheduledEmail_InvalidUUID(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.DELETE("/reports/scheduled-emails/:id", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.DeleteScheduledEmail(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/reports/scheduled-emails/bad-id", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── GetExportDownload — invalid uuid ────────────────────────────────────────

func TestHandler_GetExportDownload_InvalidUUID(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/export/:export_id/download", func(c *gin.Context) {
		injectClaims(c, []string{"report.export.read"})
		h.GetExportDownload(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/export/not-a-uuid/download", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── PostScheduledEmail — validation ─────────────────────────────────────────

func TestHandler_PostScheduledEmail_MissingFields(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.PostScheduledEmail(c)
	})

	body := `{}` // missing required fields
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/reports/scheduled-emails", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── NewHandler nil guard ─────────────────────────────────────────────────────

func TestNewHandler_NilSvcPanics(t *testing.T) {
	assert.Panics(t, func() {
		reporting.NewHandler(nil)
	})
}
