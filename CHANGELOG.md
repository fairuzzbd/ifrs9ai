# Changelog

All notable changes to BLIPS IFRS9 are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/) and project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.0.0] - 2026-06-14

First production-ready release. Delivers complete Phase 3 (Master Data) + Phase 4 (ECL Engine + EIR) per PSAK 71 / IFRS 9.

### Added

#### Phase 3 — Master Data + SPPI + Business Model (APP-A, 12/12 modules)

- (app-a) `lps_coverage` parameter master with 4-eyes workflow + Idempotency-Key
- (app-a) `bobot_skenario` ALCO-approved Good/Normal/Bad weights (default 0.25/0.50/0.25)
- (app-a) `pd_pefindo` PD curve calibration tables with rating × tenor lookup
- (app-a) `impact_mev_pd` + `impact_pd` forward-looking multipliers (6-eyes)
- (app-a) `counterparty` + `rating_history` with column-level PII encryption (NPWP/no.rek/KTP via `sec.encrypt`)
- (app-a) `chart_of_accounts` CRUD + async XLSX import
- (app-a) `mapping_jurnal` header + detail with debit=kredit invariant DB CHECK
- (app-a) `portofolio` CRUD + 4-eyes
- (app-a) `instrumen` master CRUD
- (app-a) `kurs` BI JISDOR FX rate cache + 4-eyes
- (app-a) `lgd_basel` Basel-style LGD pool master
- (app-a) `mata_uang` schema + workflow

#### Phase 4 — ECL Engine + EIR (APP-C, 12/12 modules)

- (app-c) **M1 Staging Engine** — Stage 1/2/3 + SICR triggers (≥2 notch, IG→non-IG, DPD≥30) + cure 3 monthly periodes + 6-eyes manual override (DEC-011, DEC-012)
- (app-c) **M2 PD/LGD/EAD/CCF helpers** — stage-aware PD lookup with FL multiplier per scenario, NUMERIC(10,8) precision, bulk lookup ≤500ms/1000 instruments
- (app-c) **M3 LPS Aggregator** — IDR 2 miliar cap per nasabah per bank with FIFO covered allocation, ECL on excess only, 4-eyes exclusion override (DEC-014)
- (app-c) **M4 Look-through Reksadana** — fund composition 6-eyes workflow, ECL decomposition per asset class (GOVT_BOND, CORP_BOND, CASH, EQUITY, OTHER), weight sum 100%±0.01% (DEC-015)
- (app-c) **M5 EIR Newton-Raphson solver** — tolerance 1e-10, max 100 iter, 8 decimal precision, amortisasi schedule immutable (append-only), amendment 4-eyes (DEC-013)
- (app-c) **M6 Amendment Lifecycle** — document-upload detection, Asynq cron daily drift detection (02:00 WIB), auto-amendment for HIGH drift threshold, cancel pre-approval
- (app-c) **M7 ECL Core orchestrator** — 3-stage × 3-skenario × dual FL multiplier formula, routing per instrument type (FVTPL skip / REKSADANA→M4 / CASH+DEPOSITO→M3 / POCI / STANDARD), Stage 3 PD=1.0 + Net Carrying interest (PSAK 71 §5.4.1(b)), `ecl.calc_result_line` partitioned (DEC-010)
- (app-c) **M8 Calc Run + Seal** — 4-eyes seal workflow (RISK request → ALCO approve + step-up MFA), parameter snapshot freeze on /start, signature_hash SHA-256, DB trigger `fn_ecl_calc_run_no_modify_when_sealed`
- (app-c) **M9 EIR + Staging UI** — 11 screens (staging dashboard, override workflow, DPD entry, EIR + amortisasi schedule, amendment queue, drift report)
- (app-c) **M10 ECL Run UI** — 5 screens (calc run dashboard, detail with SoD-gated actions, per-instrument drill-down, portfolio summary with Recharts, roll-forward report)
- (app-c) **M11 Roll-forward CKPN** — full reconcile (opening + transfers + originations − derecognitions ± remeasurements = closing), PSAK 71 §5.5 disclosure XLSX 3-sheet export via excelize, Asynq async dispatch >1000 instruments
- (app-c) **M12 QA E2E suite** — 11 E2E scenarios covering DEC-010..018 + DEC-026/027, 5 UAT scripts (Bahasa Indonesia) for Treasury/Risk/Akuntansi/ALCO/CFO sign-off, performance benchmarks (1000 instruments ≤ 5s dev / 30s CI)

#### Phase 4.5 — POCI Credit-Adjusted EIR (partial)

- (app-c) **POCI** Newton-Raphson `SolveCreditAdjusted` for PD-adjusted cashflows, M7 routing `POCI_COMPUTED` when CA-EIR schedule present, Stage 1 forced to Stage 2 per PSAK 71 §5.5.13 (DEC-POCI-001..004)

#### Infrastructure

