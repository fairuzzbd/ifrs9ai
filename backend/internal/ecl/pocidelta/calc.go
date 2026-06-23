package pocidelta

// calc.go — Pure computation functions for POCI Delta ECL.
//
// All functions are STATELESS and use shopspring/decimal (DEC-016 — never float64).
// 100% test coverage required (compliance-critical: PSAK 71 §5.5.13-14).
//
// Rounding: HALF_EVEN (banker's rounding) per SoW_v1.4 §4.
// Delta stored as NUMERIC(20,4) → RoundBank(4).
//
// Citations:
//   - PSAK 71 §5.5.13 — POCI recognition, credit-adjusted EIR
//   - PSAK 71 §5.5.14 — ECL movement: only delta recognised in P&L
//   - FSD-APP-C-ECL-EIR-v1.0 §5 — POCI computation
//   - DEC-010 — default weights Good 0.25 / Normal 0.50 / Bad 0.25
//   - DEC-016 — shopspring/decimal, NUMERIC(20,4) IDR

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	decZero = decimal.Zero
)

// ComputeDelta computes the signed delta ECL and its direction.
//
// Per PSAK 71 §5.5.14:
//
//	delta_ecl = current_lifetime_ecl − baseline_lifetime_ecl
//	direction = INCREASE  if delta_ecl > 0  (credit quality deteriorated)
//	          = DECREASE  if delta_ecl < 0  (credit quality improved)
//	          = ZERO      if delta_ecl == 0 (exact decimal comparison)
//
// Returns (delta, direction). No error — inputs are validated by caller.
// Result rounded to NUMERIC(20,4) per DEC-016 (HALF_EVEN).
func ComputeDelta(currentECL, baselineECL decimal.Decimal) (delta decimal.Decimal, dir Direction) {
	// delta = current − baseline (shopspring/decimal, exact arithmetic)
	// Round to 4 decimal places (NUMERIC(20,4) storage precision) per SoW §4.
	delta = currentECL.Sub(baselineECL).RoundBank(4)

	switch {
	case delta.IsPositive():
		dir = DirectionIncrease
	case delta.IsNegative():
		dir = DirectionDecrease
	default:
		dir = DirectionZero
	}
	return delta, dir
}

// ResolveJurnalEventCode maps Direction to the P5-M2 jurnal event code.
//
// Sign convention per PSAK 71 §5.5.14:
//
//	INCREASE → "POCI_ECL_DELTA_INCREASE"
//	           D Beban Penurunan Nilai ECL POCI / K Cadangan ECL POCI
//	DECREASE → "POCI_ECL_DELTA_DECREASE"
//	           D Cadangan ECL POCI / K Pendapatan Pemulihan ECL POCI
//	ZERO     → ("", ErrSkipZero) — caller must skip jurnal posting
//
// Returns (eventCode, nil) for INCREASE/DECREASE.
// Returns ("", ErrSkipZero) for ZERO (sentinel — not a fatal error).
// Returns ("", error) for invalid direction (should never happen if domain is correct).
func ResolveJurnalEventCode(dir Direction) (eventCode string, err error) {
	switch dir {
	case DirectionIncrease:
		return "POCI_ECL_DELTA_INCREASE", nil
	case DirectionDecrease:
		return "POCI_ECL_DELTA_DECREASE", nil
	case DirectionZero:
		return "", ErrSkipZero
	default:
		return "", fmt.Errorf("ResolveJurnalEventCode: direction tidak valid: %q (valid: INCREASE, DECREASE, ZERO)", dir)
	}
}

// ValidateJurnalDirection is a defensive bug-guard that checks that delta_ecl
// sign is consistent with the direction enum stored in ecl.poci_delta_log.
//
// This guards against data inconsistency (e.g. a previous bug wrote wrong direction).
// Should be called before any jurnal posting in service.go.
//
// Returns nil if consistent; ErrJurnalDirectionMismatch if not.
//
// Example mismatches (should never occur if ComputeDelta is always used):
//
//	delta_ecl = 200.0000, direction = "DECREASE"  → mismatch
//	delta_ecl = -150.0000, direction = "INCREASE" → mismatch
//	delta_ecl = 0, direction = "INCREASE"         → mismatch
func ValidateJurnalDirection(deltaECL decimal.Decimal, dir Direction) error {
	switch {
	case deltaECL.IsPositive() && dir != DirectionIncrease:
		return fmt.Errorf("%s: delta_ecl %s positif tetapi direction = %q (ekspektasi INCREASE)",
			CodePociJurnalDirectionMismatch, deltaECL.StringFixed(4), dir)
	case deltaECL.IsNegative() && dir != DirectionDecrease:
		return fmt.Errorf("%s: delta_ecl %s negatif tetapi direction = %q (ekspektasi DECREASE)",
			CodePociJurnalDirectionMismatch, deltaECL.StringFixed(4), dir)
	case deltaECL.Equal(decZero) && dir != DirectionZero:
		return fmt.Errorf("%s: delta_ecl = 0 tetapi direction = %q (ekspektasi ZERO)",
			CodePociJurnalDirectionMismatch, dir)
	}
	return nil
}

// AbsDeltaForJurnal returns the absolute value of delta_ecl for use as
// jurnal amount. Jurnal amount is always non-negative; direction enum carries
// the sign semantics (INCREASE / DECREASE).
//
// Per PSAK 71 §5.5.14: "amount" in debit/kredit lines is always positive;
// the direction of the movement is expressed by which account is debited.
func AbsDeltaForJurnal(deltaECL decimal.Decimal) decimal.Decimal {
	return deltaECL.Abs().RoundBank(4)
}
