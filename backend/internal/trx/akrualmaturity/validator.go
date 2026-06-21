package akrualmaturity

// validator.go — Domain validation for akrualmaturity package.
// All validators return domain errors that map to API error codes.

import (
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ValidateInstrumenForAkrual validates an instrument is eligible for daily accrual.
// Returns nil if valid.
func ValidateInstrumenForAkrual(inst InstrumenAkrualInfo) error {
	if inst.Status != "ACTIVE" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s tidak ACTIVE (status=%s). Akrual hanya untuk instrumen ACTIVE.",
				inst.KodeInstrumen, inst.Status))
	}
	if inst.KlasifikasiPSAK71 == "FVTPL" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s klasifikasi FVTPL dikecualikan dari akrual EIR.",
				inst.KodeInstrumen))
	}
	if inst.EIRPersen.IsZero() && inst.KlasifikasiPSAK71 != "FVTPL" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s tidak memiliki EIR yang valid. Kode: %s",
				inst.KodeInstrumen, CodeAkrualEIRNotFound))
	}
	return nil
}

// ValidateInstrumenForMaturity validates an instrument is eligible for maturity processing.
func ValidateInstrumenForMaturity(inst InstrumenAkrualInfo) error {
	if inst.Status != "ACTIVE" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s tidak ACTIVE (status=%s). Kode: %s",
				inst.KodeInstrumen, inst.Status, CodeMaturityInstrumenNotActive))
	}
	return nil
}

// ValidatePeriodeOpen validates that a periode_buku is OPEN for posting.
// Returns nil if valid.
func ValidatePeriodeOpen(statusPeriode string) error {
	if statusPeriode != "OPEN" {
		return domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("Periode buku status=%s. Akrual hanya untuk periode OPEN. Kode: %s",
				statusPeriode, CodeAkrualPeriodeLocked))
	}
	return nil
}

// ValidateSignatureMethod validates that signatureMethod is "JWT_STEP_UP".
func ValidateSignatureMethod(method string) error {
	if method != "JWT_STEP_UP" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("signatureMethod tidak valid: '%s'. Harus 'JWT_STEP_UP'.", method),
			domainerrors.Detail{
				Field:   "signatureMethod",
				Rule:    "enum",
				Message: "Harus 'JWT_STEP_UP'.",
			},
		)
	}
	return nil
}

// ValidateOverrideReason validates that override reason is at least 30 characters.
func ValidateOverrideReason(reason string) error {
	if len(reason) < 30 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Override reason terlalu pendek (%d karakter). Minimal 30 karakter.", len(reason)),
			domainerrors.Detail{
				Field:   "reason",
				Rule:    "min=30",
				Message: "Minimal 30 karakter diperlukan untuk audit trail.",
			},
		)
	}
	return nil
}

// ValidateDividenInput validates create dividen request.
func ValidateDividenInput(jumlahKotor decimal.Decimal, tanggalTerima string) error {
	if jumlahKotor.IsNegative() || jumlahKotor.IsZero() {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("gross_dividen_IDR harus > 0, got %s. Kode: %s",
				jumlahKotor, CodeDividenValidationFailed),
			domainerrors.Detail{
				Field:   "jumlahKotor",
				Rule:    "gt=0",
				Message: "gross_dividen_IDR harus > 0.",
			},
		)
	}
	if tanggalTerima == "" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			"tanggal_terima wajib diisi.",
			domainerrors.Detail{Field: "tanggalTerima", Rule: "required"},
		)
	}
	return nil
}
