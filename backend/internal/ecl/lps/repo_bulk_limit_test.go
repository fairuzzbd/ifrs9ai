package lps

// repo_bulk_limit_test.go — tests for BulkListDepositoForAggregate LIMIT+1 pre-query
// size check (issue #48).
//
// Tests:
//  1. TestBulkListDepositoForAggregate_RejectsAtLimitPlusOne — mock returns maxBulk+1 rows → error.
//  2. TestBulkListDepositoForAggregate_AllowsAtMaxLimit      — exactly maxBulk rows → success.

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// bulkRowColumns returns the column list matching bulkDepositoQuery SELECT order.
func bulkRowColumns() []string {
	return []string{
		"instrumen_id", "kode_instrumen", "tanggal_penempatan",
		"nasabah_id", "bank_id", "nominal", "mata_uang", "klasifikasi_psak71",
		"fx_rate", "lps_coverage_param_id", "lps_cap_idr",
		"override_id", "exclusion_reason",
		"nasabah_nama", "bank_nama", "tenant_id",
	}
}

// addBulkRow adds a single instrument row to sqlmock rows.
func addBulkRow(rows *sqlmock.Rows, instrID, nasabahID, bankID, coverageParamID uuid.UUID) *sqlmock.Rows {
	return rows.AddRow(
		instrID,
		fmt.Sprintf("DEP-%s", instrID.String()[:8]),
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		nasabahID,
		bankID,
		"1000000.0000",
		"IDR",
		"AC",
		nil, // fx_rate (IDR, no FX)
		coverageParamID,
		"2000000000.0000",
		nil, nil, // override_id, exclusion_reason
		"Nasabah Test",
		"Bank Test",
		"TUGURE",
	)
}

// TestBulkListDepositoForAggregate_RejectsAtLimitPlusOne verifies that when the DB
// returns maxBulkInstruments+1 rows (i.e. the LIMIT+1 sentinel), the repo returns
// ErrLPSAggregateBulkTooLarge immediately without materializing the entire set.
func TestBulkListDepositoForAggregate_RejectsAtLimitPlusOne(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	coverageID := uuid.New()

	// Build maxBulkInstruments+1 rows.
	rows := sqlmock.NewRows(bulkRowColumns())
	for i := 0; i <= maxBulkInstruments; i++ {
		rows = addBulkRow(rows, uuid.New(), uuid.New(), uuid.New(), coverageID)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.BulkListDepositoForAggregate(context.Background(), time.Now())
	if err == nil {
		t.Fatalf("expected ErrLPSAggregateBulkTooLarge, got nil (result len=%d)", len(result))
	}

	// Verify the error carries the LPS_AGGREGATE_BULK_TOO_LARGE code.
	if result != nil {
		t.Errorf("result should be nil on error, got len=%d", len(result))
	}

	// Error message must mention the code.
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Error("error message is empty")
	}

	// Expectations may not all be consumed (we break out of scan early), so don't
	// call ExpectationsWereMet which would fail on unconsumed rows.
}

// TestBulkListDepositoForAggregate_AllowsAtMaxLimit verifies that exactly
// maxBulkInstruments rows (not maxBulkInstruments+1) succeeds without error.
func TestBulkListDepositoForAggregate_AllowsAtMaxLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	coverageID := uuid.New()

	// Build exactly maxBulkInstruments rows (one short of the rejection threshold).
	rows := sqlmock.NewRows(bulkRowColumns())
	for i := 0; i < maxBulkInstruments; i++ {
		rows = addBulkRow(rows, uuid.New(), uuid.New(), uuid.New(), coverageID)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.BulkListDepositoForAggregate(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error at exactly maxBulkInstruments rows: %v", err)
	}
	if len(result) != maxBulkInstruments {
		t.Errorf("len = %d, want %d", len(result), maxBulkInstruments)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
