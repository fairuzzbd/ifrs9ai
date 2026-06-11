package lookthrough

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── mock services for handler tests ─────────────────────────────────────────

// mockCompositionService implements CompositionServiceIface for handler tests.
type mockCompositionService struct {
	submitResult  *CompositionGroup
	submitErr     error
	reviewResult  *FundComposition
	reviewErr     error
	approveResult *FundComposition
	approveErr    error
	rejectResult  *FundComposition
	rejectErr     error
}

func (m *mockCompositionService) Submit(_ context.Context, _ SubmitCompositionRequest, _ uuid.UUID, _ string) (*CompositionGroup, error) {
	return m.submitResult, m.submitErr
}
func (m *mockCompositionService) Review(_ context.Context, _ WorkflowActionRequest) (*FundComposition, error) {
	return m.reviewResult, m.reviewErr
}
func (m *mockCompositionService) Approve(_ context.Context, _ WorkflowActionRequest, _ *uuid.UUID) (*FundComposition, error) {
	return m.approveResult, m.approveErr
}
func (m *mockCompositionService) Reject(_ context.Context, _ WorkflowActionRequest) (*FundComposition, error) {
	return m.rejectResult, m.rejectErr
}
func (m *mockCompositionService) GetCompositionGroup(_ context.Context, _ uuid.UUID) (*CompositionGroup, error) {
	return nil, nil
}
func (m *mockCompositionService) ListCompositions(_ context.Context, _ uuid.UUID, _, _ string, _ int, _, _ string) ([]FundComposition, string, bool, error) {
	return nil, "", false, nil
}

// mockLookthroughService implements LookthroughServiceIface for handler tests.
type mockLookthroughService struct {
	computeResult  *LookthroughResult
	computeErr     error
	previewRows    []PreviewSummaryRow
	previewHasMore bool
	previewCursor  string
	previewErr     error
}

func (m *mockLookthroughService) Compute(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, _ time.Time) (*LookthroughResult, error) {
	return m.computeResult, m.computeErr
}
func (m *mockLookthroughService) Preview(_ context.Context, _ uuid.UUID, _ time.Time, _ string, _ int) ([]PreviewSummaryRow, string, bool, error) {
	return m.previewRows, m.previewCursor, m.previewHasMore, m.previewErr
}

// mockResultRepoForHandler implements LookthroughResultRepo for handler tests.
type mockResultRepoForHandler struct {
	stored *StoredLookthroughResult
	getErr error
}

func (m *mockResultRepoForHandler) UpsertResult(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ LookthroughResult, _ uuid.UUID, _ uuid.UUID, _ time.Time, _ string) error {
	return nil
}
func (m *mockResultRepoForHandler) GetByInstrumenAndRun(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*StoredLookthroughResult, error) {
	return m.stored, m.getErr
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// buildRouter creates a test Gin engine with the handler registered.
func buildRouter(h *Handler) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/api/v1")

	// Register all routes directly (no JWT middleware in test).
	v1.POST("/ecl/lookthrough/composition", h.SubmitComposition)
	v1.POST("/ecl/lookthrough/composition/:id/review", h.ReviewComposition)
	v1.POST("/ecl/lookthrough/composition/:id/approve", h.ApproveComposition)
	v1.POST("/ecl/lookthrough/composition/:id/reject", h.RejectComposition)
	v1.POST("/ecl/lookthrough/compute", h.ComputeLookthrough)
	v1.POST("/ecl/lookthrough/bulk-compute", h.BulkComputeLookthrough)
	v1.GET("/ecl/lookthrough/preview", h.ListLookthroughPreview)
	v1.GET("/ecl/lookthrough/result/:instrumenId/:runId", h.GetLookthroughResult)
	return r
}

