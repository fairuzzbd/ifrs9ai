*\[ LOGO TUGURE \]*

**ENTITY RELATIONSHIP DIAGRAM (ERD)**

**BLIPS IFRS 9 — ENTITY RELATIONSHIP DIAGRAM**

*Database Schema Specification • 9 Schemas • \~50 Tables*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**           | **Keterangan**                                                                |
| --------------------- | ----------------------------------------------------------------------------- |
| Judul Dokumen         | Entity Relationship Diagram (ERD) — Sistem BLIPS IFRS 9                       |
| Kode Dokumen          | ERD-BLIPS-IFRS9-2026-001                                                      |
| Versi                 | 1.2                                                                           |
| Status                | DRAFT FOR REVIEW                                                              |
| Tanggal Terbit        | 02 Mei 2026                                                                   |
| Bahasa                | Bahasa Indonesia (technical schema in English)                                |
| Klasifikasi Informasi | INTERNAL — CONFIDENTIAL                                                       |
| Pemilik Dokumen       | Direktorat Teknologi Informasi (Database Architect Lead)                      |
| Penyusun              | Database Architect, IT Architect, Lead Developer                              |
| Reviewer              | PMO, Working Group, Vendor Implementor                                        |
| Approver              | Direktur Teknologi Informasi + CFO                                            |
| Reference Upstream    | FSD Master v1.0; FSD Appendix A v1.1; FSD Appendix B v1.1; BRD v1.1; SoW v1.3 |
| Companion Artifact    | BLIPS\_init\_schema.sql — ready-to-execute DDL script                         |
| Target DBMS           | PostgreSQL 15+ (primary); Oracle 19c+ atau SQL Server 2019+ (alternatif)      |
| Total Schemas         | 9 (mst, trx, ecl, sppi, doc, jrnl, aud, sec, sys)                             |
| Total Entitas         | ± 52 tables (incl. 2 new upload batch tables v1.2)                            |

# Revision History

| **Versi** | **Tanggal** | **Penyusun**          | **Ringkasan**                                 |
| --------- | ----------- | --------------------- | --------------------------------------------- |
| 0.1       | 13 Apr 2026 | DB Architect          | Initial schema design (mst+trx)               |
| 0.5       | 23 Apr 2026 | DB Architect          | Add ecl/eir schemas + cross-references        |
| 0.9       | 30 Apr 2026 | DB Architect + Vendor | Refine indexing, partitioning, audit triggers |
| 1.0       | 02 Mei 2026 | DB Architect          | Final draft + DDL script ready                |

# Reference Documents

| **Tipe**  | **Kode**                       | **Judul**                                                            |
| --------- | ------------------------------ | -------------------------------------------------------------------- |
| Upstream  | BRD-BLIPS-IFRS9-2026-001 v1.0  | Business Requirements Document                                       |
| Upstream  | SOW-BLIPS-IFRS9-2026-001 v1.1  | Scope of Work (with EIR module)                                      |
| Upstream  | FSD-BLIPS-MASTER-2026-001 v1.0 | FSD Master — Cross-Cutting (mendefinisikan database design strategy) |
| Upstream  | FSD-APP-A v1.0                 | Master Data + SPPI/BM Test                                           |
| Upstream  | FSD-APP-B v1.0                 | Transaction Lifecycle                                                |
| Upstream  | FSD-APP-C v1.0                 | ECL Engine + EIR & Amortisasi                                        |
| Upstream  | FSD-APP-D v1.0                 | Periode Buku + FX + Mapping Jurnal                                   |
| Upstream  | FSD-APP-E v1.0                 | Reporting & Dashboard                                                |
| Companion | BLIPS\_init\_schema.sql        | DDL script ready-to-execute                                          |
| Standard  | ISO/IEC 11179                  | Metadata registries — naming conventions                             |
| Standard  | PostgreSQL 15 Documentation    | Target DBMS spec                                                     |

# Daftar Isi

Outline 18 Bab utama:

1\. Pendahuluan

2\. Conceptual Model — High-Level Entity Overview

3\. Schema Architecture (9 Schemas)

4\. Schema mst — Master Data (13 entities)

5\. Schema trx — Transactional Data (7 entities)

6\. Schema ecl — ECL/EIR Compliance (6 entities)

7\. Schema sppi — SPPI/BM/Klasifikasi (4 entities)

8\. Schema doc — Document Management (3 entities)

9\. Schema jrnl — Jurnal & GL Interface (3 entities)

10\. Schema aud — Audit Trail (3 entities)

11\. Schema sec — Security & RBAC (5 entities)

12\. Schema sys — System Configuration (4 entities)

13\. Cross-Schema Relationships

14\. Data Dictionary (Alphabetical Reference)

15\. Indexing Strategy

16\. Partitioning Strategy

17\. Migration & Initial Seeding

18\. Lampiran (Naming Conventions, Common Columns)

# 1\. Pendahuluan

## 1.1 Tujuan Dokumen

Entity Relationship Diagram (ERD) ini menetapkan model data lengkap untuk sistem BLIPS IFRS 9 Instrumen Investasi PT Tugu Reasuransi Indonesia. Dokumen mencakup 9 schemas dan ±50 tables, dengan detail per-entitas (atribut, tipe data, constraint, FK relationships, indexes), Mermaid ER diagrams per cluster, dan companion DDL script ready-to-execute.

ERD ini menjadi single source of truth untuk database design — di-derive dari FSD spec yang sudah disetujui, dan menjadi basis untuk migration scripts, ORM mapping (Hibernate JPA / Entity Framework), dan query optimization.

## 1.2 Audiens Dokumen

  - Database Architect & DBA — sebagai blueprint untuk schema implementation, tuning, monitoring.

  - Backend Developer — sebagai reference untuk ORM entity mapping & query design.

  - Vendor Implementor — sebagai spec lengkap untuk delivery database layer.

  - DevOps / DBA Ops — untuk migration script execution, backup/recovery planning, partitioning maintenance.

  - Internal Audit & Security — untuk review data classification, encryption, audit trail integrity.

  - Quality Assurance — untuk database integrity testing.

## 1.3 Methodology

ERD dirancang mengikuti prinsip:

  - Normalisasi 3NF (Third Normal Form) untuk OLTP entities; selective denormalization untuk reporting via materialized views (lihat FSD Appendix E).

  - Soft delete pattern (is\_deleted flag) untuk semua master & transactional entities — hard delete tidak diperbolehkan kecuali untuk data ephemeral.

  - Audit columns wajib (created\_by/at, updated\_by/at, version) — lihat Bab 18.2 untuk detail.

  - UUID v7 (time-ordered) sebagai primary key — universal uniqueness + index efficiency.

  - Partitioning untuk tabel volume besar (audit\_log, transaksi historical) — lihat Bab 16.

  - FK dengan ON DELETE RESTRICT (default) untuk integrity; ON DELETE CASCADE hanya untuk parent-child dengan no audit value (mis. mapping\_jurnal\_detail child of mapping\_jurnal\_header).

  - CHECK constraints untuk business invariants di level database (defense-in-depth).

  - Encryption at column level untuk PII (NPWP, no rekening) menggunakan pgcrypto extension atau application-level encryption.

## 1.4 Konvensi Notasi ERD

Notasi yang digunakan:

| **Notasi**           | **Arti**                                                                   |
| -------------------- | -------------------------------------------------------------------------- |
| PK                   | Primary Key — unique identifier, NOT NULL                                  |
| FK → tbl             | Foreign Key referencing target table                                       |
| UQ                   | Unique Constraint (single column atau composite)                           |
| IDX                  | Index (non-unique)                                                         |
| CK                   | Check Constraint                                                           |
| NOT NULL / NULL      | Nullability                                                                |
| DEFAULT x            | Default value                                                              |
| 1..1                 | One to One relationship                                                    |
| 1..N atau 1..\*      | One to Many                                                                |
| N..M                 | Many to Many (via junction table)                                          |
| 0..1                 | Optional one (nullable FK)                                                 |
| {audit}              | Shorthand untuk standard audit columns (lihat Bab 18.2)                    |
| UUID                 | uuid type, generated via uuidv7() function                                 |
| TIMESTAMPTZ          | Timezone-aware timestamp (UTC stored)                                      |
| NUMERIC(p,s)         | Fixed precision number; p = total digit, s = decimal                       |
| VARCHAR(n)           | Variable-length text, max n characters                                     |
| JSONB                | Binary JSON (PostgreSQL); for flexible schema (audit before/after)         |
| ENUM (lookup\_group) | Stored as VARCHAR; values validated via FK ke sys\_lookup atau application |

## 1.5 Hubungan dengan FSD

ERD ini bersifat downstream dari FSD. FSD menyediakan field-level specifications (lihat misalnya FSD Appendix A §1.1.3 untuk Master Instrumen). ERD mengintegrasikan field-field tersebut menjadi model data lengkap dengan:

  - Detail tipe data SQL spesifik (mis. NUMERIC(20,2) bukan abstrak NUMBER).

  - Constraint database-level (PK, FK, UQ, CK).

  - Indexes (PK index implicit, plus FK indexes, plus business indexes).

  - Partitioning specifications (untuk historical tables).

  - Trigger logic (untuk audit, immutability, soft-delete).

# 2\. Conceptual Model — High-Level Entity Overview

## 2.1 Bird's-Eye View

Pada level tertinggi, sistem BLIPS dibangun di sekitar 4 domain utama:

| **Domain**               | **Core Entities**                                                        | **Tujuan**                                                                  |
| ------------------------ | ------------------------------------------------------------------------ | --------------------------------------------------------------------------- |
| Investasi                | Instrumen, Counterparty, Portofolio                                      | Identifikasi aset keuangan dan pihak terkait                                |
| Klasifikasi & Compliance | SPPI Test, BM Test, Klasifikasi History, Reklasifikasi                   | Penetapan klasifikasi PSAK 71 (AC/FVOCI/FVTPL) — compliance backbone        |
| Transaksi Lifecycle      | Penempatan, MTM, Renewal, Penjualan, Jatuh Tempo, Pendapatan, Amortisasi | Pencatatan event seluruh siklus hidup instrumen                             |
| Compliance Computation   | ECL Calc, EIR Schedule, Stage History, Reestimation Log                  | Engine perhitungan PSAK 71 — Expected Credit Loss & Effective Interest Rate |

## 2.2 Conceptual ER Diagram (Mermaid)

Mermaid syntax — dapat di-render di Confluence, Notion, GitHub, atau tools yang support Mermaid (mis. mermaid.live):

