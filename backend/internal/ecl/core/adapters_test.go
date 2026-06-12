package core

// adapters_test.go — tests for DBInstrumenReader and DBBobotRepo DB adapters.

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── joinStrings ──────────────────────────────────────────────────────────────

func TestJoinStrings(t *testing.T) {
	t.Parallel()
	if got := joinStrings([]string{"a", "b", "c"}, ","); got != "a,b,c" {
		t.Errorf("joinStrings: want 'a,b,c', got %q", got)
	}
	if got := joinStrings([]string{"x"}, ","); got != "x" {
		t.Errorf("joinStrings single: want 'x', got %q", got)
	}
	if got := joinStrings(nil, ","); got != "" {
		t.Errorf("joinStrings empty: want '', got %q", got)
	}
}

// ─── DBInstrumenReader ────────────────────────────────────────────────────────

func TestNewDBInstrumenReader_PanicsOnNilDB(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	_ = NewDBInstrumenReader(nil)
}

func TestDBInstrumenReader_GetByID_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	counterpartyID := uuid.New()
	nasabahID := uuid.New()
	portofolioID := uuid.New()

	cols := []string{"id", "klasifikasi_psak71", "tipe_instrumen", "status", "workflow_status",
		"flag_poci", "counterparty_id", "nasabah_id", "portofolio_id", "tenant_id"}
	mock.ExpectQuery(`SELECT id, klasifikasi_psak71`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			instrID, "AC", "OBLIGASI", "AKTIF", "APPROVED",
			false, counterpartyID, nasabahID, &portofolioID, "TUGURE",
		))

	reader := NewDBInstrumenReader(db)
	snap, err := reader.GetByID(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.ID != instrID {
		t.Errorf("ID: want %s, got %s", instrID, snap.ID)
	}
	if snap.KlasifikasiPsak71 != "AC" {
		t.Errorf("KlasifikasiPsak71: want AC, got %s", snap.KlasifikasiPsak71)
	}
	if snap.TipeInstrumen != "OBLIGASI" {
		t.Errorf("TipeInstrumen: want OBLIGASI, got %s", snap.TipeInstrumen)
	}
}

func TestDBInstrumenReader_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	mock.ExpectQuery(`SELECT id, klasifikasi_psak71`).
		WithArgs(instrID).
		WillReturnError(sql.ErrNoRows)

	reader := NewDBInstrumenReader(db)
	snap, err := reader.GetByID(context.Background(), instrID)
	if snap != nil {
		t.Error("expected nil snap for not found")
	}
	ce, ok := err.(*coreError)
	if !ok {
		t.Fatalf("want *coreError, got %T: %v", err, err)
	}
	if ce.code != CodeECLInstrumenNotFound {
		t.Errorf("code: want %s, got %s", CodeECLInstrumenNotFound, ce.code)
	}
}

func TestDBInstrumenReader_ListActiveByScope_Empty(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cols := []string{"id", "klasifikasi_psak71", "tipe_instrumen", "status", "workflow_status",
		"flag_poci", "counterparty_id", "nasabah_id", "portofolio_id", "tenant_id"}
	mock.ExpectQuery(`SELECT id, klasifikasi_psak71`).
		WillReturnRows(sqlmock.NewRows(cols))

	reader := NewDBInstrumenReader(db)
	result, err := reader.ListActiveByScope(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListActiveByScope: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("want 0 items, got %d", len(result))
	}
}

func TestDBInstrumenReader_ListActiveByScope_WithPortofolioFilter(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	portoID := uuid.New()
	counterpartyID := uuid.New()
	nasabahID := uuid.New()

	cols := []string{"id", "klasifikasi_psak71", "tipe_instrumen", "status", "workflow_status",
		"flag_poci", "counterparty_id", "nasabah_id", "portofolio_id", "tenant_id"}
	mock.ExpectQuery(`SELECT id, klasifikasi_psak71`).
		WithArgs(portoID).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			instrID, "AC", "OBLIGASI", "AKTIF", "APPROVED",
			false, counterpartyID, nasabahID, &portoID, "TUGURE",
		))

	reader := NewDBInstrumenReader(db)
	scope := &BulkScope{PortofolioIDs: []uuid.UUID{portoID}}
	result, err := reader.ListActiveByScope(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListActiveByScope filtered: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("want 1 item, got %d", len(result))
	}
}

