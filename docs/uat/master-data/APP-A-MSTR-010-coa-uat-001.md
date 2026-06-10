# UAT Script — Chart of Accounts (CoA)
**ID**: APP-A-MSTR-010-UAT-001
**Modul**: APP-A Master Data
**Fitur**: Manajemen Bagan Akun (`mst.chart_of_accounts`)
**Tipe Workflow**: 4-eyes (Maker → Reviewer → Approver)
**Revisi**: 1.0 | **Tanggal**: 2026-06-04
**Author**: qa-engineer

---

## Prasyarat

### Infrastruktur
- Stack dev berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Migrasi terakhir diterapkan: `go run ./cmd/migrator up` (termasuk migration 0016)
- Browser: Chrome/Firefox modern, developer tools tersedia

### Seed data pengguna (3 user berbeda untuk penegakan SoD)

| Username | Role | Dipakai pada |
|---|---|---|
| `uat_coa_maker` | ROLE-AKUN | TC-001, TC-002, TC-003, TC-005, TC-006, TC-007 |
| `uat_coa_reviewer` | ROLE-AKUN-CTL | TC-004 (langkah review) |
| `uat_coa_approver` | ROLE-AKUN-CTL | TC-004 (langkah approve), TC-007 |

Buat ketiga user via halaman `/admin/users` (ROLE-IT-ADMIN) atau via Keycloak admin console sebelum menjalankan UAT.

### Periode buku
Tidak diperlukan (CoA adalah data master yang tidak bergantung pada periode buku aktif).

### Data awal
Tidak ada data CoA yang diperlukan sebelum TC-001 (data CoA yang dibuat di TC-001 dipakai oleh TC-002).

---

## TC-001 — Buat akun root "1" ASET DEBIT

**Tujuan**: Verifikasi pembuatan akun CoA level root tanpa parent.
**Actor**: `uat_coa_maker` (ROLE-AKUN)
**Pre-kondisi**: Kode akun "1" belum ada di sistem.

### Langkah-langkah

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke menu **Master Data → Bagan Akun (CoA)**.
3. Klik tombol **Tambah Akun Baru**.
4. Isi formulir:
   - **Kode Akun**: `1`
   - **Nama Akun**: `ASET`
   - **Tipe Akun**: `ASET`
   - **Sub Tipe Akun**: `ROOT`
   - **Posisi Normal**: `DEBIT`
   - **Sumber CoA**: `MANUAL`
   - **Tanggal Mulai Aktif**: `2026-01-01`
   - **Parent Akun Kode**: (kosongkan)
5. Klik **Simpan**.

### Hasil yang diharapkan

- Toast notifikasi hijau muncul: _"Akun 1 berhasil dibuat. Menunggu review."_
- Akun muncul di daftar CoA dengan:
  - `workflowStatus` = **DRAFT**
  - `kodeAkun` = `1`
  - `tipeAkun` = `ASET`
  - `posisiNormal` = `DEBIT`
- Baris `aud.audit_log` dengan `action = 'CHART_OF_ACCOUNTS.CREATE'` dan `entity_id` = UUID akun tersebut.

### Cleanup
Tidak diperlukan — akun ini dipakai sebagai parent di TC-002.

---

## TC-002 — Buat akun child "1.1.01" dengan parent "1"

**Tujuan**: Verifikasi bahwa parent harus APPROVED sebelum bisa direferensikan oleh child.
**Actor**: `uat_coa_maker` (ROLE-AKUN)
**Pre-kondisi**: Akun "1" ada di sistem dari TC-001 (dalam status DRAFT atau APPROVED).

### Langkah-langkah (skenario A — parent DRAFT, harus gagal)

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun → Tambah Akun Baru**.
3. Isi formulir:
   - **Kode Akun**: `1.1.01`
   - **Nama Akun**: `Kas dan Setara Kas`
   - **Tipe Akun**: `ASET`
   - **Sub Tipe Akun**: `LANCAR`
   - **Posisi Normal**: `DEBIT`
   - **Sumber CoA**: `MANUAL`
   - **Tanggal Mulai Aktif**: `2026-01-01`
   - **Parent Akun Kode**: `1`
