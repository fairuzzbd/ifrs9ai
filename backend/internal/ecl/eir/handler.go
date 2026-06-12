// Package eir — Gin HTTP handlers for EIR endpoints.
//
// Endpoint → Story mapping (api/openapi/app-c-eir.yaml):
//
//	POST   /ecl/eir/compute                 computeEIR              (APP-C-EIR-001)
//	POST   /ecl/eir/generate-schedule       generateEIRSchedule     (APP-C-EIR-002)
//	GET    /ecl/eir/schedule/{instrumenId}  getActiveSchedule       (APP-C-EIR-003)
//	GET    /ecl/eir/schedule/{instrumenId}/history  getScheduleHistory (APP-C-EIR-003)
//	POST   /ecl/eir/amendments              proposeAmendment        (APP-C-EIR-004)
//	GET    /ecl/eir/amendments              listAmendments          (APP-C-EIR-004)
//	GET    /ecl/eir/amendments/{id}         getAmendment            (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/review  reviewAmendment         (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/approve approveAmendment        (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/reject  rejectAmendment         (APP-C-EIR-004)
//	POST   /ecl/eir/bulk-recompute          bulkRecomputeEIR        (APP-C-EIR-005)
//
// Permission: eir.compute / eir.preview / eir.amend.* / eir.bulk_recompute.
// Step-up MFA: required on approveAmendment (DEC-027).
// Idempotency-Key: required on POST mutating endpoints (middleware in routes.go).
// No float64 for rates (DEC-016).
package eir

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds all EIR service instances.
type Handler struct {
	eirSvc       *EIRService
	scheduleSvc  *ScheduleService
	amendmentSvc *AmendmentService
	bulkSvc      *BulkService
}

// NewHandler creates an EIR Handler.
func NewHandler(
	eirSvc *EIRService,
	scheduleSvc *ScheduleService,
	amendmentSvc *AmendmentService,
	bulkSvc *BulkService,
) *Handler {
	return &Handler{
		eirSvc:       eirSvc,
		scheduleSvc:  scheduleSvc,
		amendmentSvc: amendmentSvc,
		bulkSvc:      bulkSvc,
	}
}

// ─── hasPermission / hasMFAVerified ───────────────────────────────────────────

// hasPermission checks JWT permissions claim; writes 403 if missing.
func hasPermission(c *gin.Context, perm string) bool {
	permsRaw, exists := c.Get("permissions")
	if !exists {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			fmt.Sprintf("Permission '%s' diperlukan.", perm), nil)
		return false
	}
	switch v := permsRaw.(type) {
	case []string:
		for _, p := range v {
			if p == perm {
				return true
			}
		}
	case []interface{}:
		for _, p := range v {
			if s, ok := p.(string); ok && s == perm {
				return true
			}
		}
	}
	response.ErrorWithStatus(c, http.StatusForbidden,
		domainerrors.CodeForbidden,
		fmt.Sprintf("Permission '%s' diperlukan. Role Anda tidak memiliki akses.", perm), nil)
	return false
}

