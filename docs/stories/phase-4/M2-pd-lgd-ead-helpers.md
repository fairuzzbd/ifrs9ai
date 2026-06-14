# P4-M2 — PD/LGD/EAD/CCF Lookup Helpers: User Stories

**Story Set ID**: P4-M2
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 1)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-11
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (ECL Parameters), §4 (EAD computation)
**Linked BRD**: BRD §8.3 (ECL Requirements), RACI: ROLE-RISK (R), ROLE-ALCO (A), ROLE-AKUN-CTL (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-010 — ECL formula 3-stage × 3-skenario × dual FL; Stage 3 PD = 1.0
- DEC-011 — SICR triggers (tidak langsung dipakai di M2, tapi PD stage-awareness bergantung padanya)
- DEC-016 — NUMERIC precision: PD/LGD `NUMERIC(10,8)`, IDR `NUMERIC(20,4)`, FX `NUMERIC(20,8)`
- DEC-017 — Tidak ada workflow baru di M2 (read-only helpers); parameter master sudah punya workflow sendiri dari Phase 3

**Scope modul**: Read-only helper service `internal/ecl/params` — menyediakan fungsi lookup dan kalkulasi PD, LGD, EAD, CCF untuk dikonsumsi oleh ECL core engine (P4-M7) dan UI preview (ROLE-RISK). Modul ini TIDAK melakukan kalkulasi ECL akhir.

**Handoff berikutnya**: `system-analyst` (Go interface contract + endpoint contract `GET /ecl/params/preview`), lalu `ecl-eir-engineer` (implementasi domain logic di `backend/internal/ecl/params/`)

---

## Konteks & Dependensi

### Phase 3 master parameter yang dikonsumsi P4-M2

| Tabel | Kolom Kunci | Catatan |
|---|---|---|
| `mst.pd_pefindo` | `rating`, `pd_12month NUMERIC(10,8)`, `pd_lifetime_3y/5y/7y/10y NUMERIC(10,8)`, `workflow_status`, `periode_berlaku_dari`, `periode_berlaku_sampai` | Migration 0013 + 0014. Hanya row `workflow_status = 'APPROVED'` yang dipakai. |
| `mst.lgd_basel` | `tipe_eksposur`, `lgd NUMERIC(10,8)`, `karakteristik`, `periode_berlaku_dari`, `periode_berlaku_sampai`, `workflow_status` | Migration 0010. Hanya row `APPROVED`. |
| `mst.bobot_skenario` | `skenario IN ('GOOD','NORMAL','BAD')`, `bobot NUMERIC(10,8)`, `periode_berlaku_dari`, `periode_berlaku_sampai` | Migration 0011. Default 0.25/0.50/0.25. |
| `mst.impact_mev_pd` | `skenario IN ('GOOD','BAD')`, `impact_multiplier NUMERIC(10,8)`, `periode_id` | Migration 0015. |
| `mst.impact_pd` | `impact_multiplier NUMERIC(10,8)` (flat, per periode), `periode_id` | Migration 0015. |
| `mst.kurs` | `kode_mata_uang`, `nilai_kurs NUMERIC(20,8)`, `sumber_kurs = 'BI_JISDOR'`, `tanggal_berlaku` | Migration 0020. |
| `mst.instrumen` | `id`, `tipe_instrumen`, `mata_uang`, `nominal`, `klasifikasi_psak71`, `counterparty_id`, `tanggal_jatuh_tempo` | Init schema 0001 + 0019. |
| `mst.counterparty` | `id`, `tipe_eksposur_lgd` (atau derivasi dari `tipe_counterparty`) | Init schema 0001. |
| `ecl.stage_history` | `instrumen_id`, `stage_sesudah` (current stage via MAX `tanggal_migrasi`) | Init schema 0001 + migration 0022 (P4-M1). |

### OQ-E dari Phase 4 Plan (CCF relevance)

**OQ-E** — "Apakah CCF relevan untuk instrumen BLIPS Phase 1?" — Per plan §7 default assumption:
> CCF = 0 untuk semua instrumen Phase 1 (deposito, obligasi, saham, reksadana). Tidak ada undrawn commitment untuk instrumen ini.
> EAD = Outstanding_Principal + Accrued_Interest.

Story 4 (CCF lookup) tetap ditulis untuk kelengkapan arsitektur dan future-proofing, namun untuk Phase 1 hasilnya akan selalu CCF = 0.000000 kecuali config table berkata lain.

### OQ-A dari Phase 4 Plan (FL multiplier semantik)

**OQ-A** belum di-resolve final. Default assumption yang berlaku untuk M2:
> `impact_pd.impact_multiplier` (flat, per periode) di-apply ke semua 3 skenario.
> `impact_mev_pd.impact_multiplier` per skenario (GOOD/BAD) di-multiply di atas `impact_pd` secara independen.
> Formula final per skenario: `PD_FL_skenario = PD_base × impact_pd × impact_mev_pd_skenario` (untuk GOOD/BAD); NORMAL = `PD_base × impact_pd × 1.0`.

Flag OQ-A **HARUS dikonfirmasi** oleh `ifrs9-compliance-reviewer` + ALCO sebelum P4-M7 merge.

### Accrued Interest dependency

EAD formula = Outstanding + Accrued_Interest + (Undrawn × CCF). Nilai `Accrued_Interest` bersumber dari `ecl.eir_amortization_schedule` (P4-M5) atau `trx.pendapatan_akrual` (APP-B Phase 5). **Dependency ini belum tersedia di Phase 4 Sprint 1.**

**Workaround Phase 4**: EAD = Outstanding_Principal + Accrued_Interest. Jika `eir_amortization_schedule` belum tersedia untuk instrumen (P4-M5 belum selesai), Accrued_Interest dianggap 0 dan engine mencatat warning ke `sys.job.result_jsonb`. Ini didokumentasikan sebagai EAD underestimate sementara.

---

## Open Questions (Baru — M2 spesifik)

| ID | Pertanyaan | Asumsi Default | Perlu Konfirmasi |
|---|---|---|---|
| OQ-M2-1 | PD Lifetime untuk Stage 2: rumus interpolasi antar tenor bucket (`pd_lifetime_3y`, `5y`, `7y`, `10y`) — linear interpolasi berdasarkan sisa tenor, atau ambil bucket terdekat? | Linear interpolasi berdasarkan `tenor_remaining` vs tenor bucket dalam tahun. Jika tenor > 10y → gunakan `pd_lifetime_10y`. | FSD-APP-C §3 + ecl-eir-engineer |
| OQ-M2-2 | `tipe_eksposur` untuk LGD lookup — dari mana nilainya diambil per instrumen? `mst.counterparty` tidak memiliki kolom `tipe_eksposur_lgd` eksplisit di schema 0001. Apakah derive dari `tipe_counterparty` (BANK/KORPORASI/PEMERINTAH/dll)? | Derive dari `mst.counterparty.tipe_counterparty` dengan mapping BLIPS → pool LGD (BANK → 'BANK', KORPORASI → 'CORPORATE', dll). Mapping dikonfigurasi di `sys.config`. | data-modeler (apakah perlu kolom baru `mst.counterparty.tipe_eksposur_lgd`) + FSD-APP-C |
| OQ-M2-3 | Outstanding_Principal untuk EAD — dari kolom mana di `mst.instrumen`? `nominal` adalah nilai nominal awal, bukan outstanding saat ini (setelah amortisasi). Outstanding aktual ada di `ecl.eir_amortization_schedule` (P4-M5). | Untuk instrumen bullet (deposito, obligasi non-amortizing): outstanding = `mst.instrumen.nominal`. Untuk instrumen amortizing: outstanding dari EIR schedule jika tersedia, else `nominal`. EIR schedule dependency ke P4-M5. | FSD-APP-C §4 + ecl-eir-engineer |
| OQ-M2-4 | `skenario NORMAL` untuk FL multiplier — apakah `impact_mev_pd` memiliki row untuk NORMAL? Schema 0001 `mst.impact_mev_pd` hanya accept `skenario IN ('GOOD','BAD')` per CHECK constraint. | NORMAL multiplier dari `mst.impact_mev_pd` = 1.0 (tidak ada adjustment). NORMAL PD = `PD_base × impact_pd.impact_multiplier`. Konfirmasi lewat OQ-A resolution. | ifrs9-compliance-reviewer + ALCO |
| OQ-M2-5 | Apakah parameter snapshot untuk M2 (PD, LGD, bobot, FL) disimpan per instrumen per calc run, atau sekali per calc run header? | Snapshot disimpan **sekali per calc run** sebagai JSONB blob di `ecl.calc_header.parameter_snapshot_id`. M2 helpers hanya membaca dari master; snapshot dilakukan oleh M7/M8 saat sealing. | system-analyst (schema `ecl.calc_header.parameter_snapshot_id` referencing tabel apa?) |
| OQ-M2-6 | Kurs untuk EAD multi-currency — apakah harus kurs BI JISDOR tanggal `evaluation_date`, atau kurs tanggal `tanggal_penempatan`? | Kurs BI JISDOR **tanggal evaluation_date** (per IFRS9 §B5.5.44 — EAD dihitung pada tanggal assessment, bukan acquisition). | FSD-APP-C §4 + ifrs9-compliance-reviewer |

---

## Story APP-C-PAR-001 — PD Lookup per Instrumen per Stage per Skenario

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/engine`); ROLE-RISK (preview UI)
**Trigger**: Dipanggil oleh ECL engine saat memproses instrumen dalam calc run; atau dipanggil langsung via endpoint preview untuk ROLE-RISK sebelum menjalankan calc run
**Goal**: Mendapatkan nilai PD `NUMERIC(10,8)` yang tepat untuk instrumen tertentu, untuk stage tertentu, per skenario (GOOD/NORMAL/BAD), termasuk penerapan dual forward-looking multiplier

### Pre-conditions
1. `mst.pd_pefindo` memiliki row `APPROVED` dengan rating yang sesuai dan `periode_berlaku_sampai IS NULL` (atau mencakup `evaluation_date`)
2. Instrumen memiliki `counterparty_id` yang valid dengan rating aktif di `mst.rating_history_counterparty` (`workflow_status = 'APPROVED'`, `tanggal_berlaku <= evaluation_date`)
3. `mst.impact_mev_pd` dan `mst.impact_pd` memiliki row `APPROVED` untuk `periode_id` yang sedang dievaluasi
4. Stage instrumen diketahui dari `ecl.stage_history` (P4-M1 output)

### Logika Lookup

```
Stage 1:
  PD_base = mst.pd_pefindo.pd_12month (untuk rating aktif counterparty)
  PD_FL_GOOD   = PD_base × impact_pd × impact_mev_pd(GOOD)
  PD_FL_NORMAL = PD_base × impact_pd × 1.0          (NORMAL tidak ada impact_mev_pd)
  PD_FL_BAD    = PD_base × impact_pd × impact_mev_pd(BAD)

Stage 2:
  PD_base = interpolasi linear pd_lifetime_Xy dari mst.pd_pefindo
            berdasarkan tenor_remaining (tanggal_jatuh_tempo − evaluation_date)
  Jika tenor > 10y → gunakan pd_lifetime_10y
  Jika tenor ≤ 3y → gunakan pd_lifetime_3y
  Kemudian terapkan FL multiplier sama seperti Stage 1

Stage 3:
  PD = 1.00000000 (fixed, per DEC-010)
  FL multiplier tidak diterapkan (PD sudah 1.0)
```

### Data References

| Tabel | Akses | Kolom Utama yang Dibaca |
|---|---|---|
| `mst.pd_pefindo` | READ | `rating`, `pd_12month`, `pd_lifetime_3y`, `pd_lifetime_5y`, `pd_lifetime_7y`, `pd_lifetime_10y`, `periode_berlaku_dari`, `periode_berlaku_sampai`, `workflow_status` |
| `mst.impact_pd` | READ | `impact_multiplier`, `periode_id`, `workflow_status` |
| `mst.impact_mev_pd` | READ | `skenario`, `impact_multiplier`, `periode_id`, `workflow_status` |
| `mst.rating_history_counterparty` | READ | `counterparty_id`, `rating_pefindo`, `tanggal_berlaku`, `workflow_status` |
| `ecl.stage_history` | READ | `instrumen_id`, `stage_sesudah` (current stage via latest `tanggal_migrasi`) |
| `mst.instrumen` | READ | `counterparty_id`, `tanggal_jatuh_tempo`, `klasifikasi_psak71` |

### Permissions Needed

| Actor | Permission | MFA |
|---|---|---|
| ECL calc engine (service account) | `instrumen.read`, `ecl_parameter.read` | Tidak (service account, bukan human) |
| ROLE-RISK (preview UI) | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-AUDIT | `ecl_parameter.read` (read-only) | Tidak |

### Audit Events

| Action | Kapan | Keterangan |
|---|---|---|
| `ECL_PARAM.PD_LOOKUP_SNAPSHOT` | Saat calc run dimulai — satu kali per run, bukan per instrumen | Snapshot parameter PD + FL yang aktif dicatat ke `aud.audit_log` via M7/M8 parameter snapshot mechanism. M2 helper sendiri tidak menulis audit log per panggilan (high-frequency call dalam loop instrumen). |

### Acceptance Criteria

```gherkin
Feature: PD lookup per instrumen per stage per skenario

  Background:
    Given periode evaluasi: 2026-06-30 (evaluation_date)
    And periode buku: Jun-2026 (aktif, ID = "PBUKU-2026-06")
    And mst.impact_pd aktif untuk periode Jun-2026: impact_multiplier = 1.05000000
    And mst.impact_mev_pd aktif periode Jun-2026:
      | skenario | impact_multiplier |
      | GOOD     | 0.85000000        |
      | BAD      | 1.20000000        |
    And mst.pd_pefindo APPROVED untuk rating 'idAA':
      | pd_12month | pd_lifetime_3y | pd_lifetime_5y | pd_lifetime_7y | pd_lifetime_10y |
      | 0.00350000 | 0.01200000     | 0.02100000     | 0.03000000     | 0.04100000      |

  # ─── HAPPY PATH 1: Stage 1 — PD 12-month dengan dual FL ─────────────────

  Scenario: PD Stage 1 dengan tiga skenario
    Given instrumen INST-0100 klasifikasi_psak71 = 'AC', current stage = STAGE_1
    And rating aktif counterparty = 'idAA' per tanggal 2026-06-30
    When ECL engine memanggil LookupPD(instrumenID=INST-0100, stage=STAGE_1, periodeID=PBUKU-2026-06, evaluationDate=2026-06-30)
    Then hasil lookup:
      | skenario | PD_FL                                                         |
      | GOOD     | 0.00350000 × 1.05000000 × 0.85000000 = 0.00312375 (8 dp)    |
      | NORMAL   | 0.00350000 × 1.05000000 × 1.00000000 = 0.00367500 (8 dp)    |
      | BAD      | 0.00350000 × 1.05000000 × 1.20000000 = 0.00441000 (8 dp)    |
    And semua nilai dikembalikan sebagai NUMERIC(10,8), tanpa float64
    And tidak ada row yang ditulis ke DB (pure read)

  # ─── HAPPY PATH 2: Stage 2 — Lifetime PD dengan interpolasi ─────────────

  Scenario: PD Stage 2 instrumen dengan tenor remaining 4 tahun — interpolasi
    Given instrumen INST-0101 klasifikasi_psak71 = 'AC', current stage = STAGE_2
    And tanggal_jatuh_tempo INST-0101 = 2030-06-30 (tenor remaining dari evaluation_date = 4.0 tahun)
    And rating aktif counterparty = 'idAA'
    When ECL engine memanggil LookupPD(instrumenID=INST-0101, stage=STAGE_2, ...)
    Then PD_base = interpolasi linear antara pd_lifetime_3y (1.20%) dan pd_lifetime_5y (2.10%):
      t = (4.0 - 3.0) / (5.0 - 3.0) = 0.5
      PD_lifetime_4y = 0.01200000 + 0.5 × (0.02100000 − 0.01200000) = 0.01650000
    And FL multiplier diterapkan sama seperti Stage 1
    And PD_FL_NORMAL = 0.01650000 × 1.05000000 = 0.01732500

  # ─── HAPPY PATH 3: Stage 3 — PD = 1.0 fixed ─────────────────────────────

  Scenario: PD Stage 3 selalu mengembalikan 1.0 tanpa FL
    Given instrumen INST-0102, current stage = STAGE_3
    When ECL engine memanggil LookupPD(instrumenID=INST-0102, stage=STAGE_3, ...)
    Then PD untuk semua skenario = 1.00000000
    And FL multiplier TIDAK diterapkan
    And return value bertipe NUMERIC(10,8)

  # ─── EDGE CASE: FVTPL / FVOCI_ELECTION di-skip ───────────────────────────

  Scenario: Instrumen FVTPL tidak boleh di-lookup PD untuk ECL
    Given instrumen INST-0103, klasifikasi_psak71 = 'FVTPL'
    When ECL engine memanggil LookupPD(instrumenID=INST-0103, ...)
    Then helper mengembalikan error: INSTRUMENT_ECL_NOT_APPLICABLE
    And message: "Instrumen INST-0103 klasifikasi FVTPL tidak memerlukan ECL (IFRS9 §5.5.1)"
    And tidak ada PD value yang dikembalikan

  # ─── ERROR CASE: Rating counterparty tidak tersedia ──────────────────────

  Scenario: Rating aktif counterparty tidak ada per evaluation_date
    Given instrumen INST-0104, klasifikasi_psak71 = 'AC'
    And tidak ada row di mst.rating_history_counterparty untuk counterparty-nya per 2026-06-30
    When ECL engine memanggil LookupPD(instrumenID=INST-0104, ...)
    Then helper mengembalikan error: PD_LOOKUP_RATING_MISSING
    And message: "Rating aktif untuk counterparty CP-XXX tidak ditemukan per tanggal 2026-06-30. Perlu upload rating Pefindo terbaru."
    And engine mencatat instrumen ini sebagai SKIPPED dalam job result

  # ─── ERROR CASE: pd_pefindo APPROVED tidak ada untuk rating ──────────────

  Scenario: Rating tersedia tapi tidak ada entri pd_pefindo APPROVED
    Given counterparty memiliki rating aktif = 'idA+'
    And tidak ada row pd_pefindo APPROVED untuk rating 'idA+'
    When ECL engine memanggil LookupPD(...)
    Then helper mengembalikan error: PD_LOOKUP_CURVE_NOT_FOUND
    And message: "Kurva PD tidak ditemukan untuk rating idA+ per periode Jun-2026. Pastikan mst.pd_pefindo sudah diisi dan di-approve oleh ALCO."

  # ─── ERROR CASE: FL parameter tidak tersedia untuk periode ───────────────

  Scenario: impact_pd atau impact_mev_pd belum tersedia untuk periode buku berjalan
    Given mst.impact_pd tidak memiliki row APPROVED untuk periode Jun-2026
    When ECL engine memanggil LookupPD(...)
    Then helper mengembalikan error: PD_LOOKUP_FL_PARAM_MISSING
    And message: "Forward-looking multiplier (impact_pd) tidak ditemukan untuk periode Jun-2026. Parameter ECL belum disetujui ALCO untuk periode ini."
```

### Open Questions — Story 1

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-PAR-1a | Apakah PD interpolasi harus monotone (PD_3y ≤ PD_5y ≤ PD_7y ≤ PD_10y)? Jika tidak monotone akibat input Pefindo, apakah interpolasi tetap jalan atau error? | Validasi monotonicity dilakukan di service layer saat upload/approve PD. Jika non-monotone di DB (data lama), interpolasi tetap jalan tapi catat warning ke audit log. Sesuai catatan migration 0013. |
| OQ-PAR-1b | Jika tenor_remaining < 0 (instrumen sudah jatuh tempo tapi belum diderecognize) — PD berapa yang dipakai? | PD_12month (Stage 1) atau 1.0 (Stage 3). Instrumen jatuh tempo tapi masih AKTIF adalah anomali — flag ke ROLE-RISK via notifikasi. |

---

## Story APP-C-PAR-002 — LGD Lookup per Instrumen per Pool

**Actor**: ECL calc engine (Asynq worker); ROLE-RISK (preview UI)
**Trigger**: Dipanggil oleh ECL engine bersamaan atau setelah PD lookup, untuk setiap instrumen dalam calc run
**Goal**: Mendapatkan nilai LGD `NUMERIC(10,8)` dari pool `mst.lgd_basel` yang sesuai berdasarkan tipe eksposur instrumen/counterparty; menerapkan collateral haircut jika ada

### Pre-conditions
1. `mst.lgd_basel` memiliki row `APPROVED` untuk `tipe_eksposur` yang sesuai dengan `periode_berlaku_sampai IS NULL` (atau mencakup evaluation_date)
2. Mapping `tipe_eksposur` untuk instrumen tersedia (derive dari tipe counterparty atau config)
3. Instrumen `klasifikasi_psak71 IN ('AC', 'FVOCI')` — FVTPL/FVOCI_ELECTION di-skip

### Logika Lookup

```
LGD_pool = mst.lgd_basel.lgd WHERE tipe_eksposur = derive(instrumen) AND APPROVED AND active

Collateral haircut (jika ada): LGD_eff = LGD_pool × (1 - collateral_haircut_rate)
  collateral_haircut_rate diambil dari sys.config 'LGD_COLLATERAL_HAIRCUT_{tipe_kolateral}'
  Default Phase 1: haircut = 0 (tidak ada collateral untuk deposito/obligasi/saham/reksadana)

LGD final = LGD_eff dikembalikan sebagai NUMERIC(10,8)
```

**Mapping tipe eksposur (derived dari `mst.counterparty.tipe_counterparty`):**

| tipe_counterparty | tipe_eksposur LGD |
|---|---|
| BANK | BANK |
| KORPORASI | CORPORATE |
| PEMERINTAH | SOVEREIGN |
| ASURANSI | CORPORATE |
| REKSADANA | (lihat M4 look-through — tidak pakai pool tunggal) |

Mapping disimpan di `sys.config` key `'LGD_COUNTERPARTY_TYPE_MAPPING'` sebagai JSONB.

### Data References

| Tabel | Akses | Kolom Utama |
|---|---|---|
| `mst.lgd_basel` | READ | `tipe_eksposur`, `lgd NUMERIC(10,8)`, `karakteristik`, `periode_berlaku_dari`, `periode_berlaku_sampai`, `workflow_status` |
| `mst.instrumen` | READ | `counterparty_id`, `klasifikasi_psak71`, `tipe_instrumen` |
| `mst.counterparty` | READ | `id`, `tipe_counterparty` |
| `sys.config` | READ | `LGD_COUNTERPARTY_TYPE_MAPPING`, `LGD_COLLATERAL_HAIRCUT_*` |

### Permissions Needed

| Actor | Permission | MFA |
|---|---|---|
| ECL calc engine (service account) | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-RISK (preview) | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-AUDIT | `ecl_parameter.read` | Tidak |

### Audit Events

| Action | Kapan |
|---|---|
| `ECL_PARAM.LGD_LOOKUP_SNAPSHOT` | Satu kali per calc run — snapshot seluruh LGD parameter aktif yang dipakai. Tidak per instrumen. |

### Acceptance Criteria

```gherkin
Feature: LGD lookup per instrumen dari mst.lgd_basel pool

  Background:
    Given periode evaluasi: Jun-2026
    And mst.lgd_basel APPROVED untuk tipe_eksposur:
      | tipe_eksposur | lgd         |
      | BANK          | 0.45000000  |
      | CORPORATE     | 0.55000000  |
      | SOVEREIGN     | 0.15000000  |
    And sys.config 'LGD_COUNTERPARTY_TYPE_MAPPING':
      { "BANK": "BANK", "KORPORASI": "CORPORATE", "PEMERINTAH": "SOVEREIGN" }

  # ─── HAPPY PATH 1: Instrumen deposito — counterparty BANK ────────────────

  Scenario: LGD lookup untuk deposito di bank
    Given instrumen INST-0200, tipe_instrumen = 'DEPOSITO', klasifikasi_psak71 = 'AC'
    And counterparty instrumen adalah CP-BANK-02, tipe_counterparty = 'BANK'
    When ECL engine memanggil LookupLGD(instrumenID=INST-0200, periodeID=PBUKU-2026-06)
    Then LGD = 0.45000000 (dari pool BANK)
    And nilai dikembalikan sebagai NUMERIC(10,8), tanpa float64
    And tidak ada DB write (pure read)

  # ─── HAPPY PATH 2: Instrumen obligasi korporasi ───────────────────────────

  Scenario: LGD lookup untuk obligasi dari emiten korporasi
    Given instrumen INST-0201, tipe_instrumen = 'OBLIGASI', klasifikasi_psak71 = 'FVOCI'
    And counterparty = CP-CORP-01, tipe_counterparty = 'KORPORASI'
    When ECL engine memanggil LookupLGD(instrumenID=INST-0201, ...)
    Then LGD = 0.55000000 (pool CORPORATE)

  # ─── HAPPY PATH 3: LGD dengan collateral haircut ─────────────────────────

  Scenario: Instrumen dengan collateral — LGD efektif lebih rendah
    Given instrumen INST-0202, tipe_instrumen = 'OBLIGASI', pool = CORPORATE (LGD=0.55)
    And sys.config 'LGD_COLLATERAL_HAIRCUT_PROPERTI' = 0.40 (haircut 40%)
    And instrumen ini memiliki collateral_type = 'PROPERTI' di sys.config atau mst.instrumen extension
    When ECL engine memanggil LookupLGD(instrumenID=INST-0202, ...)
    Then LGD_eff = 0.55000000 × (1 - 0.40000000) = 0.33000000
    And nilai dikembalikan: 0.33000000 NUMERIC(10,8)

  # ─── EDGE CASE: Tipe_counterparty tidak ada di mapping ───────────────────

  Scenario: tipe_counterparty tidak ada dalam LGD mapping config
    Given counterparty CP-MULTI-01, tipe_counterparty = 'MULTILATERAL'
    And 'MULTILATERAL' tidak ada dalam sys.config 'LGD_COUNTERPARTY_TYPE_MAPPING'
    When ECL engine memanggil LookupLGD(...)
    Then helper mengembalikan error: LGD_LOOKUP_MAPPING_NOT_FOUND
    And message: "Tipe counterparty 'MULTILATERAL' tidak memiliki mapping LGD pool. Konfigurasikan di sys.config LGD_COUNTERPARTY_TYPE_MAPPING."
    And instrumen SKIPPED dalam job result

  # ─── ERROR CASE: LGD pool APPROVED tidak tersedia ────────────────────────

  Scenario: Pool LGD untuk tipe_eksposur tidak ada atau belum APPROVED
    Given tipe_eksposur = 'BANK' tapi tidak ada row APPROVED di mst.lgd_basel
    When ECL engine memanggil LookupLGD(...)
    Then helper mengembalikan error: LGD_LOOKUP_POOL_NOT_FOUND
    And message: "LGD pool untuk tipe_eksposur BANK tidak ditemukan atau belum di-approve ALCO untuk periode Jun-2026."

  # ─── EDGE CASE: Instrumen Reksadana — tidak menggunakan pool tunggal ──────

  Scenario: Reksadana tidak di-lookup via Story 2 — diarahkan ke look-through M4
    Given instrumen INST-0203, tipe_instrumen = 'REKSADANA', klasifikasi_psak71 = 'AC'
    When ECL engine memanggil LookupLGD(instrumenID=INST-0203, ...)
    Then helper mengembalikan error: LGD_LOOKUP_USE_LOOKTHROUGH
    And message: "Instrumen jenis REKSADANA menggunakan mekanisme look-through ECL (P4-M4). LGD tidak di-lookup per pool tunggal."
```

### Open Questions — Story 2

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-PAR-2a | Apakah perlu tabel `mst.instrumen_collateral` untuk menyimpan collateral per instrumen, atau cukup via sys.config? | Collateral tidak umum di instrumen Phase 1 (deposito/obligasi standar). Default haircut = 0. Tabel collateral = scope Phase 5 bila ada instrumen beragunan. |
| OQ-PAR-2b | Apakah mst.counterparty punya kolom `tipe_counterparty` yang sudah terisi konsisten di data? | Ya, diasumsikan ada dari Phase 3. Perlu verifikasi oleh data-modeler bahwa kolom ini ada dan ter-isi di seed data. |

---

## Story APP-C-PAR-003 — EAD Calculation per Instrumen

**Actor**: ECL calc engine (Asynq worker); ROLE-RISK (preview UI)
**Trigger**: Dipanggil oleh ECL engine setelah PD dan LGD diketahui; juga dapat dipanggil preview untuk ROLE-RISK
**Goal**: Menghitung nilai EAD dalam IDR `NUMERIC(20,4)` dan breakdown komponen per instrumen pada tanggal evaluasi, termasuk konversi multi-currency menggunakan kurs BI JISDOR

### Pre-conditions
1. `mst.instrumen` tersedia dengan `status = 'AKTIF'`, `klasifikasi_psak71 IN ('AC', 'FVOCI')`
2. Untuk instrumen FCY (non-IDR): kurs BI JISDOR per `evaluation_date` tersedia di `mst.kurs` dengan `sumber_kurs = 'BI_JISDOR'` dan `workflow_status = 'APPROVED'`
3. P4-M5 EIR schedule tersedia untuk komponen Accrued Interest (jika sudah selesai); jika tidak, Accrued_Interest = 0 dengan warning

### Formula EAD (DEC-016 precision)

```
EAD_FCY = Outstanding_Principal_FCY + Accrued_Interest_FCY + (Undrawn_FCY × CCF)

Untuk Phase 1 (CCF = 0 per OQ-E):
  EAD_FCY = Outstanding_Principal_FCY + Accrued_Interest_FCY

EAD_IDR = EAD_FCY × kurs_BI_JISDOR(evaluation_date, kode_mata_uang)

Untuk instrumen IDR (mata_uang = 'IDR'):
  EAD_IDR = EAD_FCY (tidak ada konversi)
```

### Sumber Outstanding_Principal per tipe instrumen

| Tipe Instrumen | Sumber Outstanding |
|---|---|
| DEPOSITO | `mst.instrumen.nominal` (bullet, tidak ada amortisasi) |
| OBLIGASI | `ecl.eir_amortization_schedule.principal_outstanding` per eval date (P4-M5); fallback ke `nominal` jika schedule belum ada |
| SAHAM | `mst.instrumen.nominal` × jumlah lot × harga terkini (harga dari BEI feed); atau NAV untuk MTM |
| REKSADANA | `mst.instrumen.nominal` × NAB terkini (dari KSEI feed) |

**Catatan**: Untuk instrumen SAHAM dan REKSADANA yang klasifikasinya FVTPL atau FVOCI_ELECTION, ECL tidak berlaku sehingga EAD calc tidak akan dipanggil untuk mereka.

### Data References

| Tabel | Akses | Kolom Utama |
|---|---|---|
| `mst.instrumen` | READ | `id`, `tipe_instrumen`, `mata_uang`, `nominal`, `klasifikasi_psak71`, `tanggal_jatuh_tempo` |
| `mst.kurs` | READ | `kode_mata_uang`, `nilai_kurs NUMERIC(20,8)`, `sumber_kurs`, `tanggal_berlaku`, `workflow_status` |
| `ecl.eir_amortization_schedule` | READ | `instrumen_id`, `tanggal_cicilan`, `principal_outstanding`, `bunga_akrual`, `schedule_version` (P4-M5 dependency) |

### Permissions Needed

| Actor | Permission | MFA |
|---|---|---|
| ECL calc engine | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-RISK (preview) | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-AUDIT | `instrumen.read`, `ecl_parameter.read` | Tidak |

### Audit Events

| Action | Kapan |
|---|---|
| `ECL_PARAM.EAD_SNAPSHOT` | Satu kali per calc run — kurs aktif yang digunakan di-snapshot. Tidak per instrumen. |

### Acceptance Criteria

```gherkin
Feature: EAD calculation per instrumen dengan konversi multi-currency

  Background:
    Given evaluation_date = 2026-06-30
    And mst.kurs BI JISDOR APPROVED per 2026-06-30:
      | kode_mata_uang | nilai_kurs     |
      | USD            | 16425.00000000 |
      | EUR            | 17800.50000000 |

  # ─── HAPPY PATH 1: Instrumen IDR — deposito ──────────────────────────────

  Scenario: EAD deposito IDR tanpa Accrued Interest (P4-M5 belum selesai)
    Given instrumen INST-0300, tipe_instrumen = 'DEPOSITO', mata_uang = 'IDR'
    And nominal = 5.000.000.000 IDR (NUMERIC(20,4))
    And ecl.eir_amortization_schedule belum tersedia untuk INST-0300 (P4-M5 belum selesai)
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0300, evaluationDate=2026-06-30)
    Then hasil:
      | outstanding_principal_idr | 5000000000.0000 |
      | accrued_interest_idr      | 0.0000 (dengan WARNING: EIR schedule P4-M5 belum tersedia) |
      | undrawn_commitment_idr    | 0.0000 |
      | ead_idr                   | 5000000000.0000 |
      | ead_fcy                   | 5000000000.0000 (sama, karena IDR) |
      | kurs_digunakan            | 1.00000000 (IDR tidak perlu konversi) |
    And warning dicatat di job result: "INST-0300: Accrued Interest = 0 karena EIR schedule belum tersedia (P4-M5)"
    And tidak ada DB write

  # ─── HAPPY PATH 2: Instrumen FCY — obligasi USD ──────────────────────────

  Scenario: EAD obligasi USD dengan konversi ke IDR
    Given instrumen INST-0301, tipe_instrumen = 'OBLIGASI', mata_uang = 'USD'
    And nominal = 1.000.000 USD
    And ecl.eir_amortization_schedule tersedia: outstanding = 950.000 USD, bunga_akrual = 15.000 USD
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0301, evaluationDate=2026-06-30)
    Then EAD_FCY = 950000.0000 + 15000.0000 = 965000.0000 USD
    And EAD_IDR = 965000.0000 × 16425.00000000 = 15850125000.0000 IDR
    And semua komponen dikembalikan: outstanding_principal_fcy, accrued_interest_fcy, ead_fcy, ead_idr, kurs_digunakan

  # ─── HAPPY PATH 3: Instrumen IDR dengan Accrued Interest dari EIR schedule ─

  Scenario: EAD obligasi IDR dengan accrued interest tersedia
    Given instrumen INST-0302, tipe_instrumen = 'OBLIGASI', mata_uang = 'IDR'
    And ecl.eir_amortization_schedule: outstanding = 10.000.000.000, bunga_akrual = 250.000.000
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0302, ...)
    Then EAD_IDR = 10000000000.0000 + 250000000.0000 = 10250000000.0000
    And tidak ada kurs konversi (IDR)

  # ─── ERROR CASE: Kurs BI JISDOR tidak tersedia untuk valuta instrumen ────

  Scenario: Instrumen EUR tapi kurs EUR tidak tersedia per evaluation_date
    Given instrumen INST-0303, mata_uang = 'EUR'
    And mst.kurs tidak memiliki row APPROVED untuk EUR per 2026-06-30
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0303, ...)
    Then helper mengembalikan error: EAD_FX_RATE_MISSING
    And message: "Kurs BI JISDOR untuk EUR tidak tersedia per 2026-06-30. Upload kurs manual atau tunggu feed BI."
    And instrumen SKIPPED dalam job result

  # ─── ERROR CASE: Instrumen FCY tapi kurs belum APPROVED ─────────────────

  Scenario: Kurs ada tapi masih workflow_status DRAFT
    Given mst.kurs untuk USD per 2026-06-30 ada tapi workflow_status = 'DRAFT'
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0301, ...)
    Then helper mengembalikan error: EAD_FX_RATE_NOT_APPROVED
    And message: "Kurs USD per 2026-06-30 belum di-approve (status: DRAFT). Kurs harus APPROVED sebelum dipakai ECL."

  # ─── EDGE CASE: Instrumen saham FVTPL — EAD tidak dihitung ──────────────

  Scenario: Saham FVTPL tidak memiliki ECL — EAD tidak dihitung
    Given instrumen INST-0304, tipe_instrumen = 'SAHAM', klasifikasi_psak71 = 'FVTPL'
    When ECL engine memanggil ComputeEAD(instrumenID=INST-0304, ...)
    Then helper mengembalikan error: INSTRUMENT_ECL_NOT_APPLICABLE
    And message: "Instrumen FVTPL tidak memerlukan ECL/EAD calculation."
