package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// generateTestKeyPair membuat pasangan RSA key untuk testing.
func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pk, &pk.PublicKey
}

// signToken membuat JWT yang ditandatangani dengan key yang diberikan.
func signToken(t *testing.T, pk *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	str, err := token.SignedString(pk)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return str
}

// validClaims mengembalikan claims JWT yang valid untuk testing.
func validClaims(extraTTL time.Duration) map[string]any {
	now := time.Now()
	return map[string]any{
		"sub":                "550e8400-e29b-41d4-a716-446655440001",
		"preferred_username": "budi.santoso",
		"roles":              []string{"ROLE-MAKER-TR"},
		"permissions":        []string{"instrumen.create", "instrumen.read", "penempatan.submit"},
		"tenant_id":          "TUGURE",
		"mfa_verified":       false,
		"exp":                now.Add(15*time.Minute + extraTTL).Unix(),
		"iat":                now.Unix(),
		"nbf":                now.Unix(),
		"iss":                "http://localhost:8080/realms/blips",
	}
}

// TestVerifyToken_Valid memverifikasi token valid dapat di-parse.
func TestVerifyToken_Valid(t *testing.T) {
	pk, pub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(pub, "http://localhost:8080/realms/blips")

	tokenStr := signToken(t, pk, validClaims(0))
	claims, err := verifier.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if claims.Sub != "550e8400-e29b-41d4-a716-446655440001" {
		t.Errorf("sub mismatch: %s", claims.Sub)
	}
	if claims.TenantID != "TUGURE" {
		t.Errorf("tenant_id mismatch: %s", claims.TenantID)
	}
	if len(claims.Permissions) != 3 {
		t.Errorf("expected 3 permissions, got %d", len(claims.Permissions))
	}
}

// TestVerifyToken_Expired memverifikasi token expired menghasilkan error.
func TestVerifyToken_Expired(t *testing.T) {
	pk, pub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(pub, "http://localhost:8080/realms/blips")

	expiredClaims := validClaims(0)
	// Set exp ke masa lalu.
	expiredClaims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	expiredClaims["iat"] = time.Now().Add(-2 * time.Hour).Unix()

	tokenStr := signToken(t, pk, expiredClaims)
	_, err := verifier.VerifyToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

// TestVerifyToken_WrongKey memverifikasi token dengan key berbeda menghasilkan error.
func TestVerifyToken_WrongKey(t *testing.T) {
	pk, _ := generateTestKeyPair(t)
	_, wrongPub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(wrongPub, "http://localhost:8080/realms/blips")

	tokenStr := signToken(t, pk, validClaims(0))
	_, err := verifier.VerifyToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for wrong key, got nil")
	}
}

// TestVerifyToken_InvalidFormat memverifikasi string random menghasilkan error.
func TestVerifyToken_InvalidFormat(t *testing.T) {
	_, pub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(pub, "http://localhost:8080/realms/blips")

	_, err := verifier.VerifyToken("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for invalid JWT, got nil")
	}
}

// TestClaims_HasPermission verifies permission check does NOT use role strings.
func TestClaims_HasPermission(t *testing.T) {
	claims := &auth.Claims{
		Sub:         "user-1",
		TenantID:    "TUGURE",
		Permissions: []string{"instrumen.create", "instrumen.read"},
		Roles:       []string{"ROLE-MAKER-TR"},
	}

	// Permission check — pakai {entity}.{action}, BUKAN role string.
	if !claims.HasPermission("instrumen.create") {
		t.Error("should have instrumen.create")
	}
	if !claims.HasPermission("instrumen.read") {
		t.Error("should have instrumen.read")
	}
	// Permission yang tidak ada.
	if claims.HasPermission("instrumen.delete") {
		t.Error("should NOT have instrumen.delete")
	}
	// Role string comparison adalah FORBIDDEN anti-pattern — test harus pakai HasPermission.
	// Verifikasi bahwa role string tidak di-check di HasPermission.
	if claims.HasPermission("ROLE-MAKER-TR") {
		t.Error("HasPermission should NOT match role strings — use {entity}.{action} only")
	}
}

