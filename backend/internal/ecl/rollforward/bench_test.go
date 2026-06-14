package rollforward_test

// bench_test.go — BenchmarkComputeRollForward1000 (P4-M11 SLA: ≤ 5s for 1000 instruments).
//
// Tests pure computation functions (detectTransfers, detectLifecycle, SumMovement,
// remeasurements residual formula) without DB to isolate algorithm performance.
// With 1000 instruments and O(n) maps, this should run in <100ms for in-process test.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

func BenchmarkComputeRollForward1000(b *testing.B) {
	const n = 1000

	// Build 1000 prior lines: mixed Stage 1/2/3
	prior := make([]rollforward.ResultLineHeader, n)
	current := make([]rollforward.ResultLineHeader, n)
	stageHistory := make(map[uuid.UUID]rollforward.StageHistoryRow, n)
	instrumenStatuses := make(map[uuid.UUID]rollforward.InstrumenStatusSnapshot, n)

	// 700 same-stage (remeasurement), 100 stage1→2, 100 stage2→1, 100 stage2→3.
	// Remaining 0: these are the boundary case.
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		ids[i] = uuid.New()
	}

	ecl100 := decimal.RequireFromString("100000.0000")
	ecl200 := decimal.RequireFromString("200000.0000")

	for i := 0; i < n; i++ {
		id := ids[i]
		ead := decimal.RequireFromString("1000000.0000")
		switch {
		case i < 700:
			// Same stage 1 (remeasurement bucket)
			prior[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 1, EclWeightedIdr: &ecl100, EadIdr: ead}
			current[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 1, EclWeightedIdr: &ecl200, EadIdr: ead}
		case i < 800:
			// Stage 1→2
			prior[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 1, EclWeightedIdr: &ecl100, EadIdr: ead}
			current[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 2, EclWeightedIdr: &ecl200, EadIdr: ead}
		case i < 900:
			// Stage 2→1 cure
			prior[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 2, EclWeightedIdr: &ecl200, EadIdr: ead}
			current[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 1, EclWeightedIdr: &ecl100, EadIdr: ead}
		default:
			// Stage 2→3 default
			prior[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 2, EclWeightedIdr: &ecl100, EadIdr: ead}
			current[i] = rollforward.ResultLineHeader{InstrumenID: id, Stage: 3, EclWeightedIdr: &ecl200, EadIdr: ead}
		}
	}

	assessmentDate := time.Now()
	currentCalcRunID := uuid.New()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		// This mirrors the core computation in ComputeRollForward.
		transfers, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)
		originations, derecognitions, _ := rollforward.ExportDetectLifecycle(
			prior, current, instrumenStatuses, currentCalcRunID, assessmentDate,
		)

		opening := sumDecPtrs(prior)
		closing := sumDecPtrs(current)

		remeasurements := closing.
			Sub(opening).
			Sub(transfers.SumMovement()).
			Sub(originations.EclIdr).
			Add(derecognitions.PriorEclIdr)

		reconcileCheck := opening.
			Add(transfers.SumMovement()).
			Add(originations.EclIdr).
			Sub(derecognitions.PriorEclIdr).
			Add(remeasurements)

		delta := closing.Sub(reconcileCheck)
		_ = delta
	}
}

func sumDecPtrs(lines []rollforward.ResultLineHeader) decimal.Decimal {
	total := decimal.Zero
	for _, l := range lines {
		if l.EclWeightedIdr != nil {
			total = total.Add(*l.EclWeightedIdr)
		}
	}
	return total
}
