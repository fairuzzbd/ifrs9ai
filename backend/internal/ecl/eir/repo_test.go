// Package eir — repository tests using go-sqlmock (no live PostgreSQL required).
//
// All NUMERIC columns are handled via ::text to avoid float64 (DEC-016).
// DB transaction tests use mock.ExpectBegin / ExpectCommit / ExpectExec.
//
// References: repo.go, DEC-016, DEC-018.
package eir

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Constructor tests ────────────────────────────────────────────────────────

func TestNewDBEIRScheduleRepo_NotNil(t *testing.T) {
	db, _ := newMockDB(t)
	defer db.Close()
	if NewDBEIRScheduleRepo(db) == nil {
		t.Fatal("expected non-nil")
	}
}

func TestNewDBInstrumenEIRRepo_NotNil(t *testing.T) {
	db, _ := newMockDB(t)
	defer db.Close()
	if NewDBInstrumenEIRRepo(db) == nil {
		t.Fatal("expected non-nil")
	}
}

func TestNewDBAmendmentRepo_NotNil(t *testing.T) {
	db, _ := newMockDB(t)
	defer db.Close()
	if NewDBAmendmentRepo(db) == nil {
		t.Fatal("expected non-nil")
	}
}

// ─── DBEIRScheduleRepo.InsertBatch ───────────────────────────────────────────

