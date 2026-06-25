# P5-M14 State Machine — Report Execution Flow

## Report Execution States

```
ParamsParsed → QueryBuilt → CountChecked → InlineFetched → Delivered
                                        ↘ AsyncEnqueued → [JobRunning] → Delivered
                                                                       ↘ Failed
```

### State descriptions

| State | Description |
|---|---|
| `ParamsParsed` | slug validated vs Registry; permission check; sort/filter validated vs allowedCols |
| `QueryBuilt` | SQL WHERE + ORDER BY constructed via `listquery.ToSQL`; statement_timeout=30s set |
| `CountChecked` | `SELECT COUNT(*)` estimate; if export: compare to inline threshold (10k) |
| `InlineFetched` | rows streamed from read-replica; file built by exporter; HTTP 200 |
| `AsyncEnqueued` | Asynq task enqueued; sys.export_log row inserted; HTTP 202 |
| `JobRunning` | worker processes rows in chunks; progress reported to sys.job + Redis |
| `Delivered` | file in MinIO; signed URL; SMTP email; audit EXPORT.GENERATED committed |
| `Failed` | worker failed; sys.export_log status=FAILED; SMTP error notification |

### Transitions and guards

| From | To | Guard |
|---|---|---|
| `ParamsParsed` | `QueryBuilt` | slug in Registry AND permission OK |
| `ParamsParsed` | **error** REPORT_NOT_FOUND | slug not in Registry |
| `ParamsParsed` | **error** REPORT_PERMISSION_DENIED | permission check fails |
| `ParamsParsed` | **error** REPORT_PARAMS_INVALID | sort/filter col not in allowedCols |
| `QueryBuilt` | `CountChecked` | SQL built without error |
| `QueryBuilt` | **error** REPORT_QUERY_TIMEOUT | statement_timeout exceeded |
| `QueryBuilt` | **error** REPORT_PERIODE_INVALID | date range outside data bounds |
| `CountChecked` | `InlineFetched` | rowCount ≤ inlineThreshold (10k) |
| `CountChecked` | `AsyncEnqueued` | rowCount > inlineThreshold |
| `AsyncEnqueued` | `JobRunning` | Asynq worker picks task |
| `JobRunning` | `Delivered` | file built + uploaded to MinIO + audit committed |
| `JobRunning` | `Failed` | any error; retry once (MaxRetry=1) |

### Special: RPT-28 Regulator Pack

```
POST /reports/rpt-28/export
  → StepUpVerified → PermissionChecked → PayloadValidated
  → RPT28JobEnqueued → [SheetAssembly] → SHA256Attested
  → MinIOUpload → AuditCommitted → SMTPNotified → Delivered
```

Guards:
- `X-Step-Up-Token` present AND scope=`regulator_pack` AND age ≤ 5min → StepUpVerified
- `report.rpt-28.export` in claims.permissions → PermissionChecked
- `periode_id` + `format=xlsx` present → PayloadValidated

### Special: RPT-18 ECL Roll-Forward and RPT-27 Sensitivity

RPT-18: `reconcile_diff` computed at query time:
- `reconcile_diff = ecl_closing - Σ(ecl_calc_result_line.ecl_weighted)` for the periode
- If `reconcile_diff ≠ 0` → response field `reconcile_flag = 'UNRECONCILED'`
- Audit `REPORT.RPT18_VIEWED` written in-tx regardless of reconcile status

RPT-27: weights validated at ParamsParsed:
- `|w_good + w_normal + w_bad - 1.0| ≤ 1e-6` required → else REPORT_PARAMS_INVALID
- Audit `REPORT.RPT27_SENSITIVITY_RUN` written in-tx with `{bobot, calc_run_id}` in after_jsonb

---

## Per-Report Metadata Table (28 rows)

