// Package lps — route registration for LPS Aggregator endpoints.
package lps

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all LPS Aggregator endpoints under the provided router group
// (expected to be /api/v1).
//
// Route layout (per api/openapi/app-c-lps.yaml):
//
//	POST  /ecl/lps/aggregate              aggregateLpsSingle
//	POST  /ecl/lps/aggregate/bulk         aggregateLpsBulk
//	GET   /ecl/lps/preview                listLpsPreview
//	GET   /ecl/lps/preview/export         exportLpsPreview
//	POST  /ecl/lps/override/submit        submitLpsExclusionOverride
//	POST  /ecl/lps/override/:id/approve   approveLpsExclusionOverride
//	POST  /ecl/lps/override/:id/reject    rejectLpsExclusionOverride
//	GET   /ecl/lps/overrides              listLpsExclusionOverrides
//	GET   /ecl/lps/overrides/:id          getLpsExclusionOverride
//
// Auth: all routes require JWT via auth.Middleware.
// Idempotency-Key: POST mutating endpoints checked via middleware.Idempotency (DEC-021).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	lpsGroup := authed.Group("/ecl/lps")
	{
		// Aggregate.
		lpsGroup.POST("/aggregate", idmp, h.AggregateSingle)
		lpsGroup.POST("/aggregate/bulk", idmp, h.AggregateBulk)

		// Preview DataTable (read-only).
		lpsGroup.GET("/preview", h.ListPreview)
		lpsGroup.GET("/preview/export", h.ExportPreview)

		// Override list + detail (read-only).
		lpsGroup.GET("/overrides", h.ListOverrides)
		lpsGroup.GET("/overrides/:id", h.GetOverride)

		// Override mutations.
		overrideGroup := lpsGroup.Group("/override")
		{
			overrideGroup.POST("/submit", idmp, h.SubmitOverride)
			overrideGroup.POST("/:id/approve", idmp, h.ApproveOverride)
			overrideGroup.POST("/:id/reject", idmp, h.RejectOverride)
		}
	}
}
