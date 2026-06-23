# UAT-APP-B-003: Renewal Deposito (P5-M7)

**Modul**: APP-B Transaction Lifecycle
**Fase**: P5-M7
**Versi Dokumen**: 1.0
**Tanggal**: 2026-06-19
**Penulis**: qa-engineer

---

## Scope

Proses renewal deposito PSAK 71: amandemen kontrak (§5.4.3), kalkulasi PPh 20% (PP No. 131/2000), EIR re-estimasi Newton-Raphson (DEC-013), instrumen baru otomatis, jurnal RENEWAL_DEPOSITO (P5-M2), dan workflow Maker → Approver dengan SoD 4-eyes.

---

## Pre-Conditions

### Roles yang diperlukan

| Actor | Role | Username (UAT) |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | `treasury.maker` |
| Treasury Approver | ROLE-APPR-TR | `treasury.approver` |
| Finance Controller | ROLE-AKUN-CTL | `finance.controller` |
| Internal Auditor | ROLE-AUDIT | `internal.auditor` |
| Admin IT | ROLE-IT-ADMIN | `it.admin` |

### SQL Seed (jalankan sebelum UAT, dalam urutan ini)

```sql
-- 1. Pastikan periode buku bulan aktif ada dan OPEN
INSERT INTO sys.periode_buku (id, bulan, status, tenant_id, created_by, updated_by)
VALUES (
  'eee00001-0007-0000-0000-000000000001',
  '2026-07-01',
  'OPEN',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001', -- system user
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (bulan) DO UPDATE SET status = 'OPEN';

-- 2. Counterparty: Bank BCA
INSERT INTO mst.counterparty (id, kode_counterparty, nama, jenis, tenant_id, created_by, updated_by)
VALUES (
  'cntry001-0000-0000-0000-000000000001',
  'BCA',
  'PT Bank Central Asia Tbk',
  'BANK',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (kode_counterparty) DO NOTHING;

-- 3. Portofolio AC
INSERT INTO mst.portofolio (id, kode, nama, business_model, tenant_id, created_by, updated_by)
VALUES (
  'portf001-0000-0000-0000-000000000001',
  'HTC-DEPOSITO',
  'Hold-to-Collect Deposito',
  'HTC',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (kode) DO NOTHING;

-- 4. Instrumen deposito lama yang akan di-renew
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama, jenis_instrumen, status,
  klasifikasi_psak71, klasifikasi_locked,
  pokok, mata_uang, rate_persen, tenor_bulan,
  tanggal_penempatan, tanggal_jatuh_tempo,
  counterparty_id, portofolio_id,
  sppi_test_locked, bm_assessment_locked,
  tenant_id, created_by, updated_by
) VALUES (
  'dep00042-0000-0000-0000-000000000001',
  'DEP-0042',
  'Deposito BCA 6 Bulan – UAT Renewal',
  'DEPOSITO',
  'ACTIVE',
  'AC',
  TRUE,
  1000000000.0000, -- IDR 1 Miliar
  'IDR',
  5.50,
  6,
  '2026-01-01',
  '2026-07-01',
  'cntry001-0000-0000-0000-000000000001',
  'portf001-0000-0000-0000-000000000001',
  TRUE,
  TRUE,
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (id) DO NOTHING;

-- 5. EIR schedule awal (sebelum renewal)
INSERT INTO ecl.amortisasi_schedule (
  id, instrumen_id, schedule_version, eir_persen,
  effective_from, effective_to,
  tenant_id, created_by, updated_by
) VALUES (
  gen_random_uuid(),
  'dep00042-0000-0000-0000-000000000001',
  1,
  0.04400000, -- EIR awal 4.40% pa (after-PPh)
  '2026-01-01',
  NULL, -- infinity
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT DO NOTHING;
```

### State awal yang harus diverifikasi sebelum TC-01

```sql
SELECT id, kode_instrumen, status, klasifikasi_locked
FROM mst.instrumen
WHERE id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: 1 row, status='ACTIVE', klasifikasi_locked=TRUE

SELECT COUNT(*) FROM trx.renewal
WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  AND status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED');
-- Expected: 0 (tidak ada renewal aktif)
```

---

## TC-01: Create Renewal POKOK_PLUS_BUNGA Happy Path

**Prioritas**: P0 (Critical)
**AC**: S1-AC1
**Persona**: Treasury Maker

### Pre-condition

Instrumen DEP-0042 aktif, tidak ada renewal aktif, periode OPEN.

### Langkah-Langkah

1. Login sebagai `treasury.maker`.
2. Navigasi ke **Transaksi → Renewal Deposito → Buat Renewal Baru**.
3. Isi form:
   - **Instrumen Lama**: cari dan pilih `DEP-0042`
   - **Skema**: `POKOK_PLUS_BUNGA`
   - **Tenor Baru**: `12` bulan
   - **Rate Baru**: `5.75` %
   - **Tanggal Efektif Baru**: `2026-07-01`
