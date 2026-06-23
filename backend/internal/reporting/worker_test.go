package reporting_test

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

// testExportedAt is shared across reporting_test package tests.
var workerTestExportedAt = time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC)

// ─── Worker task constants ────────────────────────────────────────────────────

func TestWorker_TaskConstants_Unique(t *testing.T) {
	types := []string{
		reporting.TaskMVRefresh,
		reporting.TaskExportAsync,
		reporting.TaskScheduledEmailSend,
	}
	seen := make(map[string]bool)
	for _, typ := range types {
		assert.False(t, seen[typ], "duplicate task type: %s", typ)
		seen[typ] = true
	}
}

// ─── NewWorker ────────────────────────────────────────────────────────────────

func TestNewWorker_Smoke(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("worker-secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)
	assert.NotNil(t, w)
}

// ─── HandleMVRefresh — invalid payload ───────────────────────────────────────

func TestWorker_HandleMVRefresh_InvalidJSON(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	task := asynq.NewTask(reporting.TaskMVRefresh, []byte("not-valid-json"))
	err = w.HandleMVRefresh(context.Background(), task)
	assert.Error(t, err)
}

func TestWorker_HandleMVRefresh_UnknownMVName(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.MVRefreshPayload{
		MVName:      "rpt.mv_fake",
		TriggeredBy: "MANUAL",
		TenantID:    "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskMVRefresh, payload)
	err = w.HandleMVRefresh(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mv_name")
}

// ─── HandleExportAsync — invalid payload ─────────────────────────────────────

func TestWorker_HandleExportAsync_InvalidJSON(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	task := asynq.NewTask(reporting.TaskExportAsync, []byte("{bad json"))
	err = w.HandleExportAsync(context.Background(), task)
	assert.Error(t, err)
}

func TestWorker_HandleExportAsync_InvalidExportLogID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ExportWorkerPayload{
		ExportLogID: "not-a-uuid",
		ReportSlug:  "mv-status-periode",
		Format:      "csv",
		TenantID:    "TUGURE",
		ActorID:     uuid.New().String(),
	})
	task := asynq.NewTask(reporting.TaskExportAsync, payload)
	err = w.HandleExportAsync(context.Background(), task)
	assert.Error(t, err)
}

func TestWorker_HandleExportAsync_UnsupportedFormat(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ExportWorkerPayload{
		ExportLogID: uuid.New().String(),
		ReportSlug:  "mv-status-periode",
		Format:      "odf", // unsupported
		TenantID:    "TUGURE",
		ActorID:     uuid.New().String(),
	})
	task := asynq.NewTask(reporting.TaskExportAsync, payload)
	err = w.HandleExportAsync(context.Background(), task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

// ─── HandleScheduledEmailSend — invalid payload ───────────────────────────────

func TestWorker_HandleScheduledEmailSend_InvalidJSON(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	task := asynq.NewTask(reporting.TaskScheduledEmailSend, []byte("bad"))
	err = w.HandleScheduledEmailSend(context.Background(), task)
	assert.Error(t, err)
}

func TestWorker_HandleScheduledEmailSend_InvalidScheduledEmailID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	w := reporting.NewWorker(svc, repo, mvRepo, nil, nil, nil, nil)

	payload, _ := json.Marshal(reporting.ScheduledEmailPayload{
		ScheduledEmailID: "not-a-uuid",
		TenantID:         "TUGURE",
	})
	task := asynq.NewTask(reporting.TaskScheduledEmailSend, payload)
	err = w.HandleScheduledEmailSend(context.Background(), task)
	assert.Error(t, err)
}

// ─── buildExportBuffer ────────────────────────────────────────────────────────

func TestBuildExportBuffer_CSV(t *testing.T) {
	fb, sha, err := reporting.BuildExportBuffer("mv-status", reporting.FormatCSV,
		[][]string{{"a", "b"}, {"1", "2"}},
		[]string{"col1", "col2"},
		workerTestExportedAt, "u")
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, fb[:3], "CSV must start with BOM")
}

func TestBuildExportBuffer_XLSX(t *testing.T) {
	fb, sha, err := reporting.BuildExportBuffer("mv-status", reporting.FormatXLSX,
		[][]string{{"r1c1"}},
		[]string{"header"},
		workerTestExportedAt, "u")
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
}

func TestBuildExportBuffer_PDF(t *testing.T) {
	fb, sha, err := reporting.BuildExportBuffer("mv-status", reporting.FormatPDF,
		[][]string{{"x"}},
		[]string{"h"},
		workerTestExportedAt, "u")
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
}

func TestBuildExportBuffer_UnknownFormat(t *testing.T) {
	_, _, err := reporting.BuildExportBuffer("mv-status", reporting.ExportFormat("odf"),
		nil, nil, workerTestExportedAt, "u")
	assert.Error(t, err)
}

