package jurnal

// service_integration_test.go — service-level tests backed by sqlmock.
//
// These tests exercise service methods that hit the DB, using sqlmock to
// simulate DB responses WITHOUT a live PostgreSQL instance.
// Strategy: mock the minimum rows needed to reach specific guard conditions.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── sqlmock setup helpers ─────────────────────────────────────────────────────

func newMockDB(t *testing.T) (*MappingRepo, *JurnalRepo, *DLQRepo, *audit.Writer, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)
	return NewMappingRepo(db), NewJurnalRepo(db), NewDLQRepo(db), audit.NewWriter(db), mock
}

// ─── MappingService.Submit ─────────────────────────────────────────────────────

// TestMappingService_Submit_NotFound tests Submit when the header doesn't exist.
func TestMappingService_Submit_NotFound(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	// Mock GetByID returning no rows.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	_, err := svc.Submit(ctx, uuid.New(), uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	if ok {
		assert.Equal(t, domainerrors.CodeJurnalHeaderNotFound, de.Code())
	}
}

// TestMappingService_Submit_WrongStatus tests Submit when status is not DRAFT.
func TestMappingService_Submit_WrongStatus(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRow(headerID, MappingStatusPendingReview))

	ctx := context.Background()
	_, err := svc.Submit(ctx, headerID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "must be domain error")
	assert.Equal(t, domainerrors.CodeJurnalInvalidTransition, de.Code())
}

// TestMappingService_Review_SoDViolation: reviewer == maker → SoD error.
func TestMappingService_Review_SoDViolation(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	makerID := uuid.New()
	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRowWithMaker(headerID, MappingStatusPendingReview, &makerID))

	ctx := context.Background()
	_, err := svc.Review(ctx, headerID, WorkflowSigningRequest{
		Comment: "lgtm", SignatureMethod: "JWT_STEP_UP",
	}, makerID) // same as maker → SoD violation
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalSoDViolation, de.Code())
}

// TestMappingService_Approve_SoDViolation_MakerIsApprover.
func TestMappingService_Approve_SoDViolation_MakerIsApprover(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	makerID := uuid.New()
	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRowWithMaker(headerID, MappingStatusPendingApproval, &makerID))

	claims := freshClaims(makerID)
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.Approve(ctx, headerID, WorkflowSigningRequest{
		Comment: "approved", SignatureMethod: "JWT_STEP_UP",
	}, makerID, claims) // makerID == approverID → SoD
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalSoDViolation, de.Code())
}

// TestMappingService_Approve_StepUpRequired for regulated event codes.
func TestMappingService_Approve_StepUpRequired(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	makerID := uuid.New()
	approverID := uuid.New()
	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRowRegulated(headerID, MappingStatusPendingApproval, &makerID, EventCodeECLPembentukan))

	// Claims without step-up → NeedsStepUp() = true.
	claims := &auth.Claims{
		Sub: approverID.String(), TenantID: "TUGURE",
		StepupVerifiedAt: nil, // no step-up
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.Approve(ctx, headerID, WorkflowSigningRequest{
		Comment: "approve", SignatureMethod: "JWT_STEP_UP",
	}, approverID, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalStepUpRequired, de.Code())
}

// TestMappingService_Approve2_StepUpRequired: always needs step-up.
func TestMappingService_Approve2_StepUpRequired(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	makerID := uuid.New()
	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRowWithMaker(headerID, MappingStatusPendingApproval2, &makerID))

	approver2ID := uuid.New()
	claims := &auth.Claims{
		Sub: approver2ID.String(), TenantID: "TUGURE",
		StepupVerifiedAt: nil, // NeedsStepUp() = true
	}
	ctx := auth.ContextWithClaims(context.Background(), claims)
	_, err := svc.Approve2(ctx, headerID, WorkflowSigningRequest{
		Comment: "approve2", SignatureMethod: "JWT_STEP_UP",
	}, approver2ID, claims)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalStepUpRequired, de.Code())
}

