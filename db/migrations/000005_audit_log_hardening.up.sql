-- migration: 0005 audit_log_hardening
-- author: data-modeler
-- requires: 0001, 0004
-- description: (1) Add missing canonical columns to aud.audit_log (hash-chain pair,
--              trace_id, idempotency_key, tenant_id per db-conventions.md canonical spec).
--              (2) Hard-delete protection triggers for aud.*, jrnl.*, ecl.* schemas.
--              (3) sec.compute_audit_hash() helper function for hash-chain computation.
--
-- AUDIT LOG DELTA vs 0001:
--   0001 had:  hash_chain_prev CHAR(64)          — partial, non-standard column name
--   0005 adds: current_hash BYTEA NOT NULL,       — canonical per db-conventions.md
--              previous_hash BYTEA,               — canonical (BYTEA, not CHAR)
--              trace_id TEXT,                     — request trace propagation
--              idempotency_key UUID,              — link to sys.idempotency_key
--              tenant_id TEXT NOT NULL DEFAULT 'TUGURE'
--   NOTE: hash_chain_prev CHAR(64) from 0001 is KEPT for backward compat;
--         new writes should populate current_hash + previous_hash.
--         Migration to fully replace hash_chain_prev → current_hash is a separate
--         data-backfill task (outside DDL scope; no NOT NULL on current_hash for now).

BEGIN;

-- ====================================================================
-- 1. Add missing columns to aud.audit_log
-- ====================================================================
-- aud.audit_log is partitioned; ALTER TABLE on parent propagates to all partitions.

ALTER TABLE aud.audit_log
    ADD COLUMN IF NOT EXISTS current_hash      BYTEA,      -- SHA-256 of this row (canonical)
    ADD COLUMN IF NOT EXISTS previous_hash     BYTEA,      -- SHA-256 of preceding row (canonical)
    ADD COLUMN IF NOT EXISTS trace_id          TEXT,       -- X-Trace-Id request header
    ADD COLUMN IF NOT EXISTS idempotency_key   UUID,       -- link to sys.idempotency_key (soft FK)
    ADD COLUMN IF NOT EXISTS tenant_id         TEXT        NOT NULL DEFAULT 'TUGURE';

COMMENT ON COLUMN aud.audit_log.current_hash    IS 'SHA-256(previous_hash || canonical_json(row)). Computed by audit middleware at write time.';
COMMENT ON COLUMN aud.audit_log.previous_hash   IS 'current_hash of the immediately preceding audit_log row for the same entity_id. NULL for first event.';
COMMENT ON COLUMN aud.audit_log.trace_id        IS 'Request trace ID (X-Trace-Id / slog request_id) for correlation.';
COMMENT ON COLUMN aud.audit_log.idempotency_key IS 'Idempotency-Key UUID from originating request. Links to sys.idempotency_key for dedup traceability.';
COMMENT ON COLUMN aud.audit_log.tenant_id       IS 'Tenant identifier — placeholder for Phase 2 multi-tenant (DEC-023).';

-- ====================================================================
-- 2. Hash-chain compute helper function
-- ====================================================================
-- Used by Go audit middleware to compute current_hash before writing.
-- Also used by cmd/audit-verify --range to verify chain integrity.
--
-- Formula: current_hash = sha256(coalesce(previous_hash,'') || canonical_json)
-- canonical_json = jsonb_build_object with sorted keys, no whitespace.
CREATE OR REPLACE FUNCTION sec.compute_audit_hash(
    p_previous_hash  BYTEA,
    p_canonical_json JSONB
)
RETURNS BYTEA
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    SELECT digest(
        COALESCE(p_previous_hash, '\x'::BYTEA) || convert_to(p_canonical_json::TEXT, 'UTF8'),
        'sha256'
    );
$$;

COMMENT ON FUNCTION sec.compute_audit_hash(BYTEA, JSONB) IS
    'Computes SHA-256 hash for audit log hash-chain: sha256(previous_hash || canonical_json). '
    'canonical_json must be produced with consistent key ordering (jsonb_build_object ensures PG key-sorted output). '
    'Go equivalent: sha256.Sum256(append(previousHash, []byte(canonicalJSON)...))';