4. Klik **Preview Kalkulasi**. Verifikasi nilai ditampilkan (lihat Expected Result).
5. Klik **Simpan & Ajukan**.
6. Catat nomor renewal yang terbuat (misal `RNW-000001`).

### Expected Result

Halaman preview menampilkan:

| Field | Nilai |
|---|---|
| Hari Berjalan | 181 hari (2026-01-01 → 2026-07-01) |
| Bunga Kotor | IDR 27.260.273,9726 (tampil 4dp) |
| PPh 20% | IDR 5.452.054,7945 |
| Bunga Bersih | IDR 21.808.219,1781 |
| Pokok Lama | IDR 1.000.000.000,0000 |
| Pokok Baru | IDR 1.021.808.219,1781 |
| EIR Baru | ≈ 4,60% pa (8 desimal) |
| Tanggal JT Baru | 2026-07-01 |

Toast sukses: `"Renewal DEP-0042 berhasil diajukan. Menunggu approval."` + link ke detail renewal.

Status di list: `PENDING_APPROVAL`.

### SQL Verify

```sql
SELECT r.id, r.status, r.skema, r.tenor_baru_bulan,
       r.bunga_kotor, r.pph_amount, r.bunga_bersih, r.pokok_baru, r.eir_baru,
       r.maker_id
FROM trx.renewal r
WHERE r.instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  AND r.status = 'PENDING_APPROVAL';
-- Expected: 1 row, bunga_kotor > 0, pph_amount > 0, eir_baru berisi 8 dp

SELECT COUNT(*) FROM aud.audit_log
WHERE entity_id = (SELECT id FROM trx.renewal WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001')
  AND action = 'RENEWAL.CREATED';
-- Expected: 1 audit row
```

### Rollback

```sql
DELETE FROM trx.renewal
WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  AND status = 'PENDING_APPROVAL';
```

---

## TC-02: Create Renewal — Tenor Di Luar Range

**Prioritas**: P1
**AC**: S1-AC2
**Persona**: Treasury Maker

### Pre-condition

Instrumen DEP-0042 aktif.

### Langkah-Langkah

1. Login sebagai `treasury.maker`.
2. Buka form **Buat Renewal Baru**, pilih `DEP-0042`.
3. Isi **Tenor Baru**: `72` bulan (di luar range 1–60).
4. Klik **Simpan & Ajukan**.

### Expected Result

Toast merah persistent: `"Tenor 72 bulan di luar range yang diizinkan (1–60 bulan). Kode: RENEWAL_TENOR_OUT_OF_RANGE"`.

Field tenor di-highlight merah dengan pesan `"Tenor harus antara 1 dan 60 bulan"`.

Tidak ada renewal ter-INSERT ke database.

### SQL Verify

```sql
SELECT COUNT(*) FROM trx.renewal
WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: 0 (atau tetap dari TC-01 jika tidak rollback)
```

---

## TC-03: Create Renewal — Rate Di Luar Range

**Prioritas**: P1
**AC**: S1-AC3
**Persona**: Treasury Maker

### Langkah-Langkah

1. Login sebagai `treasury.maker`.
2. Buka form **Buat Renewal Baru**, pilih `DEP-0042`.
3. Isi **Rate Baru**: `35` % (di luar range 0–30%).
4. Klik **Simpan & Ajukan**.

### Expected Result

Validasi inline muncul sebelum submit (front-end Zod validation). Toast error jika lolos front-end: kode `RENEWAL_RATE_OUT_OF_RANGE`.

Tidak ada INSERT ke database.

---

## TC-04: Create Renewal — Instrumen Bukan DEPOSITO

**Prioritas**: P1
**AC**: S1-AC4
**Persona**: Treasury Maker

### SQL Seed (TC-04)

```sql
-- Instrumen obligasi untuk TC-04
INSERT INTO mst.instrumen (id, kode_instrumen, jenis_instrumen, status, klasifikasi_locked,
  pokok, mata_uang, tanggal_penempatan, tanggal_jatuh_tempo,
  counterparty_id, portofolio_id, tenant_id, created_by, updated_by, klasifikasi_psak71)
VALUES (
  'obl00099-0000-0000-0000-000000000001', 'OBL-0099', 'OBLIGASI', 'ACTIVE', TRUE,
  500000000.0000, 'IDR', '2025-01-01', '2026-12-31',
  'cntry001-0000-0000-0000-000000000001', 'portf001-0000-0000-0000-000000000001',
  'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'FVOCI'
) ON CONFLICT DO NOTHING;
```

