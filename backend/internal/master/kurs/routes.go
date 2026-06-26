// Package kurs — route registration for mst.kurs endpoints.
//
// Route layout:
//
//	GET    /master/kurs                               → List              (kurs.read)
//	POST   /master/kurs                               → Create            (kurs.create)
//	GET    /master/kurs/export                        → Export            (kurs.read)
//	POST   /master/kurs/jisdor-sync                   → JISDORSyncV2      (kurs.jisdor_sync)  [P5-M5]
//	POST   /master/kurs/upload                        → Upload            (kurs.upload)        [P5-M5]
//	POST   /master/kurs/upload/:batch_id/approve      → BatchApprove      (kurs.approve)       [P5-M5]
//	POST   /master/kurs/upload/:batch_id/reject       → BatchReject       (kurs.approve)       [P5-M5]
//	GET    /master/kurs/treatment/:instrumen_id        → Treatment         (kurs.read)          [P5-M5]
//	GET    /master/kurs/:id                           → GetByID           (kurs.read)
//	PUT    /master/kurs/:id                           → Update            (kurs.update)
//	DELETE /master/kurs/:id                           → Delete            (kurs.delete)
//	GET    /master/kurs/:id/history                   → History           (kurs.read)
//	GET    /master/kurs/:id/workflow                  → WorkflowStatus    (kurs.read)
//	POST   /master/kurs/:id/submit                    → Submit            (kurs.submit)
//	POST   /master/kurs/:id/review                    → Review            (kurs.review)
//	POST   /master/kurs/:id/approve                   → Approve           (kurs.approve)
//	POST   /master/kurs/:id/reject                    → Reject            (kurs.reject)
//
// IMPORTANT: all static sub-paths (/export, /jisdor-sync, /upload, /treatment/:id)
// MUST be registered BEFORE /:id to avoid Gin treating them as path param values.
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

	// ── Static sub-paths (MUST be before /:id) ────────────────────────────────

	// Export (read-only)
	kg.GET("/export", auth.RequirePermission("kurs.read"), h.Export)

	// JISDOR sync — P5-M5 replaces Phase 3 stub with full implementation.
	kg.POST("/jisdor-sync", auth.RequirePermission("kurs.jisdor_sync"), h.JISDORSyncV2)

	// P5-M5: Manual upload batch workflow.
	kg.POST("/upload", auth.RequirePermission("kurs.upload"), h.Upload)
	kg.POST("/upload/:batch_id/approve", auth.RequirePermission("kurs.approve"), h.BatchApprove)
	kg.POST("/upload/:batch_id/reject", auth.RequirePermission("kurs.approve"), h.BatchReject)

	// P5-M5: FX treatment decision.
	kg.GET("/treatment/:instrumen_id", auth.RequirePermission("kurs.read"), h.Treatment)

	// ── Single-record endpoints (parameterised — must come AFTER static paths) ─
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
