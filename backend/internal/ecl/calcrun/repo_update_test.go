package calcrun_test

// repo_update_test.go — Tests for CalcRunRepo Update* methods using sqlmock.
// Each Update* method: SET fields → ExecContext → getByIDTx (SELECT).
// Tests verify: DB error propagated, success path returns updated CalcRun.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func buildUpdateRows(id uuid.UUID, status string) *sqlmock.Rows {
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	createdBy := uuid.New()
	return sqlmock.NewRows([]string{
		"id", "periode_id", "evaluation_date", "scope", "status",
		"job_id",
		"total_instrumen", "processed_count", "error_count",
		"started_at", "completed_at",
		"parameter_snapshot_jsonb",
		"seal_requested_by", "seal_requested_at",
		"sealed_by", "sealed_at",
		"signature_hash_seal",
		"seal_rejected_by", "seal_rejected_at", "reject_reason",
		"cancelled_by", "cancelled_at", "cancel_reason",
		"superseded_by_run_id",
		"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
	}).AddRow(
		id, "periode-2026-06", evalDate, "ALL_ACTIVE", status,
		nil,
		nil, 0, 0,
		nil, nil,
		nil,
		nil, nil,
		nil, nil,
		nil,
		nil, nil, nil,
		nil, nil, nil,
		nil,
		now, createdBy, now, createdBy, 1, "TUGURE",
	)
}

// ─── UpdateStatus ─────────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateStatus_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "IN_PROGRESS"))

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	updated, err := repo.UpdateStatus(context.Background(), tx, id, calcrun.StatusInProgress, actorID)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if string(updated.Status) != "IN_PROGRESS" {
		t.Errorf("status = %q; want IN_PROGRESS", updated.Status)
	}
	_ = tx.Rollback()
}

func TestCalcRunRepo_UpdateStatus_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnError(errDB("lock timeout"))

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err = repo.UpdateStatus(context.Background(), tx, id, calcrun.StatusInProgress, actorID)
	if err == nil {
		t.Error("expected error on DB failure")
	}
	_ = tx.Rollback()
}

// ─── UpdateStartFields ────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateStartFields_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()
	snap, _ := json.Marshal(map[string]any{"frozenAt": "2026-06-13T00:00:00Z"})

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "IN_PROGRESS"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateStartFields(context.Background(), tx, id, snap, "job-123", 100, actorID)
	if err != nil {
		t.Fatalf("UpdateStartFields: %v", err)
	}
	if string(updated.Status) != "IN_PROGRESS" {
		t.Errorf("status = %q; want IN_PROGRESS", updated.Status)
	}
	_ = tx.Rollback()
}

// ─── UpdateProgress (non-transactional) ──────────────────────────────────────

func TestCalcRunRepo_UpdateProgress_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.UpdateProgress(context.Background(), id, 50, 0, actorID)
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
}

func TestCalcRunRepo_UpdateProgress_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnError(errDB("connection reset"))

	err = repo.UpdateProgress(context.Background(), id, 50, 0, actorID)
	if err == nil {
		t.Error("expected error on DB failure")
	}
}

// ─── UpdateCompletion ─────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateCompletion_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "COMPLETED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateCompletion(context.Background(), tx, id, calcrun.StatusCompleted, 100, 0, actorID)
	if err != nil {
		t.Fatalf("UpdateCompletion: %v", err)
	}
	if string(updated.Status) != "COMPLETED" {
		t.Errorf("status = %q; want COMPLETED", updated.Status)
	}
	_ = tx.Rollback()
}

func TestCalcRunRepo_UpdateCompletion_WithErrors_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	actorID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "COMPLETED_WITH_ERRORS"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateCompletion(context.Background(), tx, id, calcrun.StatusCompletedWithErrors, 95, 5, actorID)
	if err != nil {
		t.Fatalf("UpdateCompletion: %v", err)
	}
	if string(updated.Status) != "COMPLETED_WITH_ERRORS" {
		t.Errorf("status = %q; want COMPLETED_WITH_ERRORS", updated.Status)
	}
	_ = tx.Rollback()
}

// ─── UpdateSealRequest ────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateSealRequest_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	requestedBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "SEAL_REQUESTED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateSealRequest(context.Background(), tx, id, requestedBy, "Requesting seal for audit.")
	if err != nil {
		t.Fatalf("UpdateSealRequest: %v", err)
	}
	if string(updated.Status) != "SEAL_REQUESTED" {
		t.Errorf("status = %q; want SEAL_REQUESTED", updated.Status)
	}
	_ = tx.Rollback()
}

