# UAT-APP-B-004: Penjualan/Pencairan Instrumen (P5-M8)

**Modul**: APP-B Transaction Lifecycle
**Fase**: P5-M8
**Versi Dokumen**: 1.0
**Tanggal**: 2026-06-20
**Penulis**: qa-engineer

---

## Scope

Penjualan dan pencairan instrumen PSAK 71 (disposal): create penjualan dengan preview proceeds/cost_basis/realized_gl, OCI recycling untuk FVOCI debt (REKLAS_OCI_PL) dan no-recycle untuk FVOCI Election (PSAK 71 §B5.7.1), BM frequency check untuk portofolio HTC (warn 5%, block 10% dari sys.config), routing jurnal 5 klasifikasi via P5-M2, derecognition instrumen, dan workflow Maker → Approver dengan SoD 4-eyes.

---

## Pre-Conditions

### Roles yang diperlukan

| Actor | Role | Username (UAT) |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | `treasury.maker` |
| Treasury Approver | ROLE-APPR-TR | `treasury.approver` |
| Risk Officer | ROLE-RISK | `risk.officer` |
| Finance Controller | ROLE-AKUN-CTL | `finance.controller` |
| Internal Auditor | ROLE-AUDIT | `internal.auditor` |

### SQL Seed (jalankan sebelum UAT, dalam urutan ini)

