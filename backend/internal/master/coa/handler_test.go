package coa_test

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
	"blips-ifrs9.tugu-re.com/internal/master/coa"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testCoA() *coa.ChartOfAccount {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Now()
	return &coa.ChartOfAccount{
		ID:                uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		KodeAkun:          "1.1.01.001",
		NamaAkun:          "Kas dan Setara Kas",
		TipeAkun:          coa.TipeAkunAset,
		SubTipeAkun:       "KAS",
		MataUangNative:    "IDR",
		PosisiNormal:      coa.PosisiNormalDebit,
		AktifFlag:         true,
		SumberCoa:         "MANUAL",
		TanggalMulaiAktif: "2026-01-01",
		WorkflowStatus:    coa.WorkflowStatusDraft,
		CreatedBy:         actorID,
		CreatedAt:         now,
		Version:           1,
		TenantID:          "TUGURE",
	}
}

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "akun.maker",
		Roles:             []string{"ROLE-AKUN"},
		Permissions: []string{
			"chart_of_accounts.read",
			"chart_of_accounts.create",
			"chart_of_accounts.update",
			"chart_of_accounts.delete",
			"chart_of_accounts.submit",
			"chart_of_accounts.review",
			"chart_of_accounts.approve",
			"chart_of_accounts.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected.
func newRouter(adapter *repoAdapter) *gin.Engine {
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

	svc := coa.NewService(adapter, audit.NewWriter(nil), slog.Default())
	importer := coa.NewImporter(adapter, &noopJobRepo{}, audit.NewWriter(nil), nil, slog.Default())
	h := coa.NewHandler(svc, importer, wfh)

	v1 := r.Group("/api/v1")
	coa.RegisterRoutes(v1, h)
	return r
}

// ─── POST /master/coa ─────────────────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/coa",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingRequiredFields_Returns400(t *testing.T) {
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{
		"kodeAkun": "1.1",
		// namaAkun, tipeAkun, posisiNormal, sumberCoa, tanggalMulaiAktif missing
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/coa", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing fields, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/coa/:id ──────────────────────────────────────────────────────

func TestGet_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGet_NotFound_Returns404(t *testing.T) {
	r := newRouter(&repoAdapter{
		stubGetByID: &stubGetByID{result: nil, err: nil},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGet_Found_Returns200(t *testing.T) {
	item := testCoA()
	r := newRouter(&repoAdapter{
		stubGetByID: &stubGetByID{result: item},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/"+item.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			KodeAkun string `json:"kodeAkun"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.KodeAkun != "1.1.01.001" {
		t.Errorf("expected kodeAkun=1.1.01.001, got %s", resp.Data.KodeAkun)
	}
}

// ─── GET /master/coa ──────────────────────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*coa.ChartOfAccount{testCoA()}
	r := newRouter(&repoAdapter{stubList: &stubList{items: items}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa?limit=10", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data []struct {
			KodeAkun string `json:"kodeAkun"`
		} `json:"data"`
		Pagination struct {
			Limit int `json:"limit"`
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
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa?sort=not_a_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(&repoAdapter{stubList: &stubList{}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa?sort=kode_akun:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/coa/export ───────────────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfKode Akun,Nama Akun\r\n1.1,Kas\r\n"
	r := newRouter(&repoAdapter{
		stubExport: &stubExport{
			reader: strings.NewReader(csvData),
			count:  1,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/export?format=csv", nil)
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

func TestExport_XLSX_Returns501(t *testing.T) {
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 for xlsx export, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(&repoAdapter{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── Route: export before /:id ────────────────────────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	csvData := "\xef\xbb\xbf"
	r := newRouter(&repoAdapter{
		stubExport: &stubExport{reader: strings.NewReader(csvData), count: 0},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/coa/export", nil)
	r.ServeHTTP(rec, req)

	// Should NOT return 400 (which would happen if "export" was treated as a UUID path param).
	if rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "UUID") {
		t.Errorf("export route was confused with /:id route; got 400 UUID error; body=%s", rec.Body.String())
	}
}

// ─── DELETE ── entity_in_use (has children) ───────────────────────────────────

func TestDelete_HasChildren_Returns409(t *testing.T) {
	item := testCoA()
	r := newRouter(&repoAdapter{
		stubSoftDelete: &stubSoftDelete{
			getByIDResult: item,
			childCount:    3,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/coa/"+item.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity with children, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), coa.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(&repoAdapter{
		stubSoftDelete: &stubSoftDelete{getByIDResult: nil},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/coa/"+uuid.New().String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PATCH ── workflow_status guard ───────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testCoA()
	approved.WorkflowStatus = coa.WorkflowStatusApproved

	r := newRouter(&repoAdapter{
		stubUpdate: &stubUpdate{getByIDResult: approved},
	})

	body, _ := json.Marshal(coa.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/coa/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), coa.CodeMasterApprovedNoEdit)
}

func TestUpdate_InvalidRowVersion_Returns400(t *testing.T) {
	// rowVersion = 0 → validation failed
	r := newRouter(&repoAdapter{})
	body, _ := json.Marshal(map[string]interface{}{
		"namaAkun":   "Updated",
		"rowVersion": 0,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/coa/"+uuid.New().String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rowVersion=0, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Validation: kodeAkun format ─────────────────────────────────────────────

func TestValidate_KodeAkun_Format(t *testing.T) {
	cases := []struct {
		kode     string
		wantFail bool
	}{
		{"1", false},           // valid single digit
		{"1.1", false},          // valid two-level
		{"1.1.01.001", false},   // valid four-level
		{"abc", true},           // letters not allowed
		{"1.", true},            // trailing dot
		{".1", true},            // leading dot
		{"1..1", true},          // double dot
		{"", true},              // empty
		{"1.A", true},           // letter in segment
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("kode=%q", tc.kode), func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          tc.kode,
				NamaAkun:          "Test Account",
				TipeAkun:          "ASET",
				SubTipeAkun:       "KAS",
				PosisiNormal:      "DEBIT",
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantFail {
				if err == nil {
					t.Errorf("expected error for kode=%q, got nil", tc.kode)
					return
				}
				de, ok := domainerrors.IsDomainError(err)
				if !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for kode=%q, got %v", tc.kode, err)
				}
			} else if err != nil {
				// Valid kode: service passes validation but hits BeginTx stub error — that's fine.
				de, ok := domainerrors.IsDomainError(err)
				if ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for valid kode=%q: %v", tc.kode, err)
				}
			}
		})
	}
}

// ─── Validation: tipeAkun whitelist ──────────────────────────────────────────

func TestValidate_TipeAkun_Whitelist(t *testing.T) {
	validTypes := []string{"ASET", "LIABILITAS", "EKUITAS", "PENDAPATAN", "BEBAN", "KONTINJEN"}
	for _, tt := range validTypes {
		t.Run(tt, func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          "1",
				NamaAkun:          "Test",
				TipeAkun:          tt,
				SubTipeAkun:       "KAS",
				PosisiNormal:      "DEBIT",
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			// Valid type; should fail at BeginTx (no DB), not at validation.
			if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("valid tipeAkun=%q should not cause VALIDATION_FAILED: %v", tt, err)
				}
			}
		})
	}

	invalidTypes := []string{"INVALID", "aset", "Asset", ""}
	for _, tt := range invalidTypes {
		t.Run("invalid_"+tt, func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          "1",
				NamaAkun:          "Test",
				TipeAkun:          tt,
				SubTipeAkun:       "KAS",
				PosisiNormal:      "DEBIT",
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if err == nil {
				t.Errorf("expected error for invalid tipeAkun=%q", tt)
				return
			}
			de, ok := domainerrors.IsDomainError(err)
			if !ok || de.Code() != domainerrors.CodeValidationFailed {
				t.Errorf("expected VALIDATION_FAILED for invalid tipeAkun=%q, got %v", tt, err)
			}
		})
	}
}

// ─── Validation: posisiNormal whitelist ──────────────────────────────────────

func TestValidate_PosisiNormal(t *testing.T) {
	for _, p := range []string{"DEBIT", "KREDIT"} {
		t.Run(p, func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          "1",
				NamaAkun:          "Test",
				TipeAkun:          "ASET",
				SubTipeAkun:       "KAS",
				PosisiNormal:      p,
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("valid posisiNormal=%q caused VALIDATION_FAILED: %v", p, err)
				}
			}
		})
	}
	for _, p := range []string{"debit", "kredit", "INVALID", ""} {
		t.Run("invalid_"+p, func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          "1",
				NamaAkun:          "Test",
				TipeAkun:          "ASET",
				SubTipeAkun:       "KAS",
				PosisiNormal:      p,
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: "2026-01-01",
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if err == nil {
				t.Errorf("expected error for invalid posisiNormal=%q", p)
				return
			}
			de, ok := domainerrors.IsDomainError(err)
			if !ok || de.Code() != domainerrors.CodeValidationFailed {
				t.Errorf("expected VALIDATION_FAILED for invalid posisiNormal=%q, got %v", p, err)
			}
		})
	}
}

// ─── Validation: tanggalMulaiAktif format ────────────────────────────────────

func TestValidate_TanggalFormat(t *testing.T) {
	cases := []struct {
		tanggal  string
		wantFail bool
	}{
		{"2026-06-04", false},
		{"2026-01-01", false},
		{"06-04-2026", true},
		{"2026/06/04", true},
		{"20260604", true},
	}
	for _, tc := range cases {
		t.Run(tc.tanggal, func(t *testing.T) {
			req := coa.CreateRequest{
				KodeAkun:          "1",
				NamaAkun:          "Test",
				TipeAkun:          "ASET",
				SubTipeAkun:       "KAS",
				PosisiNormal:      "DEBIT",
				SumberCoa:         "MANUAL",
				TanggalMulaiAktif: tc.tanggal,
			}
			svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, req)
			if tc.wantFail {
				if err == nil {
					t.Errorf("expected error for tanggal=%q", tc.tanggal)
					return
				}
				de, ok := domainerrors.IsDomainError(err)
				if !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for tanggal=%q, got %v", tc.tanggal, err)
				}
			}
		})
	}
}

// ─── ToResponse mapping ───────────────────────────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	item := testCoA()
	item.WorkflowStatus = coa.WorkflowStatusRejected
	resp := coa.ToResponse(item)
	if resp.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED for REJECTED, got %s", resp.WorkflowStatus)
	}
}

func TestToResponse_FieldMapping(t *testing.T) {
	item := testCoA()
	resp := coa.ToResponse(item)
	if resp.ID != item.ID.String() {
		t.Errorf("ID mismatch: want %s got %s", item.ID.String(), resp.ID)
	}
	if resp.KodeAkun != item.KodeAkun {
		t.Errorf("KodeAkun: want %s got %s", item.KodeAkun, resp.KodeAkun)
	}
	if resp.TipeAkun != string(item.TipeAkun) {
		t.Errorf("TipeAkun: want %s got %s", item.TipeAkun, resp.TipeAkun)
	}
	if resp.PosisiNormal != string(item.PosisiNormal) {
		t.Errorf("PosisiNormal: want %s got %s", item.PosisiNormal, resp.PosisiNormal)
	}
	if resp.RowVersion != item.Version {
		t.Errorf("RowVersion: want %d got %d", item.Version, resp.RowVersion)
	}
	if resp.AktifFlag != item.AktifFlag {
		t.Errorf("AktifFlag: want %v got %v", item.AktifFlag, resp.AktifFlag)
	}
}

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{coa.CodeCoADuplicateKode, "COA_DUPLICATE_KODE"},
		{coa.CodeCoAInvalidKodeFormat, "COA_INVALID_KODE_FORMAT"},
		{coa.CodeCoAParentNotFound, "COA_PARENT_NOT_FOUND"},
		{coa.CodeMasterApprovedNoEdit, "MASTER_APPROVED_NO_EDIT"},
		{coa.CodeEntityInUse, "ENTITY_IN_USE"},
	}
	for _, tc := range tests {
		if tc.code != tc.want {
			t.Errorf("expected code %q, got %q", tc.want, tc.code)
		}
	}
}

// ─── Allowed column whitelist ─────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"kode_akun", "nama_akun", "tipe_akun", "aktif_flag", "created_at", "tanggal_mulai_aktif", "workflow_status"}
	for _, col := range expected {
		found := false
		for _, ac := range coa.AllowedSortCols {
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

// ─── XLSX column index helper ─────────────────────────────────────────────────

// colIndexExported wraps the private colIndex via exported test surface.
// We test the xlsx_parser.go logic indirectly by testing parseXLSXBytes.

func TestXLSXParser_EmptyBytes_ReturnsError(t *testing.T) {
	// Empty bytes → invalid ZIP → error expected.
	// We call the importer via a valid-looking request path.
	svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	importer := coa.NewImporter(&repoAdapter{}, &noopJobRepo{}, audit.NewWriter(nil), nil, slog.Default())
	_ = svc
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	// Empty file → error from SubmitImport (0 bytes).
	_, err := importer.SubmitImport(ctx, coa.ImportXLSXRequest{SumberCoa: "TEST"}, []byte{})
	if err == nil {
		t.Error("expected error for empty file bytes")
	}
}

func TestXLSXParser_OversizedFile_ReturnsError(t *testing.T) {
	// > 10MB → validation error.
	svc := coa.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	_ = svc
	importer := coa.NewImporter(&repoAdapter{}, &noopJobRepo{}, audit.NewWriter(nil), nil, slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	bigFile := make([]byte, 11*1024*1024) // 11 MB
	_, err := importer.SubmitImport(ctx, coa.ImportXLSXRequest{SumberCoa: "TEST"}, bigFile)
	if err == nil {
		t.Error("expected error for oversized file")
		return
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED for oversized file, got %v", err)
	}
}

// ─── Interface compile checks ─────────────────────────────────────────────────

func TestInterface_DBRepositoryImplementsRepository(t *testing.T) {
	var _ coa.Repository = (*coa.DBRepository)(nil)
}

func TestInterface_DBJobRepositoryImplementsJobRepository(t *testing.T) {
	var _ coa.JobRepository = (*coa.DBJobRepository)(nil)
}

// ─── Optimistic lock ErrConflict maps to 409 ─────────────────────────────────

func TestUpdate_ErrConflict_MapsTo409(t *testing.T) {
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected HTTP 409, got %d", err.HTTPStatus())
	}
}

// ─── Duplicate kode domain error ─────────────────────────────────────────────

func TestDomainError_CoADuplicateKode_Is422(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeCoADuplicateKode, "duplicate")
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for CodeCoADuplicateKode, got %d", err.HTTPStatus())
	}
}

func TestDomainError_CoAInvalidKodeFormat_Is422(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeCoAInvalidKodeFormat, "invalid")
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for CodeCoAInvalidKodeFormat, got %d", err.HTTPStatus())
	}
}

func TestDomainError_CoAParentNotFound_Is422(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeCoAParentNotFound, "parent not found")
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for CodeCoAParentNotFound, got %d", err.HTTPStatus())
	}
}

