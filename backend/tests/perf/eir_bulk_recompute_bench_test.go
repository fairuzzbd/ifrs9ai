// Package perf — P4-M12 performance benchmark for EIR bulk re-estimation.
//
// SLA assertion: BulkRecompute 1000 instruments ≤ 5 seconds wall-clock.
// Tests pure Newton-Raphson solver computation (no DB) to isolate algorithm perf.
// Real DB latency must be benchmarked separately via k6 load test.
//
// Decision refs:
//   DEC-013: Newton-Raphson IRR solver, tolerance 1e-10, max 100 iter.
//   DEC-016: shopspring/decimal throughout — no float64 for rates.
//
// Run:
//   go test ./tests/perf/... -bench=BenchmarkEIRBulkRecompute1000 -benchtime=3s
package perf

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/ecl/eir"
)

// ─── Test data generators ─────────────────────────────────────────────────────

// baseDate is the reference date for cashflow construction.
var baseDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// buildObligasiCashflows builds a simple annual-coupon bond cashflow.
// CF[0] = -principal (initial outflow), CF[1..n] = coupon, CF[n] += principal (repayment).
// couponRate in decimal (e.g. 0.065 for 6.5%).
// termYears: number of annual coupon periods.
func buildObligasiCashflows(principal decimal.Decimal, couponRate decimal.Decimal, termYears int) []eir.CashflowItem {
	cf := make([]eir.CashflowItem, termYears+1)
	cf[0] = eir.CashflowItem{
		Date:      baseDate,
		AmountIDR: principal.Neg(), // negative = outflow
	}
	couponAmt := principal.Mul(couponRate).RoundBank(4)
	for i := 1; i <= termYears; i++ {
		amt := couponAmt
		if i == termYears {
			amt = amt.Add(principal) // final payment: coupon + principal
		}
		cf[i] = eir.CashflowItem{
			Date:      baseDate.AddDate(i, 0, 0),
			AmountIDR: amt,
		}
	}
	return cf
}

// buildEIRInputs generates n cashflow slices with realistic distribution.
// Mix:
//   50% short-term (3yr, 6.5% coupon, IDR 5B)
//   30% medium-term (7yr, 7.2% coupon, IDR 10B)
//   20% long-term (15yr, 8.0% coupon, IDR 20B)
func buildEIRInputs(n int) [][]eir.CashflowItem {
	inputs := make([][]eir.CashflowItem, n)
	p5B := decimal.NewFromFloat(5_000_000_000)
	p10B := decimal.NewFromFloat(10_000_000_000)
	p20B := decimal.NewFromFloat(20_000_000_000)
	r065 := decimal.NewFromFloat(0.065)
	r072 := decimal.NewFromFloat(0.072)
	r080 := decimal.NewFromFloat(0.080)

	for i := range inputs {
		switch {
		case i%10 < 5: // 50% short-term 3yr
			inputs[i] = buildObligasiCashflows(p5B, r065, 3)
		case i%10 < 8: // 30% medium-term 7yr
			inputs[i] = buildObligasiCashflows(p10B, r072, 7)
		default: // 20% long-term 15yr
			inputs[i] = buildObligasiCashflows(p20B, r080, 15)
		}
	}
	return inputs
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkEIRBulkRecompute1000 measures Newton-Raphson EIR computation for 1000 instruments.
// Each instrument runs its own solver with tolerance 1e-10 (DEC-013).
func BenchmarkEIRBulkRecompute1000(b *testing.B) {
	const n = 1000
	inputs := buildEIRInputs(n)
	solver := eir.NewSolver()

	b.ResetTimer()
	b.ReportAllocs()

	for iter := 0; iter < b.N; iter++ {
		for _, cf := range inputs {
			_, _, err := solver.Solve(cf, nil)
			if err != nil {
				b.Errorf("unexpected solver error: %v", err)
			}
		}
	}
}

// BenchmarkEIRBulkRecompute1000_SLACheck verifies ≤ 5s wall-clock per benchmark iteration.
func BenchmarkEIRBulkRecompute1000_SLACheck(b *testing.B) {
	const n = 1000
	inputs := buildEIRInputs(n)
	solver := eir.NewSolver()

	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		start := time.Now()
		for _, cf := range inputs {
			_, _, err := solver.Solve(cf, nil)
			if err != nil {
				b.Errorf("unexpected solver error: %v", err)
			}
		}
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			b.Errorf("SLA violation: EIR bulk recompute 1000 instruments took %v (> 5s)", elapsed)
		}
	}
}

// TestEIRBulkRecompute1000_SLACheck is a non-benchmark SLA test (runnable with go test -run).
// Confirms that 1000 Newton-Raphson EIR solves complete within 5 seconds.
func TestEIRBulkRecompute1000_SLACheck(t *testing.T) {
	const n = 1000
	inputs := buildEIRInputs(n)
	solver := eir.NewSolver()

	start := time.Now()
	var convergenceFailures int
	for i, cf := range inputs {
		_, detail, err := solver.Solve(cf, nil)
		if err != nil {
			t.Logf("instrument %d: solver error: %v", i, err)
			convergenceFailures++
			continue
		}
		if !detail.Converged {
			t.Logf("instrument %d: non-convergent after %d iters, residual=%s",
				i, detail.IterationsUsed, detail.ConvergenceResidual.String())
			convergenceFailures++
		}
	}
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("SLA violation: EIR BulkRecompute 1000 instruments took %v (> 5s SLA)", elapsed)
	}
	if convergenceFailures > 0 {
		t.Errorf("convergence failures: %d / %d instruments failed", convergenceFailures, n)
	}
	t.Logf("EIR BulkRecompute 1000 instruments: %v (SLA ≤ 5s), convergence failures: %d",
		elapsed, convergenceFailures)
}

// TestEIRSolverReproducibility verifies that identical cashflows produce identical EIR
// across multiple calls (DEC-013 precision guarantee).
// This is the property-based reproducibility test.
func TestEIRSolverReproducibility(t *testing.T) {
	solver := eir.NewSolver()

	// Standard 10B 7yr 7.2% instrument (mirrors OBL-EIR-UAT-001 in UAT-APP-C-003)
	cf := buildObligasiCashflows(
		decimal.NewFromFloat(10_000_000_000),
		decimal.NewFromFloat(0.072),
		7,
	)

	// Solve 5 times — all results must be identical to last decimal
	var results [5]decimal.Decimal
	for i := range results {
		r, _, err := solver.Solve(cf, nil)
		if err != nil {
			t.Fatalf("run %d: solver error: %v", i, err)
		}
		results[i] = r
	}

	for i := 1; i < len(results); i++ {
		if !results[0].Equal(results[i]) {
			t.Errorf("reproducibility violation: run 0 = %s, run %d = %s",
				results[0].String(), i, results[i].String())
		}
	}
	t.Logf("EIR reproducibility: 5 identical runs → %s (all equal)", results[0].String())
}
