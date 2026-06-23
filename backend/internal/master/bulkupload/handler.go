package bulkupload

// handler.go — Thin HTTP handlers for bulk upload master instrumen (P5-M11).
// No business logic — parse request → call service → map result to response envelope.
//
// Endpoints (all under /api/v1/master/instrumen/bulk-upload):
//   POST   /                          → UploadBatch     (bulkupload.create)
//   GET    /:batch_id                 → GetBatch        (bulkupload.read)
//   POST   /:batch_id/dry-run        → DryRun          (bulkupload.create)
//   POST   /:batch_id/commit         → Commit          (bulkupload.create) enqueues Asynq
//   POST   /:batch_id/approve        → Approve         (bulkupload.approve) ROLE-APPR-TR
//   POST   /:batch_id/rollback-request → RollbackRequest (bulkupload.rollback) ROLE-CFO
//   POST   /:batch_id/rollback-approve → RollbackApprove (bulkupload.rollback) ROLE-CFO step-up

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// maxUploadSizeHTTP is the max multipart read size enforced at HTTP layer (50MB + 1MB overhead).
const maxUploadSizeHTTP = 51 * 1024 * 1024

// AllowedBatchRowsSortCols defines allowed sort columns for GET /:batch_id.
var AllowedBatchRowsSortCols = []string{"row_number", "sheet_name", "row_status", "created_at"}

// AllowedBatchRowsFilterCols defines allowed filter columns for GET /:batch_id.
var AllowedBatchRowsFilterCols = []string{"row_status", "sheet_name"}

// HTTPHandler is the bulk upload HTTP handler.
type HTTPHandler struct {
	svc         *Service
	asynqClient *asynq.Client // must be non-nil in production (M9 B2 lesson)
}

// NewHTTPHandler creates a new bulk upload HTTPHandler.
// asynqClient must be a real *asynq.Client (not nil) in production — POST
// /:batch_id/commit returns 501 if asynqClient is nil.
func NewHTTPHandler(svc *Service, asynqClient *asynq.Client) *HTTPHandler {
	return &HTTPHandler{svc: svc, asynqClient: asynqClient}
}

// actorAndTenant extracts actorID + tenantID from Gin context.
// Falls back to uuid.Nil / "TUGURE" for unit test scenarios without auth middleware.
func actorAndTenant(c *gin.Context) (uuid.UUID, string) {
	claims := auth.ClaimsFromGin(c)
	if claims == nil {
		return uuid.Nil, "TUGURE"
	}
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		actorID = uuid.Nil
	}
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}
	return actorID, tenantID
}

// parseBatchID parses :batch_id path param.
func parseBatchID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("batch_id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "batch_id bukan UUID valid"))
		return uuid.Nil, false
	}
	return id, true
}

// ─── POST /master/instrumen/bulk-upload ──────────────────────────────────────

// UploadBatch handles multipart XLSX upload.
// S1-AC2: server reads the file body (not Content-Length) for size validation.
// S1-AC3: MIME magic byte check happens in Service.UploadBatch.
func (h *HTTPHandler) UploadBatch(c *gin.Context) {
	// Parse multipart — size limit enforced server-side (S1-AC2)
	if err := c.Request.ParseMultipartForm(maxUploadSizeHTTP); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Gagal parse multipart: "+err.Error()))
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' tidak ditemukan di request multipart"))
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeInternal, "Gagal membaca file upload"))
		return
	}

	actorID, tenantID := actorAndTenant(c)

	result, svcErr := h.svc.UploadBatch(c.Request.Context(), header.Filename, fileData, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	response.Created(c, result)
}

// ─── GET /master/instrumen/bulk-upload/:batch_id ──────────────────────────────

// GetBatch returns batch detail with row breakdown (paginated).
func (h *HTTPHandler) GetBatch(c *gin.Context) {
	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	allCols := append(AllowedBatchRowsSortCols, AllowedBatchRowsFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	_, tenantID := actorAndTenant(c)

	batch, svcErr := h.svc.GetBatch(c.Request.Context(), batchID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	if batch == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeNotFound, "Batch tidak ditemukan"))
		return
	}

	rows, pag, svcErr := h.svc.ListBatchRows(c.Request.Context(), batchID, q, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	response.OK(c, gin.H{
		"batch": batch,
		"rows":  rows,
		"pagination": response.PaginationMeta{
			NextCursor:    pag.NextCursor,
			HasMore:       pag.HasMore,
			TotalEstimate: pag.TotalEstimate,
			Limit:         pag.Limit,
		},
	})
}

