// Package perf — P5-M10 POCI Delta ECL performance benchmarks.
//
// SLA targets:
//
//	COMPUTE_DELTA_SINGLE_MAX       = 100 µs  — pure decimal delta+direction for one POCI instrument
//	COMPUTE_DELTA_1000_POCI_MAX    = 5 min   — batch calc run across 1000 POCI instruments
//	LIST_DELTA_LOG_P95_MAX         = 200 ms  — GET /poci/delta-log with 10k rows, direction filter
//	BASELINE_LOOKUP_MAX            = 50 µs   — single map lookup baseline by instrumen_id
//	JURNAL_DIRECTION_VALIDATE_MAX  = 10 µs   — m10ValidateJurnalDirection (zero alloc)
//	ABS_DELTA_MAX                  = 5 µs    — m10AbsDeltaForJurnal (abs + roundbank)
//	HASH_CHAIN_ENTRY_MAX           = 200 µs  — single audit entry sha256 hash-chain step
//	LARGE_DELTA_CHECK_MAX          = 5 µs    — isLargeDelta comparison
//	CUMULATIVE_10K_MAX             = 50 ms   — sum all prior delta for 1 instrument in 10k row repo
//	IDEMPOTENCY_CHECK_MAX          = 20 µs   — map lookup for duplicate (calc_run_id, instrumen_id)
//
// Compliance references:
//
//	DEC-010: POCI delta = current ECL − baseline ECL (no staging engine)
//	DEC-016: shopspring/decimal, NUMERIC(20,4) IDR, 8-dp EIR
//	DEC-018: baseline WORM — insert path only, no update alloc
//	DEC-021: idempotency key check must be O(1)
//	DEC-022: cursor-based pagination — list test excludes COUNT(*)
//	UX-§3: ComputeDeltaForCalcRun 1000 POCI ≤ 5 min (long-running job, Asynq worker)
//
// Run:
//
//	go test ./backend/tests/perf/... -bench=BenchP5M10 -benchtime=10s -benchmem -race
package perf

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// ─── SLA constants (P5-M10) ───────────────────────────────────────────────────

const (
	slaPociComputeSingleMax    = 100 * time.Microsecond
	slaPociCompute1000Max      = 5 * time.Minute
	slaPociListP95Max          = 200 * time.Millisecond
	slaPociBaselineLookupMax   = 50 * time.Microsecond
	slaPociValidateDirectMax   = 10 * time.Microsecond
	slaPociAbsDeltaMax         = 5 * time.Microsecond
	slaPociHashChainEntryMax   = 200 * time.Microsecond
	slaPociLargeDeltaCheckMax  = 5 * time.Microsecond
	slaPociCumulative10kMax    = 50 * time.Millisecond
	slaPociIdempotencyCheckMax = 20 * time.Microsecond

	benchPociInstrumens1k = 1000
	benchPociListRows10k  = 10_000
	benchLargeDeltaThresh = int64(500_000_000)
)

// ─── Bench fixtures ───────────────────────────────────────────────────────────

type benchPociInstrumen struct {
	ID          uuid.UUID
	IsPoci      bool
	BaselineECL decimal.Decimal // NUMERIC(20,4)
	CurrentECL  decimal.Decimal // NUMERIC(20,4)
	EIR         decimal.Decimal // NUMERIC(10,8)
}

type benchPociDeltaRow struct {
	ID          uuid.UUID
	CalcRunID   uuid.UUID
	InstrumenID uuid.UUID
	DeltaECL    decimal.Decimal
	Direction   string
}

