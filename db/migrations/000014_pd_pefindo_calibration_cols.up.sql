-- migration: 0014 pd_pefindo_calibration_cols
-- author: data-modeler
-- requires: 0013
-- description: Add calibration columns (published_reference, calibration_delta,
--              calibration_status) to mst.pd_pefindo per F-001 compliance finding.
--              Option (b): retain flat schema (no release/curve split), add calibration
--              cols + deprecation note to satisfy PSAK 71 traceability.
--              DEC-016: NUMERIC(10,8) for published_reference and calibration_delta.
--              Formally supersedes plan §4 release+curve split — decision recorded here
--              as DEC inline note. See docs/plans/PLAN-20260608-phase-3-pd-pefindo.md §4.

BEGIN;

-- ============================================================
-- 1. CALIBRATION COLUMNS
--    published_reference  — Pefindo published rate for the bucket (from PDF source).
--                           Nullable: not every tenor bucket is explicitly published.
--    calibration_delta    — |pd_value - published_reference|; null when reference is null.
--    calibration_status   — Summary status: PENDING|PASSED|WARNING|FAILED.
-- ============================================================

ALTER TABLE mst.pd_pefindo
    ADD COLUMN IF NOT EXISTS published_reference  NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS calibration_delta    NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS calibration_status   TEXT NOT NULL DEFAULT 'PENDING';

-- ============================================================
-- 2. CHECK CONSTRAINTS — calibration values
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_published_reference_range;

ALTER TABLE mst.pd_pefindo
    ADD CONSTRAINT chk_pd_pefindo_published_reference_range
        CHECK (published_reference IS NULL OR published_reference BETWEEN 0 AND 1);

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_calibration_delta_nonneg;

ALTER TABLE mst.pd_pefindo
    ADD CONSTRAINT chk_pd_pefindo_calibration_delta_nonneg
        CHECK (calibration_delta IS NULL OR calibration_delta >= 0);

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_calibration_status;

ALTER TABLE mst.pd_pefindo
    ADD CONSTRAINT chk_pd_pefindo_calibration_status
        CHECK (calibration_status IN ('PENDING', 'PASSED', 'WARNING', 'FAILED'));

-- ============================================================
-- 3. INDEX — calibration_status for reviewer queue
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_pd_pefindo_calibration_status
    ON mst.pd_pefindo (calibration_status)
    WHERE deleted_at IS NULL;

-- ============================================================
-- 4. DEPRECATION NOTE
--    Formally document that the release+curve split (plan §4) was
--    superseded by this flat-schema approach (option b). Existing rows
--    preserved for audit continuity. Release+curve design deferred to
--    Phase 5 if extensibility is required.
-- ============================================================

COMMENT ON TABLE mst.pd_pefindo IS
    'PD Pefindo parameter table. Migration 0013 added audit cols + workflow_status.
     Migration 0014 adds calibration columns (published_reference, calibration_delta,
     calibration_status) satisfying PSAK 71 traceability per F-001 compliance finding.
     The plan §4 release+curve split was formally deferred (option b) — flat schema
     retained; calibration data is per-row alongside the PD values.
     Scheduled review for release+curve split in Phase 5 if new tenor buckets are needed.';

COMMIT;
