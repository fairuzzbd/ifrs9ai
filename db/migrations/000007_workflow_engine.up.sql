-- migration: 0007 workflow_engine
-- author: data-modeler
-- requires: 0001, 0004, 0005
-- description: Generic workflow_instance + workflow_signature tables per
--              workflow-state-machine.md §7. Config seed for WORKFLOW_CONFIG_* rows.
--              Implements shared tables (not per-entity) using entity_type + entity_id
--              composite — one workflow_instance per entity instance (enforced by UNIQUE).
--              Signature table is APPEND-ONLY: triggers refuse UPDATE and DELETE.

BEGIN;

-- ====================================================================
-- 1. sys.workflow_instance — generic state machine record per entity
-- ====================================================================
CREATE TABLE sys.workflow_instance (
    -- Identity
    id                  UUID    PRIMARY KEY DEFAULT uuidv7(),

    -- Entity link (polymorphic — no FK across schemas, enforced at app layer)
    entity_type         TEXT    NOT NULL,       -- e.g. 'KLASIFIKASI', 'PENEMPATAN', 'ECL_PARAMETER'
    entity_id           UUID    NOT NULL,
    entity_schema       TEXT    NOT NULL,       -- e.g. 'sppi', 'trx', 'ecl'

    -- Config
    workflow_config_key TEXT    NOT NULL,       -- key in sys.config, e.g. WORKFLOW_CONFIG_KLASIFIKASI
    eyes                SMALLINT NOT NULL DEFAULT 4
                            CONSTRAINT ck_wf_eyes CHECK (eyes IN (4, 6)),

    -- State
    current_state       TEXT    NOT NULL DEFAULT 'DRAFT',

    -- Actors (UUID refs — not FK to support cross-schema isolation)
    maker_id            UUID    NOT NULL REFERENCES sec.user(id),
    reviewer_id         UUID    REFERENCES sec.user(id),
    approver1_id        UUID    REFERENCES sec.user(id),
    approver2_id        UUID    REFERENCES sec.user(id),   -- 6-eyes only
    rejected_by         UUID    REFERENCES sec.user(id),

    -- Step timestamps (set once, never overwritten by trigger below)
    submitted_at        TIMESTAMPTZ,
    reviewed_at         TIMESTAMPTZ,
    approved1_at        TIMESTAMPTZ,
    approved2_at        TIMESTAMPTZ,
    rejected_at         TIMESTAMPTZ,

    -- Reject metadata
    reject_comment      TEXT,
    reject_step         TEXT,   -- REVIEW|APPROVE|APPROVE2

    -- Audit fields (db-conventions.md wajib)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by          UUID        NOT NULL REFERENCES sec.user(id),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          UUID        NOT NULL REFERENCES sec.user(id),
    deleted_at          TIMESTAMPTZ,            -- soft-delete if workflow is orphaned
    deleted_by          UUID        REFERENCES sec.user(id),
    row_version         BIGINT      NOT NULL DEFAULT 1,
    tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT ck_wf_state CHECK (
        current_state IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED'
        )
    ),
    -- One active workflow per entity instance
    CONSTRAINT uq_wf_entity UNIQUE (entity_type, entity_id)
);

-- Indexes
CREATE INDEX ix_wf_entity        ON sys.workflow_instance(entity_type, entity_id);
CREATE INDEX ix_wf_state         ON sys.workflow_instance(current_state) WHERE deleted_at IS NULL;
CREATE INDEX ix_wf_maker         ON sys.workflow_instance(maker_id, created_at DESC);
CREATE INDEX ix_wf_reviewer      ON sys.workflow_instance(reviewer_id) WHERE reviewer_id IS NOT NULL AND current_state = 'PENDING_REVIEW';
CREATE INDEX ix_wf_approver1     ON sys.workflow_instance(approver1_id) WHERE approver1_id IS NOT NULL AND current_state IN ('PENDING_APPROVAL','PENDING_APPROVAL_2');
CREATE INDEX ix_wf_tenant_state  ON sys.workflow_instance(tenant_id, current_state, created_at DESC);

