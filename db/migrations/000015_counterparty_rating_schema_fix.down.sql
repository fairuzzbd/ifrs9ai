-- migration: 0015 counterparty_rating_schema_fix (DOWN)
-- author: data-modeler
-- description: Revert schema additions to mst.counterparty and
--              mst.rating_history_counterparty made by 0015.
--              Does NOT touch:
--                - nomor_rekening_encrypted / ktp_encrypted (added by 0003 —
--                  removing them here would break 0003 DOWN; those columns
--                  must be dropped by 0003 DOWN if needed)
--                - WORKFLOW_CONFIG_COUNTERPARTY / WORKFLOW_CONFIG_RATING_HISTORY
--                  (seeded in 0008 — not touched here)
--                - existing data in is_deleted / version (legacy cols from 0001,
--                  not modified by this migration)

BEGIN;

-- ============================================================
-- SECTION 2 (reversed first — child table): mst.rating_history_counterparty
-- ============================================================

DROP INDEX IF EXISTS mst.idx_rating_history_active_counterparty;
DROP INDEX IF EXISTS mst.idx_rating_history_counterparty_fk;
DROP INDEX IF EXISTS mst.idx_rating_history_tenant_created;
DROP INDEX IF EXISTS mst.idx_rating_history_workflow_status;

ALTER TABLE mst.rating_history_counterparty
    DROP CONSTRAINT IF EXISTS chk_rating_history_workflow_status;

ALTER TABLE mst.rating_history_counterparty
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- ============================================================
-- SECTION 1 (reversed): mst.counterparty
-- ============================================================

DROP INDEX IF EXISTS mst.idx_counterparty_tenant_active;
DROP INDEX IF EXISTS mst.idx_counterparty_pii_lookup;
DROP INDEX IF EXISTS mst.idx_counterparty_tenant_created;
DROP INDEX IF EXISTS mst.idx_counterparty_workflow_status;

ALTER TABLE mst.counterparty
    DROP CONSTRAINT IF EXISTS chk_counterparty_workflow_status;

-- Restore original ck_counterparty_tipe (6-value set from 0001)
ALTER TABLE mst.counterparty
    DROP CONSTRAINT IF EXISTS ck_counterparty_tipe;

ALTER TABLE mst.counterparty
    ADD CONSTRAINT ck_counterparty_tipe CHECK (tipe IN (
        'BANK',
        'BANK_KUSTODIAN',
        'KORPORASI',
        'PEMERINTAH',
        'MANAJER_INVESTASI',
        'EMITEN_SAHAM'
    ));

-- Drop columns added by this migration
-- NOTE: nomor_rekening_encrypted and ktp_encrypted are NOT dropped here —
--       they were added by migration 0003 and must be reverted by 0003 DOWN.
ALTER TABLE mst.counterparty
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at;

COMMIT;
