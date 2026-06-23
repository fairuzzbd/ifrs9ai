-- migration: 000048 Bulk Upload Master Instrumen (P5-M11)
-- author: data-modeler (driven by system-analyst P5-M11)
-- requires: 000047 (poci_delta_p5m10), 000001 (init_schema — sys.upload_batch, sys.upload_batch_row, mst.instrumen)
-- description:
--   P5-M11 — Bulk Upload Master Instrumen XLSX 5-sheet.
--   Extends sys.upload_batch + sys.upload_batch_row for INSTRUMEN_BULK batch type.
--   Adds bulk_upload_batch_id FK to mst.instrumen for rollback lookup.
--   Adds sys.config_param rows for grace window + file size + DRY_RUN TTL.
--   State machine: PARSED → DRY_RUN_PASSED/FAILED → COMMITTING → COMMITTED/PARTIAL_COMMIT
--                  → APPROVED → ROLLBACK_PENDING → ROLLED_BACK
--
-- References: P5-M11-S1..S5, docs/state-machines/p5-m11-bulk-upload.md, DEC-017/018/021/022/027.

BEGIN;

-- ─── 1. Extend sys.upload_batch ───────────────────────────────────────────────
-- The init_schema already has: status, rollback_status, batch_type, sheet_breakdown_json.
-- We ADD columns specific to INSTRUMEN_BULK workflow (dry_run cache, grace window, etc.)

-- 1A. Add dry_run_cached_at (when DRY_RUN last ran and result was cached)
ALTER TABLE sys.upload_batch
    ADD COLUMN IF NOT EXISTS dry_run_cached_at TIMESTAMPTZ;

-- 1B. Add dry_run_expires_at (TTL 1 hour from dry_run_cached_at)
ALTER TABLE sys.upload_batch
    ADD COLUMN IF NOT EXISTS dry_run_expires_at TIMESTAMPTZ;

-- 1C. Add dry_run_result_jsonb (cached DRY_RUN result: stage_summary, errors_per_row, etc.)
ALTER TABLE sys.upload_batch
    ADD COLUMN IF NOT EXISTS dry_run_result_jsonb JSONB;

-- 1D. Add rollback_grace_expires_at (committed_at + BULK_ROLLBACK_GRACE_DAYS)
ALTER TABLE sys.upload_batch
    ADD COLUMN IF NOT EXISTS rollback_grace_expires_at TIMESTAMPTZ;

-- 1E. Ensure rollback_status has NOT_REQUESTED as default (init_schema has rollback_status nullable)
-- We use NULL = NOT_REQUESTED for existing rows; new INSTRUMEN_BULK rows get explicit NOT_REQUESTED
-- No schema change needed — semantic mapping in application layer.

-- 1F. Add batch_mode expansion for INSTRUMEN_BULK (PARSE_ONLY, DRY_RUN, COMMIT modes are logical
--     not a batch_mode column — the status column tracks this already. Skip batch_mode column.)

