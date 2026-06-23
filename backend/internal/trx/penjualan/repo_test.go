package penjualan

// repo_test.go — Repo unit tests using go-sqlmock.
// Covers happy path + error paths for each public repo method.
// No real DB; all SQL verified via sqlmock.
//
// M7 F2 lesson: scan NUMERIC as ::text then decimal.NewFromString.
// Cursor: base64(created_at_rfc3339nano|uuid) keyset.

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return db, mock, func() { _ = db.Close() }
}

var testPjID = uuid.New()
var testInstID2 = uuid.New()
var testMakerID2 = uuid.New()
var testPeriodeID2 = uuid.New()
var testPortoID2 = uuid.New()
var anyArg = sqlmock.AnyArg()

// minimalPenjualanRow returns a sqlmock.Rows with all GetByID columns populated.
func minimalPenjualanRow(id uuid.UUID, makerID uuid.UUID) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{
		"id", "instrumen_id", "jenis_disposal",
		"qty_terjual", "qty_holding_pre", "qty_holding_post_str",
		"harga_jual_per_unit", "proceed", "cost_basis", "realized_gl",
		"oci_recycled_str", "oci_cumulative_str",
		"klasifikasi_snapshot", "jurnal_event_code", "tanggal_eksekusi",
		"bm_violation_risk", "bm_violation_pct_str",
		"status", "maker_id", "approver_id",
		"approve_comment", "reject_reason", "signature_method",
		"approved_at", "jurnal_header_id", "periode_bulanan_id", "instrumen_status_after",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		id.String(), testInstID2.String(), "PARTIAL",
		"500.00000000", "1000.00000000", "",
		"1100.0000", "550000.0000", "490000.0000", "60000.0000",
		"", "",
		"AC", nil, now,
		false, "",
		"PENDING_APPROVAL", makerID.String(), nil,
		nil, nil, nil,
		nil, nil, nil, nil,
		now, makerID, now, makerID,
		nil, nil, 1, "TUGURE",
	)
}

// ─── BeginTx ─────────────────────────────────────────────────────────────────

func TestRepo_BeginTx(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	repo := NewRepo(db)
	tx, err := repo.BeginTx(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tx)
	_ = tx.Rollback()
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

func TestRepo_GetByID_Found(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.penjualan`).
		WithArgs(testPjID).
		WillReturnRows(minimalPenjualanRow(testPjID, testMakerID2))

	repo := NewRepo(db)
	pj, err := repo.GetByID(context.Background(), testPjID)
	require.NoError(t, err)
	require.NotNil(t, pj)
	assert.Equal(t, testPjID, pj.ID)
	assert.Equal(t, "500.00000000", pj.QtyTerjual.StringFixed(8))
}

func TestRepo_GetByID_NotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.penjualan`).
		WithArgs(testPjID).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	pj, err := repo.GetByID(context.Background(), testPjID)
	require.NoError(t, err)
	assert.Nil(t, pj)
}

func TestRepo_GetByID_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.penjualan`).
		WithArgs(testPjID).
		WillReturnError(fmt.Errorf("db error"))

	repo := NewRepo(db)
	_, err := repo.GetByID(context.Background(), testPjID)
	require.Error(t, err)
}

// ─── GetInstrumenInfo ─────────────────────────────────────────────────────────

func TestRepo_GetInstrumenInfo_Found(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen",
		"status", "klasifikasi_psak71", "klasifikasi_locked",
		"qty_holding_str", "harga_perolehan_str",
		"portofolio_id", "business_model", "mata_uang",
		"counterparty_id", "sppi_test_run_id", "bm_assessment_id",
	}).AddRow(
		testInstID2.String(), "OBL-001", "Obligasi Test",
		"ACTIVE", "AC", true,
		"1000.00000000", "900000000.0000",
		testPortoID2.String(), "HTC&S", "IDR",
		uuid.New().String(), nil, nil,
	)

	mock.ExpectQuery(`FROM mst.instrumen i`).
		WithArgs(testInstID2).
		WillReturnRows(rows)

	repo := NewRepo(db)
	inst, err := repo.GetInstrumenInfo(context.Background(), testInstID2)
	require.NoError(t, err)
	require.NotNil(t, inst)
	assert.Equal(t, "OBL-001", inst.KodeInstrumen)
	assert.Equal(t, "ACTIVE", inst.Status)
	assert.True(t, inst.KlasifikasiLocked)
}

func TestRepo_GetInstrumenInfo_NotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM mst.instrumen i`).
		WithArgs(testInstID2).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	inst, err := repo.GetInstrumenInfo(context.Background(), testInstID2)
	require.NoError(t, err)
	assert.Nil(t, inst)
}

// ─── HasActivePenjualan ──────────────────────────────────────────────────────

