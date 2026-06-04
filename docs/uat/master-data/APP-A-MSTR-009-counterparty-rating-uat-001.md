# UAT Script: Counterparty Master Data + Rating History
**Module**: APP-A Master Data  
**Proses**: MSTR-003 Counterparty + MSTR-003b Rating History Counterparty  
**UAT ID**: APP-A-MSTR-009-counterparty-rating-uat-001  
**Versi dokumen**: 1.0  
**Tanggal**: 2026-06-04  
**Referensi**: FSD-APP-A-MASTER-DATA-v1.0.docx §3.3; SoW_v1.4.docx §2.1 MSTR-003; BRD_BLIPS_IFRS9_v1.1.docx §4.2; DEC-011, DEC-017, DEC-028  
**Tester**: QA Engineer  
**Environment**: UAT (Docker Compose dev stack, PostgreSQL 18, migasi s/d 0015)

---

## Pre-kondisi

1. **Data seed awal** — jalankan migration s/d 0015: `go run ./cmd/migrator up`.
2. **Pengguna UAT** — buat 4 akun berbeda di Keycloak UAT:

   | Username | Role | MFA |
   |---|---|---|
   | `uat-maker-01` | ROLE-MAKER-TR | Tidak wajib |
   | `uat-reviewer-01` | ROLE-APPR-TR | Tidak wajib |
   | `uat-approver-01` | ROLE-APPR-TR | Tidak wajib |
   | `uat-auditor-01` | ROLE-AUDIT | Tidak wajib |

3. **Tidak ada data counterparty** dengan kode `CP-UAT-001` atau `CP-UAT-002` di database UAT.
4. **Browser**: Chrome/Firefox terbaru, DevTools Network tab aktif.
5. **Base URL**: `https://blips-uat.tugu-re.com` (atau `http://localhost:3000` untuk dev local).

---

## TC-001: Buat Counterparty Baru dengan PII — Verifikasi List Response Masked

**Aktor**: uat-maker-01 (ROLE-MAKER-TR)  
**Tujuan**: Memastikan bahwa data PII (NPWP, No. Rekening, KTP) yang diinput pada saat create counterparty TIDAK tampil dalam bentuk plaintext di list response (hanya masked/sentinel).  
**Referensi**: DEC-028, security-baseline.md §Encryption.

### Langkah

1. Login sebagai `uat-maker-01`.
2. Navigasi ke menu **Master Data > Counterparty**.
3. Klik tombol **+ Tambah Counterparty**.
4. Isi form dengan data berikut:

   | Field | Nilai |
   |---|---|
   | Kode Counterparty | `CP-UAT-001` |
   | Nama | `PT Bank Uji Coba` |
   | Tipe | `BANK` |
   | Tipe Eksposur Basel | `BANK` |
   | Eligible LPS | Ya |
   | Status | `ACTIVE` |
   | NPWP | `123456789012345` |
   | No. Rekening | `1234567890` |
   | KTP | `3201010101900001` |

5. Klik **Simpan**. Tunggu toast notifikasi sukses.
6. Kembali ke halaman **List Counterparty**.
7. Cari `CP-UAT-001` menggunakan search box.
8. Perhatikan kolom NPWP, No. Rekening, KTP di baris hasil pencarian.

### Hasil yang Diharapkan

- Toast hijau muncul: `"Counterparty CP-UAT-001 berhasil dibuat. Menunggu review."` dengan link **Lihat detail**.
- Di halaman List, baris `CP-UAT-001`:
  - Kolom **NPWP** menampilkan `null` atau `***` (bukan `123456789012345`).
  - Kolom **No. Rekening** menampilkan `null` atau `***` (bukan `1234567890`).
  - Kolom **KTP** menampilkan `null` atau `***` (bukan `3201010101900001`).
- Status counterparty: `DRAFT`.

### Audit Check

- `aud.audit_log` memiliki 1 row dengan `action = 'COUNTERPARTY.CREATE'` dan `entity_id` = UUID counterparty baru.
- Kolom `after_value` TIDAK mengandung plaintext NPWP/No. Rek/KTP — hanya nilai `REDACTED`.

### Rollback

```sql
DELETE FROM aud.audit_log WHERE entity_id = (SELECT id FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001');
DELETE FROM sys.workflow_instance WHERE entity_id = (SELECT id FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001');
DELETE FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001';
```

---

## TC-002: Alur 4-Eyes Lengkap (Maker → Reviewer → Approver)