-- 1G. Verify INSTRUMEN_BULK is covered by ck_batch_type constraint (from init_schema: yes).
--     Verify COMMITTING + PARTIAL_COMMIT are covered by ck_batch_status.
DO $$
DECLARE
    v_committing_ok      BOOLEAN;
    v_partial_commit_ok  BOOLEAN;
    v_rollback_pending_ok BOOLEAN;
    v_instrumen_bulk_ok  BOOLEAN;
    v_constraint_def     TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
    FROM pg_constraint c
    JOIN pg_class cl ON cl.oid = c.conrelid
    JOIN pg_namespace ns ON ns.oid = cl.relnamespace
    WHERE ns.nspname = 'sys' AND cl.relname = 'upload_batch' AND c.conname = 'ck_batch_status'
    INTO v_constraint_def;

    IF v_constraint_def IS NULL THEN
        RAISE NOTICE 'ck_batch_status not found — adding full constraint for P5-M11';
        ALTER TABLE sys.upload_batch
            ADD CONSTRAINT ck_batch_status
            CHECK (status IN (
                'PARSING','STAGED','PENDING_REVIEW','PENDING_APPROVAL',
                'APPROVED','REJECTED','COMMITTING','COMMITTED','FAILED','ROLLED_BACK',
                'PARSED','DRY_RUN_PASSED','DRY_RUN_FAILED',
                'PARTIAL_COMMIT','ROLLBACK_PENDING'
            ));
    ELSE
        v_committing_ok      := (v_constraint_def ILIKE '%COMMITTING%');
        v_partial_commit_ok  := (v_constraint_def ILIKE '%PARTIAL_COMMIT%');
        v_rollback_pending_ok := (v_constraint_def ILIKE '%ROLLBACK_PENDING%');
        v_instrumen_bulk_ok  := TRUE; -- init_schema has INSTRUMEN_BULK in ck_batch_type

        IF NOT (v_committing_ok AND v_partial_commit_ok AND v_rollback_pending_ok) THEN
            RAISE NOTICE 'ck_batch_status missing P5-M11 statuses — extending constraint';
            ALTER TABLE sys.upload_batch DROP CONSTRAINT ck_batch_status;
            ALTER TABLE sys.upload_batch
                ADD CONSTRAINT ck_batch_status
                CHECK (status IN (
                    'PARSING','STAGED','PENDING_REVIEW','PENDING_APPROVAL',
                    'APPROVED','REJECTED','COMMITTING','COMMITTED','FAILED','ROLLED_BACK',
                    'PARSED','DRY_RUN_PASSED','DRY_RUN_FAILED',
                    'PARTIAL_COMMIT','ROLLBACK_PENDING'
                ));
        ELSE
            RAISE NOTICE 'ck_batch_status already covers P5-M11 statuses — no change needed';
        END IF;
    END IF;
END;
$$;

-- ─── 2. Extend sys.upload_batch_row ──────────────────────────────────────────
-- init_schema has: status_validation, status_commit, validation_errors_json.
-- We ADD instrumen-bulk-specific columns.

-- 2A. Add row_status: unified row lifecycle status for INSTRUMEN_BULK
--     (replaces status_commit semantic for bulk upload rows)
ALTER TABLE sys.upload_batch_row
    ADD COLUMN IF NOT EXISTS row_status TEXT DEFAULT 'PENDING';

-- Apply check constraint if column is new
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace ns ON ns.oid = cl.relnamespace
        WHERE ns.nspname = 'sys' AND cl.relname = 'upload_batch_row'
          AND c.conname = 'ck_bulk_row_status'
    ) THEN
        ALTER TABLE sys.upload_batch_row
            ADD CONSTRAINT ck_bulk_row_status
            CHECK (row_status IN ('PENDING','COMMITTED','FAILED','ROLLED_BACK','FLAGGED_MANUAL_REVIEW'));
    END IF;
END;
$$;

-- 2B. Add instrumen_id: FK to mst.instrumen created at commit time
--     NULL until worker commits; populated after successful INSERT
ALTER TABLE sys.upload_batch_row
    ADD COLUMN IF NOT EXISTS bulk_instrumen_id UUID;

-- Note: FK to mst.instrumen added after mst.instrumen column addition (below)

-- 2C. Add row_error_jsonb: detailed error info per row (parse, validation, commit errors)
ALTER TABLE sys.upload_batch_row
    ADD COLUMN IF NOT EXISTS row_error_jsonb JSONB;

-- ─── 3. Extend mst.instrumen ─────────────────────────────────────────────────
-- Add bulk_upload_batch_id for rollback lookup (soft-delete all instruments from a batch)

-- 3A. Add bulk_upload_batch_id FK column
ALTER TABLE mst.instrumen
    ADD COLUMN IF NOT EXISTS bulk_upload_batch_id UUID;

