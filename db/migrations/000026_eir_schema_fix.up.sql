-- migration: 0026 eir_schema_fix
-- author: data-modeler
-- requires: 0001 (init_schema — ecl.eir_amortization_schedule, ecl.eir_reestimation_log,
--                               trx.amortisasi, mst.instrumen created),
--           0005 (fn_ecl_no_hard_delete — no-hard-delete trigger on ecl.*),
--           0024 (fund_composition_lookthrough),
--           0025 (lookthrough_underlying_audit_not_null)
-- description:
--   P4-M5 EIR schema fixes (DEC-016 precision + audit cols + immutability trigger):
--   (A) ecl.eir_amortization_schedule
--       — precision: NUMERIC(12,8) → NUMERIC(10,8) for eir_periode
--       — precision: NUMERIC(20,2) → NUMERIC(20,4) for 5 IDR amount cols
--       — add audit cols: created_by, updated_at, updated_by, deleted_at, deleted_by,
--                         row_version, tenant_id
--       — add flag_poci BOOLEAN NOT NULL DEFAULT false (POCI forward-compat, M5 stub)
--       — add trigger fn_eir_schedule_amounts_immutable / tg_eir_schedule_amounts_immutable
--         (BEFORE UPDATE — reject changes to financial amount cols; only recomputed_from_seq
--         and audit cols allowed to change)
--       — add triggers trg_eir_schedule_updated_at + trg_eir_schedule_row_version
--       — add indexes: idx_eir_schedule_instrumen_tanggal, idx_eir_schedule_deleted
--   (B) ecl.eir_reestimation_log
--       — precision: NUMERIC(12,8) → NUMERIC(10,8) for eir_sebelum, eir_sesudah
--       — precision: NUMERIC(20,2) → NUMERIC(20,4) for carrying_sebelum, carrying_sesudah,
--                    catch_up_adjustment
--       — add audit cols: created_by, updated_at, updated_by, deleted_at, deleted_by,
--                         row_version, tenant_id
--       — add workflow cols: rejected_at, reject_reason, reviewer_comment, approver_comment,
--                            reviewer_signature_hash, approver_signature_hash
--       — add CHECK constraints: chk_eir_log_workflow_status, chk_eir_log_sod
--       — add triggers trg_eir_log_updated_at + trg_eir_log_row_version
--   (C) trx.amortisasi
--       — precision: NUMERIC(20,2) → NUMERIC(20,4) for amortisasi_premium_diskonto_idr
--       — add audit cols: created_by, updated_at, updated_by, deleted_at, deleted_by,
--                         row_version, tenant_id
--       — add triggers trg_amortisasi_updated_at + trg_amortisasi_row_version
--   (D) mst.instrumen
--       — precision: NUMERIC(12,8) → NUMERIC(10,8) for eir_awal (DEC-016)
--       — add flag_poci BOOLEAN NOT NULL DEFAULT false

BEGIN;

