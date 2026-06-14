# UAT Script — APP-C-005: EIR Drift Detection — Auto-Proposal Amandemen
**ID UAT**: UAT-APP-C-005-001
**Story**: APP-C-EIR-005..006 — M6 EIR Drift Detection Cron + Auto-Proposal
**Modul**: APP-C ECL Engine — Phase 4
**Tanggal**: 2026-06-13
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- Stack berjalan; migrasi 000032+ diterapkan (drift detection cron, eir_reestimation_log)
- Asynq scheduler aktif (`go run ./cmd/worker`)
- Periode buku `PBUKU-2026-06` berstatus `OPEN`

### Aktor
| Username | Role | Keterangan |
|----------|------|------------|
| `risk.officer.uat1` | ROLE-RISK | Reviewer auto-proposal |
| `alco.member.uat1` | ROLE-ALCO | Approver (jika proposal disetujui) |

### Terminologi
- **EIR Drift**: Selisih antara EIR yang terdaftar di jadwal amortisasi vs EIR yang dihitung ulang dari cashflow aktual terkini.
- **Threshold Drift**: Parameter konfigurasi — default `0.00005000` (0.005%) berdasarkan DEC dan SoW §4.
- **Auto-Proposal**: Sistem otomatis membuat draft amandemen jika drift melampaui threshold.

### Seed Data — Instrumen dengan Cashflow yang Sudah Berubah

```sql
-- Instrumen dengan EIR terdaftar 6.5% tapi cashflow aktual menunjukkan 7.1% (drift 0.6%)
INSERT INTO mst.instrumen (id, kode_instrumen, nama_instrumen, tipe_instrumen,
  klasifikasi_psak71, nominal, mata_uang_kode, tanggal_penempatan, tanggal_jatuh_tempo,
  status, stage, created_by, updated_by)
VALUES (
  'k1000000-0000-0000-0000-000000000001',
  'OBL-DRIFT-UAT-001',
  'Obligasi Drift Detection (UAT)',
  'OBLIGASI', 'AC', 8000000000.0000, 'IDR',
  '2023-01-01', '2030-12-31',
  'AKTIF', 1,
  'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO NOTHING;

-- EIR terdaftar: 6.5%
INSERT INTO ecl.amortisasi_schedule (id, instrumen_id, schedule_version,
  tanggal_periode, eir_persen, effective_from, effective_to, created_by, updated_by)
VALUES (
  gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', 1,
  '2030-12-31', 0.06500000, '2023-01-01', 'infinity',
  'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'
) ON CONFLICT DO NOTHING;

-- Cashflow aktual terbaru: kupon berubah de facto menjadi 7.1% (via pembayaran aktual)
-- Ini disimulasikan via tabel cashflow_aktual atau market_data
INSERT INTO ecl.cashflow_aktual (id, instrumen_id, tanggal_cashflow, jumlah_idr,
  tipe_cashflow, created_by, updated_by)
VALUES
  (gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', '2026-12-31', 284000000.0000, 'KUPON',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', '2027-12-31', 284000000.0000, 'KUPON',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', '2028-12-31', 284000000.0000, 'KUPON',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', '2029-12-31', 284000000.0000, 'KUPON',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'k1000000-0000-0000-0000-000000000001', '2030-12-31', 8284000000.0000, 'POKOK_DAN_KUPON',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;
-- EIR aktual dari cashflow ini ≈ 7.1% → drift = 7.1% - 6.5% = 0.6% >> 0.005% threshold
```

---

## Referensi Numerik Drift Detection

```
EIR Terdaftar (version 1): 0.06500000 (6.5%)

EIR Aktual dari cashflow:
  Initial CF: -8.000.000.000 (nilai buku instrumen)
  Future CFs: [284M, 284M, 284M, 284M, 8284M] (annual)

  Newton-Raphson solve:
    r_initial = 0.1 (seed)
    tolerance = 1e-10
    max_iter  = 100
    r_converge ≈ 0.07100000 (7.1%)

Drift = |EIR_aktual - EIR_terdaftar|
      = |0.071000000 - 0.065000000|
      = 0.006000000 (0.6%)

Threshold = 0.000050000 (0.005%)
Drift > Threshold → AUTO-PROPOSAL dibuat
```

---

## Skenario UAT

### TC-001: Konfigurasi Threshold Drift (Admin)

**Aktor**: `risk.officer.uat1` (ROLE-RISK — konfigurasi)

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **APP-C → Konfigurasi → EIR Drift Detection**.
3. Verifikasi threshold aktif:
   - **Threshold Drift EIR**: `0.005%` (0.00005000)
   - **Jadwal Cron**: `0 1 * * 1` (setiap Senin 01:00 WIB)
   - **Scope**: Semua instrumen AC aktif
