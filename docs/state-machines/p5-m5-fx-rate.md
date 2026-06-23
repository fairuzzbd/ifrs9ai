# P5-M5 FX Rate Management — State Machine + Integration Contract

> Scope: `mst.kurs` workflow + `locked_flag` + BI JISDOR cron + FX treatment routing.
> Companion to `api/openapi/app-d-fx-rate.yaml` and `docs/stories/phase-5/P5-M5-fx-rate.md`.

## 1. Entities

### 1.1 `mst.kurs` columns relevant to state machine
| Col | Type | Notes |
|---|---|---|
| `workflow_status` | TEXT | PENDING_APPROVAL / ACTIVE / REJECTED |
| `locked_flag` | BOOLEAN | TRUE when periode hard-closed; mutations refused |
| `deviation_flag` | BOOLEAN | TRUE when rate deviation > 20% from prior day |
| `rate_deviation_pct` | NUMERIC(7,4) | computed at insert |
| `jisdor_fetch_metadata` | JSONB | `{source:'JISDOR'|'MANUAL', fetched_at, attempt, raw_response}` |
| `reject_reason` | TEXT | required when workflow_status=REJECTED |
| `upload_batch_id` | UUID | links to `sys.upload_batch` row |

## 2. Workflow state machine — `mst.kurs.workflow_status`

```
                ┌─────────────────────────┐
                │                         │
                ▼                         │
        ┌─────────────────┐               │
JISDOR  │ PENDING_APPROVAL│ ── reject ──▶ │ REJECTED (terminal)
auto    └──────┬──────────┘               │
upload         │                          │
               │  approve (4-eyes, SoD)   │
               ▼                          │
        ┌─────────────────┐               │
        │     ACTIVE      │               │
        └────────┬────────┘               │
                 │                        │
       periode   │                        │
       hard-     ▼                        │
       close   locked_flag = TRUE         │
                 │                        │
        ┌────────┴────────┐               │
        │ ACTIVE + LOCKED │ ── reopen ──▶ ACTIVE (locked_flag=FALSE)
        └─────────────────┘
```

### 2.1 Transitions

| From | Action | To | Actor | Required | Side effects |
|---|---|---|---|---|---|
| (none) | JISDOR cron insert (auto) | PENDING_APPROVAL | System (Asynq) | holiday check pass; rate deviation evaluated | `KURS.JISDOR_FETCH` audit; if deviation ≤ 20% AND auto-approve config TRUE → ACTIVE in same tx |
| (none) | Manual upload | PENDING_APPROVAL | ROLE-AKUN | Idempotency-Key; XLSX/CSV parsed; rate range valid | `KURS.UPLOADED` audit; `upload_batch_id` set |
| PENDING_APPROVAL | Approve | ACTIVE | ROLE-AKUN-CTL | Idempotency-Key; SoD (maker ≠ approver) | `KURS.APPROVED` audit in-tx |
| PENDING_APPROVAL | Reject | REJECTED | ROLE-AKUN-CTL | reject_reason ≥ 30 char | `KURS.REJECTED` audit in-tx |
| ACTIVE | Periode hard-close fires | ACTIVE (locked_flag=TRUE) | System (P5-M4 hook) | tanggal_berlaku ∈ periode | `KURS.LOCKED` audit; UPDATE/DELETE refused 423 FX_RATE_LOCKED |
| ACTIVE (locked) | Periode reopen | ACTIVE (locked_flag=FALSE) | System (P5-M4 hook) | reopen approve | `KURS.UNLOCKED` audit |

### 2.2 Invariants
- `workflow_status='ACTIVE'` ∧ `locked_flag=TRUE` → no UPDATE/DELETE allowed; INSERT for same (kode_mata_uang, tanggal_berlaku) → 409 KURS_DUPLICATE_DATE.
- `workflow_status='ACTIVE'` ∧ `locked_flag=FALSE` → UPDATE allowed only by ROLE-AKUN-CTL with audit.
- `workflow_status='REJECTED'` → terminal; new upload for same date allowed (separate row).
- Only ONE `ACTIVE` row per (kode_mata_uang, tanggal_berlaku) — enforced by partial unique index `WHERE workflow_status='ACTIVE'`.

## 3. BI JISDOR worker (Asynq cron)

### 3.1 Schedule
- Cron: `30 10 * * 1-5` Asia/Jakarta (10:30 WIB Mon-Fri)
- Skip if `tanggal_hari_ini ∈ sys.holiday_calendar` (Indonesia libur nasional + libur khusus BI)
- Task name: `fx:jisdor-fetch`
- Manual trigger: `POST /master/kurs/jisdor-sync` (ROLE-IT-ADMIN only)

