package impactmevpd_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "risk.officer.1",
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
		},
		TenantID:    "TUGURE",
		MFAVerified: true,
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
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := impactmevpd.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	impactmevpd.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *impactmevpd.Service {
	return impactmevpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// TC-001: Create GOOD skenario for a new periode — validation passes (BeginTx
// fails because tests have no real DB; the assertion is that validation
// succeeded so we hit 5xx, not a 4xx validation rejection).
func TestCreate_Good_PassesValidation(t *testing.T) {
	periodeID := uuid.New()
	r := newRouter(buildSvc(&repoAdapter{dupCount: 0}))
	body := map[string]interface{}{
		"periodeId":        periodeID.String(),
		"skenario":         "GOOD",
		"impactMultiplier": "1.05000000",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code < 500 {
		t.Errorf("expected validation to pass (>=500 from BeginTx), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-002: Create duplicate (periode_id, skenario) that's already active → 422 FL_PERIODE_DUPLICATE.
func TestCreate_Duplicate_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{dupCount: 1}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"skenario":         "GOOD",
		"impactMultiplier": "1.10000000",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "FL_PERIODE_DUPLICATE" {
		t.Errorf("expected FL_PERIODE_DUPLICATE got %v", errObj["code"])
	}
}

// TC-003: Skenario NORMAL rejected with VALIDATION_FAILED (OQ-1: no NORMAL rows).
func TestCreate_NormalSkenario_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"skenario":         "NORMAL",
		"impactMultiplier": "1.00000000",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-004: impact_multiplier = 0 (not positive) → VALIDATION_FAILED (OQ-3: must be > 0).
func TestCreate_ZeroMultiplier_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"skenario":         "BAD",
		"impactMultiplier": "0",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-mev-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-005: GET /active returns shape compatible with ECL engine Phase 4 (TC-014).
func TestGetActive_ReturnsMultipliersMap(t *testing.T) {
	periodeID := uuid.New()
	goodRow := &impactmevpd.ImpactMevPd{
		ID:               uuid.New(),
		PeriodeID:        periodeID,
		Skenario:         impactmevpd.SkenarioGood,
		ImpactMultiplier: decimal.NewFromFloat(1.05),
		WorkflowStatus:   impactmevpd.WorkflowStatusApproved,
	}
	badRow := &impactmevpd.ImpactMevPd{
		ID:               uuid.New(),
		PeriodeID:        periodeID,
		Skenario:         impactmevpd.SkenaroBad,
		ImpactMultiplier: decimal.NewFromFloat(0.95),
		WorkflowStatus:   impactmevpd.WorkflowStatusApproved,
	}
	r := newRouter(buildSvc(&repoAdapter{activeRows: []*impactmevpd.ImpactMevPd{goodRow, badRow}}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd/active?periode_id="+periodeID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	mults := data["multipliers"].(map[string]interface{})
	if mults["GOOD"] != "1.05000000" {
		t.Errorf("expected GOOD=1.05000000 got %v", mults["GOOD"])
	}
	if mults["BAD"] != "0.95000000" {
		t.Errorf("expected BAD=0.95000000 got %v", mults["BAD"])
	}
}

// TC-006: Edit APPROVED record → 403 MASTER_APPROVED_NO_EDIT.
func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	existing := &impactmevpd.ImpactMevPd{
		ID:               uuid.New(),
		WorkflowStatus:   impactmevpd.WorkflowStatusApproved,
		ImpactMultiplier: decimal.NewFromFloat(1.05),
		Skenario:         impactmevpd.SkenarioGood,
		PeriodeID:        uuid.New(),
		RowVersion:       1,
	}
	r := newRouter(buildSvc(&repoAdapter{getByIDRow: existing}))
	body := map[string]interface{}{
		"impactMultiplier": "1.10000000",
		"rowVersion":       1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/impact-mev-pd/"+existing.ID.String(), bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-007: GET /active without periode_id → 400 VALIDATION_FAILED.
func TestGetActive_NoPeriodeID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd/active", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-008 (F2): GET /active when no APPROVED row exists for the period → 404 NOT_FOUND.
func TestGetActive_NoApprovedRecord_Returns404(t *testing.T) {
	periodeID := uuid.New()
	// activeRows is nil / empty — no APPROVED rows in DB.
	r := newRouter(buildSvc(&repoAdapter{activeRows: nil}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-mev-pd/active?periode_id="+periodeID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND got %v", errObj["code"])
	}
}
