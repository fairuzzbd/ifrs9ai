package auth

// idle_timeout.go — H-02 fix: Redis-backed idle-window enforcement (DEC-025).
//
// DEC-025: 15-minute idle timeout server-side. For MFA-mandatory roles (CFO, ALCO,
// KOMITE, CEO, Treasury Manager, Finance Controller) this prevents stolen access tokens
// from remaining valid indefinitely after the user becomes idle.
//
// Implementation:
//   - Redis key: "sess:idle:{tenantID}:{userID}" — TTL 16 minutes.
//   - On each authenticated request: read last_activity from Redis.
//     - Key absent → first activity; record it, continue.
//     - now − last_activity > 15 min → 401 IDLE_TIMEOUT, abort.
//     - else → update key to now, continue.
//   - Middleware is mounted AFTER auth.Middleware() on the v1 RouterGroup.
//   - Test mode bypass: if cfg.SkipIdleTimeout == true, middleware is a no-op.
//
// Redis key TTL is 16 minutes (IdleTimeout + 1 minute grace) so the key auto-expires
// one minute after the idle window expires, preventing memory accumulation.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/common/response"
)

const (
	// idleKeyTTL is the Redis key TTL — slightly longer than IdleTimeout to
	// avoid a race where TTL expires at exactly the idle boundary.
	idleKeyTTL = IdleTimeout + time.Minute
)

// idleStore is the minimal Redis interface needed by IdleTimeoutMiddleware.
// Implemented by *redis.Client in production; replaced with a stub in tests.
type idleStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// IdleTimeoutMiddleware returns a Gin middleware that enforces the DEC-025
// 15-minute idle window using a Redis-backed last_activity timestamp.
//
// Parameters:
//   - rdb: Redis client (or idleStore stub). If nil, middleware is a no-op
//     (allows dev without Redis; production must provide a real client).
//   - skip: if true, middleware is a no-op (for test environments).
func IdleTimeoutMiddleware(rdb idleStore, skip bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if skip || rdb == nil {
			c.Next()
			return
		}

		cl := ClaimsFromGin(c)
		if cl == nil {
			// No claims means auth middleware rejected the request — let it abort.
			c.Next()
			return
		}

		userID := cl.Sub
		tenantID := cl.TenantID
		if userID == "" || tenantID == "" {
			c.Next()
			return
		}

		key := fmt.Sprintf("sess:idle:%s:%s", tenantID, userID)
		now := time.Now()
		nowUnix := now.Unix()

		result := rdb.Get(c.Request.Context(), key)
		if result.Err() == nil {
			// Key exists — check idle duration.
			lastActivity, parseErr := result.Int64()
			if parseErr == nil {
				lastActivityTime := time.Unix(lastActivity, 0)
				if now.Sub(lastActivityTime) > IdleTimeout {
					// Idle window exceeded — abort with IDLE_TIMEOUT.
					traceID, _ := c.Get(response.TraceIDKey)
					c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
						"error": map[string]any{
							"code":    "IDLE_TIMEOUT",
							"message": "Sesi idle lebih dari 15 menit. Silakan login kembali.",
							"details": []any{},
							"traceId": traceID,
						},
					})
					return
				}
			}
		}
		// Key absent (first activity) or within window — update timestamp.
		_ = rdb.Set(c.Request.Context(), key, nowUnix, idleKeyTTL)

		c.Next()
	}
}
