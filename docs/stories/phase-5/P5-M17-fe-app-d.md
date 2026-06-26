# P5-M17 — APP-D Frontend: Periode Buku, Kurs, Mapping Jurnal, Jurnal Header, Reconciliation: User Stories

**Story Set ID**: P5-M17
**Modul**: APP-D — Periode Buku + FX + Mapping Jurnal + Jurnal Engine + GL Delivery (Frontend-only)
**Status**: DRAFT — menunggu handoff ke `uiux-designer` (tab layout + MFA modal placement); `frontend-engineer-nextjs` (implementasi); `security-engineer` (role-gate + MFA step-up BLOCKING)
**Author**: business-analyst
**Tanggal**: 2026-06-25
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §5 (APP-D Periode, FX, Mapping, Jurnal); FSD-APP-D-*.docx
**Linked BRD**: BRD §4.4 (APP-D kontrol periode, kurs, mapping jurnal, jurnal posting); §3 RACI: ROLE-AKUN (R), ROLE-AKUN-CTL (A), ROLE-CFO (A hard-close), ROLE-RISK (R klasifikasi mapping), ROLE-KOMITE (A 6-eyes mapping), ROLE-AUDIT (R read-only), ROLE-IT-ADMIN (R DLQ)
**Linked Decision Log**:
- `DEC-002` (LOCKED) — Next.js 14+ App Router, TypeScript strict, shadcn/ui, React Hook Form + Zod, Zustand, TanStack Query
- `DEC-005` (LOCKED) — GL Integration Phase 1: file batch. REST real-time = Phase 2. Reconciliation UI = read-only view atas data yang sudah dikirim, bukan trigger manual.
- `DEC-017` (LOCKED) — 4-eyes workflow rutin; 6-eyes untuk mapping jurnal (klasifikasi PSAK 71 territory). SoD maker ≠ reviewer ≠ approver.
- `DEC-018` (LOCKED) — audit trail append-only; `{ENTITY}.EXPORT` wajib tiap export
- `DEC-020` (LOCKED) — REST `/api/v1/`; camelCase JSON; snake_case DB
- `DEC-021` (LOCKED) — Idempotency-Key wajib pada setiap mutation endpoint
- `DEC-022` (LOCKED) — cursor-based pagination only; no offset
- `DEC-025` (LOCKED) — JWT RSA-2048; permission check via `{entity}.{action}`, bukan role string
- `DEC-026` (LOCKED) — MFA mandatory: ROLE-AKUN-CTL (Finance Controller), ROLE-CFO, ROLE-KOMITE, ROLE-ALCO
- `DEC-027` (LOCKED) — Step-up MFA: hard-close periode, ECL parameter approve, klasifikasi approve, calc-run seal

**Dependensi (WAJIB selesai sebelum M17)**:
- **P5-M4** (commit tersedia) — Periode close endpoints; OpenAPI: `api/openapi/app-d-periode-close.yaml`; `MFAStepUpModal.tsx` dari M4 tersedia di `frontend/src/components/blips/MFAStepUpModal.tsx`
- **P5-M5** (commit tersedia) — FX rate + JISDOR sync endpoints; OpenAPI: `api/openapi/app-d-fx-rate.yaml`
- **P5-M12** (commit tersedia) — Mapping jurnal 6-eyes endpoints; OpenAPI: `api/openapi/app-d-mapping-jurnal.yaml`
- **P5-M2** (commit tersedia) — Jurnal engine endpoints; OpenAPI: `api/openapi/app-d-jurnal-engine.yaml`
- **P5-M3** (commit tersedia) — GL delivery + DLQ endpoints; OpenAPI: `api/openapi/app-d-gl-delivery.yaml`
- **P5-M13** (commit tersedia) — `sys.job` table + `GET /api/v1/jobs/{jobId}` + SSE stream + cancel
- **P5-M14** (commit tersedia) — 25 report endpoints termasuk RPT22b (gl_delivery) untuk reconciliation
- **`frontend/src/components/blips/DataTable.tsx`** — DataTable UX §1 tersedia
- **`frontend/src/components/blips/JobProgressPanel.tsx`** — JobProgressPanel UX §3 tersedia
- **`frontend/src/lib/auth/permissions.ts`** — `requirePermission()` + `requirePermissionWithMfa()` dari M15 tersedia

**Gate**:
- `security-engineer` **BLOCKING** — (a) absent-from-DOM per role untuk semua tombol mutasi dan aksi sensitif; (b) MFA step-up hard-close: server component + `MFAStepUpModal` + server-side verification token sebelum execute; (c) DLQ replay MFA step-up; (d) SoD 6-eyes mapping jurnal enforced di server component; (e) 308 redirect tidak bocorkan data lama; (f) Idempotency-Key auto-inject di setiap form submit.
- `ifrs9-compliance-reviewer` **TIDAK ADA GATE** — M17 adalah murni UI layer atas endpoint yang sudah compliance-reviewed di M2/M4/M5/M12; tidak ada ECL/EIR/SPPI/BM computation baru.

---

## Konteks & Scope P5-M17

P5-M17 adalah **frontend-only modul** yang mengkonsolidasi dan melengkapi screen APP-D di bawah namespace canonical berikut:

- `/periode-buku/*` — timeline closing workflow (soft close, hard close + MFA step-up, reopen)
- `/master/kurs/*` — FX rate CRUD, JISDOR sync, bulk upload batch
- `/master/mapping-jurnal/*` — 6-eyes workflow chain UI (canonical; redirect dari duplikat)
- `/jurnal/header/*` — jurnal header list + detail + DLQ queue (**MISSING**, perlu dibuat baru)
- `/reconciliation/daily` — daily BLIPS vs GL Host recon view read-only (**MISSING**, perlu dibuat baru)

### State saat ini (hasil baca tree)

| Path sekarang | Isi | Masalah / Aksi M17 |
|---|---|---|
| `frontend/src/app/periode-buku/` | list `page.tsx`, `[id]/page.tsx` — M4 simple view | Namespace benar; perlu audit: timeline view, closing workflow buttons (soft/hard/reopen), MFA step-up belum terverifikasi ada |
| `frontend/src/app/master/periode-buku/` | list, new, edit, detail, history — full CRUD | CRUD dipindah ke canonical `/periode-buku/`; file di `/master/periode-buku/` → 308 redirect |
| `frontend/src/app/master/kurs/` | list, new, edit, detail, history, upload, jisdor-sync | Namespace benar; perlu audit UX §1/§2/§3 upload batch |
| `frontend/src/app/mapping-jurnal/` | list `page.tsx`, `[event_code]/page.tsx`, `import/page.tsx` — event_code-based duplikat | 308 redirect ke canonical `/master/mapping-jurnal/` |
| `frontend/src/app/jrnl/mapping/` | list, new, edit, [id] — alt namespace duplikat | 308 redirect ke canonical `/master/mapping-jurnal/` |
| `frontend/src/app/master/mapping-jurnal/` | list, new, edit, detail, history — canonical 6-eyes | Perlu audit 6-eyes workflow chain UI (4 step: submit→review→approve→approve-2→activate) |
| `frontend/src/app/jrnl/journal-entries/` | list `page.tsx`, `[id]/page.tsx` — ada tapi bukan di `/jurnal/header/` | Namespace berbeda dari target; buat `/jurnal/header/` baru atau git mv + redirect |
| `frontend/src/app/jrnl/dlq/` | list `page.tsx`, `[id]/page.tsx` — DLQ ada | Sudah ada; konsolidasi ke `/jurnal/` layout |
| `frontend/src/app/jrnl/resolve/` | `page.tsx` — IT-ADMIN debug | Sudah ada; masuk ke `/jurnal/` layout tab "Resolve" |
| `frontend/src/app/jrnl/rekonsiliasi/` | `page.tsx`, `riwayat/page.tsx` — rekonsiliasi stub | Namespace `/jrnl/rekonsiliasi/` bukan target canonical; 308 redirect ke `/reconciliation/daily` |
| `/jurnal/header` | **MISSING** | Buat baru; konsumsi jurnal-engine endpoints |
| `/reconciliation/daily` | **MISSING** | Buat baru; konsumsi M14 RPT22b atau `api/openapi/app-d-gl-delivery.yaml` recon endpoint |

### Target state setelah M17

```
frontend/src/app/
  periode-buku/
    layout.tsx                      ← NEW: shared layout dengan timeline nav + closing workflow header
    page.tsx                        ← EXISTING (M4); upgrade: timeline view, closing action buttons
    new/page.tsx                    ← git mv dari /master/periode-buku/new/page.tsx
    [id]/
      page.tsx                      ← EXISTING; upgrade: detail dengan closing workflow panel
      edit/page.tsx                 ← git mv dari /master/periode-buku/[id]/edit/
      history/page.tsx              ← git mv dari /master/periode-buku/[id]/history/

  master/
    kurs/                           ← EXISTING; audit UX §1/§2/§3 (upload batch JobProgressPanel)
    mapping-jurnal/                 ← EXISTING canonical; audit 6-eyes workflow chain UI
    periode-buku/ → 308 redirect ke /periode-buku/*

  mapping-jurnal/ → 308 redirect ke /master/mapping-jurnal/*
  jrnl/
    mapping/ → 308 redirect ke /master/mapping-jurnal/*
    journal-entries/ → 308 redirect ke /jurnal/header/*
    rekonsiliasi/ → 308 redirect ke /reconciliation/daily

  jurnal/
    layout.tsx                      ← NEW: shared layout tab nav (Header | DLQ | Resolve)
    header/
      page.tsx                      ← NEW: list jurnal header (DataTable UX §1)
      [id]/page.tsx                 ← NEW: detail jurnal header + approval panel 4-eyes
    dlq/
      page.tsx                      ← git mv dari /jrnl/dlq/page.tsx
      [id]/page.tsx                 ← git mv dari /jrnl/dlq/[id]/page.tsx
    resolve/
      page.tsx                      ← git mv dari /jrnl/resolve/page.tsx (IT-ADMIN only)

  reconciliation/
    daily/
      page.tsx                      ← NEW: BLIPS vs GL recon view (read-only, DataTable)

next.config.js                      ← UPDATE: tambah 308 redirects (append ke M16 list)
```

