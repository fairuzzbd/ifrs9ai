// Package helpers — Gin HTTP handler for all 10 ECL helper paths.
//
// Paths implemented (api/openapi/app-c-helpers.yaml):
//
//	GET  /ecl/helpers/pd            — single PD lookup  (APP-C-PAR-001)
//	POST /ecl/helpers/pd/bulk       — batch PD          (APP-C-PAR-001)
//	GET  /ecl/helpers/lgd           — single LGD        (APP-C-PAR-002)
//	POST /ecl/helpers/lgd/bulk      — batch LGD         (APP-C-PAR-002)
//	GET  /ecl/helpers/ead           — single EAD        (APP-C-PAR-003)
//	POST /ecl/helpers/ead/bulk      — batch EAD         (APP-C-PAR-003)
//	GET  /ecl/helpers/ccf           — single CCF        (APP-C-PAR-004)
//	GET  /ecl/helpers/preview       — preview list      (APP-C-PAR-005)
//	GET  /ecl/helpers/preview/export — async export     (APP-C-PAR-005)
//	POST /ecl/helpers/bulk-lookup   — combined bulk     (APP-C-PAR-006)
//
// All endpoints: READ-ONLY. No Idempotency-Key required.
// Permission: ecl_helpers.read (stories 1–4, 6) or ecl_helpers.preview (story 5).
// Audit: only bulk-lookup and preview/export are audited.
package helpers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds all helper services.
type Handler struct {
	svc         *Services
	previewRepo previewInstrumentLister // optional; nil in tests without DB
}

// NewHandler creates a Handler.
func NewHandler(svc *Services) *Handler {
	h := &Handler{svc: svc}
	// Wire preview repo if the instrRepo also implements ListECLApplicableInstruments.
	if p, ok := svc.previewRepoFromInstrRepo(); ok {
		h.previewRepo = p
	}
	return h
}

// hasPermission checks whether the current JWT (from gin context) has a given permission.
// Returns false and writes 403 if not.
func hasPermission(c *gin.Context, perm string) bool {
	permsRaw, exists := c.Get("permissions")
	if !exists {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			fmt.Sprintf("Permission '%s' diperlukan.", perm),
			nil)
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
		fmt.Sprintf("Permission '%s' diperlukan. Role Anda tidak memiliki akses.", perm),
		nil)
	return false
}

// traceID returns the trace ID from gin context.
func traceID(c *gin.Context) string {
	if v, ok := c.Get(response.TraceIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Trace-Id")
}

// ── GET /ecl/helpers/pd ───────────────────────────────────────────────────────

// GetPD handles single PD lookup.
// Params: instrumenId, stage, scenario, evaluationDate (YYYY-MM-DD), periodeId.
func (h *Handler) GetPD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	instrID, ok := parseUUIDParam(c, "instrumenId")
	if !ok {
		return
	}

	stageStr := c.Query("stage")
	stage, ok := parseStage(c, stageStr)
	if !ok {
		return
	}

	scenarioStr := c.Query("scenario")
	scenario, ok := parseScenario(c, scenarioStr)
	if !ok {
		return
	}

	evalDate, ok := parseDateParam(c, "evaluationDate")
	if !ok {
		return
	}

	periodeID := c.Query("periodeId")
	if periodeID == "" {
		validationFailed(c, "periodeId", "required", "periodeId wajib diisi")
		return
	}

	pd, detail, err := h.svc.PD.GetPD(c.Request.Context(), instrID, stage, scenario, periodeID, evalDate)
	if err != nil {
		response.Error(c, err)
		return
	}
	_ = pd // detail.PD is the same value, used in buildPDResult

	response.OK(c, buildPDResult(instrID, stage, scenario, detail))
}

// ── POST /ecl/helpers/pd/bulk ─────────────────────────────────────────────────

type bulkPDRequest struct {
	EvaluationDate string `json:"evaluationDate" binding:"required"`
	PeriodeID      string `json:"periodeId"      binding:"required"`
	Items          []struct {
		InstrumenID string `json:"instrumenId" binding:"required"`
		Stage       string `json:"stage"       binding:"required"`
		Scenario    string `json:"scenario"    binding:"required"`
	} `json:"items" binding:"required"`
}

