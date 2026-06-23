package akrualmaturity

// calc_test.go — 100% coverage required for compliance (PSAK 71 §5.4.1(b)).
// Every branch in calc.go must be exercised.

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ComputeAkrualBunga ───────────────────────────────────────────────────────

func TestComputeAkrualBunga_Stage1_GrossCarrying(t *testing.T) {
	// gross = 1,000,000, eir = 7.5% → daily = 1_000_000 × 0.075 / 365 = 205.4795
	gross := decimal.NewFromInt(1_000_000)
	ecl := decimal.Zero
	eir := decimal.NewFromFloat(0.075)

	akrual, basis, err := ComputeAkrualBunga(1, gross, ecl, eir)
	require.NoError(t, err)
	assert.Equal(t, gross, basis, "Stage 1 basis must be gross")
	// 1_000_000 × 0.075 / 365 = 205.4794520547945... → HALF_EVEN 4dp = 205.4795
	expected := decimal.NewFromFloat(205.4795)
	assert.True(t, expected.Equal(akrual), "Stage 1 daily accrual mismatch: got %s, want %s", akrual, expected)
}

func TestComputeAkrualBunga_Stage2_GrossCarrying(t *testing.T) {
	// Stage 2 uses same gross basis as Stage 1
	gross := decimal.NewFromInt(500_000)
	ecl := decimal.NewFromInt(10_000) // ECL ignored for Stage 2 basis
	eir := decimal.NewFromFloat(0.08)

	akrual, basis, err := ComputeAkrualBunga(2, gross, ecl, eir)
	require.NoError(t, err)
	assert.Equal(t, gross, basis, "Stage 2 basis must be gross (ECL not deducted)")
	// 500_000 × 0.08 / 365 = 109.5890...  → HALF_EVEN 4dp
	expected := decimal.NewFromFloat(109.589)
	diff := akrual.Sub(expected).Abs()
	assert.True(t, diff.LessThan(decimal.NewFromFloat(0.01)), "Stage 2 accrual out of range: %s", akrual)
}

func TestComputeAkrualBunga_Stage3_NetCarrying_ECLPositive(t *testing.T) {
	// PSAK 71 §5.4.1(b): Stage 3 uses (Gross - ECL) as basis
	gross := decimal.NewFromInt(1_000_000)
	ecl := decimal.NewFromInt(200_000) // ECL = 200k
	eir := decimal.NewFromFloat(0.075)

	akrual, basis, err := ComputeAkrualBunga(3, gross, ecl, eir)
	require.NoError(t, err)

	expectedBasis := decimal.NewFromInt(800_000) // 1_000_000 - 200_000
	assert.Equal(t, expectedBasis, basis, "Stage 3 basis must be net carrying (gross - ECL)")

	// 800_000 × 0.075 / 365 = 164.3835...
	expected := decimal.NewFromFloat(164.3836)
	diff := akrual.Sub(expected).Abs()
	assert.True(t, diff.LessThan(decimal.NewFromFloat(0.01)), "Stage 3 net accrual out of range: %s", akrual)
}

func TestComputeAkrualBunga_Stage3_ECLExceedsGross_ClampedToZero(t *testing.T) {
	// When ECL >= gross → net carrying clamped to 0 → accrual = 0
	gross := decimal.NewFromInt(100_000)
	ecl := decimal.NewFromInt(150_000) // ECL > gross
	eir := decimal.NewFromFloat(0.075)

	akrual, basis, err := ComputeAkrualBunga(3, gross, ecl, eir)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(basis), "ECL > gross: net basis must be clamped to 0")
	assert.True(t, decimal.Zero.Equal(akrual), "ECL > gross: accrual must be 0")
}

func TestComputeAkrualBunga_InvalidStage(t *testing.T) {
	gross := decimal.NewFromInt(1_000_000)
	ecl := decimal.Zero
	eir := decimal.NewFromFloat(0.075)

	_, _, err := ComputeAkrualBunga(0, gross, ecl, eir)
	assert.Error(t, err, "stage 0 must return error")

	_, _, err = ComputeAkrualBunga(4, gross, ecl, eir)
	assert.Error(t, err, "stage 4 must return error")
}

