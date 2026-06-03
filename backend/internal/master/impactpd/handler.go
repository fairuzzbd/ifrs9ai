package impactpd

import (
	"fmt"
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

// Handler is the HTTP handler for impact_pd endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/impact-pd ────────────────────────────────────────────────────

// List handles GET /api/v1/master/impact-pd.
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
	for _, m := range result.Items {
		items = append(items, ToResponse(m))
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

// ─── POST /master/impact-pd ───────────────────────────────────────────────────

// Create handles POST /api/v1/master/impact-pd.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	m, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(m))
}

// ─── GET /master/impact-pd/export ─────────────────────────────────────────────

// Export handles GET /api/v1/master/impact-pd/export.
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := c.DefaultQuery("format", "csv")
	if format != "csv" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Format export xlsx belum tersedia. Gunakan 'csv'.",
			domainerrors.Detail{Field: "query.format", Rule: "oneof", Message: "Hanya csv yang didukung saat ini"},
		))
		return
	}

	reader, rowCount, err := h.svc.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("impact-pd-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/impact-pd/:id ────────────────────────────────────────────────

// GetByID handles GET /api/v1/master/impact-pd/:id.
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	m, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(m))
}

// ─── PUT /master/impact-pd/:id ────────────────────────────────────────────────

// Update handles PUT /api/v1/master/impact-pd/:id.
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
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

// ─── DELETE /master/impact-pd/:id ─────────────────────────────────────────────

// Delete handles DELETE /api/v1/master/impact-pd/:id.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
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

// ─── GET /master/impact-pd/:id/history ───────────────────────────────────────

// History handles GET /api/v1/master/impact-pd/:id/history.
func (h *Handler) History(c *gin.Context) {
	id, ok := parseID(c)
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

// ─── Workflow action endpoints ─────────────────────────────────────────────────

// Submit handles POST /api/v1/master/impact-pd/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/impact-pd/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/impact-pd/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve(c)
}

// Approve2 handles POST /api/v1/master/impact-pd/:id/approve2.
func (h *Handler) Approve2(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve2(c)
}

// Reject handles POST /api/v1/master/impact-pd/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/impact-pd/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "impact-pd"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func parseID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.Param("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter 'id' harus berformat UUID v4.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}
