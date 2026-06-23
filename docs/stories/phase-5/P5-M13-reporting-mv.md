# P5-M13 — APP-E Reporting Foundation: Materialized Views + Export Engine + Scheduled Email: User Stories

**Story Set ID**: P5-M13
**Modul**: APP-E — Reporting & Dashboard
**Status**: DRAFT — menunggu handoff ke `system-analyst`; `security-engineer` (BLOCKING — audit in-tx, PII, export); `ifrs9-compliance-reviewer` advisory gate
**Author**: business-analyst
**Tanggal**: 2026-06-23
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §6 (APP-E Reporting), FSD-APP-E (Report Engine + MV Refresh + Export)
**Linked BRD**: BRD §4.5 (APP-E laporan), §3 RACI: ROLE-AKUN-CTL (A), ROLE-RISK (R), ROLE-CFO (A), ROLE-AUDIT (I), ROLE-IT-ADMIN (I config)
**Linked Decision Log**:
- `DEC-007` (LOCKED) — Asynq job queue; Redis-based; DLQ on failure
- `DEC-018` (LOCKED) — audit trail append-only, 10+10 tahun; in-transaction wajib
- `DEC-021` (LOCKED) — Idempotency-Key wajib di semua mutating endpoints
- `DEC-022` (LOCKED) — cursor-based pagination only

**Dependensi**:
- **P5-M3** — `jrnl.gl_delivery` + GL recon data (dipakai `mv_gl_delivery_status`)
- **P5-M4** — `mst.periode_buku` hard-close event memicu refresh semua MV (trigger dari worker)
- **P5-M9** — `trx.akrual` + `trx.jatuh_tempo` (dipakai `mv_akrual_summary`)
- **P5-M12** — `mst.mapping_jurnal_header` + `jrnl.*` (dipakai `mv_jurnal_summary`)
- Migration 000049 (P5-M12 terdahulu) harus sudah applied

**Gate**: `security-engineer` **BLOCKING** (audit in-tx, watermark, SHA-256, MinIO TTL, SMTP retry). `ifrs9-compliance-reviewer` **ADVISORY** (M13 = infra; ECL/EIR formula tidak disentuh di M13 — disentuh M14).

---

## Konteks & Arsitektur P5-M13

P5-M13 membangun **infrastruktur laporan** yang akan dipakai 28 laporan di P5-M14. M13 tidak menyentuh ECL/EIR formula — hanya foundation:

1. **8 Materialized Views** (`rpt.mv_*`) — pre-aggregate data dari 9 schema
2. **Asynq refresh worker** — CONCURRENT refresh (butuh unique idx per MV); cron + on-demand
3. **Export engine** — CSV/XLSX (`excelize`) / PDF (`gofpdf`); watermark RAHASIA + SHA-256
4. **Async export >10k rows** — Asynq job → MinIO → signed URL 24h → SMTP notif
5. **Scheduled email** — ROLE-AKUN-CTL konfigurasi per laporan; Asynq cron; opt-out

### 8 Materialized Views

| MV Name | Source schema | Unique Index (CONCURRENT) |
|---|---|---|
| `rpt.mv_status_periode` | `mst.periode_buku` | `(periode_id, tenant_id)` |
| `rpt.mv_jurnal_summary` | `jrnl.jurnal_header`, `jrnl.jurnal_detail` | `(periode_id, event_code, tenant_id)` |
| `rpt.mv_gl_delivery_status` | `jrnl.gl_delivery` | `(periode_id, delivery_id, tenant_id)` |
| `rpt.mv_mtm_daily_summary` | `trx.mtm_adjustment` | `(tanggal_mtm, instrumen_id, tenant_id)` |
| `rpt.mv_akrual_summary` | `trx.akrual`, `trx.jatuh_tempo` | `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_renewal_summary` | `trx.renewal` | `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_penjualan_summary` | `trx.penjualan_pencairan` | `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_poci_delta_summary` | `ecl.poci_delta_log` | `(periode_id, instrumen_id, tenant_id)` |

Semua query ke `rpt.mv_*` di-route ke **read-replica DSN** (`MV_DSN` env var). Jika `MV_DSN` tidak di-set, fallback ke primary DSN dengan warning log (bukan error).

### sys.mv_refresh_log (baru di migration 000050)

```sql
CREATE TABLE sys.mv_refresh_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mv_name         TEXT NOT NULL,
    triggered_by    TEXT NOT NULL,   -- 'CRON', 'HARD_CLOSE', 'MANUAL'
    trigger_actor   UUID,            -- null jika CRON
    status          TEXT NOT NULL,   -- RUNNING | COMPLETED | FAILED
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    error_detail    TEXT,
    -- audit cols standard
);
```

