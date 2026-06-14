package matauang_test

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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/master/matauang"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ──────────────────────────────────────────────────────────

// testMataUang returns a sample MataUang entity.
func testMataUang() *matauang.MataUang {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Now()
	return &matauang.MataUang{
		KodeMataUang:      "USD",
		ID:                uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		NamaMataUang:      "Dolar Amerika Serikat",
		Simbol:            "$",
		DecimalPlaces:     2,
		SumberKursDefault: "BI_JISDOR",
		FrekuensiUpdate:   "HARIAN",
		AktifFlag:         true,
		TanggalMulaiAktif: "2020-01-01",
		IsSystemCurrency:  false,
		WorkflowStatus:    matauang.WorkflowStatusApproved,
		CreatedAt:         now,
		CreatedBy:         &createdBy,
		RowVersion:        1,
		TenantID:          "TUGURE",
	}
}

// testClaims returns JWT claims for a regular AKUN maker user.
func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "akun.maker",
		Roles:             []string{"ROLE-AKUN"},
		Permissions: []string{
			"mata_uang.read",
			"mata_uang.create",
			"mata_uang.update",
			"mata_uang.delete",
			"mata_uang.submit",
			"mata_uang.review",
			"mata_uang.approve",
			"mata_uang.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected.
func newRouter(svc *matauang.Service) *gin.Engine {
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
	h := matauang.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	matauang.RegisterRoutes(v1, h)
	return r
}

// buildSvc is shorthand to create a Service backed by a repoAdapter.
func buildSvc(adapter *repoAdapter) *matauang.Service {
	return matauang.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/mata-uang ── binding ────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mata-uang",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/mata-uang/:kode ── kode validation ───────────────────────

func TestGetByKode_TwoLetterKode_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/ID", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 2-letter kode, got %d", rec.Code)
	}
}

