package renewal

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeBungaKotor validates bunga kotor = pokok × rate × hari / 365
func TestComputeBungaKotor(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(1_000_000_000)    // 1 miliar
	rate := decimal.NewFromFloat(6.0)             // 6% p.a.

	result := ComputeBungaKotor(pokok, rate, penempatan, efektif)
	// 181 days from Jan 1 to Jul 1 2026
	// expected = 1_000_000_000 × 6 / 100 × 181 / 365
	days := int64(efektif.Sub(penempatan).Hours() / 24)
	expected := decimal.NewFromInt(1_000_000_000).
		Mul(decimal.NewFromFloat(6.0)).
		Div(decimalHundred).
		Mul(decimal.NewFromInt(days)).
		Div(decimal.NewFromInt(365))

	assert.Equal(t, expected.StringFixed(4), result.StringFixed(4),
		"bunga_kotor mismatch for %d days", days)
}

// TestComputeBungaKotor_ZeroRate should return zero
func TestComputeBungaKotor_ZeroRate(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(1_000_000_000)
	rate := decimal.Zero

	result := ComputeBungaKotor(pokok, rate, penempatan, efektif)
	assert.True(t, result.IsZero())
}

// TestComputeBungaKotor_SameDay should return zero
func TestComputeBungaKotor_SameDay(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	result := ComputeBungaKotor(decimal.NewFromInt(1_000_000), decimal.NewFromFloat(5.0), t0, t0)
	assert.True(t, result.IsZero(), "same-day should produce zero bunga")
}

// TestComputePPh validates PPh = bunga_kotor × 0.20
func TestComputePPh(t *testing.T) {
	cases := []struct {
		bungaKotor string
		expected   string
	}{
		{"30000000.0000", "6000000.0000"},
		{"1000000.0000", "200000.0000"},
		{"0.0000", "0.0000"},
		{"100.0000", "20.0000"},
	}
	for _, tc := range cases {
		bk, _ := decimal.NewFromString(tc.bungaKotor)
		result := ComputePPh(bk)
		assert.Equal(t, tc.expected, result.StringFixed(4), "input=%s", tc.bungaKotor)
	}
}

// TestComputeBungaBersih validates bunga_bersih = bunga_kotor - pph
func TestComputeBungaBersih(t *testing.T) {
	bk := decimal.NewFromInt(30_000_000)
	pph := decimal.NewFromInt(6_000_000)
	result := ComputeBungaBersih(bk, pph)
	assert.Equal(t, "24000000.0000", result.StringFixed(4))
}

// TestComputePokokBaru_PokokSaja validates pokok_baru = pokok_lama
func TestComputePokokBaru_PokokSaja(t *testing.T) {
	pokokLama := decimal.NewFromInt(500_000_000)
	bungaBersih := decimal.NewFromInt(10_000_000)
	result, err := ComputePokokBaru(SkemaPokokSaja, pokokLama, bungaBersih)
	require.NoError(t, err)
	assert.Equal(t, pokokLama.StringFixed(4), result.StringFixed(4))
}

// TestComputePokokBaru_PokokPlusBunga validates pokok_baru = pokok_lama + bunga_bersih
func TestComputePokokBaru_PokokPlusBunga(t *testing.T) {
	pokokLama := decimal.NewFromInt(500_000_000)
	bungaBersih := decimal.NewFromInt(10_000_000)
	result, err := ComputePokokBaru(SkemaPokokPlusBunga, pokokLama, bungaBersih)
	require.NoError(t, err)
	assert.Equal(t, "510000000.0000", result.StringFixed(4))
}

// TestComputePokokBaru_InvalidSkema should return error
func TestComputePokokBaru_InvalidSkema(t *testing.T) {
	_, err := ComputePokokBaru("INVALID", decimal.NewFromInt(1_000_000), decimal.NewFromInt(100_000))
	require.Error(t, err)
}

// TestBuildCashflowsAfterTax validates cashflow array structure
func TestBuildCashflowsAfterTax(t *testing.T) {
	pokok := decimal.NewFromInt(100_000_000) // 100 juta
	rate := decimal.NewFromFloat(6.0)        // 6% p.a.
	tenor := 3

	cfs := BuildCashflowsAfterTax(pokok, rate, tenor)

	require.Len(t, cfs, 4, "should have tenor+1 entries (t=0..tenor)")

	// cfs[0] should be negative outflow
	assert.True(t, cfs[0].IsNegative(), "cfs[0] should be negative (outflow)")
	assert.Equal(t, pokok.Neg().StringFixed(4), cfs[0].StringFixed(4))

	// cfs[1] and cfs[2] should be positive kupon bersih (after 20% PPh)
	for i := 1; i < tenor; i++ {
		assert.True(t, cfs[i].IsPositive(), "cfs[%d] should be positive", i)
	}

	// cfs[tenor] = pokok + kupon_bersih (positive)
	assert.True(t, cfs[tenor].IsPositive(), "cfs[tenor] should be positive")
	assert.True(t, cfs[tenor].GreaterThan(pokok), "cfs[tenor] should include pokok + kupon")
}

