//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestSoDBypass_MakerApproverViaAPI is the end-to-end SoD bypass test from
// security-baseline.md:
//
// "Maker mencoba menjadi Approver lewat API langsung dengan JWT valid"
//
// The test issues real HTTP requests through the Gin router (with auth middleware
// bypassed to inject a crafted JWT), proceeds through SUBMIT + REVIEW, then
// attempts APPROVE with the maker's user ID and valid JWT that has the
// penempatan.approve permission. The service layer MUST return 403 SOD_VIOLATION.
//
// This is the WAJIB security-baseline test.
func TestSoDBypass_MakerApproverViaAPI(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "e2e_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "e2e_sod_reviewer")
	entityID := uuid.New()

	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	// Build full HTTP stack with real DB.
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	auditWriter := audit.NewWriter(infra.DB)
	svc := workflow.NewService(engine, repo, auditWriter, nil)
	h := workflow.NewHandler(svc)

	// Register routes (no real JWT verifier — use test claim injector).
	apiGroup := router.Group("/api/v1")
	apiGroup.Use(testClaimsMiddleware) // inject claims from X-Test-Claims header
	workflow.RegisterRoutes(apiGroup, h)

	// --- Helper: build a test request ---
	post := func(path, claimsJSON, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, path,
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.New().String())
		req.Header.Set("X-Test-Claims", claimsJSON) // injected by testClaimsMiddleware
		router.ServeHTTP(w, req)
		return w
	}

	makeClaimsJSON := func(userID uuid.UUID, permissions ...string) string {
		now := time.Now().Unix()
		c := auth.Claims{
			Sub:               userID.String(),
			PreferredUsername: "testuser",
			Roles:             []string{"ROLE-MAKER-TR"},
			Permissions:       permissions,
			TenantID:          "TUGURE",
			MFAVerified:       true,
			Exp:               now + 3600,
			Iat:               now,
		}
		b, _ := json.Marshal(c)
		return string(b)
	}

	rv1 := fmt.Sprintf(`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	rv2 := fmt.Sprintf(`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	rv3 := fmt.Sprintf(`{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)

	// STEP 1: SUBMIT as maker.
	makerClaims := makeClaimsJSON(makerID,
		"penempatan.submit", "penempatan.review", "penempatan.approve", "penempatan.read")
	w1 := post("/api/v1/penempatan/"+entityID.String()+"/submit", makerClaims, rv1)
	if w1.Code != http.StatusOK {
		t.Fatalf("SUBMIT: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("SUBMIT: OK state=PENDING_REVIEW")

	// STEP 2: REVIEW as reviewer.
	reviewerClaims := makeClaimsJSON(reviewerID,
		"penempatan.review", "penempatan.approve")
	w2 := post("/api/v1/penempatan/"+entityID.String()+"/review", reviewerClaims, rv2)
	if w2.Code != http.StatusOK {
		t.Fatalf("REVIEW: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	t.Logf("REVIEW: OK state=PENDING_APPROVAL")

	// STEP 3: APPROVE attempt as MAKER (same user as step 1).
	// The maker's JWT HAS penempatan.approve permission — bypass attempt.
	// Expected: 403 SOD_VIOLATION.
	w3 := post("/api/v1/penempatan/"+entityID.String()+"/approve", makerClaims, rv3)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD bypass test FAILED: expected 403 SOD_VIOLATION when maker tries to approve, got %d body=%s",
			w3.Code, w3.Body.String())
	} else {
		var errResp map[string]any
		if err := json.Unmarshal(w3.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("parse 403 response: %v", err)
		}
		errObj, _ := errResp["error"].(map[string]any)
		code, _ := errObj["code"].(string)
		if code != "SOD_VIOLATION" {
			t.Errorf("expected error code SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD bypass blocked correctly: code=%s msg=%v", code, errObj["message"])
	}

	// Confirm workflow is still in PENDING_APPROVAL (not tampered to APPROVED).
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// TestSoDBypass_ReviewerApproverViaAPI verifies that the reviewer cannot
// also be the approver.
func TestSoDBypass_ReviewerApproverViaAPI(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "e2e_sod2_maker")
	reviewerID := seedUserSQL(t, infra.DB, "e2e_sod2_reviewer")
	entityID := uuid.New()

	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	svc := workflow.NewService(engine, repo, nil, nil)
	h := workflow.NewHandler(svc)

	apiGroup := router.Group("/api/v1")
	apiGroup.Use(testClaimsMiddleware)
	workflow.RegisterRoutes(apiGroup, h)

	post := func(path, claimsJSON, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.New().String())
		req.Header.Set("X-Test-Claims", claimsJSON)
		router.ServeHTTP(w, req)
		return w
	}

	makeClaimsJSON := func(userID uuid.UUID, perms ...string) string {
		now := time.Now().Unix()
		c := auth.Claims{
			Sub: userID.String(), PreferredUsername: "u",
			Roles: []string{"ROLE-APPR-TR"}, Permissions: perms,
			TenantID: "TUGURE", MFAVerified: true,
			Exp: now + 3600, Iat: now,
		}
		b, _ := json.Marshal(c)
		return string(b)
	}

	// Submit.
	makerClaims := makeClaimsJSON(makerID, "penempatan.submit")
	w1 := post("/api/v1/penempatan/"+entityID.String()+"/submit",
		makerClaims, `{"rowVersion":1}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("SUBMIT: %d %s", w1.Code, w1.Body)
	}

	// Review.
	reviewerClaims := makeClaimsJSON(reviewerID, "penempatan.review", "penempatan.approve")
	w2 := post("/api/v1/penempatan/"+entityID.String()+"/review",
		reviewerClaims, `{"rowVersion":2}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("REVIEW: %d %s", w2.Code, w2.Body)
	}

	// Reviewer tries to APPROVE — must be blocked.
	w3 := post("/api/v1/penempatan/"+entityID.String()+"/approve",
		reviewerClaims, `{"rowVersion":3}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("reviewer-as-approver: expected 403, got %d body=%s", w3.Code, w3.Body.String())
	} else {
		t.Logf("reviewer-as-approver blocked correctly: 403")
	}
}

// testClaimsMiddleware is a Gin middleware that reads X-Test-Claims header
// and injects auth.Claims into the Gin and request context.
// ONLY for integration testing — never use in production code.
var testClaimsMiddleware gin.HandlerFunc = func(c *gin.Context) {
	raw := c.GetHeader("X-Test-Claims")
	if raw == "" {
		c.Next()
		return
	}

	var claims auth.Claims
	if err := json.Unmarshal([]byte(raw), &claims); err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"error": map[string]any{"code": "UNAUTHORIZED", "message": "bad test claims"},
		})
		return
	}

	c.Set("userId", claims.Sub)
	c.Set("tenantId", claims.TenantID)
	c.Set("roles", claims.Roles)
	c.Set("permissions", claims.Permissions)
	c.Set("mfaVerified", claims.MFAVerified)
	c.Set("claims", &claims)

	ctx := auth.ContextWithClaims(c.Request.Context(), &claims)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}
