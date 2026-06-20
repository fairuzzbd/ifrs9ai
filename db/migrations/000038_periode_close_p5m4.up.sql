-- migration: 0038 periode_close_p5m4
-- author: data-modeler
-- requires: 0001 (init_schema — mst.periode_buku, sys.config, sec.user FK targets,
--                              fn_update_updated_at, fn_increment_row_version),
--           0009 (periode_buku_schema_fix — row_version, audit cols, tenant_id on mst.periode_buku),
--           0035 (jurnal_engine_p5_m2 — jrnl.header, jrnl.gl_status),
--           0037 (gl_delivery_p5_m3 — sys.gl_reconciliation_report referenced by checklist)
-- description:
--   P5-M4 Periode Buku Close Workflow schema:
--   (A) ALTER mst.periode_buku — add 11 close-workflow tracking columns + step_up_token_ref;
--       upgrade ck_periode_status CHECK to include HARD_CLOSE_PENDING.
--   (B) CREATE TABLE sys.closing_checklist_snapshot — append-only audit-grade snapshot
--       of every 4-item closing checklist evaluation. Monthly partitioned.
--       BEFORE DELETE and BEFORE UPDATE triggers enforce immutability.
--   (C) Indexes on mst.periode_buku for status filter + grace window check.
--   (D) Indexes on sys.closing_checklist_snapshot (periode_id + transition + created_at,
--       all_passed partial, tenant_id).
--   (E) sys.config seed — 3 keys: SOFT_CLOSE_CHECKLIST_STALE_HOURS,
--       HARD_CLOSE_GRACE_WINDOW_HOURS, PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST.
--
-- PERIODE lock cascade enforcement decision (§4 of state machine doc):
--   Enforcement is APP-LAYER (PeriodeLockMiddleware — Gin middleware).
--   DB trigger on trx.*/jrnl.*/mst.instrumen is NOT implemented here because:
--     1. trx.transaction is partitioned; per-partition triggers are complex and brittle.
--     2. The allowlist (GL delivery retry, CORRECTION_PERIODE_CLOSED) is config-driven
--        and cannot be cleanly evaluated inside a PL/pgSQL trigger without sys.config reads.
--     3. App-layer SELECT FOR SHARE on mst.periode_buku gives the same guarantee with
--        simpler reasoning and no hidden trigger overhead.
--   DB-level protection retained: ck_periode_status CHECK constraint ensures only valid
--   status values can be written; status column is NOT NULL DEFAULT 'OPEN' — no NULL bypass.
--   See §4.2 of p5-m4-periode-close.md for middleware implementation notes.

BEGIN;

-- ====================================================================
-- A. ALTER mst.periode_buku — close-workflow tracking columns
-- ====================================================================

-- A1. Drop the old CHECK constraint (OPEN|SOFT_CLOSED|CLOSED)
--     and re-add including HARD_CLOSE_PENDING.
--     Name in 000001: ck_periode_status

ALTER TABLE mst.periode_buku
    DROP CONSTRAINT IF EXISTS ck_periode_status;

ALTER TABLE mst.periode_buku
    ADD CONSTRAINT ck_periode_status
        CHECK (status_periode IN (
            'OPEN',
            'SOFT_CLOSED',
            'HARD_CLOSE_PENDING',
            'CLOSED'
        ));

COMMENT ON CONSTRAINT ck_periode_status ON mst.periode_buku IS
    'P5-M4 state machine: OPEN → SOFT_CLOSED → HARD_CLOSE_PENDING → CLOSED. '
    'HARD_CLOSE_PENDING added (P5-M4, migration 000038). '
    'CLOSED = terminal; reopen only within grace window (CLOSED → SOFT_CLOSED, step-up MFA). '
    'Status is updated atomically with INSERT into sys.closing_checklist_snapshot.';

-- A2. Backfill: existing rows without status will keep their current value.
--     Any NULL status_periode (should not exist per NOT NULL DEFAULT, but guard here).
UPDATE mst.periode_buku
    SET status_periode = 'OPEN'
WHERE status_periode IS NULL;

-- A3. Soft-close request tracking columns.
--     soft_close_requested_by: actor who submitted the soft-close request (Maker, ROLE-AKUN-CTL).
--     Cleared (set to NULL) when HARD_CLOSE_PENDING → SOFT_CLOSED via reject.
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS soft_close_requested_by
        UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS soft_close_requested_at
        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS soft_close_request_reason
        TEXT;

