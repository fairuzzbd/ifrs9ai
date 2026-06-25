package akrualmaturity

// repo_test.go — Repo unit tests using go-sqlmock.
// Covers happy path + error paths for key repo methods.
// No real DB; all SQL verified via sqlmock ExpectQuery/ExpectExec.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newRepoDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return db, mock, func() { _ = db.Close() }
}

var anyArgR = sqlmock.AnyArg()
var testInstrID = uuid.New()
var testActorID = uuid.New()
var testPeriodeIDR = uuid.New()
var testAkrualID = uuid.New()
var testJatuhTempoID = uuid.New()
var testDividenID = uuid.New()

// ─── BeginTx ─────────────────────────────────────────────────────────────────

func TestRepo_BeginTx(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	repo := NewRepo(db)
	tx, err := repo.BeginTx(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, tx)
	_ = tx.Rollback()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_BeginTx_Error(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin().WillReturnError(fmt.Errorf("conn error"))
	repo := NewRepo(db)
	_, err := repo.BeginTx(context.Background())
	require.Error(t, err)
}

// ─── IsHoliday ───────────────────────────────────────────────────────────────

func TestRepo_IsHoliday_Yes(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM sys.holiday_calendar`)).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	repo := NewRepo(db)
	is, err := repo.IsHoliday(context.Background(), time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, is)
}

func TestRepo_IsHoliday_No(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM sys.holiday_calendar`)).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewRepo(db)
	is, err := repo.IsHoliday(context.Background(), time.Now())
	require.NoError(t, err)
	assert.False(t, is)
}

func TestRepo_IsHoliday_DBError(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM sys.holiday_calendar`)).
		WithArgs(anyArgR).
		WillReturnError(fmt.Errorf("db error"))

	repo := NewRepo(db)
	_, err := repo.IsHoliday(context.Background(), time.Now())
	require.Error(t, err)
}

// ─── GetPeriodeByTanggal ─────────────────────────────────────────────────────

func TestRepo_GetPeriodeByTanggal_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, status_periode, tanggal_mulai, tanggal_akhir`)).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status_periode", "tanggal_mulai", "tanggal_akhir"}).
			AddRow(testPeriodeIDR, "OPEN", now, now.AddDate(0, 1, 0)))

	repo := NewRepo(db)
	p, err := repo.GetPeriodeByTanggal(context.Background(), now)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "OPEN", p.StatusPeriode)
}

func TestRepo_GetPeriodeByTanggal_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, status_periode, tanggal_mulai, tanggal_akhir`)).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status_periode", "tanggal_mulai", "tanggal_akhir"}))

	repo := NewRepo(db)
	p, err := repo.GetPeriodeByTanggal(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Nil(t, p)
}

func TestRepo_GetPeriodeByTanggal_DBError(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, status_periode, tanggal_mulai, tanggal_akhir`)).
		WithArgs(anyArgR).
		WillReturnError(fmt.Errorf("db error"))

	repo := NewRepo(db)
	_, err := repo.GetPeriodeByTanggal(context.Background(), time.Now())
	require.Error(t, err)
}

// ─── GetStaleDaysConfig ───────────────────────────────────────────────────────

func TestRepo_GetStaleDaysConfig_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM sys.config_param`)).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("45"))

	repo := NewRepo(db)
	days, err := repo.GetStaleDaysConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 45, days)
}

func TestRepo_GetStaleDaysConfig_Default(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT value FROM sys.config_param`)).
		WillReturnRows(sqlmock.NewRows([]string{"value"})) // empty → ErrNoRows → default 30

	repo := NewRepo(db)
	days, err := repo.GetStaleDaysConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 30, days)
}

// ─── IsDuplicateAkrual ───────────────────────────────────────────────────────

