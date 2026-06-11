-- migration: 0022 staging_engine_tables — DOWN
-- author: data-modeler
-- description: Reverse of 000022_staging_engine_tables.up.sql
-- Order: reverse of up.sql — augmentations first (innermost deps), then tables.
-- WARNING: If any data exists in these tables this rollback will FAIL (FK RESTRICT).
--          Run with zero-data environment only (dev/UAT pre-population).

BEGIN;

-- ============================================================
-- SECTION 3 REVERSAL: ecl.stage_history augmentation
-- ============================================================

-- 3d. Drop indexes added in Section 3d
DROP INDEX IF EXISTS ecl.idx_stage_history_evaluation_job_id;
DROP INDEX IF EXISTS ecl.idx_stage_history_override_proposal_id;
DROP INDEX IF EXISTS ecl.idx_stage_history_tenant_created;
DROP INDEX IF EXISTS ecl.uq_stage_history_idempotency;
DROP INDEX IF EXISTS ecl.idx_stage_history_cure_assessment;

-- 3c. Drop CHECK constraints added in Section 3c
ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_status_approval;

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_trigger_type;

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_stage_sesudah;

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_stage_sebelum;

-- 3b. Drop columns added in Section 3b
ALTER TABLE ecl.stage_history
    DROP COLUMN IF EXISTS evaluation_job_id;

ALTER TABLE ecl.stage_history
    DROP COLUMN IF EXISTS tenant_id;

-- 3a. Drop override_proposal_id FK column from stage_history
--     Must drop BEFORE dropping ecl.staging_override_proposal (FK dependency)
ALTER TABLE ecl.stage_history
    DROP COLUMN IF EXISTS override_proposal_id;

-- ============================================================
-- SECTION 4 REVERSAL: no-delete trigger on staging_override_proposal
-- ============================================================

DROP TRIGGER IF EXISTS tg_ecl_staging_override_no_delete ON ecl.staging_override_proposal;

-- ============================================================
-- SECTION 2 REVERSAL: ecl.staging_override_proposal
-- ============================================================

-- Drop triggers
DROP TRIGGER IF EXISTS trg_sop_row_version   ON ecl.staging_override_proposal;
DROP TRIGGER IF EXISTS trg_sop_updated_at    ON ecl.staging_override_proposal;

-- Drop indexes (most are dropped implicitly with the table, but explicit for clarity)
DROP INDEX IF EXISTS ecl.idx_sop_pending_approval;
DROP INDEX IF EXISTS ecl.idx_sop_tenant_created;
DROP INDEX IF EXISTS ecl.idx_sop_approver_komite_id;
DROP INDEX IF EXISTS ecl.idx_sop_approver_alco_id;
DROP INDEX IF EXISTS ecl.idx_sop_reviewer_id;
DROP INDEX IF EXISTS ecl.idx_sop_periode_id;
DROP INDEX IF EXISTS ecl.idx_sop_expiry_active;
DROP INDEX IF EXISTS ecl.idx_sop_maker_id;
DROP INDEX IF EXISTS ecl.idx_sop_instrumen_status;
DROP INDEX IF EXISTS ecl.idx_sop_workflow_status_created;

-- Drop table
DROP TABLE IF EXISTS ecl.staging_override_proposal;

-- ============================================================
-- SECTION 1 REVERSAL: trx.dpd_record
-- ============================================================

-- Drop triggers
DROP TRIGGER IF EXISTS trg_dpd_record_row_version ON trx.dpd_record;
DROP TRIGGER IF EXISTS trg_dpd_record_updated_at  ON trx.dpd_record;

-- Drop indexes (implicit on DROP TABLE, but explicit for clarity)
DROP INDEX IF EXISTS trx.idx_dpd_record_recorded_by;
DROP INDEX IF EXISTS trx.idx_dpd_record_tenant_created;
DROP INDEX IF EXISTS trx.idx_dpd_record_source_manual;
DROP INDEX IF EXISTS trx.idx_dpd_record_instrumen_periode;

-- Drop table
DROP TABLE IF EXISTS trx.dpd_record;

COMMIT;
