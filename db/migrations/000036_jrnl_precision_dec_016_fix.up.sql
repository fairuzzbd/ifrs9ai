-- migration: 0036 jrnl_precision_dec_016_fix
-- author: ecl-eir-engineer
-- requires: 0001 (init_schema — jrnl.header, jrnl.detail baseline columns),
--           0035 (jurnal_engine_p5_m2 — append-only triggers now active on jrnl.*)
-- description:
--   F1 BLOCKER — DEC-016 compliance fix: promote NUMERIC(20,2) to NUMERIC(20,4) on
--   jrnl.header.total_debit, jrnl.header.total_kredit,
--   jrnl.detail.debit_amount, jrnl.detail.kredit_amount.
--
--   DEC-016 mandates NUMERIC(20,4) for all IDR amounts.  The 2-decimal precision in
--   migration 0001 was insufficient; this migration aligns the schema with the locked
--   decision without touching any Go application code (shopspring/decimal already
--   serialises to 4 decimal places).
--
--   Ordering note: migration 0035 installed BEFORE UPDATE / BEFORE DELETE triggers on
--   jrnl.header and jrnl.detail (fn_jrnl_header_no_update etc.).  ALTER COLUMN TYPE is
--   a DDL operation that does NOT fire row-level DML triggers, so no special ordering
--   with respect to those triggers is required.
--
--   Risk: no existing data truncation because NUMERIC(20,4) is a superset of NUMERIC(20,2).
--   Any previously stored value "123.45" becomes "123.4500" — identical numeric value.

BEGIN;

-- ────────────────────────────────────────────────────────────────────────────────
-- jrnl.header — total_debit, total_kredit
-- ────────────────────────────────────────────────────────────────────────────────

ALTER TABLE jrnl.header
    ALTER COLUMN total_debit  TYPE NUMERIC(20,4),
    ALTER COLUMN total_kredit TYPE NUMERIC(20,4);

COMMENT ON COLUMN jrnl.header.total_debit IS
    'Sum of all DEBIT detail lines for this journal entry. '
    'NUMERIC(20,4) per DEC-016. Mirror of balance invariant check in PostingService.';

COMMENT ON COLUMN jrnl.header.total_kredit IS
    'Sum of all KREDIT detail lines for this journal entry. '
    'NUMERIC(20,4) per DEC-016. Mirror of balance invariant check in PostingService.';

-- ────────────────────────────────────────────────────────────────────────────────
-- jrnl.detail — debit_amount, kredit_amount
-- ────────────────────────────────────────────────────────────────────────────────

ALTER TABLE jrnl.detail
    ALTER COLUMN debit_amount  TYPE NUMERIC(20,4),
    ALTER COLUMN kredit_amount TYPE NUMERIC(20,4);

COMMENT ON COLUMN jrnl.detail.debit_amount IS
    'Debit amount for this journal line. Exactly one of debit_amount / kredit_amount '
    'is non-zero per line (XOR invariant). NUMERIC(20,4) per DEC-016.';

COMMENT ON COLUMN jrnl.detail.kredit_amount IS
    'Kredit amount for this journal line. Exactly one of debit_amount / kredit_amount '
    'is non-zero per line (XOR invariant). NUMERIC(20,4) per DEC-016.';

COMMIT;
