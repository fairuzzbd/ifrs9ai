# RESUME — Phase 3 Master Data (handoff sesi berikutnya)

**Ditulis**: 2026-06-03 oleh tech-lead-orchestrator
**Alasan**: sesi terhenti karena tool `Bash` + `Agent` (dispatch subagent) di-disable oleh harness (kemungkinan terkait session limit, reset ~12:00 Asia/Jakarta). Tidak ada kerja hilang — semua penting sudah ter-merge ke `develop`.

> Catatan: file ini kemungkinan UNCOMMITTED (ditulis saat Bash/git tidak tersedia). Commit-kan saat tool aktif.

---

## 1. State repo (per akhir sesi)
- `origin/develop` = `b338d14` — sudah berisi:
  - **PR #10** Phase 2 Foundation (auth/RBAC/SoD, audit hash-chain, workflow engine config-driven, notification, document, common middleware, cmd/migrator, cmd/audit-verify).
  - **PR #11** Phase 3 discovery + pilot `mata_uang` full-stack.
- Branch lokal `feature/phase-3-periode-buku` = di-reset ke `b338d14` (base bersih, BELUM ada kerja, BELUM di-push).
- **P0-1/P0-2/P0-3 semua RESOLVED.** CI hijau di PR #10 & #11.

### ⚠️ Jebakan yang sudah ketemu (hindari)
- **Local `develop` DIVERGED** dari `origin/develop`. JANGAN branch dari local develop. Selalu:
  `git fetch origin && git checkout -b feature/<x> origin/develop` (atau `git reset --hard origin/develop`).
- **golangci-lint** dipakai untuk verify lokal diinstall di `/tmp/glci/golangci-lint` **v1.59.1** (mungkin TIDAK persist antar sesi/container — reinstall bila perlu: `curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b /tmp/glci v1.59.1`).
- **CI toolchain**: go.mod `go 1.22.0`, golangci-lint `v1.59.1` (ci.yml). `.golangci.yml` v1.59 schema (skip-dirs/files ada di `issues.exclude-*`, misspell `ignore-words` bilingual). JANGAN turunkan.
- **docker no-access** di env dev → integration test (`-tags=integration`) hanya jalan di CI. Coverage repo-layer ditutup di sana.
- **Anti-pattern**: orchestrator TIDAK menulis DDL/domain code sendiri. DDL→data-modeler, CRUD→backend-engineer-go, screen→frontend-engineer-nextjs.

---

## 2. Pola pilot `mata_uang` (TEMPLATE replikasi)
Reuse untuk 15 modul mst.* lain:
- **DB**: pola `db/migrations/000008_mata_uang_schema_fix.up.sql` — ADD audit cols yang kurang + `workflow_status` + (kalau PK non-UUID) `id UUID` surrogate. Cek 0001 dulu (mst.* sering kurang audit cols).
- **Backend**: `backend/internal/master/matauang/{domain,repo,service,handler,routes}.go` — pola reusable (listquery sort/filter/cursor, soft-delete, CSV/XLSX export, workflow signing reuse Phase 2, audit same-tx, idempotency). Wire di `cmd/api/main.go`.
- **Frontend**: `frontend/src/components/blips/*` (DataTable, WorkflowStatusBadge, MakerReviewerApproverPanel, dll) + `lib/{api,notify}.ts` REUSABLE. Per modul: `lib/schemas/{m}.schema.ts` + `lib/api/{m}.api.ts` + `app/master/{m}/` (list/new/[id]/edit/history).
- **QA**: UAT `docs/uat/` + integration `backend/internal/test/integration/{m}_test.go`.

---

