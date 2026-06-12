// repo_m6_test.go — DB repo tests for M6 additions: Cancel, ListQueue,
// DBDriftReportRepo (Create, Update, GetByID, GetInProgressReport, List, LoadThresholds),
// and NewDriftCronHandler/handle worker tests.
//
// Uses go-sqlmock — no live PostgreSQL required.
// DEC-016: NUMERIC columns via ::text, no float64.
// References: repo.go M6 section, worker_tasks.go.
package eir

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── helper: driftReportSelectCols ───────────────────────────────────────────

// driftReportSelectCols returns columns matching scanDriftReport scan order.
func driftReportSelectCols() []string {
	return []string{
		"id", "tanggal_run", "trigger_source", "triggered_by",
		"asynq_job_id", "status",
		"started_at", "completed_at",
		"total_instrumen", "drift_low_count", "drift_high_count",
		"missing_schedule_count", "error_count", "error_summary",
		"drift_flag_threshold", "drift_high_threshold",
		"created_at", "created_by", "updated_at", "updated_by", "tenant_id", "row_version",
	}
}

// addDriftReportRow adds a single DriftReport row to a sqlmock.Rows.
func addDriftReportRow(rows *sqlmock.Rows, id uuid.UUID, status DriftReportStatus) *sqlmock.Rows {
	now := time.Now()
	return rows.AddRow(
		id,                            // id
		now,                           // tanggal_run
		string(DriftTriggerCronDaily), // trigger_source
		nil,                           // triggered_by
		nil,                           // asynq_job_id
		string(status),                // status
		&now,                          // started_at
		nil,                           // completed_at
		5,                             // total_instrumen
		2,                             // drift_low_count
		1,                             // drift_high_count
		0,                             // missing_schedule_count
		0,                             // error_count
		nil,                           // error_summary
		"0.00010000",                  // drift_flag_threshold::text
		"0.00100000",                  // drift_high_threshold::text
		now,                           // created_at
		uuid.New(),                    // created_by
		now,                           // updated_at
		uuid.New(),                    // updated_by
		"TUGURE",                      // tenant_id
		int64(1),                      // row_version
	)
}

// ─── DBAmendmentRepo.Cancel ───────────────────────────────────────────────────

