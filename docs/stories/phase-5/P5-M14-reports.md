# P5-M14 — APP-E Reports: 25 REST Endpoints pakai M13 MV Foundation + Export Engine: User Stories

**Story Set ID**: P5-M14
**Modul**: APP-E — Reporting & Dashboard
**Status**: DRAFT — menunggu handoff ke `system-analyst`; `security-engineer` (BLOCKING — audit in-tx, SHA-256 RPT-28, PII); `ifrs9-compliance-reviewer` (BLOCKING — RPT-13..18 ECL/EIR reports)
**Author**: business-analyst
**Tanggal**: 2026-06-23
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §6 (APP-E Reporting), FSD-APP-E (28 laporan catalog)
**Linked BRD**: BRD §4.5 (APP-E laporan), §3 RACI: ROLE-AKUN-CTL (A), ROLE-RISK (R), ROLE-CFO (A), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-007` (LOCKED) — Asynq job queue; async export reuse pattern M13-S4
- `DEC-018` (LOCKED) — audit trail append-only; `EXPORT.GENERATED` in-transaction
- `DEC-021` (LOCKED) — Idempotency-Key wajib di export trigger endpoint (async path)
- `DEC-022` (LOCKED) — cursor-based pagination only; `?cursor=...&limit=50`

**Dependensi**:
- **P5-M13** — export engine, async export >10k rows, MV foundation, `sys.export_log`, `sys.job`, SHA-256 watermark (WAJIB selesai sebelum M14)
- **P5-M3** — `jrnl.gl_delivery` (RPT-22b GL Delivery Status)
- **P5-M4** — `mst.periode_buku` (filter periode, RPT-23 Periode Close Audit)
- **P5-M9** — `trx.akrual`, `trx.jatuh_tempo` (RPT-11 Jatuh Tempo Log, RPT-11 Akrual Harian)
- **P5-M10** — `ecl.poci_delta_log` (RPT-16 POCI Delta History)
- **P5-M12** — `mst.mapping_jurnal_header` + `jrnl.*` (RPT-19/20/21 sudah ada di M12 — bukan scope M14)
- Migration 000050 (P5-M13) harus sudah applied

**Gate**: `security-engineer` **BLOCKING** (audit in-tx semua 25 endpoint, SHA-256 RPT-28, read-replica routing). `ifrs9-compliance-reviewer` **BLOCKING** untuk RPT-13..18 (ECL/EIR — formula tidak dihitung ulang di M14, hanya query; tetap BLOCKING karena menyentuh ECL output dan staging history).

---

## Konteks & Scope P5-M14

P5-M14 mengimplementasi **25 REST endpoint laporan** dari katalog 28 laporan BLIPS.

**Sudah ada (tidak di-scope M14):**
- RPT-19 Mapping Coverage — M12
- RPT-20 Mapping Validation — M12
- RPT-21 Mapping History — M12
- RPT-24 Status Periode — M4

**M14 scope — 25 laporan:**

| Kategori | Slug / ID | Nama Laporan |
|---|---|---|
| **Master Data** | `rpt-01` `rpt-02` `rpt-03` `rpt-04` `rpt-05` | Daftar Instrumen, Daftar Counterparty, Daftar Bank, COA, FX Rate History |
| **Transaksi** | `rpt-06` `rpt-07` `rpt-08` `rpt-09` `rpt-10` `rpt-11` `rpt-12` | Penempatan Log, MTM Daily, Renewal Log, Penjualan Log, Jatuh Tempo Log, Akrual Harian, Dividen Log |
| **ECL/EIR** | `rpt-13` `rpt-14` `rpt-15` `rpt-16` `rpt-17` `rpt-18` | ECL Calc Run Detail, Stage Movement, SICR Trigger Log, POCI Delta History, EIR Amortisasi Schedule, ECL Roll-Forward |
| **Jurnal/GL** | `rpt-22` `rpt-22b` | Jurnal Posting Log, GL Delivery Status |
| **Compliance & Periode** | `rpt-23` `rpt-25` `rpt-26` `rpt-27` `rpt-28` | Periode Close Audit, Audit Log Browser, Workflow Pending Approval, ECL Sensitivity Analysis, Regulator Submission Pack |

**Arsitektur umum M14:**
- Semua query di-route ke read-replica (`MV_DSN`) default; fallback primary dengan warning (M13-S1 pattern).
- Export sync < 10k → M13-S3 engine. Export async ≥ 10k → M13-S4 Asynq + MinIO.
- RBAC per laporan: 28 permission `report.{slug}.read` + `report.{slug}.export`. ROLE-AUDIT wildcard `report.*.read` + `report.*.export`.
- Filter + sort + cursor per UX §1. State di URL (deep-link friendly).
- Hot reports (RPT-08 MTM Daily, RPT-13 ECL Calc Run Detail) butuh composite index — noted; `data-modeler` tambah di migration 000051 jika belum ada dari MV definition.

---

## Story P5-M14-S1 — Master Data Reports (RPT-01..05)

**Actor**: ROLE-AKUN (primary), ROLE-AUDIT (read+export all), ROLE-RISK (read RPT-04 COA)
**Trigger**: `GET /api/v1/reports/{rpt-01|rpt-02|rpt-03|rpt-04|rpt-05}` — list view per master entity dengan sort + filter + cursor. Source: `mst.*` tables (bukan MV — data kecil, query langsung read-replica).
**Goal**: 5 laporan master data tersedia dengan filter/sort/export standar §1.

### Pre-conditions
1. Migration 000050 (M13) applied; read-replica routing aktif
2. `mst.instrumen`, `mst.counterparty`, `mst.bank`, `jrnl.coa_mapping`, `sys.fx_rate` ter-populate
3. Permission `report.rpt-01.read` (dan slug lainnya) granted ke ROLE-AKUN
4. ROLE-AUDIT punya wildcard `report.*.read`

### Acceptance Criteria

```gherkin
Feature: Master Data Reports RPT-01..05 — list + sort + filter + export

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'report.rpt-01.read'
    And read-replica DSN aktif (MV_DSN set)
    And mst.instrumen: 350 rows aktif

  Scenario: S1-AC1 — RPT-01 Daftar Instrumen: sort multi-kolom + filter per §1; cursor pagination
    When USR-AKUN-001 GET /api/v1/reports/rpt-01?sort=created_at:desc&filter[jenis_instrumen]=DEPOSITO&limit=50
    Then HTTP 200:
      | data           | array instrumen dengan field: kode_instrumen, nama, jenis_instrumen, klasifikasi_psak71, stage, ead_idr, tanggal_jatuh_tempo |
      | pagination.hasMore | true (jika > 50)                                                    |
      | appliedFilter  | { jenis_instrumen: "DEPOSITO" }                                         |
      | appliedSort    | [{ col: "created_at", dir: "desc" }]                                    |
    And query di-route ke read-replica DSN (bukan primary)
    And filter + sort parameter di-validate via allowedCols whitelist; jika kolom tidak diizinkan → HTTP 400 REPORT_PARAMS_INVALID

  Scenario: S1-AC2 — RPT-05 FX Rate History: filter date range + export CSV inline
    When USR-AKUN-001 GET /api/v1/reports/rpt-05?filter[tanggal]=between:2026-06-01,2026-06-30&sort=tanggal:desc
    Then HTTP 200: data fx_rate dengan kolom tanggal, kode_valuta, rate, sumber (JISDOR/MANUAL)
    When USR-AKUN-001 GET /api/v1/reports/rpt-05/export?format=csv&filter[tanggal]=between:2026-06-01,2026-06-30
    Then HTTP 200 streaming CSV (< 10k rows → sync export via M13-S3):
      | Content-Disposition | attachment; filename="fx-rate-history-20260623.csv" |
      | UTF-8 BOM           | ada (untuk Excel ID kompatibilitas)                 |
    And aud.audit_log.action = 'EXPORT.GENERATED' — in-transaction
    And sys.export_log INSERT: { report_slug:'rpt-05', format:'csv', file_hash_sha256 }

  Scenario: S1-AC3 — Slug tidak dikenal: REPORT_NOT_FOUND
    When USR-AKUN-001 GET /api/v1/reports/rpt-99
    Then HTTP 404:
      | error.code    | REPORT_NOT_FOUND                               |
      | error.message | "Laporan 'rpt-99' tidak ditemukan."            |

  Scenario: S1-AC4 — ROLE-AUDIT wildcard bypass: akses RPT-04 COA tanpa permission eksplisit
    Given USR-AUDIT-001 ROLE-AUDIT; tidak ada permission 'report.rpt-04.read' eksplisit
    When USR-AUDIT-001 GET /api/v1/reports/rpt-04
    Then HTTP 200: data COA dari jrnl.coa_mapping
    And backend permission check: ROLE-AUDIT matches wildcard 'report.*.read' → authorized
    And ROLE-AKUN-001 tanpa permission 'report.rpt-04.read' → HTTP 403 REPORT_PERMISSION_DENIED