func TestRepo_IsDuplicateAkrual_True(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
		WithArgs(anyArgR, anyArgR, anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	repo := NewRepo(db)
	dup, err := repo.IsDuplicateAkrual(context.Background(), testInstrID, time.Now(), JenisBunga)
	require.NoError(t, err)
	assert.True(t, dup)
}

func TestRepo_IsDuplicateAkrual_False(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
		WithArgs(anyArgR, anyArgR, anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewRepo(db)
	dup, err := repo.IsDuplicateAkrual(context.Background(), testInstrID, time.Now(), JenisBunga)
	require.NoError(t, err)
	assert.False(t, dup)
}

// ─── InsertAkrual ────────────────────────────────────────────────────────────

func TestRepo_InsertAkrual_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO trx.pendapatan_akrual`)).
		WithArgs(
			anyArgR, anyArgR, anyArgR, anyArgR, anyArgR,
			anyArgR, anyArgR, anyArgR, anyArgR, anyArgR,
			anyArgR, anyArgR, anyArgR,
			anyArgR, anyArgR,
			anyArgR, anyArgR,
			anyArgR, anyArgR, anyArgR,
			anyArgR, anyArgR, anyArgR, anyArgR,
			anyArgR, anyArgR,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	stage := 1
	eir := decimal.NewFromFloat(0.075)
	a := &PendapatanAkrual{
		ID:               testAkrualID,
		InstrumenID:      testInstrID,
		TanggalAkrual:    time.Now(),
		Jenis:            JenisBunga,
		Stage:            &stage,
		CarryingBasisIDR: decimal.NewFromInt(1_000_000),
		EIRPersen:        &eir,
		BungaKotor:       decimal.NewFromFloat(205.48),
		PPh:              decimal.Zero,
		BungaBersih:      decimal.NewFromFloat(205.48),
		MataUang:         "IDR",
		Status:           AkrualAutoPosted,
		CreatedAt:        time.Now(),
		CreatedBy:        testActorID,
		UpdatedAt:        time.Now(),
		UpdatedBy:        testActorID,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}

	repo := NewRepo(db)
	err = repo.InsertAkrual(context.Background(), tx, a)
	require.NoError(t, err)
	_ = tx.Commit()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_InsertAkrual_Error(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO trx.pendapatan_akrual`)).
		WillReturnError(fmt.Errorf("unique constraint violation"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.InsertAkrual(context.Background(), tx, &PendapatanAkrual{
		TanggalAkrual: time.Now(),
		Jenis:         JenisBunga,
		CreatedBy:     testActorID,
		UpdatedBy:     testActorID,
		TenantID:      "TUGURE",
	})
	require.Error(t, err)
	_ = tx.Rollback()
}

// ─── GetAkrualByID ───────────────────────────────────────────────────────────

func TestRepo_GetAkrualByID_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// returns nil, nil on ErrNoRows (not error)
	mock.ExpectQuery(`SELECT id, instrumen_id, tanggal_akrual, jenis, stage`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty resultset

	repo := NewRepo(db)
	a, err := repo.GetAkrualByID(context.Background(), testAkrualID)
	require.NoError(t, err)
	assert.Nil(t, a)
}

// ─── GetFXRateApproved ────────────────────────────────────────────────────────

func TestRepo_GetFXRateApproved_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	fxID := uuid.New()
	now := time.Now().UTC()
	// Actual query: SELECT id, mata_uang, tanggal, COALESCE(rate_idr::text,'0') FROM sys.fx_rate ...
	mock.ExpectQuery(`SELECT id, mata_uang, tanggal`).
		WithArgs(anyArgR, anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id", "mata_uang", "tanggal", "rate_idr"}).
			AddRow(fxID, "USD", now, "15432.12345678"))

	repo := NewRepo(db)
	fx, err := repo.GetFXRateApproved(context.Background(), "USD", time.Now())
	require.NoError(t, err)
	require.NotNil(t, fx)
	assert.Equal(t, "USD", fx.MataUang)
	assert.Equal(t, fxID, fx.ID)
}