4. Klik **Simpan**.

**Hasil yang diharapkan (Skenario A — parent masih DRAFT)**:

- Toast merah persisten: _"Parent akun dengan kode '1' tidak ditemukan atau belum disetujui (APPROVED)."_
- Error code: `COA_PARENT_NOT_FOUND` (422)
- Tidak ada baris baru di `mst.chart_of_accounts`.

### Langkah-langkah (skenario B — parent APPROVED, harus berhasil)

1. Terlebih dahulu selesaikan alur 4-eyes untuk akun "1" (jalankan TC-004 dulu pada akun "1").
2. Ulangi langkah 1–5 di atas.

**Hasil yang diharapkan (Skenario B — parent APPROVED)**:

- Toast hijau: _"Akun 1.1.01 berhasil dibuat. Menunggu review."_
- `workflowStatus` = **DRAFT**.
- `parentAkunId` terisi UUID akun "1".

### Audit checks
Setelah skenario B berhasil, verifikasi di `aud.audit_log`:
- `action = 'CHART_OF_ACCOUNTS.CREATE'`
- `after_value` memuat `kode_akun = '1.1.01'` dan `parent_akun_id` yang sesuai.

### Cleanup
Tidak diperlukan (akun dipakai di TC selanjutnya jika diperlukan).

---

## TC-003 — Import XLSX standar template

**Tujuan**: Verifikasi alur import XLSX asinkron: upload → 202 jobId → progress panel → 50 baris DRAFT.
**Actor**: `uat_coa_maker` (ROLE-AKUN)
**Pre-kondisi**: File XLSX template sesuai kolom standar tersedia (kolom A–H: `kode_akun`, `nama_akun`, `tipe_akun`, `sub_tipe_akun`, `kategori_investasi`, `mata_uang_native`, `posisi_normal`, `parent_akun_kode`).

### Persiapan file
Siapkan file `coa-template-uat.xlsx` dengan:
- Baris 1: header sesuai template
- Baris 2–51: 50 baris data valid dengan kode akun unik (mis. `9001` hingga `9050`)
- Semua baris menggunakan `tipe_akun` = `ASET`, `posisi_normal` = `DEBIT`

### Langkah-langkah

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun → Import XLSX**.
3. Upload file `coa-template-uat.xlsx`.
4. Isi field **Sumber CoA**: `UAT_IMPORT`.
5. Klik **Import**.

### Hasil yang diharapkan

- Respons HTTP 202 diterima segera setelah klik (browser tidak hang).
- Komponen `<JobProgressPanel>` muncul:
  - Progress bar mulai bergerak dari 0%.
  - Label langkah berubah: _"Parsing file XLSX..."_ → _"Ditemukan 50 baris. Memulai validasi dan import..."_ → _"Selesai"_.
  - ETA ditampilkan.
- Setelah selesai: toast hijau _"Import CoA selesai. 50 baris berhasil diimpor."_ + link _"Lihat daftar →"_.
- Di daftar CoA: 50 baris baru muncul dengan `workflowStatus = DRAFT` dan `sumberCoa = UAT_IMPORT`.
- `aud.audit_log` memuat baris `action = 'CHART_OF_ACCOUNTS.IMPORT_XLSX'` dengan `rows_done = 50`.

### Audit checks

```sql
SELECT after_value->>'rows_done', after_value->>'sumber_coa'
FROM aud.audit_log
WHERE action = 'CHART_OF_ACCOUNTS.IMPORT_XLSX'
ORDER BY event_time DESC
LIMIT 1;
-- Expected: rows_done = 50, sumber_coa = 'UAT_IMPORT'
```

### Cleanup (opsional setelah TC selesai)
```sql
DELETE FROM mst.chart_of_accounts WHERE sumber_coa = 'UAT_IMPORT';
```

---

## TC-004 — 4-eyes happy path (DRAFT → APPROVED)

**Tujuan**: Verifikasi alur lengkap 4-eyes: Maker submit → Reviewer review → Approver approve → status APPROVED.
**Actor**:
- `uat_coa_maker` (ROLE-AKUN) — langkah Submit
- `uat_coa_reviewer` (ROLE-AKUN-CTL) — langkah Review
- `uat_coa_approver` (ROLE-AKUN-CTL) — langkah Approve

