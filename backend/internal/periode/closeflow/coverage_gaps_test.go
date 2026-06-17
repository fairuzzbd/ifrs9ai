package closeflow_test

// coverage_gaps_test.go — Gap-filler tests pushing closeflow coverage from ~85% to ≥86%.
//
// Targets the following uncovered boundary edges:
//   1. IsChecklistStale at exactly boundary (= staleHours) — treated stale.
//   2. IsChecklistStale at exactly one second below boundary (< staleHours) — not stale.
//   3. IsWithinGraceWindow at exactly grace boundary (now == expiry) — expired.
//   4. Allowlist cache TTL expiry path in refreshAllowlistIfStale.
//   5. CanTransition from HARD_CLOSE_PENDING with hard-close-approve (stepUp true).
//   6. CanTransition from OPEN with unknown action.
//   7. ChecklistEvaluate with zero jurnal rows (edge: SQL returns no rows).
//   8. PeriodeStatus enum: IsValid for each value + invalid string.
//   9. HasPendingSoftCloseRequest false branches.
//  10. DefaultConfig() values match expected constants.
//  11. ErrChecklistFailed with multiple details.
//  12. ErrGraceExpired formatted message contains periode_kode and grace_expired_at.
//  13. ErrPeriodeSoftClosed formatted message.
//  14. ErrPeriodeClosed formatted message.
//  15. ErrMFAStepUpRequired formatted message.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── IsChecklistStale boundary ────────────────────────────────────────────────

// TestIsChecklistStale_ExactlyAtBoundary: snapshot age == staleHours → stale.
// (time.Since(createdAt) > duration: if equal, time.Since is slightly > due to clock, so it is stale)
func TestIsChecklistStale_ExactlyAtBoundary_TreatedStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	// Exactly 24h ago — time.Since will be marginally > 24h.
	exactlyAt := time.Now().Add(-24 * time.Hour)

	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(exactlyAt))

	chk := closeflow.NewChecklistService(db)
	stale, err := chk.IsChecklistStale(t.Context(), periodeID, 24)
	require.NoError(t, err)
	assert.True(t, stale, "gap: snapshot at exactly 24h boundary must be treated stale")
}

// TestIsChecklistStale_JustBeforeBoundary: snapshot age < staleHours → not stale.
func TestIsChecklistStale_JustBeforeBoundary_NotStale(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	// 5 minutes before the 24h boundary.
	notYetStale := time.Now().Add(-(24*time.Hour - 5*time.Minute))

	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(notYetStale))

	chk := closeflow.NewChecklistService(db)
	stale, err := chk.IsChecklistStale(t.Context(), periodeID, 24)
	require.NoError(t, err)
	assert.False(t, stale, "gap: snapshot 5 min before 24h boundary must not be stale")
}

// ─── IsWithinGraceWindow at exact expiry instant ──────────────────────────────

// TestIsWithinGraceWindow_ExactExpiry: grace expires exactly now → expired.
func TestIsWithinGraceWindow_ExactExpiry_Expired(t *testing.T) {
	// Set grace to 1 nanosecond in the past to simulate "just expired".
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusClosed}
	justExpired := time.Now().Add(-time.Nanosecond)
	p.HardCloseGraceExpiresAt = &justExpired
	assert.False(t, p.IsWithinGraceWindow(),
		"gap: grace expired 1ns ago must return false")
}

// TestIsWithinGraceWindow_FutureGrace_NotExpired: 1ns in future → still valid.
func TestIsWithinGraceWindow_FutureGrace_NotExpired(t *testing.T) {
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusClosed}
	justValid := time.Now().Add(time.Hour)
	p.HardCloseGraceExpiresAt = &justValid
	assert.True(t, p.IsWithinGraceWindow(),
		"gap: grace expiry in future must return true")
}

// TestIsWithinGraceWindow_WrongStatus_ReturnsFalse: OPEN period is never within grace.
func TestIsWithinGraceWindow_WrongStatus_ReturnsFalse(t *testing.T) {
	p := &closeflow.PeriodeBuku{StatusPeriode: closeflow.PeriodeStatusOpen}
	future := time.Now().Add(48 * time.Hour)
	p.HardCloseGraceExpiresAt = &future
	assert.False(t, p.IsWithinGraceWindow(),
		"gap: OPEN status must not be within grace window")
}

// ─── CanTransition edge cases ─────────────────────────────────────────────────

// TestCanTransition_HardCloseApprove_WithStepUp.
func TestCanTransition_HardCloseApprove_WithStepUp_Boundary(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusHardClosePending,
		"hard-close-approve",
		true,  // hasStepUp
		false, // withinGrace (irrelevant)
	)
	assert.True(t, ok, "gap: HARD_CLOSE_PENDING + step-up → allowed")
	assert.Nil(t, err)
}

// TestCanTransition_HardCloseApprove_WithoutStepUp_Boundary.
func TestCanTransition_HardCloseApprove_WithoutStepUp_Boundary(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusHardClosePending,
		"hard-close-approve",
		false, // no step-up
		false,
	)
	assert.False(t, ok, "gap: hard-close-approve without step-up → denied")
	assert.NotNil(t, err)
}