### Langkah-Langkah

1. Login sebagai `treasury.maker`.
2. Cari dan pilih `OBL-0099` di form Renewal.
3. Klik **Simpan & Ajukan**.

### Expected Result

Toast merah: `"OBL-0099 bukan instrumen deposito (jenis=OBLIGASI). Renewal hanya untuk DEPOSITO ACTIVE. Kode: RENEWAL_INSTRUMEN_NOT_ELIGIBLE"`.

Tidak ada renewal ter-INSERT.

---

## TC-05: Approve Renewal Happy Path — Full Flow

**Prioritas**: P0 (Critical)
**AC**: S2-AC1, S3-AC1, S3-AC2, S4-AC1, S5-AC1
**Persona**: Treasury Approver

### Pre-condition

TC-01 telah dijalankan berhasil. Renewal `RNW-000001` dalam status `PENDING_APPROVAL`. Login approver berbeda dari maker (SoD).

### Langkah-Langkah

1. Login sebagai `treasury.approver`.
2. Navigasi ke **Transaksi → Renewal Deposito → Queue Approval**.
3. Klik `RNW-000001`, baca detail dan nilai kalkulasi.
4. Klik **Approve**.
5. Isi **Komentar Approval**: `"Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Semua dokumen lengkap. Disetujui."` (≥ 30 karakter).
6. Klik **Konfirmasi** pada dialog konfirmasi.
7. Sistem meminta **JWT Step-Up** (MFA for sensitive action) — selesaikan step-up.
8. Amati progress bar (operasi 12-step dalam 1 transaksi).

### Expected Result

Toast hijau: `"Renewal DEP-0042 berhasil di-approve dan di-posting. Instrumen baru DEP-0042B dibuat."` + link ke instrumen baru.

Status renewal: `POSTED`.

Panel Detail menampilkan:
- `instrumen_baru_id`: UUID instrumen baru
- `jurnal_header_id`: UUID jurnal posting
- `eir_baru`: nilai 8 desimal

### SQL Verify (S3, S4, S5)

```sql
-- Status renewal = POSTED
SELECT status, instrumen_baru_id, jurnal_header_id, eir_baru
FROM trx.renewal
WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  AND status = 'POSTED';
-- Expected: 1 row

-- S3-AC2: instrumen lama = MATURED
SELECT status FROM mst.instrumen
WHERE id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: status = 'MATURED'

-- S3-AC1: instrumen baru ACTIVE, inherit klasifikasi
SELECT kode_instrumen, status, klasifikasi_psak71, klasifikasi_locked,
       counterparty_id, portofolio_id, renewal_dari_instrumen_id
FROM mst.instrumen
WHERE renewal_dari_instrumen_id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: 1 row, status='ACTIVE', klasifikasi_psak71='AC', klasifikasi_locked=TRUE

-- S4-AC1: EIR schedule baru (effective_to = NULL = infinity)
SELECT instrumen_id, schedule_version, eir_persen, effective_from, effective_to
FROM ecl.amortisasi_schedule
WHERE instrumen_id = (
  SELECT instrumen_baru_id FROM trx.renewal
  WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
);
-- Expected: 1 row, schedule_version=1, effective_to IS NULL

-- S4-AC1: EIR schedule lama effective_to ter-set (TIDAK NULL)
SELECT effective_to, eir_persen FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: effective_to = '2026-07-01' (NOT NULL), eir_persen TIDAK berubah (immutabilitas)

-- S5-AC1: jurnal RENEWAL_DEPOSITO posted (4 legs)
SELECT jh.event_code, COUNT(jl.id) AS leg_count
FROM jrnl.jurnal_header jh
JOIN jrnl.jurnal_leg jl ON jl.jurnal_header_id = jh.id
WHERE jh.id = (
  SELECT jurnal_header_id FROM trx.renewal
  WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
)
GROUP BY jh.event_code;
-- Expected: event_code='RENEWAL_DEPOSITO', leg_count=4

-- Audit chain: 6 events
SELECT action, entity_id FROM aud.audit_log
WHERE entity_id IN (
  SELECT id FROM trx.renewal WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  UNION
  SELECT instrumen_baru_id FROM trx.renewal WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001'
  UNION
  SELECT 'dep00042-0000-0000-0000-000000000001'::UUID
)
ORDER BY event_time;
-- Expected: RENEWAL.CREATED, RENEWAL.APPROVED, RENEWAL.POSTED, INSTRUMEN.CREATED, INSTRUMEN.MATURED, EIR.RECOMPUTED
```

### Rollback