- (db) 32 migrations covering all 9 schemas (`mst`, `trx`, `ecl`, `sppi`, `doc`, `jrnl`, `aud`, `sec`, `sys`) + `rpt` materialized view scaffolding
- (api) 40+ OpenAPI 3.0 contracts with state machines + error catalogs (200+ stable error codes)
- (web) Next.js 14 App Router with shadcn/ui, 20+ blips components (DataTable, MFAStepUpModal, JobProgressPanel, StageBadge, RoutingPathBadge, JSONBTreeView, RollForwardWaterfall, ReconcileBadge, SealWorkflowPanel, etc.)
- (infra) Docker Compose dev + UAT stacks with Traefik, Keycloak, MinIO, Redis, PostgreSQL 18
- (ci) GitHub Actions CI with golangci-lint v1.64.5 (backend) + ESLint + Vitest (frontend); `deploy-uat` job stubbed with `[NEEDS-DEVOPS]` markers

### Fixed

- (app-c) M3 LPS — Asynq batch expiry job for APPROVED_ACTIVE → EXPIRED override rows
- (app-c) M3 LPS — `AggregateBulk` LIMIT+1 pre-query size check (avoids materializing 60k+ rows)
- (app-c) M4 look-through — SupersedeOld `effective_to = new.effective_from - 1 day` (eliminates 1-day overlap)
- (app-c) M5 EIR — implement IFRS9 §5.4.3 catch-up adjustment via NPV @ original EIR (jurnal P&L booking deferred Phase 5)
- (app-a) lps_coverage — ExportCSV audit write inside same transaction (DEC-018)
- (app-a) lps_coverage — `UpdateWorkflowStatusTx` adds `tenant_id` + `deleted_at IS NULL` guards
- (app-a) lps_coverage — handler returns persisted `deleted_at` not `time.Now()`
- (web) lps-coverage form — replace `parseFloat` with string-based money handling (DEC-016)
- (db) migration 000012 — add 0005 dependency to requires header + down migration precision guard

### Security

- (sec) DEC-027 step-up MFA enforced on M5 EIR amendment approve and M8 calc run seal (uses `claims.NeedsStepUp()` 5-min window, not static `mfa_verified` claim)
- (sec) SoD 4-eyes on M5 amendment + M8 seal — server-side enforcement plus DB CHECK `chk_ecl_calc_run_sod_seal` and `chk_eir_log_sod_complete` (all 3 axes: reviewer≠maker, approver≠maker, approver≠reviewer)
- (sec) Audit-in-tx mandatory across all mutations — constructor panic on nil `auditWriter`, audit failure aborts transaction (DEC-018)
- (sec) POCI guard precedence — PD service rejects POCI before Stage 3 in batch path (mirrors single path ordering)
- (db) `ecl.lookthrough_underlying` audit columns set NOT NULL with sentinel UUID backfill
- (db) migration 000026 — EIR amortization schedule immutability trigger `tg_eir_schedule_amounts_immutable` (only audit cols + `recomputed_from_seq` allowed to update)
- (db) migration 000028 — partial UNIQUE index on `(document_id, instrumen_id)` for amendment detection idempotency

### Compliance

- ifrs9-compliance-reviewer PASS verdicts on M1, M2, M3, M4, M5, M6, M7, M8, M11, M12 (10 modules)
- security-engineer PASS verdicts on M8 seal + M11 export
- DEC-010..018 compliance verified via 11 E2E scenarios + domain unit tests
- PSAK 71 §5.4.1(b) Stage 3 Net Carrying interest covered (UAT-002 + automated assertion)
- PSAK 71 §5.4.3 catch-up adjustment computed at amendment approval
- PSAK 71 §5.5 disclosure roll-forward XLSX 3-sheet (Movement Table + Gross Carrying + Sign-Off)
- PSAK 71 §5.5.13 POCI handled with credit-adjusted EIR routing (jurnal deferred Phase 5)
- PSAK 71 §5.5.15 FVTPL skip — affirmative no-ECL assertion in E2E suite

### Outstanding (Phase 5 backlog)

- POCI delta computation + jurnal P&L direct booking
- APP-B Transaction Lifecycle (origination, MTM, renewal, jatuh tempo) — required for roll-forward FULL_LIFECYCLE detection
- APP-D GL Host REST real-time integration (currently Phase 1 file batch deferred)
- APP-E Reporting & Dashboard (25+ reports)
- SOFT_CLOSED periode acceptance for cure (currently HARD_CLOSED only)
- Real UAT deploy infra wiring (registry credentials, `UAT_HOST`, `SSH_DEPLOY_KEY` secrets)

### Decision Log entries locked

- DEC-001..029 (tech stack, domain, architecture, security baselines)
- DEC-POCI-001..004 (Phase 4.5 POCI scope, Stage 1 prohibition)
- DEC-M6-001..002 (drift cron UTC location, auto-proposal maker UUID)

[Unreleased]: https://github.com/fairuzzbd/ifrs9ai/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/fairuzzbd/ifrs9ai/releases/tag/v1.0.0
