-- migration: 000024 fund_composition_lookthrough
-- author: data-modeler
-- requires: 0001 (init_schema — mst.instrumen, ecl.lookthrough_underlying, sec.user),
--           0004 (fn_increment_row_version, fn_update_updated_at),
--           0005 (fn_ecl_no_hard_delete — no-hard-delete on ecl.*),
--           0006 (doc.document FK target),
--           0007 (workflow pattern reference),
--           0023 (lps_exclusion_override — confirms ecl + mst schema pattern)
-- description: P4-M4 Look-through Reksadana.
--   (1) CREATE TABLE mst.fund_composition — 6-eyes workflow header (AKUN→RISK→ALCO)
--       with versioning via effective_from / effective_to, SoD CHECKs, signature columns.
--   (2) CREATE TABLE mst.fund_composition_detail — per-asset-class weight lines,
--       1-to-many to fund_composition, UNIQUE (fund_composition_id, asset_class),
--       trigger validates sum(weight_pct) = 100.0000 ± 0.0100 on APPROVED_ACTIVE transition.
--   (3) ALTER TABLE ecl.lookthrough_underlying — fix DEC-016 precision violations:
--       NUMERIC(20,2)→NUMERIC(20,4) for IDR amounts, NUMERIC(8,4)→NUMERIC(10,8) for PD/LGD.
--       ADD 11 new columns (fund_composition_id FK + per-scenario PD/ECL breakdowns + audit cols).
--       ADD triggers: no-hard-delete (ecl schema rule DEC-018) + updated_at + row_version.
--   Workflow states: DRAFT|PENDING_REVIEW|PENDING_APPROVAL|APPROVED_ACTIVE|SUPERSEDED|REJECTED
--   (DRAFT is a DB-level placeholder; API flow starts at PENDING_REVIEW on submit).

BEGIN;

-- ====================================================================
-- 1. mst.fund_composition — Fund Composition Header (6-eyes workflow)
-- ====================================================================
-- Versioning model (immutable after APPROVED_ACTIVE):
--   - effective_from / effective_to form a non-overlapping date range per instrumen.
--   - On amendment approve (atomik TX):
--       old row: workflow_status→SUPERSEDED, effective_to = new.effective_from - 1 day
--       new row: workflow_status→APPROVED_ACTIVE
--   - NEVER UPDATE rows that are APPROVED_ACTIVE or SUPERSEDED after approval.
-- 6-eyes: ROLE-AKUN (maker) → ROLE-RISK (reviewer) → ROLE-ALCO (approver, MFA wajib DEC-026).
-- SoD enforced by 3 CHECK constraints + application layer.
-- Soft-delete via deleted_at; hard-delete blocked by trigger (mst schema: not ecl, but best practice).

