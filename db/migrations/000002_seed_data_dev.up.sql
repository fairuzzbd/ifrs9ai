-- migration: 0002 seed_data_dev
-- author: data-modeler
-- requires: 0001
-- description: Dev/UAT seed (bootstrap user, 10 role, 8 mata_uang, LGD/bobot/LPS, 18 rating lookup, 8 PD Pefindo aktual, 50+ CoA, 20 counterparty, 5 portofolio, periode 2026, sample FX). PRODUCTION HARUS REPLACE dengan data Tugure aktual.

BEGIN;

-- ====================================================================
-- 12.1  BOOTSTRAP USERS  (wajib pertama — FK anchor untuk semua tabel)
-- ====================================================================

INSERT INTO sec.user (id, username, email, full_name, status, created_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'system', 'system@blips.tugu-re.com', 'System User', 'AKTIF', now())
ON CONFLICT (username) DO NOTHING;

INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
VALUES ('00000000-0000-0000-0000-000000000002', 'admin@tugu-re.com', 'admin@tugu-re.com', 'IT Admin Bootstrap', 'AKTIF', now(),
        '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;

-- ====================================================================
-- 12.2  ROLES  (10 row — gate test: SELECT count(*) FROM sec.role = 10)
-- ====================================================================

INSERT INTO sec.role (role_code, nama_role, deskripsi) VALUES
('ROLE-MAKER-TR',  'Treasury Maker',       'Input transaksi treasury'),
('ROLE-APPR-TR',   'Treasury Approver',    'Approve transaksi maker'),
('ROLE-RISK',      'Risk Officer',         'Master parameter risiko & ECL review'),
('ROLE-AKUN',      'Akuntansi',            'Posting jurnal & periode buku'),
('ROLE-AKUN-CTL',  'Finance Controller',   'Approve adjustment & soft-close'),
('ROLE-CFO',       'CFO',                  'Hard-close approver & critical override'),
('ROLE-AUDIT',     'Auditor (Read-Only)',   'Audit trail & dokumen view'),
('ROLE-IT-ADMIN',  'IT Admin',             'User management'),
('ROLE-KOMITE',    'Komite Investasi',      'Approve klasifikasi PSAK 71'),
('ROLE-ALCO',      'ALCO Member',          'Approve ECL parameter')
ON CONFLICT (role_code) DO NOTHING;

-- ====================================================================
-- 12.3  LGD BASEL (4 row)
-- ====================================================================

INSERT INTO mst.lgd_basel (tipe_eksposur, lgd, karakteristik, periode_berlaku_dari, sumber, maker_id)
VALUES
('SOVEREIGN',        0.4500, 'SUN, SBN, Obligasi Pemerintah',                              '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SENIOR_SECURED',   0.2500, 'Obligasi dengan jaminan aktiva spesifik',                    '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SENIOR_UNSECURED', 0.4500, 'Cash bank, deposito, obligasi korporasi tanpa jaminan',      '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001'),
('SUBORDINATED',     0.7500, 'Obligasi/sukuk subordinasi',                                 '2026-01-01', 'BASEL_III_IRB', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.4  BOBOT SKENARIO  (DEC-010: GOOD=0.25 / NORMAL=0.50 / BAD=0.25)
-- ====================================================================

INSERT INTO mst.bobot_skenario (skenario, bobot, periode_berlaku_dari, maker_id)
VALUES
('GOOD',   0.2500, '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('NORMAL', 0.5000, '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('BAD',    0.2500, '2026-01-01', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.5  LPS COVERAGE  (DEC-014: IDR 2 miliar)
-- ====================================================================

INSERT INTO mst.lps_coverage (coverage_amount, periode_berlaku_dari, regulasi_referensi, maker_id)
VALUES (2000000000.00, '2026-01-01', 'POJK No. 03/POJK.05/2017 tentang LPS', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.6  MATA UANG  (8 row — gate test: SELECT count(*) FROM mst.mata_uang = 8)
-- ====================================================================

INSERT INTO mst.mata_uang (kode_mata_uang, nama_mata_uang, simbol, sumber_kurs_default, frekuensi_update, tanggal_mulai_aktif)
VALUES
('IDR', 'Rupiah Indonesia',   'Rp', 'INTERNAL',         'HARIAN', '2026-01-01'),
('USD', 'US Dollar',          '$',  'BI_JISDOR',        'HARIAN', '2026-01-01'),
('SGD', 'Singapore Dollar',   'S$', 'BI_KURS_TENGAH',   'HARIAN', '2026-01-01'),
('EUR', 'Euro',               '€',  'BI_KURS_TENGAH',   'HARIAN', '2026-01-01'),
('JPY', 'Japanese Yen',       '¥',  'BI_KURS_TENGAH',   'HARIAN', '2026-01-01'),
('AUD', 'Australian Dollar',  'A$', 'BI_KURS_TENGAH',   'HARIAN', '2026-01-01'),
('CNY', 'Chinese Yuan',       '¥',  'BI_KURS_TENGAH',   'HARIAN', '2026-01-01'),
('GBP', 'British Pound',      '£',  'BI_KURS_TENGAH',   'HARIAN', '2026-01-01')
ON CONFLICT (kode_mata_uang) DO NOTHING;

-- ====================================================================
-- 12.7  LOOKUP DATA
-- ====================================================================

INSERT INTO sys.lookup (lookup_group, lookup_key, lookup_value, sort_order) VALUES
-- TIPE_INSTRUMEN (5)
('TIPE_INSTRUMEN', 'CASH',       'Cash di Bank',  1),
('TIPE_INSTRUMEN', 'DEPOSITO',   'Deposito',      2),
('TIPE_INSTRUMEN', 'OBLIGASI',   'Obligasi',      3),
('TIPE_INSTRUMEN', 'SAHAM',      'Saham',         4),
('TIPE_INSTRUMEN', 'REKSADANA',  'Reksadana',     5),
-- SUBTIPE_CASH (2)
('SUBTIPE_CASH', 'GIRO',      'Giro',      1),
('SUBTIPE_CASH', 'TABUNGAN',  'Tabungan',  2),
-- SUBTIPE_DEPOSITO (2)
('SUBTIPE_DEPOSITO', 'BERJANGKA', 'Deposito Berjangka', 1),
('SUBTIPE_DEPOSITO', 'ON_CALL',   'Deposito On-Call',   2),
-- SUBTIPE_OBLIGASI (4)
('SUBTIPE_OBLIGASI', 'NEGARA',          'Obligasi Negara (SUN/SBN/ORI)',                      1),
('SUBTIPE_OBLIGASI', 'KORPORASI',       'Obligasi Korporasi',                                  2),
('SUBTIPE_OBLIGASI', 'SUKUK_NEGARA',    'Sukuk Negara (SR/ST/PBS)',                            3),
('SUBTIPE_OBLIGASI', 'SUKUK_KORPORASI', 'Sukuk Korporasi (Ijarah/Mudharabah/Wakalah)',        4),
-- SUBTIPE_SAHAM (4)
('SUBTIPE_SAHAM', 'LQ45',              'Saham LQ45',                1),
('SUBTIPE_SAHAM', 'IDX30',             'Saham IDX30',               2),
('SUBTIPE_SAHAM', 'NON_LQ45',          'Saham di luar LQ45/IDX30', 3),
('SUBTIPE_SAHAM', 'PAPAN_PENGEMBANGAN','Saham Papan Pengembangan',  4),
-- SUBTIPE_REKSADANA (5)
('SUBTIPE_REKSADANA', 'PENDAPATAN_TETAP', 'Reksadana Pendapatan Tetap', 1),
('SUBTIPE_REKSADANA', 'CAMPURAN',         'Reksadana Campuran',          2),
('SUBTIPE_REKSADANA', 'SAHAM',            'Reksadana Saham',             3),
('SUBTIPE_REKSADANA', 'PASAR_UANG',       'Reksadana Pasar Uang',        4),
('SUBTIPE_REKSADANA', 'ETF',              'Exchange-Traded Fund (ETF)',   5),
-- KLASIFIKASI_PSAK71 (4)
('KLASIFIKASI_PSAK71', 'AC',             'Amortized Cost',                        1),
('KLASIFIKASI_PSAK71', 'FVOCI',          'Fair Value through OCI',                2),
('KLASIFIKASI_PSAK71', 'FVOCI_ELECTION', 'FVOCI Election (Equity Irrevocable)',   3),
('KLASIFIKASI_PSAK71', 'FVTPL',          'Fair Value through Profit/Loss',        4),
-- RATING_PEFINDO (18)
('RATING_PEFINDO', 'idAAA',  'idAAA - Highest grade',          1),
('RATING_PEFINDO', 'idAA+',  'idAA+',                           2),
('RATING_PEFINDO', 'idAA',   'idAA',                            3),
('RATING_PEFINDO', 'idAA-',  'idAA-',                           4),
('RATING_PEFINDO', 'idA+',   'idA+',                            5),
('RATING_PEFINDO', 'idA',    'idA',                             6),
('RATING_PEFINDO', 'idA-',   'idA-',                            7),
('RATING_PEFINDO', 'idBBB+', 'idBBB+ - Lower investment grade', 8),
('RATING_PEFINDO', 'idBBB',  'idBBB',                           9),
('RATING_PEFINDO', 'idBBB-', 'idBBB-',                         10),
('RATING_PEFINDO', 'idBB+',  'idBB+ - Speculative',            11),
('RATING_PEFINDO', 'idBB',   'idBB',                           12),
('RATING_PEFINDO', 'idBB-',  'idBB-',                          13),
('RATING_PEFINDO', 'idB+',   'idB+ - Highly speculative',      14),
('RATING_PEFINDO', 'idB',    'idB',                            15),
('RATING_PEFINDO', 'idB-',   'idB-',                           16),
('RATING_PEFINDO', 'idCCC',  'idCCC - Substantial risk',       17),
('RATING_PEFINDO', 'idD',    'idD - Default',                  18)
ON CONFLICT (lookup_group, lookup_key) DO NOTHING;

-- ====================================================================
-- 12.8  PD PEFINDO  (8 row — aktual dari Pefindo Annual Default Study 2007-2025)
--       Source: Pefindo_Annual_Default_Study_2007-2025_EN.pdf, Appendix 2
--       Survival Pool Cumulative Average Default Rate.
--       JANGAN ubah angka — sudah audit-ready.
-- ====================================================================

INSERT INTO mst.pd_pefindo (rating, pd_12month, pd_lifetime_3y, pd_lifetime_5y, pd_lifetime_7y, pd_lifetime_10y,
                             sumber, tanggal_publikasi, periode_berlaku_dari, uploaded_by)
VALUES
('idAAA', 0.0000, 0.0000, 0.0000, 0.0000, 0.0000, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idAA',  0.0000, 0.0000, 0.0020, 0.0020, 0.0020, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idA',   0.0031, 0.0290, 0.0549, 0.0549, 0.0549, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idBBB', 0.0567, 0.1734, 0.1866, 0.1934, 0.1934, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idBB',  0.5008, 0.5683, 0.5683, 0.5683, 0.5683, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idB',   0.0000, 0.0000, 0.0000, 0.0000, 0.0000, 'PEFINDO_DS_2007_2025_AppendixA2_LIMITED_POPULATION', '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idCCC', 0.0939, 0.6633, 0.6633, 0.6633, 0.6633, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('idD',   1.0000, 1.0000, 1.0000, 1.0000, 1.0000, 'PEFINDO_DS_2007_2025_AppendixA2',                '2026-04-14', '2026-01-01', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.10.2  CHART OF ACCOUNTS — Extended version (50+ row, sample Tugure)
--          Pakai versi extended 12.10.2 (bukan singkat 12.9).
--          ON CONFLICT (kode_akun) DO NOTHING untuk idempotency.
-- ====================================================================

INSERT INTO mst.chart_of_accounts (kode_akun, nama_akun, tipe_akun, sub_tipe_akun, kategori_investasi, posisi_normal, sumber_coa, tanggal_mulai_aktif, created_by)
VALUES
-- ASET LANCAR — Cash & Bank
('1.1.1.001', 'Kas - Bank Mandiri (IDR)',           'ASET', 'LANCAR',        NULL,        'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.002', 'Kas - Bank BCA (IDR)',               'ASET', 'LANCAR',        NULL,        'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.003', 'Kas - Bank BNI (IDR)',               'ASET', 'LANCAR',        NULL,        'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.004', 'Kas - Bank BRI (IDR)',               'ASET', 'LANCAR',        NULL,        'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.1.010', 'Kas Bank USD - Bank Mandiri',        'ASET', 'LANCAR',        NULL,        'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga AC
('1.1.2.001', 'Surat Berharga AC - Obligasi Negara',     'ASET', 'TIDAK_LANCAR', 'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.002', 'Surat Berharga AC - Obligasi Korporasi',  'ASET', 'TIDAK_LANCAR', 'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.003', 'Surat Berharga AC - Sukuk Negara',        'ASET', 'TIDAK_LANCAR', 'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.004', 'Surat Berharga AC - Sukuk Korporasi',     'ASET', 'TIDAK_LANCAR', 'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.005', 'Surat Berharga AC - Deposito Berjangka',  'ASET', 'LANCAR',       'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.2.006', 'Surat Berharga AC - Deposito On-Call',    'ASET', 'LANCAR',       'AC',   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga FVOCI
('1.1.3.001', 'Surat Berharga FVOCI - Obligasi Negara',           'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.002', 'Surat Berharga FVOCI - Obligasi Korporasi',        'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.003', 'Surat Berharga FVOCI - Sukuk Negara',              'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.004', 'Surat Berharga FVOCI - Sukuk Korporasi',           'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.005', 'Surat Berharga FVOCI - Reksadana',                 'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.3.010', 'Surat Berharga FVOCI Election - Saham Strategis',  'ASET', 'TIDAK_LANCAR', 'FVOCI', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Surat Berharga FVTPL
('1.1.4.001', 'Surat Berharga FVTPL - Saham',                          'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.002', 'Surat Berharga FVTPL - Reksadana Pasar Uang',           'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.003', 'Surat Berharga FVTPL - Reksadana Pendapatan Tetap',     'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.004', 'Surat Berharga FVTPL - Reksadana Saham',                'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.005', 'Surat Berharga FVTPL - Reksadana Campuran',             'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.006', 'Surat Berharga FVTPL - ETF',                            'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.4.007', 'Surat Berharga FVTPL - Obligasi Trading',               'ASET', 'LANCAR', 'FVTPL', 'DEBIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- Akrual & CKPN
('1.1.9.001', 'CKPN - Surat Berharga AC',    'ASET', 'KONTRA', 'CKPN', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.003', 'Akrual Bunga - Deposito',     'ASET', 'LANCAR',  NULL,   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.004', 'Akrual Kupon - Obligasi',     'ASET', 'LANCAR',  NULL,   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('1.1.9.005', 'Akrual Bunga - Cash Bank',    'ASET', 'LANCAR',  NULL,   'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- EKUITAS - OCI
('3.2.1.001', 'OCI - Selisih MTM FVOCI Obligasi',              'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.002', 'OCI - Selisih MTM FVOCI Saham (Election)',      'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.003', 'OCI - CKPN FVOCI (Memo)',                       'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('3.2.1.004', 'OCI - Selisih MTM FVOCI Reksadana',             'EKUITAS', 'OCI', 'OCI_FVOCI', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- PENDAPATAN
('4.1.1.001', 'Pendapatan Bunga - Deposito',                        'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.002', 'Pendapatan Kupon - Obligasi',                        'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.003', 'Pendapatan Bagi Hasil - Sukuk',                      'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.1.004', 'Pendapatan Bunga - Cash Bank',                       'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.2.001', 'Pendapatan Dividen',                                 'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.2.002', 'Pendapatan Distribusi Reksadana',                    'PENDAPATAN', 'OPERASIONAL',     NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.001', 'Realized Gain/Loss - Penjualan SB',                  'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.002', 'Unrealized Gain/Loss - MTM FVTPL',                   'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.3.003', 'Realized Gain/Loss - Reklasifikasi OCI ke P&L',      'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.4.001', 'Realized FX Gain/Loss',                              'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('4.1.4.002', 'Unrealized FX Gain/Loss',                            'PENDAPATAN', 'NON_OPERASIONAL', NULL, 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
-- BEBAN
('5.1.1.001', 'Beban CKPN - Surat Berharga',               'BEBAN', 'OPERASIONAL', 'CKPN', 'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.1.002', 'Pemulihan Beban CKPN',                      'BEBAN', 'OPERASIONAL', 'CKPN', 'KREDIT', 'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.001', 'Beban PPh Final - Bunga Deposito (20%)',    'BEBAN', 'OPERASIONAL',  NULL,  'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.002', 'Beban PPh Final - Kupon Obligasi (10%)',    'BEBAN', 'OPERASIONAL',  NULL,  'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.2.003', 'Beban PPh Final - Dividen (10%)',           'BEBAN', 'OPERASIONAL',  NULL,  'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('5.1.3.001', 'Beban Komisi - Transaksi Investasi',        'BEBAN', 'OPERASIONAL',  NULL,  'DEBIT',  'INTERNAL', '2026-01-01', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (kode_akun) DO NOTHING;

-- ====================================================================
-- 12.10.1  SAMPLE USERS (10 persona) + USER_ROLE ASSIGNMENT
-- ====================================================================

INSERT INTO sec.user (id, username, email, full_name, unit_kerja, jabatan, status, mfa_enrolled, created_by)
VALUES
('00000000-0000-0000-0000-000000000010', 'treasury.maker@tugu-re.com',    'treasury.maker@tugu-re.com',    'Andi Treasury Maker (Sample)',          'Direktorat Investasi & Treasury', 'Treasury Officer',   'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000011', 'treasury.approver@tugu-re.com', 'treasury.approver@tugu-re.com', 'Budi Treasury Manager (Sample)',         'Direktorat Investasi & Treasury', 'Treasury Manager',   'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000012', 'risk.officer@tugu-re.com',      'risk.officer@tugu-re.com',      'Citra Risk Officer (Sample)',            'Direktorat Risk Management',      'Risk Officer',       'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000013', 'akuntansi@tugu-re.com',         'akuntansi@tugu-re.com',         'Dewi Akuntansi (Sample)',               'Direktorat Keuangan',             'Senior Accountant',  'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000014', 'finance.controller@tugu-re.com','finance.controller@tugu-re.com','Eko Finance Controller (Sample)',        'Direktorat Keuangan',             'Finance Controller', 'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000015', 'cfo@tugu-re.com',               'cfo@tugu-re.com',               'Fauzi CFO (Sample)',                    'Board of Directors',              'CFO',                'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000016', 'auditor@tugu-re.com',           'auditor@tugu-re.com',           'Gita Internal Auditor (Sample)',        'Internal Audit',                  'Auditor',            'AKTIF', FALSE, '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000017', 'komite.investasi@tugu-re.com',  'komite.investasi@tugu-re.com',  'Hadi Komite Investasi Chair (Sample)',  'Komite Investasi',                'Komite Chair',       'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000018', 'alco@tugu-re.com',              'alco@tugu-re.com',              'Indri ALCO Member (Sample)',            'ALCO / Komite Risiko',            'ALCO Member',        'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001'),
('00000000-0000-0000-0000-000000000019', 'it.admin@tugu-re.com',          'it.admin@tugu-re.com',          'Joko IT Admin (Sample)',               'Direktorat Teknologi Informasi',  'IT Admin',           'AKTIF', TRUE,  '00000000-0000-0000-0000-000000000001')
ON CONFLICT (username) DO NOTHING;

-- Assign each persona user to its corresponding role (idempotent via partial unique index on aktif_flag=TRUE)
INSERT INTO sec.user_role (user_id, role_id, assigned_by)
SELECT u.id, r.id, '00000000-0000-0000-0000-000000000001'
FROM sec.user u
JOIN sec.role r ON (
    (u.username = 'treasury.maker@tugu-re.com'    AND r.role_code = 'ROLE-MAKER-TR')  OR
    (u.username = 'treasury.approver@tugu-re.com' AND r.role_code = 'ROLE-APPR-TR')   OR
    (u.username = 'risk.officer@tugu-re.com'      AND r.role_code = 'ROLE-RISK')      OR
    (u.username = 'akuntansi@tugu-re.com'         AND r.role_code = 'ROLE-AKUN')      OR
    (u.username = 'finance.controller@tugu-re.com'AND r.role_code = 'ROLE-AKUN-CTL')  OR
    (u.username = 'cfo@tugu-re.com'               AND r.role_code = 'ROLE-CFO')       OR
    (u.username = 'auditor@tugu-re.com'           AND r.role_code = 'ROLE-AUDIT')     OR
    (u.username = 'komite.investasi@tugu-re.com'  AND r.role_code = 'ROLE-KOMITE')    OR
    (u.username = 'alco@tugu-re.com'              AND r.role_code = 'ROLE-ALCO')      OR
    (u.username = 'it.admin@tugu-re.com'          AND r.role_code = 'ROLE-IT-ADMIN')
)
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.10.3  COUNTERPARTY (~21 row) + RATING HISTORY (11 row)
-- ====================================================================

INSERT INTO mst.counterparty (id, kode_counterparty, nama, tipe, rating_pefindo_current, tipe_eksposur_basel, eligible_lps_flag, status, created_by)
VALUES
-- Pemerintah RI (Sovereign)
('11111111-0000-0000-0000-000000000001', 'CP-0001', 'Pemerintah Republik Indonesia',                         'PEMERINTAH',       NULL,    'SOVEREIGN',        FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Bank Besar (Cash + Deposito + LPS eligible)
('11111111-0000-0000-0000-000000000002', 'CP-0002', 'PT Bank Mandiri (Persero) Tbk',                        'BANK',             'idAAA', 'SENIOR_UNSECURED', TRUE,  'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000003', 'CP-0003', 'PT Bank Central Asia Tbk',                             'BANK',             'idAAA', 'SENIOR_UNSECURED', TRUE,  'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000004', 'CP-0004', 'PT Bank Negara Indonesia (Persero) Tbk',               'BANK',             'idAAA', 'SENIOR_UNSECURED', TRUE,  'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000005', 'CP-0005', 'PT Bank Rakyat Indonesia (Persero) Tbk',               'BANK',             'idAAA', 'SENIOR_UNSECURED', TRUE,  'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000006', 'CP-0006', 'PT Bank CIMB Niaga Tbk',                              'BANK',             'idAAA', 'SENIOR_UNSECURED', TRUE,  'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Korporasi Issuer (Obligasi)
('11111111-0000-0000-0000-000000000010', 'CP-0010', 'PT Telkom Indonesia (Persero) Tbk',                   'KORPORASI',        'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000011', 'CP-0011', 'PT Perusahaan Listrik Negara (Persero)',              'KORPORASI',        'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000012', 'CP-0012', 'PT Jasa Marga (Persero) Tbk',                        'KORPORASI',        'idAA',  'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000013', 'CP-0013', 'PT Indosat Tbk',                                     'KORPORASI',        'idAA',  'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000014', 'CP-0014', 'PT Adhi Karya (Persero) Tbk',                        'KORPORASI',        'idA',   'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000015', 'CP-0015', 'PT Pegadaian (Persero)',                              'KORPORASI',        'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Manajer Investasi (Reksadana)
('11111111-0000-0000-0000-000000000020', 'CP-0020', 'PT Schroder Investment Management Indonesia',        'MANAJER_INVESTASI','idAA',  'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000021', 'CP-0021', 'PT Bahana TCW Investment Management',               'MANAJER_INVESTASI','idAA',  'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000022', 'CP-0022', 'PT Mandiri Manajemen Investasi',                    'MANAJER_INVESTASI','idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000023', 'CP-0023', 'PT BNP Paribas Asset Management',                   'MANAJER_INVESTASI','idAA',  'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Bank Kustodian
('11111111-0000-0000-0000-000000000030', 'CP-0030', 'Standard Chartered Bank - Custody Services',        'BANK_KUSTODIAN',   'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000031', 'CP-0031', 'Citibank N.A. - Custody',                           'BANK_KUSTODIAN',   'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000032', 'CP-0032', 'PT Bank Mandiri (Persero) Tbk - Custody Division', 'BANK_KUSTODIAN',   'idAAA', 'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
-- Emiten Saham (FVOCI Election)
('11111111-0000-0000-0000-000000000040', 'CP-0040', 'PT Bank Central Asia Tbk (BBCA)',   'EMITEN_SAHAM',     NULL,    'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001'),
('11111111-0000-0000-0000-000000000041', 'CP-0041', 'PT Astra International Tbk (ASII)', 'EMITEN_SAHAM',     NULL,    'SENIOR_UNSECURED', FALSE, 'AKTIF', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- Rating History (initial per counterparty yang punya rating)
-- maker_id = risk.officer user (…0012) — sesuai persona §12.10.1
INSERT INTO mst.rating_history_counterparty
    (rating_history_id_kode, counterparty_id, tanggal_berlaku, rating_pefindo, rating_outlook,
     sumber_rating, tanggal_publikasi_rating, action_type, notch_change, maker_id)
VALUES
('RTH-2026-00001', '11111111-0000-0000-0000-000000000002', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00002', '11111111-0000-0000-0000-000000000003', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00003', '11111111-0000-0000-0000-000000000004', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00004', '11111111-0000-0000-0000-000000000005', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00005', '11111111-0000-0000-0000-000000000006', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00010', '11111111-0000-0000-0000-000000000010', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00011', '11111111-0000-0000-0000-000000000011', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00012', '11111111-0000-0000-0000-000000000012', '2026-01-01', 'idAA',  'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00013', '11111111-0000-0000-0000-000000000013', '2026-01-01', 'idAA',  'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00014', '11111111-0000-0000-0000-000000000014', '2026-01-01', 'idA',   'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012'),
('RTH-2026-00015', '11111111-0000-0000-0000-000000000015', '2026-01-01', 'idAAA', 'STABLE', 'PEFINDO_REGULAR', '2025-12-15', 'INITIAL', 0, '00000000-0000-0000-0000-000000000012')
ON CONFLICT (rating_history_id_kode) DO NOTHING;

-- ====================================================================
-- 12.10.4  PORTOFOLIO (5 row)
-- ====================================================================

INSERT INTO mst.portofolio (id, kode_portofolio, nama, tujuan_pengelolaan, bm_category_default, benchmark, kompensasi_manager_basis, periode_review_terakhir, created_by)
VALUES
('22222222-0000-0000-0000-000000000001', 'PORT-TR-LIQ',   'Treasury Liquidity',   'Pengelolaan likuiditas harian — Cash & Deposito jangka pendek',        'HTC',   'BI Rate',                      'Berbasis bunga',                   '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000002', 'PORT-INV-LT',   'Investment Long-Term', 'Investasi jangka panjang — Obligasi held-to-maturity',                  'HTC',   'INDOBeX Composite Bond Index', 'Berbasis bunga + holding',         '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000003', 'PORT-INV-LIQ',  'Investment Liquidity', 'Obligasi yang dapat dijual untuk manajemen likuiditas — FVOCI',         'HTCS',  'IBPA Govt Bond Index',         'Berbasis bunga + realized gain/loss','2026-01-01','00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000004', 'PORT-TRADING',  'Trading Portfolio',    'Trading book — FVTPL, profit-taking',                                   'OTHER', 'Total return',                 'Berbasis fair value performance',  '2026-01-01', '00000000-0000-0000-0000-000000000001'),
('22222222-0000-0000-0000-000000000005', 'PORT-STRATEGIC','Strategic Equity',     'Penyertaan strategis saham (FVOCI Election)',                           'OTHER', 'Long-term holding',            'Berbasis dividen + holding',       '2026-01-01', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- ====================================================================
-- 12.10.5  PERIODE BUKU 2026 (12 BULANAN + 4 TRIWULANAN + 1 TAHUNAN = 17 row)
-- ====================================================================

INSERT INTO mst.periode_buku (periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan, tanggal_mulai, tanggal_akhir, status_periode)
VALUES
('PRD-2026-01',  'BULANAN',    2026,  1, NULL, '2026-01-01', '2026-01-31', 'OPEN'),
('PRD-2026-02',  'BULANAN',    2026,  2, NULL, '2026-02-01', '2026-02-28', 'OPEN'),
('PRD-2026-03',  'BULANAN',    2026,  3, NULL, '2026-03-01', '2026-03-31', 'OPEN'),
('PRD-2026-04',  'BULANAN',    2026,  4, NULL, '2026-04-01', '2026-04-30', 'OPEN'),
('PRD-2026-05',  'BULANAN',    2026,  5, NULL, '2026-05-01', '2026-05-31', 'OPEN'),
('PRD-2026-06',  'BULANAN',    2026,  6, NULL, '2026-06-01', '2026-06-30', 'OPEN'),
('PRD-2026-07',  'BULANAN',    2026,  7, NULL, '2026-07-01', '2026-07-31', 'OPEN'),
('PRD-2026-08',  'BULANAN',    2026,  8, NULL, '2026-08-01', '2026-08-31', 'OPEN'),
('PRD-2026-09',  'BULANAN',    2026,  9, NULL, '2026-09-01', '2026-09-30', 'OPEN'),
('PRD-2026-10',  'BULANAN',    2026, 10, NULL, '2026-10-01', '2026-10-31', 'OPEN'),
('PRD-2026-11',  'BULANAN',    2026, 11, NULL, '2026-11-01', '2026-11-30', 'OPEN'),
('PRD-2026-12',  'BULANAN',    2026, 12, NULL, '2026-12-01', '2026-12-31', 'OPEN'),
('PRD-2026-Q1',  'TRIWULANAN', 2026, NULL, 1,  '2026-01-01', '2026-03-31', 'OPEN'),
('PRD-2026-Q2',  'TRIWULANAN', 2026, NULL, 2,  '2026-04-01', '2026-06-30', 'OPEN'),
('PRD-2026-Q3',  'TRIWULANAN', 2026, NULL, 3,  '2026-07-01', '2026-09-30', 'OPEN'),
('PRD-2026-Q4',  'TRIWULANAN', 2026, NULL, 4,  '2026-10-01', '2026-12-31', 'OPEN'),
('PRD-2026',     'TAHUNAN',    2026, NULL, NULL,'2026-01-01', '2026-12-31', 'OPEN')
ON CONFLICT (periode_id_kode) DO NOTHING;

-- ====================================================================
-- 12.10.6  SAMPLE FX RATE (USD/IDR 1 Jan 2026 — 1 row)
--          maker_id = akuntansi user (…0013) sesuai persona §12.10.1
-- ====================================================================

INSERT INTO mst.kurs (fx_rate_id_kode, kode_mata_uang, tanggal_berlaku, kurs_tengah, sumber_kurs, periode_bulanan_id, maker_id)
SELECT 'FX-USD-20260101', 'USD', '2026-01-01', 16000.0000, 'BI_JISDOR',
       p.id,
       '00000000-0000-0000-0000-000000000013'
FROM mst.periode_buku p
WHERE p.periode_id_kode = 'PRD-2026-01'
ON CONFLICT (kode_mata_uang, tanggal_berlaku) DO NOTHING;

COMMIT;