COMMENT ON COLUMN mst.periode_buku.soft_close_requested_by IS
    'Actor (ROLE-AKUN-CTL Maker) who submitted POST /soft-close-request. '
    'NOT NULL when a pending soft-close request exists (status_periode = OPEN, pending approval). '
    'SoD enforcement: soft_close_approved_by MUST differ from this value (DEC-017). '
    'Cleared (NULL) when period is reopened to OPEN.';

COMMENT ON COLUMN mst.periode_buku.soft_close_requested_at IS
    'Timestamp of the soft-close request submission. '
    'Used to compute checklist staleness: if (now() - soft_close_requested_at) '
    '> SOFT_CLOSE_CHECKLIST_STALE_HOURS then re-run checklist at approve time.';

COMMENT ON COLUMN mst.periode_buku.soft_close_request_reason IS
    'Optional free-text catatan from the soft-close request body. '
    'Max 1000 chars (enforced at application layer). Stored for audit context.';

-- A4. Soft-close approve tracking columns.
--     soft_close_approved_by: actor who ran POST /soft-close-approve (Approver, ROLE-AKUN-CTL, SoD).
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS soft_close_approved_by
        UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS soft_close_approved_at
        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS soft_close_approve_reason
        TEXT;

COMMENT ON COLUMN mst.periode_buku.soft_close_approved_by IS
    'Actor (ROLE-AKUN-CTL Approver) who ran POST /soft-close-approve. '
    'SoD: MUST differ from soft_close_requested_by (DEC-017). '
    'Set atomically with status_periode = SOFT_CLOSED.';

COMMENT ON COLUMN mst.periode_buku.soft_close_approved_at IS
    'Equals tanggal_soft_close. Stored separately for explicit actor attribution '
    'and to preserve the approved-at timestamp even if tanggal_soft_close is later cleared.';

COMMENT ON COLUMN mst.periode_buku.soft_close_approve_reason IS
    'Optional comment from soft-close approve body. Max 1000 chars.';

-- A5. Hard-close request tracking columns.
--     hard_close_requested_by: actor who submitted POST /hard-close-request (ROLE-AKUN-CTL).
--     Cleared (NULL) when CFO rejects (HARD_CLOSE_PENDING → SOFT_CLOSED).
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS hard_close_requested_by
        UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS hard_close_requested_at
        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hard_close_request_reason
        TEXT;

COMMENT ON COLUMN mst.periode_buku.hard_close_requested_by IS
    'Actor (ROLE-AKUN-CTL) who submitted POST /hard-close-request. '
    'Set atomically with status_periode = HARD_CLOSE_PENDING. '
    'Cleared (NULL) by CFO hard-close-reject (HARD_CLOSE_PENDING → SOFT_CLOSED, OQ-M4-3a).';

COMMENT ON COLUMN mst.periode_buku.hard_close_requested_at IS
    'Timestamp of hard-close request. Cleared with hard_close_requested_by on CFO reject.';

COMMENT ON COLUMN mst.periode_buku.hard_close_request_reason IS
    'Optional catatan from hard-close request body. Max 1000 chars.';

-- A6. Hard-close approve tracking columns.
--     hard_close_approved_by: ROLE-CFO who executed POST /hard-close-approve with step-up MFA.
--     hard_close_grace_expires_at: computed at approve time as now() + HARD_CLOSE_GRACE_WINDOW_HOURS.
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS hard_close_approved_by
        UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS hard_close_approved_at
        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hard_close_approve_reason
        TEXT,
    ADD COLUMN IF NOT EXISTS hard_close_grace_expires_at
        TIMESTAMPTZ;

COMMENT ON COLUMN mst.periode_buku.hard_close_approved_by IS
    'Actor (ROLE-CFO, DEC-026 MFA mandatory) who executed POST /hard-close-approve. '
    'step-up MFA (DEC-027) is required; step_up_token_ref stores the hash reference. '
    'Set atomically with status_periode = CLOSED.';

COMMENT ON COLUMN mst.periode_buku.hard_close_approved_at IS
    'Equals tanggal_hard_close. Stored separately for explicit attribution.';

