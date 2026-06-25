package mtm

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ValidatePricePositive ────────────────────────────────────────────────────

func TestValidatePricePositive_Zero(t *testing.T) {
	err := ValidatePricePositive(decimal.Zero, "harga_pasar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harus > 0")
}

func TestValidatePricePositive_Negative(t *testing.T) {
	err := ValidatePricePositive(decimal.NewFromFloat(-1.5), "harga_pasar")
	require.Error(t, err)
}

func TestValidatePricePositive_Positive(t *testing.T) {
	err := ValidatePricePositive(decimal.NewFromFloat(100.50), "harga_pasar")
	assert.NoError(t, err)
}

func TestValidatePricePositive_TinyPositive(t *testing.T) {
	err := ValidatePricePositive(decimal.NewFromFloat(0.0001), "harga_pasar")
	assert.NoError(t, err)
}

// ─── ValidateBookValuePositive ────────────────────────────────────────────────

func TestValidateBookValuePositive_Zero(t *testing.T) {
	err := ValidateBookValuePositive(decimal.Zero)
	require.Error(t, err)
}

func TestValidateBookValuePositive_Negative(t *testing.T) {
	err := ValidateBookValuePositive(decimal.NewFromFloat(-1))
	require.Error(t, err)
}

func TestValidateBookValuePositive_Positive(t *testing.T) {
	err := ValidateBookValuePositive(decimal.NewFromFloat(1_000_000))
	assert.NoError(t, err)
}

// ─── ComputeHargaAgeDays ──────────────────────────────────────────────────────

func TestComputeHargaAgeDays_ZeroHargaTanggal(t *testing.T) {
	tanggalMtm := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	days := ComputeHargaAgeDays(tanggalMtm, time.Time{})
	assert.Equal(t, int16(999), days, "sentinel 999 when harga_tanggal is zero")
}

func TestComputeHargaAgeDays_SameDay(t *testing.T) {
	d := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	days := ComputeHargaAgeDays(d, d)
	assert.Equal(t, int16(0), days)
}

func TestComputeHargaAgeDays_OneDayAgo(t *testing.T) {
	mtm := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	harga := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	days := ComputeHargaAgeDays(mtm, harga)
	assert.Equal(t, int16(1), days)
}

func TestComputeHargaAgeDays_FiveDays(t *testing.T) {
	mtm := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	harga := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	days := ComputeHargaAgeDays(mtm, harga)
	assert.Equal(t, int16(5), days)
}

func TestComputeHargaAgeDays_FuturePriceDate(t *testing.T) {
	mtm := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	harga := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC) // future
	days := ComputeHargaAgeDays(mtm, harga)
	assert.Equal(t, int16(0), days, "future harga_tanggal treated as 0 (fresh)")
}

func TestComputeHargaAgeDays_IgnoresTimeComponent(t *testing.T) {
	mtm := time.Date(2026, 6, 10, 23, 59, 59, 0, time.UTC)
	harga := time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC)
	days := ComputeHargaAgeDays(mtm, harga)
	assert.Equal(t, int16(3), days)
}

// ─── IsStalePriceByAge ────────────────────────────────────────────────────────

func TestIsStalePriceByAge_BelowThreshold(t *testing.T) {
	assert.False(t, IsStalePriceByAge(5, 5), "exactly at threshold is NOT stale")
}

func TestIsStalePriceByAge_AboveThreshold(t *testing.T) {
	assert.True(t, IsStalePriceByAge(6, 5), "6 > 5 threshold → stale")
}

func TestIsStalePriceByAge_Zero(t *testing.T) {
	assert.False(t, IsStalePriceByAge(0, 5))
}

func TestIsStalePriceByAge_Sentinel999(t *testing.T) {
	assert.True(t, IsStalePriceByAge(999, 5), "sentinel 999 always stale")
}

// ─── IsStalePriceEscalation ───────────────────────────────────────────────────

func TestIsStalePriceEscalation_BelowEscalation(t *testing.T) {
	assert.False(t, IsStalePriceEscalation(7, 7))
}

func TestIsStalePriceEscalation_AboveEscalation(t *testing.T) {
	assert.True(t, IsStalePriceEscalation(8, 7))
}

// ─── ComputeDelta ─────────────────────────────────────────────────────────────

func TestComputeDelta_DivisionByZero(t *testing.T) {
	_, _, err := ComputeDelta(decimal.NewFromFloat(100), decimal.Zero)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pembagian dengan nol")
}

func TestComputeDelta_PositiveGain(t *testing.T) {
	// 110 - 100 = 10, pct = 10/100*100 = 10.0%
	deltaIdr, deltaPct, err := ComputeDelta(
		decimal.NewFromFloat(110),
		decimal.NewFromFloat(100),
	)
	require.NoError(t, err)
	assert.Equal(t, "10.0000", deltaIdr.StringFixed(4))
	assert.Equal(t, "10.0000", deltaPct.StringFixed(4))
}

func TestComputeDelta_NegativeLoss(t *testing.T) {
	// 95 - 100 = -5, pct = -5/100*100 = -5.0%
	deltaIdr, deltaPct, err := ComputeDelta(
		decimal.NewFromFloat(95),
		decimal.NewFromFloat(100),
	)
	require.NoError(t, err)
	assert.True(t, deltaIdr.IsNegative())
	assert.Equal(t, "-5.0000", deltaPct.StringFixed(4))
}

