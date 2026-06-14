// Package coa — route registration for mst.chart_of_accounts endpoints.
//
// IMPORTANT: /master/coa/export and /master/coa/import-xlsx MUST be registered
// BEFORE /master/coa/:id to avoid Gin treating "export" or "import-xlsx" as an
// :id value.
package coa

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all chart_of_accounts HTTP routes under /api/v1.
//
// Route layout:
//
//	GET    /master/coa                          → List            (chart_of_accounts.read)
//	POST   /master/coa                          → Create          (chart_of_accounts.create)
//	GET    /master/coa/export                   → Export          (chart_of_accounts.read)
//	POST   /master/coa/import-xlsx              → ImportXLSX      (chart_of_accounts.create)
//	GET    /master/coa/import-jobs/:jobId       → GetImportJob    (chart_of_accounts.read)
//	GET    /master/coa/:id                      → Get             (chart_of_accounts.read)
//	PATCH  /master/coa/:id                      → Update          (chart_of_accounts.update)
//	DELETE /master/coa/:id                      → Delete          (chart_of_accounts.delete)
//	GET    /master/coa/:id/history              → History         (chart_of_accounts.read)
//	GET    /master/coa/:id/workflow             → WorkflowStatus  (chart_of_accounts.read)
//	POST   /master/coa/:id/submit               → Submit          (chart_of_accounts.submit)
//	POST   /master/coa/:id/review               → Review          (chart_of_accounts.review)
//	POST   /master/coa/:id/approve              → Approve         (chart_of_accounts.approve)
//	POST   /master/coa/:id/reject               → Reject          (chart_of_accounts.reject)
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	cg := v1.Group("/master/coa")

	// ── Collection endpoints ──────────────────────────────────────────────────
	cg.GET("", auth.RequirePermission("chart_of_accounts.read"), h.List)
	cg.POST("", auth.RequirePermission("chart_of_accounts.create"), h.Create)

	// ── Static sub-paths before /:id to avoid path conflict ──────────────────
	cg.GET("/export", auth.RequirePermission("chart_of_accounts.read"), h.Export)
	cg.POST("/import-xlsx", auth.RequirePermission("chart_of_accounts.create"), h.ImportXLSX)
	cg.GET("/import-jobs/:jobId", auth.RequirePermission("chart_of_accounts.read"), h.GetImportJob)

	// ── Single-record endpoints ───────────────────────────────────────────────
	cg.GET("/:id", auth.RequirePermission("chart_of_accounts.read"), h.Get)
	cg.PATCH("/:id", auth.RequirePermission("chart_of_accounts.update"), h.Update)
	cg.DELETE("/:id", auth.RequirePermission("chart_of_accounts.delete"), h.Delete)

	// ── Sub-resources ─────────────────────────────────────────────────────────
	cg.GET("/:id/history", auth.RequirePermission("chart_of_accounts.read"), h.History)
	cg.GET("/:id/workflow", auth.RequirePermission("chart_of_accounts.read"), h.WorkflowStatus)

	// ── Workflow mutation endpoints ────────────────────────────────────────────
	cg.POST("/:id/submit", auth.RequirePermission("chart_of_accounts.submit"), h.Submit)
	cg.POST("/:id/review", auth.RequirePermission("chart_of_accounts.review"), h.Review)
	cg.POST("/:id/approve", auth.RequirePermission("chart_of_accounts.approve"), h.Approve)
	cg.POST("/:id/reject", auth.RequirePermission("chart_of_accounts.reject"), h.Reject)
}