func TestRepo_GetFXRateApproved_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT id, mata_uang, tanggal`).
		WithArgs(anyArgR, anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id", "mata_uang", "tanggal", "rate_idr"}))

	repo := NewRepo(db)
	fx, err := repo.GetFXRateApproved(context.Background(), "USD", time.Now())
	require.NoError(t, err)
	assert.Nil(t, fx)
}

// ─── InsertJatuhTempo ────────────────────────────────────────────────────────

func TestRepo_InsertJatuhTempo_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO trx.jatuh_tempo`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	jt := &JatuhTempo{
		ID:                testJatuhTempoID,
		InstrumenID:       testInstrID,
		TanggalJatuhTempo: time.Now(),
		Jenis:             "MATURITY",
		PokokReturned:     decimal.NewFromInt(1_000_000),
		BungaReturned:     decimal.NewFromFloat(5000),
		PPh:               decimal.NewFromFloat(1000),
		Proceeds:          decimal.NewFromFloat(1_004_000),
		Status:            JatuhTempoPending,
		CreatedAt:         time.Now(),
		CreatedBy:         testActorID,
		UpdatedAt:         time.Now(),
		UpdatedBy:         testActorID,
		RowVersion:        1,
		TenantID:          "TUGURE",
	}

	repo := NewRepo(db)
	err = repo.InsertJatuhTempo(context.Background(), tx, jt)
	require.NoError(t, err)
	_ = tx.Commit()
}

func TestRepo_InsertJatuhTempo_Error(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO trx.jatuh_tempo`)).
		WillReturnError(fmt.Errorf("constraint violation"))
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.InsertJatuhTempo(context.Background(), tx, &JatuhTempo{
		TanggalJatuhTempo: time.Now(),
		CreatedBy:         testActorID,
		UpdatedBy:         testActorID,
		TenantID:          "TUGURE",
	})
	require.Error(t, err)
	_ = tx.Rollback()
}

// ─── InsertDividen ───────────────────────────────────────────────────────────

func TestRepo_InsertDividen_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO trx.dividen`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	d := &Dividen{
		ID:           testDividenID,
		InstrumenID:  testInstrID,
		TanggalTerima: time.Now(),
		JumlahKotor:  decimal.NewFromInt(5000),
		PPHDividen:   decimal.NewFromFloat(500),
		JumlahBersih: decimal.NewFromFloat(4500),
		MakerID:      testActorID,
		Status:       DividenPendingApproval,
		CreatedAt:    time.Now(),
		CreatedBy:    testActorID,
		UpdatedAt:    time.Now(),
		UpdatedBy:    testActorID,
		RowVersion:   1,
		TenantID:     "TUGURE",
	}

	repo := NewRepo(db)
	err = repo.InsertDividen(context.Background(), tx, d)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── InsertDLQ ───────────────────────────────────────────────────────────────

func TestRepo_InsertDLQ_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// 4 args: job_type, instrumen_id, error_code, error_detail (retry_count/max_retry/created_at are literal in SQL)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys.dlq`)).
		WithArgs(anyArgR, anyArgR, anyArgR, anyArgR).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepo(db)
	err := repo.InsertDLQ(context.Background(), "DAILY_ACCRUAL_JOB", testInstrID, "AKRUAL_TX_ERROR", "detail")
	require.NoError(t, err)
}