```

### Open Questions — Story 3

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-PAR-3a | Untuk instrumen saham (AC classification edge case — rare tapi possible), outstanding = harga pasar × lot atau harga perolehan? | Harga perolehan (`nominal`) untuk AC; harga pasar untuk FVOCI. Konfirmasi ke FSD-APP-C. |
| OQ-PAR-3b | Jika kurs BI JISDOR tidak tersedia per evaluation_date TAPI tersedia T-1 (hari kerja sebelumnya) — apakah boleh gunakan T-1? | Tidak boleh otomatis fallback ke T-1 tanpa approval. Engine harus fail dengan EAD_FX_RATE_MISSING. ROLE-AKUN harus upload manual atau menunggu feed. |

---

## Story APP-C-PAR-004 — CCF Lookup per Tipe Instrumen

**Actor**: ECL calc engine (Asynq worker)
**Trigger**: Dipanggil oleh ComputeEAD (Story 3) sebagai sub-step untuk mendapatkan CCF sebelum EAD final dihitung
**Goal**: Mendapatkan CCF percentage `NUMERIC(7,4)` untuk tipe instrumen/fasilitas tertentu dari tabel konfigurasi; untuk Phase 1 semua instrumen BLIPS mengembalikan CCF = 0.000000

### Pre-conditions
1. `sys.config` memiliki key `'CCF_TABLE'` berisi JSONB mapping `tipe_instrumen → ccf_value`
2. Instrumen memiliki `tipe_instrumen` yang valid di mapping

### Logika Lookup

```
Baca sys.config.config_value WHERE config_key = 'CCF_TABLE'
Parse JSONB, lookup nilai CCF untuk instrumen.tipe_instrumen
Default (jika tipe tidak ada di mapping): CCF = 0.0000

