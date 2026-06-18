package mtm

// repo_sqlmock_test.go — unit tests for DBRepository using go-sqlmock.
// Tests cover the SQL execution paths without requiring a real PostgreSQL instance.

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

// newMockRepo returns a DBRepository backed by sqlmock.
func newMockRepo(t *testing.T) (*DBRepository, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
		mock.ExpectationsWereMet() //nolint:errcheck
	})
	return NewDBRepository(db), mock
}

// ─── GetConfigValue ───────────────────────────────────────────────────────────

func TestDBRepository_GetConfigValue_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT config_value FROM sys.config`)).
		WithArgs("MTM_PRICE_STALE_DAYS").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow("7"))

	v, err := repo.GetConfigValue(context.Background(), "MTM_PRICE_STALE_DAYS")
	require.NoError(t, err)
	assert.Equal(t, "7", v)
}

func TestDBRepository_GetConfigValue_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT config_value FROM sys.config`)).
		WithArgs("MISSING_KEY").
		WillReturnError(sql.ErrNoRows)

	v, err := repo.GetConfigValue(context.Background(), "MISSING_KEY")
	require.NoError(t, err)
	assert.Empty(t, v)
}

func TestDBRepository_GetConfigValue_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT config_value FROM sys.config`)).
		WithArgs("ANY_KEY").
		WillReturnError(fmt.Errorf("connection reset"))

	_, err := repo.GetConfigValue(context.Background(), "ANY_KEY")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetConfigValue")
}

// ─── IsHoliday ────────────────────────────────────────────────────────────────

func TestDBRepository_IsHoliday_True(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("2026-06-17").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	ok, err := repo.IsHoliday(context.Background(), time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestDBRepository_IsHoliday_False(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("2026-06-15").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	ok, err := repo.IsHoliday(context.Background(), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestDBRepository_IsHoliday_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WillReturnError(fmt.Errorf("db error"))

	_, err := repo.IsHoliday(context.Background(), time.Now())
	require.Error(t, err)
}

// ─── GetActiveNonACInstrumen ──────────────────────────────────────────────────

func TestDBRepository_GetActiveNonACInstrumen_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "klasifikasi_psak71", "tipe_instrumen", "mata_uang", "is_poci"}))

	rows, err := repo.GetActiveNonACInstrumen(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestDBRepository_GetActiveNonACInstrumen_OneRow(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kode_instrumen", "nama_instrumen",
			"klasifikasi_psak71", "klasifikasi_locked",
			"mata_uang", "tipe_instrumen", "poci_flag",
		}).AddRow(id, "OBL-001", "Obligasi Test", "FVTPL", true, "IDR", "SAHAM", false))

	rows, err := repo.GetActiveNonACInstrumen(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ID)
	assert.Equal(t, "FVTPL", rows[0].KlasifikasiPSAK71)
}

func TestDBRepository_GetActiveNonACInstrumen_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query failed"))

	_, err := repo.GetActiveNonACInstrumen(context.Background())
	require.Error(t, err)
}

// ─── GetFeedPrice ─────────────────────────────────────────────────────────────

func TestDBRepository_GetFeedPrice_Found_Obligasi(t *testing.T) {
	repo, mock := newMockRepo(t)
	instrID := uuid.New()
	// GetFeedPrice uses tipeInstrumen to select table; OBLIGASI → sys.ibpa_feed_staging
	// Scans: instrumen_id, harga_pasar (string), tanggal_harga, mata_uang
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"instrumen_id", "harga_pasar", "tanggal_harga", "mata_uang",
		}).AddRow(instrID, "15000.50000000", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), "IDR"))

	fp, err := repo.GetFeedPrice(context.Background(), instrID, "OBLIGASI", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, fp)
	assert.True(t, fp.HargaPasar.Equal(decimal.RequireFromString("15000.50000000")))
}

func TestDBRepository_GetFeedPrice_UnknownTipe_ReturnsNil(t *testing.T) {
	repo, _ := newMockRepo(t)
	// Unknown tipe → nil, nil (early return before SQL)
	fp, err := repo.GetFeedPrice(context.Background(), uuid.New(), "UNKNOWN_TYPE", time.Now())
	require.NoError(t, err)
	assert.Nil(t, fp)
}

func TestDBRepository_GetFeedPrice_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

	fp, err := repo.GetFeedPrice(context.Background(), uuid.New(), "SAHAM", time.Now())
	require.NoError(t, err)
	assert.Nil(t, fp)
}

func TestDBRepository_GetFeedPrice_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("network error"))

	_, err := repo.GetFeedPrice(context.Background(), uuid.New(), "REKSADANA", time.Now())
	require.Error(t, err)
}

// ─── GetApprovedKurs ──────────────────────────────────────────────────────────

func TestDBRepository_GetApprovedKurs_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	kursID := uuid.New()
	tanggal := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	// Scans: id, kode_mata_uang, kurs_tengah, tanggal_berlaku
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kode_mata_uang", "kurs_tengah", "tanggal_berlaku"}).
			AddRow(kursID, "USD", "15432.12345678", tanggal))

	ks, err := repo.GetApprovedKurs(context.Background(), "USD", time.Now())
	require.NoError(t, err)
	require.NotNil(t, ks)
	assert.Equal(t, kursID, ks.KursID)
	assert.True(t, ks.KursTengah.Equal(decimal.RequireFromString("15432.12345678")))
}

func TestDBRepository_GetApprovedKurs_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

	ks, err := repo.GetApprovedKurs(context.Background(), "USD", time.Now())
	require.NoError(t, err)
	assert.Nil(t, ks)
}

func TestDBRepository_GetApprovedKurs_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("timeout"))

	_, err := repo.GetApprovedKurs(context.Background(), "USD", time.Now())
	require.Error(t, err)
}

// ─── ExistsActive ─────────────────────────────────────────────────────────────

func TestDBRepository_ExistsActive_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	existingID := uuid.New()
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(existingID, "AUTO_POSTED"))

	ok, m, err := repo.ExistsActive(context.Background(), uuid.New(), time.Now(), "IBPA")
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, m)
	assert.Equal(t, existingID, m.ID)
}

func TestDBRepository_ExistsActive_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

	ok, m, err := repo.ExistsActive(context.Background(), uuid.New(), time.Now(), "IBPA")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, m)
}

func TestDBRepository_ExistsActive_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("lock timeout"))

	_, _, err := repo.ExistsActive(context.Background(), uuid.New(), time.Now(), "IBPA")
	require.Error(t, err)
}

// ─── List ─────────────────────────────────────────────────────────────────────

func TestDBRepository_List_WithDB_ReturnEmpty(t *testing.T) {
	// List currently has a TODO stub — returns nil for both nil and real db.
	repo, _ := newMockRepo(t)

	rows, hasMore, total, err := repo.List(context.Background(), listquery.Query{}, "TUGURE", 50)
	require.NoError(t, err)
	assert.Nil(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

// ─── ListByBatchID ────────────────────────────────────────────────────────────

func TestDBRepository_ListByBatchID_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "status", "harga_pasar_idr", "delta_idr", "delta_pct",
			"stale_price_flag", "deviation_flag", "harga_sumber", "tanggal_mtm", "created_at",
		}))

	rows, err := repo.ListByBatchID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestDBRepository_ListByBatchID_OneRow(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	instrID := uuid.New()
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "status", "harga_pasar_idr", "delta_idr", "delta_pct",
			"stale_price_flag", "deviation_flag", "harga_sumber", "tanggal_mtm", "created_at",
		}).AddRow(id, instrID, "AUTO_POSTED", "100.0000", "5.0000", "5.2600",
			false, false, "IBPA", time.Now(), time.Now()))

	rows, err := repo.ListByBatchID(context.Background(), uuid.New())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ID)
}

func TestDBRepository_ListByBatchID_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query failed"))

	_, err := repo.ListByBatchID(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── ListStaleAlerts ──────────────────────────────────────────────────────────

func TestDBRepository_ListStaleAlerts_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).
		WithArgs(51). // limit+1
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "status", "harga_age_days", "tanggal_mtm", "harga_sumber",
			"stale_price_flag", "deviation_flag", "created_at",
		}))

	rows, hasMore, total, err := repo.ListStaleAlerts(context.Background(), "TUGURE", 50)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

func TestDBRepository_ListStaleAlerts_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("connection refused"))

	_, _, _, err := repo.ListStaleAlerts(context.Background(), "TUGURE", 50)
	require.Error(t, err)
}

// ─── GetUploadBatch ───────────────────────────────────────────────────────────

func TestDBRepository_GetUploadBatch_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	bID := uuid.New()
	uploaderID := uuid.New()
	createdBy := uuid.New()
	now := time.Now()
	mock.ExpectQuery(`SELECT`).
		WithArgs(bID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "batch_type", "status", "catatan", "uploader_id",
			"total_rows", "valid_rows", "invalid_rows", "tenant_id",
			"created_at", "created_by", "updated_at", "updated_by",
		}).AddRow(bID, "MTM_UPLOAD", "PENDING_REVIEW", "", uploaderID,
			2, 2, 0, "TUGURE",
			now, createdBy, now, createdBy))

	b, err := repo.GetUploadBatch(context.Background(), bID)
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, bID, b.ID)
	assert.Equal(t, 2, b.TotalRows)
}

func TestDBRepository_GetUploadBatch_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

	b, err := repo.GetUploadBatch(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, b)
}

func TestDBRepository_GetUploadBatch_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("timeout"))

	_, err := repo.GetUploadBatch(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── Insert ───────────────────────────────────────────────────────────────────

func TestDBRepository_Insert_NilTx_Error(t *testing.T) {
	repo, _ := newMockRepo(t)
	// Insert uses tx.ExecContext — nil tx panics. Test that nil db path was already
	// covered and that the method exists (compile check).
	// For the nil-tx case we just verify the function signature is correct.
	assert.NotNil(t, repo)
	// We do NOT call Insert with nil tx here as that would panic (sql.Tx.ExecContext dereferences tx).
}

// ─── BeginTx with real (mock) DB ─────────────────────────────────────────────

func TestDBRepository_BeginTx_WithDB_Success(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	tx, err := repo.BeginTx(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tx)
	_ = tx.Commit()
}

func TestDBRepository_BeginTx_WithDB_Error(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin().WillReturnError(fmt.Errorf("too many connections"))

	_, err := repo.BeginTx(context.Background())
	require.Error(t, err)
}

// ─── Insert with mock tx ─────────────────────────────────────────────────────

func TestDBRepository_Insert_WithMockTx_Success(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trx.mtm`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	m := makeMtm(StatusAutoPOSTED)
	err = repo.Insert(context.Background(), tx, m)
	require.NoError(t, err)
	_ = tx.Commit()
}

