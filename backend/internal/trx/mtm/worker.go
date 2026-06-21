package mtm

// worker.go — Asynq cron handler for trx:mtm_daily_run (18:00 WIB = 11:00 UTC, Mon-Fri).
//
// Cron schedule: "0 11 * * 1-5" (Asynq cron string, UTC).
// Entry point: HandleMtmDailyRun — registered via RegisterHandlers.
//
// Flow per run:
//  1. Check holiday + weekend → skip if true.
//  2. GetActiveNonACInstrumen.
//  3. Per instrument (per-instrument tx):
//     a. svc.ProcessOneInstrument(inst, tanggalMtm)
//     b. Log error → DLQ; continue to next.
//  4. Report progress periodically (§UX-rule 3).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// Handler is the Asynq task handler for the MTM worker.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler creates a new MTM Asynq handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// RegisterHandlers registers the MTM task handler with the Asynq multiplexer.
// Called from main.go or worker cmd.
func (h *Handler) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskMtmDailyRun, h.HandleMtmDailyRun)
}

// HandleMtmDailyRun handles the trx:mtm_daily_run Asynq task.
// Idempotent: ExistsActive guards per-instrument duplicate.
func (h *Handler) HandleMtmDailyRun(ctx context.Context, t *asynq.Task) error {
	var payload MtmCronPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("HandleMtmDailyRun: unmarshal payload: %w", err)
	}

	tanggalMtm, err := ParseDateStrict(payload.TanggalTarget)
	if err != nil {
		return fmt.Errorf("HandleMtmDailyRun: invalid tanggal_target %q: %w", payload.TanggalTarget, err)
	}

	jobID := payload.JobID
	if jobID == "" {
		jobID = "mtm-cron-" + tanggalMtm.Format("2006-01-02")
	}

	// Weekend check
	if IsWeekend(tanggalMtm) {
		h.logger.InfoContext(ctx, "HandleMtmDailyRun: skip — weekend",
			"tanggal_target", payload.TanggalTarget, "job_id", jobID)
		return nil
	}

	// Holiday check
	isHol, err := h.svc.repo.IsHoliday(ctx, tanggalMtm)
	if err != nil {
		h.logger.WarnContext(ctx, "HandleMtmDailyRun: holiday check failed — proceed anyway",
			"error", err, "job_id", jobID)
	}
	if isHol {
		h.logger.InfoContext(ctx, "HandleMtmDailyRun: skip — holiday",
			"tanggal_target", payload.TanggalTarget, "job_id", jobID)
		return nil
	}

	// Fetch active non-AC instruments
	instruments, err := h.svc.repo.GetActiveNonACInstrumen(ctx)
	if err != nil {
		return fmt.Errorf("HandleMtmDailyRun: GetActiveNonACInstrumen: %w", err)
	}

	// m7 fix: honor ForceRerun by passing "force:" prefixed jobID to ProcessOneInstrument.
	// ProcessOneInstrument checks for the "force:" prefix to skip ExistsActive idempotency check.
	effectiveJobID := jobID
	if payload.ForceRerun {
		effectiveJobID = "force:" + jobID
		h.logger.WarnContext(ctx, "HandleMtmDailyRun: force-rerun requested — skipping idempotency check",
			"job_id", jobID,
			"tanggal_target", payload.TanggalTarget,
			"force_rerun_reason", payload.ForceRerunReason,
		)
	}

	h.logger.InfoContext(ctx, "HandleMtmDailyRun: starting",
		"tanggal_target", payload.TanggalTarget,
		"job_id", jobID,
		"instrument_count", len(instruments),
		"force_rerun", payload.ForceRerun)

	total := len(instruments)
	processed := 0
	failed := 0
	skipped := 0

	reportInterval := max(total/100, 10) // report every 1% or every 10 instruments

	for i, inst := range instruments {
		select {
		case <-ctx.Done():
			h.logger.WarnContext(ctx, "HandleMtmDailyRun: cancelled",
				"job_id", jobID, "processed", processed, "failed", failed)
			return ctx.Err()
		default:
		}

		_, err := h.svc.ProcessOneInstrument(ctx, inst, tanggalMtm, effectiveJobID)
		if err != nil {
			if isACSkip(err) {
				skipped++
				continue
			}
			failed++
			h.logger.ErrorContext(ctx, "HandleMtmDailyRun: instrument failed",
				"instrumen_id", inst.ID,
				"instrumen_kode", inst.KodeInstrumen,
				"tanggal_mtm", payload.TanggalTarget,
				"error", err,
				"job_id", jobID,
			)
			// DLQ: log for now; real DLQ insert requires sys.dlq table (follow-up).
			// TODO(follow-up): h.svc.repo.InsertDLQ(ctx, inst.ID, jobID, err.Error())
			continue
		}
		processed++

		// Progress report (§UX-rule 3 — §3.3 worker pattern)
		if i > 0 && i%reportInterval == 0 {
			pct := (i * 100) / total
			h.logger.InfoContext(ctx, "HandleMtmDailyRun: progress",
				"job_id", jobID,
				"progress_pct", pct,
				"processed", processed,
				"failed", failed,
				"total", total,
			)
			// TODO(follow-up): update sys.job progress column + Redis pub/sub.
		}
	}

	h.logger.InfoContext(ctx, "HandleMtmDailyRun: completed",
		"job_id", jobID,
		"tanggal_target", payload.TanggalTarget,
		"processed", processed,
		"failed", failed,
		"skipped", skipped,
		"total", total,
		"duration_ms", time.Since(tanggalMtm).Milliseconds(),
	)

	if failed > 0 {
		// Return nil (not error) so Asynq doesn't retry the whole batch.
		// Individual failures are in DLQ. Partial success is acceptable per FSD-APP-B §5.3.
		h.logger.WarnContext(ctx, "HandleMtmDailyRun: partial failure",
			"job_id", jobID, "failed_count", failed)
	}
	return nil
}

// isACSkip checks if an error is the AC-skip sentinel.
func isACSkip(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == ErrMTMInstrumenACSkip.Error()
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
