# UAT Script — APP-A-MSTR-004: Master LGD Basel (ECL Parameter)

**ID UAT**: UAT-APP-A-MSTR-004-001
**Story**: APP-A-MSTR-004 Master LGD Basel — ECL Parameter 6-Eyes Workflow
**Modul**: APP-A Master Data Management / APP-C ECL Engine
**Tanggal**: 2026-06-03
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

> **PERHATIAN — ECL PARAMETER MODULE**
> Modul ini adalah parameter ECL (DEC-010). Setiap perubahan yang di-approve langsung
> mempengaruhi perhitungan ECL dan CKPN. Seluruh skenario UAT harus lulus sebelum
> Phase 4 dimulai dan sebelum digunakan dalam ECL calc run apapun.
> Gate BLOCKING: `ifrs9-compliance-reviewer` harus sign-off setelah UAT ini selesai.

---

## Pre-conditions

### Infrastruktur

- BLIPS dev/UAT stack berjalan:
  ```
  docker compose -f deploy/docker/docker-compose.dev.yml up -d
  ```
- Database sudah dimigrasikan (termasuk migration 0008 + 0010):
  ```
  go run ./cmd/migrator up
  ```
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`
- Migrasi 0008 memuat `WORKFLOW_CONFIG_LGD_BASEL` ke `sys.config` — verifikasi:
  ```sql
  SELECT key FROM sys.config WHERE key = 'WORKFLOW_CONFIG_LGD_BASEL';
  -- Expected: 1 row
  ```

### Persona yang Dibutuhkan (4 user BERBEDA — wajib)

| Alias | Role Keycloak | Permissions Minimal | MFA |
|---|---|---|---|
| `risk.maker` | ROLE-RISK | `ecl_parameter.create`, `ecl_parameter.read`, `ecl_parameter.submit` | Tidak wajib |
| `ctl.reviewer` | ROLE-AKUN-CTL | `ecl_parameter.read`, `ecl_parameter.review`, `ecl_parameter.reject` | Wajib (role) |
| `alco.approver1` | ROLE-ALCO | `ecl_parameter.read`, `ecl_parameter.approve`, `ecl_parameter.reject` | **WAJIB + step-up** |
| `alco.approver2` | ROLE-ALCO | `ecl_parameter.read`, `ecl_parameter.approve`, `ecl_parameter.reject` | **WAJIB + step-up** |

> `alco.approver1` dan `alco.approver2` HARUS user yang berbeda dari semua aktor sebelumnya.
> Gunakan Keycloak Admin Console untuk assign role dan aktifkan MFA (TOTP/WebAuthn).

### Seed Data Minimal

```sql
-- Bersihkan data test sebelumnya (hati-hati di lingkungan shared).
DELETE FROM mst.lgd_basel WHERE tipe_eksposur IN ('CORPORATE', 'BANK', 'RETAIL')
  AND tenant_id = 'TUGURE' AND karakteristik LIKE '%UAT%';

-- Pastikan tabel ecl.ecl_calc_result_line ada (migration 0010).
SELECT COUNT(*) FROM ecl.ecl_calc_result_line LIMIT 1;
```

### Nilai Contoh Numerik (dari SoW §4)

| Field | Nilai |
|---|---|
| Tipe Eksposur | CORPORATE |
| LGD (target) | 0.4500 (45%) |
| Periode Dari | 2026-01-01 |
| Periode Sampai | NULL (aktif sekarang) |
| Sumber | BASEL_III_IRB |

---

## SKENARIO UAT

---

### TC-001: Buat LGD Pool CORPORATE 45%

**Aktor**: `risk.maker` (ROLE-RISK)
**Pre-condition**: Tidak ada LGD CORPORATE aktif di database (periode overlap kosong)

**Langkah-langkah**:

1. Login sebagai `risk.maker` di `http://localhost:3001`
2. Navigasi ke **Master Data > LGD Basel** (`/master/lgd-basel`)
3. Verifikasi: banner peringatan "ECL PARAMETER — Perubahan memerlukan persetujuan ALCO" tampak di halaman list
4. Klik tombol **"+ Tambah LGD Basel"**
5. Isi form:
   - **Tipe Eksposur**: pilih `CORPORATE`
   - **LGD (%)**: `45.00` (UI mungkin tampilkan sebagai persentase; backend menerima `0.4500`)
   - **Periode Berlaku Dari**: `2026-01-01`
   - **Periode Berlaku Sampai**: biarkan kosong (open-ended = aktif sekarang)
   - **Sumber**: `BASEL_III_IRB`
   - **Karakteristik**: `UAT — LGD CORPORATE Pilot`
6. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 dari API; response mengandung `"tipeEksposur": "CORPORATE"` | |
| 2 | `"lgd": "0.4500"` (4 desimal) | |
| 3 | `"workflowStatus": "DRAFT"` | |
| 4 | Toast sukses hijau: *"LGD Basel CORPORATE berhasil dibuat. Menunggu review Finance Controller."* | |
| 5 | Toast mengandung link "Lihat detail" menuju halaman detail entitas baru | |
| 6 | `aud.audit_log` memiliki baris `action='LGD_BASEL.CREATE'` | |
| 7 | Record muncul di list dengan badge status **DRAFT** | |

**Verifikasi Audit SQL**:
```sql
SELECT action, actor_role, entity_type
FROM aud.audit_log
WHERE action = 'LGD_BASEL.CREATE'
  AND after_value::text LIKE '%CORPORATE%'
ORDER BY event_time DESC LIMIT 1;
-- Expected: 1 row, action='LGD_BASEL.CREATE'
```

---

### TC-002: Validation LGD Out-of-Range

**Aktor**: `risk.maker` (ROLE-RISK)
**Pre-condition**: Halaman form create terbuka

**Langkah-langkah**:

1. Buka form **"+ Tambah LGD Basel"**
2. Isi **Tipe Eksposur**: `BANK`
3. Isi **LGD (%)**: `150` (setara 1.5 — di luar range)
4. Isi **Periode Berlaku Dari**: `2026-01-01`
5. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 VALIDATION_FAILED diterima | |
| 2 | Field "LGD" di-highlight merah | |
| 3 | Pesan inline: *"LGD harus antara 0% dan 100% (inklusif)"* | |
| 4 | Toast error merah **persistent** (tidak auto-close) | |
| 5 | Toast mengandung traceId | |
| 6 | Tidak ada record dibuat di database | |
| 7 | Tombol "Simpan" aktif kembali setelah error | |

**Variasi**: ulangi dengan LGD = `-10` (negatif) — harus ditolak dengan error yang sama.

**Catatan**: LGD = `0` (0%) dan LGD = `100` (100%) adalah nilai valid (batas inklusif). Verifikasi keduanya diterima.

---

### TC-003: 6-Eyes Workflow Happy Path

**Aktor**: `risk.maker`, `ctl.reviewer`, `alco.approver1`, `alco.approver2`
**Pre-condition**: LGD CORPORATE dari TC-001 ada dengan status DRAFT

**Bagian A: Maker Submit**

1. Login sebagai `risk.maker`
2. Buka halaman detail LGD CORPORATE (`/master/lgd-basel/{id}`)
3. Klik **"Kirim untuk Review"**
4. Dialog konfirmasi muncul; klik **"Lanjut"**