---

## Story P5-M13-S1 — 8 MV Foundation + Read-Replica Routing

**Actor**: System (migration), ROLE-IT-ADMIN (verify), ROLE-AUDIT (read-only)
**Trigger**: Migration 000050 applied — `CREATE MATERIALIZED VIEW rpt.mv_*` dengan unique index per MV (syarat CONCURRENT refresh). Backend membaca env `MV_DSN` untuk routing read-replica; fallback primary jika unset.
**Goal**: 8 MV exist dengan unique index. Query laporan di-route ke read-replica tanpa mengubah kontrak API.

### Pre-conditions
1. Migration 000049 (P5-M12) sudah applied
2. Schema `rpt` sudah exist (dibuat di migration awal P5-M2 atau earlier)
3. Source tables (`jrnl.*`, `trx.*`, `ecl.*`, `mst.periode_buku`) ter-populate minimal 1 row
4. PostgreSQL read-replica DSN tersedia di env `MV_DSN` (opsional; fallback ke `DATABASE_URL`)

### Acceptance Criteria

```gherkin
Feature: 8 MV Foundation dengan unique index + read-replica routing

  Background:
    Given migration 000050 telah di-apply di environment dev
    And env MV_DSN = "postgres://replica-host:5432/blips"

  Scenario: S1-AC1 — 8 MV exist + unique index per MV; CONCURRENT refresh tidak error
    When ROLE-IT-ADMIN menjalankan: SELECT matviewname FROM pg_matviews WHERE schemaname = 'rpt'
    Then result set mengandung tepat 8 MV:
      | mv_status_periode, mv_jurnal_summary, mv_gl_delivery_status, mv_mtm_daily_summary |
      | mv_akrual_summary, mv_renewal_summary, mv_penjualan_summary, mv_poci_delta_summary |
    And setiap MV punya unique index (pg_indexes): SELECT indexdef ... WHERE unique = true
    And REFRESH MATERIALIZED VIEW CONCURRENTLY rpt.mv_status_periode; — tidak error
    And migration 000050.down.sql: DROP MATERIALIZED VIEW ... berhasil dijalankan tanpa error

  Scenario: S1-AC2 — Read-replica routing aktif: query rpt.mv_* via MV_DSN, bukan primary
    When GET /api/v1/reports/rpt-mv-status-periode dipanggil
    Then backend log mengandung: "MV query routed to read-replica DSN"
    And response HTTP 200: data dari rpt.mv_status_periode
    And primary DSN tidak menerima query SELECT ke rpt.mv_*

  Scenario: S1-AC3 — Fallback ke primary jika MV_DSN tidak di-set; warning log muncul
    Given env MV_DSN = "" (tidak di-set)
    When GET /api/v1/reports/rpt-mv-status-periode dipanggil
    Then HTTP 200: data tetap ter-return (dari primary)
    And backend log level=WARN: "MV_DSN not set — falling back to primary DSN. Set MV_DSN for read-replica routing."
    And tidak ada error 500 atau panic

  Scenario: S1-AC4 — Refresh di periode HARD_CLOSED berjalan OK; audit dicatat
    Given mst.periode_buku PRD-2026-06: status_periode = 'HARD_CLOSED'
    When Asynq worker trigger refresh rpt.mv_status_periode (triggered_by='HARD_CLOSE')
    Then REFRESH MATERIALIZED VIEW CONCURRENTLY rpt.mv_status_periode; sukses
    And sys.mv_refresh_log INSERT: { mv_name: 'rpt.mv_status_periode', triggered_by: 'HARD_CLOSE', status: 'COMPLETED' }
    And aud.audit_log.action = 'REPORT.MV_REFRESH' — in-transaction
      With after_jsonb: { mv_name, triggered_by, duration_ms, tenant_id }
```

---

## Story P5-M13-S2 — Asynq Refresh Worker (Cron + On-Demand + DLQ)

**Actor**: System (cron), ROLE-IT-ADMIN (manual trigger + DLQ inspect), mst.periode_buku hard-close event (on-demand trigger)
**Trigger**: (a) Asynq cron setiap hari kerja jam 01:00 WIB (refresh semua 8 MV); (b) POST hard-close periode (P5-M4) meng-enqueue refresh job; (c) ROLE-IT-ADMIN POST `/api/v1/admin/mv-refresh` → manual trigger per MV atau semua. CONCURRENT refresh; jika refresh MV sedang berjalan → `MV_REFRESH_LOCKED` (tidak boleh refresh 2x bersamaan). Failure → DLQ, log `sys.mv_refresh_log.status = 'FAILED'`, alert.
**Goal**: MV selalu fresh setelah hard-close. Manual trigger tersedia tanpa downtime.