// TestComputePreview_PokokSaja validates full preview computation
func TestComputePreview_PokokSaja(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(1_000_000_000)
	rateLama := decimal.NewFromFloat(6.0)
	rateBaru := decimal.NewFromFloat(7.0)
	tenor := 12

	preview, err := ComputePreview(pokok, rateLama, penempatan, SkemaPokokSaja, rateBaru, tenor, efektif)
	require.NoError(t, err)

	// pokok_baru = pokok_lama for POKOK_SAJA
	assert.Equal(t, pokok.StringFixed(4), preview.PokokBaru.StringFixed(4))

	// bunga_kotor > 0
	assert.True(t, preview.BungaKotor.IsPositive())

	// pph = 20% of bunga_kotor
	expectedPph := preview.BungaKotor.Mul(decimal.NewFromFloat(0.20)).RoundBank(4)
	assert.Equal(t, expectedPph.StringFixed(4), preview.Pph20pct.StringFixed(4))

	// bunga_bersih = bunga_kotor - pph
	expectedBB := preview.BungaKotor.Sub(preview.Pph20pct)
	assert.Equal(t, expectedBB.StringFixed(4), preview.BungaBersih.StringFixed(4))

	// EIR should be reasonable (> 0, < 1)
	assert.True(t, preview.EirBaru.IsPositive())
	assert.True(t, preview.EirBaru.LessThan(decimal.NewFromFloat(1.0)))

	// TanggalJatuhTempoBaru = efektif + tenor months
	expectedJT := AddMonths(efektif, tenor)
	assert.Equal(t, expectedJT.Format("2006-01-02"), preview.TanggalJatuhTempoBaru.Format("2006-01-02"))
}

// TestComputePreview_PokokPlusBunga validates that pokok_baru > pokok_lama
func TestComputePreview_PokokPlusBunga(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(1_000_000_000)
	rateLama := decimal.NewFromFloat(6.0)
	rateBaru := decimal.NewFromFloat(7.0)
	tenor := 12

	preview, err := ComputePreview(pokok, rateLama, penempatan, SkemaPokokPlusBunga, rateBaru, tenor, efektif)
	require.NoError(t, err)

	assert.True(t, preview.PokokBaru.GreaterThan(pokok),
		"POKOK_PLUS_BUNGA: pokok_baru should be > pokok_lama")
}

// TestBuildCashflowsAfterTax_ZeroTenor returns nil for tenor < 1
func TestBuildCashflowsAfterTax_ZeroTenor(t *testing.T) {
	pokok := decimal.NewFromInt(1_000_000)
	rate := decimal.NewFromFloat(6.0)
	result := BuildCashflowsAfterTax(pokok, rate, 0)
	assert.Nil(t, result, "zero tenor should return nil cashflows")

	result2 := BuildCashflowsAfterTax(pokok, rate, -1)
	assert.Nil(t, result2, "negative tenor should return nil cashflows")
}

// TestComputePreview_InvalidSkema returns error from ComputePokokBaru
func TestComputePreview_InvalidSkema(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(1_000_000_000)
	_, err := ComputePreview(pokok, decimal.NewFromFloat(6.0), penempatan,
		Skema("INVALID"), decimal.NewFromFloat(7.0), 12, efektif)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ComputePreview")
}

// TestComputePreview_EIRNoConverge should not occur for normal bank deposits
// but we verify no panic occurs for edge case.
func TestComputePreview_SmallTenor(t *testing.T) {
	penempatan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	efektif := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	pokok := decimal.NewFromInt(10_000_000)
	rateLama := decimal.NewFromFloat(5.0)
	rateBaru := decimal.NewFromFloat(5.0)
	tenor := 1

	// Tenor=1 is valid; EIR monthly = rate_net/12
	preview, err := ComputePreview(pokok, rateLama, penempatan, SkemaPokokSaja, rateBaru, tenor, efektif)
	require.NoError(t, err)
	assert.True(t, preview.EirBaru.IsPositive())
}
