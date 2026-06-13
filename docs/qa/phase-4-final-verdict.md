# Phase 4 — Final QA Verdict

**Date:** 2026-06-14
**QA Owner:** qa-engineer
**Compliance Gate:** ifrs9-compliance-reviewer
**Branch:** `feature/phase-4-m12-qa-suite`

---

## Module Verdict Table

| Module | Story | Test Coverage | Compliance Finding | Verdict |
|---|---|---|---|---|
| M1 — Staging Engine | APP-C-STG-001..005 | 80.2% | DEC-011 all 3 arms verified (F1 SICR IG→non-IG added) | **PASS** |
| M2 — PD/LGD/EAD Helpers | APP-C-ECL-M2-001..006 | 85.2% | POCI deferred flag present (F6) | **PASS** |
| M3 — LPS Aggregator | APP-C-ECL-M3-001..004 | 85.0% | IDR 2B cap, covered ECL=0, excess ECL>0 | **PASS** |
| M4 — Look-through Reksadana | APP-C-ECL-M4-001..004 | 85.0% | Breakdown reconcile, CASH/GOVT_BOND ECL=0 | **PASS** |
| M5 — EIR Amendment | APP-C-EIR-M5-001..005 | 85.3% | Immutable versioning, catch-up amount, 4-eyes SoD | **PASS** |
| M6 — Drift Cron | APP-C-EIR-M6-001..003 | 85.3% | HIGH drift detection, auto-proposal maker=SYSTEM | **PASS** |
| M7 — ECL Core Formula | APP-C-ECL-M7-001..006 | 85.3% | Stage 3 net carrying (F3 added), FVTPL skip (F2 added) | **PASS** |
| M8 — Calc Run Lifecycle | APP-C-ECL-M8-001..008 | 85.2% | Seal immutability, SoD, MFA step-up, audit hash | **PASS** |
| M11 — Roll-forward CKPN | APP-C-RF-M11-001..005 | 86.0% | Reconcile delta < IDR 1.0, XLSX export wired | **PASS** |
| M12 — E2E Integration | APP-C-E2E-M12-001..010 | (E2E) | All 7 scenarios + 3 cross-cutting pass | **PASS** |

---

## Compliance Findings Resolution

| ID | Severity | Finding | Resolution | Status |
|---|---|---|---|---|
| F1 | MAJOR | IG→non-IG SICR trigger not independently tested | `TestE2E_ScenarioA_SICR_IGToNonIG` added — seeds idBBB-→idBB+ (delta=1), asserts `TriggerIGToNonIG` fires | **RESOLVED** |
| F2 | MAJOR | No affirmative assertion that FVTPL produces zero ECL rows | `TestE2E_FVTPL_NoECLAssertion` added — seeds FVTPL instrument, asserts `findResultLine` returns nil | **RESOLVED** |
| F3 | MAJOR | Stage 3 net carrying interest base not E2E verified | `TestE2E_Stage3_NetCarryingInterestBase` added — calls `core.ComputeFormula` with Gross=2B, PriorECL=1.435B, asserts NetCarrying=565M | **RESOLVED** |
| F4 | MINOR | Coverage manifest and final verdict docs absent | `docs/qa/phase-4-coverage-report.md` and this file created | **RESOLVED** |
| F5 | MINOR | Handler-level idempotency integration test absent for calc-run endpoint | `TestIdempotency_CalcRunCreate_HandlerLevel` added to `internal/test/integration/idempotency_test.go` | **RESOLVED** |
| F6 | MINOR | POCI deferred flag not documented in E2E file header | Comment block added to `phase4_ecl_engine_test.go` header; GitHub issue filed | **RESOLVED** |

---

## Outstanding Items for Phase 5

The following items are out of scope for Phase 4 and are tracked as Phase 5 sprint 1 backlog:

1. **POCI full credit-adjusted EIR** (PSAK 71 §5.5.13, DEC-013 follow-up)
   - M2 helpers return `CodePOCIDeferredToM7`; M7 sets `RoutingPath=POCI_DEFERRED` with warning.
   - Full implementation requires credit-adjusted EIR from the Newton-Raphson solver at origination.
   - GitHub issue: `feat(app-c): Phase 5 — POCI full credit-adjusted EIR implementation (PSAK 71 §5.5.13)`.

2. **Jurnal P&L booking for EIR catch-up adjustment**
   - M5 computes `catchup_adjustment_idr`; APP-D integration to post `jrnl.jurnal_line` is Phase 5.

3. **Roll-forward FULL_LIFECYCLE_PHASE_5 detection method**
   - `DetectionMethod=BASIC_STATUS_DIFF` covers Phase 4. Full origination/derecognition from APP-B events is Phase 5.

4. **APP-B origination/derecognition → ECL engine integration**
   - `trx.transaction` INSERT events must feed `ecl.stage_history` and calc run scope.

5. **Gross carrying disclosure (proxy ead_idr)**
   - `gross_carrying_idr` = `ead_idr` in Phase 4 (proxy). True gross carrying requires APP-B post-accrual balance.

6. **SOFT_CLOSED periode eligibility for cure evaluation**
   - Production service filters `HARD_CLOSED` only. FSD-APP-C §3.3 permits SOFT_CLOSED. Verify in Phase 5.

---

## Build and Test Status at Gate

```
cd backend
go build ./...         → OK (0 errors)
go vet ./...           → OK (0 findings)
go test -race ./tests/... → OK (all scenarios pass)
golangci-lint run ./... → 0 findings
```

All Phase 4 compliance findings (F1–F6) resolved. Phase 5 items logged and tracked.
