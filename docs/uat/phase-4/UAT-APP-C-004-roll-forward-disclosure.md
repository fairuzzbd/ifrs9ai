# UAT Script — APP-C-004: Roll-Forward CKPN — Disclosure & Rekonsiliasi
**ID UAT**: UAT-APP-C-004-001
**Story**: APP-C-RFW-001..003 — Roll-Forward CKPN Disclosure
**Modul**: APP-C ECL Engine — Phase 4
**Tanggal**: 2026-06-13
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- Stack berjalan; migrasi 000030+ diterapkan (roll-forward module)
- Periode buku `PBUKU-2026-05` (Mei) berstatus `CLOSED` — opening period
- Periode buku `PBUKU-2026-06` (Juni) berstatus `OPEN` — current period
- Calc run SEALED untuk kedua periode tersebut:
  - CR-2026-05: SEALED (opening ECL)
  - CR-2026-06: SEALED (closing ECL) — buat via TC-005 UAT-APP-C-002 atau seed manual

### Aktor
| Username | Role | Keterangan |
|----------|------|------------|
| `akuntansi.uat1` | ROLE-AKUN | Membuat draft roll-forward |
| `akun.ctl.uat1` | ROLE-AKUN-CTL | Finance Controller — review |
| `cfo.uat1` | ROLE-CFO | Approval final (MFA) |

### Seed Data ECL Opening & Closing

```sql
-- ECL hasil setiap instrumen untuk Mei 2026 (Opening) dan Juni 2026 (Closing)
-- Disederhanakan untuk UAT — 4 instrumen dengan berbagai movement bucket

-- CR-2026-05 (Opening)
-- Instrumen A: Stage 1, ECL 11.531.250 (tidak berubah dari UAT-APP-C-002)
-- Instrumen B: Stage 1, ECL 5.000.000 (akan naik stage ke Stage 2)
-- Instrumen C: BARU di periode Juni (origination)
-- Instrumen D: Stage 2, ECL 100.000.000 (akan lunas / derecognition)

-- Lihat data aktual dari UAT-APP-C-002. Untuk UAT mandiri, seed:
INSERT INTO ecl.ecl_calc_result_line (id, calc_run_id, instrumen_id, stage,
  ead_idr, ecl_weighted_idr, created_by, updated_by)
VALUES
  (gen_random_uuid(), 'CR-2026-05', 'f1000000-0000-0000-0000-000000000001', 1,
   10000000000.0000, 11531250.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'CR-2026-05', 'h1000000-0000-0000-0000-000000000001', 1,
   5000000000.0000, 5000000.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'CR-2026-05', 'h1000000-0000-0000-0000-000000000002', 2,
   2000000000.0000, 100000000.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- CR-2026-06 (Closing)
INSERT INTO ecl.ecl_calc_result_line (id, calc_run_id, instrumen_id, stage,
  ead_idr, ecl_weighted_idr, created_by, updated_by)
VALUES
  -- Instrumen A: tetap Stage 1, ECL sedikit naik (remeasurement)
  (gen_random_uuid(), 'CR-2026-06', 'f1000000-0000-0000-0000-000000000001', 1,
   10000000000.0000, 12000000.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  -- Instrumen B: SICR → Stage 2 (transfer out from Stage 1 to Stage 2)
  (gen_random_uuid(), 'CR-2026-06', 'h1000000-0000-0000-0000-000000000001', 2,
   5000000000.0000, 150000000.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001'),
  -- Instrumen C: BARU (origination di periode Juni)
  (gen_random_uuid(), 'CR-2026-06', 'h1000000-0000-0000-0000-000000000003', 1,
   3000000000.0000, 7500000.0000,
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001')
  -- Instrumen D: TIDAK ada di closing → derecognition
ON CONFLICT DO NOTHING;
```

---

## Referensi Numerik Roll-Forward

