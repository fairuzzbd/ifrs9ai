package lps

// service_extra_test.go — additional coverage for OverrideService success paths
// and adapter types (AuditWriterAdapter, KursAdapter, DBPeriodeBukuReader).
// Uses go-sqlmock to provide a real *sql.DB so db.BeginTx is available.
//
// DEC-018: verifies audit write happens in same tx as override mutation.
// DEC-017: verifies SoD is enforced in ApproveOverride success path.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── OverrideService success paths ──────────────────────────────────────────

func TestSubmitOverride_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	fromID := uuid.New()
	toID := uuid.New()
	makerID := uuid.New()
	newID := uuid.New()
	now := time.Now()

	// HasActiveOrPendingForInstrumen is called before BeginTx.
	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "workflow_status"}))

	// Expect BeginTx → Create INSERT → Commit.
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO ecl.lps_exclusion_override").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "row_version"}).
			AddRow(newID, now, now, 1))
	mock.ExpectCommit()

	periodeRepo := &mockPeriodeRepo{
		starts: map[uuid.UUID]time.Time{fromID: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		ends:   map[uuid.UUID]time.Time{toID: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	ovRepo := newDBOverrideRepoForTest(db)
	auditW := &mockAuditWriter{}

	svc := &OverrideService{
		db:           db,
		overrideRepo: ovRepo,
		periodeRepo:  periodeRepo,
		auditWriter:  auditW,
	}

	req := SubmitOverrideRequest{
		InstrumenID:        uuid.New(),
		ExclusionReason:    "This exclusion reason is definitely longer than thirty characters",
		ValidFromPeriodeID: fromID,
		ValidToPeriodeID:   toID,
	}
	result, err := svc.Submit(context.Background(), req, makerID, "ROLE-RISK", "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != newID {
		t.Errorf("ID = %s, want %s", result.ID, newID)
	}
	// Audit should have been called.
	if len(auditW.events) != 1 {
		t.Errorf("audit events = %d, want 1", len(auditW.events))
	}
	if auditW.events[0].Action != "LPS_EXCLUSION_OVERRIDE.SUBMIT" {
		t.Errorf("audit action = %s, want LPS_EXCLUSION_OVERRIDE.SUBMIT", auditW.events[0].Action)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestApproveOverride_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	approverID := uuid.New() // different from makerID → SoD ok
	overrideID := uuid.New()
	now := time.Now()

	// DBOverrideRepo.GetByID called before BeginTx (in service validation).
	existingRows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		overrideID, uuid.New(), "Reason long enough for this test (>30)", uuid.New(), uuid.New(),
		"PENDING_APPROVAL", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(existingRows)

	// BeginTx → Approve UPDATE → Commit → GetByID re-fetch.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE ecl.lps_exclusion_override").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// Re-fetch after commit.
	updatedRows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		overrideID, uuid.New(), "Reason long enough for this test (>30)", uuid.New(), uuid.New(),
		"APPROVED_ACTIVE", makerID, approverID, now, []byte("hash"), "comment", nil,
		now, makerID, now, approverID, nil, nil, 2, "TUGURE",
	)
	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(updatedRows)

	ovRepo := newDBOverrideRepoForTest(db)
	auditW := &mockAuditWriter{}
	svc := &OverrideService{
		db:           db,
		overrideRepo: ovRepo,
		auditWriter:  auditW,
	}

	result, err := svc.ApproveOverride(context.Background(), overrideID, approverID, "ROLE-ALCO", "comment", "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.WorkflowStatus != WorkflowStatusApprovedActive {
		t.Errorf("status = %s, want APPROVED_ACTIVE", result.WorkflowStatus)
	}
	if len(auditW.events) != 1 || auditW.events[0].Action != "LPS_EXCLUSION_OVERRIDE.APPROVE" {
		t.Errorf("audit not written correctly: %+v", auditW.events)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRejectOverride_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	makerID := uuid.New()
	actorID := uuid.New()
	overrideID := uuid.New()
	now := time.Now()

	// GetByID before tx.
	existingRows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		overrideID, uuid.New(), "Rejection test reason longer than thirty chars", uuid.New(), uuid.New(),
		"PENDING_APPROVAL", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(existingRows)

	// BeginTx → Reject UPDATE → Commit → GetByID re-fetch.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE ecl.lps_exclusion_override").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// Re-fetch after commit.
	rejectedRows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		overrideID, uuid.New(), "Rejection test reason longer than thirty chars", uuid.New(), uuid.New(),
		"REJECTED", makerID, nil, nil, nil, nil, "bad override",
		now, makerID, now, actorID, nil, nil, 2, "TUGURE",
	)
	mock.ExpectQuery("SELECT .* FROM ecl.lps_exclusion_override WHERE id").
		WillReturnRows(rejectedRows)

	ovRepo := newDBOverrideRepoForTest(db)
	auditW := &mockAuditWriter{}
	svc := &OverrideService{
		db:           db,
		overrideRepo: ovRepo,
		auditWriter:  auditW,
	}

	result, err := svc.RejectOverride(context.Background(), overrideID, actorID, "ROLE-ALCO", "bad override", "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WorkflowStatus != WorkflowStatusRejected {
		t.Errorf("status = %s, want REJECTED", result.WorkflowStatus)
	}
	if len(auditW.events) != 1 || auditW.events[0].Action != "LPS_EXCLUSION_OVERRIDE.REJECT" {
		t.Errorf("audit not written: %+v", auditW.events)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListOverrides_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		ovID, uuid.New(), "Some exclusion reason longer than thirty chars", uuid.New(), uuid.New(),
		"PENDING_APPROVAL", makerID, nil, nil, nil, nil, nil,
		now, makerID, now, makerID, nil, nil, 1, "TUGURE",
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ovRepo := NewDBOverrideRepo(db)
	svc := &OverrideService{overrideRepo: ovRepo}

	result, nextCursor, hasMore, err := svc.ListOverrides(context.Background(),
		"PENDING_APPROVAL", "", "", "", "created_at", "desc", "", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len(result) = %d, want 1", len(result))
	}
	if result[0].ID != ovID {
		t.Errorf("ID = %s, want %s", result[0].ID, ovID)
	}
	if hasMore {
		t.Error("hasMore should be false")
	}
	_ = nextCursor
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── Adapter coverage ───────────────────────────────────────────────────────

func TestNewOverrideService_Constructor(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewOverrideService(db, &mockOverrideRepo{}, &mockPeriodeRepo{}, &mockAuditWriter{})
	if svc == nil {
		t.Fatal("expected non-nil OverrideService")
	}
}

func TestDBPeriodeBukuReader_Found(t *testing.T) {
	fromStr := "2026-06-01"
	toStr := "2026-06-30"
	r := NewDBPeriodeBukuReader(func(ctx context.Context, id uuid.UUID) (string, string, bool, error) {
		return fromStr, toStr, true, nil
	})
	start, end, err := r.GetStartEndDate(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start = %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end = %v, want %v", end, wantEnd)
	}
}

func TestDBPeriodeBukuReader_NotFound(t *testing.T) {
	r := NewDBPeriodeBukuReader(func(ctx context.Context, id uuid.UUID) (string, string, bool, error) {
		return "", "", false, nil
	})
	start, end, err := r.GetStartEndDate(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.IsZero() || !end.IsZero() {
		t.Error("expected zero times for not-found")
	}
}

func TestKursAdapter_Found(t *testing.T) {
	rate := "15432.12345678"
	a := NewKursAdapter(func(ctx context.Context, kode string, tanggal time.Time) (string, bool, error) {
		return rate, true, nil
	})
	got, err := a.GetByDate(context.Background(), "USD", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := decimal.NewFromString(rate)
	if !got.Equal(expected) {
		t.Errorf("rate = %s, want %s", got, expected)
	}
}

func TestKursAdapter_NotFound(t *testing.T) {
	a := NewKursAdapter(func(ctx context.Context, kode string, tanggal time.Time) (string, bool, error) {
		return "", false, nil
	})
	_, err := a.GetByDate(context.Background(), "USD", time.Now())
	if err == nil {
		t.Fatal("expected error for not found kurs")
	}
}

// ─── workflow_hook coverage ──────────────────────────────────────────────────

func TestOverrideWorkflowHook_EntityType(t *testing.T) {
	hook := NewOverrideWorkflowHook(&mockOverrideRepo{})
	if hook.EntityType() != "LPS_EXCLUSION_OVERRIDE" {
		t.Errorf("EntityType = %s, want LPS_EXCLUSION_OVERRIDE", hook.EntityType())
	}
}

func TestOverrideWorkflowHook_NilRepo(t *testing.T) {
	hook := NewOverrideWorkflowHook(nil)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: uuid.New(), NewState: "APPROVED_ACTIVE", OldState: "PENDING_APPROVAL", ActorID: uuid.New(),
	})
	if err != nil {
		t.Errorf("expected nil error for nil repo, got %v", err)
	}
}

func TestOverrideWorkflowHook_TerminalState(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(),
		WorkflowStatus: WorkflowStatusRejected,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	hook := NewOverrideWorkflowHook(ovRepo)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: ovID, NewState: "APPROVED_ACTIVE", OldState: "REJECTED", ActorID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error for transition from terminal state")
	}
}

func TestOverrideWorkflowHook_ApproveSoDViolation(t *testing.T) {
	makerID := uuid.New()
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        makerID,
		WorkflowStatus: WorkflowStatusPendingApproval,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	hook := NewOverrideWorkflowHook(ovRepo)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: ovID, NewState: "APPROVED_ACTIVE", OldState: "PENDING_APPROVAL", ActorID: makerID,
	})
	if err == nil {
		t.Fatal("expected SoD violation error")
	}
}

func TestOverrideWorkflowHook_RejectFromWrongState(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(),
		WorkflowStatus: WorkflowStatusApprovedActive,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	hook := NewOverrideWorkflowHook(ovRepo)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: ovID, NewState: "REJECTED", OldState: "APPROVED_ACTIVE", ActorID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error: APPROVED_ACTIVE is terminal")
	}
}

