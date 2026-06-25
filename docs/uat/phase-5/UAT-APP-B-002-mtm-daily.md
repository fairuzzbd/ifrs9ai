# UAT APP-B-002 — MTM Harian (Mark-to-Market Daily Job)

**Modul**: APP-B Transaction Lifecycle  
**Fase**: P5-M6  
**Versi dokumen**: 1.0  
**Tanggal**: 2026-06-18  
**Penulis**: QA Engineer  
**Referensi story**: `docs/stories/phase-5/P5-M6-mtm-daily.md` — S1..S5 × AC1..AC4  

---

## Scope

Proses bisnis yang diuji:

1. Cron job harian MTM (auto-run & manual trigger via ROLE-IT-ADMIN)
2. Upload batch manual XLSX/CSV harga pasar
3. Monitoring stale price & eskalasi ROLE-RISK
4. Override approve / reject dengan SoD enforcement
5. Routing jurnal sesuai klasifikasi PSAK 71 (matrix compliance)

---

## Pre-conditions (wajib dipenuhi sebelum menjalankan TC)

### Data seed

```sql
-- 1. Buat periode buku Juni 2026 dengan status OPEN
INSERT INTO mst.periode_buku (
  id, periode_id_kode, tahun_buku, bulan_buku, status_periode,
  created_by, updated_by, tenant_id
) VALUES (
  'eeeeeeee-0001-0000-0000-000000000001', 'PB-2026-06', 2026, 6, 'OPEN',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- 2. Buat instrumen test (satu per klasifikasi)
-- AC: Deposito (tidak boleh masuk MTM)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama_instrumen, klasifikasi_psak71, mata_uang,
  is_poci, harga_buku_idr, periode_buku_id, created_by, updated_by, tenant_id
) VALUES (
  'bbbbbbbb-0001-0000-0000-000000000001', 'DEP-UAT-001', 'Deposito BCA 1 tahun',
  'AC', 'IDR', false, 1000000000.0000,
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- FVOCI_DEBT IDR: Obligasi Negara FR0094
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama_instrumen, klasifikasi_psak71, mata_uang,
  is_poci, harga_buku_idr, periode_buku_id, created_by, updated_by, tenant_id
) VALUES (
  'bbbbbbbb-0002-0000-0000-000000000001', 'FR0094', 'Obligasi Negara FR0094',
  'FVOCI_DEBT', 'IDR', false, 1000000.0000,
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- FVOCI_DEBT FCY: Obligasi USD
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama_instrumen, klasifikasi_psak71, mata_uang,
  is_poci, harga_buku_idr, periode_buku_id, created_by, updated_by, tenant_id
) VALUES (
  'bbbbbbbb-0003-0000-0000-000000000001', 'FR0100-USD', 'Obligasi USD Pemerintah',
  'FVOCI_DEBT', 'USD', false, 15000000.0000,
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- FVOCI_ELECTION: Saham ASII (irrevocable election)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama_instrumen, klasifikasi_psak71, mata_uang,
  is_poci, harga_buku_idr, periode_buku_id, created_by, updated_by, tenant_id
) VALUES (
  'bbbbbbbb-0004-0000-0000-000000000001', 'ASII', 'Astra International Tbk',
  'FVOCI_ELECTION', 'IDR', false, 5200.0000,
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- FVTPL: Saham BBRI
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama_instrumen, klasifikasi_psak71, mata_uang,
  is_poci, harga_buku_idr, periode_buku_id, created_by, updated_by, tenant_id
) VALUES (
  'bbbbbbbb-0005-0000-0000-000000000001', 'BBRI', 'Bank Rakyat Indonesia Tbk',
  'FVTPL', 'IDR', false, 4100.0000,
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;

-- 3. Buat user test (5 persona berbeda untuk SoD)
INSERT INTO sec.user_shadow (id, username, role, tenant_id) VALUES
  ('11111111-0000-0000-0000-000000000001', 'akun.maker.1',    'ROLE-AKUN',     'TUGURE'),
  ('22222222-0000-0000-0000-000000000001', 'akun.ctl.1',      'ROLE-AKUN-CTL', 'TUGURE'),
  ('33333333-0000-0000-0000-000000000001', 'akun.ctl.2',      'ROLE-AKUN-CTL', 'TUGURE'),
  ('44444444-0000-0000-0000-000000000001', 'it.admin.1',      'ROLE-IT-ADMIN', 'TUGURE'),
  ('55555555-0000-0000-0000-000000000001', 'risk.officer.1',  'ROLE-RISK',     'TUGURE')
ON CONFLICT DO NOTHING;

-- 4. Pastikan sys.config mempunyai parameter MTM
INSERT INTO sys.config (key, value, tenant_id) VALUES
  ('MTM_PRICE_DEVIATION_THRESHOLD_PCT', '5.0', 'TUGURE'),
  ('MTM_PRICE_STALE_DAYS',              '5',   'TUGURE'),
  ('MTM_STALE_ESCALATION_DAYS',         '7',   'TUGURE')
ON CONFLICT (key, tenant_id) DO UPDATE SET value = EXCLUDED.value;
```

### Role assignments

| Username | Role | Digunakan untuk TC |
|---|---|---|
| `akun.maker.1` | ROLE-AKUN | Uploader MTM manual |
| `akun.ctl.1` | ROLE-AKUN-CTL | Override approver (SoD pair dengan maker.1) |
| `akun.ctl.2` | ROLE-AKUN-CTL | Override approver alternatif |
| `it.admin.1` | ROLE-IT-ADMIN | Trigger MTM cron manual |
| `risk.officer.1` | ROLE-RISK | Viewer stale price alerts |