func TestComputeAkrualBunga_NegativeEIR(t *testing.T) {
	gross := decimal.NewFromInt(1_000_000)
	ecl := decimal.Zero
	eir := decimal.NewFromFloat(-0.01)

	_, _, err := ComputeAkrualBunga(1, gross, ecl, eir)
	assert.Error(t, err, "negative EIR must return error")
}

func TestComputeAkrualBunga_NegativeGross(t *testing.T) {
	_, _, err := ComputeAkrualBunga(1, decimal.NewFromInt(-1), decimal.Zero, decimal.NewFromFloat(0.05))
	assert.Error(t, err, "negative gross must return error")
}

func TestComputeAkrualBunga_NegativeECL(t *testing.T) {
	_, _, err := ComputeAkrualBunga(3, decimal.NewFromInt(1_000_000), decimal.NewFromInt(-100), decimal.NewFromFloat(0.05))
	assert.Error(t, err, "negative ECL must return error")
}

func TestComputeAkrualBunga_ZeroEIR(t *testing.T) {
	// EIR = 0 is allowed (zero-coupon or POCI edge case); accrual = 0
	gross := decimal.NewFromInt(1_000_000)
	akrual, _, err := ComputeAkrualBunga(1, gross, decimal.Zero, decimal.Zero)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(akrual), "zero EIR must yield zero accrual")
}

// ─── ComputePPH ───────────────────────────────────────────────────────────────

func TestComputePPH_Deposito_20pct(t *testing.T) {
	// PPh deposito bunga: 20%
	gross := decimal.NewFromInt(100_000)
	pph, net, err := ComputePPH(JenisBunga, gross)
	require.NoError(t, err)

	expectedPPH := decimal.NewFromInt(20_000) // 100_000 × 20%
	expectedNet := decimal.NewFromInt(80_000)
	assert.True(t, expectedPPH.Equal(pph), "PPh deposito must be 20%: got %s", pph)
	assert.True(t, expectedNet.Equal(net), "Net setelah PPh deposito: got %s", net)
}

func TestComputePPH_Dividen_10pct(t *testing.T) {
	// PPh dividen: 10% (UU PPh §17 2c)
	gross := decimal.NewFromInt(50_000)
	pph, net, err := ComputePPH(JenisDividen, gross)
	require.NoError(t, err)

	expectedPPH := decimal.NewFromInt(5_000) // 50_000 × 10%
	expectedNet := decimal.NewFromInt(45_000)
	assert.True(t, expectedPPH.Equal(pph), "PPh dividen must be 10%: got %s", pph)
	assert.True(t, expectedNet.Equal(net), "Net setelah PPh dividen: got %s", net)
}

func TestComputePPH_DistribusiRD_10pct(t *testing.T) {
	gross := decimal.NewFromInt(30_000)
	pph, net, err := ComputePPH(JenisDistribusiRD, gross)
	require.NoError(t, err)

	assert.True(t, decimal.NewFromInt(3_000).Equal(pph), "PPh distribusi RD must be 10%")
	assert.True(t, decimal.NewFromInt(27_000).Equal(net))
}

func TestComputePPH_AmortisasiPremium_ZeroPPH(t *testing.T) {
	gross := decimal.NewFromInt(5_000)
	pph, net, err := ComputePPH(JenisAmortisasiPremium, gross)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(pph), "Amortisasi premium: no PPh")
	assert.True(t, gross.Equal(net), "Amortisasi premium: net = gross")
}

func TestComputePPH_AmortisasiDiskon_ZeroPPH(t *testing.T) {
	gross := decimal.NewFromInt(5_000)
	pph, net, err := ComputePPH(JenisAmortisasiDiskon, gross)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(pph))
	assert.True(t, gross.Equal(net))
}

func TestComputePPH_UnknownJenis(t *testing.T) {
	_, _, err := ComputePPH("UNKNOWN_JENIS", decimal.NewFromInt(100))
	assert.Error(t, err)
}

func TestComputePPH_NegativeAmount(t *testing.T) {
	_, _, err := ComputePPH(JenisBunga, decimal.NewFromInt(-100))
	assert.Error(t, err)
}

// ─── ComputeAmortisasi ────────────────────────────────────────────────────────

