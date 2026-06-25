-- migration: 0040 mtm_p5m6 — ROLLBACK
-- author: data-modeler (driven by system-analyst P5-M6)
-- description: Reverse all changes from 000040_mtm_p5m6.up.sql.
--   Order: (F) mapping_jurnal seed → (E) config seeds → (C/B) triggers/functions
--          → (A) trx.mtm partitions + parent table.
-- WARNING: Dropping trx.mtm will permanently DELETE all MTM data.
--          Run only in dev/test environments. Production rollback requires data backup plan.

BEGIN;

-- ====================================================================
-- F. Remove mst.mapping_jurnal MTM event code seed rows.
--    Only removes DRAFT rows with these codes; leaves any approved/reviewed rows.
-- ====================================================================

DO $$
DECLARE
    v_table_exists BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = 'mst'
          AND table_name   = 'mapping_jurnal'
    ) INTO v_table_exists;

    IF v_table_exists THEN
        DELETE FROM mst.mapping_jurnal
        WHERE event_code IN (
            'MTM_FVOCI',
            'MTM_FX_OCI_RESERVE',
            'MTM_FVOCI_ELECTION',
            'MTM_FVTPL',
            'MTM_FVTPL_POCI'
        )
        AND status = 'DRAFT';

        RAISE NOTICE 'mst.mapping_jurnal: MTM DRAFT event code rows removed (if existed).';
    END IF;
END $$;

-- ====================================================================
-- E. Remove sys.config MTM seeds.
-- ====================================================================

DELETE FROM sys.config
WHERE config_key IN (
    'MTM_PRICE_DEVIATION_THRESHOLD_PCT',
    'MTM_PRICE_STALE_DAYS',
    'MTM_STALE_ESCALATION_DAYS',
    'MTM_CRON_SCHEDULE'
);

-- ====================================================================
-- C + B. Remove triggers and functions (applied at parent table level).
--         Dropping parent table cascades to all partitions, but drop
--         triggers explicitly first for clarity.
-- ====================================================================

DROP TRIGGER IF EXISTS tg_mtm_locked_check ON trx.mtm;
DROP TRIGGER IF EXISTS trg_mtm_updated_at  ON trx.mtm;
DROP TRIGGER IF EXISTS trg_mtm_row_version ON trx.mtm;

DROP FUNCTION IF EXISTS fn_mtm_locked_check();

-- ====================================================================
-- A. Drop trx.mtm and all partitions (CASCADE drops child tables).
--    WARNING: All MTM data will be permanently deleted.
-- ====================================================================

DROP TABLE IF EXISTS trx.mtm CASCADE;
-- ^ CASCADE drops:
--     trx.mtm_y2026m01 ... trx.mtm_y2026m12, trx.mtm_y2027m01, trx.mtm_default
--     All indexes defined on trx.mtm (including cross-partition ones).
--     All FK constraints referencing trx.mtm from other tables (if any).

COMMIT;
