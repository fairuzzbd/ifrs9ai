# P5-M11 State Machine — Bulk Upload Master Instrumen

**Module**: APP-A — Bulk Upload Master Instrumen (P5-M11)
**Author**: system-analyst
**Date**: 2026-06-21
**Linked Stories**: P5-M11-S1..S5 (`docs/stories/phase-5/P5-M11-bulk-upload.md`)
**OpenAPI**: `api/openapi/app-b-bulk-upload.yaml`
**Migration**: `db/migrations/000048_bulk_upload_p5m11.up.sql`
**Backend**: `backend/internal/master/bulkupload/`

---

## 1. sys.upload_batch.status State Machine

```
                    ┌─────────────────────────────────────┐
                    │         PARSED (initial)             │
                    │  POST /bulk-upload → parse XLSX      │
                    │  Audit: BULK.UPLOADED in-tx          │
                    └──────────┬──────────────────────────┘
                               │
              ┌────────────────┴───────────────┐
              │  POST /dry-run                  │
              │  (4-stage validation pipeline)  │
              └────────────────┬───────────────┘
                               │
              ┌────────────────┴───────────────┐
              │                                 │
    ┌─────────▼──────────┐         ┌────────────▼──────────────────┐
    │  DRY_RUN_PASSED     │         │  DRY_RUN_FAILED (terminal      │
    │  TTL: dry_run_      │         │  for commit; re-upload needed) │
    │  expires_at = +1h   │         └───────────────────────────────┘
    └─────────┬──────────┘
              │  POST /commit
              │  (verify TTL + periode OPEN)
              │  Enqueue Asynq bulkupload:commit_instrumen
              │
    ┌─────────▼──────────┐
    │  COMMITTING         │
    │  (Asynq worker      │
    │   running)          │
    └─────────┬──────────┘
              │
    ┌─────────┴──────────────────────────┐
    │                                     │
┌───▼──────────────┐          ┌──────────▼──────────────────┐
│  COMMITTED        │          │  PARTIAL_COMMIT              │
│  (all rows ok)    │          │  (some rows FAILED, ok)      │
└───┬──────────────┘          └──────────┬──────────────────┘
    │                                     │
    └──────────────┬──────────────────────┘
                   │  POST /approve
                   │  ROLE-APPR-TR, SoD, signatureMethod
                   │  instrumen COMMITTED → ACTIVE
                   │  FLAGGED rows → PENDING_CLASSIFICATION
                   │
          ┌────────▼────────────────┐
          │  APPROVED                │
          │  Audit: BULK.APPROVED    │
          └────────┬────────────────┘
                   │
                   │  POST /rollback-request
                   │  ROLE-CFO, reason ≥ 50 chars
                   │  grace window check
                   │
          ┌────────▼────────────────┐
          │  ROLLBACK_PENDING        │
          │  Audit: BULK.ROLLBACK_   │
          │         REQUESTED        │
          └────────┬────────────────┘
                   │
                   │  POST /rollback-approve
                   │  ROLE-CFO, step-up MFA (scope=bulk_rollback)
                   │  freshness ≤ 5 menit (DEC-027)
                   │
          ┌────────▼────────────────┐
          │  ROLLED_BACK (terminal)  │
          │  Soft-delete instrumen   │
          │  Audit: BULK.ROLLBACK_   │
          │         APPROVED         │
          └─────────────────────────┘
```

### Valid Transitions Summary

