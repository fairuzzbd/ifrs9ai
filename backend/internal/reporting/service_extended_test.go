package reporting_test

// service_extended_test.go — covers Service methods with stub repo (sqlmock) + nil infra.
// Targets: RequestExport (inline vs async paths), CreateScheduledEmail (happy + validation),
// SoftDeleteScheduledEmail, OptOutRecipient, ListExportLogs, GetExportDownload,
// BuildInlineExport, ListMVStatus, checkExportPermission, nilableStr, isValidFrequency.

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// claimsCtx returns a context carrying auth.Claims with given permissions.
func claimsCtx(permissions ...string) context.Context {
	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-AKUN"},
		TenantID:    "TUGURE",
		Permissions: permissions,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

// ─── BuildInlineExport ───────────────────────────────────────────────────────

func TestService_BuildInlineExport_CSVEmptyRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	_ = mock

	// queryMVRows will try to SELECT * FROM rpt.mv_status_periode LIMIT ...
	// We use sqlmock to return a no-column, no-row result.
	mock.ExpectQuery(`SELECT \* FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("val1"))

	repo := reporting.NewRepository(db, db) // use same db for replica
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	_, _, ct, err := svc.BuildInlineExport(ctx, "mv-status-periode", reporting.FormatCSV, "testuser")
	require.NoError(t, err)
	assert.Contains(t, ct, "text/csv")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestService_BuildInlineExport_XLSX(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT \* FROM rpt.mv_jurnal_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"col_a"}).AddRow("row1"))

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	fb, sha, ct, err := svc.BuildInlineExport(ctx, "mv-jurnal-summary", reporting.FormatXLSX, "testuser")
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
	assert.Contains(t, ct, "spreadsheetml")
}

func TestService_BuildInlineExport_PDF(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT \* FROM rpt.mv_akrual_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow("y"))

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	fb, sha, ct, err := svc.BuildInlineExport(ctx, "mv-akrual-summary", reporting.FormatPDF, "u")
	require.NoError(t, err)
	assert.NotEmpty(t, fb)
	assert.NotEmpty(t, sha)
	assert.Equal(t, "application/pdf", ct)
}

func TestService_BuildInlineExport_InvalidSlug(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	_, _, _, err = svc.BuildInlineExport(ctx, "mv-not-exist", reporting.FormatCSV, "u")
	assert.Error(t, err)
}

// ─── checkExportPermission via RequestExport ─────────────────────────────────

func TestService_RequestExport_NoClaimsReturnsUnauthorized(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	// context WITHOUT claims
	_, _, err = svc.RequestExport(context.Background(), reporting.ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
}

func TestService_RequestExport_ForbiddenPermission(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("instrumen.read") // no export permission
	_, _, err = svc.RequestExport(ctx, reporting.ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
}

func TestService_RequestExport_InvalidFormat(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	_, _, err = svc.RequestExport(ctx, reporting.ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     reporting.ExportFormat("odf"),
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
}

func TestService_RequestExport_InvalidSlug(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	_, _, err = svc.RequestExport(ctx, reporting.ExportRequest{
		ReportSlug: "mv-unknown-slug",
		Format:     reporting.FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
}

func TestService_RequestExport_InlinePath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// CountMVRows: Sscanf("%4s.%s", "rpt.mv_status_periode") reads "rpt." but fails
	// on the "." separator because it consumes it as part of %4s → err != nil →
	// fallback COUNT(*) on replica (db here since primary==replica).
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(100)))

	// BeginTx + InsertExportLog + Commit
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	actorID := uuid.New()
	ctx := claimsCtx("audit_log.read")
	jobRef, logRow, err := svc.RequestExport(ctx, reporting.ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		ActorID:    actorID,
		TenantID:   "TUGURE",
	})
	require.NoError(t, err)
	assert.Nil(t, jobRef, "inline path: no job ref")
	require.NotNil(t, logRow)
	assert.Equal(t, reporting.ExportStatusRequested, logRow.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestService_RequestExport_TooLarge(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// CountMVRows fallback COUNT(*) (see comment in InlinePath test)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(200_000)))

	repo := reporting.NewRepository(db, db)
	mvRepo := reporting.NewMVRepo(db, db)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("audit_log.read")
	_, _, err = svc.RequestExport(ctx, reporting.ExportRequest{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		ActorID:    uuid.New(),
		TenantID:   "TUGURE",
	})
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── CreateScheduledEmail ────────────────────────────────────────────────────

func TestService_CreateScheduledEmail_InvalidSlug(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	_, err = svc.CreateScheduledEmail(ctx, reporting.ScheduledEmailCreateReq{
		ReportSlug: "mv-not-found",
		Format:     reporting.FormatCSV,
		Frequency:  reporting.FreqDaily,
		SendTime:   "07:00",
		Recipients: []string{"x@y.com"},
		Active:     true,
	})
	assert.Error(t, err)
}

func TestService_CreateScheduledEmail_InvalidFormat(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	_, err = svc.CreateScheduledEmail(ctx, reporting.ScheduledEmailCreateReq{
		ReportSlug: "mv-status-periode",
		Format:     reporting.ExportFormat("odf"),
		Frequency:  reporting.FreqDaily,
		SendTime:   "07:00",
		Recipients: []string{"x@y.com"},
	})
	assert.Error(t, err)
}

func TestService_CreateScheduledEmail_InvalidFrequency(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	_, err = svc.CreateScheduledEmail(ctx, reporting.ScheduledEmailCreateReq{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		Frequency:  reporting.ScheduledEmailFrequency("hourly"),
		SendTime:   "07:00",
		Recipients: []string{"x@y.com"},
	})
	assert.Error(t, err)
}

func TestService_CreateScheduledEmail_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	item, err := svc.CreateScheduledEmail(ctx, reporting.ScheduledEmailCreateReq{
		ReportSlug: "mv-status-periode",
		Format:     reporting.FormatCSV,
		Frequency:  reporting.FreqWeekly,
		SendTime:   "09:00",
		Recipients: []string{"finance@tugu-re.com"},
		Active:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "mv-status-periode", item.ReportSlug)
	assert.True(t, item.Active)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── SoftDeleteScheduledEmail ────────────────────────────────────────────────

func TestService_SoftDeleteScheduledEmail_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	err = svc.SoftDeleteScheduledEmail(ctx, schedID)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── ListExportLogs (via service wrapper) ────────────────────────────────────

func TestService_ListExportLogs_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx()
	items, cursor, hasMore, err := svc.ListExportLogs(ctx, "", 50)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Nil(t, cursor)
	assert.False(t, hasMore)
}

// ─── OptOutRecipient ─────────────────────────────────────────────────────────

func TestService_OptOutRecipient_InvalidToken(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	err = svc.OptOutRecipient(context.Background(), reporting.OptOutRequest{
		ScheduledEmailID: uuid.New(),
		Email:            "x@example.com",
		Token:            "invalid.token",
	})
	assert.Error(t, err)
}

func TestService_OptOutRecipient_ValidToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	email := "optout@example.com"

	mock.ExpectExec(`INSERT INTO sys.scheduled_email_optout`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("test-secret-32bytesXXXXXXXXXXXX"))

	token := svc.GenerateOptOutToken(schedID, email, time.Hour)

	err = svc.OptOutRecipient(context.Background(), reporting.OptOutRequest{
		ScheduledEmailID: schedID,
		Email:            email,
		Token:            token,
	})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetExportDownload ───────────────────────────────────────────────────────

func TestService_GetExportDownload_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// GetExportLog returns no row.
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "report_slug", "format", "status",
			"row_count", "file_minio_path", "sha256_hash", "signed_url",
			"requested_by", "requested_at", "completed_at", "expires_at", "downloaded_at",
			"job_id", "tenant_id"}))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.export.read")
	_, err = svc.GetExportDownload(ctx, uuid.New())
	assert.Error(t, err)
}

func TestService_GetExportDownload_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	requestedBy := uuid.New()
	now := time.Now()
	minioPath := "TUGURE/user/2026/06/23/job.csv"
	signedURL := "https://minio/signed"

	cols := []string{"id", "report_slug", "format", "status",
		"row_count", "file_minio_path", "sha256_hash", "signed_url",
		"requested_by", "requested_at", "completed_at", "expires_at", "downloaded_at",
		"job_id", "tenant_id"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			exportID, "mv-status-periode", "csv", "COMPLETED",
			nil, minioPath, nil, signedURL,
			requestedBy, now, nil, nil, nil,
			nil, "TUGURE"))

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.export.read")
	row, err := svc.GetExportDownload(ctx, exportID)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, exportID, row.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── TriggerRefresh — locked path ────────────────────────────────────────────

func TestService_TriggerRefresh_InvalidMVName(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	badName := "rpt.mv_unknown"
	ctx := claimsCtx("report.admin")
	_, err = svc.TriggerRefresh(ctx, &badName)
	assert.Error(t, err)
}

func TestService_TriggerRefresh_LockedRefresh(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_status_periode"
	id := uuid.New()
	cols := []string{"id", "mv_name", "triggered_by", "trigger_actor", "status", "started_at", "tenant_id"}
	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WithArgs(mvName, "TUGURE").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			id, mvName, "MANUAL", nil, "RUNNING", time.Now(), "TUGURE"))

	repo := reporting.NewRepository(db, nil)
	mvRepo := reporting.NewMVRepo(db, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("secret"))

	ctx := claimsCtx("report.admin")
	_, err = svc.TriggerRefresh(ctx, &mvName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Refresh")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo: GetExportLog ───────────────────────────────────────────────────────

func TestRepo_GetExportLog_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status",
		"row_count", "file_minio_path", "sha256_hash", "signed_url",
		"requested_by", "requested_at", "completed_at", "expires_at", "downloaded_at",
		"job_id", "tenant_id"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	row, err := repo.GetExportLog(context.Background(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestRepo_GetExportLog_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	exportID := uuid.New()
	requestedBy := uuid.New()
	now := time.Now()

	cols := []string{"id", "report_slug", "format", "status",
		"row_count", "file_minio_path", "sha256_hash", "signed_url",
		"requested_by", "requested_at", "completed_at", "expires_at", "downloaded_at",
		"job_id", "tenant_id"}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			exportID, "mv-status-periode", "csv", "REQUESTED",
			nil, nil, nil, nil,
			requestedBy, now, nil, nil, nil,
			nil, "TUGURE"))

	repo := reporting.NewRepository(db, nil)
	row, err := repo.GetExportLog(context.Background(), exportID, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, exportID, row.ID)
}

// ─── Repo: SoftDeleteScheduledEmail ──────────────────────────────────────────

func TestRepo_SoftDeleteScheduledEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.scheduled_email`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := reporting.NewRepository(db, nil)
	err = repo.SoftDeleteScheduledEmail(context.Background(), tx, uuid.New(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo: GetScheduledEmail ──────────────────────────────────────────────────

func TestRepo_GetScheduledEmail_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	se, recipients, err := repo.GetScheduledEmail(context.Background(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, se)
	assert.Nil(t, recipients)
}

func TestRepo_GetScheduledEmail_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	createdBy := uuid.New()
	now := time.Now()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			id, "mv-akrual-summary", "xlsx", "monthly", "08:00",
			[]byte(`["finance@tugu-re.com"]`), true, nil, nil,
			nil, nil, now, createdBy, "TUGURE"))

	repo := reporting.NewRepository(db, nil)
	se, recipients, err := repo.GetScheduledEmail(context.Background(), id, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, se)
	assert.Equal(t, []string{"finance@tugu-re.com"}, recipients)
}

// ─── Repo: ListActiveScheduledEmails ─────────────────────────────────────────

func TestRepo_ListActiveScheduledEmails_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "frequency", "send_time",
		"recipients_jsonb", "active", "subject_template", "body_template",
		"last_sent_at", "last_status", "created_at", "created_by", "tenant_id"}
	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	rows, err := repo.ListActiveScheduledEmails(context.Background(), "TUGURE")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// ─── Repo: UpdateExportLogCompleted ──────────────────────────────────────────