// benchGenPociInstrumen generates a reproducible POCI fixture at index i.
func benchGenPociInstrumen(i int) benchPociInstrumen {
	baseline := decimal.NewFromFloat(float64(500_000_000 + i*1_000_000)).RoundBank(4)
	// Alternating INCREASE / DECREASE / ZERO pattern for realistic distribution
	var current decimal.Decimal
	switch i % 3 {
	case 0:
		current = baseline.Add(decimal.NewFromFloat(float64(i+1) * 100_000)).RoundBank(4)
	case 1:
		current = baseline.Sub(decimal.NewFromFloat(float64(i+1) * 80_000)).RoundBank(4)
	default:
		current = baseline // ZERO
	}
	return benchPociInstrumen{
		ID:          uuid.New(),
		IsPoci:      true,
		BaselineECL: baseline,
		CurrentECL:  current,
		EIR:         decimal.NewFromFloat(0.04500000).RoundBank(8),
	}
}

// benchComputeDelta mirrors calc.go's ComputeDelta (decimal arithmetic only).
func benchComputeDelta(current, baseline decimal.Decimal) (delta decimal.Decimal, dir string) {
	delta = current.Sub(baseline).RoundBank(4)
	switch {
	case delta.IsPositive():
		dir = "INCREASE"
	case delta.IsNegative():
		dir = "DECREASE"
	default:
		dir = "ZERO"
	}
	return
}

// benchValidateDirection mirrors ValidateJurnalDirection.
func benchValidateDirection(delta decimal.Decimal, dir string) error {
	switch {
	case delta.IsPositive() && dir != "INCREASE":
		return fmt.Errorf("POCI_JURNAL_DIRECTION_MISMATCH: delta %s positif tapi dir=%s", delta.StringFixed(4), dir)
	case delta.IsNegative() && dir != "DECREASE":
		return fmt.Errorf("POCI_JURNAL_DIRECTION_MISMATCH: delta %s negatif tapi dir=%s", delta.StringFixed(4), dir)
	case delta.Equal(decimal.Zero) && dir != "ZERO":
		return fmt.Errorf("POCI_JURNAL_DIRECTION_MISMATCH: delta=0 tapi dir=%s", dir)
	}
	return nil
}

// benchAbsDelta mirrors AbsDeltaForJurnal.
func benchAbsDelta(delta decimal.Decimal) decimal.Decimal {
	return delta.Abs().RoundBank(4)
}

// benchIsLargeDelta mirrors m10IsLargeDelta.
func benchIsLargeDelta(delta decimal.Decimal, threshold int64) bool {
	return delta.Abs().GreaterThan(decimal.NewFromInt(threshold))
}

// ─── Unit benchmarks ──────────────────────────────────────────────────────────

// BenchP5M10_ComputeDeltaSingle — DEC-016: single decimal delta < 100µs.
func BenchP5M10_ComputeDeltaSingle(b *testing.B) {
	inst := benchGenPociInstrumen(42)
	b.ResetTimer()
	b.ReportAllocs()

	var (
		delta decimal.Decimal
		dir   string
	)
	for i := 0; i < b.N; i++ {
		delta, dir = benchComputeDelta(inst.CurrentECL, inst.BaselineECL)
	}
	// Prevent compiler eliding the calls
	b.StopTimer()
	_ = delta
	_ = dir

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociComputeSingleMax,
		"ComputeDelta single must be < 100µs, got %s", elapsed)
}

// BenchP5M10_ValidateDirection — zero-alloc path for valid direction, < 10µs.
func BenchP5M10_ValidateDirection(b *testing.B) {
	delta := decimal.NewFromFloat(200_000_000.0).RoundBank(4)
	b.ResetTimer()
	b.ReportAllocs()

	var err error
	for i := 0; i < b.N; i++ {
		err = benchValidateDirection(delta, "INCREASE")
	}
	b.StopTimer()
	require.NoError(b, err)

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociValidateDirectMax,
		"ValidateDirection must be < 10µs, got %s", elapsed)
}

// BenchP5M10_AbsDelta — |delta| RoundBank, < 5µs.
func BenchP5M10_AbsDelta(b *testing.B) {
	delta := decimal.NewFromFloat(-150_000_000.0).RoundBank(4)
	b.ResetTimer()
	b.ReportAllocs()

	var result decimal.Decimal
	for i := 0; i < b.N; i++ {
		result = benchAbsDelta(delta)
	}
	b.StopTimer()
	_ = result

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociAbsDeltaMax,
		"AbsDeltaForJurnal must be < 5µs, got %s", elapsed)
}

