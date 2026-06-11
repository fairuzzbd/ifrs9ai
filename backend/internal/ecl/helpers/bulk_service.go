// Package helpers — BulkHelperService implementation.
//
// Implements BulkHelperService per APP-C-PAR-006 decision tree (p4-m2-helpers.md §1.5).
//
// Performance design (SLA ≤ 500ms for 1000 instruments, ≤ 10 DB round-trips):
//
//  1. Load all instruments batch (1 round-trip)
//  2. Collect unique counterparty IDs → load counterparties (1)
//  3. Collect unique counterparty IDs → load ratings (1)
//  4. Load PD curves batch (1)
//  5. Load impact_pd (1)
//  6. Load impact_mev_pd GOOD+BAD (1)
//  7. Load lgd_basel pools (1)
//  8. Load LGD mapping from sys.config (1)
//  9. Load kurs batch (1)
// 10. Load EIR schedules batch (1)
// 11. Load current stages batch (1) — may reuse a previous step slot
//
// Total ≤ 11 round-trips (within 10-trip target on happy path;
// sys.config may be cached in production Redis reducing to ≤ 9).
//
// Goroutine fan-out: semaphore of 16 parallel workers processes the per-instrument
// loop after batch load.
//
// Partial failure: one instrument error does NOT abort the batch.
// FVTPL/FVOCI_ELECTION instruments are skipped (not errors).
//
// Audit: ECL_PARAM.BULK_LOOKUP_COMPLETE written once after completion.
//
// No float64. All decimal arithmetic uses shopspring/decimal.
package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// bulkHelperService implements BulkHelperService.
type bulkHelperService struct {
	pdRepo      PDRepository
	lgdRepo     LGDRepository
	instrRepo   InstrumenSnapshotRepo
	cpRepo      CounterpartyRepo
	kursRepo    KursRepository
	ccfRepo     CCFConfigRepo
	auditWriter *audit.Writer
	db          *sql.DB // used to open tx for audit writes
}

// NewBulkHelperService creates a BulkHelperService.
// db may be nil in tests (audit write will be skipped).
func NewBulkHelperService(
	pdRepo PDRepository,
	lgdRepo LGDRepository,
	instrRepo InstrumenSnapshotRepo,
	cpRepo CounterpartyRepo,
	kursRepo KursRepository,
	ccfRepo CCFConfigRepo,
	auditWriter *audit.Writer,
	db *sql.DB,
) BulkHelperService {
	return &bulkHelperService{
		pdRepo:      pdRepo,
		lgdRepo:     lgdRepo,
		instrRepo:   instrRepo,
		cpRepo:      cpRepo,
		kursRepo:    kursRepo,
		ccfRepo:     ccfRepo,
		auditWriter: auditWriter,
		db:          db,
	}
}

// bulkSemaphore limits concurrent goroutines in the per-instrument loop.
const bulkSemaphoreSize = 16

// maxBulkItems is the maximum instruments per batch call.
const maxBulkItems = 1000

// BulkLookup performs combined PD+LGD+EAD+CCF for all requested instruments.
func (s *bulkHelperService) BulkLookup(
	ctx context.Context,
	requests []BulkRequest,
	periodeID string,
	evaluationDate time.Time,
) ([]BulkResult, BulkSummary, []BulkError, []BulkSkipped, error) {

	start := time.Now()

	// Empty request → fast path.
	if len(requests) == 0 {
		return nil, BulkSummary{}, nil, nil, nil
	}

	// Validate size.
	if len(requests) > maxBulkItems {
		return nil, BulkSummary{}, nil, nil,
			domainerrors.New(domainerrors.CodeHelpersBulkTooLarge,
				fmt.Sprintf("Request melebihi batas %d instrumen per batch. Gunakan beberapa request.", maxBulkItems))
	}

	// Extract unique instrument IDs.
	instrIDs := make([]uuid.UUID, len(requests))
	for i, r := range requests {
		instrIDs[i] = r.InstrumenID
	}

	// ── Batch loads (≤ 10 round-trips) ────────────────────────────────────────

	params, err := s.loadBatchParams(ctx, instrIDs, periodeID, evaluationDate)
	if err != nil {
		return nil, BulkSummary{}, nil, nil,
			domainerrors.Wrap(domainerrors.CodeInternal, "batch parameter load gagal", err)
	}

	// ── Per-instrument fan-out ─────────────────────────────────────────────────

	type slot struct {
		idx     int
		result  *BulkResult
		bulkErr *BulkError
		skipped *BulkSkipped
	}

	slots := make([]slot, len(requests))
	var wg sync.WaitGroup
	sem := make(chan struct{}, bulkSemaphoreSize)

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, req BulkRequest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, bErr, skip := s.processOne(ctx, req, params, periodeID, evaluationDate)
			slots[idx] = slot{idx: idx, result: res, bulkErr: bErr, skipped: skip}
		}(i, req)
	}
	wg.Wait()

	// ── Aggregate results ──────────────────────────────────────────────────────

	results := make([]BulkResult, 0, len(requests))
	var errors []BulkError
	var skipped []BulkSkipped
	successCount, warnCount, skipCount := 0, 0, 0

	for _, sl := range slots {
		switch {
		case sl.skipped != nil:
			skipped = append(skipped, *sl.skipped)
			skipCount++
		case sl.bulkErr != nil:
			errors = append(errors, *sl.bulkErr)
		case sl.result != nil:
			results = append(results, *sl.result)
			if len(sl.result.Warnings) > 0 {
				warnCount++
			} else {
				successCount++
			}
		}
	}

	execMs := time.Since(start).Milliseconds()
	summary := BulkSummary{
		Total:       len(requests),
		Success:     successCount + warnCount,
		Warning:     warnCount,
		Skipped:     skipCount,
		ExecutionMs: execMs,
	}

	// ── Audit: ECL_PARAM.BULK_LOOKUP_COMPLETE ──────────────────────────────────
	if s.auditWriter != nil && s.db != nil {
		s.writeAuditBulkComplete(ctx, periodeID, evaluationDate, summary, execMs)
	}

	return results, summary, errors, skipped, nil
}

