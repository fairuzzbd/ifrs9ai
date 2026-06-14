// Package helpers — sqlmock DB-backed repository tests.
//
// These tests drive the real SQL execution paths in each repo using go-sqlmock.
// They do NOT test the nil-db early returns (those are covered in repo_test.go).
//
// Compliance: all NUMERIC columns returned as strings (no float64 intermediate).
package helpers

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// ─── DBPDRepository ───────────────────────────────────────────────────────────

func TestDBPDRepo_GetPefindoCurve_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT rating")).
		WithArgs("idAA", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{
			"rating", "pd_12month", "pd_lifetime_3y",
			"pd_lifetime_5y", "pd_lifetime_7y", "pd_lifetime_10y",
		}).AddRow("idAA", "0.00350000", "0.01200000", "0.02000000", "0.03000000", "0.04500000"))

	r := NewDBPDRepository(db)
	row, err := r.GetPefindoCurve(context.Background(), "idAA", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetPefindoCurve: %v", err)
	}
	if row == nil {
		t.Fatal("Expected row, got nil")
	}
	if row.Rating != "idAA" {
		t.Errorf("Rating want idAA got %s", row.Rating)
	}
	if !row.PD12Month.Equal(d("0.00350000")) {
		t.Errorf("PD12Month want 0.00350000 got %s", row.PD12Month)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("mock expectations: %v", err)
	}
}

func TestDBPDRepo_GetPefindoCurve_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT rating")).
		WithArgs("UNRATED", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{
			"rating", "pd_12month", "pd_lifetime_3y",
			"pd_lifetime_5y", "pd_lifetime_7y", "pd_lifetime_10y",
		})) // empty result → ErrNoRows

	r := NewDBPDRepository(db)
	row, err := r.GetPefindoCurve(context.Background(), "UNRATED", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetPefindoCurve not-found error: %v", err)
	}
	if row != nil {
		t.Error("Expected nil for not-found")
	}
}

func TestDBPDRepo_GetActiveImpactPD_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT impact_multiplier")).
		WithArgs("PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"impact_multiplier", "periode_id"}).
			AddRow("1.05000000", "PBUKU-2026-06"))

	r := NewDBPDRepository(db)
	imp, err := r.GetActiveImpactPD(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetActiveImpactPD: %v", err)
	}
	if imp == nil {
		t.Fatal("Expected impact row")
	}
	if !imp.ImpactMultiplier.Equal(d("1.05000000")) {
		t.Errorf("ImpactMultiplier want 1.05 got %s", imp.ImpactMultiplier)
	}
}

func TestDBPDRepo_GetActiveImpactMevPD_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT impact_multiplier")).
		WithArgs("GOOD", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"impact_multiplier", "periode_id", "skenario"}).
			AddRow("0.85000000", "PBUKU-2026-06", "GOOD"))

	r := NewDBPDRepository(db)
	imp, err := r.GetActiveImpactMevPD(context.Background(), "GOOD", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetActiveImpactMevPD: %v", err)
	}
	if imp == nil {
		t.Fatal("Expected row")
	}
	if !imp.ImpactMultiplier.Equal(d("0.85000000")) {
		t.Errorf("ImpactMultiplier want 0.85 got %s", imp.ImpactMultiplier)
	}
}

func TestDBPDRepo_GetActiveRating_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cpID := uuid.New()
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT rating_pefindo")).
		WithArgs(cpID, evalDate).
		WillReturnRows(sqlmock.NewRows([]string{"rating_pefindo"}).AddRow("idAA"))

	r := NewDBPDRepository(db)
	rating, err := r.GetActiveRating(context.Background(), cpID, evalDate)
	if err != nil {
		t.Fatalf("GetActiveRating: %v", err)
	}
	if rating != "idAA" {
		t.Errorf("Rating want idAA got %s", rating)
	}
}

