// Package impactmevpd — route registration for mst.impact_mev_pd endpoints.
//
// IMPORTANT: /master/impact-mev-pd/export and /master/impact-mev-pd/active
// MUST be registered BEFORE /master/impact-mev-pd/:id to avoid Gin treating
// "export" or "active" as an :id path value.
package impactmevpd

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all impact_mev_pd HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/impact-mev-pd                   → List         (ecl_parameter.read)
//	POST   /master/impact-mev-pd                   → Create       (ecl_parameter.create)
//	GET    /master/impact-mev-pd/export            → Export       (ecl_parameter.read)   [BEFORE /:id]
//	GET    /master/impact-mev-pd/active            → GetActive    (ecl_parameter.read)   [BEFORE /:id]
//	GET    /master/impact-mev-pd/:id               → GetByID      (ecl_parameter.read)
//	PUT    /master/impact-mev-pd/:id               → Update       (ecl_parameter.update)
//	DELETE /master/impact-mev-pd/:id               → Delete       (ecl_parameter.delete)
//	GET    /master/impact-mev-pd/:id/history       → History      (ecl_parameter.read)
//	GET    /master/impact-mev-pd/:id/workflow      → WorkflowStatus (ecl_parameter.read)
//	POST   /master/impact-mev-pd/:id/submit        → Submit       (ecl_parameter.submit)
//	POST   /master/impact-mev-pd/:id/review        → Review       (ecl_parameter.review)
//	POST   /master/impact-mev-pd/:id/approve       → Approve      (ecl_parameter.approve) — step-up MFA
//	POST   /master/impact-mev-pd/:id/approve2      → Approve2     (ecl_parameter.approve) — step-up MFA
//	POST   /master/impact-mev-pd/:id/reject        → Reject       (ecl_parameter.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	mevpd := v1.Group("/master/impact-mev-pd")

	// ── Collection endpoints ──────────────────────────────────────────────────
	mevpd.GET("", auth.RequirePermission("ecl_parameter.read"), h.List)
	mevpd.POST("", auth.RequirePermission("ecl_parameter.create"), h.Create)

	// Fixed-path routes MUST come before /:id.
	mevpd.GET("/export", auth.RequirePermission("ecl_parameter.read"), h.Export)
	mevpd.GET("/active", auth.RequirePermission("ecl_parameter.read"), h.GetActive)

	// ── Single-record endpoints ───────────────────────────────────────────────
	mevpd.GET("/:id", auth.RequirePermission("ecl_parameter.read"), h.GetByID)
	mevpd.PUT("/:id", auth.RequirePermission("ecl_parameter.update"), h.Update)
	mevpd.DELETE("/:id", auth.RequirePermission("ecl_parameter.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	mevpd.GET("/:id/history", auth.RequirePermission("ecl_parameter.read"), h.History)
	mevpd.GET("/:id/workflow", auth.RequirePermission("ecl_parameter.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (Idempotency-Key checked by global middleware).
	mevpd.POST("/:id/submit", auth.RequirePermission("ecl_parameter.submit"), h.Submit)
	mevpd.POST("/:id/review", auth.RequirePermission("ecl_parameter.review"), h.Review)
	mevpd.POST("/:id/approve", auth.RequirePermission("ecl_parameter.approve"), h.Approve)
	mevpd.POST("/:id/approve2", auth.RequirePermission("ecl_parameter.approve"), h.Approve2)
	mevpd.POST("/:id/reject", auth.RequirePermission("ecl_parameter.reject"), h.Reject)
}
