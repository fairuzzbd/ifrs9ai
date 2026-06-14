# PLAN — Phase 5: Transaction Lifecycle, GL Posting, Reporting & Dashboard

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-14
**Branch base**: `develop` (setelah v1.0.0 — Phase 3 master data + Phase 4 ECL Engine + POCI Phase 4.5 selesai)
**Sumber kebenaran**:
- `FSD-APP-B-TransactionLifecycle-v1.1.docx` — 9 modul APP-B
- `FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx` — Periode Buku, FX, Jurnal, GL Interface
- `FSD-APP-E-Reporting-v1.0.docx` — 28 reports + dashboard
- `BRD_BLIPS_IFRS9_v1.1.docx` — stakeholder intent + scope
- `SoW_v1.4.docx` §5, §10 — formula + field lists
- `ERD-BLIPS-IFRS9-v1.2.docx` — schema `trx.*`, `jrnl.*`, `rpt.*`
- `docs/decisions/poci-phase-4-5.md` — DEC-POCI-002 (POCI delta + jurnal P&L deferred ke Phase 5)
- `.claude/memory/locked-decisions.md` — DEC-001..029
- `.claude/memory/formulas.md` — ECL/EIR + jurnal formula reference

**Klasifikasi**: feature — multi-regulated path (ECL jurnal, periode close, POCI delta, staging SOFT_CLOSED cure)
**Target**: ~10–12 minggu dari kick-off Phase 5

---

## 1. Scope & Exclusions

### Dalam scope Phase 5

| Domain | Cakupan |
|---|---|
| **APP-B Penempatan** | Modul penempatan deposito/obligasi/saham/reksadana, CRUD + 4-eyes workflow, EIR auto-trigger post-approve, jurnal posting PENEMPATAN |
| **APP-B MTM** | MTM daily job (price feed apply), manual adjustment + bulk upload XLSX (FSD-APP-B §2 + §8), jurnal MTM_FVOCI / MTM_FVTPL |
| **APP-B Renewal** | Renewal deposito (POKOK_SAJA + POKOK_PLUS_BUNGA), 4-eyes, instrumen baru auto-create, EIR re-compute, jurnal |
| **APP-B Penjualan** | Penjualan/pencairan partial + full, OCI recycling (FVOCI debt), FVOCI Election no-recycling, derecognition, jurnal PENJUALAN |
| **APP-B Jatuh Tempo** | Maturity job harian 09:00, settlement, amortization closing-carrying verify, derecognition, jurnal JATUH_TEMPO |
| **APP-B Pendapatan** | Akrual harian (EIR-method + simple), dividen, distribusi reksadana, PPh, FX unrealized harian, jurnal AKRUAL_BUNGA |
| **APP-B Media Upload** | Document service full (sudah ada infra), entity_type extension ke semua TRX events, virus scan, MinIO, pre-signed URL |
| **APP-B Bulk Upload Instrumen** | Bulk onboarding XLSX (5 sheets CASH/DEP/OBL/SHM/RDN), 4-stages validation, DRY_RUN, commit pipeline, rollback |
| **APP-D Periode Buku close** | OPEN → SOFT_CLOSED (4-eyes Akuntansi + FinCon) + SOFT_CLOSED → CLOSED (CFO, step-up MFA), CLOSED → REOPEN (CEO sign), closing-checklist pre-conditions |
| **APP-D FX Rate** | BI JISDOR job harian 10:30, manual upload FX + Akuntansi Maker → FinCon Approver, locked_flag on CLOSED, FX gain/loss treatment |
| **APP-D Mapping Jurnal** | Master mapping jurnal header + detail CRUD, 6-eyes workflow (ROLE-AKUN Maker → ROLE-AKUN-CTL Approver + ROLE-RISK second-approver), runtime resolver, import/export XLSX |
| **APP-D GL Host REST** | Jurnal delivery ke GL Host via REST (queued via Asynq), retry + DLQ, daily reconciliation job, status dashboard |
| **APP-D POCI delta + jurnal P&L** | DEC-POCI-002: delta ECL since origination + jurnal P&L booking per PSAK 71 §5.5.14, Stage guard POCI (no Stage 1) enforcement |
| **APP-D Cure SOFT_CLOSED** | Cure evaluation pada status SOFT_CLOSED periode buku (per FSD-APP-B §1.4 BR-2) |
| **APP-E Materialized views** | 8 MVs: posisi_portofolio, ckpn_rollforward, stage_distribution, concentration_risk, amortization_summary, eir_summary, fx_position, ecl_summary_period |
| **APP-E 28 reports** | RPT-01..28 lengkap — query engine, export (CSV/XLSX/PDF), scheduled email distribution, RBAC per report |
| **APP-E Dashboards** | Treasury, Risk, Akuntansi, CFO/Direksi, Auditor — role-specific widgets, real-time refresh via SSE/polling |
| **UAT Deploy Infra** | Docker Compose UAT env finalization, K8s prod prep, Prometheus + Grafana + Loki full setup, runbooks |

### Exclusions (bukan Phase 5 — deferred ke Phase 6)

| Excluded | Alasan |
|---|---|
| GL Host real-time streaming (gRPC/WebSocket) | Phase 6 — DEC-005 menetapkan Phase 1 = file batch / REST async |
| Advanced ML drift detection (PD curve auto-refit) | Phase 6 — butuh data lake infra |
| External regulator API submission (OJK portal) | Phase 6 — regulatory API belum final |
| Multi-tenant (tenant_id active enforcement) | Phase 6 — DEC-023 single tenant Phase 1 |
| Mobile native apps | Out of scope seluruhnya |
| Dividen saham treatment otomatis dari corporate action feed | Phase 6 — feed integration belum ada vendor |

---

## 2. Modul Breakdown dengan Dependency Order

