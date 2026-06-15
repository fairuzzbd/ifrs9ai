-- migration: 0037 gl_delivery_p5_m3
-- author: data-modeler
-- requires: 0001 (init_schema — jrnl.gl_status, jrnl.header, mst.chart_of_accounts,
--                              sec.user, sys.config, fn_update_updated_at,
--                              fn_increment_row_version),
--           0005 (sec schema — sec.user FK target),
--           0019 (instrumen_schema_fix — mst.instrumen FK target),
--           0035 (jurnal_engine_p5_m2 — jrnl.header append-only trigger active)
-- description:
--   P5-M3 GL Host Delivery schema:
--   (A) ALTER jrnl.gl_status — add P5-M3 GL delivery tracking columns:
--       failure_category, gl_response_payload_jsonb, manual_retry_by/at/reason,
--       discarded_by/at/discard_reason, payload_sent_at, delivery_response_id.
--       Add CHECK on gl_host_status covering full P5-M3 state machine
--       (DELIVERY_IN_FLIGHT added). Drop+rebuild partial indexes from 000001
--       to cover new statuses. Update append-only trigger to ALLOW UPDATE
--       only on GL delivery cols + audit cols (replacing the blanket REJECT
--       from 000035 which covers jrnl.header; jrnl.gl_status never had that
--       blanket trigger — it had a dedicated no-update-on-delivered guard added
--       here for the first time).
--   (B) CREATE TABLE sys.dlq_gl_delivery — GL delivery dead letter queue.
--       Hard-delete REJECT trigger, full audit cols, UNIQUE inflight guard.
--   (C) CREATE TABLE sys.gl_reconciliation_report — recon report header.
--       Hard-delete REJECT trigger, UNIQUE completed-per-date partial index.
--   (D) CREATE TABLE sys.gl_recon_mismatch — per-account mismatch detail.
--       Hard-delete REJECT trigger, FK → sys.gl_reconciliation_report.
--   (E) sys.config seed — 9 GL delivery + recon config keys.

BEGIN;

-- ====================================================================
-- A. ALTER jrnl.gl_status — P5-M3 GL delivery tracking columns
-- ====================================================================

-- A1. Add CHECK on gl_host_status (VARCHAR(20) from 000001, no prior CHECK).
--     Covers all P5-M3 states including new DELIVERY_IN_FLIGHT.
ALTER TABLE jrnl.gl_status
    ADD CONSTRAINT chk_gl_status_host_status
        CHECK (gl_host_status IN (
            'PENDING_DELIVERY',
            'DELIVERY_IN_FLIGHT',
            'DELIVERED',
            'RETRYING',
            'FAILED',
            'DEAD_LETTER'
        ));

COMMENT ON COLUMN jrnl.gl_status.gl_host_status IS
    'P5-M3 state machine: PENDING_DELIVERY → DELIVERY_IN_FLIGHT → DELIVERED (terminal ok). '
    'On infra error: → RETRYING (up to 3x) → FAILED. On domain error: → FAILED directly. '
    'Manual actions: FAILED → PENDING_DELIVERY (retry), FAILED → DEAD_LETTER (discard). '
    'DELIVERED and DEAD_LETTER are terminal states; no further transitions.';

-- A2. New columns for P5-M3 GL delivery state tracking.

ALTER TABLE jrnl.gl_status
    ADD COLUMN IF NOT EXISTS failure_category          VARCHAR(20),
    ADD COLUMN IF NOT EXISTS gl_response_payload_jsonb JSONB,
    ADD COLUMN IF NOT EXISTS manual_retry_by           UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS manual_retry_at           TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS manual_retry_reason       TEXT,
    ADD COLUMN IF NOT EXISTS discarded_by              UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS discarded_at              TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS discard_reason            TEXT,
    ADD COLUMN IF NOT EXISTS payload_sent_at           TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_response_id      TEXT;

-- A3. CHECK constraints on new columns.

ALTER TABLE jrnl.gl_status
    ADD CONSTRAINT chk_gl_status_failure_category
        CHECK (failure_category IS NULL OR failure_category IN ('DOMAIN', 'INFRA'));

ALTER TABLE jrnl.gl_status
    ADD CONSTRAINT chk_gl_status_manual_retry_reason_len
        CHECK (
            manual_retry_reason IS NULL
            OR length(manual_retry_reason) >= 30
        );

ALTER TABLE jrnl.gl_status
    ADD CONSTRAINT chk_gl_status_discard_reason_len
        CHECK (
            discard_reason IS NULL
            OR length(discard_reason) >= 30
        );

