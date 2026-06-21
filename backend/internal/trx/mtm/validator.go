package mtm

// validator.go — Price range, deviation, and stale threshold validation for MTM.
//
// All validation functions are pure (no DB I/O) so they are easily unit-tested.
// DB-dependent checks (instrumen exists, periode OPEN) live in service.go.

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ─── Threshold defaults ───────────────────────────────────────────────────────

const (
	// DefaultDeviationThresholdPct is the default value for sys.config MTM_PRICE_DEVIATION_THRESHOLD_PCT.
	DefaultDeviationThresholdPct = 5.0

	// DefaultStalePriceDays is the default value for sys.config MTM_PRICE_STALE_DAYS.
	DefaultStalePriceDays = 5

	// DefaultStaleEscalationDays is the default value for sys.config MTM_STALE_ESCALATION_DAYS.
	DefaultStaleEscalationDays = 7
)

// ─── Price validation ─────────────────────────────────────────────────────────

// ValidatePricePositive returns an error if hargaPasar is not > 0.
// DEC-016: uses decimal.Decimal (never float64).
func ValidatePricePositive(hargaPasar decimal.Decimal, fieldName string) error {
	if hargaPasar.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%s harus > 0 (diterima: %s)", fieldName, hargaPasar.String())
	}
	return nil
}

// ValidateBookValuePositive returns an error if hargaBukuIdr is not > 0.
// Stage 3 FVTPL: harga_buku_idr = Net Carrying (Gross − ECL).
// If Net Carrying is ≤ 0, MTM computation is undefined — caller must flag STALE_PRICE.
func ValidateBookValuePositive(hargaBukuIdr decimal.Decimal) error {
	if hargaBukuIdr.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("harga_buku_idr harus > 0 untuk MTM dapat dihitung (diterima: %s)", hargaBukuIdr.String())
	}
	return nil
}

// ─── Stale price check ────────────────────────────────────────────────────────

// ComputeHargaAgeDays returns tanggalMtm − hargaTanggal in days (non-negative).
// Returns 999 if hargaTanggal is zero (price not found in feed).
func ComputeHargaAgeDays(tanggalMtm, hargaTanggal time.Time) int16 {
	if hargaTanggal.IsZero() {
		return 999 // sentinel: price not found
	}
	// Truncate to date (midnight) for clean day diff regardless of time component.
	t1 := time.Date(tanggalMtm.Year(), tanggalMtm.Month(), tanggalMtm.Day(), 0, 0, 0, 0, time.UTC)
	t2 := time.Date(hargaTanggal.Year(), hargaTanggal.Month(), hargaTanggal.Day(), 0, 0, 0, 0, time.UTC)
	diff := t1.Sub(t2).Hours() / 24
	if diff < 0 {
		return 0 // harga_tanggal in future — treat as fresh
	}
	return int16(diff)
}

// IsStalePriceByAge returns true if harga_age_days > stalePriceDays threshold.
// stalePriceDays should be read from sys.config MTM_PRICE_STALE_DAYS (default 5).
func IsStalePriceByAge(hargaAgeDays int16, stalePriceDays int) bool {
	return int(hargaAgeDays) > stalePriceDays
}

// IsStalePriceEscalation returns true if harga_age_days > escalation threshold.
// Triggers additional notify to ROLE-RISK per state machine §4.
// escalationDays should be MTM_STALE_ESCALATION_DAYS (default 7).
func IsStalePriceEscalation(hargaAgeDays int16, escalationDays int) bool {
	return int(hargaAgeDays) > escalationDays
}

// ─── Deviation check ──────────────────────────────────────────────────────────

// ComputeDelta computes delta_idr and delta_pct.
// delta_idr = harga_pasar_idr - harga_buku_idr
// delta_pct  = (delta_idr / harga_buku_idr) × 100
//
// Returns (deltaIdr, deltaPct, error).
// Returns error if hargaBukuIdr == 0 (division by zero).
// Uses HALF_EVEN banker's rounding per DEC-016.
func ComputeDelta(hargaPasarIdr, hargaBukuIdr decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	if hargaBukuIdr.IsZero() {
		return decimal.Zero, decimal.Zero,
			fmt.Errorf("harga_buku_idr = 0: tidak dapat menghitung delta_pct (pembagian dengan nol)")
	}
	deltaIdr := hargaPasarIdr.Sub(hargaBukuIdr)
	hundred := decimal.NewFromInt(100)
	deltaPct := deltaIdr.Div(hargaBukuIdr).Mul(hundred).RoundBank(4)
	return deltaIdr, deltaPct, nil
}

// IsDeviationExceeded returns true if ABS(delta_pct) > threshold.
// thresholdPct should come from sys.config MTM_PRICE_DEVIATION_THRESHOLD_PCT (default 5.0).
func IsDeviationExceeded(deltaPct decimal.Decimal, thresholdPct decimal.Decimal) bool {
	return deltaPct.Abs().GreaterThan(thresholdPct)
}

// ─── Override comment validation ─────────────────────────────────────────────

// ValidateOverrideComment returns an error if comment is shorter than minLength.
// Per state machine §2.2: override_comment ≥ 30 chars when status=REJECTED.
func ValidateOverrideComment(comment string, minLength int) error {
	if len([]rune(comment)) < minLength {
		return fmt.Errorf("comment wajib minimal %d karakter (diterima: %d karakter)", minLength, len([]rune(comment)))
	}
	return nil
}

// MinOverrideCommentLen is the minimum comment length for override-approve/reject.
const MinOverrideCommentLen = 30

// ─── Date validation ──────────────────────────────────────────────────────────

// ParseDateStrict parses "YYYY-MM-DD" strictly, returning time.Time at midnight UTC.
func ParseDateStrict(dateStr string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("format tanggal tidak valid (harus YYYY-MM-DD): %w", err)
	}
	return t, nil
}

// IsWeekend returns true if t is Saturday or Sunday (Sabtu atau Minggu).
func IsWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}
