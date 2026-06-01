*\[ LOGO TUGURE \]*

**BUSINESS REQUIREMENTS DOCUMENT (BRD)**

**SISTEM BLIPS IFRS 9 INSTRUMEN INVESTASI**

*Modul Penempatan • Mark-to-Market • Renewal • Penjualan • Jatuh Tempo*

*Pendapatan Investasi • Media Upload • ECL Engine • EIR & Amortisasi*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR APPROVAL*

# Atribut Dokumen

| **Atribut**           | **Keterangan**                                                                                  |
| --------------------- | ----------------------------------------------------------------------------------------------- |
| Judul Dokumen         | Business Requirements Document — Sistem BLIPS IFRS 9 Instrumen Investasi                        |
| Kode Dokumen          | BRD-BLIPS-IFRS9-2026-001                                                                        |
| Versi                 | 1.1                                                                                             |
| Status                | DRAFT FOR APPROVAL                                                                              |
| Tanggal Terbit        | 02 Mei 2026                                                                                     |
| Bahasa                | Bahasa Indonesia                                                                                |
| Klasifikasi Informasi | INTERNAL — CONFIDENTIAL                                                                         |
| Pemilik Dokumen       | Direktorat Treasury & Investment / Direktorat Risk Management                                   |
| Penyusun              | Tim Project BLIPS IFRS 9 — Working Group                                                        |
| Reviewer              | Komite Investasi, ALCO, Akuntansi, IT Steering                                                  |
| Approver              | Direktur Keuangan (CFO) selaku Sponsor; Direksi Tugure selaku Steering Committee                |
| Standar Acuan         | PSAK 71 (IFRS 9), PSAK 50/55, PSAK 65, PSAK 25; Basel III IRB Foundation; Pefindo Default Study |
| Reference SoW         | SoW v1.2 — Sistem BLIPS IFRS 9 Instrumen Investasi (SOW-BLIPS-IFRS9-2026-001)                   |

# Revision History

| **Versi** | **Tanggal** | **Penyusun**  | **Reviewer**    | **Ringkasan Perubahan**                                        |
| --------- | ----------- | ------------- | --------------- | -------------------------------------------------------------- |
| 0.1       | 10 Apr 2026 | Working Group | —               | Initial draft — kerangka & objectives.                         |
| 0.5       | 20 Apr 2026 | Working Group | Risk, Akuntansi | Tambah functional requirements 16 modul + traceability ke SoW. |
| 0.9       | 28 Apr 2026 | Working Group | Komite, ALCO    | Revisi NFR, risk assessment, sign-off matrix.                  |
| 1.0       | 02 Mei 2026 | Working Group | Steering, CFO   | Final draft — siap untuk approval Steering Committee.          |

# Distribution List

Dokumen ini didistribusikan untuk review dan persetujuan kepada:

| **Posisi**                    | **Direktorat**       | **Peran**                                    |
| ----------------------------- | -------------------- | -------------------------------------------- |
| Direktur Utama                | BoD Tugure           | Steering Committee                           |
| Direktur Keuangan (CFO)       | Direktorat Keuangan  | Sponsor & Final Approver                     |
| Direktur Investasi & Treasury | Direktorat Investasi | Business Owner                               |
| Direktur Risk Management      | Direktorat Risk      | Risk Approver                                |
| Direktur Teknologi Informasi  | Direktorat IT        | Technical Approver                           |
| Komite Investasi              | Komite               | Reviewer — keputusan klasifikasi PSAK 71     |
| ALCO / Komite Risiko          | Komite               | Reviewer — bobot skenario PD, MEV, Impact PD |
| Kepala Treasury               | Treasury             | Business Lead — Maker workflow               |
| Kepala Akuntansi & Pelaporan  | Akuntansi            | Process Owner — jurnal, periode, FX          |
| Kepala Risk Officer           | Risk                 | Process Owner — rating, SICR, ECL, parameter |
| Kepala Compliance             | Compliance           | Reviewer — pemenuhan regulasi                |
| Kepala Internal Audit         | Audit                | Reviewer — audit trail & kontrol             |
| Project Manager BLIPS         | PMO                  | Project Owner — tracking deliverable         |
| External Auditor              | —                    | Read-only — periodic audit                   |

# Reference Documents

BRD ini mengacu ke dokumen-dokumen berikut. Konsistensi terhadap referensi wajib dijaga sepanjang siklus implementasi.

| **No** | **Kode**                 | **Judul**                                                     | **Versi / Tanggal** |
| ------ | ------------------------ | ------------------------------------------------------------- | ------------------- |
| 1      | SOW-BLIPS-IFRS9-2026-001 | Scope of Work & Flow Sistem BLIPS IFRS 9 Instrumen Investasi  | v1.1 / 02 Mei 2026  |
| 2      | PSAK-71                  | PSAK 71 — Instrumen Keuangan                                  | Berlaku 1 Jan 2020  |
| 3      | PSAK-50/55               | PSAK 50 & 55 — Instrumen Keuangan: Penyajian & Pengukuran     | Selaras PSAK 71     |
| 4      | PSAK-65                  | PSAK 65 — Konsolidasian: Look-through Reksadana               | Berlaku saat ini    |
| 5      | PSAK-25                  | PSAK 25 — Kebijakan Akuntansi, Perubahan Estimasi & Kesalahan | Berlaku saat ini    |
| 6      | BASEL-III-IRB            | Basel III Foundation IRB Approach (LGD)                       | BCBS publication    |
| 7      | PEFINDO-DS               | Pefindo Default Study — PD Normal & Cumulative                | Update triwulanan   |
| 8      | BI-JISDOR                | JISDOR & Kurs Tengah BI                                       | Harian              |
| 9      | POJK-LPS                 | Penjaminan LPS Rp 2 Miliar                                    | Berlaku saat ini    |
| 10     | TUGURE-INV-POL           | Investment & Treasury Policy Tugure                           | v3.2 / 2025         |
| 11     | TUGURE-RISK-FW           | Risk Management Framework Tugure                              | v2.5 / 2025         |
| 12     | TUGURE-COA               | Chart of Accounts Tugure (existing GL)                        | Live                |

# Daftar Isi

*\[Daftar isi otomatis akan ter-generate saat dokumen di-finalize di Microsoft Word via menu References → Table of Contents → Update Field. Halaman ini placeholder.\]*

**Outline Bab Utama:**

1\. Pendahuluan

2\. Executive Summary

3\. Business Context — Tugure & Lanskap Industri

4\. Stakeholder Analysis

5\. Business Objectives, KPI & Critical Success Factors

6\. Scope & Out-of-Scope

7\. Current State (As-Is) & Future State (To-Be)

8\. Business Requirements per Modul

9\. Non-Functional Requirements

10\. Compliance & Regulatory Requirements

11\. Use Cases per Role

12\. Assumptions, Constraints, Dependencies

13\. Risk Assessment & Mitigation

14\. Acceptance Criteria & Sign-Off Matrix

15\. Implementation Approach & Milestone

16\. Traceability Matrix BR ↔ SoW

17\. Glossary

18\. Lampiran

# 1\. Pendahuluan

## 1.1 Tujuan Dokumen

Business Requirements Document (BRD) ini menetapkan kebutuhan bisnis dan fungsional untuk pembangunan Sistem BLIPS IFRS 9 Instrumen Investasi di lingkungan PT Tugu Reasuransi Indonesia (Tugure). BRD merupakan dokumen kontraktual antara Business Owner (Direktorat Investasi & Treasury, Direktorat Risk Management, Direktorat Keuangan) dengan Project Delivery Team (internal IT + vendor implementor) yang menjadi basis untuk pembentukan Functional Specification Document (FSD), Test Cases, dan kriteria User Acceptance Test (UAT).

## 1.2 Audiens Dokumen

BRD ini ditujukan untuk:

  - Steering Committee Tugure (Direksi) — sebagai dasar pengambilan keputusan investasi proyek.

  - Business Owner & Process Owner — sebagai komitmen scope dan acceptance criteria.

  - Project Manager & Tim Implementasi — sebagai input untuk perencanaan FSD, arsitektur teknis, dan testing.

  - Audit & Compliance — sebagai bukti dokumentasi pemenuhan PSAK 71.

  - Auditor Eksternal — sebagai dokumen referensi saat review pengendalian internal dan substantive testing.

## 1.3 Hubungan dengan Dokumen Lain

BRD ini bersifat upstream terhadap dokumen teknis. Hierarki dokumen proyek BLIPS IFRS 9 sebagai berikut:

| **Level**    | **Dokumen**                 | **Fokus**                                                        | **Audiens Utama**       |
| ------------ | --------------------------- | ---------------------------------------------------------------- | ----------------------- |
| Strategic    | Investment Decision Memo    | Justifikasi investasi proyek, ROI, governance.                   | BoD, Steering           |
| Business     | BRD (dokumen ini)           | Kebutuhan bisnis, scope, requirements, acceptance.               | Business + IT Lead      |
| Technical    | SoW v1.1                    | Detail scope teknis, modul, formula, field, jurnal.              | Vendor + IT + Akuntansi |
| Detailed     | FSD per modul               | Spesifikasi fungsional level granular per screen, API, validasi. | Developer + QA          |
| Architecture | Solution Architecture & ERD | Database schema, integrasi, infrastruktur.                       | IT Architect + DevOps   |
| Testing      | UAT Scripts & Test Cases    | Skenario test berbasis BRD requirements.                         | QA + Business User      |
| Operational  | Runbook & User Manual       | Operasional sistem post go-live.                                 | Ops + End-User          |

*Bila terdapat ketidaksesuaian antara BRD dan SoW, BRD adalah dokumen yang DIDAHULUKAN dan memerlukan formal Change Request (CR) untuk amandemen.*

## 1.4 Konvensi & Notasi

Notasi yang digunakan dalam BRD:

  - BR-XXX-\#\#\# — Business Requirement ID, di mana XXX = kode modul (mis. MAS, SPP, ECL, EIR) dan \#\#\# = nomor urut. Contoh: BR-ECL-007.

  - WAJIB / SHOULD / OPSIONAL — tingkat priority requirement (mengikuti RFC 2119: MUST = WAJIB, SHOULD = SEBAIKNYA, MAY = OPSIONAL).

  - Klasifikasi: H (High — must-have untuk go-live), M (Medium — should-have, dapat phase 2), L (Low — nice-to-have).

  - Reference ke SoW: dituliskan sebagai 'Lihat SoW Bab x.y' atau 'SoW v1.1 §x.y'.

# 2\. Executive Summary

## 2.1 Latar Belakang

PT Tugu Reasuransi Indonesia (Tugure) sebagai perusahaan reasuransi nasional terikat pada kewajiban penerapan PSAK 71 (selaras IFRS 9) untuk instrumen keuangan. Portofolio investasi Tugure mencakup empat tipe instrumen utama — Cash di Bank, Deposito Berjangka, Obligasi (pemerintah & korporasi), Saham, dan Reksadana — yang seluruhnya memerlukan klasifikasi PSAK 71, pengukuran subsequent (Amortized Cost / FVOCI / FVTPL), dan untuk klasifikasi non-FVTPL: pengakuan Expected Credit Loss (ECL) berbasis tiga skenario probabilitas default dengan penyesuaian forward-looking.

Sistem existing yang dipakai Tugure saat ini belum mampu mendukung seluruh requirement PSAK 71 secara terintegrasi: klasifikasi instrumen masih manual via spreadsheet, ECL dihitung di luar sistem akuntansi, jurnal investasi tidak otomatis ter-posting ke GL, dan audit trail dokumen pendukung tersebar di multiple repositori. Kondisi ini menimbulkan risiko material misstatement, ketidakefisienan operasional, dan temuan audit berulang.

## 2.2 Problem Statement

| **\#** | **Problem**                                                                                                                                         | **Dampak**                                                                                   |
| ------ | --------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| 1      | Klasifikasi PSAK 71 (SPPI Test, BM Test) dilakukan manual dan tidak terdokumentasi sistematis.                                                      | Risiko misclassification → restatement; temuan audit eksternal.                              |
| 2      | Perhitungan ECL 3-skenario dengan forward-looking dilakukan di Excel terpisah, tidak terintegrasi ke GL.                                            | Reconciliation gap antara ECL workbook dan GL; risiko salah posting CKPN.                    |
| 3      | Tidak ada engine EIR (Effective Interest Rate); pendapatan bunga obligasi diakui via simple interest sehingga premium/diskonto tidak teramortisasi. | Misstatement pendapatan bunga; carrying amount AC tidak akurat; non-compliance PSAK 71 §5.4. |
| 4      | Look-through reksadana untuk ECL dilakukan ad-hoc; komposisi underlying tidak ter-track per snapshot bulanan.                                       | Risk-management view ECL tidak akurat; sulit untuk concentration analysis.                   |
| 5      | Multi-currency: konversi FX dilakukan manual; tidak ada locking saat periode CLOSED.                                                                | Risiko inkonsistensi kurs antar laporan; potensi tampering data historis.                    |
| 6      | Periode buku tidak ter-enforce di sistem; backdated entry mungkin tanpa kontrol.                                                                    | Material weakness pengendalian internal; risiko PSAK 25 prior-period adjustment ad-hoc.      |
| 7      | Dokumen bukti (term sheet, prospektus, NoA) tersebar di file server tanpa link ke transaksi.                                                        | Inefisiensi audit; risiko kehilangan dokumen; ketidakmampuan trace audit trail.              |
| 8      | Tidak ada workflow formal Maker-Reviewer-Approver (3-eyes) untuk klasifikasi & ECL parameter.                                                       | Material weakness segregation of duty; risiko conflict of interest.                          |

## 2.3 Solusi yang Diusulkan

Tugure mengusulkan pembangunan Sistem BLIPS IFRS 9 Instrumen Investasi — sebuah sistem terintegrasi berbasis web yang mencakup 16 modul fungsional (lihat SoW v1.1) dengan kapabilitas:

  - Engine SPPI Test (10 pertanyaan) & Business Model Test dengan workflow approval Komite Investasi.

  - Klasifikasi PSAK 71 otomatis (matriks SPPI × BM → AC/FVOCI/FVTPL) dengan jurnal reklasifikasi prospektif.

  - Engine EIR & Amortisasi (Newton-Raphson IRR solver) dengan amortization schedule per instrumen.

  - ECL Engine 3-skenario (Optimistic/Base/Pessimistic) dengan dual forward-looking layer (Impact MEV to PD + Impact PD).

  - 3-Stage model PSAK 71 dengan SICR & default trigger otomatis dari Counterparty Rating History.

  - Logika LPS aggregator untuk Cash + Deposito per bank.

  - Look-through reksadana ke level underlying (sovereign / korporasi / cash / equity).

  - Multi-currency dengan kurs Tengah BI per event akuntansi & FX gain/loss treatment yang benar.

  - Periode buku 3-status (OPEN / SOFT\_CLOSED / CLOSED) dengan workflow soft & hard close.

  - Master Mapping Jurnal generic dengan resolusi runtime → posting otomatis ke GL host.

  - Media upload terintegrasi dengan SHA-256 hash, encryption-at-rest (S3 + KMS), dan akses log.

  - Reporting komprehensif: 25+ laporan (CKPN, Stage Distribution, Amortization Schedule, Closing Audit Trail, dll).

  - Three-eyes principle Maker-Reviewer-Approver untuk seluruh transaksi material.

## 2.4 Manfaat Bisnis

| **Kategori**           | **Manfaat**                                                                                                     |
| ---------------------- | --------------------------------------------------------------------------------------------------------------- |
| Compliance             | Pemenuhan penuh PSAK 71 (klasifikasi, ECL, EIR, staging); minimisasi risiko temuan audit eksternal & regulator. |
| Operational Efficiency | Reduksi effort manual ≥ 70% untuk closing bulanan; akselerasi closing dari H+15 menjadi H+10.                   |
| Akurasi Akuntansi      | Pendapatan bunga & carrying amount akurat via EIR; ECL akurat via 3-skenario forward-looking.                   |
| Auditability           | Audit trail terintegrasi dengan media upload; auditor dapat melakukan substantive testing langsung dari sistem. |
| Risk Management        | Real-time monitoring exposure per counterparty, rating, stage; alert otomatis SICR & default.                   |
| Decision Support       | Dashboard manajemen untuk Komite Investasi/ALCO dengan stress test & sensitivitas MEV.                          |
| Cost Avoidance         | Hindari restatement biaya (rata-rata Rp 500 juta–2 miliar per kasus); hindari fines OJK & reputational damage.  |

## 2.5 Ringkasan Investasi & Timeline

Berdasarkan SoW v1.1 §11.2, total durasi implementasi diestimasi 33 minggu (≈ 8 bulan) dengan 11 fase: Discovery → Desain → 5 fase Development → SIT → UAT → Production → Hypercare. Investasi terdiri dari komponen license/SaaS, jasa implementasi vendor, infrastruktur (produksi + DR + UAT), training, dan kontinjensi. Detail business case investment didokumentasikan di Investment Decision Memo terpisah.

## 2.6 Approval Requested

Steering Committee diminta untuk menyetujui:

1.  Business case dan scope yang tertuang dalam BRD ini.

2.  Pembentukan Project Charter dan resourcing internal sesuai struktur stakeholder Bab 4.

3.  Komitmen budget sesuai breakdown di Investment Decision Memo (terpisah).

4.  Mandat pemilihan vendor implementor via proses procurement formal.

5.  Otorisasi sponsor kepada CFO untuk go/no-go quarterly milestone review.

# 3\. Business Context — Tugure & Lanskap Industri

## 3.1 Profil PT Tugu Reasuransi Indonesia

PT Tugu Reasuransi Indonesia (Tugure) merupakan perusahaan reasuransi nasional yang menyediakan kapasitas reasuransi untuk industri asuransi umum dan jiwa di Indonesia. Sebagai institusi keuangan yang memiliki portofolio investasi material (premi cadangan teknis, modal sendiri, dan dana investasi pemegang polis), Tugure terikat pada penerapan PSAK 71 untuk instrumen keuangan, di samping standar khusus industri reasuransi (PSAK 74 / IFRS 17 untuk kontrak asuransi).

Karakteristik portofolio investasi Tugure (asumsi tipikal industri reasuransi nasional):

| **Aspek**            | **Profil**                                                                                                                          |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Tujuan Investasi     | Backing premi cadangan teknis (HtC dominan), surplus capital (HtC\&S), trading book terbatas.                                       |
| Komposisi Portofolio | Obligasi pemerintah (SUN, SBN) \~40-50%; Obligasi korporasi \~20-25%; Deposito \~15-20%; Reksadana \~10-15%; Saham minoritas; Cash. |
| Mata Uang            | Mayoritas IDR; eksposur USD/SGD untuk diversifikasi pada obligasi global & reksadana.                                               |
| Holding Period       | Mayoritas long-term (\>1 tahun) seiring profil liabilitas reasuransi.                                                               |
| Risk Appetite        | Konservatif — dominan investment grade (idA atau lebih tinggi); subordinated bond minimal.                                          |
| Regulasi             | OJK (POJK reasuransi), DSAK IAI (PSAK), Bank Indonesia (transaksi devisa).                                                          |

## 3.2 Lanskap Industri & Tantangan IFRS 9 / PSAK 71

PSAK 71 yang berlaku efektif 1 Januari 2020 mengubah secara fundamental cara pengukuran instrumen keuangan, terutama melalui pengenalan model Expected Credit Loss (ECL) yang menggantikan model Incurred Loss di PSAK 55. Bagi perusahaan reasuransi, dampak utama:

  - ECL diakui sejak hari pertama (Day-1 loss) untuk seluruh instrumen AC dan FVOCI utang — kontras dengan model lama yang menunggu objective evidence of impairment.

  - Klasifikasi berbasis dua uji simultan (SPPI Test + Business Model Test) — bukan lagi berbasis intent management seperti PSAK 55.

  - Forward-looking adjustment WAJIB — ECL tidak boleh hanya berbasis historical default; harus mempertimbangkan proyeksi makroekonomi (GDP, inflasi, suku bunga, FX).

  - 3-Stage model dengan Significant Increase in Credit Risk (SICR) sebagai trigger migrasi → memerlukan tracking rating history granular.

  - FVOCI dengan recycling untuk debt instrument dan no-recycling untuk equity (election) — treatment berbeda di OCI.

  - Effective Interest Rate (EIR) wajib untuk AC dan FVOCI utang — tidak boleh simple interest.

## 3.3 Regulatory Landscape