-- ============================================================
-- NOTE on sentinel UUID for backfill:
-- Rows inserted before this migration have no actor UUID for
-- created_by / updated_by (those cols didn't exist).
-- Sentinel '00000000-0000-0000-0000-000000000000' is used per
-- the pattern established in migration 000025 (lookthrough_audit).
-- Application code must supply real UUIDs on all future writes.
-- ============================================================

-- ============================================================
-- A. ecl.eir_amortization_schedule
-- ============================================================

-- A-1. Precision fixes
ALTER TABLE ecl.eir_amortization_schedule
    ALTER COLUMN eir_periode              TYPE NUMERIC(10,8),
    ALTER COLUMN opening_carrying         TYPE NUMERIC(20,4),
    ALTER COLUMN cash_inflow              TYPE NUMERIC(20,4),
    ALTER COLUMN pendapatan_bunga_eir     TYPE NUMERIC(20,4),
    ALTER COLUMN amortisasi_p_d           TYPE NUMERIC(20,4),
    ALTER COLUMN pelunasan_pokok          TYPE NUMERIC(20,4),
    ALTER COLUMN closing_carrying         TYPE NUMERIC(20,4);

-- A-2. Audit columns
ALTER TABLE ecl.eir_amortization_schedule
    ADD COLUMN IF NOT EXISTS created_by  UUID,
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by  UUID,
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID,
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- A-3. Backfill created_by / updated_by with sentinel
UPDATE ecl.eir_amortization_schedule
    SET created_by = '00000000-0000-0000-0000-000000000000'::UUID
WHERE created_by IS NULL;

UPDATE ecl.eir_amortization_schedule
    SET updated_by = '00000000-0000-0000-0000-000000000000'::UUID
WHERE updated_by IS NULL;

-- A-4. Enforce NOT NULL on created_by / updated_by after backfill
ALTER TABLE ecl.eir_amortization_schedule
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN updated_by SET NOT NULL;

-- A-5. flag_poci — POCI forward-compat stub (full logic in M7)
ALTER TABLE ecl.eir_amortization_schedule
    ADD COLUMN IF NOT EXISTS flag_poci BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN ecl.eir_amortization_schedule.flag_poci IS
    'TRUE when the parent instrumen is POCI (Purchased or Originated Credit Impaired). '
    'Stub column added in M5; full POCI schedule logic implemented in M7. '
    'EIR for POCI schedules is credit-adjusted EIR per IFRS 9 §5.4.1.';

-- A-6. Immutability trigger — reject UPDATE on financial amount columns.
--      Only recomputed_from_seq (supersede marker) and audit cols are allowed to change.
CREATE OR REPLACE FUNCTION fn_eir_schedule_amounts_immutable()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.opening_carrying         IS DISTINCT FROM OLD.opening_carrying)         OR
       (NEW.cash_inflow               IS DISTINCT FROM OLD.cash_inflow)               OR
       (NEW.pendapatan_bunga_eir      IS DISTINCT FROM OLD.pendapatan_bunga_eir)      OR
       (NEW.amortisasi_p_d            IS DISTINCT FROM OLD.amortisasi_p_d)            OR
       (NEW.pelunasan_pokok           IS DISTINCT FROM OLD.pelunasan_pokok)           OR
       (NEW.closing_carrying          IS DISTINCT FROM OLD.closing_carrying)          OR
       (NEW.eir_periode               IS DISTINCT FROM OLD.eir_periode)               OR
       (NEW.tanggal_posting           IS DISTINCT FROM OLD.tanggal_posting)           OR
       (NEW.periode_seq               IS DISTINCT FROM OLD.periode_seq)               OR
       (NEW.instrumen_id              IS DISTINCT FROM OLD.instrumen_id)
    THEN
        RAISE EXCEPTION
            'ecl.eir_amortization_schedule row % is immutable: financial amounts and '
            'tanggal_posting/periode_seq/instrumen_id cannot be changed after insert. '
            'Amendment must INSERT new rows with recomputed_from_seq set to mark superseded rows. '
            'Only recomputed_from_seq, status_posting, jurnal_reference_id, and audit cols '
            '(updated_at, updated_by, deleted_at, deleted_by, row_version) may be updated.',
            OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_eir_schedule_amounts_immutable
    BEFORE UPDATE ON ecl.eir_amortization_schedule
    FOR EACH ROW EXECUTE FUNCTION fn_eir_schedule_amounts_immutable();

COMMENT ON FUNCTION fn_eir_schedule_amounts_immutable() IS
    'Immutability guard for ecl.eir_amortization_schedule. '
    'Rejects any UPDATE that touches financial columns (amounts, EIR, date, sequence). '
    'Amendment flow must INSERT new rows; superseded rows are marked via recomputed_from_seq. '
    'Required per p4-m5-eir.md §3 amendment execution rule and DEC-018 audit-grade immutability.';

-- A-7. Updated_at and row_version triggers
CREATE TRIGGER trg_eir_schedule_updated_at
    BEFORE UPDATE ON ecl.eir_amortization_schedule
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_eir_schedule_row_version
    BEFORE UPDATE ON ecl.eir_amortization_schedule
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- A-8. Indexes
-- (ix_schedule_instrumen already exists from 000001 on (instrumen_id, periode_seq))
-- Add descending-date index for schedule lookup / schedule history queries
CREATE INDEX IF NOT EXISTS idx_eir_schedule_instrumen_tanggal
    ON ecl.eir_amortization_schedule(instrumen_id, tanggal_posting DESC);

-- Partial index for active-only queries (recomputed_from_seq IS NULL = aktif)
CREATE INDEX IF NOT EXISTS idx_eir_schedule_active_instrumen
    ON ecl.eir_amortization_schedule(instrumen_id, periode_seq)
    WHERE recomputed_from_seq IS NULL AND deleted_at IS NULL;

-- Tenant + created_at for hot queries
CREATE INDEX IF NOT EXISTS idx_eir_schedule_tenant_created
    ON ecl.eir_amortization_schedule(tenant_id, created_at DESC);


-- ============================================================
-- B. ecl.eir_reestimation_log
-- ============================================================

-- B-1. Precision fixes
ALTER TABLE ecl.eir_reestimation_log
    ALTER COLUMN eir_sebelum         TYPE NUMERIC(10,8),
    ALTER COLUMN eir_sesudah         TYPE NUMERIC(10,8),
    ALTER COLUMN carrying_sebelum    TYPE NUMERIC(20,4),
    ALTER COLUMN carrying_sesudah    TYPE NUMERIC(20,4),
    ALTER COLUMN catch_up_adjustment TYPE NUMERIC(20,4);

-- B-2. Audit columns
ALTER TABLE ecl.eir_reestimation_log
    ADD COLUMN IF NOT EXISTS created_by  UUID,
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by  UUID,
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID,
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- B-3. Backfill created_by with sentinel (maker_id exists — use it; fallback to sentinel)
UPDATE ecl.eir_reestimation_log
    SET created_by = maker_id
WHERE created_by IS NULL;

UPDATE ecl.eir_reestimation_log
    SET updated_by = maker_id
WHERE updated_by IS NULL;

-- B-4. Enforce NOT NULL
ALTER TABLE ecl.eir_reestimation_log
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN updated_by SET NOT NULL;

