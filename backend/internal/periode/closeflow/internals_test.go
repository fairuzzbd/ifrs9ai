package closeflow_test

// internals_test.go — Targeted tests using exported internal helpers (via export_test.go)
// to cover branches that are hard to reach via the normal call graph.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── reopenMessage — all three branches ───────────────────────────────────────

func TestReopenMessage_FromClosed_FXUnlocked(t *testing.T) {
	msg := closeflow.ReopenMessage(true, true)
	assert.Contains(t, msg, "FX")
}

func TestReopenMessage_FromClosed_FXNotUnlocked(t *testing.T) {
	// Dead branch in production (fxUnlocked always true when fromClosed),
	// but reachable via the exported helper.
	msg := closeflow.ReopenMessage(true, false)
	assert.NotEmpty(t, msg)
}

func TestReopenMessage_NotFromClosed(t *testing.T) {
	msg := closeflow.ReopenMessage(false, false)
	assert.NotEmpty(t, msg)
}

// ─── GetChecklist — Evaluate fails ───────────────────────────────────────────

func TestGetChecklist_EvalError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// GetByID returns OPEN period.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// checkPendingApprovalZero fails with a connection error.
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnError(sql.ErrConnDone)

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}

	_, err = svc.GetChecklist(context.Background(), periodeID, actor)
	require.Error(t, err)
}

// ─── ListStatusPeriode — invalid hex cursor is silently ignored ───────────────

func TestListStatusPeriode_InvalidHexCursor_IgnoresCursor(t *testing.T) {
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

	svc := buildTestSvc(t, db)
	q := closeflow.EmptyListQuery()

	// Invalid hex cursor — decodeCursor should fail gracefully (cursor skipped).
	items, pagination, _, _, listErr := svc.ListStatusPeriode(
		context.Background(), q, "ZZZ-NOT-VALID-HEX!@#", 50)
	require.NoError(t, listErr)
	assert.Empty(t, items)
	assert.NotNil(t, pagination)
}

// ─── PeriodeLockMiddleware — HARD_CLOSE_PENDING status blocks mutations ────────

func TestPeriodeLockMiddleware_HardClosePending_BlocksMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000006"
	now := time.Now()

	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").
		WillReturnRows(sqlmock.NewRows(periodeRowCols()).AddRow(
			periodeID, "2026-05", 2026, nil, "BULANAN",
			now, now, "HARD_CLOSE_PENDING",
			&now, nil,
			false, nil, nil, nil, nil,
			int64(2), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
		))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r, _ := setupMiddlewareRouter(t, mw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 423, w.Code)
}

// ─── ApproveSoftClose — SoD skipped when soft_close_requested_by is nil ──────

func TestApproveSoftClose_NoRequestedBy_SoDSkipped(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE — OPEN period, soft_close_requested_by = nil → SoD check skipped.
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil, // soft_close_requested_by = nil
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// IsChecklistStale — fresh snapshot.
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))

	// SetSoftCloseApproved.
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	// InsertChecklistSnapshot.
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	// Audit SELECT + INSERT.
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	// COMMIT.
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	result, err := svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.StatusPeriode)
}

// ─── RequestHardClose — checklist all fail, but snapshot still committed ──────

func TestRequestHardClose_AllChecklistFails_SnapshotPersisted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: SOFT_CLOSED.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// All 4 checklist items fail.
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5))
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(3, "200.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(1, "{jrnl-1}"))
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}))

	// BEGIN → snapshot persisted even on checklist failure.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	// Returns ErrChecklistNotAllPassed when items fail.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "item")
}

// ─── RequestReopen — wrong source status (default case) ──────────────────────

func TestRequestReopen_WrongSourceStatus_ReturnsInvalidTransition(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// OPEN period → not a valid source for reopen.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestReopen(context.Background(), periodeID, closeflow.PeriodeStatusOpen, "", 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPEN")
}

// ─── RejectHardClose — periode not found ─────────────────────────────────────

func TestRejectHardClose_PeriodeNotFound_ReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(periodeRowCols()))
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	_, err = svc.RejectHardClose(context.Background(), periodeID, "rejected", actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
}

// ─── ApproveReopen — invalid source status (default case in switch) ───────────

func TestApproveReopen_InvalidSourceStatus_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	mock.ExpectBegin()
	// OPEN period — invalid for reopen-approve.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	_, err = svc.ApproveReopen(context.Background(), periodeID, "ok", "", true, actor)
	require.Error(t, err)
}

// ─── PeriodeLockMiddleware — stale allowlist cache refresh ───────────────────

func TestPeriodeLockMiddleware_StaleCache_TriggersRefresh(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000007"
	now := time.Now()

	// The middleware will query DB for period status.
	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").
		WillReturnRows(openPeriodeRows(periodeID, now))

	// GetConfigValue (called in background goroutine — may or may not fire within test window).
	// We can't reliably set an expectation for a goroutine, so allow unused expectations.

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	// Force the cache to appear stale.
	mw.ExpireAllowlistCache()

	r, _ := setupMiddlewareRouter(t, mw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// OPEN period → allowed through regardless of allowlist state.
	assert.Equal(t, http.StatusOK, w.Code)

	// Give the background goroutine a short window to finish (best-effort).
	time.Sleep(10 * time.Millisecond)
}

// ─── RequestHardClose: additional uncovered paths ─────────────────────────────

// TestRequestHardClose_PeriodeNotFound: GetByID returns nil → ErrPeriodeNotFound.
func TestRequestHardClose_PeriodeNotFound_ReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(periodeRowCols())) // empty → nil

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
}

// TestRequestHardClose_WrongStatus_ReturnsInvalidTransition: OPEN → hard-close-request denied.
func TestRequestHardClose_WrongStatus_ReturnsInvalidTransition(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SOFT_CLOSED")
}

// TestRequestHardClose_ChecklistEvalError: DB error in checklist eval propagates.
func TestRequestHardClose_ChecklistEvalError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	// GetByID: SOFT_CLOSED
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))
	// Checklist: first query returns DB error
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnError(sql.ErrConnDone)

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checklist eval")
}

// ─── ApproveSoftClose: error paths ───────────────────────────────────────────

// TestApproveSoftClose_PeriodeNotFound: SELECT FOR SHARE returns nil.
func TestApproveSoftClose_PeriodeNotFound_ReturnsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(periodeRowCols()))
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
}

// TestApproveSoftClose_WrongStatus_InvalidTransition: period OPEN → approve fails.
func TestApproveSoftClose_WrongStatus_InvalidTransition(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	_, err = svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.Error(t, err)
	// OPEN → soft-close-approve transition is invalid per CanTransition.
}
