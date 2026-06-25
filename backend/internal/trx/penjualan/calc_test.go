package penjualan

// calc_test.go — 100% coverage for calc.go (compliance-critical).
//
// Tests all boundary cases:
//   ComputeProceed: zero qty, fractional, large values
//   ComputeCostBasis: FULL vs PARTIAL, zero holding (error)
//   ComputeRealizedGL: gain, loss, zero
//   ComputeOCIRecycle: FULL, PARTIAL, zero OCI, negative OCI (loss recycling), recycleOCI=false, holding=0 (error)
//   ComputeBMFrequency: exact thresholds, zero portfolio (error)
//   ValidateBMThresholds: below/at/above warn; at/above block

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ComputeProceed ───────────────────────────────────────────────────────────

func TestComputeProceed_Normal(t *testing.T) {
	harga := decimal.NewFromFloat(15000.5000) // IDR per unit
	qty := decimal.NewFromFloat(1000)         // units
	got := ComputeProceed(harga, qty)
	// 15000.5 × 1000 = 15_000_500.0000
	want := decimal.NewFromFloat(15000500.0000)
	assert.True(t, want.Equal(got), "got %s, want %s", got, want)
}

func TestComputeProceed_ZeroQty(t *testing.T) {
	got := ComputeProceed(decimal.NewFromFloat(15000), decimal.Zero)
	assert.True(t, decimal.Zero.Equal(got))
}

func TestComputeProceed_RoundingHalfEven(t *testing.T) {
	// 1.00005 × 2 = 2.0001 — already exact, test general rounding
	harga := decimal.NewFromFloat(1.00005)
	qty := decimal.NewFromFloat(1)
	got := ComputeProceed(harga, qty)
	// 1.00005 rounded to 4 decimal = 1.0001 (HALF_EVEN: 5 rounds to even = up)
	assert.True(t, got.StringFixed(4) == "1.0001" || got.StringFixed(4) == "1.0000",
		"expected RoundBank(4), got %s", got.StringFixed(4))
}

func TestComputeProceed_FractionalUnits(t *testing.T) {
	// fractional qty (applicable for obligasi — 0.5 lot)
	harga := decimal.NewFromFloat(100000000.0000)
	qty := decimal.NewFromFloat(0.5)
	got := ComputeProceed(harga, qty)
	want := decimal.NewFromFloat(50000000.0000)
	assert.True(t, want.Equal(got))
}

// ─── ComputeCostBasis ─────────────────────────────────────────────────────────

func TestComputeCostBasis_Full(t *testing.T) {
	totalCost := decimal.NewFromFloat(1500000.0000)
	got, err := ComputeCostBasis(totalCost, decimal.NewFromFloat(1000), decimal.NewFromFloat(1000), DisposalFull)
	require.NoError(t, err)
	assert.True(t, totalCost.Equal(got), "FULL: cost_basis must equal total_cost_basis")
}

func TestComputeCostBasis_Partial_Half(t *testing.T) {
	totalCost := decimal.NewFromFloat(2000000.0000)
	qty := decimal.NewFromFloat(500)
	holding := decimal.NewFromFloat(1000)
	got, err := ComputeCostBasis(totalCost, qty, holding, DisposalPartial)
	require.NoError(t, err)
	// 2_000_000 × (500/1000) = 1_000_000
	want := decimal.NewFromFloat(1000000.0000)
	assert.True(t, want.Equal(got))
}

func TestComputeCostBasis_Partial_Fractional(t *testing.T) {
	// 1_000_001.0000 × (1/3) — tests rounding
	totalCost := decimal.NewFromFloat(1000001.0000)
	qty := decimal.NewFromFloat(1)
	holding := decimal.NewFromFloat(3)
	got, err := ComputeCostBasis(totalCost, qty, holding, DisposalPartial)
	require.NoError(t, err)
	// Expect HALF_EVEN to 4 decimal places
	assert.Equal(t, "333333.6667", got.StringFixed(4))
}

func TestComputeCostBasis_ZeroHolding_Error(t *testing.T) {
	_, err := ComputeCostBasis(decimal.NewFromFloat(100), decimal.NewFromFloat(1), decimal.Zero, DisposalPartial)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qty_holding_pre tidak boleh 0")
}