4. Jangan ubah — verifikasi bahwa nilai sesuai konfigurasi default.

**Hasil yang Diharapkan**:
- Halaman konfigurasi menampilkan threshold `0.00005000`.
- Log run terakhir tersedia: tanggal, jumlah instrumen yang diperiksa, jumlah yang flagged.

---

### TC-002: Trigger Drift Detection Manual (untuk UAT, tanpa menunggu cron)

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Navigasi ke **APP-C → EIR Drift Detection → Jalankan Manual**.
2. Isi form:
   - **Scope**: `OBL-DRIFT-UAT-001` (satu instrumen saja untuk UAT)
   - **Per**: Tanggal assessment `2026-06-13`
3. Klik **Jalankan Sekarang**.
4. Progress panel muncul:
   - "Memuat instrumen... → Menghitung EIR aktual via Newton-Raphson... → Membandingkan dengan EIR terdaftar... → Selesai."
5. Setelah selesai, toast hijau:
   `"Drift detection selesai. 1 instrumen diperiksa. 1 instrumen flagged (drift > threshold). Auto-proposal dibuat: AMD-DRIFT-001."`

**Hasil yang Diharapkan**:
- `ecl.eir_reestimation_log` mencatat row baru:
  - `instrumen_id = 'k1000000-0000-0000-0000-000000000001'`
  - `eir_lama = 0.06500000`
  - `eir_baru = 0.07100000` (±0.0000001 dari Newton-Raphson)
  - `delta_eir = 0.00600000`
  - `threshold = 0.00005000`
  - `is_flagged = TRUE`
  - `auto_proposal_id = 'AMD-DRIFT-001'`

---

### TC-003: Review Auto-Proposal oleh Risk Officer

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Navigasi ke **APP-C → EIR → Antrian Auto-Proposal Amandemen**.
2. Buka AMD-DRIFT-001.
3. Halaman menampilkan:
   - **Instrumen**: OBL-DRIFT-UAT-001
   - **EIR Saat Ini**: 6.500000%
   - **EIR Baru Diusulkan**: 7.100000%
   - **Drift**: 0.600000% (0.60000000)
   - **Status Proposal**: `PENDING_REVIEW`
   - **Dibuat oleh**: `SYSTEM (Drift Detection Cron)`
4. Tinjau detail cashflow yang digunakan untuk menghitung EIR baru.
5. Klik **Setujui Proposal (Lanjutkan ke Amandemen)** ATAU **Tolak Proposal (Drift Diabaikan)**.
6. Untuk skenario positif: klik **Setujui Proposal**.
7. Isi **Catatan Review**: `"Drift 0.6% terverifikasi. Perubahan cashflow aktual dikonfirmasi. Proposal dilanjutkan ke approval ALCO."`
8. Klik **Konfirmasi**.
   - Toast hijau: `"Proposal AMD-DRIFT-001 disetujui. Status: REVIEWED. Menunggu approval ALCO."`

**Hasil yang Diharapkan**:
- Proposal status: `PENDING_REVIEW` → `REVIEWED`.
- Audit log: `EIR_PROPOSAL.REVIEW`.

---

### TC-004: Skenario Negatif — Tolak Proposal Drift

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Pada proposal lain (buat instrumen kedua dengan drift kecil di bawah threshold):
   - Seed: `EIR_terdaftar = 6.5%`, `EIR_aktual = 6.502%` → drift = 0.002% < 0.005% threshold
   - Instrumen ini TIDAK boleh menghasilkan auto-proposal (drift < threshold).
2. Verifikasi: tidak ada `AMD-DRIFT-002` yang dibuat untuk instrumen ini.

**Verifikasi DB**:
```sql
SELECT COUNT(*) FROM ecl.eir_reestimation_log
WHERE instrumen_id = 'k2000000-0000-0000-0000-000000000001'
  AND is_flagged = TRUE;
-- Expected: 0 (drift di bawah threshold, tidak di-flag)
```

---

### TC-005: Approval ALCO & Aktivasi Jadwal Baru

**Aktor**: `alco.member.uat1` (ROLE-ALCO — MFA wajib)

**Langkah-langkah**:

1. Login sebagai `alco.member.uat1`.
2. Navigasi ke **APP-C → EIR → Antrian Approval ALCO**.
3. Buka AMD-DRIFT-001.
4. Klik **Approve**.
5. MFA Step-Up → masukkan kode TOTP.
6. Isi **Catatan**: `"EIR drift terverifikasi. Disetujui. Jadwal versi 2 aktif mulai 2026-06-13."`
7. Klik **Konfirmasi**.
   - Toast hijau: `"AMD-DRIFT-001 di-approve. Jadwal EIR baru OBL-DRIFT-UAT-001 aktif."`
