package kurs

// handler_p5m5.go — P5-M5 HTTP handlers: Upload, BatchApprove, BatchReject,
// JISDORSyncV2, Treatment.
//
// All handlers are thin:
//   parse request → call service → map result to response envelope.
// No business logic here.

import (
	"encoding/csv"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

const (
	maxUploadRows   = 200 // inline sync threshold; above this → async (P5-M11)
	maxUploadFileMB = 5   // 5MB file size limit
)

// ─── POST /master/kurs/upload ─────────────────────────────────────────────────

// Upload handles POST /api/v1/master/kurs/upload.
// Permission: kurs.upload (ROLE-AKUN)
//
// multipart/form-data: field "file" (CSV, max 5MB).
// Inline processing for ≤ 200 rows; async for > 200 rows (P5-M11).
func (h *Handler) Upload(c *gin.Context) {
	// Enforce file size limit
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxUploadFileMB)<<20)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Field 'file' wajib ada dalam multipart form-data: "+err.Error()))
		return
	}
	defer file.Close() //nolint:errcheck

	rawRows, parseErr := parseUploadFile(file, header)
	if parseErr != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Gagal membaca file upload: "+parseErr.Error()))
		return
	}

	if len(rawRows) == 0 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"File upload kosong — tidak ada baris data yang ditemukan."))
		return
	}

	// Large file → async (stub P5-M11)
	if len(rawRows) > maxUploadRows {
		// TODO(P5-M11): enqueue TaskFxUploadProcess + store file in MinIO, return 202.
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("File berisi %d baris (maks %d untuk sync). Fitur async upload direncanakan di P5-M11.",
				len(rawRows), maxUploadRows)))
		return
	}

	result, err := h.svc.UploadManual(c.Request.Context(), rawRows)
	if err != nil {
		response.Error(c, err)
		return
	}

	status := http.StatusOK
	if result.InvalidRows > 0 && result.ValidRows == 0 {
		status = http.StatusUnprocessableEntity
	}

	c.JSON(status, gin.H{
		"data": result,
		"meta": gin.H{"traceId": c.GetString("traceId")},
	})
}

// parseUploadFile reads multipart file and returns RawUploadRow slice.
// Supports CSV only in P5-M5 (XLSX via excelize planned for P5-M11).
func parseUploadFile(file multipart.File, header *multipart.FileHeader) ([]RawUploadRow, error) {
	name := strings.ToLower(header.Filename)
	if strings.HasSuffix(name, ".csv") {
		return parseCSVUpload(file)
	}
	return nil, fmt.Errorf("format file tidak didukung: %q. Gunakan .csv", header.Filename)
}

// parseCSVUpload reads a multipart CSV file into RawUploadRow slice.
// Expected columns (any order, first row is header):
//
//	kode_mata_uang, tanggal_berlaku, kurs_tengah, kurs_beli (opt), kurs_jual (opt), sumber_kurs (opt)
func parseCSVUpload(file multipart.File) ([]RawUploadRow, error) {
	r := csv.NewReader(file)
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("file CSV harus punya baris header + minimal 1 baris data")
	}

	// Build header column index map (case-insensitive)
	headerRow := records[0]
	colIdx := make(map[string]int, len(headerRow))
	for i, col := range headerRow {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	required := []string{"kode_mata_uang", "tanggal_berlaku", "kurs_tengah"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("kolom wajib tidak ditemukan di header CSV: %q", col)
		}
	}

	getCol := func(row []string, name string) string {
		idx, ok := colIdx[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	var rows []RawUploadRow
	for i, record := range records[1:] {
		rows = append(rows, RawUploadRow{
			RowNumber:    i + 2, // 1-based; header is row 1
			KodeMataUang: getCol(record, "kode_mata_uang"),
			Tanggal:      getCol(record, "tanggal_berlaku"),
			KursTengah:   getCol(record, "kurs_tengah"),
			KursBeli:     getCol(record, "kurs_beli"),
			KursJual:     getCol(record, "kurs_jual"),
			SumberKurs:   getCol(record, "sumber_kurs"),
		})
	}
	return rows, nil
}

// ─── POST /master/kurs/upload/{batch_id}/approve ──────────────────────────────

// BatchApprove handles POST /api/v1/master/kurs/upload/{batch_id}/approve.
// Permission: kurs.approve (ROLE-AKUN-CTL)
func (h *Handler) BatchApprove(c *gin.Context) {
	batchID := c.Param("batch_id")
	if batchID == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "batch_id wajib diisi di path."))
		return
	}

	var req BatchApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	result, err := h.svc.ApproveBatch(c.Request.Context(), batchID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
		"meta": gin.H{"traceId": c.GetString("traceId")},
	})
}

