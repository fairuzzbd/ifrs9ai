-- migration: 0044 penjualan_p5m8 — DOWN (reversal)
-- author: backend-engineer-go
-- description:
--   Reverses migration 0044:
--   (E) Remove sys.config_param BM threshold seeds
--   (D) Remove mst.mapping_jurnal PENJUALAN_* seeds
--   (C) Drop SoD trigger + function
--   (B) Drop updated_at + row_version triggers
--   (A) DROP TABLE trx.penjualan (cascades all partitions + indexes)

BEGIN;

-- (E) Remove config param seeds
DELETE FROM sys.config_param
WHERE key IN ('PENJUALAN_BM_WARN_THRESHOLD_PCT', 'PENJUALAN_BM_BLOCK_THRESHOLD_PCT')
  AND tenant_id = 'TUGURE';

-- (D) Remove mapping_jurnal seeds
DELETE FROM mst.mapping_jurnal
WHERE event_code IN (
    'PENJUALAN_AC',
    'PENJUALAN_FVOCI_DEBT',
    'PENJUALAN_FVOCI_ELECTION',
    'PENJUALAN_FVTPL',
    'PENJUALAN_POCI',
    'REKLAS_OCI_PL'
) AND tenant_id = 'TUGURE'
  AND status = 'DRAFT';

-- (C) Drop SoD trigger + function
DROP TRIGGER IF EXISTS trg_penjualan_sod_check ON trx.penjualan;
DROP FUNCTION IF EXISTS trg_penjualan_sod_check_fn();

-- (B) Drop audit triggers
DROP TRIGGER IF EXISTS trg_penjualan_row_version ON trx.penjualan;
DROP TRIGGER IF EXISTS trg_penjualan_updated_at  ON trx.penjualan;

-- (A) Drop table (CASCADE drops all partitions, indexes, constraints)
DROP TABLE IF EXISTS trx.penjualan CASCADE;

COMMIT;