CREATE TABLE mst.fund_composition (
    -- Identity
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Instrument under composition (must be tipe_instrumen='REKSADANA')
    -- Validated at application layer + trigger below; DB FK enforces existence.
    instrumen_id            UUID        NOT NULL
                                REFERENCES mst.instrumen(id)
                                ON DELETE RESTRICT,

    -- Versioning date range (DATE semantics — business date, not timestamp)
    -- effective_from < effective_to enforced by CHECK below.
    -- Default effective_to = '9999-12-31' (open-ended / active version).
    effective_from          DATE        NOT NULL,
    effective_to            DATE        NOT NULL DEFAULT '9999-12-31',

    -- 6-eyes workflow state
    -- DRAFT              : internal placeholder (API flow does not expose this state externally).
    -- PENDING_REVIEW     : submitted by ROLE-AKUN, awaiting ROLE-RISK review.
    -- PENDING_APPROVAL   : reviewed by ROLE-RISK, awaiting ROLE-ALCO approval.
    -- APPROVED_ACTIVE    : fully approved; ECL calc engine may use this composition.
    -- SUPERSEDED         : replaced atomically when newer amendment is APPROVED_ACTIVE.
    -- REJECTED           : rejected at PENDING_REVIEW or PENDING_APPROVAL; terminal.
    workflow_status         TEXT        NOT NULL DEFAULT 'DRAFT'
                                CONSTRAINT chk_fc_workflow_status
                                CHECK (workflow_status IN (
                                    'DRAFT',
                                    'PENDING_REVIEW',
                                    'PENDING_APPROVAL',
                                    'APPROVED_ACTIVE',
                                    'SUPERSEDED',
                                    'REJECTED'
                                )),

    -- 6-eyes actors
    maker_id                UUID        NOT NULL
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,
    reviewer_id             UUID
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,
    approver_id             UUID
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,

    -- Review signature (populated atomically on PENDING_REVIEW → PENDING_APPROVAL)
    -- SHA-256(reviewer_id || 'REVIEW' || id || signed_at_review || comment_review)
    signed_at_review        TIMESTAMPTZ,
    signature_hash_review   BYTEA,
    comment_review          TEXT,

    -- Approval signature (populated atomically on PENDING_APPROVAL → APPROVED_ACTIVE)
    -- SHA-256(approver_id || 'APPROVE' || id || signed_at_approve || comment_approve)
    signed_at_approve       TIMESTAMPTZ,
    signature_hash_approve  BYTEA,
    comment_approve         TEXT,

    -- Rejection (populated when any actor rejects; terminal state)
    reject_reason           TEXT,

    -- Optional source document (fact sheet from MI/KSEI)
    source_doc_id           UUID
                                REFERENCES doc.document(id)
                                ON DELETE RESTRICT,

    -- ----------------------------------------------------------------
    -- Business constraint: effective date range must be ordered
    -- ----------------------------------------------------------------
    CONSTRAINT chk_fc_effective_date_order
        CHECK (effective_from < effective_to),

    -- ----------------------------------------------------------------
    -- SoD: 6-eyes — all three actors must be distinct (DEC-017)
    -- Null-safe: reviewr/approver may be NULL while still DRAFT/PENDING_REVIEW
    -- ----------------------------------------------------------------
    CONSTRAINT chk_fc_sod_rev
        CHECK (reviewer_id IS NULL OR maker_id <> reviewer_id),

    CONSTRAINT chk_fc_sod_appr
        CHECK (approver_id IS NULL OR maker_id <> approver_id),

    CONSTRAINT chk_fc_sod_rev_appr
        CHECK (reviewer_id IS NULL OR approver_id IS NULL OR reviewer_id <> approver_id),

    -- ----------------------------------------------------------------
    -- Approval consistency: when APPROVED_ACTIVE, all signature fields required
    -- ----------------------------------------------------------------
    CONSTRAINT chk_fc_approve_fields_when_approved
        CHECK (
            workflow_status <> 'APPROVED_ACTIVE'
            OR (
                approver_id           IS NOT NULL
                AND signed_at_approve IS NOT NULL
                AND signature_hash_approve IS NOT NULL
                AND reviewer_id           IS NOT NULL
                AND signed_at_review      IS NOT NULL
                AND signature_hash_review IS NOT NULL
            )
        ),

    -- ----------------------------------------------------------------
    -- Standard audit columns (db-conventions.md)
    -- ----------------------------------------------------------------
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE mst.fund_composition IS
    'Fund composition header for Reksadana instruments (P4-M4 Look-through ECL). '
    '6-eyes workflow: ROLE-AKUN maker → ROLE-RISK reviewer → ROLE-ALCO approver (MFA wajib). '
    'Versioning: effective_from/effective_to form non-overlapping date ranges per instrumen. '
    'Immutable after APPROVED_ACTIVE. Amendment creates new row; old row set to SUPERSEDED atomically. '
    'State machine: docs/state-machines/p4-m4-lookthrough.md §1. '
    'SoD: chk_fc_sod_rev, chk_fc_sod_appr, chk_fc_sod_rev_appr. '
    'instrumen_id must reference a REKSADANA instrument (enforced by trg_fc_check_reksadana).';

COMMENT ON COLUMN mst.fund_composition.effective_from IS
    'First date (inclusive) from which this composition version is valid. '
    'Must be < effective_to (chk_fc_effective_date_order).';

COMMENT ON COLUMN mst.fund_composition.effective_to IS
    'Last date (inclusive) through which this composition version is valid. '
    'Default ''9999-12-31'' means open-ended (currently active). '
    'Set to new_version.effective_from - 1 day when superseded by an amendment.';

COMMENT ON COLUMN mst.fund_composition.signature_hash_review IS
    'SHA-256(reviewer_id || ''REVIEW'' || id || signed_at_review || comment_review). '
    'Computed by application layer. Stored as BYTEA (32 bytes). Immutable after set.';

COMMENT ON COLUMN mst.fund_composition.signature_hash_approve IS
    'SHA-256(approver_id || ''APPROVE'' || id || signed_at_approve || comment_approve). '
    'Computed by application layer. Stored as BYTEA (32 bytes). Immutable after set.';

-- ====================================================================
-- TRIGGER 1a: Validate instrumen tipe = 'REKSADANA'
-- PG does not allow subqueries inside CHECK constraints; enforce via trigger.
-- Fires on INSERT and on UPDATE of instrumen_id (rare, but guard it).
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_fc_check_reksadana()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_tipe VARCHAR(30);
BEGIN
    SELECT tipe_instrumen INTO v_tipe
    FROM mst.instrumen
    WHERE id = NEW.instrumen_id;

    IF v_tipe IS NULL THEN
        RAISE EXCEPTION
            'LOOKTHROUGH_INSTRUMEN_NOT_FOUND: instrumen_id % not found in mst.instrumen',
            NEW.instrumen_id
            USING ERRCODE = 'P0001';
    END IF;

    IF v_tipe <> 'REKSADANA' THEN
        RAISE EXCEPTION
            'LOOKTHROUGH_INSTRUMEN_NOT_REKSADANA: fund_composition only valid for '
            'tipe_instrumen=REKSADANA. Instrument % has tipe_instrumen=%',
            NEW.instrumen_id, v_tipe
            USING ERRCODE = 'P0001';
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_fc_check_reksadana() IS
    'Validates that mst.fund_composition.instrumen_id references an instrument '
    'with tipe_instrumen=''REKSADANA''. '
    'Error codes: LOOKTHROUGH_INSTRUMEN_NOT_FOUND, LOOKTHROUGH_INSTRUMEN_NOT_REKSADANA (P0001).';

CREATE TRIGGER trg_fc_check_reksadana
    BEFORE INSERT OR UPDATE OF instrumen_id
    ON mst.fund_composition
    FOR EACH ROW EXECUTE FUNCTION fn_fc_check_reksadana();

-- ====================================================================
-- TRIGGER 1b: Standard updated_at + row_version
-- ====================================================================

CREATE TRIGGER trg_fc_updated_at
    BEFORE UPDATE ON mst.fund_composition
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_fc_row_version
    BEFORE UPDATE ON mst.fund_composition
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- TRIGGER 1c: Soft-delete enforcement (mst schema best practice)
-- Blocks hard DELETE to preserve audit history. Soft-delete via deleted_at.
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_mst_fc_no_hard_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'Hard DELETE on mst.fund_composition is not permitted. '
        'Use soft-delete: UPDATE ... SET deleted_at = now(), deleted_by = $user WHERE id = $id.'
        USING ERRCODE = 'P0001';
END;
$$;

COMMENT ON FUNCTION fn_mst_fc_no_hard_delete() IS
    'Blocks hard DELETE on mst.fund_composition. '
    'Soft-delete via deleted_at is the only permitted removal path. '
    'Error ERRCODE P0001.';

CREATE TRIGGER trg_fc_no_hard_delete
    BEFORE DELETE ON mst.fund_composition
    FOR EACH ROW EXECUTE FUNCTION fn_mst_fc_no_hard_delete();

-- ====================================================================
-- INDEXES on mst.fund_composition
-- ====================================================================

-- (1) Active composition lookup per instrument (hot path — ECL BulkCompute JOIN)
--     Aligns with query: instrumen_id=$1 AND workflow_status='APPROVED_ACTIVE'
--     AND effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $2)
CREATE INDEX idx_fc_instrumen_status
    ON mst.fund_composition (instrumen_id, workflow_status)
    WHERE deleted_at IS NULL;

