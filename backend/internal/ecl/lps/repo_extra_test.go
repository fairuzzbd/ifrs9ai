package lps

// repo_extra_test.go — additional repo coverage for ListByNasabahBank,
// BulkListDepositoForAggregate, GetActiveForInstrumen, GetActiveSetForInstrumens,
// and scanInstrumenDepositoRows.

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func overrideRowColumns() []string {
	return []string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}
}

// ─── DBDepositoInstrumenRepo.ListByNasabahBank ───────────────────────────────

func TestDBDepositoInstrumenRepo_ListByNasabahBank_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	nasabah := uuid.New()
	bank := uuid.New()
	instrID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen", "counterparty_id", "bank_counterparty_id",
		"nominal", "mata_uang", "tanggal_penempatan", "klasifikasi_psak71", "status", "tenant_id",
	}).AddRow(instrID, "DEP-001", "Deposito 001", nasabah, bank, "500000000.0000", "IDR", now, "AC", "AKTIF", "TUGURE")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.ListByNasabahBank(context.Background(), nasabah, bank, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
	if result[0].ID != instrID {
		t.Errorf("ID = %s, want %s", result[0].ID, instrID)
	}
	expectedNominal, _ := decimal.NewFromString("500000000.0000")
	if !result[0].Nominal.Equal(expectedNominal) {
		t.Errorf("Nominal = %s, want %s", result[0].Nominal, expectedNominal)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBDepositoInstrumenRepo_ListByNasabahBank_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen", "counterparty_id", "bank_counterparty_id",
		"nominal", "mata_uang", "tanggal_penempatan", "klasifikasi_psak71", "status", "tenant_id",
	}))

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.ListByNasabahBank(context.Background(), uuid.New(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %d rows", len(result))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBDepositoInstrumenRepo.BulkListDepositoForAggregate ───────────────────

func TestDBDepositoInstrumenRepo_BulkList_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	nasabah := uuid.New()
	bank := uuid.New()
	instrID := uuid.New()
	capID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"instrumen_id", "kode_instrumen", "tanggal_penempatan",
		"nasabah_id", "bank_id", "nominal", "mata_uang", "klasifikasi_psak71",
		"fx_rate", "lps_coverage_param_id", "lps_cap_idr",
		"override_id", "exclusion_reason", "nasabah_nama", "bank_nama", "tenant_id",
	}).AddRow(
		instrID, "DEP-001", now,
		nasabah, bank, "500000000.0000", "IDR", "AC",
		nil, capID, "2000000000.0000",
		nil, nil, "PT Test", "Bank Test", "TUGURE",
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.BulkListDepositoForAggregate(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].InstrumenID != instrID {
		t.Errorf("InstrumenID = %s, want %s", result[0].InstrumenID, instrID)
	}
	if result[0].FXRate != nil {
		t.Error("expected nil FXRate for IDR instrument")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBDepositoInstrumenRepo_BulkList_WithFXRate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	instrID := uuid.New()
	capID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"instrumen_id", "kode_instrumen", "tanggal_penempatan",
		"nasabah_id", "bank_id", "nominal", "mata_uang", "klasifikasi_psak71",
		"fx_rate", "lps_coverage_param_id", "lps_cap_idr",
		"override_id", "exclusion_reason", "nasabah_nama", "bank_nama", "tenant_id",
	}).AddRow(
		instrID, "DEP-USD-001", now,
		uuid.New(), uuid.New(), "100000.0000", "USD", "AC",
		"15432.12345678", capID, "2000000000.0000",
		nil, nil, "PT Test FCY", "Bank FCY", "TUGURE",
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	result, err := repo.BulkListDepositoForAggregate(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].FXRate == nil {
		t.Fatal("expected non-nil FXRate for USD instrument")
	}
	expectedRate, _ := decimal.NewFromString("15432.12345678")
	if !result[0].FXRate.Equal(expectedRate) {
		t.Errorf("FXRate = %s, want %s", result[0].FXRate, expectedRate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBOverrideRepo.GetActiveForInstrumen ────────────────────────────────────

func TestDBOverrideRepo_GetActiveForInstrumen_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows(overrideRowColumns()).AddRow(
		ovID, instrID, "Exclusion reason that is more than 30 chars", uuid.New(), uuid.New(),
		"APPROVED_ACTIVE", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	result, err := repo.GetActiveForInstrumen(context.Background(), instrID, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != ovID {
		t.Errorf("ID = %s, want %s", result.ID, ovID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_GetActiveForInstrumen_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(overrideRowColumns()))

	repo := NewDBOverrideRepo(db)
	result, err := repo.GetActiveForInstrumen(context.Background(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for no rows")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBOverrideRepo.GetActiveSetForInstrumens ────────────────────────────────

func TestDBOverrideRepo_GetActiveSetForInstrumens_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows(overrideRowColumns()).AddRow(
		ovID, instrID, "Exclusion reason that is more than 30 chars", uuid.New(), uuid.New(),
		"APPROVED_ACTIVE", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	result, err := repo.GetActiveSetForInstrumens(context.Background(), []uuid.UUID{instrID}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
	if ov, ok := result[instrID]; !ok || ov.ID != ovID {
		t.Errorf("result missing instrID key or wrong ID")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_GetActiveSetForInstrumens_Empty(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewDBOverrideRepo(db)
	// Empty input slice → short-circuit, no query.
	result, err := repo.GetActiveSetForInstrumens(context.Background(), []uuid.UUID{}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map for empty input, got %d", len(result))
	}
}

// ─── DBOverrideRepo.GetByID ───────────────────────────────────────────────────

func TestDBOverrideRepo_GetByID_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows(overrideRowColumns()).AddRow(
		ovID, instrID, "GetByID test reason that is > 30 chars", uuid.New(), uuid.New(),
		"PENDING_APPROVAL", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	result, err := repo.GetByID(context.Background(), ovID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != ovID {
		t.Errorf("ID = %s, want %s", result.ID, ovID)
	}
	if result.WorkflowStatus != WorkflowStatusPendingApproval {
		t.Errorf("status = %s, want PENDING_APPROVAL", result.WorkflowStatus)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(sqlmock.NewRows(overrideRowColumns()))

	repo := NewDBOverrideRepo(db)
	result, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for not found")
	}
}

// ─── DBOverrideRepo.List — filter branches ───────────────────────────────────

func TestDBOverrideRepo_List_NoFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(overrideRowColumns()))

	repo := NewDBOverrideRepo(db)
	result, cursor, hasMore, err := repo.List(context.Background(), "", "", "", "", "created_at", "desc", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 || cursor != "" || hasMore {
		t.Errorf("expected empty result, got %d rows cursor=%q hasMore=%v", len(result), cursor, hasMore)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_List_AllFiltersAndCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	// Return limit+1 rows to trigger hasMore=true path.
	rows := sqlmock.NewRows(overrideRowColumns())
	for i := 0; i < 3; i++ {
		rows = rows.AddRow(
			ovID, instrID, "Exclusion reason that is more than 30 chars long", uuid.New(), uuid.New(),
			"PENDING_APPROVAL", makerID, nil, nil, nil, nil, nil,
			now, makerID, now, makerID, nil, nil, 1, "TUGURE",
		)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	// limit=2 → 3 rows returned → hasMore=true
	result, cursor, hasMore, err := repo.List(
		context.Background(),
		"PENDING_APPROVAL", instrID.String(), makerID.String(),
		"exclusion keyword",
		"created_at", "asc",
		uuid.New().String(), // cursor → "<" (asc dir)
		2,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("len = %d, want 2 (hasMore trimmed)", len(result))
	}
	if !hasMore {
		t.Error("hasMore should be true")
	}
	if cursor == "" {
		t.Error("nextCursor should be non-empty when hasMore")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_List_InvalidSortCol(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(overrideRowColumns()))

	repo := NewDBOverrideRepo(db)
	// Invalid sort col + invalid sort dir → should fall back to defaults.
	_, _, _, err = repo.List(context.Background(), "", "", "", "", "injected_col; DROP TABLE", "invalid_dir", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_List_NilDB(t *testing.T) {
	repo := NewDBOverrideRepo(nil)
	result, cursor, hasMore, err := repo.List(context.Background(), "", "", "", "", "created_at", "desc", "", 10)
	if err != nil {
		t.Fatalf("unexpected error for nil db: %v", err)
	}
	if len(result) != 0 || cursor != "" || hasMore {
		t.Error("expected empty result for nil db")
	}
}
