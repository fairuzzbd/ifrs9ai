-- migration: 0039 fx_rate_p5m5
-- author: data-modeler
-- requires: 0001 (init_schema — mst.kurs, sys.config, sec.user FK targets,
--                              fn_update_updated_at, fn_increment_row_version),
--           0020 (kurs_schema_fix — workflow_status, audit cols, tenant_id),
--           0038 (periode_close_p5m4 — sys.config table stable)
-- description:
--   P5-M5 FX Rate Management:
--   (A) ALTER mst.kurs — precision upgrade NUMERIC(15,4)→NUMERIC(20,8) for
--       kurs_tengah, kurs_beli, kurs_jual; add deviation_flag, rate_deviation_pct,
--       jisdor_fetch_metadata, reject_reason, upload_batch_id; add partial unique
--       index for ACTIVE workflow rows; add upload_batch_id column.
--   (B) Upgrade CHECK constraint on workflow_status to accept PENDING_APPROVAL
--       and APPROVED as primary states (P5-M5 lifecycle simplification).
--   (C) CREATE TABLE sys.holiday_calendar — Indonesia public holiday calendar.
--   (D) CREATE TABLE sys.dlq_fx_jisdor — JISDOR fetch dead letter queue.
--   (E) sys.config seed — 5 FX/JISDOR config keys.
--   (F) Verify / add DB trigger tg_kurs_locked_check (BEFORE UPDATE OR DELETE).

BEGIN;

-- ====================================================================
-- A. ALTER mst.kurs — P5-M5 new columns + precision upgrade
-- ====================================================================

-- A1. Precision upgrade: NUMERIC(15,4) → NUMERIC(20,8) per DEC-016.
--     USING clause handles existing data; no data loss (only widening precision).

ALTER TABLE mst.kurs
    ALTER COLUMN kurs_tengah TYPE NUMERIC(20,8)
        USING kurs_tengah::NUMERIC(20,8),
    ALTER COLUMN kurs_beli   TYPE NUMERIC(20,8)
        USING kurs_beli::NUMERIC(20,8),
    ALTER COLUMN kurs_jual   TYPE NUMERIC(20,8)
        USING kurs_jual::NUMERIC(20,8);

COMMENT ON COLUMN mst.kurs.kurs_tengah IS
    'Mid rate (kurs tengah). NUMERIC(20,8) per DEC-016 FX precision (P5-M5 upgrade from 15,4). '
    'shopspring/decimal at service layer. NOT NULL.';

COMMENT ON COLUMN mst.kurs.kurs_beli IS
    'Buy rate (kurs beli). NUMERIC(20,8) per DEC-016 (P5-M5). NULL for JISDOR auto-fetch if unavailable.';

COMMENT ON COLUMN mst.kurs.kurs_jual IS
    'Sell rate (kurs jual). NUMERIC(20,8) per DEC-016 (P5-M5). NULL for JISDOR auto-fetch if unavailable.';

-- A2. New columns for P5-M5 FX workflow.

ALTER TABLE mst.kurs
    ADD COLUMN IF NOT EXISTS deviation_flag          BOOLEAN         NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS rate_deviation_pct      NUMERIC(7,4),
    ADD COLUMN IF NOT EXISTS jisdor_fetch_metadata   JSONB,
    ADD COLUMN IF NOT EXISTS reject_reason           TEXT,
    ADD COLUMN IF NOT EXISTS upload_batch_id         UUID;
-- Note: upload_batch_id intentionally has no FK to sys.upload_batch here.
-- sys.upload_batch table is planned for P5-M11. FK will be added in migration 000049.
-- TODO(P5-M11): ADD CONSTRAINT fk_kurs_upload_batch
--               FOREIGN KEY (upload_batch_id) REFERENCES sys.upload_batch(id).

COMMENT ON COLUMN mst.kurs.deviation_flag IS
    'TRUE when abs(rate_deviation_pct) > FX_RATE_DEVIATION_THRESHOLD_PCT (default 20%). '
    'Set at INSERT by worker or manual upload service. '
    'Triggers human review requirement even if FX_JISDOR_AUTOAPPROVE=true.';

COMMENT ON COLUMN mst.kurs.rate_deviation_pct IS
    'Percentage deviation from prior business-day ACTIVE rate for same kode_mata_uang. '
    'Formula: (kurs_tengah_hari_ini - kurs_tengah_kemarin) / kurs_tengah_kemarin * 100. '
    'NULL if no prior ACTIVE rate exists (first occurrence). NUMERIC(7,4) = 4 decimal places.';

