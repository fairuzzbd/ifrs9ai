package pocidelta

// validator.go — Pre-condition validators for POCI delta operations.
// All validators are pure (no DB calls) — they operate on loaded domain objects.
// DB-dependent checks live in service.go (load → validate → act pattern).
//
// Returns *DomainError so response.Error maps to the correct HTTP status.

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ValidateInstrumenIsPoci returns 422 DomainError if is_poci = false.
// Called before any POCI-specific endpoint to guard against wrong instrumen.
func ValidateInstrumenIsPoci(instrumenID uuid.UUID, isPoci bool) error {
	if !isPoci {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s: instrumen %s: is_poci = FALSE, endpoint ini hanya untuk instrumen POCI",
				CodePociInstrumenNotPoci, instrumenID))
	}
	return nil
}

// ValidateBaselineNotExists returns 422 DomainError if a baseline already
// exists for this instrumen. Used before INSERT in CaptureBaseline.
func ValidateBaselineNotExists(instrumenID uuid.UUID, existing *Baseline) error {
	if existing != nil {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s: instrumen %s sudah memiliki baseline (origination: %s, ECL: %s). "+
				"DEC-018: tidak dapat di-overwrite.",
				CodePociBaselineImmutableViolation,
				instrumenID,
				existing.TanggalBaseline.Format("2006-01-02"),
				existing.LifetimeECLAtOrigination.StringFixed(4)))
	}
	return nil
}

// ValidatePeriodeLocked returns 423 DomainError if the periode status is CLOSED.
func ValidatePeriodeLocked(periodeStatus string) error {
	if periodeStatus == "CLOSED" || periodeStatus == "HARD_CLOSED" {
		return domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("%s: periode_buku status = %q — posting POCI delta tidak diperbolehkan (DEC-010)",
				CodePociPeriodeLocked, periodeStatus))
	}
	return nil
}

// ValidateCalcRunSealed ensures a calc run is in SEALED or COMPLETED status.
// POCI delta can only be computed against completed runs.
func ValidateCalcRunSealed(calcRunID uuid.UUID, status string) error {
	if status != "SEALED" && status != "COMPLETED" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("poci_delta: calc run %s status = %q — harus SEALED atau COMPLETED untuk compute delta",
				calcRunID, status))
	}
	return nil
}

// ValidateCaptureBaselineRequest validates the CaptureBaselineRequest fields.
func ValidateCaptureBaselineRequest(req CaptureBaselineRequest) error {
	if req.InstrumenID == uuid.Nil {
		return domainerrors.New(domainerrors.CodeValidationFailed, "instrumenId wajib diisi")
	}
	if req.LifetimeECLAtOrigination.IsNegative() {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("lifetimeEclAtOrigination tidak boleh negatif, got %s",
				req.LifetimeECLAtOrigination))
	}
	if req.CreditAdjustedEIR.IsZero() || req.CreditAdjustedEIR.IsNegative() {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("creditAdjustedEir harus > 0, got %s", req.CreditAdjustedEIR))
	}
	if req.CreditAdjustedEIR.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("creditAdjustedEir harus < 1 (rate desimal), got %s", req.CreditAdjustedEIR))
	}
	return nil
}
