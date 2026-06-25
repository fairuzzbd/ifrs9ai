-- migration: 000047 POCI Delta ECL (P5-M10) — DOWN (reversal)
-- author: ecl-eir-engineer
-- WARNING: reversal drops ecl.poci_baseline and ecl.poci_delta_log.
-- Only run in dev/test. NEVER on production without explicit ALCO + CFO approval.

BEGIN;

-- Remove mapping_jurnal seed rows
DELETE FROM mst.mapping_jurnal
WHERE event_code IN ('POCI_ECL_DELTA_INCREASE', 'POCI_ECL_DELTA_DECREASE')
  AND workflow_status = 'DRAFT';

-- Drop row version trigger and function
DROP TRIGGER IF EXISTS trg_poci_delta_log_row_version ON ecl.poci_delta_log;
DROP FUNCTION IF EXISTS ecl.trg_poci_delta_log_row_version();

-- Drop WORM trigger and function
DROP TRIGGER IF EXISTS trg_poci_baseline_no_update_delete ON ecl.poci_baseline;
DROP FUNCTION IF EXISTS ecl.trg_poci_baseline_immutable();

-- Drop partitioned table (drops all partitions + indexes automatically)
DROP TABLE IF EXISTS ecl.poci_delta_log CASCADE;

-- Drop baseline table (WORM — trigger already dropped above)
DROP TABLE IF EXISTS ecl.poci_baseline CASCADE;

COMMIT;
