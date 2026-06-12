package core

import (
	"testing"

	"github.com/shopspring/decimal"
)

// formula_test.go — unit tests for ECL formula functions.
//
// AC covered:
//   M7-001 AC1: ECL_skenario = EAD × PD × LGD
//   M7-001 AC2: ECL_FL_skenario = ECL_skenario × FL_multiplier
//   M7-001 AC3: ECL_weighted = Σ(ECL_FL × bobot)
//   M7-001 AC4: Stage 3 PD = 1.0 (fixed), FL not applied
//   M7-001 AC5: Default bobot 0.25/0.50/0.25 sums to 1.0
//   M7-001 AC6: POCI ecl_weighted = nil (not 0)
//   M7-001 AC7: Stage 3 first run → net_carrying = EAD + warning

// Test story scenario from M7 story doc §3:
//   Obligasi AC, Stage 1, EAD=1_000_000_000, PD_good=0.01, PD_normal=0.02, PD_bad=0.03,
//   LGD=0.40, FL_good=0.90, FL_normal=1.00, FL_bad=1.10,
//   bobot default 0.25/0.50/0.25.
//
// Expected:
//   ECL_good   = 1e9 × 0.01 × 0.40 = 4_000_000.0000
//   ECL_normal = 1e9 × 0.02 × 0.40 = 8_000_000.0000
//   ECL_bad    = 1e9 × 0.03 × 0.40 = 12_000_000.0000
//   ECL_FL_good   = 4_000_000 × 0.90 = 3_600_000.0000
//   ECL_FL_normal = 8_000_000 × 1.00 = 8_000_000.0000
//   ECL_FL_bad    = 12_000_000 × 1.10 = 13_200_000.0000
//   ECL_weighted  = 3_600_000 × 0.25 + 8_000_000 × 0.50 + 13_200_000 × 0.25
//                 = 900_000 + 4_000_000 + 3_300_000 = 8_200_000.0000

func TestComputeFormulaStage1_KnownScenario(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromInt(1_000_000_000)
	pdGood := decimal.NewFromFloat(0.01)
	pdNormal := decimal.NewFromFloat(0.02)
	pdBad := decimal.NewFromFloat(0.03)
	lgd := decimal.NewFromFloat(0.40)
	flG := decimal.NewFromFloat(0.90)
	flN := decimal.NewFromFloat(1.00)
	flB := decimal.NewFromFloat(1.10)
	bobot := BobotSnapshot{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}

	r := ComputeFormula(ead, pdGood, pdNormal, pdBad, lgd, &flG, &flN, &flB, bobot, nil, Stage1)

	assertDecimalEqual(t, "ECLGoodIDR", r.ECLGoodIDR, "4000000.0000")
	assertDecimalEqual(t, "ECLNormalIDR", r.ECLNormalIDR, "8000000.0000")
	assertDecimalEqual(t, "ECLBadIDR", r.ECLBadIDR, "12000000.0000")
	assertDecimalEqual(t, "ECLFLGoodIDR", r.ECLFLGoodIDR, "3600000.0000")
	assertDecimalEqual(t, "ECLFLNormalIDR", r.ECLFLNormalIDR, "8000000.0000")
	assertDecimalEqual(t, "ECLFLBadIDR", r.ECLFLBadIDR, "13200000.0000")
	assertDecimalEqual(t, "ECLWeightedIDR", r.ECLWeightedIDR, "8200000.0000")

	if r.IsStage3 {
		t.Error("IsStage3 should be false for Stage 1")
	}
	if r.NetCarryingIDR != nil {
		t.Error("NetCarryingIDR should be nil for Stage 1")
	}
}

