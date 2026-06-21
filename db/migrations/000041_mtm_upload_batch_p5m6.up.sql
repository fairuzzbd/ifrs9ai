-- migration: 0041 mtm_upload_batch_p5m6
-- author: data-modeler (driven by system-analyst P5-M6)
-- requires: 0001 (init_schema — sys.upload_batch + sys.upload_batch_row already exist),
--           0040 (mtm_p5m6 — trx.mtm exists + FK to sys.upload_batch already added)
-- description:
--   P5-M6 MTM — sys.upload_batch alignment:
--   (A) CONFLICT RESOLUTION: migration 000001 already defines sys.upload_batch with
--       ck_batch_type CHECK including 'MTM_UPLOAD'. This migration VERIFIES that
--       'MTM_UPLOAD' is present in the existing constraint (as expected).
--       Stories doc mentioned 'MTM' as batch_type but existing schema uses 'MTM_UPLOAD'.
--       Decision: use 'MTM_UPLOAD' (consistent with existing schema, no ALTER needed).
--       If the existing constraint is MISSING 'MTM_UPLOAD' (older schema variant),
--       this migration performs the necessary ADD.
--   (B) ADD precision upgrade to sys.upload_batch_row for MTM-specific fields:
--       harga_native and harga_sebelumnya: NUMERIC(15,4) → NUMERIC(20,8) per DEC-016.
--   (C) ADD index on sys.upload_batch for batch_type='MTM_UPLOAD' queries.
--   (D) ADD sys.upload_batch.periode_bulanan_id column if not exists —
--       MTM batches must reference a specific periode to validate tanggal_mtm range.

BEGIN;

-- ====================================================================
-- A. Verify and align sys.upload_batch ck_batch_type constraint.
--    migration 000001 defines:
--      CONSTRAINT ck_batch_type CHECK (batch_type IN (
--        'MTM_UPLOAD','INSTRUMEN_BULK','IMPACT_MEV','PD_PEFINDO','FUND_FACT_SHEET'))
--    This migration ensures 'MTM_UPLOAD' is present (idempotent check).
--    If the existing CHECK is absent or mismatched, it is replaced.
-- ====================================================================

DO $$
DECLARE
    v_constraint_exists     BOOLEAN;
    v_mtm_upload_covered    BOOLEAN;
BEGIN
    -- Check if ck_batch_type constraint exists on sys.upload_batch
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.table_constraints tc
        WHERE tc.table_schema = 'sys'
          AND tc.table_name   = 'upload_batch'
          AND tc.constraint_name = 'ck_batch_type'
          AND tc.constraint_type = 'CHECK'
    ) INTO v_constraint_exists;

    IF NOT v_constraint_exists THEN
        -- No constraint: add it with all known batch types including MTM_UPLOAD.
        RAISE NOTICE 'ck_batch_type not found on sys.upload_batch — adding constraint.';

        EXECUTE '
            ALTER TABLE sys.upload_batch
            ADD CONSTRAINT ck_batch_type
            CHECK (batch_type IN (
                ''MTM_UPLOAD'', ''INSTRUMEN_BULK'', ''IMPACT_MEV'',
                ''PD_PEFINDO'', ''FUND_FACT_SHEET'',
                ''FX_RATE'', ''KURS''
            ))';

    ELSE
        -- Constraint exists: verify 'MTM_UPLOAD' is covered by trying an INSERT validation.
        -- Use pg_get_constraintdef to inspect the existing constraint definition.
        SELECT (pg_get_constraintdef(c.oid) ILIKE '%MTM_UPLOAD%')
        FROM pg_constraint c
        JOIN pg_class     cl ON cl.oid = c.conrelid
        JOIN pg_namespace ns ON ns.oid = cl.relnamespace
        WHERE ns.nspname   = 'sys'
          AND cl.relname   = 'upload_batch'
          AND c.conname    = 'ck_batch_type'
        INTO v_mtm_upload_covered;

        IF NOT v_mtm_upload_covered THEN
            -- MTM_UPLOAD is not in existing constraint — extend it.
            RAISE NOTICE 'ck_batch_type exists but MTM_UPLOAD not covered — extending constraint.';

            ALTER TABLE sys.upload_batch
                DROP CONSTRAINT ck_batch_type;

            ALTER TABLE sys.upload_batch
                ADD CONSTRAINT ck_batch_type
                CHECK (batch_type IN (
                    'MTM_UPLOAD', 'INSTRUMEN_BULK', 'IMPACT_MEV',
                    'PD_PEFINDO', 'FUND_FACT_SHEET',
                    'FX_RATE', 'KURS'
                ));

        ELSE
            RAISE NOTICE 'ck_batch_type already covers MTM_UPLOAD — no change needed.';
        END IF;
    END IF;
END $$;

