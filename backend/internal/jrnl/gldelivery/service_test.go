package gldelivery_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

// requireDomainCode asserts that err is a domain error with the given code.
func requireDomainCode(t *testing.T, err error, code domainerrors.Code) {
	t.Helper()
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "expected domain error, got: %v", err)
	assert.Equal(t, code, de.Code())
}

// ─── constructor panic tests ─────────────────────────────────────────────────

func TestDeliveryService_NilRepo_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()
	assert.Panics(t, func() {
		NewDeliveryService(nil, NewDLQRepo(db), stub, aw, nil, DefaultConfig(), nil)
	})
}

func TestDeliveryService_NilAuditWriter_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	stub := NewStubAdapter()
	assert.Panics(t, func() {
		NewDeliveryService(NewJurnalGLRepo(db), NewDLQRepo(db), stub, nil, nil, DefaultConfig(), nil)
	})
}

func TestDeliveryService_NilAdapter_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	aw := audit.NewWriter(db)
	assert.Panics(t, func() {
		NewDeliveryService(NewJurnalGLRepo(db), NewDLQRepo(db), nil, aw, nil, DefaultConfig(), nil)
	})
}

func TestDLQService_NilAuditWriter_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	stub := NewStubAdapter()
	aw := audit.NewWriter(db)
	delivery := NewDeliveryService(NewJurnalGLRepo(db), NewDLQRepo(db), stub, aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewDLQService(NewDLQRepo(db), NewJurnalGLRepo(db), delivery, nil, nil, nil)
	})
}

func TestReconciliationService_NilAuditWriter_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	stub := NewStubAdapter()
	assert.Panics(t, func() {
		NewReconciliationService(
			NewJurnalGLRepo(db), NewReconReportRepo(db), NewReconMismatchRepo(db),
			stub, nil, nil, DefaultConfig(), nil,
		)
	})
}

// ─── DeliveryService.DeliverToGL ─────────────────────────────────────────────

func TestDeliverToGL_NotFound(t *testing.T) {
	_, delivery, _, _, _, mock := newTestDelivery(t) //nolint:dogsled
	headerID := uuid.New()
	// Header not found — return empty rows.
	mock.ExpectQuery(`SELECT h\.id`).WillReturnRows(sqlmock.NewRows(nil))

	err := delivery.DeliverToGL(context.Background(), headerID)
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryJurnalNotFound)
}

func TestDeliverToGL_AlreadyDelivered_SkipsResend(t *testing.T) {
	_, delivery, _, stub, _, mock := newTestDelivery(t)

	headerID, statusID := uuid.New(), uuid.New()
	mockHeaderAndDetail(t, mock, headerID, statusID, GlHostStatusDelivered, 0)

	err := delivery.DeliverToGL(context.Background(), headerID)
	require.NoError(t, err)
	// Adapter should NOT have been called for already-delivered status.
	assert.Len(t, stub.Calls(), 0)
}

func TestDeliverToGL_Success(t *testing.T) {
	_, delivery, _, stub, _, mock := newTestDelivery(t)

	headerID, statusID := uuid.New(), uuid.New()
	mockHeaderAndDetail(t, mock, headerID, statusID, GlHostStatusPendingDelivery, 0)
	// Expect IN_FLIGHT update + audit tx.
	expectStatusUpdateTx(mock)
	// Expect DELIVERED update + audit tx.
	expectStatusUpdateTx(mock)

	err := delivery.DeliverToGL(context.Background(), headerID)
	require.NoError(t, err)
	assert.Len(t, stub.Calls(), 1)
}

