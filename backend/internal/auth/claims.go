// Package auth menyediakan JWT verification, RBAC permission check, dan SoD enforcement.
//
// Prinsip keamanan utama:
// 1. Permission check SELALU pakai {entity}.{action} string dari JWT claims.permissions[].
//    DILARANG KERAS role-string comparison (if role == "CFO") — red flag security-baseline.
// 2. SoD (Segregation of Duties): maker ≠ reviewer ≠ approver, enforced di service layer.
// 3. Step-up MFA: action DEC-027 re-prompt jika stepup_verified_at > 5 menit.
package auth

import (
	"time"
)

// Claims adalah JWT claims canonical sesuai security-baseline.md.
// Field names menggunakan JSON snake_case sesuai Keycloak convention.
type Claims struct {
	// Standard JWT claims.
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
	Nbf int64  `json:"nbf"`

	// BLIPS custom claims.
	PreferredUsername string   `json:"preferred_username"`
	Roles             []string `json:"roles"`
	Permissions       []string `json:"permissions"`
	TenantID          string   `json:"tenant_id"`
	MFAVerified       bool     `json:"mfa_verified"`
	MFAMethod         string   `json:"mfa_method,omitempty"`
	// StepupVerifiedAt adalah Unix timestamp step-up terakhir.
	// Nil jika step-up belum pernah dilakukan dalam session ini.
	StepupVerifiedAt *int64 `json:"stepup_verified_at,omitempty"`
}

// HasPermission mengecek apakah user memiliki permission {entity}.{action}.
// WAJIB pakai method ini — JANGAN compare role string secara langsung.
func (c *Claims) HasPermission(permission string) bool {
	for _, p := range c.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// NeedsStepUp mengecek apakah step-up MFA diperlukan.
// Returns true jika stepup_verified_at nil atau > 5 menit yang lalu (DEC-027).
func (c *Claims) NeedsStepUp() bool {
	if c.StepupVerifiedAt == nil {
		return true
	}
	stepupTime := time.Unix(*c.StepupVerifiedAt, 0)
	return time.Since(stepupTime) > 5*time.Minute
}

// IsStepUpFresh mengecek apakah step-up masih valid (< 5 menit).
func (c *Claims) IsStepUpFresh() bool {
	return !c.NeedsStepUp()
}

// UserID mengembalikan user UUID dari sub claim.
func (c *Claims) UserID() string {
	return c.Sub
}

// IsExpired mengecek apakah token sudah expired.
func (c *Claims) IsExpired() bool {
	return time.Now().Unix() > c.Exp
}

// IsNotYetValid mengecek apakah token belum valid (nbf).
func (c *Claims) IsNotYetValid() bool {
	return c.Nbf > 0 && time.Now().Unix() < c.Nbf
}
