# UAT-APP-E-001 — Reporting MV Foundation + Export Engine + Scheduled Email

**Modul**: APP-E  
**Story Set**: P5-M13 (S1–S5)  
**Tanggal UAT**: 2026-06-23  
**Penyusun**: qa-engineer  
**Gate**: security-engineer BLOCKING (audit in-tx, SHA-256, MinIO TTL, SMTP creds, export permission); ifrs9-compliance-reviewer ADVISORY (M13 = infra, ECL/EIR tidak disentuh)

---

## Pre-Kondisi Global

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. Migration 000050 ter-apply: 8 MV exist (`rpt.mv_*`), tabel `sys.mv_refresh_log`, `sys.export_log`, `sys.scheduled_email`, `sys.scheduled_email_optout`
3. Source tables ter-populate: `mst.periode_buku` PRD-2026-06 `OPEN`, `jrnl.jurnal_header` ≥ 1 row, `trx.akrual` ≥ 100 rows
4. MinIO bucket `exports` exist dengan lifecycle `Expiration: 1 day`
5. SMTP config valid (staging SMTP di env `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`)
6. `MV_DSN` di-set ke read-replica DSN
7. User test sesuai tabel di bawah:

| User ID | Role | MFA |
|---|---|---|
| USR-IT-001 | ROLE-IT-ADMIN | Ya |
| USR-AKUN-001 | ROLE-AKUN | Tidak |
| USR-CTL-001 | ROLE-AKUN-CTL | Ya |
| USR-CFO-001 | ROLE-CFO | Ya |
| USR-AUDIT-001 | ROLE-AUDIT | Tidak |
| USR-RISK-001 | ROLE-RISK | Tidak |

---

## TC-001 — S1-AC1: 8 MV Exist + Unique Index

**Actor**: USR-IT-001  
**Pre-kondisi**: Migration 000050 applied

**Langkah**:
1. Buka `/admin/mv-refresh`
2. Verifikasi 8 kartu MV tampil
3. Lihat DB: `SELECT matviewname FROM pg_matviews WHERE schemaname = 'rpt'`

**Hasil yang Diharapkan**:
- Halaman menampilkan 8 kartu MV (status badges muncul per kartu)
- `pg_matviews` mengembalikan tepat 8 baris: `mv_status_periode`, `mv_jurnal_summary`, `mv_gl_delivery_status`, `mv_mtm_daily_summary`, `mv_akrual_summary`, `mv_renewal_summary`, `mv_penjualan_summary`, `mv_poci_delta_summary`
- `pg_indexes` untuk setiap MV: `unique = true` pada index yang terdaftar

---

## TC-002 — S1-AC2: Read-Replica Routing Aktif

**Actor**: System (backend log)  
**Pre-kondisi**: `MV_DSN` ter-set

**Langkah**:
1. Akses `GET /api/v1/admin/mv-status`
2. Periksa backend log

**Hasil yang Diharapkan**:
- HTTP 200 dengan data 8 MV
- Backend log mengandung: `"MV query routed to read-replica DSN"`
- Primary DB tidak menerima SELECT query ke `rpt.mv_*`

---

## TC-003 — S1-AC3: Fallback ke Primary Jika MV_DSN Kosong

**Actor**: System (log)  
**Pre-kondisi**: `MV_DSN` di-unset sementara

**Langkah**:
1. Unset `MV_DSN` di env
2. Akses `GET /api/v1/admin/mv-status`
3. Periksa backend log

**Hasil yang Diharapkan**:
- HTTP 200 tetap ter-return (data dari primary)
- Log level=WARN: `"MV_DSN not set — falling back to primary DSN"`
- Tidak ada panic atau error 500

---

## TC-004 — S2-AC1: Cron Refresh Semua 8 MV

**Actor**: System (Asynq cron)  
**Pre-kondisi**: Asynq + Redis running

**Langkah**:
1. Trigger cron secara manual (via Asynq CLI atau tunggu 01:00 WIB)
2. Periksa `sys.mv_refresh_log`
3. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- 8 rows baru di `sys.mv_refresh_log`: `status='COMPLETED'`, `triggered_by='CRON'`, `trigger_actor IS NULL`
- 8 rows di `aud.audit_log`: `action='REPORT.MV_REFRESH'` — 1 per MV, in-transaction
- Prometheus metric `blips_mv_refresh_duration_ms` ter-update

---

## TC-005 — S2-AC3: Advisory Lock — Refresh Duplikat Ditolak

