-- ====================================================================
-- BLIPS IFRS 9 — INITIAL DATABASE SCHEMA SCRIPT
-- ====================================================================
-- Document  : BLIPS_init_schema.sql
-- Reference : ERD-BLIPS-IFRS9-2026-001 v1.2 + Sample Seed Data v1.3 (comprehensive sample for Phase 1 dev)
-- Author    : Database Architect Tugure
-- Date      : 29 Mei 2026 (DDL v1.3 — comprehensive sample seed data)
-- Target    : PostgreSQL 15+
-- Purpose   : Initialize 9 schemas + ~50 tables + indexes + triggers + seed data
--
-- Execution :
--   psql -U blips_admin -d blips_db -f BLIPS_init_schema.sql
--
-- Pre-requisite:
--   - PostgreSQL 15 or higher
--   - Database 'blips_db' exists
--   - User 'blips_admin' has CREATE privilege
--   - Extensions pgcrypto installed
-- ====================================================================

\echo '======================================================'
\echo '  BLIPS IFRS 9 — Schema Initialization Starting...    '
\echo '======================================================'

-- ====================================================================
-- 0. EXTENSIONS & PREREQUISITES
-- ====================================================================
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gin;
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- UUID v7 (time-ordered) — replace with extension if available, or use plpgsql:
CREATE OR REPLACE FUNCTION uuidv7() RETURNS UUID AS $$
DECLARE
    v_time_ms BIGINT;
    v_random BYTEA;
BEGIN
    v_time_ms := (extract(epoch from clock_timestamp()) * 1000)::BIGINT;
    v_random := gen_random_bytes(10);
    RETURN (
        lpad(to_hex((v_time_ms >> 16) & 4294967295), 8, '0') ||
        lpad(to_hex(v_time_ms & 65535), 4, '0') ||
        '7' || encode(substring(v_random, 1, 1), 'hex')::text || substring(encode(substring(v_random, 1, 2), 'hex'), 1, 3) ||
        '8' || substring(encode(substring(v_random, 3, 2), 'hex'), 1, 3) ||
        encode(substring(v_random, 5, 6), 'hex')
    )::UUID;
END;
$$ LANGUAGE plpgsql;

-- ====================================================================
-- 1. CREATE SCHEMAS
-- ====================================================================
\echo '  Creating schemas...'
CREATE SCHEMA IF NOT EXISTS sec;
CREATE SCHEMA IF NOT EXISTS sys;
CREATE SCHEMA IF NOT EXISTS mst;
CREATE SCHEMA IF NOT EXISTS doc;
CREATE SCHEMA IF NOT EXISTS sppi;
CREATE SCHEMA IF NOT EXISTS trx;
CREATE SCHEMA IF NOT EXISTS jrnl;
CREATE SCHEMA IF NOT EXISTS ecl;
CREATE SCHEMA IF NOT EXISTS aud;

COMMENT ON SCHEMA sec IS 'Security & RBAC — users, roles, permissions, sessions';
COMMENT ON SCHEMA sys IS 'System configuration & lookup data';
COMMENT ON SCHEMA mst IS 'Master/Reference data — instrumen, counterparty, parameters';
COMMENT ON SCHEMA doc IS 'Document management — uploads, links, access log';
COMMENT ON SCHEMA sppi IS 'SPPI Test, BM Test, Klasifikasi PSAK 71, Reklasifikasi';
COMMENT ON SCHEMA trx IS 'Transactional data — penempatan, MTM, renewal, etc.';
COMMENT ON SCHEMA jrnl IS 'Jurnal accounting & GL Host interface';
COMMENT ON SCHEMA ecl IS 'ECL/EIR computation results & schedules';
COMMENT ON SCHEMA aud IS 'Immutable audit trail — append-only';

-- ====================================================================
-- 2. SCHEMA: sec (Security & RBAC) — FOUNDATION
-- ====================================================================
\echo '  Creating sec schema tables...'

-- 2.1 sec.user
CREATE TABLE sec.user (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    username                 VARCHAR(100) NOT NULL,
    email                    VARCHAR(200) NOT NULL,
    full_name                VARCHAR(200) NOT NULL,
    display_name             VARCHAR(100),
    unit_kerja               VARCHAR(100),
    jabatan                  VARCHAR(100),
    mfa_enrolled             BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method               VARCHAR(20),
    status                   VARCHAR(20) NOT NULL DEFAULT 'AKTIF',
    locked_at                TIMESTAMPTZ,
    locked_reason            VARCHAR(100),
    last_login_at            TIMESTAMPTZ,
    external_idp_id          VARCHAR(200),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by               UUID,
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_user_username UNIQUE (username),
    CONSTRAINT uq_user_email UNIQUE (email),
    CONSTRAINT ck_user_status CHECK (status IN ('AKTIF','TIDAK_AKTIF','LOCKED','PENDING_ACTIVATION'))
);
ALTER TABLE sec.user ADD CONSTRAINT fk_user_created_by FOREIGN KEY (created_by) REFERENCES sec.user(id);
CREATE INDEX ix_user_status ON sec.user(status);
CREATE INDEX ix_user_idp ON sec.user(external_idp_id) WHERE external_idp_id IS NOT NULL;

-- 2.2 sec.role
CREATE TABLE sec.role (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    role_code                VARCHAR(50) NOT NULL,
    nama_role                VARCHAR(200) NOT NULL,
    deskripsi                TEXT,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_role_code UNIQUE (role_code)
);
CREATE INDEX ix_role_aktif ON sec.role(aktif_flag);

-- 2.3 sec.permission
CREATE TABLE sec.permission (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    permission_code          VARCHAR(100) NOT NULL,
    entity                   VARCHAR(50) NOT NULL,
    action                   VARCHAR(30) NOT NULL,
    deskripsi                VARCHAR(500),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_permission_code UNIQUE (permission_code)
);
CREATE INDEX ix_permission_entity ON sec.permission(entity);

