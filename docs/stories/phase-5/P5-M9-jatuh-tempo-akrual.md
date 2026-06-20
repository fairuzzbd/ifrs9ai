# P5-M9 — APP-B Jatuh Tempo + Pendapatan Akrual Harian: User Stories

**Story Set ID**: P5-M9
**Modul**: APP-B — Transaction Lifecycle (Jatuh Tempo + Akrual, Phase 5)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `ifrs9-compliance-reviewer`
**Author**: business-analyst
**Tanggal**: 2026-06-20
**Linked FSD**: FSD-APP-B-TransactionLifecycle-v1.0.docx §8 (Jatuh Tempo/Maturity), §9 (Akrual Harian EIR), §10 (Dividen + Distribusi), §11 (Amortisasi Premium/Diskon)
**Linked BRD**: BRD §6.4 (APP-B Akrual + Maturitas), RACI: ROLE-RISK (A untuk Stage 3), ROLE-AKUN (R akrual review), ROLE-AKUN-CTL (A override), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-010` (LOCKED) — ECL 3-stage × 3-skenario; Stage 3 PD = 1.0; bunga Stage 3 = Net Carrying × EIR (**never Gross**)
- `DEC-013` (LOCKED) — EIR Newton-Raphson; presisi 8 desimal; re-estimation = insert schedule_version baru (**NEVER UPDATE**)
- `DEC-016` (LOCKED) — `shopspring/decimal`; `NUMERIC(20,4)` IDR; **never float64**
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint

**Dependensi**:
- **P5-M1** — `mst.instrumen` ACTIVE, klasifikasi PSAK 71 locked; akrual hanya untuk instrumen `status = 'ACTIVE'`
- **P5-M2** — jurnal engine; event codes `AKRUAL_BUNGA`, `AMORTISASI_PD`, `DIVIDEN`, `MATURITY_SETTLEMENT` harus tersedia di mapping master
- **P5-M6** — MTM; FX rate APPROVED per tanggal akrual (multi-currency)
- **Phase 4 ECL core** — `ecl.calc_result_line` latest sealed run per instrumen per periode diperlukan untuk Net Carrying (Stage 3) dan validasi staging currency

**Handoff berikutnya**:
- `system-analyst` → OpenAPI: 5 endpoints (`GET /trx/akrual`, `GET /trx/akrual/{instrumen_id}/summary`, `GET /trx/dividen`, `GET /trx/jatuh-tempo`, `GET /jobs/{jobId}`); state machine `trx.jatuh_tempo.status`; 7 error codes baru (§Error Codes Proposed); SSE stream untuk cron jobs
- `data-modeler` → migration: `trx.jatuh_tempo`, `trx.pendapatan_akrual`, `trx.dividen`, `ecl.amortisasi_schedule` (POCI version), idempotency per `(instrumen_id, tanggal_akrual)`; holiday calendar table `sys.holiday_calendar`
- `ifrs9-compliance-reviewer` → **BLOCKING gate**: (a) Stage 3 net carrying basis PSAK 71 §5.4.1(b); (b) POCI credit-adjusted EIR; (c) FVOCI Election dividen ke OCI vs FVTPL dividen ke P&L; (d) amortisasi premium/diskon EIR schedule immutability (DEC-013)
- `security-engineer` → audit in-transaction semua cron events; DLQ review permission (ROLE-IT-ADMIN); SoD cron service account vs human override

**Compliance path**: P5-M9 menyentuh Stage 3 Net Carrying (PSAK 71 §5.4.1(b)) dan POCI credit-adjusted EIR — **ifrs9-compliance-reviewer BLOCKING** wajib sebelum implementasi S1, S2, S4. **security-engineer BLOCKING** untuk audit completeness semua cron events (S1–S5).

---

## Konteks & Arsitektur P5-M9

### Cron Schedule Harian

```
09:00 WIB — Jatuh Tempo cron (S1)
              Asynq job: MATURITY_PROCESS_JOB
              Per instrumen ACTIVE dengan tanggal_jatuh_tempo = today
              → maturity settlement + derecognition

09:15 WIB — Akrual Harian cron (S2)
              Asynq job: DAILY_ACCRUAL_JOB
              Per instrumen ACTIVE non-FVTPL
              → hitung bunga harian EIR per stage
              → insert trx.pendapatan_akrual
              → jurnal AKRUAL_BUNGA via P5-M2

10:00 WIB — Amortisasi Premium/Diskon cron (S4)
              Asynq job: AMORTISASI_PD_JOB
              Per instrumen AC/FVOCI saja
              → insert ecl.amortisasi_schedule entry (schedule_version+1)
              → jurnal AMORTISASI_PD via P5-M2

Trigger manual — Dividen / Distribusi (S3)
              ROLE-MAKER-TR input → ROLE-APPR-TR approve (4-eyes)
```

### Kalkulasi Akrual per Stage

```
# Stage 1 dan Stage 2
akrual_harian = Gross_Carrying_Amount × EIR / 365

# Stage 3 (PSAK 71 §5.4.1(b))
akrual_harian = Net_Carrying_Amount × EIR / 365
             di mana:
             Net_Carrying_Amount = Gross_Carrying_Amount − ECL
             ECL diambil dari ecl.calc_result_line latest sealed run

# POCI
akrual_harian = Gross_Carrying_Amount × credit_adjusted_EIR / 365
             credit_adjusted_EIR dari ecl.amortisasi_schedule versi POCI

# Multi-currency
akrual_harian_IDR = akrual_harian_FCY × FX_rate_APPROVED(tanggal_akrual)

