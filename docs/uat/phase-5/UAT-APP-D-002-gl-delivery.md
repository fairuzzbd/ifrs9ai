# UAT-APP-D-002 — GL Host REST Delivery Engine (P5-M3)

**Modul**: APP-D — Periode Buku + FX + Mapping Jurnal & GL  
**Fase**: Phase 5 — Milestone 3  
**Versi Dokumen**: 1.0  
**Tanggal**: 2026-06-17  
**Penulis**: qa-engineer  
**Referensi**: `docs/stories/phase-5/P5-M3-gl-delivery.md`, `docs/state-machines/p5-m3-gl-delivery.md`

---

## 1. Cakupan UAT

Script ini mencakup 5 user story × 20 acceptance criteria P5-M3:

| Story | Judul | AC |
|---|---|---|
| S1 | Auto-delivery jurnal ke GL Host | S1-AC1 … S1-AC5 |
| S2 | Status tracking per jurnal | S2-AC1 … S2-AC3 |
| S3 | Manual retry oleh ROLE-AKUN-CTL | S3-AC1 … S3-AC4 |
| S4 | Rekonsiliasi harian BLIPS vs GL Host | S4-AC1 … S4-AC4 |
| S5 | DLQ Console (inspect + replay + discard) | S5-AC1 … S5-AC4 |

---

## 2. Pre-conditions

### 2.1 Data seed (jalankan sebelum UAT)

```sql
-- Seed test periode buku (hard-closed agar tidak bisa diubah)
INSERT INTO mst.periode_buku (id, bulan_tahun, status, tenant_id, created_by, updated_by)
VALUES (
  'periodo-d002-0000-0000-000000000001',
  '2026-06-01',
  'HARD_CLOSED',
  'TUGURE',
  'seed-user-00000000-0000-0000-0000-000000000001',
  'seed-user-00000000-0000-0000-0000-000000000001'
);

-- Seed jurnal header POSTED + gl_status PENDING_DELIVERY (untuk S1, S2)
INSERT INTO jrnl.jurnal_header (id, no_jurnal, tanggal_posting, event_code, narrative,
  total_debit, total_kredit, idempotency_key, status_internal, tenant_id, created_by, updated_by)
VALUES
  ('jrnl-d002-uat1-0000-000000000001', 'JRN-UAT-2026-0001', '2026-06-15', 'PENEMPATAN',
   'UAT Penempatan Deposito BCA', 500000000.0000, 500000000.0000,
   'idem-uat-001-00000000000000000001', 'POSTED', 'TUGURE',
   'uat-maker-0000-0000-0000-000000000001', 'uat-maker-0000-0000-0000-000000000001'),
  ('jrnl-d002-uat2-0000-000000000002', 'JRN-UAT-2026-0002', '2026-06-15', 'PENEMPATAN',
   'UAT Deposito Mandiri (untuk domain error)', 200000000.0000, 200000000.0000,
   'idem-uat-002-00000000000000000002', 'POSTED', 'TUGURE',
   'uat-maker-0000-0000-0000-000000000001', 'uat-maker-0000-0000-0000-000000000001');

-- gl_status PENDING_DELIVERY untuk jurnal 1
INSERT INTO jrnl.gl_status (id, jurnal_header_id, gl_host_status, retry_count,
  delivery_mode, tenant_id, created_by, updated_by)
VALUES
  ('glst-d002-uat1-0000-000000000001', 'jrnl-d002-uat1-0000-000000000001',
   'PENDING_DELIVERY', 0, 'API', 'TUGURE',
   'seed-user-00000000-0000-0000-0000-000000000001',
   'seed-user-00000000-0000-0000-0000-000000000001'),
  ('glst-d002-uat2-0000-000000000002', 'jrnl-d002-uat2-0000-000000000002',
   'FAILED', 0, 'API', 'TUGURE',
   'seed-user-00000000-0000-0000-0000-000000000001',
   'seed-user-00000000-0000-0000-0000-000000000001');

-- DLQ entry untuk S5 (jurnal 2 yang gagal)
INSERT INTO sys.dlq_gl_delivery (id, jurnal_header_id, gl_status_id, failure_category,
  error_code, error_message, attempt_count, status, no_jurnal, tenant_id, created_by, updated_by)
VALUES (
  'dlq-d002-uat1-0000-000000000001',
  'jrnl-d002-uat2-0000-000000000002',
  'glst-d002-uat2-0000-000000000002',
  'DOMAIN', 'GL_DELIVERY_HOST_4XX',
  'Kode akun 1101 tidak ditemukan di GL Host',
  1, 'FAILED',
  'JRN-UAT-2026-0002', 'TUGURE',
  '00000000-0000-0000-0000-000000000002',
  '00000000-0000-0000-0000-000000000002'
);

-- Reconciliation report untuk S4
INSERT INTO sys.gl_reconciliation_report (id, tanggal_run, trigger_source,
  status, started_at, total_jurnal_idr, mismatch_count, tolerance_idr,
  tenant_id, created_by, updated_by)
VALUES (
  'recon-d002-uat1-0000-000000000001',
  '2026-06-15', 'CRON', 'COMPLETED',
  '2026-06-15 10:00:00+07', 700000000.0000, 0, 1.0000,
  'TUGURE',
  '00000000-0000-0000-0000-000000000002',
  '00000000-0000-0000-0000-000000000002'
);
```

