---
name: migration-scaffold
description: Generate golang-migrate up/down migration files dengan konvensi BLIPS (audit columns, namespace, partitioning, indexing). Gunakan saat data-modeler perlu scaffold migration baru.
---

# Migration Scaffold — golang-migrate

## File naming
```
db/migrations/{NNNN}_{description_snake_case}.up.sql
db/migrations/{NNNN}_{description_snake_case}.down.sql
```
- `NNNN`: 4-digit zero-padded, sequential. Cek `ls db/migrations/ | sort -r | head -1` untuk next number.
- `description`: snake_case verb_object, e.g. `0042_add_sppi_test_run_table.up.sql`.

## UP template

```sql
-- migration: {NNNN}_{description}
-- author: data-modeler (BLIPS)
-- requires: {prior_migration_NNNN}
-- description: {one-line summary}

BEGIN;

-- 1. Schema namespace check (idempotent — won't fail if exists)
CREATE SCHEMA IF NOT EXISTS {schema};

-- 2. Table DDL
CREATE TABLE {schema}.{table_name} (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- business columns here
    {col1}       {TYPE} NOT NULL,
    {col2}       {TYPE},
    -- ...

    -- audit columns (wajib di setiap tabel non-`aud`)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by   UUID NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by   UUID NOT NULL,
    deleted_at   TIMESTAMPTZ,
    deleted_by   UUID,
    row_version  BIGINT NOT NULL DEFAULT 1,
    tenant_id    TEXT NOT NULL DEFAULT 'TUGURE'
);

-- 3. Constraints
ALTER TABLE {schema}.{table_name}
    ADD CONSTRAINT chk_{table}_{rule}
    CHECK ({invariant});

-- 4. Foreign keys (wajib di-index)
ALTER TABLE {schema}.{table_name}
    ADD CONSTRAINT fk_{table}_{ref_table}_{ref_col}
    FOREIGN KEY ({ref_col}) REFERENCES {ref_schema}.{ref_table}(id)
    ON DELETE RESTRICT;

-- 5. Indexes
CREATE INDEX idx_{table}_{col} ON {schema}.{table_name} ({col});
CREATE INDEX idx_{table}_tenant_created ON {schema}.{table_name} (tenant_id, created_at DESC);
CREATE INDEX idx_{table}_active ON {schema}.{table_name} ({col}) WHERE deleted_at IS NULL;

-- 6. Triggers (updated_at + row_version auto-increment)
CREATE TRIGGER trg_{table}_set_updated_at
    BEFORE UPDATE ON {schema}.{table_name}
    FOR EACH ROW EXECUTE FUNCTION sys.set_updated_at();

CREATE TRIGGER trg_{table}_increment_version
    BEFORE UPDATE ON {schema}.{table_name}
    FOR EACH ROW EXECUTE FUNCTION sys.increment_row_version();

-- 7. Comments (untuk auto-doc)
COMMENT ON TABLE {schema}.{table_name} IS '{description}';
COMMENT ON COLUMN {schema}.{table_name}.{col1} IS '{semantics}';

COMMIT;
```

## DOWN template
```sql
-- migration: {NNNN}_{description} (DOWN)

BEGIN;

DROP TABLE IF EXISTS {schema}.{table_name} CASCADE;

COMMIT;
```

## Partitioned table template (untuk fact tables)
```sql
-- UP
CREATE TABLE {schema}.{table_name} (
    -- columns ...
    event_time TIMESTAMPTZ NOT NULL,
    -- audit cols
) PARTITION BY RANGE (event_time);

-- Initial partitions (current month + 2 future months)
CREATE TABLE {schema}.{table_name}_y2026m06 PARTITION OF {schema}.{table_name}
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE {schema}.{table_name}_y2026m07 PARTITION OF {schema}.{table_name}
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
-- ...

-- pg_partman config (auto-create future partitions)
SELECT partman.create_parent(
    p_parent_table => '{schema}.{table_name}',
    p_control => 'event_time',
    p_type => 'range',
    p_interval => 'monthly'
);
```

## Materialized view template (`rpt` schema)
```sql
-- UP
CREATE MATERIALIZED VIEW rpt.mv_{report_id}_{slug} AS
SELECT
    ...
FROM {source_schema}.{table}
WHERE deleted_at IS NULL
GROUP BY ...;

-- Unique index required for REFRESH CONCURRENTLY
CREATE UNIQUE INDEX uq_mv_{report_id}_{slug}_pk
    ON rpt.mv_{report_id}_{slug} ({key_columns});

CREATE INDEX idx_mv_{report_id}_{slug}_{col}
    ON rpt.mv_{report_id}_{slug} ({col});

-- Schedule refresh in Asynq cron / hard-close hook
COMMENT ON MATERIALIZED VIEW rpt.mv_{report_id}_{slug}
    IS 'Refreshed by Asynq job post hard-close. See cmd/worker/refresh-mv.go';
```

## ECL/EIR-specific tables (sample)
```sql
-- ecl.ecl_calc_run
CREATE TABLE ecl.ecl_calc_run (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    periode_id      UUID NOT NULL REFERENCES sys.periode_buku(id),
    triggered_by    UUID NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('DRAFT','RUNNING','COMPLETED','SEALED','FAILED')),
    formula_version TEXT NOT NULL,
    sealed_at       TIMESTAMPTZ,
    sealed_by       UUID,
    -- audit cols
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID NOT NULL,
    row_version BIGINT NOT NULL DEFAULT 1,
    tenant_id TEXT NOT NULL DEFAULT 'TUGURE'
);

-- Refuse hard delete (ecl schema rule)
CREATE RULE no_delete_calc_run AS ON DELETE TO ecl.ecl_calc_run DO INSTEAD NOTHING;
-- ATAU lebih baik: hapus DELETE permission di GRANT.
```

## Audit table — no hard delete enforcement
```sql
REVOKE DELETE ON ALL TABLES IN SCHEMA aud FROM PUBLIC;
REVOKE DELETE ON ALL TABLES IN SCHEMA ecl FROM PUBLIC;
REVOKE DELETE ON ALL TABLES IN SCHEMA jrnl FROM PUBLIC;
-- Only granted to a special "archive" role that needs explicit elevation.
```

## Pre-flight checklist (sebelum write migration)
- [ ] Sudah Read `BLIPS_init_schema.sql` + migration terakhir?
- [ ] Schema namespace correct? (`mst|trx|ecl|sppi|doc|jrnl|aud|sec|sys|rpt`)
- [ ] Audit cols included (kecuali `aud.*`)?
- [ ] Money fields pakai `NUMERIC` bukan `FLOAT`?
- [ ] FK punya index?
- [ ] Down migration ada dan tested?
- [ ] Existing data: ada migration step untuk backfill jika NOT NULL ditambah?
- [ ] Partition strategy for fact tables?
- [ ] Triggers untuk updated_at + row_version?
- [ ] Comment di table + key columns?

## Pre-flight checklist (migration yang merubah existing tabel)
- [ ] Bisa run dengan ZERO downtime? (PG18 mostly OK untuk ADD COLUMN, ADD INDEX CONCURRENTLY)
- [ ] Backfill plan jika ada NOT NULL baru?
- [ ] Deprecation cycle: 2 release cycles sebelum DROP column?
- [ ] Rollback path realistic? (DROP COLUMN bisa di-restore dari backup, tapi data hilang)
- [ ] Stakeholder approval (orchestrator + DBA)?

## Citation
- `BLIPS_init_schema.sql` — schema baseline
- `ERD-BLIPS-IFRS9-v1.2.docx` — relationship reference
- @.claude/memory/db-conventions.md
