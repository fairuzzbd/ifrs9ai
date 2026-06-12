-- migration: 0029 ecl_core_tables
-- author: data-modeler
-- requires: 0001 (init_schema — ecl.calc_header, ecl.calc_detail_skenario, sys.job_run_history),
--           0004 (sys_job_idempotency — sys.job, fn_increment_row_version),
--           0005 (audit_log_hardening — fn_ecl_no_hard_delete),
--           0006 (doc_document),
--           0026 (eir_schema_fix),
--           0027 (drift_report_and_amendment_lifecycle),
--           0028 (amendment_detection_idempotency)
-- description:
--   P4-M7 ECL Core schema support:
--   (A) CREATE TABLE ecl.calc_result_line — consolidated per-instrument-per-run ECL result.
--       3 scenarios flattened inline (no child table split).
--       PARTITION BY RANGE (created_at) monthly.
--       UNIQUE (calc_run_id, instrumen_id). Hard-delete REJECT via fn_ecl_no_hard_delete.
--       Triggers: updated_at, row_version.
--       Indexes: calc_run_id, instrumen_id+eval_date, stage (partial), routing_path (partial),
--                flag_poci (partial), tenant_id+created_at.
--   (B) ALTER TABLE ecl.calc_header — precision fixes (NUMERIC(20,4) + NUMERIC(10,8)),
--       new columns (routing_path, flag_poci, ead_idr precision fix, pd_used_*, lgd_used,
--       net_carrying_idr, sealed_at, calc_run_id migration FK → sys.job, catatan),
--       missing audit cols (updated_at, updated_by, deleted_at, deleted_by, row_version, tenant_id),
--       stage CHECK expansion (SMALLINT 1/2/3 alias for STAGE_1/STAGE_2/STAGE_3),
--       status column + CHECK, triggers.
--       Trigger: fn_ecl_calc_no_modify_when_sealed (BEFORE UPDATE guard when sealed_at IS NOT NULL).
--   (C) ALTER TABLE ecl.calc_detail_skenario — precision fixes (NUMERIC(10,8) / NUMERIC(20,4)),
--       new columns (fl_multiplier, ecl_fl_idr, ead_skenario_idr, audit cols),
--       triggers: updated_at, row_version.
--
--   OQ-M7-2 resolution: calc_run_id on ecl.calc_header currently FK → sys.job_run_history(id)
--   (init schema 000001). sys.job(id) (TEXT PK, ULID) exists from 000004 but has a different
--   PK type (TEXT vs UUID). To avoid a breaking PK-type change on the FK side, this migration:
--   (i)  Adds new column calc_run_job_id TEXT REFERENCES sys.job(id) alongside the existing
--        calc_run_id UUID FK for backward compat.
--   (ii) Documents intent: M8 will migrate calc_run_id data → calc_run_job_id and drop
--        the old column in a separate deprecation-cycle migration.
--   ecl.calc_result_line.calc_run_id is an unresolved UUID (no FK) with COMMENT explaining
--   the deferred FK until the PK-type resolution in M8.

BEGIN;

-- ====================================================================
-- A. ecl.calc_result_line  (new table, partitioned)
-- ====================================================================

