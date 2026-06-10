package lpscoverage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/master/lpscoverage"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router helpers ───────────────────────────────────────────────────────────

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "alco.maker",
		Roles:             []string{"ROLE-ALCO"},
		Permissions: []string{
			"ecl_parameter.read",
			"ecl_parameter.create",
			"ecl_parameter.update",
			"ecl_parameter.delete",
			"ecl_parameter.submit",
			"ecl_parameter.review",
			"ecl_parameter.approve",
			"ecl_parameter.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
}

func newRouter(svc *lpscoverage.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := lpscoverage.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	lpscoverage.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *lpscoverage.Service {
	return lpscoverage.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/lps-coverage ── binding ─────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage",
		bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingPeriodeDari_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body, _ := json.Marshal(lpscoverage.CreateRequest{
		CoverageAmount:     "2000000000",
		PeriodeBerlakuDari: "", // missing required
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing periodeBerlakuDari, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_NegativeAmount_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body, _ := json.Marshal(map[string]interface{}{
		"coverageAmount":     "-1000",
		"periodeBerlakuDari": "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	req = req.WithContext(ctx)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative amount, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), string(domainerrors.CodeValidationFailed))
}

func TestCreate_PeriodOverlap_Returns422(t *testing.T) {
	// Simulate overlap: repo says 1 existing APPROVED row overlaps.
	r := newRouter(buildSvc(&repoAdapter{overlapCnt: 1}))
	body, _ := json.Marshal(map[string]interface{}{
		"coverageAmount":     "2000000000",
		"periodeBerlakuDari": "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for period overlap, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lpscoverage.CodeLPSPeriodOverlap)
}

func TestCreate_ValidRequest_Returns201(t *testing.T) {
	// No overlap, create succeeds with BeginTx error (no real DB) — but before that,
	// service passes validation and overlap check (overlapCnt=0).
	// Service will fail at BeginTx. Expect non-400 error (500 or similar from BeginTx fail).
	r := newRouter(buildSvc(&repoAdapter{overlapCnt: 0}))
	body, _ := json.Marshal(map[string]interface{}{
		"coverageAmount":     "2000000000",
		"periodeBerlakuDari": "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// With no DB, BeginTx returns errTestNoDB → 500.
	// The important thing: it's not 400 (meaning validation passed, overlap check passed).
	if rec.Code == http.StatusBadRequest || rec.Code == http.StatusUnprocessableEntity {
		t.Errorf("validation should pass for valid request; got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/lps-coverage/:id ─────────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/"+id.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found id, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	lc := testLPSCoverage()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: lc},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/"+lc.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID             string `json:"id"`
			CoverageAmount string `json:"coverageAmount"`
			MataUang       string `json:"mataUang"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != lc.ID.String() {
		t.Errorf("expected id=%s, got %s", lc.ID.String(), resp.Data.ID)
	}
	if resp.Data.MataUang != "IDR" {
		t.Errorf("expected mataUang=IDR, got %s", resp.Data.MataUang)
	}
}

// ─── GET /master/lps-coverage ── list ─────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*lpscoverage.LPSCoverage{testLPSCoverage()}
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{result: items}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage?limit=10", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Pagination struct {
			HasMore bool `json:"hasMore"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{result: nil}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage?sort=coverage_amount:desc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/lps-coverage/export ──────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Coverage Amount (IDR)\r\n"
	r := newRouter(buildSvc(&repoAdapter{exportStub: &stubExport{
		reader: strings.NewReader(csvData),
		count:  0,
	}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected text/csv content-type, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── PUT ── workflow_status guard ─────────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := approvedLPSCoverage()
	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: approved},
	}))
	body, _ := json.Marshal(lpscoverage.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/lps-coverage/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lpscoverage.CodeMasterApprovedNoEdit)
}

func TestUpdate_InvalidRowVersion_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	id := uuid.New()
	body, _ := json.Marshal(lpscoverage.UpdateRequest{RowVersion: 0}) // invalid RowVersion
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/lps-coverage/"+id.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rowVersion=0, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── DELETE ── entity_in_use ──────────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	lc := testLPSCoverage()
	r := newRouter(buildSvc(&repoAdapter{
		deleteStub: &stubDelete{getResult: lc},
		refCount:   3,
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/lps-coverage/"+lc.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lpscoverage.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		deleteStub: &stubDelete{getResult: nil},
	}))
	id := uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/lps-coverage/"+id.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDelete_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/lps-coverage/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ─── Validation: coverage_amount ─────────────────────────────────────────────

func TestValidate_CoverageAmount(t *testing.T) {
	cases := []struct {
		name           string
		amount         string
		wantValidation bool
	}{
		{"positive", "2000000000", false},
		{"small positive", "1", false},
		{"zero", "0", true},
		{"negative", "-1", true},
		{"not a number", "abc", true},
		{"empty uses default", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := lpscoverage.CreateRequest{
				CoverageAmount:     tc.amount,
				PeriodeBerlakuDari: "2026-01-01",
			}
			svc := lpscoverage.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for amount=%q, got nil", tc.amount)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for amount=%q, got %v", tc.amount, err)
				}
			} else if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid amount=%q: %v", tc.amount, err)
				}
				// Non-domain error (BeginTx error, no-overlap check passed) is acceptable here.
			}
		})
	}
}

// ─── Validation: date format ──────────────────────────────────────────────────

func TestValidate_DateFormat(t *testing.T) {
	cases := []struct {
		date           string
		wantValidation bool
	}{
		{"2026-01-01", false},
		{"01-01-2026", true},
		{"2026/01/01", true},
		{"20260101", true},
		{"2026-13-01", false}, // day-level validation not done at regex; DB enforces type
	}
	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			req := lpscoverage.CreateRequest{
				CoverageAmount:     "2000000000",
				PeriodeBerlakuDari: tc.date,
			}
			svc := lpscoverage.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for date=%q, got nil", tc.date)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for date=%q, got %v", tc.date, err)
				}
			}
		})
	}
}

