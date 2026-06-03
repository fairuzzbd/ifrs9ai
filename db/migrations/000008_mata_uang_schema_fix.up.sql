-- migration: 0008 mata_uang_schema_fix
-- author: data-modeler
-- requires: 0001, 0007
-- description: (1) Backfill mst.mata_uang with missing audit cols, workflow_status,
--              decimal_places, is_system_currency, and UUID surrogate id.
--              (2) Seed WORKFLOW_CONFIG_* rows in sys.config for all Phase-3
--              master entities not yet seeded in 0007.
--              Tables with their own full audit-col set (portofolio, counterparty,
--              chart_of_accounts, instrumen) are NOT touched here — their audit
--              inconsistencies (version vs row_version, is_deleted vs deleted_at)
--              are tracked as separate debt; this migration focuses on mata_uang
--              which is the only mst table that is completely missing audit cols.

BEGIN;

-- ============================================================
-- 1. mst.mata_uang — ADD MISSING COLUMNS
-- ============================================================

-- 1a. Surrogate UUID id (workflow_instance.entity_id references UUID)
--     kode_mata_uang (CHAR 3) remains PK (business key).
--     id is UNIQUE NOT NULL — backfilled with gen_random_uuid() for existing rows.
ALTER TABLE mst.mata_uang
    ADD COLUMN IF NOT EXISTS id UUID;

UPDATE mst.mata_uang SET id = gen_random_uuid() WHERE id IS NULL;

ALTER TABLE mst.mata_uang
    ALTER COLUMN id SET NOT NULL,
    ALTER COLUMN id SET DEFAULT gen_random_uuid();

CREATE UNIQUE INDEX IF NOT EXISTS uq_mata_uang_id ON mst.mata_uang(id);

-- 1b. Audit columns missing from 0001
ALTER TABLE mst.mata_uang
    ADD COLUMN IF NOT EXISTS created_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by  UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id   TEXT   NOT NULL DEFAULT 'TUGURE';

-- 1c. Business columns required by mata-uang.yaml
ALTER TABLE mst.mata_uang
    ADD COLUMN IF NOT EXISTS decimal_places     SMALLINT NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS is_system_currency BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS workflow_status    VARCHAR(30) NOT NULL DEFAULT 'DRAFT';

-- 1d. CHECK constraints
ALTER TABLE mst.mata_uang
    DROP CONSTRAINT IF EXISTS chk_mata_uang_decimal_places,
    DROP CONSTRAINT IF EXISTS chk_mata_uang_workflow_status;

ALTER TABLE mst.mata_uang
    ADD CONSTRAINT chk_mata_uang_decimal_places
        CHECK (decimal_places BETWEEN 0 AND 8),
    ADD CONSTRAINT chk_mata_uang_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- 1e. Seed IDR as system currency (protect functional currency from delete/code change)
UPDATE mst.mata_uang
    SET is_system_currency = TRUE,
        workflow_status    = 'APPROVED'
WHERE kode_mata_uang = 'IDR';

-- 1f. Set all existing rows to APPROVED (they were in use pre-workflow)
UPDATE mst.mata_uang
    SET workflow_status = 'APPROVED'
WHERE workflow_status = 'DRAFT';

