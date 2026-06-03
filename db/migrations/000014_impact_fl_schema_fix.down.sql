-- migration: 0014 impact_fl_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.impact_pd changes, then mst.impact_mev_pd changes
--              (reverse order of up.sql sections).
--              Legacy maker_id / approver_id columns are left intact (owned by 0001).

BEGIN;

-- ============================================================
-- SECTION 2 REVERT: mst.impact_pd
-- ============================================================

-- 2e. Drop indexes
DROP INDEX IF EXISTS mst.idx_impact_pd_workflow_status;
DROP INDEX IF EXISTS mst.idx_impact_pd_tenant_created;

-- 2c. Drop CHECK constraint + column
ALTER TABLE mst.impact_pd
    DROP CONSTRAINT IF EXISTS chk_impact_pd_workflow_status;

ALTER TABLE mst.impact_pd
    DROP COLUMN IF EXISTS workflow_status;

-- 2b. Drop audit columns
ALTER TABLE mst.impact_pd
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- 2a. Revert impact_multiplier precision: NUMERIC(10,8) → NUMERIC(8,4)
--     Values outside NUMERIC(8,4) representable range are truncated — acceptable
--     because the CHECK constraint (BETWEEN 0.5 AND 2.0) bounds all values within range.
ALTER TABLE mst.impact_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(8,4)
        USING impact_multiplier::NUMERIC(8,4);

-- ============================================================
-- SECTION 1 REVERT: mst.impact_mev_pd
-- ============================================================

-- 1e. Drop indexes
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_periode_skenario;
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_workflow_status;
DROP INDEX IF EXISTS mst.idx_impact_mev_pd_tenant_created;

-- 1c. Drop CHECK constraint + column
ALTER TABLE mst.impact_mev_pd
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_workflow_status;

ALTER TABLE mst.impact_mev_pd
    DROP COLUMN IF EXISTS workflow_status;

-- 1b. Drop audit columns
ALTER TABLE mst.impact_mev_pd
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- 1a. Revert impact_multiplier precision: NUMERIC(10,8) → NUMERIC(8,4)
ALTER TABLE mst.impact_mev_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(8,4)
        USING impact_multiplier::NUMERIC(8,4);

COMMIT;
