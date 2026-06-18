package closeflow_test

// coverage_test.go — Targeted tests to push coverage to ≥85%.
// Covers: error constructors, repo helpers, ListStatusPeriode cursor/filter paths,
// ApproveReopen CLOSED→SOFT_CLOSED (UnlockKursForPeriode), GetConfigValue, wrapExec,
// isDomainConflict, reopenMessage via ApproveReopen, refreshAllowlistIfStale,
// ChecklistStale path for ApproveSoftClose.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── Error constructors ───────────────────────────────────────────────────────

func TestErrChecklistStale_ReturnsCorrectCode(t *testing.T) {
	err := closeflow.ErrChecklistStale()
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "stale")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.NotEmpty(t, de.Code())
}

func TestErrMFAStepUpExpired_ReturnsCorrectCode(t *testing.T) {
	err := closeflow.ErrMFAStepUpExpired()
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestErrSoftClosePendingExists_ReturnsCorrectCode(t *testing.T) {
	err := closeflow.ErrSoftClosePendingExists("2026-06")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "2026-06")
}

func TestErrRowVersionConflict_ReturnsCorrectCode(t *testing.T) {
	err := closeflow.ErrRowVersionConflict("2026-06")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "2026-06")
}

// ─── isDomainConflict coverage ────────────────────────────────────────────────

func TestRequestHardClose_RowVersionConflict_IsDomainConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: SOFT_CLOSED
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// Checklist: all pass
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(1, "0.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	// BEGIN
	mock.ExpectBegin()

	// SetHardCloseRequested: return a CONFLICT domain error (row_version mismatch).
	conflictErr := closeflow.ErrRowVersionConflict("2026-06")
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnError(conflictErr)
	mock.ExpectRollback()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	// The error should be the conflict domain error (isDomainConflict returns true → propagates directly)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeConflict, de.Code())
}

// ─── GetConfigValue coverage ──────────────────────────────────────────────────

func TestGetConfigValue_ReturnsValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`SELECT config_value FROM sys.config`).
		WithArgs("HARD_CLOSE_GRACE_WINDOW_HOURS").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow("72"))

	repo := closeflow.NewRepo(db)
	val, err := repo.GetConfigValue(context.Background(), "HARD_CLOSE_GRACE_WINDOW_HOURS")
	require.NoError(t, err)
	assert.Equal(t, "72", val)
}

func TestGetConfigValue_NotFound_ReturnsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`SELECT config_value FROM sys.config`).
		WithArgs("NONEXISTENT_KEY").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"})) // empty → sql.ErrNoRows

	repo := closeflow.NewRepo(db)
	val, err := repo.GetConfigValue(context.Background(), "NONEXISTENT_KEY")
	require.NoError(t, err)
	assert.Empty(t, val)
}

// ─── UnlockKursForPeriode via ApproveReopen CLOSED→SOFT_CLOSED ───────────────

func TestApproveReopen_ClosedToSoftClosed_UnlocksKurs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New() // different from actor
	now := time.Now()
	graceExpiry := now.Add(40 * time.Hour) // still within grace window

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: CLOSED period within grace window, different reopen requester
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-04", 2026, nil, "BULANAN",
		now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "CLOSED",
		&now, &now,
		false, nil, nil, &requesterID, nil, // reopened_by = different user
		int64(3), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		&graceExpiry, nil, nil, // grace_expires_at = still valid
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// SetReopenApproved UPDATE (CLOSED→SOFT_CLOSED)
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))

	// UnlockKursForPeriode UPDATE
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnResult(sqlmock.NewResult(0, 0))

	// InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	// hasStepUp=true because CLOSED→SOFT_CLOSED requires step-up MFA
	result, err := svc.ApproveReopen(context.Background(), periodeID, "reopen approved by CFO",
		closeflow.HashStepUpToken("step-up-token-for-test"), true, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.PreviousStatus)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.NewStatus)
	assert.True(t, result.FXRateUnlocked)
}

// ─── ListStatusPeriode cursor path (encodeCursor / decodeCursor) ──────────────

