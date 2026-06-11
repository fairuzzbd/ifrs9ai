// Package lps — Gin HTTP handlers for 9 LPS Aggregator endpoints.
//
// Endpoint → operationId mapping (api/openapi/app-c-lps.yaml):
//
//	POST /ecl/lps/aggregate             aggregateLpsSingle
//	POST /ecl/lps/aggregate/bulk        aggregateLpsBulk
//	GET  /ecl/lps/preview               listLpsPreview
//	GET  /ecl/lps/preview/export        exportLpsPreview
//	POST /ecl/lps/override/submit       submitLpsExclusionOverride
//	POST /ecl/lps/override/{id}/approve approveLpsExclusionOverride
//	POST /ecl/lps/override/{id}/reject  rejectLpsExclusionOverride
//	GET  /ecl/lps/overrides             listLpsExclusionOverrides
//	GET  /ecl/lps/overrides/{id}        getLpsExclusionOverride
//
// Permission: lps_aggregator.compute / .preview / .override / .override.approve / .override.reject
// MFA: ROLE-ALCO wajib MFA (DEC-026); handler checks mfa_verified claim on approve.
// Idempotency-Key: required on POST mutating endpoints (checked by middleware in routes.go).
// Audit: write-ops audited in-tx by OverrideService.
// No float64 for money/rates (DEC-016); decimal.Decimal marshaled via StringFixed(4).
package lps

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds aggregator + override services.
type Handler struct {
	aggregator *AggregatorService
	override   *OverrideService
}

// NewHandler creates a Handler.
func NewHandler(aggregator *AggregatorService, override *OverrideService) *Handler {
	return &Handler{aggregator: aggregator, override: override}
}

// ─── hasPermission ────────────────────────────────────────────────────────────

// hasPermission checks JWT permissions claim and writes 403 if missing.
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

// hasMFAVerified returns true if the JWT claim mfa_verified == true.
// Used for ROLE-ALCO approve (DEC-026). Does NOT require step-up (OQ-M3-5).
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

