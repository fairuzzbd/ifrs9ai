# UAT Script — Instrumen Master Data (APP-A-MSTR-013)
**Modul**: APP-A Master Data — mst.instrumen  
**Versi**: 1.0  
**Tanggal**: 2026-06-05  
**Author**: qa-engineer  
**Status**: READY FOR EXECUTION

---

## Referensi Dokumen
- FSD-APP-A-MASTER-v1.1.docx §4 (Instrumen)
- ERD-BLIPS-IFRS9-v1.2.docx — mst.instrumen
- SoW_v1.4.docx §2.1 (PSAK 71 klasifikasi)
- BRD_BLIPS_IFRS9_v1.1.docx §3 (RACI)
- security-baseline.md (SoD enforcement)

---

## Prasyarat Global

### 1. Data Master (harus di-seed SEBELUM mulai skenario)

Jalankan query berikut di DB dev/UAT untuk memastikan data prasyarat ada:

```sql
-- Pastikan workflow_status kolom ada
ALTER TABLE mst.counterparty ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) DEFAULT 'APPROVED';
ALTER TABLE mst.portofolio   ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) DEFAULT 'APPROVED';

-- Counterparty APPROVED
INSERT INTO mst.counterparty (id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
    eligible_lps_flag, status, workflow_status, created_at, created_by)
VALUES
('AA000001-0000-0000-0000-000000000001', 'CP-UAT-001', 'Bank UAT Mandiri',  'BANK',             'SENIOR_UNSECURED', TRUE,  'AKTIF', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('AA000001-0000-0000-0000-000000000002', 'CP-UAT-002', 'Schroder UAT MI',   'MANAJER_INVESTASI','SENIOR_UNSECURED', FALSE, 'AKTIF', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('AA000001-0000-0000-0000-000000000003', 'CP-UAT-003', 'StanChart Kustodian','BANK_KUSTODIAN',   'SENIOR_UNSECURED', FALSE, 'AKTIF', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('AA000001-0000-0000-0000-000000000004', 'CP-UAT-004', 'BCA Tbk Emiten',    'EMITEN_SAHAM',     'SENIOR_UNSECURED', FALSE, 'AKTIF', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('AA000001-0000-0000-0000-000000000005', 'CP-UAT-DRAFT','Bank Draft (DRAFT)','BANK',             'SENIOR_UNSECURED', FALSE, 'AKTIF', 'DRAFT',    now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- Portofolio APPROVED
INSERT INTO mst.portofolio (id, kode_portofolio, nama, bm_category_default, workflow_status, created_at, created_by)
VALUES
('BB000001-0000-0000-0000-000000000001', 'PORT-UAT-HTC',   'Treasury Liquidity UAT', 'HTC',   'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('BB000001-0000-0000-0000-000000000002', 'PORT-UAT-HTCS',  'Investment Liquidity UAT','HTCS', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('BB000001-0000-0000-0000-000000000003', 'PORT-UAT-OTHER', 'Trading Portfolio UAT',  'OTHER', 'APPROVED', now(), '00000000-0000-0000-0000-000000000001'),
('BB000001-0000-0000-0000-000000000004', 'PORT-UAT-DRAFT', 'Draft Porto',            'HTC',   'DRAFT',    now(), '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- Pastikan IDR APPROVED
UPDATE mst.mata_uang SET workflow_status = 'APPROVED' WHERE kode_mata_uang = 'IDR';
```

### 2. User Prasyarat (3 user berbeda — SoD enforcement)

| User | Username | Role | Digunakan pada |
|---|---|---|---|
| U-MAKER | treasury.maker.uat@tugu-re.com | ROLE-MAKER-TR | Membuat instrumen |
| U-REVIEWER | risk.officer.uat@tugu-re.com | ROLE-RISK | Review instrumen |
| U-APPROVER | treasury.approver.uat@tugu-re.com | ROLE-APPR-TR | Approve instrumen |

Pastikan ketiga user login via Keycloak dan memiliki JWT valid.

