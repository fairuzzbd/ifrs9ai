package reporting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
)

// Service is the business logic layer for P5-M13 reporting.
type Service struct {
	repo       *Repository
	mvRepo     *MVRepo
	asynqClient *asynq.Client
	minio      *MinIOClient
	smtp       *SMTPClient
	aw         *audit.Writer
	logger     *slog.Logger
	// Config thresholds (loaded from sys.config_param or defaults).
	inlineThreshold int64  // REPORT_EXPORT_INLINE_THRESHOLD
	maxRows         int64  // REPORT_EXPORT_MAX_ROWS
	minioTTLHours   int64  // REPORT_EXPORT_MINIO_TTL_HOURS
	optOutSecret    []byte // HMAC secret for opt-out token
}

// NewService creates a Service. Nil minio/smtp are tolerated in test mode.
func NewService(
	repo *Repository,
	mvRepo *MVRepo,
	asynqClient *asynq.Client,
	minio *MinIOClient,
	smtpClient *SMTPClient,
	aw *audit.Writer,
	logger *slog.Logger,
	optOutSecret []byte,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:            repo,
		mvRepo:          mvRepo,
		asynqClient:     asynqClient,
		minio:           minio,
		smtp:            smtpClient,
		aw:              aw,
		logger:          logger,
		inlineThreshold: 10_000,
		maxRows:         100_000,
		minioTTLHours:   24,
		optOutSecret:    optOutSecret,
	}
}

// ─── S1/S2 — MV Status + Trigger Refresh ─────────────────────────────────────

// ListMVStatus returns status metadata for all 8 MVs.
func (s *Service) ListMVStatus(ctx context.Context) ([]MVStatusItem, error) {
	db := ChooseDBWithContext(ctx, s.repo.primary, s.repo.replica, ReadIntentPrimary)
	return ListMVStatus(ctx, db, tenantFromCtx(ctx))
}

// TriggerRefresh validates mvName, checks for in-progress refresh, and enqueues Asynq job.
// S2-AC3: MV_REFRESH_LOCKED if RUNNING row exists.
func (s *Service) TriggerRefresh(ctx context.Context, mvName *string) (*AsyncJobRef, error) {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantFromCtx(ctx)
	var actorID string
	if claims != nil {
		actorID = claims.Sub
	}

	// Determine which MVs to refresh.
	var names []string
	if mvName != nil && *mvName != "" {
		if !isValidMVName(*mvName) {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				fmt.Sprintf("mv_name %q tidak dikenal. Nilai valid: %v", *mvName, AllMVNames))
		}
		names = []string{*mvName}
	} else {
		names = AllMVNames
	}

	// Check for running refresh on first MV (single check covers most cases).
	for _, n := range names {
		running, log, err := s.mvRepo.IsRefreshRunning(ctx, n, tid)
		if err != nil {
			return nil, fmt.Errorf("TriggerRefresh: check running: %w", err)
		}
		if running {
			elapsed := time.Since(log.StartedAt).Round(time.Second)
			return nil, domainerrors.New(domainerrors.CodeMVRefreshLocked,
				fmt.Sprintf("Refresh %s sedang berjalan (started %s lalu). Coba lagi setelah selesai.", n, elapsed))
		}
	}

	// Enqueue Asynq job(s).
	triggered := TriggeredByManual
	jobID := uuid.New().String()

	payload := MVRefreshPayload{
		TriggeredBy:  string(triggered),
		TriggerActor: actorID,
		TenantID:     tid,
	}
	if len(names) == 1 {
		payload.MVName = names[0]
	}
	// "" MVName → worker refreshes all 8.

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("TriggerRefresh: marshal: %w", err)
	}
	task := asynq.NewTask(TaskMVRefresh, payloadBytes,
		asynq.MaxRetry(1), asynq.Timeout(30*time.Minute),
		asynq.TaskID(jobID))
	if s.asynqClient == nil {
		return nil, fmt.Errorf("TriggerRefresh: asynq client not configured")
	}
	if _, err = s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, fmt.Errorf("TriggerRefresh: enqueue: %w", err)
	}

	return &AsyncJobRef{
		JobID:     jobID,
		StatusURL: "/api/v1/jobs/" + jobID,
		StreamURL: "/api/v1/jobs/" + jobID + "/stream",
	}, nil
}