```
Opening ECL (Mei 2026):
  Instrumen A (Stage 1):  11.531.250,0000
  Instrumen B (Stage 1):   5.000.000,0000
  Instrumen D (Stage 2): 100.000.000,0000
  TOTAL OPENING          116.531.250,0000

Movements (Juni 2026):
  + Originations (Instrumen C, Stage 1):         +7.500.000,0000
  - Derecognitions (Instrumen D, ECL terhapus): -100.000.000,0000
  + Transfers Stage 1→2 (Instrumen B):
      Prior ECL Instrumen B Stage 1:              -5.000.000,0000  (keluar dari Stage 1)
      Current ECL Instrumen B Stage 2:           +150.000.000,0000 (masuk ke Stage 2)
      Net Transfer impact:                       +145.000.000,0000
  ± Remeasurements:
      Instrumen A: 12.000.000 - 11.531.250 =        +468.750,0000  (same-stage ECL movement)

  Closing ECL (Juni 2026):
    Instrumen A (Stage 1):  12.000.000,0000
    Instrumen B (Stage 2): 150.000.000,0000
    Instrumen C (Stage 1):   7.500.000,0000
    TOTAL CLOSING           169.500.000,0000

Rekonsiliasi:
  Opening                              116.531.250,0000
  + Originations                         7.500.000,0000
  - Derecognitions                    -100.000.000,0000
  + Transfer Stage1→Stage2 (net)       145.000.000,0000
  + Remeasurements                         468.750,0000
  ─────────────────────────────────────────────────────
  = Closing Calculated                 169.500.000,0000
  Closing Actual (from CR-2026-06)     169.500.000,0000
  Delta                                          0,0000  ✓ (tolerance ≤ IDR 1,0000)
```

---

## Skenario UAT

### TC-001: Generate Draft Roll-Forward Report

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Login sebagai `akuntansi.uat1`.
2. Navigasi ke **APP-C → Roll-Forward CKPN → Buat Roll-Forward**.
3. Isi form:
   - **Periode Opening**: `PBUKU-2026-05 (Mei 2026)`
   - **Periode Closing**: `PBUKU-2026-06 (Juni 2026)`
   - **Calc Run Opening**: `CR-2026-05`
   - **Calc Run Closing**: `CR-2026-06`
4. Klik **Generate Roll-Forward**.
   - Progress panel muncul: "Menghitung roll-forward... Memproses transfers... Memproses lifecycle..."
5. Setelah selesai, toast hijau: `"Roll-forward RFW-2026-06 berhasil dibuat. Status rekonsiliasi: RECONCILED."`

**Hasil yang Diharapkan**:
- Roll-forward dibuat dengan `reconcile_status = RECONCILED`.
- Delta rekonsiliasi = `0,0000` (zero variance).
- Halaman summary menampilkan tabel:

| Bucket | Nilai IDR |
|--------|-----------|
| ECL Opening | 116.531.250,0000 |
| Originations | 7.500.000,0000 |
| Derecognitions | -100.000.000,0000 |
| Transfer Stage1→Stage2 | 145.000.000,0000 |
| Remeasurements | 468.750,0000 |
| ECL Closing (Calculated) | 169.500.000,0000 |
| ECL Closing (Actual) | 169.500.000,0000 |
| **Variance** | **0,0000** |

---

### TC-002: Drill-Down per Instrumen

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Pada halaman roll-forward RFW-2026-06, klik **Lihat Detail per Instrumen**.
2. DataTable menampilkan: Kode, Stage Opening, Stage Closing, ECL Opening, ECL Closing, Bucket Movement, Nilai Movement.
3. Filter: `Bucket = TRANSFER` → tampil hanya Instrumen B (Stage1→Stage2).
4. Klik row Instrumen B → detail breakdown:
   - Stage Opening: 1, ECL Opening: 5.000.000,0000
   - Stage Closing: 2, ECL Closing: 150.000.000,0000
   - Bucket: TRANSFER_TO_STAGE_2
   - Movement: +145.000.000,0000

**Hasil yang Diharapkan**:
- DataTable sort/filter berfungsi (UX §1).
- Drill-down menampilkan stage history yang menjadi trigger transfer.

---

### TC-003: Export Disclosure XLSX

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Pada halaman RFW-2026-06, klik **Export ▾ → XLSX**.
2. Pilih template: `PSAK 71 Disclosure Format`.
3. Klik **Export**.
   - Untuk UAT ini (<10k row), export sync: file langsung terdownload.
4. Buka file `roll-forward-CKPN-2026-06-YYYYMMDD.xlsx`.

**Verifikasi XLSX**:
- Sheet 1 "Summary": tabel 5 bucket + Opening + Closing + Variance.
- Sheet 2 "Detail": satu row per instrumen dengan semua kolom.
- Format angka: `#,##0.0000` (4 desimal, ribuan dengan titik).
- Baris footer: timestamp export + filter aktif.

---

### TC-004: Submit Roll-Forward untuk Review

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Klik **Submit untuk Review**.
2. Isi **Catatan Maker**: `"Roll-forward Juni 2026 siap direview. Variance = 0. Semua transfer sudah diverifikasi."`
3. Klik **Submit**.
   - Toast hijau: `"Roll-forward RFW-2026-06 berhasil di-submit. Menunggu review Finance Controller."`