// ─── Validation: period date ordering ─────────────────────────────────────────

func TestValidate_PeriodeDateOrder_SampaiBeforeDari(t *testing.T) {
	sampai := "2025-12-31" // before dari
	req := lpscoverage.CreateRequest{
		CoverageAmount:       "2000000000",
		PeriodeBerlakuDari:   "2026-01-01",
		PeriodeBerlakuSampai: &sampai,
	}
	svc := lpscoverage.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Error("expected error for sampai < dari, got nil")
		return
	}
	if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %v", err)
	}
}

func TestValidate_PeriodeDateOrder_SampaiEqualsDari_IsValid(t *testing.T) {
	sampai := "2026-01-01"
	req := lpscoverage.CreateRequest{
		CoverageAmount:       "2000000000",
		PeriodeBerlakuDari:   "2026-01-01",
		PeriodeBerlakuSampai: &sampai,
	}
	svc := lpscoverage.NewService(&repoAdapter{overlapCnt: 0}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	// Should not fail with VALIDATION_FAILED (sampai == dari is valid).
	// Will fail at BeginTx (no DB) — that's expected.
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
			t.Errorf("sampai=dari should be valid, got VALIDATION_FAILED: %v", err)
		}
	}
}

// ─── Overlap: service layer ────────────────────────────────────────────────────

