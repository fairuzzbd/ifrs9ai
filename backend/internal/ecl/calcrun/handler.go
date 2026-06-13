package calcrun

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// handler.go — 10 Gin HTTP handlers for ECL Calc Run (P4-M8).
//
// Endpoint mapping (api/openapi/app-c-calc-run.yaml):
//
//	POST   /ecl/calc-runs                        createCalcRun       (M8-001)
//	GET    /ecl/calc-runs                        listCalcRuns        (M8-003)
//	GET    /ecl/calc-runs/:id                    getCalcRun          (M8-003)
//	POST   /ecl/calc-runs/:id/start              startCalcRun        (M8-002)
//	POST   /ecl/calc-runs/:id/cancel             cancelCalcRun       (M8-005)
//	GET    /ecl/calc-runs/:id/parameter-snapshot getSnapshot         (M8-002)
//	POST   /ecl/calc-runs/:id/seal/request       requestSeal         (M8-004)
//	POST   /ecl/calc-runs/:id/seal/approve       approveSeal         (M8-004)
//	POST   /ecl/calc-runs/:id/seal/reject        rejectSeal          (M8-004)
//	GET    /ecl/calc-runs/:id/result-lines       listResultLines     (M8-003)
//
// Auth: JWT via auth.Middleware (wired in routes.go).
// Idempotency-Key: required on POST mutating endpoints (middleware).
// Step-up MFA: enforced in-handler for seal/approve (DEC-027).
// No float64 in any response (DEC-016).

// Handler holds the calcrun service.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("calcrun.NewHandler: svc must not be nil")
	}
	return &Handler{svc: svc}
}

// ─── POST /ecl/calc-runs ─────────────────────────────────────────────────────

func (h *Handler) CreateCalcRun(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunCreate) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunCreate+" tidak terpenuhi.", nil)
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}
	if len(req.Comment) > 500 {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "comment maksimal 500 karakter.", nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID invalid", nil)
		return
	}

	run, err := h.svc.Create(c.Request.Context(), req, actorID)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.Created(c, run)
}

// ─── GET /ecl/calc-runs ──────────────────────────────────────────────────────

func (h *Handler) ListCalcRuns(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunRead+" tidak terpenuhi.", nil)
		return
	}

	periodeID := c.Query("periode_id")
	limit := 50
	cursor := c.Query("cursor")

	items, nextCursor, hasMore, err := h.svc.List(c.Request.Context(), periodeID, limit, cursor)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}

	var nc *string
	if nextCursor != "" {
		nc = &nextCursor
	}
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"pagination": gin.H{
			"nextCursor": nc,
			"hasMore":    hasMore,
			"limit":      limit,
		},
		"meta": gin.H{"traceId": c.GetString("X-Trace-Id")},
	})
}

// ─── GET /ecl/calc-runs/:id ──────────────────────────────────────────────────

func (h *Handler) GetCalcRun(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunRead+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	run, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.OK(c, run)
}

// ─── POST /ecl/calc-runs/:id/start ───────────────────────────────────────────

func (h *Handler) StartCalcRun(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunStart) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunStart+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "actor UUID dalam JWT tidak valid.", nil)
		return
	}
	resp, err := h.svc.Start(c.Request.Context(), id, actorID)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.Accepted(c, resp)
}

// ─── POST /ecl/calc-runs/:id/cancel ──────────────────────────────────────────

func (h *Handler) CancelCalcRun(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunCancel) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunCancel+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	var req CancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "actor UUID dalam JWT tidak valid.", nil)
		return
	}
	run, err := h.svc.Cancel(c.Request.Context(), id, req, actorID)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.OK(c, run)
}

// ─── GET /ecl/calc-runs/:id/parameter-snapshot ───────────────────────────────

func (h *Handler) GetParameterSnapshot(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunRead+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	raw, err := h.svc.GetParameterSnapshot(c.Request.Context(), id)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	if raw == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": nil,
			"meta": gin.H{"traceId": c.GetString("X-Trace-Id")},
		})
		return
	}
	// Unmarshal to avoid double-encoding.
	var snap any
	if err := json.Unmarshal(raw, &snap); err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "snapshot parse error", nil)
		return
	}
	response.OK(c, snap)
}

// ─── POST /ecl/calc-runs/:id/seal/request ────────────────────────────────────

func (h *Handler) RequestSeal(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunSealRequest) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunSealRequest+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	var req SealRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "actor UUID dalam JWT tidak valid.", nil)
		return
	}
	run, err := h.svc.RequestSeal(c.Request.Context(), id, req, actorID)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.OK(c, run)
}

// ─── POST /ecl/calc-runs/:id/seal/approve ────────────────────────────────────

func (h *Handler) ApproveSeal(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunSealApprove) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunSealApprove+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	var req SealApproveBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	// Step-up MFA check (DEC-027): token must be fresh (< 5 min).
	stepUpFresh := !claims.NeedsStepUp()
	// Also check X-Step-Up-Token header presence (required per OpenAPI contract).
	if c.GetHeader("X-Step-Up-Token") == "" {
		stepUpFresh = false
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "actor UUID dalam JWT tidak valid.", nil)
		return
	}
	run, err := h.svc.ApproveSeal(c.Request.Context(), id, req, actorID, stepUpFresh)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.OK(c, run)
}

// ─── POST /ecl/calc-runs/:id/seal/reject ─────────────────────────────────────

func (h *Handler) RejectSeal(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunSealApprove) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunSealApprove+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	var req SealRejectBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "actor UUID dalam JWT tidak valid.", nil)
		return
	}
	run, err := h.svc.RejectSeal(c.Request.Context(), id, req, actorID)
	if err != nil {
		writeCalcRunError(c, err)
		return
	}
	response.OK(c, run)
}

// ─── GET /ecl/calc-runs/:id/result-lines ─────────────────────────────────────

func (h *Handler) ListResultLines(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermCalcRunRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermCalcRunRead+" tidak terpenuhi.", nil)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	// Delegate to a simple list via ecl.calc_result_line table.
	// Full DataTable (sort/filter/export) is wired through M7 handler.
	// This endpoint returns basic info linking to M7.
	response.OK(c, gin.H{
		"calcRunId": id,
		"message":   "Gunakan GET /api/v1/ecl/results/:calcRunId untuk detail result lines (M7 endpoint).",
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// claimsFromCtx extracts JWT claims from Gin context.
// Returns nil and writes 401 if not present.
func claimsFromCtx(c *gin.Context) *auth.Claims {
	val, ok := c.Get("claims")
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims tidak ditemukan.", nil)
		return nil
	}
	claims, ok := val.(*auth.Claims)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims format tidak valid.", nil)
		return nil
	}
	return claims
}

// writeCalcRunError maps calcRunError to the appropriate HTTP response.
func writeCalcRunError(c *gin.Context, err error) {
	if ce, ok := IsCalcRunError(err); ok {
		c.JSON(ce.HTTPStatus(), gin.H{
			"error": gin.H{
				"code":    ce.Code(),
				"message": ce.Error(),
				"traceId": c.GetString("X-Trace-Id"),
			},
		})
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal. Hubungi admin dengan traceId.", nil)
}
