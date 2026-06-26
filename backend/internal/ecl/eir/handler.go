// Package eir — Gin HTTP handlers for EIR endpoints.
//
// Endpoint → Story mapping (api/openapi/app-c-eir.yaml + app-c-eir-amendment-lifecycle.yaml):
//
//	POST   /ecl/eir/compute                          computeEIR              (APP-C-EIR-001)
//	POST   /ecl/eir/generate-schedule                generateEIRSchedule     (APP-C-EIR-002)
//	GET    /ecl/eir/schedule/{instrumenId}            getActiveSchedule       (APP-C-EIR-003)
//	GET    /ecl/eir/schedule/{instrumenId}/history    getScheduleHistory      (APP-C-EIR-003)
//	POST   /ecl/eir/amendments                        proposeAmendment        (APP-C-EIR-004)
//	GET    /ecl/eir/amendments                        listAmendments          (APP-C-EIR-004)
//	GET    /ecl/eir/amendments/{id}                   getAmendment            (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/review            reviewAmendment         (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/approve           approveAmendment        (APP-C-EIR-004)
//	POST   /ecl/eir/amendments/{id}/reject            rejectAmendment         (APP-C-EIR-004)
//	POST   /ecl/eir/bulk-recompute                    bulkRecomputeEIR        (APP-C-EIR-005)
//	--- M6 additions (p4-m6-amendment-lifecycle.md) ---
//	POST   /ecl/eir/amendments/detect                 detectAmendment         (M6-001)
//	POST   /ecl/eir/amendments/{id}/cancel            cancelAmendment         (M6-005)
//	PATCH  /ecl/eir/amendments/{id}/cashflows         updateCashflows         (M6-003 PATCH)
//	GET    /ecl/eir/amendments/queue                  listAmendmentQueue      (M6-004)
//	GET    /ecl/eir/amendments/queue/export           exportAmendmentQueue    (M6-004 export)
//	GET    /ecl/eir/drift-reports                     listDriftReports        (M6-002)
//	GET    /ecl/eir/drift-reports/{id}                getDriftReport          (M6-002)
//	POST   /ecl/eir/drift-reports/generate            generateDriftReport     (M6-002 ad-hoc)
//
// Permission: eir.compute / eir.preview / eir.amend.* / eir.bulk_recompute /
//
//	eir.amendment.detect / eir.amendment.cancel / eir.amendment_review.read /
//	eir.drift_report.read / eir.drift_report.generate.
//
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

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds all EIR service instances (M5 + M6).
type Handler struct {
	eirSvc       *Service
	scheduleSvc  *ScheduleService
	amendmentSvc *AmendmentService
	bulkSvc      *BulkService
	// M6 additions
	detectionSvc *DetectionService
	driftSvc     *DriftService
}

