# UAT-FULL-006 — Reksadana Look-Through ECL

**Modul**: APP-C (ECL)
**Story**: Reksadana dengan komposisi underlying — ECL dihitung per asset class, aggregasi berbobot
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-006 |
| Referensi test | `TestP5M18_Reksadana_LookThrough_ECL` |
| Referensi FSD | FSD-APP-C §6 (Reksadana look-through) |
| PSAK 71 | §B5.5.22–24 (look-through untuk kolektif ECL) |
| DEC terkait | DEC-015 (look-through Reksadana) |
| Formula | `.claude/memory/formulas.md §Look-through ECL` |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Risk Officer | ROLE-RISK | Input komposisi, jalankan ECL calc run |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal ECL Reksadana |
| Compliance Officer | — | Verifikasi look-through methodology |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Instrumen Reksadana RDN-SCHRODER-001 klasifikasi AC sudah APPROVED |
| P2 | Komposisi underlying Reksadana sudah diinput: 60% obligasi korporat + 40% sukuk pemerintah |
| P3 | PD dan LGD per asset class sudah ada di master parameter |
| P4 | Mapping jurnal ECL_PEMBENTUKAN APPROVED |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen | RDN-SCHRODER-001 (Reksadana Pendapatan Tetap) |
| NAB Total | IDR 20.000.000.000 |
| **Underlying OBLIGASI_CORP (60%)** | |
| NAB kelas | IDR 12.000.000.000 |
| PD (12M) | 1,50% |
| LGD | 45% |
| FL Normal | 1,10 |
| ECL Normal kelas (estimasi) | IDR 89.100.000 |
| **Underlying SUKUK_GOV (40%)** | |
| NAB kelas | IDR 8.000.000.000 |
| PD (12M) | 0,05% |
| LGD | 10% |
| FL Normal | 1,02 |
| ECL Normal kelas (estimasi) | IDR 4.080.000 |
| **Total ECL (estimasi)** | IDR 93.180.000 |

*Catatan: Angka di atas menggunakan Normal scenario saja untuk ilustrasi. Sistem menggunakan 3 skenario berbobot 0.25/0.50/0.25.*

---

## 5. Langkah-Langkah

### Fase 1: Setup Komposisi Underlying

**Step 1.1 — Input komposisi Reksadana (Risk Officer)**

1. Login sebagai `usr-risk-01`.
2. Buka `/master/instrumen/RDN-SCHRODER-001/komposisi`.
3. Tambahkan:
   - OBLIGASI_CORP: 60%
   - SUKUK_GOV: 40%

Hasil yang diharapkan:
- [ ] Total komposisi = 100%.
- [ ] Sistem tolak jika total ≠ 100% (`VALIDATION_FAILED`).
- [ ] Audit: `REKSADANA.KOMPOSISI_UPDATED`.

### Fase 2: ECL Calc Run dengan Look-Through

**Step 2.1 — Jalankan ECL calc run**

1. Jalankan ECL calc run untuk periode PBUKU-2026-06.
2. Pantau `<JobProgressPanel>`.

Hasil yang diharapkan:
- [ ] Sistem detect instrumen Reksadana → trigger look-through path.
- [ ] ECL dihitung per asset class:
  - OBLIGASI_CORP: `NAB(12M) × PD_corp × LGD_corp × FL_corp`
  - SUKUK_GOV: `NAB(40%) × PD_gov × LGD_gov × FL_gov`
- [ ] ECL total Reksadana = Σ ECL per kelas.
- [ ] **KRITIS**: OBLIGASI_CORP ECL > SUKUK_GOV ECL (risiko kredit lebih tinggi).
- [ ] Hasil numerik sesuai formula (tolerance ≤ IDR 1.000 akibat rounding 4dp).
- [ ] Audit: `ECL.LOOK_THROUGH_APPLIED` dengan `class_count: 2`.

### Fase 3: Verifikasi Jurnal ECL

**Step 3.1 — Verifikasi posting ECL_PEMBENTUKAN**

1. Buka `/jurnal/header?filter[event_code]=ECL_PEMBENTUKAN&filter[instrumen_id]=RDN-SCHRODER-001`.

Hasil yang diharapkan:
- [ ] Jurnal ECL_PEMBENTUKAN dengan amount = total ECL Reksadana.
- [ ] Balanced: D Beban CKPN = K Cadangan CKPN.
- [ ] GL status: `DELIVERED`.

### Fase 4: Verifikasi Report Look-Through

**Step 4.1 — Buka report komposisi**

1. Buka `/reports/RPT-06-ecl-summary?instrumen=RDN-SCHRODER-001`.

Hasil yang diharapkan:
- [ ] Report menampilkan ECL per underlying asset class.
- [ ] Kolom: Asset Class | NAB | PD | LGD | FL | ECL | Weight.
- [ ] Total row = aggregasi berbobot.
- [ ] Export CSV valid dengan header Bahasa Indonesia.

---

## 6. Contoh Numerik (3 Skenario, DEC-010)

Untuk NAB = IDR 20 miliar, OBLIGASI_CORP (60%, PD=1.5%, LGD=45%):

| Skenario | PD | FL | ECL_kelas |
|---|---|---|---|
| Good (W=0.25) | 1.05% | 0.95 | IDR 53.865.000 |
| Normal (W=0.50) | 1.50% | 1.10 | IDR 89.100.000 |
| Bad (W=0.25) | 3.75% | 1.45 | IDR 293.850.000 |
| **ECL weighted OBLIGASI_CORP** | | | **IDR 131.455.000** |

---

## 7. Rollback / Cleanup

1. ECL result immutable setelah calc run di-seal.
2. Komposisi Reksadana bisa diupdate sebelum calc run berikutnya.
3. Soft-delete komposisi history dipertahankan untuk audit.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Risk Officer (UAT Actor) | | | PASS / FAIL | |
| ifrs9-compliance-reviewer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