-- 1g. Indexes for audit and tenant queries
CREATE INDEX IF NOT EXISTS idx_mata_uang_tenant_created
    ON mst.mata_uang(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_mata_uang_aktif
    ON mst.mata_uang(aktif_flag) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mata_uang_workflow_status
    ON mst.mata_uang(workflow_status) WHERE deleted_at IS NULL;

-- ============================================================
-- 2. sys.config — SEED WORKFLOW_CONFIG_* for Phase-3 entities
--    (ON CONFLICT DO NOTHING — idempotent, safe to re-run)
--    Already seeded in 0007: PENEMPATAN, JURNAL, PERIODE,
--    UPLOAD_BATCH, KLASIFIKASI, ECL_PARAMETER
-- ============================================================

INSERT INTO sys.config (config_key, config_value, config_type, sensitive, description, category)
VALUES

-- ----------------------------------------------------------
-- 4-eyes: MATA_UANG
-- Maker: ROLE-AKUN  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-AKUN-CTL (different user)
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_MATA_UANG',
    '{
        "entityType": "MATA_UANG",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "mata_uang.submit",
            "review":  "mata_uang.review",
            "approve": "mata_uang.approve",
            "reject":  "mata_uang.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.mata_uang — 4-eyes AKUN→AKUN-CTL(review)→AKUN-CTL(approve), SoD 3 distinct users',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: PORTOFOLIO
-- Maker: ROLE-MAKER-TR  Reviewer: ROLE-RISK  Approver: ROLE-APPR-TR
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_PORTOFOLIO',
    '{
        "entityType": "PORTOFOLIO",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "portofolio.submit",
            "review":  "portofolio.review",
            "approve": "portofolio.approve",
            "reject":  "portofolio.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.portofolio — 4-eyes MAKER-TR→RISK(review)→APPR-TR(approve)',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: CHART_OF_ACCOUNTS
-- Maker: ROLE-AKUN  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-AKUN-CTL
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_CHART_OF_ACCOUNTS',
    '{
        "entityType": "CHART_OF_ACCOUNTS",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "chart_of_accounts.submit",
            "review":  "chart_of_accounts.review",
            "approve": "chart_of_accounts.approve",
            "reject":  "chart_of_accounts.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.chart_of_accounts — 4-eyes AKUN→AKUN-CTL(review)→AKUN-CTL(approve)',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: MAPPING_JURNAL (header; detail inherits header workflow)
-- Maker: ROLE-AKUN  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-AKUN-CTL
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_MAPPING_JURNAL',
    '{
        "entityType": "MAPPING_JURNAL",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "mapping_jurnal.submit",
            "review":  "mapping_jurnal.review",
            "approve": "mapping_jurnal.approve",
            "reject":  "mapping_jurnal.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.mapping_jurnal_header (+ detail) — 4-eyes AKUN→AKUN-CTL(review)→AKUN-CTL(approve)',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: KURS (manual override; BI JISDOR feed = Varian C auto-approve)
-- Maker: ROLE-AKUN  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-AKUN-CTL
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_KURS',
    '{
        "entityType": "KURS",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "kurs.submit",
            "review":  "kurs.review",
            "approve": "kurs.approve",
            "reject":  "kurs.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.kurs (manual override path) — 4-eyes AKUN→AKUN-CTL(review)→AKUN-CTL(approve). BI JISDOR feed auto-approves via integration worker.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: COUNTERPARTY (Varian B — PII + security gate)
-- Maker: ROLE-MAKER-TR  Reviewer: ROLE-RISK  Approver: ROLE-APPR-TR
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_COUNTERPARTY',
    '{
        "entityType": "COUNTERPARTY",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "counterparty.submit",
            "review":  "counterparty.review",
            "approve": "counterparty.approve",
            "reject":  "counterparty.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.counterparty — 4-eyes MAKER-TR→RISK(review)→APPR-TR(approve). PII fields: DEC-028, security-engineer BLOCKING gate.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: RATING_HISTORY (child of counterparty; SICR auto-trigger on approve)
-- Maker: ROLE-RISK  Reviewer: ROLE-RISK (different user)  Approver: ROLE-APPR-TR
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_RATING_HISTORY',
    '{
        "entityType": "RATING_HISTORY",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "rating_history.submit",
            "review":  "rating_history.review",
            "approve": "rating_history.approve",
            "reject":  "rating_history.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.rating_history_counterparty — 4-eyes RISK(make)→RISK(review, different user)→APPR-TR(approve). SICR trigger fires on approve.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 4-eyes: INSTRUMEN (most complex — SPPI test triggered on approve)
-- Maker: ROLE-MAKER-TR  Reviewer: ROLE-RISK  Approver: ROLE-APPR-TR
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_INSTRUMEN',
    '{
        "entityType": "INSTRUMEN",
        "eyes": 4,
        "retractable": false,
        "requiredPermissions": {
            "submit":  "instrumen.submit",
            "review":  "instrumen.review",
            "approve": "instrumen.approve",
            "reject":  "instrumen.reject"
        },
        "stepUpRequired": {
            "approve": false
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": false
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.instrumen — 4-eyes MAKER-TR→RISK(review)→APPR-TR(approve). SPPI test triggered on approve.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: PD_PEFINDO (ECL param — Varian A+C)
-- Maker: ROLE-RISK  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-ALCO  Approver2: ROLE-ALCO
-- step-up MFA wajib untuk kedua approval
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_PD_PEFINDO',
    '{
        "entityType": "PD_PEFINDO",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.pd_pefindo (ECL param) — 6-eyes RISK→AKUN-CTL(review)→ALCO(approve)→ALCO2(approve2). Both approvals require step-up MFA (DEC-027). ifrs9-compliance-reviewer BLOCKING gate.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: LGD_BASEL (ECL param)
-- Maker: ROLE-RISK  Reviewer: ROLE-AKUN-CTL  Approver: ROLE-ALCO  Approver2: ROLE-ALCO
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_LGD_BASEL',
    '{
        "entityType": "LGD_BASEL",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.lgd_basel (ECL param) — 6-eyes, both ALCO approvals require step-up MFA (DEC-027). ifrs9-compliance-reviewer BLOCKING gate.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: BOBOT_SKENARIO (ECL param — DEC-010 default 0.25/0.50/0.25)
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_BOBOT_SKENARIO',
    '{
        "entityType": "BOBOT_SKENARIO",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.bobot_skenario (ECL param) — 6-eyes. Default weights 0.25/0.50/0.25 (DEC-010), ALCO can override. Both approvals step-up MFA.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: LPS_COVERAGE (ECL param — DEC-014 IDR 2 miliar cap)
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_LPS_COVERAGE',
    '{
        "entityType": "LPS_COVERAGE",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.lps_coverage (ECL param) — 6-eyes. IDR 2 miliar cap (DEC-014). Both ALCO approvals require step-up MFA.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: IMPACT_MEV_PD (ECL param — dual FL multiplier MEV component)
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_IMPACT_MEV_PD',
    '{
        "entityType": "IMPACT_MEV_PD",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.impact_mev_pd (ECL param) — 6-eyes dual FL multiplier MEV PD. Both ALCO approvals require step-up MFA.',
    'WORKFLOW'
),

-- ----------------------------------------------------------
-- 6-eyes: IMPACT_PD (ECL param — FL multiplier per skenario)
-- ----------------------------------------------------------
(
    'WORKFLOW_CONFIG_IMPACT_PD',
    '{
        "entityType": "IMPACT_PD",
        "eyes": 6,
        "retractable": false,
        "requiredPermissions": {
            "submit":   "ecl_parameter.submit",
            "review":   "ecl_parameter.review",
            "approve":  "ecl_parameter.approve",
            "approve2": "ecl_parameter.approve",
            "reject":   "ecl_parameter.reject"
        },
        "stepUpRequired": {
            "approve":  true,
            "approve2": true
        },
        "sodRules": {
            "reviewerNotMaker": true,
            "approverNotMakerOrReviewer": true,
            "approver2NotAnyPrevious": true
        }
    }',
    'JSON', FALSE,
    'Workflow config for mst.impact_pd (ECL param) — 6-eyes FL multiplier per skenario. Both ALCO approvals require step-up MFA.',
    'WORKFLOW'
)

ON CONFLICT (config_key) DO NOTHING;

COMMIT;
