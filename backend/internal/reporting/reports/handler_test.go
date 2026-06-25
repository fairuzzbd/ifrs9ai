package reports_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl"
)

func init() { gin.SetMode(gin.TestMode) }

// ─── Test router ──────────────────────────────────────────────────────────────

func newTestRouter() *gin.Engine {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	// Manual route registration (skip auth middleware for tests).
	rg.GET("/reports/:slug", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-test",
			TenantID:    "TUGURE",
			Permissions: []string{"report.*.read", "audit_log.read"},
		})
		h.GetReport(c)
	})
	rg.GET("/reports/:slug/export", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-test",
			TenantID:    "TUGURE",
			Permissions: []string{"report.*.export"},
		})
		h.ExportReport(c)
	})
	rg.POST("/reports/rpt-28/export", func(c *gin.Context) {
		now := int64(9999999999)
		c.Set("claims", &auth.Claims{
			Sub:              "user-cfo",
			TenantID:         "TUGURE",
			Permissions:      []string{"report.rpt-28.export"},
			StepupVerifiedAt: &now,
		})
		h.ExportRegulatorPackHandler(c)
	})
	return r
}

// ─── GET /reports/:slug — REPORT_NOT_FOUND ────────────────────────────────────

func TestHandler_GetReport_NotFound(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/rpt-99", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errMap := body["error"].(map[string]any)
	assert.Equal(t, "REPORT_NOT_FOUND", errMap["code"])
}

// ─── GET /reports/:slug — parametric table-driven for all 25 slugs ────────────

func TestHandler_GetReport_All25Slugs_NoDBReturns404OrError(t *testing.T) {
	// Without a DB, the service.List will fail when calling r.Query (nil DB panic).
	// We only assert the handler returns either 404 (not found) or 500 (no DB).
	// Real DB tests belong in integration tests.
	r := newTestRouter()
	slugs := expectedSlugs // from registry_test.go

	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/"+slug, nil)
			r.ServeHTTP(w, req)
			// Acceptable: 200 (empty), 500 (no DB), not 404 (slug must be found)
			assert.NotEqual(t, http.StatusNotFound, w.Code,
				"slug %q must be found in registry (not 404)", slug)
		})
	}
}

// ─── GET /reports/:slug/export — REPORT_NOT_FOUND ────────────────────────────

func TestHandler_ExportReport_NotFound(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/rpt-99/export?format=csv", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── GET /reports/:slug/export — invalid format ───────────────────────────────

func TestHandler_ExportReport_InvalidFormat(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/rpt-01/export?format=docx", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errMap := body["error"].(map[string]any)
	assert.Equal(t, "EXPORT_FORMAT_UNSUPPORTED", errMap["code"])
}

// ─── POST /reports/rpt-28/export — missing body → 400 ───────────────────────

func TestHandler_RPT28_MissingBody(t *testing.T) {
	r := newTestRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reports/rpt-28/export",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Step-Up-Token", "valid-token")
	r.ServeHTTP(w, req)
	// Missing periode_id → REPORT_PARAMS_INVALID or VALIDATION_FAILED
	assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusUnprocessableEntity,
		"expected 400 or 422, got %d", w.Code)
}

// ─── POST /reports/rpt-28/export — no step-up → 403 ─────────────────────────

func TestHandler_RPT28_NoStepUp(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	// Claims without stepup
	rg.POST("/reports/rpt-28/export", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-cfo",
			Permissions: []string{"report.rpt-28.export"},
			// StepupVerifiedAt nil → NeedsStepUp() = true
		})
		h.ExportRegulatorPackHandler(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/reports/rpt-28/export",
		strings.NewReader(`{"periode_id":"PRD-2026-06","format":"xlsx"}`))
	req.Header.Set("Content-Type", "application/json")
	// No X-Step-Up-Token
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errMap := body["error"].(map[string]any)
	assert.Equal(t, "STEP_UP_REQUIRED", errMap["code"])
}

// ─── parseQueryParams tests ───────────────────────────────────────────────────

func TestHandler_ParseQueryParams_Sort(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test?sort=created_at:desc,stage:asc&limit=25", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	assert.Equal(t, 25, params.Limit)
	require.Len(t, params.Sort, 2)
	assert.Equal(t, "created_at", params.Sort[0].Col)
	assert.Equal(t, "desc", params.Sort[0].Dir)
	assert.Equal(t, "stage", params.Sort[1].Col)
	assert.Equal(t, "asc", params.Sort[1].Dir)
}

func TestHandler_ParseQueryParams_Filter(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test?filter[stage]=2&filter[periode_id]=eq:PRD-2026-06", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	require.NotEmpty(t, params.Filters)
	filterMap := map[string]reports.FilterSpec{}
	for _, f := range params.Filters {
		filterMap[f.Col] = f
	}
	assert.Equal(t, "eq", filterMap["stage"].Op)
	assert.Equal(t, "2", filterMap["stage"].Value)
}