**Pre-kondisi**: Akun CoA dalam status DRAFT tersedia (mis. dari TC-001 atau TC-003).

### Langkah-langkah

#### Langkah 1 — Submit (Maker)

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun**, temukan akun `1` (status DRAFT).
3. Klik **Ajukan Review** (Submit).
4. Isi **Komentar**: _"Siap direview"_.
5. Klik **Kirim**.

**Hasil yang diharapkan**:
- Toast hijau: _"Akun 1 berhasil diajukan untuk review."_
- Status berubah menjadi **PENDING_REVIEW**.
- `aud.audit_log`: `action = 'CHART_OF_ACCOUNTS.SUBMIT'`.

#### Langkah 2 — Review (Reviewer)

1. Logout, login sebagai `uat_coa_reviewer`.
2. Navigasi ke **Master Data → Bagan Akun → Antrian Review**, temukan akun `1`.
3. Buka detail akun, klik **Review & Setujui Lanjut**.
4. Isi **Komentar**: _"Review OK"_.
5. Klik **Tanda Tangan & Kirim ke Approver**.

**Hasil yang diharapkan**:
- Toast hijau: _"Review akun 1 berhasil. Menunggu persetujuan approver."_
- Status berubah menjadi **PENDING_APPROVAL**.
- `aud.audit_log`: `action = 'CHART_OF_ACCOUNTS.REVIEW'`.

#### Langkah 3 — Approve (Approver)

1. Logout, login sebagai `uat_coa_approver`.
2. Navigasi ke **Master Data → Bagan Akun → Antrian Persetujuan**, temukan akun `1`.
3. Buka detail, klik **Setujui**.
4. Isi **Komentar**: _"Disetujui sesuai kebijakan"_.
5. Klik **Tanda Tangan & Setujui**.

**Hasil yang diharapkan**:
- Toast hijau: _"Akun 1 berhasil disetujui."_
- Status berubah menjadi **APPROVED**.
- `mst.chart_of_accounts.workflow_status = 'APPROVED'` (verifikasi di DB).
- `aud.audit_log`: `action = 'CHART_OF_ACCOUNTS.APPROVE'`.
- `sys.workflow_signature`: minimal 3 baris (submit + review + approve) untuk instance ini.

### Audit checks

```sql
-- Verifikasi 3 event di audit_log
SELECT action, actor_user_id, event_time
FROM aud.audit_log
WHERE entity_type = 'mst.chart_of_accounts'
  AND entity_id = '<UUID akun 1>'
ORDER BY event_time;
-- Expected: CHART_OF_ACCOUNTS.CREATE, CHART_OF_ACCOUNTS.SUBMIT,
--           CHART_OF_ACCOUNTS.REVIEW, CHART_OF_ACCOUNTS.APPROVE

-- Verifikasi signature count
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = '<UUID akun 1>';
-- Expected: >= 3
```

---

## TC-005 — Kode format invalid (huruf) → inline error

**Tujuan**: Verifikasi bahwa kode akun berformat non-numerik menghasilkan inline validation error.
**Actor**: `uat_coa_maker` (ROLE-AKUN)
**Pre-kondisi**: Tidak ada.

### Langkah-langkah

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun → Tambah Akun Baru**.
3. Isi **Kode Akun**: `ABC.XYZ` (huruf — bukan angka).
4. Isi field lainnya dengan nilai valid:
   - **Nama Akun**: `Akun Invalid`
   - **Tipe Akun**: `ASET`
   - **Sub Tipe Akun**: `TEST`
   - **Posisi Normal**: `DEBIT`
   - **Sumber CoA**: `MANUAL`
   - **Tanggal Mulai Aktif**: `2026-01-01`
5. Klik **Simpan**.

### Hasil yang diharapkan

