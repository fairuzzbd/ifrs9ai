package reporting

// coverage_boost_test.go — white-box tests targeting low-coverage statements.
// Focus: refreshOneMV infra-error path, HandleExportAsync mid-paths, ListExportLogs
// cursor + rows.Err, RequestExport BeginTx fail, GetExportDownload update fail,
// NewMVRefreshTask, NewScheduledEmailTask, CountMVRows tenant-not-found.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── refreshOneMV — REFRESH MATERIALIZED VIEW exec fails (infra error) ───────

func TestRefreshOneMV_RefreshExecFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_gl_delivery_status"

	// Begin + InsertRefreshLog + Commit
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// pg_try_advisory_lock → true (acquired)
	mock.ExpectQuery(`SELECT pg_try_advisory_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))

	// REFRESH MATERIALIZED VIEW → infra error
	mock.ExpectExec(`REFRESH MATERIALIZED VIEW CONCURRENTLY`).
		WillReturnError(assert.AnError)

	// pg_advisory_unlock (deferred best-effort)
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// UpdateRefreshLog FAILED
	mock.ExpectExec(`UPDATE sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	err = w.refreshOneMV(context.Background(), mvName, TriggeredByManual, "", "TUGURE")
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── refreshOneMV — InsertRefreshLog fails ────────────────────────────────────

func TestRefreshOneMV_InsertLogFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_status_periode"

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.mv_refresh_log`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	err = w.refreshOneMV(context.Background(), mvName, TriggeredByCron, "", "TUGURE")
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── HandleExportAsync — UpdateExportLogCompleted fails ──────────────────────

func TestHandleExportAsync_UpdateLogFails(t *testing.T) {
	// replica=nil → queryMVRows falls back to primary.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	payload, _ := json.Marshal(ExportWorkerPayload{
		ExportLogID: exportID.String(),
		ReportSlug:  "mv-status-periode",
		Format:      "csv",
		TenantID:    "TUGURE",
		ActorID:     uuid.New().String(),
	})

	// queryMVRows → ok (replica=nil so goes to primary db)
	mock.ExpectQuery(`SELECT \* FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"col"}).AddRow("v"))

	// UpdateExportLogCompleted fails
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnError(assert.AnError)

	// NewRepository with replica=nil → queryMVRows uses primary
	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	task := asynq.NewTask(TaskExportAsync, payload)
	err = w.HandleExportAsync(context.Background(), task)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── HandleExportAsync — UpdateExportLogFailed when build fails ───────────────

func TestHandleExportAsync_BuildExportFails(t *testing.T) {
	// replica=nil → queryMVRows falls back to primary.
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	payload, _ := json.Marshal(ExportWorkerPayload{
		ExportLogID: exportID.String(),
		ReportSlug:  "mv-akrual-summary", // valid slug
		Format:      "csv",
		TenantID:    "TUGURE",
		ActorID:     uuid.New().String(),
	})

	// queryMVRows → error (replica=nil so goes to primary)
	mock.ExpectQuery(`SELECT \* FROM rpt.mv_akrual_summary`).
		WillReturnError(assert.AnError)

	// UpdateExportLogFailed
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	task := asynq.NewTask(TaskExportAsync, payload)
	err = w.HandleExportAsync(context.Background(), task)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── ListExportLogs — cursor path ─────────────────────────────────────────────

func TestListExportLogs_WithCursorPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	cursor := now.UTC().Format(time.RFC3339Nano)

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at",
		"requested_by", "requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			uuid.New(), "mv-status-periode", "csv", "COMPLETED",
			nil, nil, nil, nil,
			uuid.New(), now, nil, nil,
		))

	repo := NewRepository(db, nil)
	items, nextCursor, hasMore, err := repo.ListExportLogs(context.Background(), cursor, 5, "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 1, len(items))
	assert.Nil(t, nextCursor)
	assert.False(t, hasMore)
}

// ─── ListExportLogs — invalid cursor (falls back to now) ─────────────────────

func TestListExportLogs_InvalidCursorFallback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at",
		"requested_by", "requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := NewRepository(db, nil)
	items, _, _, err := repo.ListExportLogs(context.Background(), "not-a-time", 10, "TUGURE")
	require.NoError(t, err)
	assert.Empty(t, items)
}

// ─── ListExportLogs — hasMore=true (limit+1 rows returned) ───────────────────

