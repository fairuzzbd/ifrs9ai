-- migration: 000051 reports_indexes_p5m14
-- author: backend-engineer-go
-- requires: 000050
-- P5-M14: Composite indexes for hot report queries (RPT-07 MTM Daily, RPT-13 ECL Calc Run Detail,
--         RPT-23/25 Audit Log Browser). Verify not duplicate before applying.
-- References: P5-M14-S2, S3, S5; db-conventions.md §Indexes.

BEGIN;

-- ─── RPT-07 MTM Daily: composite index on (tanggal_mtm, instrumen_id, tenant_id) ─────────────
-- M13 MV rpt.mv_mtm_daily_summary already uses unique (tanggal_mtm, instrumen_id, tenant_id),
-- but the underlying trx.mtm_adjustment table needs this for RPT-07 direct queries.
CREATE INDEX IF NOT EXISTS idx_mtm_adj_tanggal_instrumen
    ON trx.mtm_adjustment (tanggal_mtm, instrumen_id, tenant_id)
    WHERE deleted_at IS NULL;

-- ─── RPT-13 ECL Calc Run Detail: composite index on (calc_run_id, instrumen_id) ───────────────
CREATE INDEX IF NOT EXISTS idx_ecl_result_calc_run
    ON ecl.ecl_calc_result_line (calc_run_id, instrumen_id)
    WHERE deleted_at IS NULL;

-- ─── RPT-23/25 Audit Log Browser: composite index on (action, event_time DESC, tenant_id) ────
-- Supports PERIODE.HARD_CLOSE filter (RPT-23) and general action filter (RPT-25).
CREATE INDEX IF NOT EXISTS idx_audit_log_action_time
    ON aud.audit_log (action, event_time DESC, tenant_id);

COMMIT;