-- ====================================================================
-- 3. Hard-delete protection: aud.* schema tables
-- ====================================================================
-- aud.audit_log already has tg_audit_log_no_update from 0001 (via fn_audit_no_modify).
-- Add protection to other aud.* tables that must be immutable.

-- aud.workflow_history — append-only
CREATE OR REPLACE FUNCTION fn_aud_no_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Hard delete is forbidden on % (schema=aud). '
        'Records in aud.* are immutable per DEC-018 (retention 10+10 years). '
        'Use archival to cold storage instead.',
        TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_workflow_history_no_delete
    BEFORE DELETE ON aud.workflow_history
    FOR EACH ROW EXECUTE FUNCTION fn_aud_no_delete();

CREATE TRIGGER tg_workflow_history_no_update
    BEFORE UPDATE ON aud.workflow_history
    FOR EACH ROW EXECUTE FUNCTION fn_audit_no_modify();

CREATE TRIGGER tg_login_history_no_delete
    BEFORE DELETE ON aud.login_history
    FOR EACH ROW EXECUTE FUNCTION fn_aud_no_delete();

-- ====================================================================
-- 4. Hard-delete protection: jrnl.* schema tables
-- ====================================================================
-- jrnl records are accounting entries — must never be hard-deleted.
-- Reversal is done via new jurnal with reversed_by_jurnal_id reference.

CREATE OR REPLACE FUNCTION fn_jrnl_no_hard_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Hard delete is forbidden on jrnl.% '
        '(schema=jrnl, DEC-018). '
        'Use reversal journal (reversed_by_jurnal_id) instead.',
        TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_jrnl_header_no_delete
    BEFORE DELETE ON jrnl.header
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_no_hard_delete();

CREATE TRIGGER tg_jrnl_detail_no_delete
    BEFORE DELETE ON jrnl.detail
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_no_hard_delete();

CREATE TRIGGER tg_jrnl_gl_status_no_delete
    BEFORE DELETE ON jrnl.gl_status
    FOR EACH ROW EXECUTE FUNCTION fn_jrnl_no_hard_delete();

-- ====================================================================
-- 5. Hard-delete protection: ecl.* schema tables
-- ====================================================================
-- ECL calculation results and schedules must be immutable (audit-grade).
-- Amendment creates new version rows, never updates/deletes existing.

CREATE OR REPLACE FUNCTION fn_ecl_no_hard_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Hard delete is forbidden on ecl.% '
        '(schema=ecl, DEC-018). '
        'ECL records are immutable calculation results. '
        'Use versioned amendment / new calc_run instead.',
        TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_ecl_calc_header_no_delete
    BEFORE DELETE ON ecl.calc_header
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

CREATE TRIGGER tg_ecl_calc_detail_no_delete
    BEFORE DELETE ON ecl.calc_detail_skenario
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

CREATE TRIGGER tg_ecl_lookthrough_no_delete
    BEFORE DELETE ON ecl.lookthrough_underlying
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

CREATE TRIGGER tg_ecl_stage_history_no_delete
    BEFORE DELETE ON ecl.stage_history
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

CREATE TRIGGER tg_ecl_eir_schedule_no_delete
    BEFORE DELETE ON ecl.eir_amortization_schedule
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

CREATE TRIGGER tg_ecl_eir_log_no_delete
    BEFORE DELETE ON ecl.eir_reestimation_log
    FOR EACH ROW EXECUTE FUNCTION fn_ecl_no_hard_delete();

-- ====================================================================
-- 6. Partial indexes for hash-chain verification
-- ====================================================================
-- Used by cmd/audit-verify to walk chain efficiently.
CREATE INDEX IF NOT EXISTS ix_audit_hash_entity
    ON aud.audit_log(entity_type, entity_id, timestamp ASC)
    WHERE current_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_audit_tenant_time
    ON aud.audit_log(tenant_id, timestamp DESC);

COMMIT;