### 2.2 Role assignments (Keycloak)

| User | Role | Digunakan di |
|---|---|---|
| `uat.akun.ctl@tugu-re.com` | ROLE-AKUN-CTL | S3 (manual retry) |
| `uat.akun@tugu-re.com` | ROLE-AKUN | S3 negatif (retry ditolak) |
| `uat.it.admin@tugu-re.com` | ROLE-IT-ADMIN | S5 (DLQ replay + discard) |
| `uat.audit@tugu-re.com` | ROLE-AUDIT | S2 (read-only verification) |

### 2.3 GL Host simulator

Pastikan GL Host stub simulator aktif di endpoint yang dikonfigurasi di `sys.config` key `GL_HOST_BASE_URL`. Untuk UAT, gunakan WireMock atau `stub-adapter` yang dikonfigurasi via flag `GL_HOST_MODE=stub`.

---

## 3. Skenario UAT

### S1 — Auto-Delivery Jurnal ke GL Host

#### S1-TC-01: Auto-deliver jurnal berhasil (S1-AC1)

**Pre-condition**: Jurnal `JRN-UAT-2026-0001` ada di `jrnl.gl_status` dengan status `PENDING_DELIVERY`.  
**GL Host simulator**: dikonfigurasi return 201.

**Langkah**:
1. Masuk sebagai `uat.it.admin@tugu-re.com`.
2. Buka halaman **Jurnal → Detail Jurnal** untuk `JRN-UAT-2026-0001`.
3. Trigger delivery secara manual (atau tunggu Asynq worker yang memproses queue `gl_delivery:deliver`).
4. Refresh halaman setelah ±5 detik.

**Hasil yang diharapkan**:
- Status badge di header detail berubah dari `PENDING_DELIVERY` ke `DELIVERED` (warna hijau).
- Field `Delivered At` terisi dengan timestamp.
- Field `GL Host Journal ID` terisi (contoh: `STUB-JRN-idem-uat`).
- Di tabel `jrnl.gl_status`: `gl_host_status = 'DELIVERED'`, `delivered_at IS NOT NULL`.
- Di `aud.audit_log`: satu baris dengan `action = 'GL_DELIVERY.SUCCESS'`, `entity_id = 'glst-d002-uat1-0000-000000000001'`.

**Verifikasi audit**:
```sql
SELECT action, actor_role, after_jsonb->>'gl_host_status' AS new_status
FROM aud.audit_log
WHERE entity_id = 'glst-d002-uat1-0000-000000000001'
AND action = 'GL_DELIVERY.SUCCESS'
ORDER BY event_time DESC LIMIT 1;
-- Expected: action='GL_DELIVERY.SUCCESS', new_status='DELIVERED'
```

---

#### S1-TC-02: Idempotency — jurnal sudah DELIVERED tidak dikirim ulang (S1-AC2)

**Pre-condition**: Jurnal `JRN-UAT-2026-0001` sudah `DELIVERED` dari TC-01.

**Langkah**:
1. Simulasikan Asynq task `gl_delivery:deliver` untuk `jrnl-d002-uat1-0000-000000000001` dikirim ulang (duplicate task di queue).
2. Amati log worker.

