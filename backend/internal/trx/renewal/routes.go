package renewal

// routes.go — Route registration for trx.renewal endpoints.
//
// Route layout:
//
//	GET    /trx/renewal                  → List           (renewal.read)
//	POST   /trx/renewal                  → Create         (renewal.create)
//	GET    /trx/renewal/:id              → GetByID        (renewal.read)
//	GET    /trx/renewal/:id/preview      → GetPreview     (renewal.read)
//	POST   /trx/renewal/:id/approve      → Approve        (renewal.approve, SensitiveRateLimit)
//	POST   /trx/renewal/:id/reject       → Reject         (renewal.reject,  SensitiveRateLimit)
//
// Note: /trx/renewal/:id/preview is a GET so no static-before-parameterised conflict
// with Gin routing. The approve/reject POST routes are under /:id which Gin handles.
//
// SensitiveRateLimit (10 req/min) applied to approve + reject per security-baseline.md §Rate limit.

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all trx/renewal HTTP routes under the given /api/v1 router group.
// h must not be nil.
// rdb is optional Redis client for rate-limiting (nil = rate limit skipped in dev mode).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	rg := v1.Group("/trx/renewal")

	// ── Collection endpoints ──────────────────────────────────────────
	rg.GET("", auth.RequirePermission("renewal.read"), h.List)
	rg.POST("", auth.RequirePermission("renewal.create"), h.Create)

	// ── Single-record read endpoints ──────────────────────────────────
	rg.GET("/:id", auth.RequirePermission("renewal.read"), h.GetByID)
	rg.GET("/:id/preview", auth.RequirePermission("renewal.read"), h.GetPreview)

	// ── Workflow transition endpoints (sensitive — rate limited) ──────
	rg.POST("/:id/approve",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("renewal.approve"),
		h.Approve,
	)
	rg.POST("/:id/reject",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("renewal.reject"),
		h.Reject,
	)
}