// BenchP5M10_LargeDeltaCheck — bool comparison, < 5µs.
func BenchP5M10_LargeDeltaCheck(b *testing.B) {
	delta := decimal.NewFromFloat(750_000_000.0).RoundBank(4)
	b.ResetTimer()
	b.ReportAllocs()

	var result bool
	for i := 0; i < b.N; i++ {
		result = benchIsLargeDelta(delta, benchLargeDeltaThresh)
	}
	b.StopTimer()
	_ = result

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociLargeDeltaCheckMax,
		"IsLargeDelta must be < 5µs, got %s", elapsed)
}

// BenchP5M10_BaselineLookup — map lookup by instrumen_id (O1), < 50µs.
func BenchP5M10_BaselineLookup(b *testing.B) {
	// Populate a baseline map with 10k entries
	repo := make(map[uuid.UUID]decimal.Decimal, benchPociListRows10k)
	ids := make([]uuid.UUID, benchPociListRows10k)
	for i := 0; i < benchPociListRows10k; i++ {
		id := uuid.New()
		ids[i] = id
		repo[id] = decimal.NewFromFloat(float64(i+1) * 1_000_000).RoundBank(4)
	}
	target := ids[benchPociListRows10k/2]
	b.ResetTimer()
	b.ReportAllocs()

	var found decimal.Decimal
	for i := 0; i < b.N; i++ {
		found = repo[target]
	}
	b.StopTimer()
	_ = found

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociBaselineLookupMax,
		"BaselineLookup must be < 50µs, got %s", elapsed)
}

// BenchP5M10_IdempotencyCheck — duplicate detection map lookup, < 20µs.
func BenchP5M10_IdempotencyCheck(b *testing.B) {
	index := make(map[string]struct{}, benchPociListRows10k)
	calcRunID := uuid.New()
	instrumenID := uuid.New()
	key := calcRunID.String() + ":" + instrumenID.String()
	index[key] = struct{}{}
	b.ResetTimer()
	b.ReportAllocs()

	var found bool
	for i := 0; i < b.N; i++ {
		_, found = index[key]
	}
	b.StopTimer()
	_ = found

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociIdempotencyCheckMax,
		"IdempotencyCheck must be < 20µs, got %s", elapsed)
}

// BenchP5M10_HashChainEntry — SHA-256 hash-chain step for one audit entry, < 200µs.
func BenchP5M10_HashChainEntry(b *testing.B) {
	type auditEntry struct {
		Action     string `json:"action"`
		EntityID   string `json:"entity_id"`
		DeltaECL   string `json:"delta_ecl"`
		Direction  string `json:"direction"`
	}
	entry := auditEntry{
		Action:    "POCI.DELTA_COMPUTED",
		EntityID:  uuid.New().String(),
		DeltaECL:  "200000000.0000",
		Direction: "INCREASE",
	}
	prevHash := make([]byte, 32)
	data, _ := json.Marshal(entry)
	b.ResetTimer()
	b.ReportAllocs()

	var h [32]byte
	for i := 0; i < b.N; i++ {
		h = sha256.Sum256(append(prevHash, data...))
	}
	b.StopTimer()
	_ = h

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociHashChainEntryMax,
		"HashChainEntry must be < 200µs, got %s", elapsed)
}

// ─── Batch benchmarks ─────────────────────────────────────────────────────────

