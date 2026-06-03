-- migration: 0003 pii_encrypt_functions (DOWN)
-- author: data-modeler
-- description: Drop PII encrypt/decrypt functions, role, and PII columns added in 0003.
--              WARNING: Dropping these columns is destructive if any PII data has been stored.
--              Ensure all data has been exported/migrated before running down in non-dev.

BEGIN;

-- Drop functions first (dependency order)
DROP FUNCTION IF EXISTS sec.decrypt(TEXT);
DROP FUNCTION IF EXISTS sec.encrypt(TEXT);
DROP FUNCTION IF EXISTS sec._get_pii_key();

-- Remove PII columns from mst.counterparty (added in 0003; npwp_encrypted was in 0001)
ALTER TABLE mst.counterparty
    DROP COLUMN IF EXISTS nomor_rekening_encrypted,
    DROP COLUMN IF EXISTS ktp_encrypted;

-- Remove PII columns from sec.user
ALTER TABLE sec.user
    DROP COLUMN IF EXISTS email_personal_encrypted,
    DROP COLUMN IF EXISTS phone_encrypted;

-- Remove config key
DELETE FROM sys.config WHERE config_key = 'PII_ENCRYPTION_KEY';

-- Drop PII accessor role (only if no members; fail loudly otherwise)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'blips_pii_accessor') THEN
        DROP ROLE blips_pii_accessor;
    END IF;
EXCEPTION WHEN dependent_objects_still_exist THEN
    RAISE NOTICE 'blips_pii_accessor role has active members — not dropped. Revoke memberships first.';
END;
$$;

COMMIT;
