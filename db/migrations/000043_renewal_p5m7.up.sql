-- migration: 0043 renewal_p5m7
-- author: data-modeler (driven by system-analyst P5-M7)
-- requires: 0001 (init_schema — fn_update_updated_at, fn_increment_row_version),
--           0040 (trx.mtm — confirms trx schema stable),
--           0041 (sys.upload_batch pattern + mst.mapping_jurnal confirmed)
-- description:
--   P5-M7 Renewal Deposito:
--   (A) CREATE TABLE trx.renewal — renewal request partitioned monthly by tanggal_efektif_baru.
--       Includes all audit columns, workflow cols, CHECK constraints, SoD constraint,
--       partial unique index (1 active renewal per instrumen_lama).
--   (B) DB TRIGGER trg_renewal_updated_at + trg_renewal_row_version — standard audit triggers.
--   (C) DB TRIGGER tg_renewal_sod_check — defence-in-depth SoD (maker ≠ approver).
--   (D) Seed mst.mapping_jurnal placeholder for RENEWAL_DEPOSITO event code (4 legs, akun kosong).
--   (E) Seed sys.config_param: RENEWAL_MIN_BUNGA_BERSIH_IDR.

BEGIN;

-- ====================================================================
-- A. CREATE TABLE trx.renewal
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.renewal (
    -- ── Primary key ──────────────────────────────────────────────────
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- ── Business keys ────────────────────────────────────────────────
    instrumen_lama_id       UUID            NOT NULL,
    -- ^ FK → mst.instrumen(id). Instrumen yang di-renew. Must be jenis=DEPOSITO, status=ACTIVE.

    instrumen_baru_id       UUID,
    -- ^ FK → mst.instrumen(id). Populated after APPROVED side-effect. NULL until then.

    -- ── Renewal parameters ───────────────────────────────────────────
    skema                   TEXT            NOT NULL,
    -- ^ POKOK_SAJA | POKOK_PLUS_BUNGA. CHECK constraint below.

    tenor_baru_bulan        SMALLINT        NOT NULL,
    -- ^ 1-60 bulan. Validated at service layer + CHECK constraint.

    rate_baru_persen        NUMERIC(7,4)    NOT NULL,
    -- ^ Rate p.a. dalam persen. 0.0000–30.0000. DEC-016: NUMERIC(7,4).

    tanggal_efektif_baru    DATE            NOT NULL,
    -- ^ Tanggal efektif renewal. Partition range key. Periode buku harus OPEN.

    tanggal_jatuh_tempo_baru DATE           NOT NULL,
    -- ^ = tanggal_efektif_baru + tenor_baru_bulan. Computed by service.

    -- ── Kalkulasi preview ────────────────────────────────────────────
    pokok_lama              NUMERIC(20,4)   NOT NULL,
    -- ^ Snapshot pokok instrumen lama saat request dibuat.

    pokok_baru              NUMERIC(20,4)   NOT NULL,
    -- ^ Hasil kalkulasi: pokok_lama (POKOK_SAJA) atau pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA).

    bunga_kotor             NUMERIC(20,4)   NOT NULL,
    -- ^ pokok_lama × (rate_lama/100) × (hari_berjalan/365). DEC-016: NUMERIC(20,4).

    pph_amount              NUMERIC(20,4)   NOT NULL,
    -- ^ bunga_kotor × 0.20 (PP No. 131/2000). Validated against: |pph_amount - bunga_kotor × 0.20| ≤ 0.01.

    bunga_bersih            NUMERIC(20,4)   NOT NULL,
    -- ^ bunga_kotor - pph_amount. DEC-016: NUMERIC(20,4).

    eir_baru                NUMERIC(10,8),
    -- ^ EIR Newton-Raphson after-tax. Populated after APPROVED. DEC-016: NUMERIC(10,8).

    schedule_baru_jsonb     JSONB,
    -- ^ Amortisasi schedule preview (optional, for display only). Not the canonical schedule.

    -- ── Workflow state ────────────────────────────────────────────────
    status                  TEXT            NOT NULL    DEFAULT 'PENDING_APPROVAL',
    -- ^ PENDING_APPROVAL | APPROVED | POSTED | REJECTED. See CHECK below.

    -- ── Workflow actors ───────────────────────────────────────────────
    maker_id                UUID            NOT NULL,
    -- ^ User who created the renewal request. FK → sec.user(id).

    approver_id             UUID,
    -- ^ User who approved/rejected. FK → sec.user(id). NULL until approved/rejected.

    -- ── Workflow reasons / audit text ────────────────────────────────
    request_reason          TEXT,
    -- ^ Optional maker comment.

    approve_reason          TEXT,
    -- ^ Approver comment on approve/reject action.

    reject_reason           TEXT,
    -- ^ Populated on REJECTED. Redundant alias for approve_reason; kept for clarity.

    -- ── Signature fields ──────────────────────────────────────────────
    signature_method        TEXT,
    -- ^ 'JWT_STEP_UP'. Validated at service layer before APPROVED.

    signature_hash_meta     JSONB,
    -- ^ Optional JWT signature metadata snapshot for audit trail.

    approved_at             TIMESTAMPTZ,
    -- ^ Timestamp of approval/rejection action.

    -- ── Jurnal linkage ────────────────────────────────────────────────
    jurnal_header_id        UUID,
    -- ^ FK → jrnl.jurnal_header(id). Populated after POSTED. NULL until jurnal posted.

    -- ── Periode linkage ───────────────────────────────────────────────
    periode_bulanan_id      UUID,
    -- ^ FK → mst.periode_buku(id). Resolved from tanggal_efektif_baru at create time.

    -- ── Audit columns (wajib per db-conventions.md) ──────────────────
    created_at              TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL    DEFAULT 1,
    tenant_id               TEXT            NOT NULL    DEFAULT 'TUGURE'

) PARTITION BY RANGE (tanggal_efektif_baru);