func TestDBEIRScheduleRepo_InsertBatch_NilSlice_NoOp(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	repo := NewDBEIRScheduleRepo(db)
	// Empty batch returns immediately without any SQL.
	err := repo.InsertBatch(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("InsertBatch nil: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBEIRScheduleRepo_InsertBatch_EmptySlice_NoOp(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	repo := NewDBEIRScheduleRepo(db)
	err := repo.InsertBatch(context.Background(), nil, []ScheduleRow{})
	if err != nil {
		t.Fatalf("InsertBatch empty slice: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBEIRScheduleRepo_InsertBatch_OneRow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.eir_amortization_schedule")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewDBEIRScheduleRepo(db)
	row := ScheduleRow{
		ID:                 uuid.New(),
		InstrumenID:        uuid.New(),
		PeriodeSeq:         1,
		TanggalPosting:     date(2026, 7, 1),
		OpeningCarrying:    mustDec("1005000000.0000"),
		CashInflow:         mustDec("40000000.0000"),
		PendapatanBungaEIR: mustDec("40200000.0000"),
		AmortisasiPD:       mustDec("200000.0000"),
		PelunasanPokok:     decimal.Zero,
		ClosingCarrying:    mustDec("1005200000.0000"),
		EIRPeriode:         mustDec("0.04000000"),
		StageSaatPosting:   "STAGE_1",
		StatusPosting:      "PROYEKSI",
		FlagPOCI:           false,
		CreatedBy:          uuid.New(),
		UpdatedBy:          uuid.New(),
		TenantID:           "TUGURE",
	}

	if err := repo.InsertBatch(context.Background(), tx, []ScheduleRow{row}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBEIRScheduleRepo_InsertBatch_MultiRow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.eir_amortization_schedule")).
		WillReturnResult(sqlmock.NewResult(2, 2))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBEIRScheduleRepo(db)

	rows := make([]ScheduleRow, 2)
	for i := range rows {
		rows[i] = ScheduleRow{
			ID:                 uuid.New(),
			InstrumenID:        uuid.New(),
			PeriodeSeq:         i + 1,
			TanggalPosting:     date(2026, 7+i, 1),
			OpeningCarrying:    mustDec("1000000000.0000"),
			CashInflow:         mustDec("40000000.0000"),
			PendapatanBungaEIR: mustDec("40000000.0000"),
			AmortisasiPD:       decimal.Zero,
			PelunasanPokok:     decimal.Zero,
			ClosingCarrying:    mustDec("1000000000.0000"),
			EIRPeriode:         mustDec("0.04000000"),
			StageSaatPosting:   "STAGE_1",
			StatusPosting:      "PROYEKSI",
			FlagPOCI:           false,
			CreatedBy:          uuid.New(),
			UpdatedBy:          uuid.New(),
			TenantID:           "TUGURE",
		}
	}

	if err := repo.InsertBatch(context.Background(), tx, rows); err != nil {
		t.Fatalf("InsertBatch multi: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBEIRScheduleRepo.MarkSuperseded ────────────────────────────────────────

func TestDBEIRScheduleRepo_MarkSuperseded(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	updatedBy := uuid.New()

	mock.ExpectBegin()
	// UPDATE ecl.eir_amortization_schedule SET recomputed_from_seq=$1, updated_by=$2 WHERE instrumen_id=$3 ...
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_amortization_schedule")).
		WithArgs(11, updatedBy, instrID).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBEIRScheduleRepo(db)

	err := repo.MarkSuperseded(context.Background(), tx, instrID, 11, updatedBy)
	if err != nil {
		t.Fatalf("MarkSuperseded: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBEIRScheduleRepo.GetMaxPeriodeSeq ──────────────────────────────────────

func TestDBEIRScheduleRepo_GetMaxPeriodeSeq_Null(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT MAX(periode_seq)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))

	repo := NewDBEIRScheduleRepo(db)
	maxSeq, err := repo.GetMaxPeriodeSeq(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetMaxPeriodeSeq null: %v", err)
	}
	if maxSeq != 0 {
		t.Errorf("expected 0 when null, got %d", maxSeq)
	}
}

func TestDBEIRScheduleRepo_GetMaxPeriodeSeq_WithValue(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT MAX(periode_seq)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(10))

	repo := NewDBEIRScheduleRepo(db)
	maxSeq, err := repo.GetMaxPeriodeSeq(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetMaxPeriodeSeq: %v", err)
	}
	if maxSeq != 10 {
		t.Errorf("expected 10, got %d", maxSeq)
	}
}

// ─── DBEIRScheduleRepo.HasActiveRows ─────────────────────────────────────────

func TestDBEIRScheduleRepo_HasActiveRows_Zero(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewDBEIRScheduleRepo(db)
	has, err := repo.HasActiveRows(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("HasActiveRows: %v", err)
	}
	if has {
		t.Error("expected false for count=0")
	}
}

func TestDBEIRScheduleRepo_HasActiveRows_NonZero(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	repo := NewDBEIRScheduleRepo(db)
	has, err := repo.HasActiveRows(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("HasActiveRows: %v", err)
	}
	if !has {
		t.Error("expected true for count=3")
	}
}

// ─── DBEIRScheduleRepo.GetActiveByPeriode ────────────────────────────────────

func TestDBEIRScheduleRepo_GetActiveByPeriode_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, instrumen_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	rows, err := repo.GetActiveByPeriode(context.Background(), uuid.New(), 0)
	if err != nil {
		t.Fatalf("GetActiveByPeriode: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestDBEIRScheduleRepo_GetActiveByPeriode_WithPeriodFilter(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	// periodSeqFilter > 0 adds second arg ($2)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, instrumen_id")).
		WithArgs(sqlmock.AnyArg(), 5).
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	_, err := repo.GetActiveByPeriode(context.Background(), uuid.New(), 5)
	if err != nil {
		t.Fatalf("GetActiveByPeriode with filter: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBEIRScheduleRepo_GetActiveByPeriode_WithRow(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	rowID := uuid.New()
	actor := uuid.New()
	now := time.Now()

	colRows := sqlmock.NewRows(scheduleSelectCols()).AddRow(
		rowID,
		instrID,
		1,
		now,
		"1005000000.0000", // opening_carrying
		"40000000.0000",   // cash_inflow
		"40200000.0000",   // pendapatan_bunga_eir
		"200000.0000",     // amortisasi_p_d
		"0.0000",          // pelunasan_pokok
		"1005200000.0000", // closing_carrying
		"0.04000000",      // eir_periode
		"STAGE_1",
		"PROYEKSI",
		false,
		nil,   // recomputed_from_seq NULL
		now,   // created_at
		actor, // created_by
		now,   // updated_at
		actor, // updated_by
		nil,   // deleted_at
		"TUGURE",
		int64(1),
	)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, instrumen_id")).
		WithArgs(instrID).
		WillReturnRows(colRows)

	repo := NewDBEIRScheduleRepo(db)
	rows, err := repo.GetActiveByPeriode(context.Background(), instrID, 0)
	if err != nil {
		t.Fatalf("GetActiveByPeriode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].OpeningCarrying.Equal(mustDec("1005000000.0000")) {
		t.Errorf("opening_carrying mismatch: %s", rows[0].OpeningCarrying)
	}
	if rows[0].StageSaatPosting != "STAGE_1" {
		t.Errorf("stage mismatch: %s", rows[0].StageSaatPosting)
	}
	if rows[0].RecomputedFromSeq != nil {
		t.Errorf("expected nil recomputed_from_seq")
	}
}

func TestDBEIRScheduleRepo_GetActiveByPeriode_WithRecomputedFromSeq(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	now := time.Now()
	recomputedSeq := 5

	colRows := sqlmock.NewRows(scheduleSelectCols()).AddRow(
		uuid.New(), instrID, 1, now,
		"1000000000.0000", "40000000.0000", "40000000.0000", "0.0000",
		"0.0000", "1000000000.0000", "0.04000000",
		"STAGE_1", "PROYEKSI", false,
		&recomputedSeq, // non-nil recomputed_from_seq
		now, uuid.New(), now, uuid.New(), nil, "TUGURE", int64(1),
	)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, instrumen_id")).
		WithArgs(instrID).
		WillReturnRows(colRows)

	repo := NewDBEIRScheduleRepo(db)
	rows, err := repo.GetActiveByPeriode(context.Background(), instrID, 0)
	if err != nil {
		t.Fatalf("GetActiveByPeriode recomputed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row")
	}
	if rows[0].RecomputedFromSeq == nil || *rows[0].RecomputedFromSeq != 5 {
		t.Errorf("expected RecomputedFromSeq=5, got %v", rows[0].RecomputedFromSeq)
	}
}

// ─── DBEIRScheduleRepo.List ───────────────────────────────────────────────────

func TestDBEIRScheduleRepo_List_DefaultLimit(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()

	// List builds dynamic SQL; match on partial prefix only
	mock.ExpectQuery("SELECT id, instrumen_id").
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	rows, meta, err := repo.List(context.Background(), instrID, listquery.Query{}, false, "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows")
	}
	if meta == nil {
		t.Fatal("meta must not be nil")
	}
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
}

func TestDBEIRScheduleRepo_List_LimitClamped(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT id, instrumen_id").
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	// limit=-1 → clamped to 50
	_, meta, err := repo.List(context.Background(), uuid.New(), listquery.Query{}, false, "", -1)
	if err != nil {
		t.Fatalf("List clamped: %v", err)
	}
	if meta.Limit != 50 {
		t.Errorf("expected limit=50 when -1, got %d", meta.Limit)
	}
}

func TestDBEIRScheduleRepo_List_IncludeSuperseded(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT id, instrumen_id").
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	_, meta, err := repo.List(context.Background(), uuid.New(), listquery.Query{}, true, "", 10)
	if err != nil {
		t.Fatalf("List includeSuperseded: %v", err)
	}
	if meta == nil {
		t.Fatal("meta nil")
	}
}

func TestDBEIRScheduleRepo_List_WithCursor(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT id, instrumen_id").
		WillReturnRows(sqlmock.NewRows(scheduleSelectCols()))

	repo := NewDBEIRScheduleRepo(db)
	cursor := encodeCursorStr("3")
	_, _, err := repo.List(context.Background(), uuid.New(), listquery.Query{}, false, cursor, 10)
	if err != nil {
		t.Fatalf("List cursor: %v", err)
	}
}

// ─── DBInstrumenEIRRepo.GetByID ───────────────────────────────────────────────

func TestDBInstrumenEIRRepo_GetByID_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, kode_instrumen")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(instrumenSelectCols()))

	repo := NewDBInstrumenEIRRepo(db)
	inst, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if inst != nil {
		t.Error("expected nil when no rows")
	}
}

func TestDBInstrumenEIRRepo_GetByID_Found_AC(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	id := uuid.New()
	eirAwal := "0.08028915"
	kupon := "0.08000000"
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, kode_instrumen")).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(instrumenSelectCols()).AddRow(
			id,
			"OBL-TEST-001",
			"AC",
			true,
			eirAwal,           // eir_awal::text (non-null string)
			false,             // flag_poci
			"1000000000.0000", // nominal
			"5000000.0000",    // biaya_transaksi_capitalized
			kupon,             // kupon::text
			now,               // tanggal_penempatan
			now,               // tanggal_jatuh_tempo
			"ACTIVE",
			nil, // deleted_at
			"TUGURE",
		))

	repo := NewDBInstrumenEIRRepo(db)
	inst, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if inst == nil {
		t.Fatal("expected instrument, got nil")
	}
	if inst.KlasifikasiPsak71 != "AC" {
		t.Errorf("klasifikasi: got %s", inst.KlasifikasiPsak71)
	}
	if inst.EIRAwal == nil {
		t.Fatal("EIRAwal must be set")
	}
	if !inst.EIRAwal.Equal(mustDec("0.08028915")) {
		t.Errorf("EIRAwal: %s", inst.EIRAwal.String())
	}
	if inst.Kupon == nil {
		t.Fatal("Kupon must be set")
	}
}

func TestDBInstrumenEIRRepo_GetByID_Found_NullEIRAwal(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	id := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, kode_instrumen")).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(instrumenSelectCols()).AddRow(
			id, "OBL-NULL-EIR", "AC", true,
			nil, // eir_awal NULL
			false,
			"1000000000.0000", "0.0000",
			nil, // kupon NULL
			now, now,
			"ACTIVE", nil, "TUGURE",
		))

	repo := NewDBInstrumenEIRRepo(db)
	inst, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID null EIRAwal: %v", err)
	}
	if inst == nil {
		t.Fatal("expected instrument")
	}
	if inst.EIRAwal != nil {
		t.Errorf("expected EIRAwal nil, got %s", inst.EIRAwal.String())
	}
	if inst.Kupon != nil {
		t.Errorf("expected Kupon nil")
	}
}

// ─── DBInstrumenEIRRepo.UpdateEIRAwal ─────────────────────────────────────────

func TestDBInstrumenEIRRepo_UpdateEIRAwal(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE mst.instrumen")).
		WithArgs("0.08028915", actor, instrID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBInstrumenEIRRepo(db)

	err := repo.UpdateEIRAwal(context.Background(), tx, instrID, mustDec("0.08028915"), actor)
	if err != nil {
		t.Fatalf("UpdateEIRAwal: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBAmendmentRepo.Create ───────────────────────────────────────────────────

func TestDBAmendmentRepo_Create(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	makerID := uuid.New()
	eirLama := mustDec("0.08000000")
	proposal := &AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		Status:              AmendStatusPendingReview,
		TanggalAmandemen:    date(2026, 6, 1),
		TanggalReEstimasi:   time.Now(),
		AlasanAmandemen:     "test amendment",
		EIRLama:             &eirLama,
		RevisedCashflowJSON: `[{"date":"2026-01-01","amount_idr":"-1005000000.0000"}]`,
		MakerID:             &makerID,
		TenantID:            "TUGURE",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreatedBy:           makerID,
		UpdatedBy:           makerID,
	}

	if err := repo.Create(context.Background(), tx, proposal); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBAmendmentRepo_Create_NilEIRLama(t *testing.T) {
	// EIRLama = nil → should use "0.00000000" default
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	makerID := uuid.New()
	proposal := &AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		Status:              AmendStatusPendingReview,
		TanggalAmandemen:    date(2026, 6, 1),
		TanggalReEstimasi:   time.Now(),
		EIRLama:             nil, // covers nil branch in Create
		RevisedCashflowJSON: `[]`,
		MakerID:             &makerID,
		TenantID:            "TUGURE",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreatedBy:           makerID,
		UpdatedBy:           makerID,
	}

	if err := repo.Create(context.Background(), tx, proposal); err != nil {
		t.Fatalf("Create nil EIRLama: %v", err)
	}
	tx.Commit()
}

func TestDBAmendmentRepo_Create_WithCarryingAndDokumen(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	makerID := uuid.New()
	dokID := uuid.New()
	eirLama := mustDec("0.08")
	carryingPre := mustDec("990000000.0000")
	carryingPost := mustDec("985000000.0000")

	proposal := &AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		Status:              AmendStatusPendingReview,
		TanggalAmandemen:    date(2026, 6, 1),
		TanggalReEstimasi:   time.Now(),
		EIRLama:             &eirLama,
		CarryingSebelum:     &carryingPre,
		CarryingSesudah:     &carryingPost,
		DokumenPendukungID:  &dokID,
		RevisedCashflowJSON: `[]`,
		MakerID:             &makerID,
		TenantID:            "TUGURE",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreatedBy:           makerID,
		UpdatedBy:           makerID,
	}

	if err := repo.Create(context.Background(), tx, proposal); err != nil {
		t.Fatalf("Create with carrying: %v", err)
	}
	tx.Commit()
}

// ─── DBAmendmentRepo.Update ───────────────────────────────────────────────────

func TestDBAmendmentRepo_Update(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	makerID := uuid.New()
	reviewerID := uuid.New()
	eirLama := mustDec("0.08")
	comment := "reviewed"
	sig := "sha256:abc123"
	now := time.Now()

	proposal := &AmendmentProposal{
		ID:                    uuid.New(),
		InstrumenID:           uuid.New(),
		Status:                AmendStatusPendingApproval,
		TanggalAmandemen:      date(2026, 6, 1),
		TanggalReEstimasi:     time.Now(),
		EIRLama:               &eirLama,
		RevisedCashflowJSON:   `[]`,
		MakerID:               &makerID,
		ReviewerID:            &reviewerID,
		ReviewerComment:       &comment,
		ReviewerSignatureHash: &sig,
		ReviewedAt:            &now,
		TenantID:              "TUGURE",
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		CreatedBy:             makerID,
		UpdatedBy:             reviewerID,
		RowVersion:            1,
	}

	if err := repo.Update(context.Background(), tx, proposal); err != nil {
		t.Fatalf("Update: %v", err)
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBAmendmentRepo_Update_Approved_WithEIRBaru(t *testing.T) {
	// Covers eirBaru and catchUp non-nil branches in Update
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBAmendmentRepo(db)

	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirLama := mustDec("0.08")
	eirBaru := mustDec("0.09")
	catchUp := mustDec("500000.0000")
	comment := "approved"
	sig := "sha256:def456"
	now := time.Now()

	proposal := &AmendmentProposal{
		ID:                    uuid.New(),
		InstrumenID:           uuid.New(),
		Status:                AmendStatusApproved,
		TanggalAmandemen:      date(2026, 6, 1),
		TanggalReEstimasi:     time.Now(),
		EIRLama:               &eirLama,
		EIRBaru:               &eirBaru,
		CatchUpAdjustment:     &catchUp,
		RevisedCashflowJSON:   `[]`,
		MakerID:               &makerID,
		ReviewerID:            &reviewerID,
		ApproverID:            &approverID,
		ApproverComment:       &comment,
		ApproverSignatureHash: &sig,
		ApprovedAt:            &now,
		TenantID:              "TUGURE",
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		CreatedBy:             makerID,
		UpdatedBy:             approverID,
		RowVersion:            2,
	}

	if err := repo.Update(context.Background(), tx, proposal); err != nil {
		t.Fatalf("Update approved: %v", err)
	}
	tx.Commit()
}

// ─── DBAmendmentRepo.GetByID ──────────────────────────────────────────────────

func TestDBAmendmentRepo_GetByID_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()))

	repo := NewDBAmendmentRepo(db)
	p, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p != nil {
		t.Error("expected nil for not found")
	}
}

func TestDBAmendmentRepo_GetByID_Found_PendingReview(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	proposalID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(proposalID).
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()).AddRow(
			proposalID,
			instrID,
			now,  // tanggal_re_estimation
			`[]`, // modifikasi_terms_json
			"PENDING_REVIEW",
			"0.08000000",  // eir_sebelum::text
			nil,           // eir_sesudah (null)
			nil,           // catch_up_adjustment (null)
			makerID,       // maker_id
			nil,           // reviewer_id
			nil,           // approver_id
			nil, nil, nil, // reviewer/approver comment, reject reason
			nil, nil, // sig hashes
			nil, nil, // approved_at, rejected_at
			nil, // dokumen_pendukung_id
			now, makerID, now, makerID,
			"TUGURE",
			int64(1),
			// M6 columns
			nil, nil, nil, // cancelled_at, cancel_reason, cancelled_by
			nil, nil, nil, // trigger_source, drift_report_id, document_id
		))

	repo := NewDBAmendmentRepo(db)
	p, err := repo.GetByID(context.Background(), proposalID)
	if err != nil {
		t.Fatalf("GetByID found: %v", err)
	}
	if p == nil {
		t.Fatal("expected proposal, got nil")
	}
	if p.Status != AmendStatusPendingReview {
		t.Errorf("status: got %s", p.Status)
	}
	if p.EIRLama == nil {
		t.Fatal("EIRLama must be set")
	}
	if !p.EIRLama.Equal(mustDec("0.08000000")) {
		t.Errorf("EIRLama: %s", p.EIRLama.String())
	}
	if p.EIRBaru != nil {
		t.Error("EIRBaru should be nil")
	}
}

func TestDBAmendmentRepo_GetByID_Found_Approved_WithEIRBaru(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	proposalID := uuid.New()
	instrID := uuid.New()
	makerID := uuid.New()
	approverID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).
		WithArgs(proposalID).
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()).AddRow(
			proposalID, instrID, now,
			`[]`,
			"APPROVED",
			"0.08000000",
			"0.09000000",  // eir_sesudah
			"500000.0000", // catch_up_adjustment
			makerID, nil, &approverID,
			nil, nil, nil,
			nil, nil,
			&now, nil, nil,
			now, makerID, now, approverID,
			"TUGURE", int64(2),
			// M6 columns
			nil, nil, nil, // cancelled_at, cancel_reason, cancelled_by
			nil, nil, nil, // trigger_source, drift_report_id, document_id
		))

	repo := NewDBAmendmentRepo(db)
	p, err := repo.GetByID(context.Background(), proposalID)
	if err != nil {
		t.Fatalf("GetByID approved: %v", err)
	}
	if p == nil {
		t.Fatal("expected proposal")
	}
	if p.EIRBaru == nil {
		t.Fatal("EIRBaru should be set")
	}
	if !p.EIRBaru.Equal(mustDec("0.09000000")) {
		t.Errorf("EIRBaru: %s", p.EIRBaru.String())
	}
	if p.CatchUpAdjustment == nil {
		t.Fatal("CatchUpAdjustment should be set")
	}
	if !p.CatchUpAdjustment.Equal(mustDec("500000.0000")) {
		t.Errorf("CatchUpAdjustment: %s", p.CatchUpAdjustment.String())
	}
}