| ID | Title | App | Deliverables | Depends | Gate | Effort |
|---|---|---|---|---|---|---|
| **P5-M1** | trx.penempatan CRUD + 4-eyes workflow | APP-B | `internal/trx/penempatan` service+repo+handler, migration 000028 (`trx.penempatan`), workflow DRAFT→PENDING_APPROVAL→APPROVED, EIR auto-trigger post-approve (call ecl/eir), `POST /transaksi/penempatan/{id}/submit\|approve\|reject`, `GET /transaksi/penempatan/{id}/eir-preview`, dokumen wajib enforce | `mst.*` Phase 3, `ecl/eir` Phase 4, `doc.*` Phase 2 | **compliance BLOCKING** (jurnal trigger + EIR trigger) | L |
| **P5-M2** | Jurnal posting engine (Mapping Resolver + jrnl schema) | APP-D | `internal/jrnl/resolver` runtime algorithm, migration 000029 (`mst.mapping_jurnal_header`, `mst.mapping_jurnal_detail`, `jrnl.header`, `jrnl.detail`), balance-check enforcement, `GET/POST /mapping-jurnal/*`, `GET /jurnal/header/*`, audit `JURNAL.POST` | P5-M1 (consumes event PENEMPATAN) | **compliance BLOCKING** (jurnal mapping accuracy) + **security BLOCKING** (jrnl append-only) | L |
| **P5-M3** | GL Host REST delivery (Asynq queue + DLQ + reconciliation) | APP-D | `internal/jrnl/gldelivery` Asynq worker, `gl_host_status` lifecycle (PENDING→DELIVERED→FAILED→RETRYING), DLQ + alert, `POST /jurnal/header/{id}/retry-gl-delivery`, daily recon job + `GET /reconciliation/daily`, `sys.job` progress | P5-M2 | advisory (no ECL/EIR path) + **security** (audit delivery) | M |
| **P5-M4** | APP-D Periode Buku close workflow | APP-D | `internal/periode/close` service, state machine OPEN→SOFT_CLOSED→CLOSED, pre-condition checklist (0 PENDING_APPROVAL, jurnal balanced, recon pass), CFO step-up MFA untuk hard-close, `POST /periode-buku/{id}/soft-close-request\|soft-close-approve\|hard-close-request\|hard-close-approve\|reopen-request\|reopen-approve`, closing-checklist endpoint, `GET /reports/status-periode` | P5-M2 (jurnal balanced check), P5-M3 (GL status) | **compliance BLOCKING** (jurnal integrity) + **security BLOCKING** (step-up MFA hard-close) | M |
| **P5-M5** | FX Rate management + BI JISDOR job | APP-D | `internal/fx` service + Asynq cron (10:30 WIB), `mst.kurs` migration 000030, manual upload workflow Akuntansi→FinCon, locked_flag on CLOSED, FX gain/loss treatment routing (P&L vs OCI per instrumen klasifikasi), `GET /master/kurs/*`, `POST /master/kurs/upload`, `POST /master/kurs/jisdor-sync` | Phase 3 `mst.mata_uang`, P5-M4 (locked_flag trigger) | advisory | S |
| **P5-M6** | APP-B MTM daily job + manual upload XLSX | APP-B | `internal/trx/mtm` batch worker (18:00 WIB), STALE_PRICE handling, migration 000031 (`trx.mtm`), `sys.upload_batch` + `sys.upload_batch_row` for MTM (migration 000032), price-deviation WARNING flow, `POST /mtm/upload/batch`, override-per-row endpoint, STALE_PRICE alert, jurnal MTM_FVOCI/MTM_FVTPL/MTM_FVOCI_ELECTION via P5-M2 | P5-M1 (instrumen approved), P5-M2 (jurnal), P5-M5 (kurs) | **compliance BLOCKING** (OCI vs P&L routing per klasifikasi) | L |
| **P5-M7** | APP-B Renewal deposito | APP-B | `internal/trx/renewal` service, migration 000033 (`trx.renewal`), 2 skema (POKOK_SAJA/POKOK_PLUS_BUNGA), auto-create instrumen baru, EIR re-compute, PPh 20% calc, jurnal PENEMPATAN baru via P5-M2, `POST /transaksi/renewal`, 4-eyes workflow | P5-M1, P5-M2 | **compliance BLOCKING** (EIR re-compute + PPh accuracy) | M |
| **P5-M8** | APP-B Penjualan/Pencairan | APP-B | `internal/trx/penjualan` service, migration 000034 (`trx.penjualan`), partial + full disposal, OCI recycling (FVOCI debt), no-recycling (FVOCI Election), realized gain/loss, BM Test frequency check (>5% threshold), jurnal PENJUALAN + REKLAS_OCI_PL, derecognition flow, `POST /transaksi/penjualan`, 4-eyes | P5-M1, P5-M2, `ecl/staging` Phase 4 | **compliance BLOCKING** (OCI recycling + BM Test trigger) | L |
| **P5-M9** | APP-B Jatuh Tempo + Pendapatan Akrual harian | APP-B | `internal/trx/maturity` + `internal/trx/akrual` services, migration 000035 (`trx.jatuh_tempo`, `trx.pendapatan_akrual`), maturity job 09:00, akrual harian cron, Stage 3 net-carrying accrual (via Phase 4 ECL), dividen + distribusi reksadana + PPh, jurnal JATUH_TEMPO + AKRUAL_BUNGA + AMORTISASI_PD, `GET /transaksi/akrual?instrumen_id=&date=` | P5-M1, P5-M2, P5-M6, Phase 4 ECL core | **compliance BLOCKING** (Stage 3 net carrying + EIR method) | L |
| **P5-M10** | POCI delta ECL + jurnal P&L booking | APP-C/D | `internal/ecl/poci_delta` service, delta = lifetime ECL − origination ECL baseline (DEC-POCI-002), jurnal P&L booking PSAK 71 §5.5.14, Stage guard (no Stage 1 enforcement in staging engine), migration 000036 (`ecl.poci_delta_log`), warning removal `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA` → real delta | Phase 4 POCI baseline, P5-M2 (jurnal), `ecl.amortisasi_schedule` FK lookup | **compliance BLOCKING** (PSAK 71 §5.5.13-14 critical path) | M |
| **P5-M11** | APP-B Bulk Upload Master Instrumen | APP-B | `internal/trx/bulkupload` service, extends `sys.upload_batch` (migration 000037 adds `batch_mode`, `sheet_breakdown_json`, `rollback_status`), 5-sheet XLSX parser, 4-stage validation pipeline, DRY_RUN, async commit job (Asynq), partial commit OK, rollback (CFO), `POST /master/instrumen/bulk-upload/batch`, progress via §3 JobProgressPanel | P5-M1, Phase 3 `mst.instrumen` | advisory | M |
| **P5-M12** | APP-D Mapping Jurnal CRUD + 6-eyes workflow | APP-D | Frontend CRUD screen `/mapping-jurnal`, DataTable + per-event detail view, 6-eyes form (ROLE-AKUN Maker → ROLE-AKUN-CTL Approver → ROLE-RISK second-approver), import/export XLSX, mapping coverage dashboard (RPT-19), mapping validation report (RPT-20), mapping change history (RPT-21) | P5-M2 backend, P5-M4 frontend | **compliance BLOCKING** (mapping integrity) | M |
| **P5-M13** | APP-E Materialized views + reporting service foundation | APP-E | `internal/reporting` service, read-replica DB connection, 8 MVs (`rpt.mv_*`) creation migrations 000038, CONCURRENT refresh Asynq jobs, export engine (CSV/XLSX/PDF — `excelize`), async export > 10k rows → MinIO + notif, watermark + SHA-256 + audit EXPORT, scheduled email SMTP | P5-M9 (data), P5-M4 (periode close trigger refresh), P5-M3 (GL recon) | advisory | L |
| **P5-M14** | APP-E 28 Reports (batch delivery) | APP-E | All 28 REST endpoints RPT-01..28, query optimization (composite indexes, read-replica routing), all export formats, filter + sort + cursor per §1 UX, RBAC per report (`report.*` permissions), scheduled email configs | P5-M13 | advisory | L |
| **P5-M15** | APP-E Dashboards per role (frontend) | APP-E | Next.js: Treasury / Risk / Akuntansi / CFO+Direksi / Auditor dashboards, Recharts widgets, SSE push notifications, 5-min polling refresh, role-gated widgets, job history page `/jobs` | P5-M14 backend, P5-M12, P5-M4 | advisory | L |
| **P5-M16** | Frontend: APP-B transaction screens | APP-B | Next.js screens: `/transaksi/penempatan/new`, `/transaksi/mtm/upload`, `/transaksi/renewal`, `/transaksi/penjualan`, `/transaksi/jatuh-tempo` (monitoring), `/transaksi/akrual` — DataTable (§1) + form notif (§2) + JobProgressPanel (§3) untuk batch jobs | P5-M1, M6, M7, M8, M9 OpenAPI | advisory | L |
| **P5-M17** | Frontend: APP-D periode + FX + mapping screens | APP-D | Next.js: `/periode-buku` (timeline + closing workflow), `/master/kurs`, `/jurnal/header` (list + detail), `/reconciliation/daily`, MFA step-up component untuk hard-close | P5-M4, M5, M12 OpenAPI | advisory | M |
| **P5-M18** | QA end-to-end full Phase 5 + UAT deploy infra | Cross | Integration tests full cycle: penempatan → MTM → akrual → ECL → jurnal → GL delivery → periode close, POCI delta verify, OCI recycling, renewal EIR round-trip. UAT Docker Compose finalization, K8s prod prep, Prometheus + Grafana + Loki dashboards, runbooks | All P5-M1..M17 | **compliance BLOCKING** (final gate semua path regulated) + **security BLOCKING** (full audit trail verify) | L |