-- (2) Point-in-time composition lookup: instrument + date range (covering index for bulk JOIN)
CREATE INDEX idx_fc_active_approved
    ON mst.fund_composition (instrumen_id, effective_from DESC)
    WHERE workflow_status = 'APPROVED_ACTIVE' AND deleted_at IS NULL;

-- (3) Approval/review queue: ROLE-RISK pending queue ordered by submission time
CREATE INDEX idx_fc_workflow_created
    ON mst.fund_composition (workflow_status, created_at DESC)
    WHERE deleted_at IS NULL;

-- (4) effective_to range scan: needed for amendment atomik update + expiry detection
CREATE INDEX idx_fc_effective_to
    ON mst.fund_composition (effective_to)
    WHERE workflow_status = 'APPROVED_ACTIVE';

-- (5) Maker's own submissions filter
CREATE INDEX idx_fc_maker_id
    ON mst.fund_composition (maker_id)
    WHERE deleted_at IS NULL;

-- (6) Reviewer filter (ROLE-RISK queue)
CREATE INDEX idx_fc_reviewer_id
    ON mst.fund_composition (reviewer_id)
    WHERE reviewer_id IS NOT NULL AND deleted_at IS NULL;

-- (7) Approver filter (ROLE-ALCO queue)
CREATE INDEX idx_fc_approver_id
    ON mst.fund_composition (approver_id)
    WHERE approver_id IS NOT NULL AND deleted_at IS NULL;