func TestDBPDRepo_BatchLoadPDCurves_MultiRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs("PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{
			"rating", "pd_12month", "pd_lifetime_3y",
			"pd_lifetime_5y", "pd_lifetime_7y", "pd_lifetime_10y",
		}).
			AddRow("idAA", "0.00350000", "0.01200000", "0.02000000", "0.03000000", "0.04500000").
			AddRow("idA", "0.00600000", "0.02000000", "0.03300000", "0.04800000", "0.07000000"))

	r := NewDBPDRepository(db)
	curves, err := r.BatchLoadPDCurves(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadPDCurves: %v", err)
	}
	if len(curves) != 2 {
		t.Errorf("Expected 2 curves, got %d", len(curves))
	}
	if !curves["idAA"].PD12Month.Equal(d("0.00350000")) {
		t.Errorf("idAA PD12M mismatch")
	}
}

func TestDBPDRepo_BatchLoadImpactMevPD_MultiRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT skenario")).
		WithArgs("PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"skenario", "impact_multiplier", "periode_id"}).
			AddRow("GOOD", "0.85000000", "PBUKU-2026-06").
			AddRow("BAD", "1.25000000", "PBUKU-2026-06"))

	r := NewDBPDRepository(db)
	result, err := r.BatchLoadImpactMevPD(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadImpactMevPD: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result))
	}
	if !result["BAD"].ImpactMultiplier.Equal(d("1.25000000")) {
		t.Errorf("BAD multiplier mismatch")
	}
}

func TestDBPDRepo_BatchLoadRatings_TwoCounterparties(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cp1 := uuid.New()
	cp2 := uuid.New()
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs(evalDate, cp1, cp2).
		WillReturnRows(sqlmock.NewRows([]string{"counterparty_id", "rating_pefindo"}).
			AddRow(cp1, "idAA").
			AddRow(cp2, "idBBB"))

	r := NewDBPDRepository(db)
	result, err := r.BatchLoadRatings(context.Background(), []uuid.UUID{cp1, cp2}, evalDate)
	if err != nil {
		t.Fatalf("BatchLoadRatings: %v", err)
	}
	if result[cp1] != "idAA" {
		t.Errorf("cp1 rating want idAA got %s", result[cp1])
	}
	if result[cp2] != "idBBB" {
		t.Errorf("cp2 rating want idBBB got %s", result[cp2])
	}
}

// ─── DBLGDRepository ──────────────────────────────────────────────────────────

func TestDBLGDRepo_GetByPool_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT tipe_eksposur")).
		WithArgs("BANK", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"tipe_eksposur", "lgd"}).
			AddRow("BANK", "0.45000000"))

	r := NewDBLGDRepository(db)
	row, err := r.GetByPool(context.Background(), "BANK", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetByPool: %v", err)
	}
	if row == nil {
		t.Fatal("Expected row")
	}
	if !row.LGD.Equal(d("0.45000000")) {
		t.Errorf("LGD want 0.45 got %s", row.LGD)
	}
}

func TestDBLGDRepo_GetCollateralHaircut_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT config_value")).
		WithArgs("LGD_COLLATERAL_HAIRCUT_CASH").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"})) // empty → returns 0

	r := NewDBLGDRepository(db)
	hc, err := r.GetCollateralHaircut(context.Background(), "CASH")
	if err != nil {
		t.Fatalf("GetCollateralHaircut: %v", err)
	}
	if !hc.IsZero() {
		t.Errorf("Expected 0 for missing key, got %s", hc)
	}
}

func TestDBLGDRepo_BatchLoadLGDPools_MultiRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs("PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"tipe_eksposur", "lgd"}).
			AddRow("BANK", "0.45000000").
			AddRow("CORPORATE", "0.55000000"))

	r := NewDBLGDRepository(db)
	pools, err := r.BatchLoadLGDPools(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadLGDPools: %v", err)
	}
	if len(pools) != 2 {
		t.Errorf("Expected 2 pools, got %d", len(pools))
	}
}

// ─── DBKursRepository ─────────────────────────────────────────────────────────

