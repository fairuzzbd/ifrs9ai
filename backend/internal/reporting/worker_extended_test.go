package reporting_test

// worker_extended_test.go — additional Asynq worker handler coverage.
// Targets: HandleMVRefresh (all 8 MVs path with sqlmock, single MV path),
// HandleScheduledEmailSend (not-found, inactive, opt-out filtered),
// RegisterHandlers smoke, NewWorker nil-logger path.

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

	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// ─── NewWorker with nil logger ────────────────────────────────────────────────

func TestNewWorker_NilLogger(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)
	assert.NotNil(t, w)
}

// ─── RegisterHandlers smoke ───────────────────────────────────────────────────

func TestWorker_RegisterHandlers_Smoke(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	mux := asynq.NewServeMux()
	// Should not panic.
	w.RegisterHandlers(mux)
}

// ─── HandleMVRefresh — valid payload + sqlmock for begin/insert/commit ────────

func TestWorker_HandleMVRefresh_SingleMV_BeginFail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// BeginTx fails → should error
	mock.ExpectBegin().WillReturnError(assert.AnError)

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.MVRefreshPayload{
		MVName:      "rpt.mv_status_periode",
		TriggeredBy: "MANUAL",
		TenantID:    "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskMVRefresh, payload)
	err = w.HandleMVRefresh(context.Background(), task)
	assert.Error(t, err)
}

func TestWorker_HandleMVRefresh_AllMVs_BeginFail(t *testing.T) {
	// All 8 MVs, BeginTx fails immediately for each → continue, return nil
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	for range reporting.AllMVNames {
		mock.ExpectBegin().WillReturnError(assert.AnError)
	}

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	// "" MVName → all 8
	payload, _ := json.Marshal(reporting.MVRefreshPayload{
		MVName:      "",
		TriggeredBy: "CRON",
		TenantID:    "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskMVRefresh, payload)
	// all-MV mode: partial errors logged, returns nil
	err = w.HandleMVRefresh(context.Background(), task)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorker_HandleMVRefresh_SingleMV_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_jurnal_summary"

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// RefreshConcurrent: advisory lock acquired
	mock.ExpectQuery(`SELECT pg_try_advisory_lock`).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`REFRESH MATERIALIZED VIEW CONCURRENTLY`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(42)))
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// UpdateRefreshLog success
	mock.ExpectExec(`UPDATE sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.MVRefreshPayload{
		MVName:      mvName,
		TriggeredBy: "MANUAL",
		TenantID:    "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskMVRefresh, payload)
	err = w.HandleMVRefresh(context.Background(), task)
	require.NoError(t, err)
}

// ─── HandleScheduledEmailSend — sched not found ───────────────────────────────

func TestWorker_HandleScheduledEmailSend_SchedNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	// returns no rows → se == nil → SkipRetry error
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ScheduledEmailPayload{
		ScheduledEmailID: schedID.String(),
		TenantID:         "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskScheduledEmailSend, payload)
	err = w.HandleScheduledEmailSend(context.Background(), task)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── HandleScheduledEmailSend — inactive sched ────────────────────────────────

func TestWorker_HandleScheduledEmailSend_Inactive(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	createdBy := uuid.New()
	now := time.Now()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			schedID, "mv-status-periode", "csv", "daily", "08:00",
			[]byte(`["x@y.com"]`), false, // active=false
			nil, nil, nil, nil, now, createdBy, "TUGURE"))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ScheduledEmailPayload{
		ScheduledEmailID: schedID.String(),
		TenantID:         "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskScheduledEmailSend, payload)
	err = w.HandleScheduledEmailSend(context.Background(), task)
	// inactive → returns nil (silent skip)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── HandleScheduledEmailSend — all recipients opted out ─────────────────────

func TestWorker_HandleScheduledEmailSend_AllOptedOut(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	createdBy := uuid.New()
	now := time.Now()

	emailRecipient := "x@y.com"

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			schedID, "mv-status-periode", "csv", "daily", "08:00",
			[]byte(`["`+emailRecipient+`"]`), true,
			nil, nil, nil, nil, now, createdBy, "TUGURE"))

	// GetOptOuts → returns the one recipient
	mock.ExpectQuery(`FROM sys.scheduled_email_optout`).
		WithArgs(schedID).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(emailRecipient))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ScheduledEmailPayload{
		ScheduledEmailID: schedID.String(),
		TenantID:         "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskScheduledEmailSend, payload)
	err = w.HandleScheduledEmailSend(context.Background(), task)
	// all opted out → log + return nil
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
