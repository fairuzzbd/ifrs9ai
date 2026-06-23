-- migration: 0040 mtm_p5m6
-- author: data-modeler (driven by system-analyst P5-M6)
-- requires: 0001 (init_schema — sys.upload_batch, sec.user, fn_update_updated_at,
--                              fn_increment_row_version, trg_set_updated_at pattern),
--           0020 (kurs_schema_fix — confirms sys.config table stable),
--           0039 (fx_rate_p5m5 — sys.holiday_calendar available; mst.kurs locked pattern)
-- description:
--   P5-M6 MTM Daily Job:
--   (A) CREATE TABLE trx.mtm — Mark-to-Market harian per instrumen FVOCI/FVTPL/POCI.
--       Partitioned monthly by tanggal_mtm (RANGE). Audit columns, CHECK constraints,
--       SoD constraint, locked_flag, partial unique index, indexes.
--   (B) DB TRIGGER tg_mtm_locked_check — defence-in-depth for locked_flag (mirror P5-M5).
--   (C) DB TRIGGER trg_mtm_updated_at + trg_mtm_row_version — standard audit triggers.
--   (D) FK constraint fk_mtm_upload_batch → sys.upload_batch (table exists since 0001).
--   (E) sys.config_param seed — MTM threshold + cron config keys.
--   (F) mst.mapping_jurnal event code placeholder seed — 5 MTM event codes (DRAFT).

BEGIN;

