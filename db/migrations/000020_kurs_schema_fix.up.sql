-- migration: 0020 kurs_schema_fix
-- author: backend-engineer-go
-- requires: 0001, 0007, 0008
-- description: Backfill mst.kurs with missing audit cols (created_by, updated_by,
--              deleted_at, deleted_by, row_version, tenant_id, workflow_status).
--              Adds CHECK for workflow_status 7-state and sumber_kurs whitelist.
--              Backfills APPROVED for rows that already have approver_id set.
--              Adds indexes for tenant + workflow_status.

BEGIN;

-- ============================================================
-- 1. mst.kurs — ADD MISSING AUDIT COLUMNS
--    Legacy cols (maker_id, approver_id, approved_at) are kept intact.
-- ============================================================

ALTER TABLE mst.kurs
    ADD COLUMN IF NOT EXISTS created_by     UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_by     UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at     TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by     UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version    BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id      TEXT   NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- Backfill updated_at from created_at where missing (created_at already existed in 0001)
ALTER TABLE mst.kurs
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

-- ============================================================
-- 2. CHECK CONSTRAINTS
-- ============================================================

ALTER TABLE mst.kurs
    DROP CONSTRAINT IF EXISTS chk_kurs_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_kurs_sumber_kurs;

ALTER TABLE mst.kurs
    ADD CONSTRAINT chk_kurs_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        )),
    ADD CONSTRAINT chk_kurs_sumber_kurs
        CHECK (sumber_kurs IN ('BI_JISDOR','BI_KURS_TENGAH','INTERNAL','MANUAL'));

-- ============================================================
-- 3. BACKFILL — rows with approver_id are already approved
-- ============================================================

UPDATE mst.kurs
    SET workflow_status = 'APPROVED',
        created_by      = maker_id,
        updated_by      = approver_id,
        updated_at      = approved_at
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- Rows without approver still get created_by from maker_id
UPDATE mst.kurs
    SET created_by = maker_id
WHERE maker_id IS NOT NULL
  AND created_by IS NULL;

-- ============================================================
-- 4. INDEXES
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_kurs_tenant_created
    ON mst.kurs(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_kurs_workflow_status
    ON mst.kurs(workflow_status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_kurs_kode_tanggal_active
    ON mst.kurs(kode_mata_uang, tanggal_berlaku DESC) WHERE deleted_at IS NULL;

-- ============================================================
-- 5. sys.config — SEED WORKFLOW_CONFIG_KURS (if not already seeded in 0008)
-- ============================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'WORKFLOW_CONFIG_KURS',
    '{
        "entityType": "KURS",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "kurs.submit",
            "review":  "kurs.review",
            "approve": "kurs.approve",
            "reject":  "kurs.reject"
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
    'Workflow config for mst.kurs (manual override) — 4-eyes AKUN→AKUN-CTL(review)→AKUN-CTL(approve). BI JISDOR feed auto-approves via integration worker.',
    'WORKFLOW'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
