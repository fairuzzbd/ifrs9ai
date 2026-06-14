# P4-M7 — ECL Core Calculation Engine: User Stories

**Story Set ID**: P4-M7
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 3)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-12
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (staging + ECL formula), §4 (EIR + net carrying), SoW_v1.4.docx §4
**Linked BRD**: BRD §8.2 (ECL Computation Requirements), RACI: ROLE-RISK (R/compute + review), ROLE-ALCO (A/parameter approve), ROLE-CFO (I/result), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-010 — ECL formula: 3-stage × 3-skenario × dual FL multiplier. Default bobot Good/Normal/Bad = 0.25/0.50/0.25.
- DEC-013 — EIR Newton-Raphson, tolerance 1e-10, max 100 iter, presisi 8 desimal.
- DEC-014 — LPS cap IDR 2 miliar per (nasabah, bank), applied SEBELUM ECL.
- DEC-015 — Look-through ECL Reksadana: decompose by underlying asset class.
- DEC-016 — NUMERIC precision: IDR `NUMERIC(20,4)`, FX `NUMERIC(20,8)`, PD/LGD/EIR `NUMERIC(10,8)`. No float64.
- DEC-017 — Workflow 4-eyes rutin. SoD `maker_id ≠ reviewer_id ≠ approver_id`.
- DEC-018 — Audit trail append-only. `ecl.*` no hard delete. `ecl.calc_header` rows immutable setelah sealed.

**Depends on (semua harus MERGED)**:
- P4-M1 — Staging engine (`ecl.stage_history`, `StagingService.GetCurrentStage()`)
- P4-M2 — PD/LGD/EAD helpers (`LookupPD()`, `LookupLGD()`, `ComputeEAD()`, `LookupFL()`, `LookupBobot()`)
- P4-M3 — LPS aggregator (`LPSService.AggregateExcess()` — cash+deposito)
- P4-M4 — Look-through Reksadana (`LookThroughService.BulkCompute()`, `mst.fund_composition`)
- P4-M5 — EIR amortisasi schedule (`ecl.eir_amortization_schedule`, accrued interest for EAD)
- P4-M6 — Amendment lifecycle (`ecl.eir_reestimation_log`, drift detection) — catch-up booking deferred Phase 5

**Handoff berikutnya**:
- `system-analyst` — OpenAPI fragment + Go interface `ECLEngine` + state machine per-instrumen routing
- `data-modeler` — migration 000029 (plan menyebut 000026; setelah 000028 dari M6, nomor berikutnya 000029): `ecl.calc_header` schema-fix (precision IDR → NUMERIC(20,4), audit cols, sealed trigger, routing_path column, flag_poci column); `ecl.calc_detail_skenario` schema-fix (precision, FL-applied values, EAD kolom)
- `ecl-eir-engineer` — implementasi `backend/internal/ecl/engine/` (orchestrator + routing logic + formula)
- `ifrs9-compliance-reviewer` — BLOCKING gate sebelum merge (verifikasi formula kanonik per plan §5 P4-M7)

**Catatan nomor migration**: Plan awal menyebut mig 000026 untuk M7. Namun M5 sudah di mig 000026 (`000026_eir_schema_fix`), M6 sudah 000027 + 000028. Nomor berikutnya untuk M7 = **000029**. Flag ke `data-modeler` dan `tech-lead-orchestrator` untuk konfirmasi.

---

## Schema Target

### `ecl.calc_header` (schema-fix mig 000029)

Existing di init 000001. Schema-fix yang dibutuhkan M7:

| Kolom | Status | Catatan |
|---|---|---|
| `ecl_weighted_idr NUMERIC(20,2)` | **FIX** → `NUMERIC(20,4)` | DEC-016 |
| `ecl_fl_idr NUMERIC(20,2)` | **FIX** → `NUMERIC(20,4)` | DEC-016 |
| `delta_ecl_fl_idr NUMERIC(20,2)` | **FIX** → `NUMERIC(20,4)` | DEC-016 |
| `impact_mev_good/bad/impact_pd NUMERIC(8,4)` | **FIX** → `NUMERIC(10,8)` | DEC-016 rate fields |
| `w_good/w_normal/w_bad NUMERIC(8,4)` | **FIX** → `NUMERIC(10,8)` | DEC-016 |
| `routing_path TEXT` | **ADD** | `STANDARD` / `LPS` / `LOOKTHROUGH` |
| `flag_poci BOOLEAN` | **ADD** | Sudah ada di M5 stub di `mst.instrumen`; perlu di `calc_header` juga |
| `ead_idr NUMERIC(20,4)` | **ADD** | EAD yang dipakai untuk audit trail; saat ini tidak ada di 000001 |
| `pd_used NUMERIC(10,8)` | **ADD** | PD efektif per skenario — perlu kolom utama (NORMAL atau per-skenario via detail) |
| `lgd_used NUMERIC(10,8)` | **ADD** | LGD yang dipakai |
| `net_carrying_idr NUMERIC(20,4)` | **ADD** | Net Carrying untuk Stage 3 interest: Gross − ECL_allowance_sebelumnya |
| `sealed_at TIMESTAMPTZ` | **ADD** | Di-set oleh M8 setelah seal |
| `catatan TEXT` | **ADD** | "FVTPL_SKIP_ECL", "POCI_DEFERRED", dst — routing note |
| Audit cols (`created_by`, `updated_at`, ...) | **ADD** | Konsisten db-conventions.md |
| DB trigger `fn_ecl_calc_no_modify_when_sealed` | **ADD** | Reject UPDATE setelah `sealed_at IS NOT NULL` (plan §10) |

### `ecl.calc_detail_skenario` (schema-fix mig 000029)

| Kolom | Status | Catatan |
|---|---|---|
| `pd_skenario NUMERIC(8,4)` | **FIX** → `NUMERIC(10,8)` | DEC-016 |
| `bobot NUMERIC(8,4)` | **FIX** → `NUMERIC(10,8)` | DEC-016 |
| `ecl_skenario_idr NUMERIC(20,2)` | **FIX** → `NUMERIC(20,4)` | ECL sebelum FL multiplier |
| `fl_multiplier NUMERIC(10,8)` | **ADD** | FL multiplier yang dipakai untuk skenario ini |
| `ecl_fl_idr NUMERIC(20,4)` | **ADD** | ECL setelah FL multiplier |
| `ead_skenario_idr NUMERIC(20,4)` | **ADD** | EAD per skenario (bisa beda jika FX multi-currency) |
| Audit cols | **ADD** | |

---

## Permissions