### 3. Environment
- URL: `https://blips-uat.tugu-re.com` (atau `http://localhost:3000` di dev)
- Menu: **Master Data → Instrumen**
- Dev stack: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`

### 4. Rollback / Cleanup
Setelah selesai UAT, jalankan:
```sql
DELETE FROM sys.workflow_instance WHERE entity_type = 'INSTRUMEN'
  AND entity_id IN (SELECT id FROM mst.instrumen WHERE kode_instrumen LIKE 'UAT-%');
DELETE FROM aud.audit_log WHERE entity_type = 'mst.instrumen'
  AND entity_id IN (SELECT id FROM mst.instrumen WHERE kode_instrumen LIKE 'UAT-%');
DELETE FROM mst.instrumen WHERE kode_instrumen LIKE 'UAT-%';
```

---

## Skenario S-001: Buat Instrumen DEPOSITO — Happy Path

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Tujuan
Memverifikasi bahwa instrumen DEPOSITO dapat dibuat dengan data valid, tersimpan
dalam state DRAFT, dan audit log tercatat.

### Langkah-langkah

1. Login sebagai **U-MAKER**.
2. Navigasi ke **Master Data → Instrumen → Buat Instrumen**.
3. Isi form dengan data berikut:

   | Field | Nilai |
   |---|---|
   | Kode Instrumen | `UAT-DEP-001` |
   | Tipe Instrumen | `DEPOSITO` |
   | Sub Tipe | `Deposito Berjangka` |
   | Nama | `Deposito BCA 3 Bulan — UAT` |
   | Counterparty | `CP-UAT-001 — Bank UAT Mandiri` |
   | Mata Uang | `IDR` |
   | Portofolio | `PORT-UAT-HTC — Treasury Liquidity UAT` |
   | Nominal | `5.000.000.000` (5 miliar) |
   | Tanggal Penempatan | `2026-06-01` |
   | Tanggal Jatuh Tempo | `2026-09-01` |
   | Kupon | `5.25` |
   | Frekuensi Bunga | `BULANAN` |
   | EIR Method | `Ya` |
   | Day Count Convention | `ACT/365` |

4. Klik **Simpan**.

### Hasil yang Diharapkan
- Toast hijau: *"Instrumen UAT-DEP-001 berhasil dibuat. Menunggu review."* + link "Lihat detail".
- Status Workflow: `DRAFT`.
- Data tersimpan di DB: `SELECT * FROM mst.instrumen WHERE kode_instrumen = 'UAT-DEP-001'`.
- Audit log: `SELECT * FROM aud.audit_log WHERE action = 'INSTRUMEN.CREATE' AND entity_type = 'mst.instrumen'` — ada 1 row.

### Pemeriksaan Audit
```sql
SELECT action, actor_user_id, entity_id, after_value
FROM aud.audit_log
WHERE action = 'INSTRUMEN.CREATE'
  AND after_value::text LIKE '%UAT-DEP-001%'
