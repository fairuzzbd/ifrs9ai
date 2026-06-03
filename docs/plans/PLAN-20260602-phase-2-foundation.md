# PLAN-20260602 — Phase 2 Foundation Layer

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-02
**Sumber**: `START_HERE.md` §5/§7 · `docs/handoff/phase-0-to-phase-2.md` §3 · `.claude/memory/locked-decisions.md`
**Klasifikasi**: foundation/cross-cutting — **menyentuh path regulated** (auth/PII/audit → `security-engineer` BLOCKING)
**Target**: 2 minggu (per handoff §3)

---

## 1. Goal
Bangun building blocks yang dipakai semua modul APP-A..E: **Auth & RBAC, Audit Trail,
Workflow Engine, Notification, Document Upload, Common Middleware, cmd/migrator**. Exit =
5 quality gate Phase 2 → 3 (lihat §7) hijau. PR Phase 2 #1 sekaligus menutup **P0-3** (CI first run).

## 2. Decision Log check (refuse to reopen)
Plan ini **patuh**, tidak ada DEC yang di-reopen:
- **DEC-006** auth lewat Keycloak (SAML/OIDC) — bukan custom auth. Phase 2 = JWT verify + Keycloak stub; full Keycloak compose = `integration-engineer`/`devops-engineer`.
- **DEC-007** Asynq untuk notification + audit-verify job.
- **DEC-016** `shopspring/decimal` — register common helper (belum dipakai berat di Phase 2, tapi disiapkan).
- **DEC-017** workflow 4-eyes default + 6-eyes klasifikasi; SoD `maker≠reviewer≠approver` **server-side**.
- **DEC-018** audit append-only, hash-chain, retensi 10+10.
- **DEC-021** Idempotency-Key wajib di mutating endpoint → middleware + `sys.idempotency_key`.
- **DEC-022** cursor pagination only.
- **DEC-024** Argon2id (di Keycloak, config-only).
- **DEC-025** JWT RSA-2048, access 15m / refresh 8h / idle 15m.
- **DEC-026** MFA mandatory (CFO/CEO/KOMITE/ALCO/Treasury Mgr/Finance Controller) + **DEC-027** step-up.
- **DEC-028** AES-256 TDE + column-level PII via `sec.encrypt()`/`sec.decrypt()` plpgsql.

## 3. Affected schemas / modul
- **Schema**: `sec` (user shadow, role, permission, session, idempotency), `aud` (audit_log trigger + hash), `sys` (job, config), `doc` (document metadata). Plus plpgsql `sec.encrypt/decrypt`.
- **Kode**: `backend/internal/{auth,audit,workflow,notification,document,common}`, `backend/cmd/{migrator,audit-verify}`.
- **Frontend**: belum (Phase 2 backend-foundation; first real screen = Phase 3). Hanya `lib/notify.ts`/`DataTable`/`JobProgressPanel` jadi acuan kontrak.

## 4. Agents + handoff order

```
system-analyst        → kontrak OpenAPI: auth, /jobs, workflow signing endpoints, error envelope; state machine workflow (DRAFT→PENDING_REVIEW→PENDING_APPROVAL→APPROVED/REJECTED)
   ↓
data-modeler          → migration 0003+: sec.encrypt/decrypt plpgsql, sys.job, sys.idempotency_key (jika belum), aud.audit_log trigger+hash, doc.document; cmd/migrator binary
   ↓ (parallel by domain setelah kontrak + schema fix)
security-engineer + backend-engineer-go   → JWT middleware, RBAC {entity}.{action}, SoD service-layer, audit middleware (same-tx write + hash chain), cmd/audit-verify
backend-engineer-go                        → workflow engine (config-driven state machine), common middleware (reqID/trace/slog/error/ratelimit), idempotency middleware
integration-engineer + backend-engineer-go → notification (Asynq + SMTP + in-app), document upload (MinIO + SHA-256 + ClamAV stub)
   ↓
qa-engineer           → unit tests ≥80% file Phase 2; integration test WAJIB: "Maker coba jadi Approver via API langsung" (SoD), audit hash-chain verify, idempotency replay/mismatch
   ↓
security-engineer     → BLOCKING review: threat-model checklist 10 poin (security-baseline §), no plaintext PII, no role-string compare, audit write in-tx, idempotency present
   ↓
devops-engineer       → Keycloak compose (DEC-006), /readyz real check (debt #2), runbook §7 wording fix (debt #8), pre-commit hooks (debt #3); konfirmasi backend-lint hijau → tutup P0-3
```