| Permission | Actor | Deskripsi |
|---|---|---|
| `ecl.compute` | ROLE-RISK (preview / ad-hoc), Asynq worker (bulk via M8) | Trigger ECL computation |
| `ecl.result.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO | Baca hasil ECL per instrumen |
| `ecl.portfolio_aggregate.read` | ROLE-RISK, ROLE-CFO, ROLE-AUDIT | Baca agregasi ECL per portofolio |
| `ecl.result.export` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Export CSV/XLSX result |

---

## Story APP-C-ECL-M7-001 — Compute ECL per Single Instrumen

**Actor**: ROLE-RISK (preview / ad-hoc debug) atau ECL engine worker (Asynq, dipanggil dari M8)
**Trigger**: ROLE-RISK meminta preview ECL untuk satu instrumen sebelum calc run resmi; atau M8 worker memanggil `ECLEngine.ComputeSingle()` per instrumen dalam bulk job
**Goal**: Hitung ECL untuk satu instrumen aktif menggunakan formula kanonik (3-stage × 3-skenario × dual FL multiplier), routing ke LPS aggregator atau look-through sesuai tipe instrumen, dan kembalikan result struct lengkap termasuk breakdown per skenario, routing path, input yang dipakai, dan warnings.

**Pre-conditions**:
- Instrumen berstatus `AKTIF`, `workflow_status = 'APPROVED'`, tidak `deleted_at`
- `klasifikasi_psak71 IN ('AC', 'FVOCI')` — FVTPL dan FVOCI equity di-skip
- Staging sudah tersedia (P4-M1 `ecl.stage_history` ada entri untuk instrumen)
- Parameter ECL aktif tersedia: `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_mev_pd` semua dengan `periodeID` relevan
- Untuk REKSADANA: `mst.fund_composition` APPROVED tersedia per `evaluationDate` (P4-M4)
- Untuk CASH/DEPOSITO: `mst.lps_coverage` tersedia (P4-M3)
- `mst.kurs` (BI JISDOR) tersedia jika instrumen FCY
- M5 `ecl.eir_amortization_schedule` tersedia (untuk accrued interest dalam EAD)

**Post-conditions** (preview mode — tidak persist ke `ecl.calc_header`):
- Result struct dikembalikan kepada caller dengan semua field terisi
- Audit event `ECL.COMPUTE_PREVIEW` ditulis ke `aud.audit_log`

**Post-conditions** (worker mode — persist):
- Baris baru di `ecl.calc_header` + 3 baris di `ecl.calc_detail_skenario` (GOOD, NORMAL, BAD), atomic
- `ecl.lookthrough_underlying` rows jika REKSADANA
- Audit event `ECL.RESULT_PERSISTED` ditulis in-transaction

**Output struct** (referensi untuk `system-analyst`):
```
ECLResult {
  instrumen_id, calc_run_id (nullable jika preview),
  evaluation_date, periode_id,
  stage,                         // STAGE_1 / STAGE_2 / STAGE_3
  routing_path,                  // STANDARD / LPS / LOOKTHROUGH
  flag_poci,                     // true → ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR warning
  ead_idr,                       // EAD setelah LPS excess jika LPS routing
  pd_used_per_skenario,          // {good, normal, bad}
  lgd_used,
  fl_multiplier_per_skenario,    // {good, normal, bad}
  bobot_skenario_snapshot,       // {good, normal, bad} — snapshot dari mst.bobot_skenario saat compute
  ecl_per_skenario_idr,          // {good, normal, bad} — sebelum FL
  ecl_fl_per_skenario_idr,       // {good, normal, bad} — setelah FL
  ecl_weighted_idr,              // Σ(ECL_FL × bobot)
  net_carrying_idr,              // untuk Stage 3; null jika Stage 1/2
  parameter_snapshot_id,         // ID snapshot param yang dipakai
  warnings: []string             // "POCI_DEFERRED", "FVTPL_SKIP", dst
}
```

**Routing logic**:
```
IF klasifikasi_psak71 = 'FVTPL' OR klasifikasi_psak71 = 'FVOCI_ELECTION'
    → skip, return ECLResult{ecl_weighted_idr: 0, routing_path: "SKIP_FVTPL"}

IF tipe_instrumen = 'REKSADANA'
    → LookThroughService.Compute() (P4-M4), routing_path = "LOOKTHROUGH"

ELSE IF tipe_instrumen IN ('CASH', 'DEPOSITO')
    → LPSService.AggregateExcess() (P4-M3), ECL pada excess only, routing_path = "LPS"

ELSE   // AC/FVOCI standard: OBLIGASI, SAHAM (AC rare), dll
    → M2 helpers: PD/LGD/EAD per skenario × FL × bobot, routing_path = "STANDARD"

IF flag_poci = TRUE (any routing)
    → emit warning "ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR", flag in result
    → TIDAK compute ECL full POCI; return POCI stub (defer Phase 5)
```

**Permissions**: `ecl.compute` (ROLE-RISK untuk preview; internal engine untuk worker)
**Audit Events**: `ECL.COMPUTE_PREVIEW` (preview mode), `ECL.RESULT_PERSISTED` (worker mode)

### Acceptance Criteria — APP-C-ECL-M7-001

```gherkin
Feature: Compute ECL per single instrumen

  Background:
    Given parameter ECL aktif untuk periodeID "JUNI-2026":
      | mst.bobot_skenario | w_good=0.25, w_normal=0.50, w_bad=0.25 |
      | mst.impact_mev_pd  | GOOD: 0.80000000, NORMAL: 1.00000000, BAD: 1.50000000 |
    And kurs BI JISDOR 2026-06-30 tersedia untuk IDR dan USD

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: instrumen OBLIGASI AC Stage 1 (STANDARD routing)
  #---------------------------------------------------------------------
  Scenario: Compute ECL — AC Stage 1, STANDARD routing, hasilnya lengkap dan presisi
    Given instrumen "OBL-2026-00001" dengan:
      | tipe_instrumen      | OBLIGASI           |
      | klasifikasi_psak71  | AC                 |
      | flag_poci           | false              |
      | stage               | STAGE_1            |
    And hasil P4-M2 helpers untuk instrumen ini:
      | EAD_IDR          | 1.000.000.000,0000 |
      | PD_Good (12M)    | 0.01600000         |
      | PD_Normal (12M)  | 0.02000000         |
      | PD_Bad (12M)     | 0.03000000         |
      | LGD              | 0.45000000         |
    When ECLEngine.ComputeSingle(ctx, "OBL-2026-00001", evaluationDate="2026-06-30", periodeID="JUNI-2026") dipanggil
    Then result.routing_path = "STANDARD"
    And result.stage = "STAGE_1"
    And result.flag_poci = false
    And result.ead_idr = 1.000.000.000,0000
    And result.ecl_per_skenario_idr.good   = 1.000.000.000 × 0.01600000 × 0.45000000 = 7.200.000,0000
    And result.ecl_per_skenario_idr.normal = 1.000.000.000 × 0.02000000 × 0.45000000 = 9.000.000,0000
    And result.ecl_per_skenario_idr.bad    = 1.000.000.000 × 0.03000000 × 0.45000000 = 13.500.000,0000
    And result.ecl_fl_per_skenario_idr.good   = 7.200.000,0000 × 0.80000000 = 5.760.000,0000
    And result.ecl_fl_per_skenario_idr.normal = 9.000.000,0000 × 1.00000000 = 9.000.000,0000
    And result.ecl_fl_per_skenario_idr.bad    = 13.500.000,0000 × 1.50000000 = 20.250.000,0000
    And result.ecl_weighted_idr = (5.760.000 × 0.25) + (9.000.000 × 0.50) + (20.250.000 × 0.25) = 11.002.500,0000
    And result.bobot_skenario_snapshot = {good:0.25000000, normal:0.50000000, bad:0.25000000}
    And result.warnings = []
    And semua nilai menggunakan shopspring/decimal (no float64)

  #---------------------------------------------------------------------
  # Skenario 2 — Stage 3: PD = 1.0, bunga pada Net Carrying
  #---------------------------------------------------------------------
  Scenario: Compute ECL — Stage 3, PD override = 1.0, net_carrying_idr terisi
    Given instrumen "OBL-2026-00002" dengan:
      | klasifikasi_psak71 | AC       |
      | stage              | STAGE_3  |
      | flag_poci          | false    |
    And hasil M2 helpers: EAD_IDR = 500.000.000,0000; LGD = 0.40000000
    And ECL allowance periode sebelumnya (dari ecl.calc_header latest) = 50.000.000,0000
    When ECLEngine.ComputeSingle dipanggil
    Then result.pd_used_per_skenario.good   = 1.00000000
    And result.pd_used_per_skenario.normal = 1.00000000
    And result.pd_used_per_skenario.bad    = 1.00000000
    And result.net_carrying_idr = 500.000.000,0000 − 50.000.000,0000 = 450.000.000,0000
    And interest revenue untuk instrumen ini = net_carrying_idr × EIR (dipakai oleh jurnal engine Phase 5)
    And result.ecl_per_skenario_idr.normal = 500.000.000 × 1.00000000 × 0.40000000 = 200.000.000,0000
    And result.routing_path = "STANDARD"
    And result.warnings = []

  #---------------------------------------------------------------------
  # Skenario 3 — LPS routing: CASH + DEPOSITO, ECL hanya pada excess
  #---------------------------------------------------------------------
  Scenario: Compute ECL — DEPOSITO, LPS routing, ECL hanya atas excess
    Given instrumen "DEP-2026-00010" dengan:
      | tipe_instrumen     | DEPOSITO |
      | klasifikasi_psak71 | AC       |
      | stage              | STAGE_1  |
      | counterparty_id    | "BANK-BCA" |
    And nasabah "NASABAH-A" memiliki total exposur di BANK-BCA = IDR 3.000.000.000,0000
    And LPS cap = IDR 2.000.000.000,0000
    And LPSService.AggregateExcess() → excess_ead_idr = IDR 1.000.000.000,0000
    When ECLEngine.ComputeSingle dipanggil untuk "DEP-2026-00010"
    Then result.routing_path = "LPS"
    And result.ead_idr = 1.000.000.000,0000 (excess only)
    And ECL dihitung hanya atas IDR 1.000.000.000,0000 (bukan 3.000.000.000)
    And result.ecl_weighted_idr > 0 dan < ECL yang dihitung atas full exposur

  #---------------------------------------------------------------------
  # Skenario 4 — LOOKTHROUGH routing: REKSADANA
  #---------------------------------------------------------------------
  Scenario: Compute ECL — REKSADANA, LOOKTHROUGH routing, delegasi ke M4
    Given instrumen "RKD-2026-00001" dengan:
      | tipe_instrumen     | REKSADANA |
      | klasifikasi_psak71 | FVOCI     |
      | stage              | STAGE_1   |
    And LookThroughService.Compute sudah dapat dipanggil dan mengembalikan:
      | ecl_weighted_idr | 17.962.500,0000 |
      | routing_path     | LOOKTHROUGH     |
    When ECLEngine.ComputeSingle dipanggil untuk "RKD-2026-00001"
    Then result.routing_path = "LOOKTHROUGH"
    And result.ecl_weighted_idr = 17.962.500,0000 (dipakai langsung dari M4 result)
    And ecl.lookthrough_underlying rows dibuat oleh M4 (tidak di-duplikasi di M7)
    And result.warnings = []

  #---------------------------------------------------------------------
  # Skenario 5 — FVTPL skip: ECL = 0, tidak ada compute
  #---------------------------------------------------------------------
  Scenario: Skip ECL — instrumen FVTPL
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When ECLEngine.ComputeSingle dipanggil untuk "SHM-2026-00001"
    Then result.ecl_weighted_idr = 0,0000
    And result.routing_path = "SKIP_FVTPL"
    And tidak ada baris baru di ecl.calc_header untuk instrumen ini (jika worker mode)
    And result.warnings = ["FVTPL_SKIP"]
    And catatan di ecl.calc_header (jika di-write) = "FVTPL_SKIP_ECL"

  #---------------------------------------------------------------------
  # Skenario 6 — POCI flag: warning emitted, compute di-stub
  #---------------------------------------------------------------------
  Scenario: POCI instrumen — flag warning, computation di-defer
    Given instrumen "OBL-POCI-00001" dengan:
      | klasifikasi_psak71 | AC        |
      | flag_poci          | true      |
    When ECLEngine.ComputeSingle dipanggil
    Then result.flag_poci = true
    And result.warnings berisi "ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR"
    And result.ecl_weighted_idr = NULL (bukan 0 — menandakan belum dihitung)
    And result.routing_path = "POCI_DEFERRED"
    And tidak ada baris baru di ecl.calc_header untuk instrumen ini
    And log WARNING: "POCI instrument OBL-POCI-00001 deferred to Phase 5"

  #---------------------------------------------------------------------
  # Skenario 7 — Error: parameter ECL tidak tersedia untuk periodeID
  #---------------------------------------------------------------------
  Scenario: Error — parameter ECL missing untuk periodeID
    Given tidak ada mst.bobot_skenario APPROVED untuk periodeID "JULI-2026"
    When ECLEngine.ComputeSingle dipanggil dengan periodeID = "JULI-2026"
    Then error dikembalikan:
      code: "ECL_PARAM_NOT_FOUND"
      message: "Parameter ECL (bobot skenario) tidak tersedia untuk periode JULI-2026. Pastikan ALCO telah menyetujui parameter."
    And tidak ada baris di ecl.calc_header

  #---------------------------------------------------------------------
  # Skenario 8 — Idempotent: re-compute sama memberikan hasil deterministik
  #---------------------------------------------------------------------
  Scenario: Deterministik — compute dua kali dengan input sama menghasilkan angka identik
    Given semua parameter dan master data tidak berubah
    When ECLEngine.ComputeSingle dipanggil dua kali berturut-turut untuk instrumen yang sama
    Then kedua result.ecl_weighted_idr identik bit-for-bit (shopspring/decimal deterministic)
    And tidak ada row duplikat di ecl.calc_header (idempotent jika calc_run_id dan periode_id sama → CONFLICT 409 di worker mode)
