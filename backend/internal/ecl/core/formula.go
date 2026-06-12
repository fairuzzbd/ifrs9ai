package core

import (
	"github.com/shopspring/decimal"
)

// formula.go — canonical ECL formula per SoW §4, FSD-APP-C §3, DEC-010.
//
// Formula:
//
//	ECL_skenario     = EAD_IDR × PD_skenario × LGD
//	ECL_FL_skenario  = ECL_skenario × (impact_pd[skenario] × impact_mev_pd[skenario])
//	                   (Stage 3: FL NOT applied — PD fixed 1.0)
//	ECL_weighted     = Σ(ECL_FL_skenario × bobot_skenario)
//
// Precision (DEC-016):
//   - IDR amounts: RoundBank(4) at each step
//   - PD/LGD/FL: RoundBank(8) at each step
//   - Bobot: RoundBank(4) at snapshot validation; full precision during multiplication
//
// No float64 used anywhere. All arithmetic uses shopspring/decimal.Decimal.
//
// Stage 3 override rule (state-machine doc §2, FSD-APP-C §3.1):
//   - PD for all scenarios = decimal.NewFromString("1.00000000") (hard-coded, invariant).
//   - FL multiplier is NOT applied (set to nil / ignored).
//   - ECL_FL_skenario = ECL_skenario (no multiplier applied).
//
// FL multiplier source (F1 fix, DEC-010): combined multiplier = impact_pd × impact_mev_pd.
// M2 PDLookupService exposes both PDDetail.ImpactPDMultiplier and PDDetail.ImpactMevPDMultiplier
// separately. M7 is the authoritative computation layer: it receives PDDetail.PDBase (pre-FL)
// and combines both multipliers here: combined_fl = ImpactPDMultiplier × ImpactMevPDMultiplier.
// Using only ImpactMevPDMultiplier (the prior bug) silently understated ECL by the impact_pd factor.
// See: docs/state-machines/p4-m7-ecl-core.md §5.4 (OQ-M7-4 resolution, F1 compliance fix).

// pdOne is the fixed PD for Stage 3 instruments.
// Stage 3: PD = 1.00000000 (exactly). FL multiplier NOT applied (DEC-010).
var pdOne = decimal.NewFromInt(1)

// ComputeScenarioECL computes one scenario's ECL value.
//
//	ECL_skenario = EAD_IDR × PD_skenario × LGD
//
// Result is rounded HALF_EVEN to 4 decimal places (IDR precision per DEC-016).
// No float64.
func ComputeScenarioECL(ead, pd, lgd decimal.Decimal) decimal.Decimal {
	// EAD × PD × LGD  — rounding at output, not intermediate, to avoid
	// accumulated rounding error across the three steps.
	// Per formulas.md: "Rounding: HALF_EVEN (banker's rounding) per spec".
	return ead.Mul(pd).Mul(lgd).RoundBank(4)
}

// ApplyFLMultiplier applies the forward-looking (MEV) multiplier to an ECL scenario value.
//
//	ECL_FL_skenario = ECL_skenario × fl_multiplier
//
// Result is rounded HALF_EVEN to 4 decimal places.
// For Stage 3: this function must NOT be called — use ECL_skenario directly.
func ApplyFLMultiplier(eclScenario, flMultiplier decimal.Decimal) decimal.Decimal {
	return eclScenario.Mul(flMultiplier).RoundBank(4)
}

// ComputeWeightedECL computes the weighted ECL sum.
//
//	ECL_weighted = ECL_FL_good × bobot_good + ECL_FL_normal × bobot_normal + ECL_FL_bad × bobot_bad
//
// Result is rounded HALF_EVEN to 4 decimal places (IDR precision per DEC-016).
// Bobot values are stored as NUMERIC(7,4) snapshots but multiplication uses full precision.
func ComputeWeightedECL(eclFLGood, eclFLNormal, eclFLBad decimal.Decimal, bobot BobotSnapshot) decimal.Decimal {
	// Σ(ECL_FL_skenario × bobot_skenario) — all in one expression to minimize rounding.
	// Round final result to 4 decimal places (IDR NUMERIC(20,4)).
	return eclFLGood.Mul(bobot.Good).
		Add(eclFLNormal.Mul(bobot.Normal)).
		Add(eclFLBad.Mul(bobot.Bad)).
		RoundBank(4)
}

// FormulaResult holds all intermediate and final values for one instrument's ECL.
// All IDR values are NUMERIC(20,4). PD/LGD/FL are NUMERIC(10,8).
type FormulaResult struct {
	// Inputs
	EADIDR   decimal.Decimal
	PDGood   decimal.Decimal // NUMERIC(10,8) — base PD (before FL for non-Stage3)
	PDNormal decimal.Decimal
	PDBad    decimal.Decimal
	LGD      decimal.Decimal // NUMERIC(10,8)

	// FL multipliers (nil for Stage 3)
	FLGood   *decimal.Decimal // NUMERIC(10,8)
	FLNormal *decimal.Decimal
	FLBad    *decimal.Decimal

	// ECL pre-FL per scenario — NUMERIC(20,4)
	ECLGoodIDR   decimal.Decimal
	ECLNormalIDR decimal.Decimal
	ECLBadIDR    decimal.Decimal

	// ECL post-FL per scenario — NUMERIC(20,4)
	ECLFLGoodIDR   decimal.Decimal
	ECLFLNormalIDR decimal.Decimal
	ECLFLBadIDR    decimal.Decimal

	// Weighted ECL — NUMERIC(20,4)
	ECLWeightedIDR decimal.Decimal

	// Stage 3 net carrying
	NetCarryingIDR    *decimal.Decimal
	PriorSealedECLIDR *decimal.Decimal

	// Bobot snapshot used
	Bobot BobotSnapshot

	// IsStage3 indicates the Stage 3 override was applied.
	IsStage3 bool
}

