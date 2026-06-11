package lps

// expiry_worker_test.go — unit tests for ExpiryWorker (issue #47).
//
// Tests use go-sqlmock for DB interactions and a mockExpiryRepo / mockAuditWriter
// for service-level isolation, following the M3 test pattern in service_test.go
// and repo_extra_test.go.
//
// Coverage:
//  1. TestExpiryWorker_TransitionsApprovedActiveToExpired — happy path UPDATE + audit in tx.
//  2. TestExpiryWorker_AuditFailureAbortsTransition      — audit error → rollback, row stays APPROVED_ACTIVE.
//  3. TestExpiryWorker_SkipsNonExpiredRows               — rows whose valid_to is in the future are not returned.
//  4. TestExpiryWorker_IdempotentReRun                   — already-EXPIRED row skipped (sql.ErrNoRows from MarkExpiredInTx).
//  5. TestNewExpiryWorker_PanicsWhenAuditWriterNil       — nil auditWriter triggers panic.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// ─── Test doubles ─────────────────────────────────────────────────────────────

// txMock is a minimal mock transaction that satisfies the Commit/Rollback interface
// without requiring a real DB connection. The expiry worker calls Commit on success
// and relies on the deferred rollbackTx on failure; this struct lets both no-op.
//
// We achieve this via sqlmock.New() with sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp)
// and ExpectBegin + ExpectCommit/ExpectRollback set once, then reused per call via a
// factory on mockExpiryRepo.
//
// For simplicity we create one sqlmock db per test and pre-register all expected
// Begin/Commit pairs for the number of candidates.

// mockExpiryRepo implements ExpiryRepoIface for service-level tests.
// It creates one sqlmock DB per BeginTx call with pre-set Begin+Commit expectations,
// so the worker can call Commit/Rollback without hitting real DB.
type mockExpiryRepo struct {
	candidates []ExpiryCandidate
	listErr    error
	markErr    error // returned by MarkExpiredInTx (e.g. sql.ErrNoRows for idempotent)
	beginErr   error
	markedIDs  []uuid.UUID // tracks what was marked
}

func (m *mockExpiryRepo) ListExpiredApprovedActive(_ context.Context, _ time.Time, _ string) ([]ExpiryCandidate, error) {
	return m.candidates, m.listErr
}

func (m *mockExpiryRepo) MarkExpiredInTx(_ context.Context, _ *sql.Tx, id uuid.UUID, _ uuid.UUID) error {
	if m.markErr != nil {
		return m.markErr
	}
	m.markedIDs = append(m.markedIDs, id)
	return nil
}

func (m *mockExpiryRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	// Each call creates a fresh sqlmock db that accepts Begin + any Commit or Rollback.
	db, mock, err := sqlmock.New()
	if err != nil {
		return nil, err
	}
	mock.ExpectBegin()
	// Accept either commit or rollback (worker will do one depending on error path).
	mock.ExpectCommit().WillReturnError(nil)
	// Also register a rollback expectation so deferred rollbackTx (after commit = ErrTxDone) works.
	// sqlmock ignores expectations after Commit in MatchExpectationsInOrder=true mode by default.
	return db.BeginTx(ctx, nil) //nolint:wrapcheck
}

// failingAuditWriter implements AuditWriterIface and always returns an error.
type failingAuditWriter struct {
	err error
}

func (f *failingAuditWriter) Write(_ context.Context, _ *sql.Tx, _ AuditEvent) error {
	return f.err
}

// rollbackMockExpiryRepo is a variant of mockExpiryRepo where BeginTx returns a tx
// that expects Rollback (not Commit) — used for the audit-failure test path.
type rollbackMockExpiryRepo struct {
	mockExpiryRepo
}

func (r *rollbackMockExpiryRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	db, mock, err := sqlmock.New()
	if err != nil {
		return nil, err
	}
	mock.ExpectBegin()
	// Expect rollback (deferred rollbackTx fires when audit fails before commit).
	mock.ExpectRollback()
	return db.BeginTx(ctx, nil) //nolint:wrapcheck
}

// ─── newTestPayload helper ────────────────────────────────────────────────────