### Tidak di-scope M17 (eksplisit)

- Tidak ada mutating backend endpoint baru — semua form submit ke endpoint M2/M4/M5/M12 yang sudah ada.
- Tidak ada migration database baru.
- Jurnal posting cron config UI — Phase 6.
- Reconciliation manual trigger UI — cron-only (Phase 6).
- Phase 6 mapping diff visualization.
- ECL/EIR screens — scope M14/M15/M18.
- Klasifikasi PSAK 71 screens — scope M1 (APP-A).

---

## Persona Table

| Role | Sub-routes yang diakses | Permission wajib | MFA |
|---|---|---|---|
| ROLE-AKUN | /master/kurs (CRUD + upload + jisdor-sync), /master/mapping-jurnal (submit), /jurnal/header (list + detail), /reconciliation/daily (read) | `fx_rate.create`, `fx_rate.update`, `jurnal.read`, `mapping_jurnal.submit` | Tidak wajib |
| ROLE-AKUN-CTL | /periode-buku (soft-close + reopen), /master/mapping-jurnal (review), /jurnal/header (approve), /reconciliation/daily (read) | `periode.softclose`, `periode.reopen`, `jurnal.approve`, `mapping_jurnal.review` | WAJIB (MFA mandatory per DEC-026) |
| ROLE-CFO | /periode-buku (hard-close) | `periode.hardclose` | WAJIB + step-up MFA per DEC-027 |
| ROLE-RISK | /master/mapping-jurnal (approve step 1 dari 6-eyes), read-only semua screen | `mapping_jurnal.approve` | Tidak wajib |
| ROLE-KOMITE | /master/mapping-jurnal (approve step 2 dari 6-eyes = final), read-only | `mapping_jurnal.approve` (second approver) | WAJIB per DEC-026 |
| ROLE-AUDIT | semua screen read-only; tidak ada aksi mutasi; export tersedia | `jurnal.read`, `periode.read`, `fx_rate.read`, `mapping_jurnal.read`, `audit_log.read` | Tidak wajib |
| ROLE-IT-ADMIN | /jurnal/dlq (list + detail + replay), /jurnal/resolve | `jurnal.dlq.read`, `jurnal.dlq.replay` | WAJIB; step-up MFA untuk replay per DEC-027 |

---

## Deliverables M17

| # | Artefak | Tipe | Keterangan |
|---|---|---|---|
| 1 | `frontend/src/app/periode-buku/layout.tsx` | Next.js layout | Shared layout: timeline nav + closing workflow action header |
| 2 | `frontend/src/app/periode-buku/page.tsx` (upgrade) | Next.js page | Upgrade M4: timeline view + soft/hard/reopen buttons |
| 3 | `frontend/src/app/periode-buku/[id]/page.tsx` (upgrade) | Next.js page | Upgrade M4: closing workflow panel + MFA step-up wiring |
| 4 | `frontend/src/app/periode-buku/new/page.tsx` | Next.js page | git mv dari `/master/periode-buku/new/` |
| 5 | `frontend/src/app/periode-buku/[id]/edit/page.tsx` | Next.js page | git mv dari `/master/periode-buku/[id]/edit/` |
| 6 | `frontend/src/app/periode-buku/[id]/history/page.tsx` | Next.js page | git mv dari `/master/periode-buku/[id]/history/` |
| 7 | `frontend/src/app/master/kurs/` (audit + fix) | Audit + fix | Audit UX §1/§2/§3; fix upload batch JobProgressPanel jika belum ada |
| 8 | `frontend/src/app/master/mapping-jurnal/` (audit + fix) | Audit + fix | Audit 6-eyes chain UI (4 step buttons + SoD enforcement) |
| 9 | `frontend/src/app/jurnal/layout.tsx` | Next.js layout | NEW: shared tab nav (Header | DLQ | Resolve) |
| 10 | `frontend/src/app/jurnal/header/page.tsx` | Next.js page | NEW: list jurnal header (DataTable UX §1) |
| 11 | `frontend/src/app/jurnal/header/[id]/page.tsx` | Next.js page | NEW: detail jurnal header + 4-eyes approval panel |
| 12 | `frontend/src/app/jurnal/dlq/page.tsx` | Next.js page | git mv dari `/jrnl/dlq/page.tsx` |
| 13 | `frontend/src/app/jurnal/dlq/[id]/page.tsx` | Next.js page | git mv dari `/jrnl/dlq/[id]/page.tsx` |
| 14 | `frontend/src/app/jurnal/resolve/page.tsx` | Next.js page | git mv dari `/jrnl/resolve/page.tsx` |
| 15 | `frontend/src/app/reconciliation/daily/page.tsx` | Next.js page | NEW: BLIPS vs GL recon view (read-only DataTable) |
| 16 | `frontend/src/components/blips/jurnal/JurnalTabNav.tsx` | Component | NEW: tab nav 3 slot Header | DLQ | Resolve |
| 17 | `frontend/src/components/blips/jurnal/index.ts` | Component index | Barrel export |
| 18 | `next.config.js` (update) | Config | 308 redirects (append ke M16 list): 8+ redirect rules |

---

## Story P5-M17-01 — Periode Buku Konsolidasi di `/periode-buku/*`: Timeline + Closing Workflow + MFA Step-up

**Actor**: ROLE-AKUN-CTL (soft-close + reopen), ROLE-CFO (hard-close), ROLE-AKUN (read-only + new periode), semua role (list read)
**Trigger**: User navigasi ke `/periode-buku` atau klik menu "Periode Buku" di sidebar.
**Goal**: Semua periode buku screens tersentralisasi di `/periode-buku/`; URL lama `/master/periode-buku/*` redirect permanent 308; list menampilkan timeline view periode (urutan kronologis + status badge); detail menyediakan panel closing workflow dengan soft-close (ROLE-AKUN-CTL), hard-close + MFA step-up (ROLE-CFO), dan reopen (ROLE-AKUN-CTL); `MFAStepUpModal` dari M4 digunakan kembali tanpa perubahan desain.

**Source OpenAPI**: `api/openapi/app-d-periode-close.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/periode` — list periode dengan sort/page/filter/export
- `POST /api/v1/periode` — create periode baru (Idempotency-Key wajib)
- `GET /api/v1/periode/{id}` — detail periode
- `PATCH /api/v1/periode/{id}` — edit draft (Idempotency-Key wajib)
- `GET /api/v1/periode/{id}/history` — riwayat perubahan periode
- `POST /api/v1/periode/{id}/soft-close` — soft close (ROLE-AKUN-CTL; Idempotency-Key wajib)
- `POST /api/v1/periode/{id}/hard-close` — hard close (ROLE-CFO; MFA step-up token wajib di header; Idempotency-Key wajib)
- `POST /api/v1/periode/{id}/reopen` — reopen dari soft-close (ROLE-AKUN-CTL; Idempotency-Key wajib)
- `GET /api/v1/periode/export?format=csv` — export daftar periode

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `WORKFLOW_INVALID_TRANSITION`, `IDEMPOTENCY_REPLAY`, `IDEMPOTENCY_MISMATCH`, `PERIODE_CLOSED`

**Komponen reused**: `DataTable.tsx`, `WorkflowStatusBadge.tsx`, `AuditHistoryTable.tsx`, `MFAStepUpModal.tsx` (dari M4 — jangan redesign), `frontend/src/app/master/periode-buku/_components/PeriodeBukuForm.tsx`

**MFA gate**: Hard-close → step-up MFA per DEC-027. `requirePermissionWithMfa()` dari `lib/auth/permissions.ts`. `MFAStepUpModal` di-trigger sebelum POST `/periode/{id}/hard-close`. Token step-up disertakan di header `X-Step-Up-Token`. Reopen dan soft-close: ROLE-AKUN-CTL MFA mandatory (DEC-026) tapi bukan step-up khusus — cukup `mfa_verified=true` di JWT.

### Pre-conditions
1. M4 deployed; endpoint `/periode/*` return 200
2. `frontend/src/app/periode-buku/page.tsx` + `[id]/page.tsx` exist (M4 simple view)
3. `frontend/src/app/master/periode-buku/` files exist dan siap di-move/redirect
4. `MFAStepUpModal.tsx` tersedia di `frontend/src/components/blips/`
5. `requirePermission()` + `requirePermissionWithMfa()` tersedia di `lib/auth/permissions.ts`

### Acceptance Criteria

