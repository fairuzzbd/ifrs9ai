-- migration: 0035 jurnal_engine_p5_m2 (DOWN)
-- author: data-modeler
-- description:
--   Reverses migration 000035 in full:
--   (E) DELETE seeded event codes (sentinel UUID, event_id_kode EVT-001..027)
--   (D) DROP sys.seq_no_jurnal_2026; DELETE sys.config seeds
--   (C) DROP append-only triggers + functions on jrnl.header + jrnl.detail;
--       DROP uq_jrnl_source_event index; GRANT UPDATE/DELETE back to blips_service_role
--   (B) DROP sys.dlq_jurnal_post (table, indexes, triggers, functions)
--   (A) DROP approver_2 cols, workflow cols, SoD CHECKs from mst.mapping_jurnal_header;
--       restore workflow_status CHECK to pre-0035 list;
--       DROP new indexes on mapping_jurnal_header

BEGIN;

-- ====================================================================
-- E. DELETE seeded event codes (sentinel UUID only — never touch operator rows)
-- ====================================================================

DELETE FROM mst.mapping_jurnal_header
WHERE created_by = '00000000-0000-0000-0000-000000000001'
  AND event_id_kode IN (
      'EVT-001','EVT-002','EVT-003','EVT-004','EVT-005','EVT-006','EVT-007',
      'EVT-008','EVT-009','EVT-010','EVT-011','EVT-012','EVT-013','EVT-014',
      'EVT-015','EVT-016','EVT-017','EVT-018','EVT-019','EVT-020','EVT-021',
      'EVT-022','EVT-023','EVT-024','EVT-025','EVT-026','EVT-027'
  );

-- ====================================================================
-- D. DROP SEQUENCE + DELETE sys.config seeds
-- ====================================================================

DROP SEQUENCE IF EXISTS sys.seq_no_jurnal_2026;

DELETE FROM sys.config
WHERE config_key IN ('NO_JURNAL_CURRENT_YEAR', 'DLQ_MAX_ATTEMPTS');

-- ====================================================================
-- C. DROP append-only triggers + functions on jrnl schema
-- ====================================================================

-- C1. jrnl.header
DROP TRIGGER IF EXISTS trg_jrnl_header_no_update ON jrnl.header;
DROP TRIGGER IF EXISTS trg_jrnl_header_no_delete ON jrnl.header;
DROP FUNCTION IF EXISTS fn_jrnl_header_no_update();
DROP FUNCTION IF EXISTS fn_jrnl_header_no_delete();

-- C2. jrnl.detail
DROP TRIGGER IF EXISTS trg_jrnl_detail_no_update ON jrnl.detail;
DROP TRIGGER IF EXISTS trg_jrnl_detail_no_delete ON jrnl.detail;
DROP FUNCTION IF EXISTS fn_jrnl_detail_no_update();
DROP FUNCTION IF EXISTS fn_jrnl_detail_no_delete();

-- C3. Drop secondary idempotency index on jrnl.header
DROP INDEX IF EXISTS jrnl.uq_jrnl_source_event;

-- C4. Restore UPDATE/DELETE on jrnl tables for blips_service_role if it exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'blips_service_role') THEN
        GRANT UPDATE, DELETE ON jrnl.header TO blips_service_role;
        GRANT UPDATE, DELETE ON jrnl.detail TO blips_service_role;
    END IF;
END;
$$;

-- ====================================================================
-- B. DROP sys.dlq_jurnal_post (triggers first, then table)
-- ====================================================================

DROP TRIGGER IF EXISTS trg_dlq_jurnal_post_updated_at    ON sys.dlq_jurnal_post;
DROP TRIGGER IF EXISTS trg_dlq_jurnal_post_row_version   ON sys.dlq_jurnal_post;
DROP TRIGGER IF EXISTS trg_dlq_jurnal_post_no_hard_delete ON sys.dlq_jurnal_post;

-- Drop function after triggers that depend on it
DROP FUNCTION IF EXISTS fn_dlq_jurnal_post_no_hard_delete();

-- Drop indexes explicitly (CASCADE on DROP TABLE would handle them, but be explicit)
DROP INDEX IF EXISTS sys.uq_dlq_source_event_inflight;
DROP INDEX IF EXISTS sys.idx_dlq_status_created;
DROP INDEX IF EXISTS sys.idx_dlq_source_event_id;
DROP INDEX IF EXISTS sys.idx_dlq_event_code_status;
DROP INDEX IF EXISTS sys.idx_dlq_retry_count_failed;
DROP INDEX IF EXISTS sys.idx_dlq_instrumen_id;
DROP INDEX IF EXISTS sys.idx_dlq_periode_id;
DROP INDEX IF EXISTS sys.idx_dlq_tenant_created;

DROP TABLE IF EXISTS sys.dlq_jurnal_post;

-- ====================================================================
-- A. ALTER mst.mapping_jurnal_header — reverse all additions
-- ====================================================================

-- A-indexes: drop first (before column drops)
DROP INDEX IF EXISTS mst.idx_mapping_header_maker;
DROP INDEX IF EXISTS mst.idx_mapping_header_reviewer;
DROP INDEX IF EXISTS mst.idx_mapping_header_approver;
DROP INDEX IF EXISTS mst.idx_mapping_header_approver_2;
DROP INDEX IF EXISTS mst.idx_mapping_header_pending_approval_2;

-- A-CHECK: restore workflow_status to pre-0035 list (matches migration 000017)
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_header_workflow_status;

ALTER TABLE mst.mapping_jurnal_header
    ADD CONSTRAINT chk_mapping_header_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));
-- Note: Any rows with workflow_status = 'APPROVED_ACTIVE' or 'WITHDRAWN' (added in this
-- migration) must be updated or deleted before running DOWN. The seeded rows are DRAFT
-- (handled in E above). Operator-created rows in those states block this ALTER unless
-- manually remediated first.

-- A-SoD CHECKs added in this migration
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_4way;

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_reviewer_vs_maker;

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_approver_vs_maker;

ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_sod_approver_vs_reviewer;

-- A-workflow_path CHECK
ALTER TABLE mst.mapping_jurnal_header
    DROP CONSTRAINT IF EXISTS chk_mapping_workflow_path;

-- A-columns: approver_2
ALTER TABLE mst.mapping_jurnal_header
    DROP COLUMN IF EXISTS approver_2_id,
    DROP COLUMN IF EXISTS approver_2_signed_at,
    DROP COLUMN IF EXISTS approver_2_signature_hash,
    DROP COLUMN IF EXISTS comment_approve_2;

-- A-columns: approver (1st)
ALTER TABLE mst.mapping_jurnal_header
    DROP COLUMN IF EXISTS approver_id,
    DROP COLUMN IF EXISTS approver_signed_at,
    DROP COLUMN IF EXISTS approver_signature_hash,
    DROP COLUMN IF EXISTS comment_approve;

-- A-columns: reviewer
ALTER TABLE mst.mapping_jurnal_header
    DROP COLUMN IF EXISTS reviewer_id,
    DROP COLUMN IF EXISTS reviewer_signed_at,
    DROP COLUMN IF EXISTS reviewer_signature_hash,
    DROP COLUMN IF EXISTS comment_review;

-- A-columns: maker + workflow metadata
ALTER TABLE mst.mapping_jurnal_header
    DROP COLUMN IF EXISTS maker_id,
    DROP COLUMN IF EXISTS submit_at,
    DROP COLUMN IF EXISTS reject_reason,
    DROP COLUMN IF EXISTS workflow_path;

COMMIT;
