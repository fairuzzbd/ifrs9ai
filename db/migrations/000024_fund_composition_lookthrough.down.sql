-- migration: 000024 fund_composition_lookthrough — DOWN
-- author: data-modeler
-- description: Reverse of 000024_fund_composition_lookthrough.up.sql
-- Order (reverse of up.sql creation order):
--   1. Drop triggers and indexes on ecl.lookthrough_underlying (ALTER revert)
--   2. Revert ecl.lookthrough_underlying column changes (DROP new cols, revert precision)
--   3. Drop mst.fund_composition_detail (triggers + indexes + table)
--   4. Drop mst.fund_composition (triggers + indexes + table)
--   5. Drop functions
--
-- DATA TRUNCATION WARNING:
--   Reverting NUMERIC(20,4) → NUMERIC(20,2) and NUMERIC(10,8) → NUMERIC(8,4) may
--   silently truncate decimal precision for existing rows. Recommended: run only in
--   zero-data dev/UAT environments. In any environment with real data, this down
--   migration requires a manual data review step before execution.
--
-- FK RESTRICTION WARNING:
--   DROP TABLE mst.fund_composition will FAIL if ecl.lookthrough_underlying has
--   non-NULL fund_composition_id references. Null-out the FK column first or run
--   only in zero-data environments.

BEGIN;

-- ====================================================================
-- 1. Revert ecl.lookthrough_underlying changes (newest additions first)
-- ====================================================================

-- 1a. Drop new triggers (updated_at + row_version added in this migration)
DROP TRIGGER IF EXISTS trg_lookthrough_row_version   ON ecl.lookthrough_underlying;
DROP TRIGGER IF EXISTS trg_lookthrough_updated_at    ON ecl.lookthrough_underlying;

-- 1b. Drop new indexes added in this migration
DROP INDEX IF EXISTS ecl.idx_lookthrough_tenant_created;
DROP INDEX IF EXISTS ecl.idx_lookthrough_asset_class;
DROP INDEX IF EXISTS ecl.idx_lookthrough_composition_id;

-- 1c. Drop audit columns
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS created_at;

-- 1d. Drop per-scenario FL ECL columns
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS ecl_fl_bad_idr,
    DROP COLUMN IF EXISTS ecl_fl_normal_idr,
    DROP COLUMN IF EXISTS ecl_fl_good_idr;

-- 1e. Drop per-scenario ECL-before-FL columns
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS ecl_skenario_bad_idr,
    DROP COLUMN IF EXISTS ecl_skenario_normal_idr,
    DROP COLUMN IF EXISTS ecl_skenario_good_idr;

-- 1f. Drop per-scenario PD columns
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS pd_bad,
    DROP COLUMN IF EXISTS pd_good;

-- 1g. Drop asset_class column (standardised enum added in this migration)
--     Existing underlying_kategori is preserved (was present in 000001).
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS asset_class;

-- 1h. Drop fund_composition_id FK column
ALTER TABLE ecl.lookthrough_underlying
    DROP COLUMN IF EXISTS fund_composition_id;

-- 1i. Revert precision on PD / LGD columns
--     WARNING: NUMERIC(10,8) → NUMERIC(8,4) truncates 4 decimal places of precision.
--     Any value stored with more than 4 decimal places will be silently truncated.
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN lgd
        TYPE NUMERIC(8,4) USING lgd::NUMERIC(8,4),
    ALTER COLUMN pd_normal
        TYPE NUMERIC(8,4) USING pd_normal::NUMERIC(8,4);

-- 1j. Revert precision on IDR columns
--     WARNING: NUMERIC(20,4) → NUMERIC(20,2) truncates 2 decimal places of precision.
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN ecl_weighted_idr
        TYPE NUMERIC(20,2) USING ecl_weighted_idr::NUMERIC(20,2),
    ALTER COLUMN ead_underlying_idr
        TYPE NUMERIC(20,2) USING ead_underlying_idr::NUMERIC(20,2);

-- ====================================================================
-- 2. Drop mst.fund_composition_detail
-- ====================================================================

-- 2a. Drop triggers (implicit with table DROP, explicit for clarity)
DROP TRIGGER IF EXISTS trg_fcd_row_version          ON mst.fund_composition_detail;
DROP TRIGGER IF EXISTS trg_fcd_updated_at           ON mst.fund_composition_detail;
DROP TRIGGER IF EXISTS trg_fcd_weight_sum_check     ON mst.fund_composition_detail;

-- 2b. Drop indexes (implicit with table DROP)
DROP INDEX IF EXISTS mst.idx_fcd_composition_id;

-- 2c. Drop table (CASCADE drops FK constraints referencing this table)
DROP TABLE IF EXISTS mst.fund_composition_detail;

-- 2d. Drop trigger function
DROP FUNCTION IF EXISTS fn_fcd_check_weight_sum();

-- ====================================================================
-- 3. Drop mst.fund_composition
-- ====================================================================

-- 3a. Drop triggers (implicit with table DROP, explicit for clarity)
DROP TRIGGER IF EXISTS trg_fc_no_hard_delete        ON mst.fund_composition;
DROP TRIGGER IF EXISTS trg_fc_row_version           ON mst.fund_composition;
DROP TRIGGER IF EXISTS trg_fc_updated_at            ON mst.fund_composition;
DROP TRIGGER IF EXISTS trg_fc_check_reksadana       ON mst.fund_composition;

-- 3b. Drop indexes (implicit with table DROP)
DROP INDEX IF EXISTS mst.idx_fc_tenant_created;
DROP INDEX IF EXISTS mst.idx_fc_approver_id;
DROP INDEX IF EXISTS mst.idx_fc_reviewer_id;
DROP INDEX IF EXISTS mst.idx_fc_maker_id;
DROP INDEX IF EXISTS mst.idx_fc_effective_to;
DROP INDEX IF EXISTS mst.idx_fc_workflow_created;
DROP INDEX IF EXISTS mst.idx_fc_active_approved;
DROP INDEX IF EXISTS mst.idx_fc_instrumen_status;

-- 3c. Drop table
DROP TABLE IF EXISTS mst.fund_composition;

-- 3d. Drop trigger functions
DROP FUNCTION IF EXISTS fn_mst_fc_no_hard_delete();
DROP FUNCTION IF EXISTS fn_fc_check_reksadana();

COMMIT;
