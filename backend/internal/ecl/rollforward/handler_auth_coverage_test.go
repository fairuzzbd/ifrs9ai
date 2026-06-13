package rollforward_test

// handler_auth_coverage_test.go — targeted tests for handler auth/permission paths
// and RegisterRoutes smoke test.
// Covers: claimsFromCtx nil-path (401), permission denied per handler,
// RegisterRoutes wiring smoke, writeAuditEvent audit-tx rollback path,
// ExportDisclosure priorCalcRunId path.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

// buildEngineNoClaims creates an engine where no claims are injected.
// This exercises the claimsFromCtx nil path → 401.
func buildEngineNoClaims(t *testing.T, h *rollforward.Handler) *gin.Engine {
	t.Helper()
	r := gin.New()
	v1 := r.Group("/api/v1")
	rfGroup := v1.Group("/ecl/roll-forward")
	// No auth middleware — gin.Context will have no "claims" key.
	rfGroup.POST("/compute", h.ComputeRollForward)
	rfGroup.GET("", h.GetRollForward)
	rfGroup.GET("/:id/export", h.ExportDisclosure)
	rfGroup.GET("/portfolios/:pid", h.GetPortfolioRollForward)
	rfGroup.GET("/portfolios/:pid/instruments", h.ListPortfolioInstruments)

	dashGroup := v1.Group("/ecl/dashboard")
	dashGroup.GET("/ckpn-trend", h.GetCKPNTrend)
	return r
}

// buildEngineWithClaims creates an engine injecting specific permissions.
func buildEngineWithClaims(t *testing.T, h *rollforward.Handler, perms []string) *gin.Engine {
	t.Helper()
	r := gin.New()
	v1 := r.Group("/api/v1")
	mw := func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:         uuid.New().String(),
			Roles:       []string{"ROLE-RISK"},
			Permissions: perms,
		})
		c.Next()
	}
	rfGroup := v1.Group("/ecl/roll-forward")
	rfGroup.Use(mw)
	rfGroup.POST("/compute", h.ComputeRollForward)
	rfGroup.GET("", h.GetRollForward)
	rfGroup.GET("/:id/export", h.ExportDisclosure)
	rfGroup.GET("/portfolios/:pid", h.GetPortfolioRollForward)
	rfGroup.GET("/portfolios/:pid/instruments", h.ListPortfolioInstruments)

	dashGroup := v1.Group("/ecl/dashboard")
	dashGroup.Use(mw)
	dashGroup.GET("/ckpn-trend", h.GetCKPNTrend)
	return r
}

// buildHandlerForCoverage creates a handler with a sqlmock-backed service.
func buildHandlerForCoverage(t *testing.T) (*rollforward.Handler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck

	repo := rollforward.NewRepo(db)
	auditWriter := audit.NewWriter(db)
	svc := rollforward.NewService(repo, db, auditWriter, slog.Default())
	h := rollforward.NewHandler(svc)
	return h, mock
}

// ─── claimsFromCtx nil path (no "claims" key in context) → 401 ───────────────

