# P5-M8 State Machine — Penjualan/Pencairan Instrumen

**Module**: APP-B — Transaction Lifecycle (Penjualan/Pencairan)
**Author**: system-analyst (driven by business-analyst P5-M8 story set)
**Date**: 2026-06-20
**Status**: READY FOR backend-engineer-go implementation

---

## State Machine `trx.penjualan.status`

```
[PENDING_APPROVAL]
       │
       ├─ [approve] ROLE-APPR-TR (approver_id ≠ maker_id, signatureMethod=JWT_STEP_UP)
       │     Side-effects (atomic, single DB tx):
       │       → APPROVED (interim)
       │       → OCI recycling check (S3)
       │       → BM frequency check (S4)
       │         if BM hard block → UPDATE status=PENDING_BM_REVIEW (halt — no jurnal)
       │         else continue:
       │       → POST jurnal via P5-M2 (event codes per klasifikasi)
       │       → UPDATE mst.instrumen (qty partial / status=DISPOSED full)
       │     → POSTED  ← (final state after all side-effects succeed)
       │
       ├─ [approve + BM hard block] ROLE-APPR-TR
       │     → PENDING_BM_REVIEW ← (requires ROLE-RISK sign-off before retry)
       │
       └─ [reject] ROLE-APPR-TR (reason ≥ 30 char)
             → REJECTED  ← (terminal)
```

Notes:
- Create endpoint immediately sets `PENDING_APPROVAL` (no separate submit step).
- `POSTED` is immutable: no further transitions.
- `REJECTED` is terminal: reopen (REJECTED → PENDING_APPROVAL) deferred to later sprint.
- `PENDING_BM_REVIEW` requires separate ROLE-RISK approval workflow (out of M8 scope — flagged only).

### Valid transitions

| From | Action | To | Guard |
|---|---|---|---|
| PENDING_APPROVAL | approve (normal) | POSTED | SoD + signatureMethod + BM ≤ block_threshold + jurnal success + derecognition |
| PENDING_APPROVAL | approve (BM block) | PENDING_BM_REVIEW | SoD + signatureMethod + BM > block_threshold |
| PENDING_APPROVAL | reject | REJECTED | SoD + reason ≥ 30 char + signatureMethod |

---

## Validation Rules

### Create (S1)

| Field | Rule | Error Code | HTTP |
|---|---|---|---|
| instrumen_id | status=ACTIVE, klasifikasi_locked=TRUE | PENJUALAN_INSTRUMEN_NOT_ACTIVE | 422 |
| instrumen_id | no active penjualan (PENDING_APPROVAL/APPROVED/POSTED) for same instrumen | PENJUALAN_INSTRUMEN_NOT_ACTIVE | 422 |
| jenis_disposal | IN ('PARTIAL','FULL') | VALIDATION_FAILED | 400 |
| qty_terjual | > 0, ≤ qty_holding_pre | PENJUALAN_QTY_EXCEEDS_HOLDING | 422 |
| qty_terjual (PARTIAL) | < qty_holding_pre (must not equal for PARTIAL) | PENJUALAN_QTY_EXCEEDS_HOLDING | 422 |
| qty_terjual (FULL) | = qty_holding_pre | PENJUALAN_QTY_EXCEEDS_HOLDING | 422 |
| harga_jual_per_unit | > 0 | PENJUALAN_HARGA_INVALID | 400 |
| tanggal_eksekusi | valid DATE YYYY-MM-DD, periode_buku OPEN | PENJUALAN_PERIODE_LOCKED / VALIDATION_FAILED | 423 / 400 |
| klasifikasi | klasifikasi_locked=TRUE | PENJUALAN_KLASIFIKASI_NOT_LOCKED | 422 |

### Approve (S2)

| Check | Rule | Error Code | HTTP |
|---|---|---|---|
| SoD | approver_id ≠ maker_id | SOD_VIOLATION | 403 |
| signatureMethod | == "JWT_STEP_UP" | VALIDATION_FAILED | 400 |
| status | == PENDING_APPROVAL | WORKFLOW_INVALID_TRANSITION | 422 |
| periode_buku | status = OPEN at posting time | PENJUALAN_PERIODE_LOCKED | 423 |
| cost_basis re-verify | server recomputes from ecl.amortisasi_schedule | INTERNAL (rollback) | 500 |
| BM block | cumulative > block_threshold (sys.config) | PENJUALAN_BM_VIOLATION_BLOCK | 422 |

