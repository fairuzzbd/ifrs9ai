// Package lookthrough — route registration for 12 look-through ECL endpoints.
package lookthrough

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all look-through ECL endpoints under the provided router group
// (expected to be /api/v1).
//
// Route layout (per api/openapi/app-c-lookthrough.yaml):
//
//	POST  /ecl/lookthrough/composition/submit           submitFundComposition
//	POST  /ecl/lookthrough/composition/:id/review       reviewFundComposition
//	POST  /ecl/lookthrough/composition/:id/approve      approveFundComposition
//	POST  /ecl/lookthrough/composition/:id/reject       rejectFundComposition
//	GET   /ecl/lookthrough/compositions                 listFundCompositions
//	GET   /ecl/lookthrough/compositions/:id             getFundComposition
//	POST  /ecl/lookthrough/compute                      computeLookthrough
//	POST  /ecl/lookthrough/compute/bulk                 bulkComputeLookthrough
//	GET   /ecl/lookthrough/preview                      listLookthroughPreview
//	GET   /ecl/lookthrough/preview/export               exportLookthroughPreview
//	GET   /ecl/lookthrough/result/:instrumenId/:runId   getLookthroughResult
//	POST  /ecl/lookthrough/composition/:id/amend        amendFundComposition
//
// Auth: all routes require JWT via auth.Middleware.
// Idempotency-Key: POST mutating endpoints checked via middleware.Idempotency (DEC-021).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	lt := authed.Group("/ecl/lookthrough")
	{
		// Fund composition workflow.
		compGroup := lt.Group("/composition")
		{
			compGroup.POST("/submit", idmp, h.SubmitComposition)
			compGroup.POST("/:id/review", idmp, h.ReviewComposition)
			compGroup.POST("/:id/approve", idmp, h.ApproveComposition)
			compGroup.POST("/:id/reject", idmp, h.RejectComposition)
			// Amend: same submit endpoint with isAmendment=true;
			// exposed as dedicated route for clarity (same handler, per AC-LKT-004).
			compGroup.POST("/:id/amend", idmp, h.SubmitComposition)
		}

		// Fund composition read.
		lt.GET("/compositions", h.ListCompositions)
		lt.GET("/compositions/:id", h.GetComposition)

		// ECL compute.
		lt.POST("/compute", idmp, h.ComputeLookthrough)
		lt.POST("/compute/bulk", idmp, h.BulkComputeLookthrough)

		// Preview DataTable (read-only).
		lt.GET("/preview", h.ListLookthroughPreview)
		lt.GET("/preview/export", h.ExportLookthroughPreview)

		// Result detail.
		lt.GET("/result/:instrumenId/:runId", h.GetLookthroughResult)
	}
}