### Pre-conditions
1. Asynq + Redis 7+ running
2. 8 MV exist dengan unique index (S1 done)
3. `sys.mv_refresh_log` tabel exist (migration 000050)
4. ROLE-IT-ADMIN permission `sys.mv_refresh.trigger`

### Acceptance Criteria

```gherkin
Feature: Asynq MV refresh worker — cron + on-demand + DLQ

  Background:
    Given Asynq worker berjalan, Redis tersedia
    And sys.mv_refresh_log: tidak ada row status='RUNNING' untuk rpt.mv_jurnal_summary

  Scenario: S2-AC1 — Cron harian 01:00 WIB: semua 8 MV di-refresh; log dicatat per MV
    When Asynq cron 01:00 WIB trigger HandleMVRefreshAll
    Then 8 Asynq job di-enqueue (1 job per MV, tipe 'mv:refresh')
    And setiap job berhasil: REFRESH MATERIALIZED VIEW CONCURRENTLY rpt.mv_{name}
    And sys.mv_refresh_log: 8 rows INSERT dengan triggered_by='CRON', status='COMPLETED'
    And aud.audit_log: 8 rows action='REPORT.MV_REFRESH' — in-transaction per job
    And total durasi 8 MV refresh: tercatat di Prometheus metric blips_mv_refresh_duration_ms

  Scenario: S2-AC2 — On-demand dari hard-close periode: refresh di-enqueue setelah hard-close commit
    Given POST /api/v1/periode/PRD-2026-06/hardclose sukses (P5-M4 flow)
    When hard-close handler enqueue job 'mv:refresh_all' dengan triggered_by='HARD_CLOSE', periode_id='PRD-2026-06'
    Then job sukses di-eksekusi oleh worker: 8 MV di-refresh CONCURRENTLY
    And sys.mv_refresh_log: triggered_by='HARD_CLOSE', trigger_actor=CFO_USER_ID

  Scenario: S2-AC3 — MV_REFRESH_LOCKED: concurrent refresh yang sama ditolak
    Given sys.mv_refresh_log: 1 row status='RUNNING', mv_name='rpt.mv_jurnal_summary', started_at=now()-30s
    When ROLE-IT-ADMIN POST /api/v1/admin/mv-refresh { mv_name: "rpt.mv_jurnal_summary" }
      With Idempotency-Key: IK-REFRESH-001
    Then HTTP 423:
      | error.code    | MV_REFRESH_LOCKED                                                              |
      | error.message | "Refresh rpt.mv_jurnal_summary sedang berjalan (started 30 detik lalu). Coba lagi setelah selesai." |
    And sys.mv_refresh_log: tidak ada row baru INSERT (request ditolak)

  Scenario: S2-AC4 — Refresh gagal (DB error): status FAILED + DLQ + audit
    Given REFRESH MATERIALIZED VIEW CONCURRENTLY rpt.mv_poci_delta_summary gagal karena lock timeout
    When Asynq worker HandleMVRefresh menangkap error
    Then sys.mv_refresh_log: status='FAILED', error_detail='lock timeout after 30s'
    And Asynq DLQ: job dipindah ke DLQ dengan payload { mv_name, error, retry_count }
    And aud.audit_log.action = 'REPORT.MV_REFRESH_FAILED' — in-transaction
      With after_jsonb: { mv_name, error_detail, triggered_by }
    And alert dikirim (Grafana/Loki): [BLIPS-ALERT] MV refresh failed: rpt.mv_poci_delta_summary
    And error.code = MV_REFRESH_FAILED (jika ROLE-IT-ADMIN poll status via GET /api/v1/jobs/{jobId})
```

---

## Story P5-M13-S3 — Export Engine (CSV/XLSX/PDF + Watermark + SHA-256)

**Actor**: ROLE-RISK, ROLE-AKUN, ROLE-CFO, ROLE-AUDIT (per permission `report.{slug}.export`), System (engine)
**Trigger**: `GET /api/v1/reports/{slug}/export?format=csv|xlsx|pdf` — backend memanggil export engine. Engine: (a) query MV via read-replica; (b) stream ke format yang diminta via `excelize` (XLSX) / `gofpdf` (PDF) / UTF-8 BOM (CSV); (c) watermark setiap halaman/sheet footer; (d) SHA-256 hash file content → simpan di `sys.export_log`; (e) audit `EXPORT.GENERATED` in-transaction.
**Goal**: Setiap format export punya watermark identitas user + timestamp + SHA-256 integrity seal. Format unsupported → `EXPORT_FORMAT_UNSUPPORTED`. Permission tidak ada → `EXPORT_PERMISSION_DENIED`.

