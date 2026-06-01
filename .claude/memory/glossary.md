# BLIPS Glossary — IFRS9 / PSAK 71 + Istilah Project

## Klasifikasi Instrumen Keuangan (PSAK 71 / IFRS 9)
- **AC** (Amortised Cost) — diukur pada biaya perolehan diamortisasi. Bunga di Gross Carrying. Stage-based ECL berlaku.
- **FVOCI debt** (Fair Value through OCI, debt) — perubahan fair value ke OCI (other comprehensive income), reclassified ke P&L saat derecognition. ECL diakui di P&L.
- **FVOCI equity** (FVOCI Election) — opsi irrevocable untuk equity instrument. Tidak ada recycling ke P&L on disposal. Tidak ada ECL.
- **FVTPL** (Fair Value through Profit & Loss) — perubahan fair value ke P&L. Tidak ada ECL.
- **POCI** (Purchased or Originated Credit Impaired) — credit-adjusted EIR sejak inisiasi.

## Tests
- **SPPI Test** — Solely Payments of Principal and Interest. 10-item checklist Q1–Q10. Failure → FVTPL.
- **Business Model (BM) Test** — per portofolio, 3 kategori:
  1. **HTC** Hold-to-Collect — koleksi cashflow kontraktual.
  2. **HTC&S** Hold-to-Collect-and-Sell — koleksi + penjualan.
  3. **Other** — termasuk trading.

## Matrix klasifikasi (SPPI × BM)
| | HTC | HTC&S | Other |
|---|---|---|---|
| **SPPI pass** | AC | FVOCI debt | FVTPL |
| **SPPI fail** | FVTPL | FVTPL | FVTPL |
| **Equity** | FVTPL (atau FVOCI Election irrevocable) |

## ECL (Expected Credit Loss)
- **Stage 1** — kredit "performing", 12-month PD, bunga di Gross Carrying.
- **Stage 2** — Significant Increase in Credit Risk (SICR), Lifetime PD, bunga di Gross Carrying.
- **Stage 3** — credit-impaired (default), PD = 1.0, bunga di **Net Carrying** (Gross − ECL).
- **SICR triggers** — rating turun ≥ 2 notch OR Investment Grade → non-IG OR DPD ≥ 30 hari.
- **Cure** — 3 bulan berturut-turut memenuhi kriteria cure.
- **PD** (Probability of Default) — kalibrasi dari Pefindo Annual Default Study + kurva internal.
- **LGD** (Loss Given Default) — pool-based, Basel-style.
- **EAD** (Exposure at Default) — saldo + akrual + komitmen.
- **Forward-Looking (FL) multiplier** — *dual* FL, applied per skenario (Good/Normal/Bad).
- **Skenario bobot default** — Good 0.25 / Normal 0.50 / Bad 0.25 (ALCO dapat override).

### Formula inti
```
ECL_skenario      = EAD × PD_skenario × LGD
ECL_FL_skenario   = ECL_skenario × Impact_PD_multiplier_skenario
ECL_weighted      = Σ (ECL_FL_skenario × bobot_skenario)
```

## EIR (Effective Interest Rate)
- **IRR solver** — Newton-Raphson, presisi 8 desimal, max 100 iter, tolerance 1e-10.
- **Re-estimation** on amendment kontrak → insert new schedule version, never UPDATE.
- **POCI** → credit-adjusted EIR (cashflow expectasi sudah PD-adjusted).

## Special handling
- **LPS Aggregator** — Cash + Deposito di-aggregate per nasabah per bank, cap IDR 2 miliar (sesuai LPS Lembaga Penjamin Simpanan), ECL hanya untuk eksess di atas cap.
- **Look-through ECL** (Reksadana) — decompose by underlying asset class, weighted ECL.
- **FVOCI Election** — saham, irrevocable, no P&L recycling on disposal.

## Workflow & Roles
- **4-eyes** — Maker → Reviewer → Approver (3 user berbeda).
- **6-eyes** — untuk klasifikasi PSAK 71 dan parameter master (4-eyes + 2 approvers).
- **SoD** Segregation of Duties — `maker_id ≠ reviewer_id ≠ approver_id`.
- **ROLE-MAKER-TR** Treasury Maker · **ROLE-APPR-TR** Treasury Approver · **ROLE-RISK** Risk Officer · **ROLE-AKUN** Akuntansi · **ROLE-AKUN-CTL** Finance Controller · **ROLE-CFO** · **ROLE-AUDIT** read-only · **ROLE-IT-ADMIN** · **ROLE-KOMITE** Komite Investasi · **ROLE-ALCO** Asset-Liability Committee.

## Periode Buku
- **Soft close** — periode masih bisa di-reopen untuk koreksi.
- **Hard close** — final, CFO approval + MFA step-up, tidak bisa di-reverse.
- **Roll-forward** — opening + transfers + originations − derecognitions ± remeasurements = closing.

## Jurnal Transitions (IFRS9)
- **AC → FVOCI** — recognize OCI gain/loss per matrix.
- **FVOCI debt → AC** — amortized cost reset.
- **Equity FVOCI on disposal** — gain/loss tetap di OCI, no recycling ke P&L.
- **Stage 1 ↔ Stage 2** — ECL movement booked, no carrying-amount change untuk AC.

## Integrasi Eksternal
- **Pefindo** — rating triwulanan (manual upload XLSX/CSV).
- **IBPA** — bond pricing harian (SFTP CSV).
- **KSEI/MI** — NAB Reksadana harian (manual upload).
- **BEI** — closing price saham harian (file feed).
- **BI JISDOR** — FX rate hari kerja 10:30 (API/scrape).
- **GL Host** — Phase 2 REST integration ke general ledger.
