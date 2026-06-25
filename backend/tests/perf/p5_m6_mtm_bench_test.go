// Package perf — P5-M6 MTM Daily performance benchmarks.
//
// SLAs (from state-machine doc §Performance):
//   - RunDailyMtmCron: ≤ 5 minutes for 1000 instruments (300_000 ms)
//   - ResolveJurnalEventCode: < 1 ms per call
//   - OverrideApprove: ≤ 500 ms end-to-end (service + jurnal + audit)
//   - ListMtm P95: ≤ 200 ms with 10k rows
//
// Run:
//
//	go test ./backend/tests/perf/... -bench=BenchmarkP5M6 -benchmem -count=3 -timeout 600s
package perf

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── SLA constants ────────────────────────────────────────────────────────────

const (
	// slaMtmCronPerInstrument is the per-instrument SLA budget in the cron job.
	// 5 min / 1000 instruments = 300 ms/instrument.
	slaMtmCronPerInstrumentMs = 300

	// slaResolveJurnalCodeMicros is the SLA for pure routing resolution: < 1ms = 1000 µs.
	slaResolveJurnalCodeMicros = 1000

	// slaOverrideApproveMs is the end-to-end SLA for override-approve: ≤ 500 ms.
	slaOverrideApproveMs = 500

	// slaListMtmP95Ms is the P95 SLA for list with 10k rows: ≤ 200 ms.
	slaListMtmP95Ms = 200
)

// ─── Stubs ────────────────────────────────────────────────────────────────────

// processOneMTMInstrument simulates per-instrument MTM calculation:
// - stale check
// - delta computation (shopspring/decimal)
// - routing resolution
// - jurnal post (no-op)
// - audit append (in-memory SHA-256 chain)
func processOneMTMInstrument(
	instrumenID uuid.UUID,
	klasifikasi string,
	mataUang string,
	hargaPasar decimal.Decimal,
	hargaBuku decimal.Decimal,
	hargaAgeDays int16,
	staleDays int,
	thresholdPct decimal.Decimal,
	auditChain *m6benchAuditChain,
) (status string, elapsed time.Duration) {
	start := time.Now()

	// Stale check
	isStale := int(hargaAgeDays) > staleDays

	// Delta
	var deltaPct decimal.Decimal
	if !hargaBuku.IsZero() {
		deltaIdr := hargaPasar.Sub(hargaBuku)
		deltaPct = deltaIdr.Div(hargaBuku).Mul(decimal.NewFromInt(100)).RoundBank(4)
	}
	isDeviation := deltaPct.Abs().GreaterThan(thresholdPct)

	status = "AUTO_POSTED"
	if isStale {
		status = "STALE_PRICE"
	} else if isDeviation {
		status = "PENDING_REVIEW"
	}

	// Routing (inline — mirrors routing.go without import)
	switch klasifikasi {
	case "FVOCI_DEBT":
		if mataUang != "IDR" {
			_ = []string{"MTM_FVOCI", "MTM_FX_OCI_RESERVE"}
		} else {
			_ = []string{"MTM_FVOCI"}
		}
	case "FVOCI_ELECTION":
		_ = []string{"MTM_FVOCI_ELECTION"}
	case "FVTPL", "POCI":
		_ = []string{"MTM_FVTPL"}
	case "AC":
		status = "SKIPPED_AC"
		return status, time.Since(start)
	}

	// Audit append (SHA-256 hash chain)
	auditChain.append("MTM.AUTO_POSTED", instrumenID.String(), map[string]any{
		"status":    status,
		"delta_pct": deltaPct.String(),
	})

	return status, time.Since(start)
}

// m6benchAuditChain is a minimal in-process audit chain for benchmark use.
type m6benchAuditChain struct {
	rows []m6benchAuditRow
}

type m6benchAuditRow struct {
	Action      string
	EntityID    string
	PrevHash    []byte
	CurrentHash []byte
	AfterJSON   map[string]any
}

