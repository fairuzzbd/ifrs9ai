package rollforward_test

// service_mock_test.go — service-level tests using sqlmock.
// These tests exercise ComputeRollForward, GetCKPNTrend, GetPortfolioRollForward,
// ExportXLSX, and GetRollForward with mocked DB queries.
//
// Tests verify:
//  - First period (prior=nil): opening=0, all originations, RECONCILED
//  - Normal period: transfers, originations, derecognitions, remeasurements, RECONCILED
//  - Current DRAFT status: CodeRollForwardCurrentInvalidState
//  - Current not found: CodeRollForwardCurrentInvalidState
//  - Prior not found: CodeRollForwardPriorNotFound
//  - Prior not SEALED + AllowNonSealedPrior=true: WarnPriorNotSealedPreview
//  - Prior not SEALED + AllowNonSealedPrior=false: CodeRollForwardPriorNotSealed
//  - Same periode: CodeRollForwardPeriodeMismatch
//  - Portfolio not found: CodeRollForwardPortfolioNotFound
//  - CKPN trend < 2 SEALED runs: CodeRollForwardTrendInsufficientData
//  - CKPN trend ≥ 2 SEALED runs: delta computed
//  - ExportXLSX reconciled: returns bytes
//  - ExportXLSX MISMATCH + forceMismatch=false: CodeRollForwardExportMismatchForbidden

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

// buildServiceWithMock creates a Service backed by a sqlmock DB.
func buildServiceWithMock(t *testing.T) (*rollforward.Service, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck

	repo := rollforward.NewRepo(db)
	auditWriter := audit.NewWriter(db)
	svc := rollforward.NewService(repo, db, auditWriter, slog.Default())
	return svc, mock
}

// expectCalcRunStatus adds a mock expectation for GetCalcRunStatus.
func expectCalcRunStatus(mock sqlmock.Sqlmock, id uuid.UUID, status, periodeID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, periode_id FROM ecl.calc_run`)).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}).AddRow(status, periodeID))
}

// expectCalcRunNotFound adds a mock expectation for GetCalcRunStatus that returns no rows.
func expectCalcRunNotFound(mock sqlmock.Sqlmock, id uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status, periode_id FROM ecl.calc_run`)).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}))
}

// expectResultLines adds a mock expectation for GetResultLinesByCalcRun.
func expectResultLines(mock sqlmock.Sqlmock, calcRunID uuid.UUID, lines []rollforward.ResultLineHeader) {
	rows := sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"})
	for _, l := range lines {
		var eclVal interface{}
		if l.EclWeightedIdr != nil {
			eclVal = l.EclWeightedIdr.StringFixed(4)
		}
		rows.AddRow(l.InstrumenID, l.Stage, eclVal, l.EadIdr.StringFixed(4))
	}
	mock.ExpectQuery(`SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr`).
		WithArgs(calcRunID).
		WillReturnRows(rows)
}

// expectStageHistory adds a mock expectation for GetStageHistoryForCalcRun.
func expectStageHistory(mock sqlmock.Sqlmock, calcRunID uuid.UUID) {
	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "calc_run_id", "trigger_type", "created_at"}))
}