### State aplikasi

- Periode buku `PB-2026-06` harus berstatus `OPEN`
- Feed harga IBPA tersedia untuk `FR0094` dengan harga IDR 1.050.000 per 2026-06-18
- Feed harga BEI tersedia untuk `ASII` dengan harga IDR 5.750 per 2026-06-18
- Tidak ada data `trx.mtm` untuk tanggal 2026-06-18 (bersih)

---

## TC-01 — Cron MTM Harian: AC Dilewati, FVOCI_DEBT Diposting Otomatis

**Story**: S1-AC1  
**Aktor**: ROLE-IT-ADMIN (trigger), SYSTEM (cron)  
**Pre-condition**: Feed harga IBPA tersedia untuk FR0094 = IDR 1.050.000  

### Langkah-langkah

**Given**: User login sebagai `it.admin.1` (ROLE-IT-ADMIN)

**When**:

1. Buka halaman `/mtm`
2. Klik tombol **"Jalankan MTM Cron"** (hanya muncul untuk ROLE-IT-ADMIN)
3. Isi dialog konfirmasi:
   - Tanggal MTM: `2026-06-18` (hari kerja)
   - Idempotency-Key: generate otomatis (UUID v4)
4. Klik **"Konfirmasi & Jalankan"**

**Then**:

- Response `202 Accepted` dengan `{ jobId, statusUrl }` tampil dalam 2 detik
- `<JobProgressPanel>` muncul, progress bar bergerak dari 0% ke 100%
- Toast sukses: `"MTM Cron 2026-06-18 selesai. 4 instrumen diproses, 1 AC dilewati."`

### Verifikasi database

```sql
-- 1. Instrumen AC (DEP-UAT-001) TIDAK ada di trx.mtm
SELECT COUNT(*) FROM trx.mtm
WHERE instrumen_id = 'bbbbbbbb-0001-0000-0000-000000000001'
  AND tanggal_mtm = '2026-06-18';
-- Expected: 0

-- 2. FR0094 ada dengan status AUTO_POSTED
SELECT status, jurnal_event_code, jurnal_entry_id, delta_idr, delta_pct
FROM trx.mtm
WHERE instrumen_id = 'bbbbbbbb-0002-0000-0000-000000000001'
  AND tanggal_mtm = '2026-06-18';
-- Expected: status=AUTO_POSTED, jurnal_event_code=MTM_FVOCI,
--           jurnal_entry_id NOT NULL, delta_idr=50000.0000, delta_pct=5.0000

-- 3. Audit log ada
SELECT action, entity_type FROM aud.audit_log
WHERE action IN ('MTM.AUTO_POSTED', 'MTM.CRON_TRIGGERED')
  AND event_time::date = '2026-06-18'
ORDER BY event_time DESC LIMIT 10;
-- Expected: setidaknya 1 baris MTM.AUTO_POSTED untuk FR0094

-- 4. Jurnal di-post ke jrnl.jurnal_entry
SELECT event_code, amount_idr FROM jrnl.jurnal_entry
WHERE id = (
  SELECT jurnal_entry_id FROM trx.mtm
  WHERE instrumen_id = 'bbbbbbbb-0002-0000-0000-000000000001'
    AND tanggal_mtm = '2026-06-18'
);
-- Expected: event_code=MTM_FVOCI, amount_idr=50000.0000
```

### Hasil yang diharapkan

| Cek | Nilai |
|---|---|
| Instrumen AC dilewati | Ya, tidak ada baris di trx.mtm |
| FR0094 status | AUTO_POSTED |
| Kode jurnal FR0094 | MTM_FVOCI (single entry, IDR) |
| delta_idr | IDR 50.000 |
| delta_pct | 5,0000% |
| Audit baris | MTM.AUTO_POSTED ada |

---

## TC-02 — Cron Pada Hari Libur: HOLIDAY_SKIP Direkam

**Story**: S1-AC2  
**Aktor**: ROLE-IT-ADMIN (trigger)  
**Pre-condition**: Seed `sys.holiday_calendar` dengan tanggal 2026-06-19 (Jumat, hari libur rekaan)

```sql
INSERT INTO sys.holiday_calendar (tanggal, keterangan, tenant_id)
VALUES ('2026-06-19', 'Hari Libur UAT Test', 'TUGURE')
ON CONFLICT DO NOTHING;
```

### Langkah-langkah

**Given**: User login sebagai `it.admin.1`

**When**:

1. Buka `/mtm`
2. Klik **"Jalankan MTM Cron"**
3. Tanggal MTM: `2026-06-19`
4. Klik **"Konfirmasi & Jalankan"**

**Then**:

- Toast info: `"MTM Cron 2026-06-19 dilewati — tanggal libur/akhir pekan."`
- Tidak ada baris baru di `trx.mtm` untuk tanggal 2026-06-19

### Verifikasi database

```sql
-- Audit HOLIDAY_SKIP ada
SELECT action, after_jsonb->>'tanggal' AS tanggal
FROM aud.audit_log
WHERE action = 'MTM.HOLIDAY_SKIP'
  AND event_time::date >= '2026-06-19';
-- Expected: 1 baris dengan tanggal=2026-06-19

-- Tidak ada baris MTM
SELECT COUNT(*) FROM trx.mtm WHERE tanggal_mtm = '2026-06-19';
-- Expected: 0
```

