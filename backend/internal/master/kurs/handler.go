// Package kurs — HTTP handler layer for mst.kurs (APP-A-MSTR-009).
//
// Thin handler: parse → service call → response envelope.
// No business logic here — all rules live in service.go.
package kurs

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

// Handler is the HTTP handler for kurs endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/kurs ─────────────────────────────────────────────────────────

// List handles GET /api/v1/master/kurs.
// Permission: kurs.read (enforced via middleware)
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
	for _, k := range result.Items {
		items = append(items, ToResponse(k))
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

// ─── POST /master/kurs ────────────────────────────────────────────────────────

// Create handles POST /api/v1/master/kurs.
// Permission: kurs.create (enforced via middleware)
// Idempotency-Key checked by middleware.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	k, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(k))
}

// ─── GET /master/kurs/export ──────────────────────────────────────────────────

// Export handles GET /api/v1/master/kurs/export.
// Permission: kurs.read (enforced via middleware)
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format != "csv" && format != "xlsx" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Format export tidak valid. Gunakan 'csv' atau 'xlsx'.",
			domainerrors.Detail{Field: "query.format", Rule: "oneof", Message: "Harus csv atau xlsx"},
		))
		return
	}

	reader, rowCount, err := h.svc.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("kurs-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/kurs/:id ─────────────────────────────────────────────────────

// GetByID handles GET /api/v1/master/kurs/:id.
// Permission: kurs.read (enforced via middleware)
func (h *Handler) GetByID(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	k, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(k))
}

// ─── PUT /master/kurs/:id ─────────────────────────────────────────────────────

// Update handles PUT /api/v1/master/kurs/:id.
// Permission: kurs.update (enforced via middleware)
// Optimistic lock via rowVersion in request body.
func (h *Handler) Update(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
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

// ─── DELETE /master/kurs/:id ──────────────────────────────────────────────────

// Delete handles DELETE /api/v1/master/kurs/:id.
// Permission: kurs.delete (enforced via middleware)
// Soft-delete: sets deleted_at/deleted_by. Rejects if locked.
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
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

// ─── GET /master/kurs/:id/history ─────────────────────────────────────────────

// History handles GET /api/v1/master/kurs/:id/history.
// Permission: kurs.read (enforced via middleware)
func (h *Handler) History(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Error(c, err)
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

	pag := &response.PaginationMeta{
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Limit:      pagParams.Limit,
	}

	response.List(c, items, pag, nil, nil)
}

// ─── Workflow action endpoints ─────────────────────────────────────────────────

// resolveEntityID looks up the kurs UUID from :id path param (it IS already UUID for kurs).
func (h *Handler) resolveEntityID(c *gin.Context) (uuid.UUID, error) {
	return parseIDParam(c)
}

// Submit handles POST /api/v1/master/kurs/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	entityID, err := h.resolveEntityID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "kurs"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/kurs/:id/review.
func (h *Handler) Review(c *gin.Context) {
	entityID, err := h.resolveEntityID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "kurs"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/kurs/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	entityID, err := h.resolveEntityID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "kurs"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/kurs/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	entityID, err := h.resolveEntityID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "kurs"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/kurs/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	entityID, err := h.resolveEntityID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "kurs"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseIDParam extracts and validates the :id path param as UUID.
func parseIDParam(c *gin.Context) (uuid.UUID, error) {
	rawID := c.Param("id")
	id, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter :id harus UUID yang valid.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		)
	}
	return id, nil
}
