package gldelivery_test

// Repo nil-panic tests + additional path coverage.

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

// ─── Constructor nil panics ───────────────────────────────────────────────────

func TestNewJurnalGLRepo_NilDB_Panics(t *testing.T) {
	assert.Panics(t, func() { NewJurnalGLRepo(nil) })
}

func TestNewDLQRepo_NilDB_Panics(t *testing.T) {
	assert.Panics(t, func() { NewDLQRepo(nil) })
}

func TestNewReconReportRepo_NilDB_Panics(t *testing.T) {
	assert.Panics(t, func() { NewReconReportRepo(nil) })
}

func TestNewReconMismatchRepo_NilDB_Panics(t *testing.T) {
	assert.Panics(t, func() { NewReconMismatchRepo(nil) })
}

// ─── UpdateGLStatus various field combinations ────────────────────────────────

func TestJurnalGLRepo_UpdateGLStatus_WithAllFields(t *testing.T) {
	db, mock := newTestDB(t)
	repo := NewJurnalGLRepo(db)

	callerID := uuid.New()
	now := time.Now()
	retryCount := 2
	lastErr := "timeout"
	category := FailureCategoryInfra
	reason := "manual retry reason"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	err = repo.UpdateGLStatus(context.Background(), tx, uuid.New(), GlStatusUpdateFields{
		GlHostStatus:      GlHostStatusRetrying,
		RetryCount:        &retryCount,
		LastRetryAt:       &now,
		LastError:         &lastErr,
		FailureCategory:   &category,
		ManualRetryBy:     &callerID,
		ManualRetryAt:     &now,
		ManualRetryReason: &reason,
	})
	require.NoError(t, err)
	_ = tx.Commit()
}

func TestJurnalGLRepo_UpdateGLStatus_DeadLetter(t *testing.T) {
	db, mock := newTestDB(t)
	repo := NewJurnalGLRepo(db)

	callerID := uuid.New()
	now := time.Now()
	reason := "discarding from DLQ"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	err = repo.UpdateGLStatus(context.Background(), tx, uuid.New(), GlStatusUpdateFields{
		GlHostStatus:  GlHostStatusDeadLetter,
		DiscardedBy:   &callerID,
		DiscardedAt:   &now,
		DiscardReason: &reason,
	})
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── JurnalGLRepo.GetDeliveryStatus NullUUID fields ─────────────────────────

func TestJurnalGLRepo_GetDeliveryStatus_WithNullableFields(t *testing.T) {
	db, mock := newTestDB(t)
	repo := NewJurnalGLRepo(db)

	statusID := uuid.New()
	// Row with manual_retry_by, delivered_at, last_retry_at all NULL.
	rows := sqlmock.NewRows([]string{
		"id", "gl_host_status", "gl_host_journal_id", "delivered_at",
		"retry_count", "last_retry_at", "last_error", "failure_category",
		"delivery_mode", "payload_sent_at", "gl_response_payload_jsonb",
		"manual_retry_by", "manual_retry_at", "manual_retry_reason",
		"delivery_response_id",
	}).AddRow(
		statusID, "RETRYING", nil, nil,
		3, nil, "timeout", "INFRA",
		"API", nil, []byte(`{"gl":"response","api_key":"REDACTED"}`),
		nil, nil, nil,
		nil,
	)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(rows)

	ds, err := repo.GetDeliveryStatus(context.Background(), uuid.New())
	require.NoError(t, err)
	require.NotNil(t, ds)
	assert.Equal(t, GlHostStatusRetrying, ds.GlHostStatus)
	assert.Equal(t, 3, ds.RetryCount)
}

// ─── ReconMismatchRepo.SoftDeleteByReportID ──────────────────────────────────

func TestReconMismatchRepo_SoftDeleteByReportID(t *testing.T) {
	db, mock := newTestDB(t)
	repo := NewReconMismatchRepo(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys\.gl_recon_mismatch SET deleted_at`).WillReturnResult(sqlmock.NewResult(5, 5))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	err = repo.SoftDeleteByReportID(context.Background(), tx, uuid.New(), uuid.New())
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── ReconReportRepo.Insert ───────────────────────────────────────────────────

func TestReconReportRepo_Insert_Success(t *testing.T) {
	db, mock := newTestDB(t)
	repo := NewReconReportRepo(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	actorID := uuid.New()
	report := &ReconciliationReport{
		ID:            uuid.New(),
		TanggalRun:    time.Now(),
		TriggerSource: "CRON",
		Status:        ReconStatusInProgress,
		ToleranceIDR:  DefaultConfig().ToleranceIDR,
	}
	err = repo.Insert(context.Background(), tx, report, actorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── ReconReportRepo.List — tested via service_success_test.go ListReports tests. ─
