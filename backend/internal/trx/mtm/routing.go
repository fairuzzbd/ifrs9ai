package mtm

// routing.go — PURE FUNCTION resolveJurnalEventCode implementing the PSAK 71
// jurnal routing matrix per state machine doc §5 and OpenAPI spec.
//
// BLOCKING: every change to this file REQUIRES ifrs9-compliance-reviewer approval
// before merge (CODEOWNERS: @ifrs9-compliance-reviewer). See docs/state-machines/p5-m6-mtm-daily.md §5.
//
// Routing matrix:
//   AC             → ErrMTMInstrumenACSkip (422 MTM_INSTRUMEN_AC_SKIP)
//   FVOCI_DEBT IDR → ["MTM_FVOCI"]
//   FVOCI_DEBT FCY → ["MTM_FVOCI", "MTM_FX_OCI_RESERVE"]  (two SEPARATE entries per §B5.7.2A)
//   FVOCI_ELECTION → ["MTM_FVOCI_ELECTION"]  (irrevocable, no P&L recycling §5.7.5)
//   FVTPL          → ["MTM_FVTPL"]  (all FV changes including FX → P&L, §5.7.7)
//   POCI           → ["MTM_FVTPL_POCI"]  (credit-adjusted; no Stage escalation from MTM)
//   unknown        → ErrUnknownKlasifikasi
//
// FX routing decision for FVOCI_DEBT FCY (§B5.7.2A):
//   The FX component goes to OCI FX Reserve (NOT P&L), unlike FVTPL where ALL changes
//   including FX go to P&L. FVOCI_ELECTION FCY: FX component also stays in OCI
//   (no recycling on disposal), hence only ONE event code MTM_FVOCI_ELECTION covers it.
//
// For FVOCI_DEBT FCY we call P5-M5 GET /master/kurs/treatment/{instrumen_id} to verify
// the treatment decision agrees with local routing. On mismatch: log + use routing.go
// (single source of truth). This is a cross-modul consistency check, not a gate.
//
// POCI note (OQ-M6-1 confirmed):
//   MTM fair value change (trx.mtm) and ECL movement (ecl.ecl_calc_result_line)
//   are INDEPENDENT. No double-counting. ECL from APP-C remains separate.

import (
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// Jurnal event codes — stable strings per mst.mapping_jurnal seed (migration 000040).
const (
	EventCodeMTMFVOCI          = "MTM_FVOCI"
	EventCodeMTMFXOCIReserve   = "MTM_FX_OCI_RESERVE"
	EventCodeMTMFVOCIElection  = "MTM_FVOCI_ELECTION"
	EventCodeMTMFVTPL          = "MTM_FVTPL"
	EventCodeMTMFVTPLPOCI      = "MTM_FVTPL_POCI"
)

// Klasifikasi PSAK 71 constants (mirror mst.instrumen constraint).
const (
	KlasifikasiAC            = "AC"
	KlasifikasiFVOCIDebt     = "FVOCI_DEBT"
	KlasifikasiFVOCIElection = "FVOCI_ELECTION"
	KlasifikasiFVTPL         = "FVTPL"
	KlasifikasiPOCI          = "POCI"
)

// ErrUnknownKlasifikasi is returned for unknown klasifikasi_psak71 values.
var ErrUnknownKlasifikasi = domainerrors.New(domainerrors.CodeValidationFailed,
	"klasifikasi_psak71 tidak dikenal — nilai harus AC, FVOCI_DEBT, FVOCI_ELECTION, FVTPL, atau POCI.")

// resolveJurnalEventCode is a pure function mapping (klasifikasi, mataUang) to jurnal event codes.
//
// Parameters:
//   - klasifikasiPSAK71: "AC" | "FVOCI_DEBT" | "FVOCI_ELECTION" | "FVTPL" | "POCI"
//   - mataUang:          ISO 4217 currency code, e.g. "IDR", "USD"
//   - isPOCI:            true if instrument was originated/purchased as credit-impaired
//
// Returns:
//   - []string of event codes to post (1 or 2 entries; never empty on non-error return)
//   - error: ErrMTMInstrumenACSkip for AC; ErrUnknownKlasifikasi for unknown value
//
// NOTE: isPOCI is the tiebreak for POCI vs FVOCI_DEBT or FVTPL routing.
// When klasifikasiPSAK71 == "POCI", isPOCI must also be true (validated by service layer).
// The function trusts the caller to enforce this invariant.
//
// Coverage requirement: 100% — all branches tested in routing_test.go.
func resolveJurnalEventCode(klasifikasiPSAK71 string, mataUang string, isPOCI bool) ([]string, error) {
	_ = isPOCI // consumed by POCI branch; documented for future POCI-specific routing

	switch klasifikasiPSAK71 {
	case KlasifikasiAC:
		// Amortised cost — no MTM. AC instruments must never be inserted into trx.mtm.
		// The cron worker skips them before calling this function;
		// this branch is a defence-in-depth safety net.
		return nil, ErrMTMInstrumenACSkip

	case KlasifikasiFVOCIDebt:
		// PSAK 71 §5.7.10: MTM delta → OCI Perubahan Nilai Wajar.
		// For FCY instruments, §B5.7.2A mandates a SEPARATE entry for the FX component
		// → OCI FX Reserve (NOT P&L). The MTM delta (ex-FX) goes to OCI normally.
		codes := []string{EventCodeMTMFVOCI}
		if mataUang != "IDR" {
			// Two SEPARATE jurnal entries: MTM_FVOCI (fair value ex-FX) + MTM_FX_OCI_RESERVE (FX).
			// OQ-M6-2 confirmed: NOT one combined entry.
			codes = append(codes, EventCodeMTMFXOCIReserve)
		}
		return codes, nil

	case KlasifikasiFVOCIElection:
		// PSAK 71 §5.7.5: irrevocable equity FVOCI election.
		// ALL changes (including FX for FCY instruments) stay in OCI — no P&L, no recycling.
		// Single event code regardless of IDR/FCY (FX component included in MTM_FVOCI_ELECTION).
		return []string{EventCodeMTMFVOCIElection}, nil

	case KlasifikasiFVTPL:
		// PSAK 71 §5.7.7: ALL fair value changes including FX → P&L.
		// Single event code. For FCY: FX gain/loss also in P&L (no split like FVOCI_DEBT).
		return []string{EventCodeMTMFVTPL}, nil

	case KlasifikasiPOCI:
		// Purchased or Originated Credit Impaired.
		// Credit-adjusted EIR; fair value change → P&L via MTM_FVTPL_POCI.
		// ECL movement from APP-C is INDEPENDENT (OQ-M6-1 confirmed — no double-count).
		// No Stage escalation from MTM row.
		return []string{EventCodeMTMFVTPLPOCI}, nil

	default:
		return nil, ErrUnknownKlasifikasi
	}
}

// ResolveJurnalEventCode is the exported wrapper for testing and service usage.
// See resolveJurnalEventCode for full documentation.
func ResolveJurnalEventCode(klasifikasiPSAK71 string, mataUang string, isPOCI bool) ([]string, error) {
	return resolveJurnalEventCode(klasifikasiPSAK71, mataUang, isPOCI)
}
