package kurs

// worker_p5m5.go — Asynq task handlers for P5-M5 FX rate background jobs.
//
// Task types:
//   - fx:jisdor-fetch     — daily cron 10:30 WIB (03:30 UTC), Mon-Fri
//   - fx:upload-process   — async processing of manual upload file (future)
//
// Progress tracking: updates sys.job table (UX rule §3).
// DLQ: failures after 3 retries → sys.dlq_fx_jisdor.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// FxJisdorWorker handles JISDOR-fetch and upload-process Asynq tasks.
type FxJisdorWorker struct {
	svc    *Service
	logger *slog.Logger
}

// NewFxJisdorWorker creates a new FxJisdorWorker.
func NewFxJisdorWorker(svc *Service, logger *slog.Logger) *FxJisdorWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &FxJisdorWorker{svc: svc, logger: logger}
}

// RegisterHandlers wires task types to their handlers on the given mux.
func (w *FxJisdorWorker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskFxJisdorFetch, w.HandleJisdorFetchTask)
	mux.HandleFunc(TaskFxUploadProcess, w.HandleUploadProcessTask)
}

// ─── JisdorFetchPayload ───────────────────────────────────────────────────────

// JisdorFetchPayload is the Asynq task payload for fx:jisdor-fetch.
type JisdorFetchPayload struct {
	TanggalBerlaku string `json:"tanggal_berlaku"` // "YYYY-MM-DD"
	TenantID       string `json:"tenant_id"`
}

// NewJisdorFetchTask builds an Asynq task for the given date.
func NewJisdorFetchTask(tanggal, tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(JisdorFetchPayload{
		TanggalBerlaku: tanggal,
		TenantID:       tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("NewJisdorFetchTask: marshal payload: %w", err)
	}
	return asynq.NewTask(TaskFxJisdorFetch, payload), nil
}

// HandleJisdorFetchTask processes the daily JISDOR fetch cron.
//
// Flow:
//  1. Parse payload.
//  2. Validate date (weekend/holiday check delegated to service).
//  3. Call svc.JISDORFetchAll with real JISDOR adapter.
//  4. Log result. Errors for individual currencies are non-fatal (logged).
//  5. On total failure → written to DLQ by service.
//
// Asynq will retry this task on error (up to MaxRetry = 3 by default).
// After 3 failures the task is archived (dead-letter) and DLQ entry is written.
func (w *FxJisdorWorker) HandleJisdorFetchTask(ctx context.Context, t *asynq.Task) error {
	var p JisdorFetchPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		w.logger.ErrorContext(ctx, "fx:jisdor-fetch: unmarshal payload failed",
			"error", err)
		return fmt.Errorf("HandleJisdorFetchTask: unmarshal: %w", err)
	}

	if p.TanggalBerlaku == "" {
		// Cron-triggered task: use today's date (worker injects it via scheduler)
		return fmt.Errorf("HandleJisdorFetchTask: tanggal_berlaku is empty in payload")
	}

	w.logger.InfoContext(ctx, "fx:jisdor-fetch: starting",
		"tanggal", p.TanggalBerlaku, "tenant_id", p.TenantID)

	provider := NewJISDORAdapter()
	result, err := w.svc.JISDORFetchAll(ctx, p.TanggalBerlaku, provider)
	if err != nil {
		w.logger.ErrorContext(ctx, "fx:jisdor-fetch: JISDORFetchAll failed",
			"tanggal", p.TanggalBerlaku, "error", err)
		// Return error so Asynq retries (up to MaxRetry).
		// DLQ entry written by service.JISDORFetchAll on provider error.
		return fmt.Errorf("HandleJisdorFetchTask: %w", err)
	}

	w.logger.InfoContext(ctx, "fx:jisdor-fetch: completed",
		"tanggal", p.TanggalBerlaku,
		"inserted", result.Inserted,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
		"auto_approved", result.AutoApproved,
	)

	if len(result.Errors) > 0 {
		for _, fe := range result.Errors {
			w.logger.WarnContext(ctx, "fx:jisdor-fetch: per-currency error",
				"kode", fe.KodeMataUang, "error", fe.Error)
		}
	}

	return nil
}

// ─── UploadProcessPayload ─────────────────────────────────────────────────────

// UploadProcessPayload is the Asynq task payload for fx:upload-process.
// Used for async large-file processing (> 200 rows → async per UX rule §3).
type UploadProcessPayload struct {
	BatchID   string `json:"batch_id"`
	S3Key     string `json:"s3_key"`   // MinIO object key
	TenantID  string `json:"tenant_id"`
	ActorID   string `json:"actor_id"`
}

// HandleUploadProcessTask processes an async manual upload file from MinIO.
// Stub in P5-M5 — inline upload (< 200 rows) is the primary path.
// Async upload (> 200 rows via MinIO) is planned for P5-M11.
func (w *FxJisdorWorker) HandleUploadProcessTask(ctx context.Context, t *asynq.Task) error {
	var p UploadProcessPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("HandleUploadProcessTask: unmarshal: %w", err)
	}

	w.logger.WarnContext(ctx, "fx:upload-process: async MinIO processing not yet implemented (P5-M11)",
		"batch_id", p.BatchID, "s3_key", p.S3Key)

	// TODO(P5-M11): download file from MinIO, parse rows, call svc.UploadManual.
	return nil
}