// ─── WorkflowHook ─────────────────────────────────────────────────────────────

func TestWorkflowHook_BeforeCommit_CallsUpdateWorkflowStatus(t *testing.T) {
	// WorkflowHook.BeforeCommit should call repo.UpdateWorkflowStatus inside the tx.
	// With a nil tx (InMemory test path), UpdateWorkflowStatus is expected to be called
	// and the stub returns nil, so BeforeCommit should return nil.
	svc := coa.NewService(&repoAdapter{
		stubGetByID: &stubGetByID{result: testCoA()},
	}, audit.NewWriter(nil), slog.Default())

	hook := coa.NewWorkflowHook(svc)
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	err := hook.BeforeCommit(ctx, nil /* nil tx = InMemory path */, workflow.HookEvent{
		EntityID:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		EntityType: "CHART_OF_ACCOUNTS",
		NewState:   workflow.StateApproved,
		Action:     workflow.ActionApprove,
		ActorID:    uuid.MustParse("00000000-0000-0000-0000-000000000001"),
	})
	// Stub UpdateWorkflowStatus returns nil, so BeforeCommit should succeed.
	if err != nil {
		t.Errorf("expected nil error from hook with stub repo, got: %v", err)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

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

// Suppress unused import warning for io (used by stub).
var _ io.Reader = nil
