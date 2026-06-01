-- migration: 0001 init_schema
-- author: data-modeler
-- requires: (none)
-- description: Initial 9 schemas (~53 tables), indexes, triggers, functions, partitions.
--              Wraps BLIPS_init_schema.sql DDL sections 0-11 (extensions through triggers).
--              Source of truth: ERD-BLIPS-IFRS9-v1.2 + BLIPS_init_schema_v1.3-legacy.sql

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
