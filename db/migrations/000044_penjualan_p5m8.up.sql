-- migration: 0044 penjualan_p5m8
-- author: backend-engineer-go (driven by system-analyst P5-M8)
-- requires: 0001 (fn_update_updated_at, fn_increment_row_version),
--           0040 (trx.mtm — confirms trx schema stable),
--           0043 (trx.renewal — same trx schema, migration ordering)
-- description:
--   P5-M8 Penjualan/Pencairan Instrumen:
--   (A) CREATE TABLE trx.penjualan — disposal request partitioned monthly by tanggal_eksekusi.
--       Columns: jenis_disposal (PARTIAL/FULL), qty fields, proceeds, cost_basis, realized_gl,
--       oci_recycled, klasifikasi_snapshot, BM violation fields, workflow cols, audit + tenant.
--       CHECK constraints: qty_terjual ≤ qty_holding_pre; PARTIAL qty < holding_pre;
--       FULL qty = holding_pre; SoD maker ≠ approver (CHECK advisory, service enforces primary).
--   (B) DB TRIGGER trg_penjualan_updated_at + trg_penjualan_row_version.
--   (C) DB TRIGGER trg_penjualan_sod_check — defence-in-depth SoD.
--   (D) Seed mst.mapping_jurnal placeholders for 6 PENJUALAN_* event codes.
--   (E) Seed sys.config_param: PENJUALAN_BM_WARN_THRESHOLD_PCT, PENJUALAN_BM_BLOCK_THRESHOLD_PCT.
--   (F) Partial unique index: 1 active disposal per instrumen.

BEGIN;

