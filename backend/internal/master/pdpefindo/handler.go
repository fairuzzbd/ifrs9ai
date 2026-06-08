// Package pdpefindo — HTTP handler layer for mst.pd_pefindo (APP-A ECL Param, MSTR-PDPefindo).
//
// Handlers are thin: parse → service call → response envelope.
// No business logic or SQL in handlers.
// Permission checks are delegated to auth.RequirePermission middleware.
package pdpefindo

import (
	"encoding/json"
	"fmt"
	"io"
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

const (
	maxUploadBytes = 10 * 1024 * 1024 // 10 MB
)

// Handler is the HTTP handler for pd_pefindo endpoints.
type Handler struct {
	svc           *Service
	uploadSvc     *UploadService
	wfHandler     *workflow.Handler
	asynqEnqueuer AsynqEnqueuer // nil = sync fallback in dev
}

// NewHandler creates a Handler.
func NewHandler(svc *Service, uploadSvc *UploadService, wfHandler *workflow.Handler, enqueuer AsynqEnqueuer) *Handler {
	return &Handler{svc: svc, uploadSvc: uploadSvc, wfHandler: wfHandler, asynqEnqueuer: enqueuer}
}

// ─── GET /master/pd-pefindo ───────────────────────────────────────────────────

// List handles GET /api/v1/master/pd-pefindo.
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

// ─── POST /master/pd-pefindo ──────────────────────────────────────────────────

// Create handles POST /api/v1/master/pd-pefindo.
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

// ─── GET /master/pd-pefindo/export ───────────────────────────────────────────

// Export handles GET /api/v1/master/pd-pefindo/export?format=csv.
// XLSX export returns 501 (not implemented — requires async job for large datasets).
func (h *Handler) Export(c *gin.Context) {
	q, err := listquery.ParseFromRequest(c.Request, AllAllowedCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	if format == "xlsx" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Export format XLSX belum diimplementasi. Gunakan format=csv.",
		))
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

	filename := fmt.Sprintf("pd-pefindo-%s.csv", time.Now().Format("20060102"))
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

// ─── POST /master/pd-pefindo/upload-xlsx ─────────────────────────────────────

// UploadXLSX handles POST /api/v1/master/pd-pefindo/upload-xlsx.
// Accepts multipart/form-data with fields: file, tanggal_publikasi,
// periode_berlaku_dari, periode_berlaku_sampai (optional).
// Returns 202 Accepted with job tracking info.
// Permission: ecl_parameter.submit (same as maker).
func (h *Handler) UploadXLSX(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(maxUploadBytes); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"File terlalu besar atau form tidak valid (max 10 MB): "+err.Error()))
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' wajib diisi (multipart/form-data).",
			domainerrors.Detail{Field: "file", Rule: "required", Message: "File XLSX wajib diupload"},
		))
		return
	}
	if fileHeader.Size > maxUploadBytes {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("File terlalu besar: %d bytes (max %d bytes)", fileHeader.Size, maxUploadBytes),
		))
		return
	}

	// Validate extension.
	lowerName := strings.ToLower(fileHeader.Filename)
	if !strings.HasSuffix(lowerName, ".xlsx") && !strings.HasSuffix(lowerName, ".xls") {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"File harus berformat XLSX (.xlsx atau .xls).",
		))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeInternal,
			"Gagal membuka file upload."))
		return
	}
	defer f.Close()

	fileContent, err := io.ReadAll(f)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeInternal,
			"Gagal membaca file upload."))
		return
	}

	tanggalPub := c.PostForm("tanggal_publikasi")
	periodeDari := c.PostForm("periode_berlaku_dari")

	if tanggalPub == "" || periodeDari == "" {
		var details []domainerrors.Detail
		if tanggalPub == "" {
			details = append(details, domainerrors.Detail{
				Field: "tanggal_publikasi", Rule: "required",
				Message: "tanggal_publikasi wajib diisi",
			})
		}
		if periodeDari == "" {
			details = append(details, domainerrors.Detail{
				Field: "periode_berlaku_dari", Rule: "required",
				Message: "periode_berlaku_dari wajib diisi",
			})
		}
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field wajib tidak lengkap", details...))
		return
	}

	var periodeSampai *string
	if ps := c.PostForm("periode_berlaku_sampai"); ps != "" {
		periodeSampai = &ps
	}

	req := XLSXUploadRequest{
		FileContent:          fileContent,
		FileName:             fileHeader.Filename,
		TanggalPublikasi:     tanggalPub,
		PeriodeBerlakuDari:   periodeDari,
		PeriodeBerlakuSampai: periodeSampai,
	}

	result, err := h.uploadSvc.SubmitUploadJob(c.Request.Context(), req, h.asynqEnqueuer)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(202, gin.H{
		"data": result,
		"meta": gin.H{},
	})
}

