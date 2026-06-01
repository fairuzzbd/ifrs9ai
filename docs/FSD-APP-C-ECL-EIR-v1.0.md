*\[ LOGO TUGURE \]*

**FUNCTIONAL SPECIFICATION DOCUMENT**

**BLIPS IFRS 9 — APPENDIX C**

*ECL Engine • EIR & Amortisasi (PSAK 71 Compliance Core)*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**        | **Keterangan**                                                               |
| ------------------ | ---------------------------------------------------------------------------- |
| Judul Dokumen      | FSD Appendix C — ECL Engine + EIR & Amortisasi                               |
| Kode Dokumen       | FSD-APP-C-2026-001                                                           |
| Versi              | 1.0                                                                          |
| Status             | DRAFT FOR REVIEW                                                             |
| Tanggal Terbit     | 02 Mei 2026                                                                  |
| Reference Upstream | FSD Master v1.0; BRD v1.0; SoW v1.1 §5.12, §7, §8                            |
| Modul Tercakup     | ECL Engine (3-Stage, 3-Skenario, dual FL); EIR & Amortisasi (Newton-Raphson) |
| BR-IDs             | BR-ECL-001 to 024; BR-EIR-001 to 020                                         |
| Komentar           | DOKUMEN COMPLIANCE-CRITICAL — review wajib oleh DSAK-certified accountant    |

# Outline Appendix C

| **Bab** | **Topik**                                           |
| ------- | --------------------------------------------------- |
| 1       | ECL Engine — Architecture & Workflow                |
| 2       | ECL Engine — Algorithm Detail (3-Skenario, Dual FL) |
| 3       | ECL Engine — 3-Stage Model (PSAK 71)                |
| 4       | ECL Engine — LPS Aggregator (Cash + Deposito)       |
| 5       | ECL Engine — Look-through Reksadana                 |
| 6       | ECL Engine — Job Batch Akhir Bulan                  |
| 7       | ECL Engine — Field Specs & Schema                   |
| 8       | EIR Engine — Newton-Raphson IRR Solver              |
| 9       | EIR Engine — Amortization Schedule                  |
| 10      | EIR Engine — Re-estimation                          |
| 11      | EIR Engine — Stage 3 (Net Carrying) Handling        |
| 12      | EIR Engine — Reklasifikasi Handling                 |
| 13      | EIR Engine — Field Specs & Schema                   |
| 14      | API Endpoints (ECL & EIR)                           |

# 1\. ECL Engine — Architecture & Workflow

## 1.1 Architectural Overview

ECL Engine merupakan compliance-critical batch service yang menghitung Expected Credit Loss berbasis PSAK 71. Arsitektur:

> ┌─────────────────────────────────────────────────────────────────┐  
> │ ECL ENGINE SERVICE │  
> │ │  
> │ ┌─────────────┐ ┌──────────────┐ ┌──────────────────────┐ │  
> │ │ Trigger │──▶│ Calculation │──▶│ Posting Service │ │  
> │ │ Service │ │ Service │ │ (Jurnal Generation) │ │  
> │ └─────┬───────┘ └──────┬───────┘ └────────┬──────────────┘ │  
> │ │ │ │ │  
> │ ┌─────▼──────────────────▼────────────────────▼──────────────┐ │  
> │ │ PARAMETER LOADER (cached) │ │  
> │ │ - Master PD Pefindo (12M & Lifetime) │ │  
> │ │ - Master LGD Basel │ │  
> │ │ - Master Bobot Skenario │ │  
> │ │ - Master Impact MEV to PD (per periode) │ │  
> │ │ - Master Impact PD multiplier (per periode) │ │  
> │ │ - Master LPS Coverage │ │  
> │ └──────────────────────────────────────────────────────────────┘ │  
> │ │  
> │ ┌─────────────┐ ┌──────────────┐ ┌─────────────────────┐ │  
> │ │ STAGING │ │ LPS │ │ LOOK-THROUGH │ │  
> │ │ EVALUATOR │ │ AGGREGATOR │ │ ENGINE (Reksadana) │ │  
> │ └─────────────┘ └──────────────┘ └─────────────────────┘ │  
> └──────────────────────────────────────────────────────────────────┘  
>   
> Triggers:  
> 1\. Scheduled: end of month batch  
> 2\. Event: rating change (re-evaluate affected instruments)  
> 3\. Event: parameter update (re-run for current period)  
> 4\. Manual: from Risk Officer (re-calc on demand)

## 1.2 ECL Computation Flow

1.  Trigger: Job batch akhir bulan; event-driven; manual.

2.  Load semua parameter master ke memory (cache 1-jam TTL).

3.  Query semua instrumen aktif dengan klasifikasi AC, FVOCI utang, FVOCI Reksadana — instrumen-instrumen yang require ECL.

4.  Untuk Cash + Deposito per Bank: jalankan LPS Aggregator dulu (Bab 4).

5.  Untuk Reksadana FVOCI: jalankan Look-through Engine (Bab 5).

6.  Untuk setiap instrumen (atau underlying): evaluate Stage berdasarkan Counterparty Rating History + DPD + qualitative trigger.

7.  Compute ECL per skenario (Good, Normal, Bad): EAD × PD × LGD.

8.  Compute ECL Weighted: Σ (w\_skenario × ECL\_skenario).

9.  Apply Impact PD multiplier untuk derive ECL FL (Forward-Looking).

10. Persist hasil ke ecl\_calc\_header + ecl\_calc\_detail\_skenario.

11. Trigger Posting Service: generate jurnal ECL\_PEMBENTUKAN, ECL\_REVERSAL, atau STAGE\_MIGRATION delta — sesuai Master Mapping Jurnal.

12. Generate ECL Detail Report ke Akuntansi & Risk.

# 2\. ECL Algorithm Detail — 3-Skenario, Dual Forward-Looking

## 2.1 Formula Inti

> \# Step 0: Konversi EAD ke IDR (untuk valas)  
> EAD\_IDR = EAD\_native\_currency × Kurs\_Tengah\_BI(tanggal\_evaluasi)  
>   
> \# Step 1: Derivasi PD per skenario dari PD Normal (Pefindo)  
> PD\_Good = PD\_Normal × Impact\_MEV\_Good \# umumnya \< 1  
> PD\_Normal = PD\_Normal \# langsung dari Pefindo  
> PD\_Bad = PD\_Normal × Impact\_MEV\_Bad \# umumnya \> 1  
>   
> \# Untuk Stage 1: gunakan PD 12-Month  
> \# Untuk Stage 2 / 3: gunakan PD Lifetime (cumulative atau derived)  
>   
> \# Step 2: ECL per skenario  
> ECL\_Good\_IDR = EAD\_IDR × PD\_Good × LGD  
> ECL\_Normal\_IDR = EAD\_IDR × PD\_Normal × LGD  
> ECL\_Bad\_IDR = EAD\_IDR × PD\_Bad × LGD  
>   
> \# Step 3: ECL Weighted  
> ECL\_Weighted\_IDR = (w\_good × ECL\_Good\_IDR)  
> \+ (w\_normal × ECL\_Normal\_IDR)  
> \+ (w\_bad × ECL\_Bad\_IDR)  
>   
> \# Default bobot: w\_good=0.25, w\_normal=0.50, w\_bad=0.25 (sum=1.00)  
>   
> \# Step 4: Apply Impact PD multiplier (Forward-Looking adjustment)  
> ECL\_FL\_IDR = ECL\_Weighted\_IDR × Impact\_PD  
>   
> \# Untuk Stage 3: PD = 1.0000 (default actual); ECL = EAD × 1 × LGD = EAD × LGD  
> \# Bunga dihitung pada Net Carrying (post-CKPN), lihat EIR Bab 11