func TestListExportLogs_HasMoreTrue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at",
		"requested_by", "requested_at", "completed_at", "downloaded_at"}
	rows := sqlmock.NewRows(cols)
	// Limit=2 → fetch=3. Return 3 rows → hasMore=true.
	for i := 0; i < 3; i++ {
		rows.AddRow(uuid.New(), "mv-status-periode", "csv", "COMPLETED",
			nil, nil, nil, nil,
			uuid.New(), now.Add(time.Duration(-i)*time.Second), nil, nil)
	}
	mock.ExpectQuery(`FROM sys.export_log`).WillReturnRows(rows)

	repo := NewRepository(db, nil)
	items, nextCursor, hasMore, err := repo.ListExportLogs(context.Background(), "", 2, "TUGURE")
	require.NoError(t, err)
	assert.True(t, hasMore)
	assert.NotNil(t, nextCursor)
	assert.Len(t, items, 2)
}

// ─── ListExportLogs — limit clamping (>200) ──────────────────────────────────

func TestListExportLogs_LimitClamp(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "report_slug", "format", "status", "row_count",
			"sha256_hash", "file_minio_path", "expires_at", "requested_by", "requested_at", "completed_at", "downloaded_at"}))

	repo := NewRepository(db, nil)
	_, _, _, err = repo.ListExportLogs(context.Background(), "", 9999, "TUGURE")
	require.NoError(t, err) // should not fail; limit clamped to 200
}

// ─── RequestExport — BeginTx fails ───────────────────────────────────────────
// CountMVRows uses r.replica for COUNT(*). Pass db as replica.
// Permission bypass: use "audit_log.read" to avoid per-slug permission check.

func TestRequestExport_BeginTxFails(t *testing.T) {
	primaryDB, primaryMock, err := sqlmock.New()
	require.NoError(t, err)
	defer primaryDB.Close()

	replicaDB, replicaMock, err := sqlmock.New()
	require.NoError(t, err)
	defer replicaDB.Close()

	// CountMVRows → replica COUNT(*)
	replicaMock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	// BeginTx → primary
	primaryMock.ExpectBegin().WillReturnError(assert.AnError)

	repo := NewRepository(primaryDB, replicaDB)
	mvRepo := NewMVRepo(primaryDB, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:         uuid.New().String(),
		TenantID:    "TUGURE",
		Permissions: []string{"audit_log.read"}, // bypass per-slug check
	})

	_, _, err = svc.RequestExport(ctx, ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
	assert.NoError(t, primaryMock.ExpectationsWereMet())
	assert.NoError(t, replicaMock.ExpectationsWereMet())
}

// ─── RequestExport — InsertExportLog fails ────────────────────────────────────

func TestRequestExport_InsertLogFails(t *testing.T) {
	primaryDB, primaryMock, err := sqlmock.New()
	require.NoError(t, err)
	defer primaryDB.Close()

	replicaDB, replicaMock, err := sqlmock.New()
	require.NoError(t, err)
	defer replicaDB.Close()

	// CountMVRows → replica
	replicaMock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(1))

	// Begin on primary
	primaryMock.ExpectBegin()
	// InsertExportLog fails
	primaryMock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnError(assert.AnError)
	primaryMock.ExpectRollback()

	repo := NewRepository(primaryDB, replicaDB)
	mvRepo := NewMVRepo(primaryDB, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:         uuid.New().String(),
		TenantID:    "TUGURE",
		Permissions: []string{"audit_log.read"},
	})

	_, _, err = svc.RequestExport(ctx, ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
	assert.NoError(t, primaryMock.ExpectationsWereMet())
	assert.NoError(t, replicaMock.ExpectationsWereMet())
}

// ─── RequestExport — CountMVRows fails ───────────────────────────────────────

func TestRequestExport_CountMVRowsFails(t *testing.T) {
	primaryDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer primaryDB.Close()

	replicaDB, replicaMock, err := sqlmock.New()
	require.NoError(t, err)
	defer replicaDB.Close()

	replicaMock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnError(assert.AnError)

	repo := NewRepository(primaryDB, replicaDB)
	mvRepo := NewMVRepo(primaryDB, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:         uuid.New().String(),
		TenantID:    "TUGURE",
		Permissions: []string{"audit_log.read"},
	})

	_, _, err = svc.RequestExport(ctx, ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
	assert.NoError(t, replicaMock.ExpectationsWereMet())
}

// ─── GetExportDownload — UpdateExportLogDownloaded fails ─────────────────────
// GetExportLog uses r.primary; BeginTx also uses r.primary. Same mock, ordered sequence.