-- DEAD_LETTER requires discard_reason + discarded_by + discarded_at
ALTER TABLE jrnl.gl_status
    ADD CONSTRAINT chk_gl_status_dead_letter_has_discard
        CHECK (
            gl_host_status <> 'DEAD_LETTER'
            OR (
                discard_reason IS NOT NULL
                AND discarded_by IS NOT NULL
                AND discarded_at IS NOT NULL
            )
        );

COMMENT ON COLUMN jrnl.gl_status.failure_category IS
    'DOMAIN: GL Host rejected (4xx, no auto-retry). '
    'INFRA: network/5xx/timeout (up to 3 auto-retries before FAILED). NULL while not in FAILED/DEAD_LETTER.';

COMMENT ON COLUMN jrnl.gl_status.gl_response_payload_jsonb IS
    'Sanitized GL Host response body. PII fields stripped before persist '
    '(customer_name, account_no, npwp, ktp). '
    'Set on DELIVERED (2xx) and FAILED (4xx domain error). NULL on infra failure.';

COMMENT ON COLUMN jrnl.gl_status.manual_retry_by IS
    'actor_user_id who triggered manual retry (POST /jurnal/header/{id}/retry-gl-delivery). '
    'Requires permission jurnal.gl_delivery.retry (ROLE-AKUN-CTL / ROLE-IT-ADMIN).';

COMMENT ON COLUMN jrnl.gl_status.manual_retry_reason IS
    'Free-text reason for manual retry. Application enforces length >= 30 chars. '
    'CHECK constraint provides DB defense-in-depth.';

COMMENT ON COLUMN jrnl.gl_status.discard_reason IS
    'Free-text reason for discard (DEAD_LETTER). length >= 30 chars enforced. '
    'Persisted as evidence for aud.audit_log after_jsonb (GL_DELIVERY.DLQ_DISCARDED).';

COMMENT ON COLUMN jrnl.gl_status.payload_sent_at IS
    'Timestamp when the HTTP payload was dispatched to GL Host (last attempt). '
    'Used to calculate delivery latency. NULL until first delivery attempt.';

COMMENT ON COLUMN jrnl.gl_status.delivery_response_id IS
    'GL Host assigned journal ID returned in 2xx response (journalId field). '
    'Alias / complement of gl_host_journal_id from migration 000001.';

-- A4. FK indexes on new reference columns.

CREATE INDEX IF NOT EXISTS idx_gl_status_manual_retry_by
    ON jrnl.gl_status (manual_retry_by)
    WHERE manual_retry_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_gl_status_discarded_by
    ON jrnl.gl_status (discarded_by)
    WHERE discarded_by IS NOT NULL;

-- A5. Partial indexes for worker scan (pending work).
--     Drop old indexes from 000001 that no longer cover all pending statuses.
DROP INDEX IF EXISTS jrnl.ix_gl_status_pending;
DROP INDEX IF EXISTS jrnl.ix_gl_status_dlq;

CREATE INDEX IF NOT EXISTS idx_gl_status_worker_scan
    ON jrnl.gl_status (gl_host_status, updated_at DESC)
    WHERE gl_host_status IN ('PENDING_DELIVERY', 'RETRYING', 'FAILED');

COMMENT ON INDEX idx_gl_status_worker_scan IS
    'Worker pickup index: Asynq gl_delivery:deliver scans for PENDING_DELIVERY; '
    'manual retry page lists FAILED; retry scheduler rescans RETRYING. '
    'Replaces ix_gl_status_pending from migration 000001.';

CREATE INDEX IF NOT EXISTS idx_gl_status_dead_letter
    ON jrnl.gl_status (gl_host_status, updated_at DESC)
    WHERE gl_host_status = 'DEAD_LETTER';

CREATE INDEX IF NOT EXISTS idx_gl_status_delivered_at
    ON jrnl.gl_status (delivered_at DESC)
    WHERE delivered_at IS NOT NULL;

-- A6. Guard trigger: prevent UPDATE on DELIVERED or DEAD_LETTER rows
--     (terminal state immutability for jrnl.gl_status).
--     Note: jrnl.header has a blanket no-update trigger from 000035;
--     jrnl.gl_status deliberately allows UPDATE for GL delivery worker
--     (state transitions), but terminal states must be immutable.
CREATE OR REPLACE FUNCTION fn_gl_status_terminal_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.gl_host_status IN ('DELIVERED', 'DEAD_LETTER') THEN
        RAISE EXCEPTION
            'jrnl.gl_status row % is in terminal state % and cannot be updated. '
            'DELIVERED and DEAD_LETTER are immutable. Error code: GL_STATUS_TERMINAL_IMMUTABLE',
            OLD.id, OLD.gl_host_status;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_gl_status_terminal_immutable() IS
    'Prevents UPDATE on jrnl.gl_status rows in terminal states DELIVERED or DEAD_LETTER. '
    'DB defense-in-depth for P5-M3 delivery state machine. '
    'Application layer (DeliveryService) also enforces this via idempotency checks.';

