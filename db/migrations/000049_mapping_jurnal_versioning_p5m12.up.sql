-- migration: 000049 mapping_jurnal_versioning_p5m12
-- author: backend-engineer-go (data-modeler pattern)
-- requires: 000035 (jurnal_engine_p5_m2 — mst.mapping_jurnal_header full workflow cols),
--           000048 (bulk_upload_p5m11 — sys.upload_batch MAPPING_BULK batch_type)
-- description:
--   P5-M12 Mapping Jurnal CRUD + 6-Eyes Workflow versioning schema.
--   (A) ALTER mst.mapping_jurnal_header: add version-chain columns
--       (parent_id, effective_from, effective_to, regulated_flag, signature_hash_2, step_up_token_ref)
--   (B) Add partial UNIQUE index: one ACTIVE mapping per event_code per tenant
--   (C) BEFORE UPDATE trigger: immutability guard for APPROVED_ACTIVE rows
--       (only effective_to + standard audit cols allowed to change)
--   (D) Seed sys.config: MAPPING_REGULATED_EVENT_CODES
--   (E) UPDATE existing seeded rows: set regulated_flag based on event_code
--
-- References: P5-M12-S1..S5, DEC-017, DEC-018, DEC-021, DEC-027.

BEGIN;

-- ====================================================================
-- A. ALTER mst.mapping_jurnal_header — version chain columns
-- ====================================================================

-- A1. parent_id: FK to self — links new version to its predecessor (immutable history, DEC-018)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS parent_id UUID REFERENCES mst.mapping_jurnal_header(id);

COMMENT ON COLUMN mst.mapping_jurnal_header.parent_id IS
    'FK to predecessor version of same event_code. NULL for seed rows and first versions. '
    'When editing APPROVED_ACTIVE, INSERT new row with parent_id = prior version''s id. '
    'Never UPDATE existing rows (DEC-018 immutability). Chain: v1.parent_id=NULL, v2.parent_id=v1.id, etc.';

-- A2. effective_from: set at INSERT of new version (now())
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS effective_from TIMESTAMPTZ;

COMMENT ON COLUMN mst.mapping_jurnal_header.effective_from IS
    'Timestamp when this version became / will become effective. '
    'Set at INSERT time. For APPROVED_ACTIVE: the activate timestamp. '
    'NULL for seed DRAFT rows (set at submit time by application).';

-- A3. effective_to: NULL = open-ended (current ACTIVE); set when superseded by newer APPROVED_ACTIVE
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS effective_to TIMESTAMPTZ DEFAULT 'infinity'::TIMESTAMPTZ;

COMMENT ON COLUMN mst.mapping_jurnal_header.effective_to IS
    'Null / infinity = this is the current version for resolver. '
    'Set to now() when a newer version is approved (atomic flip in same TX as approve step). '
    'Only this column (+ standard audit cols) may be UPDATED on APPROVED_ACTIVE rows (immutability trigger).';

-- A4. regulated_flag: TRUE for ECL/EIR/MTM/REKLAS events requiring 6-eyes (DEC-017)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS regulated_flag BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN mst.mapping_jurnal_header.regulated_flag IS
    'TRUE if event_code is in MAPPING_REGULATED_EVENT_CODES (sys.config). '
    'Determines 6-eyes path at submit time. Set by backend service, not by user. '
    'Immutable once set (APPROVED_ACTIVE rows cannot change this).';

-- A5. signature_hash_2 for 6-eyes approver_2 (ROLE-RISK)
--     Note: approver_2_signature_hash already exists from migration 000035 (A5 there).
--     This adds an alias column for step_up_token_ref as audit evidence.
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS step_up_token_ref BYTEA;

COMMENT ON COLUMN mst.mapping_jurnal_header.step_up_token_ref IS
    'SHA-256 hash of the X-Step-Up-Token used for approve-2 (DEC-027). '
    'Stored as BYTEA audit evidence. NULL for 4-eyes path. '
    'Cleared on rejection back to DRAFT.';

-- A6. Index on parent_id for version chain traversal
CREATE INDEX IF NOT EXISTS idx_mapping_header_parent_id
    ON mst.mapping_jurnal_header (parent_id)
    WHERE parent_id IS NOT NULL;

-- A7. Composite index for list query performance (tenant + status + event_code)
CREATE INDEX IF NOT EXISTS idx_mapping_header_tenant_status_event
    ON mst.mapping_jurnal_header (tenant_id, workflow_status, event_code)
    WHERE deleted_at IS NULL;

-- A8. Index for resolver lookup (active mapping by event_code per tenant)
CREATE INDEX IF NOT EXISTS idx_mapping_header_active_event
    ON mst.mapping_jurnal_header (tenant_id, event_code)
    WHERE workflow_status = 'APPROVED_ACTIVE' AND deleted_at IS NULL;

