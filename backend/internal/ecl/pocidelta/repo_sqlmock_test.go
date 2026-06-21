package pocidelta

// repo_sqlmock_test.go — sqlmock-based tests for sqlRepo (concrete DB implementation).
// These tests cover all repo methods that were 0% covered.
// Does NOT use a real PostgreSQL — uses DATA-DOG/go-sqlmock for isolation.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// newTestDB opens a *sql.DB backed by sqlmock.
func newTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

// ─── NewRepository ────────────────────────────────────────────────────────────

func TestNewRepository_ReturnsRepo(t *testing.T) {
	db, _ := newTestDB(t)
	r := NewRepository(db)
	if r == nil {
		t.Fatal("expected non-nil repository")
	}
}

// ─── repoLimit ────────────────────────────────────────────────────────────────

func TestRepoLimit_Default(t *testing.T) {
	got := repoLimit(listquery.Query{})
	if got != defaultLimit {
		t.Fatalf("expected %d, got %d", defaultLimit, got)
	}
}

// ─── paginateBaseline ─────────────────────────────────────────────────────────

func TestPaginateBaseline_NoMore(t *testing.T) {
	rows := []Baseline{{ID: uuid.New()}, {ID: uuid.New()}}
	out, pag, err := paginateBaseline(rows, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false")
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
}

func TestPaginateBaseline_HasMore(t *testing.T) {
	// limit+1 trick: provide limit+1 rows → HasMore=true, return only first limit
	limit := 2
	rows := []Baseline{{ID: uuid.New()}, {ID: uuid.New()}, {ID: uuid.New()}}
	out, pag, err := paginateBaseline(rows, limit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pag.HasMore {
		t.Fatal("expected HasMore=true")
	}
	if len(out) != limit {
		t.Fatalf("expected %d rows, got %d", limit, len(out))
	}
}

// ─── paginateDeltaLog ─────────────────────────────────────────────────────────

func TestPaginateDeltaLog_NoMore(t *testing.T) {
	rows := []DeltaLog{{ID: uuid.New()}}
	out, pag, err := paginateDeltaLog(rows, 50)
	if err != nil {
		t.Fatal(err)
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false")
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out))
	}
}

