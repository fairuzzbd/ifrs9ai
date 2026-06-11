# PLAN — Phase 4 ECL Engine + EIR (APP-C)

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-11
**Branch base**: `develop` (setelah PR #34 merge — semua 12/12 modul Phase 3 selesai)
**Sumber kebenaran**:
- `FSD-APP-C-ECL-EIR-v1.0.docx` — master spec modul ini
- `SoW_v1.4.docx` §4 — ECL formula reference
- `BLIPS_Decision_Log_v1.0.docx` DEC-010..018 — locked decisions
- `Pefindo_Annual_Default_Study_2007-2025_EN.pdf` — PD kalibrasi reference
- `.claude/memory/formulas.md` — ECL + EIR reference implementation
- `.claude/memory/locked-decisions.md` — quick ref DEC
- `db/migrations/000001_init_schema.up.sql` §9 — schema `ecl.*` existing (calc_header, calc_detail_skenario, lookthrough_underlying, stage_history, eir_amortization_schedule, eir_reestimation_log, trx.amortisasi)
**Klasifikasi**: feature — **SEMUA path regulated** (ECL/EIR/staging/sealing). Seluruh modul wajib lewat `ifrs9-compliance-reviewer` gate.
**Target**: ~4–5 minggu dari kick-off

---

## 1. Scope & Exclusions

### Dalam scope Phase 4

| Domain | Cakupan |
|---|---|
| **Staging engine** | Evaluasi SICR (≥2 notch, IG→non-IG, DPD≥30), stage assignment, cure 3 bulan, `ecl.stage_history` |
| **PD/LGD/EAD helpers** | Lookup service dari master Phase 3 (pd_pefindo, lgd_basel, lps_coverage), EAD = Outstanding + Accrued + (Undrawn × CCF), multi-currency via kurs |
| **ECL core calculation** | 3-stage × 3-skenario × dual FL multiplier, `ecl.calc_header` + `ecl.calc_detail_skenario` |
| **LPS aggregator** | Cash + Deposito per nasabah per bank, cap IDR 2M, ECL hanya atas excess |
| **Look-through Reksadana** | Decompose underlying asset class, weighted ECL, `ecl.lookthrough_underlying` |
| **EIR — Newton-Raphson** | IRR solver (tolerance 1e-10, max 100 iter), schedule generation, `ecl.eir_amortization_schedule` |
| **EIR amendment & re-estimation** | Trigger on contract modification, insert new schedule version (immutable), `ecl.eir_reestimation_log` |
| **ECL calc run orchestration** | Asynq worker, long-running job (progress UX §3), `sys.job`, SSE stream, sealing |
| **Parameter snapshot** | Freeze active ECL params (pd_pefindo, lgd_basel, bobot_skenario, impact_pd, lps_coverage) at run-time for audit trail |
| **ECL run UI** | Trigger + monitor + detail view + seal UI |
| **EIR schedule UI** | View amortization schedule per instrumen, re-estimation trigger, history |
| **Staging history UI** | Stage timeline per instrumen, SICR evidence, audit trail |
| **Roll-forward CKPN** | Agregasi: opening + origination − derecognition ± transfer ± remeasurement = closing (query read-only, bukan table baru) |
| **Read-only pending TRX integration** | ECL engine membaca `mst.instrumen`, `trx.penempatan`, `trx.pendapatan_akrual` untuk EAD — tidak menulis ke `trx.*` |

### Exclusions (bukan Phase 4)

| Excluded | Alasan |
|---|---|
| SPPI Test + BM Assessment workflow | Sudah ada di Phase 3 schema, Phase 4 hanya consume hasil klasifikasi |
| GL posting dari ECL | Phase 5 (Jurnal & GL posting engine) |
| Transaction lifecycle (APP-B) | Phase 5 — penempatan, MTM, renewal, penjualan, jatuh tempo |
| Laporan ECL formal (APP-E) | Phase 6 — reporting dashboard |
| Full APP-D periode close dengan jurnal | Phase 5 |
| POCI (Purchased or Originated Credit Impaired) | Backlog post-Phase 4 bila ada instrument jenis ini |

---

## 2. Modul Breakdown

Prasyarat bersama semua modul: schema `ecl.*` di `000001` sudah ada. Migrasi Phase 4 (`000022+`) bersifat **schema-fix** (audit cols, precision, workflow, index) dan tambahan tabel baru bila perlu.

| ID | Title | Deliverables | Depends | Gate | Effort |
|---|---|---|---|---|---|
| **P4-M1** | Staging engine + SICR/cure | `internal/ecl/staging` service+repo+tests, eval per instrumen, `ecl.stage_history` schema-fix (mig 000022), `GET /ecl/staging/current/{instrumen_id}`, `GET /ecl/staging/history/{instrumen_id}` | `mst.instrumen`, `mst.rating_history_counterparty`, `mst.pd_pefindo` Phase 3 | **compliance BLOCKING** | M |
| **P4-M2** | PD/LGD/EAD lookup helpers | `internal/ecl/params` service: `LookupPD(rating, stage, periodeID)`, `LookupLGD(tipeEksposur, periodeID)`, `ComputeEAD(instrumen, kurs)`, `LookupFL(periodeID)`, `LookupBobot(periodeID)` — NUMERIC precision tests, no float64 | P4-M1, Phase 3 masters (pd_pefindo, lgd_basel, bobot_skenario, impact_pd, lps_coverage, kurs) | **compliance BLOCKING** | S |
| **P4-M3** | LPS aggregator | `internal/ecl/lps` service: aggregate per (counterparty, bank), cap IDR 2M, output excess EAD. Unit tests di atas P4-M2 EAD | P4-M2, `mst.lps_coverage` | **compliance BLOCKING** | S |
| **P4-M4** | Look-through Reksadana | `internal/ecl/lookthrough` service: decompose komposisi underlying, weighted ECL, `ecl.lookthrough_underlying` schema-fix (mig 000023) | P4-M2, P4-M3 | **compliance BLOCKING** | M |
| **P4-M5** | EIR Newton-Raphson solver + schedule generation | `internal/ecl/eir` package: IRR solver (tolerance 1e-10, 100 iter, seed=kupon), schedule builder, `ecl.eir_amortization_schedule` schema-fix (mig 000024), day-count ACT/365, `trx.amortisasi` writer | P4-M2, `mst.instrumen` (kupon, tanggal_jatuh_tempo, biaya_transaksi_capitalized) | **compliance BLOCKING** | L |
| **P4-M6** | EIR amendment & re-estimation | `internal/ecl/eir/amendment` service: trigger on contract modification event, insert new schedule version (immutable, `recomputed_from_seq`), `ecl.eir_reestimation_log` schema-fix (mig 000025), 4-eyes workflow | P4-M5 | **compliance BLOCKING** | M |
| **P4-M7** | ECL core calculation engine | `internal/ecl/engine` service: 3-stage × 3-skenario × dual FL, weighted aggregate, `ecl.calc_header` + `ecl.calc_detail_skenario` schema-fix (mig 000026), parameter snapshot, net-carrying Stage 3 interest | P4-M1, P4-M2, P4-M3, P4-M4, P4-M5 | **compliance BLOCKING** | L |
| **P4-M8** | ECL calc run orchestration (Asynq + seal) | `internal/ecl/calcrun` worker+handler: POST `/ecl/calc-runs` → 202+jobId, Asynq job, progress via Redis+SSE, seal endpoint + step-up MFA, `sys.job` persist, `ecl.calc_header.calc_run_id` FK | P4-M7, Phase 2 Asynq+job infra | **compliance BLOCKING** + **security BLOCKING** (seal + step-up) | L |
| **P4-M9** | EIR + staging UI screens | Next.js: `/ecl/eir/instrumen/[id]` (schedule view + re-estimation form), `/ecl/staging/instrumen/[id]` (timeline), history DataTable (UX §1), form notif (UX §2) | P4-M5, P4-M6, P4-M1 OpenAPI contract | — | M |
| **P4-M10** | ECL calc run UI | Next.js: `/ecl/runs` (list DataTable), `/ecl/runs/new` (trigger form), `/ecl/runs/[id]` (detail + scenario breakdown + seal button + JobProgressPanel UX §3) | P4-M8 OpenAPI contract | — | M |
| **P4-M11** | Roll-forward CKPN read query + staging history UI aggregation | `internal/ecl/rollforward` read-only service: opening + origination − derecognition ± transfer ± remeasurement. Endpoint `GET /ecl/roll-forward?periode_id=` + UI summary card on calc run detail page | P4-M7, P4-M8 | **compliance BLOCKING** | M |
| **P4-M12** | QA end-to-end + compliance gate final | Integration tests full ECL cycle (instrument → stage → EAD → ECL calc → seal), EIR round-trip, LPS excess verify, look-through verify. UAT scripts. Compliance VERDICT final. | All P4-M1..M11 | **compliance BLOCKING** (final gate) | M |

**Total: 12 modul.** Effort legend: S = ~3 hari, M = ~5 hari, L = ~8 hari.

---

## 3. Sequencing Decision

### Batch 1 — Foundations (run pertama, blocker untuk semua)
**M1 + M2 + M3** secara sekuensial (M2 dependen pada M1 selesai, M3 dependen pada M2).

Reasoning: M1 (staging) dan M2 (param lookups) adalah primitive building blocks. Tidak ada modul lain yang bisa diimplementasi tanpa keduanya. M3 (LPS) adalah add-on kecil di atas M2 — kerjakan bersamaan dalam satu sprint.

Sprint 1 target: M1 → M2 → M3 selesai + compliance sign-off per modul.

### Batch 2 — Vertical domain completion (paralel M4 ‖ M5)
**M4** (look-through Reksadana) dan **M5** (EIR solver) dapat dikerjakan **paralel** setelah M3 selesai karena keduanya consume M2 secara independen.

Reasoning: M4 adalah special-case ECL yang self-contained. M5 adalah pure math engine yang tidak bergantung pada M4. Paralel menghemat waktu ~8 hari.

M6 (EIR amendment) dikerjakan langsung setelah M5 selesai karena heavily dependen.

### Batch 3 — ECL core + orchestration (M7 → M8 sekuensial)
**M7** (calc engine) adalah integrasi semua output Batch 1+2. Harus selesai sebelum M8. M8 (Asynq orchestration + seal) wraps M7 dalam worker context.

Reasoning: M7 adalah modul paling kompleks, compliance review paling dalam. Sequential dependency ketat.

### Batch 4 — Frontend + reporting (M9 ‖ M10, lalu M11)
**M9** dan **M10** dapat dikerjakan **paralel** setelah OpenAPI contract M5/M6 dan M8 masing-masing committed.

**M11** (roll-forward) dikerjakan setelah M7+M8 selesai karena butuh `ecl.calc_header` data populated.

### Batch 5 — QA gate (M12)
M12 menunggu semua M1..M11 selesai. Full E2E integration tests + final compliance VERDICT.

### Summary ordering

```
Sprint 1:  M1 → M2 → M3
Sprint 2:  M4 ‖ M5 → M6
Sprint 3:  M7 → M8
Sprint 4:  M9 ‖ M10 → M11
Sprint 5:  M12 (QA + compliance final)
```

---

## 4. Per-Modul Handoff Sequence (Template)

Setiap modul mengikuti rantai ini. Paralel berlaku pada tahap yang tidak saling block.

```
business-analyst      → user story + AC Gherkin (input: modul spec dari plan ini + FSD-APP-C)
        ↓
system-analyst        → OpenAPI fragment + state machine + validation rules + error codes
        ↓
data-modeler          → migration 000022..000027 (schema-fix: audit cols, precision, indexes, workflow_status) — HANYA jika ada DDL change
        ↓
uiux-designer         → desain screen (paralel dengan backend-engineer setelah system-analyst selesai)
        ↓
ecl-eir-engineer      → SEMUA ECL/EIR math — staging SICR/cure, PD/LGD/EAD compute, LPS, look-through, EIR solver, ECL formula, roll-forward
        ↓
backend-engineer-go   → HTTP handler, repo, Gin routing, Asynq worker plumbing, middleware integration (TIDAK menulis ECL math)
        ↓
frontend-engineer-nextjs → setelah OpenAPI contract committed
        ↓
qa-engineer           → integration tests + UAT scripts + run report
        ↓
security-engineer     → BLOCKING review untuk M8 (seal + step-up MFA + audit trail)
        ↓
ifrs9-compliance-reviewer → BLOCKING gate untuk SEMUA modul ECL/EIR/staging
        ↓
devops-engineer       → (post-Phase 4 gate) deploy ke UAT
```

**Penting**: `ecl-eir-engineer` dan `backend-engineer-go` HARUS berbeda. Backend menulis infra (handler/repo/router), ECL-EIR menulis domain logic (`engine/`, `staging/`, `eir/`, `params/`, `lps/`, `lookthrough/`, `rollforward/`). Interface didefinisikan oleh system-analyst (OpenAPI + Go interface contracts).

---

## 5. Blocking Gates Per Modul

Semua 12 modul menyentuh ECL/EIR/staging → `ifrs9-compliance-reviewer` **BLOCKING** untuk semua. Namun tabel di bawah merinci kriteria spesifik per modul:

| Modul | Compliance Reviewer Criteria | Security Reviewer |
|---|---|---|
| **P4-M1** Staging | Verify SICR: ≥2 notch rating drop AND IG→non-IG AND DPD≥30 (any of, per DEC-011). Cure = 3 bulan consecutive (DEC-012). stage_history append-only (no hard-delete). | advisory |
| **P4-M2** PD/LGD/EAD | Verify Stage1=PD_12M, Stage2=PD_Lifetime, Stage3=PD=1.0 (DEC-010). LGD dari `lgd_basel` pool. EAD = outstanding + akrual + (undrawn × CCF). Multi-currency via BI JISDOR kurs. NUMERIC(10,8) throughout (DEC-016). No float64. | advisory |
| **P4-M3** LPS | Verify cap = IDR 2M per (nasabah, bank) per DEC-014. Aggregasi SEBELUM ECL. ECL = 0 for covered portion. | advisory |
| **P4-M4** Look-through | Verify komposisi underlying decompose by asset class (DEC-015). Weighted ECL = Σ(NAB × %class × ECL_class). `ecl.lookthrough_underlying` rows match decomposition. | advisory |
| **P4-M5** EIR solver | Verify IRR Newton-Raphson: tolerance=1e-10, max_iter=100, seed=kupon rate (DEC-013). Fail-safe jika non-convergence → explicit error (tidak return garbage). Day-count ACT/365. `eir_amortization_schedule` immutable (never UPDATE rows, insert new version). | advisory |
| **P4-M6** EIR amendment | Verify re-estimation trigger on contract modification: insert new schedule version (new `periode_seq` starting from amendment date), `effective_from` = tanggal amandemen, old rows tidak di-UPDATE. Catch-up adjustment = NPV difference. 4-eyes workflow. | advisory |
| **P4-M7** ECL engine | Verify formula kanonik: `ECL_skenario = EAD × PD_skenario × LGD`, `ECL_FL = ECL × impact_multiplier`, `ECL_weighted = Σ(ECL_FL × bobot)`. Stage 3 interest di Net Carrying (DEC-010). Parameter snapshot frozen at run time. NUMERIC(20,4) IDR, NUMERIC(10,8) rate. | advisory |
| **P4-M8** Calc run + seal | Verify seal = irreversible (no re-run over sealed). Step-up MFA untuk seal (DEC-027). ECL parameters tidak bisa diubah setelah sealed (`ECL_PARAM_FROZEN` 423). Audit `ECL_RUN.SEAL` di `aud.audit_log` in-transaction. | **BLOCKING** (seal + step-up MFA + audit trail) |
| **P4-M9** EIR + staging UI | Verify UI menampilkan schedule version history (tidak hide old versions). Staging timeline menampilkan semua SICR triggers lengkap dengan evidence. | advisory |
| **P4-M10** ECL run UI | Verify seal button = destructive dialog + MFA step-up UI (per DEC-027). Scenario breakdown tampilkan ECL per skenario + bobot. | advisory |
| **P4-M11** Roll-forward | Verify roll-forward formula: `ECL_closing = opening + originations − derecognitions ± transfers ± remeasurements`. Reconcile ke laporan posisi. | advisory |
| **P4-M12** QA final | Full E2E: instrument → SICR trigger → Stage 2 → ECL calc → sealed. EIR round-trip dari kupon → schedule → amortisasi. LPS cap applied. Look-through reksadana. Roll-forward reconcile. | **BLOCKING** (final review semua path) |

---

## 6. Decision Log Items — DEC-010..018

Semua DEC berikut **LOCKED** dan berlaku langsung untuk Phase 4. Tidak ada RFC yang perlu dibuka kecuali noted.

| DEC | Implikasi Phase 4 | RFC diperlukan? |
|---|---|---|
| **DEC-010** | ECL formula 3-stage × 3-skenario × dual FL. Default bobot Good/Normal/Bad = 0.25/0.50/0.25 (override via `bobot_skenario` ALCO-approved). `impact_pd.impact_multiplier` per skenario = FL multiplier. | Tidak — sudah locked. Namun **OQ-A** di bawah perlu konfirmasi FSD-APP-C tentang skenario NORMAL FL multiplier default = 1.0 (impact_mev_pd tidak punya row untuk NORMAL). |
| **DEC-011** | SICR triggers: rating ≥2 notch, IG→non-IG, DPD≥30 (any of). | Tidak. Perlu konfirmasi: apakah **notch scale** Pefindo explicitly defined di FSD-APP-C §3.1 — jika belum, ecl-eir-engineer perlu lookup Pefindo study. |
| **DEC-012** | Cure: 3 bulan consecutive. | Tidak — perlu definisi "bulan" (calendar atau periode_buku). OQ-B di bawah. |
| **DEC-013** | EIR Newton-Raphson: tolerance 1e-10, max 100 iter, presisi 8 desimal. | Tidak. |
| **DEC-014** | LPS cap = IDR 2M per nasabah per bank, applied SEBELUM ECL. | Tidak. |
| **DEC-015** | Look-through ECL Reksadana: decompose underlying. | Tidak. Perlu sumber data komposisi underlying (KSEI/fund-fact-sheet) — ini adalah input master yang harus ada sebelum look-through bisa berjalan. OQ-C di bawah. |
| **DEC-016** | NUMERIC precision: IDR `NUMERIC(20,4)`, FX `NUMERIC(20,8)`, PD/LGD/EIR `NUMERIC(10,8)`. No float64. shopspring/decimal Go. | Tidak. Enforce di schema-fix migrations. |
| **DEC-017** | Workflow: 4-eyes (EIR re-estimation), step-up MFA untuk seal (calc run + EIR approve). SoD maker≠reviewer≠approver. | Tidak. Lihat EIR amendment 4-eyes config. |
| **DEC-018** | Audit trail append-only, hash-chain, 10+10 tahun. `ecl.*` no hard delete. | Tidak. `ecl.stage_history` dan `ecl.calc_header` append-only. Trigger pengaman sudah ada di 0001 untuk `aud.audit_log` — perlu tambah trigger untuk `ecl.calc_header` no-UPDATE. |

---

## 7. Open Questions — Perlu Klarifikasi Sebelum Modul 1

Berikut adalah open questions yang HARUS dijawab sebelum (atau bersamaan dengan) dispatch P4-M1. Beberapa bisa di-assume default dan di-confirm kemudian oleh compliance reviewer.

| ID | Pertanyaan | Default assumsi jika belum dijawab | Butuh konfirmasi dari |
|---|---|---|---|
| **OQ-A** | Skenario NORMAL di ECL: apakah `impact_pd` per skenario atau satu flat multiplier? Tabel `mst.impact_pd` Phase 3 hanya punya satu multiplier per periode (bukan per skenario) — apakah multiplier ini di-apply ke semua 3 skenario sama, atau hanya NORMAL = flat, dan GOOD/BAD dapat dari `impact_mev_pd`? | Apply `impact_pd.impact_multiplier` flat ke semua 3 skenario. GOOD dan BAD juga punya `impact_mev_pd.impact_multiplier` yang di-multiply di atas. Resolusi OQ-2 Phase 3 (independent) → keduanya apply. | ifrs9-compliance-reviewer + ALCO |
| **OQ-B** | "3 bulan cure" — apakah 3 `mst.periode_buku` consecutive atau 3 calendar month? | 3 `mst.periode_buku` consecutive dengan `tipe_periode = 'BULANAN'`. | FSD-APP-C §3.2 |
| **OQ-C** | Komposisi underlying Reksadana: dari mana data ini diambil? Apakah ada `mst.*` tabel untuk fund composition (fund fact sheet), atau look-through memerlukan tabel baru (`mst.fund_composition`)? | Perlu tabel baru `mst.fund_composition` (data-modeler assess). Tidak ada tabel ini di schema 0001. | data-modeler + FSD-APP-C |
| **OQ-D** | Storage format `ecl.calc_header`: schema 0001 sudah ada `ecl.calc_header` dengan kolom aggregate (ecl_weighted_idr, ecl_fl_idr). Apakah detail per skenario cukup di `ecl.calc_detail_skenario` (ada di 0001), atau diperlukan kolom tambahan (mis. per-skenario FL-applied value)? | Schema 0001 sudah memadai. schema-fix mig 000026 hanya perlu audit cols + precision fix jika ada kolom NUMERIC yang salah. | system-analyst review 0001 schema vs FSD-APP-C |
| **OQ-E** | EAD: apakah `Committed_Undrawn × CCF` relevan untuk instrumen BLIPS (deposito, obligasi, saham, reksadana)? Undrawn commitment biasanya untuk kredit/fasilitas. | CCF = 0 untuk semua instrumen Phase 1 (deposito, obligasi, saham, reksadana tidak punya undrawn commitment). EAD = Outstanding + Accrued_Interest. | business-analyst + FSD-APP-C |
| **OQ-F** | Stage 3 net carrying interest: FSD-APP-C §3.2 konfirmasi Net Carrying Amount = Gross − ECL_allowance. Di `ecl.calc_header` tidak ada explicit `ecl_allowance_balance` kolom — dari mana nilai ini diambil saat accrual? | Dari `ecl.calc_header.ecl_fl_idr` (hasil calc run periode sebelumnya). Engine harus lookup calc run terbaru sebelum periode evaluasi. | FSD-APP-C §3.2 + ecl-eir-engineer |
| **OQ-G** | Instrumen dengan `klasifikasi_psak71 = 'FVTPL'` atau `'FVOCI_ELECTION'` (equity) — apakah masuk ECL calc run? | FVTPL = skip (no ECL per IFRS9). FVOCI equity election = skip (no ECL per IFRS9). Hanya AC + FVOCI debt yang di-ECL. | ifrs9-compliance-reviewer |
| **OQ-H** | Tipe instrumen scope untuk EIR: apakah EIR berlaku untuk semua tipe instrumen, atau hanya AC-classified? FVOCI debt — EIR dipakai untuk interest revenue, tapi amortisasi premium/diskonto ke P&L. | EIR berlaku untuk AC + FVOCI debt. FVTPL + FVOCI equity = no EIR. | FSD-APP-C §4.1 |
| **OQ-I** | Calc run granularity: per instrumen per periode atau per portofolio per periode? | Per instrumen per periode (sesuai `ecl.calc_header` schema: UNIQUE `(periode_id, instrumen_id, calc_run_id)`). | confirmed dari schema 0001 |

---

## 8. First Modul (P4-M1) — Siap Delegate

### Prompt untuk `business-analyst`

```
Tolong tulis user story + AC Gherkin untuk P4-M1: Staging Engine + SICR/Cure.

Konteks:
- Modul: APP-C ECL Engine, Phase 4
- Sumber: FSD-APP-C-ECL-EIR-v1.0.docx §3 (staging, SICR, cure), SoW_v1.4.docx §4, DEC-011 + DEC-012
- Schema existing: ecl.stage_history (000001 init_schema §9), mst.rating_history_counterparty, mst.instrumen.status
- Phase 3 sudah deliver: mst.pd_pefindo, mst.rating_history_counterparty, mst.instrumen (fully CRUD)

Cakupan story:
1. Story APP-C-ECL-001: Evaluasi SICR per instrumen — triggered on rating update
   - Actor: System (otomatis triggered setelah rating update di mst.rating_history_counterparty) + ROLE-RISK (manual trigger)
   - SICR triggers (any of): (a) rating drop ≥2 notch dari inisiasi, (b) IG→non-IG transition, (c) DPD ≥30 hari
   - Output: stage_history row (append-only), instrumen.stage updated
   - AC Gherkin: skenario GIVEN rating drop ≥2 notch WHEN eval triggered THEN Stage 1 → Stage 2

2. Story APP-C-ECL-002: Cure evaluation — Stage 2 → Stage 1
   - Actor: System (triggered setelah 3 consecutive mst.periode_buku BULANAN memenuhi cure criteria)
   - Cure criteria: tidak ada SICR trigger selama 3 periode berturut-turut
   - Output: stage_history row CURE, instrumen kembali Stage 1

3. Story APP-C-ECL-003: Stage 3 entry — credit default
   - Actor: System (triggered on DPD ≥90 hari atau rating default/D)
   - Output: stage_history row, PD override = 1.0 untuk ECL calc

4. Story APP-C-ECL-004: View staging history per instrumen
   - Actor: ROLE-RISK, ROLE-AUDIT
   - AC: DataTable (sort/filter/export UX §1), filter by stage, date, trigger type

Output yang diminta:
- `docs/stories/APP-C-ECL-001-staging-sicr.md` sampai `APP-C-ECL-004`
- AC Gherkin minimal 3 skenario per story (happy path, SoD/permission, edge case)
- Open questions ke orchestrator bila ada ambiguitas FSD (khususnya OQ-B: definisi "3 bulan cure")

Handoff berikutnya: system-analyst (OpenAPI + state machine staging transitions + validation rules)
```

### Setelah business-analyst — prompt untuk `system-analyst`

```
Tolong buat OpenAPI contract + state machine untuk P4-M1 staging engine.

Input: stories APP-C-ECL-001..004 dari business-analyst
Sumber tambahan: FSD-APP-C §3, ecl.stage_history schema (000001 §9)

Deliverables:
1. api/openapi/ecl-staging.yaml — endpoint:
   - GET /ecl/staging/current/{instrumen_id}
   - GET /ecl/staging/history/{instrumen_id}
   - POST /ecl/staging/evaluate/{instrumen_id} (manual trigger, ROLE-RISK)
2. State machine diagram staging (Stage_1, Stage_2, Stage_3) + valid transitions + triggers
3. Validation rules per endpoint (error codes, field constraints)
4. Go interface contract untuk ecl-eir-engineer:
   interface StagingService {
     EvaluateSICR(ctx, instrumenID, asOfDate) (StageTransition?, error)
     EvaluateCure(ctx, instrumenID, periodeID) (StageTransition?, error)
     GetCurrentStage(ctx, instrumenID) (StageSnapshot, error)
     GetHistory(ctx, instrumenID, query) ([]StageHistory, Pagination, error)
   }

Pastikan: error codes kanonik (api-conventions.md), cursor pagination, Idempotency-Key
```

### Setelah system-analyst — prompt untuk `data-modeler`

```
Tolong buat migration 000022 untuk Phase 4 P4-M1 staging engine.

Schema target: ecl.stage_history (existing di 000001 §9) perlu:
- ADD missing audit cols: created_by, updated_at, updated_by, deleted_at, deleted_by, row_version, tenant_id
- ADD workflow_status kalau diperlukan untuk manual review flow
- Precision check: semua NUMERIC kolom sesuai DEC-016
- Indexes: (tenant_id, instrumen_id, tanggal_migrasi DESC), (trigger_type), (stage_sesudah, tanggal_migrasi)
- Assess apakah perlu tabel baru untuk mst.fund_composition (OQ-C) — bila ya, include di migration ini

Tag file:
-- migration: 0022 ecl_staging_schema_fix
-- author: data-modeler
-- requires: 0001, 0021
```

### Sequence P4-M1 → M2

Setelah P4-M1 system-analyst selesai (OpenAPI + interface contract), dispatch **ecl-eir-engineer** untuk implementasi `internal/ecl/staging`:

```
Tolong implementasi P4-M1 staging engine di backend/internal/ecl/staging/.

Input:
- OpenAPI: api/openapi/ecl-staging.yaml
- Interface: StagingService (dari system-analyst output)
- Schema: ecl.stage_history (000001 + migration 000022)
- Master dependencies: mst.rating_history_counterparty (existing Phase 3), mst.instrumen

Aturan keras (dari locked decisions):
- DEC-011: SICR = rating drop ≥2 notch OR IG→non-IG OR DPD≥30 (any of)
- DEC-012: Cure = 3 mst.periode_buku BULANAN consecutive tanpa SICR trigger
- DEC-016: NUMERIC(10,8) untuk semua rate, shopspring/decimal (no float64)
- DEC-018: stage_history append-only, no UPDATE/DELETE

Package: backend/internal/ecl/staging/
Files: domain.go, repo.go, service.go, handler.go, routes.go
Tests: unit tests (service) + integration hook markers (jangan butuh Docker)

Handoff setelah selesai: backend-engineer-go (HTTP handler wiring + Gin routing),
lalu ecl-eir-engineer langsung lanjut P4-M2 PD/LGD/EAD helpers.
```

---

## 9. Sequencing & PR Strategy

- Branch naming: `feature/app-c-{slug}` dari `develop`
- Satu PR per modul (M1..M12), squash and merge ke `develop`
- Compliance + security review di PR body sebelum merge
- Migration numbering: 000022..000027 (M1→M7, satu migration per modul schema-fix)
- Jika OQ-C menghasilkan tabel baru (`mst.fund_composition`), migration 000022 bisa include atau pisah 000028

### PR order (blocking dependency)
```
PR-M1 → PR-M2 → PR-M3 → (PR-M4 ‖ PR-M5) → PR-M6 → PR-M7 → PR-M8
→ (PR-M9 ‖ PR-M10) → PR-M11 → PR-M12
```

---

## 10. Risk & Rollback

| Risk | Severity | Mitigasi |
|---|---|---|
| OQ-A unresolved: FL multiplier semantik salah → ECL over/under-stated | HIGH | Default flat apply; compliance reviewer MANDATORY verify sebelum M7 merge |
| OQ-C: tidak ada fund composition master data → look-through berjalan dengan data kosong | HIGH | Block M4 sampai `mst.fund_composition` tabel + seed data ada; jangan merge M4 tanpa data |
| Stage 3 net carrying: lookup calc run sebelumnya bisa miss jika calc belum pernah run | MEDIUM | Fallback: first-run → gross carrying = net carrying (ECL allowance = 0). Document di code. |
| EIR non-convergence di IRR solver: return garbage silently | HIGH | ecl-eir-engineer: explicit error on non-convergence, test convergence edge cases (zero coupon, par bond) |
| `ecl.calc_header` di-UPDATE setelah sealed | HIGH | Add DB trigger `fn_ecl_calc_no_modify_when_sealed` (seperti `fn_audit_no_modify`) di migration 000026 |
| Performance: ECL calc run untuk semua instrumen bisa timeout | MEDIUM | Asynq worker (P4-M8), batch per 100 instruments, progress reporting, cancellation support |
| float64 slip di ecl-eir-engineer | HIGH | golangci-lint rule `forbidigo` + `go vet`; compliance reviewer verify semua NUMERIC via decimal |
| Seal endpoint tanpa MFA step-up | BLOCKING | security-engineer verifikasi M8 sebelum merge |

### Rollback plan
- Setiap migration `000022..000027` memiliki `.down.sql` idempotent
- `ecl.*` data tidak di-hard-delete — rollback schema revertible tanpa data loss
- Calc run unsealing: tidak ada; sealed = permanent (by design DEC-027). Rollback = create new calc run untuk periode yang sama dengan updated params, seal yang baru menjadi "active"

---

## 11. Verifikasi (Gate 4→5)

Sebelum Phase 5 boleh dimulai:
- [ ] Semua 12 modul P4-M1..M12 merged ke `develop` + CI hijau
- [ ] `ifrs9-compliance-reviewer` VERDICT = PASS untuk setiap modul
- [ ] M8 security review = PASS (seal + step-up MFA)
- [ ] Integration test full cycle: instrumen → SICR → Stage 2 → ECL calc (3-skenario × dual FL) → seal
- [ ] EIR round-trip: dari kupon + cashflow schedule → IRR solver → amortization schedule → `trx.amortisasi` rows
- [ ] LPS excess calc verified: instrument deposito nasabah X di bank Y > 2M IDR → ECL hanya untuk excess
- [ ] Look-through Reksadana: komposisi 2 underlying → weighted ECL = expected value
- [ ] Roll-forward reconcile: ECL_closing = opening + movements (delta < IDR 1)
- [ ] UX rules semua terpenuhi: list = sort+filter+cursor+export, form notif, JobProgressPanel untuk calc run
- [ ] OQ-A..OQ-I semua dijawab dan dicatat di plan ini (update §7)
- [ ] `pnpm -C web build && pnpm -C web lint` hijau
- [ ] `go build ./... && golangci-lint run && go test ./...` hijau

---

## Lampiran — Commit Convention Phase 4

Branch: `feature/app-c-{slug}` (mis. `feature/app-c-staging-sicr`)

Expected commit scopes:
```
feat(app-c): implement staging engine SICR/cure (P4-M1)
feat(app-c): add PD/LGD/EAD lookup helpers (P4-M2)
feat(app-c): implement LPS aggregator service (P4-M3)
feat(app-c): implement look-through ECL for Reksadana (P4-M4)
feat(app-c): implement EIR Newton-Raphson solver + schedule (P4-M5)
feat(app-c): implement EIR amendment re-estimation workflow (P4-M6)
feat(app-c): implement ECL core calculation engine 3-stage (P4-M7)
feat(app-c): implement ECL calc run Asynq worker + seal (P4-M8)
feat(db): migration 000022 ecl staging schema-fix
feat(db): migration 000023 ecl lookthrough schema-fix
feat(db): migration 000024 ecl eir schedule schema-fix
feat(db): migration 000025 ecl eir reestimation schema-fix
feat(db): migration 000026 ecl calc header schema-fix + sealed trigger
feat(web): add ECL staging history screens (P4-M9)
feat(web): add EIR schedule + re-estimation screens (P4-M9)
feat(web): add ECL calc run trigger + progress + seal UI (P4-M10)
feat(app-c): implement roll-forward CKPN read service (P4-M11)
test(app-c): add integration tests P4-M1..M11 full cycle (P4-M12)
```

---

_Dokumen ini diupdate seiring OQ-A..I dijawab dan modul-modul merged._
