package pocidelta

// routes.go — Route registration for POCI delta ECL endpoints (P5-M10).
//
// Route layout (all under /api/v1):
//
//   GET  /poci/baseline                    → ListBaseline         (poci.baseline.read)
//   POST /poci/baseline                    → CaptureBaseline      (poci.baseline.create)
//   GET  /poci/baseline/:instrumen_id      → GetBaseline          (poci.baseline.read)
//   GET  /poci/delta-log                   → ListDeltaLog         (poci.delta.read)
//   GET  /poci/delta-history/summary       → GetDeltaSummary      (poci.delta.read) ← STATIC before /:id
//   GET  /poci/delta-history               → GetDeltaHistory      (poci.delta.read)
//   POST /poci/compute-delta-batch         → ComputeDeltaBatch    (poci.delta.compute)
//
// Gin routing note: /poci/delta-history/summary (static path) registered BEFORE
// any dynamic segment route to avoid routing conflict — same pattern as M9 routes.go.
//
// SensitiveRateLimit on POST /poci/compute-delta-batch: 10 req/min
// per security-baseline.md (DEC-027).

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all pocidelta HTTP routes under the given /api/v1 router group.
// h must not be nil.
// rdb is optional Redis client for rate-limiting (nil = rate limit skipped in dev mode).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	// ── /poci/baseline ────────────────────────────────────────────────────────

	baseline := v1.Group("/poci/baseline")

	baseline.GET("",
		auth.RequirePermission("poci.baseline.read"),
		h.ListBaseline,
	)
	baseline.POST("",
		auth.RequirePermission("poci.baseline.create"),
		h.CaptureBaseline,
	)
	// /:instrumen_id — must come AFTER static paths
	baseline.GET("/:instrumen_id",
		auth.RequirePermission("poci.baseline.read"),
		h.GetBaseline,
	)

	// ── /poci/delta-log ───────────────────────────────────────────────────────

	deltaLog := v1.Group("/poci/delta-log")

	deltaLog.GET("",
		auth.RequirePermission("poci.delta.read"),
		h.ListDeltaLog,
	)

	// ── /poci/delta-history ───────────────────────────────────────────────────

	history := v1.Group("/poci/delta-history")

	// ── Static path BEFORE any dynamic segment ────────────────────────────────
	// GET /poci/delta-history/summary must be registered before :instrumen_id
	// would capture it — Gin uses registration order for static vs param routing.
	history.GET("/summary",
		auth.RequirePermission("poci.delta.read"),
		h.GetDeltaSummary,
	)

	// Collection endpoint (instrumen_id in query param, not path)
	history.GET("",
		auth.RequirePermission("poci.delta.read"),
		h.GetDeltaHistory,
	)

	// ── /poci/compute-delta-batch ─────────────────────────────────────────────

	v1.POST("/poci/compute-delta-batch",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("poci.delta.compute"),
		h.ComputeDeltaBatch,
	)
}
