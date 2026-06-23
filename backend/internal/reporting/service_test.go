package reporting_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// ─── ExportFormat.IsValid ─────────────────────────────────────────────────────

func TestExportFormat_IsValid(t *testing.T) {
	assert.True(t, reporting.FormatCSV.IsValid())
	assert.True(t, reporting.FormatXLSX.IsValid())
	assert.True(t, reporting.FormatPDF.IsValid())
	assert.False(t, reporting.ExportFormat("doc").IsValid())
	assert.False(t, reporting.ExportFormat("").IsValid())
}

// ─── GenerateOptOutToken / verifyOptOutToken (white-box via exported helpers) ─

// OptOutToken is tested via the public service method.
// We stub a minimal service just for token generation.

func TestOptOutToken_RoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	_ = mock

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("test-secret-32bytesXXXXXXXXXXXX"))

	schedID := uuid.New()
	email := "user@example.com"
	token := svc.GenerateOptOutToken(schedID, email, 1*time.Hour)
	assert.NotEmpty(t, token)

	// Verify succeeds.
	err = svc.VerifyOptOutToken(schedID, email, token)
	assert.NoError(t, err)
}

func TestOptOutToken_WrongEmail(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("test-secret-32bytesXXXXXXXXXXXX"))

	schedID := uuid.New()
	token := svc.GenerateOptOutToken(schedID, "real@example.com", 1*time.Hour)
	err = svc.VerifyOptOutToken(schedID, "other@example.com", token)
	assert.Error(t, err, "wrong email should fail verification")
}

func TestOptOutToken_Expired(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("test-secret-32bytesXXXXXXXXXXXX"))

	schedID := uuid.New()
	token := svc.GenerateOptOutToken(schedID, "u@x.com", -1*time.Second) // already expired
	err = svc.VerifyOptOutToken(schedID, "u@x.com", token)
	assert.Error(t, err, "expired token should fail")
}

func TestOptOutToken_TamperedToken(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("test-secret-32bytesXXXXXXXXXXXX"))

	schedID := uuid.New()
	token := svc.GenerateOptOutToken(schedID, "u@x.com", time.Hour)
	// Tamper: flip last char.
	tampered := token[:len(token)-1] + "X"
	err = svc.VerifyOptOutToken(schedID, "u@x.com", tampered)
	assert.Error(t, err)
}

// ─── ExportObjectName ─────────────────────────────────────────────────────────

func TestExportObjectName_Format(t *testing.T) {
	ts := time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC)
	name := reporting.ExportObjectName("TUGURE", "user-id", "job-001", "csv", ts)
	assert.Equal(t, "TUGURE/user-id/2026/06/23/job-001.csv", name)
}

// ─── ContentTypeFor ───────────────────────────────────────────────────────────

func TestContentTypeFor(t *testing.T) {
	assert.Equal(t, "text/csv; charset=UTF-8", reporting.ContentTypeFor(reporting.FormatCSV))
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", reporting.ContentTypeFor(reporting.FormatXLSX))
	assert.Equal(t, "application/pdf", reporting.ContentTypeFor(reporting.FormatPDF))
	assert.Equal(t, "application/octet-stream", reporting.ContentTypeFor(reporting.ExportFormat("unknown")))
}

// ─── NewMVRefreshTask ─────────────────────────────────────────────────────────

func TestNewMVRefreshTask_ValidPayload(t *testing.T) {
	task, err := reporting.NewMVRefreshTask("rpt.mv_status_periode", reporting.TriggeredByManual, uuid.New().String(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, reporting.TaskMVRefresh, task.Type())

	var p reporting.MVRefreshPayload
	err = json.Unmarshal(task.Payload(), &p)
	require.NoError(t, err)
	assert.Equal(t, "rpt.mv_status_periode", p.MVName)
	assert.Equal(t, "MANUAL", p.TriggeredBy)
	assert.Equal(t, "TUGURE", p.TenantID)
}

func TestNewMVRefreshTask_AllMVs(t *testing.T) {
	// mvName="" means all
	task, err := reporting.NewMVRefreshTask("", reporting.TriggeredByCron, "", "TUGURE")
	require.NoError(t, err)
	var p reporting.MVRefreshPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "", p.MVName)
}

// ─── NewScheduledEmailTask ────────────────────────────────────────────────────

func TestNewScheduledEmailTask_ValidPayload(t *testing.T) {
	schedID := uuid.New().String()
	task, err := reporting.NewScheduledEmailTask(schedID, "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, reporting.TaskScheduledEmailSend, task.Type())

	var p reporting.ScheduledEmailPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, schedID, p.ScheduledEmailID)
	assert.Equal(t, "TUGURE", p.TenantID)
}

// ─── Repo: InsertExportLog / ListExportLogs ───────────────────────────────────

func TestRepo_InsertExportLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := reporting.NewRepository(db, nil)
	row := reporting.ExportLogRow{
		ID:          uuid.New(),
		ReportSlug:  "mv-status-periode",
		Format:      reporting.FormatCSV,
		Status:      reporting.ExportStatusRequested,
		RequestedBy: uuid.New(),
		RequestedAt: time.Now(),
		TenantID:    "TUGURE",
	}
	err = repo.InsertExportLog(context.Background(), tx, row)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_ListExportLogs_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	items, _, hasMore, err := repo.ListExportLogs(context.Background(), "", 50, "TUGURE")
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.False(t, hasMore)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo: InsertOptOut / GetOptOuts ─────────────────────────────────────────

