# P5-M10 — APP-C POCI Delta ECL: User Stories

**Story Set ID**: P5-M10
**Modul**: APP-C — ECL Engine (POCI Delta, Phase 5)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `ifrs9-compliance-reviewer` (BLOCKING)
**Author**: business-analyst
**Tanggal**: 2026-06-20
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §5 (POCI), §6 (ECL Delta Booking); FSD-BLIPS-MASTER-v1.1.docx §4 (klasifikasi AC POCI)
**Linked BRD**: BRD §6.5 (APP-C POCI Impairment), RACI: ROLE-RISK (A delta dashboard), ROLE-AKUN (R jurnal review), ROLE-CFO (A large delta), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-010` (LOCKED) — ECL 3-stage × 3-skenario; POCI skip staging engine (tidak ada Stage 1/2/3 biasa)
- `DEC-013` (LOCKED) — EIR Newton-Raphson; credit-adjusted EIR untuk POCI; **NEVER UPDATE** existing schedule rows
- `DEC-016` (LOCKED) — `shopspring/decimal`; `NUMERIC(20,4)` IDR; `NUMERIC(10,8)` PD/LGD/EIR; never float64
- `DEC-017` (LOCKED) — 4-eyes workflow; SoD `maker ≠ reviewer ≠ approver`
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun; `ecl.poci_baseline` immutable

**Compliance reference**: PSAK 71 §5.5.13 (POCI recognition), §5.5.14 (ECL movement P&L booking — delta basis only)

**Dependensi**:
- **Phase 4 ECL core** — `ecl.poci_baseline` row harus sudah ada sejak origination (M10 membaca, tidak overwrite). ECL calc run Phase 4 trigger M10 pada POCI instrumen.
- **P5-M1** — `mst.instrumen.is_poci = TRUE` + `klasifikasi_psak71` locked pada penempatan POCI; capture baseline di event approve penempatan
- **P5-M2** — jurnal engine; event code `POCI_ECL_DELTA` harus tersedia di mapping master sebelum M10 posting pertama
- **P5-M9** — akrual POCI menggunakan `credit_adjusted_eir` dari `ecl.amortisasi_schedule` versi POCI; M10 adalah consumer downstream akrual POCI

**Handoff berikutnya**:
- `system-analyst` → OpenAPI: 5 endpoints (`POST /ecl/calc-runs` (POCI flow), `GET /ecl/calc-run/{id}/result-line?type=POCI`, `GET /poci/delta-history`, `GET /poci/baseline/{instrumen_id}`, `GET /jobs/{jobId}`); state machine `ecl.poci_delta_log.direction`; 6 error codes baru (§Error Codes Proposed); SSE stream job progress
- `data-modeler` → migration: `ecl.poci_baseline` (Phase 4 sudah punya — verify schema); `ecl.poci_delta_log` baru; unique constraint `(calc_run_id, instrumen_id)` di `poci_delta_log`; index `(instrumen_id, created_at DESC)` untuk delta history
- `ifrs9-compliance-reviewer` → **BLOCKING gate**: (a) POCI delta = current lifetime ECL − origination baseline per PSAK 71 §5.5.13-14; (b) NO Stage enforcement (POCI stage marker); (c) jurnal sign convention P&L vs OCI; (d) baseline immutability DEC-018
- `security-engineer` → audit in-transaction `POCI.BASELINE_CAPTURED`, `POCI.DELTA_COMPUTED`, `POCI.DELTA_POSTED`, `POCI.WARNING_REMOVED`; idempotency `(calc_run_id, instrumen_id)`; ROLE-CFO large delta review permission

**Compliance path**: P5-M10 menyentuh PSAK 71 §5.5.13-14 critical path (POCI impairment measurement) — **ifrs9-compliance-reviewer BLOCKING** wajib sebelum implementasi S2, S3. **security-engineer BLOCKING** untuk audit immutability S1 dan in-transaction S3.

---

## Konteks & Arsitektur P5-M10

### POCI Delta Flow

```
Penempatan POCI APPROVED (P5-M1)
    ↓
S1: Insert ecl.poci_baseline (immutable)
    { instrumen_id, lifetime_ecl_at_origination, cashflow_expectasi_jsonb, credit_adjusted_eir }
    Audit: POCI.BASELINE_CAPTURED in-transaction

ECL Calc Run (Phase 4 trigger, setiap periode)
    ↓
S2: Untuk setiap POCI instrumen dalam run:
    Skip staging engine → stage_marker = 'POCI'
    current_lifetime_ecl = hitung ECL(EAD, PD_lifetime, LGD) × FL × bobot
    delta_ecl = current_lifetime_ecl − baseline_lifetime_ecl
    direction = INCREASE (delta > 0) | DECREASE (delta < 0) | UNCHANGED
    Insert ecl.poci_delta_log { calc_run_id, instrumen_id, ... }
    Audit: POCI.DELTA_COMPUTED in-transaction

Jurnal posting (per delta, per run)
    ↓
S3: delta > 0 (deterioration):
      D Beban Penurunan Nilai ECL POCI / K Cadangan ECL POCI = delta_ecl
    delta < 0 (improvement):
      D Cadangan ECL POCI / K Pendapatan Pemulihan ECL POCI = |delta_ecl|
    Event code: POCI_ECL_DELTA (P5-M2 jurnal engine)
    Idempotency: (calc_run_id, instrumen_id)
    Audit: POCI.DELTA_POSTED in-transaction