ORDER BY event_time DESC LIMIT 1;
```
Harus ada 1 row dengan `action = 'INSTRUMEN.CREATE'`.

---

## Skenario S-002: Buat Instrumen OBLIGASI — Happy Path

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Langkah-langkah

1. Login sebagai **U-MAKER**.
2. Navigasi ke **Master Data → Instrumen → Buat Instrumen**.
3. Isi form:

   | Field | Nilai |
   |---|---|
   | Kode Instrumen | `UAT-OBL-001` |
   | Tipe Instrumen | `OBLIGASI` |
   | Sub Tipe | `Obligasi Korporasi` |
   | Nama | `Obligasi Telkom 2026 — UAT` |
   | Counterparty | `CP-UAT-001 — Bank UAT Mandiri` |
   | Mata Uang | `IDR` |
   | Portofolio | `PORT-UAT-HTC` |
   | Nominal | `10.000.000.000` |
   | ISIN | `IDG000049902` |
   | Tanggal Penempatan | `2026-06-01` |
   | Tanggal Jatuh Tempo | `2031-06-01` |
   | Kupon | `7.50` |
   | Frekuensi Bunga | `SEMESTERAN` |
   | EIR Awal | `0.0748` |
   | Premium/Diskonto | `0` |
   | Biaya Transaksi | `50000000` |

4. Klik **Simpan**.

### Hasil yang Diharapkan
- Toast hijau dengan kode instrumen.
- `workflow_status = 'DRAFT'` di DB.
- Nominal tersimpan: `10000000000.0000` (NUMERIC(20,4), HALF_EVEN rounding).

---

## Skenario S-003: Buat Instrumen SAHAM — Field Kustodian Wajib

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Tujuan
Memverifikasi bahwa SAHAM menampilkan field `bankKustodianId` sebagai **wajib**,
dan menolak submit jika tidak diisi.

### Langkah-langkah — Negatif

1. Login sebagai **U-MAKER**.
2. Pilih tipe `SAHAM`.
3. Isi semua field KECUALI **Bank Kustodian**.
4. Klik **Simpan**.

### Hasil yang Diharapkan (negatif)
- Toast merah persisten: field `bankKustodianId` di-highlight merah.
- Error code: `VALIDATION_FAILED`.
- Tidak ada row baru di `mst.instrumen`.

### Langkah-langkah — Positif

5. Isi field **Bank Kustodian**: `CP-UAT-003 — StanChart Kustodian`.
6. Isi sisa data:

   | Field | Nilai |
   |---|---|
   | Kode Instrumen | `UAT-SAHAM-001` |
   | Sub Tipe | `Saham Biasa` |
   | Nama | `Saham BCA — UAT` |
   | Counterparty | `CP-UAT-004 — BCA Tbk Emiten` |
   | Jumlah Lot | `100` |
   | FVOCI Election | `Ya` |
   | Tanggal Penempatan | `2026-06-01` |

7. Klik **Simpan**.

### Hasil yang Diharapkan (positif)
- Toast hijau: *"Instrumen UAT-SAHAM-001 berhasil dibuat."*
- `fvoci_election = TRUE` di DB.
- Field `tanggal_jatuh_tempo` tidak wajib dan kosong (saham tidak jatuh tempo).

---

## Skenario S-004: Buat Instrumen REKSADANA — Kustodian + Manajer Investasi Wajib

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Tujuan
Memverifikasi bahwa REKSADANA memerlukan KEDUA field `bankKustodianId` DAN
`manajerInvestasiId`. Menolak jika salah satu atau keduanya kosong.

### Langkah-langkah — Negatif (manajer kosong)

1. Login sebagai **U-MAKER**.
2. Pilih tipe `REKSADANA`.
3. Isi `bankKustodianId` = `CP-UAT-003` TAPI kosongkan `manajerInvestasiId`.
4. Klik **Simpan**.

### Hasil yang Diharapkan (negatif)
- Toast merah: field `manajerInvestasiId` highlighted.
- Error code: `VALIDATION_FAILED` (field: `body.manajerInvestasiId`, rule: `required_for_tipe`).

### Langkah-langkah — Positif

5. Lengkapi data:

   | Field | Nilai |
   |---|---|
   | Kode Instrumen | `UAT-REKSA-001` |
   | Sub Tipe | `Reksadana Pendapatan Tetap` |
   | Nama | `Reksadana Schroder Fixed Income — UAT` |
   | Counterparty | `CP-UAT-001` |
   | Bank Kustodian | `CP-UAT-003 — StanChart Kustodian` |
   | Manajer Investasi | `CP-UAT-002 — Schroder UAT MI` |
   | Jumlah Lot | `5000` |
   | Tanggal Penempatan | `2026-06-01` |

6. Klik **Simpan**.

### Hasil yang Diharapkan (positif)
- Toast hijau: *"Instrumen UAT-REKSA-001 berhasil dibuat."*
- `manajer_investasi_id IS NOT NULL` dan `bank_kustodian_id IS NOT NULL` di DB.

---

## Skenario S-005: Validasi FK — Counterparty Belum APPROVED

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Tujuan
Memverifikasi bahwa memilih counterparty yang belum disetujui (status DRAFT)
ditolak dengan error eksplisit.

### Langkah-langkah

1. Login sebagai **U-MAKER**.
2. Buat instrumen DEPOSITO, isi **Counterparty** dengan `CP-UAT-DRAFT` (status DRAFT).
3. Klik **Simpan**.

### Hasil yang Diharapkan
- Toast merah persisten.
- Error code: `INSTRUMEN_COUNTERPARTY_NOT_APPROVED`.
- Pesan: *"Counterparty [UUID] tidak ditemukan atau belum APPROVED. Pastikan counterparty sudah disetujui sebelum membuat instrumen."*
- Tidak ada row baru di `mst.instrumen`.

### Catatan
Skenario yang sama berlaku untuk `portofolioId` (error: `INSTRUMEN_PORTOFOLIO_NOT_APPROVED`)
dan `mataUang` (error: `INSTRUMEN_MATA_UANG_NOT_APPROVED`).

---

## Skenario S-006: Klasifikasi Locked — Tidak Bisa Ubah via CRUD

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Prasyarat Tambahan
Instrumen `UAT-DEP-001` sudah di-lock klasifikasinya (setelah SPPI/BM workflow approved
di Phase 4). Simulasikan dengan:
```sql
UPDATE mst.instrumen
SET klasifikasi_locked_at = now(),
    klasifikasi_locked_by = '00000000-0000-0000-0000-000000000001',
    klasifikasi_psak71 = 'AC',
    bm_category = 'HTC'
