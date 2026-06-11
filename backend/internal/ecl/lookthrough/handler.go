// Package lookthrough — Gin HTTP handlers for 12 look-through ECL endpoints.
//
// Endpoint → operationId mapping (api/openapi/app-c-lookthrough.yaml):
//
//	POST /ecl/lookthrough/composition/submit         submitFundComposition
//	POST /ecl/lookthrough/composition/{id}/review    reviewFundComposition
//	POST /ecl/lookthrough/composition/{id}/approve   approveFundComposition
//	POST /ecl/lookthrough/composition/{id}/reject    rejectFundComposition
//	GET  /ecl/lookthrough/compositions               listFundCompositions
//	GET  /ecl/lookthrough/compositions/{id}          getFundComposition
//	POST /ecl/lookthrough/compute                    computeLookthrough
//	POST /ecl/lookthrough/compute/bulk               bulkComputeLookthrough
//	GET  /ecl/lookthrough/preview                    listLookthroughPreview
//	GET  /ecl/lookthrough/preview/export             exportLookthroughPreview
//	GET  /ecl/lookthrough/result/{instrumenId}/{runId} getLookthroughResult
//	POST /ecl/lookthrough/composition/{id}/amend     amendFundComposition
//
// Permission guards:
//   - Submit/Amend:  fund_composition.create  (ROLE-AKUN)
//   - Review:        fund_composition.review  (ROLE-RISK)
//   - Approve:       fund_composition.approve (ROLE-ALCO, MFA wajib DEC-026)
//   - List/Get:      fund_composition.read
//   - Compute/Bulk:  lookthrough.compute
//   - Preview:       lookthrough.preview
//
// Idempotency-Key: required on POST mutating endpoints (checked by middleware, routes.go).
// MFA: ROLE-ALCO approve checks mfa_verified claim. No step-up (state-machine §5.2).
// No float64 for money/rates (DEC-016); Decimal serialized via StringFixed(4 or 8).
package lookthrough

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// CompositionServiceIface is the subset of *CompositionService that Handler uses.
// Tests inject mockCompositionService via this interface.
type CompositionServiceIface interface {
	Submit(ctx context.Context, req SubmitCompositionRequest, actorID uuid.UUID, actorRole string) (*CompositionGroup, error)
	Review(ctx context.Context, req WorkflowActionRequest) (*FundComposition, error)
	Approve(ctx context.Context, req WorkflowActionRequest, supersedesID *uuid.UUID) (*FundComposition, error)
	Reject(ctx context.Context, req WorkflowActionRequest) (*FundComposition, error)
	GetCompositionGroup(ctx context.Context, compositionID uuid.UUID) (*CompositionGroup, error)
	ListCompositions(ctx context.Context, instrumenID uuid.UUID, filterStatus, cursor string, limit int, sortCol, sortDir string) ([]FundComposition, string, bool, error)
}

// ServiceIface is the subset of *LookthroughService that Handler uses.
// Tests inject mockLookthroughService via this interface.
type ServiceIface interface {
	Compute(ctx context.Context, instrumenID, runID, periodeID uuid.UUID, evaluationDate time.Time) (*Result, error)
	Preview(ctx context.Context, periodeID uuid.UUID, evaluationDate time.Time, cursor string, limit int) ([]PreviewSummaryRow, string, bool, error)
}

// Handler holds composition + lookthrough services.
type Handler struct {
	composition CompositionServiceIface
	lookthrough ServiceIface
	resultRepo  ResultRepo
}

// NewHandler creates a Handler.
func NewHandler(composition CompositionServiceIface, lookthrough ServiceIface, resultRepo ResultRepo) *Handler {
	return &Handler{
		composition: composition,
		lookthrough: lookthrough,
		resultRepo:  resultRepo,
	}
}

// ─── Auth helpers ─────────────────────────────────────────────────────────────

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

// hasMFAVerified returns true if JWT claim mfa_verified == true.
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