-- ====================================================================
-- A. CREATE TABLE trx.mtm — MTM harian per instrumen
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.mtm (
    -- ── Primary key ──────────────────────────────────────────────────
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- ── Business keys ────────────────────────────────────────────────
    instrumen_id            UUID            NOT NULL,
    -- ^ FK mst.instrumen(id) — added as named constraint below (post-partition).

    periode_bulanan_id      UUID            NOT NULL,
    -- ^ FK mst.periode_buku(id) — period this MTM belongs to.

    tanggal_mtm             DATE            NOT NULL,
    -- ^ Business date of valuation. Partition range key.
    --   NOTE: Partitioning is by tanggal_mtm (business date) for IFRS9 compliance.
    --   created_at is the insert time (may differ if manual upload after-close corrective).

    -- ── Price data ───────────────────────────────────────────────────
    harga_sumber            VARCHAR(30)     NOT NULL,
    -- ^ Feed source: 'IBPA' | 'BEI' | 'KSEI' | 'MANUAL' | 'IBPA_MANUAL' | 'BEI_MANUAL'.

    harga_tanggal           DATE            NOT NULL,
    -- ^ Date of the price used (may differ from tanggal_mtm if stale).

    harga_age_days          SMALLINT        NOT NULL,
    -- ^ tanggal_mtm − harga_tanggal. Computed at INSERT by service layer.
    --   DEC-016: compute via shopspring/decimal-aware date diff (integer days).

    harga_pasar_fcy         NUMERIC(20,8),
    -- ^ Market price in instrument native currency (NULL if IDR instrument).
    --   DEC-016: NUMERIC(20,8) for FX-denominated prices.

    harga_pasar_idr         NUMERIC(20,4)   NOT NULL,
    -- ^ Market price converted to IDR. DEC-016: NUMERIC(20,4) for IDR amounts.
    --   For IDR instruments: harga_pasar_idr = harga_pasar directly.
    --   For FCY instruments: harga_pasar_idr = harga_pasar_fcy × kurs_tengah.

    harga_buku_idr          NUMERIC(20,4)   NOT NULL,
    -- ^ Book value in IDR at time of MTM. Sourced from trx.penempatan current saldo.
    --   For Stage 3 FVTPL: must be Net Carrying (Gross − ECL provisioned).
    --   ecl-eir-engineer to confirm correct column from trx.penempatan.

    delta_idr               NUMERIC(20,4)   NOT NULL,
    -- ^ harga_pasar_idr − harga_buku_idr. Positive = unrealized gain; negative = loss.

    delta_pct               NUMERIC(7,4)    NOT NULL,
    -- ^ (delta_idr / harga_buku_idr) × 100. Stored for deviation check and alerting.

    -- ── FX ───────────────────────────────────────────────────────────
    kurs_id                 UUID,
    -- ^ FK mst.kurs(id). NULL if IDR instrument.

    kurs_tengah             NUMERIC(20,8),
    -- ^ Snapshot kurs_tengah at tanggal_mtm. NUMERIC(20,8) per DEC-016.
    --   NULL for IDR instruments.

    -- ── Classification snapshot ──────────────────────────────────────
    klasifikasi_snapshot    VARCHAR(30)     NOT NULL,
    -- ^ Snapshot of mst.instrumen.klasifikasi_psak71 at MTM time.
    --   Values: 'FVOCI_DEBT' | 'FVTPL' | 'FVOCI_ELECTION' | 'POCI'.
    --   AC instruments are never inserted (resolveJurnalEventCode returns ErrMTMInstrumenACSkip).

    treatment_snapshot      VARCHAR(60),
    -- ^ Snapshot of FX treatment routing from P5-M5 GET /treatment/{instrumen_id}.
    --   e.g. 'OCI_FOREIGN_EXCHANGE_RESERVE', 'P&L_FOREIGN_EXCHANGE', 'NO_FX_TREATMENT'.
    --   NULL for IDR-only instruments or if treatment query skipped (non-FCY).

    -- ── Jurnal linkage ───────────────────────────────────────────────
    jurnal_entry_id         UUID,
    -- ^ FK jrnl.jurnal_entry(id). NULL until jurnal posted.
    --   For FVOCI_DEBT FCY: primary jurnal_entry_id (MTM_FVOCI).
    --   Second entry (MTM_FX_OCI_RESERVE) stored in jurnal_entry_id_2.

    jurnal_entry_id_2       UUID,
    -- ^ Secondary jurnal_entry_id for FVOCI_DEBT FCY dual-entry (MTM_FX_OCI_RESERVE).
    --   NULL for all other classifications.

    jurnal_event_code       VARCHAR(50),
    -- ^ Primary event code posted. One of: MTM_FVOCI, MTM_FVTPL, MTM_FVOCI_ELECTION, MTM_FVTPL_POCI.
    --   For FVOCI_DEBT FCY: 'MTM_FVOCI' (primary); MTM_FX_OCI_RESERVE in jurnal_event_code_2.

    jurnal_event_code_2     VARCHAR(50),
    -- ^ Secondary event code. 'MTM_FX_OCI_RESERVE' for FVOCI_DEBT FCY dual-entry.
    --   NULL for all other classifications.

    -- ── Flags ────────────────────────────────────────────────────────
    stale_price_flag        BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE if harga_age_days > sys.config MTM_PRICE_STALE_DAYS (default 5)
    --   OR kurs FCY not available (KURS_FCY_TIDAK_TERSEDIA).

    deviation_flag          BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE if ABS(delta_pct) > sys.config MTM_PRICE_DEVIATION_THRESHOLD_PCT (default 5.0%).

    locked_flag             BOOLEAN         NOT NULL DEFAULT FALSE,
    -- ^ TRUE after periode hard-close (P5-M4 hook LockMtmForPeriode).
    --   When TRUE: UPDATE and DELETE refused (app layer MtmLockMiddleware + DB trigger).

    -- ── Workflow / status ────────────────────────────────────────────
    status                  VARCHAR(20)     NOT NULL,
    -- ^ Workflow state: AUTO_POSTED | PENDING_REVIEW | APPROVED | REJECTED | STALE_PRICE.
    --   See state machine doc: docs/state-machines/p5-m6-mtm-daily.md.

    -- ── Upload / cron linkage ────────────────────────────────────────
    upload_batch_id         UUID,
    -- ^ FK sys.upload_batch(id) (exists since migration 000001).
    --   NULL for cron auto-generated rows. Non-null for manual upload rows.
    --   batch_type = 'MTM_UPLOAD' in sys.upload_batch (matching existing CHECK constraint).

    uploader_id             UUID,
    -- ^ FK sec.user(id). NULL for cron auto rows. Set for manual upload rows.
    --   SoD: uploader_id ≠ override_approver_id (enforced by chk_mtm_sod + service layer).

    cron_job_id             TEXT,
    -- ^ Asynq job ID of the cron run that generated this row.
    --   Format: 'job_MTM_CRON_<ULID>'. NULL for manual upload rows.

    -- ── Override ──────────────────────────────────────────────────────
    override_approver_id    UUID,
    -- ^ FK sec.user(id). Set when ROLE-AKUN-CTL override-approve or override-reject.
    --   SoD enforced: override_approver_id ≠ uploader_id.

    override_comment        TEXT,
    -- ^ Komentar override. Wajib ≥ 30 char when status='REJECTED' (chk_mtm_override_comment).
    --   Wajib ada (any length) when override_approver_id is set.

    override_at             TIMESTAMPTZ,
    -- ^ Timestamp of override action. Set atomically with status update.

    -- ── Standard audit columns (db-conventions.md) ─────────────────
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ── CHECK CONSTRAINTS ────────────────────────────────────────────

    CONSTRAINT chk_mtm_status
        CHECK (status IN ('AUTO_POSTED', 'PENDING_REVIEW', 'APPROVED', 'REJECTED', 'STALE_PRICE')),

    CONSTRAINT chk_mtm_harga_sumber
        CHECK (harga_sumber IN ('IBPA', 'BEI', 'KSEI', 'MANUAL', 'IBPA_MANUAL', 'BEI_MANUAL')),

    CONSTRAINT chk_mtm_klasifikasi_snapshot
        CHECK (klasifikasi_snapshot IN ('FVOCI_DEBT', 'FVTPL', 'FVOCI_ELECTION', 'POCI')),
    -- ^ AC is excluded at service layer; constraint is defence-in-depth.

    CONSTRAINT chk_mtm_override_comment
        CHECK (
            status != 'REJECTED'
            OR (override_comment IS NOT NULL AND length(override_comment) >= 30)
        ),
    -- ^ When status=REJECTED, override_comment must be present and ≥ 30 characters.
    --   Mirrors P5-M5 reject_reason pattern for mst.kurs.

    CONSTRAINT chk_mtm_sod
        CHECK (
            override_approver_id IS NULL
            OR uploader_id IS NULL
            OR override_approver_id != uploader_id
        ),
    -- ^ Segregation of Duties: override_approver_id ≠ uploader_id.
    --   Primary enforcement at service layer (before DB write). This is defence-in-depth.
    --   DEC-017.

    CONSTRAINT chk_mtm_harga_pasar_idr_positive
        CHECK (harga_pasar_idr > 0),
    -- ^ Sanity: market price in IDR must be positive.

    CONSTRAINT chk_mtm_harga_buku_idr_positive
        CHECK (harga_buku_idr > 0),
    -- ^ Book value must be positive.

    CONSTRAINT chk_mtm_harga_age_nonneg
        CHECK (harga_age_days >= 0)
    -- ^ Age in days must be non-negative.

) PARTITION BY RANGE (tanggal_mtm);
-- ^ Partition strategy: RANGE by tanggal_mtm (business date) monthly.
--   Partition naming: trx_mtm_y2026m06, trx_mtm_y2026m07, etc.
--   Managed via pg_partman or manual CREATE TABLE ... PARTITION OF.
--   Initial partitions for 2026: created below.