WHERE kode_instrumen = 'UAT-DEP-001';
```

### Tujuan
Memverifikasi bahwa field klasifikasi menampilkan banner "Terkunci" dan menolak
perubahan via form edit.

### Langkah-langkah

1. Login sebagai **U-MAKER**.
2. Buka detail instrumen `UAT-DEP-001`.

### Pemeriksaan Visual
- Banner kuning: *"Klasifikasi PSAK 71 telah dikunci. Perubahan klasifikasi harus melalui workflow SPPI/BM."*
- Field `Klasifikasi PSAK71`, `BM Category`, `FVOCI Election` di-disable (tidak bisa diedit).
- Tombol Edit tetap tersedia untuk field lain (nama, kupon, dll).

3. Klik **Edit** → ubah **BM Category** ke `HTC_S` → klik **Simpan**.

### Hasil yang Diharapkan
- Toast merah: *"Instrumen UAT-DEP-001 telah dikunci klasifikasinya. Perubahan bm_category harus melalui workflow SPPI/BM."*
- Error code: `INSTRUMEN_KLASIFIKASI_LOCKED`.
- HTTP 423 Locked (atau 403 Forbidden tergantung mapping).
- DB tidak berubah: `bm_category` masih `HTC`.

4. Ubah **Nama** ke `Deposito BCA 3 Bulan — UAT Updated` → klik **Simpan**.

### Hasil yang Diharapkan (non-klasifikasi)
- Toast hijau: nama berhasil diperbarui.
- `nama` di DB berubah; `bm_category` tetap `HTC`.

---

## Skenario S-007: Workflow 4-Eyes — DRAFT → APPROVED

### Aktor: U-MAKER, U-REVIEWER, U-APPROVER (3 user berbeda)

### Tujuan
Memverifikasi alur lengkap Maker → Reviewer → Approver dengan SoD enforcement.

### Prasyarat
- Instrumen `UAT-DEP-002` sudah dibuat oleh U-MAKER (state DRAFT).
- Buat jika belum ada:
  ```
  POST /api/v1/master/instrumen
  body: { kodeInstrumen: "UAT-DEP-002", tipeInstrumen: "DEPOSITO", ... }
  ```

### Langkah-langkah

**SUBMIT — U-MAKER**

1. Login sebagai **U-MAKER**.
2. Buka detail instrumen `UAT-DEP-002`.
3. Klik **Ajukan untuk Review** → tambah komentar *"Data sudah dilengkapi sesuai dokumen penempatan."* → klik **Konfirmasi**.

### Hasil yang Diharapkan setelah SUBMIT
- Badge status: `MENUNGGU REVIEW`.
- Toast hijau: *"Instrumen UAT-DEP-002 berhasil diajukan untuk review."*
- DB: `workflow_status = 'PENDING_REVIEW'`.

**REVIEW — U-REVIEWER**

4. Logout. Login sebagai **U-REVIEWER**.
5. Navigasi ke **Master Data → Instrumen → Antrian Review**.
6. Pilih `UAT-DEP-002` → klik **Review**.
7. Periksa data instrumen. Klik **Setujui untuk Approval** → tambah komentar *"Review OK. Data sesuai."* → klik **Konfirmasi**.

### Hasil yang Diharapkan setelah REVIEW
- Badge status: `MENUNGGU APPROVAL`.
- DB: `workflow_status = 'PENDING_APPROVAL'`.

**SoD CHECK — U-REVIEWER mencoba Approve**

8. (Masih login sebagai U-REVIEWER) Coba klik **Approve** pada `UAT-DEP-002`.

### Hasil yang Diharapkan (SoD)
- Tombol **Approve** di-disable karena U-REVIEWER sudah bertindak sebagai reviewer.
- Jika bypass via API: `POST /api/v1/master/instrumen/{id}/approve` dengan JWT U-REVIEWER → `403 SOD_VIOLATION`.

**APPROVE — U-APPROVER**

9. Logout. Login sebagai **U-APPROVER**.
10. Navigasi ke **Master Data → Instrumen → Antrian Approval**.
11. Pilih `UAT-DEP-002` → klik **Approve** → komentar *"Disetujui. Siap untuk SPPI assessment."* → klik **Konfirmasi**.

### Hasil yang Diharapkan setelah APPROVE
- Badge status: `DISETUJUI`.
- Toast hijau: *"Instrumen UAT-DEP-002 berhasil disetujui."*
- DB: `workflow_status = 'APPROVED'`.
- Instrumen tidak bisa di-edit normal lagi (tombol Edit disabled).

### Pemeriksaan Audit
```sql
SELECT action, actor_user_id, event_time
FROM aud.audit_log
WHERE entity_type = 'mst.instrumen'
  AND entity_id = (SELECT id FROM mst.instrumen WHERE kode_instrumen = 'UAT-DEP-002')