-- ====================================================================
-- A. CREATE TABLE trx.penjualan
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.penjualan (
    -- ── Primary key ──────────────────────────────────────────────────
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- ── Business keys ────────────────────────────────────────────────
    instrumen_id            UUID            NOT NULL,
    -- ^ FK → mst.instrumen(id). Instrumen yang dijual. Must be status=ACTIVE.

    -- ── Disposal parameters ──────────────────────────────────────────
    jenis_disposal          TEXT            NOT NULL,
    -- ^ 'PARTIAL' | 'FULL'. CHECK constraint below.

    qty_terjual             NUMERIC(20,8)   NOT NULL,
    -- ^ Quantity yang dijual (unit/nominal). DEC-016: NUMERIC(20,8). Must be > 0.

    qty_holding_pre         NUMERIC(20,8)   NOT NULL,
    -- ^ Snapshot qty_holding sebelum transaksi ini. From mst.instrumen at request time.

    qty_holding_post        NUMERIC(20,8),
    -- ^ Snapshot qty_holding setelah transaksi. Populated saat APPROVED/POSTED.
    -- FULL disposal: NULL (instrumen → DISPOSED). PARTIAL: qty_holding_pre - qty_terjual.

    harga_jual_per_unit     NUMERIC(20,4)   NOT NULL,
    -- ^ Harga jual per unit IDR. Must be > 0. DEC-016: NUMERIC(20,4).

    -- ── Computed financials ──────────────────────────────────────────
    proceed                 NUMERIC(20,4)   NOT NULL,
    -- ^ harga_jual_per_unit × qty_terjual. DEC-016: NUMERIC(20,4).

    cost_basis              NUMERIC(20,4)   NOT NULL,
    -- ^ Per klasifikasi: AC/POCI = amortized carrying; FVOCI = amortized; FVTPL = MTM; FVOCI_ELECTION = cost.

    realized_gl             NUMERIC(20,4)   NOT NULL,
    -- ^ proceed - cost_basis. Positive = gain, negative = loss.

    oci_recycled            NUMERIC(20,4),
    -- ^ OCI recycled ke P&L (FVOCI debt only). NULL for AC/FVTPL/POCI/FVOCI_ELECTION.
    -- PARTIAL: proportional (qty_terjual/qty_holding_pre × oci_cumulative).
    -- FULL: full oci_cumulative.

    oci_cumulative_total    NUMERIC(20,4),
    -- ^ Snapshot of total OCI cumulative at time of disposal (informational, from trx.mtm).

    -- ── Klasifikasi snapshot ─────────────────────────────────────────
    klasifikasi_snapshot    TEXT            NOT NULL,
    -- ^ Snapshot of mst.instrumen.klasifikasi_psak71 at create time.
    -- AC | FVOCI | FVOCI_ELECTION | FVTPL | POCI

    jurnal_event_code       TEXT,
    -- ^ Primary event code posted to P5-M2 (e.g. 'PENJUALAN_FVOCI_DEBT').
    -- Populated after APPROVED. May be composite (comma-separated for multi-leg).

    tanggal_eksekusi        DATE            NOT NULL,
    -- ^ Tanggal eksekusi penjualan. Partition range key. Periode buku must be OPEN.

    -- ── BM frequency tracking ────────────────────────────────────────
    bm_violation_risk       BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE if cumulative 12-month disposal exceeded warn threshold at time of approval.

    bm_violation_pct        NUMERIC(7,4),
    -- ^ Actual cumulative disposal % at time of BM check. NULL if non-HTC portofolio.

    -- ── Workflow state ────────────────────────────────────────────────
    status                  TEXT            NOT NULL DEFAULT 'PENDING_APPROVAL',
    -- ^ PENDING_APPROVAL | APPROVED | POSTED | REJECTED | PENDING_BM_REVIEW

    -- ── Workflow actors ───────────────────────────────────────────────
    maker_id                UUID            NOT NULL,
    -- ^ User who created the disposal request. FK → sec.user(id).

    approver_id             UUID,
    -- ^ User who approved/rejected. FK → sec.user(id). NULL until acted.

    approve_comment         TEXT,
    -- ^ Approver comment.

    reject_reason           TEXT,
    -- ^ Reject reason (≥ 30 char if status=REJECTED).

    signature_method        TEXT,
    -- ^ 'JWT_STEP_UP' — recorded when approved or rejected.

    signature_hash_meta     JSONB,
    -- ^ Optional JWT signature metadata for audit trail.

    approved_at             TIMESTAMPTZ,
    -- ^ Timestamp when approve/reject action occurred. DEC-018.

    -- ── Post-approval references ─────────────────────────────────────
    jurnal_header_id        UUID,
    -- ^ FK → jrnl.jurnal_header(id). Populated after jurnal posting (step POSTED).

    periode_bulanan_id      UUID,
    -- ^ FK → mst.periode_buku(id). Linked at create time.

    instrumen_status_after  TEXT,
    -- ^ 'DISPOSED' (FULL disposal) or 'ACTIVE' (PARTIAL). Populated after POSTED.

    -- ── Standard audit columns (db-conventions.md) ────────────────────
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ── CHECK constraints ────────────────────────────────────────────

    CONSTRAINT chk_penjualan_jenis_disposal
        CHECK (jenis_disposal IN ('PARTIAL', 'FULL')),

    CONSTRAINT chk_penjualan_status
        CHECK (status IN ('PENDING_APPROVAL', 'APPROVED', 'POSTED', 'REJECTED', 'PENDING_BM_REVIEW')),

    CONSTRAINT chk_penjualan_qty_terjual_positive
        CHECK (qty_terjual > 0),

    CONSTRAINT chk_penjualan_qty_lte_holding
        CHECK (qty_terjual <= qty_holding_pre),

    CONSTRAINT chk_penjualan_partial_qty_lt_holding
        CHECK (jenis_disposal <> 'PARTIAL' OR qty_terjual < qty_holding_pre),

    CONSTRAINT chk_penjualan_full_qty_eq_holding
        CHECK (jenis_disposal <> 'FULL' OR qty_terjual = qty_holding_pre),

    CONSTRAINT chk_penjualan_harga_positive
        CHECK (harga_jual_per_unit > 0),

    CONSTRAINT chk_penjualan_proceed_positive
        CHECK (proceed > 0),

    -- SoD advisory: primary enforcement at service layer (DEC-017).
    CONSTRAINT chk_penjualan_sod
        CHECK (maker_id <> approver_id OR approver_id IS NULL),

    -- FVOCI_ELECTION must not have oci_recycled set (no P&L recycling per §B5.7.1).
    CONSTRAINT chk_penjualan_fvoci_election_no_oci_recycle
        CHECK (klasifikasi_snapshot <> 'FVOCI_ELECTION' OR oci_recycled IS NULL OR oci_recycled = 0),

    CONSTRAINT chk_penjualan_klasifikasi_snapshot
        CHECK (klasifikasi_snapshot IN ('AC', 'FVOCI', 'FVOCI_ELECTION', 'FVTPL', 'POCI'))

) PARTITION BY RANGE (tanggal_eksekusi);

-- ── Partition: default catch-all + current year partitions ───────────

CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m01 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m02 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m03 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m04 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m05 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m06 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m07 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m08 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m09 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m10 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m11 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_y2026m12 PARTITION OF trx.penjualan
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE IF NOT EXISTS trx.penjualan_default  PARTITION OF trx.penjualan DEFAULT;

-- ── Indexes ───────────────────────────────────────────────────────────

-- FK indexes (PG does not auto-index FK columns)
CREATE INDEX idx_penjualan_instrumen_id      ON trx.penjualan (instrumen_id);
CREATE INDEX idx_penjualan_maker_id          ON trx.penjualan (maker_id);
CREATE INDEX idx_penjualan_approver_id       ON trx.penjualan (approver_id) WHERE approver_id IS NOT NULL;
CREATE INDEX idx_penjualan_jurnal_header_id  ON trx.penjualan (jurnal_header_id) WHERE jurnal_header_id IS NOT NULL;
CREATE INDEX idx_penjualan_periode_id        ON trx.penjualan (periode_bulanan_id) WHERE periode_bulanan_id IS NOT NULL;

