package penempatan_test

// handler_test.go — HTTP handler tests for Penempatan Deposito (P5-M1).
//
// Pattern: build a stubHooks, wrap in NewHandlerWithHooks, wire into test router,
// fire httptest requests, assert HTTP status.
// No DB required. Auth/perm logic exercised by setting/omitting claims.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/app-b/penempatan"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

func init() { gin.SetMode(gin.TestMode) }

// ─── stubHooks ────────────────────────────────────────────────────────────

// stubHooks implements penempatan.TestStubHooks with per-method function fields.
type stubHooks struct {
	createFn           func(context.Context, penempatan.CreateRequest, *auth.Claims) (*penempatan.Penempatan, error)
	listFn             func(context.Context, listquery.Query, bool, *auth.Claims) (penempatan.ListResult, error)
	getByIDFn          func(context.Context, uuid.UUID, *auth.Claims) (*penempatan.Penempatan, error)
	updateFn           func(context.Context, uuid.UUID, penempatan.UpdateRequest, *auth.Claims) (*penempatan.Penempatan, error)
	withdrawFn         func(context.Context, uuid.UUID, *auth.Claims) error
	submitFn           func(context.Context, uuid.UUID, penempatan.WorkflowActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	reviewFn           func(context.Context, uuid.UUID, penempatan.WorkflowActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	approveFn          func(context.Context, uuid.UUID, penempatan.WorkflowActionRequest, *auth.Claims) (*penempatan.ApproveResult, error)
	rejectFn           func(context.Context, uuid.UUID, penempatan.RejectActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	terminateFn        func(context.Context, uuid.UUID, penempatan.TerminateRequestBody, *auth.Claims) (*penempatan.Penempatan, error)
	terminateReviewFn  func(context.Context, uuid.UUID, penempatan.WorkflowActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	terminateApproveFn func(context.Context, uuid.UUID, penempatan.WorkflowActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	terminateRejectFn  func(context.Context, uuid.UUID, penempatan.RejectActionRequest, *auth.Claims) (*penempatan.Penempatan, error)
	eirPreviewFn       func(context.Context, uuid.UUID, *auth.Claims) (*penempatan.EIRPreviewResult, error)
	auditTimelineFn    func(context.Context, uuid.UUID, *auth.Claims) ([]penempatan.AuditTimelineEvent, error)
}

func (s *stubHooks) DoCreate(ctx context.Context, req penempatan.CreateRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.createFn != nil {
		return s.createFn(ctx, req, cl)
	}
	return nil, errNilStub("Create")
}
func (s *stubHooks) DoList(ctx context.Context, q listquery.Query, inc bool, cl *auth.Claims) (penempatan.ListResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, q, inc, cl)
	}
	return penempatan.ListResult{}, errNilStub("List")
}
func (s *stubHooks) DoGetByID(ctx context.Context, id uuid.UUID, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id, cl)
	}
	return nil, errNilStub("GetByID")
}
func (s *stubHooks) DoUpdate(ctx context.Context, id uuid.UUID, req penempatan.UpdateRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, req, cl)
	}
	return nil, errNilStub("Update")
}
func (s *stubHooks) DoWithdraw(ctx context.Context, id uuid.UUID, cl *auth.Claims) error {
	if s.withdrawFn != nil {
		return s.withdrawFn(ctx, id, cl)
	}
	return errNilStub("Withdraw")
}
func (s *stubHooks) DoSubmit(ctx context.Context, id uuid.UUID, req penempatan.WorkflowActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.submitFn != nil {
		return s.submitFn(ctx, id, req, cl)
	}
	return nil, errNilStub("Submit")
}
func (s *stubHooks) DoReview(ctx context.Context, id uuid.UUID, req penempatan.WorkflowActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.reviewFn != nil {
		return s.reviewFn(ctx, id, req, cl)
	}
	return nil, errNilStub("Review")
}
func (s *stubHooks) DoApprove(ctx context.Context, id uuid.UUID, req penempatan.WorkflowActionRequest, cl *auth.Claims) (*penempatan.ApproveResult, error) {
	if s.approveFn != nil {
		return s.approveFn(ctx, id, req, cl)
	}
	return nil, errNilStub("Approve")
}
func (s *stubHooks) DoReject(ctx context.Context, id uuid.UUID, req penempatan.RejectActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.rejectFn != nil {
		return s.rejectFn(ctx, id, req, cl)
	}
	return nil, errNilStub("Reject")
}
func (s *stubHooks) DoTerminateRequest(ctx context.Context, id uuid.UUID, req penempatan.TerminateRequestBody, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.terminateFn != nil {
		return s.terminateFn(ctx, id, req, cl)
	}
	return nil, errNilStub("TerminateRequest")
}
func (s *stubHooks) DoTerminateReview(ctx context.Context, id uuid.UUID, req penempatan.WorkflowActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.terminateReviewFn != nil {
		return s.terminateReviewFn(ctx, id, req, cl)
	}
	return nil, errNilStub("TerminateReview")
}
func (s *stubHooks) DoTerminateApprove(ctx context.Context, id uuid.UUID, req penempatan.WorkflowActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.terminateApproveFn != nil {
		return s.terminateApproveFn(ctx, id, req, cl)
	}
	return nil, errNilStub("TerminateApprove")
}
func (s *stubHooks) DoTerminateReject(ctx context.Context, id uuid.UUID, req penempatan.RejectActionRequest, cl *auth.Claims) (*penempatan.Penempatan, error) {
	if s.terminateRejectFn != nil {
		return s.terminateRejectFn(ctx, id, req, cl)
	}
	return nil, errNilStub("TerminateReject")
}
func (s *stubHooks) DoEIRPreview(ctx context.Context, id uuid.UUID, cl *auth.Claims) (*penempatan.EIRPreviewResult, error) {
	if s.eirPreviewFn != nil {
		return s.eirPreviewFn(ctx, id, cl)
	}
	return nil, errNilStub("EIRPreview")
}
func (s *stubHooks) DoAuditTimeline(ctx context.Context, id uuid.UUID, cl *auth.Claims) ([]penempatan.AuditTimelineEvent, error) {
	if s.auditTimelineFn != nil {
		return s.auditTimelineFn(ctx, id, cl)
	}
	return nil, errNilStub("AuditTimeline")
}

