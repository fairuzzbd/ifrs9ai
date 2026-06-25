-- migration: 000050 reporting_mv_p5m13
-- author: backend-engineer-go
-- requires: 000049
-- P5-M13: 8 Materialized Views (rpt.*), sys.mv_refresh_log, sys.export_log,
--         sys.scheduled_email, sys.scheduled_email_optout, sys.config_param seeds.
-- References: P5-M13-S1..S5, db-conventions.md, DEC-018, DEC-021.

BEGIN;

-- ---------------------------------------------------------------------------
-- Create rpt schema (idempotent; may already exist from earlier migrations)
-- ---------------------------------------------------------------------------
CREATE SCHEMA IF NOT EXISTS rpt;

-- ---------------------------------------------------------------------------
-- 8 Materialized Views (skeleton aggregations; real queries in M14)
-- Each MV needs a UNIQUE index for REFRESH MATERIALIZED VIEW CONCURRENTLY.
-- ---------------------------------------------------------------------------

-- 1. mv_status_periode
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_status_periode AS
SELECT
    pb.id                AS periode_id,
    pb.periode_bulanan,
    pb.status_periode,
    pb.soft_close_at,
    pb.hard_close_at,
    pb.created_at,
    pb.updated_at,
    pb.tenant_id
FROM mst.periode_buku pb
WHERE pb.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_status_periode_pid_tid
    ON rpt.mv_status_periode (periode_id, tenant_id);

-- 2. mv_jurnal_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_jurnal_summary AS
SELECT
    jh.periode_bulanan_id                    AS periode_id,
    jh.event_code,
    COUNT(*)                                 AS jurnal_count,
    COALESCE(SUM(jd.amount_idr), 0::NUMERIC) AS total_amount,
    jh.tenant_id,
    MIN(jh.created_at)                       AS first_posted_at,
    MAX(jh.created_at)                       AS last_posted_at
FROM jrnl.jurnal_header jh
LEFT JOIN jrnl.jurnal_detail jd ON jd.jurnal_header_id = jh.id
    AND jd.deleted_at IS NULL
WHERE jh.deleted_at IS NULL
GROUP BY jh.periode_bulanan_id, jh.event_code, jh.tenant_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_jurnal_summary_pid_ec_tid
    ON rpt.mv_jurnal_summary (periode_id, event_code, tenant_id);

-- 3. mv_gl_delivery_status
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_gl_delivery_status AS
SELECT
    gd.id                AS delivery_id,
    gd.jurnal_header_id,
    gd.periode_id,
    gd.gl_host_status,
    gd.attempt_count,
    gd.last_attempt_at,
    gd.delivered_at,
    gd.created_at,
    gd.tenant_id
FROM jrnl.gl_delivery gd
WHERE gd.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_gl_delivery_pid_did_tid
    ON rpt.mv_gl_delivery_status (periode_id, delivery_id, tenant_id);

-- 4. mv_mtm_daily_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_mtm_daily_summary AS
SELECT
    ma.tanggal_mtm,
    ma.instrumen_id,
    ma.harga_pasar_idr,
    ma.unrealized_gainloss_idr,
    ma.created_at,
    ma.tenant_id
FROM trx.mtm_adjustment ma
WHERE ma.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_mtm_daily_tid_iid_tid
    ON rpt.mv_mtm_daily_summary (tanggal_mtm, instrumen_id, tenant_id);

-- 5. mv_akrual_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_akrual_summary AS
SELECT
    ak.periode_id,
    ak.instrumen_id,
    ak.jenis_akrual,
    ak.jumlah_idr,
    ak.tanggal_akrual,
    ak.created_at,
    ak.tenant_id
FROM trx.akrual ak
WHERE ak.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_akrual_pid_iid_tid
    ON rpt.mv_akrual_summary (periode_id, instrumen_id, tenant_id);

-- 6. mv_renewal_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_renewal_summary AS
SELECT
    rn.periode_id,
    rn.instrumen_id,
    rn.skema_renewal,
    rn.nominal_pokok_idr,
    rn.kupon_baru_persen,
    rn.workflow_status,
    rn.created_at,
    rn.tenant_id
FROM trx.renewal rn
WHERE rn.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_renewal_pid_iid_tid
    ON rpt.mv_renewal_summary (periode_id, instrumen_id, tenant_id);

-- 7. mv_penjualan_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_penjualan_summary AS
SELECT
    pp.periode_id,
    pp.instrumen_id,
    pp.qty_terjual,
    pp.harga_jual_per_unit,
    pp.proceeds_idr,
    pp.realized_gainloss_idr,
    pp.workflow_status,
    pp.created_at,
    pp.tenant_id
FROM trx.penjualan_pencairan pp
WHERE pp.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_penjualan_pid_iid_tid
    ON rpt.mv_penjualan_summary (periode_id, instrumen_id, tenant_id);

-- 8. mv_poci_delta_summary
CREATE MATERIALIZED VIEW IF NOT EXISTS rpt.mv_poci_delta_summary AS
SELECT
    pd.periode_id,
    pd.instrumen_id,
    pd.delta_ecl_idr,
    pd.direction,
    pd.created_at,
    pd.tenant_id
FROM ecl.poci_delta_log pd
WHERE pd.deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mv_poci_delta_pid_iid_tid
    ON rpt.mv_poci_delta_summary (periode_id, instrumen_id, tenant_id);

