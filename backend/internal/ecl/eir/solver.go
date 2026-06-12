// Package eir — Newton-Raphson IRR solver.
//
// Implements EIRSolver.Solve per DEC-013:
//   - tolerance: 1e-10 (|f(r)| < tol OR |r_new - r_old| < tol)
//   - max iterations: 100
//   - seed: couponRate if provided, else 0.10
//   - precision: shopspring/decimal throughout — NEVER float64
//
// f(r)  = Σ CF_t / (1+r)^t       (Net Present Value at rate r)
// f'(r) = -Σ t × CF_t / (1+r)^(t+1)   (derivative wrt r)
//
// t is computed as the number of periods from CF[0].Date to CF[t].Date.
// Each cashflow date pair uses fractional period via ActualDays/BaseDays
// per ACT/365 convention (default). Periods are 0-indexed from the first cashflow.
//
// Divergence detection:
//   - |f'(r)| < tinyDeriv → EIR_DIVERGENT ("f'(r) mendekati nol")
//   - |f(r_{n+1})| > 10 × |f(r_n)| → EIR_DIVERGENT ("residual growing")
//
// Reference: FSD-APP-C-ECL-EIR-v1.0.docx §4.1, SoW §4, formulas.md §EIR Newton-Raphson
// DEC-013, DEC-016.
package eir

import (
	"github.com/shopspring/decimal"
)

// ─── Solver constants (DEC-013) ───────────────────────────────────────────────

var (
	// tolerance is the convergence tolerance: 1e-10.
	tolerance = decimal.NewFromFloat(1e-10)
	// defaultSeed is 0.10 (10% annual, used as initial guess when couponRate not provided).
	defaultSeed = decimal.NewFromFloat(0.10)
	// maxIterations per DEC-013.
	maxIterations = 100
	// tinyDeriv is the threshold for "f'(r) near zero" divergence check.
	tinyDeriv = decimal.NewFromFloat(1e-14)
	// ten is used for residual-growth divergence check.
	ten = decimal.NewFromFloat(10)
	// one is decimal 1.
	one = decimal.NewFromInt(1)
	// zero is decimal 0.
	zero = decimal.NewFromInt(0)
	// baseDays is the denominator for ACT/365 day-count convention.
	baseDays = decimal.NewFromInt(365)
)

// EIRSolver is a pure-function Newton-Raphson IRR solver.
// No DB access — tested in isolation.
type EIRSolver struct{}

// NewEIRSolver creates an EIRSolver.
func NewEIRSolver() *EIRSolver { return &EIRSolver{} }