```

### Perbedaan POCI vs non-POCI di ECL Calc Run

```
non-POCI:
  stage = 1 | 2 | 3
  ECL = EAD × PD_12M|Lifetime|1.0 × LGD × FL × bobot
  P&L: PERUBAHAN PENUH dari periode sebelumnya

POCI (§5.5.13-14):
  stage_marker = 'POCI' (bukan 1/2/3)
  ECL = lifetime ECL dari cashflow expectasi (PD-adjusted sejak origination)
  P&L: HANYA DELTA = current − baseline (bukan full lifetime ECL)
  Baseline IMMUTABLE (DEC-018)
```

### Presisi & Formula Delta

```
# Dalam calc run, per POCI instrumen
EAD_IDR          = saldo_pokok + akrual_piutang + (komitmen × CCF)
                   dikonversi via FX_rate_BI_JISDOR jika FCY

current_lifetime_ecl = Σ (ECL_FL_skenario × bobot_skenario)
                     = EAD_IDR × PD_Lifetime_skenario × LGD × FL_multiplier × bobot
                       (Good×0.25 + Normal×0.50 + Bad×0.25 per DEC-010)

delta_ecl        = current_lifetime_ecl − baseline_lifetime_ecl
                   (shopspring/decimal, HALF_EVEN, NUMERIC(20,4))

direction        = INCREASE  jika delta_ecl > 0
                 = DECREASE  jika delta_ecl < 0
                 = UNCHANGED jika delta_ecl == 0 (exact decimal)

prior_delta_cumulative = Σ delta_ecl semua run sebelumnya untuk instrumen ini
```

---

## Story P5-M10-S1 — Capture POCI Baseline saat Penempatan POCI Di-approve

**Actor**: System (triggered by P5-M1 penempatan approve event), ROLE-AUDIT (read)
**Trigger**: `POST /api/v1/transaksi/penempatan/{id}/approve` selesai dengan `mst.instrumen.is_poci = TRUE`. Dalam transaksi yang sama, sebelum commit, insert `ecl.poci_baseline`. Jika baseline sudah ada untuk instrumen ini → `POCI_BASELINE_IMMUTABLE_VIOLATION` (refuse; tidak overwrite). Audit `POCI.BASELINE_CAPTURED` in-transaction.
**Goal**: Setiap instrumen POCI memiliki satu immutable baseline row di `ecl.poci_baseline` sejak origination. Baseline ini digunakan selamanya sebagai denominator delta computation.

### Pre-conditions
1. Penempatan POCI: `mst.instrumen.is_poci = TRUE` dan `klasifikasi_psak71` locked (AC atau FVOCI debt)
2. Credit-adjusted EIR sudah dihitung oleh ECL engine (P5-M1 flow, Newton-Raphson DEC-013)
3. `ecl.poci_baseline` belum ada untuk `instrumen_id` ini — insert only
4. `mst.periode_buku.status_periode = 'OPEN'`

### Acceptance Criteria

```gherkin
Feature: Capture POCI ECL baseline saat penempatan POCI di-approve

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And ROLE-APPR-TR (USR-APPR-001) meng-approve penempatan POCI-DEP-0001
    And mst.instrumen POCI-DEP-0001: is_poci = TRUE, klasifikasi_psak71 = 'AC'

  Scenario: S1-AC1 — Insert poci_baseline in-transaction saat approve penempatan POCI
    Given ecl.poci_baseline belum ada untuk POCI-DEP-0001
    And credit_adjusted_eir = 0.04500000 (dihitung ECL engine, PD-adjusted cashflow)
    And lifetime_ecl_at_origination = 1250000000.0000 (shopspring/decimal, NUMERIC(20,4))
    When USR-APPR-001 meng-approve POST /api/v1/transaksi/penempatan/POCI-DEP-0001/approve
      With Idempotency-Key: IK-APPR-POCI-001
    Then dalam transaksi yang sama (sebelum commit):
      ecl.poci_baseline INSERT:
        | instrumen_id                | POCI-DEP-0001      |
        | lifetime_ecl_at_origination | 1250000000.0000    |
        | cashflow_expectasi_jsonb    | { PD-adjusted CFs dari origination assessment } |
        | credit_adjusted_eir         | 0.04500000         |
        | origination_date            | 2026-06-20         |
        | created_by                  | USR-APPR-001       |
    And aud.audit_log.action = POCI.BASELINE_CAPTURED — in-transaction
      With after_jsonb: { instrumen_id, lifetime_ecl_at_origination: 1250000000.0000, credit_adjusted_eir: 0.04500000 }
    And HTTP 200 approve berhasil
    And toast ke USR-APPR-001: "Penempatan POCI-DEP-0001 berhasil di-approve. ECL baseline IDR 1.250.000.000 dicatat. Immutable."

  Scenario: S1-AC2 — Baseline sudah ada: refuse overwrite, return POCI_BASELINE_IMMUTABLE_VIOLATION
    Given ecl.poci_baseline sudah ada untuk POCI-DEP-0001 (origination_date = 2026-03-15)
    When sistem mencoba insert baseline kedua untuk POCI-DEP-0001 (mis. via retry atau bug)
    Then HTTP 422:
      | error.code    | POCI_BASELINE_IMMUTABLE_VIOLATION                                  |
      | error.message | "POCI baseline untuk POCI-DEP-0001 sudah ada (DEC-018). Tidak dapat di-overwrite." |
    And ecl.poci_baseline: row origination tetap tidak berubah (WORM — append-only)
    And aud.audit_log.action = POCI.BASELINE_VIOLATION_ATTEMPT — in-transaction dengan detail upaya overwrite

  Scenario: S1-AC3 — Instrumen bukan POCI: baseline tidak di-insert, proses approve lanjut normal
    Given mst.instrumen DEP-0099: is_poci = FALSE, klasifikasi_psak71 = 'AC'
    When USR-APPR-001 meng-approve penempatan DEP-0099
    Then ecl.poci_baseline: tidak ada INSERT untuk DEP-0099
    And approve sukses normal tanpa error POCI
    And aud.audit_log tidak mencatat POCI.BASELINE_CAPTURED untuk DEP-0099

  Scenario: S1-AC4 — POCI baseline read-only oleh ROLE-AUDIT; ROLE-AKUN tidak bisa mutate
    Given ecl.poci_baseline row untuk POCI-DEP-0001 sudah ada
    When ROLE-AUDIT (USR-AUDIT-001) mengirim GET /api/v1/poci/baseline/POCI-DEP-0001
    Then HTTP 200: row baseline lengkap (instrumen_id, lifetime_ecl_at_origination, credit_adjusted_eir, origination_date)
    And jika USR-AKUN-001 (ROLE-AKUN) mencoba PATCH /api/v1/poci/baseline/POCI-DEP-0001
    Then HTTP 403: { error.code: "FORBIDDEN", error.message: "ecl_poci_baseline.update tidak diizinkan. Baseline immutable (DEC-018)." }