# Semua operasi: shopspring/decimal, HALF_EVEN rounding
```

### Idempotency per Akrual

```
Unique constraint: (instrumen_id, tanggal_akrual, akrual_type)
akrual_type: 'BUNGA' | 'AMORTISASI_PD' | 'DIVIDEN'
Idempotency-Key untuk cron: hash(job_run_id + instrumen_id + tanggal_akrual)
```

### Error ke DLQ

Setiap item yang gagal dalam cron (per instrumen) → insert ke `sys.dlq`:
```json
{
  "job_type": "DAILY_ACCRUAL_JOB",
  "instrumen_id": "<uuid>",
  "tanggal_akrual": "2026-06-20",
  "error_code": "AKRUAL_EIR_NOT_FOUND",
  "error_detail": "...",
  "retry_count": 0,
  "max_retry": 3
}
```
Cron tidak halt karena 1 instrumen gagal. Proses instrumen lain dilanjutkan.

---

## Story P5-M9-S1 — Jatuh Tempo Cron 09:00 WIB

**Actor**: System cron (Asynq), ROLE-AUDIT (read), ROLE-AKUN (notifikasi)
**Trigger**: Asynq job `MATURITY_PROCESS_JOB` berjalan 09:00 WIB setiap hari kerja (holiday calendar skip). Per instrumen `mst.instrumen` dengan `status = 'ACTIVE'` dan `tanggal_jatuh_tempo = today`.
**Goal**: Settlement maturity: deposito → pokok + bunga last + PPh 20% dipotong ke kas; bond → derecognition amortized carrying; semua → `status = 'MATURED'`. Insert `trx.jatuh_tempo` row. Jurnal `MATURITY_SETTLEMENT` via P5-M2. Audit `MATURITY.DERECOGNIZED` in-transaction. Instrumen tidak ACTIVE → `MATURITY_INSTRUMEN_NOT_ACTIVE` ke DLQ; tidak halt proses instrumen lain.

### Pre-conditions
1. `mst.instrumen.status = 'ACTIVE'` dan `tanggal_jatuh_tempo = today`
2. `mst.periode_buku.status_periode = 'OPEN'` untuk tanggal hari ini
3. Holiday calendar `sys.holiday_calendar` tidak menandai hari ini sebagai libur
4. `ecl.amortisasi_schedule` aktif tersedia untuk carrying amount final

### Acceptance Criteria

```gherkin
Feature: Jatuh tempo cron 09:00 WIB — maturity settlement + derecognition

  Background:
    Given Asynq job MATURITY_PROCESS_JOB dijadwalkan 09:00 WIB setiap hari kerja
    And sys.holiday_calendar tidak mencatat 2026-06-20 sebagai hari libur
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'

  Scenario: S1-AC1 — Deposito jatuh tempo: pokok + bunga last + PPh 20% → kas, status MATURED
    Given mst.instrumen DEP-0055:
      | status              | ACTIVE              |
      | klasifikasi_psak71  | AC                  |
      | tanggal_jatuh_tempo | 2026-06-20          |
      | pokok_IDR           | 5000000000.0000     |
      | bunga_last_IDR      | 87671.2329 (akrual hari terakhir dari trx.pendapatan_akrual) |
    When MATURITY_PROCESS_JOB memproses DEP-0055
    Then pph_dipotong = bunga_last_IDR × 0.20 = 17534.2466 (NUMERIC(20,4), HALF_EVEN)
    And net_diterima = pokok_IDR + bunga_last_IDR − pph_dipotong = 5000070136.9863
    And trx.jatuh_tempo INSERT:
      | instrumen_id        | DEP-0055            |
      | tanggal_jatuh_tempo | 2026-06-20          |
      | pokok_IDR           | 5000000000.0000     |
      | bunga_last_IDR      | 87671.2329          |
      | pph_IDR             | 17534.2466          |
      | net_kas_IDR         | 5000070136.9863     |
    And P5-M2 memposting jurnal event_code = 'MATURITY_SETTLEMENT':
      | Dr Kas/Bank                  | 5000070136.9863 |
      | Dr Utang PPh Final           | 17534.2466      |
      | Cr Deposito AC (DEP-0055)    | 5000000000.0000 |
      | Cr Pendapatan Bunga Akrual   | 87671.2329      |
    And mst.instrumen DEP-0055: status = 'MATURED', updated_at = now()
    And aud.audit_log.action = MATURITY.DERECOGNIZED — in-transaction
      With after_jsonb: { instrumen_id, pokok_IDR, bunga_last_IDR, pph_IDR, net_kas_IDR }
    And notifikasi ke ROLE-AKUN: "Deposito DEP-0055 jatuh tempo 2026-06-20. Net kas: IDR 5.000.070.136,99. Jurnal diposting."

  Scenario: S1-AC2 — Bond jatuh tempo: derecognition amortized carrying, realized G/L = 0 (par)
    Given mst.instrumen OBL-0099:
      | status              | ACTIVE           |
      | klasifikasi_psak71  | AC               |
      | tanggal_jatuh_tempo | 2026-06-20       |
      | amortized_carrying  | 10000000000.0000 (par — sudah fully amortized) |
    When MATURITY_PROCESS_JOB memproses OBL-0099
    Then P5-M2 memposting jurnal MATURITY_SETTLEMENT:
      | Dr Kas/Bank              | 10000000000.0000 | Face value diterima |
      | Cr Aset Bond AC          | 10000000000.0000 | Carrying = par, G/L = 0 |
    And mst.instrumen OBL-0099: status = 'MATURED'
    And aud.audit_log.action = MATURITY.DERECOGNIZED — in-transaction

  Scenario: S1-AC3 — Instrumen sudah MATURED atau tidak ACTIVE saat cron berjalan → DLQ, instrumen lain lanjut
    Given mst.instrumen DEP-0060: status = 'DISPOSED' (sudah dijual sebelumnya, tanggal_jatuh_tempo = today karena data lama)
    When MATURITY_PROCESS_JOB menemukan DEP-0060 dalam query
    Then sys.dlq INSERT:
      | job_type    | MATURITY_PROCESS_JOB         |
      | instrumen_id | DEP-0060                    |
      | error_code  | MATURITY_INSTRUMEN_NOT_ACTIVE |
      | error_detail | "status = DISPOSED, hanya ACTIVE yang eligible" |
    And DEP-0060 dilewati, job melanjutkan ke instrumen berikutnya
    And notifikasi ke ROLE-IT-ADMIN: "DLQ: 1 instrumen gagal proses maturity — lihat sys.dlq entry job_run_id=..."

  Scenario: S1-AC4 — Hari libur nasional: cron skip, tidak ada processing
    Given sys.holiday_calendar mencatat 2026-06-17 sebagai 'LIBUR_NASIONAL'
    When MATURITY_PROCESS_JOB dijadwalkan 09:00 WIB 2026-06-17
    Then job exit early dengan log: "Holiday detected (2026-06-17). Maturity processing skipped."
    And tidak ada INSERT ke trx.jatuh_tempo atau mutasi mst.instrumen
    And tidak ada jurnal terposting
    And aud.audit_log.action = MATURITY.HOLIDAY_SKIP — informatif
