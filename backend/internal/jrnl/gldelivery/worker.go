// Package gldelivery — Worker: Asynq task handlers for P5-M3 GL Host REST Delivery.
//
// Tasks:
//   - "gl_delivery:deliver"         — POST one jurnal entry to GL Host (triggered per JURNAL.POSTED).
//   - "gl_delivery:reconcile-daily" — Daily reconciliation BLIPS vs GL Host (cron 01:00 UTC).
//
// Error policy (per state machine P5-M3):
//   - Domain errors (GL Host 4xx) → SkipRetry, write DLQ immediately.
//   - Infra errors (5xx / timeout) → return error to Asynq (3x exponential backoff: 30s/120s/600s), then DLQ.
//
// Cron schedule: "0 1 * * *" (01:00 UTC = 08:00 WIB).
package gldelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

const (
	// TaskGLDelivery is the Asynq task type for delivering one jurnal to GL Host.
	TaskGLDelivery = "gl_delivery:deliver"
	// TaskGLReconcileDaily is the Asynq task type for the daily reconciliation cron.
	TaskGLReconcileDaily = "gl_delivery:reconcile-daily"
)

// deliverPayload is the Asynq task payload for TaskGLDelivery.
type deliverPayload struct {
	JurnalHeaderID string `json:"jurnal_header_id"`
}

// reconcilePayload is the Asynq task payload for TaskGLReconcileDaily.
type reconcilePayload struct {
	ReportID string `json:"report_id"`
	Date     string `json:"date"` // YYYY-MM-DD
	TenantID string `json:"tenant_id"`
}

// GLDeliveryWorker handles GL delivery + reconciliation Asynq tasks.
type GLDeliveryWorker struct { //nolint:revive
	delivery *DeliveryService
	recon    *ReconciliationService
	cfg      Config
	logger   *slog.Logger
}

// NewGLDeliveryWorker creates a GLDeliveryWorker. Panics on nil deps.
func NewGLDeliveryWorker(
	delivery *DeliveryService,
	recon *ReconciliationService,
	cfg Config,
	logger *slog.Logger,
) *GLDeliveryWorker {
	if delivery == nil {
		panic("gldelivery.NewGLDeliveryWorker: delivery must not be nil")
	}
	if recon == nil {
		panic("gldelivery.NewGLDeliveryWorker: recon must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GLDeliveryWorker{
		delivery: delivery,
		recon:    recon,
		cfg:      cfg,
		logger:   logger,
	}
}

// RegisterHandlers registers all GL delivery task handlers on the given Asynq ServeMux.
func (w *GLDeliveryWorker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskGLDelivery, w.HandleDeliverTask)
	mux.HandleFunc(TaskGLReconcileDaily, w.HandleReconcileDailyTask)
}

// HandleDeliverTask processes a single gl_delivery:deliver task.
// Error policy: domain errors (4xx) → SkipRetry + DLQ; infra errors → Asynq retry.
func (w *GLDeliveryWorker) HandleDeliverTask(ctx context.Context, t *asynq.Task) error {
	var p deliverPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("gldelivery.Worker.HandleDeliverTask: unmarshal: %w", err)
	}

	headerID, err := uuid.Parse(p.JurnalHeaderID)
	if err != nil {
		return fmt.Errorf("gldelivery.Worker.HandleDeliverTask: invalid jurnal_header_id %q: %w", p.JurnalHeaderID, err)
	}

	w.logger.InfoContext(ctx, "gldelivery.worker: deliver task received", "headerID", headerID)

	deliveryErr := w.delivery.DeliverToGL(workerContext(ctx), headerID)
	if deliveryErr == nil {
		return nil
	}

	// Classify error for Asynq retry vs SkipRetry.
	de, isDomain := domainerrors.IsDomainError(deliveryErr)
	if isDomain {
		switch de.Code() {
		case domainerrors.CodeGLDeliveryHost4XX,
			domainerrors.CodeGLDeliveryAuthFailed,
			domainerrors.CodeGLDeliveryMaxAttemptsExceeded:
			// Domain / budget exhausted — do NOT retry via Asynq.
			w.logger.WarnContext(ctx, "gldelivery.worker: domain error → SkipRetry",
				"headerID", headerID, "code", de.Code())
			return fmt.Errorf("%w: %v", asynq.SkipRetry, deliveryErr)
		}
	}

	// Infra error: let Asynq retry with exponential backoff.
	w.logger.WarnContext(ctx, "gldelivery.worker: infra error → Asynq retry",
		"headerID", headerID, "error", deliveryErr)
	return deliveryErr
}

