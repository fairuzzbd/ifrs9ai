# P5-M9 State Machines — Jatuh Tempo + Pendapatan Akrual

**Author**: backend-engineer-go (P5-M9)
**Tanggal**: 2026-06-20
**Linked stories**: P5-M9-S1..S5

---

## 1. trx.pendapatan_akrual.status

```
                  ┌──────────────────────────────────────────┐
                  │         DAILY_ACCRUAL_JOB berjalan       │
                  │         AMORTISASI_PD_JOB berjalan       │
                  └────────────────┬─────────────────────────┘
                                   │
                      ┌────────────▼──────────────┐
               ┌─────►│    Cek Stage + ECL run     │
               │      └──┬────────────────────┬───┘
               │         │                    │
               │  Stage 1/2, ECL OK       Stage 3, NO sealed ECL
               │  OR Stage 1/2            OR stale > DAYS
               │         │                    │
               │         ▼                    ▼
               │   ┌───────────┐    ┌──────────────────────┐
               │   │AUTO_POSTED│    │ PENDING_STALE_REVIEW  │◄──── DLQ alert
               │   └─────┬─────┘    └──────────┬───────────┘
               │         │                     │
               │    (terminal for              │ ROLE-AKUN-CTL
               │    normal cron)              │ POST /override-stale
               │                              │ reason ≥ 30 char
               │                              │ signatureMethod=JWT_STEP_UP
               │                              ▼
               │                   ┌──────────────────────┐
               │                   │  OVERRIDE_APPROVED   │
               │                   └──────────┬───────────┘
               │                              │ recompute + jurnal post
               │                              ▼
               │                         ┌────────┐
               └─────────────────────────│ POSTED │
                                         └────────┘

     SKIPPED ──── FVTPL instrumen, holiday, AC bond at par (no accrual edge),
                  duplicate (idempotency guard catches → DLQ AKRUAL_DUPLICATE)
```

### Status Transitions

| From | To | Trigger | Actor | Condition |
|---|---|---|---|---|
| (new) | AUTO_POSTED | cron sukses | System | Stage 1/2 OR Stage 3 dengan sealed ECL, jurnal terposting |
| (new) | PENDING_STALE_REVIEW | cron, no sealed ECL | System | Stage 3, ecl.calc_result_line tidak ada sealed run, OR stale > AKRUAL_STAGING_STALE_DAYS |
| PENDING_STALE_REVIEW | OVERRIDE_APPROVED | POST /override-stale | ROLE-AKUN-CTL | reason ≥ 30 char, JWT_STEP_UP |
| OVERRIDE_APPROVED | POSTED | recompute + jurnal post | System (in-tx) | Atomic jurnal posting sukses |
| (new) | SKIPPED | cron, instrumen FVTPL / holiday / idempotency dup | System | Per business rule |

### Audit events
- `AKRUAL.POSTED` — setiap AUTO_POSTED, before=null, after={instrumen_id, stage, basis, eir, akrual_idr, ecl_run_id}
- `AKRUAL.POSTED_OVERRIDE` — OVERRIDE_APPROVED → POSTED, after={override_by, reason, akrual_idr_recomputed}
- `STAGING_STALE_ALERT` — 1x per (instrumen, hari), bukan per pageload, after={instrumen_id, days_stale, last_sealed_at}
- `AKRUAL.SKIPPED` — holiday / FVTPL / dup

---

## 2. trx.jatuh_tempo.status

```
                  ┌───────────────────────────────────────┐
                  │  MATURITY_PROCESS_JOB 09:00 WIB       │
                  │  per instrumen ACTIVE + jatuh_tempo=today │
                  └──────────────────┬────────────────────┘
                                     │
                             ┌───────▼────────┐
                             │    PENDING      │
                             └──┬──────────┬──┘
                                │          │
                     maturity   │          │ error per instrumen
                     settlement │          │ (jurnal gagal, EIR hilang, etc.)
                     sukses     │          │
                                ▼          ▼
                          ┌──────────┐  ┌────────┐
                          │ SETTLED  │  │ FAILED │──► sys.dlq
                          └──────────┘  └────────┘
                                              │
                                    instrumen lain lanjut (tidak halt)

     SKIPPED ──── hari libur (MATURITY.HOLIDAY_SKIP audit event, tanpa tx)
```

### Status Transitions

| From | To | Trigger | Condition |
|---|---|---|---|
| (new) | PENDING | cron create row | tanggal_jatuh_tempo = today, status ACTIVE |
| PENDING | SETTLED | maturity posting sukses | jurnal MATURITY_SETTLEMENT terposting, instrumen → MATURED |
| PENDING | FAILED | error | error → DLQ; instrumen tidak di-MATURED |