**Total: 18 modul.** Effort legend: S = ~3 hari, M = ~5 hari, L = ~8 hari. Perkiraan total effort: ~3 S + 7 M + 8 L = ~9 + 35 + 64 = **~108 hari/sprint-point** (dikerjakan paralel dalam team).

---

## 3. Sprint Breakdown

Setiap sprint ~2 minggu. Paralel dikerjakan oleh backend-engineer-go + ecl-eir-engineer + integration-engineer secara domain-separated, frontend-engineer-nextjs mulai saat OpenAPI committed sprint sebelumnya.

### Sprint 1 — Jurnal foundation + Penempatan (Blocker untuk semua)
**Modul**: P5-M1 → P5-M2 → P5-M5 (paralel dengan M2)

Reasoning: P5-M2 (jurnal resolver) adalah shared infrastructure yang dikonsumsi oleh semua event berikutnya. P5-M1 (penempatan) adalah first transaction event yang sekaligus test bed untuk P5-M2. P5-M5 (FX) dikerjakan paralel oleh integration-engineer karena tidak blocking M1/M2 start, tapi dibutuhkan M6.

Deliverables akhir Sprint 1:
- `trx.penempatan` fully tested + 4-eyes workflow
- Jurnal resolver engine operational + mapping template seed data
- FX rate management live + BI JISDOR cron configured

### Sprint 2 — GL delivery + Periode Buku + MTM
**Modul**: P5-M3 → P5-M4 (paralel) + P5-M6

Reasoning: P5-M3 (GL delivery) dan P5-M4 (periode close) dapat dikerjakan paralel setelah P5-M2 selesai. P5-M6 (MTM) mulai setelah P5-M1 + P5-M5 selesai.

Deliverables akhir Sprint 2:
- GL Host delivery pipeline (Asynq + retry + DLQ)
- Periode close OPEN→SOFT_CLOSED→CLOSED dengan step-up MFA
- MTM daily job + bulk upload XLSX + jurnal routing

### Sprint 3 — Renewal + Penjualan + Jatuh Tempo + Akrual
**Modul**: P5-M7 + P5-M8 (paralel) → P5-M9

