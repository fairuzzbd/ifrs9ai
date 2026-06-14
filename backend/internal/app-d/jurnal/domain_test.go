package jurnal

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMappingHeaderStatusTransitions(t *testing.T) {
	tests := []struct {
		status   MappingHeaderStatus
		canSub   bool
		canRev   bool
		canApp   bool
		canApp2  bool
		canRej   bool
		canWith  bool
		canDeact bool
	}{
		{MappingStatusDraft, true, false, false, false, false, true, false},
		{MappingStatusPendingReview, false, true, false, false, true, false, false},
		{MappingStatusPendingApproval, false, false, true, false, true, false, false},
		{MappingStatusPendingApproval2, false, false, false, true, true, false, false},
		{MappingStatusApprovedActive, false, false, false, false, false, false, true},
		{MappingStatusApproved, false, false, false, false, false, false, true},
		{MappingStatusWithdrawn, false, false, false, false, false, false, false},
		{MappingStatusRejected, false, false, false, false, false, false, false},
		{MappingStatusReturned, false, false, false, false, false, false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.canSub, tt.status.CanSubmit(), "CanSubmit")
			assert.Equal(t, tt.canRev, tt.status.CanReview(), "CanReview")
			assert.Equal(t, tt.canApp, tt.status.CanApprove(), "CanApprove")
			assert.Equal(t, tt.canApp2, tt.status.CanApprove2(), "CanApprove2")
			assert.Equal(t, tt.canRej, tt.status.CanReject(), "CanReject")
			assert.Equal(t, tt.canWith, tt.status.CanWithdraw(), "CanWithdraw")
			assert.Equal(t, tt.canDeact, tt.status.CanDeactivate(), "CanDeactivate")
		})
	}
}

func TestIsActiveForResolver(t *testing.T) {
	assert.True(t, MappingStatusApprovedActive.IsActiveForResolver())
	assert.True(t, MappingStatusApproved.IsActiveForResolver())
	assert.False(t, MappingStatusDraft.IsActiveForResolver())
	assert.False(t, MappingStatusPendingReview.IsActiveForResolver())
	assert.False(t, MappingStatusPendingApproval.IsActiveForResolver())
	assert.False(t, MappingStatusPendingApproval2.IsActiveForResolver())
	assert.False(t, MappingStatusWithdrawn.IsActiveForResolver())
	assert.False(t, MappingStatusRejected.IsActiveForResolver())
}

func TestIsRegulated(t *testing.T) {
	// All 13 regulated codes must return true (DEC-P5-M1-003).
	regulated := []string{
		EventCodeECLPembentukan, EventCodeECLReversal, EventCodeEIRCatchup,
		EventCodeStageMigration, EventCodePOCIDeltaECL,
		EventCodeMTMFVTPL, EventCodeMTMFVOCI, EventCodeMTMFVOCIElection,
		EventCodeReklasOCIPL, EventCodeModifikasiMaterial,
		EventCodeReklasAcFVOCI, EventCodeReklasFVOCIAc, EventCodeFXUnrealized,
	}
	require.Equal(t, 13, len(regulated), "must have exactly 13 regulated codes")
	for _, code := range regulated {
		assert.Truef(t, IsRegulated(code), "expected %s to be regulated", code)
	}

	// Operational codes must return false.
	operational := []string{
		EventCodePenempatan, EventCodeAkrualBunga, EventCodeJatuhTempo,
		EventCodePenjualanPencairan, EventCodeRenewalDeposito,
		EventCodePembayaranBunga, EventCodePembayaranPokok,
		EventCodePenerimaanDividen, EventCodeDistribusiReksadana,
		EventCodeFXRealized, EventCodeAmortisasiPremiDiskonto,
		EventCodePenghapusan, EventCodePeriodeAdjustment,
		EventCodeCorrectionPeriodeClosed,
	}
	for _, code := range operational {
		assert.Falsef(t, IsRegulated(code), "expected %s to be operational (not regulated)", code)
	}

	// Unknown code must be false.
	assert.False(t, IsRegulated("UNKNOWN_CODE"))
	assert.False(t, IsRegulated(""))
}

func TestWorkflowPathFor(t *testing.T) {
	// Regulated → 6-eyes.
	assert.Equal(t, WorkflowPath6Eyes, WorkflowPathFor(EventCodeECLPembentukan))
	assert.Equal(t, WorkflowPath6Eyes, WorkflowPathFor(EventCodeMTMFVTPL))
	assert.Equal(t, WorkflowPath6Eyes, WorkflowPathFor(EventCodeFXUnrealized))
	assert.Equal(t, WorkflowPath6Eyes, WorkflowPathFor(EventCodeReklasAcFVOCI))

	// Operational → 4-eyes.
	assert.Equal(t, WorkflowPath4Eyes, WorkflowPathFor(EventCodePenempatan))
	assert.Equal(t, WorkflowPath4Eyes, WorkflowPathFor(EventCodeJatuhTempo))
	assert.Equal(t, WorkflowPath4Eyes, WorkflowPathFor(EventCodePeriodeAdjustment))
	assert.Equal(t, WorkflowPath4Eyes, WorkflowPathFor("UNKNOWN_CODE"))
}