func TestClaimsFromCtx_Nil_Returns401_Compute(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/roll-forward/compute",
		nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimsFromCtx_Nil_Returns401_GetRollForward(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ecl/roll-forward?currentCalcRunId="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimsFromCtx_Nil_Returns401_ExportDisclosure(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/"+uuid.New().String()+"/export", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimsFromCtx_Nil_Returns401_GetPortfolio(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/portfolios/"+uuid.New().String()+"?currentCalcRunId="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimsFromCtx_Nil_Returns401_ListInstruments(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/portfolios/"+uuid.New().String()+"/instruments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimsFromCtx_Nil_Returns401_CKPNTrend(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineNoClaims(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ecl/dashboard/ckpn-trend", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Permission denied paths → 403 ──────────────────────────────────────────

func TestPermDenied_ExportDisclosure_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	// No export permission.
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/"+uuid.New().String()+"/export", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPermDenied_GetPortfolioRollForward_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	// No portfolio perm.
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/portfolios/"+uuid.New().String()+"?currentCalcRunId="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPermDenied_ListPortfolioInstruments_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/portfolios/"+uuid.New().String()+"/instruments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPermDenied_Compute_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/roll-forward/compute",
		nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPermDenied_GetRollForward_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{}) // no permissions

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward?currentCalcRunId="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPermDenied_CKPNTrend_Returns403(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{}) // no permissions

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ecl/dashboard/ckpn-trend", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── writeRollForwardError — unknown error (non-domainError) → 500 ───────────

func TestWriteRollForwardError_NonDomainError_Returns500(t *testing.T) {
	h, mock := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	currentRunID := uuid.New()

	// GetCalcRunStatus query returns a DB error (forces non-domainError from service)
	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(currentRunID).
		WillReturnError(fmt.Errorf("connection refused"))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward?currentCalcRunId="+currentRunID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Service wraps the DB error in fmt.Errorf (not *domainError) → 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── RegisterRoutes smoke test ────────────────────────────────────────────────

// TestRegisterRoutes_Smoke verifies that RegisterRoutes wires all expected routes
// without panicking. Each route is tested by sending a request without an
// Authorization header — the JWT middleware (auth.Middleware) aborts with 401
// before it ever dereferences the verifier, so nil is safe here.
func TestRegisterRoutes_Smoke(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close() //nolint:errcheck
	_ = mock

	repo := rollforward.NewRepo(db)
	auditWriter := audit.NewWriter(db)
	svc := rollforward.NewService(repo, db, auditWriter, slog.Default())
	h := rollforward.NewHandler(svc)

	r := gin.New()
	v1 := r.Group("/api/v1")

	// nil verifier is safe: auth.Middleware checks Authorization header first.
	// If header is absent → 401 before v.VerifyToken is called.
	rollforward.RegisterRoutes(v1, h, nil, db)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/ecl/roll-forward/compute"},
		{http.MethodGet, "/api/v1/ecl/roll-forward?currentCalcRunId=" + uuid.New().String()},
		{http.MethodGet, "/api/v1/ecl/roll-forward/" + uuid.New().String() + "/export"},
		{http.MethodGet, "/api/v1/ecl/roll-forward/portfolios/" + uuid.New().String() + "?currentCalcRunId=" + uuid.New().String()},
		{http.MethodGet, "/api/v1/ecl/roll-forward/portfolios/" + uuid.New().String() + "/instruments"},
		{http.MethodGet, "/api/v1/ecl/dashboard/ckpn-trend"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// 401 from JWT middleware = route exists. 404 = not registered.
		if w.Code == http.StatusNotFound {
			t.Errorf("route not registered: %s %s → got 404", rt.method, rt.path)
		}
	}
}

// ─── writeAuditEvent — audit tx rollback path ─────────────────────────────────

func TestComputeRollForward_AuditTxRollback_StillReturnsReport(t *testing.T) {
	// Verifies that audit tx failure does NOT fail the compute.
	// The report is returned; only the audit write is best-effort.
	svc, mock := buildServiceWithMock(t)

	currentRunID := uuid.New()
	instrID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	currentLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "1000000.0000"}})
	expectResultLines(mock, currentRunID, currentLines)

	// Audit tx: Begin fails → writeAuditEvent logs and returns (best-effort).
	mock.ExpectBegin().WillReturnError(fmt.Errorf("DB connection pool exhausted"))

	report, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   nil,
		ActorID:          uuid.New(),
	})
	if err != nil {
		t.Fatalf("compute should succeed despite audit failure: %v", err)
	}
	if report.ReconcileStatus != rollforward.ReconcileStatusReconciled {
		t.Errorf("want RECONCILED, got %s", report.ReconcileStatus)
	}
}

// ─── ExportDisclosure with prior calc run ID ──────────────────────────────────

func TestExportDisclosure_WithPriorCalcRunId_Success(t *testing.T) {
	r, mock := buildFullTestEngine(t)

	currentRunID := uuid.New()
	priorRunID := uuid.New()
	instrID := uuid.New()

	// GetRollForward → ComputeRollForward (with prior)
	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow("SEALED", "JUNI-2026"))
	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(priorRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow("SEALED", "MEI-2026"))

	// validatePeriodeOrdering fetches tanggal_mulai from mst.periode_buku (F1).
	mock.ExpectQuery(`SELECT tanggal_mulai FROM mst.periode_buku`).
		WithArgs("MEI-2026").
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai"}).AddRow(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)))
	mock.ExpectQuery(`SELECT tanggal_mulai FROM mst.periode_buku`).
		WithArgs("JUNI-2026").
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai"}).AddRow(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)))

	priorLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "1000000.0000"}})
	currentLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "1100000.0000"}})

	mock.ExpectQuery(`SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr`).
		WithArgs(priorRunID).
		WillReturnRows(buildMockRows(priorLines))
	mock.ExpectQuery(`SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr`).
		WithArgs(currentRunID).
		WillReturnRows(buildMockRows(currentLines))

	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "calc_run_id", "trigger_type", "created_at"}))

	// Audit (1 event for ComputeRollForward)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT current_hash`).WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Audit (1 event for ExportXLSX — ECL.ROLL_FORWARD_DISCLOSURE_EXPORT, F4)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT current_hash`).WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/"+currentRunID.String()+
			"/export?priorCalcRunId="+priorRunID.String(),
		nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Invalid priorCalcRunId in GetRollForward → 400 ──────────────────────────

func TestGetRollForward_InvalidPriorCalcRunId_Returns400(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardRead})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward?currentCalcRunId="+uuid.New().String()+"&priorCalcRunId=not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Invalid priorCalcRunId in GetPortfolioRollForward → 400 ─────────────────

func TestGetPortfolioRollForward_InvalidPriorCalcRunId_Returns400(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermPortfolioAggregateRead})

	url := fmt.Sprintf("/api/v1/ecl/roll-forward/portfolios/%s?currentCalcRunId=%s&priorCalcRunId=not-uuid",
		uuid.New(), uuid.New())
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Invalid priorCalcRunId in ExportDisclosure → 400 ────────────────────────

