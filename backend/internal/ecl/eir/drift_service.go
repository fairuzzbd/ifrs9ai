// drift_service.go — P4-M6 stories M6-002 and M6-003 (drift detection).
//
// DriftService.GenerateReport: runs bulk EIR drift detection, creates sys.drift_report,
//
//	auto-creates HIGH-severity amendment proposals (DRIFT_DETECTION_AUTO).
//
// DriftService.GetReport:    returns DriftReportResult with embedded entry slices.
// DriftService.ListReports:  paginated list of sys.drift_report rows.
//
// Drift threshold matrix (state-machine p4-m6 §5):
//
//	abs_diff = |eir_awal - eir_recomputed|
//	abs_diff ≤ drift_flag_threshold              → no entry (OK)
//	drift_flag_threshold < abs_diff ≤ high       → LOW flag only
//	abs_diff > drift_high_threshold              → HIGH + auto-create DRAFT proposal
//	no active schedule / eir_awal IS NULL         → MISSING_SCHEDULE entry
//
// EIR re-compute uses the same Newton-Raphson solver as M5.
// All decimal comparisons via shopspring/decimal (never float64, DEC-016).
//
// Concurrent guard: DB UNIQUE partial index on sys.drift_report (status=IN_PROGRESS)
// + application-level GetInProgressReport check before insert.
//
// References:
//   - docs/stories/phase-4/M6-eir-amendment-lifecycle.md §M6-002, M6-003
//   - docs/state-machines/p4-m6-amendment-lifecycle.md §5
//   - db/migrations/000027_drift_report_and_amendment_lifecycle.up.sql
//   - DEC-010, DEC-013, DEC-016, DEC-018.
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

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// DriftService implements drift detection and report management (M6-002, M6-003).
type DriftService struct {
	db          *sql.DB
	instrRepo   InstrumenEIRRepoIface
	schedRepo   ScheduleRepoIface
	amendRepo   AmendmentRepoIface
	driftRepo   DriftReportRepoIface
	solver      *Solver
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewDriftService constructs a DriftService.
// Panics if auditWriter is nil (audit-in-tx mandatory per DEC-018).
func NewDriftService(
	db *sql.DB,
	instrRepo InstrumenEIRRepoIface,
	schedRepo ScheduleRepoIface,
	amendRepo AmendmentRepoIface,
	driftRepo DriftReportRepoIface,
	solver *Solver,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
) *DriftService {
	if auditWriter == nil {
		panic("eir.DriftService: auditWriter must not be nil (audit-in-tx mandatory)")
	}
	return &DriftService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		driftRepo:   driftRepo,
		solver:      solver,
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// GenerateReport runs a full drift detection pass and persists results to sys.drift_report.
//
// Algorithm:
//  1. Check no IN_PROGRESS drift job is running (concurrent guard).
//  2. Load sys.parameter thresholds (drift_low_threshold, drift_high_threshold).
//  3. Create sys.drift_report row with status=IN_PROGRESS.
//  4. Stream all AC/FVOCI instruments with eir_method_flag=TRUE.
//  5. Per instrument:
//     a. If eir_awal IS NULL or no active schedule → MISSING entry.
//     b. Reconstruct cashflows from schedule rows → re-run NR solver.
//     c. Classify severity by abs_diff vs thresholds.
//     d. HIGH severity → auto-create DRAFT proposal (trigger_source=DRIFT_DETECTION_AUTO)
//     if no active proposal exists.
//  6. Update sys.drift_report with counts and status=COMPLETED.
//  7. Write audit event "EIR.DRIFT_REPORT_COMPLETED" in tx.
//
// Returns the completed DriftReport header row (without embedded entries).
// Entry slices are stored as JSON in the same report row for detail endpoint.
func (s *DriftService) GenerateReport(ctx context.Context, req DriftGenerateRequest) (*DriftReport, error) {
	// 1. Concurrent guard.
	existing, err := s.driftRepo.GetInProgressReport(ctx)
	if err != nil {
		return nil, fmt.Errorf("drift generate: check in progress: %w", err)
	}
	if existing != nil {
		jobID := ""
		if existing.AsynqJobID != nil {
			jobID = *existing.AsynqJobID
		}
		return nil, ErrEIRDriftGenerationInProgress(jobID)
	}

	// 2. Load thresholds.
	flagThreshold, highThreshold, err := s.driftRepo.LoadThresholds(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Create IN_PROGRESS row.
	now := time.Now().UTC()
	report := &DriftReport{
		ID:                 uuid.New(),
		TanggalRun:         now.Truncate(24 * time.Hour),
		TriggerSource:      req.TriggerSource,
		TriggeredBy:        req.TriggeredBy,
		Status:             DriftStatusInProgress,
		StartedAt:          &now,
		DriftFlagThreshold: flagThreshold,
		DriftHighThreshold: highThreshold,
		TenantID:           req.TenantID,
	}
	var actorID uuid.UUID
	if req.TriggeredBy != nil {
		actorID = *req.TriggeredBy
	}
	report.CreatedBy = actorID
	report.UpdatedBy = actorID

	createTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("drift generate: begin create tx: %w", err)
	}
	if err := s.driftRepo.Create(ctx, createTx, report); err != nil {
		_ = createTx.Rollback() //nolint:errcheck // best-effort rollback on create failure
		return nil, fmt.Errorf("drift generate: create report: %w", err)
	}
	if err := createTx.Commit(); err != nil {
		return nil, fmt.Errorf("drift generate: commit create: %w", err)
	}

	// 4-5. Stream instruments and process.
	var (
		driftLow    []DriftReportEntry
		driftHigh   []DriftReportEntry
		missingList []DriftMissingEntry
		errorList   []DriftErrorEntry
	)

	instCh, err := s.instrRepo.ListActiveForBulk(ctx, BulkScopeAllActive)
	if err != nil {
		return nil, fmt.Errorf("drift generate: stream instruments: %w", err)
	}

	for inst := range instCh {
		entry, missingEntry, errEntry := s.processDriftInstrument(
			ctx, inst, report.ID, flagThreshold, highThreshold, actorID, req.TenantID,
		)
		switch {
		case errEntry != nil:
			errorList = append(errorList, *errEntry)
		case missingEntry != nil:
			missingList = append(missingList, *missingEntry)
		case entry != nil && entry.Severity == DriftSeverityHigh:
			driftHigh = append(driftHigh, *entry)
		case entry != nil && entry.Severity == DriftSeverityLow:
			driftLow = append(driftLow, *entry)
		}
	}

	// 6. Update report with completion.
	completedAt := time.Now().UTC()
	report.Status = DriftStatusCompleted
	report.CompletedAt = &completedAt
	report.TotalInstrumen = len(driftLow) + len(driftHigh) + len(missingList) + len(errorList)
	report.DriftLowCount = len(driftLow)
	report.DriftHighCount = len(driftHigh)
	report.MissingScheduleCount = len(missingList)
	report.ErrorCount = len(errorList)
	if len(errorList) > 0 {
		summary := fmt.Sprintf("%d instrumen gagal diproses; first error: %s",
			len(errorList), errorList[0].ErrorMessage)
		report.ErrorSummary = &summary
	}
	report.UpdatedBy = actorID

	finishTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("drift generate: begin finish tx: %w", err)
	}
	defer finishTx.Rollback() //nolint:errcheck

	if err := s.driftRepo.Update(ctx, finishTx, report); err != nil {
		return nil, fmt.Errorf("drift generate: update report: %w", err)
	}

	// Store entry slices as JSON blobs in the report row for detail endpoint.
	// Silently skips if columns not present (forward-compat).
	_ = s.storeDriftEntries(ctx, finishTx, report.ID, driftLow, driftHigh, missingList, errorList) //nolint:errcheck // intentional best-effort

	if err := s.auditWriter.Write(ctx, finishTx, AuditEvent{
		ActorUserID: actorID,
		Action:      "EIR.DRIFT_REPORT_COMPLETED",
		EntityType:  "sys.drift_report",
		EntityID:    report.ID,
		AfterJSON: map[string]interface{}{
			"report_id":        report.ID.String(),
			"trigger_source":   string(req.TriggerSource),
			"drift_low_count":  report.DriftLowCount,
			"drift_high_count": report.DriftHighCount,
			"missing_count":    report.MissingScheduleCount,
			"error_count":      report.ErrorCount,
		},
		TenantID: req.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("drift generate: audit: %w", err)
	}

	if err := finishTx.Commit(); err != nil {
		return nil, fmt.Errorf("drift generate: commit finish: %w", err)
	}

	s.logger.Info("eir drift report completed",
		slog.String("report_id", report.ID.String()),
		slog.String("trigger", string(req.TriggerSource)),
		slog.Int("low", report.DriftLowCount),
		slog.Int("high", report.DriftHighCount),
		slog.Int("missing", report.MissingScheduleCount),
		slog.Int("errors", report.ErrorCount),
	)
	return report, nil
}

// processDriftInstrument handles one instrument during drift detection.
// Returns exactly one non-nil result pointer; others are nil.
// Never returns float64 — all comparisons via decimal.Decimal (DEC-016).
func (s *DriftService) processDriftInstrument(
	ctx context.Context,
	inst InstrumenForEIR,
	reportID uuid.UUID,
	flagThreshold, highThreshold decimal.Decimal,
	actorID uuid.UUID,
	tenantID string,
) (*DriftReportEntry, *DriftMissingEntry, *DriftErrorEntry) {
	// Missing: no eir_awal.
	if inst.EIRAwal == nil {
		return nil, &DriftMissingEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			Reason:        "eir_awal IS NULL",
		}, nil
	}

	// Missing: no active schedule rows.
	scheduleRows, err := s.schedRepo.GetActiveByPeriode(ctx, inst.ID, 0)
	if err != nil {
		return nil, nil, &DriftErrorEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			ErrorCode:     CodeEIRScheduleNotFound,
			ErrorMessage:  "failed to load schedule: " + err.Error(),
		}
	}
	if len(scheduleRows) == 0 {
		return nil, &DriftMissingEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			Reason:        "No active schedule rows",
		}, nil
	}

	// Reconstruct cashflows from schedule: CF[0] = -opening_carrying[0], rest = cash_inflow.
	cfs := make([]CashflowItem, 0, len(scheduleRows)+1)
	cfs = append(cfs, CashflowItem{
		Date:      scheduleRows[0].TanggalPosting,
		AmountIDR: scheduleRows[0].OpeningCarrying.Neg(),
	})
	for i := range scheduleRows {
		cfs = append(cfs, CashflowItem{
			Date:      scheduleRows[i].TanggalPosting,
			AmountIDR: scheduleRows[i].CashInflow,
		})
	}

	// Re-run NR solver using stored EIR as seed for better convergence (DEC-013).
	seedVal := *inst.EIRAwal
	recomputedEIR, _, solveErr := s.solver.Solve(cfs, &seedVal)
	if solveErr != nil {
		return nil, nil, &DriftErrorEntry{
			InstrumenID:   inst.ID,
			KodeInstrumen: inst.KodeInstrumen,
			ErrorCode:     CodeEIRNonConvergent,
			ErrorMessage:  "NR solver failed: " + solveErr.Error(),
		}
	}
	recomputedEIR = recomputedEIR.RoundBank(8) //nolint:mnd // 8dp per DEC-016

	// Compute abs_diff = |eir_awal - eir_recomputed|. (NUMERIC(10,8) precision, DEC-016)
	storedEIR := *inst.EIRAwal
	absDiff := storedEIR.Sub(recomputedEIR).Abs().RoundBank(8) //nolint:mnd // 8 decimal places per DEC-016

	// abs_diff <= flagThreshold → no entry (OK).
	if absDiff.LessThanOrEqual(flagThreshold) {
		return nil, nil, nil
	}

	// 10000 basis points per 1.0; round to 2 decimal places.
	bpFactor := decimal.NewFromInt(10000) //nolint:mnd // bp conversion factor
	basisPoints := absDiff.Mul(bpFactor).RoundBank(2)

	severity := DriftSeverityLow
	if absDiff.GreaterThan(highThreshold) {
		severity = DriftSeverityHigh
	}

	entry := &DriftReportEntry{
		InstrumenID:   inst.ID,
		KodeInstrumen: inst.KodeInstrumen,
		EIRAwal:       storedEIR,
		EIRRecomputed: recomputedEIR,
		AbsDiff:       absDiff,
		BasisPoints:   basisPoints,
		Severity:      severity,
	}

	// HIGH: auto-create DRAFT proposal (trigger_source=DRIFT_DETECTION_AUTO) if no active exists.
	if severity == DriftSeverityHigh {
		hasActive, checkErr := s.amendRepo.HasActiveProposal(ctx, inst.ID)
		if checkErr == nil && !hasActive {
			proposal := &AmendmentProposal{
				ID:                uuid.New(),
				InstrumenID:       inst.ID,
				TanggalAmandemen:  time.Now().UTC().Truncate(24 * time.Hour),
				TanggalReEstimasi: time.Now().UTC().Truncate(24 * time.Hour),
				AlasanAmandemen: fmt.Sprintf("Auto-detected EIR drift: %s bp (HIGH severity)",
					basisPoints.StringFixed(2)),
				EIRLama:       inst.EIRAwal,
				Status:        AmendStatusDraft,
				TriggerSource: AmendTriggerDriftDetectionAuto,
				DriftReportID: &reportID,
				MakerID:       &actorID,
				CreatedBy:     actorID,
				UpdatedBy:     actorID,
				TenantID:      tenantID,
			}
			createTx, createErr := s.db.BeginTx(ctx, nil)
			if createErr == nil {
				if insertErr := s.amendRepo.Create(ctx, createTx, proposal); insertErr == nil {
					_ = createTx.Commit() //nolint:errcheck // best-effort commit for auto-proposal
					entry.ProposalCreated = true
					entry.ProposalID = &proposal.ID
				} else {
					_ = createTx.Rollback() //nolint:errcheck // best-effort rollback on insert failure
					s.logger.Warn("drift: failed to auto-create proposal",
						slog.String("instrumen_id", inst.ID.String()),
						slog.Any("error", insertErr),
					)
				}
			}
		}
	}

	return entry, nil, nil
}

