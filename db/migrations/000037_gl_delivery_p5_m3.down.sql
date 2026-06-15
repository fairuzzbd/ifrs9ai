-- migration: 0037 gl_delivery_p5_m3 (DOWN)
-- author: data-modeler
-- description:
--   Reversal of migration 000037 gl_delivery_p5_m3.
--   Drops in reverse dependency order:
--     1. sys.config rows seeded in E
--     2. sys.gl_recon_mismatch (D) — child of gl_reconciliation_report
--     3. sys.gl_reconciliation_report (C)
--     4. sys.dlq_gl_delivery (B)
--     5. jrnl.gl_status triggers + columns + constraints (A) — restores 000001 state
--
--   WARNING: This down migration drops tables with data.
--   Run only in dev/test environments or after confirming no production data exists.
--   Hard-delete REJECT triggers are dropped before CASCADE drops to allow clean rollback.

BEGIN;

-- ====================================================================
-- E. Remove sys.config seeds
-- ====================================================================

DELETE FROM sys.config
WHERE config_key IN (
    'GL_HOST_URL',
    'GL_HOST_AUTH_TYPE',
    'GL_HOST_API_KEY_VAULT_KEY',
    'GL_DELIVERY_RETRY_MAX',
    'GL_DELIVERY_RETRY_BACKOFF_SECONDS',
    'GL_DELIVERY_TIMEOUT_SECONDS',
    'GL_DELIVERY_MAX_TOTAL_ATTEMPTS',
    'GL_RECON_TOLERANCE_IDR',
    'GL_RECON_CRON',
    'GL_HOST_PII_FIELDS_TO_REDACT'
);

-- ====================================================================
-- D. DROP sys.gl_recon_mismatch (drop triggers first, then table)
-- ====================================================================

DROP TRIGGER IF EXISTS trg_gl_recon_mismatch_no_hard_delete ON sys.gl_recon_mismatch;
DROP TRIGGER IF EXISTS trg_gl_recon_mismatch_updated_at     ON sys.gl_recon_mismatch;
DROP TRIGGER IF EXISTS trg_gl_recon_mismatch_row_version    ON sys.gl_recon_mismatch;

DROP TABLE IF EXISTS sys.gl_recon_mismatch CASCADE;

DROP FUNCTION IF EXISTS fn_gl_recon_mismatch_no_hard_delete();

-- ====================================================================
-- C. DROP sys.gl_reconciliation_report
-- ====================================================================

DROP TRIGGER IF EXISTS trg_gl_recon_report_no_hard_delete ON sys.gl_reconciliation_report;
DROP TRIGGER IF EXISTS trg_gl_recon_report_updated_at     ON sys.gl_reconciliation_report;
DROP TRIGGER IF EXISTS trg_gl_recon_report_row_version    ON sys.gl_reconciliation_report;

DROP TABLE IF EXISTS sys.gl_reconciliation_report CASCADE;

DROP FUNCTION IF EXISTS fn_gl_recon_report_no_hard_delete();

-- ====================================================================
-- B. DROP sys.dlq_gl_delivery
-- ====================================================================

DROP TRIGGER IF EXISTS trg_dlq_gl_delivery_no_hard_delete ON sys.dlq_gl_delivery;
DROP TRIGGER IF EXISTS trg_dlq_gl_delivery_updated_at     ON sys.dlq_gl_delivery;
DROP TRIGGER IF EXISTS trg_dlq_gl_delivery_row_version    ON sys.dlq_gl_delivery;

DROP TABLE IF EXISTS sys.dlq_gl_delivery CASCADE;

DROP FUNCTION IF EXISTS fn_dlq_gl_delivery_no_hard_delete();

-- ====================================================================
-- A. Revert jrnl.gl_status changes
-- ====================================================================

-- A8. Remove standard triggers added in this migration
DROP TRIGGER IF EXISTS trg_gl_status_updated_at   ON jrnl.gl_status;
DROP TRIGGER IF EXISTS trg_gl_status_row_version   ON jrnl.gl_status;

-- A7. Remove hard-delete guard trigger + function
DROP TRIGGER IF EXISTS trg_gl_status_no_hard_delete ON jrnl.gl_status;
DROP FUNCTION IF EXISTS fn_gl_status_no_hard_delete();

-- A6. Remove terminal immutability trigger + function
DROP TRIGGER IF EXISTS trg_gl_status_terminal_immutable ON jrnl.gl_status;
DROP FUNCTION IF EXISTS fn_gl_status_terminal_immutable();

-- A5. Remove new indexes; restore 000001 indexes
DROP INDEX IF EXISTS jrnl.idx_gl_status_worker_scan;
DROP INDEX IF EXISTS jrnl.idx_gl_status_dead_letter;
DROP INDEX IF EXISTS jrnl.idx_gl_status_delivered_at;

CREATE INDEX IF NOT EXISTS ix_gl_status_pending
    ON jrnl.gl_status (gl_host_status)
    WHERE gl_host_status IN ('PENDING_DELIVERY', 'RETRYING', 'FAILED');

CREATE INDEX IF NOT EXISTS ix_gl_status_dlq
    ON jrnl.gl_status (gl_host_status)
    WHERE gl_host_status = 'DEAD_LETTER';

-- A4. Remove FK indexes on new columns
DROP INDEX IF EXISTS jrnl.idx_gl_status_manual_retry_by;
DROP INDEX IF EXISTS jrnl.idx_gl_status_discarded_by;

-- A3. Remove CHECK constraints added in A3
ALTER TABLE jrnl.gl_status
    DROP CONSTRAINT IF EXISTS chk_gl_status_failure_category,
    DROP CONSTRAINT IF EXISTS chk_gl_status_manual_retry_reason_len,
    DROP CONSTRAINT IF EXISTS chk_gl_status_discard_reason_len,
    DROP CONSTRAINT IF EXISTS chk_gl_status_dead_letter_has_discard;

-- A2. Remove columns added in A2 and A8
ALTER TABLE jrnl.gl_status
    DROP COLUMN IF EXISTS failure_category,
    DROP COLUMN IF EXISTS gl_response_payload_jsonb,
    DROP COLUMN IF EXISTS manual_retry_by,
    DROP COLUMN IF EXISTS manual_retry_at,
    DROP COLUMN IF EXISTS manual_retry_reason,
    DROP COLUMN IF EXISTS discarded_by,
    DROP COLUMN IF EXISTS discarded_at,
    DROP COLUMN IF EXISTS discard_reason,
    DROP COLUMN IF EXISTS payload_sent_at,
    DROP COLUMN IF EXISTS delivery_response_id,
    DROP COLUMN IF EXISTS row_version;

-- A1. Remove CHECK constraint added in A1
ALTER TABLE jrnl.gl_status
    DROP CONSTRAINT IF EXISTS chk_gl_status_host_status;

COMMIT;