COMMENT ON TABLE sys.upload_batch IS
    'Generic upload batch tracking table. Shared by: '
    'MTM manual upload (batch_type=''MTM_UPLOAD''), '
    'Instrumen bulk (''INSTRUMEN_BULK''), '
    'ECL parameter uploads (''IMPACT_MEV'', ''PD_PEFINDO''), '
    'Reksadana fund fact sheet (''FUND_FACT_SHEET''). '
    'batch_type=''MTM_UPLOAD'' added/verified in P5-M6 migration 000041. '
    'Referenced by trx.mtm.upload_batch_id (FK added in migration 000040).';

-- ====================================================================
-- B. Precision upgrade on sys.upload_batch_row:
--    harga_native, harga_sebelumnya: NUMERIC(15,4) → NUMERIC(20,8) per DEC-016.
--    MTM harga_pasar requires 8 decimal places for FCY prices (DEC-016).
-- ====================================================================

DO $$
DECLARE
    v_col_type TEXT;
BEGIN
    -- Check current type of harga_native
    SELECT data_type || COALESCE('(' || numeric_precision || ',' || numeric_scale || ')', '')
    FROM information_schema.columns
    WHERE table_schema = 'sys'
      AND table_name   = 'upload_batch_row'
      AND column_name  = 'harga_native'
    INTO v_col_type;

    IF v_col_type IS NULL THEN
        RAISE NOTICE 'sys.upload_batch_row.harga_native not found — may not exist yet. Skipping precision upgrade.';
    ELSIF v_col_type NOT ILIKE '%20,8%' THEN
        RAISE NOTICE 'Upgrading sys.upload_batch_row.harga_native from % to NUMERIC(20,8).', v_col_type;

        ALTER TABLE sys.upload_batch_row
            ALTER COLUMN harga_native       TYPE NUMERIC(20,8) USING harga_native::NUMERIC(20,8),
            ALTER COLUMN harga_sebelumnya   TYPE NUMERIC(20,8) USING harga_sebelumnya::NUMERIC(20,8);

        COMMENT ON COLUMN sys.upload_batch_row.harga_native IS
            'Native price from upload file. NUMERIC(20,8) per DEC-016 (P5-M6 upgrade from 15,4). '
            'Stores FCY price (e.g. bond price 98.50000000) or IDR price.';

        COMMENT ON COLUMN sys.upload_batch_row.harga_sebelumnya IS
            'Previous price for deviation calculation. NUMERIC(20,8) per DEC-016 (P5-M6 upgrade). '
            'Sourced from prior day AUTO_POSTED or APPROVED row.';

    ELSE
        RAISE NOTICE 'sys.upload_batch_row.harga_native already NUMERIC(20,8) — no change needed.';
    END IF;
END $$;

-- ====================================================================
-- C. Add index on sys.upload_batch for MTM batch queries.
--    Covers: GET /trx/mtm/upload/batch/{id} and P5-M6 batch status queries.
-- ====================================================================

CREATE INDEX IF NOT EXISTS idx_upload_batch_mtm_type
    ON sys.upload_batch (batch_type, status, uploaded_at DESC)
    WHERE batch_type = 'MTM_UPLOAD';

COMMENT ON INDEX idx_upload_batch_mtm_type IS
    'P5-M6: queries for MTM upload batches by status. '
    'Covers ROLE-AKUN-CTL antrian: batch_type=MTM_UPLOAD AND status=PENDING_REVIEW.';

-- ====================================================================
-- D. ADD sys.upload_batch.periode_bulanan_id column (if not exists).
--    MTM batches need to reference a specific periode for tanggal_mtm range validation.
--    tanggal_valuasi already exists in upload_batch (migration 000001) but
--    periode_bulanan_id provides explicit link to mst.periode_buku.
-- ====================================================================

ALTER TABLE sys.upload_batch
    ADD COLUMN IF NOT EXISTS periode_bulanan_id UUID;
-- ^ FK to mst.periode_buku(id). Intentionally no FK constraint here:
--   the table may not have mst.periode_buku FK target defined yet.
--   Service layer validates periode_bulanan_id via business logic.
--   FK will be added in a later migration if mst.periode_buku is confirmed stable.

COMMENT ON COLUMN sys.upload_batch.periode_bulanan_id IS
    'Periode buku ID for this batch. Used by MTM upload validation to confirm '
    'tanggal_mtm is within an OPEN periode. NULL for non-MTM batch types. '
    'Added in P5-M6 migration 000041. '
    'FK to mst.periode_buku(id) deferred to later migration.';

-- Index for periode_bulanan_id lookups
CREATE INDEX IF NOT EXISTS idx_upload_batch_periode
    ON sys.upload_batch (periode_bulanan_id)
    WHERE periode_bulanan_id IS NOT NULL;

COMMENT ON INDEX idx_upload_batch_periode IS
    'Lookup: sys.upload_batch by periode_bulanan_id (MTM upload + future modules).';

COMMIT;