func TestDBAmendmentRepo_Cancel(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	proposalID := uuid.New()
	cancelledBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_reestimation_log")).
		WithArgs("Pembatalan karena perubahan regulasi", cancelledBy, proposalID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	err := repo.Cancel(context.Background(), tx, proposalID, "Pembatalan karena perubahan regulasi", cancelledBy)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBAmendmentRepo.ListQueue ────────────────────────────────────────────────

func TestDBAmendmentRepo_ListQueue_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	queueCols := []string{
		"id", "instrumen_id", "kode_instrumen",
		"workflow_status", "trigger_source",
		"eir_sebelum",
		"maker_id", "reviewer_id",
		"tanggal_re_estimation",
		"created_at",
		"drift_report_id",
		"document_id",
	}
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(queueCols))

	repo := NewDBAmendmentRepo(db)
	rows, meta, err := repo.ListQueue(context.Background(), listquery.Query{}, "", 50)
	if err != nil {
		t.Fatalf("ListQueue: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
	if meta == nil {
		t.Fatal("meta must not be nil")
	}
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
}

func TestDBAmendmentRepo_ListQueue_WithRow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	amendID := uuid.New()
	makerID := uuid.New()
	now := time.Now()
	eirStr := "0.08000000"

	queueCols := []string{
		"id", "instrumen_id", "kode_instrumen",
		"workflow_status", "trigger_source",
		"eir_sebelum",
		"maker_id", "reviewer_id",
		"tanggal_re_estimation",
		"created_at",
		"drift_report_id",
		"document_id",
	}

	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows(queueCols).AddRow(
			amendID, instrID, "BOND-Q001",
			"PENDING_REVIEW", "DOCUMENT_UPLOAD",
			eirStr,
			&makerID, nil, // reviewer_id nil
			now, now,
			nil, nil, // drift_report_id, document_id
		),
	)

	repo := NewDBAmendmentRepo(db)
	rows, meta, err := repo.ListQueue(context.Background(), listquery.Query{}, "", 50)
	if err != nil {
		t.Fatalf("ListQueue with row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AmendmentID != amendID {
		t.Errorf("amendmentID mismatch")
	}
	if rows[0].KodeInstrumen != "BOND-Q001" {
		t.Errorf("kodeInstrumen mismatch: %s", rows[0].KodeInstrumen)
	}
	if rows[0].EIRLama == nil || !rows[0].EIRLama.Equal(decimal.NewFromFloat(0.08)) {
		t.Errorf("eirLama mismatch: %v", rows[0].EIRLama)
	}
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
}

func TestDBAmendmentRepo_ListQueue_NullTriggerSource(t *testing.T) {
	// trigger_source NULL → default to AmendTriggerManual
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	amendID := uuid.New()
	now := time.Now()

	queueCols := []string{
		"id", "instrumen_id", "kode_instrumen",
		"workflow_status", "trigger_source",
		"eir_sebelum",
		"maker_id", "reviewer_id",
		"tanggal_re_estimation",
		"created_at",
		"drift_report_id",
		"document_id",
	}
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows(queueCols).AddRow(
			amendID, instrID, "BOND-001",
			"DRAFT", nil, // trigger_source NULL
			nil,      // eir_sebelum NULL
			nil, nil, // maker_id, reviewer_id
			now, now,
			nil, nil,
		),
	)

	repo := NewDBAmendmentRepo(db)
	rows, _, err := repo.ListQueue(context.Background(), listquery.Query{}, "", 10)
	if err != nil {
		t.Fatalf("ListQueue null trigger: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].TriggerSource != AmendTriggerManual {
		t.Errorf("expected MANUAL default, got %s", rows[0].TriggerSource)
	}
	if rows[0].EIRLama != nil {
		t.Error("expected nil EIRLama")
	}
}

// ─── NewDBDriftReportRepo ─────────────────────────────────────────────────────

func TestNewDBDriftReportRepo_NotNil(t *testing.T) {
	db, _ := newMockDB(t)
	defer db.Close()
	if NewDBDriftReportRepo(db) == nil {
		t.Fatal("expected non-nil")
	}
}

// ─── DBDriftReportRepo.Create ─────────────────────────────────────────────────

func TestDBDriftReportRepo_Create(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	triggered := uuid.New()
	now := time.Now()
	report := &DriftReport{
		ID:                 uuid.New(),
		TanggalRun:         now.Truncate(24 * time.Hour),
		TriggerSource:      DriftTriggerManualAdHoc,
		TriggeredBy:        &triggered,
		Status:             DriftStatusInProgress,
		StartedAt:          &now,
		DriftFlagThreshold: decimal.NewFromFloat(0.0001),
		DriftHighThreshold: decimal.NewFromFloat(0.001),
		CreatedBy:          triggered,
		UpdatedBy:          triggered,
		TenantID:           "TUGURE",
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBDriftReportRepo(db)
	if err := repo.Create(context.Background(), tx, report); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBDriftReportRepo_Create_NilTriggeredBy(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	now := time.Now()
	report := &DriftReport{
		ID:                 uuid.New(),
		TanggalRun:         now,
		TriggerSource:      DriftTriggerCronDaily,
		TriggeredBy:        nil, // covers nil branch
		Status:             DriftStatusInProgress,
		StartedAt:          &now,
		DriftFlagThreshold: decimal.NewFromFloat(0.0001),
		DriftHighThreshold: decimal.NewFromFloat(0.001),
		TenantID:           "TUGURE",
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBDriftReportRepo(db)
	if err := repo.Create(context.Background(), tx, report); err != nil {
		t.Fatalf("Create nil triggered: %v", err)
	}
	tx.Commit()
}

// ─── DBDriftReportRepo.Update ─────────────────────────────────────────────────

func TestDBDriftReportRepo_Update(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	now := time.Now()
	summary := "2 errors"
	updater := uuid.New()
	report := &DriftReport{
		ID:                   uuid.New(),
		Status:               DriftStatusCompleted,
		CompletedAt:          &now,
		TotalInstrumen:       10,
		DriftLowCount:        2,
		DriftHighCount:       1,
		MissingScheduleCount: 0,
		ErrorCount:           2,
		ErrorSummary:         &summary,
		UpdatedBy:            updater,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBDriftReportRepo(db)
	if err := repo.Update(context.Background(), tx, report); err != nil {
		t.Fatalf("Update: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBDriftReportRepo.GetByID ────────────────────────────────────────────────

func TestDBDriftReportRepo_GetByID_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(driftReportSelectCols()))

	repo := NewDBDriftReportRepo(db)
	dr, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if dr != nil {
		t.Error("expected nil for not found")
	}
}

func TestDBDriftReportRepo_GetByID_Found(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	id := uuid.New()
	rows := addDriftReportRow(sqlmock.NewRows(driftReportSelectCols()), id, DriftStatusCompleted)

	mock.ExpectQuery("SELECT").
		WithArgs(id).
		WillReturnRows(rows)

	repo := NewDBDriftReportRepo(db)
	dr, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID found: %v", err)
	}
	if dr == nil {
		t.Fatal("expected report, got nil")
	}
	if dr.ID != id {
		t.Errorf("id mismatch")
	}
	if dr.Status != DriftStatusCompleted {
		t.Errorf("status: got %s", dr.Status)
	}
	if !dr.DriftFlagThreshold.Equal(decimal.NewFromFloat(0.0001)) {
		t.Errorf("flag threshold mismatch: %s", dr.DriftFlagThreshold)
	}
}

// ─── DBDriftReportRepo.GetInProgressReport ───────────────────────────────────

func TestDBDriftReportRepo_GetInProgressReport_None(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(driftReportSelectCols()))

	repo := NewDBDriftReportRepo(db)
	dr, err := repo.GetInProgressReport(context.Background())
	if err != nil {
		t.Fatalf("GetInProgress none: %v", err)
	}
	if dr != nil {
		t.Error("expected nil when no in-progress reports")
	}
}

func TestDBDriftReportRepo_GetInProgressReport_Found(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	id := uuid.New()
	rows := addDriftReportRow(sqlmock.NewRows(driftReportSelectCols()), id, DriftStatusInProgress)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDriftReportRepo(db)
	dr, err := repo.GetInProgressReport(context.Background())
	if err != nil {
		t.Fatalf("GetInProgress found: %v", err)
	}
	if dr == nil {
		t.Fatal("expected report, got nil")
	}
	if dr.Status != DriftStatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", dr.Status)
	}
}

// ─── DBDriftReportRepo.List ───────────────────────────────────────────────────

func TestDBDriftReportRepo_List_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(driftReportSelectCols()))

	repo := NewDBDriftReportRepo(db)
	reports, meta, err := repo.List(context.Background(), listquery.Query{}, "", 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0, got %d", len(reports))
	}
	if meta == nil {
		t.Fatal("meta must not be nil")
	}
}

func TestDBDriftReportRepo_List_WithRow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	id := uuid.New()
	rows := addDriftReportRow(sqlmock.NewRows(driftReportSelectCols()), id, DriftStatusCompleted)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDriftReportRepo(db)
	reports, meta, err := repo.List(context.Background(), listquery.Query{}, "", 50)
	if err != nil {
		t.Fatalf("List with row: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].ID != id {
		t.Errorf("id mismatch")
	}
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
}

func TestDBDriftReportRepo_List_LimitClamped(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(driftReportSelectCols()))

	repo := NewDBDriftReportRepo(db)
	_, meta, err := repo.List(context.Background(), listquery.Query{}, "", 9999)
	if err != nil {
		t.Fatalf("List clamped: %v", err)
	}
	if meta.Limit != 50 {
		t.Errorf("expected limit=50 for 9999, got %d", meta.Limit)
	}
}

