-- migration: 0025 lookthrough_underlying_audit_not_null
-- author: data-modeler / ecl-eir-engineer
-- requires: 0001 (init_schema — ecl.lookthrough_underlying created),
--           0005 (fn_ecl_no_hard_delete — no-hard-delete trigger on ecl.*),
--           0024 (fund_composition_lookthrough — added audit cols as NULLABLE)
-- description: Backfill NULL created_by/updated_by on ecl.lookthrough_underlying
--   using sentinel system UUID '00000000-0000-0000-0000-000000000000' (pre-migration rows
--   have no actor — system sentinel is acceptable per db-conventions.md "created_by NOT NULL").
--   Then enforce NOT NULL + DEFAULT on all standard audit columns per db-conventions.md.
--   Columns affected: created_by, updated_by (backfilled + NOT NULL),
--                     created_at, updated_at, row_version, tenant_id (NOT NULL + DEFAULT).
--   deleted_at / deleted_by remain nullable (optional soft-delete semantics).

BEGIN;

-- =========================================================
-- Step 1: Backfill NULL created_by / updated_by
-- Sentinel UUID '00000000-0000-0000-0000-000000000000' indicates a
-- row that pre-dates the 6-eyes actor model (pre-migration 000024).
-- =========================================================
UPDATE ecl.lookthrough_underlying
SET created_by = '00000000-0000-0000-0000-000000000000'::uuid
WHERE created_by IS NULL;

UPDATE ecl.lookthrough_underlying
SET updated_by = '00000000-0000-0000-0000-000000000000'::uuid
WHERE updated_by IS NULL;

-- =========================================================
-- Step 2: Enforce NOT NULL + DEFAULT on created_at
-- DEFAULT now() already present from migration 000024; enforce NOT NULL.
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN created_at SET DEFAULT now();

-- =========================================================
-- Step 3: Enforce NOT NULL on created_by
-- No FK to sec.user needed for sentinel; FK already defined in 000024 as nullable.
-- We only add NOT NULL; the FK constraint on the column remains.
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN created_by SET NOT NULL;

-- =========================================================
-- Step 4: Enforce NOT NULL + DEFAULT on updated_at
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now();

-- =========================================================
-- Step 5: Enforce NOT NULL on updated_by
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN updated_by SET NOT NULL;

-- =========================================================
-- Step 6: Enforce NOT NULL + DEFAULT on row_version
-- DEFAULT 1 already set in 000024; enforce NOT NULL.
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN row_version SET NOT NULL,
    ALTER COLUMN row_version SET DEFAULT 1;

-- =========================================================
-- Step 7: Enforce NOT NULL + DEFAULT on tenant_id
-- DEFAULT 'TUGURE' already set in 000024; enforce NOT NULL.
-- =========================================================
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN tenant_id SET NOT NULL,
    ALTER COLUMN tenant_id SET DEFAULT 'TUGURE';

COMMIT;
