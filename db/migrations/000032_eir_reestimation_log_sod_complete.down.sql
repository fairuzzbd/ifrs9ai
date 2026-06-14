-- migration: 0032 eir_reestimation_log_sod_complete (DOWN)
BEGIN;

ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod_approver_vs_reviewer;
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod_approver_vs_maker;
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod_complete;

-- Restore original partial constraint
ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_sod
    CHECK (reviewer_id IS NULL OR reviewer_id <> maker_id);

COMMIT;