> **ifrs9-compliance-reviewer**: TIDAK terpicu di Phase 2 (tidak menyentuh ECL/EIR/SPPI/BM/klasifikasi). Akan aktif Phase 3+ saat APP-A/C mulai.

## 5. Blocking dependencies
1. **system-analyst kontrak** harus committed sebelum backend mulai (anti-pattern: frontend/backend start sebelum OpenAPI). Auth + workflow + jobs contract = prasyarat.
2. **data-modeler `sec.encrypt/decrypt` + `sys.job`** harus ada sebelum security-engineer wiring PII & sebelum job-bearing services.
3. **audit middleware** harus jadi sebelum service mutating lain (semua mutation wajib tulis `aud.audit_log` same-tx).
4. **security-engineer BLOCKING sign-off** sebelum merge apa pun yang menyentuh auth/PII/audit.

## 6. Risk + rollback
| Risk | Mitigasi | Rollback |
|---|---|---|
| Audit write di luar business tx (red flag security-baseline) | Audit middleware bungkus handler dalam 1 DB tx | Reject di security review; revert PR |
| Role-string comparison bocor ke kode | RBAC pakai `{entity}.{action}` permission check; lint/grep gate | Reject merge |
| SoD bypass via API langsung | qa integration test wajib skenario maker=approver | Block release sampai test hijau |
| `commit.gpgsign` + branch protection → developer commit gagal | P0-1 sudah RESOLVED (signed commit verified) | n/a |
| Keycloak full setup molor | Phase 2 cukup JWT-verify + stub; full Keycloak = devops/integration paralel, tidak blok foundation | feature-flag auth source |
| Migration 0003 tidak reversible | `down.sql` wajib + test rollback (gate §7) | golang-migrate down |

## 7. Verifikasi (quality gate Phase 2 → 3, dari handoff §3)
- [ ] Auth flow working — login sample user dari seed (10 persona) → JWT valid + claims benar
- [ ] Audit log auto-populate saat mutation (same-tx, hash chain valid via `cmd/audit-verify`)
- [ ] Workflow engine transition DRAFT → PENDING → APPROVED (+ SoD reject path)
- [ ] Document upload + SHA-256 verified (MinIO)
- [ ] `cmd/migrator` run 0001+0002 dari empty DB + rollback OK
- [ ] qa run report: unit ≥80% file Phase 2 + SoD/idempotency/hash-chain integration test hijau
- [ ] security-engineer BLOCKING sign-off (threat-model 10 poin)
- [ ] `backend-lint` hijau di PR #1 → **P0-3 closed**, §1 Gate 4 PENDING→PASS

## 8. Sequencing & branch
- GitFlow: semua `feature/*` dari `develop`, target `develop` (squash). Scope commit: `feat(sec)`, `feat(api)`, `feat(db)`, `feat(worker)`, `feat(integ)`.
- Saran pemecahan PR (kecil, reviewable):
  - PR#1 `feature/app-foundation-migrator` (data-modeler) — cmd/migrator + migration 0003 (sec.encrypt/decrypt, sys.job) → **trigger backend-lint pertama → P0-3**
  - PR#2 `feature/sec-jwt-rbac-sod` (security + backend)
  - PR#3 `feature/audit-middleware-hashchain` (security + backend)
  - PR#4 `feature/workflow-engine` (backend + system-analyst state machine)
  - PR#5 `feature/common-middleware-idempotency` (backend)
  - PR#6 `feature/notification-document` (integration + backend)
  - tiap PR: qa test + (jika auth/PII/audit) security BLOCKING review.

## 9. Per-agent dispatch prompts (self-contained)

### → system-analyst
> Bikin OpenAPI 3.0 fragment + state machine untuk Phase 2 foundation. Scope: (a) auth endpoints (login via Keycloak OIDC callback, refresh, step-up MFA) + JWT claims per `security-baseline.md`; (b) generic workflow signing endpoints `/{resource}/{id}/{submit,review,approve,reject}` per `api-conventions.md` §"Workflow endpoints"; (c) `/api/v1/jobs/{id}` + `/stream` (SSE) + cancel per `ux-patterns.md` §3.2; (d) error envelope + stable error codes (`SOD_VIOLATION`, `IDEMPOTENCY_*`, `WORKFLOW_INVALID_TRANSITION`, dll). State machine workflow: DRAFT→PENDING_REVIEW→PENDING_APPROVAL→APPROVED|REJECTED, dengan guard SoD + 6-eyes variant untuk klasifikasi (config flag). Output: `api/openapi/` fragment + diagram state machine. Pertanyaan kunci: bagaimana represent 4-eyes vs 6-eyes di satu config-driven engine?

