// worker_tasks.go — Asynq job handlers for P4-M6 EIR drift detection (M6-002).
//
// TaskDriftCron:   daily cron at 19:00 UTC (= 02:00 WIB).
//
//	Registered in main.go ServeMux with CronTask("0 19 * * *", ...).
//
// TaskDriftAdHoc: ad-hoc drift generation triggered via POST /ecl/eir/drift-reports/generate.
//
// Both tasks invoke DriftService.GenerateReport with appropriate DriftGenerateRequest.
// Job progress is NOT SSE-streamed (drift is advisory-only per M6 decisions).
//
// References:
//   - docs/stories/phase-4/M6-eir-amendment-lifecycle.md §M6-002
//   - docs/state-machines/p4-m6-amendment-lifecycle.md §7 (cron schedule "0 19 * * *")
//   - DEC-007 (Asynq), DEC-016 (decimal).
package eir

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Task type constants for Asynq queue.
const (
	// TaskDriftCron is the Asynq task type for the daily EIR drift cron job.
	// Schedule: "0 19 * * *" (19:00 UTC = 02:00 WIB, per state-machine §7).
	TaskDriftCron = "eir:drift_cron"

	// TaskDriftAdHoc is the Asynq task type for ad-hoc drift generation.
	// Triggered by POST /ecl/eir/drift-reports/generate.
	TaskDriftAdHoc = "eir:drift_adhoc"
)

// DriftJobPayload is the JSON payload for both cron and ad-hoc drift tasks.
type DriftJobPayload struct {
	TriggerSource string  `json:"trigger_source"`
	TriggeredBy   *string `json:"triggered_by,omitempty"` // UUID string; nil for cron
	TenantID      string  `json:"tenant_id"`
	JobID         string  `json:"job_id"` // sys.job.id for progress tracking
}

// DriftCronHandler handles both TaskDriftCron and TaskDriftAdHoc Asynq tasks.
type DriftCronHandler struct {
	driftSvc *DriftService
	logger   *slog.Logger
}

// NewDriftCronHandler creates a DriftCronHandler.
func NewDriftCronHandler(driftSvc *DriftService, logger *slog.Logger) *DriftCronHandler {
	return &DriftCronHandler{driftSvc: driftSvc, logger: logger}
}

// HandleDriftCronTask processes the daily EIR drift cron task.
// trigger_source = CRON_DAILY; triggered_by = nil.
func (h *DriftCronHandler) HandleDriftCronTask(ctx context.Context, t *asynq.Task) error {
	return h.handle(ctx, t, DriftTriggerCronDaily)
}

// HandleDriftAdHocTask processes an ad-hoc drift generation request.
// trigger_source = MANUAL_AD_HOC; triggered_by = actorID from payload.
func (h *DriftCronHandler) HandleDriftAdHocTask(ctx context.Context, t *asynq.Task) error {
	return h.handle(ctx, t, DriftTriggerManualAdHoc)
}

func (h *DriftCronHandler) handle(ctx context.Context, t *asynq.Task, defaultTrigger DriftTriggerSource) error {
	var payload DriftJobPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("drift handler: unmarshal payload: %w", err)
	}

	triggerSource := defaultTrigger
	if payload.TriggerSource != "" {
		triggerSource = DriftTriggerSource(payload.TriggerSource)
	}

	var triggeredBy *uuid.UUID
	if payload.TriggeredBy != nil && *payload.TriggeredBy != "" {
		id, err := uuid.Parse(*payload.TriggeredBy)
		if err == nil {
			triggeredBy = &id
		}
	}

	req := DriftGenerateRequest{
		TriggerSource: triggerSource,
		TriggeredBy:   triggeredBy,
		TenantID:      payload.TenantID,
	}

	report, err := h.driftSvc.GenerateReport(ctx, req)
	if err != nil {
		h.logger.Error("eir drift task failed",
			slog.String("task_type", t.Type()),
			slog.String("trigger", string(triggerSource)),
			slog.Any("error", err),
		)
		return err
	}

	h.logger.Info("eir drift task completed",
		slog.String("task_type", t.Type()),
		slog.String("report_id", report.ID.String()),
		slog.Int("low", report.DriftLowCount),
		slog.Int("high", report.DriftHighCount),
	)
	return nil
}

// NewDriftCronTask builds an Asynq task for the daily cron job (no actor — system-triggered).
func NewDriftCronTask(tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(DriftJobPayload{
		TriggerSource: string(DriftTriggerCronDaily),
		TenantID:      tenantID,
		JobID:         uuid.New().String(),
	})
	if err != nil {
		return nil, fmt.Errorf("NewDriftCronTask: %w", err)
	}
	return asynq.NewTask(TaskDriftCron, payload), nil
}

// NewDriftAdHocTask builds an Asynq task for an ad-hoc drift generation.
func NewDriftAdHocTask(tenantID string, actorID uuid.UUID) (*asynq.Task, error) {
	actorStr := actorID.String()
	payload, err := json.Marshal(DriftJobPayload{
		TriggerSource: string(DriftTriggerManualAdHoc),
		TriggeredBy:   &actorStr,
		TenantID:      tenantID,
		JobID:         uuid.New().String(),
	})
	if err != nil {
		return nil, fmt.Errorf("NewDriftAdHocTask: %w", err)
	}
	return asynq.NewTask(TaskDriftAdHoc, payload), nil
}
