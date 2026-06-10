-- migration: 0015 impact_fl_multiplier_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert all changes made by 0015 up.sql.
--              Drops indexes, CHECK constraints, audit cols, workflow cols.
--              Reverts impact_multiplier precision NUMERIC(10,8) → NUMERIC(8,4).
--
--   NOTE: WORKFLOW_CONFIG_IMPACT_MEV_PD + WORKFLOW_CONFIG_IMPACT_PD were seeded
--         in 0008 — they are NOT removed here (owned by 0008 down.sql).
--
--   NOTE: ck_impact_pd_range (BETWEEN 0.5 AND 2.0) was created in 0001 —
--         it is NOT dropped here (owned by 0001 down.sql).
--
--   NOTE: ck_impact_skenario (IN 'GOOD','BAD') was created in 0001 —
--         it is NOT dropped here (owned by 0001 down.sql).
--
--   RISK: Narrowing cast NUMERIC(10,8) → NUMERIC(8,4) may truncate values
--         with more than 4 decimal places. In practice, Phase 3 initial data
--         is entered via UI with ≤ 4 decimal places. Explicit USING clause
--         included to surface any truncation as a migration error rather than
--         silent data loss.

BEGIN;

-- ====================================================================
-- PART A — mst.impact_mev_pd (reverse order: indexes → constraints → cols → precision)
-- ====================================================================

-- A6. Drop indexes added by this migration
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_active;
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_workflow_status;
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_tenant_created;

-- A3. Drop CHECK constraints added by this migration
ALTER TABLE mst.impact_mev_pd
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_multiplier_positive,
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_workflow_status;

-- A2. Drop audit + workflow columns added by this migration
ALTER TABLE mst.impact_mev_pd
    DROP COLUMN IF EXISTS workflow_instance_id,
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- A1. Revert precision NUMERIC(10,8) → NUMERIC(8,4)
--     Explicit USING cast — will ERROR if any value has > 4 significant decimals
--     (expected to be zero rows at rollback time in UAT; review before prod rollback).
ALTER TABLE mst.impact_mev_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(8,4)
        USING impact_multiplier::NUMERIC(8,4);

-- ====================================================================
-- PART B — mst.impact_pd (reverse order)
-- ====================================================================

-- B6. Drop indexes added by this migration
DROP INDEX IF EXISTS mst.idx_impact_pd_active;
DROP INDEX IF EXISTS mst.idx_impact_pd_workflow_status;
DROP INDEX IF EXISTS mst.idx_impact_pd_tenant_created;

-- B3. Drop CHECK constraint added by this migration
ALTER TABLE mst.impact_pd
    DROP CONSTRAINT IF EXISTS chk_impact_pd_workflow_status;

-- B2. Drop audit + workflow columns added by this migration
ALTER TABLE mst.impact_pd
    DROP COLUMN IF EXISTS workflow_instance_id,
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- B1. Revert precision NUMERIC(10,8) → NUMERIC(8,4)
--     Same truncation risk note as Part A above.
ALTER TABLE mst.impact_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(8,4)
        USING impact_multiplier::NUMERIC(8,4);

COMMIT;