```sql
-- Rollback TC-05 secara manual (hanya jika terjadi error — sistem harusnya rollback atomik)
UPDATE mst.instrumen SET status = 'ACTIVE', updated_at = now()
WHERE id = 'dep00042-0000-0000-0000-000000000001';

DELETE FROM mst.instrumen WHERE renewal_dari_instrumen_id = 'dep00042-0000-0000-0000-000000000001';

UPDATE trx.renewal SET status = 'PENDING_APPROVAL', instrumen_baru_id = NULL, jurnal_header_id = NULL
WHERE instrumen_lama_id = 'dep00042-0000-0000-0000-000000000001';

UPDATE ecl.amortisasi_schedule SET effective_to = NULL
WHERE instrumen_id = 'dep00042-0000-0000-0000-000000000001';
```

---

## TC-06: SoD — Maker Tidak Bisa Approve Sendiri

**Prioritas**: P0 (Security)
**AC**: S2-AC3
**Persona**: Treasury Maker (mencoba approve sendiri)

### Pre-condition

Buat renewal baru dengan `treasury.maker` (ulangi TC-01, catat renewal ID).

### Langkah-Langkah

1. Tetap login sebagai `treasury.maker` (jangan switch akun).
2. Navigasi ke Queue Approval, cari renewal yang baru dibuat.
3. Klik **Approve**.
4. Sistem harus menolak sebelum step-up MFA ditampilkan.

### Expected Result

Toast merah persistent: `"Anda tidak bisa menjadi approver untuk renewal yang Anda ajukan sendiri. Kode: SOD_VIOLATION"`.

Status renewal tetap `PENDING_APPROVAL`.

Audit log mencatat `RENEWAL.SOD_VIOLATION_ATTEMPT` (advisory — tidak blocking audit chain).

### SQL Verify

```sql
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'RENEWAL.SOD_VIOLATION_ATTEMPT'
  AND actor_user_id = (SELECT id FROM sec.user WHERE username = 'treasury.maker');
-- Expected: ≥ 1
```

---

## TC-07: Idempotency Replay

**Prioritas**: P1
**AC**: S2-AC4
**Persona**: Treasury Approver

### Langkah-Langkah

1. Siapkan renewal baru dalam status `PENDING_APPROVAL`.
2. Intercept permintaan approve (Postman/mitmproxy) — catat header `Idempotency-Key` (misal `IDEM-001`).
3. Approve dari UI — berhasil.
4. Re-submit permintaan approve yang sama lewat Postman dengan header `Idempotency-Key: IDEM-001`.

### Expected Result

HTTP 200 dengan body identik dengan response pertama. Error code `IDEMPOTENCY_REPLAY` di field `meta`.

Tidak ada instrumen baru kedua ter-INSERT. Jurnal posting tidak terduplikasi.

### SQL Verify

```sql
SELECT COUNT(*) FROM mst.instrumen
WHERE renewal_dari_instrumen_id = '<id_instrumen_lama>';
-- Expected: 1 (bukan 2)
```

---

## TC-08: Reject Renewal Happy Path

**Prioritas**: P1
**Persona**: Treasury Approver

### Pre-condition

Renewal baru dalam status `PENDING_APPROVAL` (buat ulang jika diperlukan).

### Langkah-Langkah

1. Login sebagai `treasury.approver`.
2. Buka renewal di Queue Approval.
3. Klik **Tolak (Reject)**.
4. Isi **Alasan Penolakan**: `"Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO terlebih dahulu."` (≥ 30 karakter).
5. Klik **Konfirmasi**.

### Expected Result

Toast: `"Renewal ditolak. Maker dapat merevisi dan mengajukan ulang."`.

Status: `REJECTED`.

Instrumen lama tetap `ACTIVE` (tidak berubah).

EIR schedule tidak berubah (effective_to tetap NULL).

### SQL Verify

```sql
SELECT status, reject_reason FROM trx.renewal WHERE id = '<renewal_id>';
-- Expected: status='REJECTED', reject_reason berisi alasan

SELECT status FROM mst.instrumen WHERE id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: 'ACTIVE' (tidak berubah)

SELECT effective_to FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: NULL (tidak berubah)
```

---

## TC-09: Reject — Komentar Terlalu Pendek

**Prioritas**: P2
**Persona**: Treasury Approver

### Langkah-Langkah

1. Di form Reject, isi alasan: `"Tidak setuju."` (< 30 karakter).
2. Klik **Konfirmasi**.

### Expected Result

Field alasan di-highlight merah: `"Komentar minimal 30 karakter (saat ini 14 karakter)."`.

Status renewal tidak berubah.

---

## TC-10: Kalkulasi PPh 20% dan Bunga Bersih

**Prioritas**: P0
**AC**: S4-AC3 (validasi konsistensi PPh)
**Persona**: Treasury Maker + Treasury Approver

### Langkah-Langkah

