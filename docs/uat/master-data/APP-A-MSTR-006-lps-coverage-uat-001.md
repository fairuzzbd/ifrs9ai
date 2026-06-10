# UAT Script — APP-A-MSTR-006: LPS Coverage
**ID UAT**: UAT-APP-A-MSTR-006-001
**Story**: APP-A-MSTR-006 Master LPS Coverage (DEC-014 IDR 2 Miliar Cap, 6-Eyes Workflow)
**Modul**: APP-A Master Data Management
**Tanggal**: 2026-06-03
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Latar Belakang Bisnis

LPS Coverage adalah batas garansi Lembaga Penjamin Simpanan sebesar **IDR 2.000.000.000** (2 miliar) per nasabah per bank (DEC-014). Nilai ini digunakan oleh ECL Aggregator untuk mengecualikan eksposur yang dijamin dari perhitungan ECL — hanya eksposur di atas batas yang dikenakan bobot risiko.

Karena nilai ini berdampak langsung pada ECL calculation, perubahannya memerlukan workflow **6-eyes** dengan **2 step-up MFA** dari ALCO (DEC-017, DEC-027).

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Database sudah dimigrasikan (termasuk migration 0012): `go run ./cmd/migrator up`
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`
- Keycloak berjalan, user test sudah di-assign role

### 4 User Test Wajib

| Username | Role | Kebutuhan MFA | Catatan |
|---|---|---|---|
| `risk.officer.1` | ROLE-RISK | Tidak wajib | REVIEWER — tidak boleh sama dengan maker |
| `akun.ctl.1` | ROLE-AKUN-CTL | Wajib (MFA) | MAKER — bisa create, tidak bisa approve |
| `alco.1` | ROLE-ALCO | Wajib (MFA + step-up) | APPROVER 1 (6-eyes step ke-3) |
| `alco.2` | ROLE-ALCO | Wajib (MFA + step-up) | APPROVER 2 (6-eyes step ke-4), tidak boleh sama dengan alco.1 |

### Seed Data (jalankan sebelum UAT)

```sql
-- Cleanup data test lama (jika ada)
DELETE FROM mst.lps_coverage
WHERE tenant_id = 'TUGURE'
  AND regulasi_referensi LIKE '%UAT-TEST%';

-- Verifikasi WORKFLOW_CONFIG_LPS_COVERAGE tersedia
SELECT key, value::jsonb->>'eyes' AS eyes, value::jsonb->>'entityType' AS entity_type
FROM sys.config
WHERE key = 'WORKFLOW_CONFIG_LPS_COVERAGE';
-- Expected: 1 row, eyes='6', entity_type='LPS_COVERAGE'
```

---

## SKENARIO UAT

---

### TC-001: Buat LPS Coverage Default (DEC-014)

**Aktor**: akun.ctl.1 (ROLE-AKUN-CTL)
**Tujuan**: Verifikasi pembuatan record dengan nilai default IDR 2 miliar, mata uang IDR, periode terbuka.

**Langkah-langkah**:

1. Login sebagai `akun.ctl.1` di `http://localhost:3001`
2. Navigasi ke **Master Data > LPS Coverage** (`/master/lps-coverage`)
3. Klik tombol **"+ Tambah LPS Coverage"**
4. Isi form:
   - **Coverage Amount**: `2000000000.00`
   - **Mata Uang**: `IDR` (read-only, tidak bisa diubah)
   - **Periode Berlaku Dari**: `2026-01-01`
   - **Periode Berlaku Sampai**: kosong (open-ended)
   - **Regulasi Referensi**: `PP LPS No. 3 Tahun 2024 — UAT-TEST-001`
5. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 diterima | |
| 2 | `"coverageAmount": "2000000000.0000"` di response (4 desimal, DEC-016) | |
| 3 | `"mataUang": "IDR"` di response | |
| 4 | `"workflowStatus": "DRAFT"` | |
| 5 | Toast sukses hijau: "LPS Coverage berhasil dibuat. Menunggu review." + link "Lihat detail" | |
| 6 | Record muncul di list dengan badge **DRAFT** | |
| 7 | Banner informasi **"DEC-014 — IDR 2 Miliar"** tampil di halaman detail | |

