// Package eir — HTTP handler tests for all 11 EIR endpoints.
//
// Uses httptest + Gin test mode with mock services backed by stub repos.
// Covers: permission checks, JSON parsing, domain error mapping, 202 acceptance.
// DEC-016: no float64 in any assertion.
package eir

import (
	"bytes"
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

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Handler test helpers ──────────────────────────────────────────────────────

// buildRouter sets up a Gin router with the EIR handler, bypassing JWT middleware.
// JWT claims are injected directly into Gin context via middleware.
// When mfaVerified=true a full *auth.Claims with a fresh StepupVerifiedAt (now-1min)
// is injected so ApproveAmendment's NeedsStepUp() check (DEC-027) passes.
func buildRouter(h *Handler, perms []string, mfaVerified bool) *gin.Engine {
	r := gin.New()
	// Inject auth claims into context (simulates JWT middleware)
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uuid.New().String())
		c.Set("role", "ROLE-RISK")
		c.Set("permissions", perms)
		c.Set("mfa_verified", mfaVerified)
		// Inject full *auth.Claims so claimsFromGin + NeedsStepUp() works (DEC-027).
		cl := makeClaims(perms, mfaVerified)
		c.Set("claims", cl)
		c.Next()
	})
	v1 := r.Group("/api/v1")
	eirGroup := v1.Group("/ecl/eir")
	eirGroup.POST("/compute", h.ComputeEIR)
	eirGroup.POST("/generate-schedule", h.GenerateSchedule)
	eirGroup.GET("/schedule/:instrumenId", h.GetActiveSchedule)
	eirGroup.GET("/schedule/:instrumenId/history", h.GetScheduleHistory)
	eirGroup.POST("/amendments", h.ProposeAmendment)
	eirGroup.GET("/amendments", h.ListAmendments)
	eirGroup.GET("/amendments/:id", h.GetAmendment)
	eirGroup.POST("/amendments/:id/review", h.ReviewAmendment)
	eirGroup.POST("/amendments/:id/approve", h.ApproveAmendment)
	eirGroup.POST("/amendments/:id/reject", h.RejectAmendment)
	eirGroup.POST("/bulk-recompute", h.BulkRecompute)
	return r
}

// allPerms returns all EIR permissions.
func allPerms() []string {
	return []string{
		PermEIRCompute, PermEIRPreview,
		PermEIRAmendPropose, PermEIRAmendReview, PermEIRAmendApprove,
		PermEIRBulkRecompute,
	}
}

// makeClaims builds an *auth.Claims for test injection.
// When mfaVerified=true, StepupVerifiedAt is set to 1 minute ago (fresh window).
// When mfaVerified=false, StepupVerifiedAt is nil (NeedsStepUp() == true).
func makeClaims(perms []string, mfaVerified bool) *auth.Claims {
	cl := &auth.Claims{
		Sub:         uuid.New().String(),
		MFAVerified: mfaVerified,
		Permissions: perms,
		Roles:       []string{"ROLE-RISK"},
		TenantID:    "TUGURE",
	}
	if mfaVerified {
		// Fresh step-up: 1 minute ago — well within the 5-minute DEC-027 window.
		ts := time.Now().Add(-1 * time.Minute).Unix()
		cl.StepupVerifiedAt = &ts
	}
	return cl
}

// buildRouterWithClaims is like buildRouter but takes a pre-built *auth.Claims.
// Use this for precise step-up timestamp control in DEC-027 tests.
func buildRouterWithClaims(h *Handler, cl *auth.Claims) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", cl.Sub)
		c.Set("role", "ROLE-RISK")
		c.Set("permissions", cl.Permissions)
		c.Set("mfa_verified", cl.MFAVerified)
		c.Set("claims", cl)
		c.Next()
	})
	v1 := r.Group("/api/v1")
	eirGroup := v1.Group("/ecl/eir")
	eirGroup.POST("/amendments/:id/approve", h.ApproveAmendment)
	return r
}

