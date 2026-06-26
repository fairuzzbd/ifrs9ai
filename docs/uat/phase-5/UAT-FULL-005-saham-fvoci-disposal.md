# UAT-FULL-005 — Saham FVOCI Election: MTM Harian → Disposal Tanpa Recycling P&L

**Modul**: APP-B (MTM + Penjualan)
**Story**: Saham dengan FVOCI Election irrevocable — OCI accumulation, disposal, verifikasi tidak ada P&L recycling
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-005 |
| Referensi test | `TestP5M18_Saham_FVOCI_Election_Disposal` |
| Referensi FSD | FSD-APP-B §2 (MTM saham), §4 (Penjualan) |
| PSAK 71 | §5.7.5 (FVOCI Election irrevocable), §B5.7.1 (no recycling on disposal) |
| DEC terkait | DEC-010 (no ECL for FVOCI Election) |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Input MTM, buat penjualan |
| Treasury Approver | ROLE-APPR-TR | Approve penjualan |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal OCI tidak recycle |
| Compliance Officer | — | Review PSAK 71 §B5.7.1 compliance |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Instrumen saham SHM-FVOCI-ELEC-001 klasifikasi `FVOCI_ELECTION` APPROVED_ACTIVE |
| P2 | FVOCI Election flag bersifat irrevocable (tidak dapat diubah setelah approve) |
| P3 | Mapping jurnal PENJUALAN_SAHAM_FVOCI_ELECTION sudah APPROVED, tidak mengandung leg ke GL account P&L Realized G/L (4201) |
| P4 | Periode OPEN |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen | SHM-FVOCI-ELEC-001 (saham PT XYZ, FVOCI Election) |
| Nilai awal | IDR 2.000.000.000 |
| MTM harian (10 hari) | Gain hari 1: +10 juta, hari 2: +20 juta, ..., hari 10: +100 juta |
| Total OCI accumulated | IDR +550 juta (gain) |
| Harga penjualan | IDR 2.400.000.000 (< gross carrying setelah MTM) |
| GL account OCI | 3201 (Komponen Ekuitas Lain) |
| GL account Retained Earnings | 3101 |
| GL account P&L Realized G/L | 4201 (TIDAK boleh ada di jurnal) |

---

## 5. Langkah-Langkah

### Fase 1: MTM Harian (OCI Accumulation)

**Step 1.1 — Upload MTM harian 10 hari (Treasury Maker)**

1. Buka `/transaksi/mtm/upload`.
2. Upload file MTM dengan harga meningkat selama 10 hari.

Hasil yang diharapkan per hari:
- [ ] Jurnal MTM_FVOCI_ELECTION:
  - D Investasi Saham / K OCI (saat gain)
  - Tidak ada leg ke P&L
- [ ] `oci_balance` instrumen meningkat setiap hari.
- [ ] Audit: `MTM.POSTED` + `MTM.OCI_ACCUMULATED` dengan `fvoci_election: true`.
- [ ] **KRITIS**: Tidak ada jurnal ke GL account P&L (4001, 4201, 6001) untuk FVOCI Election.

**Step 1.2 — Verifikasi OCI balance (Akuntansi)**

1. `GET /api/v1/master/instrumen/SHM-FVOCI-ELEC-001`.

Hasil yang diharapkan:
- [ ] `oci_balance_idr = 550000000` setelah 10 hari.
- [ ] `gross_carrying = 2550000000` (nominal + OCI gain).

### Fase 2: Verifikasi Tidak Ada ECL (FVOCI Election Tidak Kena ECL)

**Step 2.1 — Jalankan ECL calc run**

1. Jalankan ECL calc run untuk periode.

Hasil yang diharapkan:
- [ ] Instrumen SHM-FVOCI-ELEC-001 di-skip dari ECL calc: `staging_action = "SKIPPED_FVTPL"` (FVOCI Election sama treatment dengan FVTPL untuk ECL).
- [ ] Tidak ada `ecl_calc_result_line` untuk instrumen ini.
- [ ] Audit: `PENEMPATAN.STAGING_SKIPPED_FVTPL`.

### Fase 3: Disposal (Penjualan Saham)

**Step 3.1 — Buat penjualan saham (Treasury Maker)**

1. Buka `/transaksi/penjualan/new`.
2. Pilih SHM-FVOCI-ELEC-001, masukkan harga penjualan IDR 2.400.000.000.
3. Submit → Review → Approve (SoD: 3 user berbeda).

Hasil yang diharapkan saat approve:
- [ ] Jurnal PENJUALAN_SAHAM_FVOCI_ELECTION ter-post dengan leg:
  1. K Investasi Saham (1301): IDR 2.550.000.000 (gross carrying after MTM)
  2. D Kas (1001): IDR 2.400.000.000 (proceeds)
  3. D Ekuitas — Retained Earnings atau Komponen Ekuitas (3101): IDR 150.000.000 (net loss to equity)
- [ ] **KRITIS**: TIDAK ada leg ke GL account 4201 (P&L Realized Gain/Loss).
- [ ] OCI balance di-transfer ke Retained Earnings (bukan di-recycle ke P&L).
- [ ] Jurnal balanced (total debit = total kredit).
- [ ] Audit: `PENJUALAN.OCI_NO_RECYCLING_FVOCI_ELECTION` dengan `recycled_to_pl: false`.
- [ ] `deleted_at` terisi pada instrumen.

**Step 3.2 — Verifikasi tidak ada P&L impact (Compliance Officer)**

1. Buka `/reports/RPT-08-laba-rugi?instrumen=SHM-FVOCI-ELEC-001&periode=PBUKU-2026-06`.

Hasil yang diharapkan:
- [ ] Tidak ada baris realized gain/loss untuk SHM-FVOCI-ELEC-001 di laporan P&L.
- [ ] OCI tersebut masuk ke laporan OCI terpisah, tidak masuk P&L.
- [ ] Sesuai PSAK 71 §B5.7.1: "Entitas tidak boleh kemudian mentransfer keuntungan atau kerugian tersebut ke laba rugi".

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| MTM posting (10×) | `MTM.POSTED` × 10, `MTM.OCI_ACCUMULATED` × 10 |
| ECL skip | `PENEMPATAN.STAGING_SKIPPED_FVTPL` |
| Penjualan | `JURNAL.POSTED` (PENJUALAN_SAHAM_FVOCI_ELECTION) |
| OCI no recycling | `PENJUALAN.OCI_NO_RECYCLING_FVOCI_ELECTION` |

---

## 7. Rollback / Cleanup

1. Penjualan setelah DELIVERED ke GL tidak dapat di-reverse.
2. Soft-delete instrumen jika periode masih OPEN.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Akuntansi (UAT Actor) | | | PASS / FAIL | |
| ifrs9-compliance-reviewer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
