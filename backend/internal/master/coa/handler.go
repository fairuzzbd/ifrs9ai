// Package coa — HTTP handler layer for mst.chart_of_accounts (APP-A-MSTR-COA).
//
// Handler is intentionally thin: parse → service call → response envelope.
// Permission checks are via auth.RequirePermission middleware.
// Workflow signing delegates to the generic workflow.Handler after resolving id.
package coa

import (
	"fmt"
	"io"
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

// Handler is the HTTP handler for chart_of_accounts endpoints.
type Handler struct {
	svc      *Service
	importer *Importer
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, importer *Importer, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, importer: importer, wfHandler: wfHandler}
}

// ─── GET /master/coa ─────────────────────────────────────────────────────────

// List handles GET /api/v1/master/coa.
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
	for _, item := range result.Items {
		items = append(items, ToResponse(item))
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

// ─── POST /master/coa ────────────────────────────────────────────────────────

// Create handles POST /api/v1/master/coa.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	created, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(created))
}

// ─── GET /master/coa/export ──────────────────────────────────────────────────

// Export handles GET /api/v1/master/coa/export.
// Only CSV is implemented; XLSX returns 501.
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format == "xlsx" {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"code":    "NOT_IMPLEMENTED",
				"message": "Export XLSX belum tersedia. Gunakan format=csv.",
			},
		})
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

	filename := fmt.Sprintf("chart-of-accounts-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/coa/:id ─────────────────────────────────────────────────────

// Get handles GET /api/v1/master/coa/:id.
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}

	item, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(item))
}

// ─── PATCH /master/coa/:id ───────────────────────────────────────────────────

// Update handles PATCH /api/v1/master/coa/:id.
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseUUID(c)
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

// ─── DELETE /master/coa/:id ──────────────────────────────────────────────────

// Delete handles DELETE /api/v1/master/coa/:id.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseUUID(c)
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

// ─── GET /master/coa/:id/history ─────────────────────────────────────────────

// History handles GET /api/v1/master/coa/:id/history.
func (h *Handler) History(c *gin.Context) {
	id, ok := parseUUID(c)
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

// Submit handles POST /api/v1/master/coa/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	h.forwardToWorkflow(c, id, "CHART_OF_ACCOUNTS", h.wfHandler.Submit)
}

// Review handles POST /api/v1/master/coa/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	h.forwardToWorkflow(c, id, "CHART_OF_ACCOUNTS", h.wfHandler.Review)
}

// Approve handles POST /api/v1/master/coa/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	h.forwardToWorkflow(c, id, "CHART_OF_ACCOUNTS", h.wfHandler.Approve)
}

// Reject handles POST /api/v1/master/coa/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	h.forwardToWorkflow(c, id, "CHART_OF_ACCOUNTS", h.wfHandler.Reject)
}

// WorkflowStatus handles GET /api/v1/master/coa/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	h.forwardToWorkflow(c, id, "CHART_OF_ACCOUNTS", h.wfHandler.GetStatus)
}

// forwardToWorkflow rewrites gin params and delegates to a generic workflow handler func.
func (h *Handler) forwardToWorkflow(c *gin.Context, id uuid.UUID, resource string, fn func(*gin.Context)) {
	c.Params = gin.Params{
		{Key: "resource", Value: resource},
		{Key: "id", Value: id.String()},
	}
	fn(c)
}

// ─── Import endpoints ─────────────────────────────────────────────────────────

// ImportXLSX handles POST /api/v1/master/coa/import-xlsx.
// Multipart: file (XLSX, max 10MB) + sumber_coa (text).
func (h *Handler) ImportXLSX(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' wajib diisi. "+err.Error()))
		return
	}
	defer file.Close() //nolint:errcheck

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Gagal membaca file yang diupload."))
		return
	}

	sumberCoa := strings.TrimSpace(c.Request.FormValue("sumber_coa"))
	if sumberCoa == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'sumber_coa' wajib diisi.",
			domainerrors.Detail{Field: "form.sumber_coa", Rule: "required"}))
		return
	}

	req := ImportXLSXRequest{SumberCoa: sumberCoa}
	result, err := h.importer.SubmitImport(c.Request.Context(), req, fileBytes)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": result})
}

// GetImportJob handles GET /api/v1/master/coa/import-jobs/:jobId.
func (h *Handler) GetImportJob(c *gin.Context) {
	jobID := c.Param("jobId")
	if jobID == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "jobId wajib diisi."))
		return
	}

	status, err := h.importer.GetJobStatus(c.Request.Context(), jobID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, status)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseUUID parses the :id path param as UUID. Returns false + writes error response on failure.
func parseUUID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.Param("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"ID harus berupa UUID yang valid.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}