**Hasil yang diharapkan**:
- Worker log: `"gldelivery: skip delivery — terminal state"`.
- Status tetap `DELIVERED`. Tidak ada audit row baru. GL Host simulator tidak dipanggil (call count tidak bertambah).

---

#### S1-TC-03: Domain error 4xx → DLQ (S1-AC3)

**Pre-condition**: GL Host simulator dikonfigurasi return 422 untuk `JRN-UAT-2026-0002`.

**Langkah**:
1. Trigger delivery untuk `JRN-UAT-2026-0002`.
2. Tunggu worker memproses.
3. Buka halaman **GL Delivery → DLQ Console**.

**Hasil yang diharapkan**:
- Status `JRN-UAT-2026-0002` berubah ke `FAILED` (warna merah).
- DLQ Console menampilkan entry baru: `failure_category = DOMAIN`, `error_code = GL_DELIVERY_HOST_4XX`.
- Tidak ada retry otomatis (Asynq tidak mengantri ulang — SkipRetry).
- Audit: `action = 'GL_DELIVERY.FAILED'` dengan `failure_category = 'DOMAIN'`.

---

#### S1-TC-04: Infra error 5xx → retry → DLQ (S1-AC4)

**Pre-condition**: GL Host simulator dikonfigurasi return 503 untuk 4 request berturut-turut.

**Langkah**:
1. Trigger delivery jurnal baru `JRN-UAT-2026-0003` (buat seed baru atau gunakan endpoint test).
2. Amati status setiap 30 detik (backoff: 30s/120s/600s).
3. Setelah 3 retry habis, amati DLQ.

**Hasil yang diharapkan**:
- Status berpindah: `PENDING_DELIVERY` → `RETRYING` → `RETRYING` → `RETRYING` → `FAILED` → masuk DLQ.
- DLQ: `failure_category = INFRA`, `attempt_count = 3`.
- Audit: 3 baris `GL_DELIVERY.RETRY`, 1 baris `GL_DELIVERY.FAILED`.

---

#### S1-TC-05: Status DEAD_LETTER diabaikan oleh worker (S1-AC5)

**Pre-condition**: Seed satu entry dengan `gl_host_status = 'DEAD_LETTER'`.

**Langkah**:
1. Kirim Asynq task delivery untuk header tersebut.
2. Amati log worker.

**Hasil yang diharapkan**:
- Worker log: `"gldelivery: skip delivery — terminal state"`. Task selesai tanpa error. Status tidak berubah.

---

### S2 — Status Tracking

#### S2-TC-01: Tampilkan status DELIVERED (S2-AC1)

**Langkah** (login sebagai `uat.audit@tugu-re.com`):
1. Buka `GET /api/v1/jurnal/header/jrnl-d002-uat1-0000-000000000001/gl-delivery-status`.
2. Periksa response.

**Hasil yang diharapkan**:
```json
{
  "data": {
    "glHostStatus": "DELIVERED",
    "canRetry": false,
    "deliveredAt": "2026-06-...",
    "retryCount": 0
  }
}
```
- `canRetry = false` (DELIVERED tidak bisa di-retry).
- `glResponsePayloadJsonb` tidak ada di response (butuh `PermGlDeliveryReadRaw`).

---

#### S2-TC-02: Tampilkan status FAILED (S2-AC2)

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Buka `GET /api/v1/jurnal/header/jrnl-d002-uat2-0000-000000000002/gl-delivery-status`.

**Hasil yang diharapkan**:
```json
{
  "data": {
    "glHostStatus": "FAILED",
    "canRetry": true,
    "failureCategory": "DOMAIN",
    "retryCount": 0
  }
}
```
- `canRetry = true` — ROLE-AKUN-CTL dapat melakukan manual retry.

---

#### S2-TC-03: Tampilkan status PENDING_DELIVERY (S2-AC3)

**Langkah**:
1. Buat jurnal baru dengan status `PENDING_DELIVERY` (belum diproses worker).
2. Panggil GET delivery status.

**Hasil yang diharapkan**: `glHostStatus = "PENDING_DELIVERY"`, `canRetry = false`.

---

### S3 — Manual Retry

#### S3-TC-01: Manual retry berhasil oleh ROLE-AKUN-CTL (S3-AC1)

