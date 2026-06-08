-- migration: 0009 periode_buku_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert mst.periode_buku schema additions introduced in 0009.
--              WORKFLOW_CONFIG_PERIODE (seeded in 0007) is NOT touched.

BEGIN;

-- ============================================================
-- 0. Remove WORKFLOW_CONFIG_PERIODE_BUKU seed (added in up.sql §5)
--    WORKFLOW_CONFIG_PERIODE (0007) is NOT touched.
-- ============================================================

DELETE FROM sys.config WHERE config_key = 'WORKFLOW_CONFIG_PERIODE_BUKU';

-- ============================================================
-- 1. Drop indexes added by this migration
-- ============================================================

DROP INDEX IF EXISTS mst.idx_periode_buku_active;
DROP INDEX IF EXISTS mst.idx_periode_buku_workflow_status;
DROP INDEX IF EXISTS mst.idx_periode_buku_tenant_created;

-- ============================================================
-- 2. Drop CHECK constraint on workflow_status
-- ============================================================

ALTER TABLE mst.periode_buku
    DROP CONSTRAINT IF EXISTS chk_periode_workflow_status;

-- ============================================================
-- 3. Drop columns added by this migration
-- ============================================================

ALTER TABLE mst.periode_buku
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by;

COMMIT;