Phase 1 config (locked via DEC via OQ-E resolution):
{
  "DEPOSITO":   0.0000,
  "OBLIGASI":   0.0000,
  "SAHAM":      0.0000,
  "REKSADANA":  0.0000,
  "SBI":        0.0000,
  "COMMITMENT": 0.7500   -- future use, Basel-style
}
```

### Data References

| Tabel | Akses | Kolom Utama |
|---|---|---|
| `sys.config` | READ | `config_key = 'CCF_TABLE'`, `config_value JSONB` |
| `mst.instrumen` | READ | `tipe_instrumen` |

### Permissions Needed

| Actor | Permission |
|---|---|
| ECL calc engine | `ecl_parameter.read` |
| ROLE-RISK (preview) | `ecl_parameter.read` |

### Audit Events

Tidak ada audit log per-call (pure read, high-frequency). CCF config termasuk dalam parameter snapshot di M7/M8.

### Acceptance Criteria

```gherkin
Feature: CCF lookup per tipe instrumen dari sys.config

  Background:
    Given sys.config 'CCF_TABLE' berisi:
      { "DEPOSITO": 0.0000, "OBLIGASI": 0.0000, "SAHAM": 0.0000, "REKSADANA": 0.0000 }

  # ─── HAPPY PATH 1: Deposito CCF = 0 ─────────────────────────────────────

  Scenario: CCF untuk deposito selalu 0 (Phase 1)
    Given instrumen INST-0400, tipe_instrumen = 'DEPOSITO'
    When ECL engine memanggil LookupCCF(tipeInstrumen='DEPOSITO')
    Then CCF = 0.0000 NUMERIC(7,4)
    And tidak ada DB write
    And tidak ada exception

  # ─── HAPPY PATH 2: CCF untuk obligasi = 0 ───────────────────────────────

  Scenario: CCF untuk obligasi = 0
    Given instrumen INST-0401, tipe_instrumen = 'OBLIGASI'
    When ECL engine memanggil LookupCCF(tipeInstrumen='OBLIGASI')
    Then CCF = 0.0000

  # ─── EDGE CASE: Tipe instrumen tidak ada di CCF_TABLE ────────────────────

  Scenario: Tipe instrumen belum terdaftar di CCF config — fallback ke 0
    Given instrumen INST-0402, tipe_instrumen = 'REPO'
    And 'REPO' tidak ada dalam sys.config 'CCF_TABLE'
    When ECL engine memanggil LookupCCF(tipeInstrumen='REPO')
    Then CCF = 0.0000 (default fallback)
    And catat warning ke job result: "Tipe instrumen REPO tidak ada di CCF_TABLE config. CCF default 0 digunakan."

  # ─── EDGE CASE: CCF_TABLE config tidak ditemukan ─────────────────────────

  Scenario: sys.config 'CCF_TABLE' tidak ada (misconfigured environment)
    Given sys.config tidak memiliki key 'CCF_TABLE'
    When ECL engine memanggil LookupCCF(...)
    Then helper mengembalikan error: CCF_CONFIG_MISSING
    And message: "sys.config 'CCF_TABLE' tidak ditemukan. Pastikan seed data config sudah dijalankan."

  # ─── FUTURE READINESS: Komitmen dengan CCF > 0 ───────────────────────────

  Scenario: (Future) Instrumen COMMITMENT dengan CCF 75%
    Given instrumen INST-COMMIT-001, tipe_instrumen = 'COMMITMENT'
    And sys.config 'CCF_TABLE' diupdate: { ..., "COMMITMENT": 0.7500 }
    When ECL engine memanggil LookupCCF(tipeInstrumen='COMMITMENT')
    Then CCF = 0.7500 NUMERIC(7,4)
    And ComputeEAD mengalikan Undrawn × 0.7500 sebelum dijumlahkan ke EAD
