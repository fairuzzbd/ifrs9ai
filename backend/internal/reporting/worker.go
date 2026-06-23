package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
)

const (
	// TaskMVRefresh is the Asynq task type for MV refresh.
	TaskMVRefresh = "reporting:mv-refresh"
	// TaskExportAsync is the Asynq task type for async export.
	TaskExportAsync = "reporting:export-async"
	// TaskScheduledEmailSend is the Asynq task type for scheduled email send.
	TaskScheduledEmailSend = "reporting:scheduled-email-send"
)

// Worker handles P5-M13 Asynq tasks.
type Worker struct {
	svc    *Service
	repo   *Repository
	mvRepo *MVRepo
	minio  *MinIOClient
	smtp   *SMTPClient
	aw     *audit.Writer
	logger *slog.Logger
}

// NewWorker creates a Worker.
func NewWorker(svc *Service, repo *Repository, mvRepo *MVRepo, minio *MinIOClient, smtp *SMTPClient, aw *audit.Writer, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, repo: repo, mvRepo: mvRepo, minio: minio, smtp: smtp, aw: aw, logger: logger}
}

// RegisterHandlers registers all reporting task handlers on the Asynq ServeMux.
func (w *Worker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskMVRefresh, w.HandleMVRefresh)
	mux.HandleFunc(TaskExportAsync, w.HandleExportAsync)
	mux.HandleFunc(TaskScheduledEmailSend, w.HandleScheduledEmailSend)
}

// ─── MV Refresh Handler ───────────────────────────────────────────────────────

// HandleMVRefresh processes a reporting:mv-refresh task.
// S2-AC1: refresh all 8 MVs; S2-AC2: triggered by hard-close; S2-AC4: failure → DLQ + audit.
func (w *Worker) HandleMVRefresh(ctx context.Context, t *asynq.Task) error {
	var p MVRefreshPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("worker.HandleMVRefresh: unmarshal: %w", err)
	}
	if p.TenantID == "" {
		p.TenantID = "TUGURE"
	}
	triggered := TriggeredBy(p.TriggeredBy)
	if triggered == "" {
		triggered = TriggeredByCron
	}

	// Determine names.
	names := AllMVNames
	if p.MVName != "" {
		if !isValidMVName(p.MVName) {
			return fmt.Errorf("worker.HandleMVRefresh: unknown mv_name %q", p.MVName)
		}
		names = []string{p.MVName}
	}

	for _, mvName := range names {
		if err := w.refreshOneMV(ctx, mvName, triggered, p.TriggerActor, p.TenantID); err != nil {
			w.logger.ErrorContext(ctx, "worker.HandleMVRefresh: refresh failed",
				"mv_name", mvName, "error", err)
			// For cron (all 8 MVs), continue on error (partial success).
			// For single MV, return error → Asynq DLQ.
			if len(names) == 1 {
				return fmt.Errorf("worker.HandleMVRefresh: %w", err)
			}
		}
	}
	return nil
}