// TestMappingService_Reject_WrongStatus.
func TestMappingService_Reject_WrongStatus(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRow(headerID, MappingStatusDraft))

	ctx := context.Background()
	_, err := svc.Reject(ctx, headerID, WorkflowRejectRequest{
		RejectReason:    "reason that is definitely 30+ characters long",
		SignatureMethod: "JWT_STEP_UP",
	}, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalInvalidTransition, de.Code())
}

// TestMappingService_Deactivate_WrongStatus.
func TestMappingService_Deactivate_WrongStatus(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRow(headerID, MappingStatusDraft))

	_, err := svc.Deactivate(context.Background(), headerID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalInvalidTransition, de.Code())
}

// TestMappingService_Withdraw_WrongStatus.
func TestMappingService_Withdraw_WrongStatus(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	headerID := uuid.New()
	expectGetByID(mock, buildMappingHeaderRow(headerID, MappingStatusPendingReview)) // not DRAFT

	err := svc.Withdraw(context.Background(), headerID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalInvalidTransition, de.Code())
}

// ─── ResolverService.Resolve ───────────────────────────────────────────────────

// TestResolverService_Resolve_NoMapping: no mapping found → EventNotMapped error.
func TestResolverService_Resolve_NoMapping(t *testing.T) {
	mappingRepo, _, _, _, mock := newMockDB(t)
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { _ = db.Close() })

	svc := NewResolverService(mappingRepo, db, nil)

	// GetByEventCode returns no rows.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := svc.Resolve(context.Background(), ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		PeriodeID:         uuid.New(),
		AmountIDR:         decimal.NewFromFloat(1000.0),
		SourceEventID:     uuid.New(),
		SourceEventType:   "test",
	})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalEventNotMapped, de.Code())
}

// TestResolverService_Resolve_MappingNotActive: GetByEventCode returns nil (no APPROVED_ACTIVE) → EventNotMapped.
// Note: GetByEventCode only returns rows with workflow_status IN ('APPROVED_ACTIVE','APPROVED') AND aktif_flag=true,
// so a DRAFT mapping simply returns no rows → EventNotMapped (not MappingWorkflowGate).
func TestResolverService_Resolve_MappingNotActive(t *testing.T) {
	mappingRepo, _, _, _, mock := newMockDB(t)
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { _ = db.Close() })
	svc := NewResolverService(mappingRepo, db, nil)

	// GetByEventCode returns no rows (WHERE clause filters to APPROVED_ACTIVE only).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "event_id_kode", "event_code", "nama_event", "kategori_event",
			"trigger_source", "klasifikasi_berlaku", "aktif_flag", "workflow_status", "workflow_path",
		}))

	_, err := svc.Resolve(context.Background(), ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		PeriodeID:         uuid.New(),
		AmountIDR:         decimal.NewFromFloat(1000.0),
		SourceEventID:     uuid.New(),
		SourceEventType:   "test",
	})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalEventNotMapped, de.Code())
}

// ─── PostingService.PostResolved ──────────────────────────────────────────────

// TestPostingService_PostResolved_Idempotent: same key already posted → return existing ID.
func TestPostingService_PostResolved_Idempotent(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)

	existingID := uuid.New()
	// CheckIdempotency returns an existing ID → idempotency replay.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM jrnl.header WHERE idempotency_key`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))

	srcID := uuid.New()
	returned, err := postingSvc.PostResolved(context.Background(), ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		PeriodeID:         uuid.New(),
		AmountIDR:         decimal.NewFromFloat(100.0),
		Currency:          "IDR",
		SourceEventID:     srcID,
		SourceEventType:   "penempatan:approved",
	})
	require.NoError(t, err)
	assert.Equal(t, existingID, returned, "must return existing ID on idempotent call")
}