| **Regulator / Body** | **Standar / Regulasi**                               | **Dampak terhadap BLIPS IFRS 9**                                                               |
| -------------------- | ---------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| DSAK IAI             | PSAK 71 — Instrumen Keuangan                         | Core requirement — klasifikasi, ECL, EIR, staging, reklasifikasi.                              |
| DSAK IAI             | PSAK 50 & 55 — Penyajian & Pengukuran                | Selaras PSAK 71; supplementary untuk presentation.                                             |
| DSAK IAI             | PSAK 65 — Look-through reksadana                     | Wajib look-through untuk consolidation; basis untuk ECL look-through.                          |
| DSAK IAI             | PSAK 25 — Prior-Period Adjustment                    | Treatment untuk koreksi error material di periode CLOSED.                                      |
| BCBS (Basel)         | Basel III IRB Foundation Approach                    | Sumber LGD per tipe eksposur (Sovereign 0,4500; Senior Unsecured 0,4500; Subordinated 0,7500). |
| Pefindo              | Pefindo Default Study                                | Sumber PD Normal per rating; cumulative PD multi-tenor untuk Lifetime PD.                      |
| Bank Indonesia       | JISDOR & Kurs Tengah BI                              | Sumber resmi konversi mata uang asing → IDR equivalent.                                        |
| LPS                  | Lembaga Penjamin Simpanan — Rp 2 Miliar/nasabah/bank | Cap untuk eksposur tak terjamin pada Cash + Deposito di bank.                                  |
| OJK                  | POJK Reasuransi (sektor)                             | Compliance reporting, risk-based capital, governance.                                          |
| IBPA                 | Indonesia Bond Pricing Agency                        | Sumber harga referensi obligasi untuk MTM harian.                                              |
| KSEI / MI            | NAB Reksadana harian                                 | Sumber harga referensi reksadana.                                                              |

## 3.4 Existing System Landscape (As-Is)

Sistem yang saat ini dipakai Tugure untuk pengelolaan investasi (assumed baseline):

| **Fungsi**             | **Tools Existing**                                     | **Issue**                                                             |
| ---------------------- | ------------------------------------------------------ | --------------------------------------------------------------------- |
| Master Instrumen       | Spreadsheet + sistem GL legacy                         | Inkonsistensi field; tidak ada link ke SPPI/BM Test.                  |
| Pencatatan Transaksi   | Sistem GL legacy                                       | Hanya posting summary; granularitas instrumen rendah.                 |
| MTM Harian             | Manual import IBPA / NAB ke spreadsheet → upload ke GL | Latency H+1; risiko user error.                                       |
| SPPI / BM Test         | Tidak ada; klasifikasi by judgment + memo Komite       | Tidak ada audit trail digital; risiko inconsistency.                  |
| Perhitungan ECL        | Excel workbook terpisah (per portfolio manager)        | Tidak terintegrasi; reconciliation gap; sulit audit trail.            |
| EIR & Amortisasi       | Tidak ada engine EIR — pakai simple interest           | Non-compliance PSAK 71 §5.4; pendapatan bunga overstated/understated. |
| Multi-currency         | Spreadsheet konversi manual                            | Risk inkonsistensi kurs; tidak ada locking periode.                   |
| Periode Buku           | Manual cut-off, tanpa enforcement sistem               | Risiko backdated entry; material weakness.                            |
| Media Upload (dokumen) | File server (shared drive)                             | Tidak link ke transaksi; risiko kehilangan; tidak ada hash integrity. |
| Reporting CKPN         | Excel pivot manual                                     | Slow; tidak real-time; tidak drill-down.                              |

## 3.5 Pain Points & Risiko Status Quo

Risiko tetap menggunakan sistem existing tanpa perbaikan:

  - Audit external risk: temuan repetitif material weakness → kualifikasi opini auditor.

  - Regulatory risk: ketidakmampuan submit reporting OJK/BI sesuai timeline.

  - Operational risk: manual processing → human error; key-person dependency.

  - Financial risk: misstatement carrying amount & ECL → dampak ke laba berjalan & capital adequacy.

  - Reputational risk: bila ada error material yang ter-publish → erosi kepercayaan investor & ceding company.

  - Cost-of-control: setiap tahun memerlukan additional resources untuk re-perform manual reconciliation.

# 4\. Stakeholder Analysis

## 4.1 Stakeholder Map

Stakeholder dipetakan menjadi empat kuadran berdasarkan dua dimensi: Influence (tingkat pengaruh terhadap keputusan proyek) dan Interest (tingkat ketertarikan terhadap outcome proyek).

| **Kuadran**                                     | **Strategi Engagement**                               | **Stakeholder Tugure**                                                          |
| ----------------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------- |
| High Influence × High Interest (Manage Closely) | Engagement intensif; consult on every major decision. | Direksi/Steering, CFO, Direktur Investasi & Treasury, Direktur Risk.            |
| High Influence × Low Interest (Keep Satisfied)  | Update berkala; high-level summary.                   | Direktur Utama (bila bukan sponsor langsung), Direktur Compliance, Direktur IT. |
| Low Influence × High Interest (Keep Informed)   | Newsletter berkala; townhall update.                  | Treasury Officers, Risk Officers, Akuntansi Staff, IT Operation.                |
| Low Influence × Low Interest (Monitor)          | Minimum communication; respond when asked.            | Internal Audit (review post-implementasi), Tax Department, Legal.               |

## 4.2 Roles & Responsibilities (RACI Matrix)

Matriks RACI berikut memetakan tanggung jawab utama per workstream proyek. R = Responsible (yang mengeksekusi); A = Accountable (yang bertanggung jawab dan approve); C = Consulted (yang dimintakan input); I = Informed (yang diberi tahu).

| **Workstream**             | **CFO** | **Dir Inv** | **Dir Risk** | **Dir IT** | **Komite Inv** | **Treasury** | **Risk Officer** | **Akuntansi** | **PMO** |
| -------------------------- | ------- | ----------- | ------------ | ---------- | -------------- | ------------ | ---------------- | ------------- | ------- |
| BRD Approval               | A       | R           | R            | C          | C              | C            | C                | C             | I       |
| SoW & FSD Sign-off         | A       | C           | C            | R          | I              | C            | C                | C             | R       |
| Vendor Selection           | A       | C           | C            | R          | I              | I            | I                | I             | R       |
| SPPI/BM Test Workflow      | I       | A           | C            | I          | R              | R            | C                | C             | I       |
| ECL Parameter (PD/LGD/MEV) | I       | C           | A            | I          | R              | I            | R                | C             | I       |
| Klasifikasi PSAK 71        | I       | A           | R            | I          | R              | C            | C                | C             | I       |
| EIR Methodology            | C       | C           | C            | I          | I              | I            | C                | A/R           | I       |
| Master Mapping Jurnal      | C       | I           | I            | C          | I              | I            | I                | A/R           | I       |
| Periode Buku Closing       | A       | I           | I            | C          | I              | I            | I                | R             | I       |
| UAT Sign-off               | A       | C           | C            | C          | C              | R            | R                | R             | R       |
| Production Go-Live         | A       | C           | C            | R          | I              | I            | I                | I             | R       |
| Audit Trail Review         | I       | I           | I            | I          | I              | I            | I                | C             | I       |

## 4.3 Key Decision-Making Bodies

**A. Steering Committee**

  - Komposisi: Direktur Utama (chair), CFO, Direktur Investasi, Direktur Risk, Direktur IT.

  - Kewenangan: keputusan budget, approval scope change material, go/no-go phase milestone.

  - Frekuensi rapat: bulanan + ad-hoc untuk eskalasi kritis.

**B. Komite Investasi (Investment Committee)**

  - Komposisi: CFO (chair), Direktur Investasi, Kepala Treasury, Kepala Risk, Kepala Akuntansi, plus 1-2 Komisaris Independen sebagai advisor.

  - Kewenangan: approve klasifikasi PSAK 71 per instrumen baru, FVOCI Election untuk saham, perubahan kebijakan investasi, override SPPI/BM Test result.

  - Frekuensi rapat: bulanan + ad-hoc untuk transaksi besar (\> Rp 50 miliar).

**C. ALCO (Asset-Liability Committee) / Komite Risiko**

  - Komposisi: CFO (chair), Direktur Risk, Direktur Investasi, Kepala Treasury, Kepala Akuntansi, Chief Actuary.

  - Kewenangan: approve bobot skenario PD (default 0,25/0,50/0,25), Impact MEV to PD per periode, Impact PD multiplier, threshold SICR, kebijakan curing Stage 3.

  - Frekuensi rapat: triwulanan + ad-hoc bila terjadi shock makro.

**D. Working Group BLIPS (Project Team)**

  - Komposisi: Project Manager (PMO), 2 Business Analyst dari Investasi & Akuntansi, 1 Risk Analyst, 1 IT Architect, 1 representative dari vendor implementor.

  - Kewenangan: day-to-day project execution, escalation ke Steering bila ada blocker.

  - Frekuensi rapat: weekly stand-up + bi-weekly review.

# 5\. Business Objectives, KPI & Critical Success Factors

## 5.1 Strategic Objectives

| **\#** | **Strategic Objective**                                                                                           | **Alignment ke Corporate Strategy**                     |
| ------ | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| SO-01  | Mencapai 100% compliance terhadap PSAK 71 untuk seluruh instrumen investasi by Q1 2027.                           | Pillar: Operational Excellence & Regulatory Compliance. |
| SO-02  | Mengurangi waktu closing periode bulanan dari H+15 menjadi H+10 hari kerja.                                       | Pillar: Operational Efficiency.                         |
| SO-03  | Meningkatkan kemampuan risk monitoring real-time terhadap exposure per counterparty/rating/stage.                 | Pillar: Risk Management Excellence.                     |
| SO-04  | Memberikan single source of truth untuk reporting investasi ke management, BoD, regulator, dan auditor eksternal. | Pillar: Data-Driven Decision Making.                    |
| SO-05  | Mencapai zero material audit findings terkait pengelolaan instrumen investasi by audit cycle 2027.                | Pillar: Governance & Audit.                             |

## 5.2 Operational Objectives

| **\#** | **Operational Objective**                                                                      | **Target Measurable**                                                               |
| ------ | ---------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| OO-01  | Otomatisasi klasifikasi PSAK 71 via SPPI Test + BM Test engine.                                | 100% transaksi penempatan baru lulus pre-trade clearance via sistem.                |
| OO-02  | Otomatisasi perhitungan ECL 3-skenario dengan forward-looking adjustment.                      | ECL bulanan ter-generate dalam \< 4 jam batch run untuk 1.000+ instrumen.           |
| OO-03  | Otomatisasi posting jurnal investasi ke GL host.                                               | ≥ 99% jurnal otomatis ter-posting tanpa manual intervention; ≤ 1% perlu adjustment. |
| OO-04  | Implementasi engine EIR dengan amortization schedule per instrumen AC/FVOCI utang.             | 100% instrumen AC/FVOCI utang memiliki EIR computed & schedule aktif.               |
| OO-05  | Integrasi media upload terenkripsi dengan SHA-256 hash per dokumen.                            | 100% transaksi material memiliki minimal 1 dokumen pendukung ter-link.              |
| OO-06  | Workflow Maker-Reviewer-Approver (3-eyes) untuk seluruh transaksi material & parameter master. | 0 bypass workflow; 0 segregation of duty violation.                                 |
| OO-07  | Periode buku 3-status (OPEN/SOFT\_CLOSED/CLOSED) ter-enforce di sistem.                        | 0 transaksi backdated tanpa adjustment journal entry & approval Akuntansi.          |
| OO-08  | Multi-currency dengan kurs Tengah BI ter-locked saat periode CLOSED.                           | 100% kurs ter-update dari BI JISDOR otomatis pada hari kerja jam 10:30 WIB.         |

## 5.3 Compliance Objectives

| **\#** | **Compliance Objective**                                                                          | **Standar Acuan**         |
| ------ | ------------------------------------------------------------------------------------------------- | ------------------------- |
| CO-01  | Klasifikasi seluruh aset keuangan via SPPI Test + Business Model Test dengan audit trail digital. | PSAK 71 §4.1, 4.4         |
| CO-02  | Pengakuan ECL 3-stage (12-Month / Lifetime) dengan SICR & default trigger.                        | PSAK 71 §5.5              |
| CO-03  | Effective Interest Rate (EIR) untuk pengakuan pendapatan bunga AC & FVOCI utang.                  | PSAK 71 §5.4 & Lampiran A |
| CO-04  | Forward-looking adjustment ECL berbasis MEV.                                                      | PSAK 71 §5.5.17           |
| CO-05  | FVOCI Election irrevocable & no-recycling untuk strategic equity.                                 | PSAK 71 §5.7.5            |
| CO-06  | Look-through reksadana untuk consolidation & ECL.                                                 | PSAK 65                   |
| CO-07  | Prior-period adjustment treatment untuk error material di periode CLOSED.                         | PSAK 25                   |
| CO-08  | LGD per tipe eksposur sesuai Basel III IRB Foundation.                                            | Basel III IRB             |
| CO-09  | PD per rating dari Pefindo Default Study (1-Yr & Lifetime).                                       | Pefindo DS                |
| CO-10  | Konversi mata uang via Kurs Tengah BI / JISDOR.                                                   | Bank Indonesia            |

## 5.4 Key Performance Indicators (KPI)

KPI dipakai untuk mengukur keberhasilan implementasi BLIPS IFRS 9 post go-live:

| **KPI ID** | **Metric**                                                  | **Baseline (As-Is)**          | **Target (To-Be Y1)** | **Target (To-Be Y2)** |
| ---------- | ----------------------------------------------------------- | ----------------------------- | --------------------- | --------------------- |
| KPI-01     | Waktu Closing Periode Bulanan                               | H+15 hari kerja               | H+12                  | H+10                  |
| KPI-02     | % Jurnal Otomatis ter-posting tanpa adjustment              | Tidak applicable (manual)     | ≥ 95%                 | ≥ 99%                 |
| KPI-03     | % Transaksi dengan dokumen pendukung ter-link               | \~ 60% (file server)          | ≥ 95%                 | 100%                  |
| KPI-04     | % Instrumen AC/FVOCI utang dengan EIR computed              | 0%                            | 100%                  | 100%                  |
| KPI-05     | Reconciliation gap ECL workbook vs GL                       | Rata-rata 0,5–2% per bulan    | ≤ 0,1%                | 0%                    |
| KPI-06     | Material weakness audit findings                            | 2-3 per audit cycle (assumed) | ≤ 1                   | 0                     |
| KPI-07     | Mean Time to Recovery (MTTR) ECL batch failure              | Tidak applicable              | ≤ 4 jam               | ≤ 2 jam               |
| KPI-08     | User satisfaction score (UAT survey)                        | —                             | ≥ 4,0/5               | ≥ 4,5/5               |
| KPI-09     | Alert SICR/Default response time (rating downgrade ke aksi) | Manual, beberapa hari         | ≤ 1 hari kerja        | Same-day              |
| KPI-10     | % Backdated entry tanpa proper adjustment journal           | Tidak ter-monitor             | 0%                    | 0%                    |

## 5.5 Critical Success Factors (CSF)

Faktor-faktor yang harus dipenuhi agar proyek berhasil:

| **CSF** | **Deskripsi**                                                                                                                                                 | **Owner**              |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| CSF-01  | Komitmen sponsorship dari CFO sepanjang siklus proyek (33 minggu).                                                                                            | CFO                    |
| CSF-02  | Ketersediaan key business users (Treasury, Risk, Akuntansi) untuk requirement gathering, UAT, dan training — minimal 30% effort selama Discovery & UAT phase. | Direktur masing-masing |
| CSF-03  | Vendor implementor yang memiliki track record IFRS 9/PSAK 71 di institusi keuangan Indonesia (asuransi, bank, atau reasuransi).                               | PMO + IT               |
| CSF-04  | Stabilitas kebijakan PSAK 71 selama implementasi (tidak ada major standard change).                                                                           | External — DSAK IAI    |
| CSF-05  | Ketersediaan API / file batch interface ke GL host yang sudah teruji.                                                                                         | Direktorat IT          |
| CSF-06  | Akses berkala ke publikasi Pefindo Default Study (rating + cumulative PD).                                                                                    | Risk + Procurement     |
| CSF-07  | Akses BI JISDOR & Kurs Tengah BI via scheduled feed atau scrape resmi.                                                                                        | Akuntansi + IT         |
| CSF-08  | Approved budget tanpa pemotongan signifikan di tengah implementasi.                                                                                           | CFO + BoD              |
| CSF-09  | Change management & training plan untuk seluruh end-user (\~50-100 users).                                                                                    | HR + PMO               |
| CSF-10  | Disaster Recovery & Business Continuity sudah dirancang sejak awal (Active-Passive minimum).                                                                  | IT Architect           |

# 6\. Scope & Out-of-Scope

## 6.1 In-Scope — Functional

Modul fungsional yang termasuk dalam scope BLIPS IFRS 9 (sesuai SoW v1.1 §2.1):

6.  Master Data: instrumen, counterparty (bank, issuer, MI, kustodian, emiten saham), rating Pefindo, mapping LGD Basel, master portofolio, master mata uang & kurs, Master CoA, Master Mapping Jurnal, master periode buku.

7.  SPPI Test & Business Model Test: 10-question checklist, BM Test indicators, matriks klasifikasi PSAK 71, reklasifikasi prospektif (6 kombinasi).

8.  Modul Penempatan Instrumen — pencatatan transaksi pembelian, validasi pre-posting, upload dokumen.

9.  Modul Mark-to-Market (MTM) — update harga harian dari IBPA/NAB/BEI, jurnal otomatis OCI atau P\&L per klasifikasi.

10. Modul Renewal — auto-rollover deposito, dengan skema Pokok Saja atau Pokok+Bunga Net.

11. Modul Penjualan/Pencairan — disposal pre-maturity dengan realized gain/loss + reklasifikasi OCI ke P\&L (FVOCI utang).

12. Modul Jatuh Tempo (Closure) — settlement otomatis pokok + kupon final.

13. Modul Pendapatan Investasi — akrual harian bunga via EIR (AC/FVOCI utang), distribusi reksadana, dividen.

14. Modul Media Upload — repositori terenkripsi (S3 + KMS) dengan SHA-256 hash & access log.

15. ECL Engine — perhitungan ECL Stage 1/2/3 dengan dual forward-looking layer (Impact MEV to PD + Impact PD).

16. EIR & Amortisasi (NEW di SoW v1.1) — Newton-Raphson IRR solver, amortization schedule, treatment biaya transaksi & premium/diskonto, re-estimation.

17. Modul Periode Buku — 3-status (OPEN/SOFT\_CLOSED/CLOSED) dengan workflow soft & hard close.

18. Modul FX Rate Management — kurs harian BI dengan locking saat periode CLOSED.

19. Modul Mapping Jurnal — generic event-detail dengan resolusi runtime untuk klasifikasi/tipe/underlying.

20. Reporting & Dashboard — 25+ laporan (CKPN, Stage Distribution, Amortization Schedule, Closing Audit Trail, dll).

21. Jurnal & GL Interface — generate jurnal otomatis dan kirim ke GL host via API atau file batch.

## 6.2 In-Scope — Tipe Instrumen

| **Tipe**                   | **Sub-Tipe**                        | **Klasifikasi PSAK 71 (Default)**                       |
| -------------------------- | ----------------------------------- | ------------------------------------------------------- |
| Cash di Bank               | Giro, Tabungan                      | AC                                                      |
| Deposito Berjangka         | Berjangka, On-Call                  | AC                                                      |
| Obligasi Pemerintah        | SUN, SBN, ORI, SR, INDON            | FVOCI (HtC\&S) atau AC (HtC)                            |
| Obligasi Korporasi         | Plain, Sukuk, Subordinated          | FVOCI (HtC\&S), AC (HtC), atau FVTPL (Trading)          |
| Saham                      | LQ45, IDX30, Non-LQ45, Pengembangan | FVTPL (default) atau FVOCI Election (irrevocable)       |
| Reksadana Pasar Uang       | Konvensional, Syariah               | FVTPL (default); FVOCI dengan kebijakan akuntansi       |
| Reksadana Pendapatan Tetap | Konvensional, Syariah               | FVTPL (default); FVOCI dengan kebijakan                 |
| Reksadana Saham            | Konvensional, Syariah, Indeks       | FVTPL (default); FVOCI dengan kebijakan (tidak ada ECL) |
| Reksadana Campuran         | Konvensional, Syariah               | FVTPL (default); FVOCI dengan kebijakan                 |

## 6.3 Out-of-Scope

Item-item berikut SECARA EKSPLISIT TIDAK termasuk dalam scope BLIPS IFRS 9 dan akan ditangani via sistem terpisah atau proses manual berkelanjutan (sesuai SoW v1.1 §2.2):

  - Instrumen derivatif: swap (interest rate, currency), forward, option, structured products.

  - Repo / Reverse repo transactions.

  - Manajemen kas dan cash-flow forecasting (treasury planning) — di luar pencatatan saldo.

  - Manajemen counterparty limit dan credit approval workflow.

  - Pelaporan regulasi spesifik OJK / BI (LBU/LBBU) yang memerlukan format dedicated — disebut ringkas, format detail menjadi proyek terpisah.

  - Pelaporan PSAK 74 / IFRS 17 untuk kontrak asuransi/reasuransi — dikelola oleh sistem actuarial terpisah.

  - Tax provisioning & SPT tahunan — interface ke sistem pajak via export.

  - Asset-Liability Management (ALM) modeling — dikelola oleh sistem ALM terpisah, BLIPS hanya menyediakan data.

  - Risk-based capital calculation untuk reasuransi — di luar lingkup; output BLIPS digunakan sebagai input.

  - Bond pricing modeling internal — Tugure tetap mengandalkan IBPA sebagai sumber resmi.

## 6.4 System Boundaries (Interfaces)

