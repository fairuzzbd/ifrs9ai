// Package eir — BulkService implements batch EIR re-computation for all active AC/FVOCI instruments.
//
// Story: APP-C-EIR-005 — Bulk Re-compute (report-only, no DB writes to schedule/eir_awal).
// Scope: SLA ≤ 5s per 1000 instruments. Streaming per instrument ≤ 10KB in memory.
// Output: drift report (instruments where |EIR_old - EIR_new| > threshold), missing schedules,
// errors. Persisted to sys.job.result_jsonb; no schedule/instrumen rows changed.
//
// Asynq: submitted via POST /api/v1/eir/bulk-recompute → 202 + jobId.
// Progress: Redis pub/sub + sys.job table (DEC-007).
// Cancellation: checks ctx.Done() per instrument.
package eir

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BulkService handles bulk EIR re-computation (Story 5: APP-C-EIR-005).
type BulkService struct {
	db         *sql.DB
	instrRepo  InstrumenEIRRepoIface
	schedRepo  ScheduleRepoIface
	solver     *Solver
	jobRepo    JobRepoIface
	progressFn ProgressFn // injected progress reporter
	logger     *slog.Logger
}

// ProgressFn updates job progress (Redis pub/sub, sys.job percent).
// pct in [0,100], step is human-readable description.
type ProgressFn func(ctx context.Context, jobID string, pct int, step string)

// noopProgress is used when progressFn is nil.
func noopProgress(_ context.Context, _ string, _ int, _ string) {}

// JobRepoIface persists sys.job status + result.
type JobRepoIface interface {
	UpdateProgress(ctx context.Context, jobID string, pct int, step string) error
	Complete(ctx context.Context, jobID string, resultJSON []byte) error
	Fail(ctx context.Context, jobID string, errMsg string) error
}

// NewBulkService creates a BulkService.
func NewBulkService(
	db *sql.DB,
	instrRepo InstrumenEIRRepoIface,
	schedRepo ScheduleRepoIface,
	jobRepo JobRepoIface,
	progressFn ProgressFn,
	logger *slog.Logger,
) *BulkService {
	pf := progressFn
	if pf == nil {
		pf = noopProgress
	}
	return &BulkService{
		db:         db,
		instrRepo:  instrRepo,
		schedRepo:  schedRepo,
		solver:     NewSolver(),
		jobRepo:    jobRepo,
		progressFn: pf,
		logger:     logger,
	}
}

// driftThreshold: EIR difference > 1 bp (0.0001) flagged as drift.
var driftThreshold = decimal.NewFromFloat(0.0001)