### Pre-conditions
1. User ter-autentikasi dengan permission `report.{slug}.export`
2. MV ter-refresh (data exist di `rpt.mv_*`)
3. `sys.export_log` tabel exist (migration 000050)
4. Dataset ≤ 10k rows (> 10k rows → async job, story S4)

### Acceptance Criteria

```gherkin
Feature: Export engine — CSV/XLSX/PDF + watermark + SHA-256 + audit

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'report.jurnal_summary.export'
    And rpt.mv_jurnal_summary ter-refresh: 450 rows

  Scenario: S3-AC1 — Export XLSX: watermark di footer setiap sheet + SHA-256 dicatat
    When USR-AKUN-001 GET /api/v1/reports/mv-jurnal-summary/export?format=xlsx
    Then HTTP 200 streaming XLSX:
      | Content-Disposition | attachment; filename="jurnal-summary-20260623.xlsx" |
      | Sheet "Data"        | 450 rows data dari rpt.mv_jurnal_summary            |
      | Footer baris terakhir | "RAHASIA - BLIPS Tugu Re — exported 2026-06-23T10:30:00+07:00 by USR-AKUN-001" |
    And SHA-256 dari file bytes dihitung backend
    And sys.export_log INSERT: { user_id, report_slug, format:'xlsx', row_count:450, file_hash_sha256:..., exported_at }
    And aud.audit_log.action = 'EXPORT.GENERATED' — in-transaction
      With after_jsonb: { report_slug, format, row_count, file_hash_sha256, actor }

  Scenario: S3-AC2 — Export PDF: watermark setiap halaman; gofpdf; SHA-256
    When USR-AKUN-001 GET /api/v1/reports/mv-status-periode/export?format=pdf
    Then HTTP 200 streaming PDF:
      | Content-Type        | application/pdf                                             |
      | Setiap halaman PDF  | footer watermark: "RAHASIA - BLIPS Tugu Re — exported {timestamp} by {user}" |
      | Halaman terakhir    | "SHA-256: {hex hash}" |
    And sys.export_log INSERT: { format:'pdf', file_hash_sha256 }
    And aud.audit_log.action = 'EXPORT.GENERATED' — in-transaction

  Scenario: S3-AC3 — Format tidak didukung: EXPORT_FORMAT_UNSUPPORTED
    When USR-AKUN-001 GET /api/v1/reports/mv-jurnal-summary/export?format=xml
    Then HTTP 400:
      | error.code    | EXPORT_FORMAT_UNSUPPORTED                                       |
      | error.message | "Format 'xml' tidak didukung. Format tersedia: csv, xlsx, pdf." |
    And tidak ada file yang di-generate; tidak ada sys.export_log INSERT

  Scenario: S3-AC4 — Permission tidak ada: EXPORT_PERMISSION_DENIED; ROLE-AUDIT bypass semua
    Given USR-RISK-002 tidak punya permission 'report.renewal_summary.export'
    When USR-RISK-002 GET /api/v1/reports/mv-renewal-summary/export?format=csv
    Then HTTP 403:
      | error.code    | EXPORT_PERMISSION_DENIED                                                        |
      | error.message | "Tidak punya permission 'report.renewal_summary.export'. Hubungi ROLE-IT-ADMIN." |
    And ROLE-AUDIT USR-AUDIT-001: GET /api/v1/reports/mv-renewal-summary/export?format=csv → HTTP 200
    And ROLE-AUDIT bypass: permission 'audit_log.read' mencakup semua report export (read-all bypass per personas.md)
```

---

## Story P5-M13-S4 — Async Export >10k Rows (Asynq + MinIO + SMTP Notif)

**Actor**: ROLE-RISK, ROLE-AKUN, ROLE-CFO, ROLE-AUDIT, System (Asynq worker, SMTP)
**Trigger**: `GET /api/v1/reports/{slug}/export?format=xlsx` di mana row count > 10.000 — backend return 202 `{ jobId, statusUrl, streamUrl }` (bukan stream langsung). Asynq worker stream rows ke MinIO bucket `exports/{tenant}/{user}/{yyyy/mm/dd}/{jobId}.xlsx`. Setelah selesai: signed URL TTL 24h + SMTP notif email user. MinIO auto-delete setelah 24h (bucket lifecycle policy). Dataset > 100k rows → `EXPORT_TOO_LARGE`.
**Goal**: Export besar tidak memblok server thread. User lanjut kerja, notif email saat selesai. File di-hash SHA-256 sebelum upload ke MinIO.

