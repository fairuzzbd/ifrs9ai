package penjualan

// validator_test.go — Unit tests for validator.go.
// No DB — pure stateless validation function coverage.

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ValidateDisposalType ─────────────────────────────────────────────────────

func TestValidateDisposalType_Valid(t *testing.T) {
	assert.NoError(t, ValidateDisposalType("PARTIAL"))
	assert.NoError(t, ValidateDisposalType("FULL"))
}

func TestValidateDisposalType_Invalid(t *testing.T) {
	cases := []string{"partial", "full", "HALF", "", "DISPOSE", "SELL"}
	for _, c := range cases {
		assert.Error(t, ValidateDisposalType(c), "expected error for '%s'", c)
	}
}

// ─── ValidateHarga ────────────────────────────────────────────────────────────

func TestValidateHarga_Positive(t *testing.T) {
	assert.NoError(t, ValidateHarga(decimal.NewFromFloat(0.0001)))
	assert.NoError(t, ValidateHarga(decimal.NewFromFloat(1000000)))
}

func TestValidateHarga_ZeroOrNegative(t *testing.T) {
	assert.Error(t, ValidateHarga(decimal.Zero))
	assert.Error(t, ValidateHarga(decimal.NewFromFloat(-1)))
	assert.Error(t, ValidateHarga(decimal.NewFromFloat(-0.0001)))
}

// ─── ValidateQtyPositive ─────────────────────────────────────────────────────

func TestValidateQtyPositive_Valid(t *testing.T) {
	assert.NoError(t, ValidateQtyPositive(decimal.NewFromFloat(0.00000001)))
	assert.NoError(t, ValidateQtyPositive(decimal.NewFromFloat(1000000)))
}

func TestValidateQtyPositive_ZeroOrNegative(t *testing.T) {
	assert.Error(t, ValidateQtyPositive(decimal.Zero))
	assert.Error(t, ValidateQtyPositive(decimal.NewFromFloat(-1)))
}

// ─── ValidateQtyVsHolding ─────────────────────────────────────────────────────

func TestValidateQtyVsHolding_PARTIAL_LessThanHolding(t *testing.T) {
	assert.NoError(t, ValidateQtyVsHolding(
		decimal.NewFromFloat(499),
		decimal.NewFromFloat(500),
		DisposalPartial,
	))
}

func TestValidateQtyVsHolding_PARTIAL_EqualHolding_Error(t *testing.T) {
	// PARTIAL where qty == holding should fail → use FULL instead
	err := ValidateQtyVsHolding(
		decimal.NewFromFloat(500),
		decimal.NewFromFloat(500),
		DisposalPartial,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FULL")
}

func TestValidateQtyVsHolding_PARTIAL_ExceedsHolding_Error(t *testing.T) {
	err := ValidateQtyVsHolding(
		decimal.NewFromFloat(501),
		decimal.NewFromFloat(500),
		DisposalPartial,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "melebihi")
}

func TestValidateQtyVsHolding_FULL_Equal(t *testing.T) {
	assert.NoError(t, ValidateQtyVsHolding(
		decimal.NewFromFloat(1000),
		decimal.NewFromFloat(1000),
		DisposalFull,
	))
}

func TestValidateQtyVsHolding_FULL_NotEqual_Error(t *testing.T) {
	err := ValidateQtyVsHolding(
		decimal.NewFromFloat(999),
		decimal.NewFromFloat(1000),
		DisposalFull,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sama")
}

func TestValidateQtyVsHolding_FULL_ExceedsHolding_Error(t *testing.T) {
	err := ValidateQtyVsHolding(
		decimal.NewFromFloat(1001),
		decimal.NewFromFloat(1000),
		DisposalFull,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "melebihi")
}

// ─── ValidateInstrumenEligibility ────────────────────────────────────────────

func TestValidateInstrumenEligibility_ActiveAndLocked(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "DEPT-001",
		Status:            "ACTIVE",
		KlasifikasiLocked: true,
	}
	assert.NoError(t, ValidateInstrumenEligibility(inst))
}

func TestValidateInstrumenEligibility_NotActive_Error(t *testing.T) {
	for _, status := range []string{"DRAFT", "DISPOSED", "INACTIVE", "TERMINATED", ""} {
		inst := InstrumenInfo{
			KodeInstrumen:     "DEPT-001",
			Status:            status,
			KlasifikasiLocked: true,
		}
		err := ValidateInstrumenEligibility(inst)
		require.Errorf(t, err, "status=%s", status)
		assert.Contains(t, err.Error(), "ACTIVE")
	}
}

func TestValidateInstrumenEligibility_NotLocked_Error(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "DEPT-001",
		Status:            "ACTIVE",
		KlasifikasiLocked: false,
	}
	err := ValidateInstrumenEligibility(inst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "klasifikasi_locked")
}

// ─── ValidateSignatureMethod ─────────────────────────────────────────────────

func TestValidateSignatureMethod_Valid(t *testing.T) {
	assert.NoError(t, ValidateSignatureMethod("JWT_STEP_UP"))
}

func TestValidateSignatureMethod_Invalid(t *testing.T) {
	cases := []string{"jwt_step_up", "STEP_UP", "PASSWORD", "", "TOTP", "BASIC"}
	for _, c := range cases {
		assert.Errorf(t, ValidateSignatureMethod(c), "expected error for '%s'", c)
	}
}

// ─── ValidateRejectReason ─────────────────────────────────────────────────────

func TestValidateRejectReason_AtLeast30Chars(t *testing.T) {
	// exactly 30 rune characters
	reason := strings.Repeat("a", 30)
	assert.NoError(t, ValidateRejectReason(reason))
}

func TestValidateRejectReason_MoreThan30(t *testing.T) {
	reason := "Penjualan ditolak karena harga jual tidak sesuai ketentuan treasury."
	assert.NoError(t, ValidateRejectReason(reason))
}

func TestValidateRejectReason_TooShort_Error(t *testing.T) {
	reason := strings.Repeat("a", 29)
	err := ValidateRejectReason(reason)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
}

func TestValidateRejectReason_Empty_Error(t *testing.T) {
	err := ValidateRejectReason("")
	require.Error(t, err)
}

func TestValidateRejectReason_MultibyteUnicode(t *testing.T) {
	// Indonesian characters — each rune = 1 rune, but bytes may differ
	reason := strings.Repeat("あ", 30) // 30 Unicode runes
	assert.NoError(t, ValidateRejectReason(reason))
}

func TestValidateRejectReason_MultibyteUnicodeShort(t *testing.T) {
	reason := strings.Repeat("あ", 29) // only 29 runes
	err := ValidateRejectReason(reason)
	require.Error(t, err)
}