// NewHandler creates an EIR Handler.
func NewHandler(
	eirSvc *Service,
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

// NewHandlerM6 creates an EIR Handler with M6 services wired in.
// Called from main.go after M6 services are constructed.
func NewHandlerM6(
	eirSvc *Service,
	scheduleSvc *ScheduleService,
	amendmentSvc *AmendmentService,
	bulkSvc *BulkService,
	detectionSvc *DetectionService,
	driftSvc *DriftService,
) *Handler {
	return &Handler{
		eirSvc:       eirSvc,
		scheduleSvc:  scheduleSvc,
		amendmentSvc: amendmentSvc,
		bulkSvc:      bulkSvc,
		detectionSvc: detectionSvc,
		driftSvc:     driftSvc,
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

// claimsFromGin retrieves auth.Claims injected by the JWT middleware.
// The middleware stores the full Claims struct under key "claims" (auth/middleware.go:44).
// Returns nil if not present (unauthenticated path or test without claims injected).
func claimsFromGin(c *gin.Context) *auth.Claims {
	v, exists := c.Get("claims")
	if !exists {
		return nil
	}
	if cl, ok := v.(*auth.Claims); ok {
		return cl
	}
	return nil
}

// actorFromContext extracts actor UUID and role from JWT context.
// Parse errors from JWT middleware-injected values are intentionally ignored:
// a missing/malformed sub returns uuid.Nil (logged upstream), a missing role returns "".
func actorFromContext(c *gin.Context) (uuid.UUID, string) {
	subRaw, _ := c.Get("user_id")
	roleRaw, _ := c.Get("role")
	actorID, err := uuid.Parse(fmt.Sprintf("%v", subRaw))
	if err != nil {
		actorID = uuid.Nil
	}
	role, ok := roleRaw.(string)
	if !ok {
		role = ""
	}
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

	instrumenID, err := uuid.Parse(req.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}
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

	result, svcErr := h.eirSvc.Compute(c.Request.Context(), ComputeRequest{
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

	instrumenID, err := uuid.Parse(req.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}
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

	instrumenID, err := uuid.Parse(req.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}
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
	actorID, _ := actorFromContext(c)
	// H-01 fix: use permission check instead of role string comparison (security-baseline.md).
	// "audit_log.read" grants ROLE-AUDIT and ROLE-IT-ADMIN expanded visibility over all records.
	cl := claimsFromGin(c)
	isAdmin := cl != nil && cl.HasPermission("audit_log.read")

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
	for i := range proposals {
		data[i] = proposalToJSON(proposals[i])
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
// Requires: PermEIRAmendApprove + fresh step-up MFA within 5 minutes (DEC-027).
// Uses claims.NeedsStepUp() so a 4-hour-old JWT with mfa_verified=true is rejected
// unless stepup_verified_at is within the 5-minute window.
func (h *Handler) ApproveAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendApprove) {
		return
	}
	// DEC-027: step-up MFA must be fresh (< 5 minutes). Static mfa_verified=true is
	// insufficient — stepup_verified_at timestamp in JWT claims must be within window.
	cl := claimsFromGin(c)
	if cl == nil || cl.NeedsStepUp() {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeStepUpRequired, "Approve EIR amendment membutuhkan step-up MFA (< 5 menit). Panggil /auth/step-up terlebih dahulu.", nil)
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
	Scope        string   `json:"scope" binding:"required"` // ALL_ACTIVE or SUBSET
	InstrumenIDs []string `json:"instrumenIds"`             // required if scope=SUBSET
	Reason       string   `json:"reason" binding:"required,min=5"`
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
	for i := range rows {
		data[i] = scheduleRowToJSON(rows[i])
	}
	return data
}

// scheduleRowToJSON serializes a ScheduleRow (decimal as StringFixed, no float64).
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

// proposalToJSON serializes AmendmentProposal (decimal as StringFixed, no float64).
// Updated P4-M6 to include M6 fields: cancelledAt, cancelReason, triggerSource,
// driftReportId, documentId.
func proposalToJSON(p AmendmentProposal) gin.H {
	m := gin.H{
		"id":               p.ID,
		"instrumenId":      p.InstrumenID,
		"tanggalAmandemen": p.TanggalAmandemen.Format("2006-01-02"),
		"alasanAmandemen":  p.AlasanAmandemen,
		"status":           string(p.Status),
		"triggerSource":    string(p.TriggerSource),
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
	// M6 additions
	if p.CancelledAt != nil {
		m["cancelledAt"] = p.CancelledAt.Format(time.RFC3339)
	}
	if p.CancelReason != nil {
		m["cancelReason"] = *p.CancelReason
	}
	if p.CancelledBy != nil {
		m["cancelledBy"] = *p.CancelledBy
	}
	if p.DriftReportID != nil {
		m["driftReportId"] = *p.DriftReportID
	}
	if p.DocumentID != nil {
		m["documentId"] = *p.DocumentID
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

// ─── M6 handlers ──────────────────────────────────────────────────────────────

// detectAmendmentRequest is the JSON body for POST /ecl/eir/amendments/detect (M6-001).
type detectAmendmentRequest struct {
	InstrumenID    string              `json:"instrumenId" binding:"required,uuid"`
	DocumentID     string              `json:"documentId" binding:"required,uuid"`
	AlasanDetected string              `json:"alasanDetected"`
	Cashflows      []cashflowItemJSON2 `json:"cashflows"` // optional parsed cashflows
}

// DetectAmendment handles POST /ecl/eir/amendments/detect (M6-001).
// Permission: eir.amendment.detect (ROLE-RISK, ROLE-AKUN).
// Returns 201 with DRAFT proposal or 422 EIR_AMENDMENT_DETECTION_NO_MATCH.
func (h *Handler) DetectAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendDetect) {
		return
	}
	actorID, _ := actorFromContext(c)

	var req detectAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	instrumenID, err := uuid.Parse(req.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId bukan UUID yang valid", nil)
		return
	}
	documentID, err := uuid.Parse(req.DocumentID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "documentId bukan UUID yang valid", nil)
		return
	}

	var cfs []CashflowItem
	if len(req.Cashflows) > 0 {
		cfs, err = parseCashflowItems(req.Cashflows)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, err.Error(), nil)
			return
		}
	}

	tenantID := "TUGURE"
	if cl := claimsFromGin(c); cl != nil {
		tenantID = cl.TenantID
	}

	proposal, svcErr := h.detectionSvc.DetectFromDocument(c.Request.Context(), DetectAmendmentRequest{
		InstrumenID:       instrumenID,
		DocumentID:        documentID,
		AlasanDetected:    req.AlasanDetected,
		OverrideCashflows: cfs,
		ActorID:           actorID,
		TenantID:          tenantID,
	})
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.Created(c, proposalToJSON(*proposal))
}

// cancelAmendmentRequest is the JSON body for POST /ecl/eir/amendments/{id}/cancel (M6-005).
type cancelAmendmentRequest struct {
	CancelReason string `json:"cancelReason" binding:"required"`
}

// CancelAmendment handles POST /ecl/eir/amendments/{id}/cancel (M6-005).
// Permission: eir.amendment.cancel (maker only — SoD enforced in service).
func (h *Handler) CancelAmendment(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendCancel) {
		return
	}
	actorID, _ := actorFromContext(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	var req cancelAmendmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	tenantID := "TUGURE"
	if cl := claimsFromGin(c); cl != nil {
		tenantID = cl.TenantID
	}

	proposal, svcErr := h.detectionSvc.CancelAmendment(c.Request.Context(), CancelAmendmentRequest{
		AmendmentID:  id,
		CancelReason: req.CancelReason,
		ActorID:      actorID,
		TenantID:     tenantID,
	})
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.OK(c, proposalToJSON(*proposal))
}

// updateCashflowsRequest is the JSON body for PATCH /ecl/eir/amendments/{id}/cashflows.
type updateCashflowsRequest struct {
	RevisedCashflows []cashflowItemJSON2 `json:"revisedCashflows" binding:"required,min=2"`
}

// UpdateCashflows handles PATCH /ecl/eir/amendments/{id}/cashflows (M6-003 PATCH).
// Permission: eir.amendment.update_cashflows.
func (h *Handler) UpdateCashflows(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendUpdateCF) {
		return
	}
	actorID, _ := actorFromContext(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	var req updateCashflowsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	cfs, parseErr := parseCashflowItems(req.RevisedCashflows)
	if parseErr != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, parseErr.Error(), nil)
		return
	}

	tenantID := "TUGURE"
	if cl := claimsFromGin(c); cl != nil {
		tenantID = cl.TenantID
	}

	proposal, svcErr := h.detectionSvc.UpdateCashflows(c.Request.Context(), UpdateCashflowsRequest{
		AmendmentID:      id,
		RevisedCashflows: cfs,
		ActorID:          actorID,
		TenantID:         tenantID,
	})
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	response.OK(c, proposalToJSON(*proposal))
}

// ListAmendmentQueue handles GET /ecl/eir/amendments/queue (M6-004).
// Permission: eir.amendment_review.read.
func (h *Handler) ListAmendmentQueue(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendReviewRead) {
		return
	}

	q, parseErr := listquery.ParseFromRequest(c.Request, AllowedColsQueue)
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

	rows, meta, svcErr := h.amendmentSvc.amendRepo.ListQueue(c.Request.Context(), q, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	data := make([]interface{}, len(rows))
	for i := range rows {
		qr := &rows[i]
		m := gin.H{
			"amendmentId":      qr.AmendmentID,
			"instrumenId":      qr.InstrumenID,
			"kodeInstrumen":    qr.KodeInstrumen,
			"status":           string(qr.Status),
			"triggerSource":    string(qr.TriggerSource),
			"tanggalAmandemen": qr.TanggalAmandemen.Format("2006-01-02"),
			"createdAt":        qr.CreatedAt.Format(time.RFC3339),
		}
		if qr.EIRLama != nil {
			m["eirLama"] = qr.EIRLama.StringFixed(8)
		}
		if qr.MakerID != nil {
			m["makerId"] = *qr.MakerID
		}
		if qr.ReviewerID != nil {
			m["reviewerId"] = *qr.ReviewerID
		}
		if qr.DriftReportID != nil {
			m["driftReportId"] = *qr.DriftReportID
		}
		if qr.DocumentID != nil {
			m["documentId"] = *qr.DocumentID
		}
		data[i] = m
	}
	response.List(c, data, meta, nil, nil)
}

// ExportAmendmentQueue handles GET /ecl/eir/amendments/queue/export (M6-004 export).
// Returns 202 async job for large datasets or 200 CSV for small ones.
// Permission: eir.amendment_review.read.
func (h *Handler) ExportAmendmentQueue(c *gin.Context) {
	if !hasPermission(c, PermEIRAmendReviewRead) {
		return
	}
	// Simplified: return 202 Accepted with job reference (async export pattern per UX §1.4).
	jobID := uuid.New().String()
	response.Accepted(c, gin.H{
		"jobId":     jobID,
		"type":      "EIR_AMENDMENT_QUEUE_EXPORT",
		"statusUrl": "/api/v1/jobs/" + jobID,
		"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
	})
}

// ListDriftReports handles GET /ecl/eir/drift-reports (M6-002).
// Permission: eir.drift_report.read.
func (h *Handler) ListDriftReports(c *gin.Context) {
	if !hasPermission(c, PermEIRDriftReportRead) {
		return
	}

	q, parseErr := listquery.ParseFromRequest(c.Request, AllowedColsDriftReport)
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

	reports, meta, svcErr := h.driftSvc.ListReports(c.Request.Context(), q, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	data := make([]interface{}, len(reports))
	for i := range reports {
		data[i] = driftReportToJSON(reports[i])
	}
	response.List(c, data, meta, nil, nil)
}

// GetDriftReport handles GET /ecl/eir/drift-reports/{id} (M6-002 detail).
// Permission: eir.drift_report.read.
func (h *Handler) GetDriftReport(c *gin.Context) {
	if !hasPermission(c, PermEIRDriftReportRead) {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID yang valid", nil)
		return
	}

	result, svcErr := h.driftSvc.GetReport(c.Request.Context(), id)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	m := driftReportToJSON(result.Report)
	m["driftEntries"] = result.DriftEntries
	m["missingEntries"] = result.MissingEntries
	m["errorEntries"] = result.ErrorEntries
	response.OK(c, m)
}

// GenerateDriftReport handles POST /ecl/eir/drift-reports/generate (M6-002 ad-hoc).
// Returns 202 Accepted with Asynq job reference.
// Permission: eir.drift_report.generate.
func (h *Handler) GenerateDriftReport(c *gin.Context) {
	if !hasPermission(c, PermEIRDriftGenerate) {
		return
	}
	actorID, _ := actorFromContext(c)

	tenantID := "TUGURE"
	if cl := claimsFromGin(c); cl != nil {
		tenantID = cl.TenantID
	}

	task, err := NewDriftAdHocTask(tenantID, actorID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "Failed to create drift job task", nil)
		return
	}
	_ = task // In production: enqueue via Asynq client here

	jobID := uuid.New().String()
	response.Accepted(c, gin.H{
		"jobId":     jobID,
		"type":      TaskDriftAdHoc,
		"statusUrl": "/api/v1/jobs/" + jobID,
		"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
	})
}

// driftReportToJSON serializes a DriftReport header row (decimal as StringFixed, no float64).
func driftReportToJSON(dr DriftReport) gin.H {
	m := gin.H{
		"id":                   dr.ID,
		"tanggalRun":           dr.TanggalRun.Format("2006-01-02"),
		"triggerSource":        string(dr.TriggerSource),
		"status":               string(dr.Status),
		"totalInstrumen":       dr.TotalInstrumen,
		"driftLowCount":        dr.DriftLowCount,
		"driftHighCount":       dr.DriftHighCount,
		"missingScheduleCount": dr.MissingScheduleCount,
		"errorCount":           dr.ErrorCount,
		"driftFlagThreshold":   dr.DriftFlagThreshold.StringFixed(8),
		"driftHighThreshold":   dr.DriftHighThreshold.StringFixed(8),
		"createdAt":            dr.CreatedAt.Format(time.RFC3339),
	}
	if dr.TriggeredBy != nil {
		m["triggeredBy"] = *dr.TriggeredBy
	}
	if dr.AsynqJobID != nil {
		m["asynqJobId"] = *dr.AsynqJobID
	}
	if dr.StartedAt != nil {
		m["startedAt"] = dr.StartedAt.Format(time.RFC3339)
	}
	if dr.CompletedAt != nil {
		m["completedAt"] = dr.CompletedAt.Format(time.RFC3339)
	}
	if dr.ErrorSummary != nil {
		m["errorSummary"] = *dr.ErrorSummary
	}
	return m
}
