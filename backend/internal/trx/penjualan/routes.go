package penjualan

// routes.go — Route registration for trx.penjualan endpoints (P5-M8).
//
// Route layout:
//
//	GET    /trx/penjualan                           → List                  (penjualan.read)
//	POST   /trx/penjualan                           → Create                (penjualan.create)
//	GET    /trx/penjualan/bm-frequency-alerts       → GetBMAlerts           (penjualan.read)  ← STATIC — registered BEFORE /:id
//	GET    /trx/penjualan/:id                       → GetByID               (penjualan.read)
//	GET    /trx/penjualan/:id/preview               → GetPreview            (penjualan.read)
//	POST   /trx/penjualan/:id/approve               → Approve               (penjualan.approve, SensitiveRateLimit)
//	POST   /trx/penjualan/:id/reject                → Reject                (penjualan.reject,  SensitiveRateLimit)
//
// Gin routing note: Static path "/bm-frequency-alerts" MUST be registered before "/:id"
// to avoid Gin treating the literal string "bm-frequency-alerts" as a UUID parameter.
//
// SensitiveRateLimit: 10 req/min per security-baseline.md §Rate limit (DEC-027).

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all trx/penjualan HTTP routes under the given /api/v1 router group.
// h must not be nil.
// rdb is optional Redis client for rate-limiting (nil = rate limit skipped in dev mode).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	rg := v1.Group("/trx/penjualan")

	// ── Collection endpoints ──────────────────────────────────────────────────
	rg.GET("", auth.RequirePermission("penjualan.read"), h.List)
	rg.POST("", auth.RequirePermission("penjualan.create"), h.Create)

	// ── Static path BEFORE /:id to prevent Gin routing conflict ──────────────
	// GET /trx/penjualan/bm-frequency-alerts must come before GET /trx/penjualan/:id.
	rg.GET("/bm-frequency-alerts", auth.RequirePermission("penjualan.read"), h.GetBMAlerts)

	// ── Single-record read endpoints ──────────────────────────────────────────
	rg.GET("/:id", auth.RequirePermission("penjualan.read"), h.GetByID)
	rg.GET("/:id/preview", auth.RequirePermission("penjualan.read"), h.GetPreview)

	// ── Workflow transition endpoints (sensitive — rate limited) ──────────────
	rg.POST("/:id/approve",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("penjualan.approve"),
		h.Approve,
	)
	rg.POST("/:id/reject",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("penjualan.reject"),
		h.Reject,
	)
}