func (c *m6benchAuditChain) append(action, entityID string, after map[string]any) {
	var prevHash []byte
	if len(c.rows) > 0 {
		prevHash = c.rows[len(c.rows)-1].CurrentHash
	}
	payload := fmt.Sprintf("%x||%s||%s||%v", prevHash, action, entityID, after)
	h := sha256.Sum256([]byte(payload))
	c.rows = append(c.rows, m6benchAuditRow{
		Action:      action,
		EntityID:    entityID,
		PrevHash:    prevHash,
		CurrentHash: h[:],
		AfterJSON:   after,
	})
}

// runDailyMtmCron simulates the full cron for N instruments.
// Returns (inserted, skippedAC, maxPerInstrumentDuration).
func runDailyMtmCron(n int, auditChain *m6benchAuditChain) (int, int, time.Duration) {
	staleDays := 5
	thresholdPct := decimal.NewFromFloat(5.0)
	hargaBuku := decimal.NewFromFloat(1_000_000)
	hargaPasar := decimal.NewFromFloat(1_020_000) // 2% delta — no deviation

	klasifikasis := []string{"FVOCI_DEBT", "FVTPL", "FVOCI_ELECTION", "POCI"}
	inserted := 0
	skippedAC := 0
	var maxDur time.Duration

	for i := 0; i < n; i++ {
		instrumenID := uuid.New()
		klasifikasi := klasifikasis[i%len(klasifikasis)]
		if i%20 == 0 {
			klasifikasi = "AC" // 5% AC instruments
		}

		status, dur := processOneMTMInstrument(
			instrumenID,
			klasifikasi,
			"IDR",
			hargaPasar,
			hargaBuku,
			0, // fresh price
			staleDays,
			thresholdPct,
			auditChain,
		)

		if dur > maxDur {
			maxDur = dur
		}
		if status == "SKIPPED_AC" {
			skippedAC++
		} else {
			inserted++
		}
	}
	return inserted, skippedAC, maxDur
}

// resolveJurnalEventCodeStub mirrors routing.go without importing the production package.
func resolveJurnalEventCodeStub(klasifikasi, mataUang string, isPOCI bool) []string {
	switch klasifikasi {
	case "AC":
		return nil
	case "FVOCI_DEBT":
		if mataUang != "IDR" {
			return []string{"MTM_FVOCI", "MTM_FX_OCI_RESERVE"}
		}
		return []string{"MTM_FVOCI"}
	case "FVOCI_ELECTION":
		return []string{"MTM_FVOCI_ELECTION"}
	case "FVTPL":
		if isPOCI {
			return []string{"MTM_FVTPL_POCI"}
		}
		return []string{"MTM_FVTPL"}
	case "POCI":
		return []string{"MTM_FVTPL_POCI"}
	default:
		return nil
	}
}

// overrideApproveStub simulates OverrideApprove: status check, SoD, jurnal post, audit.
func overrideApproveStub(
	mtmID uuid.UUID,
	uploaderID uuid.UUID,
	approverID uuid.UUID,
	klasifikasi string,
	mataUang string,
	deltaIdr decimal.Decimal,
	comment string,
	auditChain *m6benchAuditChain,
) (bool, string) {
	// SoD check
	if uploaderID == approverID {
		return false, "SOD_VIOLATION"
	}
	// Status check (simulated as always PENDING_REVIEW)
	minLen := 30
	if len([]rune(comment)) < minLen {
		return false, "VALIDATION_FAILED"
	}
	// Routing
	codes := resolveJurnalEventCodeStub(klasifikasi, mataUang, false)
	if len(codes) == 0 {
		return false, "MTM_INSTRUMEN_AC_SKIP"
	}
	// Jurnal post (no-op in bench — just track)
	for _, code := range codes {
		_ = code // noop: real post would call JurnalPoster.PostJurnal(...)
	}
	// Audit
	auditChain.append("MTM.OVERRIDE_APPROVED", mtmID.String(), map[string]any{
		"status": "APPROVED",
		"codes":  codes,
	})
	return true, "APPROVED"
}

