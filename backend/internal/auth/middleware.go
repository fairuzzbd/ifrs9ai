package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Middleware mengembalikan Gin middleware yang memverifikasi JWT Bearer token.
// Jika token valid, Claims di-set ke Gin context dan request context.
// Jika tidak, return 401 UNAUTHORIZED dan abort.
func Middleware(v *Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortUnauthorized(c, "Token JWT tidak ditemukan. Sertakan 'Authorization: Bearer <token>'.")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			abortUnauthorized(c, "Format Authorization header tidak valid. Gunakan 'Bearer <token>'.")
			return
		}

		tokenStr := parts[1]
		claims, err := v.VerifyToken(tokenStr)
		if err != nil {
			errMsg := classifyJWTError(err)
			abortWithCode(c, errMsg.code, errMsg.msg)
			return
		}

		// Set claims ke Gin context.
		c.Set("userId", claims.Sub)
		c.Set("tenantId", claims.TenantID)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
		c.Set("mfaVerified", claims.MFAVerified)
		c.Set("claims", claims)

		// Set claims ke request context untuk service/repo layer.
		ctx := ContextWithClaims(c.Request.Context(), claims)
		// Inject client IP + User-Agent untuk audit trail (security-baseline.md §"Audit trail").
		// c.ClientIP() respects X-Forwarded-For (Traefik injects this).
		ctx = middleware.ContextWithIPUA(ctx, c.ClientIP(), c.Request.UserAgent())
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

type errInfo struct {
	code string
	msg  string
}

// classifyJWTError mengklasifikasikan error JWT menjadi error code yang sesuai.
func classifyJWTError(err error) errInfo {
	s := err.Error()
	if strings.Contains(s, "expired") || strings.Contains(s, "exp") {
		return errInfo{code: "UNAUTHORIZED", msg: "Token JWT sudah expired. Gunakan /auth/refresh untuk memperbarui."}
	}
	if strings.Contains(s, "idle") || strings.Contains(s, "IDLE") {
		return errInfo{code: "IDLE_TIMEOUT", msg: "Sesi idle lebih dari 15 menit. Silakan login kembali."}
	}
	return errInfo{code: "UNAUTHORIZED", msg: "Token JWT tidak valid."}
}

// abortUnauthorized menulis 401 error envelope dan abort chain.
func abortUnauthorized(c *gin.Context, message string) {
	traceID, _ := c.Get(response.TraceIDKey)
	c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
		"error": map[string]any{
			"code":    "UNAUTHORIZED",
			"message": message,
			"details": []any{},
			"traceId": traceID,
		},
	})
}

// abortWithCode menulis error envelope dengan code eksplisit dan abort.
func abortWithCode(c *gin.Context, code, message string) {
	traceID, _ := c.Get(response.TraceIDKey)
	status := http.StatusUnauthorized
	if code == "IDLE_TIMEOUT" {
		status = http.StatusUnauthorized
	}
	c.AbortWithStatusJSON(status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": []any{},
			"traceId": traceID,
		},
	})
}

// RequirePermission mengembalikan Gin middleware yang mengecek permission spesifik.
// Gunakan di route group atau individual route, SETELAH Middleware().
//
// WAJIB: permission adalah string {entity}.{action}, BUKAN role string.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.Get("claims")
		if !ok {
			abortUnauthorized(c, "Claims tidak ditemukan. Auth middleware mungkin tidak terpasang.")
			return
		}

		cl, ok := claims.(*Claims)
		if !ok || cl == nil {
			abortUnauthorized(c, "Claims tidak valid.")
			return
		}

		if !cl.HasPermission(permission) {
			traceID, _ := c.Get(response.TraceIDKey)
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"error": map[string]any{
					"code":    "FORBIDDEN",
					"message": "Anda tidak memiliki permission '" + permission + "'.",
					"details": []any{},
					"traceId": traceID,
				},
			})
			return
		}

		c.Next()
	}
}

// RequireMFA mengembalikan Gin middleware yang memastikan mfa_verified = true.
func RequireMFA() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.Get("claims")
		if !ok {
			abortUnauthorized(c, "Claims tidak ditemukan.")
			return
		}
		cl, ok := claims.(*Claims)
		if !ok || !cl.MFAVerified {
			traceID, _ := c.Get(response.TraceIDKey)
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"error": map[string]any{
					"code":    "MFA_REQUIRED",
					"message": "Action ini membutuhkan verifikasi MFA.",
					"details": []any{},
					"traceId": traceID,
				},
			})
			return
		}
		c.Next()
	}
}

// RequireStepUpMiddleware mengembalikan Gin middleware yang memastikan step-up MFA fresh.
// Dipakai untuk endpoint DEC-027: hard-close, ECL param approve, klasifikasi approve, calc-run seal.
func RequireStepUpMiddleware(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.Get("claims")
		if !ok {
			abortUnauthorized(c, "Claims tidak ditemukan.")
			return
		}
		cl, ok := claims.(*Claims)
		if !ok || cl == nil {
			abortUnauthorized(c, "Claims tidak valid.")
			return
		}

		traceID, _ := c.Get(response.TraceIDKey)
		if cl.StepupVerifiedAt == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"error": map[string]any{
					"code":    "STEP_UP_REQUIRED",
					"message": "Action '" + action + "' membutuhkan step-up MFA. Hubungi /auth/step-up.",
					"details": []any{},
					"traceId": traceID,
				},
			})
			return
		}
		if cl.NeedsStepUp() {
			c.AbortWithStatusJSON(http.StatusForbidden, map[string]any{
				"error": map[string]any{
					"code":    "STEP_UP_EXPIRED",
					"message": "Step-up MFA sudah expired (> 5 menit). Ulangi /auth/step-up.",
					"details": []any{},
					"traceId": traceID,
				},
			})
			return
		}

		c.Next()
	}
}

// ClaimsFromGin mengambil *Claims dari Gin context.
// Convenience method untuk dipakai di handler.
func ClaimsFromGin(c *gin.Context) *Claims {
	if v, ok := c.Get("claims"); ok {
		if cl, ok := v.(*Claims); ok {
			return cl
		}
	}
	return nil
}
