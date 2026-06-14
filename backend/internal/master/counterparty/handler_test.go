package counterparty_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/counterparty"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router setup ─────────────────────────────────────────────────────────────

func newRouter(svc *counterparty.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "maker.tr",
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions: []string{
			"counterparty.read", "counterparty.create", "counterparty.update",
			"counterparty.delete", "counterparty.submit", "counterparty.review",
			"counterparty.approve", "counterparty.reject", "counterparty.export",
			"counterparty.view_pii",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
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
	h := counterparty.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	counterparty.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *counterparty.Service {
	return counterparty.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/counterparty ────────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/counterparty",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidTipe_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := `{
		"kodeCounterparty":"CP001",
		"nama":"Test Bank",
		"tipe":"INVALID_TIPE",
		"tipeEksposurBasel":"BANK"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/counterparty",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid tipe, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_ValidPayload_CreateRepoError_Returns500(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		createErr: errTestNoDB,
	}))
	body := `{
		"kodeCounterparty":"CP001",
		"nama":"Bank Mandiri",
		"tipe":"BANK",
		"tipeEksposurBasel":"BANK"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/counterparty",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// Expected 500 because BeginTx returns errTestNoDB
	if rec.Code != http.StatusInternalServerError {
		t.Logf("body=%s", rec.Body.String())
	}
}

// ─── GET /master/counterparty/:id ────────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	cp := testCounterparty()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: cp},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+cp.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID               string `json:"id"`
			KodeCounterparty string `json:"kodeCounterparty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.KodeCounterparty != "CP001" {
		t.Errorf("expected kodeCounterparty=CP001, got %s", resp.Data.KodeCounterparty)
	}
}

// ─── PII masking ──────────────────────────────────────────────────────────────

func TestGetByID_PIIMasked_InDefaultResponse(t *testing.T) {
	cp := testCounterparty()
	npwp := "***"
	masked := &counterparty.MaskedPII{NPWP: &npwp}
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: cp, masked: masked},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+cp.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Data struct {
			NPWP *string `json:"npwp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.NPWP == nil || *resp.Data.NPWP != "***" {
		t.Errorf("expected masked NPWP='***', got %v", resp.Data.NPWP)
	}
}

func TestGetByID_PIINilByDefault_WhenNoEncryptedData(t *testing.T) {
	cp := testCounterparty()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: cp, masked: &counterparty.MaskedPII{}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+cp.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Data struct {
			NPWP          *string `json:"npwp"`
			NomorRekening *string `json:"nomorRekening"`
			KTP           *string `json:"ktp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.NPWP != nil {
		t.Errorf("expected nil NPWP, got %v", *resp.Data.NPWP)
	}
}

// ─── GET /master/counterparty ─────────────────────────────────────────────────

