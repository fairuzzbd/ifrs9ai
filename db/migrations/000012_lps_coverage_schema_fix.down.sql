-- migration: 0012 lps_coverage_schema_fix (DOWN)
-- author: data-modeler
-- requires: 0001, 0005 (audit hardening), 0007, 0008
-- description: Revert all changes made by 0012 up.sql.
--              Drops indexes, CHECK constraints, audit cols, workflow_status.
--              Reverts coverage_amount precision NUMERIC(20,4) → NUMERIC(20,2).
--              NOTE: WORKFLOW_CONFIG_LPS_COVERAGE was seeded in 0008 —
--              it is NOT removed here (owned by 0008 down.sql).

BEGIN;

-- ============================================================
-- 1. Drop indexes added by this migration
--    ix_lps_current (added in 0001) is left intact.
-- ============================================================

DROP INDEX IF EXISTS mst.idx_lps_coverage_active;
DROP INDEX IF EXISTS mst.idx_lps_coverage_workflow_status;
DROP INDEX IF EXISTS mst.idx_lps_coverage_tenant_created;

-- ============================================================
-- 2. Drop CHECK constraints
-- ============================================================

ALTER TABLE mst.lps_coverage
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_workflow_status,
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_currency,
    DROP CONSTRAINT IF EXISTS chk_lps_coverage_amount_positive;

-- ============================================================
-- 3. Drop columns added by this migration
-- ============================================================

ALTER TABLE mst.lps_coverage
    DROP COLUMN IF EXISTS workflow_status,
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- ============================================================
-- 4. Revert coverage_amount precision NUMERIC(20,4) → NUMERIC(20,2)
--    Narrowing cast: values with more than 2 decimal places will be
--    rounded.  Pre-condition guard aborts down migration if any row
--    has > 2-decimal precision to prevent silent data truncation.
--    In practice, LPS amounts are always whole IDR so the guard
--    should never fire.  Explicit USING clause required by PG.
-- ============================================================

DO $$ BEGIN
  IF EXISTS (
    SELECT 1 FROM mst.lps_coverage
    WHERE coverage_amount != TRUNC(coverage_amount, 2)
  ) THEN
    RAISE EXCEPTION 'down 0012 blocked: rows with > 2-decimal precision exist in mst.lps_coverage.coverage_amount';
  END IF;
END $$;

ALTER TABLE mst.lps_coverage
    ALTER COLUMN coverage_amount TYPE NUMERIC(20,2)
        USING coverage_amount::NUMERIC(20,2);

COMMIT;
