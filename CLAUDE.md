# BLIPS IFRS9 — Claude Code Project Guide

Repository ini adalah implementasi BLIPS (Bond, Loan, Investment Portfolio System) untuk Tugu Reasuransi, sesuai PSAK 71 / IFRS 9. Project ini dikelola oleh tim **14 subagent Claude Code** dengan pembagian peran yang ketat.

## Konvensi project (read this first)

### Stack (LOCKED — lihat `BLIPS_Decision_Log_v1.0.docx`)
- **Backend**: Go 1.22+, Gin, GORM v2 + sqlx, Asynq, golang-migrate
- **Frontend**: Next.js 14+ (App Router, TS strict), shadcn/ui, React Hook Form + Zod, Zustand, TanStack Query, Recharts
- **DB**: PostgreSQL 18 (+ read replica), Redis 7+
- **Storage**: MinIO (S3-compatible, on-prem)
- **Auth**: Keycloak (SAML 2.0/OIDC) + LDAP federation
- **Infra**: Traefik, Docker Compose (dev/UAT), Kubernetes on-prem (prod), Terraform + Ansible, GitLab CI
- **Observability**: Prometheus + Grafana + Loki

### Dokumen sumber kebenaran
Sumber primer (urutan prioritas saat konflik):
1. `BLIPS_Decision_Log_v1.0.docx` — locked decisions (refuse to reopen)
2. `FSD-BLIPS-MASTER-v1.1.docx` — master FSD
3. `FSD-APP-A/B/C/D/E-*.docx` — per modul
4. `ERD-BLIPS-IFRS9-v1.2.docx` + `BLIPS_init_schema.sql`
5. `SoW_v1.4.docx` — formula + field lists
6. `BRD_BLIPS_IFRS9_v1.1.docx` — stakeholder intent + RACI
7. `Pefindo_Annual_Default_Study_2007-2025_EN.pdf` — kalibrasi PD

### Modul
- **APP-A** Master Data + SPPI Test + Business Model (klasifikasi PSAK 71)
- **APP-B** Transaction Lifecycle (penempatan, MTM, renewal, jatuh tempo)
- **APP-C** ECL Engine + EIR (compliance core)
- **APP-D** Periode Buku + FX + Mapping Jurnal & GL
- **APP-E** Reporting & Dashboard (25+ laporan)

### Schema DB (9 namespaces)
`mst` · `trx` · `ecl` · `sppi` · `doc` · `jrnl` · `aud` · `sec` · `sys` (+`rpt` materialized views)

### Aturan keras
- **No hard delete** in `aud`, `jrnl`, `ecl`. Soft-delete (`deleted_at`) elsewhere.
- **No `float64`** for money/rates — `shopspring/decimal` always, 8 decimal places.
- **Audit trail** append-only, hash-chain optional, retention 10+10 thn.
- **SoD**: `maker_id ≠ reviewer_id ≠ approver_id` enforced server-side.
- **MFA mandatory** untuk CEO, CFO, ROLE-KOMITE, ROLE-ALCO, Treasury Manager, Finance Controller.
- **Idempotency-Key** wajib di setiap endpoint mutating.

### UX & feedback rules (cross-cutting, mandatory)

Berlaku untuk **setiap** screen, endpoint, dan worker. Pelanggaran = MR rejected oleh `uiux-designer` atau `qa-engineer`. Implementasi detail di @.claude/memory/ux-patterns.md.

#### 1. List / tabel data wajib punya: sort + paging + filter + export

Setiap list endpoint (`GET /api/v1/{resource}`) dan setiap tabel di frontend WAJIB menyediakan keempat fitur. Tidak ada list "polos".