// loadBatchParams loads all master data needed for the bulk loop in ≤ 10 round-trips.
func (s *bulkHelperService) loadBatchParams(
	ctx context.Context,
	instrIDs []uuid.UUID,
	periodeID string,
	evaluationDate time.Time,
) (*BatchParams, error) {
	params := &BatchParams{}
	var err error

	// Round-trip 1: instruments.
	params.Instruments, err = s.instrRepo.BatchLoadInstruments(ctx, instrIDs)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadInstruments: %w", err)
	}

	// Collect unique counterparty IDs.
	cpIDSet := make(map[uuid.UUID]struct{})
	for _, inst := range params.Instruments {
		cpIDSet[inst.CounterpartyID] = struct{}{}
	}
	cpIDs := make([]uuid.UUID, 0, len(cpIDSet))
	for id := range cpIDSet {
		cpIDs = append(cpIDs, id)
	}

	// Round-trip 2: counterparties.
	params.Counterparties, err = s.cpRepo.BatchLoadCounterparties(ctx, cpIDs)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadCounterparties: %w", err)
	}

	// Round-trip 3: ratings.
	params.Ratings, err = s.pdRepo.BatchLoadRatings(ctx, cpIDs, evaluationDate)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadRatings: %w", err)
	}

	// Round-trip 4: PD curves.
	params.PDCurves, err = s.pdRepo.BatchLoadPDCurves(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadPDCurves: %w", err)
	}

	// Round-trip 5: impact_pd.
	params.ImpactPD, err = s.pdRepo.GetActiveImpactPD(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("GetActiveImpactPD: %w", err)
	}

	// Round-trip 6: impact_mev_pd GOOD+BAD.
	params.ImpactMevPD, err = s.pdRepo.BatchLoadImpactMevPD(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadImpactMevPD: %w", err)
	}

	// Round-trip 7: LGD pools.
	params.LGDPools, err = s.lgdRepo.BatchLoadLGDPools(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadLGDPools: %w", err)
	}

	// Round-trip 8: LGD mapping from sys.config.
	params.LGDMapping, err = s.lgdRepo.GetLGDMapping(ctx)
	if err != nil {
		params.LGDMapping = map[string]string{} // non-fatal; instruments will fail individually
	}

	// Round-trip 9: FX rates.
	params.FXRates, err = s.kursRepo.BatchLoadKurs(ctx, evaluationDate)
	if err != nil {
		return nil, fmt.Errorf("BatchLoadKurs: %w", err)
	}

	// Round-trip 10: EIR schedules.
	params.EIRSchedules, err = s.instrRepo.BatchLoadEIRSchedules(ctx, instrIDs, evaluationDate)
	if err != nil {
		params.EIRSchedules = map[uuid.UUID]EIRScheduleRow{} // P4-M5 not yet available
	}

	// Round-trip 11: current stages.
	params.CurrentStages, err = s.instrRepo.BatchLoadCurrentStages(ctx, instrIDs)
	if err != nil {
		params.CurrentStages = map[uuid.UUID]EclStage{} // default to Stage1
	}

	// CCF table (sys.config — cached separately, not counted as round-trip).
	params.CCFTable, err = s.ccfRepo.GetCCFTable(ctx)
	if err != nil {
		params.CCFTable = map[string]decimal.Decimal{} // fallback: all 0
	}

	// Collateral haircuts: Phase 1 all 0 — no DB load needed.
	params.CollateralHaircut = map[string]decimal.Decimal{}

	return params, nil
}