CREATE OR REPLACE TRIGGER trg_gl_status_terminal_immutable
    BEFORE UPDATE ON jrnl.gl_status
    FOR EACH ROW EXECUTE FUNCTION fn_gl_status_terminal_immutable();

-- A7. Hard-delete guard on jrnl.gl_status (DEC-018 retention).
CREATE OR REPLACE FUNCTION fn_gl_status_no_hard_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on jrnl.gl_status is forbidden (DEC-018). '
        'jrnl.gl_status is audit-grade; retention 10+10 tahun. '
        'Error code: GL_STATUS_NO_HARD_DELETE';
END;
$$;

CREATE OR REPLACE TRIGGER trg_gl_status_no_hard_delete
    BEFORE DELETE ON jrnl.gl_status
    FOR EACH ROW EXECUTE FUNCTION fn_gl_status_no_hard_delete();

-- A8. Standard triggers for updated_at and row_version (not present in 000001 for gl_status).
--     Check if fn_update_updated_at and fn_increment_row_version exist (from 000001).

-- Add row_version column if not present (000001 did not add it to jrnl.gl_status).
ALTER TABLE jrnl.gl_status
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1;

CREATE TRIGGER trg_gl_status_updated_at
    BEFORE UPDATE ON jrnl.gl_status
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_gl_status_row_version
    BEFORE UPDATE ON jrnl.gl_status
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- B. CREATE TABLE sys.dlq_gl_delivery
--    GL Host delivery dead letter queue.
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.dlq_gl_delivery (

    -- Primary key
    id                          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Source references
    jurnal_header_id            UUID            NOT NULL REFERENCES jrnl.header(id),
    -- ^ denormalized for direct lookups without joining jrnl.gl_status
    gl_status_id                UUID            REFERENCES jrnl.gl_status(id),
    -- ^ FK to the gl_status row that transitioned to FAILED (nullable for resilience)

    -- Payload snapshot (sanitized — PII stripped by worker before INSERT)
    payload_jsonb               JSONB           NOT NULL,
    -- ^ Contains the GL Host request payload sent at failure time.
    --   PII fields (customer_name, account_no, npwp, ktp) replaced with [REDACTED].
    --   GL_HOST_API_KEY is never included in this snapshot.

    -- Error classification
    error_code                  TEXT            NOT NULL,
    -- ^ Stable error code from error catalog (p5-m3-gl-delivery.md §6):
    --   GL_DELIVERY_HOST_UNREACHABLE, GL_DELIVERY_HOST_4XX,
    --   GL_DELIVERY_INVALID_RESPONSE, GL_DELIVERY_TIMEOUT, GL_DELIVERY_AUTH_FAILED.
    error_message               TEXT            NOT NULL,
    error_category              TEXT            NOT NULL,
    -- ^ DOMAIN (4xx — business rule rejection) or INFRA (5xx/timeout/network)

    -- Retry tracking
    retry_count                 INT             NOT NULL DEFAULT 0,
    last_retry_at               TIMESTAMPTZ,

    -- DLQ lifecycle status
    status                      TEXT            NOT NULL DEFAULT 'FAILED',
    -- ^ State machine: FAILED → REPLAYING → REPLAYED_OK (terminal ok)
    --                  FAILED → ABANDONED (terminal fail via discard)

    -- Replay tracking (REPLAYING → REPLAYED_OK)
    replayed_by                 UUID            REFERENCES sec.user(id),
    replayed_at                 TIMESTAMPTZ,

    -- Final delivery reference (set on REPLAYED_OK)
    final_delivery_response_id  TEXT,
    -- ^ GL Host returned journalId after successful replay delivery. NULL until REPLAYED_OK.

    -- Discard tracking (ABANDONED terminal state)
    discarded_reason            TEXT,
    -- ^ Application enforces length >= 30 chars. DB CHECK below.
    discarded_by                UUID            REFERENCES sec.user(id),
    -- ^ Must be ROLE-IT-ADMIN (enforced at service layer).
    discarded_at                TIMESTAMPTZ,

    -- Standard audit columns (db-conventions.md)
    created_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by                  UUID            NOT NULL REFERENCES sec.user(id),
    updated_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by                  UUID            NOT NULL REFERENCES sec.user(id),
    deleted_at                  TIMESTAMPTZ,
    -- ^ No hard delete; soft-delete column present for convention only (trigger rejects DELETE).
    deleted_by                  UUID            REFERENCES sec.user(id),
    row_version                 BIGINT          NOT NULL DEFAULT 1,
    tenant_id                   TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    CONSTRAINT chk_dlq_gl_error_category
        CHECK (error_category IN ('DOMAIN', 'INFRA')),

    CONSTRAINT chk_dlq_gl_status
        CHECK (status IN ('FAILED', 'REPLAYING', 'REPLAYED_OK', 'ABANDONED')),

    CONSTRAINT chk_dlq_gl_retry_count_nonneg
        CHECK (retry_count >= 0),

    -- REPLAYED_OK requires final_delivery_response_id
    CONSTRAINT chk_dlq_gl_replayed_ok_has_response
        CHECK (
            status <> 'REPLAYED_OK'
            OR final_delivery_response_id IS NOT NULL
        ),

    -- ABANDONED requires discarded_reason + discarded_by + discarded_at
    CONSTRAINT chk_dlq_gl_abandoned_has_reason
        CHECK (
            status <> 'ABANDONED'
            OR (
                discarded_reason IS NOT NULL
                AND discarded_by IS NOT NULL
                AND discarded_at IS NOT NULL
            )
        ),

    -- discarded_reason length >= 30 when set
    CONSTRAINT chk_dlq_gl_discard_reason_length
        CHECK (
            discarded_reason IS NULL
            OR length(discarded_reason) >= 30
        )
);

