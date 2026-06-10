-- migration: 0014 pd_pefindo_calibration_cols (DOWN)
-- author: data-modeler
-- description: Revert calibration columns added by 0014.

BEGIN;

-- ============================================================
-- 1. Drop index
-- ============================================================

DROP INDEX IF EXISTS mst.idx_pd_pefindo_calibration_status;

-- ============================================================
-- 2. Drop CHECK constraints
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_calibration_status,
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_calibration_delta_nonneg,
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_published_reference_range;

-- ============================================================
-- 3. Drop columns
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP COLUMN IF EXISTS calibration_status,
    DROP COLUMN IF EXISTS calibration_delta,
    DROP COLUMN IF EXISTS published_reference;

-- ============================================================
-- 4. Restore original table comment (from 0013 state)
-- ============================================================

COMMENT ON TABLE mst.pd_pefindo IS NULL;

COMMIT;