| From | To | Trigger | Actor | Condition |
|---|---|---|---|---|
| (new) | PARSED | POST /bulk-upload | ROLE-MAKER-TR | MIME + size valid; periode OPEN |
| PARSED | DRY_RUN_PASSED | POST /dry-run | ROLE-MAKER-TR (owner) | Stage 1-3 pass |
| PARSED | DRY_RUN_FAILED | POST /dry-run | ROLE-MAKER-TR (owner) | Stage 1-3 any fail |
| DRY_RUN_FAILED | PARSED | POST /bulk-upload (re-upload) | ROLE-MAKER-TR | Periodic re-upload creates new batch |
| DRY_RUN_PASSED | COMMITTING | POST /commit | ROLE-MAKER-TR (owner) | TTL valid + periode OPEN |
| COMMITTING | COMMITTED | Asynq job complete | System | All rows COMMITTED |
| COMMITTING | PARTIAL_COMMIT | Asynq job complete | System | Some rows FAILED |
| COMMITTED | APPROVED | POST /approve | ROLE-APPR-TR | SoD: approver ≠ maker |
| PARTIAL_COMMIT | APPROVED | POST /approve | ROLE-APPR-TR | SoD: approver ≠ maker |
| APPROVED | ROLLBACK_PENDING | POST /rollback-request | ROLE-CFO | Grace window valid; reason ≥ 50 |
| ROLLBACK_PENDING | ROLLED_BACK | POST /rollback-approve | ROLE-CFO | Step-up MFA; freshness ≤ 5 min |

### Rollback Status (separate field)

`sys.upload_batch.rollback_status`:
- `NOT_REQUESTED` (default)
- `PENDING` (rollback-request submitted)
- `APPROVED` → triggers ROLLED_BACK batch status
- `EXPIRED` (grace window passed without completion)

---

## 2. Per-Row Status (sys.upload_batch_row.row_status)

```
PENDING
  → COMMITTED        (worker INSERT mst.instrumen ok)
  → FAILED           (INSERT error; error detail in row_error_jsonb; batch continues)
  → ROLLED_BACK      (CFO rollback within grace window; soft-delete instrumen)

Special state:
FLAGGED_MANUAL_REVIEW  (Stage 4 SPPI+BM ambiguous; COMMIT inserts with PENDING_CLASSIFICATION)
  → COMMITTED (treat as committed to mst.instrumen; instrumen.status = PENDING_CLASSIFICATION)
  → ROLLED_BACK (CFO rollback applies to all rows incl. flagged)
```

---

## 3. mst.instrumen.status for Bulk-Uploaded Rows

```
POST /commit (worker):
  instrumen.status = 'PENDING_APPROVAL_BULK'
  instrumen.bulk_upload_batch_id = batch_id

POST /approve (ROLE-APPR-TR):
  row_status=COMMITTED → instrumen.status = 'ACTIVE' (= 'AKTIF')
  row_status=FLAGGED_MANUAL_REVIEW → instrumen.status = 'PENDING_CLASSIFICATION'
  row_status=FAILED → no change

PATCH /master/instrumen/{id}/klasifikasi-manual (ROLE-RISK):
  PENDING_CLASSIFICATION → ACTIVE after manual klasifikasi resolution

POST /rollback-approve (ROLE-CFO):
  All instrumen from batch → deleted_at = now(), deleted_by = CFO_user_id (soft-delete DEC-018)
```

---

## 4. 4-Stage DRY_RUN Validation Pipeline

### Stage 1 — Validate Format
- Cell types match schema per sheet
- Mandatory columns present (kode, counterparty_id/bank_id, mata_uang, etc.)
- No blank kode_instrumen
- Date fields parseable (YYYY-MM-DD)
- Numeric fields parseable (saldo, kupon, nilai_nominal, etc.)
- **Failure**: row_status = FAILED; collect all errors, continue next row
- **DRY_RUN result**: DRY_RUN_FAILED if any Stage 1 error

### Stage 2 — Validate Business Rules
- Range checks: saldo > 0, bunga ∈ [0, 1], tenor > 0
- Enum checks: mata_uang ∈ valid codes, tipe_instrumen matches sheet
- jatuh_tempo > tanggal_penempatan
- Duplicate kode_instrumen within batch → error
- **Failure**: row_status = FAILED; continue
- **DRY_RUN result**: DRY_RUN_FAILED if any Stage 2 error

### Stage 3 — Validate Cross-Sheet References
- counterparty_id → mst.counterparty (status=APPROVED, not deleted)
- bank_id → mst.counterparty (tipe='BANK', status=APPROVED)
- mata_uang → mst.mata_uang (status=APPROVED)
- portofolio_id → mst.portofolio (status=APPROVED)
- kode_instrumen → mst.instrumen: must NOT already exist (conflict check)
- **Failure**: row_status = FAILED; continue
- **DRY_RUN result**: DRY_RUN_FAILED if any Stage 3 error