// BulkGetPD handles batch PD lookup.
func (h *Handler) BulkGetPD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	var req bulkPDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFailed(c, "body", "json_parse", err.Error())
		return
	}

	if len(req.Items) > maxBulkItems {
		response.ErrorWithStatus(c, http.StatusRequestEntityTooLarge,
			domainerrors.CodeHelpersBulkTooLarge,
			fmt.Sprintf("Request melebihi batas %d instrumen per batch.", maxBulkItems),
			nil)
		return
	}

	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		validationFailed(c, "evaluationDate", "date_format", "Format harus YYYY-MM-DD")
		return
	}

	type pdResultItem struct {
		InstrumenID string      `json:"instrumenId"`
		Stage       string      `json:"stage"`
		Scenario    string      `json:"scenario"`
		Detail      interface{} `json:"detail,omitempty"`
		Error       *string     `json:"error,omitempty"`
	}

	results := make([]pdResultItem, 0, len(req.Items))
	for _, item := range req.Items {
		instrID, err := uuid.Parse(item.InstrumenID)
		if err != nil {
			errMsg := "invalid instrumenId UUID"
			results = append(results, pdResultItem{
				InstrumenID: item.InstrumenID, Stage: item.Stage, Scenario: item.Scenario, Error: &errMsg,
			})
			continue
		}

		stage, ok := EclStageFromString(item.Stage)
		if !ok {
			errMsg := fmt.Sprintf("stage tidak valid: %s", item.Stage)
			results = append(results, pdResultItem{
				InstrumenID: item.InstrumenID, Stage: item.Stage, Scenario: item.Scenario, Error: &errMsg,
			})
			continue
		}

		scenario, ok := EclScenarioFromString(item.Scenario)
		if !ok {
			errMsg := fmt.Sprintf("scenario tidak valid: %s", item.Scenario)
			results = append(results, pdResultItem{
				InstrumenID: item.InstrumenID, Stage: item.Stage, Scenario: item.Scenario, Error: &errMsg,
			})
			continue
		}

		_, detail, err := h.svc.PD.GetPD(c.Request.Context(), instrID, stage, scenario, req.PeriodeID, evalDate)
		if err != nil {
			errMsg := domainErrMsg(err)
			results = append(results, pdResultItem{
				InstrumenID: item.InstrumenID, Stage: item.Stage, Scenario: item.Scenario, Error: &errMsg,
			})
			continue
		}

		results = append(results, pdResultItem{
			InstrumenID: item.InstrumenID,
			Stage:       item.Stage,
			Scenario:    item.Scenario,
			Detail:      buildPDResult(instrID, stage, scenario, detail),
		})
	}

	response.OK(c, gin.H{
		"results":   results,
		"periodeId": req.PeriodeID,
		"total":     len(results),
	})
}

// ── GET /ecl/helpers/lgd ─────────────────────────────────────────────────────

// GetLGD handles single LGD lookup.
// Params: instrumenId, periodeId.
func (h *Handler) GetLGD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	instrID, ok := parseUUIDParam(c, "instrumenId")
	if !ok {
		return
	}

	periodeID := c.Query("periodeId")
	if periodeID == "" {
		validationFailed(c, "periodeId", "required", "periodeId wajib diisi")
		return
	}

	lgd, detail, err := h.svc.LGD.GetLGD(c.Request.Context(), instrID, periodeID)
	if err != nil {
		response.Error(c, err)
		return
	}
	_ = lgd // detail.LGD is the same value

	response.OK(c, gin.H{
		"instrumenId":       instrID,
		"lgd":               detail.LGD.StringFixed(8),
		"baseLGD":           detail.BaseLGD.StringFixed(8),
		"collateralHaircut": detail.CollateralHaircut.StringFixed(8),
		"lgdEffective":      detail.LGDEffective.StringFixed(8),
		"poolUsed":          detail.PoolUsed,
		"tipeCounterparty":  detail.TipeCounterparty,
		"warnings":          detail.Warnings,
	})
}

// ── POST /ecl/helpers/lgd/bulk ────────────────────────────────────────────────

type bulkLGDRequest struct {
	PeriodeID string `json:"periodeId" binding:"required"`
	Items     []struct {
		InstrumenID string `json:"instrumenId" binding:"required"`
	} `json:"items" binding:"required"`
}