**Pre-condition**: `JRN-UAT-2026-0002` berstatus `FAILED`. GL Host simulator sudah diperbaiki (kembali return 201).

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Buka halaman **Jurnal → Detail `JRN-UAT-2026-0002`**.
2. Klik tombol **Retry GL Delivery**.
3. Isi alasan: `"Kode akun 1101 sudah diperbaiki di GL Host per tanggal 2026-06-17."`.
4. Klik **Konfirmasi Retry**.

**Hasil yang diharapkan**:
- Toast sukses: `"Retry jurnal JRN-UAT-2026-0002 berhasil diajukan. Menunggu diproses."`.
- Status berubah ke `PENDING_DELIVERY`.
- Di `aud.audit_log`: baris `action = 'GL_DELIVERY.MANUAL_RETRY_INITIATED'`, `before.gl_host_status = 'FAILED'`, `after.gl_host_status = 'PENDING_DELIVERY'`.
- **Kritis**: Audit row harus ada SEBELUM task Asynq di-enqueue (verified via timestamp).
- Setelah Asynq memproses: status menjadi `DELIVERED`.

**Verifikasi urutan audit-before-enqueue**:
```sql
-- Ambil timestamp audit MANUAL_RETRY_INITIATED
SELECT event_time AS audit_ts FROM aud.audit_log
WHERE action = 'GL_DELIVERY.MANUAL_RETRY_INITIATED'
AND entity_id = 'glst-d002-uat2-0000-000000000002'
ORDER BY event_time DESC LIMIT 1;

-- Bandingkan dengan Asynq task enqueue timestamp (ambil dari Redis atau log worker)
-- audit_ts HARUS <= worker_received_ts
```

---

#### S3-TC-02: Manual retry dari DEAD_LETTER ditolak (S3-AC2)

**Pre-condition**: Jurnal dengan status `DEAD_LETTER`.

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Kirim `POST /api/v1/jurnal/header/{id}/retry-gl-delivery` dengan body valid.

**Hasil yang diharapkan**:
- HTTP 422, `error.code = "WORKFLOW_INVALID_TRANSITION"`.
- Status tidak berubah.

---

#### S3-TC-03: Alasan kurang dari 30 karakter ditolak (S3-AC3)

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Isi alasan: `"Terlalu pendek."` (15 karakter).
2. Klik **Konfirmasi Retry**.

**Hasil yang diharapkan**:
- Field `alasan` highlight merah.
- Pesan inline: `"Alasan minimal 30 karakter."`.
- Tombol submit tetap disabled sampai alasan cukup panjang.
- API: HTTP 422, `details[0].field = "reason"`.

---

#### S3-TC-04: ROLE-AKUN tidak bisa retry (S3-AC4)

**Langkah** (login sebagai `uat.akun@tugu-re.com`):
1. Buka `POST /api/v1/jurnal/header/{id}/retry-gl-delivery`.

**Hasil yang diharapkan**:
- HTTP 403, `error.code = "FORBIDDEN"`, `error.message` menyebutkan permission `jurnal.gl_delivery.retry`.
- Tombol **Retry** tidak tampil di UI untuk ROLE-AKUN.

---

### S4 — Rekonsiliasi Harian

#### S4-TC-01: Rekonsiliasi seimbang → COMPLETED (S4-AC1)

**Pre-condition**: BLIPS total jurnal IDR = GL Host total = IDR 700.000.000 untuk tanggal 2026-06-15.

**Langkah**:
1. Tunggu cron harian (atau trigger manual).
2. Buka halaman **Rekonsiliasi → Riwayat**.

**Hasil yang diharapkan**:
- Report `2026-06-15`: status `COMPLETED`, `mismatch_count = 0`, `delta_idr = 0.0000`.
- Badge hijau.

---

#### S4-TC-02: Rekonsiliasi ada selisih → COMPLETED_WITH_MISMATCH (S4-AC2)

**Pre-condition**: GL Host mengirimkan akun tambahan yang tidak ada di BLIPS (GL-only), atau ada selisih amount (tolerance > IDR 1.0000).

**Langkah**:
1. Trigger rekonsiliasi untuk tanggal dengan mismatch.
2. Buka detail laporan.

**Hasil yang diharapkan**:
- Status `COMPLETED_WITH_MISMATCH`, `mismatch_count ≥ 1`.
- Tabel **Mismatch Lines** menampilkan: `kode_akun`, `blips_amount_idr`, `gl_host_amount_idr`, `delta_idr`, `mismatch_type` (`BLIPS_ONLY` / `GL_ONLY` / `AMOUNT_DIFF`).
- Badge amber/oranye.