-- Auto-update triggers
CREATE TRIGGER tg_wf_instance_updated_at
    BEFORE UPDATE ON sys.workflow_instance
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER tg_wf_instance_row_version
    BEFORE UPDATE ON sys.workflow_instance
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Immutability of signing timestamps: once set, submitted_at/reviewed_at/approved*_at/rejected_at
-- MUST NOT be overwritten. Enforce at DB level.
CREATE OR REPLACE FUNCTION fn_wf_protect_signing_timestamps()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.submitted_at  IS NOT NULL AND NEW.submitted_at  IS DISTINCT FROM OLD.submitted_at  THEN
        RAISE EXCEPTION 'workflow_instance.submitted_at is immutable once set'
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.reviewed_at   IS NOT NULL AND NEW.reviewed_at   IS DISTINCT FROM OLD.reviewed_at   THEN
        RAISE EXCEPTION 'workflow_instance.reviewed_at is immutable once set'
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.approved1_at  IS NOT NULL AND NEW.approved1_at  IS DISTINCT FROM OLD.approved1_at  THEN
        RAISE EXCEPTION 'workflow_instance.approved1_at is immutable once set'
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.approved2_at  IS NOT NULL AND NEW.approved2_at  IS DISTINCT FROM OLD.approved2_at  THEN
        RAISE EXCEPTION 'workflow_instance.approved2_at is immutable once set'
            USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.rejected_at   IS NOT NULL AND NEW.rejected_at   IS DISTINCT FROM OLD.rejected_at   THEN
        RAISE EXCEPTION 'workflow_instance.rejected_at is immutable once set'
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_wf_protect_timestamps
    BEFORE UPDATE ON sys.workflow_instance
    FOR EACH ROW EXECUTE FUNCTION fn_wf_protect_signing_timestamps();

COMMENT ON TABLE sys.workflow_instance IS
    'Generic Maker-Reviewer-Approver state machine per workflow-state-machine.md. '
    'One record per entity instance (uq_wf_entity). '
    'Config-driven: eyes=4 or 6 from workflow_config_key → sys.config. '
    'Signing timestamps are immutable once set (trigger enforced).';

-- ====================================================================
-- 2. sys.workflow_signature — append-only signing ledger
-- ====================================================================
-- Each workflow action (SUBMIT, REVIEW, APPROVE, APPROVE2, REJECT) creates
-- one row. NEVER UPDATE or DELETE. Trigger enforces this.
-- signature_hash = SHA-256(userId || action || entityId || signedAt || comment)
-- computed by Go workflow engine before insert.

CREATE TABLE sys.workflow_signature (
    id                  UUID        PRIMARY KEY DEFAULT uuidv7(),
    workflow_id         UUID        NOT NULL REFERENCES sys.workflow_instance(id),
    action              TEXT        NOT NULL,   -- SUBMIT|REVIEW|APPROVE|APPROVE2|REJECT
    user_id             UUID        NOT NULL REFERENCES sec.user(id),
    role_at_time        TEXT        NOT NULL,   -- snapshot of user's role at signing moment
    signed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    signature_hash      TEXT        NOT NULL,   -- hex SHA-256
    signature_method    TEXT        NOT NULL,   -- JWT_STEP_UP|JWT_STANDARD
    comment             TEXT,
    tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',
    -- NO updated_at/deleted_at — this record is IMMUTABLE (append-only)
    CONSTRAINT ck_sig_action   CHECK (action IN ('SUBMIT','REVIEW','APPROVE','APPROVE2','REJECT')),
    CONSTRAINT ck_sig_method   CHECK (signature_method IN ('JWT_STEP_UP','JWT_STANDARD'))
);

