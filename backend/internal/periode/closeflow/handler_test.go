package closeflow_test

// handler_test.go — Thin handler tests using httptest + mock service.
// Tests: permission checks, bad route params, idempotency header validation.

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
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupRouter creates a test router with JWT claims pre-set.
// F-02: RegisterRoutes now accepts *gin.RouterGroup; we create the v1 group here.
func setupRouter(t *testing.T, h *closeflow.Handler, claims *auth.Claims) *gin.Engine {
	t.Helper()
	r := gin.New()

	// Inject claims into context — simulates auth middleware.
	r.Use(func(c *gin.Context) {
		if claims != nil {
			ctx := auth.ContextWithClaims(c.Request.Context(), claims)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})

	v1 := r.Group("/api/v1")
	closeflow.RegisterRoutes(v1, h, nil)
	return r
}

// claimsWithPermission creates minimal claims with a single permission.
func claimsWithPermission(perm string) *auth.Claims {
	return &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{perm},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
}

// ─── Permission denied tests ──────────────────────────────────────────────────

func TestSoftCloseRequest_MissingPermission_Returns403(t *testing.T) {
	// Build a minimal real service (will fail on DB, but handler checks perm first).
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	// Claims without softclose.request permission.
	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHardCloseApprove_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read") // wrong perm
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.WorkflowApproveBody{Comment: "test"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── Invalid route param ──────────────────────────────────────────────────────

func TestSoftCloseRequest_BadPeriodeID_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeSoftcloseRequest)
	r := setupRouter(t, h, claims)

	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/not-a-uuid/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── Missing Idempotency-Key ──────────────────────────────────────────────────

func TestSoftCloseRequest_MissingIdempotencyKey_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeSoftcloseRequest)
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	// No Idempotency-Key header!
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	require.NoError(t, err)
	errBody := errResp["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_FAILED", errBody["code"])
}

// ─── HardCloseApprove — missing step-up MFA ──────────────────────────────────

func TestHardCloseApprove_NoStepUp_Returns401(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	// Claims with valid permission but NO step-up (StepupVerifiedAt = nil).
	claims := &auth.Claims{
		Sub:              uuid.New().String(),
		Roles:            []string{"ROLE-CFO"},
		Permissions:      []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: nil, // no step-up!
	}
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.WorkflowApproveBody{Comment: "test"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	// Handler returns 401 for MFA_STEP_UP_REQUIRED.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── GetClosingChecklist — permission check ───────────────────────────────────

func TestGetClosingChecklist_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("other.permission")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/periode-buku/"+periodeID.String()+"/closing-checklist", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── ExportStatusPeriode — permission check ───────────────────────────────────

func TestExportStatusPeriode_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read") // not export
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/status-periode/export?format=csv", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestExportStatusPeriode_ValidPermission_Returns202(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeExport)
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/status-periode/export?format=csv", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	data := resp["data"].(map[string]any)
	assert.NotEmpty(t, data["jobId"])
}

// ─── NewHandler panics ────────────────────────────────────────────────────────

func TestNewHandler_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		closeflow.NewHandler(nil)
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func setupSQLMockT(t *testing.T) (*sql.DB, sqlmock.Sqlmock, error) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db, mock, err
}