### Pre-conditions
1. MinIO bucket `exports` exist dengan lifecycle policy `Expiration: 1 day`
2. SMTP config tersedia di env (`SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`)
3. `sys.job` tabel exist (migration 000050); SSE endpoint `/api/v1/jobs/{jobId}/stream` tersedia
4. Permission `report.{slug}.export`

### Acceptance Criteria

```gherkin
Feature: Async export >10k rows — Asynq + MinIO + SMTP

  Background:
    Given rpt.mv_akrual_summary: 45.000 rows
    And ROLE-AKUN USR-AKUN-001 dengan permission 'report.akrual_summary.export'
    And MinIO bucket 'exports' lifecycle: Expiration 1 day
    And SMTP config valid

  Scenario: S4-AC1 — Export 45k rows: 202 return; Asynq job stream ke MinIO; signed URL notif
    When USR-AKUN-001 GET /api/v1/reports/mv-akrual-summary/export?format=xlsx
      With Idempotency-Key: IK-EXPORT-001
    Then HTTP 202:
      | data.jobId     | JOB-EXP-001                                   |
      | data.statusUrl | /api/v1/jobs/JOB-EXP-001                      |
      | data.streamUrl | /api/v1/jobs/JOB-EXP-001/stream              |
    And sys.job INSERT: { type:'EXPORT', status:'queued', created_by:USR-AKUN-001 }
    When Asynq worker HandleExportJob berhasil
    Then MinIO object: exports/TUGURE/USR-AKUN-001/2026/06/23/JOB-EXP-001.xlsx exists
    And sys.export_log INSERT: { file_hash_sha256: SHA-256(file bytes), minio_path, signed_url_ttl_24h }
    And sys.job UPDATE: status='completed', result_jsonb: { signed_url, row_count:45000, file_hash_sha256 }
    And aud.audit_log.action = 'EXPORT.GENERATED' — in-transaction (saat job complete)
    And SMTP email dikirim ke user email USR-AKUN-001:
      | Subject | "[BLIPS] Export akrual_summary siap diunduh" |
      | Body    | Bahasa Indonesia; link signed URL; TTL 24 jam; SHA-256 hash untuk verifikasi |

  Scenario: S4-AC2 — SSE progress: frontend <JobProgressPanel> menerima update per progress event
    Given JOB-EXP-001 status='running', progress=47
    When frontend GET /api/v1/jobs/JOB-EXP-001/stream (SSE)
    Then SSE event 'progress': { progress: 47, currentStep: "Streaming baris 21.150 dari 45.000" }
    And SSE event 'completed' saat selesai: { result: { signed_url, row_count, file_hash_sha256 } }
    And toast sukses frontend: "Export akrual_summary selesai. 45.000 baris siap diunduh (TTL 24 jam)."
      With action link: "Unduh sekarang →" (signed URL)

  Scenario: S4-AC3 — Dataset >100k rows: EXPORT_TOO_LARGE ditolak sebelum job dibuat
    Given rpt.mv_akrual_summary: 120.000 rows (row count override untuk test)
    When USR-AKUN-001 GET /api/v1/reports/mv-akrual-summary/export?format=xlsx
    Then HTTP 422:
      | error.code    | EXPORT_TOO_LARGE                                                                    |
      | error.message | "Dataset 120.000 rows melebihi batas 100.000 rows per export. Gunakan filter untuk mempersempit data." |
    And tidak ada Asynq job yang di-enqueue; tidak ada sys.job INSERT

  Scenario: S4-AC4 — Download audit: EXPORT.DOWNLOADED dicatat saat user klik signed URL
    Given JOB-EXP-001 completed; signed URL aktif
    When USR-AKUN-001 GET signed URL (via MinIO presigned endpoint)
    Then aud.audit_log.action = 'EXPORT.DOWNLOADED' — in-transaction (via MinIO webhook atau backend redirect)
      With after_jsonb: { minio_path, user_id: USR-AKUN-001, downloaded_at }
    And setelah 24 jam: MinIO object auto-delete per bucket lifecycle; signed URL return 404
```

---

## Story P5-M13-S5 — Scheduled Email Config + SMTP (ROLE-AKUN-CTL)