CREATE TABLE ecl.calc_result_line (
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Run identity
    -- calc_run_id intentionally has NO FK constraint.
    -- Reason: ecl.calc_header.id is UUID; sys.job.id is TEXT (ULID).
    -- The canonical join key for the run is ecl.calc_header.calc_run_id (UUID FK → sys.job_run_history).
    -- OQ-M7-2: M8 will resolve the FK chain once sys.job.id is normalised to UUID or
    -- a bridge table is introduced.  Do NOT add FK here until M8 confirms target table + type.
    calc_run_id             UUID            NOT NULL,

    instrumen_id            UUID            NOT NULL REFERENCES mst.instrumen(id)
                                                ON DELETE RESTRICT,
    evaluation_date         DATE            NOT NULL,
    periode_id              TEXT            NOT NULL,

    -- Staging
    stage                   SMALLINT        NOT NULL
                                CONSTRAINT chk_ecl_result_line_stage
                                CHECK (stage IN (1, 2, 3)),

    -- Routing decision (STANDARD|LPS|LOOKTHROUGH|SKIP_FVTPL|POCI_DEFERRED)
    routing_path            TEXT            NOT NULL
                                CONSTRAINT chk_ecl_result_line_routing_path
                                CHECK (routing_path IN (
                                    'STANDARD', 'LPS', 'LOOKTHROUGH',
                                    'SKIP_FVTPL', 'POCI_DEFERRED'
                                )),

    -- EAD (post-LPS excess if LPS routing; NULL for SKIP_FVTPL)
    ead_idr                 NUMERIC(20,4),

    -- PD snapshot per scenario (Stage 3: all = 1.00000000 forced)
    pd_used_good            NUMERIC(10,8),
    pd_used_normal          NUMERIC(10,8),
    pd_used_bad             NUMERIC(10,8),

    -- LGD snapshot (pool-based, Basel-style)
    lgd_used                NUMERIC(10,8),

    -- FL multipliers — NULL for Stage 3 (FL not applied) and SKIP_FVTPL/POCI_DEFERRED
    fl_multiplier_good      NUMERIC(10,8),
    fl_multiplier_normal    NUMERIC(10,8),
    fl_multiplier_bad       NUMERIC(10,8),

    -- ECL pre-FL per scenario (NULL for SKIP_FVTPL/POCI_DEFERRED)
    ecl_good_idr            NUMERIC(20,4),
    ecl_normal_idr          NUMERIC(20,4),
    ecl_bad_idr             NUMERIC(20,4),

    -- ECL post-FL per scenario (Stage 3: same as pre-FL; FL skipped)
    ecl_fl_good_idr         NUMERIC(20,4),
    ecl_fl_normal_idr       NUMERIC(20,4),
    ecl_fl_bad_idr          NUMERIC(20,4),

    -- Weighted ECL = Σ(ECL_FL_skenario × bobot_skenario)
    -- NULL for POCI_DEFERRED (semantics differ from 0 — credit-adjusted EIR not yet computed)
    -- 0.0000 for SKIP_FVTPL
    ecl_weighted_idr        NUMERIC(20,4),

    -- Bobot snapshot at compute time (ALCO may override — snapshot preserved for audit)
    bobot_good              NUMERIC(7,4)    NOT NULL DEFAULT 0.2500,
    bobot_normal            NUMERIC(7,4)    NOT NULL DEFAULT 0.5000,
    bobot_bad               NUMERIC(7,4)    NOT NULL DEFAULT 0.2500,
    CONSTRAINT chk_ecl_result_line_bobot_sum
        CHECK (bobot_good + bobot_normal + bobot_bad BETWEEN 0.9999 AND 1.0001),

    -- Stage 3 net carrying = EAD − prior_sealed_ecl (NULL for Stage 1/2 and SKIP_FVTPL)
    net_carrying_idr        NUMERIC(20,4),

    -- Prior sealed ECL used for Stage 3 net-carrying base (NULL if first run)
    prior_sealed_ecl_idr    NUMERIC(20,4),

    -- POCI flag (snapshot from mst.instrumen at compute time)
    flag_poci               BOOLEAN         NOT NULL DEFAULT FALSE,

    -- Parameter set snapshot reference (FK intentionally nullable; parameter snapshot
    -- table is managed by M8 parameter-sealing workflow)
    parameter_snapshot_id   UUID,

    -- Warnings — array of warning code strings, e.g. ["STAGE_3_NET_CARRYING_FIRST_RUN"]
    warnings_json           JSONB           NOT NULL DEFAULT '[]',

    -- Standard audit columns (db-conventions.md — mandatory)
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    created_by              UUID            NOT NULL,
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_by              UUID            NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT          NOT NULL DEFAULT 1,
    tenant_id               TEXT            NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT uq_ecl_calc_result_run_instrumen
        UNIQUE (calc_run_id, instrumen_id)
)
PARTITION BY RANGE (created_at);

COMMENT ON TABLE ecl.calc_result_line IS
    'Consolidated per-instrument-per-run ECL result. One row per instrumen per calc run. '
    '3 scenarios flattened inline (good/normal/bad). '
    'routing_path determines which computation path was taken. '
    'POCI_DEFERRED: ecl_weighted_idr = NULL (not 0 — credit-adjusted EIR not computed). '
    'SKIP_FVTPL: ecl_weighted_idr = 0.0000, no row written in practice (M7 routing). '
    'Stage 3: fl_multiplier_* = NULL (FL not applied per PSAK 71/IFRS 9). '
    'Partitioned monthly by created_at per db-conventions.md. '
    'No hard delete — guard: trg_ecl_calc_result_line_no_delete (fn_ecl_no_hard_delete). '
    'OQ-M7-2: calc_run_id FK to sys.job deferred to M8 (PK type mismatch UUID vs TEXT).';