// traceID reads X-Trace-Id from gin context (injected by gateway middleware).
func traceID(c *gin.Context) string {
	if t, exists := c.Get("trace_id"); exists {
		if s, ok := t.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Trace-Id")
}

// currentUserID extracts the JWT subject as uuid.UUID.
func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	sub, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	switch v := sub.(type) {
	case uuid.UUID:
		return v, true
	case string:
		id, err := uuid.Parse(v)
		if err == nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

// currentUserRole reads the first role from JWT claims.
func currentUserRole(c *gin.Context) string {
	rolesRaw, exists := c.Get("roles")
	if !exists {
		return "UNKNOWN"
	}
	switch v := rolesRaw.(type) {
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return "UNKNOWN"
}

// parseUUIDParam parses a path parameter as uuid.UUID; writes 400 on error.
func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	s := c.Param(name)
	id, err := uuid.Parse(s)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed,
			fmt.Sprintf("Parameter '%s' harus valid UUID v4.", name), nil)
		return uuid.Nil, false
	}
	return id, true
}

// parseDateQuery parses a query param as time.Time (YYYY-MM-DD).
func parseDateQuery(c *gin.Context, name string, required bool) (time.Time, bool) {
	s := c.Query(name)
	if s == "" {
		if required {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed,
				fmt.Sprintf("Query parameter '%s' wajib diisi (format YYYY-MM-DD).", name), nil)
			return time.Time{}, false
		}
		return time.Time{}, true
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed,
			fmt.Sprintf("Query parameter '%s' harus format YYYY-MM-DD. Terima: '%s'.", name, s), nil)
		return time.Time{}, false
	}
	return t, true
}

// parseLimitQuery parses ?limit= query; default 50, max 200.
func parseLimitQuery(c *gin.Context) int {
	s := c.DefaultQuery("limit", "50")
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// handleDomainError maps a domain error to the appropriate Gin response.
func handleDomainError(c *gin.Context, err error) {
	if de, ok := domainerrors.IsDomainError(err); ok {
		response.ErrorWithStatus(c, de.HTTPStatus(), de.Code(), de.Message(), de.Details())
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal. Hubungi admin dengan traceId.", nil)
}

// requireUserID is a helper that writes 401 and returns false if user_id is missing.
func requireUserID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := currentUserID(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "User ID tidak ditemukan di JWT.", nil)
		return uuid.Nil, false
	}
	return id, true
}

// ─── Response DTOs ────────────────────────────────────────────────────────────

type compositionDetailDTO struct {
	ID         string `json:"id"`
	AssetClass string `json:"assetClass"`
	WeightPct  string `json:"weightPct"`
	Position   int    `json:"position"`
}

type compositionDTO struct {
	ID              string                 `json:"id"`
	InstrumenID     string                 `json:"instrumenId"`
	EffectiveFrom   string                 `json:"effectiveFrom"`
	EffectiveTo     string                 `json:"effectiveTo"`
	WorkflowStatus  string                 `json:"workflowStatus"`
	MakerID         string                 `json:"makerId"`
	ReviewerID      *string                `json:"reviewerId,omitempty"`
	ApproverID      *string                `json:"approverId,omitempty"`
	SignedAtReview  *string                `json:"signedAtReview,omitempty"`
	SignedAtApprove *string                `json:"signedAtApprove,omitempty"`
	CommentReview   *string                `json:"commentReview,omitempty"`
	CommentApprove  *string                `json:"commentApprove,omitempty"`
	RejectReason    *string                `json:"rejectReason,omitempty"`
	SourceDocID     *string                `json:"sourceDocId,omitempty"`
	Details         []compositionDetailDTO `json:"details,omitempty"`
	TotalWeightPct  string                 `json:"totalWeightPct,omitempty"`
	CreatedAt       string                 `json:"createdAt"`
	UpdatedAt       string                 `json:"updatedAt"`
	RowVersion      int64                  `json:"rowVersion"`
	TenantID        string                 `json:"tenantId"`
}

