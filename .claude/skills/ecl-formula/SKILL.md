---
name: ecl-formula
description: Reference implementation pattern untuk ECL calculation (3-stage × 3-skenario × dual FL), termasuk LPS aggregator dan look-through Reksadana. Gunakan saat menulis atau mereview kode ECL agar formula konsisten dengan FSD-APP-C dan SoW.
---

# ECL Formula — Reference Implementation

## Core formula
```
ECL_skenario      = EAD × PD_skenario × LGD                       (1)
ECL_FL_skenario   = ECL_skenario × Impact_PD_multiplier_skenario   (2)
ECL_weighted      = Σ (ECL_FL_skenario × bobot_skenario)           (3)
```
Default bobot: Good 0.25 / Normal 0.50 / Bad 0.25 (sum = 1.0, ALCO override allowed).

## Staging-aware PD
| Stage | PD source |
|---|---|
| 1 | `pd_curve` where `tenor_bucket = '12M'` |
| 2 | `pd_curve` where `tenor_bucket = 'lifetime'` (sum until maturity) |
| 3 | `1.00000000` (fixed, 8-decimal) |

## Skenario PD
PD per skenario diturunkan dari **macro stress applied to base PD**:
```
PD_Good    = PD_base × stress_factor_Good     (typically 0.5–0.8)
PD_Normal  = PD_base × 1.0
PD_Bad     = PD_base × stress_factor_Bad      (typically 1.5–3.0)
```
Stress factor dari ALCO-approved scenario table.

## Implementation pattern (Go, `shopspring/decimal`)

```go
package ecl

import "github.com/shopspring/decimal"

type CalcInput struct {
    EAD               decimal.Decimal  // IDR
    PDByScenario      map[Scenario]decimal.Decimal
    LGD               decimal.Decimal
    FLMultiplier      map[Scenario]decimal.Decimal
    ScenarioWeights   map[Scenario]decimal.Decimal // sum must = 1
    Stage             Stage
}

type CalcResultLine struct {
    EADUsed       decimal.Decimal
    LGDUsed       decimal.Decimal
    PerScenario   map[Scenario]ScenarioResult
    ECLWeighted   decimal.Decimal
    FormulaVersion string
}

type ScenarioResult struct {
    PD            decimal.Decimal
    FLMultiplier  decimal.Decimal
    ECLRaw        decimal.Decimal  // (1)
    ECLWithFL     decimal.Decimal  // (2)
    Weight        decimal.Decimal
    Contribution  decimal.Decimal  // ECLWithFL × Weight
}

func ComputeECL(in CalcInput) (CalcResultLine, error) {
    if err := validateInput(in); err != nil {
        return CalcResultLine{}, err
    }

    result := CalcResultLine{
        EADUsed:        in.EAD,
        LGDUsed:        in.LGD,
        PerScenario:    map[Scenario]ScenarioResult{},
        FormulaVersion: "v1.0",
    }

    weighted := decimal.Zero
    for sc, pd := range in.PDByScenario {
        fl := in.FLMultiplier[sc]
        w  := in.ScenarioWeights[sc]

        eclRaw    := in.EAD.Mul(pd).Mul(in.LGD)                  // (1)
        eclWithFL := eclRaw.Mul(fl)                              // (2)
        contrib   := eclWithFL.Mul(w)                            // (3 per-term)
        weighted   = weighted.Add(contrib)

        result.PerScenario[sc] = ScenarioResult{
            PD: pd, FLMultiplier: fl, Weight: w,
            ECLRaw: eclRaw, ECLWithFL: eclWithFL, Contribution: contrib,
        }
    }

    // Round result ke presisi storage (NUMERIC(20,4))
    result.ECLWeighted = weighted.Round(4)
    return result, nil
}

func validateInput(in CalcInput) error {
    sum := decimal.Zero
    for _, w := range in.ScenarioWeights {
        sum = sum.Add(w)
    }
    if !sum.Equal(decimal.NewFromInt(1)) {
        return fmt.Errorf("scenario weights must sum to 1.0, got %s", sum)
    }
    if in.LGD.LessThan(decimal.Zero) || in.LGD.GreaterThan(decimal.NewFromInt(1)) {
        return fmt.Errorf("LGD out of range [0,1]: %s", in.LGD)
    }
    // ... PD bounds, EAD non-negative, etc.
    return nil
}
```