-- (8) Mandatory tenant + time composite per db-conventions.md
CREATE INDEX idx_fc_tenant_created
    ON mst.fund_composition (tenant_id, created_at DESC);


-- ====================================================================
-- 2. mst.fund_composition_detail — Asset Class Weight Lines
-- ====================================================================
-- 1-to-many to mst.fund_composition.
-- UNIQUE (fund_composition_id, asset_class) — one row per asset class per composition version.
-- weight_pct: NUMERIC(7,4) → range [0.0000, 100.0000] (CHECK constraint).
-- Sum validation (= 100.0000 ± 0.0100) enforced by trigger trg_fcd_weight_sum_check
--   which fires when parent fund_composition.workflow_status transitions to APPROVED_ACTIVE.
--   Trigger on THIS table fires on INSERT/UPDATE/DELETE to validate sum for the parent header.

CREATE TABLE mst.fund_composition_detail (
    -- Identity
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent fund composition header
    fund_composition_id     UUID        NOT NULL
                                REFERENCES mst.fund_composition(id)
                                ON DELETE CASCADE,

    -- Asset class (DEC-015 / §2.3 of p4-m4-lookthrough.md)
    asset_class             TEXT        NOT NULL
                                CONSTRAINT chk_fcd_asset_class
                                CHECK (asset_class IN (
                                    'GOVT_BOND',
                                    'CORP_BOND',
                                    'CASH',
                                    'EQUITY',
                                    'OTHER'
                                )),

    -- Weight percentage: NUMERIC(7,4) per DEC-016, range [0.0000, 100.0000]
    -- Individual line validation; sum validation is in trigger trg_fcd_weight_sum_check.
    weight_pct              NUMERIC(7,4) NOT NULL
                                CONSTRAINT chk_fcd_weight_positive
                                CHECK (weight_pct >= 0 AND weight_pct <= 100),

    -- UI display ordering (0 = first)
    position                INT         NOT NULL DEFAULT 0,

    -- One asset class per composition version
    CONSTRAINT uq_fcd_composition_asset_class
        UNIQUE (fund_composition_id, asset_class),

    -- ----------------------------------------------------------------
    -- Standard audit columns (db-conventions.md)
    -- ----------------------------------------------------------------
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL REFERENCES sec.user(id) ON DELETE RESTRICT,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    row_version     BIGINT      NOT NULL DEFAULT 1,
    tenant_id       TEXT        NOT NULL DEFAULT 'TUGURE'
);

