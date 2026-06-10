-- migration: 0012 lps_coverage_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008
-- description: Backfill mst.lps_coverage with audit cols + workflow_status.
--              Fix coverage_amount precision NUMERIC(20,2) → NUMERIC(20,4)
--              per db-conventions IDR standard. Add CHECK constraints:
--              coverage_amount > 0, mata_uang = 'IDR' (DEC-014 — LPS is
--              Indonesia-only context). WORKFLOW_CONFIG_LPS_COVERAGE
--              already seeded in 0008.

BEGIN;

-- ============================================================
-- 1. mst.lps_coverage — PRECISION FIX
--    db-conventions: IDR amount = NUMERIC(20,4)
--    0001 has NUMERIC(20,2) — closes the 2-decimal gap.
--    No data loss: NUMERIC(20,4) is a lossless widening of NUMERIC(20,2).
-- ============================================================

ALTER TABLE mst.lps_coverage
    ALTER COLUMN coverage_amount TYPE NUMERIC(20,4);

-- ============================================================
-- 2. ADD MISSING AUDIT COLUMNS (same pattern as 0008)
-- ============================================================

ALTER TABLE mst.lps_coverage
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- ============================================================
-- 3. ADD workflow_status
-- ============================================================

ALTER TABLE mst.lps_coverage
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- ============================================================
-- 4. ADD CHECK CONSTRAINTS
-- ============================================================

-- Drop first (idempotent re-run guard)
ALTER TABLE mst.lps_coverage
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_amount_positive,
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_currency,
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_workflow_status;

ALTER TABLE mst.lps_coverage
    ADD CONSTRAINT chk_lps_coverage_amount_positive
        CHECK (coverage_amount > 0),
    ADD CONSTRAINT chk_lps_coverage_currency
        CHECK (mata_uang = 'IDR'),
    ADD CONSTRAINT chk_lps_coverage_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- ============================================================
-- 5. BACKFILL — existing rows
--    Rows that already have a non-null approver_id were manually
--    approved before the workflow engine existed → APPROVED.
--    Remaining rows (no approver) were drafts → DRAFT (default).
-- ============================================================

-- 5a. Rows with approver_id set → treat as APPROVED
UPDATE mst.lps_coverage
    SET workflow_status = 'APPROVED'
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- 5b. Backfill created_by from maker_id for rows that have no created_by yet
--     (maker_id is the originating identity before audit cols existed)
UPDATE mst.lps_coverage
    SET created_by = maker_id
WHERE created_by IS NULL;

-- ============================================================
-- 6. INDEXES
-- ============================================================

-- 6a. Tenant + time — hot path for tenant queries
CREATE INDEX IF NOT EXISTS idx_lps_coverage_tenant_created
    ON mst.lps_coverage(tenant_id, created_at DESC);

-- 6b. Workflow status — queue queries, partial on active rows only
CREATE INDEX IF NOT EXISTS idx_lps_coverage_workflow_status
    ON mst.lps_coverage(workflow_status) WHERE deleted_at IS NULL;

-- 6c. Active period lookup — replaces the ix_lps_current created in 0001
--     (ix_lps_current is left intact; this index adds deleted_at guard
--     and DESC ordering for "latest active" queries)
CREATE INDEX IF NOT EXISTS idx_lps_coverage_active
    ON mst.lps_coverage(periode_berlaku_dari DESC)
    WHERE deleted_at IS NULL AND periode_berlaku_sampai IS NULL;

COMMIT;