4. Status: `DRAFT` → `PENDING_REVIEW`.

---

### TC-005: Review Finance Controller

**Aktor**: `akun.ctl.uat1` (ROLE-AKUN-CTL)

**Langkah-langkah**:

1. Login sebagai `akun.ctl.uat1`.
2. Navigasi ke **APP-C → Roll-Forward CKPN → Antrian Review**.
3. Buka RFW-2026-06.
4. Verifikasi angka (tabel referensi TC-001).
5. Klik **Approve Review**.
6. Isi **Catatan Reviewer**: `"Rekonsiliasi 0 variance. Transfers Stage1→Stage2 sesuai staging history. OK."`
7. Klik **Konfirmasi**.
   - Toast hijau: `"Review berhasil. Menunggu approval CFO."`
8. Status: `PENDING_REVIEW` → `REVIEWED`.

---

### TC-006: Approval Final CFO (MFA)

**Aktor**: `cfo.uat1` (ROLE-CFO — MFA wajib)

**Langkah-langkah**:

1. Login sebagai `cfo.uat1`.
2. Navigasi ke **APP-C → Roll-Forward CKPN → Antrian Approval CFO**.
3. Buka RFW-2026-06.
4. Verifikasi summary: Opening 116.5M, Closing 169.5M, Variance 0.
5. Klik **Approve Final**.
6. Sistem meminta **MFA Step-Up**.
7. Masukkan kode TOTP.
8. Isi **Catatan Approver**: `"Disetujui. ECL closing Juni 2026 sebesar IDR 169.500.000 akan dilaporkan di PSAK 71 disclosure."`
9. Klik **Konfirmasi Approval**.
   - Toast hijau: `"Roll-forward RFW-2026-06 berhasil di-approve. Laporan disclosure final tersedia."`
10. Status: `REVIEWED` → `APPROVED`.

---

### TC-007: Verifikasi Tolerance Rekonsiliasi

**Tujuan**: Skenario negatif — variance melebihi IDR 1,0000 harus menghasilkan status `UNRECONCILED`.

**Langkah-langkah**:

1. Seed data dengan ECL closing yang sengaja berbeda IDR 2 dari yang dikalkulasi.
2. Generate roll-forward baru `RFW-2026-06-FAIL`.

**Hasil yang Diharapkan**:
- Status rekonsiliasi: `UNRECONCILED`
- Pesan di UI: `"Rekonsiliasi gagal: Variance IDR 2,0000 melebihi toleransi IDR 1,0000. Periksa result line dan ulang kalkulasi."`
- Roll-forward TIDAK bisa di-submit selama `UNRECONCILED`.

---

## Checklist Audit Pasca UAT

```sql
-- 1. Roll-forward record tersimpan
SELECT rfw_id, reconcile_status, delta_variance, approved_by, approved_at
FROM ecl.roll_forward
WHERE periode_closing_id = 'PBUKU-2026-06';
-- Expected: 1 row, reconcile_status = RECONCILED, delta_variance = 0.0000

-- 2. Transfer buckets tersimpan
SELECT bucket_type, instrumen_id, nilai_idr
FROM ecl.roll_forward_bucket
WHERE rfw_id = (SELECT rfw_id FROM ecl.roll_forward WHERE periode_closing_id = 'PBUKU-2026-06')
ORDER BY bucket_type;

-- 3. Audit log approval CFO ada signature
SELECT action, actor_role, after_jsonb->>'signature_hash' as sig
FROM aud.audit_log
WHERE entity_type = 'ecl.roll_forward'
  AND action = 'ROLL_FORWARD.APPROVE'
ORDER BY event_time DESC LIMIT 1;
```

---

## Rollback / Cleanup

```sql
BEGIN;
DELETE FROM ecl.roll_forward_bucket WHERE rfw_id IN (
  SELECT rfw_id FROM ecl.roll_forward WHERE periode_closing_id = 'PBUKU-2026-06'
);
DELETE FROM ecl.roll_forward WHERE periode_closing_id = 'PBUKU-2026-06';
ROLLBACK; -- ubah ke COMMIT setelah verifikasi
```

---

## Sign-off UAT

| Nama | Jabatan | Tanda Tangan | Tanggal |
|------|---------|--------------|---------|
| | Akuntansi (Maker) | | |
| | Finance Controller (Reviewer) | | |
| | CFO (Approver) | | |
| | QA Engineer | | |
| | IFRS9 Compliance Reviewer | | |