## 2.2 Presisi Numerik

| **Komponen**                  | **Presisi**                   | **Note**                   |
| ----------------------------- | ----------------------------- | -------------------------- |
| PD (Normal/Good/Bad/Lifetime) | 4 desimal                     | Stored NUMERIC(8,4)        |
| LGD                           | 4 desimal                     | Stored NUMERIC(8,4)        |
| Impact MEV                    | 4 desimal                     | Stored NUMERIC(8,4)        |
| Impact PD                     | 4 desimal                     | Stored NUMERIC(8,4)        |
| Bobot Skenario                | 4 desimal                     | Sum harus = 1,0000 ±0,0001 |
| EAD (IDR)                     | 2 desimal internal            | Stored NUMERIC(20,2)       |
| ECL (IDR)                     | 2 desimal internal            | Stored NUMERIC(20,2)       |
| ECL FL Final (Display)        | 2 desimal IDR                 | Round-half-to-even         |
| EIR                           | 8 desimal internal, 4 display | Stored NUMERIC(12,8)       |

## 2.3 Sample Numerical Walkthrough

Example: Obligasi PT XYZ (rating idA-, FVOCI), Stage 1, EAD Rp 5.075.000.000:

> Input:  
> EAD\_IDR = 5,075,000,000.00  
> Rating Pefindo = idA (PD 12-Month Normal = 0.0020 = 0.20%)  
> Impact MEV Good = 0.5000 → PD Good = 0.0020 × 0.5000 = 0.0010  
> Impact MEV Bad = 2.5000 → PD Bad = 0.0020 × 2.5000 = 0.0050  
> LGD = 0.4500 (Senior Unsecured Korporasi)  
> Bobot: Good=0.25, Normal=0.50, Bad=0.25  
> Impact PD = 1.1500 (standard FL adjustment)  
>   
> Step 1: PD per skenario  
> PD Good = 0.0010  
> PD Normal = 0.0020  
> PD Bad = 0.0050  
>   
> Step 2: ECL per skenario  
> ECL\_Good = 5,075,000,000 × 0.0010 × 0.4500 = 2,283,750.00  
> ECL\_Normal = 5,075,000,000 × 0.0020 × 0.4500 = 4,567,500.00  
> ECL\_Bad = 5,075,000,000 × 0.0050 × 0.4500 = 11,418,750.00  
>   
> Step 3: ECL Weighted  
> ECL\_Weighted = 0.25 × 2,283,750.00  
> \+ 0.50 × 4,567,500.00  
> \+ 0.25 × 11,418,750.00  
> \= 570,937.50 + 2,283,750.00 + 2,854,687.50  
> \= 5,709,375.00  
>   
> Step 4: ECL FL  
> ECL\_FL = 5,709,375.00 × 1.1500 = 6,565,781.25  
>   
> Output:  
> ecl\_weighted\_idr = 5,709,375.00  
> ecl\_fl\_idr = 6,565,781.25 (post jurnal)

# 3\. ECL Engine — 3-Stage Model (PSAK 71)

## 3.1 Definisi 3 Stage

| **Stage** | **Karakteristik**                                | **Sumber PD**                               | **Pendapatan Bunga**           |
| --------- | ------------------------------------------------ | ------------------------------------------- | ------------------------------ |
| Stage 1   | Performing — origination atau no SICR            | PD 12-Month (Pefindo)                       | Gross Carrying × EIR           |
| Stage 2   | Underperforming — SICR triggered, belum default  | Lifetime PD                                 | Gross Carrying × EIR           |
| Stage 3   | Non-Performing / Credit-Impaired — sudah default | Lifetime PD (PD = 1.0 untuk default actual) | Net Carrying × EIR (post-CKPN) |

## 3.2 Trigger Migrasi

**A. Stage 1 → Stage 2 (SICR Indicator):**

13. Penurunan rating Pefindo ≥ 2 notch dari rating saat origination.

14. Rating berpindah dari investment grade (idBBB+ ke atas) ke non-investment grade (idBB+ ke bawah).

15. Tunggakan pembayaran 30-90 hari.

16. Outlook rating NEGATIVE 2 review berturut-turut.

17. PD 12-Month meningkat ≥ 100% dari level origination (kuantitatif).

18. Kondisi keuangan issuer memburuk material (covenant breach, qualified opinion auditor).

**B. Stage 2 → Stage 3 (Default / Credit-Impaired):**

19. Rating Pefindo = idD.

20. Tunggakan pembayaran \> 90 hari (rebuttable presumption).

21. Counterparty mengajukan PKPU/Pailit.

22. Forced restructuring dengan kerugian material.

23. Gagal bayar kupon/pokok actual.

**C. Curing — Migrasi Mundur (Stage 3 → 2 → 1):**

24. Probationary period 3-6 bulan dengan kondisi yang memicu SICR/default tidak lagi terpenuhi.

25. Curing wajib bertahap (tidak boleh Stage 3 langsung ke Stage 1).

26. Approval Komite Risiko + dokumen pendukung.

## 3.3 Stage Migration Algorithm

> function evaluateStage(instrumen, evaluation\_date):  
> rating\_history = getRatingHistory(instrumen.counterparty\_id, evaluation\_date)  
> current\_rating = rating\_history\[-1\].rating  
> origination\_rating = rating\_history\[0\].rating \# rating saat penempatan  
>   
> \# Get current stage dari stage\_history  
> current\_stage = getCurrentStage(instrumen) or 'STAGE\_1' \# default  
>   
> \# Check Default trigger (Stage 3)  
> if current\_rating == 'idD':  
> return ('STAGE\_3', 'DEFAULT\_RATING\_D')  
>   
> dpd = getDaysPastDue(instrumen)  
> if dpd \> 90:  
> return ('STAGE\_3', 'DPD\_GT\_90')  
>   
> if hasPKPUorPailit(instrumen.counterparty\_id):  
> return ('STAGE\_3', 'PKPU\_PAILIT')  
>   
> if hasForcedRestructuring(instrumen):  
> return ('STAGE\_3', 'RESTRUKTURISASI')  
>   
> \# Check SICR trigger (Stage 2)  
> notch\_change = computeNotchChange(origination\_rating, current\_rating)  
> if notch\_change \<= -2:  
> return ('STAGE\_2', 'RATING\_DOWNGRADE')  
>   
> if isInvestmentGrade(origination\_rating) and not isInvestmentGrade(current\_rating):  
> return ('STAGE\_2', 'IG\_TO\_NON\_IG')  
>   
> if 30 \<= dpd \<= 90:  
> return ('STAGE\_2', 'DPD\_30\_90')  
>   
> pd\_origination = lookupPD(origination\_rating, '12M')  
> pd\_current = lookupPD(current\_rating, '12M')  
> if pd\_current \>= pd\_origination \* 2:  
> return ('STAGE\_2', 'PD\_DOUBLED')  
>   
> \# Check Curing — manual approval required, processed separately  
> \# (not auto via this evaluator)  
>   
> \# Default: Stage 1  
> return ('STAGE\_1', 'NO\_SICR')  
>   
>   
> function migrateStage(instrumen, new\_stage, trigger\_type):  
> old\_stage = instrumen.current\_stage  
> if old\_stage == new\_stage:  
> return \# no change  
>   
> \# Insert stage\_history record  
> stage\_history = {  
> instrumen\_id: instrumen.id,  
> tanggal\_migrasi: today(),  
> stage\_sebelum: old\_stage,  
> stage\_sesudah: new\_stage,  
> trigger\_type: trigger\_type,  
> status\_approval: 'AUTO' if forward\_migration else 'PENDING\_APPROVAL'  
> }  
> insertStageHistory(stage\_history)  
>   
> \# Re-calc ECL on new stage basis  
> if new\_stage in ('STAGE\_2', 'STAGE\_3'):  
> \# use Lifetime PD instead of 12M  
> recalcECL(instrumen, use\_lifetime\_pd=True)  
>   
> \# Trigger STAGE\_MIGRATION jurnal: Δ ECL = ECL\_baru - ECL\_lama  
> delta\_ecl = computeECL(instrumen, new\_stage) - computeECL(instrumen, old\_stage)  
> postJurnal('STAGE\_MIGRATION', delta\_ecl)

