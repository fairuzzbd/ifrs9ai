# Formula Reference — ECL, EIR, Staging

## ECL per instrument per periode

```
# Step 1 — per skenario (Good, Normal, Bad)
ECL_skenario = EAD_IDR × PD_skenario × LGD

# Step 2 — apply dual forward-looking multiplier
ECL_FL_skenario = ECL_skenario × Impact_PD_multiplier_skenario

# Step 3 — weighted aggregate
ECL_weighted = Σ (ECL_FL_skenario × bobot_skenario)
             = ECL_FL_Good   × W_Good
             + ECL_FL_Normal × W_Normal
             + ECL_FL_Bad    × W_Bad

# Default bobot (ALCO dapat override)
W_Good = 0.25,  W_Normal = 0.50,  W_Bad = 0.25
constraint: W_Good + W_Normal + W_Bad = 1.0
```

### Staging-aware PD
- **Stage 1**: PD_12M (12-month probability of default)
- **Stage 2**: PD_Lifetime (sampai jatuh tempo)
- **Stage 3**: PD = 1.0 (sudah default)

### Bunga (interest revenue)
| Stage | Base |
|---|---|
| 1 | Gross Carrying Amount × EIR |
| 2 | Gross Carrying Amount × EIR |
| 3 | **Net Carrying Amount** × EIR  *(Gross − ECL)* |

## EAD (Exposure at Default)
```
EAD = Outstanding_Principal + Accrued_Interest + (Committed_Undrawn × CCF)
```
CCF = Credit Conversion Factor (Basel-style untuk komitmen yang belum di-drawdown).

## Multi-currency EAD
```
EAD_IDR = EAD_FCY × FX_rate_BI_JISDOR(tanggal_assessment)
```

## SICR Triggers (Stage 1 → Stage 2)
Triggered jika **salah satu** terpenuhi:
- Rating turun ≥ 2 notch sejak inisiasi
- Rating berubah dari Investment Grade (IG) ke non-IG
- DPD (Days Past Due) ≥ 30 hari

## Cure (Stage 2 → Stage 1)
- 3 bulan berturut-turut memenuhi cure criteria
- History downgrade tetap disimpan untuk audit trail

## EIR — Newton-Raphson IRR Solver

Mencari `r` (effective interest rate per periode) sedemikian rupa:
```
Σ CF_t / (1 + r)^t = -CF_0    (PV cashflow = initial outflow)
```

Iterasi Newton-Raphson:
```
r_{n+1} = r_n − f(r_n) / f'(r_n)

f(r)  = Σ CF_t / (1 + r)^t
f'(r) = − Σ t × CF_t / (1 + r)^(t+1)
```

**Parameter wajib:**
- `tolerance = 1e-10`
- `max_iter = 100`
- `r_initial = 0.1` (10%, atau kupon rate sebagai seed yang lebih baik)
- Fail-safe: jika non-convergence atau divergence → error explicit, jangan return garbage.

**Re-estimation pada amendment:**
- Insert new row di `ecl.amortisasi_schedule` dengan `schedule_version` baru
- `effective_from` = tanggal amandemen
- `effective_to` = `'infinity'`
- Update row sebelumnya: `effective_to` = tanggal amandemen
- **NEVER UPDATE** existing schedule rows (audit-grade immutability)

## POCI (Purchased or Originated Credit Impaired)
- Cashflow estimate sudah PD-adjusted sejak inisiasi
- EIR yang dihitung = **credit-adjusted EIR**
- ECL movement direkognisi langsung di P&L (tidak ada Stage 1)

## LPS Aggregator (Cash + Deposito)
```
For each (nasabah, bank):
    total_exposure = Σ (saldo_cash + saldo_deposito_di_bank_tsb)
    covered       = min(total_exposure, IDR 2_000_000_000)
    excess        = total_exposure − covered

ECL_LPS = ECL_calc(excess only, PD_bank, LGD_bank)
covered → ECL = 0 (dijamin LPS)
```

## Look-through ECL (Reksadana)
```
For each Reksadana fund:
    Read komposisi underlying (% per asset class)
    For each underlying asset class:
        ECL_class = ECL_calc(NAB × %class, PD_class, LGD_class)
    ECL_reksadana = Σ ECL_class
```

## Roll-forward (untuk audit & laporan)
```
ECL_closing = ECL_opening
            + Σ Transfers_to_stage(Stage_1→2, 2→3, etc.)
            − Σ Transfers_from_stage(Stage_2→1, 3→2, etc.)
            + Σ New_originations
            − Σ Derecognitions
            ± Σ Remeasurements
```
**Wajib reconcile** ke laporan posisi.

## Presisi & Rounding
- Semua compute pakai `shopspring/decimal.Decimal` di Go
- Storage: `NUMERIC(20,4)` IDR, `NUMERIC(20,8)` FX, `NUMERIC(10,8)` PD/LGD/EIR
- Rounding: HALF_EVEN (banker's rounding) per spec
- Dokumentasi rounding **per langkah** di code comment

## Citation
Lihat SoW_v1.4.docx §4 (ECL formula), FSD-APP-C-ECL-EIR-v1.0.docx §3 (staging) & §4 (EIR), Pefindo_Annual_Default_Study (PD kalibrasi).
