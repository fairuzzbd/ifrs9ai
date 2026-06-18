package mtm

// handler.go — Thin HTTP handlers for 8 MTM endpoints.
// No business logic here — parse request → call service → map result to envelope.
//
// Endpoints:
//   GET    /trx/mtm                         → List
//   GET    /trx/mtm/alerts/stale-price      → StalePriceAlerts  (STATIC — must register before /:id)
//   GET    /trx/mtm/upload/batch/:batch_id  → GetUploadBatch    (STATIC — must register before /:id)
//   POST   /trx/mtm/upload/batch            → UploadBatch
//   POST   /trx/mtm/cron/trigger            → CronTrigger       (STATIC — must register before /:id)
//   GET    /trx/mtm/:id                     → GetByID
//   POST   /trx/mtm/:id/override-approve    → OverrideApprove
//   POST   /trx/mtm/:id/override-reject     → OverrideReject

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// HTTPHandler is the MTM HTTP handler. Depends on Service.
type HTTPHandler struct {
	svc      *Service
	enqueuer AsynqEnqueuer
}

// NewHTTPHandler creates a new MTM HTTP handler.
func NewHTTPHandler(svc *Service, enqueuer AsynqEnqueuer) *HTTPHandler {
	return &HTTPHandler{svc: svc, enqueuer: enqueuer}
}

// ─── GET /trx/mtm ─────────────────────────────────────────────────────────────

// List handles GET /api/v1/trx/mtm.
// Permission: mtm.read (ROLE-AKUN, ROLE-AUDIT, etc.)
// m2 fix: comment corrected from fx_rate.read → mtm.read.
func (h *HTTPHandler) List(c *gin.Context) {
	allCols := append(AllowedSortCols, AllowedFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	cursor := c.Query("cursor")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	rows, hasMore, total, err := h.svc.GetList(c.Request.Context(), q, cursor, limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	items := make([]ListItem, 0, len(rows))
	for _, m := range rows {
		items = append(items, ToListItem(m))
	}

	totalEstimate := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEstimate,
		Limit:         limit,
	}
	response.List(c, items, pagination, nil, nil)
}

// ─── GET /trx/mtm/:id ─────────────────────────────────────────────────────────

// GetByID handles GET /api/v1/trx/mtm/:id.
func (h *HTTPHandler) GetByID(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	m, svcErr := h.svc.GetDetail(c.Request.Context(), id)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, ToDetail(m))
}

// ─── POST /trx/mtm/upload/batch ───────────────────────────────────────────────

// UploadBatch handles POST /api/v1/trx/mtm/upload/batch.
// Permission: mtm.create (ROLE-AKUN)
// multipart/form-data: field "file" (XLSX or CSV, max 5MB).
// M4 fix: MIME type detection — rejects non-XLSX/CSV files before parsing.
// m2 fix: comment corrected from fx_rate.create → mtm.create.
func (h *HTTPHandler) UploadBatch(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)

	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		response.Error(c, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan."))
		return
	}
	uploaderID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.Error(c, domainerrors.ErrUnauthorized("sub claim bukan UUID valid."))
		return
	}

	file, _, fileErr := c.Request.FormFile("file")
	if fileErr != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' wajib: "+fileErr.Error()))
		return
	}
	defer file.Close() //nolint:errcheck

	// M4 fix: detect MIME type from first 512 bytes to prevent non-XLSX/CSV uploads.
	// XLSX is a ZIP archive; CSV is plain text. Both are detected via http.DetectContentType.
	sniff := make([]byte, 512)
	n, _ := io.ReadFull(file, sniff)
	detected := http.DetectContentType(sniff[:n])
	// XLSX (Office Open XML) is a ZIP: application/zip or application/octet-stream
	// CSV is text/plain
	validMIME := detected == "application/zip" ||
		detected == "application/octet-stream" ||
		detected == "text/plain; charset=utf-8" ||
		detected == "text/plain; charset=utf-8\x00"
	if !validMIME {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"File harus berformat XLSX atau CSV. MIME terdeteksi: "+detected))
		return
	}
	// Seek back to start so the parser can read from beginning
	if seeker, ok := file.(io.Seeker); ok {
		if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
			// Reconstruct reader from already-read bytes + remainder
			remainder := make([]byte, 0, 4*1024*1024)
			rest, _ := io.ReadAll(file)
			full := append(sniff[:n], remainder...)
			full = append(full, rest...)
			_ = full // parser not yet wired (TODO follow-up)
		}
	} else {
		// Fallback: combine sniffed + remaining bytes into buffer
		rest, _ := io.ReadAll(file)
		combined := bytes.NewReader(append(sniff[:n], rest...))
		_ = combined // parser not yet wired (TODO follow-up)
	}

	catatan := c.PostForm("catatan")

	// Parse XLSX/CSV — stub: return empty rows to demonstrate endpoint structure.
	// Real parser uses xuri/excelize; deferred to integration tests.
	var rows []UploadFileRow
	// TODO(follow-up): rows, err = parseUploadFile(file, header)

	result, svcErr := h.svc.UploadManual(c.Request.Context(), uploaderID, rows, catatan)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.Created(c, result)
}