COMMENT ON COLUMN mst.periode_buku.hard_close_approve_reason IS
    'CFO comment from hard-close approve body. Max 1000 chars.';

COMMENT ON COLUMN mst.periode_buku.hard_close_grace_expires_at IS
    'Reopen grace window expiry: now() + HARD_CLOSE_GRACE_WINDOW_HOURS (default 48h) at approve time. '
    'CLOSED → SOFT_CLOSED reopen is only permitted while now() < hard_close_grace_expires_at. '
    'After expiry: 423 PERIODE_GRACE_EXPIRED. '
    'Global (not per-period): value of HARD_CLOSE_GRACE_WINDOW_HOURS is read from sys.config at '
    'hard-close-approve time and snapshotted here so config changes do not affect past periods. '
    'NULL while status_periode ≠ CLOSED.';

-- A7. Step-up MFA token reference.
--     Stores SHA-256 hash of the step-up token ID — NEVER the raw token.
--     Used for audit trail: aud.audit_log.after_jsonb.step_up_token_ref matches this value.
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS step_up_token_ref
        VARCHAR(100);

COMMENT ON COLUMN mst.periode_buku.step_up_token_ref IS
    'SHA-256 hash (hex-encoded) of the X-Step-Up-Token ID used at hard-close-approve '
    'or reopen-approve (CLOSED → SOFT_CLOSED). NEVER stores plaintext MFA code or raw token. '
    'Value format: hex(sha256(tokenId)). Max 100 chars (SHA-256 hex = 64 chars; padded for algo prefix). '
    'Set to NULL for transitions that do not require step-up MFA (e.g., reopen SOFT_CLOSED → OPEN). '
    'Matches aud.audit_log.after_jsonb.step_up_token_ref for cross-audit verification.';

-- A8. Reopen reason.
--     reopened_reason already exists from migration 000001 — verified present.
--     Adding reopen_reason as a synonym that is explicitly set at reopen-request time.
--     (reopened_reason from 000001 is set at reopen-approve time; reopen_reason is set at request).
ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS reopen_reason
        TEXT;

COMMENT ON COLUMN mst.periode_buku.reopen_reason IS
    'Mandatory reason provided at POST /reopen-request. minLength 30, maxLength 2000 (app-enforced). '
    'Persisted in aud.audit_log.after_jsonb.reason as well (DEC-018). '
    'Distinct from reopened_reason (migration 000001) which was set at approve time; '
    'reopen_reason is set at request time (the "why" the CFO is initiating). '
    'Not NULL after reopen-request is received; NULL if period has never had a reopen request.';

-- A9. FK indexes on all new UUID reference columns.

CREATE INDEX IF NOT EXISTS idx_periode_buku_soft_close_requested_by
    ON mst.periode_buku (soft_close_requested_by)
    WHERE soft_close_requested_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_periode_buku_soft_close_approved_by
    ON mst.periode_buku (soft_close_approved_by)
    WHERE soft_close_approved_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_periode_buku_hard_close_requested_by
    ON mst.periode_buku (hard_close_requested_by)
    WHERE hard_close_requested_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_periode_buku_hard_close_approved_by
    ON mst.periode_buku (hard_close_approved_by)
    WHERE hard_close_approved_by IS NOT NULL;

-- ====================================================================
-- C. Indexes on mst.periode_buku for close-workflow queries
-- ====================================================================

-- C1. Status filter for PeriodeLockMiddleware (every mutating request checks this).
--     Replaces ix_periode_status from 000001 with partial guard (deleted_at IS NULL).
CREATE INDEX IF NOT EXISTS idx_periode_buku_status
    ON mst.periode_buku (status_periode)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_periode_buku_status IS
    'PeriodeLockMiddleware hot path: SELECT status_periode WHERE id = $1 AND deleted_at IS NULL. '
    'Also used by closing checklist endpoint to filter by status. '
    'Partial (deleted_at IS NULL) to exclude archived periods from active path.';

-- C2. Grace window expiry index — reopen-request grace check query:
--     SELECT id, hard_close_grace_expires_at FROM mst.periode_buku
--     WHERE status_periode = 'CLOSED' AND deleted_at IS NULL AND id = $1
--     Partial on status_periode = 'CLOSED' because only CLOSED periods have a grace window.
CREATE INDEX IF NOT EXISTS idx_periode_buku_grace_expires
    ON mst.periode_buku (hard_close_grace_expires_at)
    WHERE status_periode = 'CLOSED' AND deleted_at IS NULL;