---

## TC-03 — Upload Batch Manual: 3 Baris Berhasil

**Story**: S2-AC1  
**Aktor**: ROLE-AKUN (`akun.maker.1`)  
**Pre-condition**: File `mtm-upload-uat.xlsx` disiapkan dengan 3 baris:

| kode_instrumen | tanggal_mtm | harga_pasar | mata_uang | harga_sumber |
|---|---|---|---|---|
| ASII | 2026-06-18 | 5750 | IDR | BEI_MANUAL |
| BBRI | 2026-06-18 | 4700 | IDR | BEI_MANUAL |
| FR0094 | 2026-06-18 | 1060000 | IDR | MANUAL |

> Catatan: ASII delta = (5750-5200)/5200 × 100 = 10,58% > 5% → PENDING_REVIEW
> BBRI delta = (4700-4100)/4100 × 100 = 14,63% > 5% → PENDING_REVIEW  
> FR0094 delta = (1060000-1000000)/1000000 × 100 = 6,00% > 5% → PENDING_REVIEW

### Langkah-langkah

**Given**: User login sebagai `akun.maker.1` (ROLE-AKUN)

**When**:

1. Buka `/mtm/upload`
2. Klik **"Pilih File"** → upload `mtm-upload-uat.xlsx`
3. Preview tabel tampil dengan 3 baris
4. Klik **"Upload & Proses"**
5. Masukkan Idempotency-Key (atau biarkan sistem generate)

**Then**:

- Progress bar muncul selama parsing
- Toast sukses: `"Upload batch berhasil. 3 baris diproses: 0 gagal, 3 menunggu review."`
- Link **"Lihat Status Batch"** tersedia

### Verifikasi database

```sql
-- 3 baris PENDING_REVIEW
SELECT kode_instrumen_snapshot, status, deviation_flag, delta_pct
FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE m.tanggal_mtm = '2026-06-18'
  AND m.harga_sumber IN ('BEI_MANUAL', 'MANUAL')
  AND m.status = 'PENDING_REVIEW'
ORDER BY i.kode_instrumen;
-- Expected: 3 baris (ASII, BBRI, FR0094) semua deviation_flag=TRUE

-- delta_pct ASII: HALF_EVEN 4dp
SELECT delta_pct FROM trx.mtm
JOIN mst.instrumen i ON trx.mtm.instrumen_id = i.id
WHERE i.kode_instrumen = 'ASII' AND tanggal_mtm = '2026-06-18';
-- Expected: 10.5769 (550/5200×100, HALF_EVEN 4dp)

-- Audit MTM.UPLOAD_BATCH ada
SELECT action FROM aud.audit_log
WHERE action = 'MTM.UPLOAD_BATCH' AND event_time::date = '2026-06-18';
-- Expected: ≥ 1 baris
```

---

## TC-04 — Upload dengan Instrumen AC: Per-Row Skip, Lainnya Lanjut

**Story**: S2-AC2  
**Aktor**: ROLE-AKUN (`akun.maker.1`)  
**Pre-condition**: File `mtm-upload-ac.xlsx`:

| kode_instrumen | tanggal_mtm | harga_pasar | mata_uang |
|---|---|---|---|
| DEP-UAT-001 | 2026-06-18 | 1000000001 | IDR |
| BBRI | 2026-06-18 | 4700 | IDR |

### Langkah-langkah

**Given**: User login sebagai `akun.maker.1`

**When**:

1. Buka `/mtm/upload`
2. Upload `mtm-upload-ac.xlsx`
3. Klik **"Upload & Proses"**

**Then**:

- Toast warning: `"Upload batch selesai: 1 berhasil, 1 dilewati (DEP-UAT-001: instrumen AC tidak masuk MTM)."`
- Tabel preview menampilkan baris DEP-UAT-001 dengan badge merah `MTM_INSTRUMEN_AC_SKIP`

### Verifikasi database

```sql
-- DEP-UAT-001 TIDAK ada di trx.mtm
SELECT COUNT(*) FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'DEP-UAT-001';
-- Expected: 0

-- BBRI ada
SELECT status FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'BBRI' AND m.tanggal_mtm = '2026-06-18';
-- Expected: PENDING_REVIEW (karena deviasi)
```

---

## TC-05 — Stale Price: harga_age_days > 5

**Story**: S3-AC1  
**Aktor**: ROLE-AKUN (`akun.maker.1`)  
**Pre-condition**: File `mtm-upload-stale.xlsx`:

| kode_instrumen | tanggal_mtm | harga_pasar | harga_tanggal | mata_uang |
|---|---|---|---|---|
| BBRI | 2026-06-18 | 4200 | 2026-06-10 | IDR |

> harga_age_days = 18 − 10 = 8 hari > 5 → STALE_PRICE  
> harga_age_days = 8 > 7 → STALE_ESCALATION juga

### Langkah-langkah

**Given**: User login sebagai `akun.maker.1`

**When**:

1. Upload `mtm-upload-stale.xlsx`
2. Klik **"Upload & Proses"**

**Then**:

- Toast warning: `"1 baris stale price (BBRI: harga berumur 8 hari). Menunggu override approver."`
- Baris BBRI tampil dengan badge oranye `STALE PRICE` dan `ESKALASI RISK` di tabel MTM

