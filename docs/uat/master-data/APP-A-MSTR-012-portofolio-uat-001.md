# UAT Script — APP-A-MSTR-012: Master Portofolio
**ID UAT**: UAT-APP-A-MSTR-012-001
**Story**: APP-A-MSTR-012 Master Portofolio (4-eyes workflow, PSAK 71 BM Category)
**Modul**: APP-A Master Data Management
**Tanggal**: 2026-06-04
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

### Seed Data Minimal

Jalankan SQL berikut di psql sebelum memulai UAT:

```sql
-- Persona test (assign role di Keycloak admin setelah insert)
-- Gunakan tiga user BERBEDA untuk menjamin SoD

-- User 1: Treasury Maker (membuat portofolio)
INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES (
  'aaaa0001-0000-0000-0000-000000000001',
  'uat.pt.maker',
  'uat.pt.maker@tugu-re.com',
  'UAT Portofolio Maker',
  'AKTIF', now(),
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (username) DO NOTHING;

-- User 2: Risk Officer (reviewer SPPI/BM)
INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES (
  'aaaa0002-0000-0000-0000-000000000001',
  'uat.pt.reviewer',
  'uat.pt.reviewer@tugu-re.com',
  'UAT Portofolio Reviewer',
  'AKTIF', now(),
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (username) DO NOTHING;

-- User 3: Treasury Approver
INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES (
  'aaaa0003-0000-0000-0000-000000000001',
  'uat.pt.approver',
  'uat.pt.approver@tugu-re.com',
  'UAT Portofolio Approver',
  'AKTIF', now(),
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (username) DO NOTHING;

-- Seed portofolio awal (untuk S-002 validasi duplikat)
INSERT INTO mst.portofolio (
    id, kode_portofolio, nama, bm_category_default, aktif_flag,
    workflow_status, created_at, created_by, updated_at, updated_by,
    version, tenant_id, is_deleted
) VALUES (
    gen_random_uuid(),
    'EKUITAS_EXIST',
    'Portofolio Ekuitas Existing (Seed)',
    'HTC', true,
    'APPROVED',
    now(), '00000000-0000-0000-0000-000000000001',
    now(), '00000000-0000-0000-0000-000000000001',
    1, 'TUGURE', false
) ON CONFLICT (kode_portofolio) DO NOTHING;
```

### Keycloak Role Assignment
Assign role di Keycloak Admin Console (`http://localhost:8090/admin`):

| Username | Role | Permission kunci |
|---|---|---|
| `uat.pt.maker` | `ROLE-MAKER-TR` | `portofolio.create`, `portofolio.read`, `portofolio.update`, `portofolio.delete`, `portofolio.submit` |
| `uat.pt.reviewer` | `ROLE-RISK` | `portofolio.read`, `portofolio.review` |
| `uat.pt.approver` | `ROLE-APPR-TR` | `portofolio.read`, `portofolio.review`, `portofolio.approve`, `portofolio.reject` |

---

## SKENARIO UAT

---

### S-001: Happy Path — Create Portofolio Baru (HTC Bond)

**Aktor**: `uat.pt.maker` (ROLE-MAKER-TR)
**Pre-condition**: Kode `BOND_HTC_2026` belum ada di database

**Langkah-langkah**:

1. Login sebagai `uat.pt.maker` di `http://localhost:3001`
2. Navigasi ke **Master Data > Portofolio** (`/master/portofolio`)
3. Klik tombol **"+ Tambah Portofolio"**
4. Isi form:
   - **Kode Portofolio**: `BOND_HTC_2026`
   - **Nama**: `Portofolio Obligasi HTC 2026`
   - **BM Category Default**: pilih `HTC` (Hold-to-Collect)
   - **Tujuan Pengelolaan**: `Koleksi arus kas kontraktual obligasi korporasi`
   - **Benchmark**: `IndoBeX`
   - **Aktif**: centang `true`
5. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 dari API | |
| 2 | Response body mengandung `"kodePortofolio": "BOND_HTC_2026"` | |
| 3 | Response mengandung `"workflowStatus": "DRAFT"` | |
| 4 | Response mengandung `"bmCategoryDefault": "HTC"` | |
| 5 | Toast sukses hijau tampil: *"Portofolio BOND_HTC_2026 berhasil dibuat. Menunggu review."* | |
| 6 | Toast mengandung link "Lihat detail" yang mengarah ke `/master/portofolio/BOND_HTC_2026` | |
| 7 | Record muncul di tabel list dengan status **DRAFT** | |

**Audit Check**:
```sql
SELECT action, actor_role, after_value
FROM aud.audit_log
WHERE action = 'PORTOFOLIO.CREATE'
  AND after_value::text LIKE '%BOND_HTC_2026%'
ORDER BY event_time DESC LIMIT 1;
```
Ekspektasi: 1 row dengan `action = 'PORTOFOLIO.CREATE'` dan `actor_role = 'ROLE-MAKER-TR'`.

**Cleanup**:
```sql
DELETE FROM mst.portofolio WHERE kode_portofolio = 'BOND_HTC_2026';
```

---

### S-002: Validasi Duplikat Kode — Conflict 409

**Aktor**: `uat.pt.maker` (ROLE-MAKER-TR)
**Pre-condition**: Kode `EKUITAS_EXIST` sudah ada (di-seed di pre-condition)

**Langkah-langkah**:

1. Login sebagai `uat.pt.maker`
2. Navigasi ke **Master Data > Portofolio**
3. Klik **"+ Tambah Portofolio"**
4. Isi form dengan kode yang sama:
   - **Kode Portofolio**: `EKUITAS_EXIST`
   - **Nama**: `Duplikat Portofolio Ekuitas`
   - **BM Category Default**: `HTC`
5. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 409 dari API | |
| 2 | Error code `PORTOFOLIO_DUPLICATE_KODE` atau `CONFLICT` | |
| 3 | Toast error merah **persistent** (tidak auto-dismiss) tampil | |
| 4 | Toast message spesifik: mengandung `EKUITAS_EXIST` dan "sudah terdaftar" | |
| 5 | Hanya 1 row `EKUITAS_EXIST` di database (tidak ada duplikat) | |
| 6 | Form tetap terbuka dengan data yang sudah diisi (tidak di-reset) | |

**Audit Check**:
```sql
-- Harus hanya ada 1 row EKUITAS_EXIST
SELECT COUNT(*) FROM mst.portofolio
WHERE kode_portofolio = 'EKUITAS_EXIST' AND deleted_at IS NULL;
-- Ekspektasi: 1
```

---

### S-003: Validasi Format Kode — 400/422

**Aktor**: `uat.pt.maker` (ROLE-MAKER-TR)
**Pre-condition**: Tidak diperlukan seed khusus

**Langkah-langkah**:

Ulangi sub-test berikut, masing-masing dengan kode berbeda:

| Sub | Kode | Alasan |
|---|---|---|
| 3a | `lower-case` | Huruf kecil tidak diizinkan |
| 3b | `HAS SPACE` | Spasi tidak diizinkan |
| 3c | `TOOLONGKODEXYZ_12345!` | Lebih dari 20 karakter + karakter khusus |
| 3d | *(kosong)* | Kode wajib diisi |

Untuk setiap sub-test:
1. Login sebagai `uat.pt.maker`
2. Buka form tambah portofolio
3. Isi kode sesuai tabel, nama "Test", BM = HTC
4. Klik **"Simpan"**

**Hasil yang Diharapkan** (per sub-test):

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 400 atau 422 dari API | |
| 2 | Error code `VALIDATION_FAILED` atau `PORTOFOLIO_INVALID_KODE_FORMAT` | |
| 3 | Field `kodePortofolio` di-highlight merah dengan pesan inline | |
| 4 | Toast error merah persistent muncul | |
| 5 | Tidak ada row baru di `mst.portofolio` | |

---

### S-004: Alur 4-Eyes Lengkap — DRAFT → APPROVED

**Aktor**: `uat.pt.maker`, `uat.pt.reviewer`, `uat.pt.approver` (3 user berbeda)
**Pre-condition**: Kode `BOND_HTC_4EYES` belum ada di database

**Langkah-langkah**:

#### Langkah 1 — Maker: Create + Submit

1. Login sebagai `uat.pt.maker`
2. Buat portofolio baru:
   - **Kode**: `BOND_HTC_4EYES`
   - **Nama**: `Portofolio Obligasi HTC (4-eyes UAT)`
   - **BM Category Default**: `HTC`
   - **Benchmark**: `IndoBeX Composite`
3. Klik **"Simpan"** → catat `entityId` dari response
4. Buka detail portofolio `BOND_HTC_4EYES`
5. Klik tombol **"Submit untuk Review"**
6. Isi komentar: `Portofolio baru untuk investasi obligasi rating A+`
7. Klik **"Konfirmasi Submit"**

Verifikasi Langkah 1:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah ke `PENDING_REVIEW` | |
| 2 | Toast sukses: *"Portofolio BOND_HTC_4EYES berhasil di-submit untuk review."* | |
| 3 | Tombol "Submit" tidak lagi tampil; tombol "Tarik Kembali" muncul (jika Retractable=true) | |
| 4 | `sys.workflow_instance.current_state = 'PENDING_REVIEW'` | |

#### Langkah 2 — Reviewer: Review

1. **Logout** dari `uat.pt.maker`
2. Login sebagai `uat.pt.reviewer`
3. Navigasi ke queue **Pending Review** atau ke detail `BOND_HTC_4EYES`
4. Verifikasi data portofolio (kode, nama, BM category)
5. Klik tombol **"Review — Setujui"**
6. Isi komentar: `Data sudah sesuai standar BM classification`
7. Klik **"Konfirmasi Review"**

Verifikasi Langkah 2:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah ke `PENDING_APPROVAL` | |
| 2 | Toast sukses: mengandung "BOND_HTC_4EYES" dan "menunggu persetujuan" | |
| 3 | `sys.workflow_instance.current_state = 'PENDING_APPROVAL'` | |

#### Langkah 3 — Approver: Approve

1. **Logout** dari `uat.pt.reviewer`
2. Login sebagai `uat.pt.approver`
3. Navigasi ke queue **Pending Approval** atau ke detail `BOND_HTC_4EYES`
4. Verifikasi data
5. Klik tombol **"Approve"**
6. Isi komentar: `Disetujui. Sesuai kebijakan investasi ALCO Juni 2026.`
7. Klik **"Konfirmasi Approve"**

Verifikasi Langkah 3:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah ke `APPROVED` | |
| 2 | Toast sukses: *"Portofolio BOND_HTC_4EYES berhasil disetujui."* | |
| 3 | `mst.portofolio.workflow_status = 'APPROVED'` | |
| 4 | Record tidak bisa diedit lagi (tombol Edit disabled atau 403 jika diakses via API) | |

**Audit Check**:
```sql
-- Harus ada 3 event untuk portofolio ini
SELECT action, actor_user_id, actor_role, event_time
FROM aud.audit_log
WHERE entity_type = 'mst.portofolio'
  AND action IN ('PORTOFOLIO.CREATE','PORTOFOLIO.SUBMIT','PORTOFOLIO.REVIEW','PORTOFOLIO.APPROVE')
  AND after_value::text LIKE '%BOND_HTC_4EYES%'
ORDER BY event_time;

-- Harus ada 3 signature (submit + review + approve)
SELECT s.step, s.actor_user_id, s.signed_at
FROM sys.workflow_signature s
JOIN sys.workflow_instance wi ON wi.id = s.workflow_instance_id
WHERE wi.entity_id = (
    SELECT id FROM mst.portofolio WHERE kode_portofolio = 'BOND_HTC_4EYES'
)
ORDER BY s.signed_at;
-- Ekspektasi: 3 baris (SUBMIT, REVIEW, APPROVE)
```

**Cleanup**:
```sql
DELETE FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.portofolio WHERE kode_portofolio = 'BOND_HTC_4EYES');
DELETE FROM mst.portofolio WHERE kode_portofolio = 'BOND_HTC_4EYES';
```

---

### S-005: SoD Violation — Maker Tidak Bisa Approve

