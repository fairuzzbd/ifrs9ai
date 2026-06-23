# P5-M7 State Machine — Renewal Deposito

**Module**: APP-B — Transaction Lifecycle (Renewal Deposito)
**Author**: system-analyst (driven by business-analyst P5-M7 story set)
**Date**: 2026-06-19
**Status**: READY FOR backend-engineer-go implementation

---

## State Machine `trx.renewal.status`

```
[PENDING_APPROVAL]
       │
       ├─ [approve] ROLE-APPR-TR (approver_id ≠ maker_id, signatureMethod=JWT_STEP_UP)
       │     Side-effects (atomic, single DB tx):
       │       → APPROVED (interim)
       │       → INSERT instrumen baru + UPDATE instrumen lama (MATURED)
       │       → INSERT ecl.amortisasi_schedule (EIR baru)
       │       → UPDATE ecl.amortisasi_schedule lama (effective_to)
       │       → POST jurnal RENEWAL_DEPOSITO (P5-M2)
       │     → POSTED  ← (final state after all side-effects succeed)
       │
       └─ [reject] ROLE-APPR-TR (comment ≥ 30 char)
             → REJECTED  ← (terminal)
```

Notes:
- `DRAFT` state removed from implementation: create endpoint immediately sets `PENDING_APPROVAL`.
  The story mentions DRAFT as a conceptual state but there is no separate submit step per endpoint spec.
- `POSTED` is immutable: no further transitions.
- `REJECTED` is terminal: no reopen in this sprint (story mentions reopen but it is deferred).

### Valid transitions

| From | Action | To | Guard |
|---|---|---|---|
| PENDING_APPROVAL | approve | POSTED | SoD + signatureMethod + periode OPEN + EIR convergence |
| PENDING_APPROVAL | reject | REJECTED | SoD + comment ≥ 30 char + signatureMethod |

---

## Validation Rules

### Create (S1)

| Field | Rule | Error Code | HTTP |
|---|---|---|---|
| instrumen_id | jenis=DEPOSITO, status=ACTIVE, klasifikasi_locked=TRUE | RENEWAL_INSTRUMEN_NOT_ELIGIBLE | 422 |
| instrumen_id | no active renewal (PENDING_APPROVAL/APPROVED/POSTED) | RENEWAL_INSTRUMEN_NOT_ELIGIBLE | 422 |
| skema | IN ('POKOK_SAJA', 'POKOK_PLUS_BUNGA') | RENEWAL_SKEMA_INVALID | 400 |
| tenor_baru_bulan | 1 ≤ x ≤ 60 | RENEWAL_TENOR_OUT_OF_RANGE | 400 |
| rate_baru_persen | 0.00 ≤ x ≤ 30.00 | RENEWAL_RATE_OUT_OF_RANGE | 400 |
| tanggal_efektif_baru | periode_buku OPEN, valid date | PERIODE_CLOSED / VALIDATION_FAILED | 423 / 400 |
| bunga_bersih (POKOK_PLUS_BUNGA) | ≥ IDR 100,000 | RENEWAL_BUNGA_BERSIH_TOO_SMALL | 422 |

### Approve (S2)

| Check | Rule | Error Code | HTTP |
|---|---|---|---|
| SoD | approver_id ≠ maker_id | SOD_VIOLATION | 403 |
| signatureMethod | == "JWT_STEP_UP" | VALIDATION_FAILED | 400 |
| status | == PENDING_APPROVAL | WORKFLOW_INVALID_TRANSITION | 422 |
| periode_buku | status = OPEN at posting time | PERIODE_CLOSED | 423 |
| PPh re-verify | server_pph == stored_pph (tolerance 0.01 IDR) | RENEWAL_PPH_CALC_MISMATCH | 422 |
| bunga_bersih (POKOK_PLUS_BUNGA) | ≥ IDR 100,000 (server re-verify) | RENEWAL_BUNGA_BERSIH_TOO_SMALL | 422 |
| EIR convergence | Newton-Raphson converges ≤ 100 iter | INTERNAL (rollback) | 500 |

### Reject (S2)

| Check | Rule | Error Code | HTTP |
|---|---|---|---|
| SoD | approver_id ≠ maker_id | SOD_VIOLATION | 403 |
| status | == PENDING_APPROVAL | WORKFLOW_INVALID_TRANSITION | 422 |
| comment | len ≥ 30 char | VALIDATION_FAILED | 400 |
| signatureMethod | == "JWT_STEP_UP" | VALIDATION_FAILED | 400 |

---

## Kalkulasi Preview

### Bunga Akrual

```
hari_berjalan = (tanggal_efektif_baru - tanggal_penempatan_lama).Days()
bunga_kotor   = pokok_lama × (rate_lama / 100) × (hari_berjalan / 365)
                [shopspring/decimal, HALF_EVEN rounding, 4 decimal places]

PPh_20pct     = bunga_kotor × Decimal("0.20")   [PP No. 131/2000]
bunga_bersih  = bunga_kotor - PPh_20pct
```

### Pokok Baru

