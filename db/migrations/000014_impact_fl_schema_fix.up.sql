-- migration: 0014 impact_fl_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008
-- description: Backfill mst.impact_mev_pd + mst.impact_pd with audit
--              cols + workflow_status. Fix impact_multiplier precision
--              NUMERIC(8,4) → NUMERIC(10,8) per DEC-016. impact_pd
--              CHECK range 0.5..2.0 (multiplier, not probability)
--              left intact from 0001.

BEGIN;

-- ============================================================
-- SECTION 1: mst.impact_mev_pd
-- ============================================================

-- 1a. Fix impact_multiplier precision: NUMERIC(8,4) → NUMERIC(10,8)
--     Existing rows: cast is lossless (additional decimal places filled with 0s).
ALTER TABLE mst.impact_mev_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8)
        USING impact_multiplier::NUMERIC(10,8);

-- 1b. Audit columns (db-conventions.md — wajib di semua tabel)
ALTER TABLE mst.impact_mev_pd
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- Backfill created_by from legacy maker_id (maker_id is NOT NULL in 0001)
UPDATE mst.impact_mev_pd
    SET created_by = maker_id
WHERE created_by IS NULL;

-- 1c. workflow_status — 7-state enum (matches sys.workflow_instance.current_state)
ALTER TABLE mst.impact_mev_pd
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.impact_mev_pd
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_workflow_status;

ALTER TABLE mst.impact_mev_pd
    ADD CONSTRAINT chk_impact_mev_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- 1d. Backfill workflow_status: rows with approver_id set were manually approved
--     pre-workflow. Mark them APPROVED; everything else stays DRAFT.
UPDATE mst.impact_mev_pd
    SET workflow_status = 'APPROVED'
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- 1e. Indexes
--     (a) Tenant + time — mandatory for tenant queries (db-conventions.md)
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_tenant_created
    ON mst.impact_mev_pd(tenant_id, created_at DESC);

--     (b) workflow_status — hot filter for pending queues; partial on active rows
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_workflow_status
    ON mst.impact_mev_pd(workflow_status)
    WHERE deleted_at IS NULL;

--     (c) (periode_id, skenario) — ECL engine lookup; composite covers FK index too
--         The existing uq_impact_mev_periode_skenario unique constraint already provides
--         the uniqueness guard, but does not serve partial-index active-only lookups.
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_periode_skenario
    ON mst.impact_mev_pd(periode_id, skenario)
    WHERE deleted_at IS NULL;

-- ============================================================
-- SECTION 2: mst.impact_pd
-- ============================================================

-- 2a. Fix impact_multiplier precision: NUMERIC(8,4) → NUMERIC(10,8)
--     The CHECK constraint ck_impact_pd_range (BETWEEN 0.5 AND 2.0) remains intact —
--     the range is correct for a multiplier (not a probability). Cast is lossless.
ALTER TABLE mst.impact_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8)
        USING impact_multiplier::NUMERIC(10,8);

-- 2b. Audit columns
ALTER TABLE mst.impact_pd
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- Backfill created_by from legacy maker_id
UPDATE mst.impact_pd
    SET created_by = maker_id
WHERE created_by IS NULL;

-- 2c. workflow_status — 7-state enum
ALTER TABLE mst.impact_pd
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.impact_pd
    DROP CONSTRAINT IF EXISTS chk_impact_pd_workflow_status;

ALTER TABLE mst.impact_pd
    ADD CONSTRAINT chk_impact_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- 2d. Backfill workflow_status
UPDATE mst.impact_pd
    SET workflow_status = 'APPROVED'
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- 2e. Indexes
--     (a) Tenant + time
CREATE INDEX IF NOT EXISTS idx_impact_pd_tenant_created
    ON mst.impact_pd(tenant_id, created_at DESC);

--     (b) workflow_status — partial active rows
CREATE INDEX IF NOT EXISTS idx_impact_pd_workflow_status
    ON mst.impact_pd(workflow_status)
    WHERE deleted_at IS NULL;

COMMIT;
