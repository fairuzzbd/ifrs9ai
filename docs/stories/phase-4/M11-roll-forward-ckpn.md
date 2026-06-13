# P4-M11 — Roll-Forward CKPN: User Stories

**Story Set ID**: P4-M11
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 4)
**Status**: DRAFT — gate: `ifrs9-compliance-reviewer` BLOCKING (menyentuh ECL formula + PSAK 71 §5.5 disclosure).
**Author**: business-analyst
**Tanggal**: 2026-06-13
**Branch target**: `feature/app-c-roll-forward-ckpn`

**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §5 (calc run orchestration), §3 (staging + transfers), §6 (seal); FSD-BLIPS-MASTER-v1.1.docx §9 (PSAK 71 §5.5 disclosure)
**Linked BRD**: BRD §8.3 (CKPN Roll-Forward), §9.4 (Laporan posisi reconcile), RACI: ROLE-RISK (R/compute), ROLE-AKUN-CTL (R/review), ROLE-CFO (A/approve disclosure), ROLE-ALCO (I), ROLE-AUDIT (I/read-only)

**Linked Decision Log**:
- DEC-010 — ECL formula: 3-stage × 3-skenario × dual FL. Roll-forward adalah downstream-read dari `ecl.calc_header`.
- DEC-016 — NUMERIC: IDR `NUMERIC(20,4)`, PD/LGD `NUMERIC(10,8)`. Semua komponan roll-forward = `NUMERIC(20,4)`.
- DEC-017 — Workflow: tidak ada 4-eyes untuk read-query roll-forward; 4-eyes hanya untuk export yang dipakai sebagai lampiran resmi disclosure.
- DEC-018 — `ecl.*` no hard delete. Roll-forward hanya read.
- DEC-021 — Idempotency-Key wajib pada compute endpoint (POST/trigger).
- DEC-022 — Pagination cursor only.
- UX §1 — DataTable: sort + cursor + filter + export wajib.
- UX §2 — Form notification: sukses/gagal eksplisit.
- UX §3 — Long-running: jika roll-forward computation > 2 detik → Asynq job + SSE.

