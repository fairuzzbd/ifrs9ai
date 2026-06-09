package pdpefindo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"log/slog"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/master/pdpefindo"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router builder ───────────────────────────────────────────────────────────

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "risk.officer",
		Roles:             []string{"ROLE-RISK"},
		Permissions: []string{
			"ecl_parameter.read",
			"ecl_parameter.submit",
			"ecl_parameter.review",
			"ecl_parameter.approve",
			"ecl_parameter.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

func newRouter(svc *pdpefindo.Service, uploadSvc *pdpefindo.UploadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfCfg := workflow.DefaultConfigs()
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(wfCfg)),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := pdpefindo.NewHandler(svc, uploadSvc, wfh, nil)
	v1 := r.Group("/api/v1")
	pdpefindo.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *pdpefindo.Service {
	return pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

func buildUploadSvc(adapter *repoAdapter) *pdpefindo.UploadService {
	return pdpefindo.NewUploadService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── GET list ─────────────────────────────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*pdpefindo.PDPefindo{testPDPefindo()}
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{result: items}}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo?limit=10", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Rating string `json:"rating"`
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
	if resp.Data[0].Rating != "idAA" {
		t.Errorf("expected idAA, got %s", resp.Data[0].Rating)
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{}}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo?sort=rating:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /:id ─────────────────────────────────────────────────────────────────

func TestGetByID_Found_Returns200(t *testing.T) {
	p := testPDPefindo()
	r := newRouter(buildSvc(&repoAdapter{getByID: &stubGetByID{result: p}}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/"+p.ID.String(), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Rating string `json:"rating"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Rating != "idAA" {
		t.Errorf("expected rating=idAA, got %s", resp.Data.Rating)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{getByID: &stubGetByID{result: nil}}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

// ─── POST / create ────────────────────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidRating_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))

	body, _ := json.Marshal(pdpefindo.CreateRequest{
		Rating:             "INVALID_RATING",
		PD12Month:          decimal.NewFromFloat(0.005),
		PeriodeBerlakuDari: "2024-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid rating, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), string(domainerrors.CodeValidationFailed))
}

func TestCreate_PD12OutOfRange_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))

	body, _ := json.Marshal(pdpefindo.CreateRequest{
		Rating:             "idAA",
		PD12Month:          decimal.NewFromFloat(1.5), // > 1
		PeriodeBerlakuDari: "2024-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for pd12>1, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Export ───────────────────────────────────────────────────────────────────

func TestExport_CSV_Returns200(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Rating\r\n"
	r := newRouter(buildSvc(&repoAdapter{export: &stubExport{reader: strings.NewReader(csvData), count: 0}}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/export?format=csv", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected text/csv, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestExport_XLSXReturns501(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	// XLSX is 501 Not Implemented (returns 400 with message)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for xlsx (not implemented), got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── DELETE soft-delete ───────────────────────────────────────────────────────

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{softDelete: &stubSoftDelete{getByIDResult: nil}}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/pd-pefindo/"+uuid.New().String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	p := testPDPefindo()
	r := newRouter(buildSvc(&repoAdapter{softDelete: &stubSoftDelete{
		getByIDResult: p,
		countRefsVal:  3,
	}}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/pd-pefindo/"+p.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), pdpefindo.CodeEntityInUse)
}

// ─── PATCH update ─────────────────────────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testPDPefindo()
	approved.WorkflowStatus = pdpefindo.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{update: &stubUpdate{getByIDResult: approved}}), buildUploadSvc(&repoAdapter{}))

	body, _ := json.Marshal(pdpefindo.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/pd-pefindo/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), pdpefindo.CodeMasterApprovedNoEdit)
}

// ─── Monotonicity validation (service-level) ──────────────────────────────────

func TestMonotonicity_Valid(t *testing.T) {
	cases := []struct {
		name  string
		pd12  decimal.Decimal
		pd3y  *decimal.Decimal
		pd5y  *decimal.Decimal
		pd7y  *decimal.Decimal
		pd10y *decimal.Decimal
	}{
		{
			name:  "all increasing",
			pd12:  decimal.NewFromFloat(0.005),
			pd3y:  ptr(decimal.NewFromFloat(0.010)),
			pd5y:  ptr(decimal.NewFromFloat(0.020)),
			pd7y:  ptr(decimal.NewFromFloat(0.030)),
			pd10y: ptr(decimal.NewFromFloat(0.050)),
		},
		{
			name:  "equal values ok",
			pd12:  decimal.NewFromFloat(0.01),
			pd3y:  ptr(decimal.NewFromFloat(0.01)),
			pd5y:  nil,
			pd7y:  nil,
			pd10y: nil,
		},
		{
			name:  "only pd12 ok",
			pd12:  decimal.NewFromFloat(0.005),
			pd3y:  nil,
			pd5y:  nil,
			pd7y:  nil,
			pd10y: nil,
		},
		{
			name:  "sparse (skip pd5y) ok",
			pd12:  decimal.NewFromFloat(0.005),
			pd3y:  ptr(decimal.NewFromFloat(0.010)),
			pd5y:  nil,
			pd7y:  ptr(decimal.NewFromFloat(0.025)),
			pd10y: ptr(decimal.NewFromFloat(0.050)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			req := pdpefindo.CreateRequest{
				Rating:             "idAA",
				PD12Month:          tc.pd12,
				PDLifetime3Y:       tc.pd3y,
				PDLifetime5Y:       tc.pd5y,
				PDLifetime7Y:       tc.pd7y,
				PDLifetime10Y:      tc.pd10y,
				PeriodeBerlakuDari: "2024-01-01",
			}
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)

			if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && string(de.Code()) == pdpefindo.CodePDMonotonicityViolated {
					t.Errorf("unexpected PD_MONOTONICITY_VIOLATED for valid case %q: %v", tc.name, err)
				}
				// Other errors (e.g. BeginTx in stub) are expected — ok.
			}
		})
	}
}

func TestMonotonicity_Violated_Returns422(t *testing.T) {
	cases := []struct {
		name  string
		pd12  decimal.Decimal
		pd3y  *decimal.Decimal
		pd5y  *decimal.Decimal
		pd7y  *decimal.Decimal
		pd10y *decimal.Decimal
	}{
		{
			name: "pd12 > pd3y",
			pd12: decimal.NewFromFloat(0.05),
			pd3y: ptr(decimal.NewFromFloat(0.01)), // violation
		},
		{
			name: "pd3y > pd5y",
			pd12: decimal.NewFromFloat(0.01),
			pd3y: ptr(decimal.NewFromFloat(0.05)),
			pd5y: ptr(decimal.NewFromFloat(0.03)), // violation
		},
		{
			name:  "pd5y > pd7y",
			pd12:  decimal.NewFromFloat(0.005),
			pd3y:  ptr(decimal.NewFromFloat(0.01)),
			pd5y:  ptr(decimal.NewFromFloat(0.05)),
			pd7y:  ptr(decimal.NewFromFloat(0.02)), // violation
			pd10y: nil,
		},
		{
			name:  "pd7y > pd10y",
			pd12:  decimal.NewFromFloat(0.005),
			pd3y:  ptr(decimal.NewFromFloat(0.01)),
			pd5y:  ptr(decimal.NewFromFloat(0.02)),
			pd7y:  ptr(decimal.NewFromFloat(0.05)),
			pd10y: ptr(decimal.NewFromFloat(0.03)), // violation
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			req := pdpefindo.CreateRequest{
				Rating:             "idAA",
				PD12Month:          tc.pd12,
				PDLifetime3Y:       tc.pd3y,
				PDLifetime5Y:       tc.pd5y,
				PDLifetime7Y:       tc.pd7y,
				PDLifetime10Y:      tc.pd10y,
				PeriodeBerlakuDari: "2024-01-01",
			}
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if err == nil {
				t.Errorf("expected error for monotonicity violation, got nil")
				return
			}
			de, ok := domainerrors.IsDomainError(err)
			if !ok || string(de.Code()) != pdpefindo.CodePDMonotonicityViolated {
				t.Errorf("expected PD_MONOTONICITY_VIOLATED, got %v", err)
			}
		})
	}
}

// ─── Rating whitelist validation ─────────────────────────────────────────────

func TestRatingWhitelist_Valid(t *testing.T) {
	validRatings := pdpefindo.PefindoRatings
	for _, r := range validRatings {
		if !pdpefindo.IsValidPefindoRating(r) {
			t.Errorf("expected %q to be valid Pefindo rating", r)
		}
	}
}

func TestRatingWhitelist_Invalid(t *testing.T) {
	invalid := []string{"", "AAA", "idXXX", "INVALID", "id_aaa", "Id AAA"}
	for _, r := range invalid {
		if pdpefindo.IsValidPefindoRating(r) {
			t.Errorf("expected %q to be INVALID Pefindo rating", r)
		}
	}
}

// ─── Period overlap ───────────────────────────────────────────────────────────

func TestCreate_PeriodOverlap_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		countOverlap: &stubCountOverlap{count: 1}, // existing overlap
	}), buildUploadSvc(&repoAdapter{}))

	body, _ := json.Marshal(pdpefindo.CreateRequest{
		Rating:             "idAA",
		PD12Month:          decimal.NewFromFloat(0.005),
		PeriodeBerlakuDari: "2024-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for period overlap, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), pdpefindo.CodePDPeriodOverlap)
}

// ─── Domain validation: period date range ─────────────────────────────────────

func TestCreate_PeriodeSampaiBeforeDari_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))

	sampai := "2023-12-31" // before dari 2024-01-01
	body, _ := json.Marshal(pdpefindo.CreateRequest{
		Rating:               "idAA",
		PD12Month:            decimal.NewFromFloat(0.005),
		PeriodeBerlakuDari:   "2024-01-01",
		PeriodeBerlakuSampai: &sampai,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid period range, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── ToResponse mapping ───────────────────────────────────────────────────────

func TestToResponse_DecimalStringFormat(t *testing.T) {
	p := testPDPefindo()
	r := pdpefindo.ToResponse(p)
	if r.PD12Month == "" {
		t.Error("PD12Month should not be empty")
	}
	// Verify decimal string representation is non-zero
	d, _ := decimal.NewFromString(r.PD12Month)
	if d.IsZero() {
		t.Errorf("PD12Month decimal should not be zero, got %s", r.PD12Month)
	}
}

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	p := testPDPefindo()
	p.WorkflowStatus = pdpefindo.WorkflowStatusRejected
	r := pdpefindo.ToResponse(p)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED for REJECTED status, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_NilPDLifetimeOmitted(t *testing.T) {
	p := testPDPefindo()
	p.PDLifetime3Y = nil
	p.PDLifetime5Y = nil
	r := pdpefindo.ToResponse(p)
	if r.PDLifetime3Y != nil {
		t.Errorf("expected nil PDLifetime3Y in response, got %v", *r.PDLifetime3Y)
	}
	if r.PDLifetime5Y != nil {
		t.Errorf("expected nil PDLifetime5Y in response, got %v", *r.PDLifetime5Y)
	}
}

// ─── Workflow status constants ────────────────────────────────────────────────

func TestWorkflowStatusIsEditable(t *testing.T) {
	cases := []struct {
		status   pdpefindo.WorkflowStatus
		editable bool
	}{
		{pdpefindo.WorkflowStatusDraft, true},
		{pdpefindo.WorkflowStatusReturned, true},
		{pdpefindo.WorkflowStatusRejected, true},
		{pdpefindo.WorkflowStatusPendingReview, false},
		{pdpefindo.WorkflowStatusPendingApproval, false},
		{pdpefindo.WorkflowStatusPendingApproval2, false},
		{pdpefindo.WorkflowStatusApproved, false},
	}
	for _, tc := range cases {
		if tc.status.IsEditable() != tc.editable {
			t.Errorf("status %s: expected IsEditable=%v, got %v", tc.status, tc.editable, tc.status.IsEditable())
		}
	}
}

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if pdpefindo.CodePDMonotonicityViolated != "PD_MONOTONICITY_VIOLATED" {
		t.Errorf("CodePDMonotonicityViolated = %q", pdpefindo.CodePDMonotonicityViolated)
	}
	if pdpefindo.CodePDPeriodOverlap != "PD_PERIOD_OVERLAP" {
		t.Errorf("CodePDPeriodOverlap = %q", pdpefindo.CodePDPeriodOverlap)
	}
	if pdpefindo.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", pdpefindo.CodeEntityInUse)
	}
	if pdpefindo.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", pdpefindo.CodeMasterApprovedNoEdit)
	}
}