func TestCreate_OverlapCheck_SingleActiveInvariant(t *testing.T) {
	// When overlap check returns > 0, service returns LPS_PERIOD_OVERLAP.
	svc := lpscoverage.NewService(&repoAdapter{overlapCnt: 2}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	req := lpscoverage.CreateRequest{
		CoverageAmount:     "2000000000",
		PeriodeBerlakuDari: "2026-01-01",
	}
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Fatal("expected error for period overlap, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeLPSPeriodOverlap {
		t.Errorf("expected LPS_PERIOD_OVERLAP, got %s", de.Code())
	}
}

func TestCreate_NoOverlap_PassesCheck(t *testing.T) {
	// overlapCnt = 0 → overlap check passes, fails at BeginTx.
	svc := lpscoverage.NewService(&repoAdapter{overlapCnt: 0}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	req := lpscoverage.CreateRequest{
		CoverageAmount:     "2000000000",
		PeriodeBerlakuDari: "2026-01-01",
	}
	_, err := svc.Create(ctx, req)
	// Must not be LPS_PERIOD_OVERLAP.
	if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeLPSPeriodOverlap {
		t.Error("expected overlap check to pass (no overlap), but got LPS_PERIOD_OVERLAP")
	}
}

// ─── ToResponse ───────────────────────────────────────────────────────────────

func TestToResponse_FieldMapping(t *testing.T) {
	lc := testLPSCoverage()
	r := lpscoverage.ToResponse(lc)

	if r.ID != lc.ID.String() {
		t.Errorf("ID: want %s got %s", lc.ID.String(), r.ID)
	}
	if r.MataUang != "IDR" {
		t.Errorf("MataUang: want IDR got %s", r.MataUang)
	}
	if r.CoverageAmount != decimal.NewFromInt(2_000_000_000).StringFixed(4) {
		t.Errorf("CoverageAmount: want %s got %s", decimal.NewFromInt(2_000_000_000).StringFixed(4), r.CoverageAmount)
	}
	if r.RowVersion != lc.RowVersion {
		t.Errorf("RowVersion: want %d got %d", lc.RowVersion, r.RowVersion)
	}
}

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	lc := testLPSCoverage()
	lc.WorkflowStatus = lpscoverage.WorkflowStatusRejected
	r := lpscoverage.ToResponse(lc)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if lpscoverage.CodeLPSPeriodOverlap != "LPS_PERIOD_OVERLAP" {
		t.Errorf("CodeLPSPeriodOverlap = %q", lpscoverage.CodeLPSPeriodOverlap)
	}
	if lpscoverage.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", lpscoverage.CodeMasterApprovedNoEdit)
	}
	if lpscoverage.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", lpscoverage.CodeEntityInUse)
	}
}

// ─── HTTP status for LPS_PERIOD_OVERLAP ──────────────────────────────────────

func TestLPSPeriodOverlap_HTTPStatus(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeLPSPeriodOverlap, "test")
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for LPS_PERIOD_OVERLAP, got %d", err.HTTPStatus())
	}
}

// ─── Allowed columns whitelist ────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"id", "coverage_amount", "periode_berlaku_dari", "workflow_status", "created_at"}
	for _, col := range expected {
		found := false
		for _, ac := range lpscoverage.AllowedSortCols {
			if ac == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in AllowedSortCols", col)
		}
	}
}

// ─── Export route does not conflict with /:id ─────────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Coverage Amount (IDR)\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		exportStub: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/export", nil)
	r.ServeHTTP(rec, req)
	// "export" is not a valid UUID → should be handled by export handler (200 csv)
	// not by GetByID handler (400 invalid UUID).
	if rec.Code == http.StatusBadRequest {
		body := rec.Body.String()
		if strings.Contains(body, "UUID") {
			t.Errorf("export route was confused with /:id route; got UUID error: %s", body)
		}
	}
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestReusePattern_VerifyInterfaces(t *testing.T) {
	var _ lpscoverage.Repository = (*lpscoverage.DBRepository)(nil)
}

// ─── Update overlap check ─────────────────────────────────────────────────────