```

---

## Story APP-C-ECL-M7-002 — Bulk ECL Compute (Calc Run Scope)

**Actor**: ECL engine Asynq worker, dipanggil oleh P4-M8 calc run orchestrator
**Trigger**: P4-M8 job handler memanggil `ECLEngine.BulkCompute(ctx, calcRunID, periodeID, evaluationDate, scope)` setelah calc_run dibuat dan parameter snapshot di-freeze
**Goal**: Iterasi semua instrumen aktif dalam scope (atau subset), routing per instrumen ke STANDARD/LPS/LOOKTHROUGH, compute ECL, persist `ecl.calc_header` + `ecl.calc_detail_skenario` rows, aggregate summary, flag drift entries, dan report progress via Redis + SSE. Idempotent per `calc_run_id`.

**Pre-conditions**:
- `calc_run_id` sudah ada di `sys.job` / `ecl.calc_header` dengan status bukan SEALED
- `parameter_snapshot_id` sudah di-freeze (P4-M8 meng-freeze sebelum memanggil M7)
- Semua pre-conditions M7-001 terpenuhi untuk setiap instrumen dalam scope
- Tidak ada calc run SEALED untuk (periodeID, instrumenID) yang sama (CONFLICT jika ada)

**Post-conditions**:
- Untuk setiap instrumen aktif dalam scope: satu baris di `ecl.calc_header` + 3 baris di `ecl.calc_detail_skenario` (GOOD, NORMAL, BAD)
- Instrumen FVTPL / FVOCI equity → di-skip, tidak ada baris (atau baris dengan catatan FVTPL_SKIP jika di-write untuk audit trail)
- Instrumen POCI → flag di result, tidak ada ECL row, dicatat di job result
- Summary agregat tersedia di job result (`sys.job.result_jsonb`)
- `ecl.calc_header.calc_run_id` = `calcRunID` untuk semua rows periode ini

**Performance SLA**: ≤ 30 detik untuk 1.000 instrumen (P95). Batch per 100 instrumen. Progress report setiap 100 instrumen atau setiap 10 detik.

**Long-running**: UX §3 mandatory — progress via Redis pub/sub + SSE stream (dihandle oleh M8 wrapper). M7 engine melaporkan progress via callback/channel ke M8.

**Permissions**: `ecl.compute` (internal engine, dipanggil dari M8 worker context)
**Audit Events**: `ECL.COMPUTE_STARTED`, `ECL.COMPUTE_COMPLETED`, `ECL.RESULT_PERSISTED` (per batch)

### Acceptance Criteria — APP-C-ECL-M7-002

```gherkin
Feature: Bulk ECL compute untuk satu calc run

  Background:
    Given calcRunID = "CR-JUNI-2026-001", periodeID = "JUNI-2026", evaluationDate = 2026-06-30
    And parameter snapshot ID sudah di-freeze oleh M8
    And 1.000 instrumen aktif dalam scope:
      | 600 OBLIGASI AC Stage 1   | → STANDARD routing |
      | 200 DEPOSITO AC Stage 1   | → LPS routing       |
      | 100 REKSADANA FVOCI       | → LOOKTHROUGH       |
      |  80 OBLIGASI AC Stage 2   | → STANDARD routing  |
      |  15 OBLIGASI AC Stage 3   | → STANDARD routing  |
      |   5 SAHAM FVTPL           | → SKIP              |

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: bulk selesai, semua instrumen diproses
  #---------------------------------------------------------------------
  Scenario: Bulk compute selesai dalam SLA, semua instrumen diproses
    When ECLEngine.BulkCompute(ctx, calcRunID, periodeID, evaluationDate, scope="ALL_ACTIVE") dipanggil
    Then:
      1. ecl.calc_header berisi 995 rows baru dengan calc_run_id = "CR-JUNI-2026-001"
         (1000 − 5 FVTPL yang di-skip — FVTPL tidak menghasilkan baris ecl.calc_header)
      2. ecl.calc_detail_skenario berisi 2.985 rows (995 × 3 skenario)
      3. ecl.lookthrough_underlying berisi rows untuk 100 REKSADANA (dari M4)
      4. Durasi total ≤ 30 detik (P95)
      5. sys.job.result_jsonb berisi:
         | total_scanned        | 1000  |
         | total_computed       | 995   |
         | total_skipped_fvtpl  | 5     |
         | total_poci_deferred  | 0     |
         | ecl_weighted_idr_total | {aggregate IDR} |
    And audit log "ECL.COMPUTE_COMPLETED" ditulis dengan after_jsonb.total_computed = 995
    And progress callbacks dikirim setiap ~100 instrumen

  #---------------------------------------------------------------------
  # Skenario 2 — Idempotent: re-run dengan calc_run_id sama di-reject
  #---------------------------------------------------------------------
  Scenario: Idempotent re-run — CONFLICT jika calc_run_id + periode_id + instrumen_id sudah ada
    Given ecl.calc_header sudah punya row untuk (periodeID="JUNI-2026", instrumenID="OBL-001", calcRunID="CR-JUNI-2026-001")
    When BulkCompute dipanggil ulang dengan calcRunID yang sama
    Then untuk instrumen tersebut: error CONFLICT 409 dikembalikan per instrumen (tidak fail bulk)
    And bulk job melanjutkan instrumen lain
    And sys.job.result_jsonb mencatat "skipped_duplicate" count
    And aud.audit_log berisi event "ECL.COMPUTE_DUPLICATE_SKIP"

  #---------------------------------------------------------------------
  # Skenario 3 — Partial failure: satu instrumen error tidak halt bulk
  #---------------------------------------------------------------------
  Scenario: Partial failure — instrumen dengan data EAD corrupt, batch lain lanjut
    Given instrumen "OBL-2026-CORRUPT" tidak memiliki outstanding principal (data error)
    And EAD computation untuk instrumen tersebut mengembalikan error "EAD_MISSING_OUTSTANDING"
    When BulkCompute dipanggil
    Then instrumen "OBL-2026-CORRUPT" gagal, error dicatat di job result
    And instrumen lain tetap diproses
    And job selesai dengan status "completed_with_errors"
    And sys.job.result_jsonb.errors berisi:
      | instrumen_id   | "OBL-2026-CORRUPT"       |
      | error_code     | "EAD_MISSING_OUTSTANDING" |
    And ROLE-RISK mendapat notifikasi: "ECL calc run selesai dengan X error. Lihat detail job."

  #---------------------------------------------------------------------
  # Skenario 4 — SLA: 1.000 instrumen ≤ 30 detik
  #---------------------------------------------------------------------
  Scenario: Performance SLA — 1.000 instrumen dalam ≤ 30 detik
    Given 1.000 instrumen aktif dengan berbagai tipe routing
    When BulkCompute selesai
    Then sys.job.completed_at − sys.job.started_at ≤ 30.000ms
    And Prometheus metric tersedia:
      ecl_bulk_compute_duration_seconds{percentile="p95"} <= 30
      ecl_bulk_compute_instrument_count 1000

  #---------------------------------------------------------------------
  # Skenario 5 — Cancellation: job dibatalkan mid-run
  #---------------------------------------------------------------------
  Scenario: Cancellation mid-run — rows yang sudah di-persist tidak di-rollback
    Given BulkCompute sedang berjalan, sudah memproses 400 instrumen
    When M8 trigger cancellation (ctx.Done() fired)
    Then engine berhenti memproses instrumen baru setelah batch yang sedang berjalan selesai
    And 400 rows ecl.calc_header yang sudah di-commit TIDAK di-rollback
    And sys.job.status = "cancelled"
    And sys.job.result_jsonb.total_computed = 400 (partial)
    And notifikasi: "ECL calc run dibatalkan. 400 dari 1.000 instrumen sudah diproses."

  #---------------------------------------------------------------------
  # Skenario 6 — POCI instrumen dalam bulk: flag, skip, tidak gagal
  #---------------------------------------------------------------------
  Scenario: POCI instrumen dalam bulk — di-skip dengan flag, bulk tidak gagal
    Given instrumen "OBL-POCI-001" (flag_poci = true) termasuk dalam scope
    When BulkCompute dipanggil
    Then "OBL-POCI-001" di-skip dari ECL computation
    And tidak ada baris ecl.calc_header untuk instrumen tersebut
    And sys.job.result_jsonb.total_poci_deferred += 1
    And bulk job lanjut ke instrumen berikutnya
    And notifikasi ringkasan mencantumkan: "N instrumen POCI di-defer ke Phase 5"
