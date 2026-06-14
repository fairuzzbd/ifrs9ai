// Package ratinghistory — route registration for rating_history endpoints.
//
// Route layout:
//
//	GET    /master/rating-history                                      → List          (rating_history.read)
//	POST   /master/rating-history                                      → Create        (rating_history.create)
//	GET    /master/rating-history/export                               → Export        (rating_history.read)
//	GET    /master/rating-history/:id                                  → GetByID       (rating_history.read)
//	PUT    /master/rating-history/:id                                  → Update        (rating_history.update)
//	DELETE /master/rating-history/:id                                  → Delete        (rating_history.delete)
//	GET    /master/rating-history/:id/history                          → History       (rating_history.read)
//	GET    /master/rating-history/:id/workflow                         → WorkflowStatus(rating_history.read)
//	POST   /master/rating-history/:id/submit                           → Submit        (rating_history.submit)
//	POST   /master/rating-history/:id/review                           → Review        (rating_history.review)
//	POST   /master/rating-history/:id/approve                          → Approve       (rating_history.approve)
//	POST   /master/rating-history/:id/reject                           → Reject        (rating_history.reject)
//
// Also registered under counterparty (nested resource):
//
//	GET    /master/counterparty/:counterpartyId/rating-history → ListByCounterparty (rating_history.read)
package ratinghistory

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all rating_history HTTP routes under the given /api/v1 router group.
// Also requires the counterparty group to register the nested route.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	rg := v1.Group("/master/rating-history")

	// Collection endpoints
	rg.GET("", auth.RequirePermission("rating_history.read"), h.List)
	rg.POST("", auth.RequirePermission("rating_history.create"), h.Create)

	// Export — registered before /:id to avoid path conflict.
	rg.GET("/export", auth.RequirePermission("rating_history.read"), h.Export)

	// Single-record endpoints
	rg.GET("/:id", auth.RequirePermission("rating_history.read"), h.GetByID)
	rg.PUT("/:id", auth.RequirePermission("rating_history.update"), h.Update)
	rg.DELETE("/:id", auth.RequirePermission("rating_history.delete"), h.Delete)

	// Sub-resources
	rg.GET("/:id/history", auth.RequirePermission("rating_history.read"), h.History)
	rg.GET("/:id/workflow", auth.RequirePermission("rating_history.read"), h.WorkflowStatus)

	// Workflow mutation endpoints
	rg.POST("/:id/submit", auth.RequirePermission("rating_history.submit"), h.Submit)
	rg.POST("/:id/review", auth.RequirePermission("rating_history.review"), h.Review)
	rg.POST("/:id/approve", auth.RequirePermission("rating_history.approve"), h.Approve)
	rg.POST("/:id/reject", auth.RequirePermission("rating_history.reject"), h.Reject)
}

// RegisterCounterpartyNestedRoutes registers the nested rating-history route under counterparty.
// Call this after RegisterRoutes in cmd/api/main.go.
//
//	GET /master/counterparty/:counterpartyId/rating-history
func RegisterCounterpartyNestedRoutes(v1 *gin.RouterGroup, h *Handler) {
	v1.GET(
		"/master/counterparty/:counterpartyId/rating-history",
		auth.RequirePermission("rating_history.read"),
		h.ListByCounterparty,
	)
}