### Stage 4 — SPPI+BM Auto-Eval (Phase 3 Integration)
- Call Phase 3 SPPI+BM auto-eval service per row
- Result: klasifikasi_psak71 ∈ ('AC', 'FVOCI', 'FVOCI_ELECTION', 'FVTPL')
- Ambiguous result → row_status = FLAGGED_MANUAL_REVIEW; flag_reason populated
- Phase 3 service unavailable → stage_summary.sppiServiceUnavailable = true; treat all as FLAGGED
- **Failure on ambiguous**: row marked FLAGGED — does NOT make DRY_RUN_FAILED
- **DRY_RUN result**: DRY_RUN_PASSED even with flagged rows (Stage 1-3 must all pass)

### Stage 4 Stub (P5-M11 Phase — Phase 3 not wired yet)
`SPPIBMEvaluator` interface + stub returning default `klasifikasi='AC'` for all rows.
Real wiring in `cmd/api/main.go` when Phase 3 service is available.

---

## 5. Asynq Commit Job Flow

**Task name**: `bulkupload:commit_instrumen`
**Queue**: `default`
**Max retry**: 2 (idempotent — row_status check before INSERT)
**Timeout**: 30 minutes

```
Worker HandleCommitInstrumen(ctx, task):
  1. Unmarshal payload: { batch_id, actor_id, tenant_id, job_id }
  2. Load batch; verify status = COMMITTING; verify periode OPEN
  3. Load all rows WHERE row_status = 'PENDING'
  4. Progress: 0% → update sys.job + Redis pub/sub
  5. For each row (ordered by sheet, row_number):
     a. BEGIN SAVEPOINT sp_row_{i}
     b. INSERT mst.instrumen (from row_data_jsonb + klasifikasi from dry_run result)
     c. UPDATE sys.upload_batch_row SET row_status='COMMITTED', instrumen_id=new_id
     d. RELEASE SAVEPOINT
     e. On error: ROLLBACK TO SAVEPOINT sp_row_{i}
                  UPDATE row_status='FAILED', row_error_jsonb={error}
     f. Every 10% OR every 100 rows: UPDATE sys.job.progress + Redis publish
  6. Count committed_rows, failed_rows
  7. BEGIN TX (final):
     a. UPDATE sys.upload_batch SET status = 'COMMITTED' or 'PARTIAL_COMMIT'
                                     committed_rows, failed_rows
                                     rollback_grace_expires_at = committed_at + GRACE_DAYS
     b. Audit BULK.COMMITTED or BULK.PARTIAL_COMMIT in-TX
     c. COMMIT
  8. UPDATE sys.job SET status='completed', progress=100
  9. Redis pub/sub: {"event":"completed", "progress":100, "committedRows":N, "failedRows":M}
```

**Per-row savepoint pattern** ensures partial commit: failed rows isolated, committed rows persist.

---

## 6. Rollback Flow

```
POST /rollback-request:
  1. Verify batch.status = APPROVED
  2. Verify now() ≤ batch.committed_at + BULK_ROLLBACK_GRACE_DAYS
  3. Verify reason.length ≥ 50
  4. BEGIN TX:
     a. UPDATE batch.rollback_status = 'PENDING', batch.status = 'ROLLBACK_PENDING'
     b. Audit BULK.ROLLBACK_REQUESTED in-TX
     c. COMMIT

POST /rollback-approve:
  1. Verify X-Step-Up-Token freshness ≤ 5 min, scope='bulk_rollback'
  2. Verify batch.status = ROLLBACK_PENDING
  3. BEGIN TX:
     a. UPDATE mst.instrumen SET deleted_at=now(), deleted_by=cfo_id
        WHERE bulk_upload_batch_id = batch_id AND deleted_at IS NULL
     b. UPDATE sys.upload_batch_row SET row_status='ROLLED_BACK'
        WHERE batch_id = batch_id AND row_status IN ('COMMITTED','FLAGGED_MANUAL_REVIEW')
     c. UPDATE batch.status = 'ROLLED_BACK', rollback_status = 'APPROVED', rollback_at = now()
     d. Audit BULK.ROLLBACK_APPROVED in-TX (with rolled_back_count)
     e. COMMIT
  4. Jurnal compensation: stub (Phase 5 M12 wiring when jurnal entries exist)
```

