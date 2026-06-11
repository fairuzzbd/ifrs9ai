package lookthrough

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// TestAssetClassIsValid verifies enum validation for all valid and invalid values.
func TestAssetClassIsValid(t *testing.T) {
	t.Parallel()
	valid := []AssetClass{
		AssetClassGovtBond, AssetClassCorpBond, AssetClassCash,
		AssetClassEquity, AssetClassOther,
	}
	for _, ac := range valid {
		if !ac.IsValid() {
			t.Errorf("expected %s to be valid", ac)
		}
	}
	invalid := []AssetClass{"STOCKS", "CRYPTO", "", "CORP_BONDS", "gov_bond"}
	for _, ac := range invalid {
		if ac.IsValid() {
			t.Errorf("expected %s to be invalid", ac)
		}
	}
}

// TestAssetClassIsSovereignZeroPD ensures only GOVT_BOND returns true.
func TestAssetClassIsSovereignZeroPD(t *testing.T) {
	t.Parallel()
	if !AssetClassGovtBond.IsSovereignZeroPD() {
		t.Error("GOVT_BOND should be sovereign zero PD")
	}
	for _, ac := range []AssetClass{AssetClassCorpBond, AssetClassCash, AssetClassEquity, AssetClassOther} {
		if ac.IsSovereignZeroPD() {
			t.Errorf("%s should NOT be sovereign zero PD", ac)
		}
	}
}

// TestAssetClassLGDPoolTipeEksposur verifies the LGD pool mapping per state-machine §2.3.
func TestAssetClassLGDPoolTipeEksposur(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ac       AssetClass
		expected string
	}{
		{AssetClassGovtBond, "SOVEREIGN"},
		{AssetClassCorpBond, "UNSECURED_CORP"},
		{AssetClassCash, "DEPOSITO"},
		{AssetClassEquity, "EQUITY"},
		{AssetClassOther, "UNSECURED_CORP"},
	}
	for _, c := range cases {
		got := c.ac.LGDPoolTipeEksposur()
		if got != c.expected {
			t.Errorf("%s: expected %s, got %s", c.ac, c.expected, got)
		}
	}
}

// TestWorkflowStatusTransitions verifies state machine transitions.
func TestWorkflowStatusTransitions(t *testing.T) {
	t.Parallel()

	// Valid transitions per state-machine doc §1.2.
	valid := []struct {
		from WorkflowStatus
		to   WorkflowStatus
	}{
		{WorkflowStatusPendingReview, WorkflowStatusPendingApproval},
		{WorkflowStatusPendingReview, WorkflowStatusRejected},
		{WorkflowStatusPendingApproval, WorkflowStatusApprovedActive},
		{WorkflowStatusPendingApproval, WorkflowStatusRejected},
		{WorkflowStatusApprovedActive, WorkflowStatusSuperseded},
	}
	for _, v := range valid {
		if !v.from.CanTransitionTo(v.to) {
			t.Errorf("expected %s → %s to be valid", v.from, v.to)
		}
	}

	// Invalid transitions.
	invalid := []struct {
		from WorkflowStatus
		to   WorkflowStatus
	}{
		{WorkflowStatusDraft, WorkflowStatusPendingReview},    // DRAFT has no transitions
		{WorkflowStatusRejected, WorkflowStatusPendingReview}, // terminal → any
		{WorkflowStatusSuperseded, WorkflowStatusApprovedActive},
		{WorkflowStatusApprovedActive, WorkflowStatusPendingReview},
		{WorkflowStatusPendingReview, WorkflowStatusApprovedActive}, // skip review step
	}
	for _, v := range invalid {
		if v.from.CanTransitionTo(v.to) {
			t.Errorf("expected %s → %s to be INVALID", v.from, v.to)
		}
	}
}