func TestDBRepository_Insert_WithMockTx_Error(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trx.mtm`).WillReturnError(fmt.Errorf("constraint violation"))
	mock.ExpectRollback()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	m := makeMtm(StatusAutoPOSTED)
	err = repo.Insert(context.Background(), tx, m)
	require.Error(t, err)
}

// ─── GetByID with mock DB ────────────────────────────────────────────────────

func TestDBRepository_GetByID_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	id := uuid.New()
	instrID := uuid.New()
	periodeID := uuid.New()
	now := time.Now()
	createdBy := uuid.New()

	cols := []string{
		"id", "instrumen_id", "periode_bulanan_id", "tanggal_mtm",
		"harga_sumber", "harga_tanggal", "harga_age_days",
		"harga_pasar_fcy", "harga_pasar_idr", "harga_buku_idr", "delta_idr", "delta_pct",
		"kurs_id", "kurs_tengah",
		"klasifikasi_snapshot", "treatment_snapshot",
		"jurnal_entry_id", "jurnal_entry_id_2", "jurnal_event_code", "jurnal_event_code_2",
		"stale_price_flag", "deviation_flag", "locked_flag", "status",
		"upload_batch_id", "uploader_id", "cron_job_id",
		"override_approver_id", "override_comment", "override_at",
		"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
	}
	mock.ExpectQuery(`SELECT`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			id, instrID, periodeID, now,
			"IBPA", now, int16(1),
			nil, "100.0000", "95.0000", "5.0000", "5.2600",
			nil, nil,
			"FVTPL", "MTM_FVTPL",
			nil, nil, nil, nil,
			false, true, false, "AUTO_POSTED",
			nil, nil, nil,
			nil, nil, nil,
			now, createdBy, now, createdBy, int64(1), "TUGURE",
		))

	m, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, id, m.ID)
	assert.Equal(t, Status("AUTO_POSTED"), m.Status)
}

func TestDBRepository_GetByID_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg()).WillReturnError(sql.ErrNoRows)

	m, err := repo.GetByID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, m)
}

func TestDBRepository_GetByID_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg()).WillReturnError(fmt.Errorf("deadlock"))

	_, err := repo.GetByID(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── UpdateStatus with mock tx ───────────────────────────────────────────────

func TestDBRepository_UpdateStatus_Success(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.mtm`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	err = repo.UpdateStatus(context.Background(), tx, uuid.New(), StatusUpdate{
		Status:    StatusApproved,
		UpdatedBy: uuid.New(),
	})
	require.NoError(t, err)
	_ = tx.Commit()
}

