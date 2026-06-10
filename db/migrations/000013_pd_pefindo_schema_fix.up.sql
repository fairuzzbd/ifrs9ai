-- migration: 0013 pd_pefindo_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008
-- description: Backfill mst.pd_pefindo with audit cols + workflow_status.
--              Fix pd_12month + pd_lifetime_{3,5,7,10}y precision from
--              NUMERIC(8,4) to NUMERIC(10,8) per DEC-016 (5 columns).
--              CHECK pd_lifetime ranges 0..1. Rating whitelist deferred.

BEGIN;

-- ============================================================
-- 1. PRECISION FIX — DEC-016 (NUMERIC(10,8) for PD/LGD/EIR)
--    Existing rows: NUMERIC(8,4) fits inside NUMERIC(10,8) with no data loss.
--    Estimated downtime: ~0 ms for table sizes typical at Phase-3 (< 10k rows);
--    PostgreSQL re-writes column in-place for numeric precision widening.
-- ============================================================

ALTER TABLE mst.pd_pefindo
    ALTER COLUMN pd_12month    TYPE NUMERIC(10,8),
    ALTER COLUMN pd_lifetime_3y  TYPE NUMERIC(10,8),
    ALTER COLUMN pd_lifetime_5y  TYPE NUMERIC(10,8),
    ALTER COLUMN pd_lifetime_7y  TYPE NUMERIC(10,8),
    ALTER COLUMN pd_lifetime_10y TYPE NUMERIC(10,8);

-- ============================================================
-- 2. CHECK CONSTRAINTS — pd_12month already constrained (ck_pd_range);
--    add companion constraint for pd_lifetime_* columns.
--
--    NOTE — PD monotonicity rule (pd_12month ≤ pd_lifetime_3y ≤ … ≤ pd_lifetime_10y):
--    This is a BUSINESS rule validated at the service layer
--    (internal/ecl/pd_pefindo_service.go — ValidateMonotonicity).
--    Not enforced as a DB CHECK because:
--      (a) NULL columns make the comparison expression non-trivial to read;
--      (b) some Pefindo studies legitimately omit mid-tenor buckets.
--    TODO(ecl-eir-engineer): enforce in service; reject if violated.
-- ============================================================

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_lifetime_ranges;

ALTER TABLE mst.pd_pefindo
    ADD CONSTRAINT chk_pd_lifetime_ranges CHECK (
        (pd_lifetime_3y  IS NULL OR pd_lifetime_3y  BETWEEN 0 AND 1) AND
        (pd_lifetime_5y  IS NULL OR pd_lifetime_5y  BETWEEN 0 AND 1) AND
        (pd_lifetime_7y  IS NULL OR pd_lifetime_7y  BETWEEN 0 AND 1) AND
        (pd_lifetime_10y IS NULL OR pd_lifetime_10y BETWEEN 0 AND 1)
    );

-- ============================================================
-- 3. ADD AUDIT COLUMNS (same pattern as 0008 mata_uang_schema_fix)
--    created_at and uploaded_at already exist in 0001 schema;
--    the six missing audit cols are added here.
-- ============================================================

ALTER TABLE mst.pd_pefindo
    ADD COLUMN IF NOT EXISTS created_by  UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID        REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT      NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT        NOT NULL DEFAULT 'TUGURE';

-- ============================================================
-- 4. WORKFLOW_STATUS COLUMN + 7-state CHECK
--    uploaded_by / approved_by legacy columns are KEPT (read-only history).
-- ============================================================

ALTER TABLE mst.pd_pefindo
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.pd_pefindo
    DROP CONSTRAINT IF EXISTS chk_pd_pefindo_workflow_status;

ALTER TABLE mst.pd_pefindo
    ADD CONSTRAINT chk_pd_pefindo_workflow_status CHECK (workflow_status IN (
        'DRAFT',
        'PENDING_REVIEW',
        'PENDING_APPROVAL',
        'PENDING_APPROVAL_2',
        'APPROVED',
        'REJECTED',
        'RETURNED'
    ));

-- ============================================================
-- 5. BACKFILL workflow_status
--    Rows that were approved (approved_by IS NOT NULL) → APPROVED.
--    Remaining rows → DRAFT (already the column default).
-- ============================================================

UPDATE mst.pd_pefindo
    SET workflow_status = 'APPROVED'
WHERE approved_by IS NOT NULL
  AND workflow_status = 'DRAFT';

-- ============================================================
-- 6. INDEXES
--    ix_pd_pefindo_rating_periode / ix_pd_pefindo_current already exist
--    from 0001; only new indexes are created here.
-- ============================================================

-- Tenant + time composite for hot tenant queries
CREATE INDEX IF NOT EXISTS idx_pd_pefindo_tenant_created
    ON mst.pd_pefindo (tenant_id, created_at DESC);

-- Workflow queue (pending items only, no soft-deleted rows)
CREATE INDEX IF NOT EXISTS idx_pd_pefindo_workflow_status
    ON mst.pd_pefindo (workflow_status)
    WHERE deleted_at IS NULL;

-- Active PD lookup by rating (current rows = periode_berlaku_sampai IS NULL)
CREATE INDEX IF NOT EXISTS idx_pd_pefindo_active_rating
    ON mst.pd_pefindo (rating, periode_berlaku_dari DESC)
    WHERE deleted_at IS NULL
      AND periode_berlaku_sampai IS NULL;

-- ============================================================
-- 7. TODO — Rating whitelist (idAAA..idD) DB CHECK
--    Deferred to migration 0014 (pending Pefindo rating enum finalisation
--    from Pefindo_Annual_Default_Study_2007-2025 appendix).
--    Service layer (internal/ecl/pd_pefindo_service.go) enforces the
--    whitelist via ValidatePefindoRating() until 0014 is applied.
-- ============================================================

COMMIT;