func TestComputeAmortisasi_PrecomputedHarian_DiskonBond(t *testing.T) {
	// Discount bond: EIR > kupon → jenis = JenisAmortisasiDiskon
	kupon := decimal.NewFromFloat(0.05)
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.07),
		AmortisasiHarian: decimal.NewFromFloat(54.7945), // pre-computed
		KuponRate:        &kupon,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.Equal(t, JenisAmortisasiDiskon, jenis)
	assert.True(t, decimal.NewFromFloat(54.7945).Equal(amount), "pre-computed amount: got %s", amount)
}

func TestComputeAmortisasi_PrecomputedHarian_PremiumBond(t *testing.T) {
	// Premium bond: EIR < kupon → jenis = JenisAmortisasiPremium
	kupon := decimal.NewFromFloat(0.09)
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.07),
		AmortisasiHarian: decimal.NewFromFloat(54.7945),
		KuponRate:        &kupon,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.Equal(t, JenisAmortisasiPremium, jenis)
	assert.True(t, amount.IsPositive(), "amortisasi premium amount must be positive, got %s", amount)
}

func TestComputeAmortisasi_FirstPrinciples_DiskonBond(t *testing.T) {
	// No pre-computed harian — compute from first principles
	// EIR=8%, kupon=5%, carrying=1_000_000
	// interestIncome = 1_000_000 × 0.08 / 365 = 219.1780
	// couponCash     = 1_000_000 × 0.05 / 365 = 136.9863
	// delta = 219.1780 - 136.9863 = 82.1918 (positive → discount)
	kupon := decimal.NewFromFloat(0.05)
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.08),
		AmortisasiHarian: decimal.Zero, // trigger first-principles
		KuponRate:        &kupon,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.Equal(t, JenisAmortisasiDiskon, jenis)
	assert.True(t, amount.IsPositive(), "discount delta should be positive: %s", amount)
}

func TestComputeAmortisasi_FirstPrinciples_PremiumBond(t *testing.T) {
	// EIR=5%, kupon=8% → premium bond
	kupon := decimal.NewFromFloat(0.08)
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.05),
		AmortisasiHarian: decimal.Zero,
		KuponRate:        &kupon,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.Equal(t, JenisAmortisasiPremium, jenis)
	assert.True(t, amount.IsPositive(), "premium absolute amount must be positive")
}

func TestComputeAmortisasi_ParBond_ZeroDelta(t *testing.T) {
	// EIR == kupon → par bond → amortisasi = 0
	kupon := decimal.NewFromFloat(0.07)
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.07),
		AmortisasiHarian: decimal.Zero,
		KuponRate:        &kupon,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(amount), "par bond delta must be 0")
	assert.Equal(t, JenisAmortisasiDiskon, jenis) // default direction for zero
}

func TestComputeAmortisasi_POCI_CreditAdjustedEIR(t *testing.T) {
	// POCI: CreditAdjustedEIR overrides EIRPersen
	creditEIR := decimal.NewFromFloat(0.06)
	kupon := decimal.NewFromFloat(0.08)
	row := AmortisasiScheduleRow{
		EIRPersen:         decimal.NewFromFloat(0.10), // ignored for POCI
		AmortisasiHarian:  decimal.Zero,
		KuponRate:         &kupon,
		CreditAdjustedEIR: &creditEIR,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	// 0.06 < 0.08 → premium bond
	assert.Equal(t, JenisAmortisasiPremium, jenis, "POCI with credit EIR < kupon = premium")
	_ = amount
}

func TestComputeAmortisasi_NilKuponRate(t *testing.T) {
	// nil KuponRate treated as 0 → discount bond if EIR > 0
	row := AmortisasiScheduleRow{
		EIRPersen:        decimal.NewFromFloat(0.07),
		AmortisasiHarian: decimal.Zero,
		KuponRate:        nil,
	}
	prevCarrying := decimal.NewFromInt(1_000_000)

	amount, jenis, err := ComputeAmortisasi(row, prevCarrying)
	require.NoError(t, err)
	assert.Equal(t, JenisAmortisasiDiskon, jenis)
	assert.True(t, amount.IsPositive())
}

func TestComputeAmortisasi_NegativePrevCarrying(t *testing.T) {
	row := AmortisasiScheduleRow{EIRPersen: decimal.NewFromFloat(0.07)}
	_, _, err := ComputeAmortisasi(row, decimal.NewFromInt(-1))
	assert.Error(t, err)
}

// ─── ComputeMaturitySettlement ────────────────────────────────────────────────

func TestComputeMaturitySettlement_HappyPath(t *testing.T) {
	// pokok = 1_000_000, bunga_last = 15_000
	// pph = 15_000 × 20% = 3_000
	// net_kas = 1_000_000 + 15_000 - 3_000 = 1_012_000
	pokok := decimal.NewFromInt(1_000_000)
	bunga := decimal.NewFromInt(15_000)

	pph, netKas, err := ComputeMaturitySettlement(pokok, bunga)
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(3_000).Equal(pph), "PPh maturity: got %s", pph)
	assert.True(t, decimal.NewFromInt(1_012_000).Equal(netKas), "net_kas: got %s", netKas)
}