// BenchP5M10_ComputeDelta1000 — 1000 POCI instruments via inner loop; checks total < 5min.
// Proxies the Asynq worker ComputeDeltaForCalcRun.
func BenchP5M10_ComputeDelta1000(b *testing.B) {
	instruments := make([]benchPociInstrumen, benchPociInstrumens1k)
	for i := range instruments {
		instruments[i] = benchGenPociInstrumen(i)
	}
	b.ResetTimer()
	b.ReportAllocs()

	start := time.Now()
	for iter := 0; iter < b.N; iter++ {
		totalDelta := decimal.Zero
		for _, inst := range instruments {
			delta, _ := benchComputeDelta(inst.CurrentECL, inst.BaselineECL)
			totalDelta = totalDelta.Add(delta)
		}
		_ = totalDelta
	}
	elapsed := time.Since(start)
	b.StopTimer()

	// SLA: 1 run of 1000 instruments (1 iteration) must finish within 5 minutes.
	// With benchtime=10s and b.N ≥ 1, this checks that b.N iterations of 1000
	// instruments each completed in well under the 5-minute target.
	if b.N > 0 {
		perRun := elapsed / time.Duration(b.N)
		require.LessOrEqual(b, perRun, slaPociCompute1000Max,
			"ComputeDelta 1000 POCI must be < 5min per run, got %s", perRun)
	}
}

// BenchP5M10_Cumulative10k — sum prior delta for 1 instrument across 10k-row repo.
// Represents worst-case prior_delta_cumulative computation before opt/index.
func BenchP5M10_Cumulative10k(b *testing.B) {
	instrumenID := uuid.New()
	rows := make([]benchPociDeltaRow, benchPociListRows10k)
	// Every 5th row belongs to instrumenID (2k rows per instrument)
	for i := range rows {
		id := uuid.New()
		if i%5 == 0 {
			id = instrumenID
		}
		delta := decimal.NewFromFloat(float64(i+1) * 1000).RoundBank(4)
		rows[i] = benchPociDeltaRow{ID: uuid.New(), InstrumenID: id, DeltaECL: delta}
	}
	b.ResetTimer()
	b.ReportAllocs()

	var cumulative decimal.Decimal
	for iter := 0; iter < b.N; iter++ {
		cumulative = decimal.Zero
		for _, r := range rows {
			if r.InstrumenID == instrumenID {
				cumulative = cumulative.Add(r.DeltaECL)
			}
		}
	}
	b.StopTimer()
	_ = cumulative

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	require.LessOrEqual(b, elapsed, slaPociCumulative10kMax,
		"Cumulative10k must be < 50ms, got %s", elapsed)
}

// ─── Pagination / list benchmark ──────────────────────────────────────────────

// BenchP5M10_ListDeltaLog10k — simulate cursor-based list query over 10k rows, P95 < 200ms.
// No COUNT(*), cursor-based (DEC-022).
func BenchP5M10_ListDeltaLog10k(b *testing.B) {
	rows := make([]benchPociDeltaRow, benchPociListRows10k)
	calcRunID := uuid.New()
	for i := range rows {
		dir := "INCREASE"
		switch i % 3 {
		case 1:
			dir = "DECREASE"
		case 2:
			dir = "ZERO"
		}
		rows[i] = benchPociDeltaRow{
			ID:          uuid.New(),
			CalcRunID:   calcRunID,
			InstrumenID: uuid.New(),
			DeltaECL:    decimal.NewFromFloat(float64(i+1) * 100_000).RoundBank(4),
			Direction:   dir,
		}
	}

	const limit = 50
	// Cursor simulation: use index offset as cursor
	type listResult struct {
		rows      []benchPociDeltaRow
		nextIdx   int
		hasMore   bool
		totalEst  int
	}

	listQuery := func(filterDir string, cursorIdx int) listResult {
		var filtered []benchPociDeltaRow
		for _, r := range rows {
			if filterDir == "" || r.Direction == filterDir {
				filtered = append(filtered, r)
			}
		}
		end := cursorIdx + limit + 1
		if end > len(filtered) {
			end = len(filtered)
		}
		page := filtered[cursorIdx:end]
		hasMore := false
		if len(page) > limit {
			page = page[:limit]
			hasMore = true
		}
		return listResult{
			rows:     page,
			nextIdx:  cursorIdx + len(page),
			hasMore:  hasMore,
			totalEst: len(filtered),
		}
	}

	// Warmup
	_ = listQuery("INCREASE", 0)

	b.ResetTimer()
	b.ReportAllocs()

	samples := make([]time.Duration, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		result := listQuery("INCREASE", 0)
		samples[i] = time.Since(t0)
		_ = result
	}
	b.StopTimer()

	// Compute P95
	if b.N < 2 {
		return
	}
	// Sort samples (insertion sort for small b.N)
	for i := 1; i < len(samples); i++ {
		for j := i; j > 0 && samples[j] < samples[j-1]; j-- {
			samples[j], samples[j-1] = samples[j-1], samples[j]
		}
	}
	p95idx := int(float64(len(samples)) * 0.95)
	if p95idx >= len(samples) {
		p95idx = len(samples) - 1
	}
	p95 := samples[p95idx]
	require.LessOrEqual(b, p95, slaPociListP95Max,
		"ListDeltaLog10k P95 must be < 200ms, got %s", p95)

	b.Logf("ListDeltaLog10k P95=%s (n=%d)", p95, b.N)
}