1. Buat renewal DEP-0042, skema POKOK_PLUS_BUNGA, rate 5.75%, tenor 12 bulan, tanggal efektif 2026-07-01.
2. Catat nilai **Bunga Kotor**, **PPh 20%**, **Bunga Bersih** dari preview.
3. Hitung manual:
   - Hari: 2026-07-01 − 2026-01-01 = **181 hari**
   - Bunga Kotor = 1.000.000.000 × (5,50/100) × (181/365) = **IDR 27.260.273,9726**
   - PPh 20% = 27.260.273,9726 × 0,20 = **IDR 5.452.054,7945**
   - Bunga Bersih = 27.260.273,9726 − 5.452.054,7945 = **IDR 21.808.219,1781**
4. Bandingkan nilai system vs perhitungan manual.
5. Approve renewal. Sistem harus re-verify PPh di sisi server (approver tidak bisa manipulasi PPh).

### Expected Result

Nilai system cocok dengan perhitungan manual (toleransi ±IDR 0,01).

PPh di panel approver terkunci (read-only) — tidak bisa diubah dari UI.

Approve berhasil POSTED.

---

## TC-11: EIR Baru Ter-Rekompute dengan Newton-Raphson

**Prioritas**: P0
**AC**: S4-AC1
**Persona**: Treasury Approver

### Pre-condition

Renewal dalam PENDING_APPROVAL.

### Langkah-Langkah

1. Approve renewal DEP-0042 (TC-05).
2. Setelah POSTED, navigasi ke detail instrumen baru.
3. Lihat tab **EIR Schedule**.

### Expected Result

Tab EIR Schedule instrumen baru menampilkan:

| Field | Nilai |
|---|---|
| Schedule Version | 1 |
| EIR Baru (pa) | ≈ 0,04600000 (8 desimal, after-PPh) |
| Effective From | 2026-07-01 |
| Effective To | (kosong / infinity) |

EIR baru < 5,75% gross rate (karena cashflow after-PPh).

EIR schedule instrumen LAMA: `effective_to = 2026-07-01`, nilai `eir_persen` TIDAK berubah (immutabilitas PSAK 71 §B5.4.6).

### SQL Verify

```sql
-- Old schedule: effective_to set, eir_persen immutable
SELECT schedule_version, eir_persen, effective_from, effective_to
FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'dep00042-0000-0000-0000-000000000001'
ORDER BY schedule_version;
-- Expected: effective_to = '2026-07-01', eir_persen = 0.04400000 (tidak berubah dari seed awal)

-- New schedule: version=1, effective_to NULL
SELECT s.instrumen_id, s.schedule_version, s.eir_persen, s.effective_from, s.effective_to
FROM ecl.amortisasi_schedule s
JOIN mst.instrumen i ON i.id = s.instrumen_id
WHERE i.renewal_dari_instrumen_id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: schedule_version=1, effective_to IS NULL, eir_persen < 0.0575
```

---

## TC-12: POKOK_SAJA — Pokok Baru Sama dengan Lama

**Prioritas**: P1
**AC**: S3-AC3
**Persona**: Treasury Maker + Treasury Approver

### Langkah-Langkah

1. Buat renewal DEP-0042, skema **POKOK_SAJA**, rate 5.75%, tenor 12 bulan.
2. Verifikasi di preview: **Pokok Baru = IDR 1.000.000.000,0000** (sama dengan pokok lama).
3. Approve renewal.
4. Verifikasi jurnal: leg ke-3 harus menggunakan pokok lama (IDR 1 miliar), bukan pokok + bunga.

### Expected Result

Jurnal RENEWAL_DEPOSITO — 4 leg:
- Leg 1: PPh 20% (debet Kewajiban PPh)
- Leg 2: IDR 1.000.000.000,0000 (pelunasan pokok lama)
- **Leg 3: IDR 1.000.000.000,0000** (penempatan pokok baru = pokok lama untuk POKOK_SAJA)
- Leg 4: IDR `bunga_bersih` (bunga bersih dikembalikan ke kas)

### SQL Verify

```sql
SELECT jl.urutan, jl.nilai
FROM jrnl.jurnal_leg jl
JOIN jrnl.jurnal_header jh ON jh.id = jl.jurnal_header_id
WHERE jh.event_code = 'RENEWAL_DEPOSITO'
  AND jh.renewal_id = '<renewal_id>'
ORDER BY jl.urutan;
-- Expected: leg urutan 3 nilai = 1000000000.0000 untuk POKOK_SAJA
```

---

## TC-13: Bunga Bersih Terlalu Kecil (POKOK_PLUS_BUNGA)

**Prioritas**: P1
**AC**: S2-AC2
**Persona**: Treasury Maker

### SQL Seed (TC-13)

