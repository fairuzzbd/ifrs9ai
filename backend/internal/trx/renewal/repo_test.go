package renewal

// repo_test.go — Unit tests for Repo using go-sqlmock.
// Tests key SQL paths without a real database.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

func newMockRepo(t *testing.T) (*Repo, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewRepo(db), mock, db
}

// ─── HasActiveRenewal ─────────────────────────────────────────────────────────

func TestRepo_HasActiveRenewal_True(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs(instrumenID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	has, err := repo.HasActiveRenewal(context.Background(), instrumenID)
	require.NoError(t, err)
	assert.True(t, has)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_HasActiveRenewal_False(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs(instrumenID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	has, err := repo.HasActiveRenewal(context.Background(), instrumenID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestRepo_HasActiveRenewal_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs(instrumenID).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.HasActiveRenewal(context.Background(), instrumenID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.HasActiveRenewal")
}

// ─── Insert ───────────────────────────────────────────────────────────────────

func TestRepo_Insert_HappyPath(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trx\.renewal`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	r := goodRenewal(StatusPendingApproval)
	err = repo.Insert(context.Background(), tx, r)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestRepo_Insert_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trx\.renewal`).
		WillReturnError(sql.ErrConnDone)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	r := goodRenewal(StatusPendingApproval)
	err = repo.Insert(context.Background(), tx, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.Insert")
}

// ─── UpdateStatus ─────────────────────────────────────────────────────────────

func TestRepo_UpdateStatus_HappyPath(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx\.renewal`).
		WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	approver := approverUUID
	reason := "approved"
	sig := "JWT_STEP_UP"
	now := time.Now()
	eir := decimal.NewFromFloat(0.05)

	update := StatusUpdate{
		Status:         StatusPosted,
		ApproverID:     &approver,
		ApproveReason:  &reason,
		SignatureMethod: &sig,
		ApprovedAt:     &now,
		EirBaru:        &eir,
		UpdatedBy:      approverUUID,
		RowVersion:     1,
	}
	err = repo.UpdateStatus(context.Background(), tx, renewalID, update)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestRepo_UpdateStatus_OptimisticLockConflict(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx\.renewal`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected = lock conflict

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	update := StatusUpdate{
		Status:     StatusPosted,
		UpdatedBy:  approverUUID,
		RowVersion: 99, // stale version
	}
	err = repo.UpdateStatus(context.Background(), tx, renewalID, update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "optimistic lock conflict")
}

func TestRepo_UpdateStatus_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx\.renewal`).
		WillReturnError(sql.ErrConnDone)

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	update := StatusUpdate{UpdatedBy: approverUUID, RowVersion: 1}
	err = repo.UpdateStatus(context.Background(), tx, renewalID, update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.UpdateStatus")
}

// ─── GetPeriodeByTanggal ──────────────────────────────────────────────────────

func TestRepo_GetPeriodeByTanggal_Found(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status_periode", "tanggal_mulai", "tanggal_akhir"}).
			AddRow(periodeID, "OPEN", start, end))

	tanggal := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	periode, err := repo.GetPeriodeByTanggal(context.Background(), tanggal)
	require.NoError(t, err)
	require.NotNil(t, periode)
	assert.Equal(t, "OPEN", periode.StatusPeriode)
}

func TestRepo_GetPeriodeByTanggal_NotFound(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status_periode", "tanggal_mulai", "tanggal_akhir"}))

	tanggal := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	periode, err := repo.GetPeriodeByTanggal(context.Background(), tanggal)
	require.NoError(t, err)
	assert.Nil(t, periode)
}

func TestRepo_GetPeriodeByTanggal_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, status_periode`).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.GetPeriodeByTanggal(context.Background(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.GetPeriodeByTanggal")
}

// ─── List ─────────────────────────────────────────────────────────────────────

// listCols is the column set returned by repo.List (matches SELECT in impl).
var listCols = []string{
	"id", "instrumen_lama_id", "instrumen_baru_id", "skema", "tenor_baru_bulan",
	"rate_baru_persen", "tanggal_efektif_baru", "tanggal_jatuh_tempo_baru",
	"pokok_lama", "pokok_baru", "bunga_kotor", "pph_amount", "bunga_bersih",
	"eir_baru_str", "status", "maker_id", "approver_id",
	"approve_reason", "reject_reason", "jurnal_header_id", "periode_bulanan_id",
	"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
}

// addListRow adds a minimal valid row to a sqlmock Rows set.
func addListRow(rows *sqlmock.Rows, id uuid.UUID, createdAt time.Time) *sqlmock.Rows {
	return rows.AddRow(
		id, instrumenID, nil, "POKOK_SAJA", int16(12),
		"7.0000", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC),
		"1000000000.0000", "1000000000.0000", "29753424.6575", "5950684.9315", "23802739.7260",
		"0.04800000",
		"PENDING_APPROVAL", makerUUID, nil,
		nil, nil, nil, nil,
		createdAt, makerUUID, createdAt, makerUUID, int64(1), "TUGURE",
	)
}

// TestRepo_List_Empty validates empty result set + no hasMore.
func TestRepo_List_Empty(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id`).
		WithArgs("TUGURE", 11). // tenant_id + limit+1
		WillReturnRows(sqlmock.NewRows(listCols))

	rows, hasMore, _, err := repo.List(context.Background(), listquery.Query{}, "", 10)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepo_List_TenantIDBinding asserts tenant_id is passed as first bind param.
func TestRepo_List_TenantIDBinding(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	now := time.Now().UTC()
	r1 := uuid.New()

	mock.ExpectQuery(`SELECT id`).
		WithArgs("TUGURE", 6). // tenant_id = $1, limit+1 = $2
		WillReturnRows(addListRow(sqlmock.NewRows(listCols), r1, now))

	rows, hasMore, count, err := repo.List(context.Background(), listquery.Query{}, "", 5)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Equal(t, 1, count)
	require.Len(t, rows, 1)
	assert.Equal(t, r1, rows[0].ID)
	assert.Equal(t, "TUGURE", rows[0].TenantID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepo_List_HasMoreTrue validates limit+1 trick detects next page.
func TestRepo_List_HasMoreTrue(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	now := time.Now().UTC()
	cols := sqlmock.NewRows(listCols)
	ids := make([]uuid.UUID, 3) // limit=2 → returns 3 rows (limit+1) → hasMore=true
	for i := range ids {
		ids[i] = uuid.New()
		addListRow(cols, ids[i], now.Add(time.Duration(-i)*time.Second))
	}

	mock.ExpectQuery(`SELECT id`).
		WithArgs("TUGURE", 3). // limit=2, fetch limit+1=3
		WillReturnRows(cols)

	rows, hasMore, count, err := repo.List(context.Background(), listquery.Query{}, "", 2)
	require.NoError(t, err)
	assert.True(t, hasMore, "should detect more rows via limit+1 trick")
	assert.Equal(t, 2, len(rows), "returned slice trimmed to limit")
	assert.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepo_List_WithCursor validates cursor-based keyset pagination args.
func TestRepo_List_WithCursor(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	// Build a cursor for a known (created_at, id) pair.
	cursorTime := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	cursorUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cursor := encodeCursor(cursorTime, cursorUUID)

	now := time.Now().UTC()

	// With cursor the query has 4 args: tenant_id, limit+1, cursorTime, cursorUUID.
	mock.ExpectQuery(`SELECT id`).
		WithArgs("TUGURE", 6, cursorTime, cursorUUID).
		WillReturnRows(addListRow(sqlmock.NewRows(listCols), uuid.New(), now))

	rows, hasMore, _, err := repo.List(context.Background(), listquery.Query{}, cursor, 5)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, rows, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepo_List_InvalidCursor returns an error without touching DB.
func TestRepo_List_InvalidCursor(t *testing.T) {
	repo, _, db := newMockRepo(t)
	defer db.Close()

	_, _, _, err := repo.List(context.Background(), listquery.Query{}, "not-valid-base64!!!", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cursor")
}

func TestRepo_List_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id`).
		WillReturnError(sql.ErrConnDone)

	_, _, _, err := repo.List(context.Background(), listquery.Query{}, "", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.List")
}

// ─── F3: encodeCursor / decodeCursor roundtrip ────────────────────────────────

// TestCursorRoundtrip validates encode→decode identity.
func TestCursorRoundtrip(t *testing.T) {
	ts := time.Date(2026, 6, 15, 10, 30, 0, 123456789, time.UTC)
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	encoded := encodeCursor(ts, id)
	decodedT, decodedID, err := decodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, ts.UTC().Format(time.RFC3339Nano), decodedT.UTC().Format(time.RFC3339Nano))
	assert.Equal(t, id, decodedID)
}

func TestDecodeCursor_BadBase64(t *testing.T) {
	_, _, err := decodeCursor("!!!not-base64!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64 decode")
}

func TestDecodeCursor_MissingPipe(t *testing.T) {
	// Encode a string that has no "|" separator.
	noPipe := base64.StdEncoding.EncodeToString([]byte("2026-01-01T00:00:00Z"))
	_, _, err := decodeCursor(noPipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor format invalid")
}

// ─── BeginTx ─────────────────────────────────────────────────────────────────

func TestRepo_BeginTx_Success(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	tx, err := repo.BeginTx(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_BeginTx_Error(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

	_, err := repo.BeginTx(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.BeginTx")
}

// ─── decimalPtrToStr (package-level) ─────────────────────────────────────────

func TestDecimalPtrToStr_Nil(t *testing.T) {
	result := decimalPtrToStr(nil, 8)
	assert.Nil(t, result)
}

func TestDecimalPtrToStr_NonNil(t *testing.T) {
	d := decimal.NewFromFloat(0.04800000)
	result := decimalPtrToStr(&d, 8)
	require.NotNil(t, result)
	s, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, "0.04800000", s)
}

// ─── NewRepo ─────────────────────────────────────────────────────────────────

func TestNewRepo(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewRepo(db)
	assert.NotNil(t, repo)
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

func TestRepo_GetByID_NotFound(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id`).
		WithArgs(renewalID).
		WillReturnRows(sqlmock.NewRows(nil)) // empty → ErrNoRows

	result, err := repo.GetByID(context.Background(), renewalID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestRepo_GetByID_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id`).
		WithArgs(renewalID).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.GetByID(context.Background(), renewalID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.GetByID")
}

func TestRepo_GetByID_HappyPath(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	rows := sqlmock.NewRows([]string{
		"id", "instrumen_lama_id", "instrumen_baru_id", "skema", "tenor_baru_bulan",
		"rate_baru_persen", "tanggal_efektif_baru", "tanggal_jatuh_tempo_baru",
		"pokok_lama", "pokok_baru", "bunga_kotor", "pph_amount", "bunga_bersih",
		"eir_baru_str",
		"status", "maker_id", "approver_id",
		"request_reason", "approve_reason", "reject_reason", "signature_method",
		"approved_at", "jurnal_header_id", "periode_bulanan_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		renewalID, instrumenID, nil, "POKOK_SAJA", int16(12),
		"7.0000", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC),
		"1000000000.0000", "1000000000.0000", "29753424.6575", "5950684.9315", "23802739.7260",
		"0.04800000",
		"PENDING_APPROVAL", makerUUID, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		now, makerUUID, now, makerUUID,
		nil, nil, int64(1), "TUGURE",
	)

	mock.ExpectQuery(`SELECT id`).
		WithArgs(renewalID).
		WillReturnRows(rows)

	result, err := repo.GetByID(context.Background(), renewalID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, renewalID, result.ID)
	assert.Equal(t, SkemaPokokSaja, result.Skema)
	assert.Equal(t, "7.0000", result.RateBaruPersen.StringFixed(4))
}

// ─── GetInstrumenInfo ─────────────────────────────────────────────────────────

func TestRepo_GetInstrumenInfo_NotFound(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, kode_instrumen`).
		WithArgs(instrumenID).
		WillReturnRows(sqlmock.NewRows(nil))

	result, err := repo.GetInstrumenInfo(context.Background(), instrumenID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestRepo_GetInstrumenInfo_DBError(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, kode_instrumen`).
		WithArgs(instrumenID).
		WillReturnError(sql.ErrConnDone)

	_, err := repo.GetInstrumenInfo(context.Background(), instrumenID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo.GetInstrumenInfo")
}

func TestRepo_GetInstrumenInfo_HappyPath(t *testing.T) {
	repo, mock, db := newMockRepo(t)
	defer db.Close()

	placementDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	maturityDate := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	cpartyID := uuid.New()
	portID := uuid.New()

	rows := sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen",
		"jenis_instrumen", "status", "klasifikasi_psak71",
		"klasifikasi_locked", "pokok_str", "rate_persen_str",
		"tanggal_penempatan", "tanggal_jatuh_tempo", "mata_uang",
		"counterparty_id", "portofolio_id",
		"sppi_test_run_id", "bm_assessment_id", "renewal_dari_instrumen_id",
	}).AddRow(
		instrumenID, "DEP-001", "Deposito BCA",
		"DEPOSITO", "ACTIVE", "AC",
		true, "1000000000.0000", "6.0000",
		placementDate, maturityDate, "IDR",
		cpartyID, portID,
		nil, nil, nil,
	)

	mock.ExpectQuery(`SELECT id, kode_instrumen`).
		WithArgs(instrumenID).
		WillReturnRows(rows)

	result, err := repo.GetInstrumenInfo(context.Background(), instrumenID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "DEP-001", result.KodeInstrumen)
	assert.Equal(t, "DEPOSITO", result.JenisInstrumen)
	assert.Equal(t, "1000000000.0000", result.Pokok.StringFixed(4))
}

// Prevent unused import
var _ = uuid.Nil