COMMENT ON TABLE trx.mtm IS
    'MTM (Mark-to-Market) harian per instrumen FVOCI/FVTPL/POCI. '
    'Partitioned monthly by tanggal_mtm. '
    'Populated by Asynq cron "trx:mtm_daily_run" (18:00 WIB Senin-Jumat) '
    'and manual upload via POST /trx/mtm/upload/batch (ROLE-AKUN Maker). '
    'Status workflow: AUTO_POSTED | PENDING_REVIEW | APPROVED | REJECTED | STALE_PRICE. '
    'State machine: docs/state-machines/p5-m6-mtm-daily.md. '
    'AC instruments are NEVER inserted (resolveJurnalEventCode → ErrMTMInstrumenACSkip). '
    'P5-M6 migration 000040.';

COMMENT ON COLUMN trx.mtm.harga_pasar_fcy IS
    'Market price in instrument native FCY. NUMERIC(20,8) per DEC-016. '
    'NULL for IDR instruments. For FCY: harga_pasar_idr = harga_pasar_fcy × kurs_tengah.';

COMMENT ON COLUMN trx.mtm.harga_buku_idr IS
    'Book value in IDR at MTM time. For Stage 3 FVTPL = Net Carrying (Gross − ECL). '
    'ecl-eir-engineer must confirm source column from trx.penempatan (OQ-M6-6). '
    'NUMERIC(20,4) per DEC-016.';