```

---

## Story P5-M10-S2 — Compute POCI Delta pada ECL Calc Run

**Actor**: System (ECL calc run engine, Phase 4 trigger), ROLE-RISK (review hasil delta), ROLE-AUDIT (read)
**Trigger**: ECL calc run berjalan (Phase 4 `POST /api/v1/ecl/calc-runs`). Untuk setiap instrumen dengan `is_poci = TRUE` dalam run, staging engine di-bypass dan `stage_marker = 'POCI'` di-set. Hitung `current_lifetime_ecl`, compute `delta_ecl = current − baseline`, insert `ecl.poci_delta_log`. Jika baseline tidak ada → `POCI_BASELINE_MISSING` ke calc run error log (bukan halt seluruh run). Idempotency per `(calc_run_id, instrumen_id)` → `POCI_DELTA_DUPLICATE` jika sudah ada.
**Goal**: Setiap POCI instrumen memiliki delta ECL row per calc run. Staging engine tidak dipanggil. Direction enum INCREASE/DECREASE/UNCHANGED tersedia untuk jurnal sign convention dan dashboard.

### Pre-conditions
1. ECL calc run dalam status `RUNNING`; instrumen `is_poci = TRUE` dan `status = 'ACTIVE'`
2. `ecl.poci_baseline` tersedia untuk instrumen — jika tidak → `POCI_BASELINE_MISSING`
3. ECL parameter (PD curve, LGD pool, FL multiplier, bobot skenario) sudah ALCO-approved
4. FX rate APPROVED tersedia jika instrumen FCY (DEC-016)
5. Belum ada `ecl.poci_delta_log` untuk `(calc_run_id, instrumen_id)` — idempotency

### Acceptance Criteria

```gherkin
Feature: Hitung POCI delta ECL per instrumen dalam ECL calc run

  Background:
    Given ECL calc run CALC-2026-06-001: status = 'RUNNING', periode = PRD-2026-06
    And ALCO-approved parameters: W_Good=0.25, W_Normal=0.50, W_Bad=0.25

  Scenario: S2-AC1 — POCI instrumen: stage_marker = 'POCI', delta = current − baseline
    Given mst.instrumen POCI-DEP-0001: is_poci = TRUE, status = 'ACTIVE'
    And ecl.poci_baseline POCI-DEP-0001: lifetime_ecl_at_origination = 1250000000.0000
    And current_lifetime_ecl dihitung = 1450000000.0000 (deterioration — skenario Good/Normal/Bad × FL × bobot)
    When ECL calc run memproses POCI-DEP-0001
    Then staging engine tidak dipanggil untuk POCI-DEP-0001
    And stage_marker di ecl.calc_result_line = 'POCI' (bukan 1, 2, atau 3)
    And delta_ecl = 1450000000.0000 − 1250000000.0000 = 200000000.0000
    And ecl.poci_delta_log INSERT:
      | calc_run_id             | CALC-2026-06-001      |
      | instrumen_id            | POCI-DEP-0001         |
      | baseline_ecl            | 1250000000.0000       |
      | current_ecl             | 1450000000.0000       |
      | delta_ecl               | 200000000.0000        |
      | direction               | INCREASE              |
      | prior_delta_cumulative  | 50000000.0000 (dari run sebelumnya) |
    And aud.audit_log.action = POCI.DELTA_COMPUTED — in-transaction
      With after_jsonb: { calc_run_id, instrumen_id, baseline_ecl, current_ecl, delta_ecl, direction: "INCREASE" }

  Scenario: S2-AC2 — POCI delta negatif (improvement): direction = DECREASE
    Given ecl.poci_baseline POCI-OBL-0002: lifetime_ecl_at_origination = 800000000.0000
    And current_lifetime_ecl = 650000000.0000 (improvement — rating naik, kondisi membaik)
    When ECL calc run memproses POCI-OBL-0002
    Then delta_ecl = 650000000.0000 − 800000000.0000 = −150000000.0000
    And ecl.poci_delta_log INSERT: direction = 'DECREASE', delta_ecl = −150000000.0000
    And aud.audit_log.action = POCI.DELTA_COMPUTED — in-transaction dengan direction = 'DECREASE'

  Scenario: S2-AC3 — POCI_BASELINE_MISSING: baseline tidak ada → error log, run lanjut ke instrumen lain
    Given mst.instrumen POCI-NEW-9999: is_poci = TRUE, status = 'ACTIVE'
    And ecl.poci_baseline tidak ada untuk POCI-NEW-9999
    When ECL calc run menemukan POCI-NEW-9999
    Then ecl.calc_run_error_log INSERT:
      | calc_run_id  | CALC-2026-06-001      |
      | instrumen_id | POCI-NEW-9999         |
      | error_code   | POCI_BASELINE_MISSING |
      | error_detail | "Baseline tidak ditemukan. Pastikan penempatan POCI sudah di-approve dan baseline ter-capture (P5-M10-S1)." |
    And POCI-NEW-9999 dilewati; instrumen lain dalam run dilanjutkan (tidak halt)
    And notifikasi ke ROLE-RISK: "CALC-2026-06-001: 1 instrumen POCI baseline missing — lihat error log."

  Scenario: S2-AC4 — Idempotency: POCI_DELTA_DUPLICATE jika (calc_run_id, instrumen_id) sudah ada
    Given ecl.poci_delta_log sudah ada untuk (CALC-2026-06-001, POCI-DEP-0001)
    When ECL calc run mencoba insert ulang untuk (CALC-2026-06-001, POCI-DEP-0001) (mis. retry job)
    Then unique constraint (calc_run_id, instrumen_id) menolak insert
    And ecl.calc_run_error_log INSERT: error_code = POCI_DELTA_DUPLICATE
    And tidak ada duplikat di ecl.poci_delta_log
    And run melanjutkan instrumen berikutnya tanpa interrupt
