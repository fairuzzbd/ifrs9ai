# UAT Script — APP-C-002: ECL Calc Run Lifecycle (Submit → Review → Seal)
**ID UAT**: UAT-APP-C-002-001
**Story**: APP-C-ECL-004..007 — ECL Calc Run Full Lifecycle
**Modul**: APP-C ECL Engine — Phase 4
**Tanggal**: 2026-06-13
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- Stack berjalan: `docker compose -f deploy/docker-compose.dev.yml up -d`
- Backend API: `http://localhost:8080`, Frontend: `http://localhost:3001`
- Migrasi 000028+ diterapkan (calcrun service)
- Periode buku `PBUKU-2026-06` berstatus `OPEN`
- Parameter ECL aktif (PD, LGD, bobot skenario 0.25/0.50/0.25) sudah diapprove ALCO
- MFA terdaftar untuk aktor ALCO dan RISK

### Aktor
| Username | Role | MFA |
|----------|------|-----|
| `risk.officer.uat1` | ROLE-RISK | tidak wajib |
| `alco.member.uat1` | ROLE-ALCO | WAJIB |
| `risk.officer.uat2` | ROLE-RISK | tidak wajib (reviewer) |

### Seed Data

```sql
-- 3 instrumen AC yang cukup untuk validasi formula
INSERT INTO mst.instrumen (id, kode_instrumen, nama_instrumen, tipe_instrumen,
  klasifikasi_psak71, stage, ead_idr, pd_12m, pd_lifetime, lgd,
  mata_uang_kode, created_by, updated_by)
VALUES
  -- Stage 1 instrumen
  ('f1000000-0000-0000-0000-000000000001',
   'DEP-UAT-001', 'Deposito AC Stage1 (UAT)', 'DEPOSITO', 'AC', 1,
   10000000000.0000,  -- EAD IDR 10B
   0.00250000,        -- PD 12M 0.25%
   0.01200000,        -- PD Lifetime
   0.45000000,        -- LGD 45%
   'IDR',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  -- Stage 2 instrumen
  ('f1000000-0000-0000-0000-000000000002',
   'OBL-UAT-002', 'Obligasi AC Stage2 (UAT)', 'OBLIGASI', 'AC', 2,
   5000000000.0000,   -- EAD IDR 5B
   0.05000000,        -- PD 12M (unused, use Lifetime for Stage 2)
   0.12000000,        -- PD Lifetime 12%
   0.60000000,        -- LGD 60%
   'IDR',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  -- Stage 3 instrumen
  ('f1000000-0000-0000-0000-000000000003',
   'OBL-UAT-003', 'Obligasi AC Stage3 (UAT)', 'OBLIGASI', 'AC', 3,
   2000000000.0000,   -- EAD IDR 2B
   1.00000000,        -- PD = 1.0 (Stage 3)
   1.00000000,
   0.70000000,        -- LGD 70%
   'IDR',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001')
ON CONFLICT (kode_instrumen) DO NOTHING;

-- Forward-looking multiplier aktif (Good/Normal/Bad per periode)
INSERT INTO ecl.fl_multiplier_param (id, periode_id, skenario, impact_pd_multiplier, bobot, is_active, created_by, updated_by)
VALUES
  (gen_random_uuid(), 'PBUKU-2026-06', 'GOOD',   0.80000000, 0.25000000, TRUE,
   'b1000000-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000002'),
  (gen_random_uuid(), 'PBUKU-2026-06', 'NORMAL', 1.00000000, 0.50000000, TRUE,
   'b1000000-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000002'),
  (gen_random_uuid(), 'PBUKU-2026-06', 'BAD',    1.30000000, 0.25000000, TRUE,
   'b1000000-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000002')
ON CONFLICT DO NOTHING;
```

---

## Kalkulasi Referensi (Verifikasi Numerik)

Formula (DEC-010):
```
ECL_skenario = EAD × PD × LGD
ECL_FL       = ECL_skenario × impact_pd_multiplier
ECL_weighted = Σ (ECL_FL × bobot)
```

