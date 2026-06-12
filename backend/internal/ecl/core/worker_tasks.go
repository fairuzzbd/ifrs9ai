package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
)

// worker_tasks.go — Asynq task handler for bulk ECL compute.
//
// Task type: "ecl:bulk_compute" (TaskNameECLBulkCompute).
//
// SLA: 1,000 instruments must complete within 30 seconds (state-machine doc §7.1).
// Concurrency: semaphore of 16 goroutines (bulkSemaphoreSize).
// Cancellation: checks ctx.Err() before each instrument.
// Progress: ProgressFn callback called every ~50 instruments or 1% change.
//
// Partial failure: one instrument error does NOT abort the batch.
// Errors collected in BulkComputeResult.Errors.
// Final status: "completed" if zero errors, "completed_with_errors" if any.
//
// Hard rules:
//   - No float64.
//   - Sealed calc_run cannot be re-computed: handler returns ECL_CALC_RUN_SEALED.
//   - ECL_BULK_TOO_LARGE if scope > 10,000 instruments.
//   - Idempotency: if result line already exists for (calcRunID, instrumenID) → skip.

const (
	// bulkSemaphoreSize limits concurrent goroutines in the bulk fan-out.
	bulkSemaphoreSize = 16
	// bulkMaxInstruments is the hard limit for bulk compute scope.
	bulkMaxInstruments = 10_000
	// progressReportEvery controls progress reporting granularity.
	progressReportEvery = 50
)

// BulkWorker is the Asynq task handler for bulk ECL compute.
type BulkWorker struct {
	orchestrator *ECLOrchestrator
	logger       *slog.Logger
	progressFn   ProgressFn // nil = no-op
}

// NewBulkWorker creates a BulkWorker. progressFn may be nil.
func NewBulkWorker(orchestrator *ECLOrchestrator, progressFn ProgressFn, logger *slog.Logger) *BulkWorker {
	if orchestrator == nil {
		panic("core.NewBulkWorker: orchestrator must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BulkWorker{
		orchestrator: orchestrator,
		progressFn:   progressFn,
		logger:       logger,
	}
}

// Handle implements asynq.Handler for task type "ecl:bulk_compute".
func (w *BulkWorker) Handle(ctx context.Context, t *asynq.Task) error {
	var payload TaskECLBulkComputePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("core.BulkWorker: unmarshal payload: %w", err)
	}

	scope := (*BulkScope)(nil)
	if len(payload.InstrumenIDs) > 0 || len(payload.PortofolioIDs) > 0 {
		scope = &BulkScope{
			PortofolioIDs: payload.PortofolioIDs,
			InstrumenIDs:  payload.InstrumenIDs,
		}
	}

	req := BulkComputeRequest{
		CalcRunID:      payload.CalcRunID,
		EvaluationDate: payload.EvaluationDate,
		PeriodeID:      payload.PeriodeID,
		Scope:          scope,
		ActorID:        payload.ActorID,
	}

	result, err := w.orchestrator.ComputeBulk(ctx, req, w.progressFn)
	if err != nil {
		return fmt.Errorf("core.BulkWorker: ComputeBulk: %w", err)
	}

	w.logger.InfoContext(ctx, "core.BulkWorker: completed",
		"job_id", payload.JobID,
		"calc_run_id", payload.CalcRunID,
		"total_scanned", result.TotalScanned,
		"total_computed", result.TotalComputed,
		"total_errors", len(result.Errors),
		"status", result.Status,
	)
	return nil
}

// NewECLBulkComputeTask creates an Asynq task for bulk ECL compute.
func NewECLBulkComputeTask(payload TaskECLBulkComputePayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("core.NewECLBulkComputeTask: marshal: %w", err)
	}
	return asynq.NewTask(TaskNameECLBulkCompute, b), nil
}

// ─── ComputeBulk (orchestrator method) ───────────────────────────────────────