// TestPostingService_PostResolved_PeriodeHardClosed.
func TestPostingService_PostResolved_PeriodeHardClosed(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)

	// CheckIdempotency: not found.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM jrnl.header WHERE idempotency_key`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// IsPeriodeHardClosed: return hard-closed.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("HARD_CLOSED"))

	periodeID := uuid.New()
	_, err := postingSvc.PostResolved(context.Background(), ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		PeriodeID:         periodeID,
		AmountIDR:         decimal.NewFromFloat(100.0),
		Currency:          "IDR",
		SourceEventID:     uuid.New(),
		SourceEventType:   "penempatan:approved",
	})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalPeriodeHardClosed, de.Code())
}

// ─── DLQService.Replay ─────────────────────────────────────────────────────────

// TestDLQService_Replay_NotFound.
func TestDLQService_Replay_NotFound(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	// GetByID returns no rows.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := dlqSvc.Replay(context.Background(), uuid.New(), uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqNotFound, de.Code())
}

// TestDLQService_Replay_AlreadyReplayed.
func TestDLQService_Replay_AlreadyReplayed(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	rows := buildDLQRow(dlqID, DLQStatusReplayedOK)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	_, err := dlqSvc.Replay(context.Background(), dlqID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqAlreadyReplayed, de.Code())
}

// TestDLQService_Replay_AlreadyReplaying.
func TestDLQService_Replay_AlreadyReplaying(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	rows := buildDLQRow(dlqID, DLQStatusReplaying)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	_, err := dlqSvc.Replay(context.Background(), dlqID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqAlreadyReplayed, de.Code())
}

// TestDLQService_Replay_Abandoned.
func TestDLQService_Replay_Abandoned(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	rows := buildDLQRow(dlqID, DLQStatusAbandoned)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	_, err := dlqSvc.Replay(context.Background(), dlqID, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqAlreadyReplayed, de.Code())
}

// TestDLQService_Discard_NotFound.
func TestDLQService_Discard_NotFound(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	// GetByID returns no rows.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := dlqSvc.Discard(context.Background(), uuid.New(), DLQDiscardRequest{
		DiscardReason: "This reason is definitely at least 30 characters long.",
	}, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqNotFound, de.Code())
}

// TestDLQService_Discard_ShortReason_DomainError: short reason rejected before DB.
func TestDLQService_Discard_ShortReason_DomainError(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, _ := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	// No DB call expected — reason length check fires first.
	err := dlqSvc.Discard(context.Background(), uuid.New(), DLQDiscardRequest{
		DiscardReason: "too short",
	}, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqDiscardReasonTooShort, de.Code())
}

// TestDLQService_Discard_AlreadyReplayedOK.
func TestDLQService_Discard_AlreadyReplayedOK(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	rows := buildDLQRow(dlqID, DLQStatusReplayedOK)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(rows)

	err := dlqSvc.Discard(context.Background(), dlqID, DLQDiscardRequest{
		DiscardReason: "This reason is definitely at least 30 characters long.",
	}, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqAlreadyReplayed, de.Code())
}

// ─── DLQService.Replay — audit-in-tx contract (F2 DEC-018) ──────────────────

