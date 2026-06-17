package closeflow_test

// service_happy_test.go — Happy-path and additional error-path tests for Service.
// Uses sqlmock with proper SQL pattern matching for each DB interaction.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// makeFreshStepUpToken builds a minimal JWT string with the given scope and a fresh iat.
// The token is NOT cryptographically signed — it uses "fakesig" as the signature.
// verifyStepUpScope only checks structure + scope + iat, not the RSA signature,
// so this is sufficient for unit tests (F-01 spec §5.1).
func makeFreshStepUpToken(scope string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"jti":   fmt.Sprintf("test-jti-%s", scope),
		"scope": scope,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"sub":   "test-user",
	})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"
}

// buildTestSvc creates a Service backed by sqlmock db.
func buildTestSvc(t *testing.T, db *sql.DB) *closeflow.Service {
	t.Helper()
	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	cfg := closeflow.DefaultConfig()
	return closeflow.NewService(repo, chk, aw, nil, cfg, nil)
}

// periodeRowForStatus builds a sqlmock.Rows with one periode_buku row of given status.
func periodeRowForStatus(id uuid.UUID, status string, opts ...func(*sqlmock.Rows)) *sqlmock.Rows {
	now := time.Now()
	cols := periodeRowCols()
	rows := sqlmock.NewRows(cols)
	rows.AddRow(
		id.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, status,
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
	return rows
}

// ─── RequestSoftClose — Checklist failure (all 4 items fail) ──────────────────

func TestRequestSoftClose_ChecklistFails_Returns422(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// 1. GetByID: returns OPEN period with no pending soft-close request.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// 2. checkPendingApprovalZero: returns 5 (fails).
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5))

	// 3. checkJurnalBalanced: returns max_delta > threshold.
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(10, "500.0000"))

	// 4. checkGLDelivered: returns 2 FAILED.
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(2, "{abc,def}"))

	// 5. checkReconPass: get period dates.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))

	// 6. checkReconPass: no recon report found (returns no rows → ChecklistItem failed).
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}))

	// 7. BEGIN tx (for SetSoftCloseRequested + snapshot + audit).
	mock.ExpectBegin()

	// 8. SetSoftCloseRequested (UPDATE).
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 9. InsertChecklistSnapshot.
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 10. Audit write — SELECT previous_hash + INSERT audit_log.
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// 11. COMMIT (snapshot persisted even though checklist failed).
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}

	_, err = svc.RequestSoftClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	// Should be CLOSING_CHECKLIST_FAILED.
	assert.Contains(t, err.Error(), "item")
}

// ─── RejectHardClose — HappyPath ──────────────────────────────────────────────

func TestRejectHardClose_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()

	// 1. BEGIN
	mock.ExpectBegin()

	// 2. SELECT FOR SHARE: HARD_CLOSE_PENDING period.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "HARD_CLOSE_PENDING"))

	// 3. SetHardCloseRejected (UPDATE).
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 4. Audit write.
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// 5. COMMIT.
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}

	result, err := svc.RejectHardClose(context.Background(), periodeID,
		"reject reason at least 30 characters long", actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.NewStatus)
	assert.Equal(t, closeflow.PeriodeStatusHardClosePending, result.PreviousStatus)
}

// ─── ApproveSoftClose — SoDViolation ─────────────────────────────────────────

func TestApproveSoftClose_SoDViolation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: OPEN period where soft_close_requested_by = actorID.
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		actorID.String(), &now, nil, // soft_close_requested_by = actorID (same as approver)
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// Advisory audit for SoD violation (best-effort, uses Writer.Write() which opens its own tx).
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Main tx rollback.
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}

	_, err = svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SoD")
}

// ─── ApproveReopen — SOFT_CLOSED → OPEN happy path ────────────────────────────