```

---

## Story APP-C-ECL-M7-003 — Read ECL Result per Instrumen

**Actor**: ROLE-RISK, ROLE-AUDIT, ROLE-AKUN (read-only)
**Trigger**: User membuka halaman detail instrumen dan ingin melihat hasil ECL untuk periode / calc run tertentu, termasuk breakdown per skenario dan input yang dipakai
**Goal**: Menyediakan endpoint dan UI DataTable yang menampilkan `ecl.calc_header` + `ecl.calc_detail_skenario` untuk satu instrumen, dengan support sort + filter + cursor pagination + export (UX §1).

**Pre-conditions**:
- User login dengan permission `ecl.result.read`
- `ecl.calc_header` sudah ada untuk instrumen yang dimaksud (minimal satu calc run selesai)

**Post-conditions**: Read-only — tidak ada mutasi data
**Permissions**: `ecl.result.read`
**Audit Events**: Baca tidak di-audit kecuali export: `ECL.RESULT_EXPORT`

### Acceptance Criteria — APP-C-ECL-M7-003

```gherkin
Feature: Read ECL result per instrumen

  Background:
    Given instrumen "OBL-2026-00001" dengan 3 calc run untuk periodeID "JUNI-2026"
    And RISK-01 memiliki role ROLE-RISK dan permission ecl.result.read

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: baca result untuk instrumen + calc_run_id
  #---------------------------------------------------------------------
  Scenario: Baca ECL result lengkap untuk satu instrumen + calc_run_id
    When RISK-01 GET /api/v1/ecl/results?instrumen_id=OBL-2026-00001&calc_run_id=CR-JUNI-2026-001
    Then response 200 dengan:
      data.header:
        | instrumen_id        | OBL-2026-00001          |
        | stage               | STAGE_1                 |
        | routing_path        | STANDARD                |
        | ead_idr             | 1.000.000.000,0000      |
        | pd_used (normal)    | 0.02000000              |
        | lgd_used            | 0.45000000              |
        | ecl_weighted_idr    | 11.002.500,0000         |
        | flag_poci           | false                   |
        | sealed_at           | null (belum sealed)     |
      data.detail_skenario: array 3 items:
        | skenario | pd_skenario | fl_multiplier | ecl_skenario_idr | ecl_fl_idr |
        | GOOD     | 0.01600000  | 0.80000000    | 7.200.000,0000   | 5.760.000,0000 |
        | NORMAL   | 0.02000000  | 1.00000000    | 9.000.000,0000   | 9.000.000,0000 |
        | BAD      | 0.03000000  | 1.50000000    | 13.500.000,0000  | 20.250.000,0000 |
      And bobot_skenario_snapshot: {good:0.25, normal:0.50, bad:0.25}

  #---------------------------------------------------------------------
  # Skenario 2 — DataTable list per instrumen (multi-periode, multi-run)
  #---------------------------------------------------------------------
  Scenario: List semua calc result untuk instrumen, DataTable UX §1
    When RISK-01 GET /api/v1/ecl/results?instrumen_id=OBL-2026-00001&sort=evaluation_date:desc&limit=10
    Then response berisi list ecl.calc_header untuk OBL-2026-00001
    And appliedSort = [{col:"evaluation_date", dir:"desc"}]
    And pagination.nextCursor terisi jika hasMore = true
    And UI DataTable menampilkan: sort header, filter chips, Prev/Next pagination

  #---------------------------------------------------------------------
  # Skenario 3 — Filter multi-kolom
  #---------------------------------------------------------------------
  Scenario: Filter berdasarkan stage + routing_path
    When RISK-01 GET /api/v1/ecl/results?instrumen_id=OBL-2026-00001&filter[stage]=STAGE_2&filter[routing_path]=STANDARD
    Then hanya hasil dengan stage=STAGE_2 dan routing_path=STANDARD yang dikembalikan
    And appliedFilter berisi {stage:"STAGE_2", routing_path:"STANDARD"}

  #---------------------------------------------------------------------
  # Skenario 4 — Permission: ROLE-MAKER-TR ditolak
  #---------------------------------------------------------------------
  Scenario: Permission denied — ROLE-MAKER-TR tidak punya ecl.result.read
    Given MAKER-01 dengan role ROLE-MAKER-TR login
    When MAKER-01 GET /api/v1/ecl/results?instrumen_id=OBL-2026-00001
    Then response 403 dengan error code "FORBIDDEN"
    And message "Permission ecl.result.read tidak terpenuhi."

  #---------------------------------------------------------------------
  # Skenario 5 — Export CSV/XLSX sesuai UX §1
  #---------------------------------------------------------------------
  Scenario: Export hasil ECL per instrumen — CSV sesuai filter aktif
    Given filter aktif: filter[stage]=STAGE_2 menghasilkan 12 rows
    When RISK-01 klik Export CSV
    Then response: Content-Disposition attachment; filename="ecl-result-OBL-2026-00001-{date}.csv"
    And CSV berisi 12 rows (sesuai filter) + header Bahasa Indonesia
    And aud.audit_log berisi "ECL.RESULT_EXPORT" dengan after_jsonb.row_count = 12

  #---------------------------------------------------------------------
  # Skenario 6 — ROLE-AUDIT akses semua instrumen (wildcard read)
  #---------------------------------------------------------------------
  Scenario: ROLE-AUDIT dapat membaca ECL result semua instrumen lintas portofolio
    Given AUDIT-01 dengan role ROLE-AUDIT
    When AUDIT-01 GET /api/v1/ecl/results?sort=instrumen_id:asc
    Then response 200 berisi semua instrumen yang punya ecl.calc_header
    And tidak ada filtering per portofolio diterapkan (AUDIT = cross-portfolio read)
    And tidak ada tombol aksi mutasi (read-only view)