### Reject (S2)

| Check | Rule | Error Code | HTTP |
|---|---|---|---|
| SoD | approver_id ≠ maker_id | SOD_VIOLATION | 403 |
| status | == PENDING_APPROVAL | WORKFLOW_INVALID_TRANSITION | 422 |
| reason | len ≥ 30 char | VALIDATION_FAILED | 400 |
| signatureMethod | == "JWT_STEP_UP" | VALIDATION_FAILED | 400 |

---

## Klasifikasi Routing Matrix (S5)

### `routing.ResolveJurnalEventCode(klasifikasi, jenis_disposal)`

| Klasifikasi | JenisDisposal | EventCodes | recycleOCI | noRecyclingFlag |
|---|---|---|---|---|
| AC | PARTIAL / FULL | [PENJUALAN_AC] | false | false |
| FVOCI (debt) | PARTIAL / FULL | [PENJUALAN_FVOCI_DEBT, REKLAS_OCI_PL] | true | false |
| FVOCI_ELECTION | PARTIAL / FULL | [PENJUALAN_FVOCI_ELECTION] | false | true |
| FVTPL | PARTIAL / FULL | [PENJUALAN_FVTPL] | false | false |
| POCI | PARTIAL / FULL | [PENJUALAN_POCI] | false | false |
| klasifikasi_locked=false | any | error | — | — |

Error on `klasifikasi_locked=false`: `PENJUALAN_KLASIFIKASI_NOT_LOCKED`.
Error on unknown klasifikasi: `VALIDATION_FAILED`.

### OCI Recycling Detail (S3)

**FVOCI debt:**
- PARTIAL: `oci_recycled = oci_cumulative × (qty_terjual / qty_holding_pre)`
- FULL: `oci_recycled = oci_cumulative`
- Jurnal REKLAS_OCI_PL posted in same tx:
  - Gain (oci_cumulative > 0): Dr OCI Reserve / Cr Realized Gain P&L
  - Loss (oci_cumulative < 0): Dr Realized Loss P&L / Cr OCI Reserve
- OCI cumulative updated (reduced by recycled amount).
- Audit: `PENJUALAN.OCI_RECYCLED` in-tx.

**FVOCI_ELECTION (saham — §B5.7.1):**
- No REKLAS_OCI_PL jurnal ever posted.
- G/L stays in equity (or transferred to retained earnings per Tugure policy — non-recycled).
- `no_recycling_note` populated in response: "Gain/loss IDR X tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L."
- Warning code: `PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN` (embedded in response, HTTP 200).
- Audit: `PENJUALAN.OCI_NO_RECYCLE` in-tx with `{instrumen_id, oci_cumulative, reason: "FVOCI_ELECTION_NO_RECYCLE_PSAK71_B5.7.1"}`.

### Cost Basis per Klasifikasi

```
AC:
    cost_basis = amortized_carrying_amount (ecl.amortisasi_schedule, tanggal_eksekusi)
    Stage 3: cost_basis = gross_carrying − ecl_allowance (Net Carrying per §5.4.1(b))

FVOCI debt:
    cost_basis = amortized_carrying_amount (same as AC — separate from fair value)

FVOCI_ELECTION:
    cost_basis = cost_at_origination (acquisition cost, not amortized)

FVTPL:
    cost_basis = fair_value dari trx.mtm terkini (MTM basis)

POCI:
    cost_basis = amortized carrying per credit-adjusted EIR schedule
```

### Partial Disposal

```
proceed_idr     = harga_jual_per_unit × qty_terjual
cost_basis      = cost_basis_total × (qty_terjual / qty_holding_pre)
realized_gl     = proceed_idr − cost_basis
qty_holding_post = qty_holding_pre − qty_terjual
mst.instrumen.qty_holding = qty_holding_post   [status tetap ACTIVE]
```

### Full Disposal

```
proceed_idr     = harga_jual_per_unit × qty_terjual (= qty_holding_pre)
cost_basis      = cost_basis_total (full)
realized_gl     = proceed_idr − cost_basis
mst.instrumen.status = 'DISPOSED'
```

---

## BM Frequency Check (S4)

**Applicable only to HTC portofolio** (Business Model = HTC). Skipped for HTC&S and Other.