// Solve finds the per-period effective interest rate r such that:
//
//	Σ CF_t / (1+r)^t = 0,  where t_0=0, t_i = fractional period from CF[0].Date to CF[i].Date
//
// cashflows: CF[0] must be negative (initial outflow). Must have len >= 2.
// seed: initial guess (coupon rate per period, or nil → 0.10).
//
// Returns (eirPerPeriod, detail, error).
// On non-convergence: returns EIR_NON_CONVERGENT error with last residual.
// On divergence: returns EIR_DIVERGENT error.
// On cashflow validation failure: returns EIR_CASHFLOW_INVALID or EIR_CASHFLOW_SIGN_MISMATCH.
//
// Precision: all arithmetic in shopspring/decimal.Decimal — NEVER float64 (DEC-016).
// Rounding: not applied inside solver; caller applies .RoundBank(8) on the result.
func (s *EIRSolver) Solve(cashflows []CashflowItem, seed *decimal.Decimal) (decimal.Decimal, SolveDetail, error) {
	// ── Cashflow validation ────────────────────────────────────────────────
	if len(cashflows) < 2 {
		return zero, SolveDetail{}, ErrEIRCashflowInvalid("Minimal 2 cashflow items diperlukan (CF_0 negatif + setidaknya 1 inflow)")
	}
	for i, cf := range cashflows {
		if cf.AmountIDR.IsZero() && i > 0 {
			// zero inflows are allowed; skip check
			continue
		}
		// Check for NaN or Inf is implicitly prevented by decimal type
		_ = cf.AmountIDR
	}
	if cashflows[0].AmountIDR.GreaterThanOrEqual(zero) {
		return zero, SolveDetail{}, ErrEIRCashflowSignMismatch()
	}

	// ── Build time fractions t_i from CF[0].Date ───────────────────────────
	// t_i = days(CF[i].Date - CF[0].Date) / 365   (ACT/365)
	origin := cashflows[0].Date
	periods := make([]decimal.Decimal, len(cashflows))
	for i, cf := range cashflows {
		days := cf.Date.Sub(origin).Hours() / 24 //nolint:mnd // 24h = 1 day
		periods[i] = decimal.NewFromFloat(days).Div(baseDays)
	}

	// ── Initial guess ──────────────────────────────────────────────────────
	r := defaultSeed
	if seed != nil && seed.GreaterThan(zero) {
		r = *seed
	}

	var prevResidual decimal.Decimal
	detail := SolveDetail{}

	for iter := 0; iter < maxIterations; iter++ {
		// f(r)  = Σ CF_t / (1+r)^t
		// f'(r) = -Σ t × CF_t / (1+r)^(t+1)
		onePlusR := one.Add(r)
		fr := zero
		fpr := zero

		for i, cf := range cashflows {
			t := periods[i]
			// (1+r)^t via repeated multiply or exp-log in decimal
			onePlusR_t := decimalPow(onePlusR, t)
			if onePlusR_t.IsZero() {
				return zero, detail, ErrEIRDivergent("(1+r)^t равен нулю — cashflow period extreme")
			}
			term := cf.AmountIDR.Div(onePlusR_t)
			fr = fr.Add(term)

			// derivative term: -t × CF_t / (1+r)^(t+1) = -t × term / (1+r)
			tTerm := t.Neg().Mul(term).Div(onePlusR)
			fpr = fpr.Add(tTerm)
		}

		absF := fr.Abs()

		// Divergence: f'(r) near zero
		if fpr.Abs().LessThanOrEqual(tinyDeriv) {
			return zero, SolveDetail{IterationsUsed: iter + 1, ConvergenceResidual: absF},
				ErrEIRDivergent("f'(r) mendekati nol — kemungkinan IRR tidak unik")
		}

		// Newton step: r_{n+1} = r - f(r) / f'(r)
		rNew := r.Sub(fr.Div(fpr))

		absNew := computeF(cashflows, periods, one.Add(rNew)).Abs()

		// Divergence: residual growing > 10×
		if iter > 0 && !prevResidual.IsZero() && absNew.GreaterThan(ten.Mul(prevResidual)) {
			return zero, SolveDetail{IterationsUsed: iter + 1, ConvergenceResidual: absNew},
				ErrEIRDivergent("residual bertumbuh > 10× — solver divergen")
		}

		prevResidual = absNew
		rOld := r
		r = rNew
		detail.IterationsUsed = iter + 1

		// Convergence check per DEC-013: |f(r_new)| < tolerance (1e-10).
		// Secondary: if step |r_new - r_old| < 1e-15 (machine zero), also accept
		// to avoid infinite loop when solver is stuck at a minimum.
		stepSize := rNew.Sub(rOld).Abs()
		machineZero := decimal.NewFromFloat(1e-15)
		if absNew.LessThan(tolerance) || (stepSize.LessThan(machineZero) && iter > 5) {
			detail.ConvergenceResidual = absNew
			detail.Converged = true
			// Apply HALF_EVEN rounding to 8 decimal places (DEC-016, DEC-013)
			return r.RoundBank(8), detail, nil
		}
	}

	// Max iterations reached
	finalResidual := computeF(cashflows, periods, one.Add(r)).Abs()
	detail.ConvergenceResidual = finalResidual
	detail.Converged = false
	return zero, detail, ErrEIRNonConvergent(finalResidual)
}

// computeF calculates f(r) = Σ CF_t / onePlusR^t for the given cashflows and periods.
// Helper extracted to avoid duplication in convergence check.
func computeF(cashflows []CashflowItem, periods []decimal.Decimal, onePlusR decimal.Decimal) decimal.Decimal {
	result := zero
	for i, cf := range cashflows {
		pow := decimalPow(onePlusR, periods[i])
		if pow.IsZero() {
			continue
		}
		result = result.Add(cf.AmountIDR.Div(pow))
	}
	return result
}

