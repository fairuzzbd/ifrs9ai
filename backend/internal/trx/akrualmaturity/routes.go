package akrualmaturity

// routes.go — Route registration for akrualmaturity endpoints (P5-M9).
//
// Route layout:
//
//	GET  /transaksi/akrual                            → ListAkrual          (akrual.read)
//	GET  /transaksi/akrual/dashboard                  → GetDashboard         (akrual.read)    ← STATIC — before /:id
//	POST /transaksi/akrual/cron-trigger               → TriggerAkrualCron   (sys.cron.trigger) ← STATIC — before /:id
//	GET  /transaksi/akrual/:id                        → GetAkrualByID        (akrual.read)
//	POST /transaksi/akrual/:id/override-stale         → OverrideStale        (akrual.override_stale, SensitiveRateLimit)
//	GET  /transaksi/jatuh-tempo                       → ListJatuhTempo       (maturity.read)
//	POST /transaksi/jatuh-tempo/cron-trigger          → TriggerMaturityCron  (sys.cron.trigger) ← STATIC — before /:id
//
// Gin routing note: All static paths MUST be registered before /:id routes.
// "dashboard" and "cron-trigger" both registered before /:id.
//
// SensitiveRateLimit: 10 req/min per security-baseline.md (DEC-027).

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all akrualmaturity HTTP routes under the given /api/v1 router group.
// h must not be nil.
// rdb is optional Redis client for rate-limiting (nil = rate limit skipped in dev mode).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	// ── /transaksi/akrual ─────────────────────────────────────────────────────

	akrual := v1.Group("/transaksi/akrual")

	// Collection endpoints
	akrual.GET("", auth.RequirePermission("akrual.read"), h.ListAkrual)

	// ── Static paths BEFORE /:id to prevent Gin routing conflict ──────────────
	//
	// "dashboard" and "cron-trigger" must come before "/:id".
	akrual.GET("/dashboard",
		auth.RequirePermission("akrual.read"),
		h.GetDashboard,
	)
	akrual.POST("/cron-trigger",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("sys.cron.trigger"),
		h.TriggerAkrualCron,
	)

	// ── Single-record endpoints ────────────────────────────────────────────────
	akrual.GET("/:id",
		auth.RequirePermission("akrual.read"),
		h.GetAkrualByID,
	)
	akrual.POST("/:id/override-stale",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("akrual.override_stale"),
		h.OverrideStale,
	)

	// ── /transaksi/jatuh-tempo ────────────────────────────────────────────────

	jt := v1.Group("/transaksi/jatuh-tempo")

	// ── Static path BEFORE /:id ───────────────────────────────────────────────
	jt.POST("/cron-trigger",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("sys.cron.trigger"),
		h.TriggerMaturityCron,
	)

	// Collection endpoint
	jt.GET("",
		auth.RequirePermission("maturity.read"),
		h.ListJatuhTempo,
	)
}