// ─── DBAmendmentRepo.HasActiveProposal ───────────────────────────────────────

func TestDBAmendmentRepo_HasActiveProposal_False(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewDBAmendmentRepo(db)
	has, err := repo.HasActiveProposal(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("HasActiveProposal: %v", err)
	}
	if has {
		t.Error("expected false")
	}
}

func TestDBAmendmentRepo_HasActiveProposal_True(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	repo := NewDBAmendmentRepo(db)
	has, err := repo.HasActiveProposal(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("HasActiveProposal: %v", err)
	}
	if !has {
		t.Error("expected true")
	}
}

// ─── DBAmendmentRepo.List ─────────────────────────────────────────────────────

func TestDBAmendmentRepo_List_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()))

	repo := NewDBAmendmentRepo(db)
	proposals, meta, err := repo.List(context.Background(), listquery.Query{}, "", 50, uuid.New(), false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0, got %d", len(proposals))
	}
	if meta == nil {
		t.Fatal("meta must not be nil")
	}
	if meta.HasMore {
		t.Error("hasMore should be false")
	}
}

func TestDBAmendmentRepo_List_Admin_NoMakerFilter(t *testing.T) {
	// isAdmin=true → no maker_id filter in WHERE
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()))

	repo := NewDBAmendmentRepo(db)
	_, meta, err := repo.List(context.Background(), listquery.Query{}, "", 50, uuid.New(), true)
	if err != nil {
		t.Fatalf("List admin: %v", err)
	}
	if meta == nil {
		t.Error("meta must not be nil")
	}
}