- Field **Kode Akun** ditandai merah (invalid) dengan pesan inline: _"Kode akun harus berupa angka dengan titik sebagai separator hierarki, contoh: 1.1.01.001"_.
- Toast merah persisten muncul: _"1 field bermasalah — lihat form di bawah."_ + error code `COA_INVALID_KODE_FORMAT` atau `VALIDATION_FAILED`.
- Tidak ada baris baru di `mst.chart_of_accounts`.
- Form tidak ter-reset (data yang sudah diisi tetap ada).

### Audit checks
Tidak ada baris `aud.audit_log` untuk aksi gagal ini (pre-validation error, tidak sampai ke DB).

---

## TC-006 — Parent tidak APPROVED → toast error

**Tujuan**: Verifikasi bahwa referensi parent yang belum APPROVED menghasilkan toast error yang jelas.

> Scenario ini adalah verifikasi UI dari test integrasi `TestCoA_ParentNotApproved_Returns422`.

**Actor**: `uat_coa_maker` (ROLE-AKUN)
**Pre-kondisi**: Akun "2" ada di sistem dengan `workflow_status = DRAFT` (buat dulu via TC-001 dengan kode "2", tanpa di-approve).

### Langkah-langkah

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun → Tambah Akun Baru**.
3. Isi formulir:
   - **Kode Akun**: `2.1`
   - **Nama Akun**: `Sub Akun Liabilitas`
   - **Tipe Akun**: `LIABILITAS`
   - **Sub Tipe Akun**: `LANCAR`
   - **Posisi Normal**: `KREDIT`
   - **Sumber CoA**: `MANUAL`
   - **Tanggal Mulai Aktif**: `2026-01-01`
   - **Parent Akun Kode**: `2` (masih DRAFT)
4. Klik **Simpan**.

### Hasil yang diharapkan

- Toast merah persisten: _"Parent akun dengan kode '2' tidak ditemukan atau belum disetujui (APPROVED). Selesaikan workflow akun parent terlebih dahulu."_
- Error code: `COA_PARENT_NOT_FOUND` (HTTP 422).
- Tidak ada baris baru di DB.

---

## TC-007 — SoD: maker tidak bisa approve akun miliknya sendiri

**Tujuan**: Verifikasi penegakan SoD di level API — maker yang memiliki permission `chart_of_accounts.approve` tetap tidak bisa menyetujui akun yang ia buat sendiri.
**Actor**:
- `uat_coa_maker` — membuat dan submit akun
- `uat_coa_reviewer` — review
- `uat_coa_maker` — mencoba approve (harus gagal)
- `uat_coa_approver` — approve yang sah

**Pre-kondisi**: Tidak ada akun CoA baru. User `uat_coa_maker` memiliki permission `chart_of_accounts.approve` (ditambahkan sementara untuk tujuan UAT ini via Keycloak — simulasi bypass).

### Langkah-langkah

1. Login sebagai `uat_coa_maker`, buat akun baru kode `3` nama `LIABILITAS` tipe `LIABILITAS` posisi `KREDIT`.
2. Submit akun "3" (klik **Ajukan Review**).
3. Logout, login sebagai `uat_coa_reviewer`, review akun "3" (klik **Review & Kirim ke Approver**).
4. Logout, login kembali sebagai `uat_coa_maker`.
5. Navigasi ke antrian approval, temukan akun "3" (seharusnya tidak ada di antrian karena SoD).
6. Coba akses langsung URL: `POST /api/v1/master/coa/<id>/approve` via curl atau Postman dengan JWT `uat_coa_maker`:

```bash
curl -X POST https://blips.tugu-re.com/api/v1/master/coa/<UUID_AKUN_3>/approve \
  -H "Authorization: Bearer <JWT_MAKER>" \
  -H "Content-Type: application/json" \
  -d '{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Bypass attempt"}'
```

### Hasil yang diharapkan

- HTTP 403 dengan body:
  ```json
  {
    "error": {
      "code": "SOD_VIOLATION",
      "message": "Maker tidak bisa menjadi approver untuk entitas yang sama."
    }
  }
  ```
- `workflow_status` di DB tetap `PENDING_APPROVAL` (tidak berubah ke APPROVED).
- UI antrian approver: akun "3" tidak tampil di antrian `uat_coa_maker`.

#### Approval yang sah (langkah verifikasi positif)

