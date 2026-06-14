package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// Worker adalah Asynq task handler untuk TaskTypeNotify.
// Dipasang ke asynq.ServeMux di main.go (Phase 2 worker binary).
type Worker struct {
	svc    *Service
	logger *slog.Logger
}

// NewWorker membuat Worker.
func NewWorker(svc *Service, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, logger: logger}
}

// ProcessTask adalah implementasi asynq.Handler untuk TaskTypeNotify.
// Dipanggil oleh Asynq worker saat ada task notification:deliver di queue.
//
// Retry: Asynq mengelola retries (max 5 dengan exponential backoff).
// Setelah max retry, task masuk dead-letter dan alert ROLE-IT-ADMIN.
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var req DeliveryRequest
	if err := json.Unmarshal(t.Payload(), &req); err != nil {
		// Parse error tidak bisa di-retry — return nil agar tidak masuk infinite retry.
		w.logger.ErrorContext(ctx, "notification worker: failed to unmarshal payload",
			"error", err,
			"payload", string(t.Payload()),
		)
		return nil
	}

	if err := w.svc.Deliver(ctx, req); err != nil {
		w.logger.WarnContext(ctx, "notification worker: deliver failed, will retry",
			"templateCode", req.TemplateCode,
			"channel", req.Channel,
			"traceId", req.TraceID,
			"error", err,
		)
		// Return error agar Asynq melakukan retry.
		return fmt.Errorf("notification worker: deliver: %w", err)
	}

	return nil
}

// RegisterHandler mendaftarkan notification worker ke asynq.ServeMux.
// Dipanggil dari worker binary setup.
func RegisterHandler(mux *asynq.ServeMux, w *Worker) {
	mux.HandleFunc(TaskTypeNotify, w.ProcessTask)
}
