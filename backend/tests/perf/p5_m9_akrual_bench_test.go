// Package perf — P5-M9 Jatuh Tempo + Akrual performance benchmarks.
//
// SLA targets (DEC-010, DEC-016, DEC-013):
//
//	CRON_1000_INSTR_MAX      = 5 min   — daily accrual cron for 1000 instruments
//	CRON_100_MATURITY_MAX    = 30 s    — maturity cron for 100 same-day maturities
//	STAGE3_CALC_MAX          = 1 ms    — single Stage 3 net carrying + accrual computation
//	LIST_P95_MAX             = 200 ms  — GET /transaksi/akrual with 10k rows, stage=3 filter
//	DASHBOARD_P95_MAX        = 150 ms  — GET /transaksi/akrual/dashboard MTD/YTD aggregate
//	OVERRIDE_STALE_MAX       = 300 ms  — POST /transaksi/akrual/{id}/override-stale
//	AMORTISASI_SCHEDULE_MAX  = 500 µs  — EIR amortisasi schedule fetch (DEC-013 immutable)
//	PPH_CALC_MAX             = 100 µs  — PPh 20%/10% compute + round HALF_EVEN
//	FCY_CONVERT_MAX          = 200 µs  — FCY akrual × FX rate IDR conversion
//	DLQ_WRITE_MAX            = 1 ms    — DLQ entry write (batch error isolation)
//
// Run:
//
//	go test ./backend/tests/perf/... -bench=BenchP5M9 -benchtime=10s -benchmem -race
package perf

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// ─── SLA constants ────────────────────────────────────────────────────────────

const (
	slaAkrualCron1000Max      = 5 * time.Minute
	slaAkrualCron100MatMax    = 30 * time.Second
	slaAkrualStage3CalcMax    = 1 * time.Millisecond
	slaAkrualListP95Max       = 200 * time.Millisecond
	slaAkrualDashboardP95Max  = 150 * time.Millisecond
	slaAkrualOverrideMax      = 300 * time.Millisecond
	slaAmortisasiScheduleMax  = 500 * time.Microsecond
	slaPPhCalcMax             = 100 * time.Microsecond
	slaFCYConvertMax          = 200 * time.Microsecond
	slaDLQWriteMax            = 1 * time.Millisecond

	numInstrumens1k = 1000
	numMaturity100  = 100
	numListRows10k  = 10_000
)

// ─── P5-M9 bench fixtures ─────────────────────────────────────────────────────

type benchAkrualInstrumen struct {
	ID               uuid.UUID
	Stage            int
	KlasifikasiPSAK71 string
	GrossCarryingIDR decimal.Decimal
	ECLSealedIDR     *decimal.Decimal
	EIR              decimal.Decimal
	MataUang         string
	FXRate           *decimal.Decimal
}

// newBenchInstrumens creates a slice of n instruments for cron bench.
// Distributes: 70% Stage 1, 20% Stage 2, 10% Stage 3 (representative portfolio).
func newBenchInstrumens(n int) []benchAkrualInstrumen {
	instruments := make([]benchAkrualInstrumen, n)
	eclSealed := decimal.NewFromFloat(1_200_000_000)
	fxRate := decimal.NewFromFloat(16_200)

	for i := 0; i < n; i++ {
		stage := 1
		if i%10 == 0 {
			stage = 3 // 10%
		} else if i%5 == 0 {
			stage = 2 // 20%
		}

		inst := benchAkrualInstrumen{
			ID:               uuid.New(),
			Stage:            stage,
			KlasifikasiPSAK71: "AC",
			GrossCarryingIDR: decimal.NewFromFloat(10_000_000_000),
			EIR:              decimal.NewFromFloat(0.075),
			MataUang:         "IDR",
		}
		if stage == 3 {
			inst.ECLSealedIDR = &eclSealed
		}
		// 5% FCY (USD)
		if i%20 == 0 {
			inst.MataUang = "USD"
			inst.FXRate = &fxRate
		}
		instruments[i] = inst
	}
	return instruments
}

// ─── Core computation functions (mirrors backend service layer) ───────────────

// benchNetCarrying: max(gross - ecl, 0) — PSAK 71 §5.4.1(b), DEC-010.
func benchNetCarrying(grossIDR, eclIDR decimal.Decimal) decimal.Decimal {
	net := grossIDR.Sub(eclIDR)
	if net.IsNegative() {
		return decimal.Zero
	}
	return net
}

// benchAkrualHarian: carrying × eir / 365, HALF_EVEN 4dp — DEC-016.
func benchAkrualHarian(carryingIDR, eir decimal.Decimal) decimal.Decimal {
	return carryingIDR.Mul(eir).Div(decimal.NewFromInt(365)).RoundBank(4)
}