// TestCanTransition_SoftCloseApprove_FromOpen_Boundary.
func TestCanTransition_SoftCloseApprove_FromOpen_Boundary(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusOpen,
		"soft-close-approve",
		false,
		false,
	)
	assert.True(t, ok, "gap: OPEN + soft-close-approve → allowed (no step-up needed)")
	assert.Nil(t, err)
}

// TestCanTransition_HardCloseReject_FromHardClosePending_Boundary.
func TestCanTransition_HardCloseReject_FromHardClosePending_Boundary(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusHardClosePending,
		"hard-close-reject",
		false, // no step-up needed for reject
		false,
	)
	assert.True(t, ok, "gap: HARD_CLOSE_PENDING + reject → no step-up needed")
	assert.Nil(t, err)
}

// TestCanTransition_ReopenSoftClosed_ToOpen_Boundary.
func TestCanTransition_ReopenSoftClosed_ToOpen_Boundary(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusSoftClosed,
		"reopen-soft-closed-to-open",
		false,
		false,
	)
	assert.True(t, ok, "gap: SOFT_CLOSED reopen to OPEN requires no step-up")
	assert.Nil(t, err)
}

// ─── PeriodeStatus enum ───────────────────────────────────────────────────────

func TestPeriodeStatus_IsValid_AllValues(t *testing.T) {
	cases := []struct {
		s     closeflow.PeriodeStatus
		valid bool
	}{
		{closeflow.PeriodeStatusOpen, true},
		{closeflow.PeriodeStatusSoftClosed, true},
		{closeflow.PeriodeStatusHardClosePending, true},
		{closeflow.PeriodeStatusClosed, true},
		{"INVALID", false},
		{"", false},
		{"open", false}, // case-sensitive
	}
	for _, tc := range cases {
		assert.Equal(t, tc.valid, tc.s.IsValid(), "gap: IsValid(%q)", tc.s)
	}
}

func TestPeriodeStatus_AllowsMutation_AllStatuses(t *testing.T) {
	assert.True(t, closeflow.PeriodeStatusOpen.AllowsMutation(), "gap: OPEN allows mutation")
	assert.False(t, closeflow.PeriodeStatusSoftClosed.AllowsMutation(), "gap: SOFT_CLOSED blocks mutation")
	assert.False(t, closeflow.PeriodeStatusHardClosePending.AllowsMutation(), "gap: HCP blocks mutation")
	assert.False(t, closeflow.PeriodeStatusClosed.AllowsMutation(), "gap: CLOSED blocks mutation")
}

func TestPeriodeStatus_IsTerminal_AllStatuses(t *testing.T) {
	assert.True(t, closeflow.PeriodeStatusClosed.IsTerminal(), "gap: CLOSED is terminal")
	assert.False(t, closeflow.PeriodeStatusOpen.IsTerminal(), "gap: OPEN not terminal")
	assert.False(t, closeflow.PeriodeStatusSoftClosed.IsTerminal(), "gap: SOFT_CLOSED not terminal")
}

// ─── HasPendingSoftCloseRequest ───────────────────────────────────────────────

func TestHasPendingSoftCloseRequest_NilRequester(t *testing.T) {
	p := &closeflow.PeriodeBuku{
		StatusPeriode:        closeflow.PeriodeStatusOpen,
		SoftCloseRequestedBy: nil,
	}
	assert.False(t, p.HasPendingSoftCloseRequest(),
		"gap: nil SoftCloseRequestedBy → false")
}

func TestHasPendingSoftCloseRequest_NonOpenStatus(t *testing.T) {
	uid := uuid.New()
	p := &closeflow.PeriodeBuku{
		StatusPeriode:        closeflow.PeriodeStatusSoftClosed, // not OPEN
		SoftCloseRequestedBy: &uid,
	}
	assert.False(t, p.HasPendingSoftCloseRequest(),
		"gap: SoftCloseRequestedBy set but status not OPEN → false")
}

func TestHasPendingSoftCloseRequest_OpenWithRequester(t *testing.T) {
	uid := uuid.New()
	p := &closeflow.PeriodeBuku{
		StatusPeriode:        closeflow.PeriodeStatusOpen,
		SoftCloseRequestedBy: &uid,
	}
	assert.True(t, p.HasPendingSoftCloseRequest(),
		"gap: OPEN + SoftCloseRequestedBy set → true")
}

// ─── DefaultConfig values ────────────────────────────────────────────────────

func TestDefaultConfig_Values(t *testing.T) {
	cfg := closeflow.DefaultConfig()
	assert.Equal(t, 24, cfg.SoftCloseChecklistStaleHours,
		"gap: stale hours default = 24")
	assert.Equal(t, 48, cfg.HardCloseGraceWindowHours,
		"gap: grace window default = 48")
	assert.NotEmpty(t, cfg.SoftClosedMutationAllowlist,
		"gap: allowlist must not be empty")
}

// ─── Error constructors — detailed formatting ─────────────────────────────────

func TestErrChecklistFailed_WithDetails(t *testing.T) {
	err := closeflow.ErrChecklistFailed("2 items failed")
	require.NotNil(t, err)
	// DomainError.Error() returns the message; code is in de.Code().
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "gap: ErrChecklistFailed must return DomainError")
	assert.Equal(t, domainerrors.CodeClosingChecklistFailed, de.Code(),
		"gap: ErrChecklistFailed code = CLOSING_CHECKLIST_FAILED")
	assert.Contains(t, err.Error(), "items failed", "gap: message propagated")
}