### 3.2 Worker flow
1. Read `sys.config_param FX_JISDOR_CURRENCIES` → list of (kode_mata_uang) to fetch
2. For each currency:
   1. Idempotency check: `SELECT 1 FROM mst.kurs WHERE kode_mata_uang=? AND tanggal_berlaku=? AND workflow_status IN ('ACTIVE','PENDING_APPROVAL')` — skip if exists
   2. HTTP GET to BI JISDOR endpoint (URL via `sys.config_param FX_JISDOR_BASE_URL`; integration-engineer adapter)
   3. Parse rate tengah/beli/jual
   4. Compute `rate_deviation_pct` vs previous ACTIVE row (same currency, prior business day)
   5. `deviation_flag = (abs(rate_deviation_pct) > 20.0)`
   6. INSERT row workflow_status=PENDING_APPROVAL; if NOT deviation_flag AND `FX_JISDOR_AUTOAPPROVE=true` → workflow_status=ACTIVE in same tx
   7. Audit row `KURS.JISDOR_FETCH` + (optional `KURS.APPROVED` if auto-approved)
3. On failure (BI JISDOR 5xx, timeout, parse error): retry 3× exponential backoff → DLQ `sys.dlq_fx_jisdor`; alert ROLE-IT-ADMIN
4. On success: emit metric `fx_jisdor_fetch_success_total{currency}`

### 3.3 Holiday calendar
- Table `sys.holiday_calendar` columns: `tanggal DATE PRIMARY KEY`, `nama_libur TEXT`, `sumber TEXT` ('KEPRES'|'BI'|'INTERNAL')
- Seed 2026 from Keppres libur nasional (data-modeler responsibility); maintenance by ROLE-IT-ADMIN
- Worker skips fetch silently; logs `KURS.JISDOR_SKIPPED_HOLIDAY`

## 4. Rate validation rules

### 4.1 Range bounds (per currency)
- Hard-coded sanity check (USD: 5000-50000 IDR; EUR: 5000-50000; JPY: 50-500; SGD: 5000-30000; AUD: 5000-25000; GBP: 5000-40000)
- Out-of-range → 422 KURS_UPLOAD_VALIDATION_FAILED with field-level error

### 4.2 Deviation threshold
- > 20% from prior business-day ACTIVE rate → `deviation_flag=TRUE`
- Auto-approve disabled when flag=TRUE → must go through 4-eyes regardless of config

### 4.3 Date validation
- `tanggal_berlaku` must be valid business day (Mon-Fri) OR explicit holiday-override flag in upload
- `tanggal_berlaku` must fall within an OPEN or SOFT_CLOSED periode_buku — else 422 KURS_PERIODE_MISMATCH
- `tanggal_berlaku` cannot be > 7 days in future

## 5. FX gain/loss treatment routing

### 5.1 `GET /master/kurs/treatment/{instrumen_id}` response

```json
{
  "instrumen_id": "...",
  "klasifikasi_psak71": "AC|FVOCI_DEBT|FVOCI_EQUITY|FVTPL|POCI",
  "mata_uang": "USD|IDR|...",
  "treatment": "P_AND_L_FOREIGN_EXCHANGE|OCI_FOREIGN_EXCHANGE_RESERVE|NO_OCI_RECYCLING|NO_FX_TREATMENT",
  "reasoning": "AC + FCY → P&L per PSAK 71 §B5.7.2",
  "klasifikasi_used": { "snapshot_at": "...", "version": 3 }
}
```

### 5.2 Decision tree

| Klasifikasi | Mata uang | Treatment | Recycling on derecognition? |
|---|---|---|---|
| AC | FCY | `P_AND_L_FOREIGN_EXCHANGE` | N/A (amortised cost) |
| AC | IDR | `NO_FX_TREATMENT` | — |
| FVOCI debt | FCY | `OCI_FOREIGN_EXCHANGE_RESERVE` (FX component to P&L per PSAK 71 §B5.7.2A); other fair value to OCI | YES on derecognition |
| FVOCI debt | IDR | `NO_FX_TREATMENT` | — |
| FVOCI Election | FCY | `NO_OCI_RECYCLING` (all to OCI, never recycled) | NO |
| FVTPL | FCY | `P_AND_L_FOREIGN_EXCHANGE` | N/A |
| POCI | FCY | `P_AND_L_FOREIGN_EXCHANGE` | N/A |
| Any | klasifikasi locked=FALSE | 422 KLASIFIKASI_NOT_LOCKED | — |