Reasoning: M7 (renewal) dan M8 (penjualan) keduanya dependen P5-M1 + P5-M2 dan dapat dikerjakan paralel. P5-M9 (maturity + akrual harian) membutuhkan M6 (MTM done) dan lebih kompleks sehingga dikerjakan setelah M7/M8 clear.

Deliverables akhir Sprint 3:
- Renewal deposito end-to-end (2 skema)
- Penjualan + OCI recycling verified
- Jatuh Tempo job + akrual harian operational

### Sprint 4 — POCI delta + Bulk Upload + Frontend APP-B
**Modul**: P5-M10 (paralel dengan M11) + P5-M16

Reasoning: M10 (POCI delta) adalah compliance-critical tetapi tidak blocking frontend. M11 (bulk upload) adalah independent feature. Frontend Sprint 1-3 backend hasil bisa dikerjakan mulai Sprint 4 karena OpenAPI contracts sudah committed.

Deliverables akhir Sprint 4:
- POCI delta ECL + jurnal P&L live (removes warning `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA`)
- Bulk upload instrumen operational
- Frontend APP-B screens penuh (penempatan, MTM upload, renewal, penjualan, JT)

### Sprint 5 — Mapping Jurnal frontend + APP-D screens + Reporting foundation
**Modul**: P5-M12 + P5-M13 (paralel) + P5-M17

Reasoning: M12 (mapping jurnal frontend) dan M13 (reporting service + MVs) dapat dikerjakan paralel. M17 (APP-D screens) paralel dengan M13.

Deliverables akhir Sprint 5:
- Mapping jurnal CRUD + 6-eyes screen live
- Periode Buku + FX + GL screens live
- Reporting service + 8 MVs + export engine operational

### Sprint 6 — 28 Reports + Dashboards + QA final
**Modul**: P5-M14 → P5-M15 (paralel near end) + P5-M18

Reasoning: M14 (28 reports API) harus selesai sebelum M15 (dashboard frontend) bisa complete. M18 (QA + infra) dimulai bersamaan dengan M15 dan menjadi final gate.

Deliverables akhir Sprint 6:
- 28 reports lengkap + scheduled email
- Semua dashboard role-gated
- E2E integration tests PASS
- UAT infra deployed + compliance VERDICT PASS

### Summary sprint ordering

```
Sprint 1:  P5-M1 → P5-M2 ‖ P5-M5
Sprint 2:  (P5-M3 ‖ P5-M4) + P5-M6
Sprint 3:  (P5-M7 ‖ P5-M8) → P5-M9
Sprint 4:  (P5-M10 ‖ P5-M11) + P5-M16
Sprint 5:  (P5-M12 ‖ P5-M13) + P5-M17
Sprint 6:  P5-M14 → (P5-M15 ‖ P5-M18)
```

---

## 4. Per-Modul Handoff Sequence

Setiap modul mengikuti rantai standar. Paralel berlaku pada tahap yang tidak saling block.

```
business-analyst      → user story + AC Gherkin (input: modul spec dari plan ini + FSD source)
        ↓
system-analyst        → OpenAPI fragment + state machine + validation rules + error codes
                        (untuk APP-D modul: tambahkan jurnal mapping contract)
        ↓
data-modeler          → migration 000028..000038 — WAJIB: down.sql, audit cols, indexes, soft-delete
        ↓
uiux-designer         → desain screen/wireframe (paralel dengan backend setelah system-analyst selesai)
        ↓
backend-engineer-go   → HTTP handler, repo, Gin routing, Asynq worker, middleware — TIDAK menulis ECL/EIR math
ecl-eir-engineer      → POCI delta + ECL-related logic di M10, akrual EIR-method di M9 (Stage 3 net carrying)
integration-engineer  → GL Host REST adapter (M3), BI JISDOR job (M5), scheduled email SMTP (M13)
(semua 3 paralel per domain)
        ↓
frontend-engineer-nextjs → setelah OpenAPI contract committed per modul
        ↓
qa-engineer           → integration tests + UAT scripts + run report
        ↓
security-engineer     → BLOCKING review untuk: P5-M4 (MFA hard-close), P5-M10 (audit trail POCI), P5-M2 (jrnl append-only), P5-M18 (full audit verify)
        ↓
ifrs9-compliance-reviewer → BLOCKING gate untuk: P5-M1, M2, M4, M6, M7, M8, M9, M10, M13, M14, M18
        ↓
devops-engineer       → P5-M18 UAT + prod infra deploy + runbooks
```

**Catatan role separation**:
- `ecl-eir-engineer` owns M10 (POCI delta) dan akrual EIR-method dalam M9. Backend menulis plumbing.
- `integration-engineer` owns GL Host adapter (M3), BI JISDOR scraper/parser (M5), dan SMTP scheduler (M13). Backend menulis handler.
- Mapping jurnal resolver runtime (M2) — implementasi oleh `backend-engineer-go`, formula/filter logic di-review `ifrs9-compliance-reviewer`.

---

## 5. Blocking Gates per Modul

### ifrs9-compliance-reviewer — BLOCKING (regulated path)

