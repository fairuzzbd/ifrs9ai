# UAT-APP-D-003 — Periode Buku Close Workflow (P5-M4)

**Module:** APP-D — Periode Buku + FX + Mapping Jurnal & GL  
**Feature:** Soft-close, Hard-close (CFO MFA step-up), Reopen (grace window), Closing Checklist  
**Referensi:** P5-M4-periode-close.md (stories), p5-m4-periode-close.md (state machine)  
**Versi dokumen:** 1.0 | 2026-06-17  
**Status:** DRAFT — Menunggu sign-off QA Engineer

---

## Peta Test Cases

| TC | Story | AC | Persona | Fokus |
|---|---|---|---|---|
| TC-01 | S1 | AC1 | ROLE-AKUN-CTL | Soft-close request happy path |
| TC-02 | S1 | AC2 | ROLE-AKUN-CTL | Duplicate pending → 409 |
| TC-03 | S1 | AC3 | ROLE-AKUN-CTL | GL_DELIVERED gagal → 422 + snapshot |
| TC-04 | S1 | AC4 | ROLE-AKUN-CTL / ROLE-AUDIT | MANUAL_CHECK via checklist panel |
| TC-05 | S2 | AC1 | ROLE-AKUN-CTL (maker=approver) | SoD violation → 403 |
| TC-06 | S2 | AC2 | ROLE-AKUN-CTL (approver) | Stale checklist > 24h → 422 STALE |
| TC-07 | S2 | AC3 | ROLE-AKUN-CTL (approver) | Soft-close approve happy path → SOFT_CLOSED |
| TC-08 | S2 | AC4 | ROLE-AKUN-CTL | Row-version conflict → 409 |
| TC-09 | S3 | AC1 | ROLE-AKUN-CTL + ROLE-CFO | Full hard-close flow → CLOSED |
| TC-10 | S3 | AC2 | ROLE-CFO | Hard-close-approve tanpa step-up MFA → 401 |
| TC-11 | S3 | AC3 | ROLE-CFO | Step-up token kedaluwarsa → 401 EXPIRED |
| TC-12 | S3 | AC4 | ROLE-CFO | Hard-close-reject → kembali SOFT_CLOSED |
| TC-13 | S4 | AC1 | ROLE-CFO | Reopen SOFT_CLOSED → OPEN, tanpa step-up |
| TC-14 | S4 | AC2 | ROLE-CFO | Reopen CLOSED → SOFT_CLOSED, step-up, kurs unlock |
| TC-15 | S4 | AC3 | ROLE-CFO | Alasan reopen < 30 karakter → 400 |
| TC-16 | S4 | AC4 | ROLE-CFO | Reopen CLOSED setelah grace window → 423 |
| TC-17 | S5 | AC1 | ROLE-AKUN-CTL | GET closing-checklist — 4 item, 1 gagal |
| TC-18 | S5 | AC2 | ROLE-AUDIT / ROLE-MAKER-TR | ListStatusPeriode — filter + 403 |
| TC-19 | S5 | AC3 | ROLE-AUDIT | Export CSV — job progress + audit row |
| TC-20 | S5 | AC4 | ROLE-AUDIT | Checklist setelah CLOSED — snapshot HARD_CLOSE_APPROVE |

---

## Pre-kondisi Global

### Seed Data (SQL)

```sql
-- 1. Buat periode buku untuk UAT P5-M4
INSERT INTO mst.periode_buku (
    id, periode_id_kode, tahun_buku, bulan, tipe_periode,
    tanggal_mulai, tanggal_akhir, status_periode,
    row_version, tenant_id, created_by, updated_by
) VALUES
    ('uat-p5m4-periode-06'::uuid, '2026-06', 2026, 6, 'BULANAN',
     '2026-06-01', '2026-06-30', 'OPEN', 1, 'TUGURE',
     'uat-maker-user'::uuid, 'uat-maker-user'::uuid);

-- 2. User seed (via Keycloak atau sec.user)
-- ROLE-AKUN-CTL maker  : user ID = uat-akun-ctl-maker-01
-- ROLE-AKUN-CTL approver: user ID = uat-akun-ctl-approver-01 (BEDA dari maker)
-- ROLE-CFO              : user ID = uat-cfo-01
-- ROLE-AUDIT            : user ID = uat-audit-01
-- ROLE-MAKER-TR         : user ID = uat-maker-tr-01

-- 3. Seed jurnal yang sudah POSTED dan GL DELIVERED
INSERT INTO jrnl.header (id, periode_buku_id, no_jurnal, event_code, tanggal_posting,
    total_debit, total_kredit, status_internal, created_by, updated_by, tenant_id)
VALUES
    ('uat-jrnl-001'::uuid, 'uat-p5m4-periode-06'::uuid, 'JRN-2026-001', 'PLACEMENT',
     '2026-06-10', 100000000.0000, 100000000.0000, 'POSTED',
     'uat-maker-user'::uuid, 'uat-maker-user'::uuid, 'TUGURE');

INSERT INTO jrnl.gl_status (id, header_id, gl_host_status, delivery_mode, tenant_id, created_by, updated_by)
VALUES ('uat-gl-001'::uuid, 'uat-jrnl-001'::uuid, 'DELIVERED', 'API', 'TUGURE',
        'uat-system'::uuid, 'uat-system'::uuid);

-- 4. Seed rekonsiliasi terakhir COMPLETED
INSERT INTO sys.gl_reconciliation_report (
    id, periode_buku_id, tanggal_rekonsiliasi, status, mismatch_count,
    total_jurnal_idr, gl_host_total_idr, trigger_source, tenant_id, created_by, updated_by)
VALUES ('uat-recon-001'::uuid, 'uat-p5m4-periode-06'::uuid, '2026-06-17', 'COMPLETED', 0,
        100000000.0000, 100000000.0000, 'MANUAL', 'TUGURE',
        'uat-system'::uuid, 'uat-system'::uuid);
```

