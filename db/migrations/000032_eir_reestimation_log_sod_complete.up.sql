-- migration: 0032 eir_reestimation_log_sod_complete
-- author: data-modeler
-- requires: 0001, 0026
-- description: Defense-in-depth — DB CHECK enforces all 3 SoD axes for
--              4-eyes EIR amendment workflow per DEC-017.

BEGIN;

-- Drop existing partial SoD check
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod;

-- Add complete SoD check covering all 3 axes:
-- 1. reviewer != maker
-- 2. approver != maker (NULL-safe: only check when approver set)
-- 3. approver != reviewer (NULL-safe)
ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_sod_complete
    CHECK (
        -- Axis 1: reviewer != maker (always enforced)
        reviewer_id IS NULL OR reviewer_id <> maker_id
    ) NOT VALID;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_sod_approver_vs_maker
    CHECK (
        approver_id IS NULL OR approver_id <> maker_id
    ) NOT VALID;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_sod_approver_vs_reviewer
    CHECK (
        approver_id IS NULL OR reviewer_id IS NULL OR approver_id <> reviewer_id
    ) NOT VALID;

-- Validate existing data (will raise error if any row violates)
ALTER TABLE ecl.eir_reestimation_log VALIDATE CONSTRAINT chk_eir_log_sod_complete;
ALTER TABLE ecl.eir_reestimation_log VALIDATE CONSTRAINT chk_eir_log_sod_approver_vs_maker;
ALTER TABLE ecl.eir_reestimation_log VALIDATE CONSTRAINT chk_eir_log_sod_approver_vs_reviewer;

COMMIT;