-- Indexes
CREATE INDEX ix_wf_sig_workflow    ON sys.workflow_signature(workflow_id, signed_at ASC);
CREATE INDEX ix_wf_sig_user        ON sys.workflow_signature(user_id, signed_at DESC);
CREATE INDEX ix_wf_sig_tenant      ON sys.workflow_signature(tenant_id, signed_at DESC);

-- APPEND-ONLY enforcement: refuse UPDATE and DELETE
CREATE OR REPLACE FUNCTION fn_wf_signature_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'workflow_signature records are immutable (append-only). '
        'Action: %, Table: sys.workflow_signature',
        TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_wf_signature_no_update
    BEFORE UPDATE ON sys.workflow_signature
    FOR EACH ROW EXECUTE FUNCTION fn_wf_signature_immutable();

CREATE TRIGGER tg_wf_signature_no_delete
    BEFORE DELETE ON sys.workflow_signature
    FOR EACH ROW EXECUTE FUNCTION fn_wf_signature_immutable();

COMMENT ON TABLE sys.workflow_signature IS
    'Append-only signing ledger. Each Maker/Reviewer/Approver action creates one row. '
    'signature_hash = SHA-256(userId||action||entityId||signedAt||comment). '
    'Computed by Go workflow engine. UPDATE and DELETE refused by trigger.';

-- ====================================================================
-- 3. Seed sys.config with WORKFLOW_CONFIG_* rows
-- ====================================================================
-- Per workflow-state-machine.md §5 and §8.
-- These configs are read by WorkflowEngine at startup (cached 5 min).

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES
-- 4-eyes configs ---------------------------------------------------------
(
    'WORKFLOW_CONFIG_PENEMPATAN',
    '{
        "entityType": "PENEMPATAN",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "penempatan.submit",
            "review":  "penempatan.review",
            "approve": "penempatan.approve",
            "reject":  "penempatan.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for trx.penempatan — 4-eyes Maker→Reviewer→Approver',
    'WORKFLOW'
),
(
    'WORKFLOW_CONFIG_JURNAL',
    '{
        "entityType": "JURNAL",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "jurnal.submit",
            "review":  "jurnal.review",
            "approve": "jurnal.approve",
            "reject":  "jurnal.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for jrnl entries — 4-eyes',
    'WORKFLOW'
),
(
    'WORKFLOW_CONFIG_PERIODE',
    '{
        "entityType": "PERIODE",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "periode.softclose",
            "review":  "periode.softclose",
            "approve": "periode.hardclose",
            "reject":  "periode.reject"
        },
        "stepUpRequired": {
            "approve": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.periode_buku close — 4-eyes, approve requires step-up MFA (DEC-027)',
    'WORKFLOW'
),
(
    'WORKFLOW_CONFIG_UPLOAD_BATCH',
    '{
        "entityType": "UPLOAD_BATCH",
        "eyes": 4,
        "retractable": true,
        "requiredPermissions": {
            "submit":  "upload_batch.submit",
            "review":  "upload_batch.review",
            "approve": "upload_batch.approve",
            "reject":  "upload_batch.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for sys.upload_batch — 4-eyes, retractable=true',
    'WORKFLOW'
),
-- 6-eyes configs ---------------------------------------------------------
(
    'WORKFLOW_CONFIG_KLASIFIKASI',
    '{
        "entityType": "KLASIFIKASI",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "klasifikasi.submit",
            "review":   "klasifikasi.review",
            "approve":  "klasifikasi.approve",
            "approve2": "klasifikasi.approve",
            "reject":   "klasifikasi.reject"
        },
        "stepUpRequired": {
            "approve":  false,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for sppi.klasifikasi (SPPI × BM → PSAK 71) — 6-eyes, approve2 requires step-up MFA (DEC-017/027)',
    'WORKFLOW'
),
(
    'WORKFLOW_CONFIG_ECL_PARAMETER',
    '{
        "entityType": "ECL_PARAMETER",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for ecl parameters (PD curve, LGD pool, scenario weights, FL multiplier) — 6-eyes, both approvals require step-up MFA (DEC-027)',
    'WORKFLOW'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