### Role Assignment (Keycloak)

```
uat-akun-ctl-maker-01    → ROLE-AKUN-CTL, permissions: periode.softclose.request, periode.hardclose.request
uat-akun-ctl-approver-01 → ROLE-AKUN-CTL, permissions: periode.softclose.approve, periode.status.read
uat-cfo-01               → ROLE-CFO, MFA WAJIB, permissions: periode.hardclose.approve, periode.hardclose.reject, periode.reopen.*
uat-audit-01             → ROLE-AUDIT, permissions: periode.status.read, periode.export
uat-maker-tr-01          → ROLE-MAKER-TR, NO periode permissions
```

---

## TC-01 — Soft-Close Request Happy Path

**AC:** S1-AC1  
**Persona:** ROLE-AKUN-CTL Maker (`uat-akun-ctl-maker-01`)

### Pre-kondisi
- Periode `2026-06` dalam status `OPEN`
- Semua jurnal DELIVERED, GL rekon COMPLETED, tidak ada transaksi PENDING_APPROVAL

### Langkah

1. Login sebagai `uat-akun-ctl-maker-01`.
2. Navigasi ke **APP-D → Periode Buku → 2026-06 → Close Workflow**.
3. Klik tombol **"Ajukan Soft Close"**.
4. Dialog konfirmasi muncul — periksa:
   - Panel Checklist menampilkan 4 item.
   - Semua item menampilkan ikon hijau (lulus).
   - Transition label = `SOFT_CLOSE_REQUEST`.
5. Klik **"Konfirmasi"**.
6. Observasi toast sukses dan status badge.

### Expected Result

- HTTP 202 dari `POST /api/v1/periode/{id}/soft-close-request`.
- Toast hijau: *"Soft-close request untuk periode 2026-06 berhasil diajukan. Menunggu review."*
- Status badge masih `OPEN` (belum di-approve).
- Field `soft_close_requested_by` ter-isi di DB.

### Verifikasi SQL

```sql
-- Periode record
SELECT soft_close_requested_by, soft_close_requested_at, row_version, status_periode
FROM mst.periode_buku WHERE periode_id_kode = '2026-06' AND tenant_id = 'TUGURE';
-- Ekspektasi: soft_close_requested_by IS NOT NULL, status_periode = 'OPEN', row_version = 2

-- Snapshot
SELECT id, transition, all_passed, created_at
FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06')
ORDER BY created_at DESC LIMIT 1;
-- Ekspektasi: transition = 'SOFT_CLOSE_REQUEST', all_passed = true

-- Audit
SELECT action, actor_user_id, after_jsonb->>'status' AS status
FROM aud.audit_log
WHERE entity_type = 'PERIODE' AND action = 'PERIODE.SOFT_CLOSE_REQUESTED'
ORDER BY event_time DESC LIMIT 1;
```

---

## TC-02 — Duplicate Soft-Close Request

**AC:** S1-AC2  
**Persona:** ROLE-AKUN-CTL (user kedua berbeda)

### Pre-kondisi
- TC-01 sudah berhasil → `soft_close_requested_by` sudah ter-isi.

### Langkah

1. Login sebagai user ROLE-AKUN-CTL berbeda.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Ajukan Soft Close"**.
4. Klik **"Konfirmasi"**.

### Expected Result

