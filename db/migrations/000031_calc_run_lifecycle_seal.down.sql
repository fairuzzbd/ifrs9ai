-- migration: 0031 calc_run_lifecycle_seal — DOWN
-- author: data-modeler
-- Reverses migration 000031 in strict reverse order:
--   1. Drop FK fk_ecl_calc_result_line_calc_run (added in B)
--   2. Drop triggers on ecl.calc_run
--   3. Drop trigger function fn_ecl_calc_run_no_modify_when_sealed
--      (this is the 000031 version installed on ecl.calc_run — NOT the 000029 version
--       installed on ecl.calc_header, which remains untouched)
--   4. DROP TABLE ecl.calc_run (cascades indexes + constraints)
--
-- Pre-condition: ecl.calc_run must have zero rows (or be acceptable to drop).
-- The FK on ecl.calc_result_line is removed before the table is dropped.

BEGIN;

-- ----------------------------------------------------------------
-- B. Remove FK constraint added to ecl.calc_result_line in 000031
-- ----------------------------------------------------------------
ALTER TABLE ecl.calc_result_line
    DROP CONSTRAINT IF EXISTS fk_ecl_calc_result_line_calc_run;

-- Restore comment to 000029 state (deferred FK pending OQ-M7-2 resolution)
COMMENT ON COLUMN ecl.calc_result_line.calc_run_id IS
    'UUID identifying the ECL calc run. Logically FK → ecl.calc_header(calc_run_id column). '
    'No DB-level FK constraint here (OQ-M7-2: M8 will resolve once sys.job PK type is unified). '
    'Application must validate existence before INSERT.';

-- ----------------------------------------------------------------
-- A. Remove triggers and function on ecl.calc_run, then drop table
-- ----------------------------------------------------------------

DROP TRIGGER IF EXISTS trg_ecl_calc_run_row_version    ON ecl.calc_run;
DROP TRIGGER IF EXISTS trg_ecl_calc_run_updated_at     ON ecl.calc_run;
DROP TRIGGER IF EXISTS trg_ecl_calc_run_sealed_guard   ON ecl.calc_run;

-- Drop the 000031 version of the guard function.
-- The 000029 version (guarding ecl.calc_header) has a different name and is NOT dropped here.
DROP FUNCTION IF EXISTS fn_ecl_calc_run_no_modify_when_sealed();

-- Indexes are dropped automatically with the table.
DROP TABLE IF EXISTS ecl.calc_run;

COMMIT;
