package akrualmaturity

// validator_test.go — Coverage for all ValidateXxx functions in validator.go.

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ValidateInstrumenForAkrual ───────────────────────────────────────────────

func TestValidateInstrumenForAkrual_Valid(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen:     "INST-001",
		Status:            "ACTIVE",
		KlasifikasiPSAK71: "AC",
		EIRPersen:         decimal.NewFromFloat(0.075),
	}
	require.NoError(t, ValidateInstrumenForAkrual(inst))
}

func TestValidateInstrumenForAkrual_NotActive(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen:     "INST-002",
		Status:            "MATURED",
		KlasifikasiPSAK71: "AC",
		EIRPersen:         decimal.NewFromFloat(0.075),
	}
	err := ValidateInstrumenForAkrual(inst)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ACTIVE")
}

func TestValidateInstrumenForAkrual_FVTPL_Excluded(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen:     "INST-003",
		Status:            "ACTIVE",
		KlasifikasiPSAK71: "FVTPL",
		EIRPersen:         decimal.NewFromFloat(0.075),
	}
	err := ValidateInstrumenForAkrual(inst)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "FVTPL")
}

func TestValidateInstrumenForAkrual_ZeroEIR(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen:     "INST-004",
		Status:            "ACTIVE",
		KlasifikasiPSAK71: "AC",
		EIRPersen:         decimal.Zero,
	}
	err := ValidateInstrumenForAkrual(inst)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), CodeAkrualEIRNotFound)
}

func TestValidateInstrumenForAkrual_FVOCI_NoEIR(t *testing.T) {
	// FVOCI is not FVTPL and not excluded — but requires EIR
	inst := InstrumenAkrualInfo{
		KodeInstrumen:     "INST-005",
		Status:            "ACTIVE",
		KlasifikasiPSAK71: "FVOCI",
		EIRPersen:         decimal.Zero,
	}
	err := ValidateInstrumenForAkrual(inst)
	assert.Error(t, err)
}

// ─── ValidateInstrumenForMaturity ─────────────────────────────────────────────

func TestValidateInstrumenForMaturity_Valid(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen: "INST-010",
		Status:        "ACTIVE",
	}
	require.NoError(t, ValidateInstrumenForMaturity(inst))
}

func TestValidateInstrumenForMaturity_Matured(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen: "INST-011",
		Status:        "MATURED",
	}
	err := ValidateInstrumenForMaturity(inst)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), CodeMaturityInstrumenNotActive)
}

func TestValidateInstrumenForMaturity_Canceled(t *testing.T) {
	inst := InstrumenAkrualInfo{
		KodeInstrumen: "INST-012",
		Status:        "CANCELED",
	}
	err := ValidateInstrumenForMaturity(inst)
	assert.Error(t, err)
}

// ─── ValidatePeriodeOpen ──────────────────────────────────────────────────────

func TestValidatePeriodeOpen_Open(t *testing.T) {
	require.NoError(t, ValidatePeriodeOpen("OPEN"))
}

func TestValidatePeriodeOpen_SoftClosed(t *testing.T) {
	err := ValidatePeriodeOpen("SOFT_CLOSED")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), CodeAkrualPeriodeLocked)
}

func TestValidatePeriodeOpen_HardClosed(t *testing.T) {
	err := ValidatePeriodeOpen("HARD_CLOSED")
	assert.Error(t, err)
}

// ─── ValidateSignatureMethod ──────────────────────────────────────────────────

func TestValidateSignatureMethod_JWTStepUp(t *testing.T) {
	require.NoError(t, ValidateSignatureMethod("JWT_STEP_UP"))
}

func TestValidateSignatureMethod_Empty(t *testing.T) {
	err := ValidateSignatureMethod("")
	assert.Error(t, err)
}

func TestValidateSignatureMethod_WrongValue(t *testing.T) {
	err := ValidateSignatureMethod("TOTP")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_STEP_UP")
}

// ─── ValidateOverrideReason ───────────────────────────────────────────────────

func TestValidateOverrideReason_Valid(t *testing.T) {
	reason := "ECL sealed run diperbarui — staging telah dikonfirmasi oleh Risk Officer sebelum override."
	require.NoError(t, ValidateOverrideReason(reason))
}

func TestValidateOverrideReason_TooShort(t *testing.T) {
	err := ValidateOverrideReason("short")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "30")
}

func TestValidateOverrideReason_Exactly30Chars(t *testing.T) {
	// Exactly 30 chars = valid
	reason := "1234567890123456789012345678901234567890"[:30]
	require.NoError(t, ValidateOverrideReason(reason))
}

func TestValidateOverrideReason_29Chars(t *testing.T) {
	reason := "12345678901234567890123456789" // 29 chars
	err := ValidateOverrideReason(reason)
	assert.Error(t, err)
}

// ─── ValidateDividenInput ─────────────────────────────────────────────────────

func TestValidateDividenInput_Valid(t *testing.T) {
	require.NoError(t, ValidateDividenInput(decimal.NewFromInt(100_000), "2026-06-20"))
}

func TestValidateDividenInput_ZeroAmount(t *testing.T) {
	err := ValidateDividenInput(decimal.Zero, "2026-06-20")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), CodeDividenValidationFailed)
}

func TestValidateDividenInput_NegativeAmount(t *testing.T) {
	err := ValidateDividenInput(decimal.NewFromInt(-1), "2026-06-20")
	assert.Error(t, err)
}

func TestValidateDividenInput_EmptyTanggal(t *testing.T) {
	err := ValidateDividenInput(decimal.NewFromInt(100), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tanggal_terima")
}