// ─── GET /trx/mtm/upload/batch/:batch_id ─────────────────────────────────────

// GetUploadBatch handles GET /api/v1/trx/mtm/upload/batch/:batch_id.
func (h *HTTPHandler) GetUploadBatch(c *gin.Context) {
	batchID, err := parseUUID(c, "batch_id")
	if err != nil {
		response.Error(c, err)
		return
	}
	detail, svcErr := h.svc.GetUploadBatch(c.Request.Context(), batchID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, detail)
}

// ─── POST /trx/mtm/:id/override-approve ──────────────────────────────────────

// OverrideApprove handles POST /api/v1/trx/mtm/:id/override-approve.
// Permission: mtm.override (ROLE-AKUN-CTL).
// SoD: approver ≠ uploader (enforced in service).
// m2 fix: comment corrected from fx_rate.approve → mtm.override.
func (h *HTTPHandler) OverrideApprove(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req OverrideApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.OverrideApprove(c.Request.Context(), id, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /trx/mtm/:id/override-reject ───────────────────────────────────────

// OverrideReject handles POST /api/v1/trx/mtm/:id/override-reject.
// Permission: mtm.override (ROLE-AKUN-CTL).
// m2 fix: comment corrected from fx_rate.approve → mtm.override.
func (h *HTTPHandler) OverrideReject(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req OverrideRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.OverrideReject(c.Request.Context(), id, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /trx/mtm/cron/trigger ──────────────────────────────────────────────

// CronTrigger handles POST /api/v1/trx/mtm/cron/trigger.
// Permission: mtm.trigger (ROLE-AKUN or ROLE-IT-ADMIN).
// B3 fix: comment corrected from fx_rate.create → mtm.trigger.
// B4 fix: SensitiveRateLimit applied at route registration level (routes.go).
func (h *HTTPHandler) CronTrigger(c *gin.Context) {
	var req CronTriggerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Empty body OK (use defaults)
		req = CronTriggerRequest{}
	}
	result, svcErr := h.svc.TriggerCron(c.Request.Context(), h.enqueuer, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.Accepted(c, result)
}

// ─── GET /trx/mtm/alerts/stale-price ─────────────────────────────────────────

// StalePriceAlerts handles GET /api/v1/trx/mtm/alerts/stale-price.
// Permission: mtm.read.
// m2 fix: comment corrected from fx_rate.read → mtm.read.
func (h *HTTPHandler) StalePriceAlerts(c *gin.Context) {
	cursor := c.Query("cursor")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	items, hasMore, total, err := h.svc.GetStalePriceAlerts(c.Request.Context(), cursor, limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	totalEstimate := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEstimate,
		Limit:         limit,
	}
	response.List(c, items, pagination, nil, nil)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// parseUUID parses a UUID path param by name, returning DomainError on failure.
func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter '"+param+"' harus berupa UUID v4 yang valid.")
	}
	return id, nil
}