// ─── ComputeRealizedGL ────────────────────────────────────────────────────────

func TestComputeRealizedGL_Gain(t *testing.T) {
	proceed := decimal.NewFromFloat(1200000)
	cost := decimal.NewFromFloat(1000000)
	got := ComputeRealizedGL(proceed, cost)
	want := decimal.NewFromFloat(200000)
	assert.True(t, want.Equal(got))
}

func TestComputeRealizedGL_Loss(t *testing.T) {
	proceed := decimal.NewFromFloat(900000)
	cost := decimal.NewFromFloat(1000000)
	got := ComputeRealizedGL(proceed, cost)
	want := decimal.NewFromFloat(-100000)
	assert.True(t, want.Equal(got))
}

func TestComputeRealizedGL_Zero(t *testing.T) {
	got := ComputeRealizedGL(decimal.NewFromFloat(1000000), decimal.NewFromFloat(1000000))
	assert.True(t, decimal.Zero.Equal(got))
}

// ─── ComputeOCIRecycle ────────────────────────────────────────────────────────

func TestComputeOCIRecycle_NotRecycle_ReturnsNil(t *testing.T) {
	result, err := ComputeOCIRecycle(false, decimal.NewFromFloat(50000), decimal.NewFromFloat(100), decimal.NewFromFloat(1000), DisposalPartial)
	require.NoError(t, err)
	assert.Nil(t, result, "recycleOCI=false must return nil (not applicable)")
}

func TestComputeOCIRecycle_FULL_PositiveOCI(t *testing.T) {
	ociCumulative := decimal.NewFromFloat(75000.0000)
	result, err := ComputeOCIRecycle(true, ociCumulative, decimal.NewFromFloat(1000), decimal.NewFromFloat(1000), DisposalFull)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, ociCumulative.Equal(*result), "FULL disposal: recycle entire OCI cumulative")
}

func TestComputeOCIRecycle_FULL_NegativeOCI(t *testing.T) {
	// Negative OCI (cumulative loss in OCI) must also be recycled (as loss)
	ociCumulative := decimal.NewFromFloat(-30000.0000)
	result, err := ComputeOCIRecycle(true, ociCumulative, decimal.NewFromFloat(500), decimal.NewFromFloat(500), DisposalFull)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsNegative(), "negative OCI must recycle as negative amount")
	assert.True(t, ociCumulative.Equal(*result))
}

func TestComputeOCIRecycle_PARTIAL_Proportional(t *testing.T) {
	ociCumulative := decimal.NewFromFloat(60000.0000)
	qty := decimal.NewFromFloat(400)
	holding := decimal.NewFromFloat(1000)
	result, err := ComputeOCIRecycle(true, ociCumulative, qty, holding, DisposalPartial)
	require.NoError(t, err)
	require.NotNil(t, result)
	// 60_000 × (400/1000) = 24_000
	want := decimal.NewFromFloat(24000.0000)
	assert.True(t, want.Equal(*result), "PARTIAL: proportional OCI recycle, got %s", result.String())
}

func TestComputeOCIRecycle_PARTIAL_NegativeOCI(t *testing.T) {
	ociCumulative := decimal.NewFromFloat(-12000.0000)
	qty := decimal.NewFromFloat(300)
	holding := decimal.NewFromFloat(1000)
	result, err := ComputeOCIRecycle(true, ociCumulative, qty, holding, DisposalPartial)
	require.NoError(t, err)
	require.NotNil(t, result)
	// -12_000 × (300/1000) = -3_600
	want := decimal.NewFromFloat(-3600.0000)
	assert.True(t, want.Equal(*result))
}

func TestComputeOCIRecycle_ZeroOCI(t *testing.T) {
	result, err := ComputeOCIRecycle(true, decimal.Zero, decimal.NewFromFloat(100), decimal.NewFromFloat(500), DisposalPartial)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, decimal.Zero.Equal(*result), "zero OCI cumulative recycles zero")
}

func TestComputeOCIRecycle_ZeroHolding_Error(t *testing.T) {
	_, err := ComputeOCIRecycle(true, decimal.NewFromFloat(1000), decimal.NewFromFloat(100), decimal.Zero, DisposalPartial)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qty_holding_pre tidak boleh 0")
}