COMMENT ON TABLE mst.fund_composition_detail IS
    'Per-asset-class weight lines for a fund composition version (P4-M4 Look-through ECL). '
    '1-to-many to mst.fund_composition. Max 5 rows (one per asset class enum) per version. '
    'UNIQUE (fund_composition_id, asset_class). '
    'Sum(weight_pct) = 100.0000 ± 0.0100 enforced by trg_fcd_weight_sum_check '
    'which fires on INSERT/UPDATE/DELETE and re-validates the parent header sum. '
    'asset_class enum: GOVT_BOND, CORP_BOND, CASH, EQUITY, OTHER (per FSD-APP-C §3, DEC-015). '
    'weight_pct: NUMERIC(7,4) per DEC-016. position: UI display order.';

COMMENT ON COLUMN mst.fund_composition_detail.weight_pct IS
    'Weight of this asset class as a percentage of total NAB. '
    'Range [0.0000, 100.0000]. NUMERIC(7,4) per DEC-016. '
    'Sum across all non-deleted lines for parent must equal 100.0000 ± 0.0100 '
    'when parent workflow_status = APPROVED_ACTIVE (enforced by trg_fcd_weight_sum_check).';

-- ====================================================================
-- TRIGGER 2a: Validate sum(weight_pct) = 100.0000 ± 0.0100
-- Fires on INSERT/UPDATE/DELETE on detail; re-checks sum for parent fund_composition_id.
-- Validation is active only when parent workflow_status = 'APPROVED_ACTIVE'.
-- For DRAFT/PENDING_* states, partial input is allowed (sum validation at submit/approve).
-- ====================================================================

CREATE OR REPLACE FUNCTION fn_fcd_check_weight_sum()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    v_composition_id UUID;
    v_workflow_status TEXT;
    v_sum            NUMERIC(10,4);
    v_tolerance      NUMERIC(10,4) := 0.0100;
BEGIN
    -- Determine which composition_id to validate
    IF TG_OP = 'DELETE' THEN
        v_composition_id := OLD.fund_composition_id;
    ELSE
        v_composition_id := NEW.fund_composition_id;
    END IF;

    -- Read parent workflow status
    SELECT workflow_status INTO v_workflow_status
    FROM mst.fund_composition
    WHERE id = v_composition_id;

    -- Only enforce sum constraint when parent is APPROVED_ACTIVE
    -- (DRAFT / PENDING_* allow incremental line entry without full weight sum)
    IF v_workflow_status <> 'APPROVED_ACTIVE' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        ELSE
            RETURN NEW;
        END IF;
    END IF;

    -- Compute sum of non-deleted detail lines for this composition
    SELECT COALESCE(SUM(weight_pct), 0)
    INTO v_sum
    FROM mst.fund_composition_detail
    WHERE fund_composition_id = v_composition_id
      AND deleted_at IS NULL
      AND id <> CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE '00000000-0000-0000-0000-000000000000'::UUID END;

    -- Add new/updated row weight for INSERT and UPDATE
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        v_sum := v_sum + NEW.weight_pct;
    END IF;

    IF ABS(v_sum - 100.0000) > v_tolerance THEN
        RAISE EXCEPTION
            'LOOKTHROUGH_WEIGHT_INVALID: sum(weight_pct) for fund_composition % = % '
            '(must be 100.0000 ± 0.0100)',
            v_composition_id, v_sum
            USING ERRCODE = 'P0001';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$;

COMMENT ON FUNCTION fn_fcd_check_weight_sum() IS
    'Validates that SUM(weight_pct) for all non-deleted detail lines of a fund_composition '
    'equals 100.0000 ± 0.0100. Enforced only when parent workflow_status = APPROVED_ACTIVE. '
    'Error code: LOOKTHROUGH_WEIGHT_INVALID (P0001). '
    'Tolerance 0.0100 accommodates NUMERIC(7,4) rounding at UI input boundary.';