func TestComputeMaturitySettlement_ZeroBunga(t *testing.T) {
	// Zero interest (already accrued or zero-coupon)
	pokok := decimal.NewFromInt(500_000)
	bunga := decimal.Zero

	pph, netKas, err := ComputeMaturitySettlement(pokok, bunga)
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(pph))
	assert.True(t, pokok.Equal(netKas))
}

func TestComputeMaturitySettlement_NegativePokok(t *testing.T) {
	_, _, err := ComputeMaturitySettlement(decimal.NewFromInt(-1), decimal.Zero)
	assert.Error(t, err)
}

func TestComputeMaturitySettlement_NegativeBunga(t *testing.T) {
	_, _, err := ComputeMaturitySettlement(decimal.NewFromInt(1_000_000), decimal.NewFromInt(-100))
	assert.Error(t, err)
}

// ─── ConvertFCYtoIDR ─────────────────────────────────────────────────────────

func TestConvertFCYtoIDR_HappyPath(t *testing.T) {
	// 100 USD × 15,432.50 IDR/USD = 1_543_250.0000
	fcy := decimal.NewFromInt(100)
	rate := decimal.NewFromFloat(15_432.50)

	idr, err := ConvertFCYtoIDR(fcy, rate)
	require.NoError(t, err)
	expected := decimal.NewFromFloat(1_543_250.0)
	assert.True(t, expected.Equal(idr), "FCY→IDR: got %s", idr)
}

func TestConvertFCYtoIDR_ZeroFCY(t *testing.T) {
	idr, err := ConvertFCYtoIDR(decimal.Zero, decimal.NewFromFloat(15_000))
	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(idr))
}

func TestConvertFCYtoIDR_ZeroRate(t *testing.T) {
	_, err := ConvertFCYtoIDR(decimal.NewFromInt(100), decimal.Zero)
	assert.Error(t, err, "zero FX rate must return error")
}

func TestConvertFCYtoIDR_NegativeRate(t *testing.T) {
	_, err := ConvertFCYtoIDR(decimal.NewFromInt(100), decimal.NewFromInt(-1))
	assert.Error(t, err, "negative FX rate must return error")
}

// ─── IsStaleECLRun ────────────────────────────────────────────────────────────

func TestIsStaleECLRun_Stale(t *testing.T) {
	// sealed 45 days ago, stale threshold = 30 days → stale
	sealedAt := time.Now().UTC().AddDate(0, 0, -45)
	assert.True(t, IsStaleECLRun(sealedAt, 30), "45-day old run should be stale with 30-day threshold")
}

func TestIsStaleECLRun_Fresh(t *testing.T) {
	// sealed 10 days ago, stale threshold = 30 days → not stale
	sealedAt := time.Now().UTC().AddDate(0, 0, -10)
	assert.False(t, IsStaleECLRun(sealedAt, 30), "10-day old run should not be stale with 30-day threshold")
}

func TestIsStaleECLRun_ExactBoundary(t *testing.T) {
	// sealed exactly staleDays ago → stale (Before is strict less-than)
	// cutoff = now - 30 days; sealedAt = now - 30 days; sealedAt.Before(cutoff) = false
	sealedAt := time.Now().UTC().AddDate(0, 0, -30).Add(-time.Second)
	assert.True(t, IsStaleECLRun(sealedAt, 30), "Just past boundary should be stale")
}

func TestIsStaleECLRun_ZeroStaleDays(t *testing.T) {
	// staleDays = 0 → everything is stale
	sealedAt := time.Now().UTC().Add(-time.Second)
	assert.True(t, IsStaleECLRun(sealedAt, 0))
}