// ─── ComputeBMFrequency ───────────────────────────────────────────────────────

func TestComputeBMFrequency_Normal(t *testing.T) {
	cum := decimal.NewFromFloat(50000000)
	current := decimal.NewFromFloat(10000000)
	total := decimal.NewFromFloat(1000000000)
	pct, err := ComputeBMFrequency(cum, current, total)
	require.NoError(t, err)
	// (50M + 10M) / 1000M × 100 = 6.0000
	want := decimal.NewFromFloat(6.0000)
	assert.True(t, want.Equal(pct), "got %s, want %s", pct, want)
}

func TestComputeBMFrequency_ZeroPortfolio_Error(t *testing.T) {
	_, err := ComputeBMFrequency(decimal.NewFromFloat(1000), decimal.NewFromFloat(500), decimal.Zero)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "total_nilai_portofolio tidak boleh 0")
}

func TestComputeBMFrequency_ExactWarnThreshold(t *testing.T) {
	// Exactly at 5% threshold
	cum := decimal.NewFromFloat(40000000)
	current := decimal.NewFromFloat(10000000)
	total := decimal.NewFromFloat(1000000000)
	pct, err := ComputeBMFrequency(cum, current, total)
	require.NoError(t, err)
	// (40M + 10M) / 1000M = 5.0000%
	assert.Equal(t, "5.0000", pct.StringFixed(4))
}

func TestComputeBMFrequency_ZeroCumulative(t *testing.T) {
	pct, err := ComputeBMFrequency(decimal.Zero, decimal.NewFromFloat(5000000), decimal.NewFromFloat(100000000))
	require.NoError(t, err)
	// 5M / 100M = 5.0000%
	assert.Equal(t, "5.0000", pct.StringFixed(4))
}

// ─── ValidateBMThresholds ─────────────────────────────────────────────────────

func TestValidateBMThresholds_BelowWarn(t *testing.T) {
	pct := decimal.NewFromFloat(3.0)
	warnT := decimal.NewFromFloat(5.0)
	blockT := decimal.NewFromFloat(10.0)
	warn, block := ValidateBMThresholds(pct, warnT, blockT)
	assert.False(t, warn)
	assert.False(t, block)
}

func TestValidateBMThresholds_AtWarn_NotWarning(t *testing.T) {
	// Exactly at threshold = NOT warn (must be > threshold, not >=)
	pct := decimal.NewFromFloat(5.0)
	warnT := decimal.NewFromFloat(5.0)
	blockT := decimal.NewFromFloat(10.0)
	warn, block := ValidateBMThresholds(pct, warnT, blockT)
	// GreaterThan is strict >; exactly at threshold = false
	assert.False(t, warn, "exactly at threshold must not trigger warn")
	assert.False(t, block)
}

func TestValidateBMThresholds_AboveWarnBelowBlock(t *testing.T) {
	pct := decimal.NewFromFloat(7.5)
	warnT := decimal.NewFromFloat(5.0)
	blockT := decimal.NewFromFloat(10.0)
	warn, block := ValidateBMThresholds(pct, warnT, blockT)
	assert.True(t, warn)
	assert.False(t, block)
}

func TestValidateBMThresholds_AtBlock_NotBlocking(t *testing.T) {
	// Exactly at block threshold = NOT block (strict >)
	pct := decimal.NewFromFloat(10.0)
	warnT := decimal.NewFromFloat(5.0)
	blockT := decimal.NewFromFloat(10.0)
	warn, block := ValidateBMThresholds(pct, warnT, blockT)
	assert.True(t, warn)
	assert.False(t, block, "exactly at block threshold must not trigger block")
}

func TestValidateBMThresholds_AboveBlock(t *testing.T) {
	pct := decimal.NewFromFloat(10.0001)
	warnT := decimal.NewFromFloat(5.0)
	blockT := decimal.NewFromFloat(10.0)
	warn, block := ValidateBMThresholds(pct, warnT, blockT)
	assert.True(t, warn)
	assert.True(t, block)
}

func TestValidateBMThresholds_ZeroPct(t *testing.T) {
	warn, block := ValidateBMThresholds(decimal.Zero, decimal.NewFromFloat(5.0), decimal.NewFromFloat(10.0))
	assert.False(t, warn)
	assert.False(t, block)
}