- HTTP 409 dari API.
- Toast merah persistent: *"SOFT_CLOSE_PENDING_EXISTS — Soft-close request sudah ada dan belum di-approve untuk periode 2026-06."*
- Advisory audit row `PERIODE.SOFT_CLOSE_REJECTED` dengan reason `duplicate_request` ditulis di DB.

### Verifikasi SQL

```sql
SELECT action, after_jsonb->>'reason' AS reason
FROM aud.audit_log
WHERE entity_type = 'PERIODE' AND action = 'PERIODE.SOFT_CLOSE_REJECTED'
ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: reason = 'duplicate_request'
```

---

## TC-03 — Checklist Gagal (GL_DELIVERED)

**AC:** S1-AC3  
**Persona:** ROLE-AKUN-CTL Maker

### Pre-kondisi
- Reset periode ke OPEN: `UPDATE mst.periode_buku SET soft_close_requested_by = NULL, row_version = 1 WHERE periode_id_kode = '2026-06';`
- Set 1 jurnal ke status FAILED: `UPDATE jrnl.gl_status SET gl_host_status = 'FAILED' WHERE header_id = 'uat-jrnl-001';`

### Langkah

1. Login sebagai `uat-akun-ctl-maker-01`.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Ajukan Soft Close"** → Dialog muncul.
4. Periksa panel checklist — `GL_DELIVERED` menampilkan ikon merah.
5. Klik **"Konfirmasi"**.

### Expected Result

- HTTP 422, error code `CLOSING_CHECKLIST_FAILED`.
- Dialog menampilkan detail item yang gagal: *"1 jurnal berstatus FAILED"*.
- Link **"Lihat DLQ"** visible dan mengarah ke `/app-d/jurnal/gl-delivery-dlq?filter[status]=FAILED`.
- Snapshot `sys.closing_checklist_snapshot` tersimpan dengan `all_passed = false`.
- `soft_close_requested_by` TIDAK ter-isi di DB (periode tetap pristine).

### Verifikasi SQL

```sql
SELECT all_passed, items_jsonb
FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06')
ORDER BY created_at DESC LIMIT 1;
-- Ekspektasi: all_passed = false, items_jsonb contains GL_DELIVERED.passed = false

SELECT soft_close_requested_by FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: NULL
```

### Cleanup

```sql
UPDATE jrnl.gl_status SET gl_host_status = 'DELIVERED' WHERE header_id = 'uat-jrnl-001';
```

---

## TC-04 — MANUAL_CHECK via Closing Checklist Panel

**AC:** S1-AC4  
**Persona:** ROLE-AKUN-CTL atau ROLE-AUDIT

### Langkah

1. Login sebagai `uat-audit-01`.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik tombol **"Periksa Checklist"** (bukan "Ajukan Soft Close").
4. Panel **Closing Checklist** terbuka.

### Expected Result

- GET `/api/v1/periode/{id}/closing-checklist` → HTTP 200.
- Panel menampilkan 4 item dengan detail masing-masing.
- Field `transition` = `MANUAL_CHECK`.
- Snapshot `MANUAL_CHECK` tersimpan di `sys.closing_checklist_snapshot`.
- Audit advisory `PERIODE.MANUAL_CHECK` ditulis.

### Verifikasi SQL

```sql
SELECT transition, all_passed FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06')
  AND transition = 'MANUAL_CHECK'
ORDER BY created_at DESC LIMIT 1;

SELECT action FROM aud.audit_log
WHERE action = 'PERIODE.MANUAL_CHECK'
ORDER BY event_time DESC LIMIT 1;
```

---

## TC-05 — SoD Violation (Maker = Approver)

**AC:** S2-AC1  
**Persona:** ROLE-AKUN-CTL Maker mencoba approve sendiri

### Pre-kondisi
- TC-01 berhasil → `soft_close_requested_by = uat-akun-ctl-maker-01`.

### Langkah

1. Login sebagai **`uat-akun-ctl-maker-01`** (user yang sama yang membuat request).
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Approve Soft Close"**.
4. Klik **"Konfirmasi"** di dialog.

### Expected Result

- HTTP 403, error code `SOD_VIOLATION`.
- Toast merah: *"Anda tidak bisa menjadi reviewer/approver untuk transaksi yang Anda buat sendiri."*
- Status badge tetap `OPEN`.
- Advisory audit `PERIODE.SOD_VIOLATION` ditulis dengan `reason = 'maker=approver'`.

### Verifikasi SQL

```sql
SELECT action, after_jsonb->>'reason' AS reason, actor_user_id
FROM aud.audit_log WHERE action = 'PERIODE.SOD_VIOLATION' ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: reason = 'maker=approver'
```

---

## TC-06 — Stale Checklist (> 24 Jam)

**AC:** S2-AC2  
**Persona:** ROLE-AKUN-CTL Approver