func TestOverrideWorkflowHook_NotFound(t *testing.T) {
	hook := NewOverrideWorkflowHook(&mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{}})
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: uuid.New(), NewState: "APPROVED_ACTIVE", OldState: "PENDING_APPROVAL", ActorID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestOverrideWorkflowHook_ApproveSuccess(t *testing.T) {
	makerID := uuid.New()
	approverID := uuid.New()
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        makerID,
		WorkflowStatus: WorkflowStatusPendingApproval,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	hook := NewOverrideWorkflowHook(ovRepo)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: ovID, NewState: "APPROVED_ACTIVE", OldState: "PENDING_APPROVAL", ActorID: approverID,
	})
	if err != nil {
		t.Errorf("expected nil error for valid approve, got %v", err)
	}
}

func TestOverrideWorkflowHook_RejectSuccess(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(),
		WorkflowStatus: WorkflowStatusPendingApproval,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	hook := NewOverrideWorkflowHook(ovRepo)
	err := hook.BeforeCommit(context.Background(), nil, workflow.HookEvent{
		EntityID: ovID, NewState: "REJECTED", OldState: "PENDING_APPROVAL", ActorID: uuid.New(),
	})
	if err != nil {
		t.Errorf("expected nil error for valid reject, got %v", err)
	}
}

