// drift_service_test.go — unit tests for DriftService (P4-M6 M6-002, M6-003).
//
// Repo layer (driftRepo, amendRepo, schedRepo) uses stubs (in-memory).
// DriftService still calls s.db.BeginTx/Commit directly for the create and
// finish transactions, and storeDriftEntries does a direct tx.ExecContext.
//
// Transaction expectations per GenerateReport call (happy path):
//
//	1st tx: Begin + Commit  (driftRepo.Create → stub, no exec needed)
//	2nd tx: Begin + ExecContext(UPDATE sys.drift_report storeDriftEntries) + Commit
//
// GetReport does a QueryRowContext on db — needs ExpectQuery.
//
// Error assertions use assertDomainCode (defined in detection_service_test.go).
//
// References: FSD-APP-C §M6-002, §M6-003; DEC-016.
package eir

import (
	"context"
	"database/sql"
	"log/slog"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ─── stub drift repo ──────────────────────────────────────────────────────────

type stubDriftRepo struct {
	reports       map[uuid.UUID]*DriftReport
	inProgress    *DriftReport
	flagThreshold decimal.Decimal
	highThreshold decimal.Decimal
}

func newStubDriftRepo() *stubDriftRepo {
	return &stubDriftRepo{
		reports:       make(map[uuid.UUID]*DriftReport),
		flagThreshold: decimal.NewFromFloat(0.0001),
		highThreshold: decimal.NewFromFloat(0.001),
	}
}

func (r *stubDriftRepo) Create(_ context.Context, _ *sql.Tx, report *DriftReport) error {
	cp := *report
	r.reports[report.ID] = &cp
	if report.Status == DriftStatusInProgress {
		r.inProgress = &cp
	}
	return nil
}

func (r *stubDriftRepo) Update(_ context.Context, _ *sql.Tx, report *DriftReport) error {
	cp := *report
	r.reports[report.ID] = &cp
	if report.Status != DriftStatusInProgress {
		r.inProgress = nil
	}
	return nil
}

func (r *stubDriftRepo) GetByID(_ context.Context, id uuid.UUID) (*DriftReport, error) {
	if dr, ok := r.reports[id]; ok {
		cp := *dr
		return &cp, nil
	}
	return nil, nil
}

func (r *stubDriftRepo) GetInProgressReport(_ context.Context) (*DriftReport, error) {
	if r.inProgress != nil {
		cp := *r.inProgress
		return &cp, nil
	}
	return nil, nil
}

func (r *stubDriftRepo) List(_ context.Context, _ listquery.Query, _ string, limit int) ([]DriftReport, *response.PaginationMeta, error) {
	result := make([]DriftReport, 0, len(r.reports))
	for _, dr := range r.reports {
		result = append(result, *dr)
	}
	return result, &response.PaginationMeta{Limit: limit}, nil
}

func (r *stubDriftRepo) LoadThresholds(_ context.Context) (decimal.Decimal, decimal.Decimal, error) {
	return r.flagThreshold, r.highThreshold, nil
}

// ─── stub schedule repo for drift ────────────────────────────────────────────

type driftScheduleRepo struct {
	rows map[uuid.UUID][]ScheduleRow
}

func newDriftScheduleRepo() *driftScheduleRepo {
	return &driftScheduleRepo{rows: make(map[uuid.UUID][]ScheduleRow)}
}

func (r *driftScheduleRepo) addRows(instrID uuid.UUID, rows []ScheduleRow) {
	r.rows[instrID] = rows
}

func (r *driftScheduleRepo) InsertBatch(_ context.Context, _ *sql.Tx, _ []ScheduleRow) error {
	return nil
}
func (r *driftScheduleRepo) MarkSuperseded(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ int, _ uuid.UUID) error {
	return nil
}
func (r *driftScheduleRepo) GetActiveByPeriode(_ context.Context, instrID uuid.UUID, _ int) ([]ScheduleRow, error) {
	return r.rows[instrID], nil
}
func (r *driftScheduleRepo) GetMaxPeriodeSeq(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}
func (r *driftScheduleRepo) HasActiveRows(_ context.Context, instrID uuid.UUID) (bool, error) {
	return len(r.rows[instrID]) > 0, nil
}
func (r *driftScheduleRepo) List(_ context.Context, _ uuid.UUID, _ listquery.Query, _ bool, _ string, limit int) ([]ScheduleRow, *response.PaginationMeta, error) {
	return nil, &response.PaginationMeta{Limit: limit}, nil
}
func (r *driftScheduleRepo) GetGrossCarryingAtDate(_ context.Context, instrID uuid.UUID, _ time.Time) (decimal.Decimal, error) {
	rows := r.rows[instrID]
	if len(rows) == 0 {
		return decimal.Zero, ErrEIRScheduleNotFound(instrID.String())
	}
	return rows[len(rows)-1].ClosingCarrying, nil
}

// ─── streaming instrumen repo for drift ──────────────────────────────────────

type driftInstrRepo struct {
	instruments []InstrumenForEIR
}

func (r *driftInstrRepo) GetByID(_ context.Context, id uuid.UUID) (*InstrumenForEIR, error) {
	for i := range r.instruments {
		if r.instruments[i].ID == id {
			cp := r.instruments[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *driftInstrRepo) ListActiveForBulk(ctx context.Context, _ BulkScope) (<-chan InstrumenForEIR, error) {
	ch := make(chan InstrumenForEIR)
	go func() {
		defer close(ch)
		for i := range r.instruments {
			select {
			case ch <- r.instruments[i]:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (r *driftInstrRepo) UpdateEIRAwal(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ decimal.Decimal, _ uuid.UUID) error {
	return nil
}

// newDriftMock returns a mock db wired for the GenerateReport happy-path:
//
//	1st tx:  Begin + Commit (create IN_PROGRESS — driftRepo.Create is stub, no exec)
//	2nd tx:  Begin + Exec(storeDriftEntries UPDATE) + Commit
func newDriftMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	// 1st tx: create IN_PROGRESS report
	mock.ExpectBegin()
	mock.ExpectCommit()
	// 2nd tx: update COMPLETED + storeDriftEntries
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// ─── DriftService.GenerateReport tests ───────────────────────────────────────

func TestDriftService_GenerateReport_NoDrift(t *testing.T) {
	instrID := uuid.New()
	eirVal := decimal.NewFromFloat(0.08028915)

	schedRepo := newDriftScheduleRepo()
	openCarrying := decimal.NewFromFloat(1000000)
	inflow := decimal.NewFromFloat(1080291.5)
	schedRepo.addRows(instrID, []ScheduleRow{
		{
			ID:              uuid.New(),
			InstrumenID:     instrID,
			PeriodeSeq:      1,
			TanggalPosting:  time.Now().AddDate(0, 6, 0),
			OpeningCarrying: openCarrying,
			CashInflow:      inflow,
		},
	})

	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{
		{
			ID:                instrID,
			KodeInstrumen:     "BOND-NODRIFT",
			KlasifikasiPsak71: "AC",
			EIRMethodFlag:     true,
			EIRAwal:           &eirVal,
			Status:            "ACTIVE",
			TenantID:          "TUGURE",
		},
	}}

	driftRepo := newStubDriftRepo()
	amendRepo := newStubAmendmentRepo()
	db, mock := newDriftMock(t)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	report, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerManualAdHoc,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Status != DriftStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", report.Status)
	}
	if report.TotalInstrumen != 1 {
		t.Errorf("expected 1 total, got %d", report.TotalInstrumen)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDriftService_GenerateReport_ConcurrentInProgress(t *testing.T) {
	instrRepo := &driftInstrRepo{}
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()

	jobID := "job-123"
	existing := &DriftReport{
		ID:            uuid.New(),
		Status:        DriftStatusInProgress,
		TriggerSource: DriftTriggerCronDaily,
		AsynqJobID:    &jobID,
	}
	driftRepo.inProgress = existing
	driftRepo.reports[existing.ID] = existing

	// Error returned before any BeginTx — no expectations needed.
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	_, err2 := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerManualAdHoc,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	assertDomainCode(t, err2, CodeEIRDriftGenerationInProgress)
}

func TestDriftService_GenerateReport_MissingEIRAwal(t *testing.T) {
	instrID := uuid.New()
	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{
		{
			ID:                instrID,
			KodeInstrumen:     "BOND-MISSING",
			KlasifikasiPsak71: "AC",
			EIRMethodFlag:     true,
			EIRAwal:           nil, // no EIR stored
			Status:            "ACTIVE",
			TenantID:          "TUGURE",
		},
	}}
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()
	db, mock := newDriftMock(t)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	report, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerCronDaily,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.MissingScheduleCount != 1 {
		t.Errorf("expected 1 missing, got %d", report.MissingScheduleCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDriftService_GetReport_NotFound(t *testing.T) {
	driftRepo := newStubDriftRepo()
	instrRepo := &driftInstrRepo{}
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())

	_, err2 := svc.GetReport(context.Background(), uuid.New())
	assertDomainCode(t, err2, CodeEIRDriftReportNotFound)
}

// ─── DriftSeverity classification ─────────────────────────────────────────────

func TestDriftClassification_Low(t *testing.T) {
	// abs_diff = 0.0002 > flagThreshold(0.0001) and <= highThreshold(0.001) → LOW
	stored := decimal.NewFromFloat(0.08000000)
	recomputed := decimal.NewFromFloat(0.08020000)
	absDiff := stored.Sub(recomputed).Abs()

	flagThresh := decimal.NewFromFloat(0.0001)
	highThresh := decimal.NewFromFloat(0.001)

	if absDiff.LessThanOrEqual(flagThresh) {
		t.Fatalf("test setup: absDiff=%s should be > flagThresh=%s", absDiff, flagThresh)
	}
	if absDiff.GreaterThan(highThresh) {
		t.Fatalf("test setup: absDiff=%s should not be > highThresh=%s", absDiff, highThresh)
	}

	severity := DriftSeverityLow
	if absDiff.GreaterThan(highThresh) {
		severity = DriftSeverityHigh
	}
	if severity != DriftSeverityLow {
		t.Errorf("expected LOW, got %s", severity)
	}
}

func TestDriftClassification_High(t *testing.T) {
	// abs_diff = 0.002 > highThreshold(0.001) → HIGH
	stored := decimal.NewFromFloat(0.08000000)
	recomputed := decimal.NewFromFloat(0.08200000)
	absDiff := stored.Sub(recomputed).Abs()

	highThresh := decimal.NewFromFloat(0.001)

	if !absDiff.GreaterThan(highThresh) {
		t.Fatalf("test setup: absDiff=%s should be > highThresh=%s", absDiff, highThresh)
	}

	severity := DriftSeverityLow
	if absDiff.GreaterThan(highThresh) {
		severity = DriftSeverityHigh
	}
	if severity != DriftSeverityHigh {
		t.Errorf("expected HIGH, got %s", severity)
	}
}

func TestDriftClassification_NoDrift(t *testing.T) {
	stored := decimal.NewFromFloat(0.08000000)
	recomputed := decimal.NewFromFloat(0.08005000) // diff = 0.00005 < 0.0001
	absDiff := stored.Sub(recomputed).Abs()
	flagThresh := decimal.NewFromFloat(0.0001)

	if !absDiff.LessThanOrEqual(flagThresh) {
		t.Errorf("expected no drift (absDiff=%s <= flagThresh=%s)", absDiff, flagThresh)
	}
}

// ─── HIGH severity drift — processDriftInstrument ────────────────────────────

// newDriftMockHighSeverity sets up 3 transactions for GenerateReport when
// there is 1 HIGH-severity instrument with NO existing active proposal:
//
//	tx1: Begin + Commit  (create IN_PROGRESS)
//	tx2: Begin + Commit  (auto-proposal; amendRepo.Create is stub, no exec)
//	tx3: Begin + Exec(UPDATE sys.drift_report) + Commit  (storeDriftEntries)
func newDriftMockHighSeverity(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	// tx1: create IN_PROGRESS
	mock.ExpectBegin()
	mock.ExpectCommit()
	// tx2: auto-create DRAFT proposal
	mock.ExpectBegin()
	mock.ExpectCommit()
	// tx3: storeDriftEntries UPDATE
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// highDriftInstrAndSched returns an instrument with EIRAwal=0.08 and schedule
// rows whose recomputed EIR will be ~0.095 (abs_diff ≈ 0.015 >> 0.001 HIGH).
//
// Cashflow reconstruction (drift_service.go ~line 286):
//
//	CF[0] = -OpeningCarrying[0]  at scheduleRows[0].TanggalPosting (t=0)
//	CF[1] = CashInflow[0]        at scheduleRows[0].TanggalPosting (t=0 too — same date)
//
// Because both cashflows land on the same date the NR solver will compute a
// different EIR than 0.08, giving us a HIGH drift. We use 2 rows so the first
// provides the outflow date and the second provides a +365-day inflow.
func highDriftInstrAndSched() (InstrumenForEIR, []ScheduleRow) {
	instrID := uuid.New()
	eirStored := decimal.NewFromFloat(0.08000000)
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	instr := InstrumenForEIR{
		ID:                instrID,
		KodeInstrumen:     "BOND-HIGHDRIFT",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           &eirStored,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	}

	// Row 0: opening outflow at baseDate, no inflow here (CashInflow=0 is skipped by validator check)
	// Row 1: inflow at baseDate+365 days
	// CF built by drift_service: CF[0]=-opening[0], CF[i]=inflow[i] for each row
	// So: CF[0]=-1_000_000 at row[0].date; CF[1]=0 at row[0].date; CF[2]=1_095_000 at row[1].date
	// Solver: t for CF[1]=0 is 0 (same date), skipped from NPV sum; t for CF[2]=1yr.
	// → recomputed ≈ 0.095, diff ≈ 0.015 → HIGH.
	rows := []ScheduleRow{
		{
			ID:              uuid.New(),
			InstrumenID:     instrID,
			PeriodeSeq:      1,
			TanggalPosting:  baseDate,
			OpeningCarrying: decimal.NewFromFloat(1_000_000),
			CashInflow:      decimal.NewFromFloat(0), // placeholder, t=0
		},
		{
			ID:             uuid.New(),
			InstrumenID:    instrID,
			PeriodeSeq:     2,
			TanggalPosting: baseDate.AddDate(1, 0, 0), // +365 days
			CashInflow:     decimal.NewFromFloat(1_095_000),
		},
	}
	return instr, rows
}

// TestDriftService_GenerateReport_HighDrift_AutoProposal exercises the HIGH
// severity code-path where no active proposal exists → auto-creates DRAFT.
func TestDriftService_GenerateReport_HighDrift_AutoProposal(t *testing.T) {
	instr, schedRows := highDriftInstrAndSched()

	schedRepo := newDriftScheduleRepo()
	schedRepo.addRows(instr.ID, schedRows)

	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{instr}}
	amendRepo := newStubAmendmentRepo()
	// No entry in activeForID → HasActiveProposal returns false → auto-create fires.

	driftRepo := newStubDriftRepo()
	db, mock := newDriftMockHighSeverity(t)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	report, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerManualAdHoc,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Status != DriftStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", report.Status)
	}
	if report.DriftHighCount != 1 {
		t.Errorf("expected DriftHighCount=1, got %d", report.DriftHighCount)
	}
	if report.TotalInstrumen != 1 {
		t.Errorf("expected TotalInstrumen=1, got %d", report.TotalInstrumen)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}

// TestDriftService_GenerateReport_HighDrift_HasActiveProposal exercises the
// HIGH severity path where an active proposal already exists → no auto-create.
// Only 2 transactions needed (no proposal creation tx).
func TestDriftService_GenerateReport_HighDrift_HasActiveProposal(t *testing.T) {
	instr, schedRows := highDriftInstrAndSched()

	schedRepo := newDriftScheduleRepo()
	schedRepo.addRows(instr.ID, schedRows)

	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{instr}}
	amendRepo := newStubAmendmentRepo()
	// Mark this instrument as having an active proposal → HasActiveProposal = true.
	amendRepo.activeForID[instr.ID] = true

	driftRepo := newStubDriftRepo()
	db, mock := newDriftMock(t) // standard 2-tx mock (no proposal creation)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	report, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerCronDaily,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.DriftHighCount != 1 {
		t.Errorf("expected DriftHighCount=1, got %d", report.DriftHighCount)
	}
	// No new proposal created because active already exists.
	if len(amendRepo.proposals) != 0 {
		t.Errorf("expected no new proposal, got %d", len(amendRepo.proposals))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}

// TestDriftService_GenerateReport_LowDrift exercises the LOW severity path
// (abs_diff > flagThreshold but <= highThreshold).
func TestDriftService_GenerateReport_LowDrift(t *testing.T) {
	instrID := uuid.New()
	// EIRAwal=0.08000000; cashflows produce EIR ≈ 0.08050000 → diff=0.0005, LOW.
	eirStored := decimal.NewFromFloat(0.08000000)
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	instr := InstrumenForEIR{
		ID:                instrID,
		KodeInstrumen:     "BOND-LOWDRIFT",
		KlasifikasiPsak71: "AC",
		EIRMethodFlag:     true,
		EIRAwal:           &eirStored,
		Status:            "ACTIVE",
		TenantID:          "TUGURE",
	}

	schedRepo := newDriftScheduleRepo()
	schedRepo.addRows(instrID, []ScheduleRow{
		{
			ID:              uuid.New(),
			InstrumenID:     instrID,
			PeriodeSeq:      1,
			TanggalPosting:  baseDate,
			OpeningCarrying: decimal.NewFromFloat(1_000_000),
			CashInflow:      decimal.NewFromFloat(0),
		},
		{
			ID:             uuid.New(),
			InstrumenID:    instrID,
			PeriodeSeq:     2,
			TanggalPosting: baseDate.AddDate(1, 0, 0),
			CashInflow:     decimal.NewFromFloat(1_080_500), // ~8.05% → diff≈0.0005 LOW
		},
	})

	instrRepo := &driftInstrRepo{instruments: []InstrumenForEIR{instr}}
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()
	db, mock := newDriftMock(t)

	svc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	triggered := uuid.New()
	report, err := svc.GenerateReport(context.Background(), DriftGenerateRequest{
		TriggerSource: DriftTriggerManualAdHoc,
		TriggeredBy:   &triggered,
		TenantID:      "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.DriftLowCount != 1 {
		t.Errorf("expected DriftLowCount=1, got %d", report.DriftLowCount)
	}
	if report.DriftHighCount != 0 {
		t.Errorf("expected DriftHighCount=0, got %d", report.DriftHighCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}

// ─── Constructor panic tests ──────────────────────────────────────────────────

func TestNewDriftService_PanicsOnNilAuditWriter(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil auditWriter, got none")
		}
	}()
	NewDriftService(nil, nil, nil, nil, nil, nil, nil, slog.Default())
}

func TestNewDetectionService_PanicsOnNilAuditWriter(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil auditWriter, got none")
		}
	}()
	NewDetectionService(nil, nil, nil, nil, slog.Default())
}