```sql
-- Instrumen dengan pokok sangat kecil → bunga_bersih < IDR 100.000
INSERT INTO mst.instrumen (id, kode_instrumen, jenis_instrumen, status, klasifikasi_locked,
  pokok, mata_uang, rate_persen, tanggal_penempatan, tanggal_jatuh_tempo,
  counterparty_id, portofolio_id, klasifikasi_psak71, tenant_id, created_by, updated_by)
VALUES (
  'dep00099-0000-0000-0000-000000000001', 'DEP-KECIL', 'DEPOSITO', 'ACTIVE', TRUE,
  100000.0000, 'IDR', 2.0, '2026-06-28', '2026-07-01',
  'cntry001-0000-0000-0000-000000000001', 'portf001-0000-0000-0000-000000000001',
  'AC', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'
) ON CONFLICT DO NOTHING;
```

### Langkah-Langkah

1. Buat renewal DEP-KECIL, skema **POKOK_PLUS_BUNGA**, rate 2%, tenor 12 bulan, tanggal efektif 2026-07-01 (3 hari accrual).
2. Klik **Preview Kalkulasi**.
3. Klik **Simpan & Ajukan**.

### Expected Result

Preview menampilkan warning: `"Bunga bersih IDR xxx lebih kecil dari minimum IDR 100.000 untuk skema POKOK_PLUS_BUNGA. Gunakan POKOK_SAJA atau pilih instrumen dengan bunga lebih besar."`.

Sistem menolak submit: toast merah `RENEWAL_BUNGA_BERSIH_TOO_SMALL`.

Tidak ada INSERT ke database.

---

## TC-14: Periode Buku CLOSED — Tidak Bisa Post

**Prioritas**: P0
**AC**: S5-AC3
**Persona**: Finance Controller (hard-close) + Treasury Approver (gagal approve)

### Langkah-Langkah

1. Login sebagai `finance.controller`.
2. Hard-close periode buku bulan Juli 2026 (navigasi **Periode Buku → Hard Close → konfirmasi MFA**).
3. Switch ke `treasury.approver`.
4. Coba approve renewal yang masih `PENDING_APPROVAL`.

### Expected Result

Toast merah: `"Periode buku Juli 2026 sudah hard-closed. Tidak bisa memposting transaksi. Kode: PERIODE_CLOSED (HTTP 423)."`.

Renewal tetap `PENDING_APPROVAL`, tidak berubah.

Tidak ada instrumen baru, EIR schedule, atau jurnal ter-insert.

### SQL Verify

```sql
SELECT status FROM sys.periode_buku WHERE bulan = '2026-07-01';
-- Expected: status = 'CLOSED'

SELECT status FROM trx.renewal WHERE id = '<renewal_id>';
-- Expected: 'PENDING_APPROVAL'
```

### Rollback

```sql
-- Reopen periode (hanya untuk keperluan UAT berikutnya)
UPDATE sys.periode_buku SET status = 'OPEN' WHERE bulan = '2026-07-01';
```

---

## TC-15: List Renewal — Sort, Filter, Paging

**Prioritas**: P1
**Persona**: Treasury Maker / Treasury Approver

### Langkah-Langkah

1. Login sebagai `treasury.maker` atau `treasury.approver`.
2. Navigasi ke **Transaksi → Renewal Deposito** (list view).
3. Klik header kolom **Tanggal Dibuat** untuk sort descending.
4. Aktifkan filter **Status = PENDING_APPROVAL**.
5. Verifikasi paging: klik **Next** jika ada lebih dari 50 record.
6. Klik **Export → CSV**.

### Expected Result

List tampil dengan:
- Sort berhasil (ikon ↓ di kolom Tanggal Dibuat).
- Filter chip `"Status: PENDING_APPROVAL"` muncul di atas tabel.
- Baris hanya menampilkan renewal PENDING_APPROVAL.
- URL berubah menjadi `...?sort=created_at:desc&filter[status]=PENDING_APPROVAL`.
- Paging menampilkan `"Halaman 1 dari ~X"` dengan tombol Prev/Next.
- Export CSV ter-download dengan nama `renewal-YYYYMMDD.csv`.
- Audit `RENEWAL.EXPORT` ter-tulis.

### SQL Verify

```sql
SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'RENEWAL.EXPORT'
  AND actor_user_id = (SELECT id FROM sec.user WHERE username = 'treasury.maker');
-- Expected: ≥ 1
```

---

## TC-16: Audit Hash-Chain Integritas

**Prioritas**: P0
**AC**: Cross-cutting
**Persona**: Internal Auditor

### Langkah-Langkah

1. Setelah TC-05 selesai, login sebagai `internal.auditor`.
2. Navigasi ke **Audit Log → Verifikasi Hash Chain**.
3. Set rentang tanggal hari ini. Klik **Verifikasi**.
4. Atau jalankan di server:
   ```bash
   go run ./cmd/audit-verify --range "2026-07-01:2026-07-01"
   ```