### → data-modeler
> Tulis migration golang-migrate (0003+) + `cmd/migrator` binary. Deliverable: (1) plpgsql `sec.encrypt()`/`sec.decrypt()` AES-256-GCM role-gated untuk PII (`mst.counterparty.{npwp,nomor_rekening,ktp}`, `sec.user.{email_personal,phone}`) per DEC-028; (2) `sys.job` table per `ux-patterns.md` §3.3; (3) `sys.idempotency_key` per `api-conventions.md` (jika belum ada di 0001); (4) `aud.audit_log` append-only trigger (refuse hard-delete) + hash-chain columns sesuai `db-conventions.md` kanonik; (5) `doc.document` metadata table. Semua: audit cols wajib, `down.sql` reversible + tested, partition discipline. `cmd/migrator` = wrapper `golang-migrate/migrate/v4` (up/down/version). Patuhi db-conventions hard rules. Pertanyaan: apakah `sys.idempotency_key` sudah ada di migration 0001?

### → security-engineer + backend-engineer-go (parallel domain)
> Implement Auth & RBAC + Audit middleware. (A) JWT middleware: verify RSA-2048, access 15m/refresh 8h/idle 15m (DEC-025), parse claims (`roles`, `permissions`, `mfa_verified`, `tenant_id`); reject `UNAUTHORIZED`. (B) Permission check di service layer pakai `{entity}.{action}` — **DILARANG** role-string compare (red flag). (C) SoD enforcement server-side persis `security-baseline.md` (maker≠reviewer≠approver) — bukan cuma UI. (D) Step-up MFA gate untuk action DEC-027. (E) Audit middleware: tulis `aud.audit_log` row **di transaksi yang sama** dengan mutation, hash chain SHA-256 (`current=sha256(prev||canonical_json)`), + `cmd/audit-verify --range`. Keycloak = verify-only + stub (full setup devops). Semua mutating endpoint cek Idempotency-Key. Output minta: kode + unit test. CATATAN: ini path BLOCKING — saya akan minta security-engineer review akhir.

### → backend-engineer-go
> Workflow engine + common middleware. (1) Generic Maker-Reviewer-Approver engine, config-driven state machine (dari system-analyst), reusable per entity workflow-bearing; signing endpoint store `signed_at`+`signature_hash`+`signature_method` (never overwriteable). (2) Common middleware: request-id/trace propagation, structured logging slog, error handler → error envelope (`api-conventions.md`), rate limiter 100 req/min/user (sensitif 10/min). (3) Idempotency middleware pakai `sys.idempotency_key` (replay→original response, mismatch→422). JANGAN implement ECL math (bukan domain Anda — itu ecl-eir-engineer). Output: kode + unit test ≥80%.

### → integration-engineer + backend-engineer-go
> Notification + Document upload. (1) Notification service: Asynq job, SMTP email + in-app queue, trigger pada workflow transition + job completion; pesan spesifik (bukan "Berhasil") per `ux-patterns.md` §2.2. (2) Document upload: MinIO put, SHA-256 hash verify, ClamAV virus scan **stub** (Phase 1), metadata ke `doc.document`. Endpoint mutating wajib Idempotency-Key + audit write. Output: kode + adapter + runbook singkat + unit test.

### → qa-engineer
> Tulis + run tests untuk semua deliverable Phase 2. WAJIB cover: (a) SoD bypass — "Maker mencoba jadi Reviewer/Approver via API langsung" → expect `SOD_VIOLATION` 403; (b) audit hash-chain integrity via `cmd/audit-verify`; (c) idempotency replay (same key+payload→original) + mismatch (→422); (d) workflow state machine semua transition + invalid transition→422; (e) JWT expiry/idle; (f) PII encrypt/decrypt role-gated. Unit coverage ≥80% file Phase 2. Output: test code + **run report** (tidak ada "done" tanpa run report).

### → security-engineer (BLOCKING gate)
> Review final semua PR auth/PII/audit Phase 2 dengan threat-model checklist 10 poin (`security-baseline.md`). BLOCK kalau ketemu: plaintext PII non-encrypted, hardcoded secret, role-string compare, audit write di luar business tx, skip Idempotency-Key, SoD bypass TODO, return full before/after jsonb ke non-AUDIT role, log PII/JWT. Output: VERDICT + remediation list.