> \`\`\`mermaid  
> erDiagram  
> PORTOFOLIO ||--o{ INSTRUMEN : "berisi"  
> COUNTERPARTY ||--o{ INSTRUMEN : "diterbitkan oleh"  
> COUNTERPARTY ||--o{ RATING\_HISTORY : "memiliki rating"  
> INSTRUMEN ||--|| SPPI\_TEST : "diuji"  
> INSTRUMEN ||--|| BM\_TEST : "via portofolio"  
> INSTRUMEN ||--o{ KLASIFIKASI\_HISTORY : "klasifikasinya"  
> INSTRUMEN ||--o{ REKLASIFIKASI\_LOG : "direklasifikasi"  
> INSTRUMEN ||--o{ TRX\_PENEMPATAN : "ditempatkan"  
> INSTRUMEN ||--o{ TRX\_MTM : "dimark-to-market"  
> INSTRUMEN ||--o{ TRX\_RENEWAL : "diperpanjang"  
> INSTRUMEN ||--o{ TRX\_PENJUALAN : "dijual"  
> INSTRUMEN ||--o{ TRX\_JATUH\_TEMPO : "jatuh tempo"  
> INSTRUMEN ||--o{ TRX\_PENDAPATAN\_AKRUAL : "akrual harian"  
> INSTRUMEN ||--o{ EIR\_AMORTIZATION\_SCHEDULE : "schedule amortisasi"  
> INSTRUMEN ||--o{ EIR\_REESTIMATION\_LOG : "log re-estimation"  
> INSTRUMEN ||--o{ ECL\_CALC\_HEADER : "ECL per periode"  
> INSTRUMEN ||--o{ ECL\_STAGE\_HISTORY : "stage history"  
> ECL\_CALC\_HEADER ||--o{ ECL\_CALC\_DETAIL\_SKENARIO : "3 skenario"  
> ECL\_CALC\_HEADER ||--o{ ECL\_LOOKTHROUGH\_UNDERLYING : "look-through (RDN)"  
> PERIODE\_BUKU ||--o{ TRX\_PENEMPATAN : "stamp periode"  
> PERIODE\_BUKU ||--o{ TRX\_MTM : "stamp"  
> PERIODE\_BUKU ||--o{ ECL\_CALC\_HEADER : "stamp"  
> PERIODE\_BUKU ||--o{ JRNL\_HEADER : "stamp"  
> PD\_PEFINDO ||..|| ECL\_CALC\_HEADER : "lookup"  
> LGD\_BASEL ||..|| ECL\_CALC\_HEADER : "lookup"  
> KURS ||..|| TRX\_PENEMPATAN : "konversi IDR"  
> KURS ||..|| ECL\_CALC\_HEADER : "konversi IDR"  
> MAPPING\_JURNAL\_HEADER ||--o{ MAPPING\_JURNAL\_DETAIL : "header-detail"  
> MAPPING\_JURNAL\_HEADER ||..|| JRNL\_HEADER : "template"  
> JRNL\_HEADER ||--o{ JRNL\_DETAIL : "lines"  
> CHART\_OF\_ACCOUNTS ||--o{ JRNL\_DETAIL : "akun"  
> INSTRUMEN ||--o{ DOC\_LINK : "dokumen pendukung"  
> DOC\_UPLOAD ||--o{ DOC\_LINK : "linked to entity"  
> DOC\_UPLOAD ||--o{ DOC\_ACCESS\_LOG : "access log"  
> SEC\_USER ||--o{ AUDIT\_LOG : "actor"  
> SEC\_USER }o--o{ SEC\_ROLE : "via sec\_user\_role"  
> SEC\_ROLE ||--o{ SEC\_PERMISSION : "has"  
> \`\`\`

## 2.3 Textual ASCII Conceptual Diagram

> ┌─────────────────────┐  
> │ PORTOFOLIO │  
> │ (groupBM\_TEST) │  
> └────────┬────────────┘  
> │ 1..N  
> ▼  
> ┌────────────┐ ┌──────────────────┐ ┌────────────────┐  
> │COUNTERPARTY├─────▶│ INSTRUMEN │◀─────┤ RATING\_HIST │  
> │ (issuer) │ 1..N │ (asset register)│ │ (per CP, time) │  
> └─────┬──────┘ │ klasifikasi=lock│ └────────────────┘  
> │ 1..N └────┬─────┬─────┬─┘  
> │ │ │ │  
> ▼ ▼ ▼ ▼  
> ┌────────────┐ ┌──────────┐ │ ┌────────────┐  
> │ RATING\_HIST│ │SPPI\_TEST │ │ │ BM\_TEST │  
> └────────────┘ │KLASIFIKAS│ │ │ │  
> │ HISTORY │ │ │ │  
> └──────────┘ │ └────────────┘  
> ▼  
> ┌──────────────────────┴───────────────────────┐  
> │ TRANSACTION LIFECYCLE │  
> ├──────────────┬───────────────┬───────────────┤  
> │ PENEMPATAN │ MTM │ RENEWAL │  
> │ PENJUALAN │ JATUH\_TEMPO │ PENDAPATAN │  
> │ AMORTISASI │ │ │  
> └───────┬──────┴───────┬───────┴───────────────┘  
> │ │  
> ▼ ▼  
> ┌────────────────┐ ┌─────────────────────────┐  
> │ EIR\_SCHEDULE │ │ ECL\_CALC\_HEADER │  
> │ (per instrumen)│ │ + DETAIL\_SKENARIO │  
> │ │ │ + LOOKTHROUGH │  
> │ EIR\_REESTIMATION│ │ + STAGE\_HISTORY │  
> └────────────────┘ └─────────────────────────┘  
> │ │  
> └───────┬───────────────┘  
> ▼  
> ┌────────────────────┐  
> │ JRNL\_HEADER │  
> │ + JRNL\_DETAIL │  
> │ → GL HOST │  
> └────────────────────┘  
>   
> Cross-cutting:  
> PERIODE\_BUKU (stamp periode\_id pada semua transaksi)  
> MATA\_UANG + KURS (konversi IDR)  
> PD\_PEFINDO + LGD\_BASEL + IMPACT\_MEV + IMPACT\_PD (lookup ECL)  
> CHART\_OF\_ACCOUNTS + MAPPING\_JURNAL (template jurnal)  
> DOC\_UPLOAD + DOC\_LINK (lampiran event)  
> AUDIT\_LOG (immutable trail untuk semua action)  
> SEC\_USER + SEC\_ROLE + SEC\_PERMISSION (RBAC)

## 2.4 Domain Boundary & Bounded Context

Mengikuti prinsip Domain-Driven Design, sistem BLIPS terdiri dari beberapa bounded context:

| **Bounded Context**        | **Schema** | **Aggregate Root**                                                        |
| -------------------------- | ---------- | ------------------------------------------------------------------------- |
| Reference Data             | mst        | Instrumen, Counterparty, Portofolio (3 aggregates utama)                  |
| Compliance Engine          | sppi, ecl  | SPPI Test, BM Test, ECL Calc Header, EIR Schedule, Klasifikasi History    |
| Transaction Recording      | trx        | Setiap event transaksi adalah aggregate (Penempatan, MTM, Penjualan, dll) |
| Document Management        | doc        | Doc Upload (with linked metadata)                                         |
| Accounting Posting         | jrnl       | Jurnal Header (with detail children)                                      |
| Audit & Compliance Logging | aud        | Audit Log entry (immutable per record)                                    |
| Identity & Access Mgmt     | sec        | User, Role, Session                                                       |
| System Configuration       | sys        | Config, Lookup, Job Run                                                   |

# 3\. Schema Architecture (9 Schemas)

## 3.1 Schema List

| **\#** | **Schema** | **Purpose**                                                               | **Est. \# Tables** |
| ------ | ---------- | ------------------------------------------------------------------------- | ------------------ |
| 1      | mst        | Master/Reference Data — instrumen, counterparty, parameter, lookup master | 13                 |
| 2      | trx        | Transactional Data — lifecycle event per instrumen                        | 7                  |
| 3      | ecl        | ECL/EIR Compliance Computation — calc result, schedule, log               | 6                  |
| 4      | sppi       | SPPI/BM/Klasifikasi/Reklasifikasi                                         | 4                  |
| 5      | doc        | Document Management — upload, link, access log                            | 3                  |
| 6      | jrnl       | Jurnal & GL Interface — posting result, GL delivery status                | 3                  |
| 7      | aud        | Audit Trail — immutable event log, workflow history, login history        | 3                  |
| 8      | sec        | Security/RBAC — user, role, permission, session                           | 5                  |
| 9      | sys        | System — config, lookup, job run history, notification template           | 4                  |
|        | TOTAL      |                                                                           | \~48               |

## 3.2 Schema Privilege Matrix

Untuk security hardening, setiap schema memiliki privilege scope yang berbeda — application service account mendapat permission spesifik per schema, bukan superuser.

| **Schema** | **Read**                                 | **Write**                                        | **DDL**        |
| ---------- | ---------------------------------------- | ------------------------------------------------ | -------------- |
| mst        | All app services + Reporting service     | Master data services + workflow engine           | Migration only |
| trx        | Transaction services + Reporting         | Transaction services                             | Migration      |
| ecl        | ECL service + Reporting + Auditor        | ECL service (compute)                            | Migration      |
| sppi       | SPPI service + workflow + Reporting      | SPPI service                                     | Migration      |
| doc        | Document service + all (read links)      | Document service only                            | Migration      |
| jrnl       | Reporting + Auditor + GL Service         | Posting service + GL Service                     | Migration      |
| aud        | Auditor (read-only via audit role)       | Append-only via trigger; tidak ada UPDATE/DELETE | Migration only |
| sec        | Auth service + IT Admin                  | Auth service + IT Admin (limited)                | Migration      |
| sys        | All services (lookup); IT Admin (config) | IT Admin only                                    | Migration      |

## 3.3 Schema Relationships Overview

Relasi cross-schema yang utama (detail lengkap di Bab 13):

| **Source Schema** | **Target Schema** | **Relationship**                                                     |
| ----------------- | ----------------- | -------------------------------------------------------------------- |
| trx → mst         | —                 | Setiap trx record FK ke instrumen, periode\_buku, kurs (untuk valas) |
| ecl → mst         | —                 | ECL calc lookup PD/LGD/Bobot/MEV; FK ke instrumen, periode           |
| ecl → trx         | —                 | ECL trigger event mengikuti transaction lifecycle                    |
| sppi → mst        | —                 | SPPI/BM Test FK ke instrumen, portofolio                             |
| jrnl → mst        | —                 | Jurnal detail FK ke chart\_of\_accounts, mapping\_jurnal             |
| doc → all         | —                 | doc\_link generic FK ke entity di banyak schemas                     |
| aud → all         | —                 | audit\_log generic; entity\_type+entity\_id refer ke any table       |
| sec → all         | —                 | Setiap audit field (created\_by/approved\_by) FK ke sec.user         |

## 3.4 Schema Initialization Order

DDL initialization order berdasarkan dependency graph:

1.  sec — (foundation untuk audit columns FK)

2.  sys — (lookup data + config)

3.  mst — (reference data dengan FK ke sec)

4.  doc — (document base; doc\_link dapat di-init kapan saja karena pakai polymorphic FK)

5.  sppi — (FK ke mst.instrumen)

6.  trx — (FK ke mst.instrumen, mst.periode\_buku, mst.kurs)

7.  jrnl — (FK ke mst.chart\_of\_accounts, mst.mapping\_jurnal)

8.  ecl — (FK ke mst.instrumen, mst.pd\_pefindo, mst.lgd\_basel, mst.periode\_buku)

9.  aud — (last; trigger-based; FK ke semua schema lain via polymorphic)

# 4\. Schema mst — Master Data (16 Entities)

Schema mst (Master) menyimpan reference data yang digunakan lintas modul: instrumen, counterparty, parameter risiko (PD, LGD, MEV, bobot), periode buku, mata uang & kurs, chart of accounts, mapping jurnal, dan portofolio.

## 4.0 Schema-Level ER Diagram (Mermaid)

> \`\`\`mermaid  
> erDiagram  
> mst\_portofolio ||--o{ mst\_instrumen : "berisi"  
> mst\_counterparty ||--o{ mst\_instrumen : "issuer/bank"  
> mst\_counterparty ||--o{ mst\_rating\_history\_counterparty : "rating timeline"  
> mst\_counterparty ||--o{ mst\_instrumen : "MI/kustodian (RDN)"  
> mst\_pd\_pefindo }o..|| mst\_rating\_history\_counterparty : "lookup"  
> mst\_lgd\_basel }o..|| mst\_counterparty : "tipe eksposur"  
> mst\_periode\_buku ||--o{ mst\_kurs : "stamp periode"  
> mst\_mata\_uang ||--o{ mst\_kurs : "kurs harian"  
> mst\_chart\_of\_accounts ||--o{ mst\_chart\_of\_accounts : "parent (self-ref)"  
> mst\_chart\_of\_accounts ||--o{ mst\_mapping\_jurnal\_detail : "akun"  
> mst\_mapping\_jurnal\_header ||--o{ mst\_mapping\_jurnal\_detail : "header-detail"  
> mst\_bobot\_skenario {  
> VARCHAR skenario PK  
> NUMERIC bobot  
> }  
> mst\_impact\_mev\_pd {  
> UUID id PK  
> UUID periode\_id FK  
> VARCHAR skenario  
> NUMERIC impact  
> }  
> mst\_lps\_coverage {  
> UUID id PK  
> DATE effective\_date  
> NUMERIC coverage\_amount  
> }  
> \`\`\`

### 4.1 mst.portofolio

Portofolio merupakan grouping logis instrumen berdasarkan tujuan investasi. Menjadi unit Business Model Test (BM Test) — setiap portofolio memiliki BM Category (HTC/HTC\&S/Other) yang menentukan klasifikasi default instrumen di dalamnya.

*Primary Key: id (UUID, time-ordered uuidv7)*

| **Column**                 | **Type**     | **Null** | **Default** | **Description**                                        |
| -------------------------- | ------------ | -------- | ----------- | ------------------------------------------------------ |
| id                         | UUID         | NOT NULL | uuidv7()    | Primary key                                            |
| kode\_portofolio           | VARCHAR(20)  | NOT NULL | —           | Unique business key (mis. PORT-TR-LIQ, PORT-INV-LT)    |
| nama                       | VARCHAR(200) | NOT NULL | —           | Treasury Liquidity, Investment Long-Term, Trading, dll |
| tujuan\_pengelolaan        | TEXT         | NULL     | —           | Naratif tujuan strategis                               |
| bm\_category\_default      | VARCHAR(10)  | NOT NULL | 'HTC'       | HTC | HTCS | OTHER                                     |
| benchmark                  | VARCHAR(100) | NULL     | —           | Mis. INDOBeX, IBPA Govt Bond Index                     |
| kompensasi\_manager\_basis | VARCHAR(50)  | NULL     | —           | Bunga / Total Return / Fair Value                      |
| periode\_review\_terakhir  | DATE         | NULL     | —           | Tanggal BM Test terakhir                               |
| aktif\_flag                | BOOLEAN      | NOT NULL | TRUE        |                                                        |
| created\_by                | UUID         | NOT NULL | —           | FK → sec.user                                          |
| created\_at                | TIMESTAMPTZ  | NOT NULL | now()       |                                                        |
| updated\_by                | UUID         | NULL     | —           | FK → sec.user                                          |
| updated\_at                | TIMESTAMPTZ  | NULL     | —           |                                                        |
| version                    | INT          | NOT NULL | 1           | Optimistic locking                                     |
| is\_deleted                | BOOLEAN      | NOT NULL | FALSE       | Soft delete                                            |

**Foreign Keys:**

  - created\_by → sec.user(id)

  - updated\_by → sec.user(id)

**Indexes:**

  - uq\_portofolio\_kode UNIQUE(kode\_portofolio) WHERE is\_deleted=FALSE

  - ix\_portofolio\_aktif (aktif\_flag) WHERE is\_deleted=FALSE

**Notes:**

  - BM Category default di-cascade ke instrumen baru di portofolio ini saat klasifikasi initial.

  - Soft delete only — instrumen aktif tetap referensi ke portofolio meski deleted; menggunakan FK RESTRICT.

**DDL Snippet:**

> CREATE TABLE mst.portofolio (  
> id UUID PRIMARY KEY DEFAULT uuidv7(),  
> kode\_portofolio VARCHAR(20) NOT NULL,  
> nama VARCHAR(200) NOT NULL,  
> tujuan\_pengelolaan TEXT,  
> bm\_category\_default VARCHAR(10) NOT NULL DEFAULT 'HTC',  
> benchmark VARCHAR(100),  
> kompensasi\_manager\_basis VARCHAR(50),  
> periode\_review\_terakhir DATE,  
> aktif\_flag BOOLEAN NOT NULL DEFAULT TRUE,  
> created\_by UUID NOT NULL REFERENCES sec.user(id),  
> created\_at TIMESTAMPTZ NOT NULL DEFAULT now(),  
> updated\_by UUID REFERENCES sec.user(id),  
> updated\_at TIMESTAMPTZ,  
> version INT NOT NULL DEFAULT 1,  
> is\_deleted BOOLEAN NOT NULL DEFAULT FALSE,  
> CONSTRAINT ck\_bm\_category CHECK (bm\_category\_default IN ('HTC','HTCS','OTHER'))  
> );  
> CREATE UNIQUE INDEX uq\_portofolio\_kode ON mst.portofolio(kode\_portofolio) WHERE is\_deleted=FALSE;  
> CREATE INDEX ix\_portofolio\_aktif ON mst.portofolio(aktif\_flag) WHERE is\_deleted=FALSE;

### 4.2 mst.counterparty

Counterparty mencakup Bank, Bank Kustodian, Issuer Korporasi, Pemerintah, Manajer Investasi, dan Emiten Saham. Menjadi parent untuk Master Instrumen dan Rating History.

*Primary Key: id (UUID)*

| **Column**               | **Type**      | **Null** | **Default** | **Description**                                                                      |
| ------------------------ | ------------- | -------- | ----------- | ------------------------------------------------------------------------------------ |
| id                       | UUID          | NOT NULL | uuidv7()    | PK                                                                                   |
| kode\_counterparty       | VARCHAR(20)   | NOT NULL | —           | Auto-generate: CP-\#\#\#\#\#                                                         |
| nama                     | VARCHAR(200)  | NOT NULL | —           | Nama legal                                                                           |
| tipe                     | VARCHAR(30)   | NOT NULL | —           | BANK / BANK\_KUSTODIAN / KORPORASI / PEMERINTAH / MANAJER\_INVESTASI / EMITEN\_SAHAM |
| rating\_pefindo\_current | VARCHAR(8)    | NULL     | —           | Rating berlaku saat ini; cache dari rating\_history                                  |
| tipe\_eksposur\_basel    | VARCHAR(30)   | NOT NULL | —           | SOVEREIGN / SENIOR\_SECURED / SENIOR\_UNSECURED / SUBORDINATED                       |
| eligible\_lps\_flag      | BOOLEAN       | NOT NULL | FALSE       | Auto Y untuk BANK                                                                    |
| npwp\_encrypted          | VARCHAR(255)  | NULL     | —           | Encrypted via pgcrypto                                                               |
| nomor\_izin\_ojk         | VARCHAR(40)   | NULL     | —           | Untuk MANAJER\_INVESTASI                                                             |
| tanggal\_izin\_ojk       | DATE          | NULL     | —           | Untuk MI                                                                             |
| aum\_terakhir            | NUMERIC(20,2) | NULL     | —           | Untuk MI; update triwulanan                                                          |
| tanggal\_aum\_terakhir   | DATE          | NULL     | —           | Untuk MI                                                                             |
| kategori\_mi             | VARCHAR(30)   | NULL     | —           | BUMN/SWASTA\_NASIONAL/SWASTA\_ASING/JOINT\_VENTURE                                   |
| status                   | VARCHAR(20)   | NOT NULL | 'AKTIF'     | AKTIF / TIDAK\_AKTIF                                                                 |
| created\_by              | UUID          | NOT NULL | —           |                                                                                      |
| created\_at              | TIMESTAMPTZ   | NOT NULL | now()       |                                                                                      |
| updated\_by              | UUID          | NULL     | —           |                                                                                      |
| updated\_at              | TIMESTAMPTZ   | NULL     | —           |                                                                                      |
| version                  | INT           | NOT NULL | 1           |                                                                                      |
| is\_deleted              | BOOLEAN       | NOT NULL | FALSE       |                                                                                      |

**Foreign Keys:**

  - created\_by → sec.user(id)

  - updated\_by → sec.user(id)

**Indexes:**

  - uq\_counterparty\_kode UNIQUE(kode\_counterparty) WHERE is\_deleted=FALSE

  - ix\_counterparty\_tipe (tipe) WHERE is\_deleted=FALSE

  - ix\_counterparty\_rating (rating\_pefindo\_current) WHERE is\_deleted=FALSE

  - ix\_counterparty\_lps (eligible\_lps\_flag) WHERE eligible\_lps\_flag=TRUE AND is\_deleted=FALSE

**Notes:**

  - rating\_pefindo\_current adalah cache dari rating\_history terkini (tanggal\_berakhir IS NULL).

  - NPWP & nomor rekening dienkripsi at column level menggunakan pgcrypto extension; decryption hanya untuk role tertentu.

  - FK ke mst.instrumen dapat dari multiple kolom: counterparty\_id (issuer), manajer\_investasi\_id, bank\_kustodian\_id.

### 4.3 mst.rating\_history\_counterparty

Rating History menyimpan timeline rating Pefindo per counterparty. Setiap perubahan rating menjadi record baru dengan otomatisasi SICR/Default trigger sesuai PSAK 71. Menjadi sumber re-evaluasi staging ECL.

*Primary Key: id*

| **Column**                 | **Type**    | **Null** | **Default** | **Description**                                |
| -------------------------- | ----------- | -------- | ----------- | ---------------------------------------------- |
| id                         | UUID        | NOT NULL | uuidv7()    | PK                                             |
| rating\_history\_id\_kode  | VARCHAR(20) | NOT NULL | —           | Auto: RTH-YYYY-\#\#\#\#\#                      |
| counterparty\_id           | UUID        | NOT NULL | —           | FK → mst.counterparty                          |
| tanggal\_berlaku           | DATE        | NOT NULL | —           | Effective date rating                          |
| tanggal\_berakhir          | DATE        | NULL     | —           | Auto-set saat rating berikut diinput           |
| rating\_pefindo            | VARCHAR(8)  | NOT NULL | —           | idAAA, idAA+, ..., idD                         |
| rating\_outlook            | VARCHAR(20) | NULL     | —           | POSITIVE/STABLE/NEGATIVE/DEVELOPING            |
| sumber\_rating             | VARCHAR(30) | NOT NULL | —           | PEFINDO\_REGULAR/PEFINDO\_REVIEW/LEMBAGA\_LAIN |
| tanggal\_publikasi\_rating | DATE        | NOT NULL | —           | Tanggal terbit dari Pefindo                    |
| action\_type               | VARCHAR(20) | NOT NULL | —           | INITIAL/UPGRADE/DOWNGRADE/AFFIRMED/WITHDRAWN   |
| notch\_change              | INT         | NOT NULL | 0           | Vs rating sebelumnya; positif=upgrade          |
| sicr\_triggered            | BOOLEAN     | NOT NULL | FALSE       | Auto-calc                                      |
| default\_triggered         | BOOLEAN     | NOT NULL | FALSE       | Auto: TRUE jika idD                            |
| dokumen\_bukti\_id         | UUID        | NULL     | —           | FK → doc.upload (press release Pefindo)        |
| maker\_id                  | UUID        | NOT NULL | —           | Risk Officer                                   |
| approver\_id               | UUID        | NULL     | —           | Risk Manager                                   |
| created\_at                | TIMESTAMPTZ | NOT NULL | now()       |                                                |
| approved\_at               | TIMESTAMPTZ | NULL     | —           |                                                |

**Foreign Keys:**

  - counterparty\_id → mst.counterparty(id) ON DELETE RESTRICT

  - dokumen\_bukti\_id → doc.upload(id)

  - maker\_id → sec.user(id)

  - approver\_id → sec.user(id)

**Indexes:**

  - uq\_rating\_history\_kode UNIQUE(rating\_history\_id\_kode)

  - ix\_rating\_cp\_tanggal (counterparty\_id, tanggal\_berlaku DESC)

  - ix\_rating\_aktif (counterparty\_id) WHERE tanggal\_berakhir IS NULL — only one active rating per CP

  - ix\_rating\_sicr (sicr\_triggered) WHERE sicr\_triggered=TRUE

  - ix\_rating\_default (default\_triggered) WHERE default\_triggered=TRUE

**Notes:**

  - Constraint: hanya satu record dengan tanggal\_berakhir IS NULL per counterparty (enforced via partial unique index).

  - Trigger auto-set tanggal\_berakhir untuk record sebelumnya saat record baru di-insert.

  - Tidak dapat di-delete; hanya dapat di-correct via record baru (action\_type = CORRECTION).

  - Trigger SICR/Default evaluation menyebabkan downstream re-eval ECL untuk semua instrumen counterparty terkait.

### 4.4 mst.pd\_pefindo

Master PD Pefindo menyimpan PD 12-Month dan Lifetime PD per rating, di-load dari Pefindo Default Study triwulanan. Mendukung versioning untuk audit re-perform calculation pada periode lampau.

*Primary Key: id*

| **Column**               | **Type**     | **Null** | **Default** | **Description**                           |
| ------------------------ | ------------ | -------- | ----------- | ----------------------------------------- |
| id                       | UUID         | NOT NULL | uuidv7()    | PK                                        |
| rating                   | VARCHAR(8)   | NOT NULL | —           | idAAA, idAA+, ..., idD                    |
| pd\_12month              | NUMERIC(8,4) | NOT NULL | —           | Stage 1 PD per annum, 4 desimal           |
| pd\_lifetime\_3y         | NUMERIC(8,4) | NULL     | —           | Cumulative 3-year                         |
| pd\_lifetime\_5y         | NUMERIC(8,4) | NULL     | —           | Cumulative 5-year                         |
| pd\_lifetime\_7y         | NUMERIC(8,4) | NULL     | —           | Cumulative 7-year                         |
| pd\_lifetime\_10y        | NUMERIC(8,4) | NULL     | —           | Cumulative 10-year                        |
| sumber                   | VARCHAR(50)  | NOT NULL | —           | PEFINDO\_DS\_2025\_Q4 atau internal model |
| tanggal\_publikasi       | DATE         | NOT NULL | —           |                                           |
| periode\_berlaku\_dari   | DATE         | NOT NULL | —           |                                           |
| periode\_berlaku\_sampai | DATE         | NULL     | —           | NULL = current version                    |
| dokumen\_pendukung\_id   | UUID         | NULL     | —           | FK → doc.upload                           |
| uploaded\_by             | UUID         | NOT NULL | —           |                                           |
| uploaded\_at             | TIMESTAMPTZ  | NOT NULL | now()       |                                           |
| approved\_by             | UUID         | NULL     | —           |                                           |
| approved\_at             | TIMESTAMPTZ  | NULL     | —           |                                           |

**Foreign Keys:**

  - dokumen\_pendukung\_id → doc.upload(id)

  - uploaded\_by → sec.user(id)

  - approved\_by → sec.user(id)

**Indexes:**

  - ix\_pd\_pefindo\_rating\_periode (rating, periode\_berlaku\_dari DESC)

  - ix\_pd\_pefindo\_current (rating) WHERE periode\_berlaku\_sampai IS NULL

  - ck\_pd\_range CHECK (pd\_12month BETWEEN 0 AND 1)

**Notes:**

  - Multiple version per rating dimungkinkan untuk audit re-perform; current version = WHERE periode\_berlaku\_sampai IS NULL.

  - Linear interpolation untuk tenor non-standar (mis. 4 tahun) dilakukan di application layer.

  - Saat upload baru, sistem auto-set periode\_berlaku\_sampai untuk versi sebelumnya.

### 4.5 mst.lgd\_basel

Master LGD per tipe eksposur Basel III IRB Foundation Approach. Static reference table dengan versioning untuk audit.

*Primary Key: id*

| **Column**               | **Type**     | **Null** | **Default**       | **Description**                                          |
| ------------------------ | ------------ | -------- | ----------------- | -------------------------------------------------------- |
| id                       | UUID         | NOT NULL | uuidv7()          | PK                                                       |
| tipe\_eksposur           | VARCHAR(30)  | NOT NULL | —                 | SOVEREIGN/SENIOR\_SECURED/SENIOR\_UNSECURED/SUBORDINATED |
| lgd                      | NUMERIC(8,4) | NOT NULL | —                 | Default values: 0.4500/0.2500/0.4500/0.7500              |
| karakteristik            | TEXT         | NULL     | —                 | Deskripsi                                                |
| periode\_berlaku\_dari   | DATE         | NOT NULL | —                 |                                                          |
| periode\_berlaku\_sampai | DATE         | NULL     | —                 | NULL = current                                           |
| sumber                   | VARCHAR(50)  | NOT NULL | 'BASEL\_III\_IRB' |                                                          |
| dokumen\_pendukung\_id   | UUID         | NULL     | —                 | BCBS publication                                         |
| maker\_id                | UUID         | NOT NULL | —                 |                                                          |
| approver\_id             | UUID         | NULL     | —                 |                                                          |
| created\_at              | TIMESTAMPTZ  | NOT NULL | now()             |                                                          |
| approved\_at             | TIMESTAMPTZ  | NULL     | —                 |                                                          |

**Foreign Keys:**

  - dokumen\_pendukung\_id → doc.upload(id)

  - maker\_id → sec.user(id)

  - approver\_id → sec.user(id)

**Indexes:**

  - ix\_lgd\_tipe\_periode (tipe\_eksposur, periode\_berlaku\_dari DESC)

  - ix\_lgd\_current (tipe\_eksposur) WHERE periode\_berlaku\_sampai IS NULL

  - ck\_lgd\_range CHECK (lgd BETWEEN 0 AND 1)

### 4.6 mst.bobot\_skenario

Bobot probability-weighted untuk 3 skenario PD (Good/Normal/Bad). Default 0.25/0.50/0.25 (sum=1.00). Adjustable oleh ALCO via workflow approval.

*Primary Key: id*

| **Column**               | **Type**     | **Null** | **Default** | **Description**                 |
| ------------------------ | ------------ | -------- | ----------- | ------------------------------- |
| id                       | UUID         | NOT NULL | uuidv7()    | PK                              |
| skenario                 | VARCHAR(20)  | NOT NULL | —           | GOOD/NORMAL/BAD                 |
| bobot                    | NUMERIC(8,4) | NOT NULL | —           | 0.25/0.50/0.25 default          |
| periode\_berlaku\_dari   | DATE         | NOT NULL | —           |                                 |
| periode\_berlaku\_sampai | DATE         | NULL     | —           | NULL=current                    |
| catatan                  | TEXT         | NULL     | —           | Justifikasi perubahan dari ALCO |
| maker\_id                | UUID         | NOT NULL | —           | ALCO member                     |
| approver\_id             | UUID         | NULL     | —           | ALCO chair / CFO                |
| created\_at              | TIMESTAMPTZ  | NOT NULL | now()       |                                 |
| approved\_at             | TIMESTAMPTZ  | NULL     | —           |                                 |

**Foreign Keys:**

  - maker\_id → sec.user(id)

  - approver\_id → sec.user(id)

**Indexes:**

  - ix\_bobot\_skenario\_periode (skenario, periode\_berlaku\_dari DESC)

  - ix\_bobot\_current (skenario) WHERE periode\_berlaku\_sampai IS NULL

  - ck\_bobot\_range CHECK (bobot BETWEEN 0 AND 1)

**Notes:**

  - Validasi application-level: sum bobot 3 skenario aktif HARUS = 1.0000 (tolerance 0.0001).

  - Update bobot trigger re-eval ECL untuk periode aktif.

### 4.7 mst.impact\_mev\_pd

Multiplier forward-looking di tingkat INPUT — untuk derive PD Good (Optimistic) dan PD Bad (Pessimistic) dari PD Normal. Diturunkan dari proyeksi MEV oleh ALCO. Per periode evaluasi.

*Primary Key: id*

| **Column**             | **Type**     | **Null** | **Default** | **Description**                                               |
| ---------------------- | ------------ | -------- | ----------- | ------------------------------------------------------------- |
| id                     | UUID         | NOT NULL | uuidv7()    | PK                                                            |
| periode\_id            | UUID         | NOT NULL | —           | FK → mst.periode\_buku                                        |
| skenario               | VARCHAR(20)  | NOT NULL | —           | GOOD / BAD (Normal=1.0 implicit)                              |
| impact\_multiplier     | NUMERIC(8,4) | NOT NULL | —           | GOOD \< 1.0; BAD \> 1.0                                       |
| mev\_components\_json  | JSONB        | NULL     | —           | {gdp\_growth, inflasi, bi\_rate, usd\_idr, ihsg\_growth, ...} |
| catatan                | TEXT         | NULL     | —           | Justifikasi MEV projection                                    |
| dokumen\_pendukung\_id | UUID         | NULL     | —           |                                                               |
| maker\_id              | UUID         | NOT NULL | —           | Risk Officer / ALCO                                           |
| approver\_id           | UUID         | NULL     | —           | ALCO chair                                                    |
| created\_at            | TIMESTAMPTZ  | NOT NULL | now()       |                                                               |
| approved\_at           | TIMESTAMPTZ  | NULL     | —           |                                                               |

**Foreign Keys:**

  - periode\_id → mst.periode\_buku(id)

  - dokumen\_pendukung\_id → doc.upload(id)

  - maker\_id, approver\_id → sec.user(id)

**Indexes:**

  - uq\_impact\_mev\_periode\_skenario UNIQUE(periode\_id, skenario)

  - ix\_impact\_mev\_periode (periode\_id)

### 4.8 mst.impact\_pd

Multiplier forward-looking di tingkat OUTPUT — diterapkan ke ECL Weighted untuk derive ECL FL. Default 1.0000 (no overlay) sampai 1.1500 (standard FL adjustment).

*Primary Key: id*

| **Column**             | **Type**     | **Null** | **Default** | **Description**                 |
| ---------------------- | ------------ | -------- | ----------- | ------------------------------- |
| id                     | UUID         | NOT NULL | uuidv7()    | PK                              |
| periode\_id            | UUID         | NOT NULL | —           | FK → mst.periode\_buku          |
| impact\_multiplier     | NUMERIC(8,4) | NOT NULL | 1.0000      | Default 1.0; range \[0.9, 1.5\] |
| catatan                | TEXT         | NULL     | —           | Justifikasi                     |
| dokumen\_pendukung\_id | UUID         | NULL     | —           |                                 |
| maker\_id              | UUID         | NOT NULL | —           |                                 |
| approver\_id           | UUID         | NULL     | —           | CFO                             |
| created\_at            | TIMESTAMPTZ  | NOT NULL | now()       |                                 |
| approved\_at           | TIMESTAMPTZ  | NULL     | —           |                                 |

**Foreign Keys:**

  - periode\_id → mst.periode\_buku(id)

  - dokumen\_pendukung\_id → doc.upload(id)

**Indexes:**

  - uq\_impact\_pd\_periode UNIQUE(periode\_id)

  - ck\_impact\_pd\_range CHECK (impact\_multiplier BETWEEN 0.5 AND 2.0)

### 4.9 mst.lps\_coverage

Master nilai pertanggungan LPS (saat ini Rp 2 Miliar per nasabah per bank). Versioned table — bila regulasi LPS update, record baru di-insert.

*Primary Key: id*

| **Column**               | **Type**      | **Null** | **Default**   | **Description**               |
| ------------------------ | ------------- | -------- | ------------- | ----------------------------- |
| id                       | UUID          | NOT NULL | uuidv7()      | PK                            |
| coverage\_amount         | NUMERIC(20,2) | NOT NULL | 2000000000.00 | Rp 2 Miliar default           |
| mata\_uang               | CHAR(3)       | NOT NULL | 'IDR'         |                               |
| periode\_berlaku\_dari   | DATE          | NOT NULL | —             |                               |
| periode\_berlaku\_sampai | DATE          | NULL     | —             | NULL=current                  |
| regulasi\_referensi      | VARCHAR(200)  | NULL     | —             | Mis. POJK No. 03/POJK.05/2017 |
| dokumen\_pendukung\_id   | UUID          | NULL     | —             |                               |
| maker\_id                | UUID          | NOT NULL | —             | CFO                           |
| approver\_id             | UUID          | NULL     | —             |                               |
| created\_at              | TIMESTAMPTZ   | NOT NULL | now()         |                               |

**Foreign Keys:**

  - dokumen\_pendukung\_id → doc.upload(id)

**Indexes:**

  - ix\_lps\_current () WHERE periode\_berlaku\_sampai IS NULL

### 4.10 mst.instrumen

CORE ENTITY — Master Instrumen Investasi. Setiap aset keuangan yang dimiliki Tugure tercatat di sini dengan klasifikasi PSAK 71 yang ter-lock setelah approval Komite Investasi. Menjadi parent untuk seluruh entitas trx, ecl, sppi, doc terkait. Untuk full field specs lihat FSD Appendix A §1.1.3.

*Primary Key: id*

| **Column**                    | **Type**      | **Null** | **Default** | **Description**                                                       |
| ----------------------------- | ------------- | -------- | ----------- | --------------------------------------------------------------------- |
| id                            | UUID          | NOT NULL | uuidv7()    | PK                                                                    |
| kode\_instrumen               | VARCHAR(20)   | NOT NULL | —           | Auto: {TIPE}-{YYYY}-{\#\#\#\#\#}                                      |
| tipe\_instrumen               | VARCHAR(30)   | NOT NULL | —           | CASH/DEPOSITO/OBLIGASI/SAHAM/REKSADANA                                |
| sub\_tipe                     | VARCHAR(50)   | NOT NULL | —           | Sub-classification                                                    |
| nama                          | VARCHAR(200)  | NOT NULL | —           |                                                                       |
| isin                          | VARCHAR(20)   | NULL     | —           | Untuk obligasi/saham/RDN                                              |
| counterparty\_id              | UUID          | NOT NULL | —           | Issuer/Bank                                                           |
| manajer\_investasi\_id        | UUID          | NULL     | —           | Wajib untuk REKSADANA                                                 |
| bank\_kustodian\_id           | UUID          | NULL     | —           | Wajib untuk REKSADANA                                                 |
| mata\_uang                    | CHAR(3)       | NOT NULL | 'IDR'       | ISO 4217                                                              |
| nominal                       | NUMERIC(20,2) | NOT NULL | —           |                                                                       |
| jumlah\_lot                   | NUMERIC(18,0) | NULL     | —           | Khusus saham                                                          |
| tanggal\_penempatan           | DATE          | NOT NULL | —           |                                                                       |
| tanggal\_jatuh\_tempo         | DATE          | NULL     | —           | Wajib untuk DEP/OBL                                                   |
| kupon                         | NUMERIC(8,4)  | NULL     | —           | % pa, 4 desimal                                                       |
| frekuensi\_bunga              | VARCHAR(20)   | NULL     | —           | BULANAN/TRIWULANAN/SEMESTERAN/TAHUNAN/DI\_MUKA/JATUH\_TEMPO           |
| auto\_renewal\_flag           | BOOLEAN       | NULL     | FALSE       | Hanya DEPOSITO                                                        |
| fvoci\_election               | BOOLEAN       | NULL     | FALSE       | Hanya SAHAM                                                           |
| sppi\_result                  | VARCHAR(10)   | NULL     | —           | PASS/FAIL                                                             |
| bm\_category                  | VARCHAR(10)   | NULL     | —           | HTC/HTCS/OTHER                                                        |
| klasifikasi\_psak71           | VARCHAR(20)   | NULL     | —           | AC/FVOCI/FVOCI\_ELECTION/FVTPL                                        |
| klasifikasi\_locked\_at       | TIMESTAMPTZ   | NULL     | —           |                                                                       |
| klasifikasi\_locked\_by       | UUID          | NULL     | —           | Komite Investasi                                                      |
| sppi\_bm\_last\_review\_date  | DATE          | NULL     | —           | Annual review                                                         |
| eir\_awal                     | NUMERIC(12,8) | NULL     | —           | Untuk AC/FVOCI utang; 8 desimal                                       |
| tanggal\_eir\_computed        | DATE          | NULL     | —           |                                                                       |
| premium\_diskonto\_awal       | NUMERIC(20,2) | NULL     | 0           |                                                                       |
| biaya\_transaksi\_capitalized | NUMERIC(20,2) | NULL     | 0           |                                                                       |
| eir\_method\_flag             | BOOLEAN       | NULL     | TRUE        |                                                                       |
| day\_count\_convention        | VARCHAR(10)   | NULL     | 'ACT/365'   |                                                                       |
| amortization\_frequency       | VARCHAR(20)   | NULL     | —           |                                                                       |
| status                        | VARCHAR(30)   | NOT NULL | 'AKTIF'     | AKTIF/DICAIRKAN/JATUH\_TEMPO/DIJUAL/REKLASIFIKASI                     |
| portofolio\_id                | UUID          | NOT NULL | —           |                                                                       |
| workflow\_status              | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                                                                       |
| {audit\_columns}              | —             | —        | —           | created\_by/at, updated\_by/at, approved\_by/at, version, is\_deleted |

**Foreign Keys:**

  - counterparty\_id → mst.counterparty(id) ON DELETE RESTRICT

  - manajer\_investasi\_id → mst.counterparty(id) (where tipe='MANAJER\_INVESTASI')

  - bank\_kustodian\_id → mst.counterparty(id) (where tipe='BANK\_KUSTODIAN')

  - portofolio\_id → mst.portofolio(id) ON DELETE RESTRICT

  - klasifikasi\_locked\_by → sec.user(id)

  - audit FKs → sec.user(id)

**Indexes:**

  - uq\_instrumen\_kode UNIQUE(kode\_instrumen) WHERE is\_deleted=FALSE

  - ix\_instrumen\_tipe (tipe\_instrumen) WHERE is\_deleted=FALSE

  - ix\_instrumen\_klasifikasi (klasifikasi\_psak71) WHERE is\_deleted=FALSE

  - ix\_instrumen\_counterparty (counterparty\_id) WHERE is\_deleted=FALSE

  - ix\_instrumen\_status (status) WHERE is\_deleted=FALSE

  - ix\_instrumen\_isin (isin) WHERE isin IS NOT NULL AND is\_deleted=FALSE

  - ix\_instrumen\_portofolio (portofolio\_id, status) WHERE is\_deleted=FALSE

  - ix\_instrumen\_jt (tanggal\_jatuh\_tempo) WHERE status='AKTIF' AND tanggal\_jatuh\_tempo IS NOT NULL

**Notes:**

  - Setelah klasifikasi\_psak71 ter-set + klasifikasi\_locked\_at filled: field tidak dapat diubah (enforcement via trigger BEFORE UPDATE).

  - Trigger auto-compute kode\_instrumen via sequence saat INSERT.

  - Soft delete only — instrumen dengan transaksi aktif tidak dapat di-delete (FK RESTRICT mencegah).

### 4.11 mst.periode\_buku

Master Periode Buku — periode akuntansi (bulanan/triwulanan/tahunan) dengan 3-state machine. Lihat FSD Appendix D §1 untuk detail spec workflow.

*Primary Key: id*

| **Column**                | **Type**    | **Null** | **Default** | **Description**                     |
| ------------------------- | ----------- | -------- | ----------- | ----------------------------------- |
| id                        | UUID        | NOT NULL | uuidv7()    | PK                                  |
| periode\_id\_kode         | VARCHAR(20) | NOT NULL | —           | PRD-YYYY-MM, PRD-YYYY-Q\#, PRD-YYYY |
| tipe\_periode             | VARCHAR(20) | NOT NULL | —           | BULANAN/TRIWULANAN/TAHUNAN          |
| tahun\_buku               | INT         | NOT NULL | —           |                                     |
| bulan                     | INT         | NULL     | —           | 1-12; null kecuali BULANAN          |
| triwulan                  | INT         | NULL     | —           | 1-4; null kecuali TRIWULANAN        |
| tanggal\_mulai            | DATE        | NOT NULL | —           |                                     |
| tanggal\_akhir            | DATE        | NOT NULL | —           |                                     |
| status\_periode           | VARCHAR(20) | NOT NULL | 'OPEN'      | OPEN/SOFT\_CLOSED/CLOSED            |
| tanggal\_soft\_close      | TIMESTAMPTZ | NULL     | —           |                                     |
| tanggal\_hard\_close      | TIMESTAMPTZ | NULL     | —           |                                     |
| user\_closer\_id          | UUID        | NULL     | —           | Akuntansi                           |
| user\_approver\_close\_id | UUID        | NULL     | —           | FC (soft) atau CFO (hard)           |
| catatan\_closing          | TEXT        | NULL     | —           |                                     |
| reopened\_flag            | BOOLEAN     | NOT NULL | FALSE       |                                     |
| reopened\_reason          | TEXT        | NULL     | —           | Wajib jika reopened\_flag=TRUE      |
| reopened\_at              | TIMESTAMPTZ | NULL     | —           |                                     |
| reopened\_by              | UUID        | NULL     | —           | CFO                                 |
| reopened\_approved\_by    | UUID        | NULL     | —           | CEO atau Komite Audit               |
| created\_at               | TIMESTAMPTZ | NOT NULL | now()       |                                     |
| updated\_at               | TIMESTAMPTZ | NULL     | —           |                                     |

**Foreign Keys:**

  - user\_closer\_id, user\_approver\_close\_id, reopened\_by, reopened\_approved\_by → sec.user(id)

**Indexes:**

  - uq\_periode\_kode UNIQUE(periode\_id\_kode)

  - ix\_periode\_tahun\_bulan (tahun\_buku, bulan) WHERE tipe\_periode='BULANAN'

  - ix\_periode\_status (status\_periode)

  - ix\_periode\_tanggal (tanggal\_mulai, tanggal\_akhir)

  - ck\_periode\_status CHECK (status\_periode IN ('OPEN','SOFT\_CLOSED','CLOSED'))

  - ck\_periode\_tipe CHECK (tipe\_periode IN ('BULANAN','TRIWULANAN','TAHUNAN'))

**Notes:**

  - Auto-generate 12 periode bulanan + 4 triwulanan + 1 tahunan setiap awal Tahun Buku via stored procedure sp\_periode\_init.

  - Setiap tanggal kalender HARUS mapped ke tepat 1 periode bulanan (enforced via daterange exclusion constraint).

  - Hard-close periode tahunan WAJIB pre-condition: 12 bulanan + 4 triwulanan CLOSED.

### 4.12 mst.mata\_uang

Master Mata Uang aktif yang dipakai sistem. Static reference; jarang berubah.

*Primary Key: kode\_mata\_uang (CHAR(3)) — Natural PK*

| **Column**            | **Type**    | **Null** | **Default** | **Description**                      |
| --------------------- | ----------- | -------- | ----------- | ------------------------------------ |
| kode\_mata\_uang      | CHAR(3)     | NOT NULL | —           | PK; ISO 4217 (IDR, USD, SGD, dll)    |
| nama\_mata\_uang      | VARCHAR(60) | NOT NULL | —           |                                      |
| simbol                | VARCHAR(5)  | NULL     | —           | Rp, $, S$, €                         |
| sumber\_kurs\_default | VARCHAR(30) | NOT NULL | —           | BI\_JISDOR/BI\_KURS\_TENGAH/INTERNAL |
| frekuensi\_update     | VARCHAR(20) | NOT NULL | —           | HARIAN/INTRA\_DAY/BULANAN            |
| aktif\_flag           | BOOLEAN     | NOT NULL | TRUE        |                                      |
| tanggal\_mulai\_aktif | DATE        | NOT NULL | —           |                                      |
| created\_at           | TIMESTAMPTZ | NOT NULL | now()       |                                      |

### 4.13 mst.kurs

FX Rate History — kurs harian per mata uang. Auto-update dari BI JISDOR scheduled job hari kerja jam 10:30 WIB. Setelah periode bulanan terkait HARD CLOSED, kurs ter-locked dan tidak dapat diubah.

*Primary Key: id*

| **Column**           | **Type**      | **Null** | **Default** | **Description**                                         |
| -------------------- | ------------- | -------- | ----------- | ------------------------------------------------------- |
| id                   | UUID          | NOT NULL | uuidv7()    | PK                                                      |
| fx\_rate\_id\_kode   | VARCHAR(20)   | NOT NULL | —           | FX-{ccy}-{YYYYMMDD}                                     |
| kode\_mata\_uang     | CHAR(3)       | NOT NULL | —           | FK → mst.mata\_uang                                     |
| tanggal\_berlaku     | DATE          | NOT NULL | —           |                                                         |
| kurs\_beli           | NUMERIC(15,4) | NULL     | —           | Bid                                                     |
| kurs\_jual           | NUMERIC(15,4) | NULL     | —           | Ask                                                     |
| kurs\_tengah         | NUMERIC(15,4) | NOT NULL | —           | Mid — DIPAKAI UNTUK PEMBUKUAN                           |
| sumber\_kurs         | VARCHAR(30)   | NOT NULL | —           | BI\_JISDOR/BI\_KURS\_TENGAH/UPLOAD\_MANUAL/REPEAT\_RATE |
| periode\_bulanan\_id | UUID          | NOT NULL | —           | FK → mst.periode\_buku                                  |
| locked\_flag         | BOOLEAN       | NOT NULL | FALSE       | Auto Y saat periode CLOSED                              |
| maker\_id            | UUID          | NULL     | —           | Wajib untuk UPLOAD\_MANUAL                              |
| approver\_id         | UUID          | NULL     | —           |                                                         |
| dokumen\_bukti\_id   | UUID          | NULL     | —           | Wajib untuk UPLOAD\_MANUAL                              |
| created\_at          | TIMESTAMPTZ   | NOT NULL | now()       |                                                         |
| approved\_at         | TIMESTAMPTZ   | NULL     | —           |                                                         |

**Foreign Keys:**

  - kode\_mata\_uang → mst.mata\_uang(kode\_mata\_uang)

  - periode\_bulanan\_id → mst.periode\_buku(id)

  - dokumen\_bukti\_id → doc.upload(id)

  - maker\_id, approver\_id → sec.user(id)

**Indexes:**

  - uq\_kurs\_mata\_uang\_tanggal UNIQUE(kode\_mata\_uang, tanggal\_berlaku)

  - ix\_kurs\_tanggal (tanggal\_berlaku DESC)

  - ix\_kurs\_periode (periode\_bulanan\_id)

  - ix\_kurs\_lookup (kode\_mata\_uang, tanggal\_berlaku DESC) — frequent lookup

**Notes:**

  - Trigger BEFORE UPDATE/DELETE: block jika locked\_flag=TRUE.

  - Trigger setelah periode CLOSED: auto-set locked\_flag untuk semua kurs di periode tersebut.

  - Untuk reasonability check: deviasi kurs \> 3% vs hari sebelumnya → flag REPEAT\_RATE\_REVIEW (alert ke Akuntansi).

### 4.14 mst.chart\_of\_accounts

Master CoA — struktur kode akun GL. Self-referential untuk hierarki parent-child. Dapat di-import dari sistem ERP/GL existing via Excel atau API.

*Primary Key: id*

| **Column**            | **Type**     | **Null** | **Default** | **Description**                                    |
| --------------------- | ------------ | -------- | ----------- | -------------------------------------------------- |
| id                    | UUID         | NOT NULL | uuidv7()    | PK                                                 |
| kode\_akun            | VARCHAR(20)  | NOT NULL | —           | Format struktur ERP (1.1.2.001)                    |
| nama\_akun            | VARCHAR(200) | NOT NULL | —           |                                                    |
| tipe\_akun            | VARCHAR(20)  | NOT NULL | —           | ASET/LIABILITAS/EKUITAS/PENDAPATAN/BEBAN/KONTINJEN |
| sub\_tipe\_akun       | VARCHAR(30)  | NOT NULL | —           | LANCAR/TIDAK\_LANCAR/JANGKA\_PENDEK/dll            |
| kategori\_investasi   | VARCHAR(20)  | NULL     | —           | AC/FVOCI/FVTPL/OCI\_FVOCI/CKPN                     |
| mata\_uang\_native    | CHAR(3)      | NOT NULL | 'IDR'       |                                                    |
| posisi\_normal        | VARCHAR(10)  | NOT NULL | —           | DEBIT/KREDIT                                       |
| aktif\_flag           | BOOLEAN      | NOT NULL | TRUE        |                                                    |
| parent\_akun\_id      | UUID         | NULL     | —           | FK self-ref untuk hierarki                         |
| sumber\_coa           | VARCHAR(30)  | NOT NULL | —           | INTERNAL/IMPORT\_ERP/IMPORT\_EXCEL                 |
| tanggal\_mulai\_aktif | DATE         | NOT NULL | —           |                                                    |
| created\_by           | UUID         | NOT NULL | —           |                                                    |
| created\_at           | TIMESTAMPTZ  | NOT NULL | now()       |                                                    |
| updated\_by           | UUID         | NULL     | —           |                                                    |
| updated\_at           | TIMESTAMPTZ  | NULL     | —           |                                                    |
| version               | INT          | NOT NULL | 1           |                                                    |

**Foreign Keys:**

  - parent\_akun\_id → mst.chart\_of\_accounts(id) (self-ref)

  - created\_by, updated\_by → sec.user(id)

**Indexes:**

  - uq\_coa\_kode UNIQUE(kode\_akun)

  - ix\_coa\_tipe (tipe\_akun)

  - ix\_coa\_kategori (kategori\_investasi) WHERE kategori\_investasi IS NOT NULL

  - ix\_coa\_parent (parent\_akun\_id) WHERE parent\_akun\_id IS NOT NULL

  - ix\_coa\_aktif (aktif\_flag) WHERE aktif\_flag=TRUE

### 4.15 mst.mapping\_jurnal\_header

Header template event jurnal akuntansi. Setiap event (PENEMPATAN, AKRUAL\_BUNGA, ECL\_PEMBENTUKAN, dll) memiliki satu header dengan multiple detail lines (D/K).

*Primary Key: id*

| **Column**               | **Type**        | **Null** | **Default** | **Description**                        |
| ------------------------ | --------------- | -------- | ----------- | -------------------------------------- |
| id                       | UUID            | NOT NULL | uuidv7()    | PK                                     |
| event\_id\_kode          | VARCHAR(40)     | NOT NULL | —           | EVT-PENEMPATAN, EVT-AKRUAL\_BUNGA, dll |
| event\_code              | VARCHAR(40)     | NOT NULL | —           | ENUM event                             |
| nama\_event              | VARCHAR(120)    | NOT NULL | —           |                                        |
| kategori\_event          | VARCHAR(30)     | NOT NULL | —           | PENEMPATAN/AKRUAL/MUTASI\_MTM/dll      |
| trigger\_source          | VARCHAR(20)     | NOT NULL | —           | USER\_INPUT/SYSTEM\_JOB/UPLOAD         |
| tipe\_instrumen\_berlaku | VARCHAR(50)\[\] | NULL     | NULL        | Filter array; NULL=semua               |
| klasifikasi\_berlaku     | VARCHAR(20)\[\] | NULL     | NULL        | Filter array                           |
| aktif\_flag              | BOOLEAN         | NOT NULL | TRUE        |                                        |
| catatan                  | TEXT            | NULL     | —           | Reference PSAK                         |
| created\_by              | UUID            | NOT NULL | —           |                                        |
| created\_at              | TIMESTAMPTZ     | NOT NULL | now()       |                                        |
| updated\_by              | UUID            | NULL     | —           |                                        |
| updated\_at              | TIMESTAMPTZ     | NULL     | —           |                                        |

**Foreign Keys:**

  - created\_by, updated\_by → sec.user(id)

**Indexes:**

  - uq\_mapping\_event\_code UNIQUE(event\_code)

  - uq\_mapping\_event\_id\_kode UNIQUE(event\_id\_kode)

  - ix\_mapping\_event\_aktif (event\_code) WHERE aktif\_flag=TRUE

### 4.16 mst.mapping\_jurnal\_detail

Detail line per event header — satu atau lebih baris D/K dengan filter klasifikasi/tipe/underlying. Resolusi runtime saat event terpicu.

*Primary Key: id*

| **Column**               | **Type**        | **Null** | **Default** | **Description**                                  |
| ------------------------ | --------------- | -------- | ----------- | ------------------------------------------------ |
| id                       | UUID            | NOT NULL | uuidv7()    | PK                                               |
| event\_header\_id        | UUID            | NOT NULL | —           | FK → mapping\_jurnal\_header                     |
| urutan                   | INT             | NOT NULL | —           | 1, 2, 3, ...                                     |
| kode\_akun\_id           | UUID            | NOT NULL | —           | FK → chart\_of\_accounts                         |
| dk\_indicator            | VARCHAR(10)     | NOT NULL | —           | DEBIT/KREDIT                                     |
| sumber\_amount           | VARCHAR(50)     | NOT NULL | —           | EAD\_IDR/BUNGA\_AKRUAL\_IDR/ECL\_AMOUNT\_IDR/dll |
| klasifikasi\_filter      | VARCHAR(20)     | NULL     | —           | AC/FVOCI/FVTPL; NULL=semua                       |
| tipe\_instrumen\_filter  | VARCHAR(50)\[\] | NULL     | —           | Filter array                                     |
| underlying\_type\_filter | VARCHAR(20)     | NULL     | —           | NON\_EQUITY/EQUITY (untuk RDN look-through)      |
| multiplier               | NUMERIC(8,4)    | NOT NULL | 1.0000      | Mis. -0.10 untuk PPh 10%                         |
| mata\_uang\_posting      | CHAR(3)         | NOT NULL | 'IDR'       |                                                  |
| aktif\_flag              | BOOLEAN         | NOT NULL | TRUE        |                                                  |
| catatan                  | TEXT            | NULL     | —           |                                                  |
| created\_at              | TIMESTAMPTZ     | NOT NULL | now()       |                                                  |
| updated\_at              | TIMESTAMPTZ     | NULL     | —           |                                                  |

**Foreign Keys:**

  - event\_header\_id → mst.mapping\_jurnal\_header(id) ON DELETE CASCADE

  - kode\_akun\_id → mst.chart\_of\_accounts(id)

**Indexes:**

  - ix\_mapping\_detail\_event (event\_header\_id, urutan)

  - ix\_mapping\_detail\_aktif (event\_header\_id) WHERE aktif\_flag=TRUE

  - ix\_mapping\_detail\_akun (kode\_akun\_id)

**Notes:**

  - ON DELETE CASCADE — bila header di-deactivate, semua detail ikut.

  - Multiple line dengan urutan sama dan filter klasifikasi berbeda — runtime resolver pilih sesuai klasifikasi instrumen.

# 5\. Schema trx — Transactional Data (7 Entities)

Schema trx menyimpan event transaksi sepanjang lifecycle instrumen. Setiap event memiliki periode\_id stamp, FK ke instrumen, dan dapat memicu posting jurnal otomatis.

## 5.0 Schema trx — ER Diagram (Mermaid)

> \`\`\`mermaid  
> erDiagram  
> mst\_instrumen ||--o{ trx\_penempatan : "ditempatkan"  
> mst\_instrumen ||--o{ trx\_mtm : "MTM harian"  
> mst\_instrumen ||--o{ trx\_renewal : "renewal (deposito)"  
> mst\_instrumen ||--o{ trx\_penjualan : "dijual"  
> mst\_instrumen ||--o{ trx\_jatuh\_tempo : "jatuh tempo"  
> mst\_instrumen ||--o{ trx\_pendapatan\_akrual : "akrual harian"  
> mst\_instrumen ||--o{ trx\_amortisasi : "amortisasi P/D"  
> mst\_periode\_buku ||--o{ trx\_penempatan : "stamp periode"  
> mst\_periode\_buku ||--o{ trx\_mtm : "stamp"  
> mst\_periode\_buku ||--o{ trx\_pendapatan\_akrual : "stamp"  
> trx\_penempatan ||--o| jrnl\_header : "trigger jurnal"  
> trx\_mtm ||--o| jrnl\_header : "trigger jurnal"  
> \`\`\`

### 5.1 trx.penempatan

Pencatatan transaksi pembelian/penempatan instrumen baru. Trigger downstream: EIR computation, generate amortization schedule, posting jurnal PENEMPATAN. Lihat FSD Appendix B §1 untuk full spec.

*Primary Key: id*

| **Column**                | **Type**      | **Null** | **Default** | **Description**                  |
| ------------------------- | ------------- | -------- | ----------- | -------------------------------- |
| id                        | UUID          | NOT NULL | uuidv7()    | PK                               |
| no\_transaksi             | VARCHAR(20)   | NOT NULL | —           | Auto: PNP-YYYY-\#\#\#\#\#        |
| tanggal\_transaksi        | DATE          | NOT NULL | —           | Trade date                       |
| tanggal\_settlement       | DATE          | NOT NULL | —           | Value date                       |
| instrumen\_id             | UUID          | NOT NULL | —           | FK                               |
| periode\_id               | UUID          | NOT NULL | —           | FK; auto dari tanggal\_transaksi |
| nominal                   | NUMERIC(20,2) | NOT NULL | —           |                                  |
| harga\_beli               | NUMERIC(15,4) | NULL     | —           | % atau NAB; null untuk DEPOSITO  |
| jumlah\_unit              | NUMERIC(18,4) | NULL     | —           | Khusus reksadana                 |
| accrued\_interest\_dibeli | NUMERIC(20,2) | NULL     | 0           | Khusus obligasi inter-coupon     |
| total\_pembayaran         | NUMERIC(20,2) | NOT NULL | —           | Native currency                  |
| biaya\_transaksi          | NUMERIC(20,2) | NULL     | 0           |                                  |
| akun\_sumber\_dana\_id    | UUID          | NOT NULL | —           | FK ke instrumen Cash             |
| mata\_uang                | CHAR(3)       | NOT NULL | —           | Inherit dari instrumen           |
| kurs\_tengah\_bi          | NUMERIC(15,4) | NULL     | —           | Untuk valas                      |
| total\_pembayaran\_idr    | NUMERIC(20,2) | NOT NULL | —           | IDR equivalent                   |
| eir\_awal                 | NUMERIC(12,8) | NULL     | —           | Computed untuk AC/FVOCI utang    |
| carrying\_amount\_awal    | NUMERIC(20,2) | NULL     | —           | P0 = harga + biaya               |
| maker\_id                 | UUID          | NOT NULL | —           |                                  |
| approver\_id              | UUID          | NULL     | —           |                                  |
| workflow\_status          | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                                  |
| jurnal\_header\_id        | UUID          | NULL     | —           | FK setelah posting               |
| created\_at               | TIMESTAMPTZ   | NOT NULL | now()       |                                  |
| approved\_at              | TIMESTAMPTZ   | NULL     | —           |                                  |
| is\_deleted               | BOOLEAN       | NOT NULL | FALSE       |                                  |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

  - akun\_sumber\_dana\_id → mst.instrumen(id) (where tipe='CASH')

  - jurnal\_header\_id → jrnl.header(id)

  - maker\_id, approver\_id → sec.user(id)

**Indexes:**

  - uq\_penempatan\_no UNIQUE(no\_transaksi)

  - ix\_penempatan\_instrumen (instrumen\_id)

  - ix\_penempatan\_periode (periode\_id)

  - ix\_penempatan\_tanggal (tanggal\_transaksi)

  - ix\_penempatan\_status (workflow\_status) WHERE is\_deleted=FALSE

  - ck\_settlement\_after\_trade CHECK (tanggal\_settlement \>= tanggal\_transaksi)

  - ck\_total\_positive CHECK (total\_pembayaran \> 0)

### 5.2 trx.mtm

MTM (Mark-to-Market) harian — revaluation instrumen ke fair value berdasarkan IBPA/NAB/BEI. Job batch end-of-day pada hari kerja.

*Primary Key: id*

| **Column**                   | **Type**      | **Null** | **Default** | **Description**                      |
| ---------------------------- | ------------- | -------- | ----------- | ------------------------------------ |
| id                           | UUID          | NOT NULL | uuidv7()    | PK                                   |
| instrumen\_id                | UUID          | NOT NULL | —           | FK                                   |
| tanggal\_valuasi             | DATE          | NOT NULL | —           | Hari kerja                           |
| periode\_id                  | UUID          | NOT NULL | —           | FK                                   |
| carrying\_amount\_sebelumnya | NUMERIC(20,2) | NOT NULL | —           |                                      |
| harga\_referensi\_baru       | NUMERIC(15,4) | NOT NULL | —           |                                      |
| fair\_value\_baru            | NUMERIC(20,2) | NOT NULL | —           | Computed: harga × nominal/unit       |
| selisih\_mtm\_native         | NUMERIC(20,2) | NOT NULL | —           | Mata uang asli                       |
| selisih\_mtm\_idr            | NUMERIC(20,2) | NOT NULL | —           | IDR equivalent                       |
| kurs\_tengah\_bi             | NUMERIC(15,4) | NULL     | —           | Untuk valas                          |
| akun\_pengakuan              | VARCHAR(20)   | NOT NULL | —           | OCI/LABA\_RUGI/NONE (AC monitoring)  |
| sumber\_harga                | VARCHAR(30)   | NOT NULL | —           | IBPA/NAB\_MI/BEI/MANUAL              |
| dokumen\_sumber\_id          | UUID          | NULL     | —           | FK doc.upload                        |
| jurnal\_header\_id           | UUID          | NULL     | —           | Reference                            |
| status\_flag                 | VARCHAR(20)   | NOT NULL | 'POSTED'    | POSTED/STALE\_PRICE/MANUAL\_OVERRIDE |
| created\_at                  | TIMESTAMPTZ   | NOT NULL | now()       |                                      |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

  - dokumen\_sumber\_id → doc.upload(id)

  - jurnal\_header\_id → jrnl.header(id)

**Indexes:**

  - uq\_mtm\_instrumen\_tanggal UNIQUE(instrumen\_id, tanggal\_valuasi)

  - ix\_mtm\_periode (periode\_id)

  - ix\_mtm\_tanggal (tanggal\_valuasi)

  - ix\_mtm\_stale (status\_flag) WHERE status\_flag='STALE\_PRICE'

**Partitioning: RANGE (tanggal\_valuasi) — yearly partitions**

### 5.3 trx.renewal

Renewal deposito jatuh tempo — 2 skema: POKOK\_SAJA atau POKOK\_PLUS\_BUNGA.

*Primary Key: id*

| **Column**              | **Type**      | **Null** | **Default** | **Description**                |
| ----------------------- | ------------- | -------- | ----------- | ------------------------------ |
| id                      | UUID          | NOT NULL | uuidv7()    | PK                             |
| no\_renewal             | VARCHAR(20)   | NOT NULL | —           | RNW-YYYY-\#\#\#\#\#            |
| instrumen\_lama\_id     | UUID          | NOT NULL | —           | FK                             |
| instrumen\_baru\_id     | UUID          | NOT NULL | —           | Auto-create master baru        |
| tanggal\_jt\_lama       | DATE          | NOT NULL | —           | Auto                           |
| skema\_renewal          | VARCHAR(30)   | NOT NULL | —           | POKOK\_SAJA/POKOK\_PLUS\_BUNGA |
| tenor\_baru\_hari       | INT           | NULL     | —           | Untuk on-call                  |
| tenor\_baru\_bulan      | INT           | NULL     | —           | Untuk berjangka                |
| suku\_bunga\_baru       | NUMERIC(8,4)  | NOT NULL | —           | % pa                           |
| pokok\_lama             | NUMERIC(20,2) | NOT NULL | —           |                                |
| bunga\_akrual\_terakhir | NUMERIC(20,2) | NOT NULL | —           |                                |
| pph\_bunga              | NUMERIC(20,2) | NOT NULL | —           | \= bunga × 20%                 |
| bunga\_net              | NUMERIC(20,2) | NOT NULL | —           |                                |
| pokok\_baru             | NUMERIC(20,2) | NOT NULL | —           | Auto per skema                 |
| dokumen\_bukti\_id      | UUID          | NULL     | —           | FK doc.upload                  |
| maker\_id               | UUID          | NOT NULL | —           |                                |
| approver\_id            | UUID          | NULL     | —           |                                |
| workflow\_status        | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                                |
| jurnal\_header\_id      | UUID          | NULL     | —           |                                |
| created\_at             | TIMESTAMPTZ   | NOT NULL | now()       |                                |
| approved\_at            | TIMESTAMPTZ   | NULL     | —           |                                |

**Foreign Keys:**

  - instrumen\_lama\_id, instrumen\_baru\_id → mst.instrumen(id)

  - dokumen\_bukti\_id → doc.upload(id)

  - maker\_id, approver\_id → sec.user(id)

**Indexes:**

  - uq\_renewal\_no UNIQUE(no\_renewal)

  - ix\_renewal\_lama (instrumen\_lama\_id)

  - ix\_renewal\_baru (instrumen\_baru\_id)

### 5.4 trx.penjualan

Penjualan/pencairan instrumen sebelum atau pada tanggal jatuh tempo. Untuk FVOCI utang trigger REKLAS\_OCI\_PL (recycling akumulasi OCI ke P\&L).

*Primary Key: id*

| **Column**                   | **Type**      | **Null** | **Default** | **Description**     |
| ---------------------------- | ------------- | -------- | ----------- | ------------------- |
| id                           | UUID          | NOT NULL | uuidv7()    | PK                  |
| no\_penjualan                | VARCHAR(20)   | NOT NULL | —           | JUL-YYYY-\#\#\#\#\# |
| instrumen\_id                | UUID          | NOT NULL | —           | FK                  |
| periode\_id                  | UUID          | NOT NULL | —           | FK                  |
| tanggal\_penjualan           | DATE          | NOT NULL | —           |                     |
| tanggal\_settlement          | DATE          | NOT NULL | —           |                     |
| nominal\_unit\_dijual        | NUMERIC(20,4) | NOT NULL | —           | Untuk parsial       |
| harga\_jual                  | NUMERIC(15,4) | NOT NULL | —           | % atau NAB          |
| accrued\_interest\_dijual    | NUMERIC(20,2) | NULL     | 0           |                     |
| total\_penerimaan            | NUMERIC(20,2) | NOT NULL | —           |                     |
| biaya\_transaksi             | NUMERIC(20,2) | NULL     | 0           |                     |
| carrying\_amount\_saat\_jual | NUMERIC(20,2) | NOT NULL | —           | Auto                |
| realized\_gain\_loss         | NUMERIC(20,2) | NOT NULL | —           | Auto                |
| realized\_oci\_recycled      | NUMERIC(20,2) | NULL     | —           | Untuk FVOCI utang   |
| dijual\_penuh\_flag          | BOOLEAN       | NOT NULL | —           |                     |
| dokumen\_bukti\_id           | UUID          | NULL     | —           |                     |
| maker\_id                    | UUID          | NOT NULL | —           |                     |
| approver\_id                 | UUID          | NULL     | —           |                     |
| workflow\_status             | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                     |
| jurnal\_header\_id           | UUID          | NULL     | —           |                     |
| created\_at                  | TIMESTAMPTZ   | NOT NULL | now()       |                     |
| approved\_at                 | TIMESTAMPTZ   | NULL     | —           |                     |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

**Indexes:**

  - uq\_penjualan\_no UNIQUE(no\_penjualan)

  - ix\_penjualan\_instrumen (instrumen\_id)

  - ix\_penjualan\_periode (periode\_id)

### 5.5 trx.jatuh\_tempo

Closure pada tanggal jatuh tempo untuk deposito & obligasi non-auto-rollover. Settlement otomatis pokok + kupon final.

*Primary Key: id*

| **Column**              | **Type**      | **Null** | **Default** | **Description**    |
| ----------------------- | ------------- | -------- | ----------- | ------------------ |
| id                      | UUID          | NOT NULL | uuidv7()    | PK                 |
| no\_jatuh\_tempo        | VARCHAR(20)   | NOT NULL | —           | JT-YYYY-\#\#\#\#\# |
| instrumen\_id           | UUID          | NOT NULL | —           | FK                 |
| periode\_id             | UUID          | NOT NULL | —           | FK                 |
| tanggal\_jt             | DATE          | NOT NULL | —           |                    |
| pokok\_diterima         | NUMERIC(20,2) | NOT NULL | —           |                    |
| kupon\_final            | NUMERIC(20,2) | NULL     | 0           |                    |
| pph\_kupon              | NUMERIC(20,2) | NULL     | 0           |                    |
| total\_diterima         | NUMERIC(20,2) | NOT NULL | —           |                    |
| realized\_oci\_recycled | NUMERIC(20,2) | NULL     | —           | Untuk FVOCI utang  |
| dokumen\_bukti\_id      | UUID          | NULL     | —           |                    |
| jurnal\_header\_id      | UUID          | NULL     | —           |                    |
| status                  | VARCHAR(20)   | NOT NULL | 'COMPLETED' | COMPLETED/FAILED   |
| created\_at             | TIMESTAMPTZ   | NOT NULL | now()       |                    |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

**Indexes:**

  - uq\_jt\_no UNIQUE(no\_jatuh\_tempo)

  - ix\_jt\_instrumen (instrumen\_id)

  - ix\_jt\_tanggal (tanggal\_jt)

### 5.6 trx.pendapatan\_akrual

Akrual harian bunga/kupon — record per instrumen per hari. Hi-volume table; partitioned by tanggal\_akrual yearly.

*Primary Key: id*

| **Column**                     | **Type**      | **Null** | **Default** | **Description**          |
| ------------------------------ | ------------- | -------- | ----------- | ------------------------ |
| id                             | UUID          | NOT NULL | uuidv7()    | PK                       |
| instrumen\_id                  | UUID          | NOT NULL | —           | FK                       |
| tanggal\_akrual                | DATE          | NOT NULL | —           |                          |
| periode\_id                    | UUID          | NOT NULL | —           | FK                       |
| carrying\_amount               | NUMERIC(20,2) | NOT NULL | —           | Gross atau Net per Stage |
| eir                            | NUMERIC(12,8) | NULL     | —           | Yang dipakai             |
| kupon\_kontraktual\_harian     | NUMERIC(20,2) | NOT NULL | —           | Native                   |
| pendapatan\_bunga\_eir\_harian | NUMERIC(20,2) | NOT NULL | —           | Native                   |
| amortisasi\_p\_d\_harian       | NUMERIC(20,2) | NULL     | 0           | Native; = EIR - kupon    |
| kurs\_tengah\_bi               | NUMERIC(15,4) | NULL     | —           |                          |
| pendapatan\_bunga\_idr         | NUMERIC(20,2) | NOT NULL | —           | Posted ke P\&L           |
| amortisasi\_p\_d\_idr          | NUMERIC(20,2) | NULL     | 0           | Posted to carrying       |
| fx\_unrealized\_idr            | NUMERIC(20,2) | NULL     | 0           | Selisih kurs harian      |
| stage\_saat\_akrual            | VARCHAR(20)   | NOT NULL | —           | STAGE\_1/2/3             |
| jurnal\_header\_id             | UUID          | NULL     | —           |                          |
| status                         | VARCHAR(20)   | NOT NULL | 'POSTED'    | POSTED/REVERSED          |
| created\_at                    | TIMESTAMPTZ   | NOT NULL | now()       |                          |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

**Indexes:**

  - uq\_akrual\_instrumen\_tanggal UNIQUE(instrumen\_id, tanggal\_akrual)

  - ix\_akrual\_periode (periode\_id)

  - ix\_akrual\_tanggal (tanggal\_akrual DESC)

  - ix\_akrual\_stage (stage\_saat\_akrual, tanggal\_akrual)

**Partitioning: RANGE (tanggal\_akrual) — yearly partitions**

### 5.7 trx.amortisasi

Amortisasi premium/diskonto per posting periode (umumnya per kupon). Generated dari eir\_amortization\_schedule saat status berubah dari PROYEKSI ke POSTED.

*Primary Key: id*

| **Column**                         | **Type**      | **Null** | **Default** | **Description**                    |
| ---------------------------------- | ------------- | -------- | ----------- | ---------------------------------- |
| id                                 | UUID          | NOT NULL | uuidv7()    | PK                                 |
| instrumen\_id                      | UUID          | NOT NULL | —           | FK                                 |
| schedule\_periode\_id              | UUID          | NOT NULL | —           | FK ecl.eir\_amortization\_schedule |
| periode\_id                        | UUID          | NOT NULL | —           | FK periode\_buku                   |
| tanggal\_posting                   | DATE          | NOT NULL | —           |                                    |
| amortisasi\_premium\_diskonto\_idr | NUMERIC(20,2) | NOT NULL | —           |                                    |
| jurnal\_header\_id                 | UUID          | NULL     | —           |                                    |
| status                             | VARCHAR(20)   | NOT NULL | 'POSTED'    |                                    |
| created\_at                        | TIMESTAMPTZ   | NOT NULL | now()       |                                    |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - schedule\_periode\_id → ecl.eir\_amortization\_schedule(id)

  - periode\_id → mst.periode\_buku(id)

**Indexes:**

  - ix\_amortisasi\_instrumen\_tanggal (instrumen\_id, tanggal\_posting)

  - ix\_amortisasi\_periode (periode\_id)

**Partitioning: RANGE (tanggal\_posting) — yearly partitions**

# 6\. Schema ecl — ECL Engine + EIR & Amortisasi (6 Entities)

Schema ecl menyimpan hasil computation Expected Credit Loss (3-stage, 3-skenario, dual FL) dan Effective Interest Rate (Newton-Raphson IRR + amortization schedule). Compliance-critical schema. Reference: FSD Appendix C.

## 6.0 Schema ecl — ER Diagram (Mermaid)

> \`\`\`mermaid  
> erDiagram  
> mst\_instrumen ||--o{ ecl\_calc\_header : "ECL per periode"  
> mst\_periode\_buku ||--o{ ecl\_calc\_header : "stamp"  
> ecl\_calc\_header ||--o{ ecl\_calc\_detail\_skenario : "3 skenario (Good/Normal/Bad)"  
> ecl\_calc\_header ||--o{ ecl\_lookthrough\_underlying : "untuk reksadana"  
> mst\_instrumen ||--o{ ecl\_stage\_history : "stage timeline"  
> mst\_instrumen ||--o{ eir\_amortization\_schedule : "schedule"  
> mst\_instrumen ||--o{ eir\_reestimation\_log : "log re-estimation"  
> eir\_amortization\_schedule }o--o{ trx\_amortisasi : "posting"  
> ecl\_stage\_history ||--o| jrnl\_header : "STAGE\_MIGRATION jurnal"  
> \`\`\`

### 6.1 ecl.calc\_header

Hasil computation ECL per instrumen per periode evaluasi. Header berisi summary ECL Weighted dan ECL FL; detail per skenario di table terkait. Per reksadana, terdapat juga look-through detail.

*Primary Key: id*

| **Column**              | **Type**      | **Null** | **Default** | **Description**          |
| ----------------------- | ------------- | -------- | ----------- | ------------------------ |
| id                      | UUID          | NOT NULL | uuidv7()    | PK                       |
| calc\_id\_kode          | VARCHAR(20)   | NOT NULL | —           | ECL-YYYY-MM-\#\#\#\#\#   |
| instrumen\_id           | UUID          | NOT NULL | —           | FK                       |
| periode\_id             | UUID          | NOT NULL | —           | FK                       |
| evaluation\_date        | DATE          | NOT NULL | —           | Period end               |
| stage                   | VARCHAR(10)   | NOT NULL | —           | STAGE\_1/2/3             |
| pd\_horizon             | VARCHAR(10)   | NOT NULL | —           | 12M atau 3Y/5Y/7Y/10Y    |
| ead\_native             | NUMERIC(20,2) | NOT NULL | —           |                          |
| ead\_idr                | NUMERIC(20,2) | NOT NULL | —           |                          |
| kurs\_tengah\_bi        | NUMERIC(15,4) | NULL     | —           | Untuk valas              |
| lgd                     | NUMERIC(8,4)  | NOT NULL | —           | Per Basel                |
| pd\_normal              | NUMERIC(8,4)  | NOT NULL | —           | Pefindo                  |
| impact\_mev\_good       | NUMERIC(8,4)  | NOT NULL | —           |                          |
| impact\_mev\_bad        | NUMERIC(8,4)  | NOT NULL | —           |                          |
| impact\_pd              | NUMERIC(8,4)  | NOT NULL | —           | FL multiplier            |
| w\_good                 | NUMERIC(8,4)  | NOT NULL | 0.2500      |                          |
| w\_normal               | NUMERIC(8,4)  | NOT NULL | 0.5000      |                          |
| w\_bad                  | NUMERIC(8,4)  | NOT NULL | 0.2500      |                          |
| ecl\_weighted\_idr      | NUMERIC(20,2) | NOT NULL | —           | Computed                 |
| ecl\_fl\_idr            | NUMERIC(20,2) | NOT NULL | —           | Final post FL            |
| delta\_ecl\_fl\_idr     | NUMERIC(20,2) | NULL     | —           | vs periode sebelumnya    |
| pengakuan\_lk           | VARCHAR(20)   | NOT NULL | —           | BOOK/RISK\_MGMT\_ONLY    |
| parameter\_snapshot\_id | UUID          | NOT NULL | —           | Audit re-perform         |
| jurnal\_header\_id      | UUID          | NULL     | —           | Reference                |
| calc\_run\_id           | UUID          | NOT NULL | —           | FK sys.job\_run\_history |
| status                  | VARCHAR(20)   | NOT NULL | 'POSTED'    |                          |
| created\_at             | TIMESTAMPTZ   | NOT NULL | now()       |                          |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - periode\_id → mst.periode\_buku(id)

  - jurnal\_header\_id → jrnl.header(id)

  - calc\_run\_id → sys.job\_run\_history(id)

**Indexes:**

  - uq\_ecl\_periode\_instrumen UNIQUE(periode\_id, instrumen\_id, calc\_run\_id)

  - ix\_ecl\_calc\_periode (periode\_id)

  - ix\_ecl\_calc\_instrumen (instrumen\_id)

  - ix\_ecl\_calc\_eval\_date (evaluation\_date)

  - ix\_ecl\_calc\_stage (stage, periode\_id)

  - ck\_bobot\_sum CHECK (w\_good + w\_normal + w\_bad BETWEEN 0.9999 AND 1.0001)

**Partitioning: LIST (periode\_id) — partition per periode bulanan**

### 6.2 ecl.calc\_detail\_skenario

Detail per skenario PD (Good/Normal/Bad) untuk satu calc\_header. 3 records per header standard.

*Primary Key: id*

| **Column**            | **Type**      | **Null** | **Default** | **Description**                   |
| --------------------- | ------------- | -------- | ----------- | --------------------------------- |
| id                    | UUID          | NOT NULL | uuidv7()    | PK                                |
| ecl\_calc\_header\_id | UUID          | NOT NULL | —           | FK                                |
| skenario              | VARCHAR(20)   | NOT NULL | —           | GOOD/NORMAL/BAD                   |
| pd\_skenario          | NUMERIC(8,4)  | NOT NULL | —           | Derived: PD\_Normal × Impact\_MEV |
| bobot                 | NUMERIC(8,4)  | NOT NULL | —           |                                   |
| ecl\_skenario\_idr    | NUMERIC(20,2) | NOT NULL | —           | Computed: EAD × PD × LGD          |

**Foreign Keys:**

  - ecl\_calc\_header\_id → ecl.calc\_header(id) ON DELETE CASCADE

**Indexes:**

  - uq\_ecl\_detail\_skenario UNIQUE(ecl\_calc\_header\_id, skenario)

  - ix\_ecl\_detail\_header (ecl\_calc\_header\_id)

  - ck\_skenario CHECK (skenario IN ('GOOD','NORMAL','BAD'))

### 6.3 ecl.lookthrough\_underlying

Look-through detail untuk reksadana — setiap underlying instrumen (sovereign, korporasi, cash, equity) sebagai eksposur tersendiri. Equity component flagged excluded (no ECL).

*Primary Key: id*

| **Column**                     | **Type**      | **Null** | **Default** | **Description**                                                |
| ------------------------------ | ------------- | -------- | ----------- | -------------------------------------------------------------- |
| id                             | UUID          | NOT NULL | uuidv7()    | PK                                                             |
| ecl\_calc\_header\_id          | UUID          | NOT NULL | —           | FK                                                             |
| underlying\_kategori           | VARCHAR(50)   | NOT NULL | —           | OBLIGASI\_PEMERINTAH/OBLIGASI\_KORPORASI/CASH\_BANK/EQUITY/dll |
| underlying\_issuer\_or\_rating | VARCHAR(100)  | NULL     | —           | Identifier                                                     |
| weight                         | NUMERIC(8,4)  | NOT NULL | —           | Bobot dalam komposisi NAB                                      |
| ead\_underlying\_idr           | NUMERIC(20,2) | NOT NULL | —           |                                                                |
| pd\_normal                     | NUMERIC(8,4)  | NULL     | —           | NULL untuk EQUITY                                              |
| lgd                            | NUMERIC(8,4)  | NULL     | —           |                                                                |
| ecl\_weighted\_idr             | NUMERIC(20,2) | NOT NULL | 0           | 0 untuk EQUITY                                                 |
| excluded                       | BOOLEAN       | NOT NULL | FALSE       | TRUE untuk EQUITY                                              |

**Foreign Keys:**

  - ecl\_calc\_header\_id → ecl.calc\_header(id) ON DELETE CASCADE

**Indexes:**

  - ix\_lookthrough\_header (ecl\_calc\_header\_id)

  - ix\_lookthrough\_kategori (underlying\_kategori)

### 6.4 ecl.stage\_history

Timeline stage migration per instrumen. Auto-create saat staging trigger (rating change, DPD, default) atau manual override (curing).

*Primary Key: id*

| **Column**               | **Type**      | **Null** | **Default** | **Description**                                                                                                   |
| ------------------------ | ------------- | -------- | ----------- | ----------------------------------------------------------------------------------------------------------------- |
| id                       | UUID          | NOT NULL | uuidv7()    | PK                                                                                                                |
| stage\_history\_id\_kode | VARCHAR(20)   | NOT NULL | —           | STH-YYYY-\#\#\#\#\#                                                                                               |
| instrumen\_id            | UUID          | NOT NULL | —           | FK                                                                                                                |
| tanggal\_migrasi         | DATE          | NOT NULL | —           |                                                                                                                   |
| stage\_sebelum           | VARCHAR(10)   | NOT NULL | —           | STAGE\_1/2/3                                                                                                      |
| stage\_sesudah           | VARCHAR(10)   | NOT NULL | —           |                                                                                                                   |
| trigger\_type            | VARCHAR(30)   | NOT NULL | —           | RATING\_DOWNGRADE/DPD\_30\_90/DPD\_GT\_90/DEFAULT\_RATING\_D/PKPU\_PAILIT/RESTRUKTURISASI/CURING/MANUAL\_OVERRIDE |
| detail\_trigger          | TEXT          | NULL     | —           |                                                                                                                   |
| rating\_saat\_migrasi    | VARCHAR(8)    | NULL     | —           |                                                                                                                   |
| dpd                      | INT           | NULL     | —           | Days past due                                                                                                     |
| delta\_ecl\_idr          | NUMERIC(20,2) | NULL     | —           | ECL impact                                                                                                        |
| user\_approver\_id       | UUID          | NULL     | —           | Wajib untuk curing                                                                                                |
| status\_approval         | VARCHAR(30)   | NOT NULL | 'AUTO'      | AUTO/PENDING\_APPROVAL/APPROVED/REJECTED                                                                          |
| dokumen\_pendukung\_id   | UUID          | NULL     | —           |                                                                                                                   |
| jurnal\_header\_id       | UUID          | NULL     | —           | STAGE\_MIGRATION jurnal                                                                                           |
| created\_at              | TIMESTAMPTZ   | NOT NULL | now()       |                                                                                                                   |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - user\_approver\_id → sec.user(id)

  - dokumen\_pendukung\_id → doc.upload(id)

  - jurnal\_header\_id → jrnl.header(id)

**Indexes:**

  - ix\_stage\_instrumen\_tanggal (instrumen\_id, tanggal\_migrasi DESC)

  - ix\_stage\_trigger (trigger\_type)

  - ix\_stage\_sesudah (stage\_sesudah)

### 6.5 ecl.eir\_amortization\_schedule

Amortization schedule per instrumen AC/FVOCI utang. Setiap baris = satu periode amortisasi (per kupon). Generated saat penempatan; di-recompute saat re-estimation.

*Primary Key: id*

| **Column**             | **Type**      | **Null** | **Default** | **Description**                     |
| ---------------------- | ------------- | -------- | ----------- | ----------------------------------- |
| id                     | UUID          | NOT NULL | uuidv7()    | PK                                  |
| schedule\_id\_kode     | VARCHAR(30)   | NOT NULL | —           | AMSCH-{kode\_instrumen}-{seq}       |
| instrumen\_id          | UUID          | NOT NULL | —           | FK                                  |
| periode\_seq           | INT           | NOT NULL | —           | 1, 2, 3, ...                        |
| tanggal\_posting       | DATE          | NOT NULL | —           |                                     |
| opening\_carrying      | NUMERIC(20,2) | NOT NULL | —           | \= closing periode sebelumnya       |
| cash\_inflow           | NUMERIC(20,2) | NOT NULL | —           | Kupon kontraktual                   |
| pendapatan\_bunga\_eir | NUMERIC(20,2) | NOT NULL | —           | Carrying × EIR/freq                 |
| amortisasi\_p\_d       | NUMERIC(20,2) | NOT NULL | —           | \= EIR - cash; negatif=premium      |
| pelunasan\_pokok       | NUMERIC(20,2) | NOT NULL | 0           | \= nominal saat last period         |
| closing\_carrying      | NUMERIC(20,2) | NOT NULL | —           | \= opening + amort - pelunasan      |
| eir\_periode           | NUMERIC(12,8) | NOT NULL | —           |                                     |
| stage\_saat\_posting   | VARCHAR(10)   | NULL     | 'STAGE\_1'  |                                     |
| status\_posting        | VARCHAR(20)   | NOT NULL | 'PROYEKSI'  | PROYEKSI/POSTED/REVERSED/RECOMPUTED |
| jurnal\_reference\_id  | UUID          | NULL     | —           | Setelah POSTED                      |
| recomputed\_from\_seq  | INT           | NULL     | —           | Versioning untuk re-estimation      |
| created\_at            | TIMESTAMPTZ   | NOT NULL | now()       |                                     |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - jurnal\_reference\_id → jrnl.header(id)

**Indexes:**

  - uq\_schedule\_instrumen\_periode UNIQUE(instrumen\_id, periode\_seq) WHERE recomputed\_from\_seq IS NULL

  - ix\_schedule\_instrumen (instrumen\_id, periode\_seq)

  - ix\_schedule\_tanggal (tanggal\_posting)

  - ix\_schedule\_status (status\_posting, tanggal\_posting)

**Notes:**

  - Saat re-estimation, baris PROYEKSI di-update ke RECOMPUTED dengan recomputed\_from\_seq referensi versi lama; baris baru di-insert.

  - Validasi: closing\_carrying baris terakhir HARUS ≈ 0 (toleransi 0.01 IDR after par discharge).

### 6.6 ecl.eir\_reestimation\_log

Log perubahan EIR — modifikasi material (derecognition + new EIR) atau revisi cash flow (EIR original retained, catch-up adjustment).

*Primary Key: id*

| **Column**              | **Type**      | **Null** | **Default** | **Description**                                                      |
| ----------------------- | ------------- | -------- | ----------- | -------------------------------------------------------------------- |
| id                      | UUID          | NOT NULL | uuidv7()    | PK                                                                   |
| log\_id\_kode           | VARCHAR(30)   | NOT NULL | —           | EIR-RE-YYYY-\#\#\#\#\#                                               |
| instrumen\_id           | UUID          | NOT NULL | —           | FK                                                                   |
| tanggal\_re\_estimation | DATE          | NOT NULL | —           |                                                                      |
| trigger\_type           | VARCHAR(50)   | NOT NULL | —           | MODIFIKASI\_MATERIAL/REVISI\_CASH\_FLOW/PREPAYMENT/STEP\_UP\_TRIGGER |
| eir\_sebelum            | NUMERIC(12,8) | NOT NULL | —           |                                                                      |
| eir\_sesudah            | NUMERIC(12,8) | NOT NULL | —           | Same untuk REVISI\_CASH\_FLOW                                        |
| carrying\_sebelum       | NUMERIC(20,2) | NOT NULL | —           |                                                                      |
| carrying\_sesudah       | NUMERIC(20,2) | NOT NULL | —           |                                                                      |
| catch\_up\_adjustment   | NUMERIC(20,2) | NULL     | 0           | Δ posted to P\&L                                                     |
| modifikasi\_terms\_json | JSONB         | NULL     | —           | Detail term changes                                                  |
| dokumen\_pendukung\_id  | UUID          | NULL     | —           |                                                                      |
| maker\_id               | UUID          | NOT NULL | —           |                                                                      |
| reviewer\_id            | UUID          | NULL     | —           | Akuntansi                                                            |
| approver\_id            | UUID          | NULL     | —           | FC / CFO                                                             |
| workflow\_status        | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                                                                      |
| jurnal\_header\_id      | UUID          | NULL     | —           |                                                                      |
| created\_at             | TIMESTAMPTZ   | NOT NULL | now()       |                                                                      |
| approved\_at            | TIMESTAMPTZ   | NULL     | —           |                                                                      |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - dokumen\_pendukung\_id → doc.upload(id)

  - maker\_id, reviewer\_id, approver\_id → sec.user(id)

  - jurnal\_header\_id → jrnl.header(id)

**Indexes:**

  - uq\_eir\_log\_kode UNIQUE(log\_id\_kode)

  - ix\_eir\_log\_instrumen (instrumen\_id, tanggal\_re\_estimation DESC)

  - ix\_eir\_log\_trigger (trigger\_type)

# 7\. Schema sppi — SPPI/BM/Klasifikasi (4 Entities)

Schema sppi mengelola pre-trade clearance: SPPI Test (10 questions), BM Test, klasifikasi history, dan reklasifikasi prospektif.

## 7.0 ER Diagram (Mermaid)

> \`\`\`mermaid  
> erDiagram  
> mst\_instrumen ||--o{ sppi\_sppi\_test : "diuji"  
> mst\_portofolio ||--o{ sppi\_bm\_test : "tested"  
> mst\_instrumen ||--o{ sppi\_klasifikasi\_history : "klasifikasi timeline"  
> mst\_instrumen ||--o{ sppi\_reklasifikasi\_log : "reklas events"  
> sppi\_sppi\_test ||--o| sec\_user : "maker/reviewer/approver"  
> sppi\_bm\_test ||--o| sec\_user : "maker/approver"  
> \`\`\`

### 7.1 sppi.sppi\_test

SPPI Test result per instrumen — 10-question checklist + auto-evaluation. Multiple records per instrumen untuk INITIAL, PERIODIC review, atau TRIGGERED reassessment.

*Primary Key: id*

| **Column**              | **Type**     | **Null** | **Default** | **Description**                                           |
| ----------------------- | ------------ | -------- | ----------- | --------------------------------------------------------- |
| id                      | UUID         | NOT NULL | uuidv7()    | PK                                                        |
| sppi\_test\_id\_kode    | VARCHAR(20)  | NOT NULL | —           | SPPI-YYYY-\#\#\#\#\#                                      |
| instrumen\_id           | UUID         | NOT NULL | —           | FK                                                        |
| tanggal\_test           | DATE         | NOT NULL | —           |                                                           |
| tipe\_test              | VARCHAR(20)  | NOT NULL | —           | INITIAL/PERIODIC/TRIGGERED                                |
| jawaban\_checklist      | JSONB        | NOT NULL | —           | {Q1: 'YES', Q1\_note: '...', ..., Q10: ...}               |
| hasil\_sppi             | VARCHAR(10)  | NOT NULL | —           | PASS/FAIL — auto-derived                                  |
| fail\_indicator\_reason | VARCHAR(500) | NULL     | —           | Reason jika FAIL (mis. 'Q4: konversi ekuitas')            |
| catatan\_penilaian      | TEXT         | NULL     | —           | Justifikasi naratif                                       |
| dokumen\_bukti\_id      | UUID         | NULL     | —           | Term sheet, prospektus                                    |
| maker\_id               | UUID         | NOT NULL | —           | Treasury                                                  |
| reviewer\_id            | UUID         | NULL     | —           | Risk/Akuntansi                                            |
| approver\_id            | UUID         | NULL     | —           | Komite Investasi                                          |
| status                  | VARCHAR(30)  | NOT NULL | 'DRAFT'     | DRAFT/PENDING\_REVIEW/PENDING\_APPROVAL/APPROVED/REJECTED |
| created\_at             | TIMESTAMPTZ  | NOT NULL | now()       |                                                           |
| reviewed\_at            | TIMESTAMPTZ  | NULL     | —           |                                                           |
| approved\_at            | TIMESTAMPTZ  | NULL     | —           |                                                           |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - dokumen\_bukti\_id → doc.upload(id)

  - maker\_id, reviewer\_id, approver\_id → sec.user(id)

**Indexes:**

  - uq\_sppi\_test\_kode UNIQUE(sppi\_test\_id\_kode)

  - ix\_sppi\_instrumen\_tanggal (instrumen\_id, tanggal\_test DESC)

  - ix\_sppi\_status (status) WHERE status NOT IN ('APPROVED','REJECTED')

  - ix\_sppi\_hasil (hasil\_sppi)

**Notes:**

  - Latest active SPPI test = WHERE status='APPROVED' ORDER BY tanggal\_test DESC LIMIT 1

  - Annual periodic review WAJIB; sistem alert 30 hari sebelum expired (1 tahun setelah last APPROVED).

### 7.2 sppi.bm\_test

Business Model Test result per portofolio. Auto-suggest dari indikator (frekuensi penjualan, volume, evaluasi kinerja); override hanya dengan justifikasi tertulis + approval Komite.

*Primary Key: id*

| **Column**                 | **Type**     | **Null** | **Default** | **Description**                                      |
| -------------------------- | ------------ | -------- | ----------- | ---------------------------------------------------- |
| id                         | UUID         | NOT NULL | uuidv7()    | PK                                                   |
| bm\_test\_id\_kode         | VARCHAR(20)  | NOT NULL | —           | BMT-YYYY-\#\#\#\#\#                                  |
| portofolio\_id             | UUID         | NOT NULL | —           | FK                                                   |
| tanggal\_penilaian         | DATE         | NOT NULL | —           |                                                      |
| tipe\_test                 | VARCHAR(20)  | NOT NULL | —           | INITIAL/PERIODIC/TRIGGERED                           |
| tujuan\_pengelolaan        | TEXT         | NOT NULL | —           |                                                      |
| indikator\_penilaian       | JSONB        | NOT NULL | —           | {volume\_jual\_pct, frekuensi, dasar\_evaluasi, ...} |
| frekuensi\_penjualan\_12m  | NUMERIC(8,4) | NOT NULL | —           | % volume vs total portofolio                         |
| hasil\_bm\_test\_suggested | VARCHAR(10)  | NOT NULL | —           | HTC/HTCS/OTHER (auto)                                |
| hasil\_bm\_test\_final     | VARCHAR(10)  | NOT NULL | —           | HTC/HTCS/OTHER (after override jika ada)             |
| override\_flag             | BOOLEAN      | NOT NULL | FALSE       |                                                      |
| justifikasi\_override      | TEXT         | NULL     | —           | Required jika override\_flag=TRUE                    |
| dokumen\_bukti\_id         | UUID         | NULL     | —           | Investment Policy, KPI manager, memo Komite          |
| approver\_id               | UUID         | NULL     | —           | Komite Investasi                                     |
| periode\_berlaku\_dari     | DATE         | NOT NULL | —           |                                                      |
| periode\_berlaku\_sampai   | DATE         | NOT NULL | —           | Auto +1 year                                         |
| status                     | VARCHAR(30)  | NOT NULL | 'DRAFT'     |                                                      |
| created\_at                | TIMESTAMPTZ  | NOT NULL | now()       |                                                      |
| approved\_at               | TIMESTAMPTZ  | NULL     | —           |                                                      |

**Foreign Keys:**

  - portofolio\_id → mst.portofolio(id)

  - dokumen\_bukti\_id → doc.upload(id)

  - approver\_id → sec.user(id)

**Indexes:**

  - uq\_bm\_test\_kode UNIQUE(bm\_test\_id\_kode)

  - ix\_bm\_portofolio\_tanggal (portofolio\_id, tanggal\_penilaian DESC)

  - ix\_bm\_aktif (portofolio\_id) WHERE status='APPROVED' AND CURRENT\_DATE BETWEEN periode\_berlaku\_dari AND periode\_berlaku\_sampai

### 7.3 sppi.klasifikasi\_history

Timeline klasifikasi PSAK 71 per instrumen. Setiap perubahan klasifikasi (initial atau reklas) menjadi record baru dengan tanggal efektif dan reference ke SPPI/BM Test.

*Primary Key: id*

| **Column**             | **Type**    | **Null** | **Default** | **Description**                   |
| ---------------------- | ----------- | -------- | ----------- | --------------------------------- |
| id                     | UUID        | NOT NULL | uuidv7()    | PK                                |
| instrumen\_id          | UUID        | NOT NULL | —           | FK                                |
| tanggal\_efektif       | DATE        | NOT NULL | —           |                                   |
| klasifikasi            | VARCHAR(20) | NOT NULL | —           | AC/FVOCI/FVOCI\_ELECTION/FVTPL    |
| sppi\_test\_id         | UUID        | NULL     | —           | FK ke SPPI Test result            |
| bm\_test\_id           | UUID        | NULL     | —           | FK ke BM Test                     |
| reklasifikasi\_log\_id | UUID        | NULL     | —           | FK jika perubahan via reklas      |
| alasan                 | TEXT        | NOT NULL | —           | Initial atau reklasifikasi reason |
| approved\_by           | UUID        | NOT NULL | —           | Komite Investasi                  |
| approved\_at           | TIMESTAMPTZ | NOT NULL | now()       |                                   |
| periode\_berakhir      | DATE        | NULL     | —           | Saat reklas berikutnya            |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - sppi\_test\_id → sppi.sppi\_test(id)

  - bm\_test\_id → sppi.bm\_test(id)

  - reklasifikasi\_log\_id → sppi.reklasifikasi\_log(id)

  - approved\_by → sec.user(id)

**Indexes:**

  - ix\_klasifikasi\_instrumen\_tanggal (instrumen\_id, tanggal\_efektif DESC)

  - ix\_klasifikasi\_aktif (instrumen\_id) WHERE periode\_berakhir IS NULL

### 7.4 sppi.reklasifikasi\_log

Log reklasifikasi prospektif. Setiap reklas mencatat from-to klasifikasi, FV pada tanggal efektif, treatment OCI, dan jurnal transisi yang ter-posting.

*Primary Key: id*

| **Column**                    | **Type**      | **Null** | **Default** | **Description**              |
| ----------------------------- | ------------- | -------- | ----------- | ---------------------------- |
| id                            | UUID          | NOT NULL | uuidv7()    | PK                           |
| reklas\_id\_kode              | VARCHAR(20)   | NOT NULL | —           | REKLAS-YYYY-\#\#\#\#\#       |
| instrumen\_id                 | UUID          | NOT NULL | —           | FK                           |
| klasifikasi\_dari             | VARCHAR(20)   | NOT NULL | —           | AC/FVOCI/FVTPL               |
| klasifikasi\_ke               | VARCHAR(20)   | NOT NULL | —           | AC/FVOCI/FVTPL               |
| tanggal\_efektif              | DATE          | NOT NULL | —           |                              |
| fair\_value\_tanggal\_efektif | NUMERIC(20,2) | NOT NULL | —           | Untuk computation            |
| carrying\_amount\_dari        | NUMERIC(20,2) | NOT NULL | —           | Sebelum reklas               |
| accumulated\_oci\_dari        | NUMERIC(20,2) | NULL     | 0           | Untuk dari FVOCI             |
| eir\_dari                     | NUMERIC(12,8) | NULL     | —           |                              |
| eir\_ke                       | NUMERIC(12,8) | NULL     | —           | Auto-compute jika applicable |
| justifikasi                   | TEXT          | NOT NULL | —           |                              |
| dokumen\_bukti\_id            | UUID          | NULL     | —           |                              |
| maker\_id                     | UUID          | NOT NULL | —           |                              |
| approver\_id                  | UUID          | NULL     | —           | Komite Investasi             |
| jurnal\_header\_id            | UUID          | NULL     | —           | Reference jurnal transisi    |
| status                        | VARCHAR(30)   | NOT NULL | 'DRAFT'     |                              |
| created\_at                   | TIMESTAMPTZ   | NOT NULL | now()       |                              |
| approved\_at                  | TIMESTAMPTZ   | NULL     | —           |                              |

**Foreign Keys:**

  - instrumen\_id → mst.instrumen(id)

  - dokumen\_bukti\_id → doc.upload(id)

  - maker\_id, approver\_id → sec.user(id)

  - jurnal\_header\_id → jrnl.header(id)

**Indexes:**

  - uq\_reklas\_kode UNIQUE(reklas\_id\_kode)

  - ix\_reklas\_instrumen (instrumen\_id, tanggal\_efektif DESC)

  - ix\_reklas\_status (status)

# 8\. Schema doc — Document Management (3 Entities)

Schema doc mengelola dokumen pendukung yang ter-link ke setiap event di sistem. Storage di S3-compatible dengan KMS encryption; metadata di DB dengan SHA-256 hash integrity.

## 8.0 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> doc\_upload ||--o{ doc\_link : "linked to entities"  
> doc\_upload ||--o{ doc\_access\_log : "access tracking"  
> doc\_upload }o..|| sec\_user : "uploader"  
> \`\`\`

### 8.1 doc.upload

Document upload registry — file metadata + S3 path + SHA-256 hash. Setiap dokumen unique; multiple links via doc.link untuk reference dari berbagai entitas.

*Primary Key: id*

| **Column**          | **Type**     | **Null** | **Default** | **Description**                                |
| ------------------- | ------------ | -------- | ----------- | ---------------------------------------------- |
| id                  | UUID         | NOT NULL | uuidv7()    | PK                                             |
| filename            | VARCHAR(255) | NOT NULL | —           | Original filename                              |
| filename\_storage   | VARCHAR(500) | NOT NULL | —           | documents/{year}/{month}/{entity}/{uuid}.{ext} |
| mime\_type          | VARCHAR(100) | NOT NULL | —           |                                                |
| file\_size\_bytes   | BIGINT       | NOT NULL | —           | Max 50 MB                                      |
| sha256\_hash        | CHAR(64)     | NOT NULL | —           | Integrity check                                |
| uploader\_id        | UUID         | NOT NULL | —           |                                                |
| uploaded\_at        | TIMESTAMPTZ  | NOT NULL | now()       |                                                |
| uploader\_ip        | INET         | NULL     | —           |                                                |
| virus\_scan\_result | VARCHAR(20)  | NOT NULL | 'PENDING'   | CLEAN/INFECTED/PENDING                         |
| virus\_scan\_at     | TIMESTAMPTZ  | NULL     | —           |                                                |
| status              | VARCHAR(20)  | NOT NULL | 'ACTIVE'    | ACTIVE/INACTIVE                                |
| inactive\_by        | UUID         | NULL     | —           | Hanya CFO                                      |
| inactive\_at        | TIMESTAMPTZ  | NULL     | —           |                                                |
| inactive\_reason    | TEXT         | NULL     | —           |                                                |
| s3\_kms\_key\_id    | VARCHAR(100) | NULL     | —           | KMS key reference                              |
| s3\_version\_id     | VARCHAR(100) | NULL     | —           | S3 versioning                                  |

**Foreign Keys:**

  - uploader\_id, inactive\_by → sec.user(id)

**Indexes:**

  - ix\_doc\_upload\_uploader (uploader\_id, uploaded\_at DESC)

  - ix\_doc\_upload\_status (status) WHERE status='ACTIVE'

  - ix\_doc\_upload\_hash (sha256\_hash)

  - ix\_doc\_upload\_virus (virus\_scan\_result) WHERE virus\_scan\_result IN ('PENDING','INFECTED')

  - ck\_file\_size CHECK (file\_size\_bytes \<= 52428800)

**Notes:**

  - Hard delete TIDAK DIPERBOLEHKAN. Inactive only via CFO + justifikasi.

  - Virus scan async — file ter-store di staging area sampai CLEAN; INFECTED → block + delete.

### 8.2 doc.link

Polymorphic linking — satu dokumen dapat di-link ke beberapa entitas di multiple schemas. Generic FK pattern: entity\_type + entity\_id.

*Primary Key: id*

| **Column**      | **Type**    | **Null** | **Default** | **Description**                                                                                                                                             |
| --------------- | ----------- | -------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| id              | UUID        | NOT NULL | uuidv7()    | PK                                                                                                                                                          |
| doc\_upload\_id | UUID        | NOT NULL | —           | FK                                                                                                                                                          |
| entity\_type    | VARCHAR(50) | NOT NULL | —           | INSTRUMEN/SPPI\_TEST/BM\_TEST/PENEMPATAN/MTM/RENEWAL/PENJUALAN/JATUH\_TEMPO/AKRUAL/RATING\_HISTORY/REKLAS\_LOG/ECL\_CALC/EIR\_REESTIMATION/PERIODE\_CLOSING |
| entity\_id      | UUID        | NOT NULL | —           | Generic FK ke target                                                                                                                                        |
| link\_type      | VARCHAR(30) | NOT NULL | —           | BUKTI/MEMO/AMANDEMEN/PERSETUJUAN/REFERENSI                                                                                                                  |
| linked\_by      | UUID        | NOT NULL | —           |                                                                                                                                                             |
| linked\_at      | TIMESTAMPTZ | NOT NULL | now()       |                                                                                                                                                             |

**Foreign Keys:**

  - doc\_upload\_id → doc.upload(id)

  - linked\_by → sec.user(id)

**Indexes:**

  - ix\_doc\_link\_doc (doc\_upload\_id)

  - ix\_doc\_link\_entity (entity\_type, entity\_id)

  - uq\_doc\_link UNIQUE(doc\_upload\_id, entity\_type, entity\_id, link\_type)

**Notes:**

  - Polymorphic FK — entity\_id tidak punya FK constraint formal (karena multi-target); validation di application layer.

  - Trigger BEFORE INSERT/UPDATE: validate entity\_id exists in expected entity\_type schema.

### 8.3 doc.access\_log

Log setiap akses (view/download) dokumen. High-volume audit table; partitioned by month.

*Primary Key: id*

| **Column**           | **Type**     | **Null** | **Default** | **Description**                      |
| -------------------- | ------------ | -------- | ----------- | ------------------------------------ |
| id                   | UUID         | NOT NULL | uuidv7()    | PK                                   |
| doc\_upload\_id      | UUID         | NOT NULL | —           | FK                                   |
| accessed\_by         | UUID         | NOT NULL | —           |                                      |
| accessed\_at         | TIMESTAMPTZ  | NOT NULL | now()       |                                      |
| access\_type         | VARCHAR(20)  | NOT NULL | —           | VIEW/DOWNLOAD/PREVIEW                |
| ip\_address          | INET         | NULL     | —           |                                      |
| user\_agent          | VARCHAR(500) | NULL     | —           |                                      |
| pre\_signed\_url\_id | VARCHAR(100) | NULL     | —           | Untuk download via S3 pre-signed URL |
| url\_expires\_at     | TIMESTAMPTZ  | NULL     | —           | 5 menit window                       |

**Foreign Keys:**

  - doc\_upload\_id → doc.upload(id)

  - accessed\_by → sec.user(id)

**Indexes:**

  - ix\_doc\_access\_doc (doc\_upload\_id, accessed\_at DESC)

  - ix\_doc\_access\_user (accessed\_by, accessed\_at DESC)

  - ix\_doc\_access\_time (accessed\_at DESC)

**Partitioning: RANGE (accessed\_at) — monthly**

# 9\. Schema jrnl — Jurnal & GL Interface (3 Entities)

Schema jrnl menyimpan semua jurnal accounting yang dihasilkan oleh resolusi runtime mapping engine. Detail per line + GL Host delivery status.

## 9.0 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> mst\_mapping\_jurnal\_header }o..|| jrnl\_header : "template untuk event"  
> jrnl\_header ||--o{ jrnl\_detail : "lines"  
> mst\_chart\_of\_accounts ||--o{ jrnl\_detail : "akun"  
> jrnl\_header ||--o{ jrnl\_gl\_status : "delivery tracking"  
> mst\_periode\_buku ||--o{ jrnl\_header : "stamp"  
> \`\`\`

### 9.1 jrnl.header

Jurnal Header — satu posting event = satu header. Mengikat semua detail D/K lines. Trigger delivery ke GL Host async.

*Primary Key: id*

| **Column**               | **Type**      | **Null** | **Default** | **Description**                                |
| ------------------------ | ------------- | -------- | ----------- | ---------------------------------------------- |
| id                       | UUID          | NOT NULL | uuidv7()    | PK                                             |
| no\_jurnal               | VARCHAR(20)   | NOT NULL | —           | JNL-YYYY-MM-\#\#\#\#\#                         |
| tanggal\_posting         | DATE          | NOT NULL | —           |                                                |
| periode\_id              | UUID          | NOT NULL | —           | FK                                             |
| event\_code              | VARCHAR(40)   | NOT NULL | —           | PENEMPATAN/AKRUAL\_BUNGA/dll                   |
| mapping\_header\_id      | UUID          | NULL     | —           | FK template                                    |
| instrumen\_id            | UUID          | NULL     | —           | Optional                                       |
| reference\_event\_type   | VARCHAR(50)   | NOT NULL | —           | trx\_penempatan/trx\_mtm/ecl\_calc\_header/etc |
| reference\_event\_id     | UUID          | NOT NULL | —           | Polymorphic FK                                 |
| currency                 | CHAR(3)       | NOT NULL | 'IDR'       |                                                |
| total\_debit             | NUMERIC(20,2) | NOT NULL | —           |                                                |
| total\_kredit            | NUMERIC(20,2) | NOT NULL | —           |                                                |
| narrative                | VARCHAR(500)  | NULL     | —           | Description                                    |
| status\_internal         | VARCHAR(20)   | NOT NULL | 'POSTED'    | POSTED/REVERSED/PENDING                        |
| reversed\_by\_jurnal\_id | UUID          | NULL     | —           | Self-ref jika ada reversal jurnal              |
| created\_at              | TIMESTAMPTZ   | NOT NULL | now()       |                                                |
| created\_by              | UUID          | NULL     | —           | User-input atau SYSTEM untuk job               |
| idempotency\_key         | VARCHAR(100)  | NULL     | —           | Untuk dedup retry                              |

**Foreign Keys:**

  - periode\_id → mst.periode\_buku(id)

  - mapping\_header\_id → mst.mapping\_jurnal\_header(id)

  - instrumen\_id → mst.instrumen(id)

  - reversed\_by\_jurnal\_id → jrnl.header(id) (self-ref)

  - created\_by → sec.user(id)

**Indexes:**

  - uq\_jrnl\_no UNIQUE(no\_jurnal)

  - ix\_jrnl\_periode (periode\_id)

  - ix\_jrnl\_event (event\_code, tanggal\_posting)

  - ix\_jrnl\_reference (reference\_event\_type, reference\_event\_id)

  - ix\_jrnl\_tanggal (tanggal\_posting DESC)

  - uq\_jrnl\_idempotency UNIQUE(idempotency\_key) WHERE idempotency\_key IS NOT NULL

  - ck\_balance CHECK (total\_debit = total\_kredit)

**Partitioning: RANGE (tanggal\_posting) — yearly**

### 9.2 jrnl.detail

Detail line per jurnal header — satu record per debit/kredit line. Total debit harus sama dengan total kredit (validated at header level).

*Primary Key: id*

| **Column**      | **Type**      | **Null** | **Default** | **Description** |
| --------------- | ------------- | -------- | ----------- | --------------- |
| id              | UUID          | NOT NULL | uuidv7()    | PK              |
| header\_id      | UUID          | NOT NULL | —           | FK              |
| urutan          | INT           | NOT NULL | —           | 1, 2, 3, ...    |
| kode\_akun\_id  | UUID          | NOT NULL | —           | FK              |
| debit\_amount   | NUMERIC(20,2) | NOT NULL | 0           |                 |
| kredit\_amount  | NUMERIC(20,2) | NOT NULL | 0           |                 |
| mata\_uang      | CHAR(3)       | NOT NULL | 'IDR'       |                 |
| narrative\_line | VARCHAR(500)  | NULL     | —           |                 |
| created\_at     | TIMESTAMPTZ   | NOT NULL | now()       |                 |

**Foreign Keys:**

  - header\_id → jrnl.header(id) ON DELETE CASCADE

  - kode\_akun\_id → mst.chart\_of\_accounts(id)

**Indexes:**

  - ix\_jrnl\_detail\_header (header\_id, urutan)

  - ix\_jrnl\_detail\_akun (kode\_akun\_id)

  - ck\_dk\_exclusive CHECK ((debit\_amount \> 0 AND kredit\_amount = 0) OR (debit\_amount = 0 AND kredit\_amount \> 0))

### 9.3 jrnl.gl\_status

Tracking delivery status jurnal ke GL Host (REST API atau file batch). Per jurnal header, satu record gl\_status — di-update saat retry, success, atau failure.

*Primary Key: id*

| **Column**            | **Type**     | **Null** | **Default**         | **Description**                                          |
| --------------------- | ------------ | -------- | ------------------- | -------------------------------------------------------- |
| id                    | UUID         | NOT NULL | uuidv7()            | PK                                                       |
| header\_id            | UUID         | NOT NULL | —                   | FK                                                       |
| gl\_host\_status      | VARCHAR(20)  | NOT NULL | 'PENDING\_DELIVERY' | PENDING\_DELIVERY/DELIVERED/FAILED/RETRYING/DEAD\_LETTER |
| gl\_host\_journal\_id | VARCHAR(50)  | NULL     | —                   | ID dari GL Host setelah ter-post                         |
| delivered\_at         | TIMESTAMPTZ  | NULL     | —                   |                                                          |
| retry\_count          | INT          | NOT NULL | 0                   |                                                          |
| last\_retry\_at       | TIMESTAMPTZ  | NULL     | —                   |                                                          |
| last\_error           | TEXT         | NULL     | —                   | Error message dari GL Host                               |
| delivery\_mode        | VARCHAR(20)  | NOT NULL | 'API'               | API/FILE\_BATCH                                          |
| batch\_file\_id       | VARCHAR(100) | NULL     | —                   | Untuk FILE\_BATCH mode                                   |
| created\_at           | TIMESTAMPTZ  | NOT NULL | now()               |                                                          |
| updated\_at           | TIMESTAMPTZ  | NULL     | —                   |                                                          |

**Foreign Keys:**

  - header\_id → jrnl.header(id) ON DELETE CASCADE

**Indexes:**

  - uq\_gl\_status\_header UNIQUE(header\_id)

  - ix\_gl\_status\_pending (gl\_host\_status) WHERE gl\_host\_status IN ('PENDING\_DELIVERY','RETRYING','FAILED')

  - ix\_gl\_status\_dlq (gl\_host\_status) WHERE gl\_host\_status='DEAD\_LETTER'

**Notes:**

  - Reconciliation daily job: cek semua jurnal status\_internal=POSTED yang gl\_host\_status \!= DELIVERED dan alert IT.

  - Dead letter queue: failed \> 3 retries; manual investigation oleh Akuntansi+IT.

# 10\. Schema aud — Audit Trail (3 Entities)

Schema aud menyediakan immutable audit trail. Tabel append-only — tidak ada UPDATE atau DELETE diperbolehkan via trigger enforcement.

## 10.0 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> aud\_audit\_log }o--o{ sec\_user : "actor"  
> aud\_workflow\_history }o--o{ sec\_user : "actor in transition"  
> aud\_login\_history }o--o{ sec\_user : "user logging"  
> aud\_audit\_log ||..|| ALL\_ENTITIES : "polymorphic FK"  
> \`\`\`

### 10.1 aud.audit\_log

Append-only immutable audit log untuk semua action material di sistem. Polymorphic — entity\_type + entity\_id refer ke any table di any schema.

*Primary Key: id*

| **Column**        | **Type**     | **Null** | **Default** | **Description**                                                                     |
| ----------------- | ------------ | -------- | ----------- | ----------------------------------------------------------------------------------- |
| id                | UUID         | NOT NULL | uuidv7()    | PK                                                                                  |
| entity\_type      | VARCHAR(50)  | NOT NULL | —           | Schema.table (mis. mst.instrumen, sppi.sppi\_test)                                  |
| entity\_id        | UUID         | NOT NULL | —           | Polymorphic FK                                                                      |
| action            | VARCHAR(30)  | NOT NULL | —           | INSERT/UPDATE/DELETE/APPROVE/REJECT/LOGIN/LOGOUT/PARAMETER\_CHANGE/EXPORT/CALCULATE |
| actor\_user\_id   | UUID         | NULL     | —           | NULL untuk SYSTEM action                                                            |
| actor\_role       | VARCHAR(50)  | NULL     | —           | Cache role saat action                                                              |
| timestamp         | TIMESTAMPTZ  | NOT NULL | now()       |                                                                                     |
| ip\_address       | INET         | NULL     | —           |                                                                                     |
| user\_agent       | VARCHAR(500) | NULL     | —           |                                                                                     |
| before\_value     | JSONB        | NULL     | —           | Snapshot sebelum perubahan (untuk UPDATE)                                           |
| after\_value      | JSONB        | NULL     | —           | Snapshot sesudah perubahan                                                          |
| changed\_columns  | VARCHAR\[\]  | NULL     | —           | List kolom yang berubah                                                             |
| metadata          | JSONB        | NULL     | —           | Additional context                                                                  |
| session\_id       | VARCHAR(100) | NULL     | —           |                                                                                     |
| request\_id       | VARCHAR(100) | NULL     | —           | API request trace                                                                   |
| hash\_chain\_prev | CHAR(64)     | NULL     | —           | Hash dari record sebelumnya — tamper detection                                      |

**Foreign Keys:**

  - actor\_user\_id → sec.user(id) — soft FK (nullable untuk SYSTEM action)

**Indexes:**

  - ix\_audit\_entity (entity\_type, entity\_id, timestamp DESC)

  - ix\_audit\_actor (actor\_user\_id, timestamp DESC)

  - ix\_audit\_action (action, timestamp DESC)

  - ix\_audit\_timestamp (timestamp DESC)

  - GIN ix\_audit\_before\_gin (before\_value)

  - GIN ix\_audit\_after\_gin (after\_value)

**Partitioning: RANGE (timestamp) — monthly partitions; retention 10 tahun online + 10 tahun cold storage**

**Notes:**

  - TRIGGER tg\_audit\_no\_update\_delete: BEFORE UPDATE/DELETE — RAISE EXCEPTION (immutable enforcement).

  - Hash chain optional — setiap record include SHA-256 dari record sebelumnya untuk tamper detection (blockchain-like).

  - Auto-populate via application middleware atau database trigger pada setiap mutation.

### 10.2 aud.workflow\_history

Tracking state transitions untuk semua workflow (Maker-Reviewer-Approver). Setiap state transition menjadi record baru.

*Primary Key: id*

| **Column**      | **Type**    | **Null** | **Default** | **Description**                                           |
| --------------- | ----------- | -------- | ----------- | --------------------------------------------------------- |
| id              | UUID        | NOT NULL | uuidv7()    | PK                                                        |
| entity\_type    | VARCHAR(50) | NOT NULL | —           | trx.penempatan, sppi.sppi\_test, dll                      |
| entity\_id      | UUID        | NOT NULL | —           |                                                           |
| state\_from     | VARCHAR(30) | NULL     | —           | DRAFT/PENDING\_REVIEW/PENDING\_APPROVAL/APPROVED/REJECTED |
| state\_to       | VARCHAR(30) | NOT NULL | —           |                                                           |
| actor\_user\_id | UUID        | NOT NULL | —           |                                                           |
| actor\_role     | VARCHAR(50) | NULL     | —           |                                                           |
| action\_type    | VARCHAR(30) | NOT NULL | —           | SUBMIT/APPROVE/REJECT/REVOKE/EXPIRE                       |
| comment         | TEXT        | NULL     | —           | Wajib untuk REJECT                                        |
| timestamp       | TIMESTAMPTZ | NOT NULL | now()       |                                                           |
| sla\_deadline   | TIMESTAMPTZ | NULL     | —           | SLA target untuk step ini                                 |
| sla\_status     | VARCHAR(20) | NULL     | —           | ON\_TIME/LATE/EXPIRED                                     |

**Foreign Keys:**

  - actor\_user\_id → sec.user(id)

**Indexes:**

  - ix\_workflow\_entity (entity\_type, entity\_id, timestamp DESC)

  - ix\_workflow\_actor (actor\_user\_id, timestamp DESC)

  - ix\_workflow\_state (state\_to, sla\_status)

**Partitioning: RANGE (timestamp) — quarterly**

### 10.3 aud.login\_history

Tracking login/logout activities. Untuk security analysis (brute force detection, unusual login patterns).

*Primary Key: id*

| **Column**          | **Type**     | **Null** | **Default** | **Description**                                                              |
| ------------------- | ------------ | -------- | ----------- | ---------------------------------------------------------------------------- |
| id                  | UUID         | NOT NULL | uuidv7()    | PK                                                                           |
| user\_id            | UUID         | NULL     | —           | NULL untuk failed login dengan unknown user                                  |
| username\_attempted | VARCHAR(100) | NOT NULL | —           |                                                                              |
| login\_at           | TIMESTAMPTZ  | NOT NULL | now()       |                                                                              |
| logout\_at          | TIMESTAMPTZ  | NULL     | —           |                                                                              |
| session\_id         | VARCHAR(100) | NULL     | —           |                                                                              |
| ip\_address         | INET         | NULL     | —           |                                                                              |
| user\_agent         | VARCHAR(500) | NULL     | —           |                                                                              |
| status              | VARCHAR(20)  | NOT NULL | —           | SUCCESS/FAILED/LOCKED/MFA\_FAIL/EXPIRED                                      |
| failure\_reason     | VARCHAR(100) | NULL     | —           | Untuk failed: WRONG\_PASSWORD/MFA\_REQUIRED/ACCOUNT\_LOCKED/USER\_NOT\_FOUND |
| mfa\_used           | BOOLEAN      | NOT NULL | FALSE       |                                                                              |
| geo\_country        | VARCHAR(2)   | NULL     | —           | ISO country code dari IP                                                     |

**Foreign Keys:**

  - user\_id → sec.user(id)

**Indexes:**

  - ix\_login\_user\_time (user\_id, login\_at DESC)

  - ix\_login\_status\_time (status, login\_at DESC)

  - ix\_login\_ip\_time (ip\_address, login\_at DESC)

  - ix\_login\_failed (status) WHERE status='FAILED'

**Partitioning: RANGE (login\_at) — monthly; retention 1 tahun online + 9 tahun cold**

# 11\. Schema sec — Security & RBAC (5 Entities)

Schema sec mengelola identity & access management. Mengikuti RBAC pattern dengan many-to-many user-role dan role-permission.

## 11.0 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> sec\_user }o--o{ sec\_role : "via sec\_user\_role"  
> sec\_role }o--o{ sec\_permission : "via sec\_role\_permission"  
> sec\_user ||--o{ sec\_session : "active sessions"  
> sec\_user\_role ||--o| sec\_user : "user"  
> sec\_user\_role ||--o| sec\_role : "role"  
> \`\`\`

### 11.1 sec.user

User account. Authentication via SSO (SAML 2.0/OIDC); password tidak di-store local — hanya cache info dari IDP.

*Primary Key: id*

| **Column**        | **Type**     | **Null** | **Default** | **Description**                               |
| ----------------- | ------------ | -------- | ----------- | --------------------------------------------- |
| id                | UUID         | NOT NULL | uuidv7()    | PK                                            |
| username          | VARCHAR(100) | NOT NULL | —           | Email atau username dari IDP                  |
| email             | VARCHAR(200) | NOT NULL | —           |                                               |
| full\_name        | VARCHAR(200) | NOT NULL | —           |                                               |
| display\_name     | VARCHAR(100) | NULL     | —           |                                               |
| unit\_kerja       | VARCHAR(100) | NULL     | —           | Direktorat / Divisi                           |
| jabatan           | VARCHAR(100) | NULL     | —           | Position                                      |
| mfa\_enrolled     | BOOLEAN      | NOT NULL | FALSE       |                                               |
| mfa\_method       | VARCHAR(20)  | NULL     | —           | TOTP/SMS/PUSH                                 |
| status            | VARCHAR(20)  | NOT NULL | 'AKTIF'     | AKTIF/TIDAK\_AKTIF/LOCKED/PENDING\_ACTIVATION |
| locked\_at        | TIMESTAMPTZ  | NULL     | —           |                                               |
| locked\_reason    | VARCHAR(100) | NULL     | —           |                                               |
| last\_login\_at   | TIMESTAMPTZ  | NULL     | —           |                                               |
| external\_idp\_id | VARCHAR(200) | NULL     | —           | ID dari Tugure AD                             |
| created\_at       | TIMESTAMPTZ  | NOT NULL | now()       |                                               |
| created\_by       | UUID         | NULL     | —           | Self-ref FK; null untuk system-init           |
| updated\_at       | TIMESTAMPTZ  | NULL     | —           |                                               |

**Foreign Keys:**

  - created\_by → sec.user(id) (self-ref)

**Indexes:**

  - uq\_user\_username UNIQUE(username)

  - uq\_user\_email UNIQUE(email)

  - ix\_user\_status (status)

  - ix\_user\_idp (external\_idp\_id) WHERE external\_idp\_id IS NOT NULL

### 11.2 sec.role

Role definition. Standar 10 role: Treasury Maker, Treasury Approver, Risk Officer, Akuntansi, Finance Controller, CFO, Auditor (Read-Only), IT Admin, Komite Investasi, ALCO Member.

*Primary Key: id*

| **Column**  | **Type**     | **Null** | **Default** | **Description**                             |
| ----------- | ------------ | -------- | ----------- | ------------------------------------------- |
| id          | UUID         | NOT NULL | uuidv7()    | PK                                          |
| role\_code  | VARCHAR(50)  | NOT NULL | —           | ROLE-MAKER-TR, ROLE-APPR-TR, ROLE-RISK, dll |
| nama\_role  | VARCHAR(200) | NOT NULL | —           |                                             |
| deskripsi   | TEXT         | NULL     | —           |                                             |
| aktif\_flag | BOOLEAN      | NOT NULL | TRUE        |                                             |
| created\_at | TIMESTAMPTZ  | NOT NULL | now()       |                                             |

**Indexes:**

  - uq\_role\_code UNIQUE(role\_code)

  - ix\_role\_aktif (aktif\_flag)

### 11.3 sec.permission

Permission catalog — granular per entity + action (mis. instrumen.create, ecl.calculate). Linked ke role via sec\_role\_permission junction.

*Primary Key: id*

| **Column**       | **Type**     | **Null** | **Default** | **Description**                                          |
| ---------------- | ------------ | -------- | ----------- | -------------------------------------------------------- |
| id               | UUID         | NOT NULL | uuidv7()    | PK                                                       |
| permission\_code | VARCHAR(100) | NOT NULL | —           | {entity}.{action}                                        |
| entity           | VARCHAR(50)  | NOT NULL | —           | instrumen, ecl, jurnal, dll                              |
| action           | VARCHAR(30)  | NOT NULL | —           | create/read/update/delete/approve/reject/override/export |
| deskripsi        | VARCHAR(500) | NULL     | —           |                                                          |
| created\_at      | TIMESTAMPTZ  | NOT NULL | now()       |                                                          |

**Indexes:**

  - uq\_permission\_code UNIQUE(permission\_code)

  - ix\_permission\_entity (entity)

### 11.4 sec.user\_role

Junction table user ↔ role. User dapat memiliki multiple roles; permission = union dari semua roles.

*Primary Key: id*

| **Column**   | **Type**    | **Null** | **Default** | **Description**                        |
| ------------ | ----------- | -------- | ----------- | -------------------------------------- |
| id           | UUID        | NOT NULL | uuidv7()    | PK                                     |
| user\_id     | UUID        | NOT NULL | —           | FK                                     |
| role\_id     | UUID        | NOT NULL | —           | FK                                     |
| assigned\_at | TIMESTAMPTZ | NOT NULL | now()       |                                        |
| assigned\_by | UUID        | NOT NULL | —           | IT Admin                               |
| expires\_at  | TIMESTAMPTZ | NULL     | —           | Optional expiry untuk temporary access |
| aktif\_flag  | BOOLEAN     | NOT NULL | TRUE        |                                        |

**Foreign Keys:**

  - user\_id → sec.user(id) ON DELETE CASCADE

  - role\_id → sec.role(id)

  - assigned\_by → sec.user(id)

**Indexes:**

  - uq\_user\_role UNIQUE(user\_id, role\_id) WHERE aktif\_flag=TRUE

  - ix\_user\_role\_user (user\_id) WHERE aktif\_flag=TRUE

  - ix\_user\_role\_role (role\_id) WHERE aktif\_flag=TRUE

### 11.5 sec.session

Active user sessions. Per token; concurrent session limit = 1 per user (force-logout previous saat new login).

*Primary Key: id*

| **Column**         | **Type**     | **Null** | **Default** | **Description**                                            |
| ------------------ | ------------ | -------- | ----------- | ---------------------------------------------------------- |
| id                 | UUID         | NOT NULL | uuidv7()    | PK                                                         |
| user\_id           | UUID         | NOT NULL | —           | FK                                                         |
| session\_token     | VARCHAR(500) | NOT NULL | —           | Hashed JWT atau session ID                                 |
| created\_at        | TIMESTAMPTZ  | NOT NULL | now()       |                                                            |
| expires\_at        | TIMESTAMPTZ  | NOT NULL | —           | absolute timeout 8 jam                                     |
| last\_activity\_at | TIMESTAMPTZ  | NOT NULL | now()       | Update setiap request; idle timeout 15 menit               |
| ip\_address        | INET         | NULL     | —           |                                                            |
| user\_agent        | VARCHAR(500) | NULL     | —           |                                                            |
| status             | VARCHAR(20)  | NOT NULL | 'ACTIVE'    | ACTIVE/EXPIRED/REVOKED                                     |
| revoked\_at        | TIMESTAMPTZ  | NULL     | —           |                                                            |
| revoke\_reason     | VARCHAR(100) | NULL     | —           | USER\_LOGOUT/IDLE\_TIMEOUT/CONCURRENT\_LOGIN/ADMIN\_REVOKE |

**Foreign Keys:**

  - user\_id → sec.user(id) ON DELETE CASCADE

**Indexes:**

  - uq\_session\_token UNIQUE(session\_token)

  - ix\_session\_user\_active (user\_id) WHERE status='ACTIVE'

  - ix\_session\_expires (expires\_at) WHERE status='ACTIVE'

**Notes:**

  - Cleanup job: setiap jam, mark sessions yang last\_activity\_at \> 15 menit ago sebagai EXPIRED.

  - Saat user login baru: revoke semua existing ACTIVE sessions (force concurrent=1).

# 12\. Schema sys — System Configuration (4 Entities)

Schema sys menyediakan system-wide configuration, lookup data, notification templates, dan job run tracking.

## 12.0 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> sys\_config {  
> VARCHAR config\_key PK  
> TEXT config\_value  
> }  
> sys\_lookup {  
> VARCHAR lookup\_group  
> VARCHAR lookup\_key  
> TEXT lookup\_value  
> }  
> sys\_notification\_template {  
> UUID id PK  
> VARCHAR template\_code  
> TEXT body\_template  
> }  
> sys\_job\_run\_history {  
> UUID id PK  
> VARCHAR job\_name  
> TIMESTAMPTZ started\_at  
> }  
> \`\`\`

### 12.1 sys.config

Key-value system configuration. Dipakai untuk parameter yang tidak frequently changed (mis. SMTP host, S3 bucket, JISDOR URL).

*Primary Key: id*

| **Column**    | **Type**     | **Null** | **Default** | **Description**           |
| ------------- | ------------ | -------- | ----------- | ------------------------- |
| id            | UUID         | NOT NULL | uuidv7()    | PK                        |
| config\_key   | VARCHAR(100) | NOT NULL | —           | smtp.host, s3.bucket, dll |
| config\_value | TEXT         | NOT NULL | —           |                           |
| config\_type  | VARCHAR(20)  | NOT NULL | —           | STRING/INT/BOOLEAN/JSON   |
| sensitive     | BOOLEAN      | NOT NULL | FALSE       | TRUE → masked di display  |
| description   | TEXT         | NULL     | —           |                           |
| category      | VARCHAR(50)  | NULL     | —           | Group untuk admin UI      |
| updated\_by   | UUID         | NULL     | —           | IT Admin                  |
| updated\_at   | TIMESTAMPTZ  | NULL     | —           |                           |

**Foreign Keys:**

  - updated\_by → sec.user(id)

**Indexes:**

  - uq\_config\_key UNIQUE(config\_key)

  - ix\_config\_category (category)

### 12.2 sys.lookup

Lookup tables untuk ENUM-like values yang perlu metadata tambahan (description, sort order, dll). Semua dropdown values di UI di-load dari sys.lookup.

*Primary Key: id*

| **Column**    | **Type**     | **Null** | **Default** | **Description**                                                                                         |
| ------------- | ------------ | -------- | ----------- | ------------------------------------------------------------------------------------------------------- |
| id            | UUID         | NOT NULL | uuidv7()    | PK                                                                                                      |
| lookup\_group | VARCHAR(50)  | NOT NULL | —           | TIPE\_INSTRUMEN (5: CASH, DEPOSITO, OBLIGASI, SAHAM, REKSADANA)/KLASIFIKASI\_PSAK71/RATING\_PEFINDO/dll |
| lookup\_key   | VARCHAR(50)  | NOT NULL | —           | OBLIGASI/AC/idAAA/GIRO/TABUNGAN/BERJANGKA/ON\_CALL/NEGARA/KORPORASI/SUKUK\_NEGARA/SUKUK\_KORPORASI/dll  |
| lookup\_value | VARCHAR(200) | NOT NULL | —           | Display label                                                                                           |
| description   | TEXT         | NULL     | —           |                                                                                                         |
| sort\_order   | INT          | NOT NULL | 0           |                                                                                                         |
| aktif\_flag   | BOOLEAN      | NOT NULL | TRUE        |                                                                                                         |
| metadata      | JSONB        | NULL     | —           | Extra attributes                                                                                        |
| created\_at   | TIMESTAMPTZ  | NOT NULL | now()       |                                                                                                         |

**Indexes:**

  - uq\_lookup\_group\_key UNIQUE(lookup\_group, lookup\_key)

  - ix\_lookup\_group\_aktif (lookup\_group, aktif\_flag, sort\_order)

### 12.3 sys.notification\_template

Email/notification template — Mustache atau Handlebars syntax untuk variable substitution.

*Primary Key: id*

| **Column**        | **Type**     | **Null** | **Default** | **Description**                                          |
| ----------------- | ------------ | -------- | ----------- | -------------------------------------------------------- |
| id                | UUID         | NOT NULL | uuidv7()    | PK                                                       |
| template\_code    | VARCHAR(50)  | NOT NULL | —           | APPROVAL\_REQUIRED/SICR\_TRIGGERED/CLOSING\_REMINDER/dll |
| channel           | VARCHAR(20)  | NOT NULL | —           | EMAIL/IN\_APP/SMS/WEBHOOK                                |
| subject\_template | VARCHAR(500) | NULL     | —           | Untuk EMAIL                                              |
| body\_template    | TEXT         | NOT NULL | —           | HTML untuk EMAIL; plain text lainnya                     |
| variables\_schema | JSONB        | NULL     | —           | Definisi variable yang expected                          |
| language          | VARCHAR(5)   | NOT NULL | 'id-ID'     | id-ID atau en-US                                         |
| aktif\_flag       | BOOLEAN      | NOT NULL | TRUE        |                                                          |
| updated\_at       | TIMESTAMPTZ  | NULL     | —           |                                                          |

**Indexes:**

  - uq\_notif\_template UNIQUE(template\_code, channel, language)

  - ix\_notif\_aktif (template\_code, aktif\_flag)

### 12.4 sys.job\_run\_history

Tracking semua batch job runs (MTM Harian, ECL Akhir Bulan, Akrual, BI JISDOR sync, dll). Untuk monitoring & re-perform audit.

*Primary Key: id*

| **Column**                 | **Type**     | **Null** | **Default** | **Description**                                              |
| -------------------------- | ------------ | -------- | ----------- | ------------------------------------------------------------ |
| id                         | UUID         | NOT NULL | uuidv7()    | PK                                                           |
| job\_name                  | VARCHAR(100) | NOT NULL | —           | daily\_mtm\_job/ecl\_monthly/akrual\_harian/bi\_jisdor\_sync |
| job\_type                  | VARCHAR(30)  | NOT NULL | —           | SCHEDULED/MANUAL/EVENT\_DRIVEN                               |
| triggered\_by              | UUID         | NULL     | —           | User jika MANUAL                                             |
| periode\_id                | UUID         | NULL     | —           | Untuk job per-period                                         |
| started\_at                | TIMESTAMPTZ  | NOT NULL | —           |                                                              |
| completed\_at              | TIMESTAMPTZ  | NULL     | —           |                                                              |
| status                     | VARCHAR(20)  | NOT NULL | 'RUNNING'   | RUNNING/SUCCESS/FAILED/CANCELLED/PARTIAL                     |
| records\_processed         | INT          | NULL     | 0           |                                                              |
| records\_failed            | INT          | NULL     | 0           |                                                              |
| error\_message             | TEXT         | NULL     | —           |                                                              |
| parameters\_snapshot\_json | JSONB        | NULL     | —           | Untuk audit re-perform — snapshot parameter version          |
| execution\_log\_url        | VARCHAR(500) | NULL     | —           | Link ke detailed log                                         |
| duration\_seconds          | INT          | NULL     | —           | \= completed\_at - started\_at                               |

**Foreign Keys:**

  - triggered\_by → sec.user(id)

  - periode\_id → mst.periode\_buku(id)

**Indexes:**

  - ix\_job\_run\_name\_time (job\_name, started\_at DESC)

  - ix\_job\_run\_status (status) WHERE status IN ('RUNNING','FAILED')

  - ix\_job\_run\_periode (periode\_id) WHERE periode\_id IS NOT NULL

**Partitioning: RANGE (started\_at) — monthly**

# 13\. Cross-Schema Relationships

## 13.1 Master ER Diagram (Full System)

Mermaid diagram menggambarkan relasi antar entitas major lintas schema. Untuk readability, hanya FK utama yang ditampilkan; audit FK ke sec.user di-skip.

> \`\`\`mermaid  
> erDiagram  
> %% Master Data (mst)  
> mst\_portofolio ||--o{ mst\_instrumen : "groupBM"  
> mst\_counterparty ||--o{ mst\_instrumen : "issuer"  
> mst\_counterparty ||--o{ mst\_instrumen : "MI/Kustodian"  
> mst\_counterparty ||--o{ mst\_rating\_history\_counterparty : "timeline"  
>   
> %% Klasifikasi & Compliance (sppi)  
> mst\_instrumen ||--o{ sppi\_sppi\_test : "tested"  
> mst\_portofolio ||--o{ sppi\_bm\_test : "tested"  
> mst\_instrumen ||--o{ sppi\_klasifikasi\_history : "klasifikasi"  
> mst\_instrumen ||--o{ sppi\_reklasifikasi\_log : "reklas"  
> sppi\_sppi\_test ||--|| sppi\_klasifikasi\_history : "input"  
> sppi\_bm\_test ||--|| sppi\_klasifikasi\_history : "input"  
> sppi\_reklasifikasi\_log ||--|| sppi\_klasifikasi\_history : "trigger"  
>   
> %% Transaction Lifecycle (trx)  
> mst\_instrumen ||--o{ trx\_penempatan : "create"  
> mst\_instrumen ||--o{ trx\_mtm : "daily"  
> mst\_instrumen ||--o{ trx\_renewal : "rollover"  
> mst\_instrumen ||--o{ trx\_penjualan : "sell"  
> mst\_instrumen ||--o{ trx\_jatuh\_tempo : "mature"  
> mst\_instrumen ||--o{ trx\_pendapatan\_akrual : "daily"  
> mst\_instrumen ||--o{ trx\_amortisasi : "monthly"  
> mst\_periode\_buku ||--o{ trx\_penempatan : "stamp"  
> mst\_periode\_buku ||--o{ trx\_mtm : "stamp"  
> mst\_periode\_buku ||--o{ trx\_pendapatan\_akrual : "stamp"  
>   
> %% ECL & EIR (ecl)  
> mst\_instrumen ||--o{ ecl\_calc\_header : "monthly"  
> ecl\_calc\_header ||--o{ ecl\_calc\_detail\_skenario : "3 skenario"  
> ecl\_calc\_header ||--o{ ecl\_lookthrough\_underlying : "RDN look-through"  
> mst\_instrumen ||--o{ ecl\_stage\_history : "stage timeline"  
> mst\_instrumen ||--o{ ecl\_eir\_amortization\_schedule : "AC/FVOCI debt"  
> ecl\_eir\_amortization\_schedule }o--o{ trx\_amortisasi : "post"  
> mst\_instrumen ||--o{ ecl\_eir\_reestimation\_log : "modifikasi/revisi"  
>   
> %% Jurnal (jrnl)  
> mst\_mapping\_jurnal\_header ||--o{ mst\_mapping\_jurnal\_detail : "lines template"  
> mst\_chart\_of\_accounts ||--o{ mst\_mapping\_jurnal\_detail : "akun"  
> mst\_chart\_of\_accounts ||--o{ jrnl\_detail : "akun"  
> jrnl\_header ||--o{ jrnl\_detail : "lines"  
> jrnl\_header ||--o| jrnl\_gl\_status : "delivery"  
> trx\_penempatan }o--o| jrnl\_header : "posting"  
> trx\_mtm }o--o| jrnl\_header : "posting"  
> ecl\_calc\_header }o--o| jrnl\_header : "posting"  
> ecl\_stage\_history }o--o| jrnl\_header : "STAGE\_MIGRATION"  
> ecl\_eir\_reestimation\_log }o--o| jrnl\_header : "EIR\_REESTIMATION"  
>   
> %% Document (doc)  
> doc\_upload ||--o{ doc\_link : "linked entities"  
> doc\_upload ||--o{ doc\_access\_log : "access log"  
> doc\_link }o..|| ALL : "polymorphic"  
>   
> %% Currency (mst)  
> mst\_mata\_uang ||--o{ mst\_kurs : "harian"  
> mst\_periode\_buku ||--o{ mst\_kurs : "stamp + lock"  
>   
> %% Audit (aud) — polymorphic  
> aud\_audit\_log }o..|| ALL : "every entity"  
> aud\_workflow\_history }o..|| ALL\_WORKFLOW\_ENTITIES : "transitions"  
>   
> %% Security (sec)  
> sec\_user }o--o{ sec\_role : "via user\_role"  
> sec\_role }o--o{ sec\_permission : "via role\_permission"  
> sec\_user ||--o{ sec\_session : "active"  
> sec\_user ||--o{ aud\_audit\_log : "actor"  
> \`\`\`

## 13.2 Critical Cross-Schema FK Dependencies

| **Source Table**                | **FK Column**          | **Target Table**                | **Cardinality**        | **ON DELETE**  |
| ------------------------------- | ---------------------- | ------------------------------- | ---------------------- | -------------- |
| sppi.sppi\_test                 | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| sppi.bm\_test                   | portofolio\_id         | mst.portofolio                  | N..1                   | RESTRICT       |
| sppi.reklasifikasi\_log         | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| trx.penempatan                  | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| trx.penempatan                  | periode\_id            | mst.periode\_buku               | N..1                   | RESTRICT       |
| trx.penempatan                  | akun\_sumber\_dana\_id | mst.instrumen                   | N..1 (where tipe=CASH) | RESTRICT       |
| trx.penempatan                  | jurnal\_header\_id     | jrnl.header                     | 1..0/1                 | SET NULL       |
| trx.mtm                         | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| trx.amortisasi                  | schedule\_periode\_id  | ecl.eir\_amortization\_schedule | N..1                   | RESTRICT       |
| ecl.calc\_header                | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| ecl.calc\_header                | periode\_id            | mst.periode\_buku               | N..1                   | RESTRICT       |
| ecl.calc\_detail\_skenario      | ecl\_calc\_header\_id  | ecl.calc\_header                | N..1                   | CASCADE        |
| ecl.lookthrough\_underlying     | ecl\_calc\_header\_id  | ecl.calc\_header                | N..1                   | CASCADE        |
| ecl.stage\_history              | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| ecl.eir\_amortization\_schedule | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| ecl.eir\_reestimation\_log      | instrumen\_id          | mst.instrumen                   | N..1                   | RESTRICT       |
| jrnl.header                     | periode\_id            | mst.periode\_buku               | N..1                   | RESTRICT       |
| jrnl.header                     | mapping\_header\_id    | mst.mapping\_jurnal\_header     | N..1                   | RESTRICT       |
| jrnl.detail                     | header\_id             | jrnl.header                     | N..1                   | CASCADE        |
| jrnl.detail                     | kode\_akun\_id         | mst.chart\_of\_accounts         | N..1                   | RESTRICT       |
| jrnl.gl\_status                 | header\_id             | jrnl.header                     | 1..1                   | CASCADE        |
| doc.link                        | doc\_upload\_id        | doc.upload                      | N..1                   | RESTRICT       |
| doc.link                        | entity\_id             | (polymorphic)                   | N..1                   | Manual cleanup |
| doc.access\_log                 | doc\_upload\_id        | doc.upload                      | N..1                   | CASCADE        |
| aud.audit\_log                  | actor\_user\_id        | sec.user                        | N..1                   | SET NULL       |
| aud.workflow\_history           | actor\_user\_id        | sec.user                        | N..1                   | RESTRICT       |
| aud.login\_history              | user\_id               | sec.user                        | N..1                   | SET NULL       |
| sec.user\_role                  | user\_id               | sec.user                        | N..1                   | CASCADE        |
| sec.user\_role                  | role\_id               | sec.role                        | N..1                   | RESTRICT       |
| sec.session                     | user\_id               | sec.user                        | N..1                   | CASCADE        |

## 13.3 Polymorphic FK Pattern

Tabel doc.link, aud.audit\_log, dan aud.workflow\_history menggunakan polymorphic FK pattern (entity\_type + entity\_id). Pattern ini fleksibel tetapi tidak dapat di-enforce di level database.

**Strategy untuk integrity:**

  - Application-level validation: setiap insert ke polymorphic table verify entity\_id exists di expected entity\_type table.

  - Database trigger BEFORE INSERT: lookup di pg\_class metadata untuk verify table existence, lalu dynamic SELECT untuk verify row existence.

  - Cleanup job: weekly orphan detection — record di polymorphic table dengan entity\_id yang sudah tidak ada di target table → flag untuk review.

  - Soft-delete pattern: jika entity di-soft-delete, polymorphic links tetap valid (refer ke is\_deleted=TRUE record).

# 14\. Data Dictionary (Alphabetical Reference)

Alphabetical reference untuk seluruh \~50 entitas dengan quick lookup ke schema, purpose, dan referensi FSD.

| **Entitas**                   | **Schema** | **Purpose**                                             | **FSD Ref**          |
| ----------------------------- | ---------- | ------------------------------------------------------- | -------------------- |
| audit\_log                    | aud        | Immutable audit trail untuk semua action material       | Master §3.3          |
| bm\_test                      | sppi       | Business Model Test result                              | App-A §3             |
| bobot\_skenario               | mst        | Bobot probability-weighted 3 skenario PD                | App-A §1.7, App-C §2 |
| calc\_detail\_skenario        | ecl        | Detail per skenario (Good/Normal/Bad)                   | App-C §7.2           |
| calc\_header                  | ecl        | Hasil ECL computation per instrumen per periode         | App-C §7.1           |
| chart\_of\_accounts           | mst        | CoA — struktur kode akun GL                             | App-D §3             |
| counterparty                  | mst        | Bank/Issuer/MI/Kustodian/Emiten registry                | App-A §1.2           |
| eir\_amortization\_schedule   | ecl        | Amortization schedule per instrumen AC/FVOCI debt       | App-C §9             |
| eir\_reestimation\_log        | ecl        | Log perubahan EIR (modifikasi/revisi CF)                | App-C §10            |
| impact\_mev\_pd               | mst        | Multiplier FL di tingkat INPUT (derive PD Good/Bad)     | App-C §2             |
| impact\_pd                    | mst        | Multiplier FL di tingkat OUTPUT (ECL Weighted → ECL FL) | App-C §2             |
| instrumen                     | mst        | CORE — Master Instrumen Investasi                       | App-A §1.1           |
| jatuh\_tempo                  | trx        | Closure pada tanggal JT (deposito/obligasi)             | App-B §5             |
| klasifikasi\_history          | sppi       | Timeline klasifikasi PSAK 71                            | App-A §4             |
| kurs                          | mst        | FX Rate Harian per mata uang                            | App-D §2             |
| lgd\_basel                    | mst        | LGD per tipe eksposur Basel III                         | App-A §1.4           |
| login\_history                | aud        | Login/logout activity tracking                          | Master §3.1          |
| lookthrough\_underlying       | ecl        | Look-through underlying untuk reksadana                 | App-C §5             |
| lps\_coverage                 | mst        | Nilai pertanggungan LPS (Rp 2 Miliar)                   | App-C §4             |
| mapping\_jurnal\_detail       | mst        | Detail line per event jurnal mapping                    | App-D §3             |
| mapping\_jurnal\_header       | mst        | Header template event jurnal                            | App-D §3             |
| mata\_uang                    | mst        | Master mata uang aktif (ISO 4217)                       | App-D §2             |
| mtm                           | trx        | Mark-to-Market harian per instrumen                     | App-B §2             |
| pd\_pefindo                   | mst        | PD 12-Month + Lifetime per rating                       | App-A §1.3           |
| pendapatan\_akrual            | trx        | Akrual harian bunga/kupon                               | App-B §6             |
| penempatan                    | trx        | Transaksi pembelian instrumen baru                      | App-B §1             |
| penjualan                     | trx        | Penjualan/pencairan instrumen                           | App-B §4             |
| periode\_buku                 | mst        | Periode akuntansi (BULANAN/TRIWULANAN/TAHUNAN)          | App-D §1             |
| portofolio                    | mst        | Grouping instrumen — unit BM Test                       | App-A §1.8           |
| rating\_history\_counterparty | mst        | Rating Pefindo timeline per counterparty                | App-A §1.2           |
| reklasifikasi\_log            | sppi       | Log reklasifikasi prospektif PSAK 71                    | App-A §5             |
| renewal                       | trx        | Renewal deposito jatuh tempo                            | App-B §3             |
| sppi\_test                    | sppi       | SPPI Test result (10 questions)                         | App-A §2             |
| stage\_history                | ecl        | Stage migration timeline (1/2/3)                        | App-C §3             |
| amortisasi                    | trx        | Amortisasi premium/diskonto posting                     | App-B §6, App-C §9   |
| audit\_log                    | aud        | Immutable audit trail                                   | Master §3.3          |
| doc\_access\_log              | doc        | Access log dokumen                                      | App-B §7             |
| doc\_link                     | doc        | Polymorphic link dokumen ↔ entity                       | App-B §7             |
| doc\_upload                   | doc        | Document registry + S3 metadata                         | App-B §7             |
| gl\_status                    | jrnl       | GL Host delivery tracking                               | App-D §3             |
| jrnl\_detail                  | jrnl       | Jurnal line items (D/K)                                 | App-D §3             |
| jrnl\_header                  | jrnl       | Jurnal posting header                                   | App-D §3             |
| job\_run\_history             | sys        | Batch job execution tracking                            | App-C §6             |
| sec\_permission               | sec        | Permission catalog (entity.action)                      | Master §3.1          |
| sec\_role                     | sec        | Role definition (10 standar)                            | Master §3.1          |
| sec\_session                  | sec        | Active sessions tracking                                | Master §3.1          |
| sec\_user                     | sec        | User accounts                                           | Master §3.1          |
| sec\_user\_role               | sec        | Junction user ↔ role                                    | Master §3.1          |
| sys\_config                   | sys        | Key-value configuration                                 | Master §9            |
| sys\_lookup                   | sys        | Lookup tables for ENUMs                                 | Master §9            |
| sys\_notification\_template   | sys        | Email/notification templates                            | Master §3.5          |
| workflow\_history             | aud        | State transition tracking                               | Master §3.4          |

# 15\. Indexing Strategy

## 15.1 Default Index Pattern

Setiap entitas memiliki standard index baseline:

  - PRIMARY KEY index (B-tree) — implicit pada id column.

  - FOREIGN KEY index — eksplisit pada setiap FK column (PostgreSQL tidak auto-create FK index).

  - UNIQUE constraint indexes — pada business unique keys (mis. kode\_instrumen, no\_transaksi, periode\_id\_kode).

  - Partial unique index — untuk uniqueness yang scoped (mis. WHERE is\_deleted=FALSE).

## 15.2 Performance-Driven Indexes

| **Index Type**                   | **Use Case**                                        | **Sample**                                                                                                     |
| -------------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Composite (multi-column)         | Frequent multi-column query                         | (periode\_id, instrumen\_id) di ecl.calc\_header; (counterparty\_id, tanggal\_berlaku DESC) di rating\_history |
| Partial (filtered)               | Subset query (status aktif, current version)        | WHERE is\_deleted=FALSE; WHERE periode\_berlaku\_sampai IS NULL; WHERE status='APPROVED'                       |
| Covering (INCLUDE)               | Avoid table lookup untuk frequent SELECT            | PostgreSQL: CREATE INDEX ... INCLUDE (col1, col2)                                                              |
| GIN (Generalized Inverted Index) | JSONB columns search                                | audit\_log.before\_value, after\_value (full-text + key search)                                                |
| BRIN (Block Range Index)         | Sequential timestamp untuk large append-only tables | audit\_log.timestamp untuk volume sangat besar                                                                 |
| Hash                             | Equality-only lookup (rarely used)                  | Tidak dipakai di production — B-tree lebih versatile                                                           |
| Functional                       | Index pada hasil expression                         | lower(email), date\_trunc('day', timestamp)                                                                    |
| Multicolumn DESC                 | Sorting                                             | (tanggal\_transaksi DESC) untuk recent-first queries                                                           |

## 15.3 Performance Baseline & Targets

| **Query Type**                  | **Target Latency (P95)** | **Strategy**                                     |
| ------------------------------- | ------------------------ | ------------------------------------------------ |
| Single record by PK             | ≤ 5ms                    | PK index B-tree — implicit                       |
| Master data lookup (cached)     | ≤ 1ms                    | Redis cache 1-jam TTL                            |
| Recent transactions (last 100)  | ≤ 50ms                   | Index (instrumen\_id, tanggal DESC)              |
| Posisi portofolio (semua aktif) | ≤ 200ms                  | Materialized view + index                        |
| ECL Detail (1 instrumen)        | ≤ 100ms                  | Index (instrumen\_id, periode\_id)               |
| Reporting (5-tahun history)     | ≤ 30 detik               | Materialized view + partitioning                 |
| Audit log search by entity      | ≤ 5 detik                | Index (entity\_type, entity\_id, timestamp DESC) |
| Audit log full-text search      | ≤ 10 detik               | GIN index pada JSONB                             |

# 16\. Partitioning Strategy

## 16.1 Tables yang Dipartition

| **Table**              | **Partition Key**                               | **Granularity** | **Retention**         | **Rationale**                           |
| ---------------------- | ----------------------------------------------- | --------------- | --------------------- | --------------------------------------- |
| aud.audit\_log         | RANGE (timestamp)                               | Monthly         | 10y online + 10y cold | Volume sangat tinggi, append-only       |
| aud.workflow\_history  | RANGE (timestamp)                               | Quarterly       | 10y                   | Moderate volume                         |
| aud.login\_history     | RANGE (login\_at)                               | Monthly         | 1y online + 9y cold   | High volume; rapid retention            |
| doc.access\_log        | RANGE (accessed\_at)                            | Monthly         | 10y                   | High volume saat audit period           |
| trx.pendapatan\_akrual | RANGE (tanggal\_akrual)                         | Yearly          | 10y                   | Daily records × instrumen — high volume |
| trx.amortisasi         | RANGE (tanggal\_posting)                        | Yearly          | 10y                   | Per kupon × instrumen                   |
| trx.mtm                | RANGE (tanggal\_valuasi)                        | Yearly          | 10y                   | Daily × instrumen — high volume         |
| jrnl.header            | RANGE (tanggal\_posting)                        | Yearly          | 10y                   | All postings                            |
| ecl.calc\_header       | LIST (periode\_id) atau RANGE (periode bulanan) | Monthly         | 10y                   | Aggregate per periode                   |
| sys.job\_run\_history  | RANGE (started\_at)                             | Monthly         | 5y                    | Operational tracking                    |

## 16.2 Partition Maintenance

Maintenance jobs untuk partitioned tables:

  - Monthly job: create new partition untuk bulan/tahun berikutnya (1 bulan ahead).

  - Monthly job: detach old partitions yang sudah expired retention; archive ke cold storage (S3 Glacier atau equivalent).

  - Quarterly job: VACUUM ANALYZE pada hot partitions untuk update statistics.

  - Annual job: review partition strategy — bila volume melonjak, increase granularity (yearly → quarterly).

  - Pre-create partition cookbook: stored procedure sp\_create\_partitions(table\_name, periode) untuk bulk creation.

## 16.3 Sample Partition DDL (PostgreSQL)

> \-- Parent table dengan PARTITION BY  
> CREATE TABLE aud.audit\_log (  
> id UUID NOT NULL DEFAULT uuidv7(),  
> entity\_type VARCHAR(50) NOT NULL,  
> entity\_id UUID NOT NULL,  
> action VARCHAR(30) NOT NULL,  
> actor\_user\_id UUID,  
> timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),  
> before\_value JSONB,  
> after\_value JSONB,  
> metadata JSONB,  
> PRIMARY KEY (id, timestamp) -- composite PK includes partition key  
> ) PARTITION BY RANGE (timestamp);  
>   
> \-- Monthly partitions  
> CREATE TABLE aud.audit\_log\_y2026m01 PARTITION OF aud.audit\_log  
> FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');  
>   
> CREATE TABLE aud.audit\_log\_y2026m02 PARTITION OF aud.audit\_log  
> FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');  
>   
> \-- Default partition untuk tanggal di luar range yang sudah ada  
> CREATE TABLE aud.audit\_log\_default PARTITION OF aud.audit\_log DEFAULT;  
>   
> \-- Index per partition  
> CREATE INDEX ix\_audit\_2026m01\_entity ON aud.audit\_log\_y2026m01 (entity\_type, entity\_id, timestamp DESC);

# 17\. Migration & Initial Seeding

## 17.1 Migration Strategy

Database schema migrations dikelola dengan tool versioning (Flyway, Liquibase, atau Sqitch). Approach:

10. Initial baseline migration V1\_\_init\_schema.sql — full DDL dari ERD ini.

11. Subsequent migrations versioned: V2\_\_add\_field\_xxx.sql, V3\_\_add\_table\_yyy.sql.

12. Each migration idempotent dan reversible (down migration tersedia).

13. Migration metadata tracked di table flyway\_schema\_history (atau tool equivalent).

14. Production migration via CI/CD pipeline dengan dry-run di UAT dulu.

## 17.2 Initial Seed Data

Setelah schema ter-create, seed data wajib di-insert:

**17.2.1 Reference Data (Static):**

  - sys.lookup — semua ENUM groups dengan values (TIPE\_INSTRUMEN, KLASIFIKASI\_PSAK71, RATING\_PEFINDO, dll).

  - mst.mata\_uang — IDR, USD, SGD, EUR, JPY, AUD, CNY, MYR, GBP.

  - mst.lgd\_basel — 4 baris default (Sovereign 0.4500, Senior Secured 0.2500, Senior Unsecured 0.4500, Subordinated 0.7500).

  - mst.bobot\_skenario — Good 0.2500, Normal 0.5000, Bad 0.2500.

  - mst.lps\_coverage — Rp 2.000.000.000.

  - mst.chart\_of\_accounts — minimal 30+ akun mengikuti contoh di SoW §5.1.9.

  - sec.role — 10 role standar.

  - sec.permission — \~50 permission codes.

  - sec.user\_role mappings (per role: bundle of permissions).

**17.2.2 Initial Configuration:**

  - sys.config — SMTP host, S3 bucket, BI JISDOR URL, Pefindo upload path, dll.

  - sys.notification\_template — semua template (APPROVAL\_REQUIRED, SICR\_TRIGGERED, CLOSING\_REMINDER, dll).

  - mst.mapping\_jurnal\_header + mapping\_jurnal\_detail — 20+ events default mapping.

  - mst.periode\_buku — 12 bulanan + 4 triwulanan + 1 tahunan untuk tahun current + tahun berikut.

  - Initial admin user (IT Admin) dengan password bootstrap (force change on first login).

**17.2.3 Pefindo PD Initial Upload:**

  - Risk Officer upload Pefindo Default Study latest (XLSX) → populate mst.pd\_pefindo.

  - Master Counterparty initial — minimal Pemerintah RI + 5-10 bank besar untuk mulai testing.

## 17.3 Migration dari Sistem Legacy

Bila ada data legacy yang perlu di-migrate dari sistem existing:

15. Data assessment: identifikasi tabel sumber + kualitas data + transformasi yang diperlukan.

16. Data cleansing: business user (Akuntansi, Treasury) review data di staging area dengan tools/spreadsheet.

17. ETL scripts: Python/SQL scripts untuk extract from legacy → transform → load ke BLIPS staging schema.

18. Validation: rekonsiliasi total saldo, jumlah instrumen, jumlah counterparty antara legacy dan BLIPS.

19. Sign-off oleh Akuntansi sebelum cut-over ke BLIPS production.

20. Parallel run: minimum 1 bulan operate kedua sistem; reconcile output bulanan.

21. Cut-over: stop input ke legacy, BLIPS go-live; legacy jadi read-only archive 5 tahun.

# 18\. Lampiran

## 18.1 Naming Conventions

| **Object**                | **Convention**                  | **Sample**                                    |
| ------------------------- | ------------------------------- | --------------------------------------------- |
| Schema name               | Lowercase, max 4 chars          | mst, trx, ecl, sppi, doc, jrnl, aud, sec, sys |
| Table name                | {schema}.{entity\_singular}     | mst.instrumen, trx.penempatan                 |
| Column                    | snake\_case lowercase           | kode\_instrumen, tanggal\_penempatan          |
| Boolean column            | {prefix}\_flag atau is\_{state} | auto\_renewal\_flag, is\_deleted              |
| Foreign key column        | {referenced\_table}\_id         | instrumen\_id, periode\_id                    |
| Primary key               | id (UUID)                       | id                                            |
| Composite PK column order | Most-selective first            | (id, timestamp) untuk partitioned             |
| Index                     | ix\_{table}\_{cols}             | ix\_instrumen\_klasifikasi                    |
| Unique constraint         | uq\_{table}\_{cols}             | uq\_kurs\_mata\_uang\_tanggal                 |
| Check constraint          | ck\_{table}\_{rule}             | ck\_jt\_after\_penempatan                     |
| Sequence                  | seq\_{table}\_{column}          | seq\_instrumen\_kode                          |
| Stored procedure          | sp\_{module}\_{action}          | sp\_ecl\_calculate\_monthly                   |
| Function                  | fn\_{purpose}                   | fn\_calculate\_idr\_equivalent                |
| View                      | vw\_{purpose}                   | vw\_portofolio\_position                      |
| Materialized view         | mvw\_{purpose}                  | mvw\_ckpn\_rollforward                        |
| Trigger                   | tg\_{table}\_{event}            | tg\_audit\_log\_no\_delete                    |
| Audit columns             | {role}\_by, {role}\_at          | created\_by, approved\_at                     |

## 18.2 Common Audit Columns Reference

Setiap tabel master/transactional WAJIB memiliki kolom audit standard (kecuali audit tables yang append-only):

| **Column**                   | **Type**         | **Wajib**   | **Description**                        |
| ---------------------------- | ---------------- | ----------- | -------------------------------------- |
| id                           | UUID             | Ya          | PK; uuidv7() time-ordered              |
| created\_by                  | UUID FK sec.user | Ya          | User yang create                       |
| created\_at                  | TIMESTAMPTZ      | Ya          | Default now()                          |
| updated\_by                  | UUID FK sec.user | Tidak       | User last update                       |
| updated\_at                  | TIMESTAMPTZ      | Tidak       | Auto-update via trigger                |
| approved\_by                 | UUID FK sec.user | Conditional | Wajib untuk transaksi require approval |
| approved\_at                 | TIMESTAMPTZ      | Conditional |                                        |
| status atau workflow\_status | VARCHAR(30)      | Ya          | DRAFT/PENDING\_\*/APPROVED/REJECTED    |
| version                      | INT              | Ya          | Optimistic locking; default 1          |
| is\_deleted                  | BOOLEAN          | Ya          | Soft delete; default FALSE             |
| deleted\_by                  | UUID FK sec.user | Tidak       | User yang soft-delete                  |
| deleted\_at                  | TIMESTAMPTZ      | Tidak       |                                        |
| delete\_reason               | TEXT             | Tidak       | Wajib jika is\_deleted=TRUE            |

## 18.3 Encryption Strategy untuk Sensitive Columns

Kolom sensitif yang WAJIB di-encrypt at column level:

  - mst.counterparty.npwp\_encrypted — NPWP counterparty (encrypted via pgcrypto).

  - Bank account numbers (jika di-store): nomor\_rekening\_encrypted.

  - Personal phone/email karyawan internal di sec.user — bila di-store di luar IDP.

**Implementation:**

> \-- Setup pgcrypto extension  
> CREATE EXTENSION IF NOT EXISTS pgcrypto;  
>   
> \-- Encryption function (key dari KMS atau env variable)  
> INSERT INTO mst.counterparty (npwp\_encrypted, ...)  
> VALUES (pgp\_sym\_encrypt('XX.XXX.XXX.X-XXX.XXX', current\_setting('app.encryption\_key')), ...);  
>   
> \-- Decryption (hanya untuk role yang have access)  
> SELECT pgp\_sym\_decrypt(npwp\_encrypted::bytea, current\_setting('app.encryption\_key'))  
> FROM mst.counterparty WHERE id = $1;

## 18.4 UUID v7 Generation Function

UUID v7 (time-ordered) memberikan benefits PK index efficiency tanpa kekurangan UUID v4 (random distribution causing index bloat).

> \-- PostgreSQL: function uuidv7()  
> \-- Implementation reference: github.com/dverite/postgres-uuidv7  
> \-- Atau gunakan extension uuidv7 jika tersedia  
>   
> \-- Alternatif menggunakan native gen\_random\_uuid() (UUID v4) dengan trade-off:  
> \-- Random UUIDs causing index pages fragmentation untuk large tables;  
> \-- v7 preferred untuk OLTP performance.

## 18.5 Common Triggers

Standard triggers yang di-attach ke tabel-tabel utama:

> \-- Trigger 1: Auto-update updated\_at  
> CREATE OR REPLACE FUNCTION fn\_update\_updated\_at()  
> RETURNS TRIGGER AS $$  
> BEGIN  
> NEW.updated\_at = now();  
> RETURN NEW;  
> END;  
> $$ LANGUAGE plpgsql;  
>   
> CREATE TRIGGER tg\_instrumen\_update\_updated\_at  
> BEFORE UPDATE ON mst.instrumen  
> FOR EACH ROW EXECUTE FUNCTION fn\_update\_updated\_at();  
>   
> \-- Trigger 2: Audit log auto-insert  
> CREATE OR REPLACE FUNCTION fn\_audit\_log\_insert()  
> RETURNS TRIGGER AS $$  
> BEGIN  
> INSERT INTO aud.audit\_log (  
> entity\_type, entity\_id, action, actor\_user\_id,  
> before\_value, after\_value, timestamp  
> ) VALUES (  
> TG\_TABLE\_SCHEMA || '.' || TG\_TABLE\_NAME,  
> COALESCE(NEW.id, OLD.id),  
> TG\_OP,  
> current\_setting('app.current\_user\_id', TRUE)::UUID,  
> CASE WHEN TG\_OP = 'DELETE' OR TG\_OP = 'UPDATE' THEN row\_to\_json(OLD)::JSONB END,  
> CASE WHEN TG\_OP = 'INSERT' OR TG\_OP = 'UPDATE' THEN row\_to\_json(NEW)::JSONB END,  
> now()  
> );  
> RETURN COALESCE(NEW, OLD);  
> END;  
> $$ LANGUAGE plpgsql;  
>   
> \-- Attach to material tables  
> CREATE TRIGGER tg\_instrumen\_audit  
> AFTER INSERT OR UPDATE OR DELETE ON mst.instrumen  
> FOR EACH ROW EXECUTE FUNCTION fn\_audit\_log\_insert();  
>   
> \-- Trigger 3: Audit log immutability  
> CREATE OR REPLACE FUNCTION fn\_audit\_no\_modify()  
> RETURNS TRIGGER AS $$  
> BEGIN  
> RAISE EXCEPTION 'Audit log records are immutable';  
> END;  
> $$ LANGUAGE plpgsql;  
>   
> CREATE TRIGGER tg\_audit\_log\_no\_update  
> BEFORE UPDATE OR DELETE ON aud.audit\_log  
> FOR EACH ROW EXECUTE FUNCTION fn\_audit\_no\_modify();  
>   
> \-- Trigger 4: Klasifikasi lock enforcement  
> CREATE OR REPLACE FUNCTION fn\_instrumen\_klasifikasi\_lock()  
> RETURNS TRIGGER AS $$  
> BEGIN  
> IF OLD.klasifikasi\_locked\_at IS NOT NULL  
> AND NEW.klasifikasi\_psak71 IS DISTINCT FROM OLD.klasifikasi\_psak71 THEN  
> RAISE EXCEPTION 'Klasifikasi PSAK 71 sudah locked; perubahan via Reklasifikasi';  
> END IF;  
> RETURN NEW;  
> END;  
> $$ LANGUAGE plpgsql;  
>   
> CREATE TRIGGER tg\_instrumen\_klasifikasi\_lock  
> BEFORE UPDATE ON mst.instrumen  
> FOR EACH ROW EXECUTE FUNCTION fn\_instrumen\_klasifikasi\_lock();

ERD ini adalah blueprint database untuk seluruh sistem BLIPS IFRS 9. Companion DDL script (BLIPS\_init\_schema.sql) tersedia ready-to-execute pada PostgreSQL 15+.

**Disusun oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Database Architect                                                   | IT Architect (Senior)                                                |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Direview oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Senior DBA                                                           | Vendor Implementor Lead                                              |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |
|                                                                      |                                                                      |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_     | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_     |
| IT Security Lead                                                     | PMO BLIPS                                                            |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Disetujui oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Direktur Teknologi Informasi                                         | Direktur Keuangan (CFO)                                              |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR DOKUMEN ERD ---*

# 12.A Schema sys — Upload Batch Tables (NEW v1.2)

Versi 1.2 menambahkan 2 tabel di schema sys untuk tracking bulk upload operations (MTM Harian dan Master Instrumen Bulk Import). Tabel ini menyediakan staging area dengan per-row validation status sebelum commit ke target schemas (trx, mst). Reference: SoW v1.3 §5.8.5 dan §5.8.6; FSD Appendix B v1.1 Bab 8-9.

## 12.A.1 ER Diagram

> \`\`\`mermaid  
> erDiagram  
> sys\_upload\_batch ||--o{ sys\_upload\_batch\_row : "header-detail"  
> sys\_upload\_batch }o--o| sec\_user : "uploader"  
> sys\_upload\_batch }o--o| sec\_user : "approver"  
> sys\_upload\_batch\_row }o..o| mst\_instrumen : "resolved (post-commit)"  
> sys\_upload\_batch\_row }o..o| trx\_mtm : "committed (MTM mode)"  
> \`\`\`

### 12.A.2 sys.upload\_batch

Header tabel untuk setiap bulk upload operation. Mendukung 2 batch\_type: MTM\_UPLOAD (Modul 5.8.5) dan INSTRUMEN\_BULK (Modul 5.8.6). Status tracking dari PARSING sampai COMMITTED atau ROLLED\_BACK.

*Primary Key: id*

| **Column**                 | **Type**     | **Null** | **Default** | **Description**                                                                                                               |
| -------------------------- | ------------ | -------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------- |
| id                         | UUID         | NOT NULL | uuidv7()    | PK                                                                                                                            |
| batch\_code                | VARCHAR(30)  | NOT NULL | —           | Auto-generate (BATCH-MTM-YYYY-MMDD-\#\#\#\#\#) atau (BATCH-INS-YYYY-\#\#\#\#\#)                                               |
| batch\_type                | VARCHAR(20)  | NOT NULL | —           | MTM\_UPLOAD / INSTRUMEN\_BULK / IMPACT\_MEV / PD\_PEFINDO / FUND\_FACT\_SHEET (extensible)                                    |
| batch\_mode                | VARCHAR(20)  | NULL     | —           | Untuk INSTRUMEN\_BULK: STANDARD / MIGRATION / TOPUP / DRY\_RUN; null untuk batch\_type lain                                   |
| filename\_original         | VARCHAR(500) | NOT NULL | —           | Original filename uploaded                                                                                                    |
| file\_sha256               | CHAR(64)     | NOT NULL | —           | Integrity check                                                                                                               |
| file\_storage\_url         | VARCHAR(500) | NOT NULL | —           | S3 reference path                                                                                                             |
| uploaded\_by               | UUID         | NOT NULL | —           | FK → sec.user                                                                                                                 |
| uploaded\_at               | TIMESTAMPTZ  | NOT NULL | now()       |                                                                                                                               |
| tanggal\_valuasi           | DATE         | NULL     | —           | Untuk MTM\_UPLOAD; null untuk INSTRUMEN\_BULK                                                                                 |
| portofolio\_target\_id     | UUID         | NULL     | —           | FK → mst.portofolio; default portofolio (INSTRUMEN\_BULK)                                                                     |
| total\_rows                | INT          | NOT NULL | 0           |                                                                                                                               |
| valid\_rows                | INT          | NOT NULL | 0           |                                                                                                                               |
| warning\_rows              | INT          | NOT NULL | 0           |                                                                                                                               |
| rejected\_rows             | INT          | NOT NULL | 0           |                                                                                                                               |
| committed\_rows            | INT          | NOT NULL | 0           | Final count yang berhasil commit                                                                                              |
| sheet\_breakdown\_json     | JSONB        | NULL     | —           | Stats per sheet untuk INSTRUMEN\_BULK                                                                                         |
| status                     | VARCHAR(30)  | NOT NULL | 'PARSING'   | PARSING / STAGED / PENDING\_REVIEW / PENDING\_APPROVAL / APPROVED / REJECTED / COMMITTING / COMMITTED / FAILED / ROLLED\_BACK |
| reviewer\_id               | UUID         | NULL     | —           | FK → sec.user                                                                                                                 |
| approver\_id               | UUID         | NULL     | —           | FK → sec.user                                                                                                                 |
| approved\_at               | TIMESTAMPTZ  | NULL     | —           |                                                                                                                               |
| rejected\_at               | TIMESTAMPTZ  | NULL     | —           |                                                                                                                               |
| reject\_reason             | TEXT         | NULL     | —           |                                                                                                                               |
| committed\_at              | TIMESTAMPTZ  | NULL     | —           |                                                                                                                               |
| committed\_instrumen\_ids  | UUID\[\]     | NULL     | —           | Array IDs yang berhasil di-INSERT (INSTRUMEN\_BULK)                                                                           |
| rollback\_status           | VARCHAR(20)  | NULL     | —           | PENDING\_ROLLBACK / ROLLED\_BACK                                                                                              |
| rollback\_by               | UUID         | NULL     | —           | FK → sec.user (CFO only)                                                                                                      |
| rollback\_at               | TIMESTAMPTZ  | NULL     | —           |                                                                                                                               |
| rollback\_reason           | TEXT         | NULL     | —           | Wajib jika rollback\_status SET                                                                                               |
| error\_summary\_json       | JSONB        | NULL     | —           | Top-level errors untuk display                                                                                                |
| processing\_metadata\_json | JSONB        | NULL     | —           | Runtime stats: parse\_duration\_ms, validation\_duration\_ms, commit\_duration\_ms                                            |

**Foreign Keys:**

  - uploaded\_by, reviewer\_id, approver\_id, rollback\_by → sec.user(id)

  - portofolio\_target\_id → mst.portofolio(id)

**Indexes:**

  - uq\_upload\_batch\_code UNIQUE(batch\_code)

  - ix\_upload\_batch\_type\_status (batch\_type, status)

  - ix\_upload\_batch\_uploader (uploaded\_by, uploaded\_at DESC)

  - ix\_upload\_batch\_committed (committed\_at DESC) WHERE status='COMMITTED'

  - ix\_upload\_batch\_rollback (rollback\_status) WHERE rollback\_status IS NOT NULL

  - ck\_batch\_type CHECK (batch\_type IN ('MTM\_UPLOAD','INSTRUMEN\_BULK','IMPACT\_MEV','PD\_PEFINDO','FUND\_FACT\_SHEET'))

  - ck\_batch\_status CHECK (status IN ('PARSING','STAGED','PENDING\_REVIEW','PENDING\_APPROVAL','APPROVED','REJECTED','COMMITTING','COMMITTED','FAILED','ROLLED\_BACK'))

**Notes:**

  - Trigger BEFORE INSERT: validate sheet\_breakdown\_json schema sesuai batch\_type.

  - Tabel ini bersifat semi-immutable — UPDATE hanya untuk status transitions; jangan UPDATE filename\_original / file\_sha256 setelah create.

  - Soft delete tidak applicable; record permanent untuk audit.

  - Retensi: 10 tahun online + cold storage untuk compliance.

### 12.A.3 sys.upload\_batch\_row

Detail per-row dari upload batch. Setiap baris di Excel/CSV → satu record di sini dengan validation status. Setelah commit, link ke target entity (instrumen atau trx.mtm) ter-set.

*Primary Key: id*

| **Column**                       | **Type**      | **Null** | **Default** | **Description**                                                                                                                                                                                                                                                                                             |
| -------------------------------- | ------------- | -------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| id                               | UUID          | NOT NULL | uuidv7()    | PK                                                                                                                                                                                                                                                                                                          |
| batch\_id                        | UUID          | NOT NULL | —           | FK → sys.upload\_batch ON DELETE CASCADE                                                                                                                                                                                                                                                                    |
| row\_number                      | INT           | NOT NULL | —           | Urutan dalam file (Excel row number)                                                                                                                                                                                                                                                                        |
| sheet\_name                      | VARCHAR(50)   | NULL     | —           | Untuk INSTRUMEN\_BULK: CASH/DEPOSITO/OBLIGASI/SAHAM/REKSADANA                                                                                                                                                                                                                                               |
| row\_data\_json                  | JSONB         | NOT NULL | —           | Snapshot data row dari Excel (semua kolom)                                                                                                                                                                                                                                                                  |
| instrumen\_id                    | UUID          | NULL     | —           | Resolved dari ISIN/Kode (MTM\_UPLOAD) atau auto-generated (INSTRUMEN\_BULK)                                                                                                                                                                                                                                 |
| sumber\_harga                    | VARCHAR(30)   | NULL     | —           | Untuk MTM: IBPA/NAB\_MI/NAB\_KSEI/BEI/MANUAL                                                                                                                                                                                                                                                                |
| harga\_native                    | NUMERIC(15,4) | NULL     | —           | Untuk MTM                                                                                                                                                                                                                                                                                                   |
| harga\_sebelumnya                | NUMERIC(15,4) | NULL     | —           | Untuk MTM                                                                                                                                                                                                                                                                                                   |
| deviation\_pct                   | NUMERIC(8,4)  | NULL     | —           | (harga - sebelumnya) / sebelumnya × 100                                                                                                                                                                                                                                                                     |
| status\_validation               | VARCHAR(50)   | NOT NULL | 'PENDING'   | PENDING / VALID / WARNING\_PRICE\_DEVIATION / WARNING\_DUPLICATE / WARNING\_FK\_FUZZY / REJECTED\_FK\_MISSING / REJECTED\_REQUIRED\_FIELD / REJECTED\_BUSINESS\_RULE / REJECTED\_CURRENCY\_MISMATCH / REJECTED\_INSTRUMEN\_TIDAK\_DITEMUKAN / REJECTED\_KURS\_TIDAK\_TERSEDIA / REJECTED\_DUPLICATE\_POSTED |
| validation\_errors\_json         | JSONB         | NULL     | —           | Array of error objects {stage, field, value, message, severity}                                                                                                                                                                                                                                             |
| validation\_warnings\_json       | JSONB         | NULL     | —           | Array of warning objects                                                                                                                                                                                                                                                                                    |
| preview\_master\_instrumen\_json | JSONB         | NULL     | —           | Untuk INSTRUMEN\_BULK preview: kode\_instrumen, klasifikasi, eir\_awal\_preview                                                                                                                                                                                                                             |
| override\_flag                   | BOOLEAN       | NOT NULL | FALSE       | TRUE jika maker override warning                                                                                                                                                                                                                                                                            |
| override\_reason                 | TEXT          | NULL     | —           | Wajib jika override\_flag=TRUE                                                                                                                                                                                                                                                                              |
| override\_by                     | UUID          | NULL     | —           | FK → sec.user                                                                                                                                                                                                                                                                                               |
| override\_at                     | TIMESTAMPTZ   | NULL     | —           |                                                                                                                                                                                                                                                                                                             |
| committed\_to\_instrumen\_id     | UUID          | NULL     | —           | FK → mst.instrumen setelah commit (INSTRUMEN\_BULK)                                                                                                                                                                                                                                                         |
| committed\_to\_mtm\_id           | UUID          | NULL     | —           | FK → trx.mtm setelah commit (MTM\_UPLOAD)                                                                                                                                                                                                                                                                   |
| status\_commit                   | VARCHAR(30)   | NOT NULL | 'PENDING'   | PENDING / COMMITTED / SKIPPED / FAILED                                                                                                                                                                                                                                                                      |
| commit\_error                    | TEXT          | NULL     | —           | Error message jika commit FAILED                                                                                                                                                                                                                                                                            |
| created\_at                      | TIMESTAMPTZ   | NOT NULL | now()       |                                                                                                                                                                                                                                                                                                             |
| updated\_at                      | TIMESTAMPTZ   | NULL     | —           |                                                                                                                                                                                                                                                                                                             |

**Foreign Keys:**

  - batch\_id → sys.upload\_batch(id) ON DELETE CASCADE

  - instrumen\_id → mst.instrumen(id)

  - override\_by → sec.user(id)

  - committed\_to\_instrumen\_id → mst.instrumen(id)

  - committed\_to\_mtm\_id → trx.mtm(id)

**Indexes:**

  - uq\_batch\_row UNIQUE(batch\_id, row\_number)

  - ix\_batch\_row\_status (batch\_id, status\_validation)

  - ix\_batch\_row\_instrumen (instrumen\_id) WHERE instrumen\_id IS NOT NULL

  - ix\_batch\_row\_committed (committed\_to\_instrumen\_id) WHERE committed\_to\_instrumen\_id IS NOT NULL

  - ix\_batch\_row\_override (override\_flag) WHERE override\_flag=TRUE

**Notes:**

  - row\_data\_json menyimpan snapshot semua kolom Excel — penting untuk audit re-perform dan untuk re-validation setelah parameter master berubah.

  - Polymorphic commit target: committed\_to\_instrumen\_id (INSTRUMEN\_BULK) atau committed\_to\_mtm\_id (MTM\_UPLOAD).

  - Status transitions one-way: PENDING → VALID/WARNING/REJECTED → COMMITTED (kecuali REJECTED yang stay).

  - Untuk INSTRUMEN\_BULK MIGRATION mode: klasifikasi\_psak71 dari row data dipakai langsung; tidak trigger SPPI/BM workflow.

# Sign-Off Page