**Verifikasi Audit SQL**:
```sql
SELECT action, actor_role, after_value
FROM aud.audit_log
WHERE action = 'LPS_COVERAGE.CREATE'
  AND after_value::text LIKE '%UAT-TEST-001%'
ORDER BY timestamp DESC LIMIT 1;
-- Expected: 1 row, action='LPS_COVERAGE.CREATE'
```

**Verifikasi DB**:
```sql
SELECT coverage_amount, mata_uang, workflow_status
FROM mst.lps_coverage
WHERE regulasi_referensi LIKE '%UAT-TEST-001%';
-- Expected: coverage_amount=2000000000.0000, mata_uang='IDR', workflow_status='DRAFT'
```

---

### TC-002: Amount Not Positive — Inline Validation Error

**Aktor**: akun.ctl.1 (ROLE-AKUN-CTL)
**Tujuan**: Verifikasi bahwa coverage_amount = 0 atau negatif ditolak dengan pesan validasi inline.

**Sub-kasus A: Amount = 0**:

1. Buka form tambah LPS Coverage
2. Isi **Coverage Amount**: `0`
3. Isi field lain dengan nilai valid (`PeriodeBerlakuDari`: `2026-06-01`)
4. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 VALIDATION_FAILED diterima | |
| 2 | Field "Coverage Amount" di-highlight merah dengan pesan: *"coverageAmount harus lebih besar dari 0"* | |
| 3 | Toast error merah **persistent** (tidak auto-close) dengan traceId | |
| 4 | Tidak ada record dibuat di database | |
| 5 | Tombol "Simpan" aktif kembali (tidak terus disabled) | |

**Sub-kasus B: Amount = -1.000.000**:

1. Ulangi dengan **Coverage Amount**: `-1000000`
2. Verifikasi: sama seperti sub-kasus A — HTTP 422, inline error, tidak ada record dibuat

---

### TC-003: Currency Lock IDR — Field Tidak Bisa Diedit

**Aktor**: akun.ctl.1 (ROLE-AKUN-CTL)
**Tujuan**: Verifikasi mata_uang selalu IDR, field tidak bisa diubah (DEC-014 IDR only).

**Langkah-langkah**:

1. Buka form tambah LPS Coverage
2. Perhatikan field **Mata Uang**:
   - Harus menampilkan `IDR`
   - Harus berstatus **disabled/read-only** — tidak ada dropdown atau input yang bisa diedit
3. Coba bypass via API langsung:

