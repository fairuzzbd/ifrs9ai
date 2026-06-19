package renewal

// eir.go — Newton-Raphson IRR solver for EIR re-computation on renewal.
//
// DEC-013 (LOCKED): tolerance 1e-10, max 100 iter, presisi 8 desimal.
// Fail-safe: non-convergence or divergence → explicit error (never return garbage).
// ifrs9-compliance-reviewer BLOCKING: cashflows must be after-PPh 20% (PSAK 71 §5.4.1).
//
// 100% unit test coverage required (eir_test.go).

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const (
	// EIRTolerance is the convergence tolerance per DEC-013.
	EIRTolerance = 1e-10
	// EIRMaxIter is the maximum number of iterations per DEC-013.
	EIRMaxIter = 100
)

// ErrEIRNoConvergence is returned when Newton-Raphson fails to converge within EIRMaxIter.
var ErrEIRNoConvergence = fmt.Errorf("EIR Newton-Raphson: tidak konvergen dalam %d iterasi (RENEWAL_EIR_NO_CONVERGENCE)", EIRMaxIter)

// ErrEIREmptyCashflows is returned when cashflows slice is empty.
var ErrEIREmptyCashflows = fmt.Errorf("EIR Newton-Raphson: cashflow array kosong")

// ErrEIRZeroDerivative is returned when the derivative is zero (division by zero risk).
var ErrEIRZeroDerivative = fmt.Errorf("EIR Newton-Raphson: turunan nol — tidak dapat melanjutkan iterasi")

// NewtonRaphsonIRR finds the periodic rate r such that NPV(cashflows, r) = 0.
//
// cashflows[0] should be negative (initial outflow).
// cashflows[1..n] should be positive inflows.
// initial is the starting guess for r (e.g. monthly coupon rate / 12).
//
// Returns the periodic rate (monthly if inputs are monthly) with precision 1e-10.
// Caller is responsible for annualizing if needed: EIR_annual = (1+r)^12 - 1.
//
// Error cases (DEC-013 fail-safe):
//   - Empty cashflows → ErrEIREmptyCashflows
//   - Derivative = 0 → ErrEIRZeroDerivative
//   - Non-convergence in 100 iter → ErrEIRNoConvergence
func NewtonRaphsonIRR(cashflows []decimal.Decimal, initial decimal.Decimal) (decimal.Decimal, error) {
	if len(cashflows) == 0 {
		return decimal.Zero, ErrEIREmptyCashflows
	}

	tolerance := decimal.NewFromFloat(EIRTolerance)
	r := initial

	for iter := 0; iter < EIRMaxIter; iter++ {
		fVal := npv(cashflows, r)
		fPrime := npvDerivative(cashflows, r)

		// Guard: derivative close to zero → solver cannot proceed
		if fPrime.IsZero() || fPrime.Abs().LessThan(tolerance) {
			return decimal.Zero, ErrEIRZeroDerivative
		}

		step := fVal.Div(fPrime)
		rNext := r.Sub(step)

		// Check convergence: |r_next - r| < tolerance
		if rNext.Sub(r).Abs().LessThan(tolerance) {
			return rNext.RoundBank(8), nil
		}

		r = rNext
	}

	return decimal.Zero, ErrEIRNoConvergence
}

// npv computes NPV = Σ CF_t / (1+r)^t
// using decimal arithmetic for full precision.
func npv(cashflows []decimal.Decimal, r decimal.Decimal) decimal.Decimal {
	one := decimal.NewFromInt(1)
	onePlusR := one.Add(r)
	result := decimal.Zero

	divisor := decimal.NewFromInt(1) // (1+r)^0 = 1
	for t, cf := range cashflows {
		if t > 0 {
			divisor = divisor.Mul(onePlusR)
		}
		if divisor.IsZero() {
			continue
		}
		result = result.Add(cf.Div(divisor))
	}
	return result
}

// npvDerivative computes f'(r) = -Σ t × CF_t / (1+r)^(t+1)
func npvDerivative(cashflows []decimal.Decimal, r decimal.Decimal) decimal.Decimal {
	one := decimal.NewFromInt(1)
	onePlusR := one.Add(r)
	result := decimal.Zero

	divisor := onePlusR // (1+r)^1 for t=0 case → contributes 0 anyway
	for t, cf := range cashflows {
		if t == 0 {
			// t=0: contribution = 0 (coefficient is 0)
			divisor = onePlusR
			continue
		}
		// Advance divisor: divisor = (1+r)^(t+1)
		// At t=1: divisor = (1+r)^2; at t=2: (1+r)^3, etc.
		divisor = divisor.Mul(onePlusR)
		if divisor.IsZero() {
			continue
		}
		tDecimal := decimal.NewFromInt(int64(t))
		result = result.Add(tDecimal.Mul(cf).Div(divisor))
	}
	return result.Neg()
}

// BuildAmortisasiSchedule generates an amortisation schedule from EIR and cashflows.
// Returns a slice of monthly entries for storage in ecl.amortisasi_schedule preview.
func BuildAmortisasiSchedule(pokokBaru decimal.Decimal, eirMonthly decimal.Decimal, tenorBulan int,
	rateBaruPersen decimal.Decimal) []AmortisasiEntry {

	result := make([]AmortisasiEntry, 0, tenorBulan)
	balance := pokokBaru
	oneMinusPph := decimal.NewFromFloat(0.80)

	for bulan := 1; bulan <= tenorBulan; bulan++ {
		kuponKotor := balance.Mul(rateBaruPersen).Div(decimalHundred).Div(decimalTwelve).RoundBank(4)
		kuponBersih := kuponKotor.Mul(oneMinusPph).RoundBank(4)
		pph := kuponKotor.Sub(kuponBersih).RoundBank(4)

		var saldoAkhir decimal.Decimal
		if bulan == tenorBulan {
			saldoAkhir = decimal.Zero
		} else {
			saldoAkhir = balance
		}

		result = append(result, AmortisasiEntry{
			Bulan:            bulan,
			BungaKotorBulan:  kuponKotor,
			PphBulan:         pph,
			BungaBersihBulan: kuponBersih,
			SaldoPokokAkhir:  saldoAkhir,
		})
	}
	return result
}

// AmortisasiEntry holds one monthly amortisation schedule row.
type AmortisasiEntry struct {
	Bulan            int
	BungaKotorBulan  decimal.Decimal
	PphBulan         decimal.Decimal
	BungaBersihBulan decimal.Decimal
	SaldoPokokAkhir  decimal.Decimal
}