```gherkin
Feature: Periode buku konsolidasi — relokasi namespace, 308 redirect, timeline view, closing workflow + MFA step-up

  Background:
    Given ROLE-CFO USR-CFO-001 ter-autentikasi dengan mfa_verified=true dan permission 'periode.hardclose', 'periode.read'
    And ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi dengan mfa_verified=true dan permission 'periode.softclose', 'periode.reopen', 'periode.read'
    And M4 periode endpoints semua return 200
    And MFAStepUpModal.tsx tersedia dan tidak dimodifikasi dari versi M4

  Scenario: M17-01-AC1 — 308 redirect dari /master/periode-buku/* ke /periode-buku/*
    Given next.config.js `redirects()` sudah dikonfigurasi untuk rute periode buku
    When browser mengakses /master/periode-buku
    Then HTTP 308 Permanent Redirect ke /periode-buku
    When browser mengakses /master/periode-buku/new
    Then HTTP 308 Permanent Redirect ke /periode-buku/new
    When browser mengakses /master/periode-buku/{id}
    Then HTTP 308 Permanent Redirect ke /periode-buku/{id}
    When browser mengakses /master/periode-buku/{id}/edit
    Then HTTP 308 Permanent Redirect ke /periode-buku/{id}/edit
    When browser mengakses /master/periode-buku/{id}/history
    Then HTTP 308 Permanent Redirect ke /periode-buku/{id}/history
    And semua 5 redirect paths tidak menghasilkan 404 di tujuan
    And breadcrumb di halaman tujuan menampilkan "Beranda / Periode Buku" (bukan "Beranda / Master / Periode Buku")

  Scenario: M17-01-AC2 — List /periode-buku: timeline view DataTable UX §1
    When USR-CTL-001 navigasi ke /periode-buku
    Then halaman menampilkan DataTable periode dengan timeline view:
      | data source    | GET /api/v1/periode?cursor=...&limit=50&sort=tanggal_mulai:desc |
      | kolom minimal  | kode_periode, nama_periode, tanggal_mulai, tanggal_selesai, status_close, tanggal_hard_close, created_by |
      | status badge   | OPEN (hijau), SOFT_CLOSED (kuning), HARD_CLOSED (merah), REOPENED (biru) — per WorkflowStatusBadge |
      | sort           | semua kolom sortable; default sort=tanggal_mulai:desc; header klik toggle; icon indicator |
      | filter         | filter[status_close], filter[tanggal_mulai] (year-month picker), ?q= text search |
      | filter chip    | filter aktif tampil sebagai chip; tombol "Bersihkan semua filter" |
      | URL state      | sort + filter di URL searchParams (deep-link friendly) |
      | pagination     | cursor-based; "Halaman X dari ~Y"; Prev/Next; limit 25/50/100; default 50 |
      | export         | tombol "Ekspor ▾" CSV + XLSX; respect filter aktif; async jika > 10k row; audit PERIODE.EXPORT |
      | CTA            | tombol "+ Periode Baru" visible hanya jika permission 'periode.create' ada di JWT |
    And loading state: skeleton rows saat fetch
    And empty state: "Tidak ada periode yang cocok." + "Bersihkan filter" CTA jika filter aktif
    And error state: pesan + error code + traceId + tombol "Coba lagi"

  Scenario: M17-01-AC3 — Hard-close /periode-buku/{id}: MFA step-up wajib (DEC-027)
    Given periode PRD-2026-06 status=SOFT_CLOSED
    When USR-CFO-001 navigasi ke /periode-buku/PRD-2026-06
    Then panel "Closing Workflow" menampilkan:
      | tombol "Hard-close Periode" | VISIBLE untuk USR-CFO-001 (permission 'periode.hardclose') |
      | tombol "Soft-close"          | ABSENT dari DOM untuk CFO (CFO hanya hard-close, bukan soft-close) |
    When USR-CFO-001 klik "Hard-close Periode"
    Then muncul DestructiveActionDialog: "Hard-close periode Juni 2026? Setelah hard-close, periode tidak bisa di-reopen. Semua jurnal akan final. Lanjut?"
    When USR-CFO-001 klik "Lanjutkan"
    Then MFAStepUpModal muncul (dari M4 — tidak dimodifikasi):
      | prompt    | "Konfirmasi hard-close dengan autentikasi tambahan" |
      | metode    | TOTP / WebAuthn / Push sesuai konfigurasi Keycloak USR-CFO-001 |
    When USR-CFO-001 memasukkan kode MFA yang valid
    Then MFAStepUpModal memanggil step-up endpoint Keycloak → dapat stepup_token
    And POST /api/v1/periode/PRD-2026-06/hard-close disertakan header X-Step-Up-Token: {stepup_token} + Idempotency-Key: {uuid}
    And tombol "Hard-close Periode" disable + spinner selama request berlangsung
    When server return 200
    Then MFAStepUpModal tertutup; toast success: "Periode Juni 2026 berhasil di-hard-close. Semua jurnal bersifat final."
    And WorkflowStatusBadge di halaman detail berubah ke HARD_CLOSED (merah) tanpa full page reload
    When USR-CFO-001 memasukkan kode MFA yang salah
    Then MFAStepUpModal tampilkan inline error: "Kode autentikasi salah. Coba lagi." (bukan toast — error di dalam modal)
    And POST hard-close TIDAK di-trigger
    When ROLE-AKUN-CTL USR-CTL-001 mencoba akses tombol "Hard-close Periode"
    Then tombol "Hard-close Periode" ABSENT dari DOM (permission 'periode.hardclose' tidak ada di JWT USR-CTL-001)
    And direct POST /api/v1/periode/PRD-2026-06/hard-close oleh USR-CTL-001 tanpa step-up → HTTP 403 FORBIDDEN

  Scenario: M17-01-AC4 — Soft-close + reopen workflow: ROLE-AKUN-CTL; form notification UX §2
    Given periode PRD-2026-07 status=OPEN
    When USR-CTL-001 navigasi ke /periode-buku/PRD-2026-07
    Then tombol "Soft-close Periode" VISIBLE (permission 'periode.softclose') dengan label yang jelas
    And tombol "Reopen Periode" ABSENT (periode masih OPEN — reopen hanya untuk SOFT_CLOSED)
    When USR-CTL-001 klik "Soft-close Periode"
    Then muncul konfirmasi dialog: "Soft-close periode Juli 2026? Periode masih bisa di-reopen untuk koreksi. Lanjut?"
    When USR-CTL-001 konfirmasi
    Then tombol disable + spinner inline; Idempotency-Key UUID v4 di-inject otomatis
    And POST /api/v1/periode/PRD-2026-07/soft-close disertakan Authorization (mfa_verified=true di JWT — cukup, bukan step-up)
    When server return 200
    Then toast success hijau 4 detik: "Periode Juli 2026 berhasil di-soft-close. Bisa dibuka kembali untuk koreksi."
    And WorkflowStatusBadge berubah ke SOFT_CLOSED (kuning)
    And tombol "Reopen Periode" muncul; tombol "Soft-close Periode" ABSENT
    When USR-CTL-001 klik "Reopen Periode" dan konfirmasi
    Then POST /api/v1/periode/PRD-2026-07/reopen → 200
    And toast success: "Periode Juli 2026 berhasil di-reopen. Status kembali ke OPEN."
    When server return 422 WORKFLOW_INVALID_TRANSITION (mis. mencoba soft-close periode yang sudah HARD_CLOSED)
    Then toast error merah persistent: "Periode ini sudah hard-closed dan tidak bisa di-soft-close kembali."
    And error menyertakan error code WORKFLOW_INVALID_TRANSITION + traceId (8 char)
```

---

## Story P5-M17-02 — Master Kurs: Audit UX + Manual Entry + JISDOR Sync + Bulk Upload JobProgressPanel

**Actor**: ROLE-AKUN (CRUD + upload + sync), semua role read (list + detail)
**Trigger**: User navigasi ke `/master/kurs` atau klik tab "Kurs" di shared `/master` layout.
**Goal**: Screen kurs yang sudah ada di namespace canonical `/master/kurs/` diaudit terhadap UX §1 (DataTable), §2 (form notification), dan §3 (JobProgressPanel untuk upload batch); upload batch kurs CSV/XLSX menggunakan `JobProgressPanel`; JISDOR sync menampilkan status sinkronisasi dan trigger manual; manual entry kurs tetap tersedia jika JISDOR feed gagal.

**Source OpenAPI**: `api/openapi/app-d-fx-rate.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/master/kurs` — list kurs dengan sort/page/filter/export
- `POST /api/v1/master/kurs` — create kurs manual (Idempotency-Key wajib)
- `GET /api/v1/master/kurs/{id}` — detail kurs
- `PATCH /api/v1/master/kurs/{id}` — edit kurs (Idempotency-Key wajib)
- `GET /api/v1/master/kurs/{id}/history` — riwayat perubahan kurs
- `POST /api/v1/master/kurs/upload` — bulk upload CSV/XLSX kurs (return 202 + jobId; Idempotency-Key wajib)
- `POST /api/v1/master/kurs/jisdor-sync` — trigger manual JISDOR sync (return 202 + jobId; Idempotency-Key wajib)
- `GET /api/v1/master/kurs/jisdor-sync/status` — status sinkronisasi JISDOR terakhir
- `GET /api/v1/jobs/{jobId}` + SSE stream — status job upload/sync (M13)
- `GET /api/v1/master/kurs/export?format=csv` — export

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `IDEMPOTENCY_REPLAY`, `IDEMPOTENCY_MISMATCH`, `INTERNAL`

**Komponen reused**: `DataTable.tsx`, `JobProgressPanel.tsx`, `frontend/src/app/master/kurs/_components/KursForm.tsx`, `AuditHistoryTable.tsx`

**Refresh cadence**: JISDOR sync status — polling 1 menit di halaman `/master/kurs/jisdor-sync`; list kurs — manual refresh on-demand; JobProgressPanel upload/sync — SSE push, fallback polling 2 detik.

### Pre-conditions
1. M5 deployed; semua endpoint `/master/kurs/*` return 200
2. M13 job SSE tersedia
3. `frontend/src/app/master/kurs/` files exist; `KursForm.tsx` tersedia
4. `DataTable.tsx`, `JobProgressPanel.tsx` tersedia
5. Tab "Kurs" di shared `/master` layout sudah wired

### Acceptance Criteria