func TestGetByKode_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByKode: &stubGetByKode{result: nil, err: nil}, // nil result = not found
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/XYZ", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found kode, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByKode_Found_Returns200(t *testing.T) {
	m := testMataUang()
	r := newRouter(buildSvc(&repoAdapter{
		getByKode: &stubGetByKode{result: m},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/USD", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			KodeMataUang string `json:"kodeMataUang"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.KodeMataUang != "USD" {
		t.Errorf("expected kodeMataUang=USD, got %s", resp.Data.KodeMataUang)
	}
}

// ─── GET /master/mata-uang ── list ────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*matauang.MataUang{testMataUang()}
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{listResult: items}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang?limit=10", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []struct {
			KodeMataUang string `json:"kodeMataUang"`
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
	if resp.Data[0].KodeMataUang != "USD" {
		t.Errorf("expected USD, got %s", resp.Data[0].KodeMataUang)
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	// "nama" is not in AllowedSortCols; "nama_mata_uang" is.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang?sort=nama:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{listResult: nil}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang?sort=kode_mata_uang:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/mata-uang/export ─────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfKode,Nama\r\nUSD,Dolar\r\n"
	r := newRouter(buildSvc(&repoAdapter{export: &stubExport{
		reader: strings.NewReader(csvData),
		count:  1,
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/export?format=csv", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── DELETE ── system_currency_protected ──────────────────────────────────

func TestDelete_SystemCurrencyProtected_Returns403(t *testing.T) {
	idr := testMataUang()
	idr.KodeMataUang = "IDR"
	idr.IsSystemCurrency = true

	r := newRouter(buildSvc(&repoAdapter{
		softDelete: &stubSoftDelete{getByKodeResult: idr},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mata-uang/IDR", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for system currency, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), matauang.CodeSystemCurrencyProtected)
}

// ─── DELETE ── entity_in_use ──────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	usd := testMataUang()
	r := newRouter(buildSvc(&repoAdapter{
		softDelete: &stubSoftDelete{
			getByKodeResult:    usd,
			countReferencesVal: 5,
		},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mata-uang/USD", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), matauang.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		softDelete: &stubSoftDelete{getByKodeResult: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mata-uang/XYZ", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PUT ── workflow_status guard ─────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testMataUang()
	approved.WorkflowStatus = matauang.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{
		update: &stubUpdate{getByKodeResult: approved},
	}))

	body, _ := json.Marshal(matauang.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/mata-uang/USD", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), matauang.CodeMasterApprovedNoEdit)
}

// ─── Service-level: optimistic lock conflict ──────────────────────────────
// This is tested at service layer because the test stub cannot provide a real
// *sql.Tx (requires live DB connection). Integration test with real DB is
// marked below for qa-engineer.
//
// INTEGRATION TEST MARKER:
//
//	TestUpdate_OptimisticLockConflict_Integration (in integration_test.go)
//	Requires: live PG, calls PUT twice with same rowVersion → expect 409 CONFLICT.
func TestUpdate_ErrConflict_MapsToConflict(t *testing.T) {
	// Service maps ErrConflict → domainerrors.ErrConflict() → 409
	// We test the mapping via domainerrors package, not through full HTTP.
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected 409 HTTP status, got %d", err.HTTPStatus())
	}
}

// ─── Validation: kode ISO 4217 format ─────────────────────────────────────

func TestValidate_KodeMataUang(t *testing.T) {
	cases := []struct {
		kode           string
		wantValidation bool // want a VALIDATION_FAILED domain error
	}{
		{"IDR", false}, // valid — passes validation; will fail later at BeginTx (no DB)
		{"USD", false},
		{"GBP", false},
		{"id", true},   // lowercase fails regex → VALIDATION_FAILED
		{"USDD", true}, // too long → VALIDATION_FAILED
		{"12D", true},  // starts with digit → VALIDATION_FAILED
		{"", true},     // empty → VALIDATION_FAILED
	}
	for _, tc := range cases {
		t.Run(tc.kode, func(t *testing.T) {
			req := matauang.CreateRequest{
				KodeMataUang:      tc.kode,
				NamaMataUang:      "Test Currency",
				Simbol:            "$",
				DecimalPlaces:     2,
				SumberKursDefault: "BI_JISDOR",
				FrekuensiUpdate:   "HARIAN",
				TanggalMulaiAktif: "2026-01-01",
			}
			// Use a stub repo that returns error on BeginTx so valid-kode paths
			// fail cleanly without nil-panic.
			svc := matauang.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
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
				// For valid kode: service passes validation but fails at BeginTx.
				// The error should NOT be a VALIDATION_FAILED.
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid kode=%q", tc.kode)
				}
				// Non-domain error (BeginTx error from stub) is expected here — ok.
			}
		})
	}
}

func TestValidate_DecimalPlaces(t *testing.T) {
	cases := []struct {
		dp             int16
		wantValidation bool
	}{
		{0, false},
		{2, false},
		{4, false},
		{-1, true},
		{5, true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("dp=%d", tc.dp), func(t *testing.T) {
			req := matauang.CreateRequest{
				KodeMataUang:      "TST",
				NamaMataUang:      "Test Currency",
				Simbol:            "T",
				DecimalPlaces:     tc.dp,
				SumberKursDefault: "BI_JISDOR",
				FrekuensiUpdate:   "HARIAN",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := matauang.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for dp=%d, got nil", tc.dp)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for dp=%d, got %v", tc.dp, err)
				}
			}
		})
	}
}

func TestValidate_TanggalFormat(t *testing.T) {
	cases := []struct {
		tanggal        string
		wantValidation bool
	}{
		{"2026-06-03", false},
		{"06-03-2026", true}, // wrong format
		{"2026/06/03", true}, // wrong separator
		{"20260603", true},   // no separators
	}
	for _, tc := range cases {
		t.Run(tc.tanggal, func(t *testing.T) {
			req := matauang.CreateRequest{
				KodeMataUang:      "TST",
				NamaMataUang:      "Test Currency",
				Simbol:            "T",
				DecimalPlaces:     2,
				SumberKursDefault: "BI_JISDOR",
				FrekuensiUpdate:   "HARIAN",
				TanggalMulaiAktif: tc.tanggal,
			}
			svc := matauang.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for tanggal=%q, got nil", tc.tanggal)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for tanggal=%q, got %v", tc.tanggal, err)
				}
			}
		})
	}
}

// ─── ToResponse: REJECTED maps to RETURNED ────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	m := testMataUang()
	m.WorkflowStatus = matauang.WorkflowStatusRejected
	r := matauang.ToResponse(m)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected workflowStatus=RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_FieldMapping(t *testing.T) {
	m := testMataUang()
	r := matauang.ToResponse(m)
	if r.KodeMataUang != m.KodeMataUang {
		t.Errorf("KodeMataUang mismatch: want %s got %s", m.KodeMataUang, r.KodeMataUang)
	}
	if r.ID != m.ID.String() {
		t.Errorf("ID mismatch: want %s got %s", m.ID.String(), r.ID)
	}
	if r.DecimalPlaces != m.DecimalPlaces {
		t.Errorf("DecimalPlaces: want %d got %d", m.DecimalPlaces, r.DecimalPlaces)
	}
	if r.IsSystemCurrency != m.IsSystemCurrency {
		t.Errorf("IsSystemCurrency: want %v got %v", m.IsSystemCurrency, r.IsSystemCurrency)
	}
	if r.RowVersion != m.RowVersion {
		t.Errorf("RowVersion: want %d got %d", m.RowVersion, r.RowVersion)
	}
}

// ─── Pagination cursor roundtrip ──────────────────────────────────────────

func TestPaginationCursorRoundtrip(t *testing.T) {
	lastID := "USD"
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

// ─── Error code constants ─────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if matauang.CodeSystemCurrencyProtected != "SYSTEM_CURRENCY_PROTECTED" {
		t.Errorf("CodeSystemCurrencyProtected = %q", matauang.CodeSystemCurrencyProtected)
	}
	if matauang.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", matauang.CodeEntityInUse)
	}
	if matauang.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", matauang.CodeMasterApprovedNoEdit)
	}
}