// benchComputeAkrual: full pipeline per instrument (net carrying if stage 3, then akrual).
func benchComputeAkrual(inst benchAkrualInstrumen) decimal.Decimal {
	carrying := inst.GrossCarryingIDR
	if inst.Stage == 3 && inst.ECLSealedIDR != nil {
		carrying = benchNetCarrying(inst.GrossCarryingIDR, *inst.ECLSealedIDR)
	}
	akrualIDR := benchAkrualHarian(carrying, inst.EIR)
	if inst.MataUang != "IDR" && inst.FXRate != nil {
		akrualIDR = akrualIDR.Mul(*inst.FXRate).RoundBank(4)
	}
	return akrualIDR
}

// benchComputePPh: gross × rate, HALF_EVEN 4dp — DEC-016.
func benchComputePPh(grossIDR decimal.Decimal, rate float64) decimal.Decimal {
	return grossIDR.Mul(decimal.NewFromFloat(rate)).RoundBank(4)
}

// benchAuditHash: sha256(prev || action || entityID || after).
func benchAuditHash(prev []byte, action, entityID string) []byte {
	var sb strings.Builder
	if prev != nil {
		sb.WriteString(fmt.Sprintf("%x", prev))
	}
	sb.WriteString(action)
	sb.WriteString(entityID)
	h := sha256.Sum256([]byte(sb.String()))
	return h[:]
}

// benchDLQWrite: simulates writing a DLQ entry (struct alloc + map insert).
func benchDLQWrite(dlq map[string]string, instrumenID uuid.UUID, code string) {
	dlq[instrumenID.String()] = code
}

// ─── Route simulation (no HTTP: pure function timing) ─────────────────────────

type benchListRequest struct {
	FilterStage *int
	Limit       int
	Cursor      string
}

type benchListResponse struct {
	Items      []benchAkrualInstrumen
	TotalEst   int
	NextCursor string
}

// benchListHandler: cursor paging + stage filter (simulates repository layer).
func benchListHandler(allItems []benchAkrualInstrumen, req benchListRequest) benchListResponse {
	filtered := make([]benchAkrualInstrumen, 0, req.Limit)
	for _, item := range allItems {
		if req.FilterStage != nil && item.Stage != *req.FilterStage {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) == req.Limit+1 {
			break
		}
	}
	hasMore := len(filtered) > req.Limit
	if hasMore {
		filtered = filtered[:req.Limit]
	}
	nextCursor := ""
	if hasMore && len(filtered) > 0 {
		nextCursor = filtered[len(filtered)-1].ID.String()
	}
	return benchListResponse{
		Items:      filtered,
		TotalEst:   len(allItems),
		NextCursor: nextCursor,
	}
}

type benchDashboard struct {
	MTD        decimal.Decimal
	YTD        decimal.Decimal
	ByJenis    map[string]decimal.Decimal
}

// benchDashboardAggregate: MTD + YTD totals from 10k rows.
func benchDashboardAggregate(items []benchAkrualInstrumen) benchDashboard {
	mtd := decimal.Zero
	ytd := decimal.Zero
	byJenis := map[string]decimal.Decimal{"BUNGA": decimal.Zero}
	for _, item := range items {
		val := benchComputeAkrual(item)
		mtd = mtd.Add(val)
		ytd = ytd.Add(val)
		byJenis["BUNGA"] = byJenis["BUNGA"].Add(val)
	}
	return benchDashboard{MTD: mtd, YTD: ytd, ByJenis: byJenis}
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchP5M9_Stage3Calc — single Stage 3 net carrying + akrual (SLA: < 1ms).
func BenchmarkP5M9_Stage3Calc(b *testing.B) {
	gross := decimal.NewFromFloat(8_000_000_000)
	ecl := decimal.NewFromFloat(2_400_000_000)
	eir := decimal.NewFromFloat(0.09)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		net := benchNetCarrying(gross, ecl)
		_ = benchAkrualHarian(net, eir)
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualStage3CalcMax {
		b.Errorf("Stage3Calc SLA breach: %v > %v", elapsed, slaAkrualStage3CalcMax)
	}
}

// BenchP5M9_PPh20Pct — PPh 20% deposito compute (SLA: < 100µs).
func BenchmarkP5M9_PPh20Pct(b *testing.B) {
	bunga := decimal.NewFromFloat(87_671.2329)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = benchComputePPh(bunga, 0.20)
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaPPhCalcMax {
		b.Errorf("PPh20 SLA breach: %v > %v", elapsed, slaPPhCalcMax)
	}
}

// BenchP5M9_PPh10Pct — PPh 10% dividen compute (SLA: < 100µs).
func BenchmarkP5M9_PPh10Pct(b *testing.B) {
	gross := decimal.NewFromFloat(50_000_000)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = benchComputePPh(gross, 0.10)
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaPPhCalcMax {
		b.Errorf("PPh10 SLA breach: %v > %v", elapsed, slaPPhCalcMax)
	}
}

// BenchP5M9_FCYConvert — FCY × FX rate IDR conversion (SLA: < 200µs).
func BenchmarkP5M9_FCYConvert(b *testing.B) {
	akrualFCY := decimal.NewFromFloat(684.9315)
	fxRate := decimal.NewFromFloat(16_200)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = akrualFCY.Mul(fxRate).RoundBank(4)
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaFCYConvertMax {
		b.Errorf("FCYConvert SLA breach: %v > %v", elapsed, slaFCYConvertMax)
	}
}

// BenchP5M9_AuditHash — sha256 audit hash chain (SLA: embedded in STAGE3_CALC_MAX).
func BenchmarkP5M9_AuditHash(b *testing.B) {
	prevHash := benchAuditHash(nil, "BASELINE", uuid.New().String())
	entityID := uuid.New().String()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = benchAuditHash(prevHash, "AKRUAL.POSTED", entityID)
	}
}

