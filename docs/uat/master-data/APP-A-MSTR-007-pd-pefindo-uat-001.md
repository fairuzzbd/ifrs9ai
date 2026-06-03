# UAT Script — APP-A-MSTR-007: Kalibrasi PD Pefindo (ECL Parameter)
**ID UAT**: UAT-APP-A-MSTR-007-001
**Story**: APP-A-MSTR-007 Master Data PD Pefindo (6-eyes, XLSX Upload, ECL Param)
**Modul**: APP-A Master Data Management
**Tanggal**: 2026-06-04
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Database sudah dimigrasikan hingga versi terbaru: `go run ./cmd/migrator up`
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`
- Migrasi 0013 (audit kolom pd_pefindo) dan 0008 (WORKFLOW_CONFIG_PD_PEFINDO seed) sudah diterapkan

### Seed Data — Jalankan SQL Berikut Sebelum UAT

```sql
-- ── Personas (4 user berbeda) ──────────────────────────────────────────────
-- Assign role di Keycloak setelah INSERT; username adalah login Keycloak.

INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES
  ('a0000001-0000-0000-0000-000000000001', 'risk.officer1',   'risk1@tugu-re.com',   'Risk Officer UAT-1',    'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
  ('a0000001-0000-0000-0000-000000000002', 'ctl.reviewer1',   'ctl1@tugu-re.com',    'Finance Controller UAT','AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
  ('a0000001-0000-0000-0000-000000000003', 'alco.approver1',  'alco1@tugu-re.com',   'ALCO Member UAT-1',     'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
  ('a0000001-0000-0000-0000-000000000004', 'alco.approver2',  'alco2@tugu-re.com',   'ALCO Member UAT-2',     'AKTIF', now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;

-- Assign Keycloak roles:
--   risk.officer1  → ROLE-RISK    (permission: ecl_parameter.{read,submit,review,reject})
--   ctl.reviewer1  → ROLE-AKUN-CTL (permission: ecl_parameter.{read,review,approve,reject})
--   alco.approver1 → ROLE-ALCO    (permission: ecl_parameter.{read,approve,reject}), MFA wajib
--   alco.approver2 → ROLE-ALCO    (permission: ecl_parameter.{read,approve,reject}), MFA wajib
```

### File Template XLSX
Download atau buat file XLSX (`pd_pefindo_template.xlsx`) dengan:
- Sheet pertama bernama apa saja
- Baris 1 (header): `rating | pd_12m | pd_3y | pd_5y | pd_7y | pd_10y`
- Baris 2+: data per rating Pefindo

Contoh data 3 baris untuk TC-001:

| rating | pd_12m | pd_3y | pd_5y | pd_7y | pd_10y |
|--------|--------|-------|-------|-------|--------|
| idBB+  | 0.08   | 0.12  | 0.16  | 0.20  | 0.25   |
| idBB   | 0.09   | 0.13  | 0.17  | 0.21  | 0.26   |
| idBB-  | 0.10   | 0.14  | 0.18  | 0.22  | 0.27   |

---

## Skenario UAT

### TC-001: Upload XLSX Pefindo (Async Job)

**Aktor**: risk.officer1 (ROLE-RISK)
**Prasyarat**: File `pd_pefindo_template.xlsx` tersedia di workstation

**Langkah-langkah**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Master Data → Kalibrasi PD Pefindo**.
3. Klik tombol **Upload XLSX Pefindo**.
4. Pada dialog upload:
   - Pilih file: `pd_pefindo_template.xlsx`
   - Isi **Tanggal Publikasi**: `2026-06-01`
   - Isi **Periode Berlaku Dari**: `2026-07-01`
   - Biarkan **Periode Berlaku Sampai** kosong (open-end)
5. Klik **Upload**.

**Hasil yang Diharapkan**:

- Tombol Upload berubah menjadi spinner (disabled) — double-submit tidak bisa.
- Panel progress `<JobProgressPanel>` muncul dengan judul "Upload PD Pefindo — Proses".
- Progress bar bergerak dari 0% → 100%.
- Setiap 2 detik progress diperbarui (SSE stream atau polling 2s fallback).
- Setelah selesai:
  - Toast hijau: `"Upload berhasil. 3 baris PD berhasil dibuat dalam status DRAFT."`
  - Link "Lihat daftar" mengarah ke halaman list pd_pefindo.
- Di halaman list, 3 baris baru dengan status DRAFT terlihat (rating: idBB+, idBB, idBB-).

**Pemeriksaan Audit (DB)**:

```sql
SELECT action, event_time, actor_user_id
FROM aud.audit_log
WHERE action = 'PD_PEFINDO.UPLOAD_XLSX'
ORDER BY event_time DESC LIMIT 1;
-- Harus ada 1 baris dengan actor_user_id = UUID risk.officer1
```

**Rollback**: `DELETE FROM mst.pd_pefindo WHERE periode_berlaku_dari = '2026-07-01' AND workflow_status = 'DRAFT';`

---

### TC-002: Buat Manual idAAA — Workflow 6-Eyes Happy Path

**Aktor**: risk.officer1 (maker), ctl.reviewer1 (reviewer), alco.approver1 (approver 1), alco.approver2 (approver 2)

**Langkah-langkah**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Master Data → Kalibrasi PD Pefindo → Tambah Baru**.
3. Isi form:
   - **Rating**: `idAAA`
   - **PD 12 Bulan**: `0.00010000`
   - **PD Lifetime 3Y**: `0.00030000`
   - **PD Lifetime 5Y**: `0.00060000`
   - **PD Lifetime 7Y**: `0.00100000`
   - **PD Lifetime 10Y**: `0.00200000`
   - **Sumber**: `PEFINDO_ANNUAL_DEFAULT_STUDY`
   - **Tanggal Publikasi**: `2026-06-01`
   - **Periode Berlaku Dari**: `2026-07-01`
4. Klik **Simpan**.
   - Toast hijau: `"PD Pefindo idAAA berhasil dibuat dalam status DRAFT."`
5. Klik **Submit untuk Review**.
   - Konfirmasi dialog muncul. Isi komentar: `"Kalibrasi Q2 2026 — Pefindo Annual Default Study"`
   - Klik **Submit**.
   - Status berubah menjadi **MENUNGGU REVIEW**.

6. Login sebagai `ctl.reviewer1`.
7. Navigasi ke **Antrian Review → PD Pefindo** → pilih record idAAA.
8. Review nilai, pastikan monotonicity naik (0.0001 → 0.0003 → 0.0006 → 0.001 → 0.002).
9. Klik **Setujui Review**.
   - Komentar: `"Review selesai — nilai PD sesuai studi Pefindo"`
   - Status berubah menjadi **MENUNGGU PERSETUJUAN**.

10. Login sebagai `alco.approver1`. (MFA step-up dipicu oleh sistem)
11. Navigasi ke **Antrian Persetujuan → PD Pefindo** → pilih record idAAA.
12. Selesaikan MFA step-up (TOTP/Push).
13. Klik **Setujui (Approve 1)**.
    - Komentar: `"Disetujui ALCO — parameter sesuai kebijakan batas risiko"`
    - Status berubah menjadi **MENUNGGU PERSETUJUAN 2**.

14. Login sebagai `alco.approver2`. (MFA step-up dipicu)
15. Navigasi ke **Antrian Persetujuan 2 → PD Pefindo** → pilih record idAAA.
16. Selesaikan MFA step-up.
17. Klik **Setujui Final (Approve 2)**.
    - Komentar: `"Disetujui final ALCO 2 — berlaku mulai 2026-07-01"`
    - Status berubah menjadi **DISETUJUI**.

**Hasil yang Diharapkan**:

- Record idAAA `workflow_status = APPROVED`.
- Toast hijau di setiap langkah, pesan spesifik.
- PATCH (edit) tidak bisa dilakukan setelah APPROVED — tombol Edit tidak muncul atau disabled.

**Pemeriksaan Audit (DB)**:

```sql
-- Harus ada minimal event SUBMIT, APPROVE (termasuk APPROVE2)
SELECT action, event_time, actor_user_id
FROM aud.audit_log
WHERE entity_type = 'mst.pd_pefindo'
  AND action IN ('PD_PEFINDO.SUBMIT','PD_PEFINDO.APPROVE','PD_PEFINDO.APPROVE2')
ORDER BY event_time;

-- Harus ada 4 signature records
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON wi.id = ws.workflow_instance_id
WHERE wi.entity_type = 'PD_PEFINDO'
  AND wi.current_state = 'APPROVED';
-- Expected: 4
```

**Rollback**:

```sql
-- Hapus test record (perlu hapus signature + instance dulu)
DELETE FROM sys.workflow_signature WHERE workflow_instance_id IN (
  SELECT id FROM sys.workflow_instance WHERE entity_type = 'PD_PEFINDO'
    AND entity_id IN (SELECT id FROM mst.pd_pefindo WHERE rating = 'idAAA' AND periode_berlaku_dari = '2026-07-01')
);
DELETE FROM sys.workflow_instance WHERE entity_type = 'PD_PEFINDO'
  AND entity_id IN (SELECT id FROM mst.pd_pefindo WHERE rating = 'idAAA' AND periode_berlaku_dari = '2026-07-01');
DELETE FROM mst.pd_pefindo WHERE rating = 'idAAA' AND periode_berlaku_dari = '2026-07-01';
```

---

### TC-003: Validasi Inline Monotonicity — Preview Warning

**Aktor**: risk.officer1 (ROLE-RISK)
**Prasyarat**: Halaman form Tambah PD Pefindo terbuka

**Langkah-langkah**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Master Data → Kalibrasi PD Pefindo → Tambah Baru**.
3. Isi **Rating**: `idBBB`.
4. Isi **PD 12 Bulan**: `0.05`.
5. Isi **PD Lifetime 3Y**: `0.03` (lebih kecil dari PD 12 Bulan).
6. Pindahkan fokus ke field berikutnya (atau klik di luar field).

**Hasil yang Diharapkan**:

- Pesan warning/error inline muncul di bawah field PD Lifetime 3Y:
  `"PD Lifetime 3 tahun (0.03) tidak boleh lebih kecil dari PD 12 bulan (0.05). PD harus non-decreasing."`
- Tombol **Simpan** dinonaktifkan selama kondisi ini berlaku.
- `aria-describedby` mengarah ke error message (aksesibilitas).

7. Ubah **PD Lifetime 3Y** menjadi `0.06`.
8. Periksa: warning hilang, tombol **Simpan** aktif kembali.

**Pemeriksaan Audit**: Tidak ada (form belum submit).

**Rollback**: Tidak diperlukan.

---

### TC-004: Validasi idD — Semua PD = 1.0 Diterima

**Aktor**: risk.officer1 (ROLE-RISK)

**Langkah-langkah**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Master Data → Kalibrasi PD Pefindo → Tambah Baru**.
3. Isi form:
   - **Rating**: `idD`
   - **PD 12 Bulan**: `1.00000000`
   - **PD Lifetime 3Y**: `1.00000000`
   - **PD Lifetime 5Y**: `1.00000000`
   - **PD Lifetime 7Y**: `1.00000000`
   - **PD Lifetime 10Y**: `1.00000000`
   - **Periode Berlaku Dari**: `2026-09-01`
4. Klik **Simpan**.

**Hasil yang Diharapkan**:

- Tidak ada warning monotonicity (semua nilai sama = non-decreasing).
- Toast hijau: `"PD Pefindo idD berhasil dibuat dalam status DRAFT."`
- Record muncul di list dengan status DRAFT.

**Catatan Domain**: rating `idD` adalah "certain default" — PD = 1.0 di semua horizon adalah data valid sesuai Pefindo Annual Default Study.

**Rollback**: `DELETE FROM mst.pd_pefindo WHERE rating = 'idD' AND periode_berlaku_dari = '2026-09-01';`

---

### TC-005: 6-Eyes Happy Path + 2 Step-Up MFA (Ringkasan Eksplisit)

Merujuk ke TC-002 untuk alur lengkap. Poin fokus TC-005:

**Verifikasi Step-Up MFA**:

1. Saat `alco.approver1` membuka halaman Approve, sistem menampilkan:
   - Dialog: "Tindakan ini memerlukan verifikasi MFA tambahan (Step-Up)"
   - Metode yang ditawarkan: TOTP / Push / WebAuthn
2. Setelah MFA berhasil, tombol **Setujui** aktif.
3. Header request berisi `X-Step-Up-Token: <token>` (verifikasi via network tab DevTools).
4. Sama untuk `alco.approver2` di langkah Approve 2.

**Verifikasi Signature DB**:

```sql
SELECT signer_user_id, action, signed_at, signature_method
FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON wi.id = ws.workflow_instance_id
WHERE wi.entity_type = 'PD_PEFINDO'
ORDER BY ws.signed_at;
-- Expected: 4 baris, signature_method = JWT_STEP_UP untuk approve dan approve2
```

---

### TC-006: SoD Enforcement — Approver 2 Tidak Boleh Sama dengan Aktor Sebelumnya

**Aktor**: alco.approver1 (mencoba double-approve)

**Prasyarat**: Record PD Pefindo dalam status `PENDING_APPROVAL_2`
(jalankan TC-002 langkah 1–13 terlebih dahulu, berhenti sebelum langkah 14)

**Langkah-langkah**:

1. `alco.approver1` (yang sudah melakukan Approve 1) mencoba membuka halaman Approve 2.
2. Klik **Setujui Final (Approve 2)**.
3. Selesaikan MFA step-up.

**Hasil yang Diharapkan**:

- Sistem menampilkan error toast merah persisten:
  `"Anda tidak bisa menjadi approver kedua untuk record yang sudah Anda setujui. (SOD_VIOLATION)"`
- HTTP 403 SOD_VIOLATION dari API.
- `workflow_status` tetap `PENDING_APPROVAL_2` (tidak berubah ke APPROVED).
- Audit log tidak mencatat APPROVE2 oleh approver1.

**Verifikasi DB**:

```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_type = 'PD_PEFINDO' AND entity_id = '<entity_id>';
-- Expected: PENDING_APPROVAL_2
```

**Rollback**: Lanjutkan dengan alco.approver2 untuk approve2 yang valid, atau delete record test.

---

### TC-007: Period Overlap — Rating Sama, Periode Tumpang Tindih

**Aktor**: risk.officer1 (ROLE-RISK)
**Prasyarat**: Sudah ada record PD Pefindo untuk rating `idA` dengan periode `2026-01-01` s.d. open-end

**Langkah-langkah**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Tambah PD Pefindo**.
3. Isi:
   - **Rating**: `idA`
   - **PD 12 Bulan**: `0.004`
   - **Periode Berlaku Dari**: `2026-06-01`
4. Klik **Simpan**.

**Hasil yang Diharapkan**:

- Error toast merah: `"Terdapat record PD untuk rating 'idA' dengan periode yang overlap. Tutup periode record lama sebelum membuat record baru. (PD_PERIOD_OVERLAP)"`
- HTTP 422 PD_PERIOD_OVERLAP.
- Tidak ada record baru terbuat di DB.

**Verifikasi DB**:

```sql
SELECT COUNT(*) FROM mst.pd_pefindo
WHERE rating = 'idA' AND periode_berlaku_dari = '2026-06-01';
-- Expected: 0
```

**Rollback**: Tidak diperlukan (record tidak terbuat).

---

### TC-008: Reject Flow — RETURNED → Edit → Resubmit

**Aktor**: risk.officer1 (maker), ctl.reviewer1 (reviewer/penolak)

**Langkah-langkah**:

1. `risk.officer1` membuat record PD baru untuk rating `idA-`, periode `2027-01-01`, PD 12m = `0.005`.
2. Submit untuk review.
3. `ctl.reviewer1` membuka record di Antrian Review.
4. Klik **Tolak**.
   - Dialog konfirmasi muncul.
   - Isi komentar alasan penolakan (minimal 10 karakter): `"Nilai PD tidak sesuai dengan data terbaru Pefindo Q2 2026 — perlu direvisi."`
   - Klik **Tolak (Konfirmasi)**.
5. Status record berubah menjadi **DIKEMBALIKAN** (RETURNED).
6. Toast hijau: `"Record PD idA- telah dikembalikan ke pembuat dengan komentar reviewer."`

7. `risk.officer1` membuka record di daftar filter `DIKEMBALIKAN`.
8. Klik **Edit**.
9. Ubah PD 12 Bulan menjadi `0.0045`.
10. Klik **Simpan**.
11. Klik **Submit ulang untuk Review**.
12. Status kembali menjadi **MENUNGGU REVIEW**.

**Hasil yang Diharapkan**:

- Record dapat diedit dalam status RETURNED (IsEditable=true).
- Setelah resubmit, workflow_instance bergerak ke PENDING_REVIEW dengan row_version baru.
- Komentar reject tersimpan dalam history (GET /:id/history).

**Pemeriksaan Audit**:

```sql
SELECT action, event_time, actor_user_id
FROM aud.audit_log
WHERE entity_type = 'mst.pd_pefindo'
  AND action IN ('PD_PEFINDO.REJECT','PD_PEFINDO.UPDATE','PD_PEFINDO.SUBMIT')
ORDER BY event_time;
-- Expected: REJECT oleh ctl.reviewer1, UPDATE oleh risk.officer1, SUBMIT oleh risk.officer1
```

**Rollback**:

```sql
DELETE FROM mst.pd_pefindo WHERE rating = 'idA-' AND periode_berlaku_dari = '2027-01-01';
```

---

### TC-009: Export CSV (XLSX 501)

**Aktor**: risk.officer1 (ROLE-RISK)

**Langkah-langkah**:

**Sub-skenario A — Export CSV**:

1. Login sebagai `risk.officer1`.
2. Navigasi ke **Master Data → Kalibrasi PD Pefindo**.
3. Terapkan filter: **Status** = `APPROVED`.
4. Klik tombol **Export ▾ → CSV**.

**Hasil yang Diharapkan**:

- Browser mengunduh file `pd-pefindo-YYYYMMDD.csv`.
- File berformat UTF-8 with BOM (terbuka dengan benar di Excel Indonesia).
- Baris header: `ID,Rating,PD 12 Bulan,PD Lifetime 3Y,...,Status,Dibuat Oleh,Tanggal Dibuat`
- Hanya baris dengan `workflow_status = APPROVED` yang ada di file (filter dihormati).
- Header `X-Total-Rows` sesuai jumlah baris data.
- Audit log: `PD_PEFINDO.EXPORT` tercatat dengan `filters.workflow_status = APPROVED`.

**Sub-skenario B — Export XLSX (501 — belum diimplementasi)**:

5. Klik tombol **Export ▾ → XLSX**.

**Hasil yang Diharapkan**:

- Pesan error (atau tombol disabled dengan tooltip): `"Export format XLSX belum tersedia. Gunakan format CSV."`
- HTTP 400 dari API endpoint `/export?format=xlsx`.
- Tidak ada file terdownload.

**Pemeriksaan Audit (DB)**:

```sql
SELECT action, event_time, after_value::text
FROM aud.audit_log
WHERE action = 'PD_PEFINDO.EXPORT'
ORDER BY event_time DESC LIMIT 1;
-- Expected: after_value berisi format=csv, filters yang diterapkan
```

**Rollback**: Tidak diperlukan.

---

## Matriks Cakupan AC

| Test Case | Acceptance Criteria | Layer |
|-----------|---------------------|-------|
| TC-001    | XLSX upload async → 202 + jobId, progress panel, completed result | E2E |
| TC-002    | 6-eyes DRAFT→APPROVED, audit trail, signature count=4 | E2E |
| TC-003    | Monotonicity inline warning (frontend validasi) | E2E |
| TC-004    | idD PD=1.0 semua horizon diterima | E2E |
| TC-005    | Step-up MFA pada approve+approve2, `signature_method=JWT_STEP_UP` | E2E |
| TC-006    | SoD approver2 ≠ approver1/reviewer/maker → 403 SOD_VIOLATION | E2E + Integration |
| TC-007    | Period overlap same rating → 422 PD_PERIOD_OVERLAP | Integration |
| TC-008    | Reject → RETURNED → edit → resubmit cycle | E2E |
| TC-009A   | Export CSV respects filter, audit EXPORT event | E2E + Integration |
| TC-009B   | Export XLSX → 400 (not implemented) | Integration |

---

## Checklist Audit Setelah Semua TC

Jalankan query berikut setelah seluruh TC selesai:

```sql
-- 1. Hash chain masih valid (tidak ada baris aud.audit_log yang dimodifikasi)
-- Jalankan: go run ./cmd/audit-verify --entity-type mst.pd_pefindo --range "2026-06-01:2026-06-30"
-- Expected output: "Hash chain OK — 0 violations"

-- 2. Semua upload job memiliki status akhir (tidak ada yang tertinggal di 'queued'/'running')
SELECT status, COUNT(*) FROM sys.job
WHERE type = 'PD_PEFINDO_UPLOAD_XLSX' GROUP BY status;
-- Expected: hanya 'completed' atau 'failed', tidak ada 'queued'/'running' sisa

-- 3. Tidak ada PD record yang approved tapi terindikasi editable
SELECT id, rating, workflow_status FROM mst.pd_pefindo
WHERE workflow_status = 'APPROVED' AND updated_at > now() - interval '1 hour';
-- Spot check: pastikan updated_at tidak berubah setelah APPROVED (tidak diedit)

-- 4. SoD tidak bisa dibypass via API langsung
-- Covered oleh integration test: TestPDPefindo_SoDViolation_Approver2NotPrevious
```

---

## Rollback / Cleanup Global (Setelah UAT)

```sql
-- Hapus semua test data yang dibuat selama UAT pd_pefindo
-- PERINGATAN: Hanya jalankan di environment UAT, TIDAK di production

BEGIN;

-- Hapus signatures dulu
DELETE FROM sys.workflow_signature WHERE workflow_instance_id IN (
  SELECT wi.id FROM sys.workflow_instance wi
  JOIN mst.pd_pefindo pd ON pd.id = wi.entity_id
  WHERE wi.entity_type = 'PD_PEFINDO'
    AND pd.created_by IN (
      SELECT id FROM sec.user WHERE username LIKE '%uat%' OR username LIKE 'risk.officer%' OR username LIKE 'ctl.reviewer%'
    )
);

-- Hapus workflow instances
DELETE FROM sys.workflow_instance WHERE entity_type = 'PD_PEFINDO'
  AND entity_id IN (
    SELECT id FROM mst.pd_pefindo WHERE created_by IN (
      SELECT id FROM sec.user WHERE username LIKE '%uat%' OR username LIKE 'risk.officer%'
    )
  );

-- Hapus pd_pefindo records (test data)
DELETE FROM mst.pd_pefindo WHERE created_by IN (
  SELECT id FROM sec.user WHERE username IN (
    'risk.officer1', 'ctl.reviewer1', 'alco.approver1', 'alco.approver2'
  )
);

-- Hapus upload jobs test
DELETE FROM sys.job WHERE type = 'PD_PEFINDO_UPLOAD_XLSX'
  AND created_by IN (
    SELECT id FROM sec.user WHERE username = 'risk.officer1'
  );

ROLLBACK; -- Ubah menjadi COMMIT setelah verifikasi isi query
```

---

## Catatan Eksekusi UAT

- **Environment**: UAT (bukan production)
- **Browser yang diuji**: Chrome 124+, Firefox 126+
- **Resolusi layar minimal**: 1280 x 800
- **Bahasa UI**: Bahasa Indonesia
- **Estimasi durasi**: 2–3 jam (satu siklus penuh)
- **Pre-requisite MFA**: `alco.approver1` dan `alco.approver2` wajib setup TOTP/WebAuthn di Keycloak sebelum TC-002/TC-005
