package mappingjurnal

// stepup.go — Step-up MFA token verification for mapping jurnal approve-2 (6-eyes regulated path).
//
// Mirrors internal/periode/closeflow/stepup.go pattern (DEC-027).
// Parses the step-up JWT payload without full RSA verification (the outer bearer JWT
// was already verified by auth middleware). Asserts:
//   1. scope == requiredScope (prevents cross-action token reuse)
//   2. iat within 5 minutes (freshness window per DEC-027)
//   3. exp not yet passed (if present)
//
// Returns hex SHA-256 of the jti (or full token if jti absent) for storage in step_up_token_ref.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// StepUpScopeMappingApprove is the required scope for mapping jurnal approve-2 step-up tokens.
const StepUpScopeMappingApprove = "mapping_approve"

// mappingStepUpMaxAge is the freshness window (DEC-027: 5 minutes).
const mappingStepUpMaxAge = 5 * time.Minute

// mappingStepUpClaims holds the relevant JWT payload claims.
type mappingStepUpClaims struct {
	JTI   string `json:"jti"`
	Scope string `json:"scope"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Sub   string `json:"sub"`
}

// verifyMappingStepUp parses a raw step-up JWT, asserts scope == StepUpScopeMappingApprove,
// and asserts the token was issued within mappingStepUpMaxAge.
// Returns a hex SHA-256 token reference suitable for DB storage (never stores the raw token).
// Returns ErrMFAStepUpRequired on missing / scope-mismatch; ErrMFAStepUpExpired on age violation.
func verifyMappingStepUp(tokenRaw string) (tokenRef []byte, err error) {
	if strings.TrimSpace(tokenRaw) == "" {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpRequired,
			"approve-2 pada event terregulasi wajib menyertakan X-Step-Up-Token header dengan scope '"+StepUpScopeMappingApprove+"' (DEC-027). "+
				"Lakukan challenge via POST /auth/step-up terlebih dahulu.")
	}

	claims, parseErr := parseMappingStepUpClaims(tokenRaw)
	if parseErr != nil {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpRequired,
			fmt.Sprintf("Gagal parse X-Step-Up-Token: %v. Pastikan token diperoleh dari POST /auth/step-up.", parseErr))
	}

	if claims.Scope != StepUpScopeMappingApprove {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpRequired,
			fmt.Sprintf("X-Step-Up-Token scope '%s' tidak sesuai dengan required scope '%s' (F-01: cross-action token reuse dilarang).",
				claims.Scope, StepUpScopeMappingApprove))
	}

	if claims.Iat == 0 {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpExpired,
			"X-Step-Up-Token tidak memiliki klaim 'iat'. Token tidak valid.")
	}
	issuedAt := time.Unix(claims.Iat, 0)
	if time.Since(issuedAt) > mappingStepUpMaxAge {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpExpired,
			fmt.Sprintf("X-Step-Up-Token sudah kadaluarsa (issued %s, maksimal 5 menit). "+
				"Ulangi POST /auth/step-up.", issuedAt.UTC().Format(time.RFC3339)))
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, domainerrors.New(domainerrors.CodeMFAStepUpExpired,
			"X-Step-Up-Token sudah melewati waktu exp. Ulangi POST /auth/step-up.")
	}

	ref := claims.JTI
	if ref == "" {
		ref = tokenRaw
	}
	return computeSHA256(ref), nil
}

// parseMappingStepUpClaims decodes the payload segment of a JWT without verifying the signature.
func parseMappingStepUpClaims(tokenRaw string) (*mappingStepUpClaims, error) {
	parts := strings.Split(tokenRaw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("bukan format JWT yang valid (butuh 3 bagian)")
	}

	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	b, decErr := base64.URLEncoding.DecodeString(payload)
	if decErr != nil {
		b, decErr = base64.RawURLEncoding.DecodeString(parts[1])
		if decErr != nil {
			return nil, fmt.Errorf("decode payload: %w", decErr)
		}
	}

	var claims mappingStepUpClaims
	if err := json.Unmarshal(b, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}
	return &claims, nil
}