// refreshOneMV executes CONCURRENT refresh for one MV + logs + audit.
func (w *Worker) refreshOneMV(ctx context.Context, mvName string, triggered TriggeredBy, actorStr, tenantID string) error {
	logID := uuid.New()
	var actorID *uuid.UUID
	if actorStr != "" {
		uid, err := uuid.Parse(actorStr)
		if err == nil {
			actorID = &uid
		}
	}

	// Insert RUNNING log row.
	log := &MVRefreshLog{
		ID:           logID,
		MVName:       mvName,
		TriggeredBy:  triggered,
		TriggerActor: actorID,
		Status:       "RUNNING",
		StartedAt:    Now(),
		TenantID:     tenantID,
	}
	tx, err := w.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("refreshOneMV: begin tx: %w", err)
	}
	if err = w.mvRepo.InsertRefreshLog(ctx, tx, log); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("refreshOneMV: insert log: %w", err)
	}

	// Audit REPORT.MV_REFRESH start (minimal; completed event written after success).
	if w.aw != nil {
		w.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
			Action:     "REPORT.MV_REFRESH",
			EntityType: "sys.mv_refresh_log",
			EntityID:   logID,
			After: map[string]any{
				"mv_name":      mvName,
				"triggered_by": string(triggered),
				"tenant_id":    tenantID,
			},
		})
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("refreshOneMV: commit log: %w", err)
	}

	// Execute CONCURRENT refresh.
	start := time.Now()
	rowCount, refreshErr := RefreshConcurrent(ctx, w.repo.primary, mvName)
	durationMs := time.Since(start).Milliseconds()

	if refreshErr != nil {
		// Check for MV_REFRESH_LOCKED (advisory lock not acquired).
		if de, ok := domainerrors.IsDomainError(refreshErr); ok && de.Code() == domainerrors.CodeMVRefreshLocked {
			// Update log to FAILED.
			errStr := refreshErr.Error()
			_ = w.mvRepo.UpdateRefreshLog(ctx, logID, "FAILED", nil, &errStr, tenantID)
			// Write failed audit.
			if w.aw != nil {
				w.aw.Write(ctx, audit.Event{ //nolint:errcheck
					Action:     "REPORT.MV_REFRESH_FAILED",
					EntityType: "sys.mv_refresh_log",
					EntityID:   logID,
					After:      map[string]any{"mv_name": mvName, "error": refreshErr.Error(), "triggered_by": string(triggered)},
				})
			}
			return refreshErr
		}

		// Infra error.
		errStr := refreshErr.Error()
		_ = w.mvRepo.UpdateRefreshLog(ctx, logID, "FAILED", nil, &errStr, tenantID)
		if w.aw != nil {
			w.aw.Write(ctx, audit.Event{ //nolint:errcheck
				Action:     "REPORT.MV_REFRESH_FAILED",
				EntityType: "sys.mv_refresh_log",
				EntityID:   logID,
				After:      map[string]any{"mv_name": mvName, "error": errStr, "triggered_by": string(triggered)},
			})
		}
		return fmt.Errorf("%w: %v", asynq.SkipRetry, refreshErr)
	}

	// Success.
	rc := rowCount
	_ = w.mvRepo.UpdateRefreshLog(ctx, logID, "COMPLETED", &rc, nil, tenantID)
	w.logger.InfoContext(ctx, "worker.HandleMVRefresh: completed",
		"mv_name", mvName, "row_count", rowCount, "duration_ms", durationMs)
	return nil
}

// ─── Async Export Handler ─────────────────────────────────────────────────────

// HandleExportAsync processes a reporting:export-async task.
// S4-AC1: stream to MinIO + SMTP notif.
func (w *Worker) HandleExportAsync(ctx context.Context, t *asynq.Task) error {
	var p ExportWorkerPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("worker.HandleExportAsync: unmarshal: %w", err)
	}

	exportID, err := uuid.Parse(p.ExportLogID)
	if err != nil {
		return fmt.Errorf("worker.HandleExportAsync: parse export_log_id: %w", err)
	}
	actorID, _ := uuid.Parse(p.ActorID)
	format := ExportFormat(p.Format)
	if !format.IsValid() {
		return fmt.Errorf("%w: unsupported format %q", asynq.SkipRetry, p.Format)
	}

	// Build export file.
	username := p.ActorID // use ID as username fallback; M14 can resolve display name
	fileBytes, sha256Hex, _, err := w.svc.BuildInlineExport(ctx, p.ReportSlug, format, username)
	if err != nil {
		errStr := err.Error()
		_ = w.repo.UpdateExportLogFailed(ctx, exportID, errStr, p.TenantID)
		return fmt.Errorf("%w: build export: %v", asynq.SkipRetry, err)
	}

	// Upload to MinIO.
	var jobID string
	if rw := t.ResultWriter(); rw != nil {
		jobID = rw.TaskID()
	}
	if jobID == "" {
		jobID = uuid.New().String()
	}
	objectName := ExportObjectName(p.TenantID, p.ActorID, jobID, p.Format, time.Now())
	contentType := ContentTypeFor(format)
	if w.minio != nil {
		if err = w.minio.UploadExport(ctx, objectName, fileBytes, contentType); err != nil {
			errStr := err.Error()
			_ = w.repo.UpdateExportLogFailed(ctx, exportID, errStr, p.TenantID)
			return fmt.Errorf("worker.HandleExportAsync: upload: %w", err)
		}
	}

	// Generate presigned URL.
	ttl := time.Duration(w.svc.minioTTLHours) * time.Hour
	expiresAt := time.Now().Add(ttl)
	var signedURL string
	if w.minio != nil {
		signedURL, err = w.minio.PresignedGetURL(ctx, objectName, ttl)
		if err != nil {
			w.logger.WarnContext(ctx, "worker.HandleExportAsync: presign failed; continuing",
				"error", err)
			signedURL = ""
		}
	}

	// Update sys.export_log.
	rowCount := int64(len(strings.Split(string(fileBytes), "\n"))) // rough estimate
	if err = w.repo.UpdateExportLogCompleted(ctx, exportID,
		rowCount, objectName, sha256Hex, signedURL, expiresAt, p.TenantID); err != nil {
		return fmt.Errorf("worker.HandleExportAsync: update log: %w", err)
	}

	// Audit EXPORT.GENERATED in-transaction.
	if w.aw != nil {
		tx, txErr := w.repo.BeginTx(ctx)
		if txErr == nil {
			w.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
				Action:     "EXPORT.GENERATED",
				EntityType: "sys.export_log",
				EntityID:   exportID,
				After: map[string]any{
					"report_slug":    p.ReportSlug,
					"format":         p.Format,
					"row_count":      rowCount,
					"file_hash_sha256": sha256Hex,
					"actor":          p.ActorID,
				},
			})
			_ = tx.Commit()
		}
	}

	// SMTP notification.
	if w.smtp != nil {
		w.sendExportNotification(ctx, p, signedURL, sha256Hex, actorID)
	}

	w.logger.InfoContext(ctx, "worker.HandleExportAsync: completed",
		"export_id", exportID, "minio_path", objectName, "sha256", sha256Hex)
	return nil
}