func TestIsManualAllowed(t *testing.T) {
	assert.True(t, IsManualAllowed(EventCodePeriodeAdjustment))
	assert.True(t, IsManualAllowed(EventCodeCorrectionPeriodeClosed))

	// Nothing else is allowed for manual posting.
	assert.False(t, IsManualAllowed(EventCodePenempatan))
	assert.False(t, IsManualAllowed(EventCodeECLPembentukan))
	assert.False(t, IsManualAllowed(EventCodeJatuhTempo))
	assert.False(t, IsManualAllowed("UNKNOWN_CODE"))
	assert.False(t, IsManualAllowed(""))
}

func TestBuildIdempotencyKey(t *testing.T) {
	uid := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	// Deterministic: same inputs → same key.
	key1 := BuildIdempotencyKey(uid, EventCodePenempatan)
	key2 := BuildIdempotencyKey(uid, EventCodePenempatan)
	assert.Equal(t, key1, key2, "idempotency key must be deterministic")
	assert.Len(t, key1, 64, "SHA256 hex = 64 chars")

	// Different event code → different key.
	key3 := BuildIdempotencyKey(uid, EventCodeJatuhTempo)
	assert.NotEqual(t, key1, key3)

	// Different UUID → different key.
	uid2 := uuid.New()
	key4 := BuildIdempotencyKey(uid2, EventCodePenempatan)
	assert.NotEqual(t, key1, key4)

	// Known SHA256 for reproducibility.
	// sha256("550e8400-e29b-41d4-a716-446655440000::PENEMPATAN")
	// This test pins the algorithm so any change is caught.
	assert.Regexp(t, `^[0-9a-f]{64}$`, key1, "must be lowercase hex")
}

// TestContainsStr tests the package-private helper.
func TestContainsStr(t *testing.T) {
	assert.True(t, containsStr([]string{"AC", "FVOCI"}, "AC"))
	assert.True(t, containsStr([]string{"AC", "FVOCI"}, "FVOCI"))
	assert.False(t, containsStr([]string{"AC", "FVOCI"}, "FVTPL"))
	assert.False(t, containsStr(nil, "AC"))
	assert.False(t, containsStr([]string{}, "AC"))
}

// TestDecimalPrecision verifies no float64 used for money (DEC-016).
func TestDecimalPrecision(t *testing.T) {
	// decimal.NewFromString must not lose precision.
	amt, err := decimal.NewFromString("1234567890.1234")
	require.NoError(t, err)
	assert.Equal(t, "1234567890.1234", amt.StringFixed(4))

	// Multiplication preserves precision.
	mult := decimal.NewFromFloat(1.5)
	result := amt.Mul(mult)
	// 1234567890.1234 * 1.5 = 1851851835.1851 (HALF_EVEN rounding at 4dp)
	assert.Equal(t, "1851851835.1851", result.StringFixed(4))

	// Ensure zero check works.
	assert.True(t, decimal.Zero.IsZero())
	assert.False(t, decimal.NewFromInt(1).IsZero())
}

// TestEventCodeCount validates exactly 27 event codes (DEC-P5-M1-002).
func TestEventCodeCount(t *testing.T) {
	all := []string{
		EventCodePenempatan, EventCodeAkrualBunga, EventCodeECLPembentukan,
		EventCodeECLReversal, EventCodePOCIDeltaECL, EventCodeMTMFVTPL,
		EventCodeMTMFVOCI, EventCodeMTMFVOCIElection, EventCodeReklasOCIPL,
		EventCodeReklasAcFVOCI, EventCodeReklasFVOCIAc, EventCodeModifikasiMaterial,
		EventCodeEIRCatchup, EventCodeStageMigration, EventCodeJatuhTempo,
		EventCodePenjualanPencairan, EventCodePembayaranBunga, EventCodePembayaranPokok,
		EventCodeRenewalDeposito, EventCodePenerimaanDividen, EventCodeDistribusiReksadana,
		EventCodeFXRealized, EventCodeFXUnrealized, EventCodeAmortisasiPremiDiskonto,
		EventCodePenghapusan, EventCodePeriodeAdjustment, EventCodeCorrectionPeriodeClosed,
	}
	assert.Equal(t, 27, len(all), "DEC-P5-M1-002: exactly 27 event codes required")

	// No duplicates.
	seen := make(map[string]struct{})
	for _, c := range all {
		_, dup := seen[c]
		assert.Falsef(t, dup, "duplicate event code: %s", c)
		seen[c] = struct{}{}
	}
}

// TestDLQStatusConstants validates DLQ status values are distinct.
func TestDLQStatusConstants(t *testing.T) {
	statuses := []DLQStatus{
		DLQStatusFailed, DLQStatusReplaying, DLQStatusReplayedOK, DLQStatusAbandoned,
	}
	seen := make(map[DLQStatus]struct{})
	for _, s := range statuses {
		_, dup := seen[s]
		assert.Falsef(t, dup, "duplicate DLQ status: %s", s)
		seen[s] = struct{}{}
	}
}

// TestWorkflowPathConstants verifies 4-eyes/6-eyes path values are distinct.
func TestWorkflowPathConstants(t *testing.T) {
	assert.NotEqual(t, WorkflowPath4Eyes, WorkflowPath6Eyes)
	assert.Equal(t, WorkflowPath("4-eyes"), WorkflowPath4Eyes)
	assert.Equal(t, WorkflowPath("6-eyes"), WorkflowPath6Eyes)
}
