package mtm

// routes.go — Route registration for trx.mtm endpoints.
//
// Route layout:
//
//	GET    /trx/mtm                           → List              (fx_rate.read)
//	POST   /trx/mtm/upload/batch              → UploadBatch       (fx_rate.create)
//	GET    /trx/mtm/upload/batch/:batch_id    → GetUploadBatch    (fx_rate.read)
//	POST   /trx/mtm/cron/trigger              → CronTrigger       (fx_rate.create)
//	GET    /trx/mtm/alerts/stale-price        → StalePriceAlerts  (fx_rate.read)
//	GET    /trx/mtm/:id                       → GetByID           (fx_rate.read)
//	POST   /trx/mtm/:id/override-approve      → OverrideApprove   (fx_rate.approve)
//	POST   /trx/mtm/:id/override-reject       → OverrideReject    (fx_rate.approve)
//
// CRITICAL: ALL static sub-paths (/upload/batch, /cron/trigger, /alerts/stale-price)
// MUST be registered BEFORE /:id to avoid Gin treating them as path param values.
// Gin panics on ambiguous wildcard / literal conflicts — static wins by registration order.

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all trx/mtm HTTP routes under the given /api/v1 router group.
// h must not be nil; enqueuer is injected into HTTPHandler at construction (NewHTTPHandler).
func RegisterRoutes(v1 *gin.RouterGroup, h *HTTPHandler) {
	mg := v1.Group("/trx/mtm")

	// ── Collection endpoint ───────────────────────────────────────────────────
	mg.GET("", auth.RequirePermission("fx_rate.read"), h.List)

	// ── Static sub-paths (MUST be BEFORE /:id) ────────────────────────────────

	// Upload batch (static: /upload/batch)
	mg.POST("/upload/batch", auth.RequirePermission("fx_rate.create"), h.UploadBatch)
	mg.GET("/upload/batch/:batch_id", auth.RequirePermission("fx_rate.read"), h.GetUploadBatch)

	// Cron manual trigger (static: /cron/trigger)
	mg.POST("/cron/trigger", auth.RequirePermission("fx_rate.create"), h.CronTrigger)

	// Stale-price alert (static: /alerts/stale-price)
	mg.GET("/alerts/stale-price", auth.RequirePermission("fx_rate.read"), h.StalePriceAlerts)

	// ── Single-record endpoints (parameterised — AFTER all static paths) ──────
	mg.GET("/:id", auth.RequirePermission("fx_rate.read"), h.GetByID)
	mg.POST("/:id/override-approve", auth.RequirePermission("fx_rate.approve"), h.OverrideApprove)
	mg.POST("/:id/override-reject", auth.RequirePermission("fx_rate.approve"), h.OverrideReject)
}
