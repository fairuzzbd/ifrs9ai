# UAT Script — APP-A-MSTR-003: Master Periode Buku
**ID UAT**: UAT-APP-A-MSTR-003-001
**Story**: APP-A-MSTR-003 / APP-D-MSTR-001 Master Periode Buku
**Plan Ref**: PLAN-20260603 Fase 3 — Phase 3 Master Data
**Modul**: APP-A Master Data Management — Periode Buku
**Tanggal**: 2026-06-03
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Migrasi sudah diterapkan (termasuk 000009): `go run ./cmd/migrator up`
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`

### Konfigurasi Wajib
- Migration `000009_periode_buku_schema_fix` ter-apply (audit cols + workflow_status di mst.periode_buku)
- Workflow config `WORKFLOW_CONFIG_PERIODE_BUKU` ter-seed di `sys.config` (atau in-memory fallback aktif):
  - `eyes: 4` (4-eyes flow: maker → reviewer → approver)
  - `stepUpRequired.approve: true` (DEC-027: CFO MFA step-up wajib saat approve)
  - `sodRules.reviewerNotMaker: true`
  - `sodRules.approverNotMakerOrReviewer: true`

### Seed Data — 3 User Distinct (Segregasi Peran)

Jalankan di Keycloak Admin atau SQL (user shadow table):

```sql
-- Persona test (username unik, assign role di Keycloak)
-- pb.maker     → ROLE-AKUN       (create, submit; TIDAK bisa approve)
-- pb.reviewer  → ROLE-AKUN-CTL  (review, reject; TIDAK bisa approve)
-- pb.approver  → ROLE-CFO       (approve; WAJIB MFA aktif + step-up)
-- pb.auditor   → ROLE-AUDIT     (read-only; untuk verifikasi audit trail)

