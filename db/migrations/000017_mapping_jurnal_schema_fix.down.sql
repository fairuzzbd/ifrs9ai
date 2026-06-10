-- migration: 0017 mapping_jurnal_schema_fix (DOWN)
-- Reverses: removal of indexes and added columns.
-- Note: We do not drop data added by UPDATE (backfill) — that is safe to leave.

BEGIN;

-- ============================================================
-- 1. mst.mapping_jurnal_detail — DROP added columns
-- ============================================================

DROP INDEX IF EXISTS idx_mapping_detail_active;
DROP INDEX IF EXISTS idx_mapping_detail_tenant;

ALTER TABLE mst.mapping_jurnal_detail
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by;

-- ============================================================
-- 2. mst.mapping_jurnal_header — DROP added columns
-- ============================================================

DROP INDEX IF EXISTS idx_mapping_header_aktif;
DROP INDEX IF EXISTS idx_mapping_header_workflow_status;
DROP INDEX IF EXISTS idx_mapping_header_tenant_created;

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_header_workflow_status;

ALTER TABLE mst.mapping_jurnal_header
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;

COMMIT;
