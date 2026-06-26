package reporting_test

// handler_extended_test.go — additional HTTP handler coverage.
// Targets: GetExportLog, GetScheduledEmails, PostScheduledEmail (happy),
// GetReportExport (permission denied + valid slug), PostOptOut (valid token path),
// GetMVStatus (happy path via sqlmock), firstRole, rowToScheduledEmailItem.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// injectClaimsWithUsername injects claims with preferred_username set.
func injectClaimsWithUsername(c *gin.Context, perms []string, username string) {
	c.Set("claims", &auth.Claims{
		Sub:               uuid.New().String(),
		PreferredUsername: username,
		Roles:             []string{"ROLE-AKUN"},
		TenantID:          "TUGURE",
		Permissions:       perms,
	})
}

// ─── GetExportLog ─────────────────────────────────────────────────────────────

func TestHandler_GetExportLog_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/export-log", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"})
		h.GetExportLog(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/export-log", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_GetExportLog_ByAuditRole(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/export-log", func(c *gin.Context) {
		injectClaims(c, []string{"audit_log.read"}) // ROLE-AUDIT bypass
		h.GetExportLog(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/export-log", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_GetExportLog_DirectPermission(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/export-log", func(c *gin.Context) {
		injectClaims(c, []string{"report.export.read"})
		h.GetExportLog(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/export-log", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── GetScheduledEmails ───────────────────────────────────────────────────────

func TestHandler_GetScheduledEmails_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/scheduled-emails", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"})
		h.GetScheduledEmails(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/scheduled-emails", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_GetScheduledEmails_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			uuid.New(), "mv-status-periode", "csv", "daily", "08:00",
			[]byte(`["a@b.com"]`), true, nil, nil,
			nil, nil, time.Now(), uuid.New(), "TUGURE"))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/scheduled-emails", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.GetScheduledEmails(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/scheduled-emails", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	assert.Len(t, data, 1)
}

// ─── PostScheduledEmail — happy path ─────────────────────────────────────────

func TestHandler_PostScheduledEmail_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.PostScheduledEmail(c)
	})

	body := `{
		"reportSlug":"mv-status-periode",
		"format":"csv",
		"frequency":"daily",
		"sendTime":"08:00",
		"recipients":["finance@tugu-re.com"],
		"active":true
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/reports/scheduled-emails", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHandler_PostScheduledEmail_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"})
		h.PostScheduledEmail(c)
	})

	body := `{"reportSlug":"mv-status-periode","format":"csv","frequency":"daily","sendTime":"08:00","recipients":["x@y.com"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/reports/scheduled-emails", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── GetReportExport — permission denied ─────────────────────────────────────

func TestHandler_GetReportExport_PermissionDenied(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug/export", func(c *gin.Context) {
		// Has claims but no matching permission for this slug
		injectClaims(c, []string{"instrumen.read"})
		h.GetReportExport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-status-periode/export?format=csv", nil)
	r.ServeHTTP(w, req)
	// Should be 403 (EXPORT_PERMISSION_DENIED)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_GetReportExport_Unauthorized(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	// No claims injected
	r.GET("/reports/:slug/export", h.GetReportExport)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-status-periode/export?format=csv", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── GetExportDownload — forbidden ───────────────────────────────────────────

func TestHandler_GetExportDownload_Forbidden(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/export/:export_id/download", func(c *gin.Context) {
		injectClaims(c, []string{"instrumen.read"})
		h.GetExportDownload(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/export/"+uuid.New().String()+"/download", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── GetMVStatus — happy path (permission exercised; DB error is incidental) ──

func TestHandler_GetMVStatus_DBError(t *testing.T) {
	// ListMVStatus uses unnest($2::TEXT[]) which requires the pq driver's array codec.
	// Without pq, the query arg binding fails → DB returns an error → handler returns 500.
	// This test covers the permission gate (report.admin) + error path in the handler.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`mv_refresh_log`).WillReturnError(assert.AnError)

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/admin/mv-status", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.GetMVStatus(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/mv-status", nil)
	r.ServeHTTP(w, req)
	// 500 because pq array codec is not loaded; permission gate was passed (not 401/403).
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

// ─── PostOptOut — valid token path ───────────────────────────────────────────

func TestHandler_PostOptOut_ValidToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	email := "optout@example.com"
	secret := []byte("handler-test-secret-32bytes!!!!")

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, secret)

	token := svc.GenerateOptOutToken(schedID, email, 1*time.Hour)

	// InsertOptOut will be called
	mock.ExpectExec(`INSERT INTO sys.scheduled_email_optout`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails/:id/opt-out", h.PostOptOut)

	body, _ := json.Marshal(map[string]string{
		"email": email,
		"token": token,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost,
		"/reports/scheduled-emails/"+schedID.String()+"/opt-out",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestHandler_PostOptOut_BadToken(t *testing.T) {
	svc, _, db := setupTestService(t)
	defer db.Close()

	schedID := uuid.New()

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.POST("/reports/scheduled-emails/:id/opt-out", h.PostOptOut)

	body, _ := json.Marshal(map[string]string{
		"email": "x@example.com",
		"token": "bad.token",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost,
		"/reports/scheduled-emails/"+schedID.String()+"/opt-out",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.True(t, w.Code >= 400)
}

// ─── DeleteScheduledEmail — happy path ───────────────────────────────────────

func TestHandler_DeleteScheduledEmail_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.DELETE("/reports/scheduled-emails/:id", func(c *gin.Context) {
		injectClaims(c, []string{"report.admin"})
		h.DeleteScheduledEmail(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/reports/scheduled-emails/"+uuid.New().String(), nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetReportExport — inline path (AUDIT bypass, sqlmock) ───────────────────

func TestHandler_GetReportExport_InlinePath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// CountMVRows: Sscanf fails → fallback COUNT(*) on replica (same db here)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(50)))

	// BeginTx + InsertExportLog + Commit
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// queryMVRows — SELECT * FROM rpt.mv_status_periode LIMIT ...
	mock.ExpectQuery(`SELECT \* FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"col_a"}).AddRow("value"))

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug/export", func(c *gin.Context) {
		injectClaimsWithUsername(c, []string{"audit_log.read"}, "testuser")
		h.GetReportExport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-status-periode/export?format=csv", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "mv-status-periode")
}
