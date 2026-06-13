package calcrun_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
)

// repo_test.go — sqlmock-based tests for CalcRunRepo.
// Tests DB interaction: IsSealedCalcRun, CheckExistingInProgress, CheckExistingSealed,
// NewCalcRunRepo panic guard.
//
// Full CRUD paths (Create, UpdateSealApprove etc.) are covered by integration tests.
// These tests verify the "sealed guard" path (the most critical compliance path).

func newMockDB(t *testing.T) (*calcrun.CalcRunRepo, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := calcrun.NewCalcRunRepo(db)
	return repo, mock
}

// ─── NewCalcRunRepo panic guard ───────────────────────────────────────────────

func TestNewCalcRunRepo_PanicOnNilDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	calcrun.NewCalcRunRepo(nil)
}

// ─── IsSealedCalcRun ──────────────────────────────────────────────────────────

func TestCalcRunRepo_IsSealedCalcRun_ReturnsTrue(t *testing.T) {
	repo, mock := newMockDB(t)
	calcRunID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("SEALED"))

	sealed, err := repo.IsSealedCalcRun(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sealed {
		t.Error("expected sealed=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCalcRunRepo_IsSealedCalcRun_ReturnsFalse_NonSealed(t *testing.T) {
	for _, status := range []string{"DRAFT", "IN_PROGRESS", "COMPLETED", "COMPLETED_WITH_ERRORS", "SEAL_REQUESTED", "CANCELLED"} {
		status := status
		t.Run(status, func(t *testing.T) {
			repo, mock := newMockDB(t)
			calcRunID := uuid.New()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`)).
				WithArgs(calcRunID).
				WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(status))

			sealed, err := repo.IsSealedCalcRun(context.Background(), calcRunID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sealed {
				t.Errorf("expected sealed=false for status %q", status)
			}
		})
	}
}

func TestCalcRunRepo_IsSealedCalcRun_ReturnsFalse_NotFound(t *testing.T) {
	repo, mock := newMockDB(t)
	calcRunID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"status"})) // empty result set

	sealed, err := repo.IsSealedCalcRun(context.Background(), calcRunID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sealed {
		t.Error("expected sealed=false for missing row")
	}
}

func TestCalcRunRepo_IsSealedCalcRun_DBError(t *testing.T) {
	repo, mock := newMockDB(t)
	calcRunID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(calcRunID).
		WillReturnError(errDB("connection refused"))

	_, err := repo.IsSealedCalcRun(context.Background(), calcRunID)
	if err == nil {
		t.Error("expected error on DB failure")
	}
}

// ─── CheckExistingInProgress ──────────────────────────────────────────────────

func TestCalcRunRepo_CheckExistingInProgress_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	existingID := uuid.New().String()
	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))

	found, err := repo.CheckExistingInProgress(context.Background(), "periode-2026-06")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != existingID {
		t.Errorf("found = %q; want %q", found, existingID)
	}
}

func TestCalcRunRepo_CheckExistingInProgress_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty

	found, err := repo.CheckExistingInProgress(context.Background(), "periode-2026-06")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != "" {
		t.Errorf("found = %q; want empty", found)
	}
}

// ─── CheckExistingSealed ──────────────────────────────────────────────────────

func TestCalcRunRepo_CheckExistingSealed_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	sealedID := uuid.New().String()
	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(sealedID))

	found, err := repo.CheckExistingSealed(context.Background(), "periode-2026-06")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != sealedID {
		t.Errorf("found = %q; want %q", found, sealedID)
	}
}

func TestCalcRunRepo_CheckExistingSealed_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	found, err := repo.CheckExistingSealed(context.Background(), "periode-2026-06")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != "" {
		t.Errorf("found = %q; want empty", found)
	}
}

// ─── Get: not found ───────────────────────────────────────────────────────────

func TestCalcRunRepo_Get_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	id := uuid.New()
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty → ErrNoRows

	_, err = repo.Get(context.Background(), id)
	if err == nil {
		t.Error("expected error for not found")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "CALC_RUN_NOT_FOUND" {
		t.Errorf("code = %q; want CALC_RUN_NOT_FOUND", ce.Code())
	}
	if ce.HTTPStatus() != 404 {
		t.Errorf("http = %d; want 404", ce.HTTPStatus())
	}
}

// ─── List: empty ──────────────────────────────────────────────────────────────

func TestCalcRunRepo_List_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "sealed_at",
			"created_at", "created_by",
		}))

	items, nextCursor, hasMore, err := repo.List(context.Background(), "periode-2026-06", 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("items len = %d; want 0", len(items))
	}
	if nextCursor != "" {
		t.Errorf("nextCursor = %q; want empty", nextCursor)
	}
	if hasMore {
		t.Error("hasMore = true; want false")
	}
}

// ─── List: with data + hasMore pagination ─────────────────────────────────────

func TestCalcRunRepo_List_WithData(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	id1 := uuid.New()
	id2 := uuid.New()
	createdBy := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "periode_id", "evaluation_date", "scope", "status",
		"processed_count", "error_count", "total_instrumen",
		"started_at", "completed_at", "sealed_at",
		"created_at", "created_by",
	}).
		AddRow(id1, "p-2026-06", evalDate, "ALL_ACTIVE", "COMPLETED", 100, 0, 100, nil, now, nil, now, createdBy).
		AddRow(id2, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT", 0, 0, nil, nil, nil, nil, now, createdBy)

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(rows)

	items, cursor, hasMore, err := repo.List(context.Background(), "p-2026-06", 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("items = %d; want 2", len(items))
	}
	if cursor != "" {
		t.Errorf("cursor = %q; want empty (limit not hit)", cursor)
	}
	if hasMore {
		t.Error("hasMore = true; want false")
	}
	if string(items[0].Status) != "COMPLETED" {
		t.Errorf("items[0].Status = %q; want COMPLETED", items[0].Status)
	}
}

func TestCalcRunRepo_List_HasMore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	createdBy := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Return limit+1 rows (limit=2, return 3) to trigger hasMore=true.
	rows := sqlmock.NewRows([]string{
		"id", "periode_id", "evaluation_date", "scope", "status",
		"processed_count", "error_count", "total_instrumen",
		"started_at", "completed_at", "sealed_at",
		"created_at", "created_by",
	})
	ids := make([]uuid.UUID, 3)
	for i := range ids {
		ids[i] = uuid.New()
		rows.AddRow(ids[i], "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT", 0, 0, nil, nil, nil, nil, now, createdBy)
	}

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(rows)

	items, cursor, hasMore, err := repo.List(context.Background(), "p-2026-06", 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("items = %d; want 2 (limit=2)", len(items))
	}
	if !hasMore {
		t.Error("hasMore = false; want true")
	}
	if cursor == "" {
		t.Error("cursor empty; want populated when hasMore=true")
	}
}

func TestCalcRunRepo_List_NoPeriodeID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	// No periodeID filter → different SQL args.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"processed_count", "error_count", "total_instrumen",
			"started_at", "completed_at", "sealed_at",
			"created_at", "created_by",
		}))

	items, _, _, err := repo.List(context.Background(), "" /* no periodeID */, 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("items = %d; want 0", len(items))
	}
}

func TestCalcRunRepo_List_InvalidLimit_Clamped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	// limit=0 → clamped to 50 internally.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"processed_count", "error_count", "total_instrumen",
			"started_at", "completed_at", "sealed_at",
			"created_at", "created_by",
		}))

	_, _, _, err = repo.List(context.Background(), "", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── IsSealedCalcRun — full sealed lifecycle check ────────────────────────────

func TestCalcRunRepo_IsSealedCalcRun_AllStatuses(t *testing.T) {
	// Table test: only SEALED should return true.
	tests := []struct {
		status string
		want   bool
	}{
		{"DRAFT", false},
		{"IN_PROGRESS", false},
		{"COMPLETED", false},
		{"COMPLETED_WITH_ERRORS", false},
		{"SEAL_REQUESTED", false},
		{"SEALED", true},
		{"CANCELLED", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.status, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer func() { _ = db.Close() }()
			repo := calcrun.NewCalcRunRepo(db)
			id := uuid.New()

			mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM ecl.calc_run WHERE id = $1 AND deleted_at IS NULL`)).
				WithArgs(id).
				WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(tt.status))

			got, err := repo.IsSealedCalcRun(context.Background(), id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("IsSealedCalcRun(status=%q) = %v; want %v", tt.status, got, tt.want)
			}
		})
	}
}