// sendExportNotification sends the download email to the requesting user.
func (w *Worker) sendExportNotification(ctx context.Context, p ExportWorkerPayload, signedURL, sha256Hex string, actorID uuid.UUID) {
	subject, body, err := RenderEmailTemplate(
		DefaultSubjectTemplate, DefaultBodyTemplate,
		map[string]string{
			"report_slug": p.ReportSlug,
			"tanggal":     time.Now().Format("2006-01-02"),
			"file_hash":   sha256Hex,
			"opt_out_link": "", // not applicable for one-off export notification
		},
	)
	if err != nil {
		w.logger.WarnContext(ctx, "worker: render email template failed", "error", err)
		return
	}
	_ = actorID // TODO(M14): lookup email from sec.user by actorID
	w.logger.InfoContext(ctx, "worker: export notification: signed_url available",
		"signed_url_len", len(signedURL), "subject", subject, "body_len", len(body))
}

// ─── Scheduled Email Handler ──────────────────────────────────────────────────

// HandleScheduledEmailSend processes a reporting:scheduled-email-send task.
// S5-AC2: generate export + SMTP send.
// S5-AC3: retry via Asynq (MaxRetry=3); after 3 → DLQ + update last_status=FAILED.
func (w *Worker) HandleScheduledEmailSend(ctx context.Context, t *asynq.Task) error {
	var p ScheduledEmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("worker.HandleScheduledEmailSend: unmarshal: %w", err)
	}
	if p.TenantID == "" {
		p.TenantID = "TUGURE"
	}

	schedID, err := uuid.Parse(p.ScheduledEmailID)
	if err != nil {
		return fmt.Errorf("%w: invalid scheduled_email_id: %v", asynq.SkipRetry, err)
	}

	se, recipients, err := w.repo.GetScheduledEmail(ctx, schedID, p.TenantID)
	if err != nil {
		return fmt.Errorf("worker.HandleScheduledEmailSend: get sched: %w", err)
	}
	if se == nil {
		return fmt.Errorf("%w: scheduled_email %s not found", asynq.SkipRetry, schedID)
	}
	if !se.Active {
		return nil // deactivated between enqueue and execute
	}

	// Filter opt-outs.
	optOuts, err := w.repo.GetOptOuts(ctx, schedID)
	if err != nil {
		return fmt.Errorf("worker.HandleScheduledEmailSend: get optouts: %w", err)
	}
	optOutSet := make(map[string]bool, len(optOuts))
	for _, e := range optOuts {
		optOutSet[e] = true
	}
	var activeRecipients []string
	for _, r := range recipients {
		if !optOutSet[r] {
			activeRecipients = append(activeRecipients, r)
		}
	}
	if len(activeRecipients) == 0 {
		w.logger.InfoContext(ctx, "worker: all recipients opted out; skipping", "sched_id", schedID)
		return nil
	}

	// Generate export file.
	format := se.Format
	fileBytes, sha256Hex, _, err := w.svc.BuildInlineExport(ctx, se.ReportSlug, format, "system")
	if err != nil {
		_ = w.repo.UpdateScheduledEmailLastSent(ctx, schedID, "FAILED", p.TenantID)
		return fmt.Errorf("worker.HandleScheduledEmailSend: build export: %w", err)
	}

	// Render email.
	today := time.Now().Format("2006-01-02")
	subjTpl := DefaultSubjectTemplate
	bodyTpl := DefaultBodyTemplate
	if se.SubjectTemplate != nil && *se.SubjectTemplate != "" {
		subjTpl = *se.SubjectTemplate
	}
	if se.BodyTemplate != nil && *se.BodyTemplate != "" {
		bodyTpl = *se.BodyTemplate
	}
	subject, body, err := RenderEmailTemplate(subjTpl, bodyTpl, map[string]string{
		"report_slug": se.ReportSlug,
		"tanggal":     today,
		"file_hash":   sha256Hex,
		"opt_out_link": "", // TODO: generate per-recipient opt-out link
	})
	if err != nil {
		_ = w.repo.UpdateScheduledEmailLastSent(ctx, schedID, "FAILED", p.TenantID)
		return fmt.Errorf("worker.HandleScheduledEmailSend: render template: %w", err)
	}

	attachName := fmt.Sprintf("%s-%s.%s", se.ReportSlug, today, string(format))

	// Send SMTP.
	if w.smtp != nil {
		if err = w.smtp.SendEmail(ctx, activeRecipients, subject, body, attachName, fileBytes); err != nil {
			_ = w.repo.UpdateScheduledEmailLastSent(ctx, schedID, "FAILED", p.TenantID)
			return fmt.Errorf("worker.HandleScheduledEmailSend: smtp: %w", err)
		}
	}

	// Update last_sent_at + audit.
	_ = w.repo.UpdateScheduledEmailLastSent(ctx, schedID, "SENT", p.TenantID)

	if w.aw != nil {
		tx, txErr := w.repo.BeginTx(ctx)
		if txErr == nil {
			w.aw.WithTx(tx).Write(ctx, audit.Event{ //nolint:errcheck
				Action:     "SCHEDULED_EMAIL.SENT",
				EntityType: "sys.scheduled_email",
				EntityID:   schedID,
				After: map[string]any{
					"sched_id":         schedID.String(),
					"recipient_count":  len(activeRecipients),
					"file_hash_sha256": sha256Hex,
					"sent_at":          time.Now().Format(time.RFC3339),
				},
			})
			_ = tx.Commit()
		}
	}

	w.logger.InfoContext(ctx, "worker.HandleScheduledEmailSend: sent",
		"sched_id", schedID, "recipients", len(activeRecipients), "sha256", sha256Hex)
	return nil
}

