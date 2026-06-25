package renewal

// validator.go — Business rule validation for renewal requests.
// No SQL in this file. Stateless functions only.

import (
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ValidateTenor checks that tenor_baru_bulan is within [1, 60].
func ValidateTenor(tenor int) error {
	if tenor < 1 || tenor > 60 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Tenor harus antara 1 dan 60 bulan. Nilai: %d.", tenor),
			domainerrors.Detail{Field: "tenorBaruBulan", Rule: "range:1,60",
				Message: fmt.Sprintf("Tenor %d bulan di luar range 1-60.", tenor)},
		)
	}
	return nil
}

// ValidateRate checks that rate_baru_persen is within [0, 30].
func ValidateRate(rate decimal.Decimal) error {
	minRate := decimal.Zero
	maxRate := decimal.NewFromInt(30)
	if rate.LessThan(minRate) || rate.GreaterThan(maxRate) {
		f64, _ := rate.Float64()
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Rate harus antara 0%% dan 30%%. Nilai: %.4f%%.", f64),
			domainerrors.Detail{Field: "rateBaruPersen", Rule: "range:0,30",
				Message: fmt.Sprintf("Rate %.4f%% di luar range 0-30%%.", f64)},
		)
	}
	return nil
}

// ValidateSkema checks that skema is a known enum value.
func ValidateSkema(skema string) error {
	if !IsValidSkema(skema) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("skema tidak valid: '%s'. Nilai yang diterima: POKOK_SAJA, POKOK_PLUS_BUNGA.", skema),
			domainerrors.Detail{Field: "skema", Rule: "enum",
				Message: fmt.Sprintf("Nilai '%s' tidak dikenal.", skema)},
		)
	}
	return nil
}

// ValidateInstrumenEligibility checks that the instrumen can be renewed.
// Returns CodeRenewalInstrumenNotEligible on failure.
func ValidateInstrumenEligibility(inst InstrumenInfo) error {
	if inst.JenisInstrumen != "DEPOSITO" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s bukan instrumen deposito (jenis=%s). Renewal hanya untuk DEPOSITO ACTIVE.",
				inst.KodeInstrumen, inst.JenisInstrumen),
			domainerrors.Detail{Field: "instrumenId", Rule: "deposito_active",
				Message: fmt.Sprintf("jenis_instrumen=%s. Renewal hanya untuk jenis_instrumen=DEPOSITO.", inst.JenisInstrumen)},
		)
	}
	if inst.Status != "ACTIVE" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s tidak berstatus ACTIVE (status=%s). Renewal hanya untuk deposito ACTIVE.",
				inst.KodeInstrumen, inst.Status),
			domainerrors.Detail{Field: "instrumenId", Rule: "deposito_active",
				Message: fmt.Sprintf("status=%s. Renewal hanya untuk status=ACTIVE.", inst.Status)},
		)
	}
	if !inst.KlasifikasiLocked {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s: klasifikasi_locked=FALSE. Klassifikasi PSAK 71 harus terkunci sebelum renewal.",
				inst.KodeInstrumen),
			domainerrors.Detail{Field: "instrumenId", Rule: "klasifikasi_locked",
				Message: "klasifikasi_locked harus TRUE."},
		)
	}
	return nil
}

// ValidateBungaBersihMinimum validates the minimum bunga_bersih for POKOK_PLUS_BUNGA.
// Returns CodeRenewalBungaBersihTooSmall if bunga_bersih < IDR 100.000.
func ValidateBungaBersihMinimum(skema Skema, bungaBersih decimal.Decimal) error {
	if skema != SkemaPokokPlusBunga {
		return nil // only applies to POKOK_PLUS_BUNGA
	}
	if bungaBersih.LessThan(MinBungaBersih) {
		f64, _ := bungaBersih.Float64()
		min64, _ := MinBungaBersih.Float64()
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("bunga_bersih IDR %.0f lebih kecil dari minimum IDR %.0f untuk skema POKOK_PLUS_BUNGA.",
				f64, min64),
			domainerrors.Detail{Field: "skema", Rule: "min_bunga_bersih",
				Message: "Gunakan skema POKOK_SAJA atau pilih instrumen dengan bunga lebih besar."},
		)
	}
	return nil
}

// ValidatePphConsistency checks that stored PPh matches server-computed PPh (tolerance 0.01 IDR).
// This prevents client-side PPh manipulation (S4-AC3).
func ValidatePphConsistency(storedPph, bungaKotor decimal.Decimal) error {
	expectedPph := ComputePPh(bungaKotor)
	diff := storedPph.Sub(expectedPph).Abs()
	toleranceDiff := decimal.NewFromFloat(0.01)
	if diff.GreaterThan(toleranceDiff) {
		storedF64, _ := storedPph.Float64()
		expectedF64, _ := expectedPph.Float64()
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("PPh 20%% tidak sesuai: stored=%.4f, server-computed=%.4f. Gunakan nilai server-computed.",
				storedF64, expectedF64),
			domainerrors.Detail{Field: "pphAmount", Rule: "pph_20pct_mismatch",
				Message: fmt.Sprintf("Difference %.4f melebihi toleransi 0.01.", diff.InexactFloat64())},
		)
	}
	return nil
}

// ValidateSignatureMethod ensures signatureMethod is "JWT_STEP_UP".
func ValidateSignatureMethod(method string) error {
	if method != "JWT_STEP_UP" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("signatureMethod tidak valid: '%s'. Nilai yang diterima: 'JWT_STEP_UP'.", method),
			domainerrors.Detail{Field: "signatureMethod", Rule: "enum:JWT_STEP_UP",
				Message: "Hanya JWT_STEP_UP yang didukung."},
		)
	}
	return nil
}

// ValidateRejectComment ensures reject comment is ≥ 30 characters.
func ValidateRejectComment(comment string) error {
	if len([]rune(comment)) < 30 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("comment wajib minimal 30 karakter untuk reject renewal. Panjang: %d.", len([]rune(comment))),
			domainerrors.Detail{Field: "comment", Rule: "min_length:30",
				Message: fmt.Sprintf("comment hanya %d karakter.", len([]rune(comment)))},
		)
	}
	return nil
}
