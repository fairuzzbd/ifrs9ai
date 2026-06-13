package calcrun

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	eclcore "blips-ifrs9.tugu-re.com/internal/ecl/core"
)

// worker_tasks.go — Asynq task handler for ECL_CALC_RUN tasks (P4-M8).
//
// Task type: "ecl:bulk_compute" (TaskNameECLBulkCompute from M7).
// Payload: eclcore.TaskECLBulkComputePayload.
//
// Workflow:
//  1. Unmarshal payload (calcRunID, periodeID, evaluationDate, scope, actorID).
//  2. Delegate to M7 ECLOrchestrator.ComputeBulk via progressFn callback.
//  3. On batch progress: update ecl.calc_run.processed_count + sys.job progress.
//  4. On completion: call Service.MarkCompleted → status COMPLETED or COMPLETED_WITH_ERRORS.
//  5. Update sys.job status=completed + result_jsonb.
//
// Cancellation: ctx.Done() checked by M7 ComputeBulk goroutines.
// Partial results: ecl.calc_result_line rows committed per instrument are preserved on cancel.
//
// Hard rules:
//   - No float64 (all ECL amounts flow through decimal in M7).
//   - Sealed guard: M7.ComputeBulk checks IsSealedCalcRun before processing.

// Worker handles the ECL bulk compute Asynq task.
type Worker struct {
	service      *Service
	orchestrator *eclcore.ECLOrchestrator
	jobUpdater   JobProgressUpdater
	logger       *slog.Logger
}