```sql
-- 1. Periode buku OPEN
INSERT INTO sys.periode_buku (id, bulan, status, tenant_id, created_by, updated_by)
VALUES (
  'eee00004-0008-0000-0000-000000000001',
  '2026-07-01',
  'OPEN',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (bulan) DO UPDATE SET status = 'OPEN';

-- 2. BM threshold config (sys.config)
INSERT INTO sys.config (key, value, tenant_id, created_by, updated_by)
VALUES
  ('BM_WARN_THRESHOLD_PCT', '5.0', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'),
  ('BM_BLOCK_THRESHOLD_PCT', '10.0', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- 3. Portofolio HTC
INSERT INTO mst.portofolio (id, kode_portofolio, nama, business_model, tenant_id, created_by, updated_by)
VALUES (
  'port0001-0000-0000-0000-000000000001',
  'PORTO-HTC-BOND',
  'Portofolio Obligasi HTC',
  'HTC',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_portofolio) DO NOTHING;

-- 4. Portofolio HTC&S (untuk FVOCI Election)
INSERT INTO mst.portofolio (id, kode_portofolio, nama, business_model, tenant_id, created_by, updated_by)
VALUES (
  'port0002-0000-0000-0000-000000000002',
  'PORTO-HTCS-EQUITY',
  'Portofolio Ekuitas HTC&S',
  'HTC&S',
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_portofolio) DO NOTHING;

-- 5. Instrumen FVOCI Obligasi (ACTIVE, klasifikasi locked)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71, klasifikasi_locked,
  status, qty_holding, portofolio_id, mata_uang,
  cost_basis_idr, oci_cumulative_idr,
  tenant_id, created_by, updated_by
) VALUES (
  'instr001-0000-0000-0000-000000000001',
  'OBL-TEST-0077',
  'Obligasi Negara FR0077 — UAT P5-M8',
  'OBLIGASI',
  'FVOCI',
  true,
  'ACTIVE',
  1000,
  'port0001-0000-0000-0000-000000000001',
  'IDR',
  1023500000.0000,
  18200000.0000,
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO UPDATE SET
  status = 'ACTIVE',
  qty_holding = 1000,
  klasifikasi_locked = true;

-- 6. Instrumen FVOCI Election Saham (ACTIVE, klasifikasi locked)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71, klasifikasi_locked,
  status, qty_holding, portofolio_id, mata_uang,
  cost_basis_idr, oci_cumulative_idr,
  tenant_id, created_by, updated_by
) VALUES (
  'instr002-0000-0000-0000-000000000002',
  'SHM-TEST-0011',
  'Saham PT Telkom Tbk — UAT P5-M8 FVOCI Election',
  'SAHAM',
  'FVOCI_ELECTION',
  true,
  'ACTIVE',
  1000,
  'port0002-0000-0000-0000-000000000002',
  'IDR',
  10000000.0000,
  2000000.0000,
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO UPDATE SET
  status = 'ACTIVE',
  qty_holding = 1000,
  klasifikasi_locked = true;

-- 7. Instrumen AC Deposito (ACTIVE, klasifikasi locked)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71, klasifikasi_locked,
  status, qty_holding, portofolio_id, mata_uang,
  cost_basis_idr, oci_cumulative_idr,
  tenant_id, created_by, updated_by
) VALUES (
  'instr003-0000-0000-0000-000000000003',
  'DEP-TEST-0050',
  'Deposito BCA UAT P5-M8 AC',
  'DEPOSITO',
  'AC',
  true,
  'ACTIVE',
  500,
  'port0001-0000-0000-0000-000000000001',
  'IDR',
  500000000.0000,
  0.0000,
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO UPDATE SET
  status = 'ACTIVE',
  qty_holding = 500,
  klasifikasi_locked = true;

-- 8. Instrumen MATURED (untuk TC-03 negatif)
INSERT INTO mst.instrumen (
  id, kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71, klasifikasi_locked,
  status, qty_holding, portofolio_id, mata_uang,
  cost_basis_idr, oci_cumulative_idr,
  tenant_id, created_by, updated_by
) VALUES (
  'instr004-0000-0000-0000-000000000004',
  'DEP-TEST-MATURED',
  'Deposito Matured — UAT P5-M8 negatif',
  'DEPOSITO',
  'AC',
  true,
  'MATURED',
  100,
  'port0001-0000-0000-0000-000000000001',
  'IDR',
  100000000.0000,
  0.0000,
  'TUGURE',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO NOTHING;

-- 9. P5-M2 jurnal mapping — pastikan semua event code tersedia
INSERT INTO jrnl.event_jurnal_mapping (event_code, jenis_instrumen, debit_akun, kredit_akun, deskripsi, tenant_id, created_by, updated_by)
VALUES
  ('PENJUALAN_AC', 'DEPOSITO', '111100', '117000', 'Penjualan AC — Kas Dr, Aset Cr', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'),
  ('PENJUALAN_FVOCI_DEBT', 'OBLIGASI', '111100', '118000', 'Penjualan FVOCI debt — Kas Dr, Aset Cr', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'),
  ('REKLAS_OCI_PL', 'OBLIGASI', '320000', '411100', 'Reklasifikasi OCI ke P&L — Penghasilan/(Rugi)', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001'),
  ('PENJUALAN_FVOCI_ELECTION', 'SAHAM', '111100', '118500', 'Penjualan FVOCI Election — Kas Dr, Aset Cr, G/L di OCI', 'TUGURE', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (event_code, jenis_instrumen) DO NOTHING;
```

---

## Test Cases

### TC-M8-01 — Buat Penjualan FVOCI PARTIAL: Preview Correct (S1-AC1)

**Objective**: Maker membuat penjualan 500 unit obligasi FVOCI PARTIAL. Preview menampilkan proceed/cost_basis/realized_gl/oci_recycled yang benar.

**Actor**: ROLE-MAKER-TR (`treasury.maker`)

**Langkah**:
1. Login sebagai `treasury.maker`.
2. Navigasi ke `/transaksi/penjualan/new?instrumenId=instr001-0000-0000-0000-000000000001&kode=OBL-TEST-0077`.
3. Pilih jenis disposal **Sebagian (PARTIAL)**.
4. Isi qty_terjual: `500`.
5. Isi harga_jual_per_unit: `1050000`.
6. Isi tanggal_eksekusi: `2026-07-15`.
7. Klik **Buat Penjualan**.

**Expected**:
- Respons 201 diterima.
- Preview panel tampil:
  - Proceed IDR: Rp 525.000.000,0000
  - Cost Basis: Rp 499.250.000,0000 (= 998.500.000 × 500/1000)
  - Realized G/L: Rp 25.750.000,0000
  - OCI Recycle ke P&L: Rp 9.100.000,0000 (= 18.200.000 × 500/1000)
  - Routing: `PENJUALAN_FVOCI_DEBT` + `REKLAS_OCI_PL`
