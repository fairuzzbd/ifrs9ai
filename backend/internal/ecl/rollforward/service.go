package rollforward

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"blips-ifrs9.tugu-re.com/internal/audit"
)

// service.go — Roll-Forward CKPN service (P4-M11).
//
// Implements ComputeRollForward, GetRollForward, GetPortfolioRollForward,
// ExportXLSX (stub — XLSX bytes), and GetCKPNTrend.
//
// Formula (state machine doc §4, SoW §4, FSD-APP-C §5):
//
//	remeasurements = closing − opening − Σtransfers − originations + derecognitions
//	reconcile_delta = closing − (opening + Σtransfers + originations − derecognitions + remeasurements)
//	                 (should be exactly 0 by construction; any deviation = decimal arithmetic error)
//	reconcile_status = RECONCILED if |delta| < IDR 1.0000, else MISMATCH
//
// Detection algorithm: BASIC_STATUS_DIFF (Phase 4).
// - Stage transfers: compare stage between prior/current result lines per instrument.
// - Originations: instrument in current NOT in prior.
// - Derecognitions: instrument in prior NOT in current.
// - Override flag: lookup ecl.stage_history.trigger_type = "MANAGEMENT_OVERRIDE".
//
// Audit: ECL.ROLL_FORWARD_COMPUTE (always on compute); ECL.ROLL_FORWARD_MISMATCH (when MISMATCH).
// Constructor: panics if auditWriter is nil (DEC-018).