**Aktor**: uat-maker-01, uat-reviewer-01, uat-approver-01  
**Tujuan**: Memastikan alur workflow 4-eyes berjalan dari DRAFT hingga APPROVED dengan signature tercatat.  
**Referensi**: DEC-017 (4-eyes workflow), FSD-APP-A §3.3.4.

### Pre-kondisi

- TC-001 selesai: `CP-UAT-001` ada dalam status `DRAFT`.

### Langkah

**Tahap Maker (uat-maker-01)**

1. Login sebagai `uat-maker-01`.
2. Navigasi ke **Master Data > Counterparty**, buka detail `CP-UAT-001`.
3. Klik tombol **Submit untuk Review**.
4. Isi komentar: `"Data counterparty sudah dilengkapi, mohon direview."`.
5. Klik **Konfirmasi Submit**. Tunggu toast sukses.

**Tahap Reviewer (uat-reviewer-01)**

6. Login sebagai `uat-reviewer-01`.
7. Navigasi ke **Antrian Review** atau **Master Data > Counterparty**, temukan `CP-UAT-001`.
8. Buka detail, klik **Review & Sign**.
9. Isi komentar: `"Data sudah sesuai dokumen."`.
10. Klik **Tandatangani**. Tunggu toast sukses.

**Tahap Approver (uat-approver-01)**

11. Login sebagai `uat-approver-01`.
12. Navigasi ke **Antrian Approval**, temukan `CP-UAT-001`.
13. Buka detail, klik **Approve & Sign**.
14. Isi komentar: `"Disetujui."`.
15. Klik **Tandatangani**. Tunggu toast sukses.

### Hasil yang Diharapkan

- Setelah step 5: status `CP-UAT-001` berubah menjadi `PENDING_REVIEW`. Toast: `"CP-UAT-001 berhasil diajukan ke reviewer."`.
- Setelah step 10: status `PENDING_APPROVAL`. Toast: `"Review berhasil. Menunggu approver."`.
- Setelah step 15: status `APPROVED`. Toast: `"CP-UAT-001 berhasil disetujui."`.

### Audit Check

```sql
SELECT action, actor_user_id FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001')
ORDER BY event_time;
-- Harus ada: COUNTERPARTY.CREATE, COUNTERPARTY.SUBMIT, COUNTERPARTY.REVIEW, COUNTERPARTY.APPROVE

SELECT COUNT(*) FROM sys.workflow_signature wfs
JOIN sys.workflow_instance wfi ON wfs.workflow_instance_id = wfi.id
JOIN mst.counterparty cp ON cp.workflow_instance_id = wfi.id
WHERE cp.kode_counterparty = 'CP-UAT-001';
-- Harus >= 3 (submit, review, approve)
```

---

## TC-003: Lihat PII — Tombol, Konfirmasi, Plaintext, Auto-Mask, Audit

**Aktor**: uat-auditor-01 (ROLE-AUDIT, memiliki `counterparty.view_pii`)  
**Tujuan**: Memastikan alur view PII berfungsi: (a) tombol terlihat hanya untuk yang berhak, (b) konfirmasi dialog muncul, (c) plaintext muncul di UI, (d) re-mask otomatis setelah 30 detik, (e) audit `COUNTERPARTY.VIEW_PII` tertulis.  
**Referensi**: DEC-028, security-baseline.md §Audit trail.

### Pre-kondisi

- TC-002 selesai: `CP-UAT-001` status `APPROVED`.

### Langkah

1. Login sebagai `uat-auditor-01`.
2. Navigasi ke **Master Data > Counterparty**, buka detail `CP-UAT-001`.
3. Perhatikan panel PII — NPWP, No. Rekening, KTP ditampilkan dengan nilai `***...` (masked).
4. Klik tombol **Lihat Data PII** (atau ikon kunci).
5. Dialog konfirmasi muncul: _"Tindakan ini akan diaudit. Lanjutkan untuk melihat data PII?"_. Klik **Konfirmasi**.
6. Amati nilai NPWP, No. Rekening, KTP — harus tampil plaintext.
7. Tunggu 30 detik. Amati nilai PII.

### Hasil yang Diharapkan

- Step 4: tombol **Lihat Data PII** terlihat dan aktif (karena `uat-auditor-01` punya `counterparty.view_pii`).
- Step 5: dialog konfirmasi muncul sebelum aksi dieksekusi.
- Step 6: NPWP = `123456789012345`, No. Rekening = `1234567890`, KTP = `3201010101900001`.
- Step 7: setelah 30 detik, nilai kembali masked `***...` secara otomatis (frontend timer).