### Pre-kondisi
- TC-01 berhasil → snapshot `SOFT_CLOSE_REQUEST` tersimpan.
- Back-date snapshot agar tampak > 24 jam lalu:
  ```sql
  UPDATE sys.closing_checklist_snapshot
  SET created_at = created_at - INTERVAL '25 hours'
  WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06')
    AND transition = 'SOFT_CLOSE_REQUEST';
  ```
- Pastikan ada jurnal FAILED (agar re-eval gagal):
  ```sql
  UPDATE jrnl.gl_status SET gl_host_status = 'FAILED' WHERE header_id = 'uat-jrnl-001';
  ```

### Langkah

1. Login sebagai **`uat-akun-ctl-approver-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Approve Soft Close"** → Klik **"Konfirmasi"**.

### Expected Result

- HTTP 422, error code `CLOSING_CHECKLIST_STALE`.
- Toast merah: *"Checklist lebih dari 24 jam yang lalu. Re-evaluasi gagal: GL_DELIVERED tidak pass."*
- Snapshot re-evaluasi baru tersimpan di DB.
- Periode masih `OPEN`.

### Verifikasi SQL

```sql
-- Snapshot ke-2 (re-eval) tersimpan
SELECT COUNT(*) FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06');
-- Ekspektasi: ≥ 2 rows

SELECT status_periode FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: 'OPEN'
```

### Cleanup

```sql
UPDATE jrnl.gl_status SET gl_host_status = 'DELIVERED' WHERE header_id = 'uat-jrnl-001';
```

---

## TC-07 — Soft-Close Approve Happy Path

**AC:** S2-AC3  
**Persona:** ROLE-AKUN-CTL Approver (berbeda dari maker)

### Pre-kondisi
- TC-01 berhasil (snapshot tidak dibuat stale).
- `uat-akun-ctl-approver-01` ≠ `uat-akun-ctl-maker-01`.

### Langkah

1. Login sebagai **`uat-akun-ctl-approver-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Approve Soft Close"**.
4. Tambahkan komentar opsional: *"Semua jurnal balanced, rekon passed."*
5. Klik **"Konfirmasi"**.

### Expected Result

- HTTP 200, `statusPeriode = SOFT_CLOSED`.
- Toast hijau: *"Periode 2026-06 berhasil soft-closed."*
- Status badge berubah ke **SOFT_CLOSED** (warna kuning).
- `tanggal_soft_close` ter-isi di DB.
- Tombol **"Ajukan Hard Close"** muncul di action bar.
- Snapshot `SOFT_CLOSE_APPROVE` tersimpan.
- Audit `PERIODE.SOFT_CLOSE_APPROVED` ditulis in-transaction.
- Akses ke `/app-b/transaksi?periode_id=...` menampilkan **PeriodeLockBanner**.

### Verifikasi SQL

```sql
SELECT status_periode, tanggal_soft_close, soft_close_approved_by, row_version
FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: status_periode = 'SOFT_CLOSED', tanggal_soft_close IS NOT NULL, row_version = 3

SELECT transition FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06')
  AND transition = 'SOFT_CLOSE_APPROVE';
-- Ekspektasi: 1 row

SELECT action FROM aud.audit_log
WHERE action = 'PERIODE.SOFT_CLOSE_APPROVED' ORDER BY event_time DESC LIMIT 1;
```

---

## TC-08 — Row-Version Conflict

**AC:** S2-AC4  
**Persona:** ROLE-AKUN-CTL Approver

### Pre-kondisi
- TC-07 selesai (periode SOFT_CLOSED, `row_version = 3`).
- Reset ke OPEN untuk skenario ini:
  ```sql
  UPDATE mst.periode_buku SET status_periode = 'OPEN', soft_close_requested_by = 'uat-akun-ctl-maker-01'::uuid,
    soft_close_requested_at = now(), row_version = 2 WHERE periode_id_kode = '2026-06';
  ```
- Buka 2 tab browser sebagai approver.
- Tab 1: approve sukses → row_version naik ke 3.
- Tab 2: masih menampilkan row_version = 2 (stale).

### Langkah

1. Di Tab 1: klik **"Approve Soft Close"** → Konfirmasi → sukses.
2. Di Tab 2 (tanpa refresh): klik **"Approve Soft Close"** → Konfirmasi.

### Expected Result

- Tab 2 mendapat HTTP 409, error code `CONFLICT`.
- Toast merah: *"Periode 2026-06 dimodifikasi oleh pengguna lain. Muat ulang halaman."*
- Tombol **"Muat ulang"** di toast mengarah ke reload halaman.

---

## TC-09 — Full Hard-Close Flow

**AC:** S3-AC1  
**Persona:** ROLE-AKUN-CTL (request) + ROLE-CFO (approve)

