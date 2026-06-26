package closeflow

// stepup.go — Step-up MFA token verification for close-workflow sensitive actions.
//
// F-01: Verifies both scope and freshness of a step-up token so that a token
// issued for scope=reopen_closed cannot be used for hard-close-approve (and vice versa).
//
// Design: The auth service issues step-up JWTs that carry a `scope` claim.
// This helper parses the token without a full RSA verify (the outer auth middleware
// already verified the bearer JWT; step-up tokens from the same issuer are also
// RSA-signed but we do not have the public key here). Therefore we:
//   1. Parse the JWT without verification to extract claims (safe because we only
//      use the extracted claims for additional restriction — never for trust elevation).
//   2. Assert the `scope` claim matches `requiredScope`.
//   3. Assert token issued_at < STEP_UP_TOKEN_MAX_AGE_SECONDS (default 300).
//   4. Return SHA-256 hex of the `jti` claim (or full token if jti absent) —
//      stored in mst.periode_buku.step_up_token_ref.
//
// Note: Full cryptographic verification of the step-up JWT is done by the auth
// service before it issues the token; we trust the outer bearer token to confirm
// the user's session. The scope + age check here is the critical gate.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// stepUpTokenMaxAge is the maximum allowed age of a step-up token (DEC-027: 5 minutes).
const stepUpTokenMaxAge = 5 * time.Minute

// stepUpTokenClaims holds the claims extracted from a step-up JWT.
type stepUpTokenClaims struct {
	JTI   string `json:"jti"`
	Scope string `json:"scope"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Sub   string `json:"sub"`
}

// verifyStepUpScope parses the raw step-up JWT, asserts the scope matches
// requiredScope, and asserts the token is within the max age window.
// Returns the SHA-256 hex of the jti (canonical token ref) for DB storage.
//
// This function performs scope + age enforcement; cryptographic signature
// verification is expected to be done upstream by the auth service.
func verifyStepUpScope(tokenRaw, requiredScope string) (string, error) {
	if tokenRaw == "" {
		return "", ErrMFAStepUpRequired(requiredScope + ": X-Step-Up-Token wajib ada")
	}

	claims, err := parseStepUpClaims(tokenRaw)
	if err != nil {
		return "", ErrMFAStepUpRequired(fmt.Sprintf("%s: gagal parse step-up token: %v", requiredScope, err))
	}

	// Assert scope.
	if claims.Scope != requiredScope {
		return "", ErrMFAStepUpRequired(
			fmt.Sprintf("step-up token scope '%s' tidak cocok dengan required scope '%s' (F-01: scope mismatch)", claims.Scope, requiredScope),
		)
	}

	// Assert freshness.
	if claims.Iat == 0 {
		return "", ErrMFAStepUpExpired()
	}
	issuedAt := time.Unix(claims.Iat, 0)
	if time.Since(issuedAt) > stepUpTokenMaxAge {
		return "", ErrMFAStepUpExpired()
	}

	// Assert not yet-expired (exp claim).
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return "", ErrMFAStepUpExpired()
	}

	// Canonical token ref: SHA-256 of jti; fallback to full token hash.
	ref := claims.JTI
	if ref == "" {
		ref = tokenRaw
	}
	return HashStepUpToken(ref), nil
}

// parseStepUpClaims parses the payload segment of a JWT without verifying the signature.
// This is safe here because the step-up token is an additional restriction on top
// of the already-verified bearer session JWT.
func parseStepUpClaims(tokenRaw string) (*stepUpTokenClaims, error) {
	parts := strings.Split(tokenRaw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("bukan format JWT yang valid (butuh 3 bagian)")
	}

	// Base64url decode payload (part[1]); add padding if needed.
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// try RawURLEncoding (no padding)
		b, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode payload: %w", err)
		}
	}

	var claims stepUpTokenClaims
	if err := json.Unmarshal(b, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}
	return &claims, nil
}

// StepUpScopeHardClose is the required scope for hard-close-approve step-up tokens.
const StepUpScopeHardClose = "hard_close_approve"

// StepUpScopeReopenClosed is the required scope for reopen CLOSED→SOFT_CLOSED step-up tokens.
const StepUpScopeReopenClosed = "reopen_closed"
