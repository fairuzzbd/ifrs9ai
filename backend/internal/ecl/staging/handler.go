// Package staging — HTTP handler layer for ECL Staging Engine (APP-C-STG-001..005).
//
// 11 endpoints per api/openapi/app-c-staging.yaml:
//
//	POST   /ecl/staging/evaluate                       → EvaluateHandler (202)
//	GET    /ecl/staging/instrumen/{id}                 → GetCurrentStageHandler
//	GET    /ecl/staging/instrumen/{id}/history         → GetHistoryHandler
//	POST   /ecl/staging/override/submit                → SubmitOverrideHandler
//	POST   /ecl/staging/override/{id}/review           → ReviewOverrideHandler
//	POST   /ecl/staging/override/{id}/approve          → ApproveALCOHandler
//	POST   /ecl/staging/override/{id}/approve2         → ApproveKomiteHandler
//	POST   /ecl/staging/override/{id}/reject           → RejectOverrideHandler
//	GET    /ecl/staging/overrides                      → ListOverridesHandler
//	POST   /ecl/dpd/record                             → RecordDPDHandler
//	GET    /ecl/dpd/instrumen/{id}                     → GetDPDHistoryHandler
//
// All POST endpoints require Idempotency-Key header (enforced by middleware, DEC-021).
// Handlers contain no business logic; they only parse → delegate → serialize.
package staging

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler is the HTTP handler for staging endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ─── POST /ecl/staging/evaluate ──────────────────────────────────────────────

// EvaluateHandler handles POST /api/v1/ecl/staging/evaluate.
// Returns 202 Accepted — triggers SICR evaluation for submitted instrument IDs.
// Idempotency-Key enforced by middleware.
func (h *Handler) EvaluateHandler(c *gin.Context) {
	var req EvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	// For batch evaluation, submit Asynq tasks per instrument.
	// For single-instrument debug path, InstrumenIDs can contain one item.
	if len(req.InstrumenIDs) == 0 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"instrumenIds wajib diisi (minimal 1, maksimal 500)"))
		return
	}
	if len(req.InstrumenIDs) > 500 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"instrumenIds maksimal 500 per request"))
		return
	}

	tanggal := time.Now().UTC()
	results := make([]*EvaluationResult, 0, len(req.InstrumenIDs))
	for _, id := range req.InstrumenIDs {
		result, err := h.svc.EvaluateSingleInstrumen(c.Request.Context(), id, tanggal, nil)
		if err != nil {
			response.Error(c, err)
			return
		}
		results = append(results, result)
	}

	response.Accepted(c, gin.H{"results": results, "count": len(results)})
}

// ─── GET /ecl/staging/instrumen/{id} ─────────────────────────────────────────

// GetCurrentStageHandler handles GET /api/v1/ecl/staging/instrumen/{id}.
func (h *Handler) GetCurrentStageHandler(c *gin.Context) {
	instrumenID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	status, err := h.svc.GetCurrentStage(c.Request.Context(), instrumenID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, status)
}

// ─── GET /ecl/staging/instrumen/{id}/history ─────────────────────────────────

// GetHistoryHandler handles GET /api/v1/ecl/staging/instrumen/{id}/history.
func (h *Handler) GetHistoryHandler(c *gin.Context) {
	instrumenID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	q, err := listquery.ParseFromRequest(c.Request, AllAllowedColsHistory)
	if err != nil {
		response.Error(c, err)
		return
	}
	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	items, pag, err := h.svc.GetHistory(c.Request.Context(), instrumenID, q, pagParams.Cursor, pagParams.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, items, toPaginationMeta(pag), toSortApplied(q), q.AppliedFilter())
}

// ─── POST /ecl/staging/override/submit ───────────────────────────────────────

// SubmitOverrideHandler handles POST /api/v1/ecl/staging/override/submit.
func (h *Handler) SubmitOverrideHandler(c *gin.Context) {
	var req OverrideSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	if req.InstrumenID == uuid.Nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "instrumen_id wajib diisi"))
		return
	}
	if !req.StageTarget.IsValid() {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "stage_target harus STAGE_1, STAGE_2, atau STAGE_3"))
		return
	}
	if req.Alasan == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "alasan wajib diisi"))
		return
	}

	prop, err := h.svc.SubmitOverride(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, prop)
}

// ─── POST /ecl/staging/override/{id}/review ──────────────────────────────────

// ReviewOverrideHandler handles POST /api/v1/ecl/staging/override/{id}/review.
func (h *Handler) ReviewOverrideHandler(c *gin.Context) {
	propID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req WorkflowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	prop, err := h.svc.ReviewOverride(c.Request.Context(), propID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, prop)
}