type breakdownLineDTO struct {
	AssetClass     string `json:"assetClass"`
	WeightPct      string `json:"weightPct"`
	NabPortionIdr  string `json:"nabPortionIdr"`
	PdGood         string `json:"pdGood"`
	PdNormal       string `json:"pdNormal"`
	PdBad          string `json:"pdBad"`
	Lgd            string `json:"lgd"`
	EclGoodIdr     string `json:"eclGoodIdr"`
	EclNormalIdr   string `json:"eclNormalIdr"`
	EclBadIdr      string `json:"eclBadIdr"`
	EclFlGoodIdr   string `json:"eclFlGoodIdr"`
	EclFlNormalIdr string `json:"eclFlNormalIdr"`
	EclFlBadIdr    string `json:"eclFlBadIdr"`
	EclWeightedIdr string `json:"eclWeightedIdr"`
}

type lookthroughResultDTO struct {
	InstrumenID                  string             `json:"instrumenId"`
	InstrumenNama                string             `json:"instrumenNama"`
	KlasifikasiPsak71            string             `json:"klasifikasiPsak71"`
	NabIdr                       string             `json:"nabIdr"`
	FundCompositionID            string             `json:"fundCompositionId"`
	FundCompositionEffectiveFrom string             `json:"fundCompositionEffectiveFrom"`
	TotalEclIdr                  string             `json:"totalEclIdr"`
	Breakdown                    []breakdownLineDTO `json:"breakdown"`
	FvtplSkipped                 bool               `json:"fvtplSkipped"`
	Warning                      string             `json:"warning,omitempty"`
}