func TestDBAmendmentRepo_List_LimitClamped(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()))

	repo := NewDBAmendmentRepo(db)
	_, meta, err := repo.List(context.Background(), listquery.Query{}, "", 9999, uuid.New(), false)
	if err != nil {
		t.Fatalf("List limit: %v", err)
	}
	if meta.Limit != 50 {
		t.Errorf("expected limit=50 for 9999, got %d", meta.Limit)
	}
}

func TestDBAmendmentRepo_List_WithCursor(t *testing.T) {
	// Non-empty cursor triggers the cursor decode branch in List
	db, mock := newMockDB(t)
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(amendmentSelectCols()))

	repo := NewDBAmendmentRepo(db)
	cursor := encodeCursorStr("2026-06-01T10:00:00Z")
	_, _, err := repo.List(context.Background(), listquery.Query{}, cursor, 10, uuid.New(), false)
	if err != nil {
		t.Fatalf("List cursor: %v", err)
	}
}

// ─── Column helpers for sqlmock ───────────────────────────────────────────────

// scheduleSelectCols returns column names matching scanScheduleRows order.
func scheduleSelectCols() []string {
	return []string{
		"id", "instrumen_id", "periode_seq", "tanggal_posting",
		"opening_carrying", "cash_inflow",
		"pendapatan_bunga_eir", "amortisasi_p_d",
		"pelunasan_pokok", "closing_carrying",
		"eir_periode", "stage_saat_posting", "status_posting",
		"flag_poci", "recomputed_from_seq",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "tenant_id", "row_version",
	}
}