### → devops-engineer
> (1) Keycloak ke docker-compose dev (DEC-006, SAML/OIDC). (2) `/readyz` cek Postgres+Redis+MinIO real (debt #2). (3) Runbook `phase-0-smoke-test.md` §7 Gate 4 wording "GitLab pipeline" → "GitHub Actions (backend-lint)" (debt #8). (4) Pre-commit hooks `.pre-commit-config.yaml` (debt #3). (5) Konfirmasi `backend-lint` hijau di PR #1 → lapor ke orchestrator untuk tutup P0-3.

## 10. Status (update 2026-06-03)
- [x] Plan + decision-log check + delegasi disusun
- [x] Dispatch system-analyst (kontrak) — DONE, committed `d584afb` (`api/openapi/*`)
- [x] Dispatch data-modeler (migrator + schema) — DONE, migrations 0003-0007 + `cmd/migrator` (uncommitted)
- [x] Dispatch build agents — DONE:
      - backend-engineer-go foundation core (auth/RBAC/SoD/audit/common/idempotency) — build/vet/test 0
      - backend-engineer-go workflow engine (4/6-eyes config-driven, 62 test) — build/vet/test 0
      - integration-engineer notification + document (Asynq/SMTP/MinIO/SHA-256) — build/vet/test 0
- [x] qa run report — DONE: 23 integration test ditulis (compile-clean `-tags=integration`), unit 11 paket PASS
      ⚠️ integration test SKIP graceful — **blocker: docker no-access** di environment dev (butuh CI/infra)
- [x] security-engineer BLOCKING sign-off — **APPROVED-WITH-CONDITIONS** → 4 HIGH fixed → re-review **APPROVED, gate dibuka**
      - HIGH-1 audit ip/user_agent · HIGH-2 constant-time compare · HIGH-3 hapus JWTSecret default · HIGH-4 idempotency expires_at — semua CLOSED
      - Decision A: SSE `?token=` DITOLAK → kontrak direvisi (fetch-based SSE + Authorization header; sse-ticket fallback opsional)
      - Decision B: virus PENDING block via `DOCUMENT_BLOCK_PENDING_DOWNLOAD` (default true)
- [x] BUG FIX (orchestrator catch): `aud.audit_log` writer column mismatch (event_id/before_jsonb/ip vs id/before_value/ip_address) → align ke skema 0001. Silent audit-gap dicegah (DEC-018).
- [ ] devops: Keycloak compose, /readyz real, runbook §7 fix, pre-commit
- [ ] Commit + PR#1 → `backend-lint` hijau → tutup P0-3 + flip Gate 4
- [ ] Integration test run di CI/infra ber-docker (coverage 6 paket akan ≥80% saat live)

### Backlog (security MEDIUM/LOW — pre-production, non-merge-blocking)
- MEDIUM-2 workflow GetStatus expose actor UUID ke {entity}.read → redact/permission terpisah
- MEDIUM-3 engine permission guard config-presence-only → verify via claims
- MEDIUM-5 ratelimit ROLE-AUDIT pakai role-string → ganti permission-based
- LOW-1 audit fetchPreviousHash error silently non-fatal → fail mutation
- LOW-2 sec.encrypt GRANT deferred ke devops → masukkan ke migration
- LOW-3 current_hash nullable → SET NOT NULL setelah backfill
- ClamAV real scan worker (virus stub) sebelum go-live
- **Doc drift**: `db-conventions.md` §"Audit log table" canonical (event_id/event_time/ip/before_jsonb) menyimpang dari migration 0001 aktual (id/timestamp/ip_address/before_value) → rujuk data-modeler/MDA untuk rekonsiliasi dokumen

### Open decisions (orchestrator)
- **SSE auth**: kontrak system-analyst `?token=` query vs rekomendasi backend `Authorization` header.
  Token di URL → bocor ke access log. DEFER ke security-engineer BLOCKING review. Jika ditolak →
  system-analyst revisi kontrak (fetch-based SSE / short-lived single-use token).
- **virus_scan PENDING gate**: Phase 1 file PENDING masih downloadable; security review tentukan
  apakah perlu feature-flag block.

### Catatan integritas
- Coverage unit saat ini 56.4% TOTAL — di bawah 80% KARENA path DB/Redis/MinIO hanya ter-cover
  integration test yang belum bisa run (docker blocker). BUKAN gap test, tapi gap eksekusi infra.
  Gate §7 "≥80%" baru bisa diklaim PASS setelah integration run hijau di CI.