type previewRowDTO struct {
	InstrumenID                  string  `json:"instrumenId"`
	InstrumenNama                string  `json:"instrumenNama"`
	KlasifikasiPsak71            string  `json:"klasifikasiPsak71"`
	NabIdr                       string  `json:"nabIdr,omitempty"`
	FundCompositionID            *string `json:"fundCompositionId,omitempty"`
	FundCompositionEffectiveFrom *string `json:"fundCompositionEffectiveFrom,omitempty"`
	HasComposition               bool    `json:"hasComposition"`
	TotalEclEstimateIdr          *string `json:"totalEclEstimateIdr,omitempty"`
	Warnings                     []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"warnings,omitempty"`
}

// ─── DTOs conversion helpers ─────────────────────────────────────────────────

func toCompositionDTO(fc *FundComposition, details []FundCompositionDetail) compositionDTO {
	dto := compositionDTO{
		ID:             fc.ID.String(),
		InstrumenID:    fc.InstrumenID.String(),
		EffectiveFrom:  fc.EffectiveFrom.Format("2006-01-02"),
		EffectiveTo:    fc.EffectiveTo.Format("2006-01-02"),
		WorkflowStatus: string(fc.WorkflowStatus),
		MakerID:        fc.MakerID.String(),
		CreatedAt:      fc.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      fc.UpdatedAt.Format(time.RFC3339),
		RowVersion:     fc.RowVersion,
		TenantID:       fc.TenantID,
	}
	if fc.ReviewerID != nil {
		s := fc.ReviewerID.String()
		dto.ReviewerID = &s
	}
	if fc.ApproverID != nil {
		s := fc.ApproverID.String()
		dto.ApproverID = &s
	}
	if fc.SignedAtReview != nil {
		s := fc.SignedAtReview.Format(time.RFC3339)
		dto.SignedAtReview = &s
	}
	if fc.SignedAtApprove != nil {
		s := fc.SignedAtApprove.Format(time.RFC3339)
		dto.SignedAtApprove = &s
	}
	dto.CommentReview = fc.CommentReview
	dto.CommentApprove = fc.CommentApprove
	dto.RejectReason = fc.RejectReason
	if fc.SourceDocID != nil {
		s := fc.SourceDocID.String()
		dto.SourceDocID = &s
	}

	if details != nil {
		dto.Details = make([]compositionDetailDTO, len(details))
		var total decimal.Decimal
		for i := range details {
			d := &details[i]
			dto.Details[i] = compositionDetailDTO{
				ID:         d.ID.String(),
				AssetClass: string(d.AssetClass),
				WeightPct:  d.WeightPct.StringFixed(4),
				Position:   d.Position,
			}
			total = total.Add(d.WeightPct)
		}
		dto.TotalWeightPct = total.StringFixed(4)
	}
	return dto
}

func toLookthroughResultDTO(r *Result) lookthroughResultDTO {
	dto := lookthroughResultDTO{
		InstrumenID:       r.InstrumenID.String(),
		InstrumenNama:     r.InstrumenNama,
		KlasifikasiPsak71: r.KlasifikasiPsak71,
		NabIdr:            r.NABIDR.StringFixed(4),
		TotalEclIdr:       r.TotalECLIDR.StringFixed(4),
		FvtplSkipped:      r.FVTPLSkipped,
		Warning:           r.Warning,
	}
	if r.FundCompositionID != (uuid.UUID{}) {
		dto.FundCompositionID = r.FundCompositionID.String()
		dto.FundCompositionEffectiveFrom = r.FundCompositionEffectiveFrom.Format("2006-01-02")
	}
	dto.Breakdown = make([]breakdownLineDTO, len(r.Breakdown))
	for i := range r.Breakdown {
		b := &r.Breakdown[i]
		dto.Breakdown[i] = breakdownLineDTO{
			AssetClass:     string(b.AssetClass),
			WeightPct:      b.WeightPct.StringFixed(4),
			NabPortionIdr:  b.NABPortionIDR.StringFixed(4),
			PdGood:         b.PDGood.StringFixed(8),
			PdNormal:       b.PDNormal.StringFixed(8),
			PdBad:          b.PDBad.StringFixed(8),
			Lgd:            b.LGD.StringFixed(8),
			EclGoodIdr:     b.ECLSkenariosGoodIDR.StringFixed(4),
			EclNormalIdr:   b.ECLSkenariosNormalIDR.StringFixed(4),
			EclBadIdr:      b.ECLSkenariosBadIDR.StringFixed(4),
			EclFlGoodIdr:   b.ECLFLGoodIDR.StringFixed(4),
			EclFlNormalIdr: b.ECLFLNormalIDR.StringFixed(4),
			EclFlBadIdr:    b.ECLFLBadIDR.StringFixed(4),
			EclWeightedIdr: b.ECLWeightedIDR.StringFixed(4),
		}
	}
	return dto
}

// ─── Composition workflow handlers ───────────────────────────────────────────

// SubmitComposition handles POST /ecl/lookthrough/composition/submit
// Permission: fund_composition.create (ROLE-AKUN)
// Idempotency-Key: checked by middleware.
// AC: APP-C-LKT-002-AC01..07.
func (h *Handler) SubmitComposition(c *gin.Context) {
	if !hasPermission(c, PermFundCompositionCreate) {
		return
	}
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}

	var body struct {
		InstrumenID             string `json:"instrumenId"             binding:"required"`
		EffectiveFrom           string `json:"effectiveFrom"           binding:"required"`
		SourceDocID             string `json:"sourceDocId"`
		IsAmendment             bool   `json:"isAmendment"`
		SupersedesCompositionID string `json:"supersedesCompositionId"`
		Lines                   []struct {
			AssetClass string `json:"assetClass" binding:"required"`
			WeightPct  string `json:"weightPct"  binding:"required"`
			Position   int    `json:"position"`
		} `json:"lines" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}

	instrumenID, err := uuid.Parse(body.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"instrumenId harus valid UUID v4.", nil)
		return
	}
	effectiveFrom, err := time.Parse("2006-01-02", body.EffectiveFrom)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"effectiveFrom harus format YYYY-MM-DD.", nil)
		return
	}

	lines := make([]CompositionLineInput, len(body.Lines))
	for i, l := range body.Lines {
		w, wErr := decimal.NewFromString(l.WeightPct)
		if wErr != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
				fmt.Sprintf("lines[%d].weightPct bukan angka valid: %s", i, l.WeightPct), nil)
			return
		}
		lines[i] = CompositionLineInput{
			AssetClass: AssetClass(l.AssetClass),
			WeightPct:  w,
			Position:   l.Position,
		}
	}

	req := SubmitCompositionRequest{
		InstrumenID:   instrumenID,
		EffectiveFrom: effectiveFrom,
		Lines:         lines,
		IsAmendment:   body.IsAmendment,
	}
	if body.SourceDocID != "" {
		docID, e := uuid.Parse(body.SourceDocID)
		if e != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
				"sourceDocId harus valid UUID v4.", nil)
			return
		}
		req.SourceDocID = &docID
	}
	if body.SupersedesCompositionID != "" {
		superID, e := uuid.Parse(body.SupersedesCompositionID)
		if e != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
				"supersedesCompositionId harus valid UUID v4.", nil)
			return
		}
		req.SupersedesCompositionID = &superID
	}

	group, svcErr := h.composition.Submit(c.Request.Context(), req, actorID, currentUserRole(c))
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": toCompositionDTO(&group.Header, group.Details),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ReviewComposition handles POST /ecl/lookthrough/composition/{id}/review
// Permission: fund_composition.review (ROLE-RISK)
// SoD: reviewer ≠ maker.
// AC: APP-C-LKT-002-AC08..12.
func (h *Handler) ReviewComposition(c *gin.Context) {
	if !hasPermission(c, PermFundCompositionReview) {
		return
	}
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	compositionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var body struct {
		Comment         string `json:"comment"`
		SignatureMethod string `json:"signatureMethod"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"Payload tidak valid: "+err.Error(), nil)
		return
	}

	comp, svcErr := h.composition.Review(c.Request.Context(), WorkflowActionRequest{
		CompositionID:   compositionID,
		ActorID:         actorID,
		ActorRole:       currentUserRole(c),
		Comment:         body.Comment,
		SignatureMethod: body.SignatureMethod,
		TenantID:        defaultTenantID,
	})
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toCompositionDTO(comp, nil),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ApproveComposition handles POST /ecl/lookthrough/composition/{id}/approve
// Permission: fund_composition.approve (ROLE-ALCO)
// MFA: wajib (DEC-026). No step-up (state-machine §5.2).
// SoD: approver ≠ maker AND approver ≠ reviewer.
// AC: APP-C-LKT-002-AC13..18.
func (h *Handler) ApproveComposition(c *gin.Context) {
	if !hasPermission(c, PermFundCompositionApprove) {
		return
	}
	// MFA check for ROLE-ALCO (DEC-026).
	if !hasMFAVerified(c) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeMFARequired, "ROLE-ALCO wajib MFA. Pastikan mfa_verified=true di token.", nil)
		return
	}
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	compositionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var body struct {
		Comment                 string `json:"comment"`
		SignatureMethod         string `json:"signatureMethod"`
		SupersedesCompositionID string `json:"supersedesCompositionId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"Payload tidak valid: "+err.Error(), nil)
		return
	}

	var supersedesID *uuid.UUID
	if body.SupersedesCompositionID != "" {
		id, e := uuid.Parse(body.SupersedesCompositionID)
		if e != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
				"supersedesCompositionId harus valid UUID v4.", nil)
			return
		}
		supersedesID = &id
	}

	comp, svcErr := h.composition.Approve(c.Request.Context(), WorkflowActionRequest{
		CompositionID:   compositionID,
		ActorID:         actorID,
		ActorRole:       currentUserRole(c),
		Comment:         body.Comment,
		SignatureMethod: body.SignatureMethod,
		TenantID:        defaultTenantID,
	}, supersedesID)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toCompositionDTO(comp, nil),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// RejectComposition handles POST /ecl/lookthrough/composition/{id}/reject
