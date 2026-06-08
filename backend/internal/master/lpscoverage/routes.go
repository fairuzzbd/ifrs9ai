// Package lpscoverage — route registration for mst.lps_coverage endpoints.
//
// REUSE PATTERN: Every master module creates a RegisterRoutes func with the same signature:
//
//	func RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
//
// IMPORTANT: /master/lps-coverage/export MUST be registered BEFORE /master/lps-coverage/:id
// to avoid Gin treating "export" as an :id value.
package lpscoverage

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all lps_coverage HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/lps-coverage                    → List         (ecl_parameter.read)
//	POST   /master/lps-coverage                    → Create       (ecl_parameter.create)
//	GET    /master/lps-coverage/export             → Export       (ecl_parameter.read)
//	GET    /master/lps-coverage/:id                → GetByID      (ecl_parameter.read)
//	PUT    /master/lps-coverage/:id                → Update       (ecl_parameter.update)
//	DELETE /master/lps-coverage/:id                → Delete       (ecl_parameter.delete)
//	GET    /master/lps-coverage/:id/history        → History      (ecl_parameter.read)
//	GET    /master/lps-coverage/:id/workflow       → WorkflowStatus (ecl_parameter.read)
//	POST   /master/lps-coverage/:id/submit         → Submit       (ecl_parameter.submit)
//	POST   /master/lps-coverage/:id/review         → Review       (ecl_parameter.review)
//	POST   /master/lps-coverage/:id/approve        → Approve      (ecl_parameter.approve)
//	POST   /master/lps-coverage/:id/approve2       → Approve2     (ecl_parameter.approve)
//	POST   /master/lps-coverage/:id/reject         → Reject       (ecl_parameter.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	lps := v1.Group("/master/lps-coverage")

	// ── Collection endpoints ──────────────────────────────────────────────────
	lps.GET("", auth.RequirePermission("ecl_parameter.read"), h.List)
	lps.POST("", auth.RequirePermission("ecl_parameter.create"), h.Create)

	// Export — MUST be registered before /:id to avoid path conflict.
	lps.GET("/export", auth.RequirePermission("ecl_parameter.read"), h.Export)

	// ── Single-record endpoints ───────────────────────────────────────────────
	lps.GET("/:id", auth.RequirePermission("ecl_parameter.read"), h.GetByID)
	lps.PUT("/:id", auth.RequirePermission("ecl_parameter.update"), h.Update)
	lps.DELETE("/:id", auth.RequirePermission("ecl_parameter.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	lps.GET("/:id/history", auth.RequirePermission("ecl_parameter.read"), h.History)
	lps.GET("/:id/workflow", auth.RequirePermission("ecl_parameter.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	lps.POST("/:id/submit", auth.RequirePermission("ecl_parameter.submit"), h.Submit)
	lps.POST("/:id/review", auth.RequirePermission("ecl_parameter.review"), h.Review)
	lps.POST("/:id/approve", auth.RequirePermission("ecl_parameter.approve"), h.Approve)
	lps.POST("/:id/approve2", auth.RequirePermission("ecl_parameter.approve"), h.Approve2)
	lps.POST("/:id/reject", auth.RequirePermission("ecl_parameter.reject"), h.Reject)
}