func TestDeliverToGL_InfraError_StubClassification(t *testing.T) {
	// Tests adapter-level 5xx classification (unit test, no DB needed).
	stub503 := NewStubAdapter(StubConfig{FailHTTPStatus: 503, FailMessage: "service down"})
	_, err := stub503.Post(context.Background(), makeTestPayload(), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestDeliverToGL_MaxAttemptsExceeded_GoesToDLQ(t *testing.T) {
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	stub := NewStubAdapter()
	aw := audit.NewWriter(db)
	cfg := DefaultConfig()
	cfg.MaxTotalAttempts = 2 // lower threshold for test
	delivery := NewDeliveryService(jurnalRepo, dlqRepo, stub, aw, nil, cfg, nil)

	headerID, statusID := uuid.New(), uuid.New()
	// retryCount=5 which is >= MaxTotalAttempts=2
	mockHeaderAndDetail(t, mock, headerID, statusID, GlHostStatusRetrying, 5)

	// Expect DLQ insert tx (WithTx audit — no nested BEGIN).
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.dlq_gl_delivery`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Error from DLQ path is returned (or nil if DLQ consumed it).
	_ = delivery.DeliverToGL(context.Background(), headerID)
	// Adapter should NOT have been called.
	assert.Len(t, stub.Calls(), 0)
}

// ─── DeliveryService.ManualRetry ──────────────────────────────────────────────

func TestManualRetry_ReasonTooShort(t *testing.T) {
	_, delivery, _, _, _, _ := newTestDelivery(t) //nolint:dogsled
	_, err := delivery.ManualRetry(context.Background(), uuid.New(), "short", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryReasonTooShort)
}

func TestManualRetry_NotFound(t *testing.T) {
	_, delivery, _, _, _, mock := newTestDelivery(t) //nolint:dogsled
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(sqlmock.NewRows(nil))
	_, err := delivery.ManualRetry(context.Background(), uuid.New(),
		"this is a valid reason that is more than thirty characters", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryJurnalNotFound)
}

func TestManualRetry_InvalidTransition_NotFailed(t *testing.T) {
	_, delivery, _, _, _, mock := newTestDelivery(t) //nolint:dogsled
	headerID, statusID := uuid.New(), uuid.New()
	rows := mockGLStatusRows(statusID, GlHostStatusDelivered, 0)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(rows)

	_, err := delivery.ManualRetry(context.Background(), headerID,
		"this is a valid reason that is more than thirty characters", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryInvalidTransition)
}

// ─── DLQService ──────────────────────────────────────────────────────────────

func TestDLQService_Replay_ReasonTooShort(t *testing.T) {
	_, _, dlqSvc, _, _, _ := newTestDelivery(t) //nolint:dogsled
	_, err := dlqSvc.Replay(context.Background(), uuid.New(), "short", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryReasonTooShort)
}

func TestDLQService_Replay_NotFound(t *testing.T) {
	_, _, dlqSvc, _, _, mock := newTestDelivery(t) //nolint:dogsled
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(sqlmock.NewRows(nil))
	_, err := dlqSvc.Replay(context.Background(), uuid.New(),
		"valid reason that is more than thirty characters long", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryJurnalNotFound)
}

func TestDLQService_Discard_ReasonTooShort(t *testing.T) {
	_, _, dlqSvc, _, _, _ := newTestDelivery(t) //nolint:dogsled
	_, err := dlqSvc.Discard(context.Background(), uuid.New(), "short", uuid.New())
	requireDomainCode(t, err, domainerrors.CodeGLDeliveryReasonTooShort)
}

// ─── ReconciliationService ────────────────────────────────────────────────────

func TestRunReconciliation_NoMismatches(t *testing.T) {
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)

	blipsAmt := decimal.NewFromInt(5000000)
	glData := []AkunTotal{{KodeAkun: "1101", NetAmountIDR: blipsAmt}}
	stubGL := NewStubAdapter(StubConfig{SummaryAccounts: glData})
	svc := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stubGL, aw, nil, DefaultConfig(), nil)

	reportID := uuid.New()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	blipsRows := sqlmock.NewRows([]string{"id", "kode_akun", "nama_akun", "net_idr", "header_ids"}).
		AddRow(uuid.New(), "1101", "Kas", "5000000.0000", "{"+uuid.New().String()+"}")
	mock.ExpectQuery(`SELECT c\.id, c\.kode_akun`).WillReturnRows(blipsRows)

	// Outer tx wraps UPDATE + audit (no nested BEGIN for WithTx pattern).
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.RunReconciliation(context.Background(), reportID, date, "TUGURE")
	require.NoError(t, err)
}

func TestRunReconciliation_WithMismatch(t *testing.T) {
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)

	glData := []AkunTotal{{KodeAkun: "1101", NetAmountIDR: decimal.NewFromInt(3000000)}}
	stubGL := NewStubAdapter(StubConfig{SummaryAccounts: glData})

	cfg := DefaultConfig()
	cfg.ToleranceIDR = decimal.NewFromFloat(0.5)
	svc := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stubGL, aw, nil, cfg, nil)

	reportID := uuid.New()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	blipsRows := sqlmock.NewRows([]string{"id", "kode_akun", "nama_akun", "net_idr", "header_ids"}).
		AddRow(uuid.New(), "1101", "Kas", "5000000.0000", "{"+uuid.New().String()+"}")
	mock.ExpectQuery(`SELECT c\.id, c\.kode_akun`).WillReturnRows(blipsRows)

	// Outer tx wraps all writes + audit (WithTx — no nested BEGIN).
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE sys\.gl_recon_mismatch SET deleted_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO sys\.gl_recon_mismatch`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := svc.RunReconciliation(context.Background(), reportID, date, "TUGURE")
	require.NoError(t, err)
}

func TestReconciliationService_TriggerAsync_InProgress(t *testing.T) {
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()
	svc := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stub, aw, nil, DefaultConfig(), nil)

	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(1),
	)

	_, err := svc.TriggerAsync(context.Background(), time.Now(), "MANUAL", nil, "TUGURE")
	requireDomainCode(t, err, domainerrors.CodeGLReconciliationInProgress)
}