```bash
curl -X POST http://localhost:8080/api/v1/master/lps-coverage \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_akun_ctl_1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"coverageAmount":"2000000000","periodeBerlakuDari":"2026-06-01","mataUang":"USD"}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: Field Mata Uang menampilkan "IDR" dan berstatus disabled/read-only | |
| 2 | API: Response body `"mataUang": "IDR"` meskipun request body mengirim `"mataUang":"USD"` | |
| 3 | DB: `mata_uang = 'IDR'` di record yang baru dibuat | |
| 4 | Jika di-insert langsung ke DB dengan `mata_uang='USD'` → DB CHECK constraint menolak dengan error | |

**Verifikasi DB constraint**:
```sql
-- Ini harus gagal dengan constraint error:
INSERT INTO mst.lps_coverage (id, coverage_amount, mata_uang, periode_berlaku_dari, maker_id, workflow_status, created_at, created_by, updated_at, updated_by, row_version, tenant_id)
VALUES (gen_random_uuid(), 2000000000, 'USD', '2026-06-01', '00000000-0000-0000-0000-000000000001', 'DRAFT', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE');
-- Expected: ERROR: new row for relation "lps_coverage" violates check constraint "chk_lps_coverage_currency"
```

---

### TC-004: 6-Eyes Happy Path — Full Cycle dengan 2x Step-Up MFA

**Aktor**: akun.ctl.1 (MAKER), risk.officer.1 (REVIEWER), alco.1 (APPROVER 1), alco.2 (APPROVER 2)
**Tujuan**: Verifikasi siklus lengkap DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED dengan 2 MFA step-up prompts.

**Pre-condition**: TC-001 sudah dijalankan, record UAT-TEST-001 ada dalam status DRAFT dengan ID tercatat.

**Bagian A: MAKER Submit**

1. Login sebagai `akun.ctl.1`
2. Buka halaman detail LPS Coverage UAT-TEST-001 (`/master/lps-coverage/{id}`)
3. Panel workflow menampilkan **Step 1: Pembuat** (active) — tombol "Kirim untuk Review" tersedia
4. Klik **"Kirim untuk Review"**
5. Konfirmasi dialog

**Hasil yang Diharapkan — Bagian A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_REVIEW** | |
| 2 | Toast: *"LPS Coverage berhasil dikirim untuk review."* | |
| 3 | `sys.workflow_instance.current_state = 'PENDING_REVIEW'` | |
| 4 | `mst.lps_coverage.workflow_status = 'PENDING_REVIEW'` (EntityHook sync) | |

```sql
SELECT wi.current_state AS workflow_state, lc.workflow_status AS lps_status
FROM sys.workflow_instance wi
JOIN mst.lps_coverage lc ON wi.entity_id = lc.id
WHERE lc.regulasi_referensi LIKE '%UAT-TEST-001%';
-- Expected: workflow_state='PENDING_REVIEW', lps_status='PENDING_REVIEW'
```

**Bagian B: REVIEWER Review (RISK)**

1. Login sebagai `risk.officer.1` (BUKAN akun.ctl.1 — SoD)
2. Navigasi ke queue review atau buka halaman detail langsung
3. Panel workflow: **Step 2: Pemeriksa** (active)
4. Isi komentar: *"Review OK — nilai coverage sesuai peraturan LPS yang berlaku."*
5. Klik **"Setujui Review"**

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_APPROVAL** | |
| 2 | 1 signature record tersimpan di `sys.workflow_signature` | |
| 3 | `mst.lps_coverage.workflow_status = 'PENDING_APPROVAL'` (EntityHook sync) | |

**Bagian C: APPROVER 1 — alco.1 dengan Step-Up MFA**

1. Login sebagai `alco.1`
2. Buka halaman detail LPS Coverage UAT-TEST-001
3. Panel workflow: **Step 3: Penyetuju 1 (ALCO)** (active)
4. Klik **"Setujui"** — sistem meminta **MFA step-up** (dialog konfirmasi MFA muncul)
5. Masukkan kode TOTP alco.1 di dialog step-up
6. Klik **"Konfirmasi Persetujuan"**

**Hasil yang Diharapkan — Bagian C**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Dialog step-up MFA muncul sebelum approve bisa dilanjutkan | |
| 2 | Status berubah menjadi **PENDING_APPROVAL_2** | |
| 3 | 2 signature records tersimpan | |
| 4 | `mst.lps_coverage.workflow_status = 'PENDING_APPROVAL_2'` | |
| 5 | Tombol "Setujui" di panel Step 3 menampilkan spinner saat memproses | |

**Bagian D: APPROVER 2 — alco.2 dengan Step-Up MFA**

1. Login sebagai `alco.2` (BUKAN alco.1 atau user sebelumnya — SoD)
2. Buka halaman detail LPS Coverage UAT-TEST-001
3. Panel workflow: **Step 4: Penyetuju 2 (ALCO)** (active)
4. Klik **"Setujui Final"** — sistem meminta **MFA step-up kedua** (dialog lagi)
5. Masukkan kode TOTP alco.2
6. Klik **"Konfirmasi Persetujuan Final"**

**Hasil yang Diharapkan — Bagian D**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Dialog step-up MFA muncul lagi (untuk approve2) | |
| 2 | Status berubah menjadi **APPROVED** | |
| 3 | 4 signature records tersimpan di `sys.workflow_signature` | |
| 4 | `mst.lps_coverage.workflow_status = 'APPROVED'` (EntityHook sync) | |
| 5 | `sys.workflow_instance.current_state = 'APPROVED'` | |
| 6 | Toast: *"LPS Coverage berhasil disetujui final."* | |
| 7 | Panel workflow menampilkan semua 4 step sebagai selesai (ikon centang hijau) | |
| 8 | Halaman detail menampilkan badge **APPROVED** | |

**Verifikasi Audit SQL (full chain)**:
```sql
SELECT action, actor_user_id, timestamp
FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.lps_coverage WHERE regulasi_referensi LIKE '%UAT-TEST-001%')
  AND action LIKE 'LPS_COVERAGE.%'
ORDER BY timestamp;
-- Expected: setidaknya 5 rows (CREATE, SUBMIT, REVIEW, APPROVE, APPROVE2)

-- Verifikasi 4 signatures
SELECT ws.action, ws.user_id, ws.signed_at
FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
JOIN mst.lps_coverage lc ON wi.entity_id = lc.id
WHERE lc.regulasi_referensi LIKE '%UAT-TEST-001%'
ORDER BY ws.signed_at;
-- Expected: 4 rows (SUBMIT, REVIEW, APPROVE, APPROVE2)

