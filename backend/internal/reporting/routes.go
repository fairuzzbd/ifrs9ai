// Package reporting — route registration for P5-M13 APP-E Reporting MV Foundation.
package reporting

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all reporting endpoints under the provided /api/v1 router group.
//
// Route layout (api/openapi/app-e-reporting-mv.yaml):
//
//	GET    /admin/mv-status                              GetMVStatus         (S1-AC1)
//	POST   /admin/mv-refresh                             PostMVRefresh       (S2-AC1)
//	GET    /reports/:slug/export                         GetReportExport     (S3-AC1)
//	GET    /reports/export-log                           GetExportLog        (S4-AC2)
//	GET    /reports/export/:export_id/download           GetExportDownload   (S4-AC1)
//	GET    /reports/scheduled-emails                     GetScheduledEmails  (S5-AC1)
//	POST   /reports/scheduled-emails                     PostScheduledEmail  (S5-AC1)
//	DELETE /reports/scheduled-emails/:id                 DeleteScheduledEmail(S5-AC1)
//	POST   /reports/scheduled-emails/:id/opt-out         PostOptOut          (S5-AC4 — no auth)
//
// Auth: JWT via auth.Middleware for all except opt-out (security: [] in OpenAPI).
// Rate limit on POST /admin/mv-refresh: 10 req/min (sensitive admin endpoint).
// Idempotency-Key: POST /admin/mv-refresh, POST /reports/scheduled-emails.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	// --- Admin endpoints ---
	adminGroup := authed.Group("/admin")
	{
		adminGroup.GET("/mv-status", h.GetMVStatus)
		adminGroup.POST("/mv-refresh", idmp, h.PostMVRefresh)
	}

	// --- Report export endpoints ---
	reportsGroup := authed.Group("/reports")
	{
		// Static sub-paths must be declared before :slug to avoid Gin conflicts.
		reportsGroup.GET("/export-log", h.GetExportLog)
		reportsGroup.GET("/export/:export_id/download", h.GetExportDownload)
		reportsGroup.GET("/scheduled-emails", h.GetScheduledEmails)
		reportsGroup.POST("/scheduled-emails", idmp, h.PostScheduledEmail)
		reportsGroup.DELETE("/scheduled-emails/:id", h.DeleteScheduledEmail)

		// Dynamic slug (must come after static sub-paths).
		reportsGroup.GET("/:slug/export", h.GetReportExport)
	}

	// --- Opt-out (no auth — HMAC token only) ---
	// Registered on the base group (no auth.Middleware).
	rg.POST("/reports/scheduled-emails/:id/opt-out", h.PostOptOut)
}
