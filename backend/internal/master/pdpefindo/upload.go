package pdpefindo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// XLSXUploadRequest is the parsed form fields from the upload-xlsx endpoint.
type XLSXUploadRequest struct {
	FileContent          []byte
	FileName             string
	TanggalPublikasi     string
	PeriodeBerlakuDari   string
	PeriodeBerlakuSampai *string
}

// XLSXRowResult captures per-row parse result for progress reporting.
type XLSXRowResult struct {
	RowNum  int
	Rating  string
	Skipped bool
	Error   string
}

// XLSXUploadResult is the completed job result stored in sys.job.result_jsonb.
type XLSXUploadResult struct {
	CreatedCount int             `json:"createdCount"`
	SkippedCount int             `json:"skippedCount"`
	ErrorCount   int             `json:"errorCount"`
	Rows         []XLSXRowResult `json:"rows"`
}

// UploadService handles XLSX upload processing for pd_pefindo.
// It is a separate concern from Service to keep business logic isolated.
type UploadService struct {
	repo        Repository
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewUploadService constructs an UploadService.
func NewUploadService(repo Repository, auditWriter *audit.Writer, logger *slog.Logger) *UploadService {
	if logger == nil {
		logger = slog.Default()
	}
	return &UploadService{repo: repo, auditWriter: auditWriter, logger: logger}
}

// SubmitUploadJob enqueues or (in sync fallback) immediately processes an XLSX upload.
//
// UX rule §3: operations > 2 seconds must be async. XLSX parsing can take > 2s for
// large files, so this always creates a sys.job record and returns 202.
//
// Idempotency: the file SHA-256 hash is stored in payload_jsonb.file_hash.
// If a job with the same file_hash already exists, the existing jobID is returned.
//
// The asynqClient parameter is an interface to allow nil (sync fallback in dev).
func (u *UploadService) SubmitUploadJob(
	ctx context.Context,
	req XLSXUploadRequest,
	asynqEnqueuer AsynqEnqueuer,
) (*UploadXLSXResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Compute file hash for idempotency.
	h := sha256.Sum256(req.FileContent)
	fileHash := hex.EncodeToString(h[:])

	// Build job ID (deterministic for same upload content — use file hash as suffix).
	jobID := "pdpefindo-upload-" + fileHash[:16]

	// Check idempotency: job already exists for this file?
	existing, err := u.repo.GetJobByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("UploadService.SubmitUploadJob: check existing: %w", err)
	}
	if existing != nil {
		return &UploadXLSXResponse{
			JobID:     jobID,
			StatusURL: "/api/v1/master/pd-pefindo/upload-jobs/" + jobID,
			StreamURL: "/api/v1/jobs/" + jobID + "/stream",
		}, nil
	}

	payload := map[string]interface{}{
		"file_hash":               fileHash,
		"file_name":               req.FileName,
		"tanggal_publikasi":       req.TanggalPublikasi,
		"periode_berlaku_dari":    req.PeriodeBerlakuDari,
		"periode_berlaku_sampai":  nil,
	}
	if req.PeriodeBerlakuSampai != nil {
		payload["periode_berlaku_sampai"] = *req.PeriodeBerlakuSampai
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("UploadService.SubmitUploadJob: marshal payload: %w", err)
	}

	now := time.Now()
	jobRow := &JobRow{
		ID:        jobID,
		Type:      "PD_PEFINDO_UPLOAD_XLSX",
		Status:    "queued",
		Progress:  0,
		PayloadJSON: payloadJSON,
		CanCancel:  false,
		CreatedBy:  actorID,
		CreatedAt:  now,
		TenantID:   tenantID(claims),
	}

	tx, err := u.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("UploadService.SubmitUploadJob: begin tx: %w", err)
	}

	if err := u.repo.CreateJob(ctx, tx, jobRow); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			u.logger.WarnContext(ctx, "SubmitUploadJob: rollback failed", "error", rollbackErr)
		}
		return nil, fmt.Errorf("UploadService.SubmitUploadJob: create job: %w", err)
	}

	// Audit: PD_PEFINDO.UPLOAD_XLSX job submitted.
	if err := u.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "PD_PEFINDO.UPLOAD_XLSX",
		EntityType:  "sys.job",
		EntityID:    actorID, // use actorID as proxy; job has string PK
		ActorUserID: actorID.String(),
		After: map[string]interface{}{
			"job_id":    jobID,
			"file_hash": fileHash,
			"file_name": req.FileName,
		},
	}); err != nil {
		u.logger.WarnContext(ctx, "SubmitUploadJob: audit write failed", "error", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("UploadService.SubmitUploadJob: commit: %w", err)
	}

	// If asynqEnqueuer is nil (dev/test mode), process synchronously in a goroutine.
	if asynqEnqueuer == nil {
		go func() {
			bgCtx := context.Background()
			if err := u.ProcessUploadJob(bgCtx, jobID, req); err != nil {
				u.logger.Error("SubmitUploadJob: sync process failed", "jobId", jobID, "error", err)
			}
		}()
	} else {
		if err := asynqEnqueuer.EnqueueUploadXLSX(ctx, jobID, payloadJSON); err != nil {
			u.logger.WarnContext(ctx, "SubmitUploadJob: asynq enqueue failed", "jobId", jobID, "error", err)
			// Fallback: process in goroutine.
			go func() {
				bgCtx := context.Background()
				if err := u.ProcessUploadJob(bgCtx, jobID, req); err != nil {
					u.logger.Error("SubmitUploadJob: fallback process failed", "jobId", jobID, "error", err)
				}
			}()
		}
	}

	return &UploadXLSXResponse{
		JobID:     jobID,
		StatusURL: "/api/v1/master/pd-pefindo/upload-jobs/" + jobID,
		StreamURL: "/api/v1/jobs/" + jobID + "/stream",
	}, nil
}