```
IF skema == "POKOK_SAJA":
    pokok_baru = pokok_lama

IF skema == "POKOK_PLUS_BUNGA":
    pokok_baru = pokok_lama + bunga_bersih
    REQUIRE: bunga_bersih >= Decimal("100000") [minimum IDR 100.000, BRD §6.2]
```

### EIR Re-computation (S4 — Newton-Raphson, DEC-013 LOCKED)

Cashflow array untuk instrumen baru (monthly periods):

```
kupon_kotor_per_bulan = pokok_baru × (rate_baru_persen / 100 / 12)
kupon_bersih_per_bulan = kupon_kotor_per_bulan × (1 - 0.20)  [after PPh 20%]

cashflows[0]            = -pokok_baru                              [outflow t=0]
cashflows[1..n-1]       = +kupon_bersih_per_bulan                 [bulan 1 s.d. n-1]
cashflows[n]            = +pokok_baru + kupon_bersih_per_bulan    [terminal: pokok + kupon]
n = tenor_baru_bulan
```

Newton-Raphson IRR solver:

```
Cari r sehingga: Σ CF_t / (1+r)^t = 0

r_{k+1} = r_k − f(r_k) / f'(r_k)

f(r)  = Σ CF_t / (1+r)^t
f'(r) = -Σ t × CF_t / (1+r)^(t+1)

Params (DEC-013):
  r_initial  = rate_baru_persen / 100 / 12  [seed: coupon rate]
  tolerance  = 1e-10
  max_iter   = 100
  fail-safe  = RENEWAL_EIR_NO_CONVERGENCE → explicit error, rollback
```

Compliance (PSAK 71 §5.4.3): EIR reflects after-tax yield (bunga bersih setelah PPh).
Schedule stored: NUMERIC(10,8), presisi 8 desimal.

---

## Jurnal Multi-Leg `RENEWAL_DEPOSITO`

Event code diregistrasi di `mst.mapping_jurnal` sebelum approve pertama kali.

| Leg | Dr | Cr | Nilai | Keterangan |
|---|---|---|---|---|
| 1 | Kewajiban PPh Deposito | Kas/Rekening Bank | PPh_20pct | Setoran PPh final |
| 2 | Deposito (lama, instrumen_lama_id) | Kas/Rekening Bank | pokok_lama | Pelunasan pokok lama |
| 3 | Kas/Rekening Bank | Deposito (baru, instrumen_baru_id) | pokok_baru | Penempatan pokok baru |
| 4 | Beban Bunga Deposito | Kas/Rekening Bank | bunga_bersih | Bunga bersih diterima |

- POKOK_SAJA: leg 3 = pokok_lama (= pokok_baru), leg 4 = bunga_bersih (dikreditkan terpisah).
- POKOK_PLUS_BUNGA: leg 3 = pokok_lama + bunga_bersih.
- Semua nilai NUMERIC(20,4), shopspring/decimal — never float64.

---

## Approve Side-Effects (Single DB Transaction)

Sequence dalam satu `*sql.Tx`:

1. Load `trx.renewal` (SELECT FOR UPDATE, row_version check)
2. Validate status, SoD, PPh re-verify, bunga_bersih (if POKOK_PLUS_BUNGA)
3. `UPDATE trx.renewal SET status='APPROVED', approver_id, approve_reason, signature_method, signature_hash_meta, updated_at, row_version+1`
4. `INSERT mst.instrumen` baru (kode auto-generated, inherit fields dari S3 spec)
5. `UPDATE mst.instrumen SET status='MATURED'` pada instrumen lama
6. Compute EIR_baru via Newton-Raphson (in-process, no DB yet)
7. `INSERT ecl.amortisasi_schedule` (instrumen_baru_id, schedule_version=1, eir=EIR_baru, effective_from=tanggal_efektif_baru, effective_to=INFINITY)
8. `UPDATE ecl.amortisasi_schedule SET effective_to=tanggal_efektif_baru` WHERE instrumen_id=instrumen_lama_id AND effective_to='infinity' — NEVER update EIR value
9. Call `JurnalPoster.Post(ctx, tx, PostRequest{EventCode: "RENEWAL_DEPOSITO", ...})` → get jurnal_header_id
10. `UPDATE trx.renewal SET status='POSTED', instrumen_baru_id, jurnal_header_id`
11. `INSERT aud.audit_log` multiple events: RENEWAL.APPROVED, RENEWAL.POSTED, INSTRUMEN.CREATED, INSTRUMEN.MATURED, EIR.RECOMPUTED
12. `COMMIT`

Rollback on any step failure → 500 INTERNAL. Caller gets error; `trx.renewal` status reverts to PENDING_APPROVAL.

---

## Instrumen Baru — Inherit Rules (S3)