```

---

## Story APP-C-PAR-005 — Preview PD/LGD/EAD/CCF per Instrumen untuk UI

**Actor**: ROLE-RISK (Risk Officer)
**Trigger**: User membuka halaman "ECL Preview" sebelum menjalankan calc run final; atau user membuka detail instrumen dan melihat tab "ECL Parameters"
**Goal**: ROLE-RISK dapat melihat breakdown PD/LGD/EAD/CCF untuk setiap instrumen secara interaktif sebelum eksekusi ECL run, menggunakan parameter master aktif yang sedang berlaku; dilengkapi list, sort, filter, export sesuai UX rule §1

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-RISK dengan permission `ecl_parameter.read` + `instrumen.read`
2. `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_pd`, `mst.impact_mev_pd`, `mst.kurs` semuanya sudah APPROVED untuk periode yang dipilih
3. Periode buku (opsional dipilih dari UI) atau default ke periode aktif terkini

### Logika UI

Endpoint `GET /api/v1/ecl/params/preview` menerima:
- `periode_id` (wajib)
- `instrumen_id` (opsional — jika diisi hanya satu instrumen; jika kosong = semua instrumen aktif yang ECL-applicable)
- Standard listquery params: `sort`, `cursor`, `limit`, `q`, `filter[*]`

Response: list instrumen dengan breakdown kolom:
`kode_instrumen`, `klasifikasi_psak71`, `stage`, `tipe_eksposur`, `mata_uang`,
`pd_12m_atau_lifetime`, `lgd`, `ead_idr` (dengan komponen), `ccf`,
`fl_good`, `fl_normal`, `fl_bad` (PD per skenario),
`warning` (jika ada EIR/kurs missing)

### Data References

Semua tabel yang dikonsumsi Story 1–4 di atas. Tidak ada tabel baru.

### Permissions Needed

| Actor | Permission | MFA |
|---|---|---|
| ROLE-RISK | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-AUDIT | `instrumen.read`, `ecl_parameter.read` | Tidak |
| ROLE-ALCO | `instrumen.read`, `ecl_parameter.read` (read-only preview sebelum approve) | Wajib (MFA login ALCO) |

### Audit Events

| Action | Kapan |
|---|---|
| `ECL_PARAM.PREVIEW_EXPORT` | Saat ROLE-RISK mengekspor preview ke CSV/XLSX |

### Acceptance Criteria

```gherkin
Feature: Preview PD/LGD/EAD/CCF per instrumen di UI

  Background:
    Given user ter-autentikasi sebagai ROLE-RISK
    And periode buku Jun-2026 dipilih
    And 15 instrumen aktif klasifikasi AC atau FVOCI dalam portofolio
    And semua parameter master (pd_pefindo, lgd_basel, kurs, bobot, FL) sudah APPROVED

  # ─── HAPPY PATH 1: Load preview list semua instrumen ─────────────────────

  Scenario: Preview list semua instrumen ECL-applicable untuk periode
    When user mengakses GET /api/v1/ecl/params/preview?periode_id=PBUKU-2026-06
    Then response 200 dengan array 15 instrumen
    And setiap instrumen mengandung: kode_instrumen, klasifikasi_psak71, stage, pd_fl_good, pd_fl_normal, pd_fl_bad, lgd, ead_idr, ccf, warning (null jika tidak ada)
    And diurutkan default: kode_instrumen ASC
    And pagination cursor aktif (limit default 50)
    And UI menampilkan DataTable dengan kolom sortable + filter chip

  # ─── HAPPY PATH 2: Filter preview by stage ───────────────────────────────

  Scenario: Filter preview hanya Stage 2
    When user mengakses GET /api/v1/ecl/params/preview?periode_id=PBUKU-2026-06&filter[stage]=STAGE_2
    Then response hanya berisi instrumen Stage 2
    And kolom pd_fl_* menggunakan PD Lifetime (bukan 12-month)
    And filter chip "Stage: Stage 2" muncul di UI
    And URL state di-update: ?filter[stage]=STAGE_2

  # ─── HAPPY PATH 3: Sort by EAD_IDR descending ────────────────────────────

  Scenario: Sort berdasarkan EAD terbesar
    When user klik header kolom "EAD (IDR)" → descending
    Then response dengan sort=ead_idr:desc
    And instrumen dengan EAD tertinggi berada di baris pertama
    And icon panah turun muncul di header kolom

  # ─── HAPPY PATH 4: Export preview ke CSV ─────────────────────────────────

  Scenario: Export preview list ke CSV respek filter aktif
    Given filter aktif: stage = STAGE_2
    When user klik Export → pilih CSV
    Then file CSV inline (jumlah instrumen Stage 2 < 10k)
    And header row: "Kode Instrumen,Klasifikasi,Stage,PD GOOD,PD NORMAL,PD BAD,LGD,EAD (IDR),CCF,Warning"
    And hanya instrumen Stage 2 yang masuk CSV
    And audit log: ECL_PARAM.PREVIEW_EXPORT dengan filter aktif + row count

  # ─── EDGE CASE: Ada instrumen dengan warning (kurs missing) ──────────────

  Scenario: Instrumen FCY tanpa kurs — tampilkan warning inline
    Given instrumen INST-0303 (EUR, kurs belum tersedia)
    When preview list dimuat
    Then INST-0303 tetap muncul dalam list
    And kolom "EAD (IDR)" untuk INST-0303 menampilkan "-" atau "N/A"
    And kolom "Warning" menampilkan: "Kurs EUR belum tersedia. EAD tidak dapat dihitung."
    And baris INST-0303 diberi highlight (warning row background)

  # ─── ERROR CASE: Periode belum memiliki parameter APPROVED ───────────────

  Scenario: Preview dipanggil tapi parameter periode belum APPROVED
    Given periode buku Jul-2026 dipilih
    And mst.impact_pd belum memiliki row APPROVED untuk Jul-2026
    When user mengakses preview untuk Jul-2026
    Then response 422 dengan error: ECL_PARAM_NOT_READY
    And message: "Parameter ECL untuk periode Jul-2026 belum lengkap atau belum di-approve ALCO. Pastikan pd_pefindo, lgd_basel, bobot_skenario, impact_pd, impact_mev_pd sudah APPROVED untuk periode ini."
    And UI menampilkan banner error dengan daftar parameter yang belum siap

  # ─── ERROR CASE: User tanpa permission mencoba akses ─────────────────────

  Scenario: ROLE-MAKER-TR tidak bisa akses preview parameter ECL
    Given user ter-autentikasi sebagai ROLE-MAKER-TR
    When user mencoba GET /api/v1/ecl/params/preview
    Then HTTP 403 FORBIDDEN
    And message: "Anda tidak memiliki permission ecl_parameter.read"