// Recompute runs bulk EIR recompute for all active AC/FVOCI instruments in scope.
// This is REPORT-ONLY — no schedule rows are written, no eir_awal changed.
// Implements APP-C-EIR-005 AC.
//
// Algorithm per instrument:
//  1. Load active schedule (to get existing cashflow projection)
//  2. Re-run solver with same cashflows
//  3. Compare new EIR vs stored eir_awal → if diff > driftThreshold → DriftEntry
//  4. If no active schedule → MissingScheduleEntry
//  5. If solver error → BulkErrorEntry
//
// Progress reported every 1% or every 100 instruments.
// Cancellation: checks ctx.Done() every instrument.
// SLA target: ≤ 5s / 1000 instruments (DEC-013 + Story 5 NFR).
func (s *BulkService) Recompute(ctx context.Context, scope BulkScope, jobID string, actorID uuid.UUID) (BulkRecomputeResult, error) {
	startTime := time.Now()

	result := BulkRecomputeResult{
		JobID:   jobID,
		Scope:   scope,
		RunAt:   startTime,
		Drifts:  []DriftEntry{},
		Missing: []MissingScheduleEntry{},
		Errors:  []BulkErrorEntry{},
	}

	s.progressFn(ctx, jobID, 0, "Membaca daftar instrumen aktif...")

	// Stream instruments (≤10KB per instrument in memory)
	instrCh, err := s.instrRepo.ListActiveForBulk(ctx, scope)
	if err != nil {
		return result, fmt.Errorf("eir.Recompute: list instruments: %w", err)
	}

	// Collect all instruments (streaming chan).
	// Pre-alloc at 64 to avoid repeated reallocs; grows as needed.
	instruments := make([]InstrumenForEIR, 0, 64)
	for inst := range instrCh {
		instruments = append(instruments, inst)
	}

	total := len(instruments)
	result.TotalInstruments = total

	if total == 0 {
		s.progressFn(ctx, jobID, 100, "Tidak ada instrumen aktif ditemukan")
		result.ElapsedMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	s.progressFn(ctx, jobID, 2, fmt.Sprintf("Memproses %d instrumen...", total))

	for i := range instruments {
		// Cancellation check every instrument
		select {
		case <-ctx.Done():
			result.Canceled = true
			result.ElapsedMs = time.Since(startTime).Milliseconds()
			return result, ctx.Err()
		default:
		}

		// Progress every 1% or every 100 instruments
		reportInterval := total / 100
		if reportInterval < 100 {
			reportInterval = 100
		}
		if i%reportInterval == 0 || i == total-1 {
			pct := 2 + (i * 95 / total) // leave 5% for finalize
			s.progressFn(ctx, jobID, pct, fmt.Sprintf("Instrument %d dari %d: %s", i+1, total, instruments[i].KodeInstrumen))
		}

		// Process single instrument (report-only)
		drift, missing, bulkErr := s.processInstrument(ctx, &instruments[i])
		if bulkErr != nil {
			result.Errors = append(result.Errors, *bulkErr)
			continue
		}
		if missing != nil {
			result.Missing = append(result.Missing, *missing)
			continue
		}
		if drift != nil {
			result.Drifts = append(result.Drifts, *drift)
		}
		result.ProcessedOK++
	}

	result.ElapsedMs = time.Since(startTime).Milliseconds()
	result.DriftCount = len(result.Drifts)
	result.MissingCount = len(result.Missing)
	result.ErrorCount = len(result.Errors)

	// Persist result to sys.job if jobRepo provided
	if s.jobRepo != nil {
		resultJSON, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			if err := s.jobRepo.Complete(ctx, jobID, resultJSON); err != nil && s.logger != nil {
				s.logger.WarnContext(ctx, "eir.Recompute: persist job result failed", "error", err)
			}
		}
	}

	s.progressFn(ctx, jobID, 100, fmt.Sprintf("Selesai: %d OK, %d drift, %d missing, %d error",
		result.ProcessedOK, result.DriftCount, result.MissingCount, result.ErrorCount))

	if s.logger != nil {
		s.logger.InfoContext(ctx, "eir.Recompute complete",
			"total", total,
			"processed_ok", result.ProcessedOK,
			"drifts", result.DriftCount,
			"missing", result.MissingCount,
			"errors", result.ErrorCount,
			"elapsed_ms", result.ElapsedMs,
		)
	}

	return result, nil
}

// processInstrument runs the solver for one instrument and classifies the outcome.
// Takes a pointer to avoid copying the 336B InstrumenForEIR struct.
// Returns at most one of: *DriftEntry, *MissingScheduleEntry, *BulkErrorEntry.
// If EIR matches within driftThreshold → returns (nil, nil, nil).
func (s *BulkService) processInstrument(ctx context.Context, inst *InstrumenForEIR) (*DriftEntry, *MissingScheduleEntry, *BulkErrorEntry) {
	// 1. EIR must exist
	if inst.EIRAwal == nil {
		return nil, &MissingScheduleEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			Reason:        "eir_awal IS NULL",
		}, nil
	}

	// 2. Get active schedule rows for cashflow reconstruction
	scheduleRows, err := s.schedRepo.GetActiveByPeriode(ctx, inst.ID, 0) // 0 = all
	if err != nil {
		return nil, nil, &BulkErrorEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			ErrorCode:     CodeEIRScheduleNotFound,
			ErrorMessage:  fmt.Sprintf("load schedule: %v", err),
		}
	}

	// 3. No schedule rows → missing
	if len(scheduleRows) == 0 {
		return nil, &MissingScheduleEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			Reason:        "No active schedule rows",
		}, nil
	}

	// 4. Reconstruct cashflow projection from schedule rows
	// CF[0] = -(opening_carrying of row 1)
	// CF[i] = cash_inflow of row i (+ pelunasan_pokok for last row)
	cfs := reconstructCFFromSchedule(scheduleRows)
	if len(cfs) < 2 {
		return nil, nil, &BulkErrorEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			ErrorCode:     CodeEIRCashflowInvalid,
			ErrorMessage:  "Cannot reconstruct cashflows from schedule",
		}
	}

	// 5. Re-run solver (report-only)
	newEIR, _, solveErr := s.solver.Solve(cfs, inst.EIRAwal)
	if solveErr != nil {
		return nil, nil, &BulkErrorEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			ErrorCode:     CodeEIRNonConvergent,
			ErrorMessage:  fmt.Sprintf("solver: %v", solveErr),
		}
	}

	// 6. Compare with stored eir_awal
	diff := newEIR.Sub(*inst.EIRAwal).Abs()
	if diff.GreaterThan(driftThreshold) {
		return &DriftEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			EIRAwal:       *inst.EIRAwal,
			EIRRecomputed: newEIR,
			AbsDiff:       diff,
			BasisPoints:   diff.Mul(decimal.NewFromInt(10000)).RoundBank(2),
		}, nil, nil
	}

	return nil, nil, nil
}

