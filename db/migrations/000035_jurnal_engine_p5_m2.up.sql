-- migration: 0035 jurnal_engine_p5_m2
-- author: data-modeler
-- requires: 0001 (init_schema — sec.user, mst.instrumen, mst.periode_buku, mst.chart_of_accounts,
--                              jrnl.header, jrnl.detail, mst.mapping_jurnal_header,
--                              fn_update_updated_at, fn_increment_row_version),
--           0005 (sec schema — sec.user FK target),
--           0009 (periode_buku — mst.periode_buku FK target),
--           0017 (mapping_jurnal_schema_fix — workflow_status CHECK on mapping_jurnal_header),
--           0019 (instrumen_schema_fix — mst.instrumen FK target),
--           0033 (penempatan_deposito_p5_m1 — P5-M1 baseline, trx.penempatan_deposito)
-- description:
--   P5-M2 Jurnal Engine schema:
--   (A) ALTER mst.mapping_jurnal_header — add full 4-eyes/6-eyes workflow columns:
--       maker_id, reviewer_id, approver_id + signatures + approver_2 (6-eyes path for
--       regulated codes), workflow_path discriminator, submit_at, reject_reason.
--       Rebuild workflow_status CHECK to add APPROVED_ACTIVE + WITHDRAWN.
--       Add 4-way SoD CHECK constraint (chk_mapping_sod_4way).
--   (B) CREATE TABLE sys.dlq_jurnal_post — Dead Letter Queue for failed event-driven posts.
--       Full audit cols, idempotency UNIQUE, hard-delete REJECT trigger.
--   (C) Append-only enforcement on jrnl.header + jrnl.detail — BEFORE UPDATE/DELETE triggers.
--       Idempotency index on (reference_event_id, event_code) WHERE NOT NULL.
--   (D) CREATE SEQUENCE sys.seq_no_jurnal_2026 — per-year no_jurnal counter.
--       Seed sys.config DLQ_MAX_ATTEMPTS + NO_JURNAL_CURRENT_YEAR.
--   (E) Seed 27 event codes into mst.mapping_jurnal_header as DRAFT (status: DRAFT).
--       Operator must submit → review → approve via UI before resolver can use them.
--       Sentinel UUID 00000000-0000-0000-0000-000000000001 as system creator.

BEGIN;

-- ====================================================================
-- A. ALTER mst.mapping_jurnal_header — full workflow columns
-- ====================================================================

-- A1. Workflow path discriminator (4-eyes = operational, 6-eyes = regulated)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS workflow_path  VARCHAR(10) NOT NULL DEFAULT '4-eyes';

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_workflow_path;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_workflow_path
        CHECK (workflow_path IN ('4-eyes', '6-eyes'));

COMMENT ON COLUMN mst.mapping_jurnal_header.workflow_path IS
    'Auto-set at CREATE by server (backend). 4-eyes for operational codes, 6-eyes for '
    'regulated codes (ECL/EIR/MTM/REKLAS/FX_UNREALIZED). See regulatedEventCodes whitelist '
    'in backend/internal/app-d/jurnal/domain/errors.go.';

-- A2. Maker / reviewer / approver (4-eyes and base of 6-eyes)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS maker_id                    UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS reviewer_id                 UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS approver_id                 UUID REFERENCES sec.user(id);

COMMENT ON COLUMN mst.mapping_jurnal_header.maker_id     IS 'ROLE-AKUN who created the mapping. Required from DRAFT state.';
COMMENT ON COLUMN mst.mapping_jurnal_header.reviewer_id  IS 'ROLE-AKUN-CTL reviewer. Set on PENDING_REVIEW → PENDING_APPROVAL.';
COMMENT ON COLUMN mst.mapping_jurnal_header.approver_id  IS 'ROLE-AKUN-CTL approver (4-eyes) or 1st approver (6-eyes). Set on approve step.';

-- A3. Reviewer signature (DEC-018 audit-grade, SHA-256 BYTEA)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS reviewer_signed_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reviewer_signature_hash     BYTEA,
    ADD COLUMN IF NOT EXISTS comment_review              TEXT;

COMMENT ON COLUMN mst.mapping_jurnal_header.reviewer_signature_hash IS
    'SHA-256(reviewer_id || ''REVIEW'' || id || reviewer_signed_at || comment_review). '
    'Stored as BYTEA. Immutable once set.';

-- A4. Approver signature (1st approver, both 4-eyes and 6-eyes)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS approver_signed_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approver_signature_hash     BYTEA,
    ADD COLUMN IF NOT EXISTS comment_approve             TEXT;

