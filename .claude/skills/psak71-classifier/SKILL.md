---
name: psak71-classifier
description: Lookup matrix SPPI × Business Model untuk klasifikasi instrumen ke AC / FVOCI debt / FVTPL / FVOCI equity (Election). Gunakan saat membutuhkan deterministik klasifikasi PSAK 71 dari hasil test SPPI dan BM assessment.
---

# PSAK 71 Classifier

## Input
- `sppi_result`: `PASS` | `FAIL`
- `bm_category`: `HTC` (Hold-to-Collect) | `HTC_AND_SELL` | `OTHER`
- `instrument_type`: `DEBT` | `EQUITY` | `MIXED` (hybrid)
- `fvoci_election`: `bool` (hanya untuk EQUITY; irrevocable on first recognition)

## Decision matrix

### Debt instruments (obligasi, deposito, pinjaman, dst)

| SPPI | BM | Klasifikasi |
|---|---|---|
| PASS | HTC | **AC** (Amortised Cost) |
| PASS | HTC_AND_SELL | **FVOCI debt** |
| PASS | OTHER | **FVTPL** |
| FAIL | * | **FVTPL** |

### Equity instruments (saham, reksadana saham)

| FVOCI Election | Klasifikasi |
|---|---|
| `false` (default) | **FVTPL** |
| `true` (irrevocable) | **FVOCI equity** — no recycling to P&L on disposal |

### Hybrid / contractually-linked
- Separasi: host (debt-like) + embedded derivative.
- Test host pakai matrix Debt di atas.
- Embedded derivative selalu **FVTPL** kecuali closely-related (cek FSD-APP-A §SPPI-Q5).

## Output schema
```json
{
  "klasifikasi": "AC | FVOCI_DEBT | FVOCI_EQUITY | FVTPL",
  "ecl_required": true | false,
  "rationale": [
    "SPPI PASS → cashflow karakteristik debt",
    "BM HTC → koleksi cashflow kontraktual",
    "→ AC"
  ],
  "decision_inputs": {
    "sppi_result": "PASS",
    "bm_category": "HTC",
    "instrument_type": "DEBT",
    "fvoci_election": null
  }
}
```

## ECL requirement per klasifikasi
| Klasifikasi | ECL diakui? | Dasar ECL |
|---|---|---|
| AC | YES | Carrying gross |
| FVOCI debt | YES (di P&L, tapi gross di OCI) | Carrying gross |
| FVOCI equity | NO | — |
| FVTPL | NO | — |

## Bunga (interest revenue) per klasifikasi & stage
| Klasifikasi | Stage 1 / 2 | Stage 3 |
|---|---|---|
| AC | Gross × EIR | **Net** × EIR |
| FVOCI debt | Gross × EIR | **Net** × EIR |
| FVOCI equity | n/a (dividend, bukan bunga) | n/a |
| FVTPL | n/a (mark-to-market) | n/a |

## Reklasifikasi
- **Triggered hanya** oleh perubahan **Business Model** (jarang).
- Prospective only — tidak adjust periode lalu.
- Jurnal transition matrix di `docs/jurnal/reklasifikasi-matrix.md` (data-modeler maintain).
- Workflow approval: **6-eyes** (Maker + Reviewer + RISK + KOMITE).

## Cara dipanggil dari agent
- `business-analyst` saat menulis story klasifikasi instrumen baru
- `ecl-eir-engineer` saat compute ECL — cek `ecl_required` dulu
- `ifrs9-compliance-reviewer` saat verifikasi klasifikasi result
- `system-analyst` saat design API klasifikasi

## Pseudo-code (Go)
```go
func ClassifyInstrument(in ClassificationInput) Classification {
    if in.InstrumentType == InstrumentEquity {
        if in.FVOCIElection {
            return Classification{Class: FVOCI_EQUITY, ECLRequired: false}
        }
        return Classification{Class: FVTPL, ECLRequired: false}
    }
    if in.SPPIResult == SPPI_FAIL {
        return Classification{Class: FVTPL, ECLRequired: false}
    }
    switch in.BMCategory {
    case BM_HTC:
        return Classification{Class: AC, ECLRequired: true}
    case BM_HTC_AND_SELL:
        return Classification{Class: FVOCI_DEBT, ECLRequired: true}
    case BM_OTHER:
        return Classification{Class: FVTPL, ECLRequired: false}
    }
    return Classification{}, fmt.Errorf("invalid input combination")
}
```

## Citation
- FSD-APP-A-MasterData-SPPI-BM-v1.1.docx §SPPI + §BM
- PSAK 71 §4.1, §5.7.5 (FVOCI Election)
- SoW_v1.4.docx §3 (klasifikasi matrix)