---

#### S4-TC-03: Trigger manual rekonsiliasi (S4-AC3)

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Buka halaman **Rekonsiliasi**.
2. Klik **Run Rekonsiliasi** untuk tanggal `2026-06-16`.
3. Isi body: `{"date": "2026-06-16"}`.

**Hasil yang diharapkan**:
- HTTP 202 dengan `{ jobId, statusUrl, streamUrl }`.
- Progress panel `<JobProgressPanel>` tampil dengan progress bar.
- Setelah selesai: toast `"Rekonsiliasi 2026-06-16 selesai."` + link ke laporan.
- Asynq task `gl_delivery:reconcile-daily` di-enqueue (verify via Asynq web UI atau log).

---

#### S4-TC-04: GL Host error saat rekonsiliasi (S4-AC4)

**Pre-condition**: GL Host simulator dikonfigurasi return 503 untuk daily summary.

**Langkah**:
1. Trigger rekonsiliasi manual.

**Hasil yang diharapkan**:
- Report status `FAILED`, `error_summary` berisi pesan error.
- Tidak ada baris mismatch yang di-insert.
- Toast error: `"Rekonsiliasi gagal — GL Host tidak tersedia."`.

---

### S5 — DLQ Console

#### S5-TC-01: Tampilkan daftar DLQ (S5-AC1)

**Langkah** (login sebagai `uat.it.admin@tugu-re.com`):
1. Buka halaman **GL Delivery → DLQ Console**.

**Hasil yang diharapkan**:
- Tabel menampilkan semua entry DLQ dengan kolom: `No Jurnal`, `Status`, `Failure Category`, `Error Code`, `Retry Count`, `Created At`, `Aksi`.
- Filter tersedia: `status` (FAILED / REPLAYING / REPLAYED_OK / ABANDONED), `failure_category`.
- Sort by `created_at` descending secara default.
- Tombol **Export CSV** tersedia.
- Entry `JRN-UAT-2026-0002` tampil dengan `failure_category = DOMAIN`.

---

#### S5-TC-02: Replay DLQ entry berhasil (S5-AC2)

**Pre-condition**: DLQ entry `dlq-d002-uat1-0000-000000000001` berstatus `FAILED`.

**Langkah** (login sebagai `uat.it.admin@tugu-re.com`):
1. Klik entry DLQ `JRN-UAT-2026-0002`.
2. Klik tombol **Replay**.
3. Isi alasan: `"GL Host sudah diperbaiki. Kode akun 1101 sudah valid per 2026-06-17."`.
4. Klik **Konfirmasi Replay**.

**Hasil yang diharapkan**:
- HTTP 200, status DLQ berubah ke `REPLAYING` sementara, lalu `REPLAYED_OK` setelah Asynq memproses.
- `jrnl.gl_status.gl_host_status` berubah ke `PENDING_DELIVERY`, lalu `DELIVERED`.
- Toast: `"Replay berhasil diajukan untuk JRN-UAT-2026-0002."`.
- Audit: `action = 'GL_DELIVERY.DLQ_REPLAY_INITIATED'`.

---

#### S5-TC-03: Discard DLQ entry oleh ROLE-IT-ADMIN (S5-AC3)

**Langkah** (login sebagai `uat.it.admin@tugu-re.com`):
1. Buka DLQ entry lain yang masih `FAILED`.
2. Klik tombol **Discard** (tombol merah).
3. Dialog konfirmasi muncul: `"Tindakan ini permanen dan tidak dapat dibatalkan."`.
4. Isi alasan: `"GL Host mengkonfirmasi kode akun tidak valid dan tidak bisa diperbaiki."`.
5. Klik **Konfirmasi Discard**.

**Hasil yang diharapkan**:
- Status DLQ berubah ke `ABANDONED`. Status jurnal berubah ke `DEAD_LETTER`.
- Toast: `"DLQ entry JRN-UAT-... berhasil di-discard."`.
- Audit: `action = 'GL_DELIVERY.DLQ_DISCARDED'`, `after_jsonb.discard_reason` terisi.
- Tombol **Replay** dan **Discard** menghilang (DEAD_LETTER / ABANDONED = terminal).

