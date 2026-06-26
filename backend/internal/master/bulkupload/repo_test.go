package bulkupload

// repo_test.go — Repository tests using DATA-DOG/go-sqlmock.
// Tests cover the sqlRepo implementation for sys.upload_batch and sys.upload_batch_row.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// compile-time interface compliance check
var _ Repository = (*mockRepoForHandler)(nil)
var _ Repository = (*mockRepo)(nil)

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// ─── InsertBatch ─────────────────────────────────────────────────────────────

func TestSQLRepo_InsertBatch(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.InsertBatch(context.Background(), tx, &Batch{
		ID:               batchID,
		BatchCode:        "BULK-001",
		BatchType:        "INSTRUMEN_BULK",
		FilenameOriginal: "test.xlsx",
		FileSHA256:       "abc123",
		FileStorageURL:   "",
		UploadedBy:       uuid.New(),
		UploadedAt:       now,
		TotalRows:        5,
		Status:           StatusParsed,
		CreatedAt:        now,
		CreatedBy:        uuid.New(),
		TenantID:         "TUGURE",
	})
	require.NoError(t, err)
	tx.Commit()

	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetBatch ─────────────────────────────────────────────────────────────────

func TestSQLRepo_GetBatch_Found(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	now := time.Now().UTC()

	// Columns match GetBatch SELECT: 26 columns
	rows := sqlmock.NewRows([]string{
		"id", "batch_code", "batch_type", "filename_original", "file_sha256",
		"file_storage_url", "uploaded_by", "uploaded_at", "total_rows", "valid_rows", "committed_rows",
		"sheet_breakdown_json", "status",
		"approver_id", "approved_at", "committed_at",
		"dry_run_cached_at", "dry_run_expires_at", "dry_run_result_jsonb",
		"rollback_status", "rollback_grace_expires_at", "rollback_by", "rollback_at", "rollback_reason",
		"created_at", "tenant_id",
	}).AddRow(
		batchID, "BULK-001", "INSTRUMEN_BULK", "test.xlsx", "sha256hash",
		"", uuid.New(), now, 5, 4, 4,
		[]byte(`{"Deposito":5}`), "PARSED",
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil, nil, nil,
		now, "TUGURE",
	)

	mock.ExpectQuery(`SELECT .* FROM sys.upload_batch WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(batchID, "TUGURE").
		WillReturnRows(rows)

	batch, err := repo.GetBatch(context.Background(), batchID, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, batch)
	assert.Equal(t, batchID, batch.ID)
	assert.Equal(t, StatusParsed, batch.Status)
	assert.Equal(t, "TUGURE", batch.TenantID)
}

func TestSQLRepo_GetBatch_NotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()

	mock.ExpectQuery(`SELECT .* FROM sys.upload_batch WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(batchID, "TUGURE").
		WillReturnError(sql.ErrNoRows)

	batch, err := repo.GetBatch(context.Background(), batchID, "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, batch)
}

// ─── UpdateBatchStatus ────────────────────────────────────────────────────────

func TestSQLRepo_UpdateBatchStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WithArgs(string(StatusCommitting), actor, batchID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchStatus(context.Background(), tx, batchID, StatusCommitting, actor)
	require.NoError(t, err)
	tx.Commit()
}

// ─── UpdateBatchDryRun ────────────────────────────────────────────────────────

func TestSQLRepo_UpdateBatchDryRun(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Hour)

	result := &DryRunResult{
		Status:      StatusDryRunPassed,
		TotalRows:   5,
		ValidRows:   5,
		InvalidRows: 0,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchDryRun(context.Background(), tx, batchID, result, expiresAt, actor)
	require.NoError(t, err)
	tx.Commit()
}

// ─── InsertBatchRows ──────────────────────────────────────────────────────────

func TestSQLRepo_InsertBatchRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	rowData, _ := json.Marshal(map[string]interface{}{"kode": "DEP-001"})

	rows := []BatchRow{
		{
			ID:          uuid.New(),
			BatchID:     batchID,
			RowNumber:   2,
			SheetName:   "Deposito",
			RowDataJson: rowData,
			RowStatus:   RowStatusPending,
			CreatedAt:   time.Now().UTC(),
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch_row`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.InsertBatchRows(context.Background(), tx, rows)
	require.NoError(t, err)
	tx.Commit()
}

func TestSQLRepo_InsertBatchRows_Empty(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	// No rows — should be no-op
	err := repo.InsertBatchRows(context.Background(), nil, []BatchRow{})
	require.NoError(t, err)
}

// ─── ListBatchRows ────────────────────────────────────────────────────────────

func TestSQLRepo_ListBatchRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	rowID := uuid.New()
	now := time.Now().UTC()
	rowData := []byte(`{"kode":"DEP-001"}`)

	rows := sqlmock.NewRows([]string{
		"id", "batch_id", "row_number", "sheet_name", "row_data_json",
		"row_status", "bulk_instrumen_id", "row_error_jsonb", "created_at",
	}).AddRow(
		rowID, batchID, 2, "Deposito", rowData,
		"PENDING", nil, nil, now,
	)

	mock.ExpectQuery(`SELECT .* FROM sys.upload_batch_row WHERE batch_id = \$1`).
		WithArgs(batchID, defaultLimit+1).
		WillReturnRows(rows)

	result, pag, err := repo.ListBatchRows(context.Background(), batchID, listquery.Query{}, "TUGURE")
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.False(t, pag.HasMore)
	assert.Equal(t, rowID, result[0].ID)
}

// ─── GetBatchRowsByStatus ─────────────────────────────────────────────────────

func TestSQLRepo_GetBatchRowsByStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "batch_id", "row_number", "sheet_name", "row_data_json",
		"row_status", "bulk_instrumen_id", "row_error_jsonb", "created_at",
	}).AddRow(
		uuid.New(), batchID, 2, "Deposito", []byte(`{}`),
		"PENDING", nil, nil, now,
	)

	mock.ExpectQuery(`SELECT .* FROM sys.upload_batch_row WHERE batch_id = \$1 AND row_status = \$2`).
		WithArgs(batchID, "PENDING").
		WillReturnRows(rows)

	result, err := repo.GetBatchRowsByStatus(context.Background(), batchID, RowStatusPending)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

// ─── UpdateRowStatus ─────────────────────────────────────────────────────────

func TestSQLRepo_UpdateRowStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rowID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch_row`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateRowStatus(context.Background(), tx, rowID, RowStatusCommitted, nil, nil)
	require.NoError(t, err)
	tx.Commit()
}

// ─── GetConfigParamInt ────────────────────────────────────────────────────────

func TestSQLRepo_GetConfigParamInt_Found(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"param_value"}).AddRow("7")

	mock.ExpectQuery(`SELECT param_value FROM sys.config_param WHERE param_key = \$1`).
		WithArgs("BULK_ROLLBACK_GRACE_DAYS").
		WillReturnRows(rows)

	val, err := repo.GetConfigParamInt(context.Background(), "BULK_ROLLBACK_GRACE_DAYS", 7)
	require.NoError(t, err)
	assert.Equal(t, 7, val)
}

func TestSQLRepo_GetConfigParamInt_NotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT param_value FROM sys.config_param WHERE param_key = \$1`).
		WithArgs("MISSING_PARAM").
		WillReturnError(sql.ErrNoRows)

	val, err := repo.GetConfigParamInt(context.Background(), "MISSING_PARAM", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, val) // returns default
}

// ─── GetActivePeriodeStatus ───────────────────────────────────────────────────

func TestSQLRepo_GetActivePeriodeStatus(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"status_periode"}).AddRow("OPEN")

	mock.ExpectQuery(`SELECT status_periode FROM mst.periode_buku`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	status, err := repo.GetActivePeriodeStatus(context.Background(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, "OPEN", status)
}

func TestSQLRepo_GetActivePeriodeStatus_NotFound(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT status_periode FROM mst.periode_buku`).
		WithArgs("TUGURE").
		WillReturnError(sql.ErrNoRows)

	status, err := repo.GetActivePeriodeStatus(context.Background(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, "OPEN", status) // defaults to OPEN when not found
}

// ─── Cross-ref lookups ────────────────────────────────────────────────────────

func TestSQLRepo_CounterpartyExists_True(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("CP-001", "TUGURE").
		WillReturnRows(rows)

	exists, err := repo.CounterpartyExists("CP-001", "TUGURE")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSQLRepo_BankExists_False(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("UNKNOWN", "TUGURE").
		WillReturnRows(rows)

	exists, err := repo.BankExists("UNKNOWN", "TUGURE")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSQLRepo_MataUangExists(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("IDR", "TUGURE").
		WillReturnRows(rows)

	exists, err := repo.MataUangExists("IDR", "TUGURE")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSQLRepo_InstrumenKodeExists(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("NEW-KODE", "TUGURE").
		WillReturnRows(rows)

	exists, err := repo.InstrumenKodeExists("NEW-KODE", "TUGURE")
	require.NoError(t, err)
	assert.False(t, exists)
}

// ─── UpdateBatchCommitted ─────────────────────────────────────────────────────

func TestSQLRepo_UpdateBatchCommitted_NoFailures(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchCommitted(context.Background(), tx, batchID, 5, 0, 7, actor)
	require.NoError(t, err)
	tx.Commit()
}

func TestSQLRepo_UpdateBatchCommitted_WithFailures(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchCommitted(context.Background(), tx, batchID, 3, 2, 7, actor) // 2 failures = PARTIAL_COMMIT
	require.NoError(t, err)
	tx.Commit()
}

// ─── UpdateBatchApproved ─────────────────────────────────────────────────────

func TestSQLRepo_UpdateBatchApproved(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	approverID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchApproved(context.Background(), tx, batchID, approverID, 5)
	require.NoError(t, err)
	tx.Commit()
}

// ─── UpdateBatchRollbackPending ───────────────────────────────────────────────

func TestSQLRepo_UpdateBatchRollbackPending(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchRollbackPending(context.Background(), tx, batchID, "rollback reason", actor)
	require.NoError(t, err)
	tx.Commit()
}

// ─── UpdateBatchRolledBack ────────────────────────────────────────────────────

func TestSQLRepo_UpdateBatchRolledBack(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchRolledBack(context.Background(), tx, batchID, actor, 5)
	require.NoError(t, err)
	tx.Commit()
}

// ─── UpdateRowsRolledBack ─────────────────────────────────────────────────────

func TestSQLRepo_UpdateRowsRolledBack(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch_row`).
		WithArgs(batchID).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	n, err := repo.UpdateRowsRolledBack(context.Background(), tx, batchID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	tx.Commit()
}

// ─── InsertInstrumen ─────────────────────────────────────────────────────────

func TestSQLRepo_InsertInstrumen(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()
	klasifikasi := "AC"

	row := RowValidationResult{
		SheetName:         SheetDeposito,
		RowNumber:         2,
		RowData:           map[string]interface{}{"kode": "DEP-INS", "mata_uang": "IDR"},
		Status:            RowStatusPending,
		KlasifikasiPsak71: &klasifikasi,
	}

	mock.ExpectBegin()
	// INSERT uses inline subquery for portofolio_id — no separate SELECT call
	mock.ExpectExec(`INSERT INTO mst.instrumen`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	id, err := repo.InsertInstrumen(context.Background(), tx, row, batchID, actor, "TUGURE")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
	tx.Commit()
}

func TestSQLRepo_InsertInstrumen_Flagged(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	row := RowValidationResult{
		SheetName: SheetDeposito,
		RowNumber: 2,
		RowData:   map[string]interface{}{"kode": "DEP-FLAG", "mata_uang": "IDR"},
		Status:    RowStatusFlaggedManualReview, // flagged → PENDING_CLASSIFICATION
	}

	mock.ExpectBegin()
	// INSERT uses inline subquery for portofolio_id — no separate SELECT call
	mock.ExpectExec(`INSERT INTO mst.instrumen`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	id, err := repo.InsertInstrumen(context.Background(), tx, row, batchID, actor, "TUGURE")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
	tx.Commit()
}

// ─── ActivateInstrumenByBatch ─────────────────────────────────────────────────

func TestSQLRepo_ActivateInstrumenByBatch(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.instrumen`).
		WithArgs(batchID).
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	n, err := repo.ActivateInstrumenByBatch(context.Background(), tx, batchID)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	tx.Commit()
}

// ─── SoftDeleteInstrumenByBatch ───────────────────────────────────────────────

func TestSQLRepo_SoftDeleteInstrumenByBatch(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.instrumen`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	tx, _ := db.BeginTx(context.Background(), nil)
	n, err := repo.SoftDeleteInstrumenByBatch(context.Background(), tx, batchID, actor)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	tx.Commit()
}

// ─── CountPendingManualByBatch ────────────────────────────────────────────────

func TestSQLRepo_CountPendingManualByBatch(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	batchID := uuid.New()
	rows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs(batchID).
		WillReturnRows(rows)

	count, err := repo.CountPendingManualByBatch(context.Background(), batchID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// ─── Error paths for repo methods ────────────────────────────────────────────

func TestSQLRepo_InsertBatch_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)
	batchID := uuid.New()
	actor := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch`).
		WillReturnError(fmt.Errorf("insert error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.InsertBatch(context.Background(), tx, &Batch{
		ID: batchID, UploadedBy: actor, TenantID: "TUGURE",
		FileSHA256: "abc", FilenameOriginal: "t.xlsx",
	})
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateBatchStatus_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchStatus(context.Background(), tx, uuid.New(), StatusParsed, uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateBatchDryRun_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	exp := time.Now().Add(time.Hour)
	err := repo.UpdateBatchDryRun(context.Background(), tx, uuid.New(), &DryRunResult{Status: StatusDryRunPassed}, exp, uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_InsertBatchRows_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)
	batchID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch_row`).
		WillReturnError(fmt.Errorf("insert error"))
	mock.ExpectRollback()

	rows := []BatchRow{
		{ID: uuid.New(), BatchID: batchID, SheetName: "Deposito", RowNumber: 2, RowStatus: RowStatusPending},
	}
	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.InsertBatchRows(context.Background(), tx, rows)
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_ListBatchRows_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query error"))

	_, _, err := repo.ListBatchRows(context.Background(), uuid.New(), listquery.Query{}, "TUGURE")
	assert.Error(t, err)
}

func TestSQLRepo_GetBatchRowsByStatus_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.GetBatchRowsByStatus(context.Background(), uuid.New(), RowStatusPending)
	assert.Error(t, err)
}

func TestSQLRepo_UpdateRowStatus_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch_row`).
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateRowStatus(context.Background(), tx, uuid.New(), RowStatusCommitted, nil, nil)
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateRowsRolledBack_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch_row`).
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err := repo.UpdateRowsRolledBack(context.Background(), tx, uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_InsertInstrumen_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	klasifikasi := "AC"
	row := RowValidationResult{
		SheetName: SheetDeposito, RowNumber: 2,
		RowData:           map[string]interface{}{"kode": "DEP-ERR", "mata_uang": "IDR"},
		Status:            RowStatusPending,
		KlasifikasiPsak71: &klasifikasi,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO mst.instrumen`).
		WillReturnError(fmt.Errorf("insert error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err := repo.InsertInstrumen(context.Background(), tx, row, uuid.New(), uuid.New(), "TUGURE")
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_ActivateInstrumenByBatch_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.instrumen`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err := repo.ActivateInstrumenByBatch(context.Background(), tx, uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_SoftDeleteInstrumenByBatch_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.instrumen`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err := repo.SoftDeleteInstrumenByBatch(context.Background(), tx, uuid.New(), uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_CountPendingManualByBatch_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.CountPendingManualByBatch(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestSQLRepo_GetConfigParamInt_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query error"))

	val, err := repo.GetConfigParamInt(context.Background(), "BULK_FILE_MAX_MB", 50)
	// Returns defaultVal + error on non-ErrNoRows errors
	assert.Error(t, err)
	assert.Equal(t, 50, val) // still returns default
}

func TestSQLRepo_CounterpartyExists_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.CounterpartyExists("CP-1", "TUGURE")
	assert.Error(t, err)
}

func TestSQLRepo_BankExists_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.BankExists("BCA", "TUGURE")
	assert.Error(t, err)
}

func TestSQLRepo_MataUangExists_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.MataUangExists("IDR", "TUGURE")
	assert.Error(t, err)
}

func TestSQLRepo_InstrumenKodeExists_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("query error"))

	_, err := repo.InstrumenKodeExists("DEP-001", "TUGURE")
	assert.Error(t, err)
}