**Hasil yang Diharapkan — Bagian A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_REVIEW** | |
| 2 | Panel Workflow: Step 1 (Maker) tampil sebagai "selesai" (centang hijau) | |
| 3 | Panel Workflow: Step 2 (Reviewer) tampil sebagai "aktif" | |
| 4 | Audit: `action='LGD_BASEL.SUBMIT'` | |
| 5 | Toast: *"LGD Basel CORPORATE berhasil dikirim untuk review."* | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = '<id LGD CORPORATE>';
-- Expected: 'PENDING_REVIEW'
```

**Bagian B: Finance Controller Review**

1. Login sebagai `ctl.reviewer` (bukan maker — SoD terpenuhi)
2. Buka halaman detail LGD CORPORATE
3. Panel Workflow menampilkan Step 2 sebagai aktif
4. Isi komentar: `"Review OK — LGD 45% sesuai dengan Pefindo Annual Default Study 2025 §4.2"`
5. Klik **"Setujui (Teruskan ke ALCO)"**

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_APPROVAL** | |
| 2 | Panel Workflow: Step 3 (ALCO 1) tampil sebagai aktif | |
| 3 | Audit: `action='LGD_BASEL.REVIEW'` dengan komentar | |

**Bagian C: ALCO Approval 1 (dengan step-up MFA)**

1. Login sebagai `alco.approver1`
2. Buka halaman detail LGD CORPORATE
3. Panel menampilkan Step 3 sebagai aktif dengan badge "ECL Parameter — Step-up MFA Wajib"
4. Klik **"Setujui (Approval 1)"**
5. Dialog MFA muncul: *"Konfirmasi identitas dengan Step-up MFA untuk melanjutkan."*
6. Masukkan kode TOTP (dari Google Authenticator / app autentikator)
7. Klik **"Verifikasi dan Setujui"**

**Hasil yang Diharapkan — Bagian C**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_APPROVAL_2** | |
| 2 | Panel Workflow: Step 4 (ALCO 2) tampil sebagai aktif | |
| 3 | Audit: `action='LGD_BASEL.APPROVE'` dengan `signature_method='JWT_STEP_UP'` | |
| 4 | Toast: *"Approval 1 oleh ALCO berhasil. Menunggu approval ALCO kedua."* | |

**Bagian D: ALCO Approval 2 (dengan step-up MFA)**

1. Login sebagai `alco.approver2` (BERBEDA dari approver1, reviewer, dan maker)
2. Buka halaman detail LGD CORPORATE
3. Panel menampilkan Step 4 sebagai aktif
4. Klik **"Setujui Final (Approval 2)"**
5. Dialog MFA muncul — masukkan kode TOTP
6. Klik **"Verifikasi dan Setujui Final"**

**Hasil yang Diharapkan — Bagian D**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **APPROVED** | |
| 2 | Panel Workflow: semua 4 step tampil sebagai "selesai" (centang hijau) | |
| 3 | Audit: `action='LGD_BASEL.APPROVE_2'` atau `'LGD_BASEL.APPROVE2'` | |
| 4 | 4 signature records ada di `sys.workflow_signature` | |
| 5 | Toast: *"LGD Basel CORPORATE 45% berhasil disetujui. Parameter ECL siap digunakan."* | |
| 6 | Record tampil dengan badge **APPROVED** di list | |
| 7 | Tombol Edit/Hapus tidak tampil (APPROVED = immutable melalui workflow) | |

**Verifikasi SQL Lengkap**:
```sql
-- Workflow state final
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = '<id LGD CORPORATE>';
-- Expected: 'APPROVED'

-- Audit history — 5 events
SELECT action, actor_user_id FROM aud.audit_log
WHERE action IN (
    'LGD_BASEL.CREATE', 'LGD_BASEL.SUBMIT', 'LGD_BASEL.REVIEW',
    'LGD_BASEL.APPROVE', 'LGD_BASEL.APPROVE_2'
)
  AND entity_id = '<id LGD CORPORATE>'
ORDER BY event_time;
-- Expected: 5 rows (adjust APPROVE_2 jika nama action berbeda)

-- 4 signature records
SELECT COUNT(*), signer_user_id FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = '<id LGD CORPORATE>'
GROUP BY signer_user_id;
-- Expected: 4 rows, masing-masing 1 per user (maker, reviewer, approver1, approver2)

