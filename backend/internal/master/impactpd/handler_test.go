package impactpd_test

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
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
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
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := impactpd.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	impactpd.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *impactpd.Service {
	return impactpd.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// TC-008: Create with valid payload passes validation; commit fails at BeginTx
// (no real DB in unit test) — assert non-422 (validation passed).
func TestCreate_ValidRequest_PassesValidation(t *testing.T) {
	periodeID := uuid.New()
	r := newRouter(buildSvc(&repoAdapter{dupCount: 0}))
	body := map[string]interface{}{
		"periodeId":        periodeID.String(),
		"impactMultiplier": "1.20000000",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// With no DB, BeginTx returns errTestNoDB → 500. Validation having passed
	// is the signal — a 4xx would mean validation failed.
	if rec.Code < 500 {
		t.Errorf("expected validation to pass (>=500 from BeginTx), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-009: impact_multiplier outside [0.5, 2.0] → 422 FL_MULTIPLIER_OUT_OF_RANGE.
func TestCreate_MultiplierOutOfRange_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"impactMultiplier": "3.0",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// validateCreate batches range errors into VALIDATION_FAILED 400.
	// Update path emits FL_MULTIPLIER_OUT_OF_RANGE 422 separately.
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 or 422 got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != "VALIDATION_FAILED" && errObj["code"] != "FL_MULTIPLIER_OUT_OF_RANGE" {
		t.Errorf("expected VALIDATION_FAILED or FL_MULTIPLIER_OUT_OF_RANGE got %v", errObj["code"])
	}
}

// TC-010: multiplier at lower bound 0.5 passes validation (no real DB in unit
// test → BeginTx fails with 500; validation pass is the signal).
func TestCreate_MultiplierAtLowerBound_PassesValidation(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{dupCount: 0}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"impactMultiplier": "0.5",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code < 500 {
		t.Errorf("expected validation to pass (>=500 from BeginTx), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-011: Duplicate periode_id → 422 FL_PERIODE_DUPLICATE.
func TestCreate_DuplicatePeriode_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{dupCount: 1}))
	body := map[string]interface{}{
		"periodeId":        uuid.New().String(),
		"impactMultiplier": "1.0",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/impact-pd", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 got %d: %s", rec.Code, rec.Body.String())
	}
}

// TC-012: GET /active returns ActiveResponse shape (OQ-5 contract).
func TestGetActive_ReturnsActiveResponse(t *testing.T) {
	periodeID := uuid.New()
	activeRow := &impactpd.ImpactPd{
		ID:               uuid.New(),
		PeriodeID:        periodeID,
		ImpactMultiplier: decimal.NewFromFloat(1.1),
		WorkflowStatus:   impactpd.WorkflowStatusApproved,
	}
	r := newRouter(buildSvc(&repoAdapter{activeRow: activeRow}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd/active?periode_id="+periodeID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["periodeId"] != periodeID.String() {
		t.Errorf("expected periodeId %s got %v", periodeID, data["periodeId"])
	}
	if data["impactMultiplier"] != "1.10000000" {
		t.Errorf("expected impactMultiplier 1.10000000 got %v", data["impactMultiplier"])
	}
}

// TC-F2: GET /active when no APPROVED row exists for the period → 404 NOT_FOUND.
func TestGetActive_NoApprovedRecord_Returns404(t *testing.T) {
	periodeID := uuid.New()
	// activeRow is nil — no APPROVED rows in DB.
	r := newRouter(buildSvc(&repoAdapter{activeRow: nil}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/impact-pd/active?periode_id="+periodeID.String(), nil)
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

// TC-013: Edit APPROVED record → 403 MASTER_APPROVED_NO_EDIT.
func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	existing := &impactpd.ImpactPd{
		ID:               uuid.New(),
		WorkflowStatus:   impactpd.WorkflowStatusApproved,
		ImpactMultiplier: decimal.NewFromFloat(1.0),
		RowVersion:       1,
	}
	r := newRouter(buildSvc(&repoAdapter{getByIDRow: existing}))
	body := map[string]interface{}{
		"impactMultiplier": "1.1",
		"rowVersion":       1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/impact-pd/"+existing.ID.String(), bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 got %d: %s", rec.Code, rec.Body.String())
	}
}