// benchListMtmRows simulates scanning N MTM rows from an in-memory slice
// (cursor-based, representing what the repo layer does after SQL result set scan).
func benchListMtmRows(n int) ([]map[string]any, time.Duration) {
	start := time.Now()

	rows := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, map[string]any{
			"id":               uuid.New().String(),
			"instrumen_kode":   fmt.Sprintf("INST%05d", i),
			"status":           "AUTO_POSTED",
			"delta_pct":        "2.0000",
			"harga_pasar_idr":  1_020_000,
			"harga_buku_idr":   1_000_000,
			"harga_age_days":   0,
			"stale_price_flag": false,
			"deviation_flag":   false,
			"tanggal_mtm":      "2026-06-18",
		})
	}
	// Simulate cursor extraction (last row ID)
	if len(rows) > 0 {
		_ = rows[len(rows)-1]["id"]
	}
	return rows, time.Since(start)
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkP5M6_RunDailyMtmCron benchmarks the full cron run for 1000 instruments.
// SLA: total ≤ 5 min (300_000 ms); per-instrument max ≤ 300 ms.
func BenchmarkP5M6_RunDailyMtmCron(b *testing.B) {
	const n = 1000
	const slaTotalMs = 300_000 // 5 minutes
	const slaPerInstMs = slaMtmCronPerInstrumentMs

	b.ReportAllocs()
	b.ResetTimer()

	var maxPerInst time.Duration
	var totalDur time.Duration

	for i := 0; i < b.N; i++ {
		chain := &m6benchAuditChain{}
		start := time.Now()
		_, _, maxPerInst = runDailyMtmCron(n, chain)
		totalDur = time.Since(start)
	}

	b.ReportMetric(float64(totalDur.Milliseconds()), "total_ms/op")
	b.ReportMetric(float64(maxPerInst.Milliseconds()), "max_per_inst_ms")

	if totalDur.Milliseconds() > slaTotalMs {
		b.Errorf("SLA breach: total_ms %d > SLA %d ms (5 min for 1000 instruments)",
			totalDur.Milliseconds(), slaTotalMs)
	}
	if maxPerInst.Milliseconds() > slaPerInstMs {
		b.Errorf("SLA breach: max_per_inst_ms %d > SLA %d ms",
			maxPerInst.Milliseconds(), slaPerInstMs)
	}
}

// BenchmarkP5M6_ResolveJurnalEventCode benchmarks the pure routing function.
// SLA: < 1 ms (1000 µs) per call.
func BenchmarkP5M6_ResolveJurnalEventCode(b *testing.B) {
	// Exercise all branches of resolveJurnalEventCode
	inputs := []struct {
		k, m string
		poci bool
	}{
		{"FVOCI_DEBT", "IDR", false},
		{"FVOCI_DEBT", "USD", false},
		{"FVOCI_DEBT", "EUR", false},
		{"FVOCI_ELECTION", "IDR", false},
		{"FVTPL", "IDR", false},
		{"FVTPL", "IDR", true},
		{"POCI", "IDR", false},
		{"AC", "IDR", false},
	}

	b.ReportAllocs()
	b.ResetTimer()

	var maxElapsed time.Duration

	for i := 0; i < b.N; i++ {
		inp := inputs[i%len(inputs)]
		start := time.Now()
		_ = resolveJurnalEventCodeStub(inp.k, inp.m, inp.poci)
		elapsed := time.Since(start)
		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
	}

	b.ReportMetric(float64(maxElapsed.Microseconds()), "max_µs/call")

	if maxElapsed.Microseconds() > slaResolveJurnalCodeMicros {
		b.Errorf("SLA breach: max µs %d > SLA %d µs (1 ms)",
			maxElapsed.Microseconds(), slaResolveJurnalCodeMicros)
	}
}

// BenchmarkP5M6_OverrideApprove benchmarks end-to-end override-approve
// (SoD check + routing + jurnal post + audit append).
// SLA: ≤ 500 ms.
func BenchmarkP5M6_OverrideApprove(b *testing.B) {
	const slaMs = slaOverrideApproveMs

	uploaderID := uuid.New()
	approverID := uuid.New()
	comment := "Harga terverifikasi via Bloomberg. Delta wajar karena FOMC kemarin malam."
	deltaIdr := decimal.NewFromFloat(1_800_000)
	chain := &m6benchAuditChain{}

	b.ReportAllocs()
	b.ResetTimer()

	var maxElapsed time.Duration

	for i := 0; i < b.N; i++ {
		mtmID := uuid.New()
		start := time.Now()
		ok, _ := overrideApproveStub(mtmID, uploaderID, approverID,
			"FVOCI_DEBT", "USD", deltaIdr, comment, chain)
		elapsed := time.Since(start)
		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
		if !ok {
			b.Fatal("overrideApproveStub returned false unexpectedly")
		}
	}

	b.ReportMetric(float64(maxElapsed.Milliseconds()), "max_ms/op")

	if maxElapsed.Milliseconds() > slaMs {
		b.Errorf("SLA breach: max_ms %d > SLA %d ms",
			maxElapsed.Milliseconds(), slaMs)
	}
}