// ProcessUploadJob parses the XLSX content and bulk-inserts DRAFT rows.
// Called by the Asynq worker or directly in sync fallback mode.
//
// Template columns (header row 1):
// rating | pd_12m | pd_3y | pd_5y | pd_7y | pd_10y
func (u *UploadService) ProcessUploadJob(ctx context.Context, jobID string, req XLSXUploadRequest) error {
	u.repo.UpdateJobProgress(ctx, jobID, 0, "Membaca file XLSX...") //nolint:errcheck

	f, err := excelize.OpenReader(bytesReader(req.FileContent))
	if err != nil {
		return u.failJob(ctx, jobID, fmt.Errorf("buka XLSX gagal: %w", err))
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return u.failJob(ctx, jobID, fmt.Errorf("file XLSX tidak memiliki sheet"))
	}
	sheetName := sheets[0]

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return u.failJob(ctx, jobID, fmt.Errorf("baca rows gagal: %w", err))
	}
	if len(rows) < 2 {
		return u.failJob(ctx, jobID, fmt.Errorf("file tidak memiliki data (hanya header atau kosong)"))
	}

	// Validate header row.
	header := normalizeHeader(rows[0])
	colIdx, err := resolveColumns(header)
	if err != nil {
		return u.failJob(ctx, jobID, err)
	}

	u.repo.UpdateJobProgress(ctx, jobID, 5, fmt.Sprintf("Header valid, memproses %d baris data...", len(rows)-1)) //nolint:errcheck

	// Parse data rows.
	dataRows := rows[1:]
	total := len(dataRows)

	var (
		rowResults   []XLSXRowResult
		toPersist    []*PDPefindo
		createdCount int
		skippedCount int
		errCount     int
	)

	for i, row := range dataRows {
		rowNum := i + 2 // +2: 1-indexed + skip header

		rating := cellStr(row, colIdx["rating"])
		if rating == "" {
			skippedCount++
			rowResults = append(rowResults, XLSXRowResult{RowNum: rowNum, Skipped: true})
			continue
		}

		pd12, parseErr := parsePDCell(row, colIdx, "pd_12m")
		if parseErr != nil {
			errCount++
			rowResults = append(rowResults, XLSXRowResult{RowNum: rowNum, Rating: rating, Error: parseErr.Error()})
			continue
		}

		pd3y := parsePDCellOptional(row, colIdx, "pd_3y")
		pd5y := parsePDCellOptional(row, colIdx, "pd_5y")
		pd7y := parsePDCellOptional(row, colIdx, "pd_7y")
		pd10y := parsePDCellOptional(row, colIdx, "pd_10y")

		// Validate rating whitelist.
		if !IsValidPefindoRating(rating) {
			errCount++
			rowResults = append(rowResults, XLSXRowResult{
				RowNum: rowNum, Rating: rating,
				Error: fmt.Sprintf("rating '%s' tidak valid dalam Pefindo whitelist", rating),
			})
			continue
		}

		// Validate PD ranges.
		if pd12.LessThan(decimal.Zero) || pd12.GreaterThan(decimal.NewFromInt(1)) {
			errCount++
			rowResults = append(rowResults, XLSXRowResult{
				RowNum: rowNum, Rating: rating,
				Error: fmt.Sprintf("pd_12m %.8f di luar range 0-1", pd12.InexactFloat64()),
			})
			continue
		}

		// Validate monotonicity.
		if monoErr := validateMonotonicity(pd12, pd3y, pd5y, pd7y, pd10y); monoErr != nil {
			errCount++
			rowResults = append(rowResults, XLSXRowResult{
				RowNum: rowNum, Rating: rating,
				Error: monoErr.Error(),
			})
			continue
		}

		now := time.Now()
		draftID := uuid.New()
		// Use a placeholder UUID for uploaded_by in bulk context.
		// The actual actor context may not carry a valid DB user during async processing.
		var actorID uuid.UUID
		if cl := auth.ClaimsFromContext(ctx); cl != nil {
			actorID, _ = uuid.Parse(cl.Sub)
		}
		if actorID == uuid.Nil {
			actorID = uuid.MustParse("00000000-0000-0000-0000-000000000001") // system user placeholder
		}

		p := &PDPefindo{
			ID:                   draftID,
			Rating:               rating,
			PD12Month:            pd12,
			PDLifetime3Y:         pd3y,
			PDLifetime5Y:         pd5y,
			PDLifetime7Y:         pd7y,
			PDLifetime10Y:        pd10y,
			Sumber:               DefaultSumber,
			TanggalPublikasi:     &req.TanggalPublikasi,
			PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
			PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
			WorkflowStatus:       WorkflowStatusDraft,
			UploadedBy:           actorID,
			UploadedAt:           now,
			CreatedAt:            now,
			CreatedBy:            &actorID,
			RowVersion:           1,
			TenantID:             "TUGURE",
		}
		toPersist = append(toPersist, p)
		rowResults = append(rowResults, XLSXRowResult{RowNum: rowNum, Rating: rating})

		// Report progress every 10%.
		if (i+1)%max(total/10, 1) == 0 {
			pct := 5 + (i+1)*90/total
			u.repo.UpdateJobProgress(ctx, jobID, pct, fmt.Sprintf("Diproses %d dari %d baris", i+1, total)) //nolint:errcheck
		}
	}

	u.repo.UpdateJobProgress(ctx, jobID, 90, fmt.Sprintf("Menyimpan %d baris valid ke database...", len(toPersist))) //nolint:errcheck

	// Bulk insert in one transaction.
	if len(toPersist) > 0 {
		tx, err := u.repo.BeginTx(ctx)
		if err != nil {
			return u.failJob(ctx, jobID, fmt.Errorf("begin tx gagal: %w", err))
		}

		n, err := u.repo.BulkCreate(ctx, tx, toPersist)
		if err != nil {
			tx.Rollback() //nolint:errcheck
			return u.failJob(ctx, jobID, fmt.Errorf("bulk insert gagal (baris tersimpan sebelum error: %d): %w", n, err))
		}

		createdCount = n

		if err := tx.Commit(); err != nil {
			return u.failJob(ctx, jobID, fmt.Errorf("commit gagal: %w", err))
		}
	}

	result := XLSXUploadResult{
		CreatedCount: createdCount,
		SkippedCount: skippedCount,
		ErrorCount:   errCount,
		Rows:         rowResults,
	}
	resultJSON, _ := json.Marshal(result)

	if err := u.repo.CompleteJob(ctx, jobID, resultJSON); err != nil {
		u.logger.WarnContext(ctx, "ProcessUploadJob: CompleteJob failed", "jobId", jobID, "error", err)
	}

	return nil
}

