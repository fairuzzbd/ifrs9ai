package periodebuku_test

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
	"blips-ifrs9.tugu-re.com/internal/master/periodebuku"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// testPeriodeBuku returns a sample domain entity.
func testPeriodeBuku() *periodebuku.PeriodeBuku {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	bulan := 6
	now := time.Now()
	return &periodebuku.PeriodeBuku{
		ID:             uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		PeriodeIDKode:  "2026-M06",
		TipePeriode:    periodebuku.TipePeriodeBulanan,
		TahunBuku:      2026,
		Bulan:          &bulan,
		Triwulan:       nil,
		TanggalMulai:   "2026-06-01",
		TanggalAkhir:   "2026-06-30",
		StatusPeriode:  periodebuku.StatusPeriodeOpen,
		WorkflowStatus: periodebuku.WorkflowStatusApproved,
		CreatedAt:      now,
		CreatedBy:      &createdBy,
		RowVersion:     1,
		TenantID:       "TUGURE",
	}
}

// testClaims returns JWT claims for a regular user with all periode permissions.
func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "akun.maker",
		Roles:             []string{"ROLE-AKUN"},
		Permissions: []string{
			"periode.read",
			"periode.create",
			"periode.update",
			"periode.delete",
			"periode.submit",
			"periode.review",
			"periode.approve",
			"periode.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected.
func newRouter(svc *periodebuku.Service) *gin.Engine {
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
	h := periodebuku.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	periodebuku.RegisterRoutes(v1, h)
	return r
}

// buildSvc creates a Service backed by a repoAdapter.
func buildSvc(adapter *repoAdapter) *periodebuku.Service {
	return periodebuku.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/periode-buku ── binding ─────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/periode-buku",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/periode-buku/:id ── UUID validation ─────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/not-a-uuid", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	p := testPeriodeBuku()
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: p},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/"+p.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			PeriodeIDKode string `json:"periodeIdKode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.PeriodeIDKode != "2026-M06" {
		t.Errorf("expected periodeIdKode=2026-M06, got %s", resp.Data.PeriodeIDKode)
	}
}

// ─── GET /master/periode-buku ── list ─────────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*periodebuku.PeriodeBuku{testPeriodeBuku()}
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{result: items}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku?limit=10", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			PeriodeIDKode string `json:"periodeIdKode"`
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
	if resp.Data[0].PeriodeIDKode != "2026-M06" {
		t.Errorf("expected 2026-M06, got %s", resp.Data[0].PeriodeIDKode)
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{result: nil}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku?sort=tahun_buku:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/periode-buku/export ──────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfKode Periode,Tipe Periode\r\n2026-M06,BULANAN\r\n"
	r := newRouter(buildSvc(&repoAdapter{exportStub: &stubExport{
		reader: strings.NewReader(csvData),
		count:  1,
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/export?format=csv", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── DELETE ── entity_in_use ──────────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	p := testPeriodeBuku()
	r := newRouter(buildSvc(&repoAdapter{
		deleteStub: &stubSoftDelete{
			getByIDResult: p,
			refCount:      3,
		},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/periode-buku/"+p.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), periodebuku.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		deleteStub: &stubSoftDelete{getByIDResult: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/periode-buku/"+uuid.New().String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PATCH ── workflow_status guard ───────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testPeriodeBuku()
	approved.WorkflowStatus = periodebuku.WorkflowStatusApproved

	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getByIDResult: approved},
	}))

	body, _ := json.Marshal(periodebuku.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/periode-buku/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), periodebuku.CodeMasterApprovedNoEdit)
}

// ─── Service: optimistic lock conflict mapping ─────────────────────────────────

func TestUpdate_ErrConflict_MapsToConflict(t *testing.T) {
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected 409 HTTP status, got %d", err.HTTPStatus())
	}
}

// ─── Validation: periodeIDKode format ─────────────────────────────────────────

func TestValidate_PeriodeIDKode(t *testing.T) {
	cases := []struct {
		kode           string
		wantValidation bool
	}{
		{"2026-M01", false}, // valid BULANAN
		{"2026-M12", false}, // valid BULANAN
		{"2026-Q1", false},  // valid TRIWULANAN
		{"2026-Q4", false},  // valid TRIWULANAN
		{"2026-Y", false},   // valid TAHUNAN
		{"26-M06", true},    // 2-digit year
		{"2026-M6", true},   // single-digit month
		{"2026-Q5", true},   // Q5 invalid
		{"2026-W01", true},  // unknown prefix W
		{"", true},          // empty
		{"2026M06", true},   // missing dash
	}

	for _, tc := range cases {
		t.Run(tc.kode, func(t *testing.T) {
			bulan := 6
			req := periodebuku.CreateRequest{
				PeriodeIDKode: tc.kode,
				TipePeriode:   periodebuku.TipePeriodeBulanan,
				TahunBuku:     2026,
				Bulan:         &bulan,
				TanggalMulai:  "2026-06-01",
				TanggalAkhir:  "2026-06-30",
			}
			svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
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
				// Non-domain error (BeginTx from stub) is expected for valid-kode paths.
			}
		})
	}
}

