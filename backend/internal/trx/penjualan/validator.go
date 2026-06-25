package penjualan

// validator.go — Business rule validation for penjualan requests.
// No SQL in this file. Stateless functions only.

import (
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ValidateDisposalType checks that jenis_disposal is a known enum value.
func ValidateDisposalType(jenis string) error {
	if !IsValidDisposalType(jenis) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("jenis_disposal tidak valid: '%s'. Nilai yang diterima: PARTIAL, FULL.", jenis),
			domainerrors.Detail{Field: "jenisDisposal", Rule: "enum",
				Message: fmt.Sprintf("Nilai '%s' tidak dikenal.", jenis)},
		)
	}
	return nil
}

// ValidateHarga checks that harga_jual_per_unit > 0.
func ValidateHarga(harga decimal.Decimal) error {
	if harga.LessThanOrEqual(decimal.Zero) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("harga_jual_per_unit harus lebih dari 0. Nilai: %s.", harga.String()),
			domainerrors.Detail{Field: "hargaJualPerUnit", Rule: "gt:0",
				Message: fmt.Sprintf("Harga %s tidak valid.", harga.String())},
		)
	}
	return nil
}

// ValidateQtyPositive checks that qty_terjual > 0.
func ValidateQtyPositive(qty decimal.Decimal) error {
	if qty.LessThanOrEqual(decimal.Zero) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			"qty_terjual harus lebih dari 0.",
			domainerrors.Detail{Field: "qtyTerjual", Rule: "gt:0",
				Message: "Quantity harus positif."},
		)
	}
	return nil
}

// ValidateQtyVsHolding checks qty_terjual vs qty_holding_pre per disposal type.
// PARTIAL: qty_terjual must be < qty_holding_pre (strictly less than).
// FULL:    qty_terjual must be = qty_holding_pre.
func ValidateQtyVsHolding(qtyTerjual, qtyHoldingPre decimal.Decimal, jenis DisposalType) error {
	if qtyTerjual.GreaterThan(qtyHoldingPre) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("qty_terjual %s melebihi qty_holding saat ini: %s.",
				qtyTerjual.StringFixed(8), qtyHoldingPre.StringFixed(8)),
			domainerrors.Detail{Field: "qtyTerjual", Rule: "lte:qty_holding",
				Message: fmt.Sprintf("qty_terjual %s > qty_holding %s.",
					qtyTerjual.StringFixed(8), qtyHoldingPre.StringFixed(8))},
		)
	}
	if jenis == DisposalPartial && qtyTerjual.Equal(qtyHoldingPre) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("PARTIAL disposal: qty_terjual harus kurang dari qty_holding. "+
				"Untuk menjual semua (%s unit), gunakan jenis_disposal=FULL.",
				qtyHoldingPre.StringFixed(8)),
			domainerrors.Detail{Field: "qtyTerjual", Rule: "lt:qty_holding_for_partial",
				Message: "Untuk PARTIAL, qty_terjual < qty_holding_pre."},
		)
	}
	if jenis == DisposalFull && !qtyTerjual.Equal(qtyHoldingPre) {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("FULL disposal: qty_terjual (%s) harus sama dengan qty_holding (%s).",
				qtyTerjual.StringFixed(8), qtyHoldingPre.StringFixed(8)),
			domainerrors.Detail{Field: "qtyTerjual", Rule: "eq:qty_holding_for_full",
				Message: "Untuk FULL, qty_terjual = qty_holding_pre."},
		)
	}
	return nil
}

// ValidateInstrumenEligibility checks that the instrumen can be sold.
func ValidateInstrumenEligibility(inst InstrumenInfo) error {
	if inst.Status != "ACTIVE" {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s tidak eligible untuk penjualan: status=%s. Hanya instrumen ACTIVE yang bisa dijual.",
				inst.KodeInstrumen, inst.Status),
			domainerrors.Detail{Field: "instrumenId", Rule: "instrumen_active",
				Message: fmt.Sprintf("status=%s.", inst.Status)},
		)
	}
	if !inst.KlasifikasiLocked {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%s: klasifikasi_locked=FALSE. Klasifikasi PSAK 71 harus terkunci sebelum penjualan.",
				inst.KodeInstrumen),
			domainerrors.Detail{Field: "instrumenId", Rule: "klasifikasi_locked",
				Message: "klasifikasi_locked harus TRUE."},
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

// ValidateRejectReason ensures reject reason is ≥ 30 characters.
func ValidateRejectReason(reason string) error {
	if len([]rune(reason)) < 30 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("reason wajib minimal 30 karakter untuk reject penjualan. Panjang: %d.", len([]rune(reason))),
			domainerrors.Detail{Field: "reason", Rule: "min_length:30",
				Message: fmt.Sprintf("reason hanya %d karakter.", len([]rune(reason)))},
		)
	}
	return nil
}
