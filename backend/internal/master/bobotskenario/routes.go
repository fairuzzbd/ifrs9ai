// Package bobotskenario — route registration for mst.bobot_skenario endpoints.
//
// REUSE PATTERN: Every master module creates a RegisterRoutes func with the same signature:
//
//	func RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
//
// IMPORTANT: fixed-path routes (/export, /seed-default) MUST be registered BEFORE
// /:id to avoid Gin treating those literals as :id values.
package bobotskenario

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all bobot_skenario HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/bobot-skenario                          → List             (ecl_parameter.read)
//	POST   /master/bobot-skenario                          → Create           (ecl_parameter.create)
//	GET    /master/bobot-skenario/export                   → Export           (ecl_parameter.read)
//	POST   /master/bobot-skenario/seed-default             → SeedDefault      (ecl_parameter.submit)
//	GET    /master/bobot-skenario/:id                      → GetByID          (ecl_parameter.read)
//	PATCH  /master/bobot-skenario/:id                      → Update           (ecl_parameter.update)
//	DELETE /master/bobot-skenario/:id                      → Delete           (ecl_parameter.delete)
//	GET    /master/bobot-skenario/:id/history              → History          (ecl_parameter.read)
//	GET    /master/bobot-skenario/:id/workflow             → WorkflowStatus   (ecl_parameter.read)
//	POST   /master/bobot-skenario/:id/submit               → Submit           (ecl_parameter.submit)
//	POST   /master/bobot-skenario/:id/review               → Review           (ecl_parameter.review)
//	POST   /master/bobot-skenario/:id/approve              → Approve          (ecl_parameter.approve)
//	POST   /master/bobot-skenario/:id/approve2             → Approve2         (ecl_parameter.approve)
//	POST   /master/bobot-skenario/:id/reject               → Reject           (ecl_parameter.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	bs := v1.Group("/master/bobot-skenario")

	// ── Collection endpoints ──────────────────────────────────────────────────
	bs.GET("", auth.RequirePermission(PermRead), h.List)
	bs.POST("", auth.RequirePermission(PermCreate), h.Create)

	// Fixed-path sub-routes — registered BEFORE /:id to avoid path conflict.
	bs.GET("/export", auth.RequirePermission(PermRead), h.Export)
	bs.POST("/seed-default", auth.RequirePermission(PermSubmit), h.SeedDefault)

	// ── Single-record endpoints ───────────────────────────────────────────────
	bs.GET("/:id", auth.RequirePermission(PermRead), h.GetByID)
	bs.PATCH("/:id", auth.RequirePermission(PermUpdate), h.Update)
	bs.DELETE("/:id", auth.RequirePermission(PermDelete), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	bs.GET("/:id/history", auth.RequirePermission(PermRead), h.History)
	bs.GET("/:id/workflow", auth.RequirePermission(PermRead), h.WorkflowStatus)

	// Workflow mutation endpoints (all require Idempotency-Key via global middleware).
	// 6-eyes path: submit → review → approve → approve2 (both approve steps require step-up MFA).
	bs.POST("/:id/submit", auth.RequirePermission(PermSubmit), h.Submit)
	bs.POST("/:id/review", auth.RequirePermission(PermReview), h.Review)
	bs.POST("/:id/approve", auth.RequirePermission(PermApprove), h.Approve)
	bs.POST("/:id/approve2", auth.RequirePermission(PermApprove), h.Approve2) // same permission as approve
	bs.POST("/:id/reject", auth.RequirePermission(PermReject), h.Reject)
}
