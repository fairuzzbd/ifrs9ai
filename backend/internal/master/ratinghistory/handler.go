// Package ratinghistory — HTTP handler layer for mst.rating_history_counterparty.
package ratinghistory

import (
	"fmt"
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

// Handler is the HTTP handler for rating_history endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/rating-history ───────────────────────────────────────────

// List handles GET /api/v1/master/rating-history.
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

	result, err := h.svc.List(c.Request.Context(), q, pagParams.Cursor, pagParams.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]Response, 0, len(result.Items))
	for _, rh := range result.Items {
		items = append(items, ToResponse(rh))
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

// ─── GET /counterparty/:counterpartyId/rating-history ─────────────────────

// ListByCounterparty handles GET /api/v1/master/counterparty/:counterpartyId/rating-history.
func (h *Handler) ListByCounterparty(c *gin.Context) {
	counterpartyIDStr := c.Param("counterpartyId")
	counterpartyID, err := uuid.Parse(counterpartyIDStr)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"counterpartyId harus berformat UUID.",
			domainerrors.Detail{Field: "path.counterpartyId", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return
	}

	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.svc.ListByCounterparty(c.Request.Context(), counterpartyID, pagParams.Cursor, pagParams.Limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]Response, 0, len(result.Items))
	for _, rh := range result.Items {
		items = append(items, ToResponse(rh))
	}

	pag := &response.PaginationMeta{
		HasMore:       result.Pagination.HasMore,
		NextCursor:    result.Pagination.NextCursor,
		TotalEstimate: result.Pagination.TotalEstimate,
		Limit:         result.Pagination.Limit,
	}
	response.List(c, items, pag, nil, nil)
}

// ─── POST /master/rating-history ──────────────────────────────────────────

// Create handles POST /api/v1/master/rating-history.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	rh, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(rh))
}

// ─── GET /master/rating-history/export ────────────────────────────────────

// Export handles GET /api/v1/master/rating-history/export.
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format != "csv" {
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

	filename := fmt.Sprintf("rating-history-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/rating-history/:id ───────────────────────────────────────

// GetByID handles GET /api/v1/master/rating-history/:id.
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	rh, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(rh))
}

// ─── PUT /master/rating-history/:id ───────────────────────────────────────

// Update handles PUT /api/v1/master/rating-history/:id.
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseIDParam(c)
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

// ─── DELETE /master/rating-history/:id ────────────────────────────────────

// Delete handles DELETE /api/v1/master/rating-history/:id.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c)
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

// ─── GET /master/rating-history/:id/history ───────────────────────────────

// History handles GET /api/v1/master/rating-history/:id/history.
func (h *Handler) History(c *gin.Context) {
	id, ok := parseIDParam(c)
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

// ─── Workflow action endpoints ─────────────────────────────────────────────

// Submit handles POST /api/v1/master/rating-history/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	entityID, ok := parseIDParam(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "rating-history"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/rating-history/:id/review.
func (h *Handler) Review(c *gin.Context) {
	entityID, ok := parseIDParam(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "rating-history"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/rating-history/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	entityID, ok := parseIDParam(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "rating-history"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/rating-history/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	entityID, ok := parseIDParam(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "rating-history"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/rating-history/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	entityID, ok := parseIDParam(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "rating-history"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────

// parseIDParam parses :id path param as UUID.
func parseIDParam(c *gin.Context) (uuid.UUID, bool) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter 'id' harus berformat UUID.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}