## Snapshot pattern (audit-grade reproducibility)

Setiap calc run **FREEZE** semua input ke `ecl.ecl_calc_input_snapshot`:
```sql
CREATE TABLE ecl.ecl_calc_input_snapshot (
  run_id           UUID PRIMARY KEY,
  instrument_id    UUID NOT NULL,
  ead              NUMERIC(20,4) NOT NULL,
  pd_good          NUMERIC(10,8) NOT NULL,
  pd_normal        NUMERIC(10,8) NOT NULL,
  pd_bad           NUMERIC(10,8) NOT NULL,
  lgd              NUMERIC(10,8) NOT NULL,
  fl_mult_good     NUMERIC(10,8) NOT NULL,
  fl_mult_normal   NUMERIC(10,8) NOT NULL,
  fl_mult_bad      NUMERIC(10,8) NOT NULL,
  weight_good      NUMERIC(10,8) NOT NULL,
  weight_normal    NUMERIC(10,8) NOT NULL,
  weight_bad       NUMERIC(10,8) NOT NULL,
  stage            SMALLINT NOT NULL,  -- 1/2/3
  formula_version  TEXT NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Re-run dengan snapshot yang sama harus produce **identical** result (bit-for-bit, sampai decimal terakhir). Test-it: re-run vs original `WHERE NOT EQUAL` returns zero rows.

## LPS Aggregator (untuk Cash + Deposito only)
```go
func ApplyLPSAggregator(positions []Position) []Position {
    // group by (nasabah, bank)
    grouped := groupBy(positions, func(p Position) Key {
        return Key{Nasabah: p.NasabahID, Bank: p.BankID}
    })

    out := []Position{}
    for k, group := range grouped {
        total := sumExposure(group)
        cap   := decimal.NewFromInt(2_000_000_000) // IDR 2 miliar
        if total.LessThanOrEqual(cap) {
            // fully covered, no ECL
            for _, p := range group {
                p.ECLApplicableEAD = decimal.Zero
                out = append(out, p)
            }
        } else {
            // proportional allocation of "excess" across positions
            excess := total.Sub(cap)
            for _, p := range group {
                share := p.EAD.Div(total)            // share of total
                p.ECLApplicableEAD = excess.Mul(share)
                out = append(out, p)
            }
        }
    }
    return out
}
```

## Look-through Reksadana
```go
func ComputeECLReksadana(fund Reksadana) decimal.Decimal {
    total := decimal.Zero
    for _, underlying := range fund.Komposisi {
        // underlying.PctAlokasi: 0..1
        eadShare := fund.NAB.Mul(underlying.PctAlokasi)
        pd, lgd  := lookupParamsByAssetClass(underlying.AssetClass)
        ecl, _   := ComputeECL(CalcInput{
            EAD: eadShare, PDByScenario: pd, LGD: lgd,
            FLMultiplier: defaultFL, ScenarioWeights: defaultWeights,
            Stage: fund.Stage,
        })
        total = total.Add(ecl.ECLWeighted)
    }
    return total
}
```

## Test cases wajib
1. Simple AC, Stage 1, 1 currency, single scenario weight = 1.0 → ECL = EAD×PD×LGD
2. Stage 2 dengan dual FL, 3 skenario → cross-check manual
3. Stage 3 → PD = 1.0, ECL = EAD × LGD (PD=1, no FL needed but apply for consistency)
4. SPPI fail → upstream skip ECL entirely (test via classifier)
5. LPS: 1 nasabah, 3 bank, total < cap → all ECL = 0
6. LPS: 1 nasabah, 1 bank, total > cap → excess di-distribute pro-rata
7. Reksadana look-through: 60% saham (no ECL) + 40% obligasi → ECL hanya dari obligasi
8. Snapshot reproducibility: re-run same snapshot → identical result
9. Weight sum ≠ 1.0 → reject input
10. Invalid LGD > 1.0 → reject input

## Citation
- SoW_v1.4.docx §4 (ECL formula breakdown)
- FSD-APP-C-ECL-EIR-v1.0.docx §3 (staging) + §4 (FL multiplier methodology)
- Pefindo_Annual_Default_Study_2007-2025_EN.pdf (PD kalibrasi)
- Decision Log DEC-010, DEC-014, DEC-015
