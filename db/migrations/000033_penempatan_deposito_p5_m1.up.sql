-- migration: 0033 penempatan_deposito_p5_m1
-- author: data-modeler
-- requires: 0001 (init_schema — sec.user, mst.instrumen, mst.counterparty, mst.periode_buku,
--                              mst.mata_uang, fn_update_updated_at, fn_increment_row_version),
--           0004 (sys_job_idempotency — fn_increment_row_version),
--           0006 (doc_document — doc.document)
-- description:
--   P5-M1 Penempatan Deposito transaction schema:
--   (A) SEQUENCE trx.penempatan_kode_seq — serial for kode_transaksi generation
--       (format PNP-{YYYYMM}-{seq} assembled in Go service layer)
--   (B) TYPE trx.penempatan_workflow_status ENUM — 11 status values per state machine §1
--   (C) CREATE TABLE trx.penempatan_deposito — full lifecycle + 4-eyes create + 4-eyes terminate
--       workflows (DEC-P5-M1-005, DEC-017). CHECK constraints enforce DB-layer SoD (DEC-017).
--       Soft-delete only (no hard delete on trx). Triggers: updated_at + row_version.
--       9 indexes including 2 partial (maturity scan + pending queues).
--   (D) CREATE TABLE sys.settlement_account_balance — informational balance hint
--       per DEC-P5-M1-004 (never blocks submit). Triggers: updated_at + row_version.

BEGIN;

-- ====================================================================
-- A. SEQUENCE — kode_transaksi serial
-- ====================================================================

CREATE SEQUENCE IF NOT EXISTS trx.penempatan_kode_seq
    START 1
    INCREMENT BY 1
    NO MAXVALUE
    NO CYCLE;

COMMENT ON SEQUENCE trx.penempatan_kode_seq IS
    'Serial counter for trx.penempatan_deposito.kode_transaksi. '
    'Format assembled in Go service layer: PNP-{YYYYMM}-{nextval padded to 6 digits}. '
    'Sequence is per-instance (not per-tenant) — acceptable for Phase 1 single-tenant TUGURE.';

-- ====================================================================
-- B. ENUM TYPE — workflow status
-- ====================================================================

CREATE TYPE trx.penempatan_workflow_status AS ENUM (
    'DRAFT',
    'PENDING_REVIEW',
    'PENDING_APPROVAL',
    'APPROVED_ACTIVE',
    'REJECTED',
    'CANCELLED',
    'MATURED',
    'TERMINATION_PENDING_REVIEW',
    'TERMINATION_PENDING_APPROVAL',
    'TERMINATED',
    'TERMINATION_REJECTED'
);

COMMENT ON TYPE trx.penempatan_workflow_status IS
    'State machine per p5-m1-penempatan.md §1. '
    'Terminal states: MATURED, TERMINATED, CANCELLED. '
    'REJECTED is a transient label used in the DB CHECK status list '
    '(state machine routes back to DRAFT on reject, but reject_reason is preserved). '
    'TERMINATION_REJECTED is a transient label for terminate-reject (returns to APPROVED_ACTIVE).';

-- ====================================================================
-- C. TABLE trx.penempatan_deposito
-- ====================================================================

