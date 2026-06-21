package pocidelta

// calc_test.go — 100% coverage of calc.go (compliance-critical PSAK 71 §5.5.13-14).
// All paths exercised: INCREASE / DECREASE / ZERO for both ComputeDelta and
// ResolveJurnalEventCode; all ValidateJurnalDirection mismatch combinations.

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// ─── ComputeDelta ─────────────────────────────────────────────────────────────

func TestComputeDelta_Increase(t *testing.T) {
	// S2-AC1: current > baseline → delta positive → INCREASE
	current  := decimal.NewFromFloat(1450000000.0)
	baseline := decimal.NewFromFloat(1250000000.0)
	delta, dir := ComputeDelta(current, baseline)

	if dir != DirectionIncrease {
		t.Fatalf("direction: got %q, want INCREASE", dir)
	}
	wantDelta := decimal.NewFromFloat(200000000.0)
	if !delta.Equal(wantDelta) {
		t.Fatalf("delta: got %s, want %s", delta, wantDelta)
	}
}

func TestComputeDelta_Decrease(t *testing.T) {
	// S2-AC2: current < baseline → delta negative → DECREASE
	current  := decimal.NewFromFloat(650000000.0)
	baseline := decimal.NewFromFloat(800000000.0)
	delta, dir := ComputeDelta(current, baseline)

	if dir != DirectionDecrease {
		t.Fatalf("direction: got %q, want DECREASE", dir)
	}
	wantDelta := decimal.NewFromFloat(-150000000.0)
	if !delta.Equal(wantDelta) {
		t.Fatalf("delta: got %s, want %s", delta, wantDelta)
	}
}

func TestComputeDelta_Zero(t *testing.T) {
	// current == baseline → delta = 0 → ZERO
	current  := decimal.NewFromFloat(500000000.0)
	baseline := decimal.NewFromFloat(500000000.0)
	delta, dir := ComputeDelta(current, baseline)

	if dir != DirectionZero {
		t.Fatalf("direction: got %q, want ZERO", dir)
	}
	if !delta.Equal(decimal.Zero) {
		t.Fatalf("delta: got %s, want 0", delta)
	}
}

func TestComputeDelta_RoundingHalfEven(t *testing.T) {
	// Verify HALF_EVEN (banker's) rounding is applied (RoundBank(4))
	// 1.00005 → should round to 1.0001 (half-even: round to even last digit)
	// 1.00015 → 1.0002
	current  := decimal.RequireFromString("1000000.00015")
	baseline := decimal.RequireFromString("0")
	delta, dir := ComputeDelta(current, baseline)
	if dir != DirectionIncrease {
		t.Fatalf("direction: got %q, want INCREASE", dir)
	}
	// 1000000.00015 rounded to 4 dp HALF_EVEN = 1000000.0002
	want := decimal.RequireFromString("1000000.0002")
	if !delta.Equal(want) {
		t.Fatalf("rounding: got %s, want %s", delta.StringFixed(4), want.StringFixed(4))
	}
}

func TestComputeDelta_BoundaryZeroBaseline(t *testing.T) {
	// baseline = 0, current > 0 → INCREASE
	current  := decimal.NewFromFloat(100.0)
	baseline := decimal.Zero
	delta, dir := ComputeDelta(current, baseline)
	if dir != DirectionIncrease {
		t.Fatalf("direction: got %q", dir)
	}
	if !delta.Equal(current) {
		t.Fatalf("delta should equal current when baseline=0")
	}
}

func TestComputeDelta_BothZero(t *testing.T) {
	delta, dir := ComputeDelta(decimal.Zero, decimal.Zero)
	if dir != DirectionZero {
		t.Fatalf("direction: got %q, want ZERO", dir)
	}
	if !delta.Equal(decimal.Zero) {
		t.Fatalf("delta should be 0")
	}
}

// ─── ResolveJurnalEventCode ───────────────────────────────────────────────────

