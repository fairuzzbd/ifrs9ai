-- migration: 000023 lps_exclusion_override
-- author: data-modeler
-- requires: 0001 (init_schema — mst.instrumen, mst.periode_buku, ecl.calc_header, mst.lps_coverage),
--           0004 (fn_increment_row_version, fn_update_updated_at),
--           0005 (fn_ecl_no_hard_delete),
--           0007 (sec.user FK target, sys.workflow pattern),
--           0009 (mst.periode_buku final shape — tanggal_mulai, tanggal_akhir),
--           0012 (mst.lps_coverage),
--           0022 (staging engine — confirms ecl schema pattern)
-- description: P4-M3 LPS Aggregator — manual exclusion override workflow table.
--   Creates ecl.lps_exclusion_override (4-eyes: ROLE-RISK maker → ROLE-ALCO approver).
--   State machine: PENDING_APPROVAL → APPROVED_ACTIVE | REJECTED → terminal.
--                  APPROVED_ACTIVE → EXPIRED (system/batch auto-expiry).
--   SoD: maker_id <> approver_id (DB CHECK + application layer).
--   Periode validity: valid_from_periode.tanggal_mulai <= valid_to_periode.tanggal_akhir
--     enforced via TRIGGER (PG does not allow subqueries inside CHECK constraints).
--
-- DEFERRED (M7): Three LPS columns on ecl.calc_header are intentionally excluded from
--   this migration — lps_covered_idr, lps_excess_idr, lps_covered_flag.
--   These will be added in migration 000024_calc_header_lps_cols.up.sql after
--   ecl-eir-engineer confirms calc_header schema alignment (M7).
--   Also deferred: ALTER ead_idr precision on ecl.calc_header from NUMERIC(20,2) to
--   NUMERIC(20,4) — pending impact assessment on ecl.calc_detail_skenario derived cols.

BEGIN;

-- ============================================================
-- TABLE: ecl.lps_exclusion_override
-- Manual exclusion proposal: remove an instrument from the
-- LPS coverage pool for a specified periode range.
-- Lifecycle: PENDING_APPROVAL → APPROVED_ACTIVE (if approve)
--                             → REJECTED (if reject/recall)
--            APPROVED_ACTIVE  → EXPIRED (system batch auto-expiry)
-- No hard-delete (ecl schema rule). Soft-delete via deleted_at.
-- ============================================================

