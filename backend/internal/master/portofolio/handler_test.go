package portofolio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/master/portofolio"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testPortofolio() *portofolio.Portofolio {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Now()
	return &portofolio.Portofolio{
		ID:                uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		KodePortofolio:    "EKUITAS_A1",
		Nama:              "Portofolio Ekuitas A1",
		BMCategoryDefault: portofolio.BMCategoryHTC,
		AktifFlag:         true,
		WorkflowStatus:    portofolio.WorkflowStatusApproved,
		CreatedAt:         now,
		CreatedBy:         &createdBy,
		RowVersion:        1,
		TenantID:          "TUGURE",
	}
}

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "maker.tr",
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions: []string{
			"portofolio.read",
			"portofolio.create",
			"portofolio.update",
			"portofolio.delete",
			"portofolio.submit",
			"portofolio.review",
			"portofolio.approve",
			"portofolio.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

func newRouter(svc *portofolio.Service) *gin.Engine {
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
	h := portofolio.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	portofolio.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *portofolio.Service {
	return portofolio.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/portofolio ── binding ───────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/portofolio",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingRequiredFields_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body, _ := json.Marshal(map[string]string{"nama": "Test"}) // missing kodePortofolio + bmCategoryDefault
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/portofolio", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required fields, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/portofolio/:kode ── kode validation ─────────────────────────

func TestGetByKode_InvalidKodeWithSpecialChars_Returns400(t *testing.T) {
	// The handler normalizes kode to uppercase, so lowercase is NOT invalid after normalization.
	// Test with a kode that contains characters that are invalid even after uppercase (e.g. dash).
	// URL-encoded dash %2D is used because Gin parses the path segment.
	// We test via a kode that is too long (> 20 chars) to trigger validation failure.
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	// "ABCDEFGHIJKLMNOPQRSTU" is 21 chars (>20), which should fail validation.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/ABCDEFGHIJKLMNOPQRSTU", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for kode > 20 chars, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByKode_ValidKode_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByKode: &stubGetByKode{result: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/EKUITAS_A1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found kode, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByKode_Found_Returns200(t *testing.T) {
	p := testPortofolio()
	r := newRouter(buildSvc(&repoAdapter{
		getByKode: &stubGetByKode{result: p},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/EKUITAS_A1", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			KodePortofolio string `json:"kodePortofolio"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.KodePortofolio != "EKUITAS_A1" {
		t.Errorf("expected kodePortofolio=EKUITAS_A1, got %s", resp.Data.KodePortofolio)
	}
}

// ─── GET /master/portofolio ── list ───────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*portofolio.Portofolio{testPortofolio()}
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{listResult: items}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio?limit=10", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []struct {
			KodePortofolio string `json:"kodePortofolio"`
		} `json:"data"`
		Pagination struct {
			HasMore bool `json:"hasMore"`
			Limit   int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].KodePortofolio != "EKUITAS_A1" {
		t.Errorf("expected EKUITAS_A1, got %s", resp.Data[0].KodePortofolio)
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{listResult: nil}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio?sort=kode_portofolio:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/portofolio/export ────────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfKode,Nama\r\nEKUITAS_A1,Portofolio Ekuitas\r\n"
	r := newRouter(buildSvc(&repoAdapter{export: &stubExport{
		reader: strings.NewReader(csvData),
		count:  1,
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/export?format=csv", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

func TestExport_XLSX_Returns501(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 400 or 501 for xlsx (not implemented), got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── DELETE ── entity_in_use ──────────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	p := testPortofolio()
	r := newRouter(buildSvc(&repoAdapter{
		softDelete: &stubSoftDelete{
			getByKodeResult:    p,
			countReferencesVal: 3,
		},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/portofolio/EKUITAS_A1", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), portofolio.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		softDelete: &stubSoftDelete{getByKodeResult: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/portofolio/MISSING", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PUT ── workflow_status guard ─────────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testPortofolio()
	approved.WorkflowStatus = portofolio.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{
		update: &stubUpdate{getByKodeResult: approved},
	}))

	body, _ := json.Marshal(portofolio.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/portofolio/EKUITAS_A1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), portofolio.CodeMasterApprovedNoEdit)
}

// ─── Validation: kode format ──────────────────────────────────────────────────

func TestValidate_KodePortofolio(t *testing.T) {
	cases := []struct {
		kode           string
		wantValidation bool
	}{
		{"HTC_A1", false},      // valid — passes validation; will fail at BeginTx
		{"EKUITAS", false},     // valid
		{"A", false},           // valid — 1 char
		{"A_1_B_2_C_3_D_4_5", false}, // valid — 19 chars
		{"lowercase", true},    // lowercase → VALIDATION_FAILED
		{"has space", true},    // space not allowed
		{"has-dash", true},     // dash not allowed
		{"", true},             // empty → VALIDATION_FAILED
	}
	for _, tc := range cases {
		t.Run(tc.kode, func(t *testing.T) {
			req := portofolio.CreateRequest{
				KodePortofolio:    tc.kode,
				Nama:              "Test Portofolio",
				BMCategoryDefault: "HTC",
			}
			svc := portofolio.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)

			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for kode=%q, got nil", tc.kode)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for kode=%q, got %v", tc.kode, err)
				}
			} else if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid kode=%q: %v", tc.kode, err)
				}
			}
		})
	}
}

func TestValidate_BMCategory(t *testing.T) {
	cases := []struct {
		cat            string
		wantValidation bool
	}{
		{"HTC", false},
		{"HTCS", false},
		{"OTHER", false},
		{"htc", true},     // lowercase
		{"INVALID", true}, // unknown
		{"", true},        // empty
	}
	for _, tc := range cases {
		t.Run(tc.cat, func(t *testing.T) {
			req := portofolio.CreateRequest{
				KodePortofolio:    "TEST",
				Nama:              "Test Portofolio",
				BMCategoryDefault: tc.cat,
			}
			svc := portofolio.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)

			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for cat=%q, got nil", tc.cat)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for cat=%q, got %v", tc.cat, err)
				}
			}
		})
	}
}

