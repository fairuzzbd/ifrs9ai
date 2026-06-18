package kurs

// treatment.go — PSAK 71 FX accounting treatment decision tree.
//
// Pure function: Decide(klasifikasi, mataUang) → (Treatment, reasoning, error).
// No I/O, no DB access, no HTTP. Tested in isolation in treatment_test.go.
//
// PSAK 71 / IFRS 9 reference:
//   - IFRS 9 §5.7.3: foreign currency denominated instruments — FX movement via P&L.
//   - IAS 39 §AG83: FVOCI debt: FX on monetary component via P&L for AC-equivalent,
//     but IFRS 9 §4.1.2A says FX goes to OCI (recyclable) for FVOCI debt.
//   - FSD-APP-D-FX-RATE-v1.0 §5 (FX Treatment Decision Tree).
//
// Decision matrix:
//
//	Klasifikasi       | Mata Uang | Treatment
//	------------------+-----------+------------------
//	AC                | IDR       | NO_FX_TREATMENT
//	AC                | FCY       | P_AND_L (IFRS9 §5.7.3)
//	FVOCI_DEBT        | IDR       | NO_FX_TREATMENT
//	FVOCI_DEBT        | FCY       | OCI_RECYCLABLE
//	FVOCI_ELECTION    | IDR       | NO_FX_TREATMENT
//	FVOCI_ELECTION    | FCY       | OCI_NO_RECYCLE (IFRS9 §5.7.5 — no recycling)
//	FVTPL             | IDR       | NO_FX_TREATMENT
//	FVTPL             | FCY       | P_AND_L
//	(any unknown)     | any       | error

import "fmt"

// Decide returns the PSAK 71 FX treatment for an instrument given its klasifikasi
// and mata_uang.
//
// Parameters:
//   - klasifikasi: one of "AC", "FVOCI_DEBT", "FVOCI_ELECTION", "FVTPL"
//   - mataUang: ISO 4217 3-char code; "IDR" maps to TreatmentNoFX
//
// Returns (Treatment, reasoning, nil) on success; ("", "", error) on unknown klasifikasi.
func Decide(klasifikasi, mataUang string) (Treatment, string, error) {
	if mataUang == "IDR" {
		return TreatmentNoFX,
			"Instrument is IDR-denominated; no FX revaluation required.",
			nil
	}

	switch klasifikasi {
	case "AC":
		return TreatmentPnL,
			"AC + FCY: FX gain/loss recognised in Profit & Loss per IFRS 9 §5.7.3. " +
				"EIR interest at each reporting date uses BI JISDOR mid-rate.",
			nil

	case "FVOCI_DEBT":
		return TreatmentOCIRecyclable,
			"FVOCI debt + FCY: FX component goes to OCI (recyclable). " +
				"Recycled to P&L on derecognition per IFRS 9 §4.1.2A.",
			nil

	case "FVOCI_ELECTION":
		return TreatmentOCINoRecycle,
			"FVOCI Election (equity) + FCY: FX goes to OCI with no recycling to P&L on disposal. " +
				"Irrevocable election per IFRS 9 §5.7.5.",
			nil

	case "FVTPL":
		return TreatmentPnL,
			"FVTPL + FCY: full fair value change (including FX) goes to P&L per IFRS 9 §5.7.1.",
			nil

	default:
		return "",
			"",
			fmt.Errorf("unknown klasifikasi %q: must be one of AC, FVOCI_DEBT, FVOCI_ELECTION, FVTPL", klasifikasi)
	}
}