// ─── domain JSON serialization ────────────────────────────────────────────────

func TestDLQEntry_JSON_PayloadOmitted(t *testing.T) {
	e := DLQEntry{
		ID:             uuid.New(),
		JurnalHeaderID: uuid.New(),
		ErrorCode:      "TEST",
		ErrorMessage:   "test error",
		ErrorCategory:  "INFRA",
		Status:         DLQStatusFailed,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		TenantID:       "TUGURE",
	}
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	_, hasPayload := m["payloadJsonb"]
	assert.False(t, hasPayload, "payloadJsonb should be omitted when nil")
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// newTestDelivery returns (db, delivery, dlqSvc, stub, recon, mock).
func newTestDelivery(t *testing.T) (*sql.DB, *DeliveryService, *DLQService, *StubAdapter, *ReconciliationService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()

	delivery := NewDeliveryService(jurnalRepo, dlqRepo, stub, aw, nil, DefaultConfig(), nil)
	dlqSvc := NewDLQService(dlqRepo, jurnalRepo, delivery, aw, nil, nil)
	recon := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stub, aw, nil, DefaultConfig(), nil)
	return db, delivery, dlqSvc, stub, recon, mock
}

// mockGLStatusRows creates sqlmock rows for GetDeliveryStatus query.
func mockGLStatusRows(statusID uuid.UUID, status GlHostStatus, retryCount int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "gl_host_status", "gl_host_journal_id", "delivered_at",
		"retry_count", "last_retry_at", "last_error", "failure_category",
		"delivery_mode", "payload_sent_at", "gl_response_payload_jsonb",
		"manual_retry_by", "manual_retry_at", "manual_retry_reason",
		"delivery_response_id",
	}).AddRow(
		statusID, string(status), nil, nil,
		retryCount, nil, nil, nil,
		"API", nil, nil,
		nil, nil, nil,
		nil,
	)
}

// mockHeaderAndDetail sets up sqlmock for GetJurnalHeaderForDelivery (header + detail queries).
func mockHeaderAndDetail(t *testing.T, mock sqlmock.Sqlmock, headerID, statusID uuid.UUID, status GlHostStatus, retryCount int) {
	t.Helper()
	hRows := sqlmock.NewRows([]string{
		"id", "no_jurnal", "tanggal_posting", "event_code", "narrative",
		"total_debit", "total_kredit", "idempotency_key", "status_internal",
		"gs_id", "gl_host_status", "retry_count",
	}).AddRow(
		headerID, "JRN-001", time.Now(), "PENEMPATAN", "test",
		"1000000.0000", "1000000.0000", uuid.New().String(), "POSTED",
		statusID, string(status), retryCount,
	)
	mock.ExpectQuery(`SELECT h\.id, h\.no_jurnal`).WillReturnRows(hRows)

	dRows := sqlmock.NewRows([]string{
		"id", "urutan", "debit_amount", "kredit_amount", "mata_uang", "narrative_line", "kode_akun", "nama_akun",
	}).
		AddRow(uuid.New(), 1, "1000000.0000", "0.0000", "IDR", "Debit", "1101", "Kas").
		AddRow(uuid.New(), 2, "0.0000", "1000000.0000", "IDR", "Kredit", "3101", "Modal")
	mock.ExpectQuery(`SELECT d\.id, d\.urutan`).WillReturnRows(dRows)
}

// expectStatusUpdateTx sets up sqlmock expectations for updateStatusInTx.
// audit.WithTx uses the OUTER tx — no nested BEGIN.
func expectStatusUpdateTx(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}
