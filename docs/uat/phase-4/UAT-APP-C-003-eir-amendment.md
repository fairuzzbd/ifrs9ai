# UAT Script — APP-C-003: Amandemen EIR — Re-estimasi & 4-Eyes Approval
**ID UAT**: UAT-APP-C-003-001
**Story**: APP-C-EIR-001..004 — EIR Amendment Flow (Newton-Raphson + Versioning)
**Modul**: APP-C ECL Engine — Phase 4
**Tanggal**: 2026-06-13
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Pre-conditions

### Infrastruktur
- Stack berjalan; migrasi 000029+ diterapkan (EIR versioning, amortisasi_schedule)
- Periode buku `PBUKU-2026-06` berstatus `OPEN`

### Aktor
| Username | Role | Keterangan |
|----------|------|------------|
| `akuntansi.uat1` | ROLE-AKUN | Maker — input amandemen |
| `risk.officer.uat1` | ROLE-RISK | Reviewer |
| `alco.member.uat1` | ROLE-ALCO | Approver (MFA) |

### Seed Data Instrumen + Jadwal EIR Awal

```sql
-- Instrumen obligasi AC untuk EIR test
INSERT INTO mst.instrumen (id, kode_instrumen, nama_instrumen, tipe_instrumen,
  klasifikasi_psak71, nominal, kupon_persen, tanggal_penempatan, tanggal_jatuh_tempo,
  mata_uang_kode, status, stage, created_by, updated_by)
VALUES (
  'g1000000-0000-0000-0000-000000000001',
  'OBL-EIR-UAT-001',
  'Obligasi EIR Test (UAT)',
  'OBLIGASI',
  'AC',
  10000000000.0000,  -- Nominal IDR 10B
  0.06500000,        -- Kupon 6.5% p.a.
  '2024-01-01',
  '2029-12-31',      -- 6 tahun
  'IDR',
  'AKTIF',
  1,
  'b1000000-0000-0000-0000-000000000001',
  'b1000000-0000-0000-0000-000000000001'
) ON CONFLICT (kode_instrumen) DO NOTHING;

-- Jadwal EIR awal (version 1, sebelum amandemen)
-- EIR hasil Newton-Raphson untuk cashflow awal = 0.06543211 (6.543211% p.a.)
INSERT INTO ecl.amortisasi_schedule (id, instrumen_id, schedule_version,
  tanggal_periode, principal_awal, principal_akhir, pembayaran_bunga,
  eir_persen, effective_from, effective_to, created_by, updated_by)
VALUES
  (gen_random_uuid(), 'g1000000-0000-0000-0000-000000000001', 1,
   '2024-12-31', 10000000000.0000, 10000000000.0000, 654321100.0000,
   0.06543211, '2024-01-01', 'infinity',
   'b1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;
```

---

## Referensi Numerik — EIR Re-estimasi

Kondisi amandemen: kupon berubah dari 6.5% menjadi 7.2% mulai 2026-07-01.

**Input cashflow baru** (sisa 3.5 tahun ke maturity 2029-12-31):
```
Tanggal amandemen: 2026-06-30
Nilai buku saat amandemen: IDR 10.000.000.000,0000 (asumsi par)
Cashflow baru per semi-annual:
  2026-12-31: 360.000.000  (7.2% × 10B / 2)
  2027-06-30: 360.000.000
  2027-12-31: 360.000.000
  2028-06-30: 360.000.000
  2028-12-31: 360.000.000
  2029-06-30: 360.000.000
  2029-12-31: 360.000.000 + 10.000.000.000 (pokok + bunga)

EIR baru (Newton-Raphson, semi-annual, DEC-013):
  r_semi = 0.03600000 (approx)
  EIR annual = (1 + r_semi)^2 - 1 ≈ 0.07329600

Verifikasi: f(0.036) = Σ CF_t/(1+0.036)^t harus = -10B (initial outflow = nilai buku)
Tolerance = 1e-10, max_iter = 100
```

**Aturan versioning (DEC-013)**:
- Version 1 jadwal TIDAK boleh di-UPDATE. `effective_to` di-set ke `2026-06-30`.
- Version 2 jadwal baru di-INSERT dengan `effective_from = 2026-07-01`.
- Audit: TIDAK boleh ada UPDATE pada baris versi lama.

---

## Skenario UAT

### TC-001: Input Amandemen Kontrak (Maker = Akuntansi)

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Login sebagai `akuntansi.uat1`.
2. Navigasi ke **APP-C → EIR → Amandemen Kontrak → Buat Amandemen**.
3. Cari instrumen: ketik `OBL-EIR-UAT-001`.
4. Isi form amandemen:
   - **Tanggal Efektif Amandemen**: `2026-06-30`
   - **Alasan Amandemen**: `Penyesuaian tingkat kupon dari 6.5% ke 7.2% per kesepakatan kontraktual`
   - **Tipe Amandemen**: `PERUBAHAN_TINGKAT_BUNGA`
   - **Kupon Baru (% p.a.)**: `7.2`
   - **Nilai Buku Saat Amandemen**: `10.000.000.000` (IDR)