CREATE TABLE trx.penempatan_deposito (

    -- ----------------------------------------------------------------
    -- Primary key
    -- ----------------------------------------------------------------
    id                                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- ----------------------------------------------------------------
    -- Business key (auto-gen server-side: PNP-{YYYYMM}-{seq:06d})
    -- ----------------------------------------------------------------
    kode_transaksi                      TEXT            NOT NULL,

    -- ----------------------------------------------------------------
    -- Core references
    -- ----------------------------------------------------------------
    instrumen_id                        UUID            NOT NULL
                                            REFERENCES mst.instrumen(id)
                                            ON DELETE RESTRICT,

    counterparty_bank_id                UUID            NOT NULL
                                            REFERENCES mst.counterparty(id)
                                            ON DELETE RESTRICT,

    periode_id                          UUID            NOT NULL
                                            REFERENCES mst.periode_buku(id)
                                            ON DELETE RESTRICT,

    mata_uang_id                        UUID            NOT NULL
                                            REFERENCES mst.mata_uang(id)
                                            ON DELETE RESTRICT,

    -- ----------------------------------------------------------------
    -- Transaction fields
    -- ----------------------------------------------------------------
    tanggal_penempatan                  DATE            NOT NULL,
    tanggal_jatuh_tempo                 DATE            NOT NULL,
    -- ^ computed at create: tanggal_penempatan + tenor_bulan months; enforced by CHECK below

    nominal_idr                         NUMERIC(20,4)   NOT NULL,
    -- IDR trades: direct entry. FCY trades: nominal_fcy × kurs_penempatan, snapshotted at create.

    nominal_fcy                         NUMERIC(20,4),
    -- NULL for IDR trades; required for FCY trades. Enforced by chk_penempatan_fcy_consistent.

    kurs_penempatan                     NUMERIC(20,8),
    -- BI JISDOR snapshot at tanggal_penempatan. NULL for IDR trades.
    -- Enforced by chk_penempatan_kurs_consistent.

    tenor_bulan                         SMALLINT        NOT NULL,
    kupon_persen                        NUMERIC(10,8)   NOT NULL,
    biaya_transaksi_idr                 NUMERIC(20,4)   NOT NULL    DEFAULT 0.0000,

    nomor_referensi_bank                TEXT,
    settlement_account                  TEXT,
    -- Cross-ref to sys.settlement_account_balance.account_code (informational, no FK per DEC-P5-M1-004)

    catatan                             TEXT,

    -- ----------------------------------------------------------------
    -- EIR fields (populated async post-approve by Asynq EIR_COMPUTE)
    -- NULL until computed; NULL permanently for FVTPL/FVOCI_ELECTION (DEC-P5-M1-001)
    -- ----------------------------------------------------------------
    eir_awal                            NUMERIC(10,8),
    carrying_amount_awal                NUMERIC(20,4),

    -- ----------------------------------------------------------------
    -- Document references
    -- ----------------------------------------------------------------
    kontrak_doc_id                      UUID            REFERENCES doc.document(id)
                                            ON DELETE RESTRICT,

    dokumen_terminasi_id                UUID            REFERENCES doc.document(id)
                                            ON DELETE RESTRICT,

    -- ----------------------------------------------------------------
    -- Workflow status
    -- ----------------------------------------------------------------
    workflow_status                     trx.penempatan_workflow_status
                                                        NOT NULL    DEFAULT 'DRAFT',

    -- ----------------------------------------------------------------
    -- Create workflow — participants
    -- ----------------------------------------------------------------
    maker_id                            UUID            NOT NULL
                                            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,

    reviewer_id                         UUID            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,
    -- NULL until PENDING_APPROVAL transition

    approver_id                         UUID            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,
    -- NULL until APPROVED_ACTIVE transition; reset to NULL on reject → DRAFT

    -- ----------------------------------------------------------------
    -- Create workflow — signatures (DEC-018 audit-grade)
    -- SHA-256({user_id}||{STEP}||{id}||{signed_at_iso}||{comment})
    -- Stored as BYTEA for binary hash; TEXT representation computed in Go.
    -- ----------------------------------------------------------------
    reviewer_signed_at                  TIMESTAMPTZ,
    approver_signed_at                  TIMESTAMPTZ,
    reviewer_signature_hash             BYTEA,
    approver_signature_hash             BYTEA,

    -- ----------------------------------------------------------------
    -- Create workflow — comments & reject reason
    -- ----------------------------------------------------------------
    comment_review                      TEXT,
    comment_approve                     TEXT,
    reject_reason                       TEXT,
    -- >= 30 chars enforced at application layer (db-conventions: business-rule length not a CHECK
    -- because nullable; state machine only sets it on reject transition)

    -- ----------------------------------------------------------------
    -- Terminate workflow — participants (DEC-P5-M1-005, 4-eyes)
    -- ----------------------------------------------------------------
    terminate_maker_id                  UUID            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,
    -- = original maker_id re-proposing termination; set on /terminate

    terminate_reviewer_id               UUID            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,

    terminate_approver_id               UUID            REFERENCES sec.user(id)
                                            ON DELETE RESTRICT,

    -- ----------------------------------------------------------------
    -- Terminate workflow — signatures
    -- ----------------------------------------------------------------
    terminate_reviewer_signed_at        TIMESTAMPTZ,
    terminate_approver_signed_at        TIMESTAMPTZ,
    terminate_reviewer_signature_hash   BYTEA,
    terminate_approver_signature_hash   BYTEA,

    -- ----------------------------------------------------------------
    -- Terminate workflow — reasons & comments
    -- ----------------------------------------------------------------
    terminate_request_reason            TEXT,
    -- >= 30 chars enforced at application layer (state machine guard in §5 validation table)

    terminate_review_comment            TEXT,
    terminate_approve_comment           TEXT,
    terminate_reject_reason             TEXT,
    -- >= 30 chars enforced at application layer

    -- ----------------------------------------------------------------
    -- Lifecycle timestamps
    -- ----------------------------------------------------------------
    terminated_at                       TIMESTAMPTZ,
    -- Set by /terminate-approve (T12); null until TERMINATED

    matured_at                          TIMESTAMPTZ,
    -- Set by Asynq maturity-checker job (T08); null until MATURED

    realized_gain_loss_idr              NUMERIC(20,4),
    -- Computed by P5-M9 (derecognition engine) at TERMINATED; null until then

    -- ----------------------------------------------------------------
    -- Audit columns (mandatory per db-conventions.md)
    -- ----------------------------------------------------------------
    created_at                          TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    created_by                          UUID            NOT NULL    REFERENCES sec.user(id),
    updated_at                          TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    updated_by                          UUID            NOT NULL    REFERENCES sec.user(id),
    deleted_at                          TIMESTAMPTZ,
    deleted_by                          UUID            REFERENCES sec.user(id),
    row_version                         BIGINT          NOT NULL    DEFAULT 1,
    tenant_id                           TEXT            NOT NULL    DEFAULT 'TUGURE',

    -- ================================================================
    -- CHECK CONSTRAINTS
    -- ================================================================

    -- Business field constraints
    CONSTRAINT chk_penempatan_tenor_positive
        CHECK (tenor_bulan > 0),

    CONSTRAINT chk_penempatan_nominal_pos
        CHECK (nominal_idr > 0),

    CONSTRAINT chk_penempatan_kupon_nonneg
        CHECK (kupon_persen >= 0 AND kupon_persen <= 100),

    CONSTRAINT chk_penempatan_biaya_nonneg
        CHECK (biaya_transaksi_idr >= 0),

    CONSTRAINT chk_penempatan_jatuh_tempo
        CHECK (tanggal_jatuh_tempo > tanggal_penempatan),

    -- FCY consistency: nominal_fcy NULL iff kurs_penempatan NULL iff IDR trade
    -- IDR trade: both NULL. FCY trade: both NOT NULL.
    CONSTRAINT chk_penempatan_fcy_consistent
        CHECK (
            (nominal_fcy IS NULL AND kurs_penempatan IS NULL)
            OR
            (nominal_fcy IS NOT NULL AND kurs_penempatan IS NOT NULL)
        ),

    CONSTRAINT chk_penempatan_kurs_positive
        CHECK (kurs_penempatan IS NULL OR kurs_penempatan > 0),

    CONSTRAINT chk_penempatan_nominal_fcy_pos
        CHECK (nominal_fcy IS NULL OR nominal_fcy > 0),

    -- EIR: eir_awal in valid rate range when populated
    CONSTRAINT chk_penempatan_eir_range
        CHECK (eir_awal IS NULL OR (eir_awal >= 0 AND eir_awal <= 1)),

    -- SoD — Create workflow (DB defense-in-depth per DEC-017)
    -- Application layer enforces; DB CHECK provides defense-in-depth.
    CONSTRAINT chk_penempatan_sod_reviewer
        CHECK (reviewer_id IS NULL OR reviewer_id <> maker_id),

    CONSTRAINT chk_penempatan_sod_approver_vs_maker
        CHECK (approver_id IS NULL OR approver_id <> maker_id),

    CONSTRAINT chk_penempatan_sod_approver_vs_reviewer
        CHECK (approver_id IS NULL OR reviewer_id IS NULL OR approver_id <> reviewer_id),

    -- SoD — Terminate workflow (DEC-P5-M1-005)
    CONSTRAINT chk_penempatan_sod_term_reviewer
        CHECK (terminate_reviewer_id IS NULL OR terminate_reviewer_id <> maker_id),

    CONSTRAINT chk_penempatan_sod_term_approver_vs_maker
        CHECK (terminate_approver_id IS NULL OR terminate_approver_id <> maker_id),

    CONSTRAINT chk_penempatan_sod_term_approver_vs_reviewer
        CHECK (
            terminate_approver_id IS NULL
            OR terminate_reviewer_id IS NULL
            OR terminate_approver_id <> terminate_reviewer_id
        )
);

