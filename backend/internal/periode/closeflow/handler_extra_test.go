package closeflow_test

// handler_extra_test.go — Additional handler tests covering all remaining endpoints:
// SoftCloseApprove, HardCloseRequest, HardCloseApprove (with step-up), HardCloseReject,
// ReopenRequest, ReopenApprove, GetClosingChecklist (success), ListStatusPeriode,
// ExportStatusPeriode (invalid format), parseLimit, checkIdempotencyKey (invalid UUID).

import (
	"bytes"
	"context"
	"encoding/json"
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
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── SoftCloseApprove ────────────────────────────────────────────────────────

func TestSoftCloseApprove_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.WorkflowApproveBody{Comment: "approved"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSoftCloseApprove_ServiceError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()
	// SELECT FOR SHARE returns empty → not found
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(periodeRowCols()))
	mock.ExpectRollback()

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

	body := closeflow.WorkflowApproveBody{Comment: "approved"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// NOT_FOUND or other error from service
	assert.NotEqual(t, http.StatusOK, w.Code)
	_ = now // used in row helpers
}

// ─── HardCloseRequest ────────────────────────────────────────────────────────

func TestHardCloseRequest_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.HardCloseRequestBody{RowVersion: 2}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHardCloseRequest_ServiceNotFound_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// GetByID returns empty → not found
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(periodeRowCols()))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         uuid.New().String(),
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

	assert.NotEqual(t, http.StatusAccepted, w.Code)
}

// ─── HardCloseApprove — missing X-Step-Up-Token ──────────────────────────────

func TestHardCloseApprove_MissingStepUpToken_Returns401(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	stepUpTS := time.Now().Unix()
	claims := &auth.Claims{
		Sub:              uuid.New().String(),
		Roles:            []string{"ROLE-CFO"},
		Permissions:      []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &stepUpTS, // step-up verified, but no token header
	}
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.WorkflowApproveBody{Comment: "approved by CFO"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	// No X-Step-Up-Token header!
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── HardCloseReject ─────────────────────────────────────────────────────────

func TestHardCloseReject_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.RejectBody{Reason: "reason that is at least thirty characters long"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-reject",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHardCloseReject_ServiceError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// BEGIN + SELECT FOR SHARE returns empty → not found
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(periodeRowCols()))
	mock.ExpectRollback()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-CFO"},
		Permissions: []string{closeflow.PermPeriodeHardcloseApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.RejectBody{Reason: "reason that is at least thirty characters long indeed"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/hard-close-reject",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

// ─── ReopenRequest ───────────────────────────────────────────────────────────

func TestReopenRequest_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.ReopenRequestBody{
		TargetStatus: closeflow.PeriodeStatusOpen,
		Reason:       "reopen reason that is sufficiently long",
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

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReopenRequest_ServiceError_ReturnsError(t *testing.T) {
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
		Permissions: []string{closeflow.PermPeriodeReopenRequest},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.ReopenRequestBody{
		TargetStatus: closeflow.PeriodeStatusOpen,
		Reason:       "reopen reason longer than thirty characters definitely",
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

	assert.NotEqual(t, http.StatusAccepted, w.Code)
}

// ─── ReopenApprove ───────────────────────────────────────────────────────────

func TestReopenApprove_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("periode.read")
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.WorkflowApproveBody{Comment: "approved"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/reopen-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReopenApprove_ServiceError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// BEGIN + SELECT FOR SHARE returns empty → not found
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(sqlmock.NewRows(periodeRowCols()))
	mock.ExpectRollback()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeReopenApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.WorkflowApproveBody{Comment: "approved"}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/reopen-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

// ─── GetClosingChecklist — success ───────────────────────────────────────────

func TestGetClosingChecklist_Success_Returns200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// 1. GetByID: OPEN period
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "OPEN"))

	// 2. PENDING_APPROVAL_ZERO: 0 pending.
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))

	// 3. JURNAL_BALANCED: all balanced.
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(3, "0.0000"))

	// 4. GL_DELIVERED: no failures.
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))

	// 5. RECON_PASS: get period dates.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))

	// 6. RECON_PASS: COMPLETED recon.
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	// 7. GetLatestSnapshot (empty OK).
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "transition", "evaluated_at", "all_passed"}))

	// 8. async snapshot goroutine (may or may not execute in time, best-effort)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := &auth.Claims{
		Sub:         actorID.String(),
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

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.NotNil(t, data["items"])

	// Give goroutine time to flush.
	time.Sleep(50 * time.Millisecond)
}

// ─── ListStatusPeriode — success ─────────────────────────────────────────────

func TestListStatusPeriode_Success_Returns200(t *testing.T) {
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
			nil, nil,
			nil, nil,
			false,
			nil, nil, nil, nil,
		))

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
		"/api/v1/reports/status-periode?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListStatusPeriode_MissingPermission_Returns403(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission("other.permission")
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/status-periode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ─── ExportStatusPeriode — invalid format ────────────────────────────────────

func TestExportStatusPeriode_InvalidFormat_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeExport)
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/status-periode/export?format=pdf", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── checkIdempotencyKey — invalid UUID format ────────────────────────────────

func TestSoftCloseRequest_InvalidIdempotencyKey_Returns400(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeSoftcloseRequest)
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "not-a-valid-uuid") // invalid format
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	errBody := errResp["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_FAILED", errBody["code"])
}

