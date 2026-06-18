-- migration: 0039 fx_rate_p5m5 — ROLLBACK
-- author: data-modeler
-- description: Reverse all changes from 000039_fx_rate_p5m5.up.sql.
--   Order: (F) triggers → (E) config seeds → (D) dlq_fx_jisdor → (C) holiday_calendar
--          → (A) mst.kurs columns/indexes → (B) CHECK constraint restore.
-- WARNING: Precision downgrade NUMERIC(20,8) → NUMERIC(15,4) will SILENTLY TRUNCATE
--          decimal places 5-8. Run only in non-production environments unless confirmed
--          that all FX rate values are within 4 decimal places of significant precision.

BEGIN;

-- ====================================================================
-- F. Remove trigger + function added in 000039 for mst.kurs locked check.
--    If a tg_kurs_locked_check existed BEFORE this migration (migration 000020),
--    it was DROP + RECREATED — restoring it here would need its original definition.
--    Since migration 000020 analysis shows NO such trigger existed, safe to drop.
-- ====================================================================

DROP TRIGGER IF EXISTS tg_kurs_locked_check ON mst.kurs;
DROP FUNCTION IF EXISTS fn_kurs_locked_check();

-- ====================================================================
-- E. Remove sys.config seeds added by 000039.
--    Only removes rows with these exact keys; does not touch other config.
-- ====================================================================

DELETE FROM sys.config
WHERE config_key IN (
    'FX_JISDOR_CURRENCIES',
    'FX_JISDOR_BASE_URL',
    'FX_JISDOR_AUTOAPPROVE',
    'FX_RATE_DEVIATION_THRESHOLD_PCT',
    'FX_JISDOR_CRON_SCHEDULE'
);

-- ====================================================================
-- D. Drop sys.dlq_fx_jisdor (and its triggers/functions).
-- ====================================================================

DROP TRIGGER  IF EXISTS trg_dlq_fx_jisdor_no_delete ON sys.dlq_fx_jisdor;
DROP FUNCTION IF EXISTS fn_dlq_fx_jisdor_no_delete();
DROP TABLE    IF EXISTS sys.dlq_fx_jisdor;

-- ====================================================================
-- C. Drop sys.holiday_calendar (and its indexes / seed data).
-- ====================================================================

DROP TABLE IF EXISTS sys.holiday_calendar;

-- ====================================================================
-- A. Remove P5-M5 additions to mst.kurs: indexes first, then columns.
-- ====================================================================

-- Indexes
DROP INDEX IF EXISTS mst.idx_kurs_active_unique;
DROP INDEX IF EXISTS mst.idx_kurs_upload_batch_id;
DROP INDEX IF EXISTS mst.idx_kurs_deviation_flag;

-- Columns
ALTER TABLE mst.kurs
    DROP COLUMN IF EXISTS deviation_flag,
    DROP COLUMN IF EXISTS rate_deviation_pct,
    DROP COLUMN IF EXISTS jisdor_fetch_metadata,
    DROP COLUMN IF EXISTS reject_reason,
    DROP COLUMN IF EXISTS upload_batch_id;

-- Precision downgrade: NUMERIC(20,8) → NUMERIC(15,4).
-- WARNING: truncates decimal places 5-8. Acceptable in dev/test rollback scenarios.
ALTER TABLE mst.kurs
    ALTER COLUMN kurs_tengah TYPE NUMERIC(15,4)
        USING kurs_tengah::NUMERIC(15,4),
    ALTER COLUMN kurs_beli   TYPE NUMERIC(15,4)
        USING kurs_beli::NUMERIC(15,4),
    ALTER COLUMN kurs_jual   TYPE NUMERIC(15,4)
        USING kurs_jual::NUMERIC(15,4);

-- ====================================================================
-- B. Restore original CHECK constraint (7-state, no ACTIVE).
-- ====================================================================

ALTER TABLE mst.kurs
    DROP CONSTRAINT IF EXISTS chk_kurs_workflow_status;

ALTER TABLE mst.kurs
    ADD CONSTRAINT chk_kurs_workflow_status
        CHECK (workflow_status IN (
            'DRAFT',
            'PENDING_REVIEW',
            'PENDING_APPROVAL',
            'PENDING_APPROVAL_2',
            'APPROVED',
            'REJECTED',
            'RETURNED'
        ));

COMMIT;