COMMENT ON COLUMN trx.mtm.jurnal_entry_id IS
    'FK jrnl.jurnal_entry(id). Primary jurnal entry posted. '
    'For FVOCI_DEBT FCY: MTM_FVOCI entry. Second entry (MTM_FX_OCI_RESERVE) in jurnal_entry_id_2. '
    'NULL until status=AUTO_POSTED or APPROVED.';

COMMENT ON COLUMN trx.mtm.jurnal_entry_id_2 IS
    'Secondary jurnal entry for FVOCI_DEBT FCY dual-entry per §B5.7.2A. '
    'Stores MTM_FX_OCI_RESERVE entry_id. NULL for all other classifications.';

COMMENT ON COLUMN trx.mtm.locked_flag IS
    'TRUE after periode hard-close (P5-M4 LockMtmForPeriode hook, same transaction). '
    'When TRUE: MtmLockMiddleware (app) + tg_mtm_locked_check (DB) block UPDATE/DELETE. '
    'Mirror: mst.kurs.locked_flag pattern from P5-M5.';

COMMENT ON COLUMN trx.mtm.upload_batch_id IS
    'FK sys.upload_batch(id). NULL for cron auto rows. '
    'batch_type=''MTM_UPLOAD'' in sys.upload_batch (consistent with existing ck_batch_type constraint). '
    'FK constraint fk_mtm_upload_batch added below.';

COMMENT ON CONSTRAINT chk_mtm_sod ON trx.mtm IS
    'Defence-in-depth SoD: override_approver_id ≠ uploader_id. '
    'Primary enforcement is at service layer (service/mtm/service.go) before DB write. '
    'DEC-017 locked decision.';

-- ====================================================================
-- A1. Monthly partitions for 2026 (months MTM data expected)
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m01
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m02
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m03
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m04
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m05
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m06
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m07
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m08
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m09
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m10
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m11
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2026m12
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

CREATE TABLE IF NOT EXISTS trx.mtm_y2027m01
    PARTITION OF trx.mtm
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

-- Default partition catches any future or out-of-range tanggal_mtm.
CREATE TABLE IF NOT EXISTS trx.mtm_default
    PARTITION OF trx.mtm DEFAULT;

-- ====================================================================
-- A2. FK constraints on partitioned table
--     (FK on partitioned tables requires all partitions to exist first)
-- ====================================================================

ALTER TABLE trx.mtm
    ADD CONSTRAINT fk_mtm_instrumen
        FOREIGN KEY (instrumen_id) REFERENCES mst.instrumen(id) ON DELETE RESTRICT;

ALTER TABLE trx.mtm
    ADD CONSTRAINT fk_mtm_kurs
        FOREIGN KEY (kurs_id) REFERENCES mst.kurs(id) ON DELETE RESTRICT;

ALTER TABLE trx.mtm
    ADD CONSTRAINT fk_mtm_uploader
        FOREIGN KEY (uploader_id) REFERENCES sec.user(id) ON DELETE RESTRICT;

ALTER TABLE trx.mtm
    ADD CONSTRAINT fk_mtm_override_approver
        FOREIGN KEY (override_approver_id) REFERENCES sec.user(id) ON DELETE RESTRICT;

-- Upload batch FK: sys.upload_batch exists since migration 000001.
ALTER TABLE trx.mtm
    ADD CONSTRAINT fk_mtm_upload_batch
        FOREIGN KEY (upload_batch_id) REFERENCES sys.upload_batch(id) ON DELETE RESTRICT;

-- ====================================================================
-- A3. Indexes
-- ====================================================================