```

### Open Questions — Story 5

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-PAR-5a | Apakah endpoint preview harus real-time (hitung saat request) atau bisa cached per periode? | Real-time untuk preview UI (paramter bisa berubah). ECL run final menggunakan snapshot ter-frozen. Cache TTL = 5 menit untuk UI responsiveness. |
| OQ-PAR-5b | Apakah ROLE-ALCO perlu endpoint ini untuk review sebelum approve parameter? | Ya — ALCO lihat dampak parameter sebelum approve. Read-only, MFA login. Ini use case valid untuk M2. |

---

## Story APP-C-PAR-006 — Batch Bulk Lookup PD/LGD/EAD/CCF

**Actor**: ECL engine batch worker (Asynq job dalam P4-M7)
**Trigger**: Dipanggil oleh ECL calc engine saat memulai calc run — satu panggilan untuk semua instrumen dalam periode, bukan per instrumen satu per satu
**Goal**: Mengembalikan hasil lookup PD+LGD+EAD+CCF untuk seluruh daftar instrumen sekaligus dalam satu operasi untuk efisiensi; target performa ≤ 500ms untuk 1000 instrumen

### Pre-conditions
1. Semua pre-conditions Story 1–4 terpenuhi untuk semua instrumen dalam batch
2. `instrumen_ids` adalah daftar UUID instrumen yang `klasifikasi_psak71 IN ('AC', 'FVOCI')`, `status = 'AKTIF'`
3. Parameter master untuk periode sudah di-resolve dan di-cache dalam memory sebelum loop (tidak boleh N+1 query)

### Logika Performa

```
Strategi anti-N+1:
1. Load sekali: seluruh rating aktif counterparty untuk periode → map counterpartyID → rating
2. Load sekali: seluruh pd_pefindo APPROVED untuk periode → map rating → PD struct
3. Load sekali: seluruh lgd_basel APPROVED untuk periode → map tipe_eksposur → LGD
4. Load sekali: bobot_skenario, impact_pd, impact_mev_pd per periode → cached
5. Load sekali: kurs aktif per mata_uang → map kode_mata_uang → rate
6. Load batch: EIR schedule outstanding per instrumen_id (jika tersedia)