Note (PSAK 71 §B5.7.2A): for FVOCI debt FCY, the FX component of the change in carrying amount is recognised in P&L, while the remaining fair-value change is recognised in OCI. This decision is consumed by P5-M6 (MTM jurnal mapping) and P5-M8 (disposal recycling).

## 6. Locked-flag cascade (P5-M4 → P5-M5 contract)

### 6.1 On hard-close approve (`PERIODE.HARD_CLOSED`)
- P5-M4 `ApproveHardClose` calls `closeflow → fx.LockRatesForPeriode(ctx, tx, periode_id)` in the SAME transaction
- `fx.LockRatesForPeriode` UPDATE `mst.kurs SET locked_flag=TRUE WHERE tanggal_berlaku BETWEEN periode.tanggal_mulai AND periode.tanggal_akhir AND tenant_id=?`
- Writes one `KURS.LOCKED` audit row per affected currency (or single bulk row with count)

### 6.2 On reopen approve (CLOSED → SOFT_CLOSED)
- Symmetric `fx.UnlockRatesForPeriode` in same tx
- Audit `KURS.UNLOCKED`

### 6.3 Enforcement
- App-layer: `mst.kurs` repository checks `locked_flag` before UPDATE/DELETE; raises `ErrFXRateLocked` (423)
- DB-layer (defence in depth): trigger `tg_kurs_locked_check BEFORE UPDATE OR DELETE ON mst.kurs` raises if `OLD.locked_flag=TRUE` (defined in migration 000020 per stories doc; verify)

## 7. Error catalog mapping (5 new codes)

| Code | HTTP | Trigger | AC |
|---|---|---|---|
| `FX_RATE_LOCKED` | 423 | UPDATE/DELETE on row with `locked_flag=TRUE`; INSERT into locked (currency, date) | S4-AC2, S4-AC3 |
| `KURS_DUPLICATE_DATE` | 409 | Unique violation on (kode_mata_uang, tanggal_berlaku) with workflow_status IN ('ACTIVE','PENDING_APPROVAL') | S1-AC2, S2-AC2 |
| `KURS_UPLOAD_VALIDATION_FAILED` | 422 | Range out-of-bounds; deviation > 20% without override; future date > 7d; non-business day without override | S2-AC3 |
| `KLASIFIKASI_NOT_LOCKED` | 422 | `GET /treatment/{instrumen_id}` while klasifikasi PSAK 71 not yet locked | S5-AC2 |
| `KURS_PERIODE_MISMATCH` | 422 | `tanggal_berlaku` not in any OPEN/SOFT_CLOSED periode | S2-AC4 |

## 8. Audit events

| Event | Action string | When | In-tx? |
|---|---|---|---|
| Insert via JISDOR | `KURS.JISDOR_FETCH` | worker insert | YES |
| Manual upload | `KURS.UPLOADED` | POST /upload | YES |
| Approve | `KURS.APPROVED` | POST /approve | YES (same tx as workflow_status update) |
| Reject | `KURS.REJECTED` | POST /reject | YES |
| Lock | `KURS.LOCKED` | P5-M4 hard-close hook | YES (in P5-M4 hard-close tx) |
| Unlock | `KURS.UNLOCKED` | P5-M4 reopen hook | YES |
| JISDOR sync trigger | `KURS.JISDOR_SYNC_TRIGGERED` | POST /jisdor-sync | YES |
| Holiday skip | `KURS.JISDOR_SKIPPED_HOLIDAY` | worker skip | YES |
| FX treatment read | (no audit — read-only by design) | GET /treatment | — |
| Export | `KURS.EXPORT` | DataTable export | YES |

All in-tx audit writes use `aud.audit_log` with hash chain (DEC-018).

## 9. Performance SLA

| Operation | Target | Notes |
|---|---|---|
| `GET /master/kurs` (list, 50 row default) | P95 ≤ 200ms | cursor pagination, indexed |
| `GET /master/kurs/{id}` | P95 ≤ 50ms | PK lookup |
| `POST /master/kurs/upload` (parsing 100 row XLSX) | P95 ≤ 3s | inline |
| `POST /master/kurs/upload` (≥ 1000 row) | async Asynq + JobProgressPanel | per §3 ux-patterns |
| `POST /master/kurs/jisdor-sync` | enqueue ≤ 100ms; worker fetch ≤ 30s | retries 3× |
| `POST /master/kurs/upload/{id}/approve` | P95 ≤ 200ms | single row UPDATE |
| `GET /master/kurs/treatment/{instrumen_id}` | P95 ≤ 100ms | klasifikasi cached |
| BI JISDOR cron (full 6 currency) | wall-clock ≤ 60s | sequential acceptable |