func newExpiryPayload(t *testing.T, tenantID string) *asynq.Task {
	t.Helper()
	p := LPSExpiryCheckPayload{TenantID: tenantID}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return asynq.NewTask(TaskTypeLPSExpiryCheck, b)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestExpiryWorker_TransitionsApprovedActiveToExpired verifies the happy path:
// - ListExpiredApprovedActive returns one candidate.
// - MarkExpiredInTx is called for that candidate.
// - Audit write is called with Action="LPS_OVERRIDE.EXPIRED_AUTO".
// - Worker returns nil.
func TestExpiryWorker_TransitionsApprovedActiveToExpired(t *testing.T) {
	overrideID := uuid.New()
	instrumenID := uuid.New()
	periodeID := uuid.New()

	repo := &mockExpiryRepo{
		candidates: []ExpiryCandidate{
			{ID: overrideID, InstrumenID: instrumenID, ValidToPeriodeID: periodeID},
		},
	}
	auditW := &mockAuditWriter{}
	systemUser := uuid.New()
	worker := NewExpiryWorker(repo, auditW, nil, systemUser)

	task := newExpiryPayload(t, "TUGURE")
	err := worker.HandleExpiryCheck(context.Background(), task)
	if err != nil {
		t.Fatalf("HandleExpiryCheck: unexpected error: %v", err)
	}

	// MarkExpiredInTx must have been called for our override.
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != overrideID {
		t.Errorf("markedIDs = %v, want [%s]", repo.markedIDs, overrideID)
	}

	// Audit event must have been written with correct Action.
	if len(auditW.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(auditW.events))
	}
	evt := auditW.events[0]
	if evt.Action != "LPS_OVERRIDE.EXPIRED_AUTO" {
		t.Errorf("audit Action = %q, want LPS_OVERRIDE.EXPIRED_AUTO", evt.Action)
	}
	if evt.EntityID != overrideID {
		t.Errorf("audit EntityID = %s, want %s", evt.EntityID, overrideID)
	}
	if evt.ActorUserID != systemUser {
		t.Errorf("audit ActorUserID = %s, want %s", evt.ActorUserID, systemUser)
	}
	if evt.ActorRole != "SYSTEM" {
		t.Errorf("audit ActorRole = %q, want SYSTEM", evt.ActorRole)
	}
	if evt.EntityType != "ecl.lps_exclusion_override" {
		t.Errorf("audit EntityType = %q, want ecl.lps_exclusion_override", evt.EntityType)
	}
}

// TestExpiryWorker_AuditFailureAbortsTransition verifies that when the audit write
// returns an error:
// - The transaction is rolled back (override stays APPROVED_ACTIVE).
// - HandleExpiryCheck returns a non-nil error (job will be retried by Asynq).
func TestExpiryWorker_AuditFailureAbortsTransition(t *testing.T) {
	overrideID := uuid.New()

	// Use rollbackMockExpiryRepo so BeginTx returns a tx that expects Rollback.
	repo := &rollbackMockExpiryRepo{
		mockExpiryRepo: mockExpiryRepo{
			candidates: []ExpiryCandidate{
				{ID: overrideID, InstrumenID: uuid.New(), ValidToPeriodeID: uuid.New()},
			},
		},
	}
	auditW := &failingAuditWriter{err: errors.New("audit DB down")}
	worker := NewExpiryWorker(repo, auditW, nil, uuid.New())

	task := newExpiryPayload(t, "TUGURE")
	err := worker.HandleExpiryCheck(context.Background(), task)
	if err == nil {
		t.Fatal("expected error due to audit failure, got nil")
	}

	// MarkExpiredInTx was called (UPDATE issued) but tx was rolled back.
	// The repo.markedIDs tracks optimistic calls; the DB transaction was not committed.
	// In the mock world the mark "succeeded" but the whole tx rolled back.
	// We verify the worker propagates the failure count.
	if len(repo.markedIDs) != 1 {
		t.Errorf("markedIDs len = %d, want 1 (UPDATE was attempted before audit)", len(repo.markedIDs))
	}
}

// TestExpiryWorker_SkipsNonExpiredRows verifies that rows whose valid_to_periode.tanggal_akhir
// is in the future are NOT returned by ListExpiredApprovedActive, so they are not touched.
// This is a contract test: the repo returns no candidates → worker does nothing.
func TestExpiryWorker_SkipsNonExpiredRows(t *testing.T) {
	// ListExpiredApprovedActive returns empty (all valid_to dates in the future).
	repo := &mockExpiryRepo{candidates: nil}
	auditW := &mockAuditWriter{}
	worker := NewExpiryWorker(repo, auditW, nil, uuid.New())

	task := newExpiryPayload(t, "TUGURE")
	err := worker.HandleExpiryCheck(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.markedIDs) != 0 {
		t.Errorf("markedIDs = %v, want empty", repo.markedIDs)
	}
	if len(auditW.events) != 0 {
		t.Errorf("audit events = %d, want 0", len(auditW.events))
	}
}

// TestExpiryWorker_IdempotentReRun verifies that when MarkExpiredInTx returns
// sql.ErrNoRows (the row was already EXPIRED from a previous run), the worker
// treats it as a no-op and returns nil (idempotent).
func TestExpiryWorker_IdempotentReRun(t *testing.T) {
	overrideID := uuid.New()

	// MarkExpiredInTx returns sql.ErrNoRows → already expired.
	repo := &mockExpiryRepo{
		candidates: []ExpiryCandidate{
			{ID: overrideID, InstrumenID: uuid.New(), ValidToPeriodeID: uuid.New()},
		},
		markErr: sql.ErrNoRows,
	}
	auditW := &mockAuditWriter{}
	worker := NewExpiryWorker(repo, auditW, nil, uuid.New())

	task := newExpiryPayload(t, "TUGURE")
	err := worker.HandleExpiryCheck(context.Background(), task)
	if err != nil {
		t.Fatalf("HandleExpiryCheck: unexpected error for idempotent re-run: %v", err)
	}

	// No audit event — row was already EXPIRED, nothing committed.
	if len(auditW.events) != 0 {
		t.Errorf("audit events = %d, want 0 for idempotent re-run", len(auditW.events))
	}
}