```

---

## Story P5-M9-S2 — Pendapatan Akrual Harian Cron

**Actor**: System cron (Asynq), ROLE-AKUN (review akrual list), ROLE-RISK (oversight Stage 3)
**Trigger**: Asynq job `DAILY_ACCRUAL_JOB` berjalan 09:15 WIB setiap hari kerja. Per instrumen `status = 'ACTIVE'` non-FVTPL. Skip jika hari libur. Idempotency per `(instrumen_id, tanggal_akrual, 'BUNGA')`.
**Goal**: Hitung akrual EIR harian per stage. Stage 1/2: `Gross × EIR / 365`. Stage 3: `(Gross − ECL) × EIR / 365` — ECL dari `ecl.calc_result_line` latest sealed (per lesson P5-M8 B1). POCI: credit-adjusted EIR. Multi-currency: FCY × FX rate APPROVED. Insert `trx.pendapatan_akrual`. Jurnal `AKRUAL_BUNGA` via P5-M2. Audit `AKRUAL.POSTED` in-transaction. Failure per instrumen → DLQ, tidak halt.

### Pre-conditions
1. `mst.instrumen.status = 'ACTIVE'` dan `klasifikasi_psak71 ≠ 'FVTPL'`
2. `mst.periode_buku.status_periode = 'OPEN'` — jika CLOSED → `AKRUAL_PERIODE_LOCKED` ke DLQ
3. EIR tersedia di `ecl.amortisasi_schedule` (latest `schedule_version` per instrumen)
4. FX rate APPROVED tersedia di `sys.fx_rate` untuk tanggal akrual (jika FCY)
5. Belum ada `trx.pendapatan_akrual` untuk `(instrumen_id, tanggal_akrual, 'BUNGA')` — idempotency

### Acceptance Criteria

```gherkin
Feature: Akrual EIR harian per instrumen per stage

  Background:
    Given Asynq job DAILY_ACCRUAL_JOB berjalan 09:15 WIB 2026-06-20
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And sys.holiday_calendar: 2026-06-20 bukan hari libur

  Scenario: S2-AC1 — Stage 1 instrumen (AC, FVOCI): akrual Gross × EIR / 365
    Given mst.instrumen OBL-0101:
      | status             | ACTIVE     |
      | klasifikasi_psak71 | AC         |
      | staging            | Stage 1    |
      | gross_carrying_IDR | 10000000000.0000 |
      | eir                | 0.07500000 (7.5% per annum) |
    And ecl.amortisasi_schedule OBL-0101: eir = 0.07500000, schedule_version = 1 (aktif)
    When DAILY_ACCRUAL_JOB memproses OBL-0101 tanggal 2026-06-20
    Then akrual_harian = 10000000000.0000 × 0.07500000 / 365 = 2054794.5205 (NUMERIC(20,4), HALF_EVEN)
    And trx.pendapatan_akrual INSERT:
      | instrumen_id  | OBL-0101          |
      | tanggal_akrual | 2026-06-20       |
      | akrual_type   | BUNGA             |
      | stage         | 1                 |
      | basis_IDR     | 10000000000.0000  |
      | eir           | 0.07500000        |
      | akrual_IDR    | 2054794.5205      |
    And P5-M2 memposting jurnal event_code = 'AKRUAL_BUNGA':
      | Dr Akrual Bunga Piutang    | 2054794.5205 |
      | Cr Pendapatan Bunga        | 2054794.5205 |
    And aud.audit_log.action = AKRUAL.POSTED — in-transaction
      With after_jsonb: { instrumen_id, stage: 1, basis: "GROSS", akrual_IDR: 2054794.5205 }

  Scenario: S2-AC2 — Stage 3 instrumen: akrual Net Carrying (Gross − ECL) × EIR / 365, PSAK 71 §5.4.1(b)
    Given mst.instrumen OBL-0202:
      | status             | ACTIVE          |
      | klasifikasi_psak71 | AC              |
      | staging            | Stage 3         |
      | gross_carrying_IDR | 8000000000.0000 |
    And ecl.calc_result_line latest sealed untuk OBL-0202: ecl_IDR = 2400000000.0000
    And ecl.amortisasi_schedule OBL-0202: eir = 0.09000000
    When DAILY_ACCRUAL_JOB memproses OBL-0202 tanggal 2026-06-20
    Then net_carrying = 8000000000.0000 − 2400000000.0000 = 5600000000.0000
    And akrual_harian = 5600000000.0000 × 0.09000000 / 365 = 1380821.9178 (NUMERIC(20,4), HALF_EVEN)
    And trx.pendapatan_akrual INSERT: basis_IDR = 5600000000.0000 (Net, bukan Gross), stage = 3
    And aud.audit_log mencatat: { basis: "NET_CARRYING", gross: 8000000000.0000, ecl: 2400000000.0000, net: 5600000000.0000 }
    And jika ecl.calc_result_line tidak ada sealed run untuk OBL-0202 → sys.dlq: AKRUAL_STAGING_STALE

  Scenario: S2-AC3 — FCY instrumen: akrual dalam FCY dikonversi ke IDR via FX rate APPROVED
    Given mst.instrumen BOND-USD-003:
      | status             | ACTIVE        |
      | mata_uang          | USD           |
      | gross_carrying_FCY | 5000000.0000  |
      | staging            | Stage 1       |
      | eir                | 0.05000000    |
    And sys.fx_rate untuk USD tanggal 2026-06-20: rate_IDR = 16200.00000000, status = 'APPROVED'
    When DAILY_ACCRUAL_JOB memproses BOND-USD-003
    Then akrual_FCY = 5000000.0000 × 0.05000000 / 365 = 684.9315 USD (NUMERIC(20,4))
    And akrual_IDR = 684.9315 × 16200.00000000 = 11095890.4110 (NUMERIC(20,4))
    And trx.pendapatan_akrual INSERT: akrual_FCY = 684.9315, akrual_IDR = 11095890.4110, fx_rate_id = <id sys.fx_rate>
    And jika sys.fx_rate untuk USD 2026-06-20 tidak ada status APPROVED → sys.dlq: AKRUAL_FX_RATE_MISSING

  Scenario: S2-AC4 — Duplikat akrual pada hari yang sama: idempotency guard, tidak double-insert
    Given trx.pendapatan_akrual sudah ada untuk (OBL-0101, 2026-06-20, 'BUNGA')
    When DAILY_ACCRUAL_JOB mencoba insert ulang untuk OBL-0101 pada 2026-06-20
    Then unique constraint (instrumen_id, tanggal_akrual, akrual_type) menolak insert
    And sys.dlq INSERT: AKRUAL_DUPLICATE dengan instrumen_id = OBL-0101, tanggal_akrual = 2026-06-20
    And instrumen lain dalam batch dilanjutkan
    And tidak ada jurnal duplikat terposting