```

---

## Story P5-M14-S2 — Transaksi Reports (RPT-06..12)

**Actor**: ROLE-AKUN (primary), ROLE-APPR-TR (read), ROLE-AUDIT (all)
**Trigger**: `GET /api/v1/reports/{rpt-06..rpt-12}` — log per jenis transaksi; default sort `tanggal desc`; filter wajib: `periode_id`, `instrumen_id`, `status`. RPT-08 MTM Daily butuh composite index `(tanggal_mtm, instrumen_id)` pada sumber `trx.mtm_adjustment` — note untuk data-modeler (migration 000051).
**Goal**: 7 log transaksi tersedia; async export untuk MTM Daily RPT-08 dan Akrual Harian RPT-11 yang bisa > 10k rows.

### Pre-conditions
1. `trx.penempatan`, `trx.mtm_adjustment`, `trx.renewal`, `trx.penjualan_pencairan`, `trx.jatuh_tempo`, `trx.akrual`, `trx.dividen` ter-populate
2. Permission `report.rpt-06.read` (dst) granted ke ROLE-AKUN
3. M13 export engine + async job running
4. Composite index `(tanggal_mtm, instrumen_id, tenant_id)` pada `trx.mtm_adjustment` — verify via data-modeler

### Acceptance Criteria

```gherkin
Feature: Transaksi Reports RPT-06..12 — log per jenis + filter + async export

  Background:
    Given ROLE-AKUN USR-AKUN-001 dengan permission 'report.rpt-08.read', 'report.rpt-08.export'
    And trx.mtm_adjustment: 28.000 rows (melebihi threshold 10k)

  Scenario: S2-AC1 — RPT-08 MTM Daily: filter tanggal + instrumen; async export >10k → 202 + job
    When USR-AKUN-001 GET /api/v1/reports/rpt-08?filter[tanggal]=between:2026-01-01,2026-06-30&sort=tanggal:desc&limit=50
    Then HTTP 200: data 50 rows MTM dengan kolom: tanggal_mtm, instrumen_id, kode_instrumen, nilai_pasar, delta_nilai, sumber_harga
    And pagination.totalEstimate = 28000 (estimasi)
    When USR-AKUN-001 GET /api/v1/reports/rpt-08/export?format=xlsx&filter[tanggal]=between:2026-01-01,2026-06-30
      With Idempotency-Key: IK-EXP-RPT08-001
    Then HTTP 202:
      | data.jobId     | JOB-RPT08-001                                |
      | data.statusUrl | /api/v1/jobs/JOB-RPT08-001                   |
      | data.streamUrl | /api/v1/jobs/JOB-RPT08-001/stream            |
    And Asynq job di-enqueue type='EXPORT', payload: { report_slug:'rpt-08', filters, format:'xlsx' }
    And toast frontend: "Export MTM Daily sedang diproses (28.000 baris). Anda akan diberitahu via email saat selesai."

  Scenario: S2-AC2 — RPT-06 Penempatan Log: filter status + periode; sort tanggal desc default
    When USR-AKUN-001 GET /api/v1/reports/rpt-06?filter[status]=APPROVED&filter[periode_id]=PRD-2026-06
    Then HTTP 200: data penempatan log dengan kolom: tanggal_penempatan, kode_instrumen, nominal_idr, status, maker_id, approver_id
    And default sort: tanggal_penempatan DESC (jika sort param tidak disediakan)
    And ROLE-APPR-TR USR-APPR-001: GET /api/v1/reports/rpt-06 → HTTP 200 (punya permission 'report.rpt-06.read')

  Scenario: S2-AC3 — Date range di luar rentang data: REPORT_PERIODE_INVALID
    When USR-AKUN-001 GET /api/v1/reports/rpt-07?filter[tanggal]=between:2019-01-01,2019-12-31
    Then HTTP 422:
      | error.code    | REPORT_PERIODE_INVALID                                                          |
      | error.message | "Rentang tanggal 2019-01-01 s.d. 2019-12-31 di luar batas data tersedia (min: 2024-01-01)." |

  Scenario: S2-AC4 — Query timeout >30 detik: REPORT_QUERY_TIMEOUT + alert
    Given RPT-11 Akrual Harian query tanpa filter (full table scan) melebihi 30 detik
    When USR-AKUN-001 GET /api/v1/reports/rpt-11 (tanpa filter)
    Then HTTP 422:
      | error.code    | REPORT_QUERY_TIMEOUT                                                              |
      | error.message | "Query melebihi batas 30 detik. Tambahkan filter periode_id atau instrumen_id untuk mempersempit data." |
    And backend: statement_timeout = 30s set di DB session; query dibatalkan
    And Grafana alert: [BLIPS-ALERT] REPORT_QUERY_TIMEOUT rpt-11 actor=USR-AKUN-001