// storeDriftEntries persists entry slices as JSON to sys.drift_report.
// Silently skips if JSONB columns are not present (forward-compatible).
func (s *DriftService) storeDriftEntries(
	ctx context.Context,
	tx *sql.Tx,
	reportID uuid.UUID,
	low, high []DriftReportEntry,
	missing []DriftMissingEntry,
	errEntries []DriftErrorEntry,
) error {
	allDrift := make([]DriftReportEntry, 0, len(low)+len(high))
	allDrift = append(allDrift, low...)
	allDrift = append(allDrift, high...)
	driftJSON, e1 := json.Marshal(allDrift)
	missingJSON, e2 := json.Marshal(missing)
	errorJSON, e3 := json.Marshal(errEntries)
	if e1 != nil || e2 != nil || e3 != nil {
		return fmt.Errorf("storeDriftEntries: marshal failed")
	}

	q := `UPDATE sys.drift_report
		SET drift_entries_json   = $1::JSONB,
		    missing_entries_json = $2::JSONB,
		    error_entries_json   = $3::JSONB
		WHERE id = $4`
	_, err := tx.ExecContext(ctx, q, driftJSON, missingJSON, errorJSON, reportID)
	return err
}

// GetReport returns the complete drift report with embedded entry slices.
func (s *DriftService) GetReport(ctx context.Context, reportID uuid.UUID) (*DriftReportResult, error) {
	report, err := s.driftRepo.GetByID(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("get report: %w", err)
	}
	if report == nil {
		return nil, ErrEIRDriftReportNotFound(reportID.String())
	}

	result := &DriftReportResult{Report: *report}

	// Load entry JSON blobs if available (may not exist for older rows or if columns not added).
	q := `SELECT COALESCE(drift_entries_json,'[]'::JSONB)::text,
	             COALESCE(missing_entries_json,'[]'::JSONB)::text,
	             COALESCE(error_entries_json,'[]'::JSONB)::text
		FROM sys.drift_report WHERE id = $1`
	row := s.db.QueryRowContext(ctx, q, reportID)
	var driftStr, missingStr, errorStr string
	if scanErr := row.Scan(&driftStr, &missingStr, &errorStr); scanErr == nil {
		_ = json.Unmarshal([]byte(driftStr), &result.DriftEntries)     //nolint:errcheck // best-effort JSON decode
		_ = json.Unmarshal([]byte(missingStr), &result.MissingEntries) //nolint:errcheck // best-effort JSON decode
		_ = json.Unmarshal([]byte(errorStr), &result.ErrorEntries)     //nolint:errcheck // best-effort JSON decode
	}

	return result, nil
}

// ListReports returns paginated drift report headers.
func (s *DriftService) ListReports(ctx context.Context, q listquery.Query, cursor string, limit int) ([]DriftReport, *response.PaginationMeta, error) {
	return s.driftRepo.List(ctx, q, cursor, limit)
}