func TestCalcRunRepo_UpdateSealRequest_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	requestedBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnError(errDB("serialization failure"))

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err = repo.UpdateSealRequest(context.Background(), tx, id, requestedBy, "comment")
	if err == nil {
		t.Error("expected error on DB failure")
	}
	_ = tx.Rollback()
}

// TestCalcRunRepo_UpdateSealRequest_CommentPersisted verifies that seal_request_comment
// is written to the UPDATE statement (F-2 fix: column was previously omitted).
func TestCalcRunRepo_UpdateSealRequest_CommentPersisted(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	requestedBy := uuid.New()
	comment := "Meminta seal untuk audit periode Juni 2026."

	mock.ExpectBegin()
	// The UPDATE must include seal_request_comment = $3 (F-2 fix).
	mock.ExpectExec(`seal_request_comment`).
		WithArgs(string(calcrun.StatusSealRequested), requestedBy, comment, id).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "SEAL_REQUESTED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateSealRequest(context.Background(), tx, id, requestedBy, comment)
	if err != nil {
		t.Fatalf("UpdateSealRequest with comment: %v", err)
	}
	if string(updated.Status) != "SEAL_REQUESTED" {
		t.Errorf("status = %q; want SEAL_REQUESTED", updated.Status)
	}
	if expectErr := mock.ExpectationsWereMet(); expectErr != nil {
		t.Errorf("mock expectations not met (seal_request_comment not in UPDATE): %v", expectErr)
	}
	_ = tx.Rollback()
}

// ─── UpdateSealApprove ────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateSealApprove_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	approverID := uuid.New()
	sigHash := []byte("sig-hash-32-bytes-padded-here---x")

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "SEALED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateSealApprove(context.Background(), tx, id, approverID, sigHash)
	if err != nil {
		t.Fatalf("UpdateSealApprove: %v", err)
	}
	if string(updated.Status) != "SEALED" {
		t.Errorf("status = %q; want SEALED", updated.Status)
	}
	_ = tx.Rollback()
}

// ─── UpdateSealReject ─────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateSealReject_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	rejectedBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "COMPLETED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateSealReject(context.Background(), tx, id, rejectedBy, "Data tidak lengkap untuk periode ini.")
	if err != nil {
		t.Fatalf("UpdateSealReject: %v", err)
	}
	if string(updated.Status) != "COMPLETED" {
		t.Errorf("status = %q; want COMPLETED (reverted after reject)", updated.Status)
	}
	_ = tx.Rollback()
}

// ─── UpdateCancel ─────────────────────────────────────────────────────────────

func TestCalcRunRepo_UpdateCancel_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	cancelledBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(buildUpdateRows(id, "CANCELLED"))

	tx, _ := db.BeginTx(context.Background(), nil)
	updated, err := repo.UpdateCancel(context.Background(), tx, id, cancelledBy, "Dicancel karena parameter berubah — perlu re-create.")
	if err != nil {
		t.Fatalf("UpdateCancel: %v", err)
	}
	if string(updated.Status) != "CANCELLED" {
		t.Errorf("status = %q; want CANCELLED", updated.Status)
	}
	_ = tx.Rollback()
}

func TestCalcRunRepo_UpdateCancel_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	id := uuid.New()
	cancelledBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnError(errDB("constraint violation"))

	tx, _ := db.BeginTx(context.Background(), nil)
	_, err = repo.UpdateCancel(context.Background(), tx, id, cancelledBy, "some long enough cancel reason that satisfies minimum")
	if err == nil {
		t.Error("expected error on DB failure")
	}
	_ = tx.Rollback()
}

// ─── BeginTx ─────────────────────────────────────────────────────────────────

func TestCalcRunRepo_BeginTx_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	mock.ExpectBegin()

	tx, err := repo.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if tx == nil {
		t.Error("expected non-nil tx")
	}
	_ = tx.Rollback()
}

func TestCalcRunRepo_BeginTx_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewRepo(db)

	mock.ExpectBegin().WillReturnError(errDB("max connections reached"))

	_, err = repo.BeginTx(context.Background())
	if err == nil {
		t.Error("expected error on DB failure")
	}
}
