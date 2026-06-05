-- migration: 0019 instrumen_schema_fix
-- author: backend-engineer-go
-- requires: 0001, 0008
-- description: (1) Add missing audit cols (deleted_at, deleted_by, tenant_id, row_version)
--              to mst.instrumen which still uses the older is_deleted+version pattern.
--              (2) Add CHECK constraints for workflow_status 7-state, tipe_instrumen
--              whitelist, and sppi_result enum.
--              (3) Backfill deleted_at from is_deleted.
--              (4) Add composite indexes for tenant_created and workflow_status partial.
--              (5) Seed WORKFLOW_CONFIG_INSTRUMEN in sys.config (ON CONFLICT DO NOTHING).

BEGIN;

-- ============================================================
-- 1. ADD MISSING AUDIT COLUMNS
-- ============================================================

ALTER TABLE mst.instrumen
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1;

-- ============================================================
-- 2. BACKFILL deleted_at FROM is_deleted
-- ============================================================

UPDATE mst.instrumen
    SET deleted_at = now()
WHERE is_deleted = TRUE AND deleted_at IS NULL;

-- ============================================================
-- 3. CHECK CONSTRAINTS
-- ============================================================

-- 3a. workflow_status 7-state (column already exists from 0001)
ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_instrumen_workflow_status;

ALTER TABLE mst.instrumen
    ADD CONSTRAINT ck_instrumen_workflow_status
        CHECK (workflow_status IN (
            'DRAFT', 'PENDING_REVIEW', 'PENDING_APPROVAL',
            'PENDING_APPROVAL_2', 'APPROVED', 'REJECTED', 'RETURNED'
        ));

-- 3b. tipe_instrumen whitelist
ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_instrumen_tipe;

ALTER TABLE mst.instrumen
    ADD CONSTRAINT ck_instrumen_tipe
        CHECK (tipe_instrumen IN (
            'DEPOSITO', 'OBLIGASI', 'SAHAM', 'REKSADANA',
            'SBN', 'SPN', 'SUKUK'
        ));

-- 3c. sppi_result enum
ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_instrumen_sppi_result;

ALTER TABLE mst.instrumen
    ADD CONSTRAINT ck_instrumen_sppi_result
        CHECK (sppi_result IS NULL OR sppi_result IN ('PASS', 'FAIL'));

-- ============================================================
-- 4. INDEXES
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_instrumen_tenant_created
    ON mst.instrumen(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_instrumen_workflow_status_partial
    ON mst.instrumen(workflow_status)
    WHERE deleted_at IS NULL;

-- ============================================================
-- 5. SEED WORKFLOW_CONFIG_INSTRUMEN (idempotent)
-- ============================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'WORKFLOW_CONFIG_INSTRUMEN',
    '{
        "entityType": "INSTRUMEN",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "instrumen.submit",
            "review":  "instrumen.review",
            "approve": "instrumen.approve",
            "reject":  "instrumen.reject"
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
    'Workflow config for mst.instrumen — 4-eyes MAKER-TR→RISK(review)→APPR-TR(approve). SPPI test triggered on approve (Phase 4).',
    'WORKFLOW'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