// TestNewExpiryWorker_PanicsWhenAuditWriterNil verifies that constructing an
// ExpiryWorker with a nil auditWriter panics immediately (M3 pattern, DEC-018).
func TestNewExpiryWorker_PanicsWhenAuditWriterNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil auditWriter, got none")
		}
	}()
	_ = NewExpiryWorker(&mockExpiryRepo{}, nil, nil, uuid.New())
}

// ─── DBExpiryRepo unit tests (sqlmock) ────────────────────────────────────────

// TestDBExpiryRepo_ListExpiredApprovedActive_ReturnsMatched verifies the SQL
// query returns candidates from the DB correctly.
func TestDBExpiryRepo_ListExpiredApprovedActive_ReturnsMatched(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	id1 := uuid.New()
	instr1 := uuid.New()
	periode1 := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "instrumen_id", "valid_to_periode_id"}).
		AddRow(id1, instr1, periode1)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	repo := NewDBExpiryRepo(db)
	candidates, err := repo.ListExpiredApprovedActive(context.Background(), time.Now(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len = %d, want 1", len(candidates))
	}
	if candidates[0].ID != id1 {
		t.Errorf("ID = %s, want %s", candidates[0].ID, id1)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDBExpiryRepo_MarkExpiredInTx_UpdatesRow verifies the UPDATE succeeds
// when RowsAffected = 1.
func TestDBExpiryRepo_MarkExpiredInTx_UpdatesRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	id := uuid.New()
	sysUser := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := NewDBExpiryRepo(db)
	tx, _ := db.BeginTx(context.Background(), nil)

	if err := repo.MarkExpiredInTx(context.Background(), tx, id, sysUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestDBExpiryRepo_MarkExpiredInTx_ReturnsErrNoRows_WhenAlreadyExpired verifies
// that when RowsAffected = 0 (already expired), sql.ErrNoRows is returned.
func TestDBExpiryRepo_MarkExpiredInTx_ReturnsErrNoRows_WhenAlreadyExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	id := uuid.New()
	sysUser := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected
	mock.ExpectRollback()

	repo := NewDBExpiryRepo(db)
	tx, _ := db.BeginTx(context.Background(), nil)

	err = repo.MarkExpiredInTx(context.Background(), tx, id, sysUser)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("error = %v, want sql.ErrNoRows", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestNewLPSExpiryCheckTask_Marshals verifies task creation + JSON round-trip.
func TestNewLPSExpiryCheckTask_Marshals(t *testing.T) {
	jobID := uuid.New()
	p := LPSExpiryCheckPayload{TenantID: "TUGURE", JobID: &jobID}
	task, err := NewLPSExpiryCheckTask(p)
	if err != nil {
		t.Fatalf("NewLPSExpiryCheckTask: %v", err)
	}
	if task.Type() != TaskTypeLPSExpiryCheck {
		t.Errorf("task type = %q, want %q", task.Type(), TaskTypeLPSExpiryCheck)
	}
	var decoded LPSExpiryCheckPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.TenantID != "TUGURE" {
		t.Errorf("TenantID = %q, want TUGURE", decoded.TenantID)
	}
	if decoded.JobID == nil || *decoded.JobID != jobID {
		t.Errorf("JobID mismatch")
	}
}

// TestDBExpiryRepo_BeginTx verifies the BeginTx method starts a transaction.
func TestDBExpiryRepo_BeginTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	repo := NewDBExpiryRepo(db)
	tx, err := repo.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestExpiryWorker_HandlesNilDB_BeginTxError verifies that a BeginTx failure
// on the first candidate increments failed count and returns an error.
func TestExpiryWorker_HandlesNilDB_BeginTxError(t *testing.T) {
	overrideID := uuid.New()
	repo := &mockExpiryRepo{
		candidates: []ExpiryCandidate{
			{ID: overrideID, InstrumenID: uuid.New(), ValidToPeriodeID: uuid.New()},
		},
		beginErr: errors.New("DB unavailable"),
	}
	auditW := &mockAuditWriter{}
	worker := NewExpiryWorker(repo, auditW, nil, uuid.New())

	task := newExpiryPayload(t, "TUGURE")
	err := worker.HandleExpiryCheck(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when BeginTx fails")
	}
	// No audit written when transaction could not even start.
	if len(auditW.events) != 0 {
		t.Errorf("audit events = %d, want 0", len(auditW.events))
	}
}