**Actor**: ROLE-AKUN-CTL (konfigurasi jadwal per laporan), System (Asynq cron SMTP sender)
**Trigger**: ROLE-AKUN-CTL POST `/api/v1/reports/scheduled-email` — konfigurasi email terjadwal per laporan: frekuensi (daily/weekly/monthly), format, daftar penerima, active flag. Asynq cron menjalankan pengiriman sesuai jadwal — generate export (via S3/S4 engine) → attach ke email → SMTP send. Retry 3x backoff; gagal setelah 3x → DLQ + alert. Penerima dapat opt-out via `sys.scheduled_email_optout`. Template email Bahasa Indonesia.
**Goal**: Distribusi laporan otomatis tanpa aksi manual harian. SoD: ROLE-AKUN-CTL yang config, bukan ROLE-AKUN.

### Pre-conditions
1. ROLE-AKUN-CTL dengan permission `report.scheduled_email.create`
2. SMTP config valid (`SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, `SMTP_PASSWORD`)
3. `sys.scheduled_email` + `sys.scheduled_email_optout` tabel exist (migration 000050)
4. Export engine (S3) dan async export (S4) sudah berjalan

### Acceptance Criteria

```gherkin
Feature: Scheduled email config + SMTP dengan Asynq cron

  Background:
    Given ROLE-AKUN-CTL USR-CTL-001 ter-autentikasi dengan permission 'report.scheduled_email.create'
    And SMTP config valid

  Scenario: S5-AC1 — Create scheduled email config: daily XLSX jurnal_summary ke 3 penerima
    When USR-CTL-001 POST /api/v1/reports/scheduled-email
      With Idempotency-Key: IK-SCHED-001
      With body:
        {
          report_slug: "mv-jurnal-summary",
          format: "xlsx",
          frequency: "daily",
          send_time: "07:00+07:00",
          recipients: ["cfo@tugu-re.com", "risk@tugu-re.com", "akun@tugu-re.com"],
          active: true,
          subject_template: "Laporan Jurnal Summary BLIPS — {tanggal}",
          body_template: "Terlampir laporan jurnal summary BLIPS per {tanggal}. File dapat diverifikasi dengan SHA-256: {file_hash}."
        }
    Then HTTP 201:
      | data.id          | SCHED-001                          |
      | data.report_slug | mv-jurnal-summary                  |
      | data.frequency   | daily                              |
      | data.active      | true                               |
    And sys.scheduled_email INSERT: { report_slug, format, frequency, send_time, recipients_jsonb, active }
    And aud.audit_log.action = 'SCHEDULED_EMAIL.CREATED' — in-transaction
      With after_jsonb: { sched_id, report_slug, recipients_count:3, actor:USR-CTL-001 }
    And toast: "Jadwal email mv-jurnal-summary (daily 07:00) berhasil dibuat. 3 penerima aktif."

  Scenario: S5-AC2 — Asynq cron eksekusi: generate export → attach → SMTP send; audit SENT
    Given SCHED-001: active=true, frequency='daily', send_time='07:00', report_slug='mv-jurnal-summary'
    When Asynq cron HandleScheduledEmailSend eksekusi SCHED-001
    Then export engine generate XLSX (rpt.mv_jurnal_summary, format xlsx, ≤10k → sync atau >10k → via S4)
    And email dikirim via SMTP ke 3 recipients:
      | Subject | "Laporan Jurnal Summary BLIPS — 2026-06-23"                        |
      | Body    | Bahasa Indonesia; SHA-256 hash tercantum; watermark info           |
      | Attachment | file XLSX dengan watermark footer                               |
    And sys.scheduled_email: last_sent_at = now(), last_status = 'SENT'
    And aud.audit_log.action = 'SCHEDULED_EMAIL.SENT' — in-transaction
      With after_jsonb: { sched_id, recipient_count:3, file_hash_sha256, sent_at }

  Scenario: S5-AC3 — SMTP gagal: retry 3x backoff; setelah 3x → DLQ + alert
    Given SMTP server tidak tersedia saat cron eksekusi SCHED-001
    When HandleScheduledEmailSend attempt 1 → SMTP error; attempt 2 (backoff 30s) → error; attempt 3 (backoff 60s) → error
    Then Asynq DLQ: job SCHED-001 dipindah ke DLQ dengan { error: "SMTP connection refused", retry_count: 3 }
    And sys.scheduled_email: last_status = 'FAILED', error_detail = 'SCHEDULED_EMAIL_SMTP_FAILED after 3 retries'
    And error.code = SCHEDULED_EMAIL_SMTP_FAILED (queryable via GET /api/v1/jobs/{jobId})
    And alert (Grafana/Loki): [BLIPS-ALERT] Scheduled email SCHED-001 failed after 3 retries

  Scenario: S5-AC4 — Opt-out penerima: recipient di-optout tidak menerima email; config tidak dihapus
    Given sys.scheduled_email_optout: { sched_id: SCHED-001, email: 'risk@tugu-re.com', opted_out_at: now() }
    When Asynq cron eksekusi SCHED-001 (berikutnya)
    Then SMTP send hanya ke 2 recipients (bukan 3): cfo@tugu-re.com + akun@tugu-re.com
    And risk@tugu-re.com tidak menerima email
    And aud.audit_log.action = 'SCHEDULED_EMAIL.SENT' after_jsonb.recipient_count = 2
    And sys.scheduled_email.recipients_jsonb tidak berubah (opt-out disimpan terpisah di optout table)
```

---

## Ringkasan P5-M13 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M13-S1 | 8 MV Foundation + Read-Replica Routing | System, ROLE-IT-ADMIN | 4 | **security-engineer BLOCKING** (audit in-tx) |
| P5-M13-S2 | Asynq Refresh Worker + DLQ | System, ROLE-IT-ADMIN | 4 | **security-engineer BLOCKING** (audit in-tx, DLQ) |
| P5-M13-S3 | Export Engine CSV/XLSX/PDF + Watermark + SHA-256 | ROLE-RISK/AKUN/CFO/AUDIT | 4 | **security-engineer BLOCKING** (audit in-tx, SHA-256, permission) |
| P5-M13-S4 | Async Export >10k + MinIO + SMTP Notif | ROLE-RISK/AKUN/CFO/AUDIT, System | 4 | **security-engineer BLOCKING** (MinIO TTL, download audit, EXPORT_TOO_LARGE) |
| P5-M13-S5 | Scheduled Email Config + SMTP + Opt-out | ROLE-AKUN-CTL, System | 4 | **security-engineer BLOCKING** (SMTP creds, audit SENT, DLQ) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger |
|---|---|---|
| `EXPORT_TOO_LARGE` | 422 | Dataset > 100.000 rows — tolak sebelum job dibuat |
| `EXPORT_PERMISSION_DENIED` | 403 | User tidak punya `report.{slug}.export`; bukan ROLE-AUDIT |
| `MV_REFRESH_LOCKED` | 423 | Concurrent refresh untuk MV yang sama sedang berjalan |
| `MV_REFRESH_FAILED` | 500 | Worker refresh MV gagal (DB error, lock timeout); lihat DLQ |
| `SCHEDULED_EMAIL_SMTP_FAILED` | 500 | SMTP send gagal setelah 3x retry; job di-DLQ |
| `EXPORT_FORMAT_UNSUPPORTED` | 400 | format param bukan csv/xlsx/pdf |

Catatan: `FORBIDDEN` (403), `WORKFLOW_INVALID_TRANSITION` (422), `IDEMPOTENCY_REPLAY` (200) sudah ada di api-conventions.md.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M13 | MFA Level |
|---|---|---|---|
| ROLE-IT-ADMIN | `sys.mv_refresh.trigger` | Manual trigger refresh (S2), DLQ inspect | WAJIB (DEC-026) |
| ROLE-AKUN-CTL | `report.scheduled_email.create` | Config jadwal email (S5), view status | WAJIB (DEC-026) |
| ROLE-RISK | `report.{slug}.export` | Export laporan (S3/S4) | Tidak wajib |
| ROLE-AKUN | `report.{slug}.export` | Export laporan (S3/S4) | Tidak wajib |
| ROLE-CFO | `report.{slug}.export` | Export laporan (S3/S4) | WAJIB (DEC-026) |
| ROLE-AUDIT | `audit_log.read` (bypass semua report) | Export semua laporan (S3/S4), read EXPORT.DOWNLOADED | Tidak wajib |
| System (cron) | — | Asynq cron refresh (S2), Asynq cron email (S5) | N/A |

---

## Audit Events Summary

| Event | Trigger | In-transaction |
|---|---|---|
| `REPORT.MV_REFRESH` | Setiap CONCURRENT refresh berhasil (S1, S2) | Ya |
| `REPORT.MV_REFRESH_FAILED` | Worker refresh gagal (S2-AC4) | Ya |
| `EXPORT.GENERATED` | Export engine selesai (S3-AC1, S3-AC2, S4-AC1) | Ya |
| `EXPORT.DOWNLOADED` | User klik signed URL download (S4-AC4) | Ya |
| `SCHEDULED_EMAIL.CREATED` | POST /reports/scheduled-email berhasil (S5-AC1) | Ya |
| `SCHEDULED_EMAIL.SENT` | SMTP send berhasil per jadwal (S5-AC2) | Ya |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `jrnl.jurnal_header` + `jrnl.jurnal_detail` | P5-M2/M12 → S1 | Source `mv_jurnal_summary` |
| `jrnl.gl_delivery` | P5-M3 → S1 | Source `mv_gl_delivery_status` |
| `mst.periode_buku` hard-close | P5-M4 → S2 | Trigger on-demand refresh setelah hard-close |
| `trx.akrual`, `trx.jatuh_tempo` | P5-M9 → S1 | Source `mv_akrual_summary` |
| `ecl.poci_delta_log` | P5-M10 → S1 | Source `mv_poci_delta_summary` |
| `sys.upload_batch` | P5-M11 → S4 | Pattern `sys.job` reuse (job tracking) |
| `mst.mapping_jurnal_header` | P5-M12 → S1 | Source `mv_jurnal_summary` (join) |
| `sys.job` | P5-M13-S4 → P5-M14 | Job tracking pattern reused oleh laporan individual M14 |

---

## Migration 000050 — Objek Baru

| Objek | Tipe | Keterangan |
|---|---|---|
| `rpt.mv_status_periode` | MATERIALIZED VIEW | Unique idx `(periode_id, tenant_id)` |
| `rpt.mv_jurnal_summary` | MATERIALIZED VIEW | Unique idx `(periode_id, event_code, tenant_id)` |
| `rpt.mv_gl_delivery_status` | MATERIALIZED VIEW | Unique idx `(periode_id, delivery_id, tenant_id)` |
| `rpt.mv_mtm_daily_summary` | MATERIALIZED VIEW | Unique idx `(tanggal_mtm, instrumen_id, tenant_id)` |
| `rpt.mv_akrual_summary` | MATERIALIZED VIEW | Unique idx `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_renewal_summary` | MATERIALIZED VIEW | Unique idx `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_penjualan_summary` | MATERIALIZED VIEW | Unique idx `(periode_id, instrumen_id, tenant_id)` |
| `rpt.mv_poci_delta_summary` | MATERIALIZED VIEW | Unique idx `(periode_id, instrumen_id, tenant_id)` |
| `sys.mv_refresh_log` | TABLE | Track status refresh per MV |
| `sys.export_log` | TABLE | SHA-256 hash + minio_path + signed_url_ttl |
| `sys.scheduled_email` | TABLE | Konfigurasi jadwal email per laporan |
| `sys.scheduled_email_optout` | TABLE | Opt-out per recipient per sched_id |

---

## Handoff Berikutnya

- `system-analyst` → OpenAPI: endpoint `GET /api/v1/reports/{slug}/export`, `GET /api/v1/reports/{slug}/export` (202 async), `POST /api/v1/admin/mv-refresh`, `GET /api/v1/jobs/{jobId}`, `GET /api/v1/jobs/{jobId}/stream` (SSE), `POST /api/v1/reports/scheduled-email`, `DELETE /api/v1/reports/scheduled-email/{id}/optout`; 6 error codes baru; `sys.job` state machine `queued→running→completed|failed|cancelled`
- `data-modeler` → migration 000050: 8 MV dengan unique idx; 4 sys.* tabel (mv_refresh_log, export_log, scheduled_email, scheduled_email_optout); bucket lifecycle MinIO policy (runbook)
- `security-engineer` → **BLOCKING**: audit semua 6 events in-transaction; SHA-256 dual (file content + audit log row); MinIO signed URL TTL 24h; SMTP creds tidak di-hardcode; EXPORT_PERMISSION_DENIED enforce (`report.{slug}.export` per-report, bukan wildcard); ROLE-AUDIT read-all bypass documented
- `ifrs9-compliance-reviewer` → **ADVISORY** (M13 tidak menyentuh ECL/EIR formula; gate formal ada di M14 saat laporan ECL individual diimplementasi)
- `devops-engineer` → MinIO bucket `exports` + lifecycle `Expiration: 1 day`; env `MV_DSN`, `SMTP_HOST/PORT/FROM/PASSWORD`; Grafana alert rule untuk `MV_REFRESH_FAILED` + `SCHEDULED_EMAIL_SMTP_FAILED`

_Story set ini siap dihandoff ke `system-analyst` (OpenAPI + SSE spec) dan `data-modeler` (migration 000050) secara paralel. `security-engineer` BLOCKING gate sebelum backend merge. `devops-engineer` setup MinIO lifecycle + env vars paralel. `ifrs9-compliance-reviewer` advisory — tidak memblok M13._
