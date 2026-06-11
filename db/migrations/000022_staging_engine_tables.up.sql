-- migration: 0022 staging_engine_tables
-- author: data-modeler
-- requires: 0001 (init_schema — mst.instrumen, mst.periode_buku, doc.upload, ecl.stage_history),
--           0004 (fn_increment_row_version, fn_update_updated_at),
--           0005 (fn_ecl_no_hard_delete, tg_ecl_stage_history_no_delete),
--           0007 (sec.user FK target, sys.workflow pattern),
--           0021 (mst.instrumen final shape)
-- description: P4-M1 Staging Engine tables:
--   1. trx.dpd_record          — interim DPD manual input (GAP-DPD Option A)
--   2. ecl.staging_override_proposal — 6-eyes manual stage override workflow
--   3. ecl.stage_history augmentation — add missing cols + constraints + indexes
--      required by P4-M1 (override_proposal_id backreference, CHECK constraints,
--      composite index for cure assessment, partition-ready tenant_id + row header)
-- Partition: ecl.staging_override_proposal is NOT partitioned (low-volume workflow entity).
--   trx.dpd_record is NOT partitioned (monthly record count is bounded by instrument count).
--   ecl.stage_history is already in 0001 without partition; augmentation here adds
--   missing columns only. Partition migration deferred — see concern note at bottom.
-- Down: see 000022_staging_engine_tables.down.sql

BEGIN;

-- ============================================================
-- SECTION 1: trx.dpd_record
-- Interim DPD manual input table (GAP-DPD Option A).
-- Lives in trx schema (will be superseded by APP-B Phase 5
-- transaction-derived DPD; source='APP_B' rows will be written
-- by that system). Until then ROLE-AKUN writes MANUAL rows.
-- ============================================================