### Audit events
- `MATURITY.DERECOGNIZED` — in-tx, after={instrumen_id, pokok_idr, bunga_last_idr, pph_idr, net_kas_idr, jenis}
- `MATURITY.HOLIDAY_SKIP` — informatif, tanpa buka tx, action logged dengan tanggal

---

## 3. trx.dividen.status (S3 — 4-eyes)

```
     ROLE-MAKER-TR                ROLE-APPR-TR (≠ maker)
          │                              │
     POST /trx/dividen           POST /trx/dividen/{id}/approve
          │                              │
          ▼                              ▼
  PENDING_APPROVAL ──────────────► APPROVED → POSTED (atomic)
          │
          └──── ROLE-APPR-TR reject → REJECTED
```

### Audit events
- `DIVIDEN.CREATED` — after submit
- `DIVIDEN.APPROVED` / `DIVIDEN.POSTED` — in-tx saat approve
- `DIVIDEN.REJECTED` — saat reject

---

## 4. Cron Flows

### 4.1 MATURITY_PROCESS_JOB — 09:00 WIB

```
START
  ├─ Is holiday? (sys.holiday_calendar) → YES: log MATURITY.HOLIDAY_SKIP, EXIT
  ├─ Is periode OPEN? → NO: DLQ AKRUAL_PERIODE_LOCKED per batch, EXIT
  └─ For each instrumen WHERE tanggal_jatuh_tempo = today AND status = 'ACTIVE':
       ├─ Get amortized carrying + akrual last (trx.pendapatan_akrual terkini)
       ├─ Compute PPh (DEPOSITO: 20%, BOND: varies, REKSADANA: N/A)
       ├─ BEGIN TX
       │   ├─ INSERT trx.jatuh_tempo (PENDING)
       │   ├─ POST jurnal MATURITY_SETTLEMENT via JurnalPoster
       │   ├─ UPDATE mst.instrumen.status = 'MATURED'
       │   ├─ UPDATE trx.jatuh_tempo.status = 'SETTLED'
       │   └─ WRITE aud.audit_log MATURITY.DERECOGNIZED
       ├─ COMMIT
       └─ ON ERROR: ROLLBACK → UPDATE trx.jatuh_tempo.status = 'FAILED' → sys.dlq INSERT

DONE — notify ROLE-AKUN (summary), ROLE-IT-ADMIN (DLQ count if any)
```

### 4.2 DAILY_ACCRUAL_JOB — 09:15 WIB

```
START
  ├─ Is holiday? → YES: EXIT
  ├─ Is periode OPEN? → NO: DLQ AKRUAL_PERIODE_LOCKED, EXIT
  └─ For each instrumen ACTIVE WHERE klasifikasi_psak71 ≠ 'FVTPL':
       ├─ Is (instrumen_id, today, 'BUNGA') unique? → NO: DLQ AKRUAL_DUPLICATE, skip
       ├─ Get EIR from ecl.amortisasi_schedule (latest active, effective_to=infinity)
       │   → MISS: DLQ AKRUAL_EIR_NOT_FOUND, skip
       ├─ Get FX rate APPROVED if FCY
       │   → MISS: DLQ AKRUAL_FX_RATE_MISSING, skip
       ├─ Get stage from ecl.staging_history (latest)
       ├─ If Stage 3:
       │   ├─ Get sealed ECL from ecl.calc_result_line (latest sealed run)
       │   │   → MISS: status = PENDING_STALE_REVIEW; DLQ AKRUAL_STAGING_STALE; skip jurnal
       │   └─ net_carrying = max(gross - ecl, 0)
       ├─ ComputeAkrualBunga(stage, gross, ecl, eir, 365)
       ├─ BEGIN TX
       │   ├─ INSERT trx.pendapatan_akrual (AUTO_POSTED or PENDING_STALE_REVIEW)
       │   ├─ POST jurnal AKRUAL_BUNGA or AKRUAL_BUNGA_STAGE3 (skip if STALE)
       │   └─ WRITE aud.audit_log AKRUAL.POSTED
       └─ COMMIT (or ROLLBACK → DLQ)
```

### 4.3 AMORTISASI_PD_JOB — 10:00 WIB

