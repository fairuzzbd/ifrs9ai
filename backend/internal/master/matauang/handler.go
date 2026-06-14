// Package matauang — HTTP handler layer for mst.mata_uang (APP-A-MSTR-002).
//
// Responsibilities of handler:
//   - Parse and validate HTTP-level inputs (path params, query params, body)
//   - Call service methods
//   - Map service results to response envelopes via common/response package
//
// Handlers must NOT contain business logic or SQL.
// Permission checks are delegated to auth.RequirePermission middleware;
// handlers only call service and return the result.
//
// Workflow signing endpoints (submit/review/approve/reject/workflow) delegate
// to the generic workflow engine via workflow.Handler; the mata_uang handler
// only resolves kode→UUID before forwarding, then syncs workflow_status back.
package matauang

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

// Handler is the HTTP handler for mata_uang endpoints.
// It is intentionally thin: parse → service call → response.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/mata-uang ─────────────────────────────────────────────────

// List handles GET /api/v1/master/mata-uang.
// Permission: mata_uang.read  (enforced via middleware)
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
		// Only ROLE-AUDIT can see deleted records.
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

	// Build appliedSort from listquery
	sorts := make([]response.SortApplied, 0, len(q.Sort))
	for _, s := range q.Sort {
		sorts = append(sorts, response.SortApplied{Col: s.Col, Dir: s.Dir})
	}

	response.List(c, items, pag, sorts, q.AppliedFilter())
}

// ─── POST /master/mata-uang ────────────────────────────────────────────────

// Create handles POST /api/v1/master/mata-uang.
// Permission: mata_uang.create  (enforced via middleware)
// Idempotency-Key checked by middleware.
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

	resp := ToResponse(m)
	response.Created(c, resp)
}

// ─── GET /master/mata-uang/export ─────────────────────────────────────────

// Export handles GET /api/v1/master/mata-uang/export.
// Permission: mata_uang.read  (enforced via middleware)
// Sync for < 10k rows; async placeholder for large datasets.
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

	// Only CSV sync export is implemented here; XLSX requires excelize (Phase 3+).
	// Async path (>= 10k rows) is flagged via row count — for mata_uang this is
	// unlikely to be reached in practice.
	reader, rowCount, err := h.svc.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("mata-uang-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/mata-uang/:kode ──────────────────────────────────────────

// GetByKode handles GET /api/v1/master/mata-uang/:kode.
// Permission: mata_uang.read  (enforced via middleware)
func (h *Handler) GetByKode(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode mata uang harus 3 huruf kapital.",
			domainerrors.Detail{Field: "path.kode", Rule: "pattern", Message: "Format kode tidak valid"},
		))
		return
	}

	m, err := h.svc.GetByKode(c.Request.Context(), kode, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := ToResponse(m)
	response.OK(c, resp)
}

// ─── PUT /master/mata-uang/:kode ──────────────────────────────────────────

// Update handles PUT /api/v1/master/mata-uang/:kode.
// Permission: mata_uang.update  (enforced via middleware)
// Optimistic lock via rowVersion in request body.
func (h *Handler) Update(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode mata uang harus 3 huruf kapital.",
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

// ─── DELETE /master/mata-uang/:kode ───────────────────────────────────────

// Delete handles DELETE /api/v1/master/mata-uang/:kode.
// Permission: mata_uang.delete  (enforced via middleware)
// Soft-delete: sets deleted_at/deleted_by. Rejects system currency and referenced records.
func (h *Handler) Delete(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode mata uang harus 3 huruf kapital.",
			domainerrors.Detail{Field: "path.kode", Rule: "pattern", Message: "Format kode tidak valid"},
		))
		return
	}

	if err := h.svc.SoftDelete(c.Request.Context(), kode); err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, DeleteResponse{
		Deleted:   true,
		DeletedAt: time.Now().Format(time.RFC3339),
		EntityID:  kode,
	})
}

// ─── GET /master/mata-uang/:kode/history ──────────────────────────────────

// History handles GET /api/v1/master/mata-uang/:kode/history.
// Permission: mata_uang.read  (enforced via middleware)
// before/after fields only visible to audit_log.read holders.
func (h *Handler) History(c *gin.Context) {
	kode := strings.ToUpper(c.Param("kode"))
	if !isValidKode(kode) {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Kode mata uang harus 3 huruf kapital."))
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

// ─── Workflow action endpoints ─────────────────────────────────────────────
//
// Pattern: resolve kode → UUID, then forward to generic workflow handler
// by rewriting the request path params so the generic handler can parse them.
//
// The generic workflow handler routes are:
//   POST /:resource/:id/{submit,review,approve,reject}
//   GET  /:resource/:id/workflow
//
// For mata-uang the :id is the surrogate UUID, resolved here from kode.

// resolveEntityID looks up the surrogate UUID for a given kode.
// Returns (uuid.Nil, error) if not found.
func (h *Handler) resolveEntityID(c *gin.Context, kode string) (uuid.UUID, error) {
	m, err := h.svc.GetByKode(c.Request.Context(), kode, false)
	if err != nil {
		return uuid.Nil, err
	}
	return m.ID, nil
}

// Submit handles POST /api/v1/master/mata-uang/:kode/submit.
// Resolves kode→UUID, then delegates to the generic workflow handler.
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
	// Rewrite gin params so the generic workflow handler can extract :resource and :id.
	c.Params = gin.Params{
		{Key: "resource", Value: "mata-uang"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/mata-uang/:kode/review.
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
		{Key: "resource", Value: "mata-uang"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/mata-uang/:kode/approve.
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
		{Key: "resource", Value: "mata-uang"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/mata-uang/:kode/reject.
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
		{Key: "resource", Value: "mata-uang"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/mata-uang/:kode/workflow.
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
		{Key: "resource", Value: "mata-uang"},
		{Key: "id", Value: entityID.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────

// isValidKode returns true if kode matches ^[A-Z]{3}$.
func isValidKode(kode string) bool {
	if len(kode) != 3 {
		return false
	}
	for _, r := range kode {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
