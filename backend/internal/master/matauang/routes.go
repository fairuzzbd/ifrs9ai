// Package matauang — route registration for mst.mata_uang endpoints.
//
// REUSE PATTERN: Every master module creates a RegisterRoutes func with the same signature:
//
//	func RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
//
// The caller (cmd/api/main.go) just calls RegisterRoutes and wires up dependencies.
// The route group is /api/v1 (already has IdempotencyMiddleware + optionally AuthMiddleware).
package matauang

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all mata_uang HTTP routes under the given /api/v1 router group.
//
// Route layout (aligned with mata-uang.yaml):
//
//	GET    /master/mata-uang                    → List          (mata_uang.read)
//	POST   /master/mata-uang                    → Create        (mata_uang.create)
//	GET    /master/mata-uang/export             → Export        (mata_uang.read)
//	GET    /master/mata-uang/:kode              → GetByKode     (mata_uang.read)
//	PUT    /master/mata-uang/:kode              → Update        (mata_uang.update)
//	DELETE /master/mata-uang/:kode              → Delete        (mata_uang.delete)
//	GET    /master/mata-uang/:kode/history      → History       (mata_uang.read)
//	POST   /master/mata-uang/:kode/submit       → Submit        (mata_uang.submit)
//	POST   /master/mata-uang/:kode/review       → Review        (mata_uang.review)
//	POST   /master/mata-uang/:kode/approve      → Approve       (mata_uang.approve)
//	POST   /master/mata-uang/:kode/reject       → Reject        (mata_uang.reject)
//	GET    /master/mata-uang/:kode/workflow     → WorkflowStatus(mata_uang.read)
//
// IMPORTANT: /master/mata-uang/export MUST be registered BEFORE /master/mata-uang/:kode
// to avoid Gin treating "export" as a :kode value.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	mg := v1.Group("/master/mata-uang")

	// ── Collection endpoints ──────────────────────────────────────────────────
	mg.GET("", auth.RequirePermission("mata_uang.read"), h.List)
	mg.POST("", auth.RequirePermission("mata_uang.create"), h.Create)

	// Export — registered before /:kode to avoid path conflict.
	mg.GET("/export", auth.RequirePermission("mata_uang.read"), h.Export)

	// ── Single-record endpoints ───────────────────────────────────────────────
	mg.GET("/:kode", auth.RequirePermission("mata_uang.read"), h.GetByKode)
	mg.PUT("/:kode", auth.RequirePermission("mata_uang.update"), h.Update)
	mg.DELETE("/:kode", auth.RequirePermission("mata_uang.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	mg.GET("/:kode/history", auth.RequirePermission("mata_uang.read"), h.History)
	mg.GET("/:kode/workflow", auth.RequirePermission("mata_uang.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	mg.POST("/:kode/submit", auth.RequirePermission("mata_uang.submit"), h.Submit)
	mg.POST("/:kode/review", auth.RequirePermission("mata_uang.review"), h.Review)
	mg.POST("/:kode/approve", auth.RequirePermission("mata_uang.approve"), h.Approve)
	mg.POST("/:kode/reject", auth.RequirePermission("mata_uang.reject"), h.Reject)
}