ORDER BY event_time;
```
Harus ada minimal 3 event: `INSTRUMEN.SUBMIT`, `INSTRUMEN.REVIEW`, `INSTRUMEN.APPROVE`.

```sql
-- Verifikasi signature count >= 3
SELECT COUNT(*) FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON wi.id = ws.workflow_instance_id
WHERE wi.entity_type = 'INSTRUMEN'
  AND wi.entity_id = (SELECT id FROM mst.instrumen WHERE kode_instrumen = 'UAT-DEP-002');
```
Harus >= 3 signature records.

---

## Skenario S-008: SoD — Maker Tidak Bisa Approve via API Langsung

### Aktor: U-MAKER (percobaan bypass)

### Tujuan
Memverifikasi bahwa SoD di-enforce di layer API (service), bukan hanya di UI.
Test ini wajib dilakukan via curl/Postman dengan JWT U-MAKER.

### Prasyarat
- Instrumen `UAT-DEP-003` dalam state `PENDING_APPROVAL`
  (sudah melewati submit + review oleh user lain).

### Langkah-langkah

1. Dapatkan JWT U-MAKER dari Keycloak.
2. Kirim request approve secara langsung:

```bash
curl -X POST "https://blips-uat.tugu-re.com/api/v1/master/instrumen/{UAT-DEP-003-UUID}/approve" \
  -H "Authorization: Bearer {JWT_U-MAKER}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"comment":"bypass test","signatureMethod":"JWT_STANDARD","rowVersion":3}'