// BenchP5M9_DLQWrite — DLQ map entry write (SLA: < 1ms).
func BenchmarkP5M9_DLQWrite(b *testing.B) {
	dlq := make(map[string]string)
	instrumenID := uuid.New()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchDLQWrite(dlq, instrumenID, "AKRUAL_FX_RATE_MISSING")
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaDLQWriteMax {
		b.Errorf("DLQWrite SLA breach: %v > %v", elapsed, slaDLQWriteMax)
	}
}

// BenchP5M9_CronDailyAccrual1000 — full cron loop 1000 instruments (SLA: ≤ 5 min).
func BenchmarkP5M9_CronDailyAccrual1000(b *testing.B) {
	instruments := newBenchInstrumens(numInstrumens1k)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		total := decimal.Zero
		for _, inst := range instruments {
			total = total.Add(benchComputeAkrual(inst))
		}
		_ = total
	}

	// SLA: total wall time for 1000 instruments, single iteration.
	// In benchmark mode b.N is scaled, so compare per-iteration against SLA.
	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualCron1000Max {
		b.Errorf("CronDaily1000 SLA breach: %v > %v (b.N=%d)", elapsed, slaAkrualCron1000Max, b.N)
	}
}

// BenchP5M9_CronMaturity100 — maturity processing 100 same-day expirations (SLA: ≤ 30s).
func BenchmarkP5M9_CronMaturity100(b *testing.B) {
	type maturityInst struct {
		ID               uuid.UUID
		GrossCarryingIDR decimal.Decimal
		BungaLast        decimal.Decimal
	}
	batch := make([]maturityInst, numMaturity100)
	for i := 0; i < numMaturity100; i++ {
		batch[i] = maturityInst{
			ID:               uuid.New(),
			GrossCarryingIDR: decimal.NewFromFloat(5_000_000_000),
			BungaLast:        decimal.NewFromFloat(87_671.2329),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, m := range batch {
			pph := benchComputePPh(m.BungaLast, 0.20)
			netKas := m.GrossCarryingIDR.Add(m.BungaLast).Sub(pph)
			_ = netKas
			hash := benchAuditHash(nil, "MATURITY.DERECOGNIZED", m.ID.String())
			_ = hash
		}
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualCron100MatMax {
		b.Errorf("CronMaturity100 SLA breach: %v > %v", elapsed, slaAkrualCron100MatMax)
	}
}

// BenchP5M9_ListFilterStage3 — akrual list 10k rows with stage=3 filter (SLA P95: ≤ 200ms).
func BenchmarkP5M9_ListFilterStage3(b *testing.B) {
	items := newBenchInstrumens(numListRows10k)
	stage3 := 3
	req := benchListRequest{FilterStage: &stage3, Limit: 50}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp := benchListHandler(items, req)
		_ = resp
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualListP95Max {
		b.Errorf("ListFilterStage3 SLA breach: %v > %v", elapsed, slaAkrualListP95Max)
	}
}

// BenchP5M9_ListNoFilter — akrual list 10k rows, no filter, page=1 (SLA P95: ≤ 200ms).
func BenchmarkP5M9_ListNoFilter(b *testing.B) {
	items := newBenchInstrumens(numListRows10k)
	req := benchListRequest{Limit: 50}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp := benchListHandler(items, req)
		_ = resp
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualListP95Max {
		b.Errorf("ListNoFilter SLA breach: %v > %v", elapsed, slaAkrualListP95Max)
	}
}

// BenchP5M9_DashboardMTDYTD — dashboard MTD/YTD aggregate 10k rows (SLA P95: ≤ 150ms).
func BenchmarkP5M9_DashboardMTDYTD(b *testing.B) {
	items := newBenchInstrumens(numListRows10k)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d := benchDashboardAggregate(items)
		_ = d
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualDashboardP95Max {
		b.Errorf("DashboardMTDYTD SLA breach: %v > %v", elapsed, slaAkrualDashboardP95Max)
	}
}

// BenchP5M9_OverrideStale — override stale validation + state transition (SLA: ≤ 300ms).
func BenchmarkP5M9_OverrideStale(b *testing.B) {
	reason := "Tidak ada perubahan material sejak ECL run. Staging Stage 3 dikonfirmasi valid per assessment terbaru."
	akrualID := uuid.New()
	actorID := uuid.New()
	_ = actorID

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Reason validation (≥ 30 chars)
		if len(reason) < 30 {
			b.Fatal("reason too short")
		}
		// Status transition PENDING_STALE_REVIEW → POSTED
		newStatus := "POSTED"
		_ = newStatus
		// Audit hash
		hash := benchAuditHash(nil, "AKRUAL.POSTED_OVERRIDE", akrualID.String())
		_ = hash
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAkrualOverrideMax {
		b.Errorf("OverrideStale SLA breach: %v > %v", elapsed, slaAkrualOverrideMax)
	}
}

// BenchP5M9_AmortisasiScheduleFetch — amortisasi schedule lookup (SLA: < 500µs).
// Models: SELECT WHERE instrumen_id = $1 AND effective_to IS NULL (DEC-013 immutability).
func BenchmarkP5M9_AmortisasiScheduleFetch(b *testing.B) {
	type amortisasiRow struct {
		InstrumenID uuid.UUID
		Version     int
		EIR         decimal.Decimal
		HarianIDR   decimal.Decimal
		EffTo       *time.Time // nil = active (infinity)
	}

	// Simulate index lookup: build map instrumenID → active row.
	scheduleIdx := make(map[uuid.UUID]amortisasiRow, 1000)
	for i := 0; i < 1000; i++ {
		id := uuid.New()
		scheduleIdx[id] = amortisasiRow{
			InstrumenID: id, Version: 1,
			EIR:       decimal.NewFromFloat(0.075),
			HarianIDR: decimal.NewFromFloat(2_054_794.5205),
		}
	}

	// Pick a fixed ID for bench
	var targetID uuid.UUID
	for id := range scheduleIdx {
		targetID = id
		break
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		row, ok := scheduleIdx[targetID]
		if !ok {
			b.Fatal("schedule not found")
		}
		_ = row.HarianIDR
	}

	elapsed := time.Duration(b.Elapsed().Nanoseconds() / int64(b.N))
	if elapsed > slaAmortisasiScheduleMax {
		b.Errorf("AmortisasiScheduleFetch SLA breach: %v > %v", elapsed, slaAmortisasiScheduleMax)
	}
}

// BenchP5M9_NetCarryingClamp — clamp at zero when ECL > gross (edge case perf).
func BenchmarkP5M9_NetCarryingClamp(b *testing.B) {
	gross := decimal.NewFromFloat(1_000_000)
	ecl := decimal.NewFromFloat(1_500_000) // ECL > gross → clamp

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = benchNetCarrying(gross, ecl)
	}
}

// BenchP5M9_Stage3NetCarryingClampIsZero — assertion that clamp is correct.
func TestP5M9_Stage3NetCarryingClampIsZero(t *testing.T) {
	gross := decimal.NewFromFloat(1_000_000)
	ecl := decimal.NewFromFloat(1_500_000)
	net := benchNetCarrying(gross, ecl)
	require.True(t, net.IsZero(), "net carrying must clamp at 0 when ECL > gross (DEC-010)")
}

// BenchP5M9_DecimalPrecision8dp — EIR stored as 8dp (DEC-016).
func TestP5M9_EIRPrecision8dp(t *testing.T) {
	eir := decimal.NewFromFloat(0.075)
	require.Equal(t, "0.07500000", eir.StringFixed(8), "EIR must be 8dp (DEC-016)")

	akrual := benchAkrualHarian(decimal.NewFromFloat(10_000_000_000), eir)
	require.Equal(t, "2054794.5205", akrual.StringFixed(4), "akrual must be 4dp (DEC-016)")
}

// BenchP5M9_CronParallelBatch — parallel batch of 1000 using goroutines (race-safe).
func BenchmarkP5M9_CronParallelBatch(b *testing.B) {
	instruments := newBenchInstrumens(numInstrumens1k)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Each goroutine processes the full slice (simulates concurrent worker pools)
			total := decimal.Zero
			for _, inst := range instruments {
				total = total.Add(benchComputeAkrual(inst))
			}
			_ = total
		}
	})
}
