-- migration: 0046 akrual_parent_id_dividen_partitions
-- author: backend-engineer-go
-- requires: 0045 (trx.pendapatan_akrual, trx.dividen)
-- description:
--   (A) M5 fix — ADD parent_akrual_id to trx.pendapatan_akrual for immutable override trail.
--       OverrideStaleAkrual now marks original row SKIPPED and inserts a new POSTED row
--       linked via parent_akrual_id instead of mutating the original row.
--   (B) m6 fix — Add missing dividen partition tables for Aug–Nov 2026.

BEGIN;

-- ====================================================================
-- A. parent_akrual_id on trx.pendapatan_akrual
-- Self-referencing FK: new POSTED row → original SKIPPED row.
-- NULL for rows created by normal cron (no parent).
-- ====================================================================

ALTER TABLE trx.pendapatan_akrual
    ADD COLUMN IF NOT EXISTS parent_akrual_id UUID REFERENCES trx.pendapatan_akrual (id);

COMMENT ON COLUMN trx.pendapatan_akrual.parent_akrual_id IS
    'Links an override POSTED row to its original PENDING_STALE_REVIEW row '
    '(which is marked SKIPPED). NULL for cron-originated rows. '
    'Part of M5 immutability fix — audit trail preserved per DEC-018.';

CREATE INDEX IF NOT EXISTS idx_pendapatan_akrual_parent_id
    ON trx.pendapatan_akrual (parent_akrual_id)
    WHERE parent_akrual_id IS NOT NULL;

-- ====================================================================
-- B. Missing dividen partitions Aug–Nov 2026 (m6 fix)
-- ====================================================================

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m08
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m09
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m10
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS trx.dividen_y2026m11
    PARTITION OF trx.dividen
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

COMMIT;