func TestSQLRepo_UpdateBatchApproved_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchApproved(context.Background(), tx, uuid.New(), uuid.New(), 3)
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateBatchRollbackPending_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchRollbackPending(context.Background(), tx, uuid.New(), "reason", uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateBatchRolledBack_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchRolledBack(context.Background(), tx, uuid.New(), uuid.New(), 2)
	assert.Error(t, err)
	tx.Rollback()
}

func TestSQLRepo_UpdateBatchCommitted_Error(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewRepository(db).(*sqlRepo)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.upload_batch`).WillReturnError(fmt.Errorf("db error"))
	mock.ExpectRollback()

	tx, _ := db.BeginTx(context.Background(), nil)
	err := repo.UpdateBatchCommitted(context.Background(), tx, uuid.New(), 3, 0, 7, uuid.New())
	assert.Error(t, err)
	tx.Rollback()
}

// ─── NewRepository ────────────────────────────────────────────────────────────

func TestNewRepository(t *testing.T) {
	db, _ := newSQLMock(t)
	repo := NewRepository(db)
	assert.NotNil(t, repo)
}

// ─── repoLimit helper ─────────────────────────────────────────────────────────

func TestRepoLimit(t *testing.T) {
	assert.Equal(t, defaultLimit, repoLimit(listquery.Query{}))
}