- Success toast: "Penjualan OBL-TEST-0077 (500 unit) berhasil dibuat. Menunggu approval Treasury Approver."
- Status penjualan: **PENDING_APPROVAL**.

**Verifikasi DB**:
```sql
SELECT id, status, proceed_idr, cost_basis, realized_gl, oci_recycled
FROM trx.penjualan
WHERE instrumen_id = 'instr001-0000-0000-0000-000000000001'
ORDER BY created_at DESC LIMIT 1;
-- proceed_idr = 525000000.0000
-- cost_basis = 499250000.0000
-- realized_gl = 25750000.0000
-- oci_recycled = 9100000.0000
-- status = 'PENDING_APPROVAL'
```

---

### TC-M8-02 — Buat Penjualan FVOCI Election FULL: No Recycling Note (S1-AC4)

**Objective**: Maker membuat penjualan 1000 unit saham FVOCI Election. System menampilkan no_recycling_note karena PSAK 71 §B5.7.1 — gain/loss tetap di OCI.

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Navigasi ke `/transaksi/penjualan/new?instrumenId=instr002-0000-0000-0000-000000000002&kode=SHM-TEST-0011`.
2. Pilih jenis disposal **Penuh (FULL)**.
3. Isi qty_terjual: `1000`.
4. Isi harga_jual_per_unit: `12000`.
5. Isi tanggal_eksekusi: `2026-07-15`.
6. Klik **Buat Penjualan**.

**Expected**:
- Preview panel tampil:
  - Proceed IDR: Rp 12.000.000,0000
  - Cost Basis: Rp 10.000.000,0000
  - Realized G/L: Rp 2.000.000,0000
  - OCI Recycled: **null** (badge "Tetap di OCI" warna slate)
  - No Recycling Note tampil: "Gain/loss IDR 2.000.000,0000 tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L."
  - Routing: `PENJUALAN_FVOCI_ELECTION` (tanpa `REKLAS_OCI_PL`)
- Status: **PENDING_APPROVAL**.

**Verifikasi DB**:
```sql
SELECT oci_recycled, no_recycling_note
FROM trx.penjualan
WHERE instrumen_id = 'instr002-0000-0000-0000-000000000002'
ORDER BY created_at DESC LIMIT 1;
-- oci_recycled IS NULL
-- no_recycling_note LIKE '%B5.7.1%'
```

---

### TC-M8-03 — Buat Penjualan: Qty Melebihi Holding (S1-AC2 Negatif)

**Objective**: System menolak create penjualan ketika qty_terjual > qty_holding instrumen.

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Navigasi ke `/transaksi/penjualan/new?instrumenId=instr001-0000-0000-0000-000000000001`.
2. Isi qty_terjual: `1500` (melebihi holding 1000).
3. Isi harga_jual_per_unit: `1050000`.
4. Isi tanggal_eksekusi: `2026-07-15`.
5. Klik **Buat Penjualan**.

**Expected**:
- Respons 422 dengan error code `PENJUALAN_QTY_EXCEEDS_HOLDING`.
- Toast merah: "qty_terjual 1500 melebihi qty_holding saat ini: 1000 unit OBL-TEST-0077."
- Field `qtyTerjual` di-highlight merah + inline message.
- Tidak ada row baru di `trx.penjualan`.

---

### TC-M8-04 — Buat Penjualan: Instrumen MATURED (S1-AC3 Negatif)

**Objective**: System menolak penjualan instrumen dengan status bukan ACTIVE.

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Navigasi ke `/transaksi/penjualan/new?instrumenId=instr004-0000-0000-0000-000000000004`.
2. Isi field secukupnya.
3. Klik **Buat Penjualan**.

**Expected**:
- Respons 422 dengan error code `PENJUALAN_INSTRUMEN_NOT_ACTIVE`.
- Toast merah: "DEP-TEST-MATURED tidak eligible untuk penjualan: status=MATURED."

---