// ─── Get: fully-populated row (all nullable columns non-NULL) ────────────────
//
// This test exercises every optional branch in scanCalcRun:
// SealRequestedBy/At, SealApprovedBy/At, SealedAt, SignatureHashSeal,
// SealRejectedBy/At, RejectReason, CancelledBy/At, CancelReason, SupersededByRunID.
//
// Per PSAK 71 compliance: the sealed state must roundtrip without data loss.

func TestCalcRunRepo_Get_WithAllFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := calcrun.NewCalcRunRepo(db)

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	createdBy := uuid.New()
	sealReqBy := uuid.New()
	sealApprBy := uuid.New()
	sealRejBy := uuid.New()
	cancelledBy := uuid.New()
	supersededByID := uuid.New()
	jobID := "job-abc-123"
	totalInstrumen := int64(500)
	sigHash := []byte("sha256hashbytes")
	rejectReason := "PD data not approved"
	cancelReason := "User requested cancellation"

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id",
			"total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at",
			"parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			id, "p-2026-06", now, "ALL_ACTIVE", "SEALED",
			jobID,
			totalInstrumen, 500, 0,
			now, now,
			[]byte(`{"periodeId":"p-2026-06"}`),
			sealReqBy.String(), now,
			sealApprBy.String(), now,
			now, sigHash,
			sealRejBy.String(), now, rejectReason,
			cancelledBy.String(), now, cancelReason,
			supersededByID.String(),
			now, createdBy, now, createdBy, int64(3), "TUGURE",
		))

	run, err := repo.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify all optional fields roundtripped correctly.
	if run.JobID == nil || *run.JobID != jobID {
		t.Errorf("JobID = %v; want %q", run.JobID, jobID)
	}
	if run.TotalInstrumen == nil || *run.TotalInstrumen != int(totalInstrumen) {
		t.Errorf("TotalInstrumen = %v; want %d", run.TotalInstrumen, totalInstrumen)
	}
	if run.SealRequestedBy == nil || *run.SealRequestedBy != sealReqBy {
		t.Errorf("SealRequestedBy = %v; want %s", run.SealRequestedBy, sealReqBy)
	}
	if run.SealRequestedAt == nil {
		t.Error("SealRequestedAt is nil; want populated")
	}
	if run.SealApprovedBy == nil || *run.SealApprovedBy != sealApprBy {
		t.Errorf("SealApprovedBy = %v; want %s", run.SealApprovedBy, sealApprBy)
	}
	if run.SealApprovedAt == nil {
		t.Error("SealApprovedAt is nil; want populated")
	}
	if run.SealedAt == nil {
		t.Error("SealedAt is nil; want populated")
	}
	if run.SealRejectedBy == nil || *run.SealRejectedBy != sealRejBy {
		t.Errorf("SealRejectedBy = %v; want %s", run.SealRejectedBy, sealRejBy)
	}
	if run.SealRejectedAt == nil {
		t.Error("SealRejectedAt is nil; want populated")
	}
	if run.RejectReason == nil || *run.RejectReason != rejectReason {
		t.Errorf("RejectReason = %v; want %q", run.RejectReason, rejectReason)
	}
	if run.CancelledBy == nil || *run.CancelledBy != cancelledBy {
		t.Errorf("CancelledBy = %v; want %s", run.CancelledBy, cancelledBy)
	}
	if run.CancelledAt == nil {
		t.Error("CancelledAt is nil; want populated")
	}
	if run.CancelReason == nil || *run.CancelReason != cancelReason {
		t.Errorf("CancelReason = %v; want %q", run.CancelReason, cancelReason)
	}
	if run.SupersededByRunID == nil || *run.SupersededByRunID != supersededByID {
		t.Errorf("SupersededByRunID = %v; want %s", run.SupersededByRunID, supersededByID)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// errDB returns a simple error for simulating DB failures.
func errDB(msg string) error {
	return &dbErr{msg}
}

type dbErr struct{ msg string }

func (e *dbErr) Error() string { return "DB error: " + e.msg }

// Verify time.Time parsing works for evaluation_date column (DB stores as date).
func TestTimeParseYYYYMMDD(t *testing.T) {
	date, err := time.Parse("2006-01-02", "2026-06-13")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if date.Year() != 2026 || date.Month() != 6 || date.Day() != 13 {
		t.Errorf("parsed date = %v; want 2026-06-13", date)
	}
}