COMMENT ON COLUMN ecl.calc_result_line.calc_run_id IS
    'UUID identifying the ECL calc run. Logically FK → ecl.calc_header(calc_run_id column). '
    'No DB-level FK constraint here (OQ-M7-2: M8 will resolve once sys.job PK type is unified). '
    'Application must validate existence before INSERT.';

COMMENT ON COLUMN ecl.calc_result_line.ecl_weighted_idr IS
    'NULL for POCI_DEFERRED — semantics differ from 0 (ECL not computed, not zero). '
    '0.0000 for SKIP_FVTPL (FVTPL/FVOCI_ELECTION instruments carry no ECL charge).';

COMMENT ON COLUMN ecl.calc_result_line.fl_multiplier_good IS
    'NULL for Stage 3 (FL multiplier not applied per PSAK 71 §5.5.17). '
    'NULL for SKIP_FVTPL and POCI_DEFERRED (not computed). '
    'Source: mst.impact_mev_pd[skenario=GOOD].impact_multiplier. No double-multiply.';

COMMENT ON COLUMN ecl.calc_result_line.prior_sealed_ecl_idr IS
    'Latest sealed ECL for this instrument (from most recent sealed calc run). '
    'Used as base for Stage 3 net carrying: net_carrying = ead_idr - prior_sealed_ecl_idr. '
    'NULL on first run → net_carrying = ead_idr + warning STAGE_3_NET_CARRYING_FIRST_RUN.';

-- ---------------------------------------------------------------
-- A-1. Default partition for rows outside explicit partitions
-- ---------------------------------------------------------------
CREATE TABLE ecl.calc_result_line_default
    PARTITION OF ecl.calc_result_line DEFAULT;

COMMENT ON TABLE ecl.calc_result_line_default IS
    'Default catch-all partition for ecl.calc_result_line. '
    'Rows here indicate a missing monthly partition — create via pg_partman maintenance job.';

-- ---------------------------------------------------------------
-- A-2. Initial monthly partitions (current + 3 forward months)
-- ---------------------------------------------------------------
CREATE TABLE ecl.calc_result_line_y2026m06
    PARTITION OF ecl.calc_result_line
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE ecl.calc_result_line_y2026m07
    PARTITION OF ecl.calc_result_line
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE ecl.calc_result_line_y2026m08
    PARTITION OF ecl.calc_result_line
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE ecl.calc_result_line_y2026m09
    PARTITION OF ecl.calc_result_line
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

-- ---------------------------------------------------------------
-- A-3. Indexes on ecl.calc_result_line
-- ---------------------------------------------------------------
-- FK-equivalent: calc_run_id (every FK/logical FK must be indexed per db-conventions.md)
CREATE INDEX idx_ecl_calc_result_line_calc_run_id
    ON ecl.calc_result_line (calc_run_id);

-- instrumen_id + evaluation_date DESC — most common lookup pattern for per-instrument history
CREATE INDEX idx_ecl_calc_result_line_instrumen_eval_date
    ON ecl.calc_result_line (instrumen_id, evaluation_date DESC);

-- Stage filter for portfolio summary queries — partial (active rows only)
CREATE INDEX idx_ecl_calc_result_line_stage
    ON ecl.calc_result_line (stage)
    WHERE deleted_at IS NULL;

-- Routing path — used by ECLOrchestrator to count FVTPL_SKIPPED / POCI / LPS splits
CREATE INDEX idx_ecl_calc_result_line_routing_path
    ON ecl.calc_result_line (routing_path)
    WHERE deleted_at IS NULL;

-- POCI flag — sparse, very selective; supports POCI deferred queue queries
CREATE INDEX idx_ecl_calc_result_line_flag_poci
    ON ecl.calc_result_line (flag_poci)
    WHERE flag_poci = TRUE AND deleted_at IS NULL;

-- Tenant + created_at DESC — mandatory for tenant queries per db-conventions.md
CREATE INDEX idx_ecl_calc_result_line_tenant_created
    ON ecl.calc_result_line (tenant_id, created_at DESC);