-- 3B. Add status values for bulk workflow
--     mst.instrumen.status is VARCHAR(30) with no existing CHECK in init_schema
--     (init_schema shows ck_instrumen has no status check — just ck_kupon_nonneg etc.)
--     We do NOT add a CHECK to avoid breaking existing AKTIF + other values.
--     Application layer enforces PENDING_APPROVAL_BULK and PENDING_CLASSIFICATION.
--     Document valid values here:
COMMENT ON COLUMN mst.instrumen.bulk_upload_batch_id IS
    'FK sys.upload_batch.id. Populated for instruments inserted via bulk upload (P5-M11). '
    'Used for rollback: soft-delete all instruments WHERE bulk_upload_batch_id = batch_id. '
    'NULL for instruments created via single-entry workflow (P5-M1).';

-- 3C. Add FK constraint for bulk_upload_batch_id
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace ns ON ns.oid = cl.relnamespace
        WHERE ns.nspname = 'mst' AND cl.relname = 'instrumen'
          AND c.conname = 'fk_instrumen_bulk_batch'
    ) THEN
        ALTER TABLE mst.instrumen
            ADD CONSTRAINT fk_instrumen_bulk_batch
            FOREIGN KEY (bulk_upload_batch_id)
            REFERENCES sys.upload_batch(id)
            ON DELETE RESTRICT;  -- soft-delete only (DEC-018)
    END IF;
END;
$$;

-- 3D. Index for rollback and audit queries
CREATE INDEX IF NOT EXISTS idx_instrumen_bulk_batch
    ON mst.instrumen (bulk_upload_batch_id)
    WHERE bulk_upload_batch_id IS NOT NULL;

-- 3E. Partial index for PENDING_APPROVAL_BULK (approval queue)
CREATE INDEX IF NOT EXISTS idx_instrumen_pending_approval_bulk
    ON mst.instrumen (bulk_upload_batch_id, status)
    WHERE status = 'PENDING_APPROVAL_BULK' AND is_deleted = FALSE;

-- ─── 4. Add FK from upload_batch_row.bulk_instrumen_id to mst.instrumen ──────
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class cl ON cl.oid = c.conrelid
        JOIN pg_namespace ns ON ns.oid = cl.relnamespace
        WHERE ns.nspname = 'sys' AND cl.relname = 'upload_batch_row'
          AND c.conname = 'fk_batch_row_bulk_instrumen'
    ) THEN
        ALTER TABLE sys.upload_batch_row
            ADD CONSTRAINT fk_batch_row_bulk_instrumen
            FOREIGN KEY (bulk_instrumen_id)
            REFERENCES mst.instrumen(id)
            ON DELETE RESTRICT;
    END IF;
END;
$$;

-- Index for bulk_instrumen_id lookups
CREATE INDEX IF NOT EXISTS idx_upload_batch_row_bulk_instrumen
    ON sys.upload_batch_row (bulk_instrumen_id)
    WHERE bulk_instrumen_id IS NOT NULL;

-- Composite index for batch_id + row_status queries (approval queue, rollback)
CREATE INDEX IF NOT EXISTS idx_upload_batch_row_status_batch
    ON sys.upload_batch_row (batch_id, row_status)
    WHERE row_status IN ('PENDING','COMMITTED','FAILED','ROLLED_BACK','FLAGGED_MANUAL_REVIEW');

-- ─── 5. sys.upload_batch indexes for P5-M11 queries ─────────────────────────
-- Index for dry_run_expires_at (TTL check at commit time)
CREATE INDEX IF NOT EXISTS idx_upload_batch_dry_run_expires
    ON sys.upload_batch (dry_run_expires_at)
    WHERE dry_run_expires_at IS NOT NULL AND status = 'DRY_RUN_PASSED';

-- Index for rollback_grace_expires_at (grace window check)
CREATE INDEX IF NOT EXISTS idx_upload_batch_rollback_grace
    ON sys.upload_batch (rollback_grace_expires_at)
    WHERE rollback_grace_expires_at IS NOT NULL AND status = 'APPROVED';

-- ─── 6. sys.config_param seed for P5-M11 ────────────────────────────────────
-- Assumes sys.config_param table exists from init or earlier migrations.
-- INSERT ON CONFLICT DO NOTHING for idempotency.