// ComputeFormula applies the full ECL formula for one instrument.
//
// Parameters:
//   - ead: EAD_IDR (NUMERIC(20,4))
//   - pdGood, pdNormal, pdBad: PD per scenario (NUMERIC(10,8)) — base PD, NOT FL-adjusted.
//     M7 owns the FL multiplication to avoid double-apply from M2 (OQ-M7-4).
//   - lgd: LGD (NUMERIC(10,8))
//   - flGood, flNormal, flBad: FL multiplier per scenario (NUMERIC(10,8)).
//     Stage 3: pass nil, nil, nil — function will skip FL application.
//   - bobot: active bobot snapshot (must sum to 1.0 ± 1e-8).
//   - priorSealedECL: latest sealed ECL for Stage 3 net carrying (nil = first run).
//   - stage: Stage 1, 2, or 3.
//
// Returns FormulaResult with all intermediates populated.
// No float64 used.
func ComputeFormula(
	ead decimal.Decimal,
	pdGood, pdNormal, pdBad decimal.Decimal,
	lgd decimal.Decimal,
	flGood, flNormal, flBad *decimal.Decimal,
	bobot BobotSnapshot,
	priorSealedECL *decimal.Decimal,
	stage Stage,
) FormulaResult {
	r := FormulaResult{
		EADIDR:   ead,
		PDGood:   pdGood.RoundBank(8),
		PDNormal: pdNormal.RoundBank(8),
		PDBad:    pdBad.RoundBank(8),
		LGD:      lgd.RoundBank(8),
		Bobot:    bobot,
		IsStage3: stage == Stage3,
	}

	// Stage 3 override: PD = 1.00000000 for ALL scenarios.
	// FL multiplier is NOT applied (PSAK 71 / IFRS 9 §5.5.17).
	// Per DEC-010, state-machine doc §2, FSD-APP-C §3.1.
	if stage == Stage3 {
		r.PDGood = pdOne.RoundBank(8)
		r.PDNormal = pdOne.RoundBank(8)
		r.PDBad = pdOne.RoundBank(8)
		r.FLGood = nil
		r.FLNormal = nil
		r.FLBad = nil
	} else {
		r.FLGood = flGood
		r.FLNormal = flNormal
		r.FLBad = flBad
	}

	// Step 1: ECL_skenario = EAD × PD × LGD (per scenario)
	r.ECLGoodIDR = ComputeScenarioECL(ead, r.PDGood, r.LGD)
	r.ECLNormalIDR = ComputeScenarioECL(ead, r.PDNormal, r.LGD)
	r.ECLBadIDR = ComputeScenarioECL(ead, r.PDBad, r.LGD)

	// Step 2: ECL_FL_skenario = ECL_skenario × FL_multiplier
	// Stage 3: ECL_FL = ECL (no multiplier applied).
	if stage == Stage3 {
		r.ECLFLGoodIDR = r.ECLGoodIDR
		r.ECLFLNormalIDR = r.ECLNormalIDR
		r.ECLFLBadIDR = r.ECLBadIDR
	} else {
		if flGood != nil {
			r.ECLFLGoodIDR = ApplyFLMultiplier(r.ECLGoodIDR, *flGood)
		} else {
			r.ECLFLGoodIDR = r.ECLGoodIDR
		}
		if flNormal != nil {
			r.ECLFLNormalIDR = ApplyFLMultiplier(r.ECLNormalIDR, *flNormal)
		} else {
			r.ECLFLNormalIDR = r.ECLNormalIDR
		}
		if flBad != nil {
			r.ECLFLBadIDR = ApplyFLMultiplier(r.ECLBadIDR, *flBad)
		} else {
			r.ECLFLBadIDR = r.ECLBadIDR
		}
	}

	// Step 3: ECL_weighted = Σ(ECL_FL × bobot)
	r.ECLWeightedIDR = ComputeWeightedECL(r.ECLFLGoodIDR, r.ECLFLNormalIDR, r.ECLFLBadIDR, bobot)

	// Step 4: Stage 3 net carrying = EAD − prior_sealed_ecl.
	// Per OQ-M7-3 (BLOCKING): source = MAX(ecl_weighted_idr WHERE sealed_at IS NOT NULL
	// ORDER BY evaluation_date DESC LIMIT 1). First run: net_carrying = ead (ECL allowance = 0).
	if stage == Stage3 {
		r.PriorSealedECLIDR = priorSealedECL
		if priorSealedECL != nil {
			netCarrying := ead.Sub(*priorSealedECL).RoundBank(4)
			r.NetCarryingIDR = &netCarrying
		} else {
			// First run: net carrying = EAD (no prior sealed ECL).
			netCarrying := ead.RoundBank(4)
			r.NetCarryingIDR = &netCarrying
		}
	}

	return r
}