// ─── S3/S4 — Export Engine ────────────────────────────────────────────────────

// RequestExport validates permissions + format, estimates row count, and either:
// - returns (nil, nil) after streaming inline (handled by handler calling engine directly), or
// - enqueues async job and returns AsyncJobRef.
//
// Permission: report.{slug}.export OR audit_log.read (ROLE-AUDIT bypass).
func (s *Service) RequestExport(ctx context.Context, req ExportRequest) (*AsyncJobRef, *ExportLogRow, error) {
	// Permission check.
	if err := s.checkExportPermission(ctx, req.ReportSlug); err != nil {
		return nil, nil, err
	}

	// Validate format.
	if !req.Format.IsValid() {
		return nil, nil, domainerrors.New(domainerrors.CodeExportFormatUnsupported,
			fmt.Sprintf("Format '%s' tidak didukung. Format tersedia: csv, xlsx, pdf.", req.Format))
	}

	// Validate slug.
	mvName, ok := ValidReportSlugs[req.ReportSlug]
	if !ok {
		return nil, nil, domainerrors.ErrNotFound("report_slug " + req.ReportSlug)
	}

	// Estimate row count.
	rowCount, err := s.repo.CountMVRows(ctx, mvName)
	if err != nil {
		return nil, nil, fmt.Errorf("RequestExport: count rows: %w", err)
	}

	// Hard cap check.
	if rowCount > s.maxRows {
		return nil, nil, domainerrors.New(domainerrors.CodeExportTooLarge,
			fmt.Sprintf("Dataset %d rows melebihi batas %d rows per export. Gunakan filter untuk mempersempit data.",
				rowCount, s.maxRows))
	}

	// Create export_log row.
	exportID := uuid.New()
	exportRow := ExportLogRow{
		ID:          exportID,
		ReportSlug:  req.ReportSlug,
		Format:      req.Format,
		Status:      ExportStatusRequested,
		RequestedBy: req.ActorID,
		RequestedAt: time.Now(),
		TenantID:    req.TenantID,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("RequestExport: begin tx: %w", err)
	}

	if err = s.repo.InsertExportLog(ctx, tx, exportRow); err != nil {
		_ = tx.Rollback()
		return nil, nil, fmt.Errorf("RequestExport: insert log: %w", err)
	}

	// Write audit EXPORT.GENERATED in-tx (S3-AC1).
	if s.aw != nil {
		s.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
			Action:     "EXPORT.GENERATED",
			EntityType: "sys.export_log",
			EntityID:   exportID,
			After: map[string]any{
				"report_slug": req.ReportSlug,
				"format":      string(req.Format),
				"row_count":   rowCount,
				"actor":       req.ActorID.String(),
			},
		})
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("RequestExport: commit: %w", err)
	}

	// Inline vs async decision.
	if rowCount <= s.inlineThreshold {
		// Signal to handler: inline export (no job).
		return nil, &exportRow, nil
	}

	// Async: enqueue Asynq job.
	jobID := uuid.New().String()
	payload := ExportWorkerPayload{
		ExportLogID: exportID.String(),
		ReportSlug:  req.ReportSlug,
		Format:      string(req.Format),
		TenantID:    req.TenantID,
		ActorID:     req.ActorID.String(),
	}
	payloadBytes, _ := json.Marshal(payload)
	task := asynq.NewTask(TaskExportAsync, payloadBytes,
		asynq.MaxRetry(1), asynq.Timeout(60*time.Minute),
		asynq.TaskID(jobID))
	if _, err = s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, nil, fmt.Errorf("RequestExport: enqueue: %w", err)
	}

	ref := &AsyncJobRef{
		JobID:     jobID,
		StatusURL: "/api/v1/jobs/" + jobID,
		StreamURL: "/api/v1/jobs/" + jobID + "/stream",
	}
	return ref, &exportRow, nil
}