---

## 7. Grace Window Configuration

`sys.config_param`:
- `BULK_ROLLBACK_GRACE_DAYS` — default `7` (days)
- `BULK_FILE_MAX_MB` — default `50`
- `BULK_DRY_RUN_TTL_SECONDS` — default `3600` (1 hour)

Grace window = `batch.committed_at + BULK_ROLLBACK_GRACE_DAYS days`.
Computed at rollback-request time (not cached). Non-retroactive for existing batches.

---

## 8. MIME Validation

XLSX files are ZIP archives. Server-side validation (S1-AC3):
1. Read first 4 bytes of uploaded file
2. Check `PK\x03\x04` (ZIP local file header signature)
3. Reject `BULK_MIME_INVALID` if not matching — do NOT trust `Content-Type` header
4. Check file size ≤ 50MB BEFORE parsing (S1-AC2)

---

## 9. Audit Events (9 total)

| Event | Trigger | In-TX | after_jsonb key fields |
|---|---|---|---|
| `BULK.UPLOADED` | POST /upload parsed | Yes | batch_id, total_rows, file_name, sheets, parse_error_count |
| `BULK.VALIDATED_DRY_RUN` | POST /dry-run complete | Yes | batch_id, status, valid/invalid/flagged counts, stage_summary |
| `BULK.COMMITTED` | Worker done, all rows ok | Yes | batch_id, committed_rows, job_id |
| `BULK.PARTIAL_COMMIT` | Worker done, some rows fail | Yes | batch_id, committed_rows, failed_rows, failed_row_ids |
| `BULK.APPROVED` | POST /approve | Yes | batch_id, activated_count, pending_manual, approver_id, comment |
| `BULK.SOD_VIOLATION_ATTEMPT` | POST /approve (SoD fail) | Yes | batch_id, approver_id, maker_id |
| `BULK.ROLLBACK_REQUESTED` | POST /rollback-request | Yes | batch_id, reason, actor_id, mfa_method |
| `BULK.ROLLBACK_APPROVED` | POST /rollback-approve | Yes | batch_id, rolled_back_count, commit_at, rollback_at |
| `SYS.CONFIG_PARAM_UPDATED` | PATCH /sys/config-param/* | Yes | param, old_value, new_value, actor_id |

---

## 10. Performance SLA

| Operation | Target | Notes |
|---|---|---|
| Upload + parse 500 rows XLSX | ≤ 10s | excelize streaming reader |
| DRY_RUN 500 rows | ≤ 30s | Stage 4: concurrent per sheet (goroutines) |
| Commit worker 500 rows | ≤ 5 min | Per-row savepoint; progress 0→100% |
| GET /batch/{id} with 500 rows | P95 ≤ 200ms | Cursor pagination; indexed row_status |

---

## 11. Periode Lock Check

Checked at TWO points:
1. **Upload (S1)**: `POST /bulk-upload` — reject 423 `BULK_PERIODE_LOCKED` before parse
2. **Commit (S3)**: `POST /commit` — verify periode still OPEN before enqueue; worker also checks at start

Period status from `mst.periode_buku` where `is_current=true` OR by explicit `periode_bulanan_id` in batch.

---

## 12. Hand-off

| To | Task |
|---|---|
| `backend-engineer-go` | Implement `backend/internal/master/bulkupload/` per spec |
| `security-engineer` | **BLOCKING** review: audit in-TX (9 events), SoD server-side, step-up MFA rollback, MIME magic-byte, idempotency all 5 endpoints, soft-delete DEC-018 |
| `ifrs9-compliance-reviewer` | Advisory (non-blocking) — P5-M11 tidak menyentuh ECL/EIR compute |
| `qa-engineer` | UAT: S1-AC1..4, S2-AC1..4, S3-AC1..4, S4-AC1..4, S5-AC1..4 (20 AC total) |
| `devops-engineer` | Worker registration in cmd/worker; sys.config_param seeding |
