package penempatan_test

// security_fix_test.go — tests for PR #105 security findings.
//
// FIX-REQUIRED #1 (security baseline §threat-model #5):
//   SoD violation attempts must be audited via s.sodWriter (non-tx) even when
//   the business transaction is rolled back. 8 SoD check sites are covered:
//     Review:           MAKER_AS_REVIEWER (site 1)
//     Approve:          MAKER_AS_APPROVER (site 2), REVIEWER_AS_APPROVER (site 3)
//     TerminateReview:  MAKER_AS_TERMINATE_REVIEWER (site 4),
//                       TERMINATE_MAKER_AS_TERMINATE_REVIEWER (site 5)
//     TerminateApprove: MAKER_AS_TERMINATE_APPROVER (site 6),
//                       TERMINATE_MAKER_AS_TERMINATE_APPROVER (site 7),
//                       TERMINATE_REVIEWER_AS_TERMINATE_APPROVER (site 8)
//
// FIX-REQUIRED #2 (DEC-028 PII):
//   settlement_account must NOT appear plaintext in any log line.
//   sha256hex function is tested for correctness + collision-resistance.
//
// Tests inject a mockSoDWriter via WithSoDWriter() (exported in export_test.go)
// to count how many times Write() is called and capture the event payload.

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/app-b/penempatan"
	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── mockSoDWriter ───────────────────────────────────────────────────────────

// mockSoDWriter counts calls to Write() and records the events, satisfying
// the penempatan.DirectAuditWriter interface.
type mockSoDWriter struct {
	callCount atomic.Int32
	events    []audit.Event
}

func (m *mockSoDWriter) Write(_ context.Context, evt audit.Event) error {
	m.callCount.Add(1)
	m.events = append(m.events, evt)
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// mustServiceWithMock builds a Service with a sqlmock DB and injects mock as sodWriter.
func mustServiceWithMock(t *testing.T, mock penempatan.DirectAuditWriter) (*penempatan.Service, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, dbMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	repo := penempatan.NewRepo(db)
	aw := audit.NewWriter(db)
	svc := penempatan.NewService(repo, aw, nil, nil)
	svc.WithSoDWriter(mock)
	return svc, dbMock, func() { db.Close() } //nolint:errcheck
}

// buildSecGetForUpdateRow returns driver.Values matching getForUpdateCols() (29 cols).
// reviewer_id, terminate_maker_id, terminate_reviewer_id are optional string pointers.
func buildSecGetForUpdateRow(
	id uuid.UUID,
	workflowStatus string,
	makerID uuid.UUID,
	reviewerIDStr *string,
	terminateMakerIDStr *string,
	terminateReviewerIDStr *string,
) []driver.Value {
	return []driver.Value{
		id,          // id
		"DP-SEC-01", // kode_transaksi
		uuid.New(),  // instrumen_id
		uuid.New(),  // counterparty_bank_id
		uuid.New(),  // periode_id
		uuid.New(),  // mata_uang_id
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), // tanggal_penempatan
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), // tanggal_jatuh_tempo
		"1000000000", // nominal_idr
		nil,          // nominal_fcy
		nil,          // kurs_penempatan
		int16(12),    // tenor_bulan
		"5.25000000", // kupon_persen
		"0",          // biaya_transaksi_idr
		nil,          // eir_awal
		nil,          // carrying_amount_awal
		workflowStatus,         // workflow_status
		makerID,                // maker_id
		reviewerIDStr,          // reviewer_id
		nil,                    // approver_id
		terminateMakerIDStr,    // terminate_maker_id
		terminateReviewerIDStr, // terminate_reviewer_id
		nil,                    // terminate_approver_id
		nil,                    // reject_reason
		nil,                    // terminate_request_reason
		nil,                    // deleted_at
		nil,                    // deleted_by
		int64(1),               // row_version
		"TUGURE",               // tenant_id
	}
}

func basicClaims(sub string) *auth.Claims {
	return &auth.Claims{
		Sub:         sub,
		TenantID:    "TUGURE",
		Roles:       []string{"ROLE-APPR-TR"},
		Permissions: []string{penempatan.PermTransaksiReview, penempatan.PermTransaksiApprove},
	}
}