### Expected Result

Semua row `aud.audit_log` untuk tanggal UAT: status `VALID` (hash chain tidak terputus).

Urutan event harus ditemukan untuk entity renewal:
1. `RENEWAL.CREATED`
2. `RENEWAL.APPROVED`
3. `RENEWAL.POSTED`
4. `INSTRUMEN.CREATED`
5. `INSTRUMEN.MATURED`
6. `EIR.RECOMPUTED`

### SQL Verify

```sql
-- Hash chain spot-check untuk renewal hari ini
SELECT a.event_id, a.action, a.entity_id,
       encode(a.previous_hash, 'hex') AS prev_hash,
       encode(a.current_hash,  'hex') AS curr_hash,
       a.event_time
FROM aud.audit_log a
WHERE a.event_time >= CURRENT_DATE
  AND a.action LIKE 'RENEWAL%' OR a.action LIKE 'INSTRUMEN%' OR a.action LIKE 'EIR%'
ORDER BY a.event_time;
-- Manual: verifikasi sha256(previous_hash || canonical_json(row)) == current_hash untuk setiap row
```

---

## TC-17: Idempotency Mismatch

**Prioritas**: P1
**Persona**: Treasury Maker (via API/Postman)

### Langkah-Langkah

1. Kirim POST `/api/v1/trx/renewal` dengan header `Idempotency-Key: UAT-IDEM-123`.
2. Kirim ulang POST yang SAMA key `UAT-IDEM-123` tapi dengan `instrumen_id` berbeda (payload berbeda).

### Expected Result

HTTP 422, error code `IDEMPOTENCY_MISMATCH`, message: `"Idempotency-Key sudah dipakai dengan payload berbeda."`.

Hanya 1 renewal ter-INSERT (dari request pertama).

---

## TC-18: Instrumen Baru Tidak Bisa Di-Renew Ulang Sebelum Jatuh Tempo

**Prioritas**: P2
**AC**: Cross-cutting

### Pre-condition

TC-05 berhasil. Instrumen baru (DEP-0042B) sudah ACTIVE.

### Langkah-Langkah

1. Coba buat renewal dengan instrumen baru DEP-0042B sebelum tanggal jatuh tempo (2027-07-01).

### Expected Result

Form: di kolom Tanggal Efektif Baru, sistem memvalidasi bahwa tanggal tidak boleh sebelum tanggal_jatuh_tempo instrumen. Error: `"Tanggal efektif baru harus sama dengan atau setelah tanggal jatuh tempo instrumen."`.

---

## TC-19: Instrumen Bukan Eligible — Klasifikasi Belum Locked

**Prioritas**: P1
**AC**: S1-AC4 variant

### SQL Seed (TC-19)

```sql
INSERT INTO mst.instrumen (id, kode_instrumen, jenis_instrumen, status, klasifikasi_locked,
  pokok, mata_uang, rate_persen, tanggal_penempatan, tanggal_jatuh_tempo,
  counterparty_id, portofolio_id, klasifikasi_psak71, tenant_id, created_by, updated_by)
VALUES (
  'dep00088-0000-0000-0000-000000000001', 'DEP-0088', 'DEPOSITO', 'ACTIVE',
  FALSE, -- klasifikasi belum locked
  500000000.0000, 'IDR', 5.50, '2026-01-01', '2026-07-01',
  'cntry001-0000-0000-0000-000000000001', 'portf001-0000-0000-0000-000000000001',
  'AC', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'
) ON CONFLICT DO NOTHING;
```

### Langkah-Langkah

1. Coba buat renewal dengan `DEP-0088` (klasifikasi_locked=FALSE).

### Expected Result

Error `RENEWAL_INSTRUMEN_NOT_ELIGIBLE`: `"DEP-0088: klasifikasi_locked=FALSE. Klassifikasi PSAK 71 harus terkunci sebelum renewal."`.

---

## TC-20: Rollback Atomik — Jurnal Gagal

**Prioritas**: P0
**AC**: S5-AC4
**Persona**: IT Admin (simulasi konfigurasi error di mapping jurnal)

### Pre-condition

Matikan konfigurasi event code `RENEWAL_DEPOSITO` di mapping jurnal (simulate dari Admin Panel atau temporarily rename di DB).

```sql
-- Simulasi: disable mapping RENEWAL_DEPOSITO
UPDATE jrnl.mapping_jurnal SET aktif = FALSE
WHERE event_code = 'RENEWAL_DEPOSITO';
```

### Langkah-Langkah

1. Approve renewal dalam status `PENDING_APPROVAL`.
2. Sistem mencoba posting jurnal → mapping tidak ditemukan → error.

