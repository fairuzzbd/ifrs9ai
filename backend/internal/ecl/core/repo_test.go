package core

// repo_test.go — CalcResultLineRepo tests using go-sqlmock.
//
// DEC-016: all NUMERIC columns stored as text via StringFixed.
// No float64. Uses go-sqlmock to avoid a live DB.
//
// Coverage target: InsertResultLine, GetPriorSealedECL, GetResultLine,
// ListResultLines, GetPortfolioAggregate, GetCalcRunECLTotal, ExistsResultLine,
// helper functions decimalPtrStr4, decimalPtrStr8, marshalWarnings.

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Helper builders ─────────────────────────────────────────────────────────

func newRepoWithMock(t *testing.T) (*CalcResultLineRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewCalcResultLineRepo(db), mock
}

func buildResultLineRow() ResultLineRow {
	pd := decimal.NewFromFloat(0.02)
	lgd := decimal.NewFromFloat(0.40)
	fl := decimal.NewFromFloat(1.10)
	ecl := decimal.NewFromFloat(4_400_000.0)
	ead := decimal.NewFromInt(1_000_000_000)
	return ResultLineRow{
		ID:             uuid.New(),
		CalcRunID:      uuid.New(),
		InstrumenID:    uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Stage:          Stage1,
		RoutingPath:    RoutingStandard,
		EADIDR:         ead,
		PDGood:         &pd,
		PDNormal:       &pd,
		PDBad:          &pd,
		LGDUsed:        &lgd,
		FLGood:         &fl,
		FLNormal:       &fl,
		FLBad:          &fl,
		ECLGoodIDR:     decimal.NewFromFloat(8_000_000.0),
		ECLNormalIDR:   decimal.NewFromFloat(8_000_000.0),
		ECLBadIDR:      decimal.NewFromFloat(8_000_000.0),
		ECLFLGoodIDR:   decimal.NewFromFloat(8_800_000.0),
		ECLFLNormalIDR: decimal.NewFromFloat(8_800_000.0),
		ECLFLBadIDR:    decimal.NewFromFloat(8_800_000.0),
		ECLWeightedIDR: &ecl,
		BobotGood:      decimal.NewFromFloat(0.25),
		BobotNormal:    decimal.NewFromFloat(0.50),
		BobotBad:       decimal.NewFromFloat(0.25),
		FlagPOCI:       false,
		Warnings:       []string{},
		ActorID:        uuid.New(),
	}
}

// ─── NewCalcResultLineRepo ────────────────────────────────────────────────────

func TestNewCalcResultLineRepo_PanicsOnNilDB(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	_ = NewCalcResultLineRepo(nil)
}

// ─── InsertResultLine ─────────────────────────────────────────────────────────

func TestInsertResultLine_OK(t *testing.T) {
	t.Parallel()
	row := buildResultLineRow()

	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.calc_result_line")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewCalcResultLineRepo(db)
	tx, _ := db.Begin()
	if err := repo.InsertResultLine(context.Background(), tx, row); err != nil {
		t.Fatalf("InsertResultLine: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestInsertResultLine_POCI_NilECL(t *testing.T) {
	t.Parallel()
	// POCI: ECLWeightedIDR = nil → stored as NULL.
	row := buildResultLineRow()
	row.ECLWeightedIDR = nil
	row.FlagPOCI = true

	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.calc_result_line")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewCalcResultLineRepo(db)
	tx, _ := db.Begin()
	if err := repo.InsertResultLine(context.Background(), tx, row); err != nil {
		t.Fatalf("InsertResultLine POCI: %v", err)
	}
}

// ─── GetPriorSealedECL ────────────────────────────────────────────────────────

func TestGetPriorSealedECL_RowExists(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	instrID := uuid.New()
	mock.ExpectQuery(`SELECT ecl_weighted_idr`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_weighted_idr"}).
			AddRow("8800000.0000"))

	got, err := repo.GetPriorSealedECL(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetPriorSealedECL: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil decimal")
	}
	want := decimal.NewFromFloat(8_800_000.0)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestGetPriorSealedECL_NoRows(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	instrID := uuid.New()
	mock.ExpectQuery(`SELECT ecl_weighted_idr`).
		WithArgs(instrID).
		WillReturnError(sql.ErrNoRows)

	got, err := repo.GetPriorSealedECL(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetPriorSealedECL no rows: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for no rows, got %s", got)
	}
}

func TestGetPriorSealedECL_NullValue(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	instrID := uuid.New()
	mock.ExpectQuery(`SELECT ecl_weighted_idr`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_weighted_idr"}).
			AddRow(nil)) // NULL → POCI row

	got, err := repo.GetPriorSealedECL(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetPriorSealedECL null: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for NULL DB value, got %s", got)
	}
}

// ─── GetResultLine ────────────────────────────────────────────────────────────

func TestGetResultLine_Found(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	instrID := uuid.New()
	lineID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id", "stage", "routing_path",
		"ead_idr", "pd_used_good", "pd_used_normal", "pd_used_bad", "lgd_used",
		"fl_multiplier_good", "fl_multiplier_normal", "fl_multiplier_bad",
		"ecl_good_idr", "ecl_normal_idr", "ecl_bad_idr",
		"ecl_fl_good_idr", "ecl_fl_normal_idr", "ecl_fl_bad_idr",
		"ecl_weighted_idr", "bobot_good", "bobot_normal", "bobot_bad",
		"net_carrying_idr", "prior_sealed_ecl_idr", "flag_poci", "parameter_snapshot_id",
		"warnings_json", "sealed_at", "created_at",
	}

	rows := sqlmock.NewRows(cols).AddRow(
		lineID, calcRunID, instrID, evalDate, "JUNI-2026", 1, "STANDARD",
		"1000000000.0000", "0.02000000", "0.02000000", "0.02000000", "0.40000000",
		"1.10000000", "1.00000000", "0.90000000",
		"8000000.0000", "8000000.0000", "8000000.0000",
		"8800000.0000", "8000000.0000", "7200000.0000",
		"8200000.0000", "0.2500", "0.5000", "0.2500",
		nil, nil, false, nil,
		"[]", nil, createdAt,
	)

	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(rows)

	row, err := repo.GetResultLine(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("GetResultLine: %v", err)
	}
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if row.ID != lineID {
		t.Errorf("ID: want %s, got %s", lineID, row.ID)
	}
	if row.Stage != Stage1 {
		t.Errorf("Stage: want 1, got %d", row.Stage)
	}
}

func TestGetResultLine_NotFound(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID, instrID := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnError(sql.ErrNoRows)

	row, err := repo.GetResultLine(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("GetResultLine not found: %v", err)
	}
	if row != nil {
		t.Error("expected nil row for ErrNoRows")
	}
}

// ─── ListResultLines ──────────────────────────────────────────────────────────

func TestListResultLines_Empty(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}
	// limit=50 → query with limit+1=51.
	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, 51).
		WillReturnRows(sqlmock.NewRows(cols))

	req := ListResultsRequest{CalcRunID: calcRunID, Limit: 50}
	resp, err := repo.ListResultLines(context.Background(), req)
	if err != nil {
		t.Fatalf("ListResultLines empty: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items: want 0, got %d", len(resp.Items))
	}
	if resp.HasMore {
		t.Error("hasMore: want false for empty result")
	}
}

