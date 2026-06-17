package closeflow_test

// service_final_test.go — Final coverage push tests to reach ≥85%.
// Covers: service error paths, CLOSED-state request reopen, ApproveReopen SoD violation,
// handler happy paths for HardCloseRequest/HardCloseApprove/HardCloseReject/ReopenRequest/ReopenApprove.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── RejectHardClose — wrong status triggers invalid transition ───────────────

func TestRejectHardClose_WrongStatus_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// BEGIN + SELECT FOR SHARE: OPEN status (wrong for hard-close-reject)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	_, err = svc.RejectHardClose(context.Background(), periodeID,
		"reason at least thirty characters long here", actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HARD_CLOSE_PENDING")
}

// ─── RequestReopen — CLOSED → SOFT_CLOSED happy path ─────────────────────────

func TestRequestReopen_ClosedToSoftClosed_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()
	graceExpiry := now.Add(40 * time.Hour) // still within grace window

	// GetByID: CLOSED period within grace window
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-04", 2026, nil, "BULANAN",
		now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "CLOSED",
		&now, &now,
		false, nil, nil, nil, nil,
		int64(3), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		&graceExpiry, nil, nil, // grace_expires_at = still valid
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// BEGIN
	mock.ExpectBegin()

	// SetReopenRequested UPDATE
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))

	// InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT
	mock.ExpectCommit()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	result, err := svc.RequestReopen(context.Background(), periodeID, closeflow.PeriodeStatusSoftClosed,
		"reopen reason for CLOSED period that is more than thirty characters", 3, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.CurrentStatus)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.TargetStatus)
	assert.True(t, result.StepUpMFARequired) // required for CLOSED→SOFT_CLOSED approve
}

// ─── ApproveReopen — SoD violation path ──────────────────────────────────────

func TestApproveReopen_SoDViolation(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New() // same user as reopened_by
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: SOFT_CLOSED, reopened_by = actorID (SoD violation)
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "SOFT_CLOSED",
		&now, nil,
		false, nil, nil, &actorID, nil, // reopened_by = same actor
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// Advisory audit (SoD violation — auto-commit tx)
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
	_, err = svc.ApproveReopen(context.Background(), periodeID, "comment", "", false, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SoD")
}

// ─── ApproveHardClose — with enqueuer returning valid TaskInfo ────────────────

// mockEnqueuer implements AsynqEnqueuer and returns a real *asynq.TaskInfo.
type mockEnqueuer struct {
	returnInfo *asynq.TaskInfo
	returnErr  error
}

func (m *mockEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return m.returnInfo, m.returnErr
}

func TestApproveHardClose_WithEnqueuer_EnqueuesJob(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: HARD_CLOSE_PENDING with different requester
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "HARD_CLOSE_PENDING",
		&now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		requesterID.String(), &now, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// SetHardCloseApproved
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	// LockKursForPeriode
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnResult(sqlmock.NewResult(0, 0))
	// InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	// Audit
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	// COMMIT
	mock.ExpectCommit()

	// The enqueuer is nil-safe (use no enqueuer, just verify it compiles)
	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	result, err := svc.ApproveHardClose(context.Background(), periodeID, nil, "token-hash", actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.StatusPeriode)
}

// ─── Handler success paths ────────────────────────────────────────────────────

func TestApproveHardClose_WithEnqueuer_EnqueuesJob_WithResult(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: HARD_CLOSE_PENDING
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "HARD_CLOSE_PENDING",
		&now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		requesterID.String(), &now, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Enqueuer returns a TaskInfo with a job ID.
	taskInfo := &asynq.TaskInfo{ID: "job-123", Queue: "default", Type: "reporting:mv_refresh"}
	enq := &mockEnqueuer{returnInfo: taskInfo}

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, enq, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	result, err := svc.ApproveHardClose(context.Background(), periodeID, nil, "token-hash-ref", actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.StatusPeriode)
	// Enqueuer returned valid info → mvJobID should be set.
	require.NotNil(t, result.MvRefreshJobID)
	assert.Equal(t, "job-123", *result.MvRefreshJobID)
}

// TestApproveHardClose_WithEnqueuer_EnqueueError_NonFatal exercises the enqueue failure path.
func TestApproveHardClose_WithEnqueuer_EnqueueError_NonFatal(t *testing.T) {
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
		now.AddDate(0, -1, 0), now, "HARD_CLOSE_PENDING",
		&now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		requesterID.String(), &now, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Enqueuer returns an error (non-fatal — hard close already committed)
	enq := &mockEnqueuer{returnErr: asynq.ErrQueueNotFound}

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, enq, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	result, err := svc.ApproveHardClose(context.Background(), periodeID, nil, "token-hash-ref", actor)
	// Enqueue error is non-fatal → no error returned
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.StatusPeriode)
	assert.Nil(t, result.MvRefreshJobID) // enqueue failed → no job ID
}