```

---

## Story P5-M9-S3 — Dividen + Distribusi Reksadana

**Actor**: ROLE-MAKER-TR (input), ROLE-APPR-TR (approve, 4-eyes), System (jurnal posting)
**Trigger**: ROLE-MAKER-TR menerima pemberitahuan dividen dari emiten atau distribusi reksadana dari MI, lalu input via form `/trx/dividen/new`. Submit → ROLE-APPR-TR approve. Pada approve: PPh final 10% dipotong (UU PPh §17 ayat 2c). Jurnal `DIVIDEN` via P5-M2. Untuk FVOCI Election: ke OCI atau P&L per kebijakan akuntansi Tugure (dikonfirmasi saat klasifikasi — bukan PSAK 71 requirement). Untuk FVTPL/Reksadana: ke P&L.
**Goal**: Dividen terposting sesuai klasifikasi. Audit `DIVIDEN.POSTED` in-transaction.

### Pre-conditions
1. ROLE-MAKER-TR ter-autentikasi dengan permission `transaksi.create`
2. `mst.instrumen.status = 'ACTIVE'`
3. Idempotency-Key wajib di setiap submission
4. `mst.periode_buku.status_periode = 'OPEN'`

### Acceptance Criteria

```gherkin
Feature: Input dan posting dividen / distribusi reksadana

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And ROLE-MAKER-TR (USR-MAKER-001) dan ROLE-APPR-TR (USR-APPR-001, ≠ maker)

  Scenario: S3-AC1 — Dividen saham FVTPL: net dividen (setelah PPh 10%) ke P&L
    Given mst.instrumen SAH-0200:
      | status             | ACTIVE |
      | klasifikasi_psak71 | FVTPL  |
    And USR-MAKER-001 menginput: gross_dividen_IDR = 50000000.0000, tanggal_cum_date = 2026-06-15
    When USR-MAKER-001 submit POST /api/v1/trx/dividen
      With Idempotency-Key: IK-DIV-001
    Then trx.dividen INSERT: status = 'PENDING_APPROVAL'
    And saat USR-APPR-001 approve:
      pph_final = 50000000.0000 × 0.10 = 5000000.0000
      net_dividen = 45000000.0000
    And P5-M2 posting jurnal DIVIDEN:
      | Dr Kas/Bank                | 45000000.0000 |
      | Dr Beban PPh Final         | 5000000.0000  |
      | Cr Pendapatan Dividen P&L  | 50000000.0000 |
    And aud.audit_log.action = DIVIDEN.POSTED — in-transaction
      With after_jsonb: { instrumen_id, gross_dividen: 50000000.0000, pph: 5000000.0000, net: 45000000.0000, klasifikasi: "FVTPL" }
    And toast ke USR-APPR-001: "Dividen SAH-0200 IDR 45.000.000 (net setelah PPh 10%) berhasil diposting ke P&L."

  Scenario: S3-AC2 — Distribusi reksadana (look-through, FVTPL): net distribusi ke P&L
    Given mst.instrumen RD-CAMPURAN-01:
      | status             | ACTIVE |
      | klasifikasi_psak71 | FVTPL  |
      | is_reksadana       | TRUE   |
    And USR-MAKER-001 input: gross_distribusi_IDR = 12000000.0000 (dari NAB distribusi MI)
    When USR-APPR-001 approve
    Then pph_final = 1200000.0000; net_distribusi = 10800000.0000
    And jurnal DIVIDEN: Dr Kas 10800000 + Dr PPh 1200000 / Cr Pendapatan Distribusi P&L 12000000.0000
    And aud.audit_log.action = DIVIDEN.POSTED — in-transaction dengan is_reksadana = TRUE

  Scenario: S3-AC3 — SoD: maker mencoba approve dividen sendiri → SOD_VIOLATION
    Given trx.dividen DIV-0055: maker_id = USR-MAKER-001
    When USR-MAKER-001 mengirim POST /api/v1/trx/dividen/DIV-0055/approve
      With Idempotency-Key: IK-DIV-SOD-001
    Then HTTP 403:
      | error.code | SOD_VIOLATION |
      | error.message | "maker tidak dapat menjadi approver untuk dividen yang sama (DEC-017)." |
    And trx.dividen.status tetap 'PENDING_APPROVAL'

  Scenario: S3-AC4 — Gross dividen kosong atau ≤ 0 → DIVIDEN_VALIDATION_FAILED
    Given USR-MAKER-001 menginput: gross_dividen_IDR = 0.0000
    When USR-MAKER-001 submit POST /api/v1/trx/dividen
      With Idempotency-Key: IK-DIV-002
    Then HTTP 422:
      | error.code             | DIVIDEN_VALIDATION_FAILED          |
      | error.details[0].field | gross_dividen_IDR                  |
      | error.details[0].rule  | "gross_dividen_IDR harus > 0"      |
    And tidak ada INSERT ke trx.dividen
