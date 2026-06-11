-- migration: 000023 lps_exclusion_override — DOWN
-- author: data-modeler
-- description: Reverse of 000023_lps_exclusion_override.up.sql
-- Order: triggers (no-delete first to allow DROP TABLE), then table, then function.
-- WARNING: Rollback will FAIL if rows exist in ecl.lps_exclusion_override
--          due to FK RESTRICT semantics. Run only in zero-data dev/UAT environment.
-- NOTE: fn_lps_override_check_periode_order is dropped last — it is used only by
--       trg_lps_override_periode_order which is dropped implicitly with the table.

BEGIN;

-- ============================================================
-- Drop no-hard-delete trigger FIRST (prevents DROP TABLE from
-- being blocked by the trigger itself attempting to fire on
-- the implicit DELETE that DROP TABLE issues internally).
-- In PostgreSQL, DROP TABLE bypasses row-level triggers, so
-- this ordering is for explicitness and mirrors the up.sql
-- creation order in reverse.
-- ============================================================

DROP TRIGGER IF EXISTS tg_ecl_lps_override_no_delete   ON ecl.lps_exclusion_override;

-- ============================================================
-- Drop row maintenance triggers
-- ============================================================

DROP TRIGGER IF EXISTS trg_lps_override_row_version     ON ecl.lps_exclusion_override;
DROP TRIGGER IF EXISTS trg_lps_override_updated_at      ON ecl.lps_exclusion_override;

-- ============================================================
-- Drop periode ordering trigger
-- (trg_lps_override_periode_order is dropped implicitly with
--  the table below, but explicit for clarity)
-- ============================================================

DROP TRIGGER IF EXISTS trg_lps_override_periode_order   ON ecl.lps_exclusion_override;

-- ============================================================
-- Drop indexes
-- (All non-PK indexes are dropped implicitly with the table,
--  but listed explicitly to match the pattern from 000022)
-- ============================================================

DROP INDEX IF EXISTS ecl.idx_lps_override_approved_active_instrumen;
DROP INDEX IF EXISTS ecl.idx_lps_override_tenant_created;
DROP INDEX IF EXISTS ecl.idx_lps_override_valid_to_periode;
DROP INDEX IF EXISTS ecl.idx_lps_override_valid_from_periode;
DROP INDEX IF EXISTS ecl.idx_lps_override_approver_id;
DROP INDEX IF EXISTS ecl.idx_lps_override_maker_id;
DROP INDEX IF EXISTS ecl.idx_lps_override_status_created;
DROP INDEX IF EXISTS ecl.idx_lps_override_instrumen_status;

-- ============================================================
-- Drop table
-- ============================================================

DROP TABLE IF EXISTS ecl.lps_exclusion_override;

-- ============================================================
-- Drop periode ordering function
-- (Drop after the trigger/table that uses it)
-- ============================================================

DROP FUNCTION IF EXISTS fn_lps_override_check_periode_order();

COMMIT;