**Actor**: USR-IT-001  
**Pre-kondisi**: `sys.mv_refresh_log` punya 1 row `status='RUNNING'` untuk `rpt.mv_jurnal_summary`

**Langkah**:
1. POST `/api/v1/admin/mv-refresh` body: `{ "mvName": "rpt.mv_jurnal_summary" }`
   Header: `Idempotency-Key: IK-REFRESH-001`

**Hasil yang Diharapkan**:
- HTTP 423:
  - `error.code = MV_REFRESH_LOCKED`
  - `error.message` mengandung "sedang berjalan"
- Tidak ada row baru di `sys.mv_refresh_log`
- Toast error persistent di UI (tidak auto-dismiss)

---

## TC-006 — S2-AC4: Refresh Gagal → DLQ + Status FAILED

**Actor**: System  
**Pre-kondisi**: DB lock timeout disimulasikan

**Langkah**:
1. Simulasikan DB lock timeout pada refresh `rpt.mv_poci_delta_summary`
2. Periksa `sys.mv_refresh_log`
3. Periksa Asynq DLQ
4. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- `sys.mv_refresh_log`: `status='FAILED'`, `error_detail` mengandung error message
- Asynq DLQ: job ada dengan payload `{ mv_name, error, retry_count }`
- `aud.audit_log`: `action='REPORT.MV_REFRESH_FAILED'` — in-transaction
- Alert Grafana/Loki ter-trigger

---

## TC-007 — S3-AC1: Export XLSX Inline (≤10k) + Watermark + SHA-256

**Actor**: USR-AKUN-001  
**Pre-kondisi**: `rpt.mv_jurnal_summary` ter-refresh, 450 rows

**Langkah**:
1. `GET /api/v1/reports/mv-jurnal-summary/export?format=xlsx`
   Header: `Idempotency-Key: IK-EXPORT-001`
2. Unduh file, buka di Excel
3. Periksa `sys.export_log`
4. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- HTTP 200 streaming XLSX
- `Content-Disposition: attachment; filename="jurnal-summary-20260623.xlsx"`
- Sheet "Data": 450 baris data
- Baris terakhir footer: `"RAHASIA - BLIPS Tugu Re — exported ... by USR-AKUN-001"`
- `sys.export_log`: row INSERT dengan `format='xlsx'`, `row_count=450`, `sha256_hash` ter-isi
- `aud.audit_log`: `action='EXPORT.GENERATED'` — in-transaction

---

## TC-008 — S3-AC3: Format Tidak Didukung

**Actor**: USR-AKUN-001

**Langkah**:
1. `GET /api/v1/reports/mv-jurnal-summary/export?format=xml`

**Hasil yang Diharapkan**:
- HTTP 400:
  - `error.code = EXPORT_FORMAT_UNSUPPORTED`
  - `error.message` mengandung "csv, xlsx, pdf"
- Tidak ada file yang di-generate
- Tidak ada INSERT ke `sys.export_log`

---

## TC-009 — S3-AC4: Permission Check + ROLE-AUDIT Bypass

**Actor**: USR-RISK-001 (tanpa permission `report.renewal_summary.export`), USR-AUDIT-001

**Langkah**:
1. USR-RISK-001: `GET /api/v1/reports/mv-renewal-summary/export?format=csv`
2. USR-AUDIT-001: `GET /api/v1/reports/mv-renewal-summary/export?format=csv`

**Hasil yang Diharapkan**:
- USR-RISK-001 → HTTP 403: `error.code = EXPORT_PERMISSION_DENIED`
- USR-AUDIT-001 → HTTP 200 (bypass semua, karena `audit_log.read`)
- Toast error persistent untuk USR-RISK-001

---

## TC-010 — S4-AC1: Async Export >10k + MinIO + SMTP

**Actor**: USR-AKUN-001  
**Pre-kondisi**: `rpt.mv_akrual_summary`: 45.000 rows

**Langkah**:
1. `GET /api/v1/reports/mv-akrual-summary/export?format=xlsx`
   Header: `Idempotency-Key: IK-ASYNC-001`
2. Lihat `JobProgressPanel` di UI
3. Tunggu email notifikasi

