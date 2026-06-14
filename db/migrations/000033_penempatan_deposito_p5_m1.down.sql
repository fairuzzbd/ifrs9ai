-- migration: 0033 penempatan_deposito_p5_m1 DOWN
-- author: data-modeler
-- description: Reverse of 000033 up.
--   Drop tables, type, sequence in dependency order.
--   Safe to run only when no data exists; hard-delete guard removed first.

BEGIN;

-- ----------------------------------------------------------------
-- 1. Drop sys.settlement_account_balance (no downstream dependencies)
-- ----------------------------------------------------------------

DROP TRIGGER IF EXISTS tg_settlement_balance_row_version  ON sys.settlement_account_balance;
DROP TRIGGER IF EXISTS tg_settlement_balance_updated_at   ON sys.settlement_account_balance;
DROP TABLE   IF EXISTS sys.settlement_account_balance;

-- ----------------------------------------------------------------
-- 2. Drop trx.penempatan_deposito
--    Must drop hard-delete guard trigger first so DROP TABLE succeeds.
-- ----------------------------------------------------------------

DROP TRIGGER IF EXISTS tg_penempatan_no_hard_delete   ON trx.penempatan_deposito;
DROP TRIGGER IF EXISTS tg_penempatan_row_version       ON trx.penempatan_deposito;
DROP TRIGGER IF EXISTS tg_penempatan_updated_at        ON trx.penempatan_deposito;
DROP TABLE   IF EXISTS trx.penempatan_deposito;

-- Drop the hard-delete guard function (only used by the above trigger)
DROP FUNCTION IF EXISTS fn_penempatan_no_hard_delete();

-- ----------------------------------------------------------------
-- 3. Drop ENUM type
-- ----------------------------------------------------------------

DROP TYPE IF EXISTS trx.penempatan_workflow_status;

-- ----------------------------------------------------------------
-- 4. Drop sequence
-- ----------------------------------------------------------------

DROP SEQUENCE IF EXISTS trx.penempatan_kode_seq;

COMMIT;