COMMENT ON INDEX idx_periode_buku_grace_expires IS
    'Reopen grace window check: only CLOSED periods have hard_close_grace_expires_at set. '
    'Supports POST /reopen-request validation: now() < hard_close_grace_expires_at check. '
    'Also used by monitoring query: "which CLOSED periods are still within grace window?".';

-- C3. Status + tahun_buku + bulan for GET /reports/status-periode list + sort.
--     Extends idx_periode_buku_active (migration 000009) — already covers (status_periode, tahun_buku, bulan)
--     but without explicit partial guard referencing deleted_at. Create with IF NOT EXISTS; 000009 index
--     has different name (idx_periode_buku_active) so this is a net-new index with a more descriptive name.
CREATE INDEX IF NOT EXISTS idx_periode_buku_status_tahun_bulan
    ON mst.periode_buku (status_periode, tahun_buku, bulan)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_periode_buku_status_tahun_bulan IS
    'GET /reports/status-periode list: filter by status_periode + tahun_buku + bulan. '
    'Sort by tanggal_akhir, tanggal_hard_close also served by this scan (index-only if projecting id). '
    'Performance target: ≤ 500 ms for paginated list (cursor on (tahun_buku, bulan, id)).';

-- ====================================================================
-- B. CREATE TABLE sys.closing_checklist_snapshot
--    Append-only audit-grade snapshot of closing checklist per transition.
--    Monthly range-partitioned on evaluated_at.
-- ====================================================================