### Verifikasi database

```sql
-- BBRI stale_price_flag=TRUE, status=STALE_PRICE
SELECT status, stale_price_flag, harga_age_days
FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'BBRI' AND m.tanggal_mtm = '2026-06-18';
-- Expected: status=STALE_PRICE, stale_price_flag=TRUE, harga_age_days=8

-- Audit eskalasi ada (harga_age_days > 7)
SELECT action, after_jsonb->>'harga_age_days' AS age
FROM aud.audit_log
WHERE action = 'MTM.STALE_ESCALATION'
  AND event_time::date = '2026-06-18';
-- Expected: 1 baris dengan age=8
```

---

## TC-06 — Override Approve: PENDING_REVIEW → APPROVED (SoD Benar)

**Story**: S4-AC1, S4-AC2  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`) — **berbeda** dari uploader `akun.maker.1`  
**Pre-condition**: Baris ASII dari TC-03 sudah ada dengan status PENDING_REVIEW

### Langkah-langkah

**Given**: User login sebagai `akun.ctl.1` (ROLE-AKUN-CTL)

**When**:

1. Buka `/mtm`
2. Filter: `Status = Menunggu Review`
3. Klik baris **ASII**
4. Klik **"Override: Setuju"**
5. Dialog terbuka → verifikasi:
   - Banner oranye: `"Deviasi harga 10,58% melebihi threshold 5,0%"`
   - Tombol Submit masih **disabled** (belum ada komentar + belum centang atestasi)
6. Isi komentar: `"Harga ASII 5750 terverifikasi via Bloomberg terminal. Delta karena FOMC malam ini."`
   - Hitungan karakter muncul: `"62 karakter (min 30 karakter)"`
7. Centang checkbox: `"Saya menyatakan harga telah diverifikasi dari sumber independen"`
8. Tombol Submit sekarang **enabled**
9. Klik **"Setuju & Posting Jurnal"**

**Then**:

- Toast sukses: `"MTM ASII 2026-06-18 disetujui. Jurnal MTM_FVTPL_ELECTION berhasil diposting."`
- Link **"Lihat Jurnal"** → navigasi ke halaman jurnal entry

### Verifikasi database

```sql
-- Status APPROVED, override_approver_id terisi
SELECT status, override_approver_id, override_comment, override_at, jurnal_entry_id
FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'ASII' AND m.tanggal_mtm = '2026-06-18';
-- Expected: status=APPROVED, override_approver_id=22222222-..., jurnal_entry_id NOT NULL

-- Jurnal MTM_FVOCI_ELECTION di-post (ASII = FVOCI_ELECTION)
SELECT event_code FROM jrnl.jurnal_entry
WHERE id = (
  SELECT jurnal_entry_id FROM trx.mtm m
  JOIN mst.instrumen i ON m.instrumen_id = i.id
  WHERE i.kode_instrumen = 'ASII' AND m.tanggal_mtm = '2026-06-18'
);
-- Expected: MTM_FVOCI_ELECTION

-- Audit MTM.OVERRIDE_APPROVED ada
SELECT action, actor_user_id FROM aud.audit_log
WHERE action = 'MTM.OVERRIDE_APPROVED'
  AND entity_id = (
    SELECT m.id::text FROM trx.mtm m
    JOIN mst.instrumen i ON m.instrumen_id = i.id
    WHERE i.kode_instrumen = 'ASII' AND m.tanggal_mtm = '2026-06-18'
  );
-- Expected: 1 baris, actor_user_id=22222222-...
```

---

## TC-07 — Override Approve: SoD Violation (Uploader = Approver)

**Story**: S4-AC3  
**Aktor**: ROLE-AKUN (`akun.maker.1`) mencoba approve upload-nya sendiri  
**Pre-condition**: Baris BBRI dari TC-03 ada dengan status PENDING_REVIEW, uploaded oleh `akun.maker.1`

### Langkah-langkah

**Given**: User login sebagai `akun.maker.1` (ROLE-AKUN)

**When**:

1. Buka `/mtm`
2. Klik baris BBRI (status PENDING_REVIEW)
3. Tombol **"Override: Setuju"** tidak tampil (permission `mtm.override.approve` hanya untuk ROLE-AKUN-CTL)

> Alternatif test langsung via API (penting untuk coverage):

```bash
curl -X POST "https://[host]/api/v1/trx/mtm/{mtm_id}/override-approve" \
  -H "Authorization: Bearer {token_akun_maker_1}" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"comment":"Harga sudah saya upload sendiri, saya setuju.", "signature_method":"JWT_STEP_UP"}'
```

**Then** (via API):

- HTTP `403 Forbidden`
- Body: `{"error":{"code":"MTM_OVERRIDE_SOD_VIOLATION","message":"Uploader dan override-approver tidak boleh orang yang sama (SoD). Minta rekan ROLE-AKUN-CTL yang lain."}}`

### Verifikasi database

```sql
-- Status BBRI tetap PENDING_REVIEW
SELECT status FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'BBRI' AND m.tanggal_mtm = '2026-06-18';
-- Expected: PENDING_REVIEW (tidak berubah)

-- Audit SoD violation ada
SELECT action FROM aud.audit_log
WHERE action = 'MTM.SOD_VIOLATION' AND event_time::date = '2026-06-18';
-- Expected: 1 baris
```

---

## TC-08 — Override Reject: Komentar ≥ 30 Karakter, Status REJECTED

**Story**: S4-AC4  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.2`) — berbeda dari uploader  
**Pre-condition**: Baris BBRI ada dengan status PENDING_REVIEW