### TC-M8-05 — Buat Penjualan: Periode CLOSED (S1 Negatif)

**Objective**: System menolak create penjualan ketika periode buku CLOSED.

**Prerequisite**:
```sql
UPDATE sys.periode_buku SET status = 'CLOSED' WHERE bulan = '2026-07-01';
```

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Coba buat penjualan instrumen apapun.

**Expected**:
- Respons 423 dengan error code `PENJUALAN_PERIODE_LOCKED`.
- Toast merah: "Periode buku sudah di-close, tidak bisa membuat penjualan baru."

**Cleanup**:
```sql
UPDATE sys.periode_buku SET status = 'OPEN' WHERE bulan = '2026-07-01';
```

---

### TC-M8-06 — Buat Penjualan: Harga ≤ 0 (S1 Negatif)

**Objective**: System menolak harga_jual_per_unit yang tidak valid.

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Navigasi ke form penjualan instrumen OBL-TEST-0077.
2. Isi harga_jual_per_unit: `0`.
3. Klik **Buat Penjualan**.

**Expected**:
- Respons 400 dengan error code `PENJUALAN_HARGA_INVALID`.
- Field `hargaJualPerUnit` di-highlight merah.

---

### TC-M8-07 — Approve FVOCI PARTIAL: Semua Side-Effect Satu Transaksi DB (S2-AC1)

**Prerequisite**: Buat penjualan via TC-M8-01 terlebih dahulu. Catat `penjualan_id`.

**Objective**: Approver menyetujui penjualan. Dalam satu DB transaction: OCI recycled diposting, jurnal P5-M2 digenerate, instrumen qty_holding dikurangi, status POSTED.

**Actor**: ROLE-APPR-TR (`treasury.approver`)

**Langkah**:
1. Login sebagai `treasury.approver`.
2. Navigasi ke `/transaksi/penjualan/{penjualan_id}`.
3. Klik **Setujui & Posting**.
4. Isi komentar: "Preview diverifikasi. Harga OBL-TEST-0077 sesuai IBPA closing 2026-07-15. OCI 9.1 juta direkognisi di P&L. Disetujui."
5. Klik **Setuju & Posting (JWT Step-Up)**.

**Expected**:
- Status penjualan berubah ke **POSTED**.
- Toast hijau: "Penjualan OBL-TEST-0077 (500 unit) disetujui dan diposting. Instrumen qty_holding: 500. Jurnal REKLAS_OCI_PL dipost."
- Jurnal P5-M2 dibuat dengan event codes `PENJUALAN_FVOCI_DEBT` + `REKLAS_OCI_PL`.

**Verifikasi DB**:
```sql
-- Penjualan status POSTED
SELECT status, jurnal_header_id, oci_recycled, instrumen_status_after
FROM trx.penjualan WHERE id = '{penjualan_id}';
-- status = 'POSTED'
-- jurnal_header_id NOT NULL
-- oci_recycled = 9100000.0000
-- instrumen_status_after = 'ACTIVE' (PARTIAL)

-- Instrumen qty_holding berkurang
SELECT qty_holding FROM mst.instrumen
WHERE id = 'instr001-0000-0000-0000-000000000001';
-- qty_holding = 500 (bukan 1000 lagi)

-- Jurnal tersimpan
SELECT event_code FROM jrnl.jurnal_header
WHERE id = (SELECT jurnal_header_id FROM trx.penjualan WHERE id = '{penjualan_id}');

-- Audit chain
SELECT action, created_at FROM aud.audit_log
WHERE entity_id = '{penjualan_id}'::uuid
ORDER BY event_time;
-- Harus ada: PENJUALAN.CREATED, PENJUALAN.APPROVED, PENJUALAN.OCI_RECYCLED, PENJUALAN.DERECOGNIZED, PENJUALAN.POSTED
```

---

### TC-M8-08 — SoD: Maker Tidak Bisa Approve Penjualan Sendiri (S2-AC2)

**Prerequisite**: Buat penjualan baru oleh `treasury.maker`.

**Objective**: System menolak ketika maker mencoba approve penjualan yang dia buat sendiri.

