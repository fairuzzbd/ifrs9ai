# P5 Final Compliance Review — VERDICT

**Reviewer**: ifrs9-compliance-reviewer
**Date**: 2026-06-25
**Scope**: Phase 5 M1-M18 (all regulated paths per PSAK 71 / IFRS 9)
**Branch**: feature/phase-5-m18-final-qa-uat
**Method**: compliance-checklist skill + code review + integration test verification

---

## VERDICT: BLOCK → CONDITIONAL-PASS (post-fix)

Phase 5 initially BLOCKED on 1 BLOCKER + 2 MAJOR + 1 LOW finding. After commit `733184a` resolved F#2 / F#3 / F#4, verdict moved to **CONDITIONAL-PASS** with the SPPI BLOCKER (F#1) explicitly deferred to dedicated modul M18b. Phase 5 cannot ship to production until M18b lands and compliance re-gate returns PASS on the SPPI/BM classification path.

The ECL formula (3-stage × 3-skenario × dual FL), EIR Newton-Raphson solver, SICR triggers, Cure logic, POCI handling, FVOCI Election no-recycling, LPS aggregator, look-through Reksadana, audit hash chain, 6-eyes klasifikasi, periode buku controls, and roll-forward CKPN reconciliation are all PASS. The single blocking gap is that production SPPI classification falls back to a stub returning `AC/PASS/HTC` for every instrument regardless of cashflow characteristics — meaning SPPI-fail instruments would be silently misclassified, and ECL would be computed for instruments that should be FVTPL.

---

## Findings

### HIGH (BLOCKER — deferred to M18b)

**F#1 — SPPI Q1-Q10 not implemented; stub returns AC/PASS/HTC for every instrument**
- **Standard**: PSAK 71 §4.1.2(b); FSD-APP-A §3 (SPPI Test Q1-Q10); DEC-010; DEC-017
- **File**: `backend/internal/master/bulkupload/sppi_bm_stub.go:20-31`; service fallback `backend/internal/master/bulkupload/service.go:68-71` (nil evaluator → stub)
- **Description**: `stubSPPIBMEvaluator.Evaluate()` hard-returns `KlasifikasiResult{KlasifikasiPsak71:"AC", SppiResult:"PASS", BmResult:"HTC", Ambiguous:false}` for every instrument. No SPPI question evaluated. BM not read from portfolio master. Production path defaults to stub because no real evaluator wired in `cmd/api/main.go`.
- **Consequences**:
  - (a) SPPI-fail instruments (convertibles, equity-linked, variable principal) silently classified AC instead of FVTPL → ECL computed unlawfully
  - (b) BM not per-portfolio — all instruments receive "HTC" regardless of portfolio's declared BM
  - (c) SPPI × BM classification matrix bypassed entirely
  - (d) No `sppi.test_result` persistence per instrument per assessment date
  - (e) No re-test trigger on amendment that modifies cashflow terms
- **Required fix (deferred to M18b)**: Implement `internal/sppi` package with real `SPPIBMEvaluator`: (a) evaluate all 10 SPPI questions per FSD-APP-A §3 per instrument; (b) route SPPI-fail to FVTPL (no ECL, no AC, no FVOCI debt); (c) read BM from `mst.portofolio`, not per-instrument; (d) persist test result per instrument per assessment date in `sppi.test_result`; (e) trigger re-test on amendment that modifies cashflow terms. Wire real evaluator in `cmd/api/main.go`. Remove or gate stub behind compile-time build tag.
- **Status**: DEFERRED → M18b dedicated modul. Compliance re-gate required after.

### MAJOR (resolved in 733184a)

**F#2 — M18 integration test EIR convergence check used float64**
- **Standard**: DEC-013 (Newton-Raphson, tolerance 1e-10, 8dp); DEC-016 (shopspring/decimal mandatory)
- **File**: `backend/tests/e2e/p5_m18_full_cycle_test.go` helpers `buildMonthlyDeposito` + `computeEIRNewtonRaphson`
- **Description**: Helpers built cashflows as `[]float64` and converted via `decimal.NewFromFloat(f)`. Local Newton-Raphson did not call production `eir.Solver.Solve`. Precision regression in production solver would not be detected.
- **Fix applied (733184a)**: `buildMonthlyDeposito` now takes `decimal.Decimal` args; `computeEIRNewtonRaphson` invokes production `eir.Solver.Solve` end-to-end. No float64 in EIR convergence verification path.
- **Status**: RESOLVED.

**F#3 — Stage 3 first-run interest accrued on Gross Carrying (priorSealedECL=nil branch)**
- **Standard**: PSAK 71 §5.4.1(b) — interest on Stage 3 = Net Carrying × EIR
- **File**: `backend/internal/ecl/core/formula.go` (Stage 3 + priorSealedECL=nil branch)
- **Description**: When instrument first entered Stage 3, no prior sealed ECL existed; `netCarrying = EAD` (gross), overstating interest income.
- **Fix applied (733184a)**: Two-pass pattern in `ecl/core/service.go` — first pass computes ECL per instrument, second pass uses in-run ECL as `netCarrying = EAD − ECL` for Stage 3 interest line.
- **Status**: RESOLVED.