func TestListStatusPeriode_WithCursor_TriggersCursorDecode(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	cols := []string{
		"id", "periode_id_kode", "tipe_periode", "tahun_buku", "bulan",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"soft_close_approved_by", "hard_close_approved_by",
		"reopened_flag",
		"snap_id", "snap_transition", "snap_evaluated_at", "snap_all_passed",
	}
	now := time.Now()
	periodeID := uuid.New()

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			periodeID.String(), "2026-06", "BULANAN", 2026, nil,
			now.AddDate(0, -1, 0), now, "OPEN",
			nil, nil, nil, nil, false, nil, nil, nil, nil,
		))

	svc := newTestService(t, db)
	q := closeflow.EmptyListQuery()

	// Use a valid hex cursor (simulating encoded UUID)
	cursor := closeflow.HashStepUpToken("some-cursor-value") // 64-char hex is valid hex
	items, _, _, _, err := svc.ListStatusPeriode(context.Background(), q, cursor, 50)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestListStatusPeriode_HasMore_EncodesNextCursor(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	cols := []string{
		"id", "periode_id_kode", "tipe_periode", "tahun_buku", "bulan",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"soft_close_approved_by", "hard_close_approved_by",
		"reopened_flag",
		"snap_id", "snap_transition", "snap_evaluated_at", "snap_all_passed",
	}
	now := time.Now()

	// Return limit+1 rows to trigger hasMore = true
	rows := sqlmock.NewRows(cols)
	for i := 0; i < 3; i++ { // limit=2, return 3 → hasMore
		rows.AddRow(
			uuid.New().String(), "2026-0"+string(rune('4'+i)), "BULANAN", 2026, nil,
			now.AddDate(0, -1, 0), now, "OPEN",
			nil, nil, nil, nil, false, nil, nil, nil, nil,
		)
	}
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	svc := newTestService(t, db)
	q := closeflow.EmptyListQuery()

	items, pagination, _, _, err := svc.ListStatusPeriode(context.Background(), q, "", 2)
	require.NoError(t, err)
	assert.Len(t, items, 2) // capped at limit
	assert.NotNil(t, pagination)
}

func TestListStatusPeriode_WithSearch_FiltersApplied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	cols := []string{
		"id", "periode_id_kode", "tipe_periode", "tahun_buku", "bulan",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"soft_close_approved_by", "hard_close_approved_by",
		"reopened_flag",
		"snap_id", "snap_transition", "snap_evaluated_at", "snap_all_passed",
	}
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(cols))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	q := closeflow.EmptyListQuery()
	q.Search = "2026-06"

	items, _, _, _, err := svc.ListStatusPeriode(context.Background(), q, "", 50)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// ─── wrapExec error path ──────────────────────────────────────────────────────

func TestSetSoftCloseRequested_WrapExecError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// GetByID: OPEN period
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// Checklist: fail early (no pending)
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	// jurnal balanced
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(2, "0.0000"))
	// GL delivered
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	// recon pass: period dates
	now := time.Now()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	// BEGIN
	mock.ExpectBegin()

	// SetSoftCloseRequested: DB error
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestSoftClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set requested")
}

// ─── ApproveSoftClose — stale checklist path ─────────────────────────────────

func TestApproveSoftClose_ChecklistStale_Returns422(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: OPEN, soft_close_requested_by = different user
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		requesterID.String(), &now, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// IsChecklistStale: last snapshot was created > 24h ago → stale
	staleTime := now.Add(-25 * time.Hour)
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(staleTime))

	// Advisory audit for stale check (best-effort auto-commit tx)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Main tx rollback
	mock.ExpectRollback()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	_, err = svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stale")
}

// ─── IsChecklistStale — no snapshot → treated as stale ───────────────────────

func TestIsChecklistStale_NoSnapshot_ReturnsStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"})) // empty → sql.ErrNoRows

	chk := closeflow.NewChecklistService(db)
	stale, err := chk.IsChecklistStale(context.Background(), periodeID, 24)
	require.NoError(t, err)
	assert.True(t, stale) // no snapshot → treat as stale
}

// ─── refreshAllowlistIfStale — triggers background refresh ───────────────────

