-- migration: 0029 ecl_core_tables (DOWN)
-- author: data-modeler
-- description: Reverses all changes from 000029_ecl_core_tables.up.sql.
--   (A) DROP ecl.calc_result_line (all monthly partitions + default partition)
--   (B) DROP fn_ecl_calc_no_modify_when_sealed + triggers added to ecl.calc_header,
--       DROP indexes added to ecl.calc_header,
--       DROP new columns added to ecl.calc_header,
--       REVERT precision changes on ecl.calc_header back to 000001 originals.
--   (C) DROP triggers + indexes added to ecl.calc_detail_skenario,
--       DROP new columns added to ecl.calc_detail_skenario,
--       REVERT precision changes back to 000001 originals.
-- Note: Reverting NUMERIC precision from NUMERIC(20,4) back to NUMERIC(20,2) will TRUNCATE
--       any data with more than 2 decimal places. This is expected — down migration is
--       for dev/CI rollback only; never run in production after data has been written.

BEGIN;

-- ====================================================================
-- C (reverse). ecl.calc_detail_skenario — remove M7 additions
-- ====================================================================

-- C-4 (reverse). Drop triggers
DROP TRIGGER IF EXISTS trg_ecl_calc_detail_no_delete    ON ecl.calc_detail_skenario;
DROP TRIGGER IF EXISTS trg_ecl_calc_detail_row_version  ON ecl.calc_detail_skenario;
DROP TRIGGER IF EXISTS trg_ecl_calc_detail_updated_at   ON ecl.calc_detail_skenario;

-- C-5 (reverse). Drop index
DROP INDEX IF EXISTS idx_ecl_calc_detail_tenant_created;

-- C-3 (reverse). Drop audit columns
ALTER TABLE ecl.calc_detail_skenario
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS created_at;

-- C-2 (reverse). Drop new M7 columns
ALTER TABLE ecl.calc_detail_skenario
    DROP COLUMN IF EXISTS ead_skenario_idr,
    DROP COLUMN IF EXISTS ecl_fl_idr,
    DROP COLUMN IF EXISTS fl_multiplier;

-- C-1 (reverse). Revert precision to 000001 originals
ALTER TABLE ecl.calc_detail_skenario
    ALTER COLUMN pd_skenario        TYPE NUMERIC(8,4),
    ALTER COLUMN bobot              TYPE NUMERIC(8,4),
    ALTER COLUMN ecl_skenario_idr   TYPE NUMERIC(20,2);


-- ====================================================================
-- B (reverse). ecl.calc_header — remove M7 additions
-- ====================================================================

-- B-7 (reverse). Drop M7-added indexes
DROP INDEX IF EXISTS idx_ecl_calc_header_tenant_created;
DROP INDEX IF EXISTS idx_ecl_calc_header_sealed_at;
DROP INDEX IF EXISTS idx_ecl_calc_header_calc_run_job_id;

-- B-6 (reverse). Drop triggers added in M7
DROP TRIGGER IF EXISTS trg_ecl_calc_header_no_delete                ON ecl.calc_header;
DROP TRIGGER IF EXISTS trg_ecl_calc_header_no_modify_when_sealed    ON ecl.calc_header;
DROP TRIGGER IF EXISTS trg_ecl_calc_header_row_version              ON ecl.calc_header;
DROP TRIGGER IF EXISTS trg_ecl_calc_header_updated_at               ON ecl.calc_header;

-- Drop the sealed-guard function (only drop if no other trigger uses it;
-- safe here because no prior migration installed this function)
DROP FUNCTION IF EXISTS fn_ecl_calc_no_modify_when_sealed();

-- B-5 (reverse). Drop audit columns added in M7
ALTER TABLE ecl.calc_header
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- B-4 (reverse). Restore original status constraint + default
ALTER TABLE ecl.calc_header
    DROP CONSTRAINT IF EXISTS ck_ecl_calc_header_status;

ALTER TABLE ecl.calc_header
    ALTER COLUMN status SET DEFAULT 'POSTED';

-- B-3 (reverse). Drop OQ-M7-2 FK column
ALTER TABLE ecl.calc_header
    DROP COLUMN IF EXISTS calc_run_job_id;

-- B-2 (reverse). Drop M7 new columns
ALTER TABLE ecl.calc_header
    DROP COLUMN IF EXISTS warnings_json,
    DROP COLUMN IF EXISTS catatan,
    DROP COLUMN IF EXISTS sealed_at,
    DROP COLUMN IF EXISTS prior_sealed_ecl_idr,
    DROP COLUMN IF EXISTS net_carrying_idr,
    DROP COLUMN IF EXISTS lgd_used,
    DROP COLUMN IF EXISTS pd_used_bad,
    DROP COLUMN IF EXISTS pd_used_good,
    DROP COLUMN IF EXISTS flag_poci,
    DROP COLUMN IF EXISTS routing_path;

-- B-1 (reverse). Revert precision to 000001 originals
ALTER TABLE ecl.calc_header
    ALTER COLUMN kurs_tengah_bi     TYPE NUMERIC(15,4),
    ALTER COLUMN w_bad              TYPE NUMERIC(8,4),
    ALTER COLUMN w_normal           TYPE NUMERIC(8,4),
    ALTER COLUMN w_good             TYPE NUMERIC(8,4),
    ALTER COLUMN impact_pd          TYPE NUMERIC(8,4),
    ALTER COLUMN impact_mev_bad     TYPE NUMERIC(8,4),
    ALTER COLUMN impact_mev_good    TYPE NUMERIC(8,4),
    ALTER COLUMN pd_normal          TYPE NUMERIC(8,4),
    ALTER COLUMN lgd                TYPE NUMERIC(8,4),
    ALTER COLUMN delta_ecl_fl_idr   TYPE NUMERIC(20,2),
    ALTER COLUMN ecl_fl_idr         TYPE NUMERIC(20,2),
    ALTER COLUMN ecl_weighted_idr   TYPE NUMERIC(20,2),
    ALTER COLUMN ead_idr            TYPE NUMERIC(20,2),
    ALTER COLUMN ead_native         TYPE NUMERIC(20,2);


-- ====================================================================
-- A (reverse). DROP ecl.calc_result_line (all partitions)
-- ====================================================================

-- Drop child partition tables first (PG requires detach/drop or CASCADE)
DROP TABLE IF EXISTS ecl.calc_result_line_y2026m09;
DROP TABLE IF EXISTS ecl.calc_result_line_y2026m08;
DROP TABLE IF EXISTS ecl.calc_result_line_y2026m07;
DROP TABLE IF EXISTS ecl.calc_result_line_y2026m06;
DROP TABLE IF EXISTS ecl.calc_result_line_default;

-- Drop parent partitioned table (also drops indexes, triggers, constraints defined on parent)
DROP TABLE IF EXISTS ecl.calc_result_line;

-- Drop sealed-guard function if not already dropped above
-- (safe no-op if dropped in B section)
DROP FUNCTION IF EXISTS fn_ecl_calc_no_modify_when_sealed();

COMMIT;