// reconstructCFFromSchedule builds a []CashflowItem from schedule rows for solver re-use.
// CF[0] = -(opening_carrying of first row) — reconstructed initial outflow.
// CF[i] = cash_inflow_i for all subsequent rows.
func reconstructCFFromSchedule(rows []ScheduleRow) []CashflowItem {
	if len(rows) == 0 {
		return nil
	}
	cfs := make([]CashflowItem, len(rows)+1)
	// Synthetic CF[0]: date is one period before first row, amount = -opening_carrying
	firstRow := rows[0]
	cfs[0] = CashflowItem{
		// Approximate origin as 6 months before first scheduled period
		Date:      firstRow.TanggalPosting.AddDate(0, -6, 0),
		AmountIDR: firstRow.OpeningCarrying.Neg(),
	}
	for i := range rows {
		cfs[i+1] = CashflowItem{
			Date:      rows[i].TanggalPosting,
			AmountIDR: rows[i].CashInflow.Add(rows[i].PelunasanPokok),
		}
	}
	return cfs
}

// ─── Worker task wiring ────────────────────────────────────────────────────────

// BulkRecomputeTaskType is the Asynq task type name.
const BulkRecomputeTaskType = "eir:bulk_recompute"

// BulkWorkerHandler wraps BulkService for Asynq.
type BulkWorkerHandler struct {
	svc    *BulkService
	db     *sql.DB
	logger *slog.Logger
}

// NewBulkWorkerHandler creates an BulkWorkerHandler.
func NewBulkWorkerHandler(svc *BulkService, db *sql.DB, logger *slog.Logger) *BulkWorkerHandler {
	return &BulkWorkerHandler{svc: svc, db: db, logger: logger}
}

// ProcessBulkRecomputeTask is the Asynq handler entrypoint.
// Deserialises BulkRecomputePayload, calls BulkService.Recompute, persists result.
func (h *BulkWorkerHandler) ProcessBulkRecomputeTask(ctx context.Context, payload []byte) error {
	var p BulkRecomputePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("eir worker: unmarshal payload: %w", err)
	}

	actorID, err := uuid.Parse(p.ActorID)
	if err != nil {
		return fmt.Errorf("eir worker: parse actor_id: %w", err)
	}

	result, err := h.svc.Recompute(ctx, p.Scope, p.JobID, actorID)
	if err != nil && !result.Canceled {
		if h.svc.jobRepo != nil {
			if failErr := h.svc.jobRepo.Fail(ctx, p.JobID, err.Error()); failErr != nil && h.logger != nil {
				h.logger.WarnContext(ctx, "eir worker: persist job fail status failed", "error", failErr)
			}
		}
		return fmt.Errorf("eir worker: recompute: %w", err)
	}

	if h.logger != nil {
		h.logger.InfoContext(ctx, "eir bulk recompute task complete",
			"job_id", p.JobID,
			"total", result.TotalInstruments,
			"drifts", result.DriftCount,
			"elapsed_ms", result.ElapsedMs)
	}
	return nil
}

// submitBulkRecomputeJob creates the Asynq payload JSON for a bulk recompute job.
// Called by handler when POST /api/v1/eir/bulk-recompute is received.
func submitBulkRecomputeJob(jobID string, scope BulkScope, actorID uuid.UUID) ([]byte, error) {
	payload := BulkRecomputePayload{
		JobID:   jobID,
		Scope:   scope,
		ActorID: actorID.String(),
	}
	return json.Marshal(payload)
}