5. Klik **Hitung EIR Baru** (preview, belum simpan).
   - Progress panel muncul: "Menghitung EIR via Newton-Raphson..."
   - Hasil: `EIR Baru = 7.329600% p.a. (0.07329600)`
6. Klik **Simpan Draft Amandemen**.
   - Toast hijau: `"Amandemen AMD-001 berhasil dibuat. Status: DRAFT. Menunggu review."`

**Hasil yang Diharapkan**:
- Amandemen tersimpan dengan status `DRAFT`.
- `amortisasi_schedule` belum diubah (versi baru belum aktif sampai approval).
- Preview EIR = 0.07329600 (toleransi ±0.00000001).

---

### TC-002: Submit untuk Review

**Aktor**: `akuntansi.uat1` (ROLE-AKUN)

**Langkah-langkah**:

1. Pada halaman amandemen AMD-001, klik **Submit untuk Review**.
2. Dialog konfirmasi: "Submit amandemen EIR OBL-EIR-UAT-001 untuk review?"
3. Isi **Catatan Maker**: `"EIR baru 7.33% dihitung dari cashflow aktual per term sheet."`
4. Klik **Submit**.
   - Toast hijau: `"AMD-001 berhasil di-submit. Menunggu review Risk Officer."`
5. Status berubah: `DRAFT` → `PENDING_REVIEW`.

---

### TC-003: Review EIR Amendment (Reviewer = Risk Officer)

**Aktor**: `risk.officer.uat1` (ROLE-RISK)

**Langkah-langkah**:

1. Login sebagai `risk.officer.uat1`.
2. Navigasi ke **APP-C → EIR → Antrian Review Amandemen**.
3. Buka AMD-001.
4. Verifikasi:
   - EIR sebelumnya: `6.543211%`
   - EIR baru: `7.329600%`
   - Selisih: `+0.786389%`
   - Catch-up adjustment estimate tampil (IFRS 9 §5.4.3).
5. Klik **Approve Review**.
6. Isi **Catatan Reviewer**: `"EIR baru terverifikasi. Cashflow sesuai term sheet yang sudah diotorisasi."`
7. Klik **Konfirmasi**.
   - Toast hijau: `"Review AMD-001 berhasil. Menunggu approval ALCO."`
8. Status: `PENDING_REVIEW` → `REVIEWED`.

**Uji SoD Negatif**:
- `akuntansi.uat1` (maker) mencoba review AMD-001 via API langsung.
- **Expected**: HTTP 403 `{ "error": { "code": "SOD_VIOLATION" } }`

---

### TC-004: Approval Amandemen EIR (Approver = ALCO, MFA)

**Aktor**: `alco.member.uat1` (ROLE-ALCO — MFA wajib DEC-027)

**Langkah-langkah**:

1. Login sebagai `alco.member.uat1`.
2. Navigasi ke **APP-C → EIR → Antrian Approval Amandemen**.
3. Buka AMD-001.
4. Klik **Approve Definitif**.
5. Sistem meminta **MFA Step-Up**: masukkan kode TOTP.
6. Isi **Catatan Approver**: `"Disetujui. EIR 7.33% berlaku mulai 2026-07-01."`
7. Klik **Konfirmasi Approval**.
   - Toast hijau: `"Amandemen AMD-001 berhasil di-approve. Jadwal EIR baru aktif mulai 2026-07-01."`
8. Status: `REVIEWED` → `APPROVED`.

---

### TC-005: Verifikasi Versioning Jadwal EIR (Audit Immutability)

**Aktor**: `akuntansi.uat1` (ROLE-AKUN — read mode)

**Langkah-langkah**:

1. Navigasi ke **APP-C → EIR → Jadwal Amortisasi → OBL-EIR-UAT-001**.
2. DataTable menampilkan 2 versi:
   - **Versi 1**: `effective_from = 2024-01-01`, `effective_to = 2026-06-30`, EIR = 6.543211%
   - **Versi 2**: `effective_from = 2026-07-01`, `effective_to = selamanya`, EIR = 7.329600%
3. Klik row Versi 1 → **TIDAK ada tombol Edit** (immutable).
4. Klik row Versi 2 → tombol **Edit Jadwal** ditampilkan (versi aktif, sebelum seal).