// Permission: fund_composition.review or fund_composition.approve (either role can reject)
// SoD: rejector ≠ maker.
// AC: APP-C-LKT-002-AC19..22.
func (h *Handler) RejectComposition(c *gin.Context) {
	// Either reviewer or approver may reject.
	if !hasPermission(c, PermFundCompositionReview) && !hasPermission(c, PermFundCompositionApprove) {
		return
	}
	actorID, ok := requireUserID(c)
	if !ok {
		return
	}
	compositionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var body struct {
		Comment         string `json:"comment" binding:"required"`
		SignatureMethod string `json:"signatureMethod"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"comment wajib diisi untuk penolakan. Error: "+err.Error(), nil)
		return
	}

	comp, svcErr := h.composition.Reject(c.Request.Context(), WorkflowActionRequest{
		CompositionID:   compositionID,
		ActorID:         actorID,
		ActorRole:       currentUserRole(c),
		Comment:         body.Comment,
		SignatureMethod: body.SignatureMethod,
		TenantID:        defaultTenantID,
	})
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toCompositionDTO(comp, nil),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ListCompositions handles GET /ecl/lookthrough/compositions
// Permission: fund_composition.read (DataTable pattern: sort + page + filter)
// AC: APP-C-LKT-002-AC23..26.
func (h *Handler) ListCompositions(c *gin.Context) {
	if !hasPermission(c, PermFundCompositionRead) {
		return
	}

	instrumenIDStr := c.Query("filter[instrumen_id]")
	instrumenID, err := uuid.Parse(instrumenIDStr)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"filter[instrumen_id] wajib diisi dan harus valid UUID v4.", nil)
		return
	}

	filterStatus := c.Query("filter[workflow_status]")
	cursor := c.Query("cursor")
	limit := parseLimitQuery(c)
	sp := parseSortParam(c.Query("sort"), AllowedSortColsComposition, "created_at", "desc")

	compositions, nextCursor, hasMore, svcErr := h.composition.ListCompositions(
		c.Request.Context(), instrumenID, filterStatus, cursor, limit, sp.col, sp.dir)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	dtos := make([]compositionDTO, 0, len(compositions))
	for i := range compositions {
		dtos = append(dtos, toCompositionDTO(&compositions[i], nil))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dtos,
		"pagination": gin.H{
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
			"limit":      limit,
		},
		"appliedSort":   []gin.H{{"col": sp.col, "dir": sp.dir}},
		"appliedFilter": gin.H{"instrumen_id": instrumenIDStr, "workflow_status": filterStatus},
		"meta":          gin.H{"traceId": traceID(c)},
	})
}

// GetComposition handles GET /ecl/lookthrough/compositions/{id}
// Permission: fund_composition.read
func (h *Handler) GetComposition(c *gin.Context) {
	if !hasPermission(c, PermFundCompositionRead) {
		return
	}
	compositionID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	group, svcErr := h.composition.GetCompositionGroup(c.Request.Context(), compositionID)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toCompositionDTO(&group.Header, group.Details),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── ECL compute handlers ─────────────────────────────────────────────────────

// ComputeLookthrough handles POST /ecl/lookthrough/compute
// Permission: lookthrough.compute
// Idempotency-Key: checked by middleware.
// AC: APP-C-LKT-001-AC01..10.
func (h *Handler) ComputeLookthrough(c *gin.Context) {
	if !hasPermission(c, PermLookthroughCompute) {
		return
	}

	var body struct {
		InstrumenID    string `json:"instrumenId"    binding:"required"`
		RunID          string `json:"runId"          binding:"required"`
		PeriodeID      string `json:"periodeId"      binding:"required"`
		EvaluationDate string `json:"evaluationDate" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"Payload tidak valid: "+err.Error(), nil)
		return
	}

	instrumenID, err := uuid.Parse(body.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "instrumenId harus valid UUID v4.", nil)
		return
	}
	runID, err := uuid.Parse(body.RunID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "runId harus valid UUID v4.", nil)
		return
	}
	periodeID, err := uuid.Parse(body.PeriodeID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "periodeId harus valid UUID v4.", nil)
		return
	}
	evalDate, err := time.Parse("2006-01-02", body.EvaluationDate)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "evaluationDate harus format YYYY-MM-DD.", nil)
		return
	}

	result, svcErr := h.lookthrough.Compute(c.Request.Context(), instrumenID, runID, periodeID, evalDate)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toLookthroughResultDTO(result),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// BulkComputeLookthrough handles POST /ecl/lookthrough/compute/bulk