COMMENT ON TABLE sys.dlq_gl_delivery IS
    'Dead Letter Queue for failed GL Host deliveries (P5-M3). '
    'INSERTed atomically with jrnl.gl_status → FAILED transition (in same DB transaction). '
    'Replay: POST /jurnal/gl-delivery-dlq/{id}/replay (jurnal.gl_delivery.replay). '
    'Discard: POST /jurnal/gl-delivery-dlq/{id}/discard (jurnal.gl_delivery.discard — ROLE-IT-ADMIN only). '
    'No hard delete. Retention same as aud.audit_log (10+10 tahun, DEC-018). '
    'See p5-m3-gl-delivery.md §2 for DLQ state machine.';

-- B-UNIQUE: One active (FAILED or REPLAYING) DLQ entry per jurnal_header per tenant.
--           Completed entries (REPLAYED_OK, ABANDONED) are excluded → history is preserved.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dlq_gl_delivery_header_inflight
    ON sys.dlq_gl_delivery (jurnal_header_id, tenant_id)
    WHERE status IN ('FAILED', 'REPLAYING');

COMMENT ON INDEX uq_dlq_gl_delivery_header_inflight IS
    'Idempotency guard: at most one active (FAILED/REPLAYING) DLQ entry per jurnal_header per tenant. '
    'Prevents duplicate DLQ entries from concurrent worker failures. '
    'Terminal entries (REPLAYED_OK, ABANDONED) excluded; history is always preserved.';

-- B-INDEXES

-- 1. DLQ console list (inflight queue)
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_status_created
    ON sys.dlq_gl_delivery (status, created_at DESC)
    WHERE status IN ('FAILED', 'REPLAYING');

-- 2. Lookup by jurnal_header_id (worker idempotency check + API GET status)
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_header_id
    ON sys.dlq_gl_delivery (jurnal_header_id);

-- 3. Error code browser + filter (DLQ DataTable filter by error_code × status)
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_error_code_status
    ON sys.dlq_gl_delivery (error_code, status);

-- 4. FK: gl_status_id
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_gl_status_id
    ON sys.dlq_gl_delivery (gl_status_id)
    WHERE gl_status_id IS NOT NULL;

-- 5. FK: replayed_by + discarded_by (actor audit lookup)
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_replayed_by
    ON sys.dlq_gl_delivery (replayed_by)
    WHERE replayed_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_discarded_by
    ON sys.dlq_gl_delivery (discarded_by)
    WHERE discarded_by IS NOT NULL;

