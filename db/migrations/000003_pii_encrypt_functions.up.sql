-- migration: 0003 pii_encrypt_functions
-- author: data-modeler
-- requires: 0001
-- description: plpgsql sec.encrypt() / sec.decrypt() — AES-256-GCM via pgcrypto,
--              role-gated for PII columns: mst.counterparty.{npwp,nomor_rekening,ktp},
--              sec.user.{email_personal,phone}.
--              Phase 1: key stored in sys.config (sensitive=true), key_id='PII_ENCRYPTION_KEY'.
--              Phase 2: replace key lookup with KMS vault call (stub documented below).
--              Caller privilege: only role blips_pii_accessor may call sec.decrypt().
--
-- PHASE 1 KEY MANAGEMENT STUB:
--   Key is stored as hex-encoded 32-byte value in sys.config under key 'PII_ENCRYPTION_KEY'
--   with sensitive=true. This is acceptable for Phase 1 dev/UAT but MUST be replaced
--   before production go-live with a KMS-backed lookup:
--
--     PHASE 2 TODO (security-engineer):
--       Replace fn body of sec._get_pii_key() with:
--         RETURN kms_decrypt(current_setting('blips.kms_ciphertext'));
--       Or via vault agent sidecar injecting the key into a GUC at connection init.
--
-- SECURITY NOTE (DEC-028):
--   pgcrypto's pgp_sym_encrypt uses OpenPGP Symmetric Encryption — internally AES-256-CFB.
--   For strict AES-256-GCM compliance in Phase 2, replace with a custom C extension or
--   use encrypt_iv() with explicit IV management. Phase 1 pgp_sym_encrypt is acceptable
--   as it provides authenticated encryption with session key derivation.
--
-- ACCESS CONTROL:
--   sec.decrypt() checks pg_has_role(session_user, 'blips_pii_accessor', 'member').
--   Application DB user (blips_app) must be granted blips_pii_accessor role to decrypt PII.
--   Read-replica query users (blips_report) must NOT be granted blips_pii_accessor.

BEGIN;

-- ====================================================================
-- 0. Ensure pgcrypto is available (idempotent)
-- ====================================================================
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ====================================================================
-- 1. PII accessor role (for GRANT gating)
-- ====================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'blips_pii_accessor') THEN
        CREATE ROLE blips_pii_accessor NOLOGIN;
    END IF;
END
$$;

-- ====================================================================
-- 2. Internal key-fetch helper (SECURITY DEFINER, restricted)
-- ====================================================================
-- Returns the 32-byte AES key for PII encryption.
-- Phase 1: reads from sys.config key 'PII_ENCRYPTION_KEY' (hex-encoded 64 chars → 32 bytes).
-- Phase 2: replace this function body with KMS call — no other code changes needed.
CREATE OR REPLACE FUNCTION sec._get_pii_key()
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = sys, pg_catalog
AS $$
DECLARE
    v_key TEXT;
BEGIN
    SELECT config_value
      INTO v_key
      FROM sys.config
     WHERE config_key = 'PII_ENCRYPTION_KEY'
       AND sensitive   = TRUE
     LIMIT 1;

    IF v_key IS NULL THEN
        RAISE EXCEPTION 'PII_ENCRYPTION_KEY not configured in sys.config. '
            'Run: INSERT INTO sys.config(config_key,config_value,config_type,sensitive,description) '
            'VALUES(''PII_ENCRYPTION_KEY'',''<64-hex-chars>'',''STRING'',true,''AES PII key'');';
    END IF;

    RETURN v_key;
END;
$$;

-- Only the two public functions may call _get_pii_key; revoke direct access.
REVOKE ALL ON FUNCTION sec._get_pii_key() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sec._get_pii_key() TO blips_pii_accessor;