| Modul | Kriteria blocking |
|---|---|
| **P5-M1** | EIR auto-trigger post-approve sesuai DEC-013. Jurnal event PENEMPATAN balance. Klasifikasi enforce (FVTPL no ECL). |
| **P5-M2** | Runtime resolver balance check wajib (`total_debit == total_kredit` ± 0.01). Jurnal rows append-only (no UPDATE jrnl.*). Mapping template coverage untuk semua 15+ event codes. |
| **P5-M4** | Pre-condition checklist enforced (0 PENDING_APPROVAL). CFO step-up MFA wajib hard-close (DEC-027). Locked FX on CLOSED. |
| **P5-M6** | OCI vs P&L routing per klasifikasi matrix (FVOCI → OCI, FVTPL → P&L, FVOCI_ELECTION → OCI no-recycle). STALE_PRICE tidak stop posting. |
| **P5-M7** | PPh 20% computation. EIR re-compute pada instrumen baru (Newton-Raphson, DEC-013). Skema POKOK_SAJA vs POKOK_PLUS_BUNGA jurnal berbeda. |
| **P5-M8** | OCI recycling FVOCI debt (REKLAS_OCI_PL event). FVOCI Election = no recycling. BM Test frequency alert trigger (>5% dalam 12 bulan). |
| **P5-M9** | Stage 3 akrual = Net Carrying × EIR (DEC-010). Amortisasi closing-carrying ≈ par ±0.01. Dividen tidak kena PPh (WP Badan) — conditional PPh logic. |
| **P5-M10** | PSAK 71 §5.5.13-14 delta ECL formula correct. Stage 1 prohibition untuk POCI (DEC-POCI-004). Jurnal P&L booking match delta. |
| **P5-M13** | MV refresh schedule sesuai FSD-APP-E §1.2. Watermark + SHA-256 + audit EXPORT wajib. RBAC per report enforce. |
| **P5-M14** | RPT-07 roll-forward formula = ECL_closing = opening + originations − derecognitions ± transfers ± remeasurements. RPT-25 carrying roll-forward per PSAK 71 §35H. |
| **P5-M18** | Full E2E: penempatan → MTM → akrual → ECL → jurnal → GL → periode close. POCI delta verified. OCI recycling verified. Roll-forward reconcile delta < IDR 1. |

### security-engineer — BLOCKING (auth/PII/audit path)

| Modul | Kriteria blocking |
|---|---|
| **P5-M2** | `jrnl.header` + `jrnl.detail` no hard delete. Audit `JURNAL.POST` in-transaction. No PERIODE_CLOSED bypass. |
| **P5-M4** | step-up MFA pada `/hard-close-approve` (DEC-027). CEO approval untuk REOPEN. Audit trail `PERIODE.HARDCLOSE` in-transaction. |
| **P5-M10** | Audit `ECL.POCI_DELTA_COMPUTE` in-transaction. POCI log rows immutable. |
| **P5-M18** | Full audit trail verify: hash chain continuity, EXPORT log, SoD enforcement skenario "Maker → Approve sendiri". |

### data-modeler — STANDARD gate (semua schema change)

Semua migrasi 000028..000038 wajib review data-modeler sebelum backend engineer mulai implementasi.

---

## 6. Decision Log Items Baru (DEC-030+)

Items berikut perlu dikonfirmasi atau diputuskan sebelum modul terkait dimulai. Jika tidak ada RFC, gunakan default yang tertera.

| ID | Topik | Pertanyaan | Default | Modul | Perlu RFC? |
|---|---|---|---|---|---|
| **DEC-030** | GL Host integration mode | Sync REST (blocking call per jurnal) vs Async REST (Asynq queue per jurnal, delivery confirmed via callback) | **Async REST via Asynq** (sesuai DEC-007, performance, retry semantik) | P5-M3 | Ya jika GL Host vendor tertentu butuh sync |
| **DEC-031** | GL Host vendor | SAP ERP, Oracle Financials, in-house, atau TBD? Ini menentukan API contract (field mapping, auth scheme) | **TBD — integration-engineer melakukan discovery** sebelum M3 start | P5-M3 | Ya — butuh info dari Tugure IT |
| **DEC-032** | Report output format mandatory | XLSX saja, PDF saja, atau keduanya wajib untuk setiap report? FSD-APP-E §1.3 menyebut keduanya. | **XLSX wajib untuk semua; PDF wajib untuk RPT-03, RPT-07, RPT-08, RPT-15, RPT-25 (disclosure-ready); lainnya PDF opsional** | P5-M14 | Tidak — default ini cukup |
| **DEC-033** | Report access RBAC granularity | Per-report permission (`report.ecl`, `report.ckpn`) atau kategori (`report.read` semua)? | **Per-report permission** sesuai distribusi list di FSD-APP-E §2 | P5-M13, M14 | Tidak |
| **DEC-034** | Jurnal event codes — master list locked | Apakah 15+ event codes di FSD-APP-D §3.2 sudah final, atau ada tambahan dari APP-B (mis. DIVIDEN, FX_REVALUATION)? | **Tambahan APP-B diperlukan**: AKRUAL_BUNGA, AKRUAL_DIVIDEN, FX_UNREALIZED, AMORTISASI_PREMI_DISKONTO, PEMBAYARAN_PPH. data-modeler + ifrs9-compliance-reviewer konfirmasi final list sebelum P5-M2 migration. | P5-M2 | Tidak — tapi butuh konfirmasi cepat |
| **DEC-035** | Scheduled email infrastructure | Internal SMTP relay Tugure vs external service (SendGrid, SES) | **Internal SMTP relay** (sesuai on-premise DEC-008, data residency UU PDP) | P5-M13 | Tidak |
| **DEC-036** | Cure evaluation di SOFT_CLOSED | Apakah cure berjalan di periode SOFT_CLOSED (adjustment window)? FSD-APP-B §1.4 menyebut cure perlu dievaluasi walaupun periode SOFT_CLOSED. | **Ya — cure evaluation berjalan di SOFT_CLOSED** jika di-trigger manual oleh ROLE-RISK; tidak otomatis. Jurnal cure-related posting masuk sebagai PERIODE_ADJUSTMENT. | P5-M4, P5-M9 | Tidak — default ini cukup, confirm compliance |

---

## 7. Open Questions — Perlu Klarifikasi Sebelum Modul 1