### Langkah-langkah

**Given**: User login sebagai `akun.ctl.2` (ROLE-AKUN-CTL)

**When**:

1. Buka `/mtm` → klik baris BBRI
2. Klik **"Override: Tolak"**
3. Dialog terbuka
4. Isi komentar pendek: `"Salah"` (5 karakter)
   - Tombol **"Tolak MTM"** tetap **disabled**, pesan: `"Komentar minimal 30 karakter"`
5. Isi komentar lengkap: `"Harga 4700 tidak sesuai data BEI hari ini. Re-upload dengan harga 4100."`
   - Tombol sekarang **enabled**
6. Klik **"Tolak MTM"**
7. Dialog konfirmasi destruktif: `"Yakin menolak MTM BBRI 2026-06-18? Uploader akan dinotifikasi."`
8. Klik **"Ya, Tolak"**

**Then**:

- Toast merah (persistent): `"MTM BBRI 2026-06-18 ditolak. Uploader telah dinotifikasi untuk re-upload."`

### Verifikasi database

```sql
-- Status REJECTED, comment terisi
SELECT status, override_comment FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'BBRI' AND m.tanggal_mtm = '2026-06-18';
-- Expected: status=REJECTED, override_comment='Harga 4700 tidak sesuai...'

-- Baris REJECTED boleh diupload ulang (unique constraint excludes REJECTED)
-- TC verifikasi: insert ulang harus berhasil
```

---

## TC-09 — Routing Jurnal: FVOCI_DEBT FCY → 2 Entri Jurnal (§B5.7.2A)

**Story**: S5-AC3  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`)  
**Pre-condition**: FR0100-USD ada dengan status PENDING_REVIEW (deviation > 5%), uploaded oleh `akun.maker.1`

```sql
-- Seed baris PENDING_REVIEW untuk FR0100-USD (simulasi hasil cron / upload)
INSERT INTO trx.mtm (
  id, instrumen_id, tanggal_mtm, harga_sumber,
  harga_pasar_idr, harga_pasar_fcy, harga_buku_idr, kurs_tengah,
  delta_idr, delta_pct, harga_age_days,
  stale_price_flag, deviation_flag, status,
  klasifikasi_snapshot, mata_uang, is_poci,
  jurnal_event_code, uploader_id, periode_bulanan_id,
  created_by, updated_by, tenant_id
) VALUES (
  'cccccccc-0001-0000-0000-000000000001',
  'bbbbbbbb-0003-0000-0000-000000000001',  -- FR0100-USD
  '2026-06-18', 'MANUAL',
  16800000.0000, 1080.0, 15000000.0000, 15555.5556,
  1800000.0000, 12.0000, 0,
  false, true, 'PENDING_REVIEW',
  'FVOCI_DEBT', 'USD', false,
  '', 'ffffffff-0000-0000-0000-000000000001',
  'eeeeeeee-0001-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'ffffffff-0000-0000-0000-000000000001',
  'TUGURE'
) ON CONFLICT DO NOTHING;
```

### Langkah-langkah

**Given**: User login sebagai `akun.ctl.1`

**When**:

1. Buka detail MTM FR0100-USD
2. Klik **"Override: Setuju"**
3. Isi komentar: `"Kurs USD/IDR bergerak signifikan. Bloomberg konfirmasi harga 1080 USD valid untuk 2026-06-18."`
4. Centang atestasi → klik **"Setuju & Posting Jurnal"**

**Then**:

- Toast sukses: `"MTM FR0100-USD disetujui. 2 jurnal diposting: MTM_FVOCI + MTM_FX_OCI_RESERVE."`

### Verifikasi database

```sql
-- Dua jurnal entry ID terisi
SELECT jurnal_event_code, jurnal_entry_id, jurnal_event_code_2, jurnal_entry_id_2
FROM trx.mtm WHERE id = 'cccccccc-0001-0000-0000-000000000001';
-- Expected: jurnal_event_code=MTM_FVOCI, jurnal_entry_id NOT NULL
--           jurnal_event_code_2=MTM_FX_OCI_RESERVE, jurnal_entry_id_2 NOT NULL

-- Dua baris di jrnl.jurnal_entry
SELECT event_code, amount_idr FROM jrnl.jurnal_entry
WHERE id IN (
  SELECT jurnal_entry_id FROM trx.mtm WHERE id = 'cccccccc-0001-0000-0000-000000000001'
  UNION ALL
  SELECT jurnal_entry_id_2 FROM trx.mtm WHERE id = 'cccccccc-0001-0000-0000-000000000001'
);
-- Expected: 2 baris: MTM_FVOCI dan MTM_FX_OCI_RESERVE
```

---

## TC-10 — Routing Jurnal: AC → Error MTM_INSTRUMEN_AC_SKIP

**Story**: S5-AC1  
**Aktor**: API langsung (verifikasi service-layer enforcement)

```bash
curl -X POST "https://[host]/api/v1/trx/mtm/upload/batch" \
  -H "Authorization: Bearer {token_akun_maker_1}" \
  -H "Idempotency-Key: $(uuidgen)" \
  -F "file=@mtm-upload-ac-only.xlsx"