// doRequest executes an HTTP request against the test router.
func doRequest(r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req, _ = http.NewRequest(method, path, bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// buildHandler builds a Handler with stub services backed by the stub repos.
func buildHandler(instrRepo *stubInstrumenRepo, schedRepo *stubScheduleRepo, amendRepo *stubAmendmentRepo, db interface{}) *Handler {
	auditW := &stubAuditWriter{}

	eirSvc := &Service{
		instrRepo:   instrRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	schedSvc := &ScheduleService{
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	amendSvc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	bulkSvc := NewBulkService(nil, instrRepo, schedRepo, nil, nil, testLogger())

	return NewHandler(eirSvc, schedSvc, amendSvc, bulkSvc)
}

// cashflowJSONItems returns a slice of cashflow JSON items.
func cashflowJSONItems() []map[string]string {
	cfs := obligasiAtDiscount2()
	items := make([]map[string]string, len(cfs))
	for i, cf := range cfs {
		items[i] = map[string]string{
			"date":      cf.Date.Format("2006-01-02"),
			"amountIdr": cf.AmountIDR.StringFixed(4),
		}
	}
	return items
}

// ─── ComputeEIR tests ─────────────────────────────────────────────────────────

func TestHandler_ComputeEIR_NoPermission_403(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	h := buildHandler(instrRepo, &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, []string{}, false) // no permissions

	w := doRequest(r, "POST", "/api/v1/ecl/eir/compute", map[string]interface{}{
		"instrumenId":        uuid.New().String(),
		"cashflowProjection": cashflowJSONItems(),
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ComputeEIR_InvalidBody_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	// Missing required cashflowProjection
	w := doRequest(r, "POST", "/api/v1/ecl/eir/compute", map[string]interface{}{
		"instrumenId": uuid.New().String(),
		// no cashflowProjection
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ComputeEIR_InstrumenNotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil) // empty repo
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/compute", map[string]interface{}{
		"instrumenId":        uuid.New().String(),
		"cashflowProjection": cashflowJSONItems(),
	})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ComputeEIR_FVTPL_422(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "FVTPL", nil))

	h := buildHandler(instrRepo, &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/compute", map[string]interface{}{
		"instrumenId":        id.String(),
		"cashflowProjection": cashflowJSONItems(),
	})

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ComputeEIR_Preview_200(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "AC", nil))

	h := buildHandler(instrRepo, &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/compute", map[string]interface{}{
		"instrumenId":        id.String(),
		"cashflowProjection": cashflowJSONItems(),
		"persistResult":      false,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["eirPerPeriod"] == nil || data["eirPerPeriod"] == "" {
		t.Error("eirPerPeriod should be set in response")
	}
	t.Logf("Response EIR: %v", data["eirPerPeriod"])
}

// ─── GenerateSchedule tests ───────────────────────────────────────────────────

func TestHandler_GenerateSchedule_EIRNotComputed_422(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	instrRepo.put(actInstrumen(id, "AC", nil)) // no eir_awal

	h := buildHandler(instrRepo, &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/generate-schedule", map[string]interface{}{
		"instrumenId":        id.String(),
		"cashflowProjection": cashflowJSONItems(),
	})

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GenerateSchedule_DuplicateGuard_409(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	h := buildHandler(instrRepo, &stubScheduleRepo{hasActive: true}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/generate-schedule", map[string]interface{}{
		"instrumenId":        id.String(),
		"cashflowProjection": cashflowJSONItems(),
	})

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GetActiveSchedule / GetScheduleHistory tests ─────────────────────────────

func TestHandler_GetActiveSchedule_InvalidUUID_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/schedule/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetActiveSchedule_EmptyList_200(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/schedule/"+uuid.New().String(), nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetScheduleHistory_EmptyList_200(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/schedule/"+uuid.New().String()+"/history", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── Amendment endpoint tests ─────────────────────────────────────────────────

func TestHandler_ProposeAmendment_NoPermission_403(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, []string{PermEIRCompute}, false) // missing amend.propose

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments", map[string]interface{}{
		"instrumenId":               uuid.New().String(),
		"tanggalAmandemen":          "2026-06-01",
		"revisedCashflowProjection": cashflowJSONItems(),
		"alasanAmandemen":           "modification reason here",
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ProposeAmendment_InstrumenNotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments", map[string]interface{}{
		"instrumenId":               uuid.New().String(),
		"tanggalAmandemen":          "2026-06-01",
		"revisedCashflowProjection": cashflowJSONItems(),
		"alasanAmandemen":           "modification reason here",
	})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ProposeAmendment_BadDate_400(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	id := uuid.New()
	eirVal := mustDec("0.08")
	instrRepo.put(actInstrumen(id, "AC", &eirVal))

	h := buildHandler(instrRepo, &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments", map[string]interface{}{
		"instrumenId":               id.String(),
		"tanggalAmandemen":          "not-a-date", // invalid
		"revisedCashflowProjection": cashflowJSONItems(),
		"alasanAmandemen":           "modification reason here",
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ListAmendments_200(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetAmendment_NotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/"+uuid.New().String(), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetAmendment_InvalidUUID_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetAmendment_Found_200(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	cfJSON, _ := marshalCashflows(obligasiAtDiscount2())
	p := makeProposal(instrID, makerID, AmendStatusPendingReview, cfJSON)
	amendRepo.proposals[p.ID] = &p

	h := buildHandler(instrRepo, &stubScheduleRepo{}, amendRepo, nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/"+p.ID.String(), nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ReviewAmendment_NotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/review", map[string]interface{}{
		"comment": "ok to proceed",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ReviewAmendment_InvalidUUID_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/not-a-uuid/review", map[string]interface{}{
		"comment": "ok to proceed",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ApproveAmendment_MFANotVerified_403(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false) // mfa_verified = false

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/approve", map[string]interface{}{
		"comment":     "approved by alco",
		"stepUpToken": "token",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing MFA, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ApproveAmendment_MFAVerified_NotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), true) // mfa_verified = true

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/approve", map[string]interface{}{
		"comment":     "approved by alco",
		"stepUpToken": "token",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_RejectAmendment_InvalidUUID_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/not-a-uuid/reject", map[string]interface{}{
		"comment": "reject this",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_RejectAmendment_NotFound_404(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/reject", map[string]interface{}{
		"comment": "rejected because insufficient docs",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── BulkRecompute tests ──────────────────────────────────────────────────────

func TestHandler_BulkRecompute_NoPermission_403(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, []string{PermEIRPreview}, false) // no bulk_recompute perm

	w := doRequest(r, "POST", "/api/v1/ecl/eir/bulk-recompute", map[string]interface{}{
		"scope":  "ALL_ACTIVE",
		"reason": "periodic validation",
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BulkRecompute_InvalidScope_400(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/bulk-recompute", map[string]interface{}{
		"scope":  "INVALID_SCOPE",
		"reason": "testing",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BulkRecompute_AllActive_202(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)
	r := buildRouter(h, allPerms(), false)

	w := doRequest(r, "POST", "/api/v1/ecl/eir/bulk-recompute", map[string]interface{}{
		"scope":  "ALL_ACTIVE",
		"reason": "periodic validation check",
	})
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["jobId"] == nil || data["jobId"] == "" {
		t.Error("jobId missing in 202 response")
	}
	if data["statusUrl"] == nil {
		t.Error("statusUrl missing in 202 response")
	}
}

// ─── scheduleRowToJSON / proposalToJSON tests ─────────────────────────────────

func TestScheduleRowToJSON_NoFloat64(t *testing.T) {
	row := ScheduleRow{
		ID:                 uuid.New(),
		InstrumenID:        uuid.New(),
		PeriodeSeq:         1,
		TanggalPosting:     date(2026, 7, 1),
		OpeningCarrying:    mustDec("1005000000.0000"),
		CashInflow:         mustDec("40000000.0000"),
		PendapatanBungaEIR: mustDec("40200000.0000"),
		AmortisasiPD:       mustDec("200000.0000"),
		PelunasanPokok:     decimal.Zero,
		ClosingCarrying:    mustDec("1005200000.0000"),
		EIRPeriode:         mustDec("0.04000000"),
		StageSaatPosting:   "STAGE_1",
		StatusPosting:      "PROYEKSI",
		FlagPOCI:           false,
		CreatedAt:          time.Now(),
		TenantID:           "TUGURE",
	}
	m := scheduleRowToJSON(row)

	// Verify all rate/amount fields are strings (no float64 in JSON)
	if _, ok := m["openingCarrying"].(string); !ok {
		t.Error("openingCarrying should be string, not float64")
	}
	if _, ok := m["eirPeriode"].(string); !ok {
		t.Error("eirPeriode should be string, not float64")
	}
	if _, ok := m["cashInflow"].(string); !ok {
		t.Error("cashInflow should be string, not float64")
	}
}

func TestProposalToJSON_OptionalFields(t *testing.T) {
	p := AmendmentProposal{
		ID:               uuid.New(),
		InstrumenID:      uuid.New(),
		Status:           AmendStatusPendingReview,
		TanggalAmandemen: date(2026, 6, 1),
		AlasanAmandemen:  "test",
		TenantID:         "TUGURE",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	m := proposalToJSON(p)

	if m["id"] == nil {
		t.Error("id must be in response")
	}
	// EIRLama and EIRBaru are nil → should not be in map
	if _, ok := m["eirLama"]; ok {
		t.Error("eirLama should not appear when nil")
	}
	if _, ok := m["eirBaru"]; ok {
		t.Error("eirBaru should not appear when nil")
	}
}

func TestParseCashflowItems_InvalidDate_Error(t *testing.T) {
	items := []cashflowItemJSON2{
		{Date: "not-a-date", AmountIdr: "-1000000"},
	}
	_, err := parseCashflowItems(items)
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestParseCashflowItems_InvalidAmount_Error(t *testing.T) {
	items := []cashflowItemJSON2{
		{Date: "2026-01-01", AmountIdr: "not-a-number"},
	}
	_, err := parseCashflowItems(items)
	if err == nil {
		t.Fatal("expected error for invalid amount")
	}
}

func TestParseCashflowItems_Valid(t *testing.T) {
	items := []cashflowItemJSON2{
		{Date: "2026-01-01", AmountIdr: "-1005000000"},
		{Date: "2026-07-01", AmountIdr: "40000000"},
	}
	cfs, err := parseCashflowItems(items)
	if err != nil {
		t.Fatalf("parseCashflowItems: %v", err)
	}
	if len(cfs) != 2 {
		t.Errorf("expected 2 items, got %d", len(cfs))
	}
	if !cfs[0].AmountIDR.Equal(mustDec("-1005000000")) {
		t.Errorf("CF[0] amount mismatch: %s", cfs[0].AmountIDR)
	}
}

func TestParseInt_Valid(t *testing.T) {
	n, ok := parseInt("50")
	if !ok || n != 50 {
		t.Errorf("parseInt(\"50\") = %d, %v", n, ok)
	}
	_, ok = parseInt("abc")
	if ok {
		t.Error("parseInt(\"abc\") should fail")
	}
	_, ok = parseInt("")
	if ok {
		t.Error("parseInt(\"\") should fail")
	}
}

// ─── handleDomainError / hasPermission / hasMFAVerified ──────────────────────

func TestHandleDomainError_DomainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	handleDomainError(c, ErrEIRInstrumenNotFound("test-id"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDomainError_GenericError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	handleDomainError(c, fmt.Errorf("some internal error"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHasPermission_SliceInterface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	// []interface{} format (simulates JWT middleware)
	c.Set("permissions", []interface{}{PermEIRCompute, PermEIRPreview})

	if !hasPermission(c, PermEIRCompute) {
		t.Error("should have eir.compute permission")
	}
}

func TestHasPermission_Missing_ReturnsFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Set("permissions", []string{PermEIRPreview})

	if hasPermission(c, PermEIRCompute) {
		t.Error("should not have eir.compute permission")
	}
}

func TestHasMFAVerified_True(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Set("mfa_verified", true)

	if !hasMFAVerified(c) {
		t.Error("should be MFA verified")
	}
}

func TestHasMFAVerified_False(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Set("mfa_verified", false)

	if hasMFAVerified(c) {
		t.Error("should NOT be MFA verified")
	}
}

// ─── NewHandler / NewAmendmentService panic tests ─────────────────────────────

func TestNewAmendmentService_NilAuditWriter_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter")
		}
	}()
	NewAmendmentService(nil, newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil, testLogger())
}

func TestNewService_NilAuditWriter_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter")
		}
	}()
	NewService(nil, newStubInstrumenRepo(), nil, testLogger())
}

func TestNewScheduleService_NilAuditWriter_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter")
		}
	}()
	NewScheduleService(nil, newStubInstrumenRepo(), &stubScheduleRepo{}, nil, testLogger())
}

// ─── submitBulkRecomputeJob + NewDomainError ─────────────────────────────────

func TestSubmitBulkRecomputeJob_ReturnsPayload(t *testing.T) {
	payload, err := submitBulkRecomputeJob("job-1", BulkScopeAllActive, uuid.New())
	if err != nil {
		t.Fatalf("submitBulkRecomputeJob: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
}

func TestNewDomainError_WrapsCode(t *testing.T) {
	err := domainerrors.NewDomainError(domainerrors.CodeEIRNonConvergent, "test message")
	if err.Code() != domainerrors.CodeEIRNonConvergent {
		t.Errorf("code mismatch: %s", err.Code())
	}
}

// ─── Repo cursor helpers ──────────────────────────────────────────────────────

func TestEncodeCursorStr_RoundTrip(t *testing.T) {
	id := uuid.New()
	encoded := encodeCursorStr(id.String())
	decoded, err := decodeCursorStr(encoded)
	if err != nil {
		t.Fatalf("decodeCursorStr: %v", err)
	}
	if decoded != id.String() {
		t.Errorf("cursor round-trip: want %s, got %s", id.String(), decoded)
	}
}

func TestDecodeCursorStr_InvalidBase64_ReturnsError(t *testing.T) {
	_, err := decodeCursorStr("not-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 cursor")
	}
}

// ─── computeCatchUpAdjustment ─────────────────────────────────────────────────

func TestComputeCatchUpAdjustment_PositiveDiff(t *testing.T) {
	oldEIR := mustDec("0.08")
	newEIR := mustDec("0.09")
	carrying := mustDec("1000000000")

	adjustment := computeCatchUpAdjustment(oldEIR, newEIR, carrying)
	// carrying × (newEIR - oldEIR) = 1_000_000_000 × 0.01 = 10_000_000
	expected := mustDec("10000000")

	if !adjustment.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.StringFixed(4), adjustment.StringFixed(4))
	}
}

// ─── Step-up MFA window tests (DEC-027, F1 compliance fix) ───────────────────

// TestApproveAmendment_StepUpStale_Returns403_StepUpRequired verifies that
// ApproveAmendment rejects a JWT where stepup_verified_at is 10 minutes ago
// with 403 STEP_UP_REQUIRED. DEC-027: window is 5 minutes.
// A 4-hour-old JWT with mfa_verified=true but stale step-up must be rejected.
func TestApproveAmendment_StepUpStale_Returns403_StepUpRequired(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)

	// Claims: mfa_verified=true but step-up was 10 minutes ago — outside 5-min window.
	staleTs := time.Now().Add(-10 * time.Minute).Unix()
	cl := &auth.Claims{
		Sub:              uuid.New().String(),
		MFAVerified:      true, // JWT claim says verified
		StepupVerifiedAt: &staleTs,
		Permissions:      allPerms(),
		Roles:            []string{"ROLE-ALCO"},
		TenantID:         "TUGURE",
	}

	r := buildRouterWithClaims(h, cl)
	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/approve", map[string]interface{}{
		"comment":     "attempt approve with stale step-up",
		"stepUpToken": "stale-token",
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for stale step-up, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, string(domainerrors.CodeStepUpRequired)) {
		t.Errorf("expected error code %s in body, got: %s", domainerrors.CodeStepUpRequired, body)
	}
}