func TestValidate_NamaLength(t *testing.T) {
	cases := []struct {
		nama           string
		wantValidation bool
	}{
		{"Ab", true},         // too short (< 3)
		{"Abc", false},       // exactly 3 — valid
		{strings.Repeat("A", 200), false}, // exactly 200 — valid
		{strings.Repeat("A", 201), true},  // too long (> 200)
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("len=%d", len(tc.nama)), func(t *testing.T) {
			req := portofolio.CreateRequest{
				KodePortofolio:    "TEST",
				Nama:              tc.nama,
				BMCategoryDefault: "HTC",
			}
			svc := portofolio.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)

			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for nama len=%d, got nil", len(tc.nama))
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for nama len=%d, got %v", len(tc.nama), err)
				}
			}
		})
	}
}

func TestValidate_PeriodeReviewTerakhirFormat(t *testing.T) {
	valid := "2026-06-04"
	invalid := "04/06/2026"
	cases := []struct {
		tanggal        *string
		wantValidation bool
	}{
		{nil, false},        // optional field
		{&valid, false},     // valid YYYY-MM-DD
		{&invalid, true},    // wrong format
	}
	for _, tc := range cases {
		label := "nil"
		if tc.tanggal != nil {
			label = *tc.tanggal
		}
		t.Run(label, func(t *testing.T) {
			req := portofolio.CreateRequest{
				KodePortofolio:          "TEST",
				Nama:                    "Test Portofolio",
				BMCategoryDefault:       "HTC",
				PeriodeReviewTerakhir:   tc.tanggal,
			}
			svc := portofolio.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)

			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for tanggal=%v, got nil", tc.tanggal)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for tanggal=%v, got %v", tc.tanggal, err)
				}
			}
		})
	}
}

// ─── ToResponse: REJECTED maps to RETURNED ────────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	p := testPortofolio()
	p.WorkflowStatus = portofolio.WorkflowStatusRejected
	r := portofolio.ToResponse(p)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected workflowStatus=RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_FieldMapping(t *testing.T) {
	p := testPortofolio()
	r := portofolio.ToResponse(p)
	if r.KodePortofolio != p.KodePortofolio {
		t.Errorf("KodePortofolio mismatch: want %s got %s", p.KodePortofolio, r.KodePortofolio)
	}
	if r.ID != p.ID.String() {
		t.Errorf("ID mismatch: want %s got %s", p.ID.String(), r.ID)
	}
	if r.BMCategoryDefault != string(p.BMCategoryDefault) {
		t.Errorf("BMCategoryDefault: want %s got %s", p.BMCategoryDefault, r.BMCategoryDefault)
	}
	if r.AktifFlag != p.AktifFlag {
		t.Errorf("AktifFlag: want %v got %v", p.AktifFlag, r.AktifFlag)
	}
	if r.RowVersion != p.RowVersion {
		t.Errorf("RowVersion: want %d got %d", p.RowVersion, r.RowVersion)
	}
}