func TestExportDisclosure_InvalidPriorCalcRunId_Returns400(t *testing.T) {
	h, _ := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermRollForwardExport})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/"+uuid.New().String()+"/export?priorCalcRunId=not-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ListPortfolioInstruments service error → 500 ────────────────────────────

func TestListPortfolioInstruments_ServiceError_Returns500(t *testing.T) {
	h, mock := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermPortfolioAggregateRead})

	portID := uuid.New()

	// GetPortofolioNama → DB error
	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnError(fmt.Errorf("DB error"))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/roll-forward/portfolios/"+portID.String()+"/instruments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// DB error from GetPortofolioNama → response.Error → 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GetPortfolioRollForward service error → 422 (domainError) ───────────────

func TestGetPortfolioRollForward_ServiceDomainError_Returns422(t *testing.T) {
	h, mock := buildHandlerForCoverage(t)
	r := buildEngineWithClaims(t, h, []string{rollforward.PermPortfolioAggregateRead})

	portID, currentRunID := uuid.New(), uuid.New()

	// Portfolio found
	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}).AddRow("Test Portfolio"))

	// Current run → DRAFT → CodeRollForwardCurrentInvalidState
	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow("DRAFT", "JUNI-2026"))

	url := fmt.Sprintf("/api/v1/ecl/roll-forward/portfolios/%s?currentCalcRunId=%s", portID, currentRunID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── service.GetPortfolioRollForward error paths (via service_mock_test helper) ─

func TestServiceGetPortfolioRollForward_DBError_CurrentLines(t *testing.T) {
	svc, mock := buildServiceWithMock(t)

	portID, currentRunID := uuid.New(), uuid.New()

	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}).AddRow("P1"))

	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow("COMPLETED", "JUNI-2026"))

	// GetResultLinesByCalcRunAndPortfolio → DB error
	mock.ExpectQuery(`FROM ecl.calc_result_line crl`).
		WillReturnError(fmt.Errorf("conn timeout"))

	_, err := svc.GetPortfolioRollForward(context.Background(), portID, currentRunID, nil, uuid.New())
	if err == nil {
		t.Fatal("expected error from DB failure")
	}
}

func TestServiceGetPortfolioRollForward_DBError_StageHistory(t *testing.T) {
	svc, mock := buildServiceWithMock(t)

	portID, currentRunID := uuid.New(), uuid.New()
	instrID := uuid.New()

	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}).AddRow("P1"))

	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow("COMPLETED", "JUNI-2026"))

	mock.ExpectQuery(`FROM ecl.calc_result_line crl`).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"}).
			AddRow(instrID, 1, "500000.0000", "5000000.0000"))

	// GetStageHistoryForCalcRun → DB error
	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WillReturnError(fmt.Errorf("connection pool exhausted"))

	_, err := svc.GetPortfolioRollForward(context.Background(), portID, currentRunID, nil, uuid.New())
	if err == nil {
		t.Fatal("expected error from stage history DB failure")
	}
}

// ─── writeAuditEvent — DB error after Begin → Rollback path ──────────────────

func TestWriteAuditEvent_AuditWriteError_StillReturnsReport(t *testing.T) {
	// Tx opens but Write fails → Rollback is called inside writeAuditEvent.
	svc, mock := buildServiceWithMock(t)

	currentRunID := uuid.New()
	instrID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	currentLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "2000000.0000"}})
	expectResultLines(mock, currentRunID, currentLines)

	// Audit tx: Begin succeeds, then INSERT fails → triggers Rollback.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT current_hash`).WillReturnError(fmt.Errorf("INSERT refused"))
	mock.ExpectRollback()

	report, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   nil,
		ActorID:          uuid.New(),
	})
	if err != nil {
		t.Fatalf("compute should succeed despite audit failure: %v", err)
	}
	if report.ReconcileStatus != rollforward.ReconcileStatusReconciled {
		t.Errorf("want RECONCILED, got %s", report.ReconcileStatus)
	}
}

// ─── NewService nil check paths ───────────────────────────────────────────────

func TestNewService_NilAuditWriter_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when auditWriter is nil")
		}
	}()
	db, _, _ := sqlmock.New()
	defer db.Close() //nolint:errcheck
	repo := rollforward.NewRepo(db)
	rollforward.NewService(repo, db, nil, slog.Default())
}

// TestNewService_NilLogger_UsesDefault verifies nil logger falls back to slog.Default
// without panic (line 50-52 in service.go — NewService normalises nil logger).
func TestNewService_NilLogger_UsesDefault(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close() //nolint:errcheck
	repo := rollforward.NewRepo(db)
	// Should not panic.
	svc := rollforward.NewService(repo, db, audit.NewWriter(db), nil)
	if svc == nil {
		t.Error("NewService with nil logger should return non-nil service")
	}
}
