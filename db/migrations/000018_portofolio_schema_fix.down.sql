-- migration: 0018 portofolio_schema_fix DOWN
-- author: backend-engineer-go
-- Reverses the changes made in 000018_portofolio_schema_fix.up.sql

BEGIN;

-- Remove the config row seeded in up (only if we added it; idempotent).
DELETE FROM sys.config WHERE config_key = 'WORKFLOW_CONFIG_PORTOFOLIO';

-- Drop indexes added in up.
DROP INDEX IF EXISTS idx_portofolio_workflow_status;
DROP INDEX IF EXISTS idx_portofolio_tenant_created;

-- Restore the original unique index on is_deleted.
DROP INDEX IF EXISTS uq_portofolio_kode;
CREATE UNIQUE INDEX uq_portofolio_kode ON mst.portofolio(kode_portofolio) WHERE is_deleted = FALSE;

-- Remove columns added in up.
ALTER TABLE mst.portofolio
    DROP CONSTRAINT IF EXISTS chk_portofolio_workflow_status,
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;

COMMIT;
