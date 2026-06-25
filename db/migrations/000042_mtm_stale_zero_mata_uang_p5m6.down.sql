-- migration: 0042 mtm_stale_zero_mata_uang_p5m6 — DOWN (rollback)
-- author: backend-engineer-go (P5-M6 compliance+security review fix)
-- description: Revert B1 constraint relaxation + drop B2 mata_uang column.

BEGIN;

-- ====================================================================
-- B2. Drop mata_uang column
-- ====================================================================

ALTER TABLE trx.mtm DROP COLUMN IF EXISTS mata_uang;

-- ====================================================================
-- B1. Restore strict positive-only constraints (original from 000040)
-- ====================================================================

ALTER TABLE trx.mtm DROP CONSTRAINT IF EXISTS chk_mtm_harga_pasar_idr_positive;
ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_harga_pasar_idr_positive
        CHECK (harga_pasar_idr > 0);

ALTER TABLE trx.mtm DROP CONSTRAINT IF EXISTS chk_mtm_harga_buku_idr_positive;
ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_harga_buku_idr_positive
        CHECK (harga_buku_idr > 0);

COMMIT;