### Pre-kondisi
- Periode `2026-06` dalam status `SOFT_CLOSED`.
- `uat-cfo-01` memiliki MFA aktif (TOTP dikonfigurasi di Keycloak).
- `uat-akun-ctl-maker-01` ≠ `uat-cfo-01` (SoD hard-close-request vs approve).

### Langkah — Bagian A: Hard-Close Request (ROLE-AKUN-CTL)

1. Login sebagai **`uat-akun-ctl-maker-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Ajukan Hard Close"**.
4. Dialog muncul — panel checklist menampilkan 4 item lulus.
5. Klik **"Konfirmasi"**.
6. Verifikasi status berubah ke `HARD_CLOSE_PENDING`.

### Langkah — Bagian B: Hard-Close Approve (ROLE-CFO + Step-Up MFA)

7. Login sebagai **`uat-cfo-01`**.
8. Navigasi ke **Periode 2026-06 → Close Workflow**.
9. Klik **"Approve Hard Close"**.
10. Dialog **Verifikasi MFA Step-Up** muncul — masukkan kode TOTP dari aplikasi authenticator.
11. Klik **"Verifikasi"**.
12. Dialog konfirmasi hard close muncul — klik **"Konfirmasi Hard Close"**.

### Expected Result

- HTTP 200, `statusPeriode = CLOSED`.
- Toast hijau: *"Periode 2026-06 berhasil hard-closed. Grace window: 48 jam."*
- Status badge **CLOSED** (merah).
- `tanggal_hard_close` dan `hard_close_grace_expires_at` ter-isi.
- `mst.kurs.locked_flag = TRUE` untuk semua kurs periode ini.
- Kartu **MV Refresh Progress** muncul di halaman dengan progress job Asynq.
- Akses ke `/app-b/transaksi?periode_id=...` → banner `PERIODE_CLOSED` (hard lock).

### Verifikasi SQL

```sql
SELECT status_periode, tanggal_hard_close, hard_close_grace_expires_at, hard_close_approved_by, row_version
FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: status_periode = 'CLOSED', tanggal_hard_close IS NOT NULL,
--   hard_close_grace_expires_at ≈ NOW() + 48h

SELECT COUNT(*) FROM mst.kurs
WHERE periode_id_kode = '2026-06' AND locked_flag = TRUE;
-- Ekspektasi: ≥ 1 (semua kurs periode dikunci)

SELECT action, after_jsonb->>'tanggal_hard_close' AS hard_close_date
FROM aud.audit_log WHERE action = 'PERIODE.HARDCLOSED' ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: action = 'PERIODE.HARDCLOSED' dalam transaksi yang sama dengan UPDATE status

-- Asynq job enqueued
SELECT payload FROM sys.job WHERE type = 'reporting:mv_refresh'
  AND payload_jsonb->>'periode_kode' = '2026-06' ORDER BY created_at DESC LIMIT 1;
```

---

## TC-10 — Hard-Close Approve Tanpa Step-Up MFA

**AC:** S3-AC2  
**Persona:** ROLE-CFO

### Pre-kondisi
- Periode `2026-06` dalam status `HARD_CLOSE_PENDING`.

### Langkah (Simulasi via API Direct — bypass UI)

```bash
curl -X POST https://uat.blips.tugu-re.com/api/v1/periode/{periodeId}/hard-close-approve \
  -H "Authorization: Bearer $CFO_JWT" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"rowVersion": 4, "comment": "Test no step-up"}'
  # TIDAK ada header X-Step-Up-Token
```

### Expected Result

- HTTP 401, error code `MFA_STEP_UP_REQUIRED`.
- Response body: `{"error": {"code": "MFA_STEP_UP_REQUIRED", "message": "Step-up MFA diperlukan untuk periode.hardclose.approve."}}`
- Status `HARD_CLOSE_PENDING` tidak berubah.

---

## TC-11 — Step-Up Token Kedaluwarsa

**AC:** S3-AC3  
**Persona:** ROLE-CFO

### Pre-kondisi
- Sama dengan TC-10.
- Dapatkan step-up token valid, tunggu > 5 menit.

### Langkah

```bash
STEP_UP_TOKEN=$(curl -s -X POST .../auth/step-up -d '{"code":"123456","scope":"periode.hardclose.approve"}' | jq -r '.data.stepUpToken')
sleep 310  # > 5 menit

curl -X POST .../api/v1/periode/{periodeId}/hard-close-approve \
  -H "Authorization: Bearer $CFO_JWT" \
  -H "X-Step-Up-Token: $STEP_UP_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"rowVersion": 4}'