func TestComputeDelta_ZeroGain(t *testing.T) {
	// pasar == buku → delta = 0
	deltaIdr, deltaPct, err := ComputeDelta(
		decimal.NewFromFloat(100),
		decimal.NewFromFloat(100),
	)
	require.NoError(t, err)
	assert.True(t, deltaIdr.IsZero())
	assert.True(t, deltaPct.IsZero())
}

func TestComputeDelta_Precision(t *testing.T) {
	// 1_000_000_000.12345678 - 999_999_999.87654322 = 0.24691356 IDR
	// deltaPct = (0.24691356 / 999999999.87654322) * 100 ≈ 2.47e-8 %
	// After RoundBank(4) that rounds to 0.0000, which IsZero() == true.
	// We only assert deltaIdr is non-zero; deltaPct precision is lossy at 4dp.
	deltaIdr, _, err := ComputeDelta(
		decimal.RequireFromString("1000000000.12345678"),
		decimal.RequireFromString("999999999.87654322"),
	)
	require.NoError(t, err)
	assert.False(t, deltaIdr.IsZero())
}

// ─── IsDeviationExceeded ──────────────────────────────────────────────────────

func TestIsDeviationExceeded_ExactlyAtThreshold(t *testing.T) {
	threshold := decimal.NewFromFloat(5.0)
	// 5.0 > 5.0 is FALSE
	assert.False(t, IsDeviationExceeded(decimal.NewFromFloat(5.0), threshold))
}

func TestIsDeviationExceeded_JustAbove(t *testing.T) {
	threshold := decimal.NewFromFloat(5.0)
	assert.True(t, IsDeviationExceeded(decimal.NewFromFloat(5.0001), threshold))
}

func TestIsDeviationExceeded_Negative_AbsChecked(t *testing.T) {
	// -6.0 → abs = 6.0 > 5.0 → exceeded
	threshold := decimal.NewFromFloat(5.0)
	assert.True(t, IsDeviationExceeded(decimal.NewFromFloat(-6.0), threshold))
}

func TestIsDeviationExceeded_Negative_BelowThreshold(t *testing.T) {
	// -3.0 → abs = 3.0 ≤ 5.0 → not exceeded
	threshold := decimal.NewFromFloat(5.0)
	assert.False(t, IsDeviationExceeded(decimal.NewFromFloat(-3.0), threshold))
}

// ─── ValidateOverrideComment ──────────────────────────────────────────────────

func TestValidateOverrideComment_TooShort(t *testing.T) {
	err := ValidateOverrideComment("short", 30)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30 karakter")
}

func TestValidateOverrideComment_ExactMinLength(t *testing.T) {
	// 30 chars exactly → OK
	comment := "abcdefghijklmnopqrstuvwxyz1234"
	err := ValidateOverrideComment(comment, 30)
	assert.NoError(t, err)
}

func TestValidateOverrideComment_ExactMinLengthUnicode(t *testing.T) {
	// Unicode runes (each is one rune, may be multibyte in UTF-8)
	comment := "Ini adalah komentar yang cukup panjang untuk override"
	err := ValidateOverrideComment(comment, 30)
	assert.NoError(t, err)
}

func TestValidateOverrideComment_Empty(t *testing.T) {
	err := ValidateOverrideComment("", 30)
	require.Error(t, err)
}

// ─── ParseDateStrict ──────────────────────────────────────────────────────────

func TestParseDateStrict_Valid(t *testing.T) {
	t1, err := ParseDateStrict("2026-06-10")
	require.NoError(t, err)
	assert.Equal(t, 2026, t1.Year())
	assert.Equal(t, time.June, t1.Month())
	assert.Equal(t, 10, t1.Day())
}

func TestParseDateStrict_InvalidFormat(t *testing.T) {
	_, err := ParseDateStrict("10-06-2026")
	require.Error(t, err)
}

func TestParseDateStrict_InvalidDate(t *testing.T) {
	_, err := ParseDateStrict("2026-13-01") // month 13
	require.Error(t, err)
}

func TestParseDateStrict_Empty(t *testing.T) {
	_, err := ParseDateStrict("")
	require.Error(t, err)
}

// ─── IsWeekend ────────────────────────────────────────────────────────────────

func TestIsWeekend_Saturday(t *testing.T) {
	// 2026-06-13 is Saturday
	sat := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	assert.True(t, IsWeekend(sat))
}

func TestIsWeekend_Sunday(t *testing.T) {
	// 2026-06-14 is Sunday
	sun := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	assert.True(t, IsWeekend(sun))
}

func TestIsWeekend_Monday(t *testing.T) {
	// 2026-06-15 is Monday
	mon := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	assert.False(t, IsWeekend(mon))
}

func TestIsWeekend_Friday(t *testing.T) {
	// 2026-06-12 is Friday
	fri := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	assert.False(t, IsWeekend(fri))
}

// ─── Constants ────────────────────────────────────────────────────────────────

func TestDefaultConstants(t *testing.T) {
	assert.Equal(t, 5.0, DefaultDeviationThresholdPct)
	assert.Equal(t, 5, DefaultStalePriceDays)
	assert.Equal(t, 7, DefaultStaleEscalationDays)
	assert.Equal(t, 30, MinOverrideCommentLen)
}