func TestRepo_InsertDLQ_Error(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys.dlq`)).
		WillReturnError(fmt.Errorf("db error"))

	repo := NewRepo(db)
	err := repo.InsertDLQ(context.Background(), "DAILY_ACCRUAL_JOB", testInstrID, "CODE", "detail")
	require.Error(t, err)
}

// ─── UpdateAkrualStatus ──────────────────────────────────────────────────────

func TestRepo_UpdateAkrualStatus_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE trx.pendapatan_akrual`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.UpdateAkrualStatus(context.Background(), tx, testAkrualID, AkrualAutoPosted, nil, nil, nil, 1, testActorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

func TestRepo_UpdateAkrualStatus_OptimisticLockConflict(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE trx.pendapatan_akrual`)).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected → lock conflict
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.UpdateAkrualStatus(context.Background(), tx, testAkrualID, AkrualAutoPosted, nil, nil, nil, 99, testActorID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "optimistic lock conflict")
	_ = tx.Rollback()
}

func TestRepo_UpdateAkrualStatus_WithNonNilParams(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE trx.pendapatan_akrual`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	jurnalID := uuid.New()
	overrideUser := uuid.New()
	comment := "override confirmed"
	repo := NewRepo(db)
	err = repo.UpdateAkrualStatus(context.Background(), tx, testAkrualID, AkrualAutoPosted,
		&jurnalID, &overrideUser, &comment, 1, testActorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── UpdateJatuhTempoStatus ──────────────────────────────────────────────────

func TestRepo_UpdateJatuhTempoStatus_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE trx.jatuh_tempo`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.UpdateJatuhTempoStatus(context.Background(), tx, testJatuhTempoID, time.Now(),
		JatuhTempoSettled, nil, nil, 1, testActorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── GetSealedECLForInstrumen ────────────────────────────────────────────────

func TestRepo_GetSealedECLForInstrumen_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT crl.ecl_calc_run_id`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_calc_run_id", "ecl_stage", "ecl_allowance", "sealed_at"}))

	repo := NewRepo(db)
	res, err := repo.GetSealedECLForInstrumen(context.Background(), testInstrID)
	require.NoError(t, err)
	assert.Nil(t, res)
}

func TestRepo_GetSealedECLForInstrumen_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	runID := uuid.New()
	now := time.Now().UTC()
	// Actual scan order: runID, stage, eclStr, sealedAt
	mock.ExpectQuery(`SELECT crl.ecl_calc_run_id`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_calc_run_id", "ecl_stage", "ecl_allowance", "sealed_at"}).
			AddRow(runID, 2, "50000.0000", now))

	repo := NewRepo(db)
	res, err := repo.GetSealedECLForInstrumen(context.Background(), testInstrID)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 2, res.Stage)
}

// ─── GetAmortisasiSchedule ────────────────────────────────────────────────────

func TestRepo_GetAmortisasiSchedule_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WithArgs(anyArgR, anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{}))

	repo := NewRepo(db)
	row, err := repo.GetAmortisasiSchedule(context.Background(), testInstrID, time.Now())
	require.NoError(t, err)
	assert.Nil(t, row)
}

// ─── ListAkrual ──────────────────────────────────────────────────────────────

func TestRepo_ListAkrual_Empty(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// sqlmock matches any SELECT query for the list
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_akrual", "jenis", "stage",
			"carrying_basis_str", "eir_persen_str", "bunga_kotor_str", "pph_str", "bunga_bersih_str",
			"fx_rate_id", "mata_uang", "klasifikasi_snapshot",
			"ecl_run_id_used", "stale_staging_flag",
			"override_user_id", "override_comment",
			"jurnal_header_id", "status", "periode_bulanan_id",
			"created_at", "created_by", "updated_at", "updated_by",
			"deleted_at", "deleted_by", "row_version", "tenant_id",
		}))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListAkrual(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

// ─── ListJatuhTempo ──────────────────────────────────────────────────────────

func TestRepo_ListJatuhTempo_Empty(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_jatuh_tempo", "jenis",
			"pokok_idr_str", "bunga_idr_str", "pph_str", "proceeds_str",
			"jurnal_header_id", "status",
			"created_at", "created_by", "updated_at", "updated_by",
			"deleted_at", "deleted_by", "row_version", "tenant_id",
		}))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListJatuhTempo(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

// ─── GetLastAkrualForInstrumen ────────────────────────────────────────────────

func TestRepo_GetLastAkrualForInstrumen_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// Actual: SELECT id, instrumen_id, tanggal_akrual, COALESCE(bunga_kotor::text,...) ...
	mock.ExpectQuery(`SELECT id, instrumen_id, tanggal_akrual`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_akrual",
			"bunga_kotor", "pph", "bunga_bersih",
			"status", "created_at", "row_version", "tenant_id",
		}))

	repo := NewRepo(db)
	a, err := repo.GetLastAkrualForInstrumen(context.Background(), testInstrID)
	// Not found → nil, nil pattern
	require.NoError(t, err)
	assert.Nil(t, a)
}

// ─── GetMTDYTDSummary ─────────────────────────────────────────────────────────

func TestRepo_GetMTDYTDSummary_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// Actual scan: 2 cols — mtd string, ytd string
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"mtd", "ytd"}).
			AddRow("100000.0000", "600000.0000"))

	repo := NewRepo(db)
	dash, err := repo.GetMTDYTDSummary(context.Background(), nil, nil, 2026, 6)
	require.NoError(t, err)
	require.NotNil(t, dash)
	assert.Equal(t, 2026, dash.Year)
	assert.Equal(t, 6, dash.Month)
	assert.Equal(t, "100000.0000", dash.AkrualMtdIdr)
}