-- A9. Index for regulated_flag filter
CREATE INDEX IF NOT EXISTS idx_mapping_header_regulated
    ON mst.mapping_jurnal_header (regulated_flag, workflow_status)
    WHERE deleted_at IS NULL;

-- ====================================================================
-- B. Partial UNIQUE index: one APPROVED_ACTIVE mapping per event_code per tenant
-- ====================================================================
-- Prevents duplicate ACTIVE versions at DB level (defense-in-depth).
-- Application (service.go) enforces this via atomic flip, but DB index is belt-and-suspenders.
CREATE UNIQUE INDEX IF NOT EXISTS uq_mapping_header_one_active_per_event
    ON mst.mapping_jurnal_header (tenant_id, event_code)
    WHERE workflow_status = 'APPROVED_ACTIVE' AND deleted_at IS NULL;

COMMENT ON INDEX uq_mapping_header_one_active_per_event IS
    'DB-level guard: only one APPROVED_ACTIVE mapping per (tenant_id, event_code). '
    'Application service atomically flips prior active to effective_to before this INSERT. '
    'Belt-and-suspenders for DEC-018 immutability.';

-- ====================================================================
-- C. BEFORE UPDATE trigger: APPROVED_ACTIVE immutability guard
-- ====================================================================
-- When a row has workflow_status = 'APPROVED_ACTIVE', only the following mutations are permitted:
--   effective_to, updated_at, updated_by, row_version
-- All other column mutations on APPROVED_ACTIVE rows MUST go through INSERT new version.
-- This enforces DEC-018 audit-grade immutability for mapping history.

CREATE OR REPLACE FUNCTION fn_mapping_header_active_immutability()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    -- Only fire for APPROVED_ACTIVE rows that exist before this UPDATE
    IF OLD.workflow_status = 'APPROVED_ACTIVE' THEN
        -- Allow: effective_to, updated_at, updated_by, row_version (standard audit flip)
        -- Block: any other column change
        IF (
            NEW.event_code               IS DISTINCT FROM OLD.event_code OR
            NEW.event_id_kode            IS DISTINCT FROM OLD.event_id_kode OR
            NEW.nama_event               IS DISTINCT FROM OLD.nama_event OR
            NEW.kategori_event           IS DISTINCT FROM OLD.kategori_event OR
            NEW.aktif_flag               IS DISTINCT FROM OLD.aktif_flag OR
            NEW.workflow_status          IS DISTINCT FROM OLD.workflow_status OR
            NEW.workflow_path            IS DISTINCT FROM OLD.workflow_path OR
            NEW.regulated_flag           IS DISTINCT FROM OLD.regulated_flag OR
            NEW.maker_id                 IS DISTINCT FROM OLD.maker_id OR
            NEW.reviewer_id              IS DISTINCT FROM OLD.reviewer_id OR
            NEW.approver_id              IS DISTINCT FROM OLD.approver_id OR
            NEW.approver_2_id            IS DISTINCT FROM OLD.approver_2_id OR
            NEW.reviewer_signature_hash  IS DISTINCT FROM OLD.reviewer_signature_hash OR
            NEW.approver_signature_hash  IS DISTINCT FROM OLD.approver_signature_hash OR
            NEW.approver_2_signature_hash IS DISTINCT FROM OLD.approver_2_signature_hash OR
            NEW.parent_id                IS DISTINCT FROM OLD.parent_id OR
            NEW.effective_from           IS DISTINCT FROM OLD.effective_from OR
            NEW.reject_reason            IS DISTINCT FROM OLD.reject_reason OR
            NEW.tenant_id                IS DISTINCT FROM OLD.tenant_id
        ) THEN
            RAISE EXCEPTION
                'MAPPING_ACTIVE_IMMUTABILITY: mst.mapping_jurnal_header row % has workflow_status=APPROVED_ACTIVE. '
                'Only effective_to and standard audit columns (updated_at, updated_by, row_version) '
                'may be changed. To modify mapping logic, INSERT a new version with parent_id = this row''s id. '
                'DEC-018 audit-grade immutability.', OLD.id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_mapping_header_active_immutability() IS
    'DEC-018 enforcement: APPROVED_ACTIVE mapping header rows are immutable except '
    'effective_to (set when superseded) and standard audit cols. '
    'Any other mutation → EXCEPTION MAPPING_ACTIVE_IMMUTABILITY. '
    'P5-M12 versioning pattern: edit = INSERT new version, never UPDATE.';

-- Drop + recreate to ensure idempotency on re-run
DROP TRIGGER IF EXISTS trg_mapping_header_active_immutability ON mst.mapping_jurnal_header;

CREATE TRIGGER trg_mapping_header_active_immutability
    BEFORE UPDATE ON mst.mapping_jurnal_header
    FOR EACH ROW EXECUTE FUNCTION fn_mapping_header_active_immutability();