// TestDLQService_Replay_AuditInTx_OnPostingSuccess verifies that after PostResolved
// succeeds, the status update (REPLAYED_OK) and audit row (JURNAL.DLQ_REPLAYED) are
// committed atomically in a single new transaction.  An audit write failure must cause
// rollback of the status update as well (no orphaned REPLAYED_OK without audit trail).
func TestDLQService_Replay_AuditInTx_OnPostingSuccess(t *testing.T) {
	_, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)

	mappingDB, mappingMock, merr := sqlmock.New()
	require.NoError(t, merr)
	t.Cleanup(func() { _ = mappingDB.Close() })

	mappingRepo := NewMappingRepo(mappingDB)
	resolverSvc := NewResolverService(mappingRepo, mappingDB, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	callerID := uuid.New()
	periodeID := uuid.New()

	// Build DLQ entry in FAILED state with a valid payload.
	goodPayload := map[string]any{
		"eventCode":         EventCodePenempatan,
		"klasifikasiPSAK71": "AC",
		"periodeId":         periodeID.String(),
		"sourceEventId":     uuid.New().String(),
		"amountIDR":         "1000000.0000",
		"currency":          "IDR",
		"fxRate":            "1",
		"sourceEventType":   "penempatan:approved",
	}
	payloadJSON, mErr := json.Marshal(goodPayload)
	require.NoError(t, mErr)

	now := time.Now()
	dlqRows := sqlmock.NewRows([]string{
		"id", "source_event_id", "source_event_type", "event_code",
		"instrumen_id", "periode_id", "payload_jsonb",
		"error_code", "error_message", "error_category",
		"retry_count", "last_retry_at", "status",
		"replayed_by", "replayed_at", "final_jurnal_header_id",
		"discarded_reason", "discarded_by", "discarded_at",
		"created_at", "updated_at", "row_version",
	}).AddRow(
		dlqID, uuid.New(), "penempatan:approved", EventCodePenempatan,
		nil, periodeID, payloadJSON,
		"INFRA_DB_TIMEOUT", "db timeout", "INFRA",
		1, nil, string(DLQStatusFailed),
		nil, nil, nil,
		nil, nil, nil,
		now, now, int64(1),
	)

	// GetByID
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, source_event_id`)).WillReturnRows(dlqRows)
	// IsPeriodeHardClosed (DLQ Replay pre-flight)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))
	// Mark REPLAYING
	mock.ExpectExec(`UPDATE sys.dlq_jurnal_post`).WillReturnResult(sqlmock.NewResult(1, 1))

	// PostResolved internals: idempotency check, periode check, resolver, insert.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM jrnl.header WHERE idempotency_key`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))
	// Resolver: GetByEventCode + detail rows.
	mappingMock.ExpectQuery(regexp.QuoteMeta(`SELECT id`)).
		WillReturnRows(buildEventCodeRow(uuid.New()))
	mappingMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(detailWithDKRows(uuid.New()))
	// PostResolved tx: Begin, NextNoJurnal, Insert header, Insert detail, Audit, Commit.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT nextval`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(42)))
	mock.ExpectExec(`INSERT INTO jrnl.header`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO jrnl.detail`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO jrnl.detail`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// F2 fix: status update REPLAYED_OK + audit JURNAL.DLQ_REPLAYED in ONE new tx.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.dlq_jurnal_post`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: callerID.String(), TenantID: "TUGURE",
	})
	result, err := dlqSvc.Replay(ctx, dlqID, callerID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, dlqID, result.DLQId)
}

// TestDLQService_Replay_AuditFailRollsBackStatusUpdate verifies that if the
// audit write fails after PostResolved succeeds, the REPLAYED_OK status update is
// also rolled back (atomicity guarantee of the F2 fix).
func TestDLQService_Replay_AuditFailRollsBackStatusUpdate(t *testing.T) {
	_, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)

	mappingDB, mappingMock, merr := sqlmock.New()
	require.NoError(t, merr)
	t.Cleanup(func() { _ = mappingDB.Close() })

	mappingRepo := NewMappingRepo(mappingDB)
	resolverSvc := NewResolverService(mappingRepo, mappingDB, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	dlqID := uuid.New()
	callerID := uuid.New()
	periodeID := uuid.New()

	goodPayload := map[string]any{
		"eventCode":         EventCodePenempatan,
		"klasifikasiPSAK71": "AC",
		"periodeId":         periodeID.String(),
		"sourceEventId":     uuid.New().String(),
		"amountIDR":         "1000000.0000",
		"currency":          "IDR",
		"fxRate":            "1",
		"sourceEventType":   "penempatan:approved",
	}
	payloadJSON, mErr := json.Marshal(goodPayload)
	require.NoError(t, mErr)

	now := time.Now()
	dlqRows := sqlmock.NewRows([]string{
		"id", "source_event_id", "source_event_type", "event_code",
		"instrumen_id", "periode_id", "payload_jsonb",
		"error_code", "error_message", "error_category",
		"retry_count", "last_retry_at", "status",
		"replayed_by", "replayed_at", "final_jurnal_header_id",
		"discarded_reason", "discarded_by", "discarded_at",
		"created_at", "updated_at", "row_version",
	}).AddRow(
		dlqID, uuid.New(), "penempatan:approved", EventCodePenempatan,
		nil, periodeID, payloadJSON,
		"INFRA_DB_TIMEOUT", "db timeout", "INFRA",
		1, nil, string(DLQStatusFailed),
		nil, nil, nil,
		nil, nil, nil,
		now, now, int64(1),
	)

	// GetByID + period check + mark REPLAYING.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, source_event_id`)).WillReturnRows(dlqRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))
	mock.ExpectExec(`UPDATE sys.dlq_jurnal_post`).WillReturnResult(sqlmock.NewResult(1, 1))

	// PostResolved internals.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM jrnl.header WHERE idempotency_key`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status`)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))
	mappingMock.ExpectQuery(regexp.QuoteMeta(`SELECT id`)).
		WillReturnRows(buildEventCodeRow(uuid.New()))
	mappingMock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).
		WillReturnRows(detailWithDKRows(uuid.New()))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT nextval`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(43)))
	mock.ExpectExec(`INSERT INTO jrnl.header`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO jrnl.detail`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO jrnl.detail`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// F2 fix: new tx for REPLAYED_OK — audit write fails → rollback.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys.dlq_jurnal_post`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnError(fmt.Errorf("audit_node_down"))
	mock.ExpectRollback()

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: callerID.String(), TenantID: "TUGURE",
	})
	_, err := dlqSvc.Replay(ctx, dlqID, callerID)
	// Must return error because audit write failed.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit_node_down")
}