// ─── GetDividenByID ──────────────────────────────────────────────────────────

func TestRepo_GetDividenByID_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT id, instrumen_id, tanggal_terima`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty → ErrNoRows → nil, nil

	repo := NewRepo(db)
	d, err := repo.GetDividenByID(context.Background(), testDividenID)
	require.NoError(t, err)
	assert.Nil(t, d)
}

// ─── UpdateDividenStatus ──────────────────────────────────────────────────────

func TestRepo_UpdateDividenStatus_OK(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE trx.dividen`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepo(db)
	err = repo.UpdateDividenStatus(context.Background(), tx,
		testDividenID, time.Now(),
		DividenApproved,
		nil, nil, nil, nil, nil, nil,
		1, testActorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── GetInstrumenInfo ─────────────────────────────────────────────────────────

func TestRepo_GetInstrumenInfo_NotFound(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{}))

	repo := NewRepo(db)
	info, err := repo.GetInstrumenInfo(context.Background(), testInstrID)
	require.NoError(t, err)
	assert.Nil(t, info)
}

// ─── GetActiveAccruingInstrumens ─────────────────────────────────────────────

func TestRepo_GetActiveAccruingInstrumens_Empty(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "tipe_instrumen", "status",
			"klasifikasi_psak71", "eir_persen_str", "credit_adjusted_eir_str",
			"gross_carrying_idr_str", "mata_uang", "stage",
			"tanggal_jatuh_tempo", "pph_rate_persen_str", "amortisasi_harian_str",
		}))

	repo := NewRepo(db)
	insts, err := repo.GetActiveAccruingInstrumens(context.Background())
	require.NoError(t, err)
	assert.Empty(t, insts)
}

// ─── GetActiveMaturityInstrumens ─────────────────────────────────────────────

func TestRepo_GetActiveMaturityInstrumens_Empty(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "tipe_instrumen", "status",
			"klasifikasi_psak71", "eir_persen_str", "credit_adjusted_eir_str",
			"gross_carrying_idr_str", "mata_uang", "stage",
			"tanggal_jatuh_tempo", "pph_rate_persen_str", "amortisasi_harian_str",
		}))

	repo := NewRepo(db)
	insts, err := repo.GetActiveMaturityInstrumens(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Empty(t, insts)
}

// scanInstrumenAkrualInfo columns: id, kode_instrumen, nama_instrumen, status,
// klasifikasi_psak71, klasifikasi_locked, mata_uang, gross_carrying_str,
// portofolio_id, is_poci, tanggal_jatuh_tempo (11 columns).

func instrumenAkrualInfoRow() *sqlmock.Rows {
	jatuhTempo := time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen", "status",
		"klasifikasi_psak71", "klasifikasi_locked", "mata_uang",
		"gross_carrying_str", "portofolio_id", "is_poci", "tanggal_jatuh_tempo",
	}).AddRow(
		testInstrID, "INST-0001", "Deposito BCA", "ACTIVE",
		"AC", true, "IDR",
		"1000000.0000", uuid.Nil, false, jatuhTempo,
	)
}

func TestRepo_GetActiveAccruingInstrumens_WithRow(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).WillReturnRows(instrumenAkrualInfoRow())

	repo := NewRepo(db)
	insts, err := repo.GetActiveAccruingInstrumens(context.Background())
	require.NoError(t, err)
	require.Len(t, insts, 1)
	assert.Equal(t, testInstrID, insts[0].ID)
	assert.Equal(t, "INST-0001", insts[0].KodeInstrumen)
	assert.True(t, insts[0].GrossCarryingIDR.Equal(decimal.NewFromInt(1_000_000)))
}

func TestRepo_GetActiveMaturityInstrumens_WithRow(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).WithArgs(anyArgR).WillReturnRows(instrumenAkrualInfoRow())

	repo := NewRepo(db)
	insts, err := repo.GetActiveMaturityInstrumens(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, insts, 1)
	assert.Equal(t, testInstrID, insts[0].ID)
}