// BulkGetLGD handles batch LGD lookup.
func (h *Handler) BulkGetLGD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	var req bulkLGDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFailed(c, "body", "json_parse", err.Error())
		return
	}

	if len(req.Items) > maxBulkItems {
		response.ErrorWithStatus(c, http.StatusRequestEntityTooLarge,
			domainerrors.CodeHelpersBulkTooLarge,
			fmt.Sprintf("Request melebihi batas %d instrumen per batch.", maxBulkItems),
			nil)
		return
	}

	type lgdItem struct {
		InstrumenID string  `json:"instrumenId"`
		LGD         string  `json:"lgd,omitempty"`
		PoolUsed    string  `json:"poolUsed,omitempty"`
		Error       *string `json:"error,omitempty"`
	}

	results := make([]lgdItem, 0, len(req.Items))
	for _, item := range req.Items {
		instrID, err := uuid.Parse(item.InstrumenID)
		if err != nil {
			errMsg := "invalid instrumenId UUID"
			results = append(results, lgdItem{InstrumenID: item.InstrumenID, Error: &errMsg})
			continue
		}

		lgd, detail, err := h.svc.LGD.GetLGD(c.Request.Context(), instrID, req.PeriodeID)
		if err != nil {
			errMsg := domainErrMsg(err)
			results = append(results, lgdItem{InstrumenID: item.InstrumenID, Error: &errMsg})
			continue
		}
		_ = lgd

		results = append(results, lgdItem{
			InstrumenID: item.InstrumenID,
			LGD:         detail.LGD.StringFixed(8),
			PoolUsed:    detail.PoolUsed,
		})
	}

	response.OK(c, gin.H{"results": results, "periodeId": req.PeriodeID, "total": len(results)})
}

// ── GET /ecl/helpers/ead ─────────────────────────────────────────────────────

// GetEAD handles single EAD computation.
// Params: instrumenId, evaluationDate (YYYY-MM-DD).
func (h *Handler) GetEAD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	instrID, ok := parseUUIDParam(c, "instrumenId")
	if !ok {
		return
	}

	evalDate, ok := parseDateParam(c, "evaluationDate")
	if !ok {
		return
	}

	eadIDR, bd, err := h.svc.EAD.ComputeEAD(c.Request.Context(), instrID, evalDate)
	if err != nil {
		response.Error(c, err)
		return
	}

	fxRateStr := ""
	if bd.FXRate != nil {
		fxRateStr = bd.FXRate.StringFixed(8)
	}

	response.OK(c, gin.H{
		"instrumenId":             instrID,
		"eadIdr":                  eadIDR.StringFixed(4),
		"eadFcy":                  bd.EADFCY.StringFixed(4),
		"currency":                bd.Currency,
		"fxRate":                  fxRateStr,
		"fxSource":                bd.FXSource,
		"outstandingPrincipalFcy": bd.OutstandingPrincipalFCY.StringFixed(4),
		"accruedInterestFcy":      bd.AccruedInterestFCY.StringFixed(4),
		"committedUndrawnFcy":     bd.CommittedUndrawnFCY.StringFixed(4),
		"ccf":                     bd.CCF.StringFixed(4),
		"accruedInterestSource":   bd.AccruedInterestSource,
		"warnings":                bd.Warnings,
	})
}

// ── POST /ecl/helpers/ead/bulk ────────────────────────────────────────────────

type bulkEADRequest struct {
	EvaluationDate string `json:"evaluationDate" binding:"required"`
	Items          []struct {
		InstrumenID string `json:"instrumenId" binding:"required"`
	} `json:"items" binding:"required"`
}

// BulkGetEAD handles batch EAD computation.
func (h *Handler) BulkGetEAD(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	var req bulkEADRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFailed(c, "body", "json_parse", err.Error())
		return
	}

	if len(req.Items) > maxBulkItems {
		response.ErrorWithStatus(c, http.StatusRequestEntityTooLarge,
			domainerrors.CodeHelpersBulkTooLarge,
			fmt.Sprintf("Request melebihi batas %d instrumen per batch.", maxBulkItems),
			nil)
		return
	}

	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		validationFailed(c, "evaluationDate", "date_format", "Format harus YYYY-MM-DD")
		return
	}

	type eadItem struct {
		InstrumenID string  `json:"instrumenId"`
		EADIDR      string  `json:"eadIdr,omitempty"`
		Currency    string  `json:"currency,omitempty"`
		Error       *string `json:"error,omitempty"`
	}

	results := make([]eadItem, 0, len(req.Items))
	for _, item := range req.Items {
		instrID, err := uuid.Parse(item.InstrumenID)
		if err != nil {
			errMsg := "invalid instrumenId UUID"
			results = append(results, eadItem{InstrumenID: item.InstrumenID, Error: &errMsg})
			continue
		}

		eadIDR, bd, err := h.svc.EAD.ComputeEAD(c.Request.Context(), instrID, evalDate)
		if err != nil {
			errMsg := domainErrMsg(err)
			results = append(results, eadItem{InstrumenID: item.InstrumenID, Error: &errMsg})
			continue
		}

		results = append(results, eadItem{
			InstrumenID: item.InstrumenID,
			EADIDR:      eadIDR.StringFixed(4),
			Currency:    bd.Currency,
		})
	}

	response.OK(c, gin.H{"results": results, "total": len(results)})
}