func TestListResultLines_WithItems(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	lineID1 := uuid.New()
	lineID2 := uuid.New()
	instrID1 := uuid.New()
	instrID2 := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(lineID1, calcRunID, instrID1, evalDate, "JUNI-2026", 1, "STANDARD", "1000000000.0000", "4000000.0000", false, nil, createdAt).
		AddRow(lineID2, calcRunID, instrID2, evalDate, "JUNI-2026", 2, "STANDARD", "500000000.0000", "2500000.0000", false, nil, createdAt)

	// limit=2 → limit+1=3.
	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, 3).
		WillReturnRows(rows)

	req := ListResultsRequest{CalcRunID: calcRunID, Limit: 2}
	resp, err := repo.ListResultLines(context.Background(), req)
	if err != nil {
		t.Fatalf("ListResultLines with items: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("items: want 2, got %d", len(resp.Items))
	}
	if resp.HasMore {
		t.Error("hasMore: want false (exactly 2 returned, limit 2)")
	}
}

func TestListResultLines_HasMore(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}

	// Return limit+1 rows to trigger hasMore.
	sqlRows := sqlmock.NewRows(cols)
	for i := 0; i < 3; i++ { // limit=2 → ask 3, get 3 → hasMore=true
		sqlRows.AddRow(uuid.New(), calcRunID, uuid.New(), evalDate, "JUNI-2026", 1, "STANDARD",
			"1000000000.0000", "4000000.0000", false, nil, createdAt)
	}

	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, 3).
		WillReturnRows(sqlRows)

	req := ListResultsRequest{CalcRunID: calcRunID, Limit: 2}
	resp, err := repo.ListResultLines(context.Background(), req)
	if err != nil {
		t.Fatalf("ListResultLines hasMore: %v", err)
	}
	if !resp.HasMore {
		t.Error("hasMore: want true when result > limit")
	}
	if len(resp.Items) != 2 { // trimmed to limit
		t.Errorf("items: want 2, got %d", len(resp.Items))
	}
	if resp.NextCursor == "" {
		t.Error("nextCursor: must not be empty when hasMore=true")
	}
}

func TestListResultLines_WithStageFilter(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	s2 := Stage2
	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}
	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, 2, 51).
		WillReturnRows(sqlmock.NewRows(cols))

	req := ListResultsRequest{CalcRunID: calcRunID, Limit: 50, Stage: &s2}
	if _, err := repo.ListResultLines(context.Background(), req); err != nil {
		t.Fatalf("ListResultLines with stage filter: %v", err)
	}
}

// ─── ExistsResultLine ─────────────────────────────────────────────────────────

func TestExistsResultLine_True(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID, instrID := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	exists, err := repo.ExistsResultLine(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("ExistsResultLine true: %v", err)
	}
	if !exists {
		t.Error("want exists=true")
	}
}