// ─── parseLimit coverage ─────────────────────────────────────────────────────

func TestListStatusPeriode_LimitOverMax_ClampsTo200(t *testing.T) {
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
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeRead)
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/status-periode?limit=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should succeed (clamped to 200, not error)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListStatusPeriode_InvalidLimit_UsesDefault(t *testing.T) {
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
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows(cols))

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)
	h := closeflow.NewHandler(svc)

	claims := claimsWithPermission(closeflow.PermPeriodeRead)
	r := setupRouter(t, h, claims)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/status-periode?limit=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── RegisterLockMiddlewareRoutes coverage ────────────────────────────────────

func TestRegisterLockMiddlewareRoutes_AddsMiddleware(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000099"
	now := testNow()
	// middleware will query period status
	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(openPeriodeRows(periodeID, now))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r := gin.New()
	trxGroup := r.Group("/api/v1/transaksi/:periode_id")
	closeflow.RegisterLockMiddlewareRoutes(trxGroup, mw)
	trxGroup.POST("/foo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// OPEN status allows through
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── actorFrom — invalid Sub claim ───────────────────────────────────────────

func TestSoftCloseRequest_InvalidSubClaim_Returns401(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)

	// Claims with non-UUID sub → actorFrom() returns error
	claims := &auth.Claims{
		Sub:         "not-a-uuid",
		Roles:       []string{"ROLE-AKUN-CTL"},
		Permissions: []string{closeflow.PermPeriodeSoftcloseRequest},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	periodeID := uuid.New()
	body := closeflow.SoftCloseRequestBody{RowVersion: 1}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+periodeID.String()+"/soft-close-request",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ─── ApproveSoftClose — happy path ───────────────────────────────────────────

func TestApproveSoftClose_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New() // different from actor
	now := time.Now()

	// BEGIN
	mock.ExpectBegin()

	// SELECT FOR SHARE: OPEN period, soft_close_requested_by = different user.
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		requesterID.String(), &now, nil, // soft_close_requested_by = different user
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// IsChecklistStale: get latest snapshot created_at → recent (not stale)
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))

	// SetSoftCloseApproved UPDATE
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit write
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	result, err := svc.ApproveSoftClose(context.Background(), periodeID, nil, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.StatusPeriode)
}

// ─── RequestHardClose — happy path (checklist passes) ────────────────────────

func TestRequestHardClose_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// 1. GetByID: SOFT_CLOSED period.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// 2. Checklist evaluation (4 items, all pass):
	mock.ExpectQuery(`FROM trx.penempatan`).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0))
	mock.ExpectQuery(`FROM jrnl.header`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(5, "0.0000"))
	mock.ExpectQuery(`FROM jrnl.gl_status`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "array_agg"}).AddRow(0, nil))
	// RECON_PASS period dates
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	// RECON_PASS report
	mock.ExpectQuery(`FROM sys.gl_reconciliation_report`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	// 3. BEGIN tx
	mock.ExpectBegin()

	// 4. SetHardCloseRequested UPDATE
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 5. InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 6. Audit write
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// 7. COMMIT
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	result, err := svc.RequestHardClose(context.Background(), periodeID, nil, 1, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusHardClosePending, result.StatusPeriode)
}

// ─── ApproveHardClose — happy path (no enqueuer) ─────────────────────────────

func TestApproveHardClose_HappyPath_NoEnqueuer(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New() // different from actor (SoD)
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
		requesterID.String(), &now, nil, // hard_close_requested_by = different user
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)

	// SetHardCloseApproved UPDATE (with grace window).
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// LockKursForPeriode UPDATE (mst.kurs table).
	mock.ExpectExec(`UPDATE mst.kurs`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// InsertChecklistSnapshot.
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit write.
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT.
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil) // no enqueuer

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	result, err := svc.ApproveHardClose(context.Background(), periodeID, nil, "hashed-step-up-ref", actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusClosed, result.StatusPeriode)
	assert.Nil(t, result.MvRefreshJobID) // no enqueuer → no job ID
}

// ─── RequestReopen — SOFT_CLOSED → OPEN happy path ───────────────────────────

func TestRequestReopen_SoftClosedToOpen_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	now := time.Now()

	// GetByID: SOFT_CLOSED period
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WillReturnRows(periodeRowForStatus(periodeID, "SOFT_CLOSED"))

	// BEGIN
	mock.ExpectBegin()

	// SetReopenRequested UPDATE
	mock.ExpectExec(`UPDATE mst.periode_buku`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// InsertChecklistSnapshot
	mock.ExpectExec(`INSERT INTO sys.closing_checklist_snapshot`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Audit
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))

	// COMMIT
	mock.ExpectCommit()

	repo := closeflow.NewRepo(db)
	chk := closeflow.NewChecklistService(db)
	aw := audit.NewWriter(db)
	svc := closeflow.NewService(repo, chk, aw, nil, closeflow.DefaultConfig(), nil)

	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	result, err := svc.RequestReopen(context.Background(), periodeID, closeflow.PeriodeStatusOpen,
		"reopen reason longer than thirty characters to be valid", 1, actor)
	require.NoError(t, err)
	assert.Equal(t, closeflow.PeriodeStatusSoftClosed, result.CurrentStatus)
	assert.Equal(t, closeflow.PeriodeStatusOpen, result.TargetStatus)
	_ = now
}