func TestRepo_GetInstrumenInfo_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).WithArgs(anyArgR).WillReturnRows(instrumenAkrualInfoRow())

	repo := NewRepo(db)
	info, err := repo.GetInstrumenInfo(context.Background(), testInstrID)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "AC", info.KlasifikasiPSAK71)
	assert.False(t, info.IsPOCI)
}

// ─── GetAmortisasiSchedule — with row ────────────────────────────────────────

func TestRepo_GetAmortisasiSchedule_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	effFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	effTo := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"schedule_version",
			"effective_from", "effective_to",
			"eir_str", "ca_eir_str", "kupon_str",
			"carrying_str", "premium_str", "diskon_str", "amort_str",
			"is_poci",
		}).AddRow(
			1,
			effFrom, effTo,
			"0.07500000", "", "0.08000000",
			"1000000.0000", "50000.0000", "0.0000", "273.9726",
			false,
		))

	repo := NewRepo(db)
	row, err := repo.GetAmortisasiSchedule(context.Background(), testInstrID, time.Now())
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.True(t, row.EIRPersen.Equal(decimal.NewFromFloat(0.075)))
	assert.Nil(t, row.CreditAdjustedEIR) // empty string → nil
	assert.True(t, row.PremiumSisa.Equal(decimal.NewFromFloat(50_000)))
	assert.True(t, row.AmortisasiHarian.IsPositive())
}

// ─── GetAkrualByID — found ────────────────────────────────────────────────────

func akrualRow(id uuid.UUID, status AkrualStatus) *sqlmock.Rows {
	stage := 1
	jurnalID := uuid.Nil
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{
		"id", "instrumen_id", "tanggal_akrual", "jenis", "stage",
		"carrying_basis_str", "eir_persen_str",
		"bunga_kotor_str", "pph_str", "bunga_bersih_str",
		"fx_rate_id", "mata_uang", "klasifikasi_snapshot",
		"ecl_run_id_used", "stale_staging_flag",
		"override_user_id", "override_comment",
		"jurnal_header_id", "status", "periode_bulanan_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		id, testInstrID, now.Truncate(24*time.Hour), JenisBunga, &stage,
		"1000000.0000", "0.07500000",
		"205.4795", "0.0000", "205.4795",
		nil, "IDR", "AC",
		nil, false,
		nil, nil,
		jurnalID, status, nil,
		now, testActorID, now, testActorID,
		nil, nil, int64(1), "TUGURE",
	)
}

func TestRepo_GetAkrualByID_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT id, instrumen_id`).
		WithArgs(anyArgR).
		WillReturnRows(akrualRow(testAkrualID, AkrualAutoPosted))

	repo := NewRepo(db)
	a, err := repo.GetAkrualByID(context.Background(), testAkrualID)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, testAkrualID, a.ID)
	assert.Equal(t, AkrualAutoPosted, a.Status)
	assert.True(t, a.BungaBersih.IsPositive())
}

// ─── GetLastAkrualForInstrumen — found ────────────────────────────────────────

func TestRepo_GetLastAkrualForInstrumen_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, instrumen_id, tanggal_akrual`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_akrual",
			"bunga_kotor_str", "pph_str", "bunga_bersih_str",
			"status", "created_at", "row_version", "tenant_id",
		}).AddRow(
			testAkrualID, testInstrID, now.Truncate(24*time.Hour),
			"205.4795", "0.0000", "205.4795",
			AkrualAutoPosted, now, int64(1), "TUGURE",
		))

	repo := NewRepo(db)
	a, err := repo.GetLastAkrualForInstrumen(context.Background(), testInstrID)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, testAkrualID, a.ID)
	assert.True(t, a.BungaKotor.IsPositive())
}

// ─── GetDividenByID — found ────────────────────────────────────────────────────