// expectAuditTx adds mock expectations for the audit transaction opened by writeAuditEvent.
// It accepts and rolls back or commits.
func expectAuditTxCommit(mock sqlmock.Sqlmock, numEvents int) {
	mock.ExpectBegin()
	for range numEvents {
		mock.ExpectQuery(`SELECT current_hash`).WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
		mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectCommit()
}

// ─── ComputeRollForward — first period ───────────────────────────────────────

func TestComputeRollForward_FirstPeriod_AllOriginations(t *testing.T) {
	svc, mock := buildServiceWithMock(t)

	currentRunID := uuid.New()
	actorID := uuid.New()
	instrID := uuid.New()

	// Step 1: GetCalcRunStatus — COMPLETED
	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JANUARI-2026")

	// Step 2: GetResultLinesByCalcRun — one instrument
	currentLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "5000000.0000"}})
	expectResultLines(mock, currentRunID, currentLines)

	// Step 9: audit tx
	expectAuditTxCommit(mock, 1)

	report, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   nil, // first period
		ActorID:          actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.OpeningEclIdr.IsZero() {
		t.Errorf("opening should be 0, got %s", report.OpeningEclIdr)
	}
	if report.NewOriginations.Count != 1 {
		t.Errorf("originations.Count: want 1, got %d", report.NewOriginations.Count)
	}
	expected := report.NewOriginations.EclIdr
	if !report.ClosingEclIdr.Equal(expected) {
		t.Errorf("closing should equal originations.EclIdr: got %s vs %s", report.ClosingEclIdr, expected)
	}
	if report.ReconcileStatus != rollforward.ReconcileStatusReconciled {
		t.Errorf("want RECONCILED, got %s", report.ReconcileStatus)
	}
	hasFirstPeriodWarn := false
	for _, w := range report.Warnings {
		if w == rollforward.WarnFirstPeriodOpeningZero {
			hasFirstPeriodWarn = true
		}
	}
	if !hasFirstPeriodWarn {
		t.Errorf("expected WarnFirstPeriodOpeningZero in warnings, got: %v", report.Warnings)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// ─── ComputeRollForward — current not found ───────────────────────────────────

func TestComputeRollForward_CurrentNotFound_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID := uuid.New()

	expectCalcRunNotFound(mock, currentRunID)

	_, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T: %v", err, err)
	}
	if de.Code() != rollforward.CodeRollForwardCurrentInvalidState {
		t.Errorf("want %s, got %s", rollforward.CodeRollForwardCurrentInvalidState, de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ComputeRollForward — current DRAFT → 422 ────────────────────────────────

func TestComputeRollForward_CurrentDraft_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "DRAFT", "JUNI-2026")

	_, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
	})

	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardCurrentInvalidState {
		t.Errorf("want ROLL_FORWARD_CURRENT_INVALID_STATE, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ComputeRollForward — prior not found ─────────────────────────────────────

func TestComputeRollForward_PriorNotFound_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID := uuid.New()
	priorRunID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "SEALED", "JUNI-2026")
	expectCalcRunNotFound(mock, priorRunID)

	_, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   &priorRunID,
	})

	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardPriorNotFound {
		t.Errorf("want ROLL_FORWARD_PRIOR_NOT_FOUND, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ComputeRollForward — prior not SEALED, AllowNonSealedPrior=false ─────────

func TestComputeRollForward_PriorNotSealed_ForbidsByDefault(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID, priorRunID := uuid.New(), uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	expectCalcRunStatus(mock, priorRunID, "COMPLETED", "MEI-2026") // not SEALED

	_, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID:    currentRunID,
		PriorCalcRunID:      &priorRunID,
		AllowNonSealedPrior: false,
	})

	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardPriorNotSealed {
		t.Errorf("want ROLL_FORWARD_PRIOR_NOT_SEALED, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ComputeRollForward — prior not SEALED but AllowNonSealedPrior=true ──────

func TestComputeRollForward_PriorNotSealed_AllowedWithWarning(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID, priorRunID := uuid.New(), uuid.New()
	instrID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	expectCalcRunStatus(mock, priorRunID, "COMPLETED", "MEI-2026") // not SEALED

	// Load prior + current lines
	lines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "1000000.0000"}})
	expectResultLines(mock, priorRunID, lines)
	expectResultLines(mock, currentRunID, lines)
	expectStageHistory(mock, currentRunID)
	// No derecognitions — empty setDifference
	// No instrumen status lookup needed

	expectAuditTxCommit(mock, 1)

	report, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID:    currentRunID,
		PriorCalcRunID:      &priorRunID,
		AllowNonSealedPrior: true,
		ActorID:             uuid.New(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hasWarn := false
	for _, w := range report.Warnings {
		if w == rollforward.WarnPriorNotSealedPreview {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected WarnPriorNotSealedPreview in warnings: %v", report.Warnings)
	}
}

// ─── ComputeRollForward — same periode ───────────────────────────────────────

func TestComputeRollForward_SamePeriode_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID, priorRunID := uuid.New(), uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	expectCalcRunStatus(mock, priorRunID, "SEALED", "JUNI-2026") // SAME periode!

	_, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   &priorRunID,
	})

	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardPeriodeMismatch {
		t.Errorf("want ROLL_FORWARD_PERIODE_MISMATCH, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ComputeRollForward — normal period with transfers ────────────────────────

func TestComputeRollForward_NormalPeriod_Transfers_Reconciled(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID, priorRunID := uuid.New(), uuid.New()
	actorID := uuid.New()

	// Instrument that stays in Stage 1 (remeasurement only)
	sameID := uuid.New()
	// Instrument that moves Stage 1→2
	upgradeID := uuid.New()
	// Instrument that is derecognized (in prior, not in current)
	derecID := uuid.New()
	// New instrument (origination)
	newID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")
	expectCalcRunStatus(mock, priorRunID, "SEALED", "MEI-2026")

	priorLines := buildLines([]lineSpec{
		{id: sameID, stage: 1, ecl: "500000.0000"},
		{id: upgradeID, stage: 1, ecl: "200000.0000"},
		{id: derecID, stage: 1, ecl: "100000.0000"},
	})
	currentLines := buildLines([]lineSpec{
		{id: sameID, stage: 1, ecl: "600000.0000"},    // same stage, ECL increased (remeasurement)
		{id: upgradeID, stage: 2, ecl: "1000000.0000"}, // Stage 1→2 transfer
		{id: newID, stage: 1, ecl: "300000.0000"},       // origination
		// derecID absent → derecognition
	})

	expectResultLines(mock, priorRunID, priorLines)
	expectResultLines(mock, currentRunID, currentLines)

	// Stage history: upgradeID has SICR_RATING trigger
	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WithArgs(currentRunID).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "calc_run_id", "trigger_type", "created_at"}).
			AddRow(upgradeID, currentRunID, "SICR_RATING", time.Now()))

	// Derecognition status lookup (derecID)
	mock.ExpectQuery(`SELECT id, kode, status, tanggal_jatuh_tempo`).
		WithArgs(derecID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kode", "status", "tanggal_jatuh_tempo"}).
			AddRow(derecID, "INST-DEC", "JATUH_TEMPO", nil))

	expectAuditTxCommit(mock, 1)

	report, err := svc.ComputeRollForward(context.Background(), rollforward.ComputeRequest{
		CurrentCalcRunID: currentRunID,
		PriorCalcRunID:   &priorRunID,
		ActorID:          actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify transfer bucket
	if report.Transfers.Stage1To2.Count != 1 {
		t.Errorf("Stage1To2.Count: want 1, got %d", report.Transfers.Stage1To2.Count)
	}

	// Verify origination
	if report.NewOriginations.Count != 1 {
		t.Errorf("originations.Count: want 1, got %d", report.NewOriginations.Count)
	}

	// Verify derecognition
	if report.Derecognitions.Count != 1 {
		t.Errorf("derecognitions.Count: want 1, got %d", report.Derecognitions.Count)
	}

	// Reconcile check
	if report.ReconcileStatus != rollforward.ReconcileStatusReconciled {
		t.Errorf("want RECONCILED, got %s (delta=%s)", report.ReconcileStatus, report.ReconcileDeltaIdr)
	}

	// Delta must be < IDR 1.0000
	if report.ReconcileDeltaIdr.Abs().GreaterThanOrEqual(rollforward.ReconcileTolerance) {
		t.Errorf("reconcile delta %s ≥ tolerance", report.ReconcileDeltaIdr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// ─── GetCKPNTrend — insufficient data ────────────────────────────────────────

func TestGetCKPNTrend_InsufficientData_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)

	// Only 1 SEALED run — need ≥ 2
	id := uuid.New()
	now := time.Now()
	mock.ExpectQuery(`SELECT id, periode_id, status, sealed_at, tenant_id`).
		WithArgs(12).
		WillReturnRows(sqlmock.NewRows([]string{"id", "periode_id", "status", "sealed_at", "tenant_id"}).
			AddRow(id, "JUNI-2026", "SEALED", now, "TUGURE"))

	_, err := svc.GetCKPNTrend(context.Background(), 12)
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardTrendInsufficientData {
		t.Errorf("want ROLL_FORWARD_TREND_INSUFFICIENT_DATA, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetCKPNTrend — normal data ───────────────────────────────────────────────

func TestGetCKPNTrend_TwoRuns_DeltaComputed(t *testing.T) {
	svc, mock := buildServiceWithMock(t)

	id1, id2 := uuid.New(), uuid.New()
	now := time.Now()

	// GetSealedCalcRunsByPeriode
	mock.ExpectQuery(`SELECT id, periode_id, status, sealed_at, tenant_id`).
		WithArgs(12).
		WillReturnRows(sqlmock.NewRows([]string{"id", "periode_id", "status", "sealed_at", "tenant_id"}).
			AddRow(id1, "MEI-2026", "SEALED", now.AddDate(0, -1, 0), "TUGURE").
			AddRow(id2, "JUNI-2026", "SEALED", now, "TUGURE"))

	// GetECLByStageForCalcRun for id1
	mock.ExpectQuery(`SELECT stage, COALESCE`).
		WithArgs(id1).
		WillReturnRows(sqlmock.NewRows([]string{"stage", "coalesce"}).
			AddRow(1, "10000000.0000").
			AddRow(2, "5000000.0000"))

	// GetECLByStageForCalcRun for id2
	mock.ExpectQuery(`SELECT stage, COALESCE`).
		WithArgs(id2).
		WillReturnRows(sqlmock.NewRows([]string{"stage", "coalesce"}).
			AddRow(1, "12000000.0000").
			AddRow(2, "4000000.0000").
			AddRow(3, "1000000.0000"))

	points, err := svc.GetCKPNTrend(context.Background(), 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("want 2 points, got %d", len(points))
	}

	// First point: no delta
	if points[0].DeltaFromPrev != nil {
		t.Errorf("first point should have no delta")
	}

	// Second point: delta = 17000000 - 15000000 = 2000000
	if points[1].DeltaFromPrev == nil {
		t.Fatal("second point should have delta")
	}
	if points[1].DeltaPct == nil {
		t.Fatal("second point should have delta pct")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// ─── GetPortfolioRollForward — not found ─────────────────────────────────────

func TestGetPortfolioRollForward_NotFound_ReturnsError(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	portID := uuid.New()

	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}))

	_, err := svc.GetPortfolioRollForward(context.Background(), portID, uuid.New(), nil, uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardPortfolioNotFound {
		t.Errorf("want ROLL_FORWARD_PORTFOLIO_NOT_FOUND, got %s", de.Code())
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetPortfolioRollForward — first period (no prior) ───────────────────────

func TestGetPortfolioRollForward_FirstPeriod(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	portID, currentRunID := uuid.New(), uuid.New()
	instrID := uuid.New()

	// GetPortofolioNama
	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}).AddRow("Test Portfolio"))

	// GetCalcRunStatus
	expectCalcRunStatus(mock, currentRunID, "COMPLETED", "JUNI-2026")

	// GetResultLinesByCalcRunAndPortfolio (current)
	mock.ExpectQuery(`FROM ecl.calc_result_line crl`).
		WithArgs(currentRunID, portID).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"}).
			AddRow(instrID, 1, "1000000.0000", "10000000.0000"))

	// GetStageHistoryForCalcRun
	expectStageHistory(mock, currentRunID)

	result, err := svc.GetPortfolioRollForward(context.Background(), portID, currentRunID, nil, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PortofolioNama != "Test Portfolio" {
		t.Errorf("want 'Test Portfolio', got %s", result.PortofolioNama)
	}
	if result.InstrumentCount != 1 {
		t.Errorf("want 1 instrument, got %d", result.InstrumentCount)
	}
	if result.OpeningEclIdr.IsPositive() {
		t.Errorf("first period opening should be 0")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── ExportXLSX — reconciled ──────────────────────────────────────────────────

func TestExportXLSX_Reconciled_ReturnsBytes(t *testing.T) {
	svc, _ := buildServiceWithMock(t)

	report := &rollforward.RollForwardReport{
		ReportID:         "rf-test-run",
		ReconcileStatus:  rollforward.ReconcileStatusReconciled,
		CurrentPeriodeID: "JUNI-2026",
	}

	bytes, err := svc.ExportXLSX(context.Background(), report, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bytes) == 0 {
		t.Error("expected non-empty bytes for RECONCILED export")
	}
}

// ─── ExportXLSX — MISMATCH blocked ────────────────────────────────────────────

func TestExportXLSX_Mismatch_BlockedWithoutForce(t *testing.T) {
	svc, _ := buildServiceWithMock(t)

	report := &rollforward.RollForwardReport{
		ReportID:          "rf-mismatch",
		ReconcileStatus:   rollforward.ReconcileStatusMismatch,
		ReconcileDeltaIdr: rollforward.ReconcileTolerance.Mul(rollforward.ReconcileTolerance), // > 1.0000
		CurrentPeriodeID:  "JUNI-2026",
	}

	_, err := svc.ExportXLSX(context.Background(), report, false)
	if err == nil {
		t.Fatal("expected error for MISMATCH")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardExportMismatchForbidden {
		t.Errorf("want ROLL_FORWARD_EXPORT_MISMATCH_FORBIDDEN, got %s", de.Code())
	}
}

// ─── ExportXLSX — MISMATCH with force ────────────────────────────────────────

func TestExportXLSX_Mismatch_AllowedWithForce(t *testing.T) {
	svc, _ := buildServiceWithMock(t)

	report := &rollforward.RollForwardReport{
		ReportID:          "rf-mismatch-force",
		ReconcileStatus:   rollforward.ReconcileStatusMismatch,
		ReconcileDeltaIdr: rollforward.ReconcileTolerance,
		CurrentPeriodeID:  "JUNI-2026",
	}

	bytes, err := svc.ExportXLSX(context.Background(), report, true)
	if err != nil {
		t.Fatalf("unexpected error with forceMismatch=true: %v", err)
	}
	if len(bytes) == 0 {
		t.Error("expected non-empty bytes")
	}
}

// ─── GetRollForward aliases ComputeRollForward ────────────────────────────────

func TestGetRollForward_FirstPeriod(t *testing.T) {
	svc, mock := buildServiceWithMock(t)
	currentRunID := uuid.New()
	instrID := uuid.New()

	expectCalcRunStatus(mock, currentRunID, "SEALED", "JUNI-2026")
	currentLines := buildLines([]lineSpec{{id: instrID, stage: 1, ecl: "2000000.0000"}})
	expectResultLines(mock, currentRunID, currentLines)
	expectAuditTxCommit(mock, 1)

	report, err := svc.GetRollForward(context.Background(), currentRunID, nil, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ReconcileStatus != rollforward.ReconcileStatusReconciled {
		t.Errorf("want RECONCILED, got %s", report.ReconcileStatus)
	}
}