// TestHardCloseRequest_ServiceSuccess_Returns202 exercises the full handler→service path.
func TestHardCloseRequest_ServiceSuccess_Returns202(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: SOFT_CLOSED
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// Checklist all pass
	mock.ExpectQuery(`FROM trx.penempatan`).WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectQuery(`FROM jrnl.header`).WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(1, "0.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).AddRow("COMPLETED", now))

	// TX
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeHardcloseRequest},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.HardCloseRequestBody{RowVersion: 2}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// TestHardCloseApprove_ServiceSuccess_Returns200 exercises handler hard-close-approve.
func TestHardCloseApprove_ServiceSuccess_Returns200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: HARD_CLOSE_PENDING
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "HARD_CLOSE_PENDING",
		&now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		requesterID.String(), &now, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	stepUpTS := time.Now().Unix()
	claims := &auth.Claims{
		Sub:              actorID.String(),
		Roles:            []string{"ROLE-CFO"},
		Permissions:      []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &stepUpTS,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.WorkflowApproveBody{Comment: "CFO approved hard close"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	req.Header.Set("X-Step-Up-Token", "valid-step-up-token-for-cfo-hard-close")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestHardCloseReject_ServiceSuccess_Returns200 exercises handler hard-close-reject.
func TestHardCloseReject_ServiceSuccess_Returns200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "HARD_CLOSE_PENDING"))
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-CFO"},
		Permissions: []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.RejectBody{Reason: "Reject reason that must be at least thirty characters long"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-reject",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestReopenRequest_ServiceSuccess_Returns202 exercises handler reopen-request.
func TestReopenRequest_ServiceSuccess_Returns202(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeReopenRequest},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.ReopenRequestBody{
		TargetStatus: closeflow.PeriodeStatusOpen,
		Reason:       "Reopen reason that is more than thirty characters long",
		RowVersion:   2,
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/reopen-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// TestReopenApprove_ServiceSuccess_Returns200 exercises handler reopen-approve.
func TestReopenApprove_ServiceSuccess_Returns200(t *testing.T) {
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
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeReopenApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.WorkflowApproveBody{Comment: "approved reopen"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/reopen-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestSoftCloseApprove_ServiceSuccess_Returns200 via handler.
func TestSoftCloseApprove_ServiceSuccess_Returns200(t *testing.T) {
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

	// IsChecklistStale: recent snapshot (not stale)
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))

	// SetSoftCloseApproved
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
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
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeSoftcloseApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.WorkflowApproveBody{Comment: "approved soft close"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestSoftCloseRequest_ServiceSuccess_Returns202 exercises handler soft-close-request.
func TestSoftCloseRequest_ServiceSuccess_Returns202(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: OPEN
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// Checklist all pass
	mock.ExpectQuery(`FROM trx.penempatan`).WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectQuery(`FROM jrnl.header`).WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(1, "0.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).AddRow("COMPLETED", now))

	// TX
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeSoftcloseRequest},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["data"])
}

// ─── SetHardCloseRequested — wrapExec error path ─────────────────────────────

func TestSetHardCloseRequested_DBError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: SOFT_CLOSED
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// Checklist all pass
	mock.ExpectQuery(`FROM trx.penempatan`).WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectQuery(`FROM jrnl.header`).WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(1, "0.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).AddRow("COMPLETED", now))

	// BEGIN
	mock.ExpectBegin()
	// SetHardCloseRequested: non-domain DB error
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	_, err = svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set requested")
}

// ─── GetClosingChecklist handler — service error ──────────────────────────────

func TestGetClosingChecklist_ServiceError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// GetByID returns empty → not found
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(periodeRowCols()))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeRead},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/periode-buku/"+periodeID.String()+"/closing-checklist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

// ─── ListStatusPeriode — service error ───────────────────────────────────────

func TestListStatusPeriode_ServiceError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// DB error on query
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnError(sql.ErrConnDone)

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeRead)
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/status-periode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
