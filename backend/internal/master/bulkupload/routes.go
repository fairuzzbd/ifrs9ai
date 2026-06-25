package bulkupload

// routes.go — Route registration for bulk upload master instrumen (P5-M11).
//
// Route layout (all under /api/v1/master/instrumen/bulk-upload):
//
//   POST   /                         → UploadBatch        (bulkupload.create)  ROLE-MAKER-TR
//   GET    /:batch_id                → GetBatch           (bulkupload.read)    ROLE-MAKER-TR | ROLE-APPR-TR
//   POST   /:batch_id/dry-run       → DryRun             (bulkupload.create)  ROLE-MAKER-TR
//   POST   /:batch_id/commit        → Commit             (bulkupload.create)  ROLE-MAKER-TR; SensitiveRateLimit
//   POST   /:batch_id/approve       → Approve            (bulkupload.approve) ROLE-APPR-TR
//   POST   /:batch_id/rollback-request  → RollbackRequest (bulkupload.rollback) ROLE-CFO
//   POST   /:batch_id/rollback-approve  → RollbackApprove  (bulkupload.rollback) ROLE-CFO; step-up MFA
//
// Security:
//   - SensitiveRateLimit (10 req/min) on commit + approve + rollback (security-baseline.md DEC-027)
//   - RequireStepUpMiddleware on rollback-approve (DEC-027 scope='bulk_rollback')
//   - Idempotency-Key checked per-handler via middleware (DEC-021)
//
// Gin routing note:
//   Static sub-paths (/dry-run, /commit, /approve, etc.) under /:batch_id registered
//   before the bare /:batch_id GET to avoid route conflict — same pattern as M9/M10.
//
// References: P5-M11-S1..S5, security-baseline.md, api-conventions.md.

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all bulk upload routes under the given /api/v1 router group.
// h must not be nil.
// rdb is optional Redis client for rate-limiting (nil = rate limit skipped in dev mode).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler, rdb ...*redis.Client) {
	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	bulk := v1.Group("/master/instrumen/bulk-upload")

	// ── POST / ────────────────────────────────────────────────────────────────
	// Upload XLSX file — maker only
	bulk.POST("",
		auth.RequirePermission("bulkupload.create"),
		h.UploadBatch,
	)

	// ── Sub-routes under /:batch_id ───────────────────────────────────────────
	// IMPORTANT: static sub-paths MUST be registered before the bare GET /:batch_id
	// so Gin does not treat "dry-run" etc. as a batch_id value.

	batchGroup := bulk.Group("/:batch_id")

	// POST /:batch_id/dry-run
	batchGroup.POST("/dry-run",
		auth.RequirePermission("bulkupload.create"),
		h.DryRun,
	)

	// POST /:batch_id/commit — SensitiveRateLimit (10 req/min, DEC-027)
	batchGroup.POST("/commit",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("bulkupload.create"),
		h.Commit,
	)

	// POST /:batch_id/approve — SensitiveRateLimit, ROLE-APPR-TR
	batchGroup.POST("/approve",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("bulkupload.approve"),
		h.Approve,
	)

	// POST /:batch_id/rollback-request — ROLE-CFO
	batchGroup.POST("/rollback-request",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("bulkupload.rollback"),
		h.RollbackRequest,
	)

	// POST /:batch_id/rollback-approve — ROLE-CFO + step-up MFA (DEC-027 scope='bulk_rollback')
	batchGroup.POST("/rollback-approve",
		middleware.RateLimiter(redisClient, middleware.SensitiveRateLimit),
		auth.RequirePermission("bulkupload.rollback"),
		auth.RequireStepUpMiddleware("bulk_rollback"),
		h.RollbackApprove,
	)

	// ── GET /:batch_id — batch detail (registered LAST to not shadow sub-paths) ──
	batchGroup.GET("",
		auth.RequirePermission("bulkupload.read"),
		h.GetBatch,
	)
}
