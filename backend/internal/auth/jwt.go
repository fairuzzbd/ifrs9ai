package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// contextKey adalah type untuk menghindari collision di context.
type contextKey struct{ name string }

var (
	// claimsKey adalah key untuk menyimpan Claims di context.
	claimsKey = contextKey{"auth_claims"}
)

// TODO(DEC-025): idle window enforcement (15-min idle → force re-auth) is not yet wired.
// When implemented, use a contextKey{"last_activity"} stored by the request middleware and
// checked by CheckPermission or a dedicated IdleCheck helper.

// IdleTimeout adalah idle window sesuai DEC-025: 15 menit.
const IdleTimeout = 15 * time.Minute

// AccessTokenTTL adalah TTL access token sesuai DEC-025: 15 menit.
const AccessTokenTTL = 15 * time.Minute

// RefreshTokenTTL adalah TTL refresh token sesuai DEC-025: 8 jam.
const RefreshTokenTTL = 8 * time.Hour

// Verifier memverifikasi JWT RSA-2048 yang diterbitkan oleh Keycloak.
// Phase 2: verify-only (full Keycloak setup = devops/integration).
type Verifier struct {
	// publicKey adalah RSA public key dari Keycloak (JWKS).
	publicKey *rsa.PublicKey
	// issuer adalah expected issuer URL dari Keycloak.
	issuer string
}

// NewVerifier membuat Verifier baru dengan RSA public key.
// publicKey harus berisi PEM-encoded RSA public key dari Keycloak JWKS.
func NewVerifier(publicKey *rsa.PublicKey, issuer string) *Verifier {
	return &Verifier{publicKey: publicKey, issuer: issuer}
}

// VerifyToken memverifikasi JWT token string dan mengembalikan Claims.
// Error codes:
//   - "UNAUTHORIZED" jika token invalid, expired, atau signature salah.
//   - "IDLE_TIMEOUT" jika token valid tapi idle > 15 menit.
func (v *Verifier) VerifyToken(tokenStr string) (*Claims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)

	var rawClaims jwt.MapClaims
	token, err := parser.ParseWithClaims(tokenStr, &rawClaims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("jwt verify: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token tidak valid")
	}

	claims, err := mapToClaims(rawClaims)
	if err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return claims, nil
}

// mapToClaims mengkonversi jwt.MapClaims ke Claims struct.
func mapToClaims(mc jwt.MapClaims) (*Claims, error) {
	// Marshal kembali ke JSON lalu unmarshal ke struct kita (safer than type assertions).
	b, err := json.Marshal(map[string]any(mc))
	if err != nil {
		return nil, fmt.Errorf("marshal claims: %w", err)
	}

	var c Claims
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	// Validasi claims wajib.
	if c.Sub == "" {
		return nil, fmt.Errorf("claim 'sub' kosong")
	}
	if c.TenantID == "" {
		return nil, fmt.Errorf("claim 'tenant_id' kosong")
	}

	return &c, nil
}

// ClaimsFromContext mengambil Claims dari context. Returns nil jika tidak ada.
func ClaimsFromContext(ctx context.Context) *Claims {
	if v, ok := ctx.Value(claimsKey).(*Claims); ok {
		return v
	}
	return nil
}

// ContextWithClaims menambahkan Claims ke context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// CheckPermission mengecek permission dari Claims di context.
// Mengembalikan error DomainError jika tidak punya permission.
// WAJIB pakai ini — JANGAN compare role string langsung.
func CheckPermission(ctx context.Context, permission string) error {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return fmt.Errorf("claims tidak ada di context: unauthorized")
	}
	if !claims.HasPermission(permission) {
		return fmt.Errorf("forbidden: permission '%s' tidak dimiliki user", permission)
	}
	return nil
}

// RequireStepUp memeriksa apakah step-up MFA masih fresh untuk action sensitif.
// Mengembalikan error jika step-up diperlukan atau sudah expired.
func RequireStepUp(ctx context.Context, action string) error {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return fmt.Errorf("claims tidak ada di context: unauthorized")
	}
	if claims.NeedsStepUp() {
		return fmt.Errorf("step_up_required: action '%s' membutuhkan step-up MFA", action)
	}
	return nil
}
