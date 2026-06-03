-- migration: 0009 periode_buku_schema_fix
-- author: data-modeler
-- requires: 0001, 0007
-- description: Backfill mst.periode_buku with missing audit cols + workflow_status.
--              id UUID PK already in place (0001) — no surrogate needed.
--              WORKFLOW_CONFIG_PERIODE already seeded in 0007 — not touched.

BEGIN;

-- ============================================================
-- 1. mst.periode_buku — ADD MISSING AUDIT COLUMNS
--    created_at  : already present (0001) — skip
--    updated_at  : already present (0001, nullable TIMESTAMPTZ) — skip
-- ============================================================

ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- ============================================================
-- 2. workflow_status — ADD COLUMN + CHECK CONSTRAINT
--    Tracks CRUD approval flow (DRAFT → APPROVED).
--    Distinct from status_periode (OPEN/SOFT_CLOSED/CLOSED)
--    which controls the financial period lifecycle.
-- ============================================================

ALTER TABLE mst.periode_buku
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

ALTER TABLE mst.periode_buku
    DROP CONSTRAINT IF EXISTS chk_periode_workflow_status;

ALTER TABLE mst.periode_buku
    ADD CONSTRAINT chk_periode_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- ============================================================
-- 3. BACKFILL — set existing rows to APPROVED
--    (rows existed before workflow engine; treat as pre-approved)
-- ============================================================

UPDATE mst.periode_buku
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- ============================================================
-- 4. INDEXES
-- ============================================================

-- Tenant + time composite for tenant-scoped list queries
CREATE INDEX IF NOT EXISTS idx_periode_buku_tenant_created
    ON mst.periode_buku(tenant_id, created_at DESC);

-- Workflow queue filter (active rows only)
CREATE INDEX IF NOT EXISTS idx_periode_buku_workflow_status
    ON mst.periode_buku(workflow_status) WHERE deleted_at IS NULL;

-- Status + period filter for list/reporting (partial — active rows only)
-- Distinct from ix_periode_tahun_bulan (0001) which covers (tahun_buku, bulan)
-- without status_periode column and without deleted_at guard.
CREATE INDEX IF NOT EXISTS idx_periode_buku_active
    ON mst.periode_buku(status_periode, tahun_buku, bulan) WHERE deleted_at IS NULL;

-- ============================================================
-- 5. sys.config — SEED WORKFLOW_CONFIG_PERIODE_BUKU (CRUD master)
--
--    Distinct from WORKFLOW_CONFIG_PERIODE (seeded 0007) which governs
--    the SOFT/HARD CLOSE workflow under APP-D Phase 5 (permissions
--    periode.softclose / periode.hardclose).
--
--    This config governs CRUD master approval flow:
--      Maker     ROLE-AKUN         (akuntansi setup periode)
--      Reviewer  ROLE-AKUN-CTL     (Finance Controller)
--      Approver  ROLE-CFO          (executive sign-off, step-up MFA per DEC-027)
--
--    Handler resolves resource "periode-buku" → entity type PERIODE_BUKU
--    → sys.config key WORKFLOW_CONFIG_PERIODE_BUKU (workflow/config.go
--    configKey + handler normalizeEntityType).
-- ============================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES (
    'WORKFLOW_CONFIG_PERIODE_BUKU',
    '{
        "entityType": "PERIODE_BUKU",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "periode_buku.submit",
            "review":  "periode_buku.review",
            "approve": "periode_buku.approve",
            "reject":  "periode_buku.reject"
        },
        "stepUpRequired": {
            "approve": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.periode_buku CRUD master — 4-eyes AKUN→AKUN-CTL(review)→CFO(approve). Step-up MFA required on approve (DEC-027). Distinct from WORKFLOW_CONFIG_PERIODE which is the soft/hard close workflow under APP-D Phase 5.',
    'WORKFLOW'
)
ON CONFLICT (config_key) DO NOTHING;

COMMIT;
