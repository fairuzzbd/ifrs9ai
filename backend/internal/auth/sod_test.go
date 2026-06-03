package auth_test

import (
	"testing"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

const (
	makerID    = "11111111-1111-1111-1111-111111111111"
	reviewerID = "22222222-2222-2222-2222-222222222222"
	approverID = "33333333-3333-3333-3333-333333333333"
	other1ID   = "44444444-4444-4444-4444-444444444444"
	other2ID   = "55555555-5555-5555-5555-555555555555"
)

// TestSoD_Review verifies maker cannot be reviewer.
func TestSoD_Review_MakerCannotBeReviewer(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID: makerID,
	}

	// Maker trying to be reviewer → SOD_VIOLATION.
	err := auth.EnforceSoD(participants, makerID, auth.SoDStepReview)
	if err == nil {
		t.Fatal("expected SOD_VIOLATION error, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeSoDViolation {
		t.Errorf("expected SOD_VIOLATION, got %s", de.Code())
	}
}

// TestSoD_Review_DifferentUserOK verifies different user can review.
func TestSoD_Review_DifferentUserOK(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID: makerID,
	}
	err := auth.EnforceSoD(participants, reviewerID, auth.SoDStepReview)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestSoD_Approve_MakerCannotBeApprover verifies maker cannot approve.
func TestSoD_Approve_MakerCannotBeApprover(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID:    makerID,
		ReviewerID: reviewerID,
	}
	err := auth.EnforceSoD(participants, makerID, auth.SoDStepApprove)
	if err == nil {
		t.Fatal("expected SOD_VIOLATION error, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if de.Code() != domainerrors.CodeSoDViolation {
		t.Errorf("expected SOD_VIOLATION, got %s", de.Code())
	}
}

// TestSoD_Approve_ReviewerCannotBeApprover verifies reviewer cannot approve.
func TestSoD_Approve_ReviewerCannotBeApprover(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID:    makerID,
		ReviewerID: reviewerID,
	}
	err := auth.EnforceSoD(participants, reviewerID, auth.SoDStepApprove)
	if err == nil {
		t.Fatal("expected SOD_VIOLATION error, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if de.Code() != domainerrors.CodeSoDViolation {
		t.Errorf("expected SOD_VIOLATION, got %s", de.Code())
	}
}

// TestSoD_Approve_ThirdPartyOK verifies third user can approve.
func TestSoD_Approve_ThirdPartyOK(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID:    makerID,
		ReviewerID: reviewerID,
	}
	err := auth.EnforceSoD(participants, approverID, auth.SoDStepApprove)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestSoD_Approve2_SixEyes_AllBlocked verifies all previous participants blocked from approve2.
func TestSoD_Approve2_SixEyes_AllBlocked(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID:    makerID,
		ReviewerID: reviewerID,
		ApproverID: approverID,
	}

	blocked := []struct {
		name   string
		userID string
		wantCode domainerrors.Code
	}{
		{"maker", makerID, domainerrors.CodeSoDApprover1SameAsMaker},
		{"reviewer", reviewerID, domainerrors.CodeSoDApprover2SameAsReviewer},
		{"approver", approverID, domainerrors.CodeSoDViolation},
	}

	for _, tc := range blocked {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.EnforceSoD(participants, tc.userID, auth.SoDStepApprove2)
			if err == nil {
				t.Fatalf("expected error for %s as approve2, got nil", tc.name)
			}
			de, ok := domainerrors.IsDomainError(err)
			if !ok {
				t.Fatalf("expected DomainError, got %T", err)
			}
			if de.Code() != tc.wantCode {
				t.Errorf("expected %s, got %s", tc.wantCode, de.Code())
			}
		})
	}
}

// TestSoD_Approve2_FourthPartyOK verifies fourth user can be approve2.
func TestSoD_Approve2_FourthPartyOK(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID:    makerID,
		ReviewerID: reviewerID,
		ApproverID: approverID,
	}
	err := auth.EnforceSoD(participants, other1ID, auth.SoDStepApprove2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestSoD_EmptyParticipants verifies no error when participants are empty.
func TestSoD_EmptyParticipants(t *testing.T) {
	participants := auth.WorkflowParticipants{}
	for _, step := range []auth.SoDStep{auth.SoDStepReview, auth.SoDStepApprove, auth.SoDStepApprove2} {
		err := auth.EnforceSoD(participants, makerID, step)
		if err != nil {
			t.Errorf("step %s: expected no error for empty participants, got %v", step, err)
		}
	}
}

// TestMustEnforceSoD_WrapsError verifies MustEnforceSoD wraps the error.
func TestMustEnforceSoD_WrapsError(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID: makerID,
	}
	err := auth.MustEnforceSoD(participants, makerID, auth.SoDStepReview)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
}

// TestMustEnforceSoD_NoError verifies MustEnforceSoD returns nil when OK.
func TestMustEnforceSoD_NoError(t *testing.T) {
	participants := auth.WorkflowParticipants{
		MakerID: makerID,
	}
	err := auth.MustEnforceSoD(participants, reviewerID, auth.SoDStepReview)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestSoD_NoRoleStringComparison verifies the implementation uses permission/ID checks,
// not role strings. This test documents the anti-pattern that is explicitly FORBIDDEN.
func TestSoD_NoRoleStringComparison(t *testing.T) {
	// The SoD check should be based on user IDs (UUIDs), not role names.
	// If a user has role "ROLE-APPR-TR" but is the same person as maker, still blocked.
	// This test verifies the ID-based check works regardless of roles.
	participants := auth.WorkflowParticipants{
		MakerID: makerID,
	}
	// Same user ID trying to review — blocked even if they have "review" role.
	err := auth.EnforceSoD(participants, makerID, auth.SoDStepReview)
	if err == nil {
		t.Fatal("SoD must block based on user ID, not role name")
	}
}
