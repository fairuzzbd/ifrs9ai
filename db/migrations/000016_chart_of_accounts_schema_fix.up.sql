-- migration: 0016 chart_of_accounts_schema_fix
-- author: data-modeler
-- requires: 0001, 0008
-- description: Add missing audit columns (deleted_at, deleted_by, tenant_id) and
--              workflow_status to mst.chart_of_accounts. WORKFLOW_CONFIG_CHART_OF_ACCOUNTS
--              was already seeded in 0008 (4-eyes, no step-up).
--              Schema drift: 'version' INT (0001) → 'row_version' BIGINT cutover
--              is DEFERRED to Phase 5 to avoid a full-table rewrite while data exists.
--              See TODO below.
-- downtime: None expected. ADD COLUMN with DEFAULT is metadata-only in PG 11+.
--           UPDATE for workflow_status backfill: table-scan, recommend off-peak.

BEGIN;

-- ============================================================
-- 1. ADD MISSING AUDIT & WORKFLOW COLUMNS
-- ============================================================

ALTER TABLE mst.chart_of_accounts
    ADD COLUMN IF NOT EXISTS deleted_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by      UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS tenant_id       TEXT NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- ============================================================
-- 2. WORKFLOW_STATUS CHECK CONSTRAINT
-- ============================================================

ALTER TABLE mst.chart_of_accounts
    DROP CONSTRAINT IF EXISTS chk_coa_workflow_status;

ALTER TABLE mst.chart_of_accounts
    ADD CONSTRAINT chk_coa_workflow_status
        CHECK (workflow_status IN (
            'DRAFT', 'PENDING_REVIEW', 'PENDING_APPROVAL', 'PENDING_APPROVAL_2',
            'APPROVED', 'REJECTED', 'RETURNED'
        ));

-- ============================================================
-- 3. BACKFILL — existing rows pre-date workflow; treat as APPROVED
-- ============================================================

UPDATE mst.chart_of_accounts
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ============================================================
-- 4. INDEXES
-- ============================================================

-- Tenant + time composite — required for every hot table (db-conventions §indexes)
CREATE INDEX IF NOT EXISTS idx_coa_tenant_created
    ON mst.chart_of_accounts(tenant_id, created_at DESC);

-- Workflow queue — partial on active rows only
CREATE INDEX IF NOT EXISTS idx_coa_workflow_status
    ON mst.chart_of_accounts(workflow_status)
    WHERE deleted_at IS NULL;

-- ============================================================
-- TODO(Phase-5): rename 'version' INT → 'row_version' BIGINT
--   Steps required (separate migration, after data-freeze window):
--     1. ALTER TABLE mst.chart_of_accounts ADD COLUMN row_version BIGINT NOT NULL DEFAULT 1;
--     2. UPDATE mst.chart_of_accounts SET row_version = version;
--     3. Update application code to use row_version (optimistic-lock middleware).
--     4. ALTER TABLE mst.chart_of_accounts DROP COLUMN version;
--   Estimated downtime: lock during DROP COLUMN (~seconds for ≤ 100k rows).
--   Ref: db-conventions.md "row_version (optimistic lock)"
-- ============================================================

COMMIT;