// NewMVRefreshTask creates an Asynq task for MV refresh.
func NewMVRefreshTask(mvName string, triggered TriggeredBy, actorID, tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(MVRefreshPayload{
		MVName:       mvName,
		TriggeredBy:  string(triggered),
		TriggerActor: actorID,
		TenantID:     tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("NewMVRefreshTask: marshal: %w", err)
	}
	return asynq.NewTask(TaskMVRefresh, payload,
		asynq.MaxRetry(1), asynq.Timeout(30*time.Minute)), nil
}

// NewScheduledEmailTask creates an Asynq task for scheduled email send.
func NewScheduledEmailTask(schedID, tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(ScheduledEmailPayload{
		ScheduledEmailID: schedID,
		TenantID:         tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("NewScheduledEmailTask: marshal: %w", err)
	}
	return asynq.NewTask(TaskScheduledEmailSend, payload,
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Minute)), nil
}

// BuildExportBuffer builds the export file in-memory.
// Exported for unit testing.
func BuildExportBuffer(slug string, format ExportFormat, rows [][]string, headers []string, exportedAt time.Time, username string) ([]byte, string, error) {
	switch format {
	case FormatCSV:
		var buf bytes.Buffer
		sha, err := exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
			Headers: headers, Rows: rows, ExportedAt: exportedAt, Username: username,
		})
		if err != nil {
			return nil, "", err
		}
		return buf.Bytes(), sha, nil
	case FormatXLSX:
		fb, sha, err := exporter.ExportXLSX(exporter.ExportXLSXOptions{
			SheetName: "Data", Headers: headers, Rows: rows, ExportedAt: exportedAt, Username: username,
		})
		return fb, sha, err
	case FormatPDF:
		fb, sha, err := exporter.ExportPDF(exporter.ExportPDFOptions{
			Title: slug, Headers: headers, Rows: rows, ExportedAt: exportedAt, Username: username,
		})
		return fb, sha, err
	}
	return nil, "", fmt.Errorf("unsupported format: %s", format)
}