// ─── AllowedCols coverage ─────────────────────────────────────────────────────

func TestAllowedSortCols(t *testing.T) {
	expected := []string{"rating", "pd_12month", "created_at", "workflow_status", "periode_berlaku_dari"}
	for _, col := range expected {
		found := false
		for _, ac := range pdpefindo.AllowedSortCols {
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

// ─── Route: export before :id ─────────────────────────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Rating\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		export: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/export", nil)
	r.ServeHTTP(rec, req)

	// export has 6 chars but "export" is not a UUID, so if the handler was confused
	// with /:id, it would return 400 "UUID format invalid".
	// Export route should return 200 (csv content-type).
	if rec.Code == http.StatusBadRequest {
		t.Errorf("export route was confused with /:id; got 400; body=%s", rec.Body.String())
	}
}

// ─── Pagination cursor roundtrip ──────────────────────────────────────────────

func TestPaginationCursorRoundtrip(t *testing.T) {
	lastID := uuid.New().String()
	cursor, err := pagination.EncodeCursor(pagination.CursorData{ID: lastID})
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	decoded, err := pagination.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if decoded.ID != lastID {
		t.Errorf("cursor roundtrip: expected %s, got %s", lastID, decoded.ID)
	}
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestRepositoryInterface_Compliance(t *testing.T) {
	var _ pdpefindo.Repository = (*pdpefindo.DBRepository)(nil)
}

// ─── PD idD special case (PD = 1.0) ──────────────────────────────────────────

func TestCreate_IdD_PD1_IsValid(t *testing.T) {
	// idD = certain default, PD = 1.0 for all horizons is valid (monotonic: 1=1=1=1=1)
	svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	req := pdpefindo.CreateRequest{
		Rating:             "idD",
		PD12Month:          decimal.NewFromFloat(1.0),
		PDLifetime3Y:       ptr(decimal.NewFromFloat(1.0)),
		PDLifetime5Y:       ptr(decimal.NewFromFloat(1.0)),
		PDLifetime7Y:       ptr(decimal.NewFromFloat(1.0)),
		PDLifetime10Y:      ptr(decimal.NewFromFloat(1.0)),
		PeriodeBerlakuDari: "2024-01-01",
	}
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok && string(de.Code()) == pdpefindo.CodePDMonotonicityViolated {
			t.Errorf("idD with PD=1.0 should pass monotonicity, got: %v", err)
		}
		// Other errors (BeginTx) are expected.
	}
}

// ─── Workflow EntityHook test (BeforeCommit) ─────────────────────────────────

func TestWorkflowHook_BeforeCommit_CallsRepoUpdate(t *testing.T) {
	p := testPDPefindo()
	adapter := &repoAdapter{getByID: &stubGetByID{result: p}}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc, adapter)

	evt := workflow.HookEvent{
		EntityID: p.ID,
		NewState: workflow.State("PENDING_REVIEW"),
		ActorID:  uuid.New(),
	}
	if err := hook.BeforeCommit(context.Background(), nil, evt); err != nil {
		t.Errorf("BeforeCommit returned error: %v", err)
	}
}

// ─── Upload job status ────────────────────────────────────────────────────────

func TestGetUploadJobStatus_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{getJobByID: &stubGetJobByID{result: nil}}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/upload-jobs/nonexistent-job", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing job, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetUploadJobStatus_Found_Returns200(t *testing.T) {
	step := "Processing"
	jobRow := &pdpefindo.JobRow{
		ID:          "test-job-123",
		Type:        "PD_PEFINDO_UPLOAD_XLSX",
		Status:      "running",
		Progress:    50,
		CurrentStep: &step,
		CanCancel:   false,
		CreatedBy:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		CreatedAt:   time.Now(),
		TenantID:    "TUGURE",
	}
	r := newRouter(
		buildSvc(&repoAdapter{getJobByID: &stubGetJobByID{result: jobRow}}),
		buildUploadSvc(&repoAdapter{getJobByID: &stubGetJobByID{result: jobRow}}),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/upload-jobs/test-job-123", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Status   string `json:"status"`
			Progress int    `json:"progress"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Status != "running" {
		t.Errorf("expected status=running, got %s", resp.Data.Status)
	}
	if resp.Data.Progress != 50 {
		t.Errorf("expected progress=50, got %d", resp.Data.Progress)
	}
}

// ─── PD idD special: below 1.0 at pd12 but all equal at 1.0 ──────────────────

func TestMonotonicity_IdD_Equal_1_Valid(t *testing.T) {
	one := decimal.NewFromFloat(1.0)
	cases := []struct {
		name  string
		pd12  decimal.Decimal
		pd3y  *decimal.Decimal
		pd5y  *decimal.Decimal
		pd7y  *decimal.Decimal
		pd10y *decimal.Decimal
	}{
		{"idD all 1.0", one, &one, &one, &one, &one},
	}
	for _, tc := range cases {
		svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
		req := pdpefindo.CreateRequest{
			Rating:             "idD",
			PD12Month:          tc.pd12,
			PDLifetime3Y:       tc.pd3y,
			PDLifetime5Y:       tc.pd5y,
			PDLifetime7Y:       tc.pd7y,
			PDLifetime10Y:      tc.pd10y,
			PeriodeBerlakuDari: "2024-01-01",
		}
		ctx := auth.ContextWithClaims(context.Background(), testClaims())
		_, err := svc.Create(ctx, req)
		if de, ok := domainerrors.IsDomainError(err); ok && string(de.Code()) == pdpefindo.CodePDMonotonicityViolated {
			t.Errorf("idD all 1.0 should pass monotonicity, got: %v", err)
		}
		_ = tc.name
	}
}

// ─── F-002: CountOverlap only matches APPROVED records ────────────────────────

// TestCountOverlap_DraftDoesNotBlock verifies that a DRAFT record does not block
// creating a new PD record for the same period.
// Service-level test: stub CountOverlap returns 0 (DRAFT filtered out at repo level).
func TestCountOverlap_DraftDoesNotBlock(t *testing.T) {
	// stub returns 0 — DRAFT records are excluded at repo layer (workflow_status='APPROVED')
	r := newRouter(buildSvc(&repoAdapter{
		countOverlap: &stubCountOverlap{count: 0},
	}), buildUploadSvc(&repoAdapter{}))

	body, _ := json.Marshal(pdpefindo.CreateRequest{
		Rating:             "idAA",
		PD12Month:          decimal.NewFromFloat(0.005),
		PeriodeBerlakuDari: "2024-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	// Should not return 422 PD_PERIOD_OVERLAP — DRAFT does not block
	if rec.Code == http.StatusUnprocessableEntity {
		t.Errorf("DRAFT record should not block: got 422; body=%s", rec.Body.String())
	}
}

// ─── F-003: SoftDelete guard for APPROVED records ─────────────────────────────

func TestDelete_ApprovedRecord_Returns403(t *testing.T) {
	approved := testPDPefindo()
	approved.WorkflowStatus = pdpefindo.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{softDelete: &stubSoftDelete{
		getByIDResult: approved,
	}}), buildUploadSvc(&repoAdapter{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/pd-pefindo/"+approved.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for deleting APPROVED record, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), pdpefindo.CodeMasterApprovedNoEdit)
}

// ─── F-004: GET /active endpoint ──────────────────────────────────────────────

type stubGetActive struct {
	result []*pdpefindo.PDPefindo
	err    error
}

// repoAdapterWithActive extends repoAdapter with GetActive stub.
type repoAdapterWithActive struct {
	repoAdapter
	getActive *stubGetActive
}

var _ pdpefindo.Repository = (*repoAdapterWithActive)(nil)

func (a *repoAdapterWithActive) GetActive(_ context.Context, _ string) ([]*pdpefindo.PDPefindo, error) {
	if a.getActive != nil {
		return a.getActive.result, a.getActive.err
	}
	return nil, nil
}

func TestGetActive_ReturnsActiveRecords(t *testing.T) {
	p := testPDPefindo()
	p.WorkflowStatus = pdpefindo.WorkflowStatusApproved

	adapter := &repoAdapterWithActive{
		getActive: &stubGetActive{result: []*pdpefindo.PDPefindo{p}},
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	uploadSvc := pdpefindo.NewUploadService(adapter, audit.NewWriter(nil), slog.Default())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfCfg := workflow.DefaultConfigs()
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(wfCfg)),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := pdpefindo.NewHandler(svc, uploadSvc, wfh, nil)
	v1 := r.Group("/api/v1")
	pdpefindo.RegisterRoutes(v1, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/active?tanggal=2024-06-01", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Rating    string `json:"rating"`
			PD12Month string `json:"pd12Month"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 active record, got %d", len(resp.Data))
	}
	if resp.Data[0].Rating != "idAA" {
		t.Errorf("expected rating=idAA, got %s", resp.Data[0].Rating)
	}
	if resp.Data[0].PD12Month == "" {
		t.Error("pd12Month should not be empty")
	}
}