## 3.4 Stage History Field Specifications

| **Field**                | **Type**      | **Note**                                                                                                          |
| ------------------------ | ------------- | ----------------------------------------------------------------------------------------------------------------- |
| id                       | UUID PK       |                                                                                                                   |
| stage\_history\_id\_kode | VARCHAR(20)   | STH-YYYY-\#\#\#\#\#                                                                                               |
| instrumen\_id FK         | UUID          |                                                                                                                   |
| tanggal\_migrasi         | DATE          |                                                                                                                   |
| stage\_sebelum           | VARCHAR(10)   | STAGE\_1/2/3                                                                                                      |
| stage\_sesudah           | VARCHAR(10)   |                                                                                                                   |
| trigger\_type            | VARCHAR(30)   | RATING\_DOWNGRADE/DPD\_30\_90/DPD\_GT\_90/DEFAULT\_RATING\_D/PKPU\_PAILIT/RESTRUKTURISASI/CURING/MANUAL\_OVERRIDE |
| detail\_trigger          | TEXT          |                                                                                                                   |
| rating\_saat\_migrasi    | VARCHAR(8)    |                                                                                                                   |
| dpd                      | INT           | Days past due                                                                                                     |
| delta\_ecl\_idr          | NUMERIC(20,2) | ECL impact                                                                                                        |
| user\_approver\_id FK    | UUID          | Wajib untuk curing & manual override                                                                              |
| status\_approval         | VARCHAR(30)   | AUTO/PENDING\_APPROVAL/APPROVED/REJECTED                                                                          |
| dokumen\_pendukung\_url  | VARCHAR(500)  |                                                                                                                   |
| created\_at              | TIMESTAMPTZ   |                                                                                                                   |

# 4\. ECL Engine — LPS Aggregator (Cash + Deposito)

## 4.1 Konsep

LPS menjamin total Cash (Tabungan/Giro) + Deposito per nasabah per bank, hingga Rp 2 Miliar. Eksposur untuk ECL hanya bagian yang tidak terjamin LPS, lalu dialokasikan kembali secara proporsional ke Cash dan Deposito.

## 4.2 Algorithm

> function aggregateLPS(bank\_id, evaluation\_date):  
> \# Step 1: Sum saldo Cash + Deposito per Bank  
> cash\_total = sum(saldo for instrumen in mst.instrumen  
> where tipe='CASH' and counterparty\_id=bank\_id and status='AKTIF')  
> deposito\_total = sum(saldo for instrumen in mst.instrumen  
> where tipe='DEPOSITO' and counterparty\_id=bank\_id and status='AKTIF')  
> total\_bank = cash\_total + deposito\_total  
>   
> \# Step 2: Eksposur tak terjamin = MAX(0, Total - LPS Coverage)  
> LPS\_COVERAGE = 2\_000\_000\_000 \# IDR  
> ead\_bank\_unjamin = max(0, total\_bank - LPS\_COVERAGE)  
>   
> if ead\_bank\_unjamin == 0:  
> \# Fully covered by LPS — no ECL  
> return {  
> 'cash\_ead': 0,  
> 'deposito\_ead': 0,  
> 'total\_ead\_unjamin': 0  
> }  
>   
> \# Step 3: Alokasi proporsional  
> proportion\_cash = cash\_total / total\_bank  
> proportion\_deposito = deposito\_total / total\_bank  
>   
> ead\_cash = proportion\_cash \* ead\_bank\_unjamin  
> ead\_deposito = proportion\_deposito \* ead\_bank\_unjamin  
>   
> return {  
> 'cash\_ead': ead\_cash,  
> 'deposito\_ead': ead\_deposito,  
> 'total\_ead\_unjamin': ead\_bank\_unjamin,  
> 'proportion\_cash': proportion\_cash,  
> 'proportion\_deposito': proportion\_deposito  
> }  
>   
>   
> function computeECL\_LPS(bank\_id, evaluation\_date):  
> aggregated = aggregateLPS(bank\_id, evaluation\_date)  
>   
> if aggregated\['total\_ead\_unjamin'\] == 0:  
> return {'cash\_ecl\_fl': 0, 'deposito\_ecl\_fl': 0}  
>   
> bank = getBank(bank\_id)  
> pd\_normal = lookupPD(bank.rating\_pefindo, '12M')  
> impact\_mev\_good = lookupImpactMEV('GOOD', evaluation\_date)  
> impact\_mev\_bad = lookupImpactMEV('BAD', evaluation\_date)  
> impact\_pd = lookupImpactPD(evaluation\_date)  
>   
> pd\_good = pd\_normal \* impact\_mev\_good  
> pd\_bad = pd\_normal \* impact\_mev\_bad  
>   
> LGD = 0.4500 \# Senior Unsecured Bank  
> w\_good, w\_normal, w\_bad = 0.25, 0.50, 0.25  
>   
> \# Compute ECL per komponen (Cash, Deposito)  
> for component in \['cash', 'deposito'\]:  
> ead = aggregated\[f'{component}\_ead'\]  
> ecl\_good = ead \* pd\_good \* LGD  
> ecl\_normal = ead \* pd\_normal \* LGD  
> ecl\_bad = ead \* pd\_bad \* LGD  
>   
> ecl\_weighted = w\_good\*ecl\_good + w\_normal\*ecl\_normal + w\_bad\*ecl\_bad  
> ecl\_fl = ecl\_weighted \* impact\_pd  
>   
> results\[f'{component}\_ecl\_weighted'\] = ecl\_weighted  
> results\[f'{component}\_ecl\_fl'\] = ecl\_fl  
>   
> return results

## 4.3 Sample (BR-ECL-010 — Bank Mandiri idAAA)

Reference: SoW v1.1 §8.1.3.

| **Parameter**                         | **Nilai**                                    |
| ------------------------------------- | -------------------------------------------- |
| Cash di Bank Mandiri                  | Rp 1.500.000.000                             |
| Deposito di Bank Mandiri              | Rp 3.000.000.000                             |
| Total Eksposur                        | Rp 4.500.000.000                             |
| LPS Coverage                          | Rp 2.000.000.000                             |
| Eksposur Tak Terjamin (EAD\_Bank)     | Rp 2.500.000.000                             |
| Proporsi Cash                         | 1.500 / 4.500 = 0,3333                       |
| Proporsi Deposito                     | 3.000 / 4.500 = 0,6667                       |
| EAD Cash                              | 0,3333 × 2.500.000.000 = Rp 833.333.333,33   |
| EAD Deposito                          | 0,6667 × 2.500.000.000 = Rp 1.666.666.666,67 |
| PD Normal idAAA                       | 0,0002                                       |
| LGD Senior Unsecured Bank             | 0,4500                                       |
| ECL FL Cash (after FL adjustment)     | Rp 107.812,50                                |
| ECL FL Deposito (after FL adjustment) | Rp 215.625,00                                |

# 5\. ECL Engine — Look-through Reksadana

## 5.1 Konsep

Reksadana di-look-through ke aset underlying. Setiap underlying diperlakukan sebagai eksposur tersendiri untuk perhitungan ECL.