CREATE TABLE IF NOT EXISTS sys.closing_checklist_snapshot (

    -- Primary key
    id                      UUID            NOT NULL DEFAULT gen_random_uuid(),

    -- Parent reference
    periode_buku_id         UUID            NOT NULL REFERENCES mst.periode_buku(id),
    -- ^ FK to mst.periode_buku; never NULL; links snapshot to the period being transitioned.

    -- Transition identity
    transition              VARCHAR(30)     NOT NULL,
    -- ^ Which state-machine transition triggered this checklist evaluation:
    --   SOFT_CLOSE_REQUEST   — POST /soft-close-request
    --   SOFT_CLOSE_APPROVE   — POST /soft-close-approve (fresh run or re-run if stale)
    --   HARD_CLOSE_REQUEST   — POST /hard-close-request
    --   HARD_CLOSE_APPROVE   — POST /hard-close-approve
    --   REOPEN_REQUEST       — POST /reopen-request
    --   REOPEN_APPROVE       — POST /reopen-approve
    --   MANUAL_CHECK         — GET /closing-checklist (explicit user request, not a state transition)

    trigger_action          TEXT            NOT NULL,
    -- ^ Enum mirrors transition but can carry additional context for MANUAL_CHECK runs.
    --   Values: SOFT_CLOSE_REQUEST, SOFT_CLOSE_APPROVE, HARD_CLOSE_REQUEST, HARD_CLOSE_APPROVE,
    --           REOPEN_REQUEST, REOPEN_APPROVE, MANUAL_CHECK.

    -- Evaluation identity
    evaluated_at            TIMESTAMPTZ     NOT NULL DEFAULT now(),
    evaluated_by            UUID            NOT NULL,
    -- ^ actor_user_id from JWT. NOT FK-constrained to allow advisory rows inserted via
    --   context.Background() child when main tx is aborted (advisory audit events per §7).

    actor_role              TEXT            NOT NULL,
    -- ^ JWT roles[0] at time of evaluation. Stored for audit context.

    -- Checklist results
    overall_status          TEXT            NOT NULL,
    -- ^ PASSED: all 4 items passed (all_passed = true, transition_status = APPROVED).
    --   FAILED: ≥ 1 item failed (all_passed = false, transition_status = REJECTED).
    --   REJECTED: transition rejected for reasons other than checklist (e.g. SoD violation,
    --             stale + re-run, optimistic lock) — items_jsonb still populated if checklist ran.

    all_passed              BOOLEAN         NOT NULL,
    -- ^ TRUE iff all 4 checklist items have passed = true. Denormalized from items_jsonb
    --   for efficient partial index queries.

    transition_status       VARCHAR(20)     NOT NULL,
    -- ^ APPROVED: transition succeeded and state was updated.
    --   REJECTED: transition blocked (checklist failed, SoD, grace expired, etc.).
    --   Stored so the approval/rejection trail is recoverable without parsing items_jsonb.

    checklist_jsonb         JSONB           NOT NULL,
    -- ^ Canonical snapshot of the 4-item closing checklist at evaluation time.
    --   Format:
    --   {
    --     "evaluated_at": "2026-06-30T17:00:00+07:00",
    --     "items": [
    --       { "key": "PENDING_APPROVAL_ZERO", "label": "...", "passed": true,  "detail": "..." },
    --       { "key": "JURNAL_BALANCED",        "label": "...", "passed": true,  "detail": "..." },
    --       { "key": "GL_DELIVERED",           "label": "...", "passed": false, "detail": "..." },
    --       { "key": "RECON_PASS",             "label": "...", "passed": true,  "detail": "..." }
    --     ]
    --   }
    --   MANUAL_CHECK snapshots for REOPEN_REQUEST/REOPEN_APPROVE may omit items if no checklist
    --   was re-run (e.g. empty items array with a note field).

    outcome_jsonb           JSONB,
    -- ^ Optional outcome context if the transition mutated state.
    --   For APPROVED transitions: { "new_status_periode": "SOFT_CLOSED", "row_version": 6 }.
    --   For REJECTED transitions: { "rejection_reason": "SoD violation", "error_code": "SOD_VIOLATION" }.
    --   NULL for MANUAL_CHECK runs (no mutation).

    -- Minimal audit columns (append-only — NO updated_at, updated_by, deleted_at, row_version)
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    -- ^ Same as evaluated_by. Kept separate to match audit convention across tables.
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    CONSTRAINT chk_checklist_snap_transition
        CHECK (transition IN (
            'SOFT_CLOSE_REQUEST',
            'SOFT_CLOSE_APPROVE',
            'HARD_CLOSE_REQUEST',
            'HARD_CLOSE_APPROVE',
            'REOPEN_REQUEST',
            'REOPEN_APPROVE',
            'MANUAL_CHECK'
        )),

    CONSTRAINT chk_checklist_snap_trigger_action
        CHECK (trigger_action IN (
            'SOFT_CLOSE_REQUEST',
            'SOFT_CLOSE_APPROVE',
            'HARD_CLOSE_REQUEST',
            'HARD_CLOSE_APPROVE',
            'REOPEN_REQUEST',
            'REOPEN_APPROVE',
            'MANUAL_CHECK'
        )),

    CONSTRAINT chk_checklist_snap_overall_status
        CHECK (overall_status IN ('PASSED', 'FAILED', 'REJECTED')),

    CONSTRAINT chk_checklist_snap_transition_status
        CHECK (transition_status IN ('APPROVED', 'REJECTED')),

    -- all_passed = TRUE requires overall_status = PASSED
    CONSTRAINT chk_checklist_snap_passed_consistency
        CHECK (
            all_passed = FALSE
            OR overall_status = 'PASSED'
        ),

    -- APPROVED transition_status requires all_passed = TRUE (checklist must pass to approve)
    CONSTRAINT chk_checklist_snap_approved_requires_passed
        CHECK (
            transition_status <> 'APPROVED'
            OR all_passed = TRUE
        )

) PARTITION BY RANGE (evaluated_at);

COMMENT ON TABLE sys.closing_checklist_snapshot IS
    'Append-only audit-grade snapshot of closing checklist evaluation for each mst.periode_buku '
    'state transition (P5-M4). Partitioned by month (evaluated_at). '
    'INSERT is done in-transaction with UPDATE mst.periode_buku for APPROVED transitions. '
    'For REJECTED advisory events (main tx aborted), INSERT runs in context.Background() child tx. '
    'No UPDATE or DELETE permitted (triggers below). '
    'Retention: same as aud.audit_log — 10+10 tahun (DEC-018). '
    'See p5-m4-periode-close.md §2 for snapshot per-transition rules and §3 for checklist items.';

-- B-PARTITIONS: Create initial monthly partitions for current + next 2 months.
-- Additional partitions are created by pg_partman or manual migration as needed.

