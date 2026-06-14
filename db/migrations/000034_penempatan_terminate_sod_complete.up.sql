-- migration: 0034 penempatan_terminate_sod_complete
-- author: ecl-eir-engineer
-- requires: 0033 (penempatan_deposito_p5_m1)
-- description:
--   F2 belt-and-suspenders DB CHECK constraints (DEC-P5-M1-005, DEC-017):
--   Terminate SoD gaps identified in compliance review PR #105.
--   (A) terminate_reviewer_id must differ from terminate_maker_id
--   (B) terminate_approver_id must differ from terminate_maker_id
--   Service layer checks are the primary enforcement; these constraints are
--   the belt-and-suspenders guard for direct-DB writes / future migrations.
--   Constraint names follow pattern: chk_{table}_{rule}.

BEGIN;

-- ────────────────────────────────────────────────────────────────────────────
-- A. terminate_reviewer ≠ terminate_maker
-- ────────────────────────────────────────────────────────────────────────────
ALTER TABLE trx.penempatan_deposito
    ADD CONSTRAINT chk_penempatan_sod_term_reviewer_vs_terminate_maker
    CHECK (
        terminate_reviewer_id IS NULL
        OR terminate_maker_id IS NULL
        OR terminate_reviewer_id <> terminate_maker_id
    );

-- ────────────────────────────────────────────────────────────────────────────
-- B. terminate_approver ≠ terminate_maker
-- ────────────────────────────────────────────────────────────────────────────
ALTER TABLE trx.penempatan_deposito
    ADD CONSTRAINT chk_penempatan_sod_term_approver_vs_terminate_maker
    CHECK (
        terminate_approver_id IS NULL
        OR terminate_maker_id IS NULL
        OR terminate_approver_id <> terminate_maker_id
    );

COMMIT;