**Treatment per klasifikasi reksadana:**

  - Reksadana FVTPL → ECL look-through hanya risk-management view (TIDAK masuk LK).

  - Reksadana FVOCI → ECL look-through DIAKUI di LK (D Beban CKPN, K OCI CKPN).

  - Reksadana Saham (semua klasifikasi) → TIDAK ada ECL (underlying ekuitas).

  - Reksadana Campuran → ECL HANYA pada komponen non-equity; ekuitas EXCLUDED.

## 5.2 Look-through Algorithm

> function computeECLLookthrough(reksadana, evaluation\_date):  
> if reksadana.tipe == 'RDN\_SAHAM':  
> return None \# Tidak ada ECL untuk RDN saham  
>   
> \# Get latest fund composition (dari Fund Fact Sheet upload)  
> composition = getLatestFundComposition(reksadana.id, evaluation\_date)  
> \# composition = \[{underlying\_type, weight, issuer\_id\_or\_rating, ...}, ...\]  
>   
> nab\_total = reksadana.jumlah\_unit \* reksadana.nab\_terkini  
> total\_ecl\_weighted = 0  
> total\_ead\_eligible = 0  
>   
> for u in composition:  
> \# Skip equity component untuk RDN Campuran  
> if u.underlying\_type == 'EQUITY':  
> continue  
>   
> \# Compute EAD per underlying  
> ead\_underlying = nab\_total \* u.weight  
>   
> \# Lookup PD & LGD per tipe underlying  
> if u.underlying\_type == 'OBLIGASI\_PEMERINTAH':  
> pd\_normal = lookupPD('SOVEREIGN', '12M')  
> lgd = 0.4500  
> elif u.underlying\_type == 'OBLIGASI\_KORPORASI':  
> pd\_normal = lookupPD(u.rating\_pefindo, '12M')  
> lgd = lookupLGD(u.tipe\_eksposur\_basel)  
> elif u.underlying\_type == 'CASH\_BANK':  
> pd\_normal = lookupPD(u.bank\_rating, '12M')  
> lgd = 0.4500 \# NOTE: LPS tidak berlaku — ini eksposur MI ke bank  
> else:  
> pd\_normal = lookupPD(u.rating\_pefindo, '12M')  
> lgd = 0.4500 \# default  
>   
> \# Apply 3-skenario  
> impact\_mev\_good = lookupImpactMEV('GOOD', evaluation\_date)  
> impact\_mev\_bad = lookupImpactMEV('BAD', evaluation\_date)  
> pd\_good = pd\_normal \* impact\_mev\_good  
> pd\_bad = pd\_normal \* impact\_mev\_bad  
>   
> ecl\_good = ead\_underlying \* pd\_good \* lgd  
> ecl\_normal = ead\_underlying \* pd\_normal \* lgd  
> ecl\_bad = ead\_underlying \* pd\_bad \* lgd  
>   
> ecl\_weighted = 0.25\*ecl\_good + 0.50\*ecl\_normal + 0.25\*ecl\_bad  
> total\_ecl\_weighted += ecl\_weighted  
> total\_ead\_eligible += ead\_underlying  
>   
> \# Apply Impact PD  
> impact\_pd = lookupImpactPD(evaluation\_date)  
> total\_ecl\_fl = total\_ecl\_weighted \* impact\_pd  
>   
> \# Persist ke ecl\_lookthrough\_underlying records  
> return {  
> 'total\_ead\_eligible': total\_ead\_eligible,  
> 'total\_ecl\_weighted': total\_ecl\_weighted,  
> 'total\_ecl\_fl': total\_ecl\_fl,  
> 'view\_type': 'BOOK' if reksadana.klasifikasi == 'FVOCI' else 'RISK\_MGMT\_ONLY'  
> }

# 6\. ECL Engine — Job Batch Akhir Bulan

## 6.1 Job Specification

> job: ecl\_monthly\_calculation\_job  
> trigger: on-demand by Risk Officer atau scheduled D+1 setelah cut-off  
> priority: HIGH  
> sla: ≤ 4 jam untuk 1.500 instrumen, 3 skenario, 3 stage  
>   
> steps:  
> 1\. Pre-flight check:  
> a. Periode bulan terkait masih OPEN (atau SOFT\_CLOSED dengan re-run permission)  
> b. Master parameter: PD/LGD/MEV/Impact PD versi terkini ter-upload  
> c. Master Kurs: kurs period-end tersedia  
> d. Tidak ada job ECL berjalan untuk periode yang sama  
>   
> 2\. Snapshot parameter version (untuk audit re-perform):  
> \- PD Pefindo version  
> \- LGD Basel version  
> \- Impact MEV upload ID  
> \- Impact PD upload ID  
> \- Bobot skenario aktif  
>   
> 3\. Refresh staging:  
> \- Re-evaluate stage untuk semua instrumen aktif (Bab 3)  
> \- Process stage migration via STAGE\_MIGRATION events  
>   
> 4\. Compute LPS Aggregator untuk Cash + Deposito per Bank (Bab 4)  
>   
> 5\. Compute ECL per instrumen aktif (parallel processing 8-16 workers):  
> \- Stage 1 / 2 / 3 per algorithm  
> \- 3-skenario (Good/Normal/Bad)  
> \- Convert ke IDR equivalent  
> \- Apply Impact PD multiplier → ECL FL  
> \- Persist ke ecl\_calc\_header + ecl\_calc\_detail\_skenario  
>   
> 6\. Compute Reksadana Look-through (Bab 5)  
>   
> 7\. Reconciliation:  
> \- Total EAD = total carrying for AC + FVOCI debt + FVOCI reksadana  
> \- Sum ECL FL by stage matches detail records  
>   
> 8\. Generate ECL Detail Report + ECL Summary Report  
>   
> 9\. Posting Service:  
> \- Compute Δ ECL = ECL\_periode\_baru - ECL\_periode\_lama per instrumen  
> \- Bila Δ \> 0 → event ECL\_PEMBENTUKAN incremental  
> \- Bila Δ \< 0 → event ECL\_REVERSAL incremental  
> \- Bila stage berubah → STAGE\_MIGRATION event  
> \- Total impact ke P\&L (Beban CKPN) + kontra ke CKPN/OCI  
>   
> 10\. Notification: ke Risk Officer + Akuntansi + CFO  
>   
> 11\. Audit log: parameter snapshot, instrument count, total ECL,  
> runtime, errors  
>   
> failure handling:  
> \- Per-instrument failure: log + continue  
> \- Critical failure (DB error): rollback partial; alert P1  
> \- Re-run on demand: snapshot existing, run baru sebagai re-run version

# 7\. ECL Engine — Field Specs & Schema

## 7.1 ecl\_calc\_header