## 3. Urutan replikasi + GATE (START_HERE §5)
| # | Modul | Eyes | Gate BLOCKING | Catatan |
|---|---|---|---|---|
| 1 | `periode_buku` | 4 | — | approver ROLE-CFO + **step-up MFA**; punya `id UUID` (no surrogate); generate periode 2026 (Gate 3→4) |
| 2 | `lgd_basel` | 6 | **ifrs9-compliance-reviewer** | ECL param, ALCO approve |
| 3 | `bobot_skenario` | 6 | **ifrs9-compliance-reviewer** | weights 0.25/0.50/0.25 (DEC-010) |
| 4 | `lps_coverage` | 6 | **ifrs9-compliance-reviewer** | IDR 2M (DEC-014) |
| 5 | `pd_pefindo` | 6 | **ifrs9-compliance-reviewer** | + upload XLSX (integration-engineer, progress UX §3); kalibrasi vs Pefindo study |
| 6 | `impact_mev_pd`,`impact_pd` | 6 | **ifrs9-compliance-reviewer** | FL multiplier |
| 7 | `counterparty`+`rating_history` | 4 | **security-engineer** | PII encrypt npwp/no_rek/ktp (DEC-028, sec.encrypt/decrypt 0003) |
| 8 | `chart_of_accounts` | 4 | — | + import Excel (progress UX §3); Gate 3→4 |
| 9 | `mapping_jurnal_header`+`detail` | 4 | — | Gate 3→4 |
| 10 | `portofolio` | 4 | — | |
| 11 | `instrumen` | 4 | — | paling kompleks (FK banyak master) |
| 12 | `kurs` | 4 | — | + BI JISDOR scheduled job (integration-engineer) |

Handoff per modul: data-modeler (00NN schema) → backend-engineer-go → frontend-engineer-nextjs → qa-engineer → [compliance/security gate bila tabel di atas] → commit → PR → CI → merge.

---

## 4. Keputusan open-question (sudah dipilih, mode "A/default")
- **OQ-1** SoD master umum: 4-eyes, 3 user distinct (maker ≠ reviewer ≠ approver).
- **OQ-2** Notifikasi Phase 3: in-app dulu (SMTP dry-run); email aktifkan saat config siap.
- **OQ-3** `rating_history`: maker=ROLE-MAKER-TR, reviewer=ROLE-RISK (2 role beda, hindari self-SoD).

WORKFLOW_CONFIG_* untuk semua 16 entity sudah di-seed (0007 + 0008). `periode_buku` (WORKFLOW_CONFIG_PERIODE) cek apakah approver=CFO + step-up sudah benar; kalau belum, UPDATE via migration baru (jangan edit file 0007/0008 yang sudah committed).

---

## 5. Backlog (non-blocking, pre-production)
- Frontend test runner **vitest** belum di-setup (test FE sudah ditulis, vitest-ready) → `chore(web): add vitest`.
- **0001 mst.* schema kurang audit cols** (ketemu di mata_uang) → cek tiap tabel saat replikasi, fix di migration per modul.
- **Doc-drift**: `db-conventions.md` §"Audit log table" canonical (event_id/event_time/ip/before_jsonb) menyimpang dari implementasi 0001 (id/timestamp/ip_address/before_value) → rekonsiliasi dokumen oleh data-modeler/MDA.
- Security MEDIUM/LOW Phase 2: MEDIUM-2 (workflow GetStatus expose actor UUID), MEDIUM-3 (engine permission guard claims-verified), MEDIUM-5 (ratelimit role-string), idle-window DEC-025 wiring (idleWindowKey dihapus + TODO), current_hash SET NOT NULL post-backfill, ClamAV real scanner. Lihat plan Phase 2 §10.
- `/audit` MDA conformance bisa dijalankan di milestone (advisory).

---

## 6. Cara resume cepat (sesi baru, tool aktif)
```
git fetch origin
git checkout -b feature/phase-3-periode-buku origin/develop   # base bersih
# lalu: /plan sudah ada (PLAN-20260603-phase-3-master-data.md). Dispatch:
#   data-modeler  → migration 0009 periode_buku (lihat pola 0008)
#   backend-engineer-go → CRUD periode_buku (pola matauang) + generate-2026
#   frontend-engineer-nextjs → screen /master/periode-buku
#   qa-engineer → UAT + integration test
# verify tiap langkah: go build/vet/test + /tmp/glci/golangci-lint run + pnpm build/lint
# commit per scope (feat(db)/feat(app-d? periode=APP-D? -> scope app-a master, periode_buku ada di mst) ), PR ke develop, CI hijau, merge (admin atas otorisasi user).
```
> Catatan scope commit: `periode_buku` adalah master data (`mst`) di Phase 3 APP-A → scope `app-a` atau `db`/`web` sesuai layer.

Plan referensi: `docs/plans/PLAN-20260603-phase-3-master-data.md`.