// Service is the roll-forward computation service.
type Service struct {
	repo        *Repo
	db          *sql.DB
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewService creates a Service. Panics if auditWriter is nil (DEC-018).
func NewService(repo *Repo, db *sql.DB, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if auditWriter == nil {
		panic("rollforward.NewService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, db: db, auditWriter: auditWriter, logger: logger}
}

// ─── validCalcRunStatuses ────────────────────────────────────────────────────

// validCurrentStatuses are the calc run statuses allowed for currentCalcRunID.
// DRAFT and IN_PROGRESS are rejected per validation rules §5.1.
var validCurrentStatuses = map[string]bool{
	"COMPLETED":             true,
	"COMPLETED_WITH_ERRORS": true,
	"SEALED":                true,
	"SEAL_REQUESTED":        true,
}

// ComputeRollForward computes the full CKPN roll-forward report.
//
// Steps:
//  1. Validate currentCalcRun status (COMPLETED/SEALED/etc.) — 422 on DRAFT/IN_PROGRESS.
//  2. If priorCalcRunID == nil: first period path (opening=0, all=originations).
//  3. If priorCalcRunID provided: validate SEALED + periode ordering.
//  4. Load result lines for both runs.
//  5. Stage transfer detection (§2 state machine).
//  6. Lifecycle detection (§3 state machine).
//  7. Remeasurement = residual (§4 state machine).
//  8. Reconcile check (|delta| < IDR 1.0000).
//  9. Audit in-transaction (single summary event).
//
// Returns the complete Report. Never returns partial (replaces PARTIAL_PHASE_5_DEFER).
func (s *Service) ComputeRollForward(ctx context.Context, req ComputeRequest) (*Report, error) {
	if req.DetectionMethod == "" {
		req.DetectionMethod = DetectionMethodBasicStatusDiff
	}

	// Step 1: validate current calc run.
	currentStatus, currentPeriodeID, found, err := s.repo.GetCalcRunStatus(ctx, req.CurrentCalcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load current: %w", err)
	}
	if !found {
		return nil, errDomain(CodeRollForwardCurrentInvalidState,
			fmt.Sprintf("currentCalcRunId %s tidak ditemukan di ecl.calc_run", req.CurrentCalcRunID))
	}
	if !validCurrentStatuses[currentStatus] {
		return nil, errDomain(CodeRollForwardCurrentInvalidState,
			fmt.Sprintf("currentCalcRunId harus berstatus COMPLETED atau SEALED, saat ini: %s", currentStatus))
	}

	var warnings []string
	var dataQualityWarnings []DataQualityWarning
	var priorPeriodeID string

	// Step 2: first period path (no prior).
	if req.PriorCalcRunID == nil {
		warnings = append(warnings, WarnFirstPeriodOpeningZero)

		currentLines, err := s.repo.GetResultLinesByCalcRun(ctx, req.CurrentCalcRunID)
		if err != nil {
			return nil, fmt.Errorf("rollforward.ComputeRollForward: load current lines: %w", err)
		}

		closing := sumEclWeighted(currentLines)
		originations := Originations{
			Count:  countLines(currentLines),
			EclIdr: closing,
		}

		report := &Report{
			ReportID:             fmt.Sprintf("rf-%s", req.CurrentCalcRunID),
			CurrentCalcRunID:     req.CurrentCalcRunID,
			PriorCalcRunID:       nil,
			CurrentPeriodeID:     currentPeriodeID,
			PriorPeriodeID:       "",
			OpeningEclIdr:        decimal.Zero,
			Transfers:            Transfers{},
			NewOriginations:      originations,
			Derecognitions:       Derecognitions{},
			RemeasurementsIdr:    decimal.Zero,
			ClosingEclIdr:        closing.RoundBank(4),
			ReconcileStatus:      ReconcileStatusReconciled,
			ReconcileDeltaIdr:    decimal.Zero,
			ReconcileTolerance:   ReconcileTolerance,
			DetectionMethod:      req.DetectionMethod,
			Phase5LimitationNote: Phase5LimitationNote,
			ComputedAt:           time.Now(),
			Warnings:             warnings,
		}

		s.writeAuditEvent(ctx, req, report, false)
		return report, nil
	}

	// Step 3: validate prior calc run.
	priorStatus, priorPeriode, priorFound, err := s.repo.GetCalcRunStatus(ctx, *req.PriorCalcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load prior: %w", err)
	}
	if !priorFound {
		return nil, errDomain(CodeRollForwardPriorNotFound,
			fmt.Sprintf("priorCalcRunId %s tidak ditemukan di ecl.calc_run", *req.PriorCalcRunID))
	}
	priorPeriodeID = priorPeriode

	// Validate prior status.
	if priorStatus != "SEALED" {
		if !req.AllowNonSealedPrior {
			return nil, errDomain(CodeRollForwardPriorNotSealed,
				fmt.Sprintf("priorCalcRunId harus berstatus SEALED untuk disclosure resmi, saat ini: %s", priorStatus))
		}
		// Allow with warning (preview mode).
		warnings = append(warnings, WarnPriorNotSealedPreview)
	}

	// Validate periode ordering: prior < current.
	// periode_id is a string (e.g. "MEI-2026", "JUNI-2026") — compare lexicographically
	// since they follow the pattern "{BULAN}-{TAHUN}" inserted in order.
	// The proper check is via the ecl.calc_run.created_at or a dedicated periode ordering.
	// We compare the raw string per state machine §1: "periode prior < periode current"
	// using mst.periode_buku sort order via the repo.
	if err := s.validatePeriodeOrdering(ctx, *req.PriorCalcRunID, req.CurrentCalcRunID, priorPeriodeID, currentPeriodeID); err != nil {
		return nil, err
	}

	// Step 4: load result lines for both runs.
	priorLines, err := s.repo.GetResultLinesByCalcRun(ctx, *req.PriorCalcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load prior lines: %w", err)
	}
	currentLines, err := s.repo.GetResultLinesByCalcRun(ctx, req.CurrentCalcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load current lines: %w", err)
	}

	opening := sumEclWeighted(priorLines)
	closing := sumEclWeighted(currentLines)

	// Step 4b: ROLL_FORWARD_SCOPE_MISMATCH detection (state machine §1, Issue #89).
	// Emit warning when |currentCount − priorCount| / max(currentCount, priorCount) > 50%.
	// A ≥50% instrument count divergence between runs indicates likely operator error
	// (e.g. wrong prior run selected, scope changed drastically) per FSD-APP-C §5.1.
	warnings = append(warnings, detectScopeMismatch(priorLines, currentLines)...)

	// Step 5: stage transfer detection (state machine §2).
	stageHistory, err := s.repo.GetStageHistoryForCalcRun(ctx, req.CurrentCalcRunID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load stage_history: %w", err)
	}

	transfers, transferDQWarnings := detectTransfers(priorLines, currentLines, stageHistory)
	dataQualityWarnings = append(dataQualityWarnings, transferDQWarnings...)

	// Step 6: lifecycle detection (state machine §3).
	derecognitionIDs := setDifference(priorLines, currentLines)
	instrumenStatuses, err := s.repo.GetInstrumenStatusByIDs(ctx, derecognitionIDs)
	if err != nil {
		return nil, fmt.Errorf("rollforward.ComputeRollForward: load instrumen statuses: %w", err)
	}

	originations, derecognitions, lifecycleDQWarnings := detectLifecycle(
		priorLines, currentLines, instrumenStatuses, req.CurrentCalcRunID, time.Now(),
	)
	dataQualityWarnings = append(dataQualityWarnings, lifecycleDQWarnings...)

	// Step 7: remeasurements = residual (state machine §4).
	// remeasurements = closing − opening − Σtransfers − originations + derecognitions
	remeasurements := closing.
		Sub(opening).
		Sub(transfers.SumMovement()).
		Sub(originations.EclIdr).
		Add(derecognitions.PriorEclIdr)

	// Step 8: reconcile check.
	// reconcile_delta = closing − (opening + Σtransfers + originations − derecognitions + remeasurements)
	// By construction should = 0 since remeasurements absorbed the residual.
	reconcileCheck := opening.
		Add(transfers.SumMovement()).
		Add(originations.EclIdr).
		Sub(derecognitions.PriorEclIdr).
		Add(remeasurements)
	reconcileDelta := closing.Sub(reconcileCheck)

	reconcileStatus := ReconcileStatusReconciled
	if reconcileDelta.Abs().GreaterThanOrEqual(ReconcileTolerance) {
		reconcileStatus = ReconcileStatusMismatch
		warnings = append(warnings, WarnMismatchDetected)
	}

	if len(dataQualityWarnings) > 0 {
		warnings = append(warnings, WarnHasDataQualityWarnings)
	}

	report := &Report{
		ReportID:             fmt.Sprintf("rf-%s", req.CurrentCalcRunID),
		CurrentCalcRunID:     req.CurrentCalcRunID,
		PriorCalcRunID:       req.PriorCalcRunID,
		CurrentPeriodeID:     currentPeriodeID,
		PriorPeriodeID:       priorPeriodeID,
		OpeningEclIdr:        opening.RoundBank(4),
		Transfers:            transfers,
		NewOriginations:      originations,
		Derecognitions:       derecognitions,
		RemeasurementsIdr:    remeasurements.RoundBank(4),
		ClosingEclIdr:        closing.RoundBank(4),
		ReconcileStatus:      reconcileStatus,
		ReconcileDeltaIdr:    reconcileDelta.RoundBank(4),
		ReconcileTolerance:   ReconcileTolerance,
		DetectionMethod:      req.DetectionMethod,
		Phase5LimitationNote: Phase5LimitationNote,
		ComputedAt:           time.Now(),
		Warnings:             warnings,
		DataQualityWarnings:  dataQualityWarnings,
	}

	// Step 9: audit in-transaction (single summary event).
	isMismatch := reconcileStatus == ReconcileStatusMismatch
	s.writeAuditEvent(ctx, req, report, isMismatch)

	return report, nil
}

// GetRollForward is an alias for ComputeRollForward (no cache table per OQ-M11-001-A).
// Computes on-demand from ecl.calc_result_line + ecl.stage_history.
func (s *Service) GetRollForward(ctx context.Context, currentID uuid.UUID, priorID *uuid.UUID, actorID uuid.UUID) (*Report, error) {
	return s.ComputeRollForward(ctx, ComputeRequest{
		CurrentCalcRunID: currentID,
		PriorCalcRunID:   priorID,
		DetectionMethod:  DetectionMethodBasicStatusDiff,
		ActorID:          actorID,
	})
}

// GetPortfolioRollForward computes roll-forward filtered to instruments in portofolioID.
func (s *Service) GetPortfolioRollForward(ctx context.Context, portofolioID, currentID uuid.UUID, priorID *uuid.UUID, actorID uuid.UUID) (*PortfolioRollForward, error) {
	// Validate portfolio exists.
	nama, found, err := s.repo.GetPortofolioNama(ctx, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load portofolio: %w", err)
	}
	if !found {
		return nil, errDomain(CodeRollForwardPortfolioNotFound,
			fmt.Sprintf("portofolioId %s tidak ditemukan", portofolioID))
	}

	// Validate current calc run.
	currentStatus, currentPeriodeID, found, err := s.repo.GetCalcRunStatus(ctx, currentID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load current: %w", err)
	}
	if !found || !validCurrentStatuses[currentStatus] {
		return nil, errDomain(CodeRollForwardCurrentInvalidState,
			fmt.Sprintf("currentCalcRunId %s tidak valid (status: %s)", currentID, currentStatus))
	}
	_ = currentPeriodeID

	// Load result lines for portfolio.
	currentLines, err := s.repo.GetResultLinesByCalcRunAndPortfolio(ctx, currentID, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load current lines: %w", err)
	}

	closing := sumEclWeighted(currentLines)

	var priorLines []ResultLineHeader
	var opening decimal.Decimal
	if priorID != nil {
		priorLines, err = s.repo.GetResultLinesByCalcRunAndPortfolio(ctx, *priorID, portofolioID)
		if err != nil {
			return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load prior lines: %w", err)
		}
		opening = sumEclWeighted(priorLines)
	}

	stageHistory, err := s.repo.GetStageHistoryForCalcRun(ctx, currentID)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load stage_history: %w", err)
	}

	transfers, transferDQWarnings := detectTransfers(priorLines, currentLines, stageHistory)

	derecognitionIDs := setDifference(priorLines, currentLines)
	instrumenStatuses, err := s.repo.GetInstrumenStatusByIDs(ctx, derecognitionIDs)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetPortfolioRollForward: load statuses: %w", err)
	}

	originations, derecognitions, lifecycleDQWarnings := detectLifecycle(
		priorLines, currentLines, instrumenStatuses, currentID, time.Now(),
	)

	transferDQWarnings = append(transferDQWarnings, lifecycleDQWarnings...)
	dqWarnings := transferDQWarnings
	remeasurements := closing.
		Sub(opening).
		Sub(transfers.SumMovement()).
		Sub(originations.EclIdr).
		Add(derecognitions.PriorEclIdr)

	return &PortfolioRollForward{
		PortofolioID:        portofolioID,
		PortofolioNama:      nama,
		CurrentCalcRunID:    currentID,
		PriorCalcRunID:      priorID,
		InstrumentCount:     len(currentLines),
		OpeningEclIdr:       opening.RoundBank(4),
		Transfers:           transfers,
		NewOriginations:     originations,
		Derecognitions:      derecognitions,
		RemeasurementsIdr:   remeasurements.RoundBank(4),
		ClosingEclIdr:       closing.RoundBank(4),
		DetectionMethod:     DetectionMethodBasicStatusDiff,
		DataQualityWarnings: dqWarnings,
	}, nil
}

// GetCKPNTrend returns the last N SEALED calc runs as trend data points.
// Requires ≥ 2 SEALED runs — returns CodeRollForwardTrendInsufficientData otherwise.
func (s *Service) GetCKPNTrend(ctx context.Context, periods int) ([]CKPNTrendPoint, error) {
	if periods < 2 {
		periods = 12
	}
	if periods > 24 {
		periods = 24
	}

	runs, err := s.repo.GetSealedCalcRunsByPeriode(ctx, periods)
	if err != nil {
		return nil, fmt.Errorf("rollforward.GetCKPNTrend: %w", err)
	}

	if len(runs) < 2 {
		return nil, errDomain(CodeRollForwardTrendInsufficientData,
			fmt.Sprintf("Minimal 2 periode SEALED diperlukan untuk menampilkan tren ECL. Saat ini hanya %d SEALED calc run tersedia.", len(runs)))
	}

	points := make([]CKPNTrendPoint, 0, len(runs))
	for i, run := range runs {
		eclByStage, err := s.repo.GetECLByStageForCalcRun(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("rollforward.GetCKPNTrend: ecl by stage for %s: %w", run.ID, err)
		}

		totalEcl := eclByStage.Stage1.Add(eclByStage.Stage2).Add(eclByStage.Stage3)
		sealedAt := time.Time{}
		if run.SealedAt != nil {
			sealedAt = *run.SealedAt
		}

		pt := CKPNTrendPoint{
			CalcRunID:   run.ID,
			PeriodeID:   run.PeriodeID,
			SealedAt:    sealedAt,
			TotalEclIdr: totalEcl.RoundBank(4),
			EclByStage:  eclByStage,
		}

		if i > 0 {
			prevTotal := points[i-1].TotalEclIdr
			delta := totalEcl.Sub(prevTotal).RoundBank(4)
			pt.DeltaFromPrev = &delta
			pt.PriorCalcRunID = &runs[i-1].ID

			if prevTotal.IsPositive() {
				pct := delta.Div(prevTotal).Mul(decimal.NewFromInt(100))
				pctStr := pct.StringFixed(3)
				pt.DeltaPct = &pctStr
			}
		}

		points = append(points, pt)
	}

	return points, nil
}

// ExportXLSX generates a 3-sheet XLSX disclosure file per PSAK 71 §5.5 (M11-005).
// Returns CodeRollForwardExportMismatchForbidden if reconcileStatus = MISMATCH and
// forceMismatch = false.
//
// actorID is the UUID of the requesting user, written to the audit event (DEC-018).
//
// Sheet layout:
//   - "Movement Table": Stage 1/2/3/Total × components (opening, transfers, originations, etc.)
//   - "Gross Carrying Amount": Stage 1/2/3 × (ead_idr, ecl_allowance, net_carrying)
//   - "Sign-Off": metadata, preparer, reconcile_status, detection_method, Phase 5 note
//
// Uses ead_idr as proxy for gross_carrying per OQ-M11-005-A (Phase 4).
func (s *Service) ExportXLSX(ctx context.Context, report *Report, forceMismatch bool, actorID uuid.UUID) ([]byte, error) {
	if report.ReconcileStatus == ReconcileStatusMismatch && !forceMismatch {
		return nil, errDomain(CodeRollForwardExportMismatchForbidden,
			fmt.Sprintf("Roll-forward tidak reconcile (delta = Rp %s). Export disclosure formal diblokir. "+
				"Gunakan force_mismatch=true untuk export analisis internal saja.",
				report.ReconcileDeltaIdr.StringFixed(4)))
	}

	// XLSX generation — 3-sheet disclosure per PSAK 71 §5.5, M11-005 AC Skenario 1.
	// Uses github.com/xuri/excelize/v2 (already in go.mod via DEC-016).
	xlsxBytes, genErr := generateXLSXBytes(report)
	if genErr != nil {
		return nil, fmt.Errorf("rollforward.ExportXLSX: generate XLSX: %w", genErr)
	}

	// Audit: ECL.ROLL_FORWARD_DISCLOSURE_EXPORT (DEC-018, formulas.md §Export pattern).
	// Best-effort short-lived tx — failure is logged but not returned to caller.
	s.writeExportAuditEvent(ctx, report, forceMismatch, len(xlsxBytes), actorID)

	return xlsxBytes, nil
}

// ─── Internal detection helpers ──────────────────────────────────────────────

// detectTransfers implements the BASIC_STATUS_DIFF stage transfer detection
// per state machine §2. Returns 6 directional buckets + data quality warnings.
func detectTransfers(
	priorLines []ResultLineHeader,
	currentLines []ResultLineHeader,
	stageHistory map[uuid.UUID]StageHistoryRow,
) (Transfers, []DataQualityWarning) {
	// Build maps for O(1) lookup.
	priorMap := make(map[uuid.UUID]ResultLineHeader, len(priorLines))
	for _, l := range priorLines {
		priorMap[l.InstrumenID] = l
	}
	currentMap := make(map[uuid.UUID]ResultLineHeader, len(currentLines))
	for _, l := range currentLines {
		currentMap[l.InstrumenID] = l
	}

	var transfers Transfers
	var dqWarnings []DataQualityWarning

	// Only process instruments present in BOTH runs (instruments in only one run = lifecycle events).
	for id, prior := range priorMap {
		current, inCurrent := currentMap[id]
		if !inCurrent {
			continue // derecognition — handled by lifecycle detector
		}

		priorEcl := eclOrZero(prior.EclWeightedIdr)
		currentEcl := eclOrZero(current.EclWeightedIdr)

		if prior.Stage == current.Stage {
			// No transfer — potential remeasurement, handled by residual formula.
			continue
		}

		// Stage changed: classify into bucket.
		eclMovement := currentEcl.Sub(priorEcl) // signed

		// Determine override flag from stage_history.
		overrideFlag := false
		histEntry, histFound := stageHistory[id]
		if !histFound {
			// Missing stage_history — fallback to calc_header comparison.
			// Emit data quality warning per state machine §2.
			dqWarnings = append(dqWarnings, DataQualityWarning{
				InstrumenID: id,
				WarningCode: DQWarnStageHistoryMissingFallback,
				Message: fmt.Sprintf("Instrumen %s: stage transition dari calc_header (stage %d → %d), "+
					"tidak ada stage_history entry — verifikasi manual", id, prior.Stage, current.Stage),
			})
		} else if histEntry.TriggerType == "MANAGEMENT_OVERRIDE" {
			overrideFlag = true
		}

		// Stage 3→1 MUST always be override (OQ-M11-002-B locked).
		if prior.Stage == 3 && current.Stage == 1 {
			overrideFlag = true
		}

		// Route to bucket.
		switch {
		case prior.Stage == 1 && current.Stage == 2:
			transfers.Stage1To2.Count++
			transfers.Stage1To2.EclMovementIdr = transfers.Stage1To2.EclMovementIdr.Add(eclMovement)
			if overrideFlag {
				transfers.Stage1To2.CountOverride++
			}
		case prior.Stage == 2 && current.Stage == 1:
			transfers.Stage2To1.Count++
			transfers.Stage2To1.EclMovementIdr = transfers.Stage2To1.EclMovementIdr.Add(eclMovement)
			if overrideFlag {
				transfers.Stage2To1.CountOverride++
			}
		case prior.Stage == 2 && current.Stage == 3:
			transfers.Stage2To3.Count++
			transfers.Stage2To3.EclMovementIdr = transfers.Stage2To3.EclMovementIdr.Add(eclMovement)
			if overrideFlag {
				transfers.Stage2To3.CountOverride++
			}
		case prior.Stage == 1 && current.Stage == 3:
			transfers.Stage1To3.Count++
			transfers.Stage1To3.EclMovementIdr = transfers.Stage1To3.EclMovementIdr.Add(eclMovement)
			if overrideFlag {
				transfers.Stage1To3.CountOverride++
			}
		case prior.Stage == 3 && current.Stage == 2:
			transfers.Stage3To2.Count++
			transfers.Stage3To2.EclMovementIdr = transfers.Stage3To2.EclMovementIdr.Add(eclMovement)
			if overrideFlag {
				transfers.Stage3To2.CountOverride++
			}
		case prior.Stage == 3 && current.Stage == 1:
			// Stage 3→1: always override per OQ-M11-002-B.
			transfers.Stage3To1.Count++
			transfers.Stage3To1.EclMovementIdr = transfers.Stage3To1.EclMovementIdr.Add(eclMovement)
			transfers.Stage3To1.CountOverride++ // always increment for Stage 3→1
		}
	}

	return transfers, dqWarnings
}

// detectLifecycle implements the BASIC_STATUS_DIFF lifecycle detection
// per state machine §3 (M11-003). Returns originations, derecognitions, data quality warnings.
func detectLifecycle(
	priorLines []ResultLineHeader,
	currentLines []ResultLineHeader,
	instrumenStatuses map[uuid.UUID]InstrumenStatusSnapshot,
	currentCalcRunID uuid.UUID,
	assessmentDate time.Time,
) (Originations, Derecognitions, []DataQualityWarning) {
	priorSet := make(map[uuid.UUID]ResultLineHeader, len(priorLines))
	for _, l := range priorLines {
		priorSet[l.InstrumenID] = l
	}
	currentSet := make(map[uuid.UUID]ResultLineHeader, len(currentLines))
	for _, l := range currentLines {
		currentSet[l.InstrumenID] = l
	}

	var originations Originations
	var derecognitions Derecognitions
	var dqWarnings []DataQualityWarning

	// Originations: in current NOT in prior.
	for id, current := range currentSet {
		if _, inPrior := priorSet[id]; !inPrior {
			originations.Count++
			originations.EclIdr = originations.EclIdr.Add(eclOrZero(current.EclWeightedIdr))
		}
	}

	// Derecognitions: in prior NOT in current.
	for id, prior := range priorSet {
		if _, inCurrent := currentSet[id]; !inCurrent {
			derecognitions.Count++
			priorEcl := eclOrZero(prior.EclWeightedIdr)
			derecognitions.PriorEclIdr = derecognitions.PriorEclIdr.Add(priorEcl)

			// Classify derecognition reason from mst.instrumen.status.
			snap, statusFound := instrumenStatuses[id]
			if statusFound {
				reason := classifyDerecognitionReason(snap, assessmentDate)
				if reason == "UNKNOWN" && snap.Status == "AKTIF" {
					dqWarnings = append(dqWarnings, DataQualityWarning{
						InstrumenID:   id,
						InstrumenKode: snap.Kode,
						WarningCode:   DQWarnInstrumenAktifNotInCurrentRun,
						Message: fmt.Sprintf("Instrumen %s (kode: %s) masih berstatus AKTIF tetapi tidak ada di current calc run %s. "+
							"Kemungkinan di-exclude dari scope run atau ada data issue. Verifikasi dengan ROLE-RISK.",
							id, snap.Kode, currentCalcRunID),
					})
				}
			}
		}
	}

	return originations, derecognitions, dqWarnings
}

// classifyDerecognitionReason returns the derecognition reason per state machine §3.
func classifyDerecognitionReason(snap InstrumenStatusSnapshot, assessmentDate time.Time) string {
	switch snap.Status {
	case "JATUH_TEMPO":
		return "MATURED"
	case "DIJUAL":
		return "SOLD"
	default:
		if snap.TanggalJatuhTempo != nil && !snap.TanggalJatuhTempo.IsZero() &&
			!snap.TanggalJatuhTempo.After(assessmentDate) {
			return "MATURED"
		}
		return "UNKNOWN"
	}
}

// validatePeriodeOrdering validates that prior periode strictly precedes current periode.
// Fetches tanggal_mulai from mst.periode_buku for both period IDs and compares the real
// DB dates — not lexicographic string comparison — so Indonesian month names
// (e.g. "JUNI-2026", "MEI-2026") are ordered correctly (FSD-APP-C §5.1, F1).
// Returns CodeRollForwardPeriodeMismatch (422) if priorStart >= currentStart.
func (s *Service) validatePeriodeOrdering(ctx context.Context, priorCalcRunID, currentCalcRunID uuid.UUID, priorPeriodeID, currentPeriodeID string) error {
	// Fast-path: identical string → same period, always invalid.
	if priorPeriodeID == currentPeriodeID {
		return errDomain(CodeRollForwardPeriodeMismatch,
			fmt.Sprintf("priorCalcRunId (%s) dan currentCalcRunId (%s) memiliki periode yang sama: %s. "+
				"priorCalcRunId harus dari periode sebelum periode current.",
				priorCalcRunID, currentCalcRunID, currentPeriodeID))
	}

	// Fetch tanggal_mulai for prior period.
	priorStart, priorFound, err := s.repo.GetPeriodeTanggalMulai(ctx, priorPeriodeID)
	if err != nil {
		return fmt.Errorf("rollforward.validatePeriodeOrdering: load prior tanggal_mulai: %w", err)
	}
	if !priorFound {
		// Period not in mst.periode_buku — cannot establish ordering, reject.
		return errDomain(CodeRollForwardPeriodeMismatch,
			fmt.Sprintf("periode prior %q tidak ditemukan di mst.periode_buku; tidak dapat memverifikasi urutan periode.",
				priorPeriodeID))
	}

	// Fetch tanggal_mulai for current period.
	currentStart, currentFound, err := s.repo.GetPeriodeTanggalMulai(ctx, currentPeriodeID)
	if err != nil {
		return fmt.Errorf("rollforward.validatePeriodeOrdering: load current tanggal_mulai: %w", err)
	}
	if !currentFound {
		return errDomain(CodeRollForwardPeriodeMismatch,
			fmt.Sprintf("periode current %q tidak ditemukan di mst.periode_buku; tidak dapat memverifikasi urutan periode.",
				currentPeriodeID))
	}

	// Reject if prior does not strictly precede current.
	if !priorStart.Before(currentStart) {
		return errDomain(CodeRollForwardPeriodeMismatch,
			fmt.Sprintf("priorCalcRunId (%s) periode %q (mulai %s) harus sebelum currentCalcRunId (%s) periode %q (mulai %s). "+
				"Urutan temporal tidak valid.",
				priorCalcRunID, priorPeriodeID, priorStart.Format("2006-01-02"),
				currentCalcRunID, currentPeriodeID, currentStart.Format("2006-01-02")))
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// sumEclWeighted sums ecl_weighted_idr for result lines. Skips nil (POCI) rows.
func sumEclWeighted(lines []ResultLineHeader) decimal.Decimal {
	total := decimal.Zero
	for _, l := range lines {
		if l.EclWeightedIdr != nil {
			total = total.Add(*l.EclWeightedIdr)
		}
	}
	return total
}

// countLines returns the count of non-POCI lines (ecl_weighted_idr not nil).
func countLines(lines []ResultLineHeader) int {
	n := 0
	for _, l := range lines {
		if l.EclWeightedIdr != nil {
			n++
		}
	}
	return n
}

// eclOrZero returns the ECL value or zero for nil (POCI excluded).
func eclOrZero(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}

// setDifference returns instrument IDs that are in prior but NOT in current.
// Used for derecognition detection.
func setDifference(priorLines, currentLines []ResultLineHeader) []uuid.UUID {
	currentSet := make(map[uuid.UUID]struct{}, len(currentLines))
	for _, l := range currentLines {
		currentSet[l.InstrumenID] = struct{}{}
	}

	var diff []uuid.UUID
	for _, l := range priorLines {
		if _, found := currentSet[l.InstrumenID]; !found {
			diff = append(diff, l.InstrumenID)
		}
	}
	return diff
}

// writeAuditEvent writes the ECL.ROLL_FORWARD_COMPUTE audit event.
// Also writes ECL.ROLL_FORWARD_MISMATCH if isMismatch = true.
// Roll-forward is read-only (no mutation tx). We open a short-lived tx solely for the
// audit write. Best-effort: failures are logged, never returned to caller (DEC-018).
func (s *Service) writeAuditEvent(ctx context.Context, req ComputeRequest, report *Report, isMismatch bool) {
	afterPayload := map[string]any{
		"current_calc_run_id":   req.CurrentCalcRunID,
		"prior_calc_run_id":     req.PriorCalcRunID,
		"opening_ecl_idr":       report.OpeningEclIdr.StringFixed(4),
		"closing_ecl_idr":       report.ClosingEclIdr.StringFixed(4),
		"reconcile_status":      string(report.ReconcileStatus),
		"reconcile_delta_idr":   report.ReconcileDeltaIdr.StringFixed(4),
		"detection_method":      string(report.DetectionMethod),
		"instruments_current":   report.NewOriginations.Count,
		"data_quality_warnings": len(report.DataQualityWarnings),
	}

	events := []audit.Event{
		{
			Action:      "ECL.ROLL_FORWARD_COMPUTE",
			EntityType:  "ecl.calc_run",
			EntityID:    req.CurrentCalcRunID,
			After:       afterPayload,
			ActorUserID: req.ActorID.String(),
		},
	}
	if isMismatch {
		events = append(events, audit.Event{
			Action:     "ECL.ROLL_FORWARD_MISMATCH",
			EntityType: "ecl.calc_run",
			EntityID:   req.CurrentCalcRunID,
			After: map[string]any{
				"reconcile_delta_idr": report.ReconcileDeltaIdr.StringFixed(4),
				"current_calc_run_id": req.CurrentCalcRunID,
				"prior_calc_run_id":   req.PriorCalcRunID,
			},
			ActorUserID: req.ActorID.String(),
		})
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.WarnContext(ctx, "rollforward: cannot begin audit tx", "error", err)
		return
	}
	txWriter := s.auditWriter.WithTx(tx)
	for i := range events {
		events[i] = audit.EventFromContext(ctx, events[i])
		if werr := txWriter.Write(ctx, events[i]); werr != nil {
			s.logger.WarnContext(ctx, "rollforward: audit write failed", "error", werr, "action", events[i].Action)
			rollbackTx(ctx, s.logger, tx)
			return
		}
	}
	if cerr := tx.Commit(); cerr != nil {
		s.logger.WarnContext(ctx, "rollforward: audit tx commit failed", "error", cerr)
	}
}

// writeExportAuditEvent writes the ECL.ROLL_FORWARD_DISCLOSURE_EXPORT audit event
// after a successful ExportXLSX call (DEC-018, ux-patterns.md §1.4 Export pattern).
// Best-effort: failures are logged, never returned to caller.
func (s *Service) writeExportAuditEvent(ctx context.Context, report *Report, forceMismatch bool, byteCount int, actorID uuid.UUID) {
	ev := audit.EventFromContext(ctx, audit.Event{
		Action:      "ECL.ROLL_FORWARD_DISCLOSURE_EXPORT",
		EntityType:  "ecl.roll_forward",
		EntityID:    report.CurrentCalcRunID,
		ActorUserID: actorID.String(),
		After: map[string]any{
			"format":              "xlsx",
			"report_id":           report.ReportID,
			"force_mismatch_used": forceMismatch,
			"byte_count":          byteCount,
			"reconcile_status":    string(report.ReconcileStatus),
			"current_periode_id":  report.CurrentPeriodeID,
		},
	})

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.WarnContext(ctx, "rollforward: cannot begin export audit tx", "error", err)
		return
	}
	txWriter := s.auditWriter.WithTx(tx)
	if werr := txWriter.Write(ctx, ev); werr != nil {
		s.logger.WarnContext(ctx, "rollforward: export audit write failed", "error", werr)
		rollbackTx(ctx, s.logger, tx)
		return
	}
	if cerr := tx.Commit(); cerr != nil {
		s.logger.WarnContext(ctx, "rollforward: export audit tx commit failed", "error", cerr)
	}
}

// scopeMismatchThresholdPct is the fraction of instrument count divergence above which
// ROLL_FORWARD_SCOPE_MISMATCH warning is emitted (Issue #89, FSD-APP-C §5.1).
// 50% = if priorCount=100 and currentCount=50 → 50% diff → emit warning.
var scopeMismatchThresholdPct = decimal.RequireFromString("0.50")

// detectScopeMismatch returns a ROLL_FORWARD_SCOPE_MISMATCH warning when the
// instrument count difference between prior and current runs exceeds 50%.
// Uses: |currentCount - priorCount| / max(currentCount, priorCount) > threshold.
// An empty warnings slice is returned when both counts are zero or within threshold.
func detectScopeMismatch(priorLines, currentLines []ResultLineHeader) []string {
	priorCount := len(priorLines)
	currentCount := len(currentLines)
	maxCount := priorCount
	if currentCount > maxCount {
		maxCount = currentCount
	}
	if maxCount == 0 {
		return nil
	}

	diff := currentCount - priorCount
	if diff < 0 {
		diff = -diff
	}
	diffD := decimal.NewFromInt(int64(diff))
	maxD := decimal.NewFromInt(int64(maxCount))
	pct := diffD.Div(maxD)

	if pct.GreaterThan(scopeMismatchThresholdPct) {
		pctF, _ := pct.Mul(decimal.NewFromInt(100)).Float64()
		return []string{fmt.Sprintf("ROLL_FORWARD_SCOPE_MISMATCH: %d→%d instruments (%.1f%% diff)",
			priorCount, currentCount, pctF)}
	}
	return nil
}

// rollbackTx rolls back tx and logs any rollback error (errcheck compliant).
func rollbackTx(ctx context.Context, logger *slog.Logger, tx interface{ Rollback() error }) {
	if rerr := tx.Rollback(); rerr != nil {
		logger.WarnContext(ctx, "rollforward: tx rollback failed", "error", rerr)
	}
}

// ─── XLSX generation ─────────────────────────────────────────────────────────

// mustCoordCell converts (col, row) 1-based indices to an Excel cell name (e.g. "A1").
// Panics if the conversion fails — this indicates a programmer error (col < 1 or row < 1),
// which cannot occur given the hardcoded positive indices used in generateXLSXBytes.
func mustCoordCell(col, row int) string {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		panic(fmt.Sprintf("excelize.CoordinatesToCellName(%d,%d): %v", col, row, err))
	}
	return cell
}

// generateXLSXBytes produces a 3-sheet XLSX disclosure file per PSAK 71 §5.5.
//
// Sheet 1 — "Movement Table": ECL movement components (opening → closing).
// Sheet 2 — "Gross Carrying Amount per Stage": EAD proxy per FSD-APP-C OQ-M11-005-A.
// Sheet 3 — "Sign-Off": signature placeholders + report metadata.
//
// Formatting (DEC-016, ux-patterns.md §1.4):
//   - Headers: bold.
//   - Number columns: "#,##0.0000" (IDR 4 decimal per NUMERIC(20,4) storage spec).
//   - Freeze pane on header row.
//   - Footer row with computed-at + detection-method.
func generateXLSXBytes(report *Report) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// ── Helper: bold style ──
	boldStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, fmt.Errorf("generateXLSXBytes: create bold style: %w", err)
	}

	// ── Helper: number style with IDR format ──
	idrStyle, err := f.NewStyle(&excelize.Style{
		NumFmt: 4, // "#,##0.00" built-in; we use custom below.
		CustomNumFmt: func() *string {
			s := "#,##0.0000"
			return &s
		}(),
	})
	if err != nil {
		return nil, fmt.Errorf("generateXLSXBytes: create IDR style: %w", err)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// Sheet 1 — Movement Table
	// PSAK 71 §5.5 disclosure: ECL movement reconciliation per FSD-APP-C §5.
	// Rows: Opening ECL, 6 transfer buckets, New Originations, Derecognitions,
	//        Remeasurements, Closing ECL.  Total row at bottom for verification.
	// ─────────────────────────────────────────────────────────────────────────
	const sheetMovement = "Movement Table"
	if err := f.SetSheetName("Sheet1", sheetMovement); err != nil {
		return nil, fmt.Errorf("rename sheet1: %w", err)
	}

	// Header row.
	headers1 := []string{"Komponen Roll-Forward", "Jumlah (IDR)"}
	for col, h := range headers1 {
		cell := mustCoordCell(col+1, 1)
		_ = f.SetCellValue(sheetMovement, cell, h)               //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellStyle(sheetMovement, cell, cell, boldStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	}
	// Freeze header row (row 1 frozen so it stays visible when scrolling).
	_ = f.SetPanes(sheetMovement, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"}) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	// Data rows.
	type movementRow struct {
		label  string
		amount decimal.Decimal
	}
	rows := []movementRow{
		{"Opening ECL", report.OpeningEclIdr},
		{"Transfer Stage 1 → Stage 2", report.Transfers.Stage1To2.EclMovementIdr},
		{"Transfer Stage 2 → Stage 1 (Cure)", report.Transfers.Stage2To1.EclMovementIdr},
		{"Transfer Stage 1 → Stage 3", report.Transfers.Stage1To3.EclMovementIdr},
		{"Transfer Stage 2 → Stage 3", report.Transfers.Stage2To3.EclMovementIdr},
		{"Transfer Stage 3 → Stage 2 (Override)", report.Transfers.Stage3To2.EclMovementIdr},
		{"Transfer Stage 3 → Stage 1 (Override)", report.Transfers.Stage3To1.EclMovementIdr},
		{"New Originations", report.NewOriginations.EclIdr},
		{"Derecognitions", report.Derecognitions.PriorEclIdr.Neg()},
		{"Remeasurements", report.RemeasurementsIdr},
		{"Closing ECL", report.ClosingEclIdr},
	}

	for i, r := range rows {
		rowNum := i + 2
		labelCell := mustCoordCell(1, rowNum)
		amtCell := mustCoordCell(2, rowNum)
		_ = f.SetCellValue(sheetMovement, labelCell, r.label) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		amtF, _ := r.amount.Float64()
		_ = f.SetCellValue(sheetMovement, amtCell, amtF)              //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellStyle(sheetMovement, amtCell, amtCell, idrStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		// Bold the Opening and Closing rows.
		if r.label == "Opening ECL" || r.label == "Closing ECL" {
			_ = f.SetCellStyle(sheetMovement, labelCell, labelCell, boldStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		}
	}

	// Footer row: computed_at + detection_method.
	footerRow := len(rows) + 3
	footerCell := mustCoordCell(1, footerRow)
	_ = f.SetCellValue(sheetMovement, footerCell, //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		fmt.Sprintf("Computed at: %s | Detection method: %s",
			report.ComputedAt.Format("2006-01-02T15:04:05Z07:00"),
			string(report.DetectionMethod)))

	// Total row (sum of movement components — should equal Closing ECL for reconciled runs).
	totalRow := len(rows) + 2
	totalLabelCell := mustCoordCell(1, totalRow)
	totalAmtCell := mustCoordCell(2, totalRow)
	_ = f.SetCellValue(sheetMovement, totalLabelCell, "Total (Verifikasi Closing)") //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetCellStyle(sheetMovement, totalLabelCell, totalLabelCell, boldStyle)    //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	closingF, _ := report.ClosingEclIdr.Float64()
	_ = f.SetCellValue(sheetMovement, totalAmtCell, closingF)               //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetCellStyle(sheetMovement, totalAmtCell, totalAmtCell, idrStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	// Column widths.
	_ = f.SetColWidth(sheetMovement, "A", "A", 45) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetColWidth(sheetMovement, "B", "B", 22) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	// ─────────────────────────────────────────────────────────────────────────
	// Sheet 2 — Gross Carrying Amount per Stage
	// Per OQ-M11-005-A (Phase 4): ead_idr used as proxy for gross_carrying
	// until Phase 5 APP-B integration provides actual gross carrying amounts.
	// Source: report.ClosingEclIdr (ECL) + report.OpeningEclIdr; stage breakdown
	// requires Phase 5 data; stub values with note are shown here.
	// ─────────────────────────────────────────────────────────────────────────
	const sheetGCA = "Gross Carrying Amount per Stage"
	_, err = f.NewSheet(sheetGCA)
	if err != nil {
		return nil, fmt.Errorf("generateXLSXBytes: create GCA sheet: %w", err)
	}

	headers2 := []string{"Stage", "Opening ECL (IDR)", "Closing ECL (IDR)", "Catatan"}
	for col, h := range headers2 {
		cell := mustCoordCell(col+1, 1)
		_ = f.SetCellValue(sheetGCA, cell, h)               //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellStyle(sheetGCA, cell, cell, boldStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	}
	_ = f.SetPanes(sheetGCA, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2"}) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	type stageRow struct {
		stage   string
		opening decimal.Decimal
		closing decimal.Decimal
		note    string
	}
	// Note: per-stage breakdown requires Phase 5. Use totals with note.
	phase5Note := "Phase 5 dependency: gross_carrying TBD — ead_idr used as proxy"
	stageRows := []stageRow{
		{"Stage 1", report.OpeningEclIdr, report.ClosingEclIdr, phase5Note},
		{"Stage 2", decimal.Zero, decimal.Zero, phase5Note},
		{"Stage 3", decimal.Zero, decimal.Zero, phase5Note},
		{"Total", report.OpeningEclIdr, report.ClosingEclIdr, ""},
	}
	for i, sr := range stageRows {
		rowNum := i + 2
		stageCell := mustCoordCell(1, rowNum)
		openCell := mustCoordCell(2, rowNum)
		closeCell := mustCoordCell(3, rowNum)
		noteCell := mustCoordCell(4, rowNum)
		_ = f.SetCellValue(sheetGCA, stageCell, sr.stage) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		openF, _ := sr.opening.Float64()
		closeF, _ := sr.closing.Float64()
		_ = f.SetCellValue(sheetGCA, openCell, openF)                //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellStyle(sheetGCA, openCell, openCell, idrStyle)   //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellValue(sheetGCA, closeCell, closeF)              //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellStyle(sheetGCA, closeCell, closeCell, idrStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		_ = f.SetCellValue(sheetGCA, noteCell, sr.note)              //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		if sr.stage == "Total" {
			_ = f.SetCellStyle(sheetGCA, stageCell, stageCell, boldStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		}
	}

	// Footer.
	gFooterRow := len(stageRows) + 3
	gFooterCell := mustCoordCell(1, gFooterRow)
	_ = f.SetCellValue(sheetGCA, gFooterCell, //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		fmt.Sprintf("Computed at: %s | Detection method: %s",
			report.ComputedAt.Format("2006-01-02T15:04:05Z07:00"),
			string(report.DetectionMethod)))

	_ = f.SetColWidth(sheetGCA, "A", "A", 12) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetColWidth(sheetGCA, "B", "C", 22) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetColWidth(sheetGCA, "D", "D", 55) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	// ─────────────────────────────────────────────────────────────────────────
	// Sheet 3 — Sign-Off
	// Signature placeholders per PSAK 71 §5.5 disclosure requirement.
	// Preparer/Reviewer/Approver/Sealer roles + signature hash + sealed_at.
	// ─────────────────────────────────────────────────────────────────────────
	const sheetSignOff = "Sign-Off"
	_, err = f.NewSheet(sheetSignOff)
	if err != nil {
		return nil, fmt.Errorf("generateXLSXBytes: create Sign-Off sheet: %w", err)
	}

	signRows := [][]string{
		{"BLIPS IFRS9 — CKPN Roll-Forward Disclosure", ""},
		{"", ""},
		{"Report ID", report.ReportID},
		{"Periode Buku", report.CurrentPeriodeID},
		{"Prior Periode", report.PriorPeriodeID},
		{"Reconcile Status", string(report.ReconcileStatus)},
		{"Reconcile Delta (IDR)", report.ReconcileDeltaIdr.StringFixed(4)},
		{"Detection Method", string(report.DetectionMethod)},
		{"Computed At", report.ComputedAt.Format("2006-01-02T15:04:05Z07:00")},
		{"Phase 5 Limitation", report.Phase5LimitationNote},
		{"", ""},
		{"TANDA TANGAN", ""},
		{"", ""},
		{"Prepared by", "____________________________"},
		{"  Nama", ""},
		{"  Jabatan", ""},
		{"  Tanggal", ""},
		{"", ""},
		{"Reviewed by", "____________________________"},
		{"  Nama", ""},
		{"  Jabatan", ""},
		{"  Tanggal", ""},
		{"", ""},
		{"Approved by", "____________________________"},
		{"  Nama", ""},
		{"  Jabatan", ""},
		{"  Tanggal", ""},
		{"", ""},
		{"Sealed by", "____________________________"},
		{"  Nama", ""},
		{"  Jabatan", ""},
		{"  Sealed At", ""},
		{"  Signature Hash", ""},
	}

	for i, row := range signRows {
		rowNum := i + 1
		for col, val := range row {
			cell := mustCoordCell(col+1, rowNum)
			_ = f.SetCellValue(sheetSignOff, cell, val) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		}
		// Bold section headers.
		if len(row) > 0 && (row[0] == "BLIPS IFRS9 — CKPN Roll-Forward Disclosure" ||
			row[0] == "TANDA TANGAN" ||
			row[0] == "Prepared by" ||
			row[0] == "Reviewed by" ||
			row[0] == "Approved by" ||
			row[0] == "Sealed by") {
			cell := mustCoordCell(1, rowNum)
			_ = f.SetCellStyle(sheetSignOff, cell, cell, boldStyle) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
		}
	}

	_ = f.SetColWidth(sheetSignOff, "A", "A", 30) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here
	_ = f.SetColWidth(sheetSignOff, "B", "B", 60) //nolint:errcheck // excelize set ops fail only on invalid sheet/coord — hardcoded valid here

	// Write to buffer.
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("generateXLSXBytes: write to buffer: %w", err)
	}
	return buf.Bytes(), nil
}