// ── GET /ecl/helpers/ccf ─────────────────────────────────────────────────────

// GetCCF handles single CCF lookup.
// Params: tipeInstrumen.
func (h *Handler) GetCCF(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	tipe := c.Query("tipeInstrumen")
	if tipe == "" {
		validationFailed(c, "tipeInstrumen", "required", "tipeInstrumen wajib diisi")
		return
	}

	ccf, detail, err := h.svc.CCF.GetCCF(c.Request.Context(), tipe)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, gin.H{
		"tipeInstrumen": tipe,
		"ccf":           ccf.StringFixed(4),
		"source":        detail.Source,
		"warnings":      detail.Warnings,
	})
}

// ── GET /ecl/helpers/preview ─────────────────────────────────────────────────

// GetPreview returns a paginated preview of PD+LGD+EAD+CCF per active instrument.
// Params: periodeId, evaluationDate, cursor, limit.
// Permission: ecl_helpers.preview.
func (h *Handler) GetPreview(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersPreview) {
		return
	}

	periodeID := c.Query("periodeId")
	if periodeID == "" {
		validationFailed(c, "periodeId", "required", "periodeId wajib diisi")
		return
	}

	evalDate, ok := parseDateParam(c, "evaluationDate")
	if !ok {
		return
	}

	limit, _ := parseLimitParam(c, 50)
	cursor := c.Query("cursor")

	// Preview uses the previewRepo (nil in tests without DB).
	if h.previewRepo == nil {
		nc := ""
		response.List(c, []interface{}{},
			&response.PaginationMeta{NextCursor: &nc, HasMore: false, Limit: limit},
			nil, nil)
		return
	}

	// Query optional filter/sort params.
	filterStage := c.Query("filter[stage]")
	filterTipe := c.Query("filter[tipe]")
	filterKlasifikasi := c.Query("filter[klasifikasi]")
	filterMatauang := c.Query("filter[matauang]")
	sortCol := c.DefaultQuery("sortCol", "kode_instrumen")
	sortDir := c.DefaultQuery("sortDir", "asc")

	instrRows, nextCursor, hasMore, err := h.previewRepo.ListECLApplicableInstruments(
		c.Request.Context(),
		periodeID, filterStage, filterTipe, filterKlasifikasi, filterMatauang,
		nil, "", sortCol, sortDir, cursor, limit,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	if len(instrRows) == 0 {
		nc := nextCursor
		response.List(c, []interface{}{},
			&response.PaginationMeta{NextCursor: &nc, HasMore: hasMore, Limit: limit},
			nil, nil)
		return
	}

	reqs := make([]BulkRequest, len(instrRows))
	for i := range instrRows {
		row := &instrRows[i]
		reqs[i] = BulkRequest{InstrumenID: row.ID}
	}

	totalEst := len(instrRows) // estimate; real count from DB in production
	nextCursorStr := nextCursor

	results, summary, bulkErrs, _, err := h.svc.Bulk.BulkLookup(c.Request.Context(), reqs, periodeID, evalDate)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]gin.H, 0, len(results)+len(bulkErrs))
	for i := range results {
		r := &results[i]
		items = append(items, gin.H{
			"instrumenId": r.InstrumenID,
			"pdGood":      r.PDGood.StringFixed(8),
			"pdNormal":    r.PDNormal.StringFixed(8),
			"pdBad":       r.PDBad.StringFixed(8),
			"lgd":         r.LGD.StringFixed(8),
			"eadIdr":      r.EADIDR.StringFixed(4),
			"ccf":         r.CCF.StringFixed(4),
			"warnings":    r.Warnings,
		})
	}
	for _, be := range bulkErrs {
		items = append(items, gin.H{
			"instrumenId": be.InstrumenID,
			"error":       gin.H{"code": be.ErrorCode, "message": be.Message},
		})
	}

	te := int64(totalEst)
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"pagination": &response.PaginationMeta{
			NextCursor:    &nextCursorStr,
			HasMore:       nextCursorStr != "",
			TotalEstimate: &te,
			Limit:         limit,
		},
		"summary": gin.H{
			"total":       summary.Total,
			"success":     summary.Success,
			"warning":     summary.Warning,
			"executionMs": summary.ExecutionMs,
		},
		"meta": response.Meta{TraceID: traceID(c)},
	})
}