func TestRepo_HasActivePenjualan_True(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	repo := NewRepo(db)
	has, err := repo.HasActivePenjualan(context.Background(), testInstID2)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestRepo_HasActivePenjualan_False(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := NewRepo(db)
	has, err := repo.HasActivePenjualan(context.Background(), testInstID2)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestRepo_HasActivePenjualan_Error(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)")).
		WithArgs(testInstID2).
		WillReturnError(fmt.Errorf("db error"))

	repo := NewRepo(db)
	_, err := repo.HasActivePenjualan(context.Background(), testInstID2)
	require.Error(t, err)
}

// ─── GetOCICumulativeByInstrumen ─────────────────────────────────────────────

func TestRepo_GetOCICumulativeByInstrumen_Found(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.mtm`).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"oci"}).AddRow("12500000.0000"))

	repo := NewRepo(db)
	oci, err := repo.GetOCICumulativeByInstrumen(context.Background(), testInstID2)
	require.NoError(t, err)
	assert.Equal(t, "12500000.0000", oci.StringFixed(4))
}

func TestRepo_GetOCICumulativeByInstrumen_NotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.mtm`).
		WithArgs(testInstID2).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	oci, err := repo.GetOCICumulativeByInstrumen(context.Background(), testInstID2)
	require.NoError(t, err)
	assert.True(t, oci.IsZero())
}

// ─── GetAmortizedCarryingByInstrumen ─────────────────────────────────────────

func TestRepo_GetAmortizedCarrying_Stage1_GrossCarrying(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	// Query 1: amortisasi_schedule returns gross carrying.
	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnRows(sqlmock.NewRows([]string{"carrying"}).AddRow("900000000.0000"))
	// Query 2: ECL lookup returns Stage 1, ecl_allowance=0.
	mock.ExpectQuery(`FROM ecl.calc_result_line`).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_stage", "ecl_allowance"}).AddRow(1, "0.0000"))

	repo := NewRepo(db)
	c, stage, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "900000000.0000", c.StringFixed(4), "Stage 1: must return gross carrying")
	assert.Equal(t, 1, stage)
}

func TestRepo_GetAmortizedCarrying_Stage3_NetCarrying(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	// Query 1: gross carrying from schedule.
	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnRows(sqlmock.NewRows([]string{"carrying"}).AddRow("900000000.0000"))
	// Query 2: ECL lookup returns Stage 3 with ECL allowance.
	mock.ExpectQuery(`FROM ecl.calc_result_line`).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_stage", "ecl_allowance"}).AddRow(3, "100000000.0000"))

	repo := NewRepo(db)
	c, stage, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.NoError(t, err)
	// net = 900M - 100M = 800M
	assert.Equal(t, "800000000.0000", c.StringFixed(4), "Stage 3: must return net carrying (gross - sealed ECL)")
	assert.Equal(t, 3, stage)
}

func TestRepo_GetAmortizedCarrying_NoSealedECL_FallsBackToGross(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	// Query 1: amortisasi_schedule has carrying.
	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnRows(sqlmock.NewRows([]string{"carrying"}).AddRow("900000000.0000"))
	// Query 2: no sealed ECL run exists.
	mock.ExpectQuery(`FROM ecl.calc_result_line`).
		WithArgs(testInstID2).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	c, stage, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "900000000.0000", c.StringFixed(4), "No sealed ECL: must fall back to gross carrying")
	assert.Equal(t, 0, stage, "stageUsed must be 0 when no sealed ECL context found")
}

func TestRepo_GetAmortizedCarrying_NotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	c, stage, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.NoError(t, err)
	assert.True(t, c.IsZero())
	assert.Equal(t, 0, stage)
}

func TestRepo_GetAmortizedCarrying_ECLLookupError(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnRows(sqlmock.NewRows([]string{"carrying"}).AddRow("900000000.0000"))
	mock.ExpectQuery(`FROM ecl.calc_result_line`).
		WithArgs(testInstID2).
		WillReturnError(fmt.Errorf("db timeout"))

	repo := NewRepo(db)
	_, _, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ecl lookup")
}

