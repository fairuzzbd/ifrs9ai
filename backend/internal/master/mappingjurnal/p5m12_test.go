package mappingjurnal

// p5m12_test.go — Unit tests for P5-M12 state machine, SoD, validator, regulated flag.
//
// Covers:
//   - State transition contract (state matrix)
//   - SoD 4-way enforcement (every combination)
//   - Regulated flag (config + fallback)
//   - Balance validator (D/K counts)
//   - Details not-empty validator
//   - signatureInputString determinism
//   - Period lock error code
//   - Helper functions: mapDKIndicator, parseInt, uuidPtrToStr, joinStr

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── State matrix tests ───────────────────────────────────────────────────────

func TestWorkflowStatus_IsEditable(t *testing.T) {
	assert.True(t, WorkflowStatusDraft.IsEditable(), "DRAFT should be editable")
	assert.True(t, WorkflowStatusRejected.IsEditable(), "REJECTED should be editable")
	assert.True(t, WorkflowStatusReturned.IsEditable(), "RETURNED should be editable")
	assert.False(t, WorkflowStatusPendingReview.IsEditable(), "PENDING_REVIEW should not be editable")
	assert.False(t, WorkflowStatusPendingApproval.IsEditable(), "PENDING_APPROVAL should not be editable")
	assert.False(t, StatusPendingApproval2.IsEditable(), "PENDING_APPROVAL_2 should not be editable")
	assert.False(t, WorkflowStatusApproved.IsEditable(), "APPROVED should not be editable")
	assert.False(t, StatusApprovedActive.IsEditable(), "APPROVED_ACTIVE should not be editable")
}

func TestStatusAliases(t *testing.T) {
	assert.Equal(t, WorkflowStatus("APPROVED_ACTIVE"), StatusApprovedActive)
	assert.Equal(t, WorkflowStatus("PENDING_APPROVAL_2"), StatusPendingApproval2)
}

// ─── SoD 4-way enforcement ────────────────────────────────────────────────────

func TestValidateSoD4Way_Review_BlockMakerAsReviewer(t *testing.T) {
	makerID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, nil, nil, nil, makerID, "review")
	require.Error(t, err, "maker cannot be reviewer")
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestValidateSoD4Way_Review_AllowDifferentUser(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, nil, nil, nil, reviewerID, "review")
	assert.NoError(t, err)
}

func TestValidateSoD4Way_Approve_BlockMakerAsApprover(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, &reviewerID, nil, nil, makerID, "approve")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestValidateSoD4Way_Approve_BlockReviewerAsApprover(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, &reviewerID, nil, nil, reviewerID, "approve")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestValidateSoD4Way_Approve_AllowFourthUser(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	approverID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, &reviewerID, nil, nil, approverID, "approve")
	assert.NoError(t, err)
}

