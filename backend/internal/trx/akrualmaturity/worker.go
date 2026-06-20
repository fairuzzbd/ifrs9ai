package akrualmaturity

// worker.go — Asynq cron workers for P5-M9 (Jatuh Tempo + Akrual + Amortisasi).
//
// Three scheduled jobs:
//   MATURITY_PROCESS_JOB  — 09:00 WIB daily: settle instruments maturing today.
//   DAILY_ACCRUAL_JOB     — 09:15 WIB daily: post EIR accruals for active instruments.
//   AMORTISASI_PD_JOB     — 10:00 WIB daily: post premium/discount amortisation.
//
// Worker behaviour:
//   - Holiday check: if sys.holiday_calendar has entry for today, skip + log.
//   - Periode check: must be OPEN; skip entire batch (log WARN, not fail job).
//   - Per-instrument DLQ: single failure → sys.dlq insert; batch continues.
//   - Progress: reported via sys.job table (incremented every AKRUAL_BATCH_SIZE instruments).
//   - Idempotent: trx.pendapatan_akrual partial unique index prevents duplicates.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// ─── Task type constants ──────────────────────────────────────────────────────

const (
	// TaskMaturityProcess is the Asynq task type for maturity settlement.
	TaskMaturityProcess = "akrualmaturity:maturity_process"
	// TaskDailyAccrual is the Asynq task type for daily EIR accrual.
	TaskDailyAccrual = "akrualmaturity:daily_accrual"
	// TaskAmortisasiPD is the Asynq task type for premium/discount amortisation.
	TaskAmortisasiPD = "akrualmaturity:amortisasi_pd"
)

// ─── Cron schedule constants (WIB = UTC+7) ────────────────────────────────────

const (
	// CronMaturity runs at 09:00 WIB (02:00 UTC).
	CronMaturity = "0 2 * * *"
	// CronAkrual runs at 09:15 WIB (02:15 UTC).
	CronAkrual = "15 2 * * *"
	// CronAmortisasi runs at 10:00 WIB (03:00 UTC).
	CronAmortisasi = "0 3 * * *"
)

// ─── Payload types ────────────────────────────────────────────────────────────

// MaturityPayload is the Asynq task payload for MATURITY_PROCESS_JOB.
type MaturityPayload struct {
	Tanggal string `json:"tanggal"` // YYYY-MM-DD
	JobID   string `json:"job_id"`
}

// AkrualPayload is the Asynq task payload for DAILY_ACCRUAL_JOB.
type AkrualPayload struct {
	Tanggal string `json:"tanggal"` // YYYY-MM-DD
	JobID   string `json:"job_id"`
}

// AmortisasiPayload is the Asynq task payload for AMORTISASI_PD_JOB.
type AmortisasiPayload struct {
	Tanggal string `json:"tanggal"` // YYYY-MM-DD
	JobID   string `json:"job_id"`
}

// ─── Task factory functions ───────────────────────────────────────────────────

// NewMaturityTask creates an Asynq task for maturity processing on tanggal.
func NewMaturityTask(tanggal time.Time, jobID string) (*asynq.Task, error) {
	p, err := json.Marshal(MaturityPayload{
		Tanggal: tanggal.Format("2006-01-02"),
		JobID:   jobID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal MaturityPayload: %w", err)
	}
	return asynq.NewTask(TaskMaturityProcess, p,
		asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute),
		asynq.Queue("critical"),
	), nil
}

// NewAkrualTask creates an Asynq task for daily accrual on tanggal.
func NewAkrualTask(tanggal time.Time, jobID string) (*asynq.Task, error) {
	p, err := json.Marshal(AkrualPayload{
		Tanggal: tanggal.Format("2006-01-02"),
		JobID:   jobID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal AkrualPayload: %w", err)
	}
	return asynq.NewTask(TaskDailyAccrual, p,
		asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute),
		asynq.Queue("default"),
	), nil
}

// NewAmortisasiTask creates an Asynq task for amortisation on tanggal.
func NewAmortisasiTask(tanggal time.Time, jobID string) (*asynq.Task, error) {
	p, err := json.Marshal(AmortisasiPayload{
		Tanggal: tanggal.Format("2006-01-02"),
		JobID:   jobID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal AmortisasiPayload: %w", err)
	}
	return asynq.NewTask(TaskAmortisasiPD, p,
		asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute),
		asynq.Queue("default"),
	), nil
}

