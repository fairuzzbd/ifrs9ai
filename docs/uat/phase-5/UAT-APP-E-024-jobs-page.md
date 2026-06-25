# UAT-APP-E-024 — Job History Page /jobs

**UAT ID**: UAT-APP-E-024
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-05 (partial — /jobs page)
**AC yang dicakup**: M15-05-AC2, M15-05-AC3, M15-05-AC4 (/jobs DataTable)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — `JOB_NOT_OWNED_BY_USER` server-side enforcement; SoD cancel (owner atau IT-ADMIN only); permission `jobs.read_all` hanya ROLE-IT-ADMIN

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M13 deployed — `GET /api/v1/jobs/{jobId}`, `POST /api/v1/jobs/{jobId}/cancel` aktif
3. M15 deployed — `GET /api/v1/jobs` (list endpoint baru; catatan: endpoint ini perlu ditambah di system-analyst handoff)
4. Data seed:
   - `sys.job`: 240 total records (semua user, semua tipe)
   - USR-MAKER-001 jobs: 5 records — JOB-00001 status=`running` (progress=47, canCancel=true, resultUrl=null); JOB-00002 status=`completed` (resultUrl aktif); JOB-00003 status=`failed`; JOB-00004 status=`completed`; JOB-00005 status=`cancelled`
   - USR-RISK-001 jobs: 2 records — JOB-ECL-RUN-001 status=`running` (canCancel=true, createdBy=USR-RISK-001)
   - ROLE-IT-ADMIN dapat melihat semua 240 jobs
5. User test:

| User ID | Role | `jobs.read` | `jobs.read_all` | MFA |
|---|---|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | Ya | TIDAK | Tidak |
| USR-RISK-001 | ROLE-RISK | Ya | TIDAK | Tidak |
| USR-IT-001 | ROLE-IT-ADMIN | Ya | Ya | Ya |
| USR-AUDIT-001 | ROLE-AUDIT | Ya | TIDAK | Tidak |

6. MinIO: bucket `exports` aktif; JOB-00002 result file ada di `exports/TUGURE/USR-MAKER-001/2026/06/25/JOB-00002.xlsx`

---

## Data Test Numerik

- Total job USR-MAKER-001: 5; total semua user: 240
- JOB-00001: ECL Calc Run, running, 47%, dimulai 10:30; ETA 10:35; canCancel=true
- JOB-00002: Export MTM Daily, completed, 100%, durasi 2 menit; resultUrl tersedia
- JOB-00003: Export Instrumen, failed, 30%, durasi 1 menit
- JOB-ECL-RUN-001: ECL Calc Run, running (milik USR-RISK-001, bukan USR-MAKER-001)

---

## Skenario UAT

### TC-001 — M15-05-AC2: DataTable /jobs load untuk owner — hanya job sendiri

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR, `jobs.read`)

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/jobs`
3. Perhatikan DataTable yang muncul

**Hasil yang Diharapkan**:
- Heading: "Riwayat Job" (dengan deskripsi "Pantau dan kelola background jobs BLIPS")
- DataTable menampilkan 5 jobs milik USR-MAKER-001 saja (tidak tampil job USR-RISK-001)
- Kolom: Job ID, Tipe (label Bahasa Indonesia), Status, Progress, Dimulai, Selesai/ETA, Durasi, Aksi
- Default sort: `created_at DESC` (JOB-00001 muncul pertama — paling baru, masih running)
- Status badge: JOB-00001 "Berjalan" (biru); JOB-00002 "Selesai" (hijau); JOB-00003 "Gagal" (merah)
- Tidak ada kolom "Dibuat Oleh" (hanya IT-ADMIN yang melihat kolom ini)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-05-AC2: DataTable sort — klik header kolom "Dimulai"

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/jobs`
2. Klik header kolom "Dimulai" satu kali (sort ascending)
3. Klik lagi (sort descending)

**Hasil yang Diharapkan**:
- Klik pertama: job diurut by `started_at ASC` — JOB-00005 (paling lama) muncul pertama
- Klik kedua: job diurut by `started_at DESC` — JOB-00001 (terbaru, masih running) muncul pertama
- Header kolom menampilkan ikon sort ↑ atau ↓ sesuai arah

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-05-AC2: DataTable filter — filter by status "Selesai"

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/jobs`
2. Klik dropdown "Status" → pilih "Selesai" (completed)
3. Perhatikan filter chip yang muncul
4. Klik "Clear" atau "×" di filter chip

**Hasil yang Diharapkan**:
- DataTable hanya menampilkan JOB-00002 dan JOB-00004 (status=completed)
- Filter chip: "[Status: Selesai ×]" tampil di filter bar
- URL diperbarui: `?filter[status]=completed` (deep-link friendly)
- Klik × filter chip → filter terhapus → semua 5 jobs tampil kembali

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-05-AC2: DataTable filter — filter by tipe "ECL Calc Run"

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/jobs`
2. Klik dropdown "Tipe" → pilih "ECL Calc Run"
3. Perhatikan hasil