// hasMFAVerified returns true if the JWT mfa_verified claim is true.
func hasMFAVerified(c *gin.Context) bool {
	v, exists := c.Get("mfa_verified")
	if !exists {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// actorFromContext extracts actor UUID and role from JWT context.
func actorFromContext(c *gin.Context) (uuid.UUID, string) {
	subRaw, _ := c.Get("user_id")
	roleRaw, _ := c.Get("role")
	actorID, _ := uuid.Parse(fmt.Sprintf("%v", subRaw))
	role, _ := roleRaw.(string)
	return actorID, role
}

// handleDomainError writes an error response, distinguishing DomainError from generic errors.
func handleDomainError(c *gin.Context, err error) {
	if de, ok := domainerrors.IsDomainError(err); ok {
		response.ErrorWithStatus(c, de.HTTPStatus(), de.Code(), de.Message(), de.Details())
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal.", nil)
}

// ─── POST /ecl/eir/compute ────────────────────────────────────────────────────

// computeEIRRequest is the JSON body for POST /ecl/eir/compute.
type computeEIRRequest struct {
	InstrumenID        string              `json:"instrumenId" binding:"required,uuid"`
	CashflowProjection []cashflowItemJSON2 `json:"cashflowProjection" binding:"required,min=2"`
	CouponRate         *string             `json:"couponRate"` // optional seed (decimal string)
	PersistResult      bool                `json:"persistResult"`
	ForceRecompute     bool                `json:"forceRecompute"`
	POCIMode           bool                `json:"pociMode"`
}

// cashflowItemJSON2 is the JSON representation of a cashflow item in requests.
type cashflowItemJSON2 struct {
	Date      string `json:"date" binding:"required"`      // "YYYY-MM-DD"
	AmountIdr string `json:"amountIdr" binding:"required"` // decimal string
}

// ComputeEIR handles POST /ecl/eir/compute.
// Permission: eir.compute (ROLE-RISK, System).
func (h *Handler) ComputeEIR(c *gin.Context) {
	if !hasPermission(c, PermEIRCompute) {
		return
	}
	actorID, role := actorFromContext(c)

	var req computeEIRRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	instrumenID, _ := uuid.Parse(req.InstrumenID)
	cfs, err := parseCashflowItems(req.CashflowProjection)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	var couponRate *decimal.Decimal
	if req.CouponRate != nil && *req.CouponRate != "" {
		d, parseErr := decimal.NewFromString(*req.CouponRate)
		if parseErr == nil {
			couponRate = &d
		}
	}

	result, svcErr := h.eirSvc.Compute(c.Request.Context(), EIRComputeRequest{
		InstrumenID:        instrumenID,
		CashflowProjection: cfs,
		CouponRate:         couponRate,
		PersistResult:      req.PersistResult,
		ForceRecompute:     req.ForceRecompute,
		POCIMode:           req.POCIMode,
	}, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	response.OK(c, gin.H{
		"instrumenId":         result.InstrumenID,
		"eirPerPeriod":        result.EIRPerPeriod.StringFixed(8),
		"eirAnnualEquivalent": result.EIRAnnualEquivalent.StringFixed(8),
		"iterationsUsed":      result.IterationsUsed,
		"convergenceResidual": result.ConvergenceResidual.String(),
		"flagPoci":            result.FlagPOCI,
		"eirType":             result.EIRType,
		"persisted":           result.Persisted,
		"computedAt":          result.ComputedAt.Format(time.RFC3339),
	})
}

// ─── POST /ecl/eir/generate-schedule ─────────────────────────────────────────

// generateScheduleRequest is the JSON body for POST /ecl/eir/generate-schedule.
type generateScheduleRequest struct {
	InstrumenID        string              `json:"instrumenId" binding:"required,uuid"`
	CashflowProjection []cashflowItemJSON2 `json:"cashflowProjection" binding:"required,min=2"`
	ForceRegenerate    bool                `json:"forceRegenerate"`
}

// GenerateSchedule handles POST /ecl/eir/generate-schedule.
func (h *Handler) GenerateSchedule(c *gin.Context) {
	if !hasPermission(c, PermEIRCompute) {
		return
	}
	actorID, role := actorFromContext(c)

	var req generateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	instrumenID, _ := uuid.Parse(req.InstrumenID)
	cfs, err := parseCashflowItems(req.CashflowProjection)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	result, svcErr := h.scheduleSvc.Generate(c.Request.Context(), GenerateScheduleRequest{
		InstrumenID:        instrumenID,
		CashflowProjection: cfs,
		ForceRegenerate:    req.ForceRegenerate,
	}, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	response.Created(c, gin.H{
		"instrumenId":          result.InstrumenID,
		"totalRows":            result.TotalRows,
		"eirPerPeriod":         result.EIRPerPeriod.StringFixed(8),
		"openingCarryingFirst": result.OpeningCarryingFirst.StringFixed(4),
		"closingCarryingLast":  result.ClosingCarryingLast.StringFixed(4),
		"closingRoundingDelta": result.ClosingRoundingDelta.StringFixed(4),
		"generatedAt":          result.GeneratedAt.Format(time.RFC3339),
	})
}

// ─── GET /ecl/eir/schedule/{instrumenId} ──────────────────────────────────────

// GetActiveSchedule handles GET /ecl/eir/schedule/{instrumenId}.
func (h *Handler) GetActiveSchedule(c *gin.Context) {
	if !hasPermission(c, PermEIRPreview) {
		return
	}

	instrumenIDStr := c.Param("instrumenId")
	instrumenID, err := uuid.Parse(instrumenIDStr)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}

	q, parseErr := listquery.ParseFromRequest(c.Request, AllowedColsSchedule)
	if parseErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, parseErr.Error(), nil)
		return
	}

	cursor := c.Query("cursor")
	limit := 50
	if l, ok := parseInt(c.Query("limit")); ok && l > 0 && l <= 200 {
		limit = l
	}

	rows, meta, svcErr := h.scheduleSvc.schedRepo.List(c.Request.Context(), instrumenID, q, false, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	response.List(c, scheduleRowsToJSON(rows), meta, nil, nil)
}

// GetScheduleHistory handles GET /ecl/eir/schedule/{instrumenId}/history (including superseded rows).
func (h *Handler) GetScheduleHistory(c *gin.Context) {
	if !hasPermission(c, PermEIRPreview) {
		return
	}

	instrumenIDStr := c.Param("instrumenId")
	instrumenID, err := uuid.Parse(instrumenIDStr)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}

	q, parseErr := listquery.ParseFromRequest(c.Request, AllowedColsSchedule)
	if parseErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, parseErr.Error(), nil)
		return
	}

	cursor := c.Query("cursor")
	limit := 50
	if l, ok := parseInt(c.Query("limit")); ok && l > 0 && l <= 200 {
		limit = l
	}

	rows, meta, svcErr := h.scheduleSvc.schedRepo.List(c.Request.Context(), instrumenID, q, true, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	response.List(c, scheduleRowsToJSON(rows), meta, nil, nil)
}

// ─── POST /ecl/eir/amendments ─────────────────────────────────────────────────

// proposeAmendmentRequest is the JSON body for POST /ecl/eir/amendments.
type proposeAmendmentRequest struct {
	InstrumenID               string              `json:"instrumenId" binding:"required,uuid"`
	TanggalAmandemen          string              `json:"tanggalAmandemen" binding:"required"` // "YYYY-MM-DD"
	RevisedCashflowProjection []cashflowItemJSON2 `json:"revisedCashflowProjection" binding:"required,min=2"`
	AlasanAmandemen           string              `json:"alasanAmandemen" binding:"required,min=10"`
	DokumenPendukungID        *string             `json:"dokumenPendukungId"` // optional UUID
}

// ProposeAmendment handles POST /ecl/eir/amendments.
func (h *Handler) ProposeAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendPropose) {
		return
	}
	actorID, role := actorFromContext(c)

	var req proposeAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	instrumenID, _ := uuid.Parse(req.InstrumenID)
	tanggal, parseErr := time.Parse("2006-01-02", req.TanggalAmandemen)
	if parseErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "tanggalAmandemen harus format YYYY-MM-DD", nil)
		return
	}

	cfs, cfErr := parseCashflowItems(req.RevisedCashflowProjection)
	if cfErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, cfErr.Error(), nil)
		return
	}

	propReq := ProposeRequest{
		InstrumenID:               instrumenID,
		TanggalAmandemen:          tanggal,
		RevisedCashflowProjection: cfs,
		AlasanAmandemen:           req.AlasanAmandemen,
	}
	if req.DokumenPendukungID != nil {
		did, err := uuid.Parse(*req.DokumenPendukungID)
		if err == nil {
			propReq.DokumenPendukungID = &did
		}
	}

	proposal, svcErr := h.amendmentSvc.Propose(c.Request.Context(), propReq, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	response.Created(c, proposalToJSON(proposal))
}