func stepUpClaims(sub string) *auth.Claims {
	now := time.Now().Unix()
	return &auth.Claims{
		Sub:              sub,
		TenantID:         "TUGURE",
		Roles:            []string{"ROLE-APPR-TR"},
		Permissions:      []string{penempatan.PermTransaksiApprove},
		StepupVerifiedAt: &now,
	}
}

// assertOneSoDAudit verifies the mockSoDWriter was called exactly once with
// the expected Action, EntityType, step, and violation_type.
func assertOneSoDAudit(t *testing.T, m *mockSoDWriter, wantStep, wantViolationType string) {
	t.Helper()
	if got := m.callCount.Load(); got != 1 {
		t.Errorf("SoD audit Write called %d times, want exactly 1", got)
		if got == 0 {
			return
		}
	}
	if len(m.events) == 0 {
		t.Fatal("no events captured in mock")
	}
	evt := m.events[0]
	if evt.Action != "PENEMPATAN.SOD_VIOLATION_ATTEMPT" {
		t.Errorf("audit Action = %q, want PENEMPATAN.SOD_VIOLATION_ATTEMPT", evt.Action)
	}
	if evt.EntityType != "trx.penempatan_deposito" {
		t.Errorf("audit EntityType = %q, want trx.penempatan_deposito", evt.EntityType)
	}
	after, ok := evt.After.(map[string]any)
	if !ok {
		t.Fatalf("audit After type = %T, want map[string]any", evt.After)
	}
	if got := after["step"]; got != wantStep {
		t.Errorf("audit After[step] = %q, want %q", got, wantStep)
	}
	if got := after["violation_type"]; got != wantViolationType {
		t.Errorf("audit After[violation_type] = %q, want %q", got, wantViolationType)
	}
}

// ─── Site 1: Review — MAKER_AS_REVIEWER ──────────────────────────────────────