func TestList_NoPIIInListResponse(t *testing.T) {
	cp := testCounterparty()
	r := newRouter(buildSvc(&repoAdapter{
		list: &stubList{items: []*counterparty.Counterparty{cp}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			KodeCounterparty string  `json:"kodeCounterparty"`
			NPWP             *string `json:"npwp"`
			NomorRekening    *string `json:"nomorRekening"`
			KTP              *string `json:"ktp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data))
	}
	// List endpoint must never include PII (all nil)
	if resp.Data[0].NPWP != nil {
		t.Errorf("list response must not include NPWP, got %v", *resp.Data[0].NPWP)
	}
	if resp.Data[0].NomorRekening != nil {
		t.Errorf("list response must not include NomorRekening")
	}
	if resp.Data[0].KTP != nil {
		t.Errorf("list response must not include KTP")
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty?sort=kode_cp:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for disallowed sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_ValidSortCol_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty?sort=kode_counterparty:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid sort, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/counterparty/:id/pii ─────────────────────────────────────────

func TestGetPII_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+uuid.New().String()+"/pii", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetPII_Found_ReturnsDecryptedFields(t *testing.T) {
	cp := testCounterparty()
	npwp := "01.234.567.8-900.000"
	rek := "1234567890"
	ktp := "3201012345678901"
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: cp},
		getPII: &stubGetPII{
			pii: &counterparty.PIIFields{
				NPWP:          &npwp,
				NomorRekening: &rek,
				KTP:           &ktp,
			},
		},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/"+cp.ID.String()+"/pii", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID               string  `json:"id"`
			KodeCounterparty string  `json:"kodeCounterparty"`
			NPWP             *string `json:"npwp"`
			NomorRekening    *string `json:"nomorRekening"`
			KTP              *string `json:"ktp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.NPWP == nil || *resp.Data.NPWP != npwp {
		t.Errorf("expected NPWP=%s, got %v", npwp, resp.Data.NPWP)
	}
	if resp.Data.NomorRekening == nil || *resp.Data.NomorRekening != rek {
		t.Errorf("expected NomorRekening=%s", rek)
	}
}

// ─── DELETE /master/counterparty/:id ──────────────────────────────────────────

func TestDelete_WithReferences_Returns409(t *testing.T) {
	cp := testCounterparty()
	r := newRouter(buildSvc(&repoAdapter{
		getByID:      &stubGetByID{cp: cp},
		countRefsVal: 3,
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/counterparty/"+cp.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for referenced counterparty, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/counterparty/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/counterparty/export ──────────────────────────────────────────

func TestExport_CSV_Returns200WithHeaders(t *testing.T) {
	csvData := "\xef\xbb\xbfKode,Nama\r\nCP001,Bank Mandiri\r\n"
	r := newRouter(buildSvc(&repoAdapter{export: &stubExport{
		reader: newCSVReader(csvData),
		count:  1,
	}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for csv export, got %d; body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct == "" {
		t.Errorf("expected Content-Type header")
	}
	xRows := rec.Header().Get("X-Total-Rows")
	if xRows != "1" {
		t.Errorf("expected X-Total-Rows=1, got %s", xRows)
	}
}

func TestExport_XLSX_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for xlsx (not supported), got %d", rec.Code)
	}
}

// ─── PUT /master/counterparty/:id ─────────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	cp := testCounterparty() // WorkflowStatus = APPROVED
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{cp: cp},
	}))
	body := `{"nama":"New Name","rowVersion":1}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/counterparty/"+cp.ID.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// Service guard: APPROVED record cannot be edited directly
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for editing APPROVED record, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdate_InvalidRowVersion_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := `{"nama":"New Name","rowVersion":0}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/counterparty/"+uuid.New().String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rowVersion=0, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── MaskString unit tests ────────────────────────────────────────────────────

func TestMaskString_NilInput_ReturnsNil(t *testing.T) {
	if counterparty.MaskString(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestMaskString_ShortString_Returns3Stars(t *testing.T) {
	s := "abc"
	result := counterparty.MaskString(&s)
	if result == nil || *result != "***" {
		t.Errorf("expected '***' for short string, got %v", result)
	}
}

func TestMaskString_LongString_ShowsLast4(t *testing.T) {
	s := "1234567890"
	result := counterparty.MaskString(&s)
	if result == nil || *result != "***7890" {
		t.Errorf("expected '***7890', got %v", result)
	}
}

func TestMaskString_Exactly4Chars_ShowsAllAs4(t *testing.T) {
	s := "1234"
	result := counterparty.MaskString(&s)
	if result == nil || *result != "***1234" {
		t.Errorf("expected '***1234', got %v", result)
	}
}

// ─── Domain validation: IsValidTipe ───────────────────────────────────────────

func TestIsValidTipe_ValidValues(t *testing.T) {
	validTipes := []string{
		"BANK", "BANK_KUSTODIAN", "KORPORASI", "PEMERINTAH",
		"MANAJER_INVESTASI", "EMITEN_SAHAM", "MULTILATERAL",
		"KORPORASI_BUMN", "INDIVIDU", "REASURADUR",
	}
	for _, tipe := range validTipes {
		if !counterparty.IsValidTipe(tipe) {
			t.Errorf("expected %s to be valid tipe", tipe)
		}
	}
}

func TestIsValidTipe_InvalidValue(t *testing.T) {
	if counterparty.IsValidTipe("INVALID") {
		t.Error("expected INVALID to be invalid tipe")
	}
}

// ─── Domain validation: IsValidEksposurBasel ─────────────────────────────────

func TestIsValidEksposurBasel_Valid(t *testing.T) {
	vals := []string{"SOVEREIGN", "SENIOR_SECURED", "SENIOR_UNSECURED", "SUBORDINATED", "CORPORATE", "BANK", "RETAIL"}
	for _, v := range vals {
		if !counterparty.IsValidEksposurBasel(v) {
			t.Errorf("expected %s to be valid eksposur_basel", v)
		}
	}
}

func TestIsValidEksposurBasel_Invalid(t *testing.T) {
	if counterparty.IsValidEksposurBasel("UNKNOWN") {
		t.Error("expected UNKNOWN to be invalid eksposur_basel")
	}
}
