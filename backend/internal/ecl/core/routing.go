package core

// DetermineRouting applies the per-instrument routing decision tree per
// docs/state-machines/p4-m7-ecl-core.md §1 and OpenAPI spec §"Routing decision".
//
// Decision order (evaluated top-to-bottom — first match wins):
//  1. FVTPL or FVOCI_ELECTION → SKIP_FVTPL
//  2. flag_poci = true AND HasCAEIRSchedule = true  → POCI_COMPUTED  (Phase 4.5, DEC-POCI-001)
//  2. flag_poci = true AND HasCAEIRSchedule = false → POCI_DEFERRED  (no CA-EIR yet)
//  3. tipe_instrumen = REKSADANA → LOOKTHROUGH
//  4. tipe_instrumen IN (CASH, DEPOSITO) → LPS
//  5. default → STANDARD
//
// Note: POCI check is performed BEFORE tipe_instrumen routing because a DEPOSITO
// that is also POCI must still be deferred/computed, not routed through LPS.
//
// POCI routing (F2 fix, DEC-POCI-001):
//   - POCI_COMPUTED: CA-EIR schedule present — standard ECL formula applied, row written.
//     Warnings: POCI_CA_EIR_COMPUTED, POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA.
//   - POCI_DEFERRED: no CA-EIR schedule yet — no ecl.calc_result_line row written, warning emitted.
//     Phase 5 will transition these to POCI_COMPUTED once CA-EIR is computed.
//   HasCAEIRSchedule is populated by InstrumenReaderIface.GetByID via HasPOCISchedule repo call.
//
// Reference: FSD-APP-C §3, state-machine doc §1, OQ-M7-4 (FL multiplier source).
func DetermineRouting(inst *InstrumenSnapshot) RoutingPath {
	// Step 1: FVTPL / FVOCI_ELECTION → no ECL.
	// Per PSAK 71 §5.5.15: FVTPL instruments are not subject to the impairment
	// requirements. FVOCI equity election (irrevocable) also carries no ECL.
	if inst.IsFVTPL() {
		return RoutingSkipFVTPL
	}

	// Step 2: POCI instruments — route based on CA-EIR schedule availability.
	// This check must come BEFORE tipe_instrumen so a POCI DEPOSITO is not routed
	// to LPS (which would produce a misleading ECL = 0 for covered portion).
	//
	// F2 fix (DEC-POCI-001): when CA-EIR schedule exists (HasCAEIRSchedule=true),
	// return POCI_COMPUTED → ECL is computed via STANDARD formula in handlePOCI.
	// When CA-EIR not yet available, return POCI_DEFERRED → no row written, warning emitted.
	if inst.FlagPOCI {
		if inst.HasCAEIRSchedule {
			return RoutingPOCIComputed
		}
		return RoutingPOCIDeferred
	}

	// Step 3: Reksadana → look-through delegation to M4.
	if inst.IsReksadana() {
		return RoutingLookthrough
	}

	// Step 4: Cash / Deposito → LPS aggregator (M3), ECL on excess only.
	// Per DEC-014: IDR 2B cap per nasabah per bank applied BEFORE ECL.
	if inst.IsLPS() {
		return RoutingLPS
	}

	// Default: STANDARD path via M2 PD/LGD/EAD helpers.
	return RoutingStandard
}
