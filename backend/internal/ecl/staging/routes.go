// Package staging — route registration for ECL Staging Engine endpoints.
package staging

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all staging engine endpoints under the provided router group
// (expected to be /api/v1).
//
// Route layout (per api/openapi/app-c-staging.yaml):
//
//	POST   /ecl/staging/evaluate
//	GET    /ecl/staging/instrumen/:id
//	GET    /ecl/staging/instrumen/:id/history
//	POST   /ecl/staging/override/submit
//	POST   /ecl/staging/override/:id/review
//	POST   /ecl/staging/override/:id/approve
//	POST   /ecl/staging/override/:id/approve2
//	POST   /ecl/staging/override/:id/reject
//	GET    /ecl/staging/overrides
//	POST   /ecl/dpd/record
//	GET    /ecl/dpd/instrumen/:id
//
// Auth: all routes require JWT via auth.Middleware.
// Idempotency: POST endpoints require Idempotency-Key via middleware.Idempotency (DEC-021).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	// All staging routes require JWT authentication.
	authed := rg.Group("", auth.Middleware(v))

	idmp := middleware.Idempotency(db)

	// Staging evaluation + query.
	stagingGroup := authed.Group("/ecl/staging")
	{
		stagingGroup.POST("/evaluate", idmp, h.EvaluateHandler)
		stagingGroup.GET("/instrumen/:id", h.GetCurrentStageHandler)
		stagingGroup.GET("/instrumen/:id/history", h.GetHistoryHandler)
		stagingGroup.GET("/overrides", h.ListOverridesHandler)
	}

	// Override proposal workflow.
	overrideGroup := authed.Group("/ecl/staging/override")
	{
		overrideGroup.POST("/submit", idmp, h.SubmitOverrideHandler)
		overrideGroup.POST("/:id/review", idmp, h.ReviewOverrideHandler)
		overrideGroup.POST("/:id/approve", idmp, h.ApproveALCOHandler)
		overrideGroup.POST("/:id/approve2", idmp, h.ApproveKomiteHandler)
		overrideGroup.POST("/:id/reject", idmp, h.RejectOverrideHandler)
	}

	// DPD records.
	dpdGroup := authed.Group("/ecl/dpd")
	{
		dpdGroup.POST("/record", idmp, h.RecordDPDHandler)
		dpdGroup.GET("/instrumen/:id", h.GetDPDHistoryHandler)
	}
}