func TestValidateSoD4Way_Approve2_BlockAllPrior(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	approverID := uuid.New().String()

	// Block maker as approver-2
	err := ValidateSoD4Way(&makerID, &reviewerID, &approverID, nil, makerID, "approve-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)

	// Block reviewer as approver-2
	err = ValidateSoD4Way(&makerID, &reviewerID, &approverID, nil, reviewerID, "approve-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)

	// Block approver as approver-2
	err = ValidateSoD4Way(&makerID, &reviewerID, &approverID, nil, approverID, "approve-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestValidateSoD4Way_Approve2_AllowFifthUser(t *testing.T) {
	makerID := uuid.New().String()
	reviewerID := uuid.New().String()
	approverID := uuid.New().String()
	approver2ID := uuid.New().String()
	err := ValidateSoD4Way(&makerID, &reviewerID, &approverID, nil, approver2ID, "approve-2")
	assert.NoError(t, err)
}

func TestValidateSoD4Way_NilMaker_Review(t *testing.T) {
	// If maker not yet set, allow any reviewer
	anyUser := uuid.New().String()
	err := ValidateSoD4Way(nil, nil, nil, nil, anyUser, "review")
	assert.NoError(t, err)
}

// ─── Regulated flag ───────────────────────────────────────────────────────────

func TestIsRegulatedFallback_KnownCodes(t *testing.T) {
	regulatedCodes := []string{
		"ECL_PEMBENTUKAN", "ECL_REVERSAL", "POCI_DELTA_ECL",
		"MTM_FVTPL", "MTM_FVOCI", "MTM_FVOCI_ELECTION",
		"REKLAS_OCI_PL", "REKLASIFIKASI_AC_FVOCI", "REKLASIFIKASI_FVOCI_AC",
		"MODIFIKASI_MATERIAL", "EIR_CATCH_UP_ADJUSTMENT", "STAGE_MIGRATION", "FX_UNREALIZED",
	}
	for _, code := range regulatedCodes {
		assert.True(t, isRegulatedFallback(code), "expected regulated: %s", code)
	}
}

func TestIsRegulatedFallback_UnknownCode_False(t *testing.T) {
	assert.False(t, isRegulatedFallback("BIAYA_ADMIN"))
	assert.False(t, isRegulatedFallback("BUNGA_DEPOSITO"))
	assert.False(t, isRegulatedFallback(""))
}

// ─── Balance validator ────────────────────────────────────────────────────────

func TestValidateBalance_Balanced(t *testing.T) {
	details := []AkunDetail{
		{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D"},
		{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "K"},
	}
	assert.NoError(t, ValidateBalance(details))
}

func TestValidateBalance_MoreDebitThanKredit(t *testing.T) {
	details := []AkunDetail{
		{DebitKredit: "D"},
		{DebitKredit: "D"},
		{DebitKredit: "K"},
	}
	err := ValidateBalance(details)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingUnbalanced)
}

func TestValidateBalance_OnlyDebit_Unbalanced(t *testing.T) {
	details := []AkunDetail{
		{DebitKredit: "D"},
		{DebitKredit: "D"},
	}
	err := ValidateBalance(details)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingUnbalanced)
}

func TestValidateBalance_Empty_Unbalanced(t *testing.T) {
	err := ValidateBalance(nil)
	require.Error(t, err)
}

func TestValidateBalance_MultipleBalanced(t *testing.T) {
	details := []AkunDetail{
		{DebitKredit: "D"},
		{DebitKredit: "D"},
		{DebitKredit: "K"},
		{DebitKredit: "K"},
	}
	assert.NoError(t, ValidateBalance(details))
}

// ─── Details not-empty validator ──────────────────────────────────────────────

func TestValidateDetailsNotEmpty_AllFilled(t *testing.T) {
	details := []AkunDetail{
		{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D"},
	}
	assert.NoError(t, ValidateDetailsNotEmpty(details))
}

func TestValidateDetailsNotEmpty_EmptyAkunDebit(t *testing.T) {
	details := []AkunDetail{
		{AkunDebit: "", AkunKredit: "2001", DebitKredit: "D"},
	}
	err := ValidateDetailsNotEmpty(details)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingAkunInvalid)
	assert.Contains(t, err.Error(), "akun_debit")
}

func TestValidateDetailsNotEmpty_EmptyAkunKredit(t *testing.T) {
	details := []AkunDetail{
		{AkunDebit: "1001", AkunKredit: "", DebitKredit: "K"},
	}
	err := ValidateDetailsNotEmpty(details)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingAkunInvalid)
	assert.Contains(t, err.Error(), "akun_kredit")
}

func TestValidateDetailsNotEmpty_Empty_Slice(t *testing.T) {
	assert.NoError(t, ValidateDetailsNotEmpty(nil)) // empty slice = no details, not empty-field error
}

// ─── signatureInputString ─────────────────────────────────────────────────────

func TestSignatureInputString_Deterministic(t *testing.T) {
	actor := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	entity := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	at := time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)

	s1 := signatureInputString(actor, "MAPPING.REVIEW", entity, at, "Sudah sesuai SOP")
	s2 := signatureInputString(actor, "MAPPING.REVIEW", entity, at, "Sudah sesuai SOP")
	assert.Equal(t, s1, s2, "signatureInputString must be deterministic")
}

