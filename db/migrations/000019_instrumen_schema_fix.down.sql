-- migration: 0019 instrumen_schema_fix (down)
-- author: backend-engineer-go
-- Reverses: add audit cols, CHECK constraints, indexes, sys.config seed.

BEGIN;

-- Reverse sys.config seed
DELETE FROM sys.config WHERE config_key = 'WORKFLOW_CONFIG_INSTRUMEN';

-- Reverse indexes
DROP INDEX IF EXISTS mst.idx_instrumen_tenant_created;
DROP INDEX IF EXISTS mst.idx_instrumen_workflow_status_partial;

-- Reverse CHECK constraints
ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_instrumen_workflow_status,
    DROP CONSTRAINT IF EXISTS ck_instrumen_tipe,
    DROP CONSTRAINT IF EXISTS ck_instrumen_sppi_result;

-- Reverse audit columns
-- NOTE: deleted_at backfill cannot be perfectly reversed (data loss acceptable in rollback).
-- We clear deleted_at for rows that were backfilled from is_deleted (best-effort).
UPDATE mst.instrumen
    SET deleted_at = NULL
WHERE is_deleted = TRUE AND deleted_at IS NOT NULL;

ALTER TABLE mst.instrumen
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version;

COMMIT;
