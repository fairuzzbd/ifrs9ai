-- migration: 0034 penempatan_terminate_sod_complete (DOWN)
-- author: ecl-eir-engineer
-- description: Drop F2 terminate SoD belt-and-suspenders CHECK constraints.

BEGIN;

ALTER TABLE trx.penempatan_deposito
    DROP CONSTRAINT IF EXISTS chk_penempatan_sod_term_reviewer_vs_terminate_maker;

ALTER TABLE trx.penempatan_deposito
    DROP CONSTRAINT IF EXISTS chk_penempatan_sod_term_approver_vs_terminate_maker;

COMMIT;
