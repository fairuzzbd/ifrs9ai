// Package penempatan — HTTP handlers for Penempatan Deposito (P5-M1).
// Handlers are thin: extract claims → permission check → bind → call service → respond.
// No SQL, no business logic here.
package penempatan

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds the service dependency for HTTP handlers.
type Handler struct {
	svc ServiceIface
}

// NewHandler constructs a Handler. Panics if svc is nil.
func NewHandler(svc ServiceIface) *Handler {
	if svc == nil {
		panic("penempatan.NewHandler: svc must not be nil")
	}
	return &Handler{svc: svc}
}

// claimsFromCtx extracts JWT claims from gin context.
func claimsFromCtx(c *gin.Context) *auth.Claims {
	v, ok := c.Get("claims")
	if !ok {
		return nil
	}
	cl, ok := v.(*auth.Claims)
	if !ok {
		return nil
	}
	return cl
}

// toSortApplied converts listquery AppliedSort to response.SortApplied slice.
func toSortApplied(sorts []map[string]string) []response.SortApplied {
	out := make([]response.SortApplied, len(sorts))
	for i, s := range sorts {
		out[i] = response.SortApplied{Col: s["col"], Dir: s["dir"]}
	}
	return out
}

// ─── Create ──────────────────────────────────────────────────────────────────

// CreatePenempatan handles POST /trx/penempatan-deposito.
func (h *Handler) CreatePenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiCreate) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.create", nil)
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, err := h.svc.Create(c.Request.Context(), req, claims)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, result)
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListPenempatan handles GET /trx/penempatan-deposito (DataTable: sort+filter+cursor).
func (h *Handler) ListPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiRead) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.read", nil)
		return
	}

	q, err := listquery.ParseFromRequest(c.Request, AllowedSortCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	includeDeleted := c.Query("include_deleted") == "true" && claims.HasPermission(PermAuditLogRead)

	result, svcErr := h.svc.List(c.Request.Context(), q, includeDeleted, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	totalEst := result.TotalEst
	pagination := &response.PaginationMeta{
		NextCursor:    result.NextCursor,
		HasMore:       result.HasMore,
		TotalEstimate: &totalEst,
		Limit:         50,
	}
	response.List(c, result.Items, pagination, toSortApplied(q.AppliedSort()), q.AppliedFilter())
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetPenempatan handles GET /trx/penempatan-deposito/:id.
func (h *Handler) GetPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiRead) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.read", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	result, svcErr := h.svc.GetByID(c.Request.Context(), id, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── Update ───────────────────────────────────────────────────────────────────

// UpdatePenempatan handles PATCH /trx/penempatan-deposito/:id (DRAFT only).
func (h *Handler) UpdatePenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiUpdate) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.update", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req UpdateRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.Update(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── Withdraw ────────────────────────────────────────────────────────────────

// WithdrawPenempatan handles DELETE /trx/penempatan-deposito/:id (soft-delete DRAFT).
func (h *Handler) WithdrawPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiDelete) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.delete", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	if svcErr := h.svc.Withdraw(c.Request.Context(), id, claims); svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.NoContent(c)
}

// ─── Submit ──────────────────────────────────────────────────────────────────

// SubmitPenempatan handles POST /trx/penempatan-deposito/:id/submit.
func (h *Handler) SubmitPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiSubmit) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.submit", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req WorkflowActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.Submit(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── Review ──────────────────────────────────────────────────────────────────

// ReviewPenempatan handles POST /trx/penempatan-deposito/:id/review.
func (h *Handler) ReviewPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiReview) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.review", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req WorkflowActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.Review(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── Approve ──────────────────────────────────────────────────────────────────

// ApprovePenempatan handles POST /trx/penempatan-deposito/:id/approve (requires MFA step-up).
func (h *Handler) ApprovePenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiApprove) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.approve", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req WorkflowActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.Approve(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, gin.H{
		"penempatan":    result.Penempatan,
		"stagingAction": result.StagingAction,
		"eirJobId":      result.EIRComputeJobID,
	})
}

// ─── Reject ──────────────────────────────────────────────────────────────────

// RejectPenempatan handles POST /trx/penempatan-deposito/:id/reject.
func (h *Handler) RejectPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiReject) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.reject", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req RejectActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.Reject(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── TerminateRequest ────────────────────────────────────────────────────────

// TerminatePenempatan handles POST /trx/penempatan-deposito/:id/terminate.
func (h *Handler) TerminatePenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiTerminate) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.terminate", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req TerminateRequestBody
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.TerminateRequest(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── TerminateReview ─────────────────────────────────────────────────────────

// TerminateReviewPenempatan handles POST /trx/penempatan-deposito/:id/terminate-review.
func (h *Handler) TerminateReviewPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiReview) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.review", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req WorkflowActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.TerminateReview(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── TerminateApprove ────────────────────────────────────────────────────────

// TerminateApprovePenempatan handles POST /trx/penempatan-deposito/:id/terminate-approve (requires MFA step-up).
func (h *Handler) TerminateApprovePenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiApprove) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.approve", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req WorkflowActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.TerminateApprove(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── TerminateReject ──────────────────────────────────────────────────────────

// TerminateRejectPenempatan handles POST /trx/penempatan-deposito/:id/terminate-reject.
func (h *Handler) TerminateRejectPenempatan(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiReject) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission transaksi.reject", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	var req RejectActionRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.svc.TerminateReject(c.Request.Context(), id, req, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── EIRPreview ───────────────────────────────────────────────────────────────

// GetEIRPreview handles GET /trx/penempatan-deposito/:id/eir-preview.
func (h *Handler) GetEIRPreview(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiRead) && !claims.HasPermission(PermEIRPreview) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission eir.preview atau transaksi.read", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	result, svcErr := h.svc.EIRPreview(c.Request.Context(), id, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── AuditTimeline ────────────────────────────────────────────────────────────

// GetAuditTimeline handles GET /trx/penempatan-deposito/:id/audit-timeline.
func (h *Handler) GetAuditTimeline(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		response.ErrorWithStatus(c, http.StatusUnauthorized, domainerrors.CodeUnauthorized, "JWT tidak valid", nil)
		return
	}
	if !claims.HasPermission(PermTransaksiRead) && !claims.HasPermission(PermAuditLogRead) {
		response.ErrorWithStatus(c, http.StatusForbidden, domainerrors.CodeForbidden, "Tidak memiliki permission untuk melihat audit timeline", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "ID tidak valid", nil)
		return
	}

	result, svcErr := h.svc.AuditTimeline(c.Request.Context(), id, claims)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}