CREATE TRIGGER trg_fcd_weight_sum_check
    BEFORE INSERT OR UPDATE OF weight_pct, deleted_at OR DELETE
    ON mst.fund_composition_detail
    FOR EACH ROW EXECUTE FUNCTION fn_fcd_check_weight_sum();

-- ====================================================================
-- TRIGGER 2b: Standard updated_at + row_version
-- ====================================================================

CREATE TRIGGER trg_fcd_updated_at
    BEFORE UPDATE ON mst.fund_composition_detail
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_fcd_row_version
    BEFORE UPDATE ON mst.fund_composition_detail
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ====================================================================
-- INDEXES on mst.fund_composition_detail
-- ====================================================================

-- (1) FK parent lookup (db-conventions.md: every FK must be indexed)
CREATE INDEX idx_fcd_composition_id
    ON mst.fund_composition_detail (fund_composition_id)
    WHERE deleted_at IS NULL;


-- ====================================================================
-- 3. ALTER TABLE ecl.lookthrough_underlying — DEC-016 precision fix + enhancements
-- ====================================================================
-- DEC-016 violations in init schema (000001):
--   ead_underlying_idr  NUMERIC(20,2) → NUMERIC(20,4)  [IDR amounts must be NUMERIC(20,4)]
--   pd_normal           NUMERIC(8,4)  → NUMERIC(10,8)  [PD/LGD must be NUMERIC(10,8)]
--   lgd                 NUMERIC(8,4)  → NUMERIC(10,8)
--   ecl_weighted_idr    NUMERIC(20,2) → NUMERIC(20,4)
-- New columns per hand-off spec (docs/state-machines/p4-m4-lookthrough.md §8):
--   fund_composition_id — FK to mst.fund_composition (version used in calc)
--   asset_class         — replaces/aliases underlying_kategori with standardised enum
--   pd_good, pd_bad     — per-scenario PD values (pd_normal already exists)
--   ecl_skenario_* IDR  — ECL per scenario before FL multiplier
--   ecl_fl_* IDR        — ECL per scenario after FL multiplier
--   audit cols          — created_at, created_by, updated_at, updated_by,
--                         deleted_at, deleted_by, row_version, tenant_id
-- No-hard-delete trigger already exists (tg_ecl_lookthrough_no_delete in migration 000005).
-- Updated_at + row_version triggers are NEW (not in 000001 or 000005).

-- 3a. Fix precision on existing IDR columns
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN ead_underlying_idr
        TYPE NUMERIC(20,4) USING ead_underlying_idr::NUMERIC(20,4),
    ALTER COLUMN ecl_weighted_idr
        TYPE NUMERIC(20,4) USING ecl_weighted_idr::NUMERIC(20,4);

-- 3b. Fix precision on existing PD / LGD columns
ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN pd_normal
        TYPE NUMERIC(10,8) USING pd_normal::NUMERIC(10,8),
    ALTER COLUMN lgd
        TYPE NUMERIC(10,8) USING lgd::NUMERIC(10,8);

-- 3c. Add fund_composition_id FK (nullable: historical rows pre-000024 have no composition)
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS fund_composition_id  UUID
        REFERENCES mst.fund_composition(id)
        ON DELETE RESTRICT;

COMMENT ON COLUMN ecl.lookthrough_underlying.fund_composition_id IS
    'FK to mst.fund_composition (the version active at calc evaluation date). '
    'NULL for rows created before migration 000024 (pre-P4-M4). '
    'Populated by ECL calc engine for all new look-through rows.';

-- 3d. Add standardised asset_class column
--     underlying_kategori (VARCHAR 50) is kept for backward-compat; asset_class is the new canonical column.
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS asset_class  TEXT
        CONSTRAINT chk_lookthrough_asset_class
        CHECK (asset_class IS NULL OR asset_class IN (
            'GOVT_BOND', 'CORP_BOND', 'CASH', 'EQUITY', 'OTHER'
        ));

COMMENT ON COLUMN ecl.lookthrough_underlying.asset_class IS
    'Standardised asset class enum (DEC-015). Replaces free-text underlying_kategori. '
    'NULL for rows created before migration 000024. '
    'Enum: GOVT_BOND, CORP_BOND, CASH, EQUITY, OTHER.';

