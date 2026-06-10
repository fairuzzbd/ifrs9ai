// Package periodebuku — route registration for mst.periode_buku endpoints.
//
// REUSE PATTERN: same signature as matauang.RegisterRoutes.
//
//	func RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
//
// Route layout (aligned with APP-D-MSTR-001 spec):
//
//	GET    /master/periode-buku                    → List          (periode.read)
//	POST   /master/periode-buku                    → Create        (periode.create)
//	GET    /master/periode-buku/export             → Export        (periode.read)
//	POST   /master/periode-buku/generate           → Generate      (periode.create)
//	GET    /master/periode-buku/:id                → GetByID       (periode.read)
//	PATCH  /master/periode-buku/:id               → Update        (periode.update)
//	DELETE /master/periode-buku/:id               → Delete        (periode.delete)
//	GET    /master/periode-buku/:id/history        → History       (periode.read)
//	GET    /master/periode-buku/:id/workflow       → WorkflowStatus(periode.read)
//	POST   /master/periode-buku/:id/submit         → Submit        (periode.submit)
//	POST   /master/periode-buku/:id/review         → Review        (periode.review)
//	POST   /master/periode-buku/:id/approve        → Approve       (periode.approve)
//	POST   /master/periode-buku/:id/reject         → Reject        (periode.reject)
//
// IMPORTANT: static sub-paths (/export, /generate) MUST be registered BEFORE /:id
// to avoid Gin treating literal path segments as :id.
//
// TODO (Phase 5 — APP-D): register period lifecycle endpoints here:
//   - POST /master/periode-buku/:id/softclose
//   - POST /master/periode-buku/:id/hardclose  (CFO MFA step-up, DEC-027)
//   - POST /master/periode-buku/:id/reopen
package periodebuku

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all periode_buku HTTP routes under the given /api/v1 router group.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	pg := v1.Group("/master/periode-buku")

	// ── Collection endpoints ──────────────────────────────────────────────────
	pg.GET("", auth.RequirePermission("periode.read"), h.List)
	pg.POST("", auth.RequirePermission("periode.create"), h.Create)

	// Static sub-paths registered BEFORE /:id to avoid path conflict.
	pg.GET("/export", auth.RequirePermission("periode.read"), h.Export)
	pg.POST("/generate", auth.RequirePermission("periode.create"), h.Generate)

	// ── Single-record endpoints ───────────────────────────────────────────────
	pg.GET("/:id", auth.RequirePermission("periode.read"), h.GetByID)
	pg.PATCH("/:id", auth.RequirePermission("periode.update"), h.Update)
	pg.DELETE("/:id", auth.RequirePermission("periode.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	pg.GET("/:id/history", auth.RequirePermission("periode.read"), h.History)
	pg.GET("/:id/workflow", auth.RequirePermission("periode.read"), h.WorkflowStatus)

	// Workflow mutation endpoints (Idempotency-Key enforced by global middleware).
	pg.POST("/:id/submit", auth.RequirePermission("periode.submit"), h.Submit)
	pg.POST("/:id/review", auth.RequirePermission("periode.review"), h.Review)
	pg.POST("/:id/approve", auth.RequirePermission("periode.approve"), h.Approve)
	pg.POST("/:id/reject", auth.RequirePermission("periode.reject"), h.Reject)

	// TODO Phase 5 — APP-D Periode Buku lifecycle:
	// pg.POST("/:id/softclose", auth.RequirePermission("periode.softclose"), h.SoftClose)
	// pg.POST("/:id/hardclose", auth.RequirePermission("periode.hardclose"), h.HardClose) // MFA step-up DEC-027
	// pg.POST("/:id/reopen",    auth.RequirePermission("periode.reopen"),    h.Reopen)
}
