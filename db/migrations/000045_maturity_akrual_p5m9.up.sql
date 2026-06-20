-- migration: 0045 maturity_akrual_p5m9
-- author: backend-engineer-go (driven by system-analyst P5-M9)
-- requires: 0001 (fn_update_updated_at, fn_increment_row_version),
--           0044 (trx.penjualan — confirms trx schema stable),
--           phase-4-ecl (ecl.calc_result_line, ecl.amortisasi_schedule assumed present)
-- description:
--   P5-M9 Jatuh Tempo + Pendapatan Akrual:
--   (A) CREATE TABLE trx.jatuh_tempo — maturity settlement events, partitioned monthly
--   (B) CREATE TABLE trx.pendapatan_akrual — daily accrual per instrumen, partitioned monthly
--   (C) CREATE TABLE trx.dividen — dividend/distribution events, partitioned monthly
--   (D) Triggers: updated_at + row_version on all three tables
--   (E) Partial unique index: prevents duplicate akrual per (instrumen, date, type)
--   (F) sys.holiday_calendar table for cron skip logic
--   (G) Seed mst.mapping_jurnal placeholders for 8 akrual event codes (DRAFT)
--   (H) Seed sys.config_param for akrual config

BEGIN;

-- ====================================================================
-- A. CREATE TABLE trx.jatuh_tempo
-- Partitioned monthly by tanggal_jatuh_tempo (DATE).
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo (
    -- ── Primary key ─────────────────────────────────────────────────
    id                      UUID            NOT NULL DEFAULT gen_random_uuid(),

    -- ── Business key ────────────────────────────────────────────────
    instrumen_id            UUID            NOT NULL,
    -- ^ FK → mst.instrumen(id). Instrumen yang jatuh tempo.

    tanggal_jatuh_tempo     DATE            NOT NULL,
    -- ^ Partition key. Tanggal maturity per kontrak.

    jenis                   TEXT            NOT NULL,
    -- ^ 'DEPOSITO' | 'BOND' | 'REKSADANA'

    -- ── Settlement amounts (DEC-016: NUMERIC(20,4) IDR) ─────────────
    pokok_returned          NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ Pokok yang dikembalikan (face value for bonds, principal for deposito)

    bunga_returned          NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ Bunga last / bunga hari terakhir yang termasuk dalam settlement

    pph                     NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ PPh final yang dipotong (deposito: 20% dari bunga)

    proceeds                NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ Net kas diterima = pokok + bunga - pph (IDR)

    fx_rate_id              UUID,
    -- ^ FK → sys.fx_rate(id). NULL if IDR instrumen.

    -- ── Snapshot & jurnal ────────────────────────────────────────────
    klasifikasi_snapshot    TEXT            NOT NULL,
    -- ^ Snapshot klasifikasi_psak71 dari mst.instrumen saat maturity

    jurnal_header_id        UUID,
    -- ^ FK → jrnl.jurnal_header(id). Populated saat SETTLED.

    -- ── Status ──────────────────────────────────────────────────────
    status                  TEXT            NOT NULL DEFAULT 'PENDING',
    -- ^ 'PENDING' | 'SETTLED' | 'FAILED' | 'SKIPPED'

    error_message           TEXT,
    -- ^ Populated saat FAILED

    -- ── Standard audit columns (db-conventions.md) ───────────────────
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ── Constraints ─────────────────────────────────────────────────
    CONSTRAINT chk_jatuh_tempo_jenis
        CHECK (jenis IN ('DEPOSITO', 'BOND', 'REKSADANA')),
    CONSTRAINT chk_jatuh_tempo_status
        CHECK (status IN ('PENDING', 'SETTLED', 'FAILED', 'SKIPPED')),
    CONSTRAINT chk_jatuh_tempo_pokok_nonneg
        CHECK (pokok_returned >= 0),
    CONSTRAINT chk_jatuh_tempo_proceeds_nonneg
        CHECK (proceeds >= 0),
    CONSTRAINT chk_jatuh_tempo_pph_nonneg
        CHECK (pph >= 0),

    PRIMARY KEY (id, tanggal_jatuh_tempo)
) PARTITION BY RANGE (tanggal_jatuh_tempo);

-- Initial partitions 2026-2027
CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m06
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m07
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m08
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m09
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m10
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m11
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS trx.jatuh_tempo_y2026m12
    PARTITION OF trx.jatuh_tempo
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Indexes
CREATE INDEX idx_jatuh_tempo_instrumen_id
    ON trx.jatuh_tempo (instrumen_id);