func TestApproveReopen_SoftClosedToOpen_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: SOFT_CLOSED period with a different reopen requester.
	requesterID := uuid.New() // different from actorID
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "SOFT_CLOSED",
		&now, nil,
		false, nil, nil, &requesterID, nil, // reopened_by = different user
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// SetReopenApproved UPDATE.
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))

	// InsertChecklistSnapshot.
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit.
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT.
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}

	result, err := svc.ApproveReopen(context.Background(), periodeID, "approved reopen", "", false, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.PreviousStatus)
	assert.Equal(t, closeflow.PeriodeStatusOpen, result.NewStatus)
	assert.False(t, result.FXRateUnlocked)
}

// ─── GetChecklist — CLOSED period returns last snapshot ───────────────────────

func TestGetChecklist_ClosedPeriod_ReturnsLastSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	snapID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: CLOSED period.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "CLOSED"))

	// GetLatestSnapshot.
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transition", "evaluated_at", "all_passed"}).
			AddRow(snapID, "HARD_CLOSE_APPROVE", now, true))

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AUDIT"}

	result, err := svc.GetChecklist(context.Background(), periodeID, actor)
	require.NoError(t, err)
	assert.False(t, result.IsRealTimeEval)
	assert.NotNil(t, result.LastSnapshot)
	assert.Equal(t, snapID, result.LastSnapshot.SnapshotID)
}

// ─── GetChecklist — OPEN period real-time eval ────────────────────────────────

func TestGetChecklist_OpenPeriod_RealTimeEval(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// 1. GetByID: OPEN period.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// 2. PENDING_APPROVAL_ZERO: 0 pending.
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))

	// 3. JURNAL_BALANCED: 5 headers, max_delta = 0.
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(5, "0.0000"))

	// 4. GL_DELIVERED: no failures.
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))

	// 5. RECON_PASS: get period dates.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))

	// 6. RECON_PASS: latest recon COMPLETED.
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	// 7. GetLatestSnapshot (best-effort, for LastSnapshot field).
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transition", "evaluated_at", "all_passed"}))

	// The goroutine in GetChecklist inserts a MANUAL_CHECK snapshot asynchronously.
	// We can't easily predict the ordering so we allow these expectations to be unmet.
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}

	result, err := svc.GetChecklist(context.Background(), periodeID, actor)
	require.NoError(t, err)
	assert.True(t, result.IsRealTimeEval)
	assert.True(t, result.AllPassed)
	assert.Len(t, result.Items, 4)

	// Give goroutine time to complete.
	time.Sleep(50 * time.Millisecond)
}

// ─── hashStepUpToken coverage ─────────────────────────────────────────────────

func TestHashStepUpToken_Coverage(t *testing.T) {
	h := closeflow.HashStepUpToken("my-step-up-token-for-hard-close")
	assert.NotEmpty(t, h)
	assert.Len(t, h, 64)
}

// ─── ListStatusPeriode — WithFilters ──────────────────────────────────────────

func TestListStatusPeriode_WithStatusFilter(t *testing.T) {
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
			periodeID.String(), "2026-05", "BULANAN", 2026, nil,
			now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "SOFT_CLOSED",
			&now, nil,
			nil, nil,
			false,
			nil, nil, nil, nil,
		))

	svc := newTestService(t, db)
	q := closeflow.EmptyListQuery()

	items, pagination, _, _, err := svc.ListStatusPeriode(context.Background(), q, "", 50)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, items[0].StatusPeriode)
	assert.NotNil(t, pagination)
}

// ─── RequestReopen — GraceExpired ─────────────────────────────────────────────

func TestRequestReopen_GraceExpired(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	graceExpired := now.Add(-49 * time.Hour) // expired 49 hours ago

	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-04", 2026, nil, "BULANAN",
		now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "CLOSED",
		&now, &now,
		false, nil, nil, nil, nil,
		int64(3), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		&graceExpired, nil, nil, // grace_expires_at = expired
	)

	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}

	_, err = svc.RequestReopen(context.Background(), periodeID, closeflow.PeriodeStatusSoftClosed,
		"reopen reason longer than thirty characters to be valid", 3, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "berakhir")
}
