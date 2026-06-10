-- migration: 0011 bobot_skenario_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.bobot_skenario schema additions from 0011.
--              Does NOT touch WORKFLOW_CONFIG_BOBOT_SKENARIO in sys.config
--              (seeded in 0008 — reverted by 0008 down, not here).

BEGIN;

-- ============================================================
-- 0. Revert bobot precision NUMERIC(10,8) → NUMERIC(8,4)
--    WARNING: truncates extra precision on existing rows.
--    Provided for migration parity only.
-- ============================================================

ALTER TABLE mst.bobot_skenario
    ALTER COLUMN bobot TYPE NUMERIC(8,4);

-- ============================================================
-- 1. Drop indexes added by this migration
--    ix_bobot_skenario_periode / ix_bobot_current (from 0001) are NOT touched.
-- ============================================================

DROP INDEX IF EXISTS mst.idx_bobot_skenario_active;
DROP INDEX IF EXISTS mst.idx_bobot_skenario_workflow_status;
DROP INDEX IF EXISTS mst.idx_bobot_skenario_tenant_created;

-- ============================================================
-- 2. Drop CHECK constraint on workflow_status
-- ============================================================

ALTER TABLE mst.bobot_skenario
    DROP CONSTRAINT IF EXISTS chk_bobot_skenario_workflow_status;

-- ============================================================
-- 3. Drop columns added by this migration
--    Order: workflow_status first (has constraint), then audit cols.
--    Legacy maker_id / approver_id / approved_at / created_at are
--    NOT dropped — they belong to 0001.
-- ============================================================

ALTER TABLE mst.bobot_skenario
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

COMMIT;
