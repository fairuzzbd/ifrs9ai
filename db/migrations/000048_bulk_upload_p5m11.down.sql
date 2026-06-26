-- migration: 000048 Bulk Upload Master Instrumen (P5-M11) — ROLLBACK
-- description: Reverse all changes from 000048_bulk_upload_p5m11.up.sql.
-- WARNING: This will drop columns and indexes added by P5-M11.
--          Data in bulk_upload_batch_id, row_status, etc. will be LOST.
--          Only run in dev/test environments. Production rollback requires data migration plan.

BEGIN;

-- ─── 7. Remove comments (implicit on column drop) ────────────────────────────

-- ─── 6. Remove sys.config_param seeds ────────────────────────────────────────
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'sys' AND table_name = 'config_param'
    ) THEN
        DELETE FROM sys.config_param
        WHERE param_key IN (
            'BULK_ROLLBACK_GRACE_DAYS',
            'BULK_FILE_MAX_MB',
            'BULK_DRY_RUN_TTL_SECONDS'
        );
        RAISE NOTICE 'sys.config_param: P5-M11 params removed';
    END IF;
END;
$$;

-- ─── 5. Remove sys.upload_batch indexes ──────────────────────────────────────
DROP INDEX IF EXISTS sys.idx_upload_batch_dry_run_expires;
DROP INDEX IF EXISTS sys.idx_upload_batch_rollback_grace;

-- ─── 4. Remove upload_batch_row.bulk_instrumen_id FK + index ─────────────────
DROP INDEX IF EXISTS sys.idx_upload_batch_row_bulk_instrumen;
DROP INDEX IF EXISTS sys.idx_upload_batch_row_status_batch;

ALTER TABLE sys.upload_batch_row
    DROP CONSTRAINT IF EXISTS fk_batch_row_bulk_instrumen;

-- ─── 3. Remove mst.instrumen changes ─────────────────────────────────────────
DROP INDEX IF EXISTS mst.idx_instrumen_pending_approval_bulk;
DROP INDEX IF EXISTS mst.idx_instrumen_bulk_batch;

ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS fk_instrumen_bulk_batch;

ALTER TABLE mst.instrumen
    DROP COLUMN IF EXISTS bulk_upload_batch_id;

-- ─── 2. Remove sys.upload_batch_row columns ───────────────────────────────────
ALTER TABLE sys.upload_batch_row
    DROP CONSTRAINT IF EXISTS ck_bulk_row_status;

ALTER TABLE sys.upload_batch_row
    DROP COLUMN IF EXISTS row_error_jsonb;

ALTER TABLE sys.upload_batch_row
    DROP COLUMN IF EXISTS bulk_instrumen_id;

ALTER TABLE sys.upload_batch_row
    DROP COLUMN IF EXISTS row_status;

-- ─── 1. Remove sys.upload_batch columns ──────────────────────────────────────
-- Note: We do NOT revert ck_batch_status constraint change here because
-- rolling back the constraint could invalidate existing rows. Callers should
-- manage constraint revert carefully or just drop+recreate in dev.
-- The columns are safe to drop (no data dependency after other steps complete).

ALTER TABLE sys.upload_batch
    DROP COLUMN IF EXISTS rollback_grace_expires_at;

ALTER TABLE sys.upload_batch
    DROP COLUMN IF EXISTS dry_run_result_jsonb;

ALTER TABLE sys.upload_batch
    DROP COLUMN IF EXISTS dry_run_expires_at;

ALTER TABLE sys.upload_batch
    DROP COLUMN IF EXISTS dry_run_cached_at;

COMMIT;