```

---

## Story P5-M10-S3 — Jurnal P&L Booking per POCI Delta

**Actor**: System (auto-triggered post delta computation, dalam calc run atau setelah seal), ROLE-AKUN (review jurnal), ROLE-CFO (review large delta)
**Trigger**: Setelah `ecl.poci_delta_log` ter-insert dan calc run di-seal, sistem memposting jurnal `POCI_ECL_DELTA` via P5-M2 jurnal engine. Idempotency per `(calc_run_id, instrumen_id)`. Jika periode CLOSED → `POCI_PERIODE_LOCKED`. Jika direction di jurnal tidak match sign delta_ecl → `POCI_JURNAL_DIRECTION_MISMATCH` (bug guard). Jika `delta_ecl = 0` (UNCHANGED) → skip posting, tidak ada jurnal kosong.
**Goal**: P&L hanya dibebani/dikreditkan sebesar delta ECL POCI — bukan full lifetime ECL. Jurnal idempoten. Direction mismatch terdeteksi sebelum posting commit. Audit `POCI.DELTA_POSTED` in-transaction.

### Pre-conditions
1. `ecl.poci_delta_log` sudah tersedia untuk `(calc_run_id, instrumen_id)`
2. `mst.periode_buku.status_periode = 'OPEN'` — jika CLOSED → `POCI_PERIODE_LOCKED`
3. Jurnal event code `POCI_ECL_DELTA` sudah ada di mapping master (P5-M2)
4. Belum ada jurnal untuk `(calc_run_id, instrumen_id, 'POCI_ECL_DELTA')` — idempotency
5. `delta_ecl ≠ 0` — jika UNCHANGED, skip (no-op)

### Acceptance Criteria

```gherkin
Feature: Posting jurnal P&L POCI delta ECL per §5.5.14

  Background:
    Given ECL calc run CALC-2026-06-001 sudah di-seal
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And jurnal mapping POCI_ECL_DELTA tersedia di P5-M2 engine

  Scenario: S3-AC1 — Delta positif (INCREASE): D Beban Penurunan Nilai / K Cadangan ECL POCI
    Given ecl.poci_delta_log (CALC-2026-06-001, POCI-DEP-0001):
      | delta_ecl | 200000000.0000 |
      | direction | INCREASE       |
    When sistem memposting jurnal POCI_ECL_DELTA untuk (CALC-2026-06-001, POCI-DEP-0001)
      With Idempotency-Key: hash(CALC-2026-06-001 + POCI-DEP-0001 + POCI_ECL_DELTA)
    Then P5-M2 posting jurnal:
      | D Beban Penurunan Nilai ECL POCI (P&L) | 200000000.0000 |
      | K Cadangan ECL POCI (Neraca)           | 200000000.0000 |
    And jrnl.jurnal INSERT: event_code = 'POCI_ECL_DELTA', direction = 'INCREASE', amount = 200000000.0000
    And aud.audit_log.action = POCI.DELTA_POSTED — in-transaction
      With after_jsonb: { calc_run_id, instrumen_id, delta_ecl: 200000000.0000, direction: "INCREASE", jurnal_id }
    And notifikasi ke ROLE-AKUN: "Jurnal POCI delta INCREASE POCI-DEP-0001 IDR 200.000.000 terposting (run CALC-2026-06-001)."

  Scenario: S3-AC2 — Delta negatif (DECREASE): D Cadangan ECL POCI / K Pendapatan Pemulihan
    Given ecl.poci_delta_log (CALC-2026-06-001, POCI-OBL-0002):
      | delta_ecl | −150000000.0000 |
      | direction | DECREASE        |
    When sistem memposting jurnal POCI_ECL_DELTA untuk (CALC-2026-06-001, POCI-OBL-0002)
    Then P5-M2 posting jurnal:
      | D Cadangan ECL POCI (Neraca)                | 150000000.0000 |
      | K Pendapatan Pemulihan ECL POCI (P&L)       | 150000000.0000 |
    And jurnal amount = |delta_ecl| = 150000000.0000 (nilai absolut — sign dari direction enum)
    And aud.audit_log.action = POCI.DELTA_POSTED — in-transaction dengan direction = 'DECREASE'
    And notifikasi ke ROLE-CFO jika |delta_ecl| > threshold (default IDR 500juta dari sys.parameter)

  Scenario: S3-AC3 — POCI_PERIODE_LOCKED: periode CLOSED → reject posting
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED'
    And ecl.poci_delta_log untuk PRD-2026-05 instrumen POCI-DEP-0003 belum diposting
    When sistem mencoba post jurnal POCI_ECL_DELTA ke PRD-2026-05
    Then HTTP 423:
      | error.code    | POCI_PERIODE_LOCKED                                                            |
      | error.message | "Periode PRD-2026-05 sudah CLOSED. POCI delta tidak dapat diposting (DEC-010)." |
    And tidak ada INSERT ke jrnl.jurnal
    And ecl.poci_delta_log.posted_status = 'BLOCKED_PERIODE_CLOSED'
    And notifikasi ke ROLE-AKUN-CTL: "POCI delta POCI-DEP-0003 blocked — periode PRD-2026-05 CLOSED."

  Scenario: S3-AC4 — POCI_JURNAL_DIRECTION_MISMATCH: sign delta tidak sesuai direction enum → reject
    Given ecl.poci_delta_log: delta_ecl = 200000000.0000 (positif) tetapi direction = 'DECREASE' (bug data)
    When sistem mencoba post jurnal untuk entry ini
    Then HTTP 422:
      | error.code    | POCI_JURNAL_DIRECTION_MISMATCH                                                          |
      | error.message | "delta_ecl 200000000.0000 positif tetapi direction = DECREASE. Inkonsistensi data — posting dibatalkan." |
    And tidak ada INSERT ke jrnl.jurnal
    And aud.audit_log.action = POCI.DIRECTION_MISMATCH_DETECTED — in-transaction
    And alert ke ROLE-IT-ADMIN + ROLE-RISK untuk investigasi data `ecl.poci_delta_log` row
