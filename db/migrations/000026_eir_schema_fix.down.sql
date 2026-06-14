-- migration: 0026 eir_schema_fix — DOWN
-- author: data-modeler
-- requires: 0026 up applied
-- description: Reverses all changes from 000026_eir_schema_fix.up.sql.
--
-- WARNING — PRECISION DOWNGRADE:
--   Reverting NUMERIC(10,8) → NUMERIC(12,8) and NUMERIC(20,4) → NUMERIC(20,2) is LOSSY
--   for any row that was inserted with higher-precision values after 000026.up ran.
--   PostgreSQL will TRUNCATE (not round) extra decimal places during the TYPE cast.
--   This down migration is safe ONLY if applied before any new data is written at the
--   higher precision. Confirm with the DBA before running in any environment with live data.
--
-- Reversal order (reverse of up):
--   D → C → B → A

BEGIN;

-- ============================================================
-- D. mst.instrumen — revert
-- ============================================================

ALTER TABLE mst.instrumen
    DROP COLUMN IF EXISTS flag_poci;

ALTER TABLE mst.instrumen
    DROP CONSTRAINT IF EXISTS ck_eir_range;

ALTER TABLE mst.instrumen
    ALTER COLUMN eir_awal TYPE NUMERIC(12,8);

ALTER TABLE mst.instrumen
    ADD CONSTRAINT ck_eir_range
        CHECK (eir_awal IS NULL OR (eir_awal >= 0 AND eir_awal < 1));


-- ============================================================
-- C. trx.amortisasi — revert
-- ============================================================

DROP TRIGGER IF EXISTS trg_amortisasi_row_version ON trx.amortisasi;
DROP TRIGGER IF EXISTS trg_amortisasi_updated_at  ON trx.amortisasi;

DROP INDEX IF EXISTS idx_amortisasi_deleted;
DROP INDEX IF EXISTS idx_amortisasi_tenant_created;

ALTER TABLE trx.amortisasi
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- WARNING: reverting NUMERIC(20,4) → NUMERIC(20,2) — see precision note above
ALTER TABLE trx.amortisasi
    ALTER COLUMN amortisasi_premium_diskonto_idr TYPE NUMERIC(20,2);


-- ============================================================
-- B. ecl.eir_reestimation_log — revert
-- ============================================================

DROP TRIGGER IF EXISTS trg_eir_log_row_version ON ecl.eir_reestimation_log;
DROP TRIGGER IF EXISTS trg_eir_log_updated_at  ON ecl.eir_reestimation_log;

DROP INDEX IF EXISTS idx_eir_log_tenant_created;
DROP INDEX IF EXISTS idx_eir_log_workflow_status_partial;

ALTER TABLE ecl.eir_reestimation_log
    DROP CONSTRAINT IF EXISTS chk_eir_log_sod,
    DROP CONSTRAINT IF EXISTS chk_eir_log_workflow_status;

ALTER TABLE ecl.eir_reestimation_log
    DROP COLUMN IF EXISTS approver_signature_hash,
    DROP COLUMN IF EXISTS reviewer_signature_hash,
    DROP COLUMN IF EXISTS approver_comment,
    DROP COLUMN IF EXISTS reviewer_comment,
    DROP COLUMN IF EXISTS reject_reason,
    DROP COLUMN IF EXISTS rejected_at;

ALTER TABLE ecl.eir_reestimation_log
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- WARNING: reverting NUMERIC precision — see note above
ALTER TABLE ecl.eir_reestimation_log
    ALTER COLUMN eir_sebelum         TYPE NUMERIC(12,8),
    ALTER COLUMN eir_sesudah         TYPE NUMERIC(12,8),
    ALTER COLUMN carrying_sebelum    TYPE NUMERIC(20,2),
    ALTER COLUMN carrying_sesudah    TYPE NUMERIC(20,2),
    ALTER COLUMN catch_up_adjustment TYPE NUMERIC(20,2);


-- ============================================================
-- A. ecl.eir_amortization_schedule — revert
-- ============================================================

DROP TRIGGER IF EXISTS trg_eir_schedule_row_version        ON ecl.eir_amortization_schedule;
DROP TRIGGER IF EXISTS trg_eir_schedule_updated_at         ON ecl.eir_amortization_schedule;
DROP TRIGGER IF EXISTS tg_eir_schedule_amounts_immutable   ON ecl.eir_amortization_schedule;

DROP FUNCTION IF EXISTS fn_eir_schedule_amounts_immutable();

DROP INDEX IF EXISTS idx_eir_schedule_tenant_created;
DROP INDEX IF EXISTS idx_eir_schedule_active_instrumen;
DROP INDEX IF EXISTS idx_eir_schedule_instrumen_tanggal;

ALTER TABLE ecl.eir_amortization_schedule
    DROP COLUMN IF EXISTS flag_poci;

ALTER TABLE ecl.eir_amortization_schedule
    DROP COLUMN IF EXISTS tenant_id,
    DROP COLUMN IF EXISTS row_version,
    DROP COLUMN IF EXISTS deleted_by,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_by;

-- WARNING: reverting NUMERIC precision — see note above
ALTER TABLE ecl.eir_amortization_schedule
    ALTER COLUMN eir_periode              TYPE NUMERIC(12,8),
    ALTER COLUMN opening_carrying         TYPE NUMERIC(20,2),
    ALTER COLUMN cash_inflow              TYPE NUMERIC(20,2),
    ALTER COLUMN pendapatan_bunga_eir     TYPE NUMERIC(20,2),
    ALTER COLUMN amortisasi_p_d           TYPE NUMERIC(20,2),
    ALTER COLUMN pelunasan_pokok          TYPE NUMERIC(20,2),
    ALTER COLUMN closing_carrying         TYPE NUMERIC(20,2);

COMMIT;