// ─── Sub-benchmark suite ──────────────────────────────────────────────────────

// BenchP5M10 runs all P5-M10 POCI benchmarks as sub-benchmarks.
func BenchP5M10(b *testing.B) {
	b.Run("ComputeDeltaSingle", BenchP5M10_ComputeDeltaSingle)
	b.Run("ValidateDirection", BenchP5M10_ValidateDirection)
	b.Run("AbsDelta", BenchP5M10_AbsDelta)
	b.Run("LargeDeltaCheck", BenchP5M10_LargeDeltaCheck)
	b.Run("BaselineLookup", BenchP5M10_BaselineLookup)
	b.Run("IdempotencyCheck", BenchP5M10_IdempotencyCheck)
	b.Run("HashChainEntry", BenchP5M10_HashChainEntry)
	b.Run("ComputeDelta1000", BenchP5M10_ComputeDelta1000)
	b.Run("Cumulative10k", BenchP5M10_Cumulative10k)
	b.Run("ListDeltaLog10k", BenchP5M10_ListDeltaLog10k)
}

// ─── Quick smoke test (not a benchmark) ───────────────────────────────────────

// TestP5M10_PerfSmoke verifies fixtures build + delta is correct before running bench.
func TestP5M10_PerfSmoke(t *testing.T) {
	inst := benchGenPociInstrumen(0)
	require.True(t, inst.IsPoci)
	delta, dir := benchComputeDelta(inst.CurrentECL, inst.BaselineECL)
	require.NotEmpty(t, dir)
	require.True(t, delta.IsPositive() || delta.IsNegative() || delta.Equal(decimal.Zero))

	err := benchValidateDirection(delta, dir)
	require.NoError(t, err, "smoke: direction must be consistent with delta sign")

	abs := benchAbsDelta(delta)
	require.True(t, abs.IsPositive() || abs.Equal(decimal.Zero), "abs delta must be ≥ 0")

	_ = benchIsLargeDelta(delta, benchLargeDeltaThresh)
	t.Logf("smoke: inst=%s delta=%s dir=%s abs=%s",
		inst.ID, delta.StringFixed(4), dir, abs.StringFixed(4))
}

// TestP5M10_PerfSmoke_StartupCost checks fixture generation time for 1000 instruments.
func TestP5M10_PerfSmoke_1000Fixtures(t *testing.T) {
	start := time.Now()
	instruments := make([]benchPociInstrumen, benchPociInstrumens1k)
	for i := range instruments {
		instruments[i] = benchGenPociInstrumen(i)
	}
	elapsed := time.Since(start)
	require.Less(t, elapsed, 1*time.Second, "1000 fixture generation must be < 1s")
	require.Len(t, instruments, benchPociInstrumens1k)
	t.Logf("1000 POCI instrument fixtures generated in %s", elapsed)
}