func TestRepo_GetDividenByID_Found(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, instrumen_id, tanggal_terima`).
		WithArgs(anyArgR).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_terima", "tanggal_cum_date",
			"jumlah_kotor_str", "pph_str", "jumlah_bersih_str",
			"klasifikasi_snapshot", "treatment", "is_reksadana",
			"status", "maker_id", "approver_id",
			"approve_comment", "reject_reason", "signature_method", "approved_at",
			"jurnal_header_id",
			"created_at", "created_by", "updated_at", "updated_by",
			"deleted_at", "deleted_by", "row_version", "tenant_id",
		}).AddRow(
			testDividenID, testInstrID, now, nil,
			"500000.0000", "75000.0000", "425000.0000",
			"FVOCI_ELECTION", "OCI", false,
			DividenPendingApproval, testActorID, nil,
			nil, nil, nil, nil,
			nil,
			now, testActorID, now, testActorID,
			nil, nil, int64(1), "TUGURE",
		))

	repo := NewRepo(db)
	d, err := repo.GetDividenByID(context.Background(), testDividenID)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, testDividenID, d.ID)
	assert.True(t, d.JumlahKotor.Equal(decimal.NewFromFloat(500_000)))
	assert.True(t, d.JumlahBersih.Equal(decimal.NewFromFloat(425_000)))
}

// ─── ListAkrual — with rows ────────────────────────────────────────────────────

func akrualListRow(id uuid.UUID) *sqlmock.Rows {
	// 23 columns matching actual SELECT in ListAkrual
	now := time.Now().UTC()
	stage := 1
	return sqlmock.NewRows([]string{
		"id", "instrumen_id", "tanggal_akrual", "jenis", "stage",
		"carrying_basis_str", "eir_persen_str",
		"bunga_kotor_str", "pph_str", "bunga_bersih_str",
		"fx_rate_id", "mata_uang", "klasifikasi_snapshot",
		"ecl_run_id_used", "stale_staging_flag",
		"jurnal_header_id", "status",
		"created_at", "created_by", "updated_at", "updated_by",
		"row_version", "tenant_id",
	}).AddRow(
		id, testInstrID, now.Truncate(24*time.Hour), JenisBunga, &stage,
		"1000000.0000", "0.07500000",
		"205.4795", "0.0000", "205.4795",
		nil, "IDR", "AC",
		nil, false,
		nil, AkrualAutoPosted,
		now, testActorID, now, testActorID,
		int64(1), "TUGURE",
	)
}

func TestRepo_ListAkrual_WithRows(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(akrualListRow(testAkrualID))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListAkrual(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
	assert.Equal(t, testAkrualID, rows[0].ID)
	assert.True(t, rows[0].BungaBersih.IsPositive())
}

func TestRepo_ListAkrual_HasMore(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// Return limit+1 rows to trigger hasMore=true
	r := sqlmock.NewRows([]string{
		"id", "instrumen_id", "tanggal_akrual", "jenis", "stage",
		"carrying_basis_str", "eir_persen_str",
		"bunga_kotor_str", "pph_str", "bunga_bersih_str",
		"fx_rate_id", "mata_uang", "klasifikasi_snapshot",
		"ecl_run_id_used", "stale_staging_flag",
		"jurnal_header_id", "status",
		"created_at", "created_by", "updated_at", "updated_by",
		"row_version", "tenant_id",
	})
	now := time.Now().UTC()
	stage := 1
	for i := 0; i < 3; i++ { // limit=2, returns 3 → hasMore
		r.AddRow(
			uuid.New(), testInstrID, now, JenisBunga, &stage,
			"1000000.0000", "0.07500000",
			"205.4795", "0.0000", "205.4795",
			nil, "IDR", "AC",
			nil, false,
			nil, AkrualAutoPosted,
			now, testActorID, now, testActorID,
			int64(1), "TUGURE",
		)
	}
	mock.ExpectQuery(`SELECT`).WillReturnRows(r)

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListAkrual(context.Background(), listquery.Query{}, "", 2)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.True(t, hasMore)
	assert.Equal(t, 2, total)
}

// ─── ListAkrual with cursor ────────────────────────────────────────────────────

func TestRepo_ListAkrual_WithCursor_Valid(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	// Build a valid cursor
	cursorTime := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	cursorID := uuid.New()
	cursor := encodeCursor(cursorTime, cursorID)

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_akrual", "jenis", "stage",
			"carrying_basis_str", "eir_persen_str",
			"bunga_kotor_str", "pph_str", "bunga_bersih_str",
			"fx_rate_id", "mata_uang", "klasifikasi_snapshot",
			"ecl_run_id_used", "stale_staging_flag",
			"jurnal_header_id", "status",
			"created_at", "created_by", "updated_at", "updated_by",
			"row_version", "tenant_id",
		}))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListAkrual(context.Background(), listquery.Query{}, cursor, 50)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

func TestRepo_ListAkrual_WithCursor_Invalid(t *testing.T) {
	db, _, cleanup := newRepoDB(t)
	defer cleanup()

	repo := NewRepo(db)
	_, _, _, err := repo.ListAkrual(context.Background(), listquery.Query{}, "!not-base64!", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor")
}

// ─── ListJatuhTempo — with rows ────────────────────────────────────────────────

func jatuhTempoListRow(id uuid.UUID) *sqlmock.Rows {
	// 19 columns matching actual SELECT in ListJatuhTempo
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{
		"id", "instrumen_id", "tanggal_jatuh_tempo", "jenis",
		"pokok_returned_str", "bunga_returned_str", "pph_str", "proceeds_str",
		"fx_rate_id", "klasifikasi_snapshot", "jurnal_header_id", "status", "error_message",
		"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
	}).AddRow(
		id, testInstrID, now, "PRINCIPAL",
		"1000000.0000", "5000.0000", "750.0000", "1004250.0000",
		nil, "AC", nil, JatuhTempoSettled, nil,
		now, testActorID, now, testActorID, int64(1), "TUGURE",
	)
}

func TestRepo_ListJatuhTempo_WithRows(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	jtID := uuid.New()
	mock.ExpectQuery(`SELECT`).WillReturnRows(jatuhTempoListRow(jtID))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListJatuhTempo(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
	assert.Equal(t, jtID, rows[0].ID)
	assert.True(t, rows[0].PokokReturned.IsPositive())
}

func TestRepo_ListJatuhTempo_WithCursor_Valid(t *testing.T) {
	db, mock, cleanup := newRepoDB(t)
	defer cleanup()

	cursorTime := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	cursorID := uuid.New()
	cursor := encodeCursor(cursorTime, cursorID)

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "tanggal_jatuh_tempo", "jenis",
			"pokok_returned_str", "bunga_returned_str", "pph_str", "proceeds_str",
			"fx_rate_id", "klasifikasi_snapshot", "jurnal_header_id", "status", "error_message",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.ListJatuhTempo(context.Background(), listquery.Query{}, cursor, 50)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

func TestRepo_ListJatuhTempo_WithCursor_Invalid(t *testing.T) {
	db, _, cleanup := newRepoDB(t)
	defer cleanup()

	repo := NewRepo(db)
	_, _, _, err := repo.ListJatuhTempo(context.Background(), listquery.Query{}, "bad!cursor", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor")
}

// ─── decodeCursor / encodeCursor direct unit tests ────────────────────────────

func TestDecodeCursor_ValidRoundtrip(t *testing.T) {
	ts := time.Date(2026, 6, 20, 9, 15, 30, 123456789, time.UTC)
	id := uuid.New()

	encoded := encodeCursor(ts, id)
	gotT, gotID, err := decodeCursor(encoded)
	require.NoError(t, err)
	assert.True(t, ts.Equal(gotT), "time should roundtrip")
	assert.Equal(t, id, gotID)
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := decodeCursor("!!!invalid!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64")
}

func b64enc(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestDecodeCursor_NoPipe(t *testing.T) {
	// Valid base64 but content has no "|" separator
	_, _, err := decodeCursor(b64enc("2026-06-20T09:15:30Z"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format invalid")
}

func TestDecodeCursor_InvalidUUID(t *testing.T) {
	// Valid time but invalid UUID part
	_, _, err := decodeCursor(b64enc("2026-06-20T09:15:30Z|not-a-uuid"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uuid")
}

func TestDecodeCursor_InvalidTime(t *testing.T) {
	// Valid separator but invalid time
	_, _, err := decodeCursor(b64enc("not-a-time|" + uuid.New().String()))
	require.Error(t, err)
}