COMMENT ON TRIGGER trg_mapping_header_active_immutability ON mst.mapping_jurnal_header IS
    'Fires BEFORE UPDATE. Blocks mutation of business columns on APPROVED_ACTIVE rows. '
    'See fn_mapping_header_active_immutability() for allowed columns.';

-- ====================================================================
-- D. Seed sys.config: MAPPING_REGULATED_EVENT_CODES
-- ====================================================================
-- Comma-separated list of event_codes that require 6-eyes workflow (ROLE-RISK approve-2 + step-up MFA).
-- ROLE-IT-ADMIN can update this via POST /api/v1/sys/config (separate config endpoint).
-- Backend reads this at submit time to determine workflow_path.

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'MAPPING_REGULATED_EVENT_CODES',
    'ECL_PEMBENTUKAN,ECL_REVERSAL,POCI_DELTA_ECL,MTM_FVTPL,MTM_FVOCI,MTM_FVOCI_ELECTION,REKLAS_OCI_PL,REKLASIFIKASI_AC_FVOCI,REKLASIFIKASI_FVOCI_AC,MODIFIKASI_MATERIAL,EIR_CATCH_UP_ADJUSTMENT,STAGE_MIGRATION,FX_UNREALIZED',
    'TEXT',
    FALSE,
    'Comma-separated list of event_codes requiring 6-eyes workflow (ROLE-RISK approve-2 + step-up MFA DEC-027). '
    'Seeded by P5-M12. ROLE-IT-ADMIN can update via /api/v1/sys/config. '
    'Backend reads this at submit time (MappingService.Submit) to set workflow_path. '
    'Changes take effect for new version submissions, not in-flight PENDING_* versions.',
    'JURNAL'
)
ON CONFLICT (config_key) DO UPDATE
    SET config_value = EXCLUDED.config_value,
        description  = EXCLUDED.description,
        updated_at   = now();

COMMENT ON TABLE sys.config IS
    'System configuration. MAPPING_REGULATED_EVENT_CODES: P5-M12 regulated event whitelist. '
    'ECL/EIR/MTM/REKLAS events require 6-eyes workflow per DEC-017. See P5-M12-S2.';

-- ====================================================================
-- E. UPDATE existing seeded rows: set regulated_flag based on event_code
-- ====================================================================
-- Seeds from migration 000035 have regulated_flag = FALSE (default).
-- Update regulated events to TRUE so the resolver and UI display correct workflow_path.
-- Note: These rows are in DRAFT status so the immutability trigger does NOT fire.

UPDATE mst.mapping_jurnal_header
SET regulated_flag = TRUE,
    updated_at     = now(),
    row_version    = row_version + 1
WHERE event_code IN (
    'ECL_PEMBENTUKAN',
    'ECL_REVERSAL',
    'POCI_DELTA_ECL',
    'MTM_FVTPL',
    'MTM_FVOCI',
    'MTM_FVOCI_ELECTION',
    'REKLAS_OCI_PL',
    'REKLASIFIKASI_AC_FVOCI',
    'REKLASIFIKASI_FVOCI_AC',
    'MODIFIKASI_MATERIAL',
    'EIR_CATCH_UP_ADJUSTMENT',
    'STAGE_MIGRATION',
    'FX_UNREALIZED'
)
  AND workflow_status != 'APPROVED_ACTIVE'  -- immutability guard (also: they are DRAFT)
  AND deleted_at IS NULL;

-- Set effective_to = 'infinity' for all existing DRAFT rows where effective_to is NULL
-- (NULL was the prior default; infinity is the new explicit default)
UPDATE mst.mapping_jurnal_header
SET effective_to = 'infinity'::TIMESTAMPTZ,
    updated_at   = now()
WHERE effective_to IS NULL
  AND deleted_at IS NULL;

-- Verify counts
DO $$
DECLARE
    v_regulated_count INT;
    v_total_count     INT;
BEGIN
    SELECT COUNT(*) INTO v_regulated_count
    FROM mst.mapping_jurnal_header
    WHERE regulated_flag = TRUE AND deleted_at IS NULL;

    SELECT COUNT(*) INTO v_total_count
    FROM mst.mapping_jurnal_header
    WHERE deleted_at IS NULL;

    RAISE NOTICE 'P5-M12 migration 000049: % total mapping_jurnal_header rows, % marked regulated',
        v_total_count, v_regulated_count;

    IF v_regulated_count < 13 THEN
        RAISE WARNING 'Expected ≥ 13 regulated rows (13 event codes seeded as 6-eyes); got %. '
            'Check mst.mapping_jurnal_header seeds from migration 000035.',
            v_regulated_count;
    END IF;
END;
$$;

COMMIT;
