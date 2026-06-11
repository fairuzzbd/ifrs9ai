// Package staging — HTTP handler layer for ECL Staging Engine (APP-C-STG-001..005).
//
// 12 endpoints per api/openapi/app-c-staging.yaml:
//
//	POST   /ecl/staging/evaluate                       → EvaluateHandler (202 + Asynq)
//	POST   /ecl/staging/evaluate/sync                  → EvaluateSyncHandler (200, debug, 1 ID, ROLE-RISK)
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
//
// F5 (UX §3): EvaluateHandler dispatches to Asynq and returns 202 + jobId.
// Operations > 2 seconds must NOT block the HTTP thread.
package staging

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// TaskEnqueuer is the minimal interface from asynq.Client used by EvaluateHandler.
// Defined as an interface so tests can inject a no-op implementation.
type TaskEnqueuer interface {
	EnqueueContext(ctx interface{}, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Handler is the HTTP handler for staging endpoints.
type Handler struct {
	svc          *Service
	taskEnqueuer TaskEnqueuer // nil = sync fallback (dev / test)
}

// NewHandler creates a Handler.
// Pass a non-nil asynq.Client for production use; pass nil for dev/test to fall back to
// the synchronous evaluate path (EvaluateSyncHandler behaviour).
func NewHandler(svc *Service, opts ...TaskEnqueuer) *Handler {
	h := &Handler{svc: svc}
	if len(opts) > 0 {
		h.taskEnqueuer = opts[0]
	}
	return h
}

// ─── POST /ecl/staging/evaluate ──────────────────────────────────────────────

// EvaluateHandler handles POST /api/v1/ecl/staging/evaluate.
//
// Returns 202 Accepted per UX §3 (long-running process).
// Each instrument ID is dispatched as an individual Asynq task
// (TaskTypeEvaluateStaging) against the default queue.
//
// When taskEnqueuer is nil (dev / test mode) the handler falls back to
// calling EvaluateSingleInstrumen synchronously and still returns 202.
//
// Idempotency-Key enforced by middleware (DEC-021).
func (h *Handler) EvaluateHandler(c *gin.Context) {
	var req EvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

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

	claims := auth.ClaimsFromContext(c.Request.Context())
	actorSub := ""
	actorRole := ""
	tenantID := "TUGURE"
	if claims != nil {
		actorSub = claims.Sub
		if len(claims.Roles) > 0 {
			actorRole = claims.Roles[0]
		}
		if claims.TenantID != "" {
			tenantID = claims.TenantID
		}
	}

	jobID := uuid.New()
	tanggal := time.Now().UTC()

	if h.taskEnqueuer != nil {
		// Production path: enqueue each instrument as an Asynq task (UX §3).
		for _, instrumenID := range req.InstrumenIDs {
			jobIDCopy := jobID
			payload := EvaluateStagingPayload{
				InstrumenID:       instrumenID,
				TanggalAssessment: tanggal,
				TenantID:          tenantID,
				JobID:             &jobIDCopy,
				ActorSub:          actorSub,
				ActorRole:         actorRole,
			}
			task, err := NewEvaluateStagingTask(payload)
			if err != nil {
				response.Error(c, domainerrors.New(domainerrors.CodeInternal,
					fmt.Sprintf("gagal membuat task untuk instrumen %s: %v", instrumenID, err)))
				return
			}
			if _, err := h.taskEnqueuer.EnqueueContext(c.Request.Context(), task); err != nil {
				response.Error(c, domainerrors.New(domainerrors.CodeInternal,
					fmt.Sprintf("gagal enqueue task untuk instrumen %s: %v", instrumenID, err)))
				return
			}
		}
	} else {
		// Dev / test fallback: synchronous evaluation (still returns 202).
		for _, instrumenID := range req.InstrumenIDs {
			jobIDCopy := jobID
			if _, err := h.svc.EvaluateSingleInstrumen(c.Request.Context(), instrumenID, tanggal, &jobIDCopy); err != nil {
				response.Error(c, err)
				return
			}
		}
	}

	response.Accepted(c, gin.H{
		"jobId":     jobID.String(),
		"statusUrl": "/api/v1/jobs/" + jobID.String(),
		"count":     len(req.InstrumenIDs),
	})
}

// ─── POST /ecl/staging/evaluate/sync ─────────────────────────────────────────

// EvaluateSyncHandler handles POST /api/v1/ecl/staging/evaluate/sync.
//
// Debug endpoint reserved for ROLE-RISK single-instrument evaluation.
// Limit: 1 instrument ID; returns 200 with full EvaluationResult.
// This endpoint must NOT be used for batch operations (use /evaluate for that).
func (h *Handler) EvaluateSyncHandler(c *gin.Context) {
	var req EvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	if len(req.InstrumenIDs) == 0 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"instrumenIds wajib diisi (tepat 1 ID untuk endpoint sync)"))
		return
	}
	if len(req.InstrumenIDs) > 1 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"endpoint sync hanya mendukung 1 instrumen ID per request"))
		return
	}

	tanggal := time.Now().UTC()
	result, err := h.svc.EvaluateSingleInstrumen(c.Request.Context(), req.InstrumenIDs[0], tanggal, nil)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, result)
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
