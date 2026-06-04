// Package portofolio — route registration for mst.portofolio endpoints.
//
// IMPORTANT: /master/portofolio/export MUST be registered BEFORE /master/portofolio/:kode
// to avoid Gin treating "export" as a :kode value.
package portofolio

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all portofolio HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/portofolio                    → List          (portofolio.read)
//	POST   /master/portofolio                    → Create        (portofolio.create)
//	GET    /master/portofolio/export             → Export        (portofolio.read)
//	GET    /master/portofolio/:kode              → GetByKode     (portofolio.read)
//	PUT    /master/portofolio/:kode              → Update        (portofolio.update)
//	DELETE /master/portofolio/:kode              → Delete        (portofolio.delete)
//	GET    /master/portofolio/:kode/history      → History       (portofolio.read)
//	GET    /master/portofolio/:kode/workflow     → WorkflowStatus(portofolio.read)
//	POST   /master/portofolio/:kode/submit       → Submit        (portofolio.submit)
//	POST   /master/portofolio/:kode/review       → Review        (portofolio.review)
//	POST   /master/portofolio/:kode/approve      → Approve       (portofolio.approve)
//	POST   /master/portofolio/:kode/reject       → Reject        (portofolio.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	pg := v1.Group("/master/portofolio")

	// ── Collection endpoints ──────────────────────────────────────────────────
	pg.GET("", auth.RequirePermission("portofolio.read"), h.List)
	pg.POST("", auth.RequirePermission("portofolio.create"), h.Create)

	// Export — registered before /:kode to avoid path conflict.
	pg.GET("/export", auth.RequirePermission("portofolio.read"), h.Export)

	// ── Single-record endpoints ───────────────────────────────────────────────
	pg.GET("/:kode", auth.RequirePermission("portofolio.read"), h.GetByKode)
	pg.PUT("/:kode", auth.RequirePermission("portofolio.update"), h.Update)
	pg.DELETE("/:kode", auth.RequirePermission("portofolio.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	pg.GET("/:kode/history", auth.RequirePermission("portofolio.read"), h.History)
	pg.GET("/:kode/workflow", auth.RequirePermission("portofolio.read"), h.WorkflowStatus)

	// Workflow mutation endpoints.
	pg.POST("/:kode/submit", auth.RequirePermission("portofolio.submit"), h.Submit)
	pg.POST("/:kode/review", auth.RequirePermission("portofolio.review"), h.Review)
	pg.POST("/:kode/approve", auth.RequirePermission("portofolio.approve"), h.Approve)
	pg.POST("/:kode/reject", auth.RequirePermission("portofolio.reject"), h.Reject)
}