```

### Expected Result

- HTTP 401, error code `MFA_STEP_UP_EXPIRED`.
- *"Token step-up MFA sudah kedaluwarsa (> 5 menit). Silakan verifikasi ulang."*
- UI: tombol **"Verifikasi ulang"** di toast.

---

## TC-12 — Hard-Close Reject

**AC:** S3-AC4  
**Persona:** ROLE-CFO

### Pre-kondisi
- Periode `2026-06` dalam status `HARD_CLOSE_PENDING`.

### Langkah

1. Login sebagai **`uat-cfo-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Tolak Hard Close"** (bukan Approve).
4. Dialog muncul — isi alasan: *"Perlu koreksi saldo jurnal akun 1110-DEP sebelum hard close disetujui."*
5. **TIDAK ADA** dialog MFA step-up.
6. Klik **"Konfirmasi Tolak"**.

### Expected Result

- HTTP 200, `statusPeriode = SOFT_CLOSED`.
- Toast hijau: *"Hard close ditolak. Periode dikembalikan ke SOFT_CLOSED."*
- Status badge kembali ke **SOFT_CLOSED**.
- `hard_close_requested_by` di-clear (`NULL`).
- Audit `PERIODE.HARD_CLOSE_REJECTED` ditulis.

### Verifikasi SQL

```sql
SELECT status_periode, hard_close_requested_by
FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: status_periode = 'SOFT_CLOSED', hard_close_requested_by IS NULL
```

---

## TC-13 — Reopen SOFT_CLOSED → OPEN

**AC:** S4-AC1  
**Persona:** ROLE-CFO (requester berbeda dari approver)

### Pre-kondisi
- Periode `2026-06` dalam status `SOFT_CLOSED`.

### Langkah — Request

1. Login sebagai **`uat-cfo-01`** (sebagai requester).
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Klik **"Ajukan Reopen"**.
4. Dialog muncul — **TIDAK ADA** MFA step-up untuk SOFT_CLOSED.
5. Isi alasan: *"Ditemukan kesalahan mapping jurnal pada tanggal 2026-06-15, perlu dikoreksi sebelum periode ditutup kembali."*
6. Klik **"Ajukan"** → HTTP 202.

### Langkah — Approve (user CFO berbeda)

7. Login sebagai user CFO kedua (atau gunakan flow 4-eyes sesuai setup UAT).
8. Klik **"Approve Reopen"** → **TIDAK ADA** MFA step-up (SOFT_CLOSED→OPEN tidak butuh).
9. Klik **"Konfirmasi"**.

### Expected Result

- HTTP 200, `statusPeriode = OPEN`.
- Toast: *"Periode 2026-06 berhasil dibuka kembali ke OPEN. Mutasi kembali diizinkan."*
- `reopened_flag = TRUE`, `reopened_at` ter-isi.
- Audit `PERIODE.REOPEN_APPROVED` ditulis.
- **PeriodeLockBanner** di halaman transaksi menghilang.

---

## TC-14 — Reopen CLOSED → SOFT_CLOSED (Grace Window)

**AC:** S4-AC2  
**Persona:** ROLE-CFO

### Pre-kondisi
- Periode `2026-06` dalam status `CLOSED`.
- `hard_close_grace_expires_at` masih di masa depan.
- `mst.kurs.locked_flag = TRUE`.

### Langkah

1. Login sebagai **`uat-cfo-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Banner grace window countdown terlihat.
4. Klik **"Ajukan Reopen"**.
5. Isi alasan ≥ 30 karakter.
6. Klik **"Ajukan"** → 202.
7. Klik **"Approve Reopen"** (sebagai CFO approver berbeda).
8. Dialog **Verifikasi MFA Step-Up** muncul (wajib untuk CLOSED→SOFT_CLOSED).
9. Masukkan TOTP → **"Verifikasi"** → **"Konfirmasi"**.

### Expected Result

- HTTP 200, `statusPeriode = SOFT_CLOSED`, `fxRateUnlocked = true`.
- Toast: *"Periode 2026-06 dibuka ke SOFT_CLOSED. FX rate di-unlock."*
- `mst.kurs.locked_flag = FALSE` untuk semua kurs periode ini.
- Audit `PERIODE.REOPEN_APPROVED` ditulis.

### Verifikasi SQL

```sql
SELECT COUNT(*) FROM mst.kurs WHERE periode_id_kode = '2026-06' AND locked_flag = FALSE;
-- Ekspektasi: semua kurs unlocked (≥ 1)