COMMENT ON COLUMN mst.kurs.jisdor_fetch_metadata IS
    'JSONB metadata for JISDOR auto-fetch: '
    '{url: str, fetched_at: timestamptz, http_status: int, response_hash: str, retry_count: int}. '
    'NULL for manual upload rows.';

COMMENT ON COLUMN mst.kurs.reject_reason IS
    'Rejection reason supplied by ROLE-AKUN-CTL approver. '
    'NOT NULL when workflow_status = ''REJECTED'' (enforced at application layer). '
    'Application enforces minimum length 20 characters.';

COMMENT ON COLUMN mst.kurs.upload_batch_id IS
    'Links this kurs row to a manual upload batch. '
    'NULL for JISDOR auto-fetch rows. '
    'FK to sys.upload_batch will be added in migration 000049 (P5-M11). '
    'Indexed below for batch-level approve/reject queries.';

-- A3. Update CHECK constraint on workflow_status to include P5-M5 states.
--     Drop old CHECK from migration 000020 (7-state enum) and replace with P5-M5 enum.
--     ACTIVE = APPROVED for backward compat; add PENDING_APPROVAL, REJECTED.
ALTER TABLE mst.kurs
    DROP CONSTRAINT IF EXISTS chk_kurs_workflow_status;

ALTER TABLE mst.kurs
    ADD CONSTRAINT chk_kurs_workflow_status
        CHECK (workflow_status IN (
            'DRAFT',
            'PENDING_REVIEW',
            'PENDING_APPROVAL',
            'PENDING_APPROVAL_2',
            'APPROVED',
            'ACTIVE',
            'REJECTED',
            'RETURNED'
        ));

COMMENT ON CONSTRAINT chk_kurs_workflow_status ON mst.kurs IS
    'P5-M5 workflow states: '
    'DRAFT (initial manual), PENDING_REVIEW, PENDING_APPROVAL (after maker submit / JISDOR fetch), '
    'PENDING_APPROVAL_2 (reserved for 6-eyes if ever required), '
    'APPROVED/ACTIVE (rate in use), REJECTED, RETURNED. '
    'JISDOR auto-approved rows use APPROVED directly. '
    'ACTIVE is synonym for APPROVED for legacy compatibility.';

-- A4. Partial unique index: only ONE active-and-not-deleted row per (kode_mata_uang, tanggal_berlaku).
--     Existing UNIQUE constraint (kode_mata_uang, tanggal_berlaku) from migration 000001 is kept
--     as the catch-all; this partial index adds business-semantics enforcement.
--     Name: idx_kurs_active_unique (referenced in state machine doc §2.2).
CREATE UNIQUE INDEX IF NOT EXISTS idx_kurs_active_unique
    ON mst.kurs (kode_mata_uang, tanggal_berlaku)
    WHERE workflow_status IN ('ACTIVE', 'APPROVED', 'PENDING_APPROVAL')
      AND deleted_at IS NULL;

COMMENT ON INDEX idx_kurs_active_unique IS
    'Enforces: at most one non-deleted ACTIVE/APPROVED/PENDING_APPROVAL row per '
    '(kode_mata_uang, tanggal_berlaku). '
    'Allows multiple REJECTED/RETURNED rows for audit history. '
    'Application raises KURS_DUPLICATE_DATE (409) on violation.';

-- A5. Index on upload_batch_id for batch-level approve/reject queries.
CREATE INDEX IF NOT EXISTS idx_kurs_upload_batch_id
    ON mst.kurs (upload_batch_id)
    WHERE upload_batch_id IS NOT NULL AND deleted_at IS NULL;

COMMENT ON INDEX idx_kurs_upload_batch_id IS
    'Batch-level queries: POST /master/kurs/upload/{batch_id}/approve reads all rows '
    'WHERE upload_batch_id = $1 AND workflow_status = ''PENDING_APPROVAL''. '
    'Partial (upload_batch_id IS NOT NULL) excludes JISDOR rows.';

-- A6. Index on deviation_flag for filtering warning rows.
CREATE INDEX IF NOT EXISTS idx_kurs_deviation_flag
    ON mst.kurs (deviation_flag, tanggal_berlaku DESC)
    WHERE deviation_flag = TRUE AND deleted_at IS NULL;

