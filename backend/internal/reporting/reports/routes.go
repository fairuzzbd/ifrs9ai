package reports

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers the 3 M14 report endpoints under /api/v1.
//
// Route layout (api/openapi/app-e-reports.yaml):
//
//	GET  /reports/:slug         GetReport        — list any of 25 report slugs
//	GET  /reports/:slug/export  ExportReport     — inline or async export
//	POST /reports/rpt-28/export ExportRegulatorPackHandler — special RPT-28
//
// IMPORTANT: the /reports/rpt-28/export POST must be registered before the
// catch-all /:slug GET to avoid Gin path conflicts. In practice, GET and POST
// on the same path prefix don't conflict in Gin's router.
//
// Auth: JWT via auth.Middleware on all routes.
// Idempotency-Key: required on export endpoints (DEC-021).
// Read-replica routing: handled inside ReportService.chooseDB().
func RegisterRoutes(rg *gin.RouterGroup, h *ReportHandler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	reports := authed.Group("/reports")
	{
		// RPT-28 special POST — must be before generic :slug to avoid ambiguity.
		reports.POST("/rpt-28/export", idmp, h.ExportRegulatorPackHandler)

		// Generic list: GET /reports/:slug
		reports.GET("/:slug", h.GetReport)

		// Generic export: GET /reports/:slug/export
		// Note: Gin registers /reports/:slug and /reports/:slug/export as
		// distinct routes (exact suffix match), no conflict.
		reports.GET("/:slug/export", idmp, h.ExportReport)
	}
}
