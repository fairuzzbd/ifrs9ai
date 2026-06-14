-- migration: 0028 amendment_detection_idempotency — ROLLBACK
-- Drops the partial unique index added by 000028 up migration.

BEGIN;

DROP INDEX IF EXISTS ecl.uq_eir_reestimation_active_doc_instrumen;

COMMIT;
