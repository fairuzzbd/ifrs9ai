// Package lgdbasel — HTTP handler layer for mst.lgd_basel (APP-C ECL Parameter).
//
// Handlers are intentionally thin: parse → service call → response.
// Permission checks are delegated to auth.RequirePermission middleware.
//
// Workflow signing endpoints (submit/review/approve/approve2/reject) delegate
// to the generic workflow engine via workflow.Handler. The 6-eyes path uses
// both Approve (PENDING_APPROVAL → PENDING_APPROVAL_2) and Approve2
// (PENDING_APPROVAL_2 → APPROVED). Both require step-up MFA per WORKFLOW_CONFIG_LGD_BASEL.
package lgdbasel

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

// Handler is the HTTP handler for lgd_basel endpoints.
type Handler struct {
	svc       *Service
	wfHandler *workflow.Handler
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, wfHandler *workflow.Handler) *Handler {
	return &Handler{svc: svc, wfHandler: wfHandler}
}

// ─── GET /master/lgd-basel ────────────────────────────────────────────────────

// List handles GET /api/v1/master/lgd-basel.
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

// ─── POST /master/lgd-basel ───────────────────────────────────────────────────

// Create handles POST /api/v1/master/lgd-basel.
// Permission: ecl_parameter.create (enforced via middleware)
// Idempotency-Key checked by middleware.
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

// ─── GET /master/lgd-basel/export ────────────────────────────────────────────

// Export handles GET /api/v1/master/lgd-basel/export.
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
		// fall through to CSV export below
	case "xlsx":
		// XLSX export not yet implemented for lgd_basel (Phase 3). Per
		// compliance audit BLOCKER 6 we refuse explicitly rather than
		// silently serving CSV. Track as follow-up: implement excelize-based
		// XLSX writer mirroring CSV semantics. CSV remains supported.
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"code":    "EXPORT_FORMAT_NOT_IMPLEMENTED",
				"message": "Format XLSX belum tersedia untuk LGD Basel. Gunakan format CSV.",
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

	filename := fmt.Sprintf("lgd-basel-%s.csv", time.Now().Format("20060102"))
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

// ─── GET /master/lgd-basel/:id ───────────────────────────────────────────────

// GetByID handles GET /api/v1/master/lgd-basel/:id.
// Permission: ecl_parameter.read (enforced via middleware)
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := h.parseID(c)
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

// ─── PATCH /master/lgd-basel/:id ─────────────────────────────────────────────

// Update handles PATCH /api/v1/master/lgd-basel/:id.
// Permission: ecl_parameter.update (enforced via middleware)
// Optimistic lock via rowVersion in request body.
func (h *Handler) Update(c *gin.Context) {
	id, ok := h.parseID(c)
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

// ─── DELETE /master/lgd-basel/:id ────────────────────────────────────────────

// Delete handles DELETE /api/v1/master/lgd-basel/:id.
// Permission: ecl_parameter.delete (enforced via middleware)
// Soft-delete: sets deleted_at/deleted_by.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := h.parseID(c)
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

// ─── GET /master/lgd-basel/:id/history ───────────────────────────────────────

// History handles GET /api/v1/master/lgd-basel/:id/history.
// Permission: ecl_parameter.read (enforced via middleware)
// before/after fields only visible to audit_log.read holders.
func (h *Handler) History(c *gin.Context) {
	id, ok := h.parseID(c)
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
// All workflow actions resolve :id → uuid.UUID then delegate to the generic
// workflow.Handler. For the 6-eyes path the approve endpoint transitions
// PENDING_APPROVAL → PENDING_APPROVAL_2 and the approve2 endpoint transitions
// PENDING_APPROVAL_2 → APPROVED. Both require step-up MFA.

// resolveEntityID looks up the entity UUID from the path parameter.
func (h *Handler) resolveEntityUUID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := h.parseID(c)
	if !ok {
		return uuid.Nil, false
	}
	// Verify the entity exists.
	if _, err := h.svc.GetByID(c.Request.Context(), id, false); err != nil {
		response.Error(c, err)
		return uuid.Nil, false
	}
	return id, true
}

// rewriteWorkflowParams injects the resource and id params so the generic
// workflow handler can extract them via c.Param("resource") / c.Param("id").
//
// Resource is set to "ecl-parameter" (not "lgd-basel") so that the generic
// workflow handler builds the permission string "ecl_parameter.{action}" — which
// matches the JWT claim "ecl_parameter.approve" seeded by WORKFLOW_CONFIG_LGD_BASEL.
// The auth.RequirePermission middleware already enforced the correct ecl_parameter.*
// permission before we arrive here; the workflow handler's checkPermission call
// provides a redundant safety check using the same ecl_parameter namespace.
func rewriteWorkflowParams(c *gin.Context, entityID uuid.UUID) {
	c.Params = gin.Params{
		{Key: "resource", Value: "ecl-parameter"},
		{Key: "id", Value: entityID.String()},
	}
}

// Submit handles POST /api/v1/master/lgd-basel/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/lgd-basel/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/lgd-basel/:id/approve.
// 6-eyes: PENDING_APPROVAL → PENDING_APPROVAL_2. Step-up MFA required.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.Approve(c)
}

// Approve2 handles POST /api/v1/master/lgd-basel/:id/approve2.
// 6-eyes second approval: PENDING_APPROVAL_2 → APPROVED. Step-up MFA required.
// SoD: approver2 ≠ maker ∧ ≠ reviewer ∧ ≠ approver1 (enforced by workflow engine).
func (h *Handler) Approve2(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.Approve2(c)
}

// Reject handles POST /api/v1/master/lgd-basel/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/lgd-basel/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := h.resolveEntityUUID(c)
	if !ok {
		return
	}
	rewriteWorkflowParams(c, id)
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseID parses and validates the :id path parameter as a UUID.
func (h *Handler) parseID(c *gin.Context) (uuid.UUID, bool) {
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