func errNilStub(name string) error {
	return domainerrors.New(domainerrors.CodeInternal, "stub not set: "+name)
}

// ─── Test helpers ─────────────────────────────────────────────────────────

func claimsWithPerms(perms ...string) *auth.Claims {
	return &auth.Claims{
		Sub:         uuid.New().String(),
		TenantID:    "TUGURE",
		Roles:       []string{"ROLE-MAKER-TR"},
		Permissions: perms,
	}
}

func claimsWithStepUp(perms ...string) *auth.Claims {
	now := time.Now().Unix()
	return &auth.Claims{
		Sub:              uuid.New().String(),
		TenantID:         "TUGURE",
		Roles:            []string{"ROLE-APPR-TR"},
		Permissions:      perms,
		StepupVerifiedAt: &now,
	}
}

func newTestPenempatan() *penempatan.Penempatan {
	return &penempatan.Penempatan{
		ID:                uuid.New(),
		KodeTransaksi:     "DP-000001",
		WorkflowStatus:    penempatan.StatusDraft,
		NominalIDR:        decimal.NewFromFloat(1_000_000_000),
		TanggalPenempatan: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalJatuhTempo: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC),
		KuponPersen:       decimal.NewFromFloat(5.25),
		TenorBulan:        12,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		TenantID:          "TUGURE",
		MakerID:           uuid.New(),
	}
}

func setupRouter(hooks penempatan.TestStubHooks, claims *auth.Claims) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			c.Set("claims", claims)
		}
		c.Next()
	})
	h := penempatan.NewHandlerWithHooks(hooks)
	penempatan.RegisterRoutesForTest(r.Group("/api/v1"), h)
	return r
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func doRequest(t *testing.T, r *gin.Engine, method, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, path, body)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, path, nil)
	}
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─── NewHandler panic tests ────────────────────────────────────────────────

func TestNewHandlerWithHooks_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil service, got none")
		}
	}()
	penempatan.NewHandler(nil) //nolint:staticcheck
}

// ─── GET /trx/penempatan-deposito (List) ─────────────────────────────────

