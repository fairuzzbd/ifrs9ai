-- migration: 0008 mata_uang_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.mata_uang schema additions and remove Phase-3
--              WORKFLOW_CONFIG_* seed rows. Does NOT touch rows seeded in 0007.

BEGIN;

-- ============================================================
-- 1. Remove Phase-3 WORKFLOW_CONFIG seed rows from sys.config
--    (only the ones added by this migration — 0007 rows are left intact)
-- ============================================================
DELETE FROM sys.config WHERE config_key IN (
    'WORKFLOW_CONFIG_MATA_UANG',
    'WORKFLOW_CONFIG_PORTOFOLIO',
    'WORKFLOW_CONFIG_CHART_OF_ACCOUNTS',
    'WORKFLOW_CONFIG_MAPPING_JURNAL',
    'WORKFLOW_CONFIG_KURS',
    'WORKFLOW_CONFIG_COUNTERPARTY',
    'WORKFLOW_CONFIG_RATING_HISTORY',
    'WORKFLOW_CONFIG_INSTRUMEN',
    'WORKFLOW_CONFIG_PD_PEFINDO',
    'WORKFLOW_CONFIG_LGD_BASEL',
    'WORKFLOW_CONFIG_BOBOT_SKENARIO',
    'WORKFLOW_CONFIG_LPS_COVERAGE',
    'WORKFLOW_CONFIG_IMPACT_MEV_PD',
    'WORKFLOW_CONFIG_IMPACT_PD'
);

-- ============================================================
-- 2. Revert mst.mata_uang schema additions
-- ============================================================

-- Drop indexes added by this migration
DROP INDEX IF EXISTS mst.idx_mata_uang_workflow_status;
DROP INDEX IF EXISTS mst.idx_mata_uang_aktif;
DROP INDEX IF EXISTS mst.idx_mata_uang_tenant_created;
DROP INDEX IF EXISTS mst.uq_mata_uang_id;

-- Drop CHECK constraints
ALTER TABLE mst.mata_uang
    DROP CONSTRAINT IF EXISTS chk_mata_uang_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_mata_uang_decimal_places;

-- Drop columns (added by this migration only)
ALTER TABLE mst.mata_uang
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS is_system_currency,
    DROP COLUMN IF EXISTS decimal_places,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS id;

COMMIT;