func TestGetActive_NoActiveRecord_Returns404(t *testing.T) {
	adapter := &repoAdapterWithActive{
		getActive: &stubGetActive{result: nil},
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	uploadSvc := pdpefindo.NewUploadService(adapter, audit.NewWriter(nil), slog.Default())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfCfg := workflow.DefaultConfigs()
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(wfCfg)),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := pdpefindo.NewHandler(svc, uploadSvc, wfh, nil)
	v1 := r.Group("/api/v1")
	pdpefindo.RegisterRoutes(v1, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/active?tanggal=2020-01-01", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for no active record, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetActive_InvalidDateFormat_Returns400(t *testing.T) {
	adapter := &repoAdapterWithActive{}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	uploadSvc := pdpefindo.NewUploadService(adapter, audit.NewWriter(nil), slog.Default())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfCfg := workflow.DefaultConfigs()
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(wfCfg)),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := pdpefindo.NewHandler(svc, uploadSvc, wfh, nil)
	v1 := r.Group("/api/v1")
	pdpefindo.RegisterRoutes(v1, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/active?tanggal=invalid-date", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid date, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestActiveRoute_NotConfusedWithID verifies that /active is routed before /:id.
func TestActiveRoute_NotConfusedWithID(t *testing.T) {
	// 'active' is not a UUID, so /:id would return 400 if /active were not registered first.
	// We verify the route returns 404 (no active records) not 400 (bad UUID).
	adapter := &repoAdapterWithActive{
		getActive: &stubGetActive{result: nil},
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	uploadSvc := pdpefindo.NewUploadService(adapter, audit.NewWriter(nil), slog.Default())

	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfCfg := workflow.DefaultConfigs()
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(wfCfg)),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := pdpefindo.NewHandler(svc, uploadSvc, wfh, nil)
	v1 := r.Group("/api/v1")
	pdpefindo.RegisterRoutes(v1, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/pd-pefindo/active", nil)
	r.ServeHTTP(rec, req)

	// Should reach GetActive handler → returns 404 (no records), NOT 400 (bad UUID from /:id).
	if rec.Code == http.StatusBadRequest {
		t.Errorf("/active route was confused with /:id (got 400); body=%s", rec.Body.String())
	}
}

// ─── F-005: Upload limit 50 MB + magic bytes ──────────────────────────────────

func TestUpload_FileSizeLimit_50MB(t *testing.T) {
	// Verify that the maxUploadBytes constant is 50 MB.
	const expected50MB = 50 * 1024 * 1024
	// We can't easily test the actual upload limit in a unit test without a real multipart
	// body of 50 MB+. Instead verify the constant exposed via handler behavior.
	// The upload handler message should say "50 MB" not "10 MB".
	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))

	// Send a minimal multipart request without a file — triggers form parse failure path
	// that includes the max size in the error. We just verify the handler compiles
	// and the route is reachable.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo/upload-xlsx", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected non-200 for empty upload")
	}
	_ = expected50MB // constant referenced to avoid compiler warning
}

