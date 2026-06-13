# POCI Phase 4.5 Decision Log

**Date**: 2026-06-14
**Author**: ecl-eir-engineer
**Status**: LOCKED
**Refs**: PSAK 71 §5.5.13, IFRS 9 B5.4.7, SoW v1.4 §4, FSD-APP-C-ECL-EIR-v1.0 §3-4

---

## DEC-POCI-001 — Phase 4.5 scope: CA-EIR computation + baseline ECL

**Decision**: Phase 4.5 (Issue #96 partial) delivers two things and nothing more:

1. `Solver.SolveCreditAdjusted(cashflows []CashflowItem, seed *decimal.Decimal)` — Newton-Raphson IRR solver
   that accepts PD-adjusted (expected) cashflows. Math identical to `Solve`. Sets `IsCreditAdjusted=true`
   and `Algorithm="NEWTON_RAPHSON_CREDIT_ADJUSTED"` in `SolveDetail`. Per PSAK 71 §5.5.13, the credit-adjusted
   EIR is computed at origination from expected cashflows already discounted for credit losses.

2. `ECLOrchestrator.handlePOCI` computes ECL via the standard PD × LGD × EAD formula (identical to Stage
   1/2 path), with `RoutingPath=POCI_COMPUTED` persisted. Two warnings are always appended:
   `POCI_CA_EIR_COMPUTED` (from EIR service) and `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA`.

**Rationale**: PSAK 71 §5.5.13 requires POCI instruments to use credit-adjusted EIR from day-0. The full
delta-ECL-since-origination computation requires the origination CA-EIR schedule to be retrievable — that
infrastructure (EIR schedule FK lookup + delta calculation) is deferred to Phase 5 to avoid blocking the
current sprint.

**Cannot reopen without**: superseding DEC-POCI-001 via RFC in `docs/decisions/RFC-poci-phase5.md` with
ALCO + CFO sign-off on the recompute plan.

---

## DEC-POCI-002 — Phase 5 deferred: delta ECL since origination + jurnal P&L

**Decision**: The following are explicitly OUT OF SCOPE for Phase 4.5 and deferred to Phase 5:

1. **Delta ECL** — PSAK 71 §5.5.13 states ECL for POCI = cumulative changes in lifetime ECL since
   origination. Phase 4.5 reports only the initial baseline ECL. The warning
   `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA` signals this to downstream consumers.

2. **Jurnal P&L integration** — Phase 5 will wire POCI ECL movement (delta) to `jrnl.posting` following
   PSAK 71 §5.5.14. The warning `POCI_CA_EIR_REQUIRES_FULL_CAEIR` (i.e. `WarnPOCIRequiresFullCAEIR`)
   signals that the P&L booking is not yet live.

3. **Stage transition for POCI** — POCI instruments do not transit Stage 1 (per PSAK 71 §5.5.13b). Stage
   guard for POCI is deferred to Phase 5 staging enforcement.

**Impact on reporting**: Any PSAK 71 ECL disclosure that includes POCI instruments MUST annotate the
limitation until Phase 5 is shipped. ROLE-RISK and ROLE-ALCO must be notified of this baseline-only
constraint during UAT.

---

## DEC-POCI-003 — CA-EIR computed at origination only; no re-estimation except for amendment

**Decision**: Credit-adjusted EIR for POCI instruments is fixed at origination. It changes only when:
- A contractual amendment occurs (same re-estimation rules as non-POCI: new `schedule_version`, immutable
  old rows, `effective_from`/`effective_to` versioning per FSD-APP-C §4.2).
- New PD-adjusted cashflow projections are submitted by ROLE-RISK and approved by ROLE-ALCO.

Ad-hoc re-computation of CA-EIR between reporting periods is disallowed. The CA-EIR at origination
determines the EIR used for interest revenue accrual across the instrument's life (PSAK 71 §5.4.1 /
IFRS 9 5.4.1(a)).

**Enforcement**: `eir/service.go` `Compute` checks `req.POCIMode` and routes to `SolveCreditAdjusted`.
The audit log entry includes `"is_credit_adjusted": true` + `"algorithm": "NEWTON_RAPHSON_CREDIT_ADJUSTED"`.
Sealed EIR schedule rows cannot be updated (existing immutability constraint on `ecl.amortisasi_schedule`).

---

## Summary table

| ID | Scope | Status | Phase |
|---|---|---|---|
| DEC-POCI-001 | CA-EIR solver + baseline ECL | LOCKED | 4.5 (done) |
| DEC-POCI-002 | Delta ECL + jurnal P&L | DEFERRED | 5 |
| DEC-POCI-003 | CA-EIR at origination, amendment versioning | LOCKED | 4.5 (done) |