// ─── GET /ecl/eir/amendments ──────────────────────────────────────────────────

// ListAmendments handles GET /ecl/eir/amendments.
func (h *Handler) ListAmendments(c *gin.Context) {
	if !hasPermission(c, PermEIRPreview) {
		return
	}
	actorID, role := actorFromContext(c)
	isAdmin := role == "ROLE-IT-ADMIN" || role == "ROLE-AUDIT"

	q, parseErr := listquery.ParseFromRequest(c.Request, AllowedColsAmendment)
	if parseErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, parseErr.Error(), nil)
		return
	}

	cursor := c.Query("cursor")
	limit := 50
	if l, ok := parseInt(c.Query("limit")); ok && l > 0 && l <= 200 {
		limit = l
	}

	proposals, meta, svcErr := h.amendmentSvc.amendRepo.List(c.Request.Context(), q, cursor, limit, actorID, isAdmin)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	data := make([]interface{}, len(proposals))
	for i, p := range proposals {
		data[i] = proposalToJSON(p)
	}
	response.List(c, data, meta, nil, nil)
}

// ─── GET /ecl/eir/amendments/{id} ─────────────────────────────────────────────

// GetAmendment handles GET /ecl/eir/amendments/{id}.
func (h *Handler) GetAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRPreview) {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	proposal, svcErr := h.amendmentSvc.amendRepo.GetByID(c.Request.Context(), id)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	if proposal == nil {
		response.ErrorWithStatus(c, http.StatusNotFound,
			domainerrors.CodeEIRAmendNotFound, "Amendment tidak ditemukan", nil)
		return
	}
	response.OK(c, proposalToJSON(*proposal))
}