```

---

## Story P5-M10-S4 — Remove Warning POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA

**Actor**: System (ECL calc run engine update), ROLE-RISK (verifikasi warning hilang), ROLE-AUDIT (read result line)
**Trigger**: P5-M10 lands (deployed). ECL engine Phase 4 saat ini menghasilkan warning `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA` pada result line untuk instrumen POCI karena delta belum diimplementasi. Setelah M10 deployed, warning di-remove dari engine output dan diganti dengan nilai delta real. `GET /ecl/calc-run/{id}/result-line?type=POCI` harus return `delta_ecl` aktual, bukan warning. Audit `POCI.WARNING_REMOVED` di-log satu kali saat engine diupdate.
**Goal**: Warning obsolete dihapus bersih dari engine. Result line POCI menampilkan data yang benar: delta_ecl, direction, baseline_ecl, current_ecl. Tidak ada warning code stale yang menyesatkan ROLE-RISK atau laporan.

### Pre-conditions
1. P5-M10-S1 dan S2 sudah deployed — `ecl.poci_baseline` dan `ecl.poci_delta_log` tersedia
2. Phase 4 ECL engine source memiliki warning block `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA`
3. `GET /ecl/calc-run/{id}/result-line` Phase 4 mengembalikan `warnings[]` array

### Acceptance Criteria

```gherkin
Feature: Hapus warning POCI baseline dari ECL engine output dan tampilkan delta real

  Background:
    Given P5-M10-S1 + S2 deployed dan berjalan
    And ECL calc run CALC-2026-06-001 sudah sealed dengan POCI delta ter-compute

  Scenario: S4-AC1 — GET result-line?type=POCI returns delta_ecl bukan warning
    When ROLE-RISK (USR-RISK-001) mengirim GET /api/v1/ecl/calc-run/CALC-2026-06-001/result-line?type=POCI
    Then HTTP 200: setiap POCI result line berisi:
      | stage_marker    | POCI               |
      | delta_ecl       | 200000000.0000     | (real delta, bukan null atau placeholder) |
      | direction       | INCREASE           |
      | baseline_ecl    | 1250000000.0000    |
      | current_ecl     | 1450000000.0000    |
      | warnings        | [] (array kosong — TIDAK ada POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA) |
    And field `warnings` tidak mengandung string 'POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA' di seluruh response

  Scenario: S4-AC2 — Warning code dihapus dari ECL engine source; audit WARNING_REMOVED
    Given Phase 4 ECL engine code sebelumnya memiliki:
      warnings.append("POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA")
    When M10 engine update dideploy dan calc run baru CALC-2026-07-001 dijalankan
    Then warning block tersebut tidak ada di engine source (verified by code review / grep)
    And aud.audit_log satu kali INSERT:
      | action      | POCI.WARNING_REMOVED                                |
      | entity_type | ecl.calc_engine                                     |
      | after_jsonb | { removed_warning: "POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA", replaced_by: "delta_ecl field", effective_from: "CALC-2026-07-001" } |
    And aud entry ini immutable (append-only per DEC-018)

  Scenario: S4-AC3 — Calc run lama (pre-M10) masih return warning untuk traceability; tidak diubah retroaktif
    Given calc run CALC-2026-05-001 dibuat sebelum M10 dideploy
    When ROLE-AUDIT membuka GET /api/v1/ecl/calc-run/CALC-2026-05-001/result-line?type=POCI
    Then HTTP 200: result line lama masih tampilkan warnings: ["POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA"]
    And tidak ada retroactive update ke result line lama (immutable — DEC-018)
    And UI menampilkan badge "Run pra-M10 — warning legacy, bukan error" untuk run dengan calc_run_version < M10

  Scenario: S4-AC4 — Non-POCI instrumen dalam result line tidak terpengaruh warning removal
    Given ECL calc run CALC-2026-06-001 mengandung instrumen POCI dan non-POCI
    When GET /api/v1/ecl/calc-run/CALC-2026-06-001/result-line (tanpa filter type)
    Then instrumen non-POCI: stage = 1|2|3, delta_ecl = null, warnings = [] (sudah bersih dari sebelumnya)
    And instrumen POCI: stage_marker = 'POCI', delta_ecl terisi, warnings = []
    And tidak ada cross-contamination warning antara POCI dan non-POCI result line
