-- migration: 0038 periode_close_p5m4 (DOWN)
-- author: data-modeler
-- description:
--   Reversal of migration 000038 periode_close_p5m4.
--   Drops in reverse dependency order:
--     1. sys.config rows seeded in E
--     2. sys.closing_checklist_snapshot partitions, indexes, triggers, functions (B + D)
--     3. mst.periode_buku new indexes (C) and new columns (A)
--     4. mst.periode_buku CHECK constraint — restore original (OPEN|SOFT_CLOSED|CLOSED)
--
--   WARNING: This down migration drops tables with data and removes columns.
--   Run only in dev/test environments or after confirming no production data exists.
--   Append-only triggers are dropped before table drop to allow clean cascade.

BEGIN;

-- ====================================================================
-- E. Remove sys.config seeds
-- ====================================================================

DELETE FROM sys.config
WHERE config_key IN (
    'SOFT_CLOSE_CHECKLIST_STALE_HOURS',
    'HARD_CLOSE_GRACE_WINDOW_HOURS',
    'PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST'
);

-- ====================================================================
-- B + D. DROP sys.closing_checklist_snapshot
--        (triggers first, then partition tables, then parent)
-- ====================================================================

-- Drop triggers on parent (propagates to partitions automatically in PG 18)
DROP TRIGGER IF EXISTS trg_checklist_snapshot_no_update ON sys.closing_checklist_snapshot;
DROP TRIGGER IF EXISTS trg_checklist_snapshot_no_delete ON sys.closing_checklist_snapshot;

-- Drop trigger functions
DROP FUNCTION IF EXISTS fn_checklist_snapshot_no_update();
DROP FUNCTION IF EXISTS fn_checklist_snapshot_no_delete();

-- Drop partition children (must drop before parent)
DROP TABLE IF EXISTS sys.closing_checklist_snapshot_y2026m06;
DROP TABLE IF EXISTS sys.closing_checklist_snapshot_y2026m07;
DROP TABLE IF EXISTS sys.closing_checklist_snapshot_y2026m08;
DROP TABLE IF EXISTS sys.closing_checklist_snapshot_default;

-- Drop parent (indexes on parent drop automatically with the table)
DROP TABLE IF EXISTS sys.closing_checklist_snapshot CASCADE;

-- ====================================================================
-- C. DROP indexes added to mst.periode_buku in section C
-- ====================================================================

DROP INDEX IF EXISTS mst.idx_periode_buku_status;
DROP INDEX IF EXISTS mst.idx_periode_buku_grace_expires;
DROP INDEX IF EXISTS mst.idx_periode_buku_status_tahun_bulan;

-- ====================================================================
-- A9. DROP FK indexes on new reference columns
-- ====================================================================

DROP INDEX IF EXISTS mst.idx_periode_buku_soft_close_requested_by;
DROP INDEX IF EXISTS mst.idx_periode_buku_soft_close_approved_by;
DROP INDEX IF EXISTS mst.idx_periode_buku_hard_close_requested_by;
DROP INDEX IF EXISTS mst.idx_periode_buku_hard_close_approved_by;

-- ====================================================================
-- A8–A3. DROP new columns added to mst.periode_buku
-- ====================================================================

ALTER TABLE mst.periode_buku
    DROP COLUMN IF EXISTS reopen_reason,
    DROP COLUMN IF EXISTS step_up_token_ref,
    DROP COLUMN IF EXISTS hard_close_grace_expires_at,
    DROP COLUMN IF EXISTS hard_close_approve_reason,
    DROP COLUMN IF EXISTS hard_close_approved_at,
    DROP COLUMN IF EXISTS hard_close_approved_by,
    DROP COLUMN IF EXISTS hard_close_request_reason,
    DROP COLUMN IF EXISTS hard_close_requested_at,
    DROP COLUMN IF EXISTS hard_close_requested_by,
    DROP COLUMN IF EXISTS soft_close_approve_reason,
    DROP COLUMN IF EXISTS soft_close_approved_at,
    DROP COLUMN IF EXISTS soft_close_approved_by,
    DROP COLUMN IF EXISTS soft_close_request_reason,
    DROP COLUMN IF EXISTS soft_close_requested_at,
    DROP COLUMN IF EXISTS soft_close_requested_by;

-- ====================================================================
-- A1. Restore original CHECK constraint on status_periode
--     Removes HARD_CLOSE_PENDING from allowed values.
--
--     WARNING: If any rows have status_periode = 'HARD_CLOSE_PENDING'
--     this ALTER will fail. Reset those rows first:
--       UPDATE mst.periode_buku SET status_periode = 'SOFT_CLOSED'
--       WHERE status_periode = 'HARD_CLOSE_PENDING';
-- ====================================================================

ALTER TABLE mst.periode_buku
    DROP CONSTRAINT IF EXISTS ck_periode_status;

ALTER TABLE mst.periode_buku
    ADD CONSTRAINT ck_periode_status
        CHECK (status_periode IN ('OPEN', 'SOFT_CLOSED', 'CLOSED'));

COMMIT;
