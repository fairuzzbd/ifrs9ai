package rollforward

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// handler.go — 6 Gin HTTP handlers for ECL Roll-Forward CKPN (P4-M11).
//
// Endpoint mapping (api/openapi/app-c-roll-forward.yaml):
//
//	POST   /ecl/roll-forward/compute                     computeRollForward     (M11-001)
//	GET    /ecl/roll-forward                             getRollForward         (M11-004)
//	GET    /ecl/roll-forward/:id/export                  exportDisclosure       (M11-005)
//	GET    /ecl/roll-forward/portfolios/:pid             getPortfolioRollForward (M11-004)
//	GET    /ecl/roll-forward/portfolios/:pid/instruments listPortfolioInstruments (M11-004)
//	GET    /ecl/dashboard/ckpn-trend                     getCKPNTrend           (M11-006)
//
// Auth: JWT via auth.Middleware (wired in routes.go).
// Idempotency-Key: required on POST /compute (DEC-021).
// No float64 in response (DEC-016).

// Handler holds the roll-forward service.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler. Panics if svc is nil.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("rollforward.NewHandler: svc must not be nil")
	}
	return &Handler{svc: svc}
}

// ─── POST /ecl/roll-forward/compute ─────────────────────────────────────────

// computeRollForwardRequest is the JSON body for POST /ecl/roll-forward/compute.
type computeRollForwardRequest struct {
	CurrentCalcRunID    string  `json:"currentCalcRunId" binding:"required"`
	PriorCalcRunID      *string `json:"priorCalcRunId"` // nullable — first period
	AllowNonSealedPrior bool    `json:"allowNonSealedPrior"`
	ForceMismatchExport bool    `json:"forceMismatchExport"`
}

// ComputeRollForward handles POST /ecl/roll-forward/compute (M11-001).
// Permission: ecl.roll_forward.compute
// Idempotency-Key: required
func (h *Handler) ComputeRollForward(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermRollForwardCompute) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermRollForwardCompute+" tidak terpenuhi.", nil)
		return
	}

	var req computeRollForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Request body tidak valid: "+err.Error(), nil)
		return
	}

	currentID, err := uuid.Parse(req.CurrentCalcRunID)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "currentCalcRunId bukan UUID valid.", nil)
		return
	}

	var priorID *uuid.UUID
	if req.PriorCalcRunID != nil {
		id, err := uuid.Parse(*req.PriorCalcRunID)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, "priorCalcRunId bukan UUID valid.", nil)
			return
		}
		priorID = &id
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID dalam JWT tidak valid.", nil)
		return
	}

	report, err := h.svc.ComputeRollForward(c.Request.Context(), ComputeRequest{
		CurrentCalcRunID:    currentID,
		PriorCalcRunID:      priorID,
		DetectionMethod:     DetectionMethodBasicStatusDiff,
		AllowNonSealedPrior: req.AllowNonSealedPrior,
		ForceMismatchExport: req.ForceMismatchExport,
		ActorID:             actorID,
	})
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	response.OK(c, report)
}

// ─── GET /ecl/roll-forward ───────────────────────────────────────────────────

// GetRollForward handles GET /ecl/roll-forward (M11-004).
// Query params: currentCalcRunId (required), priorCalcRunId (optional).
// Permission: ecl.roll_forward.read
func (h *Handler) GetRollForward(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermRollForwardRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermRollForwardRead+" tidak terpenuhi.", nil)
		return
	}

	currentIDStr := c.Query("currentCalcRunId")
	if currentIDStr == "" {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Query param currentCalcRunId wajib diisi.", nil)
		return
	}
	currentID, err := uuid.Parse(currentIDStr)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "currentCalcRunId bukan UUID valid.", nil)
		return
	}

	var priorID *uuid.UUID
	if priorStr := c.Query("priorCalcRunId"); priorStr != "" {
		id, err := uuid.Parse(priorStr)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, "priorCalcRunId bukan UUID valid.", nil)
			return
		}
		priorID = &id
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID dalam JWT tidak valid.", nil)
		return
	}

	report, err := h.svc.GetRollForward(c.Request.Context(), currentID, priorID, actorID)
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	response.OK(c, report)
}

// ─── GET /ecl/roll-forward/:id/export ────────────────────────────────────────

// ExportDisclosure handles GET /ecl/roll-forward/:id/export (M11-005).
// :id = currentCalcRunID. Query: priorCalcRunId, force_mismatch.
// Permission: ecl.roll_forward.export
func (h *Handler) ExportDisclosure(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermRollForwardExport) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermRollForwardExport+" tidak terpenuhi.", nil)
		return
	}

	currentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "calc_run id bukan UUID valid.", nil)
		return
	}

	var priorID *uuid.UUID
	if priorStr := c.Query("priorCalcRunId"); priorStr != "" {
		id, err := uuid.Parse(priorStr)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, "priorCalcRunId bukan UUID valid.", nil)
			return
		}
		priorID = &id
	}

	forceMismatch := c.Query("force_mismatch") == "true"

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID dalam JWT tidak valid.", nil)
		return
	}

	// Compute report first (on-demand).
	report, err := h.svc.GetRollForward(c.Request.Context(), currentID, priorID, actorID)
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	xlsxBytes, err := h.svc.ExportXLSX(c.Request.Context(), report, forceMismatch)
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	filename := "roll-forward-" + report.CurrentPeriodeID + ".xlsx"
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("X-Total-Rows", "1") // stub — full XLSX generation TODO Phase 4 wire excelize
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsxBytes)
}