```
cumulative_sold_12m_idr = Σ proceed_idr WHERE tanggal_eksekusi ≥ (today - 12 months)
                           AND portofolio_id = current.portofolio_id
                           AND status IN ('POSTED')

pct = (cumulative_sold_12m_idr + current_proceed_idr) / total_nilai_portofolio × 100

warn_threshold  = sys.config PENJUALAN_BM_WARN_THRESHOLD_PCT  (default 5.0)
block_threshold = sys.config PENJUALAN_BM_BLOCK_THRESHOLD_PCT (default 10.0)

IF pct > block_threshold:
    → INSERT sys.bm_frequency_log (flag='BM_VIOLATION_BLOCK')
    → UPDATE penjualan status=PENDING_BM_REVIEW
    → Audit PENJUALAN.BM_FREQUENCY_FLAG (flag=BM_VIOLATION_BLOCK) in-tx
    → Notify ROLE-RISK
    → Return PENJUALAN_BM_VIOLATION_BLOCK error (422)
ELIF pct > warn_threshold:
    → INSERT sys.bm_frequency_log (flag='BM_VIOLATION_RISK')
    → Audit PENJUALAN.BM_FREQUENCY_FLAG (flag=BM_VIOLATION_RISK) in-tx
    → Notify ROLE-RISK
    → Continue to jurnal posting (penjualan still POSTS)
    → Response: bm_violation_risk=true
```

Threshold sourced from `sys.config` at runtime — ALCO can override via APP-C parameter management.

---

## Approve Side-Effects (Single DB Transaction)

Sequence dalam satu `*sql.Tx`:

1. Load `trx.penjualan` (SELECT FOR UPDATE, row_version check)
2. Validate status = PENDING_APPROVAL, SoD, signatureMethod
3. Server re-verify cost_basis dari `ecl.amortisasi_schedule`
4. `UPDATE trx.penjualan SET status='APPROVED', approver_id, approve_comment, signature_method, updated_at, row_version+1`
5. Compute OCI recycling (if FVOCI debt) or flag no-recycle (if FVOCI_ELECTION)
6. BM frequency check (HTC only):
   - If block → SET status=PENDING_BM_REVIEW + INSERT sys.bm_frequency_log → audit → notify → RETURN (no jurnal)
   - If warn → continue + flag
7. Call `JurnalPoster.Post(ctx, tx, PenjualanPostRequest{EventCodes: [...], ...})` → get jurnal_header_id
8. Call `InstrumenUpdater.UpdateQty(ctx, tx, instrumenID, qtyTerjual)` (PARTIAL)
   OR `InstrumenUpdater.SetDisposed(ctx, tx, instrumenID, actorID)` (FULL)
9. `UPDATE trx.penjualan SET status='POSTED', jurnal_header_id, instrumen_status_after, oci_recycled, updated_at, row_version+2`
10. `INSERT aud.audit_log` multiple events in-tx:
    - PENJUALAN.APPROVED
    - PENJUALAN.OCI_RECYCLED or PENJUALAN.OCI_NO_RECYCLE
    - PENJUALAN.BM_FREQUENCY_FLAG (if BM warning triggered)
    - PENJUALAN.POSTED (last in chain)
    - PENJUALAN.DERECOGNIZED
11. `COMMIT`

Rollback on any failure → 500 INTERNAL. `trx.penjualan` reverts to PENDING_APPROVAL.

---

## Periode Lock

Before jurnal posting step (step 7):
- Service calls `PeriodeChecker.IsOpen(ctx, tanggalEksekusi)`.
- If closed: `PENJUALAN_PERIODE_LOCKED` → full rollback.

---

## Audit Events (all in-transaction unless advisory)

| Action | Trigger | In-tx? |
|---|---|---|
| PENJUALAN.CREATED | INSERT trx.penjualan | Yes |
| PENJUALAN.APPROVED | UPDATE status=APPROVED | Yes |
| PENJUALAN.OCI_RECYCLED | FVOCI debt: REKLAS_OCI_PL posted | Yes |
| PENJUALAN.OCI_NO_RECYCLE | FVOCI_ELECTION: no recycling | Yes |
| PENJUALAN.BM_FREQUENCY_FLAG | BM warn or block triggered | Yes |
| PENJUALAN.POSTED | UPDATE status=POSTED | Yes |
| PENJUALAN.DERECOGNIZED | mst.instrumen updated/disposed | Yes |
| PENJUALAN.REJECTED | UPDATE status=REJECTED | Yes |
| PENJUALAN.SOD_VIOLATION_ATTEMPT | SoD rejected (advisory) | Yes |
| PENJUALAN.JURNAL_MISSING_CONFIG | Event code not in mapping (advisory) | After rollback |
| PENJUALAN.EXPORT | GET /trx/penjualan?export=csv | Yes |