// ─── GET /master/pd-pefindo/upload-jobs/:jobId ───────────────────────────────

// GetUploadJobStatus handles GET /api/v1/master/pd-pefindo/upload-jobs/:jobId.
func (h *Handler) GetUploadJobStatus(c *gin.Context) {
	jobID := c.Param("jobId")
	if jobID == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter jobId wajib diisi."))
		return
	}

	j, err := h.svc.GetJobStatus(c.Request.Context(), jobID)
	if err != nil {
		response.Error(c, err)
		return
	}

	resp := toJobStatusResponse(j)
	response.OK(c, resp)
}

// ─── GET /master/pd-pefindo/:id ───────────────────────────────────────────────

// GetByID handles GET /api/v1/master/pd-pefindo/:id.
func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}

	p, err := h.svc.GetByID(c.Request.Context(), id, false)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, ToResponse(p))
}

// ─── PATCH /master/pd-pefindo/:id ────────────────────────────────────────────

// Update handles PATCH /api/v1/master/pd-pefindo/:id.
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

// ─── DELETE /master/pd-pefindo/:id ───────────────────────────────────────────

// Delete handles DELETE /api/v1/master/pd-pefindo/:id.
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

// ─── GET /master/pd-pefindo/:id/history ──────────────────────────────────────

// History handles GET /api/v1/master/pd-pefindo/:id/history.
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

// Submit handles POST /api/v1/master/pd-pefindo/:id/submit.
func (h *Handler) Submit(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Submit(c)
}

// Review handles POST /api/v1/master/pd-pefindo/:id/review.
func (h *Handler) Review(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Review(c)
}

// Approve handles POST /api/v1/master/pd-pefindo/:id/approve.
func (h *Handler) Approve(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve(c)
}

// Approve2 handles POST /api/v1/master/pd-pefindo/:id/approve2 (6-eyes second approver).
func (h *Handler) Approve2(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Approve2(c)
}

// Reject handles POST /api/v1/master/pd-pefindo/:id/reject.
func (h *Handler) Reject(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.Reject(c)
}

// WorkflowStatus handles GET /api/v1/master/pd-pefindo/:id/workflow.
func (h *Handler) WorkflowStatus(c *gin.Context) {
	id, ok := parseUUID(c)
	if !ok {
		return
	}
	c.Params = gin.Params{
		{Key: "resource", Value: "pd-pefindo"},
		{Key: "id", Value: id.String()},
	}
	h.wfHandler.GetStatus(c)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func parseUUID(c *gin.Context) (uuid.UUID, bool) {
	const param = "id"
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter '"+param+"' harus berformat UUID.",
			domainerrors.Detail{Field: "path." + param, Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return uuid.Nil, false
	}
	return id, true
}

func toJobStatusResponse(j *JobRow) JobStatusResponse {
	r := JobStatusResponse{
		JobID:       j.ID,
		Type:        j.Type,
		Status:      j.Status,
		Progress:    j.Progress,
		CurrentStep: j.CurrentStep,
		CanCancel:   j.CanCancel,
	}
	if j.StartedAt != nil {
		s := j.StartedAt.Format(time.RFC3339)
		r.StartedAt = &s
	}
	if len(j.ResultJSON) > 0 {
		var result interface{}
		if err := json.Unmarshal(j.ResultJSON, &result); err == nil {
			r.Result = result
		}
	}
	if len(j.ErrorJSON) > 0 {
		var errData interface{}
		if err := json.Unmarshal(j.ErrorJSON, &errData); err == nil {
			r.Error = errData
		}
	}
	return r
}