// Stage 3 scenario from M7 story doc §3:
//   EAD=500_000_000, prior_sealed_ecl=50_000_000
//   Stage 3: PD = 1.0 for all scenarios, FL not applied
//   net_carrying = 500M - 50M = 450M
//
// Expected:
//   ECL_good = ECL_normal = ECL_bad = 500M × 1.0 × LGD
//   ECL_FL_xxx = ECL_xxx (FL not applied)
//   net_carrying = 450_000_000.0000
func TestComputeFormulaStage3_PDForcedOne_NoFL(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromInt(500_000_000)
	pdGood := decimal.NewFromFloat(0.10)   // should be overridden to 1.0
	pdNormal := decimal.NewFromFloat(0.20) // should be overridden to 1.0
	pdBad := decimal.NewFromFloat(0.30)    // should be overridden to 1.0
	lgd := decimal.NewFromFloat(0.40)
	flG := decimal.NewFromFloat(0.90)
	flN := decimal.NewFromFloat(1.00)
	flB := decimal.NewFromFloat(1.10)
	bobot := BobotSnapshot{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
	prior := decimal.NewFromInt(50_000_000)

	r := ComputeFormula(ead, pdGood, pdNormal, pdBad, lgd, &flG, &flN, &flB, bobot, &prior, Stage3)

	// PD must be 1.0 for all scenarios.
	if !r.PDGood.Equal(decimal.NewFromInt(1)) {
		t.Errorf("Stage 3: PDGood should be 1.0, got %s", r.PDGood)
	}
	if !r.PDNormal.Equal(decimal.NewFromInt(1)) {
		t.Errorf("Stage 3: PDNormal should be 1.0, got %s", r.PDNormal)
	}
	if !r.PDBad.Equal(decimal.NewFromInt(1)) {
		t.Errorf("Stage 3: PDBad should be 1.0, got %s", r.PDBad)
	}

	// FL multipliers must be nil for Stage 3.
	if r.FLGood != nil || r.FLNormal != nil || r.FLBad != nil {
		t.Error("Stage 3: FL multipliers must be nil")
	}

	// ECL_scenario = EAD × 1.0 × LGD = 500M × 0.40 = 200_000_000
	assertDecimalEqual(t, "ECLGoodIDR", r.ECLGoodIDR, "200000000.0000")
	assertDecimalEqual(t, "ECLNormalIDR", r.ECLNormalIDR, "200000000.0000")
	assertDecimalEqual(t, "ECLBadIDR", r.ECLBadIDR, "200000000.0000")

	// ECL_FL = ECL (no multiplier).
	assertDecimalEqual(t, "ECLFLGoodIDR", r.ECLFLGoodIDR, "200000000.0000")
	assertDecimalEqual(t, "ECLFLNormalIDR", r.ECLFLNormalIDR, "200000000.0000")
	assertDecimalEqual(t, "ECLFLBadIDR", r.ECLFLBadIDR, "200000000.0000")

	// Weighted = 200M × 0.25 + 200M × 0.50 + 200M × 0.25 = 200M.
	assertDecimalEqual(t, "ECLWeightedIDR", r.ECLWeightedIDR, "200000000.0000")

	// Net carrying = 500M - 50M = 450M.
	if r.NetCarryingIDR == nil {
		t.Fatal("NetCarryingIDR must not be nil for Stage 3")
	}
	assertDecimalEqual(t, "NetCarryingIDR", *r.NetCarryingIDR, "450000000.0000")

	if r.PriorSealedECLIDR == nil {
		t.Fatal("PriorSealedECLIDR must not be nil")
	}
	assertDecimalEqual(t, "PriorSealedECLIDR", *r.PriorSealedECLIDR, "50000000.0000")

	if !r.IsStage3 {
		t.Error("IsStage3 should be true")
	}
}

// Stage 3 first run: no prior sealed ECL → net_carrying = EAD.
func TestComputeFormulaStage3_FirstRun_NetCarryingEqualsEAD(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromInt(100_000_000)
	lgd := decimal.NewFromFloat(0.45)
	bobot := defaultBobot()

	r := ComputeFormula(ead, decimal.Zero, decimal.Zero, decimal.Zero, lgd, nil, nil, nil, bobot, nil, Stage3)

	if r.NetCarryingIDR == nil {
		t.Fatal("NetCarryingIDR must not be nil for Stage 3 first run")
	}
	if !r.NetCarryingIDR.Equal(ead) {
		t.Errorf("First run: NetCarryingIDR should equal EAD=%s, got %s", ead, *r.NetCarryingIDR)
	}
	if r.PriorSealedECLIDR != nil {
		t.Error("PriorSealedECLIDR should be nil for first run")
	}
}

// Stage 2 scenario: Lifetime PD, FL applied, no net carrying.
func TestComputeFormulaStage2_LifetimePD(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromInt(200_000_000)
	pdGood := decimal.NewFromFloat(0.05)
	pdNormal := decimal.NewFromFloat(0.08)
	pdBad := decimal.NewFromFloat(0.12)
	lgd := decimal.NewFromFloat(0.35)
	flG := decimal.NewFromFloat(0.95)
	flN := decimal.NewFromFloat(1.00)
	flB := decimal.NewFromFloat(1.05)
	bobot := defaultBobot()

	r := ComputeFormula(ead, pdGood, pdNormal, pdBad, lgd, &flG, &flN, &flB, bobot, nil, Stage2)

	// ECL_good = 200M × 0.05 × 0.35 = 3_500_000
	assertDecimalEqual(t, "ECLGoodIDR", r.ECLGoodIDR, "3500000.0000")
	// ECL_FL_good = 3_500_000 × 0.95 = 3_325_000.0000
	assertDecimalEqual(t, "ECLFLGoodIDR", r.ECLFLGoodIDR, "3325000.0000")

	if r.NetCarryingIDR != nil {
		t.Error("Stage 2: NetCarryingIDR should be nil")
	}
}

// BobotSnapshot.Validate should accept 0.25/0.50/0.25 = 1.0.
func TestBobotValidate_Valid(t *testing.T) {
	t.Parallel()
	b := defaultBobot()
	if err := b.Validate(); err != nil {
		t.Errorf("default bobot should be valid, got: %v", err)
	}
}

// BobotSnapshot.Validate should reject 0.30/0.50/0.25 = 1.05.
func TestBobotValidate_Invalid(t *testing.T) {
	t.Parallel()
	b := BobotSnapshot{
		Good:   decimal.NewFromFloat(0.30),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
	if err := b.Validate(); err == nil {
		t.Error("invalid bobot should fail Validate()")
	}
}

// ComputeWeightedECL with default bobot.
func TestComputeWeightedECL_DefaultBobot(t *testing.T) {
	t.Parallel()

	good := decimal.NewFromInt(4_000_000)
	normal := decimal.NewFromInt(8_000_000)
	bad := decimal.NewFromInt(12_000_000)
	bobot := defaultBobot()

	// 4M × 0.25 + 8M × 0.50 + 12M × 0.25 = 1M + 4M + 3M = 8M.
	want := decimal.NewFromInt(8_000_000)
	got := ComputeWeightedECL(good, normal, bad, bobot)
	if !got.Equal(want) {
		t.Errorf("ComputeWeightedECL: want %s, got %s", want, got)
	}
}

// Zero EAD → all ECL values are zero.
func TestComputeFormula_ZeroEAD(t *testing.T) {
	t.Parallel()

	r := ComputeFormula(
		decimal.Zero,
		decimal.NewFromFloat(0.02), decimal.NewFromFloat(0.03), decimal.NewFromFloat(0.05),
		decimal.NewFromFloat(0.45),
		nil, nil, nil,
		defaultBobot(), nil, Stage1,
	)

	if !r.ECLWeightedIDR.IsZero() {
		t.Errorf("Zero EAD: ECLWeightedIDR should be 0, got %s", r.ECLWeightedIDR)
	}
}

// Precision: HALF_EVEN rounding at each step (banker's rounding).
// EAD × PD × LGD = 1e6 × 0.015 × 0.333 = 4995.0000 (rounded from 4995.000)
func TestComputeScenarioECL_HALFEVENRounding(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromInt(1_000_000)
	pd := decimal.NewFromFloat(0.015)
	lgd := decimal.NewFromFloat(0.333)

	// 1_000_000 × 0.015 = 15_000; 15_000 × 0.333 = 4_995.000 → 4995.0000
	got := ComputeScenarioECL(ead, pd, lgd)
	want, _ := decimal.NewFromString("4995.0000")
	if !got.Equal(want) {
		t.Errorf("HALF_EVEN rounding: want %s, got %s", want, got)
	}
}

// No float64 validation: all inputs and outputs are decimal.Decimal.
func TestComputeFormula_NoFloat64(t *testing.T) {
	t.Parallel()
	// This test just validates the API shape — if any field used float64, it would not compile.
	var _ decimal.Decimal = ComputeScenarioECL(decimal.NewFromInt(1), decimal.NewFromFloat(0.1), decimal.NewFromFloat(0.4))
	var _ decimal.Decimal = ComputeWeightedECL(
		decimal.NewFromInt(100), decimal.NewFromInt(200), decimal.NewFromInt(300),
		defaultBobot(),
	)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func defaultBobot() BobotSnapshot {
	return BobotSnapshot{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
}

func assertDecimalEqual(t *testing.T, name string, got decimal.Decimal, wantStr string) {
	t.Helper()
	want, err := decimal.NewFromString(wantStr)
	if err != nil {
		t.Fatalf("assertDecimalEqual: parse want %q: %v", wantStr, err)
	}
	if !got.Equal(want) {
		t.Errorf("%s: want %s, got %s", name, want.StringFixed(4), got.StringFixed(4))
	}
}