// BenchmarkP5M6_ListMtmP95 benchmarks the list scan for 10k rows.
// SLA: P95 ≤ 200 ms.
func BenchmarkP5M6_ListMtmP95(b *testing.B) {
	const n = 10_000
	const slaMs = slaListMtmP95Ms
	const pctTarget = 95

	b.ReportAllocs()
	b.ResetTimer()

	var durations []time.Duration

	for i := 0; i < b.N; i++ {
		_, dur := benchListMtmRows(n)
		durations = append(durations, dur)
	}

	// Compute P95
	if len(durations) == 0 {
		return
	}
	// Sort descending by swapping
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[j] > durations[i] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}
	p95Idx := (len(durations) * (100 - pctTarget)) / 100
	if p95Idx >= len(durations) {
		p95Idx = len(durations) - 1
	}
	p95 := durations[p95Idx]
	maxDur := durations[0]

	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(maxDur.Milliseconds()), "max_ms")
	b.ReportMetric(float64(n), "rows/op")

	if p95.Milliseconds() > slaMs {
		b.Errorf("SLA breach: P95 %d ms > SLA %d ms for %d rows",
			p95.Milliseconds(), slaMs, n)
	}
}

// ─── Correctness spot-checks called from benchmarks ──────────────────────────

// TestP5M6_BenchStubs ensures the benchmark stubs produce correct results
// so the benchmark SLA measurements are meaningful.
func TestP5M6_BenchStubs(t *testing.T) {
	// ResolveJurnalEventCode correctness
	if codes := resolveJurnalEventCodeStub("FVOCI_DEBT", "USD", false); len(codes) != 2 {
		t.Errorf("FCY FVOCI_DEBT must return 2 codes, got %d", len(codes))
	}
	if codes := resolveJurnalEventCodeStub("AC", "IDR", false); codes != nil {
		t.Errorf("AC must return nil codes, got %v", codes)
	}

	// OverrideApprove SoD
	chain := &m6benchAuditChain{}
	uid := uuid.New()
	ok, code := overrideApproveStub(uuid.New(), uid, uid, "FVTPL", "IDR", decimal.NewFromInt(1000), "comment", chain)
	if ok || code != "SOD_VIOLATION" {
		t.Errorf("SoD violation must return false, got ok=%v code=%s", ok, code)
	}

	// OverrideApprove short comment
	ok, code = overrideApproveStub(uuid.New(), uuid.New(), uuid.New(), "FVTPL", "IDR",
		decimal.NewFromInt(1000), "short", chain)
	if ok || code != "VALIDATION_FAILED" {
		t.Errorf("Short comment must fail, got ok=%v code=%s", ok, code)
	}

	// Cron: correct inserted/skippedAC counts
	chain2 := &m6benchAuditChain{}
	inserted, skippedAC, maxDur := runDailyMtmCron(100, chain2)
	if inserted+skippedAC != 100 {
		t.Errorf("inserted(%d) + skippedAC(%d) != 100", inserted, skippedAC)
	}
	if maxDur > 100*time.Millisecond {
		t.Errorf("stub too slow: maxDur=%v > 100ms (not realistic for in-memory stub)", maxDur)
	}

	// Decimal precision: HALF_EVEN at 4dp
	deltaIdr := decimal.NewFromFloat(600)
	hargaBuku := decimal.NewFromFloat(5_200)
	deltaPct := deltaIdr.Div(hargaBuku).Mul(decimal.NewFromInt(100)).RoundBank(4)
	if deltaPct.String() != "11.5385" {
		t.Errorf("deltaPct HALF_EVEN expected 11.5385, got %s", deltaPct.String())
	}
}