**Hasil yang Diharapkan**:
- Hanya JOB-00001 (ECL Calc Run) yang tampil
- Filter chip: "[Tipe: ECL Calc Run ×]"
- Filter dapat dikombinasikan dengan status filter

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-05-AC2: DataTable pagination — cursor-based

**Actor**: USR-IT-001 (ROLE-IT-ADMIN — lihat 240 jobs)

**Langkah**:
1. Login sebagai USR-IT-001
2. Navigasi ke `/jobs`
3. Perhatikan footer pagination
4. Klik "Next →"
5. Ubah limit ke 25 dari dropdown

**Hasil yang Diharapkan**:
- Footer: "Hal. 1 dari ~5" (240 / 50 ≈ 5 halaman)
- Total estimasi: "240 jobs"
- Klik "Next →": halaman 2 tampil; cursor diperbarui di URL
- Ubah limit ke 25: DataTable load ulang dengan 25 rows per halaman; "~10 halaman"
- Tidak ada offset pagination (cursor-based sesuai DEC-022)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-05-AC2: DataTable export CSV + XLSX

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/jobs`
2. Filter status=running (JOB-00001 saja)
3. Klik dropdown "Export ▾" → pilih "CSV"
4. Klik "Export ▾" → pilih "XLSX"
5. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- Download CSV dimulai dengan file: `jobs-export-{tanggal}.csv`
- File CSV berisi hanya JOB-00001 (mengikuti filter aktif)
- Download XLSX: header row bold + freeze; kolom money diformat `#,##0`
- `aud.audit_log`: `action='EXPORT.GENERATED'` — in-transaction
  - `after_jsonb`: `{format: "csv", row_count: 1, filters: {status: "running"}}`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-05-AC2: ROLE-IT-ADMIN melihat semua jobs + kolom "Dibuat Oleh" + filter user

**Actor**: USR-IT-001 (ROLE-IT-ADMIN, `jobs.read_all`)

**Langkah**:
1. Login sebagai USR-IT-001
2. Navigasi ke `/jobs`
3. Perhatikan kolom yang tampil
4. Gunakan filter "Filter by User" — ketik "USR-MAKER"

**Hasil yang Diharapkan**:
- Kolom tambahan "Dibuat Oleh" (Created By) muncul antara "Tipe" dan "Status"
- Total estimasi: 240 (semua jobs semua user)
- Jobs dari berbagai user tampil (USR-MAKER-001, USR-RISK-001, dll.)
- Filter "Filter by User" (typeahead): ketik "USR-MAKER" → tampil hanya 5 jobs USR-MAKER-001

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-008 — M15-05-AC3: Cancel running job — confirm dialog + success + status update

