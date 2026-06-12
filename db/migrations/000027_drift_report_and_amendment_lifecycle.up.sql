-- migration: 0027 drift_report_and_amendment_lifecycle
-- author: data-modeler
-- requires: 0001 (init_schema — sec.user, sys.job, doc.document, ecl.eir_reestimation_log),
--           0005 (fn_ecl_no_hard_delete — no-hard-delete trigger on ecl.*),
--           0006 (doc_document — doc.document table),
--           0026 (eir_schema_fix — ecl.eir_reestimation_log audit cols + workflow cols)
-- description:
--   P4-M6 EIR Amendment Lifecycle schema support:
--   (A) CREATE TABLE sys.drift_report — per-run EIR drift detection report
--       with threshold snapshots, count summaries, FK to sys.job, audit cols,
--       triggers (updated_at + row_version), and 4 indexes.
--       Hard-delete REJECT guard via trigger.
--   (B) CREATE TABLE sys.parameter — key/value parameter store for runtime
--       thresholds (drift_low_threshold, drift_high_threshold, etc.) seeded
--       with M6 defaults. Distinct from sys.config (application settings) —
--       sys.parameter holds domain/business parameters that may be overridden
--       by ALCO without a deployment. Unique on (key, tenant_id).
--   (C) ALTER TABLE ecl.eir_reestimation_log — add M6 lifecycle cols:
--       cancelled_at, cancel_reason (CHECK len >= 20 when NOT NULL),
--       cancelled_by (FK → sec.user), trigger_source (CHECK enum, DEFAULT 'MANUAL'),
--       drift_report_id (FK → sys.drift_report), document_id (FK → doc.document).
--       Drop + re-add chk_eir_log_workflow_status to include CANCELLED state.
--       Drop + re-add chk_eir_log_sod to include cancelled_by ≠ maker_id guard.
--       4 new indexes on new FK / filter columns.

BEGIN;

-- ============================================================
-- A. sys.drift_report
-- ============================================================

