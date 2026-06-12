-- migration: 0030 calc_result_line_formula_version (DOWN)
-- author: ecl-eir-engineer
-- description: Reverses 000030 — drops formula_version column from ecl.calc_result_line.

BEGIN;

ALTER TABLE ecl.calc_result_line
    DROP COLUMN IF EXISTS formula_version;

COMMIT;
