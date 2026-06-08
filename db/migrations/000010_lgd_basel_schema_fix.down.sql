-- migration: 0010 lgd_basel_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.lgd_basel schema additions from 0010.
--              Does NOT touch WORKFLOW_CONFIG_LGD_BASEL in sys.config
--              (seeded in 0008 — reverted by 0008 down, not here).

BEGIN;

-- ============================================================
-- 0. Revert lgd precision NUMERIC(10,8) → NUMERIC(8,4)
--    WARNING: truncates extra precision on existing rows.
--    Provided for migration parity only.
-- ============================================================

ALTER TABLE mst.lgd_basel
    ALTER COLUMN lgd TYPE NUMERIC(8,4);

-- ============================================================
-- 1. Drop indexes added by this migration
--    ix_lgd_tipe_periode / ix_lgd_current (from 0001) are NOT touched.
-- ============================================================

DROP INDEX IF EXISTS mst.idx_lgd_basel_active;
DROP INDEX IF EXISTS mst.idx_lgd_basel_workflow_status;
DROP INDEX IF EXISTS mst.idx_lgd_basel_tenant_created;

-- ============================================================
-- 2. Drop CHECK constraint on workflow_status
-- ============================================================

ALTER TABLE mst.lgd_basel
    DROP CONSTRAINT IF EXISTS chk_lgd_basel_workflow_status;

-- ============================================================
-- 3. Drop columns added by this migration
--    Order: workflow_status first (has constraint), then audit cols.
--    legacy maker_id / approver_id / approved_at / created_at are
--    NOT dropped — they belong to 0001.
-- ============================================================

ALTER TABLE mst.lgd_basel
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

COMMIT;