### Audit Check

```sql
SELECT actor_user_id, after_value FROM aud.audit_log
WHERE action = 'COUNTERPARTY.VIEW_PII'
  AND entity_id = (SELECT id FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001')
ORDER BY event_time DESC LIMIT 1;
-- after_value HARUS berisi "pii_accessed": true, tapi NPWP/No.Rek/KTP = "REDACTED"
```

---

## TC-004: Lihat PII Tanpa Permission — Tombol Disabled

**Aktor**: uat-maker-01 (ROLE-MAKER-TR, TIDAK memiliki `counterparty.view_pii`)  
**Tujuan**: Memastikan bahwa user tanpa `counterparty.view_pii` tidak bisa mengakses data PII.  
**Referensi**: DEC-028, security-baseline.md §Permission model.

### Pre-kondisi

- TC-002 selesai: `CP-UAT-001` status `APPROVED`.

### Langkah

1. Login sebagai `uat-maker-01`.
2. Navigasi ke **Master Data > Counterparty**, buka detail `CP-UAT-001`.
3. Perhatikan area tampilan PII.

### Hasil yang Diharapkan

- Tombol **Lihat Data PII** tidak terlihat ATAU berwarna abu-abu (disabled) dengan tooltip `"Tidak punya permission counterparty.view_pii"`.
- Jika user mencoba akses langsung via URL `GET /api/v1/master/counterparty/{id}/pii` tanpa token yang memiliki permission → HTTP 403 FORBIDDEN dengan error code `FORBIDDEN`.

### Verifikasi API

```bash
curl -X GET https://blips-uat.tugu-re.com/api/v1/master/counterparty/{CP-UAT-001-UUID}/pii \
  -H "Authorization: Bearer {uat-maker-01-token}"
# Expected: 403 {"error": {"code": "FORBIDDEN", ...}}
```

---

## TC-005: Rating UPGRADE notch_change=+2 — Tidak Trigger SICR

**Aktor**: uat-maker-01, uat-reviewer-01, uat-approver-01  
**Tujuan**: Memastikan bahwa rating upgrade (notch_change positif) TIDAK men-trigger SICR.  
**Referensi**: DEC-011 (SICR triggers: only downgrade ≥2 notch, IG→non-IG, DPD≥30).

### Pre-kondisi

- `CP-UAT-001` status `APPROVED`.
- Buat counterparty kedua `CP-UAT-002` dalam status `APPROVED` (atau gunakan `CP-UAT-001`).

### Langkah

1. Login sebagai `uat-maker-01`.
2. Navigasi ke **Master Data > Rating History**, klik **+ Tambah Rating History**.
3. Isi form:

   | Field | Nilai |
   |---|---|
   | Kode | `RH-UAT-UPGRADE-001` |
   | Counterparty | `CP-UAT-001` |
   | Tanggal Berlaku | `2026-06-01` |
   | Rating Pefindo | `idAA` |
   | Sumber Rating | `PEFINDO` |
   | Tanggal Publikasi | `2026-06-01` |
   | Action Type | `UPGRADE` |
   | Notch Change | `+2` |

4. Klik **Simpan**. Lanjutkan workflow 4-eyes (submit → review → approve) dengan user berbeda.
5. Setelah APPROVED, buka detail rating history `RH-UAT-UPGRADE-001`.

### Hasil yang Diharapkan

- Field **SICR Triggered** = `false`.
- Field **Default Triggered** = `false`.
- `mst.counterparty.rating_pefindo_current` untuk `CP-UAT-001` = `idAA`.
- Banner SICR TIDAK muncul di halaman detail counterparty `CP-UAT-001`.

### Audit Check

```sql
SELECT sicr_triggered, default_triggered, rating_pefindo
FROM mst.rating_history_counterparty
WHERE rating_history_id_kode = 'RH-UAT-UPGRADE-001';
-- sicr_triggered = false, default_triggered = false

SELECT rating_pefindo_current FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-001';
-- 'idAA'
```

---

## TC-006: Rating DOWNGRADE notch_change=-2 — SICR Triggered

**Aktor**: uat-maker-01, uat-reviewer-01, uat-approver-01  
**Tujuan**: Memastikan rating downgrade 2 notch men-trigger SICR dan banner peringatan muncul di detail counterparty.  
**Referensi**: DEC-011, PSAK 71 §5.5.7, SoW_v1.4.docx §4.

