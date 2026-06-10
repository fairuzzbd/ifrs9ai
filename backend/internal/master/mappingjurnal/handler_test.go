package mappingjurnal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/master/mappingjurnal"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test fixtures ────────────────────────────────────────────────────────────

var (
	testHeaderID = uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	testActorID  = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	testCoAID    = uuid.MustParse("cccccccc-0000-0000-0000-000000000001")
)

func testHeader() *mappingjurnal.Header {
	actorID := testActorID
	now := time.Now()
	return &mappingjurnal.Header{
		ID:                   testHeaderID,
		EventIDKode:          "EVT_PENEM_DEP_001",
		EventCode:            "PENEMPATAN_DEPOSITO",
		NamaEvent:            "Penempatan Deposito Baru",
		KategoriEvent:        "PENEMPATAN",
		TriggerSource:        "SYSTEM",
		TipeInstrumenBerlaku: []string{"DEPOSITO"},
		KlasifikasiBerlaku:   []string{"AC"},
		AktifFlag:            true,
		WorkflowStatus:       mappingjurnal.WorkflowStatusDraft,
		CreatedAt:            now,
		CreatedBy:            &actorID,
		RowVersion:           1,
		TenantID:             "TUGURE",
	}
}

func testDetailPair() []*mappingjurnal.Detail {
	now := time.Now()
	actorID := testActorID
	return []*mappingjurnal.Detail{
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              1,
			KodeAkunID:          testCoAID,
			DKIndicator:         "DEBIT",
			SumberAmount:        "NOMINAL_PENEMPATAN",
			Multiplier:          decimal.NewFromFloat(1.0),
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              2,
			KodeAkunID:          testCoAID,
			DKIndicator:         "KREDIT",
			SumberAmount:        "KAS_OPERASIONAL", //nolint:misspell
			Multiplier:          decimal.NewFromFloat(1.0),
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
	}
}

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               testActorID.String(),
		PreferredUsername: "akun.maker",
		Roles:             []string{"ROLE-AKUN"},
		Permissions: []string{
			"mapping_jurnal.read",
			"mapping_jurnal.create",
			"mapping_jurnal.update",
			"mapping_jurnal.delete",
			"mapping_jurnal.submit",
			"mapping_jurnal.review",
			"mapping_jurnal.approve",
			"mapping_jurnal.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected and routes registered.
func newRouter(svc *mappingjurnal.Service) *gin.Engine {
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
	h := mappingjurnal.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	mappingjurnal.RegisterRoutes(v1, h)
	return r
}

func buildSvc(adapter *repoAdapter) *mappingjurnal.Service {
	return mappingjurnal.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// buildHookEvent constructs a workflow.HookEvent for invariant tests.
func buildHookEvent(entityID uuid.UUID, newState, action string) workflow.HookEvent {
	return workflow.HookEvent{
		EntityType: "MAPPING_JURNAL",
		EntityID:   entityID,
		Action:     workflow.Action(action),
		NewState:   workflow.State(newState),
		OldState:   workflow.StatePendingApproval,
		ActorID:    testActorID,
	}
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func mustJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func decodeError(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body.Bytes(), &e); err != nil {
		return ""
	}
	return e.Error.Code
}

// ─── POST /master/mapping-jurnal ─────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingRequiredFields_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	// Missing eventCode and namaEvent
	body := mustJSON(t, map[string]interface{}{
		"eventIdKode":   "EVT001",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []interface{}{
			map[string]interface{}{
				"urutan": 1, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "DEBIT", "sumberAmount": "NOMINAL",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
			map[string]interface{}{
				"urutan": 2, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "KREDIT", "sumberAmount": "KAS",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing fields, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidEventCode_Returns400(t *testing.T) {
	// event_code with lowercase letters should fail ^[A-Z0-9_]+$ validation
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{
		"eventIdKode":   "EVT001",
		"eventCode":     "lowercase_not_allowed",
		"namaEvent":     "Test Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []interface{}{
			map[string]interface{}{
				"urutan": 1, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "DEBIT", "sumberAmount": "NOMINAL",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
			map[string]interface{}{
				"urutan": 2, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "KREDIT", "sumberAmount": "KAS",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for lowercase eventCode, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if code := decodeError(t, rec.Body); code != string(domainerrors.CodeValidationFailed) {
		t.Errorf("expected VALIDATION_FAILED, got %s", code)
	}
}

func TestCreate_OnlyOneDetail_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{
		"eventIdKode":   "EVT001",
		"eventCode":     "EVT_001",
		"namaEvent":     "Test Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []interface{}{
			map[string]interface{}{
				"urutan": 1, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "DEBIT", "sumberAmount": "NOMINAL",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for only 1 detail, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_NegativeMultiplier_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{
		"eventIdKode":   "EVT001",
		"eventCode":     "EVT_001",
		"namaEvent":     "Test Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []interface{}{
			map[string]interface{}{
				"urutan": 1, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "DEBIT", "sumberAmount": "NOMINAL",
				"multiplier": "-1.0000", "mataUangPosting": "IDR",
			},
			map[string]interface{}{
				"urutan": 2, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "KREDIT", "sumberAmount": "KAS",
				"multiplier": "-1.0000", "mataUangPosting": "IDR",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative multiplier, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_DBError_ReturnsError(t *testing.T) {
	// createHeader returns errTestNoDB when BeginTx is called — service will fail at BeginTx
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{
		"eventIdKode":   "EVT001",
		"eventCode":     "EVT_001",
		"namaEvent":     "Test Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []interface{}{
			map[string]interface{}{
				"urutan": 1, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "DEBIT", "sumberAmount": "NOMINAL",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
			map[string]interface{}{
				"urutan": 2, "kodeAkunId": testCoAID.String(),
				"dkIndicator": "KREDIT", "sumberAmount": "KAS",
				"multiplier": "1.0000", "mataUangPosting": "IDR",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	// BeginTx returns errTestNoDB → service propagates as 500
	if rec.Code == http.StatusCreated {
		t.Errorf("expected non-201, got 201 when DB unavailable")
	}
}

// ─── GET /master/mapping-jurnal/:id ──────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not-found, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200WithDetails(t *testing.T) {
	h := testHeader()
	details := testDetailPair()
	r := newRouter(buildSvc(&repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: details},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID      string `json:"id"`
			Details []struct {
				DKIndicator string `json:"dkIndicator"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != testHeaderID.String() {
		t.Errorf("expected id=%s, got %s", testHeaderID, resp.Data.ID)
	}
	if len(resp.Data.Details) != 2 {
		t.Errorf("expected 2 details, got %d", len(resp.Data.Details))
	}
}

// ─── GET /master/mapping-jurnal ── list ───────────────────────────────────────

func TestList_Returns200WithItems(t *testing.T) {
	h := testHeader()
	r := newRouter(buildSvc(&repoAdapter{
		listHeaders: &stubListHeaders{items: []*mappingjurnal.Header{h}},
		getDetails:  &stubGetDetails{details: []*mappingjurnal.Detail{}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal?limit=10", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			EventCode string `json:"eventCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].EventCode != "PENEMPATAN_DEPOSITO" {
		t.Errorf("expected eventCode=PENEMPATAN_DEPOSITO, got %s", resp.Data[0].EventCode)
	}
}

func TestList_EmptyResult_Returns200(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		listHeaders: &stubListHeaders{items: []*mappingjurnal.Header{}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for empty list, got %d", rec.Code)
	}
}

// ─── PATCH /master/mapping-jurnal/:id ────────────────────────────────────────

func TestUpdate_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{"rowVersion": 1})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/mapping-jurnal/not-a-uuid", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusApproved
	r := newRouter(buildSvc(&repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: testDetailPair()},
	}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{"rowVersion": 1})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record, got %d; body=%s", rec.Code, rec.Body.String())
	}
	code := decodeError(t, rec.Body)
	if code != string(domainerrors.CodeMasterApprovedNoEdit) {
		t.Errorf("expected MASTER_APPROVED_NO_EDIT, got %s", code)
	}
}

func TestUpdate_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{"rowVersion": 1})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdate_MissingRowVersion_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{"namaEvent": "Updated Event"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing rowVersion, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── DELETE /master/mapping-jurnal/:id ───────────────────────────────────────

func TestDelete_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mapping-jurnal/bad", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDelete_WithActiveReferences_Returns409(t *testing.T) {
	h := testHeader()
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: h},
		countRefs: &stubCountRefs{count: 3},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/mapping-jurnal/"+testHeaderID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for referenced entity, got %d; body=%s", rec.Code, rec.Body.String())
	}
	code := decodeError(t, rec.Body)
	if code != string(domainerrors.CodeEntityInUse) {
		t.Errorf("expected ENTITY_IN_USE, got %s", code)
	}
}

// ─── Service unit tests — debit/credit invariant ───────────────────────────────

// buildHook creates a WorkflowHook wired to a service with the given repo stub.
func buildHook(repo *repoAdapter) *mappingjurnal.WorkflowHook {
	svc := buildSvc(repo)
	return mappingjurnal.NewWorkflowHook(svc, repo)
}

// TestDebitCreditMismatch_BlocksApprove verifies the invariant is checked on approve.
func TestDebitCreditMismatch_BlocksApprove(t *testing.T) {
	// Details with DEBIT=1.0 and KREDIT=1.5 → mismatch
	now := time.Now()
	actorID := testActorID
	mismatchedDetails := []*mappingjurnal.Detail{
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              1,
			KodeAkunID:          testCoAID,
			DKIndicator:         "DEBIT",
			SumberAmount:        "NOMINAL",
			Multiplier:          decimal.NewFromFloat(1.0),
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              2,
			KodeAkunID:          testCoAID,
			DKIndicator:         "KREDIT",
			SumberAmount:        "KAS",
			Multiplier:          decimal.NewFromFloat(1.5), // mismatch
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
	}

	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusPendingApproval

	repo := &repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: mismatchedDetails},
		checkCoA:   &stubCheckCoA{approved: true},
	}
	hook := buildHook(repo)

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	evt := buildHookEvent(testHeaderID, "APPROVED", "APPROVE")

	err := hook.BeforeCommit(ctx, nil, evt)
	if err == nil {
		t.Fatal("expected error for debit/credit mismatch, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got: %v", err)
	}
	if de.Code() != domainerrors.CodeMappingJurnalDebitCreditMismatch {
		t.Errorf("expected MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH, got %s", de.Code())
	}
}

// TestDebitCreditBalance_PassesApprove verifies balanced details pass approve.
func TestDebitCreditBalance_PassesApprove(t *testing.T) {
	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusPendingApproval
	details := testDetailPair() // 1.0 DEBIT + 1.0 KREDIT = balanced

	repo := &repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: details},
		checkCoA:   &stubCheckCoA{approved: true},
	}
	hook := buildHook(repo)

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	evt := buildHookEvent(testHeaderID, "APPROVED", "APPROVE")

	// BeforeCommit with stub repo: UpdateWorkflowStatus returns nil → no error expected.
	if err := hook.BeforeCommit(ctx, nil, evt); err != nil {
		t.Errorf("balanced details should not produce error, got: %v", err)
	}
}

// TestCoANotApproved_BlocksApprove verifies CoA check blocks approve.
func TestCoANotApproved_BlocksApprove(t *testing.T) {
	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusPendingApproval
	details := testDetailPair()

	repo := &repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: details},
		checkCoA:   &stubCheckCoA{approved: false}, // CoA not approved
	}
	hook := buildHook(repo)

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	evt := buildHookEvent(testHeaderID, "APPROVED", "APPROVE")

	err := hook.BeforeCommit(ctx, nil, evt)
	if err == nil {
		t.Fatal("expected error for unapproved CoA, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got: %v", err)
	}
	if de.Code() != domainerrors.CodeMappingJurnalKodeAkunNotApproved {
		t.Errorf("expected MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED, got %s", de.Code())
	}
}

// TestNonApproveTransition_SkipsInvariant verifies non-approve transitions don't check invariant.
func TestNonApproveTransition_SkipsInvariant(t *testing.T) {
	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusDraft

	// Mismatched details — but we're doing SUBMIT not APPROVE, so invariant not checked
	now := time.Now()
	actorID := testActorID
	badDetails := []*mappingjurnal.Detail{
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              1,
			KodeAkunID:          testCoAID,
			DKIndicator:         "DEBIT",
			SumberAmount:        "NOMINAL",
			Multiplier:          decimal.NewFromFloat(2.0),
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
		{
			ID:                  uuid.New(),
			EventHeaderID:       testHeaderID,
			Urutan:              2,
			KodeAkunID:          testCoAID,
			DKIndicator:         "KREDIT",
			SumberAmount:        "KAS",
			Multiplier:          decimal.NewFromFloat(3.0), // deliberately mismatched
			MataUangPosting:     "IDR",
			AktifFlag:           true,
			CreatedAt:           now,
			CreatedBy:           &actorID,
			RowVersion:          1,
			TenantID:            "TUGURE",
			TipeInstrumenFilter: []string{},
		},
	}

	repo := &repoAdapter{
		getHeader:  &stubGetHeader{header: h},
		getDetails: &stubGetDetails{details: badDetails},
	}
	hook := buildHook(repo)

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	evt := buildHookEvent(testHeaderID, "PENDING_REVIEW", "SUBMIT")

	// SUBMIT should not trigger debit/credit invariant check.
	err := hook.BeforeCommit(ctx, nil, evt)
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok {
			if de.Code() == domainerrors.CodeMappingJurnalDebitCreditMismatch {
				t.Error("SUBMIT should not trigger debit/credit invariant check")
			}
		}
		// Any other error is unexpected here too
		t.Errorf("unexpected error on SUBMIT: %v", err)
	}
}

// ─── Service unit tests — validation ─────────────────────────────────────────

func TestCreate_ServiceValidation_EventCodeRegex(t *testing.T) {
	svc := buildSvc(&repoAdapter{})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	_, err := svc.Create(ctx, mappingjurnal.CreateRequest{
		EventIDKode:   "EVT001",
		EventCode:     "has space", // invalid
		NamaEvent:     "Test",
		KategoriEvent: "PENEMPATAN",
		TriggerSource: "SYSTEM",
		Details: []mappingjurnal.DetailRequest{
			{Urutan: 1, KodeAkunID: testCoAID.String(), DKIndicator: "DEBIT", SumberAmount: "NOMINAL", Multiplier: "1.0", MataUangPosting: "IDR"},
			{Urutan: 2, KodeAkunID: testCoAID.String(), DKIndicator: "KREDIT", SumberAmount: "KAS", Multiplier: "1.0", MataUangPosting: "IDR"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid event_code")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got: %v", err)
	}
}

func TestCreate_ServiceValidation_NamaEventTooShort(t *testing.T) {
	svc := buildSvc(&repoAdapter{})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	_, err := svc.Create(ctx, mappingjurnal.CreateRequest{
		EventIDKode:   "EVT001",
		EventCode:     "EVT_001",
		NamaEvent:     "AB", // too short (< 3)
		KategoriEvent: "PENEMPATAN",
		TriggerSource: "SYSTEM",
		Details: []mappingjurnal.DetailRequest{
			{Urutan: 1, KodeAkunID: testCoAID.String(), DKIndicator: "DEBIT", SumberAmount: "NOMINAL", Multiplier: "1.0", MataUangPosting: "IDR"},
			{Urutan: 2, KodeAkunID: testCoAID.String(), DKIndicator: "KREDIT", SumberAmount: "KAS", Multiplier: "1.0", MataUangPosting: "IDR"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for namaEvent too short")
	}
}

func TestCreate_ServiceValidation_InsufficientDetails(t *testing.T) {
	svc := buildSvc(&repoAdapter{})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	_, err := svc.Create(ctx, mappingjurnal.CreateRequest{
		EventIDKode:   "EVT001",
		EventCode:     "EVT_001",
		NamaEvent:     "Test Event",
		KategoriEvent: "PENEMPATAN",
		TriggerSource: "SYSTEM",
		Details: []mappingjurnal.DetailRequest{
			{Urutan: 1, KodeAkunID: testCoAID.String(), DKIndicator: "DEBIT", SumberAmount: "NOMINAL", Multiplier: "1.0", MataUangPosting: "IDR"},
		}, // only 1 detail
	})
	if err == nil {
		t.Fatal("expected validation error for insufficient details")
	}
	de, _ := domainerrors.IsDomainError(err)
	if de == nil || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got: %v", err)
	}
}

func TestUpdate_ServiceValidation_RowVersionRequired(t *testing.T) {
	svc := buildSvc(&repoAdapter{})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	_, err := svc.Update(ctx, testHeaderID, mappingjurnal.UpdateRequest{
		RowVersion: 0, // invalid
	})
	if err == nil {
		t.Fatal("expected validation error for rowVersion=0")
	}
	de, _ := domainerrors.IsDomainError(err)
	if de == nil || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got: %v", err)
	}
}

// ─── Domain conversion tests ──────────────────────────────────────────────────

func TestToHeaderResponse_MultiplicandDecimalPrecision(t *testing.T) {
	h := testHeader()
	d1 := testDetailPair()[0]
	d1.Multiplier = decimal.NewFromFloat(1.5)
	hwd := &mappingjurnal.HeaderWithDetails{Header: h, Details: []*mappingjurnal.Detail{d1}}
	r := mappingjurnal.ToHeaderResponse(hwd)
	if len(r.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(r.Details))
	}
	if r.Details[0].Multiplier != "1.5000" {
		t.Errorf("expected '1.5000', got %s", r.Details[0].Multiplier)
	}
}

func TestToHeaderResponse_WorkflowStatusRejectedDisplayedAsReturned(t *testing.T) {
	h := testHeader()
	h.WorkflowStatus = mappingjurnal.WorkflowStatusRejected
	hwd := &mappingjurnal.HeaderWithDetails{Header: h, Details: []*mappingjurnal.Detail{}}
	r := mappingjurnal.ToHeaderResponse(hwd)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED for REJECTED status, got %s", r.WorkflowStatus)
	}
}

func TestToHeaderResponse_NilArraysBecomEmptySlices(t *testing.T) {
	h := testHeader()
	h.TipeInstrumenBerlaku = nil
	h.KlasifikasiBerlaku = nil
	hwd := &mappingjurnal.HeaderWithDetails{Header: h, Details: nil}
	r := mappingjurnal.ToHeaderResponse(hwd)
	if r.TipeInstrumenBerlaku == nil {
		t.Error("TipeInstrumenBerlaku should be empty slice not nil")
	}
	if r.KlasifikasiBerlaku == nil {
		t.Error("KlasifikasiBerlaku should be empty slice not nil")
	}
	if r.Details == nil {
		t.Error("Details should be empty slice not nil")
	}
}

// ─── GET /master/mapping-jurnal/export ───────────────────────────────────────

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

// ─── Workflow status endpoint ──────────────────────────────────────────────────

func TestWorkflowStatus_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/"+testHeaderID.String()+"/workflow", nil)
	r.ServeHTTP(rec, req)
	// GetByID returns 404 before forwarding to workflow handler
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── History endpoint ─────────────────────────────────────────────────────────

func TestHistory_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/invalid-uuid/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHistory_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/"+testHeaderID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHistory_Returns200WithItems(t *testing.T) {
	h := testHeader()
	items := []mappingjurnal.AuditHistoryItem{
		{
			EventID:     uuid.New().String(),
			EventTime:   time.Now().Format(time.RFC3339),
			ActorUserID: testActorID.String(),
			ActorRole:   "ROLE-AKUN",
			Action:      "MAPPING_JURNAL.CREATE",
		},
	}
	r := newRouter(buildSvc(&repoAdapter{
		getHeader:    &stubGetHeader{header: h},
		auditHistory: &stubAuditHistory{items: items, hasMore: false},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/mapping-jurnal/"+testHeaderID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 history item, got %d", len(resp.Data))
	}
}

// ─── Workflow routes — basic routing checks ───────────────────────────────────

func TestSubmitRoute_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getHeader: &stubGetHeader{header: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	body := mustJSON(t, map[string]interface{}{"signatureMethod": "JWT_STANDARD"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal/"+testHeaderID.String()+"/submit", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when entity not found before submit, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestReviewRoute_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/mapping-jurnal/not-uuid/review", nil)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ─── Decimal precision test ───────────────────────────────────────────────────

func TestDecimalMultiplier_4DecimalPlaces(t *testing.T) {
	d := testDetailPair()[0]
	d.Multiplier = decimal.NewFromFloat(1.23456789)
	resp := mappingjurnal.ToDetailResponse(d)
	// StringFixed(4) rounds to 4 decimal places
	expected := fmt.Sprintf("%.4f", 1.2346)
	if resp.Multiplier != expected {
		t.Errorf("expected multiplier=%s, got %s", expected, resp.Multiplier)
	}
}