-- Primary lookup: per instrumen per tanggal (most common query in MTM worker + detail page)
CREATE INDEX IF NOT EXISTS idx_mtm_instrumen_tanggal
    ON trx.mtm (instrumen_id, tanggal_mtm DESC)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_instrumen_tanggal IS
    'Primary lookup: instrumen history + idempotency check in cron worker. '
    'Covers: SELECT id FROM trx.mtm WHERE instrumen_id=? AND tanggal_mtm=? AND ...';

-- Status filter: antrian PENDING_REVIEW + STALE_PRICE for ROLE-AKUN-CTL dashboard
CREATE INDEX IF NOT EXISTS idx_mtm_status
    ON trx.mtm (status, tanggal_mtm DESC)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_status IS
    'DataTable filter[status]=PENDING_REVIEW,STALE_PRICE for ROLE-AKUN-CTL antrian.';

-- Stale price flag: GET /alerts/stale-price endpoint
CREATE INDEX IF NOT EXISTS idx_mtm_stale_flag
    ON trx.mtm (stale_price_flag, tanggal_mtm DESC)
    WHERE stale_price_flag = TRUE AND deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_stale_flag IS
    'Partial index: GET /trx/mtm/alerts/stale-price — only stale_price_flag=TRUE rows.';

-- Deviation flag: filter deviation warnings
CREATE INDEX IF NOT EXISTS idx_mtm_deviation_flag
    ON trx.mtm (deviation_flag, tanggal_mtm DESC)
    WHERE deviation_flag = TRUE AND deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_deviation_flag IS
    'Partial index: GET /trx/mtm?filter[deviation_flag]=true — deviation monitoring.';

-- Periode lookup: list by periode_bulanan_id
CREATE INDEX IF NOT EXISTS idx_mtm_periode
    ON trx.mtm (periode_bulanan_id, tanggal_mtm DESC)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_periode IS
    'Lookup by periode_bulanan_id for P5-M4 lock cascade and monthly reports.';

-- Upload batch lookup: GET /trx/mtm/upload/batch/{id}
CREATE INDEX IF NOT EXISTS idx_mtm_upload_batch_id
    ON trx.mtm (upload_batch_id)
    WHERE upload_batch_id IS NOT NULL AND deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_upload_batch_id IS
    'Batch-level queries: GET /trx/mtm/upload/batch/{batch_id} breakdown.';

-- Locked flag: P5-M4 LockMtmForPeriode UPDATE scan
CREATE INDEX IF NOT EXISTS idx_mtm_locked_flag
    ON trx.mtm (locked_flag, tanggal_mtm)
    WHERE locked_flag = TRUE AND deleted_at IS NULL;

COMMENT ON INDEX idx_mtm_locked_flag IS
    'Scan for locked rows during reopen (UnlockMtmForPeriode). '
    'Also used by MtmLockMiddleware pre-check (reads single row via PK, but useful for bulk ops).';

-- Partial unique: only ONE AUTO_POSTED or APPROVED row per (instrumen_id, tanggal_mtm, harga_sumber)
-- REJECTED rows are excluded (can have multiple REJECTED rows for same combo)
CREATE UNIQUE INDEX IF NOT EXISTS uq_mtm_active_per_instrumen_tanggal_sumber
    ON trx.mtm (instrumen_id, tanggal_mtm, harga_sumber)
    WHERE status IN ('AUTO_POSTED', 'APPROVED') AND deleted_at IS NULL;

COMMENT ON INDEX uq_mtm_active_per_instrumen_tanggal_sumber IS
    'Business rule: at most ONE AUTO_POSTED or APPROVED MTM per (instrumen, tanggal, sumber). '
    'REJECTED rows allowed to accumulate for audit trail. '
    'PENDING_REVIEW and STALE_PRICE not blocked by this index '
    '(multiple pending possible from different sources before resolution).';