func (u *UploadService) failJob(ctx context.Context, jobID string, cause error) error {
	errJSON, _ := json.Marshal(map[string]interface{}{
		"code":    "XLSX_PARSE_ERROR",
		"message": cause.Error(),
	})
	if err := u.repo.FailJob(ctx, jobID, errJSON); err != nil {
		u.logger.WarnContext(ctx, "failJob: FailJob DB update failed", "jobId", jobID, "error", err)
	}
	return cause
}

// ─── XLSX parsing helpers ─────────────────────────────────────────────────────

// normalizeHeader lowercases and trims all header cells.
func normalizeHeader(row []string) []string {
	out := make([]string, len(row))
	for i, c := range row {
		out[i] = trimLower(c)
	}
	return out
}

// resolveColumns maps expected column names to their 0-based indices.
// Returns error if required columns are missing.
func resolveColumns(header []string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[h] = i
	}

	required := []string{"rating", "pd_12m"}
	for _, col := range required {
		if _, ok := idx[col]; !ok {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				fmt.Sprintf("Kolom wajib '%s' tidak ditemukan di header XLSX. Header ditemukan: %v", col, header),
			)
		}
	}
	return idx, nil
}

func cellStr(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return trimLower(row[idx])
}

func parsePDCell(row []string, colIdx map[string]int, colName string) (decimal.Decimal, error) {
	col, ok := colIdx[colName]
	if !ok || col >= len(row) {
		return decimal.Zero, fmt.Errorf("kolom '%s' tidak ditemukan", colName)
	}
	v := row[col]
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero, fmt.Errorf("nilai '%s' tidak valid sebagai decimal: %v", v, err)
	}
	return d, nil
}