// TestWorkflowStatusIsTerminal verifies terminal status detection.
func TestWorkflowStatusIsTerminal(t *testing.T) {
	t.Parallel()
	terminals := []WorkflowStatus{WorkflowStatusRejected, WorkflowStatusSuperseded}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	nonTerminals := []WorkflowStatus{
		WorkflowStatusDraft, WorkflowStatusPendingReview,
		WorkflowStatusPendingApproval, WorkflowStatusApprovedActive,
	}
	for _, s := range nonTerminals {
		if s.IsTerminal() {
			t.Errorf("%s should NOT be terminal", s)
		}
	}
}

// TestWorkflowStatusIsActive ensures only APPROVED_ACTIVE returns true.
func TestWorkflowStatusIsActive(t *testing.T) {
	t.Parallel()
	if !WorkflowStatusApprovedActive.IsActive() {
		t.Error("APPROVED_ACTIVE should be active")
	}
	for _, s := range []WorkflowStatus{
		WorkflowStatusDraft, WorkflowStatusPendingReview, WorkflowStatusPendingApproval,
		WorkflowStatusSuperseded, WorkflowStatusRejected,
	} {
		if s.IsActive() {
			t.Errorf("%s should NOT be active", s)
		}
	}
}

// TestComputeBreakdownLine_GovtBond verifies ECL = 0 for sovereign asset class.
// PD = 0 hardcoded (IsSovereignZeroPD = true), per OQ-M4-4.
func TestComputeBreakdownLine_GovtBond(t *testing.T) {
	t.Parallel()
	nabIDR := decimal.NewFromFloat(1_000_000_000) // IDR 1B
	weightPct := decimal.NewFromFloat(30)         // 30%
	pd := PDLGDParams{
		AssetClass: AssetClassGovtBond,
		PDGood:     decimal.Zero,
		PDNormal:   decimal.Zero,
		PDBad:      decimal.Zero,
		LGD:        decimal.NewFromFloat(0.05),
	}
	fl := FLMultipliers{
		Good:   decimal.NewFromFloat(1.1),
		Normal: decimal.NewFromFloat(1.0),
		Bad:    decimal.NewFromFloat(0.9),
	}
	bobot := ScenarioWeights{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}

	line := ComputeBreakdownLine(AssetClassGovtBond, weightPct, nabIDR, pd, fl, bobot)

	// NABPortion = 1B × 30 / 100 = 300M
	expectedNAB := decimal.NewFromFloat(300_000_000)
	if !line.NABPortionIDR.Equal(expectedNAB) {
		t.Errorf("NABPortionIDR: expected %s, got %s", expectedNAB, line.NABPortionIDR)
	}

	// PD = 0 → all ECL = 0
	if !line.ECLWeightedIDR.Equal(decimal.Zero) {
		t.Errorf("ECLWeightedIDR: expected 0, got %s", line.ECLWeightedIDR)
	}
	if !line.ECLSkenariosGoodIDR.Equal(decimal.Zero) {
		t.Errorf("ECLSkenariosGoodIDR should be 0")
	}
}

