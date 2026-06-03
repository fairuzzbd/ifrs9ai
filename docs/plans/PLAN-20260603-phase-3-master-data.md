# PLAN-20260603 — Phase 3 Master Data Module (APP-A)

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-03
**Sumber**: `START_HERE.md` §5 Phase 3 + §7 Gate 3→4 · `docs/FSD-APP-A-MasterData-SPPI-BM-v1.1.md` · ERD/`000001_init_schema.up.sql`
**Klasifikasi**: feature (modul) — sebagian menyentuh **path regulated** (ECL parameter masters) + **PII** (counterparty)
**Target**: 3 minggu

---

## 1. Goal
Master Data CRUD untuk 16 tabel `mst.*` (APP-A), tiap modul: **CRUD endpoint + workflow approval + UI screen + audit trail + test**. Exit = Gate 3→4: semua master CRUD passing, CoA + Mapping Jurnal terbentuk dari sample, periode buku 2026 generated.

## 2. Decision Log check
- **DEC-010/014/015** ECL params (PD/LGD/scenario weights/LPS) — masters `pd_pefindo`, `lgd_basel`, `bobot_skenario`, `lps_coverage`, `impact_mev_pd`, `impact_pd` adalah **parameter ECL** → approval **ALCO** (DEC, persona ALCO) + **ifrs9-compliance-reviewer** gate.
- **DEC-017** workflow 4-eyes (master umum) / **6-eyes** untuk parameter master + klasifikasi → pakai workflow engine Phase 2 (config-driven, `sys.config WORKFLOW_CONFIG_*`).
- **DEC-021/022** Idempotency-Key + cursor pagination → middleware Phase 2.
- **DEC-028** PII column-level encrypt: `counterparty.{npwp,nomor_rekening,ktp}` via `sec.encrypt/decrypt` (migration 0003).
- **UX rules** (CLAUDE.md): tiap list = sort+page+filter+export; tiap form = notif sukses/gagal; long-process (upload Pefindo/CoA import, JISDOR job) = progress.