Loop instrumen: O(1) lookup dari maps di atas, tidak ada DB query di dalam loop
```

### Data References

Sama dengan Story 1–4 — tetapi pattern query adalah **batch load + in-memory map**, bukan individual lookup.

### Permissions Needed

| Actor | Permission |
|---|---|
| ECL calc engine (service account) | `instrumen.read`, `ecl_parameter.read` |

### Audit Events

| Action | Kapan |
|---|---|
| `ECL_PARAM.BULK_LOOKUP_COMPLETE` | Satu kali per calc run — jumlah instrumen di-lookup, warnings count, skipped count |

### Acceptance Criteria

```gherkin
Feature: Bulk PD/LGD/EAD/CCF lookup untuk ECL calc run

  Background:
    Given calc run CALCRUN-2026-06-001 untuk periode Jun-2026
    And 1200 instrumen aktif ECL-applicable dalam portofolio
    And semua parameter master APPROVED

  # ─── HAPPY PATH 1: Bulk lookup sukses untuk semua instrumen ──────────────

  Scenario: Bulk lookup 1000 instrumen dalam batas waktu
    Given 1000 instrumen ACT/FVOCI tanpa error/warning
    When ECL engine memanggil BulkLookup(instrumenIDs=[...1000 IDs...], periodeID, evaluationDate)
    Then response mengembalikan map instrumenID → {pd_good, pd_normal, pd_bad, lgd, ead_idr, ccf, warnings}
    And total waktu eksekusi ≤ 500ms
    And tidak ada N+1 query (diverifikasi via query count di integration test)
    And semua 1000 instrumen memiliki PD, LGD, EAD dalam hasil

  # ─── HAPPY PATH 2: Bulk lookup dengan sebagian instrumen error ───────────

  Scenario: Bulk lookup 1200 instrumen — 5 dengan warning, 3 SKIPPED
    Given 1192 instrumen normal, 5 dengan kurs FCY missing (warning), 3 dengan rating missing (skip)
    When ECL engine memanggil BulkLookup(instrumenIDs=[...1200 IDs...], ...)
    Then response berisi:
      | sukses      | 1192 instrumen dengan PD/LGD/EAD penuh     |
      | warning     | 5 instrumen dengan EAD = 0 + warning kurs  |
      | skipped     | 3 instrumen dengan error PD_LOOKUP_RATING_MISSING |
    And hasil dikembalikan sebagai satu object (tidak abort karena partial failure)
    And summary: { total: 1200, success: 1192, warning: 5, skipped: 3, errors: [...] }

  # ─── EDGE CASE: Semua instrumen FVTPL — hasil kosong ─────────────────────

  Scenario: Portofolio berisi hanya instrumen FVTPL
    Given semua instrumen_ids adalah FVTPL
    When ECL engine memanggil BulkLookup(...)
    Then response: { total: 0, success: 0, skipped: N, errors: ["Semua instrumen FVTPL — tidak ada yang ECL-applicable"] }
    And tidak ada DB query yang sia-sia

  # ─── PERFORMANCE TEST: 1000 instrumen ≤ 500ms ────────────────────────────

  Scenario: Performance benchmark bulk lookup
    Given 1000 instrumen (mix AC dan FVOCI, berbagai mata_uang, rating)
    When BulkLookup dipanggil dengan cold cache (Redis kosong)
    Then eksekusi selesai dalam ≤ 500ms
    And jumlah DB round-trips ≤ 10 (semua parameter di-batch load)
    And Jika dengan warm cache (parameter sudah di-load sebelumnya): ≤ 100ms

  # ─── ERROR CASE: Daftar instrumen kosong ─────────────────────────────────

  Scenario: BulkLookup dipanggil dengan daftar instrumen kosong
    When ECL engine memanggil BulkLookup(instrumenIDs=[], ...)
    Then response: { total: 0, success: 0 } — tidak ada error
    And tidak ada DB query yang dibuat