-- mst.lgd_basel sync
SELECT workflow_status FROM mst.lgd_basel WHERE id = '<id>';
-- Expected: 'APPROVED'
```

---

### TC-004: SoD — Approver2 Tidak Boleh Sama dengan Aktor Sebelumnya

**Aktor**: `alco.approver1` (sudah menjadi approver1 di TC-003)
**Pre-condition**: Ada LGD baru dalam state PENDING_APPROVAL_2 (buat entitas terpisah untuk test ini)

**Setup**:
```bash
# Buat LGD BANK baru dan advance ke PENDING_APPROVAL_2
# (langkah-langkah sama dengan TC-003 A, B, C menggunakan aktor yang sama)
```

**Langkah via UI**:

1. Login sebagai `alco.approver1` (sudah menjadi approver1 pada entitas baru ini)
2. Buka halaman detail LGD BANK dalam state PENDING_APPROVAL_2
3. Panel Workflow harus menampilkan **SodBlockBanner**: *"Anda sudah berperan sebagai Approver 1 pada entitas ini dan tidak bisa menjadi Approver 2."*
4. Tombol "Setujui Final" harus **disabled** atau tidak tampil

**Langkah via API (bypass test)**:

```bash
curl -X POST http://localhost:8080/api/v1/master/lgd-basel/<id>/approve2 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_alco_approver1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"signatureMethod":"JWT_STEP_UP","rowVersion":4}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: SodBlockBanner tampil dengan pesan yang sesuai | |
| 2 | UI: Tombol approve2 tidak aktif untuk approver1 | |
| 3 | API: HTTP 403 SOD_VIOLATION diterima | |
| 4 | `"error.code": "SOD_VIOLATION"` | |
| 5 | Toast UI: *"Anda tidak bisa menjadi approver kedua karena sudah terlibat di langkah sebelumnya."* | |
| 6 | Status entitas tetap **PENDING_APPROVAL_2** | |

**Variasi**: ulangi dengan `risk.maker` (maker) dan `ctl.reviewer` (reviewer) — keduanya harus ditolak dengan 403 SOD_VIOLATION.

---

### TC-005: Step-up MFA Wajib di Approve dan Approve2

**Aktor**: `alco.approver1` (dengan dan tanpa MFA aktif)
**Pre-condition**: Ada LGD dalam state PENDING_APPROVAL

**Langkah — Tanpa MFA Aktif**:

1. Login sebagai `alco.approver1` dengan token JWT di mana `stepup_verified_at` tidak ada atau > 5 menit
2. Buka halaman detail LGD dalam state PENDING_APPROVAL
3. Klik **"Setujui (Approval 1)"** — dialog MFA harus muncul
4. Klik **"Batalkan"** di dialog MFA

