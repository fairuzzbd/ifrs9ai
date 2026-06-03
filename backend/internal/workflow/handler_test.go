package workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// -----------------------------------------------------------------------
// Router setup helper
// -----------------------------------------------------------------------

func buildTestRouter(svc *Service) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Inject trace ID
		c.Set(response.TraceIDKey, "test-trace-id")
		ctx := middleware.TraceIDFromContext(c.Request.Context())
		_ = ctx
		c.Next()
	})
	h := NewHandler(svc)
	v1 := r.Group("/api/v1")
	RegisterRoutes(v1, h)
	return r
}

func buildTestRouterWithClaims(svc *Service, userID uuid.UUID, username string, permissions ...string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(response.TraceIDKey, "test-trace-id")
		claims := &auth.Claims{
			Sub:               userID.String(),
			PreferredUsername: username,
			Roles:             []string{"ROLE-MAKER-TR"},
			Permissions:       permissions,
			TenantID:          "TUGURE",
		}
		c.Set("claims", claims)
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewHandler(svc)
	v1 := r.Group("/api/v1")
	RegisterRoutes(v1, h)
	return r
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewBuffer(b)
}

// -----------------------------------------------------------------------
// Submit endpoint tests
// -----------------------------------------------------------------------

func TestHandler_Submit_Success(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	r := buildTestRouterWithClaims(svc, mUUID, "maker", "penempatan.submit")

	body := jsonBody(t, map[string]any{"signatureMethod": "JWT_STANDARD"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/"+inst.EntityID.String()+"/submit", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data := resp["data"].(map[string]any)
	if data["currentState"] != string(StatePendingReview) {
		t.Errorf("expected PENDING_REVIEW, got %v", data["currentState"])
	}
}

func TestHandler_Submit_InvalidUUID(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, _ := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	r := buildTestRouterWithClaims(svc, mUUID, "maker", "penempatan.submit")

	body := jsonBody(t, map[string]any{"signatureMethod": "JWT_STANDARD"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/not-a-uuid/submit", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_Submit_MissingPermission(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	// No penempatan.submit permission
	r := buildTestRouterWithClaims(svc, mUUID, "maker" /* no permissions */)

	body := jsonBody(t, map[string]any{"signatureMethod": "JWT_STANDARD"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/"+inst.EntityID.String()+"/submit", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Submit_NoClaims(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	inst := seedWorkflow(repo, cfg, uuid.New())
	// No claims injected
	r := buildTestRouter(svc)

	body := jsonBody(t, map[string]any{"signatureMethod": "JWT_STANDARD"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/"+inst.EntityID.String()+"/submit", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// No claims → handler calls svc.Submit → service returns UNAUTHORIZED
	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 or 403, got %d: %s", w.Code, w.Body.String())
	}
}

// -----------------------------------------------------------------------
// Reject endpoint — comment validation
// -----------------------------------------------------------------------

func TestHandler_Reject_ShortComment(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	// Submit first
	ctxM := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, _ = svc.Submit(ctxM, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})

	r := buildTestRouterWithClaims(svc, rUUID, "reviewer", "penempatan.reject")

	// Comment is only 5 chars — should fail validation at handler
	body := jsonBody(t, map[string]any{"signatureMethod": "JWT_STANDARD", "comment": "short"})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/"+inst.EntityID.String()+"/reject", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short reject comment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Reject_ValidComment(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	ctxM := ctxWithClaims(mUUID, "maker", "penempatan.submit")
	_, _ = svc.Submit(ctxM, SubmitInput{EntityType: cfg.EntityType, EntityID: inst.EntityID, Request: defaultActionRequest()})

	r := buildTestRouterWithClaims(svc, rUUID, "reviewer", "penempatan.reject")

	body := jsonBody(t, map[string]any{
		"signatureMethod": "JWT_STANDARD",
		"comment":         "Data tidak sesuai prosedur, harap perbaiki",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/penempatan/"+inst.EntityID.String()+"/reject", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// -----------------------------------------------------------------------
// GetStatus endpoint
// -----------------------------------------------------------------------

func TestHandler_GetStatus_Success(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)

	r := buildTestRouterWithClaims(svc, mUUID, "maker", "penempatan.read")

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/penempatan/"+inst.EntityID.String()+"/workflow", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["currentState"] != string(StateDraft) {
		t.Errorf("expected DRAFT, got %v", data["currentState"])
	}
}

func TestHandler_GetStatus_NotFound(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, _ := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	r := buildTestRouterWithClaims(svc, mUUID, "maker", "penempatan.read")

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/penempatan/"+uuid.New().String()+"/workflow", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// -----------------------------------------------------------------------
// normalizeEntityType + permissionFor helpers
// -----------------------------------------------------------------------

func TestNormalizeEntityType(t *testing.T) {
	cases := map[string]string{
		"penempatan":    "PENEMPATAN",
		"ecl-parameter": "ECL_PARAMETER",
		"upload-batch":  "UPLOAD_BATCH",
		"klasifikasi":   "KLASIFIKASI",
	}
	for input, want := range cases {
		got := normalizeEntityType(input)
		if got != want {
			t.Errorf("normalizeEntityType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPermissionFor(t *testing.T) {
	if got := permissionFor("penempatan", "submit"); got != "penempatan.submit" {
		t.Errorf("got %q", got)
	}
	if got := permissionFor("ecl-parameter", "approve"); got != "ecl_parameter.approve" {
		t.Errorf("got %q, want ecl_parameter.approve", got)
	}
}

func TestParseSignatureMethod(t *testing.T) {
	if parseSignatureMethod("JWT_STEP_UP") != SignatureMethodJWTStepUp {
		t.Error("expected JWT_STEP_UP")
	}
	if parseSignatureMethod("jwt_step_up") != SignatureMethodJWTStepUp {
		t.Error("expected JWT_STEP_UP case-insensitive")
	}
	if parseSignatureMethod("JWT_STANDARD") != SignatureMethodJWTStandard {
		t.Error("expected JWT_STANDARD")
	}
	if parseSignatureMethod("") != SignatureMethodJWTStandard {
		t.Error("expected JWT_STANDARD default")
	}
}

// -----------------------------------------------------------------------
// Full 4-eyes workflow via HTTP
// -----------------------------------------------------------------------

func TestHandler_FullFourEyesFlow_HTTP(t *testing.T) {
	cfg := cfg4Eyes(false)
	svc, repo := buildTestService(map[string]*Config{cfg.EntityType: cfg})

	mUUID := uuid.New()
	rUUID := uuid.New()
	aUUID := uuid.New()
	inst := seedWorkflow(repo, cfg, mUUID)
	eid := inst.EntityID.String()

	// Submit
	rMaker := buildTestRouterWithClaims(svc, mUUID, "maker", "penempatan.submit")
	w := doPost(t, rMaker, "/api/v1/penempatan/"+eid+"/submit", map[string]any{"signatureMethod": "JWT_STANDARD"})
	if w.Code != http.StatusOK {
		t.Fatalf("submit: got %d: %s", w.Code, w.Body.String())
	}

	// Review
	rReviewer := buildTestRouterWithClaims(svc, rUUID, "reviewer", "penempatan.review")
	w = doPost(t, rReviewer, "/api/v1/penempatan/"+eid+"/review", map[string]any{"signatureMethod": "JWT_STANDARD"})
	if w.Code != http.StatusOK {
		t.Fatalf("review: got %d: %s", w.Code, w.Body.String())
	}

	// Approve
	rApprover := buildTestRouterWithClaims(svc, aUUID, "approver", "penempatan.approve")
	w = doPost(t, rApprover, "/api/v1/penempatan/"+eid+"/approve", map[string]any{"signatureMethod": "JWT_STANDARD"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve: got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["currentState"] != string(StateApproved) {
		t.Errorf("expected APPROVED, got %v", data["currentState"])
	}
	if int(data["workflowEyes"].(float64)) != 4 {
		t.Errorf("expected workflowEyes=4, got %v", data["workflowEyes"])
	}
}

// doPost is an HTTP test helper.
func doPost(t *testing.T, r *gin.Engine, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