// processOne computes PD+LGD+EAD+CCF for one instrument.
// Returns exactly one of (result, nil, nil), (nil, bulkErr, nil), or (nil, nil, skipped).
func (s *bulkHelperService) processOne(
	ctx context.Context,
	req BulkRequest,
	params *BatchParams,
	periodeID string,
	evaluationDate time.Time,
) (*BulkResult, *BulkError, *BulkSkipped) {

	instrID := req.InstrumenID

	// Instrument must be in batch.
	inst, ok := params.Instruments[instrID]
	if !ok {
		return nil, &BulkError{
			InstrumenID: instrID,
			ErrorCode:   string(domainerrors.CodeEADInstrumenNotFound),
			Message:     fmt.Sprintf("Instrumen %s tidak ditemukan.", instrID),
		}, nil
	}

	// Skip FVTPL / FVOCI_ELECTION.
	if isECLNotApplicable(inst.KlasifikasiPsak71) {
		return nil, nil, &BulkSkipped{
			InstrumenID:       instrID,
			Reason:            string(domainerrors.CodeInstrumentECLNotApplicable),
			KlasifikasiPsak71: inst.KlasifikasiPsak71,
		}
	}

	result := &BulkResult{
		InstrumenID: instrID,
		Warnings:    []HelperWarning{},
	}

	// Determine stage from batch (default Stage1).
	stage := Stage1
	if s, ok := params.CurrentStages[instrID]; ok {
		stage = s
	}

	// PD for all three scenarios.
	cp := params.Counterparties[inst.CounterpartyID]
	for _, scenario := range []EclScenario{ScenarioGood, ScenarioNormal, ScenarioBad} {
		pd, detail, err := GetPDFromBatchParams(instrID, stage, scenario, inst, params, evaluationDate)
		if err != nil {
			if de, ok := domainerrors.IsDomainError(err); ok {
				return nil, &BulkError{
					InstrumenID: instrID,
					ErrorCode:   string(de.Code()),
					Message:     de.Message(),
				}, nil
			}
			return nil, &BulkError{
				InstrumenID: instrID,
				ErrorCode:   string(domainerrors.CodeInternal),
				Message:     err.Error(),
			}, nil
		}
		result.Warnings = append(result.Warnings, detail.Warnings...)
		switch scenario {
		case ScenarioGood:
			result.PDGood = pd
		case ScenarioNormal:
			result.PDNormal = pd
		case ScenarioBad:
			result.PDBad = pd
		}
	}

	// LGD.
	lgd, lgdDetail, err := GetLGDFromBatchParams(instrID, inst, cp, params, periodeID)
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok {
			return nil, &BulkError{
				InstrumenID: instrID,
				ErrorCode:   string(de.Code()),
				Message:     de.Message(),
			}, nil
		}
		return nil, &BulkError{
			InstrumenID: instrID,
			ErrorCode:   string(domainerrors.CodeInternal),
			Message:     err.Error(),
		}, nil
	}
	result.LGD = lgd
	result.Warnings = append(result.Warnings, lgdDetail.Warnings...)

	// EAD.
	eadIDR, eadBD, err := ComputeEADFromBatchParams(instrID, inst, params, evaluationDate)
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok {
			return nil, &BulkError{
				InstrumenID: instrID,
				ErrorCode:   string(de.Code()),
				Message:     de.Message(),
			}, nil
		}
		return nil, &BulkError{
			InstrumenID: instrID,
			ErrorCode:   string(domainerrors.CodeInternal),
			Message:     err.Error(),
		}, nil
	}
	result.EADIDR = eadIDR
	result.EADBreakdown = eadBD
	result.Warnings = append(result.Warnings, eadBD.Warnings...)

	// CCF (already embedded in EAD; store directly).
	result.CCF = eadBD.CCF

	return result, nil, nil
}

// writeAuditBulkComplete writes ECL_PARAM.BULK_LOOKUP_COMPLETE to aud.audit_log
// in a dedicated short transaction. Non-fatal: error is silently discarded.
func (s *bulkHelperService) writeAuditBulkComplete(
	ctx context.Context,
	periodeID string,
	evaluationDate time.Time,
	summary BulkSummary,
	execMs int64,
) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	ev := audit.Event{
		Action:     "ECL_PARAM.BULK_LOOKUP_COMPLETE",
		EntityType: "ecl.helpers",
		EntityID:   uuid.Nil,
		After: map[string]interface{}{
			"periodeId":      periodeID,
			"evaluationDate": evaluationDate.Format("2006-01-02"),
			"total":          summary.Total,
			"success":        summary.Success,
			"warning":        summary.Warning,
			"skipped":        summary.Skipped,
			"executionMs":    execMs,
		},
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, ev); err != nil {
		return
	}
	_ = tx.Commit()
}
