// Package impactpd — route registration for mst.impact_pd endpoints.
//
// IMPORTANT: /master/impact-pd/export and /master/impact-pd/active
// MUST be registered BEFORE /master/impact-pd/:id.
package impactpd

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all impact_pd HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/impact-pd                  → List         (ecl_parameter.read)
//	POST   /master/impact-pd                  → Create       (ecl_parameter.create)
//	GET    /master/impact-pd/export           → Export       (ecl_parameter.read)   [BEFORE /:id]
//	GET    /master/impact-pd/active           → GetActive    (ecl_parameter.read)   [BEFORE /:id]
//	GET    /master/impact-pd/:id              → GetByID      (ecl_parameter.read)
//	PUT    /master/impact-pd/:id              → Update       (ecl_parameter.update)
//	DELETE /master/impact-pd/:id              → Delete       (ecl_parameter.delete)
//	GET    /master/impact-pd/:id/history      → History      (ecl_parameter.read)
//	GET    /master/impact-pd/:id/workflow     → WorkflowStatus (ecl_parameter.read)
//	POST   /master/impact-pd/:id/submit       → Submit       (ecl_parameter.submit)
//	POST   /master/impact-pd/:id/review       → Review       (ecl_parameter.review)
//	POST   /master/impact-pd/:id/approve      → Approve      (ecl_parameter.approve) — step-up MFA
//	POST   /master/impact-pd/:id/approve2     → Approve2     (ecl_parameter.approve) — step-up MFA (ALCO)
//	POST   /master/impact-pd/:id/reject       → Reject       (ecl_parameter.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	ipd := v1.Group("/master/impact-pd")

	// ── Collection endpoints ──────────────────────────────────────────────────
	ipd.GET("", auth.RequirePermission("ecl_parameter.read"), h.List)
	ipd.POST("", auth.RequirePermission("ecl_parameter.create"), h.Create)

	// Fixed-path routes MUST come before /:id.
	ipd.GET("/export", auth.RequirePermission("ecl_parameter.read"), h.Export)
	ipd.GET("/active", auth.RequirePermission("ecl_parameter.read"), h.GetActive)

	// ── Single-record endpoints ───────────────────────────────────────────────
	ipd.GET("/:id", auth.RequirePermission("ecl_parameter.read"), h.GetByID)
	ipd.PUT("/:id", auth.RequirePermission("ecl_parameter.update"), h.Update)
	ipd.DELETE("/:id", auth.RequirePermission("ecl_parameter.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	ipd.GET("/:id/history", auth.RequirePermission("ecl_parameter.read"), h.History)
	ipd.GET("/:id/workflow", auth.RequirePermission("ecl_parameter.read"), h.WorkflowStatus)

	// Workflow mutation endpoints.
	ipd.POST("/:id/submit", auth.RequirePermission("ecl_parameter.submit"), h.Submit)
	ipd.POST("/:id/review", auth.RequirePermission("ecl_parameter.review"), h.Review)
	ipd.POST("/:id/approve", auth.RequirePermission("ecl_parameter.approve"), h.Approve)
	ipd.POST("/:id/approve2", auth.RequirePermission("ecl_parameter.approve"), h.Approve2)
	ipd.POST("/:id/reject", auth.RequirePermission("ecl_parameter.reject"), h.Reject)
}