// HandleReconcileDailyTask processes a single gl_delivery:reconcile-daily task.
func (w *GLDeliveryWorker) HandleReconcileDailyTask(ctx context.Context, t *asynq.Task) error {
	var p reconcilePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// Cron task: payload may be nil (scheduler enqueue without body for daily cron).
		// Fall back to yesterday's date.
		p.Date = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
		p.TenantID = "TUGURE"
		p.ReportID = ""
	}

	date, err := time.Parse("2006-01-02", p.Date)
	if err != nil {
		date = time.Now().UTC().Add(-24 * time.Hour)
		date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}

	tenantID := "TUGURE"
	if p.TenantID != "" {
		tenantID = p.TenantID
	}

	w.logger.InfoContext(ctx, "gldelivery.worker: reconcile-daily task received",
		"date", date.Format("2006-01-02"), "tenantID", tenantID)

	wCtx := workerContext(ctx)

	// If reportID provided (triggered by TriggerAsync), run for existing report.
	// Otherwise, trigger a new one (cron path).
	if p.ReportID != "" {
		reportID, parseErr := uuid.Parse(p.ReportID)
		if parseErr == nil {
			return w.recon.RunReconciliation(wCtx, reportID, date, tenantID)
		}
	}

	// Cron path: trigger via service (creates report + runs sync since we're already in worker).
	resp, triggerErr := w.recon.TriggerAsync(wCtx, date, "CRON", nil, tenantID)
	if triggerErr != nil {
		// CodeGLReconciliationInProgress means already running — not an error from cron perspective.
		if de, ok := domainerrors.IsDomainError(triggerErr); ok && de.Code() == domainerrors.CodeGLReconciliationInProgress {
			w.logger.InfoContext(ctx, "gldelivery.worker: reconciliation already in progress — skip",
				"date", date.Format("2006-01-02"))
			return nil
		}
		return fmt.Errorf("gldelivery.Worker.HandleReconcileDailyTask: trigger: %w", triggerErr)
	}

	// Since we already have a worker context, run the computation inline
	// rather than re-queuing (avoids double-enqueue from cron path).
	if resp != nil {
		w.logger.InfoContext(ctx, "gldelivery.worker: reconcile-daily triggered",
			"jobID", resp.JobID, "date", resp.TanggalRekonsiliasi)
	}
	return nil
}

// NewDeliverTask constructs an Asynq task for delivering a single jurnal.
func NewDeliverTask(jurnalHeaderID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(deliverPayload{JurnalHeaderID: jurnalHeaderID.String()})
	if err != nil {
		return nil, fmt.Errorf("gldelivery.NewDeliverTask: marshal: %w", err)
	}
	return asynq.NewTask(
		TaskGLDelivery, payload,
		asynq.MaxRetry(3),
		asynq.Timeout(5*time.Minute),
	), nil
}

// NewReconcileDailyTask constructs the Asynq task for the daily recon cron.
func NewReconcileDailyTask(date time.Time, tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(reconcilePayload{
		Date:     date.Format("2006-01-02"),
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("gldelivery.NewReconcileDailyTask: marshal: %w", err)
	}
	return asynq.NewTask(
		TaskGLReconcileDaily, payload,
		asynq.MaxRetry(1),
		asynq.Timeout(30*time.Minute),
	), nil
}
