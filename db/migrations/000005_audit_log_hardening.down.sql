-- migration: 0005 audit_log_hardening (DOWN)
-- author: data-modeler
-- description: Remove columns, triggers, and functions added in 0005.
--              NOTE: Dropping columns from a partitioned table requires PG 14+.
--              The hash-chain_prev column from 0001 is NOT touched.

BEGIN;

-- Drop hard-delete protection triggers (ecl)
DROP TRIGGER IF EXISTS tg_ecl_eir_log_no_delete          ON ecl.eir_reestimation_log;
DROP TRIGGER IF EXISTS tg_ecl_eir_schedule_no_delete     ON ecl.eir_amortization_schedule;
DROP TRIGGER IF EXISTS tg_ecl_stage_history_no_delete    ON ecl.stage_history;
DROP TRIGGER IF EXISTS tg_ecl_lookthrough_no_delete      ON ecl.lookthrough_underlying;
DROP TRIGGER IF EXISTS tg_ecl_calc_detail_no_delete      ON ecl.calc_detail_skenario;
DROP TRIGGER IF EXISTS tg_ecl_calc_header_no_delete      ON ecl.calc_header;

-- Drop hard-delete protection triggers (jrnl)
DROP TRIGGER IF EXISTS tg_jrnl_gl_status_no_delete       ON jrnl.gl_status;
DROP TRIGGER IF EXISTS tg_jrnl_detail_no_delete          ON jrnl.detail;
DROP TRIGGER IF EXISTS tg_jrnl_header_no_delete          ON jrnl.header;

-- Drop hard-delete protection triggers (aud)
DROP TRIGGER IF EXISTS tg_login_history_no_delete        ON aud.login_history;
DROP TRIGGER IF EXISTS tg_workflow_history_no_update     ON aud.workflow_history;
DROP TRIGGER IF EXISTS tg_workflow_history_no_delete     ON aud.workflow_history;

-- Drop helper functions
DROP FUNCTION IF EXISTS fn_ecl_no_hard_delete();
DROP FUNCTION IF EXISTS fn_jrnl_no_hard_delete();
DROP FUNCTION IF EXISTS fn_aud_no_delete();
DROP FUNCTION IF EXISTS sec.compute_audit_hash(BYTEA, JSONB);

-- Drop indexes added in 0005
DROP INDEX IF EXISTS aud.ix_audit_hash_entity;
DROP INDEX IF EXISTS aud.ix_audit_tenant_time;

-- Drop columns added to aud.audit_log
-- (PG 14+ supports ALTER TABLE on partitioned parent)
ALTER TABLE aud.audit_log
    DROP COLUMN IF EXISTS current_hash,
    DROP COLUMN IF EXISTS previous_hash,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS tenant_id;

COMMIT;
