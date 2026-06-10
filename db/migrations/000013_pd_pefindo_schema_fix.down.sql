-- migration: 0013 pd_pefindo_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.pd_pefindo precision fix, audit cols, workflow_status,
--              lifetime CHECK constraint, and new indexes added by 0013.
--              Legacy columns uploaded_by, approved_by, uploaded_at, approved_at
--              and the 0001 indexes (ix_pd_pefindo_rating_periode,
--              ix_pd_pefindo_current) are NOT touched.

BEGIN;

-- ============================================================
-- 1. Drop indexes added by 0013
-- ============================================================

DROP INDEX IF EXISTS mst.idx_pd_pefindo_active_rating;
DROP INDEX IF EXISTS mst.idx_pd_pefindo_workflow_status;
DROP INDEX IF EXISTS mst.idx_pd_pefindo_tenant_created;

-- ============================================================
-- 2. Drop CHECK constraints added by 0013
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_pd_lifetime_ranges;

-- ============================================================
-- 3. Drop columns added by 0013
--    Order: workflow_status first (has CHECK dependency), then audit cols.
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- ============================================================
-- 4. PRECISION REVERT — NUMERIC(10,8) → NUMERIC(8,4)
--    Risk: if any value stored has more than 4 decimal places,
--    this ALTER will raise a numeric precision error. Mitigate
--    by rounding before reverting (should not occur in Phase-3
--    data — Pefindo source provides max 4 decimal places).
-- ============================================================

ALTER TABLE mst.pd_pefindo
    ALTER COLUMN pd_12month    TYPE NUMERIC(8,4),
    ALTER COLUMN pd_lifetime_3y  TYPE NUMERIC(8,4),
    ALTER COLUMN pd_lifetime_5y  TYPE NUMERIC(8,4),
    ALTER COLUMN pd_lifetime_7y  TYPE NUMERIC(8,4),
    ALTER COLUMN pd_lifetime_10y TYPE NUMERIC(8,4);

COMMIT;
