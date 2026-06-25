-- migration: 0046 akrual_parent_id_dividen_partitions (DOWN)
-- author: backend-engineer-go
-- Reversal: drop parent_akrual_id, drop Aug–Nov 2026 dividen partitions.
-- WARNING: dropping partitions removes all data in those ranges.

BEGIN;

-- Drop dividen partitions
DROP TABLE IF EXISTS trx.dividen_y2026m11;
DROP TABLE IF EXISTS trx.dividen_y2026m10;
DROP TABLE IF EXISTS trx.dividen_y2026m09;
DROP TABLE IF EXISTS trx.dividen_y2026m08;

-- Drop parent_akrual_id column
DROP INDEX IF EXISTS idx_pendapatan_akrual_parent_id;
ALTER TABLE trx.pendapatan_akrual DROP COLUMN IF EXISTS parent_akrual_id;

COMMIT;