```gherkin
Feature: Master kurs — audit UX §1/§2/§3, manual entry, JISDOR sync, bulk upload JobProgressPanel

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'fx_rate.create', 'fx_rate.update', 'fx_rate.read'
    And M5 kurs endpoints return 200; M13 job SSE tersedia

  Scenario: M17-02-AC1 — List /master/kurs: DataTable UX §1 (sort + page + filter + export)
    When USR-AKUN-001 navigasi ke /master/kurs
    Then DataTable menampilkan daftar kurs dengan UX §1 lengkap:
      | data source    | GET /api/v1/master/kurs?cursor=...&limit=50&sort=tanggal_kurs:desc |
      | kolom minimal  | kode_mata_uang, nama_mata_uang, tanggal_kurs, kurs_jisdor, kurs_manual, sumber, created_at |
      | sort           | semua kolom sortable; default sort=tanggal_kurs:desc; multi-column via shift+click |
      | filter         | filter[kode_mata_uang] (multi-value: USD, EUR, SGD, dst), filter[sumber] (JISDOR | MANUAL), filter[tanggal_kurs] (date range) |
      | filter chip    | setiap filter aktif tampil sebagai chip; tombol "Bersihkan semua filter" |
      | URL state      | filter + sort di URL searchParams (deep-link friendly; bookmark berfungsi) |
      | pagination     | cursor-based; "Halaman X dari ~Y"; Prev/Next; limit 25/50/100/200; default 50 |
      | export         | CSV + XLSX; filter aktif ikut ke export; async jika > 10k row → JobProgressPanel; audit KURS.EXPORT |
    And badge "Sumber" kolom: chip biru "JISDOR" vs chip abu "MANUAL"
    And jika implementasi existing tidak memiliki salah satu dari (sort/page/filter/export): gap dicatat dan diperbaiki dalam M17
    And empty state: "Tidak ada data kurs yang cocok." + CTA "Bersihkan filter" jika filter aktif
    And ROLE-AUDIT: tombol "+ Kurs Baru" dan "Upload Kurs" ABSENT dari DOM (read-only)

  Scenario: M17-02-AC2 — Manual entry /master/kurs/new: form notif UX §2
    When USR-AKUN-001 navigasi ke /master/kurs/new
    Then form menampilkan KursForm dengan field: kode_mata_uang (select), tanggal_kurs (date), kurs_jisdor (numeric), kurs_manual (numeric, opsional), catatan (textarea)
    When USR-AKUN-001 mengisi form dan klik "Simpan Kurs"
    Then tombol "Simpan Kurs" disable + spinner inline (tidak ada double submit)
    And Idempotency-Key UUID v4 di-generate otomatis dan disertakan di header POST
    When server return 201 Created
    Then toast success hijau 4 detik: "Kurs {kode_mata_uang} tanggal {tanggal_kurs} berhasil disimpan." + link "Lihat detail →"
    And form reset ke state kosong setelah sukses
    When POST gagal 400 VALIDATION_FAILED (mis. kurs_jisdor kosong)
    Then toast error merah persistent: "{N} field bermasalah — lihat highlight di form."
    And field bermasalah: border merah + inline message (aria-describedby)
    And tombol kembali enabled; data form dipertahankan
    When POST gagal 409 CONFLICT
    Then toast error persistent: "Kurs {kode_mata_uang} tanggal {tanggal_kurs} sudah ada. Edit entri yang ada atau gunakan tanggal berbeda."

  Scenario: M17-02-AC3 — JISDOR sync /master/kurs/jisdor-sync: trigger manual + JobProgressPanel UX §3
    When USR-AKUN-001 navigasi ke /master/kurs/jisdor-sync
    Then halaman menampilkan:
      | status terakhir | GET /api/v1/master/kurs/jisdor-sync/status → card: tanggal sync terakhir, jumlah currency di-sync, status (SUCCESS/FAILED/RUNNING) |
      | polling status  | otomatis refresh setiap 60 detik (bukan SSE — status endpoint sederhana)             |
      | tombol trigger  | "Sinkronisasi Sekarang" (permission 'fx_rate.create')                                  |
    When USR-AKUN-001 klik "Sinkronisasi Sekarang"
    Then konfirmasi dialog: "Sinkronisasi kurs BI JISDOR sekarang? Kurs hari ini akan di-overwrite dengan data terbaru."
    When USR-AKUN-001 konfirmasi
    Then tombol disable + spinner
    And POST /api/v1/master/kurs/jisdor-sync → 202 Accepted { jobId: "JOB-JISDOR-SYNC-001", statusUrl, streamUrl }
    And Idempotency-Key UUID v4 di-inject otomatis
    And <JobProgressPanel> render di halaman:
      | progress bar   | 0→100%; current step: "Mengambil data dari BI JISDOR API..." / "Menyimpan 15 kurs mata uang..." |
      | ETA display    | "Estimasi selesai: {timestamp}"                                                        |
      | cancel button  | ABSENT (JISDOR sync tidak cancellable — canCancel=false dari job response)              |
    When SSE event: completed
    Then toast success: "Sinkronisasi JISDOR selesai. 15 mata uang diperbarui." + link "Lihat daftar kurs →"
    And status card di halaman diperbarui: tanggal sync = hari ini, status = SUCCESS
    When SSE event: failed (mis. BI API tidak tersedia)
    Then toast error persistent: "Sinkronisasi JISDOR gagal: {error.message}. Kurs hari ini belum tersedia — gunakan entry manual." + link "Entry Manual →"

  Scenario: M17-02-AC4 — Bulk upload /master/kurs/upload: dropzone + JobProgressPanel UX §3
    When USR-AKUN-001 navigasi ke /master/kurs/upload
    Then halaman menampilkan dropzone upload:
      | format diterima | CSV, XLSX (template BI JISDOR atau format internal) |
      | ukuran max      | 10MB; validasi ukuran di frontend sebelum POST       |
      | template link   | "Unduh template CSV" → download template kosong     |
    When USR-AKUN-001 upload file kurs-2026-06-25.csv valid
    Then POST /api/v1/master/kurs/upload → 202 Accepted { jobId, statusUrl, streamUrl }
    And Idempotency-Key UUID v4 di-inject di header POST
    And <JobProgressPanel> render dengan progress parse + insert kurs
    When SSE completed
    Then toast success: "Upload kurs selesai. {N} entri kurs berhasil diimpor." + link "Lihat daftar kurs →"
    When file format tidak valid (mis. Excel dengan kolom berbeda)
    Then frontend validasi format sebelum POST → toast error instant: "Format file tidak valid. Gunakan template yang tersedia." (bukan POST ke server)
```

---

## Story P5-M17-03 — Mapping Jurnal Konsolidasi di `/master/mapping-jurnal/*`: 6-Eyes Workflow Chain UI + 308 Redirect

**Actor**: ROLE-AKUN (submit), ROLE-AKUN-CTL (review), ROLE-RISK (approve step 1), ROLE-KOMITE (approve step 2 + activate — final 6-eyes)
**Trigger**: User navigasi ke `/master/mapping-jurnal` atau klik tab "Mapping Jurnal" di shared `/master` layout.
**Goal**: Semua mapping jurnal screens tersentralisasi di canonical `/master/mapping-jurnal/`; URL duplikat `/mapping-jurnal/*` dan `/jrnl/mapping/*` redirect permanent 308; UI 6-eyes workflow chain (submit → review → approve-1 → approve-2 → activate) dipastikan berfungsi penuh dengan SoD enforcement di setiap step; tombol per step visible hanya untuk role yang berwenang dan ABSENT dari DOM untuk role lain.

**Source OpenAPI**: `api/openapi/app-d-mapping-jurnal.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/master/mapping-jurnal` — list mapping jurnal dengan sort/page/filter/export
- `POST /api/v1/master/mapping-jurnal` — create mapping baru (Maker = ROLE-AKUN; Idempotency-Key wajib)
- `GET /api/v1/master/mapping-jurnal/{id}` — detail mapping
- `PATCH /api/v1/master/mapping-jurnal/{id}` — edit draft (Idempotency-Key wajib)
- `GET /api/v1/master/mapping-jurnal/{id}/history` — riwayat perubahan
- `POST /api/v1/master/mapping-jurnal/{id}/submit` — Maker submit ke Reviewer
- `POST /api/v1/master/mapping-jurnal/{id}/review` — Reviewer (ROLE-AKUN-CTL) sign review
- `POST /api/v1/master/mapping-jurnal/{id}/approve` — Approver-1 (ROLE-RISK) sign
- `POST /api/v1/master/mapping-jurnal/{id}/approve-2` — Approver-2 (ROLE-KOMITE) sign (final 6-eyes)
- `POST /api/v1/master/mapping-jurnal/{id}/activate` — Activate (ROLE-AKUN-CTL setelah 6-eyes selesai)
- `POST /api/v1/master/mapping-jurnal/{id}/reject` — reject oleh reviewer atau approver mana pun
- `GET /api/v1/master/mapping-jurnal/export?format=csv` — export

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `SOD_VIOLATION`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `WORKFLOW_INVALID_TRANSITION`, `IDEMPOTENCY_REPLAY`, `IDEMPOTENCY_MISMATCH`

**Komponen reused**: `DataTable.tsx`, `WorkflowStatusBadge.tsx`, `AuditHistoryTable.tsx`, `SixEyesWorkflowPanel.tsx` (komponen baru untuk 6-eyes jika belum ada — buat dari `MakerReviewerApproverPanel.tsx` pattern), `frontend/src/app/master/mapping-jurnal/_components/MappingJurnalForm.tsx`

**6-eyes state machine**:
```
DRAFT → SUBMITTED → REVIEWED → APPROVED_1 → APPROVED_2 → ACTIVE
                 ↘ REJECTED ←←←←←←←←←←←←←←←←←←←←←←←←←←←↙
```
SoD constraint: maker_id ≠ reviewer_id ≠ approver1_id ≠ approver2_id (4 user berbeda).

### Pre-conditions
1. M12 deployed; semua endpoint `/master/mapping-jurnal/*` return 200; 6-eyes state machine di backend berfungsi
2. `frontend/src/app/master/mapping-jurnal/` canonical files exist
3. `frontend/src/app/mapping-jurnal/` dan `frontend/src/app/jrnl/mapping/` files exist dan siap di-redirect
4. `DataTable.tsx`, `WorkflowStatusBadge.tsx` tersedia
5. Tab "Mapping Jurnal" di shared `/master` layout sudah wired

### Acceptance Criteria

