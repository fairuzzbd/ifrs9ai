-- migration: 0030 calc_result_line_formula_version
-- author: ecl-eir-engineer
-- requires: 0029 (ecl_core_tables — ecl.calc_result_line)
-- description:
--   F8 fix (compliance finding): add formula_version column to ecl.calc_result_line.
--   Stores the ECL formula algorithm version used for each result row.
--   Enables regression detection when formula changes across releases.
--   Default 'M7-v1.0' applied to all existing rows via column default.
--   Referenced by: ecl-eir-engineer F8 compliance fix, PSAK 71 audit trail.

BEGIN;

ALTER TABLE ecl.calc_result_line
    ADD COLUMN IF NOT EXISTS formula_version TEXT NOT NULL DEFAULT 'M7-v1.0';

COMMENT ON COLUMN ecl.calc_result_line.formula_version IS
    'ECL formula algorithm version used to compute this row. '
    'Default: M7-v1.0 (P4-M7 initial implementation, combined impact_pd × impact_mev_pd FL). '
    'Updated on each algorithm change. Used for regression detection and audit trail. '
    'Added by migration 000030 (F8 compliance fix).';

-- Update any existing rows that may have been inserted before this migration
-- (defensive backfill — in practice 000030 runs before any prod data).
UPDATE ecl.calc_result_line
SET formula_version = 'M7-v1.0'
WHERE formula_version IS NULL OR formula_version = '';

COMMIT;
