-- migration: 0010 lgd_basel_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008
-- description: Backfill mst.lgd_basel with missing audit cols + workflow_status.
--              id UUID PK already in place (0001). Adds updated_at (not present
--              in 0001 — only created_at + approved_at). WORKFLOW_CONFIG_LGD_BASEL
--              already seeded in 0008. tipe_eksposur enum CHECK deferred to 0011
--              pending ALCO whitelist sign-off.

BEGIN;

-- ============================================================
-- 0. FIX PRECISION — lgd from NUMERIC(8,4) to NUMERIC(10,8)
--    Per DEC-016 + db-conventions.md: PD/LGD/EIR storage MUST be
--    NUMERIC(10,8). The 0001 schema used NUMERIC(8,4) which silently
--    rounds LGD to 4dp at write — Go layer computes at 8dp but DB
--    truncates, causing calculable ECL drift on large portfolios.
--    NOTE: ecl.calc_header.lgd + ecl.lookthrough_underlying.lgd have
--    the same defect but are ECL engine scope (Phase 5) — flagged
--    for ecl-eir-engineer follow-up (ref: compliance audit
--    docs/audit/COMPLIANCE-lgd-basel-*.md BLOCKER 1).
-- ============================================================

ALTER TABLE mst.lgd_basel
    ALTER COLUMN lgd TYPE NUMERIC(10,8);

-- ============================================================
-- 1. mst.lgd_basel — ADD MISSING AUDIT COLUMNS
--    0001 only has: created_at, approved_at.
--    updated_at is NOT present in 0001 — added here as nullable TIMESTAMPTZ.
--    legacy maker_id / approver_id / approved_at columns are LEFT IN PLACE
--    (FK integrity, service layer ignores them in Phase 3 — tracked as
--    technical debt for 0011 deprecation cycle).
-- ============================================================

ALTER TABLE mst.lgd_basel
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- ============================================================
-- 2. workflow_status column + CHECK constraint
-- ============================================================

ALTER TABLE mst.lgd_basel
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.lgd_basel
    DROP CONSTRAINT IF EXISTS chk_lgd_basel_workflow_status;

ALTER TABLE mst.lgd_basel
    ADD CONSTRAINT chk_lgd_basel_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- ============================================================
-- 3. Backfill existing rows to APPROVED
--    Rows that existed pre-workflow were in active use and are
--    considered approved. approver_id IS NOT NULL is used as the
--    discriminator — rows that already had an approver get APPROVED,
--    rows without an approver remain DRAFT (edge case: in-flight data).
-- ============================================================

UPDATE mst.lgd_basel
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT'
  AND approver_id IS NOT NULL;

-- Rows with no approver_id but created before this migration are also
-- set APPROVED (paranoid backfill — all pre-workflow rows are active).
UPDATE mst.lgd_basel
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ============================================================
-- 4. Indexes
--    ix_lgd_tipe_periode / ix_lgd_current (from 0001) remain in place.
--    New indexes: tenant queries, workflow hot-path, active pool lookup.
-- ============================================================

-- Tenant + chronological audit queries
CREATE INDEX IF NOT EXISTS idx_lgd_basel_tenant_created
    ON mst.lgd_basel(tenant_id, created_at DESC);

-- Workflow queue (PENDING_* states filtered efficiently)
CREATE INDEX IF NOT EXISTS idx_lgd_basel_workflow_status
    ON mst.lgd_basel(workflow_status) WHERE deleted_at IS NULL;

-- Active pool lookup: most-recent non-expired entry per tipe_eksposur
-- Supersedes ix_lgd_current from 0001 (kept for compat); this one is
-- tenant-safe and respects soft-delete.
CREATE INDEX IF NOT EXISTS idx_lgd_basel_active
    ON mst.lgd_basel(tipe_eksposur, periode_berlaku_dari DESC)
    WHERE deleted_at IS NULL AND periode_berlaku_sampai IS NULL;

-- ============================================================
-- TODO (0011): Add CHECK constraint on tipe_eksposur whitelist once
--   ALCO finalizes Basel IRB pool definitions for Tugu Reasuransi.
--   Candidate values: 'SOVEREIGN','BANK','CORPORATE','RETAIL',
--   'EQUITY','REINSURANCE' (ref: SoW §4 LGD, Pefindo Annual Default
--   Study, PSAK 71 / IFRS 9 Basel III IRB categorisation).
--   Constraint name to use: chk_lgd_basel_tipe_eksposur.
-- ============================================================

COMMIT;