func TestGetExportDownload_UpdateDownloadedFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	now := time.Now()

	// GetExportLog uses primary (QueryRow)
	scanCols := []string{"id", "report_slug", "format", "status", "row_count",
		"file_minio_path", "sha256_hash", "signed_url",
		"requested_by", "requested_at", "completed_at",
		"expires_at", "downloaded_at", "job_id", "tenant_id"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(scanCols).AddRow(
			exportID, "mv-status-periode", "csv", "COMPLETED",
			nil, nil, nil, nil,
			uuid.New(), now, nil, nil, nil, nil, "TUGURE"))

	// BeginTx
	mock.ExpectBegin()
	// UpdateExportLogDownloaded fails
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	_, err = svc.GetExportDownload(ctx, exportID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetExportDownload — Commit fails ────────────────────────────────────────

func TestGetExportDownload_CommitFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	now := time.Now()

	scanCols := []string{"id", "report_slug", "format", "status", "row_count",
		"file_minio_path", "sha256_hash", "signed_url",
		"requested_by", "requested_at", "completed_at",
		"expires_at", "downloaded_at", "job_id", "tenant_id"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(scanCols).AddRow(
			exportID, "mv-status-periode", "csv", "COMPLETED",
			nil, nil, nil, nil,
			uuid.New(), now, nil, nil, nil, nil, "TUGURE"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	_, err = svc.GetExportDownload(ctx, exportID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── NewMVRefreshTask — happy path ───────────────────────────────────────────

func TestNewMVRefreshTask_HappyPath(t *testing.T) {
	task, err := NewMVRefreshTask("rpt.mv_status_periode", TriggeredByManual, uuid.New().String(), "TUGURE")
	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, TaskMVRefresh, task.Type())
}

// ─── NewScheduledEmailTask — happy path ──────────────────────────────────────

func TestNewScheduledEmailTask_HappyPath(t *testing.T) {
	task, err := NewScheduledEmailTask(uuid.New().String(), "TUGURE")
	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, TaskScheduledEmailSend, task.Type())
}

// ─── UpdateRefreshLog — success path ─────────────────────────────────────────

func TestMVRepo_UpdateRefreshLog_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logID := uuid.New()
	rowCount := int64(42)

	mock.ExpectExec(`UPDATE sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mvRepo := NewMVRepo(db, nil)
	err = mvRepo.UpdateRefreshLog(context.Background(), logID, "COMPLETED", &rowCount, nil, "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── UpdateRefreshLog — error path ───────────────────────────────────────────

func TestMVRepo_UpdateRefreshLog_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE sys.mv_refresh_log`).
		WillReturnError(assert.AnError)

	mvRepo := NewMVRepo(db, nil)
	err = mvRepo.UpdateRefreshLog(context.Background(), uuid.New(), "FAILED", nil, nil, "TUGURE")
	assert.Error(t, err)
}

// ─── InsertOptOut — success path ─────────────────────────────────────────────

func TestRepo_InsertOptOut_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectExec(`INSERT INTO sys.scheduled_email_optout`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepository(db, nil)
	err = repo.InsertOptOut(context.Background(), schedID, "user@tugu-re.com", "tokenhash", "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── InsertOptOut — error path ───────────────────────────────────────────────

func TestRepo_InsertOptOut_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`INSERT INTO sys.scheduled_email_optout`).
		WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	err = repo.InsertOptOut(context.Background(), uuid.New(), "x@y.com", "tokenhash", "TUGURE")
	assert.Error(t, err)
}

// ─── UpdateScheduledEmailLastSent ────────────────────────────────────────────

func TestRepo_UpdateScheduledEmailLastSent_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewRepository(db, nil)
	err = repo.UpdateScheduledEmailLastSent(context.Background(), schedID, "SENT", "TUGURE")
	require.NoError(t, err)
}

// ─── UpdateScheduledEmailLastSent — error ────────────────────────────────────

func TestRepo_UpdateScheduledEmailLastSent_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	err = repo.UpdateScheduledEmailLastSent(context.Background(), uuid.New(), "FAILED", "TUGURE")
	assert.Error(t, err)
}

// ─── InsertExportLog — success path ──────────────────────────────────────────

func TestRepo_InsertExportLog_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	err = repo.InsertExportLog(context.Background(), tx, ExportLogRow{
		ID:          uuid.New(),
		ReportSlug:  "mv-status-periode",
		Format:      FormatCSV,
		Status:      ExportStatusRequested,
		RequestedBy: uuid.New(),
		RequestedAt: time.Now(),
		TenantID:    "TUGURE",
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// ─── InsertExportLog — error path ────────────────────────────────────────────

func TestRepo_InsertExportLog_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	err = repo.InsertExportLog(context.Background(), tx, ExportLogRow{
		ID:          uuid.New(),
		ReportSlug:  "mv-status-periode",
		Format:      FormatCSV,
		Status:      ExportStatusRequested,
		RequestedBy: uuid.New(),
		RequestedAt: time.Now(),
		TenantID:    "TUGURE",
	})
	assert.Error(t, err)
	_ = tx.Rollback()
}

// ─── UpdateExportLogCompleted — error path ────────────────────────────────────

func TestRepo_UpdateExportLogCompleted_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	err = repo.UpdateExportLogCompleted(context.Background(), uuid.New(),
		100, "path/to/file.csv", "sha256hex", "https://url", time.Now(), "TUGURE")
	assert.Error(t, err)
}