---

#### S5-TC-04: ROLE-AKUN-CTL tidak bisa discard (S5-AC4)

**Langkah** (login sebagai `uat.akun.ctl@tugu-re.com`):
1. Buka DLQ Console.

**Hasil yang diharapkan**:
- Tombol **Discard** tidak ada di DOM (bukan hanya disabled — benar-benar tidak dirender).
- API `POST /api/v1/jurnal/gl-delivery-dlq/{id}/discard` → HTTP 403, `error.code = "FORBIDDEN"`.

---

## 4. Verifikasi Audit & Keamanan

### 4.1 Hash chain integrity

Setelah semua skenario dijalankan, verifikasi hash chain tidak rusak:

```bash
go run ./cmd/audit-verify --range "2026-06-17:2026-06-17" --tenant TUGURE
# Expected: "Hash chain verified: N rows, 0 broken"
```

### 4.2 PII Sanitization di DLQ payload

```sql
-- Payload di DLQ TIDAK boleh mengandung customer_name / account_no / npwp dalam plaintext
SELECT payload_snapshot_jsonb
FROM sys.dlq_gl_delivery
WHERE id = 'dlq-d002-uat1-0000-000000000001';

-- Verifikasi: customer_name = '[REDACTED]', account_no = '[REDACTED]'
-- GL_HOST_API_KEY = '[REDACTED]' (jika ada di payload asli)
```

### 4.3 SoD — user tidak bisa retry jurnal sendiri

```bash
# Buat penempatan sebagai uat.akun.ctl, lalu coba retry sebagai user yang sama
curl -X POST .../jurnal/header/{id}/retry-gl-delivery \
  -H "Authorization: Bearer <token-akun-ctl>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"reason": "SoD test — akun ctl yang sama mencoba retry jurnal sendiri."}'
# Expected: 403 FORBIDDEN (SoD violation)
```

*Catatan: SoD untuk GL delivery spesifik pada approval workflow, bukan pada retry. Verifikasi bahwa ROLE-AKUN tidak dapat mengakses endpoint retry (hanya ROLE-AKUN-CTL dan ROLE-IT-ADMIN).*

### 4.4 Idempotency

```bash
KEY=$(uuidgen)
# Kirim request retry dua kali dengan key yang sama
curl -X POST .../jurnal/header/{id}/retry-gl-delivery \
  -H "Idempotency-Key: $KEY" \
  -d '{"reason": "Idempotency test — request pertama dari dua request identik."}'

curl -X POST .../jurnal/header/{id}/retry-gl-delivery \
  -H "Idempotency-Key: $KEY" \
  -d '{"reason": "Idempotency test — request pertama dari dua request identik."}'

# Expected: response kedua = IDEMPOTENCY_REPLAY (200) dengan body yang identik
# Hanya 1 Asynq task di-enqueue, bukan 2
```

---

## 5. Rollback / Cleanup

```sql
-- Hapus data UAT (soft delete di tabel yang relevan)
UPDATE sys.dlq_gl_delivery
SET deleted_at = NOW(), deleted_by = 'seed-user-00000000-0000-0000-0000-000000000001'
WHERE no_jurnal LIKE 'JRN-UAT-2026-%';

UPDATE sys.gl_reconciliation_report
SET deleted_at = NOW(), deleted_by = 'seed-user-00000000-0000-0000-0000-000000000001'
WHERE id = 'recon-d002-uat1-0000-000000000001';

-- CATATAN: jrnl.gl_status dan aud.audit_log TIDAK boleh di-delete (aturan keras DEC-018)
-- Data UAT di aud.audit_log dipertahankan dan dikecualikan dari audit periodik via tag 'UAT'
```

---

## 6. Kriteria Lulus UAT

| Kriteria | Target |
|---|---|
| Semua 20 AC (S1-AC1 s/d S5-AC4) pass | 100% |
| Audit hash chain valid | 0 broken |
| PII tidak terekspos di DLQ payload | 0 field plaintext |
| Idempotency replay berfungsi | Verified |
| SoD 403 untuk ROLE-AKUN | Verified |
| Performance delivery < 300ms p95 | Verified via Grafana |

**Tanda tangan UAT Lead**: ___________________________  
**Tanggal UAT selesai**: ___________________________  
**Versi build yang diuji**: ___________________________