// BuildInlineExport generates the file bytes for an inline export (≤ INLINE_THRESHOLD rows).
// Returns fileBytes, sha256Hex, contentType.
func (s *Service) BuildInlineExport(ctx context.Context, slug string, format ExportFormat, username string) ([]byte, string, string, error) {
	rows, headers, err := s.queryMVRows(ctx, slug)
	if err != nil {
		return nil, "", "", err
	}

	exportedAt := time.Now()
	var fileBytes []byte
	var sha256Hex string

	switch format {
	case FormatCSV:
		var buf bytes.Buffer
		sha256Hex, err = exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, "", "", fmt.Errorf("BuildInlineExport: csv: %w", err)
		}
		fileBytes = buf.Bytes()
		return fileBytes, sha256Hex, "text/csv; charset=UTF-8", nil

	case FormatXLSX:
		fileBytes, sha256Hex, err = exporter.ExportXLSX(exporter.ExportXLSXOptions{
			SheetName:  "Data",
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, "", "", fmt.Errorf("BuildInlineExport: xlsx: %w", err)
		}
		return fileBytes, sha256Hex, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil

	case FormatPDF:
		fileBytes, sha256Hex, err = exporter.ExportPDF(exporter.ExportPDFOptions{
			Title:      slug,
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, "", "", fmt.Errorf("BuildInlineExport: pdf: %w", err)
		}
		return fileBytes, sha256Hex, "application/pdf", nil

	default:
		return nil, "", "", domainerrors.New(domainerrors.CodeExportFormatUnsupported,
			"format tidak dikenal: "+string(format))
	}
}

// GetExportDownload returns the signed URL or signals to stream the file.
// Writes audit EXPORT.DOWNLOADED in-tx (S4-AC4).
func (s *Service) GetExportDownload(ctx context.Context, exportID uuid.UUID) (*ExportLogRow, error) {
	tid := tenantFromCtx(ctx)
	row, err := s.repo.GetExportLog(ctx, exportID, tid)
	if err != nil {
		return nil, fmt.Errorf("GetExportDownload: get: %w", err)
	}
	if row == nil {
		return nil, domainerrors.ErrNotFound("export " + exportID.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetExportDownload: begin tx: %w", err)
	}
	if err = s.repo.UpdateExportLogDownloaded(ctx, tx, exportID, tid); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("GetExportDownload: update: %w", err)
	}
	if s.aw != nil {
		s.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
			Action:     "EXPORT.DOWNLOADED",
			EntityType: "sys.export_log",
			EntityID:   exportID,
			After: map[string]any{
				"minio_path":   row.MinioPath,
				"downloaded_at": time.Now().Format(time.RFC3339),
			},
		})
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("GetExportDownload: commit: %w", err)
	}
	return row, nil
}

// ─── S5 — Scheduled Email ─────────────────────────────────────────────────────

