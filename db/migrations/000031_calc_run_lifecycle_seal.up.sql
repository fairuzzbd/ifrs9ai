-- migration: 0031 calc_run_lifecycle_seal
-- author: data-modeler
-- requires: 0001 (init_schema — sec.user, ecl schema),
--           0004 (sys_job_idempotency — sys.job),
--           0005 (audit_log_hardening — fn_ecl_no_hard_delete, fn_update_updated_at),
--           0007 (workflow_engine),
--           0009 (periode_buku_schema_fix — mst.periode_buku),
--           0029 (ecl_core_tables — ecl.calc_result_line, fn_ecl_calc_no_modify_when_sealed),
--           0030 (calc_result_line_formula_version)
-- description:
--   P4-M8 Calc Run Lifecycle + Seal:
--   (A) CREATE TABLE ecl.calc_run — run-level header (new entity, separate from per-instrument
--       ecl.calc_header). Owns the lifecycle: DRAFT → IN_PROGRESS → COMPLETED →
--       SEAL_REQUESTED → SEALED (terminal). Cancel and reject paths included.
--       4-eyes seal: ROLE-RISK requests, ROLE-ALCO/CFO approves (SoD enforced by CHECK).
--       Trigger fn_ecl_calc_run_no_modify_when_sealed blocks all non-audit-col UPDATEs
--       after sealed_at IS NOT NULL and all DELETEs always.
--   (B) ALTER TABLE ecl.calc_result_line — resolve OQ-M7-2: add FK constraint
--       calc_run_id → ecl.calc_run(id) now that the target table exists (UUID PK).
--   (C) Indexes for lifecycle guard queries, approval queue, job-progress lookup.

BEGIN;

-- ====================================================================
-- A. ecl.calc_run — run-level ECL computation header
-- ====================================================================