```

---

## Story P5-M10-S5 — POCI Delta Dashboard + History

**Actor**: ROLE-RISK (utama — review delta per instrumen per periode), ROLE-CFO (aggregate delta besar), ROLE-AUDIT (export), ROLE-AKUN (jurnal review cross-check)
**Trigger**: User membuka `/poci/delta-history` (instrumen-level) atau dashboard portofolio POCI. `GET /poci/delta-history?instrumen_id=&periode=` mengembalikan movement delta kumulatif per instrumen. Dashboard MTD/YTD aggregate per portofolio. DataTable pattern (UX §1) dengan sort/filter/export. Large delta (> sys.parameter `POCI_LARGE_DELTA_THRESHOLD`) ditandai merah + alert ke ROLE-CFO.
**Goal**: ROLE-RISK dapat memonitor drift ECL POCI per instrumen dari baseline. ROLE-CFO melihat aggregate delta yang material. Export audit-compliant. Deep-link URL state.

### Pre-conditions
1. User ter-autentikasi minimum `ecl_run.read` permission
2. `ecl.poci_delta_log` dan `ecl.poci_baseline` ter-populate (S1+S2 running)
3. `sys.parameter 'POCI_LARGE_DELTA_THRESHOLD'` tersedia (default IDR 500.000.000)
4. Sort/filter/export sesuai UX rule §1

### Acceptance Criteria

```gherkin
Feature: POCI delta history dan dashboard per instrumen dan portofolio

  Background:
    Given ROLE-RISK (USR-RISK-001) ter-autentikasi dengan permission ecl_run.read
    And sys.parameter 'POCI_LARGE_DELTA_THRESHOLD' = 500000000.0000

  Scenario: S5-AC1 — GET /poci/delta-history: list delta per run untuk satu instrumen
    Given ecl.poci_delta_log memiliki 12 rows untuk POCI-DEP-0001 (Jan–Jun 2026, monthly runs)
    When USR-RISK-001 mengirim GET /api/v1/poci/delta-history?instrumen_id=POCI-DEP-0001&sort=calc_run_date:desc&limit=50
    Then HTTP 200:
      | data[]              | 12 rows terurut desc by calc_run_date               |
      | setiap row          | { calc_run_id, calc_run_date, baseline_ecl, current_ecl, delta_ecl, direction, prior_delta_cumulative } |
      | pagination.hasMore  | false (12 < 50)                                     |
      | appliedSort         | [{ col: "calc_run_date", dir: "desc" }]             |
    And filter direction=INCREASE mengembalikan hanya rows deterioration
    And export CSV/XLSX tersedia di GET /api/v1/poci/delta-history/export
      With audit POCI.EXPORT in-transaction (action, filters, row_count, actor)

  Scenario: S5-AC2 — Dashboard MTD/YTD aggregate delta per portofolio POCI
    When USR-RISK-001 mengirim GET /api/v1/poci/delta-history/summary?portofolio_id=PRT-POCI-01&year=2026&month=6
    Then HTTP 200:
      | data.portofolio_id           | PRT-POCI-01                         |
      | data.instrumen_count         | jumlah POCI instrumen aktif di portfolio |
      | data.delta_ecl_mtd_IDR       | Σ delta_ecl semua runs bulan Juni 2026   |
      | data.delta_ecl_ytd_IDR       | Σ delta_ecl Jan–Jun 2026                 |
      | data.net_cumulative_delta_IDR | Σ delta sejak origination semua instrumen |
      | data.direction_breakdown     | { INCREASE: count+amount, DECREASE: count+amount, UNCHANGED: count } |
    And Recharts chart tersedia di frontend: line chart cumulative delta per bulan

  Scenario: S5-AC3 — Large delta flag: delta > threshold → badge merah + alert ke ROLE-CFO
    Given ecl.poci_delta_log (CALC-2026-06-001, POCI-DEP-0001): delta_ecl = 750000000.0000 (> 500juta threshold)
    When halaman /poci/delta-history ditampilkan ke USR-RISK-001
    Then baris POCI-DEP-0001 menampilkan badge merah: "LARGE DELTA — IDR 750.000.000 > threshold"
    And alert otomatis dikirim ke ROLE-CFO: "POCI large delta detected: POCI-DEP-0001 run CALC-2026-06-001 IDR 750.000.000 (INCREASE). Review diperlukan."
    And aud.audit_log.action = POCI.LARGE_DELTA_ALERT — event (satu kali per (run, instrumen), bukan per pageload)

  Scenario: S5-AC4 — ROLE-AUDIT export delta history: filter aktif diaudit, non-AUDIT tidak bisa export full unfiltered
    Given ROLE-AUDIT (USR-AUDIT-001) request export semua POCI delta 2026
    When USR-AUDIT-001 mengirim GET /api/v1/poci/delta-history/export?year=2026&format=xlsx
    Then Asynq async job dibuat (> 10k row threshold — UX rule §3 — 202 Accepted + jobId)
    And saat selesai: file tersedia di MinIO exports/ dengan signed URL TTL 24h
    And aud.audit_log.action = POCI.EXPORT — in-transaction:
      { actor: USR-AUDIT-001, format: "xlsx", filters: { year: 2026 }, row_count: actual }
    And ROLE-AKUN (non-AUDIT) mencoba export tanpa filter seluruh data → HTTP 403:
      { error.code: "FORBIDDEN", error.message: "Export tanpa filter tidak diizinkan untuk role ROLE-AKUN." }
