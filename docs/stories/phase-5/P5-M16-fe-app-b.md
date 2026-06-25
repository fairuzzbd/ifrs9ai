# P5-M16 — APP-B Frontend Transaction Screens: Next.js Konsolidasi + Audit UX: User Stories

**Story Set ID**: P5-M16
**Modul**: APP-B — Transaction Lifecycle (Frontend-only)
**Status**: DRAFT — menunggu handoff ke `uiux-designer` (tab layout + redirect path review); `frontend-engineer-nextjs` (implementasi); `security-engineer` (role-gate audit BLOCKING)
**Author**: business-analyst
**Tanggal**: 2026-06-25
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §4 (APP-B Transaction Lifecycle); FSD-APP-B-*.docx
**Linked BRD**: BRD §4.2 (APP-B transaksi penempatan, MTM, renewal, penjualan, jatuh tempo, akrual); §3 RACI: ROLE-MAKER-TR (R), ROLE-APPR-TR (A), ROLE-AKUN (R), ROLE-AKUN-CTL (A)
**Linked Decision Log**:
- `DEC-002` (LOCKED) — Next.js 14+ App Router, TypeScript strict, shadcn/ui, React Hook Form + Zod, Zustand, TanStack Query
- `DEC-017` (LOCKED) — 4-eyes workflow; SoD maker ≠ reviewer ≠ approver enforced server-side
- `DEC-018` (LOCKED) — audit trail append-only; `{ENTITY}.EXPORT` wajib tiap export
- `DEC-020` (LOCKED) — REST `/api/v1/`; camelCase JSON; snake_case DB
- `DEC-021` (LOCKED) — Idempotency-Key wajib pada setiap mutation endpoint
- `DEC-022` (LOCKED) — cursor-based pagination only; no offset
- `DEC-025` (LOCKED) — JWT RSA-2048; permission check via `{entity}.{action}`, bukan role string
- `DEC-026` (LOCKED) — MFA mandatory: ROLE-AKUN-CTL (untuk approve jurnal); ROLE-CFO (hard-close); screen ini tidak trigger MFA kecuali ROLE-AKUN-CTL approve workflow

**Dependensi (WAJIB selesai sebelum M16)**:
- **P5-M1** (commit tersedia) — `POST /api/v1/transaksi/penempatan` + full workflow endpoints (submit/review/approve/reject); OpenAPI: `api/openapi/app-b-penempatan-deposito.yaml`
- **P5-M6** (commit tersedia) — MTM daily endpoints; OpenAPI: `api/openapi/app-b-mtm.yaml`
- **P5-M7** (commit tersedia) — Renewal deposito endpoints; OpenAPI: `api/openapi/app-b-renewal-deposito.yaml`
- **P5-M8** (commit tersedia) — Penjualan endpoints; OpenAPI: `api/openapi/app-b-penjualan.yaml`
- **P5-M9** (commit tersedia) — Jatuh tempo + akrual endpoints; OpenAPI: `api/openapi/app-b-jatuh-tempo-akrual.yaml`
- **P5-M11** (commit tersedia) — Bulk upload endpoints; OpenAPI: `api/openapi/app-b-bulk-upload.yaml`
- **P5-M13** (commit tersedia) — `sys.job` table + `GET /api/v1/jobs/{jobId}` + SSE `GET /api/v1/jobs/{jobId}/stream` + `POST /api/v1/jobs/{jobId}/cancel`
- **`frontend/src/components/blips/DataTable.tsx`** — DataTable UX §1 (sort+page+filter+export) tersedia
- **`frontend/src/components/blips/JobProgressPanel.tsx`** — JobProgressPanel UX §3 tersedia
- **`frontend/src/components/blips/penempatan/*`** — komponen penempatan sudah ada di `/trx/penempatan/`

**Gate**:
- `security-engineer` **BLOCKING** — (a) absent-from-DOM per role; (b) 308 redirect tidak bocorkan data lama; (c) permission check server component; (d) Idempotency-Key auto-inject di setiap form submit. Tidak ada `ifrs9-compliance-reviewer` gate (M16 tidak compute ECL/EIR/SPPI/BM — murni UI layer atas endpoint yang sudah ada).

---

## Konteks & Scope P5-M16

P5-M16 adalah **frontend-only modul** yang mengkonsolidasi semua screen APP-B di bawah namespace `/transaksi/*` dan mengaudit gap UX terhadap tiga pola wajib:

- **UX §1** DataTable: sort + paging + filter + export (CSV/XLSX)
- **UX §2** Form notification: sukses spesifik, gagal persistent, pending inline spinner
- **UX §3** JobProgressPanel: untuk MTM batch upload dan akrual harian batch trigger

### State saat ini (hasil baca tree)

| Path sekarang | Isi | Masalah |
|---|---|---|
| `frontend/src/app/trx/penempatan/` | list `page.tsx`, `new/page.tsx`, `[id]/page.tsx`, `[id]/edit/page.tsx` | Namespace salah — harus `/transaksi/penempatan/` |
| `frontend/src/app/mtm/` | list `page.tsx`, `upload/page.tsx`, `upload/batch/[batch_id]/page.tsx`, `cron/page.tsx`, `[id]/page.tsx`, `alerts/stale-price/page.tsx` | Namespace salah — harus `/transaksi/mtm/` |
| `frontend/src/app/transaksi/renewal/` | list, new, [id], [id]/preview | Namespace benar; perlu audit UX |
| `frontend/src/app/transaksi/penjualan/` | list, new, [id], bm-alerts | Namespace benar; perlu audit UX |
| `frontend/src/app/transaksi/akrual/` | list, dashboard, [id] | Namespace benar; batch trigger perlu JobProgressPanel |
| `frontend/src/app/transaksi/jatuh-tempo/` | list (read-only) | Namespace benar; perlu audit UX §1 |

### Target state setelah M16

```
frontend/src/app/transaksi/
  layout.tsx                    ← NEW: shared layout dengan tab nav 6 sub-routes
  penempatan/
    page.tsx                    ← git mv dari /trx/penempatan/page.tsx
    new/page.tsx                ← git mv dari /trx/penempatan/new/page.tsx
    [id]/page.tsx               ← git mv dari /trx/penempatan/[id]/page.tsx
    [id]/edit/page.tsx          ← git mv dari /trx/penempatan/[id]/edit/page.tsx
  mtm/
    page.tsx                    ← git mv dari /mtm/page.tsx
    upload/page.tsx             ← git mv dari /mtm/upload/page.tsx + audit JobProgressPanel
    upload/batch/[batch_id]/page.tsx ← git mv dari /mtm/upload/batch/[batch_id]/page.tsx
    cron/page.tsx               ← git mv dari /mtm/cron/page.tsx
    [id]/page.tsx               ← git mv dari /mtm/[id]/page.tsx
    alerts/stale-price/page.tsx ← git mv dari /mtm/alerts/stale-price/page.tsx
  renewal/
    page.tsx, new/page.tsx, [id]/page.tsx, [id]/preview/page.tsx (sudah ada, audit only)
  penjualan/
    page.tsx, new/page.tsx, [id]/page.tsx, bm-alerts/page.tsx (sudah ada, audit only)
  jatuh-tempo/
    page.tsx (sudah ada, audit only)
  akrual/
    page.tsx, dashboard/page.tsx, [id]/page.tsx (sudah ada; audit + batch trigger fix)

next.config.js                  ← UPDATE: tambah 308 redirects
```