| **Field**                  | **Type**      | **Note**                                                                     |
| -------------------------- | ------------- | ---------------------------------------------------------------------------- |
| id                         | UUID PK       |                                                                              |
| calc\_id\_kode             | VARCHAR(20)   | ECL-YYYY-MM-\#\#\#\#\#                                                       |
| instrumen\_id FK           | UUID          | Untuk reksadana: this is fund itself; underlying detail di lookthrough table |
| periode\_id FK             | UUID          | Periode bulanan                                                              |
| evaluation\_date           | DATE          | Period end                                                                   |
| stage                      | VARCHAR(10)   | STAGE\_1/2/3                                                                 |
| pd\_horizon                | VARCHAR(10)   | 12M atau LIFETIME (3Y/5Y/7Y/10Y)                                             |
| ead\_native                | NUMERIC(20,2) | Mata uang asli                                                               |
| ead\_idr                   | NUMERIC(20,2) | IDR equivalent                                                               |
| kurs\_tengah\_bi           | NUMERIC(15,4) | Period-end rate                                                              |
| lgd                        | NUMERIC(8,4)  | Per Basel                                                                    |
| pd\_normal                 | NUMERIC(8,4)  | Pefindo                                                                      |
| impact\_mev\_good          | NUMERIC(8,4)  |                                                                              |
| impact\_mev\_bad           | NUMERIC(8,4)  |                                                                              |
| impact\_pd                 | NUMERIC(8,4)  | FL multiplier final                                                          |
| w\_good, w\_normal, w\_bad | NUMERIC(8,4)  | Bobot skenario                                                               |
| ecl\_weighted\_idr         | NUMERIC(20,2) | Computed                                                                     |
| ecl\_fl\_idr               | NUMERIC(20,2) | Final post FL                                                                |
| delta\_ecl\_fl\_idr        | NUMERIC(20,2) | vs periode sebelumnya                                                        |
| pengakuan\_lk              | VARCHAR(20)   | BOOK/RISK\_MGMT\_ONLY                                                        |
| parameter\_snapshot\_id    | UUID          | Reference ke parameter version                                               |
| jurnal\_header\_id FK      | UUID          | Reference ke jurnal yang ter-post                                            |
| calc\_run\_id              | UUID          | Reference ke job run history                                                 |
| status                     | VARCHAR(20)   | POSTED/RE\_RUN\_REQUIRED                                                     |

## 7.2 ecl\_calc\_detail\_skenario

| **Field**                | **Type**      | **Note**                          |
| ------------------------ | ------------- | --------------------------------- |
| id                       | UUID PK       |                                   |
| ecl\_calc\_header\_id FK | UUID          |                                   |
| skenario                 | VARCHAR(20)   | GOOD/NORMAL/BAD                   |
| pd\_skenario             | NUMERIC(8,4)  | Derived: PD\_Normal × Impact\_MEV |
| bobot                    | NUMERIC(8,4)  |                                   |
| ecl\_skenario\_idr       | NUMERIC(20,2) | Computed                          |

## 7.3 ecl\_lookthrough\_underlying

| **Field**                      | **Type**      | **Note**                                                       |
| ------------------------------ | ------------- | -------------------------------------------------------------- |
| id                             | UUID PK       |                                                                |
| ecl\_calc\_header\_id FK       | UUID          | Reference ke header reksadana                                  |
| underlying\_kategori           | VARCHAR(50)   | OBLIGASI\_PEMERINTAH/OBLIGASI\_KORPORASI/CASH\_BANK/EQUITY/dll |
| underlying\_issuer\_or\_rating | VARCHAR(100)  | Atau identifier                                                |
| weight                         | NUMERIC(8,4)  | Bobot dalam komposisi NAB                                      |
| ead\_underlying\_idr           | NUMERIC(20,2) |                                                                |
| pd\_normal                     | NUMERIC(8,4)  |                                                                |
| lgd                            | NUMERIC(8,4)  |                                                                |
| ecl\_weighted\_idr             | NUMERIC(20,2) |                                                                |
| excluded                       | BOOLEAN       | TRUE untuk EQUITY component (no ECL)                           |

## 7.4 Schema DDL Snippet

> CREATE TABLE ecl.calc\_header (  
> id UUID PRIMARY KEY DEFAULT uuidv7(),  
> calc\_id\_kode VARCHAR(20) NOT NULL UNIQUE,  
> instrumen\_id UUID NOT NULL REFERENCES mst.instrumen(id),  
> periode\_id UUID NOT NULL REFERENCES mst.periode\_buku(id),  
> evaluation\_date DATE NOT NULL,  
> stage VARCHAR(10) NOT NULL,  
> pd\_horizon VARCHAR(10) NOT NULL,  
> ead\_native NUMERIC(20,2) NOT NULL,  
> ead\_idr NUMERIC(20,2) NOT NULL,  
> kurs\_tengah\_bi NUMERIC(15,4),  
> lgd NUMERIC(8,4) NOT NULL,  
> pd\_normal NUMERIC(8,4) NOT NULL,  
> impact\_mev\_good NUMERIC(8,4) NOT NULL,  
> impact\_mev\_bad NUMERIC(8,4) NOT NULL,  
> impact\_pd NUMERIC(8,4) NOT NULL,  
> w\_good NUMERIC(8,4) NOT NULL,  
> w\_normal NUMERIC(8,4) NOT NULL,  
> w\_bad NUMERIC(8,4) NOT NULL,  
> ecl\_weighted\_idr NUMERIC(20,2) NOT NULL,  
> ecl\_fl\_idr NUMERIC(20,2) NOT NULL,  
> delta\_ecl\_fl\_idr NUMERIC(20,2),  
> pengakuan\_lk VARCHAR(20) NOT NULL,  
> parameter\_snapshot\_id UUID NOT NULL,  
> jurnal\_header\_id UUID REFERENCES jrnl.header(id),  
> calc\_run\_id UUID NOT NULL,  
> status VARCHAR(20) NOT NULL DEFAULT 'POSTED',  
> created\_at TIMESTAMPTZ NOT NULL DEFAULT now(),  
> CONSTRAINT ck\_bobot\_sum CHECK (w\_good + w\_normal + w\_bad BETWEEN 0.9999 AND 1.0001),  
> CONSTRAINT uq\_periode\_instrumen UNIQUE (periode\_id, instrumen\_id, calc\_run\_id)  
> ) PARTITION BY LIST (periode\_id);  
>   
> CREATE INDEX ix\_ecl\_calc\_periode ON ecl.calc\_header(periode\_id);  
> CREATE INDEX ix\_ecl\_calc\_instrumen ON ecl.calc\_header(instrumen\_id);  
> CREATE INDEX ix\_ecl\_calc\_eval\_date ON ecl.calc\_header(evaluation\_date);

# 8\. EIR Engine — Newton-Raphson IRR Solver

## 8.1 Implementation Spec

Reference: SoW v1.1 §5.12.4. EIR adalah r yang memenuhi P0 = Σ CFt/(1+r)^t.

> function computeEIR(P0, cash\_flows, max\_iter=50, tolerance=1e-8):  
> \# P0: Carrying Amount Awal (harga + biaya transaksi capitalized)  
> \# cash\_flows: list of (period, amount) — coupons + final principal  
> \# Returns: r per period, atau raises EIR\_NOT\_CONVERGED  
>   
> \# Initial guess: kupon kontraktual / frekuensi  
> r = (sum(cf for \_, cf in cash\_flows) / P0 - 1) / len(cash\_flows)  
> if r \<= 0:  
> r = 0.01 \# 1% per period as fallback initial  
>   
> for iteration in range(max\_iter):  
> \# f(r) = P0 - Σ CFt/(1+r)^t  
> f\_r = P0 - sum(cf / (1+r)\*\*t for t, cf in cash\_flows)  
>   
> \# f'(r) = Σ t × CFt / (1+r)^(t+1)  
> f\_prime\_r = sum(t \* cf / (1+r)\*\*(t+1) for t, cf in cash\_flows)  
>   
> if abs(f\_r) \< tolerance:  
> return r \# converged  
>   
> if f\_prime\_r == 0:  
> \# Avoid division by zero — switch ke bisection  
> return bisectionSolveEIR(P0, cash\_flows)  
>   
> \# Newton-Raphson update  
> r\_new = r - f\_r / f\_prime\_r  
>   
> \# Sanity check: r dalam range wajar  
> if r\_new \<= -0.99 or r\_new \>= 1.0:  
> return bisectionSolveEIR(P0, cash\_flows)  
>   
> r = r\_new  
>   
> \# Did not converge in max\_iter  
> raise EIRNotConvergedException(f"Failed after {max\_iter} iterations")  
>   
>   
> function bisectionSolveEIR(P0, cash\_flows, low=-0.99, high=1.0):  
> \# Fallback solver — slower but more robust  
> for \_ in range(100):  
> mid = (low + high) / 2  
> f\_mid = P0 - sum(cf / (1+mid)\*\*t for t, cf in cash\_flows)  
> if abs(f\_mid) \< 1e-8:  
> return mid  
> f\_low = P0 - sum(cf / (1+low)\*\*t for t, cf in cash\_flows)  
> if f\_low \* f\_mid \< 0:  
> high = mid  
> else:  
> low = mid  
> raise EIRNotConvergedException("Bisection did not converge")  
>   
>   
> function annualizeEIR(r\_per\_period, frequency\_per\_year):  
> return (1 + r\_per\_period) \*\* frequency\_per\_year - 1