func TestSignatureInputString_DifferentAction(t *testing.T) {
	actor := uuid.New()
	entity := uuid.New()
	at := time.Now()

	s1 := signatureInputString(actor, "MAPPING.REVIEW", entity, at, "comment")
	s2 := signatureInputString(actor, "MAPPING.APPROVE", entity, at, "comment")
	assert.NotEqual(t, s1, s2, "different action must produce different input")
}

// ─── computeSHA256 ────────────────────────────────────────────────────────────

func TestComputeSHA256_NonEmpty(t *testing.T) {
	h := computeSHA256("test")
	assert.Len(t, h, 32, "SHA-256 must produce 32 bytes")
}

func TestComputeSHA256_Deterministic(t *testing.T) {
	h1 := computeSHA256("hello world")
	h2 := computeSHA256("hello world")
	assert.Equal(t, h1, h2)
}

func TestComputeSHA256_DifferentInput(t *testing.T) {
	h1 := computeSHA256("abc")
	h2 := computeSHA256("abd")
	assert.NotEqual(t, h1, h2)
}

// ─── Helper functions ─────────────────────────────────────────────────────────

func TestMapDKIndicator(t *testing.T) {
	assert.Equal(t, "D", mapDKIndicator("D"))
	assert.Equal(t, "D", mapDKIndicator("DEBIT"))
	assert.Equal(t, "D", mapDKIndicator("debit"))
	assert.Equal(t, "K", mapDKIndicator("K"))
	assert.Equal(t, "K", mapDKIndicator("KREDIT"))
	assert.Equal(t, "K", mapDKIndicator("kredit"))
	assert.Equal(t, "X", mapDKIndicator("X")) // pass-through unknown
}

func TestParseInt(t *testing.T) {
	assert.Equal(t, 5, parseInt("5", 1))
	assert.Equal(t, 42, parseInt("42", 1))
	assert.Equal(t, 1, parseInt("", 1))
	assert.Equal(t, 1, parseInt("abc", 1))
	assert.Equal(t, 1, parseInt("1.5", 1))
}

func TestUUIDPtrToStr(t *testing.T) {
	assert.Equal(t, "", uuidPtrToStr(nil))
	u := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", uuidPtrToStr(&u))
}

func TestJoinStr(t *testing.T) {
	assert.Equal(t, "a; b; c", joinStr([]string{"a", "b", "c"}, "; "))
	assert.Equal(t, "only", joinStr([]string{"only"}, "; "))
	assert.Equal(t, "", joinStr(nil, "; "))
}

func TestStringPtr(t *testing.T) {
	s := stringPtr("hello")
	require.NotNil(t, s)
	assert.Equal(t, "hello", *s)
}

// ─── Error code constants ─────────────────────────────────────────────────────

func TestErrorCodeConstants_NotEmpty(t *testing.T) {
	codes := []string{
		CodeMappingEventNotFound,
		CodeMappingAkunInvalid,
		CodeMappingUnbalanced,
		CodeMappingRegulatedRequiresRisk,
		CodeMappingDuplicateVersion,
		CodeMappingSoDViolation,
		CodeMappingPeriodeLocked,
	}
	for _, c := range codes {
		assert.NotEmpty(t, c, "error code must not be empty")
	}
}

// ─── CoverageStatus constants ─────────────────────────────────────────────────

func TestCoverageStatus_Values(t *testing.T) {
	assert.Equal(t, CoverageStatusP5("OK"), CoverageStatusOK)
	assert.Equal(t, CoverageStatusP5("MISSING"), CoverageStatusMissing)
	assert.Equal(t, CoverageStatusP5("INCOMPLETE"), CoverageStatusIncomplete)
}