```gherkin
Feature: Mapping jurnal konsolidasi — 308 redirect, 6-eyes workflow chain UI, SoD enforcement per step

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'mapping_jurnal.create', 'mapping_jurnal.submit'
    And ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi (mfa_verified=true) dengan permission 'mapping_jurnal.review', 'mapping_jurnal.activate'
    And ROLE-RISK USR-RISK-001 ter-autentikasi dengan permission 'mapping_jurnal.approve'
    And ROLE-KOMITE USR-KOMITE-001 ter-autentikasi (mfa_verified=true) dengan permission 'mapping_jurnal.approve'
    And M12 mapping jurnal endpoints return 200

  Scenario: M17-03-AC1 — 308 redirect dari /mapping-jurnal/* dan /jrnl/mapping/* ke /master/mapping-jurnal/*
    Given next.config.js `redirects()` sudah dikonfigurasi untuk rute mapping jurnal
    When browser mengakses /mapping-jurnal
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal
    When browser mengakses /mapping-jurnal/{event_code}
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal (list; event_code tidak ada di canonical route)
    When browser mengakses /mapping-jurnal/import
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal/new (paling mendekati)
    When browser mengakses /jrnl/mapping
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal
    When browser mengakses /jrnl/mapping/new
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal/new
    When browser mengakses /jrnl/mapping/{id}
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal/{id}
    When browser mengakses /jrnl/mapping/{id}/edit
    Then HTTP 308 Permanent Redirect ke /master/mapping-jurnal/{id}/edit
    And semua 7 redirect paths tidak menghasilkan 404

  Scenario: M17-03-AC2 — 6-eyes workflow panel: tombol per step visible hanya untuk role yang berwenang
    Given mapping jurnal MJ-001 status=SUBMITTED, maker_id=USR-AKUN-001, reviewer belum sign
    When USR-CTL-001 (ROLE-AKUN-CTL, reviewer) navigasi ke /master/mapping-jurnal/MJ-001
    Then panel 6-eyes menampilkan stepper: [DRAFT ✓] → [SUBMITTED ✓] → [REVIEWED (aktif)] → [APPROVED-1] → [APPROVED-2] → [ACTIVE]
    And tombol "Review & Tandatangani" VISIBLE untuk USR-CTL-001
    And tombol "Approve (Risk)" ABSENT dari DOM untuk USR-CTL-001 (bukan role RISK)
    And tombol "Approve Final (Komite)" ABSENT dari DOM untuk USR-CTL-001
    When USR-AKUN-001 (maker) navigasi ke /master/mapping-jurnal/MJ-001
    Then tombol "Review & Tandatangani" ABSENT dari DOM (SoD: maker tidak bisa menjadi reviewer)
    And banner info: "Menunggu review oleh Finance Controller." tampil sebagai status informational
    When USR-RISK-001 navigasi ke /master/mapping-jurnal/MJ-001 (status masih SUBMITTED, belum REVIEWED)
    Then tombol "Approve (Risk)" ABSENT dari DOM (state machine: belum di-review, tidak bisa approve)
    And banner: "Belum dapat di-approve. Menunggu review dari Finance Controller terlebih dahulu."
    Given MJ-001 status=REVIEWED setelah USR-CTL-001 sign
    When USR-RISK-001 navigasi ke /master/mapping-jurnal/MJ-001
    Then tombol "Approve (Risk)" VISIBLE; tombol "Approve Final (Komite)" ABSENT (belum giliran)
    Given MJ-001 status=APPROVED_1 setelah USR-RISK-001 sign
    When USR-KOMITE-001 navigasi ke /master/mapping-jurnal/MJ-001
    Then tombol "Approve Final (Komite)" VISIBLE (ROLE-KOMITE, mfa_verified=true dari JWT)
    And tidak diperlukan step-up MFA tambahan untuk approve-2 mapping jurnal (DEC-027 step-up hanya untuk hard-close + ECL param + calc-run seal + klasifikasi approve; mapping jurnal KOMITE approve cukup mfa_verified=true di JWT)

  Scenario: M17-03-AC3 — 6-eyes sign: form notification UX §2 + SoD reject di API
    Given MJ-001 status=REVIEWED, reviewer_id=USR-CTL-001
    When USR-RISK-001 klik "Approve (Risk)" di panel 6-eyes + isi komentar "Mapping sesuai standar GL"
    Then tombol disable + spinner; Idempotency-Key UUID v4 di-inject otomatis
    And POST /api/v1/master/mapping-jurnal/MJ-001/approve disertakan { comment, signature_method: "JWT_STEP_UP" }
    When server return 200
    Then toast success 4 detik: "Mapping jurnal MJ-001 berhasil di-approve (Risk). Menunggu approval final dari Komite Investasi."
    And stepper update: [APPROVED-1 ✓] highlighted
    When USR-CTL-001 (reviewer) mencoba langsung POST /api/v1/master/mapping-jurnal/MJ-001/approve
    Then server return 403 SOD_VIOLATION
    And jika terjadi dari UI: tombol "Approve (Risk)" ABSENT dari DOM untuk USR-CTL-001 (reviewer tidak bisa jadi approver)
    Given MJ-001 status=APPROVED_2 setelah USR-KOMITE-001 sign
    When USR-CTL-001 klik "Aktifkan Mapping" (activate)
    Then POST /api/v1/master/mapping-jurnal/MJ-001/activate → 200
    And toast success: "Mapping jurnal MJ-001 berhasil diaktifkan. Jurnal engine akan menggunakan mapping ini mulai sekarang."
    And stepper seluruhnya selesai: semua step ✓; status badge ACTIVE (hijau)
    When approver mana pun klik "Tolak" (reject) + wajib isi alasan penolakan (textarea required)
    Then POST /api/v1/master/mapping-jurnal/MJ-001/reject { reason, rejector_role }
    And toast success 4 detik: "Mapping jurnal MJ-001 ditolak. Maker akan dinotifikasi untuk perbaikan."
    And status badge berubah ke REJECTED (merah); alasan penolakan tampil di panel detail

  Scenario: M17-03-AC4 — List /master/mapping-jurnal: DataTable UX §1 + audit history tab
    When USR-AKUN-001 navigasi ke /master/mapping-jurnal
    Then DataTable menampilkan daftar mapping dengan UX §1:
      | data source    | GET /api/v1/master/mapping-jurnal?cursor=...&limit=50&sort=created_at:desc |
      | kolom minimal  | kode_mapping, event_code, nama_mapping, debit_coa, kredit_coa, status_workflow, active_since |
      | sort           | semua kolom sortable; default sort=created_at:desc |
      | filter         | filter[status_workflow], filter[event_code] (multi-value), ?q= text search nama/kode |
      | filter chip    | setiap filter aktif tampil sebagai chip; tombol "Bersihkan semua filter" |
      | pagination     | cursor-based; default limit 50 |
      | export         | CSV + XLSX; filter aktif; async jika > 10k row; audit MAPPING_JURNAL.EXPORT |
    And kolom "Status" menampilkan WorkflowStatusBadge: DRAFT/SUBMITTED/REVIEWED/APPROVED_1/APPROVED_2/ACTIVE/REJECTED
    And detail /master/mapping-jurnal/{id}: tab "History" menampilkan AuditHistoryTable (GET /api/v1/master/mapping-jurnal/{id}/history)
    And kolom history: tanggal, actor, role, aksi (SUBMIT/REVIEW/APPROVE/REJECT/ACTIVATE), komentar
    And tombol "+ Mapping Baru" ABSENT dari DOM untuk ROLE-APPR-TR, ROLE-RISK, ROLE-AUDIT, ROLE-KOMITE (tidak punya permission mapping_jurnal.create)
```

---

## Story P5-M17-04 — Jurnal Header `/jurnal/header` List + Detail + DLQ Queue

**Actor**: ROLE-AKUN (list + detail, submit), ROLE-AKUN-CTL (approve/reject), ROLE-IT-ADMIN (DLQ list + detail + replay dengan MFA)
**Trigger**: User navigasi ke `/jurnal/header` (list) atau klik tab "Header" di shared `/jurnal` layout.
**Goal**: Halaman `/jurnal/header` (MISSING) dibuat baru — list jurnal header dengan DataTable UX §1, detail jurnal dengan 4-eyes approval panel, workflow submit → approve/reject; DLQ queue untuk jurnal yang gagal di-post ke GL Host tersedia di `/jurnal/dlq/`; DLQ replay action memerlukan MFA step-up (DEC-027); semua file di `/jrnl/journal-entries/` dan `/jrnl/dlq/` di-move ke `/jurnal/` namespace.

**Source OpenAPI**: `api/openapi/app-d-jurnal-engine.yaml`, `api/openapi/app-d-gl-delivery.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/jurnal` — list jurnal header dengan sort/page/filter/export
- `GET /api/v1/jurnal/{id}` — detail jurnal header + line items
- `POST /api/v1/jurnal/{id}/submit` — Maker submit jurnal (Idempotency-Key wajib)
- `POST /api/v1/jurnal/{id}/approve` — Approver sign (ROLE-AKUN-CTL; Idempotency-Key wajib)
- `POST /api/v1/jurnal/{id}/reject` — Approver reject (Idempotency-Key wajib)
- `GET /api/v1/jurnal/export?format=csv` — export jurnal
- `GET /api/v1/jurnal/dlq` — list DLQ entries (jurnal gagal GL delivery)
- `GET /api/v1/jurnal/dlq/{id}` — detail DLQ entry (error detail, retry count, last_error)
- `POST /api/v1/jurnal/dlq/{id}/replay` — replay DLQ entry (ROLE-IT-ADMIN; MFA step-up wajib; Idempotency-Key wajib)
- `GET /api/v1/jobs/{jobId}` + SSE stream — status replay job (M13)

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `SOD_VIOLATION`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `WORKFLOW_INVALID_TRANSITION`, `IDEMPOTENCY_REPLAY`, `IDEMPOTENCY_MISMATCH`, `PERIODE_CLOSED`

**Komponen reused**: `DataTable.tsx`, `WorkflowStatusBadge.tsx`, `AuditHistoryTable.tsx`, `MFAStepUpModal.tsx` (untuk DLQ replay step-up), `JobProgressPanel.tsx`

**MFA gate**: DLQ replay → step-up MFA per DEC-027. IT-ADMIN wajib MFA (DEC-026). `MFAStepUpModal` di-trigger sebelum POST `/jurnal/dlq/{id}/replay`.

### Pre-conditions
1. M2 (jurnal engine) + M3 (GL delivery) deployed; endpoint `/jurnal/*` + `/jurnal/dlq/*` return 200
2. M13 job SSE tersedia
3. `frontend/src/app/jrnl/journal-entries/` dan `/jrnl/dlq/` files exist; siap di-move
4. `/jurnal/header/` dan `/jurnal/dlq/` target paths belum ada → buat baru atau git mv
5. `MFAStepUpModal.tsx` tersedia