```

### Open Questions — Story 6

| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-PAR-6a | Apakah hasil BulkLookup di-cache di Redis untuk reuse dalam satu calc run (jika ada retry)? | Ya, cache di Redis dengan key `ecl:params:bulk:{calc_run_id}:{periode_id}`, TTL 2 jam. Invalidate saat parameter master berubah. |
| OQ-PAR-6b | Apakah BulkLookup harus mengembalikan breakdown komponen EAD (principal + accrued + undrawn) atau cukup EAD total? | M2 mengembalikan full breakdown. M7 (ECL engine) butuh total EAD saja untuk formula, tapi breakdown tetap dicatat untuk audit/traceability di `ecl.calc_header`. |

---

## Cross-Story: Dependency Map P4-M2

```
Phase 3 Master Data (APPROVED)
  mst.pd_pefindo      ──► Story 1 (PD lookup)
  mst.lgd_basel       ──► Story 2 (LGD lookup)
  mst.bobot_skenario  ──┐
  mst.impact_pd       ──┤► Story 1 (FL multiplier)
  mst.impact_mev_pd   ──┘
  mst.kurs (BI JISDOR)──► Story 3 (EAD FCY conversion)
  sys.config (CCF_TABLE)► Story 4 (CCF lookup)

P4-M1 Staging Engine ──► Story 1 (current stage) + Story 3 (EAD staging context)