// ─── MappingService.Create ─────────────────────────────────────────────────────

// TestMappingService_Create_Success — happy path with full audit-in-tx.
func TestMappingService_Create_Success(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	callerID := uuid.New()
	// Expect Begin, INSERT mapping_jurnal_header, INSERT detail rows, INSERT audit_log, Commit.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_header`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_detail`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_detail`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO aud.audit_log`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	akunID1 := uuid.New()
	akunID2 := uuid.New()
	req := MappingHeaderCreateRequest{
		EventCode:     EventCodePenempatan,
		NamaEvent:     "Penempatan Deposito",
		KategoriEvent: "ASSET",
		TriggerSource: "SYSTEM_JOB",
		DetailRows: []MappingDetailRowInput{
			{Urutan: 1, KodeAkunID: akunID1, DKIndicator: "DEBIT", SumberAmount: "AMOUNT"},
			{Urutan: 2, KodeAkunID: akunID2, DKIndicator: "KREDIT", SumberAmount: "AMOUNT"},
		},
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: callerID.String(), TenantID: "TUGURE",
	})
	result, err := svc.Create(ctx, req, callerID)
	require.NoError(t, err)
	assert.Equal(t, MappingStatusDraft, result.WorkflowStatus)
	assert.Equal(t, EventCodePenempatan, result.EventCode)
	assert.Equal(t, WorkflowPath4Eyes, result.WorkflowPath)
	assert.Equal(t, 2, len(result.DetailRows))
}

// TestMappingService_Create_RegulatedSetsWorkflow6Eyes.
func TestMappingService_Create_RegulatedSetsWorkflow6Eyes(t *testing.T) {
	mappingRepo, _, _, aw, mock := newMockDB(t)
	svc := NewMappingService(mappingRepo, aw, nil)

	callerID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_header`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_detail`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO mst.mapping_jurnal_detail`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO aud.audit_log`)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	req := MappingHeaderCreateRequest{
		EventCode:     EventCodeECLPembentukan, // regulated → 6-eyes
		NamaEvent:     "ECL Pembentukan",
		KategoriEvent: "ECL",
		TriggerSource: "SYSTEM_JOB",
		DetailRows: []MappingDetailRowInput{
			{Urutan: 1, KodeAkunID: uuid.New(), DKIndicator: "DEBIT", SumberAmount: "AMOUNT"},
			{Urutan: 2, KodeAkunID: uuid.New(), DKIndicator: "KREDIT", SumberAmount: "AMOUNT"},
		},
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: callerID.String(), TenantID: "TUGURE"})
	result, err := svc.Create(ctx, req, callerID)
	require.NoError(t, err)
	assert.Equal(t, WorkflowPath6Eyes, result.WorkflowPath, "regulated code must use 6-eyes workflow")
}

