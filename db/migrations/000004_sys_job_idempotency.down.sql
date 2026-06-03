-- migration: 0004 sys_job_idempotency (DOWN)
-- author: data-modeler
-- description: Drop sys.job and sys.idempotency_key added in 0004.

BEGIN;

DROP TABLE IF EXISTS sys.idempotency_key;
DROP TABLE IF EXISTS sys.job;
DROP FUNCTION IF EXISTS fn_increment_row_version();

COMMIT;