func TestRepo_GetAmortizedCarrying_Stage3_ECLExceedsGross_ClampsToZero(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM ecl.amortisasi_schedule`).
		WithArgs(testInstID2, anyArg).
		WillReturnRows(sqlmock.NewRows([]string{"carrying"}).AddRow("100000.0000"))
	// ECL allowance exceeds gross — net would be negative; must clamp to zero.
	mock.ExpectQuery(`FROM ecl.calc_result_line`).
		WithArgs(testInstID2).
		WillReturnRows(sqlmock.NewRows([]string{"ecl_stage", "ecl_allowance"}).AddRow(3, "200000.0000"))

	repo := NewRepo(db)
	c, stage, err := repo.GetAmortizedCarryingByInstrumen(context.Background(), testInstID2, time.Now())
	require.NoError(t, err)
	assert.True(t, c.IsZero(), "net carrying must clamp to zero when ECL > gross")
	assert.Equal(t, 3, stage)
}

// ─── GetRolling12mDisposalIDR ─────────────────────────────────────────────────

func TestRepo_GetRolling12mDisposal(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`SUM\(p.proceed\)`).
		WithArgs(testPortoID2).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("50000000.0000"))

	repo := NewRepo(db)
	v, err := repo.GetRolling12mDisposalIDR(context.Background(), testPortoID2)
	require.NoError(t, err)
	assert.Equal(t, "50000000.0000", v.StringFixed(4))
}

// ─── GetPortofolioNilai ───────────────────────────────────────────────────────

func TestRepo_GetPortofolioNilai(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`SUM\(qty_holding \* harga_perolehan\)`).
		WithArgs(testPortoID2).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow("1000000000.0000"))

	repo := NewRepo(db)
	v, err := repo.GetPortofolioNilai(context.Background(), testPortoID2)
	require.NoError(t, err)
	assert.Equal(t, "1000000000.0000", v.StringFixed(4))
}

// ─── GetBMConfigThresholds ────────────────────────────────────────────────────

func TestRepo_GetBMConfigThresholds_Found(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM sys.config_param`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).
			AddRow("PENJUALAN_BM_WARN_THRESHOLD_PCT", "5.0").
			AddRow("PENJUALAN_BM_BLOCK_THRESHOLD_PCT", "10.0"),
		)

	repo := NewRepo(db)
	warn, block, err := repo.GetBMConfigThresholds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "5.0000", warn.StringFixed(4))
	assert.Equal(t, "10.0000", block.StringFixed(4))
}

func TestRepo_GetBMConfigThresholds_Empty_UsesDefaults(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM sys.config_param`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))

	repo := NewRepo(db)
	warn, block, err := repo.GetBMConfigThresholds(context.Background())
	require.NoError(t, err)
	// defaults: 5 and 10
	assert.Equal(t, "5", warn.String())
	assert.Equal(t, "10", block.String())
}

// ─── Insert ───────────────────────────────────────────────────────────────────

func TestRepo_Insert_Success(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trx.penjualan`).
		WithArgs(anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepo(db)
	tx, err := db.Begin()
	require.NoError(t, err)

	now := time.Now().UTC()
	pj := &Penjualan{
		ID:                  uuid.New(),
		InstrumenID:         testInstID2,
		JenisDisposal:       DisposalPartial,
		QtyTerjual:          decimal.NewFromInt(500),
		QtyHoldingPre:       decimal.NewFromInt(1000),
		HargaJualPerUnit:    decimal.NewFromInt(1100),
		Proceed:             decimal.NewFromInt(550000),
		CostBasis:           decimal.NewFromInt(490000),
		RealizedGL:          decimal.NewFromInt(60000),
		KlasifikasiSnapshot: KlasifikasiAC,
		TanggalEksekusi:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Status:              StatusPendingApproval,
		MakerID:             testMakerID2,
		CreatedAt:           now,
		CreatedBy:           testMakerID2,
		UpdatedAt:           now,
		UpdatedBy:           testMakerID2,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}
	err = repo.Insert(context.Background(), tx, pj)
	require.NoError(t, err)
}

// ─── UpdateStatus ────────────────────────────────────────────────────────────

func TestRepo_UpdateStatus_Success(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.penjualan SET`).
		WithArgs(
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			anyArg, anyArg, anyArg, anyArg, anyArg,
			testPjID, int64(1),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewRepo(db)
	tx, err := db.Begin()
	require.NoError(t, err)

	approverID := testApproverID
	u := StatusUpdate{
		Status:     StatusPosted,
		ApproverID: &approverID,
		UpdatedBy:  testApproverID,
		RowVersion: 1,
	}
	err = repo.UpdateStatus(context.Background(), tx, testPjID, u)
	require.NoError(t, err)
}

func TestRepo_UpdateStatus_OptimisticLockFail(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.penjualan SET`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected = lock conflict

	repo := NewRepo(db)
	tx, err := db.Begin()
	require.NoError(t, err)

	u := StatusUpdate{Status: StatusPosted, UpdatedBy: testApproverID, RowVersion: 99}
	err = repo.UpdateStatus(context.Background(), tx, testPjID, u)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "optimistic lock")
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestRepo_List_NoCursor(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	now := time.Now().UTC()
	mock.ExpectQuery(`FROM trx.penjualan`).
		WithArgs("TUGURE", 51).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "jenis_disposal",
			"qty_terjual", "qty_holding_pre", "qty_holding_post_str",
			"harga_jual_per_unit", "proceed", "cost_basis", "realized_gl",
			"oci_recycled_str", "oci_cumulative_str",
			"klasifikasi_snapshot", "jurnal_event_code", "tanggal_eksekusi",
			"bm_violation_risk", "bm_pct_str",
			"status", "maker_id", "approver_id",
			"approve_comment", "reject_reason", "jurnal_header_id", "periode_bulanan_id",
			"instrumen_status_after",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			testPjID.String(), testInstID2.String(), "PARTIAL",
			"500.00000000", "1000.00000000", "",
			"1100.0000", "550000.0000", "490000.0000", "60000.0000",
			"", "",
			"AC", nil, now,
			false, "",
			"PENDING_APPROVAL", testMakerID2.String(), nil,
			nil, nil, nil, nil,
			nil,
			now, testMakerID2, now, testMakerID2, int64(1), "TUGURE",
		))

	repo := NewRepo(db)
	rows, hasMore, total, err := repo.List(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
}