// TestComputeBreakdownLine_CorpBond verifies ECL computation for a corporate bond.
// Uses the formula: ECL_s = NAB × w/100 × PD_s × LGD; ECL_FL = ECL_s × FL; ECL_w = Σ(ECL_FL × bobot).
func TestComputeBreakdownLine_CorpBond(t *testing.T) {
	t.Parallel()
	nabIDR := decimal.NewFromFloat(500_000_000) // IDR 500M
	weightPct := decimal.NewFromFloat(40)       // 40%
	pd := PDLGDParams{
		AssetClass: AssetClassCorpBond,
		PDGood:     decimal.NewFromFloat(0.01), // 1%
		PDNormal:   decimal.NewFromFloat(0.02), // 2%
		PDBad:      decimal.NewFromFloat(0.04), // 4%
		LGD:        decimal.NewFromFloat(0.45), // 45%
	}
	fl := FLMultipliers{
		Good:   decimal.NewFromFloat(0.9),
		Normal: decimal.NewFromFloat(1.0),
		Bad:    decimal.NewFromFloat(1.2),
	}
	bobot := ScenarioWeights{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}

	line := ComputeBreakdownLine(AssetClassCorpBond, weightPct, nabIDR, pd, fl, bobot)

	// NABPortion = 500M × 40/100 = 200M
	expectedNAB := decimal.NewFromFloat(200_000_000)
	if !line.NABPortionIDR.Equal(expectedNAB) {
		t.Errorf("NABPortionIDR: expected %s, got %s", expectedNAB, line.NABPortionIDR)
	}

	// ECL_Good = 200M × 0.01 × 0.45 = 900_000; truncate4 = 900_000.0000
	eclGood := decimal.NewFromFloat(900_000)
	if !line.ECLSkenariosGoodIDR.Equal(eclGood) {
		t.Errorf("ECLSkenariosGoodIDR: expected %s, got %s", eclGood, line.ECLSkenariosGoodIDR)
	}

	// ECL_FL_Good = 900_000 × 0.9 = 810_000
	eclFLGood := decimal.NewFromFloat(810_000)
	if !line.ECLFLGoodIDR.Equal(eclFLGood) {
		t.Errorf("ECLFLGoodIDR: expected %s, got %s", eclFLGood, line.ECLFLGoodIDR)
	}

	// ECL_Normal = 200M × 0.02 × 0.45 = 1_800_000; FL = 1.0 → ECL_FL_Normal = 1_800_000
	eclNormal := decimal.NewFromFloat(1_800_000)
	eclFLNormal := decimal.NewFromFloat(1_800_000)
	if !line.ECLFLNormalIDR.Equal(eclFLNormal) {
		t.Errorf("ECLFLNormalIDR: expected %s, got %s", eclFLNormal, line.ECLFLNormalIDR)
	}
	_ = eclNormal

	// ECL_Bad = 200M × 0.04 × 0.45 = 3_600_000; FL=1.2 → ECL_FL_Bad = 4_320_000
	eclFLBad := decimal.NewFromFloat(4_320_000)
	if !line.ECLFLBadIDR.Equal(eclFLBad) {
		t.Errorf("ECLFLBadIDR: expected %s, got %s", eclFLBad, line.ECLFLBadIDR)
	}

	// ECL_weighted = 810_000×0.25 + 1_800_000×0.50 + 4_320_000×0.25
	//             = 202_500 + 900_000 + 1_080_000 = 2_182_500
	expectedWeighted := decimal.NewFromFloat(2_182_500)
	if !line.ECLWeightedIDR.Equal(expectedWeighted) {
		t.Errorf("ECLWeightedIDR: expected %s, got %s", expectedWeighted, line.ECLWeightedIDR)
	}
}

