-- migration: 000051 reports_indexes_p5m14 (DOWN)
-- author: backend-engineer-go
-- Drops composite indexes added for P5-M14 report queries.

BEGIN;

DROP INDEX IF EXISTS trx.idx_mtm_adj_tanggal_instrumen;
DROP INDEX IF EXISTS ecl.idx_ecl_result_calc_run;
DROP INDEX IF EXISTS aud.idx_audit_log_action_time;

COMMIT;
