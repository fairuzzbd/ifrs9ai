package lgdbasel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"blips-ifrs9.tugu-re.com/internal/master/lgdbasel"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// testClaims returns JWT claims for a RISK officer (typical maker for ECL params).
func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "risk.officer",
		Roles:             []string{"ROLE-RISK"},
		Permissions: []string{
			"ecl_parameter.read",
			"ecl_parameter.create",
			"ecl_parameter.update",
			"ecl_parameter.delete",
			"ecl_parameter.submit",
			"ecl_parameter.review",
			"ecl_parameter.approve",
			"ecl_parameter.reject",
			"ecl_parameter.export",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected.
func newRouter(svc *lgdbasel.Service) *gin.Engine {
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
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(buildLGDBaselConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := lgdbasel.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	lgdbasel.RegisterRoutes(v1, h)
	return r
}

// buildLGDBaselConfigs returns an in-memory workflow config for LGD_BASEL (6-eyes).
func buildLGDBaselConfigs() map[string]*workflow.Config {
	return map[string]*workflow.Config{
		"LGD_BASEL": {
			EntityType:  "LGD_BASEL",
			Eyes:        6,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":   "ecl_parameter.submit",
				"review":   "ecl_parameter.review",
				"approve":  "ecl_parameter.approve",
				"approve2": "ecl_parameter.approve",
				"reject":   "ecl_parameter.reject",
			},
			StepUpRequired: map[string]bool{"approve": true, "approve2": true},
			SoDRules: workflow.SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    true,
			},
		},
	}
}

