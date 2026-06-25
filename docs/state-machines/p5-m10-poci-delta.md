# State Machine — P5-M10 POCI Delta ECL

**Module**: APP-C — ECL Engine (POCI Delta)
**Date**: 2026-06-20
**References**: PSAK 71 §5.5.13-14, FSD-APP-C-ECL-EIR-v1.0 §5-6, DEC-010/013/016/017/018

---

## 1. POCI Baseline — WORM State (Write Once Read Many)

```
[No row] ──── CaptureBaseline ────> [CAPTURED] (terminal — immutable forever)
```

`ecl.poci_baseline` has **no state column** because it is append-only. There is exactly
one valid transition: from non-existence to existence. Any subsequent write attempt
raises `POCI_BASELINE_IMMUTABLE_VIOLATION`.

### DB enforcement (defence-in-depth)

```sql
-- Migration 000047 creates this trigger (not service-layer-only)
CREATE OR REPLACE FUNCTION ecl.trg_poci_baseline_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'poci_baseline is append-only (DEC-018). Operation: %, instrumen_id: %',
        TG_OP, OLD.instrumen_id;
END;
$$;

CREATE TRIGGER trg_poci_baseline_no_update_delete
    BEFORE UPDATE OR DELETE ON ecl.poci_baseline
    FOR EACH ROW EXECUTE FUNCTION ecl.trg_poci_baseline_immutable();
```

Service layer ALSO checks before INSERT: if `GetBaselineByInstrumen` returns a row,
return `POCI_BASELINE_IMMUTABLE_VIOLATION` before touching DB. Dual guard.

### When triggered
P5-M1 penempatan approve flow: after `transaksi.penempatan` workflow reaches APPROVED,
within the same DB transaction and before COMMIT, `CaptureBaseline` is called for
instruments where `mst.instrumen.is_poci = TRUE`.

Audit event `POCI.BASELINE_CAPTURED` written in-transaction. If `is_poci = FALSE`,
flow continues normally without baseline insert (no error).

---

## 2. POCI Delta Log — Lifecycle States

```
                ECL calc run RUNNING
                      │
                      ▼ (per POCI instrumen)
              [–– baseline check ––]
               ├─ missing ──────────> error_log POCI_BASELINE_MISSING (run continues)
               └─ found ────────────> compute delta_ecl
                                           │
                                           ▼
                              [COMPUTED] ──┬─ direction = ZERO ──────> [SKIPPED_ZERO]
                                           │
                                           └─ direction ≠ ZERO
                                                    │
                                         ┌──────────▼──────────┐
                                         │   periode OPEN?      │
                                         └──────────────────────┘
                                              │          │
                                           YES          NO
                                              │          └──> [SKIPPED_ZERO] (BLOCKED)
                                              │               status=BLOCKED_PERIODE_CLOSED
                                              ▼               (POCI_PERIODE_LOCKED error)
                              jurnal_poster.PostPociDelta()
                                              │
                                      ┌───────▼────────┐
                                      │ direction check │
                                      └────────────────┘
                                         │          │
                                    MATCH        MISMATCH
                                         │          └──> POCI_JURNAL_DIRECTION_MISMATCH
                                         │               (no INSERT to jrnl.jurnal)
                                         ▼
                                    [POSTED] (terminal per run)
```

### Status enum (ecl.poci_delta_log.status)

| Status | Meaning |
|---|---|
| `COMPUTED` | Delta computed, jurnal not yet posted (transient — rare in production) |
| `POSTED` | Jurnal posted in-transaction, jurnalHeaderId populated |
| `SKIPPED_ZERO` | delta_ecl = 0 (ZERO direction) — no jurnal needed; OR periode locked |

### Idempotency

Partial unique index on `ecl.poci_delta_log (calc_run_id, instrumen_id) WHERE deleted_at IS NULL`
prevents duplicate inserts per (run × instrumen). If Asynq retries the job and the row
already exists, service catches the unique constraint violation and logs
`POCI_DELTA_DUPLICATE` to `ecl.calc_run_error_log` — batch continues to next instrument.

---

## 3. Delta Computation Formula

```
# Per POCI instrumen, per ECL calc run (PSAK 71 §5.5.13-14)

# Step 1 — compute current lifetime ECL (same as non-POCI but PD=Lifetime always)
# Stage engine is BYPASSED for POCI — stage_marker = 'POCI' is set directly.
EAD_IDR          = saldo_pokok + akrual_piutang + (komitmen × CCF)
                   if FCY: × FX_rate_BI_JISDOR(tanggal_assessment) [DEC-016]

current_lifetime_ecl = Σ (ECL_FL_skenario × bobot_skenario)
                     = EAD_IDR × PD_Lifetime_skenario × LGD × FL_multiplier × bobot
                       (Good × 0.25 + Normal × 0.50 + Bad × 0.25 per DEC-010)
                       (ALCO dapat override per periode)

# Step 2 — compute delta (shopspring/decimal, HALF_EVEN, NUMERIC(20,4))
delta_ecl = current_lifetime_ecl − baseline_lifetime_ecl
            [baseline from ecl.poci_baseline.lifetime_ecl_at_origination — immutable]

# Step 3 — direction enum
direction = INCREASE  if delta_ecl.GreaterThan(zero)
          = DECREASE  if delta_ecl.LessThan(zero)
          = ZERO      if delta_ecl.Equal(zero)   [exact decimal comparison]

# Step 4 — cumulative
prior_delta_cumulative = Σ delta_ecl from all prior ecl.poci_delta_log rows
                         for same instrumen_id, ordered by tanggal_compute ASC
```

**Precision rule**: all intermediate values use `shopspring/decimal`. No `float64`.
`delta_ecl` stored as `NUMERIC(20,4)`. Round per `RoundBank(4)` (HALF_EVEN per SoW §4).