CREATE TABLE IF NOT EXISTS sys.closing_checklist_snapshot_y2026m06
    PARTITION OF sys.closing_checklist_snapshot
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS sys.closing_checklist_snapshot_y2026m07
    PARTITION OF sys.closing_checklist_snapshot
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS sys.closing_checklist_snapshot_y2026m08
    PARTITION OF sys.closing_checklist_snapshot
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

-- Default partition for rows that fall outside named partitions.
-- pg_partman will manage future monthly partitions; this catches overflow.
CREATE TABLE IF NOT EXISTS sys.closing_checklist_snapshot_default
    PARTITION OF sys.closing_checklist_snapshot
    DEFAULT;

-- B-PK: Primary key must include the partition key (evaluated_at) for partitioned tables.
ALTER TABLE sys.closing_checklist_snapshot
    ADD CONSTRAINT pk_closing_checklist_snapshot PRIMARY KEY (id, evaluated_at);

-- ====================================================================
-- D. Indexes on sys.closing_checklist_snapshot
-- ====================================================================

-- D1. Primary lookup: checklist history for a period, most-recent first.
--     Hot path: GET /closing-checklist for CLOSED period returns last snapshot.
--     Hot path: stale check at soft-close-approve reads latest SOFT_CLOSE_REQUEST snapshot.
CREATE INDEX IF NOT EXISTS idx_checklist_snap_periode_transition_created
    ON sys.closing_checklist_snapshot (periode_buku_id, transition, created_at DESC);

COMMENT ON INDEX idx_checklist_snap_periode_transition_created IS
    'Primary checklist lookup: newest snapshot per (periode_buku_id, transition). '
    'Used by stale check (SOFT_CLOSE_APPROVE): '
    'SELECT created_at WHERE periode_buku_id = $1 AND transition = ''SOFT_CLOSE_REQUEST'' '
    'ORDER BY created_at DESC LIMIT 1. '
    'Also used by GET /closing-checklist to return last snapshot for CLOSED periods.';

-- D2. all_passed partial index — fast filter for "find failed evaluations" in monitoring.
CREATE INDEX IF NOT EXISTS idx_checklist_snap_periode_all_passed
    ON sys.closing_checklist_snapshot (periode_buku_id, all_passed, created_at DESC)
    WHERE all_passed = FALSE;

COMMENT ON INDEX idx_checklist_snap_periode_all_passed IS
    'Monitoring query: "show failed checklist evaluations for period X". '
    'Partial (all_passed = FALSE) keeps index small — PASSED rows are the majority.';

-- D3. overall_status partial for FAILED rows — alert/monitoring dashboard.
CREATE INDEX IF NOT EXISTS idx_checklist_snap_overall_status_failed
    ON sys.closing_checklist_snapshot (evaluated_at DESC, tenant_id)
    WHERE overall_status = 'FAILED';

COMMENT ON INDEX idx_checklist_snap_overall_status_failed IS
    'Monitoring: list all FAILED checklist evaluations across tenants, newest first. '
    'Partial (overall_status = FAILED) avoids index bloat from PASSED majority.';

-- D4. trigger_action index for filtering by action type.
CREATE INDEX IF NOT EXISTS idx_checklist_snap_trigger_action
    ON sys.closing_checklist_snapshot (trigger_action, evaluated_at DESC);

-- D5. Tenant + created_at for multi-tenant readiness.
CREATE INDEX IF NOT EXISTS idx_checklist_snap_tenant_created
    ON sys.closing_checklist_snapshot (tenant_id, created_at DESC);

-- ====================================================================
-- B-TRIGGERS: Append-only enforcement on sys.closing_checklist_snapshot
-- ====================================================================

-- Prevent any UPDATE (row is immutable once inserted).
CREATE OR REPLACE FUNCTION fn_checklist_snapshot_no_update()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'sys.closing_checklist_snapshot is append-only. '
        'UPDATE is not permitted (DEC-018). '
        'Each state transition inserts a new snapshot row. '
        'Error code: CHECKLIST_SNAPSHOT_APPEND_ONLY';
END;
$$;

COMMENT ON FUNCTION fn_checklist_snapshot_no_update() IS
    'Append-only guard for sys.closing_checklist_snapshot. '
    'Mirrors aud.audit_log immutability protection (security-baseline.md §audit-trail). '
    'Advisory audit rows (written via context.Background() on aborted tx) are INSERT only.';