func TestDBInstrumenReader_ListActiveByScope_WithInstrumenFilter(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	counterpartyID := uuid.New()
	nasabahID := uuid.New()

	cols := []string{"id", "klasifikasi_psak71", "tipe_instrumen", "status", "workflow_status",
		"flag_poci", "counterparty_id", "nasabah_id", "portofolio_id", "tenant_id"}
	mock.ExpectQuery(`SELECT id, klasifikasi_psak71`).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			instrID, "FVTPL", "SAHAM", "AKTIF", "APPROVED",
			false, counterpartyID, nasabahID, nil, "TUGURE",
		))

	reader := NewDBInstrumenReader(db)
	scope := &BulkScope{InstrumenIDs: []uuid.UUID{instrID}}
	result, err := reader.ListActiveByScope(context.Background(), scope)
	if err != nil {
		t.Fatalf("ListActiveByScope instrumen filter: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("want 1 item, got %d", len(result))
	}
	if result[0].TipeInstrumen != "SAHAM" {
		t.Errorf("TipeInstrumen: want SAHAM, got %s", result[0].TipeInstrumen)
	}
}

// ─── DBBobotRepo ──────────────────────────────────────────────────────────────

func TestNewDBBobotRepo_PanicsOnNilDB(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	_ = NewDBBobotRepo(nil)
}

func TestDBBobotRepo_GetActiveBobot_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cols := []string{"skenario", "bobot"}
	mock.ExpectQuery(`SELECT skenario, bobot`).
		WithArgs("JUNI-2026").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("GOOD", "0.2500").
			AddRow("NORMAL", "0.5000").
			AddRow("BAD", "0.2500"))

	repo := NewDBBobotRepo(db)
	bobot, err := repo.GetActiveBobot(context.Background(), "JUNI-2026")
	if err != nil {
		t.Fatalf("GetActiveBobot: %v", err)
	}
	if !bobot.Good.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("Good: want 0.25, got %s", bobot.Good)
	}
	if !bobot.Normal.Equal(decimal.NewFromFloat(0.50)) {
		t.Errorf("Normal: want 0.50, got %s", bobot.Normal)
	}
	if !bobot.Bad.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("Bad: want 0.25, got %s", bobot.Bad)
	}
}

func TestDBBobotRepo_GetActiveBobot_FallbackOnNoRows(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Only 2 rows returned (incomplete set) → fallback to defaults.
	cols := []string{"skenario", "bobot"}
	mock.ExpectQuery(`SELECT skenario, bobot`).
		WithArgs("JUNI-2026").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("GOOD", "0.3000").
			AddRow("BAD", "0.3000"))

	repo := NewDBBobotRepo(db)
	bobot, err := repo.GetActiveBobot(context.Background(), "JUNI-2026")
	if err != nil {
		t.Fatalf("GetActiveBobot fallback: %v", err)
	}
	// Must fall back to DEC-010 defaults.
	if !bobot.Good.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("Good fallback: want 0.25, got %s", bobot.Good)
	}
	if !bobot.Normal.Equal(decimal.NewFromFloat(0.50)) {
		t.Errorf("Normal fallback: want 0.50, got %s", bobot.Normal)
	}
}

func TestDBBobotRepo_GetActiveBobot_FallbackOnDBError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// DB error → fallback to defaults (non-fatal for bobot lookup).
	mock.ExpectQuery(`SELECT skenario, bobot`).
		WithArgs("JUNI-2026").
		WillReturnError(sql.ErrConnDone)

	repo := NewDBBobotRepo(db)
	bobot, err := repo.GetActiveBobot(context.Background(), "JUNI-2026")
	if err != nil {
		t.Fatalf("GetActiveBobot DB error: expected nil error (fallback), got %v", err)
	}
	// Must fall back to DEC-010 defaults.
	if !bobot.Good.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("Good DB error fallback: want 0.25, got %s", bobot.Good)
	}
}

// ─── defaultBobotFallback ─────────────────────────────────────────────────────

func TestDefaultBobotFallback(t *testing.T) {
	t.Parallel()
	b := defaultBobotFallback()
	sum := b.Good.Add(b.Normal).Add(b.Bad)
	if !sum.Equal(decimal.NewFromInt(1)) {
		t.Errorf("defaultBobotFallback: sum must be 1, got %s", sum)
	}
}