```

**Then**:

- HTTP 422
- Per-row error: `{"code":"MTM_INSTRUMEN_AC_SKIP","instrumen_kode":"DEP-UAT-001","message":"Instrumen AC tidak dihitung MTM (PSAK 71 §4.1.2)."}`

### Verifikasi database

```sql
SELECT COUNT(*) FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'DEP-UAT-001';
-- Expected: 0 (tidak pernah diinsert)
```

---

## TC-11 — Periode Hard-Close Memblokir Semua Mutasi MTM

**Story**: Cross-cutting S1-S5  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`)  
**Pre-condition**: Lakukan hard-close periode PB-2026-06 (lihat UAT-APP-D-003 TC-09 untuk flow CFO hard-close)

```sql
-- Simulasi hard-close: set locked_flag = TRUE
UPDATE trx.mtm SET locked_flag = TRUE
WHERE periode_bulanan_id = 'eeeeeeee-0001-0000-0000-000000000001';
```

### Langkah-langkah

**When**: Coba override-approve baris PENDING_REVIEW yang ada setelah lock

```bash
curl -X POST "https://[host]/api/v1/trx/mtm/{mtm_id}/override-approve" \
  -H "Authorization: Bearer {token_akun_ctl_1}" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"comment":"Coba approve setelah periode closed.", "signature_method":"JWT_STEP_UP"}'
```

**Then**:

- HTTP `423 Locked`
- Body: `{"error":{"code":"MTM_PERIODE_LOCKED","message":"Periode buku sudah hard-closed. Mutasi trx.mtm tidak diizinkan."}}`
- UI: button **"Override: Setuju"** dan **"Override: Tolak"** di-disable dengan tooltip `"Periode sudah closed."`

### Verifikasi database

```sql
-- Status tidak berubah
SELECT status FROM trx.mtm WHERE locked_flag = TRUE LIMIT 5;
-- Expected: semua tetap pada status sebelum lock (PENDING_REVIEW / STALE_PRICE)
```

---

## TC-12 — Idempotency: Replay Request Tidak Membuat Baris Duplikat

**Story**: Cross-cutting (DEC-021)  
**Aktor**: ROLE-AKUN (`akun.maker.1`)

### Langkah-langkah

**Given**: Siapkan request upload batch dengan Idempotency-Key tetap

```bash
IDEM_KEY="test-idem-$(uuidgen)"

# Request pertama
curl -X POST "https://[host]/api/v1/trx/mtm/upload/batch" \
  -H "Authorization: Bearer {token}" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -F "file=@mtm-upload-uat.xlsx"
# Expected: 201 Created, batchId=...

# Request kedua dengan key yang sama
curl -X POST "https://[host]/api/v1/trx/mtm/upload/batch" \
  -H "Authorization: Bearer {token}" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -F "file=@mtm-upload-uat.xlsx"
# Expected: 200 OK, code=IDEMPOTENCY_REPLAY, data=(original response)
```

**Then**:

- Request kedua mengembalikan response identik tanpa side-effect baru
- Tidak ada baris duplikat di `trx.mtm`

### Verifikasi database

```sql
-- Hanya 3 baris (bukan 6) setelah 2 request
SELECT COUNT(*) FROM trx.mtm
WHERE upload_batch_id IN (
  SELECT id FROM trx.mtm_upload_batch
  WHERE created_by = '11111111-0000-0000-0000-000000000001'
  ORDER BY created_at DESC LIMIT 2
);
-- Expected: 3 (bukan 6)

-- Hanya 1 audit baris MTM.UPLOAD_BATCH
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'MTM.UPLOAD_BATCH'
  AND idempotency_key = '{IDEM_KEY}';
-- Expected: 1
```

---

## TC-13 — Hash Chain Audit Tidak Bisa Dimanipulasi

**Story**: Cross-cutting (DEC-018)  
**Aktor**: ROLE-AUDIT (`audit.user`) — baca-saja  

### Langkah-langkah

**When**: Jalankan verifikasi hash-chain via CLI:

```bash
go run ./cmd/audit-verify \
  --range "2026-06-18:2026-06-18" \
  --entity-type "trx.mtm"
```

**Then**:

- Output: `"Hash chain verification PASSED. 42 rows verified. No tampering detected."`

Simulasi tamper (untuk verifikasi deteksi):

```sql
-- Tamper: ubah after_jsonb di salah satu baris (jangan di prod)
UPDATE aud.audit_log SET after_jsonb = '{"status":"APPROVED","tampered":true}'
WHERE action = 'MTM.AUTO_POSTED'
  AND event_time::date = '2026-06-18'
LIMIT 1;
```

Jalankan ulang verifikasi:

```bash
go run ./cmd/audit-verify --range "2026-06-18:2026-06-18" --entity-type "trx.mtm"
```

**Expected**: `"Hash chain BROKEN at row {event_id}. Tampering detected at 2026-06-18T..."`

---

## TC-14 — List MTM: Sort, Filter, Paging, Export (DEC-022)

**Story**: S1-AC2 (UI feature)  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`)

### Langkah-langkah

1. Buka `/mtm`
2. Verifikasi tabel mempunyai kolom: Kode, Nama, Tanggal, Harga Pasar, Delta%, Status, Jurnal
3. Klik header **"Delta%"** → sort descending (panah ↓ tampil)
4. Filter status: **"Menunggu Review"** → hanya baris PENDING_REVIEW tampil
5. Filter chip `status:Menunggu Review` muncul dengan tombol ×
6. URL berubah menjadi `/mtm?filter[status]=PENDING_REVIEW&sort=delta_pct:desc`
7. Klik **"Export ▾ → CSV"**
8. File CSV terunduh dengan header Bahasa Indonesia, filter+sort aktif

### Verifikasi

```sql
-- Export audit log
SELECT action, after_jsonb FROM aud.audit_log
WHERE action = 'MTM.EXPORT'
  AND event_time::date = '2026-06-18';
