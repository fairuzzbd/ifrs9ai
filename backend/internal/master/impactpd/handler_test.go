package impactpd_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testEntity() *impactpd.ImpactPD {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	periodeID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	return &impactpd.ImpactPD{
		ID:               uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		PeriodeID:        periodeID,
		ImpactMultiplier: decimal.NewFromFloat(1.1),
		WorkflowStatus:   impactpd.WorkflowStatusDraft,
		CreatedAt:        time.Now(),
		CreatedBy:        &actorID,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}
}

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

func newRouter(svc *impactpd.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})

	wfSvc := workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	)
	wfh := workflow.NewHandler(wfSvc)
	h := impactpd.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	impactpd.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *impactpd.Service {
	return impactpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestList_Empty_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_WithItems_Returns200(t *testing.T) {
	items := []*impactpd.ImpactPD{testEntity()}
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{items: items}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd?limit=10", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Create ───────────────────────────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString("{bad json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_MissingRequired_Returns400(t *testing.T) {
	// Missing impactMultiplier
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidPeriodeID_Returns422(t *testing.T) {
	body := `{"periodeId":"not-a-uuid","impactMultiplier":"1.1"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MultiplierTooLow_Returns422(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"0.3"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for multiplier < 0.5, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MultiplierTooHigh_Returns422(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"2.5"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for multiplier > 2.0, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MultiplierExactlyMin_Passes(t *testing.T) {
	// 0.5 is the minimum allowed (inclusive)
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"0.5"}`
	r := newRouter(buildSvc(&repoAdapter{countByPeriode: 0}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// Should pass validation, fail at BeginTx (test stub = 500) — NOT 422
	if rec.Code == http.StatusUnprocessableEntity {
		t.Errorf("should not return 422 for multiplier=0.5; got body=%s", rec.Body.String())
	}
}

func TestCreate_MultiplierExactlyMax_Passes(t *testing.T) {
	// 2.0 is the maximum allowed (inclusive)
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"2.0"}`
	r := newRouter(buildSvc(&repoAdapter{countByPeriode: 0}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnprocessableEntity {
		t.Errorf("should not return 422 for multiplier=2.0; got body=%s", rec.Body.String())
	}
}

func TestCreate_DuplicatePeriode_Returns422(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"1.1"}`
	r := newRouter(buildSvc(&repoAdapter{countByPeriode: 1}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for duplicate periode, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MultiplierInvalidDecimal_Returns422(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"abc"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/00000000-0000-0000-0000-000000000099", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	m := testEntity()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: m},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/"+m.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID               string `json:"id"`
			ImpactMultiplier string `json:"impactMultiplier"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != m.ID.String() {
		t.Errorf("expected id=%s, got %s", m.ID.String(), resp.Data.ID)
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func TestUpdate_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/impact-pd/bad-id",
		bytes.NewBufferString(`{"rowVersion":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/master/impact-pd/00000000-0000-0000-0000-000000000099",
		bytes.NewBufferString(`{"rowVersion":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdate_ApprovedEntity_Returns403(t *testing.T) {
	m := testEntity()
	m.WorkflowStatus = impactpd.WorkflowStatusApproved
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: m},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/master/impact-pd/"+m.ID.String(),
		bytes.NewBufferString(`{"rowVersion":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for approved entity, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdate_MultiplierOutOfRange_Returns422(t *testing.T) {
	m := testEntity()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: m},
	}))
	// impact_multiplier = 3.5 → out of range [0.5, 2.0]
	body := `{"impactMultiplier":"3.5","rowVersion":1}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/master/impact-pd/"+m.ID.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for multiplier out of range, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/impact-pd/not-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/master/impact-pd/00000000-0000-0000-0000-000000000099", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Export ───────────────────────────────────────────────────────────────────

func TestExport_XLSX_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422 for xlsx, got %d", rec.Code)
	}
}

func TestExport_CSV_Returns200(t *testing.T) {
	csvData := bytes.NewBufferString("ID,Periode ID,Impact Multiplier\n")
	r := newRouter(buildSvc(&repoAdapter{
		export: &stubExport{reader: csvData, count: 1},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for CSV export, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── History ──────────────────────────────────────────────────────────────────

func TestHistory_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/not-uuid/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHistory_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/00000000-0000-0000-0000-000000000099/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHistory_Found_Returns200(t *testing.T) {
	m := testEntity()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: m},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-pd/"+m.ID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Domain: validateMultiplier via service ───────────────────────────────────

func TestValidateMultiplier_Boundary(t *testing.T) {
	tests := []struct {
		name      string
		val       string
		wantError bool
	}{
		{"min boundary 0.5", "0.5", false},
		{"max boundary 2.0", "2.0", false},
		{"below min 0.49", "0.49", true},
		{"above max 2.01", "2.01", true},
		{"middle 1.0", "1.0", false},
		{"middle 1.5", "1.5", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{
				"periodeId":        "00000000-0000-0000-0000-000000000010",
				"impactMultiplier": tt.val,
			})
			r := newRouter(buildSvc(&repoAdapter{countByPeriode: 0}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd",
				bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)
			isValidationFail := rec.Code == http.StatusUnprocessableEntity || rec.Code == http.StatusBadRequest
			if tt.wantError && !isValidationFail {
				t.Errorf("expected 400/422 for %s, got %d; body=%s", tt.val, rec.Code, rec.Body.String())
			}
			if !tt.wantError && isValidationFail {
				t.Errorf("expected pass for %s, got %d; body=%s", tt.val, rec.Code, rec.Body.String())
			}
		})
	}
}