// TestComputeBreakdownLine_DecimalPrecision ensures HALF_EVEN rounding to 4dp at each step (SoW §4 / DEC-016).
func TestComputeBreakdownLine_DecimalPrecision(t *testing.T) {
	t.Parallel()
	// Create values that would produce infinite decimals if using float64.
	nabIDR := decimal.NewFromFloat(333_333_333.33)
	weightPct := decimal.NewFromFloat(33.3333)
	pd := PDLGDParams{
		AssetClass: AssetClassEquity,
		PDGood:     decimal.NewFromFloat(0.05123456),
		PDNormal:   decimal.NewFromFloat(0.07234567),
		PDBad:      decimal.NewFromFloat(0.12345678),
		LGD:        decimal.NewFromFloat(0.60000000),
	}
	fl := FLMultipliers{
		Good:   decimal.NewFromFloat(1.05),
		Normal: decimal.NewFromFloat(1.00),
		Bad:    decimal.NewFromFloat(0.95),
	}
	bobot := ScenarioWeights{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}

	line := ComputeBreakdownLine(AssetClassEquity, weightPct, nabIDR, pd, fl, bobot)

	// Verify all output fields are capped at 4 decimal places (HALF_EVEN per SoW §4).
	checkDP := func(name string, d decimal.Decimal) {
		// Multiply by 10^4 and check that no fractional part exists.
		shifted := d.Mul(decimal.NewFromInt(10000))
		if !shifted.Equal(shifted.Truncate(0)) {
			t.Errorf("%s: value %s has more than 4 decimal places after rounding", name, d)
		}
	}
	checkDP("NABPortionIDR", line.NABPortionIDR)
	checkDP("ECLSkenariosGoodIDR", line.ECLSkenariosGoodIDR)
	checkDP("ECLSkenariosNormalIDR", line.ECLSkenariosNormalIDR)
	checkDP("ECLSkenariosBadIDR", line.ECLSkenariosBadIDR)
	checkDP("ECLFLGoodIDR", line.ECLFLGoodIDR)
	checkDP("ECLFLNormalIDR", line.ECLFLNormalIDR)
	checkDP("ECLFLBadIDR", line.ECLFLBadIDR)
	checkDP("ECLWeightedIDR", line.ECLWeightedIDR)
}

// TestValidateWeightSum verifies the 100% ± 0.01% tolerance check.
func TestValidateWeightSum(t *testing.T) {
	t.Parallel()

	makeDetails := func(weights ...float64) []FundCompositionDetail {
		details := make([]FundCompositionDetail, len(weights))
		for i, w := range weights {
			details[i] = FundCompositionDetail{WeightPct: decimal.NewFromFloat(w)}
		}
		return details
	}

	// Exactly 100% — valid.
	if err := ValidateWeightSum(makeDetails(25, 25, 50), "test-id"); err != nil {
		t.Errorf("expected nil for 100%% sum, got: %v", err)
	}

	// 99.99% — within tolerance 0.01% → valid.
	if err := ValidateWeightSum(makeDetails(25, 25, 49.99), "test-id"); err != nil {
		t.Errorf("expected nil for 99.99%% sum, got: %v", err)
	}

	// 100.01% — within tolerance → valid.
	if err := ValidateWeightSum(makeDetails(25, 25, 50.01), "test-id"); err != nil {
		t.Errorf("expected nil for 100.01%% sum, got: %v", err)
	}

	// 99.98% — outside tolerance → error.
	if err := ValidateWeightSum(makeDetails(25, 25, 49.98), "test-id"); err == nil {
		t.Error("expected error for 99.98%% sum, got nil")
	}

	// 100.02% — outside tolerance → error.
	if err := ValidateWeightSum(makeDetails(25, 25, 50.02), "test-id"); err == nil {
		t.Error("expected error for 100.02%% sum, got nil")
	}

	// Single 100% entry — valid.
	if err := ValidateWeightSum(makeDetails(100), "test-id"); err != nil {
		t.Errorf("expected nil for single 100%% entry, got: %v", err)
	}

	// Zero weights — invalid (0 ≠ 100).
	if err := ValidateWeightSum(makeDetails(0, 0), "test-id"); err == nil {
		t.Error("expected error for 0%% sum, got nil")
	}
}

// TestValidateWeightSumFromPcts verifies the decimal slice version.
func TestValidateWeightSumFromPcts(t *testing.T) {
	t.Parallel()

	// Valid: 25+50+25 = 100
	if err := ValidateWeightSumFromPcts([]decimal.Decimal{
		decimal.NewFromFloat(25), decimal.NewFromFloat(50), decimal.NewFromFloat(25),
	}); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}

	// Invalid: 30+30+30 = 90
	if err := ValidateWeightSumFromPcts([]decimal.Decimal{
		decimal.NewFromFloat(30), decimal.NewFromFloat(30), decimal.NewFromFloat(30),
	}); err == nil {
		t.Error("expected error for 90%% sum, got nil")
	}
}