// withUser adds JWT-like claims into Gin context via middleware, matching the exact
// keys that handler.go reads: user_id, roles, mfa_verified, permissions.
// Permissions are derived from role so all relevant permission checks pass.
func withUser(userID uuid.UUID, role string, mfaVerified bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Set("roles", []string{role})
		c.Set("mfa_verified", mfaVerified)
		// Grant all lookthrough permissions so hasPermission() passes.
		c.Set("permissions", []string{
			PermFundCompositionCreate,
			PermFundCompositionReview,
			PermFundCompositionApprove,
			PermFundCompositionRead,
			PermLookthroughCompute,
			PermLookthroughPreview,
		})
		c.Next()
	}
}

// buildRouterWithUser creates router with user claims pre-set.
func buildRouterWithUser(h *Handler, userID uuid.UUID, role string, mfaVerified bool) *gin.Engine {
	r := gin.New()
	r.Use(withUser(userID, role, mfaVerified))
	v1 := r.Group("/api/v1")
	v1.POST("/ecl/lookthrough/composition", h.SubmitComposition)
	v1.POST("/ecl/lookthrough/composition/:id/review", h.ReviewComposition)
	v1.POST("/ecl/lookthrough/composition/:id/approve", h.ApproveComposition)
	v1.POST("/ecl/lookthrough/composition/:id/reject", h.RejectComposition)
	v1.POST("/ecl/lookthrough/compute", h.ComputeLookthrough)
	v1.POST("/ecl/lookthrough/bulk-compute", h.BulkComputeLookthrough)
	v1.GET("/ecl/lookthrough/preview", h.ListLookthroughPreview)
	v1.GET("/ecl/lookthrough/result/:instrumenId/:runId", h.GetLookthroughResult)
	return r
}

// ─── Handler tests ────────────────────────────────────────────────────────────

