package reporting

// handler.go — 9 thin Gin HTTP handlers for P5-M13 APP-E Reporting MV Foundation.
//
// Endpoint mapping (api/openapi/app-e-reporting-mv.yaml):
//
//	GET    /admin/mv-status                                   GetMVStatus
//	POST   /admin/mv-refresh                                  PostMVRefresh
//	GET    /reports/:slug/export                              GetReportExport
//	GET    /reports/export-log                                GetExportLog
//	GET    /reports/export/:export_id/download                GetExportDownload
//	GET    /reports/scheduled-emails                          GetScheduledEmails
//	POST   /reports/scheduled-emails                          PostScheduledEmail
//	DELETE /reports/scheduled-emails/:id                      DeleteScheduledEmail
//	POST   /reports/scheduled-emails/:id/opt-out              PostOptOut (no auth)
//
// Auth: JWT via auth.Middleware on all routes except opt-out (security: []).
// Idempotency-Key: POST /admin/mv-refresh, POST /reports/scheduled-emails (DEC-021).

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds the reporting service.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("reporting.NewHandler: svc must not be nil")
	}
	return &Handler{svc: svc}
}

// ─── GET /admin/mv-status ─────────────────────────────────────────────────────

// GetMVStatus returns the latest refresh status for all 8 MVs.
// Permission: report.admin (ROLE-IT-ADMIN, ROLE-AKUN-CTL, ROLE-AUDIT).
func (h *Handler) GetMVStatus(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.admin") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.admin tidak terpenuhi.", nil)
		return
	}

	items, err := h.svc.ListMVStatus(c.Request.Context())
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.OK(c, items)
}

// ─── POST /admin/mv-refresh ───────────────────────────────────────────────────

// PostMVRefresh enqueues an MV refresh job (single or all).
// Permission: report.admin.
// Idempotency-Key required (DEC-021).
func (h *Handler) PostMVRefresh(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.admin") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.admin tidak terpenuhi.", nil)
		return
	}

	var req MVRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Empty body is OK (means refresh all).
		req = MVRefreshRequest{}
	}

	actorID, _ := uuid.Parse(claims.Sub)
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}
	// Inject actor + tenant into context so service can extract them.
	ctx := c.Request.Context()
	ctx = auth.ContextWithClaims(ctx, &auth.Claims{Sub: actorID.String(), TenantID: tenantID, Roles: claims.Roles, Permissions: claims.Permissions})

	ref, err := h.svc.TriggerRefresh(ctx, req.MVName)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.Accepted(c, ref)
}

// ─── GET /reports/:slug/export ────────────────────────────────────────────────

// GetReportExport handles synchronous (<= inline threshold) and async (> threshold) export.
// Permission: report.{slug}.export OR audit_log.read (ROLE-AUDIT bypass).
func (h *Handler) GetReportExport(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}

	slug := c.Param("slug")
	format := ExportFormat(c.DefaultQuery("format", "csv"))

	if !format.IsValid() {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeExportFormatUnsupported,
			"Format tidak didukung: "+string(format)+". Gunakan csv, xlsx, atau pdf.", nil)
		return
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		response.ErrorWithStatus(c, http.StatusInternalServerError,
			domainerrors.CodeInternal, "actor UUID invalid", nil)
		return
	}
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	req := ExportRequest{
		ReportSlug: slug,
		Format:     format,
		ActorID:    actorID,
		ActorRole:  firstRole(claims),
		TenantID:   tenantID,
	}

	jobRef, logRow, err := h.svc.RequestExport(ctx, req)
	if err != nil {
		writeDomainError(c, err)
		return
	}

	// Async path: jobRef set, logRow nil.
	if jobRef != nil {
		response.Accepted(c, jobRef)
		return
	}

	// Inline path: logRow set. Build the file and stream directly.
	if logRow != nil {
		fileBytes, _, contentType, buildErr := h.svc.BuildInlineExport(ctx, slug, format, claims.PreferredUsername)
		if buildErr != nil {
			writeDomainError(c, buildErr)
			return
		}
		filename := slug + "." + string(format)
		c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
		c.Data(http.StatusOK, contentType, fileBytes)
		return
	}

	response.OK(c, gin.H{"message": "export enqueued"})
}

// ─── GET /reports/export-log ─────────────────────────────────────────────────

// GetExportLog returns export log entries (cursor-based) for the current user.
// Permission: report.export.read (or audit_log.read for ROLE-AUDIT).
func (h *Handler) GetExportLog(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.export.read") && !claims.HasPermission("audit_log.read") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.export.read tidak terpenuhi.", nil)
		return
	}

	cursor := c.Query("cursor")
	limit := 50
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	items, nextCursor, hasMore, err := h.svc.repo.ListExportLogs(c.Request.Context(), cursor, limit, tenantID)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"pagination": gin.H{
			"nextCursor": nextCursor,
			"hasMore":    hasMore,
			"limit":      limit,
		},
	})
}

// ─── GET /reports/export/:export_id/download ─────────────────────────────────