```

---

## Story P5-M9-S4 — Amortisasi Premium/Diskon Bond AC/FVOCI

**Actor**: System cron (Asynq), ROLE-AKUN (review), ROLE-AUDIT (read)
**Trigger**: Asynq job `AMORTISASI_PD_JOB` berjalan 10:00 WIB setiap hari kerja. Per instrumen `klasifikasi_psak71 IN ('AC', 'FVOCI')` dengan jadwal amortisasi dari `ecl.amortisasi_schedule`. POCI: gunakan credit-adjusted EIR dari versi POCI di schedule. Insert entry baru di `ecl.amortisasi_schedule` (schedule_version + 1 jika ada amendment) — **NEVER UPDATE existing rows** (DEC-013). Jurnal `AMORTISASI_PD` via P5-M2.
**Goal**: Carrying amount diamortisasi per schedule EIR. Premium turun / Diskon naik ke P&L. Immutability schedule terjaga. Audit `AMORTISASI.POSTED` in-transaction.

### Pre-conditions
1. `mst.instrumen.klasifikasi_psak71 IN ('AC', 'FVOCI')` dan `status = 'ACTIVE'`
2. `ecl.amortisasi_schedule` aktif tersedia (latest `schedule_version`, `effective_to = 'infinity'`)
3. `mst.periode_buku.status_periode = 'OPEN'`
4. Belum ada amortisasi entry untuk `(instrumen_id, tanggal_akrual)` — idempotency

### Acceptance Criteria

```gherkin
Feature: Amortisasi premium/diskon bond AC/FVOCI via EIR schedule

  Background:
    Given Asynq job AMORTISASI_PD_JOB berjalan 10:00 WIB 2026-06-20
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'

  Scenario: S4-AC1 — Bond premium AC: amortisasi premium turunkan carrying, posting ke P&L
    Given mst.instrumen OBL-0303: klasifikasi_psak71 = 'AC', status = 'ACTIVE'
    And ecl.amortisasi_schedule OBL-0303 (schedule_version = 1, effective_to = 'infinity'):
      | eir                     | 0.06000000       |
      | kupon_rate              | 0.07000000       |
      | carrying_amount_awal    | 10500000000.0000 |
      | premium_sisa            | 500000000.0000   |
      | tanggal_efektif         | 2026-06-20 (entry hari ini per schedule) |
      | amortisasi_harian       | 136986.3014 (pre-calculated dari schedule) |
    When AMORTISASI_PD_JOB memproses OBL-0303 2026-06-20
    Then P5-M2 posting jurnal AMORTISASI_PD:
      | Dr Beban Premium (P&L)     | 136986.3014 |
      | Cr Aset Bond AC (OBL-0303) | 136986.3014 | (carrying turun = premium amortisasi)
    And ecl.amortisasi_schedule: row lama TIDAK diupdate; carrying_amount_baru dicatat di kolom atau schedule entry berikutnya
    And aud.audit_log.action = AMORTISASI.POSTED — in-transaction
      With after_jsonb: { instrumen_id, schedule_version: 1, amortisasi_IDR: 136986.3014, premium_atau_diskon: "PREMIUM" }

  Scenario: S4-AC2 — Bond diskon FVOCI: amortisasi diskon naikkan carrying, posting ke P&L (income)
    Given mst.instrumen OBL-0404: klasifikasi_psak71 = 'FVOCI', status = 'ACTIVE'
    And ecl.amortisasi_schedule OBL-0404:
      | eir          | 0.09000000       |
      | kupon_rate   | 0.08000000       |
      | diskon_sisa  | 300000000.0000   |
      | amortisasi_harian | 82191.7808  |
    When AMORTISASI_PD_JOB memproses OBL-0404
    Then P5-M2 posting jurnal AMORTISASI_PD:
      | Dr Aset Bond FVOCI (OBL-0404) | 82191.7808 | (carrying naik = diskon amortisasi)
      | Cr Pendapatan Amortisasi (P&L) | 82191.7808 |
    And aud.audit_log.action = AMORTISASI.POSTED — in-transaction

  Scenario: S4-AC3 — POCI bond: amortisasi via credit-adjusted EIR dari schedule versi POCI
    Given mst.instrumen POCI-0010: klasifikasi_psak71 = 'AC', is_poci = TRUE
    And ecl.amortisasi_schedule POCI-0010 versi POCI:
      | credit_adjusted_eir | 0.04500000 (lebih rendah dari gross EIR karena PD-adjusted) |
      | schedule_version    | 2 (versi POCI, effective_from = tanggal_inisiasi)           |
      | amortisasi_harian   | 61643.8356                                                  |
    When AMORTISASI_PD_JOB memproses POCI-0010
    Then akrual menggunakan credit_adjusted_eir = 0.04500000 (bukan gross EIR)
    And jurnal AMORTISASI_PD terposting dengan basis credit-adjusted
    And aud.audit_log mencatat: { is_poci: true, credit_adjusted_eir: 0.04500000, schedule_version: 2 }

  Scenario: S4-AC4 — EIR tidak ditemukan di ecl.amortisasi_schedule → AKRUAL_EIR_NOT_FOUND ke DLQ
    Given mst.instrumen OBL-0505: klasifikasi_psak71 = 'AC', status = 'ACTIVE'
    And ecl.amortisasi_schedule tidak memiliki row aktif (effective_to = 'infinity') untuk OBL-0505
    When AMORTISASI_PD_JOB mencoba proses OBL-0505
    Then sys.dlq INSERT:
      | job_type     | AMORTISASI_PD_JOB         |
      | instrumen_id | OBL-0505                  |
      | error_code   | AKRUAL_EIR_NOT_FOUND      |
      | error_detail | "Tidak ada amortisasi schedule aktif untuk OBL-0505" |
    And OBL-0505 dilewati, instrumen lain dilanjutkan
    And notifikasi ke ROLE-IT-ADMIN dan ROLE-AKUN: "DLQ: 1 instrumen tidak memiliki EIR schedule — OBL-0505"