## 8.2 Sample Computation (SoW §5.12.11)

Obligasi PT XYZ, AC, Premium:

> Input:  
> Nominal (Par) = 5,000,000,000  
> Harga Beli = 101.5% → 5,075,000,000  
> Biaya Transaksi = 5,000,000  
> Carrying Awal (P0) = 5,080,000,000  
> Kupon = 5.00% pa, semesteran  
> Tenor = 5 tahun = 10 periode kupon  
>   
> Cash Flows:  
> CF\[1..9\] = (5% × 5,000,000,000) / 2 = 125,000,000 (kupon semesteran)  
> CF\[10\] = 125,000,000 + 5,000,000,000 = 5,125,000,000 (kupon + pokok)  
>   
> Solver:  
> Initial r = (sum(CF)/P0 - 1) / N = ((1,125 + 5,125)M / 5,080M - 1) / 10 ≈ 0.023  
>   
> Iteration 1: r=0.023, f=..., r\_new=0.0238...  
> Iteration 2: r=0.0238, f=..., r\_new=0.02384...  
> Iteration 3: r=0.02384, f=..., r\_new=0.0238492  
> Iteration 4: r=0.0238492, f=..., r\_new=0.02384921  
> Iteration 5: |f| \< 1e-8 → CONVERGED  
>   
> r\_per\_semester = 0.02384921  
> EIR Annualized = (1 + 0.02384921)^2 - 1 = 0.04826688 ≈ 4.8267%

## 8.3 Day Count Convention

| **Convention**    | **Formula**                                     | **Use Case**                   |
| ----------------- | ----------------------------------------------- | ------------------------------ |
| ACT/365 (default) | (actual days) / 365                             | Standard, simple               |
| ACT/360           | (actual days) / 360                             | Money market                   |
| 30/360            | 30 × full months + day count / 360              | Some legacy bond conventions   |
| ACT/ACT           | (actual days in period) / (actual days in year) | Most accurate; some govt bonds |

# 9\. EIR Engine — Amortization Schedule

## 9.1 Generation Logic

> function generateAmortizationSchedule(instrumen):  
> if instrumen.klasifikasi not in ('AC', 'FVOCI') or instrumen.eir\_method\_flag \!= True:  
> return \# Tidak generate untuk FVTPL / FVOCI Election  
>   
> \# Compute cash flows kontraktual  
> cash\_flows = \[\]  
> n\_periods = computeNumPeriods(instrumen) \# tenor / frekuensi  
> coupon\_per\_period = (instrumen.kupon \* instrumen.nominal) / instrumen.frekuensi\_per\_year  
> payment\_dates = computePaymentDates(instrumen)  
>   
> for t in range(1, n\_periods + 1):  
> if t \< n\_periods:  
> cf = coupon\_per\_period  
> else:  
> cf = coupon\_per\_period + instrumen.nominal \# last: kupon + pokok  
> cash\_flows.append((t, cf))  
>   
> \# Compute EIR  
> P0 = instrumen.harga\_beli + instrumen.biaya\_transaksi\_capitalized  
> r\_per\_period = computeEIR(P0, cash\_flows)  
> eir\_annualized = annualizeEIR(r\_per\_period, instrumen.frekuensi\_per\_year)  
>   
> \# Generate schedule rows  
> opening\_carrying = P0  
> schedule\_rows = \[\]  
> for t, cf in cash\_flows:  
> \# Pendapatan bunga EIR per periode  
> pendapatan\_bunga\_eir = opening\_carrying \* r\_per\_period  
>   
> \# Amortisasi premium (negatif jika P0 \> Par) atau diskonto (positif)  
> amortisasi\_pd = pendapatan\_bunga\_eir - cf  
> \# Note: untuk last period, cf includes pokok — perlu adjustment  
> if t == n\_periods:  
> pelunasan\_pokok = instrumen.nominal  
> cf\_minus\_pokok = cf - instrumen.nominal  
> pendapatan\_bunga\_eir = opening\_carrying \* r\_per\_period  
> amortisasi\_pd = pendapatan\_bunga\_eir - cf\_minus\_pokok  
> closing\_carrying = opening\_carrying + amortisasi\_pd - pelunasan\_pokok  
> else:  
> pelunasan\_pokok = 0  
> closing\_carrying = opening\_carrying + amortisasi\_pd  
>   
> schedule\_rows.append({  
> 'periode': t,  
> 'tanggal\_posting': payment\_dates\[t-1\],  
> 'opening\_carrying': opening\_carrying,  
> 'cash\_inflow': cf,  
> 'pendapatan\_bunga\_eir': pendapatan\_bunga\_eir,  
> 'amortisasi\_p\_d': amortisasi\_pd,  
> 'pelunasan\_pokok': pelunasan\_pokok,  
> 'closing\_carrying': closing\_carrying,  
> 'eir\_periode': eir\_annualized,  
> 'stage\_saat\_posting': 'STAGE\_1', \# default at origination  
> 'status\_posting': 'PROYEKSI'  
> })  
> opening\_carrying = closing\_carrying  
>   
> \# Validate: final closing\_carrying ≈ 0 (toleransi 0.01 IDR)  
> if abs(schedule\_rows\[-1\]\['closing\_carrying'\]) \> 0.01:  
> raise InvalidScheduleException("Final carrying \!= 0 (par discharge)")  
>   
> \# Persist ke eir\_amortization\_schedule  
> persistSchedule(instrumen.id, schedule\_rows)  
>   
> \# Update Master Instrumen dengan EIR computed  
> instrumen.eir\_awal = eir\_annualized  
> instrumen.tanggal\_eir\_computed = today()  
> instrumen.premium\_diskonto\_awal = P0 - instrumen.nominal \# positif=premium, negatif=diskonto

## 9.2 Sample Schedule (SoW §5.12.11)

3 periode pertama dari schedule obligasi PT XYZ:

| **Periode** | **Tanggal** | **Opening Carrying** | **Cash Kupon** | **Pend Bunga EIR** | **Amortisasi P/D** | **Closing Carrying** |
| ----------- | ----------- | -------------------- | -------------- | ------------------ | ------------------ | -------------------- |
| 1           | 30/06/2026  | 5.080.000.000        | 125.000.000    | 121.154.001        | (3.845.999)        | 5.076.154.001        |
| 2           | 31/12/2026  | 5.076.154.001        | 125.000.000    | 121.062.286        | (3.937.714)        | 5.072.216.287        |
| 3           | 30/06/2027  | 5.072.216.287        | 125.000.000    | 120.968.385        | (4.031.615)        | 5.068.184.672        |
| ...         | ...         | ...                  | ...            | ...                | ...                | ...                  |
| 10          | 31/12/2030  | 5.004.762.137        | 5.125.000.000  | 120.237.863        | (4.762.137)        | 0 (par dilunasi)     |

