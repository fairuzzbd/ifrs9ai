-- migration: 0011 bobot_skenario_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008
-- description: Backfill mst.bobot_skenario with missing audit cols +
--              workflow_status. Fix bobot precision NUMERIC(8,4) →
--              NUMERIC(10,8) per DEC-016. Cross-row sum=1.0 invariant
--              enforced at service layer (DEC-010 default 0.25/0.50/0.25).

BEGIN;

-- ============================================================
-- 0. FIX PRECISION — bobot from NUMERIC(8,4) to NUMERIC(10,8)
--    Per DEC-016 + db-conventions.md: PD/LGD/EIR and scenario-weight
--    storage MUST be NUMERIC(10,8). The 0001 schema used NUMERIC(8,4)
--    which silently rounds bobot to 4dp at write — Go layer computes at
--    8dp (shopspring/decimal) but DB truncates, causing calculable ECL
--    drift when ALCO overrides use non-round weights.
--    Same defect as lgd in 0010. Existing values (e.g. 0.2500) are
--    losslessly widened to 0.25000000.
-- ============================================================

ALTER TABLE mst.bobot_skenario
    ALTER COLUMN bobot TYPE NUMERIC(10,8);

-- ============================================================
-- 1. mst.bobot_skenario — ADD MISSING AUDIT COLUMNS
--    0001 only has: created_at, approved_at.
--    updated_at is NOT present in 0001 — added here as nullable TIMESTAMPTZ.
--    Legacy maker_id / approver_id / approved_at columns are LEFT IN PLACE
--    (FK integrity, service layer ignores them in Phase 3 — tracked as
--    technical debt for a future deprecation cycle).
-- ============================================================

ALTER TABLE mst.bobot_skenario
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

ALTER TABLE mst.bobot_skenario
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.bobot_skenario
    DROP CONSTRAINT IF EXISTS chk_bobot_skenario_workflow_status;

ALTER TABLE mst.bobot_skenario
    ADD CONSTRAINT chk_bobot_skenario_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- ============================================================
-- 3. Backfill existing rows to APPROVED
--    Rows that existed pre-workflow were in active use and are
--    considered approved. approver_id IS NOT NULL is used as the
--    discriminator — rows that already had an approver get APPROVED
--    first. Then any remaining DRAFT rows (no approver yet) are
--    also swept to APPROVED (all pre-workflow rows are active data).
-- ============================================================

UPDATE mst.bobot_skenario
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT'
  AND approver_id IS NOT NULL;

-- Paranoid sweep: all remaining pre-workflow DRAFT rows → APPROVED.
UPDATE mst.bobot_skenario
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ============================================================
-- 4. Indexes
--    ix_bobot_skenario_periode / ix_bobot_current (from 0001) remain
--    in place. New indexes: tenant queries, workflow hot-path, active
--    weight lookup per skenario.
-- ============================================================

-- Tenant + chronological audit queries
CREATE INDEX IF NOT EXISTS idx_bobot_skenario_tenant_created
    ON mst.bobot_skenario(tenant_id, created_at DESC);

-- Workflow queue (PENDING_* states filtered efficiently)
CREATE INDEX IF NOT EXISTS idx_bobot_skenario_workflow_status
    ON mst.bobot_skenario(workflow_status) WHERE deleted_at IS NULL;

-- Active weight lookup: most-recent non-expired entry per skenario.
-- Supersedes ix_bobot_current from 0001 (kept for compat); this one is
-- tenant-safe and respects soft-delete.
CREATE INDEX IF NOT EXISTS idx_bobot_skenario_active
    ON mst.bobot_skenario(skenario, periode_berlaku_dari DESC)
    WHERE deleted_at IS NULL AND periode_berlaku_sampai IS NULL;

-- ============================================================
-- TODO (Phase 5): Cross-row sum=1.0 invariant for the triple
--   (GOOD, NORMAL, BAD) weights within the same effective period.
--   Per DEC-010, default is 0.25/0.50/0.25; ALCO can override but
--   sum MUST equal 1.0. Enforcement strategy: service layer validates
--   atomically within a single transaction before committing any
--   bobot_skenario INSERT/UPDATE. DB-level enforcement via a
--   DEFERRABLE constraint or trigger is deferred to Phase 5 once the
--   period-overlap semantics are finalized by ALCO. Trigger candidate:
--   trg_bobot_skenario_sum_check AFTER INSERT OR UPDATE FOR EACH ROW.
-- ============================================================

COMMIT;
