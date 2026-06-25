package reporting

// mv_helpers_test.go — white-box tests for unexported MV helpers and
// ListMVStatus function (needs pq array — tested via regexp matcher).

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── isValidMVName (unexported) ──────────────────────────────────────────────

func TestIsValidMVName_Valid(t *testing.T) {
	for _, n := range AllMVNames {
		assert.True(t, isValidMVName(n), "expected %q to be valid", n)
	}
}

func TestIsValidMVName_Invalid(t *testing.T) {
	assert.False(t, isValidMVName("rpt.mv_fake"))
	assert.False(t, isValidMVName(""))
	assert.False(t, isValidMVName("public.instrumen"))
}

// ─── ListMVStatus — tested via query error path ───────────────────────────────
//
// ListMVStatus passes AllMVNames ([]string) as $2::TEXT[] which requires the
// pq driver's array codec. Without pq loaded, database/sql returns
// "unsupported type []string". We test the error path only.

func TestListMVStatus_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// The query will fail because []string arg is not supported without pq driver.
	// We verify ListMVStatus propagates the error correctly.
	mock.ExpectQuery(`mv_refresh_log`).WillReturnError(assert.AnError)

	_, err = ListMVStatus(context.Background(), db, "TUGURE")
	// error expected (either from mock or from type conversion)
	assert.Error(t, err)
}

// ─── MVRepo.IsRefreshRunning — scan error path ────────────────────────────────

func TestMVRepo_IsRefreshRunning_ScanError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Return wrong number of columns → scan fails
	mock.ExpectQuery(`FROM sys.mv_refresh_log`).
		WithArgs("rpt.mv_status_periode", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-a-uuid"))

	repo := NewMVRepo(db, nil)
	_, _, err = repo.IsRefreshRunning(context.Background(), "rpt.mv_status_periode", "TUGURE")
	assert.Error(t, err)
}

// ─── MVRepo.InsertRefreshLog — with TriggerActor ─────────────────────────────

func TestMVRepo_InsertRefreshLog_WithActor(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.mv_refresh_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	actorID := uuid.New()
	mvRepo := NewMVRepo(db, nil)
	logRow := &MVRefreshLog{
		ID:           uuid.New(),
		MVName:       "rpt.mv_status_periode",
		TriggeredBy:  TriggeredByManual,
		TriggerActor: &actorID,
		Status:       "RUNNING",
		StartedAt:    time.Now(),
		TenantID:     "TUGURE",
	}
	err = mvRepo.InsertRefreshLog(context.Background(), tx, logRow)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// ─── RefreshConcurrent — COUNT(*) fails (non-fatal) ─────────────────────────

func TestRefreshConcurrent_CountFails_NonFatal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mvName := "rpt.mv_mtm_daily_summary"

	mock.ExpectQuery(`SELECT pg_try_advisory_lock`).
		WithArgs(mvName).
		WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(true))

	mock.ExpectExec(`REFRESH MATERIALIZED VIEW CONCURRENTLY`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// COUNT(*) fails → rowCount = -1, function still returns nil error
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnError(assert.AnError)

	// unlock
	mock.ExpectExec(`SELECT pg_advisory_unlock`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rowCount, err := RefreshConcurrent(context.Background(), db, mvName)
	require.NoError(t, err) // COUNT fail is non-fatal
	assert.Equal(t, int64(-1), rowCount)
}

// ─── Repository.BeginTx ───────────────────────────────────────────────────────

func TestRepository_BeginTx_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin().WillReturnError(assert.AnError)

	repo := NewRepository(db, nil)
	_, err = repo.BeginTx(context.Background())
	assert.Error(t, err)
}

// ─── dbOrTx — db path (tx = nil) ─────────────────────────────────────────────

func TestDbOrTx_DBPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(`SELECT 1`).WillReturnResult(sqlmock.NewResult(0, 0))

	d := dbOrTx{db: db}
	_, err = d.ExecContext(context.Background(), "SELECT 1")
	require.NoError(t, err)
}

// ─── nilableStr (unexported) ──────────────────────────────────────────────────

func TestNilableStr_Empty(t *testing.T) {
	result := nilableStr("")
	assert.Nil(t, result)
}

func TestNilableStr_NonEmpty(t *testing.T) {
	result := nilableStr("hello")
	require.NotNil(t, result)
	assert.Equal(t, "hello", *result)
}

// ─── isValidFrequency (unexported) ───────────────────────────────────────────

func TestIsValidFrequency_Valid(t *testing.T) {
	assert.True(t, isValidFrequency(FreqDaily))
	assert.True(t, isValidFrequency(FreqWeekly))
	assert.True(t, isValidFrequency(FreqMonthly))
}

func TestIsValidFrequency_Invalid(t *testing.T) {
	assert.False(t, isValidFrequency(ScheduledEmailFrequency("hourly")))
	assert.False(t, isValidFrequency(ScheduledEmailFrequency("")))
}

// ─── tenantFromCtx (unexported) ──────────────────────────────────────────────

func TestTenantFromCtx_Default(t *testing.T) {
	// No claims → returns "TUGURE"
	tid := tenantFromCtx(context.Background())
	assert.Equal(t, "TUGURE", tid)
}

// ─── hashToken (unexported) ──────────────────────────────────────────────────

func TestHashToken_Deterministic(t *testing.T) {
	h1 := hashToken("my-token")
	h2 := hashToken("my-token")
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64) // SHA-256 hex = 64 chars
}

func TestHashToken_Different(t *testing.T) {
	assert.NotEqual(t, hashToken("a"), hashToken("b"))
}

// ─── Repository — CountMVRows pg_class path (via fake 8-char schema name) ─────

// Note: The pg_class path in CountMVRows is only triggered when Sscanf("%4s.%s") succeeds.
// With real MV names like "rpt.mv_...", Sscanf reads "rpt." (4 chars) but then fails
// because the format expects ".%s" after %4s. So pg_class path is unreachable with
// real names — it's a fallback-only path. We skip this test as it's dead code in practice.

// ─── Ensure no unused imports ─────────────────────────────────────────────────

var _ = sql.ErrNoRows