## 9.3 Field Specs (eir\_amortization\_schedule)

| **Field**                | **Type**      | **Note**                                         |
| ------------------------ | ------------- | ------------------------------------------------ |
| id                       | UUID PK       |                                                  |
| schedule\_id\_kode       | VARCHAR(30)   | AMSCH-{kode\_instrumen}-{seq}                    |
| instrumen\_id FK         | UUID          |                                                  |
| periode                  | INT           | 1, 2, 3, ...                                     |
| tanggal\_posting         | DATE          |                                                  |
| opening\_carrying        | NUMERIC(20,2) | \= closing periode sebelumnya                    |
| cash\_inflow             | NUMERIC(20,2) | Kupon kontraktual                                |
| pendapatan\_bunga\_eir   | NUMERIC(20,2) | Carrying × EIR/freq                              |
| amortisasi\_p\_d         | NUMERIC(20,2) | \= EIR - cash; negatif=premium, positif=diskonto |
| pelunasan\_pokok         | NUMERIC(20,2) | Default 0; = nominal saat last period            |
| closing\_carrying        | NUMERIC(20,2) | \= opening + amort - pelunasan                   |
| eir\_periode             | NUMERIC(12,8) | EIR yang berlaku                                 |
| stage\_saat\_posting     | VARCHAR(10)   | STAGE\_1/2/3 — relevant untuk Net Carrying basis |
| status\_posting          | VARCHAR(20)   | PROYEKSI/POSTED/REVERSED/RECOMPUTED              |
| jurnal\_reference\_id FK | UUID          | Reference ke jurnal AKRUAL\_BUNGA + AMORTISASI   |

# 10\. EIR Engine — Re-estimation

## 10.1 Two Scenarios

**A. Modifikasi Material (PSAK 71 §3.3.2)**

Trigger: perubahan kupon ≥ 10%, perpanjangan tenor signifikan, perubahan currency, atau perubahan counterparty.

Treatment: derecognition aset asli + recognition aset baru pada nilai wajar. EIR aset baru dihitung ulang dari awal.

> function processModifikasiMaterial(instrumen\_lama, modifikasi\_term\_baru):  
> \# 1. Derecognize aset asli  
> fair\_value\_now = computeFairValue(instrumen\_lama, today())  
> carrying = getCurrentCarrying(instrumen\_lama)  
> realized\_gl = fair\_value\_now - carrying  
>   
> \# Posting:  
> \# D Investasi Baru = fair\_value\_now  
> \# K Investasi Lama = carrying  
> \# D/K Realized Gain/Loss (P\&L untuk AC; OCI untuk FVOCI) = realized\_gl  
> postJurnal('MODIFIKASI\_MATERIAL', ...)  
>   
> \# 2. Create instrumen baru dengan term modifikasi  
> instrumen\_baru = createMasterInstrumen(modifikasi\_term\_baru)  
> instrumen\_baru.harga\_beli = fair\_value\_now \# carrying baru = FV  
> instrumen\_baru.biaya\_transaksi\_capitalized = 0 \# no new transaction cost  
>   
> \# 3. Compute EIR baru dari fair\_value\_now  
> instrumen\_baru.eir\_awal = computeEIR(fair\_value\_now, new\_cash\_flows)  
> generateAmortizationSchedule(instrumen\_baru)  
>   
> \# 4. Status update  
> instrumen\_lama.status = 'REKLASIFIKASI' \# atau MODIFIED

**B. Revisi Estimasi Cash Flow (PSAK 71 §B5.4.6)**

Trigger: revisi prepayment assumption, revisi step-up rate trigger, perubahan tidak material.

Treatment: EIR original tetap dipakai. Recompute carrying = Σ revised\_CFt/(1+EIR\_original)^t. Selisih dengan carrying lama = catch-up adjustment.

> function processRevisiCashFlow(instrumen, revised\_cash\_flows):  
> \# 1. EIR original tetap  
> eir\_original = instrumen.eir\_awal  
>   
> \# 2. Recompute carrying dengan revised CF  
> new\_carrying = sum(cf / (1 + eir\_original)\*\*t for t, cf in revised\_cash\_flows)  
> old\_carrying = getCurrentCarrying(instrumen)  
> catch\_up\_adjustment = new\_carrying - old\_carrying  
>   
> \# 3. Posting catch-up  
> if catch\_up\_adjustment \!= 0:  
> \# D/K Investasi = catch\_up\_adjustment  
> \# K/D Penyesuaian Pendapatan Bunga (P\&L) = catch\_up\_adjustment  
> postJurnal('EIR\_REESTIMATION', catch\_up\_adjustment)  
>   
> \# 4. Regenerate amortization schedule dari periode aktif ke depan  
> regenerateScheduleFromPeriod(instrumen, current\_period, revised\_cash\_flows, eir\_original)  
>   
> \# 5. Log re-estimation event  
> insertReestimationLog({  
> instrumen\_id: instrumen.id,  
> eir\_sebelum: eir\_original,  
> eir\_sesudah: eir\_original, \# tetap  
> carrying\_sebelum: old\_carrying,  
> carrying\_sesudah: new\_carrying,  
> catch\_up\_adjustment: catch\_up\_adjustment,  
> trigger\_type: 'REVISI\_CASH\_FLOW',  
> ...  
> })

## 10.2 Field Specs (eir\_reestimation\_log)

| **Field**               | **Type**      | **Note**                                                             |
| ----------------------- | ------------- | -------------------------------------------------------------------- |
| id                      | UUID PK       |                                                                      |
| log\_id\_kode           | VARCHAR(30)   | EIR-RE-YYYY-\#\#\#\#\#                                               |
| instrumen\_id FK        | UUID          |                                                                      |
| tanggal\_re\_estimation | DATE          |                                                                      |
| trigger\_type           | VARCHAR(50)   | MODIFIKASI\_MATERIAL/REVISI\_CASH\_FLOW/PREPAYMENT/STEP\_UP\_TRIGGER |
| eir\_sebelum            | NUMERIC(12,8) |                                                                      |
| eir\_sesudah            | NUMERIC(12,8) | Same as before untuk REVISI\_CASH\_FLOW                              |
| carrying\_sebelum       | NUMERIC(20,2) |                                                                      |
| carrying\_sesudah       | NUMERIC(20,2) |                                                                      |
| catch\_up\_adjustment   | NUMERIC(20,2) | Δ posted to P\&L                                                     |
| modifikasi\_terms\_json | JSONB         | Detail term changes                                                  |
| dokumen\_pendukung\_url | VARCHAR(500)  | Amandemen kontrak                                                    |
| maker\_id FK            | UUID          |                                                                      |
| reviewer\_id FK         | UUID          | Akuntansi                                                            |
| approver\_id FK         | UUID          | Finance Controller / CFO                                             |
| status                  | VARCHAR(30)   |                                                                      |
| jurnal\_header\_id FK   | UUID          |                                                                      |

# 11\. EIR Engine — Stage 3 (Net Carrying) Handling

PSAK 71 §5.4.1(b): saat instrumen migrasi ke Stage 3 (credit-impaired), pendapatan bunga = Net Carrying × EIR (bukan Gross).

## 11.1 Algorithm