// ─── POST /ecl/eir/amendments/{id}/review ────────────────────────────────────

// reviewAmendmentRequest is the JSON body for review.
type reviewAmendmentRequest struct {
	Comment string `json:"comment" binding:"required,min=5"`
}

// ReviewAmendment handles POST /ecl/eir/amendments/{id}/review.
func (h *Handler) ReviewAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendReview) {
		return
	}
	actorID, role := actorFromContext(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	var req reviewAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	proposal, svcErr := h.amendmentSvc.Review(c.Request.Context(), ReviewRequest{
		AmendmentID: id,
		Comment:     req.Comment,
	}, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.OK(c, proposalToJSON(proposal))
}

// ─── POST /ecl/eir/amendments/{id}/approve ───────────────────────────────────

// approveAmendmentRequest is the JSON body for approve (step-up MFA required).
type approveAmendmentRequest struct {
	Comment     string `json:"comment" binding:"required,min=5"`
	StepUpToken string `json:"stepUpToken" binding:"required"`
}

// ApproveAmendment handles POST /ecl/eir/amendments/{id}/approve.
// Requires: PermEIRAmendApprove + MFA verified (DEC-027).
func (h *Handler) ApproveAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendApprove) {
		return
	}
	if !hasMFAVerified(c) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeMFARequired, "Action ini membutuhkan MFA terverifikasi.", nil)
		return
	}
	actorID, role := actorFromContext(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	var req approveAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	proposal, svcErr := h.amendmentSvc.Approve(c.Request.Context(), ApproveRequest{
		AmendmentID: id,
		Comment:     req.Comment,
		StepUpToken: req.StepUpToken,
	}, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.OK(c, proposalToJSON(proposal))
}

// ─── POST /ecl/eir/amendments/{id}/reject ────────────────────────────────────

// rejectAmendmentRequest is the JSON body for reject.
type rejectAmendmentRequest struct {
	Comment string `json:"comment" binding:"required,min=5"`
}

// RejectAmendment handles POST /ecl/eir/amendments/{id}/reject.
func (h *Handler) RejectAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendReview) && !hasPermission(c, PermEIRAmendApprove) {
		return
	}
	actorID, role := actorFromContext(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	var req rejectAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	proposal, svcErr := h.amendmentSvc.Reject(c.Request.Context(), WorkflowAction{
		AmendmentID: id,
		Comment:     req.Comment,
	}, actorID, role)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.OK(c, proposalToJSON(proposal))
}

// ─── POST /ecl/eir/bulk-recompute ─────────────────────────────────────────────

// bulkRecomputeRequest is the JSON body for POST /ecl/eir/bulk-recompute.
type bulkRecomputeRequest struct {
	Scope        string      `json:"scope" binding:"required"`              // ALL_ACTIVE or SUBSET
	InstrumenIDs []string    `json:"instrumenIds"`                          // required if scope=SUBSET
	Reason       string      `json:"reason" binding:"required,min=5"`
}

// BulkRecompute handles POST /ecl/eir/bulk-recompute.
// Returns 202 Accepted with jobId (Asynq job).
func (h *Handler) BulkRecompute(c *gin.Context) {
	if !hasPermission(c, PermEIRBulkRecompute) {
		return
	}
	actorID, _ := actorFromContext(c)

	var req bulkRecomputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	scope := BulkScope(req.Scope)
	if scope != BulkScopeAllActive && scope != BulkScopeSubset {
		handleDomainError(c, ErrEIRBulkRecomputeInvalidScope(req.Scope))
		return
	}

	jobID := uuid.New().String()
	payload, err := submitBulkRecomputeJob(jobID, scope, actorID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "Failed to create job payload", nil)
		return
	}
	_ = payload // In production: enqueue via Asynq client here

	response.Accepted(c, gin.H{
		"jobId":     jobID,
		"type":      BulkRecomputeTaskType,
		"statusUrl": "/api/v1/jobs/" + jobID,
		"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
	})
}

