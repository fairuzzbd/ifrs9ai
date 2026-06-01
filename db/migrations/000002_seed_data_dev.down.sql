-- migration: 0002 seed_data_dev (DOWN)
-- author: data-modeler
-- requires: 0001
-- description: Reverse dev/UAT seed — deletes in reverse FK dependency order.
--              Does NOT drop schemas (owned by migration 0001).
--              Safe to run via cmd/migrator down; postgres entrypoint never runs .down.sql.

BEGIN;

-- ====================================================================
-- Reverse order of FK dependency:
--   kurs → periode_buku → rating_history → counterparty → portofolio
--   → user_role (sample) → user (sample) → CoA → pd_pefindo
--   → lookup → mata_uang → lps_coverage → bobot_skenario
--   → lgd_basel → role → user (bootstrap)
-- ====================================================================

-- 12.10.6  FX Rate sample
DELETE FROM mst.kurs
WHERE fx_rate_id_kode = 'FX-USD-20260101';

-- 12.10.5  Periode Buku 2026
DELETE FROM mst.periode_buku
WHERE periode_id_kode IN (
    'PRD-2026-01','PRD-2026-02','PRD-2026-03','PRD-2026-04',
    'PRD-2026-05','PRD-2026-06','PRD-2026-07','PRD-2026-08',
    'PRD-2026-09','PRD-2026-10','PRD-2026-11','PRD-2026-12',
    'PRD-2026-Q1','PRD-2026-Q2','PRD-2026-Q3','PRD-2026-Q4',
    'PRD-2026'
);

-- 12.10.3  Rating History
DELETE FROM mst.rating_history_counterparty
WHERE rating_history_id_kode IN (
    'RTH-2026-00001','RTH-2026-00002','RTH-2026-00003','RTH-2026-00004',
    'RTH-2026-00005','RTH-2026-00010','RTH-2026-00011','RTH-2026-00012',
    'RTH-2026-00013','RTH-2026-00014','RTH-2026-00015'
);

-- 12.10.3  Counterparty
DELETE FROM mst.counterparty
WHERE kode_counterparty IN (
    'CP-0001','CP-0002','CP-0003','CP-0004','CP-0005','CP-0006',
    'CP-0010','CP-0011','CP-0012','CP-0013','CP-0014','CP-0015',
    'CP-0020','CP-0021','CP-0022','CP-0023',
    'CP-0030','CP-0031','CP-0032',
    'CP-0040','CP-0041'
);

-- 12.10.4  Portofolio
DELETE FROM mst.portofolio
WHERE kode_portofolio IN (
    'PORT-TR-LIQ','PORT-INV-LT','PORT-INV-LIQ','PORT-TRADING','PORT-STRATEGIC'
);

-- 12.10.1  User Role assignments (sample persona)
DELETE FROM sec.user_role
WHERE user_id IN (
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000011',
    '00000000-0000-0000-0000-000000000012',
    '00000000-0000-0000-0000-000000000013',
    '00000000-0000-0000-0000-000000000014',
    '00000000-0000-0000-0000-000000000015',
    '00000000-0000-0000-0000-000000000016',
    '00000000-0000-0000-0000-000000000017',
    '00000000-0000-0000-0000-000000000018',
    '00000000-0000-0000-0000-000000000019'
);

-- 12.10.1  Sample persona users (10 row)
DELETE FROM sec.user
WHERE id IN (
    '00000000-0000-0000-0000-000000000010',
    '00000000-0000-0000-0000-000000000011',
    '00000000-0000-0000-0000-000000000012',
    '00000000-0000-0000-0000-000000000013',
    '00000000-0000-0000-0000-000000000014',
    '00000000-0000-0000-0000-000000000015',
    '00000000-0000-0000-0000-000000000016',
    '00000000-0000-0000-0000-000000000017',
    '00000000-0000-0000-0000-000000000018',
    '00000000-0000-0000-0000-000000000019'
);

-- 12.10.2  Chart of Accounts (extended set)
DELETE FROM mst.chart_of_accounts
WHERE kode_akun IN (
    '1.1.1.001','1.1.1.002','1.1.1.003','1.1.1.004','1.1.1.010',
    '1.1.2.001','1.1.2.002','1.1.2.003','1.1.2.004','1.1.2.005','1.1.2.006',
    '1.1.3.001','1.1.3.002','1.1.3.003','1.1.3.004','1.1.3.005','1.1.3.010',
    '1.1.4.001','1.1.4.002','1.1.4.003','1.1.4.004','1.1.4.005','1.1.4.006','1.1.4.007',
    '1.1.9.001','1.1.9.003','1.1.9.004','1.1.9.005',
    '3.2.1.001','3.2.1.002','3.2.1.003','3.2.1.004',
    '4.1.1.001','4.1.1.002','4.1.1.003','4.1.1.004',
    '4.1.2.001','4.1.2.002',
    '4.1.3.001','4.1.3.002','4.1.3.003',
    '4.1.4.001','4.1.4.002',
    '5.1.1.001','5.1.1.002',
    '5.1.2.001','5.1.2.002','5.1.2.003',
    '5.1.3.001'
);

-- 12.8  PD Pefindo
DELETE FROM mst.pd_pefindo
WHERE sumber IN ('PEFINDO_DS_2007_2025_AppendixA2','PEFINDO_DS_2007_2025_AppendixA2_LIMITED_POPULATION')
  AND periode_berlaku_dari = '2026-01-01';

-- 12.7  Lookup Data
DELETE FROM sys.lookup
WHERE lookup_group IN (
    'TIPE_INSTRUMEN','SUBTIPE_CASH','SUBTIPE_DEPOSITO','SUBTIPE_OBLIGASI',
    'SUBTIPE_SAHAM','SUBTIPE_REKSADANA','KLASIFIKASI_PSAK71','RATING_PEFINDO'
);

-- 12.6  Mata Uang
DELETE FROM mst.mata_uang
WHERE kode_mata_uang IN ('IDR','USD','SGD','EUR','JPY','AUD','CNY','GBP');

-- 12.5  LPS Coverage
DELETE FROM mst.lps_coverage
WHERE periode_berlaku_dari = '2026-01-01'
  AND maker_id = '00000000-0000-0000-0000-000000000001';

-- 12.4  Bobot Skenario
DELETE FROM mst.bobot_skenario
WHERE periode_berlaku_dari = '2026-01-01'
  AND maker_id = '00000000-0000-0000-0000-000000000001';

-- 12.3  LGD Basel
DELETE FROM mst.lgd_basel
WHERE periode_berlaku_dari = '2026-01-01'
  AND maker_id = '00000000-0000-0000-0000-000000000001';

-- 12.2  Roles
DELETE FROM sec.role
WHERE role_code IN (
    'ROLE-MAKER-TR','ROLE-APPR-TR','ROLE-RISK','ROLE-AKUN','ROLE-AKUN-CTL',
    'ROLE-CFO','ROLE-AUDIT','ROLE-IT-ADMIN','ROLE-KOMITE','ROLE-ALCO'
);

-- 12.1  Bootstrap users (last — admin before system because of FK created_by)
DELETE FROM sec.user WHERE id = '00000000-0000-0000-0000-000000000002';
DELETE FROM sec.user WHERE id = '00000000-0000-0000-0000-000000000001';

COMMIT;