SELECT status_periode, reopened_flag, reopened_at
FROM mst.periode_buku WHERE periode_id_kode = '2026-06';
-- Ekspektasi: status = 'SOFT_CLOSED', reopened_flag = true
```

---

## TC-15 — Alasan Reopen Terlalu Pendek

**AC:** S4-AC3  
**Persona:** ROLE-CFO

### Langkah

1. Login sebagai **`uat-cfo-01`**, periode dalam status `SOFT_CLOSED`.
2. Klik **"Ajukan Reopen"**.
3. Isi alasan hanya: *"Koreksi"* (8 karakter, < 30).
4. Klik **"Ajukan"**.

### Expected Result

- Validasi client-side — form TIDAK submit ke server.
- Pesan inline di bawah field: *"Alasan minimal 30 karakter (8/30)."*
- Field `reason` di-highlight merah dengan `aria-invalid = "true"`.
- HTTP call TIDAK terjadi.

---

## TC-16 — Reopen CLOSED Setelah Grace Expired

**AC:** S4-AC4  
**Persona:** ROLE-CFO

### Pre-kondisi
- Periode `2026-06` dalam status `CLOSED`.
- Set `hard_close_grace_expires_at` ke masa lalu:
  ```sql
  UPDATE mst.periode_buku SET hard_close_grace_expires_at = NOW() - INTERVAL '1 hour'
  WHERE periode_id_kode = '2026-06';
  ```

### Langkah

1. Login sebagai **`uat-cfo-01`**.
2. Navigasi ke **Periode 2026-06 → Close Workflow**.
3. Banner grace window menampilkan **"Grace window telah berakhir"**.
4. Klik **"Ajukan Reopen"** (atau coba langsung via API):

```bash
curl -X POST .../api/v1/periode/{periodeId}/reopen-request \
  -H "Authorization: Bearer $CFO_JWT" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"reason": "Perlu reopen meskipun sudah lewat grace, alasan kritis panjang sekali ini", "targetStatus": "SOFT_CLOSED", "rowVersion": 5}'
```

### Expected Result

- HTTP 423, error code `PERIODE_GRACE_EXPIRED`.
- Toast merah: *"Grace window untuk periode 2026-06 sudah berakhir. Tidak dapat direopen. Hubungi CFO."*
- Status tetap `CLOSED`.

---

## TC-17 — GET Closing Checklist (1 Item Gagal)

**AC:** S5-AC1  
**Persona:** ROLE-AKUN-CTL

### Pre-kondisi
- Ada 1 jurnal FAILED di periode aktif.

### Langkah

1. Login sebagai `uat-akun-ctl-maker-01`.
2. GET `/api/v1/periode/{periodeId}/closing-checklist` via browser atau API client.

### Expected Result

- HTTP 200.
- Response memuat 4 item:
  - `PENDING_APPROVAL_ZERO`: passed = true
  - `JURNAL_BALANCED`: passed = true
  - `GL_DELIVERED`: passed = **false**, detail = "N jurnal berstatus FAILED", `actionUrl` berisi link DLQ
  - `RECON_PASS`: passed = true
- `allPassed = false`.
- `transition = MANUAL_CHECK`.
- Detail `GL_DELIVERED` TIDAK menggunakan float64 (threshold IDR 0.01 dinyatakan via decimal).

---

## TC-18 — List Status Periode (Filter + 403)

**AC:** S5-AC2

### Langkah A: ROLE-AUDIT dapat akses

1. Login sebagai `uat-audit-01`.
2. GET `/api/v1/reports/status-periode?filter[status_periode]=CLOSED&sort=tahun_buku:desc&limit=50`.

### Expected Result A

- HTTP 200, data hanya periode dengan `status_periode = CLOSED`.
- `appliedSort = [{"col":"tahun_buku","dir":"desc"}]`.
- `appliedFilter` = `{"status_periode":"CLOSED"}`.
- Cursor pagination: jika lebih dari `limit`, `hasMore = true`, `nextCursor` berisi opaque base64.

### Langkah B: ROLE-MAKER-TR ditolak

```bash
curl -X GET .../api/v1/reports/status-periode \
  -H "Authorization: Bearer $MAKER_TR_JWT"