**Actor**: USR-MAKER-001
**Pre-kondisi**: JOB-00001 status=`running`, `canCancel=true`, dimiliki USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/jobs`
3. Perhatikan row JOB-00001 — tombol "Batalkan" (✗)
4. Klik "Batalkan"
5. Baca dialog konfirmasi
6. Klik "Ya, batalkan" (konfirmasi)

**Hasil yang Diharapkan**:
- Tombol "Batalkan" visible pada row JOB-00001 (running job, owner)
- Dialog konfirmasi: "Batalkan job JOB-00001 (ECL Calc Run)? Proses yang sudah selesai tidak bisa dikembalikan."
- Klik konfirmasi → `POST /api/v1/jobs/JOB-00001/cancel` → HTTP 200 `{status: "cancelled"}`
- Toast success: "Job JOB-00001 berhasil dibatalkan."
- Row JOB-00001 status update ke "Dibatalkan" (polling 2 detik atau SSE)
- Tombol "Batalkan" hilang dari row (job sudah terminal)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-009 — M15-05-AC3: Download completed job result → browser download + toast

**Actor**: USR-MAKER-001
**Pre-kondisi**: JOB-00002 status=`completed`, `resultUrl` aktif (signed URL MinIO TTL 24 jam)

**Langkah**:
1. Navigasi ke `/jobs`
2. Perhatikan row JOB-00002 — tombol "Unduh" (↓)
3. Klik "Unduh"
4. Periksa browser download dan toast

**Hasil yang Diharapkan**:
- Tombol "Unduh" visible pada row JOB-00002 (completed + resultUrl tersedia)
- Klik "Unduh" → MinIO signed URL di-fetch → browser trigger download
- Toast success: "Download dimulai — file JOB-00002.xlsx sedang diunduh."
- File JOB-00002.xlsx berhasil diunduh (konten XLSX valid)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-010 — M15-05-AC3: Non-owner tidak bisa cancel job milik user lain — Absent from DOM

**Actor**: USR-RISK-001 mencoba cancel JOB-00001 (milik USR-MAKER-001)

**Langkah**:
1. Login sebagai USR-RISK-001
2. Navigasi ke `/jobs`
3. Perhatikan daftar job yang tampil
4. Inspect DOM: cari tombol "Batalkan" untuk JOB-00001

**Hasil yang Diharapkan**:
- USR-RISK-001 TIDAK melihat JOB-00001 di list (`jobs.read` filter by owner=USR-RISK-001 di backend)
- JOB-00001 tidak muncul sama sekali (backend filter server-side)
- Tombol "Batalkan" TIDAK ADA di DOM untuk job milik user lain

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-011 — M15-05-AC3: Direct API cancel tanpa ownership → 403 JOB_NOT_OWNED_BY_USER

**Actor**: USR-RISK-001 mencoba `POST /api/v1/jobs/JOB-00001/cancel` langsung via API

**Langkah**:
1. Login sebagai USR-RISK-001
2. Buka browser DevTools Console
3. Jalankan: `fetch("/api/v1/jobs/JOB-00001/cancel", { method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": "IK-CANCEL-TEST" }, body: "{}" })`
4. Periksa response

**Hasil yang Diharapkan**:
- HTTP 403
- Response body: `{ "error": { "code": "JOB_NOT_OWNED_BY_USER", "message": "...", "traceId": "..." } }`
- Job JOB-00001 status TIDAK berubah (tetap running)
- `aud.audit_log`: tidak ada entry untuk cancel yang gagal (403 tidak mengubah state)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-012 — M15-05-AC3: Cancel job terminal → 409 JOB_ALREADY_TERMINAL

**Actor**: USR-MAKER-001 mencoba cancel JOB-00002 (sudah COMPLETED)

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/jobs`
3. Perhatikan row JOB-00002 — tombol "Batalkan" TIDAK ADA (completed job)
4. Via API: `POST /api/v1/jobs/JOB-00002/cancel`

**Hasil yang Diharapkan**:
- Tombol "Batalkan" TIDAK ADA untuk JOB-00002 di UI (terminal status)
- API return: HTTP 409 `{ "error": { "code": "JOB_ALREADY_TERMINAL", ... } }`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-013 — M15-05-AC4: Aksesibilitas /jobs DataTable — aria-labels + keyboard

**Actor**: USR-AUDIT-001 (atau USR-MAKER-001)
**Tools**: DevTools Accessibility Inspector; keyboard only

**Langkah**:
1. Navigasi ke `/jobs`
2. Inspeksi table via Accessibility Inspector
3. Inspeksi tombol Batalkan dan Unduh
4. Inspeksi dropdown filter Status dan Tipe
5. Tab navigate seluruh halaman

**Hasil yang Diharapkan**:
- Table: `aria-label="Riwayat Job BLIPS"`
- Row: `aria-label="Job {id} — {type} — {status} — dibuat {created_at}"`
- Tombol "Batalkan": `aria-label="Batalkan job {id}"`
- Tombol "Unduh": `aria-label="Unduh hasil job {id}"`
- Filter dropdown "Status": `aria-label="Filter status job"`
- Filter dropdown "Tipe": `aria-label="Filter tipe job"`
- Keyboard Tab: header filter → rows → action buttons → pagination controls; semua reachable

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-014 — M15-05-AC4: Job detail drawer /jobs/[jobId]

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/jobs`
2. Klik tombol "→" (Lihat detail) pada row JOB-00001
3. Perhatikan drawer atau navigasi ke `/jobs/JOB-00001`

**Hasil yang Diharapkan**:
- Drawer atau halaman detail terbuka: menampilkan JobProgressPanel + raw payload job
- JobProgressPanel: progress bar 47%; step; ETA; (cancel button jika masih running dan user = owner)
- Raw payload: `type=ECL_CALC_RUN`, `created_by`, timestamps

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

| Event | Trigger | Expected `aud.audit_log` |
|---|---|---|
| `JOB.CANCEL_REQUESTED` | `POST /api/v1/jobs/{jobId}/cancel` berhasil | `action='JOB.CANCEL_REQUESTED'`, `entity_id={jobId}` — in-transaction |
| `JOB.RESULT_DOWNLOADED` | Download result URL diklik | `action='JOB.RESULT_DOWNLOADED'` (via MinIO callback atau gateway log) |
| `EXPORT.GENERATED` | Export /jobs DataTable CSV/XLSX | `action='EXPORT.GENERATED'` dengan `after_jsonb.format`, `row_count`, `filters` |

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-05-AC2 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-05-AC2 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-05-AC2 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-05-AC2 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-05-AC2 | USR-IT-001 | ☐ Pass ☐ Fail |
| TC-006 | M15-05-AC2 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-05-AC2 | USR-IT-001 | ☐ Pass ☐ Fail |
| TC-008 | M15-05-AC3 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-009 | M15-05-AC3 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-010 | M15-05-AC3 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-011 | M15-05-AC3 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-012 | M15-05-AC3 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-013 | M15-05-AC4 | USR-AUDIT-001 | ☐ Pass ☐ Fail |
| TC-014 | M15-05-AC4 | USR-MAKER-001 | ☐ Pass ☐ Fail |

**Total: 14 TC covering AC M15-05-AC2, M15-05-AC3, M15-05-AC4 (/jobs page)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security — BLOCKING) | | | |
| Approver (IT-Admin/PM) | | | |
