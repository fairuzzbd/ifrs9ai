-- migration: 0021 counterparty_rating_schema_fix
-- author: data-modeler
-- requires: 0001, 0003 (sec.encrypt/decrypt), 0007, 0008
-- description: PII columns (nomor_rekening_encrypted, ktp_encrypted)
--              for DEC-028 — added defensively (already present from 0003,
--              IF NOT EXISTS guards idempotency). workflow_status + full
--              audit cols for both mst.counterparty and
--              mst.rating_history_counterparty. Schema drift
--              (version vs row_version, is_deleted vs deleted_at) is
--              co-existed for Phase 3 — deprecation cycle deferred to
--              Phase 5 alignment (see TODO comments below).
--              BLOCKING security-engineer gate: PII column comments
--              and blips_pii_accessor grant documented here; actual
--              GRANT is performed by devops runbook (env-specific).
--              WORKFLOW_CONFIG_COUNTERPARTY + WORKFLOW_CONFIG_RATING_HISTORY
--              were seeded in 0008 — NOT re-seeded here.

BEGIN;

-- ============================================================
-- SECTION 1: mst.counterparty
-- ============================================================

-- ------------------------------------------------------------
-- 1a. PII columns (DEC-028)
--     NOTE: 0003 already added these columns via ADD COLUMN IF NOT EXISTS.
--     Re-applying with IF NOT EXISTS here for idempotency and
--     to make the audit trail explicit in this migration.
--     SECURITY: blips_pii_accessor role required for sec.decrypt().
--     Service layer calls sec.encrypt() on INSERT/UPDATE; never
--     stores plaintext. Column type TEXT (no length cap — ciphertext
--     length varies with pgcrypto pgp_sym_encrypt output).
-- ------------------------------------------------------------
ALTER TABLE mst.counterparty
    ADD COLUMN IF NOT EXISTS nomor_rekening_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS ktp_encrypted             TEXT;

COMMENT ON COLUMN mst.counterparty.nomor_rekening_encrypted IS
    'AES-256 encrypted nomor rekening (DEC-028). Encrypt: sec.encrypt(). '
    'Decrypt: sec.decrypt() — requires blips_pii_accessor role. '
    'First added by 0003; idempotent re-declaration in 0015.';

COMMENT ON COLUMN mst.counterparty.ktp_encrypted IS
    'AES-256 encrypted KTP/NIK (DEC-028). Encrypt: sec.encrypt(). '
    'Decrypt: sec.decrypt() — requires blips_pii_accessor role. '
    'First added by 0003; idempotent re-declaration in 0015.';

COMMENT ON COLUMN mst.counterparty.npwp_encrypted IS
    'AES-256 encrypted NPWP (DEC-028). Encrypt: sec.encrypt(). '
    'Decrypt: sec.decrypt() — requires blips_pii_accessor role. '
    'Present since 0001.';

-- ------------------------------------------------------------
-- 1b. Missing audit cols
--     co-exist with legacy `version INT` and `is_deleted BOOLEAN`
--     from 0001 — do NOT rename/drop them here.
--     TODO (Phase 5 alignment): migrate `version` → `row_version`,
--           `is_deleted` → `deleted_at`, then drop the legacy columns
--           after all read paths are updated.
-- ------------------------------------------------------------
ALTER TABLE mst.counterparty
    ADD COLUMN IF NOT EXISTS deleted_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by    UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version   BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id     TEXT   NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- 1c. Backfill deleted_at from is_deleted (best-effort; is_deleted is the
--     authoritative soft-delete flag until Phase 5 cutover).
--     Rows that were is_deleted=TRUE get deleted_at = now() as a sentinel
--     so that queries filtering on deleted_at IS NULL are consistent
--     with is_deleted=FALSE filters.
UPDATE mst.counterparty
    SET deleted_at = now()
WHERE is_deleted = TRUE
  AND deleted_at IS NULL;

-- ------------------------------------------------------------
-- 1d. CHECK constraints
--     Extend tipe whitelist: original 0001 CHECK ck_counterparty_tipe
--     covers ('BANK','BANK_KUSTODIAN','KORPORASI','PEMERINTAH',
--             'MANAJER_INVESTASI','EMITEN_SAHAM').
--     Added for Tugu Reasuransi context:
--       MULTILATERAL — multilateral/supranational counterparty
--       KORPORASI_BUMN — state-owned enterprise (BUMN)
--       INDIVIDU — individual (e.g. FVOCI equity issuer)
--       REASURADUR — reinsurance cession counterparty
--     Drop & recreate to include extended values.
-- ------------------------------------------------------------
ALTER TABLE mst.counterparty
    DROP CONSTRAINT IF EXISTS ck_counterparty_tipe;

ALTER TABLE mst.counterparty
    ADD CONSTRAINT ck_counterparty_tipe CHECK (tipe IN (
        'BANK',
        'BANK_KUSTODIAN',
        'KORPORASI',
        'PEMERINTAH',
        'MANAJER_INVESTASI',
        'EMITEN_SAHAM',
        'MULTILATERAL',
        'KORPORASI_BUMN',
        'INDIVIDU',
        'REASURADUR'
    ));

