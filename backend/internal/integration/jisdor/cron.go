// Package jisdor — scheduled job for automatic BI JISDOR rate fetching.
//
// Phase 3 status: This cron handler is a placeholder.
// The Asynq task registration and real fetch logic are deferred to Phase 4.
//
// Expected Phase 4 design:
//   - Asynq task type: "integration:jisdor_sync"
//   - Schedule: every working day at 10:31 WIB (after BI publishes JISDOR at 10:30)
//   - On fetch success: call kurs.Service.CreateApproved for each Rate (auto-approved)
//   - Idempotent: skip if kurs already exists for (kode_mata_uang, tanggal_berlaku)
//   - On fetch failure: DLQ entry + alert notification
package jisdor

import (
	"context"
	"log/slog"
)

// CronHandler is the Asynq task handler for the JISDOR sync job.
// In Phase 3 it logs a tick and does nothing.
type CronHandler struct {
	fetcher Fetcher
	logger  *slog.Logger
}

// NewCronHandler creates a CronHandler.
func NewCronHandler(fetcher Fetcher, logger *slog.Logger) *CronHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CronHandler{fetcher: fetcher, logger: logger}
}

// Handle is the Asynq task handler function.
// In Phase 3 it logs a tick. In Phase 4, fetch + upsert logic goes here.
func (h *CronHandler) Handle(ctx context.Context) error {
	h.logger.InfoContext(ctx, "jisdor cron tick — fetcher not implemented yet (Phase 3 stub)")
	// Phase 4: parse payload date, call h.fetcher.Fetch, call kurs service.CreateApproved.
	return nil
}