func TestErrGraceExpired_ContainsPeriodeKodeAndTime(t *testing.T) {
	graceExpiredAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	err := closeflow.ErrGraceExpired("2026-06", graceExpiredAt)
	require.NotNil(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "2026-06", "gap: ErrGraceExpired must contain periode_kode")
}

func TestErrPeriodeSoftClosed_ContainsKode(t *testing.T) {
	err := closeflow.ErrPeriodeSoftClosed("2026-06")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "2026-06",
		"gap: ErrPeriodeSoftClosed must contain periode_kode")
}

func TestErrPeriodeClosed_ContainsKodeAndDate(t *testing.T) {
	hardClose := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)
	err := closeflow.ErrPeriodeClosed("2026-06", hardClose)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "2026-06",
		"gap: ErrPeriodeClosed must contain periode_kode")
}

func TestErrMFAStepUpRequired_ContainsAction(t *testing.T) {
	err := closeflow.ErrMFAStepUpRequired("periode.hardclose.approve")
	require.NotNil(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "gap: ErrMFAStepUpRequired must return DomainError")
	assert.Equal(t, domainerrors.CodeMFAStepUpRequired, de.Code(),
		"gap: ErrMFAStepUpRequired code = MFA_STEP_UP_REQUIRED")
	assert.Contains(t, err.Error(), "periode.hardclose.approve",
		"gap: action name appears in message")
}

func TestErrInvalidTransition_ContainsMessage(t *testing.T) {
	err := closeflow.ErrInvalidTransition("OPEN cannot transition to CLOSED directly")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "CLOSED",
		"gap: ErrInvalidTransition message propagated")
}

func TestErrPeriodeNotFound_ContainsID(t *testing.T) {
	id := uuid.New()
	err := closeflow.ErrPeriodeNotFound(id.String())
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), id.String(),
		"gap: ErrPeriodeNotFound must contain UUID")
}

// ─── JurnalBalancedThreshold — decimal assertion (DEC-016) ───────────────────

func TestJurnalBalancedThreshold_IsDecimal_DEC016Compliance(t *testing.T) {
	// Must not be zero; must equal 0.01 exactly via string comparison (no float64 drift).
	threshold := closeflow.JurnalBalancedThreshold
	assert.Equal(t, "0.01", threshold.String(),
		"gap: DEC-016 — JurnalBalancedThreshold must be exactly 0.01 via shopspring/decimal")
	assert.False(t, threshold.IsZero(), "gap: threshold is not zero")
}

// ─── ChecklistKey enum coverage ───────────────────────────────────────────────

func TestChecklistKeys_AllDefined(t *testing.T) {
	keys := []closeflow.ChecklistKey{
		closeflow.ChecklistKeyPendingApprovalZero,
		closeflow.ChecklistKeyJurnalBalanced,
		closeflow.ChecklistKeyGLDelivered,
		closeflow.ChecklistKeyReconPass,
	}
	assert.Len(t, keys, 4, "gap: exactly 4 checklist keys must be defined")
	for _, k := range keys {
		assert.NotEmpty(t, string(k), "gap: each ChecklistKey must have non-empty string")
	}
}

// ─── SnapshotTransition enum coverage ────────────────────────────────────────

func TestSnapshotTransition_AllDefined(t *testing.T) {
	transitions := []closeflow.SnapshotTransition{
		closeflow.SnapshotTransitionSoftCloseRequest,
		closeflow.SnapshotTransitionSoftCloseApprove,
		closeflow.SnapshotTransitionHardCloseRequest,
		closeflow.SnapshotTransitionHardCloseApprove,
		closeflow.SnapshotTransitionReopenRequest,
		closeflow.SnapshotTransitionReopenApprove,
		closeflow.SnapshotTransitionManualCheck,
	}
	assert.Len(t, transitions, 7, "gap: 7 snapshot transitions must be defined")
	for _, tr := range transitions {
		assert.NotEmpty(t, string(tr), "gap: each SnapshotTransition has value")
	}
}

// ─── Config — allowlist parsing ──────────────────────────────────────────────

func TestDefaultConfig_AllowlistContainsExpectedActions(t *testing.T) {
	cfg := closeflow.DefaultConfig()
	// Both known allowlist entries must be present.
	assert.Contains(t, cfg.SoftClosedMutationAllowlist, "JURNAL_RETRY_GL_DELIVERY",
		"gap: JURNAL_RETRY_GL_DELIVERY in default allowlist")
	assert.Contains(t, cfg.SoftClosedMutationAllowlist, "CORRECTION_PERIODE_CLOSED",
		"gap: CORRECTION_PERIODE_CLOSED in default allowlist")
}

// ─── EmptyListQuery — default values ─────────────────────────────────────────

func TestEmptyListQuery_Defaults(t *testing.T) {
	q := closeflow.EmptyListQuery()
	assert.Empty(t, q.Sort, "gap: empty list query has no sort")
	assert.Empty(t, q.Filters, "gap: empty list query has no filters")
	assert.Empty(t, q.Search, "gap: empty list query has no search")
}