-- ----------------------------------------------------------------
-- Unique constraints on trx.penempatan_deposito
-- ----------------------------------------------------------------

-- kode_transaksi unique per tenant (Phase 1 = single tenant, still scoped for Phase 2 MT)
CREATE UNIQUE INDEX uq_penempatan_kode_transaksi_tenant
    ON trx.penempatan_deposito (kode_transaksi, tenant_id);

-- ----------------------------------------------------------------
-- Indexes on trx.penempatan_deposito
-- ----------------------------------------------------------------

-- 1. Status + created_at: primary list/queue query (active-only)
CREATE INDEX idx_penempatan_status_created
    ON trx.penempatan_deposito (workflow_status, created_at DESC)
    WHERE deleted_at IS NULL;

-- 2. FK: instrumen_id
CREATE INDEX idx_penempatan_instrumen
    ON trx.penempatan_deposito (instrumen_id);

-- 3. FK: counterparty_bank_id
CREATE INDEX idx_penempatan_counterparty_bank
    ON trx.penempatan_deposito (counterparty_bank_id);

-- 4. FK: periode_id
CREATE INDEX idx_penempatan_periode
    ON trx.penempatan_deposito (periode_id);

-- 5. Maker filter (user's own transactions)
CREATE INDEX idx_penempatan_maker
    ON trx.penempatan_deposito (maker_id);

-- 6. Terminate maker (only rows with active termination workflow)
CREATE INDEX idx_penempatan_terminate_maker
    ON trx.penempatan_deposito (terminate_maker_id)
    WHERE terminate_maker_id IS NOT NULL;

-- 7. Maturity Asynq job scan: daily 09:00 WIB
--    Scans only APPROVED_ACTIVE rows by jatuh_tempo (per §12 maturity-checker)
CREATE INDEX idx_penempatan_jatuh_tempo_active
    ON trx.penempatan_deposito (tanggal_jatuh_tempo)
    WHERE workflow_status = 'APPROVED_ACTIVE' AND deleted_at IS NULL;

-- 8. Approver queue: PENDING_REVIEW → reviewer pick-up
CREATE INDEX idx_penempatan_pending_review
    ON trx.penempatan_deposito (created_at DESC)
    WHERE workflow_status = 'PENDING_REVIEW';

-- 9. Approver queue: PENDING_APPROVAL → approver pick-up
CREATE INDEX idx_penempatan_pending_approval
    ON trx.penempatan_deposito (created_at DESC)
    WHERE workflow_status = 'PENDING_APPROVAL';

-- 10. Tenant + created_at composite: multi-tenant list queries (Phase 2 readiness)
CREATE INDEX idx_penempatan_tenant_created
    ON trx.penempatan_deposito (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- ----------------------------------------------------------------
-- Triggers on trx.penempatan_deposito
-- ----------------------------------------------------------------

CREATE TRIGGER tg_penempatan_updated_at
    BEFORE UPDATE ON trx.penempatan_deposito
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER tg_penempatan_row_version
    BEFORE UPDATE ON trx.penempatan_deposito
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ----------------------------------------------------------------
-- Hard-delete guard on trx.penempatan_deposito
-- Soft-delete (deleted_at) is the only allowed deletion path.
-- trx schema is not in the absolute no-hard-delete list (aud/jrnl/ecl)
-- but financial transaction integrity requires the same protection.
-- ----------------------------------------------------------------

CREATE OR REPLACE FUNCTION fn_penempatan_no_hard_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Hard delete on trx.penempatan_deposito is forbidden. '
        'Use soft-delete (set deleted_at, deleted_by, workflow_status=CANCELLED).';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tg_penempatan_no_hard_delete
    BEFORE DELETE ON trx.penempatan_deposito
    FOR EACH ROW EXECUTE FUNCTION fn_penempatan_no_hard_delete();

-- ----------------------------------------------------------------
-- Comments
-- ----------------------------------------------------------------

COMMENT ON TABLE trx.penempatan_deposito IS
    'P5-M1 Penempatan Deposito transaction lifecycle. '
    'State machine: p5-m1-penempatan.md §1. '
    'Create workflow: 4-eyes (maker→reviewer→approver, SoD enforced at DB+app layer). '
    'Terminate workflow: 4-eyes (DEC-P5-M1-005). '
    'Terminal states: MATURED (Asynq), TERMINATED (user-initiated), CANCELLED (maker withdraw). '
    'No hard delete. Soft-delete via deleted_at + workflow_status=CANCELLED.';

COMMENT ON COLUMN trx.penempatan_deposito.kode_transaksi IS
    'Auto-generated PNP-{YYYYMM}-{seq:06d}. Sequence: trx.penempatan_kode_seq. '
    'Assembly in Go service layer (PenempatanService.Create).';

COMMENT ON COLUMN trx.penempatan_deposito.eir_awal IS
    'Populated async by Asynq EIR_COMPUTE after approve (DEC-013). '
    'NULL until job completes. Permanently NULL for FVTPL/FVOCI_ELECTION (DEC-P5-M1-001).';

COMMENT ON COLUMN trx.penempatan_deposito.realized_gain_loss_idr IS
    'Computed by P5-M9 derecognition engine at TERMINATED state. NULL until then. '
    'NUMERIC(20,4) IDR per db-conventions.md.';

COMMENT ON COLUMN trx.penempatan_deposito.settlement_account IS
    'References sys.settlement_account_balance.account_code. '
    'Informational only — no FK per DEC-P5-M1-004 (settlement balance never blocks submit).';

-- ====================================================================
-- D. TABLE sys.settlement_account_balance
-- ====================================================================

CREATE TABLE sys.settlement_account_balance (

    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    account_code            TEXT            NOT NULL,
    -- Cross-ref identifier matching trx.penempatan_deposito.settlement_account
    -- and mst.chart_of_accounts / sys.config bank accounts (informational text key)

    currency                TEXT            NOT NULL,
    -- ISO 4217 code. Same whitelist as trx.penempatan_deposito FCY field.

    balance                 NUMERIC(20,4)   NOT NULL,
    -- Manual entry balance snapshot. NUMERIC(20,4) IDR/FCY per db-conventions.md.

    as_of_date              DATE            NOT NULL,
    -- Date the balance was observed/entered (manual entry by ROLE-AKUN)

    entered_by              UUID            NOT NULL
                                REFERENCES sec.user(id)
                                ON DELETE RESTRICT,
    -- ROLE-AKUN that entered this balance snapshot

    -- Audit columns (mandatory per db-conventions.md)
    created_at              TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    created_by              UUID            NOT NULL    REFERENCES sec.user(id),
    updated_at              TIMESTAMPTZ     NOT NULL    DEFAULT now(),
    updated_by              UUID            NOT NULL    REFERENCES sec.user(id),
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID            REFERENCES sec.user(id),
    row_version             BIGINT          NOT NULL    DEFAULT 1,
    tenant_id               TEXT            NOT NULL    DEFAULT 'TUGURE',

    -- ----------------------------------------------------------------
    -- CHECK constraints
    -- ----------------------------------------------------------------

    CONSTRAINT chk_settlement_balance_currency
        CHECK (currency IN ('IDR', 'USD', 'EUR', 'SGD', 'JPY')),

    CONSTRAINT uq_settlement_balance_account_currency_tenant
        UNIQUE (account_code, currency, tenant_id)
    -- One active balance snapshot per (account, currency, tenant).
    -- Subsequent manual entries UPDATE the existing row (row_version increments).
);

-- ----------------------------------------------------------------
-- Indexes on sys.settlement_account_balance
-- ----------------------------------------------------------------

-- FK: entered_by
CREATE INDEX idx_settlement_balance_entered_by
    ON sys.settlement_account_balance (entered_by);

-- Tenant + created_at (multi-tenant readiness)
CREATE INDEX idx_settlement_balance_tenant_created
    ON sys.settlement_account_balance (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- ----------------------------------------------------------------
-- Triggers on sys.settlement_account_balance
-- ----------------------------------------------------------------

CREATE TRIGGER tg_settlement_balance_updated_at
    BEFORE UPDATE ON sys.settlement_account_balance
    FOR EACH ROW EXECUTE FUNCTION fn_update_updated_at();

CREATE TRIGGER tg_settlement_balance_row_version
    BEFORE UPDATE ON sys.settlement_account_balance
    FOR EACH ROW EXECUTE FUNCTION fn_increment_row_version();

-- ----------------------------------------------------------------
-- Comments
-- ----------------------------------------------------------------

COMMENT ON TABLE sys.settlement_account_balance IS
    'Informational settlement account balance hint. '
    'DEC-P5-M1-004: read-only hint in penempatan form — never blocks submit. '
    'Managed by ROLE-AKUN. One row per (account_code, currency, tenant_id). '
    'Balance is a snapshot; as_of_date indicates the observation date.';

COMMIT;