```

---

## Story P5-M14-S3 — ECL/EIR Reports (RPT-13..18)

**Actor**: ROLE-RISK (primary), ROLE-CFO (read + export), ROLE-AUDIT (all)
**Trigger**: `GET /api/v1/reports/{rpt-13..rpt-18}` — query `ecl.*` dan `rpt.mv_*` via read-replica. RPT-14 Stage Movement: stage transition history S1↔S2↔S3 dengan SICR trigger ref. RPT-16 POCI Delta History: consumer `ecl.poci_delta_log` (M10). RPT-17 EIR Amortisasi Schedule: filter per `instrumen_id` + `schedule_version`. RPT-18 ECL Roll-Forward: opening + transfers + originations − derecognitions ± remeasurements = closing; wajib reconcile ke laporan posisi.
**Goal**: 6 laporan ECL/EIR tersedia untuk review ROLE-RISK + CFO. Formula tidak dihitung ulang (query hasil kalkulasi yang sudah ada). Compliance gate: `ifrs9-compliance-reviewer` BLOCKING.

### Pre-conditions
1. `ecl.ecl_calc_result_line`, `ecl.staging_history`, `ecl.sicr_trigger_log`, `ecl.poci_delta_log`, `ecl.amortisasi_schedule`, `ecl.ecl_roll_forward` ter-populate (M5 M6 M10)
2. Permission `report.rpt-13.read` granted ke ROLE-RISK
3. M13 MV `rpt.mv_poci_delta_summary` ter-refresh
4. `ifrs9-compliance-reviewer` harus review + approve story set ini sebelum backend merge

### Acceptance Criteria

```gherkin
Feature: ECL/EIR Reports RPT-13..18 — ECL calc detail + staging + POCI + amortisasi + roll-forward

  Background:
    Given ROLE-RISK USR-RISK-001 dengan permission 'report.rpt-13.read', 'report.rpt-18.read'
    And ecl.ecl_calc_result_line: 12.000 rows untuk calc_run_id CR-2026-06

  Scenario: S3-AC1 — RPT-13 ECL Calc Run Detail: filter calc_run_id + instrumen; export async >10k
    When USR-RISK-001 GET /api/v1/reports/rpt-13?filter[calc_run_id]=CR-2026-06&sort=ead_idr:desc&limit=50
    Then HTTP 200: 50 rows dengan kolom: instrumen_id, kode_instrumen, stage, ead_idr, pd_good, pd_normal, pd_bad, lgd, ecl_good, ecl_normal, ecl_bad, ecl_weighted, fl_multiplier_good, fl_multiplier_normal, fl_multiplier_bad
    And query via read-replica DSN
    When USR-RISK-001 GET /api/v1/reports/rpt-13/export?format=xlsx&filter[calc_run_id]=CR-2026-06
      With Idempotency-Key: IK-EXP-RPT13-001
    Then HTTP 202: { jobId, statusUrl, streamUrl } (12.000 > threshold 10k → async M13-S4)
    And saat selesai: SMTP email ke USR-RISK-001 dengan subject "[BLIPS] Export ECL Calc Run Detail CR-2026-06 siap diunduh"

  Scenario: S3-AC2 — RPT-14 Stage Movement: tampilkan S1↔S2↔S3 transitions + SICR trigger ref
    When USR-RISK-001 GET /api/v1/reports/rpt-14?filter[periode_id]=PRD-2026-06
    Then HTTP 200: data stage transitions dengan kolom: instrumen_id, stage_sebelum, stage_sesudah, tanggal_transisi, sicr_trigger (rating_downgrade|ig_to_nonig|dpd_30), rating_sebelum, rating_sesudah, dpd, cure_flag
    And instrumen yang cure (Stage 2 → Stage 1) tampil dengan cure_flag=true + history downgrade tetap visible
    And kolom sicr_trigger merujuk ke ecl.sicr_trigger_log.trigger_type (DEC-011)

  Scenario: S3-AC3 — RPT-18 ECL Roll-Forward: reconcile formula wajib tersedia per periode
    When USR-RISK-001 GET /api/v1/reports/rpt-18?filter[periode_id]=PRD-2026-06
    Then HTTP 200: 1 row summary dengan kolom:
      | ecl_opening         | saldo ECL awal periode                        |
      | transfers_in        | masuk ke stage lebih tinggi (S1→S2, S2→S3)   |
      | transfers_out       | keluar ke stage lebih rendah (S2→S1, S3→S2)  |
      | new_originations    | instrumen baru periode ini                    |
      | derecognitions      | instrumen yang jatuh tempo / dijual           |
      | remeasurements      | perubahan ECL dalam stage yang sama           |
      | ecl_closing         | = opening + transfers_in - transfers_out + new_originations - derecognitions ± remeasurements |
      | reconcile_diff      | selisih vs ecl_calc_result_line (wajib = 0.0000 atau explain) |
    And jika reconcile_diff ≠ 0 → HTTP 200 tetapi response field reconcile_flag = 'UNRECONCILED' + penjelasan
    And aud.audit_log.action = 'REPORT.RPT18_VIEWED' — in-transaction (compliance evidence)

  Scenario: S3-AC4 — RPT-17 EIR Amortisasi Schedule: filter instrumen + schedule_version; immutable
    When USR-RISK-001 GET /api/v1/reports/rpt-17?filter[instrumen_id]=INST-001234&filter[schedule_version]=2
    Then HTTP 200: data amortisasi schedule version 2 dengan kolom: periode, saldo_awal, bunga_eir, pokok, saldo_akhir, effective_from, effective_to
    And baris tidak bisa di-edit via laporan (read-only per DEC-018 immutability)
    And RPT-17 query: ecl.amortisasi_schedule WHERE instrumen_id = $1 AND schedule_version = $2 AND deleted_at IS NULL
    And ROLE-AKUN-001 tanpa permission 'report.rpt-17.read' → HTTP 403 REPORT_PERMISSION_DENIED