COMMENT ON COLUMN mst.mapping_jurnal_header.approver_signature_hash IS
    'SHA-256(approver_id || ''APPROVE'' || id || approver_signed_at || comment_approve). '
    'Immutable once set. 6-eyes path: set when PENDING_APPROVAL → PENDING_APPROVAL_2.';

-- A5. Approver-2 columns (6-eyes path only — regulated codes, ROLE-RISK)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS approver_2_id               UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS approver_2_signed_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approver_2_signature_hash   BYTEA,
    ADD COLUMN IF NOT EXISTS comment_approve_2           TEXT;

COMMENT ON COLUMN mst.mapping_jurnal_header.approver_2_id IS
    'ROLE-RISK second approver for regulated codes (6-eyes path). '
    'NULL for 4-eyes (operational) mappings. Set on PENDING_APPROVAL_2 → APPROVED_ACTIVE.';

COMMENT ON COLUMN mst.mapping_jurnal_header.approver_2_signature_hash IS
    'SHA-256(approver_2_id || ''APPROVE_2'' || id || approver_2_signed_at || comment_approve_2). '
    'Step-up MFA required before approve-2 (DEC-027, OQ-M2-1c).';

-- A6. Workflow timestamps
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS submit_at                   TIMESTAMPTZ;

COMMENT ON COLUMN mst.mapping_jurnal_header.submit_at IS
    'Set by POST /{id}/submit (maker). Cleared on rejection back to DRAFT.';

-- A7. Reject / withdraw reason
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS reject_reason               TEXT;

COMMENT ON COLUMN mst.mapping_jurnal_header.reject_reason IS
    'Populated on any rejection (JURNAL_MAPPING.REJECT). '
    'Application-layer enforces length >= 30 chars. Cleared when workflow restarts from DRAFT.';

-- A8. Rebuild workflow_status CHECK to include APPROVED_ACTIVE + WITHDRAWN
--     (000017 had: DRAFT, PENDING_REVIEW, PENDING_APPROVAL, PENDING_APPROVAL_2, APPROVED,
--                  REJECTED, RETURNED)
--     P5-M2 state machine adds APPROVED_ACTIVE (operational name) and WITHDRAWN (terminal).
--     Keep APPROVED + REJECTED + RETURNED for backward compatibility with any existing rows.
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_header_workflow_status;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_header_workflow_status
        CHECK (workflow_status IN (
            'DRAFT',
            'PENDING_REVIEW',
            'PENDING_APPROVAL',
            'PENDING_APPROVAL_2',
            'APPROVED_ACTIVE',
            'WITHDRAWN',
            'APPROVED',      -- legacy compat (pre-P5-M2 rows; alias for APPROVED_ACTIVE)
            'REJECTED',      -- legacy compat (transient reject label)
            'RETURNED'       -- legacy compat (returned to maker)
        ));

COMMENT ON COLUMN mst.mapping_jurnal_header.workflow_status IS
    'P5-M2 state machine: DRAFT→PENDING_REVIEW→PENDING_APPROVAL'
    '→[PENDING_APPROVAL_2 for 6-eyes]→APPROVED_ACTIVE. '
    'Terminal: WITHDRAWN (soft-deleted). '
    'Legacy: APPROVED = pre-P5-M2 approved rows (treated as APPROVED_ACTIVE by resolver). '
    'See p5-m2-jurnal-engine.md §1 for full transition table.';

-- A9. SoD 4-way CHECK constraint (DB defense-in-depth, DEC-017)
--     All comparisons NULL-safe: constraints only fire when both sides are non-NULL.
--     approver_2_id is only set on 6-eyes path, so this naturally applies only there.
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_4way;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_sod_4way
        CHECK (
            (approver_2_id IS NULL)   -- 4-eyes: approver_2 not set → constraint vacuously true
            OR (
                (maker_id    IS NULL OR approver_2_id <> maker_id)
                AND (reviewer_id IS NULL OR approver_2_id <> reviewer_id)
                AND (approver_id IS NULL OR approver_2_id <> approver_id)
            )
        );

COMMENT ON CONSTRAINT chk_mapping_sod_4way ON mst.mapping_jurnal_header IS
    'DEC-017 SoD defense-in-depth: approver_2 must differ from maker, reviewer, and approver. '
    'Vacuously true for 4-eyes (approver_2_id IS NULL). '
    'Application layer (MappingService) is primary enforcement; this is belt-and-suspenders.';