**Verifikasi via API (simulasi JWT tanpa step-up)**:
```bash
# Gunakan token JWT valid tanpa stepup_verified_at
curl -X POST http://localhost:8080/api/v1/master/lgd-basel/<id>/approve \
  -H "Authorization: Bearer <token_tanpa_stepup>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":3}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: Dialog MFA muncul saat klik "Setujui" | |
| 2 | UI: Setelah "Batalkan" — status TIDAK berubah, tetap PENDING_APPROVAL | |
| 3 | API: HTTP 403 STEP_UP_REQUIRED diterima | |
| 4 | `"error.code": "STEP_UP_REQUIRED"` | |
| 5 | Pesan: *"Aksi 'approve' untuk 'LGD_BASEL' memerlukan step-up MFA."* | |
| 6 | Status entitas tidak berubah | |

**Langkah — Dengan MFA Aktif**:

1. Login sebagai `alco.approver1`
2. Di dialog MFA, masukkan kode TOTP yang valid
3. Klik **"Verifikasi dan Setujui"**
4. Verifikasi status berubah ke PENDING_APPROVAL_2

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 7 | Setelah MFA berhasil: status berubah ke PENDING_APPROVAL_2 | |
| 8 | `signature_method = 'JWT_STEP_UP'` di `sys.workflow_signature` | |

---

### TC-006: Period Overlap — Dua LGD CORPORATE Aktif

**Aktor**: `risk.maker`
**Pre-condition**: LGD CORPORATE aktif (periode open-ended, dari TC-001) sudah ada

**Langkah-langkah**:

1. Login sebagai `risk.maker`
2. Buka form **"+ Tambah LGD Basel"**
3. Isi:
   - **Tipe Eksposur**: `CORPORATE`
   - **LGD (%)**: `50.00`
   - **Periode Berlaku Dari**: `2026-07-01`
   - **Periode Berlaku Sampai**: kosong (open-ended)
4. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 LGD_PERIOD_OVERLAP diterima | |
| 2 | Toast error merah: *"Periode berlaku LGD untuk CORPORATE sudah ada — tutup periode sebelumnya (set tanggal sampai) terlebih dahulu."* | |
| 3 | Field "Periode Berlaku Dari" di-highlight atau pesan inline tampil | |
| 4 | Tidak ada record baru dibuat | |

**Catatan ALCO**: Untuk menambah entry CORPORATE baru, entry lama harus ditutup terlebih dahulu dengan mengatur `periodeBerlakuSampai` ke tanggal yang tepat.

---

### TC-007: Reject Flow — Dikembalikan ke Maker

**Aktor**: `ctl.reviewer` (ROLE-AKUN-CTL)
**Pre-condition**: Ada LGD dalam state PENDING_REVIEW

**Langkah-langkah**:

1. Login sebagai `ctl.reviewer`
2. Buka halaman detail LGD dalam state PENDING_REVIEW
3. Klik **"Tolak"** pada panel workflow
4. Dialog konfirmasi muncul dengan input "Alasan Penolakan" (required, minimal 10 karakter)
5. Isi alasan: `"LGD 45% tidak sesuai dengan SoW §4.3 tabel kalibrasi LGD CORPORATE terkini. Gunakan nilai 42%."`
6. Klik **"Lanjut Tolak"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **RETURNED** (DB: REJECTED) | |
| 2 | Alasan penolakan tersimpan di `sys.workflow_signature.comment` | |
| 3 | Audit: `action='LGD_BASEL.REJECT'` dengan komentar | |
| 4 | Banner **RETURNED** dengan alasan penolakan tampil di halaman detail | |
| 5 | Tombol **"Edit"** tersedia untuk `risk.maker` | |
| 6 | `risk.maker` bisa edit LGD menjadi `0.4200` dan re-submit | |
| 7 | Setelah re-submit: status kembali ke PENDING_REVIEW | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = '<id>';
-- Expected: 'REJECTED'

SELECT comment FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = '<id>'
ORDER BY ws.created_at DESC LIMIT 1;
-- Expected: alasan penolakan tersimpan
```

---

### TC-008: Soft Delete dengan ECL Reference — Ditolak

**Aktor**: `risk.maker`
**Pre-condition**: Ada LGD yang sudah digunakan dalam ECL calc run (memiliki referensi di `ecl.ecl_calc_result_line`)

**Setup (jika belum ada referensi)**:
```sql
-- Ambil ID LGD yang ingin dihapus
SELECT id FROM mst.lgd_basel WHERE tipe_eksposur = 'CORPORATE' LIMIT 1;

-- Simulasi ECL reference
INSERT INTO ecl.ecl_calc_result_line (
    id, lgd_pool_id,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    gen_random_uuid(), '<id LGD>',
    now(), '00000000-0000-0000-0000-000000000001',
    now(), '00000000-0000-0000-0000-000000000001',
    1, 'TUGURE'
);
```

**Langkah via API**:
```bash
curl -X DELETE http://localhost:8080/api/v1/master/lgd-basel/<id> \
  -H "Authorization: Bearer <token_risk_maker>" \
  -H "Idempotency-Key: $(uuidgen)"
```

**Langkah via UI** (jika tombol Hapus tersedia untuk DRAFT):
1. Buka halaman detail LGD
2. Klik **"Hapus"**
3. Dialog konfirmasi muncul
4. Klik **"Hapus"** di dialog

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 409 ENTITY_IN_USE diterima | |
| 2 | Pesan: *"LGD Basel tidak bisa dihapus karena masih direferensikan oleh N calc result line. Tutup atau archive calc run terkait terlebih dahulu."* | |
| 3 | `deleted_at IS NULL` — record tidak dihapus | |
| 4 | Toast error merah persistent | |