```

---

## Story APP-C-ECL-M7-004 — Portfolio Aggregation Summary

**Actor**: ROLE-RISK, ROLE-CFO
**Trigger**: User membuka halaman ringkasan ECL untuk suatu portofolio + calc_run_id, atau dari dashboard ECL Run Detail (P4-M10)
**Goal**: Menampilkan agregasi ECL per stage per portofolio, perbandingan dengan calc run sebelumnya (delta ECL), dan roll-forward CKPN (opening + originations − derecognitions ± transfers ± remeasurements = closing). Input untuk laporan formal (Phase 6).

**Pre-conditions**:
- User login dengan permission `ecl.portfolio_aggregate.read`
- `ecl.calc_header` sudah ada untuk portofolio + calc_run_id yang dimaksud
- Untuk roll-forward: calc run periode sebelumnya sudah ada (jika tidak ada → opening = 0)

**Post-conditions**: Read-only aggregation query (tidak ada tabel baru — computed dari `ecl.calc_header`)
**Permissions**: `ecl.portfolio_aggregate.read`
**Audit Events**: `ECL.PORTFOLIO_AGGREGATE_READ` (setiap panggilan), `ECL.PORTFOLIO_AGGREGATE_EXPORT`

### Acceptance Criteria — APP-C-ECL-M7-004

```gherkin
Feature: Portfolio aggregation ECL summary

  Background:
    Given portofolio "PTF-OBLIGASI-IDR" berisi 80 instrumen OBLIGASI AC aktif
    And calc_run_id = "CR-JUNI-2026-001" (sudah selesai compute)
    And calc_run_id = "CR-MEI-2026-001" (periode sebelumnya, sudah sealed)
    And RISK-01 memiliki permission ecl.portfolio_aggregate.read

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: aggregasi per stage + perbandingan prior run
  #---------------------------------------------------------------------
  Scenario: Baca portfolio aggregation summary — per stage + delta vs prior run
    When RISK-01 GET /api/v1/ecl/portfolio-summary?portofolio_id=PTF-OBLIGASI-IDR&calc_run_id=CR-JUNI-2026-001
    Then response 200 dengan:
      data.summary_by_stage:
        | stage   | count | ead_total_idr | ecl_weighted_total_idr | delta_vs_prior_idr |
        | STAGE_1 | 65    | {aggregate}   | {aggregate}            | {delta}            |
        | STAGE_2 | 12    | {aggregate}   | {aggregate}            | {delta}            |
        | STAGE_3 | 3     | {aggregate}   | {aggregate}            | {delta}            |
        | TOTAL   | 80    | {aggregate}   | {aggregate}            | {delta}            |
      data.prior_calc_run_id = "CR-MEI-2026-001"
      data.ecl_weighted_total_idr = Σ(all 80 instrumen ecl.calc_header.ecl_weighted_idr)
    And semua delta_vs_prior_idr dihitung: ecl_juni − ecl_mei per stage

  #---------------------------------------------------------------------
  # Skenario 2 — Roll-forward CKPN
  #---------------------------------------------------------------------
  Scenario: Roll-forward CKPN — formula kanonik opening + movements = closing
    When RISK-01 GET /api/v1/ecl/portfolio-summary/roll-forward?portofolio_id=PTF-OBLIGASI-IDR&calc_run_id=CR-JUNI-2026-001
    Then response 200 dengan:
      data.roll_forward:
        | opening_ecl_idr         | {ECL closing periode sebelumnya (CR-MEI-2026-001)} |
        | new_originations_idr    | {ECL instrumen baru yang masuk di periode JUNI}     |
        | derecognitions_idr      | {ECL instrumen yang jatuh tempo / dijual di JUNI}   |
        | transfers_to_stage2_idr | {ECL transfer Stage 1 → Stage 2}                   |
        | transfers_to_stage3_idr | {ECL transfer Stage 2 → Stage 3}                   |
        | transfers_from_stage2_idr | {ECL cure Stage 2 → Stage 1}                      |
        | remeasurements_idr      | {Perubahan ECL tanpa transfer stage}               |
        | closing_ecl_idr         | {= ecl_weighted_total_idr untuk CR-JUNI-2026-001}  |
      And closing_ecl_idr = opening + originations − derecognitions ± transfers ± remeasurements
      And reconcile check: |closing − Σecl.calc_header| < IDR 1,0000 (tolerance rounding)

  #---------------------------------------------------------------------
  # Skenario 3 — Tidak ada prior calc run (periode pertama): opening = 0
  #---------------------------------------------------------------------
  Scenario: Roll-forward saat tidak ada prior calc run — opening = 0
    Given tidak ada ecl.calc_header untuk portofolio + periode sebelum JUNI-2026
    When GET roll-forward untuk CR-JUNI-2026-001
    Then data.roll_forward.opening_ecl_idr = 0,0000
    And catatan: "Tidak ada calc run sebelumnya. Opening = 0 (periode pertama)."
    And closing_ecl_idr = new_originations_idr (semua instrumen dianggap origination baru)

  #---------------------------------------------------------------------
  # Skenario 4 — Permission denied: ROLE-AKUN tidak punya portfolio aggregate
  #---------------------------------------------------------------------
  Scenario: ROLE-AKUN tidak memiliki ecl.portfolio_aggregate.read
    Given AKUN-01 dengan role ROLE-AKUN
    When AKUN-01 GET /api/v1/ecl/portfolio-summary?portofolio_id=PTF-OBLIGASI-IDR
    Then response 403 dengan error code "FORBIDDEN"

  #---------------------------------------------------------------------
  # Skenario 5 — Export ringkasan per portofolio
  #---------------------------------------------------------------------
  Scenario: Export portfolio summary CSV
    When RISK-01 klik Export CSV pada halaman portfolio summary
    Then file CSV ter-download dengan:
      | baris ringkasan per stage |
      | baris roll-forward         |
      | header row Bahasa Indonesia |
    And aud.audit_log berisi "ECL.PORTFOLIO_AGGREGATE_EXPORT" dengan portofolio_id + calc_run_id
