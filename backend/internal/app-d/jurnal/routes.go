package jurnal

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all jurnal engine endpoints under the given router group.
// All endpoints require JWT authentication via auth.Middleware.
// Mutating endpoints have idempotency middleware.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	// ─── Mapping Header CRUD + Workflow ─────────────────────────────────────
	mappingHeaders := authed.Group("/jurnal/mapping-headers")
	{
		mappingHeaders.GET("", h.ListMappingHeaders)
		mappingHeaders.GET("/export", h.ExportMappingHeaders)
		mappingHeaders.POST("", idmp, h.CreateMappingHeader)
		mappingHeaders.GET("/:id", h.GetMappingHeader)
		mappingHeaders.PATCH("/:id", idmp, h.EditMappingHeader)

		// Workflow transitions
		mappingHeaders.POST("/:id/submit", idmp, h.SubmitMappingHeader)
		mappingHeaders.POST("/:id/review", idmp, h.ReviewMappingHeader)
		mappingHeaders.POST("/:id/approve", idmp, h.ApproveMappingHeader)
		mappingHeaders.POST("/:id/approve-2", idmp, h.ApproveMappingHeader2)
		mappingHeaders.POST("/:id/reject", idmp, h.RejectMappingHeader)
		mappingHeaders.POST("/:id/withdraw", idmp, h.WithdrawMappingHeader)
		mappingHeaders.POST("/:id/deactivate", idmp, h.DeactivateMappingHeader)
	}

	// ─── Resolver (preview, no DB write) ─────────────────────────────────────
	authed.POST("/jurnal/resolve", h.ResolveJurnal)

	// ─── Manual Posting ───────────────────────────────────────────────────────
	authed.POST("/jurnal/post", idmp, h.PostManualJurnal)

	// ─── Jurnal Read ──────────────────────────────────────────────────────────
	jurnal := authed.Group("/jurnal")
	{
		jurnal.GET("", h.ListJurnal)
		jurnal.GET("/export", h.ExportJurnalList)
		jurnal.GET("/:id", h.GetJurnal)
		jurnal.GET("/:id/export", h.ExportJurnal)

		// Manual posting workflow
		jurnal.POST("/:id/submit", idmp, h.SubmitManualJurnal)
		jurnal.POST("/:id/approve", idmp, h.ApproveManualJurnal)
		jurnal.POST("/:id/reject", idmp, h.RejectManualJurnal)
	}

	// ─── DLQ ─────────────────────────────────────────────────────────────────
	dlq := authed.Group("/jurnal/dlq")
	{
		dlq.GET("", h.ListDLQ)
		dlq.GET("/:id", h.GetDLQ)
		dlq.POST("/:id/replay", idmp, h.ReplayDLQ)
		dlq.POST("/:id/discard", idmp, h.DiscardDLQ)
	}
}