**Verifikasi SQL**:
```sql
SELECT deleted_at FROM mst.lgd_basel WHERE id = '<id>';
-- Expected: NULL (tidak terhapus)
```

---

### TC-009: Export CSV/XLSX dengan Filter

**Aktor**: `risk.maker` (permission `ecl_parameter.export`)
**Pre-condition**: Ada beberapa record LGD dengan tipe_eksposur berbeda

**Langkah-langkah**:

1. Buka `/master/lgd-basel`
2. Aktifkan filter: **Tipe Eksposur = CORPORATE**
3. Verifikasi tabel hanya menampilkan record CORPORATE
4. Klik **"Export" → "CSV"**
5. Download file
6. Buka file di Excel

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | File ter-download dengan nama `lgd-basel-YYYYMMDD.csv` | |
| 2 | File hanya mengandung record **CORPORATE** (filter dihormati) | |
| 3 | Header row: `ID,Tipe Eksposur,LGD (%),Karakteristik,Periode Dari,Periode Sampai,Sumber,Status Workflow,Dibuat Oleh,Dibuat Pada` (atau equivalent) | |
| 4 | LGD tampil sebagai desimal 4 digit (contoh: `0.4500`) | |
| 5 | Encoding UTF-8 with BOM (buka di Excel tanpa garbled characters) | |
| 6 | `Content-Type: text/csv` dan `Content-Disposition: attachment` | |
| 7 | `X-Total-Rows` header sesuai jumlah record | |
| 8 | Audit: `action='LGD_BASEL.EXPORT'` dengan `format='csv'` dan `row_count` | |

**Ulangi dengan format XLSX**:
- Klik **"Export" → "XLSX"**
- Verifikasi file .xlsx ter-download, header row bold, LGD formatted sebagai `#,##0.0000`

**Verifikasi Audit SQL**:
```sql
SELECT after_value FROM aud.audit_log
WHERE action = 'LGD_BASEL.EXPORT'
ORDER BY event_time DESC LIMIT 1;
-- Expected: {format: 'csv', row_count: N, filters: {tipe_eksposur: 'CORPORATE'}}
```

---

### TC-010: Banner ECL Parameter di List dan Detail

**Aktor**: Semua role dengan `ecl_parameter.read`
**Pre-condition**: Halaman `/master/lgd-basel` dan detail dapat diakses

**Langkah-langkah**:

1. Login sebagai `risk.maker`
2. Buka `/master/lgd-basel` (halaman list)
3. Perhatikan area header halaman
4. Klik salah satu record untuk membuka halaman detail
5. Perhatikan area header halaman detail

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Halaman list menampilkan banner peringatan: "ECL PARAMETER — Perubahan LGD memerlukan 6-eyes approval (ALCO) dan step-up MFA. Data ini secara langsung mempengaruhi perhitungan CKPN PSAK 71." | |
| 2 | Banner tampil dengan warna amber/kuning (warning) | |
| 3 | Halaman detail juga menampilkan banner yang sama | |
| 4 | Tombol Export tersedia di halaman list | |
| 5 | Sort, filter, dan pagination berfungsi di tabel list | |

---

### TC-011: Sort + Filter + Pagination

**Aktor**: `risk.maker`
**Pre-condition**: Ada beberapa record LGD dengan tipe berbeda

**Test A: Sort**
1. Klik header kolom **"LGD"** → sort ASC
2. Verifikasi: LGD urut dari terkecil ke terbesar; URL: `?sort=lgd:asc`
3. Klik lagi **"LGD"** → sort DESC
4. Verifikasi: LGD urut dari terbesar ke terkecil

**Test B: Filter Tipe Eksposur**
1. Pilih filter **"Tipe Eksposur = CORPORATE"**
2. Verifikasi: filter chip muncul, hanya CORPORATE tampil
3. URL: `?filter[tipe_eksposur]=CORPORATE`
4. Klik **"Hapus semua filter"**
5. Verifikasi: semua record tampil kembali