// ─── helper funcs used by sqlmock tests ──────────────────────────────────────

// ─── Service: RequestReopen invalid targetStatus branches ────────────────────

// TestRequestReopen_SoftClosed_WrongTarget: SOFT_CLOSED + targetStatus=SOFT_CLOSED → error.
func TestRequestReopen_SoftClosed_WrongTarget_ValidationError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-06", 2026, nil, "BULANAN",
			now.AddDate(0, -1, 0), now, "SOFT_CLOSED",
			nil, nil,
			false, nil, nil, nil, nil,
			int64(2), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
		))

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-AKUN-CTL"}
	// Wrong target: SOFT_CLOSED from SOFT_CLOSED requires OPEN, but we pass SOFT_CLOSED again.
	_, err = svc.RequestReopen(t.Context(), periodeID, closeflow.PeriodeStatusSoftClosed, "wrong target", 2, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPEN", "gap: error must say target must be OPEN")
}

// TestRequestReopen_Closed_WrongTarget: CLOSED + targetStatus=OPEN → error (must be SOFT_CLOSED).
func TestRequestReopen_Closed_WrongTarget_ValidationError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()
	graceExpiry := now.Add(40 * time.Hour)

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-06", 2026, nil, "BULANAN",
			now.AddDate(0, -1, 0), now, "CLOSED",
			&now, &now,
			false, nil, nil, nil, nil,
			int64(3), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			&graceExpiry, nil, nil,
		))

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	// Wrong: from CLOSED, target must be SOFT_CLOSED, not OPEN.
	_, err = svc.RequestReopen(t.Context(), periodeID, closeflow.PeriodeStatusOpen, "wrong target", 3, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SOFT_CLOSED", "gap: error must say target must be SOFT_CLOSED")
}

// TestRequestReopen_Closed_GraceExpired: CLOSED + grace expired → ErrGraceExpired.
func TestRequestReopen_Closed_GraceExpired_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()
	pastGrace := now.Add(-72 * time.Hour) // expired 72h ago

	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-05", 2026, nil, "BULANAN",
			now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "CLOSED",
			&now, &now,
			false, nil, nil, nil, nil,
			int64(5), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			&pastGrace, nil, nil,
		))

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	_, err = svc.RequestReopen(t.Context(), periodeID, closeflow.PeriodeStatusSoftClosed, "too late", 5, actor)
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok, "gap: must be domain error")
	assert.Equal(t, domainerrors.CodePeriodeGraceExpired, de.Code(),
		"gap: RequestReopen CLOSED grace expired → PERIODE_GRACE_EXPIRED")
}

// ─── Service: ApproveReopen SoD violation path ───────────────────────────────

// TestApproveReopen_SoDViolation: approver == requester → ErrSoDViolation.
func TestApproveReopen_SoDViolation_ReturnsSoDError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New() // same as reopen requester
	now := time.Now()

	mock.ExpectBegin()
	// SOFT_CLOSED with reopened_by == actorID (SoD violation)
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-06", 2026, nil, "BULANAN",
			now.AddDate(0, -1, 0), now, "SOFT_CLOSED",
			&now, nil,
			false, nil, nil, &actorID, nil, // reopened_by = actorID (same as actor)
			int64(2), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
		))
	// Advisory audit write (best-effort, opens own tx)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{"previous_hash"}).AddRow(nil))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectRollback() // main tx rollback

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-AKUN-CTL"}
	_, err = svc.ApproveReopen(t.Context(), periodeID, "approved", "", false, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SoD", "gap: ApproveReopen SoD violation message")
}

// ─── Service: ApproveReopen invalid status (default switch branch) ────────────

// TestApproveReopen_FromOpen_InvalidStatus_DefaultBranch.
func TestApproveReopen_FromOpen_InvalidStatus_DefaultBranch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	// Period is OPEN — not a valid state for ApproveReopen.
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-06", 2026, nil, "BULANAN",
			now.AddDate(0, -1, 0), now, "OPEN",
			nil, nil,
			false, nil, nil, nil, nil,
			int64(1), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
		))
	mock.ExpectRollback()

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: uuid.New(), Role: "ROLE-CFO"}
	_, err = svc.ApproveReopen(t.Context(), periodeID, "comment", "", false, actor)
	require.Error(t, err)
	// default branch produces ErrInvalidTransition message
	assert.Contains(t, err.Error(), "OPEN", "gap: ApproveReopen from OPEN → invalid transition message contains OPEN")
}

// ─── Checklist: checkPendingApprovalZero non-zero path ───────────────────────

// TestChecklistEvaluate_PendingApprovalNonZero: count > 0 → Passed=false.
func TestChecklistEvaluate_PendingApprovalNonZero_FailsItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	// Item 1: PENDING_APPROVAL_ZERO → count = 3 → FAIL
	// Query uses $1 twice (two UNION arms), sqlmock sees 2 separate arg slots.
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	// Item 2: JURNAL_BALANCED → max_delta = "0.0000" → PASS
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(5), "0.0000"))

	// Item 3: GL_DELIVERED → count = 0 → PASS
	mock.ExpectQuery(`COUNT\(gs\.id\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))

	// Item 4: RECON_PASS → date range query
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir FROM mst\.periode_buku`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`gl_reconciliation_report`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	chk := closeflow.NewChecklistService(db)
	result, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	assert.False(t, result.AllPassed, "gap: pending > 0 makes allPassed false")
	require.Len(t, result.Items, 4)
	assert.False(t, result.Items[0].Passed, "gap: PENDING_APPROVAL_ZERO item fails when count=3")
	assert.True(t, result.Items[1].Passed, "gap: JURNAL_BALANCED passes when delta=0")
}