**Hasil yang Diharapkan**:
- HTTP 202: `{ jobId, statusUrl, streamUrl }`
- `sys.job`: INSERT dengan `type='EXPORT'`, `status='queued'`
- Worker stream ke MinIO: `exports/TUGURE/USR-AKUN-001/2026/06/23/{jobId}.xlsx`
- `sys.export_log`: `sha256_hash` ter-isi, `signed_url_ttl_24h` ter-isi
- SMTP email dikirim ke USR-AKUN-001: subject mengandung "Export akrual_summary siap diunduh"
- Toast sukses UI: "Export selesai. 45.000 baris siap diunduh (TTL 24 jam)."
- `aud.audit_log`: `action='EXPORT.GENERATED'` — in-transaction

---

## TC-011 — S4-AC3: Dataset >100k → EXPORT_TOO_LARGE

**Actor**: USR-AKUN-001

**Langkah**:
1. `GET /api/v1/reports/mv-akrual-summary/export?format=xlsx` (row count override ke 120k via test env)

**Hasil yang Diharapkan**:
- HTTP 422: `error.code = EXPORT_TOO_LARGE`
- Error message: "120.000 rows melebihi batas 100.000 rows"
- Tidak ada Asynq job di-enqueue; tidak ada `sys.job` INSERT
- Toast error persistent

---

## TC-012 — S4-AC4: Download Audit EXPORT.DOWNLOADED

**Actor**: USR-AKUN-001  
**Pre-kondisi**: Job async selesai; signed URL aktif

**Langkah**:
1. Klik link signed URL dari email
2. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- File diunduh
- `aud.audit_log`: `action='EXPORT.DOWNLOADED'` — in-transaction
  - `after_jsonb`: `{ minio_path, user_id: USR-AKUN-001, downloaded_at }`

---

## TC-013 — S5-AC1: Buat Jadwal Email

**Actor**: USR-CTL-001  
**Pre-kondisi**: ROLE-AKUN-CTL, permission `report.scheduled_email.create`

**Langkah**:
1. Buka `/admin/scheduled-emails`
2. Klik "Tambah Jadwal"
3. Isi: laporan=mv-jurnal-summary, format=xlsx, frekuensi=daily, waktu=07:00+07:00, penerima=cfo@tugu-re.com,risk@tugu-re.com,akun@tugu-re.com
4. Submit

**Hasil yang Diharapkan**:
- HTTP 201: `data.id` ter-return
- `sys.scheduled_email`: INSERT dengan `active=true`, `recipients_jsonb` berisi 3 email
- `aud.audit_log`: `action='SCHEDULED_EMAIL.CREATED'` — in-transaction
  - `after_jsonb.recipients_count = 3`
- Toast sukses: "Jadwal email mv-jurnal-summary (daily 07:00) berhasil dibuat. 3 penerima aktif."

---

## TC-014 — S5-AC2: Cron Eksekusi → SMTP Send + Audit SENT

**Actor**: System (Asynq cron)

**Langkah**:
1. Trigger cron SCHED-001 secara manual (Asynq CLI)
2. Periksa inbox cfo@, risk@, akun@
3. Periksa `sys.scheduled_email`
4. Periksa `aud.audit_log`

**Hasil yang Diharapkan**:
- 3 email diterima: subject "Laporan Jurnal Summary BLIPS — 2026-06-23"
- Email body Bahasa Indonesia; SHA-256 hash tercantum; attachment XLSX dengan watermark
- `sys.scheduled_email`: `last_sent_at` ter-update, `last_status='SENT'`
- `aud.audit_log`: `action='SCHEDULED_EMAIL.SENT'` — `recipient_count=3`

---

## TC-015 — S5-AC3: SMTP Gagal 3x → DLQ

**Actor**: System  
**Pre-kondisi**: SMTP server dimatikan sementara

**Langkah**:
1. Matikan SMTP server di env UAT
2. Trigger cron SCHED-001

**Hasil yang Diharapkan**:
- 3 percobaan SMTP (backoff 30s, 60s)
- Setelah percobaan ke-3: Asynq DLQ menerima job
- `sys.scheduled_email`: `last_status='FAILED'`, `error_detail` mengandung "SMTP"
- Alert Grafana/Loki: `[BLIPS-ALERT] Scheduled email SCHED-001 failed after 3 retries`
- Error code: `SCHEDULED_EMAIL_SMTP_FAILED`

---

## TC-016 — S5-AC4: Opt-Out Penerima

**Actor**: risk@tugu-re.com (klik link opt-out di email)

**Langkah**:
1. Buka link opt-out dari email SCHED-001 (format: `/api/v1/reports/scheduled-emails/{id}/opt-out?token=...&email=risk@tugu-re.com`)
2. Trigger cron SCHED-001 berikutnya

