// Package mappingjurnal — HTTP handler layer for mst.mapping_jurnal_header + detail (APP-D).
//
// Pattern identical to internal/master/matauang/ — thin handler → service → repo.
// Workflow signing endpoints resolve ID from path then delegate to generic workflow.Handler.
package mappingjurnal

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

// Handler is the HTTP handler for mapping_jurnal endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/mapping-jurnal ───────────────────────────────────────────────

// List handles GET /api/v1/master/mapping-jurnal.
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

	items := make([]HeaderResponse, 0, len(result.Items))
	for _, h2 := range result.Items {
		items = append(items, ToHeaderResponseNoDetails(h2))
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

// ─── POST /master/mapping-jurnal ──────────────────────────────────────────────

// Create handles POST /api/v1/master/mapping-jurnal.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	hwd, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToHeaderResponse(hwd))
}

// ─── GET /master/mapping-jurnal/export ───────────────────────────────────────

// Export handles GET /api/v1/master/mapping-jurnal/export.
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

	filename := fmt.Sprintf("mapping-jurnal-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/mapping-jurnal/:id ──────────────────────────────────────────

// GetByID handles GET /api/v1/master/mapping-jurnal/:id.
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	hwd, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToHeaderResponse(hwd))
}

// ─── PATCH /master/mapping-jurnal/:id ────────────────────────────────────────

// Update handles PATCH /api/v1/master/mapping-jurnal/:id.
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

	hwd, err := h.svc.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToHeaderResponse(hwd))
}

// ─── DELETE /master/mapping-jurnal/:id ───────────────────────────────────────

// Delete handles DELETE /api/v1/master/mapping-jurnal/:id.
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

// ─── GET /master/mapping-jurnal/:id/history ───────────────────────────────────

// History handles GET /api/v1/master/mapping-jurnal/:id/history.
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

	pag := &response.PaginationMeta{
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Limit:      pagParams.Limit,
	}

	response.List(c, items, pag, nil, nil)
}

// ─── Workflow action endpoints ────────────────────────────────────────────────
//
// Pattern: parse UUID from path, rewrite gin params, forward to generic workflow.Handler.

// Submit handles POST /api/v1/master/mapping-jurnal/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	// Verify header exists before delegating to workflow.
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "mapping-jurnal"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/mapping-jurnal/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "mapping-jurnal"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/mapping-jurnal/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "mapping-jurnal"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/mapping-jurnal/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "mapping-jurnal"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/mapping-jurnal/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "mapping-jurnal"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseUUIDParam extracts and parses a UUID path parameter.
// Returns false and writes a 400 response on failure.
func parseUUIDParam(c *gin.Context, paramName string) (uuid.UUID, bool) {
	raw := c.Param(paramName)
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(
			domainerrors.CodeValidationFailed,
			fmt.Sprintf("Parameter '%s' harus berformat UUID v4.", paramName),
			domainerrors.Detail{Field: "path." + paramName, Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}