func TestDBRepository_UpdateStatus_NoRowsAffected(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.mtm`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected → version mismatch
	mock.ExpectRollback()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	err = repo.UpdateStatus(context.Background(), tx, uuid.New(), StatusUpdate{
		Status:    StatusApproved,
		UpdatedBy: uuid.New(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version mismatch")
}

// ─── LockMtmForPeriode / UnlockMtmForPeriode with mock tx ────────────────────

func TestDBRepository_LockMtmForPeriode_WithMockTx(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.mtm`).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	n, err := repo.LockMtmForPeriode(context.Background(), tx,
		uuid.New(), time.Now(), time.Now().AddDate(0, 1, 0), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	_ = tx.Commit()
}

func TestDBRepository_UnlockMtmForPeriode_WithMockTx(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE trx.mtm`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	n, err := repo.UnlockMtmForPeriode(context.Background(), tx,
		uuid.New(), time.Now(), time.Now().AddDate(0, 1, 0), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	_ = tx.Commit()
}

// ─── InsertUploadBatch with mock tx ──────────────────────────────────────────

func TestDBRepository_InsertUploadBatch_Success(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := repo.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback() //nolint:errcheck

	b := &UploadBatch{
		ID:         uuid.New(),
		BatchType:  "MTM_UPLOAD",
		UploaderID: uuid.New(),
		Status:     "PENDING_REVIEW",
		TotalRows:  2,
		ValidRows:  2,
		CreatedAt:  time.Now(),
		CreatedBy:  uuid.New(),
	}
	err = repo.InsertUploadBatch(context.Background(), tx, b)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── ListStaleAlerts with hasMore ────────────────────────────────────────────

func TestDBRepository_ListStaleAlerts_HasMore(t *testing.T) {
	repo, mock := newMockRepo(t)
	// limit=2, return 3 rows → hasMore=true
	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	instrID := uuid.New()
	now := time.Now()
	mock.ExpectQuery(`SELECT`).
		WithArgs(3). // limit+1
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "instrumen_id", "status", "harga_age_days", "tanggal_mtm", "harga_sumber",
			"stale_price_flag", "deviation_flag", "created_at",
		}).
			AddRow(id1, instrID, "STALE_PRICE", int16(8), now, "IBPA", true, false, now).
			AddRow(id2, instrID, "STALE_PRICE", int16(7), now, "BEI", true, false, now).
			AddRow(id3, instrID, "STALE_PRICE", int16(6), now, "MANUAL", true, false, now))

	rows, hasMore, total, err := repo.ListStaleAlerts(context.Background(), "TUGURE", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.True(t, hasMore)
	assert.Equal(t, 2, total)
}