Antar muka antara BLIPS dan sistem lain di lingkungan Tugure:

| **Interface**               | **Direction**   | **Mode**                                  | **Frekuensi**                   |
| --------------------------- | --------------- | ----------------------------------------- | ------------------------------- |
| GL Host                     | BLIPS → GL      | API REST atau file batch CSV/XML          | Real-time atau batch akhir hari |
| Pefindo Rating Feed         | Pefindo → BLIPS | Upload manual XLSX/CSV oleh Risk Officer  | Triwulanan + ad-hoc             |
| IBPA Bond Pricing           | IBPA → BLIPS    | File feed harian (CSV/XML)                | Harian H+0 atau H+1             |
| KSEI / MI NAB               | KSEI/MI → BLIPS | Upload manual XLSX atau API jika tersedia | Harian                          |
| BEI Saham Closing Price     | BEI → BLIPS     | File feed harian (CSV)                    | Harian post-market close        |
| BI JISDOR & Kurs Tengah     | BI → BLIPS      | Web scraping resmi atau API publikasi     | Hari kerja jam 10:30 WIB        |
| Fund Fact Sheet (Reksadana) | MI → BLIPS      | Upload manual PDF + XLSX bulanan          | Bulanan                         |
| LDAP / SSO / IDP            | LDAP → BLIPS    | Active Directory atau SAML 2.0            | Continuous (auth)               |
| Email Server (SMTP)         | BLIPS → SMTP    | Outbound notification                     | Event-driven                    |
| Antivirus Scanner           | BLIPS → AV      | API on-upload virus scanning              | Per upload                      |
| Backup / DR                 | BLIPS → DR Site | Async replication                         | Continuous                      |

# 7\. Current State (As-Is) & Future State (To-Be)

## 7.1 As-Is — Process Flow Investasi Existing

Alur pengelolaan instrumen investasi pada kondisi As-Is bersifat fragmented, dengan multiple tools dan handoffs manual antar unit. Berikut alur tipikal untuk satu siklus penempatan obligasi:

22. Treasury Officer mendapat alokasi penempatan dari Komite Investasi (memo physical / email).

23. Treasury me-review term sheet & prospektus secara manual, judgment SPPI dilakukan ad-hoc.

24. Treasury input transaksi penempatan ke sistem GL legacy (hanya summary level).

25. Treasury upload dokumen ke shared drive dengan naming convention manual.

26. Akuntansi melakukan rekonsiliasi mingguan antara saldo broker dan GL.

27. Risk Officer membuat workbook ECL bulanan terpisah, mengambil data manual dari GL + spreadsheet master.

28. Risk Officer menghitung ECL via formula Excel, dengan parameter PD/LGD yang di-update triwulanan via copy-paste.

29. Risk Officer kirim hasil ECL ke Akuntansi via email; Akuntansi posting jurnal CKPN manual ke GL.

30. Setiap akhir bulan, Akuntansi melakukan closing manual dengan checklist physical; periode tidak di-lock di sistem.

31. Auditor eksternal melakukan substantive testing dengan request dokumen ad-hoc — beberapa dokumen tidak ditemukan atau tertukar.

## 7.2 Gap Analysis As-Is vs PSAK 71 Requirements

| **Area**               | **As-Is**                                              | **PSAK 71 Requirement**                                             | **Gap**                              |
| ---------------------- | ------------------------------------------------------ | ------------------------------------------------------------------- | ------------------------------------ |
| SPPI Test              | Judgment manual; tidak ter-dokumentasi.                | 10-question checklist + audit trail.                                | FULL — perlu engine SPPI.            |
| BM Test                | Implicit dari investment policy.                       | Indicator-based assessment per portofolio + 12-month sales history. | FULL — perlu engine BM Test.         |
| Klasifikasi            | Set manual saat masuk GL, tidak via matriks.           | Auto-derive dari SPPI × BM (matriks 6-cell).                        | FULL — perlu klasifikasi engine.     |
| EIR                    | Tidak ada — pakai simple interest.                     | Wajib EIR untuk AC & FVOCI utang.                                   | FULL — perlu EIR engine.             |
| ECL 3-Stage            | 1-Stage model lama (Excel).                            | 3-Stage (12M / Lifetime) dengan SICR & default trigger.             | FULL — perlu ECL engine.             |
| Forward-Looking        | Tidak ada (historical only).                           | Forward-looking adjustment via MEV.                                 | FULL — perlu Impact MEV engine.      |
| LPS Aggregator         | Tidak ada — Cash & Deposito tidak diagregasi per bank. | EAD = MAX(0; Total per Bank − LPS Coverage); proporsional alokasi.  | FULL — perlu logic LPS.              |
| Look-through Reksadana | Tidak ada — diperlakukan single exposure.              | Look-through ke underlying, ECL per komponen non-equity.            | FULL — perlu look-through engine.    |
| Multi-currency         | Manual conversion per laporan.                         | Otomatis ke IDR equivalent dengan kurs Tengah BI per event.         | FULL — perlu FX engine.              |
| Periode Buku Locking   | Manual cut-off, tanpa enforcement.                     | 3-status state machine (OPEN/SOFT\_CLOSED/CLOSED).                  | FULL — perlu periode buku module.    |
| Audit Trail            | Sebagian; tersebar di multiple tools.                  | Single dashboard, immutable history, SHA-256 hash dokumen.          | FULL — perlu integrated audit trail. |

## 7.3 To-Be — Vision & Target State

Sistem BLIPS IFRS 9 akan menjadi single source of truth untuk seluruh aktivitas pengelolaan instrumen investasi Tugure, dengan karakteristik:

  - End-to-end: pre-trade clearance (SPPI/BM Test) → penempatan → daily MTM/akrual → corporate actions → closure → reporting.

  - Real-time: posisi portofolio, ECL, exposure per counterparty/rating/stage tersedia secara live di dashboard.

  - Auditable: setiap event memiliki dokumen pendukung ter-link, audit trail immutable, hash integrity check.

  - Compliant: built-in compliance dengan PSAK 71/25/65, Basel III, regulasi BI/OJK.

  - Automated: jurnal otomatis ter-posting ke GL; alert otomatis untuk SICR, default, periode closing tenggat.

  - Governed: workflow Maker-Reviewer-Approver untuk seluruh transaksi material; segregation of duty hard-enforced.

  - Scalable: arsitektur cloud-ready dengan capacity untuk pertumbuhan portofolio 3-5x dalam 5 tahun.

## 7.4 Target Capability Matrix

| **Capability Area**         | **Target State**                                                                                        | **Modul SoW yang Mendukung** |
| --------------------------- | ------------------------------------------------------------------------------------------------------- | ---------------------------- |
| Klasifikasi & Reklasifikasi | Otomatis via engine SPPI + BM Test; matriks 6-cell; 6 kombinasi reklasifikasi prospektif.               | SoW Bab 4                    |
| Subsequent Measurement      | EIR-based untuk AC & FVOCI utang; FV-based untuk FVOCI/FVTPL.                                           | SoW Bab 5.12, 5.3            |
| Impairment (ECL)            | 3-Stage; 3-skenario; dual forward-looking; LPS aggregator; look-through reksadana.                      | SoW Bab 7, 8                 |
| Pendapatan Bunga            | Carrying × EIR ÷ 365 (harian); amortisasi premium/diskonto; Stage 3 net carrying.                       | SoW Bab 5.7, 5.12            |
| Multi-currency              | IDR equivalent universal; Kurs Tengah BI; FX gain/loss treatment per PSAK 71.                           | SoW Bab 5.1.8, 5.10          |
| Periode Akuntansi           | 3-status state machine; soft close H+5; hard close H+15; reopen via CFO.                                | SoW Bab 5.9                  |
| Jurnal & GL Integration     | Master Mapping Jurnal generic; resolusi runtime; balance check per posting.                             | SoW Bab 5.1.10, 5.11         |
| Audit & Compliance          | Single dashboard audit trail; SHA-256 hash dokumen; immutable history.                                  | SoW Bab 4.6, 5.8, 10         |
| Reporting                   | 25+ laporan termasuk CKPN roll-forward, Stage Distribution, Amortization Schedule, Closing Audit Trail. | SoW Bab 10.3                 |
| Risk Monitoring             | Real-time exposure, alert SICR/default, MEV sensitivity, stress test.                                   | SoW Bab 8.5, 10.3            |

# 8\. Business Requirements per Modul

