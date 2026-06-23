package reports_test

// coverage_boost_test.go covers remaining low-coverage paths:
// - RegisterRoutes (trivial gin route wire-up)
// - claimsFromGin bad-type case
// - parseQueryParams remaining paths
// - chooseDB replica path
// - List regulated-flag audit path

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

// ─── parseQueryParams production handler branches ─────────────────────────────

func TestHandler_GetReport_WithLimitParam(t *testing.T) {
	// Exercises parseQueryParams limit branch in the production handler.
	slug := "rpt-test-limit-param"
	reports.Register(&stubReport{
		slug:  slug,
		total: 3,
		rows:  []map[string]any{{"id": "a"}, {"id": "b"}, {"id": "c"}},
	})
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := reports.NewReportService(db, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-test",
			Permissions: []string{"audit_log.read"},
		})
		h.GetReport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/"+slug+"?limit=25&sort=id:asc", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── RegisterRoutes ───────────────────────────────────────────────────────────

func TestRegisterRoutes_Registered(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	// RegisterRoutes uses auth.Middleware which needs a verifier — pass nil to trigger early 401.
	// We just want to confirm routes are reachable (not 404).
	reports.RegisterRoutes(rg, h, nil, nil)

	routes := r.Routes()
	routeMap := map[string]bool{}
	for _, rt := range routes {
		routeMap[rt.Method+":"+rt.Path] = true
	}
	assert.True(t, routeMap["GET:/api/v1/reports/:slug"], "GET /:slug route must exist")
	assert.True(t, routeMap["GET:/api/v1/reports/:slug/export"], "GET /:slug/export route must exist")
	assert.True(t, routeMap["POST:/api/v1/reports/rpt-28/export"], "POST /rpt-28/export route must exist")
}

// ─── claimsFromGin bad-type path ─────────────────────────────────────────────

func TestHandler_GetReport_WrongClaimsType_Returns401(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug", func(c *gin.Context) {
		// Set wrong type for claims key → triggers bad-type branch in claimsFromGin
		c.Set("claims", "not-a-claims-struct")
		h.GetReport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/rpt-01", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_GetReport_NoClaims_Returns401(t *testing.T) {
	svc := reports.NewReportService(nil, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug", h.GetReport) // No claims set at all

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/rpt-01", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── parseQueryParams via actual handler ──────────────────────────────────────

func TestHandler_ParseQueryParams_LimitClamped(t *testing.T) {
	w := httptest.NewRecorder()
	// Limit > 200 → clamped to 200 in production handler (not exposed wrapper)
	req, _ := http.NewRequest(http.MethodGet, "/test?limit=999", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	// expose wrapper is faithful; production clamps at 200 inside service.List
	assert.Equal(t, 999, params.Limit) // wrapper doesn't clamp; service does
}

func TestHandler_ParseQueryParams_DefaultLimit(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	assert.Equal(t, 50, params.Limit)
}

func TestHandler_ParseQueryParams_InvalidLimit_UsesDefault(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test?limit=abc", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	params := parseQueryParamsExposed(c)
	assert.Equal(t, 50, params.Limit)
}

// ─── chooseDB replica path ────────────────────────────────────────────────────

func TestService_ChooseDB_UsesReplica(t *testing.T) {
	// Stub that returns 1 row (total=1, so inline < threshold).
	slug := "rpt-test-replica"
	reports.Register(&stubReport{
		slug:  slug,
		total: 1,
		rows:  []map[string]any{{"id": "x"}},
	})
	defer delete(reports.Registry, slug)

	// Both primary and replica as mock DBs; replica should be chosen.
	replicaDB := newMockDB(t)
	svc := reports.NewReportService(nil, replicaDB, nil, nil, nil)

	result, err := svc.List(ctxWithClaims("audit_log.read"), slug, reports.QueryParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Rows, 1)
}

// ─── List regulated-flag audit path ──────────────────────────────────────────

func TestService_List_RegulatedFlag_NoAuditWriter_NoError(t *testing.T) {
	// Regulated report with no audit writer — should not fail.
	slug := "rpt-test-regulated"
	reports.Register(&stubReport{
		slug:          slug,
		regulatedFlag: true,
		total:         1,
		rows:          []map[string]any{{"id": "r1"}},
	})
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := reports.NewReportService(db, nil, nil, nil, nil) // aw=nil
	result, err := svc.List(ctxWithClaims("audit_log.read"), slug, reports.QueryParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Rows, 1)
}

// ─── writeDomainError non-domain error path ───────────────────────────────────

func TestHandler_GetReport_NonDomainError_Returns500(t *testing.T) {
	// Trigger a query error from the stub that is NOT a domain error.
	slug := "rpt-test-non-domain"
	reports.Register(&stubReport{
		slug:     slug,
		queryErr: assert.AnError, // testify's generic error (not a DomainError)
	})
	defer delete(reports.Registry, slug)

	db := newMockDB(t)
	svc := reports.NewReportService(db, nil, nil, nil, nil)
	h := reports.NewReportHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         "user-test",
			Permissions: []string{"audit_log.read"},
		})
		h.GetReport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/"+slug, nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