-- ---------------------------------------------------------------
-- A-4. Triggers on ecl.calc_result_line
-- ---------------------------------------------------------------
CREATE TRIGGER trg_ecl_calc_result_line_updated_at
    BEFORE UPDATE ON ecl.calc_result_line
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_ecl_calc_result_line_row_version
    BEFORE UPDATE ON ecl.calc_result_line
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Hard-delete guard (DEC-018: no hard delete on ecl.* namespace)
CREATE TRIGGER trg_ecl_calc_result_line_no_delete
    BEFORE DELETE ON ecl.calc_result_line
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();


-- ====================================================================
-- B. ecl.calc_header — precision fixes + new columns + audit cols + triggers
-- ====================================================================
-- Existing table from 000001 init_schema. Current column types use NUMERIC(20,2) for
-- IDR amounts and NUMERIC(8,4) for rates — both non-conforming per DEC-016.
-- Fix: IDR → NUMERIC(20,4), PD/LGD/rates → NUMERIC(10,8), bobot → NUMERIC(7,4).

-- B-1. Precision fixes (IDR amounts: NUMERIC(20,2) → NUMERIC(20,4))
ALTER TABLE ecl.calc_header
    ALTER COLUMN ead_native            TYPE NUMERIC(20,4),
    ALTER COLUMN ead_idr               TYPE NUMERIC(20,4),
    ALTER COLUMN ecl_weighted_idr      TYPE NUMERIC(20,4),
    ALTER COLUMN ecl_fl_idr            TYPE NUMERIC(20,4);

-- delta_ecl_fl_idr may be NULL
ALTER TABLE ecl.calc_header
    ALTER COLUMN delta_ecl_fl_idr      TYPE NUMERIC(20,4);

-- Rates: NUMERIC(8,4) → NUMERIC(10,8) per DEC-016
ALTER TABLE ecl.calc_header
    ALTER COLUMN lgd                   TYPE NUMERIC(10,8),
    ALTER COLUMN pd_normal             TYPE NUMERIC(10,8),
    ALTER COLUMN impact_mev_good       TYPE NUMERIC(10,8),
    ALTER COLUMN impact_mev_bad        TYPE NUMERIC(10,8),
    ALTER COLUMN impact_pd             TYPE NUMERIC(10,8);

-- Bobot: NUMERIC(8,4) → NUMERIC(7,4) per DEC-016
ALTER TABLE ecl.calc_header
    ALTER COLUMN w_good                TYPE NUMERIC(7,4),
    ALTER COLUMN w_normal              TYPE NUMERIC(7,4),
    ALTER COLUMN w_bad                 TYPE NUMERIC(7,4);

-- FX rate precision
ALTER TABLE ecl.calc_header
    ALTER COLUMN kurs_tengah_bi        TYPE NUMERIC(20,8);

-- B-2. New columns for M7
ALTER TABLE ecl.calc_header
    ADD COLUMN IF NOT EXISTS routing_path       TEXT
                                CONSTRAINT chk_ecl_calc_header_routing_path
                                CHECK (routing_path IS NULL OR routing_path IN (
                                    'STANDARD', 'LPS', 'LOOKTHROUGH',
                                    'SKIP_FVTPL', 'POCI_DEFERRED'
                                )),
    ADD COLUMN IF NOT EXISTS flag_poci          BOOLEAN         NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS pd_used_good       NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS pd_used_bad        NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS lgd_used           NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS net_carrying_idr   NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS prior_sealed_ecl_idr NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS sealed_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS catatan            TEXT,
    ADD COLUMN IF NOT EXISTS warnings_json      JSONB           NOT NULL DEFAULT '[]';

-- B-3. OQ-M7-2: add calc_run_job_id (TEXT → sys.job(id)) alongside existing UUID FK
-- The existing calc_run_id UUID FK → sys.job_run_history(id) is preserved for backward compat.
-- M8 will backfill calc_run_job_id for all existing rows and deprecate calc_run_id.
ALTER TABLE ecl.calc_header
    ADD COLUMN IF NOT EXISTS calc_run_job_id    TEXT REFERENCES sys.job(id) ON DELETE RESTRICT;

