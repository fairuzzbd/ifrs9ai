package pocidelta

// worker.go — Asynq task handler for POCI delta batch computation (P5-M10).
//
// Task: poci:compute-delta-batch
// Triggered by: POST /api/v1/poci/compute-delta-batch (handler.go)
// Schedule: on-demand only (not a cron job — triggered per calc run seal)
//
// Progress reported via Redis pub/sub + sys.job table (UX §3).
// Per-instrument errors go to CalcRunErrors slice — batch does not halt.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// TaskComputeDeltaBatch is the Asynq task type for POCI delta batch computation.
const TaskComputeDeltaBatch = "poci:compute-delta-batch"

// ComputeDeltaPayload is the Asynq task payload.
type ComputeDeltaPayload struct {
	CalcRunID string `json:"calc_run_id"`
	ActorID   string `json:"actor_id"`
	TenantID  string `json:"tenant_id"`
	JobID     string `json:"job_id"`
}

// NewComputeDeltaTask creates an Asynq task for delta batch computation.
func NewComputeDeltaTask(calcRunID, actorID, tenantID, jobID uuid.UUID) (*asynq.Task, error) {
	p, err := json.Marshal(ComputeDeltaPayload{
		CalcRunID: calcRunID.String(),
		ActorID:   actorID.String(),
		TenantID:  tenantID.String(),
		JobID:     jobID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("NewComputeDeltaTask: marshal payload: %w", err)
	}
	return asynq.NewTask(TaskComputeDeltaBatch, p,
		asynq.MaxRetry(1),         // only 1 retry — idempotency via duplicate check
		asynq.Timeout(30*time.Minute),
		asynq.Queue("critical"),   // POCI delta is compliance-critical
	), nil
}

// Worker holds the Asynq task handler for POCI delta.
type Worker struct {
	svc    *Service
	redis  *redis.Client
	logger *slog.Logger
}

// NewWorker creates a new pocidelta Worker.
func NewWorker(svc *Service, rdb *redis.Client, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, redis: rdb, logger: logger}
}

// RegisterHandlers registers all POCI delta task handlers with an Asynq mux.
func (w *Worker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskComputeDeltaBatch, w.HandleComputeDeltaBatch)
}

// HandleComputeDeltaBatch handles the poci:compute-delta-batch Asynq task.
func (w *Worker) HandleComputeDeltaBatch(ctx context.Context, t *asynq.Task) error {
	var p ComputeDeltaPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("HandleComputeDeltaBatch: unmarshal payload: %w", err)
	}

	calcRunID, err := uuid.Parse(p.CalcRunID)
	if err != nil {
		return fmt.Errorf("HandleComputeDeltaBatch: invalid calc_run_id: %w", err)
	}
	actorID, err := uuid.Parse(p.ActorID)
	if err != nil {
		actorID = systemActorID
	}

	w.logger.InfoContext(ctx, "poci:compute-delta-batch started",
		"calc_run_id", p.CalcRunID,
		"job_id", p.JobID,
	)

	// Report progress: started
	w.updateJobProgress(ctx, p.JobID, 0, "Memulai POCI delta batch computation...")

	calcErrors, runErr := w.svc.ComputeDeltaForCalcRun(ctx, calcRunID, actorID, p.TenantID)
	if runErr != nil {
		w.logger.ErrorContext(ctx, "poci:compute-delta-batch fatal error",
			"calc_run_id", p.CalcRunID,
			"error", runErr.Error(),
		)
		w.updateJobFailed(ctx, p.JobID, runErr.Error())
		return runErr
	}

	w.logger.InfoContext(ctx, "poci:compute-delta-batch completed",
		"calc_run_id", p.CalcRunID,
		"per_instrument_errors", len(calcErrors),
	)

	w.updateJobComplete(ctx, p.JobID, len(calcErrors))
	return nil
}

// ─── Progress helpers ─────────────────────────────────────────────────────────

func (w *Worker) updateJobProgress(ctx context.Context, jobID string, pct int, step string) {
	if jobID == "" || w.redis == nil {
		return
	}
	key := "job:" + jobID
	_ = w.redis.HSet(ctx, key, map[string]interface{}{
		"status":      "running",
		"progress":    pct,
		"currentStep": step,
		"updatedAt":   time.Now().UTC().Unix(),
	})
	_ = w.redis.Publish(ctx, "job-events:"+jobID, fmt.Sprintf(`{"event":"progress","progress":%d}`, pct))
}

func (w *Worker) updateJobComplete(ctx context.Context, jobID string, errorCount int) {
	if jobID == "" || w.redis == nil {
		return
	}
	key := "job:" + jobID
	_ = w.redis.HSet(ctx, key, map[string]interface{}{
		"status":      "completed",
		"progress":    100,
		"currentStep": "Selesai",
		"errorCount":  errorCount,
		"updatedAt":   time.Now().UTC().Unix(),
	})
	_ = w.redis.Expire(ctx, key, 24*time.Hour)
	_ = w.redis.Publish(ctx, "job-events:"+jobID, `{"event":"completed","progress":100}`)
}

func (w *Worker) updateJobFailed(ctx context.Context, jobID string, errMsg string) {
	if jobID == "" || w.redis == nil {
		return
	}
	key := "job:" + jobID
	_ = w.redis.HSet(ctx, key, map[string]interface{}{
		"status":    "failed",
		"error":     errMsg,
		"updatedAt": time.Now().UTC().Unix(),
	})
	_ = w.redis.Publish(ctx, "job-events:"+jobID, `{"event":"failed"}`)
}
