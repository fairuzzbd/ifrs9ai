package lps

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── DBLPSCoverageRepo tests ──────────────────────────────────────────────────

func TestDBLPSCoverageRepo_GetActiveByEvaluationDate_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	capID := uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expectedCap, _ := decimal.NewFromString("2000000000.0000")

	rows := sqlmock.NewRows([]string{
		"id", "coverage_amount", "mata_uang",
		"periode_berlaku_dari", "periode_berlaku_sampai", "workflow_status",
	}).AddRow(capID, "2000000000.0000", "IDR", from, nil, "APPROVED")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBLPSCoverageRepo(db)
	result, err := repo.GetActiveByEvaluationDate(context.Background(), time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.CoverageAmountIDR.Equal(expectedCap) {
		t.Errorf("CoverageAmountIDR = %s, want %s", result.CoverageAmountIDR, expectedCap)
	}
	if result.ID != capID {
		t.Errorf("ID = %s, want %s", result.ID, capID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBLPSCoverageRepo_GetActiveByEvaluationDate_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{
		"id", "coverage_amount", "mata_uang", "periode_berlaku_dari", "periode_berlaku_sampai", "workflow_status",
	}))

	repo := NewDBLPSCoverageRepo(db)
	result, err := repo.GetActiveByEvaluationDate(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for no rows")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBOverrideRepo.List tests ────────────────────────────────────────────────

func TestDBOverrideRepo_List_Basic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	instrID := uuid.New()
	fromID := uuid.New()
	toID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "exclusion_reason", "valid_from_periode_id", "valid_to_periode_id",
		"workflow_status", "maker_id", "approver_id", "signed_at_approve", "signature_hash_approve",
		"comment_approve", "reject_reason",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "deleted_by",
		"row_version", "tenant_id",
	}).AddRow(
		ovID, instrID, "A reason longer than 30 chars for test", fromID, toID,
		"PENDING_APPROVAL", makerID, nil, nil, nil,
		nil, nil,
		now, makerID, now, makerID, nil, nil,
		1, "TUGURE",
	)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	result, next, hasMore, err := repo.List(context.Background(), "PENDING_APPROVAL", "", "", "", "created_at", "desc", "", 50)
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
	if next != "" {
		t.Errorf("nextCursor should be empty, got %q", next)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBOverrideRepo.Create tests ─────────────────────────────────────────────

func TestDBOverrideRepo_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	newID := uuid.New()

	mock.ExpectQuery("INSERT INTO ecl.lps_exclusion_override").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "row_version"}).
			AddRow(newID, now, now, 1))

	repo := NewDBOverrideRepo(db)
	o := &LPSExclusionOverride{
		InstrumenID:        uuid.New(),
		ExclusionReason:    "Test reason that is longer than 30 characters",
		ValidFromPeriodeID: uuid.New(),
		ValidToPeriodeID:   uuid.New(),
		WorkflowStatus:     WorkflowStatusPendingApproval,
		MakerID:            uuid.New(),
		CreatedBy:          uuid.New(),
		UpdatedBy:          uuid.New(),
		TenantID:           "TUGURE",
	}

	if err := repo.Create(context.Background(), nil, o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.ID != newID {
		t.Errorf("ID = %s, want %s", o.ID, newID)
	}
	if o.RowVersion != 1 {
		t.Errorf("RowVersion = %d, want 1", o.RowVersion)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── DBOverrideRepo.Approve ───────────────────────────────────────────────────

func TestDBOverrideRepo_Approve(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	ovID := uuid.New()
	approverID := uuid.New()
	now := time.Now()
	sigHash := ComputeApproveSignatureHash(approverID, ovID, now, "approved")

	mock.ExpectExec("UPDATE ecl.lps_exclusion_override").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewDBOverrideRepo(db)
	if err := repo.Approve(context.Background(), nil, ovID, approverID, now, sigHash, "approved", approverID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBOverrideRepo_Approve_NoRowAffected(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE ecl.lps_exclusion_override").
		WillReturnResult(sqlmock.NewResult(0, 0)) // no rows

	repo := NewDBOverrideRepo(db)
	err = repo.Approve(context.Background(), nil, uuid.New(), uuid.New(), time.Now(), []byte{}, "comment", uuid.New())
	if err == nil {
		t.Fatal("expected error for no rows affected")
	}
}

// ─── DBOverrideRepo.Reject ────────────────────────────────────────────────────

func TestDBOverrideRepo_Reject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE ecl.lps_exclusion_override").
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := NewDBOverrideRepo(db)
	if err := repo.Reject(context.Background(), nil, uuid.New(), uuid.New(), "reason", uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── DBDepositoInstrumenRepo.ListAllActivePairs ───────────────────────────────

func TestDBDepositoInstrumenRepo_ListAllActivePairs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	n1 := uuid.New()
	b1 := uuid.New()

	rows := sqlmock.NewRows([]string{"nasabah_id", "bank_id"}).
		AddRow(n1, b1)
	mock.ExpectQuery("SELECT DISTINCT").WillReturnRows(rows)

	repo := NewDBDepositoInstrumenRepo(db)
	pairs, err := repo.ListAllActivePairs(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 1 {
		t.Errorf("len(pairs) = %d, want 1", len(pairs))
	}
	if pairs[0].NasabahID != n1 || pairs[0].BankID != b1 {
		t.Errorf("pair = (%s,%s), want (%s,%s)", pairs[0].NasabahID, pairs[0].BankID, n1, b1)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ─── HasActiveOrPendingForInstrumen ──────────────────────────────────────────

func TestDBOverrideRepo_HasActiveOrPending_True(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	existingID := uuid.New()
	rows := sqlmock.NewRows([]string{"id", "workflow_status"}).
		AddRow(existingID.String(), "APPROVED_ACTIVE")
	mock.ExpectQuery("SELECT id").WillReturnRows(rows)

	repo := NewDBOverrideRepo(db)
	exists, conflictID, err := repo.HasActiveOrPendingForInstrumen(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
	if conflictID != existingID.String() {
		t.Errorf("conflictID = %q, want %q", conflictID, existingID.String())
	}
}

func TestDBOverrideRepo_HasActiveOrPending_False(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT id").WillReturnRows(sqlmock.NewRows([]string{"id", "workflow_status"}))

	repo := NewDBOverrideRepo(db)
	exists, _, err := repo.HasActiveOrPendingForInstrumen(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected exists=false")
	}
}

// ─── allowedOverrideSortCols init assertion ───────────────────────────────────

func TestAllowedOverrideSortCols_AllValid(t *testing.T) {
	for _, col := range AllowedSortColsOverride {
		if !allowedOverrideSortCols[col] {
			t.Errorf("column %q in AllowedSortColsOverride not in allowedOverrideSortCols map", col)
		}
	}
}