- **Sort** — multi-column. Query `?sort=col1:asc,col2:desc`. Header click toggle asc→desc→none. Icon indicator wajib.
- **Paging** — cursor-based (`?cursor=...&limit=50`). UI: total estimasi, "Page X of ~Y", Prev/Next. Default 50, max 200.
- **Filter** — text search global + filter per kolom relevant. Query `?q=...&filter[col]=val`. UI: filter chip + clear-all. State di URL (deep-link friendly).
- **Export** — minimum CSV + XLSX. Respect filter+sort aktif. Dataset > 10k row → async export ke MinIO + notif download link (rule #3). Audit `{ENTITY}.EXPORT` setiap export.

#### 2. Form submission wajib notifikasi sukses atau gagal

Setiap form (create/update/submit/approve/reject/upload/config) WAJIB feedback eksplisit. Tidak boleh diam.

- **Sukses** — toast hijau top-right, 4 detik, message spesifik (bukan "Berhasil"). Contoh: `"Instrumen INST-001234 berhasil dibuat. Menunggu review."` + link ke entity baru.
- **Gagal** — toast merah persistent (manual close). Tampilkan error code + message + traceId. Validation error: highlight field + inline message (`aria-describedby`).
- **Pending** — disable submit button + spinner inline. Block double-submit (idempotency).
- **Destructive** (delete/reject/hard-close/seal) — confirm dialog dulu, lalu notif setelah selesai. MFA step-up jika diperlukan.

#### 3. Long-running process wajib progress notification

Operasi > 2 detik WAJIB tampilkan progress. User tidak boleh "menunggu buta". Cakupan: ECL calc run, EIR re-estimation, file upload feeds (Pefindo/IBPA/KSEI), export besar, MV refresh, batch jurnal posting.

- **Backend** — Asynq job. Submit endpoint return `202 { jobId, statusUrl, streamUrl }`. Status endpoint `GET /api/v1/jobs/{jobId}` return `{status, progress 0-100, currentStep, ETA, result|error, canCancel}`. SSE stream untuk live update.
- **Frontend** — komponen `<JobProgressPanel>` subscribe via SSE (fallback polling 2s). Tampilkan: progress bar, ETA, current step, cancel button. Background mode: user lanjut kerja, global notif badge nyala saat selesai.
- **Selesai** — toast sukses + link ke result (download file, view calc run).
- **Gagal** — toast error persistent + link ke detail (retry button jika applicable).
- **Job history** — page `/jobs` (pakai DataTable pattern §1).

## Project memory (auto-loaded ke konteks)

Memory files berisi domain knowledge & konvensi yang dipakai berulang oleh semua agent. Diimport via `@`:

- @.claude/memory/glossary.md — IFRS9/PSAK 71 + istilah BLIPS
- @.claude/memory/formulas.md — ECL & EIR formula reference
- @.claude/memory/locked-decisions.md — quick ref Decision Log
- @.claude/memory/api-conventions.md — REST + error codes + pagination + auth
- @.claude/memory/db-conventions.md — schema rules, types, migration discipline
- @.claude/memory/security-baseline.md — auth chain, MFA, encryption, audit
- @.claude/memory/personas.md — 10 RBAC roles + permission matrix
- @.claude/memory/git-conventions.md — GitFlow branching, Conventional Commits, signed commits, release workflow
- @.claude/memory/ux-patterns.md — list (sort/page/filter/export), form notif, long-process progress

## Tim subagent

14 agent terpasang di `.claude/agents/`. Lihat `.claude/AGENT-TEAM.md` untuk peran, handoff order, dan decision rights.

### Quick reference
| Squad | Agents |
|---|---|
| **Discovery & Design** | business-analyst · system-analyst · data-modeler · uiux-designer |
| **Build — Backend** | backend-engineer-go · ecl-eir-engineer · integration-engineer |
| **Build — Frontend** | frontend-engineer-nextjs |
| **Quality & Compliance** | qa-engineer · security-engineer · ifrs9-compliance-reviewer |
| **Platform** | devops-engineer |
| **Orchestration** | tech-lead-orchestrator |
| **Governance / Oversight** | mda (entry gate + Auditor Tertinggi — default agent diload pertama; delegasi ke tech-lead-orchestrator + devops-engineer langsung untuk ops infra/git/CI) |

### Standard flow untuk perubahan
```
user request
  → mda (ENTRY GATE — default agent, diload pertama)
       · nilai terhadap dokumen + locked decisions
       · putuskan: APPROVED / REJECTED / NEED_HUMAN
       · catat ke ledger (.claude/memory/mda-ledger.md)
       · jika REJECTED / NEED_HUMAN → balik ke user (stop)
       · jika APPROVED → pilih channel hilir:
            ├─ [perubahan fungsional/regulated] → tech-lead-orchestrator
            └─ [ops infra/git/CI/deploy murni]  → devops-engineer (langsung)
  → tech-lead-orchestrator (untuk perubahan fungsional; plan + delegate; boleh lapor balik ke mda)
  → business-analyst (story + AC)
  → system-analyst (OpenAPI + state machine)
  → data-modeler (if schema)
  → uiux-designer (parallel)
  → backend-engineer-go / ecl-eir-engineer / integration-engineer
  → frontend-engineer-nextjs
  → qa-engineer (tests + UAT)
  → security-engineer (review)
  → ifrs9-compliance-reviewer (GATE for ECL/EIR/SPPI/BM)
  → devops-engineer (deploy)
```

### Blocking veto rights
- `ifrs9-compliance-reviewer` — BLOCKING veto untuk merge yang menyentuh ECL/EIR/SPPI/BM/klasifikasi.
- `security-engineer` — BLOCKING veto untuk auth/PII/audit changes.

### Governance layer (mda) — entry gate
- `mda` adalah **Auditor Tertinggi + gerbang masuk (entry gate)**. Ia di-set sebagai **default agent yang diload pertama** via `.claude/settings.json` → `"agent": "mda"` (main thread sesi = MDA).
- **Tiap request user lewat MDA dulu**: MDA membaca dokumen referensi/regulasi + locked decisions, memutuskan (`APPROVED`/`REJECTED`/`NEED_HUMAN`), mencatat ledger, lalu — bila `APPROVED` — mendelegasikan ke `tech-lead-orchestrator` via Task. Bila `REJECTED`/`NEED_HUMAN`, MDA stop di gerbang dan balas ke user (eskalasi bila perlu).
- **Dual downstream channel**: MDA boleh memanggil **dua** agent hilir, dan hanya dua — (1) `tech-lead-orchestrator` (default, untuk SEMUA perubahan fungsional/regulated; orchestrator yang fan-out ke subagent build/quality), dan (2) `devops-engineer` **langsung** (KHUSUS ops murni infra/git/CI/branch-protection/deploy/observability yang tidak menyentuh kode aplikasi atau domain regulated). Subagent build/quality lain (BA, SA, data-modeler, builders, QA, security, compliance) **tidak pernah** dipanggil MDA langsung — selalu lewat orchestrator. Keduanya boleh lapor balik ke MDA untuk keputusan strategis di tengah eksekusi. **Guard SoD**: MDA tetap memutuskan + mencatat ledger sebelum dispatch; eksekusi git/deploy tetap di tangan `devops-engineer`, bukan MDA sendiri.
- **Batas tool MDA**: `Bash` hanya untuk read-only situational awareness (mis. `git status`/`ls`), bukan build/test/deploy. `Write`/`Edit` hanya untuk ledger. `Task` untuk memanggil `tech-lead-orchestrator` atau `devops-engineer` (dua channel itu saja).
- `mda` **tidak menimpa** veto BLOCKING `ifrs9-compliance-reviewer` / `security-engineer`; ia menilai di lapisan keputusan strategis, bukan menggantikan gate teknis.
- **Ledger wajib**: setiap keputusan MDA (dari gate maupun laporan balik orchestrator) disimpan MDA ke `.claude/memory/mda-ledger.md` (append-only, satu entri per exchange). File ini sengaja **tidak** di-`@`-import (agar tidak membengkakkan context); dibaca on-demand. Hanya MDA yang menulis; orchestrator boleh membaca.

## Slash commands (.claude/commands/)

Shortcut prompt untuk trigger workflow agent. Pakai `/` di Claude Code:

| Command | Untuk |
|---|---|
| `/plan <request>` | tech-lead-orchestrator → bikin plan + delegasi |
| `/story <topik>` | business-analyst → user story dengan AC Gherkin |
| `/api <input>` | system-analyst → OpenAPI fragment + state machine |
| `/migration <perubahan>` | data-modeler → golang-migrate up/down |
| `/ecl <task>` | ecl-eir-engineer → implementasi ECL/EIR (compliance core) |
| `/integration <sistem>` | integration-engineer → adapter + runbook |
| `/ui <screen>` | uiux-designer → frontend-engineer-nextjs (2-tahap) |
| `/uat <story>` | qa-engineer → UAT script + tests |
| `/security <scope>` | security-engineer → checklist + remediation |
| `/compliance <scope>` | ifrs9-compliance-reviewer → VERDICT gate |
| `/deploy <env>` | devops-engineer → IaC + pipeline + runbook |
| `/release <version>` | Generate release notes + SemVer bump + signed tag |
| `/standup <range>` | cross-module status dari git + plans |

## Skills (.claude/skills/)

Reference yang dipakai berulang oleh agent. Di-load saat dibutuhkan:

| Skill | Isi |
|---|---|
| `psak71-classifier` | Matrix lookup SPPI × BM → AC/FVOCI/FVTPL klasifikasi |
| `ecl-formula` | Reference implementation ECL (3-stage × 3-skenario × dual FL), LPS, look-through |
| `eir-newton-raphson` | IRR solver Go implementation, amortisasi schedule, amendment versioning |
| `migration-scaffold` | golang-migrate template (audit cols, partitioning, indexes) |
| `audit-trail-template` | `aud.audit_log` row writing pattern + hash chain |
| `compliance-checklist` | IFRS9 reviewer checklist runner + verdict format |

## Cara memulai task baru

Sebagai user (atau main Claude), mulai dengan slash command yang sesuai:

```
/plan Implementasi user story X di modul APP-B (penempatan deposito dengan workflow Maker-Reviewer-Approver)
```

Atau panggil agent langsung untuk task kecil:

```
@business-analyst tolong tulis user story untuk amandemen kontrak deposito
```

Untuk perubahan yang menyentuh ECL/EIR/SPPI/BM/klasifikasi/audit → **selalu** lewat orchestrator + compliance reviewer.

## Bahasa
Primary: Bahasa Indonesia (label UI, docs internal). Secondary: English (technical terms, code identifiers, exported reports).

## Run lokal
```bash
# Dev stack
docker compose -f deploy/docker-compose.dev.yml up -d
# Migrate
go run ./cmd/migrator up
# Backend
go run ./cmd/api
# Frontend
cd web && pnpm dev
```

Untuk runbook operasional lengkap, lihat `docs/runbooks/`.

## Git workflow

- **Branching**: GitFlow (`main` ← `release/*` ← `develop` ← `feature/*`, plus `hotfix/*` ← `main`)
- **Commits**: Conventional Commits dengan scope BLIPS (`feat(app-c):`, `fix(sec):`, dst)
- **Tags**: SemVer (`v1.4.0`), signed (GPG/SSH)
- **Protected branches**: `main`, `develop`, `release/*`, `hotfix/*` — no direct push, signed commits required
- **MR template**: `.gitlab/merge_request_templates/default.md` + `hotfix.md`

Full convention: @.claude/memory/git-conventions.md