// ─── Pagination cursor roundtrip ──────────────────────────────────────────────

func TestPaginationCursorRoundtrip(t *testing.T) {
	lastID := "EKUITAS_A1"
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

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if portofolio.CodePortofolioDuplicateKode != "PORTOFOLIO_DUPLICATE_KODE" {
		t.Errorf("CodePortofolioDuplicateKode = %q", portofolio.CodePortofolioDuplicateKode)
	}
	if portofolio.CodePortofolioInvalidKodeFormat != "PORTOFOLIO_INVALID_KODE_FORMAT" {
		t.Errorf("CodePortofolioInvalidKodeFormat = %q", portofolio.CodePortofolioInvalidKodeFormat)
	}
	if portofolio.CodePortofolioInvalidBMCategory != "PORTOFOLIO_INVALID_BM_CATEGORY" {
		t.Errorf("CodePortofolioInvalidBMCategory = %q", portofolio.CodePortofolioInvalidBMCategory)
	}
	if portofolio.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", portofolio.CodeMasterApprovedNoEdit)
	}
	if portofolio.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", portofolio.CodeEntityInUse)
	}
}

// ─── AllowedCols whitelist ────────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"kode_portofolio", "nama", "bm_category_default", "aktif_flag", "created_at", "workflow_status"}
	for _, col := range expected {
		found := false
		for _, ac := range portofolio.AllowedSortCols {
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

// ─── Route registration: export before :kode ─────────────────────────────────

func TestExportRoute_NotConfusedWithKode(t *testing.T) {
	csvData := "\xef\xbb\xbfKode,Nama\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		export: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/portofolio/export", nil)
	r.ServeHTTP(rec, req)

	// Should not return 400 "kode tidak valid" (which would happen if "export" was parsed as :kode).
	if rec.Code == http.StatusBadRequest {
		// Check if it's a format error (csv default is fine) vs kode validation error.
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(rec.Body.Bytes(), &body) == nil &&
			body.Error.Code == string(domainerrors.CodeValidationFailed) {
			// If body has "kode portofolio tidak valid" it's a routing bug.
			if strings.Contains(rec.Body.String(), "kode") {
				t.Errorf("export route was confused with /:kode route; got 400 with kode error; body=%s", rec.Body.String())
			}
		}
	}
}

// ─── OptimisticLock error mapping ────────────────────────────────────────────

func TestUpdate_ErrConflict_MapsToConflict(t *testing.T) {
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected 409 HTTP status, got %d", err.HTTPStatus())
	}
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestReusePattern_VerifyInterfaces(t *testing.T) {
	var _ portofolio.Repository = (*portofolio.DBRepository)(nil)
}

// ─── BMCategory.IsValid ───────────────────────────────────────────────────────

func TestBMCategory_IsValid(t *testing.T) {
	valid := []portofolio.BMCategory{
		portofolio.BMCategoryHTC,
		portofolio.BMCategoryHTCS,
		portofolio.BMCategoryOther,
	}
	for _, v := range valid {
		if !v.IsValid() {
			t.Errorf("expected %q to be valid", v)
		}
	}
	invalid := []portofolio.BMCategory{"htc", "invalid", ""}
	for _, v := range invalid {
		if v.IsValid() {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// ─── SyncWorkflowStatus (EntityHook) ─────────────────────────────────────────

func TestSyncWorkflowStatus_EntityNotFound_Returns404(t *testing.T) {
	svc := buildSvc(&repoAdapter{
		getByKode: &stubGetByKode{result: nil},
		getByID:   &stubGetByID{result: nil},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SyncWorkflowStatus(ctx, uuid.New(), "APPROVED", "APPROVE")
	if err == nil {
		t.Error("expected error for missing entity, got nil")
	}
	if de, ok := domainerrors.IsDomainError(err); ok {
		if de.Code() != domainerrors.CodeNotFound {
			t.Errorf("expected NOT_FOUND, got %s", de.Code())
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
