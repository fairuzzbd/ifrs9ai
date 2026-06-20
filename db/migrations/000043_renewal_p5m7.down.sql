-- migration: 0043 renewal_p5m7 DOWN
-- author: data-modeler
-- description: Reverse migration 000043. Removes trx.renewal table, triggers,
--              RENEWAL_DEPOSITO mapping_jurnal seed, RENEWAL_MIN_BUNGA_BERSIH_IDR config.

BEGIN;

-- E. Remove config seed
DELETE FROM sys.config_param
WHERE param_key = 'RENEWAL_MIN_BUNGA_BERSIH_IDR'
  AND tenant_id = 'TUGURE';

-- D. Remove mapping_jurnal seed
DELETE FROM mst.mapping_jurnal
WHERE event_code = 'RENEWAL_DEPOSITO'
  AND tenant_id = 'TUGURE';

-- C. Drop SoD trigger + function
DROP TRIGGER IF EXISTS tg_renewal_sod_check ON trx.renewal;
DROP FUNCTION IF EXISTS fn_renewal_sod_check();

-- B. Drop audit triggers (functions are shared, so only drop triggers)
DROP TRIGGER IF EXISTS trg_renewal_updated_at ON trx.renewal;
DROP TRIGGER IF EXISTS trg_renewal_row_version ON trx.renewal;

-- A. Drop table + all partitions (CASCADE drops partitions + indexes)
DROP TABLE IF EXISTS trx.renewal CASCADE;

COMMIT;
