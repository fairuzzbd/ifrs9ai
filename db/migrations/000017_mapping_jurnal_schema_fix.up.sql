-- migration: 0017 mapping_jurnal_schema_fix
-- author: backend-engineer-go
-- requires: 0001, 0008
-- description: (1) Add missing audit columns (deleted_at, deleted_by, row_version, tenant_id,
--              workflow_status) to mst.mapping_jurnal_header; backfill APPROVED.
--              (2) Add missing audit columns (created_by, updated_by, deleted_at, deleted_by,
--              row_version, tenant_id) to mst.mapping_jurnal_detail.
--              (3) Add supporting indexes for tenant/workflow queries.

BEGIN;

-- ============================================================
-- 1. mst.mapping_jurnal_header — ADD MISSING COLUMNS
-- ============================================================

-- 1a. deleted_at / deleted_by  (soft-delete, per db-conventions.md)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id);

-- 1b. row_version  (optimistic lock, per db-conventions.md)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1;

-- 1c. tenant_id  (multi-tenant placeholder, DEC-023)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- 1d. workflow_status  (7-state, matches mst.mata_uang pattern from 000008)
ALTER TABLE mst.mapping_jurnal_header
    ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- 1e. DROP then re-ADD constraint so re-runs are idempotent
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_header_workflow_status;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_header_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- 1f. Backfill: all pre-existing rows are considered APPROVED (in use pre-workflow)
UPDATE mst.mapping_jurnal_header
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- 1g. Indexes for tenant and workflow queries on header
CREATE INDEX IF NOT EXISTS idx_mapping_header_tenant_created
    ON mst.mapping_jurnal_header(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_mapping_header_workflow_status
    ON mst.mapping_jurnal_header(workflow_status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mapping_header_aktif
    ON mst.mapping_jurnal_header(aktif_flag) WHERE deleted_at IS NULL;

-- ============================================================
-- 2. mst.mapping_jurnal_detail — ADD MISSING COLUMNS
-- ============================================================

-- 2a. created_by / updated_by  (per db-conventions.md — wajib di semua tabel)
ALTER TABLE mst.mapping_jurnal_detail
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id);

-- 2b. deleted_at / deleted_by  (soft-delete)
ALTER TABLE mst.mapping_jurnal_detail
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id);

-- 2c. row_version  (detail rows use optimistic lock individually)
ALTER TABLE mst.mapping_jurnal_detail
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1;

-- 2d. tenant_id  (must match header's tenant)
ALTER TABLE mst.mapping_jurnal_detail
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- NOTE: No workflow_status on detail — detail inherits workflow from header.
--       The header workflow_status governs the entire mapping (header + all its details).

-- 2e. Indexes for tenant queries on detail
CREATE INDEX IF NOT EXISTS idx_mapping_detail_tenant
    ON mst.mapping_jurnal_detail(tenant_id, event_header_id);

-- Partial index for active detail rows (exclude soft-deleted)
CREATE INDEX IF NOT EXISTS idx_mapping_detail_active
    ON mst.mapping_jurnal_detail(event_header_id, urutan)
    WHERE deleted_at IS NULL;

COMMIT;
