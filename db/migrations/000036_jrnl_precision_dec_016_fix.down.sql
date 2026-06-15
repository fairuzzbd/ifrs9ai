-- migration: 0036 jrnl_precision_dec_016_fix (DOWN)
-- author: ecl-eir-engineer
-- description:
--   Reverts NUMERIC(20,4) → NUMERIC(20,2) on the four jrnl columns altered in the UP migration.
--
--   DATA TRUNCATION WARNING: Any value with more than 2 fractional decimal digits stored
--   since 000036.up.sql was applied WILL BE TRUNCATED to 2 decimal places by the USING
--   cast below.  Reversion should only be performed in development or after confirming
--   no such fractional data exists (run: SELECT COUNT(*) FROM jrnl.detail
--   WHERE debit_amount <> debit_amount::NUMERIC(20,2) OR kredit_amount <> kredit_amount::NUMERIC(20,2);
--   and similarly for jrnl.header).
--
--   Ordering note: BEFORE UPDATE triggers installed by 0035 do NOT fire on DDL ALTER
--   COLUMN TYPE, so this reversion is safe regardless of trigger state.

BEGIN;

-- ────────────────────────────────────────────────────────────────────────────────
-- jrnl.header — revert to NUMERIC(20,2)
-- ────────────────────────────────────────────────────────────────────────────────

-- DATA TRUNCATION WARNING: USING cast below truncates any value with >2 decimal digits.
ALTER TABLE jrnl.header
    ALTER COLUMN total_debit  TYPE NUMERIC(20,2) USING total_debit::NUMERIC(20,2),
    ALTER COLUMN total_kredit TYPE NUMERIC(20,2) USING total_kredit::NUMERIC(20,2);

-- ────────────────────────────────────────────────────────────────────────────────
-- jrnl.detail — revert to NUMERIC(20,2)
-- ────────────────────────────────────────────────────────────────────────────────

-- DATA TRUNCATION WARNING: USING cast below truncates any value with >2 decimal digits.
ALTER TABLE jrnl.detail
    ALTER COLUMN debit_amount  TYPE NUMERIC(20,2) USING debit_amount::NUMERIC(20,2),
    ALTER COLUMN kredit_amount TYPE NUMERIC(20,2) USING kredit_amount::NUMERIC(20,2);

COMMIT;
