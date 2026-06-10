-- migration: 0016 chart_of_accounts_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.chart_of_accounts schema additions from 0016.
--              Does NOT touch columns, constraints, or indexes from 0001.
--              Does NOT touch sys.config WORKFLOW_CONFIG_CHART_OF_ACCOUNTS
--              (seeded in 0008; reverted by 0008 down).

BEGIN;

-- ============================================================
-- 1. DROP INDEXES added by 0016
-- ============================================================

DROP INDEX IF EXISTS mst.idx_coa_workflow_status;
DROP INDEX IF EXISTS mst.idx_coa_tenant_created;

-- ============================================================
-- 2. DROP CHECK CONSTRAINT added by 0016
-- ============================================================

ALTER TABLE mst.chart_of_accounts
    DROP CONSTRAINT IF EXISTS chk_coa_workflow_status;

-- ============================================================
-- 3. DROP COLUMNS added by 0016
-- ============================================================

ALTER TABLE mst.chart_of_accounts
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;

COMMIT;