// ─── DBDriftReportRepo.LoadThresholds ────────────────────────────────────────

func TestDBDriftReportRepo_LoadThresholds_Success(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM sys.parameter")).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("drift_low_threshold", "0.0001").
			AddRow("drift_high_threshold", "0.001"))

	repo := NewDBDriftReportRepo(db)
	low, high, err := repo.LoadThresholds(context.Background())
	if err != nil {
		t.Fatalf("LoadThresholds: %v", err)
	}
	if !low.Equal(decimal.NewFromFloat(0.0001)) {
		t.Errorf("low mismatch: %s", low)
	}
	if !high.Equal(decimal.NewFromFloat(0.001)) {
		t.Errorf("high mismatch: %s", high)
	}
}

func TestDBDriftReportRepo_LoadThresholds_MissingKeys(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	// Only one key returned — missing the other.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM sys.parameter")).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("drift_low_threshold", "0.0001"))

	repo := NewDBDriftReportRepo(db)
	_, _, err := repo.LoadThresholds(context.Background())
	assertDomainCode(t, err, CodeEIRDriftThresholdInvalid)
}

func TestDBDriftReportRepo_LoadThresholds_InvalidLow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM sys.parameter")).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("drift_low_threshold", "0"). // zero → invalid
			AddRow("drift_high_threshold", "0.001"))

	repo := NewDBDriftReportRepo(db)
	_, _, err := repo.LoadThresholds(context.Background())
	assertDomainCode(t, err, CodeEIRDriftThresholdInvalid)
}

func TestDBDriftReportRepo_LoadThresholds_HighNotGreaterThanLow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value FROM sys.parameter")).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("drift_low_threshold", "0.001").
			AddRow("drift_high_threshold", "0.0005")) // high < low → invalid

	repo := NewDBDriftReportRepo(db)
	_, _, err := repo.LoadThresholds(context.Background())
	assertDomainCode(t, err, CodeEIRDriftThresholdInvalid)
}