### Pre-kondisi

- TC-005 selesai: `CP-UAT-001` memiliki rating aktif `idAA`.

### Langkah

1. Login sebagai `uat-maker-01`.
2. Navigasi ke **Master Data > Rating History**, klik **+ Tambah Rating History**.
3. Isi form:

   | Field | Nilai |
   |---|---|
   | Kode | `RH-UAT-DOWN-001` |
   | Counterparty | `CP-UAT-001` |
   | Tanggal Berlaku | `2026-07-01` |
   | Rating Pefindo | `idBBB` |
   | Sumber Rating | `PEFINDO` |
   | Tanggal Publikasi | `2026-07-01` |
   | Action Type | `DOWNGRADE` |
   | Notch Change | `-2` |

4. Klik **Simpan**.
5. Jalankan workflow 4-eyes: submit (maker) → review (reviewer) → approve (approver).
6. Setelah APPROVED, buka detail counterparty `CP-UAT-001`.

### Hasil yang Diharapkan

- Field **SICR Triggered** pada `RH-UAT-DOWN-001` = `true`.
- Field **Default Triggered** = `false`.
- `mst.counterparty.rating_pefindo_current` = `idBBB`.
- Banner SICR **MUNCUL** di halaman detail counterparty: `"PERHATIAN: SICR terpicu pada 2026-07-01. Rating turun dari idAA ke idBBB (-2 notch). Counterparty ini mungkin perlu di-staging ke Stage 2."`.
- Rating sebelumnya (`RH-UAT-UPGRADE-001`, rating `idAA`) memiliki `tanggal_berakhir` = `2026-06-30` (= tanggal_berlaku baru - 1 hari).

### Audit Check

```sql
SELECT sicr_triggered, default_triggered, tanggal_berakhir
FROM mst.rating_history_counterparty
WHERE rating_history_id_kode = 'RH-UAT-DOWN-001';
-- sicr_triggered = true, default_triggered = false, tanggal_berakhir = null

SELECT tanggal_berakhir
FROM mst.rating_history_counterparty
WHERE rating_history_id_kode = 'RH-UAT-UPGRADE-001';
-- tanggal_berakhir = '2026-06-30'
```

---

## TC-007: Rating idAA → idBB+ (IG Transition) — SICR Triggered

**Aktor**: uat-maker-01, uat-reviewer-01, uat-approver-01  
**Tujuan**: Memastikan transisi dari Investment Grade (idAA) ke Non-IG (idBB+) men-trigger SICR, meskipun notch_change tidak mencapai -2.  
**Referensi**: DEC-011 (IG→non-IG SICR rule), PSAK 71 §5.5.7.

### Pre-kondisi

- `CP-UAT-001` memiliki rating aktif `idBBB` (hasil TC-006). Untuk skenario ini, kita gunakan `CP-UAT-002` baru agar kondisi bersih.

### Setup CP-UAT-002

1. Buat counterparty `CP-UAT-002` dengan rating awal `idAA` (IG):
   - Buat CP-UAT-002 (TC-001 pattern), approval 4-eyes.
   - Buat RatingHistory kode `RH-UAT-CP2-INIT`: rating `idAA`, action `INITIAL`, notchChange `0`. Approval 4-eyes.
2. Verifikasi `mst.counterparty.rating_pefindo_current` untuk `CP-UAT-002` = `idAA`.

### Langkah

3. Sebagai `uat-maker-01`, buat Rating History baru:

   | Field | Nilai |
   |---|---|
   | Kode | `RH-UAT-IGNONIG-001` |
   | Counterparty | `CP-UAT-002` |
   | Tanggal Berlaku | `2026-08-01` |
   | Rating Pefindo | `idBB+` |
   | Sumber Rating | `PEFINDO` |
   | Tanggal Publikasi | `2026-08-01` |
   | Action Type | `DOWNGRADE` |
   | Notch Change | `-1` |

4. Jalankan workflow 4-eyes hingga APPROVED.
5. Buka detail counterparty `CP-UAT-002`.

### Hasil yang Diharapkan

- Field **SICR Triggered** pada `RH-UAT-IGNONIG-001` = `true` (IG→non-IG rule terpicu meskipun notch=-1).
- Field **isInvestmentGrade** pada rating lama `idAA` = `true`; pada rating baru `idBB+` = `false`.
- Banner SICR muncul di detail counterparty `CP-UAT-002` dengan informasi transisi IG→non-IG.

