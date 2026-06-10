// Package instrumen — route registration for mst.instrumen endpoints (APP-A-MSTR-011).
//
// Route layout:
//
//	GET    /master/instrumen                    → List          (instrumen.read)
//	POST   /master/instrumen                    → Create        (instrumen.create)
//	GET    /master/instrumen/export             → Export        (instrumen.read)
//	GET    /master/instrumen/:id               → GetByID       (instrumen.read)
//	PUT    /master/instrumen/:id               → Update        (instrumen.update)
//	DELETE /master/instrumen/:id               → Delete        (instrumen.delete)
//	GET    /master/instrumen/:id/history       → History       (instrumen.read)
//	GET    /master/instrumen/:id/workflow      → WorkflowStatus(instrumen.read)
//	POST   /master/instrumen/:id/submit        → Submit        (instrumen.submit)
//	POST   /master/instrumen/:id/review        → Review        (instrumen.review)
//	POST   /master/instrumen/:id/approve       → Approve       (instrumen.approve)
//	POST   /master/instrumen/:id/reject        → Reject        (instrumen.reject)
//
// IMPORTANT: /export MUST be registered BEFORE /:id to avoid Gin treating "export" as :id.
package instrumen

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all instrumen HTTP routes under /api/v1.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	ig := v1.Group("/master/instrumen")

	// Collection
	ig.GET("", auth.RequirePermission("instrumen.read"), h.List)
	ig.POST("", auth.RequirePermission("instrumen.create"), h.Create)

	// Export — before /:id to avoid path conflict
	ig.GET("/export", auth.RequirePermission("instrumen.read"), h.Export)

	// Single record
	ig.GET("/:id", auth.RequirePermission("instrumen.read"), h.GetByID)
	ig.PUT("/:id", auth.RequirePermission("instrumen.update"), h.Update)
	ig.DELETE("/:id", auth.RequirePermission("instrumen.delete"), h.Delete)

	// Sub-resources
	ig.GET("/:id/history", auth.RequirePermission("instrumen.read"), h.History)
	ig.GET("/:id/workflow", auth.RequirePermission("instrumen.read"), h.WorkflowStatus)

	// Workflow mutations (all require Idempotency-Key via global middleware)
	ig.POST("/:id/submit", auth.RequirePermission("instrumen.submit"), h.Submit)
	ig.POST("/:id/review", auth.RequirePermission("instrumen.review"), h.Review)
	ig.POST("/:id/approve", auth.RequirePermission("instrumen.approve"), h.Approve)
	ig.POST("/:id/reject", auth.RequirePermission("instrumen.reject"), h.Reject)
}
