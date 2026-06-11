-- migration: 0025 lookthrough_underlying_audit_not_null — DOWN
-- author: data-modeler / ecl-eir-engineer
-- description: Revert NOT NULL constraints added in 000025 up migration.
--   Drops NOT NULL from created_by, updated_by, created_at, updated_at,
--   row_version, tenant_id on ecl.lookthrough_underlying.
--   DATA PRESERVATION: backfilled sentinel rows (created_by / updated_by =
--   '00000000-0000-0000-0000-000000000000') are NOT deleted. After rolling back,
--   those columns simply become nullable again — existing data is untouched.

BEGIN;

-- Revert NOT NULL constraints (restore to nullable state as after migration 000024).
-- Drop NOT NULL only — DEFAULTs are left in place (harmless and DB-convention-compliant).

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN created_at   DROP NOT NULL;

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN created_by   DROP NOT NULL;

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN updated_at   DROP NOT NULL;

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN updated_by   DROP NOT NULL;

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN row_version  DROP NOT NULL;

ALTER TABLE ecl.lookthrough_underlying
    ALTER COLUMN tenant_id    DROP NOT NULL;

COMMIT;
