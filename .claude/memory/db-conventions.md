# DB Conventions — PostgreSQL 18 untuk BLIPS

## 9 Schema namespace (+ `rpt` MV)

| Schema | Isi |
|---|---|
| `mst` | Master data (instrumen, counterparty, bank, rating, portofolio) |
| `trx` | Transaction lifecycle (penempatan, MTM, renewal, jatuh tempo) |
| `ecl` | ECL & EIR (calc run, result line, staging history, amortisasi schedule) |
| `sppi` | SPPI test + BM assessment |
| `doc` | Document/media metadata (MinIO refs) |
| `jrnl` | Jurnal & GL interface staging |
| `aud` | Audit log (immutable, hash-chain) |
| `sec` | Security: user shadow, role, permission, session, idempotency |
| `sys` | System config: parameter, FX rate cache, DLQ |
| `rpt` | Materialized views untuk reporting (auto-created) |

## Audit columns wajib di semua tabel (kecuali `aud.*`)
```sql
created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
created_by  UUID        NOT NULL,
updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
updated_by  UUID        NOT NULL,
deleted_at  TIMESTAMPTZ,
deleted_by  UUID,
row_version BIGINT      NOT NULL DEFAULT 1,
tenant_id   TEXT        NOT NULL DEFAULT 'TUGURE'  -- placeholder for future MT
```

Trigger `trg_set_updated_at` + `trg_increment_row_version` BEFORE UPDATE di tiap tabel.

## Tipe data — money & rates
| Use | Type | Example |
|---|---|---|
| IDR amount | `NUMERIC(20,4)` | 1_234_567_890.5000 |
| FCY amount | `NUMERIC(20,4)` | (sebelum konversi ke IDR) |
| FX rate | `NUMERIC(20,8)` | 15_432.12345678 |
| PD / LGD / EIR | `NUMERIC(10,8)` | 0.02350000 (2.35%) |
| Percentage (UI) | `NUMERIC(7,4)` | 2.3500 |

**Never** `FLOAT`, `DOUBLE PRECISION`, atau `REAL` untuk money/rate.

## Tipe data — time
| Use | Type |
|---|---|
| Timestamp event | `TIMESTAMPTZ` (selalu, never `TIMESTAMP`) |
| Date-only business semantics | `DATE` (mis. `tanggal_efektif_eir`, `tanggal_jatuh_tempo`) |
| Period (bulan buku) | `DATE` truncated to `'YYYY-MM-01'` |

## Naming
- Tabel: snake_case, plural rare (BLIPS pakai singular), prefix schema (mis. `mst.instrumen`).
- Kolom: snake_case (mis. `tanggal_penempatan`, `eir_persen`).
- FK: `{referenced_table}_id` (mis. `instrumen_id UUID REFERENCES mst.instrumen(id)`).
- Index: `idx_{table}_{col1}_{col2}` atau `uq_{table}_{col}` untuk unique.
- Constraint: `chk_{table}_{rule}` (mis. `chk_ecl_result_pd_range`).

## Primary keys
- Default: `UUID PRIMARY KEY DEFAULT gen_random_uuid()`.
- Exception: numbering yang business-meaningful (mis. `nomor_urut`, `kode_bank`) bisa pakai TEXT/INT, dengan UUID surrogate tetap.

## Foreign keys
- Wajib di-index. PG **tidak** auto-index FK.
- Default `ON DELETE RESTRICT` (no cascade). Soft-delete only.
- Cross-schema FK boleh, tapi avoid circular.

## Partitioning
Partition by month (`RANGE (created_at)`):
- `trx.transaction`
- `ecl.ecl_calc_result_line`
- `aud.audit_log`

Partition naming: `{parent}_y2026m06` etc. Use `pg_partman` extension.

## Indexes
- FK columns → btree index always.
- `(tenant_id, created_at DESC)` → composite on hot tables.
- Partial `WHERE deleted_at IS NULL` untuk tabel sering di-query active-only.
- `GIN` untuk `JSONB` fields yang di-query (mis. `aud.audit_log.before`).

## Materialized Views (`rpt` schema)
- Naming: `rpt.mv_{report_id}_{slug}`.
- `REFRESH MATERIALIZED VIEW CONCURRENTLY` (butuh unique index).
- Refresh dipicu Asynq job setelah hard-close periode buku.

## Migration discipline (golang-migrate)
- File naming: `db/migrations/{NNNN}_{description}.up.sql` + `.down.sql`.
- Numbering: 4-digit zero-padded, sequential.
- Setiap migration **wajib reversible** (`down.sql` ada dan ditest).
- Dilarang dalam satu migration: drop column TANPA deprecation cycle, change PK type setelah data, add NOT NULL tanpa default + backfill plan.
- Pakai `BEGIN; ... COMMIT;` (transactional DDL — PostgreSQL support).
- Tag setiap migration di top file: `-- migration: {NNNN} {description}` + `-- author: {agent}` + `-- requires: {NNNN}` (deps).

## Hard rules — refuse to violate
1. **No hard delete** di `aud`, `jrnl`, `ecl`. Throw constraint error.
2. **Audit fields wajib** di setiap tabel non-`aud`.
3. **Money** never `FLOAT`.
4. **Time** never `TIMESTAMP` (without TZ).
5. **Migration** always has `down`.
6. **Workflow tables** — `signed_at` + `signature_hash` tracked, never overwriteable.

## Audit log table — kanonik
```sql
CREATE TABLE aud.audit_log (
  event_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_time      TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor_user_id   UUID NOT NULL,
  actor_role      TEXT NOT NULL,
  action          TEXT NOT NULL,         -- e.g. INSTRUMEN.CREATE
  entity_type     TEXT NOT NULL,
  entity_id       UUID NOT NULL,
  before_jsonb    JSONB,                  -- null on INSERT
  after_jsonb     JSONB,                  -- null on DELETE
  ip              INET,
  user_agent      TEXT,
  trace_id        TEXT,
  idempotency_key UUID,
  previous_hash   BYTEA,                  -- hash chain
  current_hash    BYTEA NOT NULL,
  tenant_id       TEXT NOT NULL
) PARTITION BY RANGE (event_time);
```

`current_hash = sha256(previous_hash || canonical_json(row))` — verifikasi periodik di job.