func TestExistsResultLine_False(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID, instrID := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	exists, err := repo.ExistsResultLine(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("ExistsResultLine false: %v", err)
	}
	if exists {
		t.Error("want exists=false")
	}
}

// ─── GetCalcRunECLTotal ───────────────────────────────────────────────────────

func TestGetCalcRunECLTotal(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID := uuid.New()
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("12345678.9000"))

	got, err := repo.GetCalcRunECLTotal(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("GetCalcRunECLTotal: %v", err)
	}
	want := decimal.NewFromFloat(12_345_678.9)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want, got)
	}
}

// ─── Helper functions ─────────────────────────────────────────────────────────

func TestDecimalPtrStr8_Nil(t *testing.T) {
	t.Parallel()
	got := decimalPtrStr8(nil)
	if got != nil {
		t.Errorf("decimalPtrStr8(nil): want nil, got %v", got)
	}
}

func TestDecimalPtrStr8_Value(t *testing.T) {
	t.Parallel()
	d := decimal.NewFromFloat(0.02)
	got := decimalPtrStr8(&d)
	if got == nil {
		t.Fatal("decimalPtrStr8: want non-nil")
	}
	// Returns interface{} containing a string.
	s, ok := got.(string)
	if !ok {
		t.Fatalf("decimalPtrStr8: want string, got %T", got)
	}
	if s != "0.02000000" {
		t.Errorf("decimalPtrStr8: want '0.02000000', got %q", s)
	}
}

func TestDecimalPtrStr4_Nil(t *testing.T) {
	t.Parallel()
	got := decimalPtrStr4(nil)
	if got != nil {
		t.Errorf("decimalPtrStr4(nil): want nil, got %v", got)
	}
}

func TestDecimalPtrStr4_Value(t *testing.T) {
	t.Parallel()
	d := decimal.NewFromFloat(1234567.89)
	got := decimalPtrStr4(&d)
	if got == nil {
		t.Fatal("decimalPtrStr4: want non-nil")
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("decimalPtrStr4: want string, got %T", got)
	}
	if s != "1234567.8900" {
		t.Errorf("decimalPtrStr4: want '1234567.8900', got %q", s)
	}
}

func TestMarshalWarnings_Empty(t *testing.T) {
	t.Parallel()
	// marshalWarnings returns nil, nil for nil/empty slice (SQL NULL).
	got, err := marshalWarnings(nil)
	if err != nil {
		t.Fatalf("marshalWarnings nil: %v", err)
	}
	if got != nil {
		t.Errorf("marshalWarnings(nil): want nil, got %v", got)
	}

	got2, err := marshalWarnings([]string{})
	if err != nil {
		t.Fatalf("marshalWarnings empty slice: %v", err)
	}
	if got2 != nil {
		t.Errorf("marshalWarnings([]string{}): want nil, got %v", got2)
	}
}

func TestMarshalWarnings_WithValues(t *testing.T) {
	t.Parallel()
	got, err := marshalWarnings([]string{"W1", "W2"})
	if err != nil {
		t.Fatalf("marshalWarnings with values: %v", err)
	}
	// Should be valid JSON string.
	if got == nil {
		t.Error("marshalWarnings: want non-nil output for non-empty slice")
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("marshalWarnings: want string, got %T", got)
	}
	if len(s) == 0 {
		t.Error("marshalWarnings: want non-empty JSON string")
	}
}

// ─── GetPortfolioAggregate ────────────────────────────────────────────────────

func TestGetPortfolioAggregate_OK(t *testing.T) {
	t.Parallel()
	repo, mock := newRepoWithMock(t)

	calcRunID, portfolioID := uuid.New(), uuid.New()
	cols := []string{"stage_label", "cnt", "ead_total", "ecl_total"}
	// GetPortfolioAggregate builds TOTAL row itself — only return stage rows from DB.
	rows := sqlmock.NewRows(cols).
		AddRow("STAGE_1", 10, "5000000000.0000", "2500000.0000").
		AddRow("STAGE_2", 2, "1000000000.0000", "800000.0000")

	mock.ExpectQuery(`SELECT`).
		WithArgs(calcRunID, portfolioID).
		WillReturnRows(rows)

	summaryRows, err := repo.GetPortfolioAggregate(context.Background(), calcRunID, portfolioID)
	if err != nil {
		t.Fatalf("GetPortfolioAggregate: %v", err)
	}
	// Expect: STAGE_1, STAGE_2, TOTAL (auto-appended).
	if len(summaryRows) != 3 {
		t.Errorf("want 3 rows (2 stages + TOTAL), got %d", len(summaryRows))
	}
	// Verify TOTAL row = 2500000 + 800000 = 3300000.
	var total StageSummaryRow
	for _, r := range summaryRows {
		if r.Stage == "TOTAL" {
			total = r
		}
	}
	want := decimal.NewFromFloat(3_300_000.0)
	if !total.ECLWeightedTotalIDR.Equal(want) {
		t.Errorf("TOTAL ECL: want %s, got %s", want, total.ECLWeightedTotalIDR)
	}
}
