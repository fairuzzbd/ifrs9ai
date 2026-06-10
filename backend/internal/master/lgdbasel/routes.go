// Package lgdbasel — route registration for mst.lgd_basel endpoints.
//
// REUSE PATTERN: Every master module creates a RegisterRoutes func with the same signature:
//
//	func RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
//
// The caller (cmd/api/main.go) just calls RegisterRoutes and wires up dependencies.
//
// IMPORTANT: /master/lgd-basel/export MUST be registered BEFORE /master/lgd-basel/:id
// to avoid Gin treating "export" as an :id value.
package lgdbasel

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all lgd_basel HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/lgd-basel                     → List          (ecl_parameter.read)
//	POST   /master/lgd-basel                     → Create        (ecl_parameter.create)
//	GET    /master/lgd-basel/export              → Export        (ecl_parameter.read)
//	GET    /master/lgd-basel/:id                 → GetByID       (ecl_parameter.read)
//	PATCH  /master/lgd-basel/:id                 → Update        (ecl_parameter.update)
//	DELETE /master/lgd-basel/:id                 → Delete        (ecl_parameter.delete)
//	GET    /master/lgd-basel/:id/history         → History       (ecl_parameter.read)
//	GET    /master/lgd-basel/:id/workflow        → WorkflowStatus(ecl_parameter.read)
//	POST   /master/lgd-basel/:id/submit          → Submit        (ecl_parameter.submit)
//	POST   /master/lgd-basel/:id/review          → Review        (ecl_parameter.review)
//	POST   /master/lgd-basel/:id/approve         → Approve       (ecl_parameter.approve)
//	POST   /master/lgd-basel/:id/approve2        → Approve2      (ecl_parameter.approve)
//	POST   /master/lgd-basel/:id/reject          → Reject        (ecl_parameter.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	lg := v1.Group("/master/lgd-basel")

	// ── Collection endpoints ──────────────────────────────────────────────────
	lg.GET("", auth.RequirePermission(PermRead), h.List)
	lg.POST("", auth.RequirePermission(PermCreate), h.Create)

	// Export — registered before /:id to avoid path conflict.
	lg.GET("/export", auth.RequirePermission(PermRead), h.Export)

	// ── Single-record endpoints ───────────────────────────────────────────────
	lg.GET("/:id", auth.RequirePermission(PermRead), h.GetByID)
	lg.PATCH("/:id", auth.RequirePermission(PermUpdate), h.Update)
	lg.DELETE("/:id", auth.RequirePermission(PermDelete), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	lg.GET("/:id/history", auth.RequirePermission(PermRead), h.History)
	lg.GET("/:id/workflow", auth.RequirePermission(PermRead), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	// 6-eyes path: submit → review → approve → approve2 (both approve steps require step-up MFA).
	lg.POST("/:id/submit", auth.RequirePermission(PermSubmit), h.Submit)
	lg.POST("/:id/review", auth.RequirePermission(PermReview), h.Review)
	lg.POST("/:id/approve", auth.RequirePermission(PermApprove), h.Approve)
	lg.POST("/:id/approve2", auth.RequirePermission(PermApprove), h.Approve2) // same permission as approve
	lg.POST("/:id/reject", auth.RequirePermission(PermReject), h.Reject)
}