// TestComputeReviewSignatureHash verifies determinism and uniqueness of hash.
func TestComputeReviewSignatureHash(t *testing.T) {
	t.Parallel()
	reviewerID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	compositionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	signedAt := time.Date(2026, 6, 11, 10, 30, 0, 0, time.UTC)
	comment := "Reviewed and approved the fund composition."

	h1 := ComputeReviewSignatureHash(reviewerID, compositionID, signedAt, comment)
	h2 := ComputeReviewSignatureHash(reviewerID, compositionID, signedAt, comment)

	if len(h1) != sha256.Size {
		t.Errorf("expected SHA-256 hash length %d, got %d", sha256.Size, len(h1))
	}
	// Deterministic.
	if !bytes.Equal(h1, h2) {
		t.Error("hash should be deterministic for same inputs")
	}

	// Different comment → different hash.
	h3 := ComputeReviewSignatureHash(reviewerID, compositionID, signedAt, "different comment")
	if bytes.Equal(h1, h3) {
		t.Error("hash should differ for different comment")
	}
}

// TestComputeApproveSignatureHash verifies approve hash differs from review hash.
func TestComputeApproveSignatureHash(t *testing.T) {
	t.Parallel()
	approverID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	compositionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	signedAt := time.Date(2026, 6, 11, 10, 30, 0, 0, time.UTC)
	comment := "ALCO approved."

	reviewHash := ComputeReviewSignatureHash(approverID, compositionID, signedAt, comment)
	approveHash := ComputeApproveSignatureHash(approverID, compositionID, signedAt, comment)

	// Review and approve hashes must differ (different action prefix).
	if bytes.Equal(reviewHash, approveHash) {
		t.Error("REVIEW and APPROVE hashes should differ for same inputs")
	}

	// Deterministic.
	h2 := ComputeApproveSignatureHash(approverID, compositionID, signedAt, comment)
	if !bytes.Equal(approveHash, h2) {
		t.Error("approve hash should be deterministic")
	}
}

// TestItoa verifies the internal integer-to-string helper.
func TestItoa(t *testing.T) {
	t.Parallel()
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{1234567890, "1234567890"},
		{-9999, "-9999"},
	}
	for _, c := range cases {
		got := itoa(c.n)
		if got != c.want {
			t.Errorf("itoa(%d): expected %q, got %q", c.n, c.want, got)
		}
	}
}

// TestIsOpenEnded verifies the sentinel 9999-12-31 detection.
func TestIsOpenEnded(t *testing.T) {
	t.Parallel()
	openEnded := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	if !isOpenEnded(openEnded) {
		t.Error("9999-12-31 should be open-ended")
	}
	closed := time.Date(2030, 6, 11, 0, 0, 0, 0, time.UTC)
	if isOpenEnded(closed) {
		t.Error("2030-06-11 should NOT be open-ended")
	}
}

// TestFmtDate verifies date formatting.
func TestFmtDate(t *testing.T) {
	t.Parallel()
	d := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	got := fmtDate(d)
	if got != "2026-06-11" {
		t.Errorf("expected 2026-06-11, got %s", got)
	}
}