-- 2.4 sec.user_role (junction)
CREATE TABLE sec.user_role (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id                  UUID NOT NULL REFERENCES sec.user(id) ON DELETE CASCADE,
    role_id                  UUID NOT NULL REFERENCES sec.role(id),
    assigned_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    assigned_by              UUID NOT NULL REFERENCES sec.user(id),
    expires_at               TIMESTAMPTZ,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE UNIQUE INDEX uq_user_role_active ON sec.user_role(user_id, role_id) WHERE aktif_flag=TRUE;
CREATE INDEX ix_user_role_user ON sec.user_role(user_id) WHERE aktif_flag=TRUE;

-- 2.5 sec.role_permission (junction)
CREATE TABLE sec.role_permission (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    role_id                  UUID NOT NULL REFERENCES sec.role(id) ON DELETE CASCADE,
    permission_id            UUID NOT NULL REFERENCES sec.permission(id),
    assigned_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_role_permission UNIQUE (role_id, permission_id)
);
CREATE INDEX ix_role_permission_role ON sec.role_permission(role_id);

-- 2.6 sec.session
CREATE TABLE sec.session (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id                  UUID NOT NULL REFERENCES sec.user(id) ON DELETE CASCADE,
    session_token            VARCHAR(500) NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at               TIMESTAMPTZ NOT NULL,
    last_activity_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_address               INET,
    user_agent               VARCHAR(500),
    status                   VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    revoked_at               TIMESTAMPTZ,
    revoke_reason            VARCHAR(100),
    CONSTRAINT uq_session_token UNIQUE (session_token),
    CONSTRAINT ck_session_status CHECK (status IN ('ACTIVE','EXPIRED','REVOKED'))
);
CREATE INDEX ix_session_user_active ON sec.session(user_id) WHERE status='ACTIVE';
CREATE INDEX ix_session_expires ON sec.session(expires_at) WHERE status='ACTIVE';

-- ====================================================================
-- 3. SCHEMA: sys (System Configuration)
-- ====================================================================
\echo '  Creating sys schema tables...'

-- 3.1 sys.config
CREATE TABLE sys.config (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    config_key               VARCHAR(100) NOT NULL,
    config_value             TEXT NOT NULL,
    config_type              VARCHAR(20) NOT NULL,
    sensitive                BOOLEAN NOT NULL DEFAULT FALSE,
    description              TEXT,
    category                 VARCHAR(50),
    updated_by               UUID REFERENCES sec.user(id),
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_config_key UNIQUE (config_key),
    CONSTRAINT ck_config_type CHECK (config_type IN ('STRING','INT','BOOLEAN','JSON'))
);
CREATE INDEX ix_config_category ON sys.config(category);

-- 3.2 sys.lookup
CREATE TABLE sys.lookup (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    lookup_group             VARCHAR(50) NOT NULL,
    lookup_key               VARCHAR(50) NOT NULL,
    lookup_value             VARCHAR(200) NOT NULL,
    description              TEXT,
    sort_order               INT NOT NULL DEFAULT 0,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    metadata                 JSONB,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_lookup UNIQUE (lookup_group, lookup_key)
);
CREATE INDEX ix_lookup_group_aktif ON sys.lookup(lookup_group, aktif_flag, sort_order);

-- 3.3 sys.notification_template
CREATE TABLE sys.notification_template (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    template_code            VARCHAR(50) NOT NULL,
    channel                  VARCHAR(20) NOT NULL,
    subject_template         VARCHAR(500),
    body_template            TEXT NOT NULL,
    variables_schema         JSONB,
    language                 VARCHAR(5) NOT NULL DEFAULT 'id-ID',
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_notif_template UNIQUE (template_code, channel, language)
);
CREATE INDEX ix_notif_aktif ON sys.notification_template(template_code, aktif_flag);

-- 3.4 sys.job_run_history
CREATE TABLE sys.job_run_history (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    job_name                 VARCHAR(100) NOT NULL,
    job_type                 VARCHAR(30) NOT NULL,
    triggered_by             UUID REFERENCES sec.user(id),
    periode_id               UUID,
    started_at               TIMESTAMPTZ NOT NULL,
    completed_at             TIMESTAMPTZ,
    status                   VARCHAR(20) NOT NULL DEFAULT 'RUNNING',
    records_processed        INT DEFAULT 0,
    records_failed           INT DEFAULT 0,
    error_message            TEXT,
    parameters_snapshot_json JSONB,
    execution_log_url        VARCHAR(500),
    duration_seconds         INT
);
CREATE INDEX ix_job_run_name_time ON sys.job_run_history(job_name, started_at DESC);
CREATE INDEX ix_job_run_status ON sys.job_run_history(status) WHERE status IN ('RUNNING','FAILED');

-- 3.5 sys.upload_batch (NEW v1.2)
CREATE TABLE sys.upload_batch (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    batch_code                  VARCHAR(30) NOT NULL,
    batch_type                  VARCHAR(20) NOT NULL,
    batch_mode                  VARCHAR(20),
    filename_original           VARCHAR(500) NOT NULL,
    file_sha256                 CHAR(64) NOT NULL,
    file_storage_url            VARCHAR(500) NOT NULL,
    uploaded_by                 UUID NOT NULL REFERENCES sec.user(id),
    uploaded_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    tanggal_valuasi             DATE,
    portofolio_target_id        UUID,
    total_rows                  INT NOT NULL DEFAULT 0,
    valid_rows                  INT NOT NULL DEFAULT 0,
    warning_rows                INT NOT NULL DEFAULT 0,
    rejected_rows               INT NOT NULL DEFAULT 0,
    committed_rows              INT NOT NULL DEFAULT 0,
    sheet_breakdown_json        JSONB,
    status                      VARCHAR(30) NOT NULL DEFAULT 'PARSING',
    reviewer_id                 UUID REFERENCES sec.user(id),
    approver_id                 UUID REFERENCES sec.user(id),
    approved_at                 TIMESTAMPTZ,
    rejected_at                 TIMESTAMPTZ,
    reject_reason               TEXT,
    committed_at                TIMESTAMPTZ,
    committed_instrumen_ids     UUID[],
    rollback_status             VARCHAR(20),
    rollback_by                 UUID REFERENCES sec.user(id),
    rollback_at                 TIMESTAMPTZ,
    rollback_reason             TEXT,
    error_summary_json          JSONB,
    processing_metadata_json    JSONB,
    CONSTRAINT uq_upload_batch_code UNIQUE (batch_code),
    CONSTRAINT ck_batch_type CHECK (batch_type IN ('MTM_UPLOAD','INSTRUMEN_BULK','IMPACT_MEV','PD_PEFINDO','FUND_FACT_SHEET')),
    CONSTRAINT ck_batch_status CHECK (status IN ('PARSING','STAGED','PENDING_REVIEW','PENDING_APPROVAL','APPROVED','REJECTED','COMMITTING','COMMITTED','FAILED','ROLLED_BACK')),
    CONSTRAINT ck_batch_mode CHECK (batch_mode IS NULL OR batch_mode IN ('STANDARD','MIGRATION','TOPUP','DRY_RUN')),
    CONSTRAINT ck_rollback_status CHECK (rollback_status IS NULL OR rollback_status IN ('PENDING_ROLLBACK','ROLLED_BACK'))
);
CREATE INDEX ix_upload_batch_type_status ON sys.upload_batch(batch_type, status);
CREATE INDEX ix_upload_batch_uploader ON sys.upload_batch(uploaded_by, uploaded_at DESC);
CREATE INDEX ix_upload_batch_committed ON sys.upload_batch(committed_at DESC) WHERE status='COMMITTED';
CREATE INDEX ix_upload_batch_rollback ON sys.upload_batch(rollback_status) WHERE rollback_status IS NOT NULL;

-- 3.6 sys.upload_batch_row (NEW v1.2)
CREATE TABLE sys.upload_batch_row (
    id                                  UUID PRIMARY KEY DEFAULT uuidv7(),
    batch_id                            UUID NOT NULL REFERENCES sys.upload_batch(id) ON DELETE CASCADE,
    row_number                          INT NOT NULL,
    sheet_name                          VARCHAR(50),
    row_data_json                       JSONB NOT NULL,
    instrumen_id                        UUID,
    sumber_harga                        VARCHAR(30),
    harga_native                        NUMERIC(15,4),
    harga_sebelumnya                    NUMERIC(15,4),
    deviation_pct                       NUMERIC(8,4),
    status_validation                   VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    validation_errors_json              JSONB,
    validation_warnings_json            JSONB,
    preview_master_instrumen_json       JSONB,
    override_flag                       BOOLEAN NOT NULL DEFAULT FALSE,
    override_reason                     TEXT,
    override_by                         UUID REFERENCES sec.user(id),
    override_at                         TIMESTAMPTZ,
    committed_to_instrumen_id           UUID,
    committed_to_mtm_id                 UUID,
    status_commit                       VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    commit_error                        TEXT,
    created_at                          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                          TIMESTAMPTZ,
    CONSTRAINT uq_batch_row UNIQUE (batch_id, row_number),
    CONSTRAINT ck_row_status_validation CHECK (status_validation IN (
        'PENDING','VALID',
        'WARNING_PRICE_DEVIATION','WARNING_DUPLICATE','WARNING_FK_FUZZY',
        'REJECTED_FK_MISSING','REJECTED_REQUIRED_FIELD','REJECTED_BUSINESS_RULE',
        'REJECTED_CURRENCY_MISMATCH','REJECTED_INSTRUMEN_TIDAK_DITEMUKAN',
        'REJECTED_KURS_TIDAK_TERSEDIA','REJECTED_DUPLICATE_POSTED'
    )),
    CONSTRAINT ck_row_status_commit CHECK (status_commit IN ('PENDING','COMMITTED','SKIPPED','FAILED'))
);
CREATE INDEX ix_batch_row_status ON sys.upload_batch_row(batch_id, status_validation);
CREATE INDEX ix_batch_row_instrumen ON sys.upload_batch_row(instrumen_id) WHERE instrumen_id IS NOT NULL;
CREATE INDEX ix_batch_row_committed ON sys.upload_batch_row(committed_to_instrumen_id) WHERE committed_to_instrumen_id IS NOT NULL;
CREATE INDEX ix_batch_row_override ON sys.upload_batch_row(override_flag) WHERE override_flag=TRUE;

-- FK ke mst.instrumen dan trx.mtm akan ditambahkan setelah tabel target dibuat
-- (lihat ALTER TABLE section di bawah, setelah mst.instrumen dan trx.mtm created)


-- ====================================================================
-- 4. SCHEMA: mst (Master Data)
-- ====================================================================
\echo '  Creating mst schema tables...'

-- 4.1 mst.portofolio
CREATE TABLE mst.portofolio (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    kode_portofolio          VARCHAR(20) NOT NULL,
    nama                     VARCHAR(200) NOT NULL,
    tujuan_pengelolaan       TEXT,
    bm_category_default      VARCHAR(10) NOT NULL DEFAULT 'HTC',
    benchmark                VARCHAR(100),
    kompensasi_manager_basis VARCHAR(50),
    periode_review_terakhir  DATE,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    created_by               UUID NOT NULL REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by               UUID REFERENCES sec.user(id),
    updated_at               TIMESTAMPTZ,
    version                  INT NOT NULL DEFAULT 1,
    is_deleted               BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT ck_portofolio_bm CHECK (bm_category_default IN ('HTC','HTCS','OTHER'))
);
CREATE UNIQUE INDEX uq_portofolio_kode ON mst.portofolio(kode_portofolio) WHERE is_deleted=FALSE;
CREATE INDEX ix_portofolio_aktif ON mst.portofolio(aktif_flag) WHERE is_deleted=FALSE;

-- 4.2 mst.counterparty
CREATE TABLE mst.counterparty (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    kode_counterparty        VARCHAR(20) NOT NULL,
    nama                     VARCHAR(200) NOT NULL,
    tipe                     VARCHAR(30) NOT NULL,
    rating_pefindo_current   VARCHAR(8),
    tipe_eksposur_basel      VARCHAR(30) NOT NULL,
    eligible_lps_flag        BOOLEAN NOT NULL DEFAULT FALSE,
    npwp_encrypted           VARCHAR(255),
    nomor_izin_ojk           VARCHAR(40),
    tanggal_izin_ojk         DATE,
    aum_terakhir             NUMERIC(20,2),
    tanggal_aum_terakhir     DATE,
    kategori_mi              VARCHAR(30),
    status                   VARCHAR(20) NOT NULL DEFAULT 'AKTIF',
    created_by               UUID NOT NULL REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by               UUID REFERENCES sec.user(id),
    updated_at               TIMESTAMPTZ,
    version                  INT NOT NULL DEFAULT 1,
    is_deleted               BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT ck_counterparty_tipe CHECK (tipe IN ('BANK','BANK_KUSTODIAN','KORPORASI','PEMERINTAH','MANAJER_INVESTASI','EMITEN_SAHAM')),
    CONSTRAINT ck_counterparty_eksposur CHECK (tipe_eksposur_basel IN ('SOVEREIGN','SENIOR_SECURED','SENIOR_UNSECURED','SUBORDINATED'))
);
CREATE UNIQUE INDEX uq_counterparty_kode ON mst.counterparty(kode_counterparty) WHERE is_deleted=FALSE;
CREATE INDEX ix_counterparty_tipe ON mst.counterparty(tipe) WHERE is_deleted=FALSE;
CREATE INDEX ix_counterparty_rating ON mst.counterparty(rating_pefindo_current) WHERE is_deleted=FALSE;
CREATE INDEX ix_counterparty_lps ON mst.counterparty(eligible_lps_flag) WHERE eligible_lps_flag=TRUE AND is_deleted=FALSE;

-- 4.3 mst.rating_history_counterparty
CREATE TABLE mst.rating_history_counterparty (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    rating_history_id_kode   VARCHAR(20) NOT NULL,
    counterparty_id          UUID NOT NULL REFERENCES mst.counterparty(id),
    tanggal_berlaku          DATE NOT NULL,
    tanggal_berakhir         DATE,
    rating_pefindo           VARCHAR(8) NOT NULL,
    rating_outlook           VARCHAR(20),
    sumber_rating            VARCHAR(30) NOT NULL,
    tanggal_publikasi_rating DATE NOT NULL,
    action_type              VARCHAR(20) NOT NULL,
    notch_change             INT NOT NULL DEFAULT 0,
    sicr_triggered           BOOLEAN NOT NULL DEFAULT FALSE,
    default_triggered        BOOLEAN NOT NULL DEFAULT FALSE,
    dokumen_bukti_id         UUID,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_rating_history_kode UNIQUE (rating_history_id_kode),
    CONSTRAINT ck_rating_action CHECK (action_type IN ('INITIAL','UPGRADE','DOWNGRADE','AFFIRMED','WITHDRAWN','CORRECTION'))
);
CREATE INDEX ix_rating_cp_tanggal ON mst.rating_history_counterparty(counterparty_id, tanggal_berlaku DESC);
CREATE UNIQUE INDEX uq_rating_aktif ON mst.rating_history_counterparty(counterparty_id) WHERE tanggal_berakhir IS NULL;
CREATE INDEX ix_rating_sicr ON mst.rating_history_counterparty(sicr_triggered) WHERE sicr_triggered=TRUE;
CREATE INDEX ix_rating_default ON mst.rating_history_counterparty(default_triggered) WHERE default_triggered=TRUE;

-- 4.4 mst.pd_pefindo
CREATE TABLE mst.pd_pefindo (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    rating                   VARCHAR(8) NOT NULL,
    pd_12month               NUMERIC(8,4) NOT NULL,
    pd_lifetime_3y           NUMERIC(8,4),
    pd_lifetime_5y           NUMERIC(8,4),
    pd_lifetime_7y           NUMERIC(8,4),
    pd_lifetime_10y          NUMERIC(8,4),
    sumber                   VARCHAR(50) NOT NULL,
    tanggal_publikasi        DATE NOT NULL,
    periode_berlaku_dari     DATE NOT NULL,
    periode_berlaku_sampai   DATE,
    dokumen_pendukung_id     UUID,
    uploaded_by              UUID NOT NULL REFERENCES sec.user(id),
    uploaded_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_by              UUID REFERENCES sec.user(id),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT ck_pd_range CHECK (pd_12month BETWEEN 0 AND 1)
);
CREATE INDEX ix_pd_pefindo_rating_periode ON mst.pd_pefindo(rating, periode_berlaku_dari DESC);
CREATE INDEX ix_pd_pefindo_current ON mst.pd_pefindo(rating) WHERE periode_berlaku_sampai IS NULL;

-- 4.5 mst.lgd_basel
CREATE TABLE mst.lgd_basel (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    tipe_eksposur            VARCHAR(30) NOT NULL,
    lgd                      NUMERIC(8,4) NOT NULL,
    karakteristik            TEXT,
    periode_berlaku_dari     DATE NOT NULL,
    periode_berlaku_sampai   DATE,
    sumber                   VARCHAR(50) NOT NULL DEFAULT 'BASEL_III_IRB',
    dokumen_pendukung_id     UUID,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT ck_lgd_range CHECK (lgd BETWEEN 0 AND 1)
);
CREATE INDEX ix_lgd_tipe_periode ON mst.lgd_basel(tipe_eksposur, periode_berlaku_dari DESC);
CREATE INDEX ix_lgd_current ON mst.lgd_basel(tipe_eksposur) WHERE periode_berlaku_sampai IS NULL;

-- 4.6 mst.bobot_skenario
CREATE TABLE mst.bobot_skenario (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    skenario                 VARCHAR(20) NOT NULL,
    bobot                    NUMERIC(8,4) NOT NULL,
    periode_berlaku_dari     DATE NOT NULL,
    periode_berlaku_sampai   DATE,
    catatan                  TEXT,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT ck_bobot_range CHECK (bobot BETWEEN 0 AND 1),
    CONSTRAINT ck_skenario CHECK (skenario IN ('GOOD','NORMAL','BAD'))
);
CREATE INDEX ix_bobot_skenario_periode ON mst.bobot_skenario(skenario, periode_berlaku_dari DESC);
CREATE INDEX ix_bobot_current ON mst.bobot_skenario(skenario) WHERE periode_berlaku_sampai IS NULL;

-- 4.7 mst.periode_buku
CREATE TABLE mst.periode_buku (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    periode_id_kode          VARCHAR(20) NOT NULL,
    tipe_periode             VARCHAR(20) NOT NULL,
    tahun_buku               INT NOT NULL,
    bulan                    INT,
    triwulan                 INT,
    tanggal_mulai            DATE NOT NULL,
    tanggal_akhir            DATE NOT NULL,
    status_periode           VARCHAR(20) NOT NULL DEFAULT 'OPEN',
    tanggal_soft_close       TIMESTAMPTZ,
    tanggal_hard_close       TIMESTAMPTZ,
    user_closer_id           UUID REFERENCES sec.user(id),
    user_approver_close_id   UUID REFERENCES sec.user(id),
    catatan_closing          TEXT,
    reopened_flag            BOOLEAN NOT NULL DEFAULT FALSE,
    reopened_reason          TEXT,
    reopened_at              TIMESTAMPTZ,
    reopened_by              UUID REFERENCES sec.user(id),
    reopened_approved_by     UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_periode_kode UNIQUE (periode_id_kode),
    CONSTRAINT ck_periode_status CHECK (status_periode IN ('OPEN','SOFT_CLOSED','CLOSED')),
    CONSTRAINT ck_periode_tipe CHECK (tipe_periode IN ('BULANAN','TRIWULANAN','TAHUNAN'))
);
CREATE INDEX ix_periode_tahun_bulan ON mst.periode_buku(tahun_buku, bulan) WHERE tipe_periode='BULANAN';
CREATE INDEX ix_periode_status ON mst.periode_buku(status_periode);
CREATE INDEX ix_periode_tanggal ON mst.periode_buku(tanggal_mulai, tanggal_akhir);

-- 4.8 mst.mata_uang
CREATE TABLE mst.mata_uang (
    kode_mata_uang           CHAR(3) PRIMARY KEY,
    nama_mata_uang           VARCHAR(60) NOT NULL,
    simbol                   VARCHAR(5),
    sumber_kurs_default      VARCHAR(30) NOT NULL,
    frekuensi_update         VARCHAR(20) NOT NULL,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    tanggal_mulai_aktif      DATE NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.9 mst.kurs
CREATE TABLE mst.kurs (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    fx_rate_id_kode          VARCHAR(20) NOT NULL,
    kode_mata_uang           CHAR(3) NOT NULL REFERENCES mst.mata_uang(kode_mata_uang),
    tanggal_berlaku          DATE NOT NULL,
    kurs_beli                NUMERIC(15,4),
    kurs_jual                NUMERIC(15,4),
    kurs_tengah              NUMERIC(15,4) NOT NULL,
    sumber_kurs              VARCHAR(30) NOT NULL,
    periode_bulanan_id       UUID NOT NULL REFERENCES mst.periode_buku(id),
    locked_flag              BOOLEAN NOT NULL DEFAULT FALSE,
    maker_id                 UUID REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    dokumen_bukti_id         UUID,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_kurs_mata_uang_tanggal UNIQUE (kode_mata_uang, tanggal_berlaku)
);
CREATE INDEX ix_kurs_tanggal ON mst.kurs(tanggal_berlaku DESC);
CREATE INDEX ix_kurs_periode ON mst.kurs(periode_bulanan_id);
CREATE INDEX ix_kurs_lookup ON mst.kurs(kode_mata_uang, tanggal_berlaku DESC);

-- 4.10 mst.chart_of_accounts
CREATE TABLE mst.chart_of_accounts (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    kode_akun                VARCHAR(20) NOT NULL,
    nama_akun                VARCHAR(200) NOT NULL,
    tipe_akun                VARCHAR(20) NOT NULL,
    sub_tipe_akun            VARCHAR(30) NOT NULL,
    kategori_investasi       VARCHAR(20),
    mata_uang_native         CHAR(3) NOT NULL DEFAULT 'IDR',
    posisi_normal            VARCHAR(10) NOT NULL,
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    parent_akun_id           UUID REFERENCES mst.chart_of_accounts(id),
    sumber_coa               VARCHAR(30) NOT NULL,
    tanggal_mulai_aktif      DATE NOT NULL,
    created_by               UUID NOT NULL REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by               UUID REFERENCES sec.user(id),
    updated_at               TIMESTAMPTZ,
    version                  INT NOT NULL DEFAULT 1,
    CONSTRAINT uq_coa_kode UNIQUE (kode_akun),
    CONSTRAINT ck_coa_tipe CHECK (tipe_akun IN ('ASET','LIABILITAS','EKUITAS','PENDAPATAN','BEBAN','KONTINJEN')),
    CONSTRAINT ck_coa_posisi CHECK (posisi_normal IN ('DEBIT','KREDIT'))
);
CREATE INDEX ix_coa_tipe ON mst.chart_of_accounts(tipe_akun);
CREATE INDEX ix_coa_kategori ON mst.chart_of_accounts(kategori_investasi) WHERE kategori_investasi IS NOT NULL;
CREATE INDEX ix_coa_parent ON mst.chart_of_accounts(parent_akun_id) WHERE parent_akun_id IS NOT NULL;
CREATE INDEX ix_coa_aktif ON mst.chart_of_accounts(aktif_flag) WHERE aktif_flag=TRUE;

-- 4.11 mst.mapping_jurnal_header
CREATE TABLE mst.mapping_jurnal_header (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    event_id_kode            VARCHAR(40) NOT NULL,
    event_code               VARCHAR(40) NOT NULL,
    nama_event               VARCHAR(120) NOT NULL,
    kategori_event           VARCHAR(30) NOT NULL,
    trigger_source           VARCHAR(20) NOT NULL,
    tipe_instrumen_berlaku   VARCHAR(50)[],
    klasifikasi_berlaku      VARCHAR(20)[],
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    catatan                  TEXT,
    created_by               UUID NOT NULL REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by               UUID REFERENCES sec.user(id),
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_mapping_event_code UNIQUE (event_code),
    CONSTRAINT uq_mapping_event_id_kode UNIQUE (event_id_kode)
);
CREATE INDEX ix_mapping_event_aktif ON mst.mapping_jurnal_header(event_code) WHERE aktif_flag=TRUE;

-- 4.12 mst.mapping_jurnal_detail
CREATE TABLE mst.mapping_jurnal_detail (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    event_header_id          UUID NOT NULL REFERENCES mst.mapping_jurnal_header(id) ON DELETE CASCADE,
    urutan                   INT NOT NULL,
    kode_akun_id             UUID NOT NULL REFERENCES mst.chart_of_accounts(id),
    dk_indicator             VARCHAR(10) NOT NULL,
    sumber_amount            VARCHAR(50) NOT NULL,
    klasifikasi_filter       VARCHAR(20),
    tipe_instrumen_filter    VARCHAR(50)[],
    underlying_type_filter   VARCHAR(20),
    multiplier               NUMERIC(8,4) NOT NULL DEFAULT 1.0000,
    mata_uang_posting        CHAR(3) NOT NULL DEFAULT 'IDR',
    aktif_flag               BOOLEAN NOT NULL DEFAULT TRUE,
    catatan                  TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ,
    CONSTRAINT ck_mapping_dk CHECK (dk_indicator IN ('DEBIT','KREDIT'))
);
CREATE INDEX ix_mapping_detail_event ON mst.mapping_jurnal_detail(event_header_id, urutan);
CREATE INDEX ix_mapping_detail_aktif ON mst.mapping_jurnal_detail(event_header_id) WHERE aktif_flag=TRUE;
CREATE INDEX ix_mapping_detail_akun ON mst.mapping_jurnal_detail(kode_akun_id);

-- 4.13 mst.impact_mev_pd
CREATE TABLE mst.impact_mev_pd (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    periode_id               UUID NOT NULL REFERENCES mst.periode_buku(id),
    skenario                 VARCHAR(20) NOT NULL,
    impact_multiplier        NUMERIC(8,4) NOT NULL,
    mev_components_json      JSONB,
    catatan                  TEXT,
    dokumen_pendukung_id     UUID,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_impact_mev_periode_skenario UNIQUE (periode_id, skenario),
    CONSTRAINT ck_impact_skenario CHECK (skenario IN ('GOOD','BAD'))
);
CREATE INDEX ix_impact_mev_periode ON mst.impact_mev_pd(periode_id);

-- 4.14 mst.impact_pd
CREATE TABLE mst.impact_pd (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    periode_id               UUID NOT NULL REFERENCES mst.periode_buku(id),
    impact_multiplier        NUMERIC(8,4) NOT NULL DEFAULT 1.0000,
    catatan                  TEXT,
    dokumen_pendukung_id     UUID,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_impact_pd_periode UNIQUE (periode_id),
    CONSTRAINT ck_impact_pd_range CHECK (impact_multiplier BETWEEN 0.5 AND 2.0)
);

-- 4.15 mst.lps_coverage
CREATE TABLE mst.lps_coverage (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    coverage_amount          NUMERIC(20,2) NOT NULL DEFAULT 2000000000.00,
    mata_uang                CHAR(3) NOT NULL DEFAULT 'IDR',
    periode_berlaku_dari     DATE NOT NULL,
    periode_berlaku_sampai   DATE,
    regulasi_referensi       VARCHAR(200),
    dokumen_pendukung_id     UUID,
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_lps_current ON mst.lps_coverage(periode_berlaku_dari) WHERE periode_berlaku_sampai IS NULL;

-- 4.16 mst.instrumen (CORE TABLE)
CREATE TABLE mst.instrumen (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    kode_instrumen              VARCHAR(20) NOT NULL,
    tipe_instrumen              VARCHAR(30) NOT NULL,
    sub_tipe                    VARCHAR(50) NOT NULL,
    nama                        VARCHAR(200) NOT NULL,
    isin                        VARCHAR(20),
    counterparty_id             UUID NOT NULL REFERENCES mst.counterparty(id),
    manajer_investasi_id        UUID REFERENCES mst.counterparty(id),
    bank_kustodian_id           UUID REFERENCES mst.counterparty(id),
    mata_uang                   CHAR(3) NOT NULL DEFAULT 'IDR' REFERENCES mst.mata_uang(kode_mata_uang),
    nominal                     NUMERIC(20,2) NOT NULL,
    jumlah_lot                  NUMERIC(18,0),
    tanggal_penempatan          DATE NOT NULL,
    tanggal_jatuh_tempo         DATE,
    kupon                       NUMERIC(8,4),
    frekuensi_bunga             VARCHAR(20),
    auto_renewal_flag           BOOLEAN DEFAULT FALSE,
    fvoci_election              BOOLEAN DEFAULT FALSE,
    sppi_result                 VARCHAR(10),
    bm_category                 VARCHAR(10),
    klasifikasi_psak71          VARCHAR(20),
    klasifikasi_locked_at       TIMESTAMPTZ,
    klasifikasi_locked_by       UUID REFERENCES sec.user(id),
    sppi_bm_last_review_date    DATE,
    eir_awal                    NUMERIC(12,8),
    tanggal_eir_computed        DATE,
    premium_diskonto_awal       NUMERIC(20,2) DEFAULT 0,
    biaya_transaksi_capitalized NUMERIC(20,2) DEFAULT 0,
    eir_method_flag             BOOLEAN DEFAULT TRUE,
    day_count_convention        VARCHAR(10) DEFAULT 'ACT/365',
    amortization_frequency      VARCHAR(20),
    status                      VARCHAR(30) NOT NULL DEFAULT 'AKTIF',
    portofolio_id               UUID NOT NULL REFERENCES mst.portofolio(id),
    workflow_status             VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    created_by                  UUID NOT NULL REFERENCES sec.user(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by                  UUID REFERENCES sec.user(id),
    updated_at                  TIMESTAMPTZ,
    approved_by                 UUID REFERENCES sec.user(id),
    approved_at                 TIMESTAMPTZ,
    version                     INT NOT NULL DEFAULT 1,
    is_deleted                  BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT ck_jt_after_penempatan CHECK (tanggal_jatuh_tempo IS NULL OR tanggal_jatuh_tempo > tanggal_penempatan),
    CONSTRAINT ck_kupon_nonneg CHECK (kupon IS NULL OR kupon >= 0),
    CONSTRAINT ck_eir_range CHECK (eir_awal IS NULL OR (eir_awal >= 0 AND eir_awal < 1)),
    CONSTRAINT ck_klasifikasi CHECK (klasifikasi_psak71 IS NULL OR klasifikasi_psak71 IN ('AC','FVOCI','FVOCI_ELECTION','FVTPL'))
);
CREATE UNIQUE INDEX uq_instrumen_kode ON mst.instrumen(kode_instrumen) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_tipe ON mst.instrumen(tipe_instrumen) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_klasifikasi ON mst.instrumen(klasifikasi_psak71) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_counterparty ON mst.instrumen(counterparty_id) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_status ON mst.instrumen(status) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_isin ON mst.instrumen(isin) WHERE isin IS NOT NULL AND is_deleted=FALSE;
CREATE INDEX ix_instrumen_portofolio ON mst.instrumen(portofolio_id, status) WHERE is_deleted=FALSE;
CREATE INDEX ix_instrumen_jt ON mst.instrumen(tanggal_jatuh_tempo) WHERE status='AKTIF' AND tanggal_jatuh_tempo IS NOT NULL;

-- ====================================================================
-- 5. SCHEMA: doc (Document Management)
-- ====================================================================
\echo '  Creating doc schema tables...'

CREATE TABLE doc.upload (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    filename                 VARCHAR(255) NOT NULL,
    filename_storage         VARCHAR(500) NOT NULL,
    mime_type                VARCHAR(100) NOT NULL,
    file_size_bytes          BIGINT NOT NULL,
    sha256_hash              CHAR(64) NOT NULL,
    uploader_id              UUID NOT NULL REFERENCES sec.user(id),
    uploaded_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploader_ip              INET,
    virus_scan_result        VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    virus_scan_at            TIMESTAMPTZ,
    status                   VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    inactive_by              UUID REFERENCES sec.user(id),
    inactive_at              TIMESTAMPTZ,
    inactive_reason          TEXT,
    s3_kms_key_id            VARCHAR(100),
    s3_version_id            VARCHAR(100),
    CONSTRAINT ck_file_size CHECK (file_size_bytes <= 52428800),
    CONSTRAINT ck_doc_status CHECK (status IN ('ACTIVE','INACTIVE')),
    CONSTRAINT ck_virus_scan CHECK (virus_scan_result IN ('PENDING','CLEAN','INFECTED'))
);
CREATE INDEX ix_doc_upload_uploader ON doc.upload(uploader_id, uploaded_at DESC);
CREATE INDEX ix_doc_upload_status ON doc.upload(status) WHERE status='ACTIVE';
CREATE INDEX ix_doc_upload_hash ON doc.upload(sha256_hash);
CREATE INDEX ix_doc_upload_virus ON doc.upload(virus_scan_result) WHERE virus_scan_result IN ('PENDING','INFECTED');

CREATE TABLE doc.link (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    doc_upload_id            UUID NOT NULL REFERENCES doc.upload(id),
    entity_type              VARCHAR(50) NOT NULL,
    entity_id                UUID NOT NULL,
    link_type                VARCHAR(30) NOT NULL,
    linked_by                UUID NOT NULL REFERENCES sec.user(id),
    linked_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_doc_link UNIQUE (doc_upload_id, entity_type, entity_id, link_type)
);
CREATE INDEX ix_doc_link_doc ON doc.link(doc_upload_id);
CREATE INDEX ix_doc_link_entity ON doc.link(entity_type, entity_id);

CREATE TABLE doc.access_log (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    doc_upload_id            UUID NOT NULL REFERENCES doc.upload(id),
    accessed_by              UUID NOT NULL REFERENCES sec.user(id),
    accessed_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    access_type              VARCHAR(20) NOT NULL,
    ip_address               INET,
    user_agent               VARCHAR(500),
    pre_signed_url_id        VARCHAR(100),
    url_expires_at           TIMESTAMPTZ,
    CONSTRAINT ck_access_type CHECK (access_type IN ('VIEW','DOWNLOAD','PREVIEW'))
);
CREATE INDEX ix_doc_access_doc ON doc.access_log(doc_upload_id, accessed_at DESC);
CREATE INDEX ix_doc_access_user ON doc.access_log(accessed_by, accessed_at DESC);
CREATE INDEX ix_doc_access_time ON doc.access_log(accessed_at DESC);

-- ====================================================================
-- 6. SCHEMA: sppi (SPPI/BM/Klasifikasi/Reklasifikasi)
-- ====================================================================
\echo '  Creating sppi schema tables...'

CREATE TABLE sppi.sppi_test (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    sppi_test_id_kode        VARCHAR(20) NOT NULL,
    instrumen_id             UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_test             DATE NOT NULL,
    tipe_test                VARCHAR(20) NOT NULL,
    jawaban_checklist        JSONB NOT NULL,
    hasil_sppi               VARCHAR(10) NOT NULL,
    fail_indicator_reason    VARCHAR(500),
    catatan_penilaian        TEXT,
    dokumen_bukti_id         UUID REFERENCES doc.upload(id),
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    reviewer_id              UUID REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    status                   VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at              TIMESTAMPTZ,
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_sppi_test_kode UNIQUE (sppi_test_id_kode),
    CONSTRAINT ck_sppi_hasil CHECK (hasil_sppi IN ('PASS','FAIL'))
);
CREATE INDEX ix_sppi_instrumen_tanggal ON sppi.sppi_test(instrumen_id, tanggal_test DESC);
CREATE INDEX ix_sppi_status ON sppi.sppi_test(status) WHERE status NOT IN ('APPROVED','REJECTED');
CREATE INDEX ix_sppi_hasil ON sppi.sppi_test(hasil_sppi);

CREATE TABLE sppi.bm_test (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    bm_test_id_kode          VARCHAR(20) NOT NULL,
    portofolio_id            UUID NOT NULL REFERENCES mst.portofolio(id),
    tanggal_penilaian        DATE NOT NULL,
    tipe_test                VARCHAR(20) NOT NULL,
    tujuan_pengelolaan       TEXT NOT NULL,
    indikator_penilaian      JSONB NOT NULL,
    frekuensi_penjualan_12m  NUMERIC(8,4) NOT NULL,
    hasil_bm_test_suggested  VARCHAR(10) NOT NULL,
    hasil_bm_test_final      VARCHAR(10) NOT NULL,
    override_flag            BOOLEAN NOT NULL DEFAULT FALSE,
    justifikasi_override     TEXT,
    dokumen_bukti_id         UUID REFERENCES doc.upload(id),
    approver_id              UUID REFERENCES sec.user(id),
    periode_berlaku_dari     DATE NOT NULL,
    periode_berlaku_sampai   DATE NOT NULL,
    status                   VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_bm_test_kode UNIQUE (bm_test_id_kode),
    CONSTRAINT ck_bm_test_hasil CHECK (hasil_bm_test_final IN ('HTC','HTCS','OTHER'))
);
CREATE INDEX ix_bm_portofolio_tanggal ON sppi.bm_test(portofolio_id, tanggal_penilaian DESC);

CREATE TABLE sppi.reklasifikasi_log (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    reklas_id_kode              VARCHAR(20) NOT NULL,
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    klasifikasi_dari            VARCHAR(20) NOT NULL,
    klasifikasi_ke              VARCHAR(20) NOT NULL,
    tanggal_efektif             DATE NOT NULL,
    fair_value_tanggal_efektif  NUMERIC(20,2) NOT NULL,
    carrying_amount_dari        NUMERIC(20,2) NOT NULL,
    accumulated_oci_dari        NUMERIC(20,2) DEFAULT 0,
    eir_dari                    NUMERIC(12,8),
    eir_ke                      NUMERIC(12,8),
    justifikasi                 TEXT NOT NULL,
    dokumen_bukti_id            UUID REFERENCES doc.upload(id),
    maker_id                    UUID NOT NULL REFERENCES sec.user(id),
    approver_id                 UUID REFERENCES sec.user(id),
    jurnal_header_id            UUID,  -- FK setelah jrnl.header dibuat
    status                      VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at                 TIMESTAMPTZ,
    CONSTRAINT uq_reklas_kode UNIQUE (reklas_id_kode)
);
CREATE INDEX ix_reklas_instrumen ON sppi.reklasifikasi_log(instrumen_id, tanggal_efektif DESC);
CREATE INDEX ix_reklas_status ON sppi.reklasifikasi_log(status);

CREATE TABLE sppi.klasifikasi_history (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    instrumen_id             UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_efektif          DATE NOT NULL,
    klasifikasi              VARCHAR(20) NOT NULL,
    sppi_test_id             UUID REFERENCES sppi.sppi_test(id),
    bm_test_id               UUID REFERENCES sppi.bm_test(id),
    reklasifikasi_log_id     UUID REFERENCES sppi.reklasifikasi_log(id),
    alasan                   TEXT NOT NULL,
    approved_by              UUID NOT NULL REFERENCES sec.user(id),
    approved_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    periode_berakhir         DATE
);
CREATE INDEX ix_klasifikasi_instrumen ON sppi.klasifikasi_history(instrumen_id, tanggal_efektif DESC);
CREATE INDEX ix_klasifikasi_aktif ON sppi.klasifikasi_history(instrumen_id) WHERE periode_berakhir IS NULL;

-- ====================================================================
-- 7. SCHEMA: jrnl (Jurnal & GL Interface) — created before trx (referenced)
-- ====================================================================
\echo '  Creating jrnl schema tables...'

CREATE TABLE jrnl.header (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    no_jurnal                VARCHAR(20) NOT NULL,
    tanggal_posting          DATE NOT NULL,
    periode_id               UUID NOT NULL REFERENCES mst.periode_buku(id),
    event_code               VARCHAR(40) NOT NULL,
    mapping_header_id        UUID REFERENCES mst.mapping_jurnal_header(id),
    instrumen_id             UUID REFERENCES mst.instrumen(id),
    reference_event_type     VARCHAR(50) NOT NULL,
    reference_event_id       UUID NOT NULL,
    currency                 CHAR(3) NOT NULL DEFAULT 'IDR',
    total_debit              NUMERIC(20,2) NOT NULL,
    total_kredit             NUMERIC(20,2) NOT NULL,
    narrative                VARCHAR(500),
    status_internal          VARCHAR(20) NOT NULL DEFAULT 'POSTED',
    reversed_by_jurnal_id    UUID REFERENCES jrnl.header(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by               UUID REFERENCES sec.user(id),
    idempotency_key          VARCHAR(100),
    CONSTRAINT uq_jrnl_no UNIQUE (no_jurnal),
    CONSTRAINT ck_jrnl_balance CHECK (total_debit = total_kredit)
);
CREATE INDEX ix_jrnl_periode ON jrnl.header(periode_id);
CREATE INDEX ix_jrnl_event ON jrnl.header(event_code, tanggal_posting);
CREATE INDEX ix_jrnl_reference ON jrnl.header(reference_event_type, reference_event_id);
CREATE INDEX ix_jrnl_tanggal ON jrnl.header(tanggal_posting DESC);
CREATE UNIQUE INDEX uq_jrnl_idempotency ON jrnl.header(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE jrnl.detail (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    header_id                UUID NOT NULL REFERENCES jrnl.header(id) ON DELETE CASCADE,
    urutan                   INT NOT NULL,
    kode_akun_id             UUID NOT NULL REFERENCES mst.chart_of_accounts(id),
    debit_amount             NUMERIC(20,2) NOT NULL DEFAULT 0,
    kredit_amount            NUMERIC(20,2) NOT NULL DEFAULT 0,
    mata_uang                CHAR(3) NOT NULL DEFAULT 'IDR',
    narrative_line           VARCHAR(500),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_dk_exclusive CHECK ((debit_amount > 0 AND kredit_amount = 0) OR (debit_amount = 0 AND kredit_amount > 0))
);
CREATE INDEX ix_jrnl_detail_header ON jrnl.detail(header_id, urutan);
CREATE INDEX ix_jrnl_detail_akun ON jrnl.detail(kode_akun_id);

CREATE TABLE jrnl.gl_status (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    header_id                UUID NOT NULL REFERENCES jrnl.header(id) ON DELETE CASCADE,
    gl_host_status           VARCHAR(20) NOT NULL DEFAULT 'PENDING_DELIVERY',
    gl_host_journal_id       VARCHAR(50),
    delivered_at             TIMESTAMPTZ,
    retry_count              INT NOT NULL DEFAULT 0,
    last_retry_at            TIMESTAMPTZ,
    last_error               TEXT,
    delivery_mode            VARCHAR(20) NOT NULL DEFAULT 'API',
    batch_file_id            VARCHAR(100),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ,
    CONSTRAINT uq_gl_status_header UNIQUE (header_id)
);
CREATE INDEX ix_gl_status_pending ON jrnl.gl_status(gl_host_status) WHERE gl_host_status IN ('PENDING_DELIVERY','RETRYING','FAILED');
CREATE INDEX ix_gl_status_dlq ON jrnl.gl_status(gl_host_status) WHERE gl_host_status='DEAD_LETTER';

-- ====================================================================
-- 8. SCHEMA: trx (Transactional Data)
-- ====================================================================
\echo '  Creating trx schema tables...'

CREATE TABLE trx.penempatan (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    no_transaksi                VARCHAR(20) NOT NULL,
    tanggal_transaksi           DATE NOT NULL,
    tanggal_settlement          DATE NOT NULL,
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    periode_id                  UUID NOT NULL REFERENCES mst.periode_buku(id),
    nominal                     NUMERIC(20,2) NOT NULL,
    harga_beli                  NUMERIC(15,4),
    jumlah_unit                 NUMERIC(18,4),
    accrued_interest_dibeli     NUMERIC(20,2) DEFAULT 0,
    total_pembayaran            NUMERIC(20,2) NOT NULL,
    biaya_transaksi             NUMERIC(20,2) DEFAULT 0,
    akun_sumber_dana_id         UUID NOT NULL REFERENCES mst.instrumen(id),
    mata_uang                   CHAR(3) NOT NULL DEFAULT 'IDR',
    kurs_tengah_bi              NUMERIC(15,4),
    total_pembayaran_idr        NUMERIC(20,2) NOT NULL,
    eir_awal                    NUMERIC(12,8),
    carrying_amount_awal        NUMERIC(20,2),
    maker_id                    UUID NOT NULL REFERENCES sec.user(id),
    approver_id                 UUID REFERENCES sec.user(id),
    workflow_status             VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    jurnal_header_id            UUID REFERENCES jrnl.header(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at                 TIMESTAMPTZ,
    is_deleted                  BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT uq_penempatan_no UNIQUE (no_transaksi),
    CONSTRAINT ck_settlement_after_trade CHECK (tanggal_settlement >= tanggal_transaksi),
    CONSTRAINT ck_total_positive CHECK (total_pembayaran > 0)
);
CREATE INDEX ix_penempatan_instrumen ON trx.penempatan(instrumen_id);
CREATE INDEX ix_penempatan_periode ON trx.penempatan(periode_id);
CREATE INDEX ix_penempatan_tanggal ON trx.penempatan(tanggal_transaksi);
CREATE INDEX ix_penempatan_status ON trx.penempatan(workflow_status) WHERE is_deleted=FALSE;

CREATE TABLE trx.mtm (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_valuasi             DATE NOT NULL,
    periode_id                  UUID NOT NULL REFERENCES mst.periode_buku(id),
    carrying_amount_sebelumnya  NUMERIC(20,2) NOT NULL,
    harga_referensi_baru        NUMERIC(15,4) NOT NULL,
    fair_value_baru             NUMERIC(20,2) NOT NULL,
    selisih_mtm_native          NUMERIC(20,2) NOT NULL,
    selisih_mtm_idr             NUMERIC(20,2) NOT NULL,
    kurs_tengah_bi              NUMERIC(15,4),
    akun_pengakuan              VARCHAR(20) NOT NULL,
    sumber_harga                VARCHAR(30) NOT NULL,
    dokumen_sumber_id           UUID REFERENCES doc.upload(id),
    jurnal_header_id            UUID REFERENCES jrnl.header(id),
    status_flag                 VARCHAR(20) NOT NULL DEFAULT 'POSTED',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_mtm_instrumen_tanggal UNIQUE (instrumen_id, tanggal_valuasi)
);
CREATE INDEX ix_mtm_periode ON trx.mtm(periode_id);
CREATE INDEX ix_mtm_tanggal ON trx.mtm(tanggal_valuasi);
CREATE INDEX ix_mtm_stale ON trx.mtm(status_flag) WHERE status_flag='STALE_PRICE';

CREATE TABLE trx.renewal (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    no_renewal               VARCHAR(20) NOT NULL,
    instrumen_lama_id        UUID NOT NULL REFERENCES mst.instrumen(id),
    instrumen_baru_id        UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_jt_lama          DATE NOT NULL,
    skema_renewal            VARCHAR(30) NOT NULL,
    tenor_baru_hari          INT,
    tenor_baru_bulan         INT,
    suku_bunga_baru          NUMERIC(8,4) NOT NULL,
    pokok_lama               NUMERIC(20,2) NOT NULL,
    bunga_akrual_terakhir    NUMERIC(20,2) NOT NULL,
    pph_bunga                NUMERIC(20,2) NOT NULL,
    bunga_net                NUMERIC(20,2) NOT NULL,
    pokok_baru               NUMERIC(20,2) NOT NULL,
    dokumen_bukti_id         UUID REFERENCES doc.upload(id),
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    workflow_status          VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    jurnal_header_id         UUID REFERENCES jrnl.header(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_renewal_no UNIQUE (no_renewal),
    CONSTRAINT ck_renewal_skema CHECK (skema_renewal IN ('POKOK_SAJA','POKOK_PLUS_BUNGA'))
);
CREATE INDEX ix_renewal_lama ON trx.renewal(instrumen_lama_id);
CREATE INDEX ix_renewal_baru ON trx.renewal(instrumen_baru_id);

CREATE TABLE trx.penjualan (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    no_penjualan                VARCHAR(20) NOT NULL,
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    periode_id                  UUID NOT NULL REFERENCES mst.periode_buku(id),
    tanggal_penjualan           DATE NOT NULL,
    tanggal_settlement          DATE NOT NULL,
    nominal_unit_dijual         NUMERIC(20,4) NOT NULL,
    harga_jual                  NUMERIC(15,4) NOT NULL,
    accrued_interest_dijual     NUMERIC(20,2) DEFAULT 0,
    total_penerimaan            NUMERIC(20,2) NOT NULL,
    biaya_transaksi             NUMERIC(20,2) DEFAULT 0,
    carrying_amount_saat_jual   NUMERIC(20,2) NOT NULL,
    realized_gain_loss          NUMERIC(20,2) NOT NULL,
    realized_oci_recycled       NUMERIC(20,2),
    dijual_penuh_flag           BOOLEAN NOT NULL,
    dokumen_bukti_id            UUID REFERENCES doc.upload(id),
    maker_id                    UUID NOT NULL REFERENCES sec.user(id),
    approver_id                 UUID REFERENCES sec.user(id),
    workflow_status             VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    jurnal_header_id            UUID REFERENCES jrnl.header(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at                 TIMESTAMPTZ,
    CONSTRAINT uq_penjualan_no UNIQUE (no_penjualan)
);
CREATE INDEX ix_penjualan_instrumen ON trx.penjualan(instrumen_id);
CREATE INDEX ix_penjualan_periode ON trx.penjualan(periode_id);

CREATE TABLE trx.jatuh_tempo (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    no_jatuh_tempo           VARCHAR(20) NOT NULL,
    instrumen_id             UUID NOT NULL REFERENCES mst.instrumen(id),
    periode_id               UUID NOT NULL REFERENCES mst.periode_buku(id),
    tanggal_jt               DATE NOT NULL,
    pokok_diterima           NUMERIC(20,2) NOT NULL,
    kupon_final              NUMERIC(20,2) DEFAULT 0,
    pph_kupon                NUMERIC(20,2) DEFAULT 0,
    total_diterima           NUMERIC(20,2) NOT NULL,
    realized_oci_recycled    NUMERIC(20,2),
    dokumen_bukti_id         UUID REFERENCES doc.upload(id),
    jurnal_header_id         UUID REFERENCES jrnl.header(id),
    status                   VARCHAR(20) NOT NULL DEFAULT 'COMPLETED',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_jt_no UNIQUE (no_jatuh_tempo)
);
CREATE INDEX ix_jt_instrumen ON trx.jatuh_tempo(instrumen_id);
CREATE INDEX ix_jt_tanggal ON trx.jatuh_tempo(tanggal_jt);

CREATE TABLE trx.pendapatan_akrual (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_akrual              DATE NOT NULL,
    periode_id                  UUID NOT NULL REFERENCES mst.periode_buku(id),
    carrying_amount             NUMERIC(20,2) NOT NULL,
    eir                         NUMERIC(12,8),
    kupon_kontraktual_harian    NUMERIC(20,2) NOT NULL,
    pendapatan_bunga_eir_harian NUMERIC(20,2) NOT NULL,
    amortisasi_p_d_harian       NUMERIC(20,2) DEFAULT 0,
    kurs_tengah_bi              NUMERIC(15,4),
    pendapatan_bunga_idr        NUMERIC(20,2) NOT NULL,
    amortisasi_p_d_idr          NUMERIC(20,2) DEFAULT 0,
    fx_unrealized_idr           NUMERIC(20,2) DEFAULT 0,
    stage_saat_akrual           VARCHAR(20) NOT NULL,
    jurnal_header_id            UUID REFERENCES jrnl.header(id),
    status                      VARCHAR(20) NOT NULL DEFAULT 'POSTED',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_akrual_instrumen_tanggal UNIQUE (instrumen_id, tanggal_akrual)
);
CREATE INDEX ix_akrual_periode ON trx.pendapatan_akrual(periode_id);
CREATE INDEX ix_akrual_tanggal ON trx.pendapatan_akrual(tanggal_akrual DESC);
CREATE INDEX ix_akrual_stage ON trx.pendapatan_akrual(stage_saat_akrual, tanggal_akrual);

-- ====================================================================
-- 9. SCHEMA: ecl (ECL/EIR Compliance)
-- ====================================================================
\echo '  Creating ecl schema tables...'

CREATE TABLE ecl.calc_header (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    calc_id_kode                VARCHAR(20) NOT NULL,
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    periode_id                  UUID NOT NULL REFERENCES mst.periode_buku(id),
    evaluation_date             DATE NOT NULL,
    stage                       VARCHAR(10) NOT NULL,
    pd_horizon                  VARCHAR(10) NOT NULL,
    ead_native                  NUMERIC(20,2) NOT NULL,
    ead_idr                     NUMERIC(20,2) NOT NULL,
    kurs_tengah_bi              NUMERIC(15,4),
    lgd                         NUMERIC(8,4) NOT NULL,
    pd_normal                   NUMERIC(8,4) NOT NULL,
    impact_mev_good             NUMERIC(8,4) NOT NULL,
    impact_mev_bad              NUMERIC(8,4) NOT NULL,
    impact_pd                   NUMERIC(8,4) NOT NULL,
    w_good                      NUMERIC(8,4) NOT NULL DEFAULT 0.2500,
    w_normal                    NUMERIC(8,4) NOT NULL DEFAULT 0.5000,
    w_bad                       NUMERIC(8,4) NOT NULL DEFAULT 0.2500,
    ecl_weighted_idr            NUMERIC(20,2) NOT NULL,
    ecl_fl_idr                  NUMERIC(20,2) NOT NULL,
    delta_ecl_fl_idr            NUMERIC(20,2),
    pengakuan_lk                VARCHAR(20) NOT NULL,
    parameter_snapshot_id       UUID NOT NULL,
    jurnal_header_id            UUID REFERENCES jrnl.header(id),
    calc_run_id                 UUID NOT NULL REFERENCES sys.job_run_history(id),
    status                      VARCHAR(20) NOT NULL DEFAULT 'POSTED',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_ecl_periode_instrumen UNIQUE (periode_id, instrumen_id, calc_run_id),
    CONSTRAINT ck_bobot_sum CHECK (w_good + w_normal + w_bad BETWEEN 0.9999 AND 1.0001),
    CONSTRAINT ck_stage CHECK (stage IN ('STAGE_1','STAGE_2','STAGE_3'))
);
CREATE INDEX ix_ecl_calc_periode ON ecl.calc_header(periode_id);
CREATE INDEX ix_ecl_calc_instrumen ON ecl.calc_header(instrumen_id);
CREATE INDEX ix_ecl_calc_eval_date ON ecl.calc_header(evaluation_date);
CREATE INDEX ix_ecl_calc_stage ON ecl.calc_header(stage, periode_id);

CREATE TABLE ecl.calc_detail_skenario (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    ecl_calc_header_id       UUID NOT NULL REFERENCES ecl.calc_header(id) ON DELETE CASCADE,
    skenario                 VARCHAR(20) NOT NULL,
    pd_skenario              NUMERIC(8,4) NOT NULL,
    bobot                    NUMERIC(8,4) NOT NULL,
    ecl_skenario_idr         NUMERIC(20,2) NOT NULL,
    CONSTRAINT uq_ecl_detail UNIQUE (ecl_calc_header_id, skenario),
    CONSTRAINT ck_skenario_detail CHECK (skenario IN ('GOOD','NORMAL','BAD'))
);
CREATE INDEX ix_ecl_detail_header ON ecl.calc_detail_skenario(ecl_calc_header_id);

CREATE TABLE ecl.lookthrough_underlying (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    ecl_calc_header_id          UUID NOT NULL REFERENCES ecl.calc_header(id) ON DELETE CASCADE,
    underlying_kategori         VARCHAR(50) NOT NULL,
    underlying_issuer_or_rating VARCHAR(100),
    weight                      NUMERIC(8,4) NOT NULL,
    ead_underlying_idr          NUMERIC(20,2) NOT NULL,
    pd_normal                   NUMERIC(8,4),
    lgd                         NUMERIC(8,4),
    ecl_weighted_idr            NUMERIC(20,2) NOT NULL DEFAULT 0,
    excluded                    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX ix_lookthrough_header ON ecl.lookthrough_underlying(ecl_calc_header_id);
CREATE INDEX ix_lookthrough_kategori ON ecl.lookthrough_underlying(underlying_kategori);

CREATE TABLE ecl.stage_history (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    stage_history_id_kode    VARCHAR(20) NOT NULL,
    instrumen_id             UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_migrasi          DATE NOT NULL,
    stage_sebelum            VARCHAR(10) NOT NULL,
    stage_sesudah            VARCHAR(10) NOT NULL,
    trigger_type             VARCHAR(30) NOT NULL,
    detail_trigger           TEXT,
    rating_saat_migrasi      VARCHAR(8),
    dpd                      INT,
    delta_ecl_idr            NUMERIC(20,2),
    user_approver_id         UUID REFERENCES sec.user(id),
    status_approval          VARCHAR(30) NOT NULL DEFAULT 'AUTO',
    dokumen_pendukung_id     UUID REFERENCES doc.upload(id),
    jurnal_header_id         UUID REFERENCES jrnl.header(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_stage_instrumen ON ecl.stage_history(instrumen_id, tanggal_migrasi DESC);
CREATE INDEX ix_stage_trigger ON ecl.stage_history(trigger_type);
CREATE INDEX ix_stage_sesudah ON ecl.stage_history(stage_sesudah);

CREATE TABLE ecl.eir_amortization_schedule (
    id                          UUID PRIMARY KEY DEFAULT uuidv7(),
    schedule_id_kode            VARCHAR(30) NOT NULL,
    instrumen_id                UUID NOT NULL REFERENCES mst.instrumen(id),
    periode_seq                 INT NOT NULL,
    tanggal_posting             DATE NOT NULL,
    opening_carrying            NUMERIC(20,2) NOT NULL,
    cash_inflow                 NUMERIC(20,2) NOT NULL,
    pendapatan_bunga_eir        NUMERIC(20,2) NOT NULL,
    amortisasi_p_d              NUMERIC(20,2) NOT NULL,
    pelunasan_pokok             NUMERIC(20,2) NOT NULL DEFAULT 0,
    closing_carrying            NUMERIC(20,2) NOT NULL,
    eir_periode                 NUMERIC(12,8) NOT NULL,
    stage_saat_posting          VARCHAR(10) DEFAULT 'STAGE_1',
    status_posting              VARCHAR(20) NOT NULL DEFAULT 'PROYEKSI',
    jurnal_reference_id         UUID REFERENCES jrnl.header(id),
    recomputed_from_seq         INT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_status_posting CHECK (status_posting IN ('PROYEKSI','POSTED','REVERSED','RECOMPUTED'))
);
CREATE UNIQUE INDEX uq_schedule_active ON ecl.eir_amortization_schedule(instrumen_id, periode_seq) WHERE recomputed_from_seq IS NULL;
CREATE INDEX ix_schedule_instrumen ON ecl.eir_amortization_schedule(instrumen_id, periode_seq);
CREATE INDEX ix_schedule_tanggal ON ecl.eir_amortization_schedule(tanggal_posting);
CREATE INDEX ix_schedule_status ON ecl.eir_amortization_schedule(status_posting, tanggal_posting);

CREATE TABLE ecl.eir_reestimation_log (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    log_id_kode              VARCHAR(30) NOT NULL,
    instrumen_id             UUID NOT NULL REFERENCES mst.instrumen(id),
    tanggal_re_estimation    DATE NOT NULL,
    trigger_type             VARCHAR(50) NOT NULL,
    eir_sebelum              NUMERIC(12,8) NOT NULL,
    eir_sesudah              NUMERIC(12,8) NOT NULL,
    carrying_sebelum         NUMERIC(20,2) NOT NULL,
    carrying_sesudah         NUMERIC(20,2) NOT NULL,
    catch_up_adjustment      NUMERIC(20,2) DEFAULT 0,
    modifikasi_terms_json    JSONB,
    dokumen_pendukung_id     UUID REFERENCES doc.upload(id),
    maker_id                 UUID NOT NULL REFERENCES sec.user(id),
    reviewer_id              UUID REFERENCES sec.user(id),
    approver_id              UUID REFERENCES sec.user(id),
    workflow_status          VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    jurnal_header_id         UUID REFERENCES jrnl.header(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at              TIMESTAMPTZ,
    CONSTRAINT uq_eir_log_kode UNIQUE (log_id_kode)
);
CREATE INDEX ix_eir_log_instrumen ON ecl.eir_reestimation_log(instrumen_id, tanggal_re_estimation DESC);
CREATE INDEX ix_eir_log_trigger ON ecl.eir_reestimation_log(trigger_type);

CREATE TABLE trx.amortisasi (
    id                              UUID PRIMARY KEY DEFAULT uuidv7(),
    instrumen_id                    UUID NOT NULL REFERENCES mst.instrumen(id),
    schedule_periode_id             UUID NOT NULL REFERENCES ecl.eir_amortization_schedule(id),
    periode_id                      UUID NOT NULL REFERENCES mst.periode_buku(id),
    tanggal_posting                 DATE NOT NULL,
    amortisasi_premium_diskonto_idr NUMERIC(20,2) NOT NULL,
    jurnal_header_id                UUID REFERENCES jrnl.header(id),
    status                          VARCHAR(20) NOT NULL DEFAULT 'POSTED',
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_amortisasi_instrumen ON trx.amortisasi(instrumen_id, tanggal_posting);
CREATE INDEX ix_amortisasi_periode ON trx.amortisasi(periode_id);

-- ====================================================================

-- ====================================================================
-- 9.5 ALTER TABLE: Add FK constraints from sys.upload_batch_row to mst/trx (NEW v1.2)
-- (Placed here after mst.instrumen and trx.mtm are created)
-- ====================================================================
ALTER TABLE sys.upload_batch ADD CONSTRAINT fk_upload_batch_portofolio
    FOREIGN KEY (portofolio_target_id) REFERENCES mst.portofolio(id);

ALTER TABLE sys.upload_batch_row ADD CONSTRAINT fk_batch_row_instrumen
    FOREIGN KEY (instrumen_id) REFERENCES mst.instrumen(id);

ALTER TABLE sys.upload_batch_row ADD CONSTRAINT fk_batch_row_committed_instrumen
    FOREIGN KEY (committed_to_instrumen_id) REFERENCES mst.instrumen(id);

ALTER TABLE sys.upload_batch_row ADD CONSTRAINT fk_batch_row_committed_mtm
    FOREIGN KEY (committed_to_mtm_id) REFERENCES trx.mtm(id);


-- 10. SCHEMA: aud (Audit Trail)
-- ====================================================================
\echo '  Creating aud schema tables...'

CREATE TABLE aud.audit_log (
    id                       UUID NOT NULL DEFAULT uuidv7(),
    entity_type              VARCHAR(50) NOT NULL,
    entity_id                UUID NOT NULL,
    action                   VARCHAR(30) NOT NULL,
    actor_user_id            UUID,
    actor_role               VARCHAR(50),
    timestamp                TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_address               INET,
    user_agent               VARCHAR(500),
    before_value             JSONB,
    after_value              JSONB,
    changed_columns          VARCHAR[],
    metadata                 JSONB,
    session_id               VARCHAR(100),
    request_id               VARCHAR(100),
    hash_chain_prev          CHAR(64),
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

CREATE INDEX ix_audit_entity ON aud.audit_log(entity_type, entity_id, timestamp DESC);
CREATE INDEX ix_audit_actor ON aud.audit_log(actor_user_id, timestamp DESC);
CREATE INDEX ix_audit_action ON aud.audit_log(action, timestamp DESC);

-- Partition example untuk 2026
CREATE TABLE aud.audit_log_y2026m01 PARTITION OF aud.audit_log
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE aud.audit_log_y2026m02 PARTITION OF aud.audit_log
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
-- Add more partitions as needed via maintenance job

CREATE TABLE aud.workflow_history (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    entity_type              VARCHAR(50) NOT NULL,
    entity_id                UUID NOT NULL,
    state_from               VARCHAR(30),
    state_to                 VARCHAR(30) NOT NULL,
    actor_user_id            UUID NOT NULL REFERENCES sec.user(id),
    actor_role               VARCHAR(50),
    action_type              VARCHAR(30) NOT NULL,
    comment                  TEXT,
    timestamp                TIMESTAMPTZ NOT NULL DEFAULT now(),
    sla_deadline             TIMESTAMPTZ,
    sla_status               VARCHAR(20)
);
CREATE INDEX ix_workflow_entity ON aud.workflow_history(entity_type, entity_id, timestamp DESC);
CREATE INDEX ix_workflow_actor ON aud.workflow_history(actor_user_id, timestamp DESC);

CREATE TABLE aud.login_history (
    id                       UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id                  UUID REFERENCES sec.user(id),
    username_attempted       VARCHAR(100) NOT NULL,
    login_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    logout_at                TIMESTAMPTZ,
    session_id               VARCHAR(100),
    ip_address               INET,
    user_agent               VARCHAR(500),
    status                   VARCHAR(20) NOT NULL,
    failure_reason           VARCHAR(100),
    mfa_used                 BOOLEAN NOT NULL DEFAULT FALSE,
    geo_country              VARCHAR(2)
);
CREATE INDEX ix_login_user_time ON aud.login_history(user_id, login_at DESC);
CREATE INDEX ix_login_status ON aud.login_history(status, login_at DESC);
CREATE INDEX ix_login_ip ON aud.login_history(ip_address, login_at DESC);

-- ====================================================================
-- 11. TRIGGERS
-- ====================================================================
\echo '  Creating triggers...'

-- Auto-update updated_at
CREATE OR REPLACE FUNCTION fn_update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Audit log immutability
CREATE OR REPLACE FUNCTION fn_audit_no_modify()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit log records are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_audit_log_no_update
    BEFORE UPDATE OR DELETE ON aud.audit_log
    FOR EACH ROW EXECUTE FUNCTION fn_audit_no_modify();

-- Klasifikasi lock enforcement
CREATE OR REPLACE FUNCTION fn_instrumen_klasifikasi_lock()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.klasifikasi_locked_at IS NOT NULL
       AND NEW.klasifikasi_psak71 IS DISTINCT FROM OLD.klasifikasi_psak71 THEN
        RAISE EXCEPTION 'Klasifikasi PSAK 71 already locked. Use Reklasifikasi flow.';
    END IF;
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_instrumen_klasifikasi_lock
    BEFORE UPDATE ON mst.instrumen
    FOR EACH ROW EXECUTE FUNCTION fn_instrumen_klasifikasi_lock();

-- Auto-update for various tables
CREATE TRIGGER tg_portofolio_updated_at BEFORE UPDATE ON mst.portofolio FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_counterparty_updated_at BEFORE UPDATE ON mst.counterparty FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_periode_buku_updated_at BEFORE UPDATE ON mst.periode_buku FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_coa_updated_at BEFORE UPDATE ON mst.chart_of_accounts FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_mapping_header_updated_at BEFORE UPDATE ON mst.mapping_jurnal_header FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_mapping_detail_updated_at BEFORE UPDATE ON mst.mapping_jurnal_detail FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();
CREATE TRIGGER tg_kurs_updated_at BEFORE UPDATE ON mst.kurs FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

-- Kurs lock when periode CLOSED
CREATE OR REPLACE FUNCTION fn_kurs_no_modify_when_locked()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.locked_flag = TRUE THEN
        RAISE EXCEPTION 'Kurs is locked because periode is CLOSED. Use prior-period adjustment.';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_kurs_locked_check BEFORE UPDATE OR DELETE ON mst.kurs
    FOR EACH ROW EXECUTE FUNCTION fn_kurs_no_modify_when_locked();

-- ====================================================================
-- 12. SEED DATA
-- ====================================================================
\echo '  Inserting seed data...'

-- 12.1 Initial system user (bootstrap)
INSERT INTO sec.user (id, username, email, full_name, status, created_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'system', 'system@blips.tugu-re.com', 'System User', 'AKTIF', now());

INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES ('00000000-0000-0000-0000-000000000002', 'admin@tugu-re.com', 'admin@tugu-re.com', 'IT Admin Bootstrap', 'AKTIF', now(),
        '00000000-0000-0000-0000-000000000001');

-- 12.2 Roles
INSERT INTO sec.role (role_code, nama_role, deskripsi) VALUES
('ROLE-MAKER-TR', 'Treasury Maker', 'Input transaksi treasury'),
('ROLE-APPR-TR', 'Treasury Approver', 'Approve transaksi maker'),
('ROLE-RISK', 'Risk Officer', 'Master parameter risiko & ECL review'),
('ROLE-AKUN', 'Akuntansi', 'Posting jurnal & periode buku'),
('ROLE-AKUN-CTL', 'Finance Controller', 'Approve adjustment & soft-close'),
('ROLE-CFO', 'CFO', 'Hard-close approver & critical override'),
('ROLE-AUDIT', 'Auditor (Read-Only)', 'Audit trail & dokumen view'),
('ROLE-IT-ADMIN', 'IT Admin', 'User management'),
('ROLE-KOMITE', 'Komite Investasi', 'Approve klasifikasi PSAK 71'),
('ROLE-ALCO', 'ALCO Member', 'Approve ECL parameter');

-- 12.3 LGD Basel
INSERT INTO mst.lgd_basel (tipe_eksposur, lgd, karakteristik, periode_berlaku_dari, sumber, maker_id) VALUES
('SOVEREIGN', 0.4500, 'SUN, SBN, Obligasi Pemerintah', '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SENIOR_SECURED', 0.2500, 'Obligasi dengan jaminan aktiva spesifik', '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SENIOR_UNSECURED', 0.4500, 'Cash bank, deposito, obligasi korporasi tanpa jaminan', '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SUBORDINATED', 0.7500, 'Obligasi/sukuk subordinasi', '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001');

-- 12.4 Bobot Skenario
INSERT INTO mst.bobot_skenario (skenario, bobot, periode_berlaku_dari, maker_id) VALUES
('GOOD', 0.2500, '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('NORMAL', 0.5000, '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('BAD', 0.2500, '2026-01-01', '00000000-0000-0000-0000-000000000001');

-- 12.5 LPS Coverage
INSERT INTO mst.lps_coverage (coverage_amount, periode_berlaku_dari, regulasi_referensi, maker_id) VALUES
(2000000000.00, '2026-01-01', 'POJK No. 03/POJK.05/2017 tentang LPS', '00000000-0000-0000-0000-000000000001');

-- 12.6 Mata Uang
INSERT INTO mst.mata_uang (kode_mata_uang, nama_mata_uang, simbol, sumber_kurs_default, frekuensi_update, tanggal_mulai_aktif) VALUES
('IDR', 'Rupiah Indonesia', 'Rp', 'INTERNAL', 'HARIAN', '2026-01-01'),
('USD', 'US Dollar', '$', 'BI_JISDOR', 'HARIAN', '2026-01-01'),
('SGD', 'Singapore Dollar', 'S$', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01'),
('EUR', 'Euro', '€', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01'),
('JPY', 'Japanese Yen', '¥', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01'),
('AUD', 'Australian Dollar', 'A$', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01'),
('CNY', 'Chinese Yuan', '¥', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01'),
('GBP', 'British Pound', '£', 'BI_KURS_TENGAH', 'HARIAN', '2026-01-01');

-- 12.7 Lookup Data (sample)
INSERT INTO sys.lookup (lookup_group, lookup_key, lookup_value, sort_order) VALUES
-- Tipe Instrumen (5 values — revised v1.2)
('TIPE_INSTRUMEN', 'CASH', 'Cash di Bank', 1),
('TIPE_INSTRUMEN', 'DEPOSITO', 'Deposito', 2),
('TIPE_INSTRUMEN', 'OBLIGASI', 'Obligasi', 3),
('TIPE_INSTRUMEN', 'SAHAM', 'Saham', 4),
('TIPE_INSTRUMEN', 'REKSADANA', 'Reksadana', 5),
-- Sub-Tipe Instrumen (per tipe — revised v1.2)
-- CASH sub-tipe
('SUBTIPE_CASH', 'GIRO', 'Giro', 1),
('SUBTIPE_CASH', 'TABUNGAN', 'Tabungan', 2),
-- DEPOSITO sub-tipe
('SUBTIPE_DEPOSITO', 'BERJANGKA', 'Deposito Berjangka', 1),
('SUBTIPE_DEPOSITO', 'ON_CALL', 'Deposito On-Call', 2),
-- OBLIGASI sub-tipe
('SUBTIPE_OBLIGASI', 'NEGARA', 'Obligasi Negara (SUN/SBN/ORI)', 1),
('SUBTIPE_OBLIGASI', 'KORPORASI', 'Obligasi Korporasi', 2),
('SUBTIPE_OBLIGASI', 'SUKUK_NEGARA', 'Sukuk Negara (SR/ST/PBS)', 3),
('SUBTIPE_OBLIGASI', 'SUKUK_KORPORASI', 'Sukuk Korporasi (Ijarah/Mudharabah/Wakalah)', 4),
-- SAHAM sub-tipe
('SUBTIPE_SAHAM', 'LQ45', 'Saham LQ45', 1),
('SUBTIPE_SAHAM', 'IDX30', 'Saham IDX30', 2),
('SUBTIPE_SAHAM', 'NON_LQ45', 'Saham di luar LQ45/IDX30', 3),
('SUBTIPE_SAHAM', 'PAPAN_PENGEMBANGAN', 'Saham Papan Pengembangan', 4),
-- REKSADANA sub-tipe
('SUBTIPE_REKSADANA', 'PENDAPATAN_TETAP', 'Reksadana Pendapatan Tetap', 1),
('SUBTIPE_REKSADANA', 'CAMPURAN', 'Reksadana Campuran', 2),
('SUBTIPE_REKSADANA', 'SAHAM', 'Reksadana Saham', 3),
('SUBTIPE_REKSADANA', 'PASAR_UANG', 'Reksadana Pasar Uang', 4),
('SUBTIPE_REKSADANA', 'ETF', 'Exchange-Traded Fund (ETF)', 5),
('KLASIFIKASI_PSAK71', 'AC', 'Amortized Cost', 1),
('KLASIFIKASI_PSAK71', 'FVOCI', 'Fair Value through OCI', 2),
('KLASIFIKASI_PSAK71', 'FVOCI_ELECTION', 'FVOCI Election (Equity Irrevocable)', 3),
('KLASIFIKASI_PSAK71', 'FVTPL', 'Fair Value through Profit/Loss', 4),
('RATING_PEFINDO', 'idAAA', 'idAAA - Highest grade', 1),
('RATING_PEFINDO', 'idAA+', 'idAA+', 2),
('RATING_PEFINDO', 'idAA', 'idAA', 3),
('RATING_PEFINDO', 'idAA-', 'idAA-', 4),
('RATING_PEFINDO', 'idA+', 'idA+', 5),
('RATING_PEFINDO', 'idA', 'idA', 6),
('RATING_PEFINDO', 'idA-', 'idA-', 7),
('RATING_PEFINDO', 'idBBB+', 'idBBB+ - Lower investment grade', 8),
('RATING_PEFINDO', 'idBBB', 'idBBB', 9),
('RATING_PEFINDO', 'idBBB-', 'idBBB-', 10),
('RATING_PEFINDO', 'idBB+', 'idBB+ - Speculative', 11),
('RATING_PEFINDO', 'idBB', 'idBB', 12),
('RATING_PEFINDO', 'idBB-', 'idBB-', 13),
('RATING_PEFINDO', 'idB+', 'idB+ - Highly speculative', 14),
('RATING_PEFINDO', 'idB', 'idB', 15),
('RATING_PEFINDO', 'idB-', 'idB-', 16),
('RATING_PEFINDO', 'idCCC', 'idCCC - Substantial risk', 17),
('RATING_PEFINDO', 'idD', 'idD - Default', 18);

-- 12.8 PD Pefindo — ACTUAL DATA from Pefindo Annual Default Study 2007-2025 (Appendix 2, Debt Instrument basis)
-- Reference: Pefindo_Annual_Default_Study_2007-2025_EN.pdf, Appendix 2 (Survival Pool Cumulative Average Default Rate)
-- Note: idB shows 0.00% due to PEFINDO's limited monitoring population for B-rated instruments
-- Note: idBB Y1 is very high (50.08%) — actual observed historical default rate; if conservative Tugure portfolio avoids non-IG, exposure should be minimal
INSERT INTO mst.pd_pefindo (rating, pd_12month, pd_lifetime_3y, pd_lifetime_5y, pd_lifetime_7y, pd_lifetime_10y, sumber, tanggal_publikasi, periode_berlaku_dari, uploaded_by) VALUES
('idAAA', 0.0000, 0.0000, 0.0000, 0.0000, 0.0000, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idAA',  0.0000, 0.0000, 0.0020, 0.0020, 0.0020, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idA',   0.0031, 0.0290, 0.0549, 0.0549, 0.0549, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idBBB', 0.0567, 0.1734, 0.1866, 0.1934, 0.1934, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idBB',  0.5008, 0.5683, 0.5683, 0.5683, 0.5683, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idB',   0.0000, 0.0000, 0.0000, 0.0000, 0.0000, 'PEFINDO_DS_2007_2025_AppendixA2_LIMITED_POPULATION', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idCCC', 0.0939, 0.6633, 0.6633, 0.6633, 0.6633, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idD',   1.0000, 1.0000, 1.0000, 1.0000, 1.0000, 'PEFINDO_DS_2007_2025_AppendixA2', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001');

-- Note: Pefindo Y1 PD for B-rated dan limited population indicates need for Internal Model Adjustment
-- Risk Management policy boleh override dengan internal model bila monitoring confidence rendah

-- 12.9 Sample CoA (10 most important accounts)
INSERT INTO mst.chart_of_accounts (kode_akun, nama_akun, tipe_akun, sub_tipe_akun, kategori_investasi, posisi_normal, sumber_coa, tanggal_mulai_aktif, created_by) VALUES
('1.1.1.001', 'Kas - Bank Mandiri (IDR)', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.001', 'Surat Berharga AC - Obligasi', 'ASET', 'TIDAK_LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.002', 'Surat Berharga AC - Deposito', 'ASET', 'LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.001', 'Surat Berharga FVOCI - Obligasi', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.001', 'Surat Berharga FVTPL - Saham', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.002', 'Surat Berharga FVTPL - Reksadana', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.001', 'CKPN - Surat Berharga AC', 'ASET', 'KONTRA', 'CKPN', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.001', 'OCI - Selisih MTM FVOCI Obligasi', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.003', 'OCI - CKPN FVOCI', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.002', 'Pendapatan Kupon - Obligasi', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.1.001', 'Beban CKPN - Surat Berharga', 'BEBAN', 'OPERASIONAL', 'CKPN', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.001', 'Beban PPh Final - Bunga', 'BEBAN', 'OPERASIONAL', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001');

-- ====================================================================
-- 12.10 EXTENDED SAMPLE SEED DATA (v1.3 — for Phase 1 development)
-- Comprehensive sample data covering all open items per Decision Log DEC-004 etc.
-- Catatan: Sample data ini untuk development/UAT environment.
-- Production seed harus di-replace dengan actual Tugure data.
-- ====================================================================

-- 12.10.1 ADDITIONAL USERS (10 standard roles)
INSERT INTO sec.user (id, username, email, full_name, unit_kerja, jabatan, status, mfa_enrolled, created_by) VALUES
('00000000-0000-0000-0000-000000000010', 'treasury.maker@tugu-re.com', 'treasury.maker@tugu-re.com', 'Andi Treasury Maker (Sample)', 'Direktorat Investasi & Treasury', 'Treasury Officer', 'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000011', 'treasury.approver@tugu-re.com', 'treasury.approver@tugu-re.com', 'Budi Treasury Manager (Sample)', 'Direktorat Investasi & Treasury', 'Treasury Manager', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000012', 'risk.officer@tugu-re.com', 'risk.officer@tugu-re.com', 'Citra Risk Officer (Sample)', 'Direktorat Risk Management', 'Risk Officer', 'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000013', 'akuntansi@tugu-re.com', 'akuntansi@tugu-re.com', 'Dewi Akuntansi (Sample)', 'Direktorat Keuangan', 'Senior Accountant', 'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000014', 'finance.controller@tugu-re.com', 'finance.controller@tugu-re.com', 'Eko Finance Controller (Sample)', 'Direktorat Keuangan', 'Finance Controller', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000015', 'cfo@tugu-re.com', 'cfo@tugu-re.com', 'Fauzi CFO (Sample)', 'Board of Directors', 'CFO', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000016', 'auditor@tugu-re.com', 'auditor@tugu-re.com', 'Gita Internal Auditor (Sample)', 'Internal Audit', 'Auditor', 'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000017', 'komite.investasi@tugu-re.com', 'komite.investasi@tugu-re.com', 'Hadi Komite Investasi Chair (Sample)', 'Komite Investasi', 'Komite Chair', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000018', 'alco@tugu-re.com', 'alco@tugu-re.com', 'Indri ALCO Member (Sample)', 'ALCO / Komite Risiko', 'ALCO Member', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000019', 'it.admin@tugu-re.com', 'it.admin@tugu-re.com', 'Joko IT Admin (Sample)', 'Direktorat Teknologi Informasi', 'IT Admin', 'AKTIF', TRUE, '00000000-0000-0000-0000-000000000001');

-- Assign users to roles
INSERT INTO sec.user_role (user_id, role_id, assigned_by)
SELECT u.id, r.id, '00000000-0000-0000-0000-000000000001'
FROM sec.user u, sec.role r
WHERE (u.username = 'treasury.maker@tugu-re.com' AND r.role_code = 'ROLE-MAKER-TR')
   OR (u.username = 'treasury.approver@tugu-re.com' AND r.role_code = 'ROLE-APPR-TR')
   OR (u.username = 'risk.officer@tugu-re.com' AND r.role_code = 'ROLE-RISK')
   OR (u.username = 'akuntansi@tugu-re.com' AND r.role_code = 'ROLE-AKUN')
   OR (u.username = 'finance.controller@tugu-re.com' AND r.role_code = 'ROLE-AKUN-CTL')
   OR (u.username = 'cfo@tugu-re.com' AND r.role_code = 'ROLE-CFO')
   OR (u.username = 'auditor@tugu-re.com' AND r.role_code = 'ROLE-AUDIT')
   OR (u.username = 'komite.investasi@tugu-re.com' AND r.role_code = 'ROLE-KOMITE')
   OR (u.username = 'alco@tugu-re.com' AND r.role_code = 'ROLE-ALCO')
   OR (u.username = 'it.admin@tugu-re.com' AND r.role_code = 'ROLE-IT-ADMIN');

-- 12.10.2 EXPANDED CHART OF ACCOUNTS (50+ accounts — sample Tugure structure)
-- Replace nantinya dengan actual export dari Tugure GL existing
INSERT INTO mst.chart_of_accounts (kode_akun, nama_akun, tipe_akun, sub_tipe_akun, kategori_investasi, posisi_normal, sumber_coa, tanggal_mulai_aktif, created_by) VALUES
-- ASET LANCAR — Cash & Bank
('1.1.1.001', 'Kas - Bank Mandiri (IDR)', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.002', 'Kas - Bank BCA (IDR)', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.003', 'Kas - Bank BNI (IDR)', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.004', 'Kas - Bank BRI (IDR)', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.010', 'Kas Bank USD - Bank Mandiri', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga AC
('1.1.2.001', 'Surat Berharga AC - Obligasi Negara', 'ASET', 'TIDAK_LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.002', 'Surat Berharga AC - Obligasi Korporasi', 'ASET', 'TIDAK_LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.003', 'Surat Berharga AC - Sukuk Negara', 'ASET', 'TIDAK_LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.004', 'Surat Berharga AC - Sukuk Korporasi', 'ASET', 'TIDAK_LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.005', 'Surat Berharga AC - Deposito Berjangka', 'ASET', 'LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.006', 'Surat Berharga AC - Deposito On-Call', 'ASET', 'LANCAR', 'AC', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga FVOCI
('1.1.3.001', 'Surat Berharga FVOCI - Obligasi Negara', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.002', 'Surat Berharga FVOCI - Obligasi Korporasi', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.003', 'Surat Berharga FVOCI - Sukuk Negara', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.004', 'Surat Berharga FVOCI - Sukuk Korporasi', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.005', 'Surat Berharga FVOCI - Reksadana', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.010', 'Surat Berharga FVOCI Election - Saham Strategis', 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga FVTPL
('1.1.4.001', 'Surat Berharga FVTPL - Saham', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.002', 'Surat Berharga FVTPL - Reksadana Pasar Uang', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.003', 'Surat Berharga FVTPL - Reksadana Pendapatan Tetap', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.004', 'Surat Berharga FVTPL - Reksadana Saham', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.005', 'Surat Berharga FVTPL - Reksadana Campuran', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.006', 'Surat Berharga FVTPL - ETF', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.007', 'Surat Berharga FVTPL - Obligasi Trading', 'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Akrual & CKPN
('1.1.9.001', 'CKPN - Surat Berharga AC', 'ASET', 'KONTRA', 'CKPN', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.003', 'Akrual Bunga - Deposito', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.004', 'Akrual Kupon - Obligasi', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.005', 'Akrual Bunga - Cash Bank', 'ASET', 'LANCAR', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- EKUITAS - OCI
('3.2.1.001', 'OCI - Selisih MTM FVOCI Obligasi', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.002', 'OCI - Selisih MTM FVOCI Saham (Election)', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.003', 'OCI - CKPN FVOCI (Memo)', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.004', 'OCI - Selisih MTM FVOCI Reksadana', 'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- PENDAPATAN
('4.1.1.001', 'Pendapatan Bunga - Deposito', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.002', 'Pendapatan Kupon - Obligasi', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.003', 'Pendapatan Bagi Hasil - Sukuk', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.004', 'Pendapatan Bunga - Cash Bank', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.2.001', 'Pendapatan Dividen', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.2.002', 'Pendapatan Distribusi Reksadana', 'PENDAPATAN', 'OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.001', 'Realized Gain/Loss - Penjualan SB', 'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.002', 'Unrealized Gain/Loss - MTM FVTPL', 'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.003', 'Realized Gain/Loss - Reklasifikasi OCI ke P&L', 'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.4.001', 'Realized FX Gain/Loss', 'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.4.002', 'Unrealized FX Gain/Loss', 'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- BEBAN
('5.1.1.001', 'Beban CKPN - Surat Berharga', 'BEBAN', 'OPERASIONAL', 'CKPN', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.1.002', 'Pemulihan Beban CKPN', 'BEBAN', 'OPERASIONAL', 'CKPN', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.001', 'Beban PPh Final - Bunga Deposito (20%)', 'BEBAN', 'OPERASIONAL', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.002', 'Beban PPh Final - Kupon Obligasi (10%)', 'BEBAN', 'OPERASIONAL', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.003', 'Beban PPh Final - Dividen (10%)', 'BEBAN', 'OPERASIONAL', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.3.001', 'Beban Komisi - Transaksi Investasi', 'BEBAN', 'OPERASIONAL', NULL, 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001');

-- 12.10.3 SAMPLE COUNTERPARTY (Bank, Issuer, MI, Kustodian — minimal essential)
INSERT INTO mst.counterparty (id, kode_counterparty, nama, tipe, rating_pefindo_current, tipe_eksposur_basel, eligible_lps_flag, status, created_by) VALUES
-- Pemerintah RI (Sovereign)
('11111111-0000-0000-0000-000000000001', 'CP-0001', 'Pemerintah Republik Indonesia', 'PEMERINTAH', NULL, 'SOVEREIGN', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Bank Besar (untuk Cash + Deposito + LPS)
('11111111-0000-0000-0000-000000000002', 'CP-0002', 'PT Bank Mandiri (Persero) Tbk', 'BANK', 'idAAA', 'SENIOR_UNSECURED', TRUE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000003', 'CP-0003', 'PT Bank Central Asia Tbk', 'BANK', 'idAAA', 'SENIOR_UNSECURED', TRUE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000004', 'CP-0004', 'PT Bank Negara Indonesia (Persero) Tbk', 'BANK', 'idAAA', 'SENIOR_UNSECURED', TRUE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000005', 'CP-0005', 'PT Bank Rakyat Indonesia (Persero) Tbk', 'BANK', 'idAAA', 'SENIOR_UNSECURED', TRUE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000006', 'CP-0006', 'PT Bank CIMB Niaga Tbk', 'BANK', 'idAAA', 'SENIOR_UNSECURED', TRUE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Issuer Korporasi (untuk Obligasi)
('11111111-0000-0000-0000-000000000010', 'CP-0010', 'PT Telkom Indonesia (Persero) Tbk', 'KORPORASI', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000011', 'CP-0011', 'PT Perusahaan Listrik Negara (Persero)', 'KORPORASI', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000012', 'CP-0012', 'PT Jasa Marga (Persero) Tbk', 'KORPORASI', 'idAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000013', 'CP-0013', 'PT Indosat Tbk', 'KORPORASI', 'idAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000014', 'CP-0014', 'PT Adhi Karya (Persero) Tbk', 'KORPORASI', 'idA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000015', 'CP-0015', 'PT Pegadaian (Persero)', 'KORPORASI', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Manajer Investasi (untuk Reksadana)
('11111111-0000-0000-0000-000000000020', 'CP-0020', 'PT Schroder Investment Management Indonesia', 'MANAJER_INVESTASI', 'idAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000021', 'CP-0021', 'PT Bahana TCW Investment Management', 'MANAJER_INVESTASI', 'idAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000022', 'CP-0022', 'PT Mandiri Manajemen Investasi', 'MANAJER_INVESTASI', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000023', 'CP-0023', 'PT BNP Paribas Asset Management', 'MANAJER_INVESTASI', 'idAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Bank Kustodian (untuk Reksadana custodian)
('11111111-0000-0000-0000-000000000030', 'CP-0030', 'Standard Chartered Bank - Custody Services', 'BANK_KUSTODIAN', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000031', 'CP-0031', 'Citibank N.A. - Custody', 'BANK_KUSTODIAN', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000032', 'CP-0032', 'PT Bank Mandiri (Persero) Tbk - Custody Division', 'BANK_KUSTODIAN', 'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Emiten Saham (untuk Saham FVOCI Election)
('11111111-0000-0000-0000-000000000040', 'CP-0040', 'PT Bank Central Asia Tbk (BBCA)', 'EMITEN_SAHAM', NULL, 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000041', 'CP-0041', 'PT Astra International Tbk (ASII)', 'EMITEN_SAHAM', NULL, 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001');

-- Rating History initial untuk Counterparty
INSERT INTO mst.rating_history_counterparty (rating_history_id_kode, counterparty_id, tanggal_berlaku, rating_pefindo, rating_outlook, sumber_rating, tanggal_publikasi_rating, action_type, notch_change, maker_id) VALUES
('RTH-2026-00001', '11111111-0000-0000-0000-000000000002', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00002', '11111111-0000-0000-0000-000000000003', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00003', '11111111-0000-0000-0000-000000000004', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00004', '11111111-0000-0000-0000-000000000005', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00005', '11111111-0000-0000-0000-000000000006', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00010', '11111111-0000-0000-0000-000000000010', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00011', '11111111-0000-0000-0000-000000000011', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00012', '11111111-0000-0000-0000-000000000012', '2026-01-01', 'idAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00013', '11111111-0000-0000-0000-000000000013', '2026-01-01', 'idAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00014', '11111111-0000-0000-0000-000000000014', '2026-01-01', 'idA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00015', '11111111-0000-0000-0000-000000000015', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012');

-- 12.10.4 SAMPLE PORTOFOLIO (5 standard portfolios)
INSERT INTO mst.portofolio (id, kode_portofolio, nama, tujuan_pengelolaan, bm_category_default, benchmark, kompensasi_manager_basis, periode_review_terakhir, created_by) VALUES
('22222222-0000-0000-0000-000000000001', 'PORT-TR-LIQ', 'Treasury Liquidity', 'Pengelolaan likuiditas harian — Cash & Deposito jangka pendek', 'HTC', 'BI Rate', 'Berbasis bunga', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000002', 'PORT-INV-LT', 'Investment Long-Term', 'Investasi jangka panjang — Obligasi held-to-maturity', 'HTC', 'INDOBeX Composite Bond Index', 'Berbasis bunga + holding', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000003', 'PORT-INV-LIQ', 'Investment Liquidity', 'Obligasi yang dapat dijual untuk manajemen likuiditas — FVOCI', 'HTCS', 'IBPA Govt Bond Index', 'Berbasis bunga + realized gain/loss', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000004', 'PORT-TRADING', 'Trading Portfolio', 'Trading book — FVTPL, profit-taking', 'OTHER', 'Total return', 'Berbasis fair value performance', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000005', 'PORT-STRATEGIC', 'Strategic Equity', 'Penyertaan strategis saham (FVOCI Election)', 'OTHER', 'Long-term holding', 'Berbasis dividen + holding', '2026-01-01', '00000000-0000-0000-0000-000000000001');

-- 12.10.5 INITIAL PERIODE BUKU 2026 (12 bulanan + 4 triwulanan + 1 tahunan)
INSERT INTO mst.periode_buku (periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan, tanggal_mulai, tanggal_akhir, status_periode) VALUES
-- Bulanan 2026
('PRD-2026-01', 'BULANAN', 2026, 1, NULL, '2026-01-01', '2026-01-31', 'OPEN'),
('PRD-2026-02', 'BULANAN', 2026, 2, NULL, '2026-02-01', '2026-02-28', 'OPEN'),
('PRD-2026-03', 'BULANAN', 2026, 3, NULL, '2026-03-01', '2026-03-31', 'OPEN'),
('PRD-2026-04', 'BULANAN', 2026, 4, NULL, '2026-04-01', '2026-04-30', 'OPEN'),
('PRD-2026-05', 'BULANAN', 2026, 5, NULL, '2026-05-01', '2026-05-31', 'OPEN'),
('PRD-2026-06', 'BULANAN', 2026, 6, NULL, '2026-06-01', '2026-06-30', 'OPEN'),
('PRD-2026-07', 'BULANAN', 2026, 7, NULL, '2026-07-01', '2026-07-31', 'OPEN'),
('PRD-2026-08', 'BULANAN', 2026, 8, NULL, '2026-08-01', '2026-08-31', 'OPEN'),
('PRD-2026-09', 'BULANAN', 2026, 9, NULL, '2026-09-01', '2026-09-30', 'OPEN'),
('PRD-2026-10', 'BULANAN', 2026, 10, NULL, '2026-10-01', '2026-10-31', 'OPEN'),
('PRD-2026-11', 'BULANAN', 2026, 11, NULL, '2026-11-01', '2026-11-30', 'OPEN'),
('PRD-2026-12', 'BULANAN', 2026, 12, NULL, '2026-12-01', '2026-12-31', 'OPEN'),
-- Triwulanan 2026
('PRD-2026-Q1', 'TRIWULANAN', 2026, NULL, 1, '2026-01-01', '2026-03-31', 'OPEN'),
('PRD-2026-Q2', 'TRIWULANAN', 2026, NULL, 2, '2026-04-01', '2026-06-30', 'OPEN'),
('PRD-2026-Q3', 'TRIWULANAN', 2026, NULL, 3, '2026-07-01', '2026-09-30', 'OPEN'),
('PRD-2026-Q4', 'TRIWULANAN', 2026, NULL, 4, '2026-10-01', '2026-12-31', 'OPEN'),
-- Tahunan 2026
('PRD-2026', 'TAHUNAN', 2026, NULL, NULL, '2026-01-01', '2026-12-31', 'OPEN');

-- 12.10.6 INITIAL FX RATE (sample untuk USD/IDR 1 Jan 2026)
INSERT INTO mst.kurs (fx_rate_id_kode, kode_mata_uang, tanggal_berlaku, kurs_tengah, sumber_kurs, periode_bulanan_id, maker_id) 
SELECT 
    'FX-USD-20260101', 'USD', '2026-01-01', 16000.0000, 'BI_JISDOR', 
    p.id, 
    '00000000-0000-0000-0000-000000000013'
FROM mst.periode_buku p WHERE p.periode_id_kode = 'PRD-2026-01';

-- 12.10.7 INITIAL IMPACT MEV TO PD (sample untuk PRD-2026-01)
INSERT INTO mst.impact_mev_pd (periode_id, skenario, impact_multiplier, mev_components_json, catatan, maker_id, approver_id)
SELECT 
    p.id, 'GOOD', 0.5000, 
    '{"gdp_growth_yoy": 5.5, "inflasi_yoy": 2.5, "bi_rate": 5.00, "usd_idr": 15500, "ihsg_growth_yoy": 15.0}'::JSONB,
    'Sample optimistic scenario — GDP +5.5%, BI Rate turun, USD/IDR menguat', 
    '00000000-0000-0000-0000-000000000018', '00000000-0000-0000-0000-000000000015'
FROM mst.periode_buku p WHERE p.periode_id_kode = 'PRD-2026-01';

INSERT INTO mst.impact_mev_pd (periode_id, skenario, impact_multiplier, mev_components_json, catatan, maker_id, approver_id)
SELECT 
    p.id, 'BAD', 2.5000,
    '{"gdp_growth_yoy": 3.5, "inflasi_yoy": 6.0, "bi_rate": 7.50, "usd_idr": 17500, "ihsg_growth_yoy": -10.0}'::JSONB,
    'Sample pessimistic scenario — GDP melemah, inflasi naik, BI Rate naik', 
    '00000000-0000-0000-0000-000000000018', '00000000-0000-0000-0000-000000000015'
FROM mst.periode_buku p WHERE p.periode_id_kode = 'PRD-2026-01';

-- 12.10.8 INITIAL IMPACT PD MULTIPLIER (sample untuk PRD-2026-01)
INSERT INTO mst.impact_pd (periode_id, impact_multiplier, catatan, maker_id, approver_id)
SELECT 
    p.id, 1.1500,
    'Sample standard forward-looking adjustment — 15% overlay untuk uncertainty buffer',
    '00000000-0000-0000-0000-000000000018', '00000000-0000-0000-0000-000000000015'
FROM mst.periode_buku p WHERE p.periode_id_kode = 'PRD-2026-01';

-- 12.10.9 SAMPLE INSTRUMEN (3 sample untuk smoke test development)
-- 1 deposito, 1 obligasi negara, 1 reksadana
INSERT INTO mst.instrumen (id, kode_instrumen, tipe_instrumen, sub_tipe, nama, isin, counterparty_id, mata_uang, nominal, tanggal_penempatan, tanggal_jatuh_tempo, kupon, frekuensi_bunga, sppi_result, bm_category, klasifikasi_psak71, klasifikasi_locked_at, klasifikasi_locked_by, sppi_bm_last_review_date, status, portofolio_id, workflow_status, created_by, approved_by) VALUES
-- Deposito Bank Mandiri 6M
('33333333-0000-0000-0000-000000000001', 'DEP-2026-00001', 'DEPOSITO', 'BERJANGKA', 
 'Deposito Bank Mandiri 6M Sample', NULL, 
 '11111111-0000-0000-0000-000000000002', 'IDR', 1000000000.00, '2026-01-15', '2026-07-15',
 6.0000, 'JATUH_TEMPO', 'PASS', 'HTC', 'AC', '2026-01-15 10:00:00+07', 
 '00000000-0000-0000-0000-000000000017', '2026-01-15', 'AKTIF',
 '22222222-0000-0000-0000-000000000001', 'APPROVED', 
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011'),
-- Obligasi Negara ORI Sample
('33333333-0000-0000-0000-000000000002', 'OBL-2026-00001', 'OBLIGASI', 'NEGARA',
 'ORI Sample 2026 Seri 023', 'IDG000016605',
 '11111111-0000-0000-0000-000000000001', 'IDR', 5000000000.00, '2026-01-20', '2029-01-20',
 5.5000, 'SEMESTERAN', 'PASS', 'HTCS', 'FVOCI', '2026-01-20 14:00:00+07',
 '00000000-0000-0000-0000-000000000017', '2026-01-20', 'AKTIF',
 '22222222-0000-0000-0000-000000000003', 'APPROVED',
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011'),
-- Reksadana Pendapatan Tetap
('33333333-0000-0000-0000-000000000003', 'RDN-2026-00001', 'REKSADANA', 'PENDAPATAN_TETAP',
 'Reksadana Sample Pendapatan Tetap (FVTPL Trading)', 'IDN000XYZABC',
 '11111111-0000-0000-0000-000000000020', 'IDR', 2000000000.00, '2026-02-01', NULL,
 NULL, NULL, 'FAIL', 'OTHER', 'FVTPL', '2026-02-01 09:00:00+07',
 '00000000-0000-0000-0000-000000000017', '2026-02-01', 'AKTIF',
 '22222222-0000-0000-0000-000000000004', 'APPROVED',
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011'),
-- Reksadana Pendapatan Tetap FVOCI (kebijakan akuntansi) — long-term holding
('33333333-0000-0000-0000-000000000004', 'RDN-2026-00002', 'REKSADANA', 'PENDAPATAN_TETAP',
 'Reksadana Sample Pendapatan Tetap (FVOCI Long-Term)', 'IDN000PTLT001',
 '11111111-0000-0000-0000-000000000022', 'IDR', 3000000000.00, '2026-02-15', NULL,
 NULL, NULL, 'FAIL', 'HTCS', 'FVOCI', '2026-02-15 11:00:00+07',
 '00000000-0000-0000-0000-000000000017', '2026-02-15', 'AKTIF',
 '22222222-0000-0000-0000-000000000003', 'APPROVED',
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011'),
-- Reksadana Campuran FVOCI (kebijakan akuntansi) — long-term holding
('33333333-0000-0000-0000-000000000005', 'RDN-2026-00003', 'REKSADANA', 'CAMPURAN',
 'Reksadana Sample Campuran (FVOCI Long-Term)', 'IDN000CPLT001',
 '11111111-0000-0000-0000-000000000023', 'IDR', 2500000000.00, '2026-03-01', NULL,
 NULL, NULL, 'FAIL', 'HTCS', 'FVOCI', '2026-03-01 13:00:00+07',
 '00000000-0000-0000-0000-000000000017', '2026-03-01', 'AKTIF',
 '22222222-0000-0000-0000-000000000002', 'APPROVED',
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011'),
-- Reksadana Pasar Uang FVTPL (default short-term)
('33333333-0000-0000-0000-000000000006', 'RDN-2026-00004', 'REKSADANA', 'PASAR_UANG',
 'Reksadana Sample Pasar Uang (FVTPL Trading)', 'IDN000PUST001',
 '11111111-0000-0000-0000-000000000021', 'IDR', 1500000000.00, '2026-01-25', NULL,
 NULL, NULL, 'FAIL', 'OTHER', 'FVTPL', '2026-01-25 10:30:00+07',
 '00000000-0000-0000-0000-000000000017', '2026-01-25', 'AKTIF',
 '22222222-0000-0000-0000-000000000001', 'APPROVED',
 '00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000011');

-- Update reksadana dengan manajer_investasi_id dan bank_kustodian_id
UPDATE mst.instrumen SET manajer_investasi_id = '11111111-0000-0000-0000-000000000020', bank_kustodian_id = '11111111-0000-0000-0000-000000000030' WHERE kode_instrumen = 'RDN-2026-00001'; -- Schroder + Std Chartered Custody
UPDATE mst.instrumen SET manajer_investasi_id = '11111111-0000-0000-0000-000000000022', bank_kustodian_id = '11111111-0000-0000-0000-000000000032' WHERE kode_instrumen = 'RDN-2026-00002'; -- Mandiri MI + Mandiri Custody (FVOCI PT)
UPDATE mst.instrumen SET manajer_investasi_id = '11111111-0000-0000-0000-000000000023', bank_kustodian_id = '11111111-0000-0000-0000-000000000031' WHERE kode_instrumen = 'RDN-2026-00003'; -- BNP Paribas + Citi Custody (FVOCI CP)
UPDATE mst.instrumen SET manajer_investasi_id = '11111111-0000-0000-0000-000000000021', bank_kustodian_id = '11111111-0000-0000-0000-000000000030' WHERE kode_instrumen = 'RDN-2026-00004'; -- Bahana TCW + Std Chartered Custody (FVTPL PU)

-- 12.10.10 SAMPLE MAPPING JURNAL HEADER (4 core events untuk smoke test)
INSERT INTO mst.mapping_jurnal_header (event_id_kode, event_code, nama_event, kategori_event, trigger_source, klasifikasi_berlaku, catatan, created_by) VALUES
('EVT-PENEMPATAN', 'PENEMPATAN', 'Penempatan Instrumen Investasi', 'PENEMPATAN', 'USER_INPUT', ARRAY['AC','FVOCI','FVTPL'], 'Triggered saat trx.penempatan APPROVED', '00000000-0000-0000-0000-000000000013'),
('EVT-AKRUAL_BUNGA', 'AKRUAL_BUNGA', 'Akrual Bunga Harian', 'AKRUAL', 'SYSTEM_JOB', ARRAY['AC','FVOCI'], 'Job daily end-of-day untuk DEPOSITO & OBLIGASI', '00000000-0000-0000-0000-000000000013'),
('EVT-MTM_FVOCI', 'MTM_FVOCI', 'MTM Harian FVOCI', 'MUTASI_MTM', 'SYSTEM_JOB', ARRAY['FVOCI'], 'Job daily MTM untuk FVOCI utang', '00000000-0000-0000-0000-000000000013'),
('EVT-ECL_PEMBENTUKAN', 'ECL_PEMBENTUKAN', 'ECL Akhir Bulan', 'ECL', 'SYSTEM_JOB', ARRAY['AC','FVOCI'], 'Monthly job untuk pembentukan/update CKPN', '00000000-0000-0000-0000-000000000013');

-- 12.10.11 SAMPLE MAPPING JURNAL DETAIL (per event)
-- Event PENEMPATAN
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'EAD_IDR', 'AC', 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='PENEMPATAN' AND a.kode_akun='1.1.2.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'EAD_IDR', 'FVOCI', 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='PENEMPATAN' AND a.kode_akun='1.1.3.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'EAD_IDR', 'FVTPL', 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='PENEMPATAN' AND a.kode_akun='1.1.4.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 2, a.id, 'KREDIT', 'EAD_IDR', NULL, 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='PENEMPATAN' AND a.kode_akun='1.1.1.001';

-- Event AKRUAL_BUNGA — Deposito
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, tipe_instrumen_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'BUNGA_AKRUAL_IDR', ARRAY['DEPOSITO'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='AKRUAL_BUNGA' AND a.kode_akun='1.1.9.003';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, tipe_instrumen_filter, multiplier)
SELECT h.id, 2, a.id, 'KREDIT', 'BUNGA_AKRUAL_IDR', ARRAY['DEPOSITO'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='AKRUAL_BUNGA' AND a.kode_akun='4.1.1.001';

-- Event AKRUAL_BUNGA — Obligasi
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, tipe_instrumen_filter, multiplier)
SELECT h.id, 3, a.id, 'DEBIT', 'KUPON_AKRUAL_IDR', ARRAY['OBLIGASI'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='AKRUAL_BUNGA' AND a.kode_akun='1.1.9.004';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, tipe_instrumen_filter, multiplier)
SELECT h.id, 4, a.id, 'KREDIT', 'KUPON_AKRUAL_IDR', ARRAY['OBLIGASI'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='AKRUAL_BUNGA' AND a.kode_akun='4.1.1.002';

-- Event ECL_PEMBENTUKAN (AC: kontra-aset; FVOCI: kontra OCI)
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'ECL_AMOUNT_IDR', NULL, 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='ECL_PEMBENTUKAN' AND a.kode_akun='5.1.1.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 2, a.id, 'KREDIT', 'ECL_AMOUNT_IDR', 'AC', 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='ECL_PEMBENTUKAN' AND a.kode_akun='1.1.9.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, multiplier)
SELECT h.id, 2, a.id, 'KREDIT', 'ECL_AMOUNT_IDR', 'FVOCI', 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='ECL_PEMBENTUKAN' AND a.kode_akun='3.2.1.003';

-- Event MTM_FVOCI — Obligasi (tipe_instrumen filter OBLIGASI)
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, tipe_instrumen_filter, multiplier)
SELECT h.id, 1, a.id, 'DEBIT', 'MTM_DELTA_IDR', 'FVOCI', ARRAY['OBLIGASI'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='MTM_FVOCI' AND a.kode_akun='1.1.3.001';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, tipe_instrumen_filter, multiplier)
SELECT h.id, 2, a.id, 'KREDIT', 'MTM_DELTA_IDR', 'FVOCI', ARRAY['OBLIGASI'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='MTM_FVOCI' AND a.kode_akun='3.2.1.001';

-- Event MTM_FVOCI — Reksadana (tipe_instrumen filter REKSADANA — FVOCI kebijakan)
INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, tipe_instrumen_filter, multiplier)
SELECT h.id, 3, a.id, 'DEBIT', 'MTM_DELTA_IDR', 'FVOCI', ARRAY['REKSADANA'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='MTM_FVOCI' AND a.kode_akun='1.1.3.005';

INSERT INTO mst.mapping_jurnal_detail (event_header_id, urutan, kode_akun_id, dk_indicator, sumber_amount, klasifikasi_filter, tipe_instrumen_filter, multiplier)
SELECT h.id, 4, a.id, 'KREDIT', 'MTM_DELTA_IDR', 'FVOCI', ARRAY['REKSADANA'], 1.0000
FROM mst.mapping_jurnal_header h, mst.chart_of_accounts a
WHERE h.event_code='MTM_FVOCI' AND a.kode_akun='3.2.1.004';

-- ====================================================================
-- END OF EXTENDED SAMPLE SEED DATA v1.3
-- ====================================================================

-- ====================================================================
-- 13. GRANT PRIVILEGES (per Role example)
-- ====================================================================
\echo '  Setting up role privileges...'

-- Application service account untuk read-write per schema (sample — adjust per environment)
-- CREATE ROLE blips_app_svc;
-- GRANT USAGE ON SCHEMA mst, trx, ecl, sppi, doc, jrnl TO blips_app_svc;
-- GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA mst, trx, ecl, sppi, doc, jrnl TO blips_app_svc;
-- GRANT SELECT, INSERT ON aud.audit_log TO blips_app_svc;  -- append-only

-- Auditor read-only role
-- CREATE ROLE blips_auditor;
-- GRANT USAGE ON SCHEMA mst, trx, ecl, sppi, doc, jrnl, aud TO blips_auditor;
-- GRANT SELECT ON ALL TABLES IN SCHEMA mst, trx, ecl, sppi, doc, jrnl, aud TO blips_auditor;

\echo '======================================================'
\echo '  BLIPS IFRS 9 — Schema Initialization COMPLETE       '
\echo '======================================================'
\echo ''
\echo '  Next Steps:'
\echo '  1. Configure application connection string'
\echo '  2. Run additional seed: full lookup data, mapping jurnal events'
\echo '  3. Upload Pefindo Default Study latest via UI'
\echo '  4. Generate periode_buku untuk tahun fiskal current via sp_periode_init'
\echo '  5. Configure scheduled jobs (BI JISDOR, MTM, Akrual, ECL)'
\echo ''
\echo '  See ERD-BLIPS-IFRS9-v1.0.docx for complete schema reference.'
\echo '======================================================'