// ─── PostingService.CreateManualDraft — guard: non-manual event code ──────────

func TestPostingService_CreateManualDraft_InvalidEventCode(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, _ := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)

	_, err := postingSvc.CreateManualDraft(context.Background(), ManualPostRequest{
		EventCode: EventCodePenempatan, // NOT in manualAllowedEventCodes
		PeriodeID: uuid.New(),
		AmountIDR: decimal.NewFromFloat(1000.0),
		Narasi:    "Test narasi",
	}, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalInvalidTransition, de.Code())
}

// TestPostingService_ApproveManual_SoDViolation.
func TestPostingService_ApproveManual_SoDViolation(t *testing.T) {
	mappingRepo, jurnalRepo, dlqRepo, aw, mock := newMockDB(t)
	resolverSvc := NewResolverService(mappingRepo, jurnalRepo.db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)

	headerID := uuid.New()
	makerID := uuid.New()
	// JurnalRepo.GetByID issues TWO queries: jrnl.header then jrnl.detail.
	// Both must be mocked or the second query fails with "unexpected call".
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(buildJurnalHeaderRow(headerID, JurnalStatusDraftManual))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(emptyJurnalDetailRows())

	// makerID == callerID → SoD violation.
	_, err := postingSvc.ApproveManual(context.Background(), headerID, makerID, makerID)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalSoDViolation, de.Code())
}

// ─── sqlmock row builder helpers ──────────────────────────────────────────────

// detailCols matches MappingRepo.listDetails SELECT (12 columns).
var detailCols = []string{
	"id", "event_header_id", "urutan", "kode_akun_id",
	"kode_akun_kode", "kode_akun_nama",
	"dk_indicator", "sumber_amount", "klasifikasi_filter",
	"multiplier", "catatan", "aktif_flag",
}

// emptyDetailRows returns an empty mock rows for listDetails.
func emptyDetailRows() *sqlmock.Rows {
	return sqlmock.NewRows(detailCols)
}

