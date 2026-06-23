-- migration: 000047 POCI Delta ECL (P5-M10)
-- author: ecl-eir-engineer
-- requires: 000046
-- References: PSAK 71 §5.5.13-14, FSD-APP-C-ECL-EIR-v1.0 §5-6, DEC-010/013/016/018

BEGIN;

-- ─── ecl.poci_baseline — WORM (Write Once Read Many) ─────────────────────────
-- One row per POCI instrumen, immutable since origination (DEC-018).
-- No row_version — append-only, no UPDATE ever expected.
-- BEFORE UPDATE OR DELETE trigger enforces WORM at DB layer.

CREATE TABLE ecl.poci_baseline (
    id                          UUID        NOT NULL DEFAULT gen_random_uuid(),
    instrumen_id                UUID        NOT NULL UNIQUE,       -- one baseline per instrument
    tanggal_baseline            DATE        NOT NULL,              -- tanggal penempatan POCI di-approve
    lifetime_ecl_at_origination NUMERIC(20,4) NOT NULL,           -- lifetime ECL (not 12-month) per §5.5.13
    cashflow_expectasi_jsonb    JSONB,                             -- PD-adjusted CFs at origination (optional detail)
    credit_adjusted_eir         NUMERIC(10,8) NOT NULL,           -- credit-adjusted EIR per DEC-013 Newton-Raphson
    origination_date            DATE        NOT NULL,              -- alias tanggal_baseline for clarity
    -- Audit columns (no row_version — immutable)
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by                  UUID        NOT NULL,
    tenant_id                   TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_poci_baseline PRIMARY KEY (id),
    CONSTRAINT chk_poci_baseline_ecl_positive CHECK (lifetime_ecl_at_origination >= 0),
    CONSTRAINT chk_poci_baseline_eir_range CHECK (credit_adjusted_eir > 0 AND credit_adjusted_eir < 1),
    CONSTRAINT chk_poci_baseline_dates CHECK (origination_date <= tanggal_baseline OR origination_date = tanggal_baseline)
);

COMMENT ON TABLE ecl.poci_baseline IS
    'PSAK 71 §5.5.13 — Immutable ECL baseline for POCI instruments at origination. '
    'WORM: no UPDATE or DELETE ever. DEC-018.';

-- WORM trigger — defence-in-depth beyond service layer
CREATE OR REPLACE FUNCTION ecl.trg_poci_baseline_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ecl.poci_baseline is append-only (DEC-018, P5-M10). '
        'Operation % rejected for instrumen_id: %',
        TG_OP, OLD.instrumen_id
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_poci_baseline_no_update_delete
    BEFORE UPDATE OR DELETE ON ecl.poci_baseline
    FOR EACH ROW EXECUTE FUNCTION ecl.trg_poci_baseline_immutable();

-- Index: FK + tenant query pattern
CREATE INDEX idx_poci_baseline_instrumen_id
    ON ecl.poci_baseline (instrumen_id);
CREATE INDEX idx_poci_baseline_tanggal_baseline
    ON ecl.poci_baseline (tanggal_baseline DESC);
CREATE INDEX idx_poci_baseline_tenant
    ON ecl.poci_baseline (tenant_id, created_at DESC);

-- ─── ecl.poci_delta_log — Partitioned monthly by tanggal_compute ─────────────
-- One row per (calc_run_id × instrumen_id). Idempotency via partial unique index.
-- NUMERIC(20,4) for IDR amounts, signed delta_ecl.

CREATE TABLE ecl.poci_delta_log (
    id                      UUID        NOT NULL DEFAULT gen_random_uuid(),
    calc_run_id             UUID        NOT NULL,
    instrumen_id            UUID        NOT NULL,
    tanggal_compute         DATE        NOT NULL,             -- partition key
    baseline_ecl            NUMERIC(20,4) NOT NULL,          -- snapshot from poci_baseline (immutable)
    current_ecl             NUMERIC(20,4) NOT NULL,          -- lifetime ECL from this calc run
    delta_ecl               NUMERIC(20,4) NOT NULL,          -- signed: current - baseline
    direction               TEXT        NOT NULL,
    prior_delta_cumulative  NUMERIC(20,4),                   -- Σ delta all prior runs for this instrumen
    jurnal_header_id        UUID,                            -- populated when status = POSTED
    periode_bulanan_id      UUID,
    status                  TEXT        NOT NULL DEFAULT 'COMPUTED',
    -- Audit
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by              UUID        NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by              UUID        NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT      NOT NULL DEFAULT 1,
    tenant_id               TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT pk_poci_delta_log PRIMARY KEY (id, tanggal_compute),
    CONSTRAINT chk_poci_delta_direction
        CHECK (direction IN ('INCREASE', 'DECREASE', 'ZERO')),
    CONSTRAINT chk_poci_delta_status
        CHECK (status IN ('COMPUTED', 'POSTED', 'SKIPPED_ZERO')),
    -- Business invariant: sign of delta_ecl must match direction
    CONSTRAINT chk_poci_delta_sign_direction CHECK (
        (direction = 'INCREASE' AND delta_ecl >  0) OR
        (direction = 'DECREASE' AND delta_ecl <  0) OR
        (direction = 'ZERO'     AND delta_ecl =  0)
    )
) PARTITION BY RANGE (tanggal_compute);