**Test C: Text Search**
1. Ketik `BASEL` di search box
2. Verifikasi: hanya tampil record dengan "BASEL" di sumber atau karakteristik
3. URL: `?q=BASEL`

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Sort berfungsi, ikon panah tampil di header kolom aktif | |
| 2 | Filter chip tampil dan bisa di-clear | |
| 3 | State filter/sort tersimpan di URL (deep-link friendly) | |

---

### TC-012: ROLE-AUDIT — Read-Only

**Aktor**: user dengan ROLE-AUDIT
**Pre-condition**: Beberapa record LGD ada

**Langkah-langkah**:

1. Login sebagai user ROLE-AUDIT
2. Buka `/master/lgd-basel`
3. Verifikasi UI read-only
4. Buka detail salah satu record
5. Akses `/master/lgd-basel/{id}/history`

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Tidak ada tombol **"+ Tambah LGD Basel"** | |
| 2 | Tidak ada tombol **"Edit"** atau **"Hapus"** | |
| 3 | Tombol **"Export"** tersedia | |
| 4 | `/history` menampilkan audit trail lengkap dengan `before_value` dan `after_value` | |
| 5 | API coba POST/PATCH/DELETE → 403 FORBIDDEN | |

---

## Cleanup / Rollback

Setelah UAT selesai:

```sql
-- Hapus ECL reference test (jika ada dari TC-008)
DELETE FROM ecl.ecl_calc_result_line
WHERE lgd_pool_id IN (
    SELECT id FROM mst.lgd_basel
    WHERE karakteristik LIKE '%UAT%'
);

-- Hapus workflow instances
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
    SELECT id FROM mst.lgd_basel
    WHERE karakteristik LIKE '%UAT%'
);

-- Soft-delete entitas LGD test (JANGAN hard delete — audit retention)
UPDATE mst.lgd_basel
SET deleted_at = now(), deleted_by = '00000000-0000-0000-0000-000000000001'
WHERE karakteristik LIKE '%UAT%'
  AND tenant_id = 'TUGURE';

-- Di lingkungan UAT (bukan production) — audit log test boleh di-exclude dari report
-- Catatan: DEC-018 melarang hard-delete audit log di production.
```

---

## Checklist Gate QA — ECL Parameter

| Gate | Status |
|---|---|
| TC-001 Buat LGD CORPORATE 45% (happy path) | |
| TC-002 Validation LGD out-of-range (>1 dan negatif) | |
| TC-003 6-eyes workflow lengkap (submit → review → approve1 MFA → approve2 MFA → APPROVED) | |
| TC-004 SoD approver2 ≠ maker/reviewer/approver1 (via UI dan API) | |
| TC-005 Step-up MFA wajib di approve dan approve2 | |
| TC-006 Period overlap CORPORATE ditolak 422 | |
| TC-007 Reject flow → RETURNED → revisi → re-submit | |
| TC-008 Delete dengan ECL reference ditolak 409 | |
| TC-009 Export CSV/XLSX dengan filter tipe_eksposur | |
| TC-010 Banner ECL Parameter tampak di list dan detail | |
| TC-011 Sort + filter + pagination berfungsi | |
| TC-012 ROLE-AUDIT read-only | |

**Semua TC-001 s.d. TC-012 harus PASS sebelum modul LGD Basel dapat digunakan dalam ECL calc run.**

---

## Sign-off

| Role | Nama | Tanda Tangan | Tanggal |
|---|---|---|---|
| QA Engineer | | | |
| IFRS9 Compliance Reviewer | | | |
| Risk Officer (ROLE-RISK rep.) | | | |
| ALCO Representative | | | |
| Finance Controller | | | |

> **Catatan Compliance**: Modul ini menyentuh parameter ECL (LGD) yang langsung mempengaruhi perhitungan CKPN PSAK 71. Approval dari `ifrs9-compliance-reviewer` adalah BLOCKING gate per aturan tim. Sign-off dari ALCO Representative wajib sebelum parameter ini digunakan dalam ECL calc run produksi pertama.