**Hasil yang Diharapkan**:
- Halaman opt-out: pesan "Opt-out Berhasil"
- `sys.scheduled_email_optout`: row INSERT dengan email=risk@tugu-re.com
- Cron berikutnya: email hanya ke 2 penerima (bukan 3)
- `aud.audit_log`: `action='SCHEDULED_EMAIL.SENT'`, `recipient_count=2`
- `sys.scheduled_email.recipients_jsonb` tidak berubah (3 email tetap ada)

---

## TC-017 — Non-IT-ADMIN Tidak Melihat Refresh Button

**Actor**: USR-AKUN-001 (bukan ROLE-IT-ADMIN)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Buka `/admin/mv-refresh`

**Hasil yang Diharapkan**:
- Halaman ter-render (jika diizinkan akses — server bisa return 403)
- Jika halaman tampil: tombol "Refresh" TIDAK ADA di DOM (inspect element: tidak ada elemen button dengan teks "Refresh")

---

## TC-018 — Non-AKUN-CTL Tidak Melihat Tombol Tambah Jadwal

**Actor**: USR-AKUN-001

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Buka `/admin/scheduled-emails`

**Hasil yang Diharapkan**:
- Tombol "Tambah Jadwal" TIDAK ADA di DOM
- Kolom delete di tabel TIDAK ADA

---

## TC-019 — Export History Table Sort + Filter + Download

**Actor**: USR-AKUN-001

**Langkah**:
1. Buka `/admin/exports`
2. Filter status=COMPLETED
3. Klik header "Diminta Pada" untuk sort descending
4. Klik tombol "Unduh" pada export pertama

**Hasil yang Diharapkan**:
- Filter chip "Status: Selesai" muncul; tabel menampilkan hanya COMPLETED rows
- Header kolom "Diminta Pada" menampilkan ikon sort ↓
- Klik "Unduh" → browser redirect ke signed MinIO URL → file diunduh
- `aud.audit_log`: `action='EXPORT.DOWNLOADED'` ter-tulis

---

## TC-020 — MV Status Badge WCAG AA

**Actor**: QA (visual check)

**Langkah**:
1. Buka `/admin/mv-refresh`
2. Inspect elemen badge IDLE, REFRESHING, FAILED dengan color contrast analyzer

**Hasil yang Diharapkan**:
- Badge IDLE (green): contrast ratio ≥ 4.5:1 antara text dan background
- Badge REFRESHING (amber): contrast ratio ≥ 4.5:1
- Badge FAILED (red): contrast ratio ≥ 4.5:1
- Badge memiliki `role="status"` dan `aria-label` yang deskriptif

---

## Ringkasan TC

| TC | Story | AC | Actor Utama | Gate |
|---|---|---|---|---|
| TC-001 | S1 | S1-AC1 | IT-ADMIN | security-engineer |
| TC-002 | S1 | S1-AC2 | System | security-engineer |
| TC-003 | S1 | S1-AC3 | System | — |
| TC-004 | S2 | S2-AC1 | System | security-engineer |
| TC-005 | S2 | S2-AC3 | IT-ADMIN | — |
| TC-006 | S2 | S2-AC4 | System | security-engineer |
| TC-007 | S3 | S3-AC1 | ROLE-AKUN | security-engineer |
| TC-008 | S3 | S3-AC3 | ROLE-AKUN | — |
| TC-009 | S3 | S3-AC4 | ROLE-RISK, ROLE-AUDIT | security-engineer |
| TC-010 | S4 | S4-AC1 | ROLE-AKUN | security-engineer |
| TC-011 | S4 | S4-AC3 | ROLE-AKUN | — |
| TC-012 | S4 | S4-AC4 | ROLE-AKUN | security-engineer |
| TC-013 | S5 | S5-AC1 | ROLE-AKUN-CTL | security-engineer |
| TC-014 | S5 | S5-AC2 | System | security-engineer |
| TC-015 | S5 | S5-AC3 | System | — |
| TC-016 | S5 | S5-AC4 | Recipient | security-engineer |
| TC-017 | Cross | persona-gate | ROLE-AKUN | — |
| TC-018 | Cross | persona-gate | ROLE-AKUN | — |
| TC-019 | Cross | UX §1 | ROLE-AKUN | — |
| TC-020 | Cross | WCAG AA | QA | — |

**Total: 20 TC covering all 20 AC (S1-AC1..S1-AC4, S2-AC1..S2-AC4, S3-AC1..S3-AC4, S4-AC1..S4-AC4, S5-AC1..S5-AC4)**