COMMENT ON TABLE ecl.poci_delta_log IS
    'PSAK 71 §5.5.14 — POCI ECL delta per (calc_run × instrumen) per period. '
    'P&L books only the delta, not full lifetime ECL. Partitioned monthly.';

-- Seed initial partitions (2026)
CREATE TABLE ecl.poci_delta_log_y2026m01 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE ecl.poci_delta_log_y2026m02 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE ecl.poci_delta_log_y2026m03 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE ecl.poci_delta_log_y2026m04 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE ecl.poci_delta_log_y2026m05 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE ecl.poci_delta_log_y2026m06 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE ecl.poci_delta_log_y2026m07 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE ecl.poci_delta_log_y2026m08 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE ecl.poci_delta_log_y2026m09 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE ecl.poci_delta_log_y2026m10 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE ecl.poci_delta_log_y2026m11 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE ecl.poci_delta_log_y2026m12 PARTITION OF ecl.poci_delta_log
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Idempotency: partial unique index prevents duplicate per (run × instrumen)
CREATE UNIQUE INDEX uq_poci_delta_log_run_instrumen
    ON ecl.poci_delta_log (calc_run_id, instrumen_id)
    WHERE deleted_at IS NULL;

-- FK-equivalent indexes (PG does not auto-index FKs)
CREATE INDEX idx_poci_delta_log_instrumen_id
    ON ecl.poci_delta_log (instrumen_id, tanggal_compute DESC);
CREATE INDEX idx_poci_delta_log_calc_run_id
    ON ecl.poci_delta_log (calc_run_id);
CREATE INDEX idx_poci_delta_log_direction
    ON ecl.poci_delta_log (direction, tanggal_compute DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_poci_delta_log_tenant
    ON ecl.poci_delta_log (tenant_id, created_at DESC);

-- Row version trigger (mirrors standard pattern)
CREATE OR REPLACE FUNCTION ecl.trg_poci_delta_log_row_version()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.row_version := OLD.row_version + 1;
    NEW.updated_at  := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_poci_delta_log_row_version
    BEFORE UPDATE ON ecl.poci_delta_log
    FOR EACH ROW EXECUTE FUNCTION ecl.trg_poci_delta_log_row_version();

-- ─── Seed mst.mapping_jurnal placeholders ────────────────────────────────────
-- DRAFT rows — akun debit/kredit kosong until P5-M12 fills them.
-- Assumes mst.mapping_jurnal table exists from P5-M2 migration.

INSERT INTO mst.mapping_jurnal (
    id, event_code, klasifikasi_psak71, workflow_status,
    description, akun_debit_id, akun_kredit_id,
    created_at, created_by, updated_at, updated_by, tenant_id
)
VALUES
    (
        gen_random_uuid(),
        'POCI_ECL_DELTA_INCREASE',
        NULL,      -- berlaku untuk semua klasifikasi POCI (AC / FVOCI debt)
        'DRAFT',
        'POCI ECL Delta INCREASE: D Beban Penurunan Nilai ECL POCI / K Cadangan ECL POCI. '
        'Akun diisi oleh ROLE-AKUN di P5-M12. PSAK 71 §5.5.14.',
        NULL,      -- akun_debit_id: TBD P5-M12
        NULL,      -- akun_kredit_id: TBD P5-M12
        now(), '00000000-0000-0000-0000-000000000001',
        now(), '00000000-0000-0000-0000-000000000001',
        'TUGURE'
    ),
    (
        gen_random_uuid(),
        'POCI_ECL_DELTA_DECREASE',
        NULL,
        'DRAFT',
        'POCI ECL Delta DECREASE: D Cadangan ECL POCI / K Pendapatan Pemulihan ECL POCI. '
        'Akun diisi oleh ROLE-AKUN di P5-M12. PSAK 71 §5.5.14.',
        NULL,
        NULL,
        now(), '00000000-0000-0000-0000-000000000001',
        now(), '00000000-0000-0000-0000-000000000001',
        'TUGURE'
    )
ON CONFLICT DO NOTHING;

COMMIT;