## 10. Hand-off notes

### data-modeler (migration 000039)
- ALTER `mst.kurs`:
  - `kurs_tengah`, `kurs_beli`, `kurs_jual` precision upgrade to `NUMERIC(20,8)` per DEC-016 (was 15,4)
  - Add `deviation_flag BOOLEAN NOT NULL DEFAULT FALSE`
  - Add `rate_deviation_pct NUMERIC(7,4)`
  - Add `jisdor_fetch_metadata JSONB`
  - Add `reject_reason TEXT`
  - Add `upload_batch_id UUID REFERENCES sys.upload_batch(id)`
  - Partial unique index `idx_kurs_active_unique ON mst.kurs(kode_mata_uang, tanggal_berlaku) WHERE workflow_status='ACTIVE' AND deleted_at IS NULL`
- CREATE `sys.holiday_calendar` (tanggal PK, nama_libur, sumber, audit cols)
- CREATE `sys.dlq_fx_jisdor` mirror of dlq pattern
- Seed `sys.config_param`:
  - `FX_JISDOR_CURRENCIES` = `USD,EUR,JPY,SGD,AUD,GBP`
  - `FX_JISDOR_BASE_URL` = placeholder for runtime override
  - `FX_JISDOR_AUTOAPPROVE` = `false` (safe default; ROLE-AKUN-CTL must approve manually)
  - `FX_RATE_DEVIATION_THRESHOLD_PCT` = `20.0`
- Verify `tg_kurs_locked_check` trigger exists from migration 000020; if not, add

### integration-engineer (BI JISDOR adapter)
- BI JISDOR public endpoint: scrape HTML at `https://www.bi.go.id/en/statistik/informasi-kurs/jisdor-sbn/default.aspx` OR JSON API if available
- **Recommendation**: scrape with goquery + retry/backoff. No official REST API as of cutoff. Document adapter interface so swap-in REST is trivial later.
- Implement `FxRateProvider` interface in `internal/integration/fx/` with `Fetch(ctx, currency, date) (Rate, error)`
- Production adapter `JISDORAdapter` (scrape); dev/test adapter `MockAdapter`
- Cache HTML page per fetch session (single HTTP call returns all currencies)

### backend-engineer-go
- Package `backend/internal/fx/` — service, repo, handler, routes, worker
- Worker `fx:jisdor-fetch` (cron) + `fx:upload-process` (after large upload)
- Service methods: `JISDORFetchAll`, `UploadManual`, `ApproveBatch`, `RejectBatch`, `LockRatesForPeriode`, `UnlockRatesForPeriode`, `GetTreatment`
- `LockRatesForPeriode` / `UnlockRatesForPeriode` exported for P5-M4 hard-close hook (must accept `*sql.Tx`)
- Permission keys: `kurs.read`, `kurs.create`, `kurs.approve`, `kurs.reject`, `kurs.jisdor_trigger`, `kurs.export`, `fx_treatment.read`
- Audit + Idempotency-Key + SoD enforced server-side
- Coverage target ≥ 85%

### frontend-engineer-nextjs
- Routes: `/master/kurs` (list with sort/filter/export), `/master/kurs/upload` (form + batch detail), `/master/kurs/jisdor-sync` (admin panel button + history)
- Components: `KursWorkflowBadge` (4-state: PENDING/ACTIVE/REJECTED/LOCKED + icon), `KursDeviationBadge` (warning when deviation_flag), `KursUploadDropzone`, `KursApproveDialog`, `KursRejectDialog` (reason ≥ 30), `JisdorSyncTriggerButton`, `JisdorJobProgressPanel` (JobProgressPanel wrapper), `FxTreatmentBadge` (display per instrumen)
- Notif copy in Bahasa Indonesia (specific per §2)

### qa-engineer
- E2E scenarios for 20 AC + cross-cutting (idempotency, audit, SoD, lock cascade integration with P5-M4)
- Perf benchmarks: JISDOR worker latency, treatment lookup, list with 50k rows
- UAT script in Bahasa Indonesia (Given/When/Then + SQL verify)
- Mock BI JISDOR server fixture for integration tests