**Verifikasi DB Immutability**:
```sql
-- Versi 1 harus tetap ada dengan effective_to = 2026-06-30 (bukan didelete/diupdate)
SELECT schedule_version, effective_from, effective_to, eir_persen
FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'g1000000-0000-0000-0000-000000000001'
ORDER BY schedule_version;
-- Expected: 2 rows
-- Row 1: version=1, from=2024-01-01, to=2026-06-30, eir=0.06543211
-- Row 2: version=2, from=2026-07-01, to=infinity, eir=0.07329600

-- Verifikasi TIDAK ADA update pada row lama (audit log)
SELECT COUNT(*) FROM aud.audit_log
WHERE entity_type = 'ecl.amortisasi_schedule'
  AND action = 'AMORTISASI_SCHEDULE.UPDATE'
  AND entity_id IN (
    SELECT id FROM ecl.amortisasi_schedule
    WHERE instrumen_id = 'g1000000-0000-0000-0000-000000000001'
      AND schedule_version = 1
  );
-- Expected: 0 (no UPDATE event on version 1)
```

---

### TC-006: EIR Catch-Up Adjustment (IFRS 9 §5.4.3)

**Tujuan**: Verifikasi bahwa catch-up adjustment dihitung dan dijurnal saat amandemen di-approve.

**Langkah-langkah**:

1. Setelah AMD-001 di-approve, navigasi ke **APP-D → Jurnal → Filter: OBL-EIR-UAT-001 → 2026-06**.
2. Cari jurnal dengan tipe `EIR_CATCH_UP_ADJUSTMENT`.

**Hasil yang Diharapkan**:
- Jurnal ada dengan akun debit/kredit sesuai mapping GL.
- Nilai jurnal = catch-up = present value selisih EIR lama vs baru dihitung mundur ke tanggal amandemen.
- Jurnal berstatus `PENDING_POSTING` (belum di-post ke GL sampai periode tutup).

---

### TC-007: Reprodusibilitas EIR Solver

**Tujuan**: Solver Newton-Raphson menghasilkan nilai yang sama persis (DEC-013).

**Langkah-langkah**:

1. Via API: `POST /api/v1/eir/preview` dua kali dengan payload yang identik.
2. Bandingkan `eir_persen` di kedua response.

**Expected**: Nilai identik sampai 8 desimal (0.07329600 vs 0.07329600).

**Via curl**:
```bash
curl -s -X POST http://localhost:8080/api/v1/eir/preview \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{
    "instrumen_id": "g1000000-0000-0000-0000-000000000001",
    "tanggal_amandemen": "2026-06-30",
    "cashflows": [
      {"tanggal": "2026-12-31", "jumlah": 360000000},
      {"tanggal": "2027-06-30", "jumlah": 360000000},
      {"tanggal": "2027-12-31", "jumlah": 360000000},
      {"tanggal": "2028-06-30", "jumlah": 360000000},
      {"tanggal": "2028-12-31", "jumlah": 360000000},
      {"tanggal": "2029-06-30", "jumlah": 360000000},
      {"tanggal": "2029-12-31", "jumlah": 10360000000}
    ],
    "nilai_buku": 10000000000
  }'
-- Expected response: { "data": { "eir_persen": 0.07329600, "iterations": N, "converged": true } }
```

Ulangi 3 kali → nilai `eir_persen` harus identik setiap kali.

---

## Checklist Audit Pasca UAT

```sql
-- 1. Setiap state amandemen tercatat di audit_log
SELECT action, COUNT(*) FROM aud.audit_log
WHERE entity_type = 'ecl.eir_amendment'
GROUP BY action ORDER BY action;
-- Expected: EIR_AMENDMENT.CREATE, SUBMIT, REVIEW, APPROVE masing-masing ≥ 1

-- 2. Signature ALCO tersimpan
SELECT signed_by, signature_hash, signed_at FROM ecl.eir_amendment_approval
WHERE amendment_id = (SELECT id FROM ecl.eir_amendment WHERE kode = 'AMD-001');
-- Expected: 1 row

-- 3. Hash chain audit tidak rusak (run setelah UAT selesai)
-- go run ./cmd/audit-verify --entity-type ecl.amortisasi_schedule
```

---

## Rollback / Cleanup

```sql
BEGIN;
-- Hapus versi 2 jadwal (versi 1 tetap aktif kembali)
DELETE FROM ecl.amortisasi_schedule
WHERE instrumen_id = 'g1000000-0000-0000-0000-000000000001'
  AND schedule_version = 2;
-- Kembalikan effective_to versi 1 ke infinity
UPDATE ecl.amortisasi_schedule
SET effective_to = 'infinity'
WHERE instrumen_id = 'g1000000-0000-0000-0000-000000000001'
  AND schedule_version = 1;
-- Hapus amandemen record
DELETE FROM ecl.eir_amendment WHERE instrumen_id = 'g1000000-0000-0000-0000-000000000001';
ROLLBACK; -- ubah ke COMMIT setelah verifikasi
```

---

## Sign-off UAT

| Nama | Jabatan | Tanda Tangan | Tanggal |
|------|---------|--------------|---------|
| | Akuntansi (Maker) | | |
| | Risk Officer (Reviewer) | | |
| | ALCO Member (Approver) | | |
| | QA Engineer | | |
| | IFRS9 Compliance Reviewer | | |