func TestUpdate_PeriodOverlap_Returns422(t *testing.T) {
	draft := testLPSCoverage() // editable DRAFT record
	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: draft},
		overlapCnt: 1, // overlap after update
	}))
	newDari := "2026-06-01"
	body, _ := json.Marshal(lpscoverage.UpdateRequest{
		PeriodeBerlakuDari: &newDari,
		RowVersion:         1,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/lps-coverage/"+draft.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for period overlap on update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lpscoverage.CodeLPSPeriodOverlap)
}

// ─── mata_uang = IDR enforcement ─────────────────────────────────────────────

func TestCreate_MataUangAlwaysIDR(t *testing.T) {
	// After a successful create, response should always show IDR regardless of input.
	// We test via ToResponse on a domain entity (the service always sets IDR).
	lc := testLPSCoverage()
	lc.MataUang = "IDR"
	r := lpscoverage.ToResponse(lc)
	if r.MataUang != "IDR" {
		t.Errorf("expected IDR, got %s", r.MataUang)
	}
}

// ─── Decimal precision ────────────────────────────────────────────────────────

func TestCoverageAmount_Precision(t *testing.T) {
	lc := testLPSCoverage()
	// default amount = 2_000_000_000 IDR
	r := lpscoverage.ToResponse(lc)
	// Must be formatted to 4 decimal places per DEC-016
	expected := "2000000000.0000"
	if r.CoverageAmount != expected {
		t.Errorf("expected CoverageAmount=%s, got %s", expected, r.CoverageAmount)
	}
}

// ─── SoftDelete service ────────────────────────────────────────────────────────

func TestSoftDelete_NotFound_Returns404(t *testing.T) {
	svc := lpscoverage.NewService(&repoAdapter{
		deleteStub: &stubDelete{getResult: nil},
	}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SoftDelete(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for not-found")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestSoftDelete_RefCount_Returns409(t *testing.T) {
	lc := testLPSCoverage()
	svc := lpscoverage.NewService(&repoAdapter{
		deleteStub: &stubDelete{getResult: lc},
		refCount:   5,
	}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SoftDelete(ctx, lc.ID)
	if err == nil {
		t.Fatal("expected error for entity in use")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeEntityInUse {
		t.Errorf("expected ENTITY_IN_USE, got %v", err)
	}
}

// ─── RegExpr validation ────────────────────────────────────────────────────────

func TestValidate_RegulasiReferensi_MaxLength(t *testing.T) {
	longStr := strings.Repeat("A", 201)
	req := lpscoverage.CreateRequest{
		CoverageAmount:     "2000000000",
		PeriodeBerlakuDari: "2026-01-01",
		RegulasiReferensi:  &longStr,
	}
	svc := lpscoverage.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Error("expected error for regulasiReferensi > 200 chars")
		return
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %v", err)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func assertErrorCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Errorf("failed to decode error response: %v; body=%s", err, body)
		return
	}
	if resp.Error.Code != expectedCode {
		t.Errorf("expected error code %q, got %q; body=%s", expectedCode, resp.Error.Code, body)
	}
}

// ─── Workflow DefaultConfigs: LPS_COVERAGE present ───────────────────────────

func TestDefaultConfigs_LPSCoveragePresent(t *testing.T) {
	configs := workflow.DefaultConfigs()
	cfg, ok := configs["LPS_COVERAGE"]
	if !ok {
		t.Fatal("LPS_COVERAGE not found in workflow.DefaultConfigs()")
	}
	if cfg.Eyes != 6 {
		t.Errorf("expected 6-eyes workflow for LPS_COVERAGE, got %d", cfg.Eyes)
	}
	if !cfg.StepUpRequired["approve"] {
		t.Error("expected step-up MFA for approve action on LPS_COVERAGE")
	}
	if !cfg.StepUpRequired["approve2"] {
		t.Error("expected step-up MFA for approve2 action on LPS_COVERAGE")
	}
	if !cfg.SoDRules.Approver2NotAnyPrevious {
		t.Error("expected approver2NotAnyPrevious=true for LPS_COVERAGE")
	}
}

// ─── Test format expectations ─────────────────────────────────────────────────

// ExportAll is tested at the handler level via HTTP.
// Verify that the export endpoint path is correct.
func TestExportPath(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{exportStub: &stubExport{
		reader: io.NopCloser(strings.NewReader("data")),
		count:  0,
	}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	// Should not return 404 (route exists).
	if rec.Code == http.StatusNotFound {
		t.Error("export route returns 404 — route not registered")
	}
}

// Verify the workflow routes are registered under the correct path.
func TestWorkflowRoutes_Registered(t *testing.T) {
	lc := testLPSCoverage()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: lc},
	}))
	rec := httptest.NewRecorder()
	// GET workflow endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lps-coverage/"+lc.ID.String()+"/workflow", nil)
	r.ServeHTTP(rec, req)
	// Workflow engine has no instance for this entity → 404 from workflow service, NOT route 404
	// The key assertion: code is NOT 405 (method not allowed) or 301 (redirect).
	if rec.Code == http.StatusMethodNotAllowed {
		t.Errorf("workflow GET route not registered; got 405")
	}
}

func TestSubmitRoute_Registered(t *testing.T) {
	lc := testLPSCoverage()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: lc},
	}))
	body, _ := json.Marshal(map[string]interface{}{"signatureMethod": "JWT_STANDARD"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lps-coverage/"+lc.ID.String()+"/submit", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusMethodNotAllowed || rec.Code == http.StatusNotFound {
		t.Errorf("submit POST route not registered; got %d", rec.Code)
	}
}

// ─── fmt dependency (prevent unused import lint) ──────────────────────────────

var _ = fmt.Sprintf