```

---

## Story APP-C-ECL-M7-005 — Recompute Single Instrumen Ad-Hoc

**Actor**: ROLE-RISK
**Trigger**: ROLE-RISK menemukan anomali pada hasil ECL instrumen tertentu (setelah correction data master / parameter / melihat drill-down) dan ingin melihat perbandingan antara ECL yang tersimpan dengan ECL baru berdasarkan data terkini — tanpa menyentuh calc run resmi
**Goal**: Jalankan `ECLEngine.ComputeSingle()` on-the-fly untuk instrumen yang dipilih (preview mode — tidak persist), tampilkan perbandingan side-by-side dengan hasil `ecl.calc_header` yang tersimpan. Memberikan transparansi dan kemampuan debug tanpa mengganggu integritas calc run.

**Pre-conditions**:
- User login dengan permission `ecl.compute` (ROLE-RISK)
- Instrumen target memiliki setidaknya satu baris `ecl.calc_header` tersimpan
- Parameter ECL aktif tersedia

**Post-conditions**: Tidak ada mutasi `ecl.calc_header` atau `ecl.calc_detail_skenario` — preview only
**Permissions**: `ecl.compute` (ROLE-RISK)
**Audit Events**: `ECL.RECOMPUTE_AD_HOC` per setiap panggilan (audit trail debug activity)

### Acceptance Criteria — APP-C-ECL-M7-005

```gherkin
Feature: Recompute single instrumen ad-hoc untuk debug

  Background:
    Given instrumen "OBL-2026-00001" memiliki ecl.calc_header tersimpan:
      | calc_run_id         | CR-JUNI-2026-001     |
      | ecl_weighted_idr    | 11.002.500,0000      |
      | pd_used (normal)    | 0.02000000           |
    And RISK-01 memiliki role ROLE-RISK dan permission ecl.compute

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: recompute + comparison
  #---------------------------------------------------------------------
  Scenario: Ad-hoc recompute — menampilkan perbandingan vs tersimpan
    Given mst.pd_pefindo untuk instrumen telah di-update (setelah Pefindo feed baru):
      PD_Normal sekarang = 0.02500000 (naik dari 0.02000000)
    When RISK-01 POST /api/v1/ecl/recompute dengan body:
      { "instrumen_id": "OBL-2026-00001", "evaluation_date": "2026-06-30", "periode_id": "JUNI-2026" }
    Then response 200 dengan:
      data.recomputed:
        | ecl_weighted_idr | {nilai baru dengan PD 0.025} |
        | pd_used.normal   | 0.02500000                   |
      data.stored:
        | ecl_weighted_idr | 11.002.500,0000              |
        | pd_used.normal   | 0.02000000                   |
      data.delta:
        | ecl_weighted_delta_idr | {recomputed - stored}   |
    And tidak ada baris baru di ecl.calc_header (preview mode)
    And aud.audit_log berisi "ECL.RECOMPUTE_AD_HOC" dengan:
      | entity_type        | mst.instrumen                 |
      | entity_id          | OBL-2026-00001                |
      | actor_user_id      | RISK-01                       |
      | after_jsonb.delta  | {ecl_weighted_delta_idr}      |

  #---------------------------------------------------------------------
  # Skenario 2 — Recompute instrumen yang belum punya stored result
  #---------------------------------------------------------------------
  Scenario: Recompute tanpa stored result — recomputed tersedia, data.stored = null
    Given instrumen "OBL-2026-00099" belum pernah di-compute (tidak ada ecl.calc_header)
    When RISK-01 POST recompute untuk "OBL-2026-00099"
    Then response 200 dengan:
      data.recomputed: {nilai ECL baru}
      data.stored: null
      data.delta: null
    And warning dalam response: "Tidak ada stored result untuk instrumen ini. Ini merupakan compute pertama."

  #---------------------------------------------------------------------
  # Skenario 3 — Recompute instrumen SEALED: stored data ditampilkan, delta dihitung
  #---------------------------------------------------------------------
  Scenario: Recompute instrumen dalam sealed calc run — delta vs sealed ditampilkan, tidak ada modifikasi
    Given "OBL-2026-00001" dalam sealed calc run (ecl.calc_header.sealed_at IS NOT NULL)
    When RISK-01 POST recompute
    Then data.stored berisi nilai sealed (ecl_weighted_idr dari sealed run)
    And data.recomputed berisi nilai baru (on-the-fly, tidak di-persist)
    And warning dalam response: "Stored result ini bagian dari sealed calc run CR-JUNI-2026-001. Perubahan hanya efektif pada calc run baru."
    And aud.audit_log berisi "ECL.RECOMPUTE_AD_HOC" dengan after_jsonb.is_sealed_comparison = true

  #---------------------------------------------------------------------
  # Skenario 4 — Error: permission denied untuk ROLE-AKUN
  #---------------------------------------------------------------------
  Scenario: ROLE-AKUN tidak boleh ad-hoc recompute
    Given AKUN-01 dengan role ROLE-AKUN (tidak memiliki ecl.compute)
    When AKUN-01 POST /api/v1/ecl/recompute
    Then response 403 dengan error code "FORBIDDEN"
    And message "Permission ecl.compute tidak terpenuhi. Hanya ROLE-RISK yang dapat melakukan recompute ad-hoc."

  #---------------------------------------------------------------------
  # Skenario 5 — Recompute POCI instrumen: warning dihasilkan, delta tidak tersedia
  #---------------------------------------------------------------------
  Scenario: Ad-hoc recompute instrumen POCI — warning + no delta
    Given instrumen "OBL-POCI-001" dengan flag_poci = true
    When RISK-01 POST recompute untuk "OBL-POCI-001"
    Then response 200 dengan:
      data.recomputed.ecl_weighted_idr = null
      data.recomputed.warnings = ["ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR"]
      data.recomputed.routing_path = "POCI_DEFERRED"
      data.delta = null