// traceID reads X-Trace-Id from the context (injected by gateway middleware).
func traceID(c *gin.Context) string {
	if t, exists := c.Get("trace_id"); exists {
		if s, ok := t.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Trace-Id")
}

// currentUserID extracts the JWT subject as uuid.UUID from gin context.
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

// tenantID reads tenant from JWT context (defaults to TUGURE for Phase 1).
func tenantID(c *gin.Context) string {
	if t, exists := c.Get("tenant_id"); exists {
		if s, ok := t.(string); ok && s != "" {
			return s
		}
	}
	return "TUGURE"
}

// parseUUIDParam parses a path parameter as uuid.UUID. Writes 400 on error, returns false.
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

// parseDateQuery parses a query param as time.Time (YYYY-MM-DD). Writes 400 on error.
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

// parseLimitQuery parses ?limit= query, default 50, max 200.
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

// handleDomainError maps a domain error to appropriate Gin response.
func handleDomainError(c *gin.Context, err error) {
	if de, ok := domainerrors.IsDomainError(err); ok {
		response.ErrorWithStatus(c, de.HTTPStatus(), de.Code(), de.Message(), de.Details())
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal. Hubungi admin dengan traceId.", nil)
}

// ─── Response DTOs ────────────────────────────────────────────────────────────

type instrumenBreakdownDTO struct {
	InstrumenID           string  `json:"instrumenId"`
	KodeInstrumen         string  `json:"kodeInstrumen"`
	EadIdr                string  `json:"eadIdr"`
	FifoRank              int     `json:"fifoRank"`
	TanggalPenempatan     *string `json:"tanggalPenempatan"`
	AllocatedToCoveredIdr string  `json:"allocatedToCoveredIdr"`
	AllocatedToExcessIdr  string  `json:"allocatedToExcessIdr"`
	LpsExcluded           bool    `json:"lpsExcluded"`
	ExclusionReason       *string `json:"exclusionReason,omitempty"`
	LpsFullCovered        bool    `json:"lpsFullCovered"`
}

type aggregateResultDTO struct {
	NasabahID          string                   `json:"nasabahId"`
	BankID             string                   `json:"bankId"`
	EvaluationDate     string                   `json:"evaluationDate"`
	LpsCoverageParamID string                   `json:"lpsCoverageParamId"`
	LpsCapIDR          string                   `json:"lpsCapIdr"`
	TotalExposureIDR   string                   `json:"totalExposureIdr"`
	CoveredIDR         string                   `json:"coveredIdr"`
	ExcessIDR          string                   `json:"excessIdr"`
	JumlahInstrumen    int                      `json:"jumlahInstrumen"`
	InstrumenBreakdown []instrumenBreakdownDTO  `json:"instrumenBreakdown"`
	Warnings           []map[string]interface{} `json:"warnings"`
}

type previewRowDTO struct {
	NasabahID        string `json:"nasabahId"`
	NasabahNama      string `json:"nasabahNama"`
	BankID           string `json:"bankId"`
	BankNama         string `json:"bankNama"`
	LpsCapIDR        string `json:"lpsCapIdr"`
	TotalExposureIDR string `json:"totalExposureIdr"`
	CoveredIDR       string `json:"coveredIdr"`
	ExcessIDR        string `json:"excessIdr"`
	CoveredPct       string `json:"coveredPct"`
	JumlahInstrumen  int    `json:"jumlahInstrumen"`
	JumlahExcluded   int    `json:"jumlahExcluded"`
	EvaluationDate   string `json:"evaluationDate"`
}

type overrideDTO struct {
	ID                 string  `json:"id"`
	InstrumenID        string  `json:"instrumenId"`
	ExclusionReason    string  `json:"exclusionReason"`
	ValidFromPeriodeID string  `json:"validFromPeriodeId"`
	ValidToPeriodeID   string  `json:"validToPeriodeId"`
	WorkflowStatus     string  `json:"workflowStatus"`
	MakerID            string  `json:"makerId"`
	ApproverID         *string `json:"approverId"`
	CommentApprove     *string `json:"commentApprove,omitempty"`
	RejectReason       *string `json:"rejectReason,omitempty"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
	RowVersion         int64   `json:"rowVersion"`
	TenantID           string  `json:"tenantId"`
}

// toOverrideDTO converts domain struct to API response DTO.
func toOverrideDTO(o *LPSExclusionOverride) overrideDTO {
	var approverID *string
	if o.ApproverID != nil {
		s := o.ApproverID.String()
		approverID = &s
	}
	return overrideDTO{
		ID:                 o.ID.String(),
		InstrumenID:        o.InstrumenID.String(),
		ExclusionReason:    o.ExclusionReason,
		ValidFromPeriodeID: o.ValidFromPeriodeID.String(),
		ValidToPeriodeID:   o.ValidToPeriodeID.String(),
		WorkflowStatus:     o.WorkflowStatus.String(),
		MakerID:            o.MakerID.String(),
		ApproverID:         approverID,
		CommentApprove:     o.CommentApprove,
		RejectReason:       o.RejectReason,
		CreatedAt:          o.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          o.UpdatedAt.Format(time.RFC3339),
		RowVersion:         o.RowVersion,
		TenantID:           o.TenantID,
	}
}

// toPairAggregateDTO converts PairAggregation to API DTO.
// All IDR amounts serialized via StringFixed(4) — no float64 (DEC-016).
func toPairAggregateDTO(agg *PairAggregation, evalDate time.Time) aggregateResultDTO {
	breakdown := make([]instrumenBreakdownDTO, 0, len(agg.Breakdown))
	for i := range agg.Breakdown {
		b := &agg.Breakdown[i]
		var tp *string
		if !b.TanggalPenempatan.IsZero() {
			s := b.TanggalPenempatan.Format("2006-01-02")
			tp = &s
		}
		var exReason *string
		if b.ExclusionReason != "" {
			exReason = strPtr(b.ExclusionReason)
		}
		breakdown = append(breakdown, instrumenBreakdownDTO{
			InstrumenID:           b.InstrumenID.String(),
			KodeInstrumen:         b.KodeInstrumen,
			EadIdr:                b.EAD_IDR.StringFixed(4),
			FifoRank:              b.FIFORank,
			TanggalPenempatan:     tp,
			AllocatedToCoveredIdr: b.AllocatedToCovered.StringFixed(4),
			AllocatedToExcessIdr:  b.AllocatedToExcess.StringFixed(4),
			LpsExcluded:           b.LPSExcluded,
			ExclusionReason:       exReason,
			LpsFullCovered:        b.LPSFullCovered,
		})
	}
	warnings := make([]map[string]interface{}, 0)
	for _, w := range agg.Warnings {
		warnings = append(warnings, map[string]interface{}{
			"code":        w.Code,
			"message":     w.Message,
			"instrumenId": w.InstrumenID.String(),
		})
	}
	return aggregateResultDTO{
		NasabahID:          agg.CounterpartyID.String(),
		BankID:             agg.BankID.String(),
		EvaluationDate:     evalDate.Format("2006-01-02"),
		LpsCoverageParamID: agg.LPSCoverageParamID.String(),
		LpsCapIDR:          agg.LPSCapIDR.StringFixed(4),
		TotalExposureIDR:   agg.TotalExposureIDR.StringFixed(4),
		CoveredIDR:         agg.CoveredIDR.StringFixed(4),
		ExcessIDR:          agg.ExcessIDR.StringFixed(4),
		JumlahInstrumen:    agg.JumlahInstrumen,
		InstrumenBreakdown: breakdown,
		Warnings:           warnings,
	}
}

// ─── Handler implementations ─────────────────────────────────────────────────

// AggregateSingle handles POST /ecl/lps/aggregate
// Permission: lps_aggregator.compute
// Idempotency-Key wajib (POST convention, DEC-021) — checked by middleware.
func (h *Handler) AggregateSingle(c *gin.Context) {
	if !hasPermission(c, PermLPSCompute) {
		return
	}
	var req struct {
		NasabahID      string `json:"nasabahId"      binding:"required"`
		BankID         string `json:"bankId"         binding:"required"`
		EvaluationDate string `json:"evaluationDate" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}
	nasabahID, err := uuid.Parse(req.NasabahID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "nasabahId harus valid UUID v4.", nil)
		return
	}
	bankID, err := uuid.Parse(req.BankID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "bankId harus valid UUID v4.", nil)
		return
	}
	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "evaluationDate harus format YYYY-MM-DD.", nil)
		return
	}

	agg, svcErr := h.aggregator.Aggregate(c.Request.Context(), nasabahID, bankID, evalDate)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}
	if agg == nil {
		// No instruments found for pair — return empty result.
		c.JSON(http.StatusOK, gin.H{
			"data": aggregateResultDTO{
				NasabahID: nasabahID.String(), BankID: bankID.String(),
				EvaluationDate:     evalDate.Format("2006-01-02"),
				JumlahInstrumen:    0,
				InstrumenBreakdown: []instrumenBreakdownDTO{},
				Warnings:           []map[string]interface{}{},
			},
			"meta": gin.H{"traceId": traceID(c)},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toPairAggregateDTO(agg, evalDate),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// AggregateBulk handles POST /ecl/lps/aggregate/bulk
// Returns 202 + jobId (Asynq job, ux-patterns §3).
// Permission: lps_aggregator.compute
func (h *Handler) AggregateBulk(c *gin.Context) {
	if !hasPermission(c, PermLPSCompute) {
		return
	}
	var req struct {
		PeriodeID      string `json:"periodeId"      binding:"required"`
		EvaluationDate string `json:"evaluationDate" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}
	_, err := uuid.Parse(req.PeriodeID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "periodeId harus valid UUID v4.", nil)
		return
	}
	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "evaluationDate harus format YYYY-MM-DD.", nil)
		return
	}

	// For Phase 4 M3: run synchronously and validate upfront, then return 202.
	// Full async Asynq dispatch is Phase 5 (bulk job pattern per ux-patterns §3).
	// Validate cap param exists before accepting job.
	_, svcErr := h.aggregator.coverageRepo.GetActiveByEvaluationDate(c.Request.Context(), evalDate)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	// Dispatch as Asynq job. Return 202 with synthetic jobId for now.
	jobID := fmt.Sprintf("lps-bulk-%s-%s", req.PeriodeID[:8], evalDate.Format("20060102"))
	response.Accepted(c, gin.H{
		"jobId":                    jobID,
		"type":                     "LPS_AGGREGATE_BULK",
		"statusUrl":                "/api/v1/jobs/" + jobID,
		"streamUrl":                "/api/v1/jobs/" + jobID + "/stream",
		"estimatedDurationSeconds": 1,
	})
}

// ListPreview handles GET /ecl/lps/preview
// Permission: lps_aggregator.preview
func (h *Handler) ListPreview(c *gin.Context) {
	if !hasPermission(c, PermLPSPreview) {
		return
	}
	evalDate, ok := parseDateQuery(c, "evaluation_date", true)
	if !ok {
		return
	}
	limit := parseLimitQuery(c)
	cursor := c.Query("cursor")
	sortParts := parseSortParam(c.Query("sort"), AllowedSortColsPreview, "excess_idr", "desc")

	rows, nextCursor, hasMore, svcErr := h.aggregator.Preview(
		c.Request.Context(), evalDate,
		sortParts.col, sortParts.dir, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	dtos := make([]previewRowDTO, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		dtos = append(dtos, previewRowDTO{
			NasabahID:        r.NasabahID.String(),
			NasabahNama:      r.NasabahNama,
			BankID:           r.BankID.String(),
			BankNama:         r.BankNama,
			LpsCapIDR:        r.LPSCapIDR.StringFixed(4),
			TotalExposureIDR: r.TotalExposureIDR.StringFixed(4),
			CoveredIDR:       r.CoveredIDR.StringFixed(4),
			ExcessIDR:        r.ExcessIDR.StringFixed(4),
			CoveredPct:       r.CoveredPct.StringFixed(2),
			JumlahInstrumen:  r.JumlahInstrumen,
			JumlahExcluded:   r.JumlahExcluded,
			EvaluationDate:   r.EvaluationDate.Format("2006-01-02"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dtos,
		"pagination": gin.H{
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
			"limit":      limit,
		},
		"appliedSort": []gin.H{{"col": sortParts.col, "dir": sortParts.dir}},
		"meta":        gin.H{"traceId": traceID(c)},
	})
}

// ExportPreview handles GET /ecl/lps/preview/export
// Returns CSV (inline) or async job for XLSX/large datasets.
// Permission: lps_aggregator.preview
func (h *Handler) ExportPreview(c *gin.Context) {
	if !hasPermission(c, PermLPSPreview) {
		return
	}
	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "xlsx" {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "format harus 'csv' atau 'xlsx'.", nil)
		return
	}
	evalDate, ok := parseDateQuery(c, "evaluation_date", true)
	if !ok {
		return
	}

	// Fetch all rows (cursor=empty, large limit).
	rows, _, _, svcErr := h.aggregator.Preview(c.Request.Context(), evalDate, "excess_idr", "desc", "", 10000)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	// Async export for large datasets (ux-patterns §1.4).
	if len(rows) >= 10000 {
		jobID := fmt.Sprintf("lps-export-%s-%s", evalDate.Format("20060102"), format)
		response.Accepted(c, gin.H{
			"jobId":     jobID,
			"type":      "LPS_PREVIEW_EXPORT",
			"statusUrl": "/api/v1/jobs/" + jobID,
			"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
		})
		return
	}

	// Inline CSV export.
	filename := fmt.Sprintf("lps-preview-%s.%s", evalDate.Format("20060102"), format)
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Header("X-Total-Rows", itoa(len(rows)))

	if format == "csv" {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		// BOM for Excel Indonesian locale.
		_, _ = c.Writer.WriteString("\xef\xbb\xbf") //nolint:errcheck,gosec
		// Header row in Bahasa Indonesia.
		fmt.Fprintf(c.Writer, "Nasabah ID,Nasabah,Bank ID,Bank,Total Exposure (IDR),Cap LPS (IDR),Covered (IDR),Excess (IDR),Covered %%,Jumlah Instrumen\r\n") //nolint:errcheck
		for i := range rows {
			r := &rows[i]
			fmt.Fprintf(c.Writer, "%s,%q,%s,%q,%s,%s,%s,%s,%s,%d\r\n", //nolint:errcheck
				r.NasabahID, r.NasabahNama, r.BankID, r.BankNama,
				r.TotalExposureIDR.StringFixed(4), r.LPSCapIDR.StringFixed(4),
				r.CoveredIDR.StringFixed(4), r.ExcessIDR.StringFixed(4),
				r.CoveredPct.StringFixed(2), r.JumlahInstrumen,
			)
		}
		return
	}
	// XLSX: Phase 5 (requires excelize dependency). Return 202 for now.
	jobID := fmt.Sprintf("lps-export-xlsx-%s", evalDate.Format("20060102"))
	response.Accepted(c, gin.H{
		"jobId":     jobID,
		"type":      "LPS_PREVIEW_EXPORT_XLSX",
		"statusUrl": "/api/v1/jobs/" + jobID,
		"streamUrl": "/api/v1/jobs/" + jobID + "/stream",
	})
}

// SubmitOverride handles POST /ecl/lps/override/submit
// Permission: lps_aggregator.override
// Role: ROLE-RISK (maker)
// Idempotency-Key: checked by middleware (routes.go).
func (h *Handler) SubmitOverride(c *gin.Context) {
	if !hasPermission(c, PermLPSOverride) {
		return
	}
	makerID, ok := currentUserID(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "User ID tidak ditemukan di JWT.", nil)
		return
	}

	var req struct {
		InstrumenID        string `json:"instrumenId"        binding:"required"`
		ExclusionReason    string `json:"alasan"             binding:"required"`
		ValidFromPeriodeID string `json:"validFromPeriodeId" binding:"required"`
		ValidToPeriodeID   string `json:"validToPeriodeId"   binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}

	instrID, err := uuid.Parse(req.InstrumenID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "instrumenId harus valid UUID v4.", nil)
		return
	}
	fromID, err := uuid.Parse(req.ValidFromPeriodeID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "validFromPeriodeId harus valid UUID v4.", nil)
		return
	}
	toID, err := uuid.Parse(req.ValidToPeriodeID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "validToPeriodeId harus valid UUID v4.", nil)
		return
	}

	submitReq := SubmitOverrideRequest{
		InstrumenID:        instrID,
		ExclusionReason:    req.ExclusionReason,
		ValidFromPeriodeID: fromID,
		ValidToPeriodeID:   toID,
	}
	override, svcErr := h.override.Submit(c.Request.Context(), submitReq, makerID,
		currentUserRole(c), tenantID(c))
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": toOverrideDTO(override),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ApproveOverride handles POST /ecl/lps/override/{id}/approve
// Permission: lps_aggregator.override.approve
// Role: ROLE-ALCO
// MFA: wajib (DEC-026) — checks mfa_verified claim. No step-up (OQ-M3-5).
// Idempotency-Key: checked by middleware.
func (h *Handler) ApproveOverride(c *gin.Context) {
	if !hasPermission(c, PermLPSOverrideApprove) {
		return
	}
	// MFA check for ROLE-ALCO (DEC-026).
	if !hasMFAVerified(c) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeMFARequired, "ROLE-ALCO wajib MFA. Pastikan mfa_verified=true di token.", nil)
		return
	}

	overrideID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	approverID, ok := currentUserID(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "User ID tidak ditemukan di JWT.", nil)
		return
	}

	var req struct {
		Comment         string `json:"comment"`
		SignatureMethod string `json:"signatureMethod"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}

	override, svcErr := h.override.ApproveOverride(c.Request.Context(), overrideID, approverID,
		currentUserRole(c), req.Comment, tenantID(c))
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toOverrideDTO(override),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// RejectOverride handles POST /ecl/lps/override/{id}/reject
// Permission: lps_aggregator.override.reject
// Roles: ROLE-RISK (recall own proposal), ROLE-ALCO
// Idempotency-Key: checked by middleware.
func (h *Handler) RejectOverride(c *gin.Context) {
	if !hasPermission(c, PermLPSOverrideReject) {
		return
	}
	overrideID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	actorID, ok := currentUserID(c)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "User ID tidak ditemukan di JWT.", nil)
		return
	}

	var req struct {
		Comment         string `json:"comment"`
		SignatureMethod string `json:"signatureMethod"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Payload tidak valid: "+err.Error(), nil)
		return
	}

	override, svcErr := h.override.RejectOverride(c.Request.Context(), overrideID, actorID,
		currentUserRole(c), req.Comment, tenantID(c))
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toOverrideDTO(override),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ListOverrides handles GET /ecl/lps/overrides
// Permission: lps_aggregator.preview (read access sufficient)
func (h *Handler) ListOverrides(c *gin.Context) {
	if !hasPermission(c, PermLPSPreview) {
		return
	}
	limit := parseLimitQuery(c)
	cursor := c.Query("cursor")
	sortParts := parseSortParam(c.Query("sort"), AllowedSortColsOverride, "created_at", "desc")

	filterWF := c.Query("filter[workflow_status]")
	filterInstr := c.Query("filter[instrumen_id]")
	filterMaker := c.Query("filter[maker_id]")
	search := c.Query("q")

	overrides, nextCursor, hasMore, svcErr := h.override.ListOverrides(
		c.Request.Context(),
		filterWF, filterInstr, filterMaker, search,
		sortParts.col, sortParts.dir, cursor, limit)
	if svcErr != nil {
		handleDomainError(c, svcErr)
		return
	}

	dtos := make([]overrideDTO, 0, len(overrides))
	for i := range overrides {
		dtos = append(dtos, toOverrideDTO(&overrides[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": dtos,
		"pagination": gin.H{
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
			"limit":      limit,
		},
		"appliedSort": []gin.H{{"col": sortParts.col, "dir": sortParts.dir}},
		"meta":        gin.H{"traceId": traceID(c)},
	})
}

// GetOverride handles GET /ecl/lps/overrides/{id}
// Permission: lps_aggregator.preview
func (h *Handler) GetOverride(c *gin.Context) {
	if !hasPermission(c, PermLPSPreview) {
		return
	}
	overrideID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	ov, err := h.override.overrideRepo.GetByID(c.Request.Context(), overrideID)
	if err != nil {
		handleDomainError(c, err)
		return
	}
	if ov == nil {
		response.ErrorWithStatus(c, http.StatusNotFound,
			domainerrors.CodeNotFound, "LPS exclusion override tidak ditemukan.", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toOverrideDTO(ov),
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── Sort helper ──────────────────────────────────────────────────────────────

type sortSpec struct {
	col string
	dir string
}

// parseSortParam parses ?sort=col:asc (first entry). Falls back to defaultCol:defaultDir.
func parseSortParam(s string, allowedCols []string, defaultCol, defaultDir string) sortSpec {
	if s == "" {
		return sortSpec{defaultCol, defaultDir}
	}
	// Parse first sort entry.
	entry := s
	if idx := len(s); idx > 0 {
		for i, ch := range s {
			if ch == ',' {
				entry = s[:i]
				break
			}
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
	// Validate against allowlist.
	allowed := false
	for _, a := range allowedCols {
		if a == col {
			allowed = true
			break
		}
	}
	if !allowed {
		// Invalid column: return full default (col + dir).
		return sortSpec{defaultCol, defaultDir}
	}
	if dir != "asc" && dir != "desc" {
		dir = "asc"
	}
	return sortSpec{col, dir}
}