// ─── POST /ecl/staging/override/{id}/approve ─────────────────────────────────

// ApproveALCOHandler handles POST /api/v1/ecl/staging/override/{id}/approve.
// Requires step-up MFA (DEC-027). Enforced in service.
func (h *Handler) ApproveALCOHandler(c *gin.Context) {
	propID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req WorkflowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	prop, err := h.svc.ApproveALCO(c.Request.Context(), propID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, prop)
}

// ─── POST /ecl/staging/override/{id}/approve2 ────────────────────────────────

// ApproveKomiteHandler handles POST /api/v1/ecl/staging/override/{id}/approve2.
// 6-eyes second approval (KOMITE). Requires step-up MFA (DEC-027).
func (h *Handler) ApproveKomiteHandler(c *gin.Context) {
	propID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req WorkflowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	prop, err := h.svc.ApproveKomite(c.Request.Context(), propID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, prop)
}

// ─── POST /ecl/staging/override/{id}/reject ──────────────────────────────────

// RejectOverrideHandler handles POST /api/v1/ecl/staging/override/{id}/reject.
func (h *Handler) RejectOverrideHandler(c *gin.Context) {
	propID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req WorkflowRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	if req.Comment == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "comment wajib diisi"))
		return
	}

	prop, err := h.svc.RejectOverride(c.Request.Context(), propID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, prop)
}

// ─── GET /ecl/staging/overrides ──────────────────────────────────────────────

// ListOverridesHandler handles GET /api/v1/ecl/staging/overrides.
func (h *Handler) ListOverridesHandler(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedColsOverride)
	if err != nil {
		response.Error(c, err)
		return
	}
	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	items, pag, err := h.svc.ListOverrides(c.Request.Context(), q, pagParams.Cursor, pagParams.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, items, toPaginationMeta(pag), toSortApplied(q), q.AppliedFilter())
}

// ─── POST /ecl/dpd/record ────────────────────────────────────────────────────

// RecordDPDHandler handles POST /api/v1/ecl/dpd/record.
// Idempotency-Key enforced by middleware.
func (h *Handler) RecordDPDHandler(c *gin.Context) {
	var req DPDRecordCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	if req.InstrumenID == uuid.Nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "instrumen_id wajib diisi"))
		return
	}
	if req.DPDValue < 0 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "dpd_value tidak boleh negatif"))
		return
	}
	if req.Source == "" {
		req.Source = "MANUAL"
	}
	if req.Source != "MANUAL" && req.Source != "APP_B" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"source harus 'MANUAL' atau 'APP_B'"))
		return
	}

	// Parse periode string to time.Time.
	periode, err := time.Parse("2006-01-02", req.Periode)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"periode harus berformat YYYY-MM-DD (mis. 2026-06-01)"))
		return
	}

	rec, err := h.svc.RecordDPD(c.Request.Context(), req.InstrumenID, periode, req.DPDValue, req.Source, req.Catatan)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": rec})
}

// ─── GET /ecl/dpd/instrumen/{id} ─────────────────────────────────────────────

// GetDPDHistoryHandler handles GET /api/v1/ecl/dpd/instrumen/{id}.
func (h *Handler) GetDPDHistoryHandler(c *gin.Context) {
	instrumenID, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	q, err := listquery.ParseFromRequest(c.Request, AllAllowedColsDPD)
	if err != nil {
		response.Error(c, err)
		return
	}
	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	items, pag, err := h.svc.GetDPDHistory(c.Request.Context(), instrumenID, q, pagParams.Cursor, pagParams.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, items, toPaginationMeta(pag), toSortApplied(q), q.AppliedFilter())
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseIDParam parses the ":id" route parameter as a UUID.
func parseIDParam(c *gin.Context) (uuid.UUID, error) {
	s := c.Param("id")
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter 'id' harus berformat UUID")
	}
	return id, nil
}

func toPaginationMeta(p pagination.Result) *response.PaginationMeta {
	return &response.PaginationMeta{
		HasMore:       p.HasMore,
		NextCursor:    p.NextCursor,
		TotalEstimate: p.TotalEstimate,
		Limit:         p.Limit,
	}
}

func toSortApplied(q listquery.Query) []response.SortApplied {
	sorts := make([]response.SortApplied, 0, len(q.Sort))
	for _, s := range q.Sort {
		sorts = append(sorts, response.SortApplied{Col: s.Col, Dir: s.Dir})
	}
	return sorts
}