**Aktor**: `uat.pt.maker` (melakukan bypass attempt)
**Pre-condition**: Ada portofolio `BOND_SOD_TEST` dalam status `PENDING_APPROVAL`
(sudah melalui langkah create + submit oleh `uat.pt.maker` + review oleh `uat.pt.reviewer`)

**Setup** (jalankan terlebih dahulu S-004 steps 1 dan 2 untuk kode `BOND_SOD_TEST`):
```sql
-- Atau seed langsung via API:
-- 1. POST /api/v1/master/portofolio  (sebagai maker) → kode BOND_SOD_TEST
-- 2. POST /api/v1/master/portofolio/BOND_SOD_TEST/submit  (sebagai maker)
-- 3. POST /api/v1/master/portofolio/BOND_SOD_TEST/review  (sebagai reviewer)
-- Sekarang status = PENDING_APPROVAL
```

**Langkah-langkah — Bypass Attempt via UI**:

1. Login sebagai `uat.pt.maker`
2. Navigasi ke detail `BOND_SOD_TEST`
3. Coba akses tombol "Approve" — seharusnya tidak ada tombol Approve untuk maker
4. Jika tombol Approve tersedia (edge case), klik dan konfirmasi

**Langkah-langkah — Bypass Attempt via API langsung**:

1. Pastikan `uat.pt.maker` memiliki token JWT aktif
2. Kirim request API secara langsung (misal via Postman/curl):
```bash
curl -X POST http://localhost:8080/api/v1/master/portofolio/BOND_SOD_TEST/approve \
  -H "Authorization: Bearer <token_maker>" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"bypass attempt"}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 dari API | |
| 2 | Error code `SOD_VIOLATION` | |
| 3 | Message mengandung kalimat tentang "maker tidak bisa menjadi approver" | |
| 4 | Status TETAP `PENDING_APPROVAL` (tidak berubah ke APPROVED) | |
| 5 | Tidak ada audit event `PORTOFOLIO.APPROVE` untuk user `uat.pt.maker` | |
| 6 | Di UI: tombol Approve tidak tersedia bagi `uat.pt.maker` untuk portofolio yang dia buat | |

**Audit Check** (memastikan bypass tidak terjadi):
```sql
-- Harus 0 baris APPROVE oleh maker untuk portofolio ini
SELECT COUNT(*) FROM aud.audit_log a
JOIN sec.user u ON u.id = a.actor_user_id
WHERE a.action = 'PORTOFOLIO.APPROVE'
  AND u.username = 'uat.pt.maker'
  AND a.after_value::text LIKE '%BOND_SOD_TEST%';
-- Ekspektasi: 0

-- Workflow harus masih PENDING_APPROVAL
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (
    SELECT id FROM mst.portofolio WHERE kode_portofolio = 'BOND_SOD_TEST'
);
-- Ekspektasi: PENDING_APPROVAL
```

**Cleanup**:
```sql
DELETE FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.portofolio WHERE kode_portofolio = 'BOND_SOD_TEST');
DELETE FROM mst.portofolio WHERE kode_portofolio = 'BOND_SOD_TEST';
```

---

### S-006: Optimistic Lock — Edit Bersamaan Ditolak

**Aktor**: `uat.pt.maker` (2 sesi/tab berbeda)
**Pre-condition**: Ada portofolio `BOND_OPTL_TEST` dalam status `DRAFT`

**Setup**:
```sql
INSERT INTO mst.portofolio (
    id, kode_portofolio, nama, bm_category_default, aktif_flag,
    workflow_status, created_at, created_by, updated_at, updated_by,
    version, tenant_id, is_deleted
) VALUES (
    gen_random_uuid(), 'BOND_OPTL_TEST', 'Portofolio Optlock Test', 'HTC', true,
    'DRAFT', now(), 'aaaa0001-0000-0000-0000-000000000001',
    now(), 'aaaa0001-0000-0000-0000-000000000001',
    1, 'TUGURE', false
) ON CONFLICT (kode_portofolio) DO NOTHING;
```

**Langkah-langkah — Simulasi dua sesi bersamaan**:

1. Buka **Tab A**: Login sebagai `uat.pt.maker`, buka detail `BOND_OPTL_TEST`
2. Buka **Tab B**: Login sebagai `uat.pt.maker` (sesi berbeda), buka detail `BOND_OPTL_TEST`
3. Di **Tab A**:
   - Klik Edit
   - Ubah Nama menjadi: `Portofolio Optlock Update A`
   - Klik **"Simpan"** → harus sukses (rowVersion=1 → menjadi 2)
4. Di **Tab B** (masih menggunakan rowVersion=1 dari sebelum update Tab A):
   - Klik Edit
   - Ubah Nama menjadi: `Portofolio Optlock Update B`
   - Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Tab A: HTTP 200, Nama berubah ke "Portofolio Optlock Update A", rowVersion=2 | |
| 2 | Tab B: HTTP 409 CONFLICT karena rowVersion stale | |
| 3 | Toast error di Tab B: *"Data sudah diubah oleh pengguna lain. Refresh dan ulangi."* | |
| 4 | Nama di database tetap "Portofolio Optlock Update A" (bukan B) | |
| 5 | Setelah refresh di Tab B, rowVersion sudah 2 | |

**API Verification**:
```bash
# Kirim PUT dengan rowVersion stale=1 setelah Tab A sudah update ke rowVersion=2
curl -X PUT http://localhost:8080/api/v1/master/portofolio/BOND_OPTL_TEST \
  -H "Authorization: Bearer <token_maker>" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"nama":"Stale Update","rowVersion":1}'