```
START
  ├─ Is holiday? → YES: EXIT
  └─ For each instrumen ACTIVE WHERE klasifikasi_psak71 IN ('AC', 'FVOCI'):
       ├─ Get amortisasi_schedule entry for today (schedule_version DESC, effective_to=infinity)
       │   → MISS: DLQ AKRUAL_EIR_NOT_FOUND, skip
       ├─ Is POCI? → use credit_adjusted_eir from POCI schedule version
       ├─ Is (instrumen_id, today, 'AMORTISASI_PREMIUM'/'AMORTISASI_DISKON') unique? → skip dup
       ├─ ComputeAmortisasi(scheduleRow, prevCarrying)
       ├─ BEGIN TX
       │   ├─ INSERT trx.pendapatan_akrual (jenis=AMORTISASI_*)
       │   ├─ POST jurnal AMORTISASI_PD (NEVER UPDATE ecl.amortisasi_schedule — DEC-013)
       │   └─ WRITE aud.audit_log AMORTISASI.POSTED
       └─ COMMIT
```

---

## 5. ECL Sealed Run for Stage 3 (M8 B1 pattern)

```sql
-- Pattern untuk get ECL dari sealed run terbaru (sama dengan M8 penjualan/repo.go)
SELECT crl.ecl_stage, crl.ecl_allowance
FROM ecl.calc_result_line crl
JOIN ecl.ecl_calc_run run ON run.id = crl.ecl_calc_run_id
WHERE crl.instrumen_id = $1
  AND run.sealed_at IS NOT NULL
  AND run.deleted_at IS NULL
  AND crl.deleted_at IS NULL
ORDER BY run.sealed_at DESC, run.created_at DESC
LIMIT 1
```

Staleness check: `NOW() - run.sealed_at > AKRUAL_STAGING_STALE_DAYS` → PENDING_STALE_REVIEW.

---

## 6. Decimal Precision (DEC-013, DEC-016)

| Nilai | Type | Rounding |
|---|---|---|
| IDR amounts | NUMERIC(20,4) | HALF_EVEN |
| EIR | NUMERIC(10,8) | — |
| FX rate | NUMERIC(20,8) | — |
| PPh pct | NUMERIC(7,4) | — |

Formula: `akrual = carrying × eir / 365` (integer 365, not float).
Stage 3: `net_carrying = max(gross - ecl_allowance, 0)` — clamp at zero.

---

## 7. POCI credit-adjusted EIR

POCI instrumen mempunyai versi khusus di `ecl.amortisasi_schedule` dengan flag `is_poci=TRUE`.
Credit-adjusted EIR lebih rendah dari gross EIR karena cashflow sudah PD-adjusted.
Amortisasi cron mengambil schedule dengan `is_poci=TRUE` untuk POCI instrumen.

---

## 8. FX Multi-currency

```
akrual_FCY = gross_FCY × eir / 365
akrual_IDR = akrual_FCY × fx_rate_APPROVED(tanggal_akrual)
```

FX rate diambil dari `sys.fx_rate` dengan `status = 'APPROVED'` dan `tanggal = tanggal_akrual`.
Missing → DLQ AKRUAL_FX_RATE_MISSING; IDR instrumen tidak terdampak.

---

## 9. Periode Lock

`mst.periode_buku.status_periode = 'OPEN'` required sebelum insert.
CLOSED → DLQ AKRUAL_PERIODE_LOCKED per seluruh batch untuk periode tersebut.

---

## 10. Idempotency

Partial unique index pada `trx.pendapatan_akrual (instrumen_id, tanggal_akrual, jenis) WHERE deleted_at IS NULL`.
Cron cek sebelum INSERT. Collision → DLQ AKRUAL_DUPLICATE; instrumen lain lanjut.
Manual endpoints: Idempotency-Key header wajib di POST /override-stale + POST /trx/dividen*.

---

## 11. Performance SLA

- MATURITY_PROCESS_JOB ≤ 5 menit untuk 1000 instrumen (P95)
- DAILY_ACCRUAL_JOB ≤ 5 menit untuk 1000 instrumen (P95)
- Read endpoints ≤ 200ms P95
- Batch chunked 100 instrumen per iteration; progress via Asynq inspector

---

## 12. Hand-off

- `ifrs9-compliance-reviewer` BLOCKING: Stage 3 §5.4.1(b) net carrying; POCI credit-adjusted EIR; FVOCI Election dividen OCI vs P&L; DEC-013 amortisasi immutability
- `security-engineer` BLOCKING: audit in-tx semua cron events; service account actor_user_id di audit_log; SoD dividen maker≠approver; DLQ permission ROLE-IT-ADMIN only
- `qa-engineer`: happy path cron + holiday skip + DLQ path + stale override + SoD dividen + idempotency dup
