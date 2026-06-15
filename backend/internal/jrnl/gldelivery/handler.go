package gldelivery

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler implements the 9 REST endpoints for P5-M3 GL Delivery.
type Handler struct {
	delivery *DeliveryService
	dlq      *DLQService
	recon    *ReconciliationService
}

// NewHandler creates a Handler. Panics on nil deps.
func NewHandler(delivery *DeliveryService, dlq *DLQService, recon *ReconciliationService) *Handler {
	if delivery == nil {
		panic("gldelivery.NewHandler: delivery must not be nil")
	}
	if dlq == nil {
		panic("gldelivery.NewHandler: dlq must not be nil")
	}
	if recon == nil {
		panic("gldelivery.NewHandler: recon must not be nil")
	}
	return &Handler{delivery: delivery, dlq: dlq, recon: recon}
}

// ─── 1. GET /jurnal/header/:id/gl-delivery-status ────────────────────────────

// GetDeliveryStatus returns current GL delivery state for a jurnal header.
// Permission: jurnal.gl_delivery.read.
func (h *Handler) GetDeliveryStatus(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryRead) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryRead))
		return
	}

	headerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "invalid header ID format"))
		return
	}

	ds, err := h.delivery.jurnalRepo.GetDeliveryStatus(c.Request.Context(), headerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	if ds == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound,
			"gl_status not found for header "+headerID.String()))
		return
	}

	ds.CanRetry = ds.GlHostStatus.CanManualRetry()

	// Redact raw response payload unless caller has read_raw permission.
	if !claims.HasPermission(PermGlDeliveryReadRaw) {
		ds.GlResponsePayloadJsonb = nil
	}

	response.OK(c, ds)
}

// ─── 2. POST /jurnal/header/:id/retry-gl-delivery ────────────────────────────

// RetryGLDelivery manually re-queues a failed GL delivery.
// Permission: jurnal.gl_delivery.retry.
func (h *Handler) RetryGLDelivery(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryRetry) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryRetry))
		return
	}

	headerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "invalid header ID format"))
		return
	}

	callerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeUnauthorized, "invalid actor UUID"))
		return
	}

	var req RetryGlDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.delivery.ManualRetry(c.Request.Context(), headerID, req.Reason, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": result, "meta": gin.H{"traceId": c.GetString(response.TraceIDKey)}})
}

// ─── 3. GET /jurnal/gl-delivery-dlq ──────────────────────────────────────────

// ListDLQ returns paginated DLQ entries.
// Permission: jurnal.gl_delivery.read.
func (h *Handler) ListDLQ(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryRead) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryRead))
		return
	}

	limit := 50
	statusFilter := c.DefaultQuery("status", "FAILED")

	items, page, err := h.dlq.List(c.Request.Context(), limit, statusFilter)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, items, page.PaginationMeta(), nil, nil)
}

// ─── 4. GET /jurnal/gl-delivery-dlq/:id ──────────────────────────────────────

// GetDLQEntry returns a full DLQ entry by ID.
// Permission: jurnal.gl_delivery.read.
func (h *Handler) GetDLQEntry(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryRead) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryRead))
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "invalid DLQ entry ID format"))
		return
	}

	entry, err := h.dlq.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if entry == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound,
			"DLQ entry not found: "+id.String()))
		return
	}

	// Redact raw payload unless read_raw permission.
	if !claims.HasPermission(PermGlDeliveryReadRaw) {
		entry.PayloadJsonb = nil
	}

	response.OK(c, entry)
}

// ─── 5. POST /jurnal/gl-delivery-dlq/:id/replay ──────────────────────────────

// ReplayDLQEntry triggers re-delivery for a FAILED DLQ entry.
// Permission: jurnal.gl_delivery.replay.
func (h *Handler) ReplayDLQEntry(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryReplay) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryReplay))
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "invalid DLQ entry ID format"))
		return
	}

	callerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeUnauthorized, "invalid actor UUID"))
		return
	}

	var req DlqActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.dlq.Replay(c.Request.Context(), id, req.Reason, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": result, "meta": gin.H{"traceId": c.GetString(response.TraceIDKey)}})
}

// ─── 6. POST /jurnal/gl-delivery-dlq/:id/discard ─────────────────────────────

// DiscardDLQEntry moves FAILED → ABANDONED + gl_status → DEAD_LETTER.
// Permission: jurnal.gl_delivery.discard.
func (h *Handler) DiscardDLQEntry(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermGlDeliveryDiscard) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermGlDeliveryDiscard))
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "invalid DLQ entry ID format"))
		return
	}

	callerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeUnauthorized, "invalid actor UUID"))
		return
	}

	var req DlqActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	result, err := h.dlq.Discard(c.Request.Context(), id, req.Reason, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, result)
}

// ─── 7. POST /jurnal/reconciliation/run ──────────────────────────────────────

// RunReconciliation triggers an async reconciliation job for a given date.
// Permission: jurnal.reconciliation.run.
func (h *Handler) RunReconciliation(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermReconciliationRun) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermReconciliationRun))
		return
	}

	callerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeUnauthorized, "invalid actor UUID"))
		return
	}

	var req RunReconciliationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	date, parseErr := time.Parse("2006-01-02", req.Date)
	if parseErr != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeGLReconciliationDateInvalid,
			"date must be YYYY-MM-DD format"))
		return
	}

	tenantID := "TUGURE"
	if claims.TenantID != "" {
		tenantID = claims.TenantID
	}

	result, err := h.recon.TriggerAsync(c.Request.Context(), date, "MANUAL", &callerID, tenantID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Accepted(c, result)
}

// ─── 8. GET /jurnal/reconciliation/:date ─────────────────────────────────────

// GetReconciliationReport returns a reconciliation report for a specific date.
// Permission: jurnal.reconciliation.read.
func (h *Handler) GetReconciliationReport(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermReconciliationRead) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermReconciliationRead))
		return
	}

	date, err := time.Parse("2006-01-02", c.Param("date"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeGLReconciliationDateInvalid,
			"date path param must be YYYY-MM-DD"))
		return
	}

	tenantID := "TUGURE"
	if claims.TenantID != "" {
		tenantID = claims.TenantID
	}

	report, err := h.recon.GetReport(c.Request.Context(), date, tenantID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, report)
}

// ─── 9. GET /jurnal/reconciliation/history ────────────────────────────────────

// ListReconciliationHistory returns paginated reconciliation report summaries.
// Permission: jurnal.reconciliation.read.
func (h *Handler) ListReconciliationHistory(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil || !claims.HasPermission(PermReconciliationRead) {
		response.Error(c, domainerrors.New(domainerrors.CodeGLDeliveryPermissionDenied,
			"permission required: "+PermReconciliationRead))
		return
	}

	limit := 50
	statusFilter := c.DefaultQuery("status", "")

	items, page, err := h.recon.ListReports(c.Request.Context(), limit, statusFilter)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.List(c, items, page.PaginationMeta(), nil, nil)
}
