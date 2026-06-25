package penjualan

// routing.go — PURE klasifikasi routing matrix for penjualan event codes.
//
// Compliance-critical: 100% test coverage required (ifrs9-compliance-reviewer gate).
//
// Matrix (state machine doc p5-m8-penjualan.md §Klasifikasi Routing Matrix):
//   AC            → [PENJUALAN_AC],           recycleOCI=false, noRecyclingFlag=false
//   FVOCI (debt)  → [PENJUALAN_FVOCI_DEBT, REKLAS_OCI_PL], recycleOCI=true
//   FVOCI_ELECTION → [PENJUALAN_FVOCI_ELECTION], recycleOCI=false, noRecyclingFlag=true
//   FVTPL         → [PENJUALAN_FVTPL],        recycleOCI=false
//   POCI          → [PENJUALAN_POCI],         recycleOCI=false
//   klasifikasi_locked=false → error PENJUALAN_KLASIFIKASI_NOT_LOCKED
//   unknown       → error VALIDATION_FAILED
//
// Note: jenis_disposal (PARTIAL/FULL) does not affect event codes — only affects
// the OCI recycle amount computation in calc.go.

import (
	"fmt"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// RoutingResult holds the result of ResolveJurnalEventCode.
type RoutingResult struct {
	EventCodes      []string // primary + secondary event codes
	RecycleOCI      bool     // true iff FVOCI debt (requires REKLAS_OCI_PL jurnal)
	NoRecyclingFlag bool     // true iff FVOCI_ELECTION (§B5.7.1 — no P&L recycle)
}

// ResolveJurnalEventCode maps klasifikasi + klasifikasi_locked to jurnal event codes.
// Returns an error if klasifikasi is not locked or unknown.
// jenis_disposal is accepted but does not change event codes (only affects amount calc).
func ResolveJurnalEventCode(klasifikasi KlasifikasiPSAK71, locked bool, jenis DisposalType) (RoutingResult, error) {
	if !locked {
		return RoutingResult{}, domainerrors.New(
			domainerrors.CodeValidationFailed,
			fmt.Sprintf("Klasifikasi PSAK 71 belum di-lock untuk instrumen ini (klasifikasi=%s). "+
				"Penjualan hanya bisa diproses setelah klasifikasi_locked=TRUE.", klasifikasi),
			domainerrors.Detail{
				Field:   "klasifikasi",
				Rule:    "klasifikasi_locked",
				Message: "klasifikasi_locked harus TRUE sebelum penjualan diproses.",
			},
		)
	}
	// jenis is validated by validator.go before routing — accepted here for signature completeness.
	_ = jenis

	switch klasifikasi {
	case KlasifikasiAC:
		return RoutingResult{
			EventCodes:      []string{"PENJUALAN_AC"},
			RecycleOCI:      false,
			NoRecyclingFlag: false,
		}, nil

	case KlasifikasiFVOCI:
		// FVOCI debt: requires both the main penjualan leg AND REKLAS_OCI_PL recycling leg.
		return RoutingResult{
			EventCodes:      []string{"PENJUALAN_FVOCI_DEBT", "REKLAS_OCI_PL"},
			RecycleOCI:      true,
			NoRecyclingFlag: false,
		}, nil

	case KlasifikasiFVOCIElection:
		// FVOCI Election (equity): no recycling to P&L per PSAK 71 §B5.7.1.
		// G/L stays in OCI / transferred to retained earnings (non-recycled).
		return RoutingResult{
			EventCodes:      []string{"PENJUALAN_FVOCI_ELECTION"},
			RecycleOCI:      false,
			NoRecyclingFlag: true,
		}, nil

	case KlasifikasiFVTPL:
		return RoutingResult{
			EventCodes:      []string{"PENJUALAN_FVTPL"},
			RecycleOCI:      false,
			NoRecyclingFlag: false,
		}, nil

	case KlasifikasiPOCI:
		// POCI: credit-adjusted EIR basis; same event structure as AC but different cost_basis source.
		return RoutingResult{
			EventCodes:      []string{"PENJUALAN_POCI"},
			RecycleOCI:      false,
			NoRecyclingFlag: false,
		}, nil

	default:
		return RoutingResult{}, domainerrors.New(
			domainerrors.CodeValidationFailed,
			fmt.Sprintf("Klasifikasi PSAK 71 '%s' tidak dikenal. Nilai yang valid: AC, FVOCI, FVOCI_ELECTION, FVTPL, POCI.", klasifikasi),
			domainerrors.Detail{
				Field:   "klasifikasi",
				Rule:    "enum",
				Message: fmt.Sprintf("Klasifikasi '%s' tidak ada dalam matrix routing.", klasifikasi),
			},
		)
	}
}