```

### Expected Result B

- HTTP 403, `FORBIDDEN`.
- *"Anda tidak memiliki izin periode.status.read."*

---

## TC-19 — Export CSV Async Job

**AC:** S5-AC3  
**Persona:** ROLE-AUDIT

### Langkah

1. Login sebagai `uat-audit-01`.
2. Navigasi ke **APP-D → Reports → Status Periode**.
3. Klik **Export ▾ → CSV**.
4. Observasi progress panel.

### Expected Result

- POST `/api/v1/reports/status-periode/export?format=csv` → HTTP 202, `jobId` di response.
- **JobProgressPanel** muncul: progress bar, ETA, current step.
- Setelah selesai: toast sukses + link download file CSV (signed MinIO URL).
- Audit row `PERIODE.EXPORT` ditulis dengan `format = csv`, `row_count`.

### Verifikasi SQL

```sql
SELECT action, after_jsonb->>'format' AS format, after_jsonb->>'row_count' AS row_count
FROM aud.audit_log WHERE action = 'PERIODE.EXPORT' ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: format = 'csv', row_count = N
```

---

## TC-20 — Checklist Setelah CLOSED (Snapshot HARD_CLOSE_APPROVE)

**AC:** S5-AC4  
**Persona:** ROLE-AUDIT

### Pre-kondisi
- Periode `2026-06` dalam status `CLOSED`.
- Snapshot `HARD_CLOSE_APPROVE` ada di DB.

### Langkah

1. Login sebagai `uat-audit-01`.
2. GET `/api/v1/periode/{periodeId}/closing-checklist`.
3. Atau navigasi ke **Periode 2026-06 → Close Workflow → Periksa Checklist**.

### Expected Result

- HTTP 200.
- Panel menampilkan 4 item, semua `passed = true`.
- `transition = HARD_CLOSE_APPROVE`.
- `checklistSnapshotId` = ID snapshot dari saat hard close disetujui.
- Tombol **"Ajukan Soft Close"** tidak tersedia (periode CLOSED).

### Verifikasi SQL

```sql
SELECT s.id, s.transition, s.all_passed, s.created_at
FROM sys.closing_checklist_snapshot s
JOIN mst.periode_buku p ON p.id = s.periode_buku_id
WHERE p.periode_id_kode = '2026-06' AND s.transition = 'HARD_CLOSE_APPROVE'
ORDER BY s.created_at DESC LIMIT 1;
-- Ekspektasi: transition = 'HARD_CLOSE_APPROVE', all_passed = true
```

---

## Rollback / Cleanup

```sql
-- Reset periode ke OPEN untuk menjalankan ulang UAT
UPDATE mst.periode_buku SET
    status_periode = 'OPEN',
    soft_close_requested_by = NULL, soft_close_requested_at = NULL,
    soft_close_approved_by = NULL, soft_close_approved_at = NULL,
    tanggal_soft_close = NULL,
    hard_close_requested_by = NULL, hard_close_requested_at = NULL,
    hard_close_approved_by = NULL, hard_close_approved_at = NULL,
    tanggal_hard_close = NULL, hard_close_grace_expires_at = NULL,
    reopened_flag = FALSE, reopened_reason = NULL, reopened_at = NULL,
    row_version = 1
WHERE periode_id_kode = '2026-06' AND tenant_id = 'TUGURE';

-- Reset kurs
UPDATE mst.kurs SET locked_flag = FALSE WHERE periode_id_kode = '2026-06';

-- Reset GL status
UPDATE jrnl.gl_status SET gl_host_status = 'DELIVERED' WHERE header_id = 'uat-jrnl-001';

-- Hapus snapshot UAT (perlu ON DELETE CASCADE atau manual)
DELETE FROM sys.closing_checklist_snapshot
WHERE periode_buku_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06');
-- CATATAN: Jika trigger append-only aktif di PROD, disable dulu:
-- SET session_replication_role = replica; -- atau ALTER TABLE ... DISABLE TRIGGER ...

-- Hapus audit log UAT (hanya di UAT env, BUKAN PROD)
DELETE FROM aud.audit_log WHERE entity_type = 'PERIODE'
  AND entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-06');
```

---

## Audit Checks (Ringkasan)

| TC | Audit Event | Ditulis In-Transaction | Advisory |
|---|---|---|---|
| TC-01 | `PERIODE.SOFT_CLOSE_REQUESTED` | Ya | Tidak |
| TC-02 | `PERIODE.SOFT_CLOSE_REJECTED` (reason=duplicate) | Tidak | Ya |
| TC-05 | `PERIODE.SOD_VIOLATION` | Tidak | Ya |
| TC-07 | `PERIODE.SOFT_CLOSE_APPROVED` | Ya | Tidak |
| TC-09 | `PERIODE.HARD_CLOSE_REQUESTED`, `PERIODE.HARDCLOSED` | Ya | Tidak |
| TC-12 | `PERIODE.HARD_CLOSE_REJECTED` | Ya | Tidak |
| TC-13 | `PERIODE.REOPEN_REQUESTED`, `PERIODE.REOPEN_APPROVED` | Ya | Tidak |
| TC-19 | `PERIODE.EXPORT` | Ya (within same request context) | Tidak |

---

## Sign-Off

| Peran | Nama | Tanda Tangan | Tanggal |
|---|---|---|---|
| QA Engineer | | | |
| Finance Controller | | | |
| CFO | | | |
| IT Admin | | | |