-- B-5. Workflow enhancement columns
ALTER TABLE ecl.eir_reestimation_log
    ADD COLUMN IF NOT EXISTS rejected_at              TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reject_reason            TEXT,
    ADD COLUMN IF NOT EXISTS reviewer_comment         TEXT,
    ADD COLUMN IF NOT EXISTS approver_comment         TEXT,
    ADD COLUMN IF NOT EXISTS reviewer_signature_hash  TEXT,
    ADD COLUMN IF NOT EXISTS approver_signature_hash  TEXT;

COMMENT ON COLUMN ecl.eir_reestimation_log.reviewer_signature_hash IS
    'SHA-256(reviewer_id || action || proposal_id || reviewed_at || reviewer_comment). '
    'Computed and persisted by backend at review sign-off step.';

COMMENT ON COLUMN ecl.eir_reestimation_log.approver_signature_hash IS
    'SHA-256(approver_id || action || proposal_id || approved_at || approver_comment). '
    'Computed and persisted by backend at ALCO approve step (step-up MFA required, DEC-027).';

-- B-6. CHECK constraints
ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_workflow_status;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_workflow_status
        CHECK (workflow_status IN (
            'DRAFT', 'PENDING_REVIEW', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED'
        ));

ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod;

ALTER TABLE ecl.eir_reestimation_log
    ADD CONSTRAINT chk_eir_log_sod
        CHECK (reviewer_id IS NULL OR reviewer_id <> maker_id);

-- B-7. Updated_at and row_version triggers
CREATE TRIGGER trg_eir_log_updated_at
    BEFORE UPDATE ON ecl.eir_reestimation_log
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_eir_log_row_version
    BEFORE UPDATE ON ecl.eir_reestimation_log
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- B-8. Additional index for workflow queue (pending states)
CREATE INDEX IF NOT EXISTS idx_eir_log_workflow_status_partial
    ON ecl.eir_reestimation_log(workflow_status, created_at DESC)
    WHERE workflow_status IN ('DRAFT','PENDING_REVIEW','PENDING_APPROVAL')
      AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_eir_log_tenant_created
    ON ecl.eir_reestimation_log(tenant_id, created_at DESC);


-- ============================================================
-- C. trx.amortisasi
-- ============================================================
-- NOTE: trx.amortisasi exists since 000001 (verified grep).
-- No conditional skip needed.

-- C-1. Precision fix
ALTER TABLE trx.amortisasi
    ALTER COLUMN amortisasi_premium_diskonto_idr TYPE NUMERIC(20,4);

-- C-2. Audit columns
ALTER TABLE trx.amortisasi
    ADD COLUMN IF NOT EXISTS created_by  UUID,
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by  UUID,
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID,
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- C-3. Backfill with sentinel
UPDATE trx.amortisasi
    SET created_by = '00000000-0000-0000-0000-000000000000'::UUID
WHERE created_by IS NULL;

UPDATE trx.amortisasi
    SET updated_by = '00000000-0000-0000-0000-000000000000'::UUID
WHERE updated_by IS NULL;

-- C-4. Enforce NOT NULL
ALTER TABLE trx.amortisasi
    ALTER COLUMN created_by SET NOT NULL,
    ALTER COLUMN updated_by SET NOT NULL;

-- C-5. Triggers
CREATE TRIGGER trg_amortisasi_updated_at
    BEFORE UPDATE ON trx.amortisasi
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_amortisasi_row_version
    BEFORE UPDATE ON trx.amortisasi
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- C-6. Additional indexes
CREATE INDEX IF NOT EXISTS idx_amortisasi_tenant_created
    ON trx.amortisasi(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_amortisasi_deleted
    ON trx.amortisasi(instrumen_id, tanggal_posting)
    WHERE deleted_at IS NULL;


-- ============================================================
-- D. mst.instrumen — EIR precision + flag_poci
-- ============================================================

-- D-1. Precision fix: eir_awal NUMERIC(12,8) → NUMERIC(10,8) per DEC-016
--      CHECK constraint ck_eir_range references eir_awal but uses IS NULL / range —
--      must drop and re-add after type change.
ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_eir_range;

ALTER TABLE mst.instrumen
    ALTER COLUMN eir_awal TYPE NUMERIC(10,8);

ALTER TABLE mst.instrumen
    ADD CONSTRAINT ck_eir_range
        CHECK (eir_awal IS NULL OR (eir_awal >= 0 AND eir_awal < 1));

-- D-2. flag_poci (POCI forward-compat stub — full logic in M7)
ALTER TABLE mst.instrumen
    ADD COLUMN IF NOT EXISTS flag_poci BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN mst.instrumen.flag_poci IS
    'TRUE when this instrumen was Purchased or Originated Credit Impaired (POCI) at inception. '
    'Requires credit-adjusted EIR (IFRS 9 §5.4.1(c)). '
    'Stub added in M5; full POCI credit-adjusted EIR logic implemented in M7. '
    'Corresponds to flag_poci on ecl.eir_amortization_schedule rows for this instrument.';

COMMIT;
