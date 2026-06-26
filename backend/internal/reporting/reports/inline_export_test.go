package reports_test

// inline_export_test.go covers the buildInline paths (CSV, XLSX, PDF)
// and the ExportReport handler inline response path.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting/reports"
	_ "blips-ifrs9.tugu-re.com/internal/reporting/reports/impl"
)

// newInlineRouter builds a test Gin router for export that sets wildcard export permission.
func newInlineRouter(svc *reports.ReportService) *gin.Engine {
	h := reports.NewReportHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	rg.GET("/reports/:slug/export", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-test",
			TenantID:    "TUGURE",
			Permissions: []string{"report.*.export"},
		})
		h.ExportReport(c)
	})
	return r
}

func TestExport_BuildInline_CSV(t *testing.T) {
	slug := "rpt-test-inline-csv"
	reports.Register(&stubReport{
		slug:  slug,
		total: 2,
		rows:  []map[string]any{{"id": "1"}, {"id": "2"}},
	})
	defer delete(reports.Registry, slug)

	// Export uses chooseDB to call r.Query for count (stub ignores DB).
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	ctx := ctxWithClaims("report." + slug + ".export")
	result, err := svc.Export(ctx, slug, reports.QueryParams{Limit: 10}, "csv")
	require.NoError(t, err)
	require.NotNil(t, result.Inline)
	assert.Equal(t, "text/csv; charset=UTF-8", result.Inline.ContentType)
	assert.NotEmpty(t, result.Inline.Bytes)
	assert.NotEmpty(t, result.Inline.SHA256Hex)
}

func TestExport_BuildInline_XLSX(t *testing.T) {
	slug := "rpt-test-inline-xlsx"
	reports.Register(&stubReport{
		slug:  slug,
		total: 1,
		rows:  []map[string]any{{"id": "abc"}},
	})
	defer delete(reports.Registry, slug)

	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	ctx := ctxWithClaims("report." + slug + ".export")
	result, err := svc.Export(ctx, slug, reports.QueryParams{Limit: 10}, "xlsx")
	require.NoError(t, err)
	require.NotNil(t, result.Inline)
	assert.Contains(t, result.Inline.ContentType, "spreadsheetml")
	assert.NotEmpty(t, result.Inline.Bytes)
}

func TestExport_BuildInline_PDF(t *testing.T) {
	slug := "rpt-test-inline-pdf"
	reports.Register(&stubReport{
		slug:  slug,
		total: 1,
		rows:  []map[string]any{{"id": "xyz"}},
	})
	defer delete(reports.Registry, slug)

	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	ctx := ctxWithClaims("report." + slug + ".export")
	result, err := svc.Export(ctx, slug, reports.QueryParams{Limit: 10}, "pdf")
	require.NoError(t, err)
	require.NotNil(t, result.Inline)
	assert.Equal(t, "application/pdf", result.Inline.ContentType)
	assert.NotEmpty(t, result.Inline.Bytes)
}

func TestHandler_ExportReport_InlineCSV_Returns200(t *testing.T) {
	slug := "rpt-test-hdl-inline"
	reports.Register(&stubReport{
		slug:  slug,
		total: 1,
		rows:  []map[string]any{{"id": "handler-test"}},
	})
	defer delete(reports.Registry, slug)

	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	r := newInlineRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/"+slug+"/export?format=csv", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "csv")
	assert.NotEmpty(t, w.Header().Get("Content-Disposition"))
}

func TestHandler_ExportReport_InlineXLSX_Returns200(t *testing.T) {
	slug := "rpt-test-hdl-xlsx"
	reports.Register(&stubReport{
		slug:  slug,
		total: 1,
		rows:  []map[string]any{{"id": "xlsx-test"}},
	})
	defer delete(reports.Registry, slug)

	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	r := newInlineRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/reports/"+slug+"/export?format=xlsx", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "spreadsheetml")
}

func TestHandler_ParseQueryParams_ExtraParams(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet,
		"/test?calc_run_id=CR-001&periode_id=PRD-2026-06&instrumen_id=INST-999&w_good=0.3&w_normal=0.4&w_bad=0.3&q=deposito",
		nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	// Search maps to q param
	assert.Equal(t, "deposito", params.Search)
}

func TestHandler_ParseQueryParams_Cursor(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test?cursor=abc123&limit=100", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	assert.Equal(t, "abc123", params.Cursor)
	assert.Equal(t, 100, params.Limit)
}
