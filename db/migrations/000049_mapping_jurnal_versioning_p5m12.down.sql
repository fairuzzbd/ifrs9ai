-- migration: 000049 mapping_jurnal_versioning_p5m12 — DOWN
-- Reverses all changes from 000049 up.sql.
-- author: backend-engineer-go

BEGIN;

-- E. Undo: reset regulated_flag back to FALSE for seeded rows
UPDATE mst.mapping_jurnal_header
SET regulated_flag = FALSE,
    updated_at     = now(),
    row_version    = row_version + 1
WHERE event_code IN (
    'ECL_PEMBENTUKAN', 'ECL_REVERSAL', 'POCI_DELTA_ECL',
    'MTM_FVTPL', 'MTM_FVOCI', 'MTM_FVOCI_ELECTION',
    'REKLAS_OCI_PL', 'REKLASIFIKASI_AC_FVOCI', 'REKLASIFIKASI_FVOCI_AC',
    'MODIFIKASI_MATERIAL', 'EIR_CATCH_UP_ADJUSTMENT', 'STAGE_MIGRATION', 'FX_UNREALIZED'
)
  AND deleted_at IS NULL;

-- D. Remove MAPPING_REGULATED_EVENT_CODES config
DELETE FROM sys.config WHERE config_key = 'MAPPING_REGULATED_EVENT_CODES';

-- C. Drop immutability trigger + function
DROP TRIGGER IF EXISTS trg_mapping_header_active_immutability ON mst.mapping_jurnal_header;
DROP FUNCTION IF EXISTS fn_mapping_header_active_immutability();

-- B. Drop UNIQUE index
DROP INDEX IF EXISTS uq_mapping_header_one_active_per_event;

-- A. Drop indexes added in this migration
DROP INDEX IF EXISTS idx_mapping_header_parent_id;
DROP INDEX IF EXISTS idx_mapping_header_tenant_status_event;
DROP INDEX IF EXISTS idx_mapping_header_active_event;
DROP INDEX IF EXISTS idx_mapping_header_regulated;

-- A. Drop added columns (in reverse order, only if they exist)
ALTER TABLE mst.mapping_jurnal_header DROP COLUMN IF EXISTS step_up_token_ref;
ALTER TABLE mst.mapping_jurnal_header DROP COLUMN IF EXISTS regulated_flag;
ALTER TABLE mst.mapping_jurnal_header DROP COLUMN IF EXISTS effective_to;
ALTER TABLE mst.mapping_jurnal_header DROP COLUMN IF EXISTS effective_from;
ALTER TABLE mst.mapping_jurnal_header DROP COLUMN IF EXISTS parent_id;

COMMIT;
