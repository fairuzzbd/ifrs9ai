package mappingjurnal

// p5m12_handler.go — HTTP handlers for P5-M12 endpoints.
//
// New endpoints (in addition to existing handler.go):
//   POST /mapping-jurnal/{event_code}/new-version
//   POST /mapping-jurnal/{event_code}/version/{version_id}/submit
//   POST /mapping-jurnal/{event_code}/version/{version_id}/review
//   POST /mapping-jurnal/{event_code}/version/{version_id}/approve
//   POST /mapping-jurnal/{event_code}/version/{version_id}/approve-2
//   POST /mapping-jurnal/{event_code}/version/{version_id}/reject
//   POST /mapping-jurnal/bulk-import
//   GET  /reports/rpt-19-mapping-coverage
//   GET  /reports/rpt-19-mapping-coverage/export
//   GET  /reports/rpt-20-mapping-validation
//   GET  /reports/rpt-21-mapping-history
//   GET  /reports/rpt-21-mapping-history/export
//
// Thin handler: only parse input, call service, shape response. No SQL here.
// Idempotency-Key: checked by middleware (idempotency.Middleware) for POST endpoints.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// P5M12Handler handles P5-M12 specific endpoints.
type P5M12Handler struct {
	svc *P5M12Service
}

// NewP5M12Handler creates a P5M12Handler.
func NewP5M12Handler(svc *P5M12Service) *P5M12Handler {
	return &P5M12Handler{svc: svc}
}

// ─── POST /mapping-jurnal/{event_code}/new-version ───────────────────────────

// NewVersion handles POST /api/v1/master/mapping-jurnal/{event_code}/new-version.
func (h *P5M12Handler) NewVersion(c *gin.Context) {
	eventCode := c.Param("event_code")
	if eventCode == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "event_code wajib diisi di path"))
		return
	}

	var req NewVersionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, err := h.svc.CreateNewVersion(c.Request.Context(), eventCode, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, result)
}

// ─── POST /mapping-jurnal/{event_code}/version/{version_id}/submit ───────────

