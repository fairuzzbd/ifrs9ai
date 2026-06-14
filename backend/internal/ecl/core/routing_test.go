package core

import (
	"testing"
)

// routing_test.go — exhaustive routing decision tree tests.
//
// Decision order (DetermineRouting):
//  1. FVTPL / FVOCI_ELECTION  → SKIP_FVTPL
//  2a. flag_poci + CA-EIR     → POCI_COMPUTED  (F2 fix, DEC-POCI-001)
//  2b. flag_poci + no CA-EIR  → POCI_DEFERRED
//  3. REKSADANA               → LOOKTHROUGH
//  4. CASH / DEPOSITO         → LPS
//  5. default                 → STANDARD

func TestDetermineRouting_FVTPL(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "FVTPL", TipeInstrumen: "OBLIGASI"}
	got := DetermineRouting(inst)
	if got != RoutingSkipFVTPL {
		t.Errorf("FVTPL: want %s, got %s", RoutingSkipFVTPL, got)
	}
}

func TestDetermineRouting_FVOCIElection(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "FVOCI_ELECTION", TipeInstrumen: "SAHAM"}
	got := DetermineRouting(inst)
	if got != RoutingSkipFVTPL {
		t.Errorf("FVOCI_ELECTION: want %s, got %s", RoutingSkipFVTPL, got)
	}
}

func TestDetermineRouting_POCI_BeforeLPS(t *testing.T) {
	t.Parallel()
	// POCI DEPOSITO must be deferred, NOT routed to LPS.
	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "DEPOSITO",
		FlagPOCI:          true,
	}
	got := DetermineRouting(inst)
	if got != RoutingPOCIDeferred {
		t.Errorf("POCI DEPOSITO: want %s, got %s", RoutingPOCIDeferred, got)
	}
}

func TestDetermineRouting_POCI_BeforeReksadana(t *testing.T) {
	t.Parallel()
	// POCI REKSADANA must be deferred, NOT routed to LOOKTHROUGH.
	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "REKSADANA",
		FlagPOCI:          true,
	}
	got := DetermineRouting(inst)
	if got != RoutingPOCIDeferred {
		t.Errorf("POCI REKSADANA: want %s, got %s", RoutingPOCIDeferred, got)
	}
}

func TestDetermineRouting_FVTPL_TakesPrecedenceOverPOCI(t *testing.T) {
	t.Parallel()
	// FVTPL with POCI flag: FVTPL check is first in order.
	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "FVTPL",
		TipeInstrumen:     "OBLIGASI",
		FlagPOCI:          true,
	}
	got := DetermineRouting(inst)
	if got != RoutingSkipFVTPL {
		t.Errorf("FVTPL+POCI: FVTPL wins, want %s, got %s", RoutingSkipFVTPL, got)
	}
}

func TestDetermineRouting_Reksadana(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "AC", TipeInstrumen: "REKSADANA"}
	got := DetermineRouting(inst)
	if got != RoutingLookthrough {
		t.Errorf("REKSADANA: want %s, got %s", RoutingLookthrough, got)
	}
}

func TestDetermineRouting_Cash(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "AC", TipeInstrumen: "CASH"}
	got := DetermineRouting(inst)
	if got != RoutingLPS {
		t.Errorf("CASH: want %s, got %s", RoutingLPS, got)
	}
}

func TestDetermineRouting_Deposito(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "AC", TipeInstrumen: "DEPOSITO"}
	got := DetermineRouting(inst)
	if got != RoutingLPS {
		t.Errorf("DEPOSITO: want %s, got %s", RoutingLPS, got)
	}
}

func TestDetermineRouting_Obligasi_Standard(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "AC", TipeInstrumen: "OBLIGASI"}
	got := DetermineRouting(inst)
	if got != RoutingStandard {
		t.Errorf("OBLIGASI AC: want %s, got %s", RoutingStandard, got)
	}
}