func TestListPenempatan_NoAuth_401(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, nil)
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListPenempatan_NoReadPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms("transaksi.create"))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.listFn = func(_ context.Context, _ listquery.Query, _ bool, _ *auth.Claims) (penempatan.ListResult, error) {
		p := newTestPenempatan()
		return penempatan.ListResult{
			Items: []penempatan.ListItem{
				{
					ID:                p.ID,
					KodeTransaksi:     p.KodeTransaksi,
					WorkflowStatus:    p.WorkflowStatus,
					NominalIDR:        p.NominalIDR,
					TanggalPenempatan: p.TanggalPenempatan,
					TanggalJatuhTempo: p.TanggalJatuhTempo,
					KuponPersen:       p.KuponPersen,
					TenorBulan:        p.TenorBulan,
					MakerID:           p.MakerID,
					CreatedAt:         p.CreatedAt,
				},
			},
			TotalEst: 1,
			HasMore:  false,
		}, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito (Create) ──────────────────────────────

func TestCreatePenempatan_NoAuth_401(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, nil)
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito", jsonBody(t, map[string]any{"foo": 1}))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreatePenempatan_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms("transaksi.read"))
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito", jsonBody(t, map[string]any{"foo": 1}))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCreatePenempatan_BadBody_400(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiCreate))
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito", bytes.NewBufferString("{bad json"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreatePenempatan_ServiceError_404(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.createFn = func(_ context.Context, _ penempatan.CreateRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return nil, domainerrors.ErrNotFound("Instrumen")
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiCreate))
	req := validCreateRequest()
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito", jsonBody(t, req))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePenempatan_OK_201(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.createFn = func(_ context.Context, _ penempatan.CreateRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return newTestPenempatan(), nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiCreate))
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito", jsonBody(t, validCreateRequest()))
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── GET /trx/penempatan-deposito/:id ────────────────────────────────────

