package closeflow

// routes.go — Register P5-M4 close workflow routes under the v1 RouterGroup.
//
// F-02 fix: RegisterRoutes now accepts *gin.RouterGroup (the v1 group from main.go)
// instead of *gin.Engine. This ensures the Idempotency middleware mounted on the
// v1 group in main.go is inherited by all closeflow routes.
//
// Route groups:
//   /api/v1/periode-buku/:periode_id  — workflow endpoints (10 total)
//   /api/v1/reports/status-periode    — list + export

import "github.com/gin-gonic/gin"

// RegisterRoutes attaches all P5-M4 close-workflow routes to the v1 RouterGroup.
// The caller must pass the /api/v1 group (with Idempotency + Auth middleware already
// mounted) so that all closeflow endpoints inherit those middleware layers (F-02).
//
// Parameters:
//   - v1: the /api/v1 RouterGroup from main.go (must have Idempotency + Auth mounted).
//   - h: Handler instance.
//   - lockMiddleware: PeriodeLockMiddleware (may be nil in tests — skipped when nil).
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler, lockMiddleware *PeriodeLockMiddleware) {
	// Close-workflow endpoints — these are the state-transition endpoints themselves,
	// so they are NOT behind the PeriodeLockMiddleware (they mutate the lock state).
	pb := v1.Group("/periode-buku/:periode_id")
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
	rpt := v1.Group("/reports")
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
//	trxGroup := v1.Group("/transaksi/:periode_id", lockMiddleware.Handler())
func RegisterLockMiddlewareRoutes(group *gin.RouterGroup, lockMiddleware *PeriodeLockMiddleware) {
	group.Use(lockMiddleware.Handler())
}