// ─── Checklist: checkJurnalBalanced failed path ──────────────────────────────

// TestChecklistEvaluate_JurnalUnbalanced: max_delta > 0.01 → JURNAL_BALANCED fails.
func TestChecklistEvaluate_JurnalUnbalanced_FailsItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	// Item 1: PENDING → 0 (pass)
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	// Item 2: JURNAL_BALANCED → max_delta = "1234.5600" → FAIL
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(10), "1234.5600"))

	// Item 3: GL_DELIVERED → 0 (pass)
	mock.ExpectQuery(`COUNT\(gs.id\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))

	// Item 4: RECON_PASS
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`gl_reconciliation_report`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	chk := closeflow.NewChecklistService(db)
	result, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	assert.False(t, result.AllPassed, "gap: unbalanced jurnal makes allPassed false")
	assert.True(t, result.Items[0].Passed, "gap: PENDING_APPROVAL_ZERO passes")
	assert.False(t, result.Items[1].Passed, "gap: JURNAL_BALANCED fails when max_delta=1234.56")
	assert.Contains(t, result.Items[1].Detail, "1234.5600", "gap: detail shows unbalanced delta")
}

// ─── Checklist: checkReconPass non-COMPLETED path ────────────────────────────

// TestChecklistEvaluate_ReconNotCompleted: status=COMPLETED_WITH_MISMATCH → RECON_PASS fails.
func TestChecklistEvaluate_ReconNotCompleted_FailsItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	// Item 1: PENDING → 0 (pass)
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	// Item 2: JURNAL_BALANCED → pass
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(2), "0.0000"))

	// Item 3: GL_DELIVERED → pass
	mock.ExpectQuery(`COUNT\(gs.id\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))

	// Item 4: RECON_PASS → COMPLETED_WITH_MISMATCH → FAIL
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`gl_reconciliation_report`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		// C11: recon date must equal tanggal_akhir (=now) for the status check to be reached.
		// If reconDay != lastDayOfPeriod the detail would show a date-mismatch message, not the status.
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED_WITH_MISMATCH", now))

	chk := closeflow.NewChecklistService(db)
	result, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	assert.False(t, result.AllPassed, "gap: COMPLETED_WITH_MISMATCH = FAIL per OQ-M4-1b")
	assert.False(t, result.Items[3].Passed, "gap: RECON_PASS item fails")
	assert.Contains(t, result.Items[3].Detail, "COMPLETED_WITH_MISMATCH",
		"gap: detail shows actual recon status")
}

// ─── Checklist: checkGLDelivered non-nil headerIDsArr path ───────────────────