-- 3e. Add per-scenario PD columns (NUMERIC(10,8) per DEC-016)
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS pd_good  NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS pd_bad   NUMERIC(10,8);

COMMENT ON COLUMN ecl.lookthrough_underlying.pd_good IS
    'PD for Good scenario. NUMERIC(10,8) per DEC-016. NULL for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.pd_bad IS
    'PD for Bad scenario. NUMERIC(10,8) per DEC-016. NULL for pre-000024 rows.';

-- 3f. Add per-scenario ECL before FL multiplier (NUMERIC(20,4) per DEC-016)
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS ecl_skenario_good_idr    NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS ecl_skenario_normal_idr  NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS ecl_skenario_bad_idr     NUMERIC(20,4);

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_skenario_good_idr IS
    'ECL for Good scenario before applying FL multiplier. '
    '= NAB_portion × PD_good × LGD. NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_skenario_normal_idr IS
    'ECL for Normal scenario before applying FL multiplier. '
    '= NAB_portion × pd_normal × LGD. NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_skenario_bad_idr IS
    'ECL for Bad scenario before applying FL multiplier. '
    '= NAB_portion × PD_bad × LGD. NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

-- 3g. Add per-scenario ECL after FL multiplier (NUMERIC(20,4) per DEC-016)
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS ecl_fl_good_idr    NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS ecl_fl_normal_idr  NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS ecl_fl_bad_idr     NUMERIC(20,4);

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_fl_good_idr IS
    'ECL for Good scenario after FL multiplier = ecl_skenario_good_idr × fl_multiplier_good. '
    'NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_fl_normal_idr IS
    'ECL for Normal scenario after FL multiplier. NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.ecl_fl_bad_idr IS
    'ECL for Bad scenario after FL multiplier. NUMERIC(20,4) per DEC-016. NULL for pre-000024 rows.';

-- 3h. Add full audit columns (db-conventions.md — required on all tables)
--     These were absent from the init schema (000001) for this table.
ALTER TABLE ecl.lookthrough_underlying
    ADD COLUMN IF NOT EXISTS created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS created_by   UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by   UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS deleted_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by   UUID        REFERENCES sec.user(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS row_version  BIGINT      NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id    TEXT        NOT NULL DEFAULT 'TUGURE';

COMMENT ON COLUMN ecl.lookthrough_underlying.created_at IS
    'Row creation timestamp. Backfilled to now() for pre-000024 rows.';

COMMENT ON COLUMN ecl.lookthrough_underlying.tenant_id IS
    'Tenant identifier. Single-tenant Phase 1 = ''TUGURE''. Placeholder for Phase 2 MT.';

-- 3i. Add updated_at + row_version triggers
--     (no-hard-delete tg_ecl_lookthrough_no_delete already exists from migration 000005)

CREATE TRIGGER trg_lookthrough_updated_at
    BEFORE UPDATE ON ecl.lookthrough_underlying
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_lookthrough_row_version
    BEFORE UPDATE ON ecl.lookthrough_underlying
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- 3j. New indexes for added columns
--     Existing indexes: ix_lookthrough_header (ecl_calc_header_id),
--                       ix_lookthrough_kategori (underlying_kategori)
--     tg_ecl_lookthrough_no_delete on this table from 000005 — still in force.

-- FK index: fund_composition_id
CREATE INDEX idx_lookthrough_composition_id
    ON ecl.lookthrough_underlying (fund_composition_id)
    WHERE fund_composition_id IS NOT NULL;

-- asset_class lookup (aligns with bulk JOIN per-class ECL aggregation)
CREATE INDEX idx_lookthrough_asset_class
    ON ecl.lookthrough_underlying (asset_class)
    WHERE asset_class IS NOT NULL;

-- Tenant + time (mandatory per db-conventions.md)
CREATE INDEX idx_lookthrough_tenant_created
    ON ecl.lookthrough_underlying (tenant_id, created_at DESC);

COMMIT;
