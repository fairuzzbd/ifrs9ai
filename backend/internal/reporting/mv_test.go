package reporting_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// ─── AllMVNames ───────────────────────────────────────────────────────────────

func TestAllMVNames_Count(t *testing.T) {
	assert.Len(t, reporting.AllMVNames, 8, "must manage exactly 8 MVs")
}

func TestAllMVNames_SchemaPrefix(t *testing.T) {
	for _, name := range reporting.AllMVNames {
		assert.Regexp(t, `^rpt\.mv_`, name, "all MVs must be in rpt schema")
	}
}

func TestValidReportSlugs_AllHaveKnownMV(t *testing.T) {
	mvSet := make(map[string]bool)
	for _, n := range reporting.AllMVNames {
		mvSet[n] = true
	}
	for slug, mv := range reporting.ValidReportSlugs {
		assert.True(t, mvSet[mv], "slug %q maps to unknown MV %q", slug, mv)
	}
}

// ─── isValidMVName (via RefreshConcurrent) ───────────────────────────────────

func TestRefreshConcurrent_RejectsUnknownMVName(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	_ = mock // no expectations: function returns before any DB call

	_, err = reporting.RefreshConcurrent(context.Background(), db, "malicious; DROP TABLE")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mv_name")
}

func TestRefreshConcurrent_MV_REFRESH_LOCKED_WhenLockNotAcquired(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_status_periode"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock`)).
		WithArgs(mvName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	_, err = reporting.RefreshConcurrent(context.Background(), db, mvName)
	assert.Error(t, err)
	// Should be domain error MV_REFRESH_LOCKED.
	assert.Contains(t, err.Error(), "Refresh")
}

func TestRefreshConcurrent_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_jurnal_summary"

	// pg_try_advisory_lock → acquired.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock`)).
		WithArgs(mvName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	// REFRESH MATERIALIZED VIEW CONCURRENTLY.
	mock.ExpectExec(`REFRESH MATERIALIZED VIEW CONCURRENTLY`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// COUNT(*).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(500)))

	// pg_advisory_unlock (defer).
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock`)).
		WithArgs(mvName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rowCount, err := reporting.RefreshConcurrent(context.Background(), db, mvName)
	require.NoError(t, err)
	assert.Equal(t, int64(500), rowCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── MVRepo.IsRefreshRunning ──────────────────────────────────────────────────

func TestMVRepo_IsRefreshRunning_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WithArgs("rpt.mv_akrual_summary", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{}))

	repo := reporting.NewMVRepo(db, nil)
	running, log, err := repo.IsRefreshRunning(context.Background(), "rpt.mv_akrual_summary", "TUGURE")
	require.NoError(t, err)
	assert.False(t, running)
	assert.Nil(t, log)
}

func TestMVRepo_IsRefreshRunning_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	id := uuid.New()
	cols := []string{"id", "mv_name", "triggered_by", "trigger_actor", "status", "started_at", "tenant_id"}
	rows := sqlmock.NewRows(cols).AddRow(
		id, "rpt.mv_status_periode", "CRON", nil, "RUNNING", time.Now(), "TUGURE")

	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WithArgs("rpt.mv_status_periode", "TUGURE").
		WillReturnRows(rows)

	repo := reporting.NewMVRepo(db, nil)
	running, log, err := repo.IsRefreshRunning(context.Background(), "rpt.mv_status_periode", "TUGURE")
	require.NoError(t, err)
	assert.True(t, running)
	require.NotNil(t, log)
	assert.Equal(t, "RUNNING", log.Status)
}

// ─── Now variable (mockable) ──────────────────────────────────────────────────

func TestNow_Mockable(t *testing.T) {
	fixed := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)
	orig := reporting.Now
	reporting.Now = func() time.Time { return fixed }
	defer func() { reporting.Now = orig }()

	got := reporting.Now()
	assert.Equal(t, fixed, got)
}

// ─── DSN routing ─────────────────────────────────────────────────────────────

func TestChooseDB_PrimaryWhenReplicaNil(t *testing.T) {
	primary := &sql.DB{}
	got := reporting.ChooseDB(primary, nil, reporting.ReadIntentReporting)
	assert.Same(t, primary, got, "should return primary when replica is nil")
}

func TestChooseDB_ReplicaForReportingIntent(t *testing.T) {
	primary := &sql.DB{}
	replica := &sql.DB{}
	got := reporting.ChooseDB(primary, replica, reporting.ReadIntentReporting)
	assert.Same(t, replica, got)
}

func TestChooseDB_PrimaryForPrimaryIntent(t *testing.T) {
	primary := &sql.DB{}
	replica := &sql.DB{}
	got := reporting.ChooseDB(primary, replica, reporting.ReadIntentPrimary)
	assert.Same(t, primary, got)
}

// ─── ChooseDBWithContext ──────────────────────────────────────────────────────

func TestChooseDBWithContext_PrimaryWhenReplicaNil(t *testing.T) {
	primary := &sql.DB{}
	got := reporting.ChooseDBWithContext(context.Background(), primary, nil, reporting.ReadIntentReporting)
	assert.Same(t, primary, got)
}

func TestChooseDBWithContext_ReplicaForReportingIntent(t *testing.T) {
	primary := &sql.DB{}
	replica := &sql.DB{}
	got := reporting.ChooseDBWithContext(context.Background(), primary, replica, reporting.ReadIntentReporting)
	assert.Same(t, replica, got)
}

func TestChooseDBWithContext_PrimaryForPrimaryIntent(t *testing.T) {
	primary := &sql.DB{}
	replica := &sql.DB{}
	got := reporting.ChooseDBWithContext(context.Background(), primary, replica, reporting.ReadIntentPrimary)
	assert.Same(t, primary, got)
}