// TestHandler_SubmitComposition_201 verifies 201 on valid submission.
// AC: APP-C-LKT-002-AC01.
func TestHandler_SubmitComposition_201(t *testing.T) {
	t.Parallel()
	compositionID := uuid.New()
	instrumenID := uuid.New()
	compSvc := &mockCompositionService{
		submitResult: &CompositionGroup{
			Header: FundComposition{
				ID:             compositionID,
				InstrumenID:    instrumenID,
				WorkflowStatus: WorkflowStatusPendingReview,
				MakerID:        uuid.New(),
				EffectiveFrom:  time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
				EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	h := NewHandler(compSvc, &mockLookthroughService{}, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-AKUN", false)

	body := map[string]interface{}{
		"instrumenId":   instrumenID.String(),
		"effectiveFrom": "2026-06-11",
		"lines": []map[string]interface{}{
			{"assetClass": "GOVT_BOND", "weightPct": "60.00"},
			{"assetClass": "CORP_BOND", "weightPct": "40.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/lookthrough/composition",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandler_SubmitComposition_NotReksadana_422 verifies 422 for non-REKSADANA.
// AC: APP-C-LKT-002-AC02.
func TestHandler_SubmitComposition_NotReksadana_422(t *testing.T) {
	t.Parallel()
	compSvc := &mockCompositionService{
		submitErr: ErrInstrumenNotReksadana(uuid.New().String(), "DEPOSITO"),
	}
	h := NewHandler(compSvc, &mockLookthroughService{}, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-AKUN", false)

	body := map[string]interface{}{
		"instrumenId":   uuid.New().String(),
		"effectiveFrom": "2026-06-11",
		"lines": []map[string]interface{}{
			{"assetClass": "GOVT_BOND", "weightPct": "100.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/lookthrough/composition",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	verifyErrorCode(t, w.Body.Bytes(), CodeLookthroughInstrumenNotReksadana)
}

// TestHandler_ApproveComposition_ALCO_MFARequired_403 verifies ROLE-ALCO without MFA gets 403.
// DEC-026: ROLE-ALCO MFA mandatory; step-up on approve (state-machine §5.2).
func TestHandler_ApproveComposition_ALCO_MFARequired_403(t *testing.T) {
	t.Parallel()
	compSvc := &mockCompositionService{
		approveResult: &FundComposition{ID: uuid.New()},
	}
	h := NewHandler(compSvc, &mockLookthroughService{}, &mockResultRepoForHandler{})
	// ROLE-ALCO user WITHOUT MFA verified.
	r := buildRouterWithUser(h, uuid.New(), "ROLE-ALCO", false)

	body := map[string]interface{}{
		"comment":          "ALCO approves",
		"signature_method": "JWT_STEP_UP",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/ecl/lookthrough/composition/"+uuid.New().String()+"/approve",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for ALCO without MFA, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandler_ApproveComposition_ALCO_WithMFA_200 verifies ROLE-ALCO with MFA succeeds.
func TestHandler_ApproveComposition_ALCO_WithMFA_200(t *testing.T) {
	t.Parallel()
	compositionID := uuid.New()
	compSvc := &mockCompositionService{
		approveResult: &FundComposition{
			ID:             compositionID,
			MakerID:        uuid.New(),
			WorkflowStatus: WorkflowStatusApprovedActive,
			EffectiveFrom:  time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
			EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		},
	}
	h := NewHandler(compSvc, &mockLookthroughService{}, &mockResultRepoForHandler{})
	// ROLE-ALCO WITH MFA.
	r := buildRouterWithUser(h, uuid.New(), "ROLE-ALCO", true)

	body := map[string]interface{}{
		"comment":          "ALCO approves",
		"signature_method": "JWT_STEP_UP",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/ecl/lookthrough/composition/"+compositionID.String()+"/approve",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for ALCO with MFA, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandler_ReviewComposition_SoDViolation_403 verifies SoD returns 403.
// AC: APP-C-LKT-002-AC10.
func TestHandler_ReviewComposition_SoDViolation_403(t *testing.T) {
	t.Parallel()
	compSvc := &mockCompositionService{
		reviewErr: ErrCompositionSoDViolation("ROLE-RISK", uuid.New().String()),
	}
	h := NewHandler(compSvc, &mockLookthroughService{}, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	body := map[string]interface{}{
		"comment":          "review",
		"signature_method": "JWT_STEP_UP",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/ecl/lookthrough/composition/"+uuid.New().String()+"/review",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for SoD violation, got %d: %s", w.Code, w.Body.String())
	}
	verifyErrorCode(t, w.Body.Bytes(), CodeLookthroughCompositionSoDViolation)
}

// TestHandler_ComputeLookthrough_FVTPLSkip_200 verifies 200 with warning for FVTPL.
// AC: APP-C-LKT-001-AC07.
func TestHandler_ComputeLookthrough_FVTPLSkip_200(t *testing.T) {
	t.Parallel()
	ltSvc := &mockLookthroughService{
		computeResult: &LookthroughResult{
			InstrumenID:  uuid.New(),
			TotalECLIDR:  decimal.Zero,
			FVTPLSkipped: true,
			Warning:      "FVTPL instrument — ECL not applicable per PSAK 71",
		},
	}
	h := NewHandler(&mockCompositionService{}, ltSvc, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	body := map[string]interface{}{
		"instrumenId":    uuid.New().String(),
		"runId":          uuid.New().String(),
		"periodeId":      uuid.New().String(),
		"evaluationDate": "2026-06-11",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/lookthrough/compute",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for FVTPL skip, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("expected data in response")
	}
	// fvtplSkipped should be present in DTO.
	if _, ok := data["fvtplSkipped"]; !ok {
		t.Error("expected fvtplSkipped in FVTPL skip response")
	}
}

// TestHandler_BulkComputeLookthrough_202 verifies 202 + jobId returned.
// AC: APP-C-LKT-001-AC14 (async job pattern).
func TestHandler_BulkComputeLookthrough_202(t *testing.T) {
	t.Parallel()
	// BulkComputeLookthrough returns 202+jobId immediately without calling service.BulkCompute.
	// The actual compute is async (Asynq job). Mock svc has no bulk fields needed.
	ltSvc := &mockLookthroughService{}
	h := NewHandler(&mockCompositionService{}, ltSvc, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	body := map[string]interface{}{
		"runId":          uuid.New().String(),
		"periodeId":      uuid.New().String(),
		"evaluationDate": "2026-06-11",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/lookthrough/bulk-compute",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 for bulk compute, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("expected data in response")
	}
	if _, ok := data["jobId"]; !ok {
		t.Error("expected jobId in 202 response")
	}
}

// TestHandler_ComputeLookthrough_POCIDeferred_422 verifies 422 for POCI.
// AC: APP-C-LKT-001-AC08.
func TestHandler_ComputeLookthrough_POCIDeferred_422(t *testing.T) {
	t.Parallel()
	ltSvc := &mockLookthroughService{
		computeErr: ErrPOCIDeferred(uuid.New().String()),
	}
	h := NewHandler(&mockCompositionService{}, ltSvc, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	body := map[string]interface{}{
		"instrumenId":    uuid.New().String(),
		"runId":          uuid.New().String(),
		"periodeId":      uuid.New().String(),
		"evaluationDate": "2026-06-11",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ecl/lookthrough/compute",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for POCI, got %d: %s", w.Code, w.Body.String())
	}
	verifyErrorCode(t, w.Body.Bytes(), CodeLookthroughPOCIDeferred)
}

// TestHandler_PreviewLookthrough_200 verifies paginated preview response.
// AC: APP-C-LKT-001-AC05.
func TestHandler_PreviewLookthrough_200(t *testing.T) {
	t.Parallel()
	instrumenID := uuid.New()
	ecl := decimal.NewFromFloat(78_750).StringFixed(4)
	ltSvc := &mockLookthroughService{
		previewRows: []PreviewSummaryRow{
			{
				InstrumenID:            instrumenID,
				InstrumenNama:          "Reksa Dana Campuran X",
				KlasifikasiPsak71:      "AC",
				NABIDRStr:              decimal.NewFromFloat(10_000_000).StringFixed(4),
				HasComposition:         true,
				TotalECLEstimateIDRStr: &ecl,
			},
		},
		previewHasMore: false,
	}
	h := NewHandler(&mockCompositionService{}, ltSvc, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/lookthrough/preview?periode_id="+uuid.New().String()+"&evaluation_date=2026-06-11&limit=50",
		nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("expected 1 row, got %d", len(data))
	}
}

// TestHandler_GetLookthroughResult_NotFound verifies appropriate error when result not found.
// FundCompositionMissing maps to HTTP 422 per domain error HTTPStatus().
func TestHandler_GetLookthroughResult_NotFound(t *testing.T) {
	t.Parallel()
	resultRepo := &mockResultRepoForHandler{
		getErr: ErrFundCompositionMissing(uuid.New().String(), "2026-06-11"),
	}
	h := NewHandler(&mockCompositionService{}, &mockLookthroughService{}, resultRepo)
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/ecl/lookthrough/result/"+uuid.New().String()+"/"+uuid.New().String(),
		nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for missing result, got %d: %s", w.Code, w.Body.String())
	}
	verifyErrorCode(t, w.Body.Bytes(), CodeLookthroughFundCompositionMissing)
}

// TestHandler_InvalidUUID_400 verifies 400 for malformed UUID in path.
func TestHandler_InvalidUUID_400(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockCompositionService{}, &mockLookthroughService{}, &mockResultRepoForHandler{})
	r := buildRouterWithUser(h, uuid.New(), "ROLE-RISK", false)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/ecl/lookthrough/composition/not-a-uuid/review",
		bytes.NewReader([]byte(`{"comment":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── helper ──────────────────────────────────────────────────────────────────

// verifyErrorCode asserts the response body contains the expected error code.
func verifyErrorCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	errObj, _ := resp["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatalf("expected 'error' field in response, got: %s", string(body))
	}
	code, _ := errObj["code"].(string)
	if code != expectedCode {
		t.Errorf("expected error code %s, got %s", expectedCode, code)
	}
}