-- Tenant + created_at for hot queries with tenant scope
CREATE INDEX IF NOT EXISTS idx_mtm_tenant_created
    ON trx.mtm (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- ====================================================================
-- B. DB TRIGGER: tg_mtm_locked_check (BEFORE UPDATE OR DELETE)
--    Defence-in-depth: prevent mutation of locked MTM rows at DB layer.
--    Mirror of fn_kurs_locked_check from migration 000039.
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_mtm_locked_check()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.locked_flag = TRUE THEN
        RAISE EXCEPTION
            'trx.mtm row % is locked (locked_flag=TRUE). '
            'The associated periode_buku is hard-closed. UPDATE/DELETE not permitted. '
            'Error code: MTM_PERIODE_LOCKED',
            OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_mtm_locked_check() IS
    'Defence-in-depth trigger: prevents UPDATE or DELETE on any trx.mtm row '
    'where locked_flag = TRUE (i.e. periode_buku is hard-closed). '
    'Primary enforcement is MtmLockMiddleware at app layer (P5-M6 backend). '
    'Error code: MTM_PERIODE_LOCKED (HTTP 423). '
    'Mirror of fn_kurs_locked_check (P5-M5 migration 000039). '
    'P5-M6 migration 000040.';

DROP TRIGGER IF EXISTS tg_mtm_locked_check ON trx.mtm;

CREATE TRIGGER tg_mtm_locked_check
    BEFORE UPDATE OR DELETE ON trx.mtm
    FOR EACH ROW EXECUTE FUNCTION fn_mtm_locked_check();

COMMENT ON TRIGGER tg_mtm_locked_check ON trx.mtm IS
    'Enforces locked_flag immutability at DB layer. '
    'Applied to parent table — PostgreSQL propagates to all partitions. '
    'DEC-017: SoD + immutability after close.';

-- ====================================================================
-- C. Standard audit triggers for trx.mtm
-- ====================================================================

-- updated_at trigger (uses fn_update_updated_at from migration 000001)
DROP TRIGGER IF EXISTS trg_mtm_updated_at ON trx.mtm;
CREATE TRIGGER trg_mtm_updated_at
    BEFORE UPDATE ON trx.mtm
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

-- row_version trigger (uses fn_increment_row_version from migration 000001)
DROP TRIGGER IF EXISTS trg_mtm_row_version ON trx.mtm;
CREATE TRIGGER trg_mtm_row_version
    BEFORE UPDATE ON trx.mtm
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

COMMENT ON TRIGGER trg_mtm_updated_at ON trx.mtm IS
    'Auto-update updated_at on every UPDATE. Standard audit col. DEC-018.';

COMMENT ON TRIGGER trg_mtm_row_version ON trx.mtm IS
    'Increment row_version on every UPDATE for optimistic locking. DEC-018.';

-- ====================================================================
-- E. sys.config_param seed — MTM threshold + cron config keys
-- ====================================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES

(
    'MTM_PRICE_DEVIATION_THRESHOLD_PCT',
    '5.0',
    'DECIMAL',
    FALSE,
    'Threshold persentase deviasi harga MTM dari harga buku. '
    'Jika ABS(delta_pct) > nilai ini: deviation_flag=TRUE, status=PENDING_REVIEW. '
    'Default 5.0 (5%). ALCO atau ROLE-IT-ADMIN dapat mengubah. '
    'Range valid: [0.01, 100.0]. '
    'Efektif pada MTM cron run berikutnya (dibaca per run). '
    'P5-M6 migration 000040.',
    'MTM'
),

(
    'MTM_PRICE_STALE_DAYS',
    '5',
    'INTEGER',
    FALSE,
    'Threshold hari kedaluwarsa harga MTM. '
    'Jika harga_age_days > nilai ini: stale_price_flag=TRUE, status=STALE_PRICE. '
    'Default 5 hari. Applicable juga jika kurs FCY tidak tersedia. '
    'Range valid: [1, 30]. '
    'P5-M6 migration 000040.',
    'MTM'
),

(
    'MTM_STALE_ESCALATION_DAYS',
    '7',
    'INTEGER',
    FALSE,
    'Threshold hari untuk eskalasi STALE_PRICE ke ROLE-RISK. '
    'Jika harga_age_days > nilai ini: notifikasi tambahan dikirim ke ROLE-RISK. '
    'Default 7 hari (harus > MTM_PRICE_STALE_DAYS). '
    'Range valid: [MTM_PRICE_STALE_DAYS+1, 60]. '
    'P5-M6 migration 000040.',
    'MTM'
),

(
    'MTM_CRON_SCHEDULE',
    '0 11 * * 1-5',
    'STRING',
    FALSE,
    'Asynq cron schedule untuk MTM daily run. '
    'Format: standard cron (minute hour dom month dow). '
    'Default: 0 11 * * 1-5 = 18:00 WIB (11:00 UTC), Senin-Jumat. '
    'Catatan: timezone cron diset oleh TZ=Asia/Jakarta di container (bukan UTC offset). '
    'Restart worker diperlukan setelah perubahan schedule ini. '
    'P5-M6 migration 000040.',
    'MTM'
)

ON CONFLICT (config_key) DO NOTHING;

-- ====================================================================
-- F. mst.mapping_jurnal event code placeholder seed (5 MTM event codes)
--    Status DRAFT — akun debit/kredit kosong, diisi via P5-M2 workflow.
--    Requires mst.mapping_jurnal table to exist (migration 000001 or earlier).
-- ====================================================================

-- Only seed if mst.mapping_jurnal table exists (graceful: P5-M2 may have been applied).
-- Use DO $$ block to conditionally insert.
DO $$
DECLARE
    v_table_exists BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'mst'
          AND table_name   = 'mapping_jurnal'
    ) INTO v_table_exists;

    IF v_table_exists THEN

        -- Seed 5 MTM event codes as DRAFT placeholder rows.
        -- akun_debit and akun_kredit are empty/NULL — to be filled via P5-M2 jurnal engine workflow.
        -- ON CONFLICT: skip if event_code already exists (idempotent).

        INSERT INTO mst.mapping_jurnal (event_code, deskripsi, klasifikasi_psak71, status,
                                        created_by, updated_by, tenant_id)
        VALUES
            ('MTM_FVOCI',
             'MTM delta ke OCI — instrumen FVOCI_DEBT IDR. Perubahan nilai wajar ke OCI Perubahan Nilai Wajar. PSAK 71 §5.7.10.',
             'FVOCI_DEBT', 'DRAFT',
             '00000000-0000-0000-0000-000000000001'::UUID,
             '00000000-0000-0000-0000-000000000001'::UUID,
             'TUGURE'),

            ('MTM_FX_OCI_RESERVE',
             'MTM FX component ke OCI FX Reserve — instrumen FVOCI_DEBT FCY (second entry, §B5.7.2A). '
             'FX gain/loss ke OCI Reserve (bukan P&L). Diposting bersama MTM_FVOCI untuk instrumen FCY.',
             'FVOCI_DEBT', 'DRAFT',
             '00000000-0000-0000-0000-000000000001'::UUID,
             '00000000-0000-0000-0000-000000000001'::UUID,
             'TUGURE'),

            ('MTM_FVOCI_ELECTION',
             'MTM ke OCI Ekuitas — instrumen FVOCI_ELECTION. Tidak ada P&L recycling on disposal (irrevocable §5.7.5).',
             'FVOCI_ELECTION', 'DRAFT',
             '00000000-0000-0000-0000-000000000001'::UUID,
             '00000000-0000-0000-0000-000000000001'::UUID,
             'TUGURE'),

            ('MTM_FVTPL',
             'MTM ke P&L — instrumen FVTPL. Semua perubahan fair value (termasuk FX) ke P&L §5.7.7.',
             'FVTPL', 'DRAFT',
             '00000000-0000-0000-0000-000000000001'::UUID,
             '00000000-0000-0000-0000-000000000001'::UUID,
             'TUGURE'),

            ('MTM_FVTPL_POCI',
             'MTM ke P&L — instrumen POCI. Credit-adjusted; tidak ada Stage escalation dari MTM row. '
             'ECL dari APP-C independent dari MTM ini (tidak double-count).',
             'POCI', 'DRAFT',
             '00000000-0000-0000-0000-000000000001'::UUID,
             '00000000-0000-0000-0000-000000000001'::UUID,
             'TUGURE')

        ON CONFLICT (event_code) DO NOTHING;

        RAISE NOTICE 'mst.mapping_jurnal: 5 MTM event code placeholder rows seeded (or skipped if existing).';

    ELSE
        RAISE NOTICE 'mst.mapping_jurnal table not found — skipping MTM event code seed. '
                     'Seed manually after P5-M2 mst.mapping_jurnal table is created.';
    END IF;
END $$;

COMMIT;