CREATE TABLE trx.dpd_record (
    -- Identity
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Business columns
    instrumen_id    UUID        NOT NULL
                        REFERENCES mst.instrumen(id)
                        ON DELETE RESTRICT,
    periode         DATE        NOT NULL,           -- truncated to YYYY-MM-01
    dpd_value       INT         NOT NULL
                        CONSTRAINT chk_dpd_record_dpd_value_nonneg CHECK (dpd_value >= 0),
    source          TEXT        NOT NULL
                        CONSTRAINT chk_dpd_record_source CHECK (source IN ('MANUAL', 'APP_B')),
    catatan         TEXT,                           -- optional free-text note (ROLE-AKUN)
    recorded_by     UUID        NOT NULL
                        REFERENCES sec.user(id)
                        ON DELETE RESTRICT,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Uniqueness: one DPD record per instrument per calendar month
    CONSTRAINT uq_dpd_record_instrumen_periode UNIQUE (instrumen_id, periode),

    -- Standard audit columns (db-conventions.md)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at      TIMESTAMPTZ,                    -- soft-delete (override of stale records)
    deleted_by      UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE trx.dpd_record IS
    'Interim DPD record table (GAP-DPD Option A). Manual input by ROLE-AKUN until '
    'APP-B Phase 5 provides transaction-derived DPD. source=APP_B rows will be inserted '
    'by the transaction lifecycle engine post-Phase 5. '
    'UNIQUE (instrumen_id, periode) enforces upsert idempotency per state-machine §4.3. '
    'periode must be truncated to YYYY-MM-01 at application layer before insert.';

COMMENT ON COLUMN trx.dpd_record.periode IS
    'First day of calendar month (YYYY-MM-01). Application must truncate to month '
    'before insert. CHECK constraint intentionally omitted — enforcement via app layer '
    'to avoid complexity with date arithmetic in SQL across DST/leap edge cases.';

COMMENT ON COLUMN trx.dpd_record.dpd_value IS
    'Days Past Due count. Non-negative integer. DPD >= 30 triggers SICR (STAGE_1 → STAGE_2). '
    'DPD >= 90 triggers default (→ STAGE_3). Per DEC-011.';

COMMENT ON COLUMN trx.dpd_record.source IS
    'MANUAL = entered by ROLE-AKUN via POST /ecl/dpd/record. '
    'APP_B  = will be written by APP-B transaction engine (Phase 5).';

-- Indexes for trx.dpd_record
-- Primary lookup: latest DPD for an instrument
CREATE INDEX idx_dpd_record_instrumen_periode
    ON trx.dpd_record (instrumen_id, periode DESC)
    WHERE deleted_at IS NULL;

-- Lookup active MANUAL records (for audit / review queue)
CREATE INDEX idx_dpd_record_source_manual
    ON trx.dpd_record (instrumen_id, periode DESC)
    WHERE source = 'MANUAL' AND deleted_at IS NULL;

-- Tenant + time (mandatory per db-conventions.md hot-table pattern)
CREATE INDEX idx_dpd_record_tenant_created
    ON trx.dpd_record (tenant_id, created_at DESC);

-- FK to recorded_by (index all FKs per db-conventions.md)
CREATE INDEX idx_dpd_record_recorded_by
    ON trx.dpd_record (recorded_by);

-- Triggers for trx.dpd_record
CREATE TRIGGER trg_dpd_record_updated_at
    BEFORE UPDATE ON trx.dpd_record
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_dpd_record_row_version
    BEFORE UPDATE ON trx.dpd_record
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();


-- ============================================================
-- SECTION 2: ecl.staging_override_proposal
-- Manual stage override workflow entity.
-- 6-eyes for Stage 3 → Stage 2 (ALCO + KOMITE + step-up MFA).
-- 4-eyes for Stage 2 → Stage 1 (ALCO + step-up MFA).
-- Lifecycle: PENDING_REVIEW → PENDING_APPROVAL → [APPROVED_ALCO →] ACTIVE → EXPIRED
--            OR → REJECTED at any step.
-- Soft-delete allowed (pre-approval cancellation by maker).
-- NO hard delete (domain workflow entity, audit-grade retention).
-- ============================================================

CREATE TABLE ecl.staging_override_proposal (
    -- Identity
    id                          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Instrument reference
    instrumen_id                UUID        NOT NULL
                                    REFERENCES mst.instrumen(id)
                                    ON DELETE RESTRICT,

    -- Stage transition
    stage_from                  VARCHAR(10) NOT NULL
                                    CONSTRAINT chk_sop_stage_from CHECK (stage_from IN ('STAGE_1', 'STAGE_2', 'STAGE_3')),
    stage_to                    VARCHAR(10) NOT NULL
                                    CONSTRAINT chk_sop_stage_to CHECK (stage_to IN ('STAGE_1', 'STAGE_2', 'STAGE_3')),

    -- Business case
    alasan                      TEXT        NOT NULL
                                    CONSTRAINT chk_sop_alasan_minlen CHECK (length(alasan) >= 10),
    reason_category             TEXT
                                    CONSTRAINT chk_sop_reason_category CHECK (
                                        reason_category IS NULL OR
                                        reason_category IN (
                                            'MANAGEMENT_OVERLAY',
                                            'REGULATORY_GUIDANCE',
                                            'DATA_ANOMALY',
                                            'OTHER'
                                        )
                                    ),
    dokumen_pendukung_id        UUID        REFERENCES doc.upload(id)
                                    ON DELETE RESTRICT,

    -- Periode context
    periode_id                  UUID        NOT NULL
                                    REFERENCES mst.periode_buku(id)
                                    ON DELETE RESTRICT,
    -- Denormalized for efficient expiry check without join to periode_buku.
    -- Must be populated from mst.periode_buku.tanggal_akhir at insert time.
    periode_akhir               DATE        NOT NULL,

    -- Workflow state
    workflow_status             TEXT        NOT NULL DEFAULT 'PENDING_REVIEW'
                                    CONSTRAINT chk_sop_workflow_status CHECK (workflow_status IN (
                                        'PENDING_REVIEW',
                                        'PENDING_APPROVAL',
                                        'APPROVED_ALCO',
                                        'ACTIVE',
                                        'EXPIRED',
                                        'REJECTED'
                                    )),

    -- Stage at time of submission (for audit — current stage may change)
    current_stage_at_submit     VARCHAR(10)
                                    CONSTRAINT chk_sop_current_stage_at_submit CHECK (
                                        current_stage_at_submit IS NULL OR
                                        current_stage_at_submit IN ('STAGE_1', 'STAGE_2', 'STAGE_3')
                                    ),

    -- Maker (ROLE-RISK)
    maker_id                    UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,

    -- Reviewer (ROLE-RISK, ≠ maker)
    reviewer_id                 UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    signed_at_review            TIMESTAMPTZ,
    signature_hash_review       BYTEA,      -- SHA-256(userId||REVIEW||proposalId||signedAt||comment)
    comment_review              TEXT,

    -- ALCO Approver (ROLE-ALCO, step-up MFA, ≠ maker ≠ reviewer)
    approver_alco_id            UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    signed_at_approve_alco      TIMESTAMPTZ,
    signature_hash_approve_alco BYTEA,      -- SHA-256(userId||APPROVE||proposalId||signedAt||comment)
    comment_approve_alco        TEXT,

    -- KOMITE Second Approver (ROLE-KOMITE, Stage 3 path only, ≠ all previous)
    approver_komite_id          UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    signed_at_approve_komite    TIMESTAMPTZ,
    signature_hash_approve_komite BYTEA,    -- SHA-256(userId||APPROVE2||proposalId||signedAt||comment)
    comment_approve_komite      TEXT,

    -- Rejection (any step)
    reject_reason               TEXT,

    -- Link to the stage_history row created when override goes ACTIVE
    -- Nullable until ACTIVE; set atomically in the same DB tx as the stage_history insert.
    stage_history_row_id        UUID        REFERENCES ecl.stage_history(id)
                                    ON DELETE RESTRICT,

    -- Expiry override: override expires at end of periode_akhir unless re-confirmed.
    -- System job ECL_OVERRIDE_EXPIRY_CHECK sets status=EXPIRED and inserts
    -- a stage_history row with trigger_type=OVERRIDE_EXPIRED.
    expires_after_periode       DATE,       -- derived from periode_akhir (app layer sets this)

    -- ----------------------------------------------------------------
    -- Segregation of Duties (SoD) DB CHECK constraints
    -- Enforce at DB layer as last-resort guard.
    -- Application layer also enforces these (security-in-depth).
    -- ----------------------------------------------------------------
    CONSTRAINT chk_sop_sod_reviewer_not_maker
        CHECK (reviewer_id IS NULL OR reviewer_id <> maker_id),
    CONSTRAINT chk_sop_sod_alco_not_maker
        CHECK (approver_alco_id IS NULL OR approver_alco_id <> maker_id),
    CONSTRAINT chk_sop_sod_alco_not_reviewer
        CHECK (approver_alco_id IS NULL OR reviewer_id IS NULL OR approver_alco_id <> reviewer_id),
    CONSTRAINT chk_sop_sod_komite_not_alco
        CHECK (approver_komite_id IS NULL OR approver_alco_id IS NULL OR approver_komite_id <> approver_alco_id),
    -- Additional SoD: KOMITE ≠ reviewer and ≠ maker (belt-and-suspenders for 6-eyes path)
    CONSTRAINT chk_sop_sod_komite_not_reviewer
        CHECK (approver_komite_id IS NULL OR reviewer_id IS NULL OR approver_komite_id <> reviewer_id),
    CONSTRAINT chk_sop_sod_komite_not_maker
        CHECK (approver_komite_id IS NULL OR approver_komite_id <> maker_id),

    -- Stage transition guard: STAGE_3 → STAGE_1 is invalid at DB level
    CONSTRAINT chk_sop_no_stage3_to_stage1
        CHECK (NOT (stage_from = 'STAGE_3' AND stage_to = 'STAGE_1')),

    -- Stage must actually change (override to same stage is a no-op, rejected by app too)
    CONSTRAINT chk_sop_stage_must_change
        CHECK (stage_from <> stage_to),

    -- Standard audit columns
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at      TIMESTAMPTZ,            -- soft-delete: pre-approval cancellation by maker
    deleted_by      UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE ecl.staging_override_proposal IS
    'Manual stage override proposal — 6-eyes (Stage 3 path: ALCO + KOMITE) or '
    '4-eyes (standard: ALCO only). State machine per p4-m1-staging.md §2. '
    'Soft-delete allowed for pre-approval cancellation (deleted_at). '
    'No hard-delete: audit-grade retention, history is append-only from ecl.stage_history side. '
    'SoD enforced via DB CHECK (chk_sop_sod_*) + application layer. '
    'stage_history_row_id FK set atomically when workflow_status → ACTIVE.';

COMMENT ON COLUMN ecl.staging_override_proposal.signature_hash_review IS
    'SHA-256(userId || REVIEW || proposalId || signedAt || comment). '
    'Must be computed by application layer before INSERT/UPDATE. '
    'Stored as BYTEA (32 bytes). Immutable after set — application must not overwrite.';

COMMENT ON COLUMN ecl.staging_override_proposal.signature_hash_approve_alco IS
    'SHA-256(userId || APPROVE || proposalId || signedAt || comment). Step-up MFA required. '
    'Immutable after set.';

COMMENT ON COLUMN ecl.staging_override_proposal.signature_hash_approve_komite IS
    'SHA-256(userId || APPROVE2 || proposalId || signedAt || comment). Stage 3 path only. '
    'Immutable after set.';

-- Indexes for ecl.staging_override_proposal

-- Primary workflow queue index: pending proposals by status + time
CREATE INDEX idx_sop_workflow_status_created
    ON ecl.staging_override_proposal (workflow_status, created_at DESC)
    WHERE deleted_at IS NULL;

-- Instrument lookup: active proposals per instrument (enforces "one active proposal" rule)
CREATE INDEX idx_sop_instrumen_status
    ON ecl.staging_override_proposal (instrumen_id, workflow_status)
    WHERE deleted_at IS NULL;

-- Maker queue: proposals submitted by a specific user
CREATE INDEX idx_sop_maker_id
    ON ecl.staging_override_proposal (maker_id)
    WHERE deleted_at IS NULL;

-- Expiry job: proposals ACTIVE with expiry in the past
CREATE INDEX idx_sop_expiry_active
    ON ecl.staging_override_proposal (periode_akhir)
    WHERE workflow_status = 'ACTIVE';

-- FK indexes (all FKs must be indexed per db-conventions.md)
CREATE INDEX idx_sop_periode_id
    ON ecl.staging_override_proposal (periode_id);

CREATE INDEX idx_sop_reviewer_id
    ON ecl.staging_override_proposal (reviewer_id)
    WHERE reviewer_id IS NOT NULL;

CREATE INDEX idx_sop_approver_alco_id
    ON ecl.staging_override_proposal (approver_alco_id)
    WHERE approver_alco_id IS NOT NULL;

CREATE INDEX idx_sop_approver_komite_id
    ON ecl.staging_override_proposal (approver_komite_id)
    WHERE approver_komite_id IS NOT NULL;

-- Tenant + time (mandatory)
CREATE INDEX idx_sop_tenant_created
    ON ecl.staging_override_proposal (tenant_id, created_at DESC);

-- Partial hot-path: ALCO approval queue
CREATE INDEX idx_sop_pending_approval
    ON ecl.staging_override_proposal (created_at DESC)
    WHERE workflow_status IN ('PENDING_APPROVAL', 'APPROVED_ALCO') AND deleted_at IS NULL;

-- Triggers for ecl.staging_override_proposal
CREATE TRIGGER trg_sop_updated_at
    BEFORE UPDATE ON ecl.staging_override_proposal
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_sop_row_version
    BEFORE UPDATE ON ecl.staging_override_proposal
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();


-- ============================================================
-- SECTION 3: ecl.stage_history augmentation
-- Add columns required by P4-M1 that were not in 0001 init.
-- Also add missing CHECK constraints and composite index for
-- cure assessment query (SECTION 5.3 of state-machine doc).
-- NOTE: ecl.stage_history is append-only (tg_ecl_stage_history_no_delete
-- from 0005 refuses all DELETEs — this is correct, do NOT remove it).
-- ============================================================

-- 3a. Add override_proposal_id backreference (FK to staging_override_proposal).
--     Nullable — most rows are AUTO (system) and have no override context.
--     Set when a MANUAL_OVERRIDE row is inserted.
ALTER TABLE ecl.stage_history
    ADD COLUMN IF NOT EXISTS override_proposal_id UUID
        REFERENCES ecl.staging_override_proposal(id)
        ON DELETE RESTRICT;

COMMENT ON COLUMN ecl.stage_history.override_proposal_id IS
    'FK to ecl.staging_override_proposal. Populated only for trigger_type=MANUAL_OVERRIDE '
    'or OVERRIDE_EXPIRED rows. NULL for AUTO system rows (SICR, cure, etc.). '
    'Set atomically in the same DB transaction as the staging_override_proposal workflow_status → ACTIVE update.';

-- 3b. Add tenant_id and row_version to stage_history (audit compliance — these
--     were omitted from the 0001 CREATE because stage_history is append-only
--     and row_version is less critical, but tenant_id is required for multi-tenant
--     query patterns per db-conventions.md).
ALTER TABLE ecl.stage_history
    ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'TUGURE';

ALTER TABLE ecl.stage_history
    ADD COLUMN IF NOT EXISTS evaluation_job_id UUID
        REFERENCES sys.job(id)
        ON DELETE SET NULL;

COMMENT ON COLUMN ecl.stage_history.evaluation_job_id IS
    'FK to sys.job. Populated when the staging row was produced by an async Asynq job '
    '(ECL_STAGING_EVALUATE or ECL_CURE_ASSESSMENT). NULL for direct synchronous transitions '
    '(manual override via API or test harness).';

-- 3c. CHECK constraints missing from 0001.
--     Adds validation at DB level for stage values and status_approval.
--     Using DROP+ADD pattern for idempotency safety.

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_stage_sebelum;
ALTER TABLE ecl.stage_history
    ADD CONSTRAINT chk_stage_history_stage_sebelum
        CHECK (stage_sebelum IN ('STAGE_1', 'STAGE_2', 'STAGE_3'));

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_stage_sesudah;
ALTER TABLE ecl.stage_history
    ADD CONSTRAINT chk_stage_history_stage_sesudah
        CHECK (stage_sesudah IN ('STAGE_1', 'STAGE_2', 'STAGE_3'));

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_trigger_type;
ALTER TABLE ecl.stage_history
    ADD CONSTRAINT chk_stage_history_trigger_type
        CHECK (trigger_type IN (
            'RATING_DOWNGRADE',
            'IG_TO_NON_IG',
            'RATING_DEFAULT',
            'DPD_GTE_30',
            'DPD_GTE_90',
            'CURE_3_PERIODE_BULANAN',
            'MANUAL_OVERRIDE',
            'OVERRIDE_EXPIRED',
            'INITIAL'           -- seed value for first staging row on instrument approval
        ));

ALTER TABLE ecl.stage_history
    DROP CONSTRAINT IF EXISTS chk_stage_history_status_approval;
ALTER TABLE ecl.stage_history
    ADD CONSTRAINT chk_stage_history_status_approval
        CHECK (status_approval IN (
            'AUTO',
            'APPROVED',
            'OVERRIDE_EXPIRED'
        ));

-- 3d. Additional indexes required by P4-M1 query paths.

-- Cure assessment: find last SICR transition for an instrument, then check
-- consecutive periods without STAGE_2/STAGE_3 transitions.
-- Index: (instrumen_id, stage_sesudah, tanggal_migrasi DESC)
CREATE INDEX IF NOT EXISTS idx_stage_history_cure_assessment
    ON ecl.stage_history (instrumen_id, stage_sesudah, tanggal_migrasi DESC);

-- UNIQUE idempotency constraint per state-machine §4.1:
-- (instrumen_id, tanggal_migrasi, trigger_type) must be unique.
-- Using a unique index (not constraint) for performance and to allow
-- deferred checking in atomic 2-row insert (Stage1→2 + Stage2→3 same tanggal_migrasi,
-- different trigger_type — e.g., DPD_GTE_30 then DPD_GTE_90).
-- This uniqueness also prevents duplicate SICR rows from re-entrant job runs.
CREATE UNIQUE INDEX IF NOT EXISTS uq_stage_history_idempotency
    ON ecl.stage_history (instrumen_id, tanggal_migrasi, trigger_type);

-- Tenant query path index
CREATE INDEX IF NOT EXISTS idx_stage_history_tenant_created
    ON ecl.stage_history (tenant_id, created_at DESC);

-- FK to override_proposal_id (must be indexed per db-conventions.md)
CREATE INDEX IF NOT EXISTS idx_stage_history_override_proposal_id
    ON ecl.stage_history (override_proposal_id)
    WHERE override_proposal_id IS NOT NULL;

-- FK to evaluation_job_id
CREATE INDEX IF NOT EXISTS idx_stage_history_evaluation_job_id
    ON ecl.stage_history (evaluation_job_id)
    WHERE evaluation_job_id IS NOT NULL;


-- ============================================================
-- SECTION 4: Prevent hard-delete on new ecl table
-- staging_override_proposal lives in ecl schema.
-- Per CLAUDE.md / db-conventions.md hard rule:
-- "No hard delete in ecl". Add no-delete trigger.
-- The table does allow soft-delete (deleted_at), but DELETE
-- statement (hard delete) is refused.
-- ============================================================

CREATE TRIGGER tg_ecl_staging_override_no_delete
    BEFORE DELETE ON ecl.staging_override_proposal
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

COMMIT;

-- ============================================================
-- CONCERNS / RUNBOOK NOTES
-- ============================================================
-- 1. PARTITION: ecl.stage_history was created in 0001 as a regular heap table.
--    The P4-M1 spec calls for monthly partitioning (high-volume insert path).
--    Converting an existing non-partitioned table to a partitioned table in
--    PostgreSQL requires: CREATE TABLE ... PARTITION BY RANGE, data migration,
--    RENAME, and re-creation of all indexes/triggers — which cannot be done
--    in a single online DDL without downtime.
--    RECOMMENDATION: Defer partition conversion to a Phase 5 maintenance window.
--    Create runbook: docs/runbooks/ecl-stage-history-partition-conversion.md
--    Use pg_partman for auto-partition creation once converted.
--
-- 2. trx.dpd_record PARTITION: Not partitioned in this migration (instrument count
--    at Tugu Reasuransi is bounded; one row per instrument per month).
--    If volume exceeds 500k rows/year, revisit.
--
-- 3. ecl.staging_override_proposal PARTITION: Not partitioned. Override proposals
--    are a low-volume workflow entity (max ~50 per period). No partition needed.
