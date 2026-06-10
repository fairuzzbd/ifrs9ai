-- migration: 0020 kurs_schema_fix (DOWN)
-- author: backend-engineer-go
-- description: Reverse migration 0020 — remove audit cols + constraints added to mst.kurs.

BEGIN;

-- Remove constraints added in up
ALTER TABLE mst.kurs
    DROP CONSTRAINT IF EXISTS chk_kurs_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_kurs_sumber_kurs;

-- Remove indexes added in up
DROP INDEX IF EXISTS mst.idx_kurs_tenant_created;
DROP INDEX IF EXISTS mst.idx_kurs_workflow_status;
DROP INDEX IF EXISTS mst.idx_kurs_kode_tanggal_active;

-- Remove columns added in up
ALTER TABLE mst.kurs
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS workflow_status;

-- Remove seeded config (only if it was inserted by this migration, not 0008)
-- Safe: ON CONFLICT DO NOTHING means it's a no-op if 0008 already seeded it.
-- We do NOT delete the config row on down because 0008 may have seeded it first.

COMMIT;
