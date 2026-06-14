// Package penempatan — Asynq worker for Penempatan Deposito maturity check (P5-M1).
//
// MaturityCheckHandler is triggered daily at 09:00 WIB (02:00 UTC) by Asynq scheduler.
// Schedule: "0 2 * * *" (UTC).
//
// Per cron run: scan APPROVED_ACTIVE with tanggal_jatuh_tempo ≤ today,
// transition each to MATURED in its own tx (partial failure allowed),
// emit PenempatanMaturedEvent per row for downstream consumers (P5-M9).
package penempatan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// MaturityCheckPayload is the Asynq task payload for the daily maturity check cron.
type MaturityCheckPayload struct {
	AsOfDate string `json:"asOfDate"` // YYYY-MM-DD; empty = today UTC
	TenantID string `json:"tenantId"`
}

// MaturityCheckHandler handles the daily penempatan:maturity_check Asynq task.
type MaturityCheckHandler struct {
	svc    *Service
	logger *slog.Logger
}

// NewMaturityCheckHandler creates a new MaturityCheckHandler. Panics if svc is nil.
func NewMaturityCheckHandler(svc *Service, logger *slog.Logger) *MaturityCheckHandler {
	if svc == nil {
		panic("penempatan.NewMaturityCheckHandler: svc must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MaturityCheckHandler{svc: svc, logger: logger}
}

// ProcessTask implements asynq.Handler. Called by Asynq worker.
func (h *MaturityCheckHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload MaturityCheckPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("penempatan.MaturityCheckHandler: unmarshal payload: %w", err)
	}

	tenantID := payload.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}

	var asOfDate time.Time
	if payload.AsOfDate != "" {
		parsed, err := time.Parse("2006-01-02", payload.AsOfDate)
		if err != nil {
			return fmt.Errorf("penempatan.MaturityCheckHandler: invalid asOfDate %q: %w", payload.AsOfDate, err)
		}
		asOfDate = parsed
	} else {
		asOfDate = time.Now().UTC().Truncate(24 * time.Hour)
	}

	h.logger.InfoContext(ctx, "penempatan maturity check started",
		"as_of_date", asOfDate.Format("2006-01-02"),
		"tenant_id", tenantID,
	)

	maturedCount, err := h.svc.ProcessMaturity(ctx, asOfDate, tenantID)
	if err != nil {
		h.logger.ErrorContext(ctx, "penempatan maturity check failed",
			"error", err,
			"as_of_date", asOfDate.Format("2006-01-02"),
		)
		return fmt.Errorf("penempatan.MaturityCheckHandler: ProcessMaturity: %w", err)
	}

	h.logger.InfoContext(ctx, "penempatan maturity check completed",
		"matured_count", maturedCount,
		"as_of_date", asOfDate.Format("2006-01-02"),
		"tenant_id", tenantID,
	)

	return nil
}

// NewMaturityCheckTask creates a periodic Asynq task payload for the cron scheduler.
// Schedule "0 2 * * *" (daily 09:00 WIB = 02:00 UTC).
func NewMaturityCheckTask(tenantID string) *asynq.Task {
	payload := MaturityCheckPayload{
		TenantID: tenantID,
		// AsOfDate empty = today UTC
	}
	b, _ := json.Marshal(payload)
	return asynq.NewTask(MaturityCheckTaskType, b)
}
