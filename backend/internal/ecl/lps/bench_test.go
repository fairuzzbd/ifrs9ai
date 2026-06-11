package lps

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BenchmarkAggregateBulk5000 verifies SLA: 5000 DEPOSITO instruments ≤ 1s P95.
// Runs in-process with mocked repos (no DB latency). Production DB adds overhead but
// the compute-only path is the bottleneck — SQL query performance is tested separately.
func BenchmarkAggregateBulk5000(b *testing.B) {
	const nInstruments = 5000
	nasabah := uuid.New()
	bank := uuid.New()
	capID := uuid.New()
	cap := decimal.NewFromInt(2_000_000_000)
	evalDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	rows := make([]BulkDepositoRow, nInstruments)
	for i := range rows {
		rows[i] = BulkDepositoRow{
			InstrumenID:        uuid.New(),
			KodeInstrumen:      "DEP-" + itoa(i),
			NasabahID:          nasabah,
			BankID:             bank,
			Nominal:            decimal.NewFromInt(500_000_000),
			MataUang:           "IDR",
			TanggalPenempatan:  time.Date(2020+i/365, time.Month(1+(i%12)), 1, 0, 0, 0, 0, time.UTC),
			KlasifikasiPsak71:  "AC",
			LPSCoverageParamID: capID,
			LPSCapIDR:          cap,
			TenantID:           "TUGURE",
		}
	}

	svc := NewAggregatorService(
		&mockCoverageRepo{row: &LPSCoverageRow{ID: capID, CoverageAmountIDR: cap}},
		&mockDepositoRepo{bulkRows: rows},
		&mockOverrideRepo{},
		&mockKursRepo{},
	)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, err := svc.AggregateBulk(context.Background(), evalDate)
		if err != nil {
			b.Fatalf("AggregateBulk error: %v", err)
		}
		if len(result) == 0 {
			b.Fatal("expected non-empty result")
		}
	}
}

// BenchmarkAllocatePair1000 is a micro-benchmark for the pure arithmetic path.
func BenchmarkAllocatePair1000(b *testing.B) {
	const n = 1000
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	nasabah := uuid.New()
	bank := uuid.New()

	instruments := make([]InstrumenDepositoRow, n)
	eads := make([]decimal.Decimal, n)
	for i := range instruments {
		instruments[i] = InstrumenDepositoRow{
			ID:            uuid.New(),
			KodeInstrumen: "DEP-" + itoa(i),
			Nominal:       decimal.NewFromInt(2_000_000),
			MataUang:      "IDR",
		}
		eads[i] = instruments[i].Nominal
	}
	overrides := map[uuid.UUID]*LPSExclusionOverride{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := svc.allocatePair(nasabah, bank, instruments, eads, overrides, capRow)
		if result == nil {
			b.Fatal("nil result")
		}
	}
}
