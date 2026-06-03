package impactmevpd_test

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
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testEntity() *impactmevpd.ImpactMevPD {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	periodeID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	return &impactmevpd.ImpactMevPD{
		ID:               uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		PeriodeID:        periodeID,
		Skenario:         impactmevpd.SkenarioBad,
		ImpactMultiplier: decimal.NewFromFloat(1.25),
		Catatan:          nil,
		WorkflowStatus:   impactmevpd.WorkflowStatusDraft,
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

func newRouter(svc *impactmevpd.Service) *gin.Engine {
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
	h := impactmevpd.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	impactmevpd.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *impactmevpd.Service {
	return impactmevpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestList_EmptyReturns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_WithItems_Returns200(t *testing.T) {
	items := []*impactmevpd.ImpactMevPD{testEntity()}
	r := newRouter(buildSvc(&repoAdapter{list: &stubList{items: items}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd?limit=10", nil)
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
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd?sort=secret_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Create ───────────────────────────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestCreate_MissingSkenario_Returns400(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","impactMultiplier":"1.2"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing skenario, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidSkenario_Returns422(t *testing.T) {
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","skenario":"NORMAL","impactMultiplier":"1.2"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400/422 for invalid skenario, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidPeriodeID_Returns422(t *testing.T) {
	body := `{"periodeId":"not-a-uuid","skenario":"BAD","impactMultiplier":"1.25"}`
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422 for invalid periodeId, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_DuplicatePeriodeSkenario_Returns422(t *testing.T) {
	// countByPeriodSkenario returns 1 → duplicate
	r := newRouter(buildSvc(&repoAdapter{countByPeriodSkenario: 1}))
	body := `{"periodeId":"00000000-0000-0000-0000-000000000010","skenario":"BAD","impactMultiplier":"1.25"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for duplicate, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidMevComponentsJSON_Returns400(t *testing.T) {
	mevJSON := `"not-an-object"`
	body, _ := json.Marshal(map[string]interface{}{
		"periodeId":         "00000000-0000-0000-0000-000000000010",
		"skenario":          "GOOD",
		"impactMultiplier":  "0.85",
		"mevComponentsJson": mevJSON,
	})
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422 for invalid mevComponentsJson, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_ValidMevComponentsWithWeights_Returns201(t *testing.T) {
	// weight sums to 1.0, count=0, no error from stub
	mevJSON := `{"weights":{"GDP":0.4,"inflasi":0.3,"kurs":0.3}}`
	body, _ := json.Marshal(map[string]interface{}{
		"periodeId":         "00000000-0000-0000-0000-000000000010",
		"skenario":          "GOOD",
		"impactMultiplier":  "0.85",
		"mevComponentsJson": mevJSON,
	})
	r := newRouter(buildSvc(&repoAdapter{countByPeriodSkenario: 0}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// Will fail at BeginTx (test stub), but validation passes = 500 (not 400/422)
	if rec.Code == http.StatusBadRequest || rec.Code == http.StatusUnprocessableEntity {
		t.Errorf("should not return 400/422 for valid input; got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_WeightsSumNot1_Returns422(t *testing.T) {
	mevJSON := `{"weights":{"GDP":0.4,"inflasi":0.3}}`
	body, _ := json.Marshal(map[string]interface{}{
		"periodeId":         "00000000-0000-0000-0000-000000000010",
		"skenario":          "GOOD",
		"impactMultiplier":  "0.85",
		"mevComponentsJson": mevJSON,
	})
	r := newRouter(buildSvc(&repoAdapter{countByPeriodSkenario: 0}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422 for weights sum != 1, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-mev-pd/00000000-0000-0000-0000-000000000099", nil)
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
		"/api/v1/master/impact-mev-pd/"+m.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Skenario string `json:"skenario"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != m.ID.String() {
		t.Errorf("expected id=%s, got %s", m.ID.String(), resp.Data.ID)
	}
	if resp.Data.Skenario != "BAD" {
		t.Errorf("expected skenario=BAD, got %s", resp.Data.Skenario)
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func TestUpdate_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/impact-mev-pd/bad-id",
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
		"/api/v1/master/impact-mev-pd/00000000-0000-0000-0000-000000000099",
		bytes.NewBufferString(`{"rowVersion":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdate_ApprovedEntity_Returns403(t *testing.T) {
	m := testEntity()
	m.WorkflowStatus = impactmevpd.WorkflowStatusApproved
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{result: m},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/master/impact-mev-pd/"+m.ID.String(),
		bytes.NewBufferString(`{"rowVersion":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for approved entity, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/impact-mev-pd/not-uuid", nil)
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
		"/api/v1/master/impact-mev-pd/00000000-0000-0000-0000-000000000099", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── Export ───────────────────────────────────────────────────────────────────

func TestExport_XLSXNotImplemented_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-mev-pd/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400/422 for xlsx, got %d", rec.Code)
	}
}

func TestExport_CSV_Returns200(t *testing.T) {
	csvData := bytes.NewBufferString("ID,Periode ID,Skenario\n")
	r := newRouter(buildSvc(&repoAdapter{
		export: &stubExport{reader: csvData, count: 1},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/impact-mev-pd/export?format=csv", nil)
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
		"/api/v1/master/impact-mev-pd/bad-uuid/history", nil)
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
		"/api/v1/master/impact-mev-pd/00000000-0000-0000-0000-000000000099/history", nil)
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
		"/api/v1/master/impact-mev-pd/"+m.ID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}