func parsePDCellOptional(row []string, colIdx map[string]int, colName string) *decimal.Decimal {
	col, ok := colIdx[colName]
	if !ok || col >= len(row) || row[col] == "" {
		return nil
	}
	d, err := decimal.NewFromString(row[col])
	if err != nil {
		return nil
	}
	return &d
}

func trimLower(s string) string {
	// Manual trim + lower for ASCII compatibility without importing strings at package level
	out := make([]byte, 0, len(s))
	started := false
	lastNonSpace := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if started {
				out = append(out, ' ')
			}
		} else {
			if c >= 'A' && c <= 'Z' {
				c += 32
			}
			out = append(out, c)
			started = true
			lastNonSpace = len(out) - 1
		}
	}
	if lastNonSpace >= 0 {
		return string(out[:lastNonSpace+1])
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// bytesReader wraps a []byte to implement io.Reader.
type bytesReader []byte

func (b bytesReader) Read(p []byte) (int, error) {
	if len(b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b)
	return n, io.EOF
}

// AsynqEnqueuer abstracts the Asynq client for XLSX upload jobs.
// Allows nil (sync fallback in dev) and production wiring without import cycle.
type AsynqEnqueuer interface {
	EnqueueUploadXLSX(ctx context.Context, jobID string, payload []byte) error
}