ALTER TABLE mst.counterparty
    DROP CONSTRAINT IF EXISTS chk_counterparty_workflow_status;

ALTER TABLE mst.counterparty
    ADD CONSTRAINT chk_counterparty_workflow_status CHECK (workflow_status IN (
        'DRAFT',
        'PENDING_REVIEW',
        'PENDING_APPROVAL',
        'PENDING_APPROVAL_2',
        'APPROVED',
        'REJECTED',
        'RETURNED'
    ));

-- 1e. Backfill workflow_status for existing rows.
--     Rows that were previously created (pre-workflow era) are treated
--     as APPROVED — they were in operational use before workflow enforcement.
--     Rows with is_deleted=TRUE that got deleted_at backfilled stay APPROVED
--     (soft-deleted approved records). Only rows still in DRAFT default
--     that actually existed operationally are promoted.
UPDATE mst.counterparty
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ------------------------------------------------------------
-- 1f. Indexes
-- ------------------------------------------------------------

-- Workflow status partial index (active rows only)
CREATE INDEX IF NOT EXISTS idx_counterparty_workflow_status
    ON mst.counterparty(workflow_status)
    WHERE deleted_at IS NULL;

-- Tenant + time composite (mandatory for multi-tenant query pattern)
CREATE INDEX IF NOT EXISTS idx_counterparty_tenant_created
    ON mst.counterparty(tenant_id, created_at DESC);

-- Safe PII lookup by business key — exposes kode_counterparty only,
-- does not touch encrypted columns. Used by service layer to fetch
-- the id before calling sec.decrypt() in a separate privileged call.
CREATE INDEX IF NOT EXISTS idx_counterparty_pii_lookup
    ON mst.counterparty(kode_counterparty)
    WHERE deleted_at IS NULL;

-- Partial index on tenant + deleted_at IS NULL (hot query path)
CREATE INDEX IF NOT EXISTS idx_counterparty_tenant_active
    ON mst.counterparty(tenant_id)
    WHERE deleted_at IS NULL;


-- ============================================================
-- SECTION 2: mst.rating_history_counterparty
-- ============================================================

-- ------------------------------------------------------------
-- 2a. Add full audit columns
--     Legacy columns from 0001: maker_id, approver_id, created_at,
--     approved_at — retained as-is (read paths still use maker_id).
--     TODO (Phase 5): unify maker_id → created_by, approver_id →
--           updated_by after workflow engine adoption is confirmed.
-- ------------------------------------------------------------
ALTER TABLE mst.rating_history_counterparty
    ADD COLUMN IF NOT EXISTS created_by    UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by    UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by    UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version   BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id     TEXT   NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- 2b. Backfill created_by from maker_id (semantic alias until Phase 5 unification)
UPDATE mst.rating_history_counterparty
    SET created_by = maker_id
WHERE created_by IS NULL
  AND maker_id   IS NOT NULL;

-- 2c. CHECK constraint — 7-state workflow
ALTER TABLE mst.rating_history_counterparty
    DROP CONSTRAINT IF EXISTS chk_rating_history_workflow_status;

ALTER TABLE mst.rating_history_counterparty
    ADD CONSTRAINT chk_rating_history_workflow_status CHECK (workflow_status IN (
        'DRAFT',
        'PENDING_REVIEW',
        'PENDING_APPROVAL',
        'PENDING_APPROVAL_2',
        'APPROVED',
        'REJECTED',
        'RETURNED'
    ));

-- 2d. Backfill workflow_status.
--     Rows with a non-null approver_id were explicitly approved
--     pre-workflow; promote to APPROVED.
--     All other existing rows (approver_id IS NULL) that were in
--     operational use also treated as APPROVED (they are historical
--     Pefindo rating imports that were accepted operationally).
UPDATE mst.rating_history_counterparty
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ------------------------------------------------------------
-- 2e. Indexes
-- ------------------------------------------------------------

CREATE INDEX IF NOT EXISTS idx_rating_history_workflow_status
    ON mst.rating_history_counterparty(workflow_status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_rating_history_tenant_created
    ON mst.rating_history_counterparty(tenant_id, created_at DESC);

-- FK index on counterparty_id (already exists as ix_rating_cp_tanggal
-- in 0001 which covers counterparty_id; add dedicated FK index for
-- queries that don't order by tanggal_berlaku)
CREATE INDEX IF NOT EXISTS idx_rating_history_counterparty_fk
    ON mst.rating_history_counterparty(counterparty_id)
    WHERE deleted_at IS NULL;

-- Partial index for active (non-ended) ratings — hot SICR lookup path
CREATE INDEX IF NOT EXISTS idx_rating_history_active_counterparty
    ON mst.rating_history_counterparty(counterparty_id, tanggal_berlaku DESC)
    WHERE tanggal_berakhir IS NULL
      AND deleted_at IS NULL;

COMMIT;