```

---

## Story APP-C-ECL-M7-006 — POCI Handling Stub (Phase 5 Defer)

**Actor**: ECL engine (internal), ROLE-RISK (untuk visibilitas warning)
**Trigger**: `ECLEngine.ComputeSingle()` atau `BulkCompute()` menemukan instrumen dengan `mst.instrumen.flag_poci = TRUE`
**Goal**: Mengidentifikasi instrumen POCI secara akurat, meng-emit warning level error code `ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR`, skip ECL computation full (defer ke Phase 5), dan menyediakan test vectors POCI sebagai dokumentasi untuk Phase 5 implementasi. Instrumen POCI tidak boleh menghasilkan ECL = 0 secara diam-diam — harus eksplisit flagged.

**Latar Belakang POCI (per IFRS9 §5.4.1(c))**:
POCI (Purchased or Originated Credit Impaired) adalah instrumen yang sudah credit-impaired pada saat perolehan. EIR yang dihitung harus berupa credit-adjusted EIR (cashflow expectasi sudah PD-adjusted sejak inisiasi). ECL movement direkognisi langsung di P&L (tidak ada Stage 1). Full implementation di-defer ke Phase 5.

**Pre-conditions**: Instrumen memiliki `mst.instrumen.flag_poci = TRUE`
**Post-conditions**: Tidak ada baris `ecl.calc_header` untuk instrumen POCI; job result mencatat count POCI deferred
**Permissions**: N/A (internal engine behavior)
**Audit Events**: `ECL.POCI_DEFERRED` per instrumen POCI yang dijumpai

### Acceptance Criteria — APP-C-ECL-M7-006

```gherkin
Feature: POCI instrumen handling stub — defer Phase 5

  Background:
    Given instrumen "OBL-POCI-001" dengan:
      | mst.instrumen.flag_poci          | true          |
      | ecl.eir_amortization_schedule.flag_poci | true   |
      | klasifikasi_psak71               | AC            |
      | stage (dari staging engine)      | STAGE_3       |

  #---------------------------------------------------------------------
  # Skenario 1 — Happy path: POCI terdeteksi, warning emitted, skip clean
  #---------------------------------------------------------------------
  Scenario: ECL engine mendeteksi POCI, skip dengan warning eksplisit
    When ECLEngine.ComputeSingle dipanggil untuk "OBL-POCI-001"
    Then engine TIDAK memanggil LookupPD() atau ComputeEAD() untuk instrumen ini
    And result.flag_poci = true
    And result.ecl_weighted_idr = NULL (bukan 0 — perbedaan semantik penting)
    And result.routing_path = "POCI_DEFERRED"
    And result.warnings = ["ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR"]
    And aud.audit_log berisi "ECL.POCI_DEFERRED" dengan:
      | entity_id     | OBL-POCI-001 |
      | after_jsonb.flag_poci | true   |
      | after_jsonb.deferred_to | "PHASE_5" |
    And tidak ada baris baru di ecl.calc_header untuk instrumen ini

  #---------------------------------------------------------------------
  # Skenario 2 — Bulk compute: POCI di-skip, bulk lanjut, count dicatat
  #---------------------------------------------------------------------
  Scenario: Dalam bulk compute, instrumen POCI di-skip tanpa menghentikan batch
    Given 3 instrumen POCI dalam scope 1.000 instrumen
    When BulkCompute dipanggil
    Then ketiga POCI instrumen di-skip
    And bulk lanjut ke instrumen berikutnya untuk setiap POCI yang di-skip
    And sys.job.result_jsonb.total_poci_deferred = 3
    And ROLE-RISK mendapat notifikasi ringkasan: "3 instrumen POCI di-defer ke Phase 5. Tidak ada ECL yang dihitung untuk instrumen tersebut."

  #---------------------------------------------------------------------
  # Skenario 3 — Test vector POCI (dokumentasi Phase 5)
  #---------------------------------------------------------------------
  Scenario: Test vector POCI — dokumentasi expected behavior Phase 5
    # Catatan: skenario ini BUKAN executable test untuk M7 (engine belum compute POCI).
    # Ini adalah dokumentasi expected behavior yang harus dipenuhi Phase 5.
    #
    # GIVEN instrumen OBL-POCI-001:
    #   nominal = IDR 1.000.000.000
    #   credit_adjusted_EIR (credit-adjusted, dengan PD-adjusted cashflow) = 0.06500000
    #   PD adjusted cashflow dihitung dengan asumsi PD_at_inception = 0.30 (sudah impaired)
    #
    # EXPECTED (Phase 5):
    #   credit_adjusted_EIR = IRR solver dengan cashflow: CF_t = CF_contractual_t × (1 - PD_at_inception)
    #   ECL_Stage_3 = EAD × 1.0 × LGD (PD = 1.0 karena POCI sudah credit-impaired sejak awal)
    #   ECL_movement = langsung ke P&L (tidak melalui OCI untuk AC POCI)
    #   Interest revenue = Net Carrying × credit_adjusted_EIR (bukan contractual EIR)
    #
    # CONSTRAINT yang harus di-enforce Phase 5:
    #   credit_adjusted_EIR HARUS ≤ contractual EIR (karena PD-adjusted cashflow lebih kecil)
    #   Jika credit_adjusted_EIR > contractual EIR → error "POCI_EIR_INTEGRITY_VIOLATION"
    Given instrumen POCI dengan credit_adjusted_EIR = 0.06500000 dan contractual_EIR = 0.07000000
    Then credit_adjusted_EIR (0.065) < contractual_EIR (0.070) → OK (valid)
    And Phase 5 implementasi HARUS menolak jika credit_adjusted_EIR > contractual_EIR

  #---------------------------------------------------------------------
  # Skenario 4 — flag_poci konsistensi antara mst.instrumen dan eir_amortization_schedule
  #---------------------------------------------------------------------
  Scenario: flag_poci inconsistency detection — mst.instrumen vs eir_amortization_schedule mismatch
    Given mst.instrumen.flag_poci = true
    And ecl.eir_amortization_schedule.flag_poci = false untuk instrumen yang sama
    When ECLEngine.ComputeSingle dipanggil
    Then log WARNING ditulis: "POCI flag mismatch for instrumen OBL-POCI-001: mst.instrumen.flag_poci=true, eir_amortization_schedule.flag_poci=false. Using instrumen flag. Please investigate."
    And engine tetap menggunakan mst.instrumen.flag_poci = true sebagai sumber kebenaran
    And audit event "ECL.POCI_FLAG_MISMATCH" ditulis dengan detail inconsistency

  #---------------------------------------------------------------------
  # Skenario 5 — Ad-hoc recompute POCI: sama dengan ComputeSingle (skip + warning)
  #---------------------------------------------------------------------
  Scenario: ROLE-RISK mencoba recompute ad-hoc instrumen POCI via UI — mendapat warning jelas
    Given "OBL-POCI-001" dengan flag_poci = true
    When RISK-01 POST /api/v1/ecl/recompute untuk "OBL-POCI-001"
    Then response 200 (bukan error — ini adalah expected behavior) dengan:
      data.recomputed.ecl_weighted_idr = null
      data.recomputed.warnings = ["ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR"]
    And UI menampilkan warning banner:
      "Instrumen ini adalah POCI (Purchased or Originated Credit Impaired). Perhitungan ECL penuh membutuhkan credit-adjusted EIR yang akan diimplementasikan pada Phase 5. Saat ini ECL belum tersedia untuk instrumen ini."
    And tidak ada exception / 500 error yang dilempar
```

---

## Ringkasan Open Questions (M7-spesifik)

| ID | Pertanyaan | Default Sementara | Butuh Konfirmasi | Blocking? |
|---|---|---|---|---|
| **OQ-M7-1** | Nomor migration M7: plan menyebut 000026, tapi M5+M6 sudah pakai 000026–000028. Konfirmasi nomor = **000029**? | 000029 | `data-modeler` + `tech-lead-orchestrator` | Ya, sebelum data-modeler mulai |
| **OQ-M7-2** | `ecl.calc_header` kolom `calc_run_id` merujuk ke `sys.job_run_history(id)` di init schema — apakah ini sudah di-update ke `sys.job(id)` (tabel dari P4-M4/M8 infra)? Cek migration chain. | Merujuk ke `sys.job.id` (bukan `sys.job_run_history`). Konfirmasi di schema-fix mig 000029. | `data-modeler` review 000001 FK + migration chain | Ya |
| **OQ-M7-3** | Stage 3 net carrying: `ecl.calc_header` tidak memiliki kolom `ecl_allowance_balance` eksplisit. Nilai ECL allowance sebelumnya diambil dari mana? Plan §7 OQ-F menyebut "dari `ecl.calc_header.ecl_fl_idr` calc run terbaru sebelum periode evaluasi". Apakah ini confirmed? | Ambil `MAX(ecl.calc_header.ecl_fl_idr)` untuk instrumen + periode terbaru sebelum evaluationDate, dengan syarat `sealed_at IS NOT NULL`. Jika tidak ada (first run) → net_carrying = gross_carrying (ECL allowance = 0). | `ifrs9-compliance-reviewer` + `ecl-eir-engineer` | **BLOCKING** — mempengaruhi Stage 3 interest calculation |
| **OQ-M7-4** | OQ-A dari plan (FL multiplier semantik): apakah FL multiplier per skenario dari `mst.impact_mev_pd` di-apply ke semua 3 skenario secara flat (GOOD pakai row GOOD, NORMAL pakai row NORMAL, BAD pakai row BAD), atau ada tabel `mst.impact_pd` flat yang di-apply di atas semua skenario? | Tabel `mst.impact_mev_pd` memiliki row per skenario. `ECL_FL_skenario = ECL_skenario × impact_mev_pd[skenario].impact_multiplier`. Tidak ada double-multiply dengan `mst.impact_pd` yang berbeda. Konfirmasi resolusi OQ-A plan. | `ifrs9-compliance-reviewer` + ALCO | **BLOCKING** — mempengaruhi formula ECL |
| **OQ-M7-5** | FVTPL skip: apakah engine menulis baris `ecl.calc_header` dengan `ecl_weighted_idr = 0` dan `catatan = 'FVTPL_SKIP_ECL'` untuk audit trail, atau benar-benar tidak menulis baris sama sekali? | Tidak menulis baris (skip murni). Audit trail cukup dari job result JSON `total_skipped_fvtpl`. | `ifrs9-compliance-reviewer` | Ya, sebelum implementasi |
| **OQ-M7-6** | Roll-forward granularity (M7-004): apakah klasifikasi "originations", "derecognitions", "transfers", "remeasurements" mengacu pada perbandingan `mst.instrumen.status` (AKTIF vs JATUH_TEMPO vs DIJUAL) antara dua periode, atau ada tabel lifecycle event yang lebih granular? Phase 5 (APP-B) belum selesai. | Sementara: originations = instrumen yang ada di periode ini tapi tidak ada di periode sebelumnya. Derecognitions = instrumen di periode sebelumnya tapi tidak ada periode ini. Transfers = perubahan stage. Remeasurements = perubahan ECL tanpa transfer stage. Konfirmasi sumber data lifecycle event dengan Phase 5 scope. | `tech-lead-orchestrator` + Phase 5 planning | Tidak blocking M7, tapi roll-forward (M11) bergantung pada ini |
| **OQ-M7-7** | Catch-up adjustment dari M6 amendment: M6 menyimpan `catch_up_adjustment` di `ecl.eir_reestimation_log`. M7 harus membaca nilai ini untuk EAD yang tepat (accrued interest sudah amended). Apakah M7 engine langsung baca `ecl.eir_reestimation_log` atau via M5 EIR service interface? | Via M5 `EIRService.GetActiveSchedule()` yang sudah memperhitungkan versi schedule terbaru (termasuk post-amendment rows). Catch-up booking ke jurnal = Phase 5. M7 hanya pakai closing_carrying dari schedule aktif (post-amendment). | `ecl-eir-engineer` konfirmasi interface | Ya, perlu clarity sebelum implementasi |

---

## Ringkasan Data References

| Story | Tabel Read | Tabel Write | Permission |
|---|---|---|---|
| M7-001 (compute single) | `mst.instrumen`, `ecl.stage_history`, `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_mev_pd`, `mst.kurs`, `ecl.eir_amortization_schedule`, `mst.lps_coverage`, `mst.fund_composition` | `ecl.calc_header`, `ecl.calc_detail_skenario`, `ecl.lookthrough_underlying` (via M4), `aud.audit_log` | `ecl.compute` |
| M7-002 (bulk compute) | Sama dengan M7-001 (semua instrumen scope) | Sama dengan M7-001 (batch insert), `sys.job` (progress update) | `ecl.compute` (internal) |
| M7-003 (read result) | `ecl.calc_header`, `ecl.calc_detail_skenario` | `aud.audit_log` (export only) | `ecl.result.read` |
| M7-004 (portfolio agg) | `ecl.calc_header`, `mst.instrumen`, `mst.portofolio` | `aud.audit_log` (export only) | `ecl.portfolio_aggregate.read` |
| M7-005 (recompute ad-hoc) | Sama dengan M7-001 (read only) + `ecl.calc_header` (untuk comparison stored) | `aud.audit_log` (`ECL.RECOMPUTE_AD_HOC`) | `ecl.compute` |
| M7-006 (POCI stub) | `mst.instrumen.flag_poci`, `ecl.eir_amortization_schedule.flag_poci` | `aud.audit_log` (`ECL.POCI_DEFERRED`) | internal (no HTTP) |

---

## Ringkasan Audit Events

| Event | Kapan | Actor |
|---|---|---|
| `ECL.COMPUTE_PREVIEW` | Preview/ad-hoc compute tidak persist | ROLE-RISK |
| `ECL.RESULT_PERSISTED` | Setiap batch persist dalam bulk (per batch ~100 instrumen) | System (worker) |
| `ECL.COMPUTE_STARTED` | Awal bulk compute job | System (Asynq) |
| `ECL.COMPUTE_COMPLETED` | Selesai bulk compute job | System (Asynq) |
| `ECL.COMPUTE_DUPLICATE_SKIP` | Skip karena instrumen sudah punya result untuk calc_run_id yang sama | System |
| `ECL.POCI_DEFERRED` | Setiap instrumen POCI yang di-skip | System |
| `ECL.POCI_FLAG_MISMATCH` | Inkonsistensi flag_poci antara instrumen dan schedule | System |
| `ECL.RECOMPUTE_AD_HOC` | ROLE-RISK trigger recompute manual | ROLE-RISK |
| `ECL.RESULT_EXPORT` | Export CSV/XLSX result per instrumen | ROLE-RISK / ROLE-AUDIT |
| `ECL.PORTFOLIO_AGGREGATE_READ` | Setiap akses portfolio summary | ROLE-RISK / ROLE-CFO |
| `ECL.PORTFOLIO_AGGREGATE_EXPORT` | Export portfolio summary | ROLE-RISK / ROLE-CFO |

---

## Matriks Permissions

| Story | Permission | Actor | MFA |
|---|---|---|---|
| M7-001, M7-005 | `ecl.compute` | ROLE-RISK, internal engine | Tidak |
| M7-002 | `ecl.compute` | internal engine (Asynq), dipanggil dari M8 yang punya MFA step-up untuk seal | Tidak untuk compute; Ya untuk seal (M8) |
| M7-003 | `ecl.result.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO | Tidak |
| M7-004 | `ecl.portfolio_aggregate.read` | ROLE-RISK, ROLE-CFO, ROLE-AUDIT | Tidak |
| M7-003 (export) | `ecl.result.export` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Tidak |