### Tidak di-scope M16 (eksplisit)

- Tidak ada mutating backend endpoint baru — semua form submit ke endpoint M1/M6/M7/M8/M9 yang sudah ada.
- Tidak ada migration database baru.
- ECL/EIR detail screens — scope M17 (APP-D) atau M14/M15 (reporting).
- Klasifikasi PSAK 71 screens — scope M1 (APP-A).
- Phase 6 customization (drag-and-drop, theme, widget preference).
- Penjualan FVOCI election popup flow — scope M8; M16 hanya audit gap UX di existing screen.

---

## Persona Table

| Role | Sub-routes yang diakses | Permission wajib | MFA |
|---|---|---|---|
| ROLE-MAKER-TR | penempatan (create, list, detail, edit), renewal (new, list), penjualan (new, list), mtm (list, upload) | `penempatan.create`, `penempatan.read`, `renewal.create`, `penjualan.create`, `transaksi.mtm.upload` | Tidak wajib |
| ROLE-APPR-TR | penempatan (list, detail — review/approve), semua sub-routes read-only | `penempatan.review`, `penempatan.approve`, `transaksi.*.read` | Tidak wajib (kecuali Treasury Manager senior) |
| ROLE-AKUN | mtm (upload, cron, alerts), akrual (batch trigger, list) | `transaksi.mtm.upload`, `transaksi.akrual.create`, `transaksi.*.read` | Tidak wajib |
| ROLE-AKUN-CTL | akrual (approve batch), jatuh-tempo (monitoring read) | `transaksi.akrual.approve`, `transaksi.jatuh-tempo.read` | WAJIB |
| ROLE-AUDIT | semua sub-routes read-only; tidak ada aksi mutasi | `transaksi.*.read` (via `aud.*.read`) | Tidak wajib |
| ROLE-RISK | semua sub-routes read-only | `transaksi.*.read` | Tidak wajib |

---

## Deliverables M16

| # | Artefak | Tipe | Keterangan |
|---|---|---|---|
| 1 | `frontend/src/app/transaksi/layout.tsx` | Next.js layout | Shared tab nav + breadcrumb |
| 2 | `frontend/src/app/transaksi/penempatan/` (4 files) | Next.js pages | git mv dari `/trx/penempatan/`; audit gap |
| 3 | `frontend/src/app/transaksi/mtm/` (6 files) | Next.js pages | git mv dari `/mtm/`; audit + JobProgressPanel upload |
| 4 | `frontend/src/app/transaksi/renewal/` (existing 4 files) | Audit + fix | Audit UX §1/§2; fix gap |
| 5 | `frontend/src/app/transaksi/penjualan/` (existing 4 files) | Audit + fix | Audit UX §1/§2; BM-alerts widget |
| 6 | `frontend/src/app/transaksi/jatuh-tempo/` (existing 1 file) | Audit + fix | Audit UX §1 DataTable |
| 7 | `frontend/src/app/transaksi/akrual/` (existing 3 files) | Audit + fix | Batch trigger + JobProgressPanel |
| 8 | `next.config.js` (update) | Config | 308 redirects dari `/trx/penempatan/*` + `/mtm/*` |
| 9 | `frontend/src/components/blips/transaksi/TransaksiTabNav.tsx` | Component | Tab navigation 6 sub-routes |
| 10 | `frontend/src/components/blips/transaksi/index.ts` | Component index | Barrel export |

---

## Story P5-M16-01 — Penempatan Screens Konsolidasi di `/transaksi/penempatan`

**Actor**: ROLE-MAKER-TR (create + edit), ROLE-APPR-TR (review/approve), semua role read (list + detail)
**Trigger**: User navigasi ke `/transaksi/penempatan` atau klik tab "Penempatan" di layout transaksi.
**Goal**: Semua penempatan screens tersentralisasi di `/transaksi/penempatan/`; URL lama `/trx/penempatan/*` redirect permanent ke namespace baru; DataTable list memenuhi UX §1; form submit memenuhi UX §2; workflow 4-eyes Maker-Reviewer-Approver berfungsi penuh.

