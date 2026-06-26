# UAT-FULL-007 — LPS Aggregator: Cash + Deposito Per Nasabah Per Bank

**Modul**: APP-C (ECL)
**Story**: Agregasi exposures cash + deposito di bank yang sama, cap LPS IDR 2 miliar, ECL hanya pada eksess
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-007 |
| Referensi test | `TestP5M18_LPS_Aggregator_Cash_Plus_Deposito` |
| Referensi FSD | FSD-APP-C §7 (LPS Aggregator) |
| Dasar hukum | UU No. 24 Tahun 2004 (LPS), Cap IDR 2 miliar per nasabah per bank |
| DEC terkait | DEC-014 (LPS cap IDR 2 miliar) |
| Formula | `.claude/memory/formulas.md §LPS Aggregator` |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Risk Officer | ROLE-RISK | Konfigurasi LPS cap, verifikasi aggregasi |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal ECL LPS |
| Internal Auditor | ROLE-AUDIT | Audit trail LPS application |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | `sys.parameter` `lps_cap_idr = 2000000000` sudah dikonfigurasikan |
| P2 | Nasabah BANK-BNI-TUGURE: Cash CASH-BNI-001 (IDR 500 juta) + Deposito DEP-BNI-A (IDR 1.2 miliar) + Deposito DEP-BNI-B (IDR 800 juta) sudah APPROVED_ACTIVE |
| P3 | Semua instrumen di bank yang sama (BNI) dan nasabah yang sama (Tugu Re) |
| P4 | PD bank BNI = 0,20%, LGD = 30% |
| P5 | Mapping jurnal ECL_PEMBENTUKAN_LPS APPROVED |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| **Cash** | |
| Instrumen | CASH-BNI-001 |
| Saldo | IDR 500.000.000 |
| **Deposito A** | |
| Instrumen | DEP-BNI-A-001 |
| Nominal | IDR 1.200.000.000 |
| **Deposito B** | |
| Instrumen | DEP-BNI-B-001 |
| Nominal | IDR 800.000.000 |
| **Agregasi** | |
| Total Exposure | IDR 2.500.000.000 |
| LPS Cap | IDR 2.000.000.000 |
| Covered (dijamin) | IDR 2.000.000.000 → ECL = IDR 0 |
| Excess | IDR 500.000.000 |
| ECL pada Excess (estimasi, Normal scenario) | IDR 500.000.000 × 0.20% × 30% × 1.05 = IDR 315.000 |

---

## 5. Langkah-Langkah

### Fase 1: Verifikasi Konfigurasi LPS

**Step 1.1 — Cek sys.parameter**

1. `GET /api/v1/master/parameter?key=lps_cap_idr`.

Hasil yang diharapkan:
- [ ] Response: `{ "key": "lps_cap_idr", "value": "2000000000" }`.
- [ ] Tidak ada hard-code di code; parameter dari DB.

### Fase 2: ECL Calc Run dengan LPS Aggregation

**Step 2.1 — Jalankan ECL calc run (Risk Officer)**

1. Jalankan ECL calc run untuk periode.
2. Pantau JobProgressPanel.

Hasil yang diharapkan:
- [ ] Sistem mendeteksi 3 instrumen milik nasabah yang sama di bank yang sama.
- [ ] Aggregasi: total exposure = IDR 2.500.000.000.
- [ ] `covered = min(2.500.000.000, 2.000.000.000) = 2.000.000.000`.
- [ ] `excess = 500.000.000`.
- [ ] ECL untuk covered portion = **IDR 0** (dijamin LPS).
- [ ] ECL hanya dihitung pada excess IDR 500.000.000.
- [ ] ECL Excess = `500.000.000 × PD_bank × LGD_bank × FL-weighted` (≈ IDR 315.000).
- [ ] Audit: `ECL.LPS_APPLIED` dengan semua kolom yang diisi.

**Step 2.2 — Verifikasi distribusi ECL ke instrumen**

1. `GET /api/v1/ecl/calc-result-line?filter[instrumen_id]=CASH-BNI-001`.

Hasil yang diharapkan:
- [ ] `ecl_weighted_idr` di setiap instrumen menggunakan alokasi proporsional dari total excess ECL.
- [ ] Total ECL tiga instrumen = total excess ECL (tidak double-count).

### Fase 3: Jurnal ECL LPS

**Step 3.1 — Verifikasi jurnal**

1. `GET /api/v1/jurnal/header?filter[event_code]=ECL_PEMBENTUKAN_LPS`.

Hasil yang diharapkan:
- [ ] Jurnal ter-post dengan amount = excess ECL only (bukan total exposure).
- [ ] Balanced: D Beban CKPN / K Cadangan CKPN.
- [ ] Tidak ada jurnal ECL untuk covered portion.

### Fase 4: Skenario Batas LPS

**Step 4.1 — Skenario: total ≤ LPS cap**

1. Kurangi Deposito B menjadi IDR 200 juta → total = IDR 1.9 miliar < IDR 2 miliar.

Hasil yang diharapkan:
- [ ] Seluruh exposure di-cover LPS.
- [ ] `excess = 0`.
- [ ] ECL total = **IDR 0**.
- [ ] Jurnal ECL tidak ter-post (tidak ada beban CKPN).

**Step 4.2 — Skenario: nasabah berbeda, bank sama**

1. Tambahkan instrumen nasabah lain di bank BNI.

Hasil yang diharapkan:
- [ ] LPS aggregasi dilakukan per (nasabah, bank) — bukan per bank saja.
- [ ] Nasabah berbeda tidak ter-aggregate bersama.

---

## 6. Contoh Numerik Lengkap (3 Skenario)

Excess = IDR 500 juta, PD Bank = 0.20%, LGD = 30%:

| Skenario | PD | FL | ECL |
|---|---|---|---|
| Good (W=0.25) | 0.14% | 0.95 | IDR 199.500 |
| Normal (W=0.50) | 0.20% | 1.05 | IDR 315.000 |
| Bad (W=0.25) | 0.50% | 1.45 | IDR 1.087.500 |
| **ECL Weighted** | | | **IDR 479.625** |

---

## 7. Audit Checks

| Aksi | Audit Action |
|---|---|
| LPS applied | `ECL.LPS_APPLIED` (nasabah_id, bank_id, covered, excess, ecl) |
| ECL calc run | `ECL.CALC_RUN` |
| Jurnal ECL | `JURNAL.POSTED` (ECL_PEMBENTUKAN_LPS) |

---

## 8. Rollback / Cleanup

1. ECL result bisa di-override sebelum periode close jika parameter berubah.
2. Soft-delete instrumen tidak hapus ECL history.

---

## 9. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Risk Officer (UAT Actor) | | | PASS / FAIL | |
| ifrs9-compliance-reviewer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