CREATE INDEX idx_jatuh_tempo_status
    ON trx.jatuh_tempo (status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_jatuh_tempo_tanggal_status
    ON trx.jatuh_tempo (tanggal_jatuh_tempo, status)
    WHERE deleted_at IS NULL;

-- ====================================================================
-- B. CREATE TABLE trx.pendapatan_akrual
-- Partitioned monthly by tanggal_akrual (DATE).
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual (
    -- ── Primary key ─────────────────────────────────────────────────
    id                      UUID            NOT NULL DEFAULT gen_random_uuid(),

    -- ── Business key ────────────────────────────────────────────────
    instrumen_id            UUID            NOT NULL,
    -- ^ FK → mst.instrumen(id)

    tanggal_akrual          DATE            NOT NULL,
    -- ^ Partition key. Tanggal akrual harian.

    jenis                   TEXT            NOT NULL,
    -- ^ 'BUNGA' | 'DIVIDEN' | 'AMORTISASI_PREMIUM' | 'AMORTISASI_DISKON' | 'DISTRIBUSI_REKSADANA'

    -- ── Stage & carrying basis ───────────────────────────────────────
    stage                   SMALLINT,
    -- ^ Stage ECL saat tanggal_akrual (1, 2, or 3). NULL untuk non-ECL (FVTPL dividen).

    carrying_basis          NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ IDR basis yang digunakan untuk komputasi:
    --   Stage 1/2: Gross Carrying Amount
    --   Stage 3: Net Carrying Amount (Gross - ECL_allowance) per §5.4.1(b)

    eir_persen              NUMERIC(10,8),
    -- ^ Effective Interest Rate yang digunakan (annual). DEC-016: NUMERIC(10,8).
    --   POCI: credit-adjusted EIR dari ecl.amortisasi_schedule versi POCI.

    -- ── Amounts (DEC-016: NUMERIC(20,4) IDR) ────────────────────────
    bunga_kotor             NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ Akrual kotor sebelum PPh (IDR atau equivalent IDR untuk FCY)

    pph                     NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ PPh yang dipotong: deposito 20% (bunga), dividen 10% (UU PPh §17 2c)

    bunga_bersih            NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ bunga_kotor - pph

    -- ── Multi-currency ───────────────────────────────────────────────
    fx_rate_id              UUID,
    -- ^ FK → sys.fx_rate(id). NULL jika IDR.

    mata_uang               VARCHAR(10)     NOT NULL DEFAULT 'IDR',
    -- ^ ISO 4217. 'IDR' untuk instrumen rupiah.

    -- ── ECL reference ───────────────────────────────────────────────
    ecl_run_id_used         UUID,
    -- ^ FK → ecl.ecl_calc_run(id). Sealed ECL run yang digunakan untuk Stage 3 net carrying.
    --   NULL untuk Stage 1/2 atau jika tidak ada sealed run (stale_staging_flag=TRUE).

    stale_staging_flag      BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE jika ECL sealed run > AKRUAL_STAGING_STALE_DAYS lalu, atau tidak ada sealed run untuk Stage 3.
    --   Triggers PENDING_STALE_REVIEW status.

    -- ── Override (stale) ────────────────────────────────────────────
    override_user_id        UUID,
    -- ^ User ID ROLE-AKUN-CTL yang melakukan override. Populated saat OVERRIDE_APPROVED.

    override_comment        TEXT,
    -- ^ Alasan override, min 30 char (service validation).

    -- ── Snapshot & jurnal ────────────────────────────────────────────
    klasifikasi_snapshot    TEXT,
    -- ^ Snapshot klasifikasi_psak71 dari mst.instrumen saat tanggal_akrual.

    jurnal_header_id        UUID,
    -- ^ FK → jrnl.jurnal_header(id). Populated saat AUTO_POSTED atau POSTED.

    -- ── Workflow ────────────────────────────────────────────────────
    status                  TEXT            NOT NULL DEFAULT 'AUTO_POSTED',
    -- ^ 'AUTO_POSTED' | 'PENDING_STALE_REVIEW' | 'OVERRIDE_APPROVED' | 'POSTED' | 'SKIPPED'

    periode_bulanan_id      UUID,
    -- ^ FK → mst.periode_buku(id). Periode buku saat akrual dipost.

    -- ── Standard audit columns ───────────────────────────────────────
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ── Constraints ─────────────────────────────────────────────────
    CONSTRAINT chk_pendapatan_akrual_jenis
        CHECK (jenis IN ('BUNGA', 'DIVIDEN', 'AMORTISASI_PREMIUM', 'AMORTISASI_DISKON', 'DISTRIBUSI_REKSADANA')),
    CONSTRAINT chk_pendapatan_akrual_status
        CHECK (status IN ('AUTO_POSTED', 'PENDING_STALE_REVIEW', 'OVERRIDE_APPROVED', 'POSTED', 'SKIPPED')),
    CONSTRAINT chk_pendapatan_akrual_stage
        CHECK (stage IS NULL OR stage IN (1, 2, 3)),
    CONSTRAINT chk_pendapatan_akrual_carrying_nonneg
        CHECK (carrying_basis >= 0),
    CONSTRAINT chk_pendapatan_akrual_bunga_nonneg
        CHECK (bunga_kotor >= 0),
    CONSTRAINT chk_pendapatan_akrual_pph_nonneg
        CHECK (pph >= 0),
    CONSTRAINT chk_pendapatan_akrual_bersih_nonneg
        CHECK (bunga_bersih >= 0),

    PRIMARY KEY (id, tanggal_akrual)
) PARTITION BY RANGE (tanggal_akrual);

-- Initial partitions 2026
CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m06
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m07
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m08
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m09
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m10
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m11
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS trx.pendapatan_akrual_y2026m12
    PARTITION OF trx.pendapatan_akrual
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Indexes
CREATE INDEX idx_pendapatan_akrual_instrumen_id
    ON trx.pendapatan_akrual (instrumen_id);

CREATE INDEX idx_pendapatan_akrual_tanggal_status
    ON trx.pendapatan_akrual (tanggal_akrual, status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_pendapatan_akrual_stale
    ON trx.pendapatan_akrual (instrumen_id, tanggal_akrual)
    WHERE stale_staging_flag = TRUE AND deleted_at IS NULL;

-- Partial unique index: prevents duplicate akrual per (instrumen, date, type).
-- This is the idempotency guard required by S2-AC4 and cron design.
CREATE UNIQUE INDEX uq_pendapatan_akrual_instrumen_tanggal_jenis
    ON trx.pendapatan_akrual (instrumen_id, tanggal_akrual, jenis)
    WHERE deleted_at IS NULL;

-- ====================================================================
-- C. CREATE TABLE trx.dividen
-- Partitioned monthly by tanggal_terima (DATE).
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.dividen (
    -- ── Primary key ─────────────────────────────────────────────────
    id                      UUID            NOT NULL DEFAULT gen_random_uuid(),

    -- ── Business key ────────────────────────────────────────────────
    instrumen_id            UUID            NOT NULL,
    -- ^ FK → mst.instrumen(id). Saham atau Reksadana penerima dividen.

    tanggal_terima          DATE            NOT NULL,
    -- ^ Partition key. Tanggal dividen/distribusi diterima (cum date atau payment date).

    tanggal_cum_date        DATE,
    -- ^ Cum dividend date dari emiten (optional, informational).

    -- ── Amounts (DEC-016: NUMERIC(20,4) IDR) ────────────────────────
    jumlah_kotor            NUMERIC(20,4)   NOT NULL,
    -- ^ Gross dividend/distribusi sebelum PPh final.

    pph_dividen             NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ PPh final 10% (UU PPh §17 ayat 2c). = jumlah_kotor × 0.10.

    jumlah_bersih           NUMERIC(20,4)   NOT NULL,
    -- ^ Net dividend = jumlah_kotor - pph_dividen.

    -- ── Classification ───────────────────────────────────────────────
    klasifikasi_snapshot    TEXT            NOT NULL,
    -- ^ Snapshot klasifikasi_psak71 saat tanggal_terima.

    treatment               TEXT            NOT NULL DEFAULT 'P&L',
    -- ^ 'P&L' (FVTPL, Reksadana) | 'OCI' (FVOCI Election per kebijakan Tugure)
    --   Konfirmasi per RACI: ROLE-AKUN/CFO sesuai FSD-APP-B §10.

    is_reksadana            BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE jika distribusi reksadana (look-through applicable).

    -- ── Workflow (4-eyes) ────────────────────────────────────────────
    status                  TEXT            NOT NULL DEFAULT 'PENDING_APPROVAL',
    -- ^ 'PENDING_APPROVAL' | 'APPROVED' | 'POSTED' | 'REJECTED'

    maker_id                UUID            NOT NULL,
    -- ^ ROLE-MAKER-TR yang input dividen. SoD: maker_id ≠ approver_id.

    approver_id             UUID,
    -- ^ ROLE-APPR-TR. Populated saat approve.

    approve_comment         TEXT,
    reject_reason           TEXT,

    signature_method        TEXT,
    -- ^ 'JWT_STEP_UP' pada approval.

    approved_at             TIMESTAMPTZ,

    jurnal_header_id        UUID,
    -- ^ FK → jrnl.jurnal_header(id). Populated saat POSTED.

    -- ── Standard audit columns ───────────────────────────────────────
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ── Constraints ─────────────────────────────────────────────────
    CONSTRAINT chk_dividen_treatment
        CHECK (treatment IN ('P&L', 'OCI')),
    CONSTRAINT chk_dividen_status
        CHECK (status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED', 'REJECTED')),
    CONSTRAINT chk_dividen_jumlah_pos
        CHECK (jumlah_kotor > 0),
    CONSTRAINT chk_dividen_pph_nonneg
        CHECK (pph_dividen >= 0),
    CONSTRAINT chk_dividen_bersih_nonneg
        CHECK (jumlah_bersih >= 0),
    CONSTRAINT chk_dividen_sod
        CHECK (approver_id IS NULL OR maker_id <> approver_id),

    PRIMARY KEY (id, tanggal_terima)
) PARTITION BY RANGE (tanggal_terima);

-- Partitions 2026
CREATE TABLE IF NOT EXISTS trx.dividen_y2026m06
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m07
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m12
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Indexes
CREATE INDEX idx_dividen_instrumen_id
    ON trx.dividen (instrumen_id);

CREATE INDEX idx_dividen_status
    ON trx.dividen (status)
    WHERE deleted_at IS NULL;

-- ====================================================================
-- D. TRIGGERS
-- ====================================================================

-- trx.jatuh_tempo
CREATE TRIGGER trg_jatuh_tempo_updated_at
    BEFORE UPDATE ON trx.jatuh_tempo
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_jatuh_tempo_row_version
    BEFORE UPDATE ON trx.jatuh_tempo
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- trx.pendapatan_akrual
CREATE TRIGGER trg_pendapatan_akrual_updated_at
    BEFORE UPDATE ON trx.pendapatan_akrual
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_pendapatan_akrual_row_version
    BEFORE UPDATE ON trx.pendapatan_akrual
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- trx.dividen
CREATE TRIGGER trg_dividen_updated_at
    BEFORE UPDATE ON trx.dividen
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_dividen_row_version
    BEFORE UPDATE ON trx.dividen
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- SoD defense-in-depth for dividen (service layer is primary enforcement)
CREATE OR REPLACE FUNCTION fn_dividen_sod_check()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.approver_id IS NOT NULL AND NEW.approver_id = NEW.maker_id THEN
        RAISE EXCEPTION 'SoD violation: approver_id (%) cannot equal maker_id (%)',
            NEW.approver_id, NEW.maker_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dividen_sod_check
    BEFORE INSERT OR UPDATE ON trx.dividen
    FOR EACH ROW EXECUTE FUNCTION fn_dividen_sod_check();

-- ====================================================================
-- E. sys.holiday_calendar — for cron holiday skip
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.holiday_calendar (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tanggal         DATE        NOT NULL,
    keterangan      TEXT        NOT NULL,
    -- ^ e.g. 'LIBUR_NASIONAL', 'CUTI_BERSAMA', 'WEEKEND'
    jenis           TEXT        NOT NULL DEFAULT 'LIBUR_NASIONAL',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT chk_holiday_jenis
        CHECK (jenis IN ('LIBUR_NASIONAL', 'CUTI_BERSAMA', 'WEEKEND'))
);

CREATE UNIQUE INDEX uq_holiday_calendar_tanggal
    ON sys.holiday_calendar (tanggal)
    WHERE deleted_at IS NULL;

-- Seed 2026 national holidays (abbreviated — IT-ADMIN to maintain)
INSERT INTO sys.holiday_calendar (tanggal, keterangan, jenis, created_by, updated_by)
VALUES
    ('2026-01-01', 'Tahun Baru 2026',          'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-03-28', 'Hari Raya Nyepi',           'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-04-03', 'Wafat Isa Al-Masih',        'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-05-01', 'Hari Buruh',                'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-05-14', 'Kenaikan Isa Al-Masih',     'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-06-01', 'Hari Pancasila',            'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-08-17', 'Hari Kemerdekaan RI',       'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000'),
    ('2026-12-25', 'Hari Natal',                'LIBUR_NASIONAL', '00000000-0000-0000-0000-000000000000', '00000000-0000-0000-0000-000000000000')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- F. Seed mst.mapping_jurnal placeholders (DRAFT)
-- These are placeholder seeds; real GL accounts wired by ROLE-AKUN per P5-M2.
-- ====================================================================

INSERT INTO mst.mapping_jurnal (
    event_code, nama_event, keterangan, status,
    created_at, created_by, updated_at, updated_by, tenant_id
)
SELECT event_code, nama_event, keterangan, 'DRAFT',
       now(), '00000000-0000-0000-0000-000000000000',
       now(), '00000000-0000-0000-0000-000000000000', 'TUGURE'
FROM (VALUES
    ('MATURITY_SETTLEMENT_DEPOSITO', 'Jatuh Tempo Deposito', 'Settlement deposito: Dr Kas, Cr Deposito AC, Cr Pendapatan Bunga, Dr PPh'),
    ('MATURITY_SETTLEMENT_BOND',     'Jatuh Tempo Bond',     'Settlement obligasi: Dr Kas, Cr Aset Bond AC/FVOCI'),
    ('MATURITY_SETTLEMENT_REKSADANA','Jatuh Tempo Reksadana','Settlement reksadana: Dr Kas, Cr Aset Reksadana'),
    ('AKRUAL_BUNGA',                 'Akrual Bunga EIR',     'Stage 1/2: Dr Akrual Bunga Piutang, Cr Pendapatan Bunga'),
    ('AKRUAL_BUNGA_STAGE3',          'Akrual Bunga EIR Stage 3', 'Stage 3: Dr Akrual Bunga Piutang (Net Carrying basis), Cr Pendapatan Bunga'),
    ('DIVIDEN',                      'Dividen/Distribusi',   'Dr Kas, Dr PPh Final, Cr Pendapatan Dividen (P&L or OCI)'),
    ('AMORTISASI_PREMIUM',           'Amortisasi Premium Bond', 'Dr Beban Premium P&L, Cr Aset Bond (carrying turun)'),
    ('AMORTISASI_DISKON',            'Amortisasi Diskon Bond',  'Dr Aset Bond (carrying naik), Cr Pendapatan Amortisasi P&L')
) AS v(event_code, nama_event, keterangan)
WHERE NOT EXISTS (
    SELECT 1 FROM mst.mapping_jurnal mj
    WHERE mj.event_code = v.event_code AND mj.deleted_at IS NULL
);

-- ====================================================================
-- G. Seed sys.config_param for akrual
-- ====================================================================

INSERT INTO sys.config_param (key, value, keterangan, created_by, updated_by)
SELECT key, value, keterangan,
       '00000000-0000-0000-0000-000000000000',
       '00000000-0000-0000-0000-000000000000'
FROM (VALUES
    ('AKRUAL_STAGING_STALE_DAYS',   '30',           'Jumlah hari ECL sealed run dianggap stale untuk Stage 3 akrual'),
    ('DIVIDEN_PPH_PCT',             '10.0',         'PPh final dividen persen (UU PPh §17 ayat 2c). Default 10%.'),
    ('DEPOSITO_PPH_BUNGA_PCT',      '20.0',         'PPh final bunga deposito persen. Default 20%.'),
    ('AKRUAL_CRON_SCHEDULE',        '15 9 * * 1-5', 'Cron schedule DAILY_ACCRUAL_JOB (09:15 WIB hari kerja)'),
    ('AMORTISASI_CRON_SCHEDULE',    '0 10 * * 1-5', 'Cron schedule AMORTISASI_PD_JOB (10:00 WIB hari kerja)'),
    ('MATURITY_CRON_SCHEDULE',      '0 9 * * 1-5',  'Cron schedule MATURITY_PROCESS_JOB (09:00 WIB hari kerja)'),
    ('AKRUAL_BATCH_SIZE',           '100',          'Jumlah instrumen per batch dalam cron akrual')
) AS v(key, value, keterangan)
WHERE NOT EXISTS (
    SELECT 1 FROM sys.config_param cp
    WHERE cp.key = v.key AND cp.deleted_at IS NULL
);

COMMIT;