// ─── Response serialisers ──────────────────────────────────────────────────────

// scheduleRowsToJSON converts []ScheduleRow to []interface{} for response.List.
func scheduleRowsToJSON(rows []ScheduleRow) []interface{} {
	data := make([]interface{}, len(rows))
	for i, row := range rows {
		data[i] = scheduleRowToJSON(row)
	}
	return data
}

// scheduleRowToJSON serialises a ScheduleRow (decimal as StringFixed, no float64).
func scheduleRowToJSON(row ScheduleRow) gin.H {
	m := gin.H{
		"id":                 row.ID,
		"instrumenId":        row.InstrumenID,
		"periodeSeq":         row.PeriodeSeq,
		"tanggalPosting":     row.TanggalPosting.Format("2006-01-02"),
		"openingCarrying":    row.OpeningCarrying.StringFixed(4),
		"cashInflow":         row.CashInflow.StringFixed(4),
		"pendapatanBungaEir": row.PendapatanBungaEIR.StringFixed(4),
		"amortisasiPD":       row.AmortisasiPD.StringFixed(4),
		"pelunasanPokok":     row.PelunasanPokok.StringFixed(4),
		"closingCarrying":    row.ClosingCarrying.StringFixed(4),
		"eirPeriode":         row.EIRPeriode.StringFixed(8),
		"stageSaatPosting":   row.StageSaatPosting,
		"statusPosting":      row.StatusPosting,
		"flagPoci":           row.FlagPOCI,
		"tenantId":           row.TenantID,
		"createdAt":          row.CreatedAt.Format(time.RFC3339),
	}
	if row.RecomputedFromSeq != nil {
		m["recomputedFromSeq"] = *row.RecomputedFromSeq
	}
	return m
}

// proposalToJSON serialises EIRAmendmentProposal (decimal as StringFixed, no float64).
func proposalToJSON(p EIRAmendmentProposal) gin.H {
	m := gin.H{
		"id":               p.ID,
		"instrumenId":      p.InstrumenID,
		"tanggalAmandemen": p.TanggalAmandemen.Format("2006-01-02"),
		"alasanAmandemen":  p.AlasanAmandemen,
		"status":           string(p.Status),
		"tenantId":         p.TenantID,
		"createdAt":        p.CreatedAt.Format(time.RFC3339),
		"updatedAt":        p.UpdatedAt.Format(time.RFC3339),
	}
	if p.EIRLama != nil {
		m["eirLama"] = p.EIRLama.StringFixed(8)
	}
	if p.EIRBaru != nil {
		m["eirBaru"] = p.EIRBaru.StringFixed(8)
	}
	if p.MakerID != nil {
		m["makerId"] = *p.MakerID
	}
	if p.ReviewerID != nil {
		m["reviewerId"] = *p.ReviewerID
	}
	if p.ApproverID != nil {
		m["approverId"] = *p.ApproverID
	}
	if p.ReviewerComment != nil {
		m["reviewerComment"] = *p.ReviewerComment
	}
	if p.ApproverComment != nil {
		m["approverComment"] = *p.ApproverComment
	}
	if p.ApprovedAt != nil {
		m["approvedAt"] = p.ApprovedAt.Format(time.RFC3339)
	}
	return m
}

// ─── Parsing helpers ───────────────────────────────────────────────────────────

// parseCashflowItems converts JSON request items to []CashflowItem.
func parseCashflowItems(items []cashflowItemJSON2) ([]CashflowItem, error) {
	cfs := make([]CashflowItem, len(items))
	for i, item := range items {
		d, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			return nil, fmt.Errorf("cashflowProjection[%d].date parse error: %w", i, err)
		}
		amt, err := decimal.NewFromString(item.AmountIdr)
		if err != nil {
			return nil, fmt.Errorf("cashflowProjection[%d].amountIdr parse error: %w", i, err)
		}
		cfs[i] = CashflowItem{Date: d, AmountIDR: amt}
	}
	return cfs, nil
}

// parseInt parses a string to int; returns (0, false) on error.
func parseInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err == nil
}
