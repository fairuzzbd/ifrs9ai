# P4-M10 — ECL Calc Run UI: User Stories

**Story Set ID**: P4-M10
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 4)
**Status**: DRAFT — compliance review: advisory (bukan BLOCKING gate). Compliance harus verifikasi: seal button = destructive dialog + MFA step-up UI (DEC-027), scenario breakdown tampilkan ECL per skenario + bobot.
**Author**: business-analyst
**Tanggal**: 2026-06-13
**Branch target**: `feature/app-c-ecl-run-ui`

**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §5 (calc run orchestration), §6 (seal workflow), §3 (ECL formula)
**Linked BRD**: BRD §8.2 (ECL Computation), §9.3 (Seal + Immutability), RACI: ROLE-RISK (R/trigger + seal-request), ROLE-ALCO (A/seal-approve), ROLE-CFO (I + co-approver), ROLE-AUDIT (I/read-only)

**Linked Decision Log**:
- DEC-010 — ECL formula: 3-stage × 3-skenario × dual FL multiplier. Bobot default Good/Normal/Bad = 0.25/0.50/0.25. ALCO dapat override.
- DEC-016 — NUMERIC: IDR `NUMERIC(20,4)`, PD/LGD/EIR `NUMERIC(10,8)`.
- DEC-017 — Seal workflow: 4-eyes (RISK request → ALCO approve). SoD `created_by ≠ seal_approver_id`.
- DEC-018 — `ecl.*` no hard delete. Audit trail append-only 10+10 tahun.
- DEC-021 — Idempotency-Key wajib setiap mutating endpoint.
- DEC-026/DEC-027 — MFA mandatory ROLE-ALCO; step-up MFA untuk seal approve.
- UX §1 — DataTable: sort + cursor pagination + filter + export wajib.
- UX §2 — Form notification: sukses/gagal eksplisit.
- UX §3 — Long-running: JobProgressPanel + SSE + polling fallback.

