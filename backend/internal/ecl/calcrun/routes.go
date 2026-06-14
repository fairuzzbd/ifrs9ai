// Package calcrun — route registration for ECL Calc Run endpoints (P4-M8).
package calcrun

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all ECL Calc Run endpoints under the provided router group (/api/v1).
//
// Route layout (api/openapi/app-c-calc-run.yaml + docs/state-machines/p4-m8-calc-run.md):
//
//	POST   /ecl/calc-runs                              createCalcRun         (M8-001)
//	GET    /ecl/calc-runs                              listCalcRuns          (M8-003)
//	GET    /ecl/calc-runs/:id                          getCalcRun            (M8-003)
//	POST   /ecl/calc-runs/:id/start                    startCalcRun          (M8-002)
//	POST   /ecl/calc-runs/:id/cancel                   cancelCalcRun         (M8-005)
//	GET    /ecl/calc-runs/:id/parameter-snapshot       getParameterSnapshot  (M8-002)
//	GET    /ecl/calc-runs/:id/result-lines             listResultLines       (M8-003)
//	POST   /ecl/calc-runs/:id/seal/request             requestSeal           (M8-004)
//	POST   /ecl/calc-runs/:id/seal/approve             approveSeal           (M8-004, step-up MFA)
//	POST   /ecl/calc-runs/:id/seal/reject              rejectSeal            (M8-004)
//
// IMPORTANT: static sub-path "seal/request" etc. are resolved unambiguously by Gin
// because the :id segment appears before /seal. No ordering conflicts.
//
// Auth: all routes require JWT via auth.Middleware (DEC-025).
// Idempotency-Key: POST mutating endpoints require header (DEC-021).
// Step-up MFA: enforced inside ApproveSeal handler (DEC-027).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	calcRunGroup := authed.Group("/ecl/calc-runs")
	{
		// --- Collection-level ---
		calcRunGroup.POST("", idmp, h.CreateCalcRun)
		calcRunGroup.GET("", h.ListCalcRuns)

		// --- Resource-level ---
		calcRunGroup.GET("/:id", h.GetCalcRun)
		calcRunGroup.POST("/:id/start", idmp, h.StartCalcRun)
		calcRunGroup.POST("/:id/cancel", idmp, h.CancelCalcRun)
		calcRunGroup.GET("/:id/parameter-snapshot", h.GetParameterSnapshot)
		calcRunGroup.GET("/:id/result-lines", h.ListResultLines)

		// --- Seal sub-workflow ---
		calcRunGroup.POST("/:id/seal/request", idmp, h.RequestSeal)
		calcRunGroup.POST("/:id/seal/approve", idmp, h.ApproveSeal)
		calcRunGroup.POST("/:id/seal/reject", idmp, h.RejectSeal)
	}
}