func TestDBKursRepo_GetByDate_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	date := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT kode_mata_uang")).
		WithArgs("USD", date).
		WillReturnRows(sqlmock.NewRows([]string{
			"kode_mata_uang", "nilai_kurs", "tanggal_berlaku", "workflow_status",
		}).AddRow("USD", "15432.12345678", date, "APPROVED"))

	r := NewDBKursRepository(db)
	kr, err := r.GetByDate(context.Background(), "USD", date)
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if kr == nil {
		t.Fatal("Expected kurs row")
	}
	if kr.WorkflowStatus != "APPROVED" {
		t.Errorf("Status want APPROVED got %s", kr.WorkflowStatus)
	}
	if !kr.NilaiKurs.Equal(d("15432.12345678")) {
		t.Errorf("NilaiKurs mismatch: %s", kr.NilaiKurs)
	}
}

func TestDBKursRepo_BatchLoadKurs_MultiRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	date := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs(date).
		WillReturnRows(sqlmock.NewRows([]string{
			"kode_mata_uang", "nilai_kurs", "tanggal_berlaku", "workflow_status",
		}).
			AddRow("USD", "15432.12345678", date, "APPROVED").
			AddRow("EUR", "17000.00000000", date, "APPROVED"))

	r := NewDBKursRepository(db)
	result, err := r.BatchLoadKurs(context.Background(), date)
	if err != nil {
		t.Fatalf("BatchLoadKurs: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result))
	}
}

// ─── DBInstrumenSnapshotRepo ──────────────────────────────────────────────────

func TestDBInstrumenRepo_GetEADInputs_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()
	cpID := uuid.New()
	jt := time.Date(2028, 12, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, kode_instrumen")).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		}).AddRow(instrID, "INST-001", "Bank BCA Deposito", "DEPOSITO",
			"IDR", "1000000000.0000", "AC", jt, cpID, "AKTIF"))

	r := NewDBInstrumenSnapshotRepo(db)
	inst, err := r.GetEADInputs(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetEADInputs: %v", err)
	}
	if inst == nil {
		t.Fatal("Expected instrumen row")
	}
	if inst.KodeInstrumen != "INST-001" {
		t.Errorf("KodeInstrumen want INST-001 got %s", inst.KodeInstrumen)
	}
	if !inst.Nominal.Equal(d("1000000000.0000")) {
		t.Errorf("Nominal mismatch: %s", inst.Nominal)
	}
}

func TestDBInstrumenRepo_GetCurrentStage_Stage2(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT stage_sesudah")).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{"stage_sesudah"}).AddRow("STAGE_2"))

	r := NewDBInstrumenSnapshotRepo(db)
	stage, err := r.GetCurrentStage(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetCurrentStage: %v", err)
	}
	if stage != Stage2 {
		t.Errorf("Stage want STAGE_2 got %s", stage)
	}
}

func TestDBInstrumenRepo_GetEIRScheduleRow_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()
	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	cicilan := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT instrumen_id, tanggal_cicilan")).
		WithArgs(instrID, asOf).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "tanggal_cicilan", "principal_outstanding",
			"bunga_akrual", "schedule_version",
		}).AddRow(instrID, cicilan, "950000000.0000", "5000000.0000", 1))

	r := NewDBInstrumenSnapshotRepo(db)
	row, err := r.GetEIRScheduleRow(context.Background(), instrID, asOf)
	if err != nil {
		t.Fatalf("GetEIRScheduleRow: %v", err)
	}
	if row == nil {
		t.Fatal("Expected EIR schedule row")
	}
	if !row.PrincipalOutstanding.Equal(d("950000000.0000")) {
		t.Errorf("PrincipalOutstanding mismatch: %s", row.PrincipalOutstanding)
	}
	if !row.BungaAkrual.Equal(d("5000000.0000")) {
		t.Errorf("BungaAkrual mismatch: %s", row.BungaAkrual)
	}
}