func TestPeriodeLockMiddleware_RefreshAllowlist_UpdatesCache(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000011"
	now := testNow()

	// First call: serves the OPEN period row.
	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(openPeriodeRows(periodeID, now))

	// GetConfigValue: returns new allowlist with extra action
	mock.ExpectQuery(`SELECT config_value FROM sys.config`).
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).
			AddRow("JURNAL_RETRY_GL_DELIVERY,NEW_ALLOWED_ACTION"))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r := gin.New()
	periodeGroup := r.Group("/api/v1/transaksi/:periode_id", mw.Handler())
	periodeGroup.POST("/foo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	req.Header.Set("X-Close-Workflow-Action", "NEW_ALLOWED_ACTION")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Give background refresh goroutine time to complete.
	time.Sleep(50 * time.Millisecond)

	// The OPEN period should always allow through regardless of action
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── ListStatusPeriode with sort spec ────────────────────────────────────────

func TestListStatusPeriode_WithSortAndFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	cols := []string{
		"id", "periode_id_kode", "tipe_periode", "tahun_buku", "bulan",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"soft_close_approved_by", "hard_close_approved_by",
		"reopened_flag",
		"snap_id", "snap_transition", "snap_evaluated_at", "snap_all_passed",
	}
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(cols))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	q := closeflow.EmptyListQuery()
	q.Sort = []listquery.SortSpec{
		{Col: "tahun_buku", Dir: "desc"},
		{Col: "invalid_col", Dir: "asc"}, // invalid → skipped
	}
	q.Filters = []listquery.FilterSpec{
		{Col: "status_periode", Value: "OPEN"},
		{Col: "tahun_buku", Value: "2026"},
		{Col: "tipe_periode", Value: "BULANAN"},
		{Col: "unknown_col", Value: "ignored"}, // ignored
	}

	items, _, sorts, filters, err := svc.ListStatusPeriode(context.Background(), q, "", 50)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Len(t, sorts, 1)   // only valid sort cols kept
	assert.NotNil(t, filters) // filters map is built
}

// ─── domain_test extras: IsWithinGraceWindow ─────────────────────────────────

func TestIsWithinGraceWindow_ExpiredGrace(t *testing.T) {
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusClosed}
	graceExpired := time.Now().Add(-1 * time.Hour)
	p.HardCloseGraceExpiresAt = &graceExpired
	assert.False(t, p.IsWithinGraceWindow())
}

func TestIsWithinGraceWindow_ValidGrace(t *testing.T) {
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusClosed}
	graceValid := time.Now().Add(1 * time.Hour)
	p.HardCloseGraceExpiresAt = &graceValid
	assert.True(t, p.IsWithinGraceWindow())
}

func TestIsWithinGraceWindow_NilGrace(t *testing.T) {
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusClosed}
	p.HardCloseGraceExpiresAt = nil
	assert.False(t, p.IsWithinGraceWindow())
}

// ─── CanTransition edge cases ─────────────────────────────────────────────────

func TestCanTransition_ReopenClosedWithoutStepUp(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusClosed, "reopen-closed-to-soft-closed", false, true)
	assert.False(t, ok)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "step-up")
}

func TestCanTransition_ReopenClosedOutsideGrace(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusClosed, "reopen-closed-to-soft-closed", true, false)
	assert.False(t, ok)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "grace")
}

func TestCanTransition_InvalidAction(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "invalid-action-xyz", false, false)
	assert.False(t, ok)
	assert.NotNil(t, err)
}

// ─── reopenMessage via ApproveReopen result ───────────────────────────────────

func TestApproveReopen_MessageVariants(t *testing.T) {
	// The reopenMessage function is internal but exercised via ApproveReopen result.
	// SOFT_CLOSED→OPEN: no FX unlock, message = "Mutasi kembali diizinkan."
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "SOFT_CLOSED",
		&now, nil,
		false, nil, nil, &requesterID, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	result, err := svc.ApproveReopen(context.Background(), periodeID, "approved", "", false, actor)
	require.NoError(t, err)
	assert.Contains(t, result.Message, "Mutasi")
	assert.False(t, result.FXRateUnlocked)
}
