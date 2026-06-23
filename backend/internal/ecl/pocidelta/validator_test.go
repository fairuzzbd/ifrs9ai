package pocidelta

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestValidateInstrumenIsPoci_True(t *testing.T) {
	if err := ValidateInstrumenIsPoci(uuid.New(), true); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestValidateInstrumenIsPoci_False(t *testing.T) {
	err := ValidateInstrumenIsPoci(uuid.New(), false)
	if err == nil {
		t.Fatal("expected POCI_INSTRUMEN_NOT_POCI error")
	}
	if !IsCodeErr(err, CodePociInstrumenNotPoci) {
		t.Fatalf("wrong code in error: %v", err)
	}
}

func TestValidateBaselineNotExists_NoBaseline(t *testing.T) {
	if err := ValidateBaselineNotExists(uuid.New(), nil); err != nil {
		t.Fatalf("expected nil for no existing baseline, got: %v", err)
	}
}

func TestValidateBaselineNotExists_ExistingBaseline(t *testing.T) {
	existing := &Baseline{
		InstrumenID:              uuid.New(),
		TanggalBaseline:         time.Now(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
	}
	err := ValidateBaselineNotExists(uuid.New(), existing)
	if err == nil {
		t.Fatal("expected POCI_BASELINE_IMMUTABLE_VIOLATION")
	}
	if !IsCodeErr(err, CodePociBaselineImmutableViolation) {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestValidatePeriodeLocked_Open(t *testing.T) {
	if err := ValidatePeriodeLocked("OPEN"); err != nil {
		t.Fatalf("expected nil for OPEN: %v", err)
	}
}

func TestValidatePeriodeLocked_Closed(t *testing.T) {
	err := ValidatePeriodeLocked("CLOSED")
	if err == nil {
		t.Fatal("expected POCI_PERIODE_LOCKED")
	}
	if !IsCodeErr(err, CodePociPeriodeLocked) {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestValidatePeriodeLocked_HardClosed(t *testing.T) {
	err := ValidatePeriodeLocked("HARD_CLOSED")
	if err == nil {
		t.Fatal("expected POCI_PERIODE_LOCKED for HARD_CLOSED")
	}
}

func TestValidateCalcRunSealed_Sealed(t *testing.T) {
	if err := ValidateCalcRunSealed(uuid.New(), "SEALED"); err != nil {
		t.Fatalf("expected nil for SEALED: %v", err)
	}
}

func TestValidateCalcRunSealed_Completed(t *testing.T) {
	if err := ValidateCalcRunSealed(uuid.New(), "COMPLETED"); err != nil {
		t.Fatalf("expected nil for COMPLETED: %v", err)
	}
}

func TestValidateCalcRunSealed_Running(t *testing.T) {
	err := ValidateCalcRunSealed(uuid.New(), "RUNNING")
	if err == nil {
		t.Fatal("expected error for RUNNING calc run")
	}
}

func TestValidateCaptureBaselineRequest_Valid(t *testing.T) {
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	if err := ValidateCaptureBaselineRequest(req); err != nil {
		t.Fatalf("expected nil: %v", err)
	}
}

func TestValidateCaptureBaselineRequest_NilInstrumen(t *testing.T) {
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.Nil,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	if err := ValidateCaptureBaselineRequest(req); err == nil {
		t.Fatal("expected error for nil instrumenId")
	}
}

func TestValidateCaptureBaselineRequest_NegativeECL(t *testing.T) {
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(-1),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	if err := ValidateCaptureBaselineRequest(req); err == nil {
		t.Fatal("expected error for negative ECL")
	}
}

func TestValidateCaptureBaselineRequest_ZeroEIR(t *testing.T) {
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.Zero,
	}
	if err := ValidateCaptureBaselineRequest(req); err == nil {
		t.Fatal("expected error for zero EIR")
	}
}

func TestValidateCaptureBaselineRequest_EIRGreaterThanOne(t *testing.T) {
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(1.5), // > 1, invalid rate
	}
	if err := ValidateCaptureBaselineRequest(req); err == nil {
		t.Fatal("expected error for EIR >= 1")
	}
}