| ID | Slug | Name | Category | Source MV/Table | Required Permission | Async Override | Regulated Flag | Default Sort |
|---|---|---|---|---|---|---|---|---|
| RPT-01 | `rpt-01` | Daftar Instrumen | master | `mst.instrumen` | `report.rpt-01.read` | — | false | `created_at:desc` |
| RPT-02 | `rpt-02` | Daftar Counterparty | master | `mst.counterparty` | `report.rpt-02.read` | — | false | `created_at:desc` |
| RPT-03 | `rpt-03` | Daftar Bank | master | `mst.bank` | `report.rpt-03.read` | — | false | `nama_bank:asc` |
| RPT-04 | `rpt-04` | COA | master | `jrnl.coa_mapping` | `report.rpt-04.read` | — | false | `kode_akun:asc` |
| RPT-05 | `rpt-05` | FX Rate History | master | `sys.fx_rate` | `report.rpt-05.read` | — | false | `tanggal:desc` |
| RPT-06 | `rpt-06` | Penempatan Log | transaksi | `trx.penempatan` | `report.rpt-06.read` | — | false | `tanggal_penempatan:desc` |
| RPT-07 | `rpt-07` | MTM Daily | transaksi | `trx.mtm_adjustment` | `report.rpt-07.read` | — | false | `tanggal_mtm:desc` |
| RPT-08 | `rpt-08` | Renewal Log | transaksi | `trx.renewal` | `report.rpt-08.read` | — | false | `tanggal_renewal:desc` |
| RPT-09 | `rpt-09` | Penjualan Log | transaksi | `trx.penjualan_pencairan` | `report.rpt-09.read` | — | false | `tanggal_jual:desc` |
| RPT-10 | `rpt-10` | Jatuh Tempo Log | transaksi | `trx.jatuh_tempo` | `report.rpt-10.read` | — | false | `tanggal_jatuh_tempo:desc` |
| RPT-11 | `rpt-11` | Akrual Harian | transaksi | `trx.akrual` | `report.rpt-11.read` | — | false | `tanggal_akrual:desc` |
| RPT-12 | `rpt-12` | Dividen Log | transaksi | `trx.dividen` | `report.rpt-12.read` | — | false | `tanggal_dividen:desc` |
| RPT-13 | `rpt-13` | ECL Calc Run Detail | ecl_eir | `ecl.ecl_calc_result_line` | `report.rpt-13.read` | — | false | `ead_idr:desc` |
| RPT-14 | `rpt-14` | Stage Movement | ecl_eir | `ecl.staging_history` | `report.rpt-14.read` | — | false | `tanggal_transisi:desc` |
| RPT-15 | `rpt-15` | SICR Trigger Log | ecl_eir | `ecl.sicr_trigger_log` | `report.rpt-15.read` | — | false | `triggered_at:desc` |
| RPT-16 | `rpt-16` | POCI Delta History | ecl_eir | `rpt.mv_poci_delta_summary` | `report.rpt-16.read` | — | false | `created_at:desc` |
| RPT-17 | `rpt-17` | EIR Amortisasi Schedule | ecl_eir | `ecl.amortisasi_schedule` | `report.rpt-17.read` | — | false | `periode:asc` |
| RPT-18 | `rpt-18` | ECL Roll-Forward | ecl_eir | `ecl.ecl_roll_forward` | `report.rpt-18.read` | — | **true** | `periode_id:asc` |
| RPT-19 | — | Mapping Coverage | — | M12 (out of scope) | — | — | — | — |
| RPT-20 | — | Mapping Validation | — | M12 (out of scope) | — | — | — | — |
| RPT-21 | — | Mapping History | — | M12 (out of scope) | — | — | — | — |
| RPT-22 | `rpt-22` | Jurnal Posting Log | jurnal_gl | `jrnl.jurnal_header` | `report.rpt-22.read` | — | false | `posted_at:desc` |
| RPT-22b | `rpt-22b` | GL Delivery Status | jurnal_gl | `rpt.mv_gl_delivery_status` | `report.rpt-22b.read` | — | false | `last_attempt_at:desc` |
| RPT-23 | `rpt-23` | Periode Close Audit | compliance | `aud.audit_log` | `report.rpt-23.read` | — | false | `event_time:desc` |
| RPT-24 | — | Status Periode | — | M4 (out of scope) | — | — | — | — |
| RPT-25 | `rpt-25` | Audit Log Browser | compliance | `aud.audit_log` | `report.rpt-25.read` | — | false | `event_time:desc` |
| RPT-26 | `rpt-26` | Workflow Pending Approval | compliance | multi-table union | `report.rpt-26.read` | — | false | `submitted_at:desc` |
| RPT-27 | `rpt-27` | ECL Sensitivity Analysis | compliance | `ecl.ecl_calc_result_line` | `report.rpt-27.read` | — | **true** | `ead_idr:desc` |
| RPT-28 | `rpt-28` | Regulator Submission Pack | compliance | composite | `report.rpt-28.export` | always-async | **true** | — |

Notes:
- Regulated flag = true → audit `REPORT.{SLUG}_VIEWED` written in-tx on every list call
- RPT-25 before_jsonb/after_jsonb only returned to ROLE-AUDIT (claims.HasPermission("audit_log.read"))
- RPT-28 always async regardless of row count
- ROLE-AUDIT has wildcard `report.*.read` + `report.*.export` via permission check bypass

---

## Hand-off

- `data-modeler` → migration 000051: composite indexes on `trx.mtm_adjustment(tanggal_mtm, instrumen_id, tenant_id)`, `ecl.ecl_calc_result_line(calc_run_id, instrumen_id)`, `aud.audit_log(action, event_time DESC, tenant_id)`. Verify not duplicate from M13.
- `security-engineer` → BLOCKING: audit EXPORT.GENERATED in-tx all 25; RPT-25 before/after redact for non-AUDIT; RPT-28 step-up MFA; SHA-256 in sys.export_log; wildcard bypass docs.
- `ifrs9-compliance-reviewer` → BLOCKING for RPT-13..18 (ECL/EIR read-only reports) + RPT-27 (sensitivity what-if) + RPT-28 (attestation).
- `devops-engineer` → statement_timeout 30s per-session for all report queries; Grafana alert on REPORT_QUERY_TIMEOUT; ensure read-replica has migration 000051 indexes.