// CreateScheduledEmail creates a sys.scheduled_email config.
func (s *Service) CreateScheduledEmail(ctx context.Context, req ScheduledEmailCreateReq) (*ScheduledEmailItem, error) {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantFromCtx(ctx)
	var actorID uuid.UUID
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	// Validate.
	if _, ok := ValidReportSlugs[req.ReportSlug]; !ok {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"report_slug tidak valid: "+req.ReportSlug)
	}
	if !req.Format.IsValid() {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"format tidak valid: "+string(req.Format))
	}
	if !isValidFrequency(req.Frequency) {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"frequency harus daily/weekly/monthly")
	}

	schedID := uuid.New()
	subjTpl := req.SubjectTemplate
	bodyTpl := req.BodyTemplate
	row := ScheduledEmailRow{
		ID:              schedID,
		ReportSlug:      req.ReportSlug,
		Format:          req.Format,
		Frequency:       req.Frequency,
		SendTime:        req.SendTime,
		Active:          req.Active,
		SubjectTemplate: nilableStr(subjTpl),
		BodyTemplate:    nilableStr(bodyTpl),
		CreatedBy:       actorID,
		TenantID:        tid,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateScheduledEmail: begin tx: %w", err)
	}
	if err = s.repo.InsertScheduledEmail(ctx, tx, row, req.Recipients); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("CreateScheduledEmail: insert: %w", err)
	}
	if s.aw != nil {
		s.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
			Action:     "SCHEDULED_EMAIL.CREATED",
			EntityType: "sys.scheduled_email",
			EntityID:   schedID,
			After: map[string]any{
				"sched_id":         schedID.String(),
				"report_slug":      req.ReportSlug,
				"recipients_count": len(req.Recipients),
				"actor":            actorID.String(),
			},
		})
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("CreateScheduledEmail: commit: %w", err)
	}

	return &ScheduledEmailItem{
		ID:          schedID,
		ReportSlug:  req.ReportSlug,
		Format:      req.Format,
		Frequency:   req.Frequency,
		SendTime:    req.SendTime,
		Recipients:  req.Recipients,
		Active:      req.Active,
		CreatedAt:   time.Now(),
		CreatedBy:   actorID,
	}, nil
}

// SoftDeleteScheduledEmail soft-deletes a sys.scheduled_email row.
func (s *Service) SoftDeleteScheduledEmail(ctx context.Context, schedID uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantFromCtx(ctx)
	var actorID uuid.UUID
	if claims != nil {
		actorID, _ = uuid.Parse(claims.Sub)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("SoftDeleteScheduledEmail: begin tx: %w", err)
	}
	if err = s.repo.SoftDeleteScheduledEmail(ctx, tx, schedID, actorID, tid); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("SoftDeleteScheduledEmail: delete: %w", err)
	}
	if s.aw != nil {
		s.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
			Action:     "SCHEDULED_EMAIL.DELETED",
			EntityType: "sys.scheduled_email",
			EntityID:   schedID,
			After:      map[string]any{"sched_id": schedID.String(), "actor": actorID.String()},
		})
	}
	return tx.Commit()
}

// OptOutRecipient verifies the signed token and inserts an opt-out row.
// S5-AC4: no auth required; token-based.
func (s *Service) OptOutRecipient(ctx context.Context, req OptOutRequest) error {
	// Verify HMAC token.
	if !s.verifyOptOutToken(req.ScheduledEmailID, req.Email, req.Token) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			"Token opt-out tidak valid atau sudah kadaluarsa.")
	}
	tokenHash := hashToken(req.Token)
	return s.repo.InsertOptOut(ctx, req.ScheduledEmailID, req.Email, tokenHash, tenantFromCtx(ctx))
}

// ─── List helpers ─────────────────────────────────────────────────────────────

// ListExportLogs returns cursor-paged sys.export_log list.
func (s *Service) ListExportLogs(ctx context.Context, cursor string, limit int) ([]ExportLogItem, *string, bool, error) {
	return s.repo.ListExportLogs(ctx, cursor, limit, tenantFromCtx(ctx))
}

// ─── Permission check ─────────────────────────────────────────────────────────

// checkExportPermission checks report.{slug}.export OR audit_log.read (bypass).
func (s *Service) checkExportPermission(ctx context.Context, slug string) error {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return domainerrors.ErrUnauthorized("claims tidak ditemukan")
	}
	// ROLE-AUDIT has audit_log.read which bypasses per-report permission.
	if claims.HasPermission("audit_log.read") {
		return nil
	}
	perm := "report." + slug + ".export"
	if !claims.HasPermission(perm) {
		return domainerrors.New(domainerrors.CodeExportPermissionDenied,
			fmt.Sprintf("Tidak punya permission '%s'. Hubungi ROLE-IT-ADMIN.", perm))
	}
	return nil
}