-- Expected: 1 baris dengan format=csv, filter aktif
```

---

## TC-15 — Stale Price Alert: ROLE-RISK Melihat Dashboard

**Story**: S3-AC4  
**Aktor**: ROLE-RISK (`risk.officer.1`)

### Langkah-langkah

1. Login sebagai `risk.officer.1`
2. Buka `/mtm/alerts/stale-price`
3. Daftar instrumen dengan `stale_price_flag=TRUE` tampil
4. Kolom: Kode Instrumen, Tanggal MTM, Harga Sumber, harga_age_days, Status Eskalasi
5. Baris dengan `harga_age_days > 7` memiliki badge merah **"ESKALASI"**

### Verifikasi database

```sql
-- API endpoint GET /api/v1/trx/mtm/alerts/stale-price
SELECT instrumen_id, harga_age_days, status
FROM trx.mtm
WHERE stale_price_flag = TRUE
  AND tanggal_mtm = '2026-06-18'
ORDER BY harga_age_days DESC;
-- Expected: baris BBRI dengan harga_age_days=8, status=STALE_PRICE
```

---

## TC-16 — FVOCI_ELECTION: Routing MTM_FVOCI_ELECTION (Irrevocable, No P&L Recycling)

**Story**: S5-AC4  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`)  
**Pre-condition**: Baris ASII dari TC-03 sudah PENDING_REVIEW

### Langkah-langkah

1. Override-approve baris ASII (instrumen `FVOCI_ELECTION`)
2. Isi komentar valid (≥ 30 karakter)

**Then**:

- Jurnal event code: `MTM_FVOCI_ELECTION` (bukan MTM_FVTPL — tidak ada P&L recycling §5.7.5)
- Toast: `"Jurnal MTM_FVOCI_ELECTION berhasil diposting."`

### Verifikasi database

```sql
SELECT jurnal_event_code FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'ASII' AND m.tanggal_mtm = '2026-06-18'
  AND m.status = 'APPROVED';
-- Expected: MTM_FVOCI_ELECTION
-- NOT: MTM_FVTPL (FVOCI_ELECTION ≠ FVTPL — no P&L recycling on disposal §5.7.5)
```

---

## TC-17 — Upload dengan Harga Nol/Negatif: VALIDATION_FAILED Per Baris

**Story**: S2-AC3  
**Aktor**: ROLE-AKUN (`akun.maker.1`)  
**File**: `mtm-upload-invalid.xlsx`

| kode_instrumen | harga_pasar |
|---|---|
| BBRI | 0 |
| FR0094 | -1000 |

### Langkah-langkah

1. Upload `mtm-upload-invalid.xlsx`
2. Preview tabel tampil dengan highlight merah per baris

**Then**:

- Toast error: `"Upload gagal: 2 baris tidak valid. Lihat detail error di bawah."`
- Baris 1: `"BBRI: harga_pasar harus > 0 (diterima: 0)"`
- Baris 2: `"FR0094: harga_pasar harus > 0 (diterima: -1000)"`

---

## TC-18 — Upload Duplikat: 409 CONFLICT (Bukan REJECTED)

**Story**: S2-AC4  
**Aktor**: ROLE-AKUN (`akun.maker.1`)  
**Pre-condition**: BBRI sudah ada dengan status PENDING_REVIEW untuk tanggal 2026-06-18

### Langkah-langkah

1. Upload ulang baris BBRI `(kode=BBRI, tanggal=2026-06-18, sumber=BEI_MANUAL)` tanpa Idempotency-Key berbeda

**Then**:

- HTTP 409 per baris: `"Conflict: MTM untuk BBRI 2026-06-18 BEI_MANUAL sudah ada dengan status PENDING_REVIEW."`
- Saran: `"Tolak baris yang ada terlebih dahulu jika ingin upload ulang."`

---

## TC-19 — Override Approve Komentar < 30 Karakter: Ditolak

**Story**: S4-AC2 (negatif)  
**Aktor**: ROLE-AKUN-CTL (`akun.ctl.1`)

### Langkah-langkah

1. Buka dialog Override Approve untuk baris PENDING_REVIEW
2. Ketik komentar: `"Setuju."` (7 karakter)
3. Centang atestasi

**Then**:

- Tombol **"Setuju & Posting Jurnal"** tetap **disabled**
- Pesan inline: `"Komentar minimal 30 karakter (saat ini 7 karakter)"`

---

## TC-20 — FVTPL → MTM_FVTPL; POCI Flag → MTM_FVTPL_POCI

**Story**: S5-AC1 (routing completeness)

### Verifikasi routing via API

```bash
# Seed instrumen POCI (is_poci=true, klasifikasi=FVTPL)
# Jalankan cron dan verifikasi jurnal event code

SELECT jurnal_event_code FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.is_poci = true AND m.tanggal_mtm = '2026-06-18';
-- Expected: MTM_FVTPL_POCI

SELECT jurnal_event_code FROM trx.mtm m
JOIN mst.instrumen i ON m.instrumen_id = i.id
WHERE i.kode_instrumen = 'BBRI' AND m.tanggal_mtm = '2026-06-18'
  AND m.status = 'APPROVED';
-- Expected: MTM_FVTPL (BBRI bukan POCI)
```