```

---

## Story P5-M14-S4 — Jurnal/GL Reports (RPT-22 + RPT-22b)

**Actor**: ROLE-AKUN (primary), ROLE-AKUN-CTL (approve + export), ROLE-AUDIT (all)
**Trigger**: `GET /api/v1/reports/rpt-22` (Jurnal Posting Log — `jrnl.jurnal_header` join `jrnl.jurnal_detail`) dan `GET /api/v1/reports/rpt-22b` (GL Delivery Status — `rpt.mv_gl_delivery_status` dari M13 + `jrnl.gl_delivery` dari M3). Export CSV/XLSX/PDF; signed SHA-256 per file. RPT-22 audit per posting event; RPT-22b shows delivery status tiered (SENT/DELIVERED/FAILED/PENDING).
**Goal**: 2 laporan jurnal/GL tersedia; export signed; link ke M3 GL delivery data.

### Pre-conditions
1. `jrnl.jurnal_header`, `jrnl.jurnal_detail`, `jrnl.gl_delivery` ter-populate (M2, M3, M12)
2. `rpt.mv_gl_delivery_status` ter-refresh (M13)
3. Permission `report.rpt-22.read` granted ke ROLE-AKUN; `report.rpt-22b.read` ke ROLE-AKUN-CTL
4. Export engine M13-S3 running

### Acceptance Criteria

```gherkin
Feature: Jurnal/GL Reports RPT-22 + RPT-22b — posting log + GL delivery

  Background:
    Given ROLE-AKUN USR-AKUN-001 dengan permission 'report.rpt-22.read', 'report.rpt-22.export'
    And jrnl.jurnal_header: 3.200 rows untuk periode PRD-2026-06

  Scenario: S4-AC1 — RPT-22 Jurnal Posting Log: filter event_code + periode; export XLSX signed
    When USR-AKUN-001 GET /api/v1/reports/rpt-22?filter[periode_id]=PRD-2026-06&filter[event_code]=ECL_IMPAIRMENT&sort=posted_at:desc
    Then HTTP 200: data jurnal posting dengan kolom: jurnal_id, event_code, instrumen_id, debit_account, kredit_account, nominal_idr, posted_at, posted_by, status_posting
    When USR-AKUN-001 GET /api/v1/reports/rpt-22/export?format=xlsx&filter[periode_id]=PRD-2026-06
    Then HTTP 200 streaming XLSX (3.200 < 10k → sync M13-S3):
      | Sheet "Jurnal Posting" | 3.200 rows + header bold + freeze row 1           |
      | Footer watermark       | "RAHASIA - BLIPS Tugu Re — exported {ts} by {user}" |
    And sys.export_log INSERT: { report_slug:'rpt-22', file_hash_sha256 }
    And aud.audit_log.action = 'EXPORT.GENERATED' — in-transaction

  Scenario: S4-AC2 — RPT-22b GL Delivery Status: dari MV; filter status delivery; ROLE-AKUN-CTL only
    Given ROLE-AKUN-CTL USR-CTL-001 dengan permission 'report.rpt-22b.read'
    When USR-CTL-001 GET /api/v1/reports/rpt-22b?filter[status_delivery]=FAILED&filter[periode_id]=PRD-2026-06
    Then HTTP 200: data GL delivery dari rpt.mv_gl_delivery_status dengan kolom: delivery_id, jurnal_id, gl_system, status_delivery, delivery_attempt, last_attempt_at, error_detail
    And data via read-replica MV (tidak query jrnl.gl_delivery langsung untuk list)
    And USR-AKUN-001 tanpa permission 'report.rpt-22b.read' → HTTP 403 REPORT_PERMISSION_DENIED

  Scenario: S4-AC3 — Export PDF RPT-22: SHA-256 tercantum di halaman terakhir (compliance evidence)
    When USR-CTL-001 GET /api/v1/reports/rpt-22/export?format=pdf&filter[periode_id]=PRD-2026-06
    Then HTTP 200 streaming PDF:
      | Setiap halaman | footer watermark: "RAHASIA - BLIPS Tugu Re — {ts} by {user}" |
      | Halaman terakhir | "SHA-256: {hex hash file}" — untuk verifikasi integritas     |
    And sys.export_log INSERT: { format:'pdf', file_hash_sha256 }

  Scenario: S4-AC4 — RPT-22 query timeout (full scan tanpa filter): REPORT_QUERY_TIMEOUT
    When USR-AKUN-001 GET /api/v1/reports/rpt-22 (tanpa filter apapun)
    Then HTTP 422:
      | error.code    | REPORT_QUERY_TIMEOUT                                                                      |
      | error.message | "Query melebihi 30 detik. Gunakan filter periode_id atau event_code untuk mempersempit." |
    And statement_timeout = 30s enforced di DB session