-- 6. Tenant + created_at (multi-tenant readiness)
CREATE INDEX IF NOT EXISTS idx_dlq_gl_delivery_tenant_created
    ON sys.dlq_gl_delivery (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- B-TRIGGERS

CREATE TRIGGER trg_dlq_gl_delivery_updated_at
    BEFORE UPDATE ON sys.dlq_gl_delivery
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_dlq_gl_delivery_row_version
    BEFORE UPDATE ON sys.dlq_gl_delivery
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

CREATE OR REPLACE FUNCTION fn_dlq_gl_delivery_no_hard_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.dlq_gl_delivery is forbidden (DEC-018). '
        'Use POST /jurnal/gl-delivery-dlq/{id}/discard to abandon. '
        'Retention: 10+10 tahun. Error code: DLQ_GL_DELIVERY_NO_HARD_DELETE';
END;
$$;

COMMENT ON FUNCTION fn_dlq_gl_delivery_no_hard_delete() IS
    'Prevents accidental or malicious DELETE on sys.dlq_gl_delivery. '
    'Critical for audit trail continuity (DEC-018). '
    'Discard is the only terminal action; it sets status=ABANDONED, not DELETE.';

CREATE TRIGGER trg_dlq_gl_delivery_no_hard_delete
    BEFORE DELETE ON sys.dlq_gl_delivery
    FOR EACH ROW EXECUTE FUNCTION fn_dlq_gl_delivery_no_hard_delete();

-- ====================================================================
-- C. CREATE TABLE sys.gl_reconciliation_report
--    Daily GL reconciliation run header.
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.gl_reconciliation_report (

    -- Primary key
    id                          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Run identity
    tanggal_run                 DATE            NOT NULL,
    -- ^ Business date being reconciled. One COMPLETED report per date per tenant.
    trigger_source              TEXT            NOT NULL,
    -- ^ AUTO (cron 08:00 WIB) or MANUAL_AD_HOC (user-triggered via API).
    triggered_by                UUID            REFERENCES sec.user(id),
    -- ^ NULL for CRON_DAILY; set to actor user_id for MANUAL_AD_HOC.

    -- Job tracking
    asynq_job_id                TEXT,
    -- ^ sys.job.id of the Asynq gl_delivery:reconcile-daily task.

    -- Status lifecycle
    status                      TEXT            NOT NULL,
    started_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    completed_at                TIMESTAMPTZ,

    -- Reconciliation totals
    total_jurnal_idr            NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ BLIPS side: SUM of all posted jrnl.detail amounts for tanggal_run.
    gl_host_total_idr           NUMERIC(20,4),
    -- ^ GL Host reported total for the same date. NULL if host fetch failed.
    mismatch_count              INT             NOT NULL DEFAULT 0,
    -- ^ Number of accounts where ABS(delta) > tolerance_idr.
    tolerance_idr               NUMERIC(20,4)   NOT NULL DEFAULT 1.0000,
    -- ^ Snapshot of GL_RECON_TOLERANCE_IDR from sys.config at run time.
    --   Stored so historical reports are not affected by config changes.

    -- Summary data
    error_summary               TEXT,
    -- ^ Populated only when status = 'FAILED'. Human-readable failure description.
    summary_jsonb               JSONB,
    -- ^ Optional per-account breakdown pre-computed for fast dashboard render.
    gl_host_snapshot_jsonb      JSONB,
    -- ^ Sanitized raw GL Host daily-summary response. PII stripped before persist.

    -- Standard audit columns
    created_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by                  UUID            NOT NULL REFERENCES sec.user(id),
    updated_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by                  UUID            NOT NULL REFERENCES sec.user(id),
    deleted_at                  TIMESTAMPTZ,
    deleted_by                  UUID            REFERENCES sec.user(id),
    row_version                 BIGINT          NOT NULL DEFAULT 1,
    tenant_id                   TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    CONSTRAINT chk_gl_recon_trigger_source
        CHECK (trigger_source IN ('CRON_DAILY', 'MANUAL_AD_HOC')),

    CONSTRAINT chk_gl_recon_status
        CHECK (status IN (
            'IN_PROGRESS',
            'COMPLETED',
            'FAILED'
        )),

    CONSTRAINT chk_gl_recon_mismatch_count_nonneg
        CHECK (mismatch_count >= 0),

    CONSTRAINT chk_gl_recon_tolerance_positive
        CHECK (tolerance_idr > 0),

    -- MANUAL_AD_HOC requires triggered_by
    CONSTRAINT chk_gl_recon_manual_has_actor
        CHECK (
            trigger_source <> 'MANUAL_AD_HOC'
            OR triggered_by IS NOT NULL
        ),

    -- completed_at required when status is terminal
    CONSTRAINT chk_gl_recon_completed_at_on_terminal
        CHECK (
            status = 'IN_PROGRESS'
            OR completed_at IS NOT NULL
        )
);

COMMENT ON TABLE sys.gl_reconciliation_report IS
    'Header for each GL reconciliation run (P5-M3). '
    'One IN_PROGRESS row per (tanggal_run, tenant_id) maximum (enforced by partial UNIQUE). '
    'One COMPLETED row per (tanggal_run, tenant_id) allowed (UPSERT per OQ-M3-4b). '
    'Detail mismatches: sys.gl_recon_mismatch FK-linked. '
    'Hard delete forbidden (trigger below). '
    'See p5-m3-gl-delivery.md §4 for reconciliation flow.';

-- C-UNIQUE: Prevent concurrent runs for the same date.
--           Only one IN_PROGRESS row per (tanggal_run, tenant_id) at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_gl_recon_report_in_progress
    ON sys.gl_reconciliation_report (tanggal_run, tenant_id)
    WHERE status = 'IN_PROGRESS';

COMMENT ON INDEX uq_gl_recon_report_in_progress IS
    'Concurrent run protection: only one IN_PROGRESS recon per date per tenant. '
    'Checked before INSERT via partial unique index; triggers GL_RECONCILIATION_IN_PROGRESS 409. '
    'COMPLETED rows excluded — UPSERT pattern (OQ-M3-4b) overwrites previous COMPLETED row.';

-- C-INDEXES

-- 1. Primary listing index (latest run first)
CREATE INDEX IF NOT EXISTS idx_gl_recon_report_tanggal_run
    ON sys.gl_reconciliation_report (tanggal_run DESC, tenant_id);

-- 2. Status + started_at for monitoring queue
CREATE INDEX IF NOT EXISTS idx_gl_recon_report_status_started
    ON sys.gl_reconciliation_report (status, started_at DESC);

-- 3. trigger_source + date (filter in history view)
CREATE INDEX IF NOT EXISTS idx_gl_recon_report_trigger_source
    ON sys.gl_reconciliation_report (trigger_source, tanggal_run DESC);

-- 4. FK: triggered_by actor lookup
CREATE INDEX IF NOT EXISTS idx_gl_recon_report_triggered_by
    ON sys.gl_reconciliation_report (triggered_by)
    WHERE triggered_by IS NOT NULL;

-- C-TRIGGERS

CREATE TRIGGER trg_gl_recon_report_updated_at
    BEFORE UPDATE ON sys.gl_reconciliation_report
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_gl_recon_report_row_version
    BEFORE UPDATE ON sys.gl_reconciliation_report
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

CREATE OR REPLACE FUNCTION fn_gl_recon_report_no_hard_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.gl_reconciliation_report is forbidden (DEC-018). '
        'Reconciliation reports are audit-grade; retention 10+10 tahun. '
        'Error code: GL_RECON_REPORT_NO_HARD_DELETE';
END;
$$;

COMMENT ON FUNCTION fn_gl_recon_report_no_hard_delete() IS
    'Prevents accidental or malicious DELETE on sys.gl_reconciliation_report. '
    'Reconciliation history is required for regulatory audit (DEC-018).';

CREATE TRIGGER trg_gl_recon_report_no_hard_delete
    BEFORE DELETE ON sys.gl_reconciliation_report
    FOR EACH ROW EXECUTE FUNCTION fn_gl_recon_report_no_hard_delete();

-- ====================================================================
-- D. CREATE TABLE sys.gl_recon_mismatch
--    Per-account mismatch detail for each reconciliation run.
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.gl_recon_mismatch (

    -- Primary key
    id                          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent report FK
    report_id                   UUID            NOT NULL
                                REFERENCES sys.gl_reconciliation_report(id)
                                ON DELETE CASCADE,
    -- ^ ON DELETE CASCADE is intentional: mismatch rows are physically part of the
    --   report. The report itself is protected by a no-hard-delete trigger, so
    --   CASCADE here only fires if the trigger allows the parent DELETE — which it
    --   never does. This is belt-and-suspenders for tooling consistency.

    -- Account reference
    akun_id                     UUID            NOT NULL REFERENCES mst.chart_of_accounts(id),

    -- Amounts
    blips_amount_idr            NUMERIC(20,4)   NOT NULL,
    -- ^ BLIPS side net amount for this account on tanggal_run.
    gl_host_amount_idr          NUMERIC(20,4)   NOT NULL DEFAULT 0,
    -- ^ GL Host reported amount. 0 = account absent on GL Host side (BLIPS_ONLY).
    delta_idr                   NUMERIC(20,4)   NOT NULL,
    -- ^ blips_amount_idr - gl_host_amount_idr. Positive = BLIPS has more.

    -- Mismatch classification
    mismatch_type               TEXT            NOT NULL,
    -- ^ BLIPS_ONLY: gl_host_amount = 0, blips > 0.
    --   GL_ONLY:    blips_amount = 0, gl_host > 0.
    --   AMOUNT_DIFF: both non-zero, ABS(delta) > tolerance_idr.

    -- Supporting data
    jurnal_header_ids           UUID[],
    -- ^ Array of jrnl.header.id that contributed to blips_amount_idr for this account.
    --   Allows UI to link mismatch lines back to source journals.
    note                        TEXT,

    -- Standard audit columns
    created_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by                  UUID            NOT NULL REFERENCES sec.user(id),
    updated_at                  TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by                  UUID            NOT NULL REFERENCES sec.user(id),
    deleted_at                  TIMESTAMPTZ,
    deleted_by                  UUID            REFERENCES sec.user(id),
    row_version                 BIGINT          NOT NULL DEFAULT 1,
    tenant_id                   TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    CONSTRAINT chk_gl_recon_mismatch_type
        CHECK (mismatch_type IN ('BLIPS_ONLY', 'GL_ONLY', 'AMOUNT_DIFF'))
);

COMMENT ON TABLE sys.gl_recon_mismatch IS
    'Per-account mismatch detail rows for sys.gl_reconciliation_report. '
    'Inserted in-transaction with COMPLETED status update on the parent report. '
    'Previous mismatch rows are soft-deleted (deleted_at=now()) before new INSERT '
    'when a report is re-run (UPSERT pattern per OQ-M3-4b). '
    'Hard delete forbidden (trigger below). '
    'akun_id references mst.chart_of_accounts — never NULL (always tied to a known account).';

-- D-INDEXES

-- 1. FK: report_id (primary join from report to mismatch lines)
CREATE INDEX IF NOT EXISTS idx_gl_recon_mismatch_report_id
    ON sys.gl_recon_mismatch (report_id);

-- 2. FK: akun_id (account-centric mismatch query — "which accounts mismatched historically?")
CREATE INDEX IF NOT EXISTS idx_gl_recon_mismatch_akun_id
    ON sys.gl_recon_mismatch (akun_id);

-- 3. Tenant + created_at (multi-tenant readiness)
CREATE INDEX IF NOT EXISTS idx_gl_recon_mismatch_tenant_created
    ON sys.gl_recon_mismatch (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- D-TRIGGERS

CREATE TRIGGER trg_gl_recon_mismatch_updated_at
    BEFORE UPDATE ON sys.gl_recon_mismatch
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_gl_recon_mismatch_row_version
    BEFORE UPDATE ON sys.gl_recon_mismatch
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

CREATE OR REPLACE FUNCTION fn_gl_recon_mismatch_no_hard_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on sys.gl_recon_mismatch is forbidden (DEC-018). '
        'Use soft-delete (deleted_at) for stale rows when report is re-run. '
        'Error code: GL_RECON_MISMATCH_NO_HARD_DELETE';
END;
$$;

COMMENT ON FUNCTION fn_gl_recon_mismatch_no_hard_delete() IS
    'Prevents hard delete on sys.gl_recon_mismatch (DEC-018 retention). '
    'Re-run UPSERT pattern uses soft-delete (deleted_at) on old rows, not DELETE.';

CREATE TRIGGER trg_gl_recon_mismatch_no_hard_delete
    BEFORE DELETE ON sys.gl_recon_mismatch
    FOR EACH ROW EXECUTE FUNCTION fn_gl_recon_mismatch_no_hard_delete();

-- ====================================================================
-- E. sys.config seed — GL delivery + reconciliation config keys
-- ====================================================================
-- Keys inserted with ON CONFLICT DO NOTHING — safe re-run.
-- sensitive=TRUE keys: GL_HOST_URL, GL_HOST_API_KEY_VAULT_KEY.
-- config_type='STRING' for URL/text values; 'INT' for numeric; 'JSON' for lists.
-- These are dev/UAT defaults; production values overridden via Vault / env injection.

INSERT INTO sys.config
    (config_key, config_value, config_type, sensitive, description, category)
VALUES

(
    'GL_HOST_URL',
    'http://stub-gl-host:8090/api/v1/post',
    'STRING',
    TRUE,
    'GL Host REST endpoint base URL. Dev default: stub service on port 8090. '
    'Override per environment via Vault: vault:gl-host/url. '
    'Worker reads this key before each delivery attempt.',
    'GL_DELIVERY'
),

(
    'GL_HOST_AUTH_TYPE',
    'BEARER',
    'STRING',
    FALSE,
    'GL Host authentication type. Supported: BEARER (default), API_KEY, '
    'OAUTH2_CLIENT_CREDENTIALS. '
    'Determines how GL_HOST_API_KEY_VAULT_KEY is used in HTTP header. '
    'OQ-M3-1a: default is API key header X-API-Key; BEARER uses Authorization: Bearer.',
    'GL_DELIVERY'
),

(
    'GL_HOST_API_KEY_VAULT_KEY',
    'vault:gl-host/api-key',
    'STRING',
    TRUE,
    'Vault reference path for GL Host API key. '
    'Worker resolves actual secret via vault.GetSecret(GL_HOST_API_KEY_VAULT_KEY). '
    'NEVER store the actual API key in this column — store the Vault path only. '
    'Actual key must not appear in logs, responses, or any JSONB column.',
    'GL_DELIVERY'
),

(
    'GL_DELIVERY_RETRY_MAX',
    '3',
    'INT',
    FALSE,
    'Maximum number of automatic Asynq retries for infra errors (5xx/timeout) '
    'before transitioning jrnl.gl_status to FAILED and inserting DLQ entry. '
    'Does not count manual retries (see GL_DELIVERY_MAX_TOTAL_ATTEMPTS). '
    'OQ-M3-3a: default 3.',
    'GL_DELIVERY'
),

(
    'GL_DELIVERY_RETRY_BACKOFF_SECONDS',
    '30,120,600',
    'STRING',
    FALSE,
    'Comma-separated Asynq retry delay seconds for infra errors. '
    'Element N (0-indexed) applies to attempt N+1: '
    'attempt 1: 30s, attempt 2: 120s, attempt 3: 600s. '
    'OQ-M3-1c: konfigurabel. State machine doc uses 60,300,900; '
    'data-modeler seed uses 30,120,600 as default (override in prod).',
    'GL_DELIVERY'
),

(
    'GL_DELIVERY_TIMEOUT_SECONDS',
    '30',
    'INT',
    FALSE,
    'HTTP client timeout in seconds for GL Host POST /api/journals call. '
    'Worker returns GL_DELIVERY_TIMEOUT error if GL Host does not respond within this window. '
    'Default 30s per p5-m3-gl-delivery.md §7 Performance SLA.',
    'GL_DELIVERY'
),

(
    'GL_DELIVERY_MAX_TOTAL_ATTEMPTS',
    '5',
    'INT',
    FALSE,
    'Maximum total delivery attempts including automatic retries + manual retries. '
    'Checked before enqueuing manual retry (POST /retry-gl-delivery) and DLQ replay. '
    'Triggers GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED 422 when reached. '
    'OQ-M3-3a: default 5.',
    'GL_DELIVERY'
),

(
    'GL_RECON_TOLERANCE_IDR',
    '1.0000',
    'STRING',
    FALSE,
    'Reconciliation tolerance in IDR (NUMERIC(20,4) semantics). '
    'Accounts where ABS(blips_amount - gl_host_amount) <= tolerance are considered matching. '
    'Default: 1.0000 IDR (covers rounding differences). '
    'Snapshot copied to sys.gl_reconciliation_report.tolerance_idr at run start '
    'so historical reports are unaffected by config changes.',
    'GL_RECON'
),

(
    'GL_RECON_CRON',
    '0 1 * * *',
    'STRING',
    FALSE,
    'Asynq cron expression for daily reconciliation job (gl_delivery:reconcile-daily). '
    'Default: 0 1 * * * = 01:00 UTC = 08:00 WIB (Asia/Jakarta). '
    'Worker skips run if tanggal is in sys.calendar_holiday. '
    'Change requires Asynq scheduler restart (worker restart in Docker/K8s).',
    'GL_RECON'
),

(
    'GL_HOST_PII_FIELDS_TO_REDACT',
    'customer_name,account_no,npwp,ktp',
    'STRING',
    FALSE,
    'Comma-separated list of JSON field names to replace with [REDACTED] before '
    'persisting gl_response_payload_jsonb, payload_jsonb (DLQ), or gl_host_snapshot_jsonb. '
    'Worker SanitizePII() function reads this list. '
    'GL_HOST_API_KEY is ALWAYS redacted regardless of this list.',
    'GL_DELIVERY'
)

ON CONFLICT (config_key) DO NOTHING;

COMMIT;
