// Package middleware menyediakan Gin middleware cross-cutting untuk BLIPS:
// - Request ID + trace propagation
// - Structured logging (slog)
// - Error handler → error envelope
// - Rate limiter (token bucket, Redis-backed)
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// traceIDContextKey adalah type untuk menghindari collision di context.
type traceIDContextKey struct{}

// ipContextKey menyimpan client IP yang di-set oleh auth middleware.
type ipContextKey struct{}

// userAgentContextKey menyimpan User-Agent yang di-set oleh auth middleware.
type userAgentContextKey struct{}

// ContextWithIPUA menambahkan client IP dan User-Agent ke context.
// Dipanggil oleh auth middleware setelah JWT verification agar tersedia di service/audit layer.
// IP kosong string → disimpan NULL di aud.audit_log (tidak crash).
func ContextWithIPUA(ctx context.Context, ip, userAgent string) context.Context {
	ctx = context.WithValue(ctx, ipContextKey{}, ip)
	ctx = context.WithValue(ctx, userAgentContextKey{}, userAgent)
	return ctx
}

// IPFromContext mengambil client IP dari context.
// Dipakai oleh audit.writeEvent untuk mengisi kolom ip di aud.audit_log.
func IPFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ipContextKey{}).(string); ok {
		return v
	}
	return ""
}

// UserAgentFromContext mengambil User-Agent dari context.
// Dipakai oleh audit.writeEvent untuk mengisi kolom user_agent di aud.audit_log.
func UserAgentFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userAgentContextKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestID adalah middleware yang memastikan setiap request memiliki X-Trace-Id.
// Jika header sudah ada (inject oleh gateway), nilai itu dipakai.
// Jika tidak, generate random hex 16 byte.
// Trace ID di-set di: Gin context (key TraceIDKey), response header, request context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = generateTraceID()
		}

		// Set ke Gin context agar helper response.getTraceID() bisa ambil.
		c.Set(response.TraceIDKey, traceID)
		// Set ke request context untuk propagasi ke service/repo layer.
		ctx := context.WithValue(c.Request.Context(), traceIDContextKey{}, traceID)
		c.Request = c.Request.WithContext(ctx)
		// Set response header sehingga client bisa korelasi.
		c.Header("X-Trace-Id", traceID)

		c.Next()
	}
}

// TraceIDFromContext mengambil trace ID dari context. Dipakai di service/repo.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

// generateTraceID menghasilkan hex 16 byte (32 karakter).
func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback ke timestamp-based jika rand gagal (sangat jarang).
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.999999")))
	}
	return hex.EncodeToString(b)
}

// Logger adalah middleware structured logging menggunakan log/slog.
// Field yang selalu ada: traceId, method, path, status, latencyMs, ip.
// Field dari context (bila tersedia): userId, tenantId.
// PII (body, query params yang sensitif) TIDAK di-log.
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		traceID, _ := c.Get(response.TraceIDKey)
		latency := time.Since(start)
		status := c.Writer.Status()

		// Ambil user info dari context (di-set oleh auth middleware).
		userID, _ := c.Get("userId")
		tenantID, _ := c.Get("tenantId")

		attrs := []any{
			"traceId", traceID,
			"method", method,
			"path", path,
			"status", status,
			"latencyMs", latency.Milliseconds(),
			"ip", c.ClientIP(),
		}
		if userID != nil && userID != "" {
			attrs = append(attrs, "userId", userID)
		}
		if tenantID != nil && tenantID != "" {
			attrs = append(attrs, "tenantId", tenantID)
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "ginErrors", c.Errors.String())
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		logger.LogAttrs(context.Background(), level, "http_request", slogAttrs(attrs)...)
	}
}

// slogAttrs mengkonversi slice any ke slice slog.Attr (key-value pair).
func slogAttrs(kv []any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		attrs = append(attrs, slog.Any(key, kv[i+1]))
	}
	return attrs
}

// Recovery adalah middleware panic recovery yang mengembalikan 500 error envelope
// (bukan Gin default yang bisa expose stack trace ke client).
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		traceID, _ := c.Get(response.TraceIDKey)
		logger.ErrorContext(c.Request.Context(), "panic recovered",
			"traceId", traceID,
			"panic", recovered,
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"code":    "INTERNAL",
				"message": "Terjadi kesalahan internal. Hubungi admin dengan traceId.",
				"details": []any{},
				"traceId": traceID,
			},
		})
	})
}