func TestGetPenempatan_NoAuth_401(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, nil)
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetPenempatan_BadUUID_400(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetPenempatan_NotFound_404(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.getByIDFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.getByIDFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return newTestPenempatan(), nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── PATCH /trx/penempatan-deposito/:id (Update) ─────────────────────────

func TestUpdatePenempatan_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodPatch, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(),
		jsonBody(t, map[string]any{"rowVersion": 1}))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestUpdatePenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.updateFn = func(_ context.Context, _ uuid.UUID, _ penempatan.UpdateRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return newTestPenempatan(), nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiUpdate))
	body := penempatan.UpdateRequest{RowVersion: 1}
	w := doRequest(t, r, http.MethodPatch, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── DELETE /trx/penempatan-deposito/:id (Withdraw) ──────────────────────

func TestWithdrawPenempatan_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodDelete, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestWithdrawPenempatan_OK_204(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.withdrawFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) error { return nil }
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiDelete))
	w := doRequest(t, r, http.MethodDelete, "/api/v1/trx/penempatan-deposito/"+uuid.New().String(), nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/submit ────────────────────────────

func TestSubmitPenempatan_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiRead))
	body := penempatan.WorkflowActionRequest{Comment: "ok", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/submit", jsonBody(t, body))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSubmitPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.submitFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusPendingReview
		return p, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiSubmit))
	body := penempatan.WorkflowActionRequest{Comment: "submitting", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/submit", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/review ────────────────────────────

func TestReviewPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.reviewFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusPendingApproval
		return p, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiReview))
	body := penempatan.WorkflowActionRequest{Comment: "LGTM", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/review", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/approve ───────────────────────────

func TestApprovePenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.approveFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.ApproveResult, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusApprovedActive
		return &penempatan.ApproveResult{
			Penempatan:    p,
			StagingAction: "STAGE_1_ASSIGNED",
		}, nil
	}
	r := setupRouter(s, claimsWithStepUp(penempatan.PermTransaksiApprove))
	body := penempatan.WorkflowActionRequest{Comment: "approved", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/approve", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestApprovePenempatan_InvalidTransition_422(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.approveFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.ApproveResult, error) {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, "wrong status")
	}
	r := setupRouter(s, claimsWithStepUp(penempatan.PermTransaksiApprove))
	body := penempatan.WorkflowActionRequest{Comment: "try", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/approve", jsonBody(t, body))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/reject ────────────────────────────

func TestRejectPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.rejectFn = func(_ context.Context, _ uuid.UUID, _ penempatan.RejectActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		return newTestPenempatan(), nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiReject))
	body := penempatan.RejectActionRequest{Comment: "rejected because this reason is more than 30 chars", SignatureMethod: "JWT"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/reject", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/terminate ─────────────────────────

func TestTerminatePenempatan_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms(penempatan.PermTransaksiRead))
	body := penempatan.TerminateRequestBody{TerminateReason: "long enough reason for terminate that passes validation"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/terminate", jsonBody(t, body))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTerminatePenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.terminateFn = func(_ context.Context, _ uuid.UUID, _ penempatan.TerminateRequestBody, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusTerminationPendingReview
		return p, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiTerminate))
	body := penempatan.TerminateRequestBody{TerminateReason: "long enough reason for terminate that passes validation"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/terminate", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/terminate-review ──────────────────

func TestTerminateReviewPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.terminateReviewFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusTerminationPendingApproval
		return p, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiReview))
	body := penempatan.WorkflowActionRequest{Comment: "ok", SignatureMethod: "JWT"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/terminate-review", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/terminate-approve ─────────────────

func TestTerminateApprovePenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.terminateApproveFn = func(_ context.Context, _ uuid.UUID, _ penempatan.WorkflowActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusTerminated
		return p, nil
	}
	r := setupRouter(s, claimsWithStepUp(penempatan.PermTransaksiApprove))
	body := penempatan.WorkflowActionRequest{Comment: "terminate approved", SignatureMethod: "JWT_STEP_UP"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/terminate-approve", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── POST /trx/penempatan-deposito/:id/terminate-reject ──────────────────

func TestTerminateRejectPenempatan_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.terminateRejectFn = func(_ context.Context, _ uuid.UUID, _ penempatan.RejectActionRequest, _ *auth.Claims) (*penempatan.Penempatan, error) {
		p := newTestPenempatan()
		p.WorkflowStatus = penempatan.StatusApprovedActive
		return p, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiReject))
	body := penempatan.RejectActionRequest{Comment: "terminate reject reason must be more than 30 chars long"}
	w := doRequest(t, r, http.MethodPost, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/terminate-reject", jsonBody(t, body))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── GET /trx/penempatan-deposito/:id/eir-preview ────────────────────────

func TestGetEIRPreview_NoPerm_403(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, claimsWithPerms("other.perm"))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/eir-preview", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetEIRPreview_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.eirPreviewFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) (*penempatan.EIRPreviewResult, error) {
		return &penempatan.EIRPreviewResult{
			PeriodePreview:       3,
			AmortizationSchedule: []penempatan.AmortizationRow{},
		}, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/eir-preview", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetEIRPreview_EIRPermOnly_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.eirPreviewFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) (*penempatan.EIRPreviewResult, error) {
		return &penempatan.EIRPreviewResult{AmortizationSchedule: []penempatan.AmortizationRow{}}, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermEIRPreview))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/eir-preview", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── GET /trx/penempatan-deposito/:id/audit-timeline ─────────────────────

func TestGetAuditTimeline_NoAuth_401(t *testing.T) {
	t.Parallel()
	r := setupRouter(&stubHooks{}, nil)
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/audit-timeline", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetAuditTimeline_OK_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.auditTimelineFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) ([]penempatan.AuditTimelineEvent, error) {
		return []penempatan.AuditTimelineEvent{}, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermTransaksiRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/audit-timeline", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetAuditTimeline_AuditPermOnly_200(t *testing.T) {
	t.Parallel()
	s := &stubHooks{}
	s.auditTimelineFn = func(_ context.Context, _ uuid.UUID, _ *auth.Claims) ([]penempatan.AuditTimelineEvent, error) {
		return []penempatan.AuditTimelineEvent{}, nil
	}
	r := setupRouter(s, claimsWithPerms(penempatan.PermAuditLogRead))
	w := doRequest(t, r, http.MethodGet, "/api/v1/trx/penempatan-deposito/"+uuid.New().String()+"/audit-timeline", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func validCreateRequest() penempatan.CreateRequest {
	return penempatan.CreateRequest{
		InstrumenID:        uuid.New(),
		CounterpartyBankID: uuid.New(),
		PeriodeID:          uuid.New(),
		MataUangID:         uuid.New(),
		TanggalPenempatan:  "2026-06-01",
		TenorBulan:         12,
		KuponPersen:        decimal.NewFromFloat(5.25),
	}
}
