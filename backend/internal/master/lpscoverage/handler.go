// Package lpscoverage — HTTP handler layer for mst.lps_coverage (APP-A).
//
// Responsibilities of handler:
//   - Parse and validate HTTP-level inputs (path params, query params, body)
//   - Call service methods
//   - Map service results to response envelopes via common/response package
//
// Handlers must NOT contain business logic or SQL.
// Permission checks are delegated to auth.RequirePermission middleware.
//
// Workflow endpoints (submit/review/approve/approve2/reject) delegate to the
// generic workflow engine via workflow.Handler after resolving the record UUID.
// After each workflow transition the handler calls service.SyncWorkflowStatus
// to keep mst.lps_coverage.workflow_status in sync.
package lpscoverage

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

// Handler is the HTTP handler for lps_coverage endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/lps-coverage ─────────────────────────────────────────────────

// List handles GET /api/v1/master/lps-coverage.
// Permission: ecl_parameter.read (enforced via middleware)
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
	for _, lc := range result.Items {
		items = append(items, ToResponse(lc))
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

// ─── POST /master/lps-coverage ────────────────────────────────────────────────

// Create handles POST /api/v1/master/lps-coverage.
// Permission: ecl_parameter.create (enforced via middleware)
// Idempotency-Key checked by middleware.
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	lc, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, ToResponse(lc))
}

// ─── GET /master/lps-coverage/export ──────────────────────────────────────────

// Export handles GET /api/v1/master/lps-coverage/export.
// Permission: ecl_parameter.read (enforced via middleware)
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
		// XLSX export not yet implemented (Phase 3). Per compliance audit
		// pattern (lgd_basel PR #13 finding 6) — refuse explicitly rather
		// than silently serving CSV.
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"code":    "EXPORT_FORMAT_NOT_IMPLEMENTED",
				"message": "Format XLSX belum tersedia untuk LPS Coverage. Gunakan format CSV.",
			},
		})
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

	filename := fmt.Sprintf("lps-coverage-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/lps-coverage/:id ─────────────────────────────────────────────

// GetByID handles GET /api/v1/master/lps-coverage/:id.
// Permission: ecl_parameter.read (enforced via middleware)
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	lc, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(lc))
}

// ─── PUT /master/lps-coverage/:id ─────────────────────────────────────────────

// Update handles PUT /api/v1/master/lps-coverage/:id.
// Permission: ecl_parameter.update (enforced via middleware)
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

// ─── DELETE /master/lps-coverage/:id ──────────────────────────────────────────

// Delete handles DELETE /api/v1/master/lps-coverage/:id.
// Permission: ecl_parameter.delete (enforced via middleware)
// Soft-delete: sets deleted_at/deleted_by.
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

// ─── GET /master/lps-coverage/:id/history ─────────────────────────────────────

// History handles GET /api/v1/master/lps-coverage/:id/history.
// Permission: ecl_parameter.read (enforced via middleware)
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

// ─── Workflow action endpoints ─────────────────────────────────────────────────
//
// Pattern: resolve id → UUID, forward to generic workflow handler, then
// sync workflow_status back on the mst.lps_coverage row.

// Submit handles POST /api/v1/master/lps-coverage/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "lps-coverage", h.wfHandler.Submit, "SUBMIT")
}

// Review handles POST /api/v1/master/lps-coverage/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "lps-coverage", h.wfHandler.Review, "REVIEW")
}

// Approve handles POST /api/v1/master/lps-coverage/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "lps-coverage", h.wfHandler.Approve, "APPROVE")
}

// Approve2 handles POST /api/v1/master/lps-coverage/:id/approve2 (6-eyes second approver).
func (h *Handler) Approve2(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "lps-coverage", h.wfHandler.Approve2, "APPROVE2")
}

// Reject handles POST /api/v1/master/lps-coverage/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	h.delegateWorkflow(c, id, "lps-coverage", h.wfHandler.Reject, "REJECT")
}

// WorkflowStatus handles GET /api/v1/master/lps-coverage/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "lps-coverage"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

// delegateWorkflow rewrites gin params and forwards to the generic workflow handler.
// After a successful response, workflow_status is synced by the WorkflowHook
// (registered on the workflow service); no additional sync is needed here.
func (h *Handler) delegateWorkflow(c *gin.Context, entityID uuid.UUID, resource string, wfFunc gin.HandlerFunc, _ string) {
	c.Params = gin.Params{
		{Key: "resource", Value: resource},
		{Key: "id", Value: entityID.String()},
	}
	wfFunc(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseUUIDParam parses a UUID path parameter, writing a 400 error response if invalid.
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
