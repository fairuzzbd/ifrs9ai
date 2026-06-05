// Package periodebuku — HTTP handler layer for mst.periode_buku (APP-D-MSTR-001).
//
// Handler is intentionally thin: parse → service call → response envelope.
// No business logic or SQL in handlers.
//
// Workflow signing endpoints (submit/review/approve/reject) delegate to the
// generic workflow engine via workflow.Handler. The ID parameter is the UUID
// surrogate (not a business kode), resolved directly from the path.
//
// TODO (Phase 5 — APP-D): add handlers for:
//   - POST /:id/softclose  — Soft-close period (OPEN→SOFT_CLOSED); AKUN-CTL + MFA
//   - POST /:id/hardclose  — Hard-close period (SOFT_CLOSED→CLOSED); CFO + MFA step-up (DEC-027)
//   - POST /:id/reopen     — Reopen soft-closed period; 4-eyes with approver
//
// These belong to the Periode Buku domain lifecycle, NOT the CRUD master module.
package periodebuku

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

// Handler is the HTTP handler for periode_buku endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/periode-buku ─────────────────────────────────────────────────

// List handles GET /api/v1/master/periode-buku.
// Permission: periode.read (enforced via middleware)
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

// ─── POST /master/periode-buku ───────────────────────────────────────────────

// Create handles POST /api/v1/master/periode-buku.
// Permission: periode.create (enforced via middleware)
// Idempotency-Key checked by middleware.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	p, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(p))
}

// ─── GET /master/periode-buku/export ─────────────────────────────────────────

// Export handles GET /api/v1/master/periode-buku/export.
// Permission: periode.read (enforced via middleware)
// Sync for <= 10k rows; async via MinIO for larger datasets (ux-patterns.md §1.4).
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

	// Only CSV sync export implemented; XLSX deferred to Phase 3+.
	reader, rowCount, err := h.svc.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("periode-buku-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/periode-buku/:id ────────────────────────────────────────────

// GetByID handles GET /api/v1/master/periode-buku/:id.
// Permission: periode.read (enforced via middleware)
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	p, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(p))
}

// ─── PATCH /master/periode-buku/:id ──────────────────────────────────────────

// Update handles PATCH /api/v1/master/periode-buku/:id.
// Permission: periode.update (enforced via middleware)
// Optimistic lock via rowVersion in request body.
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

// ─── DELETE /master/periode-buku/:id ─────────────────────────────────────────

// Delete handles DELETE /api/v1/master/periode-buku/:id.
// Permission: periode.delete (enforced via middleware)
// Soft-delete only; refuses when active references exist.
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

// ─── POST /master/periode-buku/generate ──────────────────────────────────────

// Generate handles POST /api/v1/master/periode-buku/generate.
// Permission: periode.create (enforced via middleware)
// Idempotency-Key checked by middleware.
// Generates BULANAN/TRIWULANAN/TAHUNAN rows for the given year; skips existing rows.
func (h *Handler) Generate(c *gin.Context) {
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	result, err := h.svc.Generate(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, result)
}

// ─── GET /master/periode-buku/:id/history ────────────────────────────────────

// History handles GET /api/v1/master/periode-buku/:id/history.
// Permission: periode.read (enforced via middleware)
// before/after fields only visible to audit_log.read holders.
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
// Pattern: resolve :id UUID, then forward to generic workflow handler.
// The :id is the surrogate UUID (no kode resolution needed — UUID is in path directly).

// Submit handles POST /api/v1/master/periode-buku/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "periode-buku"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/periode-buku/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "periode-buku"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/periode-buku/:id/approve.
// Step-up MFA is enforced by the workflow engine config (WORKFLOW_CONFIG_PERIODE,
// stepUpRequired.approve=true, DEC-027). Handler does not re-implement this guard.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "periode-buku"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve(c)
}

// Reject handles POST /api/v1/master/periode-buku/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "periode-buku"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/periode-buku/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "periode-buku"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseUUIDParam extracts and validates a UUID path parameter.
// Returns (uuid.Nil, false) and writes a 400 error response on failure.
// paramName is kept as a parameter for potential reuse with other param names.
func parseUUIDParam(c *gin.Context, paramName string) (uuid.UUID, bool) { //nolint:unparam
	raw := strings.TrimSpace(c.Param(paramName))
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Parameter :%s harus berupa UUID yang valid.", paramName),
			domainerrors.Detail{
				Field:   "path." + paramName,
				Rule:    "uuid",
				Message: "Format UUID tidak valid",
			},
		))
		return uuid.Nil, false
	}
	return id, true
}
