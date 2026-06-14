-- migration: 0027 drift_report_and_amendment_lifecycle — DOWN
-- author: data-modeler
-- requires: 0027 up applied
-- description: Reverses all changes from 000027_drift_report_and_amendment_lifecycle.up.sql.
--
-- Reversal order (reverse of up sections):
--   C → B → A
--
-- WARNING — FK dependency order:
--   ecl.eir_reestimation_log.drift_report_id references sys.drift_report.
--   We must drop that FK column (section C) BEFORE dropping sys.drift_report (section A).
--   Similarly, sys.parameter is independent of sys.drift_report and dropped last (section B).
--
-- WARNING — Data loss:
--   Dropping sys.drift_report and sys.parameter permanently destroys all drift report
--   history and parameter seed rows written since 000027.up ran.
--   Only apply in environments where this data can safely be discarded.

BEGIN;

-- ============================================================
-- C. ecl.eir_reestimation_log — revert M6 lifecycle cols
-- ============================================================

-- C-1. Indexes (drop in reverse creation order)
DROP INDEX IF EXISTS idx_eir_reestimation_cancelled;
DROP INDEX IF EXISTS idx_eir_reestimation_trigger_source_created;
DROP INDEX IF EXISTS idx_eir_reestimation_document_id;
DROP INDEX IF EXISTS idx_eir_reestimation_drift_report_id;
DROP INDEX IF EXISTS idx_eir_reestimation_instrumen_status_active;

-- C-2. Restore chk_eir_log_workflow_status to pre-M6 set (without CANCELLED)
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_workflow_status;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_workflow_status
        CHECK (workflow_status IN (
            'DRAFT', 'PENDING_REVIEW', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED'
        ));

-- C-3. CHECK constraints added in M6
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_reestimation_trigger_source;

ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_reestimation_cancel_reason_len;

-- C-4. Drop M6 columns (reverse add order — FK cols first to release references to A)
ALTER TABLE ecl.eir_reestimation_log
    DROP COLUMN IF EXISTS document_id,
    DROP COLUMN IF EXISTS drift_report_id,
    DROP COLUMN IF EXISTS trigger_source,
    DROP COLUMN IF EXISTS cancelled_by,
    DROP COLUMN IF EXISTS cancel_reason,
    DROP COLUMN IF EXISTS cancelled_at;


-- ============================================================
-- B. sys.parameter — drop table + seed data
-- ============================================================

DROP TABLE IF EXISTS sys.parameter;


-- ============================================================
-- A. sys.drift_report — drop table + supporting objects
-- ============================================================

-- Triggers and function must be dropped before table
DROP TRIGGER  IF EXISTS trg_drift_report_no_hard_delete ON sys.drift_report;
DROP TRIGGER  IF EXISTS trg_drift_report_row_version    ON sys.drift_report;
DROP TRIGGER  IF EXISTS trg_drift_report_updated_at     ON sys.drift_report;

DROP FUNCTION IF EXISTS fn_drift_report_no_hard_delete();

-- Indexes are dropped automatically by DROP TABLE but listed for clarity
DROP INDEX IF EXISTS idx_drift_report_tenant_created;
DROP INDEX IF EXISTS idx_drift_report_triggered_by;
DROP INDEX IF EXISTS idx_drift_report_trigger_source_tanggal;
DROP INDEX IF EXISTS idx_drift_report_status_started;
DROP INDEX IF EXISTS idx_drift_report_tanggal_run;

DROP TABLE IF EXISTS sys.drift_report;

COMMIT;