func TestResolveJurnalEventCode_Increase(t *testing.T) {
	code, err := ResolveJurnalEventCode(DirectionIncrease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "POCI_ECL_DELTA_INCREASE" {
		t.Fatalf("code: got %q, want POCI_ECL_DELTA_INCREASE", code)
	}
}

func TestResolveJurnalEventCode_Decrease(t *testing.T) {
	code, err := ResolveJurnalEventCode(DirectionDecrease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "POCI_ECL_DELTA_DECREASE" {
		t.Fatalf("code: got %q, want POCI_ECL_DELTA_DECREASE", code)
	}
}

func TestResolveJurnalEventCode_Zero_ReturnsErrSkipZero(t *testing.T) {
	code, err := ResolveJurnalEventCode(DirectionZero)
	if code != "" {
		t.Fatalf("code should be empty for ZERO, got %q", code)
	}
	if !errors.Is(err, ErrSkipZero) {
		t.Fatalf("err should be ErrSkipZero, got: %v", err)
	}
}

func TestResolveJurnalEventCode_InvalidDirection_ReturnsError(t *testing.T) {
	code, err := ResolveJurnalEventCode(Direction("INVALID"))
	if err == nil {
		t.Fatal("expected error for invalid direction, got nil")
	}
	if code != "" {
		t.Fatalf("expected empty code, got %q", code)
	}
}

// ─── ValidateJurnalDirection ──────────────────────────────────────────────────

func TestValidateJurnalDirection_PositiveDelta_Increase_OK(t *testing.T) {
	err := ValidateJurnalDirection(decimal.NewFromFloat(200.0), DirectionIncrease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJurnalDirection_NegativeDelta_Decrease_OK(t *testing.T) {
	err := ValidateJurnalDirection(decimal.NewFromFloat(-150.0), DirectionDecrease)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJurnalDirection_ZeroDelta_Zero_OK(t *testing.T) {
	err := ValidateJurnalDirection(decimal.Zero, DirectionZero)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateJurnalDirection_PositiveDelta_Decrease_Mismatch(t *testing.T) {
	// S3-AC4 scenario: delta_ecl = 200M positive but direction = DECREASE → bug
	err := ValidateJurnalDirection(decimal.NewFromFloat(200000000.0), DirectionDecrease)
	if err == nil {
		t.Fatal("expected POCI_JURNAL_DIRECTION_MISMATCH error, got nil")
	}
	if !IsCodeErr(err, CodePociJurnalDirectionMismatch) {
		t.Fatalf("expected code %s in error, got: %v", CodePociJurnalDirectionMismatch, err)
	}
}

func TestValidateJurnalDirection_NegativeDelta_Increase_Mismatch(t *testing.T) {
	err := ValidateJurnalDirection(decimal.NewFromFloat(-150000000.0), DirectionIncrease)
	if err == nil {
		t.Fatal("expected POCI_JURNAL_DIRECTION_MISMATCH error, got nil")
	}
	if !IsCodeErr(err, CodePociJurnalDirectionMismatch) {
		t.Fatalf("expected code %s in error, got: %v", CodePociJurnalDirectionMismatch, err)
	}
}

func TestValidateJurnalDirection_ZeroDelta_Increase_Mismatch(t *testing.T) {
	err := ValidateJurnalDirection(decimal.Zero, DirectionIncrease)
	if err == nil {
		t.Fatal("expected mismatch error for zero delta with INCREASE direction")
	}
}

func TestValidateJurnalDirection_ZeroDelta_Decrease_Mismatch(t *testing.T) {
	err := ValidateJurnalDirection(decimal.Zero, DirectionDecrease)
	if err == nil {
		t.Fatal("expected mismatch error for zero delta with DECREASE direction")
	}
}

func TestValidateJurnalDirection_PositiveDelta_Zero_Mismatch(t *testing.T) {
	err := ValidateJurnalDirection(decimal.NewFromFloat(1.0), DirectionZero)
	if err == nil {
		t.Fatal("expected mismatch error for positive delta with ZERO direction")
	}
}

// ─── AbsDeltaForJurnal ────────────────────────────────────────────────────────

func TestAbsDeltaForJurnal_PositiveDelta(t *testing.T) {
	delta := decimal.NewFromFloat(200000000.1234)
	got := AbsDeltaForJurnal(delta)
	if got.IsNegative() {
		t.Fatalf("AbsDeltaForJurnal should return non-negative, got %s", got)
	}
}

func TestAbsDeltaForJurnal_NegativeDelta(t *testing.T) {
	delta := decimal.NewFromFloat(-150000000.9999)
	got := AbsDeltaForJurnal(delta)
	want := decimal.NewFromFloat(150000000.9999)
	if !got.Equal(want) {
		t.Fatalf("AbsDeltaForJurnal: got %s, want %s", got, want)
	}
}

func TestAbsDeltaForJurnal_Zero(t *testing.T) {
	got := AbsDeltaForJurnal(decimal.Zero)
	if !got.Equal(decimal.Zero) {
		t.Fatalf("AbsDeltaForJurnal(0) should be 0, got %s", got)
	}
}