---

## Rollback / Cleanup

```sql
-- Hapus baris MTM hasil UAT
DELETE FROM trx.mtm WHERE tanggal_mtm = '2026-06-18' AND tenant_id = 'TUGURE';

-- Hapus instrumen test
DELETE FROM mst.instrumen
WHERE kode_instrumen IN ('DEP-UAT-001','FR0094','FR0100-USD','ASII','BBRI')
  AND tenant_id = 'TUGURE';

-- Hapus user test
DELETE FROM sec.user_shadow
WHERE username IN ('akun.maker.1','akun.ctl.1','akun.ctl.2','it.admin.1','risk.officer.1')
  AND tenant_id = 'TUGURE';

-- Hapus holiday test
DELETE FROM sys.holiday_calendar WHERE keterangan = 'Hari Libur UAT Test';

-- Reset periode
UPDATE mst.periode_buku
SET status_periode = 'OPEN', locked_flag = FALSE
WHERE periode_id_kode = 'PB-2026-06' AND tenant_id = 'TUGURE';

-- Hapus idempotency keys test
DELETE FROM sys.idempotency_key WHERE expires_at < now() AND tenant_id = 'TUGURE';

-- CATATAN: jangan DELETE dari aud.audit_log (append-only per DEC-018)
-- Baris audit untuk tenant test dapat diarsipkan ke MinIO cold storage setelah 5 tahun
```

---

## Ringkasan Test Case

| TC | Story | AC | Aktor | Hasil Harapan |
|---|---|---|---|---|
| TC-01 | S1 | AC1 | IT-ADMIN/SYSTEM | AC dilewati, FVOCI_DEBT AUTO_POSTED + jurnal MTM_FVOCI |
| TC-02 | S1 | AC2 | IT-ADMIN/SYSTEM | Holiday skip, MTM.HOLIDAY_SKIP audit, 0 baris inserted |
| TC-03 | S2 | AC1 | AKUN | 3 baris PENDING_REVIEW, deviation_flag=TRUE |
| TC-04 | S2 | AC2 | AKUN | AC skip per-baris, instrumen lain lanjut |
| TC-05 | S3 | AC1 | AKUN | stale_price_flag=TRUE, status STALE_PRICE, eskalasi audit |
| TC-06 | S4 | AC1,AC2 | AKUN-CTL | PENDING_REVIEW → APPROVED, jurnal posted, SoD valid |
| TC-07 | S4 | AC3 | AKUN (SoD fail) | 403 MTM_OVERRIDE_SOD_VIOLATION |
| TC-08 | S4 | AC4 | AKUN-CTL | PENDING_REVIEW → REJECTED, komentar ≥30 char |
| TC-09 | S5 | AC3 | AKUN-CTL | FCY FVOCI_DEBT → 2 jurnal (MTM_FVOCI + MTM_FX_OCI_RESERVE) |
| TC-10 | S5 | AC1 | API | AC → 422 MTM_INSTRUMEN_AC_SKIP |
| TC-11 | X-cut | lock | AKUN-CTL | 423 MTM_PERIODE_LOCKED setelah hard-close |
| TC-12 | X-cut | idem | AKUN | Replay Idempotency-Key → 200 IDEMPOTENCY_REPLAY, no duplicate |
| TC-13 | X-cut | audit | AUDIT | Hash-chain PASSED; tamper terdeteksi |
| TC-14 | S1 | AC2 | AKUN-CTL | Sort+filter+paging+export berfungsi, URL deep-link |
| TC-15 | S3 | AC4 | RISK | Dashboard stale alerts, badge ESKALASI untuk age>7 |
| TC-16 | S5 | AC4 | AKUN-CTL | FVOCI_ELECTION → MTM_FVOCI_ELECTION (no P&L recycling) |
| TC-17 | S2 | AC3 | AKUN | harga ≤ 0 → VALIDATION_FAILED per baris |
| TC-18 | S2 | AC4 | AKUN | Duplikat non-REJECTED → 409 CONFLICT |
| TC-19 | S4 | AC2 | AKUN-CTL | Comment <30 char → tombol disabled |
| TC-20 | S5 | AC1 | API | FVTPL → MTM_FVTPL; POCI → MTM_FVTPL_POCI |

---

## Catatan Kepatuhan

| Keputusan | Implementasi yang Diverifikasi |
|---|---|
| DEC-016 shopspring/decimal | delta_pct dihitung dengan HALF_EVEN 4dp, bukan float64 |
| DEC-017 SoD 4-eyes | override_approver_id ≠ uploader_id diperiksa di service layer |
| DEC-018 audit append-only | DELETE pada aud.audit_log menghasilkan error; hash-chain tidak patah |
| DEC-021 Idempotency-Key | Semua endpoint mutating wajib header Idempotency-Key |
| DEC-022 cursor pagination | List MTM pakai cursor-based, tidak ada offset |
| PSAK 71 §B5.7.2A | FVOCI_DEBT FCY menghasilkan 2 jurnal terpisah (FX reserve) |
| PSAK 71 §5.7.5 | FVOCI_ELECTION → MTM_FVOCI_ELECTION, tidak ada daur ulang ke P&L |