| Field | Source | Value |
|---|---|---|
| klasifikasi_psak71 | instrumen lama | inherit (AC) |
| klasifikasi_locked | instrumen lama | TRUE |
| sppi_result | instrumen lama | copy FK |
| bm_assessment_id | instrumen lama | copy FK |
| portofolio_id | instrumen lama | copy |
| counterparty_id | instrumen lama | copy |
| mata_uang | instrumen lama | copy (IDR) |
| jenis_instrumen | instrumen lama | DEPOSITO |
| pokok | computed | pokok_baru dari preview |
| rate_persen | renewal request | rate_baru_persen |
| tanggal_penempatan | renewal request | tanggal_efektif_baru |
| tanggal_jatuh_tempo | computed | tanggal_efektif_baru + tenor_baru_bulan |
| status | system | ACTIVE |
| renewal_dari_instrumen_id | instrumen lama | instrumen_lama_id (traceability) |
| kode_instrumen | auto-generated | {prefix}-{seq} (same prefix, new seq) |

Renewal is NOT a reclassification event (PSAK 71 §4.4.1). No SPPI re-test.

---

## EIR Schedule Versioning — Immutability Rule (PSAK 71 §B5.4.6)

```sql
-- INSERT new schedule for instrumen baru (schedule_version starts at 1 for new instrumen):
INSERT INTO ecl.amortisasi_schedule (instrumen_id, schedule_version, eir_persen,
    effective_from, effective_to, ...)
VALUES (instrumen_baru_id, 1, eir_baru, tanggal_efektif_baru, 'infinity', ...);

-- UPDATE old schedule effective_to only (NEVER update EIR value of old row):
UPDATE ecl.amortisasi_schedule
SET effective_to = tanggal_efektif_baru, updated_at = now(), updated_by = actor_id
WHERE instrumen_id = instrumen_lama_id
  AND effective_to = 'infinity';
```

Invariant: `eir_persen` column in any existing row is NEVER modified after INSERT.

---

## Periode Lock

Before jurnal posting step (step 9 above):
- Service calls `PeriodeChecker.IsOpen(ctx, tanggalEfektifBaru)`.
- If closed: return `PERIODE_CLOSED` error → full rollback (S5-AC3).

---

## Audit Events (all in-transaction unless advisory)

| Action | Trigger | In-tx? |
|---|---|---|
| RENEWAL.CREATED | INSERT trx.renewal | Yes |
| RENEWAL.APPROVED | UPDATE status=APPROVED | Yes |
| RENEWAL.POSTED | UPDATE status=POSTED | Yes |
| RENEWAL.REJECTED | UPDATE status=REJECTED | Yes |
| INSTRUMEN.CREATED | INSERT mst.instrumen baru | Yes |
| INSTRUMEN.MATURED | UPDATE instrumen lama status=MATURED | Yes |
| EIR.RECOMPUTED | INSERT ecl.amortisasi_schedule | Yes |
| RENEWAL.SOD_VIOLATION_ATTEMPT | SoD rejected | Yes (advisory) |
| RENEWAL.EIR_COMPUTE_FAILED | Newton-Raphson no convergence | Advisory (after rollback) |
| RENEWAL.JURNAL_MISSING_CONFIG | Event code not in mapping | Advisory (after rollback) |
| RENEWAL.EXPORT | GET /trx/renewal?export=csv | Yes |

Audit `Before` / `After` fields:
- `RENEWAL.CREATED.After`: `{instrumen_id, skema, tenor_baru_bulan, rate_baru_persen, pokok_baru, eir_baru, tanggal_efektif_baru}`
- `RENEWAL.POSTED.After`: `{status, instrumen_baru_id, jurnal_header_id, eir_baru}`
- `INSTRUMEN.CREATED.After`: `{kode_instrumen_baru, pokok_baru, rate_persen, tanggal_jatuh_tempo_baru, renewal_dari_instrumen_id}`
- `EIR.RECOMPUTED.After`: `{instrumen_baru_id, schedule_version, eir_baru, effective_from}`

---

## Performance SLA

| Endpoint | Target |
|---|---|
| POST /trx/renewal | ≤ 500 ms |
| GET /trx/renewal (list) | ≤ 200 ms (cursor, indexed) |
| GET /trx/renewal/{id} | ≤ 100 ms |
| POST /trx/renewal/{id}/approve | ≤ 1 s (multi-step in-tx) |
| POST /trx/renewal/{id}/reject | ≤ 300 ms |
| GET /trx/renewal/{id}/preview | ≤ 200 ms |

---

## Hand-off

**backend-engineer-go**: implement `backend/internal/trx/renewal/` per this spec.
- EIR Newton-Raphson: `eir.go` (isolated, 100% test coverage, tolerance 1e-10 / max 100 iter).
- CalcPreview: `calc.go` (pure functions, 100% test coverage).
- JurnalPoster interface: mirror `backend/internal/trx/mtm/jurnal_poster.go` pattern.
- InstrumenCreator interface: abstract `mst.instrumen` insert for testability.
- Migration: `db/migrations/000043_renewal_p5m7.up.sql` + `.down.sql`.

**ifrs9-compliance-reviewer**: BLOCKING gate on S4 (EIR after-PPh cashflow) and S5 (jurnal leg mapping).
**security-engineer**: BLOCKING gate on S2 (SoD + audit in-transaction).