func TestDBInstrumenRepo_BatchLoadInstruments_TwoRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	id1 := uuid.New()
	id2 := uuid.New()
	cp := uuid.New()

	mock.ExpectQuery("SELECT id").
		WithArgs(id1, id2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		}).
			AddRow(id1, "INST-001", "Name1", "DEPOSITO", "IDR", "500000000.0000", "AC", nil, cp, "AKTIF").
			AddRow(id2, "INST-002", "Name2", "OBLIGASI", "IDR", "300000000.0000", "FVOCI_DEBT", nil, cp, "AKTIF"))

	r := NewDBInstrumenSnapshotRepo(db)
	result, err := r.BatchLoadInstruments(context.Background(), []uuid.UUID{id1, id2})
	if err != nil {
		t.Fatalf("BatchLoadInstruments: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 instruments, got %d", len(result))
	}
}

func TestDBInstrumenRepo_BatchLoadEIRSchedules_OneRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()
	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	cicilan := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs(asOf, instrID).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "tanggal_cicilan", "principal_outstanding",
			"bunga_akrual", "schedule_version",
		}).AddRow(instrID, cicilan, "900000000.0000", "3000000.0000", 2))

	r := NewDBInstrumenSnapshotRepo(db)
	result, err := r.BatchLoadEIRSchedules(context.Background(), []uuid.UUID{instrID}, asOf)
	if err != nil {
		t.Fatalf("BatchLoadEIRSchedules: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 schedule, got %d", len(result))
	}
	if !result[instrID].BungaAkrual.Equal(d("3000000.0000")) {
		t.Errorf("BungaAkrual mismatch: %s", result[instrID].BungaAkrual)
	}
}

func TestDBInstrumenRepo_BatchLoadCurrentStages_TwoInstruments(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	id1 := uuid.New()
	id2 := uuid.New()

	mock.ExpectQuery("SELECT DISTINCT ON").
		WithArgs(id1, id2).
		WillReturnRows(sqlmock.NewRows([]string{"instrumen_id", "stage_sesudah"}).
			AddRow(id1, "STAGE_1").
			AddRow(id2, "STAGE_2"))

	r := NewDBInstrumenSnapshotRepo(db)
	result, err := r.BatchLoadCurrentStages(context.Background(), []uuid.UUID{id1, id2})
	if err != nil {
		t.Fatalf("BatchLoadCurrentStages: %v", err)
	}
	if result[id1] != Stage1 {
		t.Errorf("id1 want STAGE_1 got %s", result[id1])
	}
	if result[id2] != Stage2 {
		t.Errorf("id2 want STAGE_2 got %s", result[id2])
	}
}

// ─── DBCounterpartyRepo ───────────────────────────────────────────────────────

func TestDBCounterpartyRepo_GetTipeCounterparty_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cpID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT tipe_counterparty")).
		WithArgs(cpID).
		WillReturnRows(sqlmock.NewRows([]string{"tipe_counterparty"}).AddRow("BANK"))

	r := NewDBCounterpartyRepo(db)
	tipe, err := r.GetTipeCounterparty(context.Background(), cpID)
	if err != nil {
		t.Fatalf("GetTipeCounterparty: %v", err)
	}
	if tipe != "BANK" {
		t.Errorf("tipe want BANK got %s", tipe)
	}
}

func TestDBCounterpartyRepo_BatchLoadCounterparties_TwoRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	id1 := uuid.New()
	id2 := uuid.New()

	mock.ExpectQuery("SELECT id").
		WithArgs(id1, id2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nama_counterparty", "tipe_counterparty"}).
			AddRow(id1, "BCA", "BANK").
			AddRow(id2, "PT XYZ", "KORPORASI"))

	r := NewDBCounterpartyRepo(db)
	result, err := r.BatchLoadCounterparties(context.Background(), []uuid.UUID{id1, id2})
	if err != nil {
		t.Fatalf("BatchLoadCounterparties: %v", err)
	}
	if result[id1].TipeCounterparty != "BANK" {
		t.Errorf("id1 tipe want BANK got %s", result[id1].TipeCounterparty)
	}
	if result[id2].TipeCounterparty != "KORPORASI" {
		t.Errorf("id2 tipe want KORPORASI got %s", result[id2].TipeCounterparty)
	}
}