### Expected Result

Toast merah: `"Gagal memposting jurnal: event code RENEWAL_DEPOSITO tidak terkonfigurasi. Silakan hubungi ROLE-AKUN-CTL. Trace: xxxx."`.

Renewal tetap `PENDING_APPROVAL` (rollback atomik).

Instrumen lama tetap `ACTIVE`.

Tidak ada instrumen baru ter-INSERT.

Tidak ada EIR schedule baru.

### SQL Verify

```sql
SELECT status FROM trx.renewal WHERE id = '<renewal_id>';
-- Expected: 'PENDING_APPROVAL'

SELECT status FROM mst.instrumen WHERE id = 'dep00042-0000-0000-0000-000000000001';
-- Expected: 'ACTIVE'
```

### Rollback Seed TC-20

```sql
UPDATE jrnl.mapping_jurnal SET aktif = TRUE WHERE event_code = 'RENEWAL_DEPOSITO';
```

---

## Post-UAT Cleanup

```sql
-- Cleanup semua data UAT (jalankan setelah seluruh TC selesai)
DELETE FROM trx.renewal WHERE tenant_id = 'TUGURE' AND kode_instrumen_lama LIKE 'DEP-00%';

UPDATE mst.instrumen SET status = 'ACTIVE', deleted_at = NULL
WHERE id = 'dep00042-0000-0000-0000-000000000001';

DELETE FROM mst.instrumen WHERE renewal_dari_instrumen_id = 'dep00042-0000-0000-0000-000000000001';
DELETE FROM mst.instrumen WHERE id IN (
  'dep00099-0000-0000-0000-000000000001',
  'dep00088-0000-0000-0000-000000000001',
  'obl00099-0000-0000-0000-000000000001'
);

UPDATE ecl.amortisasi_schedule SET effective_to = NULL
WHERE instrumen_id = 'dep00042-0000-0000-0000-000000000001';
```

---

## Matriks Cakupan AC

| TC | AC | Persona | SoD | Audit | EIR | Jurnal | Idempotency |
|---|---|---|---|---|---|---|---|
| TC-01 | S1-AC1 | Maker | - | RENEWAL.CREATED | Preview | - | - |
| TC-02 | S1-AC2 | Maker | - | - | - | - | - |
| TC-03 | S1-AC3 | Maker | - | - | - | - | - |
| TC-04 | S1-AC4 | Maker | - | - | - | - | - |
| TC-05 | S2-AC1, S3-AC1, S3-AC2, S4-AC1, S5-AC1 | Approver | ya | 6 events | NR convergence | 4 legs | - |
| TC-06 | S2-AC3 | Maker (fail) | **ya** | SOD_ATTEMPT | - | - | - |
| TC-07 | S2-AC4 | Approver | ya | - | - | - | **ya** |
| TC-08 | S2 reject | Approver | ya | RENEWAL.REJECTED | - | - | - |
| TC-09 | S2 reject short | Approver | - | - | - | - | - |
| TC-10 | S4-AC3 PPh | Maker+Approver | ya | - | re-verify | - | - |
| TC-11 | S4-AC1 EIR | Approver | ya | EIR.RECOMPUTED | **immutable old** | - | - |
| TC-12 | S3-AC3 POKOK_SAJA | Maker+Approver | ya | - | - | leg 3 | - |
| TC-13 | S2-AC2 bunga kecil | Maker | - | - | - | - | - |
| TC-14 | S5-AC3 periode closed | Controller+Approver | ya | - | rollback | rollback | - |
| TC-15 | Cross list | Maker/Approver | - | EXPORT audit | - | - | - |
| TC-16 | Cross audit | Auditor | - | **hash-chain** | - | - | - |
| TC-17 | Cross idem mismatch | Maker (API) | - | - | - | - | **mismatch** |
| TC-18 | Cross tenor | Maker | - | - | - | - | - |
| TC-19 | S1-AC4 variant | Maker | - | - | - | - | - |
| TC-20 | S5-AC4 rollback | IT Admin | ya | rollback | rollback | **fail→rollback** | - |

---

## Referensi

- `docs/stories/phase-5/P5-M7-renewal-deposito.md` — User stories + AC lengkap
- `docs/state-machines/p5-m7-renewal-deposito.md` — State transitions + side-effects
- `api/openapi/app-b-renewal-deposito.yaml` — Kontrak API
- `backend/tests/e2e/p5_m7_renewal_test.go` — E2E test scenarios (mirror TC ini)
- PP No. 131/2000 — PPh final deposito 20%
- PSAK 71 §5.4.3 — Amandemen kontrak (EIR re-estimasi)
- PSAK 71 §B5.4.6 — Immutabilitas EIR schedule lama