// ─── MV row query (skeleton — M14 will add per-report SQL) ───────────────────

// queryMVRows fetches all rows + headers from an MV.
// Returns string-formatted rows for export engine consumption.
// Real column definitions deferred to M14; skeleton returns empty or minimal.
func (s *Service) queryMVRows(ctx context.Context, slug string) (rows [][]string, headers []string, err error) {
	mvName, ok := ValidReportSlugs[slug]
	if !ok {
		return nil, nil, domainerrors.ErrNotFound("report_slug " + slug)
	}
	db := ChooseDBWithContext(ctx, s.repo.primary, s.repo.replica, ReadIntentReporting)
	// #nosec G202 — mvName validated by ValidReportSlugs above.
	queryRows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT %d", mvName, s.inlineThreshold+1))
	if err != nil {
		return nil, nil, fmt.Errorf("queryMVRows: %w", err)
	}
	defer queryRows.Close()

	colTypes, err := queryRows.ColumnTypes()
	if err != nil {
		return nil, nil, fmt.Errorf("queryMVRows: column types: %w", err)
	}
	for _, ct := range colTypes {
		headers = append(headers, ct.Name())
	}

	cols := make([]any, len(colTypes))
	colPtrs := make([]any, len(colTypes))
	for i := range cols {
		colPtrs[i] = &cols[i]
	}

	for queryRows.Next() {
		if err = queryRows.Scan(colPtrs...); err != nil {
			return nil, nil, fmt.Errorf("queryMVRows: scan: %w", err)
		}
		row := make([]string, len(cols))
		for i, v := range cols {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}
	return rows, headers, queryRows.Err()
}

// ─── Token helpers ────────────────────────────────────────────────────────────

// GenerateOptOutToken generates an HMAC-SHA256 signed opt-out token.
// Format: HMAC(secret, schedID+":"+email+":"+expiresUnix)
func (s *Service) GenerateOptOutToken(schedID uuid.UUID, email string, ttl time.Duration) string {
	expiresAt := time.Now().Add(ttl).Unix()
	msg := schedID.String() + ":" + email + ":" + strconv.FormatInt(expiresAt, 10)
	mac := hmac.New(sha256.New, s.optOutSecret)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil)) + "." + strconv.FormatInt(expiresAt, 10)
}

// VerifyOptOutToken is the exported test shim for verifyOptOutToken.
// Production code uses the unexported version directly.
func (s *Service) VerifyOptOutToken(schedID uuid.UUID, email, token string) error {
	if !s.verifyOptOutToken(schedID, email, token) {
		return domainerrors.New(domainerrors.CodeValidationFailed, "token opt-out tidak valid atau sudah kadaluarsa")
	}
	return nil
}

// verifyOptOutToken verifies a token generated by GenerateOptOutToken.
func (s *Service) verifyOptOutToken(schedID uuid.UUID, email, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	sigHex, expiresStr := parts[0], parts[1]
	expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiresAt {
		return false // expired
	}
	msg := schedID.String() + ":" + email + ":" + expiresStr
	mac := hmac.New(sha256.New, s.optOutSecret)
	mac.Write([]byte(msg))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sigHex))
}

// hashToken returns SHA-256 hex of the raw token (for traceability in optout table).
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ─── Context helpers ──────────────────────────────────────────────────────────

func tenantFromCtx(ctx context.Context) string {
	claims := auth.ClaimsFromContext(ctx)
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

func nilableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isValidFrequency(f ScheduledEmailFrequency) bool {
	switch f {
	case FreqDaily, FreqWeekly, FreqMonthly:
		return true
	}
	return false
}