DO $$
BEGIN
    -- Check if sys.config_param table exists
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'sys' AND table_name = 'config_param'
    ) THEN
        INSERT INTO sys.config_param (param_key, param_value, description, created_by, tenant_id)
        VALUES
            (
                'BULK_ROLLBACK_GRACE_DAYS',
                '7',
                'Grace window (days) for CFO rollback after APPROVED. '
                'Configurable by ROLE-IT-ADMIN. Non-retroactive for existing batches. '
                'P5-M11-S5-AC4.',
                '00000000-0000-0000-0000-000000000001',
                'TUGURE'
            ),
            (
                'BULK_FILE_MAX_MB',
                '50',
                'Maximum XLSX upload file size in megabytes for bulk instrumen upload. '
                'Server-side enforce. P5-M11-S1-AC2.',
                '00000000-0000-0000-0000-000000000001',
                'TUGURE'
            ),
            (
                'BULK_DRY_RUN_TTL_SECONDS',
                '3600',
                'DRY_RUN cache TTL in seconds (default 1 hour). '
                'COMMIT blocked if dry_run_expires_at < now(). P5-M11-S2-AC4.',
                '00000000-0000-0000-0000-000000000001',
                'TUGURE'
            )
        ON CONFLICT (param_key) DO NOTHING;

        RAISE NOTICE 'sys.config_param: P5-M11 params seeded (BULK_ROLLBACK_GRACE_DAYS, BULK_FILE_MAX_MB, BULK_DRY_RUN_TTL_SECONDS)';
    ELSE
        -- Fallback: use sys.parameter if config_param does not exist
        RAISE NOTICE 'sys.config_param table not found — skipping seed. '
            'Seed manually: BULK_ROLLBACK_GRACE_DAYS=7, BULK_FILE_MAX_MB=50, BULK_DRY_RUN_TTL_SECONDS=3600';
    END IF;
END;
$$;

-- ─── 7. Comments ─────────────────────────────────────────────────────────────

COMMENT ON COLUMN sys.upload_batch.dry_run_cached_at IS
    'Timestamp when DRY_RUN pipeline last ran. NULL until first DRY_RUN. P5-M11.';

COMMENT ON COLUMN sys.upload_batch.dry_run_expires_at IS
    'DRY_RUN cache expiry = dry_run_cached_at + BULK_DRY_RUN_TTL_SECONDS. '
    'COMMIT blocked if now() > dry_run_expires_at (BULK_DRY_RUN_EXPIRED). P5-M11-S2-AC4.';

COMMENT ON COLUMN sys.upload_batch.dry_run_result_jsonb IS
    'Cached 4-stage DRY_RUN result: {stage_summary, errors_per_row[], '
    'valid_rows, invalid_rows, flagged_rows}. TTL = dry_run_expires_at. P5-M11.';

COMMENT ON COLUMN sys.upload_batch.rollback_grace_expires_at IS
    'Grace window expiry = committed_at + BULK_ROLLBACK_GRACE_DAYS days. '
    'Populated when batch moves to COMMITTED. CFO rollback blocked after this. P5-M11-S5.';

COMMENT ON COLUMN sys.upload_batch_row.row_status IS
    'INSTRUMEN_BULK row lifecycle: PENDING → COMMITTED | FAILED | ROLLED_BACK. '
    'FLAGGED_MANUAL_REVIEW: SPPI+BM ambiguous from Stage 4 DRY_RUN (Phase 3). P5-M11.';

COMMENT ON COLUMN sys.upload_batch_row.bulk_instrumen_id IS
    'UUID of mst.instrumen inserted during commit worker. NULL until committed. '
    'Used to update instrumen.status on approve + soft-delete on rollback. P5-M11.';

COMMENT ON COLUMN sys.upload_batch_row.row_error_jsonb IS
    'Error detail per row. Populated on parse error (PARSE stage), '
    'validation failure (DRY_RUN stages 1-4), or commit INSERT error. '
    'Structure: {stage, col, error, sheet, row_number}. P5-M11.';

COMMIT;
