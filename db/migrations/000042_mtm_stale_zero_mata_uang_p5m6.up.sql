-- migration: 0042 mtm_stale_zero_mata_uang_p5m6
-- author: backend-engineer-go (P5-M6 compliance+security review fix)
-- requires: 0040 (mtm_p5m6 — trx.mtm table)
-- description:
--   B1: Relax chk_mtm_harga_pasar_idr_positive + chk_mtm_harga_buku_idr_positive to allow
--       zero values for STALE_PRICE rows (harga not available → market price = 0).
--   B2: Add mata_uang column to trx.mtm so OverrideApprove can route FCY jurnal correctly.

BEGIN;

-- ====================================================================
-- B1. Relax positive-price constraints to allow zero for STALE_PRICE rows
-- ====================================================================

-- harga_pasar_idr: zero is valid when status='STALE_PRICE' (no feed price available)
ALTER TABLE trx.mtm DROP CONSTRAINT IF EXISTS chk_mtm_harga_pasar_idr_positive;
ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_harga_pasar_idr_positive
        CHECK (status = 'STALE_PRICE' OR harga_pasar_idr > 0);

COMMENT ON CONSTRAINT chk_mtm_harga_pasar_idr_positive ON trx.mtm IS
    'Market price IDR must be > 0 except for STALE_PRICE rows (no feed available → stored as 0). '
    'P5-M6 compliance fix migration 000042.';

-- harga_buku_idr: same relaxation — when market price = 0, book value comparison is moot
ALTER TABLE trx.mtm DROP CONSTRAINT IF EXISTS chk_mtm_harga_buku_idr_positive;
ALTER TABLE trx.mtm
    ADD CONSTRAINT chk_mtm_harga_buku_idr_positive
        CHECK (status = 'STALE_PRICE' OR harga_buku_idr > 0);

COMMENT ON CONSTRAINT chk_mtm_harga_buku_idr_positive ON trx.mtm IS
    'Book value IDR must be > 0 except for STALE_PRICE rows (zero delta is moot when price unknown). '
    'P5-M6 compliance fix migration 000042.';

-- ====================================================================
-- B2. Add mata_uang column to trx.mtm
-- ====================================================================

ALTER TABLE trx.mtm
    ADD COLUMN IF NOT EXISTS mata_uang VARCHAR(10) NOT NULL DEFAULT 'IDR';

COMMENT ON COLUMN trx.mtm.mata_uang IS
    'ISO 4217 currency code of the instrument at MTM time. '
    'Snapshot from mst.instrumen.mata_uang. Required by OverrideApprove to route FCY '
    'jurnal event codes correctly (B2 fix). Default IDR for backward compat. '
    'P5-M6 compliance fix migration 000042.';

COMMIT;