-- Composite hot-path indexes
CREATE INDEX idx_penjualan_tenant_created    ON trx.penjualan (tenant_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_penjualan_status_eksekusi   ON trx.penjualan (status, tanggal_eksekusi DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_penjualan_bm_risk           ON trx.penjualan (bm_violation_risk) WHERE bm_violation_risk = TRUE AND deleted_at IS NULL;

-- ── Partial unique: 1 active (non-completed, non-rejected) disposal per instrumen ──
-- Prevents duplicate pending disposal for same instrumen at same time.
CREATE UNIQUE INDEX uq_penjualan_instrumen_active
    ON trx.penjualan (instrumen_id)
    WHERE status IN ('PENDING_APPROVAL', 'APPROVED', 'PENDING_BM_REVIEW') AND deleted_at IS NULL;

-- ====================================================================
-- B. Triggers — updated_at + row_version
-- ====================================================================

CREATE OR REPLACE TRIGGER trg_penjualan_updated_at
    BEFORE UPDATE ON trx.penjualan
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE OR REPLACE TRIGGER trg_penjualan_row_version
    BEFORE UPDATE ON trx.penjualan
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- C. SoD defence-in-depth trigger
-- ====================================================================

CREATE OR REPLACE FUNCTION trg_penjualan_sod_check_fn()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.approver_id IS NOT NULL AND NEW.approver_id = NEW.maker_id THEN
        RAISE EXCEPTION 'SOD_VIOLATION: approver_id % equals maker_id % for penjualan %',
            NEW.approver_id, NEW.maker_id, NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER trg_penjualan_sod_check
    BEFORE INSERT OR UPDATE ON trx.penjualan
    FOR EACH ROW EXECUTE FUNCTION trg_penjualan_sod_check_fn();

-- ====================================================================
-- D. Seed mst.mapping_jurnal placeholders for PENJUALAN_* event codes
--    (DRAFT status — akun GL to be filled by ROLE-AKUN via APP-D workflow)
-- ====================================================================

INSERT INTO mst.mapping_jurnal (
    event_code, klasifikasi_psak71, deskripsi, status,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
)
SELECT event_code, klasifikasi, deskripsi, 'DRAFT'::text,
       now(), '00000000-0000-0000-0000-000000000000'::uuid,
       now(), '00000000-0000-0000-0000-000000000000'::uuid,
       1, 'TUGURE'
FROM (VALUES
    ('PENJUALAN_AC',             'AC',             'Penjualan instrumen Amortised Cost — Dr Kas/Cr Aset AC/Dr|Cr Realized G/L P&L'),
    ('PENJUALAN_FVOCI_DEBT',     'FVOCI',          'Penjualan instrumen FVOCI debt — Dr Kas/Cr Aset FVOCI; lihat juga REKLAS_OCI_PL'),
    ('PENJUALAN_FVOCI_ELECTION', 'FVOCI_ELECTION', 'Penjualan FVOCI Election — Dr Kas/Cr Aset/Cr OCI Reserve; G/L stays in equity (§B5.7.1)'),
    ('PENJUALAN_FVTPL',          'FVTPL',          'Penjualan instrumen FVTPL — Dr Kas/Cr Aset FVTPL/Dr|Cr Realized G/L P&L'),
    ('PENJUALAN_POCI',           'POCI',           'Penjualan instrumen POCI — credit-adjusted derecognition'),
    ('REKLAS_OCI_PL',            'FVOCI',          'Reklasifikasi OCI ke P&L saat FVOCI debt disposal — Dr|Cr OCI Reserve / Cr|Dr Realized G/L')
) AS t(event_code, klasifikasi, deskripsi)
ON CONFLICT (event_code) DO NOTHING;

-- ====================================================================
-- E. Seed sys.config_param: BM frequency thresholds
-- ====================================================================

INSERT INTO sys.config_param (key, value, deskripsi, created_at, created_by, updated_at, updated_by, row_version, tenant_id)
VALUES
    ('PENJUALAN_BM_WARN_THRESHOLD_PCT',  '5.0',  'BM HTC warning threshold — cumulative disposal 12m > X% dari total portofolio HTC → flag + notif ROLE-RISK. Default 5%. Dapat dioverride ALCO.',
     now(), '00000000-0000-0000-0000-000000000000'::uuid, now(), '00000000-0000-0000-0000-000000000000'::uuid, 1, 'TUGURE'),
    ('PENJUALAN_BM_BLOCK_THRESHOLD_PCT', '10.0', 'BM HTC block threshold — cumulative disposal 12m > X% → penjualan masuk PENDING_BM_REVIEW, requires ROLE-RISK approval. Default 10%. Dapat dioverride ALCO.',
     now(), '00000000-0000-0000-0000-000000000000'::uuid, now(), '00000000-0000-0000-0000-000000000000'::uuid, 1, 'TUGURE')
ON CONFLICT (key) DO NOTHING;

COMMIT;