// ─── DBCCFConfigRepo ─────────────────────────────────────────────────────────

func TestDBCCFRepo_GetCCFTable_DB(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	jsonVal := `{"DEPOSITO":"0","OBLIGASI":"0","COMMITMENT":"0.75"}`

	mock.ExpectQuery(regexp.QuoteMeta("SELECT config_value")).
		WithArgs("CCF_TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow(jsonVal))

	r := NewDBCCFConfigRepo(db)
	table, err := r.GetCCFTable(context.Background())
	if err != nil {
		t.Fatalf("GetCCFTable: %v", err)
	}
	if !table["DEPOSITO"].IsZero() {
		t.Errorf("DEPOSITO CCF want 0 got %s", table["DEPOSITO"])
	}
	if !table["COMMITMENT"].Equal(d("0.75")) {
		t.Errorf("COMMITMENT CCF want 0.75 got %s", table["COMMITMENT"])
	}
}

func TestDBCCFRepo_GetCCFTable_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT config_value")).
		WithArgs("CCF_TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"})) // empty → ErrNoRows

	r := NewDBCCFConfigRepo(db)
	_, err := r.GetCCFTable(context.Background())
	if err == nil {
		t.Error("Expected error for missing sys.config key")
	}
}

// ─── LGDMapping DB path ───────────────────────────────────────────────────────

func TestDBLGDRepo_GetLGDMapping_DB(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	jsonVal := `{"BANK":"BANK","KORPORASI":"CORPORATE"}`

	mock.ExpectQuery(regexp.QuoteMeta("SELECT config_value")).
		WithArgs("LGD_COUNTERPARTY_TYPE_MAPPING").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow(jsonVal))

	r := NewDBLGDRepository(db)
	mapping, err := r.GetLGDMapping(context.Background())
	if err != nil {
		t.Fatalf("GetLGDMapping: %v", err)
	}
	if mapping["BANK"] != "BANK" {
		t.Errorf("BANK mapping want BANK got %s", mapping["BANK"])
	}
	if mapping["KORPORASI"] != "CORPORATE" {
		t.Errorf("KORPORASI mapping want CORPORATE got %s", mapping["KORPORASI"])
	}
}

// ─── Additional coverage: not-found paths and edge cases ─────────────────────

func TestDBPDRepo_GetActiveImpactPD_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT impact_multiplier")).
		WithArgs("PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"impact_multiplier", "periode_id"}))

	r := NewDBPDRepository(db)
	imp, err := r.GetActiveImpactPD(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetActiveImpactPD not-found: %v", err)
	}
	if imp != nil {
		t.Error("Expected nil for not-found")
	}
}

func TestDBPDRepo_GetActiveImpactMevPD_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT impact_multiplier")).
		WithArgs("BAD", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"impact_multiplier", "periode_id", "skenario"}))

	r := NewDBPDRepository(db)
	imp, err := r.GetActiveImpactMevPD(context.Background(), "BAD", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetActiveImpactMevPD not-found: %v", err)
	}
	if imp != nil {
		t.Error("Expected nil for not-found")
	}
}

func TestDBPDRepo_GetActiveRating_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cpID := uuid.New()
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT rating_pefindo")).
		WithArgs(cpID, evalDate).
		WillReturnRows(sqlmock.NewRows([]string{"rating_pefindo"})) // no row

	r := NewDBPDRepository(db)
	rating, err := r.GetActiveRating(context.Background(), cpID, evalDate)
	if err != nil {
		t.Fatalf("GetActiveRating not-found: %v", err)
	}
	if rating != "" {
		t.Errorf("Expected empty string for not-found, got %s", rating)
	}
}

func TestDBLGDRepo_GetByPool_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT tipe_eksposur")).
		WithArgs("UNKNOWN_POOL", "PBUKU-2026-06").
		WillReturnRows(sqlmock.NewRows([]string{"tipe_eksposur", "lgd"}))

	r := NewDBLGDRepository(db)
	row, err := r.GetByPool(context.Background(), "UNKNOWN_POOL", "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetByPool not-found: %v", err)
	}
	if row != nil {
		t.Error("Expected nil for not-found pool")
	}
}

