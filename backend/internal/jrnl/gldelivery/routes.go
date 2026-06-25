package gldelivery

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all GL Delivery endpoints under the given router group.
// All endpoints require JWT authentication. Mutating endpoints require Idempotency-Key.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	// ─── GL Delivery Status (per jurnal header) ───────────────────────────────
	// 1. GET /jurnal/header/:id/gl-delivery-status
	// 2. POST /jurnal/header/:id/retry-gl-delivery
	headerGroup := authed.Group("/jurnal/header/:id")
	{
		headerGroup.GET("/gl-delivery-status", h.GetDeliveryStatus)
		headerGroup.POST("/retry-gl-delivery", idmp, h.RetryGLDelivery)
	}

	// ─── DLQ Console ─────────────────────────────────────────────────────────
	// 3. GET /jurnal/gl-delivery-dlq
	// 4. GET /jurnal/gl-delivery-dlq/:id
	// 5. POST /jurnal/gl-delivery-dlq/:id/replay
	// 6. POST /jurnal/gl-delivery-dlq/:id/discard
	dlqGroup := authed.Group("/jurnal/gl-delivery-dlq")
	{
		dlqGroup.GET("", h.ListDLQ)
		dlqGroup.GET("/:id", h.GetDLQEntry)
		dlqGroup.POST("/:id/replay", idmp, h.ReplayDLQEntry)
		dlqGroup.POST("/:id/discard", idmp, h.DiscardDLQEntry)
	}

	// ─── Reconciliation ───────────────────────────────────────────────────────
	// 7. POST /jurnal/reconciliation/run
	// 8. GET /jurnal/reconciliation/:date
	// 9. GET /jurnal/reconciliation/history
	reconGroup := authed.Group("/jurnal/reconciliation")
	{
		reconGroup.POST("/run", idmp, h.RunReconciliation)
		reconGroup.GET("/history", h.ListReconciliationHistory)
		reconGroup.GET("/:date", h.GetReconciliationReport)
	}
}