// ─── UpdateExportLogFailed — error path ───────────────────────────────────────

func TestRepo_UpdateExportLogFailed_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	err = repo.UpdateExportLogFailed(context.Background(), uuid.New(), "error msg", "TUGURE")
	assert.Error(t, err)
}

// ─── UpdateExportLogDownloaded — success path ────────────────────────────────

func TestRepo_UpdateExportLogDownloaded_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	err = repo.UpdateExportLogDownloaded(context.Background(), tx, uuid.New(), "TUGURE")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// ─── SoftDeleteScheduledEmail — error path ───────────────────────────────────

func TestRepo_SoftDeleteScheduledEmail_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	err = repo.SoftDeleteScheduledEmail(context.Background(), tx, uuid.New(), uuid.New(), "TUGURE")
	assert.Error(t, err)
	_ = tx.Rollback()
}

// ─── SoftDeleteScheduledEmail — success ──────────────────────────────────────

func TestRepo_SoftDeleteScheduledEmail_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	err = repo.SoftDeleteScheduledEmail(context.Background(), tx, uuid.New(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// ─── ListActiveScheduledEmails — with rows ────────────────────────────────────

func TestRepo_ListActiveScheduledEmails_WithRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now()
	schedID := uuid.New()
	createdBy := uuid.New()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			schedID, "mv-status-periode", "csv", "daily", "08:00",
			[]byte(`["x@y.com"]`), true,
			nil, nil, nil, nil, now, createdBy, "TUGURE"))

	repo := NewRepository(db, nil)
	items, err := repo.ListActiveScheduledEmails(context.Background(), "TUGURE")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, schedID, items[0].ID)
}

// ─── SoftDeleteScheduledEmail service path ───────────────────────────────────
// Service calls BeginTx → SoftDeleteScheduledEmail(tx) → Commit (no GetScheduledEmail).

func TestService_SoftDeleteScheduledEmail_DeleteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	repo := NewRepository(db, db)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	err = svc.SoftDeleteScheduledEmail(ctx, schedID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── SoftDeleteScheduledEmail service — BeginTx fails ────────────────────────

func TestService_SoftDeleteScheduledEmail_BeginTxFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin().WillReturnError(assert.AnError)

	repo := NewRepository(db, db)
	mvRepo := NewMVRepo(db, nil)
	svc := NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	err = svc.SoftDeleteScheduledEmail(ctx, uuid.New())
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── IsRefreshRunning — not running path ─────────────────────────────────────

func TestMVRepo_IsRefreshRunning_NotRunning(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// No rows → not running
	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WithArgs("rpt.mv_status_periode", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "mv_name", "triggered_by", "trigger_actor", "status", "started_at", "tenant_id"}))

	mvRepo := NewMVRepo(db, nil)
	running, logRow, err := mvRepo.IsRefreshRunning(context.Background(), "rpt.mv_status_periode", "TUGURE")
	require.NoError(t, err)
	assert.False(t, running)
	assert.Nil(t, logRow)
}

// ─── InsertScheduledEmail — error path ───────────────────────────────────────

func TestRepo_InsertScheduledEmail_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.scheduled_email`).
		WillReturnError(assert.AnError)
	mock.ExpectRollback()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := NewRepository(db, nil)
	row := ScheduledEmailRow{
		ID:         uuid.New(),
		ReportSlug: "mv-status-periode",
		Format:     FormatCSV,
		Frequency:  FreqDaily,
		SendTime:   "08:00",
		Active:     true,
		CreatedBy:  uuid.New(),
		TenantID:   "TUGURE",
	}
	err = repo.InsertScheduledEmail(context.Background(), tx, row, []string{"x@y.com"})
	assert.Error(t, err)
	_ = tx.Rollback()
}

// ─── ContentTypeFor — all formats ────────────────────────────────────────────

func TestContentTypeFor_AllFormats(t *testing.T) {
	assert.Equal(t, "text/csv; charset=UTF-8", ContentTypeFor(FormatCSV))
	assert.Contains(t, ContentTypeFor(FormatXLSX), "spreadsheetml")
	assert.Equal(t, "application/pdf", ContentTypeFor(FormatPDF))
	assert.Equal(t, "application/octet-stream", ContentTypeFor(ExportFormat("unknown")))
}