// ─── Validation: date format ──────────────────────────────────────────────────

func TestValidate_TanggalFormat(t *testing.T) {
	cases := []struct {
		tanggal        string
		wantValidation bool
	}{
		{"2026-06-01", false},
		{"06-01-2026", true}, // wrong format
		{"2026/06/01", true}, // wrong separator
		{"20260601", true},   // no separators
	}
	for _, tc := range cases {
		t.Run(tc.tanggal, func(t *testing.T) {
			bulan := 6
			req := periodebuku.CreateRequest{
				PeriodeIDKode: "2026-M06",
				TipePeriode:   periodebuku.TipePeriodeBulanan,
				TahunBuku:     2026,
				Bulan:         &bulan,
				TanggalMulai:  tc.tanggal,
				TanggalAkhir:  "2026-06-30",
			}
			svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
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

// ─── Validation: bulan required for BULANAN ───────────────────────────────────

func TestValidate_BulanRequiredForBulanan(t *testing.T) {
	req := periodebuku.CreateRequest{
		PeriodeIDKode: "2026-M06",
		TipePeriode:   periodebuku.TipePeriodeBulanan,
		TahunBuku:     2026,
		Bulan:         nil, // missing — should fail
		TanggalMulai:  "2026-06-01",
		TanggalAkhir:  "2026-06-30",
	}
	svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Error("expected error for missing bulan on BULANAN, got nil")
		return
	}
	if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %v", err)
	}
}

// ─── Validation: triwulan required for TRIWULANAN ────────────────────────────

func TestValidate_TriwulanRequiredForTriwulanan(t *testing.T) {
	req := periodebuku.CreateRequest{
		PeriodeIDKode: "2026-Q2",
		TipePeriode:   periodebuku.TipePeriodeTriwulanan,
		TahunBuku:     2026,
		Triwulan:      nil, // missing — should fail
		TanggalMulai:  "2026-04-01",
		TanggalAkhir:  "2026-06-30",
	}
	svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, req)
	if err == nil {
		t.Error("expected error for missing triwulan on TRIWULANAN, got nil")
		return
	}
	if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %v", err)
	}
}

// ─── ToResponse: REJECTED maps to RETURNED ────────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	p := testPeriodeBuku()
	p.WorkflowStatus = periodebuku.WorkflowStatusRejected
	r := periodebuku.ToResponse(p)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected workflowStatus=RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_FieldMapping(t *testing.T) {
	p := testPeriodeBuku()
	r := periodebuku.ToResponse(p)
	if r.ID != p.ID.String() {
		t.Errorf("ID mismatch: want %s got %s", p.ID.String(), r.ID)
	}
	if r.PeriodeIDKode != p.PeriodeIDKode {
		t.Errorf("PeriodeIDKode mismatch: want %s got %s", p.PeriodeIDKode, r.PeriodeIDKode)
	}
	if r.TipePeriode != string(p.TipePeriode) {
		t.Errorf("TipePeriode mismatch: want %s got %s", p.TipePeriode, r.TipePeriode)
	}
	if r.TahunBuku != p.TahunBuku {
		t.Errorf("TahunBuku mismatch: want %d got %d", p.TahunBuku, r.TahunBuku)
	}
	if r.RowVersion != p.RowVersion {
		t.Errorf("RowVersion: want %d got %d", p.RowVersion, r.RowVersion)
	}
}

// ─── Generate: calendar bounds ────────────────────────────────────────────────