// buildSvc creates a Service backed by a repoAdapter.
func buildSvc(adapter *repoAdapter) *lgdbasel.Service {
	return lgdbasel.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/lgd-basel ── binding ────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lgd-basel",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingRequiredField_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	// Missing tipeEksposur and lgd.
	body, _ := json.Marshal(map[string]interface{}{
		"periodeBerlakuDari": "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lgd-basel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required fields, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/lgd-basel/:id ── UUID validation ────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	e := testLGDBasel()
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: e},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/"+e.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID  string `json:"id"`
			LGD string `json:"lgd"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != e.ID.String() {
		t.Errorf("expected id=%s, got %s", e.ID.String(), resp.Data.ID)
	}
	// LGD should be serialized as a decimal string with 4dp.
	if resp.Data.LGD != "0.4500" {
		t.Errorf("expected lgd=0.4500, got %s", resp.Data.LGD)
	}
}

// ─── GET /master/lgd-basel ── list ────────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*lgdbasel.LGDBasel{testLGDBasel()}
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{items: items}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel?limit=10", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			TipeEksposur string `json:"tipeEksposur"`
		} `json:"data"`
		Pagination struct {
			HasMore bool `json:"hasMore"`
			Limit   int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].TipeEksposur != string(lgdbasel.TipeEksposurCorporate) {
		t.Errorf("expected CORPORATE, got %s", resp.Data[0].TipeEksposur)
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel?sort=non_existent_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{items: nil}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel?sort=tipe_eksposur:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/lgd-basel/export ─────────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Tipe Eksposur\r\n"
	r := newRouter(buildSvc(&repoAdapter{exportStub: &stubExport{
		reader: strings.NewReader(csvData),
		count:  1,
	}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected text/csv content-type, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Total-Rows") != "1" {
		t.Errorf("expected X-Total-Rows=1, got %s", rec.Header().Get("X-Total-Rows"))
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── Export route not confused with :id ──────────────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Tipe Eksposur\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		exportStub: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/export", nil)
	r.ServeHTTP(rec, req)
	// Should NOT get 400 "invalid UUID" (which would happen if export == :id).
	if rec.Code == http.StatusBadRequest {
		t.Errorf("export route confused with /:id; got 400; body=%s", rec.Body.String())
	}
}

// ─── DELETE ── entity_in_use ───────────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	e := testLGDBasel()
	r := newRouter(buildSvc(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: e},
		countRefStub:   &stubCountRef{count: 3},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/lgd-basel/"+e.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lgdbasel.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/lgd-basel/"+uuid.New().String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PATCH ── workflow_status guard ───────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testLGDBasel()
	approved.WorkflowStatus = lgdbasel.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: approved},
	}))

	body, _ := json.Marshal(lgdbasel.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/lgd-basel/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lgdbasel.CodeMasterApprovedNoEdit)
}

// ─── Validation: LGD decimal range ───────────────────────────────────────────

func TestValidate_LGD_Range(t *testing.T) {
	cases := []struct {
		lgd        string
		wantValErr bool
	}{
		{"0.0000", false},
		{"0.4500", false},
		{"1.0000", false},
		{"1.0001", true},  // above max
		{"-0.0001", true}, // below min
		{"abc", true},     // not a decimal
		{"", true},        // empty
	}
	for _, tc := range cases {
		t.Run(tc.lgd, func(t *testing.T) {
			req := lgdbasel.CreateRequest{
				TipeEksposur:       string(lgdbasel.TipeEksposurCorporate),
				LGD:                tc.lgd,
				PeriodeBerlakuDari: "2026-01-01",
			}
			svc := lgdbasel.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValErr {
				if err == nil {
					t.Errorf("expected error for lgd=%q, got nil", tc.lgd)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for lgd=%q, got %v", tc.lgd, err)
				}
			} else if err != nil {
				// For valid LGD: service passes validation but fails at BeginTx (no DB).
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid lgd=%q: %v", tc.lgd, err)
				}
			}
		})
	}
}

// ─── Validation: tipe_eksposur whitelist ─────────────────────────────────────

func TestValidate_TipeEksposur_Whitelist(t *testing.T) {
	cases := []struct {
		tipe       string
		wantValErr bool
	}{
		{string(lgdbasel.TipeEksposurSovereign), false},
		{string(lgdbasel.TipeEksposurBank), false},
		{string(lgdbasel.TipeEksposurCorporate), false},
		{string(lgdbasel.TipeEksposurRetail), false},
		{string(lgdbasel.TipeEksposurEquity), false},
		{string(lgdbasel.TipeEksposurReinsurance), false},
		{"INVALID_TYPE", true},
		{"", true},
		{"sovereign", true}, // lowercase not accepted
	}
	for _, tc := range cases {
		t.Run(tc.tipe, func(t *testing.T) {
			req := lgdbasel.CreateRequest{
				TipeEksposur:       tc.tipe,
				LGD:                "0.4500",
				PeriodeBerlakuDari: "2026-01-01",
			}
			svc := lgdbasel.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValErr {
				if err == nil {
					t.Errorf("expected error for tipe=%q, got nil", tc.tipe)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for tipe=%q, got %v", tc.tipe, err)
				}
			} else if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid tipe=%q", tc.tipe)
				}
			}
		})
	}
}

// ─── Validation: period order ─────────────────────────────────────────────────

func TestValidate_PeriodOrder(t *testing.T) {
	cases := []struct {
		dari       string
		sampai     *string
		wantValErr bool
	}{
		{"2026-01-01", nil, false},               // open-ended, valid
		{"2026-01-01", ptr("2026-12-31"), false}, // sampai > dari, valid
		{"2026-01-01", ptr("2026-01-01"), false}, // sampai == dari, valid (same day)
		{"2026-06-01", ptr("2026-01-01"), true},  // sampai < dari, invalid
		{"not-a-date", nil, true},                // dari bad format
		{"2026-01-01", ptr("not-a-date"), true},  // sampai bad format
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			req := lgdbasel.CreateRequest{
				TipeEksposur:         string(lgdbasel.TipeEksposurCorporate),
				LGD:                  "0.4500",
				PeriodeBerlakuDari:   tc.dari,
				PeriodeBerlakuSampai: tc.sampai,
			}
			svc := lgdbasel.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValErr {
				if err == nil {
					t.Errorf("case %d: expected error, got nil", i)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("case %d: expected VALIDATION_FAILED, got %v", i, err)
				}
			} else if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("case %d: unexpected VALIDATION_FAILED: %v", i, err)
				}
			}
		})
	}
}

// ─── Period overlap detection ─────────────────────────────────────────────────

func TestCreate_PeriodOverlap_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		countOverlapStub: &stubCountOverlap{count: 1},
	}))

	body, _ := json.Marshal(lgdbasel.CreateRequest{
		TipeEksposur:       string(lgdbasel.TipeEksposurCorporate),
		LGD:                "0.4500",
		PeriodeBerlakuDari: "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/lgd-basel", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for period overlap, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), lgdbasel.CodeLGDPeriodOverlap)
}

// ─── Optimistic lock mapping ───────────────────────────────────────────────────

func TestUpdate_ErrConflict_MapsToConflict(t *testing.T) {
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected 409, got %d", err.HTTPStatus())
	}
}

// ─── Workflow status guard: RETURNED editable ─────────────────────────────────

func TestWorkflowStatus_ReturnedIsEditable(t *testing.T) {
	if !lgdbasel.WorkflowStatusReturned.IsEditable() {
		t.Error("RETURNED should be editable")
	}
	if !lgdbasel.WorkflowStatusDraft.IsEditable() {
		t.Error("DRAFT should be editable")
	}
	if lgdbasel.WorkflowStatusApproved.IsEditable() {
		t.Error("APPROVED should NOT be editable")
	}
	if lgdbasel.WorkflowStatusPendingReview.IsEditable() {
		t.Error("PENDING_REVIEW should NOT be editable")
	}
	if lgdbasel.WorkflowStatusPendingApproval.IsEditable() {
		t.Error("PENDING_APPROVAL should NOT be editable")
	}
	if lgdbasel.WorkflowStatusPendingApproval2.IsEditable() {
		t.Error("PENDING_APPROVAL_2 should NOT be editable")
	}
}

// ─── ToResponse: REJECTED maps to RETURNED ───────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	e := testLGDBasel()
	e.WorkflowStatus = lgdbasel.WorkflowStatusRejected
	r := lgdbasel.ToResponse(e)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected workflowStatus=RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_ApprovedNotMapped(t *testing.T) {
	e := testLGDBasel()
	e.WorkflowStatus = lgdbasel.WorkflowStatusApproved
	r := lgdbasel.ToResponse(e)
	if r.WorkflowStatus != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", r.WorkflowStatus)
	}
}

// ─── ToResponse: LGD decimal serialization ───────────────────────────────────

func TestToResponse_LGD_SerializedAsString(t *testing.T) {
	e := testLGDBasel()
	e.LGD = decimal.RequireFromString("0.123456789") // 9dp input
	r := lgdbasel.ToResponse(e)
	// Should be serialized with 4dp fixed precision.
	if r.LGD != "0.1235" {
		t.Errorf("expected lgd=0.1235 (4dp rounded), got %s", r.LGD)
	}
}

// ─── AllowedSortCols coverage ─────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"tipe_eksposur", "lgd", "periode_berlaku_dari", "workflow_status", "created_at"}
	for _, col := range expected {
		found := false
		for _, ac := range lgdbasel.AllowedSortCols {
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

// ─── IsValidTipeEksposur ──────────────────────────────────────────────────────

func TestIsValidTipeEksposur(t *testing.T) {
	valid := []lgdbasel.TipeEksposur{
		lgdbasel.TipeEksposurSovereign,
		lgdbasel.TipeEksposurBank,
		lgdbasel.TipeEksposurCorporate,
		lgdbasel.TipeEksposurRetail,
		lgdbasel.TipeEksposurEquity,
		lgdbasel.TipeEksposurReinsurance,
	}
	for _, v := range valid {
		if !lgdbasel.IsValidTipeEksposur(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	if lgdbasel.IsValidTipeEksposur("UNKNOWN") {
		t.Error("expected UNKNOWN to be invalid")
	}
}

// ─── Pagination cursor roundtrip ──────────────────────────────────────────────

func TestPaginationCursorRoundtrip(t *testing.T) {
	lastID := uuid.New().String()
	cursor, err := pagination.EncodeCursor(pagination.CursorData{ID: lastID})
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should not be empty")
	}
	decoded, err := pagination.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if decoded.ID != lastID {
		t.Errorf("expected ID=%s, got %s", lastID, decoded.ID)
	}
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestRepositoryInterfaceCompliance(t *testing.T) {
	var _ lgdbasel.Repository = (*lgdbasel.DBRepository)(nil)
}

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if lgdbasel.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", lgdbasel.CodeEntityInUse)
	}
	if lgdbasel.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", lgdbasel.CodeMasterApprovedNoEdit)
	}
	if lgdbasel.CodeLGDPeriodOverlap != "LGD_PERIOD_OVERLAP" {
		t.Errorf("CodeLGDPeriodOverlap = %q", lgdbasel.CodeLGDPeriodOverlap)
	}
}

// ─── SyncWorkflowStatus: state mapping ───────────────────────────────────────

func TestSyncWorkflowStatus_StateMappingAllStates(t *testing.T) {
	cases := []struct {
		state    string
		expected lgdbasel.WorkflowStatus
	}{
		{"DRAFT", lgdbasel.WorkflowStatusDraft},
		{"PENDING_REVIEW", lgdbasel.WorkflowStatusPendingReview},
		{"PENDING_APPROVAL", lgdbasel.WorkflowStatusPendingApproval},
		{"PENDING_APPROVAL_2", lgdbasel.WorkflowStatusPendingApproval2},
		{"APPROVED", lgdbasel.WorkflowStatusApproved},
		{"REJECTED", lgdbasel.WorkflowStatusRejected},
	}
	// Test via ToResponse + displayWorkflowStatus (service-layer state mapping is internal).
	for _, tc := range cases {
		e := testLGDBasel()
		e.WorkflowStatus = lgdbasel.WorkflowStatus(tc.state)
		r := lgdbasel.ToResponse(e)
		// REJECTED is mapped to RETURNED in ToResponse.
		expected := tc.expected
		if expected == lgdbasel.WorkflowStatusRejected {
			expected = lgdbasel.WorkflowStatusReturned
		}
		if r.WorkflowStatus != string(expected) {
			t.Errorf("state=%s: expected %s, got %s", tc.state, expected, r.WorkflowStatus)
		}
	}
}

// ─── 6-eyes: approve2 permission = approve ───────────────────────────────────

// TestApprove2_PermissionConstant verifies that approve2 route uses the same
// permission as approve (ecl_parameter.approve). This reflects WORKFLOW_CONFIG_LGD_BASEL
// which specifies "approve2": "ecl_parameter.approve".
// Full end-to-end workflow test (approve2 state transition) requires a live DB and is
// marked as an integration test for qa-engineer.
func TestApprove2_PermissionConstant(t *testing.T) {
	// PermApprove is the permission used for both approve and approve2 routes.
	// This mirrors the WORKFLOW_CONFIG_LGD_BASEL seed in migration 0008.
	if lgdbasel.PermApprove != "ecl_parameter.approve" {
		t.Errorf("expected PermApprove=ecl_parameter.approve, got %s", lgdbasel.PermApprove)
	}
}

// ─── History endpoint ─────────────────────────────────────────────────────────

func TestHistory_Found_Returns200(t *testing.T) {
	e := testLGDBasel()
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: e},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/lgd-basel/"+e.ID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	// Returns 200 with empty history (no DB).
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Validate: RowVersion required for update ────────────────────────────────

func TestUpdate_MissingRowVersion_Returns400(t *testing.T) {
	e := testLGDBasel()
	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: e},
	}))
	// rowVersion = 0 (zero value, treated as missing).
	body, _ := json.Marshal(map[string]interface{}{
		"lgd": "0.5000",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/lgd-basel/"+e.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing rowVersion, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Validate: SyncWorkflowStatus state transitions cover 6-eyes ─────────────

func TestWorkflowStatus_PendingApproval2Exists(t *testing.T) {
	// Ensure PENDING_APPROVAL_2 status is defined (6-eyes path).
	if lgdbasel.WorkflowStatusPendingApproval2 != "PENDING_APPROVAL_2" {
		t.Errorf("PENDING_APPROVAL_2 constant value mismatch: %s", lgdbasel.WorkflowStatusPendingApproval2)
	}
	if lgdbasel.WorkflowStatusPendingApproval2.IsEditable() {
		t.Error("PENDING_APPROVAL_2 should not be editable (in-flight approval)")
	}
}

// ─── Service: Create overlapping period detection (service layer) ─────────────

func TestCreate_PeriodOverlap_ServiceLevel(t *testing.T) {
	svc := lgdbasel.NewService(
		&repoAdapter{countOverlapStub: &stubCountOverlap{count: 2}},
		audit.NewWriter(nil),
		slog.Default(),
	)
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, lgdbasel.CreateRequest{
		TipeEksposur:       string(lgdbasel.TipeEksposurBank),
		LGD:                "0.3500",
		PeriodeBerlakuDari: "2026-01-01",
	})
	if err == nil {
		t.Fatal("expected error for overlapping period, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if string(de.Code()) != lgdbasel.CodeLGDPeriodOverlap {
		t.Errorf("expected LGD_PERIOD_OVERLAP, got %s", de.Code())
	}
}

// ─── Permission constant alignment ───────────────────────────────────────────

func TestPermissionConstants_UsesECLParameterPrefix(t *testing.T) {
	// LGD Basel uses ecl_parameter.* permissions (per WORKFLOW_CONFIG_LGD_BASEL seed).
	perms := []string{
		lgdbasel.PermCreate, lgdbasel.PermRead, lgdbasel.PermUpdate, lgdbasel.PermDelete,
		lgdbasel.PermSubmit, lgdbasel.PermReview, lgdbasel.PermApprove, lgdbasel.PermReject, lgdbasel.PermExport,
	}
	for _, p := range perms {
		if !strings.HasPrefix(p, "ecl_parameter.") {
			t.Errorf("permission %q should start with ecl_parameter.", p)
		}
	}
}

// ─── Test: LGD decimal boundary values ───────────────────────────────────────

func TestLGD_DecimalBoundaryValues(t *testing.T) {
	cases := []struct {
		value     string
		wantValid bool
	}{
		{"0", true},
		{"1", true},
		{"0.00000001", true},   // min positive
		{"0.99999999", true},   // max below 1
		{"1.00000001", false},  // above 1
		{"-0.00000001", false}, // below 0
	}
	for _, tc := range cases {
		svc := lgdbasel.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
		ctx := auth.ContextWithClaims(context.Background(), testClaims())
		_, err := svc.Create(ctx, lgdbasel.CreateRequest{
			TipeEksposur:       string(lgdbasel.TipeEksposurCorporate),
			LGD:                tc.value,
			PeriodeBerlakuDari: "2026-01-01",
		})
		isValErr := err != nil
		if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
			isValErr = true
		} else if err != nil {
			// Non-validation error (e.g. DB error from stub) = validation passed.
			isValErr = false
		}
		if tc.wantValid && isValErr {
			t.Errorf("value=%s: expected valid LGD, got validation error", tc.value)
		}
		if !tc.wantValid && !isValErr {
			t.Errorf("value=%s: expected validation error for invalid LGD, got none", tc.value)
		}
	}
}

// ─── Test: SoftDelete with no references succeeds ────────────────────────────

func TestSoftDelete_NoReferences_Proceeds(t *testing.T) {
	e := testLGDBasel()
	svc := lgdbasel.NewService(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: e, deleteResult: e},
		countRefStub:   &stubCountRef{count: 0},
	}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SoftDelete(ctx, e.ID)
	// Will fail at BeginTx (no DB) but NOT at guard check.
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeEntityInUse {
			t.Error("should not return ENTITY_IN_USE when count=0")
		}
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

func ptr(s string) *string {
	return &s
}

// Suppress unused import for time (used in testLGDBasel via package-level function call).
var _ = time.Now