**Actor**: ROLE-MAKER-TR (`treasury.maker`) mencoba approve.

**Langkah**:
1. Login sebagai `treasury.maker`.
2. Navigasi ke `/transaksi/penjualan/{penjualan_id}`.
3. Bila tombol **Setujui** visible (misalnya karena exploit direct API), kirim langsung via curl:
   ```bash
   curl -X POST /api/v1/trx/penjualan/{penjualan_id}/approve \
     -H "Authorization: Bearer {token_maker}" \
     -H "Idempotency-Key: $(uuidgen)" \
     -d '{"comment":"Self-approve","signatureMethod":"JWT_STEP_UP"}'
   ```

**Expected**:
- Respons 403 dengan error code `SOD_VIOLATION`.
- Penjualan tetap PENDING_APPROVAL.
- Audit log entry `PENJUALAN.SOD_VIOLATION_ATTEMPT` ditulis.

**UI Check**: Tombol **Setujui** harus tidak tampil untuk `treasury.maker` di detail halaman penjualan yang dia buat sendiri.

---

### TC-M8-09 — Reject: Alasan ≥ 30 Karakter (S2 Happy Path)

**Prerequisite**: Buat penjualan baru. Catat `penjualan_id`.

**Actor**: ROLE-APPR-TR

**Langkah**:
1. Navigasi ke detail penjualan.
2. Klik **Tolak**.
3. Isi alasan: "Harga jual 1.050.000 melebihi IBPA fair value 1.035.000 lebih dari 2%. Harap revisi atau klarifikasi."
4. Klik **Tolak (Konfirmasi)**.

**Expected**:
- Status berubah ke **REJECTED**.
- Toast hijau: "Penjualan ditolak. Alasan tercatat."
- Audit log `PENJUALAN.REJECTED` ditulis.

**Verifikasi DB**:
```sql
SELECT status, reject_reason FROM trx.penjualan WHERE id = '{penjualan_id}';
-- status = 'REJECTED'
-- reject_reason LIKE '%IBPA%'
```

---

### TC-M8-10 — Reject: Alasan < 30 Karakter (Negatif)

**Actor**: ROLE-APPR-TR

**Langkah**:
1. Di dialog reject, isi alasan: "Terlalu mahal" (13 karakter).
2. Perhatikan counter "13 / 30" berwarna merah.
3. Tombol **Tolak (Konfirmasi)** tetap disabled.

**Expected**:
- Tombol tidak bisa diklik.
- Bila API dipanggil langsung, respons 400 `VALIDATION_FAILED` dengan pesan "reason minimal 30 karakter."

---

### TC-M8-11 — Idempotency: Double-Submit Create (Cross-Cutting)

**Actor**: ROLE-MAKER-TR

**Langkah**:
1. Kirim POST create penjualan dengan `Idempotency-Key: {key-sama}` dua kali dengan payload identik.

**Expected**:
- Panggilan pertama: 201 Created.
- Panggilan kedua (dalam 24 jam): 200 `IDEMPOTENCY_REPLAY` — response sama dengan pertama.
- Hanya satu row di `trx.penjualan`.

---

### TC-M8-12 — Idempotency: Double-Submit Approve (Cross-Cutting)

**Prerequisite**: Penjualan dalam status PENDING_APPROVAL.

**Langkah**:
1. Approver kirim POST approve dengan `Idempotency-Key: {key-approve}` dua kali.

**Expected**:
- Panggilan pertama: 200 OK, status POSTED.
- Panggilan kedua: 200 `IDEMPOTENCY_REPLAY`.
- Tidak ada duplikasi jurnal atau audit event.

---

### TC-M8-13 — FVOCI Debt FULL: OCI Recycled = OCI Cumulative Total (S3-AC1)

**Prerequisite**: Buat penjualan FULL untuk OBL-TEST-0077 (1000 unit, semua qty).

**Actor**: ROLE-MAKER-TR (create) → ROLE-APPR-TR (approve)

**Langkah**:
1. Buat penjualan FULL disposal 1000 unit OBL-TEST-0077, harga 1.050.000.
2. Approve.

