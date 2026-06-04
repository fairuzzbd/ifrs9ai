// Package counterparty — route registration for mst.counterparty endpoints.
//
// Route layout:
//
//	GET    /master/counterparty              → List         (counterparty.read)
//	POST   /master/counterparty              → Create       (counterparty.create)
//	GET    /master/counterparty/export       → Export       (counterparty.export)
//	GET    /master/counterparty/:id          → GetByID      (counterparty.read)
//	GET    /master/counterparty/:id/pii      → GetPII       (counterparty.view_pii)
//	PUT    /master/counterparty/:id          → Update       (counterparty.update)
//	DELETE /master/counterparty/:id          → Delete       (counterparty.delete)
//	GET    /master/counterparty/:id/history  → History      (counterparty.read)
//	GET    /master/counterparty/:id/workflow → WorkflowStatus (counterparty.read)
//	POST   /master/counterparty/:id/submit   → Submit       (counterparty.submit)
//	POST   /master/counterparty/:id/review   → Review       (counterparty.review)
//	POST   /master/counterparty/:id/approve  → Approve      (counterparty.approve)
//	POST   /master/counterparty/:id/reject   → Reject       (counterparty.reject)
//
// IMPORTANT: /master/counterparty/export MUST be registered BEFORE /:id to
// avoid Gin treating "export" as an :id value.
package counterparty

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all counterparty HTTP routes under the given /api/v1 router group.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	cg := v1.Group("/master/counterparty")

	// Collection endpoints
	cg.GET("", auth.RequirePermission("counterparty.read"), h.List)
	cg.POST("", auth.RequirePermission("counterparty.create"), h.Create)

	// Export — registered before /:id to avoid path conflict.
	cg.GET("/export", auth.RequirePermission("counterparty.export"), h.Export)

	// Single-record endpoints
	cg.GET("/:id", auth.RequirePermission("counterparty.read"), h.GetByID)
	cg.PUT("/:id", auth.RequirePermission("counterparty.update"), h.Update)
	cg.DELETE("/:id", auth.RequirePermission("counterparty.delete"), h.Delete)

	// Sub-resources
	cg.GET("/:id/pii", auth.RequirePermission("counterparty.view_pii"), h.GetPII)
	cg.GET("/:id/history", auth.RequirePermission("counterparty.read"), h.History)
	cg.GET("/:id/workflow", auth.RequirePermission("counterparty.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (require Idempotency-Key via global middleware)
	cg.POST("/:id/submit", auth.RequirePermission("counterparty.submit"), h.Submit)
	cg.POST("/:id/review", auth.RequirePermission("counterparty.review"), h.Review)
	cg.POST("/:id/approve", auth.RequirePermission("counterparty.approve"), h.Approve)
	cg.POST("/:id/reject", auth.RequirePermission("counterparty.reject"), h.Reject)
}