CREATE TABLE ecl.calc_run (
    id                          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Period & scope identity
    periode_id                  TEXT        NOT NULL,
    -- FK to mst.periode_buku — validated at application layer.
    -- DB-level FK omitted for same cross-schema type-mismatch reason documented in 000009.
    -- Application enforces existence + status check before INSERT.

    evaluation_date             DATE        NOT NULL,
    scope                       TEXT        NOT NULL DEFAULT 'ALL_ACTIVE'
                                    CONSTRAINT chk_ecl_calc_run_scope
                                    CHECK (scope IN ('ALL_ACTIVE')),

    -- Run status (state machine per docs/state-machines/p4-m8-calc-run.md §1)
    status                      TEXT        NOT NULL DEFAULT 'DRAFT'
                                    CONSTRAINT chk_ecl_calc_run_status
                                    CHECK (status IN (
                                        'DRAFT',
                                        'IN_PROGRESS',
                                        'COMPLETED',
                                        'COMPLETED_WITH_ERRORS',
                                        'SEAL_REQUESTED',
                                        'SEALED',
                                        'CANCELLED',
                                        'SEAL_REJECTED'
                                    )),

    -- Asynq job linkage (sys.job.id is TEXT / ULID)
    asynq_job_id                TEXT        REFERENCES sys.job(id) ON DELETE RESTRICT,

    -- Progress counters (updated by Asynq worker via CalcRunService.markProgress)
    total_instrumen_count       INT         NOT NULL DEFAULT 0,
    processed_count             INT         NOT NULL DEFAULT 0,
    error_count                 INT         NOT NULL DEFAULT 0,

    -- Timing
    started_at                  TIMESTAMPTZ,
    completed_at                TIMESTAMPTZ,

    -- Parameter snapshot frozen atomically at POST /start
    -- Full snapshot of all APPROVED ECL parameters (bobot, PD, LGD, FL, LPS, kurs).
    -- Immutable after status leaves DRAFT. Application-layer guard + sealed trigger both enforce.
    parameter_snapshot_jsonb    JSONB,

    -- ----------------------------------------------------------------
    -- Seal workflow (4-eyes: ROLE-RISK request → ROLE-ALCO/CFO approve)
    -- ----------------------------------------------------------------
    seal_requested_by           UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    seal_requested_at           TIMESTAMPTZ,
    seal_request_comment        TEXT,

    -- seal_approved_by = the approver who called POST /seal/approve
    sealed_by                   UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    sealed_at                   TIMESTAMPTZ,
    signature_hash_seal         BYTEA,
    -- SHA-256(approver_id || "|" || calc_run_id || "|" || sealed_at::RFC3339Nano || "|" || comment)
    -- Verified by cmd/audit-verify. stored as hex-encoded bytes.

    -- Seal reject tracking (status returns to COMPLETED for re-request)
    seal_rejected_by            UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    seal_rejected_at            TIMESTAMPTZ,
    seal_reject_reason          TEXT,

    -- ----------------------------------------------------------------
    -- Cancel tracking
    -- ----------------------------------------------------------------
    cancelled_at                TIMESTAMPTZ,
    cancelled_by                UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    cancel_reason               TEXT
                                    CONSTRAINT chk_ecl_calc_run_cancel_reason
                                    CHECK (
                                        cancelled_at IS NULL
                                        OR (cancel_reason IS NOT NULL
                                            AND length(cancel_reason) >= 30)
                                    ),

    -- ----------------------------------------------------------------
    -- Cross-run audit linkage (supersede tracking — no auto-logic)
    -- ----------------------------------------------------------------
    superseded_by_run_id        UUID        REFERENCES ecl.calc_run(id) ON DELETE RESTRICT,

    -- ----------------------------------------------------------------
    -- SoD Constraints — 4-eyes: requester ≠ approver (enforced server-side + DB)
    -- ----------------------------------------------------------------
    CONSTRAINT chk_ecl_calc_run_sod_seal
        CHECK (
            sealed_by IS NULL
            OR (sealed_by IS DISTINCT FROM created_by
                AND sealed_by IS DISTINCT FROM seal_requested_by)
        ),

    -- Consistency: sealed_at set ↔ status = 'SEALED'
    CONSTRAINT chk_ecl_calc_run_sealed_consistency
        CHECK (
            (status = 'SEALED') = (sealed_at IS NOT NULL)
        ),

    -- Standard audit columns (db-conventions.md — mandatory on every table)
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by                  UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by                  UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at                  TIMESTAMPTZ,
    deleted_by                  UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version                 BIGINT      NOT NULL DEFAULT 1,
    tenant_id                   TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE ecl.calc_run IS
    'Run-level ECL computation header. One row per user-initiated ECL run per periode_id. '
    'State machine: DRAFT→IN_PROGRESS→COMPLETED→SEAL_REQUESTED→SEALED (terminal). '
    'Cancel path: DRAFT|IN_PROGRESS→CANCELLED (terminal). '
    'Seal reject path: SEAL_REQUESTED→COMPLETED (re-requestable). '
    'parameter_snapshot_jsonb frozen atomically at POST /start. '
    '4-eyes seal: ROLE-RISK requests, ROLE-ALCO or ROLE-CFO approves (SoD: sealed_by ≠ created_by AND ≠ seal_requested_by). '
    'Trigger fn_ecl_calc_run_no_modify_when_sealed blocks non-audit-col UPDATEs after seal + all DELETEs. '
    'Created by migration 000031 per docs/state-machines/p4-m8-calc-run.md §9 hand-off.';

COMMENT ON COLUMN ecl.calc_run.parameter_snapshot_jsonb IS
    'Full snapshot of all APPROVED ECL parameters at /start time (atomic). '
    'Structure: ParameterSnapshot schema in api/openapi/app-c-calc-run.yaml. '
    'Sources: mst.bobot_skenario, mst.pd_pefindo, mst.lgd_basel, mst.impact_pd, '
    'mst.impact_mev_pd (3 scenarios), mst.lps_coverage, mst.kurs. '
    'MUST NOT be updated after status leaves DRAFT. '
    'Application guard: service rejects update. DB guard: sealed trigger (after SEALED).';

COMMENT ON COLUMN ecl.calc_run.signature_hash_seal IS
    'SHA-256 of seal approval payload: '
    'hex(sha256(approver_id || "|" || calc_run_id || "|" || sealed_at::RFC3339Nano || "|" || comment)). '
    'Verified by cmd/audit-verify for PSAK 71 audit trail.';

COMMENT ON COLUMN ecl.calc_run.asynq_job_id IS
    'FK → sys.job(id) (TEXT / ULID). Set at POST /start when Asynq task is dispatched. '
    'Bridges to UX progress panel (GET /api/v1/jobs/{jobId}).';

COMMENT ON COLUMN ecl.calc_run.seal_rejected_at IS
    'Set when ROLE-ALCO/CFO rejects seal request. Status returns to COMPLETED for re-request. '
    'seal_requested_by/at are cleared by application after rejection to allow re-request.';

-- ====================================================================
-- A-1. Trigger: fn_ecl_calc_run_no_modify_when_sealed
--
-- Behaviour (updated from the stub in migration 000029 which guarded ecl.calc_header):
--   - BEFORE UPDATE: allow only row_version + updated_at + updated_by (audit-col-only updates).
--     Block ALL other column changes when sealed_at IS NOT NULL.
--   - BEFORE DELETE: block always (no hard delete on ecl.* per DEC-018).
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_ecl_calc_run_no_modify_when_sealed()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    -- Hard-delete guard — always block regardless of sealed status
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'ECL_CALC_RUN_NO_HARD_DELETE: DELETE on ecl.calc_run(id=%) is forbidden. '
            'Use soft-delete (set deleted_at). DEC-018 audit trail requirement.',
            OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    -- Sealed immutability guard — block non-audit-col UPDATEs after seal
    IF TG_OP = 'UPDATE' AND OLD.sealed_at IS NOT NULL THEN
        -- Allow audit bookkeeping columns to be updated even after sealing.
        -- All other column changes are forbidden on a sealed calc run.
        IF (
            NEW.id                          IS NOT DISTINCT FROM OLD.id
            AND NEW.periode_id              IS NOT DISTINCT FROM OLD.periode_id
            AND NEW.evaluation_date         IS NOT DISTINCT FROM OLD.evaluation_date
            AND NEW.scope                   IS NOT DISTINCT FROM OLD.scope
            AND NEW.status                  IS NOT DISTINCT FROM OLD.status
            AND NEW.asynq_job_id            IS NOT DISTINCT FROM OLD.asynq_job_id
            AND NEW.total_instrumen_count   IS NOT DISTINCT FROM OLD.total_instrumen_count
            AND NEW.processed_count         IS NOT DISTINCT FROM OLD.processed_count
            AND NEW.error_count             IS NOT DISTINCT FROM OLD.error_count
            AND NEW.started_at              IS NOT DISTINCT FROM OLD.started_at
            AND NEW.completed_at            IS NOT DISTINCT FROM OLD.completed_at
            AND NEW.parameter_snapshot_jsonb IS NOT DISTINCT FROM OLD.parameter_snapshot_jsonb
            AND NEW.seal_requested_by       IS NOT DISTINCT FROM OLD.seal_requested_by
            AND NEW.seal_requested_at       IS NOT DISTINCT FROM OLD.seal_requested_at
            AND NEW.seal_request_comment    IS NOT DISTINCT FROM OLD.seal_request_comment
            AND NEW.sealed_by               IS NOT DISTINCT FROM OLD.sealed_by
            AND NEW.sealed_at               IS NOT DISTINCT FROM OLD.sealed_at
            AND NEW.signature_hash_seal     IS NOT DISTINCT FROM OLD.signature_hash_seal
            AND NEW.seal_rejected_by        IS NOT DISTINCT FROM OLD.seal_rejected_by
            AND NEW.seal_rejected_at        IS NOT DISTINCT FROM OLD.seal_rejected_at
            AND NEW.seal_reject_reason      IS NOT DISTINCT FROM OLD.seal_reject_reason
            AND NEW.cancelled_at            IS NOT DISTINCT FROM OLD.cancelled_at
            AND NEW.cancelled_by            IS NOT DISTINCT FROM OLD.cancelled_by
            AND NEW.cancel_reason           IS NOT DISTINCT FROM OLD.cancel_reason
            AND NEW.superseded_by_run_id    IS NOT DISTINCT FROM OLD.superseded_by_run_id
            AND NEW.created_at              IS NOT DISTINCT FROM OLD.created_at
            AND NEW.created_by              IS NOT DISTINCT FROM OLD.created_by
            AND NEW.deleted_at              IS NOT DISTINCT FROM OLD.deleted_at
            AND NEW.deleted_by              IS NOT DISTINCT FROM OLD.deleted_by
            AND NEW.tenant_id               IS NOT DISTINCT FROM OLD.tenant_id
        ) THEN
            -- Only row_version / updated_at / updated_by changed — allow
            RETURN NEW;
        END IF;

        RAISE EXCEPTION
            'ECL_CALC_RUN_SEALED: ecl.calc_run(id=%) was sealed at %. '
            'Modifications to business columns are forbidden on a sealed calc run. '
            'Only row_version, updated_at, updated_by may be updated for audit bookkeeping. '
            'Error: ECL_CALC_RUN_SEALED (HTTP 423).',
            OLD.id, OLD.sealed_at
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_ecl_calc_run_no_modify_when_sealed() IS
    'BEFORE UPDATE + BEFORE DELETE trigger guard for ecl.calc_run. '
    'DELETE: always blocked (DEC-018 no hard delete on ecl.*). '
    'UPDATE when sealed_at IS NOT NULL: allows only row_version + updated_at + updated_by changes; '
    'all other column mutations raise ECL_CALC_RUN_SEALED (HTTP 423). '
    'Created migration 000031 per P4-M8 spec. '
    'Replaces the stub fn_ecl_calc_run_no_modify_when_sealed defined in migration 000029 '
    '(which was installed on ecl.calc_header, not on this new ecl.calc_run table).';

CREATE TRIGGER trg_ecl_calc_run_sealed_guard
    BEFORE UPDATE OR DELETE ON ecl.calc_run
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_calc_run_no_modify_when_sealed();

-- Standard audit triggers
CREATE TRIGGER trg_ecl_calc_run_updated_at
    BEFORE UPDATE ON ecl.calc_run
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_ecl_calc_run_row_version
    BEFORE UPDATE ON ecl.calc_run
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- A-2. Indexes on ecl.calc_run
-- ====================================================================

-- (periode_id, status) — guards: "NO IN_PROGRESS for periode", "NO SEALED for periode"
-- Called at every POST /ecl/calc-runs and POST /seal/request.
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_periode_status
    ON ecl.calc_run (periode_id, status)
    WHERE deleted_at IS NULL;

-- (seal_requested_at) partial WHERE status='SEAL_REQUESTED' — approval queue listing
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_seal_requested
    ON ecl.calc_run (seal_requested_at)
    WHERE status = 'SEAL_REQUESTED' AND deleted_at IS NULL;

-- (cancelled_at) partial WHERE cancelled_at IS NOT NULL — cancelled run reporting
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_cancelled
    ON ecl.calc_run (cancelled_at)
    WHERE cancelled_at IS NOT NULL;

-- (asynq_job_id) — job-progress lookup via GET /api/v1/jobs/{jobId}
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_asynq_job_id
    ON ecl.calc_run (asynq_job_id)
    WHERE asynq_job_id IS NOT NULL;

-- FK indexes (db-conventions.md: every FK must be indexed)
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_seal_requested_by
    ON ecl.calc_run (seal_requested_by)
    WHERE seal_requested_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_sealed_by
    ON ecl.calc_run (sealed_by)
    WHERE sealed_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_cancelled_by
    ON ecl.calc_run (cancelled_by)
    WHERE cancelled_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_created_by
    ON ecl.calc_run (created_by);

CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_superseded_by
    ON ecl.calc_run (superseded_by_run_id)
    WHERE superseded_by_run_id IS NOT NULL;

-- (tenant_id, created_at DESC) — mandatory hot-table tenant query index
CREATE INDEX IF NOT EXISTS idx_ecl_calc_run_tenant_created
    ON ecl.calc_run (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;


-- ====================================================================
-- B. ecl.calc_result_line — resolve OQ-M7-2
--
-- Add FK constraint calc_run_id → ecl.calc_run(id).
-- In migration 000029, calc_run_id was intentionally left without a FK
-- because the target table did not yet exist (OQ-M7-2 deferred to M8).
-- Now that ecl.calc_run has UUID PK, the FK can be safely added.
--
-- Impact on existing rows: ecl.calc_result_line was created in 000029 on a
-- freshly created schema with no prod data; FK validation will succeed.
-- If dev rows exist with orphaned calc_run_id values, they must be cleaned
-- before this migration runs (runbook: docs/runbooks/mig-031-pre-check.md).
-- ====================================================================

ALTER TABLE ecl.calc_result_line
    ADD CONSTRAINT fk_ecl_calc_result_line_calc_run
        FOREIGN KEY (calc_run_id)
        REFERENCES ecl.calc_run(id)
        ON DELETE RESTRICT;

COMMENT ON COLUMN ecl.calc_result_line.calc_run_id IS
    'UUID FK → ecl.calc_run(id). FK constraint added in migration 000031 (OQ-M7-2 resolved). '
    'Previously had no DB-level FK (PK type mismatch vs sys.job — resolved by new ecl.calc_run table). '
    'Application must INSERT ecl.calc_run row before inserting ecl.calc_result_line rows.';

-- idx_ecl_calc_result_line_calc_run_id already exists from migration 000029 (A-3).
-- No duplicate index needed.

COMMIT;