**Expected**:
- oci_recycled = 18.200.000,0000 (= oci_cumulative total, bukan proporsi).
- Jurnal REKLAS_OCI_PL dipost dengan nominal 18.200.000.
- Instrumen status = **DISPOSED**.
- Audit event `PENJUALAN.OCI_RECYCLED` ditulis.

**Verifikasi DB**:
```sql
SELECT oci_recycled, instrumen_status_after FROM trx.penjualan
WHERE id = '{penjualan_id}';
-- oci_recycled = 18200000.0000
-- instrumen_status_after = 'DISPOSED'

SELECT status FROM mst.instrumen
WHERE id = 'instr001-0000-0000-0000-000000000001';
-- status = 'DISPOSED'
```

---

### TC-M8-14 — FVOCI Debt PARTIAL: OCI Recycled Proporsional (S3-AC2)

**Objective**: OCI recycled = oci_cumulative × (qty_terjual / qty_holding_pre).

**Langkah**:
1. Pastikan qty_holding OBL-TEST-0077 = 1000 (reset bila perlu).
2. Buat penjualan PARTIAL 300 unit, harga 1.050.000.
3. Approve.

**Expected**:
- oci_recycled = 18.200.000 × (300 / 1000) = **5.460.000,0000**.
- Instrumen qty_holding = 700 (ACTIVE).
- Instrumen status = **ACTIVE**.

---

### TC-M8-15 — FVOCI Election FULL: Tidak Ada REKLAS_OCI_PL, Warning Ditampilkan (S3-AC3)

**Prerequisite**: Buat penjualan FULL SHM-TEST-0011 (1000 unit). Approve.

**Expected saat approve**:
- Response body berisi `warnings: ["PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN"]`.
- Toast amber: "Perhatian: Gain/loss IDR 2.000.000,0000 tetap di OCI per PSAK 71 §B5.7.1. Tidak ada REKLAS_OCI_PL."
- Jurnal yang dipost: hanya `PENJUALAN_FVOCI_ELECTION` (tanpa `REKLAS_OCI_PL`).
- Audit event `PENJUALAN.OCI_NO_RECYCLE` ditulis.

**Verifikasi DB**:
```sql
SELECT oci_recycled, no_recycling_note FROM trx.penjualan
WHERE id = '{penjualan_id}';
-- oci_recycled IS NULL
-- no_recycling_note LIKE '%B5.7.1%'

-- Jurnal tidak ada REKLAS_OCI_PL
SELECT COUNT(*) FROM jrnl.jurnal_leg jl
JOIN jrnl.jurnal_header jh ON jl.jurnal_header_id = jh.id
WHERE jh.id = (SELECT jurnal_header_id FROM trx.penjualan WHERE id = '{penjualan_id}')
AND jl.event_code = 'REKLAS_OCI_PL';
-- COUNT = 0
```

---

### TC-M8-16 — BM Warn (5–10%): Penjualan POSTED + bm_violation_risk=true (S4-AC1)

**Prerequisite**: Set cumulative penjualan portofolio HTC = 3.5% dari total.
```sql
UPDATE sys.bm_cumulative_sold_log
SET cumulative_sold_idr = 350000000
WHERE portofolio_id = 'port0001-0000-0000-0000-000000000001';
-- (total portofolio = 10B IDR)
```

**Objective**: Penjualan baru sebesar 200 juta (total 5.5% > 5% warn) → POSTED tapi bm_violation_risk=true dan ROLE-RISK mendapat notifikasi.

**Langkah**:
1. Buat penjualan PARTIAL OBL-TEST-0077: 200 unit × 1.000.000 = 200 juta.
2. Approve.

**Expected**:
- Penjualan status **POSTED**.
- Response: `bmViolationRisk: true`.
- Audit event `PENJUALAN.BM_FREQUENCY_FLAG` dengan flag `BM_VIOLATION_RISK`.
- Notifikasi ke ROLE-RISK tersimpan.
- BM Alerts page `/transaksi/penjualan/bm-alerts` menampilkan entry untuk portofolio ini.

---

