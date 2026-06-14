-- migration: 0018 portofolio_schema_fix
-- author: backend-engineer-go
-- requires: 0001, 0007, 0008
-- description: Add missing audit columns (deleted_at, deleted_by, tenant_id, workflow_status)
--              to mst.portofolio. Backfill existing rows. Add indexes.
--              Schema drift note: 0001 uses version+is_deleted (legacy pattern);
--              this migration introduces deleted_at/deleted_by/tenant_id/workflow_status
--              to align with the canonical audit-column pattern from db-conventions.md.
--              The columns version and is_deleted are retained for co-existence;
--              application code uses deleted_at (not is_deleted) for soft-delete going forward.

BEGIN;

-- ============================================================
-- 1. ADD MISSING AUDIT COLUMNS
-- ============================================================

ALTER TABLE mst.portofolio
    ADD COLUMN IF NOT EXISTS deleted_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by       UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS tenant_id        TEXT NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status  VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- ============================================================
-- 2. ADD WORKFLOW_STATUS CONSTRAINT
-- ============================================================

ALTER TABLE mst.portofolio
    DROP CONSTRAINT IF EXISTS chk_portofolio_workflow_status;

ALTER TABLE mst.portofolio
    ADD CONSTRAINT chk_portofolio_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- ============================================================
-- 3. BACKFILL EXISTING ROWS
-- ============================================================

-- Rows that were soft-deleted using is_deleted=TRUE: populate deleted_at.
-- Use updated_at as the best estimate of deletion time (fallback to now()).
UPDATE mst.portofolio
    SET deleted_at = COALESCE(updated_at, now()),
        workflow_status = 'APPROVED'
WHERE is_deleted = TRUE
  AND deleted_at IS NULL;

-- Active rows: mark as APPROVED (they were in use pre-workflow).
UPDATE mst.portofolio
    SET workflow_status = 'APPROVED'
WHERE is_deleted = FALSE
  AND workflow_status = 'DRAFT';

-- ============================================================
-- 4. UPDATE UNIQUE INDEX TO USE deleted_at (not is_deleted)
-- ============================================================

-- Drop the old unique index on is_deleted=FALSE; replace with deleted_at IS NULL
-- so soft-deleted rows are excluded from the uniqueness constraint going forward.
DROP INDEX IF EXISTS uq_portofolio_kode;
CREATE UNIQUE INDEX uq_portofolio_kode ON mst.portofolio(kode_portofolio) WHERE deleted_at IS NULL;

-- ============================================================
-- 5. INDEXES
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_portofolio_tenant_created
    ON mst.portofolio(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_portofolio_workflow_status
    ON mst.portofolio(workflow_status) WHERE deleted_at IS NULL;

-- ============================================================
-- 6. sys.config — SEED WORKFLOW_CONFIG_PORTOFOLIO
--    (already seeded in 0008; ON CONFLICT DO NOTHING is idempotent)
-- ============================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'WORKFLOW_CONFIG_PORTOFOLIO',
    '{
        "entityType": "PORTOFOLIO",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "portofolio.submit",
            "review":  "portofolio.review",
            "approve": "portofolio.approve",
            "reject":  "portofolio.reject"
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
    'Workflow config for mst.portofolio — 4-eyes MAKER-TR→RISK(review)→APPR-TR(approve)',
    'WORKFLOW'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
