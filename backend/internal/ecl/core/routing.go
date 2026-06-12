package core

// DetermineRouting applies the per-instrument routing decision tree per
// docs/state-machines/p4-m7-ecl-core.md §1 and OpenAPI spec §"Routing decision".
//
// Decision order (evaluated top-to-bottom — first match wins):
//  1. FVTPL or FVOCI_ELECTION → SKIP_FVTPL
//  2. flag_poci = true        → POCI_DEFERRED
//  3. tipe_instrumen = REKSADANA → LOOKTHROUGH
//  4. tipe_instrumen IN (CASH, DEPOSITO) → LPS
//  5. default → STANDARD
//
// Note: POCI check is performed BEFORE tipe_instrumen routing because a DEPOSITO
// that is also POCI must still be deferred, not routed through LPS.
//
// Reference: FSD-APP-C §3, state-machine doc §1, OQ-M7-4 (FL multiplier source).
func DetermineRouting(inst *InstrumenSnapshot) RoutingPath {
	// Step 1: FVTPL / FVOCI_ELECTION → no ECL.
	// Per PSAK 71 §5.5.15: FVTPL instruments are not subject to the impairment
	// requirements. FVOCI equity election (irrevocable) also carries no ECL.
	if inst.IsFVTPL() {
		return RoutingSkipFVTPL
	}

	// Step 2: POCI instruments → defer to Phase 5.
	// Per IFRS 9 §5.4.1(c): credit-adjusted EIR required, not yet implemented.
	// This check must come BEFORE tipe_instrumen so a POCI DEPOSITO is deferred,
	// not routed to LPS (which would produce a misleading ECL = 0 for covered portion).
	if inst.FlagPOCI {
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