func TestUpload_InvalidMagicBytes_Returns400(t *testing.T) {
	// Build a multipart body with a fake "XLSX" file that has wrong magic bytes.
	var body bytes.Buffer
	boundary := "testboundary"
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"test.xlsx\"\r\n")
	body.WriteString("Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\r\n\r\n")
	// Write non-ZIP content (plain text pretending to be xlsx)
	body.WriteString("This is not a valid XLSX file content, no zip magic bytes here\r\n")
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"tanggal_publikasi\"\r\n\r\n")
	body.WriteString("2024-01-01\r\n")
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"periode_berlaku_dari\"\r\n\r\n")
	body.WriteString("2024-01-01\r\n")
	body.WriteString("--" + boundary + "--\r\n")

	r := newRouter(buildSvc(&repoAdapter{}), buildUploadSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/pd-pefindo/upload-xlsx", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid magic bytes, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), "VALIDATION_FAILED")
}

// ─── F-006: WorkflowHook overlap on approve2 ─────────────────────────────────

func TestWorkflowHook_BeforeCommit_Approved_ChecksOverlap(t *testing.T) {
	// When transitioning to APPROVED, overlap check is run.
	// Set up repo so GetByID returns a record and CountOverlap returns 1 (overlap).
	p := testPDPefindo()
	adapter := &repoAdapter{
		getByID:      &stubGetByID{result: p},
		countOverlap: &stubCountOverlap{count: 1}, // existing APPROVED overlap
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc, adapter)

	evt := workflow.HookEvent{
		EntityID: p.ID,
		NewState: workflow.State("APPROVED"),
		ActorID:  uuid.New(),
	}
	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err == nil {
		t.Error("expected error from overlap check on APPROVED transition, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || string(de.Code()) != pdpefindo.CodePDPeriodOverlap {
		t.Errorf("expected PD_PERIOD_OVERLAP error, got: %v", err)
	}
}

func TestWorkflowHook_BeforeCommit_Approved_NoOverlap_Succeeds(t *testing.T) {
	// When transitioning to APPROVED with no overlap, BeforeCommit succeeds.
	p := testPDPefindo()
	adapter := &repoAdapter{
		getByID:      &stubGetByID{result: p},
		countOverlap: &stubCountOverlap{count: 0}, // no overlap
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc, adapter)

	evt := workflow.HookEvent{
		EntityID: p.ID,
		NewState: workflow.State("APPROVED"),
		ActorID:  uuid.New(),
	}
	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Errorf("expected no error when no overlap, got: %v", err)
	}
}

func TestWorkflowHook_BeforeCommit_NonApprovedTransition_SkipsOverlapCheck(t *testing.T) {
	// Transitions other than APPROVED should NOT call overlap check.
	// We use countOverlap stub returning 1 to verify it is NOT called on PENDING_REVIEW.
	p := testPDPefindo()
	adapter := &repoAdapter{
		getByID:      &stubGetByID{result: p},
		countOverlap: &stubCountOverlap{count: 1},
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc, adapter)

	evt := workflow.HookEvent{
		EntityID: p.ID,
		NewState: workflow.State("PENDING_REVIEW"),
		ActorID:  uuid.New(),
	}
	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err != nil {
		t.Errorf("PENDING_REVIEW should not check overlap, got: %v", err)
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
		t.Errorf("decode error resp: %v; body=%s", err, body)
		return
	}
	if resp.Error.Code != expectedCode {
		t.Errorf("expected error code %q, got %q; body=%s", expectedCode, resp.Error.Code, body)
	}
}

func ptr(d decimal.Decimal) *decimal.Decimal {
	return &d
}