func TestRepo_InsertOptOut_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectExec(`INSERT INTO sys.scheduled_email_optout`).
		WithArgs(schedID, "optout@example.com", "hash-abc", "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	err = repo.InsertOptOut(context.Background(), schedID, "optout@example.com", "hash-abc", "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepo_GetOptOuts_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectQuery(`FROM sys.scheduled_email_optout`).
		WithArgs(schedID).
		WillReturnRows(sqlmock.NewRows([]string{"email"}))

	repo := reporting.NewRepository(db, nil)
	emails, err := repo.GetOptOuts(context.Background(), schedID)
	require.NoError(t, err)
	assert.Empty(t, emails)
}

// ─── ValidReportSlugs — slug validation ───────────────────────────────────────

func TestValidReportSlugs_Count(t *testing.T) {
	assert.Len(t, reporting.ValidReportSlugs, 8, "must have exactly 8 report slugs")
}

// ─── Task constants ───────────────────────────────────────────────────────────

func TestTaskConstants(t *testing.T) {
	assert.Equal(t, "reporting:mv-refresh", reporting.TaskMVRefresh)
	assert.Equal(t, "reporting:export-async", reporting.TaskExportAsync)
	assert.Equal(t, "reporting:scheduled-email-send", reporting.TaskScheduledEmailSend)
}

// ─── Service.NewService — constructor smoke ───────────────────────────────────

func TestNewService_Smoke(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))
	assert.NotNil(t, svc)
}

// ─── SMTP RenderEmailTemplate ─────────────────────────────────────────────────

func TestRenderEmailTemplate_ReplacesPlaceholders(t *testing.T) {
	subj := "[BLIPS] Laporan {report_slug} — {tanggal}"
	body := "Terlampir laporan {report_slug} per {tanggal}. Hash: {file_hash}"
	data := map[string]string{
		"report_slug": "mv-status-periode",
		"tanggal":     "2026-06-23",
		"file_hash":   "abc123",
	}
	gotSubj, gotBody, err := reporting.RenderEmailTemplate(subj, body, data)
	require.NoError(t, err)
	assert.Contains(t, gotSubj, "mv-status-periode")
	assert.Contains(t, gotSubj, "2026-06-23")
	assert.Contains(t, gotBody, "abc123")
	assert.NotContains(t, gotBody, "{file_hash}")
}

func TestRenderEmailTemplate_DefaultTemplates(t *testing.T) {
	data := map[string]string{
		"report_slug": "test",
		"tanggal":     "2026-06-23",
		"file_hash":   "deadbeef",
		"opt_out_link": "https://example.com/optout",
	}
	subj, body, err := reporting.RenderEmailTemplate(
		reporting.DefaultSubjectTemplate, reporting.DefaultBodyTemplate, data)
	require.NoError(t, err)
	assert.Contains(t, subj, "test")
	assert.Contains(t, body, "BLIPS Tugu Re")
	assert.Contains(t, body, "deadbeef")
}

// ─── dbOrTx via repo helper ───────────────────────────────────────────────────

func TestRepo_InsertScheduledEmail_InTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys.scheduled_email`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := reporting.NewRepository(db, nil)
	actorID := uuid.New()
	row := reporting.ScheduledEmailRow{
		ID:         uuid.New(),
		ReportSlug: "mv-akrual-summary",
		Format:     reporting.FormatXLSX,
		Frequency:  reporting.FreqMonthly,
		SendTime:   "08:00",
		Active:     true,
		CreatedBy:  actorID,
		TenantID:   "TUGURE",
	}
	recipients := []string{"finance@tugu-re.com"}
	err = repo.InsertScheduledEmail(context.Background(), tx, row, recipients)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// helper so tests compile even without Asynq server
func TestNewScheduledEmailTask_MaxRetry(t *testing.T) {
	task, err := reporting.NewScheduledEmailTask(uuid.New().String(), "TUGURE")
	require.NoError(t, err)
	// Just confirm task is created without error; MaxRetry is internal option.
	assert.NotNil(t, task)
}

// ─── SMTP buildMIMEMessage (internal) tested via SMTPClient.SendEmail nil guard ─

func TestSMTPDefaultTemplates_NoPlaceholdersLeft(t *testing.T) {
	// After rendering with all keys present, no {key} should remain.
	data := map[string]string{
		"report_slug":  "mv-status",
		"tanggal":      "2026-06-23",
		"file_hash":    "abc",
		"opt_out_link": "https://example.com",
	}
	_, body, err := reporting.RenderEmailTemplate(reporting.DefaultSubjectTemplate, reporting.DefaultBodyTemplate, data)
	require.NoError(t, err)
	assert.NotContains(t, body, "{report_slug}")
	assert.NotContains(t, body, "{tanggal}")
	assert.NotContains(t, body, "{file_hash}")
	assert.NotContains(t, body, "{opt_out_link}")
}

// ─── ScheduledEmailFrequency ──────────────────────────────────────────────────

func TestScheduledEmailFrequency_Values(t *testing.T) {
	assert.Equal(t, reporting.ScheduledEmailFrequency("daily"), reporting.FreqDaily)
	assert.Equal(t, reporting.ScheduledEmailFrequency("weekly"), reporting.FreqWeekly)
	assert.Equal(t, reporting.ScheduledEmailFrequency("monthly"), reporting.FreqMonthly)
}

// ─── Repo UpdateExportLogFailed ───────────────────────────────────────────────

func TestRepo_UpdateExportLogFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	mock.ExpectExec(`UPDATE sys.export_log`).
		WithArgs(id, "some error", "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	err = repo.UpdateExportLogFailed(context.Background(), id, "some error", "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo UpdateScheduledEmailLastSent ───────────────────────────────────────

func TestRepo_UpdateScheduledEmailLastSent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WithArgs(id, "SENT", "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	err = repo.UpdateScheduledEmailLastSent(context.Background(), id, "SENT", "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ensure unused import is used
var _ = sql.ErrNoRows