-- ── CHECK constraints ──────────────────────────────────────────────────────────

ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_skema
        CHECK (skema IN ('POKOK_SAJA', 'POKOK_PLUS_BUNGA'));

ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_status
        CHECK (status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED', 'REJECTED'));

ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_tenor_range
        CHECK (tenor_baru_bulan BETWEEN 1 AND 60);

ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_rate_range
        CHECK (rate_baru_persen >= 0.0000 AND rate_baru_persen <= 30.0000);

-- PPh 20% check: |pph_amount - bunga_kotor * 0.20| <= 0.01 (rounding tolerance)
ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_pph_20pct
        CHECK (ABS(pph_amount - bunga_kotor * 0.20) <= 0.01);

-- bunga_bersih = bunga_kotor - pph_amount (tolerance 0.01)
ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_bunga_bersih
        CHECK (ABS(bunga_bersih - (bunga_kotor - pph_amount)) <= 0.01);

-- POKOK_PLUS_BUNGA: pokok_baru = pokok_lama + bunga_bersih (tolerance 0.01)
-- POKOK_SAJA: pokok_baru = pokok_lama
ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_pokok_baru_skema
        CHECK (
            (skema = 'POKOK_SAJA' AND ABS(pokok_baru - pokok_lama) <= 0.01)
            OR
            (skema = 'POKOK_PLUS_BUNGA' AND ABS(pokok_baru - (pokok_lama + bunga_bersih)) <= 0.01)
        );

-- SoD: maker_id ≠ approver_id (second layer — service layer is primary)
ALTER TABLE trx.renewal
    ADD CONSTRAINT chk_renewal_sod
        CHECK (approver_id IS NULL OR maker_id != approver_id);

-- ── Default partition (catch-all for rows outside explicit partitions) ─────────
-- Partitions for 2026-2028 will be added by partition management job.
-- Default partition prevents INSERT failure for dates without explicit partition.
CREATE TABLE IF NOT EXISTS trx.renewal_default
    PARTITION OF trx.renewal DEFAULT;

-- Partition for 2026 H2
CREATE TABLE IF NOT EXISTS trx.renewal_y2026m07
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2026m08
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2026m09
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2026m10
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2026m11
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2026m12
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

CREATE TABLE IF NOT EXISTS trx.renewal_y2027m01
    PARTITION OF trx.renewal
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

-- ── Indexes ───────────────────────────────────────────────────────────────────

-- Primary lookup: renewal by instrumen_lama (most common query)
CREATE INDEX IF NOT EXISTS idx_renewal_instrumen_lama
    ON trx.renewal (instrumen_lama_id, tenant_id)
    WHERE deleted_at IS NULL;

-- Status queue: PENDING_APPROVAL queue for ROLE-APPR-TR
CREATE INDEX IF NOT EXISTS idx_renewal_status_tenant
    ON trx.renewal (status, tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- Maker view: maker's own renewals
CREATE INDEX IF NOT EXISTS idx_renewal_maker_id
    ON trx.renewal (maker_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- Periode lookup
CREATE INDEX IF NOT EXISTS idx_renewal_periode_bulanan
    ON trx.renewal (periode_bulanan_id)
    WHERE deleted_at IS NULL;

-- Instrumen baru → traceability back to renewal
CREATE INDEX IF NOT EXISTS idx_renewal_instrumen_baru
    ON trx.renewal (instrumen_baru_id)
    WHERE instrumen_baru_id IS NOT NULL AND deleted_at IS NULL;

-- ── Partial unique index: 1 active renewal per instrumen_lama ─────────────────
-- Prevents creating a second renewal for an instrumen that already has one PENDING,
-- APPROVED, or POSTED. REJECTED renewals may be followed by a new attempt.
CREATE UNIQUE INDEX IF NOT EXISTS uq_renewal_instrumen_lama_active
    ON trx.renewal (instrumen_lama_id)
    WHERE status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED')
      AND deleted_at IS NULL;

-- ====================================================================
-- B. TRIGGERS — updated_at + row_version (standard pattern)
-- ====================================================================

CREATE OR REPLACE TRIGGER trg_renewal_updated_at
    BEFORE UPDATE ON trx.renewal
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE OR REPLACE TRIGGER trg_renewal_row_version
    BEFORE UPDATE ON trx.renewal
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- C. TRIGGER — SoD defence-in-depth (DB layer)
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_renewal_sod_check()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.approver_id IS NOT NULL AND NEW.maker_id = NEW.approver_id THEN
        RAISE EXCEPTION 'SoD violation: maker_id (%) = approver_id (%) in trx.renewal. DEC-017.',
            NEW.maker_id, NEW.approver_id
            USING ERRCODE = 'unique_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER tg_renewal_sod_check
    BEFORE INSERT OR UPDATE ON trx.renewal
    FOR EACH ROW EXECUTE FUNCTION fn_renewal_sod_check();

-- ====================================================================
-- D. Seed mst.mapping_jurnal — RENEWAL_DEPOSITO event code (DRAFT)
-- ====================================================================
-- Leg accounts are left empty (NULL) and must be configured by Finance Controller
-- before the first renewal can be approved + posted. If event code is missing,
-- the jurnal engine returns JURNAL_EVENT_CODE_NOT_FOUND → renewal approve fails (S5-AC4).

INSERT INTO mst.mapping_jurnal (
    event_code,
    event_name,
    leg_no,
    akun_debit_kode,
    akun_kredit_kode,
    keterangan,
    status,
    tenant_id,
    created_at,
    created_by,
    updated_at,
    updated_by,
    row_version
)
VALUES
    -- Leg 1: PPh final 20%
    ('RENEWAL_DEPOSITO', 'Renewal Deposito — PPh Final 20%',    1, NULL, NULL,
     'Setoran PPh final: Dr Kewajiban PPh Deposito / Cr Kas Bank',
     'DRAFT', 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1),
    -- Leg 2: Pelunasan pokok lama
    ('RENEWAL_DEPOSITO', 'Renewal Deposito — Pelunasan Pokok Lama', 2, NULL, NULL,
     'Pelunasan deposito lama: Dr Deposito (lama) / Cr Kas Bank',
     'DRAFT', 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1),
    -- Leg 3: Penempatan pokok baru
    ('RENEWAL_DEPOSITO', 'Renewal Deposito — Penempatan Pokok Baru', 3, NULL, NULL,
     'Penempatan deposito baru: Dr Kas Bank / Cr Deposito (baru)',
     'DRAFT', 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1),
    -- Leg 4: Bunga bersih diterima
    ('RENEWAL_DEPOSITO', 'Renewal Deposito — Bunga Bersih',     4, NULL, NULL,
     'Bunga bersih after-tax: Dr Beban Bunga Deposito / Cr Kas Bank',
     'DRAFT', 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1)

ON CONFLICT (event_code, leg_no) DO NOTHING;

-- ====================================================================
-- E. Seed sys.config_param — RENEWAL_MIN_BUNGA_BERSIH_IDR
-- ====================================================================

INSERT INTO sys.config_param (param_key, param_value, keterangan, tenant_id,
    created_at, created_by, updated_at, updated_by, row_version)
VALUES
    ('RENEWAL_MIN_BUNGA_BERSIH_IDR', '100000',
     'Minimum bunga_bersih IDR untuk skema POKOK_PLUS_BUNGA (BRD §6.2). Default: IDR 100.000.',
     'TUGURE', now(), '00000000-0000-0000-0000-000000000001',
     now(), '00000000-0000-0000-0000-000000000001', 1)
ON CONFLICT (param_key, tenant_id) DO NOTHING;

COMMIT;
