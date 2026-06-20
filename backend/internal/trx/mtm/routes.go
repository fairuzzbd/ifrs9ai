package mtm

// routes.go — Route registration for trx.mtm endpoints.
//
// Route layout:
//
//	GET    /trx/mtm                           → List              (mtm.read)
//	POST   /trx/mtm/upload/batch              → UploadBatch       (mtm.create)
//	GET    /trx/mtm/upload/batch/:batch_id    → GetUploadBatch    (mtm.read)
//	POST   /trx/mtm/cron/trigger              → CronTrigger       (mtm.trigger)
//	GET    /trx/mtm/alerts/stale-price        → StalePriceAlerts  (mtm.read)
//	GET    /trx/mtm/:id                       → GetByID           (mtm.read)
//	POST   /trx/mtm/:id/override-approve      → OverrideApprove   (mtm.override)
//	POST   /trx/mtm/:id/override-reject       → OverrideReject    (mtm.override)
//
// CRITICAL: ALL static sub-paths (/upload/batch, /cron/trigger, /alerts/stale-price)
// MUST be registered BEFORE /:id to avoid Gin treating them as path param values.
// Gin panics on ambiguous wildcard / literal conflicts — static wins by registration order.
//
// B3 fix: cron/trigger uses mtm.trigger permission (not fx_rate.create).
// B4 fix: cron/trigger has SensitiveRateLimit (10 req/min) applied at route level.
// m2 fix: all permission strings corrected to mtm.* namespace.

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all trx/mtm HTTP routes under the given /api/v1 router group.
// h must not be nil; enqueuer is injected into HTTPHandler at construction (NewHTTPHandler).
// rdb is the Redis client for rate-limiting (may be nil in dev mode — rate limit skipped).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	// Extract optional redis client (variadic for backward compat with existing main.go call).
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	mg := v1.Group("/trx/mtm")

	// ── Collection endpoint ───────────────────────────────────────────────────
	// m2: mtm.read (was fx_rate.read)
	mg.GET("", auth.RequirePermission("mtm.read"), h.List)

	// ── Static sub-paths (MUST be BEFORE /:id) ────────────────────────────────

	// Upload batch (static: /upload/batch)
	// m2: mtm.create (was fx_rate.create)
	mg.POST("/upload/batch", auth.RequirePermission("mtm.create"), h.UploadBatch)
	mg.GET("/upload/batch/:batch_id", auth.RequirePermission("mtm.read"), h.GetUploadBatch)

	// Cron manual trigger (static: /cron/trigger)
	// B3 fix: mtm.trigger permission (was fx_rate.create — wrong entity/action).
	// B4 fix: SensitiveRateLimit 10 req/min (cron trigger is an operational action that can
	//         cause mass DB writes — rate limit prevents abuse / accidental flood).
	mg.POST("/cron/trigger",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("mtm.trigger"),
		h.CronTrigger,
	)

	// Stale-price alert (static: /alerts/stale-price)
	// m2: mtm.read (was fx_rate.read)
	mg.GET("/alerts/stale-price", auth.RequirePermission("mtm.read"), h.StalePriceAlerts)

	// ── Single-record endpoints (parameterised — AFTER all static paths) ──────
	// m2: mtm.read, mtm.override (were fx_rate.read, fx_rate.approve)
	mg.GET("/:id", auth.RequirePermission("mtm.read"), h.GetByID)
	mg.POST("/:id/override-approve", auth.RequirePermission("mtm.override"), h.OverrideApprove)
	mg.POST("/:id/override-reject", auth.RequirePermission("mtm.override"), h.OverrideReject)
}
