# UAT-FULL-002 — Obligasi MTM → Stage 3 → Penjualan + OCI Recycling

**Modul**: Cross-Modul (APP-B + APP-C + APP-D)
**Story**: Obligasi FVOCI debt mengalami penurunan nilai (MTM drops), SICR → Stage 3, penjualan dengan OCI recycling ke P&L
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-002 |
| Referensi test | `TestP5M18_Obligasi_MTM_To_Stage3_To_Penjualan` |
| Referensi FSD | FSD-APP-B §2 (MTM), §4 (Penjualan), FSD-APP-C §3 (staging) |
| PSAK 71 | §5.4.1(b) interest Net Carrying Stage 3, §5.7.10(a) OCI recycling FVOCI debt |
| DEC terkait | DEC-010 (PD=1.0 Stage 3), DEC-011 (SICR DPD ≥ 30/90) |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Input MTM, buat penjualan |
| Treasury Approver | ROLE-APPR-TR | Approve penjualan |
| Risk Officer | ROLE-RISK | Monitor staging + ECL calc run |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal OCI recycling |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Instrumen obligasi OBL-CORP-001 klasifikasi FVOCI debt sudah APPROVED_ACTIVE |
| P2 | Rating Pefindo: BBB (Investment Grade) pada saat penempatan |
| P3 | Mapping jurnal event code MTM_FVOCI, REKLAS_OCI_PL sudah APPROVED_ACTIVE |
| P4 | Periode PBUKU-2026-06 status OPEN |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen | OBL-CORP-001 (obligasi korporat, FVOCI debt) |
| Nominal / Carrying awal | IDR 50.000.000.000 |
| MTM Day 1-5 | Drop total IDR 580 juta (5 hari berturut-turut) |
| DPD setelah 30 hari | 30 hari (SICR trigger) |
| DPD setelah 90 hari | 90 hari (Stage 3 trigger) |
| LGD Stage 3 | 55% |
| Harga penjualan | Gross carrying + IDR 500 juta premium |
| OCI accumulated | IDR -580.000.000 (akumulasi 5 hari MTM drop) |

---

## 5. Langkah-Langkah

### Fase 1: MTM Harian 5 Hari (OCI Accumulation)

**Step 1.1 — Upload MTM batch**

1. Login sebagai `usr-maker-tr-01`.
2. Buka `/transaksi/mtm/upload`.
3. Upload file MTM (template: harga, instrumen, tanggal).
4. Ulangi 5 hari berturut-turut dengan harga menurun.

Hasil yang diharapkan per hari:
- [ ] Jurnal MTM_FVOCI ter-post: D OCI / K Investasi Obligasi. Balanced.
- [ ] `oci_balance` instrumen bertambah negatif setiap hari.
- [ ] Audit: `MTM.POSTED` dan `MTM.OCI_ACCUMULATED`.
- [ ] Tidak ada jurnal ke P&L (FVOCI debt → OCI, bukan P&L).

### Fase 2: SICR Trigger → Stage 2

**Step 2.1 — DPD 30 hari → SICR**

1. Simulasikan DPD = 30 hari (update tanggal jatuh tempo bunga).
2. Jalankan ECL calc run.

Hasil yang diharapkan:
- [ ] System auto-detect SICR trigger: DPD ≥ 30 (DEC-011).
- [ ] Stage berubah 1 → 2.
- [ ] ECL naik: dari PD 12M ke PD Lifetime.
- [ ] Interest accrual: masih pada **Gross Carrying** (Stage 2).
- [ ] Audit: `ECL.SICR_TRIGGERED` + `ECL.STAGE_TRANSITION` (1→2).

### Fase 3: DPD 90 Hari → Stage 3

**Step 3.1 — DPD 90 hari → Stage 3**

1. Simulasikan DPD = 90 hari.
2. Jalankan ECL calc run.

Hasil yang diharapkan:
- [ ] Stage berubah 2 → 3.
- [ ] PD = 1.0 (certain default — DEC-010).
- [ ] ECL = EAD × PD(1.0) × LGD(55%) = IDR 27.500.000.000 (setelah MTM drops).
- [ ] **KRITIS**: Interest accrual berubah ke **Net Carrying Amount** (PSAK 71 §5.4.1(b)).
  - Net Carrying = Gross Carrying − ECL Reserve
  - Accrual = Net Carrying × EIR / 365
  - Nilai accrual Stage 3 HARUS LEBIH KECIL dari accrual Stage 2 (basis berbeda).
- [ ] Audit: `ECL.STAGE_TRANSITION` (2→3), `ECL.STAGE3_ACCRUAL` dengan `basis: "NET_CARRYING"`.

### Fase 4: Penjualan + OCI Recycling

**Step 4.1 — Buat transaksi penjualan (Treasury Maker)**

1. Buka `/transaksi/penjualan/new`.
2. Pilih OBL-CORP-001, masukkan harga penjualan.
3. Submit → Approve (4-eyes, SoD).

Hasil yang diharapkan:
- [ ] Jurnal PENJUALAN ter-post dengan 4 leg:
  1. K Investasi Obligasi (derecognize at carrying)
  2. D Kas (proceeds)
  3. D Beban OCI (recycle OCI loss)
  4. K/D Realized Gain/Loss P&L (net)
- [ ] Jurnal **balanced** (total debit = total kredit).
- [ ] OCI recycling event: audit `PENJUALAN.OCI_RECYCLED` dengan `recycled_to: "P&L"`.
- [ ] Instrumen status menjadi MATURED / derecognized.
- [ ] `deleted_at` terisi pada record instrumen.
- [ ] GL status: `DELIVERED`.
- [ ] Audit: `PENJUALAN.OCI_RECYCLED`.

**Step 4.2 — Verifikasi OCI balance setelah penjualan**

1. `GET /api/v1/master/instrumen/OBL-CORP-001` → `oci_balance`.

Hasil yang diharapkan:
- [ ] `oci_balance` = 0 setelah penjualan (sudah di-recycle).
- [ ] Rekonsiliasi: total gain/loss P&L = proceeds − carrying_awal + OCI_accumulated.

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| MTM posting (5×) | `MTM.POSTED` × 5, `MTM.OCI_ACCUMULATED` × 5 |
| SICR trigger | `ECL.SICR_TRIGGERED` |
| Stage 1→2 | `ECL.STAGE_TRANSITION` (from=1, to=2) |
| Stage 2→3 | `ECL.STAGE_TRANSITION` (from=2, to=3) |
| Stage 3 accrual | `ECL.STAGE3_ACCRUAL` (basis=NET_CARRYING) |
| Penjualan | `JURNAL.POSTED` (PENJUALAN) |
| OCI recycling | `PENJUALAN.OCI_RECYCLED` |

---

## 7. Rollback / Cleanup

1. Penjualan tidak dapat di-reverse setelah DELIVERED ke GL.
2. Buat jurnal koreksi manual (ROLE-AKUN + ROLE-AKUN-CTL approval).
3. Soft-delete record penjualan di `trx.penjualan` jika periode masih OPEN.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Risk Officer | | | PASS / FAIL | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