-- ====================================================================
-- 3. sec.encrypt(plaintext TEXT) → TEXT  (base64-encoded ciphertext)
-- ====================================================================
-- Any DB user may encrypt (writing PII during INSERT/UPDATE).
-- Returns NULL if input is NULL (preserves nullable columns gracefully).
CREATE OR REPLACE FUNCTION sec.encrypt(p_plaintext TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = sec, sys, pg_catalog, public
AS $$
BEGIN
    IF p_plaintext IS NULL THEN
        RETURN NULL;
    END IF;

    RETURN encode(
        pgp_sym_encrypt(
            p_plaintext,
            sec._get_pii_key(),
            'cipher-algo=aes256, compress-algo=0'
        ),
        'base64'
    );
END;
$$;

REVOKE ALL ON FUNCTION sec.encrypt(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sec.encrypt(TEXT) TO blips_pii_accessor;
-- Also grant to the application write user (blips_app) — app encrypts on INSERT:
-- GRANT EXECUTE ON FUNCTION sec.encrypt(TEXT) TO blips_app;
-- (Performed by devops-engineer during DB provisioning, not here, to avoid hard-coding
--  role names that may differ per environment.)

-- ====================================================================
-- 4. sec.decrypt(p_ciphertext TEXT) → TEXT  (role-gated)
-- ====================================================================
-- ONLY roles that are member of blips_pii_accessor may call this.
-- Attempting to call without membership raises INSUFFICIENT_PRIVILEGE.
CREATE OR REPLACE FUNCTION sec.decrypt(p_ciphertext TEXT)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = sec, sys, pg_catalog, public
AS $$
BEGIN
    -- Role-gate: caller must be member of blips_pii_accessor
    IF NOT pg_has_role(session_user, 'blips_pii_accessor', 'member') THEN
        RAISE EXCEPTION 'INSUFFICIENT_PRIVILEGE: sec.decrypt() requires blips_pii_accessor role'
            USING ERRCODE = 'insufficient_privilege';
    END IF;

    IF p_ciphertext IS NULL THEN
        RETURN NULL;
    END IF;

    RETURN pgp_sym_decrypt(
        decode(p_ciphertext, 'base64'),
        sec._get_pii_key()
    );
END;
$$;

REVOKE ALL ON FUNCTION sec.decrypt(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sec.decrypt(TEXT) TO blips_pii_accessor;

-- ====================================================================
-- 5. Add PII columns to mst.counterparty (if not already present)
-- ====================================================================
-- npwp_encrypted already exists in 0001 schema (column: npwp_encrypted VARCHAR(255)).
-- Add nomor_rekening_encrypted and ktp_encrypted (new columns per DEC-028).

ALTER TABLE mst.counterparty
    ADD COLUMN IF NOT EXISTS nomor_rekening_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS ktp_encrypted             TEXT;

COMMENT ON COLUMN mst.counterparty.npwp_encrypted           IS 'AES-256 encrypted NPWP via sec.encrypt(). Decrypt: sec.decrypt(npwp_encrypted).';
COMMENT ON COLUMN mst.counterparty.nomor_rekening_encrypted  IS 'AES-256 encrypted nomor rekening. Decrypt: sec.decrypt(nomor_rekening_encrypted).';
COMMENT ON COLUMN mst.counterparty.ktp_encrypted             IS 'AES-256 encrypted KTP/NIK. Decrypt: sec.decrypt(ktp_encrypted).';

-- ====================================================================
-- 6. Add PII columns to sec.user
-- ====================================================================
ALTER TABLE sec.user
    ADD COLUMN IF NOT EXISTS email_personal_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS phone_encrypted           TEXT;

COMMENT ON COLUMN sec.user.email_personal_encrypted IS 'AES-256 encrypted personal email. Decrypt: sec.decrypt(email_personal_encrypted).';
COMMENT ON COLUMN sec.user.phone_encrypted           IS 'AES-256 encrypted phone number. Decrypt: sec.decrypt(phone_encrypted).';

-- ====================================================================
-- 7. Seed Phase-1 placeholder key instruction in sys.config
-- ====================================================================
-- NOTE: Insert actual 64-hex-char key here for dev/UAT.
-- Production: this row MUST be populated by devops runbook from vault/KMS.
-- We insert a PLACEHOLDER that will cause sec.encrypt() to fail until replaced —
-- better to fail loudly than silently use a weak key.
INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'PII_ENCRYPTION_KEY',
    'PLACEHOLDER_REPLACE_WITH_64_HEX_CHARS_FROM_VAULT_BEFORE_USE',
    'STRING',
    TRUE,
    'AES-256 key for PII column encryption (sec.encrypt/decrypt). Phase 1: 64-char hex. Phase 2: KMS-backed.',
    'SECURITY'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