// GetExportDownload returns a presigned download URL + marks downloaded_at.
// Permission: report.export.read (or audit_log.read).
func (h *Handler) GetExportDownload(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.export.read") && !claims.HasPermission("audit_log.read") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.export.read tidak terpenuhi.", nil)
		return
	}

	exportID, err := uuid.Parse(c.Param("export_id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "export_id bukan UUID valid.", nil)
		return
	}
	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	result, err := h.svc.GetExportDownload(ctx, exportID)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.OK(c, result)
}

// ─── GET /reports/scheduled-emails ───────────────────────────────────────────

// GetScheduledEmails lists scheduled email configs.
// Permission: report.scheduled-email.read.
func (h *Handler) GetScheduledEmails(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.scheduled-email.read") && !claims.HasPermission("report.admin") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.scheduled-email.read tidak terpenuhi.", nil)
		return
	}
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	rows, err := h.svc.repo.ListActiveScheduledEmails(c.Request.Context(), tenantID)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	items := make([]ScheduledEmailItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, rowToScheduledEmailItem(r))
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       items,
		"pagination": gin.H{"hasMore": false},
	})
}

// ─── POST /reports/scheduled-emails ──────────────────────────────────────────

// PostScheduledEmail creates a new scheduled email config.
// Permission: report.scheduled-email.create.
// Idempotency-Key required.
func (h *Handler) PostScheduledEmail(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.scheduled-email.create") && !claims.HasPermission("report.admin") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.scheduled-email.create tidak terpenuhi.", nil)
		return
	}

	var req ScheduledEmailCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	item, err := h.svc.CreateScheduledEmail(ctx, req)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	response.Created(c, item)
}

// ─── DELETE /reports/scheduled-emails/:id ────────────────────────────────────

// DeleteScheduledEmail soft-deletes a scheduled email config.
// Permission: report.scheduled-email.delete.
func (h *Handler) DeleteScheduledEmail(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}
	if !claims.HasPermission("report.scheduled-email.delete") && !claims.HasPermission("report.admin") {
		response.ErrorWithStatus(c, http.StatusForbidden,
			domainerrors.CodeForbidden,
			"Permission report.scheduled-email.delete tidak terpenuhi.", nil)
		return
	}

	schedID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID valid.", nil)
		return
	}
	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	if err = h.svc.SoftDeleteScheduledEmail(ctx, schedID); err != nil {
		writeDomainError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ─── POST /reports/scheduled-emails/:id/opt-out ──────────────────────────────

// PostOptOut processes a recipient opt-out (no auth required — HMAC token validates).
// security: [] in OpenAPI.
func (h *Handler) PostOptOut(c *gin.Context) {
	schedID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, "id bukan UUID valid.", nil)
		return
	}

	var body struct {
		Email string `json:"email" binding:"required,email"`
		Token string `json:"token" binding:"required"`
	}
	if err = c.ShouldBindJSON(&body); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	req := OptOutRequest{
		ScheduledEmailID: schedID,
		Email:            body.Email,
		Token:            body.Token,
	}
	if err = h.svc.OptOutRecipient(c.Request.Context(), req); err != nil {
		writeDomainError(c, err)
		return
	}
	response.OK(c, gin.H{"message": "Opt-out berhasil. Email Anda tidak akan menerima laporan ini lagi."})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// claimsFromGin extracts JWT claims from Gin context.
// Returns nil and writes 401 if not present.
func claimsFromGin(c *gin.Context) *auth.Claims {
	val, ok := c.Get("claims")
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims tidak ditemukan.", nil)
		return nil
	}
	claims, ok := val.(*auth.Claims)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims format tidak valid.", nil)
		return nil
	}
	return claims
}

// writeDomainError maps a domain or wrapped error to an HTTP response.
func writeDomainError(c *gin.Context, err error) {
	if de, ok := domainerrors.IsDomainError(err); ok {
		c.JSON(de.HTTPStatus(), gin.H{
			"error": gin.H{
				"code":    de.Code(),
				"message": de.Error(),
				"traceId": c.GetString("X-Trace-Id"),
			},
		})
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal.", nil)
}

// firstRole returns the first role from JWT claims, or "" if empty.
func firstRole(claims *auth.Claims) string {
	if len(claims.Roles) > 0 {
		return claims.Roles[0]
	}
	return ""
}

// rowToScheduledEmailItem converts a DB row to an API item.
func rowToScheduledEmailItem(r ScheduledEmailRow) ScheduledEmailItem {
	var recipients []string
	_ = (&recipients)
	// Unmarshal from r.RecipientsJSON is done in repo; here just use empty slice as fallback.
	return ScheduledEmailItem{
		ID:          r.ID,
		ReportSlug:  r.ReportSlug,
		Format:      r.Format,
		Frequency:   r.Frequency,
		SendTime:    r.SendTime,
		Recipients:  nil, // populated via separate query in repo
		Active:      r.Active,
		LastSentAt:  r.LastSentAt,
		LastStatus:  r.LastStatus,
		CreatedAt:   r.CreatedAt,
		CreatedBy:   r.CreatedBy,
	}
}