```

---

## Story P5-M14-S5 — Compliance & Periode Reports (RPT-23, RPT-25..28)

**Actor**: ROLE-AUDIT (primary RPT-23/25/26), ROLE-RISK (RPT-27), ROLE-CFO (RPT-28 export), ROLE-AKUN-CTL (RPT-23/26)
**Trigger**: 5 compliance reports. RPT-23 Periode Close Audit: audit trail hard-close event per periode (dari `aud.audit_log` filter action `PERIODE.HARD_CLOSE`). RPT-25 Audit Log Browser: browser seluruh `aud.audit_log`; ROLE-AUDIT only. RPT-26 Workflow Pending Approval: instrumen/transaksi/klasifikasi yang menunggu approval (semua modul). RPT-27 ECL Sensitivity Analysis: what-if simulasi bobot skenario Good/Normal/Bad vs ECL weighted (query `ecl.ecl_calc_result_line` dengan bobot override — tidak recompute formula, hanya multiply). RPT-28 Regulator Submission Pack: consolidated XLSX semua jurnal + ECL + EIR dalam periode; ROLE-CFO export only; SHA-256 attestation; `aud.audit_log` entry wajib.
**Goal**: 5 compliance reports tersedia; RPT-28 export eksplisit dengan SHA-256 + audit; RPT-25 ROLE-AUDIT only.

### Pre-conditions
1. `aud.audit_log` ter-populate (immutable, DEC-018)
2. Permission `report.rpt-23.read` ke ROLE-AUDIT + ROLE-AKUN-CTL; `report.rpt-25.read` ke ROLE-AUDIT only; `report.rpt-28.export` ke ROLE-CFO only
3. ROLE-CFO MFA wajib (DEC-026) — step-up untuk RPT-28 export (di-treat seperti action sensitif)
4. M13 export engine running; async export M13-S4 untuk RPT-28 (bisa > 10k)

### Acceptance Criteria

```gherkin
Feature: Compliance & Periode Reports RPT-23/25/26/27/28 — audit + workflow + sensitivity + regulator pack

  Background:
    Given ROLE-AUDIT USR-AUDIT-001 ter-autentikasi dengan wildcard 'report.*.read'
    And aud.audit_log: 85.000 rows total; 12 rows action='PERIODE.HARD_CLOSE'

  Scenario: S5-AC1 — RPT-25 Audit Log Browser: ROLE-AUDIT only; sort + filter + cursor; export async >10k
    When USR-AUDIT-001 GET /api/v1/reports/rpt-25?filter[action]=INSTRUMEN.CREATE&filter[periode]=PRD-2026-06&sort=event_time:desc&limit=50
    Then HTTP 200: 50 rows audit_log dengan kolom: event_id, event_time, actor_user_id, actor_role, action, entity_type, entity_id, ip, trace_id
    And before_jsonb + after_jsonb TIDAK di-return ke non-AUDIT role (check in middleware)
    And ROLE-RISK USR-RISK-001 GET /api/v1/reports/rpt-25 → HTTP 403 REPORT_PERMISSION_DENIED (bukan wildcard)
    When USR-AUDIT-001 GET /api/v1/reports/rpt-25/export?format=csv (85.000 > 10k)
    Then HTTP 202: { jobId } (async M13-S4)

  Scenario: S5-AC2 — RPT-23 Periode Close Audit: tampilkan hard-close events per periode
    When USR-AUDIT-001 GET /api/v1/reports/rpt-23?filter[periode_id]=PRD-2026-06
    Then HTTP 200: data close audit dengan kolom: event_time, actor_user_id, actor_role, action (PERIODE.HARD_CLOSE), mfa_method, ip, trace_id
    And data: hanya 12 rows (filter action='PERIODE.HARD_CLOSE' + periode_id = PRD-2026-06 dari aud.audit_log)
    And ROLE-AKUN-CTL USR-CTL-001: GET /api/v1/reports/rpt-23 → HTTP 200 (punya permission 'report.rpt-23.read')

  Scenario: S5-AC3 — RPT-27 ECL Sensitivity: what-if bobot skenario; tidak recompute formula
    Given ROLE-RISK USR-RISK-001 dengan permission 'report.rpt-27.read'
    When USR-RISK-001 GET /api/v1/reports/rpt-27?filter[calc_run_id]=CR-2026-06&w_good=0.20&w_normal=0.60&w_bad=0.20
    Then HTTP 200: data sensitivity dengan kolom: instrumen_id, ecl_weighted_default, ecl_weighted_sensitivity, delta_ecl, delta_pct
    And ecl_weighted_sensitivity = ecl_fl_good * 0.20 + ecl_fl_normal * 0.60 + ecl_fl_bad * 0.20 (query-time multiply dari ecl_calc_result_line)
    And validasi: w_good + w_normal + w_bad wajib = 1.0; jika tidak → HTTP 400 REPORT_PARAMS_INVALID: "Bobot skenario harus berjumlah 1.0 (diterima: 1.00000001 toleransi 1e-6)"
    And aud.audit_log.action = 'REPORT.RPT27_SENSITIVITY_RUN' — in-transaction dengan after_jsonb: { bobot, calc_run_id }

  Scenario: S5-AC4 — RPT-28 Regulator Submission Pack: ROLE-CFO only; MFA step-up; SHA-256 attestation
    Given ROLE-CFO USR-CFO-001 ter-autentikasi; mfa_verified=true; permission 'report.rpt-28.export'
    When USR-CFO-001 POST /api/v1/reports/rpt-28/export
      With header X-Step-Up-Token: {valid_stepup_token} (MFA step-up per DEC-027)
      With Idempotency-Key: IK-RPT28-001
      With body: { periode_id: "PRD-2026-06", format: "xlsx" }
    Then HTTP 202: { jobId: JOB-RPT28-001, statusUrl, streamUrl }
    When Asynq worker selesai: consolidated XLSX berisi sheets: Jurnal, ECL Summary, EIR Schedule, GL Delivery
    Then MinIO object: exports/TUGURE/USR-CFO-001/2026/06/23/JOB-RPT28-001.xlsx
    And sys.export_log INSERT: { report_slug:'rpt-28', file_hash_sha256, mfa_method:'TOTP', periode_id:'PRD-2026-06' }
    And aud.audit_log.action = 'EXPORT.REGULATOR_PACK_GENERATED' — in-transaction
      With after_jsonb: { periode_id, file_hash_sha256, sheet_count:4, row_count_total, actor:USR-CFO-001, mfa_method }
    And SMTP email ke USR-CFO-001: "[BLIPS] Regulator Submission Pack PRD-2026-06 siap. SHA-256: {hash}."
    And ROLE-RISK USR-RISK-001 POST /api/v1/reports/rpt-28/export → HTTP 403 REPORT_PERMISSION_DENIED
    And USR-CFO-001 tanpa X-Step-Up-Token → HTTP 403 FORBIDDEN: "Step-up MFA wajib untuk RPT-28 export."