// ─── Worker handler constructor + handle path ─────────────────────────────────

func TestNewDriftCronHandler_NotNil(t *testing.T) {
	instrRepo := newStubInstrumenRepo()
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()
	db, _, _ := sqlmock.New()
	defer db.Close()

	driftSvc := NewDriftService(db, &driftInstrRepo{}, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())
	h := NewDriftCronHandler(driftSvc, slog.Default())
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	_ = instrRepo
}

func TestDriftCronHandler_HandleDriftCronTask_Success(t *testing.T) {
	// HandleDriftCronTask calls handle() which calls driftSvc.GenerateReport.
	// driftSvc uses stub repos. Set up minimal mock DB expectations.
	db, mock, _ := sqlmock.New()
	defer db.Close()
	// GenerateReport: 1st tx create, 2nd tx finish+storeDriftEntries
	mock.ExpectBegin()
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	instrRepo := &driftInstrRepo{} // empty — no instruments to process
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()
	driftSvc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())

	h := NewDriftCronHandler(driftSvc, slog.Default())

	task, err := NewDriftCronTask("TUGURE")
	if err != nil {
		t.Fatalf("NewDriftCronTask: %v", err)
	}

	if err := h.HandleDriftCronTask(context.Background(), task); err != nil {
		t.Fatalf("HandleDriftCronTask: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDriftCronHandler_HandleDriftAdHocTask_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE sys.drift_report")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	instrRepo := &driftInstrRepo{}
	schedRepo := newDriftScheduleRepo()
	amendRepo := newStubAmendmentRepo()
	driftRepo := newStubDriftRepo()
	driftSvc := NewDriftService(db, instrRepo, schedRepo, amendRepo, driftRepo, NewSolver(), stubAuditW(), slog.Default())

	h := NewDriftCronHandler(driftSvc, slog.Default())

	task, err := NewDriftAdHocTask("TUGURE", uuid.New())
	if err != nil {
		t.Fatalf("NewDriftAdHocTask: %v", err)
	}

	if err := h.HandleDriftAdHocTask(context.Background(), task); err != nil {
		t.Fatalf("HandleDriftAdHocTask: %v", err)
	}
}

// TestDBDriftReportRepo_List_WithCursor exercises the cursor decode branch.
// A valid encoded cursor is passed → baseWhere gains an extra condition.
func TestDBDriftReportRepo_List_WithCursor(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	// Build a valid cursor: encodeCursorStr wraps the string in base64.
	cursorStr := encodeCursorStr(time.Now().UTC().Format(time.RFC3339Nano))

	// The query will include the extra cursor condition — just match "SELECT".
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(driftReportSelectCols()))

	repo := NewDBDriftReportRepo(db)
	reports, _, err := repo.List(context.Background(), listquery.Query{}, cursorStr, 50)
	if err != nil {
		t.Fatalf("List with cursor: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBDriftReportRepo_List_HasMore exercises the hasMore=true path.
// Returning limit+1 rows triggers the slice truncation and nextCursor assignment.
func TestDBDriftReportRepo_List_HasMore(t *testing.T) {
	const testLimit = 2
	db, mock := newMockDB(t)
	defer db.Close()

	// Add testLimit+1 = 3 rows so hasMore triggers.
	rs := sqlmock.NewRows(driftReportSelectCols())
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range ids {
		rs = addDriftReportRow(rs, id, DriftStatusCompleted)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rs)

	repo := NewDBDriftReportRepo(db)
	reports, meta, err := repo.List(context.Background(), listquery.Query{}, "", testLimit)
	if err != nil {
		t.Fatalf("List hasMore: %v", err)
	}
	if len(reports) != testLimit {
		t.Fatalf("expected %d reports after truncation, got %d", testLimit, len(reports))
	}
	if !meta.HasMore {
		t.Error("expected hasMore=true")
	}
	if meta.NextCursor == nil {
		t.Error("expected nextCursor to be set when hasMore=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── ErrEIRDriftReportPeriodeOutOfRange and ErrEIRDriftThresholdInvalid constructors ──

func TestErrEIRDriftReportPeriodeOutOfRange(t *testing.T) {
	err := ErrEIRDriftReportPeriodeOutOfRange("2025-06")
	assertDomainCode(t, err, CodeEIRDriftReportPeriodeOutOfRange)
}

func TestErrEIRDriftThresholdInvalid(t *testing.T) {
	err := ErrEIRDriftThresholdInvalid("zero value")
	assertDomainCode(t, err, CodeEIRDriftThresholdInvalid)
}