// ── GET /ecl/helpers/preview/export ──────────────────────────────────────────

// ExportPreview submits an async export job (202 Accepted).
// Permission: ecl_helpers.preview.
// Audit: ECL_PARAM.PREVIEW_EXPORT — wired in P4-M8 Asynq job.
func (h *Handler) ExportPreview(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersPreview) {
		return
	}

	periodeID := c.Query("periodeId")
	if periodeID == "" {
		validationFailed(c, "periodeId", "required", "periodeId wajib diisi")
		return
	}

	evalDateStr := c.Query("evaluationDate")
	if evalDateStr == "" {
		validationFailed(c, "evaluationDate", "required", "evaluationDate wajib diisi")
		return
	}

	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "xlsx" {
		validationFailed(c, "format", "enum", "format harus csv atau xlsx")
		return
	}

	// Phase 1 stub — actual Asynq job wiring in P4-M8.
	jobID := fmt.Sprintf("preview-export-%s-%s-%s", periodeID, evalDateStr, format)

	response.Accepted(c, gin.H{
		"jobId":     jobID,
		"type":      "ECL_HELPERS_PREVIEW_EXPORT",
		"statusUrl": fmt.Sprintf("/api/v1/jobs/%s", jobID),
		"streamUrl": fmt.Sprintf("/api/v1/jobs/%s/stream", jobID),
		"periodeId": periodeID,
		"evalDate":  evalDateStr,
		"format":    format,
	})
}

// ── POST /ecl/helpers/bulk-lookup ─────────────────────────────────────────────

type bulkLookupRequest struct {
	PeriodeID      string        `json:"periodeId"      binding:"required"`
	EvaluationDate string        `json:"evaluationDate" binding:"required"`
	Items          []BulkRequest `json:"items"          binding:"required"`
}

// BulkLookup handles combined PD+LGD+EAD+CCF for all instruments.
func (h *Handler) BulkLookup(c *gin.Context) {
	if !hasPermission(c, PermECLHelpersRead) {
		return
	}

	var req bulkLookupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFailed(c, "body", "json_parse", err.Error())
		return
	}

	if len(req.Items) > maxBulkItems {
		response.ErrorWithStatus(c, http.StatusRequestEntityTooLarge,
			domainerrors.CodeHelpersBulkTooLarge,
			fmt.Sprintf("Request melebihi batas %d instrumen per batch.", maxBulkItems),
			nil)
		return
	}

	evalDate, err := time.Parse("2006-01-02", req.EvaluationDate)
	if err != nil {
		validationFailed(c, "evaluationDate", "date_format", "Format harus YYYY-MM-DD")
		return
	}

	results, summary, bulkErrs, skipped, err := h.svc.Bulk.BulkLookup(
		c.Request.Context(), req.Items, req.PeriodeID, evalDate,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]gin.H, 0, len(results))
	for i := range results {
		r := &results[i]
		items = append(items, gin.H{
			"instrumenId": r.InstrumenID,
			"pdGood":      r.PDGood.StringFixed(8),
			"pdNormal":    r.PDNormal.StringFixed(8),
			"pdBad":       r.PDBad.StringFixed(8),
			"lgd":         r.LGD.StringFixed(8),
			"eadIdr":      r.EADIDR.StringFixed(4),
			"ccf":         r.CCF.StringFixed(4),
			"warnings":    r.Warnings,
		})
	}

	errItems := make([]gin.H, 0, len(bulkErrs))
	for _, be := range bulkErrs {
		errItems = append(errItems, gin.H{
			"instrumenId": be.InstrumenID,
			"errorCode":   be.ErrorCode,
			"message":     be.Message,
		})
	}

	skipItems := make([]gin.H, 0, len(skipped))
	for _, sk := range skipped {
		skipItems = append(skipItems, gin.H{
			"instrumenId":       sk.InstrumenID,
			"reason":            sk.Reason,
			"klasifikasiPsak71": sk.KlasifikasiPsak71,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"results": items,
			"errors":  errItems,
			"skipped": skipItems,
		},
		"summary": gin.H{
			"total":       summary.Total,
			"success":     summary.Success,
			"warning":     summary.Warning,
			"skipped":     summary.Skipped,
			"executionMs": summary.ExecutionMs,
		},
		"meta": response.Meta{TraceID: traceID(c)},
	})
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// previewInstrumentLister is a narrower interface used by GetPreview
// to page through ECL-applicable instruments.
// Implemented by DBInstrumenSnapshotRepo.
type previewInstrumentLister interface {
	ListECLApplicableInstruments(
		ctx context.Context,
		periodeID, filterStage, filterTipe, filterKlasifikasi, filterMatauang string,
		filterHasWarning *bool, search, sortCol, sortDir, cursor string, limit int,
	) ([]InstrumenRow, string, bool, error)
}