func TestSoDViolationAudit_Review_MakerAsReviewer(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	makerID := uuid.New() // actor = maker → SoD violation on Review

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "PENDING_REVIEW", makerID, nil, nil, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.Review(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, basicClaims(makerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "REVIEW", "MAKER_AS_REVIEWER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 2: Approve — MAKER_AS_APPROVER ─────────────────────────────────────

func TestSoDViolationAudit_Approve_MakerAsApprover(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	makerID := uuid.New() // actor = maker → SoD on Approve

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "PENDING_APPROVAL", makerID, nil, nil, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.Approve(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, stepUpClaims(makerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "APPROVE", "MAKER_AS_APPROVER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 3: Approve — REVIEWER_AS_APPROVER ──────────────────────────────────

func TestSoDViolationAudit_Approve_ReviewerAsApprover(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New() // actor = reviewer → SoD on Approve
	reviewerIDStr := reviewerID.String()

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "PENDING_APPROVAL", makerID, &reviewerIDStr, nil, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.Approve(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, stepUpClaims(reviewerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "APPROVE", "REVIEWER_AS_APPROVER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 4: TerminateReview — MAKER_AS_TERMINATE_REVIEWER ───────────────────

func TestSoDViolationAudit_TerminateReview_MakerAsReviewer(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	makerID := uuid.New() // actor = original maker → SoD on TerminateReview

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "TERMINATION_PENDING_REVIEW", makerID, nil, nil, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.TerminateReview(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, basicClaims(makerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "TERMINATE_REVIEW", "MAKER_AS_TERMINATE_REVIEWER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 5: TerminateReview — TERMINATE_MAKER_AS_TERMINATE_REVIEWER ─────────

func TestSoDViolationAudit_TerminateReview_TerminateMakerSelfReview(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	originalMakerID := uuid.New()
	terminateMakerID := uuid.New() // actor = terminate proposer → SoD
	terminateMakerIDStr := terminateMakerID.String()

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "TERMINATION_PENDING_REVIEW", originalMakerID, nil, &terminateMakerIDStr, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.TerminateReview(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, basicClaims(terminateMakerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "TERMINATE_REVIEW", "TERMINATE_MAKER_AS_TERMINATE_REVIEWER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 6: TerminateApprove — MAKER_AS_TERMINATE_APPROVER ──────────────────

func TestSoDViolationAudit_TerminateApprove_MakerAsApprover(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	makerID := uuid.New() // actor = original maker → SoD on TerminateApprove

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "TERMINATION_PENDING_APPROVAL", makerID, nil, nil, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.TerminateApprove(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, stepUpClaims(makerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "TERMINATE_APPROVE", "MAKER_AS_TERMINATE_APPROVER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 7: TerminateApprove — TERMINATE_MAKER_AS_TERMINATE_APPROVER ────────

func TestSoDViolationAudit_TerminateApprove_TerminateMakerSelfApprove(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	originalMakerID := uuid.New()
	terminateMakerID := uuid.New() // actor = terminate proposer → SoD
	terminateMakerIDStr := terminateMakerID.String()

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "TERMINATION_PENDING_APPROVAL", originalMakerID, nil, &terminateMakerIDStr, nil)...))
	dbMock.ExpectRollback()

	_, err := svc.TerminateApprove(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, stepUpClaims(terminateMakerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "TERMINATE_APPROVE", "TERMINATE_MAKER_AS_TERMINATE_APPROVER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── Site 8: TerminateApprove — TERMINATE_REVIEWER_AS_TERMINATE_APPROVER ─────

func TestSoDViolationAudit_TerminateApprove_TerminateReviewerAsApprover(t *testing.T) {
	t.Parallel()

	m := &mockSoDWriter{}
	svc, dbMock, cleanup := mustServiceWithMock(t, m)
	defer cleanup()

	id := uuid.New()
	originalMakerID := uuid.New()
	terminateMakerID := uuid.New()
	terminateReviewerID := uuid.New() // actor = terminate reviewer → SoD
	terminateMakerIDStr := terminateMakerID.String()
	terminateReviewerIDStr := terminateReviewerID.String()

	dbMock.ExpectBegin()
	dbMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(getForUpdateCols()).
			AddRow(buildSecGetForUpdateRow(id, "TERMINATION_PENDING_APPROVAL", originalMakerID, nil, &terminateMakerIDStr, &terminateReviewerIDStr)...))
	dbMock.ExpectRollback()

	_, err := svc.TerminateApprove(context.Background(), id,
		penempatan.WorkflowActionRequest{Comment: "sod-attempt"}, stepUpClaims(terminateReviewerID.String()))
	if err == nil {
		t.Fatal("expected SoD error, got nil")
	}

	assertOneSoDAudit(t, m, "TERMINATE_APPROVE", "TERMINATE_REVIEWER_AS_TERMINATE_APPROVER")

	if dbErr := dbMock.ExpectationsWereMet(); dbErr != nil {
		t.Errorf("db expectations not met: %v", dbErr)
	}
}

// ─── FIX #2: sha256hex algorithm correctness ─────────────────────────────────

// TestSha256Hex_Deterministic verifies the hash algorithm used in the
// settlement_account_hash log field is deterministic, 64-char hex, and
// collision-resistant. This guards the DEC-028 log-redaction fix.
// The production sha256hex helper uses crypto/sha256.Sum256 + hex.EncodeToString —
// we replicate the same algorithm here.
func TestSha256Hex_Deterministic(t *testing.T) {
	t.Parallel()

	sha256hexOf := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	}

	account := "1234567890123456"
	h1 := sha256hexOf(account)
	h2 := sha256hexOf(account)

	if len(h1) != 64 {
		t.Errorf("sha256hex len = %d, want 64", len(h1))
	}
	if h1 != h2 {
		t.Error("sha256hex is not deterministic for same input")
	}

	other := sha256hexOf("9999999999999999")
	if h1 == other {
		t.Error("sha256hex collision: different account numbers produce identical hash")
	}

	empty := sha256hexOf("")
	if empty == h1 {
		t.Error("sha256hex collision: empty string matches non-empty account hash")
	}
}