COMMENT ON INDEX idx_kurs_deviation_flag IS
    'Filter: GET /master/kurs?filter[deviation_flag]=true — monitoring dashboard for deviation warnings.';

-- ====================================================================
-- B. DB TRIGGER: tg_kurs_locked_check (BEFORE UPDATE OR DELETE)
--    Verify if trigger already exists from migration 000020 or init_schema.
--    If not, create it now.  Use CREATE OR REPLACE FUNCTION so it is idempotent.
-- ====================================================================

-- B1. Guard function — raises if row is locked.
CREATE OR REPLACE FUNCTION fn_kurs_locked_check()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.locked_flag = TRUE THEN
        RAISE EXCEPTION
            'mst.kurs row % is locked (locked_flag=TRUE). '
            'periode_buku is hard-closed. UPDATE/DELETE not permitted. '
            'Error code: FX_RATE_LOCKED',
            OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_kurs_locked_check() IS
    'Defence-in-depth trigger: prevents UPDATE or DELETE on any mst.kurs row '
    'where locked_flag = TRUE (i.e., periode_buku is hard-closed). '
    'Application layer also enforces this via FxRateLockMiddleware (P5-M5). '
    'Error code: FX_RATE_LOCKED (HTTP 423). '
    'P5-M5 — added/verified in migration 000039.';

-- B2. Trigger — BEFORE UPDATE OR DELETE on mst.kurs.
--     Drop + recreate to ensure correct definition regardless of whether it existed.
DROP TRIGGER IF EXISTS tg_kurs_locked_check ON mst.kurs;

CREATE TRIGGER tg_kurs_locked_check
    BEFORE UPDATE OR DELETE ON mst.kurs
    FOR EACH ROW EXECUTE FUNCTION fn_kurs_locked_check();

COMMENT ON TRIGGER tg_kurs_locked_check ON mst.kurs IS
    'Enforces locked_flag immutability at DB layer. '
    'Mirrors app-layer FxRateLockMiddleware (defence-in-depth per P5-M4 §6.3). '
    'Created in P5-M5 migration 000039; replaces any earlier fn_kurs_no_modify_when_locked.';

-- ====================================================================
-- C. CREATE TABLE sys.holiday_calendar
--    Indonesia public holiday + BI off-day calendar.
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.holiday_calendar (
    -- Primary key
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Business key
    tanggal         DATE            NOT NULL,
    -- ^ Calendar date of the holiday. UNIQUE enforced below.

    nama_libur      VARCHAR(200)    NOT NULL,
    -- ^ Human-readable holiday name (e.g. "Hari Raya Idul Fitri 1447H").

    sumber          TEXT            NOT NULL DEFAULT 'KEPRES',
    -- ^ Authority source: KEPRES (Keppres RI), BI (Bank Indonesia special), INTERNAL.

    -- Minimal audit cols (no updated_at/by — holiday records are effectively immutable
    -- once seeded; changes done via soft-delete + re-insert via ROLE-IT-ADMIN).
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by      UUID            NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::UUID,
    -- ^ System actor UUID for seeded rows; override at INSERT by ROLE-IT-ADMIN.
    tenant_id       TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================
    CONSTRAINT chk_holiday_sumber
        CHECK (sumber IN ('KEPRES', 'BI', 'INTERNAL'))
);

COMMENT ON TABLE sys.holiday_calendar IS
    'Indonesia national holidays and BI off-days. '
    'JISDOR cron worker (P5-M5) checks this table before fetching to skip non-business days. '
    'Saturday/Sunday handled by ISODOW check in worker (DOW 6,7 = Sat/Sun → skip). '
    'Maintained by ROLE-IT-ADMIN. Seed: 2026 Keppres libur nasional Indonesia.';

COMMENT ON COLUMN sys.holiday_calendar.tanggal IS
    'Date of the holiday. Unique: one entry per date.';

COMMENT ON COLUMN sys.holiday_calendar.sumber IS
    'Authority source: '
    'KEPRES — Keputusan Presiden (national holidays); '
    'BI — Bank Indonesia special calendar; '
    'INTERNAL — internal holiday override by ROLE-IT-ADMIN.';

-- C-UNIQUE
CREATE UNIQUE INDEX IF NOT EXISTS uq_holiday_calendar_tanggal
    ON sys.holiday_calendar (tanggal);

COMMENT ON INDEX uq_holiday_calendar_tanggal IS
    'One entry per date. Application raises CONFLICT if inserting a duplicate date.';