func TestPaginateDeltaLog_HasMore(t *testing.T) {
	limit := 1
	rows := []DeltaLog{{ID: uuid.New()}, {ID: uuid.New()}}
	out, pag, err := paginateDeltaLog(rows, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !pag.HasMore {
		t.Fatal("expected HasMore=true")
	}
	if len(out) != limit {
		t.Fatalf("expected %d rows, got %d", limit, len(out))
	}
}

// ─── execCtx ─────────────────────────────────────────────────────────────────

func TestExecCtx_WithTx(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO test").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	err := r.execCtx(context.Background(), tx, "INSERT INTO test VALUES ($1)", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()
}

func TestExecCtx_WithoutTx(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectExec("INSERT INTO test").WillReturnResult(sqlmock.NewResult(1, 1))

	err := r.execCtx(context.Background(), nil, "INSERT INTO test VALUES ($1)", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── InsertBaseline ───────────────────────────────────────────────────────────

func TestInsertBaseline_Success(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_baseline").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()

	rawJSON := json.RawMessage(`{"key":"value"}`)
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		TanggalBaseline:         time.Now(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000),
		CashflowExpektasiJsonb:  &rawJSON,
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
		OriginationDate:         time.Now(),
		CreatedAt:               time.Now(),
		CreatedBy:               uuid.New(),
		TenantID:                "TUGURE",
	}

	if err := r.InsertBaseline(context.Background(), tx, b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertBaseline_NilCashflow(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_baseline").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		TanggalBaseline:         time.Now(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CashflowExpektasiJsonb:  nil, // nil cashflow
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
		OriginationDate:         time.Now(),
		CreatedAt:               time.Now(),
		CreatedBy:               uuid.New(),
		TenantID:                "TUGURE",
	}

	if err := r.InsertBaseline(context.Background(), tx, b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()
}

func TestInsertBaseline_UniqueConstraintViolation(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_baseline").
		WillReturnError(fmt.Errorf("pq: duplicate key value violates unique constraint"))
	mock.ExpectRollback()

	tx, _ := db.Begin()
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
		TenantID:                "TUGURE",
	}

	err := r.InsertBaseline(context.Background(), tx, b)
	if err == nil {
		t.Fatal("expected error for constraint violation")
	}
	_ = tx.Rollback()
}

// ─── GetBaselineByInstrumen ───────────────────────────────────────────────────

func TestGetBaselineByInstrumenSQL_Found(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	instrID := uuid.New()
	baseID := uuid.New()
	createdBy := uuid.New()
	now := time.Now()

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	rows := sqlmock.NewRows(cols).AddRow(
		baseID, instrID, now, "1250000000.0000",
		nil, "0.04500000", now,
		now, createdBy, "TUGURE",
	)
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(rows)

	b, err := r.GetBaselineByInstrumen(context.Background(), instrID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.InstrumenID != instrID {
		t.Fatalf("instrumen_id mismatch: got %s, want %s", b.InstrumenID, instrID)
	}
}

func TestGetBaselineByInstrumenSQL_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(sqlmock.NewRows(nil))

	b, err := r.GetBaselineByInstrumen(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != nil {
		t.Fatal("expected nil baseline for not-found")
	}
}

func TestGetBaselineByInstrumen_WithCashflow(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	instrID := uuid.New()
	now := time.Now()
	cashflow := []byte(`{"cf":[100]}`)

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	rows := sqlmock.NewRows(cols).AddRow(
		uuid.New(), instrID, now, "1000000.0000",
		cashflow, "0.05000000", now,
		now, uuid.New(), "TUGURE",
	)
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(rows)

	b, err := r.GetBaselineByInstrumen(context.Background(), instrID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.CashflowExpektasiJsonb == nil {
		t.Fatal("expected non-nil cashflow jsonb")
	}
}

func TestGetBaselineByInstrumen_BadECLParse(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	rows := sqlmock.NewRows(cols).AddRow(
		uuid.New(), uuid.New(), time.Now(), "NOT_A_NUMBER",
		nil, "0.05000000", time.Now(),
		time.Now(), uuid.New(), "TUGURE",
	)
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(rows)

	_, err := r.GetBaselineByInstrumen(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for invalid ECL")
	}
}

func TestGetBaselineByInstrumen_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnError(errors.New("connection reset"))

	_, err := r.GetBaselineByInstrumen(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected DB error")
	}
}

// ─── ListBaselines ────────────────────────────────────────────────────────────

func TestListBaselines_Empty(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(sqlmock.NewRows(nil))

	rows, pag, err := r.ListBaselines(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false for empty result")
	}
}

func TestListBaselines_HasMore(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	now := time.Now()
	// Populate defaultLimit+1 rows to trigger HasMore=true
	rowSet := sqlmock.NewRows(cols)
	for i := 0; i <= defaultLimit; i++ {
		rowSet.AddRow(uuid.New(), uuid.New(), now, "100.0000", nil, "0.05000000", now, now, uuid.New(), "TUGURE")
	}
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(rowSet)

	_, pag, err := r.ListBaselines(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pag.HasMore {
		t.Fatal("expected HasMore=true")
	}
}

func TestListBaselines_QueryError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnError(errors.New("db error"))

	_, _, err := r.ListBaselines(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── InsertDeltaLog ───────────────────────────────────────────────────────────

func TestInsertDeltaLog_Success(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_delta_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	prior := decimal.NewFromFloat(100000)
	d := &DeltaLog{
		ID:                   uuid.New(),
		CalcRunID:            uuid.New(),
		InstrumenID:          uuid.New(),
		TanggalCompute:      time.Now(),
		BaselineECL:         decimal.NewFromFloat(1000000),
		CurrentECL:          decimal.NewFromFloat(1200000),
		DeltaECL:            decimal.NewFromFloat(200000),
		Direction:           DirectionIncrease,
		PriorDeltaCumulative: &prior,
		Status:              StatusComputed,
		CreatedAt:           time.Now(),
		CreatedBy:           uuid.New(),
		UpdatedAt:           time.Now(),
		UpdatedBy:           uuid.New(),
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	if err := r.InsertDeltaLog(context.Background(), tx, d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()
}

func TestInsertDeltaLog_NilPriorCumulative(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_delta_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	d := &DeltaLog{
		ID:                   uuid.New(),
		CalcRunID:            uuid.New(),
		InstrumenID:          uuid.New(),
		TanggalCompute:      time.Now(),
		BaselineECL:         decimal.NewFromFloat(1000000),
		CurrentECL:          decimal.NewFromFloat(1000000),
		DeltaECL:            decimal.Zero,
		Direction:           DirectionZero,
		PriorDeltaCumulative: nil, // nil prior
		Status:              StatusSkippedZero,
		CreatedAt:           time.Now(),
		CreatedBy:           uuid.New(),
		UpdatedAt:           time.Now(),
		UpdatedBy:           uuid.New(),
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	if err := r.InsertDeltaLog(context.Background(), tx, d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()
}

func TestInsertDeltaLog_UniqueConstraintError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.poci_delta_log").
		WillReturnError(fmt.Errorf("pq: duplicate key value violates unique constraint uq_poci_delta_run_instrumen"))
	mock.ExpectRollback()

	tx, _ := db.Begin()
	d := &DeltaLog{
		ID:          uuid.New(),
		CalcRunID:   uuid.New(),
		InstrumenID: uuid.New(),
		BaselineECL: decimal.NewFromFloat(1000000),
		CurrentECL:  decimal.NewFromFloat(1200000),
		DeltaECL:    decimal.NewFromFloat(200000),
		Direction:   DirectionIncrease,
		Status:      StatusComputed,
		TenantID:    "TUGURE",
	}

	err := r.InsertDeltaLog(context.Background(), tx, d)
	if err == nil {
		t.Fatal("expected constraint error")
	}
	_ = tx.Rollback()
}

// ─── GetDeltaLogByRunAndInstrumen ─────────────────────────────────────────────

func makeDeltaLogRow(calcRunID, instrID uuid.UUID) *sqlmock.Rows {
	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	return sqlmock.NewRows(cols).AddRow(
		uuid.New(), calcRunID, instrID, now,
		"1000000.0000", "1200000.0000", "200000.0000", "INCREASE",
		nil, nil, nil,
		"COMPUTED",
		now, uuid.New(), now, uuid.New(),
		nil, nil, 1, "TUGURE",
	)
}

func TestGetDeltaLogByRunAndInstrumen_Found(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	calcRunID := uuid.New()
	instrID := uuid.New()

	mock.ExpectQuery("SELECT id, calc_run_id").
		WillReturnRows(makeDeltaLogRow(calcRunID, instrID))

	d, err := r.GetDeltaLogByRunAndInstrumen(context.Background(), calcRunID, instrID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil delta log")
	}
}

func TestGetDeltaLogByRunAndInstrumen_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").
		WillReturnRows(sqlmock.NewRows(nil))

	d, err := r.GetDeltaLogByRunAndInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error for not-found: %v", err)
	}
	if d != nil {
		t.Fatal("expected nil for not-found")
	}
}

func TestGetDeltaLogByRunAndInstrumen_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").
		WillReturnError(errors.New("connection refused"))

	_, err := r.GetDeltaLogByRunAndInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── UpdateDeltaLogStatus ─────────────────────────────────────────────────────

func TestUpdateDeltaLogStatus_Success(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE ecl.poci_delta_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	jurnalID := uuid.New()
	err := r.UpdateDeltaLogStatus(
		context.Background(), tx,
		uuid.New(), time.Now(),
		StatusPosted, &jurnalID, uuid.New(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = tx.Commit()
}

// ─── ListDeltaLogs ────────────────────────────────────────────────────────────

func TestListDeltaLogs_Empty(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(sqlmock.NewRows(nil))

	rows, pag, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0, got %d", len(rows))
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false")
	}
}

func TestListDeltaLogs_QueryError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnError(errors.New("db error"))

	_, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListDeltaLogs_WithRows_HasMore(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	rowSet := sqlmock.NewRows(cols)
	for i := 0; i <= defaultLimit; i++ {
		rowSet.AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"1000.0000", "1200.0000", "200.0000", "INCREASE",
			nil, nil, nil,
			"COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		)
	}
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(rowSet)

	_, pag, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pag.HasMore {
		t.Fatal("expected HasMore=true")
	}
}

// ─── GetDeltaHistoryByInstrumen ───────────────────────────────────────────────

func TestGetDeltaHistoryByInstrumen_Empty(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(sqlmock.NewRows(nil))

	rows, pag, err := r.GetDeltaHistoryByInstrumen(context.Background(), uuid.New(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false")
	}
}

func TestGetDeltaHistoryByInstrumen_QueryError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnError(errors.New("timeout"))

	_, _, err := r.GetDeltaHistoryByInstrumen(context.Background(), uuid.New(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetCumulativeDelta ───────────────────────────────────────────────────────

func TestGetCumulativeDelta_Zero(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(
		sqlmock.NewRows([]string{"sum"}).AddRow("0"),
	)

	d, err := r.GetCumulativeDelta(context.Background(), uuid.New(), time.Now(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Equal(decimal.Zero) {
		t.Fatalf("expected 0, got %s", d)
	}
}

func TestGetCumulativeDelta_NonZero(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(
		sqlmock.NewRows([]string{"sum"}).AddRow("500000.0000"),
	)

	d, err := r.GetCumulativeDelta(context.Background(), uuid.New(), time.Now(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Equal(decimal.NewFromFloat(500000)) {
		t.Fatalf("expected 500000, got %s", d)
	}
}

func TestGetCumulativeDelta_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT COALESCE").WillReturnError(errors.New("db err"))

	_, err := r.GetCumulativeDelta(context.Background(), uuid.New(), time.Now(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetDeltaSummary ──────────────────────────────────────────────────────────

// m4 fix: GetDeltaSummary now runs real SQL — update existing tests to provide mock rows.
func TestGetDeltaSummary_ReturnsStub(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"instr_count",
			"delta_mtd", "delta_ytd", "delta_net",
			"inc_count", "inc_amount",
			"dec_count", "dec_amount",
			"zero_count",
		}).AddRow(0, "0", "0", "0", 0, "0", 0, "0", 0))

	sum, err := r.GetDeltaSummary(context.Background(), nil, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.Year != 2026 || sum.Month != 6 {
		t.Fatalf("unexpected summary: year=%d month=%d", sum.Year, sum.Month)
	}
}

func TestGetDeltaSummarySQL_WithPortofolioID(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"instr_count",
			"delta_mtd", "delta_ytd", "delta_net",
			"inc_count", "inc_amount",
			"dec_count", "dec_amount",
			"zero_count",
		}).AddRow(1, "50000", "50000", "50000", 1, "50000", 0, "0", 0))

	pid := uuid.New()
	sum, err := r.GetDeltaSummary(context.Background(), &pid, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum == nil {
		t.Fatal("expected non-nil summary")
	}
}

// ─── GetInstrumenPociInfo ─────────────────────────────────────────────────────

func TestGetInstrumenPociInfo_Found(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	instrID := uuid.New()
	portoID := uuid.New()

	mock.ExpectQuery("SELECT id, kode_instrumen").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kode_instrumen", "is_poci", "status", "portofolio_id"}).
			AddRow(instrID, "INSTR-001", true, "ACTIVE", portoID),
	)

	info, err := r.GetInstrumenPociInfo(context.Background(), instrID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if !info.IsPoci {
		t.Fatal("expected IsPoci=true")
	}
}

func TestGetInstrumenPociInfo_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, kode_instrumen").WillReturnRows(sqlmock.NewRows(nil))

	info, err := r.GetInstrumenPociInfo(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatal("expected nil info")
	}
}

func TestGetInstrumenPociInfo_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT id, kode_instrumen").WillReturnError(errors.New("db error"))

	_, err := r.GetInstrumenPociInfo(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── ListPociInstrumenByCalcRun ───────────────────────────────────────────────

func TestListPociInstrumenByCalcRun_Empty(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT DISTINCT i.id").WillReturnRows(sqlmock.NewRows(nil))

	infos, err := r.ListPociInstrumenByCalcRun(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("expected 0, got %d", len(infos))
	}
}

func TestListPociInstrumenByCalcRun_WithRows(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	calcRunID := uuid.New()
	mock.ExpectQuery("SELECT DISTINCT i.id").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kode_instrumen", "is_poci", "status", "portofolio_id"}).
			AddRow(uuid.New(), "INSTR-001", true, "ACTIVE", uuid.New()).
			AddRow(uuid.New(), "INSTR-002", true, "ACTIVE", uuid.New()),
	)

	infos, err := r.ListPociInstrumenByCalcRun(context.Background(), calcRunID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2, got %d", len(infos))
	}
}

func TestListPociInstrumenByCalcRun_QueryError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT DISTINCT i.id").WillReturnError(errors.New("timeout"))

	_, err := r.ListPociInstrumenByCalcRun(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetPeriodeStatus ─────────────────────────────────────────────────────────

func TestGetPeriodeStatus_Open(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT status_periode").WillReturnRows(
		sqlmock.NewRows([]string{"status_periode"}).AddRow("OPEN"),
	)

	status, err := r.GetPeriodeStatus(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "OPEN" {
		t.Fatalf("expected OPEN, got %s", status)
	}
}

func TestGetPeriodeStatus_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT status_periode").WillReturnError(errors.New("not found"))

	_, err := r.GetPeriodeStatus(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetCalcRunStatus ─────────────────────────────────────────────────────────

func TestGetCalcRunStatus_Sealed(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	calcRunID := uuid.New()
	mock.ExpectQuery("SELECT status FROM ecl.ecl_calc_run").WillReturnRows(
		sqlmock.NewRows([]string{"status"}).AddRow("SEALED"),
	)

	status, err := r.GetCalcRunStatus(context.Background(), calcRunID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "SEALED" {
		t.Fatalf("expected SEALED, got %s", status)
	}
}

func TestGetCalcRunStatus_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT status FROM ecl.ecl_calc_run").
		WillReturnRows(sqlmock.NewRows(nil))

	_, err := r.GetCalcRunStatus(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetCalcRunStatus_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT status FROM ecl.ecl_calc_run").
		WillReturnError(errors.New("connection refused"))

	_, err := r.GetCalcRunStatus(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetCurrentECLForPociInstrumen ────────────────────────────────────────────

func TestGetCurrentECLForPociInstrumen_Found(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT ecl_weighted").WillReturnRows(
		sqlmock.NewRows([]string{"ecl_weighted"}).AddRow("1450000.0000"),
	)

	ecl, err := r.GetCurrentECLForPociInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := decimal.NewFromFloat(1450000)
	if !ecl.Equal(want) {
		t.Fatalf("expected %s, got %s", want, ecl)
	}
}

func TestGetCurrentECLForPociInstrumen_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT ecl_weighted").WillReturnRows(sqlmock.NewRows(nil))

	_, err := r.GetCurrentECLForPociInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error for not-found")
	}
}

func TestGetCurrentECLForPociInstrumen_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT ecl_weighted").WillReturnError(errors.New("db error"))

	_, err := r.GetCurrentECLForPociInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetLargeDeltaThreshold ───────────────────────────────────────────────────

func TestGetLargeDeltaThreshold_FromDB(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT value FROM sys.parameter").WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow("1000000000"),
	)

	d, err := r.GetLargeDeltaThreshold(context.Background(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := decimal.NewFromFloat(1000000000)
	if !d.Equal(want) {
		t.Fatalf("expected %s, got %s", want, d)
	}
}

func TestGetLargeDeltaThreshold_Default(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT value FROM sys.parameter").WillReturnRows(sqlmock.NewRows(nil))

	d, err := r.GetLargeDeltaThreshold(context.Background(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// default is 500000000
	want := decimal.NewFromFloat(500000000)
	if !d.Equal(want) {
		t.Fatalf("expected default %s, got %s", want, d)
	}
}

func TestGetLargeDeltaThreshold_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT value FROM sys.parameter").WillReturnError(errors.New("timeout"))

	_, err := r.GetLargeDeltaThreshold(context.Background(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── scanDeltaLog — error paths ───────────────────────────────────────────────

func TestScanDeltaLog_BadBaselineECL(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"BAD_DECIMAL", "1200.0000", "200.0000", "INCREASE",
			nil, nil, nil,
			"COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	_, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected scan error for bad baseline_ecl")
	}
}

func TestScanDeltaLog_WithPriorCumulative(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	priorStr := "100000.0000"
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"1000.0000", "1200.0000", "200.0000", "INCREASE",
			&priorStr, nil, nil,
			"COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	rows, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].PriorDeltaCumulative == nil {
		t.Fatal("expected non-nil PriorDeltaCumulative")
	}
}

// ─── GetPeriodeBulananIDForCalcRun (B2) ───────────────────────────────────────

func TestGetPeriodeBulananIDForCalcRun_Success(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	periodeID := uuid.New()
	calcRunID := uuid.New()

	mock.ExpectQuery("SELECT periode_bulanan_id FROM ecl.ecl_calc_run").
		WithArgs(calcRunID, "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"periode_bulanan_id"}).AddRow(periodeID))

	got, err := r.GetPeriodeBulananIDForCalcRun(context.Background(), calcRunID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != periodeID {
		t.Fatalf("expected %s, got %s", periodeID, got)
	}
}

func TestGetPeriodeBulananIDForCalcRun_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT periode_bulanan_id FROM ecl.ecl_calc_run").
		WillReturnRows(sqlmock.NewRows([]string{"periode_bulanan_id"}))

	_, err := r.GetPeriodeBulananIDForCalcRun(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestGetPeriodeBulananIDForCalcRun_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT periode_bulanan_id FROM ecl.ecl_calc_run").
		WillReturnError(errors.New("db error"))

	_, err := r.GetPeriodeBulananIDForCalcRun(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── GetDeltaSummary (m4) ─────────────────────────────────────────────────────

func TestGetDeltaSummary_Success(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"instr_count",
			"delta_mtd", "delta_ytd", "delta_net",
			"inc_count", "inc_amount",
			"dec_count", "dec_amount",
			"zero_count",
		}).AddRow(
			3,
			"100000.0000", "250000.0000", "50000.0000",
			2, "100000.0000",
			1, "50000.0000",
			0,
		))

	summary, err := r.GetDeltaSummary(context.Background(), nil, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.InstrumenCount != 3 {
		t.Fatalf("expected InstrumenCount=3, got %d", summary.InstrumenCount)
	}
	if summary.DirectionBreakdown.Increase.Count != 2 {
		t.Fatalf("expected Increase.Count=2, got %d", summary.DirectionBreakdown.Increase.Count)
	}
	if summary.DirectionBreakdown.Decrease.Count != 1 {
		t.Fatalf("expected Decrease.Count=1, got %d", summary.DirectionBreakdown.Decrease.Count)
	}
}

func TestGetDeltaSummary_SQL_WithPortofolioID(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	portoID := uuid.New()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows([]string{
			"instr_count",
			"delta_mtd", "delta_ytd", "delta_net",
			"inc_count", "inc_amount",
			"dec_count", "dec_amount",
			"zero_count",
		}).AddRow(
			1,
			"50000.0000", "50000.0000", "50000.0000",
			1, "50000.0000",
			0, "0.0000",
			0,
		))

	summary, err := r.GetDeltaSummary(context.Background(), &portoID, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.PortofolioID == nil {
		t.Fatal("expected non-nil PortofolioID")
	}
}

func TestGetDeltaSummary_DBError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT").WillReturnError(errors.New("db error"))

	_, err := r.GetDeltaSummary(context.Background(), nil, 2026, 6, "TUGURE")
	if err == nil {
		t.Fatal("expected error")
	}
}