```

---

## Story P5-M9-S5 — Akrual List + Dashboard Staging

**Actor**: ROLE-AKUN (review harian), ROLE-RISK (oversight Stage 3, stale staging), ROLE-AKUN-CTL (override stale), ROLE-AUDIT (read-only export)
**Trigger**: User membuka `/trx/akrual` untuk review harian per instrumen, atau dashboard per portofolio MTD/YTD. System memeriksai apakah staging aktif di `ecl.staging_history` masih segar (ECL sealed run ≤ X hari lalu) — jika stale → warning `AKRUAL_STAGING_STALE` di UI + antrian ROLE-AKUN-CTL untuk override/confirm.
**Goal**: List akrual dengan sort/paging/filter/export (UX rule §1). Summary MTD/YTD per instrumen + per portofolio. Staging staleness alert. DataTable pattern dengan status badge per Stage.

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.read` (minimum)
2. ROLE-AKUN-CTL diperlukan untuk dismiss/override stale staging alert
3. `sys.parameter 'AKRUAL_STAGING_STALE_DAYS'` = 30 (configurable, default)

### Acceptance Criteria

```gherkin
Feature: Akrual list, summary MTD/YTD, staging staleness alert

  Background:
    Given user ROLE-AKUN (USR-AKUN-001) ter-autentikasi
    And sys.parameter 'AKRUAL_STAGING_STALE_DAYS' = 30

  Scenario: S5-AC1 — GET /trx/akrual: list harian per instrumen dengan sort/filter/paging/export
    Given 1250 rows trx.pendapatan_akrual untuk PRD-2026-06
    When USR-AKUN-001 mengirim GET /api/v1/transaksi/akrual?filter[tanggal_akrual]=2026-06-20&sort=akrual_IDR:desc&limit=50
    Then HTTP 200:
      | data[]              | array trx.pendapatan_akrual hari itu, sorted descending akrual_IDR |
      | pagination.hasMore  | true                                                              |
      | pagination.totalEstimate | 1250                                                         |
      | appliedSort         | [{ col: "akrual_IDR", dir: "desc" }]                             |
    And setiap row mengandung: instrumen_id, kode_instrumen, klasifikasi, stage, basis (GROSS/NET), eir, akrual_IDR, fx_rate_id (jika FCY)
    And filter[stage]=3 mengembalikan hanya instrumen Stage 3 dengan basis = NET_CARRYING
    And export CSV/XLSX tersedia di GET /api/v1/transaksi/akrual/export (audit AKRUAL.EXPORT in-transaction)

  Scenario: S5-AC2 — Summary MTD/YTD per instrumen dan per portofolio
    When USR-AKUN-001 mengirim GET /api/v1/transaksi/akrual/OBL-0101/summary?year=2026&month=6
    Then HTTP 200:
      | data.instrumen_id       | OBL-0101               |
      | data.akrual_mtd_IDR     | Σ akrual Σ 1–20 Jun 2026 |
      | data.akrual_ytd_IDR     | Σ akrual Jan–Jun 2026    |
      | data.stage_saat_ini     | 1                        |
      | data.staging_source     | ecl.calc_result_line run ID |
      | data.ecl_run_sealed_at  | 2026-05-31T23:00:00+07:00 |
    And untuk portofolio: GET /api/v1/transaksi/akrual/summary?portofolio_id=PRT-HTC-01 mengembalikan aggregate per portofolio

  Scenario: S5-AC3 — Staging stale alert: ECL sealed run > 30 hari lalu → AKRUAL_STAGING_STALE di UI
    Given mst.instrumen OBL-0606: stage = 2 (dari ecl.staging_history)
    And ecl.calc_result_line latest sealed untuk OBL-0606: sealed_at = 2026-04-30 (51 hari lalu, > 30 hari)
    When USR-AKUN-001 membuka /trx/akrual dan melihat OBL-0606
    Then baris OBL-0606 menampilkan badge merah: "STAGING STALE — ECL terakhir sealed 51 hari lalu. Akrual Stage 2 mungkin tidak akurat."
    And warning banner di halaman: "3 instrumen memiliki staging stale (> 30 hari). Review diperlukan oleh ROLE-RISK."
    And aud.audit_log.action = STAGING_STALE_ALERT — event (per instrumen per hari, tidak per pageload)
    And antrian task muncul di dashboard ROLE-RISK: "Staging stale: OBL-0606, OBL-0707, OBL-0808 — trigger ECL rerun atau confirm staging."

  Scenario: S5-AC4 — ROLE-AKUN-CTL override stale staging: confirm staging tetap valid
    Given OBL-0606 dalam status AKRUAL_STAGING_STALE
    And ROLE-AKUN-CTL (USR-CTL-001) mereview dan memutuskan staging masih valid (tidak ada perubahan signifikan)
    When USR-CTL-001 mengirim POST /api/v1/transaksi/akrual/OBL-0606/confirm-staging
      With body: { "reason": "Tidak ada perubahan material sejak ECL run terakhir. Staging Stage 2 dikonfirmasi valid per judgement CFO 2026-06-20.", "confirm_until": "2026-07-31" }
      With Idempotency-Key: IK-STAGING-CONFIRM-001
    Then HTTP 200
    And ecl.staging_history INSERT override entry: { instrumen_id, confirmed_by: USR-CTL-001, valid_until: 2026-07-31, reason }
    And badge STAGING STALE untuk OBL-0606 hilang hingga 2026-07-31 atau sealed ECL run baru tersedia
    And aud.audit_log.action = STAGING_STALE_ALERT — override confirmed, in-transaction
```