> function getCurrentCarryingForBunga(instrumen, evaluation\_date):  
> gross\_carrying = getGrossCarrying(instrumen, evaluation\_date)  
> stage = getCurrentStage(instrumen, evaluation\_date)  
>   
> if stage in ('STAGE\_1', 'STAGE\_2'):  
> return gross\_carrying  
>   
> elif stage == 'STAGE\_3':  
> \# Net Carrying = Gross - CKPN  
> ckpn = getCKPNBalance(instrumen, evaluation\_date)  
> net\_carrying = gross\_carrying - ckpn  
> return net\_carrying  
>   
>   
> function dailyAccrual(instrumen, date):  
> eir = instrumen.eir\_awal  
> carrying = getCurrentCarryingForBunga(instrumen, date)  
> accrual = carrying \* eir / 365 \# ACT/365  
>   
> \# Posting akrual ke P\&L (Pendapatan Bunga)  
> postJurnal('AKRUAL\_BUNGA', amount=accrual)  
>   
> \# Update amortization schedule status untuk periode ini  
> updateScheduleRow(instrumen.id, current\_periode, status='POSTED')

## 11.2 Curing — Migrasi Stage 3 → Stage 2

Setelah probationary period 3-6 bulan dengan kondisi normal:

27. Risk Officer submit case curing dengan dokumen.

28. Komite Risiko approve.

29. Sistem update Stage History: STAGE\_3 → STAGE\_2.

30. Basis bunga kembali ke Gross Carrying.

31. ECL re-calc dengan Lifetime PD (Stage 2 basis).

32. Audit trail: justifikasi, dokumen, approver.

# 12\. EIR Engine — Reklasifikasi Handling

Reference: SoW v1.1 §5.12.10. Treatment EIR per kombinasi reklasifikasi:

| **From → To** | **EIR Treatment**                                                                  |
| ------------- | ---------------------------------------------------------------------------------- |
| AC → FVOCI    | EIR original tetap; selisih FV vs amortized carrying ke OCI                        |
| AC → FVTPL    | EIR & Schedule deactivated; FV-based                                               |
| FVOCI → AC    | EIR baru dihitung dari FV pada tanggal reklas; OCI cumulative dieliminasi          |
| FVOCI → FVTPL | EIR & Schedule deactivated; OCI cumulative direklas ke P\&L                        |
| FVTPL → AC    | EIR baru dihitung dari FV; schedule baru di-generate                               |
| FVTPL → FVOCI | EIR baru dihitung dari FV; schedule baru di-generate; OCI accumulated mulai dari 0 |

## 12.1 Re-EIR Logic untuk Reklasifikasi ke AC/FVOCI

> function recomputeEIROnReklas(instrumen, klasifikasi\_baru, tanggal\_efektif):  
> if klasifikasi\_baru in ('AC', 'FVOCI'):  
> fair\_value\_efektif = getFairValue(instrumen, tanggal\_efektif)  
> remaining\_cash\_flows = getRemainingCashFlows(instrumen, tanggal\_efektif)  
>   
> \# Compute EIR baru  
> new\_eir = computeEIR(fair\_value\_efektif, remaining\_cash\_flows)  
> new\_eir\_annualized = annualizeEIR(new\_eir, instrumen.frekuensi\_per\_year)  
>   
> \# Update master  
> instrumen.eir\_awal = new\_eir\_annualized  
> instrumen.tanggal\_eir\_computed = tanggal\_efektif  
>   
> \# Re-generate schedule dari tanggal\_efektif ke depan  
> regenerateAmortizationSchedule(instrumen, fair\_value\_efektif, tanggal\_efektif)  
>   
> else: \# FVTPL  
> deactivateAmortizationSchedule(instrumen)  
> instrumen.eir\_awal = NULL \# tidak relevan untuk FVTPL

# 13\. EIR Engine — Field Specs (Master Instrumen)

Reference: FSD Appendix A §1.1.3 untuk full field specs Master Instrumen. Field EIR-related:

| **Field**                   | **DB Column**                 | **Type**      | **Note**                                           |
| --------------------------- | ----------------------------- | ------------- | -------------------------------------------------- |
| EIR Awal                    | eir\_awal                     | NUMERIC(12,8) | Annualized EIR; 8 desimal internal                 |
| Tanggal EIR Computed        | tanggal\_eir\_computed        | DATE          | Saat awal atau saat re-estimation                  |
| Premium/Diskonto Awal       | premium\_diskonto\_awal       | NUMERIC(20,2) | \= P0 - nominal; positif=premium, negatif=diskonto |
| Biaya Transaksi Capitalized | biaya\_transaksi\_capitalized | NUMERIC(20,2) | Untuk AC/FVOCI utang; ke P\&L untuk FVTPL          |
| EIR Method Flag             | eir\_method\_flag             | BOOLEAN       | Y = EIR-based accrual; N = simple interest         |
| Day Count Convention        | day\_count\_convention        | VARCHAR(10)   | ACT/365, ACT/360, 30/360, ACT/ACT                  |
| Amortization Frequency      | amortization\_frequency       | VARCHAR(20)   | BULANAN/TRIWULANAN/SEMESTERAN/TAHUNAN              |

# 14\. API Endpoints (ECL & EIR)

## 14.1 ECL Engine APIs

| **Method** | **Endpoint**                                 | **Permission**                               |
| ---------- | -------------------------------------------- | -------------------------------------------- |
| POST       | /api/v1/ecl/calculate                        | ecl.calculate (manual trigger; rate-limited) |
| GET        | /api/v1/ecl/calculate/{calc\_run\_id}        | ecl.read (status & progress)                 |
| GET        | /api/v1/ecl/calc-header?periode=\&instrumen= | ecl.read                                     |
| GET        | /api/v1/ecl/calc-header/{id}/detail          | ecl.read (per-skenario detail)               |
| GET        | /api/v1/ecl/calc-header/{id}/lookthrough     | ecl.read (untuk reksadana)                   |
| GET        | /api/v1/ecl/stage-history/{instrumen\_id}    | ecl.read                                     |
| POST       | /api/v1/ecl/stage-migration/curing           | ecl.curing (Risk Officer submit case)        |
| POST       | /api/v1/ecl/stage-migration/{id}/approve     | ecl.curing.approve (Komite Risiko)           |
| GET        | /api/v1/ecl/parameter-snapshot/{id}          | ecl.read (audit re-perform)                  |
| POST       | /api/v1/ecl/recalculate-on-rating-change     | ecl.recalculate (event-driven)               |

## 14.2 EIR Engine APIs

| **Method** | **Endpoint**                                   | **Permission**                                                           |
| ---------- | ---------------------------------------------- | ------------------------------------------------------------------------ |
| POST       | /api/v1/eir/compute                            | eir.compute (auto-triggered saat penempatan; manual trigger via API)     |
| GET        | /api/v1/eir/instrumen/{id}/schedule            | eir.read (full amortization schedule)                                    |
| GET        | /api/v1/eir/instrumen/{id}/schedule?from=\&to= | eir.read (range filtering)                                               |
| POST       | /api/v1/eir/re-estimation                      | eir.re\_estimate (submit case)                                           |
| POST       | /api/v1/eir/re-estimation/{id}/approve         | eir.re\_estimate.approve                                                 |
| GET        | /api/v1/eir/re-estimation-log?instrumen=       | eir.read                                                                 |
| POST       | /api/v1/eir/preview                            | eir.preview (compute EIR tanpa persist — untuk preview di UI penempatan) |
| GET        | /api/v1/eir/instrumen/{id}/current-carrying    | eir.read (gross atau net per stage)                                      |

# Sign-Off Page

Appendix C compliance-critical. Sign-off REQUIRED dari DSAK-certified accountant + Direktur Risk + Direktur IT + CFO.

**Disusun oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Solution Designer                                                    | Senior Risk Analyst (DSAK-certified)                                 |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Disetujui oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Direktur Risk Management                                             | Direktur Keuangan (CFO)                                              |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR APPENDIX C ---*
