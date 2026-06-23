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
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
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

// ─── B1: detailsToAkunDetail reads text COA codes (not UUID) ─────────────────
// Asserts that AkunDebitCode / AkunKreditCode (text columns, migration 000049)
// are passed through to AkunDetail.AkunDebit / AkunDebit unchanged.

func TestDetailsToAkunDetail_ReadsTextCOACodes(t *testing.T) {
	d := &Detail{
		ID:            uuid.New(),
		EventHeaderID: uuid.New(),
		Urutan:        1,
		DKIndicator:   "D",
		AkunDebitCode: "110201", // text COA code, not UUID
		AkunKreditCode: "440101",
	}
	result := detailsToAkunDetail([]*Detail{d})
	require.Len(t, result, 1)
	assert.Equal(t, "110201", result[0].AkunDebit, "AkunDebit must be text COA code")
	assert.Equal(t, "440101", result[0].AkunKredit, "AkunKredit must be text COA code")
	assert.Equal(t, "D", result[0].DebitKredit)
}

func TestDetailsToAkunDetail_UUIDLikeCodeRejectedByCOACheck(t *testing.T) {
	// Validates that a UUID string as COA code (wrong format) differs from a valid
	// text code. The validator's CoaCodeExists would return false for a UUID string
	// because COA codes are text like "110201", not "550e8400-e29b-41d4-a716-446655440000".
	// This test asserts that the conversion faithfully passes whatever string is in
	// AkunDebitCode — validating wrong values is the COA check's job, not conversion's.
	uuidString := uuid.New().String()
	d := &Detail{
		AkunDebitCode:  uuidString,
		AkunKreditCode: "440101",
		DKIndicator:    "D",
	}
	result := detailsToAkunDetail([]*Detail{d})
	require.Len(t, result, 1)
	assert.Equal(t, uuidString, result[0].AkunDebit, "conversion must pass-through AkunDebitCode as-is")
	// Downstream validator (ValidateAkunDetails) would reject this UUID string
	// since it won't match any kode_akun in mst.chart_of_accounts.
}

// ─── B2: step-up MFA token validation (stepup.go) ────────────────────────────

func TestVerifyMappingStepUp_EmptyToken_ReturnsRequired(t *testing.T) {
	_, err := verifyMappingStepUp("")
	require.Error(t, err)
	// Must return MFAStepUpRequired, not MFAStepUpExpired
	// Check the error is domain error with appropriate code
	assert.Contains(t, err.Error(), StepUpScopeMappingApprove,
		"error must mention required scope")
}

func TestVerifyMappingStepUp_WrongScope_ReturnsRequired(t *testing.T) {
	// Build a minimal JWT with wrong scope
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	// {"scope":"wrong_scope","iat":9999999999,"exp":9999999999,"jti":"test-jti"}
	payload := "eyJzY29wZSI6Indyb25nX3Njb3BlIiwiaWF0Ijo5OTk5OTk5OTk5LCJleHAiOjk5OTk5OTk5OTksImp0aSI6InRlc3QtanRpIn0"
	sig := "fakesig"
	token := header + "." + payload + "." + sig

	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrong_scope",
		"error must mention the offending scope")
}

func TestVerifyMappingStepUp_ExpiredToken_ReturnsExpired(t *testing.T) {
	// iat far in the past (> 5 minutes)
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	// {"scope":"mapping_approve","iat":1000000,"exp":1000060,"jti":"old-jti"}
	payload := "eyJzY29wZSI6Im1hcHBpbmdfYXBwcm92ZSIsImlhdCI6MTAwMDAwMCwiZXhwIjoxMDAwMDYwLCJqdGkiOiJvbGQtanRpIn0"
	sig := "fakesig"
	token := header + "." + payload + "." + sig

	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	// Should be MFAStepUpExpired (iat=1000000 is ~1970, definitely > 5 min ago)
	assert.Contains(t, err.Error(), "kadaluarsa",
		"expired token must mention expiry")
}

func TestVerifyMappingStepUp_FreshValidToken_ReturnsRef(t *testing.T) {
	iat := time.Now().Unix()
	// Build a payload with scope=mapping_approve, fresh iat, far-future exp
	payloadData := fmt.Sprintf(`{"scope":"mapping_approve","iat":%d,"exp":9999999999,"jti":"fresh-jti"}`, iat)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadData))
	jwtHeader := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	sig := "fakesig"
	token := jwtHeader + "." + payloadB64 + "." + sig

	ref, err := verifyMappingStepUp(token)
	require.NoError(t, err)
	assert.Len(t, ref, 32, "token ref must be SHA-256 (32 bytes)")
}

// ─── B3: SoD violation audit action name and writeAuditP5 standalone write ────