### Acceptance Criteria

```gherkin
Feature: Jurnal header /jurnal/header list + detail + DLQ queue dengan MFA step-up replay

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'jurnal.read', 'jurnal.submit'
    And ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi (mfa_verified=true) dengan permission 'jurnal.approve', 'jurnal.reject'
    And ROLE-IT-ADMIN USR-IT-001 ter-autentikasi (mfa_verified=true) dengan permission 'jurnal.dlq.read', 'jurnal.dlq.replay'
    And M2 jurnal + M3 GL delivery endpoints return 200; M13 job SSE tersedia

  Scenario: M17-04-AC1 — List /jurnal/header: DataTable UX §1 (sort + page + filter + export)
    When USR-AKUN-001 navigasi ke /jurnal/header
    Then DataTable menampilkan daftar jurnal header dengan UX §1:
      | data source    | GET /api/v1/jurnal?cursor=...&limit=50&sort=tanggal_jurnal:desc |
      | kolom minimal  | nomor_jurnal, tanggal_jurnal, keterangan, total_debit_idr, total_kredit_idr, periode, status_workflow, created_by |
      | sort           | semua kolom sortable; default sort=tanggal_jurnal:desc |
      | filter         | filter[status_workflow], filter[periode_id], filter[tanggal_jurnal] (date range), ?q= text search nomor/keterangan |
      | filter chip    | chip per filter aktif; tombol "Bersihkan semua filter" |
      | URL state      | sort + filter di URL (deep-link friendly) |
      | pagination     | cursor-based; "Halaman X dari ~Y"; default limit 50 |
      | export         | CSV + XLSX; respect filter; async jika > 10k row → JobProgressPanel; audit JURNAL.EXPORT |
    And status badge: DRAFT (abu), SUBMITTED (biru), APPROVED (hijau), REJECTED (merah), POSTED_TO_GL (ungu)
    And baris POSTED_TO_GL: warna text lebih muted; tidak ada tombol aksi workflow
    And tombol "Submit Jurnal" per baris: VISIBLE hanya untuk ROLE-AKUN yang punya permission 'jurnal.submit' DAN jurnal status=DRAFT
    And loading skeleton, empty state, error state sesuai konvensi DataTable

  Scenario: M17-04-AC2 — Detail /jurnal/header/{id}: line items + 4-eyes approval panel
    When USR-AKUN-001 navigasi ke /jurnal/header/JRN-2026-0042
    Then halaman menampilkan:
      | header info    | nomor_jurnal, tanggal, keterangan, periode, status badge, created_by, timestamps |
      | line items     | tabel: no, kode_coa, nama_coa, keterangan_line, debit_idr, kredit_idr — sum row di footer |
      | workflow panel | stepper 4-eyes: [DRAFT] → [SUBMITTED] → [APPROVED] → [POSTED_TO_GL] |
      | tombol submit  | "Submit ke Approver" VISIBLE untuk USR-AKUN-001 jika status=DRAFT DAN jurnal miliknya |
    When USR-AKUN-001 klik "Submit ke Approver" + konfirmasi dialog
    Then tombol disable + spinner; Idempotency-Key UUID v4 di-inject
    And POST /api/v1/jurnal/JRN-2026-0042/submit → 200
    And toast success: "Jurnal JRN-2026-0042 berhasil di-submit. Menunggu approval Finance Controller."
    And stepper update ke SUBMITTED
    When USR-CTL-001 navigasi ke /jurnal/header/JRN-2026-0042 (status=SUBMITTED)
    Then tombol "Approve Jurnal" VISIBLE; tombol "Tolak" VISIBLE dengan textarea wajib alasan
    When USR-CTL-001 klik "Approve Jurnal"
    Then POST /api/v1/jurnal/JRN-2026-0042/approve { comment, signature_method: "JWT_STEP_UP" } → 200
    And toast success: "Jurnal JRN-2026-0042 disetujui. Akan di-post ke GL pada jadwal berikutnya."
    And stepper update ke APPROVED
    When USR-AKUN-001 mencoba POST /api/v1/jurnal/JRN-2026-0042/approve (maker coba approve sendiri)
    Then server return 403 SOD_VIOLATION; toast error persistent: "Anda tidak bisa menyetujui jurnal yang Anda buat sendiri."

  Scenario: M17-04-AC3 — DLQ /jurnal/dlq: list + detail + replay dengan MFA step-up (DEC-027)
    When USR-IT-001 navigasi ke /jurnal/dlq
    Then DataTable DLQ menampilkan daftar entri gagal dengan UX §1:
      | data source    | GET /api/v1/jurnal/dlq?cursor=...&limit=50&sort=created_at:desc |
      | kolom minimal  | id_dlq, nomor_jurnal, tanggal_gagal, kode_error, retry_count, last_error_message, status |
      | filter         | filter[status] (PENDING | RETRYING | DEAD_LETTER), filter[kode_error] |
      | export         | CSV; audit JURNAL_DLQ.EXPORT |
    When USR-IT-001 klik detail DLQ entry DLQ-001 → /jurnal/dlq/DLQ-001
    Then halaman menampilkan:
      | error detail   | GET /api/v1/jurnal/dlq/DLQ-001 → stack trace, last_error JSON, retry history, created_at/updated_at |
      | linked jurnal  | link "Lihat Jurnal JRN-2026-0042 →" |
      | tombol replay  | "Replay ke GL" VISIBLE untuk USR-IT-001 (permission 'jurnal.dlq.replay') |
    When USR-IT-001 klik "Replay ke GL"
    Then DestructiveActionDialog: "Replay jurnal DLQ-001 ke GL Host? Ini akan mencoba kirim ulang ke sistem GL."
    When USR-IT-001 konfirmasi
    Then MFAStepUpModal muncul (DEC-027 — replay DLQ adalah aksi sensitif setara hard-close)
    When USR-IT-001 input kode MFA valid
    Then POST /api/v1/jurnal/dlq/DLQ-001/replay disertakan X-Step-Up-Token + Idempotency-Key
    And <JobProgressPanel> inline di halaman: progress replay job dengan SSE stream
    When SSE completed (replay berhasil)
    Then toast success: "DLQ-001 berhasil di-replay ke GL Host. Status: DELIVERED."
    When SSE failed
    Then toast error persistent: "Replay DLQ-001 gagal: {error.message}. Entry dikembalikan ke antrian." + traceId
    When ROLE-AKUN-CTL USR-CTL-001 navigasi ke /jurnal/dlq
    Then DataTable DLQ: read-only; tombol "Replay ke GL" ABSENT dari DOM (permission 'jurnal.dlq.replay' tidak ada)

  Scenario: M17-04-AC4 — Shared /jurnal layout: tab nav (Header | DLQ | Resolve) + breadcrumb + role-gated tab
    When USR-AKUN-001 (tanpa jurnal.dlq.read, tanpa jurnal.resolve) navigasi ke /jurnal/header
    Then tab "Header" VISIBLE di JurnalTabNav (permission 'jurnal.read')
    And tab "DLQ" ABSENT dari DOM (tidak punya permission 'jurnal.dlq.read')
    And tab "Resolve" ABSENT dari DOM (tidak punya permission; IT-ADMIN only)
    And breadcrumb: "Beranda / Jurnal / Header"
    When USR-IT-001 navigasi ke /jurnal/dlq
    Then tab "Header" VISIBLE; tab "DLQ" VISIBLE; tab "Resolve" VISIBLE (IT-ADMIN punya semua permission)
    And tab "DLQ" memiliki badge count (GET /api/v1/jurnal/dlq?filter[status]=PENDING count) jika > 0
    And breadcrumb: "Beranda / Jurnal / DLQ"
    When browser mengakses /jrnl/journal-entries
    Then HTTP 308 Permanent Redirect ke /jurnal/header
    When browser mengakses /jrnl/dlq
    Then HTTP 308 Permanent Redirect ke /jurnal/dlq
    When browser mengakses /jrnl/resolve
    Then HTTP 308 Permanent Redirect ke /jurnal/resolve
    When browser mengakses /jrnl/rekonsiliasi
    Then HTTP 308 Permanent Redirect ke /reconciliation/daily
```

---

## Story P5-M17-05 — Reconciliation `/reconciliation/daily`: BLIPS vs GL Host Recon View

**Actor**: ROLE-AKUN-CTL (primary user), ROLE-AUDIT (read-only secondary)
**Trigger**: User navigasi ke `/reconciliation/daily` atau klik menu "Rekonsiliasi" di sidebar.
**Goal**: Halaman baru `/reconciliation/daily` (MISSING) menampilkan view read-only perbandingan BLIPS vs GL Host per tanggal; konsumsi data dari M14 RPT22b (laporan gl_delivery) atau endpoint rekonsiliasi dari `api/openapi/app-d-gl-delivery.yaml`; tampilkan jumlah jurnal BLIPS vs jurnal diterima GL, mismatch count, dan drill-down ke baris mismatch; tidak ada trigger manual reconciliation dari UI (cron-only per DEC-005/scope).

**Source OpenAPI**: `api/openapi/app-d-gl-delivery.yaml` (reconciliation report endpoint), didukung oleh M14 RPT22b registered di report registry

**Endpoint yang dikonsumsi**:
- `GET /api/v1/reports/gl-delivery?tanggal=2026-06-25` — laporan GL delivery harian (M14 RPT22b); return: jumlah jurnal terkirim, status, mismatch_count
- `GET /api/v1/jurnal/dlq?filter[status]=PENDING` — count DLQ entries yang belum terselesaikan
- `GET /api/v1/gl-delivery/recon?tanggal=2026-06-25` — recon summary (total BLIPS vs total GL, selisih nominal)
- `GET /api/v1/gl-delivery/recon/mismatches?tanggal=2026-06-25&cursor=...` — list mismatch line items (DataTable)
- `GET /api/v1/gl-delivery/recon/export?tanggal=2026-06-25&format=csv` — export mismatch report

**Error codes dikonsumsi**: `FORBIDDEN`, `NOT_FOUND`, `INTERNAL`

