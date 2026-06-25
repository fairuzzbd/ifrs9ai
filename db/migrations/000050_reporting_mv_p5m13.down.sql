-- migration: 000050 reporting_mv_p5m13 DOWN
-- Reverses 000050 up: drop seeds, tables, and materialized views in reverse dependency order.

BEGIN;

-- Remove config_param seeds
DELETE FROM sys.config_param WHERE key IN (
    'REPORT_EXPORT_INLINE_THRESHOLD',
    'REPORT_EXPORT_MAX_ROWS',
    'REPORT_EXPORT_MINIO_TTL_HOURS',
    'MV_REFRESH_CRON',
    'REPORT_SMTP_RETRY_MAX'
);

-- Drop sys tables (FK order: optout before scheduled_email)
DROP TABLE IF EXISTS sys.scheduled_email_optout;
DROP TABLE IF EXISTS sys.scheduled_email;
DROP TABLE IF EXISTS sys.export_log;
DROP TABLE IF EXISTS sys.mv_refresh_log;

-- Drop materialized views (order doesn't matter; independent)
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_poci_delta_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_penjualan_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_renewal_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_akrual_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_mtm_daily_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_gl_delivery_status;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_jurnal_summary;
DROP MATERIALIZED VIEW IF EXISTS rpt.mv_status_periode;

COMMIT;