// TestApproveAmendment_StepUpFresh_Allows verifies that ApproveAmendment proceeds
// past the MFA gate when stepup_verified_at is within the 5-minute window (DEC-027).
// The call reaches the service layer and returns 404 (no matching proposal) —
// proving the step-up check passed.
func TestApproveAmendment_StepUpFresh_Allows(t *testing.T) {
	h := buildHandler(newStubInstrumenRepo(), &stubScheduleRepo{}, newStubAmendmentRepo(), nil)

	// Claims: step-up was 1 minute ago — fresh within 5-min window.
	freshTs := time.Now().Add(-1 * time.Minute).Unix()
	cl := &auth.Claims{
		Sub:              uuid.New().String(),
		MFAVerified:      true,
		StepupVerifiedAt: &freshTs,
		Permissions:      allPerms(),
		Roles:            []string{"ROLE-ALCO"},
		TenantID:         "TUGURE",
	}

	r := buildRouterWithClaims(h, cl)
	// Use a random UUID — proposal won't exist, so service returns 404.
	// A 404 proves the MFA gate was passed (otherwise we'd get 403).
	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/"+uuid.New().String()+"/approve", map[string]interface{}{
		"comment":     "approve with fresh step-up",
		"stepUpToken": "fresh-token",
	})

	if w.Code == http.StatusForbidden {
		t.Errorf("fresh step-up must NOT be rejected: got 403 %s", w.Body.String())
	}
	// Service returns 404 (proposal not found) — expected.
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (proposal not found), got %d: %s", w.Code, w.Body.String())
	}
}