| ID | Pertanyaan | Default assumsi | Butuh konfirmasi dari | Modul |
|---|---|---|---|---|
| **OQ-P5-A** | GL Host vendor final — SAP, Oracle, atau in-house? Ini critical untuk P5-M3 adapter design (endpoint, auth, field mapping). | TBD. Integration-engineer lakukan discovery call dengan Tugure IT/Finance. | Direktur IT + Kepala Akuntansi Tugure | **P5-M3** |
| **OQ-P5-B** | Mapping Jurnal event code master list — berapa total event codes yang perlu seed di migration 000029? FSD-APP-D §3.2 menyebut kategori, bukan list exhaustive. | ~18 event codes: PENEMPATAN, AKRUAL_BUNGA, AMORTISASI_PREMI_DISKONTO, MTM_FVOCI, MTM_FVTPL, MTM_FVOCI_ELECTION, PEMBAYARAN_KUPON, DIVIDEN_SAHAM, DISTRIBUSI_RDN, PENJUALAN_PENCAIRAN, REKLAS_OCI_PL, JATUH_TEMPO, RENEWAL_DEPOSITO, ECL_CHARGE, ECL_REVERSAL, FX_UNREALIZED, FX_REALIZED, PERIODE_ADJUSTMENT. | ifrs9-compliance-reviewer + business-analyst konfirmasi | **P5-M2** |
| **OQ-P5-C** | Mapping Jurnal 6-eyes vs 4-eyes — FSD-APP-D §3 tidak explicitly state 6-eyes, tapi DEC-017 menyebut "parameter master". Apakah mapping jurnal change masuk kategori 6-eyes? | Default 4-eyes (Maker AKUN → Approver AKUN-CTL). 6-eyes hanya jika ada klasifikasi PSAK 71 impact baru. | business-analyst + ifrs9-compliance-reviewer | **P5-M2**, **P5-M12** |
| **OQ-P5-D** | POCI delta ECL computation frequency — apakah delta dihitung setiap ECL calc run atau hanya saat periode close? | Per ECL calc run (konsisten dengan non-POCI flow di Phase 4). Delta = current_lifetime_ECL − ECL_at_origination dari `ecl.calc_header` run pertama. | ecl-eir-engineer + ifrs9-compliance-reviewer | **P5-M10** |
| **OQ-P5-E** | Pendapatan akrual harian — apakah akrual harian di-post ke `jrnl.detail` setiap hari (tinggi volume) atau diagregasi bulanan? | **Bulanan aggregate** untuk posting jurnal (performance), tapi `trx.pendapatan_akrual` rows tetap harian untuk audit trail. Daily view untuk Akuntansi, posting ke GL bulanan. | Kepala Akuntansi + ifrs9-compliance-reviewer | **P5-M9**, **P5-M2** |
| **OQ-P5-F** | BM Test frequency threshold — FSD-APP-B §4.4 menyebut >5% dalam 12 bulan. Apakah threshold ini konfigurabel atau hard-coded? | **Konfigurabel per portofolio** di `mst.portofolio.bm_threshold_persen` (default 5%). | business-analyst + ROLE-RISK | **P5-M8** |
| **OQ-P5-G** | Virus scan untuk bulk upload instrumen — sync (blocking upload) atau async (post-upload scan, notify jika infected)? | **Async** untuk file > 5MB. Sync untuk < 5MB. Sesuai existing `doc.*` pattern dari Phase 2. | system-analyst konfirmasi pola konsisten | **P5-M11** |
| **OQ-P5-H** | RPT-13 MEV Sensitivity — apakah stress test dilakukan di application layer atau di database stored procedure? MEV components belum ada di schema Phase 3. | **Application layer** via `internal/reporting/mev_sensitivity.go`. MEV parameters diinput manual per kuartal di `sys.config_param` (atau tabel baru `mst.mev_param`). | data-modeler + system-analyst | **P5-M13**, **P5-M14** |

---

## 8. First Modul (P5-M1) — Siap Delegate

### Prompt untuk `business-analyst`

```
Tolong tulis user story + AC Gherkin untuk P5-M1: trx.penempatan CRUD + 4-eyes workflow.

Konteks:
- Modul: APP-B Transaction Lifecycle — Penempatan Instrumen, Phase 5
- Sumber: FSD-APP-B-TransactionLifecycle-v1.1.docx §1 (Modul Penempatan), SoW_v1.4.docx §5.2
- Phase 3 sudah deliver: mst.instrumen (fully CRUD, klasifikasi locked), mst.counterparty, mst.portofolio, mst.periode_buku (OPEN check)
- Phase 4 sudah deliver: ecl.eir service (EIR auto-compute), ecl.amortisasi_schedule, doc.* service (upload)
- DEC-017: 4-eyes SoD (maker_id ≠ reviewer_id ≠ approver_id), tidak ada 6-eyes untuk penempatan biasa

Cakupan story:
1. Story APP-B-TRX-001: Penempatan obligasi/deposito baru (Maker submit)
   - Actor: ROLE-MAKER-TR
   - Pre-conditions: instrumen aktif + klasifikasi locked, counterparty aktif + rating valid, periode OPEN, saldo rekening ≥ total pembayaran, min 1 dokumen uploaded
   - Auto-fields: no_transaksi (PNP-{YYYY}-{#####}), EIR preview (non-blocking), carrying_amount_awal
   - Workflow: DRAFT → submit → PENDING_APPROVAL
   - AC Gherkin: penempatan deposito sukses, klasifikasi belum locked (ERR-VAL-2001), saldo tidak cukup (ERR-VAL-2002)

2. Story APP-B-TRX-002: Approve penempatan (Approver)
   - Actor: ROLE-APPR-TR (bukan maker yang sama — SoD)
   - Post-approve triggers: EIR auto-compute (untuk AC/FVOCI), jurnal PENEMPATAN (async via P5-M2)
   - AC Gherkin: approve sukses → EIR computed + jurnal queued, SoD violation (ERR-SOD-VIOLATION), periode sudah SOFT_CLOSED saat approve (re-check)

3. Story APP-B-TRX-003: EIR preview sebelum submit
   - Actor: ROLE-MAKER-TR
   - GET /transaksi/penempatan/{id}/eir-preview → {eir_awal, carrying_awal, amortization_schedule_preview (10 periode)}
   - AC: tanpa nominal (ERR-VAL), dengan biaya transaksi, multi-currency instrument

4. Story APP-B-TRX-004: List + filter penempatan
   - Actor: ROLE-MAKER-TR, ROLE-APPR-TR, ROLE-RISK, ROLE-AUDIT
   - DataTable UX §1: sort, cursor pagination, filter (status, instrumen_type, periode, counterparty), export CSV/XLSX
   - AC: ROLE-AUDIT bisa read-only termasuk yang deleted_at IS NOT NULL dengan ?include_deleted=true

Output yang diminta:
- docs/stories/phase-5/APP-B-TRX-001-penempatan.md sampai APP-B-TRX-004
- AC Gherkin minimal 3 skenario per story (happy path, SoD/permission, edge case)
- Flag OQ-P5-A sampai OQ-P5-H yang relevan ke story ini

Handoff berikutnya: system-analyst (OpenAPI + state machine penempatan workflow + error codes)
```

