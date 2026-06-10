-- migration: 0015 impact_fl_multiplier_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008, 0014
-- description: Retrofit mst.impact_mev_pd + mst.impact_pd with:
--   (a) missing audit cols (created_by, updated_at/by, deleted_at/by, row_version, tenant_id)
--   (b) workflow_status CHECK + workflow_instance_id FK
--   (c) precision fix: impact_multiplier NUMERIC(8,4) → NUMERIC(10,8) per DEC-016
--   (d) new CHECK: impact_mev_pd.impact_multiplier > 0  (OQ-3 resolved 2026-06-09)
--       impact_pd.impact_multiplier BETWEEN 0.5 AND 2.0 already exists in 0001 — kept.
--   (e) indexes: tenant+created, workflow status hot-path, active lookup per periode
--
--   OQ resolutions applied (2026-06-09):
--     OQ-1: skenario enum GOOD+BAD only — CHECK in 0001 unchanged, not touched here.
--     OQ-2: impact_mev_pd and impact_pd are INDEPENDENT manual inputs — no compute logic.
--     OQ-3: impact_mev_pd.impact_multiplier > 0 only; impact_pd keeps BETWEEN 0.5 AND 2.0.
--     OQ-4: revise via soft-delete + create new; no version column.
--     OQ-5: two separate /active endpoints — no combined endpoint in Phase 3.
--
--   WORKFLOW_CONFIG_IMPACT_MEV_PD + WORKFLOW_CONFIG_IMPACT_PD already seeded in 0008.
--   No new seed required.

BEGIN;

-- ====================================================================
-- PART A — mst.impact_mev_pd
-- ====================================================================

-- A1. Precision fix — NUMERIC(8,4) → NUMERIC(10,8) per DEC-016
--     Lossless widening; no USING clause required.
ALTER TABLE mst.impact_mev_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8);

-- A2. Add missing audit columns
ALTER TABLE mst.impact_mev_pd
    ADD COLUMN IF NOT EXISTS created_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version         BIGINT      NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status     VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID       REFERENCES sys.workflow_instance(id);

-- A3. CHECK constraints — drop first for idempotent re-run
ALTER TABLE mst.impact_mev_pd
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_multiplier_positive;

ALTER TABLE mst.impact_mev_pd
    ADD CONSTRAINT chk_impact_mev_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        )),
    ADD CONSTRAINT chk_impact_mev_pd_multiplier_positive
        CHECK (impact_multiplier > 0);

-- A4. Backfill — rows that already have approver_id set were previously approved
UPDATE mst.impact_mev_pd
    SET workflow_status = 'APPROVED'
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- A5. Backfill created_by from maker_id for rows missing created_by
UPDATE mst.impact_mev_pd
    SET created_by = maker_id
WHERE created_by IS NULL;

-- A6. Indexes
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_tenant_created
    ON mst.impact_mev_pd(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_workflow_status
    ON mst.impact_mev_pd(workflow_status) WHERE deleted_at IS NULL;

-- Active lookup per (periode_id, skenario) — used by GetActive + ECL engine Phase 4
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_active
    ON mst.impact_mev_pd(periode_id, skenario)
    WHERE deleted_at IS NULL AND workflow_status = 'APPROVED';

-- ====================================================================
-- PART B — mst.impact_pd
-- ====================================================================

-- B1. Precision fix — NUMERIC(8,4) → NUMERIC(10,8) per DEC-016
--     Lossless widening. impact_pd keeps ck_impact_pd_range (BETWEEN 0.5 AND 2.0)
--     from 0001 unchanged; precision widening does not affect range semantics.
ALTER TABLE mst.impact_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8);

-- B2. Add missing audit columns
ALTER TABLE mst.impact_pd
    ADD COLUMN IF NOT EXISTS created_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by          UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version         BIGINT      NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status     VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID       REFERENCES sys.workflow_instance(id);

-- B3. CHECK constraint — drop first for idempotent re-run
ALTER TABLE mst.impact_pd
    DROP CONSTRAINT IF EXISTS chk_impact_pd_workflow_status;

ALTER TABLE mst.impact_pd
    ADD CONSTRAINT chk_impact_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- B4. Backfill — rows that already have approver_id set were previously approved
UPDATE mst.impact_pd
    SET workflow_status = 'APPROVED'
WHERE approver_id IS NOT NULL
  AND workflow_status = 'DRAFT';

-- B5. Backfill created_by from maker_id for rows missing created_by
UPDATE mst.impact_pd
    SET created_by = maker_id
WHERE created_by IS NULL;

-- B6. Indexes
CREATE INDEX IF NOT EXISTS idx_impact_pd_tenant_created
    ON mst.impact_pd(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_impact_pd_workflow_status
    ON mst.impact_pd(workflow_status) WHERE deleted_at IS NULL;

-- Active lookup per periode_id — used by GetActive + ECL engine Phase 4
CREATE INDEX IF NOT EXISTS idx_impact_pd_active
    ON mst.impact_pd(periode_id)
    WHERE deleted_at IS NULL AND workflow_status = 'APPROVED';

COMMIT;