// ─── newDBOverrideRepoForTest creates a real DBOverrideRepo for success path tests. ──

func newDBOverrideRepoForTest(db *sql.DB) *DBOverrideRepo {
	return NewDBOverrideRepo(db)
}

// ─── AuditWriterAdapter coverage ────────────────────────────────────────────

func TestAuditWriterAdapter_Write(t *testing.T) {
	// AuditWriterAdapter.Write must call WithTx(tx).Write(ctx, audit.Event{...}).
	// We test it with a real sqlmock tx so that audit.TxWriter.Write can execute
	// its INSERT into aud.audit_log within the tx.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	entityID := uuid.New()

	// audit.TxWriter.Write needs: fetchPreviousHash query + INSERT audit_log.
	// fetchPreviousHash: SELECT current_hash FROM aud.audit_log WHERE ...
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT current_hash").
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"})) // no previous hash
	mock.ExpectExec("INSERT INTO aud.audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	auditWriter := audit.NewWriter(db)
	adapter := NewAuditWriterAdapter(auditWriter)

	// Need to inject actor into context (audit.Write reads from claims).
	actorID := uuid.New()
	evt := AuditEvent{
		ActorUserID: actorID,
		ActorRole:   "ROLE-RISK",
		Action:      "LPS_EXCLUSION_OVERRIDE.SUBMIT",
		EntityType:  "ecl.lps_exclusion_override",
		EntityID:    entityID,
		BeforeJSON:  nil,
		AfterJSON:   map[string]string{"test": "value"},
		TenantID:    "TUGURE",
	}

	// adapter.Write calls audit.TxWriter.Write which needs actorUserID from context
	// or from AuditEvent.ActorUserID. AuditEvent.ActorUserID is passed directly.
	if err := adapter.Write(context.Background(), tx, evt); err != nil {
		t.Fatalf("adapter.Write: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── aggregatePairFromBulkRows coverage ──────────────────────────────────────

func TestAggregatePairFromBulkRows_Empty(t *testing.T) {
	svc := &AggregatorService{}
	cap := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	result, err := svc.aggregatePairFromBulkRows(nil, cap, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for empty rows")
	}
}

func TestAggregatePairFromBulkRows_FCYMissingRate(t *testing.T) {
	svc := &AggregatorService{}
	cap := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	nasabah := uuid.New()
	bank := uuid.New()
	rows := []BulkDepositoRow{
		{
			InstrumenID: uuid.New(), NasabahID: nasabah, BankID: bank,
			Nominal: decimal.NewFromInt(100), MataUang: "USD",
			FXRate: nil, // missing rate → error
		},
	}
	_, err := svc.aggregatePairFromBulkRows(rows, cap, time.Now())
	if err == nil {
		t.Fatal("expected error for missing FX rate in bulk rows")
	}
}

func TestAggregatePairFromBulkRows_WithOverride(t *testing.T) {
	svc := &AggregatorService{}
	cap := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	nasabah := uuid.New()
	bank := uuid.New()
	instrID := uuid.New()
	ovID := uuid.New()
	reason := "Override reason that is at least thirty chars long"
	rows := []BulkDepositoRow{
		{
			InstrumenID: instrID, NasabahID: nasabah, BankID: bank,
			Nominal: decimal.NewFromInt(500_000_000), MataUang: "IDR",
			KlasifikasiPsak71: "AC", TenantID: "TUGURE",
			OverrideID: &ovID, ExclusionReason: &reason,
		},
	}
	result, err := svc.aggregatePairFromBulkRows(rows, cap, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Breakdown) != 1 {
		t.Fatalf("breakdown len = %d, want 1", len(result.Breakdown))
	}
	b := result.Breakdown[0]
	// Override → excluded, full EAD in excess.
	if !b.LPSExcluded {
		t.Error("expected LPSExcluded=true for override instrument")
	}
	if !b.AllocatedToExcess.Equal(decimal.NewFromInt(500_000_000)) {
		t.Errorf("excess = %s, want 500000000", b.AllocatedToExcess)
	}
	if !b.AllocatedToCovered.Equal(decimal.Zero) {
		t.Errorf("covered = %s, want 0", b.AllocatedToCovered)
	}
}