### Audit `After` field payloads (key fields)

- `PENJUALAN.CREATED.After`: `{instrumen_id, jenis_disposal, qty_terjual, harga_jual_per_unit, proceed_idr, cost_basis, realized_gl, klasifikasi, tanggal_eksekusi}`
- `PENJUALAN.OCI_RECYCLED.After`: `{instrumen_id, oci_cumulative, oci_recycled, direction: "GAIN"|"LOSS", klasifikasi}`
- `PENJUALAN.OCI_NO_RECYCLE.After`: `{instrumen_id, oci_cumulative, reason: "FVOCI_ELECTION_NO_RECYCLE_PSAK71_B5.7.1"}`
- `PENJUALAN.BM_FREQUENCY_FLAG.After`: `{portofolio_id, pct_terjual, threshold_warning, threshold_block, flag}`
- `PENJUALAN.POSTED.After`: `{status, jurnal_header_id, bm_violation_risk}`
- `PENJUALAN.DERECOGNIZED.After`: `{instrumen_id, jenis_disposal, qty_terjual, qty_holding_after, instrumen_status_after}`

---

## Error Catalog (7 new codes)

| Code | HTTP | Trigger |
|---|---|---|
| `PENJUALAN_INSTRUMEN_NOT_ACTIVE` | 422 | instrumen status != 'ACTIVE' OR no active instrumen OR klasifikasi_locked=FALSE |
| `PENJUALAN_QTY_EXCEEDS_HOLDING` | 422 | qty_terjual > qty_holding OR PARTIAL qty = holding (must be < for PARTIAL) |
| `PENJUALAN_KLASIFIKASI_NOT_LOCKED` | 422 | klasifikasi_locked=FALSE at time of routing resolution |
| `PENJUALAN_HARGA_INVALID` | 400 | harga_jual_per_unit ≤ 0 |
| `PENJUALAN_PERIODE_LOCKED` | 423 | periode_buku.status_periode != 'OPEN' for tanggal_eksekusi |
| `PENJUALAN_BM_VIOLATION_BLOCK` | 422 | Cumulative disposal 12-month > block_threshold; penjualan → PENDING_BM_REVIEW |
| `PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN` | 200 (embedded in body warnings[]) | FVOCI_ELECTION penjualan berhasil — informational warning |

---

## Performance SLA

| Endpoint | Target |
|---|---|
| POST /trx/penjualan | ≤ 500 ms |
| GET /trx/penjualan (list) | ≤ 200 ms (cursor, indexed) |
| GET /trx/penjualan/{id} | ≤ 100 ms |
| POST /trx/penjualan/{id}/approve | ≤ 1 s (OCI + BM + jurnal + derecognition in-tx) |
| POST /trx/penjualan/{id}/reject | ≤ 300 ms |
| GET /trx/penjualan/{id}/preview | ≤ 200 ms |
| GET /trx/penjualan/bm-frequency-alerts | ≤ 200 ms |

---

## Hand-off

**backend-engineer-go**: implement `backend/internal/trx/penjualan/` per this spec.
- `routing.go`: ResolveJurnalEventCode matrix — 100% test coverage (compliance-critical).
- `calc.go`: ComputeProceed, ComputeCostBasis, ComputeRealizedGL, ComputeOCIRecycle, ComputeBMFrequency — 100% coverage.
- `validator.go`: all AC validation rules.
- `repo.go`: sqlx with cursor pagination + tenant_id (per M7 F2 lesson).
- `service.go`: full approve side-effects in single tx.
- `jurnal_poster.go`: JurnalPoster interface (mirror M7 pattern).
- `instrumen_updater.go`: InstrumenUpdater interface (UpdateQty/SetDisposed).
- `notification.go`: RiskNotifier interface (BM alert to ROLE-RISK).
- `handler.go + routes.go`: 7 endpoints, mount on `*gin.RouterGroup`.
- Migration: `db/migrations/000044_penjualan_p5m8.up.sql` + `.down.sql`.

**ifrs9-compliance-reviewer**: BLOCKING gate on S3 (OCI recycling §B5.7.1), S4 (BM §4.1.2b), S5 (jurnal multi-leg).
**security-engineer**: BLOCKING gate on S2 (SoD + audit in-transaction + idempotency).