---

## Ringkasan P5-M9 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M9-S1 | Jatuh Tempo cron 09:00 WIB | System cron, ROLE-AKUN | 4 | **ifrs9-compliance-reviewer BLOCKING** (maturity accounting) · **security-engineer BLOCKING** (audit completeness) |
| P5-M9-S2 | Akrual EIR harian per stage | System cron, ROLE-RISK | 4 | **ifrs9-compliance-reviewer BLOCKING** (Stage 3 §5.4.1(b), POCI) |
| P5-M9-S3 | Dividen + distribusi reksadana | ROLE-MAKER-TR, ROLE-APPR-TR | 4 | **ifrs9-compliance-reviewer BLOCKING** (FVOCI Election dividen treatment) · **security-engineer BLOCKING** (SoD) |
| P5-M9-S4 | Amortisasi premium/diskon EIR | System cron, ROLE-AKUN | 4 | **ifrs9-compliance-reviewer BLOCKING** (POCI credit-adjusted EIR, DEC-013 immutability) |
| P5-M9-S5 | Akrual list + dashboard staging | ROLE-AKUN, ROLE-RISK, ROLE-AKUN-CTL | 4 | advisory (UX rule §1 compliance check) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `MATURITY_INSTRUMEN_NOT_ACTIVE` | 422 (DLQ) | `mst.instrumen.status ≠ 'ACTIVE'` saat cron maturity berjalan | Per instrumen ke DLQ; tidak halt batch |
| `AKRUAL_STAGING_STALE` | 200 (warning in list response) | ECL sealed run untuk instrumen > `AKRUAL_STAGING_STALE_DAYS` hari | Warning di UI badge; akrual tetap diposting; antrian ROLE-RISK |
| `AKRUAL_FX_RATE_MISSING` | 422 (DLQ) | FX rate status `APPROVED` tidak tersedia untuk mata uang + tanggal akrual | Per instrumen ke DLQ; instrumen IDR tidak terdampak |
| `AKRUAL_PERIODE_LOCKED` | 423 (DLQ) | `mst.periode_buku.status_periode = 'CLOSED'` saat cron berjalan | Seluruh batch untuk periode tersebut di-DLQ |
| `AKRUAL_DUPLICATE` | 409 (DLQ) | Unique constraint `(instrumen_id, tanggal_akrual, akrual_type)` violation | Per instrumen ke DLQ; idempotency guard |
| `AKRUAL_EIR_NOT_FOUND` | 422 (DLQ) | Tidak ada `ecl.amortisasi_schedule` aktif untuk instrumen | Per instrumen ke DLQ; notif ROLE-AKUN + ROLE-IT-ADMIN |
| `DIVIDEN_VALIDATION_FAILED` | 422 | `gross_dividen_IDR ≤ 0` atau field wajib kosong | Detail field di `error.details[]` |