// instrumenSelectCols returns column names matching scanInstrumenForEIR order.
func instrumenSelectCols() []string {
	return []string{
		"id", "kode_instrumen", "klasifikasi_psak71", "eir_method_flag",
		"eir_awal", "flag_poci",
		"nominal", "biaya_transaksi_capitalized",
		"kupon", "tanggal_penempatan", "tanggal_jatuh_tempo",
		"status", "deleted_at", "tenant_id",
	}
}

// amendmentSelectCols returns column names matching scanAmendmentRow order.
func amendmentSelectCols() []string {
	return []string{
		"id", "instrumen_id", "tanggal_re_estimation",
		"modifikasi_terms_json", "workflow_status",
		"eir_sebelum", "eir_sesudah", "catch_up_adjustment",
		"maker_id", "reviewer_id", "approver_id",
		"reviewer_comment", "approver_comment", "reject_reason",
		"reviewer_signature_hash", "approver_signature_hash",
		"approved_at", "rejected_at",
		"dokumen_pendukung_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"tenant_id", "row_version",
		// M6 additions (migration 000027)
		"cancelled_at", "cancel_reason", "cancelled_by",
		"trigger_source", "drift_report_id", "document_id",
	}
}

// Suppress unused import.
var _ = decimal.Zero

// ─── DBEIRScheduleRepo.GetGrossCarryingAtDate ────────────────────────────────