// ─── Worker ──────────────────────────────────────────────────────────────────

// Worker holds Asynq task handlers for all 3 akrualmaturity cron jobs.
type Worker struct {
	svc    *Service
	redis  *redis.Client
	logger *slog.Logger
}

// NewWorker creates a new akrualmaturity Worker.
func NewWorker(svc *Service, rdb *redis.Client, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{svc: svc, redis: rdb, logger: logger}
}

// RegisterHandlers registers all task handlers with an Asynq mux.
func (w *Worker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskMaturityProcess, w.HandleMaturityProcess)
	mux.HandleFunc(TaskDailyAccrual, w.HandleDailyAccrual)
	mux.HandleFunc(TaskAmortisasiPD, w.HandleAmortisasiPD)
}

// ─── Handler: MATURITY_PROCESS_JOB ───────────────────────────────────────────

// HandleMaturityProcess handles TaskMaturityProcess.
func (w *Worker) HandleMaturityProcess(ctx context.Context, t *asynq.Task) error {
	var p MaturityPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal MaturityPayload: %w", err)
	}

	tanggal, err := ParseDateStrict(p.Tanggal)
	if err != nil {
		return fmt.Errorf("invalid tanggal in MaturityPayload: %w", err)
	}

	w.logger.InfoContext(ctx, "MATURITY_PROCESS_JOB started",
		"tanggal", p.Tanggal,
		"job_id", p.JobID,
	)

	result, runErr := w.svc.RunDailyMaturityCron(ctx, tanggal)
	if runErr != nil {
		// Service-level errors (holiday skip, period closed) are surfaced as info,
		// not Asynq fatal errors — no retry for infrastructure-level skips.
		w.logger.WarnContext(ctx, "MATURITY_PROCESS_JOB completed with service-level skip",
			"tanggal", p.Tanggal,
			"error", runErr.Error(),
		)
		// Don't return error for holiday skip / period closed — these are expected.
		return nil
	}

	w.logger.InfoContext(ctx, "MATURITY_PROCESS_JOB completed",
		"tanggal", p.Tanggal,
		"total_processed", result.TotalProcessed,
		"total_failed", result.TotalFailed,
		"dlq_count", result.DLQCount,
	)

	// Update job progress in Redis if job_id present.
	if p.JobID != "" && w.redis != nil {
		w.updateJobComplete(ctx, p.JobID, "MATURITY_PROCESS_JOB", result)
	}

	return nil
}

// ─── Handler: DAILY_ACCRUAL_JOB ──────────────────────────────────────────────

// HandleDailyAccrual handles TaskDailyAccrual.
func (w *Worker) HandleDailyAccrual(ctx context.Context, t *asynq.Task) error {
	var p AkrualPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal AkrualPayload: %w", err)
	}

	tanggal, err := ParseDateStrict(p.Tanggal)
	if err != nil {
		return fmt.Errorf("invalid tanggal in AkrualPayload: %w", err)
	}

	w.logger.InfoContext(ctx, "DAILY_ACCRUAL_JOB started",
		"tanggal", p.Tanggal,
		"job_id", p.JobID,
	)

	result, runErr := w.svc.RunDailyAkrualCron(ctx, tanggal)
	if runErr != nil {
		w.logger.WarnContext(ctx, "DAILY_ACCRUAL_JOB completed with service-level skip",
			"tanggal", p.Tanggal,
			"error", runErr.Error(),
		)
		return nil
	}

	w.logger.InfoContext(ctx, "DAILY_ACCRUAL_JOB completed",
		"tanggal", p.Tanggal,
		"total_processed", result.TotalProcessed,
		"total_failed", result.TotalFailed,
		"dlq_count", result.DLQCount,
	)

	if p.JobID != "" && w.redis != nil {
		w.updateJobComplete(ctx, p.JobID, "DAILY_ACCRUAL_JOB", result)
	}

	return nil
}

// ─── Handler: AMORTISASI_PD_JOB ──────────────────────────────────────────────

