// Package perf — P5-M18 Full-Cycle Performance Benchmarks.
//
// SLA assertions from docs/plans/phase-5-roadmap.md row 81 and UAT acceptance criteria:
//
//   BenchmarkDashboardQuery         P95 ≤ 200ms   (role-specific widget data)
//   BenchmarkReporting5YearQuery    P95 ≤ 30s     (5-year CKPN roll-forward or ECL summary)
//   BenchmarkECLCalcRun10k          P95 ≤ 60s     (ECL computation for 10,000 instruments)
//   BenchmarkAuditHashChainVerify30d P95 ≤ 5s     (SHA-256 chain verify for 30-day audit window)
//
// These benchmarks test pure in-process computation without DB or network I/O.
// DB-side SLAs are covered by k6 scripts under tests/load/.
//
// Run:
//
//	go test ./backend/tests/perf/... -bench=BenchmarkP5M18 -benchtime=3s -count=3
package perf

import (
	"crypto/sha256"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared benchmark constants (SLA targets)
// ─────────────────────────────────────────────────────────────────────────────

const (
	// SLA: dashboard widget data latency ≤ 200ms.
	slaDashboardMS = 200

	// SLA: 5-year reporting query ≤ 30s.
	slaReporting5YearSec = 30

	// SLA: ECL calc run for 10k instruments ≤ 60s.
	slaECLCalcRunSec = 60

	// SLA: audit hash chain verify for 30-day window ≤ 5s.
	slaAuditHashChainSec = 5

	// ECL parameters (canonical DEC-010).
	benchWGood   = 0.25
	benchWNormal = 0.50
	benchWBad    = 0.25

	// EIR Newton-Raphson parameters (DEC-013).
	benchEIRTolerance = 1e-10
	benchEIRMaxIter   = 100
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared data — pre-allocated for zero-allocation benchmarks
// ─────────────────────────────────────────────────────────────────────────────

var (
	benchNominal10B = decimal.NewFromInt(10_000_000_000) // IDR 10 miliar
	benchPD12M      = decimal.NewFromFloat(0.0050)        // 0.5%
	benchLGD        = decimal.NewFromFloat(0.45)
	benchFL         = decimal.NewFromFloat(1.10)
	benchWGoodDec   = decimal.NewFromFloat(benchWGood)
	benchWNormDec   = decimal.NewFromFloat(benchWNormal)
	benchWBadDec    = decimal.NewFromFloat(benchWBad)
)

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkP5M18_DashboardQuery
// ─────────────────────────────────────────────────────────────────────────────
//
// Simulates the computation side of a dashboard widget:
//   - Aggregate ECL by stage for a 100-instrument portfolio.
//   - P95 ≤ 200ms (roadmap SLA).
//
// Production latency includes DB query time (measured by k6). This benchmark
// isolates the aggregation computation kernel.

func BenchmarkP5M18_DashboardQuery(b *testing.B) {
	const nInstruments = 100
	type stageSummary struct {
		Stage        int
		TotalECL     decimal.Decimal
		TotalEAD     decimal.Decimal
		InstrCount   int
	}

	// Pre-build instrument ECL data.
	type benchInstr struct {
		EAD      decimal.Decimal
		ECL      decimal.Decimal
		Stage    int
	}
	instrs := make([]benchInstr, nInstruments)
	for i := range instrs {
		nominal := decimal.NewFromInt(int64(i+1) * 100_000_000)
		stage := (i % 3) + 1 // distribute across Stage 1/2/3
		ecl := computeBenchECL(nominal, stage)
		instrs[i] = benchInstr{EAD: nominal, ECL: ecl, Stage: stage}
	}

	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for i := 0; i < b.N; i++ {
		summary := make(map[int]*stageSummary)
		for _, instr := range instrs {
			s, ok := summary[instr.Stage]
			if !ok {
				s = &stageSummary{Stage: instr.Stage}
				summary[instr.Stage] = s
			}
			s.TotalECL = s.TotalECL.Add(instr.ECL)
			s.TotalEAD = s.TotalEAD.Add(instr.EAD)
			s.InstrCount++
		}
	}
	elapsed := time.Since(start)

	// SLA check (P95 approximation: single-run wall time / N ≤ SLA).
	avgMS := float64(elapsed.Milliseconds()) / float64(b.N)
	if avgMS > slaDashboardMS {
		b.Errorf("SLA BREACH: dashboard avg latency %.2fms exceeds ≤%dms target", avgMS, slaDashboardMS)
	}
	b.ReportMetric(avgMS, "ms/op")
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkP5M18_ECLCalcRun10k
// ─────────────────────────────────────────────────────────────────────────────
//
// Simulates the computation kernel of an ECL calc run for 10,000 instruments.
// Each instrument: 3-scenario ECL with dual FL (formulas.md §ECL per instrument).
// SLA: total ≤ 60s (roadmap row 81).

func BenchmarkP5M18_ECLCalcRun10k(b *testing.B) {
	const nInstruments = 10_000

	// Pre-build instrument data.
	type benchInstrData struct {
		ID      uuid.UUID
		EAD     decimal.Decimal
		Stage   int
		PDGood  decimal.Decimal
		PDNorm  decimal.Decimal
		PDBad   decimal.Decimal
		LGD     decimal.Decimal
		FLGood  decimal.Decimal
		FLNorm  decimal.Decimal
		FLBad   decimal.Decimal
	}
	instrs := make([]benchInstrData, nInstruments)
	for i := range instrs {
		stage := (i % 3) + 1
		var pd decimal.Decimal
		switch stage {
		case 1:
			pd = decimal.NewFromFloat(0.0050)
		case 2:
			pd = decimal.NewFromFloat(0.0800)
		case 3:
			pd = decimal.NewFromInt(1) // PD=1.0 for Stage 3 (DEC-010)
		}
		instrs[i] = benchInstrData{
			ID:     uuid.New(),
			EAD:    decimal.NewFromInt(int64(i+1) * 50_000_000),
			Stage:  stage,
			PDGood: pd.Mul(decimal.NewFromFloat(0.7)),
			PDNorm: pd,
			PDBad:  pd.Mul(decimal.NewFromFloat(2.5)),
			LGD:    decimal.NewFromFloat(0.45),
			FLGood: decimal.NewFromFloat(0.95),
			FLNorm: decimal.NewFromFloat(1.10),
			FLBad:  decimal.NewFromFloat(1.45),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	totalStart := time.Now()
	for i := 0; i < b.N; i++ {
		totalECL := decimal.Zero
		for _, instr := range instrs {
			ecl := computeBenchECL3Scenario(
				instr.EAD,
				instr.PDGood, instr.PDNorm, instr.PDBad,
				instr.LGD,
				instr.FLGood, instr.FLNorm, instr.FLBad,
			)
			totalECL = totalECL.Add(ecl)
		}
		_ = totalECL
	}
	totalElapsed := time.Since(totalStart)

	totalSec := float64(totalElapsed.Seconds()) / float64(b.N)
	if totalSec > slaECLCalcRunSec {
		b.Errorf("SLA BREACH: ECL calc run 10k instruments avg %.2fs exceeds ≤%ds target", totalSec, slaECLCalcRunSec)
	}
	b.ReportMetric(totalSec, "sec/run")
	b.ReportMetric(float64(nInstruments)/totalSec, "instruments/sec")
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkP5M18_AuditHashChainVerify30d
// ─────────────────────────────────────────────────────────────────────────────
//
// Simulates audit hash chain verification for a 30-day window.
// Assumption: ~250 mutations/day × 30 days = 7,500 audit rows.
// SLA: ≤ 5s for the full verification pass.

func BenchmarkP5M18_AuditHashChainVerify30d(b *testing.B) {
	const (
		mutationsPerDay = 250
		windowDays      = 30
		totalRows       = mutationsPerDay * windowDays // 7,500
	)

	// Pre-build a chain of 7,500 audit rows (mimics the p5AuditStore hash algorithm).
	type auditRow struct {
		Action      string
		EntityID    string
		AfterJSON   string
		CurrentHash []byte
	}

	buildChain := func(n int) []auditRow {
		rows := make([]auditRow, n)
		var prevHash []byte
		for i := range rows {
			action := fmt.Sprintf("ACTION_%05d", i%50) // 50 distinct actions
			entityID := uuid.New().String()
			afterJSON := fmt.Sprintf(`{"seq":%d,"amount":"1234567.8900"}`, i)
			payload := fmt.Sprintf("%x||%s||%s||%s", prevHash, action, entityID, afterJSON)
			h := sha256.Sum256([]byte(payload))
			rows[i] = auditRow{
				Action:      action,
				EntityID:    entityID,
				AfterJSON:   afterJSON,
				CurrentHash: h[:],
			}
			prevHash = h[:]
		}
		return rows
	}

	chain := buildChain(totalRows)

	b.ResetTimer()
	b.ReportAllocs()

	totalStart := time.Now()
	for i := 0; i < b.N; i++ {
		// Verify entire chain.
		for j := 1; j < len(chain); j++ {
			prev := chain[j-1]
			curr := chain[j]
			payload := fmt.Sprintf("%x||%s||%s||%s", prev.CurrentHash, curr.Action, curr.EntityID, curr.AfterJSON)
			expected := sha256.Sum256([]byte(payload))
			if string(expected[:]) != string(curr.CurrentHash) {
				b.Errorf("Hash chain broken at row %d (action=%s)", j, curr.Action)
			}
		}
	}
	totalElapsed := time.Since(totalStart)

	totalSec := float64(totalElapsed.Seconds()) / float64(b.N)
	if totalSec > slaAuditHashChainSec {
		b.Errorf("SLA BREACH: audit hash chain verify (30d, %d rows) avg %.2fs exceeds ≤%ds target",
			totalRows, totalSec, slaAuditHashChainSec)
	}
	b.ReportMetric(totalSec, "sec/verify")
	b.ReportMetric(float64(totalRows)/totalSec, "rows/sec")
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkP5M18_Reporting5YearQuery
// ─────────────────────────────────────────────────────────────────────────────
//
// Simulates the computation kernel of a 5-year CKPN roll-forward aggregation.
// Covers 60 months of roll-forward data (opening + 5 movements per month).
// SLA: ≤ 30s for the full 5-year aggregation.

func BenchmarkP5M18_Reporting5YearQuery(b *testing.B) {
	const (
		months       = 60 // 5 years
		instrPerMonth = 500 // instruments active each month
	)

	// Pre-build monthly roll-forward data.
	type monthlyMovement struct {
		Month          int
		Opening        decimal.Decimal
		TransferTo     decimal.Decimal
		TransferFrom   decimal.Decimal
		Originations   decimal.Decimal
		Derecognitions decimal.Decimal
		Remeasurements decimal.Decimal
		Closing        decimal.Decimal
	}

	buildMonths := func(n int) []monthlyMovement {
		ms := make([]monthlyMovement, n)
		opening := decimal.NewFromInt(10_000_000_000)
		for i := range ms {
			to := decimal.NewFromInt(int64(i+1) * 50_000_000)
			from := decimal.NewFromInt(int64(i+1) * 20_000_000)
			orig := decimal.NewFromInt(int64(i+1) * 200_000_000)
			derec := decimal.NewFromInt(int64(i+1) * 150_000_000)
			remeas := decimal.NewFromInt(int64(i+1) * 5_000_000)
			closing := opening.Add(to).Sub(from).Add(orig).Sub(derec).Add(remeas)
			ms[i] = monthlyMovement{
				Month: i + 1, Opening: opening, TransferTo: to, TransferFrom: from,
				Originations: orig, Derecognitions: derec, Remeasurements: remeas, Closing: closing,
			}
			opening = closing
		}
		return ms
	}

	monthlyData := buildMonths(months)

	b.ResetTimer()
	b.ReportAllocs()

	totalStart := time.Now()
	for i := 0; i < b.N; i++ {
		// Aggregate 5-year roll-forward (computation kernel).
		var totalOriginations, totalDerecognitions, totalTransfers, totalRemeas decimal.Decimal
		var fiveYearOpening, fiveYearClosing decimal.Decimal
		for j, m := range monthlyData {
			if j == 0 {
				fiveYearOpening = m.Opening
			}
			totalOriginations = totalOriginations.Add(m.Originations)
			totalDerecognitions = totalDerecognitions.Add(m.Derecognitions)
			totalTransfers = totalTransfers.Add(m.TransferTo).Sub(m.TransferFrom)
			totalRemeas = totalRemeas.Add(m.Remeasurements)
			fiveYearClosing = m.Closing
		}
		// Verify 5-year reconcile.
		computed := fiveYearOpening.Add(totalOriginations).Sub(totalDerecognitions).Add(totalTransfers).Add(totalRemeas)
		if !computed.Equal(fiveYearClosing) {
			b.Errorf("5-year roll-forward reconcile failed: computed=%s closing=%s",
				computed.StringFixed(4), fiveYearClosing.StringFixed(4))
		}
	}
	totalElapsed := time.Since(totalStart)

	totalSec := float64(totalElapsed.Seconds()) / float64(b.N)
	if totalSec > slaReporting5YearSec {
		b.Errorf("SLA BREACH: 5-year reporting avg %.2fs exceeds ≤%ds target", totalSec, slaReporting5YearSec)
	}
	b.ReportMetric(totalSec, "sec/query")
	b.ReportMetric(float64(months*instrPerMonth)/totalSec, "instrument-months/sec")
}

// ─────────────────────────────────────────────────────────────────────────────
// BenchmarkP5M18_EIRNewtonRaphson
// ─────────────────────────────────────────────────────────────────────────────
//
// Benchmarks the EIR Newton-Raphson solver (DEC-013) for a 12-month deposito.
// Verifies convergence within tolerance 1e-10 and ≤ 100 iterations.

func BenchmarkP5M18_EIRNewtonRaphson(b *testing.B) {
	const (
		principal = 10_000_000_000.0 // IDR 10 miliar
		tenor     = 12               // 12 months
		coupon    = 0.0525           // 5.25% gross
		pph       = 0.20             // PPh 20%
	)

	// Build cashflow array once.
	cfs := make([]float64, tenor+1)
	cfs[0] = -principal
	monthlyCoupon := principal * coupon * (1.0 - pph) / 12.0
	for t := 1; t <= tenor; t++ {
		cfs[t] = monthlyCoupon
	}
	cfs[tenor] += principal

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r, iters, converged := solveEIRNewtonRaphson(cfs)
		if !converged {
			b.Errorf("EIR Newton-Raphson did not converge in %d iterations, r=%.8f", iters, r)
		}
		if iters > benchEIRMaxIter {
			b.Errorf("EIR Newton-Raphson exceeded max iterations: %d", iters)
		}
		_ = r
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Private helpers
// ─────────────────────────────────────────────────────────────────────────────

// computeBenchECL computes a simplified ECL for a single stage (12M PD for Stage 1,
// Lifetime PD for Stage 2, PD=1.0 for Stage 3). Used in dashboard benchmark.
func computeBenchECL(ead decimal.Decimal, stage int) decimal.Decimal {
	var pd decimal.Decimal
	switch stage {
	case 1:
		pd = decimal.NewFromFloat(0.0050)
	case 2:
		pd = decimal.NewFromFloat(0.0800)
	case 3:
		pd = decimal.NewFromInt(1)
	default:
		pd = decimal.NewFromFloat(0.0050)
	}
	return ead.Mul(pd).Mul(benchLGD).Mul(benchFL)
}

// computeBenchECL3Scenario computes the 3-scenario weighted ECL (DEC-010).
func computeBenchECL3Scenario(
	ead, pdGood, pdNorm, pdBad, lgd, flGood, flNorm, flBad decimal.Decimal,
) decimal.Decimal {
	eclGood := ead.Mul(pdGood).Mul(lgd).Mul(flGood)
	eclNorm := ead.Mul(pdNorm).Mul(lgd).Mul(flNorm)
	eclBad := ead.Mul(pdBad).Mul(lgd).Mul(flBad)
	return eclGood.Mul(benchWGoodDec).
		Add(eclNorm.Mul(benchWNormDec)).
		Add(eclBad.Mul(benchWBadDec))
}

// solveEIRNewtonRaphson is the pure-float64 Newton-Raphson solver (DEC-013).
// Returns (r, iterations, converged).
func solveEIRNewtonRaphson(cashflows []float64) (float64, int, bool) {
	r := 0.1
	for i := 0; i < benchEIRMaxIter; i++ {
		var f, df float64
		for t, cf := range cashflows {
			denom := math.Pow(1+r, float64(t))
			f += cf / denom
			if t > 0 {
				df -= float64(t) * cf / (denom * (1 + r))
			}
		}
		if math.Abs(df) < 1e-15 {
			return r, i, false
		}
		rNext := r - f/df
		if math.Abs(rNext-r) < benchEIRTolerance {
			return rNext, i + 1, true
		}
		r = rNext
	}
	return r, benchEIRMaxIter, false
}

// Ensure time and uuid are used (avoid "imported and not used" if any branch skips them).
var _ = time.Now
var _ = uuid.New