func TestRepo_UpdateExportLogCompleted(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	expiresAt := time.Now().Add(24 * time.Hour)
	mock.ExpectExec(`UPDATE sys.export_log`).
		WithArgs(id, int64(500), "path/to/file.csv", "sha256hex", "https://signed-url", expiresAt, "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := reporting.NewRepository(db, nil)
	err = repo.UpdateExportLogCompleted(context.Background(), id, 500, "path/to/file.csv", "sha256hex", "https://signed-url", expiresAt, "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo: UpdateExportLogDownloaded ─────────────────────────────────────────

func TestRepo_UpdateExportLogDownloaded(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.export_log`).
		WithArgs(id, "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	repo := reporting.NewRepository(db, nil)
	err = repo.UpdateExportLogDownloaded(context.Background(), tx, id, "TUGURE")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Repo: CountMVRows ────────────────────────────────────────────────────────

func TestRepo_CountMVRows_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Sscanf fails → fallback COUNT(*) on replica
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1234)))

	repo := reporting.NewRepository(db, db)
	count, err := repo.CountMVRows(context.Background(), "rpt.mv_status_periode")
	require.NoError(t, err)
	assert.Equal(t, int64(1234), count)
}

func TestRepo_CountMVRows_UnknownMV(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := reporting.NewRepository(db, db)
	_, err = repo.CountMVRows(context.Background(), "rpt.mv_fake")
	assert.Error(t, err)
}

// ─── Repo: MVRepo InsertRefreshLog + UpdateRefreshLog ────────────────────────

func TestMVRepo_InsertRefreshLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	mvRepo := reporting.NewMVRepo(db, nil)
	logRow := &reporting.MVRefreshLog{
		ID:          uuid.New(),
		MVName:      "rpt.mv_status_periode",
		TriggeredBy: reporting.TriggeredByManual,
		Status:      "RUNNING",
		StartedAt:   time.Now(),
		TenantID:    "TUGURE",
	}
	err = mvRepo.InsertRefreshLog(context.Background(), tx, logRow)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMVRepo_UpdateRefreshLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	rc := int64(500)
	mock.ExpectExec(`UPDATE sys.mv_refresh_log`).
		WithArgs(id, "COMPLETED", rc, nil, "TUGURE").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mvRepo := reporting.NewMVRepo(db, nil)
	err = mvRepo.UpdateRefreshLog(context.Background(), id, "COMPLETED", &rc, nil, "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── ListExportLogs with cursor ──────────────────────────────────────────────

func TestRepo_ListExportLogs_WithCursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}
	cursor := time.Now().UTC().Format(time.RFC3339Nano)
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := reporting.NewRepository(db, nil)
	items, nextCursor, hasMore, err := repo.ListExportLogs(context.Background(), cursor, 50, "TUGURE")
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Nil(t, nextCursor)
	assert.False(t, hasMore)
}

func TestRepo_ListExportLogs_HasMore(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	cols := []string{"id", "report_slug", "format", "status", "row_count",
		"sha256_hash", "file_minio_path", "expires_at", "requested_by",
		"requested_at", "completed_at", "downloaded_at"}

	rowsReturn := sqlmock.NewRows(cols)
	// Add limit+1 rows to trigger hasMore=true (limit=2, fetch=3)
	for i := 0; i < 3; i++ {
		rowsReturn.AddRow(uuid.New(), "mv-status-periode", "csv", "COMPLETED",
			nil, nil, nil, nil,
			uuid.New(), time.Now(), nil, nil)
	}
	mock.ExpectQuery(`FROM sys.export_log`).
		WillReturnRows(rowsReturn)

	repo := reporting.NewRepository(db, nil)
	items, nextCursor, hasMore, err := repo.ListExportLogs(context.Background(), "", 2, "TUGURE")
	require.NoError(t, err)
	assert.Len(t, items, 2, "should trim to limit")
	assert.True(t, hasMore)
	assert.NotNil(t, nextCursor)
}