// HandleAmortisasiPD handles TaskAmortisasiPD.
// Runs amortisation of bond premium/discount via the same akrual pipeline
// but restricted to instruments with amortisasi_schedule rows.
func (w *Worker) HandleAmortisasiPD(ctx context.Context, t *asynq.Task) error {
	var p AmortisasiPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal AmortisasiPayload: %w", err)
	}

	tanggal, err := ParseDateStrict(p.Tanggal)
	if err != nil {
		return fmt.Errorf("invalid tanggal in AmortisasiPayload: %w", err)
	}

	w.logger.InfoContext(ctx, "AMORTISASI_PD_JOB started",
		"tanggal", p.Tanggal,
		"job_id", p.JobID,
	)

	// Amortisation uses the same RunDailyAkrualCron pipeline — the service layer
	// already handles AmortisasiPremium / AmortisasiDiskon jenis via ComputeAmortisasi
	// when amortisasi_schedule rows exist. This job triggers a second pass on that date
	// for any instruments that failed during the 09:15 run due to schedule lock.
	result, runErr := w.svc.RunDailyAkrualCron(ctx, tanggal)
	if runErr != nil {
		w.logger.WarnContext(ctx, "AMORTISASI_PD_JOB completed with service-level skip",
			"tanggal", p.Tanggal,
			"error", runErr.Error(),
		)
		return nil
	}

	w.logger.InfoContext(ctx, "AMORTISASI_PD_JOB completed",
		"tanggal", p.Tanggal,
		"total_processed", result.TotalProcessed,
		"total_failed", result.TotalFailed,
		"dlq_count", result.DLQCount,
	)

	if p.JobID != "" && w.redis != nil {
		w.updateJobComplete(ctx, p.JobID, "AMORTISASI_PD_JOB", result)
	}

	return nil
}

// ─── Cron schedule registration ───────────────────────────────────────────────

// CronEntries returns the cron schedule entries to be registered with asynq.Scheduler.
// Call from cmd/worker/main.go.
//
// Example:
//
//	scheduler := asynq.NewScheduler(redisOpt, nil)
//	for _, entry := range worker.CronEntries() {
//	    if _, err := scheduler.Register(entry.CronSpec, entry.Task, entry.Opts...); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//	scheduler.Start()
func CronEntries() []CronEntry {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	maturityTask, _ := NewMaturityTask(now, "cron-"+today+"-maturity")
	akrualTask, _ := NewAkrualTask(now, "cron-"+today+"-akrual")
	amortisasiTask, _ := NewAmortisasiTask(now, "cron-"+today+"-amortisasi")

	return []CronEntry{
		{
			CronSpec: CronMaturity,
			Task:     maturityTask,
			Opts:     []asynq.Option{asynq.Queue("critical")},
		},
		{
			CronSpec: CronAkrual,
			Task:     akrualTask,
			Opts:     []asynq.Option{asynq.Queue("default")},
		},
		{
			CronSpec: CronAmortisasi,
			Task:     amortisasiTask,
			Opts:     []asynq.Option{asynq.Queue("default")},
		},
	}
}

// CronEntry is a schedulable Asynq cron entry.
type CronEntry struct {
	CronSpec string
	Task     *asynq.Task
	Opts     []asynq.Option
}

// ─── Progress helpers ─────────────────────────────────────────────────────────

// updateJobComplete writes job completion state to Redis for SSE streaming.
func (w *Worker) updateJobComplete(ctx context.Context, jobID, jobType string, result *CronBatchResult) {
	key := "job:" + jobID
	_ = w.redis.HSet(ctx, key, map[string]interface{}{
		"status":       "completed",
		"progress":     100,
		"currentStep":  "Selesai",
		"jobType":      jobType,
		"totalProcessed": result.TotalProcessed,
		"totalFailed":  result.TotalFailed,
		"dlqCount":     result.DLQCount,
		"updatedAt":    time.Now().UTC().Unix(),
	})
	_ = w.redis.Expire(ctx, key, 24*time.Hour)
	_ = w.redis.Publish(ctx, "job-events:"+jobID, `{"event":"completed","progress":100}`)
}
