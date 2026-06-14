// Package penempatan — route registration for Penempatan Deposito endpoints (P5-M1).
package penempatan

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all Penempatan Deposito endpoints under the provided router group (/api/v1).
//
// Route layout (api/openapi/app-b-penempatan-deposito.yaml + docs/state-machines/p5-m1-penempatan.md):
//
//	POST   /trx/penempatan-deposito                          createPenempatan
//	GET    /trx/penempatan-deposito                          listPenempatan
//	GET    /trx/penempatan-deposito/:id                      getPenempatan
//	PATCH  /trx/penempatan-deposito/:id                      updatePenempatan   (DRAFT only)
//	DELETE /trx/penempatan-deposito/:id                      withdrawPenempatan (DRAFT only)
//	POST   /trx/penempatan-deposito/:id/submit               submitPenempatan
//	POST   /trx/penempatan-deposito/:id/review               reviewPenempatan
//	POST   /trx/penempatan-deposito/:id/approve              approvePenempatan  (step-up MFA)
//	POST   /trx/penempatan-deposito/:id/reject               rejectPenempatan
//	POST   /trx/penempatan-deposito/:id/terminate            terminatePenempatan
//	POST   /trx/penempatan-deposito/:id/terminate-review     terminateReviewPenempatan
//	POST   /trx/penempatan-deposito/:id/terminate-approve    terminateApprovePenempatan (step-up MFA)
//	POST   /trx/penempatan-deposito/:id/terminate-reject     terminateRejectPenempatan
//	GET    /trx/penempatan-deposito/:id/eir-preview          getEIRPreview
//	GET    /trx/penempatan-deposito/:id/audit-timeline       getAuditTimeline
//
// Auth: all routes require JWT via auth.Middleware (DEC-025).
// Idempotency-Key: required on all mutating POST/PATCH/DELETE (DEC-021).
// Step-up MFA: enforced inside Approve and TerminateApprove service methods (DEC-027).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	group := authed.Group("/trx/penempatan-deposito")
	{
		// --- Collection ---
		group.POST("", idmp, h.CreatePenempatan)
		group.GET("", h.ListPenempatan)

		// --- Resource ---
		group.GET("/:id", h.GetPenempatan)
		group.PATCH("/:id", idmp, h.UpdatePenempatan)
		group.DELETE("/:id", idmp, h.WithdrawPenempatan)

		// --- Primary workflow ---
		group.POST("/:id/submit", idmp, h.SubmitPenempatan)
		group.POST("/:id/review", idmp, h.ReviewPenempatan)
		group.POST("/:id/approve", idmp, h.ApprovePenempatan)
		group.POST("/:id/reject", idmp, h.RejectPenempatan)

		// --- Terminate workflow ---
		group.POST("/:id/terminate", idmp, h.TerminatePenempatan)
		group.POST("/:id/terminate-review", idmp, h.TerminateReviewPenempatan)
		group.POST("/:id/terminate-approve", idmp, h.TerminateApprovePenempatan)
		group.POST("/:id/terminate-reject", idmp, h.TerminateRejectPenempatan)

		// --- Read-only extras ---
		group.GET("/:id/eir-preview", h.GetEIRPreview)
		group.GET("/:id/audit-timeline", h.GetAuditTimeline)
	}
}