### Audit Check

```sql
SELECT sicr_triggered, default_triggered
FROM mst.rating_history_counterparty
WHERE rating_history_id_kode = 'RH-UAT-IGNONIG-001';
-- sicr_triggered = true
```

---

## TC-008: Rating idD — Default Triggered

**Aktor**: uat-maker-01, uat-reviewer-01, uat-approver-01  
**Tujuan**: Memastikan rating `idD` (default/gagal bayar) men-trigger `default_triggered=true` DAN `sicr_triggered=true`.  
**Referensi**: DEC-011, PSAK 71 §5.5.3 (definition of default), SoW_v1.4.docx §4.

### Pre-kondisi

- Gunakan `CP-UAT-002` dengan rating aktif `idBB+` (hasil TC-007).

### Langkah

1. Sebagai `uat-maker-01`, buat Rating History baru:

   | Field | Nilai |
   |---|---|
   | Kode | `RH-UAT-DEFAULT-001` |
   | Counterparty | `CP-UAT-002` |
   | Tanggal Berlaku | `2026-09-01` |
   | Rating Pefindo | `idD` |
   | Sumber Rating | `PEFINDO` |
   | Tanggal Publikasi | `2026-09-01` |
   | Action Type | `DOWNGRADE` |
   | Notch Change | `-5` |

2. Jalankan workflow 4-eyes hingga APPROVED.
3. Buka detail counterparty `CP-UAT-002`.

### Hasil yang Diharapkan

- Field **Default Triggered** pada `RH-UAT-DEFAULT-001` = `true`.
- Field **SICR Triggered** = `true` (karena notch_change=-5 ≤ -2, dan non-IG).
- `mst.counterparty.rating_pefindo_current` = `idD`.
- Banner **MERAH** muncul di detail counterparty `CP-UAT-002`: `"PERINGATAN: Counterparty ini berada dalam status DEFAULT (idD). Semua instrumen terkait harus dipindahkan ke Stage 3."`.

### Audit Check

```sql
SELECT sicr_triggered, default_triggered, rating_pefindo
FROM mst.rating_history_counterparty
WHERE rating_history_id_kode = 'RH-UAT-DEFAULT-001';
-- sicr_triggered = true, default_triggered = true, rating_pefindo = 'idD'

SELECT rating_pefindo_current FROM mst.counterparty WHERE kode_counterparty = 'CP-UAT-002';
-- 'idD'
```

---

## Cleanup Akhir UAT

Jalankan setelah seluruh skenario selesai dan hasilnya dicatat:

```sql
-- Hapus rating history UAT
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
  SELECT id FROM mst.rating_history_counterparty
  WHERE rating_history_id_kode LIKE 'RH-UAT-%'
);
DELETE FROM aud.audit_log
WHERE entity_id IN (
  SELECT id FROM mst.rating_history_counterparty
  WHERE rating_history_id_kode LIKE 'RH-UAT-%'
);
DELETE FROM mst.rating_history_counterparty WHERE rating_history_id_kode LIKE 'RH-UAT-%';

-- Hapus counterparty UAT
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
  SELECT id FROM mst.counterparty WHERE kode_counterparty IN ('CP-UAT-001', 'CP-UAT-002')
);
DELETE FROM aud.audit_log
WHERE entity_id IN (
  SELECT id FROM mst.counterparty WHERE kode_counterparty IN ('CP-UAT-001', 'CP-UAT-002')
);
DELETE FROM mst.counterparty WHERE kode_counterparty IN ('CP-UAT-001', 'CP-UAT-002');
```

---

## Ringkasan Cakupan

| TC | Business Rule | Regression Priority |
|---|---|---|
| TC-001 | PII hanya plaintext di sisi server, list = null/masked | Security §9 |
| TC-002 | 4-eyes workflow + audit trail + signatures | §3 Staging transitions |
| TC-003 | PII view = permission + konfirmasi + audit + auto-mask | DEC-028 + §7 Audit |
| TC-004 | SoD enforcement di API (tidak hanya UI) | §6 SoD |
| TC-005 | SICR TIDAK terpicu untuk upgrade | DEC-011 |
| TC-006 | SICR terpicu untuk downgrade ≥2 notch | DEC-011 |
| TC-007 | SICR terpicu untuk IG→non-IG | DEC-011 |
| TC-008 | Default triggered untuk rating idD | DEC-011, PSAK 71 §5.5.3 |