# Ekspektasi: 409 {"error":{"code":"CONFLICT","message":"Data sudah diubah..."}}
```

**Audit Check**:
```sql
-- Harus ada tepat 1 update event, bukan 2
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'PORTOFOLIO.UPDATE'
  AND after_value::text LIKE '%BOND_OPTL_TEST%';
-- Ekspektasi: 1

-- Nama harus sesuai update yang sukses (Tab A)
SELECT nama, version FROM mst.portofolio
WHERE kode_portofolio = 'BOND_OPTL_TEST';
-- Ekspektasi: "Portofolio Optlock Update A", version=2
```

**Cleanup**:
```sql
DELETE FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.portofolio WHERE kode_portofolio = 'BOND_OPTL_TEST');
DELETE FROM mst.portofolio WHERE kode_portofolio = 'BOND_OPTL_TEST';
```

---

## Rollback / Cleanup Global

Setelah seluruh skenario selesai, jalankan cleanup berikut:

```sql
-- Hapus semua portofolio test (soft delete dan hard delete untuk non-APPROVED jika diperlukan)
DO $$
DECLARE
  kode TEXT;
  eid  UUID;
BEGIN
  FOR kode IN
    SELECT kode_portofolio FROM mst.portofolio
    WHERE kode_portofolio IN (
      'BOND_HTC_2026','BOND_HTC_4EYES','BOND_SOD_TEST','BOND_OPTL_TEST',
      'EKUITAS_EXIST'
    )
  LOOP
    SELECT id INTO eid FROM mst.portofolio WHERE kode_portofolio = kode;
    IF eid IS NOT NULL THEN
      DELETE FROM sys.workflow_instance WHERE entity_id = eid;
    END IF;
    DELETE FROM mst.portofolio WHERE kode_portofolio = kode;
  END LOOP;
END $$;

-- Hapus user UAT jika tidak diperlukan lagi
DELETE FROM sec.user WHERE username IN ('uat.pt.maker','uat.pt.reviewer','uat.pt.approver');
```

---

## Referensi

| Item | Referensi |
|---|---|
| BM Category values | `FSD-APP-A` §4.2 — HTC / HTCS / OTHER |
| 4-eyes workflow | `BRD_BLIPS_IFRS9_v1.1.docx` §3 RACI + DEC-017 |
| SoD enforcement | `security-baseline.md` §SoD enforcement |
| Optimistic lock | `api-conventions.md` §Error codes — CONFLICT |
| Audit trail | `db-conventions.md` §Audit log table — kanonik |
| Kode format regex | `^[A-Z0-9_]{1,20}$` — `portofolio/service.go:kodePortofolioRe` |
| PSAK 71 BM matrix | `glossary.md` §Matrix klasifikasi (SPPI × BM) |