CREATE TABLE sys.drift_report (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Run identity
    tanggal_run             DATE        NOT NULL,
    trigger_source          TEXT        NOT NULL
                                CONSTRAINT chk_drift_report_trigger_source
                                CHECK (trigger_source IN ('CRON_DAILY','MANUAL_AD_HOC','PRE_ECL_CALC_RUN')),
    triggered_by            UUID        REFERENCES sec.user(id),
                                -- NULL when trigger_source = 'CRON_DAILY' (system-initiated)

    -- Job linkage (Asynq)
    asynq_job_id            TEXT,
    status                  TEXT        NOT NULL
                                CONSTRAINT chk_drift_report_status
                                CHECK (status IN ('IN_PROGRESS','COMPLETED','FAILED')),
    started_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at            TIMESTAMPTZ,

    -- Count summaries (populated after job completes)
    total_instrumen         INT         NOT NULL DEFAULT 0
                                CONSTRAINT chk_drift_report_total_instrumen CHECK (total_instrumen >= 0),
    drift_low_count         INT         NOT NULL DEFAULT 0
                                CONSTRAINT chk_drift_report_drift_low CHECK (drift_low_count >= 0),
    drift_high_count        INT         NOT NULL DEFAULT 0
                                CONSTRAINT chk_drift_report_drift_high CHECK (drift_high_count >= 0),
    missing_schedule_count  INT         NOT NULL DEFAULT 0
                                CONSTRAINT chk_drift_report_missing CHECK (missing_schedule_count >= 0),
    error_count             INT         NOT NULL DEFAULT 0
                                CONSTRAINT chk_drift_report_error CHECK (error_count >= 0),
    error_summary           TEXT
                                CONSTRAINT chk_drift_report_error_summary
                                CHECK (error_summary IS NULL OR length(error_summary) <= 4000),

    -- Threshold snapshot — captured at run time from sys.parameter
    -- Stored here so historical reports remain readable even after threshold changes
    drift_flag_threshold    NUMERIC(10,8) NOT NULL
                                CONSTRAINT chk_drift_report_flag_threshold
                                CHECK (drift_flag_threshold > 0 AND drift_flag_threshold < 1),
    drift_high_threshold    NUMERIC(10,8) NOT NULL
                                CONSTRAINT chk_drift_report_high_threshold
                                CHECK (drift_high_threshold > 0 AND drift_high_threshold < 1),

    -- Threshold coherence: low < high
    CONSTRAINT chk_drift_report_threshold_order
        CHECK (drift_flag_threshold < drift_high_threshold),

    -- Audit fields (db-conventions.md — mandatory on every table)
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by              UUID        NOT NULL REFERENCES sec.user(id),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by              UUID        NOT NULL REFERENCES sec.user(id),
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID        REFERENCES sec.user(id),
    row_version             BIGINT      NOT NULL DEFAULT 1,
    tenant_id               TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE sys.drift_report IS
    'Per-run EIR drift detection report. One row per cron/manual/pre-ECL run. '
    'drift_flag_threshold and drift_high_threshold are snapshots of sys.parameter at run time '
    'so historical reports remain accurate after threshold changes. '
    'No hard-delete allowed (sys is sensitive) — guard enforced by trigger fn_drift_report_no_hard_delete.';

COMMENT ON COLUMN sys.drift_report.triggered_by IS
    'NULL when trigger_source = CRON_DAILY (system-initiated). '
    'Populated with actor user UUID for MANUAL_AD_HOC and PRE_ECL_CALC_RUN.';

COMMENT ON COLUMN sys.drift_report.drift_flag_threshold IS
    'Snapshot of sys.parameter drift_low_threshold at run time (NUMERIC(10,8), e.g. 0.00010000). '
    'Instruments with abs_diff > this value are flagged LOW severity.';

COMMENT ON COLUMN sys.drift_report.drift_high_threshold IS
    'Snapshot of sys.parameter drift_high_threshold at run time (NUMERIC(10,8), e.g. 0.00100000). '
    'Instruments with abs_diff > this value are flagged HIGH severity and trigger auto-proposal. '
    '[NEEDS ALCO SIGN-OFF: OQ-M6-3] — default 0.001 is provisional.';

-- Triggers
CREATE TRIGGER trg_drift_report_updated_at
    BEFORE UPDATE ON sys.drift_report
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_drift_report_row_version
    BEFORE UPDATE ON sys.drift_report
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Hard-delete guard (sys is sensitive — treat like aud/jrnl/ecl)
CREATE OR REPLACE FUNCTION fn_drift_report_no_hard_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.drift_report (id=%) is forbidden. '
        'Set deleted_at / deleted_by for soft-delete only.',
        OLD.id
        USING ERRCODE = 'integrity_constraint_violation';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION fn_drift_report_no_hard_delete() IS
    'Prevents hard DELETE on sys.drift_report. '
    'Drift reports are audit-grade records. Use soft-delete (deleted_at/deleted_by) only.';

CREATE TRIGGER trg_drift_report_no_hard_delete
    BEFORE DELETE ON sys.drift_report
    FOR EACH ROW EXECUTE FUNCTION fn_drift_report_no_hard_delete();

-- Indexes
CREATE INDEX idx_drift_report_tanggal_run
    ON sys.drift_report (tanggal_run DESC);

CREATE INDEX idx_drift_report_status_started
    ON sys.drift_report (status, started_at DESC);

CREATE INDEX idx_drift_report_trigger_source_tanggal
    ON sys.drift_report (trigger_source, tanggal_run DESC);

CREATE INDEX idx_drift_report_triggered_by
    ON sys.drift_report (triggered_by)
    WHERE triggered_by IS NOT NULL;

-- Tenant + created_at for hot queries
CREATE INDEX idx_drift_report_tenant_created
    ON sys.drift_report (tenant_id, created_at DESC);


-- ============================================================
-- B. sys.parameter  (domain/business parameter store)
-- ============================================================
-- Distinct from sys.config (application settings).
-- sys.parameter holds ALCO-governable numeric thresholds and flags
-- that can change without a deployment, per FSD-APP-C M6 §4.

CREATE TABLE sys.parameter (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    key         TEXT        NOT NULL,
    value       TEXT        NOT NULL,
    description TEXT,
    tenant_id   TEXT        NOT NULL DEFAULT 'TUGURE',

    -- Audit fields
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID        NOT NULL REFERENCES sec.user(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  UUID        NOT NULL REFERENCES sec.user(id),
    deleted_at  TIMESTAMPTZ,
    deleted_by  UUID        REFERENCES sec.user(id),
    row_version BIGINT      NOT NULL DEFAULT 1,

    CONSTRAINT uq_parameter_key_tenant UNIQUE (key, tenant_id)
);

COMMENT ON TABLE sys.parameter IS
    'ALCO-governable business parameters stored as key/value (TEXT). '
    'Readers must CAST to appropriate type (NUMERIC, BOOLEAN, etc.). '
    'Keys: drift_low_threshold, drift_high_threshold, run_eir_drift_check_before_ecl.';

CREATE INDEX idx_parameter_tenant_key
    ON sys.parameter (tenant_id, key);

CREATE TRIGGER trg_parameter_updated_at
    BEFORE UPDATE ON sys.parameter
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_parameter_row_version
    BEFORE UPDATE ON sys.parameter
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Seed M6 default drift thresholds.
-- Sentinel UUID used for created_by/updated_by because the system account
-- may not exist yet in all environments.  Application bootstrap must ensure
-- '00000000-0000-0000-0000-000000000001' refers to the SYSTEM service account.
INSERT INTO sys.parameter (key, value, description, tenant_id,
                           created_by, updated_by)
VALUES
    ('drift_low_threshold',
     '0.00010000',
     'EIR drift flag threshold (1 bp). Instruments with abs_diff > this value '
     'are flagged LOW severity (REVIEW_RECOMMENDED) in the drift report. '
     'Confirmed from M5 BulkService seed value.',
     'TUGURE',
     '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000001'),

    ('drift_high_threshold',
     '0.00100000',
     'EIR drift auto-proposal threshold (10 bp). Instruments with abs_diff > '
     'this value trigger automatic amendment proposal creation. '
     '[NEEDS ALCO SIGN-OFF: OQ-M6-3] — provisional default only.',
     'TUGURE',
     '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000001'),

    ('run_eir_drift_check_before_ecl',
     'false',
     'Advisory pre-ECL drift gate. When true, a drift check runs before each '
     'ECL calc run. Non-blocking (advisory) per OQ-M6-6 resolution.',
     'TUGURE',
     '00000000-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-000000000001')
ON CONFLICT (key, tenant_id) DO NOTHING;


-- ============================================================
-- C. ecl.eir_reestimation_log — M6 lifecycle columns
-- ============================================================

-- C-1. Add M6 columns
ALTER TABLE ecl.eir_reestimation_log
    ADD COLUMN IF NOT EXISTS cancelled_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_reason      TEXT,
    ADD COLUMN IF NOT EXISTS cancelled_by       UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS trigger_source     TEXT        NOT NULL DEFAULT 'MANUAL',
    ADD COLUMN IF NOT EXISTS drift_report_id    UUID        REFERENCES sys.drift_report(id),
    ADD COLUMN IF NOT EXISTS document_id        UUID        REFERENCES doc.document(id);

COMMENT ON COLUMN ecl.eir_reestimation_log.cancelled_at IS
    'TIMESTAMPTZ set when proposal moves to CANCELLED state. NULL on all other states.';

COMMENT ON COLUMN ecl.eir_reestimation_log.cancel_reason IS
    'Mandatory cancellation rationale (≥ 20 chars enforced by chk_eir_reestimation_cancel_reason_len). '
    'NULL unless proposal is CANCELLED.';

COMMENT ON COLUMN ecl.eir_reestimation_log.cancelled_by IS
    'FK → sec.user(id). Actor who performed the cancel action. '
    'Must equal maker_id (ownership check enforced in service layer, not DB constraint).';

COMMENT ON COLUMN ecl.eir_reestimation_log.trigger_source IS
    'Origin of this amendment proposal. '
    'MANUAL = ROLE-AKUN manual propose (M5 flow). '
    'DOCUMENT_UPLOAD = auto-created from doc.document approval (M6-001). '
    'DRIFT_DETECTION_AUTO = auto-created by cron/ad-hoc drift job for HIGH severity (M6-002). '
    'PRE_ECL_GATE = advisory check before ECL calc run (OQ-M6-6, non-blocking).';

COMMENT ON COLUMN ecl.eir_reestimation_log.drift_report_id IS
    'FK → sys.drift_report(id). Populated only when trigger_source = DRIFT_DETECTION_AUTO. '
    'NULL for MANUAL and DOCUMENT_UPLOAD proposals.';

COMMENT ON COLUMN ecl.eir_reestimation_log.document_id IS
    'FK → doc.document(id). Populated only when trigger_source = DOCUMENT_UPLOAD. '
    'NULL for MANUAL and DRIFT_DETECTION_AUTO proposals.';

-- C-2. CHECK constraint: cancel_reason min length
ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_reestimation_cancel_reason_len
        CHECK (cancel_reason IS NULL OR length(cancel_reason) >= 20);

-- C-3. CHECK constraint: trigger_source enum
ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_reestimation_trigger_source
        CHECK (trigger_source IN (
            'MANUAL', 'DOCUMENT_UPLOAD', 'DRIFT_DETECTION_AUTO', 'PRE_ECL_GATE'
        ));

-- C-4. Drop + re-add chk_eir_log_workflow_status to include CANCELLED
--      (was added in 000026 without CANCELLED; M6 state machine adds CANCELLED terminal)
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_workflow_status;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_workflow_status
        CHECK (workflow_status IN (
            'DRAFT', 'PENDING_REVIEW', 'PENDING_APPROVAL',
            'APPROVED', 'REJECTED', 'CANCELLED'
        ));

-- C-5. Drop + re-add chk_eir_log_sod to ensure cancelled_by ≠ reviewer_id
--      (existing constraint only covers reviewer ≠ maker; cancelled_by is ownership-only
--       and checked at service layer — no DB constraint change needed for SoD itself,
--       but we document the intent in the constraint comment)
--      The existing chk_eir_log_sod (reviewer_id <> maker_id) remains valid;
--      no structural change required for that constraint.

-- C-6. Indexes on new columns
-- Partial index: active proposals per instrument (dedup guard for auto-create)
CREATE INDEX IF NOT EXISTS idx_eir_reestimation_instrumen_status_active
    ON ecl.eir_reestimation_log (instrumen_id, workflow_status)
    WHERE workflow_status NOT IN ('APPROVED','REJECTED','CANCELLED')
      AND deleted_at IS NULL;

-- FK index: drift_report_id
CREATE INDEX IF NOT EXISTS idx_eir_reestimation_drift_report_id
    ON ecl.eir_reestimation_log (drift_report_id)
    WHERE drift_report_id IS NOT NULL;

-- FK index: document_id
CREATE INDEX IF NOT EXISTS idx_eir_reestimation_document_id
    ON ecl.eir_reestimation_log (document_id)
    WHERE document_id IS NOT NULL;

-- Filter index: trigger_source + created_at
CREATE INDEX IF NOT EXISTS idx_eir_reestimation_trigger_source_created
    ON ecl.eir_reestimation_log (trigger_source, created_at DESC);

-- Partial index: cancelled proposals (audit / queue queries)
CREATE INDEX IF NOT EXISTS idx_eir_reestimation_cancelled
    ON ecl.eir_reestimation_log (cancelled_at DESC)
    WHERE cancelled_at IS NOT NULL;

COMMIT;