// expectGetByID sets up the two-query expectation for MappingRepo.GetByID:
// 1) SELECT mapping_jurnal_header
// 2) SELECT mapping_jurnal_detail (listDetails follow-up)
func expectGetByID(mock sqlmock.Sqlmock, headerRows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(headerRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT`)).WillReturnRows(emptyDetailRows())
}

// mappingHeaderCols matches MappingRepo.GetByID SELECT (29 columns, no signature hashes, no deleted_at).
var mappingHeaderCols = []string{
	"id", "event_id_kode", "event_code", "nama_event", "kategori_event",
	"trigger_source", "klasifikasi_berlaku", "aktif_flag", "workflow_status",
	"workflow_path", "deskripsi",
	"maker_id", "reviewer_id", "approver_id", "approver_2_id",
	"reviewer_signed_at", "approver_signed_at", "approver_2_signed_at",
	"comment_review", "comment_approve", "comment_approve_2",
	"submit_at", "reject_reason",
	"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
}

func buildMappingHeaderRow(id uuid.UUID, status MappingHeaderStatus) *sqlmock.Rows {
	now := time.Now()
	klasJSON, _ := json.Marshal([]string{})
	cb := uuid.New()
	return sqlmock.NewRows(mappingHeaderCols).AddRow(
		id, "EVT-TEST", EventCodePenempatan, "Test Event", "ASSET",
		"SYSTEM_JOB", klasJSON, false, string(status),
		string(WorkflowPath4Eyes), nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		now, cb, now, cb, int64(1), "TUGURE",
	)
}

func buildMappingHeaderRowWithMaker(id uuid.UUID, status MappingHeaderStatus, makerID *uuid.UUID) *sqlmock.Rows {
	now := time.Now()
	klasJSON, _ := json.Marshal([]string{})
	cb := uuid.New()
	return sqlmock.NewRows(mappingHeaderCols).AddRow(
		id, "EVT-TEST", EventCodePenempatan, "Test Event", "ASSET",
		"SYSTEM_JOB", klasJSON, false, string(status),
		string(WorkflowPath4Eyes), nil,
		makerID, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		now, cb, now, cb, int64(1), "TUGURE",
	)
}

func buildMappingHeaderRowRegulated(id uuid.UUID, status MappingHeaderStatus, makerID *uuid.UUID, eventCode string) *sqlmock.Rows {
	now := time.Now()
	klasJSON, _ := json.Marshal([]string{})
	cb := uuid.New()
	return sqlmock.NewRows(mappingHeaderCols).AddRow(
		id, "EVT-TEST", eventCode, "Regulated Event", "ECL",
		"SYSTEM_JOB", klasJSON, false, string(status),
		string(WorkflowPath6Eyes), nil,
		makerID, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		now, cb, now, cb, int64(1), "TUGURE",
	)
}

func buildDLQRow(id uuid.UUID, status DLQStatus) *sqlmock.Rows {
	now := time.Now()
	periodeID := uuid.New()
	payload := DLQPostPayload{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		PeriodeID:         periodeID.String(),
		AmountIDR:         decimal.NewFromFloat(1000.0),
		Currency:          "IDR",
		FxRate:            decimal.NewFromInt(1),
		SourceEventID:     uuid.New().String(),
		SourceEventType:   "penempatan:approved",
	}
	payloadJSON, _ := json.Marshal(payload)
	return sqlmock.NewRows([]string{
		"id", "source_event_id", "source_event_type", "event_code",
		"instrumen_id", "periode_id", "payload_jsonb",
		"error_code", "error_message", "error_category",
		"retry_count", "last_retry_at", "status",
		"replayed_by", "replayed_at", "final_jurnal_header_id",
		"discarded_reason", "discarded_by", "discarded_at",
		"created_at", "updated_at", "row_version",
	}).AddRow(
		id, uuid.New(), "penempatan:approved", EventCodePenempatan,
		nil, periodeID, payloadJSON,
		"TEST_ERROR", "test error message", "DOMAIN",
		0, nil, string(status),
		nil, nil, nil,
		nil, nil, nil,
		now, now, int64(1),
	)
}

// jurnalDetailCols matches JurnalRepo.listDetails SELECT (11 columns).
var jurnalDetailCols = []string{
	"id", "header_id", "urutan", "kode_akun_id",
	"kode_akun_kode", "kode_akun_nama",
	"debit_amount", "kredit_amount", "mata_uang",
	"narrative_line", "created_at",
}

// emptyJurnalDetailRows returns empty mock rows for JurnalRepo.listDetails.
func emptyJurnalDetailRows() *sqlmock.Rows {
	return sqlmock.NewRows(jurnalDetailCols)
}

func buildJurnalHeaderRow(id uuid.UUID, status JurnalHeaderStatus) *sqlmock.Rows {
	now := time.Now()
	periodeID := uuid.New()
	return sqlmock.NewRows([]string{
		"id", "no_jurnal", "tanggal_posting", "periode_id",
		"event_code", "mapping_header_id", "instrumen_id",
		"reference_event_type", "reference_event_id",
		"currency", "total_debit", "total_kredit",
		"narrative", "status_internal", "idempotency_key",
		"dokumen_doc_id", "created_by", "created_at",
	}).AddRow(
		id, "JRN-2026-000001", now, periodeID,
		EventCodePeriodeAdjustment, nil, nil,
		"MANUAL_POST", nil,
		"IDR", "100.0000", "100.0000",
		"Test narasi", string(status), "test-idmpkey",
		nil, uuid.New(), now,
	)
}

func freshClaims(userID uuid.UUID) *auth.Claims {
	ts := time.Now().Unix()
	return &auth.Claims{
		Sub:              userID.String(),
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &ts, // fresh step-up
	}
}