-- A10. SoD CHECK for reviewer vs maker (already partially covered by old constraint? No — adding now)
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_reviewer_vs_maker;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_sod_reviewer_vs_maker
        CHECK (
            reviewer_id IS NULL
            OR maker_id IS NULL
            OR reviewer_id <> maker_id
        );

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_approver_vs_maker;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_sod_approver_vs_maker
        CHECK (
            approver_id IS NULL
            OR maker_id IS NULL
            OR approver_id <> maker_id
        );

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_approver_vs_reviewer;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_sod_approver_vs_reviewer
        CHECK (
            approver_id IS NULL
            OR reviewer_id IS NULL
            OR approver_id <> reviewer_id
        );

-- A11. Indexes on new FK columns
CREATE INDEX IF NOT EXISTS idx_mapping_header_maker
    ON mst.mapping_jurnal_header (maker_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mapping_header_reviewer
    ON mst.mapping_jurnal_header (reviewer_id)
    WHERE reviewer_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mapping_header_approver
    ON mst.mapping_jurnal_header (approver_id)
    WHERE approver_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mapping_header_approver_2
    ON mst.mapping_jurnal_header (approver_2_id)
    WHERE approver_2_id IS NOT NULL;

-- Pending approval-2 queue inspection
CREATE INDEX IF NOT EXISTS idx_mapping_header_pending_approval_2
    ON mst.mapping_jurnal_header (workflow_status, updated_at DESC)
    WHERE workflow_status = 'PENDING_APPROVAL_2';

-- ====================================================================
-- B. CREATE TABLE sys.dlq_jurnal_post
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.dlq_jurnal_post (

    -- Primary key
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Source event identity (from Asynq task payload)
    source_event_id         UUID            NOT NULL,
    source_event_type       TEXT            NOT NULL,
    -- ^ Asynq task type name, e.g. 'penempatan:approved', 'mtm:computed'

    -- Event classification
    event_code              TEXT            NOT NULL,
    -- ^ DEC-P5-M1-002 master code, e.g. 'PENEMPATAN', 'ECL_PEMBENTUKAN'

    -- Optional FK refs (may be NULL if payload is malformed / not yet parsed)
    instrumen_id            UUID            REFERENCES mst.instrumen(id),
    periode_id              UUID            REFERENCES mst.periode_buku(id),

    -- Full Asynq task payload snapshot (ResolverInput JSON)
    payload_jsonb           JSONB           NOT NULL,
    -- ^ Validator in DLQService.Insert() must strip PII (nomor_rekening, NPWP, KTP)
    --   before storing. See security checklist §15 in p5-m2-jurnal-engine.md.

    -- Error details
    error_code              TEXT            NOT NULL,
    -- ^ One of: JURNAL_EVENT_NOT_MAPPED, JURNAL_KLASIFIKASI_NOT_ELIGIBLE,
    --   JURNAL_BALANCE_INVARIANT, JURNAL_PERIODE_HARD_CLOSED, INFRA_DB_TIMEOUT, etc.
    error_message           TEXT            NOT NULL,
    error_category          TEXT            NOT NULL,
    -- ^ DOMAIN (immediate acknowledge, no auto-retry) vs INFRA (3x retry then DLQ)

    -- Retry tracking
    retry_count             INT             NOT NULL DEFAULT 0,
    last_retry_at           TIMESTAMPTZ,

    -- Status lifecycle
    status                  TEXT            NOT NULL DEFAULT 'FAILED',

    -- Replay tracking (REPLAYING → REPLAYED_OK)
    replayed_by             UUID            REFERENCES sec.user(id),
    replayed_at             TIMESTAMPTZ,

    -- Final jurnal reference (set after successful replay)
    final_jurnal_header_id  UUID            REFERENCES jrnl.header(id),
    -- ^ NULL until status = REPLAYED_OK

    -- Discard tracking (ABANDONED)
    discarded_reason        TEXT,
    -- ^ Application-layer enforces length >= 30 chars (DEC-P5-M2 rule)
    discarded_by            UUID            REFERENCES sec.user(id),
    discarded_at            TIMESTAMPTZ,

    -- Standard audit columns (db-conventions.md)
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL REFERENCES sec.user(id),
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL REFERENCES sec.user(id),
    deleted_at              TIMESTAMPTZ,
    -- ^ sys.dlq_jurnal_post is a critical table; no hard delete from API.
    --   Soft-delete column present for convention compliance only.
    --   Trigger below rejects all DELETE statements.
    deleted_by              UUID            REFERENCES sec.user(id),
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    CONSTRAINT chk_dlq_error_category
        CHECK (error_category IN ('DOMAIN', 'INFRA')),

    CONSTRAINT chk_dlq_status
        CHECK (status IN ('FAILED', 'REPLAYING', 'REPLAYED_OK', 'ABANDONED')),

    CONSTRAINT chk_dlq_retry_count_nonneg
        CHECK (retry_count >= 0),

    -- REPLAYED_OK requires final_jurnal_header_id
    CONSTRAINT chk_dlq_replayed_ok_has_jurnal
        CHECK (
            status <> 'REPLAYED_OK'
            OR final_jurnal_header_id IS NOT NULL
        ),

    -- ABANDONED requires discarded_reason + discarded_by + discarded_at
    CONSTRAINT chk_dlq_abandoned_has_reason
        CHECK (
            status <> 'ABANDONED'
            OR (
                discarded_reason IS NOT NULL
                AND discarded_by IS NOT NULL
                AND discarded_at IS NOT NULL
            )
        ),

    -- discarded_reason length >= 30 when set (DEC-P5-M2)
    CONSTRAINT chk_dlq_discard_reason_length
        CHECK (
            discarded_reason IS NULL
            OR length(discarded_reason) >= 30
        )
);

COMMENT ON TABLE sys.dlq_jurnal_post IS
    'Dead Letter Queue for failed event-driven jurnal posts. '
    'Populated by Asynq worker (jurnal_subscriber.go) on domain or infra errors. '
    'Replay: POST /jurnal/dlq/{id}/replay by ROLE-AKUN-CTL or ROLE-IT-ADMIN. '
    'Discard: POST /jurnal/dlq/{id}/discard by ROLE-IT-ADMIN only. '
    'No hard delete allowed. Retention same as aud.audit_log (10+10 tahun). '
    'See p5-m2-jurnal-engine.md §4 for DLQ state machine.';

-- B-UNIQUE: Idempotency — same (source_event_id, source_event_type, tenant_id) cannot
-- appear twice in FAILED or REPLAYING state simultaneously.
-- Fulfilled entries (REPLAYED_OK, ABANDONED) are excluded so history is preserved.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dlq_source_event_inflight
    ON sys.dlq_jurnal_post (source_event_id, source_event_type, tenant_id)
    WHERE status IN ('FAILED', 'REPLAYING');

COMMENT ON INDEX uq_dlq_source_event_inflight IS
    'Idempotency guard: same (source_event_id, event_type) cannot have >1 active '
    '(FAILED/REPLAYING) DLQ entry per tenant. Prevents double-insert from concurrent workers.';

-- B-INDEXES
-- 1. Queue inspection: active FAILED/REPLAYING entries ordered by age
CREATE INDEX IF NOT EXISTS idx_dlq_status_created
    ON sys.dlq_jurnal_post (status, created_at DESC)
    WHERE status IN ('FAILED', 'REPLAYING');

-- 2. Lookup by source event (worker dedup check)
CREATE INDEX IF NOT EXISTS idx_dlq_source_event_id
    ON sys.dlq_jurnal_post (source_event_id);

-- 3. Filter by event code + status (DLQ browser DataTable)
CREATE INDEX IF NOT EXISTS idx_dlq_event_code_status
    ON sys.dlq_jurnal_post (event_code, status);

-- 4. Auto-replay eligibility: FAILED rows with low retry_count
CREATE INDEX IF NOT EXISTS idx_dlq_retry_count_failed
    ON sys.dlq_jurnal_post (retry_count)
    WHERE status = 'FAILED';

-- 5. FK: instrumen_id
CREATE INDEX IF NOT EXISTS idx_dlq_instrumen_id
    ON sys.dlq_jurnal_post (instrumen_id)
    WHERE instrumen_id IS NOT NULL;

-- 6. FK: periode_id
CREATE INDEX IF NOT EXISTS idx_dlq_periode_id
    ON sys.dlq_jurnal_post (periode_id)
    WHERE periode_id IS NOT NULL;

-- 7. Tenant + created_at composite (multi-tenant readiness)
CREATE INDEX IF NOT EXISTS idx_dlq_tenant_created
    ON sys.dlq_jurnal_post (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- B-TRIGGERS: updated_at + row_version (standard pattern)
CREATE TRIGGER trg_dlq_jurnal_post_updated_at
    BEFORE UPDATE ON sys.dlq_jurnal_post
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_dlq_jurnal_post_row_version
    BEFORE UPDATE ON sys.dlq_jurnal_post
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- B-TRIGGER: Hard-delete guard (sys is critical; no DELETE from API)
CREATE OR REPLACE FUNCTION fn_dlq_jurnal_post_no_hard_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.dlq_jurnal_post is forbidden. '
        'Use POST /jurnal/dlq/{id}/discard to abandon, or soft-delete (deleted_at). '
        'Retention: 10+10 tahun per DEC-018.';
END;
$$;

COMMENT ON FUNCTION fn_dlq_jurnal_post_no_hard_delete() IS
    'Prevents accidental or malicious DELETE on the DLQ table. '
    'System-critical: loss of DLQ entries breaks audit trail continuity.';

CREATE TRIGGER trg_dlq_jurnal_post_no_hard_delete
    BEFORE DELETE ON sys.dlq_jurnal_post
    FOR EACH ROW EXECUTE FUNCTION fn_dlq_jurnal_post_no_hard_delete();

-- ====================================================================
-- C. Append-only enforcement on jrnl.header + jrnl.detail
-- ====================================================================

-- C1. jrnl.header — BEFORE UPDATE trigger (DEC-018, append-only)
--     jrnl.header has no audit col triggers from migration 0001;
--     enforcement here is via REJECT of all UPDATE statements.
CREATE OR REPLACE FUNCTION fn_jrnl_header_no_update()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'jrnl.header is append-only (DEC-018). '
        'UPDATE is not permitted. To reverse a journal, insert a reversal entry. '
        'Error code: JURNAL_APPEND_ONLY_VIOLATION';
END;
$$;

COMMENT ON FUNCTION fn_jrnl_header_no_update() IS
    'Append-only enforcement for jrnl.header. '
    'Security checklist §15 (p5-m2-jurnal-engine.md): DDL trigger must exist before merge.';

CREATE OR REPLACE TRIGGER trg_jrnl_header_no_update
    BEFORE UPDATE ON jrnl.header
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_header_no_update();

-- C2. jrnl.header — BEFORE DELETE trigger
CREATE OR REPLACE FUNCTION fn_jrnl_header_no_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'jrnl.header is append-only (DEC-018). '
        'DELETE is not permitted. Retention: 10+10 tahun. '
        'Error code: JURNAL_APPEND_ONLY_VIOLATION';
END;
$$;

CREATE OR REPLACE TRIGGER trg_jrnl_header_no_delete
    BEFORE DELETE ON jrnl.header
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_header_no_delete();

-- C3. jrnl.detail — BEFORE UPDATE trigger
CREATE OR REPLACE FUNCTION fn_jrnl_detail_no_update()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'jrnl.detail is append-only (DEC-018). '
        'UPDATE is not permitted. '
        'Error code: JURNAL_APPEND_ONLY_VIOLATION';
END;
$$;

CREATE OR REPLACE TRIGGER trg_jrnl_detail_no_update
    BEFORE UPDATE ON jrnl.detail
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_detail_no_update();

-- C4. jrnl.detail — BEFORE DELETE trigger
CREATE OR REPLACE FUNCTION fn_jrnl_detail_no_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'jrnl.detail is append-only (DEC-018). '
        'DELETE is not permitted. Retention: 10+10 tahun. '
        'Error code: JURNAL_APPEND_ONLY_VIOLATION';
END;
$$;

CREATE OR REPLACE TRIGGER trg_jrnl_detail_no_delete
    BEFORE DELETE ON jrnl.detail
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_detail_no_delete();

-- C5. Idempotency index on jrnl.header: (reference_event_id, event_code) WHERE NOT NULL
--     Prevents same Asynq event from posting twice even if idempotency_key differs.
--     uq_jrnl_idempotency on idempotency_key already exists from migration 0001.
CREATE UNIQUE INDEX IF NOT EXISTS uq_jrnl_source_event
    ON jrnl.header (reference_event_id, event_code)
    WHERE reference_event_id IS NOT NULL;

COMMENT ON INDEX uq_jrnl_source_event IS
    'Secondary idempotency guard: same (reference_event_id, event_code) cannot post twice. '
    'Catches pre-computed idempotency_key conflicts. See p5-m2-jurnal-engine.md §3 (C3).';

-- C6. REVOKE UPDATE/DELETE from service role (DEC-018, security checklist §15)
--     Only executed if blips_service_role exists; NOTICE if not (CI environments may skip).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'blips_service_role') THEN
        REVOKE UPDATE, DELETE ON jrnl.header FROM blips_service_role;
        REVOKE UPDATE, DELETE ON jrnl.detail FROM blips_service_role;
    ELSE
        RAISE NOTICE 'blips_service_role does not exist — REVOKE skipped. '
            'Run manually after role is created: '
            'REVOKE UPDATE, DELETE ON jrnl.header, jrnl.detail FROM blips_service_role;';
    END IF;
END;
$$;

-- ====================================================================
-- D. SEQUENCE sys.seq_no_jurnal_2026 + sys.config seeds
-- ====================================================================

-- D1. Per-year sequence for no_jurnal (JRN-{YYYY}-{######})
CREATE SEQUENCE IF NOT EXISTS sys.seq_no_jurnal_2026
    START 1
    INCREMENT BY 1
    MAXVALUE 999999
    NO CYCLE;

COMMENT ON SEQUENCE sys.seq_no_jurnal_2026 IS
    'Annual counter for jrnl.header.no_jurnal. '
    'Format composed in Go: JRN-2026-{nextval padded to 6 digits}. '
    'New sequence per year: seq_no_jurnal_2027, etc. created by migration at year rollover. '
    'Current year tracked in sys.config key NO_JURNAL_CURRENT_YEAR.';

-- D2. sys.config seeds
INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES
(
    'NO_JURNAL_CURRENT_YEAR',
    '2026',
    'INT',
    FALSE,
    'Active year for sys.seq_no_jurnal_{YEAR} sequence. '
    'Go service reads this before calling nextval. '
    'Updated via migration at each calendar-year rollover.',
    'JURNAL'
),
(
    'DLQ_MAX_ATTEMPTS',
    '5',
    'INT',
    FALSE,
    'Maximum attempt_count before sys.dlq_jurnal_post entry triggers alert to '
    'ROLE-AKUN-CTL + ROLE-IT-ADMIN. Does not auto-abandon — human decision required. '
    'See p5-m2-jurnal-engine.md §4.1 (Auto-Abandon Rule).',
    'JURNAL'
)
ON CONFLICT (config_key) DO NOTHING;

-- ====================================================================
-- E. Seed 27 event codes → mst.mapping_jurnal_header (DRAFT)
-- ====================================================================
-- Status: DRAFT intentionally. Operator must submit → review → approve via UI
-- before resolver can use these templates (aktif_flag remains FALSE at this stage;
-- set to TRUE only on APPROVED_ACTIVE transition by the approve/approve-2 handler).
--
-- Sentinel system user: 00000000-0000-0000-0000-000000000001
-- Assumption: sentinel user exists in sec.user from migration 0001 or 0005 seed.
-- If not, Go migration runner should pre-insert sentinel before running this file.
--
-- maker_id intentionally NULL for seed rows (no human maker; system seed).
-- workflow_path auto-set: regulated codes → 6-eyes, operational → 4-eyes.
-- klasifikasi_berlaku: NULL = all classifications.

INSERT INTO mst.mapping_jurnal_header (
    id,
    event_id_kode,
    event_code,
    nama_event,
    kategori_event,
    trigger_source,
    klasifikasi_berlaku,
    aktif_flag,
    workflow_status,
    workflow_path,
    catatan,
    created_by,
    updated_by,
    created_at,
    updated_at,
    row_version,
    tenant_id
)
VALUES

-- ──────────────────────────────────────────────────────────────────
-- 1. PENEMPATAN — Penempatan awal (semua klasifikasi)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-001', 'PENEMPATAN',
    'Penempatan Instrumen Keuangan',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Status DRAFT — operator wajib approve via UI. Asynq: penempatan:approved.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 2. AKRUAL_BUNGA — Akrual bunga periodik (AC, FVOCI, POCI)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-002', 'AKRUAL_BUNGA',
    'Akrual Bunga Periodik',
    'AKRUAL', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. FVTPL dikecualikan (tidak ada EIR). Asynq: akrual:computed.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 3. ECL_PEMBENTUKAN — Pembentukan ECL (regulated 6-eyes)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-003', 'ECL_PEMBENTUKAN',
    'Pembentukan ECL (Cadangan Kerugian Penurunan Nilai)',
    'ECL', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED — 6-eyes workflow. PSAK 71 §5.5.15: FVTPL tidak punya ECL. Asynq: ecl:charged.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 4. ECL_REVERSAL — Reversal ECL (regulated 6-eyes)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-004', 'ECL_REVERSAL',
    'Reversal ECL (Pemulihan Cadangan)',
    'ECL', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED — 6-eyes. Asynq: ecl:reversed.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 5. POCI_DELTA_ECL — Delta ECL untuk instrumen POCI (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-005', 'POCI_DELTA_ECL',
    'Delta ECL untuk Instrumen POCI',
    'ECL', 'SYSTEM_JOB',
    ARRAY['POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Hanya klasifikasi POCI. Credit-adjusted EIR basis.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 6. MTM_FVTPL — Mark-to-Market FVTPL (regulated 6-eyes)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-006', 'MTM_FVTPL',
    'Mark-to-Market FVTPL — Perubahan Fair Value ke P&L',
    'MUTASI_MTM', 'SYSTEM_JOB',
    ARRAY['FVTPL'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Hanya FVTPL. Asynq: mtm:computed.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 7. MTM_FVOCI — Mark-to-Market FVOCI debt (regulated 6-eyes)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-007', 'MTM_FVOCI',
    'Mark-to-Market FVOCI Debt — Perubahan Fair Value ke OCI',
    'MUTASI_MTM', 'SYSTEM_JOB',
    ARRAY['FVOCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. FVOCI debt only (not FVOCI_ELECTION). Asynq: mtm:computed.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 8. MTM_FVOCI_ELECTION — MTM untuk saham FVOCI election (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-008', 'MTM_FVOCI_ELECTION',
    'Mark-to-Market Saham FVOCI Election — OCI tanpa Recycling ke P&L',
    'MUTASI_MTM', 'SYSTEM_JOB',
    ARRAY['FVOCI_ELECTION'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. FVOCI_ELECTION saham saja. No P&L recycling on disposal (PSAK 71).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 9. REKLAS_OCI_PL — Recycling OCI ke P&L saat derecognition FVOCI
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-009', 'REKLAS_OCI_PL',
    'Reklasifikasi OCI ke P&L saat Derecognition FVOCI Debt',
    'REKLASIFIKASI', 'SYSTEM_JOB',
    ARRAY['FVOCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Hanya FVOCI debt. FVOCI_ELECTION dikecualikan (no recycling).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 10. REKLASIFIKASI_AC_FVOCI — Reklasifikasi AC → FVOCI (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-010', 'REKLASIFIKASI_AC_FVOCI',
    'Reklasifikasi AC ke FVOCI — Pengakuan OCI Gain/Loss',
    'REKLASIFIKASI', 'USER_INPUT',
    ARRAY['AC','FVOCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Business model reassessment per PSAK 71.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 11. REKLASIFIKASI_FVOCI_AC — Reklasifikasi FVOCI → AC (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-011', 'REKLASIFIKASI_FVOCI_AC',
    'Reklasifikasi FVOCI ke AC — Reset Amortized Cost',
    'REKLASIFIKASI', 'USER_INPUT',
    ARRAY['FVOCI','AC'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Amortized cost reset per PSAK 71.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 12. MODIFIKASI_MATERIAL — Modifikasi kontrak material (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-012', 'MODIFIKASI_MATERIAL',
    'Modifikasi Kontrak Material — Pengakuan Gain/Loss Modifikasi',
    'MUTASI_MTM', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Triggers EIR re-estimation (amendment versioning).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 13. EIR_CATCH_UP_ADJUSTMENT — EIR catch-up on amendment (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-013', 'EIR_CATCH_UP_ADJUSTMENT',
    'EIR Catch-up Adjustment setelah Amandemen Kontrak',
    'AKRUAL', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Asynq: eir:amended. Re-estimation Newton-Raphson (DEC-013).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 14. STAGE_MIGRATION — Perpindahan Stage ECL (regulated)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-014', 'STAGE_MIGRATION',
    'Migrasi Stage ECL (Stage 1 ↔ 2 ↔ 3)',
    'STAGE_MIGRATION', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Perpindahan stage per SICR triggers (DEC-011) dan cure (DEC-012).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 15. JATUH_TEMPO — Penerimaan kembali pokok saat jatuh tempo
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-015', 'JATUH_TEMPO',
    'Penerimaan Pokok saat Jatuh Tempo (Derecognition)',
    'CLOSURE', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Asynq: penempatan:matured. Semua klasifikasi.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 16. PENJUALAN_PENCAIRAN — Penjualan / pencairan sebelum jatuh tempo
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-016', 'PENJUALAN_PENCAIRAN',
    'Penjualan / Pencairan Sebelum Jatuh Tempo',
    'CLOSURE', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Asynq: penempatan:terminated. Semua klasifikasi.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 17. PEMBAYARAN_BUNGA — Penerimaan pembayaran bunga/kupon
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-017', 'PEMBAYARAN_BUNGA',
    'Penerimaan Pembayaran Bunga / Kupon',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Asynq: penempatan:terminated (jika ada bunga). Instrumen dengan kupon/bunga.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 18. PEMBAYARAN_POKOK — Penerimaan cicilan pokok (amortisasi)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-018', 'PEMBAYARAN_POKOK',
    'Penerimaan Cicilan Pokok (Principal Repayment)',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Principal repayment per amortisasi schedule.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 19. RENEWAL_DEPOSITO — Renewal deposito (roll-over)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-019', 'RENEWAL_DEPOSITO',
    'Renewal Deposito (Roll-over)',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Deposito renewal dengan tenor baru. Hanya AC dan FVOCI.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 20. PENERIMAAN_DIVIDEN — Dividen saham
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-020', 'PENERIMAAN_DIVIDEN',
    'Penerimaan Dividen Saham',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['FVTPL','FVOCI_ELECTION'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Hanya instrumen saham (FVTPL dan FVOCI_ELECTION).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 21. DISTRIBUSI_REKSADANA — NAB distribution Reksadana
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-021', 'DISTRIBUSI_REKSADANA',
    'Distribusi NAB Reksadana',
    'PENEMPATAN', 'SYSTEM_JOB',
    ARRAY['FVTPL'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Reksadana NAB distribution. Hanya FVTPL.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 22. FX_REALIZED — FX gain/loss realized (semua klasifikasi)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-022', 'FX_REALIZED',
    'FX Gain/Loss Realized',
    'FX', 'SYSTEM_JOB',
    NULL,   -- NULL = all classifications
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Semua klasifikasi (NULL). FX realized pada derecognition.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 23. FX_UNREALIZED — FX gain/loss unrealized (regulated 6-eyes)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-023', 'FX_UNREALIZED',
    'FX Gain/Loss Unrealized (Akhir Periode)',
    'FX', 'SYSTEM_JOB',
    NULL,   -- NULL = all classifications
    FALSE, 'DRAFT', '6-eyes',
    'Seed P5-M2. REGULATED. Semua klasifikasi (NULL). FX unrealized end-of-period revaluation.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 24. AMORTISASI_PREMI_DISKONTO — EIR amortization premium/discount
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-024', 'AMORTISASI_PREMI_DISKONTO',
    'Amortisasi Premi / Diskonto (EIR)',
    'AKRUAL', 'SYSTEM_JOB',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. EIR amortization of premium/discount. AC, FVOCI, POCI.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 25. PENGHAPUSAN — Write-off instrumen (hapus buku)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-025', 'PENGHAPUSAN',
    'Penghapusan Instrumen (Write-off)',
    'CLOSURE', 'USER_INPUT',
    ARRAY['AC','FVOCI','POCI'],
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Write-off. Hanya AC, FVOCI, POCI. User-initiated dengan 4-eyes approval.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 26. PERIODE_ADJUSTMENT — Adjustment periode (manual posting only)
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-026', 'PERIODE_ADJUSTMENT',
    'Penyesuaian Periode Buku (Manual)',
    'KOREKSI', 'USER_INPUT',
    NULL,   -- NULL = global, instrumen optional
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Manual posting only. Requires dokumenDocId at submit. '
    'Valid for OPEN or SOFT_CLOSED periods only.',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
),

-- ──────────────────────────────────────────────────────────────────
-- 27. CORRECTION_PERIODE_CLOSED — Koreksi setelah periode soft-closed
-- ──────────────────────────────────────────────────────────────────
(
    gen_random_uuid(),
    'EVT-027', 'CORRECTION_PERIODE_CLOSED',
    'Koreksi Jurnal setelah Soft-Close Periode',
    'KOREKSI', 'USER_INPUT',
    NULL,   -- NULL = global
    FALSE, 'DRAFT', '4-eyes',
    'Seed P5-M2. Manual posting only. Valid for SOFT_CLOSED periods only. '
    'Cannot post to HARD_CLOSED period (JURNAL_PERIODE_HARD_CLOSED).',
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    now(), now(), 1, 'TUGURE'
)

ON CONFLICT (event_code) DO NOTHING;
-- ON CONFLICT DO NOTHING: safe re-run; existing approved rows are never overwritten.
-- event_id_kode unique constraint: all EVT-001..027 are new; no collision expected.

COMMIT;