### Setelah business-analyst — prompt untuk `system-analyst`

```
Tolong buat OpenAPI contract + state machine untuk P5-M1 trx.penempatan.

Input: stories APP-B-TRX-001..004 dari business-analyst
Sumber tambahan: FSD-APP-B §1.5 (API endpoints), FSD-APP-B §1.3 (field specs), FSD-APP-B §1.4 (business rules)

Deliverables:
1. api/openapi/trx-penempatan.yaml — semua endpoint dari FSD-APP-B §1.5 + eir-preview endpoint
2. State machine: DRAFT → PENDING_APPROVAL → APPROVED | REJECTED (dengan trigger EIR + jurnal)
3. Go interface contract untuk backend-engineer-go:
   interface PenempatanService {
     Create(ctx, req CreatePenempatanReq) (Penempatan, error)
     Submit(ctx, id UUID, actorID UUID) error
     Approve(ctx, id UUID, actorID UUID, comment string) error
     Reject(ctx, id UUID, actorID UUID, comment string) error
     GetEIRPreview(ctx, id UUID) (EIRPreview, error)
     List(ctx, query listquery.Query) ([]Penempatan, Pagination, error)
   }
4. Event contract untuk jurnal trigger (consumed by P5-M2):
   PenempatanApprovedEvent { ID, InstrumenID, PeriodeID, TotalPembayaranIDR, Klasifikasi, EAD }

Pastikan: error codes ERR-VAL-2001..2005, ERR-CALC-2010, ERR-INT-2020, SOD_VIOLATION, WORKFLOW_INVALID_TRANSITION, cursor pagination, Idempotency-Key wajib di setiap mutation.
```

### Setelah system-analyst — prompt untuk `data-modeler`

```
Tolong buat migration 000028 untuk P5-M1 trx.penempatan.

Target schema: trx.penempatan (baru — belum ada di 000001 init_schema).
Berdasarkan FSD-APP-B §1.6 schema preview + konvensi db-conventions.md:

Wajib include:
- Semua audit cols: created_at, created_by, updated_at, updated_by, deleted_at, deleted_by, row_version, tenant_id
- Kolom-kolom field spec FSD-APP-B §1.3
- NUMERIC precision: nominal NUMERIC(20,4), harga_beli NUMERIC(15,4), eir_awal NUMERIC(12,8) sesuai DEC-016
- Soft-delete (deleted_at) — no hard delete
- CHECK constraints: ck_settlement_after_trade, ck_total_positive
- Indexes: (instrumen_id), (periode_id), (tanggal_transaksi), (workflow_status) WHERE deleted_at IS NULL, (tenant_id, created_at DESC)
- FK ke mst.instrumen, mst.periode_buku, sec.user (maker + approver)
- Signature columns: approved_at, rejected_at, reject_reason

Tag file:
-- migration: 0028 trx_penempatan
-- author: data-modeler
-- requires: 0001, 0027 (last Phase 4 migration)
```

---

## 9. Risk & Rollback

| Risk | Severity | Mitigasi |
|---|---|---|
| OQ-P5-A (GL Host vendor TBD) blok P5-M3 | HIGH | Mulai P5-M3 dengan mock adapter + interface contract. Integration-engineer lakukan discovery dalam Sprint 1 paralel. Prod adapter di-swap sebelum UAT Sprint 6. |
| Jurnal mapping incomplete: event code seed tidak cover semua klasifikasi | HIGH | ifrs9-compliance-reviewer WAJIB review seed data sebelum P5-M2 merge. UAT test case per event code minimal. |
| POCI delta (P5-M10) — origination CA-EIR schedule lookup missing untuk instrumen lama | HIGH | Phase 4.5 sudah compute CA-EIR untuk instrument POCI baru. Untuk instrumen POCI historical (pre-go-live), butuh migration backfill plan via bulk upload (P5-M11 DRY_RUN mode). |
| Akrual harian volume: 1500 instrumen × 365 hari = 547k rows per tahun di trx.pendapatan_akrual | MEDIUM | Partition by month (pg_partman) di migration 000035. Asynq batch worker. Aggregate posting bulanan ke jurnal (OQ-P5-E answer). |
| MTM daily job timeout jika IBPA/BEI feed terlambat | MEDIUM | STALE_PRICE fallback (FSD-APP-B §2.5), retry 3× dengan 30 menit interval, alert Akuntansi jam 19:00 jika masih kosong. |
| step-up MFA tidak ter-enforce di hard-close | BLOCKING | security-engineer BLOCKING review P5-M4. Test skenario "POST /hard-close-approve tanpa X-Step-Up-Token → 401". |
| float64 slip di akrual harian calculation | HIGH | ecl-eir-engineer: shopspring/decimal enforced, golangci-lint forbidigo rule. Code review focus di M9. |
| Report query terlalu berat, hit OLTP DB | MEDIUM | Read-replica routing enforced di `internal/reporting` service. MV refresh untuk aggregate queries. Index composite audit Sprint sebelum M13 merge. |
| OCI recycling (M8) salah arah — FVOCI Election ikut recycled | BLOCKING | ifrs9-compliance-reviewer BLOCKING review P5-M8. Unit test cover: FVOCI debt → recycled, FVOCI Election → not recycled. |