### TC-M8-17 — BM Block (>10%): Penjualan PENDING_BM_REVIEW (S4-AC2)

**Prerequisite**: Set cumulative penjualan = 9.8% dari total portofolio.
```sql
UPDATE sys.bm_cumulative_sold_log
SET cumulative_sold_idr = 980000000
WHERE portofolio_id = 'port0001-0000-0000-0000-000000000001';
```

**Objective**: Penjualan baru 250 juta (total 12.3% > 10% block) → ditahan PENDING_BM_REVIEW.

**Langkah**:
1. Buat penjualan PARTIAL: 250 unit × 1.000.000.
2. Approver coba approve.

**Expected**:
- Respons 422 dengan error code `PENJUALAN_BM_VIOLATION_BLOCK`.
- Status penjualan berubah ke **PENDING_BM_REVIEW**.
- Tidak ada jurnal yang dipost.
- Audit event `PENJUALAN.BM_FREQUENCY_FLAG` dengan flag `BM_VIOLATION_BLOCK`.
- BM Alerts page menampilkan entry merah untuk portofolio ini.

---

### TC-M8-18 — BM Check: HTC&S Portofolio Dilewati (S4-AC3)

**Objective**: Instrumen di portofolio HTC&S tidak terkena BM frequency check.

**Prerequisite**: Buat instrumen FVTPL di portofolio PORTO-HTCS-EQUITY.

**Langkah**:
1. Buat penjualan FVTPL dari portofolio HTC&S dengan nilai berapapun.
2. Approve.

**Expected**:
- Penjualan POSTED tanpa BM check.
- Tidak ada audit event `PENJUALAN.BM_FREQUENCY_FLAG`.
- `bmViolationRisk: false` di response.

---

### TC-M8-19 — Dispose FULL AC: Instrumen DISPOSED (S5-AC1)

**Objective**: Full disposal instrumen AC → instrumen status DISPOSED.

**Prerequisite**: DEP-TEST-0050 qty_holding = 500.

**Langkah**:
1. Buat penjualan FULL DEP-TEST-0050: 500 unit × 1.020.000.
2. Approve.

**Expected**:
- Penjualan **POSTED**.
- Instrumen DEP-TEST-0050 status = **DISPOSED**.
- Jurnal event code: `PENJUALAN_AC`.
- Audit `PENJUALAN.DERECOGNIZED` ditulis.

**Verifikasi DB**:
```sql
SELECT status FROM mst.instrumen
WHERE kode_instrumen = 'DEP-TEST-0050';
-- status = 'DISPOSED'
```

---

### TC-M8-20 — Dispose PARTIAL AC: Instrumen ACTIVE, Qty Berkurang (S5-AC2 + Cross-Cutting)

**Objective**: Partial disposal instrumen AC → instrumen tetap ACTIVE, qty_holding berkurang tepat.

**Langkah**:
1. Buat penjualan PARTIAL DEP-TEST-0050: 200 unit × 1.010.000.
2. Approve.

**Expected**:
- Penjualan **POSTED**.
- Instrumen DEP-TEST-0050 qty_holding = 500 − 200 = **300**.
- Instrumen status tetap **ACTIVE**.
- cost_basis dikurangi proporsional: 500.000.000 × (200/500) = 200.000.000.
- Semua nilai IDR presisi 4 desimal (DEC-016: NUMERIC(20,4)).

**Verifikasi DB**:
```sql
SELECT qty_holding, status FROM mst.instrumen
WHERE kode_instrumen = 'DEP-TEST-0050';
-- qty_holding = 300
-- status = 'ACTIVE'
```

---

## Cross-Cutting Checks

### CC-01 — Audit Hash-Chain Integrity

Setelah seluruh TC selesai:
```sql
SELECT event_id, action, previous_hash, current_hash
FROM aud.audit_log
WHERE entity_type = 'trx.penjualan'
ORDER BY event_time;
```
Jalankan verifier:
```bash
go run ./cmd/audit-verify --entity-type trx.penjualan --range "2026-07-01:2026-07-31"
```
**Expected**: `CHAIN OK — 0 broken links`.

