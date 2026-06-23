# State Machine — P5-M12 Mapping Jurnal 6-Eyes Workflow

**Module**: APP-D — Mapping Jurnal  
**Author**: backend-engineer-go (system-analyst handoff)  
**Date**: 2026-06-22  
**References**: P5-M12-S1..S5, DEC-017, DEC-018, DEC-021, DEC-027

---

## 1. States

| State | aktif_flag | Resolver can use? | Description |
|---|---|---|---|
| `DRAFT` | FALSE | No | Initial state; only maker can edit |
| `PENDING_REVIEW` | FALSE | No | Submitted by maker; awaiting ROLE-AKUN-CTL review |
| `PENDING_APPROVAL` | FALSE | No | Reviewed; awaiting ROLE-AKUN-CTL approver (4-eyes path) |
| `PENDING_APPROVAL_2` | FALSE | No | Reviewed; awaiting ROLE-RISK approver-2 (6-eyes regulated path) |
| `APPROVED_ACTIVE` | TRUE | Yes | Fully approved; resolver uses this template |

Rejection at any step → `DRAFT` (reset submit_at, clear reviewer/approver fields from in-progress step).

---

## 2. Transition Table

```
DRAFT ──────────────────────────────────────────────► PENDING_REVIEW
        POST /submit (ROLE-AKUN, maker_id set)
        Pre: all detail rows have non-null akun_debit/kredit
        Audit: MAPPING.SUBMITTED in-tx

PENDING_REVIEW ─────────────────────────────────────► PENDING_APPROVAL   [4-eyes path]
               POST /review (ROLE-AKUN-CTL, SoD: reviewer≠maker)
               Pre: comment ≥ 30 chars; event_code NOT IN REGULATED_EVENT_CODES
               Sets: reviewer_id, reviewer_signed_at, reviewer_signature_hash
               Audit: MAPPING.REVIEWED in-tx

PENDING_REVIEW ─────────────────────────────────────► PENDING_APPROVAL_2  [6-eyes regulated path]
               POST /review (ROLE-AKUN-CTL, SoD: reviewer≠maker)
               Pre: comment ≥ 30 chars; event_code IN REGULATED_EVENT_CODES
               Sets: reviewer_id, reviewer_signed_at, reviewer_signature_hash
               Audit: MAPPING.REVIEWED in-tx

PENDING_APPROVAL ───────────────────────────────────► APPROVED_ACTIVE     [4-eyes final]
                 POST /approve (ROLE-AKUN-CTL, SoD: approver≠reviewer≠maker)
                 Pre: periode_buku ≠ HARD_CLOSED; comment ≥ 10
                 Action: atomic flip — prior APPROVED_ACTIVE for same event_code
                         gets effective_to = now(); new version aktif_flag = TRUE
                 Sets: approver_id, approver_signed_at, approver_signature_hash
                 Audit: MAPPING.APPROVED_ACTIVE in-tx

PENDING_APPROVAL_2 ─────────────────────────────────► APPROVED_ACTIVE     [6-eyes final]
                   POST /approve-2 (ROLE-RISK, SoD: approver_2≠approver≠reviewer≠maker)
                   Pre: X-Step-Up-Token valid < 5min (DEC-027);
                        periode_buku ≠ HARD_CLOSED; comment ≥ 10
                   Action: atomic flip — same as 4-eyes approve
                   Sets: approver_2_id, approver_2_signed_at, approver_2_signature_hash
                   Audit: MAPPING.APPROVED_ACTIVE in-tx (mfa_method in after_jsonb)

ANY [PENDING_*] ─────────────────────────────────────► DRAFT              [reject]
                POST /reject (reviewer or approver)
                Pre: reason ≥ 30 chars
                Action: reset reject_reason; clear in-progress step fields
                Audit: MAPPING.REJECTED in-tx
```

---

## 3. 4-Eyes vs 6-Eyes Path Selection

Path is determined at **submit time** by the backend service reading `REGULATED_EVENT_CODES` from `sys.config`.

- If `event_code IN REGULATED_EVENT_CODES` → `workflow_path = '6-eyes'`
- Otherwise → `workflow_path = '4-eyes'`

The `workflow_path` column is set at INSERT (new version) and is **immutable** thereafter. Changing the config param does not retroactively change existing in-flight versions.

### Regulated Event Codes (seeded from sys.config `REGULATED_EVENT_CODES`)

```
ECL_PEMBENTUKAN, ECL_REVERSAL, POCI_DELTA_ECL,
MTM_FVTPL, MTM_FVOCI, MTM_FVOCI_ELECTION,
REKLAS_OCI_PL, REKLASIFIKASI_AC_FVOCI, REKLASIFIKASI_FVOCI_AC,
MODIFIKASI_MATERIAL, EIR_CATCH_UP_ADJUSTMENT, STAGE_MIGRATION, FX_UNREALIZED
```