-- C-INDEX: Lookup by year for worker pre-load.
CREATE INDEX IF NOT EXISTS idx_holiday_calendar_year
    ON sys.holiday_calendar (EXTRACT(YEAR FROM tanggal)::INT, tanggal);

COMMENT ON INDEX idx_holiday_calendar_year IS
    'Worker pre-load: SELECT * FROM sys.holiday_calendar WHERE EXTRACT(YEAR FROM tanggal) = $1.';

-- C-SEED: 2026 Indonesia national holidays (Keppres + common BI off-days, best-effort).
-- Source: Keppres No. 18/2025 tentang Hari Libur Nasional 2026 + cuti bersama.
-- Extend via INSERT INTO sys.holiday_calendar (tanggal, nama_libur, sumber) by ROLE-IT-ADMIN.
INSERT INTO sys.holiday_calendar (tanggal, nama_libur, sumber, created_by)
VALUES
    ('2026-01-01', 'Tahun Baru Masehi 2026',             'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-01-27', 'Isra Miraj Nabi Muhammad SAW 1447H',  'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-01-29', 'Tahun Baru Imlek 2577',               'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-03-19', 'Hari Raya Nyepi Tahun Baru Saka 1948','KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-03-20', 'Cuti Bersama Hari Raya Nyepi',        'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-02', 'Hari Raya Idul Fitri 1447H (H-1)',    'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-03', 'Hari Raya Idul Fitri 1447H',          'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-06', 'Cuti Bersama Idul Fitri 1447H',       'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-07', 'Cuti Bersama Idul Fitri 1447H',       'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-08', 'Cuti Bersama Idul Fitri 1447H',       'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-04-09', 'Wafat Isa Al Masih',                  'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-05-01', 'Hari Buruh Internasional',             'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-05-14', 'Kenaikan Isa Al Masih',               'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-05-22', 'Hari Raya Waisak 2570 BE',            'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-06-01', 'Hari Lahir Pancasila',                 'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-06-10', 'Hari Raya Idul Adha 1447H',           'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-06-11', 'Cuti Bersama Idul Adha 1447H',        'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-07-01', 'Tahun Baru Islam 1448H',              'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-08-17', 'Hari Kemerdekaan Republik Indonesia',  'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-09-09', 'Maulid Nabi Muhammad SAW 1448H',      'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-12-24', 'Cuti Bersama Hari Natal',              'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-12-25', 'Hari Natal',                           'KEPRES', '00000000-0000-0000-0000-000000000001'),
    ('2026-12-31', 'Cuti Bersama Tahun Baru 2027',         'KEPRES', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (tanggal) DO NOTHING;

-- ====================================================================
-- D. CREATE TABLE sys.dlq_fx_jisdor
--    Dead Letter Queue for JISDOR fetch failures (mirror sys.dlq_gl_delivery pattern).
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.dlq_fx_jisdor (
    -- Primary key
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Context
    job_type            TEXT        NOT NULL DEFAULT 'jisdor_fetch',
    -- ^ Always 'jisdor_fetch' for JISDOR cron entries; 'jisdor_manual_sync' for API-triggered.

    tanggal_target      DATE        NOT NULL,
    -- ^ Which business date the fetch was attempted for.

    kode_mata_uang      CHAR(3),
    -- ^ Currency code if the failure was per-currency; NULL for pre-flight failures.

    -- Error info
    error_message       TEXT        NOT NULL,
    error_code          TEXT        NOT NULL DEFAULT 'JISDOR_FETCH_FAILED',
    -- ^ Stable error code: JISDOR_FETCH_FAILED, JISDOR_PARSE_ERROR, JISDOR_TIMEOUT.

    -- Retry tracking
    retry_count         INT         NOT NULL DEFAULT 0,
    last_attempt_at     TIMESTAMPTZ,

    -- DLQ lifecycle
    status              TEXT        NOT NULL DEFAULT 'FAILED',
    -- ^ FAILED → REPLAYED_OK (terminal), ABANDONED (terminal via IT-ADMIN discard).

    -- Payload snapshot
    payload_jsonb       JSONB,
    -- ^ HTTP request parameters at failure time (URL, headers without auth, date).

    -- Standard audit cols (db-conventions.md)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by          UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::UUID,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::UUID,
    deleted_at          TIMESTAMPTZ,
    deleted_by          UUID,
    row_version         BIGINT      NOT NULL DEFAULT 1,
    tenant_id           TEXT        NOT NULL DEFAULT 'TUGURE',

    -- CHECK CONSTRAINTS
    CONSTRAINT chk_dlq_jisdor_status
        CHECK (status IN ('FAILED', 'REPLAYED_OK', 'ABANDONED')),

    CONSTRAINT chk_dlq_jisdor_retry_nonneg
        CHECK (retry_count >= 0)
);

COMMENT ON TABLE sys.dlq_fx_jisdor IS
    'Dead Letter Queue for JISDOR fetch failures (P5-M5). '
    'INSERTed by worker after 3 retry exhausted. '
    'No hard delete (DEC-018). '
    'Accessible via GET /admin/dlq?filter[job_type]=jisdor_fetch (ROLE-IT-ADMIN only). '
    'Mirror of sys.dlq_gl_delivery pattern (migration 000037).';

-- D-TRIGGERS: no-hard-delete guard (DEC-018).
CREATE OR REPLACE FUNCTION fn_dlq_fx_jisdor_no_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.dlq_fx_jisdor is forbidden (DEC-018, retention 10+10 tahun). '
        'Archive to cold storage (MinIO) after retention window instead. '
        'Error code: DLQ_FX_NO_DELETE';
END;
$$;

CREATE OR REPLACE TRIGGER trg_dlq_fx_jisdor_no_delete
    BEFORE DELETE ON sys.dlq_fx_jisdor
    FOR EACH ROW EXECUTE FUNCTION fn_dlq_fx_jisdor_no_delete();

-- D-INDEX: Lookup for worker + admin.
CREATE INDEX IF NOT EXISTS idx_dlq_fx_jisdor_status_date
    ON sys.dlq_fx_jisdor (status, tanggal_target DESC)
    WHERE status = 'FAILED';

-- ====================================================================
-- E. sys.config seed — FX/JISDOR config keys
-- ====================================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES

(
    'FX_JISDOR_CURRENCIES',
    'USD,EUR,JPY,SGD,AUD,GBP',
    'STRING',
    FALSE,
    'Comma-separated list of ISO 4217 currency codes to fetch from BI JISDOR each business day. '
    'Worker reads this at runtime; change takes effect on next cron run. '
    'Extend by appending codes (e.g. add CNY after KSEI/BI data confirmed). '
    'Default: USD,EUR,JPY,SGD,AUD,GBP (the 6 major JISDOR-published currencies).',
    'FX'
),

(
    'FX_JISDOR_BASE_URL',
    '',
    'STRING',
    FALSE,
    'Base URL for BI JISDOR data endpoint. '
    'Empty string = integration-engineer adapter uses its hardcoded default '
    '(https://www.bi.go.id/en/statistik/informasi-kurs/jisdor-sbn/default.aspx). '
    'Override at runtime without restart by updating this key (read per cron run). '
    'Set to MOCK_RATES to activate MockAdapter for testing.',
    'FX'
),

(
    'FX_JISDOR_AUTOAPPROVE',
    'false',
    'BOOL',
    FALSE,
    'If true: JISDOR-fetched rates with deviation_flag=FALSE are inserted directly as APPROVED '
    '(bypassing 4-eyes manual approval). '
    'If false (safe default): all JISDOR rates are inserted as PENDING_APPROVAL and require '
    'ROLE-AKUN-CTL approval before use. '
    'deviation_flag=TRUE always requires manual approval regardless of this setting.',
    'FX'
),

(
    'FX_RATE_DEVIATION_THRESHOLD_PCT',
    '20.0',
    'DECIMAL',
    FALSE,
    'Percentage threshold for flagging kurs rate deviation from prior business-day. '
    'If abs(rate_deviation_pct) > this value → deviation_flag=TRUE, human review required. '
    'Default 20.0 (20%). ALCO can update this value; takes effect on next fetch. '
    'Range [0.1, 100.0] enforced at application layer.',
    'FX'
),

(
    'FX_JISDOR_CRON_SCHEDULE',
    '30 3 * * 1-5',
    'STRING',
    FALSE,
    'Asynq cron schedule for JISDOR daily fetch. '
    'Format: standard cron (minute hour dom month dow). '
    'Default: 30 3 * * 1-5 = 10:30 WIB (03:30 UTC), Mon–Fri. '
    'Registered in main.go Asynq scheduler; restart required after change.',
    'FX'
)

ON CONFLICT (config_key) DO NOTHING;

COMMIT;