// TestDBEIRScheduleRepo_GetGrossCarryingAtDate_HappyPath verifies that the
// closing_carrying from the latest active row on or before asOf is returned.
// NUMERIC read via ::text to avoid float64 (DEC-016).
func TestDBEIRScheduleRepo_GetGrossCarryingAtDate_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	asOf := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	expectedCarrying := "1005000000.0000"

	mock.ExpectQuery(regexp.QuoteMeta("SELECT closing_carrying::text")).
		WithArgs(instrID, asOf).
		WillReturnRows(sqlmock.NewRows([]string{"closing_carrying"}).AddRow(expectedCarrying))

	repo := NewDBEIRScheduleRepo(db)
	got, err := repo.GetGrossCarryingAtDate(context.Background(), instrID, asOf)
	if err != nil {
		t.Fatalf("GetGrossCarryingAtDate: %v", err)
	}

	want, _ := decimal.NewFromString(expectedCarrying)
	if !got.Equal(want) {
		t.Errorf("want %s, got %s", want.StringFixed(4), got.StringFixed(4))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBEIRScheduleRepo_GetGrossCarryingAtDate_NoRows verifies that ErrEIRScheduleNotFound
// is returned when no qualifying row exists (e.g., no prior schedule).
func TestDBEIRScheduleRepo_GetGrossCarryingAtDate_NoRows(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	instrID := uuid.New()
	asOf := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT closing_carrying::text")).
		WithArgs(instrID, asOf).
		WillReturnRows(sqlmock.NewRows([]string{"closing_carrying"})) // empty result set

	repo := NewDBEIRScheduleRepo(db)
	_, err := repo.GetGrossCarryingAtDate(context.Background(), instrID, asOf)
	if err == nil {
		t.Fatal("expected ErrEIRScheduleNotFound, got nil")
	}

	de, ok := err.(interface {
		Code() interface{ String() string }
	})
	_ = de
	_ = ok
	// Verify it's a domain error with the schedule-not-found code
	assertDomainErr(t, err, CodeEIRScheduleNotFound)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}