**DEP-UAT-001 (Stage 1 — PD 12M):**
```
ECL_Good   = 10B × 0.0025 × 0.45 × 0.80 = 9.000.000,0000
ECL_Normal = 10B × 0.0025 × 0.45 × 1.00 = 11.250.000,0000
ECL_Bad    = 10B × 0.0025 × 0.45 × 1.30 = 14.625.000,0000

ECL_weighted = (9.000.000 × 0.25) + (11.250.000 × 0.50) + (14.625.000 × 0.25)
             = 2.250.000 + 5.625.000 + 3.656.250
             = 11.531.250,0000 IDR
```

**OBL-UAT-002 (Stage 2 — PD Lifetime):**
```
ECL_Good   = 5B × 0.12 × 0.60 × 0.80 = 288.000.000,0000
ECL_Normal = 5B × 0.12 × 0.60 × 1.00 = 360.000.000,0000
ECL_Bad    = 5B × 0.12 × 0.60 × 1.30 = 468.000.000,0000

ECL_weighted = (288M × 0.25) + (360M × 0.50) + (468M × 0.25)
             = 72M + 180M + 117M
             = 369.000.000,0000 IDR
```

**OBL-UAT-003 (Stage 3 — PD = 1.0):**
```
ECL_Good   = 2B × 1.0 × 0.70 × 0.80 = 1.120.000.000,0000
ECL_Normal = 2B × 1.0 × 0.70 × 1.00 = 1.400.000.000,0000
ECL_Bad    = 2B × 1.0 × 0.70 × 1.30 = 1.820.000.000,0000

ECL_weighted = (1.120B × 0.25) + (1.400B × 0.50) + (1.820B × 0.25)
             = 280M + 700M + 455M
             = 1.435.000.000,0000 IDR

-- Bunga Stage 3 dihitung atas NET CARRYING:
-- Net Carrying = Gross Carrying - ECL = 2B - 1.435B = 565.000.000,0000 IDR
```

---

## Skenario UAT

### TC-001: Membuat Calc Run Baru (Maker = Risk Officer)

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **ECL Engine → Calc Run → Buat Baru**.
3. Isi form:
   - **Periode Buku**: `PBUKU-2026-06 (Juni 2026)`
   - **Deskripsi**: `UAT Calc Run - Juni 2026`
   - **Tipe**: `REGULAR`
4. Klik **Simpan**.
   - Toast hijau: `"Calc run CR-2026-06-UAT berhasil dibuat. Status: DRAFT."`
5. Sistem menampilkan halaman detail calc run dengan:
   - Status: `DRAFT`
   - Periode: `2026-06`
   - Dibuat oleh: `risk.officer.uat1`
   - Tombol: **Mulai Komputasi** (aktif)

**Hasil yang Diharapkan**:
- Calc run tersimpan dengan status `DRAFT`.
- Audit log: `ECL_CALC_RUN.CREATE`.

---

### TC-002: Memulai Komputasi (Async Job)

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Pada halaman detail calc run, klik **Mulai Komputasi**.
2. Sistem menampilkan **Konfirmasi Dialog**: "Mulai komputasi ECL untuk periode Juni 2026 terhadap semua instrumen aktif?"
3. Klik **Konfirmasi**.
4. Toast biru muncul: `"Komputasi ECL dimulai. Job ID: job_XYZ. Status real-time tampil di bawah."`
5. Komponen `<JobProgressPanel>` muncul di bawah form:
   - Progress bar bergerak dari 0% → 100%.
   - Step text berubah: "Memuat instrumen... → Menghitung Stage 1... → Menghitung Stage 2... → Selesai."
6. Setelah selesai: toast hijau: `"Komputasi ECL selesai. 3 instrumen dihitung. Lihat hasil →"`

**Hasil yang Diharapkan**:
- Endpoint `POST /api/v1/ecl/calc-runs/{id}/start` mengembalikan `202 Accepted { jobId }`.
- Job progress tersimpan di `sys.job` (bukan hanya Redis).
- Status calc run berubah: `DRAFT` → `COMPUTED`.