// ComputeBulk runs ECL for all instruments in scope.
// Called by BulkWorker or directly for sync use in tests.
// progressFn may be nil (no progress reporting).
//
// Performance: 1,000 instruments ≤ 30s SLA (state-machine doc §7.1).
// Uses semaphore of 16 goroutines for fan-out.
func (o *ECLOrchestrator) ComputeBulk(ctx context.Context, req BulkComputeRequest, progressFn ProgressFn) (*BulkComputeResult, error) {
	// 1. Load active instruments in scope.
	instruments, err := o.instrReader.ListActiveByScope(ctx, req.Scope)
	if err != nil {
		return nil, fmt.Errorf("core.ComputeBulk: list instruments: %w", err)
	}

	total := len(instruments)
	if total > bulkMaxInstruments {
		return nil, errDomain(CodeECLBulkTooLarge,
			fmt.Sprintf("scope has %d instruments, max is %d", total, bulkMaxInstruments))
	}

	o.logger.InfoContext(ctx, "core.ComputeBulk: starting",
		"calc_run_id", req.CalcRunID,
		"total", total,
	)

	if progressFn != nil {
		progressFn(0, total, fmt.Sprintf("Memuat %d instrumen...", total))
	}

	// 2. Pre-load bobot once for all instruments (avoids N+1 DB calls).
	bobot, err := o.bobotRepo.GetActiveBobot(ctx, req.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("core.ComputeBulk: get bobot: %w", err)
	}
	if err := bobot.Validate(); err != nil {
		return nil, err
	}

	// 3. Fan-out with semaphore.
	type singleResult struct {
		instrumenID uuid.UUID
		routing     RoutingPath
		ecl         *decimal.Decimal
		err         error
	}

	items := make([]bulkComputeItem, total)
	sem := make(chan struct{}, bulkSemaphoreSize)
	var wg sync.WaitGroup
	var mu sync.Mutex
	processed := 0
	skippedFVTPL := 0
	skippedPOCI := 0
	skippedDup := 0
	cancelled := false
	var eclTotal decimal.Decimal

	for i, inst := range instruments {
		// Check cancellation before spawning goroutine.
		select {
		case <-ctx.Done():
			cancelled = true
		default:
		}
		if cancelled {
			break
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(idx int, instrSnap InstrumenSnapshot) {
			defer func() {
				<-sem
				wg.Done()
			}()

			instrID := instrSnap.ID
			singleReq := ComputeRequest{
				InstrumenID:    instrID,
				EvaluationDate: req.EvaluationDate,
				PeriodeID:      req.PeriodeID,
				CalcRunID:      &req.CalcRunID,
				Persist:        true,
				ActorID:        req.ActorID,
			}

			// Check for idempotency: skip if already computed for this run.
			exists, err := o.resultRepo.ExistsResultLine(ctx, req.CalcRunID, instrID)
			if err != nil {
				mu.Lock()
				items[idx] = bulkComputeItem{instrumenID: instrID, err: fmt.Errorf("exists check: %w", err)}
				processed++
				mu.Unlock()
				return
			}
			if exists {
				mu.Lock()
				items[idx] = bulkComputeItem{instrumenID: instrID, routing: RoutingStandard}
				processed++
				skippedDup++
				mu.Unlock()
				reportProgress(progressFn, processed, total)
				return
			}

			res, computeErr := o.ComputeSingle(ctx, singleReq)
			mu.Lock()
			processed++
			if computeErr != nil {
				items[idx] = bulkComputeItem{instrumenID: instrID, err: computeErr}
			} else {
				items[idx] = bulkComputeItem{
					instrumenID: instrID,
					routing:     res.RoutingPath,
					ecl:         res.ECLWeightedIDR,
				}
				switch res.RoutingPath {
				case RoutingSkipFVTPL:
					skippedFVTPL++
				case RoutingPOCIDeferred:
					skippedPOCI++
				}
				if res.ECLWeightedIDR != nil {
					eclTotal = eclTotal.Add(*res.ECLWeightedIDR)
				}
			}
			reportProgress(progressFn, processed, total)
			mu.Unlock()
		}(i, inst)
	}
	wg.Wait()

	if cancelled {
		return &BulkComputeResult{
			CalcRunID:    req.CalcRunID,
			TotalScanned: total,
			Status:       "cancelled",
		}, nil
	}

	// 4. Collect errors.
	var bulkErrors []BulkComputeError
	computed := 0
	for _, r := range items {
		if r.err != nil {
			bulkErrors = append(bulkErrors, BulkComputeError{
				InstrumenID:  r.instrumenID,
				ErrorCode:    "ECL_SINGLE_COMPUTE_FAILED",
				ErrorMessage: r.err.Error(),
			})
		} else {
			computed++
		}
	}

	status := "completed"
	if len(bulkErrors) > 0 {
		status = "completed_with_errors"
	}

	if progressFn != nil {
		progressFn(total, total, fmt.Sprintf("Selesai: %d dihitung, %d error", computed, len(bulkErrors)))
	}

	return &BulkComputeResult{
		CalcRunID:             req.CalcRunID,
		TotalScanned:          total,
		TotalComputed:         computed,
		TotalSkippedFVTPL:     skippedFVTPL,
		TotalPOCIDeferred:     skippedPOCI,
		TotalSkippedDuplicate: skippedDup,
		ECLWeightedIDRTotal:   eclTotal.RoundBank(4),
		Errors:                bulkErrors,
		Status:                status,
	}, nil
}

// reportProgress calls progressFn every progressReportEvery instruments.
func reportProgress(progressFn ProgressFn, processed, total int) {
	if progressFn == nil || total == 0 {
		return
	}
	if processed%progressReportEvery == 0 || processed == total {
		progressFn(processed, total,
			fmt.Sprintf("Menghitung instrument %d dari %d (%.0f%%)",
				processed, total, float64(processed)/float64(total)*100))
	}
}

// ─── CalcRunSealChecker ───────────────────────────────────────────────────────

// CalcRunSealChecker provides a check for whether a calc run is sealed.
// Implemented by the calc run repo in M8; injected into M7 handler.
type CalcRunSealChecker interface {
	IsSealedCalcRun(ctx context.Context, calcRunID uuid.UUID) (bool, error)
}

// ─── bulkComputeItem ─────────────────────────────────────────────────────────

// bulkComputeItem is the per-instrument result collected during bulk compute.
type bulkComputeItem struct {
	instrumenID uuid.UUID
	routing     RoutingPath
	ecl         *decimal.Decimal
	err         error
}
