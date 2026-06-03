# UAT Script — APP-A-MSTR-002: Master Mata Uang
**ID UAT**: UAT-APP-A-MSTR-002-001
**Story**: APP-A-MSTR-002 Master Mata Uang (Pilot Pola Generik)
**Modul**: APP-A Master Data Management
**Tanggal**: 2026-06-03
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Database sudah dimigrasikan: `go run ./cmd/migrator up`
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`

### Seed Data Minimal (jalankan SQL berikut sebelum UAT)

```sql
-- Persona test (gunakan Keycloak admin untuk assign role)
-- akun.maker      → ROLE-AKUN (bisa create, tidak bisa approve)
-- akun.ctl.1      → ROLE-AKUN-CTL (bisa review/approve; BUKAN pembuat GBP)
-- akun.ctl.2      → ROLE-AKUN-CTL (untuk test SoD jika ctl.1 adalah maker)
-- audit.viewer    → ROLE-AUDIT (read-only)

-- Seed mata uang awal
INSERT INTO mst.mata_uang (
    kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
    sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
    is_system_currency, workflow_status,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES
    ('IDR', gen_random_uuid(), 'Rupiah Indonesia',   'Rp', 0, 'BI_JISDOR',      'HARIAN',  true,  '2020-01-01', true,  'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('USD', gen_random_uuid(), 'Dolar Amerika',      '$',  2, 'BI_JISDOR',      'HARIAN',  true,  '2020-01-01', false, 'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('EUR', gen_random_uuid(), 'Euro',               '€',  2, 'BI_KURS_TENGAH', 'HARIAN',  true,  '2020-01-01', false, 'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('SGD', gen_random_uuid(), 'Dolar Singapura',    'S$', 2, 'BI_KURS_TENGAH', 'HARIAN',  true,  '2020-01-01', false, 'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('JPY', gen_random_uuid(), 'Yen Jepang',         '¥',  0, 'BI_KURS_TENGAH', 'HARIAN',  true,  '2020-01-01', false, 'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('CHF', gen_random_uuid(), 'Franc Swiss',        'Fr', 2, 'INTERNAL',       'BULANAN', false, '2020-01-01', false, 'APPROVED', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'),
    ('XYZ', gen_random_uuid(), 'Test Currency',      'X',  2, 'INTERNAL',       'BULANAN', true,  '2026-06-03', false, 'DRAFT',    now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE')
ON CONFLICT (kode_mata_uang) DO NOTHING;
```

---

## SKENARIO UAT

---

### S-001: Happy Path — Create Mata Uang Baru (GBP)

**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: GBP belum ada di database

**Langkah-langkah**:

1. Login sebagai `akun.maker` di `http://localhost:3001`
2. Navigasi ke **Master Data > Mata Uang** (`/master/mata-uang`)
3. Klik tombol **"+ Tambah Mata Uang"**
4. Isi form:
   - **Kode Mata Uang**: `GBP`
   - **Nama**: `Pound Sterling`
   - **Simbol**: `£`
   - **Decimal Places**: `2`
   - **Sumber Kurs**: pilih `BI_KURS_TENGAH`
   - **Frekuensi Update**: pilih `HARIAN`
   - **Tgl Mulai Aktif**: `2026-06-03`
5. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 diterima dari API | |
| 2 | Response body mengandung `"kodeMataUang": "GBP"` | |
| 3 | Response mengandung `"workflowStatus": "DRAFT"` | |
| 4 | Toast sukses hijau tampil: *"Mata uang GBP — Pound Sterling berhasil dibuat. Menunggu review Finance Controller."* | |
| 5 | Toast mengandung link "Lihat detail" | |
| 6 | Halaman redirect ke `/master/mata-uang/GBP` | |
| 7 | `aud.audit_log` memiliki baris: `action='MATA_UANG.CREATE'`, `entity_type='mst.mata_uang'` | |
| 8 | GBP muncul di list dengan status **DRAFT** | |

**Verifikasi Audit SQL**:
```sql
SELECT action, actor_role, entity_type
FROM aud.audit_log
WHERE action = 'MATA_UANG.CREATE'
  AND after_value::text LIKE '%GBP%'
ORDER BY event_time DESC LIMIT 1;
-- Expected: 1 row dengan action='MATA_UANG.CREATE'
```

---

### S-002: Validation Error — Kode Tidak ISO 4217

**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: Halaman form create terbuka

**Langkah-langkah**:

1. Di form create, isi **Kode Mata Uang** dengan `RUPIAH` (6 karakter)
2. Isi field lain dengan nilai valid
3. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 400 VALIDATION_FAILED diterima | |
| 2 | Field "Kode Mata Uang" di-highlight merah | |
| 3 | Pesan inline: *"Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)"* | |
| 4 | Toast error merah **persistent** (tidak auto-close) | |
| 5 | Toast mengandung traceId | |
| 6 | Tidak ada record dibuat di database | |
| 7 | Tombol "Simpan" aktif kembali (bukan terus-terusan disabled) | |

**Verifikasi**: Ulangi dengan kode lowercase `gbp` — harus gagal dengan error yang sama.

---

### S-003: Validation Error — Kode Sudah Ada (Duplicate)

**Aktor**: akun.maker (ROLE-AKUN)
**Pre-condition**: `USD` sudah ada di database

**Langkah-langkah**:

1. Di form create, isi **Kode Mata Uang** dengan `USD`
2. Isi field lain dengan nilai valid
3. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 409 CONFLICT diterima dengan `"error.code": "CONFLICT"` | |
| 2 | Pesan: *"Mata uang USD sudah terdaftar di sistem."* | |
| 3 | Tidak ada record duplikat dibuat | |

---

### S-004: Idempotency Replay — Request Duplikat

**Aktor**: akun.maker (ROLE-AKUN)
**Tool**: curl atau Postman (untuk mengontrol Idempotency-Key secara eksak)

**Langkah-langkah**:

1. Buat mata uang `TST` dengan Idempotency-Key baru (mis. `550e8400-e29b-41d4-a716-446655440001`):
```bash
curl -X POST http://localhost:8080/api/v1/master/mata-uang \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_akun_maker>" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440001" \
  -d '{"kodeMataUang":"TST","namaMataUang":"Test Dollar","simbol":"T$","decimalPlaces":2,"sumberKursDefault":"INTERNAL","frekuensiUpdate":"BULANAN","tanggalMulaiAktif":"2026-06-03"}'
```
2. Catat response pertama (HTTP 201)
3. Kirim ulang **request identik** dengan **Idempotency-Key yang sama**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Request pertama: HTTP 201 | |
| 2 | Request kedua (replay): **HTTP 201 response identik** (bukan HTTP 409) | |
| 3 | Handler hanya dipanggil **1 kali** (tidak ada side effect duplikat) | |
| 4 | Database hanya memiliki **1 record** TST | |
| 5 | Audit log hanya memiliki **1 event** MATA_UANG.CREATE untuk TST | |

**Verifikasi SQL**:
```sql
SELECT COUNT(*) FROM mst.mata_uang WHERE kode_mata_uang = 'TST';
-- Expected: 1

SELECT COUNT(*) FROM aud.audit_log WHERE action = 'MATA_UANG.CREATE'
  AND after_value::text LIKE '%TST%';
-- Expected: 1
```

---

### S-005: Idempotency Mismatch — Key Sama, Payload Berbeda

**Aktor**: akun.maker
**Pre-condition**: Request create TST dengan key `550e8400-e29b-41d4-a716-446655440001` sudah berhasil

**Langkah-langkah**:

1. Kirim request dengan **key yang sama** tapi **nama berbeda** (`Test Dollar 2`):
```bash
curl -X POST http://localhost:8080/api/v1/master/mata-uang \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440001" \
  -d '{"kodeMataUang":"TST","namaMataUang":"Test Dollar 2","simbol":"T$","decimalPlaces":2,"sumberKursDefault":"INTERNAL","frekuensiUpdate":"BULANAN","tanggalMulaiAktif":"2026-06-03"}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 IDEMPOTENCY_MISMATCH diterima | |
| 2 | `"error.code": "IDEMPOTENCY_MISMATCH"` | |
| 3 | Pesan menjelaskan konflik idempotency | |
| 4 | Nama TST di database tetap `"Test Dollar"` (tidak berubah) | |

---

### S-006: Optimistic Lock — Dua User Edit Bersamaan

**Aktor**: akun.maker (User A) dan akun.ctl.1 (User B) — keduanya membuka form edit GBP
**Pre-condition**: GBP ada di database dengan `row_version = 1` dan status DRAFT

**Langkah-langkah**:

1. **User A** dan **User B** membuka halaman edit GBP secara bersamaan. Keduanya melihat `rowVersion: 1`.
2. **User A** mengubah nama menjadi `"British Pound Sterling"` dan klik **Simpan** (berhasil, `row_version` menjadi 2).
3. **User B** mengubah simbol menjadi `"£ GB"` dan klik **Simpan** (masih menggunakan `rowVersion: 1` yang stale).

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | User A: HTTP 200 berhasil, `row_version` di response = 2 | |
| 2 | User B: HTTP 409 CONFLICT diterima | |
| 3 | Toast error User B: *"Mata uang GBP telah diubah oleh user lain. Muat ulang halaman untuk melihat data terbaru."* | |
| 4 | Database mengandung nama `"British Pound Sterling"` (bukan simbol perubahan User B) | |

**Verifikasi SQL**:
```sql
SELECT nama_mata_uang, row_version FROM mst.mata_uang WHERE kode_mata_uang = 'GBP';
-- Expected: nama='British Pound Sterling', row_version=2
```

---

### S-007: Workflow 4-Eyes — Happy Path (DRAFT → PENDING_REVIEW → APPROVED)

**Aktor**: akun.maker, akun.ctl.1
**Pre-condition**: GBP ada dengan status DRAFT, dibuat oleh akun.maker

**Bagian A: Maker Submit**

1. Login sebagai `akun.maker`
2. Buka halaman detail `/master/mata-uang/GBP`
3. Klik **"Kirim untuk Review"**
4. Konfirmasi dialog (jika ada)

**Hasil yang Diharapkan — Bagian A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status GBP berubah menjadi **PENDING_REVIEW** | |
| 2 | Timestamp `submitted_at` tersimpan di workflow_instance | |
| 3 | Audit log: `action='MATA_UANG.SUBMIT'` | |
| 4 | Toast sukses: *"GBP berhasil dikirim untuk review Finance Controller."* | |
| 5 | Tombol "Kirim untuk Review" tidak lagi tampil | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.mata_uang WHERE kode_mata_uang = 'GBP');
-- Expected: 'PENDING_REVIEW'
```

**Bagian B: Finance Controller Review dan Approve**

1. Login sebagai `akun.ctl.1` (bukan maker GBP — SoD terpenuhi)
2. Navigasi ke queue review atau buka `/master/mata-uang/GBP` langsung
3. Panel workflow menampilkan **Step 2: Pemeriksa (Reviewer)** sebagai aktif
4. Isi komentar: `"Review OK — kode ISO valid, decimal places sesuai standar"`
5. Klik **"Setujui"**

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status GBP berubah menjadi **APPROVED** | |
| 2 | Audit log: `action='MATA_UANG.APPROVE'` dengan komentar tersimpan | |
| 3 | `aktif_flag` tetap `TRUE` | |
| 4 | Toast sukses akun.ctl.1: *"Mata uang GBP berhasil disetujui."* | |
| 5 | Panel workflow menampilkan semua step sebagai "done" (tanda centang hijau) | |
| 6 | Tombol Edit tidak tampil (APPROVED = read-only) | |

**Verifikasi SQL**:
```sql
-- Workflow state
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.mata_uang WHERE kode_mata_uang = 'GBP');
-- Expected: 'APPROVED'

-- Audit trail
SELECT action, actor_user_id FROM aud.audit_log
WHERE action IN ('MATA_UANG.SUBMIT', 'MATA_UANG.APPROVE')
  AND after_value::text LIKE '%GBP%'
ORDER BY event_time;
-- Expected: 2 rows

-- Signatures
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = (SELECT id FROM mst.mata_uang WHERE kode_mata_uang = 'GBP');
-- Expected: >= 1 (signature record)
```

---

### S-008: SoD Violation — Maker Tidak Bisa Approve

**Aktor**: akun.maker (sama sebagai maker GBP)
**Pre-condition**: GBP dalam status PENDING_REVIEW atau PENDING_APPROVAL

**Langkah via UI**:

1. Login sebagai `akun.maker` (yang membuat GBP)
2. Buka `/master/mata-uang/GBP`
3. Pada Step 2 (Reviewer), panel harus menampilkan **SodBlockBanner**: *"Anda adalah pembuat data ini dan tidak bisa menjadi reviewer."*
4. Tombol "Setujui" dan "Tolak" di Step 2 harus **disabled** atau tidak tampil

**Langkah via API (bypass test)**:

```bash
curl -X POST http://localhost:8080/api/v1/master/mata-uang/GBP/review \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_akun_maker>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":2}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: SodBlockBanner tampil dengan pesan yang sesuai | |
| 2 | UI: Tombol aksi workflow disabled untuk user maker | |
| 3 | API: HTTP 403 SOD_VIOLATION diterima | |
| 4 | API: `"error.code": "SOD_VIOLATION"` | |
| 5 | Status GBP tetap TIDAK BERUBAH (masih PENDING_REVIEW) | |

---

### S-009: SoD Violation — AKUN-CTL yang Membuat Tidak Bisa Approve

**Skenario**: akun.ctl.2 membuat mata uang baru `AUD`, lalu mencoba approve-nya sendiri.
**Pre-condition**: akun.ctl.2 memiliki permission `mata_uang.create` DAN `mata_uang.approve`

**Langkah-langkah**:

1. Login sebagai `akun.ctl.2`
2. Buat mata uang `AUD` (Dolar Australia) — berhasil, status DRAFT
3. Submit AUD untuk review
4. Coba approve AUD langsung via API:
```bash
curl -X POST http://localhost:8080/api/v1/master/mata-uang/AUD/approve \
  -H "Authorization: Bearer <token_akun_ctl_2>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":3}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 SOD_VIOLATION diterima | |
| 2 | Status AUD tetap **tidak berubah** | |
| 3 | Tidak ada signature record dibuat untuk tindakan ini | |

---

### S-010: AKUN-CTL Reject — Kembalikan ke Maker

**Aktor**: akun.ctl.1 (ROLE-AKUN-CTL, bukan maker)
**Pre-condition**: GBP dalam status PENDING_REVIEW

**Langkah-langkah**:

1. Login sebagai `akun.ctl.1`
2. Buka `/master/mata-uang/GBP`
3. Klik **"Tolak"** pada panel workflow Step 2
4. Dialog konfirmasi muncul dengan input "Alasan Penolakan" (required)
5. Isi alasan: `"Kode mata uang GBP sudah ada di sistem dengan variasi yang berbeda. Harap verifikasi."`
6. Klik **"Lanjut Tolak"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status GBP berubah menjadi **RETURNED** (DB value: REJECTED) | |
| 2 | Komentar penolakan tersimpan di workflow_signature | |
| 3 | Audit log: `action='MATA_UANG.REJECT'` dengan komentar | |
| 4 | Banner **RETURNED** tampil di halaman detail GBP dengan alasan penolakan | |
| 5 | Tombol **"Edit"** tersedia lagi untuk akun.maker | |
| 6 | akun.maker bisa edit dan re-submit | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.mata_uang WHERE kode_mata_uang = 'GBP');
-- Expected: 'REJECTED' (ditampilkan sebagai 'RETURNED' di UI)
```

---

### S-011: Protect System Currency IDR — Tidak Bisa Dihapus

**Aktor**: akun.maker (ROLE-AKUN) atau akun.ctl.1
**Pre-condition**: IDR ada di database dengan `is_system_currency = true`

**Langkah via UI**:

1. Buka `/master/mata-uang`
2. Cari IDR di tabel
3. Klik tombol **"Hapus"** pada row IDR (jika tersedia)

**Langkah via API**:

```bash
curl -X DELETE http://localhost:8080/api/v1/master/mata-uang/IDR \
  -H "Authorization: Bearer <token>" \
  -H "Idempotency-Key: $(uuidgen)"
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 FORBIDDEN diterima | |
| 2 | `"error.code": "SYSTEM_CURRENCY_PROTECTED"` | |
| 3 | Pesan: *"Mata uang IDR adalah currency fungsional Tugure dan tidak bisa dihapus."* | |
| 4 | IDR tetap ada di database (`deleted_at IS NULL`) | |

**Verifikasi SQL**:
```sql
SELECT deleted_at FROM mst.mata_uang WHERE kode_mata_uang = 'IDR';
-- Expected: NULL (tidak di-delete)
```

---

### S-012: Soft Delete — Mata Uang DRAFT Tanpa Referensi

**Aktor**: akun.maker
**Pre-condition**: XYZ ada di database dengan status DRAFT, `is_system_currency = false`, tidak direferensikan instrumen

**Langkah-langkah**:

1. Buka `/master/mata-uang`
2. Cari XYZ di tabel
3. Klik **"Hapus"** pada row XYZ
4. Dialog konfirmasi muncul: *"Yakin ingin menghapus mata uang XYZ — Test Currency?"*
5. Klik **"Hapus"** di dialog

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 200 dengan `"deleted": true` | |
| 2 | XYZ tidak muncul di list default | |
| 3 | `deleted_at` dan `deleted_by` tersimpan di database | |
| 4 | Toast sukses: *"Mata uang XYZ berhasil dihapus dari sistem."* | |
| 5 | Audit log: `action='MATA_UANG.DELETE'` dengan `before_value` lengkap | |

**Verifikasi SQL**:
```sql
SELECT deleted_at, deleted_by FROM mst.mata_uang WHERE kode_mata_uang = 'XYZ';
-- Expected: deleted_at IS NOT NULL, deleted_by IS NOT NULL

SELECT action FROM aud.audit_log
WHERE action = 'MATA_UANG.DELETE'
  AND before_value::text LIKE '%XYZ%';
-- Expected: 1 row
```

---

### S-013: Tidak Bisa Hapus Mata Uang yang Direferensikan

**Aktor**: akun.maker
**Pre-condition**: USD direferensikan oleh setidaknya 1 instrumen aktif

**Setup** (jika instrumen belum ada):
```sql
INSERT INTO mst.instrumen (id, kode_instrumen, mata_uang, deleted_at, created_by, updated_by, tenant_id)
VALUES (gen_random_uuid(), 'TEST-INSTR-001', 'USD', NULL, '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'TUGURE');
```

**Langkah-langkah**:

1. Coba hapus USD melalui UI atau API
2. Konfirmasi di dialog konfirmasi (jika dialog muncul)

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 409 CONFLICT diterima | |
| 2 | `"error.code": "ENTITY_IN_USE"` | |
| 3 | Pesan mengandung jumlah referensi: *"...masih digunakan oleh N entitas..."* | |
| 4 | USD tetap ada di database (`deleted_at IS NULL`) | |

---

### S-014: Export CSV — Filter Aktif Dihormati

**Aktor**: akun.maker (permission mata_uang.read)

**Langkah-langkah**:

1. Buka `/master/mata-uang`
2. Aktifkan filter **"Status = Aktif"** (aktif_flag = true)
3. Perhatikan jumlah record di tabel (seharusnya IDR, USD, EUR, SGD, JPY, TST)
4. Klik **"Export" → "CSV"**
5. Download file

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | File ter-download dengan nama format `mata-uang-YYYYMMDD.csv` | |
| 2 | File hanya mengandung mata uang **aktif** (CHF yang non-aktif TIDAK ada) | |
| 3 | Header row: `Kode,Nama,Simbol,Decimal Places,Sumber Kurs,Frekuensi,Status,Tgl Mulai Aktif,Workflow Status` | |
| 4 | File encoding UTF-8 with BOM (buka di Excel — tampil tanpa garbled characters) | |
| 5 | `Content-Disposition` header: `attachment; filename="mata-uang-YYYYMMDD.csv"` | |
| 6 | `X-Total-Rows` header sesuai jumlah record | |
| 7 | Audit log: `action='MATA_UANG.EXPORT'` dengan `format='csv'` dan `row_count` yang benar | |

**Verifikasi Audit SQL**:
```sql
SELECT after_value FROM aud.audit_log
WHERE action = 'MATA_UANG.EXPORT'
ORDER BY event_time DESC LIMIT 1;
-- Expected: {format: 'csv', row_count: N, filters: {aktif_flag: 'true'}}
```

---

### S-015: List — Sort + Filter + Pagination

**Aktor**: akun.maker
**Pre-condition**: Ada > 5 record mata uang

**Test A: Sort**
1. Buka `/master/mata-uang`
2. Klik header kolom **"Nama"** → sort ASC
3. Verifikasi: data urut alphabetical A-Z, URL: `?sort=nama_mata_uang:asc`
4. Klik lagi header **"Nama"** → sort DESC
5. Verifikasi: data urut Z-A, URL: `?sort=nama_mata_uang:desc`

**Test B: Filter**
1. Aktifkan filter `Status = Aktif`
2. Verifikasi: filter chip "Status: Aktif" muncul
3. URL: `?filter[aktif_flag]=true`
4. Klik **"Hapus semua filter"**
5. Verifikasi: semua record tampil kembali

**Test C: Text Search**
1. Ketik `Dollar` di search box
2. Verifikasi: hanya tampil record dengan "Dollar" di nama (USD, SGD)
3. URL: `?q=Dollar`
4. Hapus search
5. Verifikasi: semua record tampil

**Test D: Pagination** (jika ada > 50 record)
1. Default limit 50
2. Klik **"Next"**
3. Verifikasi: halaman berikutnya dimuat, cursor di URL berubah

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Sort berfungsi, ikon panah tampil di header aktif | |
| 2 | Filter chip tampil, filter respects backend | |
| 3 | Search memfilter data, URL di-update | |
| 4 | State URL-encoded (deep-link friendly) | |

---

### S-016: ROLE-AUDIT — Read-Only, Bisa Lihat Deleted

**Aktor**: audit.viewer (ROLE-AUDIT)

**Langkah-langkah**:

1. Login sebagai `audit.viewer`
2. Buka `/master/mata-uang`
3. Verifikasi UI read-only
4. Akses `/master/mata-uang?include_deleted=true`
5. Buka `/master/mata-uang/XYZ/history`

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Tidak ada tombol **"+ Tambah Mata Uang"** | |
| 2 | Tidak ada tombol **"Edit"** atau **"Hapus"** di kolom aksi | |
| 3 | Tombol **"Export"** tetap tersedia | |
| 4 | `?include_deleted=true` menampilkan XYZ dengan badge **"Dihapus"** | |
| 5 | `/history` menampilkan audit trail GBP dengan `before_value` dan `after_value` (ROLE-AUDIT dapat lihat detail) | |

---

### S-017: MFA Check — AKUN-CTL Tanpa MFA Ditolak

**Setup**: Buat JWT untuk akun.ctl.1 dengan `mfa_verified: false`
**Langkah via API**:

```bash
# Gunakan JWT yang dibuat manual tanpa MFA verification
curl -X POST http://localhost:8080/api/v1/master/mata-uang/GBP/approve \
  -H "Authorization: Bearer <token_tanpa_mfa>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":2}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 FORBIDDEN diterima | |
| 2 | Pesan MFA required | |
| 3 | Status GBP tidak berubah | |

---

## Cleanup / Rollback

Setelah UAT selesai, jalankan:

```sql
-- Hapus data test (bukan IDR, USD, EUR, SGD, JPY yang mungkin dibutuhkan)
DELETE FROM mst.mata_uang WHERE kode_mata_uang IN ('GBP', 'TST', 'AUD', 'XYZ');
-- Atau gunakan soft-delete:
UPDATE mst.mata_uang SET deleted_at = now()
WHERE kode_mata_uang IN ('GBP', 'TST', 'AUD')
  AND is_system_currency = false;

-- Hapus workflow instances terkait
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
    SELECT id FROM mst.mata_uang
    WHERE kode_mata_uang IN ('GBP', 'TST', 'AUD')
);

-- Hapus audit log test (HANYA jika env=UAT, bukan production)
-- Di production: audit log tidak boleh dihapus per DEC-018 (10+10 tahun retention)
```

---

## Checklist Ringkas QA Gate Phase 3 → 4

| Gate | Status |
|---|---|
| S-001 Happy path create | |
| S-002 Validation kode format | |
| S-003 Duplicate kode 409 | |
| S-004 Idempotency replay | |
| S-005 Idempotency mismatch | |
| S-006 Optimistic lock 409 | |
| S-007 4-eyes workflow full cycle | |
| S-008 SoD maker tidak bisa approve | |
| S-009 SoD AKUN-CTL-as-maker tidak bisa approve | |
| S-010 Reject → RETURNED + re-submit | |
| S-011 System currency IDR protected 403 | |
| S-012 Soft delete tanpa referensi | |
| S-013 Delete ditolak jika ada referensi 409 | |
| S-014 Export CSV respects filter | |
| S-015 Sort + filter + pagination | |
| S-016 ROLE-AUDIT read-only + include_deleted | |
| S-017 MFA check untuk AKUN-CTL | |

**SEMUA S-001 s.d. S-017 harus PASS sebelum Phase 4 dimulai.**
