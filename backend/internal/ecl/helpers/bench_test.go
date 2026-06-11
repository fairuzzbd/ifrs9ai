// Package helpers — benchmark tests.
//
// BenchmarkBulkLookup1000 verifies the ≤ 500ms SLA for 1000 instruments
// cold-cache (no DB, all in-memory stubs).
//
// Run: go test -bench=BenchmarkBulkLookup1000 -benchtime=3s ./internal/ecl/helpers/
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// newBulkStubServices creates stub services pre-loaded for 1000 instruments.
func newBulkStubServices(n int) BulkHelperService {
	instrIDs := make([]uuid.UUID, n)
	for i := range instrIDs {
		instrIDs[i] = uuid.New()
	}

	cpID := uuid.New()

	// Stubs return identical data for all instruments to mimic realistic lookup.
	instrRepo := &stubInstrRepo{
		inst: &InstrumenRow{
			ID:                uuid.New(), // overridden per instrument in BatchLoadInstruments
			CounterpartyID:    cpID,
			KlasifikasiPsak71: "AC",
			TipeInstrumen:     "DEPOSITO",
			MatauangKode:      "IDR",
			Nominal:           decimal.NewFromInt(1_000_000_000),
			Status:            "AKTIF",
		},
		stage: Stage1,
	}

	curve := &PDCurveRow{
		Rating:    "idAA",
		PD12Month: d("0.00350000"),
	}
	pdRepo := &stubPDRepo{
		curve:    curve,
		impactPD: &ImpactPDRow{PeriodeID: "PBUKU-2026-06", ImpactMultiplier: d("1.05000000")},
		rating:   "idAA",
	}

	lgdRepo := &stubLGDRepo{
		pool:    &LGDBaselRow{TipeEksposur: "BANK", LGD: d("0.45000000")},
		mapping: map[string]string{"BANK": "BANK"},
	}

	cpRepo := &stubCPRepo{tipe: "BANK"}
	kursRepo := &stubKursRepo{}
	ccfRepo := &stubCCFRepo{}

	return NewBulkHelperService(pdRepo, lgdRepo, instrRepo, cpRepo, kursRepo, ccfRepo, nil, nil)
}

func BenchmarkBulkLookup1000(b *testing.B) {
	svc := newBulkStubServices(1000)

	reqs := make([]BulkRequest, 1000)
	for i := range reqs {
		reqs[i] = BulkRequest{InstrumenID: uuid.New()}
	}

	ctx := context.Background()
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		results, summary, errs, _, err := svc.BulkLookup(ctx, reqs, "PBUKU-2026-06", evalDate)
		if err != nil {
			b.Fatalf("BulkLookup error: %v", err)
		}
		_ = results
		_ = errs
		if summary.Total != 1000 {
			b.Fatalf("Expected total 1000, got %d", summary.Total)
		}
	}
}

// BenchmarkBulkLookup1000_SLACheck asserts ≤ 500ms wall-clock for 1 run.
func BenchmarkBulkLookup1000_SLACheck(b *testing.B) {
	svc := newBulkStubServices(1000)

	reqs := make([]BulkRequest, 1000)
	for i := range reqs {
		reqs[i] = BulkRequest{InstrumenID: uuid.New()}
	}

	ctx := context.Background()
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, _, _, _, err := svc.BulkLookup(ctx, reqs, "PBUKU-2026-06", evalDate)
		elapsed := time.Since(start)
		if err != nil {
			b.Fatalf("error: %v", err)
		}
		if elapsed > 500*time.Millisecond {
			b.Errorf("SLA violation: BulkLookup 1000 instruments took %v (> 500ms)", elapsed)
		}
	}
}
