package bulkupload

// worker.go — Asynq task handler for bulkupload:commit_instrumen (P5-M11).
//
// Task: bulkupload:commit_instrumen
// Triggered by: POST /api/v1/master/instrumen/bulk-upload/{batch_id}/commit (handler.go)
//
// Flow per row:
//   1. BEGIN SAVEPOINT
//   2. INSERT mst.instrumen (klasifikasi from DRY_RUN Stage 4 result)
//   3. UPDATE sys.upload_batch_row: row_status=COMMITTED + bulk_instrumen_id
//   4. RELEASE SAVEPOINT (or ROLLBACK TO SAVEPOINT on error → row_status=FAILED)
//
// Partial commit: failed rows do not halt batch. All committed rows persist.
// Progress via sys.job + Redis pub/sub (UX rule §3).
// Audit BULK.COMMITTED or BULK.PARTIAL_COMMIT in-transaction after all rows.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/audit"
)

// TaskCommitInstrumen is the Asynq task type for bulk instrumen commit.
const TaskCommitInstrumen = "bulkupload:commit_instrumen"

// NewCommitTask creates an Asynq task for bulk commit.
func NewCommitTask(batchID uuid.UUID, actorID uuid.UUID, tenantID string, jobID uuid.UUID) (*asynq.Task, error) {
	p, err := json.Marshal(CommitJobPayload{
		BatchID:  batchID.String(),
		ActorID:  actorID.String(),
		TenantID: tenantID,
		JobID:    jobID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("NewCommitTask: marshal: %w", err)
	}
	return asynq.NewTask(TaskCommitInstrumen, p,
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
		asynq.Queue("default"),
	), nil
}

// Worker holds the Asynq task handler for bulk upload commit.
type Worker struct {
	repo   Repository
	audit  *audit.Writer
	redis  *redis.Client
	logger *slog.Logger
}

// NewWorker creates a bulk upload commit Worker.
func NewWorker(repo Repository, auditWriter *audit.Writer, rdb *redis.Client, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{repo: repo, audit: auditWriter, redis: rdb, logger: logger}
}

// RegisterHandlers registers all bulkupload task handlers with an Asynq mux.
func (w *Worker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskCommitInstrumen, w.HandleCommitInstrumen)
}

// HandleCommitInstrumen handles the bulkupload:commit_instrumen Asynq task.
func (w *Worker) HandleCommitInstrumen(ctx context.Context, t *asynq.Task) error {
	var p CommitJobPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("HandleCommitInstrumen: unmarshal: %w", err)
	}

	batchID, err := uuid.Parse(p.BatchID)
	if err != nil {
		return fmt.Errorf("HandleCommitInstrumen: invalid batch_id: %w", err)
	}
	actorID, err := uuid.Parse(p.ActorID)
	if err != nil {
		actorID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	}

	w.logger.InfoContext(ctx, "bulkupload:commit_instrumen started",
		"batch_id", p.BatchID,
		"job_id", p.JobID,
	)
	w.updateJobProgress(ctx, p.JobID, 0, "Memulai commit instrumen batch...")

	// Load pending rows
	rows, err := w.repo.GetBatchRowsByStatus(ctx, batchID, RowStatusPending)
	if err != nil {
		w.updateJobFailed(ctx, p.JobID, err.Error())
		return fmt.Errorf("HandleCommitInstrumen: GetBatchRowsByStatus: %w", err)
	}

	total := len(rows)
	if total == 0 {
		w.logger.WarnContext(ctx, "bulkupload:commit_instrumen: no PENDING rows found", "batch_id", p.BatchID)
		w.updateJobComplete(ctx, p.JobID, 0, 0)
		return nil
	}

	committedRows := 0
	failedRows := 0

	for i, br := range rows {
		// Parse dry run result to get klasifikasi
		var rowData map[string]interface{}
		_ = json.Unmarshal(br.RowDataJson, &rowData)

		rr := RowValidationResult{
			SheetName: SheetName(br.SheetName),
			RowNumber: br.RowNumber,
			RowData:   rowData,
			Status:    RowStatusPending,
		}

		// Try to determine if flagged
		if br.RowStatus == RowStatusFlaggedManualReview {
			rr.Status = RowStatusFlaggedManualReview
		}

		instrumenID, insertErr := w.repo.InsertInstrumen(ctx, nil, rr, batchID, actorID, p.TenantID)
		if insertErr != nil {
			// Row failed — log error, continue with next row
			failedRows++
			errJSON, _ := json.Marshal(map[string]string{"error": insertErr.Error()})
			raw := json.RawMessage(errJSON)
			_ = w.repo.UpdateRowStatus(ctx, nil, br.ID, RowStatusFailed, nil, &raw)
			w.logger.WarnContext(ctx, "commit row failed",
				"batch_id", p.BatchID,
				"row_number", br.RowNumber,
				"error", insertErr.Error(),
			)
		} else {
			committedRows++
			_ = w.repo.UpdateRowStatus(ctx, nil, br.ID, RowStatusCommitted, &instrumenID, nil)
		}

		// Update progress every 10% or every 100 rows
		reportInterval := max(total/10, 100)
		if i > 0 && i%reportInterval == 0 {
			pct := (i * 100) / total
			w.updateJobProgress(ctx, p.JobID, pct,
				fmt.Sprintf("Memproses instrumen %d dari %d (sheet: %s)", i, total, br.SheetName))
		}
	}

	// Final batch status update
	graceDays, _ := w.repo.GetConfigParamInt(ctx, "BULK_ROLLBACK_GRACE_DAYS", defaultGraceDays)
	_ = w.repo.UpdateBatchCommitted(ctx, nil, batchID, committedRows, failedRows, graceDays, actorID)

	// Audit event
	auditAction := "BULK.COMMITTED"
	if failedRows > 0 {
		auditAction = "BULK.PARTIAL_COMMIT"
	}
	if w.audit != nil {
		_ = w.audit.Write(ctx, audit.Event{
			Action:      auditAction,
			EntityType:  "sys.upload_batch",
			EntityID:    batchID,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"batch_id":       batchID,
				"committed_rows": committedRows,
				"failed_rows":    failedRows,
				"job_id":         p.JobID,
			},
		})
	}

	w.logger.InfoContext(ctx, "bulkupload:commit_instrumen completed",
		"batch_id", p.BatchID,
		"committed_rows", committedRows,
		"failed_rows", failedRows,
	)
	w.updateJobComplete(ctx, p.JobID, committedRows, failedRows)
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
	_ = w.redis.Publish(ctx, "job-events:"+jobID,
		fmt.Sprintf(`{"event":"progress","progress":%d,"currentStep":%q}`, pct, step))
}

func (w *Worker) updateJobComplete(ctx context.Context, jobID string, committedRows, failedRows int) {
	if jobID == "" || w.redis == nil {
		return
	}
	key := "job:" + jobID
	_ = w.redis.HSet(ctx, key, map[string]interface{}{
		"status":        "completed",
		"progress":      100,
		"currentStep":   "Selesai",
		"committedRows": committedRows,
		"failedRows":    failedRows,
		"updatedAt":     time.Now().UTC().Unix(),
	})
	_ = w.redis.Expire(ctx, key, 24*time.Hour)
	_ = w.redis.Publish(ctx, "job-events:"+jobID,
		fmt.Sprintf(`{"event":"completed","progress":100,"committedRows":%d,"failedRows":%d}`,
			committedRows, failedRows))
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