// ─── POST /master/kurs/upload/{batch_id}/reject ───────────────────────────────

// BatchReject handles POST /api/v1/master/kurs/upload/{batch_id}/reject.
// Permission: kurs.approve (ROLE-AKUN-CTL)
func (h *Handler) BatchReject(c *gin.Context) {
	batchID := c.Param("batch_id")
	if batchID == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "batch_id wajib diisi di path."))
		return
	}

	var req BatchRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	result, err := h.svc.RejectBatch(c.Request.Context(), batchID, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
		"meta": gin.H{"traceId": c.GetString("traceId")},
	})
}

// ─── POST /master/kurs/jisdor-sync (V2 — replaces stub) ──────────────────────

// JISDORSyncV2 handles POST /api/v1/master/kurs/jisdor-sync (P5-M5 full implementation).
// Permission: kurs.jisdor_sync
//
// Attempts inline JISDOR fetch. On provider error (stub) → returns 202 with explanation.
func (h *Handler) JISDORSyncV2(c *gin.Context) {
	var req JISDORSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	// Validate date format upfront
	if _, err := time.Parse("2006-01-02", req.TanggalBerlaku); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"tanggalBerlaku harus format YYYY-MM-DD."))
		return
	}

	provider := NewJISDORAdapter()
	result, err := h.svc.JISDORFetchAll(c.Request.Context(), req.TanggalBerlaku, provider)
	if err != nil {
		// Domain error (weekend/holiday) → 422
		if de, ok := domainerrors.IsDomainError(err); ok {
			response.Error(c, de)
			return
		}
		// Provider/infra error → 202 with fallback message (backward-compat)
		c.JSON(http.StatusAccepted, gin.H{
			"data": JISDORSyncResponse{
				JobID:     "provider-error",
				StatusURL: "",
				Message:   "JISDOR fetch gagal: " + err.Error() + " Gunakan upload manual via POST /api/v1/master/kurs.",
			},
			"meta": gin.H{"traceId": c.GetString("traceId")},
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"data": JISDORSyncResponse{
			JobID:     result.JobID,
			StatusURL: result.StatusURL,
			Message: fmt.Sprintf("JISDOR sync selesai: %d diinsert, %d skipped, %d error.",
				result.Inserted, result.Skipped, len(result.Errors)),
		},
		"meta": gin.H{"traceId": c.GetString("traceId")},
	})
}

// ─── GET /master/kurs/treatment/{instrumen_id} ────────────────────────────────

// Treatment handles GET /api/v1/master/kurs/treatment/{instrumen_id}.
// Permission: kurs.read
func (h *Handler) Treatment(c *gin.Context) {
	instrumenID := c.Param("instrumen_id")
	if instrumenID == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"instrumen_id wajib diisi di path."))
		return
	}

	result, err := h.svc.GetTreatment(c.Request.Context(), instrumenID)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
		"meta": gin.H{"traceId": c.GetString("traceId")},
	})
}

// csvNewReader is an alias for csv.NewReader to allow test stubbing.
// (direct csv.NewReader is fine; this just documents the dependency)
var csvNewReader = csv.NewReader