```

---

## Ringkasan P5-M10 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M10-S1 | Capture POCI ECL baseline saat penempatan approve | System, ROLE-AUDIT | 4 | **ifrs9-compliance-reviewer BLOCKING** (PSAK 71 §5.5.13 baseline) · **security-engineer BLOCKING** (audit immutability DEC-018) |
| P5-M10-S2 | Compute POCI delta pada ECL calc run | System, ROLE-RISK | 4 | **ifrs9-compliance-reviewer BLOCKING** (delta = current − baseline; NO stage 1 enforcement) |
| P5-M10-S3 | Jurnal P&L booking per POCI delta | System, ROLE-AKUN, ROLE-CFO | 4 | **ifrs9-compliance-reviewer BLOCKING** (§5.5.14 delta-only P&L; sign convention) · **security-engineer BLOCKING** (audit in-tx; idempotency) |
| P5-M10-S4 | Remove warning POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA | System, ROLE-RISK | 4 | advisory (code cleanup; immutability pre-M10 runs DEC-018) |
| P5-M10-S5 | POCI delta dashboard + history | ROLE-RISK, ROLE-CFO, ROLE-AUDIT | 4 | advisory (UX rule §1; large delta alert to CFO) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `POCI_BASELINE_MISSING` | 422 (calc run error log) | `ecl.poci_baseline` tidak ada saat compute delta untuk instrumen POCI | Per instrumen ke error log; tidak halt seluruh calc run |
| `POCI_BASELINE_IMMUTABLE_VIOLATION` | 422 | Upaya insert/update kedua ke `ecl.poci_baseline` untuk instrumen yang sudah ada baseline | WORM enforcement; DEC-018 |
| `POCI_DELTA_DUPLICATE` | 409 (calc run error log) | Unique constraint `(calc_run_id, instrumen_id)` di `ecl.poci_delta_log` violation | Idempotency guard saat retry job |
| `POCI_INSTRUMEN_NOT_POCI` | 422 | Endpoint POCI-specific dipanggil untuk instrumen dengan `is_poci = FALSE` | Guard di service layer |
| `POCI_PERIODE_LOCKED` | 423 | `mst.periode_buku.status_periode = 'CLOSED'` saat posting jurnal POCI delta | Blocking; jurnal tidak terposting |
| `POCI_JURNAL_DIRECTION_MISMATCH` | 422 | `delta_ecl > 0` tapi `direction = 'DECREASE'` atau sebaliknya (bug guard) | Reject posting; alert IT-ADMIN + RISK |

Catatan: `SOD_VIOLATION` (403), `FORBIDDEN` (403), `IDEMPOTENCY_REPLAY` (200), `NOT_FOUND` (404) sudah ada di api-conventions.md.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M10 | MFA Level |
|---|---|---|---|
| System (Asynq / ECL engine) | Service account | Capture baseline (S1), compute delta (S2), post jurnal (S3) | N/A |
| ROLE-APPR-TR | `transaksi.approve` | Approve penempatan POCI → trigger S1 baseline capture | Wajib jika Treasury Manager (DEC-026) |
| ROLE-RISK | `ecl_run.read`, `instrumen.read` | Review delta dashboard (S5), verifikasi warning hilang (S4) | Tidak wajib |
| ROLE-AKUN | `transaksi.read`, `jurnal.read` | Review jurnal POCI delta (S3 cross-check) | Tidak wajib |
| ROLE-CFO | `ecl_run.read`, alert receiver | Review large delta alert (S5-AC3); MFA wajib | WAJIB (DEC-026) |
| ROLE-AUDIT | `ecl_run.read`, `audit_log.read` | Read-only baseline + delta log + export (S1-AC4, S5-AC4) | Tidak wajib |
| ROLE-IT-ADMIN | `sys.dlq.read`, alert receiver | Menerima alert `POCI_JURNAL_DIRECTION_MISMATCH`; DLQ review | WAJIB |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `ecl.poci_baseline` (Phase 4 schema) | Phase 4 ECL → P5-M10-S1 | M10 menulis ke tabel ini; verify kolom `cashflow_expectasi_jsonb`, `credit_adjusted_eir`, `origination_date` sudah ada di migration Phase 4 |
| Penempatan POCI approve event | P5-M1 → P5-M10-S1 | S1 triggered dari approve penempatan; `is_poci` flag di `mst.instrumen` harus sudah ada |
| Jurnal event code `POCI_ECL_DELTA` | P5-M2 → P5-M10-S3 | Harus di-seed di mapping master sebelum M10 S3 posting pertama; konfirmasi akun debet/kredit dengan ROLE-AKUN |
| Phase 4 ECL calc run trigger | Phase 4 → P5-M10-S2 | Calc run yang sama yang menjalankan SICR/staging untuk non-POCI harus juga trigger POCI delta computation untuk `is_poci = TRUE` instrumen |
| `credit_adjusted_eir` dari amortisasi schedule | P5-M9 (S4-AC3) → P5-M10 | POCI credit-adjusted EIR digunakan oleh M9 akrual dan M10 verifikasi baseline EIR; harus konsisten |
| `mst.periode_buku.status_periode = 'OPEN'` | P5-M4 → P5-M10-S3 | Jurnal POCI delta tidak bisa post ke periode CLOSED |
| FX rate APPROVED | P5-M6 → P5-M10-S2 | EAD_IDR = EAD_FCY × FX_rate_JISDOR jika instrumen POCI FCY |

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — S1, S2, S3)
- [ ] **S1**: `lifetime_ecl_at_origination` di baseline harus dihitung sebagai **lifetime ECL** (bukan 12-month), per §5.5.13 — POCI tidak pernah di Stage 1
- [ ] **S1**: Apakah `cashflow_expectasi_jsonb` di baseline harus menyimpan detail CF PD-adjusted per periode, atau cukup summary scalar? Konfirmasi dengan ROLE-RISK untuk audit trail kecukupan
- [ ] **S2**: Staging engine **tidak boleh dipanggil** untuk POCI instrumen — konfirmasi implementasi engine Phase 4 sudah ada gate `if is_poci { skip_staging() }` atau harus ditambahkan di M10
- [ ] **S2**: `prior_delta_cumulative` calculation: apakah Σ semua runs sebelumnya atau hanya periodik tahunan? Konfirmasi kebijakan Tugure
- [ ] **S3**: Sign convention jurnal: INCREASE → D Beban Penurunan / K Cadangan; DECREASE → D Cadangan / K Pendapatan Pemulihan — pastikan sama dengan treatment ECL non-POCI Stage 1↔2 transition untuk konsistensi laporan
- [ ] **S3**: `delta_ecl = 0` (UNCHANGED) → no-op, tidak ada jurnal — konfirmasi apakah perlu null-jurnal entry untuk completeness laporan atau benar-benar skip
- [ ] **S4**: Calc run lama (pre-M10) warning tetap ada retroaktif — **immutable per DEC-018**; konfirmasi UI treatment "badge legacy" tidak menyesatkan auditor eksternal

### Untuk security-engineer (BLOCKING — S1 immutability, S3 in-transaction audit)
- [ ] `ecl.poci_baseline`: constraint di DB layer (unique `instrumen_id`, no UPDATE trigger) sebagai defence-in-depth selain service layer check
- [ ] Semua audit events in-transaction: `POCI.BASELINE_CAPTURED` (S1), `POCI.DELTA_COMPUTED` (S2), `POCI.DELTA_POSTED` (S3), `POCI.WARNING_REMOVED` (S4 — satu kali, tidak per run)
- [ ] `POCI.BASELINE_VIOLATION_ATTEMPT` di-log meski request gagal — audit threat detection
- [ ] Idempotency `(calc_run_id, instrumen_id)` di `ecl.poci_delta_log`: DB unique constraint wajib, bukan hanya service check
- [ ] Jurnal idempotency key pattern: `hash(calc_run_id + instrumen_id + 'POCI_ECL_DELTA')` — konfirmasi tidak collision dengan event code lain di `sys.idempotency_key`
- [ ] Large delta alert ke ROLE-CFO (S5-AC3): alert dikirim via system notification channel, tidak via user-input data (prevent injection di alert message)
- [ ] Export POCI delta (S5-AC4): ROLE-AKUN non-filter export ditolak — permission check `if !hasRole(AUDIT) && filter == empty { return 403 }`
- [ ] `POCI.LARGE_DELTA_ALERT` satu kali per `(calc_run_id, instrumen_id)` — de-duplication di service layer; tidak per pageload (avoid alert flood)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract (6 endpoints baru) + state machine `ecl.poci_delta_log.direction` + `ecl.poci_baseline` immutability constraint, ke `ifrs9-compliance-reviewer` untuk BLOCKING review S1 (§5.5.13 baseline capture), S2 (NO staging enforcement — critical), S3 (§5.5.14 delta-only P&L jurnal + sign convention), dan ke `security-engineer` untuk audit immutability S1 + in-transaction completeness S1–S4. `data-modeler` dapat mulai migration `ecl.poci_delta_log` + unique constraint setelah compliance gate S2 cleared. `ifrs9-compliance-reviewer` harus dipanggil SEBELUM implementasi S2 dan S3 dimulai — ini adalah critical path PSAK 71 §5.5.13-14._