-- ---------------------------------------------------------------------------
-- sys.mv_refresh_log — Track MV refresh status per run
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sys.mv_refresh_log (
    id              UUID         NOT NULL DEFAULT gen_random_uuid(),
    mv_name         TEXT         NOT NULL,
    triggered_by    TEXT         NOT NULL CHECK (triggered_by IN ('CRON','HARD_CLOSE','MANUAL')),
    trigger_actor   UUID,
    status          TEXT         NOT NULL CHECK (status IN ('RUNNING','COMPLETED','FAILED')) DEFAULT 'RUNNING',
    started_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    row_count       BIGINT,
    error_detail    TEXT,
    -- audit cols (db-conventions.md)
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_by      UUID         NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::UUID,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_by      UUID         NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::UUID,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID,
    row_version     BIGINT       NOT NULL DEFAULT 1,
    tenant_id       TEXT         NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_mv_refresh_log PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_mv_refresh_log_mv_name
    ON sys.mv_refresh_log (mv_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_mv_refresh_log_status
    ON sys.mv_refresh_log (status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_mv_refresh_log_tenant
    ON sys.mv_refresh_log (tenant_id, started_at DESC);

-- ---------------------------------------------------------------------------
-- sys.export_log — Track export requests + SHA-256 + MinIO path
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sys.export_log (
    id              UUID         NOT NULL DEFAULT gen_random_uuid(),
    report_slug     TEXT         NOT NULL,
    format          TEXT         NOT NULL CHECK (format IN ('csv','xlsx','pdf')),
    params_jsonb    JSONB,
    status          TEXT         NOT NULL CHECK (status IN ('REQUESTED','COMPUTING','COMPLETED','FAILED')) DEFAULT 'REQUESTED',
    row_count       BIGINT,
    file_minio_path TEXT,
    sha256_hash     TEXT,
    signed_url      TEXT,
    requested_by    UUID         NOT NULL,
    requested_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    downloaded_at   TIMESTAMPTZ,
    job_id          TEXT,
    -- audit cols
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_by      UUID         NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_by      UUID         NOT NULL,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID,
    row_version     BIGINT       NOT NULL DEFAULT 1,
    tenant_id       TEXT         NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_export_log PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_export_log_requested_by
    ON sys.export_log (requested_by, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_export_log_status
    ON sys.export_log (status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_export_log_tenant
    ON sys.export_log (tenant_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_export_log_job_id
    ON sys.export_log (job_id) WHERE job_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- sys.scheduled_email — Per-report scheduled email configuration
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sys.scheduled_email (
    id               UUID         NOT NULL DEFAULT gen_random_uuid(),
    report_slug      TEXT         NOT NULL,
    format           TEXT         NOT NULL CHECK (format IN ('csv','xlsx','pdf')),
    frequency        TEXT         NOT NULL CHECK (frequency IN ('daily','weekly','monthly')),
    send_time        TEXT         NOT NULL,       -- HH:MM+07:00
    recipients_jsonb JSONB        NOT NULL,       -- array of email strings
    active           BOOLEAN      NOT NULL DEFAULT true,
    subject_template TEXT,
    body_template    TEXT,
    last_sent_at     TIMESTAMPTZ,
    last_status      TEXT         CHECK (last_status IN ('SENT','FAILED','PENDING')),
    last_error       TEXT,
    -- audit cols
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_by       UUID         NOT NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_by       UUID         NOT NULL,
    deleted_at       TIMESTAMPTZ,
    deleted_by       UUID,
    row_version      BIGINT       NOT NULL DEFAULT 1,
    tenant_id        TEXT         NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_scheduled_email PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_email_active
    ON sys.scheduled_email (active, tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_scheduled_email_tenant
    ON sys.scheduled_email (tenant_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- sys.scheduled_email_optout — Per-recipient opt-out (append-only)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sys.scheduled_email_optout (
    id                  UUID        NOT NULL DEFAULT gen_random_uuid(),
    scheduled_email_id  UUID        NOT NULL REFERENCES sys.scheduled_email(id),
    email               TEXT        NOT NULL,
    opted_out_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    token_hash          TEXT        NOT NULL,    -- SHA-256 of HMAC token for traceability
    tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_scheduled_email_optout PRIMARY KEY (id),
    CONSTRAINT uq_scheduled_email_optout_sched_email
        UNIQUE (scheduled_email_id, email)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_email_optout_sched
    ON sys.scheduled_email_optout (scheduled_email_id);

-- ---------------------------------------------------------------------------
-- sys.config_param seeds — P5-M13 reporting parameters
-- Insert or ignore (don't overwrite existing config)
-- ---------------------------------------------------------------------------
INSERT INTO sys.config_param (key, value, keterangan, created_by, updated_by)
SELECT key, value, keterangan,
       '00000000-0000-0000-0000-000000000000'::UUID,
       '00000000-0000-0000-0000-000000000000'::UUID
FROM (VALUES
    ('REPORT_EXPORT_INLINE_THRESHOLD', '10000',
     'Row count threshold: ≤ this → inline export (HTTP 200); > → async (HTTP 202)'),
    ('REPORT_EXPORT_MAX_ROWS', '100000',
     'Row count hard cap: > this → EXPORT_TOO_LARGE (HTTP 422)'),
    ('REPORT_EXPORT_MINIO_TTL_HOURS', '24',
     'MinIO presigned URL TTL in hours for async exports'),
    ('MV_REFRESH_CRON', '0 18 * * *',
     'Asynq cron schedule for nightly MV refresh (UTC; 18:00 UTC = 01:00 WIB)'),
    ('REPORT_SMTP_RETRY_MAX', '3',
     'Max SMTP retry attempts for scheduled email before DLQ')
) AS t(key, value, keterangan)
WHERE NOT EXISTS (
    SELECT 1 FROM sys.config_param cp WHERE cp.key = t.key
);

COMMIT;