```

### Hasil yang Diharapkan
- HTTP 403 Forbidden.
- Response body:
  ```json
  {
    "error": {
      "code": "SOD_VIOLATION",
      "message": "...",
      "traceId": "..."
    }
  }
  ```
- DB: `workflow_status` TIDAK berubah ke `APPROVED`.

### Pemeriksaan DB
```sql
SELECT workflow_status FROM mst.instrumen WHERE kode_instrumen = 'UAT-DEP-003';
-- Harus tetap PENDING_APPROVAL, bukan APPROVED.
```

---

## Skenario S-009: Conditional Field Display per Tipe Instrumen

### Aktor: U-MAKER (ROLE-MAKER-TR)

### Tujuan
Memverifikasi bahwa UI menampilkan/menyembunyikan field yang relevan berdasarkan
tipe instrumen yang dipilih.

### Tabel Ekspektasi Field per Tipe

| Field | DEPOSITO | OBLIGASI | SAHAM | REKSADANA |
|---|---|---|---|---|
| Tanggal Jatuh Tempo | Wajib | Wajib | Tidak tampil | Tidak tampil |
| Kupon | Wajib | Wajib | Tidak tampil | Tidak tampil |
| Frekuensi Bunga | Optional | Wajib | Tidak tampil | Tidak tampil |
| ISIN | Optional | Wajib | Optional | Tidak tampil |
| Jumlah Lot | Tidak tampil | Tidak tampil | Wajib | Wajib |
| FVOCI Election | Tidak tampil | Optional | Wajib | Tidak tampil |
| Bank Kustodian | Tidak tampil | Tidak tampil | Wajib | Wajib |
| Manajer Investasi | Tidak tampil | Tidak tampil | Tidak tampil | Wajib |
| EIR Awal | Optional | Optional | Tidak tampil | Tidak tampil |
| Premium/Diskonto | Optional | Optional | Tidak tampil | Tidak tampil |

### Langkah-langkah

1. Login sebagai **U-MAKER**.
2. Buka form **Buat Instrumen**.
3. Pilih tipe `DEPOSITO` → verifikasi field sesuai kolom DEPOSITO di tabel.
4. Ubah tipe ke `SAHAM` → verifikasi field berubah (field Kupon hilang, Bank Kustodian muncul).
5. Ubah tipe ke `REKSADANA` → verifikasi Manajer Investasi muncul.

### Hasil yang Diharapkan
- Setiap perubahan tipe langsung mengupdate field yang terlihat/tersembunyi tanpa reload halaman.
- Validasi wajib per tipe ditampilkan via inline error saat submit tanpa field tersebut.

---

## Ringkasan Pemeriksaan Audit Wajib

Setelah semua skenario selesai, jalankan query audit check berikut:

```sql
-- Hitung event per action
SELECT action, COUNT(*) as jumlah
FROM aud.audit_log
WHERE entity_type = 'mst.instrumen'
  AND entity_id IN (SELECT id FROM mst.instrumen WHERE kode_instrumen LIKE 'UAT-%')
GROUP BY action
ORDER BY action;
```

**Nilai minimum yang diharapkan:**

| Action | Min Count |
|---|---|
| `INSTRUMEN.CREATE` | 4 (satu per instrumen dibuat) |
| `INSTRUMEN.SUBMIT` | 1 (minimal satu full cycle) |
| `INSTRUMEN.REVIEW` | 1 |
| `INSTRUMEN.APPROVE` | 1 |

```sql
-- Verifikasi hash chain tidak rusak (tidak ada row tanpa previous_hash yang valid)
-- Hash chain verifier dijalankan via: cmd/audit-verify --range "2026-06-05:2026-06-05"
```

---

## Kriteria Lulus / Gagal

| # | Kriteria | Pass | Fail |
|---|---|---|---|
| 1 | S-001: DEPOSITO dibuat, audit CREATE ada | Semua langkah sukses | Salah satu langkah gagal |
| 2 | S-002: OBLIGASI dibuat dengan EIR | Nominal tersimpan 4 desimal | Nilai float atau hilang presisi |
| 3 | S-003: SAHAM tanpa kustodian ditolak 422 | Error code VALIDATION_FAILED | Error lain atau 200 |
| 4 | S-004: REKSADANA tanpa manajer ditolak 422 | Error code VALIDATION_FAILED | Error lain atau 200 |
| 5 | S-005: FK counterparty DRAFT ditolak 422 | Error INSTRUMEN_COUNTERPARTY_NOT_APPROVED | Error lain atau 200 |
| 6 | S-006: Klasifikasi locked, edit rejected 423 | Error INSTRUMEN_KLASIFIKASI_LOCKED | Error lain atau 200 |
| 7 | S-007: 4-eyes full cycle selesai, status APPROVED | 3 audit events + 3 signatures | State tidak advance |
| 8 | S-008: API bypass SoD blocked 403 | HTTP 403 SOD_VIOLATION | HTTP 200 atau 201 (CRITICAL FAIL) |
| 9 | S-009: Conditional fields per tipe tampil benar | Visual sesuai tabel | Field salah tampil |

**BLOCKER**: Skenario S-008 gagal = CRITICAL SECURITY FAILURE. Modul tidak boleh di-release.
