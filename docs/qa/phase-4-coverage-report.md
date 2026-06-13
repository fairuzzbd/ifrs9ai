# Phase 4 — ECL Engine Test Coverage Report

**Period:** Phase 4 (M1–M8, M11, M12)
**Generated:** 2026-06-14
**QA Owner:** qa-engineer

---

## Per-Package Coverage

| Package | Coverage | Milestone | Notes |
|---|---|---|---|
| `internal/ecl/staging` | 80.2% | M1 | SICR triggers, cure path, override workflow |
| `internal/ecl/helpers` | 85.2% | M2 | PD/LGD/EAD/CCF lookups, POCI detection |
| `internal/ecl/lps` | 85.0% | M3 | LPS aggregation, IDR 2B cap, excess ECL |
| `internal/ecl/lookthrough` | 85.0% | M4 | Reksadana decomposition, weighted ECL |
| `internal/ecl/eir` | 85.3% | M5/M6 | Newton-Raphson solver, amendment versioning, drift cron |
| `internal/ecl/core` | 85.3% | M7 | ECL formula orchestrator, routing paths, Stage 3 net carrying |
| `internal/ecl/calcrun` | 85.2% | M8 | Calc run lifecycle, seal workflow, SoD, immutability |
| `internal/ecl/rollforward` | 86.0% | M11 | Roll-forward CKPN, reconcile invariant, XLSX export |

---

## Aggregate Metrics

| Metric | Value |
|---|---|
| Total test functions (unit + integration + E2E) | ~420 |
| E2E scenarios (phase4_ecl_engine_test.go) | 10 |
| Integration test files | 20 |
| Lines of test code (LOC) | ~8 200 |
| Build tags covered | `(default)`, `integration` |

---

## Untested Edge Cases (flagged for Phase 5)

1. **POCI instruments** — `RoutingPath=POCI_DEFERRED`. Credit-adjusted EIR not yet computed. `ecl_weighted_idr=NULL` is stored but not consumed downstream. Full coverage requires Phase 5 Newton-Raphson extension.

2. **Jurnal P&L booking for EIR catch-up** — M5 computes the catch-up adjustment amount but the actual `jrnl` row creation is Phase 5 (APP-D integration). No end-to-end assertion from `eir_reestimation_log` → `jrnl.jurnal_line`.

3. **Roll-forward FULL_LIFECYCLE_PHASE_5** — `DetectionMethod=BASIC_STATUS_DIFF` is used in Phase 4. Origination/derecognition detection from APP-B transaction events is Phase 5. Detection method field is flagged in domain.go but not fully tested.

4. **APP-B origination / derecognition integration** — `trx.transaction` insert/update events are not yet feeding ECL engine state. Seeded as stubs in E2E harness.

5. **Gross carrying disclosure** — `gross_carrying_idr` is proxied to `ead_idr` in current Phase 4 implementation. True gross carrying (post-accrual, pre-impairment) requires APP-B integration in Phase 5.

6. **SOFT_CLOSED periode for cure evaluation** — `EvaluateCureTest` in E2E harness accepts any closed period list. Production service filters `mst.periode_buku WHERE status = 'HARD_CLOSED'`. SOFT_CLOSED periods should also be eligible per FSD-APP-C §3.3 — verify in Phase 5 integration test with real DB.

7. **Multi-currency EAD** — FX rate conversion `EAD_IDR = EAD_FCY × BI_JISDOR` is unit-tested but not wired in E2E bulk compute. Phase 5 will seed FCY instruments with JISDOR snapshots.

---

## Coverage Methodology

- Unit tests: `go test ./internal/ecl/...` (no build tag).
- Integration tests: `go test -tags integration ./internal/test/integration/...` against real PostgreSQL.
- E2E tests: `go test ./tests/e2e/...` — in-process service wiring with in-memory stubs.
- Coverage collected via: `go test -coverprofile=coverage.out ./internal/ecl/...`
- Reported with: `go tool cover -func=coverage.out`

Coverage percentages are from the last full run on `develop` (commit `bb7e278`).