// ─── allowedCols whitelist ────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"kode_mata_uang", "nama_mata_uang", "aktif_flag", "created_at", "tanggal_mulai_aktif", "workflow_status"}
	for _, col := range expected {
		found := false
		for _, ac := range matauang.AllowedSortCols {
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

// ─── Route registration: export before :kode ─────────────────────────────

func TestExportRoute_NotConfusedWithKode(t *testing.T) {
	// If export is registered after /:kode, Gin will treat "export" as a kode param.
	// This test verifies "export" URL goes to the export handler, not GetByKode.
	csvData := "\xef\xbb\xbfKode,Nama\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		export: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mata-uang/export", nil)
	r.ServeHTTP(rec, req)

	// Should get 200 with CSV content-type, NOT 400 "kode harus 3 huruf kapital"
	// (since "export" has 6 chars, GetByKode would return 400 validation error).
	if rec.Code == http.StatusBadRequest {
		t.Errorf("export route was confused with /:kode route; got 400; body=%s", rec.Body.String())
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────

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

// ─── Stub types (used by repoAdapter in testutil_test.go) ─────────────────

type stubCreate struct {
	createErr error
}

type stubGetByKode struct {
	result *matauang.MataUang
	err    error
}

type stubList struct {
	listResult []*matauang.MataUang
	listErr    error
}

type stubUpdate struct {
	getByKodeResult *matauang.MataUang
	updateErr       error
	updateResult    *matauang.MataUang
}

type stubSoftDelete struct {
	getByKodeResult    *matauang.MataUang
	countReferencesVal int64
	softDeleteErr      error
	softDeleteResult   *matauang.MataUang
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

// ─── Reuse pattern documentation (informational assertions) ───────────────

// TestReusePattern_VerifyInterfaces ensures the module implements all
// required interfaces that the generic master pattern requires.
func TestReusePattern_VerifyInterfaces(t *testing.T) {
	// matauang.Repository is fully implemented by DBRepository.
	var _ matauang.Repository = (*matauang.DBRepository)(nil)
}