func TestDBLGDRepo_GetCollateralHaircut_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	jsonVal := `{"rate":"0.15"}`
	mock.ExpectQuery(regexp.QuoteMeta("SELECT config_value")).
		WithArgs("LGD_COLLATERAL_HAIRCUT_PROPERTY").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow(jsonVal))

	r := NewDBLGDRepository(db)
	hc, err := r.GetCollateralHaircut(context.Background(), "PROPERTY")
	if err != nil {
		t.Fatalf("GetCollateralHaircut found: %v", err)
	}
	if !hc.Equal(d("0.15")) {
		t.Errorf("Haircut want 0.15 got %s", hc)
	}
}

func TestDBKursRepo_GetByDate_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	date := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT kode_mata_uang")).
		WithArgs("JPY", date).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "nilai_kurs", "tanggal_berlaku", "workflow_status"}))

	r := NewDBKursRepository(db)
	kr, err := r.GetByDate(context.Background(), "JPY", date)
	if err != nil {
		t.Fatalf("GetByDate not-found: %v", err)
	}
	if kr != nil {
		t.Error("Expected nil for not-found kurs")
	}
}

func TestDBInstrumenRepo_GetCurrentStage_NoHistory_DefaultsStage1(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT stage_sesudah")).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{"stage_sesudah"})) // no row

	r := NewDBInstrumenSnapshotRepo(db)
	stage, err := r.GetCurrentStage(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetCurrentStage no-history: %v", err)
	}
	if stage != Stage1 {
		t.Errorf("Expected Stage1 default, got %s", stage)
	}
}

func TestDBInstrumenRepo_GetEIRScheduleRow_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()
	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT instrumen_id, tanggal_cicilan")).
		WithArgs(instrID, asOf).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "tanggal_cicilan", "principal_outstanding", "bunga_akrual", "schedule_version",
		})) // no row

	r := NewDBInstrumenSnapshotRepo(db)
	row, err := r.GetEIRScheduleRow(context.Background(), instrID, asOf)
	if err != nil {
		t.Fatalf("GetEIRScheduleRow not-found: %v", err)
	}
	if row != nil {
		t.Error("Expected nil for not-found EIR schedule")
	}
}

func TestDBInstrumenRepo_GetEADInputs_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	instrID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, kode_instrumen")).
		WithArgs(instrID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		})) // no row

	r := NewDBInstrumenSnapshotRepo(db)
	inst, err := r.GetEADInputs(context.Background(), instrID)
	if err != nil {
		t.Fatalf("GetEADInputs not-found: %v", err)
	}
	if inst != nil {
		t.Error("Expected nil for not-found instrument")
	}
}

func TestDBCounterpartyRepo_GetTipeCounterparty_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cpID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT tipe_counterparty")).
		WithArgs(cpID).
		WillReturnRows(sqlmock.NewRows([]string{"tipe_counterparty"})) // no row

	r := NewDBCounterpartyRepo(db)
	tipe, err := r.GetTipeCounterparty(context.Background(), cpID)
	if err != nil {
		t.Fatalf("GetTipeCounterparty not-found: %v", err)
	}
	if tipe != "" {
		t.Errorf("Expected empty for not-found, got %s", tipe)
	}
}

// ─── scanDecimal ─────────────────────────────────────────────────────────────

func TestScanDecimal_Nil(t *testing.T) {
	result, err := scanDecimal(nil)
	if err != nil {
		t.Fatalf("scanDecimal(nil) error: %v", err)
	}
	if !result.IsZero() {
		t.Errorf("scanDecimal(nil) want 0 got %s", result)
	}
}

func TestScanDecimal_StringInput(t *testing.T) {
	result, err := scanDecimal("0.45000000")
	if err != nil {
		t.Fatalf("scanDecimal string error: %v", err)
	}
	if !result.Equal(d("0.45000000")) {
		t.Errorf("scanDecimal string want 0.45 got %s", result)
	}
}