## 3. Prasyarat / blocking
- **PR #10 (Phase 2 foundation) HARUS merge ke `develop`** sebelum build Phase 3 (butuh auth/RBAC/audit/workflow/idempotency/DataTable). ⚠️ PR #10 `REVIEW_REQUIRED` — nunggu human approval.
- **Discovery (business-analyst + system-analyst + uiux-designer) BISA jalan paralel sekarang** (produk = story/kontrak/desain, tidak butuh kode merged).
- Schema `mst.*` (16 tabel) **sudah ada** di 0001 → data-modeler kerja minimal: seed `impact_mev_pd`/`impact_pd` (debt #4), sample `instrumen` (debt #5, ke fixture), kemungkinan index/constraint tuning.

## 4. Strategi: generic pattern dulu, lalu replikasi
Bangun **1 pola generik** master-data (CRUD + list DataTable + workflow + audit + export) lewat modul paling simpel, lalu replikasi ke 15 sisanya. Urutan (per START_HERE §5):
1. `mata_uang` (paling simple) ← **pilot pola**
2. `periode_buku` (ada seed) + generate 2026
3. `lgd_basel`, `bobot_skenario`, `lps_coverage` (static reference) — **ECL param → ALCO + compliance**
4. `pd_pefindo` (+ upload workflow XLSX/CSV) — **ECL param → ALCO + compliance + integration-engineer (feed)**
5. `counterparty` + `rating_history` — **PII → security-engineer**
6. `chart_of_accounts` (+ import Excel) — long-process import
7. `mapping_jurnal_header` + `detail`
8. `portofolio`
9. `instrumen` (paling kompleks — FK ke banyak master)
10. `kurs` + BI JISDOR scheduled job — **integration-engineer**
(+`impact_mev_pd`/`impact_pd` ECL param)

## 5. Agents + handoff (per modul)
```
business-analyst    → story + AC (per modul / kelompok modul)
   ↓
system-analyst      → OpenAPI CRUD + workflow + import/upload contract + validation rules
   ↓
data-modeler        → hanya jika perlu (seed impact_pd, index, constraint) — schema mst.* sudah ada
   ↓
uiux-designer       → desain screen (form ergonomics master, DataTable, approval panel) [paralel backend]
   ↓
backend-engineer-go → CRUD service + repo + workflow wiring + audit + idempotency (reuse Phase 2)
integration-engineer→ feed: Pefindo upload, CoA Excel import, BI JISDOR job (modul terkait)
   ↓
frontend-engineer-nextjs → screen (DataTable §1, form notif §2, progress §3) setelah kontrak fix
   ↓
qa-engineer         → CRUD + workflow + SoD + import test + UAT script; run report
   ↓
security-engineer   → BLOCKING review untuk counterparty PII (encrypt npwp/no_rek/ktp) + endpoint baru
   ↓
ifrs9-compliance-reviewer → BLOCKING GATE untuk ECL-param masters (pd_pefindo, lgd_basel, bobot_skenario, lps_coverage, impact_*) — cek kalibrasi PD vs Pefindo study, weights 0.25/0.50/0.25, ALCO approval workflow
```

## 6. Gate per modul (siapa BLOCKING)
| Modul | security (PII) | compliance (ECL param) | catatan |
|---|---|---|---|
| mata_uang, periode_buku, portofolio, CoA, mapping_jurnal, kurs, instrumen | — | — | CRUD standar + workflow |
| counterparty + rating_history | ✅ BLOCKING | — | PII encrypt DEC-028 |
| pd_pefindo, lgd_basel, bobot_skenario, lps_coverage, impact_mev_pd, impact_pd | — | ✅ BLOCKING | ALCO approval + kalibrasi |
| kurs (JISDOR job), pd_pefindo (upload) | — | (pd: ✅) | integration feed + progress UX |

## 7. Risk + rollback
| Risk | Mitigasi |
|---|---|
| Build Phase 3 di atas foundation belum merged | Tunggu PR #10 merge; discovery jalan paralel |
| ECL param masuk tanpa ALCO/compliance | compliance-reviewer BLOCKING + workflow 6-eyes config |
| PII counterparty plaintext | security BLOCKING + sec.encrypt/decrypt |
| Pola CRUD tidak konsisten antar 16 modul | pilot `mata_uang` jadi template; review pola sebelum replikasi |
| Excel/XLSX import unsafe (formula injection, besar) | integration validasi + async job + progress (UX §3) |

## 8. Verifikasi (Gate 3→4)
- [ ] Semua master CRUD passing (16 tabel) + workflow approve/reject + audit
- [ ] CoA + Mapping Jurnal terbentuk dari sample data
- [ ] Periode buku 2026 generated
- [ ] List = sort+page+filter+export; form = notif; import/job = progress (UX rules)
- [ ] counterparty PII encrypted (security sign-off)
- [ ] ECL-param masters: compliance sign-off + ALCO approval workflow
- [ ] qa run report + `go build/lint/test` hijau + `pnpm -C web build/lint` hijau

## 9. Sequencing & PR
- Branch dari `develop` (setelah PR #10 merge). PR per modul/kelompok (kecil, reviewable).
- Scope commit: `feat(app-a): ...`, `feat(web): ...`, `feat(integ): ...`.

## 10. Status (update 2026-06-03)
- [x] Plan + decision-log check + scope
- [x] **business-analyst** — story pola generik (`docs/stories/APP-A-MSTR-001`) + pilot `mata_uang` (`APP-A-MSTR-002`) + tabel varian (ECL-param/PII/import)
- [x] **system-analyst** — kontrak `api/openapi/{master-common,mata-uang}.yaml` + state machine + validation + **role matrix 16 modul** (eyes/maker/reviewer/approver/gate)
- [x] **uiux-designer** — desain screen master-data (`docs/ux/master-data/` 7 dok: wireframe list/form/detail-workflow/varian + interaction flow + tokens + handoff) — pola generik + 3 varian final untuk semua 16 modul
- [ ] **(BLOCKING) PR #10 merge ke develop** — human approval; SEMUA build Phase 3 nunggu ini
- [ ] data-modeler — migration `000008` (mata_uang schema fix + surrogate id UUID + seed WORKFLOW_CONFIG Phase 3 + impact_* seed). **SETELAH PR #10 merge**, di branch `feature/phase-3-master-data` dari develop.
- [ ] backend + frontend (pilot mata_uang) → qa → replikasi 15 modul
- [ ] compliance gate (ECL-param masters) · security gate (counterparty PII)

### Governance catch (orchestrator)
- system-analyst sempat mengedit `db/migrations/000007` (domain data-modeler + sudah committed di PR #10) → **di-revert**. Seed WORKFLOW_CONFIG Phase 3 dipindah ke `000008` (data-modeler, post-merge).

### Open questions → stakeholder (sebelum build)
- OQ-1: ketersediaan 3 user distinct untuk SoD master umum (mata_uang) — rekomendasi tetap 4-eyes 3 user.
- OQ-2: email notif Phase 3 wajib atau in-app cukup.
- OQ-3: `rating_history` maker=reviewer=ROLE-RISK → SoD self-violation; perlu sub-role RISK (junior/senior) atau maker=ROLE-MAKER-TR.

### Discovery artifacts (uncommitted)
- Saat ini di working tree branch feature/phase-2-foundation (TIDAK di-commit ke PR #10 — bukan scope Phase 2). Akan di-commit ke `feature/phase-3-master-data` (dari develop) setelah PR #10 merge.