### LOW (resolved in 733184a)

**F#4 — MTM DeviationWarning struct used float64 for percentage fields**
- **Standard**: DEC-016 (shopspring/decimal mandatory for rates/percentages)
- **File**: `backend/internal/trx/mtm/domain.go:400-410`
- **Description**: `DeltaPct` and `ThresholdPct` were `float64`. Display-only metadata, not feeding ECL/EIR/jurnal, but violated DEC-016 blanket rule.
- **Fix applied (733184a)**: Migrated both fields to `decimal.Decimal`; callers updated.
- **Status**: RESOLVED.

---

## Path-by-path checklist

| # | Path | Standard | Verdict | Note |
|---|---|---|---|---|
| 1 | ECL formula 3-stage × 3-scenario × dual FL | DEC-010 / SoW §4 | PASS | Default weights 0.25/0.50/0.25 in BobotSnapshot; ALCO override via calc-run param snapshot |
| 2 | EIR Newton-Raphson 1e-10 / 100 iter / 8dp | DEC-013 | PASS | shopspring/decimal end-to-end; divergence + non-convergence return explicit errors |
| 3 | SICR triggers (rating ≥ 2 notch / IG→non-IG / DPD ≥ 30) | DEC-011 | PASS | All 3 triggers in `staging/service.go` |
| 4 | Cure (3 months consecutive HARD_CLOSED) | DEC-012 | PASS | `staging/adapters.go` queries `status='HARD_CLOSED'` only |
| 5 | Stage 3 interest on Net Carrying | PSAK 71 §5.4.1(b) | PASS (post-fix) | F#3 fixed via two-pass; first-run no longer uses Gross |
| 6 | POCI credit-adjusted EIR, no Stage 1 | PSAK 71 §5.5.13 | PASS | `routing.go` routes POCI before LPS; direct P&L ECL movement |
| 7 | FVOCI Election irrevocable, no P&L recycling | PSAK 71 §B5.7.1 | PASS | `penjualan/routing.go` NoRecyclingFlag=true; M18 test asserts no GL 4201 posting |
| 8 | FVOCI debt OCI accumulation + recycling | PSAK 71 §5.7.10 | PASS | RecycleOCI=true; event codes include REKLAS_OCI_PL |
| 9 | LPS aggregator IDR 2B cap, ECL on excess | DEC-014 | PASS | `lps/service.go` cap parameterized; covered ECL=0; SoD on exclusion override |
| 10 | Look-through ECL Reksadana | DEC-015 | PASS | `lookthrough/service.go` decompose + 3-scenario × FL per class; Σ ECL_class == totalECL |
| 11 | Roll-forward CKPN reconciliation | SoW §4 | PASS | M18 test verifies opening + transfers + originations − derecognitions ± remeasurements = closing at 4dp |
| 12 | Jurnal IFRS9 transitions (AC↔FVOCI, FVOCI equity disposal) | SoW §5 | PASS | Event codes mapped correctly; jurnal balanced |
| 13 | Audit hash chain SHA-256 (prev_hash ‖ canonical_json) | DEC-018 | PASS | `audit/verify.go` constant-time compare; M18 test verifies 10 mutations × 9 schemas |
| 14 | 6-eyes klasifikasi PSAK 71 (Maker→Risk→Komite + MFA) | DEC-017 / DEC-027 | PASS | Chain enforced; step-up MFA on Komite approve |
| 15 | shopspring/decimal in money/rate paths | DEC-016 | PASS (post-fix) | F#4 MTM percentage fields migrated; ECL/EIR paths zero float64 |
| 16 | Periode buku hard-close irreversible + step-up MFA | DEC-027 | PASS | OPEN→SOFT→HARD irreversible; reopen after HARD forbidden; PERIODE_CLOSED 423 enforced |
| 17 | SPPI Q1-Q10 evaluation | PSAK 71 §4.1.2(b) | **BLOCK** | F#1 SPPI stub — DEFERRED to M18b |

---

## Carryover to M18b

Implement `internal/sppi` package per FSD-APP-A §3:
- 10-question evaluator with versioned question schema
- Per-instrument test result persistence in `sppi.test_result` (audit-tracked)
- Per-portfolio BM lookup from `mst.portofolio`
- Re-test trigger on amendment that mutates cashflow terms
- Wire real evaluator into `cmd/api/main.go` bulkupload service constructor
- Remove `sppi_bm_stub.go` or guard behind `//go:build test` tag
- Compliance re-gate must return PASS before Phase 5 production ship

---

## Sign-off

- **Reviewer**: ifrs9-compliance-reviewer
- **Date**: 2026-06-25
- **Post-fix verdict**: CONDITIONAL-PASS pending M18b
- **Audit trail**: M18 PR contains all 4 finding remediations + this verdict report