// TestClaims_StepUp verifies step-up MFA expiry logic.
func TestClaims_StepUp(t *testing.T) {
	t.Run("no stepup: needs stepup", func(t *testing.T) {
		claims := &auth.Claims{}
		if !claims.NeedsStepUp() {
			t.Error("should need step-up when stepup_verified_at is nil")
		}
	})

	t.Run("fresh stepup: no stepup needed", func(t *testing.T) {
		now := time.Now().Add(-1 * time.Minute).Unix() // 1 menit lalu
		claims := &auth.Claims{StepupVerifiedAt: &now}
		if claims.NeedsStepUp() {
			t.Error("should NOT need step-up when < 5 minutes ago")
		}
	})

	t.Run("stale stepup: needs stepup", func(t *testing.T) {
		old := time.Now().Add(-10 * time.Minute).Unix() // 10 menit lalu
		claims := &auth.Claims{StepupVerifiedAt: &old}
		if !claims.NeedsStepUp() {
			t.Error("should need step-up when > 5 minutes ago")
		}
	})

	t.Run("exactly 5 minutes: needs stepup", func(t *testing.T) {
		fiveMinAgo := time.Now().Add(-5 * time.Minute).Unix()
		claims := &auth.Claims{StepupVerifiedAt: &fiveMinAgo}
		if !claims.NeedsStepUp() {
			t.Error("should need step-up when exactly 5 minutes ago (boundary)")
		}
	})
}

// TestContextWithClaims verifies claims round-trip through context.
func TestContextWithClaims(t *testing.T) {
	claims := &auth.Claims{
		Sub:         "user-abc",
		TenantID:    "TUGURE",
		Permissions: []string{"instrumen.read"},
	}

	ctx := auth.ContextWithClaims(context.Background(), claims)
	got := auth.ClaimsFromContext(ctx)
	if got == nil {
		t.Fatal("expected claims in context, got nil")
	}
	if got.Sub != "user-abc" {
		t.Errorf("sub mismatch: %s", got.Sub)
	}
}

// TestContextWithClaims_NotFound verifies nil returned when not in context.
func TestContextWithClaims_NotFound(t *testing.T) {
	got := auth.ClaimsFromContext(context.Background())
	if got != nil {
		t.Error("expected nil when claims not in context")
	}
}

