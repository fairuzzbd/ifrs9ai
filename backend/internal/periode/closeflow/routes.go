package closeflow

// routes.go — Register P5-M4 close workflow routes under Gin router.
//
// Route groups:
//   /api/v1/periode-buku/:periode_id  — workflow endpoints (10 total)
//   /api/v1/reports/status-periode    — list + export

import "github.com/gin-gonic/gin"

// RegisterRoutes attaches all P5-M4 close-workflow routes to the provided Gin engine.
// The caller is responsible for mounting auth middleware on the router before calling this.
//
// Parameters:
//   - r: Gin engine (or test engine).
//   - h: Handler instance.
//   - lockMiddleware: PeriodeLockMiddleware (mounted on mutation routes only).
func RegisterRoutes(r *gin.Engine, h *Handler, lockMiddleware *PeriodeLockMiddleware) {
	api := r.Group("/api/v1")

	// Close-workflow endpoints — these are the state-transition endpoints themselves,
	// so they are NOT behind the PeriodeLockMiddleware (they mutate the lock state).
	pb := api.Group("/periode-buku/:periode_id")
	{
		pb.POST("/soft-close-request", h.SoftCloseRequest)
		pb.POST("/soft-close-approve", h.SoftCloseApprove)
		pb.POST("/hard-close-request", h.HardCloseRequest)
		pb.POST("/hard-close-approve", h.HardCloseApprove)
		pb.POST("/hard-close-reject", h.HardCloseReject)
		pb.POST("/reopen-request", h.ReopenRequest)
		pb.POST("/reopen-approve", h.ReopenApprove)
		pb.GET("/closing-checklist", h.GetClosingChecklist)
	}

	// Reporting endpoints.
	rpt := api.Group("/reports")
	{
		rpt.GET("/status-periode", h.ListStatusPeriode)
		rpt.GET("/status-periode/export", h.ExportStatusPeriode)
	}
}

// RegisterLockMiddlewareRoutes registers the PeriodeLockMiddleware on a given route group.
// Call this for any route group that carries :periode_id and performs domain mutations.
//
// Example:
//
//	trxGroup := r.Group("/api/v1/transaksi/:periode_id", lockMiddleware.Handler())
func RegisterLockMiddlewareRoutes(group *gin.RouterGroup, lockMiddleware *PeriodeLockMiddleware) {
	group.Use(lockMiddleware.Handler())
}