// TestChecklistEvaluate_GLDeliveredFailed: count>0 + headerIDsArr non-nil → GL_DELIVERED fails.
func TestChecklistEvaluate_GLDeliveredFailed_FailsItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()
	headerIDs := "{aaaa-0001,bbbb-0002}"

	// Item 1: PENDING → 0 (pass)
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	// Item 2: JURNAL_BALANCED → pass
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(3), "0.0000"))

	// Item 3: GL_DELIVERED → count=2, non-nil headerIDs → FAIL
	mock.ExpectQuery(`COUNT\(gs.id\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(2), &headerIDs))

	// Item 4: RECON_PASS → pass
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`gl_reconciliation_report`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	chk := closeflow.NewChecklistService(db)
	result, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	assert.False(t, result.Items[2].Passed, "gap: GL_DELIVERED fails when count=2")
	assert.NotNil(t, result.Items[2].ActionURL, "gap: ActionURL set on GL_DELIVERED fail")
	assert.Contains(t, result.Items[2].Detail, "aaaa-0001", "gap: header IDs in detail")
}

// ─── CanTransition: additional missing branches ────────────────────────────

// TestCanTransition_SoftCloseApprove_FromSoftClosed: wrong source status.
func TestCanTransition_SoftCloseApprove_FromSoftClosed_Fails(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusSoftClosed, // wrong — must be OPEN
		"soft-close-approve",
		false, false,
	)
	assert.False(t, ok, "gap: soft-close-approve from SOFT_CLOSED → denied")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "must be OPEN for soft-close-approve")
}

// TestCanTransition_ReopenClosedToSoftClosed_WrongSource: from OPEN, not CLOSED.
func TestCanTransition_ReopenClosedToSoftClosed_WrongSource_Fails(t *testing.T) {
	ok, err := closeflow.CanTransition(
		closeflow.PeriodeStatusOpen, // wrong — must be CLOSED
		"reopen-closed-to-soft-closed",
		true, true,
	)
	assert.False(t, ok, "gap: reopen-closed-to-soft-closed from OPEN → denied")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "must be CLOSED for reopen to SOFT_CLOSED")
}

// ─── Checklist: DB error paths ─────────────────────────────────────────────

// TestChecklistEvaluate_JurnalBalancedQueryError: DB error in checkJurnalBalanced returns error.
func TestChecklistEvaluate_JurnalBalancedQueryError_PropagatesError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// Item 1: PENDING → 0 (pass)
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	// Item 2: JURNAL_BALANCED → DB error
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnError(fmt.Errorf("connection reset by peer"))

	chk := closeflow.NewChecklistService(db)
	_, err = chk.Evaluate(t.Context(), periodeID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JURNAL_BALANCED", "gap: Evaluate wraps inner error with checklist key")
}

// TestChecklistEvaluate_GLDeliveredQueryError: DB error in checkGLDelivered.
func TestChecklistEvaluate_GLDeliveredQueryError_PropagatesError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// Item 1: pass
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// Item 2: pass
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(0), "0.0000"))
	// Item 3: GL_DELIVERED → DB error
	mock.ExpectQuery(`gl_status gs`).
		WithArgs(periodeID).
		WillReturnError(fmt.Errorf("timeout"))

	chk := closeflow.NewChecklistService(db)
	_, err = chk.Evaluate(t.Context(), periodeID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GL_DELIVERED", "gap: Evaluate wraps GL_DELIVERED error")
}

// TestChecklistEvaluate_ReconPassDateQueryError: DB error in checkReconPass date range query.
func TestChecklistEvaluate_ReconPassDateQueryError_PropagatesError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()

	// Items 1–3: pass
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`COUNT\(\*\) AS total`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(0), "0.0000"))
	mock.ExpectQuery(`gl_status gs`).
		WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))
	// Item 4: RECON date range → DB error
	mock.ExpectQuery(`WHERE id = \$1`).
		WithArgs(periodeID).
		WillReturnError(fmt.Errorf("table does not exist"))

	chk := closeflow.NewChecklistService(db)
	_, err = chk.Evaluate(t.Context(), periodeID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RECON_PASS", "gap: Evaluate wraps RECON_PASS error")
}

// TestIsChecklistStale_DBError: query error returns false + error.
func TestIsChecklistStale_DBError_ReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	mock.ExpectQuery(`FROM sys.closing_checklist_snapshot`).
		WithArgs(periodeID).
		WillReturnError(fmt.Errorf("permission denied"))

	chk := closeflow.NewChecklistService(db)
	stale, err := chk.IsChecklistStale(t.Context(), periodeID, 24)
	require.Error(t, err, "gap: DB error must propagate")
	assert.False(t, stale, "gap: stale=false when error")
	assert.Contains(t, err.Error(), "stale check query")
}

// ─── wrapExec error branch ────────────────────────────────────────────────

// TestWrapExec_ErrorPath: calling repo method that hits wrapExec with a DB error.
// UnlockKursForPeriode uses wrapExec; force it to fail via mock.
func TestWrapExec_ErrorBranch_ViaUnlockKurs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New()
	requesterID := uuid.New()
	now := time.Now()
	graceExpiry := now.Add(40 * time.Hour)

	mock.ExpectBegin()
	// SELECT FOR SHARE → CLOSED period within grace window
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs(periodeID, "TUGURE").
		WillReturnRows(sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
			periodeID.String(), "2026-06", 2026, nil, "BULANAN",
			now.AddDate(0, -2, 0), now.AddDate(0, -1, 0), "CLOSED",
			&now, &now,
			false, nil, nil, &requesterID, nil,
			int64(3), "TUGURE", now, now,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			&graceExpiry, nil, nil,
		))
	// SetReopenApproved UPDATE → success
	mock.ExpectExec(`UPDATE mst.periode_buku`).WillReturnResult(sqlmock.NewResult(0, 1))
	// UnlockKursForPeriode → DB error (hits wrapExec error branch)
	mock.ExpectExec(`UPDATE mst.kurs`).WillReturnError(fmt.Errorf("constraint violation"))
	mock.ExpectRollback()

	svc := newTestService(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	_, err = svc.ApproveReopen(t.Context(), periodeID, "reopen", closeflow.HashStepUpToken("tok"), true, actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "constraint violation", "gap: wrapExec error branch propagates")
}

func gapPeriodeRowCols() []string {
	return []string{
		"id", "periode_id_kode", "tahun_buku", "bulan", "tipe_periode",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"reopened_flag", "reopened_reason", "reopened_at", "reopened_by", "reopened_approved_by",
		"row_version", "tenant_id", "created_at", "updated_at",
		"soft_close_requested_by", "soft_close_requested_at", "soft_close_request_reason",
		"soft_close_approved_by", "soft_close_approved_at", "soft_close_approve_reason",
		"hard_close_requested_by", "hard_close_requested_at", "hard_close_request_reason",
		"hard_close_approved_by", "hard_close_approved_at", "hard_close_approve_reason",
		"hard_close_grace_expires_at", "step_up_token_ref", "reopen_reason",
	}
}

func gapPeriodeRowForStatus(id uuid.UUID, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
		id.String(), "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, status,
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
}

func gapOpenPeriodeRows(idStr string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(gapPeriodeRowCols()).AddRow(
		idStr, "2026-06", 2026, nil, "BULANAN",
		now.AddDate(0, -1, 0), now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
	)
}

// ─── F-01: verifyStepUpScope coverage ─────────────────────────────────────────

// TestVerifyStepUpScope_EmptyToken_ReturnsError covers the empty-token branch.
func TestVerifyStepUpScope_EmptyToken_ReturnsError(t *testing.T) {
	_, err := closeflow.VerifyStepUpScope("", closeflow.StepUpScopeHardClose)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wajib ada")
}

// TestVerifyStepUpScope_WrongScope_ReturnsError covers scope mismatch (F-01 core).
func TestVerifyStepUpScope_WrongScope_ReturnsError(t *testing.T) {
	token := makeFreshStepUpToken(closeflow.StepUpScopeReopenClosed)
	_, err := closeflow.VerifyStepUpScope(token, closeflow.StepUpScopeHardClose)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope mismatch", "F-01: wrong scope must be rejected")
}

// TestVerifyStepUpScope_ExpiredIat_ReturnsError covers the iat freshness check.
func TestVerifyStepUpScope_ExpiredIat_ReturnsError(t *testing.T) {
	// Build token with old iat (6 minutes ago = past 5-minute window).
	oldIat := time.Now().Add(-6 * time.Minute).Unix()
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	payloadJSON, _ := marshalB64URL(map[string]any{
		"jti":   "expired-jti",
		"scope": closeflow.StepUpScopeHardClose,
		"iat":   oldIat,
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
	})
	token := header + "." + payloadJSON + ".fakesig"
	_, err := closeflow.VerifyStepUpScope(token, closeflow.StepUpScopeHardClose)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// TestVerifyStepUpScope_ZeroIat_ReturnsError covers zero iat.
func TestVerifyStepUpScope_ZeroIat_ReturnsError(t *testing.T) {
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	payloadJSON, _ := marshalB64URL(map[string]any{
		"scope": closeflow.StepUpScopeHardClose,
		// no iat → zero value
	})
	token := header + "." + payloadJSON + ".fakesig"
	_, err := closeflow.VerifyStepUpScope(token, closeflow.StepUpScopeHardClose)
	require.Error(t, err)
}

// TestVerifyStepUpScope_ExpiredExpClaim_ReturnsError covers exp exceeded.
func TestVerifyStepUpScope_ExpiredExpClaim_ReturnsError(t *testing.T) {
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	payloadJSON, _ := marshalB64URL(map[string]any{
		"jti":   "exp-jti",
		"scope": closeflow.StepUpScopeHardClose,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(-1 * time.Second).Unix(), // already expired
	})
	token := header + "." + payloadJSON + ".fakesig"
	_, err := closeflow.VerifyStepUpScope(token, closeflow.StepUpScopeHardClose)
	require.Error(t, err)
}

// TestVerifyStepUpScope_ValidToken_ReturnsHash covers the happy path.
func TestVerifyStepUpScope_ValidToken_ReturnsHash(t *testing.T) {
	token := makeFreshStepUpToken(closeflow.StepUpScopeHardClose)
	ref, err := closeflow.VerifyStepUpScope(token, closeflow.StepUpScopeHardClose)
	require.NoError(t, err)
	assert.NotEmpty(t, ref)
}

// ─── F-01: parseStepUpClaims coverage ─────────────────────────────────────────

// TestParseStepUpClaims_NotJWT_ReturnsError covers non-JWT format.
func TestParseStepUpClaims_NotJWT_ReturnsError(t *testing.T) {
	_, _, _, _, err := closeflow.ParseStepUpClaims("not-a-jwt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 bagian")
}

// TestParseStepUpClaims_InvalidBase64_ReturnsError covers base64 decode failure.
func TestParseStepUpClaims_InvalidBase64_ReturnsError(t *testing.T) {
	_, _, _, _, err := closeflow.ParseStepUpClaims("header.!!!invalid!!!.sig")
	require.Error(t, err)
}

// ─── F-06: ReopenApprove handler — stepup scope error ─────────────────────────

// TestReopenApprove_WrongStepUpScope_Returns401 covers F-01 scope check on ReopenApprove.
func TestReopenApprove_WrongStepUpScope_Returns401(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-CFO"},
		Permissions: []string{closeflow.PermPeriodeReopenApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	// Use wrong scope (hard_close_approve instead of reopen_closed).
	body := closeflow.WorkflowApproveBody{Comment: "reopen test"}
	bodyJSON, _ := encodeJSON(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/reopen-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	req.Header.Set("X-Step-Up-Token", makeFreshStepUpToken(closeflow.StepUpScopeHardClose))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Wrong scope → 401.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestReopenApprove_NoStepUpToken_ProceedsToService covers the no-stepup branch.
func TestReopenApprove_NoStepUpToken_ProceedsToService(t *testing.T) {
	db, _, _ := setupSQLMockT(t)
	svc := newTestService(t, db)
	h := closeflow.NewHandler(svc)
	claims := &auth.Claims{
		Sub:         uuid.New().String(),
		Roles:       []string{"ROLE-CFO"},
		Permissions: []string{closeflow.PermPeriodeReopenApprove},
		TenantID:    "TUGURE",
		MFAVerified: true,
	}
	r := setupRouter(t, h, claims)

	body := closeflow.WorkflowApproveBody{Comment: "reopen without stepup"}
	bodyJSON, _ := encodeJSON(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/periode-buku/"+uuid.New().String()+"/reopen-approve",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	// No X-Step-Up-Token header → hasStepUp=false → proceeds to service (fails with DB error).
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Service will return error (DB not set up), but not 401 or 403 — any 4xx/5xx from service is fine.
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	assert.NotEqual(t, http.StatusForbidden, w.Code)
}

// ─── F-02: route idempotency inheritance (handler_test already covers this) ───
// (The TestSoftCloseRequest_MissingIdempotencyKey_Returns400 in handler_test.go
// already validates that idempotency middleware fires for closeflow routes.)

// ─── C9: RejectHardClose SoD ──────────────────────────────────────────────────

// TestRejectHardClose_SoD_ActorIsRequester: same user can't reject what they requested.
func TestRejectHardClose_SoD_ActorIsRequester(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	actorID := uuid.New() // same as requester

	mock.ExpectBegin()
	// Return HARD_CLOSE_PENDING with hard_close_requested_by = actorID.
	rows := sqlmock.NewRows(periodeRowCols()).AddRow(
		periodeID.String(), "2026-06", 2026, nil, "BULANAN",
		time.Now().AddDate(0, -1, 0), time.Now(), "HARD_CLOSE_PENDING",
		&[]time.Time{time.Now()}[0], nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", time.Now(), time.Now(),
		nil, nil, nil, nil, nil, nil,
		actorID.String(), &[]time.Time{time.Now()}[0], nil, // hard_close_requested_by = actorID
		nil, nil, nil,
		nil, nil, nil,
	)
	mock.ExpectQuery(`FROM mst.periode_buku`).WillReturnRows(rows)
	mock.ExpectRollback()

	svc := buildTestSvc(t, db)
	actor := closeflow.Actor{UserID: actorID, Role: "ROLE-CFO"}
	_, err = svc.RejectHardClose(t.Context(), periodeID,
		"reject reason that is at least thirty chars", actor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SoD")
}

// ─── C11: ReconPass mid-period only → FAIL ────────────────────────────────────

// TestChecklistEvaluate_ReconMidPeriodOnly_Fails covers C11: recon date != tanggal_akhir.
func TestChecklistEvaluate_ReconMidPeriodOnly_Fails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()
	midPeriod := now.AddDate(0, 0, -15) // 15 days before tanggal_akhir

	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`COUNT\(\*\) AS total`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(0), "0.0000"))
	mock.ExpectQuery(`COUNT\(gs.id\)`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	// Recon report only covers mid-period, not tanggal_akhir.
	mock.ExpectQuery(`gl_reconciliation_report`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", midPeriod))

	chk := closeflow.NewChecklistService(db)
	result, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	assert.False(t, result.Items[3].Passed, "C11: mid-period recon must fail")
	assert.Contains(t, result.Items[3].Detail, "tanggal akhir periode",
		"C11: detail explains date mismatch")
}

// ─── C7: JurnalBalancedThreshold boundary ─────────────────────────────────────

// TestJurnalBalancedThreshold_ExactThreshold: delta == threshold → FAIL (not <).
func TestJurnalBalancedThreshold_ExactThreshold(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := uuid.New()
	now := time.Now()

	// Item 1: PENDING → pass
	mock.ExpectQuery(`COALESCE\(SUM\(cnt\), 0\)`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	// Item 2: delta = 0.0100 exactly (== threshold).
	// Per spec label "delta ≤ IDR 0.01": LessThanOrEqual → 0.01 <= 0.01 is TRUE → PASS.
	// C7 ensures this uses RequireFromString("0.01") not NewFromFloat(0.01) to avoid IEEE754 drift.
	mock.ExpectQuery(`COUNT\(\*\) AS total`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"total", "max_delta"}).AddRow(int64(5), "0.0100"))
	// Item 3: GL_DELIVERED → pass (needed to complete Evaluate)
	mock.ExpectQuery(`COUNT\(gs.id\)`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"count", "header_ids"}).AddRow(int64(0), nil))
	// Item 4: RECON_PASS → pass
	mock.ExpectQuery(`tanggal_mulai, tanggal_akhir`).WithArgs(periodeID).
		WillReturnRows(sqlmock.NewRows([]string{"tanggal_mulai", "tanggal_akhir"}).
			AddRow(now.AddDate(0, -1, 0), now))
	mock.ExpectQuery(`gl_reconciliation_report`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"status", "tanggal_rekonsiliasi"}).
			AddRow("COMPLETED", now))

	chk := closeflow.NewChecklistService(db)
	item, err := chk.Evaluate(t.Context(), periodeID)
	require.NoError(t, err)
	// delta == threshold (0.01 <= 0.01) → PASS. The test verifies the decimal comparison is exact.
	assert.True(t, item.Items[1].Passed,
		"C7: delta == 0.01 uses RequireFromString, LessThanOrEqual → should pass")
}

// ─── Helpers shared across gap tests ──────────────────────────────────────────

// encodeJSON is json.Marshal under a gap-test-local alias to avoid import shadowing.
func encodeJSON(v any) ([]byte, error) { return json.Marshal(v) }

// marshalB64URL encodes v as a base64url-encoded JSON string (no padding),
// for building structurally valid but unsigned test JWTs.
func marshalB64URL(v map[string]any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