-- Contoh seed user shadow (jika belum ada):
INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES
    (gen_random_uuid(), 'pb.maker',    'pb.maker@test.blips',    'PB Maker',    'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
    (gen_random_uuid(), 'pb.reviewer', 'pb.reviewer@test.blips', 'PB Reviewer', 'AKTIF', now(), '00000000-0000-0000-0000-000000000001'),
    (gen_random_uuid(), 'pb.approver', 'pb.approver@test.blips', 'PB Approver', 'AKTIF', now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;
```

### Cleanup Data Sebelum UAT

Pastikan tidak ada data test lama yang mengganggu:

```sql
-- Hapus semua periode_buku tahun 2026 dari sesi UAT sebelumnya (jika ada)
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
    SELECT id FROM mst.periode_buku WHERE tahun_buku IN (2026, 2099)
);
DELETE FROM mst.periode_buku WHERE tahun_buku IN (2026, 2099);
```

---

## SKENARIO UAT

---

### TC-001: Generate Periode Tahun 2026

**ID**: TC-001
**Aktor**: pb.maker (ROLE-AKUN)
**Pre-condition**: Tidak ada periode_buku bertahun 2026 di database
**Tujuan**: Verifikasi generate 17 baris otomatis (12 BULANAN + 4 TRIWULANAN + 1 TAHUNAN)

**Langkah-langkah**:

1. Login sebagai `pb.maker` di `http://localhost:3001`
2. Navigasi ke **Master Data > Periode Buku** (`/master/periode-buku`)
3. Klik tombol **"Generate Periode Tahunan"** (atau tombol sejenisnya)
4. Pada dialog/form generate:
   - **Tahun Buku**: `2026`
   - Tipe: biarkan default (semua tipe: BULANAN, TRIWULANAN, TAHUNAN)
5. Klik **"Generate"**

**Alternatif via API (curl)**:
```bash
curl -X POST http://localhost:8080/api/v1/master/periode-buku/generate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_pb_maker>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"tahunBuku": 2026}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Response 201 dengan `data.generated = 17` dan `data.skipped = 0` | |
| 2 | Toast sukses: `"17 periode buku tahun 2026 berhasil dibuat."` | |
| 3 | List `/master/periode-buku` menampilkan 17 baris baru dengan filter tahun 2026 | |
| 4 | 12 baris BULANAN: kode `2026-M01` s.d. `2026-M12`, status **DRAFT** | |
| 5 | 4 baris TRIWULANAN: `2026-Q1`, `2026-Q2`, `2026-Q3`, `2026-Q4`, status **DRAFT** | |
| 6 | 1 baris TAHUNAN: `2026-Y`, status **DRAFT** | |
| 7 | `2026-M02`: tanggal_mulai=2026-02-01, tanggal_akhir=2026-02-28 (2026 bukan tahun kabisat) | |
| 8 | `2026-Q1`: tanggal_mulai=2026-01-01, tanggal_akhir=2026-03-31 | |
| 9 | `2026-Y`: tanggal_mulai=2026-01-01, tanggal_akhir=2026-12-31 | |
| 10 | Panggil generate kedua kali dengan body sama → `data.generated=0, data.skipped=17` (idempoten) | |

**Verifikasi SQL**:
```sql
-- Hitung per tipe
SELECT tipe_periode, COUNT(*) AS jumlah
FROM mst.periode_buku
WHERE tahun_buku = 2026 AND deleted_at IS NULL
GROUP BY tipe_periode;
-- Expected:
--   BULANAN      | 12
--   TRIWULANAN   | 4
--   TAHUNAN      | 1

-- Verifikasi tanggal akhir Q1
SELECT tanggal_akhir FROM mst.periode_buku WHERE periode_id_kode = '2026-Q1';
-- Expected: 2026-03-31

-- Verifikasi idempoten: generate kedua
-- (via API, respon harus generated=0, skipped=17)
```

---

### TC-002: CRUD Manual — Validasi Kode Format

**ID**: TC-002
**Aktor**: pb.maker (ROLE-AKUN)
**Pre-condition**: Halaman form create periode_buku terbuka
**Tujuan**: Verifikasi validasi Bahasa Indonesia untuk kode format invalid

**Langkah-langkah**:

1. Login sebagai `pb.maker`
2. Navigasi ke `/master/periode-buku` → klik **"+ Tambah Periode"**
3. Isi form dengan data valid kecuali **Kode Periode**:
   - **Tipe**: BULANAN
   - **Tahun Buku**: 2026
   - **Bulan**: 13
   - **Kode Periode**: `2026-M13` (bulan 13 tidak valid)
   - **Tanggal Mulai**: `2026-01-01`
   - **Tanggal Akhir**: `2026-01-31`
4. Klik **"Simpan"**

**Hasil yang Diharapkan — Kasus A (bulan 13)**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 VALIDATION_FAILED diterima | |
| 2 | Field "Kode Periode" atau "Bulan" di-highlight merah | |
| 3 | Pesan inline Bahasa Indonesia muncul (contoh: `"Bulan harus antara 1 dan 12"`) | |
| 4 | Toast error merah **persistent** (tidak auto-close) mengandung traceId | |
| 5 | Tidak ada record dibuat di database | |

**Langkah-langkah lanjutan — Kasus B (triwulan 5)**:

5. Bersihkan form. Isi:
   - **Tipe**: TRIWULANAN
   - **Triwulan**: 5
   - **Kode Periode**: `2026-Q5`
6. Klik **"Simpan"**

**Hasil yang Diharapkan — Kasus B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 VALIDATION_FAILED | |
| 2 | Pesan: `"Triwulan harus antara 1 dan 4"` | |
| 3 | Tidak ada record dibuat | |

**Langkah-langkah lanjutan — Kasus C (format tanpa dash)**:

7. Bersihkan form. Isi **Kode Periode**: `2026M06` (tanpa dash)
8. Klik **"Simpan"**

**Hasil yang Diharapkan — Kasus C**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 VALIDATION_FAILED | |
| 2 | Pesan: `"Kode periode harus format YYYY-Mnn, YYYY-Qn, atau YYYY-Y (contoh: 2026-M06, 2026-Q2, 2026-Y)"` | |
| 3 | Tidak ada record dibuat | |

**Verifikasi negatif — Kode valid**:
- Input `2026-M06`, BULANAN, bulan=6 → harus **sukses** (201), tidak ada error

---

### TC-003: Workflow 4-Eyes — Happy Path (DRAFT → APPROVED)

**ID**: TC-003
**Aktor**: pb.maker, pb.reviewer, pb.approver
**Pre-condition**: Generate TC-001 sudah dilakukan; periode `2026-M06` dalam status DRAFT
**Tujuan**: Verifikasi full 4-eyes cycle dengan step-up MFA di approve (DEC-027)

**Bagian A: Maker Submit**

1. Login sebagai `pb.maker`
2. Navigasi ke `/master/periode-buku`
3. Klik kode `2026-M06` untuk buka detail
4. Klik **"Kirim untuk Review"**
5. Konfirmasi dialog (jika ada), isi komentar optional: `"Ajukan review periode Juni 2026"`
6. Klik **"Konfirmasi"**

**Hasil yang Diharapkan — Bagian A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status `2026-M06` berubah menjadi **PENDING_REVIEW** | |
| 2 | Toast sukses: `"2026-M06 berhasil dikirim untuk review Finance Controller."` | |
| 3 | Tombol "Kirim untuk Review" tidak lagi tampil | |
| 4 | Panel workflow menampilkan **Step 2: Pemeriksa** sebagai langkah aktif | |

**Verifikasi SQL Bagian A**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06');
-- Expected: 'PENDING_REVIEW'

SELECT action FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06')
  AND action = 'PERIODE_BUKU.SUBMIT';
-- Expected: 1 baris
```

**Bagian B: Reviewer Memeriksa**

7. Logout. Login sebagai `pb.reviewer` (ROLE-AKUN-CTL)
8. Navigasi ke queue review atau langsung buka `/master/periode-buku/{id-2026-M06}`
9. Verifikasi panel workflow: Step 2 aktif, nama reviewer tersedia
10. Isi komentar: `"Review OK — tanggal mulai dan akhir sesuai kalender"`
11. Klik **"Setujui Review"**

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status berubah menjadi **PENDING_APPROVAL** | |
| 2 | Toast sukses: `"Review berhasil. Periode 2026-M06 menunggu persetujuan CFO."` | |
| 3 | Panel workflow: Step 2 berstatus "done" (centang hijau), Step 3 (Approver) aktif | |

**Verifikasi SQL Bagian B**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06');
-- Expected: 'PENDING_APPROVAL'
```

**Bagian C: Approver (CFO) Menyetujui dengan Step-up MFA**

12. Logout. Login sebagai `pb.approver` (ROLE-CFO)
13. Buka `/master/periode-buku/{id-2026-M06}`
14. Panel workflow: Step 3 (Approver) aktif dengan tombol "Setujui"
15. Klik **"Setujui"**
16. **Dialog MFA Step-up muncul** (DEC-027): sistem meminta konfirmasi MFA ulang
    - Masukkan TOTP/OTP
    - Klik **"Konfirmasi MFA"**
17. Isi komentar: `"Disetujui — periode Juni 2026 resmi dibuka"`
18. Klik **"Setujui"**

**Hasil yang Diharapkan — Bagian C**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Dialog MFA Step-up muncul sebelum approve dieksekusi | |
| 2 | Status berubah menjadi **APPROVED** | |
| 3 | Toast sukses: `"Periode 2026-M06 berhasil disetujui."` | |
| 4 | `mst.periode_buku.workflow_status` = APPROVED | |
| 5 | Panel workflow: semua step berstatus "done" | |
| 6 | Tombol "Edit" tidak tampil (APPROVED = read-only) | |
| 7 | Audit history menampilkan 3 event: SUBMIT, REVIEW, APPROVE | |

**Verifikasi SQL Bagian C**:
```sql
-- Workflow state
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06');
-- Expected: 'APPROVED'

-- Entity status sync
SELECT workflow_status FROM mst.periode_buku WHERE periode_id_kode = '2026-M06';
-- Expected: 'APPROVED'

-- Audit trail
SELECT action, actor_role FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06')
  AND action IN ('PERIODE_BUKU.SUBMIT', 'PERIODE_BUKU.REVIEW', 'PERIODE_BUKU.APPROVE')
ORDER BY event_time;
-- Expected: 3 baris berurutan SUBMIT → REVIEW → APPROVE

-- Signatures
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M06');
-- Expected: >= 3 (satu per action: submit, review, approve)
```

---

### TC-004: SoD — Maker Tidak Bisa Approve

**ID**: TC-004
**Aktor**: pb.maker (yang membuat / submit periode)
**Pre-condition**: Periode `2026-M07` dalam status PENDING_REVIEW atau PENDING_APPROVAL (submit sudah dilakukan pb.maker)
**Tujuan**: Verifikasi SoD enforcement — maker tidak bisa menjadi approver

**Langkah via UI**:

1. Login sebagai `pb.maker`
2. Buka detail `/master/periode-buku/{id-2026-M07}`
3. Jika ada Step 2 Reviewer atau Step 3 Approver yang aktif:
   - Panel harus menampilkan banner `"Anda adalah pembuat data ini dan tidak bisa menjadi reviewer/approver"`
   - Tombol "Setujui" dan "Tolak" harus **disabled** atau tidak tampil

**Langkah via API (bypass test — wajib dilakukan)**:

```bash
# Cari UUID dari 2026-M07
curl -s http://localhost:8080/api/v1/master/periode-buku?q=2026-M07 \
  -H "Authorization: Bearer <token_pb_maker>" | jq '.data[0].id'

# Coba approve langsung sebagai maker
curl -X POST http://localhost:8080/api/v1/master/periode-buku/{id-2026-M07}/approve \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_pb_maker>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":3}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: Banner SoD "Anda tidak bisa menjadi approver..." tampil | |
| 2 | UI: Tombol approve/review disabled untuk pb.maker | |
| 3 | API: HTTP 403 SOD_VIOLATION diterima | |
| 4 | API: `"error.code": "SOD_VIOLATION"` | |
| 5 | API: Pesan Bahasa Indonesia: `"Anda tidak bisa menjadi reviewer/approver untuk transaksi yang Anda buat sendiri."` | |
| 6 | Status workflow `2026-M07` TIDAK BERUBAH | |
| 7 | Tidak ada signature record dibuat untuk tindakan ini | |

---

### TC-005: Step-up MFA Wajib di Approve

**ID**: TC-005
**Aktor**: pb.approver (ROLE-CFO)
**Pre-condition**: Periode `2026-M08` dalam status PENDING_APPROVAL (sudah melewati submit + review)
**Tujuan**: Verifikasi DEC-027 — approve tanpa step-up MFA fresh ditolak

**Langkah via UI**:

1. Login sebagai `pb.approver` (ROLE-CFO)
2. Buka detail periode `2026-M08` dalam status PENDING_APPROVAL
3. Klik tombol **"Setujui"**
4. Dialog MFA Step-up muncul (sistem meminta ulang autentikasi)
5. **Batalkan** dialog MFA tanpa mengisi OTP
6. Verifikasi: status tidak berubah, toast warning muncul

**Langkah via API (tanpa step-up token)**:

```bash
# Token JWT standard tanpa stepup_verified_at (atau expired > 5 menit)
curl -X POST http://localhost:8080/api/v1/master/periode-buku/{id-2026-M08}/approve \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token_cfo_tanpa_stepup>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"signatureMethod":"JWT_STANDARD","rowVersion":3,"comment":"Coba tanpa step-up"}'
```

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | UI: Dialog MFA Step-up muncul sebelum approve dieksekusi | |
| 2 | UI: Jika dialog dibatalkan → toast warning `"Persetujuan memerlukan konfirmasi MFA"` | |
| 3 | UI: Status periode tetap PENDING_APPROVAL | |
| 4 | API: HTTP 403 diterima | |
| 5 | API: Error code terkait step-up requirement | |
| 6 | Workflow state tidak berubah (masih PENDING_APPROVAL) | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-M08');
-- Expected: 'PENDING_APPROVAL' (tidak berubah)
```

---

### TC-006: Optimistic Lock — Dua Tab Edit Bersamaan

**ID**: TC-006
**Aktor**: pb.maker (User A) + pb.reviewer (User B) membuka form edit periode yang sama
**Pre-condition**: Periode `2026-M09` dalam status DRAFT, `row_version = 1`
**Tujuan**: Verifikasi optimistic lock mencegah silent data loss

**Langkah-langkah**:

1. **User A** (pb.maker) buka tab `http://localhost:3001/master/periode-buku/{id-2026-M09}` → klik Edit. Terlihat `rowVersion: 1`.
2. **User B** (pb.reviewer) buka tab yang sama secara bersamaan. Terlihat `rowVersion: 1`.
3. **User A** ubah Catatan (atau field yang tersedia) → klik **"Simpan"**
   - Expected: berhasil, `row_version` menjadi `2`.
4. **User B** ubah field berbeda → klik **"Simpan"** (masih menggunakan `rowVersion: 1` yang stale)

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | User A: HTTP 200 berhasil, response mengandung `rowVersion: 2` | |
| 2 | User B: HTTP 409 CONFLICT diterima | |
| 3 | User B: Toast error merah persistent: `"Periode buku 2026-M09 telah diubah oleh user lain. Muat ulang halaman untuk melihat data terbaru."` | |
| 4 | Database: `row_version` = 2 (bukan override User B) | |
| 5 | User B: Form tidak ter-reset secara otomatis (data draft User B masih ada) | |

**Verifikasi SQL**:
```sql
SELECT row_version FROM mst.periode_buku WHERE periode_id_kode = '2026-M09';
-- Expected: 2 (bukan nilai dari User B)
```

---

### TC-007: Export CSV dengan Filter Aktif

**ID**: TC-007
**Aktor**: pb.maker (permission `periode.read`)
**Pre-condition**: TC-001 sudah dijalankan; ada 12 baris BULANAN tahun 2026
**Tujuan**: Verifikasi export CSV menghormati filter aktif (ux-patterns §1.4)

**Langkah-langkah**:

1. Login sebagai `pb.maker`
2. Navigasi ke `/master/periode-buku`
3. Aktifkan filter:
   - **Tahun Buku**: `2026`
   - **Tipe Periode**: `BULANAN`
4. Verifikasi list menampilkan 12 baris
5. Klik **"Export" → "CSV"**
6. File ter-download

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | File ter-download dengan nama format `periode-buku-YYYYMMDD.csv` | |
| 2 | File mengandung tepat **12 baris data** (+ 1 header row) | |
| 3 | Header row: `Kode Periode,Tipe Periode,Tahun Buku,Bulan,Triwulan,Tanggal Mulai,Tanggal Akhir,Status Periode,Status Workflow` | |
| 4 | File hanya berisi baris BULANAN (tidak ada TRIWULANAN atau TAHUNAN) | |
| 5 | File encoding UTF-8 with BOM (buka di Excel Indonesia — tanpa karakter aneh) | |
| 6 | `Content-Disposition` header: `attachment; filename="periode-buku-YYYYMMDD.csv"` | |
| 7 | `X-Total-Rows` header bernilai `12` | |
| 8 | Audit log: `action='PERIODE_BUKU.EXPORT'` dengan `row_count=12`, `filters={tipe_periode: 'BULANAN', tahun_buku: '2026'}` | |

**Verifikasi SQL**:
```sql
SELECT after_jsonb FROM aud.audit_log
WHERE action = 'PERIODE_BUKU.EXPORT'
ORDER BY event_time DESC LIMIT 1;
-- Expected: {format: 'csv', row_count: 12, filters: {tipe_periode: 'BULANAN', tahun_buku: '2026'}}
```

---

### TC-008: Reject Flow — Returned dan Re-submit

**ID**: TC-008
**Aktor**: pb.maker, pb.reviewer
**Pre-condition**: Periode `2026-Q3` dalam status PENDING_REVIEW (submit dilakukan pb.maker)
**Tujuan**: Verifikasi alur penolakan: REJECTED → RETURNED di UI → Maker revisi → re-submit

**Bagian A: Reviewer Menolak**

1. Login sebagai `pb.reviewer`
2. Buka detail `/master/periode-buku/{id-2026-Q3}` dalam status PENDING_REVIEW
3. Klik **"Tolak"**
4. Dialog konfirmasi muncul dengan input "Alasan Penolakan" (required, minimum 10 karakter)
5. Isi alasan: `"Tanggal akhir Q3 tidak sesuai dengan kalender fiskal perusahaan. Harap verifikasi kembali."`
6. Klik **"Lanjut Tolak"**

**Hasil yang Diharapkan — Bagian A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status `2026-Q3` berubah menjadi **RETURNED** (tampilan UI — internal DB: REJECTED) | |
| 2 | Komentar penolakan tersimpan di `sys.workflow_signature` | |
| 3 | Audit log: `action='PERIODE_BUKU.REJECT'` dengan komentar | |
| 4 | Toast sukses: `"Periode 2026-Q3 berhasil ditolak."` | |

**Verifikasi SQL Bagian A**:
```sql
SELECT current_state FROM sys.workflow_instance
WHERE entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = '2026-Q3');
-- Expected: 'REJECTED'

SELECT workflow_status FROM mst.periode_buku WHERE periode_id_kode = '2026-Q3';
-- Expected: 'REJECTED' (ditampilkan 'RETURNED' di UI via displayWorkflowStatus())
```

**Bagian B: Maker Melihat RETURNED dan Re-submit**

7. Logout. Login sebagai `pb.maker`
8. Buka detail `/master/periode-buku/{id-2026-Q3}`
9. Verifikasi: banner **"RETURNED"** tampil dengan alasan penolakan dari reviewer
10. Tombol **"Edit"** tersedia (RETURNED = bisa diedit kembali)
11. Klik **"Edit"**, perbaiki data (misal ubah catatan)
12. Klik **"Simpan"**
13. Klik **"Kirim untuk Review"** lagi

**Hasil yang Diharapkan — Bagian B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Banner RETURNED tampil dengan komentar penolakan dari reviewer | |
| 2 | Tombol Edit tersedia untuk pb.maker | |
| 3 | Setelah edit → simpan berhasil | |
| 4 | Setelah re-submit → status kembali **PENDING_REVIEW** | |
| 5 | Audit history menampilkan urutan: SUBMIT → REJECT → UPDATE → SUBMIT (siklus kedua) | |

---

### TC-009: Soft Delete dengan Referensi Aktif

**ID**: TC-009
**Aktor**: pb.maker (ROLE-AKUN)
**Pre-condition**: Terdapat periode yang sudah digunakan oleh data (kurs, transaksi, jurnal)
**Tujuan**: Verifikasi soft-delete ditolak dengan pesan yang jelas ketika ada referensi aktif

**Setup** (jika referensi belum ada, buat secara manual):
```sql
-- Contoh: seed kurs yang mereferensikan periode_buku
-- Ganti {periode_id} dengan UUID periode yang akan dites
-- INSERT INTO mst.kurs_history (periode_id, ...) VALUES ({periode_id}, ...);
-- ATAU buat via UI setelah periode di-approve.
```

**Langkah-langkah**:

1. Login sebagai `pb.maker`
2. Navigasi ke `/master/periode-buku`
3. Pilih periode yang memiliki referensi aktif (kurs, transaksi, atau jurnal)
4. Klik tombol **"Hapus"** (atau ikon hapus) pada baris periode tersebut
5. Dialog konfirmasi hapus muncul: `"Yakin ingin menghapus periode ini?"`
6. Klik **"Hapus"** di dialog

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 409 CONFLICT diterima | |
| 2 | `"error.code": "ENTITY_IN_USE"` | |
| 3 | Toast error merah persistent: `"Periode buku {...} tidak bisa dihapus karena masih digunakan oleh N entitas..."` | |
| 4 | Jumlah referensi (N) tersebut dalam pesan error | |
| 5 | Periode tetap ada di database (`deleted_at IS NULL`) | |
| 6 | Tombol "Hapus" kembali aktif (form tidak locked) | |

**Verifikasi SQL**:
```sql
SELECT deleted_at FROM mst.periode_buku WHERE periode_id_kode = '{kode-periode-tes}';
-- Expected: NULL (tidak ter-delete)
```

**Langkah tambahan — soft delete tanpa referensi**:

7. Pilih periode DRAFT yang BELUM punya referensi (misal periode baru dari TC-001 yang belum dipakai)
8. Klik "Hapus" → konfirmasi
9. Expected: HTTP 200, periode tidak muncul di list, `deleted_at IS NOT NULL` di DB

---

## Cleanup / Rollback

Setelah UAT selesai, jalankan:

```sql
-- Hapus workflow instances terkait periode buku test
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
    SELECT id FROM mst.periode_buku WHERE tahun_buku IN (2026, 2024, 2099)
);

-- Hapus periode buku test
DELETE FROM mst.periode_buku WHERE tahun_buku IN (2026, 2024, 2099);

-- Catatan: audit_log TIDAK boleh dihapus (DEC-018, retensi 10+10 tahun).
-- Di environment UAT yang dedicated, audit log boleh di-truncate untuk reset.
-- Di production: JANGAN hapus aud.audit_log.
```

---

## Checklist Ringkas QA Gate Phase 3 → 4

| Test Case | Deskripsi | Status |
|---|---|---|
| TC-001 | Generate 17 periode 2026 (idempoten) | |
| TC-002 | Validasi format kode — bulan 13, Q5, tanpa dash | |
| TC-003 | 4-eyes workflow: DRAFT → APPROVED (dengan step-up MFA di approve) | |
| TC-004 | SoD: maker tidak bisa approve via API langsung | |
| TC-005 | Step-up MFA wajib: approve tanpa MFA fresh ditolak (DEC-027) | |
| TC-006 | Optimistic lock: dua tab edit bersamaan → tab B 409 | |
| TC-007 | Export CSV: filter tipe=BULANAN tahun=2026 menghasilkan 12 baris | |
| TC-008 | Reject flow: RETURNED → maker revisi → re-submit → PENDING_REVIEW | |
| TC-009 | Soft delete dengan FK referensi aktif → 409 ENTITY_IN_USE | |

**SEMUA TC-001 s.d. TC-009 harus PASS sebelum Phase 4 (APP-B Transaction Lifecycle) dimulai.**

---

## Sign-off UAT

| Test Case | Status (PASS/FAIL/BLOCKED) | Tester | Tanggal | Catatan |
|---|---|---|---|---|
| TC-001 | | | | |
| TC-002 | | | | |
| TC-003 | | | | |
| TC-004 | | | | |
| TC-005 | | | | |
| TC-006 | | | | |
| TC-007 | | | | |
| TC-008 | | | | |
| TC-009 | | | | |

**QA Sign-off**: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ Tanggal: \_\_\_\_\_\_\_\_\_

**Finance Controller Sign-off** (TC-003, TC-005): \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ Tanggal: \_\_\_\_\_\_\_\_\_

**CFO Sign-off** (TC-003 approve step DEC-027): \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ Tanggal: \_\_\_\_\_\_\_\_\_