P4-M5 EIR Solver (dependency, partial)
  ecl.eir_amortization_schedule ──► Story 3 (Accrued Interest, Outstanding)
  [Workaround: accrued_interest = 0 jika M5 belum selesai]

P4-M2 Output dikonsumsi oleh:
  P4-M3 LPS Aggregator ──► menggunakan EAD dari Story 3
  P4-M4 Look-through   ──► menggunakan LGD dari Story 2 (per underlying)
  P4-M7 ECL Engine     ──► menggunakan semua output M2 via BulkLookup (Story 6)
  P4-M9/M10 UI         ──► menggunakan Preview endpoint (Story 5)
```

---

## Non-Functional Requirements (Semua Story P4-M2)

| NFR | Requirement | Referensi |
|---|---|---|
| Compliance gate | `ifrs9-compliance-reviewer` BLOCKING sebelum merge ke `develop` | FSD-APP-C §3/§4, DEC-010, DEC-016 |
| No float64 | Semua nilai PD/LGD/EAD/CCF menggunakan `shopspring/decimal.Decimal` di Go | DEC-016 |
| NUMERIC precision | PD/LGD NUMERIC(10,8), IDR NUMERIC(20,4), FX NUMERIC(20,8), CCF NUMERIC(7,4) | DEC-016, db-conventions.md |
| Pure read | M2 helpers tidak menulis ke DB. Write hanya dilakukan oleh M7/M8 saat calc run berjalan | Architecture decision P4-M2 scope |
| Fail loud | Setiap lookup failure mengembalikan error code kanonik (tidak diam-diam return 0) | Error codes: PD_LOOKUP_RATING_MISSING, LGD_LOOKUP_POOL_NOT_FOUND, EAD_FX_RATE_MISSING, dll |
| Parameter snapshot | Parameter master yang digunakan dalam calc run di-snapshot di `ecl.calc_header.parameter_snapshot_id` oleh M7/M8. M2 menyiapkan data; snapshot bukan tanggung jawab M2. | DEC-018, audit trail |
| Cursor pagination | Preview endpoint (Story 5) wajib cursor-based pagination | DEC-022 |
| Export UX | Preview list: sort + filter + CSV/XLSX export | CLAUDE.md UX rule §1 |
| Idempotency | Preview endpoint adalah GET (tidak butuh Idempotency-Key). BulkLookup (internal call) tidak expose ke HTTP. | DEC-021 |
| Penanganan partial failure | BulkLookup harus tetap return partial result untuk instrumen yang sukses, dengan daftar error/warning untuk yang gagal. Tidak abort entire batch karena satu instrumen bermasalah. | Phase 4 plan §10 performance risk |
| FVTPL/FVOCI_ELECTION exclusion | Semua helper functions harus check `klasifikasi_psak71` dan return INSTRUMENT_ECL_NOT_APPLICABLE untuk FVTPL/FVOCI_ELECTION | OQ-G Phase 4 plan §7 |
| No dependency on APP-B | ECL engine Phase 4 membaca `mst.instrumen` dan `ecl.eir_amortization_schedule` (P4-M5) untuk EAD. Tidak menulis ke `trx.*` schema. | Phase 4 plan §1 Scope |

---

## Handoff Checklist

Setelah story ini di-sign-off oleh `ifrs9-compliance-reviewer`:

- [ ] `system-analyst` → Go interface contract untuk `internal/ecl/params`:
  ```
  LookupPD(ctx, instrumenID, stage, periodeID, evaluationDate) → PDResult per skenario, error
  LookupLGD(ctx, instrumenID, periodeID) → LGDResult, error
  ComputeEAD(ctx, instrumenID, evaluationDate) → EADResult (breakdown + total IDR), error
  LookupCCF(ctx, tipeInstrumen) → CCFValue, error
  BulkLookup(ctx, instrumenIDs, periodeID, evaluationDate) → map[UUID]ParamBundle, BulkSummary, error
  ```
- [ ] `system-analyst` → OpenAPI fragment untuk `GET /api/v1/ecl/params/preview`
- [ ] `ecl-eir-engineer` → Implementasi domain logic di `backend/internal/ecl/params/`
- [ ] `backend-engineer-go` → HTTP handler + Gin routing untuk preview endpoint (Story 5)
- [ ] `ifrs9-compliance-reviewer` → BLOCKING gate: verifikasi PD stage logic, Lifetime PD interpolation, FL dual multiplier, no float64
- [ ] Konfirmasi OQ-A (FL multiplier semantik NORMAL skenario) ke ALCO + ifrs9-compliance-reviewer
- [ ] Konfirmasi OQ-M2-2 (sumber tipe_eksposur untuk LGD mapping) ke data-modeler — apakah `mst.counterparty.tipe_counterparty` sudah ada dan konsisten
- [ ] Konfirmasi OQ-M2-3 (sumber Outstanding Principal per tipe instrumen) ke FSD-APP-C
- [ ] Konfirmasi OQ-M2-6 (kurs evaluationDate vs tanggal_penempatan) ke ifrs9-compliance-reviewer
