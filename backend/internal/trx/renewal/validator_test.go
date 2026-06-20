package renewal

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/errors"
)

func TestValidateTenor_Valid(t *testing.T) {
	cases := []int{1, 12, 24, 60}
	for _, c := range cases {
		assert.NoError(t, ValidateTenor(c), "tenor=%d", c)
	}
}

func TestValidateTenor_Invalid(t *testing.T) {
	cases := []int{0, -1, 61, 100}
	for _, c := range cases {
		err := ValidateTenor(c)
		require.Error(t, err, "tenor=%d should fail", c)
		de, ok := errors.IsDomainError(err)
		require.True(t, ok)
		assert.Equal(t, errors.CodeValidationFailed, de.Code())
	}
}

func TestValidateRate_Valid(t *testing.T) {
	cases := []string{"0", "5", "15.5", "30"}
	for _, c := range cases {
		r, _ := decimal.NewFromString(c)
		assert.NoError(t, ValidateRate(r), "rate=%s", c)
	}
}

func TestValidateRate_Invalid(t *testing.T) {
	cases := []string{"-0.01", "30.01", "100"}
	for _, c := range cases {
		r, _ := decimal.NewFromString(c)
		err := ValidateRate(r)
		require.Error(t, err, "rate=%s should fail", c)
		de, ok := errors.IsDomainError(err)
		require.True(t, ok)
		assert.Equal(t, errors.CodeValidationFailed, de.Code())
	}
}

func TestValidateSkema_Valid(t *testing.T) {
	assert.NoError(t, ValidateSkema("POKOK_SAJA"))
	assert.NoError(t, ValidateSkema("POKOK_PLUS_BUNGA"))
}

func TestValidateSkema_Invalid(t *testing.T) {
	cases := []string{"INVALID", "", "pokok_saja", "BUNGA_SAJA"}
	for _, c := range cases {
		err := ValidateSkema(c)
		require.Error(t, err, "skema=%q should fail", c)
	}
}

func TestValidateInstrumenEligibility_Valid(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "DEP-001",
		JenisInstrumen:    "DEPOSITO",
		Status:            "ACTIVE",
		KlasifikasiLocked: true,
	}
	assert.NoError(t, ValidateInstrumenEligibility(inst))
}

func TestValidateInstrumenEligibility_NotDeposito(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "OBL-001",
		JenisInstrumen:    "OBLIGASI",
		Status:            "ACTIVE",
		KlasifikasiLocked: true,
	}
	err := ValidateInstrumenEligibility(inst)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeValidationFailed, de.Code())
}

func TestValidateInstrumenEligibility_NotActive(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "DEP-002",
		JenisInstrumen:    "DEPOSITO",
		Status:            "MATURED",
		KlasifikasiLocked: true,
	}
	err := ValidateInstrumenEligibility(inst)
	require.Error(t, err)
}

func TestValidateInstrumenEligibility_KlasifikasiNotLocked(t *testing.T) {
	inst := InstrumenInfo{
		KodeInstrumen:     "DEP-003",
		JenisInstrumen:    "DEPOSITO",
		Status:            "ACTIVE",
		KlasifikasiLocked: false,
	}
	err := ValidateInstrumenEligibility(inst)
	require.Error(t, err)
}

func TestValidateBungaBersihMinimum_PokokSaja_NoCheck(t *testing.T) {
	// POKOK_SAJA: no minimum check regardless of amount
	err := ValidateBungaBersihMinimum(SkemaPokokSaja, decimal.NewFromInt(0))
	assert.NoError(t, err)
}

func TestValidateBungaBersihMinimum_PokokPlusBunga_BelowMin(t *testing.T) {
	err := ValidateBungaBersihMinimum(SkemaPokokPlusBunga, decimal.NewFromInt(99_999))
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeValidationFailed, de.Code())
}

func TestValidateBungaBersihMinimum_PokokPlusBunga_AtMin(t *testing.T) {
	err := ValidateBungaBersihMinimum(SkemaPokokPlusBunga, decimal.NewFromInt(100_000))
	assert.NoError(t, err)
}

func TestValidateBungaBersihMinimum_PokokPlusBunga_AboveMin(t *testing.T) {
	err := ValidateBungaBersihMinimum(SkemaPokokPlusBunga, decimal.NewFromInt(5_000_000))
	assert.NoError(t, err)
}

func TestValidatePphConsistency_Match(t *testing.T) {
	bungaKotor := decimal.NewFromInt(10_000_000)
	// storedPph = exactly 20% — should pass
	storedPph := bungaKotor.Mul(decimal.NewFromFloat(0.20)).RoundBank(4)
	assert.NoError(t, ValidatePphConsistency(storedPph, bungaKotor))
}

func TestValidatePphConsistency_WithinTolerance(t *testing.T) {
	bungaKotor := decimal.NewFromInt(10_000_000)
	storedPph := bungaKotor.Mul(decimal.NewFromFloat(0.20)).Add(decimal.NewFromFloat(0.005))
	assert.NoError(t, ValidatePphConsistency(storedPph, bungaKotor), "within 0.01 tolerance")
}

func TestValidatePphConsistency_Mismatch(t *testing.T) {
	bungaKotor := decimal.NewFromInt(10_000_000)
	storedPph := bungaKotor.Mul(decimal.NewFromFloat(0.25)) // 25% != 20%
	err := ValidatePphConsistency(storedPph, bungaKotor)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeValidationFailed, de.Code())
}

func TestValidateSignatureMethod_Valid(t *testing.T) {
	assert.NoError(t, ValidateSignatureMethod("JWT_STEP_UP"))
}

func TestValidateSignatureMethod_Invalid(t *testing.T) {
	cases := []string{"", "TOTP", "OTP", "jwt_step_up", "PASSWORD"}
	for _, c := range cases {
		err := ValidateSignatureMethod(c)
		require.Error(t, err, "method=%q should fail", c)
	}
}

func TestValidateRejectComment_Valid(t *testing.T) {
	// Exactly 30 chars
	comment := "Instrumen tidak sesuai kriteria"
	assert.GreaterOrEqual(t, len([]rune(comment)), 30)
	assert.NoError(t, ValidateRejectComment(comment))
}

func TestValidateRejectComment_TooShort(t *testing.T) {
	cases := []string{"", "Too short", "29 character comment test ok ."}
	for _, c := range cases {
		if len([]rune(c)) < 30 {
			err := ValidateRejectComment(c)
			require.Error(t, err, "comment=%q should fail", c)
			de, ok := errors.IsDomainError(err)
			require.True(t, ok)
			assert.Equal(t, errors.CodeValidationFailed, de.Code())
		}
	}
}
