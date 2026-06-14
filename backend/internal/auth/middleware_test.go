package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestVerifier(t *testing.T) (*auth.Verifier, *rsa.PrivateKey) {
	t.Helper()
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return auth.NewVerifier(&pk.PublicKey, "http://test.local/realms/blips"), pk
}

func makeToken(t *testing.T, pk *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	str, err := token.SignedString(pk)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return str
}

func validTokenClaims() map[string]any {
	now := time.Now()
	return map[string]any{
		"sub":                "550e8400-e29b-41d4-a716-446655440001",
		"preferred_username": "budi.santoso",
		"roles":              []string{"ROLE-MAKER-TR"},
		"permissions":        []string{"instrumen.create", "instrumen.read"},
		"tenant_id":          "TUGURE",
		"mfa_verified":       false,
		"exp":                now.Add(15 * time.Minute).Unix(),
		"iat":                now.Unix(),
		"nbf":                now.Unix(),
	}
}

// TestAuthMiddleware_ValidToken verifies valid token sets claims in context.
func TestAuthMiddleware_ValidToken(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))

	router.GET("/test", func(c *gin.Context) {
		claims := auth.ClaimsFromGin(c)
		if claims == nil {
			t.Error("expected claims in context")
			c.Status(500)
			return
		}
		c.JSON(200, gin.H{"sub": claims.Sub})
	})

	tokenStr := makeToken(t, pk, validTokenClaims())
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAuthMiddleware_MissingToken verifies missing token returns 401.
func TestAuthMiddleware_MissingToken(t *testing.T) {
	verifier, _ := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.GET("/test", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestAuthMiddleware_ExpiredToken verifies expired token returns 401.
func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.GET("/test", func(c *gin.Context) { c.Status(200) })

	claims := validTokenClaims()
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	claims["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

// TestAuthMiddleware_MalformedToken verifies malformed token returns 401.
func TestAuthMiddleware_MalformedToken(t *testing.T) {
	verifier, _ := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.GET("/test", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.jwt.token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401 for malformed token, got %d", w.Code)
	}
}

// TestRequirePermission_Allow verifies request with correct permission passes.
func TestRequirePermission_Allow(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.GET("/test", auth.RequirePermission("instrumen.create"), func(c *gin.Context) {
		c.Status(200)
	})

	tokenStr := makeToken(t, pk, validTokenClaims())
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRequirePermission_Deny verifies request without permission returns 403.
func TestRequirePermission_Deny(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	// Require permission that user doesn't have.
	router.POST("/test", auth.RequirePermission("ecl_parameter.approve"), func(c *gin.Context) {
		c.Status(200)
	})

	tokenStr := makeToken(t, pk, validTokenClaims())
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRequirePermission_NoRoleStringComparison ensures role strings are NOT matched.
// This test verifies the security baseline rule: permission check is {entity}.{action} only.
func TestRequirePermission_NoRoleStringComparison(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))

	// Check for role string — must NOT be matched by RequirePermission.
	router.GET("/test", auth.RequirePermission("ROLE-MAKER-TR"), func(c *gin.Context) {
		c.Status(200)
	})

	// User has ROLE-MAKER-TR in roles[] but NOT in permissions[].
	claims := validTokenClaims()
	claims["roles"] = []string{"ROLE-MAKER-TR"}
	claims["permissions"] = []string{"instrumen.read"} // no ROLE-MAKER-TR here
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Must be 403 — role string "ROLE-MAKER-TR" is NOT in permissions[].
	// If 200, implementation uses role-string comparison — FORBIDDEN anti-pattern.
	if w.Code == 200 {
		t.Fatal("FATAL: permission check matched role string — this violates security-baseline.md. " +
			"RequirePermission must ONLY check permissions[], not roles[].")
	}
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestRequireStepUp_Required verifies step-up required when not done.
func TestRequireStepUp_Required(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.POST("/hard-close", auth.RequireStepUpMiddleware("PERIODE_HARD_CLOSE"), func(c *gin.Context) {
		c.Status(200)
	})

	claims := validTokenClaims()
	// No stepup_verified_at — step-up required.
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodPost, "/hard-close", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 STEP_UP_REQUIRED, got %d", w.Code)
	}
}

// TestRequireStepUp_FreshStepup verifies fresh step-up allows access.
func TestRequireStepUp_FreshStepup(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.POST("/hard-close", auth.RequireStepUpMiddleware("PERIODE_HARD_CLOSE"), func(c *gin.Context) {
		c.Status(200)
	})

	claims := validTokenClaims()
	// Fresh step-up (1 minute ago).
	claims["stepup_verified_at"] = time.Now().Add(-1 * time.Minute).Unix()
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodPost, "/hard-close", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200 with fresh step-up, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRequireMFA_NotVerified verifies MFA not verified returns 403.
func TestRequireMFA_NotVerified(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.POST("/mfa-required", auth.RequireMFA(), func(c *gin.Context) {
		c.Status(200)
	})

	claims := validTokenClaims()
	claims["mfa_verified"] = false
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodPost, "/mfa-required", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 MFA_REQUIRED, got %d", w.Code)
	}
}

// TestRequireMFA_Verified verifies MFA verified passes.
func TestRequireMFA_Verified(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.GET("/mfa-ok", auth.RequireMFA(), func(c *gin.Context) {
		c.Status(200)
	})

	claims := validTokenClaims()
	claims["mfa_verified"] = true
	claims["mfa_method"] = "TOTP"
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodGet, "/mfa-ok", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestClaimsFromGin_NoClaims verifies nil returned when no claims.
func TestClaimsFromGin_NoClaims(t *testing.T) {
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		claims := auth.ClaimsFromGin(c)
		if claims != nil {
			t.Error("expected nil claims when not set")
		}
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
}

// TestRequireStepUp_ExpiredStepup verifies expired step-up returns 403.
func TestRequireStepUp_ExpiredStepup(t *testing.T) {
	verifier, pk := newTestVerifier(t)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(auth.Middleware(verifier))
	router.POST("/hard-close", auth.RequireStepUpMiddleware("PERIODE_HARD_CLOSE"), func(c *gin.Context) {
		c.Status(200)
	})

	claims := validTokenClaims()
	// Stale step-up (10 minutes ago).
	claims["stepup_verified_at"] = time.Now().Add(-10 * time.Minute).Unix()
	tokenStr := makeToken(t, pk, claims)

	req := httptest.NewRequest(http.MethodPost, "/hard-close", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403 STEP_UP_EXPIRED, got %d", w.Code)
	}
}