CREATE TABLE ecl.lps_exclusion_override (
    -- Identity
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Instrument being excluded from LPS coverage pool
    instrumen_id            UUID        NOT NULL
                                REFERENCES mst.instrumen(id)
                                ON DELETE RESTRICT,

    -- Business case — minimum 30 chars enforced by CHECK
    exclusion_reason        TEXT        NOT NULL
                                CONSTRAINT chk_lps_override_reason_minlen
                                CHECK (length(exclusion_reason) >= 30),

    -- Periode validity range (FK to mst.periode_buku)
    -- Cross-periode ordering enforced by trigger trg_lps_override_periode_order (below).
    valid_from_periode_id   UUID        NOT NULL
                                REFERENCES mst.periode_buku(id)
                                ON DELETE RESTRICT,
    valid_to_periode_id     UUID        NOT NULL
                                REFERENCES mst.periode_buku(id)
                                ON DELETE RESTRICT,

    -- Workflow state
    -- PENDING_APPROVAL : submitted by ROLE-RISK, awaiting ROLE-ALCO
    -- APPROVED_ACTIVE  : ROLE-ALCO approved; exclusion takes effect in ECL calc
    -- REJECTED         : rejected by ROLE-ALCO or recalled by maker; terminal state
    -- EXPIRED          : APPROVED_ACTIVE whose valid_to_periode has passed; terminal state
    workflow_status         TEXT        NOT NULL DEFAULT 'PENDING_APPROVAL'
                                CONSTRAINT chk_lps_override_workflow_status
                                CHECK (workflow_status IN (
                                    'PENDING_APPROVAL',
                                    'APPROVED_ACTIVE',
                                    'REJECTED',
                                    'EXPIRED'
                                )),

    -- 4-eyes actors
    maker_id                UUID        NOT NULL
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,
    approver_id             UUID
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,

    -- Approval signature (populated atomically when status → APPROVED_ACTIVE)
    signed_at_approve       TIMESTAMPTZ,
    -- SHA-256(approver_id || APPROVE || id || signed_at_approve || comment_approve)
    -- Stored as BYTEA (32 bytes). Immutable after set.
    signature_hash_approve  BYTEA,
    comment_approve         TEXT,

    -- Rejection (populated when status → REJECTED)
    reject_reason           TEXT,

    -- ----------------------------------------------------------------
    -- Constraints
    -- ----------------------------------------------------------------

    -- SoD: maker cannot be approver (4-eyes)
    CONSTRAINT chk_lps_override_sod
        CHECK (maker_id <> approver_id OR approver_id IS NULL),

    -- Approval fields must be consistent with workflow_status
    CONSTRAINT chk_lps_override_approve_fields_when_approved
        CHECK (
            workflow_status <> 'APPROVED_ACTIVE'
            OR (signed_at_approve IS NOT NULL AND signature_hash_approve IS NOT NULL AND approver_id IS NOT NULL)
        ),

    -- Standard audit columns (db-conventions.md)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE ecl.lps_exclusion_override IS
    'Manual LPS coverage exclusion proposals (P4-M3 LPS Aggregator). '
    '4-eyes workflow: ROLE-RISK maker → ROLE-ALCO approver. '
    'State machine per docs/state-machines/p4-m3-lps.md §2. '
    'Soft-delete allowed (deleted_at); hard-delete refused by trigger '
    'tg_ecl_lps_exclusion_override_no_delete (fn_ecl_no_hard_delete). '
    'SoD enforced via chk_lps_override_sod CHECK + application layer. '
    'Periode ordering (valid_from.tanggal_mulai <= valid_to.tanggal_akhir) '
    'enforced by trigger trg_lps_override_periode_order. ';

COMMENT ON COLUMN ecl.lps_exclusion_override.exclusion_reason IS
    'Business justification for excluding this instrument from the LPS coverage pool. '
    'Minimum 30 characters (chk_lps_override_reason_minlen). Maximum 2000 characters '
    'enforced at application layer.';

COMMENT ON COLUMN ecl.lps_exclusion_override.valid_from_periode_id IS
    'First periode for which this exclusion is effective. '
    'FK to mst.periode_buku. Trigger ensures valid_from.tanggal_mulai '
    '<= valid_to.tanggal_akhir.';

COMMENT ON COLUMN ecl.lps_exclusion_override.valid_to_periode_id IS
    'Last periode for which this exclusion is effective (inclusive). '
    'FK to mst.periode_buku. May equal valid_from_periode_id for a single-periode exclusion.';

COMMENT ON COLUMN ecl.lps_exclusion_override.signature_hash_approve IS
    'SHA-256(approver_id || ''APPROVE'' || id || signed_at_approve || comment_approve). '
    'Stored as BYTEA (32 bytes). Computed by application layer. Immutable after set.';

COMMENT ON COLUMN ecl.lps_exclusion_override.workflow_status IS
    'PENDING_APPROVAL: submitted, awaiting ROLE-ALCO approval. '
    'APPROVED_ACTIVE: exclusion is active; instrument excluded from LPS pool in ECL calc. '
    'REJECTED: rejected by ROLE-ALCO or recalled by maker (terminal). '
    'EXPIRED: APPROVED_ACTIVE override whose valid_to_periode has passed (terminal, set by batch job).';

-- ============================================================
-- TRIGGER: periode ordering validation
-- PG does not allow subqueries in CHECK constraints (SQL standard
-- restriction). Enforce valid_from.tanggal_mulai <= valid_to.tanggal_akhir
-- via BEFORE INSERT OR UPDATE trigger.
-- ============================================================

CREATE OR REPLACE FUNCTION fn_lps_override_check_periode_order()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_from_start  DATE;
    v_to_end      DATE;
BEGIN
    SELECT tanggal_mulai INTO v_from_start
    FROM mst.periode_buku
    WHERE id = NEW.valid_from_periode_id;

    SELECT tanggal_akhir INTO v_to_end
    FROM mst.periode_buku
    WHERE id = NEW.valid_to_periode_id;

    IF v_from_start IS NULL THEN
        RAISE EXCEPTION 'valid_from_periode_id % not found in mst.periode_buku', NEW.valid_from_periode_id;
    END IF;

    IF v_to_end IS NULL THEN
        RAISE EXCEPTION 'valid_to_periode_id % not found in mst.periode_buku', NEW.valid_to_periode_id;
    END IF;

    IF v_from_start > v_to_end THEN
        RAISE EXCEPTION
            'LPS_OVERRIDE_PERIODE_INVALID: valid_from_periode tanggal_mulai (%) '
            'must be <= valid_to_periode tanggal_akhir (%)',
            v_from_start, v_to_end
            USING ERRCODE = 'P0001';
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_lps_override_check_periode_order() IS
    'Validates that valid_from_periode.tanggal_mulai <= valid_to_periode.tanggal_akhir. '
    'Called by trg_lps_override_periode_order on ecl.lps_exclusion_override. '
    'Error code: LPS_OVERRIDE_PERIODE_INVALID (P0001).';

CREATE TRIGGER trg_lps_override_periode_order
    BEFORE INSERT OR UPDATE OF valid_from_periode_id, valid_to_periode_id
    ON ecl.lps_exclusion_override
    FOR EACH ROW EXECUTE FUNCTION fn_lps_override_check_periode_order();

-- ============================================================
-- STANDARD TRIGGERS: updated_at + row_version
-- ============================================================

CREATE TRIGGER trg_lps_override_updated_at
    BEFORE UPDATE ON ecl.lps_exclusion_override
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_lps_override_row_version
    BEFORE UPDATE ON ecl.lps_exclusion_override
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ============================================================
-- NO HARD DELETE (ecl schema rule — db-conventions.md)
-- ============================================================

CREATE TRIGGER tg_ecl_lps_override_no_delete
    BEFORE DELETE ON ecl.lps_exclusion_override
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

-- ============================================================
-- INDEXES
-- ============================================================

-- (1) Active exclusion lookup per instrument: hot path in BulkAggregate JOIN
--     workflow_status = 'APPROVED_ACTIVE' partial filter aligns with query in §5 of p4-m3-lps.md
CREATE INDEX idx_lps_override_instrumen_status
    ON ecl.lps_exclusion_override (instrumen_id, workflow_status)
    WHERE deleted_at IS NULL;

-- (2) Approval queue: ROLE-ALCO pending queue ordered by submission time
CREATE INDEX idx_lps_override_status_created
    ON ecl.lps_exclusion_override (workflow_status, created_at DESC)
    WHERE deleted_at IS NULL;

-- (3) Maker filter: user's own proposals
CREATE INDEX idx_lps_override_maker_id
    ON ecl.lps_exclusion_override (maker_id)
    WHERE deleted_at IS NULL;

-- (4) Approver filter: approver's own decisions
CREATE INDEX idx_lps_override_approver_id
    ON ecl.lps_exclusion_override (approver_id)
    WHERE approver_id IS NOT NULL AND deleted_at IS NULL;

-- (5) Periode FK lookup: valid_from range queries
CREATE INDEX idx_lps_override_valid_from_periode
    ON ecl.lps_exclusion_override (valid_from_periode_id);

-- (6) Periode FK lookup: valid_to range queries + batch expiry job
--     Partial: only APPROVED_ACTIVE rows need expiry scan
CREATE INDEX idx_lps_override_valid_to_periode
    ON ecl.lps_exclusion_override (valid_to_periode_id)
    WHERE workflow_status = 'APPROVED_ACTIVE';

-- (7) Tenant + time: mandatory per db-conventions.md hot-table pattern
CREATE INDEX idx_lps_override_tenant_created
    ON ecl.lps_exclusion_override (tenant_id, created_at DESC);

-- (8) Partial hot-path: active approved exclusions for BulkAggregate LATERAL JOIN
--     Composite (instrumen_id, valid_from_periode_id, valid_to_periode_id) for range eval
CREATE INDEX idx_lps_override_approved_active_instrumen
    ON ecl.lps_exclusion_override (instrumen_id, valid_from_periode_id, valid_to_periode_id)
    WHERE workflow_status = 'APPROVED_ACTIVE' AND deleted_at IS NULL;

COMMIT;