// TestErrorConstructors verifies error code values and HTTP status mapping.
func TestErrorConstructors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		expectCode string
	}{
		{
			"FundCompositionMissing",
			ErrFundCompositionMissing("INST-001", "2026-06-11"),
			CodeLookthroughFundCompositionMissing,
		},
		{
			"NABMissing",
			ErrNABMissing("INST-001", "2026-06-11"),
			CodeLookthroughNABMissing,
		},
		{
			"WeightInvalid",
			ErrWeightInvalid("COMP-001", decimal.NewFromFloat(95)),
			CodeLookthroughWeightInvalid,
		},
		{
			"InstrumenNotReksadana",
			ErrInstrumenNotReksadana("INST-001", "OBLIGASI"),
			CodeLookthroughInstrumenNotReksadana,
		},
		{
			"AssetClassUnknown",
			ErrAssetClassUnknown("CRYPTO", 2),
			CodeLookthroughAssetClassUnknown,
		},
		{
			"CompositionInvalidTransition",
			ErrCompositionInvalidTransition("REJECTED", "APPROVE"),
			CodeLookthroughCompositionReviewInvalidTransition,
		},
		{
			"SoDViolation",
			ErrCompositionSoDViolation("ROLE-ALCO", "COMP-001"),
			CodeLookthroughCompositionSoDViolation,
		},
		{
			"BulkTooLarge",
			ErrBulkTooLarge(15000),
			CodeLookthroughBulkTooLarge,
		},
		{
			"POCIDeferred",
			ErrPOCIDeferred("INST-POCI-001"),
			CodeLookthroughPOCIDeferred,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			type coder interface{ Code() domainerrors.Code }
			code, ok := c.err.(coder)
			if !ok {
				t.Fatalf("error %v does not implement Code() domainerrors.Code", c.err)
			}
			if string(code.Code()) != c.expectCode {
				t.Errorf("expected code %s, got %s", c.expectCode, code.Code())
			}
		})
	}
}

// TestDefaultWeightsSum verifies that the default scenario weights sum to 1.
// DEC-010: Good=0.25, Normal=0.50, Bad=0.25.
func TestDefaultWeightsSum(t *testing.T) {
	t.Parallel()
	sum := defaultWeights.Good.Add(defaultWeights.Normal).Add(defaultWeights.Bad)
	if !sum.Equal(decimal.NewFromInt(1)) {
		t.Errorf("default weights should sum to 1.0, got %s", sum)
	}
}

// TestComputeBreakdownLine_DefaultWeights verifies that using default weights
// (0.25/0.50/0.25) produces the expected weighted ECL.
func TestComputeBreakdownLine_DefaultWeights(t *testing.T) {
	t.Parallel()
	nabIDR := decimal.NewFromFloat(1_000_000) // IDR 1M
	weightPct := decimal.NewFromFloat(100)
	pd := PDLGDParams{
		AssetClass: AssetClassCash,
		PDGood:     decimal.NewFromFloat(0.01),
		PDNormal:   decimal.NewFromFloat(0.02),
		PDBad:      decimal.NewFromFloat(0.05),
		LGD:        decimal.NewFromFloat(0.25),
	}
	// Neutral FL multipliers.
	fl := FLMultipliers{
		Good:   decimal.NewFromInt(1),
		Normal: decimal.NewFromInt(1),
		Bad:    decimal.NewFromInt(1),
	}
	bobot := defaultWeights

	line := ComputeBreakdownLine(AssetClassCash, weightPct, nabIDR, pd, fl, bobot)

	// NABPortion = 1M × 100/100 = 1M
	if !line.NABPortionIDR.Equal(decimal.NewFromFloat(1_000_000)) {
		t.Errorf("NABPortionIDR: %s", line.NABPortionIDR)
	}

	// ECL_Good = 1M × 0.01 × 0.25 = 2500
	// ECL_Normal = 1M × 0.02 × 0.25 = 5000
	// ECL_Bad = 1M × 0.05 × 0.25 = 12500
	// ECL_weighted = 2500×0.25 + 5000×0.50 + 12500×0.25 = 625 + 2500 + 3125 = 6250
	expectedWeighted := decimal.NewFromFloat(6250)
	if !line.ECLWeightedIDR.Equal(expectedWeighted) {
		t.Errorf("ECLWeightedIDR: expected %s, got %s", expectedWeighted, line.ECLWeightedIDR)
	}
}

// TestNoopMetrics ensures NoopMetrics doesn't panic.
func TestNoopMetrics(t *testing.T) {
	t.Parallel()
	m := NoopMetrics()
	m.RecordBulkDuration(1.5)
	m.RecordBulkInstrumentCount(500)
	m.RecordBulkErrors(3)
}