### Rollback plan
- Semua migrasi 000028..000038 memiliki `.down.sql` idempotent dan ditest sebelum merge
- `trx.*` records: soft-delete only (`deleted_at`)
- `jrnl.*` records: append-only, no hard-delete, reversal via JURNAL_REVERSAL event (tidak hapus original)
- Bulk upload rollback (M11): CFO-only, pre-condition = no linked transactions
- Periode close rollback: REOPEN procedure (CEO approval required per FSD-APP-D §1.4)

---

## 10. Estimated Timeline

| Sprint | Modul | Durasi | Milestone |
|---|---|---|---|
| Sprint 1 | P5-M1, P5-M2, P5-M5 | 2 minggu | Penempatan + jurnal foundation live |
| Sprint 2 | P5-M3, P5-M4, P5-M6 | 2 minggu | GL delivery + Periode close + MTM operational |
| Sprint 3 | P5-M7, P5-M8, P5-M9 | 2 minggu | Full transaction lifecycle complete |
| Sprint 4 | P5-M10, P5-M11, P5-M16 | 2 minggu | POCI delta live + frontend APP-B ready |
| Sprint 5 | P5-M12, P5-M13, P5-M17 | 2 minggu | Reporting service + APP-D frontend ready |
| Sprint 6 | P5-M14, P5-M15, P5-M18 | 2–3 minggu | 28 reports + dashboards + QA final + UAT deploy |

**Total: 6 sprint × 2 minggu = ~12–13 minggu** (3 bulan). Dengan asumsi blocker OQ-P5-A (GL Host vendor) dijawab dalam Sprint 1 dan tidak ada scope creep.

---

## 11. Verifikasi (Gate 5→6 — Phase 6 dapat dimulai)

- [ ] Semua 18 modul P5-M1..M18 merged ke `develop` + CI hijau
- [ ] `ifrs9-compliance-reviewer` VERDICT = PASS untuk semua modul regulated (M1, M2, M4, M6, M7, M8, M9, M10, M14, M18)
- [ ] `security-engineer` VERDICT = PASS untuk M2, M4, M10, M18
- [ ] Integration test full cycle: penempatan → MTM → akrual → ECL (Phase 4) → jurnal → GL delivery → periode close
- [ ] POCI delta verified: current ECL − origination ECL = delta, jurnal P&L matches
- [ ] OCI recycling verified: FVOCI debt → recycled, FVOCI Election → not recycled
- [ ] Jurnal balance: total_debit == total_kredit untuk setiap event (delta tolerance ≤ IDR 0.01)
- [ ] 28 reports semua return valid data untuk periode uji; export CSV/XLSX/PDF berhasil
- [ ] Dashboard per role: Treasury, Risk, Akuntansi, CFO, Auditor — data sesuai RBAC
- [ ] UAT deploy: Docker Compose UAT up, Prometheus + Grafana + Loki operational
- [ ] OQ-P5-A..H semua dijawab dan dicatat di §7 (update in-place)
- [ ] All migrations 000028..000038 have passing `.down.sql` test
- [ ] `pnpm -C web build && pnpm -C web lint` hijau
- [ ] `go build ./... && golangci-lint run && go test ./...` hijau
- [ ] Warning `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA` tidak muncul lagi di production ECL runs

---

## Lampiran — Commit Convention Phase 5

Branch: `feature/app-b-{slug}`, `feature/app-d-{slug}`, `feature/app-e-{slug}`, `feature/app-c-poci-delta`

Expected commit scopes:
```
feat(app-b): implement trx.penempatan CRUD + 4-eyes workflow (P5-M1)
feat(app-d): implement jurnal mapping resolver + jrnl schema (P5-M2)
feat(app-d): implement GL host async delivery + reconciliation (P5-M3)
feat(app-d): implement periode buku close workflow + step-up MFA (P5-M4)
feat(app-d): implement FX rate management + BI JISDOR cron (P5-M5)
feat(app-b): implement MTM daily job + bulk upload XLSX (P5-M6)
feat(app-b): implement renewal deposito workflow (P5-M7)
feat(app-b): implement penjualan OCI recycling + derecognition (P5-M8)
feat(app-b): implement maturity job + pendapatan akrual harian (P5-M9)
feat(app-c): implement POCI delta ECL + jurnal P&L booking (P5-M10)
feat(app-b): implement bulk upload master instrumen (P5-M11)
feat(app-d): add mapping jurnal CRUD 6-eyes frontend (P5-M12)
feat(app-e): implement reporting service + 8 materialized views (P5-M13)
feat(app-e): implement 28 reports + export engine + scheduled email (P5-M14)
feat(app-e): add dashboard screens per role (P5-M15)
feat(web): add APP-B transaction lifecycle screens (P5-M16)
feat(web): add APP-D periode + FX + mapping screens (P5-M17)
test(phase-5): E2E integration tests + UAT infra finalization (P5-M18)
feat(db): migration 000028 trx_penempatan
feat(db): migration 000029 jrnl_header_detail + mapping_jurnal_master
feat(db): migration 000030 mst_kurs_fx_management
feat(db): migration 000031 trx_mtm
feat(db): migration 000032 sys_upload_batch_mtm
feat(db): migration 000033 trx_renewal
feat(db): migration 000034 trx_penjualan
feat(db): migration 000035 trx_jatuh_tempo_pendapatan_akrual
feat(db): migration 000036 ecl_poci_delta_log
feat(db): migration 000037 sys_upload_batch_instrumen_bulk
feat(db): migration 000038 rpt_materialized_views
```

---

_Dokumen ini diupdate seiring OQ-P5-A..H dijawab dan modul-modul merged. DEC-030..036 yang belum diputuskan wajib di-resolve paling lambat awal Sprint modul terkait._
