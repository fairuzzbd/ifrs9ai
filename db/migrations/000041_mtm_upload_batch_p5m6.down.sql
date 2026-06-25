-- migration: 0041 mtm_upload_batch_p5m6 — ROLLBACK
-- author: data-modeler (driven by system-analyst P5-M6)
-- description: Reverse all changes from 000041_mtm_upload_batch_p5m6.up.sql.
-- Order: (D) periode_bulanan_id column + index → (C) MTM batch index
--        → (B) precision downgrade → (A) ck_batch_type (no-op if was already correct).
-- WARNING: Precision downgrade NUMERIC(20,8) → NUMERIC(15,4) on upload_batch_row
--          will SILENTLY TRUNCATE decimal places 5-8. Run only in non-production.

BEGIN;

-- ====================================================================
-- D. Remove periode_bulanan_id column and its index.
-- ====================================================================

DROP INDEX IF EXISTS sys.idx_upload_batch_periode;

ALTER TABLE sys.upload_batch
    DROP COLUMN IF EXISTS periode_bulanan_id;

-- ====================================================================
-- C. Remove MTM batch index.
-- ====================================================================

DROP INDEX IF EXISTS sys.idx_upload_batch_mtm_type;

-- ====================================================================
-- B. Precision downgrade: NUMERIC(20,8) → NUMERIC(15,4) on upload_batch_row.
--    WARNING: truncates decimal places 5-8. Acceptable in dev/test.
-- ====================================================================

DO $$
DECLARE
    v_col_type TEXT;
BEGIN
    SELECT data_type || COALESCE('(' || numeric_precision || ',' || numeric_scale || ')', '')
    FROM information_schema.columns
    WHERE table_schema = 'sys'
      AND table_name   = 'upload_batch_row'
      AND column_name  = 'harga_native'
    INTO v_col_type;

    IF v_col_type ILIKE '%20,8%' THEN
        ALTER TABLE sys.upload_batch_row
            ALTER COLUMN harga_native       TYPE NUMERIC(15,4) USING harga_native::NUMERIC(15,4),
            ALTER COLUMN harga_sebelumnya   TYPE NUMERIC(15,4) USING harga_sebelumnya::NUMERIC(15,4);
        RAISE NOTICE 'sys.upload_batch_row: precision downgraded NUMERIC(20,8) → NUMERIC(15,4). '
                     'WARNING: decimal places 5-8 truncated.';
    ELSE
        RAISE NOTICE 'sys.upload_batch_row.harga_native not NUMERIC(20,8) — no downgrade needed.';
    END IF;
END $$;

-- ====================================================================
-- A. ck_batch_type: restore to original (migration 000001 definition).
--    Only execute if we modified it in up.sql (i.e., MTM_UPLOAD was added by this migration).
--    Since migration 000001 already had 'MTM_UPLOAD', in the normal case this section
--    is a no-op (the constraint hasn't changed). We restore it for completeness in
--    the rare case where it was absent and we added it.
-- ====================================================================

DO $$
DECLARE
    v_constraint_exists BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.table_constraints tc
        WHERE tc.table_schema   = 'sys'
          AND tc.table_name     = 'upload_batch'
          AND tc.constraint_name = 'ck_batch_type'
          AND tc.constraint_type = 'CHECK'
    ) INTO v_constraint_exists;

    -- If this migration added ck_batch_type (it wasn't there before), removing it here
    -- would restore the pre-000041 state. Since in normal operation the constraint already
    -- existed from 000001, we leave it in place to avoid removing valid DB integrity.
    -- Only remove if the migration had to create it from scratch (unusual scenario).
    -- Decision: leave ck_batch_type in place during down migration (safe for rollback).
    RAISE NOTICE 'ck_batch_type rollback: constraint retained (safe; was present in migration 000001). '
                 'No action taken.';
END $$;

COMMIT;