func TestScanDecimal_ByteSlice(t *testing.T) {
	result, err := scanDecimal([]byte("1.05000000"))
	if err != nil {
		t.Fatalf("scanDecimal []byte error: %v", err)
	}
	if !result.Equal(d("1.05000000")) {
		t.Errorf("scanDecimal []byte want 1.05 got %s", result)
	}
}

func TestScanDecimal_IntType(t *testing.T) {
	// Covers the default fmt.Sprintf path.
	result, err := scanDecimal(int64(42))
	if err != nil {
		t.Fatalf("scanDecimal int error: %v", err)
	}
	if !result.Equal(d("42")) {
		t.Errorf("scanDecimal int want 42 got %s", result)
	}
}

// ─── ListECLApplicableInstruments ────────────────────────────────────────────

func TestListECLApplicableInstruments_DB_NoFilters(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	cpID := uuid.New()
	instrID := uuid.New()
	jt := time.Date(2028, 12, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		}).
			AddRow(instrID, "INST-001", "BCA Deposito", "DEPOSITO", "IDR",
				"1000000000.0000", "AC", jt, cpID, "AKTIF"))

	r := NewDBInstrumenSnapshotRepo(db)
	rows, cursor, hasMore, err := r.ListECLApplicableInstruments(
		context.Background(),
		"PBUKU-2026-06", "", "", "", "",
		nil, "", "kode_instrumen", "asc", "", 50,
	)
	if err != nil {
		t.Fatalf("ListECLApplicableInstruments: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
	if hasMore {
		t.Error("Expected hasMore=false")
	}
	if cursor != "" {
		t.Errorf("Expected empty cursor, got %s", cursor)
	}
}

func TestListECLApplicableInstruments_DB_WithFilters(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		})) // empty result

	r := NewDBInstrumenSnapshotRepo(db)
	rows, cursor, hasMore, err := r.ListECLApplicableInstruments(
		context.Background(),
		"PBUKU-2026-06", "STAGE_2", "OBLIGASI", "AC", "IDR",
		nil, "BCA", "kode_instrumen", "desc", "INST-100", 10,
	)
	if err != nil {
		t.Fatalf("ListECLApplicableInstruments filtered: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("Expected 0 rows, got %d", len(rows))
	}
	_ = cursor
	_ = hasMore
}

func TestListECLApplicableInstruments_DB_InvalidSort_UsesDefault(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen",
			"mata_uang", "nominal", "klasifikasi_psak71",
			"tanggal_jatuh_tempo", "counterparty_id", "status",
		}))

	r := NewDBInstrumenSnapshotRepo(db)
	rows, _, _, err := r.ListECLApplicableInstruments(
		context.Background(),
		"PBUKU-2026-06", "", "", "", "",
		nil, "", "invalid_col", "invalid_dir", "", 10,
	)
	if err != nil {
		t.Fatalf("ListECLApplicableInstruments invalid sort: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("Expected 0 rows, got %d", len(rows))
	}
}

// ─── NewServices smoke test ───────────────────────────────────────────────────

func TestNewServices_NilDB_NoAudit(t *testing.T) {
	// Verify NewServices works with nil DB and nil auditWriter (dev mode).
	svc := NewServices(nil, nil)
	if svc == nil {
		t.Fatal("NewServices returned nil")
	}
	if svc.PD == nil || svc.LGD == nil || svc.EAD == nil || svc.CCF == nil || svc.Bulk == nil {
		t.Error("One or more services are nil")
	}
	// previewRepoFromInstrRepo should succeed (instrRepo is *DBInstrumenSnapshotRepo which implements it)
	p, ok := svc.previewRepoFromInstrRepo()
	if !ok {
		t.Error("Expected previewRepoFromInstrRepo to return true for DBInstrumenSnapshotRepo")
	}
	if p == nil {
		t.Error("Expected non-nil previewInstrumentLister")
	}
}