Bab ini menetapkan Business Requirements (BR) per modul. Setiap requirement memiliki ID unik (BR-XXX-\#\#\#), deskripsi, prioritas (H/M/L), dan referensi ke SoW v1.1 untuk traceability. Detail spesifikasi teknis (field types, validations, screen layout) akan didokumentasikan di Functional Specification Document (FSD) per modul.

## 8.1 Master Data Management

Modul Master Data menyediakan registry data referensi yang dipakai seluruh transaksi dan perhitungan: Master Instrumen, Counterparty, Rating Pefindo, LGD Basel, PD Normal, Bobot Skenario, Periode Buku, Mata Uang & Kurs, Chart of Accounts, Mapping Jurnal.

| **BR-ID**  | **Requirement**                                                                                                                                                                                                                         | **Prio** | **Ref SoW**     |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | --------------- |
| BR-MAS-001 | Sistem WAJIB menyediakan CRUD Master Instrumen Investasi dengan auto-generate kode (CSH-, DEP-, OBL-, SHM-, RDN-).                                                                                                                      | H        | §5.1.1          |
| BR-MAS-002 | Sistem WAJIB menyimpan field klasifikasi PSAK 71 (AC/FVOCI/FVOCI\_ELECTION/FVTPL) yang ter-derived otomatis dari SPPI Result × BM Test Category.                                                                                        | H        | §5.1.1, 4.3     |
| BR-MAS-003 | Sistem WAJIB mengelola Master Counterparty dengan 6 tipe: BANK, BANK\_KUSTODIAN, KORPORASI, PEMERINTAH, MANAJER\_INVESTASI, EMITEN\_SAHAM, dengan field rating Pefindo & tipe eksposur Basel.                                           | H        | §5.1.2          |
| BR-MAS-004 | Sistem WAJIB menyimpan Counterparty Rating History dengan trigger otomatis SICR (≥2 notch downgrade) dan Default (rating idD atau gagal bayar \> 90 hari).                                                                              | H        | §5.1.2.a, 8.5.2 |
| BR-MAS-005 | Sistem WAJIB mengelola Master Mapping PD Normal per rating Pefindo (idAAA → idD) untuk PD 12-Month dan tabel Cumulative PD Multi-Year (Lifetime).                                                                                       | H        | §5.1.3, 5.1.3.a |
| BR-MAS-006 | Sistem WAJIB mengelola Master Mapping LGD Basel per tipe eksposur (Sovereign 0,4500; Senior Secured 0,2500; Senior Unsecured 0,4500; Subordinated 0,7500).                                                                              | H        | §5.1.4          |
| BR-MAS-007 | Sistem WAJIB mengelola Master Bobot Skenario PD (default Good 0,2500; Normal 0,5000; Bad 0,2500) yang dapat disesuaikan oleh Komite Risiko.                                                                                             | H        | §5.1.5          |
| BR-MAS-008 | Sistem WAJIB mengelola Master Periode Buku dengan hierarki Tahunan → Triwulanan → Bulanan, status (OPEN/SOFT\_CLOSED/CLOSED), dan auto-generate 12 periode bulanan + 4 triwulanan + 1 tahunan setiap awal tahun fiskal.                 | H        | §5.1.7, 5.9     |
| BR-MAS-009 | Sistem WAJIB mengelola Master Mata Uang & Kurs dengan kurs Tengah BI / JISDOR per tanggal; kurs ter-locked saat periode bulanan terkait HARD CLOSED.                                                                                    | H        | §5.1.8, 5.10    |
| BR-MAS-010 | Sistem WAJIB menyediakan Master Chart of Accounts (CoA) yang dapat di-import dari sistem ERP/GL existing via API atau Excel template.                                                                                                   | H        | §5.1.9          |
| BR-MAS-011 | Sistem WAJIB mengelola Master Mapping Jurnal dengan struktur Header-Detail untuk event-event akuntansi (PENEMPATAN, AKRUAL\_BUNGA, MTM\_FVOCI, ECL\_PEMBENTUKAN, dll), dengan resolusi runtime berdasarkan klasifikasi/tipe/underlying. | H        | §5.1.10, 5.11   |
| BR-MAS-012 | Sistem HARUS mendukung CRUD Master Portofolio untuk grouping instrumen (Treasury Liquidity, Investment, Trading, dll) — grouping menjadi unit Business Model Test.                                                                      | H        | §5.1, 4.2       |
| BR-MAS-013 | Setiap perubahan parameter master (PD, LGD, bobot skenario, MEV, Impact PD, kurs) WAJIB melewati workflow approval Maker → Approver dengan log perubahan (before/after).                                                                | H        | §10.2           |
| BR-MAS-014 | Setiap entitas master HARUS dapat diupload dengan dokumen pendukung (mis. publikasi Pefindo, screenshot BI JISDOR) dengan SHA-256 hash integrity.                                                                                       | H        | §5.8            |
| BR-MAS-015 | Master data SHOULD memiliki versi historis (slowly-changing dimension) untuk re-perform perhitungan retroaktif.                                                                                                                         | M        | §5.1.2.a        |

## 8.2 SPPI Test & Business Model Test

| **BR-ID**  | **Requirement**                                                                                                                                                                                                      | **Prio** | **Ref SoW**   |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------- |
| BR-SPP-001 | Sistem WAJIB menyediakan SPPI Test engine dengan 10-question checklist sesuai PSAK 71 §B4.1.7-B4.1.26, dengan auto-evaluate PASS/FAIL.                                                                               | H        | §4.1.2, 4.1.4 |
| BR-SPP-002 | SPPI Test result FAIL otomatis menghasilkan klasifikasi FVTPL (kecuali FVOCI Election untuk saham strategis).                                                                                                        | H        | §4.1.3        |
| BR-SPP-003 | Sistem WAJIB menyediakan Business Model Test dengan auto-suggest HTC/HTC\&S/Other berdasarkan indikator (frekuensi penjualan, volume, alasan, evaluasi kinerja manager).                                             | H        | §4.2          |
| BR-SPP-004 | Override BM Test result HANYA dengan justifikasi tertulis + approval Komite Investasi.                                                                                                                               | H        | §4.2.3        |
| BR-SPP-005 | Sistem WAJIB menyediakan Pre-Trade Clearance flow: Treasury Maker → SPPI Test → BM Test → Auto-derive Klasifikasi → Risk/Akuntansi Reviewer → Komite Investasi Approver, sebelum transaksi penempatan dapat diinput. | H        | §4.4.1        |
| BR-SPP-006 | Sistem WAJIB enforce urutan: Master Instrumen (dengan klasifikasi ter-lock) HARUS terbentuk dulu sebelum Transaksi Penempatan dapat diinput.                                                                         | H        | §4.4.1.a      |
| BR-SPP-007 | Sistem WAJIB menjalankan Periodic Review SPPI/BM Test minimal sekali setahun, dengan notifikasi 30 hari sebelum expired.                                                                                             | H        | §4.4.2        |
| BR-SPP-008 | Sistem WAJIB mendukung Triggered Reassessment otomatis bila: modifikasi kontrak material, perubahan kebijakan investasi, atau threshold volume penjualan terlampaui.                                                 | H        | §4.4.3        |
| BR-SPP-009 | Reklasifikasi WAJIB diterapkan prospektif (tanpa restate periode sebelumnya) dengan jurnal transisi otomatis untuk 6 kombinasi from-to (AC↔FVOCI↔FVTPL).                                                             | H        | §4.5          |
| BR-SPP-010 | Setiap SPPI/BM Test event WAJIB memiliki minimal 1 dokumen pendukung ter-upload (term sheet, prospektus, investment policy, memo Komite).                                                                            | H        | §4.6.1        |
| BR-SPP-011 | Sistem WAJIB menyimpan audit trail terpisah untuk SPPI Test dan BM Test (ID, versi, history jawaban, timestamp, IP user) — immutable.                                                                                | H        | §4.6          |
| BR-SPP-012 | Three-eyes principle WAJIB diberlakukan: Maker (Treasury) ≠ Reviewer (Risk/Akuntansi) ≠ Approver (Komite Investasi/CFO).                                                                                             | H        | §4.6          |

## 8.3 Modul Penempatan Instrumen

| **BR-ID**  | **Requirement**                                                                                                                                                                      | **Prio** | **Ref SoW** |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- | ----------- |
| BR-PNP-001 | Sistem WAJIB mencatat transaksi penempatan dengan auto-generate nomor transaksi (PNP-YYYY-\#\#\#\#\#).                                                                               | H        | §5.2.1      |
| BR-PNP-002 | Sistem WAJIB validasi: tanggal jatuh tempo \> tanggal penempatan untuk deposito & obligasi.                                                                                          | H        | §5.2.2      |
| BR-PNP-003 | Sistem WAJIB validasi: jumlah unit reksadana = total pembayaran ÷ NAB per unit (toleransi 4 desimal).                                                                                | H        | §5.2.2      |
| BR-PNP-004 | Sistem WAJIB validasi: rekening sumber dana memiliki saldo ≥ total pembayaran.                                                                                                       | H        | §5.2.2      |
| BR-PNP-005 | Sistem WAJIB validasi: counterparty aktif & memiliki Rating Pefindo valid (kecuali sovereign).                                                                                       | H        | §5.2.2      |
| BR-PNP-006 | Sistem WAJIB enforce minimal 1 dokumen bukti ter-upload sebelum transaksi dapat di-approve.                                                                                          | H        | §5.2.2      |
| BR-PNP-007 | Untuk obligasi: sistem WAJIB menghitung accrued interest dibeli (jika beli antara tanggal kupon).                                                                                    | H        | §5.2.1      |
| BR-PNP-008 | Untuk obligasi AC/FVOCI: sistem WAJIB menghitung EIR awal saat penempatan via Newton-Raphson IRR solver dan generate Amortization Schedule.                                          | H        | §5.12.4     |
| BR-PNP-009 | Sistem WAJIB workflow Maker (Treasury) → Approver (Treasury Manager) — pemisahan tugas 4-eyes.                                                                                       | H        | §5.2.1      |
| BR-PNP-010 | Sistem WAJIB posting jurnal otomatis sesuai Master Mapping Jurnal event PENEMPATAN, dengan akun debit ditentukan klasifikasi (AC → 1.1.2.001; FVOCI → 1.1.3.001; FVTPL → 1.1.4.001). | H        | §5.1.10     |
| BR-PNP-011 | Untuk instrumen valas: sistem WAJIB konversi ke IDR equivalent menggunakan kurs Tengah BI pada trade date.                                                                           | H        | §5.1.8      |
| BR-PNP-012 | Sistem HARUS validasi periode buku Tanggal Transaksi tidak dalam status CLOSED.                                                                                                      | H        | §5.9        |

## 8.4 Modul Mark-to-Market (MTM)

| **BR-ID**  | **Requirement**                                                                                                                                               | **Prio** | **Ref SoW**         |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------------- |
| BR-MTM-001 | Sistem WAJIB melakukan MTM harian untuk: Obligasi (FVOCI/AC monitoring), Saham (FVTPL/FVOCI), Reksadana (FVTPL/FVOCI).                                        | H        | §5.3.1              |
| BR-MTM-002 | Sumber harga: IBPA harian untuk obligasi; harga BEI close untuk saham; NAB MI/KSEI untuk reksadana.                                                           | H        | §5.3.1              |
| BR-MTM-003 | Selisih MTM untuk FVOCI utang → OCI; FVOCI Election saham → OCI no-recycling; FVTPL → P\&L.                                                                   | H        | §5.3.1              |
| BR-MTM-004 | Untuk AC: tidak ada jurnal MTM, hanya monitoring untuk impairment trigger.                                                                                    | H        | §5.3.1              |
| BR-MTM-005 | Sistem WAJIB upload harga harian (file IBPA, NAB report) sebelum job MTM running.                                                                             | H        | §5.3.2              |
| BR-MTM-006 | Untuk instrumen valas: MTM = (FX × Kurs hari ini) − Carrying IDR sebelumnya. FX revaluation pada monetary items WAJIB ke P\&L (bukan OCI).                    | H        | §5.1.8 FX treatment |
| BR-MTM-007 | Sistem WAJIB posting jurnal MTM otomatis dengan referensi instrumen dan tanggal valuasi.                                                                      | H        | §5.1.10             |
| BR-MTM-008 | Job MTM harian HARUS selesai dalam \< 30 menit untuk portofolio 1.000+ instrumen.                                                                             | H        | NFR Performance     |
| BR-MTM-009 | Bila harga referensi tidak tersedia pada hari kerja: sistem flag 'STALE\_PRICE' dan gunakan harga hari kerja sebelumnya untuk MTM, dengan alert ke Akuntansi. | M        | §5.3                |

## 8.5 Modul Renewal (Khusus Deposito)

| **BR-ID**  | **Requirement**                                                                                                                             | **Prio** | **Ref SoW** |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------- |
| BR-RNW-001 | Sistem WAJIB mendukung dua skema renewal: POKOK\_SAJA (bunga ditarik) dan POKOK\_PLUS\_BUNGA (bunga net digabung ke pokok).                 | H        | §5.4        |
| BR-RNW-002 | Untuk deposito Auto Renewal Flag = Y: sistem otomatis renewal saat jatuh tempo dengan rate baru sesuai instruksi.                           | H        | §5.6.1      |
| BR-RNW-003 | Sistem WAJIB generate Kode Instrumen baru untuk renewal (DEP-YYYY-\#\#\#\#\# baru) sambil menjaga link ke instrumen lama untuk audit trail. | H        | §5.4.1      |
| BR-RNW-004 | Untuk POKOK\_PLUS\_BUNGA: bunga net = bunga gross − PPh 4(2) Final 20%; pokok baru = pokok lama + bunga net.                                | H        | §5.4        |
| BR-RNW-005 | Sistem WAJIB upload bilyet baru atau surat instruksi rollover.                                                                              | H        | §5.4.1      |
| BR-RNW-006 | Untuk renewal yang melewati periode buku CLOSED: blocked, harus diproses pada periode terbuka berikutnya.                                   | H        | §5.9        |

## 8.6 Modul Penjualan / Pencairan

| **BR-ID**  | **Requirement**                                                                                             | **Prio** | **Ref SoW** |
| ---------- | ----------------------------------------------------------------------------------------------------------- | -------- | ----------- |
| BR-SLE-001 | Sistem WAJIB mendukung penjualan parsial dan penjualan penuh (full disposal).                               | H        | §5.5.1      |
| BR-SLE-002 | Sistem WAJIB auto-calc Realized Gain/Loss = Total Penerimaan − Carrying Amount Saat Jual − Biaya Transaksi. | H        | §5.5.1      |
| BR-SLE-003 | Untuk FVOCI utang: akumulasi OCI WAJIB di-recycle ke P\&L pada saat penjualan (event REKLAS\_OCI\_PL).      | H        | §5.1.10     |
| BR-SLE-004 | Untuk FVOCI Election Saham: akumulasi OCI TIDAK di-recycle ke P\&L (no-recycling), tetap di OCI permanent.  | H        | §4.3        |
| BR-SLE-005 | Sistem WAJIB enforce upload konfirmasi penjualan / redemption confirmation.                                 | H        | §5.5.1      |
| BR-SLE-006 | Untuk break deposito (penjualan sebelum JT): sistem hitung penalty sesuai term sheet (jika ada).            | M        | §5.5        |
| BR-SLE-007 | Untuk obligasi AC: jika dijual sebelum JT, trigger evaluasi BM Test (frekuensi penjualan vs threshold HTC). | H        | §4.2.2      |
| BR-SLE-008 | Sistem WAJIB de-activate Amortization Schedule bila instrumen AC/FVOCI utang dijual sepenuhnya.             | H        | §5.12.13    |

## 8.7 Modul Jatuh Tempo (Closure)

| **BR-ID**  | **Requirement**                                                                                                                  | **Prio** | **Ref SoW** |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------- |
| BR-MAT-001 | Sistem WAJIB tampilkan list instrumen jatuh tempo H-1 sebagai dashboard reminder.                                                | H        | §5.6        |
| BR-MAT-002 | Pada tanggal JT: settlement otomatis untuk deposito & obligasi (pokok + kupon final ke rekening).                                | H        | §5.6.1      |
| BR-MAT-003 | Untuk obligasi FVOCI utang: pada JT, akumulasi OCI di-recycle ke P\&L.                                                           | H        | §5.1.10     |
| BR-MAT-004 | Untuk obligasi AC: Closing Carrying pada baris terakhir Amortization Schedule HARUS sama dengan nilai par (toleransi ±0,01 IDR). | H        | §5.12.13    |
| BR-MAT-005 | Sistem WAJIB update status instrumen menjadi JATUH\_TEMPO dan deaktivasi schedule.                                               | H        | §5.6.1      |
| BR-MAT-006 | Reksadana tidak memiliki jatuh tempo (perpetual); hanya bisa di-redeem via Modul Penjualan.                                      | H        | §5.6.1      |

## 8.8 Modul Pendapatan Investasi

| **BR-ID**  | **Requirement**                                                                                                                                                                                         | **Prio** | **Ref SoW**    |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------------- |
| BR-PND-001 | Sistem WAJIB hitung akrual harian untuk: bunga cash (saldo × rate), bunga deposito (Carrying × EIR ÷ 365 untuk EIR Method=Y), kupon obligasi (Carrying × EIR ÷ 365 dengan amortisasi premium/diskonto). | H        | §5.7, 5.12.7   |
| BR-PND-002 | Sistem WAJIB posting jurnal akrual harian di end-of-day.                                                                                                                                                | H        | §5.1.10        |
| BR-PND-003 | PPh 4(2) Final 20% bunga deposito: diakui saat realisasi (penerimaan bunga). Jurnal: D Kas (80%) + D Beban PPh (20%) = K Akrual Bunga.                                                                  | H        | §5.7, 5.1.10   |
| BR-PND-004 | PPh Final 10% kupon obligasi korporasi: diakui saat penerimaan kupon.                                                                                                                                   | H        | §5.7           |
| BR-PND-005 | Dividen saham: diakui pada cum-date / ex-date sesuai kebijakan akuntansi; PPh Final 10% (WP OP) atau PP 9/2021 exemption (WP Badan reinvestasi).                                                        | H        | §5.7           |
| BR-PND-006 | Untuk saham FVOCI Election: dividen diakui di P\&L (tidak ke OCI), kontras dengan MTM yang ke OCI.                                                                                                      | H        | §4.3           |
| BR-PND-007 | Distribusi reksadana: diakui saat dibagikan oleh MI; tidak kena PPh Final.                                                                                                                              | H        | §5.7           |
| BR-PND-008 | Untuk Stage 3 (credit-impaired): pendapatan bunga = Net Carrying (post-CKPN) × EIR, bukan Gross.                                                                                                        | H        | §5.12.9, 8.5.5 |

## 8.9 Modul Media Upload

| **BR-ID**  | **Requirement**                                                                                                         | **Prio** | **Ref SoW**     |
| ---------- | ----------------------------------------------------------------------------------------------------------------------- | -------- | --------------- |
| BR-UPL-001 | Sistem WAJIB menyediakan repositori dokumen terenkripsi (S3 + KMS atau equivalent) untuk semua bukti dokumen transaksi. | H        | §5.8            |
| BR-UPL-002 | Setiap upload WAJIB menghasilkan SHA-256 hash untuk integrity check.                                                    | H        | §5.8            |
| BR-UPL-003 | Sistem WAJIB menyimpan metadata: uploader, timestamp, IP, file size, file type, related event/transaksi.                | H        | §10.2           |
| BR-UPL-004 | Akses dokumen (view/download) WAJIB dicatat di access log.                                                              | H        | §10.2           |
| BR-UPL-005 | Setiap upload WAJIB melalui virus scanning sebelum tersimpan.                                                           | H        | NFR Security    |
| BR-UPL-006 | Format file yang didukung: PDF (utama), JPG/PNG (gambar), XLSX/CSV (data), DOCX (memo).                                 | H        | §5.8            |
| BR-UPL-007 | Maksimum file size per upload: 50 MB. Bila lebih besar, sistem split atau reject dengan pesan jelas.                    | M        | NFR Performance |
| BR-UPL-008 | Retention: dokumen disimpan minimal 10 tahun (compliance audit + statute of limitation pajak).                          | H        | NFR Compliance  |
| BR-UPL-009 | Dokumen TIDAK dapat dihapus oleh user; hanya bisa di-mark INACTIVE oleh CFO dengan justifikasi.                         | H        | §5.8            |

## 8.10 ECL Engine — Expected Credit Loss

ECL Engine merupakan core compliance modul yang menghitung Expected Credit Loss berbasis PSAK 71 dengan model 3-Stage, 3-skenario probabilitas default, dan dual forward-looking layer.

| **BR-ID**  | **Requirement**                                                                                                                                                           | **Prio** | **Ref SoW** |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ----------- |
| BR-ECL-001 | Sistem WAJIB hitung ECL untuk klasifikasi AC dan FVOCI utang. Tidak ada ECL untuk FVTPL, saham individu, atau reksadana FVTPL (risk-management view saja).                | H        | §4.3, Bab 8 |
| BR-ECL-002 | ECL dihitung per skenario: Optimistic (Good), Base (Normal), Pessimistic (Bad), kemudian di-weighted average sesuai bobot Komite Risiko.                                  | H        | §7.1, 5.1.5 |
| BR-ECL-003 | Formula inti: ECL\_skenario = EAD\_IDR × PD\_skenario × LGD; ECL Weighted = Σ (w\_skenario × ECL\_skenario); ECL FL = ECL Weighted × Impact PD.                           | H        | §7.1        |
| BR-ECL-004 | PD Good = PD Normal × Impact MEV (Good); PD Bad = PD Normal × Impact MEV (Bad). PD Normal langsung dari Pefindo.                                                          | H        | §7.1, 5.8.3 |
| BR-ECL-005 | Stage 1 menggunakan PD 12-Month; Stage 2 dan Stage 3 menggunakan Lifetime PD (cumulative atau derivasi).                                                                  | H        | §8.5.1      |
| BR-ECL-006 | Sistem WAJIB lakukan staging otomatis: Stage 1 (performing, no SICR), Stage 2 (SICR), Stage 3 (default/credit-impaired) berdasarkan trigger di §8.5.2.                    | H        | §8.5        |
| BR-ECL-007 | Trigger SICR otomatis: rating downgrade ≥ 2 notch dari origination, atau berpindah ke non-investment grade, atau DPD 30-90 hari, atau outlook NEGATIVE 2 review berturut. | H        | §8.5.2-A    |
| BR-ECL-008 | Trigger Default otomatis: rating idD, atau DPD \> 90 hari, atau PKPU/Pailit, atau forced restructuring.                                                                   | H        | §8.5.2-B    |
| BR-ECL-009 | Curing (migrasi mundur Stage 3→2→1) HARUS bertahap dengan probationary period 3-6 bulan dan approval Komite Risiko.                                                       | H        | §8.5.2-C    |
| BR-ECL-010 | Untuk Cash & Deposito: WAJIB pakai LPS aggregator. EAD = MAX(0; Σ Cash + Σ Deposito per Bank − LPS Coverage Rp 2 Miliar). EAD dialokasikan proporsional.                  | H        | §8.1.2      |
| BR-ECL-011 | Untuk Reksadana FVTPL: ECL look-through dihitung tetapi HANYA sebagai risk-management view, tidak masuk laporan keuangan.                                                 | H        | §8.3        |
| BR-ECL-012 | Untuk Reksadana FVOCI: ECL look-through DIAKUI di laporan keuangan (beban CKPN ke P\&L, kontra ke OCI).                                                                   | H        | §8.3, 4.3   |
| BR-ECL-013 | Untuk Reksadana Saham (semua klasifikasi): TIDAK ADA perhitungan ECL — underlying ekuitas tidak menghasilkan PD.                                                          | H        | §8.3.4      |
| BR-ECL-014 | Untuk Reksadana Campuran: ECL HANYA pada komponen non-equity (obligasi, deposito, cash). Komponen ekuitas EXCLUDED.                                                       | H        | §8.3.3      |
| BR-ECL-015 | Job hitung ECL akhir bulan WAJIB berjalan otomatis pada tanggal cut-off, dengan parameter PD/LGD/MEV/Impact PD versi terkini.                                             | H        | §7.2        |
| BR-ECL-016 | Bobot skenario default Good 0,2500; Normal 0,5000; Bad 0,2500. Sum HARUS = 1,0000 (toleransi ±0,01%).                                                                     | H        | §5.1.5      |
| BR-ECL-017 | Impact MEV to PD per periode WAJIB di-upload oleh Risk Officer setelah approval ALCO (XLSX dengan dokumen pendukung).                                                     | H        | §5.8.3      |
| BR-ECL-018 | Impact PD multiplier final WAJIB di-upload terpisah oleh Risk Officer (default 1,0000 → 1,1500).                                                                          | H        | §5.8.3.a    |
| BR-ECL-019 | Sistem WAJIB posting jurnal CKPN otomatis: untuk AC kontra-aset (1.1.9.001); untuk FVOCI ke OCI (3.2.1.003).                                                              | H        | §5.1.10     |
| BR-ECL-020 | Sistem WAJIB hasilkan ECL Detail Report dengan tampilan rumusan: EAD × PD (12M/Lifetime, 3 skenario) × LGD × Impact PD per instrumen — drill-down ke level perhitungan.   | H        | §10.3       |
| BR-ECL-021 | Presisi: rasio (PD, LGD, MEV, Impact PD, bobot) 4 desimal; nilai IDR 2 desimal internal; pembulatan Banker's Rounding.                                                    | H        | §7.3        |
| BR-ECL-022 | Sistem HARUS menyediakan MEV Sensitivity Report (sensitivitas ECL terhadap perubahan GDP, CPI, BI Rate, USD/IDR) untuk stress test.                                       | M        | §10.3       |
| BR-ECL-023 | Re-run ECL pada periode SOFT\_CLOSED diperbolehkan bila ada parameter update; pada periode CLOSED tidak boleh.                                                            | H        | §5.1.7, 5.9 |
| BR-ECL-024 | Untuk Stage 3: PD = 1,0000 saat default actual; ECL = EAD × LGD; bunga selanjutnya pada Net Carrying.                                                                     | H        | §8.5.5      |

## 8.11 EIR & Amortisasi (Modul Baru v1.1)

Modul EIR & Amortisasi memenuhi PSAK 71 §5.4 dan Lampiran A untuk subsequent measurement klasifikasi AC dan FVOCI utang. Modul ini ditambahkan di SoW v1.1 untuk mengisi gap yang ditemukan pada review BRD v0.5.

| **BR-ID**  | **Requirement**                                                                                                                                                                                                                           | **Prio** | **Ref SoW**      |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ---------------- |
| BR-EIR-001 | Sistem WAJIB hitung EIR otomatis saat penempatan instrumen AC atau FVOCI utang, menggunakan Newton-Raphson IRR solver.                                                                                                                    | H        | §5.12.4          |
| BR-EIR-002 | EIR dihitung dari Carrying Awal = Harga Beli + Biaya Transaksi Kapitalisasi, sebagai r yang memenuhi P0 = Σ CFt/(1+r)^t.                                                                                                                  | H        | §5.12.3          |
| BR-EIR-003 | Konvergensi solver: |f(rk+1)| \< 0,00000001 dalam maksimum 50 iterasi. Bila tidak konvergen → exception 'EIR\_NOT\_CONVERGED' + escalate manual review.                                                                                   | H        | §5.12.4          |
| BR-EIR-004 | EIR disimpan dengan presisi 8 desimal internal (NUMERIC 12,8); ditampilkan 4 desimal di laporan.                                                                                                                                          | H        | §5.12.4          |
| BR-EIR-005 | Sistem WAJIB generate Amortization Schedule per instrumen dengan field: Periode, Tanggal Posting, Opening Carrying, Cash Inflow, Pendapatan Bunga EIR, Amortisasi Premium/Diskonto, Closing Carrying, EIR Periode, Stage, Status Posting. | H        | §5.12.6          |
| BR-EIR-006 | Closing Carrying baris terakhir (jatuh tempo) HARUS sama dengan nilai par + kupon final (toleransi ±0,01 IDR via Banker's Rounding).                                                                                                      | H        | §5.12.13         |
| BR-EIR-007 | Akrual bunga harian = Carrying × EIR ÷ 365 (ACT/365 default). Stage 1 & 2 berbasis Gross Carrying; Stage 3 berbasis Net Carrying.                                                                                                         | H        | §5.12.7          |
| BR-EIR-008 | Selisih antara Pendapatan Bunga EIR dan Kupon Kontraktual nominal = Amortisasi Premium (negatif, mengurangi carrying) atau Diskonto (positif, menambah carrying).                                                                         | H        | §5.12.7          |
| BR-EIR-009 | Untuk klasifikasi FVTPL: EIR & Amortisasi TIDAK aktif. Biaya transaksi langsung ke P\&L; tidak ada amortization schedule.                                                                                                                 | H        | §5.12.5          |
| BR-EIR-010 | Untuk klasifikasi FVOCI Election Saham: EIR & Amortisasi TIDAK aktif (instrumen ekuitas).                                                                                                                                                 | H        | §5.12.5          |
| BR-EIR-011 | Sistem WAJIB mendukung Re-estimation EIR untuk dua kondisi: (a) Modifikasi material → derecognition + EIR baru; (b) Revisi cash flow non-material → EIR original tetap, recompute carrying dengan catch-up adjustment.                    | H        | §5.12.8          |
| BR-EIR-012 | Modifikasi material trigger: perubahan kupon ≥ 10%, perpanjangan tenor signifikan, perubahan currency, atau perubahan counterparty.                                                                                                       | H        | §5.12.8          |
| BR-EIR-013 | Re-estimation HARUS via workflow: Treasury/Risk Maker → Akuntansi Reviewer → Finance Controller/CFO Approver, dengan dokumen amandemen.                                                                                                   | H        | §5.12.13         |
| BR-EIR-014 | Bila instrumen direklasifikasi: sistem WAJIB recompute EIR sesuai matriks reklasifikasi (lihat §5.12.10) — mis. FVOCI→AC: EIR baru dari FV pada tanggal reklasifikasi sebagai carrying baru.                                              | H        | §5.12.10         |
| BR-EIR-015 | Sistem WAJIB hasilkan Amortization Schedule Report, EIR Summary Report, Roll-Forward Carrying Amount Report, dan EIR Re-estimation Log Report.                                                                                            | H        | §10.3 (NEW v1.1) |
| BR-EIR-016 | Day Count Convention default ACT/365; alternatif 30/360 dapat di-override per instrumen via field di Master.                                                                                                                              | H        | §5.12.7          |
| BR-EIR-017 | Sistem WAJIB posting jurnal AMORTISASI\_PREMI\_DISKONTO bersamaan dengan AKRUAL\_BUNGA untuk instrumen EIR Method Flag = Y.                                                                                                               | H        | §5.1.10 (NEW)    |
| BR-EIR-018 | Untuk Cash di Bank dengan rate variabel harian: EIR Method Flag = N (simple interest). Untuk rate tetap: EIR Method Flag = Y.                                                                                                             | H        | §5.12.2          |
| BR-EIR-019 | Audit trail re-estimation WAJIB simpan: before/after EIR, before/after carrying, trigger event, dokumen pendukung, user maker & approver, timestamp.                                                                                      | H        | §5.12.13         |
| BR-EIR-020 | Sistem HARUS menyediakan fallback ke metode bisection bila Newton-Raphson gagal untuk cash flow non-konvensional.                                                                                                                         | M        | §5.12.4          |

## 8.12 Modul Periode Buku (Financial Period Management)

| **BR-ID**  | **Requirement**                                                                                                                                             | **Prio** | **Ref SoW**  |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | ------------ |
| BR-PRD-001 | Sistem WAJIB mengelola periode buku dengan hierarki Tahunan → Triwulanan → Bulanan, auto-generate setiap awal Tahun Buku baru.                              | H        | §5.1.7       |
| BR-PRD-002 | Periode memiliki 3 status: OPEN (boleh input/edit), SOFT\_CLOSED (hanya Akuntansi adjustment), CLOSED (hard-locked).                                        | H        | §5.1.7, 5.9  |
| BR-PRD-003 | Soft Close: H+5 hari kerja setelah akhir bulan; workflow Akuntansi Maker → Finance Controller Approver.                                                     | H        | §5.9         |
| BR-PRD-004 | Hard Close: maksimal H+15 hari kerja setelah akhir bulan; workflow Akuntansi Maker → CFO Approver.                                                          | H        | §5.9         |
| BR-PRD-005 | Periode triwulanan auto soft-close ketika 3 periode bulanan SOFT\_CLOSED; hard-close memerlukan laporan triwulanan + approval CFO.                          | H        | §5.9         |
| BR-PRD-006 | Periode tahunan hard-close memerlukan: audit eksternal selesai, approval Komite Audit, approval Direksi.                                                    | H        | §5.9         |
| BR-PRD-007 | Reopen periode CLOSED HANYA dengan persetujuan CEO atau Komite Audit, dengan memo justifikasi + audit trail Reopened Flag.                                  | H        | §5.9         |
| BR-PRD-008 | Pada SOFT\_CLOSED: hanya Akuntansi yang dapat melakukan adjustment journal entry (event PERIODE\_ADJUSTMENT).                                               | H        | §5.9, 5.1.10 |
| BR-PRD-009 | Pada CLOSED: tidak ada user yang dapat input/edit; koreksi error WAJIB via prior-period adjustment journal entry pada periode terbuka berikutnya (PSAK 25). | H        | §5.9         |
| BR-PRD-010 | Sistem WAJIB validasi setiap input transaksi: cek Status Periode dari Tanggal Transaksi.                                                                    | H        | §5.9         |
| BR-PRD-011 | Forward-dated entry (Tanggal Transaksi di masa depan): blocked. Backdated ke periode SOFT\_CLOSED: hanya Akuntansi.                                         | H        | §5.9         |
| BR-PRD-012 | Sistem WAJIB tampilkan Status Periode Dashboard dengan timeline 12 bulan + warna status + tenggat soft/hard close.                                          | H        | §10.3        |
| BR-PRD-013 | Notifikasi otomatis: H-3 sebelum tenggat soft-close; H-3 sebelum tenggat hard-close; saat ada adjustment di SOFT\_CLOSED.                                   | H        | §5.9         |
| BR-PRD-014 | Sistem WAJIB hasilkan Closing Audit Trail Report per periode dengan daftar adjustment journal entry, maker, approver, alasan, dokumen.                      | H        | §10.3        |

## 8.13 Modul FX Rate Management

| **BR-ID** | **Requirement**                                                                                                                                                          | **Prio** | **Ref SoW**         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- | ------------------- |
| BR-FX-001 | Sistem WAJIB menyimpan kurs harian per mata uang (USD, SGD, EUR, JPY, AUD, CNY, MYR, GBP, dll) dengan kurs Tengah BI / JISDOR sebagai sumber resmi.                      | H        | §5.1.8              |
| BR-FX-002 | Kurs USD WAJIB di-update otomatis dari BI JISDOR pada hari kerja jam 10:30 WIB via scheduled job.                                                                        | H        | §5.10               |
| BR-FX-003 | Kurs lainnya WAJIB di-upload manual dengan dokumen pendukung (PDF/screenshot publikasi BI), workflow Akuntansi Maker → Finance Controller Approver.                      | H        | §5.10               |
| BR-FX-004 | Bila pada hari kerja kurs BI tidak tersedia: gunakan kurs hari kerja sebelumnya, flag REPEAT\_RATE untuk audit trail.                                                    | H        | §5.1.8              |
| BR-FX-005 | Kombinasi (Kode Mata Uang × Tanggal Berlaku) WAJIB unique.                                                                                                               | H        | §5.1.8              |
| BR-FX-006 | Saat periode bulanan HARD CLOSED: seluruh kurs di periode tersebut auto Locked Flag = Y; tidak dapat diubah.                                                             | H        | §5.1.8, 5.10        |
| BR-FX-007 | Koreksi kurs di periode CLOSED HARUS via prior-period FX adjustment journal entry (PSAK 25) di periode terbuka berikutnya.                                               | H        | §5.10               |
| BR-FX-008 | Hierarki kurs per event akuntansi: Penempatan = trade date; Akrual harian = tanggal akrual; MTM = closing date; ECL akhir bulan = period-end; Closure = settlement date. | H        | §5.1.8              |
| BR-FX-009 | FX Gain/Loss treatment: Unrealized harian → P\&L untuk FVTPL/AC, OCI utang tetap ke P\&L (FX revaluation pada monetary items), saham FVOCI Election ke OCI.              | H        | §5.1.8 FX treatment |
| BR-FX-010 | Realized FX Gain/Loss saat closure event ke P\&L (akun 4.1.4.001 Realized FX).                                                                                           | H        | §5.1.10             |
| BR-FX-011 | Semua perhitungan EAD/ECL/MTM internal WAJIB dalam IDR equivalent. Mata uang asli tetap disimpan untuk audit trail & disclosure.                                         | H        | §5.1.8              |
| BR-FX-012 | Sistem WAJIB hasilkan Posisi Valas Dashboard (eksposur per mata uang dalam asli & IDR), FX Gain/Loss Report, FX Rate History Report.                                     | H        | §5.10, 10.3         |
| BR-FX-013 | Notifikasi otomatis bila ada hari kerja tanpa kurs ter-upload (alert ke Akuntansi).                                                                                      | H        | §5.10               |

## 8.14 Modul Mapping Jurnal & GL Interface

| **BR-ID**  | **Requirement**                                                                                                                                                                                                                                                                                                                                                                                                         | **Prio** | **Ref SoW**     |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | --------------- |
| BR-JNL-001 | Sistem WAJIB menyediakan Master Mapping Jurnal dengan struktur Header-Detail. Header berisi event-level info; Detail berisi line debit/kredit dengan filter klasifikasi/tipe/underlying.                                                                                                                                                                                                                                | H        | §5.1.10         |
| BR-JNL-002 | Event-event yang ter-cover: PENEMPATAN, AKRUAL\_BUNGA, AMORTISASI\_PREMI\_DISKONTO (NEW), MTM\_FVOCI, MTM\_FVTPL, PEMBAYARAN\_BUNGA/KUPON, PENERIMAAN\_DIVIDEN, DISTRIBUSI\_REKSADANA, ECL\_PEMBENTUKAN, ECL\_REVERSAL, PENJUALAN\_PENCAIRAN, JATUH\_TEMPO, REKLAS\_OCI\_PL, FX\_UNREALIZED, FX\_REALIZED, STAGE\_MIGRATION, EIR\_REESTIMATION, MODIFIKASI\_MATERIAL, PERIODE\_ADJUSTMENT, CORRECTION\_PERIODE\_CLOSED. | H        | §5.1.10         |
| BR-JNL-003 | Resolusi runtime: saat event terpicu, sistem ambil semua line detail Aktif, evaluasi filter (Klasifikasi/Tipe/Underlying), pilih line yang lulus, ambil amount sesuai Sumber Amount × Multiplier, post jurnal.                                                                                                                                                                                                          | H        | §5.1.10         |
| BR-JNL-004 | Validasi balance: total Debit = total Kredit per event posting (toleransi ±0,01 IDR). Bila tidak balance → block posting + alert Akuntansi.                                                                                                                                                                                                                                                                             | H        | §5.1.10         |
| BR-JNL-005 | Sistem WAJIB mendukung integrasi ke GL host via API REST atau file batch (CSV/XML) sesuai standar GL Tugure.                                                                                                                                                                                                                                                                                                            | H        | §5.1.10         |
| BR-JNL-006 | Master Mapping Jurnal HARUS dapat di-edit via UI (dengan validasi real-time) dan via Excel import/export untuk bulk update.                                                                                                                                                                                                                                                                                             | H        | §5.11           |
| BR-JNL-007 | Setiap perubahan mapping WAJIB tercatat audit trail dengan user, timestamp, before/after value, tipe operasi.                                                                                                                                                                                                                                                                                                           | H        | §5.11, 10.2     |
| BR-JNL-008 | Sistem WAJIB hasilkan: Mapping Coverage Dashboard (event aktif vs tidak aktif), Mapping Validation Report (event gagal balance), Mapping Change History, CoA Cross-Reference Report.                                                                                                                                                                                                                                    | H        | §10.3           |
| BR-JNL-009 | Bila API GL host down: sistem queue jurnal di outbound buffer; retry dengan exponential backoff; alert ke IT bila gagal \> 3 kali.                                                                                                                                                                                                                                                                                      | H        | NFR Reliability |
| BR-JNL-010 | Mata uang posting: default IDR. Semua posting dalam IDR equivalent (PSAK 71 single functional currency).                                                                                                                                                                                                                                                                                                                | H        | §5.1.8          |
| BR-JNL-011 | Sistem HARUS mendukung reverse posting otomatis untuk transaksi yang di-cancel sebelum periode CLOSED.                                                                                                                                                                                                                                                                                                                  | M        | §5.1.10         |

## 8.15 Modul Reporting & Dashboard

Sistem menyediakan reporting komprehensif untuk kebutuhan operational, manajemen, audit, dan disclosure regulator. Total ada 25+ laporan yang dapat di-akses via dashboard interaktif, export Excel/PDF, scheduled email, dan API endpoint.

| **BR-ID**  | **Requirement**                                                                                                                                                                                        | **Prio** | **Ref SoW**     |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- | --------------- |
| BR-RPT-001 | Sistem WAJIB hasilkan Daftar Posisi Portofolio (per tanggal): tipe instrumen, sub-tipe, counterparty, EAD, MTM, accrued interest, Stage, ECL FL, status.                                               | H        | §10.3           |
| BR-RPT-002 | Sistem WAJIB hasilkan Mutasi Instrumen per periode (penempatan, MTM, jual, renewal, jatuh tempo).                                                                                                      | H        | §10.3           |
| BR-RPT-003 | Sistem WAJIB hasilkan P\&L Investasi (bunga, kupon, dividen, distribusi, MTM gain/loss, realized gain/loss, beban PPh).                                                                                | H        | §10.3           |
| BR-RPT-004 | Sistem WAJIB hasilkan Akrual Bunga Report (per instrumen, harian/bulanan) dengan reconciliation ke GL.                                                                                                 | H        | §10.3           |
| BR-RPT-005 | Sistem WAJIB hasilkan ECL Summary (per instrumen, counterparty, stage, periode; ECL Weighted, ECL FL, Δ vs periode lalu).                                                                              | H        | §10.3           |
| BR-RPT-006 | Sistem WAJIB hasilkan ECL Detail dengan tampilan formula (EAD × PD × LGD × Impact PD per instrumen).                                                                                                   | H        | §10.3           |
| BR-RPT-007 | Sistem WAJIB hasilkan Saldo CKPN Report dengan roll-forward (opening, addition, migration, curing, parameter delta, write-off, closure → ending) per stage per tipe instrumen.                         | H        | §10.3.1         |
| BR-RPT-008 | Sistem WAJIB hasilkan Stage Distribution Report (distribusi exposure per stage, tipe, counterparty; matrix transition period-on-period).                                                               | H        | §10.3           |
| BR-RPT-009 | Sistem WAJIB hasilkan Counterparty Rating History Report (riwayat rating, action, notch, SICR/Default flag).                                                                                           | H        | §10.3           |
| BR-RPT-010 | Sistem WAJIB hasilkan Underlying Reksadana Snapshot per tanggal dengan komposisi & alert ketidaksesuaian sub-tipe.                                                                                     | H        | §10.3           |
| BR-RPT-011 | Sistem WAJIB hasilkan Konsentrasi Risiko (per counterparty, rating, tipe, sub-tipe).                                                                                                                   | H        | §10.3           |
| BR-RPT-012 | Sistem WAJIB hasilkan LPS Coverage Report (per bank: total eksposur, LPS, eksposur tak terjamin).                                                                                                      | H        | §10.3           |
| BR-RPT-013 | Sistem WAJIB hasilkan MEV Sensitivity Report (sensitivitas ECL terhadap MEV; stress test result).                                                                                                      | H        | §10.3           |
| BR-RPT-014 | Sistem WAJIB hasilkan Status Periode Dashboard (timeline 12 bulan, warna status, tenggat closing, SLA).                                                                                                | H        | §10.3           |
| BR-RPT-015 | Sistem WAJIB hasilkan Closing Audit Trail Report (adjustment journal entry per periode SOFT\_CLOSED).                                                                                                  | H        | §10.3           |
| BR-RPT-016 | Sistem WAJIB hasilkan Posisi Valas Dashboard, FX Gain/Loss Report, FX Rate History Report.                                                                                                             | H        | §10.3           |
| BR-RPT-017 | Sistem WAJIB hasilkan Amortization Schedule per Instrumen Report (NEW v1.1).                                                                                                                           | H        | §10.3 NEW       |
| BR-RPT-018 | Sistem WAJIB hasilkan EIR Summary Report (NEW v1.1) — EIR Awal vs Current, premium/diskonto, biaya transaksi, sisa amortisasi.                                                                         | H        | §10.3 NEW       |
| BR-RPT-019 | Sistem WAJIB hasilkan Roll-Forward Carrying Amount Report (NEW v1.1) memenuhi PSAK 71 §35H disclosure.                                                                                                 | H        | §10.3 NEW       |
| BR-RPT-020 | Sistem WAJIB hasilkan EIR Re-estimation Log Report (NEW v1.1).                                                                                                                                         | H        | §10.3 NEW       |
| BR-RPT-021 | Sistem WAJIB hasilkan Mapping Coverage Dashboard, Mapping Validation Report, Mapping Change History, CoA Cross-Reference Report.                                                                       | H        | §10.3           |
| BR-RPT-022 | Format output: dashboard interaktif (web), export Excel/PDF dengan watermark + timestamp + user, scheduled email (bulanan/triwulanan), API endpoint untuk integrasi sistem manajemen risiko/regulator. | H        | §10.3.1-D       |
| BR-RPT-023 | Filter & drill-down: Tipe/Sub-Tipe, Counterparty/Rating, Stage, Klasifikasi PSAK 71, Periode, dengan kemampuan drill-down dari saldo agregat → instrumen individu → detail event/jurnal.               | H        | §10.3.1-C       |
| BR-RPT-024 | Sistem HARUS mendukung custom report builder untuk power user (Risk, Akuntansi) dengan template dasar.                                                                                                 | M        | —               |
| BR-RPT-025 | Real-time dashboard refresh maksimal latency 5 detik untuk view standard.                                                                                                                              | H        | NFR Performance |

## 8.16 Modul Document Management & Audit Trail

| **BR-ID**  | **Requirement**                                                                                              | **Prio** | **Ref SoW** |
| ---------- | ------------------------------------------------------------------------------------------------------------ | -------- | ----------- |
| BR-DOC-001 | Setiap event transaksi material WAJIB memiliki minimal 1 dokumen pendukung ter-link.                         | H        | §4.6.1, 5.8 |
| BR-DOC-002 | Sistem WAJIB enforce daftar wajib upload per event (lihat SoW §4.6.1).                                       | H        | §4.6.1      |
| BR-DOC-003 | Audit trail per transaksi WAJIB simpan: Created By/At, Approved By/At, Last Modified By/At, status changes.  | H        | §10.2       |
| BR-DOC-004 | Audit trail per dokumen upload WAJIB simpan: uploader, timestamp, IP, hash SHA-256, file metadata.           | H        | §10.2       |
| BR-DOC-005 | Akses dokumen (view/download) WAJIB dicatat di access log dengan timestamp, user, action.                    | H        | §10.2       |
| BR-DOC-006 | Modifikasi parameter master (PD, LGD, Impact PD, LPS, kurs) WAJIB workflow approval dengan log before/after. | H        | §10.2       |
| BR-DOC-007 | Auditor Read-Only role WAJIB memiliki akses penuh ke audit trail dan media upload tanpa modifikasi.          | H        | §10.1       |
| BR-DOC-008 | Audit trail TIDAK BOLEH dapat dihapus oleh user manapun (immutable).                                         | H        | §10.2       |
| BR-DOC-009 | Sistem HARUS mendukung audit trail export untuk forensic analysis.                                           | M        | —           |

# 9\. Non-Functional Requirements (NFR)

Non-Functional Requirements (NFR) menetapkan kualitas operasional sistem di luar fungsionalitas. NFR diukur secara kuantitatif (mis. response time, uptime%) dan dijadikan acceptance criteria saat UAT serta SLA selama operasional.

## 9.1 Performance Requirements

| **NFR-ID**  | **Requirement**                                                  | **Target**                                                           | **Pengukuran**               |
| ----------- | ---------------------------------------------------------------- | -------------------------------------------------------------------- | ---------------------------- |
| NFR-PERF-01 | Response time UI screen transaksi (CRUD master, input transaksi) | ≤ 2 detik (P95)                                                      | Synthetic monitoring browser |
| NFR-PERF-02 | Response time dashboard refresh (real-time view)                 | ≤ 5 detik (P95)                                                      | Synthetic monitoring         |
| NFR-PERF-03 | Response time report generation (standard)                       | ≤ 10 detik untuk 1.000 instrumen                                     | Application logs             |
| NFR-PERF-04 | Job MTM Harian — durasi end-to-end                               | ≤ 30 menit untuk 1.500 instrumen                                     | Batch monitoring             |
| NFR-PERF-05 | Job ECL Akhir Bulan — durasi end-to-end                          | ≤ 4 jam untuk 1.500 instrumen, 3 skenario, 3 stage                   | Batch monitoring             |
| NFR-PERF-06 | Job Akrual Bunga Harian (EIR-based) — durasi                     | ≤ 60 menit untuk 1.500 instrumen                                     | Batch monitoring             |
| NFR-PERF-07 | Throughput input transaksi konkuren                              | ≥ 50 transaksi/detik dengan response time tetap memenuhi NFR-PERF-01 | Load testing                 |
| NFR-PERF-08 | Concurrent users (peak bulanan saat closing)                     | ≥ 100 users tanpa degradasi                                          | Load testing                 |
| NFR-PERF-09 | Database query (complex aggregation untuk reporting)             | ≤ 30 detik untuk dataset 5 tahun                                     | DB monitoring                |
| NFR-PERF-10 | File upload (single file 50 MB)                                  | ≤ 60 detik termasuk virus scan                                       | User acceptance              |

## 9.2 Availability & Disaster Recovery

| **NFR-ID**   | **Requirement**                                                | **Target**                                                                             |
| ------------ | -------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| NFR-AVAIL-01 | Uptime SLA selama jam operasional (07:00-19:00 WIB hari kerja) | ≥ 99,9% (≤ 5 menit downtime/bulan)                                                     |
| NFR-AVAIL-02 | Uptime SLA total (24×7)                                        | ≥ 99,5%                                                                                |
| NFR-AVAIL-03 | Maintenance window terjadwal                                   | Sabtu/Minggu malam, max 4 jam, dengan pre-notice ≥ 3 hari                              |
| NFR-AVAIL-04 | Recovery Time Objective (RTO)                                  | ≤ 4 jam untuk disaster site failover                                                   |
| NFR-AVAIL-05 | Recovery Point Objective (RPO)                                 | ≤ 15 menit (transaksi terakhir)                                                        |
| NFR-AVAIL-06 | Disaster Recovery topology                                     | Active-Passive minimal; Active-Active untuk DB direkomendasikan                        |
| NFR-AVAIL-07 | DR drill                                                       | Minimal 1× per tahun, dengan tabletop test setiap kuartal                              |
| NFR-AVAIL-08 | Backup retention                                               | Daily backup 30 hari; weekly 12 minggu; monthly 12 bulan; yearly 10 tahun (compliance) |
| NFR-AVAIL-09 | Backup integrity check                                         | Restore test minimal sekali per kuartal                                                |

## 9.3 Security Requirements

| **NFR-ID** | **Requirement**                   | **Target / Standard**                                                                                                         |
| ---------- | --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| NFR-SEC-01 | Authentication                    | SSO via SAML 2.0 atau OAuth 2.0; integrasi dengan Active Directory Tugure                                                     |
| NFR-SEC-02 | Multi-Factor Authentication (MFA) | Wajib untuk role Approver (Treasury Manager, Finance Controller, CFO, Komite)                                                 |
| NFR-SEC-03 | Authorization                     | Role-Based Access Control (RBAC) dengan minimal 6 role: Maker, Reviewer, Approver, Risk Officer, Akuntansi, Auditor, IT Admin |
| NFR-SEC-04 | Password policy                   | Minimum 12 karakter, kompleksitas tinggi, expiry 90 hari, history 12, lockout setelah 5 failed attempts                       |
| NFR-SEC-05 | Session management                | Idle timeout 15 menit; absolute timeout 8 jam; concurrent session limit 1                                                     |
| NFR-SEC-06 | Encryption at rest                | AES-256 untuk database & file storage; KMS-managed keys; rotated annually                                                     |
| NFR-SEC-07 | Encryption in transit             | TLS 1.2 minimum, prefer TLS 1.3; certificate pinning untuk mobile (jika ada)                                                  |
| NFR-SEC-08 | Sensitive data handling           | PII fields encrypted at column level; masking di UI untuk role tanpa hak; export logged                                       |
| NFR-SEC-09 | API security                      | OAuth 2.0 client credentials; rate limiting; request signing untuk integrasi GL host                                          |
| NFR-SEC-10 | Vulnerability management          | Quarterly penetration testing; monthly vulnerability scan; patch SLA: critical 7 hari, high 30 hari                           |
| NFR-SEC-11 | Audit log retention               | 10 tahun (sesuai retention dokumen); immutable storage (WORM atau equivalent)                                                 |
| NFR-SEC-12 | Antivirus / anti-malware          | Real-time scan untuk semua upload; signature update otomatis harian                                                           |
| NFR-SEC-13 | Database security                 | Database encryption transparent; DBA actions logged; separation of duty antara DBA dan application admin                      |
| NFR-SEC-14 | Network security                  | Web Application Firewall (WAF); DDoS protection; segregated VLAN antara web/app/DB tier                                       |
| NFR-SEC-15 | Compliance                        | Aligned dengan ISO 27001 controls; SOC 2 Type II readiness untuk vendor cloud                                                 |

## 9.4 Auditability & Compliance

| **NFR-ID** | **Requirement**                   | **Target / Standard**                                                                                                                   |
| ---------- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| NFR-AUD-01 | Audit trail granularity           | Per transaksi: Created/Modified/Approved By/At; Per dokumen: Uploader/Hash/Timestamp; Per parameter master: Maker/Approver/Before-After |
| NFR-AUD-02 | Audit trail immutability          | Audit log tidak dapat di-modify atau di-delete oleh user manapun, termasuk DBA/IT Admin                                                 |
| NFR-AUD-03 | Audit trail searchability         | Full-text search across audit log; filter by user/date/action/entity                                                                    |
| NFR-AUD-04 | Auditor access                    | Read-only role dengan akses penuh ke transactions, audit trail, media upload                                                            |
| NFR-AUD-05 | Document hash integrity           | SHA-256 per file; integrity verification on download                                                                                    |
| NFR-AUD-06 | Parameter version history         | Slowly-changing dimension untuk parameter master (PD, LGD, MEV, Impact PD, kurs)                                                        |
| NFR-AUD-07 | Re-perform calculation capability | Sistem HARUS dapat re-execute perhitungan ECL/EIR pada periode lampau (audit trail per parameter version)                               |
| NFR-AUD-08 | Regulatory reporting export       | API endpoint untuk regulator (LBU/LBBU spec dapat di-extend); read-only access                                                          |

## 9.5 Scalability Requirements

| **NFR-ID**   | **Requirement**         | **Target**                                                                                         |
| ------------ | ----------------------- | -------------------------------------------------------------------------------------------------- |
| NFR-SCALE-01 | Volume instrumen aktif  | Skala awal: 1.500; growth target 5x dalam 5 tahun (7.500 instrumen tanpa re-architecture)          |
| NFR-SCALE-02 | Volume transaksi harian | Skala awal: 200 transaksi/hari; growth 5x                                                          |
| NFR-SCALE-03 | Volume dokumen          | Skala awal: 50 GB; growth 100 GB/tahun                                                             |
| NFR-SCALE-04 | Database storage        | Skala awal: 500 GB; growth 200 GB/tahun                                                            |
| NFR-SCALE-05 | Horizontal scaling      | Web/App tier WAJIB stateless dan dapat di-scale horizontally; DB tier read-replica untuk reporting |
| NFR-SCALE-06 | Capacity planning       | Quarterly review utilisasi vs threshold; auto-alert di 70% threshold                               |

## 9.6 Usability Requirements

| **NFR-ID** | **Requirement**         | **Target**                                                                                                     |
| ---------- | ----------------------- | -------------------------------------------------------------------------------------------------------------- |
| NFR-USE-01 | Bahasa UI               | Bahasa Indonesia primary; bilingual (English) untuk reporting eksternal                                        |
| NFR-USE-02 | Browser support         | Chrome, Firefox, Edge (versi N-2); Safari basic support                                                        |
| NFR-USE-03 | Responsive design       | Desktop primary (1366×768 minimum); tablet view untuk dashboard read-only                                      |
| NFR-USE-04 | Accessibility           | WCAG 2.1 Level AA untuk fitur reporting (mempertimbangkan auditor disabilitas)                                 |
| NFR-USE-05 | Help & documentation    | Inline help per screen; user manual lengkap; quick reference card per role                                     |
| NFR-USE-06 | Training                | Role-based training modules: Maker (4 jam), Approver (2 jam), Risk (4 jam), Akuntansi (6 jam), Auditor (2 jam) |
| NFR-USE-07 | Error messaging         | Pesan error informatif dalam Bahasa Indonesia, dengan reference ke ID error untuk support                      |
| NFR-USE-08 | User satisfaction (UAT) | ≥ 4,0/5 dari survey pasca-UAT                                                                                  |

## 9.7 Maintainability & Operability

| **NFR-ID**  | **Requirement**          | **Target**                                                                                                       |
| ----------- | ------------------------ | ---------------------------------------------------------------------------------------------------------------- |
| NFR-MAIN-01 | Logging                  | Application logs structured (JSON); log retention 1 tahun online + 9 tahun cold storage                          |
| NFR-MAIN-02 | Monitoring               | APM tool (mis. New Relic, Datadog, Dynatrace); custom dashboard untuk batch jobs                                 |
| NFR-MAIN-03 | Alerting                 | Real-time alert untuk: batch job failure, response time degradation, security incidents, periode closing tenggat |
| NFR-MAIN-04 | Code quality             | Static analysis tool (SonarQube atau equivalent); minimum coverage unit test 70%                                 |
| NFR-MAIN-05 | Documentation            | Code documentation; API documentation (Swagger/OpenAPI); operational runbook lengkap                             |
| NFR-MAIN-06 | Configuration management | Infrastructure-as-Code (Terraform / equivalent); semua config di version control                                 |
| NFR-MAIN-07 | Deployment               | CI/CD pipeline; blue-green atau canary deployment untuk zero-downtime release                                    |
| NFR-MAIN-08 | Patching schedule        | Quarterly minor; semi-annual major (dengan UAT regression)                                                       |

# 10\. Compliance & Regulatory Requirements

## 10.1 Pemetaan Compliance ke PSAK 71

Setiap aspek PSAK 71 dipetakan ke modul BLIPS yang memenuhi requirement-nya:

| **PSAK 71 Paragraf**           | **Topik**                                      | **Modul BLIPS yang Memenuhi**                                   |
| ------------------------------ | ---------------------------------------------- | --------------------------------------------------------------- |
| §4.1 SPPI Test                 | Klasifikasi berbasis arus kas kontraktual      | Modul SPPI Test Engine (Bab 8.2)                                |
| §4.4 BM Test                   | Klasifikasi berbasis model bisnis              | Modul BM Test Engine (Bab 8.2)                                  |
| §5.4 Effective Interest Method | Pengakuan pendapatan bunga AC & FVOCI utang    | Modul EIR & Amortisasi (Bab 8.11)                               |
| §5.5 ECL Model                 | 3-Stage model dengan SICR                      | Modul ECL Engine (Bab 8.10)                                     |
| §5.5.17 Forward-Looking        | MEV adjustment untuk ECL                       | Impact MEV to PD + Impact PD layer (Bab 8.10)                   |
| §5.5.39 Lifetime PD            | PD horizon untuk Stage 2 & 3                   | Master PD Cumulative (Bab 8.1)                                  |
| §5.7.5 FVOCI Election          | Equity instrument irrevocable                  | Master Instrumen FVOCI Election Flag (Bab 8.1)                  |
| §3.3.2 Modifikasi Material     | Derecognition + recognition baru               | EIR Re-estimation + MODIFIKASI\_MATERIAL event (Bab 8.11, 8.14) |
| §B5.4.5-B5.4.6 Re-estimation   | Cash flow revision                             | EIR\_REESTIMATION event (Bab 8.11)                              |
| §35H Disclosure Carrying       | Roll-forward carrying amount                   | Roll-Forward Carrying Report (Bab 8.15)                         |
| §35H Disclosure CKPN           | Roll-forward CKPN per stage                    | Saldo CKPN Report (Bab 8.15)                                    |
| §5.7.10 OCI Recycling          | FVOCI utang recycling vs Election no-recycling | Modul Penjualan + REKLAS\_OCI\_PL event (Bab 8.6)               |

## 10.2 PSAK Lain & Standar Pendukung

| **Standar**           | **Aspek**                                   | **Modul BLIPS**                                         |
| --------------------- | ------------------------------------------- | ------------------------------------------------------- |
| PSAK 50/55            | Penyajian instrumen keuangan                | Reporting (Bab 8.15)                                    |
| PSAK 65               | Konsolidasi look-through reksadana          | ECL Look-through (Bab 8.10)                             |
| PSAK 25               | Prior-period adjustment, perubahan estimasi | Periode Buku Reopen + CORRECTION event (Bab 8.12, 8.14) |
| Basel III IRB         | LGD per tipe eksposur                       | Master LGD Basel (Bab 8.1)                              |
| Pefindo Default Study | PD per rating                               | Master PD (Bab 8.1)                                     |
| BI JISDOR             | Kurs tengah USD/IDR                         | Master Mata Uang & Kurs (Bab 8.13)                      |
| LPS Coverage          | Penjaminan Cash + Deposito                  | ECL LPS Aggregator (Bab 8.10)                           |

## 10.3 Audit & External Reporting

Sistem WAJIB siap untuk audit eksternal tahunan dan permintaan informasi regulator. Persiapan teknis:

  - Read-only role untuk Auditor Eksternal dengan akses penuh ke transactions + audit trail + media upload.

  - Snapshot capability — sistem dapat menampilkan posisi pada tanggal historis manapun (mis. 31/12/2026 untuk audit tahunan 2026).

  - Re-perform capability — auditor dapat trigger re-execute perhitungan ECL/EIR dengan parameter version pada tanggal audit.

  - Sample testing — auditor dapat select random sample transaksi dan trace seluruh audit trail (input → approval → posting → reporting).

  - Disclosure-ready reports — Roll-Forward CKPN, Stage Distribution, Carrying Roll-Forward, Concentration Risk, Sensitivity Analysis (memenuhi PSAK 71 §35-46).

  - Walkthrough support — sistem menyediakan demo mode untuk auditor walk-through tanpa risk impact data produksi.

## 10.4 Data Privacy & PII Handling

Meskipun BLIPS bukan sistem yang dominan PII (Personally Identifiable Information), tetap ada beberapa data sensitif yang perlu di-handle dengan benar:

  - NPWP / Registrasi counterparty: encrypted at rest; masked di UI untuk role tanpa hak.

  - Nomor rekening bank: encrypted at rest; full visibility hanya untuk Treasury & Akuntansi.

  - Data karyawan internal (user account, email): mengikuti policy HR / IT terkait PDP (Perlindungan Data Pribadi).

  - Pemenuhan UU No. 27 Tahun 2022 tentang PDP: hak akses, hak koreksi, hak penghapusan untuk subject data.

# 11\. Use Cases per Role

Bab ini menggambarkan use case utama per role pengguna. Setiap use case di-format dengan Actor, Trigger, Pre-condition, Main Flow, Post-condition, dan Exception. Detail screen flow dan API contract akan didokumentasikan di FSD.

## 11.1 Use Case: Penempatan Obligasi Baru (Treasury Maker)

| **Aspek**      | **Detail**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UC-ID          | UC-PNP-001                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Nama           | Penempatan Obligasi Baru                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Actor Utama    | Treasury Officer (Maker)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Actor Sekunder | Risk/Akuntansi (Reviewer), Komite Investasi (Approver)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Trigger        | Memo alokasi penempatan dari Komite Investasi diterima Treasury                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Pre-condition  | (1) Komite Investasi telah approve mandat investasi; (2) Term sheet & prospektus tersedia; (3) Periode buku Tanggal Transaksi dalam status OPEN.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Main Flow      | (1) Treasury input Master Instrumen baru. (2) Treasury upload term sheet & prospektus. (3) Treasury jalankan SPPI Test (10 questions). (4) Sistem auto-evaluate SPPI: PASS/FAIL. (5) Bila PASS → Treasury jalankan BM Test. (6) Sistem auto-suggest klasifikasi (PSAK 71 matriks). (7) Risk/Akuntansi review SPPI/BM/Klasifikasi. (8) Komite Investasi approve klasifikasi. (9) Treasury input Transaksi Penempatan. (10) Sistem hitung EIR via Newton-Raphson + generate Amortization Schedule (untuk AC/FVOCI utang). (11) Treasury upload bilyet/NoA. (12) Treasury Manager (Approver) review & approve. (13) Sistem post jurnal PENEMPATAN ke GL. (14) Notifikasi sukses ke Maker & Approver. |
| Post-condition | (1) Master Instrumen tercatat dengan klasifikasi PSAK 71 ter-lock; (2) Transaksi penempatan ter-posting di GL; (3) Amortization Schedule aktif (untuk AC/FVOCI utang); (4) Counterparty exposure ter-update; (5) Audit trail lengkap.                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Exception      | (a) SPPI FAIL → klasifikasi otomatis FVTPL → skip BM Test, langsung ke approval Komite. (b) BM Test override → wajib justifikasi tertulis. (c) Saldo rekening sumber tidak cukup → block transaksi. (d) Counterparty rating expired → flag untuk Risk update sebelum proceed. (e) EIR solver tidak konvergen → exception, escalate ke Akuntansi/Risk.                                                                                                                                                                                                                                                                                                                                             |

## 11.2 Use Case: Closing Bulanan (Akuntansi)

| **Aspek**      | **Detail**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UC-ID          | UC-PRD-001                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Nama           | Closing Periode Bulanan (Soft Close + Hard Close)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Actor Utama    | Finance Controller / Akuntansi                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| Actor Sekunder | Treasury (input cut-off), Risk (ECL parameter), CFO (Approver hard-close)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Trigger        | Akhir bulan kalender + H+1 hari kerja                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Pre-condition  | (1) Semua transaksi bulan berjalan ter-input & ter-approve; (2) Parameter PD/LGD/MEV/Impact PD versi terkini ter-upload.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Main Flow      | (1) H+1: Treasury complete entry transaksi bulan berjalan. (2) Job batch end-of-month: Akrual Bunga (EIR-based), MTM Final, Amortisasi P/D, ECL Akhir Bulan. (3) Akuntansi rekonsiliasi GL: cek saldo per akun, mismatch report. (4) H+5: Akuntansi (Maker) request soft-close. (5) Finance Controller (Approver) approve → status SOFT\_CLOSED. (6) Akuntansi proses adjustment journal entry yang diperlukan (PERIODE\_ADJUSTMENT). (7) Risk re-run ECL bila ada parameter update di SOFT\_CLOSED. (8) Triwulanan: laporan triwulanan ter-generate. (9) H+15 (max): Akuntansi (Maker) request hard-close. (10) CFO (Approver) approve → status CLOSED. (11) Sistem auto-lock semua kurs di periode tersebut. |
| Post-condition | (1) Periode bulanan CLOSED dan immutable; (2) Semua jurnal balanced; (3) Laporan periode tersedia; (4) Closing Audit Trail Report ter-generate; (5) Notifikasi ke seluruh stakeholder.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Exception      | (a) Mismatch GL \> toleransi → block hard-close, eskalasi ke Akuntansi. (b) Tenggat hard-close H+15 terlewat → notifikasi ke CFO + Komite Audit. (c) Reopen request → workflow CFO + CEO/Komite Audit.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

## 11.3 Use Case: Migrasi Stage SICR/Default (Risk Officer)

| **Aspek**      | **Detail**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| UC-ID          | UC-ECL-001                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Nama           | Migrasi Stage Akibat Rating Downgrade / DPD                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Actor Utama    | Risk Officer                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Actor Sekunder | Akuntansi, Komite Risiko (untuk curing)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Trigger        | (a) Risk Officer input rating baru di Counterparty Rating History; atau (b) Job batch akhir bulan deteksi DPD                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Pre-condition  | Counterparty Rating History aktif; SICR threshold ter-konfigurasi                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Main Flow      | (1) Risk Officer input rating baru ke Rating History dengan dokumen Pefindo Press Release. (2) Sistem auto-calc Notch Change vs origination. (3) Bila ≥ 2 notch downgrade atau berpindah ke non-investment grade → SICR Triggered = TRUE. (4) Bila rating = idD → Default Triggered = TRUE. (5) Sistem otomatis create Stage History record (Stage Sebelum → Stage Sesudah) dengan Trigger Type. (6) Sistem otomatis re-evaluate ECL dengan PD horizon baru (Lifetime untuk Stage 2/3). (7) Δ ECL ter-posting via STAGE\_MIGRATION event. (8) Untuk Stage 3: pendapatan bunga selanjutnya berbasis Net Carrying. (9) Notifikasi ke Risk Manager + Akuntansi + CFO. |
| Post-condition | (1) Stage instrumen ter-update; (2) ECL tercatat ulang sesuai stage baru; (3) Audit trail Stage History; (4) Beban CKPN incremental ter-posting.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Exception      | (a) Curing (mundur Stage 3→2 atau 2→1): wajib probationary period 3-6 bulan + approval Komite Risiko. (b) Manual override Stage migration: hanya CFO dengan justifikasi memo.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |

## 11.4 Use Case: Reklasifikasi Prospektif (Komite Investasi)

| **Aspek**      | **Detail**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UC-ID          | UC-RKL-001                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Nama           | Reklasifikasi Klasifikasi PSAK 71 Prospektif                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Actor Utama    | Komite Investasi (Decision-maker)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Actor Sekunder | Treasury, Risk, Akuntansi                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Trigger        | Periodic Review tahunan SPPI/BM, Triggered Reassessment, atau perubahan model bisnis                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Pre-condition  | BM Test atau SPPI Test menunjukkan kategori berbeda dari yang ter-lock di Master Instrumen                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Main Flow      | (1) Risk Officer atau Treasury identifikasi trigger reklasifikasi. (2) Risk/Treasury input case Reklasifikasi di sistem dengan justifikasi & dokumen. (3) Komite Investasi review case. (4) Komite Investasi approve dengan tanggal efektif reklasifikasi. (5) Sistem auto-determine kombinasi from-to (mis. AC → FVOCI). (6) Sistem generate jurnal transisi otomatis sesuai matriks 4.5 SoW. (7) Untuk EIR-affected reklasifikasi: sistem recompute EIR sesuai matriks §5.12.10 SoW. (8) Master Instrumen di-update dengan klasifikasi baru + tanggal efektif. (9) Akuntansi review & approve final journal. |
| Post-condition | (1) Klasifikasi instrumen ter-update prospektif; (2) Jurnal transisi ter-posting; (3) Amortization Schedule ter-recompute (jika berubah ke/dari AC); (4) OCI accumulated ter-handle sesuai matriks reklasifikasi.                                                                                                                                                                                                                                                                                                                                                                                              |
| Exception      | (a) Rejected by Komite → Master tetap; rationale tercatat. (b) Reklasifikasi dengan dampak P\&L material → tambahan approval CFO.                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

## 11.5 Use Case: Audit Trail Review (Auditor Eksternal)

| **Aspek**      | **Detail**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UC-ID          | UC-AUD-001                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Nama           | Substantive Testing oleh Auditor Eksternal                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Actor Utama    | Auditor Eksternal (Read-Only)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Actor Sekunder | Akuntansi (sebagai liaison)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Trigger        | Audit period tahunan atau interim review                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Pre-condition  | (1) Auditor diberikan Read-Only role aktif untuk periode audit; (2) Periode bulanan/tahunan yang diaudit sudah CLOSED                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Main Flow      | (1) Auditor login via SSO. (2) Auditor browse Daftar Posisi Portofolio per tanggal audit. (3) Auditor select sample transaksi (random atau stratified). (4) Auditor drill-down ke detail transaksi: input → SPPI/BM Test result → klasifikasi → jurnal posting. (5) Auditor download dokumen pendukung dengan SHA-256 hash verification. (6) Auditor re-perform perhitungan EIR/ECL untuk sample (sistem trigger calculation dengan parameter version pada tanggal). (7) Auditor cross-check ke Roll-Forward CKPN, Stage Distribution, Concentration Risk. (8) Auditor export findings ke Excel untuk working paper. |
| Post-condition | (1) Audit trail intact (tidak ada modifikasi); (2) Auditor working paper lengkap; (3) Access log ter-update.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Exception      | (a) Document hash mismatch → flagged sebagai potential tampering, eskalasi ke Internal Audit. (b) Re-perform calculation diskrepansi \> toleransi → root cause analysis bersama Risk/Akuntansi.                                                                                                                                                                                                                                                                                                                                                                                                                      |

# 12\. Assumptions, Constraints, Dependencies

## 12.1 Assumptions

Asumsi-asumsi yang menjadi basis perencanaan proyek. Bila asumsi berubah, scope/timeline/budget perlu re-evaluasi via Change Request.

| **\#** | **Asumsi**                                                                                                          | **Risiko jika Tidak Terpenuhi**                                                  |
| ------ | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| A-01   | Sistem GL host menyediakan API REST atau file batch interface yang stabil dan ter-dokumentasi.                      | Delay implementasi modul Jurnal & GL Interface; perlu adapter custom.            |
| A-02   | Pefindo Default Study tersedia secara berkala (triwulanan) dengan data PD per rating dan cumulative PD multi-tenor. | Sistem ECL memerlukan fallback ke internal model atau S\&P/Moody's data.         |
| A-03   | BI JISDOR & Kurs Tengah BI dapat diakses via API resmi atau scheduled scraping yang legal.                          | Manual upload kurs harian; risiko delay / human error.                           |
| A-04   | IBPA harga obligasi tersedia secara harian via file feed atau API.                                                  | MTM obligasi tertunda atau memakai harga manual; risiko misvaluation.            |
| A-05   | NAB reksadana harian tersedia dari MI atau KSEI dalam format yang konsisten.                                        | Manual upload NAB; potensi inkonsistensi format antar MI.                        |
| A-06   | Komite Investasi melakukan rapat bulanan dan dapat memberikan approval klasifikasi PSAK 71 dalam waktu wajar.       | Bottleneck approval; transaksi penempatan tertunda.                              |
| A-07   | ALCO/Komite Risiko menyetujui Impact MEV to PD dan Impact PD multiplier per kuartal/periode.                        | ECL forward-looking tidak ter-update; non-compliance PSAK 71 §5.5.17.            |
| A-08   | LPS coverage tetap Rp 2 Miliar per nasabah per bank selama umur sistem.                                             | Bila berubah, parameter LPS perlu di-update; perhitungan retroaktif untuk audit. |
| A-09   | Tarif PPh Final tidak berubah signifikan (PPh Bunga Deposito 20%, Kupon Obligasi 10%, Dividen 10%/Final).           | Perlu update Master Mapping Jurnal multiplier; jurnal historis tetap valid.      |
| A-10   | Investment Policy & Treasury Policy Tugure tidak mengalami perubahan material selama implementasi.                  | Re-validasi mapping BM Test; re-training user.                                   |
| A-11   | Vendor implementor memiliki kapasitas dan keahlian tim yang sesuai sepanjang 33 minggu.                             | Project delay; quality issue; re-tender.                                         |
| A-12   | Internal user (\~50-100) dapat dialokasikan untuk training & UAT minimal 30% effort selama Discovery + UAT phase.   | UAT tidak comprehensive; risiko go-live tertunda atau quality drop.              |
| A-13   | Infrastruktur Tugure (network, security perimeter, identity provider) memenuhi persyaratan teknis BLIPS.            | Tambahan investasi infrastruktur; potensi delay deployment.                      |
| A-14   | Standar PSAK 71 tidak mengalami amendement material selama 33 minggu implementasi.                                  | Re-design modul terkait; potensi re-test.                                        |

## 12.2 Constraints

| **\#** | **Constraint**                                                                                                   | **Implikasi**                                                                                             |
| ------ | ---------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| C-01   | Budget total proyek terbatas sesuai approval Steering Committee (detail di Investment Decision Memo).            | Scope freeze setelah BRD sign-off; CR untuk additional scope memerlukan budget approval.                  |
| C-02   | Timeline 33 minggu (≈ 8 bulan) adalah hard target untuk go-live sebelum closing tahunan 2026.                    | Crash schedule jika ada slippage; potensi parallel processing antar fase.                                 |
| C-03   | Bahasa primary BRD/FSD/UI/Manual: Bahasa Indonesia. English untuk reporting eksternal.                           | Translation effort untuk tools yang default English.                                                      |
| C-04   | Compliance dengan PSAK 71 tidak negotiable. Setiap requirement compliance bersifat MUST.                         | Compliance feature tidak dapat di-de-scope, hanya ditunda fase berikutnya bila bukan compliance-blocking. |
| C-05   | Data residency: data produksi WAJIB di on-premise atau cloud Indonesia (hindari penalty PDP transboundary).      | Pilihan vendor cloud terbatas (mis. tidak boleh AWS Singapore tanpa BCP).                                 |
| C-06   | Integrasi dengan GL host Tugure: API contract harus ter-validasi dengan tim IT internal Tugure.                  | Tergantung kapasitas tim IT internal untuk co-development API.                                            |
| C-07   | Vendor procurement mengikuti policy Tugure (vendor onboarding, NDAs, vendor risk assessment).                    | Lead time procurement tambahan \~4-6 minggu sebelum vendor mulai bekerja.                                 |
| C-08   | Audit external Tugure dilaksanakan pada Q1 2027; sistem WAJIB siap untuk substantive testing pada Februari 2027. | Production go-live target Desember 2026.                                                                  |

## 12.3 Dependencies

| **\#** | **Dependency**                                                          | **Owner**                    | **Critical Path?**                 |
| ------ | ----------------------------------------------------------------------- | ---------------------------- | ---------------------------------- |
| D-01   | Approval BRD oleh Steering Committee                                    | CFO + BoD                    | YES (precondition Phase 2)         |
| D-02   | Vendor selection complete & contract signed                             | PMO + IT + Procurement       | YES (precondition Phase 1)         |
| D-03   | API contract GL host finalized & test sandbox available                 | IT Internal + GL vendor      | YES (precondition Phase 5)         |
| D-04   | Master CoA Tugure di-export dan ter-review                              | Akuntansi                    | YES (precondition Phase 3)         |
| D-05   | Investment Policy & Treasury Policy v3.2 approved oleh BoD              | Komite Investasi             | Optional (gunakan v3.1 jika delay) |
| D-06   | Pefindo Default Study Q4 2025 ter-publish & ter-format konsisten        | External — Pefindo           | YES (precondition Phase 6)         |
| D-07   | Infrastruktur produksi (compute, storage, network, DR site) provisioned | IT Infrastructure            | YES (precondition Phase 10)        |
| D-08   | User training plan & schedule approved                                  | HR + PMO                     | Optional (parallel ke Phase 9)     |
| D-09   | External auditor briefing tentang sistem baru                           | Akuntansi + External Auditor | Optional (post Phase 11)           |
| D-10   | OJK / regulator briefing (jika diperlukan disclosure)                   | Compliance                   | Optional                           |

# 13\. Risk Assessment & Mitigation

Risk register proyek BLIPS IFRS 9 dengan kategori, probability (P), impact (I), risk score (P × I), mitigation strategy, dan owner. Skala 1-5 (1=Very Low, 5=Very High).

## 13.1 Risk Register

| **Risk ID** | **Kategori**      | **Risk Description**                                                                                         | **P** | **I** | **Score** | **Mitigation**                                                                                                                                  | **Owner**              |
| ----------- | ----------------- | ------------------------------------------------------------------------------------------------------------ | ----- | ----- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| R-01        | Compliance        | Misinterpretasi PSAK 71 dalam implementasi (mis. EIR, ECL formula, staging logic) menghasilkan misstatement. | 3     | 5     | 15        | Engagement konsultan PSAK 71 specialist sejak Phase 1; review BRD/FSD oleh DSAK-certified accountant; UAT dengan auditor eksternal observation. | CFO + Akuntansi        |
| R-02        | Technical         | API GL host tidak stabil atau performance buruk, menghambat posting jurnal otomatis.                         | 3     | 4     | 12        | Early integration testing di Phase 5; SLA contract dengan GL vendor; queue + retry mechanism + outbound buffer.                                 | IT Architect           |
| R-03        | Data              | Pefindo Default Study format berubah atau publikasi tertunda.                                                | 2     | 3     | 6         | Master upload manual sebagai fallback; engagement langsung dengan Pefindo Account Manager; alternative source S\&P/Moody's untuk benchmark.     | Risk + Procurement     |
| R-04        | Resource          | Key business user (Treasury/Risk/Akuntansi) tidak tersedia untuk UAT sesuai schedule.                        | 4     | 4     | 16        | Lock 30% effort di Phase 9 (UAT) sejak BRD approval; backup users diidentifikasi; UAT scenarios di-prioritaskan.                                | Direktur masing-masing |
| R-05        | Vendor            | Vendor implementor underdeliver — quality, timeline, atau scope.                                             | 3     | 5     | 15        | Vendor due diligence rigorous; reference check; milestone-based payment; SoW & KPI vendor di kontrak; quarterly Steering review.                | PMO + Procurement      |
| R-06        | Compliance        | Standar PSAK 71 mengalami amendement material selama implementasi.                                           | 1     | 4     | 4         | DSAK monitoring oleh Akuntansi; impact assessment cycle quarterly; CR process untuk perubahan scope.                                            | Akuntansi              |
| R-07        | Security          | Data breach atau unauthorized access ke data finansial sensitif.                                             | 2     | 5     | 10        | Pen-test sebelum go-live; ISO 27001 alignment; encryption at rest & in transit; MFA untuk Approver; SIEM monitoring; incident response plan.    | IT Security            |
| R-08        | Operational       | Migration data dari sistem legacy ke BLIPS error atau incomplete.                                            | 3     | 4     | 12        | Phased migration dengan UAT validation per fase; reconciliation tools; parallel run minimum 1 bulan; rollback plan.                             | PMO + IT + Akuntansi   |
| R-09        | Change Management | User adoption rendah; user kembali ke spreadsheet manual.                                                    | 3     | 3     | 9         | Change champions per direktorat; quick-win UI/UX; ongoing training; KPI per role; sponsorship CFO eksplisit.                                    | HR + PMO + CFO         |
| R-10        | Financial         | Budget overrun \> 15% akibat scope creep atau emergent requirement.                                          | 3     | 4     | 12        | BRD freeze setelah approval; CR process formal; budget contingency 10%; quarterly Steering budget review.                                       | PMO + CFO              |
| R-11        | Schedule          | Delay produksi go-live karena UAT defect tinggi.                                                             | 3     | 4     | 12        | Defect quality gates per fase; SIT lebih ekstensif; UAT environment stable; bug bash sebelum UAT.                                               | PMO + IT               |
| R-12        | Audit             | External auditor menemukan material weakness pasca go-live.                                                  | 2     | 5     | 10        | Auditor observation di UAT; audit trail review pre-go-live; tabletop substantive testing simulasi; CFO review checklist.                        | Akuntansi + Audit      |
| R-13        | Regulatory        | OJK/BI menerbitkan circular yang merubah requirement reporting selama implementasi.                          | 2     | 3     | 6         | Compliance horizon scanning; quarterly regulatory update review; flexible mapping engine untuk akomodasi format baru.                           | Compliance             |
| R-14        | Data Quality      | Data master existing (instrumen, counterparty, rating) tidak clean → impact ke perhitungan ECL/EIR.          | 4     | 4     | 16        | Data cleansing & validation di Phase 1-2 dengan business user signoff; data quality dashboard; correction workflow ter-track.                   | Risk + Akuntansi       |
| R-15        | Performance       | Job ECL akhir bulan tidak selesai dalam window closing → delay closing.                                      | 2     | 4     | 8         | Performance testing dengan dataset 5x volume produksi; query optimization; partitioning strategy; tuning per fase Development.                  | IT + DBA               |

## 13.2 Risk Heat Map

Distribusi risiko berdasarkan score:

| **Score Range** | **Severity** | **Risk IDs**                                                     | **Action**                                                   |
| --------------- | ------------ | ---------------------------------------------------------------- | ------------------------------------------------------------ |
| ≥ 15            | CRITICAL     | R-01 (15), R-04 (16), R-05 (15), R-14 (16)                       | Mitigation aktif sejak Phase 1; weekly tracking di Steering. |
| 10-14           | HIGH         | R-02 (12), R-07 (10), R-08 (12), R-10 (12), R-11 (12), R-12 (10) | Mitigation plan ter-dokumentasi; bi-weekly tracking.         |
| 5-9             | MEDIUM       | R-03 (6), R-09 (9), R-13 (6), R-15 (8)                           | Monitor monthly; escalate if probability/impact berubah.     |
| \< 5            | LOW          | R-06 (4)                                                         | Quarterly review.                                            |

## 13.3 Risk Governance

  - Risk Owner ditugaskan per risk; status update setiap 2 minggu di Working Group meeting.

  - Critical risks (score ≥ 15) di-escalate ke Steering Committee setiap bulan.

  - Risk register di-maintain di project SharePoint dengan version control.

  - Lesson learned register dibuat untuk risk yang materialized → input untuk continuous improvement.

  - Risk closure: setelah mitigation efektif terbukti, risk di-mark CLOSED dengan justifikasi.

# 14\. Acceptance Criteria & Sign-Off Matrix

## 14.1 Functional Acceptance Criteria

Sistem dianggap fungsional acceptable untuk go-live bila SELURUH kriteria berikut TERPENUHI:

| **\#**  | **Acceptance Criteria**                                                                                                                 | **Verification Method**                |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| AC-F-01 | Semua field master data dapat di-CRUD dengan validasi schema, audit trail, dan workflow approval.                                       | UAT script per master entity           |
| AC-F-02 | SPPI Test menjalankan checklist Q1-Q10 dengan auto-derive PASS/FAIL; instrumen FAIL otomatis ter-lock FVTPL.                            | UAT 10+ scenario test                  |
| AC-F-03 | BM Test menampilkan riwayat penjualan 12-bulan dan auto-suggest HTC/HTC\&S/Other; override hanya dengan justifikasi tertulis.           | UAT scenario per BM                    |
| AC-F-04 | Matriks klasifikasi 6-cell SPPI × BM ter-implementasi & klasifikasi ter-lock di Master Instrumen sebelum penempatan.                    | UAT scenario 6 kombinasi               |
| AC-F-05 | Reklasifikasi prospektif menghasilkan jurnal transisi otomatis untuk 6 kombinasi (AC↔FVOCI↔FVTPL).                                      | UAT 6 scenario reklas                  |
| AC-F-06 | EIR ter-hitung otomatis via Newton-Raphson untuk seluruh instrumen AC/FVOCI utang dengan presisi 8 desimal internal, 4 desimal display. | Test case dengan known EIR + recompute |
| AC-F-07 | Amortization Schedule ter-generate dengan Closing Carrying baris terakhir = nilai par (toleransi ±0,01 IDR).                            | Test case end-to-end obligasi          |
| AC-F-08 | Akrual bunga harian berbasis EIR (Carrying × EIR ÷ 365) untuk AC/FVOCI utang; jurnal balanced.                                          | Daily job validation 30 hari           |
| AC-F-09 | Setiap event instrumen memiliki minimal 1 dokumen upload yang ter-link dengan SHA-256 hash.                                             | UAT random sample 50 transaksi         |
| AC-F-10 | Hasil ECL FL match (manual rekalkulasi) dengan toleransi pembulatan 4 desimal pada rasio.                                               | Re-perform 100 instrumen               |
| AC-F-11 | Logika LPS aggregator menggabungkan Cash + Deposito per Bank dan menghitung EAD tak terjamin secara benar.                              | UAT 5 scenario bank                    |
| AC-F-12 | Look-through Reksadana memecah underlying minimal 3 kategori (sovereign, korporasi, cash) dan menghitung ECL per underlying.            | UAT 3 scenario reksadana               |
| AC-F-13 | Jurnal otomatis tervalidasi balance (Σ Debit = Σ Kredit) untuk setiap event posting.                                                    | Automated balance check report         |
| AC-F-14 | Audit trail menampilkan jejak lengkap dari maker, approver, hingga modifikasi parameter (immutable).                                    | Audit trail review 100 transaksi       |
| AC-F-15 | Reporting menampilkan ECL Weighted dan ECL FL dengan presisi 4 desimal pada rasio dan 2 desimal pada IDR.                               | Visual review reports                  |
| AC-F-16 | Periode buku 3-status ter-enforce: tidak ada transaksi backdated tanpa adjustment journal entry & approval Akuntansi.                   | Test case 5 scenario backdated         |
| AC-F-17 | Multi-currency: semua perhitungan ECL/EAD/MTM dalam IDR equivalent dengan kurs Tengah BI per event.                                     | Test case obligasi USD                 |
| AC-F-18 | Stage migration otomatis dari Counterparty Rating History trigger; ECL re-calc dengan PD horizon yang sesuai.                           | Test case 10 scenario migrasi          |
| AC-F-19 | Workflow Maker-Reviewer-Approver enforced; tidak boleh same user di multiple roles untuk transaksi yang sama.                           | Test case role conflict                |
| AC-F-20 | Re-perform calculation untuk audit: sistem dapat re-execute ECL/EIR pada periode lampau dengan parameter version.                       | Test case audit periode lampau         |

## 14.2 Non-Functional Acceptance Criteria

| **\#**   | **Acceptance Criteria**                                                                  | **Verification Method**       |
| -------- | ---------------------------------------------------------------------------------------- | ----------------------------- |
| AC-NF-01 | Performance: response time UI ≤ 2 detik (P95) dengan 100 concurrent users.               | Load test pre-go-live         |
| AC-NF-02 | Job MTM Harian selesai ≤ 30 menit untuk 1.500 instrumen; ECL akhir bulan ≤ 4 jam.        | Batch performance test        |
| AC-NF-03 | Uptime SLA ≥ 99,9% selama jam operasional; demonstrasi 30 hari pre-go-live.              | Uptime monitoring report      |
| AC-NF-04 | RTO ≤ 4 jam, RPO ≤ 15 menit; demonstrated via DR drill.                                  | DR drill report               |
| AC-NF-05 | Security: pen-test report dengan 0 critical, 0 high vulnerability open.                  | Pen-test report               |
| AC-NF-06 | Encryption: data at rest AES-256 verified; in transit TLS 1.2+ verified.                 | Security audit                |
| AC-NF-07 | MFA enforced untuk role Approver; SSO integrated dengan Tugure AD.                       | Login flow verification       |
| AC-NF-08 | Audit trail immutable: tidak dapat di-modify oleh DBA atau IT Admin (verified via test). | Penetration test on audit log |
| AC-NF-09 | Backup integrity: monthly restore test sukses 3 bulan berturut-turut.                    | Restore test report           |
| AC-NF-10 | User acceptance score ≥ 4,0/5 dari survey pasca-UAT.                                     | UAT survey result             |

## 14.3 Compliance Acceptance Criteria

| **\#**  | **Acceptance Criteria**                                                                                                                | **Verification**                                                        |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| AC-C-01 | Sistem patuh PSAK 71 untuk klasifikasi (SPPI + BM Test), pengukuran (AC/FVOCI/FVTPL), impairment (3-Stage ECL), pengakuan bunga (EIR). | Sign-off oleh DSAK-certified accountant + auditor eksternal observation |
| AC-C-02 | Sistem patuh PSAK 25 untuk prior-period adjustment.                                                                                    | UAT scenario reopen periode CLOSED                                      |
| AC-C-03 | Sistem patuh PSAK 65 untuk look-through reksadana.                                                                                     | UAT scenario reksadana FVOCI                                            |
| AC-C-04 | LGD per Basel III IRB Foundation ter-implementasi (Sovereign 0,4500; Senior Unsecured 0,4500; Subordinated 0,7500).                    | Master LGD review                                                       |
| AC-C-05 | PD Pefindo Default Study (1-Yr & Lifetime) ter-implementasi & dapat di-update via upload.                                              | Upload test                                                             |
| AC-C-06 | Konversi mata uang menggunakan kurs Tengah BI / JISDOR.                                                                                | Master FX rate verification                                             |
| AC-C-07 | Audit trail memenuhi requirement audit eksternal (substantive testing capability).                                                     | Auditor walkthrough sign-off                                            |
| AC-C-08 | Disclosure-ready reports untuk PSAK 71 §35-46 (Roll-forward CKPN, Stage Distribution, Concentration Risk, Sensitivity).                | Report review oleh CFO + auditor                                        |

## 14.4 Sign-Off Matrix

Tabel berikut menetapkan siapa yang men-signoff acceptance criteria. Sign-off via formal memorandum atau platform e-signature, dengan tanggal dan komentar (jika ada) tercatat.

| **Acceptance Area**                   | **Sign-off Authority**                        | **Backup Authority**                  | **Tanggal Target**                      |
| ------------------------------------- | --------------------------------------------- | ------------------------------------- | --------------------------------------- |
| Functional (AC-F-01 sd AC-F-20)       | Direktur Investasi & Treasury + Direktur Risk | Kepala Treasury + Kepala Risk         | End of Phase 9 (UAT)                    |
| Non-Functional (AC-NF-01 sd AC-NF-10) | Direktur Teknologi Informasi                  | IT Architect                          | End of Phase 9                          |
| Compliance (AC-C-01 sd AC-C-08)       | CFO + Direktur Compliance                     | Kepala Akuntansi + Compliance Officer | End of Phase 9                          |
| Security & Audit Trail                | Direktur IT + Internal Audit                  | IT Security + Auditor Internal        | End of Phase 9                          |
| BRD itself (sign-off go ahead)        | CFO (Sponsor)                                 | Direktur Utama (Steering Chair)       | End of Phase 1 (Discovery)              |
| FSD per modul                         | Working Group + Vendor                        | PMO                                   | Per modul, sebelum Phase 3-7            |
| Production Go-Live                    | Steering Committee (CEO + CFO + Direksi)      | —                                     | End of Phase 10                         |
| Hypercare Closure                     | PMO + Steering                                | —                                     | End of Phase 11 (4 minggu post go-live) |

# 15\. Implementation Approach & Milestone

Implementation approach mengikuti SoW v1.1 §11.2 dengan total durasi 33 minggu. Pendekatan: phased delivery dengan quality gates antar fase, vendor-led implementation dengan internal Working Group sebagai Business Owner.

## 15.1 Implementation Methodology

  - Hybrid waterfall + agile: Phase 1-2 (Discovery + Desain) waterfall untuk setting baseline; Phase 3-7 (Development) sprint-based dengan demo bi-weekly ke Working Group.

  - Quality gates antar fase: each phase has acceptance criteria yang HARUS terpenuhi sebelum lanjut ke fase berikutnya.

  - Vendor management: weekly status meeting + monthly Steering review; milestone-based payment schedule.

  - Knowledge transfer: vendor wajib memberikan dokumentasi lengkap + training internal IT untuk operasional pasca go-live.

## 15.2 Milestone Schedule

| **Phase** | **Aktivitas**                    | **Durasi** | **Output Utama**                                                                  | **Sign-off**                                  |
| --------- | -------------------------------- | ---------- | --------------------------------------------------------------------------------- | --------------------------------------------- |
| 1         | Discovery & Requirement          | 3 minggu   | BRD final approved, kick-off complete                                             | CFO + Steering                                |
| 2         | Desain Sistem                    | 3 minggu   | FSD per modul, ERD, mock-up UI/UX                                                 | Working Group + IT Arch                       |
| 3         | Development phase 1              | 4 minggu   | Master Data + Master Portofolio + SPPI/BM Test Engine + Modul Penempatan + Upload | Working Group demo                            |
| 4         | Development phase 2              | 4 minggu   | Modul MTM + Pendapatan + Renewal                                                  | Working Group demo                            |
| 5         | Development phase 3              | 4 minggu   | Modul Jual + Jatuh Tempo + Jurnal & GL Interface                                  | Working Group demo + IT integration test      |
| 6         | Development phase 4              | 4 minggu   | EIR Engine + ECL Engine (CSH, DEP, OBL, RDN look-through)                         | Risk + Akuntansi demo + DSAK-certified review |
| 7         | Development phase 5              | 4 minggu   | Reporting + Dashboard + Periode Buku + FX + Mapping Jurnal                        | Working Group demo                            |
| 8         | SIT — System Integration Testing | 3 minggu   | SIT report; integrasi end-to-end ter-validasi                                     | IT QA + Vendor                                |
| 9         | UAT — User Acceptance Testing    | 3 minggu   | UAT sign-off; defect closure; functional/NFR/compliance acceptance                | Working Group + CFO                           |
| 10        | Production Deployment            | 1 minggu   | Go-live; data migration; cutover; smoke test                                      | Steering                                      |
| 11        | Hypercare                        | 4 minggu   | Stabilization, fine-tuning, knowledge transfer; transisi ke BAU                   | PMO + IT Operations                           |

*Total durasi: ± 33 minggu (≈ 8 bulan).*

## 15.3 Phase Gate Criteria

Setiap fase memiliki gate criteria yang HARUS dipenuhi sebelum lanjut ke fase berikutnya:

| **Fase**               | **Gate Criteria**                                                                                                                                        |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Phase 1 → 2            | BRD final approved oleh Steering; vendor selection complete; team kick-off complete; resourcing matrix confirmed.                                        |
| Phase 2 → 3            | FSD per modul approved; ERD reviewed by IT Arch; mock-up UI approved by Working Group; technical infrastructure planning complete.                       |
| Phase 3-7 → next       | Demo dengan 0 critical defect; Working Group sign-off; documentation lengkap; unit test coverage ≥ 70%.                                                  |
| Phase 7 → 8 (SIT)      | Semua modul integrated; integration smoke test pass; test data prepared.                                                                                 |
| Phase 8 → 9 (UAT)      | SIT report dengan 0 critical, ≤ 5 high open; environment UAT stable; UAT scripts ready; users trained.                                                   |
| Phase 9 → 10 (Go-Live) | UAT sign-off oleh CFO; all functional/NFR/compliance acceptance criteria met; rollback plan ready; data migration tested; CEO/Steering go-live approval. |
| Phase 10 → 11          | Production smoke test pass; first day end-of-day batch successful; users onboarded.                                                                      |
| Phase 11 → BAU         | Hypercare complete; KPI baseline established; operational handover to IT Ops + business unit.                                                            |

# 16\. Traceability Matrix BR ↔ SoW

Traceability Matrix memetakan setiap Business Requirement ke section relevan di SoW v1.1, FSD reference (akan terisi setelah FSD finalized), dan Test Case ID. Matrix ini menjadi tools untuk memastikan setiap requirement ter-implementasi dan ter-tested.

Berikut sample traceability matrix; full matrix akan di-maintain di sistem PMO (mis. Jira, Azure DevOps, atau spreadsheet master) sepanjang siklus implementasi:

| **BR-ID**  | **Requirement (Ringkas)**                            | **SoW v1.1 Ref** | **FSD Module** | **Test Case ID**  |
| ---------- | ---------------------------------------------------- | ---------------- | -------------- | ----------------- |
| BR-MAS-001 | CRUD Master Instrumen                                | §5.1.1           | FSD-MAS-01     | TC-MAS-001 to 050 |
| BR-MAS-005 | Master Mapping PD Normal Pefindo                     | §5.1.3, 5.1.3.a  | FSD-MAS-03     | TC-MAS-150 to 170 |
| BR-MAS-008 | Master Periode Buku 3-status                         | §5.1.7, 5.9      | FSD-PRD-01     | TC-PRD-001 to 030 |
| BR-MAS-009 | Master Mata Uang & Kurs BI                           | §5.1.8, 5.10     | FSD-FX-01      | TC-FX-001 to 050  |
| BR-MAS-011 | Master Mapping Jurnal Header-Detail                  | §5.1.10, 5.11    | FSD-JNL-01     | TC-JNL-001 to 080 |
| BR-SPP-001 | SPPI Test 10-question engine                         | §4.1.2, 4.1.4    | FSD-SPP-01     | TC-SPP-001 to 050 |
| BR-SPP-005 | Pre-Trade Clearance flow SPPI→BM→Approval            | §4.4.1           | FSD-SPP-03     | TC-SPP-100 to 120 |
| BR-SPP-009 | Reklasifikasi prospektif 6 kombinasi                 | §4.5             | FSD-RKL-01     | TC-RKL-001 to 030 |
| BR-PNP-008 | EIR computed saat penempatan                         | §5.12.4          | FSD-EIR-01     | TC-EIR-001 to 020 |
| BR-MTM-001 | MTM harian per tipe & klasifikasi                    | §5.3.1           | FSD-MTM-01     | TC-MTM-001 to 050 |
| BR-RNW-001 | Renewal deposito dual scheme                         | §5.4             | FSD-RNW-01     | TC-RNW-001 to 030 |
| BR-ECL-002 | ECL 3-skenario dengan dual FL                        | §7.1, 5.8.3      | FSD-ECL-01     | TC-ECL-001 to 100 |
| BR-ECL-006 | Staging otomatis 3-Stage                             | §8.5             | FSD-ECL-03     | TC-ECL-150 to 200 |
| BR-ECL-010 | LPS Aggregator Cash+Deposito                         | §8.1.2           | FSD-ECL-05     | TC-ECL-250 to 270 |
| BR-ECL-014 | Look-through Reksadana Campuran (excl equity)        | §8.3.3           | FSD-ECL-07     | TC-ECL-300 to 320 |
| BR-EIR-001 | Newton-Raphson IRR solver EIR                        | §5.12.4          | FSD-EIR-02     | TC-EIR-050 to 100 |
| BR-EIR-005 | Amortization Schedule generation                     | §5.12.6          | FSD-EIR-03     | TC-EIR-150 to 200 |
| BR-EIR-007 | Akrual bunga harian EIR-based                        | §5.12.7          | FSD-EIR-04     | TC-EIR-250 to 280 |
| BR-EIR-011 | Re-estimation EIR (modifikasi material vs revisi CF) | §5.12.8          | FSD-EIR-05     | TC-EIR-300 to 350 |
| BR-EIR-014 | EIR pada 6 kombinasi reklasifikasi                   | §5.12.10         | FSD-EIR-06     | TC-EIR-400 to 430 |
| BR-PRD-002 | Periode 3-status (OPEN/SOFT/CLOSED)                  | §5.1.7, 5.9      | FSD-PRD-02     | TC-PRD-050 to 100 |
| BR-PRD-009 | Prior-period adjustment (PSAK 25)                    | §5.9             | FSD-PRD-04     | TC-PRD-150 to 170 |
| BR-FX-002  | BI JISDOR scheduled update 10:30 WIB                 | §5.10            | FSD-FX-02      | TC-FX-100 to 120  |
| BR-FX-009  | FX gain/loss treatment per klasifikasi               | §5.1.8 FX        | FSD-FX-03      | TC-FX-150 to 200  |
| BR-JNL-003 | Resolusi runtime mapping jurnal                      | §5.1.10          | FSD-JNL-03     | TC-JNL-100 to 150 |
| BR-JNL-004 | Validasi balance D=K per posting                     | §5.1.10          | FSD-JNL-04     | TC-JNL-180 to 200 |
| BR-RPT-007 | Saldo CKPN Roll-Forward Report                       | §10.3.1          | FSD-RPT-03     | TC-RPT-050 to 080 |
| BR-RPT-017 | Amortization Schedule Report (NEW)                   | §10.3 NEW        | FSD-RPT-12     | TC-RPT-200 to 220 |
| BR-RPT-019 | Roll-Forward Carrying Amount Report (NEW)            | §10.3 NEW        | FSD-RPT-14     | TC-RPT-260 to 280 |
| BR-DOC-002 | Daftar wajib upload per event                        | §4.6.1           | FSD-DOC-02     | TC-DOC-050 to 100 |
| BR-DOC-004 | SHA-256 hash + audit trail dokumen                   | §5.8, 10.2       | FSD-DOC-03     | TC-DOC-150 to 180 |

*Catatan: Sample di atas menampilkan \~32 BR-ID prioritas High. Total BR di BRD ini ada sekitar 180+ requirements yang seluruhnya akan ter-track di matrix master sepanjang implementasi. Setiap CR (Change Request) yang menambah/mengubah requirement akan menambah row baru di matrix.*

# 17\. Glossary

Glossary ini menggabungkan istilah-istilah utama dari SoW v1.1 §1.3 dan istilah baru yang relevan untuk konteks BRD. Definisi lengkap untuk seluruh istilah teknis tersedia di SoW v1.1 §1.3.

| **Istilah**     | **Definisi**                                                                                          |
| --------------- | ----------------------------------------------------------------------------------------------------- |
| AC              | Amortized Cost — klasifikasi pengukuran biaya perolehan diamortisasi (PSAK 71).                       |
| ALCO            | Asset-Liability Committee — komite yang menyetujui parameter risiko (bobot skenario, MEV, Impact PD). |
| BRD             | Business Requirements Document — dokumen ini; menetapkan kebutuhan bisnis & fungsional.               |
| BLIPS           | Nama internal sistem instrumen investasi Tugure (akronim project).                                    |
| BM Test         | Business Model Test — uji model bisnis pengelolaan portofolio (HTC | HTC\&S | Other).                 |
| Carrying Amount | Nilai tercatat aset keuangan; untuk AC = setelah amortisasi via EIR & dikurangi CKPN.                 |
| CFO             | Chief Financial Officer — sponsor proyek BLIPS IFRS 9.                                                |
| CKPN            | Cadangan Kerugian Penurunan Nilai — pos akuntansi untuk ECL.                                          |
| CoA             | Chart of Accounts — struktur kode akun general ledger.                                                |
| DSAK            | Dewan Standar Akuntansi Keuangan IAI — penyusun PSAK.                                                 |
| EAD             | Exposure at Default — nilai eksposur saat default; basis perhitungan ECL.                             |
| ECL             | Expected Credit Loss — cadangan kerugian penurunan nilai berbasis ekspektasi (PSAK 71).               |
| EIR             | Effective Interest Rate / Suku Bunga Efektif — tingkat diskonto cash flow kontraktual (PSAK 71 §5.4). |
| FSD             | Functional Specification Document — dokumen spesifikasi teknis level granular per modul.              |
| FVOCI           | Fair Value through Other Comprehensive Income — klasifikasi PSAK 71 dengan MTM ke OCI.                |
| FVOCI Election  | Opsi irrevocable untuk equity instrument; gain/loss tetap di OCI (no recycling).                      |
| FVTPL           | Fair Value through Profit or Loss — klasifikasi PSAK 71 dengan MTM ke P\&L.                           |
| HTC             | Held to Collect — model bisnis menahan aset untuk memungut arus kas kontraktual.                      |
| HTC\&S          | Held to Collect & Sell — model bisnis menahan untuk arus kas DAN menjual.                             |
| IBPA            | Indonesia Bond Pricing Agency — penyedia harga referensi obligasi.                                    |
| IDR Equivalent  | Nilai dalam IDR dari instrumen valas, dihitung dengan kurs Tengah BI.                                 |
| IFRS 9          | International Financial Reporting Standard 9 — selaras dengan PSAK 71.                                |
| JISDOR          | Jakarta Interbank Spot Dollar Rate — kurs USD/IDR resmi BI.                                           |
| LGD             | Loss Given Default — proporsi kerugian terhadap eksposur jika default.                                |
| LPS             | Lembaga Penjamin Simpanan — menjamin Cash + Deposito Rp 2 Miliar/nasabah/bank.                        |
| MEV             | Macroeconomic Variables — GDP, inflasi, BI Rate, USD/IDR, dll; basis Impact MEV to PD.                |
| MTM             | Mark-to-Market — penyesuaian nilai instrumen ke harga pasar terkini.                                  |
| NAB             | Nilai Aktiva Bersih — harga per unit reksadana.                                                       |
| NFR             | Non-Functional Requirements — requirement kualitas operasional (performance, security, dll).          |
| OCI             | Other Comprehensive Income — komponen ekuitas untuk gain/loss yang belum direalisasi.                 |
| PD              | Probability of Default — probabilitas counterparty default dalam horizon waktu tertentu.              |
| Pefindo         | PT Pemeringkat Efek Indonesia — sumber rating dan PD historis.                                        |
| PMO             | Project Management Office — fungsi project management & governance.                                   |
| PSAK 71         | Pernyataan Standar Akuntansi Keuangan No. 71 — Instrumen Keuangan.                                    |
| PSAK 25         | Kebijakan Akuntansi, Perubahan Estimasi & Kesalahan.                                                  |
| PSAK 65         | Konsolidasian — basis untuk look-through reksadana.                                                   |
| RACI            | Responsible-Accountable-Consulted-Informed — matriks tanggung jawab.                                  |
| RPO             | Recovery Point Objective — toleransi data loss saat disaster.                                         |
| RTO             | Recovery Time Objective — toleransi downtime saat disaster.                                           |
| SHA-256         | Secure Hash Algorithm 256-bit — untuk integrity check dokumen upload.                                 |
| SICR            | Significant Increase in Credit Risk — trigger migrasi Stage 1 → Stage 2 (PSAK 71).                    |
| SLA             | Service Level Agreement — komitmen tingkat layanan.                                                   |
| SoW             | Scope of Work — dokumen teknis project BLIPS IFRS 9 (v1.1).                                           |
| SPPI            | Solely Payments of Principal and Interest — uji karakteristik arus kas (PSAK 71).                     |
| SSO             | Single Sign-On — autentikasi terintegrasi via SAML 2.0 atau OAuth.                                    |
| Stage 1 / 2 / 3 | Klasifikasi kondisi kredit per PSAK 71: Performing / Underperforming / Credit-Impaired.               |
| Tugure          | PT Tugu Reasuransi Indonesia — perusahaan reasuransi nasional.                                        |
| UAT             | User Acceptance Testing — fase testing oleh business user sebelum go-live.                            |
| WAF             | Web Application Firewall — security perimeter untuk aplikasi web.                                     |

# 18\. Lampiran

## 18.1 Lampiran A — Detail Reference ke SoW v1.1

Section ini menyediakan cross-reference detail antara struktur BRD dan SoW v1.1. BRD bersifat business-level; SoW menyediakan technical detail. Untuk pertanyaan teknis spesifik (mis. format field, enum value, formula matematis), refer ke SoW v1.1.

| **BRD Bab**             | **Topik**                                | **Counterpart di SoW v1.1**                          |
| ----------------------- | ---------------------------------------- | ---------------------------------------------------- |
| Bab 6 Scope             | Modul fungsional                         | Bab 2 Ruang Lingkup, Bab 3 Arsitektur Fungsional     |
| Bab 7 Future State      | Visi sistem                              | Bab 3 Arsitektur Fungsional                          |
| Bab 8.1 Master Data     | CRUD entities & schema                   | Bab 5.1 (5.1.1 sd 5.1.10)                            |
| Bab 8.2 SPPI/BM         | Klasifikasi engine                       | Bab 4 Klasifikasi PSAK 71                            |
| Bab 8.3-8.7 Transaksi   | Lifecycle penempatan-MTM-renewal-jual-JT | Bab 5.2 sd 5.6                                       |
| Bab 8.8 Pendapatan      | Akrual bunga & pendapatan                | Bab 5.7                                              |
| Bab 8.9 Media Upload    | Document management                      | Bab 5.8                                              |
| Bab 8.10 ECL Engine     | Perhitungan ECL                          | Bab 7 Framework ECL & Bab 8 Perhitungan ECL per Tipe |
| Bab 8.11 EIR            | EIR & Amortisasi                         | Bab 5.12 Modul EIR & Amortisasi (NEW v1.1)           |
| Bab 8.12 Periode Buku   | Financial period management              | Bab 5.9                                              |
| Bab 8.13 FX             | Multi-currency                           | Bab 5.1.8, 5.10                                      |
| Bab 8.14 Mapping Jurnal | Master Mapping & GL Interface            | Bab 5.1.10, 5.11                                     |
| Bab 8.15 Reporting      | Reporting & dashboard                    | Bab 10.3                                             |
| Bab 9 NFR               | Non-functional                           | Implicit di SoW (perlu detail FSD)                   |
| Bab 10 Compliance       | PSAK 71/25/65, Basel, BI                 | Tersebar di seluruh SoW                              |
| Bab 14 Acceptance       | Acceptance criteria                      | Bab 11.3 Acceptance Criteria                         |
| Bab 15 Implementation   | Phased delivery                          | Bab 11.2 Milestone Indikatif                         |

## 18.2 Lampiran B — Sample SPPI Test Checklist (Reference)

Checklist 10-pertanyaan SPPI Test sesuai SoW v1.1 §4.1.2 (untuk reference, detail di SoW):

Q1. Apakah pokok dan bunga didefinisikan jelas?

Q2. Apakah ada leverage / multiplier?

Q3. Apakah ada link ke variabel non-kredit (komoditas, indeks saham)?

Q4. Apakah ada fitur konversi ekuitas?

Q5. Apakah ada fitur subordination yang non-standar?

Q6. Apakah ada fitur prepayment / extension?

Q7. Apakah suku bunga reset menggunakan tenor yang konsisten?

Q8. Apakah modifikasi suku bunga "de minimis"?

Q9. Apakah aset dijamin oleh aset tertentu (non-recourse)?

Q10. Apakah ada fitur kontingen yang dapat memodifikasi arus kas?

*Auto-evaluate: instrumen lulus SPPI bila Q1 = Yes dan Q2-Q5,Q7,Q8,Q10 tidak menghasilkan FAIL indicator.*

## 18.3 Lampiran C — Matriks Klasifikasi PSAK 71 (Reference)

Matriks SPPI × Business Model untuk auto-derive klasifikasi (sesuai SoW v1.1 §4.3):

| **Hasil SPPI** | **Held to Collect (HTC)** | **Held to Collect & Sell (HTC\&S)** | **Other (Trading/FV-managed)** |
| -------------- | ------------------------- | ----------------------------------- | ------------------------------ |
| PASS           | Amortized Cost (AC)       | FVOCI (with recycling)              | FVTPL                          |
| FAIL           | FVTPL                     | FVTPL                               | FVTPL                          |

**Pengecualian khusus:**

  - Saham (FAIL SPPI) → default FVTPL; tersedia opsi FVOCI Election irrevocable (no recycling) untuk strategic holdings dengan approval Komite Investasi.

  - Reksadana (FAIL SPPI) → default FVTPL; dapat diklasifikasi FVOCI dengan kebijakan akuntansi (HTC\&S) untuk RDN long-term.

## 18.4 Lampiran D — Project Charter Reference

Project Charter formal akan disusun terpisah pasca BRD approval, dengan komponen minimum: Project Sponsor, Project Manager, Steering Committee, Working Group composition, RACI matrix, budget allocation, timeline, success criteria, communication plan, risk management framework, change management framework. Charter ditandatangani sebelum Phase 1 (Discovery) kick-off.

## 18.5 Lampiran E — Change Request Process

Setelah BRD sign-off, perubahan scope mengikuti formal Change Request (CR) process:

1\. Initiator (siapapun) submit CR Form ke PMO dengan: deskripsi perubahan, justifikasi, dampak (scope, schedule, budget, risk), prioritas.

2\. PMO log CR di register; assign CR-ID; lakukan initial assessment.

3\. Working Group review feasibility; estimasi effort.

4\. CR di-categorize: (a) Minor (impact \< 5% scope/budget) → approve oleh Working Group + PMO; (b) Major (impact ≥ 5%) → escalate ke Steering Committee.

5\. Approved CR → BRD addendum, FSD update, schedule re-baseline jika perlu.

6\. Rejected CR → tetap di-log untuk traceability; dapat di-revisit nanti.

Target turnaround: Minor CR ≤ 5 hari kerja; Major CR ≤ 15 hari kerja.

# Sign-Off Page

Dokumen Business Requirements Document ini menjadi dasar pelaksanaan proyek BLIPS IFRS 9 Sistem Instrumen Investasi di PT Tugu Reasuransi Indonesia (Tugure). Dengan menandatangani halaman ini, pihak-pihak di bawah menyatakan bahwa kebutuhan bisnis dalam dokumen ini sudah ter-align dengan sasaran corporate, dan memberikan otorisasi untuk lanjut ke tahap berikutnya.

**Disusun oleh:**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Project Manager BLIPS IFRS 9 / PMO                               | Lead Business Analyst                                            |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

**Direview oleh:**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Komite Investasi (Chair)                                         | ALCO / Komite Risiko (Chair)                                     |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Direktur Investasi & Treasury                                    | Direktur Risk Management                                         |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Kepala Akuntansi & Pelaporan                                     | Direktur Teknologi Informasi                                     |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

**Disetujui oleh (Sign-Off Authority):**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Direktur Keuangan (CFO)                                          | Direktur Utama                                                   |
| Sponsor Proyek                                                   | Steering Committee Chair                                         |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

*Dengan tanda tangan di atas, BRD ini berstatus APPROVED dan menjadi baseline untuk seluruh fase implementasi proyek BLIPS IFRS 9.*

*--- AKHIR DOKUMEN ---*