7. Logout, login sebagai `uat_coa_approver`.
8. Temukan akun "3" di antrian, klik **Setujui**.
9. Verifikasi status berubah ke `APPROVED`.

### Audit checks

```sql
-- Verifikasi tidak ada event APPROVE dengan actor = uat_coa_maker
SELECT actor_user_id, action, event_time FROM aud.audit_log
WHERE entity_type = 'mst.chart_of_accounts'
  AND entity_id = '<UUID_AKUN_3>'
  AND action = 'CHART_OF_ACCOUNTS.APPROVE';
-- Expected: hanya 1 baris dengan actor = UUID uat_coa_approver, BUKAN uat_coa_maker
```

### Cleanup
```sql
DELETE FROM sys.workflow_instance WHERE entity_id IN (
  SELECT id FROM mst.chart_of_accounts WHERE kode_akun = '3'
);
DELETE FROM mst.chart_of_accounts WHERE kode_akun = '3';
```

---

## TC-008 — Export CSV dengan filter tipe_akun=ASET

**Tujuan**: Verifikasi bahwa export CSV menghormati filter aktif, menghasilkan file dengan hanya baris `tipe_akun = ASET`, dan audit export tercatat.
**Actor**: `uat_coa_maker` (ROLE-AKUN) — read permission cukup.
**Pre-kondisi**: Ada minimal 1 baris CoA dengan `tipe_akun = ASET` dan minimal 1 baris dengan `tipe_akun != ASET` (mis. `LIABILITAS`) di sistem.

### Langkah-langkah

1. Login sebagai `uat_coa_maker`.
2. Navigasi ke **Master Data → Bagan Akun**.
3. Di panel filter, pilih **Tipe Akun = ASET**.
4. Klik **Export ▾ → CSV**.
5. Tunggu file ter-download (untuk dataset < 10k baris: download langsung; untuk dataset besar: cek notifikasi async).

### Hasil yang diharapkan

- File `chart-of-accounts-YYYYMMDD.csv` ter-download.
- Header baris 1: `Kode Akun, Nama Akun, Tipe Akun, Sub Tipe Akun, ...`
- **Semua baris** di file memiliki kolom "Tipe Akun" = `ASET`.
- Tidak ada baris dengan `tipe_akun = LIABILITAS` (atau tipe lain).
- Header HTTP response: `Content-Type: text/csv; charset=utf-8`, `Content-Disposition: attachment; filename="chart-of-accounts-YYYYMMDD.csv"`.

### Audit checks

```sql
SELECT after_value->>'format', after_value->>'row_count',
       after_value->'filters'
FROM aud.audit_log
WHERE action = 'CHART_OF_ACCOUNTS.EXPORT'
ORDER BY event_time DESC
LIMIT 1;
-- Expected: format='csv', filters memuat filter[tipe_akun]=ASET
```

### Cleanup
Tidak diperlukan (export tidak mengubah data).

---

## Ringkasan test coverage

| TC | Skenario | Regression priority | AC yang diverifikasi |
|---|---|---|---|
| TC-001 | Buat akun root tanpa parent | §3 (staging DRAFT) | Akun baru masuk DRAFT, audit CREATE |
| TC-002 | Akun child — parent harus APPROVED | §5 (parent integrity) | COA_PARENT_NOT_FOUND jika DRAFT |
| TC-003 | Import XLSX async 50 baris | §3, §8 (async + idempotency) | 202 jobId, progress panel, rows DRAFT |
| TC-004 | Alur 4-eyes lengkap | §3, §6 (workflow, SoD) | DRAFT→APPROVED, hook sync, audit, sig |
| TC-005 | Kode format invalid | §1 (validation) | 422 COA_INVALID_KODE_FORMAT, inline |
| TC-006 | Parent tidak APPROVED | §5 | 422 COA_PARENT_NOT_FOUND, toast |
| TC-007 | SoD maker ≠ approver | §6 (SoD API-level) | 403 SOD_VIOLATION, status tidak berubah |
| TC-008 | Export CSV + filter | §1, §2 (reproducibility) | Filter dihormati, audit EXPORT |
