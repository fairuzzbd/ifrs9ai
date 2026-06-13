package rollforward

// worker_tasks.go — Asynq worker for async roll-forward compute (Issue #88).
//
// When instrument count > asyncThreshold (1000), ComputeRollForward handler
// enqueues task TaskRollForwardCompute and returns 202 Accepted.
// This worker dequeues and runs the full compute, then stores the result in sys.job.
//
// Task payload: TaskPayload (JSON) — see type TaskPayload.
// Task result: Report stored as JSON in sys.job.result_jsonb (Phase 5: wire table).
//
// Per CLAUDE.md §3 long-running process rules:
//   - Backend: Asynq job, submit endpoint returns 202 + {jobId, statusUrl, streamUrl}.
//   - Worker: update progress, complete with result, or fail with error.
//   - Frontend: subscribe SSE (not in scope for this worker file).
//
// DEC-007: Asynq (Go-native, Redis-based).
// DEC-016: No float64.
// DEC-018: Audit trail.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskRollForwardCompute is the Asynq task type for async roll-forward compute.
// Registered on asynq.ServeMux when REDIS_URL is set (see cmd/api/main.go).
const TaskRollForwardCompute = "rollforward:compute"

// asyncThreshold is the instrument count above which computation is async.
// Instruments > 1000 → 202 + jobId. Instruments ≤ 1000 → sync 200.
// Per Issue #88 state machine §1.
const asyncThreshold = 1000

// TaskPayload is the JSON payload for TaskRollForwardCompute.
type TaskPayload struct {
	// CurrentCalcRunID is the calc run being analyzed.
	CurrentCalcRunID uuid.UUID `json:"currentCalcRunId"`

	// PriorCalcRunID is the prior SEALED run. nil = first period.
	PriorCalcRunID *uuid.UUID `json:"priorCalcRunId,omitempty"`

	// ActorID is the user who triggered the compute (for audit trail, DEC-018).
	ActorID uuid.UUID `json:"actorId"`

	// TraceID is the X-Trace-Id from the originating HTTP request.
	TraceID string `json:"traceId,omitempty"`

	// AllowNonSealedPrior mirrors ComputeRequest.AllowNonSealedPrior.
	AllowNonSealedPrior bool `json:"allowNonSealedPrior,omitempty"`
}

// AsyncJobResponse is the 202 body returned when compute is dispatched async.
type AsyncJobResponse struct {
	// JobID is the Asynq task ID (opaque string).
	JobID string `json:"jobId"`

	// StatusURL is the path for GET /api/v1/jobs/{jobId} (ux-patterns.md §3.2).
	StatusURL string `json:"statusUrl"`

	// StreamURL is the SSE path for live progress (ux-patterns.md §3.2).
	StreamURL string `json:"streamUrl"`

	// Count is the estimated instrument count that triggered async dispatch.
	Count int `json:"count"`
}

// Worker holds the roll-forward compute service for async task processing.
type Worker struct {
	svc    *Service
	logger *slog.Logger
}

// NewWorker creates a Worker. Panics if svc is nil.
func NewWorker(svc *Service, logger *slog.Logger) *Worker {
	if svc == nil {
		panic("rollforward.NewWorker: svc must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, logger: logger}
}

// HandleComputeRollForward is the Asynq handler for TaskRollForwardCompute.
// It unmarshals the payload, calls svc.ComputeRollForward, and logs the result.
//
// In Phase 5: persist report to sys.job.result_jsonb + update job progress via Redis pub/sub.
// For now: best-effort log on success/failure (sys.job table not yet created in this migration).
//
// Registered on asynq.ServeMux:
//
//	asynqMux.HandleFunc(rollforward.TaskRollForwardCompute, rfWorker.HandleComputeRollForward)
func (w *Worker) HandleComputeRollForward(ctx context.Context, t *asynq.Task) error {
	var payload TaskPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("rollforward.Worker.HandleComputeRollForward: unmarshal payload: %w", err)
	}

	w.logger.InfoContext(ctx, "rollforward worker: starting async compute",
		"currentCalcRunId", payload.CurrentCalcRunID,
		"priorCalcRunId", payload.PriorCalcRunID,
		"actorId", payload.ActorID,
		"traceId", payload.TraceID,
	)

	report, err := w.svc.ComputeRollForward(ctx, ComputeRequest{
		CurrentCalcRunID:    payload.CurrentCalcRunID,
		PriorCalcRunID:      payload.PriorCalcRunID,
		DetectionMethod:     DetectionMethodBasicStatusDiff,
		AllowNonSealedPrior: payload.AllowNonSealedPrior,
		ActorID:             payload.ActorID,
	})
	if err != nil {
		w.logger.ErrorContext(ctx, "rollforward worker: compute failed",
			"currentCalcRunId", payload.CurrentCalcRunID,
			"error", err,
		)
		return fmt.Errorf("rollforward.Worker.HandleComputeRollForward: compute: %w", err)
	}

	// Phase 5: persist report JSON to sys.job.result_jsonb + update progress.
	// For now: log completion (job result accessible via re-triggering GET endpoint).
	w.logger.InfoContext(ctx, "rollforward worker: compute completed",
		"reportId", report.ReportID,
		"reconcileStatus", string(report.ReconcileStatus),
		"closingEclIdr", report.ClosingEclIdr.StringFixed(4),
		"warnings", len(report.Warnings),
	)

	return nil
}

// NewRollForwardTask creates a new Asynq task for TaskRollForwardCompute.
// Used by the HTTP handler to enqueue the task.
func NewRollForwardTask(payload TaskPayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("rollforward.NewRollForwardTask: marshal payload: %w", err)
	}
	return asynq.NewTask(TaskRollForwardCompute, b), nil
}