COMMENT ON COLUMN ecl.calc_header.calc_run_job_id IS
    'FK → sys.job(id) (TEXT ULID). Added in 000029 per OQ-M7-2 resolution. '
    'Coexists with calc_run_id (UUID FK → sys.job_run_history) for backward compat. '
    'M8 will backfill this column for existing rows then deprecate calc_run_id via ALTER DROP.';

COMMENT ON COLUMN ecl.calc_header.calc_run_id IS
    'LEGACY FK → sys.job_run_history(id). From init_schema 000001. '
    'Superseded by calc_run_job_id (FK → sys.job). '
    'Will be dropped in M8 after backfill. Do not use for new code.';

-- B-4. Status column — extend existing VARCHAR(20) to support M7 state machine
-- Existing default was 'POSTED'; M7 introduces DRAFT|IN_PROGRESS|COMPLETED|COMPLETED_WITH_ERRORS|SEALED|CANCELLED
-- Drop old default first, retype, then add new CHECK + default
ALTER TABLE ecl.calc_header
    ALTER COLUMN status SET DEFAULT 'DRAFT';

ALTER TABLE ecl.calc_header
    DROP CONSTRAINT IF EXISTS ck_ecl_calc_header_status;

ALTER TABLE ecl.calc_header
    ADD CONSTRAINT ck_ecl_calc_header_status
        CHECK (status IN (
            'DRAFT', 'IN_PROGRESS', 'COMPLETED',
            'COMPLETED_WITH_ERRORS', 'SEALED', 'CANCELLED',
            'POSTED'   -- preserve legacy value from init_schema rows
        ));

-- B-5. Audit columns — add if missing (init_schema 000001 only had created_at)
ALTER TABLE ecl.calc_header
    ADD COLUMN IF NOT EXISTS created_by         UUID,
    ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ     NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by         UUID,
    ADD COLUMN IF NOT EXISTS deleted_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by         UUID,
    ADD COLUMN IF NOT EXISTS row_version        BIGINT          NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id          TEXT            NOT NULL DEFAULT 'TUGURE';

COMMENT ON TABLE ecl.calc_header IS
    'Per-instrument-per-run ECL computation header. Legacy table from 000001. '
    'M7 adds routing_path, flag_poci, pd_used_good/bad, lgd_used, net_carrying_idr, '
    'prior_sealed_ecl_idr, sealed_at, warnings_json, calc_run_job_id (sys.job FK), '
    'full audit cols, and the fn_ecl_calc_no_modify_when_sealed trigger. '
    'No hard delete — DEC-018: guard via trg_ecl_calc_header_no_delete. '
    'OQ-M7-2: calc_run_id (UUID FK → sys.job_run_history) deprecated; '
    'use calc_run_job_id (TEXT FK → sys.job) for new code.';

-- B-6. Triggers for calc_header
CREATE TRIGGER trg_ecl_calc_header_updated_at
    BEFORE UPDATE ON ecl.calc_header
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_ecl_calc_header_row_version
    BEFORE UPDATE ON ecl.calc_header
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Sealed guard — block any UPDATE when sealed_at IS NOT NULL
-- (M7 scope: DRAFT→IN_PROGRESS→COMPLETED only; SEALED is M8 scope but trigger installed here)
CREATE OR REPLACE FUNCTION fn_ecl_calc_no_modify_when_sealed()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.sealed_at IS NOT NULL THEN
        RAISE EXCEPTION
            'ECL_CALC_RUN_SEALED: ecl.calc_header id=% was sealed at %. '
            'No modifications are permitted on a sealed calc run. '
            'Error code: ECL_CALC_RUN_SEALED (HTTP 423).',
            OLD.id, OLD.sealed_at
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION fn_ecl_calc_no_modify_when_sealed() IS
    'BEFORE UPDATE trigger guard for ecl.calc_header. '
    'Raises ECL_CALC_RUN_SEALED when sealed_at IS NOT NULL. '
    'Installed on ecl.calc_header in migration 000029. '
    'Referenced by ecl-eir-engineer for HTTP 423 error mapping. '
    'DEC-018: audit-grade immutability after M8 seal action.';

CREATE TRIGGER trg_ecl_calc_header_no_modify_when_sealed
    BEFORE UPDATE ON ecl.calc_header
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_calc_no_modify_when_sealed();

-- Hard-delete guard (DEC-018: no hard delete on ecl.*)
CREATE TRIGGER trg_ecl_calc_header_no_delete
    BEFORE DELETE ON ecl.calc_header
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

