// Package impactmevpd — HTTP handler layer for mst.impact_mev_pd (APP-A).
package impactmevpd

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/common/response"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// Handler is the HTTP handler for impact_mev_pd endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// List handles GET /api/v1/master/impact-mev-pd.
func (h *Handler) List(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	includeDeleted := false
	if c.Query("include_deleted") == "true" {
		claims := auth.ClaimsFromGin(c)
		if claims != nil && claims.HasPermission("audit_log.read") {
			includeDeleted = true
		}
	}

	result, err := h.svc.List(c.Request.Context(), q, pagParams.Cursor, pagParams.Limit, includeDeleted)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]Response, 0, len(result.Items))
	for _, e := range result.Items {
		items = append(items, ToResponse(e))
	}

	pag := &response.PaginationMeta{
		HasMore:       result.Pagination.HasMore,
		NextCursor:    result.Pagination.NextCursor,
		TotalEstimate: result.Pagination.TotalEstimate,
		Limit:         result.Pagination.Limit,
	}

	sorts := make([]response.SortApplied, 0, len(q.Sort))
	for _, s := range q.Sort {
		sorts = append(sorts, response.SortApplied{Col: s.Col, Dir: s.Dir})
	}

	response.List(c, items, pag, sorts, q.AppliedFilter())
}

// Create handles POST /api/v1/master/impact-mev-pd.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	e, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(e))
}

// Export handles GET /api/v1/master/impact-mev-pd/export.
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	switch format {
	case "csv":
		// fall through
	case "xlsx":
		c.JSON(http.StatusNotImplemented, gin.H{"error": gin.H{
			"code":    "EXPORT_FORMAT_NOT_IMPLEMENTED",
			"message": "Format XLSX belum tersedia. Gunakan format CSV.",
		}})
		return
	default:
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Format export tidak valid. Gunakan 'csv'.",
			domainerrors.Detail{Field: "query.format", Rule: "oneof", Message: "Harus csv"},
		))
		return
	}

	reader, rowCount, err := h.svc.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("impact-mev-pd-%s.csv", time.Now().Format("20060102"))
	c.Header("Content-Disposition", "attachment; filename="+fmt.Sprintf("%q", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("X-Total-Rows", fmt.Sprintf("%d", rowCount))

	buf := make([]byte, 32*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if readErr != nil {
			break
		}
	}
}

// GetActive handles GET /api/v1/master/impact-mev-pd/active.
// Returns APPROVED rows for the given periode_id.
// Used by ECL engine Phase 4 (OQ-5).
func (h *Handler) GetActive(c *gin.Context) {
	periodeIDStr := c.Query("periode_id")
	if periodeIDStr == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"periode_id query parameter wajib diisi.",
			domainerrors.Detail{Field: "query.periode_id", Rule: "required", Message: "Harus UUID valid"},
		))
		return
	}
	periodeID, err := uuid.Parse(periodeIDStr)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"periode_id harus berupa UUID valid.",
			domainerrors.Detail{Field: "query.periode_id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return
	}

	result, err := h.svc.GetActive(c.Request.Context(), periodeID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, result)
}

// GetByID handles GET /api/v1/master/impact-mev-pd/:id.
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	e, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(e))
}

// Update handles PUT /api/v1/master/impact-mev-pd/:id.
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(updated))
}

// Delete handles DELETE /api/v1/master/impact-mev-pd/:id.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.SoftDelete(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, DeleteResponse{
		Deleted:   true,
		DeletedAt: time.Now().Format(time.RFC3339),
		EntityID:  id.String(),
	})
}

// History handles GET /api/v1/master/impact-mev-pd/:id/history.
func (h *Handler) History(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	claims := auth.ClaimsFromGin(c)
	items, hasMore, err := h.svc.ListHistory(c.Request.Context(), id, pagParams.Cursor, pagParams.Limit, claims)
	if err != nil {
		response.Error(c, err)
		return
	}

	var nextCursor *string
	if hasMore && len(items) > 0 {
		nc, encErr := pagination.EncodeCursor(pagination.CursorData{ID: items[len(items)-1].EventTime})
		if encErr == nil {
			nextCursor = &nc
		}
	}

	pag := &response.PaginationMeta{HasMore: hasMore, NextCursor: nextCursor, Limit: pagParams.Limit}
	response.List(c, items, pag, nil, nil)
}

// Submit handles POST /api/v1/master/impact-mev-pd/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "impact-mev-pd", h.wfHandler.Submit, "SUBMIT")
}

// Review handles POST /api/v1/master/impact-mev-pd/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "impact-mev-pd", h.wfHandler.Review, "REVIEW")
}

// Approve handles POST /api/v1/master/impact-mev-pd/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "impact-mev-pd", h.wfHandler.Approve, "APPROVE")
}

// Approve2 handles POST /api/v1/master/impact-mev-pd/:id/approve2 (ALCO — step-up MFA).
func (h *Handler) Approve2(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "impact-mev-pd", h.wfHandler.Approve2, "APPROVE2")
}

// Reject handles POST /api/v1/master/impact-mev-pd/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "impact-mev-pd", h.wfHandler.Reject, "REJECT")
}

// WorkflowStatus handles GET /api/v1/master/impact-mev-pd/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-mev-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

//nolint:unparam
func (h *Handler) delegateWorkflow(c *gin.Context, entityID uuid.UUID, resource string, wfFunc gin.HandlerFunc, _ string) {
	c.Params = gin.Params{
		{Key: "resource", Value: resource},
		{Key: "id", Value: entityID.String()},
	}
	wfFunc(c)
}

//nolint:unparam
func parseUUIDParam(c *gin.Context, paramName string) (uuid.UUID, bool) {
	raw := c.Param(paramName)
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Parameter '%s' harus berupa UUID valid.", paramName),
			domainerrors.Detail{Field: "path." + paramName, Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}