8. Status: `REVIEWED` → `APPROVED`.

**Verifikasi Versioning**:
```sql
SELECT schedule_version, eir_persen, effective_from, effective_to
FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001'
ORDER BY schedule_version;
-- Expected:
-- version 1: eir=0.06500000, from=2023-01-01, to=2026-06-12  (effective_to diupdate)
-- version 2: eir=0.07100000, from=2026-06-13, to=infinity      (baru)
```

---

### TC-006: Verifikasi SoD di Drift Approval

**Tujuan**: Risk Officer yang menjalankan drift detection (system maker) tidak bisa approve seal amandemen.

**Langkah-langkah**:

1. Coba `risk.officer.uat1` approve AMD-DRIFT-001 via API langsung:
```bash
curl -X POST http://localhost:8080/api/v1/eir/amendments/AMD-DRIFT-001/approve \
  -H "Authorization: Bearer $RISK_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"comment": "SoD bypass attempt", "signature_method": "JWT_STEP_UP"}'
```

**Hasil yang Diharapkan**:
- HTTP 403 `{ "error": { "code": "SOD_VIOLATION", "message": "..." } }`
- `risk.officer.uat1` bukan ROLE-ALCO → forbidden pada endpoint ini.

---

### TC-007: View Drift Detection History (DataTable)

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Navigasi ke **APP-C → EIR → Drift Detection History**.
2. DataTable menampilkan semua run:
   - Kolom: Tanggal Run, Instrumen, EIR Lama, EIR Baru, Drift (%), Flagged, Proposal ID, Status Proposal.
3. Filter: `Flagged = YES` → hanya tampil OBL-DRIFT-UAT-001.
4. Sort: klik header `Drift (%)` → urutan desc.
5. Export CSV → download file dengan data yang terfilter.

**Hasil yang Diharapkan**:
- Sort, filter, export berfungsi (UX §1).
- Data akurat sesuai `ecl.eir_reestimation_log`.

---

### TC-008: Audit Hash Chain Verification

**Tujuan**: Memverifikasi bahwa audit trail tidak bisa dimanipulasi.

**Langkah-langkah**:

1. Setelah semua UAT selesai, jalankan:
```bash
go run /home/tugure/projects/ifrs9ai/backend/cmd/audit-verify/main.go \
  --entity-type ecl.eir_reestimation_log \
  --range "2026-06-01:2026-06-30"
```

**Hasil yang Diharapkan**:
- Output: `"Hash chain OK — 0 violations found for 2026-06 ecl.eir_reestimation_log"`
- Tidak ada pelanggaran integritas.

---

## Checklist Audit Pasca UAT

```sql
-- 1. Drift log entry terisi lengkap
SELECT instrumen_id, eir_lama, eir_baru, delta_eir, threshold, is_flagged, auto_proposal_id
FROM ecl.eir_reestimation_log
WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001'
ORDER BY run_at DESC LIMIT 1;
-- Expected: 1 row dengan semua field terisi, is_flagged=TRUE, auto_proposal_id='AMD-DRIFT-001'

-- 2. Tidak ada UPDATE pada schedule versi 1 (immutability DEC-013)
SELECT COUNT(*) FROM aud.audit_log
WHERE entity_type = 'ecl.amortisasi_schedule'
  AND action = 'AMORTISASI_SCHEDULE.UPDATE'
  AND after_jsonb->>'instrumen_id' = 'k1000000-0000-0000-0000-000000000001'
  AND after_jsonb->>'schedule_version' = '1';
-- Expected: 0

-- 3. 4-eyes SoD terpenuhi di tabel approval
SELECT maker_id, reviewer_id, approver_id
FROM ecl.eir_amendment_approval
WHERE amendment_id = 'AMD-DRIFT-001';
-- Expected: 3 user berbeda (SYSTEM != reviewer != approver)
```

---

## Rollback / Cleanup

```sql
BEGIN;
DELETE FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001'
  AND schedule_version = 2;
UPDATE ecl.amortisasi_schedule
SET effective_to = 'infinity'
WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001'
  AND schedule_version = 1;
DELETE FROM ecl.eir_reestimation_log WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001';
DELETE FROM ecl.cashflow_aktual WHERE instrumen_id = 'k1000000-0000-0000-0000-000000000001';
DELETE FROM mst.instrumen WHERE kode_instrumen = 'OBL-DRIFT-UAT-001';
ROLLBACK; -- ubah ke COMMIT setelah verifikasi
```

---

## Sign-off UAT

| Nama | Jabatan | Tanda Tangan | Tanggal |
|------|---------|--------------|---------|
| | Risk Officer (Reviewer auto-proposal) | | |
| | ALCO Member (Approver amandemen) | | |
| | QA Engineer | | |
| | IFRS9 Compliance Reviewer | | |