**Anti-pattern yang diverifikasi**:
- TIDAK ada spinner tanpa progress (aturan UX §3).
- TIDAK ada timeout browser (async pattern pakai SSE).

---

### TC-003: Verifikasi Hasil Numerik

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Setelah komputasi selesai, klik **Lihat Hasil**.
2. DataTable hasil tampil dengan kolom: Kode Instrumen, Stage, EAD IDR, PD, LGD, ECL Good, ECL Normal, ECL Bad, ECL Weighted.
3. Filter: tampilkan baris DEP-UAT-001.

**Verifikasi numerik wajib** (toleransi: ±0 untuk 4 desimal, banker's rounding):

| Field | Nilai di UI | Nilai Referensi |
|-------|-------------|-----------------|
| DEP-UAT-001 ECL Weighted | 11.531.250,0000 | 11.531.250,0000 |
| OBL-UAT-002 ECL Weighted | 369.000.000,0000 | 369.000.000,0000 |
| OBL-UAT-003 ECL Weighted | 1.435.000.000,0000 | 1.435.000.000,0000 |
| OBL-UAT-003 Net Carrying | 565.000.000,0000 | 565.000.000,0000 |
| Total ECL Portfolio | 1.815.531.250,0000 | 1.815.531.250,0000 |

4. Export CSV: klik **Export ▾ → CSV** → verifikasi angka sama.

---

### TC-004: Submit untuk Review

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Klik **Submit untuk Review**.
2. Dialog: "Submit calc run ke reviewer?"
3. Isi: **Catatan Maker**: `"Kalkulasi ECL Juni 2026 siap direview."`
4. Klik **Submit**.
   - Toast hijau: `"Calc run berhasil di-submit. Menunggu review oleh Risk Officer."`
5. Status berubah: `COMPUTED` → `PENDING_REVIEW`.

---

### TC-005: Review oleh Risk Officer Kedua (SoD)

**Aktor**: `risk.officer.uat2` (ROLE-RISK — bukan `risk.officer.uat1`)

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat2`.
2. Navigasi ke **ECL Engine → Calc Run → Antrian Review**.
3. Buka calc run CR-2026-06-UAT.
4. Verifikasi angka hasil (sama seperti TC-003).
5. Klik **Approve Review**.
6. Isi: **Catatan Reviewer**: `"Angka ECL sesuai formula PSAK 71. Siap di-approve untuk seal."`
7. Klik **Konfirmasi Review**.
   - Toast hijau: `"Review berhasil. Calc run menunggu approval seal oleh ALCO."`
8. Status berubah: `PENDING_REVIEW` → `REVIEWED`.

**Uji SoD Negatif** (wajib dilakukan):
- Coba login sebagai `risk.officer.uat1` (maker).
- Coba akses endpoint `POST /api/v1/ecl/calc-runs/{id}/review` langsung via curl.
- **Expected**: HTTP 403 `{ "error": { "code": "SOD_VIOLATION", "message": "Maker tidak bisa menjadi reviewer calc run yang sama." } }`

---

### TC-006: Seal oleh ALCO (MFA Step-Up)

**Aktor**: `alco.member.uat1` (ROLE-ALCO — MFA WAJIB)

**Langkah-langkah**:

1. Login sebagai `alco.member.uat1`.
2. Navigasi ke **ECL Engine → Calc Run → Antrian Approval Seal**.
3. Buka calc run CR-2026-06-UAT.
4. Verifikasi summary: Total ECL = 1.815.531.250,0000 IDR.
5. Klik **Approve Seal**.
6. Sistem meminta **MFA Step-Up**: "Masukkan kode TOTP untuk konfirmasi seal."
7. Masukkan kode TOTP yang valid.
8. Isi: **Catatan Approver**: `"Sesuai PSAK 71. ECL Juni 2026 diseal. Efektif per 2026-06-30."`
9. Klik **Seal Definitif**.
   - Toast hijau: `"Calc run CR-2026-06-UAT berhasil diseal. Immutable mulai sekarang."`
10. Status berubah: `REVIEWED` → `SEALED`.

**Verifikasi Seal Immutable**:
- Coba klik **Edit Parameter** pada calc run yang sudah SEALED.
- **Expected**: tombol dinonaktifkan / HTTP 423 `{ "error": { "code": "ECL_PARAM_FROZEN" } }`

**Uji SoD Seal** (wajib):
- Coba seal oleh `risk.officer.uat2` (reviewer) via API langsung.
- **Expected**: HTTP 403 `{ "error": { "code": "SOD_VIOLATION" } }`

---

### TC-007: Reprodusibilitas — Same Snapshot, Same Result

**Tujuan**: Membuktikan DEC-010 — snapshot yang sama menghasilkan ECL identik sampai desimal terakhir.

**Langkah-langkah**:

1. Buat calc run baru CR-2026-06-UAT-REPRO untuk periode yang sama (June 2026).
2. Gunakan `snapshot_id` yang sama dengan CR-2026-06-UAT (tidak ada perubahan instrumen/parameter).
3. Jalankan komputasi.
4. Bandingkan hasil row by row.

**Hasil yang Diharapkan**:
- Setiap baris result line identik dengan run pertama (hingga digit ke-4 desimal).
- Perbedaan = 0.0000 di semua kolom ECL.
- Diff report di UI: "0 perbedaan ditemukan antara run CR-2026-06-UAT dan CR-2026-06-UAT-REPRO."

**Verifikasi DB**:
```sql
SELECT r1.instrumen_id,
       r1.ecl_weighted_idr AS ecl_run1,
       r2.ecl_weighted_idr AS ecl_run2,
       r1.ecl_weighted_idr - r2.ecl_weighted_idr AS delta
FROM ecl.ecl_calc_result_line r1
JOIN ecl.ecl_calc_result_line r2 ON r1.instrumen_id = r2.instrumen_id
WHERE r1.calc_run_id = 'CR-2026-06-UAT'
  AND r2.calc_run_id = 'CR-2026-06-UAT-REPRO'
HAVING r1.ecl_weighted_idr - r2.ecl_weighted_idr <> 0;
-- Expected: 0 rows
```

---

## Checklist Audit Pasca UAT

```sql
-- 1. Setiap state transition tercatat di audit_log
SELECT action, COUNT(*) FROM aud.audit_log
WHERE entity_type = 'ecl.ecl_calc_run'
  AND action IN ('ECL_CALC_RUN.CREATE','ECL_CALC_RUN.START','ECL_CALC_RUN.REVIEW','ECL_CALC_RUN.SEAL')
GROUP BY action;
-- Expected: masing-masing ≥ 1

-- 2. Seal record ada signature
SELECT seal_requested_by, sealed_by, signature_hash, signed_at
FROM ecl.ecl_calc_run_seal WHERE calc_run_id = 'CR-2026-06-UAT';
-- Expected: 1 row dengan signature_hash tidak null

-- 3. Result line tidak bisa diedit setelah seal
SELECT is_editable FROM ecl.ecl_calc_run WHERE kode = 'CR-2026-06-UAT';
-- Expected: FALSE (or via CHECK constraint)
```

---

## Rollback / Cleanup

```sql
BEGIN;
DELETE FROM ecl.ecl_calc_result_line WHERE calc_run_id IN (
  SELECT id FROM ecl.ecl_calc_run WHERE periode_id = 'PBUKU-2026-06' AND kode LIKE '%UAT%'
);
DELETE FROM ecl.ecl_calc_run WHERE periode_id = 'PBUKU-2026-06' AND kode LIKE '%UAT%';
ROLLBACK; -- ubah ke COMMIT setelah verifikasi
```

---

## Sign-off UAT

| Nama | Jabatan | Tanda Tangan | Tanggal |
|------|---------|--------------|---------|
| | Risk Officer (Maker) | | |
| | Risk Officer (Reviewer) | | |
| | ALCO Member (Approver Seal) | | |
| | QA Engineer | | |
| | IFRS9 Compliance Reviewer | | |