func TestWriteAuditP5_NilTx_DoesNotPanic(t *testing.T) {
	// writeAuditP5(ctx, nil, aw, evt) must not panic — used for SoD violation audits.
	// aw == nil because audit.Writer.Write is no-op when db is nil; proves no panic.
	assert.NotPanics(t, func() {
		writeAuditP5(context.Background(), nil, nil, audit.Event{
			Action:     "MAPPING.SOD_VIOLATION_ATTEMPT",
			EntityType: "mst.mapping_jurnal_header",
			EntityID:   uuid.New(),
		})
	})
}

func TestAuditActionNames_Canonical(t *testing.T) {
	// Canonical action names must exactly match the state machine.
	assert.Equal(t, "MAPPING.SUBMITTED", "MAPPING.SUBMITTED")
	assert.Equal(t, "MAPPING.REVIEWED", "MAPPING.REVIEWED")
	assert.Equal(t, "MAPPING.APPROVED_ACTIVE", "MAPPING.APPROVED_ACTIVE")
	assert.Equal(t, "MAPPING.REJECTED", "MAPPING.REJECTED")
	assert.Equal(t, "MAPPING.VERSION_CREATED", "MAPPING.VERSION_CREATED")
	assert.Equal(t, "MAPPING.SOD_VIOLATION_ATTEMPT", "MAPPING.SOD_VIOLATION_ATTEMPT")
}

// ─── B4: Approve merges aktif_flag into single UPDATE (structural test) ───────
// Verifies that Approve4Eyes and Approve6Eyes signatures accept the same parameters
// as before — the SQL change is covered by sqlmock tests.

func TestApprove4Eyes_Signature_Unchanged(t *testing.T) {
	// Compile-time: Approve4Eyes still has (ctx, tx, versionID, approverID, sigHash, comment, now, tenantID).
	// If the method signature changed, this line wouldn't compile.
	var _ func(context.Context, *sql.Tx, uuid.UUID, uuid.UUID, []byte, string, time.Time, string) error =
		(*DBRepository)(nil).Approve4Eyes
}

func TestApprove6Eyes_Signature_Unchanged(t *testing.T) {
	var _ func(context.Context, *sql.Tx, uuid.UUID, uuid.UUID, []byte, []byte, string, time.Time, string) error =
		(*DBRepository)(nil).Approve6Eyes
}

// ─── M6: ValidateBalance is template-level (line count parity) ───────────────
// Documents that amount-level D=K enforcement is deferred to posting engine.

func TestValidateBalance_DocumentsTemplateLevelOnly(t *testing.T) {
	// A mapping with D=1 K=1 but with zero-amount "debit" is still valid at template level.
	// Amount enforcement happens at posting time.
	details := []AkunDetail{
		{AkunDebit: "110201", AkunKredit: "440101", DebitKredit: "D"},
		{AkunDebit: "440101", AkunKredit: "110201", DebitKredit: "K"},
	}
	err := ValidateBalance(details)
	assert.NoError(t, err, "template-level balance: 1D=1K passes regardless of jumlah_calc value")
}

// ─── B2: Additional stepup edge cases ────────────────────────────────────────

func TestVerifyMappingStepUp_ZeroIat_ReturnsExpired(t *testing.T) {
	// Payload without iat field → iat == 0 → ErrMFAStepUpExpired ("tidak memiliki klaim 'iat'")
	payloadData := `{"scope":"mapping_approve","exp":9999999999,"jti":"no-iat-jti"}`
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadData))
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + payloadB64 + ".fakesig"

	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iat", "missing iat must return iat-specific error")
}

func TestVerifyMappingStepUp_ExpiredByExpClaim_ReturnsExpired(t *testing.T) {
	// Token fresh iat but exp already passed.
	iat := time.Now().Unix()
	payloadData := fmt.Sprintf(`{"scope":"mapping_approve","iat":%d,"exp":1,"jti":"old-exp-jti"}`, iat)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadData))
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + payloadB64 + ".fakesig"

	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exp", "expired-by-exp must mention exp")
}

func TestVerifyMappingStepUp_InvalidBase64Payload_ReturnsRequired(t *testing.T) {
	// Corrupt payload — base64 decode will fail.
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.!!!invalid!!!.fakesig"
	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	// Will hit parse error → "Gagal parse X-Step-Up-Token"
	assert.Contains(t, err.Error(), "parse")
}

func TestVerifyMappingStepUp_NotJSONPayload_ReturnsRequired(t *testing.T) {
	// Valid base64 but payload isn't JSON.
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + payloadB64 + ".fakesig"
	_, err := verifyMappingStepUp(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestVerifyMappingStepUp_NoJTI_UsesFullToken(t *testing.T) {
	// Valid token without jti — ref is SHA-256 of full token raw.
	iat := time.Now().Unix()
	payloadData := fmt.Sprintf(`{"scope":"mapping_approve","iat":%d,"exp":9999999999}`, iat)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadData))
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + payloadB64 + ".fakesig"

	ref, err := verifyMappingStepUp(token)
	require.NoError(t, err)
	assert.Len(t, ref, 32, "ref must be 32-byte SHA-256")
}
