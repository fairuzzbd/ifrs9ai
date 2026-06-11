package lookthrough

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BenchmarkBulkCompute500 benchmarks BulkCompute for 500 REKSADANA instruments.
// SLA: ≤ 2 seconds for 500 instruments (in-memory, no DB).
// Uses all mock repos — pure computation path.
func BenchmarkBulkCompute500(b *testing.B) {
	instruments := makeBenchInstruments(500)
	compID := uuid.New()

	instRepo := &mockReksadanaRepo{bulk: instruments}
	compRepo := &mockFundCompositionRepo{
		activeComp: &FundComposition{
			ID:             compID,
			WorkflowStatus: WorkflowStatusApprovedActive,
			EffectiveFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		details: []FundCompositionDetail{
			{AssetClass: AssetClassGovtBond, WeightPct: decimal.NewFromFloat(50)},
			{AssetClass: AssetClassCorpBond, WeightPct: decimal.NewFromFloat(30)},
			{AssetClass: AssetClassCash,     WeightPct: decimal.NewFromFloat(20)},
		},
	}
	pdlgdRepo := &mockPDLGDRepo{
		params: map[AssetClass]PDLGDParams{
			AssetClassGovtBond: {
				AssetClass: AssetClassGovtBond,
				PDGood:     decimal.Zero,
				PDNormal:   decimal.Zero,
				PDBad:      decimal.Zero,
				LGD:        decimal.NewFromFloat(0),
			},
			AssetClassCorpBond: {
				AssetClass: AssetClassCorpBond,
				PDGood:     decimal.NewFromFloat(0.02),
				PDNormal:   decimal.NewFromFloat(0.03),
				PDBad:      decimal.NewFromFloat(0.06),
				LGD:        decimal.NewFromFloat(0.45),
			},
			AssetClassCash: {
				AssetClass: AssetClassCash,
				PDGood:     decimal.Zero,
				PDNormal:   decimal.Zero,
				PDBad:      decimal.Zero,
				LGD:        decimal.Zero,
			},
		},
	}

	svc := newTestLookthroughService(instRepo, compRepo, pdlgdRepo, nil, nil)
	calcRunID := uuid.New()
	runID := uuid.New()
	evalDate := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.BulkCompute(context.Background(), calcRunID, runID, evalDate)
		if err != nil {
			b.Fatalf("BulkCompute error: %v", err)
		}
	}
}

// BenchmarkComputeBreakdownLine benchmarks a single breakdown line computation.
// Verifies pure formula math is fast (no I/O).
func BenchmarkComputeBreakdownLine(b *testing.B) {
	p := PDLGDParams{
		AssetClass: AssetClassCorpBond,
		PDGood:     decimal.NewFromFloat(0.02),
		PDNormal:   decimal.NewFromFloat(0.03),
		PDBad:      decimal.NewFromFloat(0.06),
		LGD:        decimal.NewFromFloat(0.45),
	}
	w := ScenarioWeights{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
	fl := FLMultipliers{
		Good:   decimal.NewFromFloat(1),
		Normal: decimal.NewFromFloat(1),
		Bad:    decimal.NewFromFloat(1),
	}
	nab := decimal.NewFromFloat(10_000_000)
	weightPct := decimal.NewFromFloat(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// ComputeBreakdownLine(assetClass, weightPct, nabIDR, pd, fl, bobot)
		_ = ComputeBreakdownLine(AssetClassCorpBond, weightPct, nab, p, fl, w)
	}
}

// BenchmarkValidateWeightSum benchmarks weight validation for typical 5-line composition.
func BenchmarkValidateWeightSum(b *testing.B) {
	pcts := []decimal.Decimal{
		decimal.NewFromFloat(20),
		decimal.NewFromFloat(30),
		decimal.NewFromFloat(20),
		decimal.NewFromFloat(15),
		decimal.NewFromFloat(15),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateWeightSumFromPcts(pcts)
	}
}

// BenchmarkSignatureHash benchmarks SHA-256 signature computation.
func BenchmarkSignatureHash(b *testing.B) {
	actorID := uuid.New()
	compositionID := uuid.New()
	signedAt := time.Now()
	comment := "ALCO approval for Q2 2026 reksadana composition"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeApproveSignatureHash(actorID, compositionID, signedAt, comment)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeBenchInstruments(n int) []InstrumenReksadanaRow {
	nab := decimal.NewFromFloat(10_000_000)
	instruments := make([]InstrumenReksadanaRow, n)
	for i := 0; i < n; i++ {
		instruments[i] = InstrumenReksadanaRow{
			ID:                uuid.New(),
			KlasifikasiPsak71: "AC",
			NominalNABIDR:     &nab,
			TipeInstrumen:     "REKSADANA",
		}
	}
	return instruments
}