func parseUUIDParam(c *gin.Context, param string) (uuid.UUID, bool) {
	s := c.Query(param)
	if s == "" {
		validationFailed(c, param, "required", param+" wajib diisi")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		validationFailed(c, param, "uuid_format", param+" harus berupa UUID yang valid")
		return uuid.Nil, false
	}
	return id, true
}

func parseDateParam(c *gin.Context, param string) (time.Time, bool) {
	s := c.Query(param)
	if s == "" {
		validationFailed(c, param, "required", param+" wajib diisi")
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		validationFailed(c, param, "date_format", param+" format harus YYYY-MM-DD")
		return time.Time{}, false
	}
	return t, true
}

func parseLimitParam(c *gin.Context, defaultVal int) (int, bool) {
	s := c.DefaultQuery("limit", fmt.Sprintf("%d", defaultVal))
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil || v < 1 || v > 200 {
		v = defaultVal
	}
	return v, true
}

func parseStage(c *gin.Context, s string) (EclStage, bool) {
	if s == "" {
		validationFailed(c, "stage", "required", "stage wajib diisi")
		return "", false
	}
	stage, ok := EclStageFromString(s)
	if !ok {
		validationFailed(c, "stage", "enum", "stage harus STAGE_1, STAGE_2, atau STAGE_3")
		return "", false
	}
	return stage, true
}

func parseScenario(c *gin.Context, s string) (EclScenario, bool) {
	if s == "" {
		validationFailed(c, "scenario", "required", "scenario wajib diisi")
		return "", false
	}
	sc, ok := EclScenarioFromString(s)
	if !ok {
		validationFailed(c, "scenario", "enum", "scenario harus GOOD, NORMAL, atau BAD")
		return "", false
	}
	return sc, true
}

func validationFailed(c *gin.Context, field, rule, msg string) {
	response.ErrorWithStatus(c, http.StatusBadRequest,
		domainerrors.CodeValidationFailed,
		"Validasi gagal: "+msg,
		[]domainerrors.Detail{{Field: field, Rule: rule, Message: msg}},
	)
}

func domainErrMsg(err error) string {
	if de, ok := domainerrors.IsDomainError(err); ok {
		return de.Message()
	}
	return err.Error()
}

func buildPDResult(instrID uuid.UUID, stage EclStage, scenario EclScenario, detail PDDetail) gin.H {
	r := gin.H{
		"instrumenId":               instrID,
		"stage":                     stage.String(),
		"scenario":                  scenario.String(),
		"pd":                        detail.PD.StringFixed(8),
		"pdBase":                    detail.PDBase.StringFixed(8),
		"ratingUsed":                detail.RatingUsed,
		"impactPdMultiplier":        detail.ImpactPDMultiplier.StringFixed(8),
		"impactMevPdMultiplier":     detail.ImpactMevPDMultiplier.StringFixed(8),
		"normalMultiplierIsDefault": detail.NormalMultiplierIsDefault,
		"warnings":                  detail.Warnings,
	}
	if detail.TenorMonthsUsed != nil {
		r["tenorMonthsUsed"] = *detail.TenorMonthsUsed
	} else {
		r["tenorMonthsUsed"] = nil
	}
	if detail.SourcePD12M != nil {
		r["sourcePd12m"] = detail.SourcePD12M.StringFixed(8)
	} else {
		r["sourcePd12m"] = nil
	}
	if detail.SourcePDLifetime != nil {
		r["sourcePdLifetime"] = detail.SourcePDLifetime.StringFixed(8)
	} else {
		r["sourcePdLifetime"] = nil
	}
	return r
}