**Komponen reused**: `DataTable.tsx`, `WorkflowStatusBadge.tsx`

**Refresh cadence**: Halaman ini **tidak ada auto-polling** untuk tombol manual recon. Data recon sudah ter-populate oleh cron job backend. User refresh manual via tombol "Refresh" atau pilih tanggal berbeda.

**Note integrasi**: `sys.gl_reconciliation_report` table exist di backend (confirmed dari task description). `api/openapi/app-d-gl-delivery.yaml` menyediakan recon endpoint. Jika endpoint `/gl-delivery/recon/*` belum expose mismatch detail secara granular, consume M14 RPT22b yang sudah tersedia (registered di report registry M14). Pilihan endpoint final didelegasikan ke `system-analyst` saat handoff untuk konfirmasi contract.

### Pre-conditions
1. M3 (GL delivery) deployed; `sys.gl_reconciliation_report` table populated oleh cron
2. M14 RPT22b endpoint (`GET /api/v1/reports/gl-delivery`) return 200
3. `api/openapi/app-d-gl-delivery.yaml` recon endpoints tersedia (atau fallback ke M14 RPT22b)
4. `DataTable.tsx` tersedia
5. `requirePermission()` dari `lib/auth/permissions.ts` tersedia

### Acceptance Criteria

```gherkin
Feature: Reconciliation /reconciliation/daily — BLIPS vs GL Host recon view read-only

  Background:
    Given ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi (mfa_verified=true) dengan permission 'jurnal.read', 'jurnal.export'
    And ROLE-AUDIT USR-AUDIT-001 ter-autentikasi dengan permission 'audit_log.read', 'jurnal.read'
    And M3 GL delivery + M14 RPT22b endpoints return 200
    And sys.gl_reconciliation_report sudah di-populate oleh backend cron (tidak bergantung pada trigger dari UI)

  Scenario: M17-05-AC1 — Summary card: BLIPS vs GL count + nominal + mismatch count
    When USR-CTL-001 navigasi ke /reconciliation/daily
    Then halaman menampilkan:
      | date picker     | default: hari ini; user bisa pilih tanggal lain untuk lihat recon historis |
      | summary card 1  | "Jurnal BLIPS: {N} entri, Total Debit: Rp {nominal}" (sumber: BLIPS DB) |
      | summary card 2  | "Jurnal Diterima GL: {M} entri, Total Debit: Rp {nominal}" (sumber: GL delivery report) |
      | summary card 3  | "Mismatch: {X} entri" — merah jika X > 0; hijau jika X = 0 |
      | summary card 4  | "DLQ Pending: {Y} entri" — link ke /jurnal/dlq jika Y > 0 |
      | last updated    | "Data per: {timestamp} (diperbarui oleh cron setiap hari pukul 23:59)" |
    And data diambil dari GET /api/v1/gl-delivery/recon?tanggal={selected_date} atau GET /api/v1/reports/gl-delivery?tanggal={selected_date}
    And jika data recon belum tersedia untuk tanggal yang dipilih (hari ini dan cron belum jalan):
    Then tampil banner info: "Data rekonsiliasi untuk tanggal ini belum tersedia. Cron berjalan setiap hari pukul 23:59."
    And tombol "Refresh" manual tersedia; tidak ada tombol "Jalankan Rekonsiliasi" (cron-only — out of scope M17)

  Scenario: M17-05-AC2 — Mismatch DataTable: drill-down ke line items
    Given tanggal 2026-06-24 memiliki 5 mismatch entries
    When USR-CTL-001 memilih tanggal 2026-06-24 di date picker
    Then summary card "Mismatch: 5 entri" tampil dengan warna merah
    When USR-CTL-001 klik "Lihat Detail Mismatch" atau scroll ke DataTable mismatch
    Then DataTable mismatch menampilkan UX §1:
      | data source    | GET /api/v1/gl-delivery/recon/mismatches?tanggal=2026-06-24&cursor=...&limit=50 |
      | kolom minimal  | nomor_jurnal, tanggal_jurnal, nominal_blips_idr, nominal_gl_idr, selisih_idr, jenis_mismatch, status_resolusi |
      | jenis mismatch | MISSING_IN_GL (ada di BLIPS tapi tidak di GL), AMOUNT_DIFF (nominal berbeda), EXTRA_IN_GL (ada di GL tapi tidak di BLIPS) |
      | sort           | semua kolom sortable; default sort=selisih_idr:desc (mismatch terbesar di atas) |
      | filter         | filter[jenis_mismatch], filter[status_resolusi] (OPEN | RESOLVED | ACKNOWLEDGED) |
      | pagination     | cursor-based; default limit 50 |
      | export         | CSV + XLSX; filter aktif; async jika > 10k; audit REKON_HARIAN.EXPORT |
    And baris MISSING_IN_GL: highlight background kuning muda
    And baris AMOUNT_DIFF: highlight background oranye muda
    And baris EXTRA_IN_GL: highlight background merah muda
    And link "Lihat Jurnal →" per baris (jika nomor_jurnal tersedia) → /jurnal/header/{id}
    And link "Lihat DLQ →" per baris jika entri ada di DLQ → /jurnal/dlq/{id}
    And tidak ada tombol "Resolve" atau aksi mutasi di DataTable ini (read-only view; resolusi via DLQ replay di /jurnal/dlq)

  Scenario: M17-05-AC3 — Historis: navigasi recon per tanggal + URL state
    When USR-CTL-001 mengubah date picker ke 2026-06-20
    Then URL update ke /reconciliation/daily?tanggal=2026-06-20
    And DataTable mismatch re-fetch GET /api/v1/gl-delivery/recon/mismatches?tanggal=2026-06-20&cursor=...
    And summary cards re-fetch dengan parameter tanggal baru
    And tombol browser "Back" kembali ke tanggal sebelumnya (URL state preserved)
    And user bisa share URL /reconciliation/daily?tanggal=2026-06-20 → page restore tanggal yang benar

  Scenario: M17-05-AC4 — Role gate: read-only page; ROLE-AKUN tidak punya akses; ROLE-AUDIT punya akses
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi tanpa permission 'jurnal.read' (scope recon)
    When USR-AKUN-001 navigasi ke /reconciliation/daily
    Then HTTP 403 redirect ke /dashboard (ROLE-AKUN tidak punya akses ke recon view)
    And server component permission check: 'jurnal.read' wajib; dilakukan di server sebelum render (bukan client redirect)
    Given ROLE-AUDIT USR-AUDIT-001 ter-autentikasi dengan permission 'audit_log.read'
    When USR-AUDIT-001 navigasi ke /reconciliation/daily
    Then halaman tersedia read-only; DataTable + summary cards tampil
    And tombol "Ekspor" VISIBLE untuk ROLE-AUDIT (permission 'jurnal.export' — AUDIT dapat export per konvensi aud.*.read)
    And tidak ada tombol mutasi mana pun (halaman ini full read-only untuk semua role)
    When USR-CTL-001 navigasi ke /reconciliation/daily?tanggal=2026-06-25
    Then breadcrumb: "Beranda / Rekonsiliasi / Harian"
    And page title: "Rekonsiliasi Harian — 25 Juni 2026"
    And metadata `<title>` di head: "Rekonsiliasi Harian | BLIPS"
```

---

## Ringkasan P5-M17 Story Set

| Story | Judul | Actor Utama | OpenAPI Ref | AC Count | Gate |
|---|---|---|---|---|---|
| P5-M17-01 | Periode buku konsolidasi `/periode-buku/*` + 308 redirect + timeline + closing workflow + MFA step-up | ROLE-AKUN-CTL, ROLE-CFO | `app-d-periode-close.yaml` | 4 | security BLOCKING (MFA step-up hard-close, absent-from-DOM) |
| P5-M17-02 | Master kurs audit UX §1/§2/§3 + manual entry + JISDOR sync + bulk upload JobProgressPanel | ROLE-AKUN | `app-d-fx-rate.yaml` | 4 | security BLOCKING (role gate upload/sync) |
| P5-M17-03 | Mapping jurnal konsolidasi `/master/mapping-jurnal/*` + 308 redirect + 6-eyes workflow chain UI | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-KOMITE | `app-d-mapping-jurnal.yaml` | 4 | security BLOCKING (SoD 6-eyes, absent-from-DOM per step) |
| P5-M17-04 | Jurnal header `/jurnal/header` list + detail + DLQ + replay MFA step-up | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-IT-ADMIN | `app-d-jurnal-engine.yaml`, `app-d-gl-delivery.yaml` | 4 | security BLOCKING (SoD 4-eyes, DLQ replay step-up MFA, absent-from-DOM) |
| P5-M17-05 | Reconciliation `/reconciliation/daily` BLIPS vs GL recon view read-only | ROLE-AKUN-CTL, ROLE-AUDIT | `app-d-gl-delivery.yaml`, M14 RPT22b | 4 | security BLOCKING (read-only gate, absent-from-DOM; tidak ada mutation) |
| **Total** | | | | **20** | |

---

## 308 Redirects yang Perlu Ditambahkan ke `next.config.js` (Append ke M16 List)