---

## Handoff Checklist

```
P4-M7 stories selesai →

  system-analyst:
    - OpenAPI fragment: ecl-engine.yaml
      Endpoint baru:
        POST /api/v1/ecl/compute-preview        (M7-001 preview mode)
        POST /api/v1/ecl/recompute              (M7-005 ad-hoc)
        GET  /api/v1/ecl/results                (M7-003 DataTable)
        GET  /api/v1/ecl/results/{id}           (M7-003 single result)
        GET  /api/v1/ecl/portfolio-summary      (M7-004)
        GET  /api/v1/ecl/portfolio-summary/roll-forward (M7-004)
      Go interface contract untuk ecl-eir-engineer:
        interface ECLEngine {
          ComputeSingle(ctx, instrumenID, evaluationDate, periodeID, opts) (ECLResult, error)
          BulkCompute(ctx, calcRunID, periodeID, evaluationDate, scope, progressFn) (BulkResult, error)
        }
      Error codes baru: ECL_POCI_REQUIRES_FULL_CREDIT_ADJUSTED_EIR, ECL_PARAM_NOT_FOUND,
                        ECL_ROUTING_UNKNOWN, POCI_FLAG_MISMATCH

  data-modeler (PARALEL dengan system-analyst):
    - migration 000029: ecl_calc_header_schema_fix
      - ALTER TABLE ecl.calc_header: precision fixes + ADD COLUMN routing_path, flag_poci,
        ead_idr, pd_used, lgd_used, net_carrying_idr, sealed_at, catatan + audit cols
      - ALTER TABLE ecl.calc_detail_skenario: precision fixes + ADD COLUMN fl_multiplier,
        ecl_fl_idr, ead_skenario_idr + audit cols
      - ADD TRIGGER fn_ecl_calc_no_modify_when_sealed (BEFORE UPDATE on ecl.calc_header
        WHERE sealed_at IS NOT NULL → RAISE EXCEPTION)
      - ADD INDEX idx_ecl_calc_routing_path, idx_ecl_calc_flag_poci
      - NO hard delete trigger untuk ecl.calc_header + ecl.calc_detail_skenario (DEC-018)
      Konfirmasi: OQ-M7-1 (nomor mig), OQ-M7-2 (FK calc_run_id)

  ecl-eir-engineer:
    - Implementasi backend/internal/ecl/engine/:
        domain.go   — ECLResult, BulkResult, routing constants, POCI stub
        routing.go  — routing logic (STANDARD / LPS / LOOKTHROUGH / SKIP_FVTPL / POCI_DEFERRED)
        formula.go  — ECL formula kanonik (tidak boleh punya float64, semua shopspring/decimal)
        service.go  — ECLEngine interface implementation
        repo.go     — persist to ecl.calc_header + ecl.calc_detail_skenario (batch insert)
    - Wiring ke M1 StagingService, M2 PD/LGD/EAD, M3 LPS, M4 LookThrough, M5 EIR schedule
    - Konfirmasi OQ-M7-3 (net carrying lookup), OQ-M7-4 (FL multiplier semantik)

  uiux-designer (PARALEL):
    - Desain halaman /ecl/results (DataTable UX §1: sort+filter+cursor+export)
    - Desain drill-down per instrumen: header summary + skenario breakdown table
    - Desain halaman /ecl/portfolio-summary: stage summary card + roll-forward waterfall
    - Desain recompute panel + comparison side-by-side (M7-005)
    - Desain POCI warning banner (M7-006)

  ifrs9-compliance-reviewer:
    BLOCKING gate sebelum merge PR P4-M7:
    - Verify formula kanonik per plan §5:
        ECL_skenario = EAD × PD_skenario × LGD           ✓
        ECL_FL = ECL_skenario × impact_multiplier_skenario ✓
        ECL_weighted = Σ(ECL_FL × bobot)                  ✓
    - Verify Stage 3: PD = 1.0, bunga pada net carrying (DEC-010 + OQ-F plan)
    - Verify FVTPL + FVOCI equity skip (OQ-G plan resolved)
    - Verify OQ-M7-3 (net carrying source) — BLOCKING
    - Verify OQ-M7-4 (FL multiplier semantik — resolusi OQ-A plan) — BLOCKING
    - Verify NUMERIC(20,4) IDR, NUMERIC(10,8) PD/LGD throughout (DEC-016) — no float64
    - Verify ecl.calc_header append-only, sealed trigger benar (DEC-018)
    - Verify POCI stub: NULL bukan 0 untuk ecl_weighted_idr (semantik penting)
    - Verify bobot_skenario_snapshot disimpan di ecl.calc_header pada saat compute
      (parameter snapshot — penting untuk audit kemudian jika ALCO override bobot)
```
