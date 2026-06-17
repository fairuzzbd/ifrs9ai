package closeflow_test

// service_test.go — Integration-style tests for Service methods using sqlmock.
//
// Pattern: each test creates a sql.DB mock, injects into Repo + ChecklistService,
// sets expectations, calls service method, asserts result/error.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newTestService(t *testing.T, db *sql.DB) *closeflow.Service {
	t.Helper()
	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	cfg := closeflow.DefaultConfig()
	return closeflow.NewService(repo, chk, aw, nil, cfg, nil)
}

func newTestPeriodeRow() *sqlmock.Rows {
	cols := []string{
		"id", "periode_id_kode", "tahun_buku", "bulan", "tipe_periode",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"reopened_flag", "reopened_reason", "reopened_at", "reopened_by", "reopened_approved_by",
		"row_version", "tenant_id", "created_at", "updated_at",
		"soft_close_requested_by", "soft_close_requested_at", "soft_close_request_reason",
		"soft_close_approved_by", "soft_close_approved_at", "soft_close_approve_reason",
		"hard_close_requested_by", "hard_close_requested_at", "hard_close_request_reason",
		"hard_close_approved_by", "hard_close_approved_at", "hard_close_approve_reason",
		"hard_close_grace_expires_at", "step_up_token_ref", "reopen_reason",
	}
	return sqlmock.NewRows(cols)
}

// ─── NewService panics ────────────────────────────────────────────────────────

func TestNewService_PanicsOnNilRepo(t *testing.T) {
	db, _, _ := sqlmock.New()
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	assert.Panics(t, func() {
		closeflow.NewService(nil, chk, aw, nil, closeflow.DefaultConfig(), nil)
	})
}

func TestNewService_PanicsOnNilChecklist(t *testing.T) {
	db, _, _ := sqlmock.New()
	repo := closeflow.NewRepo(db)
	aw := audit.NewWriter(db)
	assert.Panics(t, func() {
		closeflow.NewService(repo, nil, aw, nil, closeflow.DefaultConfig(), nil)
	})
}

func TestNewService_PanicsOnNilAuditWriter(t *testing.T) {
	db, _, _ := sqlmock.New()
	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	assert.Panics(t, func() {
		closeflow.NewService(repo, chk, nil, nil, closeflow.DefaultConfig(), nil)
	})
}

// ─── NewRepo panics ───────────────────────────────────────────────────────────

func TestNewRepo_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		closeflow.NewRepo(nil)
	})
}

// ─── NewChecklistService panics ───────────────────────────────────────────────

func TestNewChecklistService_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		closeflow.NewChecklistService(nil)
	})
}

// ─── RequestSoftClose — PeriodeNotFound ───────────────────────────────────────

func TestRequestSoftClose_PeriodeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty result

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}

	_, err = svc.RequestSoftClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── RequestSoftClose — WrongStatus ──────────────────────────────────────────

func TestRequestSoftClose_WrongStatus_SoftClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	rows := newTestPeriodeRow().AddRow(
		periodeID, "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now,
		"SOFT_CLOSED", // wrong status
		&now, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)

	mock.ExpectQuery(`SELECT`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(rows)

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}

	_, err = svc.RequestSoftClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SOFT_CLOSED")
}

// ─── RejectHardClose — PeriodeNotFound ───────────────────────────────────────

func TestRejectHardClose_PeriodeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// Expect BEGIN + SELECT FOR SHARE → no rows.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}

	_, err = svc.RejectHardClose(context.Background(), periodeID, "reason-at-least-30-chars-long-enough", actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetChecklist — PeriodeNotFound ──────────────────────────────────────────

func TestGetChecklist_PeriodeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AUDIT"}

	_, err = svc.GetChecklist(context.Background(), periodeID, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), periodeID.String())
}

// ─── ListStatusPeriode — EmptyResult ─────────────────────────────────────────

func TestListStatusPeriode_EmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New()
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
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows(cols))

	svc := newTestService(t, db)
	q := closeflow.EmptyListQuery()

	items, pagination, _, _, err := svc.ListStatusPeriode(context.Background(), q, "", 50)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.NotNil(t, pagination)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ─── ApproveHardClose — missing step-up token (service-side guard) ────────────
// (Handler validates claims.NeedsStepUp(), but service receives the already-hashed ref)

func TestApproveHardClose_SoDViolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// SELECT FOR SHARE returns HARD_CLOSE_PENDING with same actor as requester.
	rows := newTestPeriodeRow().AddRow(
		periodeID, "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now,
		"HARD_CLOSE_PENDING",
		&now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil,
		nil, nil, nil,
		&actorID, &now, nil, // hard_close_requested_by = actorID (same as CFO)
		nil, nil, nil,
		nil, nil, nil,
	)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).WithArgs(periodeID, "TUGURE").WillReturnRows(rows)
	// After SoD check fails → advisory audit write (best-effort).
	// The advisory audit uses Writer.Write() which opens its own tx.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectRollback() // main tx rollback

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}

	_, err = svc.ApproveHardClose(context.Background(), periodeID, nil, "hash-ref-abc", actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SoD")
}