-- Final sync verification
SELECT wi.current_state AS workflow_state, lc.workflow_status AS lps_status
FROM sys.workflow_instance wi
JOIN mst.lps_coverage lc ON wi.entity_id = lc.id
WHERE lc.regulasi_referensi LIKE '%UAT-TEST-001%';
-- Expected: workflow_state='APPROVED', lps_status='APPROVED'
```

---

### TC-005: SoD — Approver 2 Tidak Boleh Sama dengan User Sebelumnya

**Aktor**: alco.1 mencoba menjadi alco.2 (approver2 = approver1)
**Tujuan**: Verifikasi SoD enforcement 6-eyes — approver2 ≠ maker/reviewer/approver1.

**Pre-condition**: Buat record baru UAT-TEST-005 sampai status PENDING_APPROVAL_2 dengan alco.1 sebagai approver1.

**Setup (via API atau UI)**:
```bash
# Buat record
RECORD_ID=$(curl -s -X POST http://localhost:8080/api/v1/master/lps-coverage \
  -H "Authorization: Bearer <token_akun_ctl_1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"coverageAmount":"2000000000","periodeBerlakuDari":"2027-01-01","regulasiReferensi":"UAT-TEST-005"}' \
  | jq -r '.data.id')

# Submit, review, approve1 ... (gunakan token user yang tepat)
```

**Langkah SoD test via API** (alco.1 mencoba approve2):
```bash
curl -X POST http://localhost:8080/api/v1/master/lps-coverage/${RECORD_ID}/approve2 \
  -H "Authorization: Bearer <token_alco_1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -H "X-Step-Up-Token: <valid_stepup_token>" \
  -d '{"signatureMethod":"JWT_STEP_UP","rowVersion":5}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 SOD_VIOLATION diterima | |
| 2 | `"error.code": "SOD_VIOLATION"` | |
| 3 | Status record tetap PENDING_APPROVAL_2 | |
| 4 | Tidak ada signature record dibuat untuk percobaan ini | |
| 5 | UI: tombol "Setujui Final" disabled/tidak tampil untuk alco.1 di step ke-4 | |
| 6 | UI: SodBlockBanner tampil: *"Anda sudah berpartisipasi di step sebelumnya dan tidak bisa menjadi penyetuju di step ini."* | |

---

### TC-006: Period Overlap — Buat Record Kedua yang Bertumpang-tindih

**Aktor**: akun.ctl.1 (ROLE-AKUN-CTL)
**Tujuan**: Verifikasi bahwa membuat record kedua yang overlap dengan record APPROVED aktif menghasilkan 422 LPS_PERIOD_OVERLAP.

**Pre-condition**: Record UAT-TEST-001 dari TC-004 sudah dalam status APPROVED, periode dari 2026-01-01 tanpa tanggal berakhir (open-ended).

**Langkah-langkah**:

1. Login sebagai `akun.ctl.1`
2. Buka form tambah LPS Coverage
3. Isi form:
   - **Coverage Amount**: `2200000000`
   - **Periode Berlaku Dari**: `2026-06-01` (overlap dengan record aktif 2026-01-01 s.d. open)
   - **Regulasi Referensi**: `UAT-TEST-006`
4. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 `LPS_PERIOD_OVERLAP` diterima | |
| 2 | Toast error merah persistent: *"Periode berlaku bertumpang-tindih dengan record LPS Coverage yang sudah aktif (APPROVED). Tutup periode record lama terlebih dahulu dengan mengisi periode_berlaku_sampai sebelum membuat record baru."* | |
| 3 | Tidak ada record baru dibuat | |

**Verifikasi DB**:
```sql
SELECT COUNT(*) FROM mst.lps_coverage
WHERE regulasi_referensi LIKE '%UAT-TEST-006%';
-- Expected: 0
```

---

### TC-007: Period Handoff — Tutup Record Lama, Buka Record Baru

**Aktor**: akun.ctl.1 (MAKER) + full 6-eyes untuk menutup record lama
**Tujuan**: Verifikasi bahwa setelah record APPROVED A diberi tanggal berakhir, record baru B dengan start date setelah end date A berhasil dibuat tanpa overlap error.

**Langkah-langkah**:

**Bagian A: Tutup Record Aktif (UAT-TEST-001)**

1. Login sebagai `akun.ctl.1`
2. Buka detail UAT-TEST-001 (status: APPROVED)
3. Karena APPROVED, tidak bisa di-edit langsung — gunakan endpoint update via API dengan kredensi reviewer atau buat record baru dengan `periode_berlaku_sampai` yang tepat:

   Alternatif: update record UAT-TEST-001 via PATCH (jika record masih DRAFT/RETURNED — sesuaikan setup):
   ```bash
   # Atau update langsung via DB untuk UAT purposes
   UPDATE mst.lps_coverage
   SET periode_berlaku_sampai = '2026-12-31'
   WHERE regulasi_referensi LIKE '%UAT-TEST-001%';
   ```

4. Verifikasi: UAT-TEST-001 sekarang memiliki `periode_berlaku_sampai = '2026-12-31'`

**Bagian B: Buat Record Baru B (dari 2027-01-01)**

1. Klik **"+ Tambah LPS Coverage"**
2. Isi form:
   - **Coverage Amount**: `2200000000`
   - **Periode Berlaku Dari**: `2027-01-01` (tidak overlap dengan A yang berakhir 2026-12-31)
   - **Regulasi Referensi**: `UAT-TEST-007`
3. Klik **"Simpan"**

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 — record baru berhasil dibuat (tidak ada overlap error) | |
| 2 | `workflowStatus = 'DRAFT'` | |
| 3 | `coverageAmount = '2200000000.0000'` | |
| 4 | Toast sukses hijau tampil | |
| 5 | Kedua record tampil di list: UAT-TEST-001 (APPROVED, sampai 2026-12-31) dan UAT-TEST-007 (DRAFT, dari 2027-01-01) | |

**Verifikasi DB**:
```sql
SELECT regulasi_referensi, periode_berlaku_dari, periode_berlaku_sampai, workflow_status
FROM mst.lps_coverage
WHERE regulasi_referensi LIKE '%UAT-TEST-00%'
ORDER BY periode_berlaku_dari;
-- Expected: UAT-TEST-001 s.d. 2026-12-31 APPROVED, UAT-TEST-007 dari 2027-01-01 DRAFT
```

---

### TC-008: Export CSV — Filter Respects Active

**Aktor**: akun.ctl.1 (atau risk.officer.1) dengan permission `ecl_parameter.read`
**Tujuan**: Verifikasi export CSV menghormati filter aktif, encoding BOM UTF-8, dan audit row tercatat.

**Langkah-langkah**:

1. Login sebagai `akun.ctl.1`
2. Navigasi ke `/master/lps-coverage`
3. Aktifkan filter **"Status = APPROVED"**
4. Verifikasi tabel hanya menampilkan record dengan status APPROVED
5. Klik **"Export" → "CSV"**
6. Download file
7. Buka file di Excel (Indonesia locale)

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | File ter-download, nama format `lps-coverage-YYYYMMDD.csv` | |
| 2 | File HANYA mengandung record APPROVED (bukan DRAFT, PENDING_*) | |
| 3 | Header row: `ID,Coverage Amount (IDR),Mata Uang,Periode Dari,Periode Sampai,Regulasi Referensi,Workflow Status` | |
| 4 | File encoding UTF-8 with BOM (tampil tanpa garbled di Excel) | |
| 5 | `Content-Disposition: attachment; filename="lps-coverage-YYYYMMDD.csv"` | |
| 6 | `X-Total-Rows` header sesuai jumlah record APPROVED | |
| 7 | Kolom "Coverage Amount (IDR)" menampilkan `2000000000.0000` (4 desimal) | |
| 8 | Audit log: `action='LPS_COVERAGE.EXPORT'` dengan `format='csv'` dan filter aktif | |

**Verifikasi Audit SQL**:
```sql
SELECT after_value
FROM aud.audit_log
WHERE action = 'LPS_COVERAGE.EXPORT'
ORDER BY timestamp DESC LIMIT 1;
-- Expected: {format:'csv', row_count:N, filters:{workflow_status:'APPROVED'}}
```

---

## Skenario Negatif Tambahan

### TC-N01: Step-Up MFA Tidak Valid untuk Approve2

**Aktor**: alco.2
**Tujuan**: Verifikasi bahwa approve2 tanpa step-up token yang valid ditolak.

```bash
# Gunakan token tanpa X-Step-Up-Token header
curl -X POST http://localhost:8080/api/v1/master/lps-coverage/${RECORD_ID}/approve2 \
  -H "Authorization: Bearer <token_alco_2>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":5}'
  # Tanpa X-Step-Up-Token
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 STEP_UP_REQUIRED | |
| 2 | Pesan: *"Aksi 'approve2' untuk 'LPS_COVERAGE' memerlukan step-up MFA."* | |
| 3 | Status record tidak berubah (masih PENDING_APPROVAL_2) | |

### TC-N02: Idempotency Replay — Request Create Duplikat

**Aktor**: akun.ctl.1

1. Kirim request create dengan Idempotency-Key tertentu (pertama kali → 201)
2. Kirim ulang request identik dengan key yang sama → harus 201 (replay, bukan duplicate create)
3. DB hanya memiliki 1 record dari key tersebut

```sql
SELECT COUNT(*) FROM mst.lps_coverage
WHERE regulasi_referensi = 'UAT-TEST-IDEM';
-- Expected: 1 (bukan 2)
```

### TC-N03: Optimistic Lock — Edit Bersamaan

**Aktor**: akun.ctl.1 (User A) dan risk.officer.1 (User B) — berdua membuka form edit record DRAFT yang sama.

1. Keduanya memuat record dengan `rowVersion = 1`
2. User A simpan dulu → sukses, `rowVersion = 2`
3. User B simpan dengan `rowVersion = 1` (stale)

**Hasil yang Diharapkan**:
- User A: HTTP 200 sukses
- User B: HTTP 409 CONFLICT, toast error: *"Data telah diubah oleh user lain. Muat ulang halaman."*

---

## Cleanup / Rollback

Setelah UAT selesai, jalankan:

```sql
-- Hapus workflow instances untuk record test
DELETE FROM sys.workflow_signature ws
USING sys.workflow_instance wi
JOIN mst.lps_coverage lc ON wi.entity_id = lc.id
WHERE ws.workflow_instance_id = wi.id
  AND lc.regulasi_referensi LIKE '%UAT-TEST%';

DELETE FROM sys.workflow_instance wi
USING mst.lps_coverage lc
WHERE wi.entity_id = lc.id
  AND lc.regulasi_referensi LIKE '%UAT-TEST%';

-- Soft-delete record test
UPDATE mst.lps_coverage
SET deleted_at = now(), deleted_by = '00000000-0000-0000-0000-000000000001'
WHERE regulasi_referensi LIKE '%UAT-TEST%';

-- Catatan: aud.audit_log TIDAK boleh dihapus (DEC-018, retention 10+10 tahun)
-- Di env UAT, biarkan log test tetap di tabel.
```

---

## Checklist QA Gate Phase 3 LPS Coverage

| # | Skenario | Status |
|---|---|---|
| TC-001 | Happy path create, default IDR 2 miliar, status DRAFT | |
| TC-002 | Amount not positive — inline validation 422 | |
| TC-003 | Currency lock IDR — field read-only + DB constraint | |
| TC-004 | 6-eyes full cycle, 2x step-up MFA, both states sync (workflow_instance + lps_coverage) | |
| TC-005 | SoD: approver2 tidak bisa sama dengan maker/reviewer/approver1 | |
| TC-006 | Period overlap active APPROVED record — 422 LPS_PERIOD_OVERLAP | |
| TC-007 | Period handoff — tutup record lama, buka record baru sukses | |
| TC-008 | Export CSV respects filter, BOM UTF-8, audit row | |
| TC-N01 | Step-up MFA required untuk approve2 — 403 tanpa token | |
| TC-N02 | Idempotency replay — tidak ada duplicate create | |
| TC-N03 | Optimistic lock — 409 pada rowVersion stale | |

**SEMUA TC HARUS PASS sebelum Phase 4 LPS Coverage modul go-live.**

---

## Referensi

- DEC-014: LPS Aggregator IDR 2 miliar per nasabah per bank
- DEC-016: Decimal `NUMERIC(20,4)` untuk IDR amount
- DEC-017: 6-eyes workflow, SoD maker≠reviewer≠approver1≠approver2
- DEC-027: Step-up MFA wajib untuk approve + approve2 LPS Coverage
- Migration 0012: `000012_lps_coverage_schema_fix.up.sql`
- Migration 0008: `WORKFLOW_CONFIG_LPS_COVERAGE` seed di `sys.config`