Catatan: `SOD_VIOLATION` (HTTP 403), `IDEMPOTENCY_REPLAY` (HTTP 200), `NOT_FOUND` (HTTP 404) sudah ada di api-conventions.md — tidak ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M9 | MFA Level |
|---|---|---|---|
| System cron (Asynq) | Service account | Jalankan MATURITY_PROCESS_JOB, DAILY_ACCRUAL_JOB, AMORTISASI_PD_JOB | N/A |
| ROLE-AKUN | `transaksi.read`, `transaksi.create` (dividen) | Review akrual harian, input dividen | Tidak wajib |
| ROLE-APPR-TR | `transaksi.approve` | Approve dividen (4-eyes SoD ≠ maker) | Wajib jika Treasury Manager (DEC-026) |
| ROLE-RISK | `transaksi.read`, `instrumen.read` | Oversight Stage 3 akrual, terima stale staging alert | Tidak wajib |
| ROLE-AKUN-CTL | `transaksi.read`, `instrumen.confirm_staging` | Override stale staging alert; approve mapping jurnal | WAJIB (DEC-026) |
| ROLE-AUDIT | `transaksi.read`, `audit_log.read` | Read-only seluruh akrual + audit trail + export | Tidak wajib |
| ROLE-IT-ADMIN | `sys.dlq.read` | Monitoring DLQ cron failure; retry job | WAJIB |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.instrumen` ACTIVE + klasifikasi locked | P5-M1 → P5-M9 | Akrual hanya untuk instrumen ACTIVE; FVTPL dikecualikan dari akrual EIR |
| Jurnal engine + event codes AKRUAL_BUNGA, AMORTISASI_PD, DIVIDEN, MATURITY_SETTLEMENT | P5-M2 → P5-M9 | Semua event codes harus di-seed di mapping master sebelum cron berjalan |
| FX rate APPROVED per tanggal akrual | P5-M6 → P5-M9 | Multi-currency instrumen butuh FX rate APPROVED; missing → AKRUAL_FX_RATE_MISSING ke DLQ |
| `ecl.calc_result_line` latest sealed run | Phase 4 ECL → P5-M9 | Stage 3 Net Carrying = Gross − ECL; ECL dari sealed run; stale > 30 hari → AKRUAL_STAGING_STALE |
| `ecl.amortisasi_schedule` aktif | APP-C / P5-M1 → P5-M9 | EIR + schedule amortisasi per instrumen; POCI: credit-adjusted EIR versi khusus |
| `mst.periode_buku.status_periode = 'OPEN'` | P5-M4 → P5-M9 | Akrual tidak bisa dipost ke periode CLOSED |
| `sys.holiday_calendar` | sys → P5-M9 | Skip cron pada hari libur nasional; tabel baru jika belum ada |
| Migration baru | P5-M9 → data-modeler | `trx.jatuh_tempo`, `trx.pendapatan_akrual`, `trx.dividen`; unique constraint akrual; `sys.holiday_calendar`; `sys.dlq` extension (jika belum ada) |

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer (BLOCKING gate — S1, S2, S3, S4)
- [ ] **S2**: Stage 3 akrual menggunakan Net Carrying (Gross − ECL) per PSAK 71 §5.4.1(b) — konfirmasi ECL source harus dari **sealed** calc run (bukan draft)
- [ ] **S2**: Jika tidak ada sealed ECL run → AKRUAL_STAGING_STALE ke DLQ atau block akrual Stage 3? Konfirmasi policy Tugure (konservatif: block; pragmatis: warn + post Gross)
- [ ] **S2**: POCI credit-adjusted EIR: konfirmasi schedule versi POCI di `ecl.amortisasi_schedule` sudah cover semua POCI instrumen yang ada
- [ ] **S3**: FVOCI Election dividen: ke OCI atau P&L? PSAK 71 tidak spesifik untuk dividen di FVOCI Election (berbeda dari disposal). Konfirmasi kebijakan Tugure dengan ROLE-AKUN dan CFO — catat di Decision Log jika belum ada
- [ ] **S3**: PPh dividen 10% (UU PPh §17 ayat 2c) — konfirmasi tarif berlaku per regulasi terbaru (apakah ada perubahan 2024/2025 yang relevan)
- [ ] **S4**: Amortisasi premium/diskon — konfirmasi `NEVER UPDATE existing ecl.amortisasi_schedule rows` (DEC-013) sudah ter-cover di migration + service layer constraint
- [ ] **S4**: POCI amortisasi — credit-adjusted EIR lebih rendah dari gross EIR; carrying amount amortisasi naik (diskon POCI) via credit-adjusted schedule; konfirmasi perlakuan akuntansi benar
- [ ] **S1**: Bond maturity carrying ≠ par (jika amortisasi tidak selesai): realized G/L di P&L — konfirmasi event code MATURITY_SETTLEMENT cover skenario ini

### Untuk security-engineer (BLOCKING — S1–S4 audit completeness)
- [ ] Semua cron events ditulis in-transaction: `MATURITY.DERECOGNIZED`, `AKRUAL.POSTED`, `DIVIDEN.POSTED`, `AMORTISASI.POSTED`, `STAGING_STALE_ALERT`
- [ ] `MATURITY.HOLIDAY_SKIP` sebagai informatif event — tidak wajib in-transaction (cron tidak buka transaksi jika skip)
- [ ] Service account cron Asynq: `actor_user_id` di audit log = system service UUID (bukan null)
- [ ] DLQ entry juga dilindungi: hanya ROLE-IT-ADMIN yang bisa `sys.dlq.read`; soft-delete tidak berlaku untuk DLQ
- [ ] SoD dividen (S3): `maker_id ≠ approver_id` di service layer, bukan hanya DB constraint
- [ ] Idempotency dividen: `Idempotency-Key` header wajib di submit + approve endpoint
- [ ] Rate limit: approve dividen 10 req/menit per user; cron endpoint tidak expose ke user langsung
- [ ] Export akrual (S5): audit `AKRUAL.EXPORT` in-transaction; ROLE-AUDIT read-only; filter aktif dicatat di audit `after_jsonb`
- [ ] `STAGING_STALE_ALERT` tidak tertulis per pageload — hanya per instrumen per hari (de-duplication di service layer)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine `trx.jatuh_tempo.status` + job submit/status/stream endpoints (SSE), ke `ifrs9-compliance-reviewer` untuk review S1 (maturity accounting), S2 (Stage 3 §5.4.1(b) + POCI — BLOCKING), S3 (dividen FVOCI Election treatment — BLOCKING), S4 (POCI credit-adjusted EIR + DEC-013 immutability — BLOCKING), dan ke `security-engineer` untuk audit completeness semua cron events (BLOCKING). `data-modeler` dapat mulai migration `trx.jatuh_tempo` + `trx.pendapatan_akrual` + `sys.holiday_calendar` paralel setelah compliance gate S2 cleared._