// Submit handles POST .../submit.
func (h *P5M12Handler) Submit(c *gin.Context) {
	eventCode, versionID, err := parseEventCodeVersionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req P5SubmitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, svcErr := h.svc.P5Submit(c.Request.Context(), eventCode, versionID, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /mapping-jurnal/{event_code}/version/{version_id}/review ───────────

// Review handles POST .../review.
func (h *P5M12Handler) Review(c *gin.Context) {
	eventCode, versionID, err := parseEventCodeVersionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req P5ReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, svcErr := h.svc.P5Review(c.Request.Context(), eventCode, versionID, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /mapping-jurnal/{event_code}/version/{version_id}/approve ──────────

// Approve handles POST .../approve (4-eyes).
func (h *P5M12Handler) Approve(c *gin.Context) {
	eventCode, versionID, err := parseEventCodeVersionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req P5ApproveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, svcErr := h.svc.P5Approve(c.Request.Context(), eventCode, versionID, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /mapping-jurnal/{event_code}/version/{version_id}/approve-2 ────────

// Approve2 handles POST .../approve-2 (6-eyes regulated).
// DEC-027: X-Step-Up-Token header required.
func (h *P5M12Handler) Approve2(c *gin.Context) {
	eventCode, versionID, err := parseEventCodeVersionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	stepUpToken := c.GetHeader("X-Step-Up-Token")

	var req P5ApproveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, svcErr := h.svc.P5Approve2(c.Request.Context(), eventCode, versionID, req, stepUpToken)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /mapping-jurnal/{event_code}/version/{version_id}/reject ───────────

// Reject handles POST .../reject.
func (h *P5M12Handler) Reject(c *gin.Context) {
	eventCode, versionID, err := parseEventCodeVersionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}

	var req P5RejectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "Body tidak valid: "+err.Error()))
		return
	}

	result, svcErr := h.svc.P5Reject(c.Request.Context(), eventCode, versionID, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /mapping-jurnal/bulk-import ────────────────────────────────────────

// BulkImport handles POST /api/v1/master/mapping-jurnal/bulk-import (multipart/form-data).
func (h *P5M12Handler) BulkImport(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "field 'file' wajib ada di form-data: "+err.Error()))
		return
	}

	result, svcErr := h.svc.ImportBulk(c.Request.Context(), fh)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── GET /reports/rpt-19-mapping-coverage ────────────────────────────────────

// GetCoverage handles GET /api/v1/reports/rpt-19-mapping-coverage.
func (h *P5M12Handler) GetCoverage(c *gin.Context) {
	resp, err := h.svc.GetCoverage(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, resp)
}

// ExportCoverage handles GET /api/v1/reports/rpt-19-mapping-coverage/export.
func (h *P5M12Handler) ExportCoverage(c *gin.Context) {
	resp, err := h.svc.GetCoverage(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("rpt19-mapping-coverage-%s.csv", time.Now().Format("20060102"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("X-Total-Rows", strconv.Itoa(len(resp.GapEvents)))

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Event Code", "Nama Event", "Gap Coverage", "Active Detail Count", "Missing Akun Count", "Last DLQ Error"})
	for _, ev := range resp.GapEvents {
		lastDlq := ""
		if ev.LastDlqError != nil {
			lastDlq = ev.LastDlqError.Format(time.RFC3339)
		}
		_ = w.Write([]string{ev.EventCode, ev.NamaEvent, string(ev.GapCoverage),
			strconv.Itoa(ev.ActiveDetailCount), strconv.Itoa(ev.MissingAkunCount), lastDlq})
	}
	w.Flush()
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ─── GET /reports/rpt-20-mapping-validation ──────────────────────────────────

// GetValidation handles GET /api/v1/reports/rpt-20-mapping-validation.
func (h *P5M12Handler) GetValidation(c *gin.Context) {
	resp, err := h.svc.GetValidation(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, resp)
}

// ─── GET /reports/rpt-21-mapping-history ─────────────────────────────────────

// GetHistory handles GET /api/v1/reports/rpt-21-mapping-history.
func (h *P5M12Handler) GetHistory(c *gin.Context) {
	eventCode := c.Query("event_code")
	cursor := c.Query("cursor")
	limit := 50
	if lStr := c.Query("limit"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			limit = n
		}
	}

	result, err := h.svc.GetHistory(c.Request.Context(), eventCode, cursor, limit)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result.Items,
		"pagination": gin.H{
			"nextCursor": result.NextCursor,
			"hasMore":    result.HasMore,
		},
	})
}

// ExportHistory handles GET /api/v1/reports/rpt-21-mapping-history/export.
// M7: pre-check row count before inline export. Dataset > 10k rows is hard-capped
// at 10000 rows and signals truncation via X-Truncated: true response header.
// Future: datasets > 10k should use async Asynq job (UX rule §1.4 async export pattern).
func (h *P5M12Handler) ExportHistory(c *gin.Context) {
	eventCode := c.Query("event_code")

	// Row count pre-check (M7)
	rowCount, err := h.svc.CountHistory(c.Request.Context(), eventCode)
	if err != nil {
		response.Error(c, err)
		return
	}
	const exportRowCap = 10000
	truncated := rowCount > exportRowCap
	if truncated {
		c.Header("X-Truncated", "true")
		c.Header("X-Total-Rows-Estimate", strconv.Itoa(rowCount))
	}

	result, err := h.svc.GetHistory(c.Request.Context(), eventCode, "", exportRowCap)
	if err != nil {
		response.Error(c, err)
		return
	}

	filename := fmt.Sprintf("rpt21-mapping-history-%s.csv", time.Now().Format("20060102"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("X-Total-Rows", strconv.Itoa(len(result.Items)))

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Event ID", "Event Time", "Actor User ID", "Actor Role", "Action", "Entity Type", "Entity ID", "Trace ID"})
	for _, e := range result.Items {
		traceID := ""
		if e.TraceID != nil {
			traceID = *e.TraceID
		}
		_ = w.Write([]string{
			e.EventID.String(),
			e.EventTime.Format(time.RFC3339),
			e.ActorUserID.String(),
			e.ActorRole,
			e.Action,
			e.EntityType,
			e.EntityID.String(),
			traceID,
		})
	}
	w.Flush()
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ─── Path parsing helpers ─────────────────────────────────────────────────────

// parseEventCodeVersionID extracts :event_code and :version_id from path.
func parseEventCodeVersionID(c *gin.Context) (string, uuid.UUID, error) {
	eventCode := c.Param("event_code")
	if eventCode == "" {
		return "", uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed, "event_code wajib di path")
	}
	versionIDStr := c.Param("version_id")
	versionID, err := uuid.Parse(versionIDStr)
	if err != nil {
		return "", uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed, "version_id bukan UUID valid: "+versionIDStr)
	}
	return eventCode, versionID, nil
}