```

---

## Ringkasan P5-M14 Story Set

| Story | Judul | Actor Utama | RPT | AC Count | Gate |
|---|---|---|---|---|---|
| P5-M14-S1 | Master Data Reports | ROLE-AKUN, ROLE-AUDIT | RPT-01..05 | 4 | security BLOCKING |
| P5-M14-S2 | Transaksi Reports | ROLE-AKUN, ROLE-APPR-TR | RPT-06..12 | 4 | security BLOCKING |
| P5-M14-S3 | ECL/EIR Reports | ROLE-RISK, ROLE-CFO | RPT-13..18 | 4 | security BLOCKING + **compliance BLOCKING** |
| P5-M14-S4 | Jurnal/GL Reports | ROLE-AKUN, ROLE-AKUN-CTL | RPT-22 + RPT-22b | 4 | security BLOCKING |
| P5-M14-S5 | Compliance & Periode Reports | ROLE-AUDIT, ROLE-RISK, ROLE-CFO | RPT-23/25/26/27/28 | 4 | security BLOCKING + **compliance BLOCKING** (RPT-27/28) |
| **Total** | | | **25 laporan** | **20** | |

**Tidak di-scope M14 (sudah ada):** RPT-19 (M12), RPT-20 (M12), RPT-21 (M12), RPT-24 (M4).

---

## 28 RPT Slug Master List — Status

| ID | Slug | Nama | Modul | Status |
|---|---|---|---|---|
| RPT-01 | `rpt-01` | Daftar Instrumen | M14-S1 | Baru |
| RPT-02 | `rpt-02` | Daftar Counterparty | M14-S1 | Baru |
| RPT-03 | `rpt-03` | Daftar Bank | M14-S1 | Baru |
| RPT-04 | `rpt-04` | COA | M14-S1 | Baru |
| RPT-05 | `rpt-05` | FX Rate History | M14-S1 | Baru |
| RPT-06 | `rpt-06` | Penempatan Log | M14-S2 | Baru |
| RPT-07 | `rpt-07` | MTM Daily | M14-S2 | Baru |
| RPT-08 | `rpt-08` | Renewal Log | M14-S2 | Baru |
| RPT-09 | `rpt-09` | Penjualan Log | M14-S2 | Baru |
| RPT-10 | `rpt-10` | Jatuh Tempo Log | M14-S2 | Baru |
| RPT-11 | `rpt-11` | Akrual Harian | M14-S2 | Baru |
| RPT-12 | `rpt-12` | Dividen Log | M14-S2 | Baru |
| RPT-13 | `rpt-13` | ECL Calc Run Detail | M14-S3 | Baru |
| RPT-14 | `rpt-14` | Stage Movement | M14-S3 | Baru |
| RPT-15 | `rpt-15` | SICR Trigger Log | M14-S3 | Baru |
| RPT-16 | `rpt-16` | POCI Delta History | M14-S3 | Baru |
| RPT-17 | `rpt-17` | EIR Amortisasi Schedule | M14-S3 | Baru |
| RPT-18 | `rpt-18` | ECL Roll-Forward | M14-S3 | Baru |
| RPT-19 | `rpt-19` | Mapping Coverage | **M12** | Sudah ada |
| RPT-20 | `rpt-20` | Mapping Validation | **M12** | Sudah ada |
| RPT-21 | `rpt-21` | Mapping History | **M12** | Sudah ada |
| RPT-22 | `rpt-22` | Jurnal Posting Log | M14-S4 | Baru |
| RPT-22b | `rpt-22b` | GL Delivery Status | M14-S4 | Baru |
| RPT-23 | `rpt-23` | Periode Close Audit | M14-S5 | Baru |
| RPT-24 | `rpt-24` | Status Periode | **M4** | Sudah ada |
| RPT-25 | `rpt-25` | Audit Log Browser | M14-S5 | Baru |
| RPT-26 | `rpt-26` | Workflow Pending Approval | M14-S5 | Baru |
| RPT-27 | `rpt-27` | ECL Sensitivity Analysis | M14-S5 | Baru |
| RPT-28 | `rpt-28` | Regulator Submission Pack | M14-S5 | Baru |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger |
|---|---|---|
| `REPORT_NOT_FOUND` | 404 | Slug tidak dikenal di registry |
| `REPORT_PERMISSION_DENIED` | 403 | User tidak punya `report.{slug}.read` atau `report.{slug}.export`; bukan ROLE-AUDIT |
| `REPORT_PARAMS_INVALID` | 400 | Kolom sort/filter tidak di allowedCols; bobot sensitivity ≠ 1.0; format tidak dikenal |
| `REPORT_PERIODE_INVALID` | 422 | Date range di luar batas data tersedia |
| `REPORT_QUERY_TIMEOUT` | 422 | Query > 30 detik; statement_timeout enforced di DB session |

Catatan: `EXPORT_TOO_LARGE`, `EXPORT_PERMISSION_DENIED`, `EXPORT_FORMAT_UNSUPPORTED` — reuse dari M13; `FORBIDDEN` (403) untuk step-up MFA missing (RPT-28).

---

## Audit Events Summary

| Event | Trigger | In-transaction |
|---|---|---|
| `EXPORT.GENERATED` | Export sync atau async job selesai (semua 25 laporan) | Ya |
| `EXPORT.DOWNLOADED` | User klik signed URL (async path) | Ya |
| `EXPORT.REGULATOR_PACK_GENERATED` | RPT-28 export selesai (S5-AC4) | Ya — khusus event nama |
| `REPORT.RPT18_VIEWED` | GET RPT-18 ECL Roll-Forward (compliance evidence) | Ya |
| `REPORT.RPT27_SENSITIVITY_RUN` | GET RPT-27 dengan bobot override (S5-AC3) | Ya |

---

## Persona Summary

| Actor | Report access | Export | MFA |
|---|---|---|---|
| ROLE-AUDIT | Semua 28 (wildcard) | Semua format | Tidak wajib |
| ROLE-RISK | RPT-13..18, RPT-23, RPT-26, RPT-27 | Ya (async jika >10k) | Tidak wajib |
| ROLE-CFO | Semua read; RPT-28 export exclusive | RPT-28 XLSX (step-up MFA) | WAJIB (DEC-026) |
| ROLE-AKUN | RPT-01..12, RPT-22 | Ya | Tidak wajib |
| ROLE-AKUN-CTL | RPT-22b, RPT-23, RPT-26 | Ya | WAJIB (DEC-026) |
| ROLE-APPR-TR | RPT-06..12 read | Tidak | Tidak wajib |
| ROLE-IT-ADMIN | Tidak ada akses report domain | Tidak | WAJIB |

---

## Migration 000051 — Objek Baru (opsional, jika belum dari M13)

| Objek | Tipe | Keterangan |
|---|---|---|
| `idx_mtm_adj_tanggal_instrumen` | INDEX `(tanggal_mtm, instrumen_id, tenant_id)` pada `trx.mtm_adjustment` | Hot index untuk RPT-08 MTM Daily |
| `idx_ecl_result_calc_run` | INDEX `(calc_run_id, instrumen_id)` pada `ecl.ecl_calc_result_line` | Hot index untuk RPT-13 ECL Calc Run Detail |
| `idx_audit_log_action_time` | INDEX `(action, event_time DESC, tenant_id)` pada `aud.audit_log` | RPT-23 Periode Close Audit + RPT-25 Browser |

Verifikasi dengan `data-modeler`: jika index sudah ada dari migration sebelumnya, skip 000051.

---

## Handoff Berikutnya

- `system-analyst` → OpenAPI: 25 endpoint `GET /api/v1/reports/{slug}` + 25 export `GET /api/v1/reports/{slug}/export` + 1 `POST /api/v1/reports/rpt-28/export`; 5 error codes baru; filter allowedCols per slug (tabel lengkap); RPT-28 POST request body schema; step-up MFA middleware spec
- `data-modeler` → migration 000051: 3 composite index (MTM, ECL calc, audit_log); verify tidak duplicate dari M13/sebelumnya
- `security-engineer` → **BLOCKING**: audit `EXPORT.GENERATED` in-transaction semua 25 endpoint; RPT-25 before/after redact untuk non-AUDIT; RPT-28 step-up MFA enforce di middleware; SHA-256 RPT-28 attestation field di `sys.export_log`; ROLE-AUDIT wildcard bypass documented per permission model
- `ifrs9-compliance-reviewer` → **BLOCKING** untuk S3 (RPT-13..18) + S5 (RPT-27/28): verifikasi RPT-18 roll-forward formula reconcile; RPT-27 sensitivity what-if tidak salah interpret sebagai recompute; RPT-13 menampilkan FL multiplier per skenario sesuai DEC-010
- `devops-engineer` → statement_timeout 30s di DB session config per-report (bukan global); Grafana alert REPORT_QUERY_TIMEOUT; pastikan read-replica sudah punya semua index dari 000051

_Story set siap dihandoff ke `system-analyst` (OpenAPI 25 endpoint) dan `data-modeler` (migration 000051) secara paralel. `security-engineer` BLOCKING sebelum backend merge. `ifrs9-compliance-reviewer` BLOCKING untuk S3 + S5 (ECL/EIR reports). M13 wajib selesai sebelum M14 dimulai._