### CC-02 — No Float64 in Money Columns

```sql
-- Pastikan tidak ada rounding error float64
SELECT id, proceed_idr, cost_basis, realized_gl, oci_recycled
FROM trx.penjualan
WHERE proceed_idr::text LIKE '%.%'
AND length(split_part(proceed_idr::text, '.', 2)) != 4;
-- Expected: 0 rows
```

### CC-03 — List Endpoint: Sort + Filter + Cursor Pagination

1. Buka `/transaksi/penjualan?filter[status]=POSTED&sort=tanggal_eksekusi:desc`.
2. Pastikan hanya record POSTED tampil.
3. Pastikan sort tanggal descending.
4. Klik **Next** — cursor berubah di URL.
5. Bookmark URL → buka di tab baru → state ter-restore.

### CC-04 — Export CSV/XLSX (Semua Filter Direspek)

1. Filter `/transaksi/penjualan?filter[klasifikasi]=FVOCI`.
2. Klik **Export ▾ → CSV**.
3. Verifikasi CSV hanya berisi record klasifikasi=FVOCI.
4. Audit log `PENJUALAN.EXPORT` tersimpan.

### CC-05 — ROLE-AUDIT Read-Only

1. Login sebagai `internal.auditor`.
2. Verifikasi: tidak ada tombol Buat/Setujui/Tolak di `/transaksi/penjualan`.
3. Verifikasi: bisa akses `/transaksi/penjualan/{id}` untuk membaca.

---

## Sign-Off Matrix

| TC | Scenario | Actor | Pass/Fail | Tanda Tangan | Tanggal |
|---|---|---|---|---|---|
| TC-M8-01 | FVOCI PARTIAL preview correct | MAKER-TR | | | |
| TC-M8-02 | FVOCI Election no recycling note | MAKER-TR | | | |
| TC-M8-03 | Qty melebihi holding (negatif) | MAKER-TR | | | |
| TC-M8-04 | Instrumen MATURED (negatif) | MAKER-TR | | | |
| TC-M8-05 | Periode CLOSED (negatif) | MAKER-TR | | | |
| TC-M8-06 | Harga ≤ 0 (negatif) | MAKER-TR | | | |
| TC-M8-07 | Approve FVOCI PARTIAL one-tx | APPR-TR | | | |
| TC-M8-08 | SoD: maker tidak bisa approve | MAKER-TR | | | |
| TC-M8-09 | Reject ≥ 30 char | APPR-TR | | | |
| TC-M8-10 | Reject < 30 char (negatif) | APPR-TR | | | |
| TC-M8-11 | Idempotency double create | MAKER-TR | | | |
| TC-M8-12 | Idempotency double approve | APPR-TR | | | |
| TC-M8-13 | FVOCI FULL OCI recycled = total | APPR-TR | | | |
| TC-M8-14 | FVOCI PARTIAL OCI recycled proporsional | APPR-TR | | | |
| TC-M8-15 | FVOCI Election no REKLAS_OCI_PL + warn | APPR-TR | | | |
| TC-M8-16 | BM warn 5–10% → POSTED + risk flag | APPR-TR | | | |
| TC-M8-17 | BM block >10% → PENDING_BM_REVIEW | APPR-TR | | | |
| TC-M8-18 | BM check skip HTC&S | APPR-TR | | | |
| TC-M8-19 | FULL AC disposal → DISPOSED | APPR-TR | | | |
| TC-M8-20 | PARTIAL AC → ACTIVE + qty correct | APPR-TR | | | |
| CC-01 | Audit hash-chain | AUDIT | | | |
| CC-02 | No float64 money | IT-ADMIN | | | |
| CC-03 | List sort+filter+cursor | AUDIT | | | |
| CC-04 | Export CSV/XLSX | AUDIT | | | |
| CC-05 | ROLE-AUDIT read-only | AUDIT | | | |

**UAT Sign-Off**:

| Role | Nama | Tanda Tangan | Tanggal |
|---|---|---|---|
| Treasury Manager | | | |
| Finance Controller | | | |
| Risk Officer | | | |
| QA Lead | | | |