// NewWorker creates a Worker. Panics on nil service or orchestrator.
func NewWorker(
	service *Service,
	orchestrator *eclcore.ECLOrchestrator,
	jobUpdater JobProgressUpdater,
	logger *slog.Logger,
) *Worker {
	if service == nil {
		panic("calcrun.NewWorker: service must not be nil")
	}
	if orchestrator == nil {
		panic("calcrun.NewWorker: orchestrator must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	jpu := jobUpdater
	if jpu == nil {
		jpu = &noopJobUpdater{}
	}
	return &Worker{
		service:      service,
		orchestrator: orchestrator,
		jobUpdater:   jpu,
		logger:       logger,
	}
}

// Handle implements asynq.Handler for task type "ecl:bulk_compute".
func (w *Worker) Handle(ctx context.Context, t *asynq.Task) error {
	var payload eclcore.TaskECLBulkComputePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("calcrun.Worker: unmarshal payload: %w", err)
	}

	w.logger.InfoContext(ctx, "calcrun.worker: starting bulk compute",
		"calc_run_id", payload.CalcRunID,
		"periode_id", payload.PeriodeID,
		"job_id", payload.JobID,
	)

	// Update sys.job: running.
	if err := w.jobUpdater.UpdateStatus(ctx, payload.JobID, "running", 0, "Memulai bulk compute..."); err != nil {
		w.logger.WarnContext(ctx, "calcrun.worker: job status update failed", "error", err)
	}

	// Build scope from payload.
	var scope *eclcore.BulkScope
	if len(payload.InstrumenIDs) > 0 || len(payload.PortofolioIDs) > 0 {
		scope = &eclcore.BulkScope{
			PortofolioIDs: payload.PortofolioIDs,
			InstrumenIDs:  payload.InstrumenIDs,
		}
	}

	// progressFn: updates ecl.calc_run.processed_count + sys.job progress.
	progressFn := func(processed, total int, currentStep string) {
		pct := 0
		if total > 0 {
			pct = processed * 100 / total
		}
		// Non-blocking progress update (best-effort).
		if err := w.service.UpdateProgress(ctx, payload.CalcRunID, processed, 0, payload.ActorID); err != nil {
			w.logger.WarnContext(ctx, "calcrun.worker: progress update failed", "error", err)
		}
		if err := w.jobUpdater.UpdateStatus(ctx, payload.JobID, "running", pct, currentStep); err != nil {
			w.logger.WarnContext(ctx, "calcrun.worker: job progress update failed", "error", err)
		}
	}

	// Delegate to M7 ComputeBulk.
	req := eclcore.BulkComputeRequest{
		CalcRunID:      payload.CalcRunID,
		EvaluationDate: payload.EvaluationDate,
		PeriodeID:      payload.PeriodeID,
		Scope:          scope,
		ActorID:        payload.ActorID,
	}

	result, err := w.orchestrator.ComputeBulk(ctx, req, progressFn)
	if err != nil {
		// Bulk compute infrastructure error (not per-instrument error).
		w.logger.ErrorContext(ctx, "calcrun.worker: ComputeBulk fatal error",
			"calc_run_id", payload.CalcRunID,
			"error", err,
		)
		if err2 := w.jobUpdater.MarkFailed(ctx, payload.JobID, "ECL_BULK_COMPUTE_FAILED", err.Error()); err2 != nil {
			w.logger.WarnContext(ctx, "calcrun.worker: job mark-failed error", "error", err2)
		}
		return fmt.Errorf("calcrun.worker: ComputeBulk: %w", err)
	}

	if result.Status == "cancelled" {
		// Calc run was cancelled mid-flight; MarkCompleted was already called via Cancel().
		w.logger.InfoContext(ctx, "calcrun.worker: bulk compute cancelled",
			"calc_run_id", payload.CalcRunID,
			"partial_computed", result.TotalComputed,
		)
		if err := w.jobUpdater.UpdateStatus(ctx, payload.JobID, "cancelled", result.TotalComputed*100/max(result.TotalScanned, 1), "Dibatalkan oleh user"); err != nil {
			w.logger.WarnContext(ctx, "calcrun.worker: job cancel update failed", "error", err)
		}
		return nil
	}

	// Mark calc_run completed.
	errorCount := len(result.Errors)
	if _, err := w.service.MarkCompleted(ctx, payload.CalcRunID, result.TotalComputed, errorCount, payload.ActorID); err != nil {
		w.logger.ErrorContext(ctx, "calcrun.worker: MarkCompleted failed",
			"calc_run_id", payload.CalcRunID,
			"error", err,
		)
		// Non-fatal: job result still reported.
	}

	// Build result summary for sys.job.
	errSummary := make([]map[string]any, 0, len(result.Errors))
	for _, e := range result.Errors {
		errSummary = append(errSummary, map[string]any{
			"instrumen_id":  e.InstrumenID.String(),
			"error_code":    e.ErrorCode,
			"error_message": e.ErrorMessage,
		})
	}

	jobResult := map[string]any{
		"calc_run_id":            payload.CalcRunID.String(),
		"total_scanned":          result.TotalScanned,
		"total_computed":         result.TotalComputed,
		"total_skipped_fvtpl":    result.TotalSkippedFVTPL,
		"total_poci_deferred":    result.TotalPOCIDeferred,
		"error_count":            errorCount,
		"ecl_weighted_idr_total": result.ECLWeightedIDRTotal.StringFixed(4),
		"status":                 result.Status,
		"errors":                 errSummary,
	}

	if err := w.jobUpdater.MarkCompleted(ctx, payload.JobID, jobResult); err != nil {
		w.logger.WarnContext(ctx, "calcrun.worker: job mark-completed error", "error", err)
	}

	w.logger.InfoContext(ctx, "calcrun.worker: bulk compute finished",
		"calc_run_id", payload.CalcRunID,
		"status", result.Status,
		"computed", result.TotalComputed,
		"errors", errorCount,
		"ecl_total_idr", result.ECLWeightedIDRTotal.StringFixed(4),
	)
	return nil
}

// ─── Asynq task payload type (re-exported alias) ──────────────────────────────

// TaskCalcRunBulkCompute is the task type name for ECL bulk compute.
// Same as eclcore.TaskNameECLBulkCompute — re-exported for M8 wiring in main.go.
const TaskCalcRunBulkCompute = eclcore.TaskNameECLBulkCompute

// NewCalcRunBulkTask creates an Asynq task for the given payload.
// Convenience wrapper for main.go wiring.
func NewCalcRunBulkTask(calcRunID uuid.UUID, periodeID, jobID string, actorID uuid.UUID) (*asynq.Task, error) {
	payload := eclcore.TaskECLBulkComputePayload{
		JobID:     jobID,
		CalcRunID: calcRunID,
		PeriodeID: periodeID,
		ActorID:   actorID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("calcrun.NewCalcRunBulkTask: marshal: %w", err)
	}
	return asynq.NewTask(TaskCalcRunBulkCompute, b), nil
}