---

## 4. Version Chain + Atomic ACTIVE Flip

Every `mst.mapping_jurnal_header` row represents **one version** of a mapping.

- `parent_id` — FK to predecessor version (NULL for first version / seeds)
- `effective_from` — set at INSERT time of new version (now())
- `effective_to` — NULL (open-ended) while DRAFT/PENDING_*; set to now() when superseded by a newer APPROVED_ACTIVE

### Atomic ACTIVE flip (in same DB transaction as approve/approve-2):

```sql
-- Step 1: flip prior ACTIVE version's effective_to
UPDATE mst.mapping_jurnal_header
SET effective_to = now(), updated_at = now(), updated_by = $actor, row_version = row_version + 1
WHERE event_code = $event_code
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL
  AND tenant_id = $tenant_id;

-- Step 2: activate new version
UPDATE mst.mapping_jurnal_header
SET workflow_status = 'APPROVED_ACTIVE',
    aktif_flag = TRUE,
    approver_id = $actor,         -- or approver_2_id for 6-eyes
    approver_signed_at = now(),
    approver_signature_hash = $hash,
    updated_at = now(), updated_by = $actor, row_version = row_version + 1
WHERE id = $version_id AND tenant_id = $tenant_id;
```

Both UPDATEs run in the same serializable transaction. The BEFORE UPDATE trigger on `mst.mapping_jurnal_header` must **allow** `effective_to` update on APPROVED_ACTIVE rows (only that column), while blocking all other field mutations.

---

## 5. SoD Enforcement

Enforced **first in service layer** (primary), then **DB CHECK constraint** (defense-in-depth, DEC-017):

| Step | Rule |
|---|---|
| Submit | maker_id = current user (implicit — sets it) |
| Review | reviewer_id ≠ maker_id |
| Approve (4-eyes) | approver_id ≠ reviewer_id AND ≠ maker_id |
| Approve-2 (6-eyes) | approver_2_id ≠ approver_id AND ≠ reviewer_id AND ≠ maker_id |

SoD violation → error code `MAPPING_SOD_VIOLATION` (403) + `MAPPING.SOD_VIOLATION_ATTEMPT` audit event in-tx.

---

## 6. Periode Lock

Checked **only at APPROVE / APPROVE-2 step** (not at DRAFT or submit). If `mst.periode_buku.status_periode = 'HARD_CLOSED'`, return `MAPPING_PERIODE_LOCKED` (423). Draft and submit are allowed even during HARD_CLOSED.

---

## 7. Audit Events (all in-transaction with business mutation)

| Event | Trigger |
|---|---|
| `MAPPING.DETAIL_CREATED` | POST new detail row |
| `MAPPING.VERSION_CREATED` | POST new-version |
| `MAPPING.SUBMITTED` | POST submit |
| `MAPPING.REVIEWED` | POST review |
| `MAPPING.APPROVED_ACTIVE` | POST approve or approve-2 |
| `MAPPING.REJECTED` | POST reject |
| `MAPPING.SOD_VIOLATION_ATTEMPT` | SoD check failure (any step) |
| `MAPPING.BULK_IMPORTED` | POST bulk-import parse complete |
| `MAPPING.EXPORT` | GET export |
| `MAPPING.RPT19_EXPORTED` | RPT-19 export |
| `MAPPING.RPT20_EXPORTED` | RPT-20 export |
| `MAPPING.RPT21_EXPORTED` | RPT-21 export (async job complete) |

---

## 8. Performance SLA

- `GET /mapping-jurnal` list: P95 ≤ 200ms (indexed on `event_code`, `workflow_status`, `tenant_id`)
- `GET /mapping-jurnal/{event_code}` detail + history: P95 ≤ 200ms
- Approve (atomic flip): P95 ≤ 500ms (serializable TX on one event_code)
- RPT-19 / RPT-20: P95 ≤ 2s (small dataset — 27 events max)
- RPT-21 (audit log query): P95 ≤ 500ms with index on `(action, tenant_id, event_time)`
- RPT-21 export > 10k rows: async Asynq job

---

## 9. Handoff

- `backend-engineer-go` → implement `backend/internal/master/mappingjurnal/` (domain, validator, repo, service, handler, routes)
- `frontend-engineer-nextjs` → implement routes + components under `frontend/src/components/blips/mapping-jurnal/`
- `qa-engineer` → E2E: full 6-eyes flow; SoD violation; periode lock; bulk import valid+invalid rows
- `security-engineer` → BLOCKING: verify SoD 4-way, step-up token freshness, audit in-tx, immutability trigger
- `ifrs9-compliance-reviewer` → BLOCKING: verify REGULATED_EVENT_CODES list covers all ECL/EIR/MTM/REKLAS event codes; RPT-20 balance check covers all D/K scenarios per PSAK 71