// Returns 202 + jobId (Asynq job, ux-patterns §3).
// Permission: lookthrough.compute
// AC: APP-C-LKT-001-AC11..16.
func (h *Handler) BulkComputeLookthrough(c *gin.Context) {
	if !hasPermission(c, PermLookthroughCompute) {
		return
	}

	var body struct {
		RunID          string `json:"runId"          binding:"required"`
		PeriodeID      string `json:"periodeId"      binding:"required"`
		EvaluationDate string `json:"evaluationDate" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"Payload tidak valid: "+err.Error(), nil)
		return
	}
	_, err := uuid.Parse(body.RunID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "runId harus valid UUID v4.", nil)
		return
	}
	_, err = uuid.Parse(body.PeriodeID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "periodeId harus valid UUID v4.", nil)
		return
	}
	_, err = time.Parse("2006-01-02", body.EvaluationDate)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed, "evaluationDate harus format YYYY-MM-DD.", nil)
		return
	}

	// Return 202 immediately — actual work dispatched via Asynq job (ux-patterns §3).
	// Full async dispatch in Phase 5. For M4: accepts job, logs, returns jobId.
	jobID := fmt.Sprintf("lt-bulk-%s-%s", body.RunID[:8], body.EvaluationDate)
	response.Accepted(c, gin.H{
		"jobId":                    jobID,
		"type":                     "LOOKTHROUGH_BULK_COMPUTE",
		"statusUrl":                "/api/v1/jobs/" + jobID,
		"streamUrl":                "/api/v1/jobs/" + jobID + "/stream",
		"estimatedDurationSeconds": 10,
	})
}

// ListLookthroughPreview handles GET /ecl/lookthrough/preview
// Permission: lookthrough.preview (DataTable pattern)
// AC: APP-C-LKT-001-AC17..20.
func (h *Handler) ListLookthroughPreview(c *gin.Context) {
	if !hasPermission(c, PermLookthroughPreview) {
		return
	}
	periodeID, err := uuid.Parse(c.Query("periode_id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"periode_id wajib diisi dan harus valid UUID v4.", nil)
		return
	}
	evalDate, ok := parseDateQuery(c, "evaluation_date", true)
	if !ok {
		return
	}
	cursor := c.Query("cursor")
	limit := parseLimitQuery(c)

	rows, nextCursor, hasMore, svcErr := h.lookthrough.Preview(c.Request.Context(), periodeID, evalDate, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	dtos := make([]previewRowDTO, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		dto := previewRowDTO{
			InstrumenID:       r.InstrumenID.String(),
			InstrumenNama:     r.InstrumenNama,
			KlasifikasiPsak71: r.KlasifikasiPsak71,
			HasComposition:    r.HasComposition,
		}
		if r.NABIDRStr != "" {
			dto.NabIdr = r.NABIDRStr
		}
		if r.FundCompositionID != nil {
			s := r.FundCompositionID.String()
			dto.FundCompositionID = &s
		}
		if r.FundCompositionEffectiveFrom != nil {
			s := r.FundCompositionEffectiveFrom.Format("2006-01-02")
			dto.FundCompositionEffectiveFrom = &s
		}
		dto.TotalEclEstimateIdr = r.TotalECLEstimateIDRStr
		for _, w := range r.Warnings {
			dto.Warnings = append(dto.Warnings, struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}{Code: w.Code, Message: w.Message})
		}
		dtos = append(dtos, dto)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dtos,
		"pagination": gin.H{
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
			"limit":      limit,
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ExportLookthroughPreview handles GET /ecl/lookthrough/preview/export
// Permission: lookthrough.preview
func (h *Handler) ExportLookthroughPreview(c *gin.Context) {
	if !hasPermission(c, PermLookthroughPreview) {
		return
	}
	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "xlsx" {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"format harus 'csv' atau 'xlsx'.", nil)
		return
	}

	periodeID, err := uuid.Parse(c.Query("periode_id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest, domainerrors.CodeValidationFailed,
			"periode_id wajib diisi dan harus valid UUID v4.", nil)
		return
	}
	evalDate, ok := parseDateQuery(c, "evaluation_date", true)
	if !ok {
		return
	}

	rows, _, _, svcErr := h.lookthrough.Preview(c.Request.Context(), periodeID, evalDate, "", 10000)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	if len(rows) >= 10000 || format == "xlsx" {
		jobID := fmt.Sprintf("lt-export-%s-%s", evalDate.Format("20060102"), format)
		response.Accepted(c, gin.H{
			"jobId":     jobID,
			"type":      "LOOKTHROUGH_PREVIEW_EXPORT",
			"statusUrl": "/api/v1/jobs/" + jobID,
			"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
		})
		return
	}

	filename := fmt.Sprintf("lookthrough-preview-%s.csv", evalDate.Format("20060102"))
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("X-Total-Rows", itoa(len(rows)))
	_, _ = c.Writer.WriteString("\xef\xbb\xbf")                                                                                      //nolint:errcheck
	fmt.Fprintf(c.Writer, "Instrumen ID,Nama,Klasifikasi,NAB IDR,Fund Composition ID,ECL Estimate IDR,Has Composition,Warnings\r\n") //nolint:errcheck
	for i := range rows {
		r := &rows[i]
		nabStr := r.NABIDRStr
		compIDStr := ""
		if r.FundCompositionID != nil {
			compIDStr = r.FundCompositionID.String()
		}
		eclStr := ""
		if r.TotalECLEstimateIDRStr != nil {
			eclStr = *r.TotalECLEstimateIDRStr
		}
		warnCount := len(r.Warnings)
		fmt.Fprintf(c.Writer, "%s,%q,%s,%s,%s,%s,%v,%d\r\n", //nolint:errcheck
			r.InstrumenID, r.InstrumenNama, r.KlasifikasiPsak71,
			nabStr, compIDStr, eclStr, r.HasComposition, warnCount,
		)
	}
}

// GetLookthroughResult handles GET /ecl/lookthrough/result/{instrumenId}/{runId}
// Permission: lookthrough.preview
func (h *Handler) GetLookthroughResult(c *gin.Context) {
	if !hasPermission(c, PermLookthroughPreview) {
		return
	}
	instrumenID, ok := parseUUIDParam(c, "instrumenId")
	if !ok {
		return
	}
	runID, ok := parseUUIDParam(c, "runId")
	if !ok {
		return
	}

	stored, err := h.resultRepo.GetByInstrumenAndRun(c.Request.Context(), instrumenID, runID)
	if err != nil {
		handleDomainError(c, err)
		return
	}
	if stored == nil {
		response.ErrorWithStatus(c, http.StatusNotFound, domainerrors.CodeNotFound,
			"Lookthrough result tidak ditemukan untuk instrumenId "+instrumenID.String()+" dan runId "+runID.String()+".", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":             stored.ID.String(),
			"instrumenId":    stored.InstrumenID.String(),
			"runId":          stored.RunID.String(),
			"compositionId":  stored.CompositionID.String(),
			"periodeId":      stored.PeriodeID.String(),
			"evaluationDate": stored.EvaluationDate.Format("2006-01-02"),
			"nabIdr":         stored.NABIDR.StringFixed(4),
			"totalEclIdr":    stored.TotalECLIDR.StringFixed(4),
			"fvtplSkipped":   stored.FVTPLSkipped,
			"warning":        stored.Warning,
			"createdAt":      stored.CreatedAt.Format(time.RFC3339),
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── Sort helper ──────────────────────────────────────────────────────────────

type sortSpec struct {
	col string
	dir string
}

// parseSortParam parses ?sort=col:asc (first entry). Falls back to defaultCol:defaultDir.
// Validates col against allowedCols whitelist.
func parseSortParam(s string, allowedCols []string, defaultCol, defaultDir string) sortSpec {
	if s == "" {
		return sortSpec{defaultCol, defaultDir}
	}
	entry := s
	for i, ch := range s {
		if ch == ',' {
			entry = s[:i]
			break
		}
	}
	col, dir := entry, "asc"
	for i, ch := range entry {
		if ch == ':' {
			col = entry[:i]
			dir = entry[i+1:]
			break
		}
	}
	allowed := false
	for _, a := range allowedCols {
		if a == col {
			allowed = true
			break
		}
	}
	if !allowed {
		return sortSpec{defaultCol, defaultDir}
	}
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	return sortSpec{col, dir}
}