| Source path | Destination | Catatan |
|---|---|---|
| `/master/periode-buku` | `/periode-buku` | CRUD CRUD pindah ke canonical |
| `/master/periode-buku/new` | `/periode-buku/new` | |
| `/master/periode-buku/:id` | `/periode-buku/:id` | wildcard |
| `/master/periode-buku/:id/edit` | `/periode-buku/:id/edit` | |
| `/master/periode-buku/:id/history` | `/periode-buku/:id/history` | |
| `/mapping-jurnal` | `/master/mapping-jurnal` | duplikat namespace 1 |
| `/mapping-jurnal/import` | `/master/mapping-jurnal/new` | closest equivalent |
| `/mapping-jurnal/:event_code` | `/master/mapping-jurnal` | event_code route tidak ada di canonical |
| `/jrnl/mapping` | `/master/mapping-jurnal` | duplikat namespace 2 |
| `/jrnl/mapping/new` | `/master/mapping-jurnal/new` | |
| `/jrnl/mapping/:id` | `/master/mapping-jurnal/:id` | |
| `/jrnl/mapping/:id/edit` | `/master/mapping-jurnal/:id/edit` | |
| `/jrnl/journal-entries` | `/jurnal/header` | namespace baru |
| `/jrnl/journal-entries/:id` | `/jurnal/header/:id` | |
| `/jrnl/dlq` | `/jurnal/dlq` | namespace baru |
| `/jrnl/dlq/:id` | `/jurnal/dlq/:id` | |
| `/jrnl/resolve` | `/jurnal/resolve` | namespace baru |
| `/jrnl/rekonsiliasi` | `/reconciliation/daily` | namespace baru |
| `/jrnl/rekonsiliasi/riwayat` | `/reconciliation/daily` | riwayat dihandle via date picker |
| `/jrnl/gl-delivery-dlq` | `/jurnal/dlq` | alias lain ke DLQ |
| `/jrnl/gl-delivery-dlq/:id` | `/jurnal/dlq/:id` | |
| `/jrnl/post` | `/jurnal/header` | posting sekarang via header list |

Total: 22 redirect rules baru dari M17 (ditambah 10 dari M16 = 32 total di `next.config.js`).

---

## Error Codes Dikonsumsi (sudah ada di `api/openapi/_common.yaml`)

| Code | HTTP | Trigger dalam M17 |
|---|---|---|
| `VALIDATION_FAILED` | 400 | Form submit periode/kurs/mapping dengan field wajib kosong |
| `UNAUTHORIZED` | 401 | JWT expired saat user di halaman |
| `FORBIDDEN` | 403 | Permission tidak ada; MFA step-up token invalid/missing |
| `SOD_VIOLATION` | 403 | 6-eyes: user yang sama mencoba menjadi dua role berbeda dalam satu workflow |
| `NOT_FOUND` | 404 | Detail page dengan ID tidak valid |
| `CONFLICT` | 409 | Optimistic lock mismatch pada edit |
| `IDEMPOTENCY_REPLAY` | 200 | Retry submit yang sudah berhasil sebelumnya |
| `IDEMPOTENCY_MISMATCH` | 422 | Same key, payload berbeda |
| `WORKFLOW_INVALID_TRANSITION` | 422 | Hard-close pada periode OPEN (harus soft-close dulu), atau approve step yang belum urutan |
| `PERIODE_CLOSED` | 423 | Mutasi jurnal pada periode yang sudah hard-closed |

Tidak ada error code baru yang diusulkan dari M17.

---

## Audit Events — APP-D (wajib per DEC-018)

| Event | Trigger | In-transaction |
|---|---|---|
| `PERIODE.EXPORT` | Export DataTable periode buku (CSV/XLSX) | Ya |
| `KURS.EXPORT` | Export DataTable kurs (CSV/XLSX) | Ya |
| `MAPPING_JURNAL.EXPORT` | Export DataTable mapping jurnal (CSV/XLSX) | Ya |
| `JURNAL.EXPORT` | Export DataTable jurnal header (CSV/XLSX) | Ya |
| `JURNAL_DLQ.EXPORT` | Export DLQ DataTable (CSV) | Ya |
| `REKON_HARIAN.EXPORT` | Export mismatch DataTable reconciliation (CSV/XLSX) | Ya |

Audit events untuk workflow actions (submit/review/approve/reject/soft-close/hard-close/replay) sudah ditangani di backend M2/M4/M5/M12 — tidak perlu duplikasi dari frontend.

---

## Refresh Cadence Summary

| Screen | Refresh Mode | Interval | Notes |
|---|---|---|---|
| Semua DataTable list | Manual (filter/sort/paging trigger re-fetch) | On demand | Tombol "Refresh" manual di action bar |
| Kurs upload JobProgressPanel | SSE push | Real-time | Fallback polling 2 detik jika SSE error |
| JISDOR sync JobProgressPanel | SSE push | Real-time | Fallback polling 2 detik jika SSE error |
| JISDOR sync status card | Polling otomatis | 60 detik | Hanya di halaman /master/kurs/jisdor-sync |
| DLQ replay JobProgressPanel | SSE push | Real-time | Fallback polling 2 detik jika SSE error |
| Reconciliation summary cards | Manual (date picker change + tombol Refresh) | On demand | Data sudah dari cron; tidak ada live update |
| WorkflowStatusBadge di detail | Re-fetch on action success | Triggered event | Setelah workflow action berhasil, re-fetch GET /{id} |
| Tab DLQ badge count | Manual refresh | On demand | Bukan polling; update saat user navigate ke tab |

---

## Shared `/master` Layout — Update Tab Nav

Tab "Periode Buku" dihapus dari `/master` layout (namespace dipindah ke `/periode-buku/` standalone). Tab yang tetap ada di `/master` layout setelah M17:

| Tab | Route | Permission | Sudah ada |
|---|---|---|---|
| Instrumen | `/master/instrumen` | `instrumen.read` | Ya (M11) |
| Counterparty | `/master/counterparty` | `counterparty.read` | Ya |
| Portofolio | `/master/portofolio` | `portofolio.read` | Ya |
| Bank / COA | `/master/coa` | `coa.read` | Ya |
| Kurs | `/master/kurs` | `fx_rate.read` | Ya (M5) — M17 audit |
| Mapping Jurnal | `/master/mapping-jurnal` | `mapping_jurnal.read` | Ya (M12) — M17 audit 6-eyes UI |
| Rating | `/master/pd-pefindo` | `pd.read` | Ya |
| LGD / LPS | `/master/lgd-basel`, `/master/lps-coverage` | `lgd.read` | Ya |

Periode Buku tab: jika sebelumnya ada di `/master` layout, hapus dari sana dan arahkan ke `/periode-buku` standalone via sidebar item terpisah.

---

## Handoff Berikutnya

- `uiux-designer` (paralel dengan security review) → wireframe:
  - `/periode-buku` layout: timeline stepper visual per periode (tanggal mulai → tanggal selesai → status); closing action buttons placement; `MFAStepUpModal` placement (centred modal, tidak desain ulang dari M4)
  - `/jurnal` layout: tab nav 3 slot (Header | DLQ | Resolve); DLQ badge count di tab; breadcrumb
  - `/reconciliation/daily`: summary cards layout (2×2 grid); mismatch DataTable dengan row highlight color coding; date picker di header halaman
  - 6-eyes stepper visual di `/master/mapping-jurnal/{id}` — 6 step dengan indicator per role yang sudah sign
  - Semua empty/loading/error states per DataTable (konsisten dengan M16 convention)

- `frontend-engineer-nextjs` → implementasi setelah `uiux-designer` selesai:
  - `next.config.js` update: tambah 22 redirect rules M17
  - `git mv`: journal-entries→jurnal/header; jrnl/dlq→jurnal/dlq; jrnl/resolve→jurnal/resolve; master/periode-buku → periode-buku
  - Buat baru: `/jurnal/layout.tsx` + `JurnalTabNav.tsx`, `/jurnal/header/page.tsx`, `/jurnal/header/[id]/page.tsx`, `/reconciliation/daily/page.tsx`
  - Upgrade existing M4: `/periode-buku/page.tsx` (timeline DataTable) + `/periode-buku/[id]/page.tsx` (closing workflow panel + MFA step-up wiring)
  - Audit + fix gaps: `/master/kurs/` (UX §1/§2/§3), `/master/mapping-jurnal/` (6-eyes chain UI)
  - Reuse `MFAStepUpModal` dari M4 — tidak perlu import ulang atau modifikasi
  - `requirePermission()` + `requirePermissionWithMfa()` dari `lib/auth/permissions.ts` digunakan di semua server components

- `security-engineer` → **BLOCKING**:
  - MFA step-up hard-close: verifikasi `X-Step-Up-Token` validated server-side sebelum `/periode/{id}/hard-close` di-execute; token dari Keycloak step-up endpoint, bukan generated client-side
  - MFA step-up DLQ replay: sama dengan hard-close pattern
  - 6-eyes SoD: server component check per step — maker_id ≠ reviewer_id ≠ approver1_id ≠ approver2_id; tidak hanya UI hide tetapi juga API-level di M12 backend
  - absent-from-DOM: setiap tombol workflow, tombol create, dan tombol destructive di-check dari JWT server component — tidak ada CSS display:none untuk keamanan
  - 308 redirect tidak bocorkan data partial (redirect sebelum page render)
  - Export RBAC: data yang diexport di-filter sesuai permission user; tidak ada client-side filter bypass
  - Idempotency-Key: auto-inject via shared form hook di semua mutation endpoints; verifikasi audit via E2E test sampling

- `qa-engineer` → E2E tests per AC (Playwright):
  - 308 redirect tests: 22 paths baru + regression 10 paths M16
  - MFA step-up: mock step-up token injection; hard-close + DLQ replay tests
  - 6-eyes workflow: 4 user fixtures (AKUN, AKUN-CTL, RISK, KOMITE); SoD violation test via direct API call
  - DataTable UX §1: sort/filter/export per screen (5 screens)
  - Role gate tests: 10 role fixtures × 5 screens = 50 absence-from-DOM assertions kritis
  - Reconciliation: date picker navigation; mismatch DataTable; export

- `system-analyst` → konfirmasi 1 item sebelum implementasi:
  - `/reconciliation/daily` endpoint contract: apakah `/api/v1/gl-delivery/recon?tanggal=...` + `/api/v1/gl-delivery/recon/mismatches?tanggal=...` sudah di-spec di `api/openapi/app-d-gl-delivery.yaml`, atau perlu fallback murni ke M14 RPT22b? Jika endpoint recon granular belum ada di OpenAPI, system-analyst perlu addendum spec sebelum frontend implementasi.

_Story set P5-M17 siap dihandoff ke `uiux-designer` dan `security-engineer` secara paralel. `frontend-engineer-nextjs` mulai setelah kedua review tersebut selesai. Tidak ada `ifrs9-compliance-reviewer` gate — M17 murni UI layer, tidak compute ECL/EIR/SPPI/BM baru._