**Depends on (harus MERGED sebelum frontend coding dimulai)**:
- P4-M7 merged (PR #72) — endpoint: `POST /ecl/calc-runs/single`, `POST /ecl/calc-runs/bulk`, `GET /ecl/calc-runs/{id}/results`, `GET /ecl/calc-runs/{id}/portfolio/{portofolio_id}/summary`, `GET /ecl/roll-forward`, `POST /ecl/calc-runs/{id}/recompute`
- P4-M8 merged (PR #76) — endpoint: `POST /ecl/calc-runs`, `POST /ecl/calc-runs/{id}/start`, `GET /ecl/calc-runs`, `GET /ecl/calc-runs/{id}`, `POST /ecl/calc-runs/{id}/cancel`, `POST /ecl/calc-runs/{id}/seal`, `GET /api/v1/jobs/{jobId}`, `GET /api/v1/jobs/{jobId}/stream` (SSE)
- Phase 3 frontend merged — reuse `<DataTable>`, `<JobProgressPanel>`, `<MFAStepUpModal>`, `<DestructiveActionDialog>`, `lib/notify.ts`, `useJobProgress` dari `web/components/blips/`

**Handoff berikutnya**:
- `system-analyst` — konfirmasi endpoint `GET /ecl/calc-runs/{id}/instrumen/{instrumenId}` (drill-down per instrumen) ada di OpenAPI M7; konfirmasi field `routing_path`, `warnings[]`, `scenario_breakdown[]` di response; konfirmasi `GET /ecl/roll-forward?calc_run_id=&prior_calc_run_id=` tersedia
- `ifrs9-compliance-reviewer` — advisory: pastikan scenario breakdown tampilkan ECL per skenario + bobot; seal button = destructive dialog + step-up MFA UI (DEC-027 UI enforcement)
- `qa-engineer` — UAT scripts per story + aksesibilitas (WCAG 2.1 AA)

---

## Komponen Shared (reuse dari Phase 3 + M9)

| Komponen | Lokasi | Dipakai di story |
|---|---|---|
| `<DataTable>` | `web/components/blips/DataTable.tsx` | S1, S3, S4, S5, S6 |
| `<JobProgressPanel>` | `web/components/blips/JobProgressPanel.tsx` | S2 |
| `<MFAStepUpModal>` | `web/components/blips/MFAStepUpModal.tsx` | S7 (ALCO approve) |
| `<DestructiveActionDialog>` | `web/components/blips/DestructiveActionDialog.tsx` | S2 (cancel confirm), S7 (seal request + approve destructive confirm) |
| `notify` (lib) | `web/lib/notify.ts` | Semua story |
| `useJobProgress` | `web/hooks/useJobProgress.ts` | S2 |

---

## Permissions Summary

| Permission | Actors | Stories |
|---|---|---|
| `calc_run.create` | ROLE-RISK | S1 |
| `calc_run.start` | ROLE-RISK | S2 |
| `calc_run.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO | S1, S2, S3, S5, S6 |
| `calc_run.cancel` | ROLE-RISK (maker only) | S2 |
| `ecl.result.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO | S3, S4, S5, S6 |
| `ecl.result.export` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | S3, S4, S5, S6 |
| `ecl.portfolio_aggregate.read` | ROLE-RISK, ROLE-CFO, ROLE-AUDIT | S5 |
| `calc_run.seal_request` | ROLE-RISK | S7 |
| `calc_run.seal_approve` | ROLE-ALCO, ROLE-CFO | S7 |

---

## Story APP-C-M10-001 — Calc Run Dashboard List + Create

**Actor**: ROLE-RISK
**Trigger**: Navigasi ke `/ecl/calc-runs`
**Goal**: ROLE-RISK melihat daftar semua calc run dalam DataTable (filter by periode, status; sort by created_at DESC), dan membuat calc run baru untuk periode yang belum di-seal via modal create.

**Pre-conditions**:
- User ter-autentikasi dengan permission `calc_run.read` (list) dan `calc_run.create` (create button visible).
- `mst.periode_buku` memiliki setidaknya satu periode dengan status bukan `HARD_CLOSED`.

**Post-conditions (setelah create)**:
- `ecl.calc_run` row baru dengan status `DRAFT` dibuat.
- Toast sukses dengan `calc_run_id` dan link ke detail view.
- User di-redirect ke `/ecl/calc-runs/{id}`.

**Komponen**:
- `<DataTable>` untuk list calc runs (UX §1: sort + cursor pagination + filter + export)
- Modal create: periode picker (`<Select>` — hanya periode non-`HARD_CLOSED`), evaluation date picker (`<DatePicker>`)
- `notify` (UX §2)

**Permissions**: `calc_run.read`, `calc_run.create`
**Audit Events**: `CALC_RUN.CREATED` (setelah create sukses — ditampilkan di audit trail, bukan di UI langsung)

### Acceptance Criteria — APP-C-M10-001

```gherkin
Feature: Calc run dashboard list dan create

  Background:
    Given 5 calc_run tersimpan untuk berbagai periode:
      | ID                 | Periode    | Status                  | Dibuat Oleh | created_at |
      | CR-2026-06-001     | JUNI-2026  | COMPLETED               | RISK-01     | 2026-06-12 |
      | CR-2026-06-002     | JUNI-2026  | CANCELLED               | RISK-01     | 2026-06-10 |
      | CR-2026-05-001     | MEI-2026   | SEALED                  | RISK-02     | 2026-05-31 |
      | CR-2026-07-001     | JULI-2026  | DRAFT                   | RISK-01     | 2026-06-13 |
      | CR-2026-06-003     | JUNI-2026  | COMPLETED_WITH_ERRORS   | RISK-01     | 2026-06-11 |
    And mst.periode_buku: JUNI-2026 (OPEN), MEI-2026 (SOFT_CLOSED), APRIL-2026 (HARD_CLOSED), JULI-2026 (OPEN)
    And RISK-01 memiliki permission calc_run.read + calc_run.create

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: halaman list tampil dengan DataTable UX §1
  # ---------------------------------------------------------------
  Scenario: Halaman list calc run tampil dengan 5 row — default sort created_at DESC
    When RISK-01 navigasi ke /ecl/calc-runs
    Then DataTable menampilkan 5 row dengan kolom:
      | ID, Periode, Status, Total Instrumen, Processed, Error, Dibuat Oleh, Mulai, Selesai, Aksi |
    And default sort: created_at DESC (CR-2026-07-001 paling atas)
    And status badge per row:
      | DRAFT                 | badge abu-abu   |
      | IN_PROGRESS           | badge biru spinner |
      | COMPLETED             | badge hijau     |
      | COMPLETED_WITH_ERRORS | badge amber     |
      | SEALED                | badge ungu + ikon gembok |
      | CANCELLED             | badge merah muted|
    And tombol "Buat Calc Run Baru" tampil di action bar
    And export button tersedia (CSV/XLSX)
    And filter chips aktif: Periode, Status, Dibuat Oleh

  # ---------------------------------------------------------------
  # Skenario 2 — Filter by periode dan status
  # ---------------------------------------------------------------
  Scenario: Filter calc run per periode JUNI-2026 dan status COMPLETED
    When RISK-01 set filter periode = "JUNI-2026" dan filter status = "COMPLETED"
    Then DataTable menampilkan 1 row: CR-2026-06-001
    And URL terupdate: ?filter[periode_id]=JUNI-2026&filter[status]=COMPLETED
    And filter chip "Periode: JUNI-2026" dan "Status: COMPLETED" tampil dengan "Clear all" button

  # ---------------------------------------------------------------
  # Skenario 3 — Create calc run: modal tampil, periode HARD_CLOSED di-exclude
  # ---------------------------------------------------------------
  Scenario: RISK-01 membuka modal create — periode APRIL-2026 (HARD_CLOSED) tidak muncul di dropdown
    When RISK-01 klik "Buat Calc Run Baru"
    Then modal create tampil dengan judul "Buat Calc Run Baru"
    And periode picker (dropdown) berisi: JUNI-2026, MEI-2026, JULI-2026
    And periode APRIL-2026 TIDAK ada di dropdown (status HARD_CLOSED)
    And evaluation date picker tersedia dengan default = hari ini (2026-06-13)

  # ---------------------------------------------------------------
  # Skenario 4 — Happy path: create DRAFT berhasil
  # ---------------------------------------------------------------
  Scenario: RISK-01 create calc run DRAFT untuk JULI-2026 sukses
    Given tidak ada calc_run IN_PROGRESS atau SEALED untuk "JULI-2026"
    When RISK-01 pilih periode "JULI-2026", evaluation_date "2026-07-31"
    And klik "Buat"
    Then tombol "Buat" berubah spinner (UX §2 pending state)
    And POST /api/v1/ecl/calc-runs dikirim dengan Idempotency-Key baru
    And response 201 → toast sukses: "Calc run untuk periode JULI-2026 berhasil dibuat (CR-2026-07-002). Status: DRAFT."
    And toast berisi action link "Lihat detail →"
    And user di-redirect ke /ecl/calc-runs/CR-2026-07-002
    And modal tertutup

  # ---------------------------------------------------------------
  # Skenario 5 — Gagal create: periode sudah SEALED
  # ---------------------------------------------------------------
  Scenario: Gagal create — MEI-2026 sudah memiliki SEALED calc run
    Given ecl.calc_run CR-2026-05-001 SEALED untuk MEI-2026
    When RISK-01 pilih periode "MEI-2026" dan klik "Buat"
    Then toast error persistent: "Periode MEI-2026 sudah memiliki calc run yang di-seal (CR-2026-05-001). Override memerlukan persetujuan ALCO — fitur belum tersedia."
    And modal tetap terbuka (user dapat memilih periode lain)
    And error.code = "CALC_RUN_PERIODE_ALREADY_SEALED"

  # ---------------------------------------------------------------
  # Skenario 6 — Sort header klik: toggle asc/desc
  # ---------------------------------------------------------------
  Scenario: Sort DataTable berdasarkan kolom "Periode" — asc
    When RISK-01 klik header kolom "Periode"
    Then DataTable di-sort berdasarkan periode_id:asc
    And URL terupdate: ?sort=periode_id:asc
    And ikon panah ↑ tampil di header kolom "Periode"
    When RISK-01 klik header kolom "Periode" kembali
    Then sort berubah menjadi desc (↓)

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: read-only, tidak ada tombol "Buat"
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT membuka halaman list — tidak ada tombol create
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 navigasi ke /ecl/calc-runs
    Then DataTable tampil lengkap dengan semua 5 row
    And tombol "Buat Calc Run Baru" TIDAK tampil
    And kolom "Aksi" hanya menampilkan "Lihat Detail" (bukan Start/Cancel)

  # ---------------------------------------------------------------
  # Skenario 8 — Export CSV list — audit event tercatat
  # ---------------------------------------------------------------
  Scenario: RISK-01 export CSV list dengan filter aktif
    Given filter aktif: periode = JUNI-2026 (3 row)
    When RISK-01 klik Export CSV
    Then file "ecl-calc-runs-JUNI-2026-20260613.csv" ter-download
    And CSV berisi 3 row sesuai filter aktif
    And aud.audit_log berisi event "CALC_RUN.EXPORT" dengan after_jsonb.row_count = 3

  # ---------------------------------------------------------------
  # Skenario 9 — Cursor pagination: list > 50 calc runs
  # ---------------------------------------------------------------
  Scenario: Paging DataTable — 60 calc runs tersimpan
    Given 60 calc_run tersimpan (berbagai periode)
    When RISK-01 membuka halaman (default limit 50)
    Then DataTable menampilkan 50 row
    And footer: "Page 1 of ~2" dan tombol "Next" aktif
    When RISK-01 klik "Next"
    Then 10 row berikutnya dimuat via cursor pagination
```

### Open Questions — M10-001
- **OQ-M10-001-A**: Apakah list endpoint `GET /ecl/calc-runs` sudah mendukung `filter[periode_id]` dan `filter[status]` di OpenAPI M8? Konfirmasi ke `system-analyst` — perlu dipastikan kolom `periode_id` dan `status` ada di `allowedCols` whitelist server-side.
- **OQ-M10-001-B**: Kolom "Total Instrumen" di DataTable — dari field `total_instrumen` di `ecl.calc_run`. Apakah field ini di-populate saat `start` (saat scope di-resolve) atau saat DRAFT create? Default assume: di-populate saat `start` (scope ALL_ACTIVE di-hitung). Saat DRAFT, tampilkan "-" (null).

---

## Story APP-C-M10-002 — Start Calc Run + Monitor Progress

**Actor**: ROLE-RISK
**Trigger**: Navigasi ke `/ecl/calc-runs/[id]` — halaman detail calc run berstatus `DRAFT`; klik tombol "Start"
**Goal**: ROLE-RISK memulai bulk ECL compute, memantau progress real-time via SSE (`<JobProgressPanel>`), mode background, dan menerima notifikasi saat selesai atau gagal. Cancel tersedia selama `IN_PROGRESS`.

**Pre-conditions**:
- `ecl.calc_run` exists dengan `status = 'DRAFT'`.
- Parameter ECL `APPROVED` tersedia untuk `periode_id`: `bobot_skenario`, `pd_pefindo`, `lgd_basel`, `impact_mev_pd`, `lps_coverage`, `kurs`.
- User memiliki permission `calc_run.start`.

**Post-conditions**:
- `ecl.calc_run.status = 'IN_PROGRESS'` lalu otomatis `'COMPLETED'` atau `'COMPLETED_WITH_ERRORS'` setelah worker selesai.
- `ecl.calc_run.parameter_snapshot_jsonb` di-freeze saat start.
- Toast sukses + link "Lihat hasil" setelah COMPLETED.

**Komponen**:
- Header panel: status badge, periode, evaluation date, counts
- Tombol "Start" (hanya jika `status = 'DRAFT'`) dengan `<DestructiveActionDialog>` confirm
- `<JobProgressPanel>` dengan SSE (`useJobProgress` hook) + polling fallback
- Tombol "Batalkan" di dalam `<JobProgressPanel>` → `<DestructiveActionDialog>` confirm + cancel_reason textarea
- `notify` (UX §2, UX §3)

**Permissions**: `calc_run.start`, `calc_run.cancel`
**Audit Events**: `CALC_RUN.STARTED`, `CALC_RUN.COMPLETED`, `CALC_RUN.COMPLETED_WITH_ERRORS`, `CALC_RUN.CANCELLED`

### Acceptance Criteria — APP-C-M10-002

```gherkin
Feature: Start calc run + monitor progress via SSE

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "DRAFT", created_by = RISK-01, periode_id = "JUNI-2026"
    And parameter ECL APPROVED tersedia untuk JUNI-2026
    And RISK-01 memiliki permission calc_run.start dan calc_run.cancel

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: start sukses, progress tampil via SSE
  # ---------------------------------------------------------------
  Scenario: RISK-01 start calc run — konfirmasi dialog, progress panel muncul
    When RISK-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001
    Then header panel menampilkan:
      | Status         | "DRAFT" (badge abu-abu)                              |
      | Periode        | "JUNI-2026"                                          |
      | Eval Date      | "2026-06-30"                                         |
      | Total Instrumen| "-" (belum di-resolve)                               |
    And tombol "Start Bulk Compute" tersedia
    When RISK-01 klik "Start Bulk Compute"
    Then <DestructiveActionDialog> muncul:
      | Judul       | "Mulai Bulk ECL Compute — JUNI-2026?"                          |
      | Deskripsi   | "Proses ini akan menghitung ECL untuk semua instrumen aktif di periode JUNI-2026. Pastikan semua parameter ECL sudah diapprove ALCO."    |
      | Tombol      | "Mulai" (biru) | "Batal"                                     |
    When RISK-01 klik "Mulai"
    Then POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/start dikirim
    And response 202 dengan jobId, statusUrl, streamUrl
    And status badge berubah ke "IN_PROGRESS" (badge biru + spinner)
    And tombol "Start" diganti dengan <JobProgressPanel>
    And <JobProgressPanel> subscribe via SSE ke streamUrl
    And <JobProgressPanel> menampilkan:
      | Progress bar        | 0% → update via SSE                              |
      | Current step        | "Menginisialisasi scope instrumen..."            |
      | ETA                 | "Menghitung..." (diperbarui saat worker report) |
      | Tombol "Batalkan"   | aktif                                            |
      | Tombol "Background" | aktif (user dapat lanjut kerja lain)            |

  # ---------------------------------------------------------------
  # Skenario 2 — SSE progress events diterima, real-time update
  # ---------------------------------------------------------------
  Scenario: SSE events diterima — progress bar dan step terupdate
    Given RISK-01 subscribed ke SSE stream untuk JOB-001 (1000 instrumen total)
    When worker mengirim events:
      | event: progress | data: { progress: 25, currentStep: "Menghitung instrumen 250 dari 1000" } |
      | event: progress | data: { progress: 75, currentStep: "Menghitung instrumen 750 dari 1000" } |
      | event: completed | data: { result: { calcRunId: "CR-JUNI-2026-001", totalInstrumen: 995, totalECLWeighted: "1234567890.0000", skippedFvtpl: 5 } } |
    Then progress bar terupdate sesuai setiap event
    And current step text terupdate
    And setelah event "completed":
      calc_run status = "COMPLETED"
      <JobProgressPanel> menampilkan "Selesai! 995 instrumen dihitung, 5 FVTPL di-skip."
      toast sukses: "ECL Calc Run JUNI-2026 selesai. 995 instrumen dihitung. Siap untuk di-seal."
      action link di toast: "Lihat hasil →"
      tombol "Request Seal" muncul di header panel
      <JobProgressPanel> berubah ke ringkasan completion (bukan spinner)

  # ---------------------------------------------------------------
  # Skenario 3 — SSE error: fallback polling setiap 2 detik
  # ---------------------------------------------------------------
  Scenario: SSE stream error — frontend fallback ke polling
    Given SSE stream CR-JUNI-2026-001 gagal (koneksi timeout setelah 5 detik)
    When useJobProgress hook mendeteksi EventSource error
    Then hook otomatis beralih ke polling GET /api/v1/jobs/JOB-001 setiap 2 detik
    And <JobProgressPanel> tetap menampilkan progress terbaru dari polling
    And indikator kecil "Polling mode" tampil (opsional, bukan blocking UX)

  # ---------------------------------------------------------------
  # Skenario 4 — Background mode: user lanjut kerja, badge notif nyala
  # ---------------------------------------------------------------
  Scenario: User klik "Background" — pindah ke halaman lain, notif badge muncul saat selesai
    Given RISK-01 ada di halaman detail CR-JUNI-2026-001, progress 40%
    When RISK-01 klik "Background"
    Then <JobProgressPanel> ditutup (panel collapse)
    And global notification badge di top bar menampilkan "1 job running"
    And RISK-01 dapat navigasi ke halaman lain (mis. /master/instrumen)
    When job selesai (COMPLETED)
    Then global notification badge berubah: "1 job selesai"
    And toast sukses muncul walau RISK-01 tidak di halaman calc run: "ECL Calc Run JUNI-2026 selesai. Klik untuk lihat hasil."
    And klik badge/toast → navigasi ke /ecl/calc-runs/CR-JUNI-2026-001

  # ---------------------------------------------------------------
  # Skenario 5 — COMPLETED_WITH_ERRORS: toast warning + link error tab
  # ---------------------------------------------------------------
  Scenario: Selesai dengan error pada 3 instrumen
    Given 3 instrumen gagal (EAD_MISSING_OUTSTANDING pada OBL-ERR-001..003)
    When worker selesai
    Then calc_run status = "COMPLETED_WITH_ERRORS"
    And toast warning: "Calc run selesai dengan 3 error. Perbaiki data instrumen sebelum seal."
    And action link: "Lihat error detail →" mengarah ke /ecl/calc-runs/CR-JUNI-2026-001?tab=errors
    And tombol "Request Seal" TIDAK muncul (status bukan COMPLETED)
    And tab "Error" di halaman detail menampilkan daftar instrumen gagal

  # ---------------------------------------------------------------
  # Skenario 6 — Cancel IN_PROGRESS: confirm dialog + cancel_reason
  # ---------------------------------------------------------------
  Scenario: RISK-01 cancel calc run saat IN_PROGRESS
    Given CR-JUNI-2026-001 status = "IN_PROGRESS", 400 instrumen sudah diproses
    When RISK-01 klik "Batalkan" di <JobProgressPanel>
    Then <DestructiveActionDialog> muncul:
      | Judul      | "Batalkan Calc Run?"                                             |
      | Deskripsi  | "Instrumen yang sudah selesai dihitung akan tetap tersimpan sebagai partial result. Partial result tidak dapat digunakan untuk pelaporan resmi." |
      | Input      | Textarea "Alasan pembatalan (wajib, minimal 30 karakter)"        |
      | Tombol     | "Batalkan Proses" (merah) | "Kembali"                           |
    When RISK-01 isi alasan "Data master PD belum final, perlu diperbarui ALCO terlebih dahulu."
    And klik "Batalkan Proses"
    Then POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/cancel dikirim
    And status badge berubah ke "CANCELLED" (merah muted)
    And toast sukses: "Calc run CR-JUNI-2026-001 berhasil dibatalkan. 400 instrumen partial tetap tersimpan."
    And tombol "Buat Calc Run Baru" muncul di header panel (shortcut re-run)

  # ---------------------------------------------------------------
  # Skenario 7 — Start gagal: parameter ECL belum APPROVED
  # ---------------------------------------------------------------
  Scenario: Start gagal — bobot_skenario untuk JUNI-2026 belum APPROVED ALCO
    Given mst.bobot_skenario JUNI-2026 status = "PENDING_APPROVAL"
    When RISK-01 klik "Mulai" di confirm dialog
    Then toast error persistent: "Start gagal: Parameter ECL (bobot skenario) untuk JUNI-2026 belum disetujui ALCO. Hubungi ROLE-ALCO."
    And error.code = "ECL_PARAM_NOT_FOUND"
    And calc_run status tetap "DRAFT"

  # ---------------------------------------------------------------
  # Skenario 8 — Permission: ROLE-AKUN tidak dapat start
  # ---------------------------------------------------------------
  Scenario: ROLE-AKUN membuka halaman detail DRAFT — tombol Start tidak tampil
    Given AKUN-01 memiliki role ROLE-AKUN (tidak punya calc_run.start)
    When AKUN-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001
    Then header panel tampil (read-only)
    And tombol "Start Bulk Compute" TIDAK tampil
    And tombol "Batalkan" TIDAK tampil
```

### Open Questions — M10-002
- **OQ-M10-002-A**: Cancel cancel_reason minimum 30 chars — validasi frontend (Zod) dan backend sudah selaras dari M8. Konfirmasi schema `ecl.calc_run.chk_ecl_calc_run_cancel_reason` sudah enforce ini.
- **OQ-M10-002-B**: Apakah `<JobProgressPanel>` perlu menampilkan ETA dari field `sys.job.estimatedCompletionAt`? Field ini opsional di M8 worker. Default assume: tampilkan jika tersedia, "Menghitung..." jika null.

---

## Story APP-C-M10-003 — Calc Run Detail + Parameter Snapshot

**Actor**: ROLE-RISK (primary), ROLE-AUDIT (read-only), ROLE-ALCO (untuk seal action)
**Trigger**: Navigasi ke `/ecl/calc-runs/[id]`
**Goal**: Melihat seluruh informasi calc run: header (status, periode, timing, counts), parameter snapshot frozen (read-only tree view JSONB), dan tabel hasil per stage (tabbed DataTable). Action buttons kontekstual sesuai status.

**Pre-conditions**:
- `ecl.calc_run` exists, user punya permission `calc_run.read`.

**Post-conditions**: Read-only, tidak ada mutasi data dari story ini (mutasi ditangani S2, S7).

**Komponen**:
- Header panel (status badge + action buttons kontekstual)
- Parameter snapshot section: collapsed `<JSONBTreeView>` expandable (baru, read-only), tombol "Expand All" / "Collapse All"
- Tabs: "Semua" / "Stage 1" / "Stage 2" / "Stage 3" / "Error" (jika `error_count > 0`) / "Di-skip" (FVTPL)
- `<DataTable>` per tab dengan kolom ECL (UX §1)
- Tombol kontekstual per status: Start (DRAFT), Cancel (IN_PROGRESS), Request Seal (COMPLETED), [Seal actions di S7]

**Permissions**: `calc_run.read`, `ecl.result.read`
**Audit Events**: Ditampilkan di halaman — link ke audit browser `/audit?entity_type=ecl.calc_run&entity_id={id}`

### Acceptance Criteria — APP-C-M10-003

```gherkin
Feature: Calc run detail view — header, parameter snapshot, results table

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001":
      | status            | "COMPLETED"                                   |
      | periode_id        | "JUNI-2026"                                   |
      | evaluation_date   | "2026-06-30"                                  |
      | created_by        | RISK-01                                       |
      | started_at        | "2026-06-13T10:30:00+07:00"                   |
      | completed_at      | "2026-06-13T10:35:00+07:00"                   |
      | total_instrumen   | 1000                                          |
      | processed_count   | 995                                           |
      | error_count       | 0                                             |
      | parameter_snapshot_jsonb | {bobot: {good: 0.25, normal: 0.50, bad: 0.25}, ...} |
    And ecl.calc_result_line: 700 Stage 1, 250 Stage 2, 45 Stage 3, 5 FVTPL di-skip
    And RISK-01 memiliki permission calc_run.read + ecl.result.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: halaman detail tampil lengkap
  # ---------------------------------------------------------------
  Scenario: Halaman detail calc run tampil dengan semua section
    When RISK-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001
    Then header panel menampilkan:
      | Status          | "COMPLETED" (badge hijau)                    |
      | Periode         | "JUNI-2026"                                  |
      | Eval Date       | "2026-06-30"                                 |
      | Dibuat Oleh     | "RISK-01"                                    |
      | Mulai           | "2026-06-13 10:30 WIB"                       |
      | Selesai         | "2026-06-13 10:35 WIB (durasi: 5 menit)"     |
      | Processed       | "995 / 1000"                                 |
      | Error           | "0"                                          |
    And tombol "Request Seal" tersedia (status = COMPLETED)
    And tombol "Lihat Audit Trail" tersedia (link ke /audit?entity_id=CR-JUNI-2026-001)

  # ---------------------------------------------------------------
  # Skenario 2 — Parameter snapshot: collapsed JSONB tree view
  # ---------------------------------------------------------------
  Scenario: Section parameter snapshot — collapsed, dapat di-expand
    When RISK-01 scrolls ke section "Parameter Snapshot"
    Then section tampil dalam collapsed state dengan judul "Parameter Snapshot (Frozen at 2026-06-13 10:30)"
    And badge "Read-only — Frozen" tampil di sebelah judul
    And tombol "Expand All" tersedia
    When RISK-01 klik "Expand All"
    Then tree JSONB ter-expand menampilkan seluruh parameter:
      | bobot_skenario  | { good: "0.25000000", normal: "0.50000000", bad: "0.25000000" } |
      | pd_pefindo      | { ... curve per rating ... }                                     |
      | lgd_basel       | { ... pool per tipe eksposur ... }                               |
      | impact_mev_pd   | { good: {...}, normal: {...}, bad: {...} }                        |
      | lps_coverage    | { cap_idr: "2000000000.0000", aktif: true }                      |
      | kurs_jisdor     | { USD_IDR: "15432.12345678", ... }                               |
    And nilai numeric tampil dengan presisi penuh (8 desimal untuk rate)
    And tidak ada tombol edit atau modify di section ini

  # ---------------------------------------------------------------
  # Skenario 3 — Tabs results: Stage 1/2/3 + Error + Di-skip
  # ---------------------------------------------------------------
  Scenario: Tab "Stage 2" menampilkan DataTable instrumen Stage 2
    When RISK-01 klik tab "Stage 2" (menampilkan badge "250")
    Then DataTable menampilkan 250 row dengan kolom:
      | Kode Instrumen, Nama, Tipe, Portofolio, Routing, Stage, EAD (IDR), PD (Lifetime), LGD, ECL Weighted (IDR), ECL FL (IDR), Warning |
    And kolom IDR diformat dengan 4 desimal (NUMERIC(20,4))
    And kolom PD/LGD diformat dengan 8 desimal (NUMERIC(10,8))
    And kolom "Routing" menampilkan badge: STANDARD (grey), LPS (biru), LOOKTHROUGH (ungu), POCI_DEFERRED (amber)
    And sort default: ecl_fl_idr DESC
    And DataTable mendukung sort semua kolom + filter (Kode, Portofolio, Routing, Stage)
    And Export CSV/XLSX tersedia untuk tab aktif

  # ---------------------------------------------------------------
  # Skenario 4 — Tab "Error": tampil jika error_count > 0
  # ---------------------------------------------------------------
  Scenario: Tab "Error" tersedia saat ada instrumen gagal
    Given ecl.calc_run "CR-ERR-001" dengan error_count = 3
    When RISK-01 navigasi ke halaman detail CR-ERR-001
    Then tab "Error (3)" tampil dengan badge merah
    And tab "Error" tidak muncul jika error_count = 0

  # ---------------------------------------------------------------
  # Skenario 5 — Action buttons kontekstual per status
  # ---------------------------------------------------------------
  Scenario: Tombol aksi berbeda-beda sesuai status calc run
    Given 4 calc run dengan status berbeda:
      | CR-DRAFT-001     | DRAFT                  |
      | CR-RUNNING-001   | IN_PROGRESS            |
      | CR-DONE-001      | COMPLETED              |
      | CR-SEALED-001    | SEALED                 |
    When RISK-01 navigasi ke masing-masing halaman detail
    Then tombol yang tampil per status:
      | DRAFT                | "Start Bulk Compute" + "Batalkan Draft"               |
      | IN_PROGRESS          | [JobProgressPanel] + "Batalkan" (di dalam panel)      |
      | COMPLETED            | "Request Seal" + "Lihat Hasil" (tab bawaan aktif)     |
      | COMPLETED_WITH_ERRORS| "Lihat Error" (tab error aktif) — seal TIDAK tersedia |
      | SEALED               | Semua action disabled + badge "Sealed" + "sealed_at"  |
      | CANCELLED            | "Buat Calc Run Baru" (link ke create modal)            |

  # ---------------------------------------------------------------
  # Skenario 6 — Status SEALED: lock semua action + tampilkan seal info
  # ---------------------------------------------------------------
  Scenario: Halaman detail calc run SEALED — semua tombol di-lock
    Given ecl.calc_run "CR-SEALED-001" dengan:
      | status           | "SEALED"                         |
      | sealed_at        | "2026-06-14T09:00:00+07:00"       |
      | seal_approved_by | ALCO-01                          |
      | seal_signature_1 | "abc123..." (SHA-256)            |
    When RISK-01 navigasi ke /ecl/calc-runs/CR-SEALED-001
    Then header panel menampilkan:
      | Status badge         | "SEALED" (ungu + gembok icon)                  |
      | Sealed pada          | "2026-06-14 09:00 WIB"                         |
      | Diseal oleh          | "ALCO-01"                                      |
      | Signature hash       | "abc123..." (truncated 16 char + tombol copy)  |
    And semua action buttons di-disable atau tidak tampil
    And tooltip pada area aksi: "Calc run sudah di-seal dan tidak dapat dimodifikasi (DEC-018)."

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: membuka detail, read-only
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT membuka halaman detail — semua section tampil, tanpa action
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001
    Then header panel tampil lengkap termasuk parameter snapshot
    And DataTable results tampil (dengan opsi export)
    And TIDAK ada tombol: Start, Cancel, Request Seal, Approve Seal
    And tombol "Lihat Audit Trail" tetap tersedia
```

### Open Questions — M10-003
- **OQ-M10-003-A**: Field `seal_approved_by` di response — apakah `GET /ecl/calc-runs/{id}` mengembalikan `seal_approved_by_1`, `seal_signature_1`, `sealed_at` sebagai top-level field atau di-nested dalam `seal_workflow` object? Konfirmasi ke `system-analyst` / OpenAPI M8.
- **OQ-M10-003-B**: `<JSONBTreeView>` adalah komponen baru. Perlu `uiux-designer` buat spec komponen ini. Alternatif: gunakan `react-json-view` (library) yang di-wrapping. Flag ke `frontend-engineer-nextjs`.

---

## Story APP-C-M10-004 — ECL Result Drill-Down per Instrumen

**Actor**: ROLE-RISK (primary), ROLE-AUDIT (read-only)
**Trigger**: Klik row instrumen di DataTable hasil (S3) → navigasi ke `/ecl/calc-runs/[id]/instrumen/[instrumenId]`
**Goal**: Melihat detail ECL per instrumen: routing path, stage aktif, breakdown per skenario × stage (Good/Normal/Bad columns), weighted ECL, warnings. Untuk REKSADANA: link ke look-through breakdown. Untuk CASH/DEPOSITO: link ke LPS aggregation.

**Pre-conditions**:
- `ecl.calc_result_line` untuk instrumen + calc_run_id exists.
- User punya permission `ecl.result.read`.

**Post-conditions**: Read-only.

**Komponen**:
- Breadcrumb: "Calc Runs / CR-JUNI-2026-001 / Instrumen / OBL-2026-00001"
- Info card: kode, nama, tipe, klasifikasi, portofolio, stage badge
- Routing badge: `STANDARD` / `LPS` / `LOOKTHROUGH` / `POCI_DEFERRED` / `FVTPL_SKIPPED`
- Tabel breakdown skenario: 3 kolom (Good/Normal/Bad) + kolom Weighted — per baris: PD, LGD, EAD (IDR), ECL_skenario, FL Multiplier, ECL_FL
- Summary row: bobot per skenario + ECL_weighted_total
- Warning cards (jika ada)
- Link "Lihat Look-through Detail" (REKSADANA) atau "Lihat LPS Aggregasi" (CASH/DEPOSITO)

**Permissions**: `ecl.result.read`
**Audit Events**: Tidak ada mutasi — bukan event-generating.

### Acceptance Criteria — APP-C-M10-004

```gherkin
Feature: ECL result drill-down per instrumen

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "COMPLETED"
    And ecl.calc_result_line untuk instrumen "OBL-2026-00001":
      | routing_path     | "STANDARD"                                          |
      | stage            | 2                                                   |
      | ead_idr          | 10000000000.0000                                    |
      | lgd_used         | 0.45000000                                          |
      | Skenario Good:   | pd=0.02000000, fl_multiplier=0.85000000, ecl_skenario=90000000.0000, ecl_fl=76500000.0000, bobot=0.25 |
      | Skenario Normal: | pd=0.03000000, fl_multiplier=1.00000000, ecl_skenario=135000000.0000, ecl_fl=135000000.0000, bobot=0.50 |
      | Skenario Bad:    | pd=0.06000000, fl_multiplier=1.20000000, ecl_skenario=270000000.0000, ecl_fl=324000000.0000, bobot=0.25 |
      | ecl_weighted_idr | 167625000.0000  (76500000×0.25 + 135000000×0.50 + 324000000×0.25) |
      | warnings         | []                                                  |
    And RISK-01 memiliki permission ecl.result.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: drill-down tampil lengkap dengan breakdown skenario
  # ---------------------------------------------------------------
  Scenario: Halaman drill-down instrumen OBL-2026-00001 tampil dengan tabel skenario
    When RISK-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001/instrumen/OBL-2026-00001
    Then halaman menampilkan:
      | Breadcrumb       | "Calc Runs / CR-JUNI-2026-001 / OBL-2026-00001" |
      | Routing badge    | "STANDARD" (grey badge)                         |
      | Stage badge      | "Stage 2 — SICR" (amber)                        |
      | EAD              | "Rp 10.000.000.000,0000"                         |
      | LGD              | "45.00000000%"                                  |
    And tabel breakdown skenario menampilkan 6 baris:
      | Baris       | Good          | Normal         | Bad           |
      | PD          | 2.00000000%   | 3.00000000%    | 6.00000000%   |
      | LGD         | 45.00000000%  | 45.00000000%   | 45.00000000%  |
      | EAD (IDR)   | Rp 10.000.000.000,0000 (sama semua skenario, instrumen IDR) |
      | ECL Skenario| Rp 90.000.000,0000 | Rp 135.000.000,0000 | Rp 270.000.000,0000 |
      | FL Multiplier| 0.85000000  | 1.00000000     | 1.20000000    |
      | ECL FL      | Rp 76.500.000,0000 | Rp 135.000.000,0000 | Rp 324.000.000,0000 |
    And summary row di bawah:
      | Bobot       | 25.00%        | 50.00%         | 25.00%        |
      | ECL Weighted (IDR) | colspan=3, nilai "Rp 167.625.000,0000" (bold) |
    And tidak ada warning card (warnings kosong)

  # ---------------------------------------------------------------
  # Skenario 2 — Stage 3: PD = 1.0, FL multiplier tidak diaplikasikan, Net Carrying tampil
  # ---------------------------------------------------------------
  Scenario: Instrumen Stage 3 — PD = 1.0, FL tidak diapply, Net Carrying Amount tampil
    Given ecl.calc_result_line untuk instrumen "OBL-2026-STAGE3":
      | stage            | 3                                    |
      | pd_used          | 1.00000000 (Stage 3 fixed)           |
      | net_carrying_idr | 9500000000.0000 (gross 10M - ECL 500k) |
    When RISK-01 navigasi ke drill-down halaman instrumen OBL-2026-STAGE3
    Then info card menampilkan:
      | Stage badge    | "Stage 3 — Credit-Impaired" (merah)                       |
      | Net Carrying   | "Rp 9.500.000.000,0000" (bukan Gross)                     |
    And tabel skenario menampilkan PD = "1.00000000" (semua skenario)
    And kolom "FL Multiplier" menampilkan "N/A" untuk Stage 3 dengan tooltip: "FL multiplier tidak diaplikasikan untuk Stage 3 (PD sudah fixed = 1.0)"
    And catatan info card: "Bunga dihitung dari Net Carrying Amount = Gross − ECL Allowance sebelumnya (PSAK 71 §5.4.1b)"

  # ---------------------------------------------------------------
  # Skenario 3 — Routing LOOKTHROUGH: link ke look-through detail
  # ---------------------------------------------------------------
  Scenario: Instrumen REKSADANA dengan routing LOOKTHROUGH
    Given instrumen "RD-2026-00001" routing_path = "LOOKTHROUGH"
    And ecl.lookthrough_underlying berisi 3 underlying asset classes untuk instrumen ini
    When RISK-01 navigasi ke drill-down halaman RD-2026-00001
    Then routing badge = "LOOKTHROUGH" (ungu)
    And section "Look-through Underlying" tampil di bawah breakdown skenario dengan:
      | Asset Class     | % NAB    | EAD (IDR)     | ECL per Class |
      | Obligasi        | 60%      | 6000000000    | ...           |
      | Deposito        | 30%      | 3000000000    | ...           |
      | Cash            | 10%      | 1000000000    | ...           |
    And total weighted ECL sama dengan ecl_weighted_idr header

  # ---------------------------------------------------------------
  # Skenario 4 — Routing LPS: link ke LPS aggregasi
  # ---------------------------------------------------------------
  Scenario: Instrumen DEPOSITO dengan routing LPS
    Given instrumen "DEP-2026-00001" routing_path = "LPS"
    And LPS aggregasi: nasabah X di bank Y total = Rp 3M, covered = Rp 2M, excess = Rp 1M
    When RISK-01 navigasi ke drill-down halaman DEP-2026-00001
    Then routing badge = "LPS" (biru)
    And section "LPS Aggregasi" tampil:
      | Total Eksposur (nasabah + bank) | Rp 3.000.000.000,0000      |
      | Dijamin LPS                     | Rp 2.000.000.000,0000 (cap)|
      | Excess (ECL basis)              | Rp 1.000.000.000,0000      |
    And ECL dihitung atas Rp 1M saja
    And catatan: "ECL hanya dihitung untuk excess di atas cap LPS IDR 2 miliar (DEC-014)"

  # ---------------------------------------------------------------
  # Skenario 5 — FVTPL_SKIPPED: halaman menampilkan info "tidak dihitung"
  # ---------------------------------------------------------------
  Scenario: Instrumen FVTPL — routing FVTPL_SKIPPED, tidak ada ECL
    Given instrumen "SHM-2026-00001" routing_path = "FVTPL_SKIPPED"
    When RISK-01 navigasi ke drill-down halaman SHM-2026-00001
    Then info banner tampil: "Instrumen ini berstatus FVTPL. ECL tidak dihitung sesuai PSAK 71 / IFRS 9."
    And tabel breakdown skenario TIDAK tampil
    And ECL Weighted = "-" (not applicable)

  # ---------------------------------------------------------------
  # Skenario 6 — Warning tampil sebagai card
  # ---------------------------------------------------------------
  Scenario: Instrumen dengan warning "POCI_FLAG_MISMATCH"
    Given ecl.calc_result_line untuk "OBL-WARN-001" dengan:
      | warnings | ["POCI_FLAG_MISMATCH: instrumen diduga POCI berdasarkan rating pada origination, perlu verifikasi manual"] |
    When RISK-01 membuka drill-down halaman
    Then warning card amber tampil di atas tabel breakdown:
      | Judul   | "1 Warning"                                      |
      | Konten  | "POCI_FLAG_MISMATCH: instrumen diduga POCI berdasarkan rating pada origination, perlu verifikasi manual" |

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: akses halaman drill-down — read-only
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT mengakses drill-down — semua data tampil, tidak ada aksi
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 navigasi ke halaman drill-down OBL-2026-00001
    Then semua section tampil identik dengan ROLE-RISK
    And tidak ada tombol action (re-compute, export individual row)
```

### Open Questions — M10-004
- **OQ-M10-004-A**: Endpoint `GET /ecl/calc-runs/{id}/instrumen/{instrumenId}` — apakah ada di OpenAPI M7 (`app-c-ecl-core.yaml`)? Perlu konfirmasi `system-analyst`. Jika belum ada, perlu ditambahkan sebelum frontend coding dimulai.
- **OQ-M10-004-B**: Untuk LOOKTHROUGH, data underlying di-embedded di `ecl.lookthrough_underlying` — apakah response drill-down mengambil ini sebagai nested array atau endpoint terpisah `GET /ecl/calc-runs/{id}/instrumen/{instrumenId}/lookthrough`? Default assume: nested dalam response utama (tidak ada extra call). Konfirmasi ke `system-analyst`.

---

## Story APP-C-M10-005 — Portfolio Summary + Perbandingan Run Sebelumnya

**Actor**: ROLE-RISK, ROLE-CFO
**Trigger**: Klik tab "Portfolio Summary" di halaman detail calc run → navigasi ke `/ecl/calc-runs/[id]/portofolio/[portofolioId]/summary`; atau dari menu `/ecl/portfolios/{id}/summary?calc_run_id=...`
**Goal**: Melihat agregasi ECL per portofolio: breakdown per stage, per routing path, dan per skenario (tabel + chart). Bandingkan dengan prior calc run (delta + %). Drill-down ke list instrumen per portofolio.

**Pre-conditions**:
- `ecl.calc_run` status IN (`COMPLETED`, `COMPLETED_WITH_ERRORS`, `SEALED`).
- User punya permission `ecl.portfolio_aggregate.read`.

**Post-conditions**: Read-only.

**Komponen**:
- KPI card row: Total ECL Weighted (IDR), Total Instrumen, Stage distribution counts, Error count
- `<BarChart>` ECL per stage via Recharts; `<PieChart>` distribusi routing path
- Comparison section: prior run picker + delta table
- `<DataTable>` drill-down list instrumen untuk portofolio ini (link ke S4 drill-down)
- Export CSV/XLSX (UX §1)

**Permissions**: `ecl.portfolio_aggregate.read`, `ecl.result.export`
**Audit Events**: `ECL.PORTFOLIO_AGGREGATE_READ`, `ECL.PORTFOLIO_AGGREGATE_EXPORT`

### Acceptance Criteria — APP-C-M10-005

```gherkin
Feature: Portfolio summary — agregasi ECL + perbandingan run

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "COMPLETED"
    And portofolio "PORT-OBLIGASI" dengan 100 instrumen:
      | Stage 1: 70 instrumen, ECL total = 500.000.000,0000   |
      | Stage 2: 25 instrumen, ECL total = 800.000.000,0000   |
      | Stage 3:  5 instrumen, ECL total = 200.000.000,0000   |
      | LOOKTHROUGH: 10 instrumen (subset dari Stage 1)        |
      | LPS: 5 instrumen (subset dari Stage 1)                 |
    And ecl.calc_run "CR-MEI-2026-001" (prior run, SEALED) dengan ECL total PORT-OBLIGASI = 1.400.000.000,0000
    And RISK-01 memiliki permission ecl.portfolio_aggregate.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: KPI cards + chart tampil
  # ---------------------------------------------------------------
  Scenario: Halaman portfolio summary tampil dengan KPI cards dan chart Recharts
    When RISK-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001/portofolio/PORT-OBLIGASI/summary
    Then KPI cards menampilkan:
      | Total ECL Weighted | "Rp 1.500.000.000,0000"   |
      | Total Instrumen    | 100                        |
      | Stage 1 Count      | 70                         |
      | Stage 2 Count      | 25                         |
      | Stage 3 Count      | 5                          |
    And <BarChart> menampilkan ECL per stage (3 bar: Stage 1/2/3) dengan label IDR
    And <PieChart> menampilkan distribusi routing path: STANDARD, LOOKTHROUGH, LPS, POCI_DEFERRED
    And chart interaktif: hover menampilkan tooltip nilai IDR

  # ---------------------------------------------------------------
  # Skenario 2 — Perbandingan dengan prior run: delta + %
  # ---------------------------------------------------------------
  Scenario: Bandingkan dengan CR-MEI-2026-001 — delta tampil di tabel
    When RISK-01 pilih prior run = "CR-MEI-2026-001" dari dropdown perbandingan
    Then tabel perbandingan menampilkan:
      | Metric               | CR-MEI-2026-001 (prior)    | CR-JUNI-2026-001 (current) | Delta         | Delta %    |
      | Total ECL Weighted   | Rp 1.400.000.000,0000      | Rp 1.500.000.000,0000      | +100.000.000  | +7.14%     |
      | Stage 1 ECL          | Rp 450.000.000,0000        | Rp 500.000.000,0000        | +50.000.000   | +11.11%    |
      | Stage 2 ECL          | Rp 750.000.000,0000        | Rp 800.000.000,0000        | +50.000.000   | +6.67%     |
      | Stage 3 ECL          | Rp 200.000.000,0000        | Rp 200.000.000,0000        | 0             | 0.00%      |
    And delta positif ditampilkan merah (ECL naik = risiko naik)
    And delta negatif ditampilkan hijau (ECL turun = risiko turun)
    And prior run dropdown hanya menampilkan run dari periode sebelumnya yang COMPLETED atau SEALED

  # ---------------------------------------------------------------
  # Skenario 3 — Tidak ada prior run: perbandingan tidak tersedia
  # ---------------------------------------------------------------
  Scenario: Tidak ada prior run untuk dibandingkan
    Given tidak ada calc_run COMPLETED/SEALED sebelum CR-JUNI-2026-001
    When RISK-01 membuka halaman summary
    Then section perbandingan menampilkan info: "Tidak ada calc run sebelumnya untuk perbandingan."
    And dropdown prior run tampil tapi kosong (disabled)

  # ---------------------------------------------------------------
  # Skenario 4 — DataTable drill-down instrumen dalam portofolio
  # ---------------------------------------------------------------
  Scenario: Drill-down ke list instrumen portofolio PORT-OBLIGASI
    When RISK-01 klik "Lihat Instrumen" di bawah KPI cards
    Then <DataTable> tampil dengan 100 row instrumen PORT-OBLIGASI
    Dan kolom: Kode, Nama, Stage, Routing, EAD (IDR), ECL Weighted (IDR), Warning
    And filter tersedia: Stage, Routing
    And setiap row dapat diklik → navigasi ke /ecl/calc-runs/{id}/instrumen/{instrumenId} (S4)
    And Export CSV/XLSX tersedia

  # ---------------------------------------------------------------
  # Skenario 5 — Export summary: audit event tercatat
  # ---------------------------------------------------------------
  Scenario: ROLE-CFO export portfolio summary ke XLSX
    Given CFO-01 memiliki permission ecl.portfolio_aggregate.read dan ecl.result.export
    When CFO-01 klik Export XLSX
    Then file "ecl-portfolio-PORT-OBLIGASI-CR-JUNI-2026-001-20260613.xlsx" ter-download
    And XLSX berisi KPI summary + tabel detail instrumen
    And aud.audit_log berisi "ECL.PORTFOLIO_AGGREGATE_EXPORT" dengan:
      | after_jsonb.calc_run_id    | CR-JUNI-2026-001  |
      | after_jsonb.portofolio_id  | PORT-OBLIGASI     |
      | after_jsonb.format         | xlsx              |

  # ---------------------------------------------------------------
  # Skenario 6 — ROLE tanpa ecl.portfolio_aggregate.read di-block
  # ---------------------------------------------------------------
  Scenario: ROLE-MAKER-TR tidak dapat akses portfolio summary
    Given MAKER-01 dengan role ROLE-MAKER-TR
    When MAKER-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001/portofolio/PORT-OBLIGASI/summary
    Then redirect ke 403 dengan pesan: "Akses tidak diizinkan. Permission ecl.portfolio_aggregate.read diperlukan."
```

### Open Questions — M10-005
- **OQ-M10-005-A**: Prior run comparison: endpoint yang dipakai apakah `GET /ecl/calc-runs/{id}/portfolio/{portId}/summary?prior_calc_run_id=...` (satu endpoint dengan query param) atau dua endpoint terpisah? Konfirmasi ke `system-analyst`. Default assume: satu endpoint dengan optional `prior_calc_run_id`.
- **OQ-M10-005-B**: Apakah ECL per-skenario (Good/Normal/Bad) perlu ditampilkan di level portofolio summary (kolom tambahan di tabel perbandingan)? Default assume: hanya total weighted + per-stage breakdown. Per-skenario tersedia di S4 drill-down instrumen.

---

## Story APP-C-M10-006 — Roll-Forward Report

**Actor**: ROLE-CFO, ROLE-AUDIT (primary); ROLE-RISK (read)
**Trigger**: Klik "Lihat Roll-Forward" di halaman detail calc run, atau navigasi ke `/ecl/calc-runs/[id]/roll-forward?priorCalcRunId=[priorId]`
**Goal**: Melihat laporan roll-forward CKPN: Opening + Originations − Derecognitions ± Transfers (stage changes) ± Remeasurements = Closing. Reconcile status harus TRUE sebelum laporan dianggap final. Export CSV/XLSX.

**Pre-conditions**:
- `ecl.calc_run` status IN (`COMPLETED`, `SEALED`).
- Prior calc run dipilih (harus COMPLETED atau SEALED dari periode sebelumnya).
- User punya permission `ecl.result.read`.

**Post-conditions**: Read-only. Export audit event tercatat.

**Komponen**:
- Prior calc run picker (`<Select>` — filter run dari periode ≤ current)
- Roll-forward waterfall table: baris per komponen + total
- Reconcile status badge: `RECONCILED` (hijau) / `PARTIAL_PHASE_5_DEFER` (amber) / `MISMATCH` (merah)
- Export CSV/XLSX (UX §1)

**Permissions**: `ecl.result.read`, `ecl.result.export`
**Audit Events**: `ECL.ROLL_FORWARD_READ`, `ECL.ROLL_FORWARD_EXPORT`

### Acceptance Criteria — APP-C-M10-006

```gherkin
Feature: Roll-forward CKPN report

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "COMPLETED"
    And prior calc_run "CR-MEI-2026-001" status = "SEALED"
    And roll-forward data tersedia dari backend:
      | Komponen             | IDR                    |
      | Opening (MEI-2026)   | 1.400.000.000,0000     |
      | Originations (+)     | 200.000.000,0000       |
      | Derecognitions (-)   | 50.000.000,0000        |
      | Transfer Stage 1→2 (+)| 80.000.000,0000      |
      | Transfer Stage 2→1 (-)| 30.000.000,0000      |
      | Transfer Stage 2→3 (+)| 100.000.000,0000     |
      | Remeasurements       | 0,0000                 |
      | Closing (JUNI-2026)  | 1.700.000.000,0000     |
    And reconcile_status = "RECONCILED" (ECL_closing dari calc run = 1.700.000.000,0000 ✓)
    And CFO-01 memiliki permission ecl.result.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: roll-forward tampil dengan status RECONCILED
  # ---------------------------------------------------------------
  Scenario: Halaman roll-forward tampil dengan waterfall table dan badge RECONCILED
    When CFO-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001/roll-forward?priorCalcRunId=CR-MEI-2026-001
    Then halaman menampilkan:
      | Judul            | "Roll-Forward CKPN: MEI-2026 → JUNI-2026"            |
      | Prior Run picker | "CR-MEI-2026-001 (MEI-2026, SEALED)" terpilih         |
    And waterfall table menampilkan semua komponen dengan format IDR 4 desimal
    And baris "Closing" menampilkan nilai bold "Rp 1.700.000.000,0000"
    And badge reconcile: "RECONCILED" (hijau) dengan tooltip: "Closing matches ecl_weighted_idr total dari calc run JUNI-2026. Selisih: Rp 0."
    And Export CSV dan Export XLSX tersedia

  # ---------------------------------------------------------------
  # Skenario 2 — Status PARTIAL_PHASE_5_DEFER: komponen null ditampilkan jelas
  # ---------------------------------------------------------------
  Scenario: Roll-forward dengan status PARTIAL_PHASE_5_DEFER — beberapa komponen null
    Given roll-forward berisi Transfers = null dan Remeasurements = null (Phase 5 defer)
    When CFO-01 membuka roll-forward report
    Then badge "PARTIAL — Fase 5 Defer" (amber) tampil
    And komponen null ditampilkan sebagai "-" (bukan Rp 0) dengan tooltip "Data belum tersedia (Phase 5)"
    And catatan di bawah tabel: "Transfer antar stage dan remeasurements akan tersedia setelah Phase 5 (GL/jurnal engine) selesai. Laporan ini bersifat partial."
    And tombol Export tetap tersedia (data partial dapat diekspor)

  # ---------------------------------------------------------------
  # Skenario 3 — Mismatch: closing tidak sesuai ECL total calc run
  # ---------------------------------------------------------------
  Scenario: MISMATCH — closing roll-forward berbeda dengan ECL total calc run
    Given roll-forward Closing = 1.700.000.000,0000 tapi ecl_weighted_total calc run = 1.700.001.000,0000 (selisih Rp 1.000)
    When CFO-01 membuka roll-forward report
    Then badge "MISMATCH" (merah) tampil dengan pesan:
      "Closing roll-forward (Rp 1.700.000.000,0000) tidak sama dengan ECL total calc run (Rp 1.700.001.000,0000). Selisih: Rp 1.000,0000. Investigasi diperlukan sebelum seal."
    And tombol "Request Seal" di halaman detail calc run di-disable selama MISMATCH
    And link ke /audit?entity_type=ecl.roll_forward untuk investigasi

  # ---------------------------------------------------------------
  # Skenario 4 — Prior run picker: hanya run COMPLETED atau SEALED dari periode sebelumnya
  # ---------------------------------------------------------------
  Scenario: Prior run picker hanya menampilkan run valid
    Given 4 calc_run sebelumnya:
      | CR-MEI-2026-001    | MEI-2026   | SEALED                 | ✓ tampil |
      | CR-MEI-2026-002    | MEI-2026   | CANCELLED              | ✗ tidak tampil |
      | CR-APRIL-2026-001  | APRIL-2026 | COMPLETED              | ✓ tampil |
      | CR-JULI-2026-001   | JULI-2026  | DRAFT                  | ✗ tidak tampil (periode lebih baru) |
    When CFO-01 membuka dropdown prior run picker
    Then hanya CR-MEI-2026-001 dan CR-APRIL-2026-001 yang tampil di dropdown

  # ---------------------------------------------------------------
  # Skenario 5 — Export XLSX — audit event tercatat
  # ---------------------------------------------------------------
  Scenario: CFO-01 export roll-forward ke XLSX
    When CFO-01 klik Export XLSX
    Then file "roll-forward-JUNI-2026-20260613.xlsx" ter-download
    And XLSX berisi: waterfall table + reconcile status + metadata (diekspor oleh, tanggal, filter aktif)
    And aud.audit_log berisi "ECL.ROLL_FORWARD_EXPORT" dengan:
      | after_jsonb.calc_run_id       | CR-JUNI-2026-001  |
      | after_jsonb.prior_calc_run_id | CR-MEI-2026-001   |
      | after_jsonb.format            | xlsx              |

  # ---------------------------------------------------------------
  # Skenario 6 — Tidak ada prior run: roll-forward tidak dapat dihasilkan
  # ---------------------------------------------------------------
  Scenario: Tidak ada prior run COMPLETED/SEALED — roll-forward tidak tersedia
    Given ini adalah calc run pertama (tidak ada run sebelumnya)
    When CFO-01 navigasi ke halaman roll-forward tanpa priorCalcRunId
    Then halaman menampilkan: "Roll-forward tidak tersedia: tidak ada calc run dari periode sebelumnya yang COMPLETED atau SEALED."
    And dropdown prior run kosong (disabled)
    And Export tidak tersedia
```

### Open Questions — M10-006
- **OQ-M10-006-A**: Endpoint roll-forward `GET /ecl/roll-forward` — apakah ada di OpenAPI M7 (`app-c-ecl-core.yaml`) atau perlu ditambahkan di M11? Dari plan §2 M11 adalah modul terpisah. Perlu konfirmasi ke `system-analyst`: apakah endpoint ini sudah include `reconcile_status` field dan MISMATCH detection, atau hanya query raw komponen?
- **OQ-M10-006-B**: MISMATCH tolerance: apakah selisih < Rp 1 (delta < IDR 1,0000) dianggap RECONCILED (floating-point rounding)? Plan §11 menyebut "delta < IDR 1". Konfirmasi ini harus di-enforce server-side di M11 service.

---

## Story APP-C-M10-007 — Seal Workflow UI (Request + Approve + Reject)

**Actor**: ROLE-RISK (request seal), ROLE-ALCO (approve seal + step-up MFA), ROLE-CFO (co-approver)
**Trigger**: Tombol "Request Seal" di halaman detail calc run (`status = 'COMPLETED'`); atau tombol "Approve Seal" / "Tolak Seal" yang muncul untuk ROLE-ALCO/ROLE-CFO saat `status = 'SEAL_REQUESTED'`
**Goal**: ROLE-RISK mengajukan seal request. ROLE-ALCO menyetujui dengan step-up MFA (4-eyes: RISK request → ALCO approve). Setelah SEALED: semua action di-lock, signature hash + seal info tampil. Jika ditolak: RISK dapat mengajukan ulang.

**Note**: Seal workflow menggunakan 4-eyes (RISK maker → ALCO approver) sesuai OpenAPI M8 (`app-c-calc-run.yaml`). OQ-M8-1 (6-eyes vs 4-eyes) belum final — jika ifrs9-compliance-reviewer memutuskan 6-eyes, story ini perlu revisi AC untuk tambah step APPROVE_STEP_2.

**Pre-conditions**:
- `ecl.calc_run.status = 'COMPLETED'` untuk Request Seal.
- `ecl.calc_run.status = 'SEAL_REQUESTED'` untuk Approve/Reject.
- SoD: `created_by ≠ seal_approver_id`.
- ROLE-ALCO atau ROLE-CFO untuk approve (permission `calc_run.seal_approve`).
- Step-up MFA selesai untuk approver (DEC-027).

**Post-conditions (SEALED)**:
- `ecl.calc_run.status = 'SEALED'`, `sealed_at`, `seal_signature_1` terisi.
- Semua action buttons di-lock.
- DB trigger `fn_ecl_calc_run_no_modify_when_sealed` aktif.
- Audit event `CALC_RUN.SEALED`.

**Post-conditions (SEAL_REJECTED)**:
- `ecl.calc_run.status = 'SEAL_REJECTED'`.
- `calc_run` kembali ke status yang memungkinkan re-request (business rule: status menjadi `COMPLETED` kembali, bukan terminal).
- ROLE-RISK dapat re-request seal.

**Komponen**:
- Tombol "Request Seal" → modal dengan comment textarea (min 20 char)
- Tombol "Approve Seal" (ROLE-ALCO saat SEAL_REQUESTED) → `<MFAStepUpModal>` + comment textarea
- Tombol "Tolak Seal" (ROLE-ALCO/ROLE-CFO) → `<DestructiveActionDialog>` + reject_reason textarea (min 30 char)
- Seal info section (setelah SEALED): signature hash, seal_at, sealed_by, badge

**Permissions**: `calc_run.seal_request` (ROLE-RISK), `calc_run.seal_approve` (ROLE-ALCO, ROLE-CFO)
**Audit Events**: `CALC_RUN.SEAL_REQUESTED`, `CALC_RUN.SEAL_APPROVED`, `CALC_RUN.SEALED`, `CALC_RUN.SEAL_REJECTED`, `CALC_RUN.SOD_VIOLATION_ATTEMPT`

### Acceptance Criteria — APP-C-M10-007

```gherkin
Feature: Seal workflow UI — request, approve (MFA), reject

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "COMPLETED", created_by = RISK-01
    And RISK-01 memiliki permission calc_run.seal_request
    And ALCO-01 memiliki permission calc_run.seal_approve (ROLE-ALCO)
    And CFO-01 memiliki permission calc_run.seal_approve (ROLE-CFO)
    And tidak ada SEALED run untuk periode JUNI-2026

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: RISK request seal → ALCO approve → SEALED
  # ---------------------------------------------------------------
  Scenario: Full seal flow — RISK-01 request, ALCO-01 approve dengan MFA
    When RISK-01 klik "Request Seal" di halaman detail CR-JUNI-2026-001
    Then modal "Request Seal — CR-JUNI-2026-001" muncul:
      | Deskripsi  | "Seal akan mengunci hasil ECL ini secara permanen. Setelah di-seal, tidak ada modifikasi yang diizinkan (DEC-018)." |
      | Input      | Textarea "Catatan request (wajib, minimal 20 karakter)"                                                             |
      | Tombol     | "Kirim Request" (biru) | "Batal"                                                                            |
    When RISK-01 isi comment "ECL JUNI-2026 siap di-seal, sudah direview tim Risk."
    And klik "Kirim Request"
    Then POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan action = "REQUEST" dikirim
    And status badge berubah ke "SEAL REQUESTED" (kuning)
    And tombol "Request Seal" diganti dengan info: "Menunggu persetujuan ALCO"
    And toast sukses (RISK-01): "Request seal CR-JUNI-2026-001 dikirim. Menunggu persetujuan ALCO."
    And ALCO-01 menerima notifikasi: "Calc run CR-JUNI-2026-001 menunggu approval seal Anda."

    When ALCO-01 navigasi ke /ecl/calc-runs/CR-JUNI-2026-001
    Then tombol "Approve Seal" dan "Tolak Seal" tampil (karena status = SEAL_REQUESTED dan ALCO-01 ≠ RISK-01)
    When ALCO-01 klik "Approve Seal"
    Then modal konfirmasi pre-MFA muncul:
      | Judul      | "Approve Seal — CR-JUNI-2026-001"                                 |
      | Deskripsi  | "Anda akan menyetujui final seal hasil ECL JUNI-2026. Tindakan ini tidak dapat dibalik."  |
      | Input      | Textarea "Catatan approval (wajib, minimal 20 karakter)"          |
      | Tombol     | "Lanjutkan ke Verifikasi MFA" | "Batal"                          |
    When ALCO-01 isi comment "Parameter dan hasil ECL konsisten. Disetujui."
    And klik "Lanjutkan ke Verifikasi MFA"
    Then <MFAStepUpModal> tampil dengan instruksi TOTP/WebAuthn
    And modal menampilkan: "Step-up MFA diperlukan untuk tindakan sensitif ini (DEC-027). Verifikasi identitas Anda."
    When ALCO-01 submit kode MFA valid
    Then POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan action = "APPROVE" + X-Step-Up-Token dikirim
    And response 200 dengan data.status = "SEALED"
    And status badge berubah ke "SEALED" (ungu + gembok)
    And semua action buttons di-lock
    And seal info section tampil:
      | Sealed pada    | {sealed_at ISO 8601}              |
      | Diseal oleh    | "ALCO-01"                         |
      | Signature hash | {16 char truncated + tombol copy} |
    And toast sukses (ALCO-01): "Calc run CR-JUNI-2026-001 berhasil di-seal. Hasil ECL JUNI-2026 final dan immutable."
    And aud.audit_log berisi "CALC_RUN.SEALED" dengan seal_chain_jsonb lengkap

  # ---------------------------------------------------------------
  # Skenario 2 — ROLE-RISK tidak dapat approve seal milik sendiri (SoD)
  # ---------------------------------------------------------------
  Scenario: SoD violation — RISK-01 mencoba approve seal yang dia sendiri request
    Given CR-JUNI-2026-001 status = "SEAL_REQUESTED", seal_requested_by = RISK-01
    And RISK-01 juga memiliki permission calc_run.seal_approve (hipotesis)
    When RISK-01 membuka halaman detail CR-JUNI-2026-001
    Then tombol "Approve Seal" TIDAK tampil untuk RISK-01 (di-hide berdasarkan created_by = current user)
    And tooltip pada area approve: "Anda adalah pembuat calc run ini. SoD tidak memperbolehkan self-approval (DEC-017)."
    And jika RISK-01 mencoba via API langsung: response 403 error.code = "SOD_VIOLATION"
    And aud.audit_log berisi "CALC_RUN.SOD_VIOLATION_ATTEMPT"

  # ---------------------------------------------------------------
  # Skenario 3 — MFA step-up gagal: approve ditolak, status tidak berubah
  # ---------------------------------------------------------------
  Scenario: Approve gagal — MFA token expired / invalid
    Given ALCO-01 ada di langkah <MFAStepUpModal>
    When ALCO-01 submit kode MFA yang expired (lebih dari 5 menit dari waktu generate)
    Then <MFAStepUpModal> menampilkan error: "Kode MFA tidak valid atau sudah kadaluarsa. Coba lagi."
    And POST seal APPROVE tidak dikirim ke backend
    And status CR-JUNI-2026-001 tetap "SEAL_REQUESTED"
    And tidak ada perubahan di aud.audit_log

  # ---------------------------------------------------------------
  # Skenario 4 — ALCO tolak seal: status ke SEAL_REJECTED, RISK dapat re-request
  # ---------------------------------------------------------------
  Scenario: ALCO-01 menolak seal — RISK dapat mengajukan ulang
    Given CR-JUNI-2026-001 status = "SEAL_REQUESTED"
    When ALCO-01 klik "Tolak Seal"
    Then <DestructiveActionDialog> muncul:
      | Judul      | "Tolak Seal Calc Run?"                                           |
      | Deskripsi  | "Penolakan akan mengembalikan calc run ke status COMPLETED. ROLE-RISK dapat mengajukan ulang setelah perbaikan." |
      | Input      | Textarea "Alasan penolakan (wajib, minimal 30 karakter)"         |
      | Tombol     | "Tolak Seal" (merah) | "Batal"                                  |
    When ALCO-01 isi alasan "Parameter LGD pool perlu diverifikasi ulang sebelum seal."
    And klik "Tolak Seal"
    Then POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan action = "REJECT" dikirim
    And response 200 dengan data.status = "COMPLETED" (kembali ke COMPLETED, bukan terminal)
    And status badge kembali ke "COMPLETED"
    And notification card merah tampil di halaman detail: "Seal ditolak oleh ALCO-01: 'Parameter LGD pool perlu diverifikasi ulang...'"
    And tombol "Request Seal" tersedia kembali untuk RISK-01
    And toast sukses (ALCO-01): "Seal CR-JUNI-2026-001 berhasil ditolak."
    And aud.audit_log berisi "CALC_RUN.SEAL_REJECTED" dengan reject_reason

  # ---------------------------------------------------------------
  # Skenario 5 — Comment request seal terlalu pendek: validasi frontend
  # ---------------------------------------------------------------
  Scenario: Request seal gagal validasi — comment kurang dari 20 karakter
    When RISK-01 isi comment = "Siap seal"
    And klik "Kirim Request"
    Then modal tidak di-submit
    And inline error pada textarea: "Catatan harus minimal 20 karakter (saat ini: 9 karakter)."
    And tombol "Kirim Request" tetap aktif setelah user memperbaiki

  # ---------------------------------------------------------------
  # Skenario 6 — Request seal tidak tersedia jika status bukan COMPLETED
  # ---------------------------------------------------------------
  Scenario: Tombol Request Seal tidak muncul untuk status DRAFT, IN_PROGRESS, CANCELLED
    Given calc_run CR-DRAFT-001 status = "DRAFT"
    When RISK-01 navigasi ke halaman detail CR-DRAFT-001
    Then tombol "Request Seal" TIDAK tampil
    And tombol yang tampil: "Start Bulk Compute" dan "Batalkan Draft"

  # ---------------------------------------------------------------
  # Skenario 7 — SEALED: lock permanen semua action + tampilkan seal detail
  # ---------------------------------------------------------------
  Scenario: Setelah SEALED — semua tombol di-lock, seal info tampil permanen
    Given CR-JUNI-2026-001 sudah SEALED
    When RISK-01 atau ALCO-01 navigasi ke halaman detail
    Then header panel menampilkan:
      | Status badge   | "SEALED" (ungu + gembok icon)                         |
      | Sealed pada    | "2026-06-14 09:00 WIB"                                |
      | Diseal oleh    | "ALCO-01"                                             |
      | Signature hash | "abc12345def67890" (16 char truncated) + tombol "Salin"|
    And semua tombol action di-hide atau di-disable
    And tooltip pada area sealed: "Calc run ini sudah di-seal dan bersifat immutable (DEC-018). Tidak dapat dimodifikasi."
    And DataTable results tetap dapat dilihat dan diekspor

  # ---------------------------------------------------------------
  # Nota OQ-M8-1: 4-eyes vs 6-eyes
  # ---------------------------------------------------------------
  # Story ini mengimplementasikan seal 4-eyes per OpenAPI app-c-calc-run.yaml:
  # RISK request → ALCO approve (satu step approve).
  # Jika ifrs9-compliance-reviewer memutuskan 6-eyes diperlukan, AC ini perlu revisi:
  # tambah Skenario "APPROVE_STEP_2" dan status "PENDING_SEAL_APPROVAL_2".
  # OQ-M8-1 harus dijawab sebelum frontend coding dimulai.
```

### Open Questions — M10-007
- **OQ-M10-007-A**: Status setelah SEAL_REJECTED — apakah kembali ke `COMPLETED` atau ada status baru `SEAL_REJECTED`? OpenAPI M8 mendefinisikan enum `DRAFT | IN_PROGRESS | COMPLETED | COMPLETED_WITH_ERRORS | SEALED | CANCELLED`. Status `SEAL_REQUESTED` dan `SEAL_REJECTED` belum eksplisit di enum (mungkin ditambah di state machine). Konfirmasi ke `system-analyst`.
- **OQ-M10-007-B**: Notifikasi ke ALCO saat seal request: apakah via in-app notifikasi (WebSocket/polling `/api/v1/notifications`) atau hanya email? Dari M8, disebutkan "notifikasi ke ROLE-ALCO" tapi tidak spesifik medium. Default assume: in-app notifikasi badge + optional email. Konfirmasi ke `tech-lead-orchestrator`.
- **OQ-M10-007-C**: OQ-M8-1 BELUM dijawab: 4-eyes vs 6-eyes. Story ini align ke 4-eyes berdasarkan OpenAPI M8. Jika compliance reviewer memutuskan 6-eyes, story ini **HARUS direvisi** sebelum PR frontend. **Flag urgent ke `ifrs9-compliance-reviewer`**.

---

## Ringkasan Handoff

```
business-analyst (stories ini selesai)
        ↓
system-analyst
  — Konfirmasi endpoint GET /ecl/calc-runs/{id}/instrumen/{instrumenId} ada di OpenAPI
  — Konfirmasi field seal_workflow state (SEAL_REQUESTED, SEAL_REJECTED) di enum
  — Konfirmasi roll-forward endpoint include reconcile_status field
  — Konfirmasi prior_calc_run comparison di portfolio summary endpoint
        ↓
uiux-designer (paralel)
  — Spec <JSONBTreeView> komponen baru (OQ-M10-003-B)
  — Responsif seal info section layout
        ↓
frontend-engineer-nextjs
  — Implement 7 story set di atas, reuse komponen Phase 3 + M9
        ↓
qa-engineer
  — UAT scripts per story + aksesibilitas WCAG 2.1 AA
        ↓
ifrs9-compliance-reviewer (advisory)
  — Jawab OQ-M8-1 (4-eyes vs 6-eyes) — URGENT, blocking M10-007
  — Verify scenario breakdown tampilkan ECL per skenario + bobot (DEC-010)
  — Verify seal button = destructive dialog + step-up MFA (DEC-027)
```