// ─── GET /ecl/roll-forward/portfolios/:pid ────────────────────────────────────

// GetPortfolioRollForward handles GET /ecl/roll-forward/portfolios/:pid (M11-004).
// Query: currentCalcRunId (required), priorCalcRunId (optional).
// Permission: ecl.portfolio_aggregate.read
func (h *Handler) GetPortfolioRollForward(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermPortfolioAggregateRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermPortfolioAggregateRead+" tidak terpenuhi.", nil)
		return
	}

	portfolioID, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "portofolio id bukan UUID valid.", nil)
		return
	}

	currentIDStr := c.Query("currentCalcRunId")
	if currentIDStr == "" {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "Query param currentCalcRunId wajib diisi.", nil)
		return
	}
	currentID, err := uuid.Parse(currentIDStr)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "currentCalcRunId bukan UUID valid.", nil)
		return
	}

	var priorID *uuid.UUID
	if priorStr := c.Query("priorCalcRunId"); priorStr != "" {
		id, err := uuid.Parse(priorStr)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, "priorCalcRunId bukan UUID valid.", nil)
			return
		}
		priorID = &id
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID dalam JWT tidak valid.", nil)
		return
	}

	result, err := h.svc.GetPortfolioRollForward(c.Request.Context(), portfolioID, currentID, priorID, actorID)
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	response.OK(c, result)
}

// ─── GET /ecl/roll-forward/portfolios/:pid/instruments ───────────────────────

// ListPortfolioInstruments handles GET /ecl/roll-forward/portfolios/:pid/instruments (M11-004).
// Returns all instrument IDs in a portfolio (DataTable drill-down — stub for Phase 4).
// Permission: ecl.portfolio_aggregate.read
func (h *Handler) ListPortfolioInstruments(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermPortfolioAggregateRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermPortfolioAggregateRead+" tidak terpenuhi.", nil)
		return
	}

	portfolioID, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "portofolio id bukan UUID valid.", nil)
		return
	}

	// Validate portfolio exists.
	nama, found, err := h.svc.repo.GetPortofolioNama(c.Request.Context(), portfolioID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if !found {
		response.ErrorWithStatus(c, http.StatusNotFound,
			domainerrors.CodeNotFound,
			"Portofolio "+portfolioID.String()+" tidak ditemukan.", nil)
		return
	}

	ids, err := h.svc.repo.GetPortofolioInstruments(c.Request.Context(), portfolioID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, ids, &response.PaginationMeta{
		NextCursor: nil,
		HasMore:    false,
		Limit:      len(ids),
	}, nil, map[string]any{"portofolioId": portfolioID, "portofolioNama": nama})
}

// ─── GET /ecl/dashboard/ckpn-trend ───────────────────────────────────────────

// GetCKPNTrend handles GET /ecl/dashboard/ckpn-trend (M11-006).
// Query: periods (int, default 12, max 24).
// Permission: ecl.roll_forward.read
func (h *Handler) GetCKPNTrend(c *gin.Context) {
	claims := claimsFromCtx(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission(PermRollForwardRead) {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission "+PermRollForwardRead+" tidak terpenuhi.", nil)
		return
	}

	periods := 12
	if pStr := c.Query("periods"); pStr != "" {
		p, err := strconv.Atoi(pStr)
		if err != nil || p < 2 || p > 24 {
			response.ErrorWithStatus(c, http.StatusBadRequest,
				domainerrors.CodeValidationFailed, "Query param periods harus angka antara 2 dan 24.", nil)
			return
		}
		periods = p
	}

	points, err := h.svc.GetCKPNTrend(c.Request.Context(), periods)
	if err != nil {
		writeRollForwardError(c, err)
		return
	}

	response.OK(c, gin.H{
		"periods": points,
		"count":   len(points),
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

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

// writeRollForwardError maps rollforward domain errors to HTTP responses.
// Handles: rollforward *domainError (package-local), common *DomainError, and fallback 500.
func writeRollForwardError(c *gin.Context, err error) {
	if de, ok := err.(*domainError); ok {
		status := rollForwardHTTPStatus(de.Code())
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":    de.Code(),
				"message": de.Error(),
				"details": []any{},
				"traceId": c.GetString("X-Trace-Id"),
			},
		})
		return
	}
	// Fall through to common domain error or 500.
	response.Error(c, err)
}

// rollForwardHTTPStatus maps roll-forward error codes to HTTP status codes.
func rollForwardHTTPStatus(code string) int {
	switch code {
	case CodeRollForwardPriorNotFound, CodeRollForwardPortfolioNotFound:
		return http.StatusNotFound
	case CodeRollForwardCurrentInvalidState, CodeRollForwardPriorNotSealed,
		CodeRollForwardPeriodeMismatch, CodeRollForwardDetectionMethodInvalid,
		CodeRollForwardExportMismatchForbidden, CodeRollForwardScopeMismatch,
		CodeRollForwardInvalidCalcRunStatus, CodeRollForwardInvalidPriorPeriod,
		CodeRollForwardTrendInsufficientData:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