func TestDetermineRouting_Saham_FVOCIDebt_Standard(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{KlasifikasiPsak71: "FVOCI", TipeInstrumen: "SAHAM"}
	// FVOCI (debt) with SAHAM — not FVTPL, not POCI, not REKSADANA, not CASH/DEPOSITO → STANDARD.
	got := DetermineRouting(inst)
	if got != RoutingStandard {
		t.Errorf("SAHAM FVOCI: want %s, got %s", RoutingStandard, got)
	}
}

// Verify IsFVTPL helper.
func TestInstrumenSnapshot_IsFVTPL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		klasifikasi string
		want        bool
	}{
		{"FVTPL", true},
		{"FVOCI_ELECTION", true},
		{"FVOCI", false},
		{"AC", false},
		{"", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.klasifikasi, func(t *testing.T) {
			t.Parallel()
			inst := &InstrumenSnapshot{KlasifikasiPsak71: tc.klasifikasi}
			if got := inst.IsFVTPL(); got != tc.want {
				t.Errorf("IsFVTPL(%q): want %v, got %v", tc.klasifikasi, tc.want, got)
			}
		})
	}
}

// Verify IsReksadana helper.
func TestInstrumenSnapshot_IsReksadana(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tipe string
		want bool
	}{
		{"REKSADANA", true},
		{"OBLIGASI", false},
		{"DEPOSITO", false},
		{"CASH", false},
		{"SAHAM", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.tipe, func(t *testing.T) {
			t.Parallel()
			inst := &InstrumenSnapshot{TipeInstrumen: tc.tipe}
			if got := inst.IsReksadana(); got != tc.want {
				t.Errorf("IsReksadana(%q): want %v, got %v", tc.tipe, tc.want, got)
			}
		})
	}
}

// Verify IsLPS helper.
func TestInstrumenSnapshot_IsLPS(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tipe string
		want bool
	}{
		{"DEPOSITO", true},
		{"CASH", true},
		{"OBLIGASI", false},
		{"REKSADANA", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.tipe, func(t *testing.T) {
			t.Parallel()
			inst := &InstrumenSnapshot{TipeInstrumen: tc.tipe}
			if got := inst.IsLPS(); got != tc.want {
				t.Errorf("IsLPS(%q): want %v, got %v", tc.tipe, tc.want, got)
			}
		})
	}
}

// ─── F2 fix: POCI routing tests (DEC-POCI-001) ───────────────────────────────

// TestDetermineRouting_POCIWithCAEIR_ReturnsComputed verifies F2 fix:
// when FlagPOCI=true AND HasCAEIRSchedule=true → RoutingPOCIComputed.
// CA-EIR present means baseline ECL can be computed (Phase 4.5, DEC-POCI-001).
func TestDetermineRouting_POCIWithCAEIR_ReturnsComputed(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "OBLIGASI",
		FlagPOCI:          true,
		HasCAEIRSchedule:  true, // CA-EIR schedule exists
	}
	got := DetermineRouting(inst)
	if got != RoutingPOCIComputed {
		t.Errorf("POCI with CA-EIR: want %s, got %s", RoutingPOCIComputed, got)
	}
}

// TestDetermineRouting_POCIWithoutCAEIR_ReturnsDeferred verifies F2 fix:
// when FlagPOCI=true AND HasCAEIRSchedule=false → RoutingPOCIDeferred.
// No CA-EIR schedule means computation deferred to Phase 5 (DEC-POCI-001).
func TestDetermineRouting_POCIWithoutCAEIR_ReturnsDeferred(t *testing.T) {
	t.Parallel()
	inst := &InstrumenSnapshot{
		KlasifikasiPsak71: "AC",
		TipeInstrumen:     "OBLIGASI",
		FlagPOCI:          true,
		HasCAEIRSchedule:  false, // no CA-EIR schedule yet
	}
	got := DetermineRouting(inst)
	if got != RoutingPOCIDeferred {
		t.Errorf("POCI without CA-EIR: want %s, got %s", RoutingPOCIDeferred, got)
	}
}
