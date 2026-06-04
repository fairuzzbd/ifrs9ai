// Package mappingjurnal — route registration for mst.mapping_jurnal endpoints.
//
// REUSE PATTERN: identical signature to matauang.RegisterRoutes.
package mappingjurnal

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all mapping_jurnal HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/mapping-jurnal                  → List          (mapping_jurnal.read)
//	POST   /master/mapping-jurnal                  → Create        (mapping_jurnal.create)
//	GET    /master/mapping-jurnal/export           → Export        (mapping_jurnal.read)
//	GET    /master/mapping-jurnal/:id              → GetByID       (mapping_jurnal.read)
//	PATCH  /master/mapping-jurnal/:id              → Update        (mapping_jurnal.update)
//	DELETE /master/mapping-jurnal/:id              → Delete        (mapping_jurnal.delete)
//	GET    /master/mapping-jurnal/:id/history      → History       (mapping_jurnal.read)
//	GET    /master/mapping-jurnal/:id/workflow     → WorkflowStatus(mapping_jurnal.read)
//	POST   /master/mapping-jurnal/:id/submit       → Submit        (mapping_jurnal.submit)
//	POST   /master/mapping-jurnal/:id/review       → Review        (mapping_jurnal.review)
//	POST   /master/mapping-jurnal/:id/approve      → Approve       (mapping_jurnal.approve)
//	POST   /master/mapping-jurnal/:id/reject       → Reject        (mapping_jurnal.reject)
//
// IMPORTANT: /export must be registered BEFORE /:id to avoid Gin treating "export" as :id.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	mg := v1.Group("/master/mapping-jurnal")

	// ── Collection endpoints ──────────────────────────────────────────────────
	mg.GET("", auth.RequirePermission("mapping_jurnal.read"), h.List)
	mg.POST("", auth.RequirePermission("mapping_jurnal.create"), h.Create)

	// Export — registered before /:id to avoid path conflict.
	mg.GET("/export", auth.RequirePermission("mapping_jurnal.read"), h.Export)

	// ── Single-record endpoints ───────────────────────────────────────────────
	mg.GET("/:id", auth.RequirePermission("mapping_jurnal.read"), h.GetByID)
	mg.PATCH("/:id", auth.RequirePermission("mapping_jurnal.update"), h.Update)
	mg.DELETE("/:id", auth.RequirePermission("mapping_jurnal.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	mg.GET("/:id/history", auth.RequirePermission("mapping_jurnal.read"), h.History)
	mg.GET("/:id/workflow", auth.RequirePermission("mapping_jurnal.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	mg.POST("/:id/submit", auth.RequirePermission("mapping_jurnal.submit"), h.Submit)
	mg.POST("/:id/review", auth.RequirePermission("mapping_jurnal.review"), h.Review)
	mg.POST("/:id/approve", auth.RequirePermission("mapping_jurnal.approve"), h.Approve)
	mg.POST("/:id/reject", auth.RequirePermission("mapping_jurnal.reject"), h.Reject)
}