func TestRepo_List_InvalidCursor_Error(t *testing.T) {
	db, _, cleanup := newMockDB(t)
	defer cleanup()

	repo := NewRepo(db)
	_, _, _, err := repo.List(context.Background(), listquery.Query{}, "not-base64!!", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor")
}

// ─── GetPeriodeByTanggal ─────────────────────────────────────────────────────

func TestRepo_GetPeriodeByTanggal_Open(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(anyArg).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "status_periode", "tanggal_mulai", "tanggal_akhir",
		}).AddRow(
			testPeriodeID2.String(), "OPEN",
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		))

	repo := NewRepo(db)
	p, err := repo.GetPeriodeByTanggal(context.Background(), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "OPEN", p.StatusPeriode)
}

func TestRepo_GetPeriodeByTanggal_NotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(anyArg).
		WillReturnError(sql.ErrNoRows)

	repo := NewRepo(db)
	p, err := repo.GetPeriodeByTanggal(context.Background(), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Nil(t, p)
}

// ─── Cursor helpers ──────────────────────────────────────────────────────────

func TestDecodeCursor_RoundTrip(t *testing.T) {
	id := uuid.New()
	ts := time.Now().UTC()
	encoded := encodeCursor(ts, id)
	tsDec, idDec, err := decodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, id, idDec)
	assert.True(t, ts.Equal(tsDec) || ts.Round(time.Nanosecond).Equal(tsDec))
}

func TestDecodeCursor_Invalid_Error(t *testing.T) {
	_, _, err := decodeCursor("!!!invalid!!!")
	require.Error(t, err)
}

func TestDecodeCursor_MissingPipe(t *testing.T) {
	import64 := "bm8tcGlwZS1oZXJl" // base64("no-pipe-here")
	_, _, err := decodeCursor(import64)
	require.Error(t, err)
}

// ─── decimalPtrToStr ─────────────────────────────────────────────────────────

func TestDecimalPtrToStr_Nil(t *testing.T) {
	result := decimalPtrToStr(nil, 4)
	assert.Nil(t, result)
}

func TestDecimalPtrToStr_NonNil(t *testing.T) {
	v := decimal.NewFromFloat(1234.5678)
	result := decimalPtrToStr(&v, 4)
	assert.Equal(t, "1234.5678", result)
}

// ─── ListBMAlerts ─────────────────────────────────────────────────────────────

func TestRepo_ListBMAlerts_Empty(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	mock.ExpectQuery(`FROM trx.penjualan p`).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "kode_instrumen",
			"portofolio_id", "portofolio_nama",
			"pct", "warn_threshold", "block_threshold", "flag_status", "updated_at",
		}))

	repo := NewRepo(db)
	alerts, err := repo.ListBMAlerts(context.Background(), decimal.NewFromInt(5), decimal.NewFromInt(10))
	require.NoError(t, err)
	assert.Empty(t, alerts)
}

func TestRepo_ListBMAlerts_Found(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	now := time.Now().UTC()
	mock.ExpectQuery(`FROM trx.penjualan p`).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "kode_instrumen",
			"portofolio_id", "portofolio_nama",
			"pct", "warn_threshold", "block_threshold", "flag_status", "updated_at",
		}).AddRow(
			testInstID2.String(), "OBL-001",
			testPortoID2.String(), "Portofolio HTC",
			"6.5000", "5.0", "10.0", "BM_VIOLATION_RISK", now,
		))

	repo := NewRepo(db)
	alerts, err := repo.ListBMAlerts(context.Background(), decimal.NewFromInt(5), decimal.NewFromInt(10))
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "OBL-001", alerts[0].InstrumenKode)
	assert.Equal(t, "6.5000", alerts[0].CumulativeSold12mPct)
}