---

## 4. Jurnal Sign Convention (PSAK 71 §5.5.14)

| Direction | delta_ecl | Debit | Kredit | Amount |
|---|---|---|---|---|
| `INCREASE` | > 0 (deterioration) | Beban Penurunan Nilai ECL POCI (P&L) | Cadangan ECL POCI (Neraca) | `delta_ecl` |
| `DECREASE` | < 0 (improvement) | Cadangan ECL POCI (Neraca) | Pendapatan Pemulihan ECL POCI (P&L) | `Abs(delta_ecl)` |
| `ZERO` | = 0 | — | — | no jurnal |

Event code: `POCI_ECL_DELTA_INCREASE` (INCREASE) / `POCI_ECL_DELTA_DECREASE` (DECREASE).
Seeded in `mst.mapping_jurnal` as DRAFT rows (migration 000047); akun debit/kredit
filled by ROLE-AKUN in P5-M12.

Jurnal idempotency key: `sha256(calc_run_id + instrumen_id + "POCI_ECL_DELTA")`.
Deduplication in `sys.idempotency_key` (DEC-021).

### Mismatch guard
Before posting, service calls `ValidateJurnalDirection(deltaEcl, direction)`:
- `delta > 0` AND `direction = DECREASE` → `POCI_JURNAL_DIRECTION_MISMATCH` (422)
- `delta < 0` AND `direction = INCREASE` → `POCI_JURNAL_DIRECTION_MISMATCH` (422)
- Alerts: `ROLE-IT-ADMIN` + `ROLE-RISK` via notification channel. Audit
  `POCI.DIRECTION_MISMATCH_DETECTED` written in-transaction.

---

## 5. POCI vs Non-POCI Distinction in ECL Engine

| Aspect | Non-POCI (Stage 1/2/3) | POCI |
|---|---|---|
| Stage | 1 / 2 / 3 (SICR/cure engine) | `'POCI'` marker (string, not int) |
| Staging engine | Called | **BYPASSED** |
| PD used | 12-month (S1) / Lifetime (S2/S3) | Lifetime always |
| P&L booking | Full ECL allowance per period | Delta only (current − baseline) |
| Baseline | None | `ecl.poci_baseline` (immutable) |
| EIR | Standard EIR | Credit-adjusted EIR (DEC-013) |

### Phase 4 integration dependency

P5-M10 introduces a dependency on the Phase 4 ECL staging engine. The engine MUST
skip POCI instruments from the Stage 1/2/3 matrix and return `stage_marker = 'POCI'`
on `ecl.ecl_calc_result_line.stage_marker`.

**If Phase 4 staging does not already implement this gate**, add to staging engine:
```go
if instrumen.IsPOCI {
    resultLine.StageMarker = "POCI"
    // skip staging matrix entirely
    continue
}
```

This follow-up is tracked as `TODO(P5-M10): verify Phase4 POCI gate in
backend/internal/ecl/staging/engine.go`. Do NOT modify Phase 4 in this PR.

### Warning removal (S4)

Search for `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA` in:
- `backend/internal/ecl/` (Phase 4 calc run engine)
- Any `warnings.append(...)` or `Warnings: []string{...}` blocks

If found: remove the warning block, replace with real `delta_ecl` field population
from `ecl.poci_delta_log`. Deprecate the constant with:
```go
// TODO(P5-M10): warning superseded by delta computation — remove constant after M10 deploy
```

Audit one-time: `POCI.WARNING_REMOVED` (entity_type: `ecl.calc_engine`, not per-run).
Pre-M10 result lines retain their legacy warnings (immutable per DEC-018).

---

## 6. Audit Events

| Event | When | Mandatory in-TX? |
|---|---|---|
| `POCI.BASELINE_CAPTURED` | CaptureBaseline succeeds | Yes |
| `POCI.BASELINE_VIOLATION_ATTEMPT` | Duplicate baseline attempt (S1-AC2) | Yes — even on failure |
| `POCI.DELTA_COMPUTED` | delta_log row inserted per instrumen | Yes |
| `POCI.DELTA_POSTED` | Jurnal posted for INCREASE/DECREASE | Yes |
| `POCI.DIRECTION_MISMATCH_DETECTED` | direction vs delta_ecl sign mismatch | Yes |
| `POCI.LARGE_DELTA_ALERT` | `|delta_ecl|` > threshold (once per run×instrumen) | Yes |
| `POCI.WARNING_REMOVED` | One-time on M10 engine update | Yes — one-time only |
| `POCI.EXPORT` | Any export of poci/delta-history | Yes |

---

## 7. Performance SLA

- Delta compute per instrument: < 100ms (excluding jurnal posting I/O)
- Batch throughput: instrument-level errors do not halt batch; per-instrument
  try/catch writes to `ecl.calc_run_error_log` and continues
- Large delta alert: de-duplicated per `(calc_run_id, instrumen_id)` — not per pageload
  (S5-AC3 guard in service layer, not in handler)

---

## 8. Hand-off

| Consumer | Dependency |
|---|---|
| P5-M1 (penempatan approve) | Calls `pocidelta.Service.CaptureBaseline()` in-tx on POCI approve |
| Phase 4 ECL calc run | Must call `pocidelta.Service.ComputeDeltaForCalcRun()` after POCI instruments are processed |
| P5-M2 jurnal engine | `POCI_ECL_DELTA_INCREASE` / `POCI_ECL_DELTA_DECREASE` mapping must exist |
| P5-M9 akrual | `credit_adjusted_eir` from `ecl.amortisasi_schedule` is also used by M10 baseline verification |
| Frontend (P5-M10) | `/poci/delta-history` + `/poci/delta-history/summary` endpoints (DataTable + Recharts) |
