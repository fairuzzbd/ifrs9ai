// Package kurs — route registration for mst.kurs endpoints.
//
// Route layout:
//
//	GET    /master/kurs                      → List          (kurs.read)
//	POST   /master/kurs                      → Create        (kurs.create)
//	GET    /master/kurs/export               → Export        (kurs.read)
//	POST   /master/kurs/jisdor-sync          → JISDORSync    (kurs.jisdor_sync)
//	GET    /master/kurs/:id                  → GetByID       (kurs.read)
//	PUT    /master/kurs/:id                  → Update        (kurs.update)
//	DELETE /master/kurs/:id                  → Delete        (kurs.delete)
//	GET    /master/kurs/:id/history          → History       (kurs.read)
//	GET    /master/kurs/:id/workflow         → WorkflowStatus(kurs.read)
//	POST   /master/kurs/:id/submit           → Submit        (kurs.submit)
//	POST   /master/kurs/:id/review           → Review        (kurs.review)
//	POST   /master/kurs/:id/approve          → Approve       (kurs.approve)
//	POST   /master/kurs/:id/reject           → Reject        (kurs.reject)
//
// IMPORTANT: /export and /jisdor-sync MUST be registered BEFORE /:id to avoid
// Gin treating those literal strings as path param values.
package kurs

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all kurs HTTP routes under the given /api/v1 router group.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	kg := v1.Group("/master/kurs")

	// ── Collection endpoints ──────────────────────────────────────────────────
	kg.GET("", auth.RequirePermission("kurs.read"), h.List)
	kg.POST("", auth.RequirePermission("kurs.create"), h.Create)

	// Static sub-paths before /:id to avoid path conflicts.
	kg.GET("/export", auth.RequirePermission("kurs.read"), h.Export)
	kg.POST("/jisdor-sync", auth.RequirePermission("kurs.jisdor_sync"), h.JISDORSync)

	// ── Single-record endpoints ───────────────────────────────────────────────
	kg.GET("/:id", auth.RequirePermission("kurs.read"), h.GetByID)
	kg.PUT("/:id", auth.RequirePermission("kurs.update"), h.Update)
	kg.DELETE("/:id", auth.RequirePermission("kurs.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	kg.GET("/:id/history", auth.RequirePermission("kurs.read"), h.History)
	kg.GET("/:id/workflow", auth.RequirePermission("kurs.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	kg.POST("/:id/submit", auth.RequirePermission("kurs.submit"), h.Submit)
	kg.POST("/:id/review", auth.RequirePermission("kurs.review"), h.Review)
	kg.POST("/:id/approve", auth.RequirePermission("kurs.approve"), h.Approve)
	kg.POST("/:id/reject", auth.RequirePermission("kurs.reject"), h.Reject)
}
