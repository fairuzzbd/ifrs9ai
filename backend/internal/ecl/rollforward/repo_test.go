package rollforward_test

// repo_test.go — unit tests for rollforward.Repo using go-sqlmock.
//
// Tests verify that:
//  - GetResultLinesByCalcRun returns correct POCI handling (nil EclWeightedIdr)
//  - GetCalcRunStatus returns status+periodeID correctly, and (false) on ErrNoRows
//  - GetSealedCalcRunsByPeriode returns ordered SEALED runs
//  - GetECLByStageForCalcRun returns correct per-stage aggregates
//  - GetStageHistoryForCalcRun returns DISTINCT ON latest entry per instrument
//  - GetInstrumenStatusByIDs handles empty slice (no DB call)
//  - GetPortofolioNama returns (name, true, nil) when found, ("", false, nil) on ErrNoRows
//  - GetPortofolioInstruments returns instrument IDs

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

func setupMock(t *testing.T) (sqlmock.Sqlmock, *rollforward.Repo) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	repo := rollforward.NewRepo(db)
	return mock, repo
}

// ─── GetResultLinesByCalcRun ─────────────────────────────────────────────────

func TestGetResultLinesByCalcRun_NormalAndPOCI(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()
	instrID1, instrID2 := uuid.New(), uuid.New()

	rows := sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"}).
		AddRow(instrID1, 1, "100000.0000", "1000000.0000").
		AddRow(instrID2, 2, nil, "2000000.0000") // POCI: NULL ecl_weighted_idr

	mock.ExpectQuery(`SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr`).
		WithArgs(calcRunID).
		WillReturnRows(rows)

	ctx := context.Background()
	lines, err := repo.GetResultLinesByCalcRun(ctx, calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	// First line: ecl_weighted_idr populated
	if lines[0].EclWeightedIdr == nil {
		t.Error("line[0] EclWeightedIdr should not be nil")
	}
	// Second line: POCI (null)
	if lines[1].EclWeightedIdr != nil {
		t.Error("line[1] EclWeightedIdr should be nil (POCI)")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

func TestGetResultLinesByCalcRun_Empty(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()

	mock.ExpectQuery(`SELECT instrumen_id, stage, ecl_weighted_idr, ead_idr`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"}))

	lines, err := repo.GetResultLinesByCalcRun(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("want 0 lines, got %d", len(lines))
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetCalcRunStatus ────────────────────────────────────────────────────────

func TestGetCalcRunStatus_Found(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()

	rows := sqlmock.NewRows([]string{"status", "periode_id"}).
		AddRow("SEALED", "JUNI-2026")
	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(calcRunID).
		WillReturnRows(rows)

	status, periodeID, found, err := repo.GetCalcRunStatus(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if status != "SEALED" {
		t.Errorf("want SEALED, got %s", status)
	}
	if periodeID != "JUNI-2026" {
		t.Errorf("want JUNI-2026, got %s", periodeID)
	}
	_ = mock.ExpectationsWereMet()
}

func TestGetCalcRunStatus_NotFound(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()

	mock.ExpectQuery(`SELECT status, periode_id FROM ecl.calc_run`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "periode_id"}))

	_, _, found, err := repo.GetCalcRunStatus(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetSealedCalcRunsByPeriode ──────────────────────────────────────────────

func TestGetSealedCalcRunsByPeriode_ReturnsTwoRuns(t *testing.T) {
	mock, repo := setupMock(t)
	id1, id2 := uuid.New(), uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{"id", "periode_id", "status", "sealed_at", "tenant_id"}).
		AddRow(id1, "MEI-2026", "SEALED", now, "TUGURE").
		AddRow(id2, "JUNI-2026", "SEALED", now, "TUGURE")

	mock.ExpectQuery(`SELECT id, periode_id, status, sealed_at, tenant_id`).
		WithArgs(12).
		WillReturnRows(rows)

	runs, err := repo.GetSealedCalcRunsByPeriode(context.Background(), 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(runs))
	}
	if runs[0].PeriodeID != "MEI-2026" {
		t.Errorf("want MEI-2026, got %s", runs[0].PeriodeID)
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetECLByStageForCalcRun ─────────────────────────────────────────────────

func TestGetECLByStageForCalcRun_AllThreeStages(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()

	rows := sqlmock.NewRows([]string{"stage", "coalesce"}).
		AddRow(1, "1000000.0000").
		AddRow(2, "5000000.0000").
		AddRow(3, "2000000.0000")

	mock.ExpectQuery(`SELECT stage, COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(rows)

	ecl, err := repo.GetECLByStageForCalcRun(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s1 := ecl.Stage1.StringFixed(4)
	s2 := ecl.Stage2.StringFixed(4)
	s3 := ecl.Stage3.StringFixed(4)
	if s1 != "1000000.0000" {
		t.Errorf("Stage1: want 1000000.0000, got %s", s1)
	}
	if s2 != "5000000.0000" {
		t.Errorf("Stage2: want 5000000.0000, got %s", s2)
	}
	if s3 != "2000000.0000" {
		t.Errorf("Stage3: want 2000000.0000, got %s", s3)
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetStageHistoryForCalcRun ────────────────────────────────────────────────

func TestGetStageHistoryForCalcRun_ReturnsMap(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID := uuid.New()
	instrID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{"instrumen_id", "calc_run_id", "trigger_type", "created_at"}).
		AddRow(instrID, calcRunID, "SICR_RATING", now)

	mock.ExpectQuery(`SELECT DISTINCT ON`).
		WithArgs(calcRunID).
		WillReturnRows(rows)

	hist, err := repo.GetStageHistoryForCalcRun(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("want 1 entry, got %d", len(hist))
	}
	if hist[instrID].TriggerType != "SICR_RATING" {
		t.Errorf("want SICR_RATING, got %s", hist[instrID].TriggerType)
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetInstrumenStatusByIDs ─────────────────────────────────────────────────

func TestGetInstrumenStatusByIDs_EmptyInput_NoDB(t *testing.T) {
	mock, repo := setupMock(t)

	// No DB expectations should be set — empty input returns early.
	result, err := repo.GetInstrumenStatusByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("want 0 entries, got %d", len(result))
	}
	// Verify no unexpected queries were made.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB call: %v", err)
	}
}

func TestGetInstrumenStatusByIDs_SingleID(t *testing.T) {
	mock, repo := setupMock(t)
	instrID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{"id", "kode", "status", "tanggal_jatuh_tempo"}).
		AddRow(instrID, "BOND-001", "AKTIF", now)

	mock.ExpectQuery(`SELECT id, kode, status, tanggal_jatuh_tempo`).
		WithArgs(instrID).
		WillReturnRows(rows)

	result, err := repo.GetInstrumenStatusByIDs(context.Background(), []uuid.UUID{instrID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("want 1, got %d", len(result))
	}
	snap := result[instrID]
	if snap.Status != "AKTIF" {
		t.Errorf("want AKTIF, got %s", snap.Status)
	}
	if snap.TanggalJatuhTempo == nil {
		t.Error("TanggalJatuhTempo should not be nil")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetPortofolioNama ────────────────────────────────────────────────────────

func TestGetPortofolioNama_Found(t *testing.T) {
	mock, repo := setupMock(t)
	portID := uuid.New()

	rows := sqlmock.NewRows([]string{"nama"}).AddRow("Portofolio Obligasi IDR")
	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(rows)

	nama, found, err := repo.GetPortofolioNama(context.Background(), portID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if nama != "Portofolio Obligasi IDR" {
		t.Errorf("want 'Portofolio Obligasi IDR', got %s", nama)
	}
	_ = mock.ExpectationsWereMet()
}

func TestGetPortofolioNama_NotFound(t *testing.T) {
	mock, repo := setupMock(t)
	portID := uuid.New()

	mock.ExpectQuery(`SELECT nama FROM mst.portofolio`).
		WithArgs(portID).
		WillReturnRows(sqlmock.NewRows([]string{"nama"}))

	_, found, err := repo.GetPortofolioNama(context.Background(), portID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetPortofolioInstruments ────────────────────────────────────────────────

func TestGetPortofolioInstruments_ReturnsTwoIDs(t *testing.T) {
	mock, repo := setupMock(t)
	portID := uuid.New()
	id1, id2 := uuid.New(), uuid.New()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(id1).AddRow(id2)
	mock.ExpectQuery(`SELECT id FROM mst.instrumen`).
		WithArgs(portID).
		WillReturnRows(rows)

	ids, err := repo.GetPortofolioInstruments(context.Background(), portID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 IDs, got %d", len(ids))
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetResultLinesByCalcRunAndPortfolio ─────────────────────────────────────

func TestGetResultLinesByCalcRunAndPortfolio_ReturnsFiltered(t *testing.T) {
	mock, repo := setupMock(t)
	calcRunID, portID := uuid.New(), uuid.New()
	instrID := uuid.New()

	rows := sqlmock.NewRows([]string{"instrumen_id", "stage", "ecl_weighted_idr", "ead_idr"}).
		AddRow(instrID, 1, "500000.0000", "5000000.0000")

	mock.ExpectQuery(`FROM ecl.calc_result_line crl`).
		WithArgs(calcRunID, portID).
		WillReturnRows(rows)

	lines, err := repo.GetResultLinesByCalcRunAndPortfolio(context.Background(), calcRunID, portID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(lines))
	}
	if lines[0].Stage != 1 {
		t.Errorf("want stage 1, got %d", lines[0].Stage)
	}
	_ = mock.ExpectationsWereMet()
}