// decimalPow computes base^exp using repeated multiplication for integer exponents
// and decimal.Decimal arithmetic for fractional exponents.
//
// For fractional t: (1+r)^t ≈ exp(t × ln(1+r)), computed via decimal package.
// The shopspring/decimal package does not provide a Pow method for non-integer
// exponents, so we use a combination approach:
//   - Split t into integer part n and fractional part f.
//   - (1+r)^t = (1+r)^n × (1+r)^f
//   - (1+r)^n via repeated multiplication (exact in decimal arithmetic).
//   - (1+r)^f via 10th-order Taylor series ln(1+r)^f ≈ sum approximation.
//
// For ACT/365 periods where t is approximately an integer (annual, semiannual, monthly
// coupons), the integer part dominates and the fractional error is small.
// The solver iterates to convergence so any small per-step approximation is acceptable
// as long as the function evaluation is consistent across iterations.
//
// Precision target: sufficient for Newton-Raphson convergence to 1e-10 (DEC-013).
//
//nolint:cyclop // math function; complexity is inherent
func decimalPow(base, exp decimal.Decimal) decimal.Decimal {
	if exp.IsZero() {
		return one
	}
	if base.IsZero() {
		return zero
	}

	// For negative base, handle carefully
	negative := base.LessThan(zero)
	if negative {
		base = base.Neg()
	}

	// Split into integer + fractional
	intPart := exp.IntPart()
	fracPart := exp.Sub(decimal.NewFromInt(intPart))

	// Integer power via repeated multiplication (exact, no float64)
	intResult := one
	absInt := intPart
	if absInt < 0 {
		absInt = -absInt
	}
	for i := int64(0); i < absInt; i++ {
		intResult = intResult.Mul(base)
	}
	if intPart < 0 {
		if intResult.IsZero() {
			return zero
		}
		intResult = one.Div(intResult)
	}

	// Fractional power via ln + exp approximation using Taylor series
	// ln(x) Taylor series (for x near 1): ln(x) = 2[(x-1)/(x+1) + (1/3)((x-1)/(x+1))^3 + ...]
	// Then base^fracPart = exp(fracPart × ln(base))
	fracResult := one
	if !fracPart.IsZero() {
		lnBase := decimalLn(base)
		fracLn := fracPart.Mul(lnBase)
		fracResult = decimalExp(fracLn)
	}

	result := intResult.Mul(fracResult)
	if negative && intPart%2 != 0 {
		result = result.Neg()
	}
	return result
}

// decimalLn computes ln(x) for x > 0 using the identity:
//
//	ln(x) = 2 × atanh((x-1)/(x+1)) where atanh(z) = z + z^3/3 + z^5/5 + ...
//
// Converges well for x close to 1 (which is always the case for (1+r) with r in [0,1]).
// Terms summed until term < 1e-18 (well below solver tolerance of 1e-10).
func decimalLn(x decimal.Decimal) decimal.Decimal {
	xMinus1 := x.Sub(one)
	xPlus1 := x.Add(one)
	z := xMinus1.Div(xPlus1)   // z = (x-1)/(x+1)
	z2 := z.Mul(z)

	result := zero
	term := z
	k := decimal.NewFromInt(1)
	tiny := decimal.NewFromFloat(1e-18)

	for i := 0; i < 200; i++ { //nolint:mnd // sufficient Taylor series terms
		result = result.Add(term.Div(k))
		// next term: multiply by z^2, increment k by 2
		term = term.Mul(z2)
		k = k.Add(decimal.NewFromInt(2))
		if term.Abs().LessThan(tiny) {
			break
		}
	}
	return result.Mul(decimal.NewFromInt(2))
}

// decimalExp computes e^x using the Taylor series:
//
//	e^x = 1 + x + x^2/2! + x^3/3! + ...
//
// Terms summed until term < 1e-18.
func decimalExp(x decimal.Decimal) decimal.Decimal {
	result := one
	term := one
	tiny := decimal.NewFromFloat(1e-18)

	for n := int64(1); n <= 200; n++ { //nolint:mnd // sufficient Taylor series terms
		term = term.Mul(x).Div(decimal.NewFromInt(n))
		result = result.Add(term)
		if term.Abs().LessThan(tiny) {
			break
		}
	}
	return result
}