-- B-7. Index for new FK calc_run_job_id
CREATE INDEX idx_ecl_calc_header_calc_run_job_id
    ON ecl.calc_header (calc_run_job_id)
    WHERE calc_run_job_id IS NOT NULL;

-- Index for sealed_at (M8 sealing queries + Stage 3 net-carrying lookup)
CREATE INDEX idx_ecl_calc_header_sealed_at
    ON ecl.calc_header (sealed_at DESC)
    WHERE sealed_at IS NOT NULL;

-- Tenant + created_at for tenant hot queries
CREATE INDEX idx_ecl_calc_header_tenant_created
    ON ecl.calc_header (tenant_id, created_at DESC);


-- ====================================================================
-- C. ecl.calc_detail_skenario — precision fixes + new columns + audit cols + triggers
-- ====================================================================
-- Existing table from 000001 init_schema.
-- Columns: id, ecl_calc_header_id, skenario, pd_skenario NUMERIC(8,4),
--          bobot NUMERIC(8,4), ecl_skenario_idr NUMERIC(20,2).

-- C-1. Precision fixes
ALTER TABLE ecl.calc_detail_skenario
    ALTER COLUMN pd_skenario            TYPE NUMERIC(10,8),
    ALTER COLUMN bobot                  TYPE NUMERIC(10,8),
    ALTER COLUMN ecl_skenario_idr       TYPE NUMERIC(20,4);

-- C-2. New columns for M7
ALTER TABLE ecl.calc_detail_skenario
    ADD COLUMN IF NOT EXISTS fl_multiplier      NUMERIC(10,8),
    ADD COLUMN IF NOT EXISTS ecl_fl_idr         NUMERIC(20,4),
    ADD COLUMN IF NOT EXISTS ead_skenario_idr   NUMERIC(20,4);

COMMENT ON COLUMN ecl.calc_detail_skenario.fl_multiplier IS
    'Forward-looking (MEV) impact multiplier for this scenario. '
    'NULL for Stage 3 rows (FL not applied per PSAK 71 §5.5.17).';

COMMENT ON COLUMN ecl.calc_detail_skenario.ecl_fl_idr IS
    'ECL after applying FL multiplier: ecl_skenario_idr × fl_multiplier. '
    'NULL for Stage 3 (ecl_fl_idr = ecl_skenario_idr when FL is skipped — '
    'set equal in application, NULL in DB means not-yet-computed or Stage 3 skip).';

COMMENT ON COLUMN ecl.calc_detail_skenario.ead_skenario_idr IS
    'EAD used for this specific scenario row (may differ per scenario for LPS routing). '
    'Usually same as calc_header.ead_idr.';

-- C-3. Audit columns — add if missing
ALTER TABLE ecl.calc_detail_skenario
    ADD COLUMN IF NOT EXISTS created_at         TIMESTAMPTZ     NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS created_by         UUID,
    ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ     NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_by         UUID,
    ADD COLUMN IF NOT EXISTS deleted_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by         UUID,
    ADD COLUMN IF NOT EXISTS row_version        BIGINT          NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id          TEXT            NOT NULL DEFAULT 'TUGURE';

COMMENT ON TABLE ecl.calc_detail_skenario IS
    'Per-scenario breakdown child table for ecl.calc_header (legacy from 000001). '
    'M7 adds fl_multiplier, ecl_fl_idr, ead_skenario_idr, full audit cols, '
    'updated_at + row_version triggers, and hard-delete guard. '
    'OQ-M7-1: ecl.calc_result_line is the PRIMARY M7 result table (consolidated, flat). '
    'ecl.calc_detail_skenario is retained for backward compat and legacy queries only.';

-- C-4. Triggers for calc_detail_skenario
CREATE TRIGGER trg_ecl_calc_detail_updated_at
    BEFORE UPDATE ON ecl.calc_detail_skenario
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER trg_ecl_calc_detail_row_version
    BEFORE UPDATE ON ecl.calc_detail_skenario
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- Hard-delete guard (DEC-018)
CREATE TRIGGER trg_ecl_calc_detail_no_delete
    BEFORE DELETE ON ecl.calc_detail_skenario
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

-- C-5. Index on new audit column
CREATE INDEX idx_ecl_calc_detail_tenant_created
    ON ecl.calc_detail_skenario (tenant_id, created_at DESC);

COMMIT;
