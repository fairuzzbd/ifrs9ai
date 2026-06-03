package impactmevpd

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes wires all impact_mev_pd HTTP routes under the given /api/v1 group.
//
// Route layout:
//
//	GET    /master/impact-mev-pd                  → List           (ecl_parameter.read)
//	POST   /master/impact-mev-pd                  → Create         (ecl_parameter.submit)
//	GET    /master/impact-mev-pd/export           → Export         (ecl_parameter.read)
//	GET    /master/impact-mev-pd/:id              → GetByID        (ecl_parameter.read)
//	PUT    /master/impact-mev-pd/:id              → Update         (ecl_parameter.submit)
//	DELETE /master/impact-mev-pd/:id              → Delete         (ecl_parameter.submit)
//	GET    /master/impact-mev-pd/:id/history      → History        (ecl_parameter.read)
//	POST   /master/impact-mev-pd/:id/submit       → Submit         (ecl_parameter.submit)
//	POST   /master/impact-mev-pd/:id/review       → Review         (ecl_parameter.review)
//	POST   /master/impact-mev-pd/:id/approve      → Approve        (ecl_parameter.approve)
//	POST   /master/impact-mev-pd/:id/approve2     → Approve2       (ecl_parameter.approve)
//	POST   /master/impact-mev-pd/:id/reject       → Reject         (ecl_parameter.reject)
//	GET    /master/impact-mev-pd/:id/workflow     → WorkflowStatus (ecl_parameter.read)
//
// NOTE: /export must be registered BEFORE /:id to prevent Gin treating "export" as an id value.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	g := v1.Group("/master/impact-mev-pd")

	// Collection
	g.GET("", auth.RequirePermission("ecl_parameter.read"), h.List)
	g.POST("", auth.RequirePermission("ecl_parameter.submit"), h.Create)

	// Export — before /:id
	g.GET("/export", auth.RequirePermission("ecl_parameter.read"), h.Export)

	// Single record
	g.GET("/:id", auth.RequirePermission("ecl_parameter.read"), h.GetByID)
	g.PUT("/:id", auth.RequirePermission("ecl_parameter.submit"), h.Update)
	g.DELETE("/:id", auth.RequirePermission("ecl_parameter.submit"), h.Delete)

	// Sub-resources
	g.GET("/:id/history", auth.RequirePermission("ecl_parameter.read"), h.History)
	g.GET("/:id/workflow", auth.RequirePermission("ecl_parameter.read"), h.WorkflowStatus)

	// Workflow mutations
	g.POST("/:id/submit", auth.RequirePermission("ecl_parameter.submit"), h.Submit)
	g.POST("/:id/review", auth.RequirePermission("ecl_parameter.review"), h.Review)
	g.POST("/:id/approve", auth.RequirePermission("ecl_parameter.approve"), h.Approve)
	g.POST("/:id/approve2", auth.RequirePermission("ecl_parameter.approve"), h.Approve2)
	g.POST("/:id/reject", auth.RequirePermission("ecl_parameter.reject"), h.Reject)
}