// ─── POST /master/instrumen/bulk-upload/:batch_id/dry-run ─────────────────────

// DryRun runs 4-stage validation pipeline and caches result for 1 hour (S2).
func (h *HTTPHandler) DryRun(c *gin.Context) {
	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	actorID, tenantID := actorAndTenant(c)

	result, svcErr := h.svc.DryRun(c.Request.Context(), batchID, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	response.OK(c, result)
}

// ─── POST /master/instrumen/bulk-upload/:batch_id/commit ─────────────────────

// Commit enqueues the Asynq bulkupload:commit_instrumen task (S3).
// Returns 202 with jobId + statusUrl + streamUrl.
func (h *HTTPHandler) Commit(c *gin.Context) {
	if h.asynqClient == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": "Asynq client tidak terkonfigurasi — wire real client di main.go (P5-M11 M9 B2 lesson)",
			},
		})
		return
	}

	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	actorID, tenantID := actorAndTenant(c)

	jobID, svcErr := h.svc.Commit(c.Request.Context(), batchID, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	task, err := NewCommitTask(batchID, actorID, tenantID, jobID)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeInternal, "Gagal membuat Asynq task: "+err.Error()))
		return
	}

	if _, err := h.asynqClient.Enqueue(task); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeInternal, "Gagal enqueue Asynq task: "+err.Error()))
		return
	}

	response.Accepted(c, gin.H{
		"jobId":     jobID.String(),
		"batchId":   batchID.String(),
		"type":      "BULK_COMMIT_INSTRUMEN",
		"statusUrl": "/api/v1/jobs/" + jobID.String(),
		"streamUrl": "/api/v1/jobs/" + jobID.String() + "/stream",
		"startedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

// ─── POST /master/instrumen/bulk-upload/:batch_id/approve ─────────────────────

// Approve activates committed instruments (S4).
// SoD: approver ≠ maker. Validated in service.Approve.
// ROLE-APPR-TR required (enforced by RequirePermission in routes.go).
func (h *HTTPHandler) Approve(c *gin.Context) {
	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	var req ApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	actorID, tenantID := actorAndTenant(c)

	result, svcErr := h.svc.Approve(c.Request.Context(), batchID, req, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	response.OK(c, result)
}

// ─── POST /master/instrumen/bulk-upload/:batch_id/rollback-request ────────────

// RollbackRequest submits CFO rollback request with reason ≥ 50 chars (S5-AC1).
// ROLE-CFO required (enforced by RequirePermission in routes.go).
func (h *HTTPHandler) RollbackRequest(c *gin.Context) {
	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	var body RollbackRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	actorID, tenantID := actorAndTenant(c)

	if err := h.svc.RollbackRequest(c.Request.Context(), batchID, body, actorID, tenantID); err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, gin.H{
		"batchId": batchID.String(),
		"status":  string(StatusRollbackPending),
		"message": "Permintaan rollback berhasil diajukan. Menunggu persetujuan CFO.",
	})
}

// ─── POST /master/instrumen/bulk-upload/:batch_id/rollback-approve ────────────

// RollbackApprove confirms CFO rollback with step-up MFA (S5-AC3, DEC-027).
// Step-up freshness (≤ 5 min) enforced via RequireStepUpMiddleware in routes.go.
// ROLE-CFO required.
func (h *HTTPHandler) RollbackApprove(c *gin.Context) {
	batchID, ok := parseBatchID(c)
	if !ok {
		return
	}

	var body RollbackApproveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	// Step-up freshness double-check (DEC-027 defense-in-depth beyond middleware)
	claims := auth.ClaimsFromGin(c)
	if claims != nil && claims.NeedsStepUp() {
		response.Error(c, domainerrors.New(domainerrors.CodeForbidden,
			"Rollback approve membutuhkan step-up MFA yang masih valid (< 5 menit). Hubungi /auth/step-up."))
		return
	}

	actorID, tenantID := actorAndTenant(c)

	result, svcErr := h.svc.RollbackApprove(c.Request.Context(), batchID, body, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	response.OK(c, result)
}