// TestCheckPermission_Allow verifies permission check passes when permission exists.
func TestCheckPermission_Allow(t *testing.T) {
	claims := &auth.Claims{
		Sub:         "user-1",
		TenantID:    "TUGURE",
		Permissions: []string{"instrumen.create"},
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	if err := auth.CheckPermission(ctx, "instrumen.create"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestCheckPermission_Deny verifies permission check fails when permission missing.
func TestCheckPermission_Deny(t *testing.T) {
	claims := &auth.Claims{
		Sub:         "user-1",
		TenantID:    "TUGURE",
		Permissions: []string{"instrumen.read"},
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	if err := auth.CheckPermission(ctx, "instrumen.create"); err == nil {
		t.Error("expected error for missing permission, got nil")
	}
}

// TestCheckPermission_NoRoleStringComparison documents the anti-pattern.
// Permission check must NEVER succeed based on role strings.
func TestCheckPermission_NoRoleStringComparison(t *testing.T) {
	// User has role ROLE-CFO but permission list is empty.
	// CheckPermission should DENY — no role-string comparison.
	claims := &auth.Claims{
		Sub:         "user-cfo",
		TenantID:    "TUGURE",
		Roles:       []string{"ROLE-CFO"},
		Permissions: []string{}, // intentionally empty
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	// If implementation used role-string comparison ("ROLE-CFO" → allow),
	// this would pass. It MUST NOT pass.
	if err := auth.CheckPermission(ctx, "periode.hardclose"); err == nil {
		t.Error("FATAL: permission check passed based on role string — this is FORBIDDEN per security-baseline.md")
	}
}

// TestVerifyToken_HS256Rejected ensures HMAC-signed tokens are rejected (only RS256 allowed).
func TestVerifyToken_HS256Rejected(t *testing.T) {
	_, pub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(pub, "http://localhost:8080/realms/blips")

	// Sign with HMAC — should be rejected.
	hmacClaims := jwt.MapClaims(validClaims(0))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, hmacClaims)
	tokenStr, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign HS256: %v", err)
	}

	_, err = verifier.VerifyToken(tokenStr)
	if err == nil {
		t.Fatal("HS256 token should be REJECTED — only RSA methods allowed (DEC-025)")
	}
}

// TestVerifyToken_IdleTimeout_Simulation documents idle timeout behavior.
// Note: actual idle timeout enforcement is at the application layer (last_activity tracking),
// not purely at JWT level. This test documents the expected behavior.
func TestVerifyToken_IdleTimeout_Simulation(t *testing.T) {
	// A token with valid exp but stepup_verified_at that's old simulates idle behavior.
	pk, pub := generateTestKeyPair(t)
	verifier := auth.NewVerifier(pub, "http://localhost:8080/realms/blips")

	tokenClaims := validClaims(0)
	oldStepup := time.Now().Add(-20 * time.Minute).Unix()
	tokenClaims["stepup_verified_at"] = oldStepup

	tokenStr := signToken(t, pk, tokenClaims)
	claims, err := verifier.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("token verification: %v", err)
	}

	// Token is valid, but step-up has expired.
	if claims.IsStepUpFresh() {
		t.Error("step-up should be expired after 20 minutes")
	}
	if !claims.NeedsStepUp() {
		t.Error("step-up should be required")
	}
}

// TestClaims_UserID verifies UserID returns sub.
func TestClaims_UserID(t *testing.T) {
	claims := &auth.Claims{Sub: "user-uuid-abc"}
	if claims.UserID() != "user-uuid-abc" {
		t.Errorf("expected user-uuid-abc, got %s", claims.UserID())
	}
}

// TestClaims_IsExpired verifies IsExpired.
func TestClaims_IsExpired(t *testing.T) {
	expired := &auth.Claims{Exp: time.Now().Add(-1 * time.Hour).Unix()}
	if !expired.IsExpired() {
		t.Error("should be expired")
	}
	valid := &auth.Claims{Exp: time.Now().Add(15 * time.Minute).Unix()}
	if valid.IsExpired() {
		t.Error("should not be expired")
	}
}

// TestClaims_IsNotYetValid verifies IsNotYetValid.
func TestClaims_IsNotYetValid(t *testing.T) {
	future := &auth.Claims{Nbf: time.Now().Add(5 * time.Minute).Unix()}
	if !future.IsNotYetValid() {
		t.Error("should not be valid yet")
	}
	past := &auth.Claims{Nbf: time.Now().Add(-5 * time.Minute).Unix()}
	if past.IsNotYetValid() {
		t.Error("should be valid now")
	}
	zero := &auth.Claims{Nbf: 0}
	if zero.IsNotYetValid() {
		t.Error("zero Nbf should not trigger not-yet-valid")
	}
}

// jsonRoundTrip marshals and unmarshals to simulate what VerifyToken does internally.
func jsonRoundTrip(t *testing.T, v any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(v)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

// TestRequireStepUp_WithoutStepup verifies error when no step-up in context.
func TestRequireStepUp_WithoutStepup(t *testing.T) {
	claims := &auth.Claims{
		Sub:              "user-1",
		TenantID:         "TUGURE",
		StepupVerifiedAt: nil, // no step-up
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	err := auth.RequireStepUp(ctx, "PERIODE_HARD_CLOSE")
	if err == nil {
		t.Error("expected error when step-up not done, got nil")
	}
	if fmt.Sprintf("%v", err) == "" {
		t.Error("error message should not be empty")
	}
}

// TestRequireStepUp_Fresh verifies no error when step-up is fresh.
func TestRequireStepUp_Fresh(t *testing.T) {
	now := time.Now().Add(-1 * time.Minute).Unix()
	claims := &auth.Claims{
		Sub:              "user-1",
		TenantID:         "TUGURE",
		StepupVerifiedAt: &now,
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	err := auth.RequireStepUp(ctx, "PERIODE_HARD_CLOSE")
	if err != nil {
		t.Errorf("expected no error for fresh step-up, got %v", err)
	}
}