CREATE OR REPLACE TRIGGER trg_checklist_snapshot_no_update
    BEFORE UPDATE ON sys.closing_checklist_snapshot
    FOR EACH ROW EXECUTE FUNCTION fn_checklist_snapshot_no_update();

-- Prevent DELETE (retention 10+10 tahun, DEC-018).
CREATE OR REPLACE FUNCTION fn_checklist_snapshot_no_delete()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'sys.closing_checklist_snapshot is append-only. '
        'DELETE is not permitted (DEC-018, retention 10+10 tahun). '
        'Archive to cold storage (MinIO) after 5 years instead. '
        'Error code: CHECKLIST_SNAPSHOT_NO_DELETE';
END;
$$;

COMMENT ON FUNCTION fn_checklist_snapshot_no_delete() IS
    'Hard-delete guard for sys.closing_checklist_snapshot. '
    'Identical pattern to aud.audit_log and sys.dlq_gl_delivery protection (migration 000037). '
    'Cold-storage archive is the only permitted data movement after retention window.';

CREATE OR REPLACE TRIGGER trg_checklist_snapshot_no_delete
    BEFORE DELETE ON sys.closing_checklist_snapshot
    FOR EACH ROW EXECUTE FUNCTION fn_checklist_snapshot_no_delete();

-- ====================================================================
-- E. sys.config seed — P5-M4 period close config keys
-- ====================================================================
-- Keys inserted with ON CONFLICT DO NOTHING — safe re-run.
-- All values are dev/UAT defaults; override per environment via sys.config update
-- (restricted to ROLE-IT-ADMIN; see permission matrix in personas.md).

INSERT INTO sys.config
    (config_key, config_value, config_type, sensitive, description, category)
VALUES

(
    'SOFT_CLOSE_CHECKLIST_STALE_HOURS',
    '24',
    'INT',
    FALSE,
    'Number of hours after a soft-close request before the checklist snapshot is considered '
    'stale. If (now() - closing_checklist_snapshot.created_at) > this value when '
    'POST /soft-close-approve is called, the server re-runs the 4-item checklist. '
    'If re-run fails: 422 CLOSING_CHECKLIST_FAILED. '
    'Default 24 hours (OQ-M4-1c RESOLVED). '
    'Konfigurabel: update via sys.config, no service restart needed (read per request).',
    'PERIODE_CLOSE'
),

(
    'HARD_CLOSE_GRACE_WINDOW_HOURS',
    '48',
    'INT',
    FALSE,
    'Grace window in hours from tanggal_hard_close during which ROLE-CFO may reopen '
    'a CLOSED period to SOFT_CLOSED (with step-up MFA, DEC-027). '
    'After expiry: 423 PERIODE_GRACE_EXPIRED — reopen not possible via API; '
    'escalate manually per BRD §3 RACI. '
    'Global (applies to all periods — not configurable per-period, OQ-M4-3b RESOLVED). '
    'Value is snapshot-copied to mst.periode_buku.hard_close_grace_expires_at at hard-close-approve '
    'time so changing this config does not retroactively affect already-closed periods. '
    'Default 48 hours.',
    'PERIODE_CLOSE'
),

(
    'PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST',
    'JURNAL_RETRY_GL_DELIVERY,CORRECTION_PERIODE_CLOSED',
    'STRING',
    FALSE,
    'Comma-separated list of action codes that PeriodeLockMiddleware allows to pass through '
    'when status_periode IN (''SOFT_CLOSED'', ''HARD_CLOSE_PENDING''). '
    'All other mutating endpoints receive 423 PERIODE_SOFT_CLOSED. '
    'JURNAL_RETRY_GL_DELIVERY: POST /jurnal/header/{id}/retry-gl-delivery (P5-M3 GL delivery retry). '
    'CORRECTION_PERIODE_CLOSED: posting journal with event code CORRECTION_PERIODE_CLOSED (DEC-036, '
    'cure evaluation by ROLE-RISK). '
    'PeriodeLockMiddleware reads this key per request (no cache). '
    'Format: comma-separated, no spaces, case-sensitive.',
    'PERIODE_CLOSE'
)

ON CONFLICT (config_key) DO NOTHING;

COMMIT;
