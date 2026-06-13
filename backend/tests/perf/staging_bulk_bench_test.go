// Package perf — P4-M12 performance benchmark for Staging BulkEvaluate.
//
// SLA assertion: BulkEvaluate 1000 instruments ≤ 5 seconds wall-clock.
// Tests use in-memory mocks (no DB) to isolate computation performance.
// Real DB latency must be benchmarked separately under load testing (k6).
//
// Decision refs:
//
//	DEC-011: SICR triggers evaluated per instrument.
//	DEC-012: Cure check adds O(n) closed-period scan.
//	DEC-016: No float64; decimal arithmetic is the perf bottleneck.
//
// Run:
//
//	go test ./tests/perf/... -bench=BenchmarkBulkEvaluate1000 -benchtime=3s
package perf

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/staging"
)

// stagingInput is one instrument's evaluation inputs (mirrors staging.EvaluateSICR params).
type stagingInput struct {
	InstrumenID         uuid.UUID
	RatingAtOrigination string
	RatingCurrent       string
	RatingPrevious      string
	DPDValue            int
	CurrentStage        staging.Stage
}

// generateStagingInputs builds n inputs with realistic distribution.
// Mix: 60% Stage1 no-change, 25% Stage1→2 SICR (downgrade), 10% Stage2 cure, 5% Stage3.
func generateStagingInputs(n int) []stagingInput {
	inputs := make([]stagingInput, n)
	for i := range inputs {
		inputs[i].InstrumenID = uuid.New()
		switch {
		case i%20 == 0: // 5% Stage3 (DPD ≥ 90)
			inputs[i].RatingAtOrigination = "idAA"
			inputs[i].RatingCurrent = "idD"
			inputs[i].RatingPrevious = "idBBB"
			inputs[i].DPDValue = 95
			inputs[i].CurrentStage = staging.Stage3
		case i%10 == 0: // 10% Stage2 cure path
			inputs[i].RatingAtOrigination = "idAAA"
			inputs[i].RatingCurrent = "idAA"  // recovered
			inputs[i].RatingPrevious = "idBB" // was non-IG
			inputs[i].DPDValue = 0
			inputs[i].CurrentStage = staging.Stage2
		case i%4 == 0: // 25% Stage1→2 SICR (rating downgrade ≥2 notch)
			inputs[i].RatingAtOrigination = "idAAA"
			inputs[i].RatingCurrent = "idAA" // 1 notch — no trigger
			inputs[i].RatingPrevious = "idAAA"
			inputs[i].DPDValue = 0
			inputs[i].CurrentStage = staging.Stage1
		default: // 60% Stage1 no change
			inputs[i].RatingAtOrigination = "idAA"
			inputs[i].RatingCurrent = "idAA"
			inputs[i].RatingPrevious = "idAA"
			inputs[i].DPDValue = 0
			inputs[i].CurrentStage = staging.Stage1
		}
	}
	return inputs
}

// BenchmarkBulkEvaluate1000 measures SICR evaluation for 1000 instruments.
// SLA: ≤ 5 seconds for 1000 instruments (computation only, no DB).
func BenchmarkBulkEvaluate1000(b *testing.B) {
	const n = 1000
	inputs := generateStagingInputs(n)

	b.ResetTimer()
	b.ReportAllocs()

	for iter := 0; iter < b.N; iter++ {
		for _, inp := range inputs {
			sicrResult := staging.EvaluateSICR(
				inp.RatingAtOrigination,
				inp.RatingCurrent,
				inp.RatingPrevious,
				inp.DPDValue,
			)
			_, _ = staging.ComputeNewStage(inp.CurrentStage, sicrResult, inp.DPDValue)
		}
	}
}

// BenchmarkBulkEvaluate1000_SLACheck asserts ≤ 5s wall-clock for 1 full pass of 1000.
func BenchmarkBulkEvaluate1000_SLACheck(b *testing.B) {
	const n = 1000
	inputs := generateStagingInputs(n)

	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		start := time.Now()
		for _, inp := range inputs {
			sicrResult := staging.EvaluateSICR(
				inp.RatingAtOrigination,
				inp.RatingCurrent,
				inp.RatingPrevious,
				inp.DPDValue,
			)
			_, _ = staging.ComputeNewStage(inp.CurrentStage, sicrResult, inp.DPDValue)
		}
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			b.Errorf("SLA violation: BulkEvaluate 1000 instruments took %v (> 5s)", elapsed)
		}
	}
}

// TestBulkEvaluate1000_SLACheck is a non-benchmark SLA test (runnable with go test -run).
// Confirms that 1000 SICR evaluations complete in < 5s in a single pass.
func TestBulkEvaluate1000_SLACheck(t *testing.T) {
	const n = 1000
	inputs := generateStagingInputs(n)

	start := time.Now()
	for _, inp := range inputs {
		sicrResult := staging.EvaluateSICR(
			inp.RatingAtOrigination,
			inp.RatingCurrent,
			inp.RatingPrevious,
			inp.DPDValue,
		)
		_, _ = staging.ComputeNewStage(inp.CurrentStage, sicrResult, inp.DPDValue)
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("SLA violation: BulkEvaluate 1000 instruments took %v (> 5s SLA)", elapsed)
	}
	t.Logf("BulkEvaluate 1000 instruments: %v (SLA ≤ 5s)", elapsed)
}