**Depends on (harus MERGED sebelum M11 dimulai)**:
- P4-M7 merged (PR #72) — `ecl.calc_header`, `ecl.calc_detail_skenario`, `ecl.stage_history`, `GET /ecl/portfolio-summary/roll-forward` (PARTIAL stub)
- P4-M8 merged (PR #76) — `ecl.calc_run`, sealing lifecycle, sealed snapshot sebagai anchor point

**Phase boundary**:
- M11 implements **proper** stage transfer detection (replace M7 PARTIAL_PHASE_5_DEFER approach).
- M11 implements **basic** origination/derecognition detection dari `mst.instrumen.status` + `ecl.calc_header` presence/absence — **tidak** bergantung pada APP-B transaction lifecycle (Phase 5). Keterbatasan ini di-expose di UI sebagai `detection_method: BASIC_STATUS_DIFF`.
- Full lifecycle-event-based origination/derecognition = Phase 5 enhancement.

**Handoff berikutnya**:
- `system-analyst` — OpenAPI contract `GET /ecl/roll-forward`, `POST /ecl/roll-forward/compute` (jika cached computation), `GET /ecl/roll-forward/export`, state machine (DRAFT → COMPUTED → EXPORTED)
- `ifrs9-compliance-reviewer` — BLOCKING gate: verify formula reconcile invariant, PSAK 71 §5.5 disclosure format
- `qa-engineer` — UAT scripts: reconcile check, edge case opening=0, stage transfer detection accuracy

---

## Formula Kanonik (reference: `.claude/memory/formulas.md` §Roll-forward)

```
ECL_closing = ECL_opening
            + Σ transfers_to_stage(1→2, 2→3, 1→3, 3→2, 3→1)
            − Σ transfers_from_stage(same bucket, sign baked into transfers)
            + new_originations
            − derecognitions
            ± remeasurements

Reconcile invariant:
  |ECL_closing − Σ ecl.calc_header.ecl_fl_idr (current run)| < IDR 1.0000
```

Transfer buckets (Stage X → Stage Y):
- **1→2** SICR (increase in credit risk)
- **2→1** Cure (recovery)
- **2→3** Credit default
- **1→3** Rare (direct default from Stage 1)
- **3→2** Management override downgrade reverse
- **3→1** Cure from Stage 3 (via override)

ECL movement per transfer bucket:
```
transfer_ecl_movement = ECL_current_stage_Y − ECL_prior_stage_X (for instrumen transitioning X→Y)
```

Remeasurement = residual after accounting for transfers, originations, derecognitions:
```
remeasurements = ECL_closing − ECL_opening − originations + derecognitions − transfer_ecl_movements
```

---

## Permissions Summary

| Permission | Actors | Stories |
|---|---|---|
| `ecl.roll_forward.read` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-ALCO | S1, S2, S3, S4 |
| `ecl.roll_forward.compute` | ROLE-RISK, ROLE-AKUN-CTL | S1 |
| `ecl.roll_forward.export` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | S4, S5 |
| `ecl.portfolio_aggregate.read` | ROLE-RISK, ROLE-CFO | S4, S6 |

---

## Story APP-C-M11-001 — Generate Roll-Forward Report

**Actor**: ROLE-RISK, ROLE-AKUN-CTL
**Trigger**: User navigasi ke `/ecl/roll-forward` dan memilih `currentCalcRunId` + `priorCalcRunId`, lalu klik "Hitung Roll-Forward"
**Goal**: Menghasilkan laporan roll-forward CKPN lengkap dengan semua komponen (opening, transfers, originations, derecognitions, remeasurements, closing) dan reconcile check terhadap total ECL current run. Menggantikan PARTIAL_PHASE_5_DEFER dari M7.

**Pre-conditions**:
- `ecl.calc_run` dengan `currentCalcRunId` berstatus `COMPLETED` atau `SEALED`.
- `ecl.calc_run` dengan `priorCalcRunId` berstatus `SEALED` dan `periode_id` = periode sebelum current.
- User memiliki permission `ecl.roll_forward.compute`.
- `ecl.calc_header` tersedia untuk kedua calc run.

**Post-conditions**:
- Objek roll-forward di-return (tidak disimpan ke tabel baru — computed query read-only dari `ecl.calc_header` + `ecl.stage_history`).
- Semua komponen bukan nullable (replace PARTIAL_PHASE_5_DEFER; origination/derecognition di-detect via basic status diff).
- `reconcile_status` = `RECONCILED` jika `|closing − Σecl_fl_idr| < IDR 1.0000`, `MISMATCH` jika tidak.
- Audit event `ECL.ROLL_FORWARD_COMPUTE` ditulis ke `aud.audit_log`.

**Data References**:
- Read: `ecl.calc_run`, `ecl.calc_header`, `ecl.calc_detail_skenario`, `ecl.stage_history`, `mst.instrumen`
- Write: `aud.audit_log` only

**Komponen**:
- Form: `currentCalcRunId` selector (dropdown — hanya COMPLETED/SEALED), `priorCalcRunId` selector (hanya SEALED, periode < current)
- Tombol "Hitung Roll-Forward" → submit → jika > 2 detik: Asynq job + `<JobProgressPanel>` (UX §3)
- Hasil: waterfall summary card + reconcile badge

**Audit Events**: `ECL.ROLL_FORWARD_COMPUTE`

### Acceptance Criteria — APP-C-M11-001

```gherkin
Feature: Generate roll-forward CKPN report

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "COMPLETED", periode = "JUNI-2026"
    And ecl.calc_run "CR-MEI-2026-001" status = "SEALED", periode = "MEI-2026"
    And ecl.calc_header total ECL current (JUNI): IDR 15.000.000.000,0000
    And ecl.calc_header total ECL prior (MEI): IDR 13.500.000.000,0000
    And RISK-01 memiliki permission ecl.roll_forward.compute + ecl.roll_forward.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: roll-forward berhasil dengan semua komponen
  # ---------------------------------------------------------------
  Scenario: RISK-01 generate roll-forward JUNI vs MEI — semua komponen non-null
    When RISK-01 memilih currentCalcRunId = "CR-JUNI-2026-001" dan priorCalcRunId = "CR-MEI-2026-001"
    And klik "Hitung Roll-Forward"
    Then POST /api/v1/ecl/roll-forward/compute dikirim dengan Idempotency-Key
    And response 200 (atau 202+jobId jika > 2 detik) berisi:
      | field                      | tipe          | not-null |
      | opening_ecl_idr            | NUMERIC(20,4) | yes      |
      | transfers_1_to_2_idr       | NUMERIC(20,4) | yes      |
      | transfers_2_to_1_idr       | NUMERIC(20,4) | yes      |
      | transfers_2_to_3_idr       | NUMERIC(20,4) | yes      |
      | transfers_1_to_3_idr       | NUMERIC(20,4) | yes      |
      | transfers_3_to_2_idr       | NUMERIC(20,4) | yes      |
      | transfers_3_to_1_idr       | NUMERIC(20,4) | yes      |
      | new_originations_idr       | NUMERIC(20,4) | yes      |
      | derecognitions_idr         | NUMERIC(20,4) | yes      |
      | remeasurements_idr         | NUMERIC(20,4) | yes      |
      | closing_ecl_idr            | NUMERIC(20,4) | yes      |
      | reconcile_status           | string        | yes      |
      | reconcile_delta_idr        | NUMERIC(20,4) | yes      |
      | detection_method           | string        | yes      |
    And opening_ecl_idr = 13.500.000.000,0000 (dari CR-MEI-2026-001)
    And closing_ecl_idr = 15.000.000.000,0000 (match CR-JUNI-2026-001 total)
    And reconcile check: |closing_ecl_idr − Σecl.calc_header.ecl_fl_idr for CR-JUNI| < IDR 1.0000
    And reconcile_status = "RECONCILED"
    And detection_method = "BASIC_STATUS_DIFF"
    And aud.audit_log berisi event "ECL.ROLL_FORWARD_COMPUTE" dengan actor = RISK-01

  # ---------------------------------------------------------------
  # Skenario 2 — Opening = 0: tidak ada prior calc run (periode pertama)
  # ---------------------------------------------------------------
  Scenario: Roll-forward periode pertama — tidak ada prior SEALED run — opening = 0
    Given tidak ada ecl.calc_run SEALED untuk periode sebelum JUNI-2026
    When RISK-01 memilih currentCalcRunId = "CR-JUNI-2026-001" dan priorCalcRunId = null
    And klik "Hitung Roll-Forward (Periode Pertama)"
    Then response berisi:
      | opening_ecl_idr      | 0.0000                                         |
      | new_originations_idr | = closing_ecl_idr (semua instrumen = origination baru) |
      | transfers_*_idr      | 0.0000 (semua transfer = 0, tidak ada prior stage) |
      | derecognitions_idr   | 0.0000                                         |
      | remeasurements_idr   | 0.0000                                         |
    And catatan tampil: "Periode pertama — tidak ada prior SEALED calc run. Opening ECL = 0."
    And reconcile_status = "RECONCILED"

  # ---------------------------------------------------------------
  # Skenario 3 — Reconcile MISMATCH: komponen tidak balance
  # ---------------------------------------------------------------
  Scenario: Reconcile MISMATCH — delta > IDR 1.0000 — error alert ditampilkan
    Given komponen roll-forward menghasilkan delta IDR 5.000,0000 (bug dalam logic deteksi)
    When roll-forward computed
    Then reconcile_status = "MISMATCH"
    And reconcile_delta_idr = 5000.0000
    And toast error persistent:
      "Roll-forward tidak reconcile. Delta = Rp 5.000,0000. Kemungkinan: instrumen baru tidak terdeteksi sebagai origination, atau ada stage_history missing. Lihat detail."
    And action link: "Lihat instrumen bermasalah →" (drilldown ke daftar instrumen dengan discrepancy)
    And aud.audit_log event "ECL.ROLL_FORWARD_MISMATCH" tercatat dengan delta amount

  # ---------------------------------------------------------------
  # Skenario 4 — currentCalcRunId berstatus DRAFT — validasi gagal
  # ---------------------------------------------------------------
  Scenario: currentCalcRunId = DRAFT — ditolak dengan error
    Given ecl.calc_run "CR-JULI-2026-DRAFT" status = "DRAFT"
    When RISK-01 mencoba memilih "CR-JULI-2026-DRAFT" sebagai currentCalcRunId
    Then dropdown hanya menampilkan run dengan status COMPLETED atau SEALED
    And "CR-JULI-2026-DRAFT" TIDAK ada di dropdown
    And jika dipaksakan via API: response 422, error.code = "ROLL_FORWARD_INVALID_CALC_RUN_STATUS"
    And error.message = "currentCalcRunId harus berstatus COMPLETED atau SEALED"

  # ---------------------------------------------------------------
  # Skenario 5 — priorCalcRunId bukan dari periode sebelumnya — validasi gagal
  # ---------------------------------------------------------------
  Scenario: priorCalcRunId dari periode yang sama atau lebih baru — ditolak
    Given CR-JUNI-2026-001 (JUNI) dan CR-JUNI-2026-002 (JUNI, SEALED)
    When RISK-01 mencoba priorCalcRunId = "CR-JUNI-2026-002" (periode sama)
    Then response 422, error.code = "ROLL_FORWARD_INVALID_PRIOR_PERIOD"
    And error.message = "priorCalcRunId harus dari periode sebelum periode current (JUNI-2026)"

  # ---------------------------------------------------------------
  # Skenario 6 — Permission: ROLE-AKUN tidak punya roll_forward.compute
  # ---------------------------------------------------------------
  Scenario: ROLE-AKUN mencoba compute — FORBIDDEN
    Given AKUN-01 hanya memiliki role ROLE-AKUN (tidak punya ecl.roll_forward.compute)
    When AKUN-01 POST /api/v1/ecl/roll-forward/compute
    Then response 403, error.code = "FORBIDDEN"
    And error.message = "Permission ecl.roll_forward.compute diperlukan"
```

### Open Questions — M11-001
- **OQ-M11-001-A**: Apakah hasil roll-forward disimpan ke tabel cache (`ecl.roll_forward_cache`) untuk akses cepat berikutnya, atau selalu dihitung ulang on-demand? Default assume: **no cache table** (computed read-only dari `ecl.calc_header` + `ecl.stage_history`). Jika runtime > 5 detik untuk 1000+ instrumen → Asynq job wajib (UX §3). Konfirmasi ke `system-analyst`.
- **OQ-M11-001-B**: Apakah `priorCalcRunId` boleh null (periode pertama)? Default assume: ya, null = periode pertama, opening = 0. Perlu UI handle null case dengan button text berbeda.
- **OQ-M11-001-C**: Reconcile tolerance IDR 1.0000 — dari plan §11 "Roll-forward reconcile: ECL_closing = opening + movements (delta < IDR 1)". Confirm tolerance ini untuk produksi vs UAT (UAT mungkin perlu lebih longgar)?

---

## Story APP-C-M11-002 — Stage Transfer Detection

**Actor**: ECL engine service (internal), dipanggil oleh roll-forward computation (M11-001)
**Trigger**: Roll-forward compute service iterasi per instrumen yang ada di kedua calc run (current + prior)
**Goal**: Mendeteksi semua stage transitions antar dua calc run, mengagregasi per bucket transfer (1→2, 2→1, 2→3, 1→3, 3→2, 3→1), dan menghitung ECL movement per bucket. Instrumen yang tidak berubah stage di-klasifikasikan sebagai potential remeasurement.

**Pre-conditions**:
- `ecl.calc_header` tersedia untuk instrumen di kedua calc run.
- `ecl.stage_history` tersedia untuk masing-masing instrumen.

**Post-conditions**:
- Transfer detection result tersedia sebagai sub-object `transfers` dalam roll-forward response.
- Setiap bucket berisi: `count_instrumen`, `ecl_movement_idr`.
- Instrumen dengan stage override vs auto-staging terbedakan (field `override_flag`).

**Data References**:
- Read: `ecl.calc_header` (prior + current), `ecl.stage_history` (untuk konfirmasi trigger type per instrumen)
- Write: none (pure query)

**Komponen**: Internal service `internal/ecl/rollforward/transfer_detector.go`

**Audit Events**: Tidak ada (internal computation step, dicakup oleh `ECL.ROLL_FORWARD_COMPUTE`)

### Acceptance Criteria — APP-C-M11-002

```gherkin
Feature: Stage transfer detection dalam roll-forward

  Background:
    Given ecl.calc_header untuk CR-MEI-2026-001 (prior) dan CR-JUNI-2026-001 (current):
      | instrumen_id | stage_prior | stage_current | ecl_fl_idr_prior | ecl_fl_idr_current |
      | INST-001     | 1           | 2             | 100.000.0000     | 350.000.0000       |  # SICR
      | INST-002     | 2           | 1             | 400.000.0000     | 120.000.0000       |  # Cure
      | INST-003     | 2           | 3             | 500.000.0000     | 1.200.000.0000     |  # Default
      | INST-004     | 1           | 3             | 80.000.0000      | 900.000.0000       |  # Rare direct
      | INST-005     | 3           | 2             | 1.000.000.0000   | 600.000.0000       |  # Override reverse
      | INST-006     | 1           | 1             | 200.000.0000     | 210.000.0000       |  # No transfer, ECL change
      | INST-007     | 2           | 2             | 300.000.0000     | 310.000.0000       |  # No transfer, ECL change

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: semua 6 bucket transfer terisi dengan benar
  # ---------------------------------------------------------------
  Scenario: Transfer detection menghasilkan 6 bucket benar
    When transfer_detector.Detect(priorRunID, currentRunID) dipanggil
    Then output transfers berisi:
      | bucket        | count | ecl_movement_idr                                       |
      | stage_1_to_2  | 1     | +250.000.0000 (350k − 100k, sign = increase)           |
      | stage_2_to_1  | 1     | −280.000.0000 (120k − 400k, sign = decrease/recovery)  |
      | stage_2_to_3  | 1     | +700.000.0000 (1.2M − 500k)                            |
      | stage_1_to_3  | 1     | +820.000.0000 (900k − 80k)                             |
      | stage_3_to_2  | 1     | −400.000.0000 (600k − 1M)                              |
      | stage_3_to_1  | 0     | 0.0000                                                 |
    And INST-006 dan INST-007 TIDAK masuk ke bucket transfer manapun
    And INST-006 dan INST-007 masuk ke kandidat remeasurement pool (stage_same)

  # ---------------------------------------------------------------
  # Skenario 2 — Stage X→X (stage tidak berubah): ECL change = remeasurement
  # ---------------------------------------------------------------
  Scenario: Instrumen Stage 1→1 dengan ECL berbeda — bukan transfer, tapi remeasurement
    Given INST-006: stage_prior = 1, stage_current = 1, ecl_prior = 200k, ecl_current = 210k
    When transfer_detector.Detect dipanggil
    Then INST-006 tidak ada di bucket transfer apapun
    And INST-006 ada di stage_same pool dengan ecl_movement = +10.000.0000
    And total stage_same_ecl_movement_idr = +10.000.0000 + +10.000.0000 = +20.000.0000 (termasuk INST-007)

  # ---------------------------------------------------------------
  # Skenario 3 — Instrumen dengan override stage: override_flag ditandai
  # ---------------------------------------------------------------
  Scenario: INST-005 (Stage 3→2) karena management override — override_flag = true
    Given ecl.stage_history untuk INST-005 di JUNI-2026:
      | transition   | stage_3 → stage_2              |
      | trigger_type | "MANAGEMENT_OVERRIDE"          |
    When transfer_detector mengklasifikasikan INST-005
    Then bucket stage_3_to_2 berisi INST-005 dengan override_flag = true
    And di summary output:
      | transfers.stage_3_to_2.count_override | 1 |
    And tooltip atau catatan UI: "1 instrumen di bucket ini karena management override — perlu justifikasi dokumen"

  # ---------------------------------------------------------------
  # Skenario 4 — Stage history missing: deteksi fallback ke ecl.calc_header
  # ---------------------------------------------------------------
  Scenario: ecl.stage_history tidak ada untuk instrumen — fallback ke stage di calc_header
    Given INST-008 ada di ecl.calc_header prior (stage=1) dan current (stage=2)
    But ecl.stage_history tidak ada entry untuk INST-008 transisi ke Stage 2
    When transfer_detector mencoba lookup stage_history
    Then INST-008 tetap di-klasifikasikan ke bucket stage_1_to_2 (dari calc_header stage fields)
    And warning ditambahkan: "INST-008: stage transition dari calc_header, tidak ada stage_history entry — verifikasi manual"
    And field `data_quality_warnings[]` dalam response berisi warning ini

  # ---------------------------------------------------------------
  # Skenario 5 — Semua instrumen Stage 1→1 (tidak ada transfer): semua bucket = 0
  # ---------------------------------------------------------------
  Scenario: Tidak ada stage transition — seluruh portfolio stabil Stage 1
    Given semua 500 instrumen stage_prior = 1 dan stage_current = 1
    When transfer_detector.Detect dipanggil
    Then semua 6 transfer bucket = count 0, ecl_movement = 0.0000
    And stage_same pool = 500 instrumen
    And remeasurements_idr = Σ(ecl_current − ecl_prior) untuk semua instrumen
```

### Open Questions — M11-002
- **OQ-M11-002-A**: ECL movement sign convention untuk transfer: apakah `stage_2_to_1` (cure/recovery) dilaporkan sebagai nilai negatif (decrease in ECL) atau selalu positif dengan separate sign column? Default assume: negatif jika ECL berkurang (nilai bisa negatif untuk bucket cure/recovery). Konfirmasi ke `ifrs9-compliance-reviewer` sesuai PSAK 71 §5.5 presentation.
- **OQ-M11-002-B**: Stage 3→1 (langsung cure dari Stage 3 ke Stage 1): PSAK 71 tidak explicit allow ini secara otomatis — apakah ini hanya via management override? Default assume: Stage 3→1 hanya via management override (bukan via cure otomatis yang 3 periode). Konfirmasi ke `ifrs9-compliance-reviewer`.

---

## Story APP-C-M11-003 — Origination + Derecognition Detection

**Actor**: ECL engine service (internal), dipanggil oleh roll-forward computation
**Trigger**: Roll-forward compute service untuk instrumen yang hadir hanya di satu dari dua calc run
**Goal**: Mendeteksi instrumen baru (origination = hadir di current tapi tidak di prior) dan instrumen yang keluar (derecognition = hadir di prior tapi tidak di current). Menghitung ECL contribution per kategori. Keterbatasan Phase 4: menggunakan basic status diff (`mst.instrumen.status`) — bukan APP-B transaction lifecycle events (deferred ke Phase 5).

**Pre-conditions**:
- `ecl.calc_header` tersedia untuk kedua run.
- `mst.instrumen.status` tersedia sebagai fallback untuk derecognition type detection.

**Post-conditions**:
- `new_originations`: list instrumen_id + ECL current; total `new_originations_idr`.
- `derecognitions`: list instrumen_id + ECL prior + reason (MATURED, SOLD, OTHER); total `derecognitions_idr`.
- `detection_method = "BASIC_STATUS_DIFF"` di-expose dalam response (transparency).

**Data References**:
- Read: `ecl.calc_header` (both runs), `mst.instrumen` (status, tanggal_jatuh_tempo)
- Write: none

**Komponen**: Internal service `internal/ecl/rollforward/lifecycle_detector.go`

**Audit Events**: Tidak ada (internal computation step)

### Acceptance Criteria — APP-C-M11-003

```gherkin
Feature: Origination dan derecognition detection dalam roll-forward

  Background:
    Given ecl.calc_header untuk CR-MEI-2026-001 (prior) dan CR-JUNI-2026-001 (current):
      | instrumen_id  | di prior | di current | mst.instrumen.status | keterangan         |
      | INST-NEW-001  | no       | yes        | AKTIF                | instrumen baru     |
      | INST-NEW-002  | no       | yes        | AKTIF                | instrumen baru     |
      | INST-MAT-001  | yes      | no         | JATUH_TEMPO          | matured            |
      | INST-SOLD-001 | yes      | no         | DIJUAL               | dijual             |
      | INST-UNK-001  | yes      | no         | AKTIF                | masih aktif tapi hilang dari run |
    And INST-NEW-001 ecl_current = 500.000.0000; INST-NEW-002 ecl_current = 300.000.0000
    And INST-MAT-001 ecl_prior = 200.000.0000; INST-SOLD-001 ecl_prior = 150.000.0000; INST-UNK-001 ecl_prior = 100.000.0000

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: origination dan derecognition terdeteksi
  # ---------------------------------------------------------------
  Scenario: Lifecycle detector menghasilkan origination + derecognition list
    When lifecycle_detector.Detect(priorRunID, currentRunID) dipanggil
    Then new_originations berisi:
      | instrumen_id | ecl_current_idr |
      | INST-NEW-001 | 500.000.0000    |
      | INST-NEW-002 | 300.000.0000    |
    And new_originations_idr = 800.000.0000
    And derecognitions berisi:
      | instrumen_id  | ecl_prior_idr  | derecognition_reason |
      | INST-MAT-001  | 200.000.0000   | MATURED              |
      | INST-SOLD-001 | 150.000.0000   | SOLD                 |
      | INST-UNK-001  | 100.000.0000   | UNKNOWN              |
    And derecognitions_idr = 450.000.0000
    And detection_method = "BASIC_STATUS_DIFF"

  # ---------------------------------------------------------------
  # Skenario 2 — Instrumen AKTIF tapi tidak ada di current run: UNKNOWN reason
  # ---------------------------------------------------------------
  Scenario: INST-UNK-001 masih AKTIF tapi tidak muncul di current run — reason = UNKNOWN + warning
    Given INST-UNK-001 mst.instrumen.status = "AKTIF"
    When lifecycle_detector mengklasifikasikan INST-UNK-001
    Then derecognition_reason = "UNKNOWN"
    And data_quality_warnings[] berisi:
      "INST-UNK-001: instrumen masih berstatus AKTIF tetapi tidak ada di current calc run CR-JUNI-2026-001. Kemungkinan di-exclude dari scope run atau ada data issue. Verifikasi dengan ROLE-RISK."
    And INST-UNK-001 tetap dihitung sebagai derecognition (ECL prior di-reverse)

  # ---------------------------------------------------------------
  # Skenario 3 — Instrumen JATUH_TEMPO terdeteksi sebagai MATURED derecognition
  # ---------------------------------------------------------------
  Scenario: INST-MAT-001 jatuh tempo dalam periode JUNI — derecognition reason = MATURED
    Given INST-MAT-001 mst.instrumen.tanggal_jatuh_tempo = "2026-06-15"
    And INST-MAT-001 mst.instrumen.status = "JATUH_TEMPO"
    When lifecycle_detector mengklasifikasikan INST-MAT-001
    Then derecognition_reason = "MATURED"
    And tidak ada warning (reason jelas)

  # ---------------------------------------------------------------
  # Skenario 4 — Phase 5 limitation notice tampil di response
  # ---------------------------------------------------------------
  Scenario: Response roll-forward menyertakan Phase 5 limitation notice
    When roll-forward compute berhasil
    Then response berisi field:
      | detection_method        | "BASIC_STATUS_DIFF"                                                      |
      | phase_5_limitation_note | "Deteksi origination/derecognition menggunakan perubahan status instrumen dan kehadiran di calc_run result. Untuk deteksi berbasis transaction lifecycle events (penempatan, penjualan, jatuh tempo), update ke Phase 5 (APP-B integration)." |
    And note ini tampil juga di UI sebagai banner info amber di seksi Origination/Derecognition

  # ---------------------------------------------------------------
  # Skenario 5 — Tidak ada origination dan derecognition: kedua = 0
  # ---------------------------------------------------------------
  Scenario: Portfolio stabil — instrumen sama persis di kedua run
    Given instrumen_id set di prior = instrumen_id set di current (identik)
    When lifecycle_detector.Detect dipanggil
    Then new_originations_idr = 0.0000
    And derecognitions_idr = 0.0000
    And new_originations list = []
    And derecognitions list = []
```

### Open Questions — M11-003
- **OQ-M11-003-A**: Apakah instrumen yang di-EXCLUDE dari scope run (mis. hanya run subset portofolio) harus di-exclude dari derecognition detection? Default assume: scope roll-forward harus match scope kedua run (all-active). Jika run A adalah partial scope dan run B adalah all-active → roll-forward tidak valid, harus return error `ROLL_FORWARD_SCOPE_MISMATCH`. Konfirmasi ke `system-analyst`.
- **OQ-M11-003-B**: Apakah instrumen FVTPL yang di-reklasifikasi ke AC antara dua periode harus muncul sebagai origination (untuk pertama kalinya masuk ECL)? Default assume: ya, FVTPL→AC reklasifikasi = origination dalam konteks ECL roll-forward. Konfirmasi ke `ifrs9-compliance-reviewer`.

---

## Story APP-C-M11-004 — Roll-Forward UI Display (Waterfall + Per-Portfolio)

**Actor**: ROLE-CFO, ROLE-RISK, ROLE-AKUN-CTL (read + export)
**Trigger**: Navigasi ke `/ecl/roll-forward?currentCalcRunId=CR-JUNI-2026-001&priorCalcRunId=CR-MEI-2026-001`
**Goal**: Menampilkan waterfall roll-forward CKPN dalam format yang intuitif — opening → movements (per bucket) → closing — dengan reconcile badge, drill-down per movement, breakdown per portofolio, dan export CSV/XLSX.

**Pre-conditions**:
- Roll-forward sudah di-compute (via S1) atau di-compute on-page-load secara otomatis.
- User memiliki permission `ecl.roll_forward.read`.

**Post-conditions**: Read-only. Export trigger ditulis ke `aud.audit_log` (S5 menangani export event).

**Komponen**:
- Form selector: `<Select>` currentCalcRunId (COMPLETED/SEALED), `<Select>` priorCalcRunId (SEALED)
- Waterfall card utama: opening + sub-baris movements + closing
- Reconcile badge: `RECONCILED` (hijau) atau `MISMATCH` (merah + delta)
- Phase 5 limitation banner (amber) untuk seksi origination/derecognition
- Per-portfolio breakdown: `<DataTable>` (UX §1: sort + filter + cursor + export)
- Drill-down per movement: slide panel atau navigasi ke `/ecl/roll-forward/transfers?bucket=1_to_2&...`
- Export bar: CSV/XLSX (UX §1)

**Permissions**: `ecl.roll_forward.read`, `ecl.roll_forward.export`
**Audit Events**: `ECL.ROLL_FORWARD_VIEW`, `ECL.ROLL_FORWARD_EXPORT`

### Acceptance Criteria — APP-C-M11-004

```gherkin
Feature: Roll-forward CKPN UI waterfall display

  Background:
    Given roll-forward computed dengan hasil:
      | opening_ecl_idr          | 13.500.000.000,0000   |
      | transfers_1_to_2_idr     | +2.000.000.000,0000   |
      | transfers_2_to_1_idr     | −500.000.000,0000     |
      | transfers_2_to_3_idr     | +800.000.000,0000     |
      | transfers_1_to_3_idr     | +100.000.000,0000     |
      | transfers_3_to_2_idr     | −300.000.000,0000     |
      | transfers_3_to_1_idr     | 0.0000                |
      | new_originations_idr     | +1.200.000.000,0000   |
      | derecognitions_idr       | −800.000.000,0000     |
      | remeasurements_idr       | +500.000.000,0000     |
      | closing_ecl_idr          | 16.500.000.000,0000   |
      | reconcile_status         | RECONCILED            |
    And RISK-01 memiliki permission ecl.roll_forward.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: waterfall tampil lengkap dengan semua baris
  # ---------------------------------------------------------------
  Scenario: Halaman roll-forward tampil dengan waterfall card benar
    When RISK-01 navigasi ke /ecl/roll-forward?currentCalcRunId=CR-JUNI-2026-001&priorCalcRunId=CR-MEI-2026-001
    Then waterfall card menampilkan (top to bottom):
      | Baris                          | Nilai IDR              | Warna      |
      | Opening CKPN (MEI-2026)        | 13.500.000.000,0000    | abu-abu    |
      | + Transfer Stage 1 → Stage 2   | +2.000.000.000,0000    | amber      |
      | − Transfer Stage 2 → Stage 1   | −500.000.000,0000      | hijau muda |
      | + Transfer Stage 2 → Stage 3   | +800.000.000,0000      | merah muda |
      | + Transfer Stage 1 → Stage 3   | +100.000.000,0000      | merah muda |
      | − Transfer Stage 3 → Stage 2   | −300.000.000,0000      | hijau muda |
      | Transfer Stage 3 → Stage 1     | 0.0000                 | abu-abu    |
      | + Originations Baru            | +1.200.000.000,0000    | biru       |
      | − Derecognitions               | −800.000.000,0000      | merah muda |
      | ± Remeasurements               | +500.000.000,0000      | abu-abu    |
      | = Closing CKPN (JUNI-2026)     | 16.500.000.000,0000    | bold hitam |
    And reconcile badge hijau: "RECONCILED — delta Rp 0,0000"
    And setiap baris movement memiliki ikon chevron → drill-down ke instrumen list

  # ---------------------------------------------------------------
  # Skenario 2 — Reconcile MISMATCH: badge merah + delta tampil
  # ---------------------------------------------------------------
  Scenario: Reconcile MISMATCH ditampilkan dengan jelas
    Given reconcile_status = "MISMATCH", reconcile_delta_idr = 3.500.0000
    When halaman di-load
    Then reconcile badge merah tampil: "MISMATCH — Delta Rp 3.500,0000"
    And alert banner merah di bawah waterfall:
      "Perhatian: Roll-forward tidak reconcile dengan total ECL calc run. Delta = Rp 3.500,0000. Mohon verifikasi instrumen dengan data_quality_warnings sebelum mempublikasikan laporan."
    And tombol "Lihat Instrumen Bermasalah" tersedia (link ke data_quality_warnings list)

  # ---------------------------------------------------------------
  # Skenario 3 — Drill-down baris transfer: instrumen list slide panel
  # ---------------------------------------------------------------
  Scenario: Klik chevron baris "Transfer Stage 1 → Stage 2" — slide panel tampil
    When RISK-01 klik ikon chevron di baris "Transfer Stage 1 → Stage 2"
    Then slide panel terbuka dari kanan dengan judul "Transfer Stage 1 → Stage 2 — 45 instrumen"
    And DataTable menampilkan kolom:
      | Kode Instrumen | Nama | Portofolio | ECL Prior (Stage 1) | ECL Current (Stage 2) | ECL Movement | Override |
    And DataTable mendukung sort, filter per kolom, cursor pagination
    And tombol Export CSV/XLSX tersedia di panel header

  # ---------------------------------------------------------------
  # Skenario 4 — Per-portfolio breakdown: DataTable dengan kolom per komponen
  # ---------------------------------------------------------------
  Scenario: Tab "Per Portofolio" menampilkan breakdown DataTable
    When RISK-01 klik tab "Per Portofolio"
    Then DataTable menampilkan satu baris per portofolio:
      | Portofolio     | Opening | Transfers Net | Originations | Derecognitions | Remeasurements | Closing |
      | PORT-OBLIGASI  | ...     | ...           | ...          | ...            | ...            | ...     |
      | PORT-DEPOSITO  | ...     | ...           | ...          | ...            | ...            | ...     |
    And baris total (footer): Σ semua portofolio = angka waterfall utama
    And sort by Closing desc (default)
    And kolom IDR diformat 4 desimal
    And export CSV/XLSX menghormati filter aktif

  # ---------------------------------------------------------------
  # Skenario 5 — Phase 5 limitation banner tampil di seksi origination
  # ---------------------------------------------------------------
  Scenario: Banner Phase 5 limitation tampil di baris origination/derecognition
    When halaman roll-forward di-load
    Then banner info amber di samping baris "Originations Baru" dan "Derecognitions":
      "Deteksi basic (status instrumen). Untuk akurasi penuh, upgrade ke Phase 5 (APP-B transaction lifecycle)."
    And ikon (i) klik → modal penjelasan:
      "detection_method: BASIC_STATUS_DIFF. Origination = instrumen hadir di current tapi tidak di prior run. Derecognition = instrumen hadir di prior tapi tidak di current run. Metode ini tidak menangkap transaksi intra-periode."

  # ---------------------------------------------------------------
  # Skenario 6 — URL state: deep-link parameter tersimpan di URL
  # ---------------------------------------------------------------
  Scenario: State currentCalcRunId + priorCalcRunId tersimpan di URL — deep-link valid
    When RISK-01 membuka halaman dengan param:
      /ecl/roll-forward?currentCalcRunId=CR-JUNI-2026-001&priorCalcRunId=CR-MEI-2026-001
    Then form selector otomatis ter-populate dengan kedua run
    And roll-forward di-compute (atau dibaca cache)
    And user dapat bookmark atau share URL ini
    When RISK-01 share URL ke CFO-01
    And CFO-01 membuka URL (dengan permission ecl.roll_forward.read)
    Then CFO-01 melihat data yang sama (no user-specific state)

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: read-only, export tersedia
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT membuka halaman roll-forward — semua data tampil, tidak ada compute
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 navigasi ke /ecl/roll-forward dengan params yang sama
    Then waterfall tampil lengkap (read-only)
    And tombol "Hitung Roll-Forward" TIDAK tampil (tidak punya ecl.roll_forward.compute)
    And tombol Export CSV/XLSX TERSEDIA (AUDIT punya ecl.roll_forward.export)
    And slide panel drill-down dapat dibuka (read-only)
```

### Open Questions — M11-004
- **OQ-M11-004-A**: Apakah halaman `/ecl/roll-forward` perlu breadcrumb link ke calc run detail (`/ecl/calc-runs/CR-JUNI-2026-001`)? Default assume: ya, breadcrumb "Calc Runs / CR-JUNI-2026-001 / Roll-Forward". Konfirmasi ke `uiux-designer`.
- **OQ-M11-004-B**: Transfer `stage_3_to_1` tampil sebagai baris terpisah atau digabung dengan `stage_3_to_2`? Default assume: baris terpisah (keduanya tampil, nilai bisa 0). Konfirmasi ke `ifrs9-compliance-reviewer` untuk PSAK 71 §5.5 disclosure format.

---

## Story APP-C-M11-005 — Disclosure-Ready Export (PSAK 71 §5.5)

**Actor**: ROLE-AKUN-CTL, ROLE-CFO
**Trigger**: Klik tombol "Export Disclosure XLSX" di halaman roll-forward atau dari halaman `/ecl/roll-forward/export`
**Goal**: Menghasilkan file XLSX berformat rapi sesuai PSAK 71 §5.5 disclosure requirements: movement table per stage (opening/closing balance + gross carrying amount), reconciliation section, dan sign-off section. File ini siap dilampirkan ke laporan keuangan formal.

**Pre-conditions**:
- Roll-forward sudah computed dan `reconcile_status = "RECONCILED"`. Jika MISMATCH, export di-block dengan warning.
- User memiliki permission `ecl.roll_forward.export`.

**Post-conditions**:
- File XLSX ter-download (dataset < 10k row → inline; ≥ 10k row → Asynq async export ke MinIO + download link).
- Audit event `ECL.ROLL_FORWARD_DISCLOSURE_EXPORT` ditulis ke `aud.audit_log`.

**Komponen**:
- Tombol "Export Disclosure XLSX" di action bar halaman roll-forward
- `<DestructiveActionDialog>` jika reconcile_status = MISMATCH (warn sebelum allow override export)
- Toast notifikasi sukses/gagal (UX §2)

**Permissions**: `ecl.roll_forward.export`
**Audit Events**: `ECL.ROLL_FORWARD_DISCLOSURE_EXPORT`

### Acceptance Criteria — APP-C-M11-005

```gherkin
Feature: Export disclosure XLSX PSAK 71 §5.5

  Background:
    Given roll-forward CR-JUNI-2026-001 vs CR-MEI-2026-001 sudah computed
    And reconcile_status = "RECONCILED"
    And AKUN-CTL-01 memiliki permission ecl.roll_forward.export

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: XLSX ter-download dengan format PSAK 71
  # ---------------------------------------------------------------
  Scenario: AKUN-CTL-01 export disclosure XLSX — format PSAK 71 §5.5
    When AKUN-CTL-01 klik "Export Disclosure XLSX"
    Then file "ckpn-roll-forward-JUNI-2026-20260613.xlsx" ter-download
    And XLSX berisi sheet "Movement Table" dengan:
      | Row                          | Stage 1 (IDR) | Stage 2 (IDR) | Stage 3 (IDR) | Total (IDR) |
      | Saldo Awal (Opening)         | ...            | ...            | ...            | ...         |
      | Transfer ke Stage 2 (+)      | ...            | N/A            | N/A            | ...         |
      | Transfer dari Stage 2 (−)    | ...            | N/A            | N/A            | ...         |
      | Transfer ke Stage 3 (+)      | ...            | ...            | N/A            | ...         |
      | Transfer dari Stage 3 (−)    | ...            | ...            | N/A            | ...         |
      | Origination Baru (+)         | ...            | ...            | ...            | ...         |
      | Penghentian Pengakuan (−)    | ...            | ...            | ...            | ...         |
      | Pengukuran Ulang (±)         | ...            | ...            | ...            | ...         |
      | Saldo Akhir (Closing)        | ...            | ...            | ...            | ...         |
    And header row di-bold, font size 11, freeze row pertama
    And kolom IDR diformat currency "Rp #.##0,0000"
    And baris "Saldo Akhir" di-bold dengan border bawah tebal (border style thick)
    And XLSX berisi sheet "Gross Carrying Amount" dengan:
      | Stage     | Gross Carrying (IDR) | ECL Allowance (IDR) | Net Carrying (IDR) |
      | Stage 1   | ...                  | ...                 | ...                |
      | Stage 2   | ...                  | ...                 | ...                |
      | Stage 3   | ...                  | ...                 | ...                |
    And XLSX berisi sheet "Sign-Off" dengan:
      | Prepared by    | [nama user AKUN-CTL-01]           |
      | Prepared date  | [tanggal export]                   |
      | Approved by    | [kosong — untuk tanda tangan CFO] |
      | Approved date  | [kosong]                           |
      | Periode        | JUNI-2026                         |
      | Calc Run ID    | CR-JUNI-2026-001                  |
      | Prior Run ID   | CR-MEI-2026-001                   |
      | Reconcile      | RECONCILED (Delta: Rp 0,0000)     |
      | Detection Method | BASIC_STATUS_DIFF (Phase 4)     |
      | Phase 5 Note   | [teks limitation note]             |
    And aud.audit_log berisi event "ECL.ROLL_FORWARD_DISCLOSURE_EXPORT" dengan:
      | actor_user_id   | AKUN-CTL-01 |
      | after_jsonb.format       | "xlsx"       |
      | after_jsonb.periode_id   | "JUNI-2026"  |
      | after_jsonb.calc_run_id  | "CR-JUNI-2026-001" |
      | after_jsonb.reconcile_status | "RECONCILED" |

  # ---------------------------------------------------------------
  # Skenario 2 — Export ketika MISMATCH: dialog warning muncul dulu
  # ---------------------------------------------------------------
  Scenario: Export saat reconcile_status = MISMATCH — dialog warning wajib
    Given reconcile_status = "MISMATCH", reconcile_delta_idr = 5000.0000
    When AKUN-CTL-01 klik "Export Disclosure XLSX"
    Then <DestructiveActionDialog> muncul:
      | Judul       | "Roll-Forward Tidak Reconcile — Tetap Export?"                                    |
      | Deskripsi   | "Delta = Rp 5.000,0000. Laporan yang di-export tidak akan reconcile. Ekspor ini hanya untuk keperluan analisis internal, BUKAN untuk disclosure resmi." |
      | Tombol      | "Export Saja (Analisis Internal)" (amber) | "Batal"                              |
    When AKUN-CTL-01 klik "Export Saja (Analisis Internal)"
    Then file ter-download dengan sheet "Sign-Off" mencantumkan:
      | Reconcile | MISMATCH (Delta: Rp 5.000,0000) — TIDAK UNTUK PUBLIKASI |
    And aud.audit_log berisi event "ECL.ROLL_FORWARD_DISCLOSURE_EXPORT_MISMATCH"

  # ---------------------------------------------------------------
  # Skenario 3 — Export dataset besar (≥ 10k instrumen): async Asynq job
  # ---------------------------------------------------------------
  Scenario: Portfolio > 10.000 instrumen — export async ke MinIO
    Given roll-forward untuk 15.000 instrumen (COMPLETED)
    When AKUN-CTL-01 klik "Export Disclosure XLSX"
    Then POST ke export endpoint → 202 Accepted, jobId dikembalikan
    And <JobProgressPanel> tampil (UX §3): "Generating XLSX disclosure report..."
    When job selesai
    Then toast sukses: "Export selesai. File 'ckpn-roll-forward-JUNI-2026-20260613.xlsx' siap diunduh."
    And action link: "Unduh File →" (signed MinIO URL, TTL 24 jam)

  # ---------------------------------------------------------------
  # Skenario 4 — Permission: ROLE-RISK tidak punya export — tombol tersembunyi
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK tanpa ecl.roll_forward.export — tombol export tidak tampil
    Given RISK-02 memiliki permission ecl.roll_forward.read TAPI bukan ecl.roll_forward.export
    When RISK-02 membuka halaman roll-forward
    Then tombol "Export Disclosure XLSX" TIDAK tampil
    And jika dipaksakan via API: 403, error.code = "FORBIDDEN"

  # ---------------------------------------------------------------
  # Skenario 5 — CSV export: format sederhana untuk analisis
  # ---------------------------------------------------------------
  Scenario: Export CSV — format flat untuk analisis data
    When AKUN-CTL-01 klik "Export CSV"
    Then file "ckpn-roll-forward-JUNI-2026-20260613.csv" ter-download
    And CSV mengandung header: Komponen,Stage_1_IDR,Stage_2_IDR,Stage_3_IDR,Total_IDR
    And CSV UTF-8 with BOM (Excel-compatible)
    And baris footer: "Diekspor pada: 2026-06-13 09:00 WIB | Calc Run: CR-JUNI-2026-001 | Reconcile: RECONCILED"
```

### Open Questions — M11-005
- **OQ-M11-005-A**: Sheet "Gross Carrying Amount" di XLSX — memerlukan kolom `gross_carrying_idr` per instrumen per stage yang di-aggregate. Apakah `ecl.calc_header` memiliki kolom `gross_carrying_idr` atau hanya `ead_idr`? Default assume: `ead_idr` ≈ gross carrying untuk instruments BLIPS Phase 4 (Phase 5 akan pisah EAD dari gross carrying untuk kredit dengan komitmen). Konfirmasi ke `ifrs9-compliance-reviewer`.
- **OQ-M11-005-B**: Sign-off section — apakah digital signature (step-up MFA CFO) diperlukan untuk export disclosure ini, atau cukup printed signature? Default assume: Phase 4 = cetak + tanda tangan manual. Digital signature dengan MFA = Phase 5 workflow. Flag ke `ifrs9-compliance-reviewer` untuk PSAK 71 compliance review.

---

## Story APP-C-M11-006 — CKPN Trend Dashboard (Periodic)

**Actor**: ROLE-CFO, ROLE-RISK (read)
**Trigger**: Navigasi ke `/dashboard/ckpn-trend`
**Goal**: Menampilkan tren ECL CKPN multi-periode dari semua SEALED calc run (hingga 12 periode terakhir) sebagai line chart + bar chart via Recharts, dengan kemampuan drill-down ke roll-forward spesifik antara periode yang dipilih.

**Pre-conditions**:
- Tersedia minimal 2 SEALED calc run dari 2 periode berbeda.
- User memiliki permission `ecl.roll_forward.read`.

**Post-conditions**: Read-only dashboard. Tidak ada mutasi.

**Komponen**:
- `<LineChart>` Recharts: total ECL per periode (x-axis), ECL amount (y-axis)
- `<BarChart>` Recharts: ECL per stage per periode (stacked bar: Stage 1/2/3)
- Period range selector: last 6 / last 12 / custom
- DataTable: tabel data underlying (UX §1: sort + filter + export)
- Drill-down: klik titik periode → navigasi ke `/ecl/roll-forward?currentCalcRunId=...&priorCalcRunId=...`

**Permissions**: `ecl.roll_forward.read`, `ecl.portfolio_aggregate.read`
**Audit Events**: Tidak ada (read-only dashboard)

### Acceptance Criteria — APP-C-M11-006

```gherkin
Feature: CKPN trend dashboard multi-periode

  Background:
    Given 8 SEALED calc run dari periode NOVEMBER-2025 s.d. JUNI-2026:
      | Periode       | ECL Total (IDR)        | Stage 1    | Stage 2    | Stage 3    |
      | NOV-2025      | 10.500.000.000,0000    | 8.000M     | 2.000M     | 500M       |
      | DES-2025      | 11.200.000.000,0000    | 8.500M     | 2.200M     | 500M       |
      | JAN-2026      | 11.800.000.000,0000    | 8.800M     | 2.500M     | 500M       |
      | FEB-2026      | 12.300.000.000,0000    | 9.000M     | 2.700M     | 600M       |
      | MAR-2026      | 13.000.000.000,0000    | 9.500M     | 2.800M     | 700M       |
      | APR-2026      | 13.500.000.000,0000    | 9.800M     | 3.000M     | 700M       |
      | MEI-2026      | 14.200.000.000,0000    | 10.000M    | 3.400M     | 800M       |
      | JUNI-2026     | 15.000.000.000,0000    | 10.500M    | 3.600M     | 900M       |
    And CFO-01 memiliki permission ecl.roll_forward.read + ecl.portfolio_aggregate.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: chart tampil dengan data 8 periode
  # ---------------------------------------------------------------
  Scenario: Dashboard trend tampil dengan line chart dan stacked bar chart
    When CFO-01 navigasi ke /dashboard/ckpn-trend (default: last 6 periode)
    Then <LineChart> menampilkan 6 titik: JAN-2026 s.d. JUNI-2026
    And y-axis diformat miliar IDR (mis. "Rp 15,0 M")
    And tooltip hover per titik: "JUNI-2026: Rp 15.000.000.000,0000 | +5.63% vs MEI-2026"
    And <BarChart> (stacked) menampilkan Stage 1 (hijau), Stage 2 (amber), Stage 3 (merah) per periode
    And legend chart tampil dengan label + warna

  # ---------------------------------------------------------------
  # Skenario 2 — Period range picker: last 12 atau custom
  # ---------------------------------------------------------------
  Scenario: CFO-01 ganti range ke "Last 12" — chart refresh dengan 8 data point (hanya 8 tersedia)
    When CFO-01 pilih range "Last 12 Periode"
    Then chart menampilkan 8 titik (semua yang tersedia: NOV-2025 s.d. JUNI-2026)
    And info text: "Menampilkan 8 periode (data tersedia dari NOV-2025)"

  # ---------------------------------------------------------------
  # Skenario 3 — Drill-down: klik titik JUNI-2026 → navigasi ke roll-forward
  # ---------------------------------------------------------------
  Scenario: CFO-01 klik titik JUNI-2026 di chart — navigasi ke roll-forward JUNI vs MEI
    When CFO-01 klik titik "JUNI-2026" di <LineChart>
    Then tooltip tampil dengan tombol "Lihat Roll-Forward →"
    When CFO-01 klik "Lihat Roll-Forward →"
    Then navigasi ke /ecl/roll-forward?currentCalcRunId=CR-JUNI-2026-001&priorCalcRunId=CR-MEI-2026-001
    And halaman roll-forward otomatis compute dengan kedua parameter tersebut

  # ---------------------------------------------------------------
  # Skenario 4 — DataTable underlying di bawah chart
  # ---------------------------------------------------------------
  Scenario: DataTable data underlying menampilkan semua periode dengan sort + export
    When CFO-01 scroll ke bawah dashboard
    Then DataTable tampil dengan kolom:
      | Periode | ECL Total (IDR) | Stage 1 (IDR) | Stage 2 (IDR) | Stage 3 (IDR) | Δ vs Sebelumnya | Δ% |
    And default sort: periode DESC
    And sort klik header: toggle asc/desc
    And export CSV/XLSX menghormati filter + sort aktif

  # ---------------------------------------------------------------
  # Skenario 5 — Hanya 1 SEALED run tersedia: chart tidak bisa tampil
  # ---------------------------------------------------------------
  Scenario: Hanya 1 SEALED calc run tersedia — chart tidak bisa ditampilkan
    Given hanya 1 SEALED calc run (MEI-2026)
    When CFO-01 navigasi ke /dashboard/ckpn-trend
    Then chart placeholder tampil: "Minimal 2 periode SEALED diperlukan untuk menampilkan tren ECL."
    And tombol "Buat Calc Run →" link ke /ecl/calc-runs/new
    And DataTable tampil dengan 1 baris (MEI-2026)
```

### Open Questions — M11-006
- **OQ-M11-006-A**: Dashboard `/dashboard/ckpn-trend` — apakah ini bagian dari APP-E Reporting (Phase 6) atau sudah bisa di-deliver sebagai MVP di Phase 4 (karena hanya read + query dari SEALED calc runs)? Default assume: **deliverable Phase 4** (hanya read dari `ecl.calc_run` + `ecl.calc_header`, tidak ada laporan formal baru). Konfirmasi ke `tech-lead-orchestrator`.
- **OQ-M11-006-B**: "Delta vs sebelumnya" di DataTable — apakah delta dihitung berdasarkan periode sebelumnya dalam urutan kronologis, atau dari sealed run dengan periode_id = current minus 1 bulan? Default assume: periode tepat sebelumnya dalam urutan `mst.periode_buku`. Konfirmasi ke `system-analyst`.

---

## Ringkasan Data References

| Story | Tabel Read | Tabel Write | Permission |
|---|---|---|---|
| M11-001 (generate) | `ecl.calc_run`, `ecl.calc_header`, `ecl.calc_detail_skenario`, `ecl.stage_history`, `mst.instrumen` | `aud.audit_log` | `ecl.roll_forward.compute` |
| M11-002 (stage transfer) | `ecl.calc_header` (both runs), `ecl.stage_history` | none (internal) | internal |
| M11-003 (origination/derecognition) | `ecl.calc_header` (both runs), `mst.instrumen` | none (internal) | internal |
| M11-004 (UI display) | computed from M11-001 | `aud.audit_log` (view event) | `ecl.roll_forward.read` |
| M11-005 (export) | computed from M11-001 | `aud.audit_log` (export event) | `ecl.roll_forward.export` |
| M11-006 (trend dashboard) | `ecl.calc_run` (SEALED only), `ecl.calc_header` aggregate | none | `ecl.roll_forward.read` |

---

## Ringkasan Audit Events

| Event | Kapan | Actor |
|---|---|---|
| `ECL.ROLL_FORWARD_COMPUTE` | Roll-forward berhasil di-compute | RISK, AKUN-CTL |
| `ECL.ROLL_FORWARD_MISMATCH` | Reconcile check gagal (delta > IDR 1) | System |
| `ECL.ROLL_FORWARD_VIEW` | User membuka halaman roll-forward | RISK, AKUN-CTL, CFO, AUDIT, ALCO |
| `ECL.ROLL_FORWARD_DISCLOSURE_EXPORT` | Export XLSX disclosure berhasil | AKUN-CTL, CFO, AUDIT |
| `ECL.ROLL_FORWARD_DISCLOSURE_EXPORT_MISMATCH` | Export saat MISMATCH (user override) | AKUN-CTL, CFO |

---

## Dependency Map

```
M11-001 (generate)
  ├── depends on M11-002 (stage transfer detection — internal)
  ├── depends on M11-003 (lifecycle detection — internal)
  └── supplies M11-004 (UI), M11-005 (export), M11-006 (chart data point)

M11-004 (UI)
  └── depends on M11-001 result

M11-005 (export)
  └── depends on M11-001 reconcile_status

M11-006 (trend dashboard)
  └── depends on ecl.calc_run SEALED data (M8) + M11-001 (drill-down target)
```

---

## Consolidated Open Questions

| ID | Pertanyaan | Default | Owner | Blocking? |
|---|---|---|---|---|
| OQ-M11-001-A | Cache vs on-demand computation untuk roll-forward | No cache, on-demand (Asynq jika > 2 detik) | `system-analyst` | Tidak |
| OQ-M11-001-B | priorCalcRunId = null untuk periode pertama — UI handling | Ya, null valid, opening = 0 | `system-analyst` | Tidak |
| OQ-M11-001-C | Reconcile tolerance produksi vs UAT | IDR 1.0000 produksi | `ifrs9-compliance-reviewer` | **Ya** |
| OQ-M11-002-A | Sign convention ECL movement per transfer bucket (negatif untuk cure?) | Negatif untuk decrease | `ifrs9-compliance-reviewer` | **Ya** |
| OQ-M11-002-B | Stage 3→1 hanya via management override? | Ya, tidak via auto-cure | `ifrs9-compliance-reviewer` | **Ya** |
| OQ-M11-003-A | Partial scope run → roll-forward harus return `SCOPE_MISMATCH`? | Ya | `system-analyst` | Tidak |
| OQ-M11-003-B | FVTPL→AC reklasifikasi = origination dalam ECL roll-forward? | Ya | `ifrs9-compliance-reviewer` | **Ya** |
| OQ-M11-004-A | Breadcrumb ke calc run detail dari roll-forward halaman | Ya | `uiux-designer` | Tidak |
| OQ-M11-004-B | Stage 3→1 baris terpisah atau digabung Stage 3→2? | Terpisah | `ifrs9-compliance-reviewer` | Tidak |
| OQ-M11-005-A | `gross_carrying_idr` tersedia di `ecl.calc_header` atau pakai `ead_idr`? | Pakai `ead_idr` (Phase 4) | `ifrs9-compliance-reviewer` | **Ya** |
| OQ-M11-005-B | Digital signature MFA CFO untuk disclosure export Phase 4? | Tidak (manual print sign) | `ifrs9-compliance-reviewer` | Tidak |
| OQ-M11-006-A | Dashboard trend deliverable Phase 4 atau deferred Phase 6? | Phase 4 MVP (read-only) | `tech-lead-orchestrator` | Tidak |
| OQ-M11-006-B | "Delta vs sebelumnya" dari periode tepat sebelumnya | Periode tepat sebelumnya di `mst.periode_buku` | `system-analyst` | Tidak |