// TestGenerate_CalendarBounds verifies month and quarter boundary calculation.
// We exercise the service directly (bypassing BeginTx) by checking service
// validation paths; full DB integration is done in integration tests.
func TestGenerate_CalendarBounds_Validation(t *testing.T) {
	cases := []struct {
		tahun          int
		wantValidation bool
	}{
		{2026, false},
		{1999, true}, // before min year 2000
		{2101, true}, // after max year 2100
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("year=%d", tc.tahun), func(t *testing.T) {
			req := periodebuku.GenerateRequest{TahunBuku: tc.tahun}
			svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Generate(ctx, req)
			if tc.wantValidation {
				if err == nil {
					t.Errorf("expected error for year=%d, got nil", tc.tahun)
					return
				}
				if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
					t.Errorf("expected VALIDATION_FAILED for year=%d, got %v", tc.tahun, err)
				}
			} else if err != nil {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
					t.Errorf("unexpected VALIDATION_FAILED for year=%d: %v", tc.tahun, err)
				}
				// BeginTx error from stub is expected — ok.
			}
		})
	}
}

// TestGenerate_InvalidTipe verifies unknown tipe values are rejected.
func TestGenerate_InvalidTipe_Returns400(t *testing.T) {
	req := periodebuku.GenerateRequest{
		TahunBuku: 2026,
		Tipe:      []periodebuku.TipePeriode{"KUARTALAN"}, // invalid
	}
	svc := periodebuku.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Generate(ctx, req)
	if err == nil {
		t.Error("expected error for invalid tipe, got nil")
		return
	}
	if de, ok := domainerrors.IsDomainError(err); !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %v", err)
	}
}

// ─── Generate HTTP endpoint ───────────────────────────────────────────────────

func TestGenerate_HTTP_Returns201(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		// allowTx=true: BeginTx returns (nil, nil) so service proceeds past tx open.
		// Audit write is skipped when tx==nil (no-DB test mode).
		allowTx:    true,
		bulkCreate: &stubBulkCreate{created: 17, skipped: 0},
	}))

	body := `{"tahunBuku": 2026}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/periode-buku/generate",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Generated int `json:"generated"`
			Skipped   int `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if resp.Data.Generated != 17 {
		t.Errorf("expected generated=17, got %d", resp.Data.Generated)
	}
	if resp.Data.Skipped != 0 {
		t.Errorf("expected skipped=0, got %d", resp.Data.Skipped)
	}
}

// ─── Route registration: static paths before /:id ────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	// "export" is not a valid UUID. If routing is correct, export handler is called.
	// If registered after /:id, "export" would be treated as :id param → 400 uuid invalid.
	csvData := "\xef\xbb\xbfKode Periode\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		exportStub: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/periode-buku/export", nil)
	r.ServeHTTP(rec, req)

	// Should be 200 CSV, NOT 400 "UUID tidak valid".
	if rec.Code == http.StatusBadRequest {
		t.Errorf("export route was confused with /:id route; got 400; body=%s", rec.Body.String())
	}
}

func TestGenerateRoute_NotConfusedWithID(t *testing.T) {
	// "generate" is not a valid UUID. If routing is correct, generate handler is called.
	r := newRouter(buildSvc(&repoAdapter{
		bulkCreate: &stubBulkCreate{created: 0, skipped: 0},
	}))

	body := `{"tahunBuku": 2026}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/periode-buku/generate",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	// Should NOT be a 400 "UUID tidak valid" (which would happen if "generate" were parsed as :id).
	if rec.Code == http.StatusBadRequest {
		var errResp struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp.Error.Code == string(domainerrors.CodeValidationFailed) {
			t.Logf("body: %s", rec.Body.String())
			if strings.Contains(rec.Body.String(), "UUID") {
				t.Errorf("generate route confused with /:id route; got UUID validation error")
			}
		}
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

// ─── AllowedCols whitelist ─────────────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{
		"periode_id_kode", "tahun_buku", "bulan", "tipe_periode",
		"status_periode", "workflow_status", "created_at",
	}
	for _, col := range expected {
		found := false
		for _, ac := range periodebuku.AllowedSortCols {
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

// ─── Interface compliance ──────────────────────────────────────────────────────

func TestReusePattern_VerifyInterfaces(t *testing.T) {
	// DBRepository fully implements Repository at compile time.
	var _ periodebuku.Repository = (*periodebuku.DBRepository)(nil)
}

// ─── Error code constants ──────────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if periodebuku.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", periodebuku.CodeEntityInUse)
	}
	if periodebuku.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", periodebuku.CodeMasterApprovedNoEdit)
	}
}

// ─── Helper assertion ─────────────────────────────────────────────────────────

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
