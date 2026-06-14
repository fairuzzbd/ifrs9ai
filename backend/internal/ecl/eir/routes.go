// Package eir — route registration for EIR endpoints (M5 + M6).
package eir

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all EIR endpoints under the provided router group (/api/v1).
//
// Route layout (api/openapi/app-c-eir.yaml + app-c-eir-amendment-lifecycle.yaml):
//
//	POST   /ecl/eir/compute                            computeEIR          (APP-C-EIR-001)
//	POST   /ecl/eir/generate-schedule                  generateSchedule    (APP-C-EIR-002)
//	GET    /ecl/eir/schedule/:instrumenId               getActiveSchedule   (APP-C-EIR-003)
//	GET    /ecl/eir/schedule/:instrumenId/history       getScheduleHistory  (APP-C-EIR-003)
//	POST   /ecl/eir/amendments                          proposeAmendment    (APP-C-EIR-004)
//	GET    /ecl/eir/amendments                          listAmendments      (APP-C-EIR-004)
//	GET    /ecl/eir/amendments/:id                      getAmendment        (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/:id/review               reviewAmendment     (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/:id/approve              approveAmendment    (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/:id/reject               rejectAmendment     (APP-C-EIR-004)
//	POST   /ecl/eir/bulk-recompute                      bulkRecompute       (APP-C-EIR-005)
//	--- M6 additions ---
//	POST   /ecl/eir/amendments/detect                   detectAmendment     (M6-001)
//	POST   /ecl/eir/amendments/:id/cancel               cancelAmendment     (M6-005)
//	PATCH  /ecl/eir/amendments/:id/cashflows            updateCashflows     (M6-003)
//	GET    /ecl/eir/amendments/queue                    listAmendmentQueue  (M6-004)
//	GET    /ecl/eir/amendments/queue/export             exportAmendmentQueue(M6-004)
//	GET    /ecl/eir/drift-reports                       listDriftReports    (M6-002)
//	GET    /ecl/eir/drift-reports/:id                   getDriftReport      (M6-002)
//	POST   /ecl/eir/drift-reports/generate              generateDriftReport (M6-002 ad-hoc)
//
// IMPORTANT: Static sub-paths (/queue, /detect) MUST be registered before parameterized (:id)
// to avoid Gin routing ambiguity.
//
// Auth: all routes require JWT via auth.Middleware.
// Idempotency-Key: POST mutating endpoints require header (DEC-021).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	eirGroup := authed.Group("/ecl/eir")
	{
		// EIR computation (Story 1)
		eirGroup.POST("/compute", idmp, h.ComputeEIR)

		// Schedule generation (Story 2)
		eirGroup.POST("/generate-schedule", idmp, h.GenerateSchedule)

		// Schedule DataTable (Story 3)
		eirGroup.GET("/schedule/:instrumenId", h.GetActiveSchedule)
		eirGroup.GET("/schedule/:instrumenId/history", h.GetScheduleHistory)

		// --- Static amendment paths FIRST to avoid :id conflict ---
		// M6-001: detect from document
		eirGroup.POST("/amendments/detect", idmp, h.DetectAmendment)
		// M6-004: review queue
		eirGroup.GET("/amendments/queue", h.ListAmendmentQueue)
		eirGroup.GET("/amendments/queue/export", h.ExportAmendmentQueue)

		// Amendment workflow (Story 4) — parameterized routes after static
		eirGroup.POST("/amendments", idmp, h.ProposeAmendment)
		eirGroup.GET("/amendments", h.ListAmendments)
		eirGroup.GET("/amendments/:id", h.GetAmendment)
		eirGroup.POST("/amendments/:id/review", idmp, h.ReviewAmendment)
		eirGroup.POST("/amendments/:id/approve", idmp, h.ApproveAmendment)
		eirGroup.POST("/amendments/:id/reject", idmp, h.RejectAmendment)
		// M6-005: cancel
		eirGroup.POST("/amendments/:id/cancel", idmp, h.CancelAmendment)
		// M6-003 PATCH cashflows
		eirGroup.PATCH("/amendments/:id/cashflows", idmp, h.UpdateCashflows)

		// Bulk re-compute (Story 5)
		eirGroup.POST("/bulk-recompute", idmp, h.BulkRecompute)

		// --- Drift reports (M6-002) ---
		// Static "generate" path FIRST
		eirGroup.POST("/drift-reports/generate", idmp, h.GenerateDriftReport)
		eirGroup.GET("/drift-reports", h.ListDriftReports)
		eirGroup.GET("/drift-reports/:id", h.GetDriftReport)
	}
}
