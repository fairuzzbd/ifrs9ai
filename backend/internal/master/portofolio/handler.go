// Package portofolio — HTTP handler layer for mst.portofolio (APP-A).
//
// Handlers are thin: parse → service call → response envelope.
// No business logic or SQL lives here.
// Workflow signing delegates to the generic workflow.Handler (same pattern as matauang).
package portofolio

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

// Handler is the HTTP handler for portofolio endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/portofolio ───────────────────────────────────────────────────

// List handles GET /api/v1/master/portofolio.
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
	for _, p := range result.Items {
		items = append(items, ToResponse(p))
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

// ─── POST /master/portofolio ─────────────────────────────────────────────────

// Create handles POST /api/v1/master/portofolio.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	// Normalize kode to uppercase.
	req.KodePortofolio = strings.ToUpper(req.KodePortofolio)

	p, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(p))
}

// ─── GET /master/portofolio/export ───────────────────────────────────────────

// Export handles GET /api/v1/master/portofolio/export.
// Only CSV is supported (XLSX → 501).
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format == "xlsx" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Export XLSX belum tersedia. Gunakan format csv.",
			domainerrors.Detail{Field: "query.format", Rule: "oneof", Message: "Hanya csv yang tersedia saat ini"},
		))
		c.Status(501)
		return
	}
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

	filename := fmt.Sprintf("portofolio-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/portofolio/:kode ────────────────────────────────────────────

// GetByKode handles GET /api/v1/master/portofolio/:kode.
func (h *Handler) GetByKode(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode portofolio tidak valid (1-20 karakter huruf kapital, angka, underscore).",
			domainerrors.Detail{Field: "path.kode", Rule: "pattern", Message: "Format kode tidak valid"},
		))
		return
	}

	p, err := h.svc.GetByKode(c.Request.Context(), kode, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(p))
}

// ─── PUT /master/portofolio/:kode ────────────────────────────────────────────

// Update handles PUT /api/v1/master/portofolio/:kode.
func (h *Handler) Update(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode portofolio tidak valid.",
			domainerrors.Detail{Field: "path.kode", Rule: "pattern", Message: "Format kode tidak valid"},
		))
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), kode, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(updated))
}

// ─── DELETE /master/portofolio/:kode ─────────────────────────────────────────

// Delete handles DELETE /api/v1/master/portofolio/:kode.
func (h *Handler) Delete(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode portofolio tidak valid.",
			domainerrors.Detail{Field: "path.kode", Rule: "pattern", Message: "Format kode tidak valid"},
		))
		return
	}

	if err := h.svc.SoftDelete(c.Request.Context(), kode); err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, DeleteResponse{
		Deleted:        true,
		DeletedAt:      time.Now().Format(time.RFC3339),
		KodePortofolio: kode,
	})
}

// ─── GET /master/portofolio/:kode/history ────────────────────────────────────

// History handles GET /api/v1/master/portofolio/:kode/history.
func (h *Handler) History(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode portofolio tidak valid."))
		return
	}

	pagParams, err := pagination.ParseParams(c.Query("cursor"), c.Query("limit"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	claims := auth.ClaimsFromGin(c)

	items, hasMore, err := h.svc.ListHistory(c.Request.Context(), kode, pagParams.Cursor, pagParams.Limit, claims)
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

// resolveEntityID looks up the surrogate UUID for a given kode.
func (h *Handler) resolveEntityID(c *gin.Context, kode string) (uuid.UUID, error) {
	p, err := h.svc.GetByKode(c.Request.Context(), kode, false)
	if err != nil {
		return uuid.Nil, err
	}
	return p.ID, nil
}

// Submit handles POST /api/v1/master/portofolio/:kode/submit.
func (h *Handler) Submit(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Kode tidak valid."))
		return
	}
	entityID, err := h.resolveEntityID(c, kode)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "portofolio"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/portofolio/:kode/review.
func (h *Handler) Review(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Kode tidak valid."))
		return
	}
	entityID, err := h.resolveEntityID(c, kode)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "portofolio"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/portofolio/:kode/approve.
func (h *Handler) Approve(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Kode tidak valid."))
		return
	}
	entityID, err := h.resolveEntityID(c, kode)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "portofolio"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/portofolio/:kode/reject.
func (h *Handler) Reject(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Kode tidak valid."))
		return
	}
	entityID, err := h.resolveEntityID(c, kode)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "portofolio"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/portofolio/:kode/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Kode tidak valid."))
		return
	}
	entityID, err := h.resolveEntityID(c, kode)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "portofolio"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// isValidKode returns true if kode matches ^[A-Z0-9_]{1,20}$.
func isValidKode(kode string) bool {
	return kodePortofolioRe.MatchString(kode)
}
