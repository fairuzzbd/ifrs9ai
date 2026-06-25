-- migration: 0045 maturity_akrual_p5m9 DOWN
-- Reverses all changes made in 000045_maturity_akrual_p5m9.up.sql

BEGIN;

-- Remove seeded config
DELETE FROM sys.config_param
WHERE key IN (
    'AKRUAL_STAGING_STALE_DAYS',
    'DIVIDEN_PPH_PCT',
    'DEPOSITO_PPH_BUNGA_PCT',
    'AKRUAL_CRON_SCHEDULE',
    'AMORTISASI_CRON_SCHEDULE',
    'MATURITY_CRON_SCHEDULE',
    'AKRUAL_BATCH_SIZE'
);

-- Remove seeded mapping_jurnal placeholders
DELETE FROM mst.mapping_jurnal
WHERE event_code IN (
    'MATURITY_SETTLEMENT_DEPOSITO',
    'MATURITY_SETTLEMENT_BOND',
    'MATURITY_SETTLEMENT_REKSADANA',
    'AKRUAL_BUNGA',
    'AKRUAL_BUNGA_STAGE3',
    'DIVIDEN',
    'AMORTISASI_PREMIUM',
    'AMORTISASI_DISKON'
);

-- Drop triggers before tables
DROP TRIGGER IF EXISTS trg_dividen_sod_check ON trx.dividen;
DROP FUNCTION IF EXISTS fn_dividen_sod_check();

DROP TRIGGER IF EXISTS trg_dividen_row_version ON trx.dividen;
DROP TRIGGER IF EXISTS trg_dividen_updated_at ON trx.dividen;

DROP TRIGGER IF EXISTS trg_pendapatan_akrual_row_version ON trx.pendapatan_akrual;
DROP TRIGGER IF EXISTS trg_pendapatan_akrual_updated_at ON trx.pendapatan_akrual;

DROP TRIGGER IF EXISTS trg_jatuh_tempo_row_version ON trx.jatuh_tempo;
DROP TRIGGER IF EXISTS trg_jatuh_tempo_updated_at ON trx.jatuh_tempo;

-- Drop tables (partitioned tables drop all partitions too)
DROP TABLE IF EXISTS trx.dividen CASCADE;
DROP TABLE IF EXISTS trx.pendapatan_akrual CASCADE;
DROP TABLE IF EXISTS trx.jatuh_tempo CASCADE;

-- Drop holiday calendar
DROP TABLE IF EXISTS sys.holiday_calendar CASCADE;

COMMIT;