**Source OpenAPI**: `api/openapi/app-b-penempatan-deposito.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/transaksi/penempatan` — list dengan sort/page/filter/export
- `POST /api/v1/transaksi/penempatan` — create (Maker; Idempotency-Key wajib)
- `GET /api/v1/transaksi/penempatan/{id}` — detail
- `PATCH /api/v1/transaksi/penempatan/{id}` — edit draft (sebelum submit; Idempotency-Key wajib)
- `POST /api/v1/transaksi/penempatan/{id}/submit` — Maker submit
- `POST /api/v1/transaksi/penempatan/{id}/review` — Reviewer sign
- `POST /api/v1/transaksi/penempatan/{id}/approve` — Approver sign
- `POST /api/v1/transaksi/penempatan/{id}/reject` — Reviewer/Approver reject
- `GET /api/v1/transaksi/penempatan/export?format=csv` — export (async jika > 10k row → job + JobProgressPanel)

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `SOD_VIOLATION`, `WORKFLOW_INVALID_TRANSITION`, `IDEMPOTENCY_REPLAY`, `IDEMPOTENCY_MISMATCH`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`

**Komponen reused**: `frontend/src/components/blips/penempatan/*` (semua sudah ada), `DataTable.tsx`, `MakerReviewerApproverPanel.tsx`, `WorkflowStatusBadge.tsx`, `AuditHistoryTable.tsx`

### Pre-conditions
1. M1 deployed; semua endpoint `GET/POST /api/v1/transaksi/penempatan*` return 200
2. `frontend/src/app/trx/penempatan/` files exist dan siap di-relocate via `git mv`
3. `DataTable.tsx`, `JobProgressPanel.tsx`, komponen penempatan blips semua tersedia
4. `next.config.js` `redirects()` dapat di-extend

### Acceptance Criteria

```gherkin
Feature: Penempatan screens konsolidasi — relokasi namespace, 308 redirect, DataTable, form notif

  Background:
    Given ROLE-MAKER-TR USR-MAKER-001 ter-autentikasi dengan permission 'penempatan.create', 'penempatan.read'
    And M1 penempatan endpoints semua return 200
    And DataTable.tsx + komponen blips/penempatan/* tersedia

  Scenario: M16-01-AC1 — 308 redirect dari /trx/penempatan/* ke /transaksi/penempatan/*
    Given next.config.js `redirects()` sudah dikonfigurasi untuk rute penempatan
    When browser mengakses /trx/penempatan
    Then HTTP 308 Permanent Redirect ke /transaksi/penempatan
    When browser mengakses /trx/penempatan/new
    Then HTTP 308 Permanent Redirect ke /transaksi/penempatan/new
    When browser mengakses /trx/penempatan/{id}
    Then HTTP 308 Permanent Redirect ke /transaksi/penempatan/{id}
    When browser mengakses /trx/penempatan/{id}/edit
    Then HTTP 308 Permanent Redirect ke /transaksi/penempatan/{id}/edit
    And semua bookmark/link lama tetap berfungsi — tidak ada 404

  Scenario: M16-01-AC2 — List /transaksi/penempatan: DataTable UX §1 sort + page + filter + export
    When USR-MAKER-001 navigasi ke /transaksi/penempatan
    Then DataTable menampilkan daftar penempatan dengan:
      | data source   | GET /api/v1/transaksi/penempatan?cursor=...&limit=50&sort=tanggal_penempatan:desc |
      | kolom minimal | kode_penempatan, jenis_instrumen, counterparty, nominal_idr, tanggal_penempatan, tanggal_jatuh_tempo, stage, workflow_status |
      | sort          | semua kolom sortable; header klik toggle asc→desc→none; icon indicator; default sort=tanggal_penempatan:desc |
      | filter        | text search global (?q=); filter[jenis_instrumen], filter[workflow_status], filter[stage], filter[counterparty_id] |
      | filter chips  | setiap filter aktif tampil sebagai chip; tombol "Bersihkan semua filter" |
      | URL state     | sort, filter, q tersimpan di URL search params (deep-link friendly) |
      | pagination    | cursor-based; "Halaman X dari ~Y"; Prev/Next; limit selector 25/50/100/200; default 50 |
      | export        | tombol "Ekspor ▾" dropdown: CSV + XLSX; respect filter+sort aktif |
    And export < 10k row: streaming langsung ke browser; Content-Disposition: attachment
    And export ≥ 10k row: POST job → 202 Accepted {jobId} → JobProgressPanel inline di export area → download link
    And setiap export: audit `PENEMPATAN.EXPORT` di `aud.audit_log` di same transaction
    And empty state: ilustrasi + "Tidak ada penempatan yang cocok dengan filter." + "Bersihkan filter" CTA
    And loading state: skeleton rows (bukan blank screen)
    And error state: pesan + error code + traceId + tombol "Coba lagi"

  Scenario: M16-01-AC3 — Form /transaksi/penempatan/new: form notif UX §2 (sukses, gagal, pending)
    When USR-MAKER-001 mengisi form penempatan baru dan klik "Simpan sebagai Draft"
    Then tombol "Simpan sebagai Draft" disable + spinner inline (tidak ada double submit)
    And Idempotency-Key UUID v4 di-generate otomatis dan disertakan di header POST
    When server return 201 Created
    Then toast success hijau 4 detik: "Penempatan {kode_penempatan} berhasil dibuat sebagai draft. Menunggu submit ke reviewer."
    And toast menyertakan link "Lihat detail →" ke /transaksi/penempatan/{id}
    And form reset ke state kosong setelah success konfirmasi diterima
    When USR-MAKER-001 submit form dengan field wajib kosong (mis. counterparty_id kosong)
    Then server return 400 VALIDATION_FAILED
    Then toast error merah persistent (tidak auto-dismiss): "3 field bermasalah — lihat highlight di form."
    And setiap field error: highlight merah border + inline message di bawah field dengan aria-describedby
    And toast menyertakan error code VALIDATION_FAILED + traceId (truncated 8 char)
    And tombol "Simpan" kembali enabled; form tidak di-reset (data user dipertahankan)
    When server return 409 CONFLICT (row_version mismatch pada edit)
    Then toast error persistent: "Data sudah diubah oleh pengguna lain. Muat ulang halaman untuk melihat versi terbaru."

  Scenario: M16-01-AC4 — Workflow submit → review → approve; SoD enforcement di UI
    Given penempatan PNP-001 status=DRAFT dibuat oleh USR-MAKER-001
    When USR-MAKER-001 klik "Submit ke Reviewer" di detail /transaksi/penempatan/PNP-001
    Then MakerReviewerApproverPanel tampilkan konfirmasi dialog; USR-MAKER-001 konfirmasi
    And POST /api/v1/transaksi/penempatan/PNP-001/submit → 200; toast success: "PNP-001 berhasil di-submit ke reviewer. Menunggu tanda tangan reviewer."
    When ROLE-APPR-TR USR-APPR-001 (berbeda dari USR-MAKER-001) navigasi ke /transaksi/penempatan/PNP-001
    Then tombol "Review & Tandatangani" visible untuk USR-APPR-001
    When USR-MAKER-001 mencoba mengakses tombol "Review" pada penempatan miliknya sendiri
    Then tombol "Review & Tandatangani" ABSENT dari DOM (SoD: maker tidak bisa menjadi reviewer — permission check server component dari JWT)
    And jika USR-MAKER-001 langsung POST /api/v1/transaksi/penempatan/PNP-001/review → HTTP 403 SOD_VIOLATION
    And toast error: "Anda tidak bisa menjadi reviewer untuk transaksi yang Anda buat sendiri."
```

---

## Story P5-M16-02 — MTM Screens Konsolidasi di `/transaksi/mtm`

**Actor**: ROLE-MAKER-TR (upload), ROLE-AKUN (upload + cron + alerts), semua role read (list + detail)
**Trigger**: User navigasi ke `/transaksi/mtm` atau klik tab "MTM" di layout transaksi.
**Goal**: Semua MTM screens tersentralisasi di `/transaksi/mtm/`; URL lama `/mtm/*` redirect 308; upload file MTM harian menggunakan dropzone + JobProgressPanel untuk batch parse (proses > 2 detik); stale price alerts real-time di `/transaksi/mtm/alerts/stale-price`.

**Source OpenAPI**: `api/openapi/app-b-mtm.yaml`, `api/openapi/app-b-bulk-upload.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/transaksi/mtm` — list MTM records dengan sort/page/filter/export
- `GET /api/v1/transaksi/mtm/{id}` — detail
- `POST /api/v1/transaksi/mtm/upload` — submit file upload batch (return 202 + jobId; Idempotency-Key wajib)
- `GET /api/v1/transaksi/mtm/upload/batch/{batch_id}` — batch detail + parse result
- `GET /api/v1/transaksi/mtm/cron` — cron job status (IBPA/BEI feed)
- `POST /api/v1/transaksi/mtm/cron/trigger` — trigger manual cron (Idempotency-Key wajib)
- `GET /api/v1/transaksi/mtm/alerts/stale-price` — daftar instrumen dengan harga stale
- `GET /api/v1/jobs/{jobId}` — status upload batch job (M13)
- `GET /api/v1/jobs/{jobId}/stream` — SSE stream progress upload batch (M13)

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `FORBIDDEN`, `NOT_FOUND`, `RATE_LIMITED`, `INTERNAL`

**Komponen reused**: `DataTable.tsx`, `JobProgressPanel.tsx`, `frontend/src/components/blips/mtm/MtmUploadDropzone.tsx`, `MtmStatusBadge.tsx`, `MtmStaleBadge.tsx`, `MtmCronTriggerButton.tsx`

### Pre-conditions
1. M6 deployed; semua MTM endpoints return 200
2. M13 deployed; `GET /api/v1/jobs/{jobId}` + SSE stream tersedia
3. `frontend/src/app/mtm/` files exist dan siap di-relocate via `git mv`
4. `MtmUploadDropzone.tsx`, `JobProgressPanel.tsx` tersedia

### Acceptance Criteria

```gherkin
Feature: MTM screens konsolidasi — relokasi namespace, 308 redirect, upload batch JobProgressPanel, stale alerts

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'transaksi.mtm.upload', 'transaksi.mtm.read'
    And M6 MTM endpoints return 200; M13 job SSE tersedia

  Scenario: M16-02-AC1 — 308 redirect dari /mtm/* ke /transaksi/mtm/*
    Given next.config.js `redirects()` sudah dikonfigurasi untuk rute MTM
    When browser mengakses /mtm
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm
    When browser mengakses /mtm/upload
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm/upload
    When browser mengakses /mtm/upload/batch/{batch_id}
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm/upload/batch/{batch_id}
    When browser mengakses /mtm/cron
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm/cron
    When browser mengakses /mtm/{id}
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm/{id}
    When browser mengakses /mtm/alerts/stale-price
    Then HTTP 308 Permanent Redirect ke /transaksi/mtm/alerts/stale-price
    And tidak ada 404 dari semua redirect di atas

  Scenario: M16-02-AC2 — Upload /transaksi/mtm/upload: dropzone + JobProgressPanel UX §3
    When USR-AKUN-001 navigasi ke /transaksi/mtm/upload
    Then halaman menampilkan MtmUploadDropzone dengan:
      | format diterima | CSV, XLSX (IBPA format atau BEI format)                                              |
      | ukuran max      | 50MB per file; validasi ukuran di frontend sebelum POST                               |
      | drag-and-drop   | area dropzone highlight saat file di-drag; label "Taruh file di sini atau klik untuk browse" |
      | validasi client | format file, ukuran — toast error instant tanpa POST jika tidak valid               |
    When USR-AKUN-001 upload file valid mtm-ibpa-2026-06-25.csv
    Then tombol "Upload" disable + spinner inline
    And POST /api/v1/transaksi/mtm/upload → 202 Accepted { jobId: "JOB-MTM-UPLOAD-001", statusUrl: ..., streamUrl: ... }
    And Idempotency-Key UUID v4 disertakan di header POST
    And halaman render <JobProgressPanel> yang subscribe ke SSE GET /api/v1/jobs/JOB-MTM-UPLOAD-001/stream:
      | progress bar   | 0→100%; current step: "Parsing baris 1.234 dari 5.678"                             |
      | ETA display    | "Estimasi selesai: {timestamp} ({N} detik lagi)"                                    |
      | cancel button  | tombol "Batalkan Upload" visible (jika job canCancel=true)                          |
      | background btn | tombol "Lanjutkan di Background" — tutup panel; badge notif di top bar               |
    When SSE event: completed
    Then JobProgressPanel update ke status selesai: "Upload berhasil. 5.678 record MTM diproses, 12 error ditemukan."
    And toast success: "Batch upload MTM JOB-MTM-UPLOAD-001 selesai. 5.678 record diproses." + link "Lihat hasil batch →" ke /transaksi/mtm/upload/batch/JOB-MTM-UPLOAD-001
    When SSE event: failed
    Then toast error persistent: "Upload batch MTM gagal: {error.message}. Trace: {traceId}." + tombol "Coba Upload Ulang"

  Scenario: M16-02-AC3 — List /transaksi/mtm: DataTable UX §1 + filter instrumen stale
    When USR-AKUN-001 navigasi ke /transaksi/mtm
    Then DataTable menampilkan daftar MTM records:
      | data source   | GET /api/v1/transaksi/mtm?cursor=...&limit=50&sort=tanggal_mtm:desc |
      | kolom minimal | kode_instrumen, jenis_instrumen, tanggal_mtm, harga_pasar, mtm_idr, sumber_harga, status |
      | sort          | semua kolom sortable; default sort=tanggal_mtm:desc                  |
      | filter        | filter[tanggal_mtm] (date range), filter[jenis_instrumen], filter[sumber_harga], filter[status] |
      | filter stale  | filter[is_stale]=true — shortcut filter: tombol "Harga Stale" di atas table              |
      | pagination    | cursor-based; default limit 50; max 200                              |
      | export        | CSV + XLSX; async jika > 10k row; audit TRANSAKSI_MTM.EXPORT         |
    And link "Lihat Stale Price Alerts" di header list → /transaksi/mtm/alerts/stale-price

  Scenario: M16-02-AC4 — Role gate: hanya ROLE-AKUN + ROLE-MAKER-TR yang bisa upload; tab MTM visible sesuai permission
    Given ROLE-RISK USR-RISK-001 ter-autentikasi tanpa permission 'transaksi.mtm.upload'
    When USR-RISK-001 navigasi ke /transaksi/mtm/upload
    Then HTTP 403 redirect ke /transaksi/mtm (list read-only jika punya 'transaksi.mtm.read')
    And tombol "Upload File" ABSENT dari DOM untuk USR-RISK-001
    And jika USR-RISK-001 tidak punya 'transaksi.mtm.read' → redirect ke /dashboard/risk
    Given ROLE-AUDIT USR-AUDIT-001 ter-autentikasi (read-only)
    When USR-AUDIT-001 navigasi ke /transaksi/mtm
    Then DataTable tersedia read-only; tidak ada tombol "Upload", "Trigger Cron", atau aksi mutasi yang di-render
    And semua tombol mutasi ABSENT dari DOM (bukan disabled — absent)
```

---

## Story P5-M16-03 — Renewal + Penjualan: Audit UX §1/§2 dan Gap Fix

**Actor**: ROLE-MAKER-TR (create), ROLE-APPR-TR (review/approve), ROLE-RISK (read-only + BM alert)
**Trigger**: User navigasi ke `/transaksi/renewal` atau `/transaksi/penjualan`.
**Goal**: Screen renewal dan penjualan yang sudah ada di namespace benar diaudit terhadap UX §1 (DataTable sort/page/filter/export) dan UX §2 (form notification); gap ditemukan dan diperbaiki dalam sprint M16; widget BM-alerts di `/transaksi/penjualan/bm-alerts` berfungsi benar.

**Source OpenAPI**: `api/openapi/app-b-renewal-deposito.yaml`, `api/openapi/app-b-penjualan.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/transaksi/renewal` — list renewal
- `POST /api/v1/transaksi/renewal` — create renewal (Idempotency-Key wajib)
- `GET /api/v1/transaksi/renewal/{id}` — detail
- `GET /api/v1/transaksi/renewal/{id}/preview` — preview cashflow
- `POST /api/v1/transaksi/renewal/{id}/submit|review|approve|reject` — workflow
- `GET /api/v1/transaksi/penjualan` — list penjualan
- `POST /api/v1/transaksi/penjualan` — create penjualan (Idempotency-Key wajib)
- `GET /api/v1/transaksi/penjualan/{id}` — detail
- `POST /api/v1/transaksi/penjualan/{id}/submit|review|approve|reject` — workflow
- `GET /api/v1/transaksi/penjualan/bm-alerts` — BM model alerts (instrumen yang BM-nya berpotensi berubah)

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `SOD_VIOLATION`, `WORKFLOW_INVALID_TRANSITION`, `FORBIDDEN`, `BM_ASSESSMENT_REQUIRED`, `PERIODE_CLOSED`

**Komponen reused**: `DataTable.tsx`, `MakerReviewerApproverPanel.tsx`, `WorkflowStatusBadge.tsx`, `SodBlockBanner.tsx`, `ReturnedBanner.tsx`

### Pre-conditions
1. M7 (renewal) + M8 (penjualan) deployed; endpoints return 200
2. `frontend/src/app/transaksi/renewal/` + `frontend/src/app/transaksi/penjualan/` sudah exist
3. Audit dilakukan terhadap implementasi yang ada; gap didokumentasikan dalam story + diperbaiki

### Acceptance Criteria

```gherkin
Feature: Renewal + Penjualan — audit UX §1 DataTable + §2 form notif + BM-alerts widget

  Background:
    Given ROLE-MAKER-TR USR-MAKER-001 ter-autentikasi dengan permission 'renewal.create', 'penjualan.create', 'transaksi.*.read'
    And M7 renewal endpoints + M8 penjualan endpoints return 200

  Scenario: M16-03-AC1 — List /transaksi/renewal: DataTable UX §1 lengkap (sort + page + filter + export)
    When USR-MAKER-001 navigasi ke /transaksi/renewal
    Then DataTable menampilkan daftar renewal dengan UX §1 lengkap:
      | data source   | GET /api/v1/transaksi/renewal?cursor=...&limit=50&sort=tanggal_renewal:desc            |
      | kolom minimal | kode_renewal, instrumen_asal, nominal_idr, suku_bunga_baru, tanggal_renewal, workflow_status |
      | sort          | multi-column sortable; header klik; icon indicator; default sort=tanggal_renewal:desc   |
      | filter        | filter[workflow_status], filter[jenis_instrumen], filter[tanggal_renewal] (date range)  |
      | filter chip   | setiap filter aktif tampil sebagai chip dengan tombol X untuk remove per-chip          |
      | URL state     | filter + sort di URL searchParams; share/bookmark berfungsi                            |
      | pagination    | cursor-based; "Halaman X dari ~Y"; limit selector 25/50/100; default 50                |
      | export        | CSV + XLSX; filter aktif ikut ke export; async jika > 10k row; audit RENEWAL.EXPORT    |
    And jika implementasi existing tidak memiliki salah satu dari (sort/page/filter/export):
      | gap ditemukan | dicatat sebagai BROKEN dalam implementasi; M16 WAJIB fix sebelum selesai               |

  Scenario: M16-03-AC2 — Form /transaksi/renewal/new: form notif UX §2 + preview cashflow
    When USR-MAKER-001 mengisi form renewal dan klik "Hitung Preview"
    Then GET /api/v1/transaksi/renewal/{id}/preview di-call; preview cashflow tampil tanpa toast (read-only)
    When USR-MAKER-001 klik "Simpan Draft"
    Then tombol disable + spinner inline; Idempotency-Key UUID v4 di-inject otomatis
    When POST berhasil (201)
    Then toast success 4 detik: "Renewal {kode_renewal} berhasil dibuat. Menunggu submit ke reviewer." + link "Lihat detail →"
    When POST gagal 422 WORKFLOW_INVALID_TRANSITION (mis. instrumen sudah jatuh tempo)
    Then toast error persistent: "Renewal tidak dapat dibuat: instrumen {kode_instrumen} sudah melewati tanggal jatuh tempo." + error code + traceId
    When POST gagal 423 PERIODE_CLOSED
    Then toast error persistent: "Periode buku {periode} sudah closed. Tidak bisa membuat transaksi baru."

  Scenario: M16-03-AC3 — List + form /transaksi/penjualan: UX §1/§2; BM-alerts widget
    When USR-MAKER-001 navigasi ke /transaksi/penjualan
    Then DataTable penjualan memenuhi UX §1 (sort, page, filter, export) — sama seperti M16-03-AC1
    And filter tambahan: filter[bm_alert]=true — shortcut menampilkan instrumen dengan BM alert
    When USR-MAKER-001 mengisi form penjualan baru untuk instrumen dengan BM alert aktif
    Then ReturnedBanner atau inline warning card tampil: "Perhatian: penjualan instrumen ini mungkin berdampak pada Business Model portfolio. Konsultasikan dengan Risk Officer."
    And warning TIDAK memblok submit — hanya informational (actual BM assessment di APP-A territory)
    When USR-MAKER-001 navigasi ke /transaksi/penjualan/bm-alerts
    Then DataTable BM-alerts menampilkan:
      | data source   | GET /api/v1/transaksi/penjualan/bm-alerts?cursor=...&limit=50&sort=created_at:desc   |
      | kolom minimal | kode_instrumen, portofolio, bm_status, trigger_event, tanggal_trigger, recommendation |
      | sort          | sortable; default tanggal_trigger:desc                                                 |
      | filter        | filter[bm_status], filter[portofolio_id]                                               |
      | export        | CSV + XLSX; audit PENJUALAN_BM_ALERT.EXPORT                                            |
    And ROLE-RISK: link "Review BM Assessment" per baris → /master/portofolio/{id}/bm-assessment (APP-A — M1 territory)

  Scenario: M16-03-AC4 — Renewal/Penjualan: SoD enforcement di workflow UI + approval toast
    Given renewal RNW-001 status=SUBMITTED, maker=USR-MAKER-001
    When ROLE-APPR-TR USR-APPR-001 (berbeda) navigasi ke /transaksi/renewal/RNW-001
    Then tombol "Review & Tandatangani" visible
    When USR-APPR-001 klik "Approve" di MakerReviewerApproverPanel + isi komentar "Suku bunga sesuai mandate"
    Then POST /api/v1/transaksi/renewal/RNW-001/approve → 200
    And toast success: "Renewal RNW-001 berhasil di-approve. Jurnal otomatis akan di-buat."
    When USR-MAKER-001 mencoba akses tombol "Approve" di renewal miliknya (dari URL langsung)
    Then button "Review" dan "Approve" ABSENT dari DOM (server component check: maker_id = current_user.id → tidak render)
    And direct API POST /api/v1/transaksi/renewal/RNW-001/approve oleh USR-MAKER-001 → HTTP 403 SOD_VIOLATION
```

---

## Story P5-M16-04 — Jatuh Tempo + Akrual: Monitoring DataTable + Batch Trigger JobProgressPanel

**Actor**: ROLE-MAKER-TR (read + trigger akrual), ROLE-AKUN (upload + trigger), ROLE-AKUN-CTL (approve batch akrual)
**Trigger**: User navigasi ke `/transaksi/jatuh-tempo` (monitoring) atau `/transaksi/akrual` (batch trigger + list).
**Goal**: Jatuh tempo screen menyediakan monitoring read-only DataTable yang memenuhi UX §1; akrual screen menyediakan tombol "Jalankan Batch Akrual Harian" dengan JobProgressPanel UX §3; ROLE-AKUN-CTL dapat approve batch akrual setelah preview.

**Source OpenAPI**: `api/openapi/app-b-jatuh-tempo-akrual.yaml`

**Endpoint yang dikonsumsi**:
- `GET /api/v1/transaksi/jatuh-tempo` — list instrumen dengan jatuh tempo mendatang atau sudah jatuh tempo
- `GET /api/v1/transaksi/akrual` — list akrual records
- `GET /api/v1/transaksi/akrual/{id}` — detail akrual (per instrumen per periode)
- `POST /api/v1/transaksi/akrual/batch` — trigger batch akrual harian (return 202 + jobId; Idempotency-Key wajib)
- `GET /api/v1/transaksi/akrual/dashboard` — summary KPI akrual (total bunga akrual, total terhutang, per jenis)
- `GET /api/v1/jobs/{jobId}` + SSE stream — status batch akrual job (M13)

**Error codes dikonsumsi**: `VALIDATION_FAILED`, `FORBIDDEN`, `PERIODE_CLOSED`, `NOT_FOUND`, `INTERNAL`

**Komponen reused**: `DataTable.tsx`, `JobProgressPanel.tsx`, `WorkflowStatusBadge.tsx`, `MFAStepUpModal.tsx` (untuk ROLE-AKUN-CTL approve jika diperlukan step-up)

### Pre-conditions
1. M9 (jatuh tempo + akrual) deployed; endpoints return 200
2. M13 job SSE tersedia
3. `frontend/src/app/transaksi/jatuh-tempo/page.tsx` + `akrual/` files exist
4. `JobProgressPanel.tsx` tersedia

### Acceptance Criteria

```gherkin
Feature: Jatuh Tempo monitoring + Akrual batch trigger dengan JobProgressPanel

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'transaksi.akrual.create', 'transaksi.jatuh-tempo.read'
    And M9 jatuh tempo + akrual endpoints return 200; M13 job SSE tersedia

  Scenario: M16-04-AC1 — /transaksi/jatuh-tempo: DataTable monitoring read-only UX §1
    When USR-AKUN-001 navigasi ke /transaksi/jatuh-tempo
    Then DataTable menampilkan daftar instrumen jatuh tempo dengan UX §1:
      | data source   | GET /api/v1/transaksi/jatuh-tempo?cursor=...&limit=50&sort=tanggal_jatuh_tempo:asc |
      | kolom minimal | kode_instrumen, jenis_instrumen, counterparty, nominal_idr, tanggal_jatuh_tempo, hari_tersisa, status_jatuh_tempo |
      | sort          | semua kolom sortable; default sort=tanggal_jatuh_tempo:asc (upcoming first)         |
      | filter        | filter[hari_tersisa]=lte:30 (shortcut "Dalam 30 hari"), filter[jenis_instrumen], filter[status_jatuh_tempo] |
      | shortcut filter| tombol quick-filter: "Dalam 7 hari", "Dalam 30 hari", "Sudah Jatuh Tempo" (past due) |
      | status badge  | UPCOMING (kuning), PAST_DUE (merah), SETTLED (hijau) — per StageBadge/WorkflowStatusBadge pattern |
      | pagination    | cursor-based; default limit 50                                                      |
      | export        | CSV + XLSX; filter aktif ikut ke export; audit JATUH_TEMPO.EXPORT                  |
    And tidak ada tombol create/edit/delete — halaman ini read-only untuk semua role
    And ROLE-MAKER-TR + ROLE-APPR-TR: shortcut CTA "Buat Renewal" per baris UPCOMING → /transaksi/renewal/new?instrumen_id={id}
    And ROLE-AUDIT: semua tombol aksi ABSENT dari DOM; export tetap tersedia

  Scenario: M16-04-AC2 — /transaksi/akrual: batch trigger + JobProgressPanel UX §3
    When USR-AKUN-001 navigasi ke /transaksi/akrual
    Then halaman menampilkan:
      | dashboard KPI    | GET /api/v1/transaksi/akrual/dashboard → card: total bunga akrual hari ini (IDR), jumlah instrumen, periode aktif |
      | DataTable list   | GET /api/v1/transaksi/akrual?cursor=...&limit=50&sort=tanggal_akrual:desc — UX §1 lengkap |
      | batch trigger    | tombol "Jalankan Batch Akrual Harian" (ROLE-AKUN permission 'transaksi.akrual.create') |
    When USR-AKUN-001 klik "Jalankan Batch Akrual Harian"
    Then muncul konfirmasi dialog: "Jalankan batch akrual harian untuk periode {PRD-2026-06}? Semua instrumen aktif akan diproses."
    When USR-AKUN-001 konfirmasi
    Then tombol disable + spinner
    And POST /api/v1/transaksi/akrual/batch → 202 Accepted { jobId: "JOB-AKRUAL-2026-06-25", statusUrl, streamUrl }
    And Idempotency-Key UUID v4 di-inject otomatis
    And <JobProgressPanel> render di atas DataTable:
      | SSE subscribe  | GET /api/v1/jobs/JOB-AKRUAL-2026-06-25/stream                                    |
      | progress bar   | 0→100%; current step: "Menghitung akrual instrumen 234 dari 1.100"                |
      | ETA display    | "Estimasi selesai: {timestamp}"                                                    |
      | cancel button  | visible jika job canCancel=true                                                    |
    When SSE completed
    Then toast success: "Batch akrual harian JOB-AKRUAL-2026-06-25 selesai. 1.100 instrumen diproses. Total akrual: Rp {nilai}."
    And DataTable refresh otomatis setelah job selesai (re-fetch dari endpoint list)
    When SSE failed
    Then toast error persistent: "Batch akrual gagal: {error.message}. Trace: {traceId}." + tombol "Coba Lagi"

  Scenario: M16-04-AC3 — Role gate: ROLE-AKUN-CTL dapat approve batch; ROLE-MAKER-TR hanya trigger
    Given batch akrual JOB-AKRUAL-2026-06-25 status=COMPLETED menunggu approval
    When ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi (mfa_verified=true) navigasi ke /transaksi/akrual
    Then tombol "Approve Batch Akrual" visible di header halaman (permission 'transaksi.akrual.approve')
    And MFA step-up tidak diperlukan untuk akrual approve (hanya hard-close yang trigger step-up per DEC-027)
    When USR-CTL-001 klik "Approve Batch Akrual" + konfirmasi dialog
    Then POST workflow approve → 200; toast success: "Batch akrual JOB-AKRUAL-2026-06-25 berhasil di-approve. Jurnal akrual akan di-post."
    When ROLE-MAKER-TR USR-MAKER-001 navigasi ke /transaksi/akrual
    Then tombol "Approve Batch Akrual" ABSENT dari DOM (permission 'transaksi.akrual.approve' tidak ada)
    And tombol "Jalankan Batch Akrual Harian" VISIBLE untuk USR-MAKER-001 (permission 'transaksi.akrual.create')

  Scenario: M16-04-AC4 — /transaksi/akrual/dashboard: KPI cards + DataTable list UX §1
    When USR-AKUN-001 navigasi ke /transaksi/akrual
    Then dashboard KPI section menampilkan:
      | data source   | GET /api/v1/transaksi/akrual/dashboard                                                |
      | KPI card 1    | "Total Akrual Hari Ini: Rp {total_akrual_idr}" — sum seluruh instrumen aktif         |
      | KPI card 2    | "Instrumen Diproses: {count}" — jumlah instrumen dalam batch terakhir                |
      | KPI card 3    | "Status Batch Terakhir: {status}" — COMPLETED/RUNNING/FAILED + timestamp             |
      | refresh       | tombol "Refresh" manual; polling 5 menit otomatis                                     |
    And DataTable akrual list memenuhi UX §1:
      | kolom minimal | kode_instrumen, jenis_instrumen, tanggal_akrual, bunga_akrual_idr, eir_persen, stage, periode |
      | sort          | semua sortable; default tanggal_akrual:desc                                           |
      | filter        | filter[periode_id], filter[jenis_instrumen], filter[stage], filter[tanggal_akrual] (date range) |
      | export        | CSV + XLSX; async jika > 10k row; audit AKRUAL.EXPORT                                |
```

---

## Story P5-M16-05 — Shared `/transaksi` Layout: Tab Nav + Breadcrumb + Role-Gated Visibility

**Actor**: Semua authenticated role (tab yang visible berbeda per permission)
**Trigger**: User mengakses route apa pun di dalam `/transaksi/*` — layout diterapkan otomatis via Next.js App Router.
**Goal**: Satu layout bersama dengan 6 tab (penempatan | MTM | renewal | penjualan | jatuh tempo | akrual) dan breadcrumb; tab yang user tidak punya permission `transaksi.{sub}.read` untuk di-read ABSENT dari DOM (bukan hanya disabled); route aktif di-highlight; tombol "Baru" kontekstual per sub-route yang support create.

**Komponen baru**: `frontend/src/components/blips/transaksi/TransaksiTabNav.tsx`

**Permission logic per tab**:

| Tab | Permission required | Default role yang punya |
|---|---|---|
| penempatan | `penempatan.read` | MAKER-TR, APPR-TR, RISK, AKUN-CTL, AUDIT, CFO |
| MTM | `transaksi.mtm.read` | MAKER-TR, AKUN, RISK, AUDIT, CFO |
| renewal | `renewal.read` | MAKER-TR, APPR-TR, RISK, AKUN-CTL, AUDIT |
| penjualan | `penjualan.read` | MAKER-TR, APPR-TR, RISK, AKUN-CTL, AUDIT |
| jatuh tempo | `transaksi.jatuh-tempo.read` | semua role kecuali IT-ADMIN |
| akrual | `transaksi.akrual.read` | AKUN, AKUN-CTL, RISK, AUDIT |

**Tombol "Baru" kontekstual** (visible hanya untuk role yang punya create permission):

| Sub-route aktif | CTA button | Route | Permission |
|---|---|---|---|
| penempatan | "+ Penempatan Baru" | `/transaksi/penempatan/new` | `penempatan.create` |
| renewal | "+ Renewal Baru" | `/transaksi/renewal/new` | `renewal.create` |
| penjualan | "+ Penjualan Baru" | `/transaksi/penjualan/new` | `penjualan.create` |
| MTM | "+ Upload MTM" | `/transaksi/mtm/upload` | `transaksi.mtm.upload` |
| jatuh tempo | — (read-only, tidak ada CTA) | — | — |
| akrual | "Jalankan Batch Akrual" | handled di page (trigger job) | `transaksi.akrual.create` |

### Pre-conditions
1. Semua sub-route pages exist (hasil M16-01 s/d M16-04)
2. JWT `permissions` array tersedia di server component via Keycloak token
3. Next.js App Router layout inheritance berfungsi

### Acceptance Criteria

```gherkin
Feature: Shared /transaksi layout — tab nav 6 sub-routes + breadcrumb + role-gated visibility + CTA

  Background:
    Given server component `frontend/src/app/transaksi/layout.tsx` deployed
    And TransaksiTabNav.tsx component tersedia dengan permission-aware rendering

  Scenario: M16-05-AC1 — Tab nav: absent-from-DOM jika user tidak punya read permission
    Given ROLE-AKUN USR-AKUN-001 dengan permission: transaksi.mtm.read, transaksi.akrual.read (tanpa penempatan.read, renewal.read, penjualan.read)
    When USR-AKUN-001 navigasi ke /transaksi/akrual
    Then tab "MTM" VISIBLE di tab nav (permission transaksi.mtm.read ada)
    And tab "Akrual" VISIBLE di tab nav (permission transaksi.akrual.read ada)
    And tab "Penempatan" ABSENT dari DOM (bukan disabled — tidak di-render karena permission tidak ada)
    And tab "Renewal" ABSENT dari DOM
    And tab "Penjualan" ABSENT dari DOM
    And tab "Jatuh Tempo" VISIBLE (semua role kecuali IT-ADMIN)
    And server component check: permission di-baca dari JWT claims di server (tidak pakai client-side JS kondisi untuk hide/show)
    And tidak ada HTML comment atau hidden DOM node yang mengandung nama tab yang absent

  Scenario: M16-05-AC2 — Tab nav: active route highlight; breadcrumb benar per sub-route
    Given ROLE-MAKER-TR USR-MAKER-001 dengan semua transaksi read permission
    When USR-MAKER-001 navigasi ke /transaksi/renewal/new
    Then tab "Renewal" di tab nav memiliki aria-current="page" dan visual active state (border-bottom accent color)
    And tab lain (Penempatan, MTM, Penjualan, Jatuh Tempo, Akrual) tidak memiliki aria-current="page"
    And breadcrumb menampilkan: "Beranda / Transaksi / Renewal / Renewal Baru"
    And setiap item breadcrumb kecuali terakhir: link yang bisa di-klik
    And "Renewal Baru" (item terakhir): bukan link; aria-current="page"
    When USR-MAKER-001 navigasi ke /transaksi/penempatan
    Then tab "Penempatan" active; breadcrumb: "Beranda / Transaksi / Penempatan"

  Scenario: M16-05-AC3 — CTA button "Baru": visible sesuai sub-route dan permission; absent jika tidak punya create
    Given ROLE-MAKER-TR USR-MAKER-001 dengan permission 'penempatan.create', 'renewal.create', 'penjualan.create', 'transaksi.mtm.upload'
    When USR-MAKER-001 berada di /transaksi/penempatan
    Then tombol "+ Penempatan Baru" VISIBLE di header layout; klik → navigasi ke /transaksi/penempatan/new
    When USR-MAKER-001 berada di /transaksi/jatuh-tempo
    Then tidak ada CTA button (jatuh tempo read-only) — header layout tidak render CTA untuk sub-route ini
    Given ROLE-APPR-TR USR-APPR-001 tanpa permission 'penempatan.create'
    When USR-APPR-001 berada di /transaksi/penempatan
    Then tombol "+ Penempatan Baru" ABSENT dari DOM (permission 'penempatan.create' tidak ada di JWT)
    And server component: permission check dari JWT → conditional render, bukan CSS display:none

  Scenario: M16-05-AC4 — Layout aksesibilitas: keyboard nav tab, aria-labels, skip-to-content
    When pengguna screen reader mengakses /transaksi/penempatan
    Then layout memiliki "skip to main content" link sebagai elemen pertama di DOM (keyboard users)
    And tab nav: `<nav aria-label="Navigasi Transaksi">` wrapping `<ul role="tablist">`
    And setiap tab yang visible: `role="tab"` + `aria-selected="true|false"` + `aria-controls="{panel-id}"`
    And panel konten: `role="tabpanel"` + `aria-labelledby="{tab-id}"`
    And breadcrumb: `<nav aria-label="Breadcrumb">` dengan `<ol>`; item terakhir `aria-current="page"`
    And keyboard: Tab key navigasi ke tab nav; Arrow keys switch antar tab; Enter/Space aktivasi tab
    And tombol CTA "Baru" per sub-route: aria-label="Tambah Penempatan Baru" (bukan hanya "+ Penempatan Baru" yang ambigu)
```

---

## Ringkasan P5-M16 Story Set

| Story | Judul | Actor Utama | OpenAPI Ref | AC Count | Gate |
|---|---|---|---|---|---|
| P5-M16-01 | Penempatan konsolidasi `/transaksi/penempatan` + 308 redirect | ROLE-MAKER-TR, ROLE-APPR-TR | `app-b-penempatan-deposito.yaml` | 4 | security BLOCKING (SoD, absent-from-DOM) |
| P5-M16-02 | MTM konsolidasi `/transaksi/mtm` + 308 redirect + JobProgressPanel upload | ROLE-MAKER-TR, ROLE-AKUN | `app-b-mtm.yaml`, `app-b-bulk-upload.yaml` | 4 | security BLOCKING (role gate upload) |
| P5-M16-03 | Renewal + Penjualan audit UX §1/§2 + BM-alerts widget | ROLE-MAKER-TR, ROLE-APPR-TR | `app-b-renewal-deposito.yaml`, `app-b-penjualan.yaml` | 4 | security BLOCKING (SoD) |
| P5-M16-04 | Jatuh Tempo monitoring DataTable + Akrual batch trigger JobProgressPanel | ROLE-AKUN, ROLE-AKUN-CTL | `app-b-jatuh-tempo-akrual.yaml` | 4 | security BLOCKING (MFA CTL gate) |
| P5-M16-05 | Shared `/transaksi` layout — tab nav + breadcrumb + role-gated visibility + CTA | Semua role | — (layout no direct endpoint) | 4 | security BLOCKING (absent-from-DOM server component) |
| **Total** | | | | **20** | |

---

## Error Codes Dikonsumsi (sudah ada di `api/openapi/_common.yaml`)

| Code | HTTP | Trigger dalam M16 |
|---|---|---|
| `VALIDATION_FAILED` | 400 | Form submit dengan field wajib kosong atau format salah |
| `UNAUTHORIZED` | 401 | JWT expired saat user di halaman |
| `FORBIDDEN` | 403 | Permission tidak ada; role tidak sesuai sub-route |
| `SOD_VIOLATION` | 403 | Maker mencoba review/approve transaksi sendiri |
| `NOT_FOUND` | 404 | Detail page dengan ID tidak valid |
| `CONFLICT` | 409 | Optimistic lock mismatch pada edit |
| `IDEMPOTENCY_REPLAY` | 200 | Retry submit yang sudah berhasil sebelumnya |
| `IDEMPOTENCY_MISMATCH` | 422 | Same key, payload berbeda |
| `WORKFLOW_INVALID_TRANSITION` | 422 | Submit/approve pada state yang salah |
| `BM_ASSESSMENT_REQUIRED` | 422 | Penjualan tanpa BM assessment pada portofolio |
| `PERIODE_CLOSED` | 423 | Mutasi pada periode yang sudah closed |

Tidak ada error code baru yang diusulkan dari M16 — semua sudah tersedia.

---

## Audit Events — Transaksi (wajib per DEC-018)

| Event | Trigger | In-transaction |
|---|---|---|
| `PENEMPATAN.EXPORT` | Export DataTable penempatan (CSV/XLSX) | Ya |
| `TRANSAKSI_MTM.EXPORT` | Export DataTable MTM (CSV/XLSX) | Ya |
| `RENEWAL.EXPORT` | Export DataTable renewal (CSV/XLSX) | Ya |
| `PENJUALAN.EXPORT` | Export DataTable penjualan (CSV/XLSX) | Ya |
| `PENJUALAN_BM_ALERT.EXPORT` | Export BM-alerts DataTable | Ya |
| `JATUH_TEMPO.EXPORT` | Export DataTable jatuh tempo (CSV/XLSX) | Ya |
| `AKRUAL.EXPORT` | Export DataTable akrual (CSV/XLSX) | Ya |

Audit event untuk workflow actions (submit/review/approve/reject) sudah ditangani di backend M1/M7/M8/M9 — tidak perlu duplikasi dari frontend.

---

## Refresh Cadence Summary

| Screen | Refresh Mode | Interval | Notes |
|---|---|---|---|
| Semua DataTable list | Manual (filter/sort/paging trigger re-fetch) | On demand | Tidak ada auto-polling untuk list; user trigger refresh |
| MTM upload JobProgressPanel | SSE push | Real-time | Fallback polling 2 detik jika SSE error |
| Akrual batch JobProgressPanel | SSE push | Real-time | Fallback polling 2 detik jika SSE error |
| Akrual dashboard KPI cards | 5-menit polling | 300.000 ms | Sama dengan pola M15 |
| Tombol "Refresh" manual | On demand | — | Setiap DataTable punya tombol refresh di action bar |

---

## Handoff Berikutnya

- `uiux-designer` → wireframe shared `/transaksi` layout (tab nav 6 slot, breadcrumb, CTA placement); mobile-first caveat (desktop-first ≥ 1280px target utama); file dropzone MTM UX; JobProgressPanel placement di upload + akrual pages; empty/loading/error states per DataTable; WCAG AA contrast check tab nav
- `frontend-engineer-nextjs` → implementasi setelah `uiux-designer` selesai:
  - `git mv` penempatan + mtm files ke namespace baru
  - `next.config.js` `redirects()` untuk 10 redirect rules (4 penempatan + 6 mtm)
  - `frontend/src/app/transaksi/layout.tsx` + `TransaksiTabNav.tsx`
  - Audit gap fix di renewal + penjualan + jatuh-tempo + akrual existing screens (UX §1/§2/§3 compliance)
  - Idempotency-Key auto-inject via shared form hook (jika belum ada utility)
- `security-engineer` → **BLOCKING**:
  - absent-from-DOM: server component JWT permission check (tidak render tab/button jika permission absent)
  - 308 redirect tidak leak data (redirect sebelum halaman render, tidak setelah partial render)
  - SoD enforcement: maker tidak bisa trigger review/approve dari UI (server component + API enforced)
  - Idempotency-Key: verifikasi wajib di semua form submit (audit via random sampling E2E test)
  - Export RBAC: export hanya respect data yang user bisa akses (filter applied at query level, bukan client-side)
- `qa-engineer` → E2E tests per AC (Playwright):
  - 308 redirect tests (semua 10 paths)
  - DataTable sort/filter/export per sub-route
  - JobProgressPanel SSE mock tests (upload MTM + akrual batch)
  - SoD E2E: maker mencoba review via API direct
  - Tab nav visible/absent per role (5 role fixtures)
  - Form notif sukses/gagal (mock API responses)

_Story set P5-M16 siap dihandoff ke `uiux-designer` dan `security-engineer` secara paralel. `frontend-engineer-nextjs` mulai setelah kedua review tersebut selesai. Tidak ada `ifrs9-compliance-reviewer` gate — M16 murni UI layer, tidak compute ECL/EIR/SPPI/BM baru._
