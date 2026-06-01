*\[ LOGO TUGURE \]*

**FUNCTIONAL SPECIFICATION DOCUMENT**

**BLIPS IFRS 9 — MASTER DOCUMENT**

*Cross-Cutting Concerns • Architecture • Standards*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**           | **Keterangan**                                                        |
| --------------------- | --------------------------------------------------------------------- |
| Judul Dokumen         | Functional Specification Document (FSD) — Master Cross-Cutting        |
| Kode Dokumen          | FSD-BLIPS-MASTER-2026-001                                             |
| Versi                 | 1.1                                                                   |
| Status                | DRAFT FOR REVIEW                                                      |
| Tanggal Terbit        | 02 Mei 2026                                                           |
| Bahasa                | Bahasa Indonesia (technical terms in English)                         |
| Klasifikasi Informasi | INTERNAL — CONFIDENTIAL                                               |
| Pemilik Dokumen       | Direktorat Teknologi Informasi (Architect Lead) + Working Group BLIPS |
| Penyusun              | IT Architect, Lead Developer, Solution Designer                       |
| Reviewer              | PMO, Working Group, Vendor Implementor                                |
| Approver              | Direktur Teknologi Informasi + CFO selaku Sponsor                     |
| Reference Upstream    | BRD v1.1; SoW v1.3; Decision Log v1.0 (29 Mei 2026)                   |
| Reference Downstream  | FSD Appendix A-E; ERD v1.2; BLIPS\_init\_schema.sql v1.2              |

# Revision History

| **Versi** | **Tanggal** | **Penyusun**      | **Reviewer**        | **Ringkasan Perubahan**                                |
| --------- | ----------- | ----------------- | ------------------- | ------------------------------------------------------ |
| 0.1       | 12 Apr 2026 | Solution Designer | —                   | Initial draft skeleton.                                |
| 0.5       | 22 Apr 2026 | IT Architect      | PMO + Working Group | Add architecture, cross-cutting concerns, integration. |
| 0.9       | 29 Apr 2026 | IT Architect      | Vendor + DevOps     | Refine API standards, error handling, UI/UX.           |
| 1.0       | 02 Mei 2026 | IT Architect      | Direktur IT + CFO   | Final draft for review.                                |

# Reference Documents

FSD Master mengacu kepada dokumen upstream dan menjadi anchor untuk dokumen downstream:

| **Tipe**   | **Kode**                 | **Judul**                                         | **Versi**          |
| ---------- | ------------------------ | ------------------------------------------------- | ------------------ |
| Upstream   | BRD-BLIPS-IFRS9-2026-001 | Business Requirements Document                    | v1.0               |
| Upstream   | SOW-BLIPS-IFRS9-2026-001 | Scope of Work & Flow                              | v1.1               |
| Downstream | FSD-APP-A                | FSD Appendix A — Master Data + SPPI/BM Test       | v1.0               |
| Downstream | FSD-APP-B                | FSD Appendix B — Transaction Lifecycle            | v1.0               |
| Downstream | FSD-APP-C                | FSD Appendix C — ECL Engine + EIR & Amortisasi    | v1.0               |
| Downstream | FSD-APP-D                | FSD Appendix D — Periode Buku, FX, Mapping Jurnal | v1.0               |
| Downstream | FSD-APP-E                | FSD Appendix E — Reporting & Dashboard            | v1.0               |
| Downstream | ERD-BLIPS-2026           | Entity Relationship Diagram                       | v1.0 (TBD)         |
| Downstream | API-SPEC-BLIPS           | API Specifications (OpenAPI 3.0/Swagger)          | v1.0 (TBD)         |
| Standard   | PSAK-71                  | PSAK 71 Instrumen Keuangan                        | Berlaku 1 Jan 2020 |
| Standard   | ISO-27001                | Information Security Management                   | ISO/IEC 27001:2022 |
| Standard   | OWASP-ASVS               | OWASP Application Security Verification Standard  | v4.0.3             |

# 1\. Pendahuluan

## 1.1 Tujuan Dokumen

Functional Specification Document (FSD) Master ini menetapkan spesifikasi teknis lintas-modul (cross-cutting concerns) untuk sistem BLIPS IFRS 9 Instrumen Investasi. FSD Master menjadi anchor untuk lima FSD Appendix per kelompok modul, sehingga konvensi teknis (arsitektur, security, error handling, API, UI/UX, database) di-standardisasi di satu tempat dan dirujuk konsisten oleh seluruh appendix.

FSD merupakan downstream BRD (yang menetapkan WHAT — kebutuhan bisnis) dan downstream SoW (yang menetapkan WHAT level teknis — modul, formula, field). FSD menjawab HOW — bagaimana setiap kebutuhan dibangun secara teknis. Output FSD menjadi input untuk Solution Architecture, Code Implementation, dan UAT Scripts.

## 1.2 Audiens Dokumen

  - Vendor Implementor — sebagai spec lengkap untuk delivery.

  - Internal Developer Team Tugure — untuk co-development & customization.

  - IT Architect & DevOps — untuk infrastructure provisioning & deployment.

  - Quality Assurance Team — untuk test case design.

  - Security & Audit Team — untuk security review & audit trail design.

  - Working Group BLIPS — untuk validasi solusi teknis vs business requirement.

## 1.3 Hubungan dengan Dokumen Lain

| **Dokumen**              | **Posisi** | **Hubungan dengan FSD Master**                                                                                                             |
| ------------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| BRD v1.0                 | Upstream   | FSD Master men-translate BR-XXX-\#\#\# dari BRD ke spec teknis. Setiap section FSD memiliki referensi ke BRD.                              |
| SoW v1.1                 | Upstream   | SoW menetapkan formula, field, dan flow. FSD menambah HOW (algoritma implementasi, API, UI).                                               |
| FSD Appendix A-E         | Downstream | Per-modul detail. Appendix MUST conform pada konvensi yang ditetapkan FSD Master.                                                          |
| ERD                      | Parallel   | Database design lengkap. FSD Master memberikan schema strategy; ERD memberikan detail per tabel.                                           |
| API Spec (Swagger)       | Parallel   | Detail per endpoint. FSD Master memberikan API standards (auth, pagination, error format); Swagger memberikan detail per request/response. |
| Test Cases / UAT Scripts | Downstream | TC mengikuti requirement di BRD + spec teknis di FSD.                                                                                      |
| Operational Runbook      | Downstream | Procedural document untuk Ops team berdasarkan FSD.                                                                                        |

## 1.4 Konvensi Notasi & Terminologi

Notasi yang digunakan dalam FSD Master & Appendices:

  - BR-XXX-\#\#\# → Business Requirement ID dari BRD (mis. BR-EIR-001).

  - FR-XXX-\#\#\# → Functional Requirement ID di FSD (lebih granular dari BR; satu BR dapat memiliki multiple FR).

  - UC-XXX-\#\#\# → Use Case ID dari BRD bab 11.

  - ERR-XXX-\#\#\#\# → Error Code (lihat Bab 8.2 untuk taxonomy).

  - API endpoint format: METHOD /api/v1/{resource} (mis. POST /api/v1/instrumen).

  - Field naming: snake\_case untuk DB column (mis. kode\_instrumen); camelCase untuk JSON property (mis. kodeInstrumen).

  - Date format: ISO 8601 (YYYY-MM-DD untuk date; YYYY-MM-DDTHH:MM:SSZ untuk timestamp UTC).

  - Currency: IDR equivalent untuk semua perhitungan internal; native currency untuk display & audit trail.

  - WAJIB / SHOULD / OPSIONAL — RFC 2119 priority levels.

## 1.5 Struktur FSD (Master + 5 Appendices)

FSD BLIPS terdiri dari 6 dokumen yang saling terkait:

| **Dokumen**      | **Cakupan**                                                                                                                      | **Modul Tercakup**                                                                      |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| FSD Master (ini) | Cross-cutting concerns: arsitektur, security, performance, audit, workflow, integrasi, API/UI/error standards, common reference. | Semua modul (foundation)                                                                |
| FSD Appendix A   | Master Data + SPPI Test + Business Model Test                                                                                    | Modul 1, 2                                                                              |
| FSD Appendix B   | Transaction Lifecycle (lifecycle instrumen end-to-end)                                                                           | Modul 3, 4, 5, 6, 7, 8, 9 (Penempatan, MTM, Renewal, Penjualan, JT, Pendapatan, Upload) |
| FSD Appendix C   | ECL Engine + EIR & Amortisasi (compliance core)                                                                                  | Modul 10, 11 (ECL, EIR)                                                                 |
| FSD Appendix D   | Periode Buku + FX Rate + Mapping Jurnal & GL Interface                                                                           | Modul 12, 13, 14                                                                        |
| FSD Appendix E   | Reporting & Dashboard (25+ laporan)                                                                                              | Modul 15, 16                                                                            |

# 2\. Architecture Overview

## 2.1 Logical Architecture (3-Tier)

Sistem BLIPS dirancang dengan arsitektur 3-tier yang ter-separasi secara logis:

| **Tier**              | **Komponen**                                                                                                                       | **Tanggung Jawab**                                                                                 |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Presentation (Web)    | Web UI (SPA — React/Vue/Angular), API Gateway                                                                                      | User interaction; routing API calls; SSO redirect; static asset serving                            |
| Application (Backend) | REST API services, Workflow Engine, Batch Job Scheduler, Notification Service, Document Service                                    | Business logic execution; orchestration; integration with external systems; transaction management |
| Data (Persistence)    | OLTP Database (PostgreSQL/Oracle/SQL Server), OLAP/Reporting DB read-replica, Object Storage (S3+KMS) untuk dokumen, Cache (Redis) | Data persistence; ACID transactions; reporting queries; document blob storage; session/cache       |

**Karakteristik utama arsitektur:**

  - Stateless application tier — memungkinkan horizontal scaling melalui load balancer.

  - Async processing — batch job (MTM harian, ECL akhir bulan, akrual harian, EIR amortization) melalui job queue (RabbitMQ/Kafka/equivalent).

  - Read-replica DB — reporting query tidak mempengaruhi OLTP performance.

  - Cache layer — Redis untuk master data lookup, session, dan API rate limiting.

  - Document storage terpisah dari RDBMS — S3-compatible dengan encryption-at-rest (KMS).

  - API Gateway — single entry point dengan auth, rate limiting, request logging, dan routing.

## 2.2 Recommended Technology Stack

Berikut technology stack rekomendasi. Final stack dapat di-review berdasarkan vendor preference, license cost, dan existing skills di Tugure. Yang dipersyaratkan: open standard, mature ecosystem, dan vendor support di Indonesia.

| **Layer**        | **Component**              | **Rekomendasi**                                                          | **Alternatif**                                   |
| ---------------- | -------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------ |
| Frontend         | Framework SPA              | Next.js 14+ (React 18, App Router, TypeScript) — LOCKED                  | —                                                |
| Frontend         | UI Component Library       | shadcn/ui (Radix UI + Tailwind) — LOCKED                                 | Ant Design (jika prefer enterprise)              |
| Frontend         | State Management           | Zustand — LOCKED (simpler untuk module independence)                     | Redux Toolkit (jika perlu time-travel debug)     |
| Frontend         | Form Handling              | React Hook Form + Zod — LOCKED                                           | —                                                |
| Frontend         | Charting (untuk dashboard) | Recharts — LOCKED                                                        | Apache ECharts                                   |
| Backend          | Language & Framework       | Golang 1.22+ + Gin atau Fiber HTTP router — LOCKED                       | —                                                |
| Backend          | ORM                        | GORM v2 untuk standard CRUD + sqlx untuk complex reporting — LOCKED      | —                                                |
| Backend          | Build/Dependency           | Go modules (go.mod) — native                                             | —                                                |
| Database         | OLTP RDBMS                 | PostgreSQL 18 — LOCKED                                                   | —                                                |
| Database         | OLAP / Reporting           | PostgreSQL 18 Read-Replica — LOCKED                                      | Materialized view scheduled refresh              |
| Database         | Cache                      | Redis 7+ — LOCKED                                                        | —                                                |
| Document Storage | Object Storage             | MinIO (S3-compatible on-premise) — LOCKED                                | —                                                |
| Job Queue        | Message Broker             | Asynq (Go-native Redis-based) — LOCKED untuk Phase 1                     | Temporal untuk complex workflow Phase 2          |
| Job Scheduler    | Batch Scheduler            | Asynq scheduler — LOCKED                                                 | Cron untuk simple jobs                           |
| Auth/IDP         | Identity Provider          | Keycloak on-premise + LDAP federation ke Tugure AD — LOCKED              | Custom SAML adapter bila Keycloak tidak feasible |
| Auth Protocol    | Authentication             | OAuth 2.0 + OIDC via Keycloak — LOCKED                                   | SAML 2.0 untuk legacy SSO compatibility          |
| API Gateway      | Gateway                    | Traefik on-premise — LOCKED                                              | Kong, Nginx                                      |
| Monitoring       | APM                        | Prometheus + Grafana — LOCKED                                            | —                                                |
| Logging          | Log Aggregation            | Loki + Grafana — LOCKED                                                  | ELK Stack alternatif                             |
| CI/CD            | Pipeline                   | GitLab CI self-hosted — LOCKED                                           | Jenkins                                          |
| IaC              | Infrastructure             | Terraform + Ansible — LOCKED                                             | —                                                |
| Container        | Orchestration              | Docker + Docker Compose (dev/UAT); Kubernetes on-premise (prod) — LOCKED | —                                                |

## 2.3 Deployment Topology

Deployment topology untuk produksi:

| **Environment**         | **Lokasi**                                                                | **Tujuan**                            | **SLA**                |
| ----------------------- | ------------------------------------------------------------------------- | ------------------------------------- | ---------------------- |
| Production (Active)     | Data Center Primary Tugure (Jakarta) — ON-PREMISE LOCKED                  | Live transaction processing           | 99,9% uptime / 24x7    |
| Production (DR Passive) | Data Center Secondary (mis. Surabaya/Cikarang) atau Cloud Region failover | Disaster recovery; async replication  | RTO ≤ 4 jam            |
| UAT                     | Same region as Prod, isolated environment                                 | User acceptance testing               | Business hours support |
| SIT                     | Cloud or shared environment                                               | System integration testing            | Working hours support  |
| Development             | Cloud or local                                                            | Development & unit testing            | Best effort            |
| DR Drill                | DR site occasional activation                                             | Quarterly tabletop, annual full drill | —                      |

Catatan data residency: SOW v1.1 dan BRD §12.2 C-05 mensyaratkan data produksi di on-premise atau cloud Indonesia (untuk pemenuhan UU PDP transboundary). Vendor cloud yang dapat dipertimbangkan: AWS Jakarta, Azure Indonesia (saat tersedia), GCP Jakarta, Telkomsigma, Lintasarta, Biznet Gio Cloud.

## 2.4 Network Architecture

Network segmentation untuk security & compliance:

| **Zone**     | **Komponen**                                 | **Akses Inbound**                                              | **Akses Outbound**                             |
| ------------ | -------------------------------------------- | -------------------------------------------------------------- | ---------------------------------------------- |
| DMZ (Public) | Load Balancer, WAF, API Gateway              | HTTPS (443) dari internet user; HTTPS dari partner integration | Ke App Tier (HTTPS internal)                   |
| App Tier     | Backend services, Workflow, Batch, Job Queue | Hanya dari API Gateway / DMZ                                   | Ke DB Tier, Cache, External APIs (whitelisted) |
| DB Tier      | OLTP DB, OLAP DB, Cache, Object Storage      | Hanya dari App Tier                                            | Tidak ada outbound (kecuali backup ke offsite) |
| Management   | Monitoring, Logging, CI/CD, IaC bastion      | VPN + MFA                                                      | Ke semua tier untuk monitoring                 |

**Security controls lintas zone:**

  - Firewall rules WAJIB whitelist-based (default deny).

  - TLS 1.2+ untuk semua komunikasi inter-zone.

  - VLAN segregation antar tier.

  - WAF rules: OWASP Top 10 protection, rate limiting, IP geo-block (allow Indonesia + whitelisted partner).

  - DDoS protection di edge.

  - Egress proxy untuk outbound calls ke internet (Pefindo, BI, IBPA).

## 2.5 High-Level Component Diagram (Textual)

Berikut representasi tekstual dari arsitektur. Diagram visual akan ditambahkan di Solution Architecture Document (terpisah).

> ┌─────────────────────────────────────────────────────────────┐  
> │ EXTERNAL USERS / SYSTEMS │  
> │ Treasury │ Risk │ Akuntansi │ Komite │ Auditor │ GL Host │  
> └──────────────────────┬──────────────────────────────────────┘  
> │ HTTPS (TLS 1.3)  
> ┌──────────────────────▼──────────────────────────────────────┐  
> │ DMZ — LOAD BALANCER + WAF │  
> │ (DDoS Protection, OWASP Rules) │  
> └──────────────────────┬──────────────────────────────────────┘  
> │  
> ┌──────────────────────▼──────────────────────────────────────┐  
> │ API GATEWAY │  
> │ (Auth via OAuth/SAML, Rate Limit, Request Log, Routing) │  
> └──┬─────────────┬───────────────┬──────────────┬─────────────┘  
> │ │ │ │  
> │ /api/v1/ │ /api/v1/ │ /api/v1/ │ /api/v1/  
> │ master │ transaction │ ecl-eir │ reporting  
> │ │ │ │  
> ┌──▼─────┐ ┌──▼───────┐ ┌──▼─────────┐ ┌─▼──────────┐  
> │ Master │ │Transaction│ │ ECL/EIR │ │ Reporting │  
> │ Data │ │ Service │ │ Engine │ │ Service │  
> │ Service│ │ │ │ Service │ │ │  
> └──┬─────┘ └──┬───────┘ └──┬─────────┘ └─┬──────────┘  
> │ │ │ │  
> └─────┬───────┴───────────────┴──────────────┘  
> │  
> ┌─────▼─────────────────────────────────────────┐  
> │ MESSAGE BROKER (RabbitMQ/Kafka) │  
> │ (Async Jobs: MTM, Akrual, ECL, Amortization) │  
> └─────┬──────────────────────────────────────────┘  
> │  
> ┌─────▼──────────┐ ┌──────────────────┐  
> │ Batch Worker │ │ Notification │  
> │ Pool │ │ Service (Email) │  
> └─────┬──────────┘ └────────┬─────────┘  
> │ │  
> ┌─────▼─────────────────────────▼──────────┐  
> │ DATABASE & CACHE LAYER │  
> │ ┌─────────┐ ┌─────────┐ ┌──────────┐ │  
> │ │ OLTP │ │ Read │ │ Redis │ │  
> │ │ DB │◀│ Replica │ │ Cache │ │  
> │ └─────────┘ └─────────┘ └──────────┘ │  
> │ ┌──────────────────────────────────┐ │  
> │ │ S3-Compatible Object Storage │ │  
> │ │ (Documents, encrypted KMS) │ │  
> │ └──────────────────────────────────┘ │  
> └───────────────────────────────────────────┘  
>   
> External Integrations (via Egress Proxy):  
> \- GL Host (REST API atau file batch)  
> \- Pefindo Rating (manual upload)  
> \- IBPA Bond Pricing (file feed harian)  
> \- BI JISDOR (scheduled scrape/API)  
> \- KSEI/MI NAB (manual upload)  
> \- BEI Closing Price (file feed harian)  
> \- LDAP/AD (SSO)  
> \- SMTP (notification email)

# 3\. Cross-Cutting Concerns

## 3.1 Security Architecture

### 3.1.1 Authentication

Sistem WAJIB mengimplementasikan SSO via SAML 2.0 atau OAuth 2.0/OpenID Connect, terintegrasi dengan Active Directory Tugure.

| **FR-ID**  | **Spec**             | **Detail**                                                                                                                                                                              |
| ---------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| FR-SEC-001 | Login Flow           | User akses URL BLIPS → redirect ke IDP (Tugure AD) → login dengan AD credentials → IDP issue JWT/SAML token → redirect kembali ke BLIPS dengan token → validasi token oleh API Gateway. |
| FR-SEC-002 | Token Type           | JWT (JSON Web Token) signed dengan RSA 2048-bit. Expiry: access token 15 menit; refresh token 8 jam (absolute timeout).                                                                 |
| FR-SEC-003 | MFA Enforcement      | Wajib MFA (TOTP/SMS/Push notification) untuk role: Treasury Manager, Finance Controller, CFO, Komite Investasi, ALCO, CEO.                                                              |
| FR-SEC-004 | Session Management   | Idle timeout 15 menit (auto-logout). Concurrent session limit = 1 per user (force-logout session lama).                                                                                 |
| FR-SEC-005 | Failed Login Lockout | 5 failed attempts → account locked 30 menit; admin notification setelah 3 lockouts dalam 1 jam.                                                                                         |
| FR-SEC-006 | Password Reset       | Self-service via email link (token expire dalam 30 menit) + MFA verification.                                                                                                           |

### 3.1.2 Authorization (RBAC)

Role-Based Access Control dengan minimum 7 base role; permission granular per modul dan per action.

| **Role ID**   | **Role Name**       | **Tipe Akses**                                        | **Contoh Permission**                                          |
| ------------- | ------------------- | ----------------------------------------------------- | -------------------------------------------------------------- |
| ROLE-MAKER-TR | Treasury Maker      | Input transaksi, upload dokumen, request approval     | instrumen.create, transaksi\_penempatan.create, dokumen.upload |
| ROLE-APPR-TR  | Treasury Approver   | Approve/reject transaksi maker                        | transaksi.approve, transaksi.reject (CANNOT input as maker)    |
| ROLE-RISK     | Risk Officer        | Master parameter PD/LGD/MEV, rating, ECL review       | rating\_history.create, ecl\_parameter.update                  |
| ROLE-AKUN     | Akuntansi           | Posting jurnal, periode buku, FX rate, mapping        | periode\_buku.close, fx\_rate.upload, mapping\_jurnal.update   |
| ROLE-AKUN-CTL | Finance Controller  | Approve adjustment, soft-close approver               | periode\_buku.softclose\_approve, jurnal\_adjustment.approve   |
| ROLE-CFO      | CFO                 | Hard-close approver, parameter critical, override     | periode\_buku.hardclose\_approve, parameter\_master.override   |
| ROLE-AUDIT    | Auditor (Read-Only) | View transactions, audit trail, dokumen               | \* (read-only); audit\_trail.view                              |
| ROLE-IT-ADMIN | IT Admin            | User management, role assignment, system config       | user.create, role.assign (CANNOT access financial data)        |
| ROLE-KOMITE   | Komite Investasi    | Approve klasifikasi, FVOCI Election, override SPPI/BM | klasifikasi\_psak71.approve, fvoci\_election.approve           |
| ROLE-ALCO     | ALCO Member         | Approve ECL parameter, bobot skenario                 | ecl\_param.approve, mev\_impact.approve                        |

**Permission model:**

  - Permission format: {entity}.{action} (mis. instrumen.create, ecl.calculate).

  - Action types: create, read, update, delete, approve, reject, override, export.

  - Wildcard: \* = all (hanya untuk Auditor dengan read-only).

  - Combination: User dapat memiliki multiple roles; permission = union dari semua roles.

  - Segregation of Duty (SoD): system-level enforcement — User yang sudah Maker pada transaksi tidak bisa Approve transaksi yang sama (cek user\_id transaksi vs user\_id approver).

### 3.1.3 Encryption

| **Aspek**                   | **Standard**                         | **Implementasi**                                                                                                                   |
| --------------------------- | ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| At Rest — DB                | AES-256                              | Transparent Data Encryption (TDE) di PostgreSQL/Oracle/SQL Server. Master key di KMS (cloud) atau HSM (on-prem). Rotated annually. |
| At Rest — Document          | AES-256 (server-side)                | S3 SSE-KMS atau equivalent. Per-file encryption dengan unique data key wrapped by KMS master key.                                  |
| At Rest — Backups           | AES-256                              | Backup encrypted dengan separate key dari production. Stored offsite.                                                              |
| At Rest — Sensitive Columns | AES-256 column-level                 | PII fields: NPWP, nomor rekening, encrypted at column level dengan separate key. Decryption hanya untuk role tertentu.             |
| In Transit — Eksternal      | TLS 1.3 (preferred), TLS 1.2 minimum | Semua HTTPS endpoint. Cipher suites: hanya yang AEAD (AES-GCM, ChaCha20-Poly1305). HSTS enforced.                                  |
| In Transit — Internal       | TLS 1.2+ atau mTLS                   | Service-to-service di internal network juga TLS encrypted (defense-in-depth).                                                      |
| In Transit — DB             | TLS                                  | Aplikasi ke DB connection via TLS.                                                                                                 |
| Key Management              | KMS / HSM                            | Hierarchical: Master Key (KMS) → Data Encryption Keys (DEK) per resource. DEK envelope-encrypted oleh master key.                  |
| Hash for Integrity          | SHA-256                              | Digunakan untuk dokumen upload integrity check (lihat Bab 3.5).                                                                    |
| Password Hashing            | Argon2id (atau bcrypt cost ≥ 12)     | Untuk password yang di-store; salt unik per user.                                                                                  |

### 3.1.4 Sensitive Data Handling

  - PII fields (NPWP, no rekening, alamat, no telepon): masked di UI untuk role tanpa hak (mis. xxxxx1234); full visibility untuk Treasury, Akuntansi, Auditor.

  - Audit log untuk setiap akses ke field sensitif (view, export).

  - Export data dengan PII WAJIB melalui workflow approval (request → approve → time-bound download link).

  - Data retention: PII data dihapus 5 tahun setelah counterparty inactive (atau sesuai regulasi terkini).

  - Right to be forgotten (UU PDP): mekanisme untuk anonimisasi data subject jika diminta.

## 3.2 Performance Optimization

| **Strategy**            | **Penjelasan**                                                       | **Aplikasi di BLIPS**                                                                                 |
| ----------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Caching (Redis)         | Master data lookup di-cache; TTL 1 jam dengan invalidation on update | Master Instrumen, Counterparty, Rating PD, LGD, Periode Buku, Kurs Tengah hari ini                    |
| Async Processing        | Operasi non-real-time via job queue                                  | MTM Harian, Akrual Bunga, ECL Akhir Bulan, EIR Amortization, Notification, Document Upload virus scan |
| Read Replica            | Reporting query di-route ke read-replica DB                          | Semua dashboard & report (Bab 8.15 BRD)                                                               |
| Query Optimization      | Index pada FK, kombinasi (entity, periode\_id), partitioning         | Audit trail by date partitions; transaksi by periode\_bulan; rating\_history by counterparty\_id      |
| Pagination Default      | Default page size 20; max 100; cursor-based untuk audit log besar    | Semua list endpoint                                                                                   |
| Lazy Loading            | Heavy nested relationships di-load on demand                         | Master Instrumen detail page: SPPI/BM Test details lazy-loaded                                        |
| Compression             | gzip/brotli untuk HTTP response \> 1 KB                              | API Gateway level                                                                                     |
| CDN                     | Static assets di CDN edge                                            | Frontend bundle, images, fonts                                                                        |
| Connection Pooling      | DB connection pool: 20-50 per instance                               | HikariCP (Java) atau equivalent                                                                       |
| Batch Processing Window | Batch jobs di luar peak hour                                         | Akrual Bunga & MTM end-of-day; ECL akhir bulan: H+1 setelah cut-off                                   |

## 3.3 Audit Trail (Immutable)

Audit trail merupakan critical compliance feature; design WAJIB ensure immutability:

| **Aspek**             | **Spec**                                                                                                                                                                                         |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Storage               | Append-only table audit\_log dengan WORM (Write-Once-Read-Many) characteristic. Bisa dipakai PostgreSQL dengan trigger BEFORE UPDATE/DELETE yang block, atau immutable storage seperti AWS QLDB. |
| Trigger               | Setiap action material auto-create audit log entry: CREATE/UPDATE/DELETE/APPROVE/REJECT/EXPORT/LOGIN/PARAMETER\_CHANGE.                                                                          |
| Schema                | audit\_log\_id (PK), entity\_type, entity\_id, action, actor\_user\_id, actor\_role, timestamp, ip\_address, user\_agent, before\_value (JSONB), after\_value (JSONB), metadata (JSONB).         |
| Retention             | 10 tahun online + cold storage 10 tahun (sesuai PSAK retention requirement). Total 20 tahun.                                                                                                     |
| Access                | Read-only untuk Auditor role; tidak ada user (termasuk DBA) yang dapat modify audit\_log entries.                                                                                                |
| Indexing              | Index pada (entity\_type, entity\_id), (actor\_user\_id, timestamp), (action, timestamp).                                                                                                        |
| Searchability         | Full-text search across audit\_log via Elasticsearch atau PostgreSQL FTS.                                                                                                                        |
| Hash Chain (optional) | Tiap audit\_log entry include hash dari entry sebelumnya → tamper detection (blockchain-like).                                                                                                   |

## 3.4 Workflow Engine (Maker-Reviewer-Approver)

Workflow engine men-orchestrate seluruh transaksi material yang memerlukan multi-eyes approval. Standard: 4-eyes (Maker-Approver) untuk transaksi rutin; 6-eyes (Maker-Reviewer-Approver) untuk klasifikasi PSAK 71 dan parameter master.

| **FR-ID** | **Spec**                        | **Detail**                                                                                                                                                                          |
| --------- | ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| FR-WF-001 | Workflow Definition             | Workflow di-define per transaction type. Configurable di sistem (tidak hardcoded). Each workflow: states, transitions, role assignments, deadlines.                                 |
| FR-WF-002 | States                          | Standard states: DRAFT, PENDING\_REVIEW, PENDING\_APPROVAL, APPROVED, REJECTED, EXPIRED, REVOKED.                                                                                   |
| FR-WF-003 | Transitions                     | Maker submit → PENDING\_REVIEW (untuk 6-eyes) atau PENDING\_APPROVAL (untuk 4-eyes). Reviewer approve → PENDING\_APPROVAL. Reviewer reject → DRAFT (back to maker dengan komentar). |
| FR-WF-004 | Role Assignment                 | Each step assigned ke role(s); user dengan role tersebut dapat claim & action. Untuk Komite Investasi: minimal 3 of 5 members harus approve (configurable quorum).                  |
| FR-WF-005 | Segregation of Duty Enforcement | Sistem WAJIB cek: maker\_user\_id ≠ reviewer\_user\_id ≠ approver\_user\_id. Bila violation → block transition dengan error ERR-WF-001.                                             |
| FR-WF-006 | Deadline & Escalation           | Each step memiliki SLA (mis. Reviewer 2 hari kerja; Approver 3 hari kerja). Bila terlewat → notification escalation ke superior + flag URGENT.                                      |
| FR-WF-007 | Comment/Justification           | Reject WAJIB include komentar; Override WAJIB include justifikasi tertulis.                                                                                                         |
| FR-WF-008 | Audit Trail                     | Setiap state transition tercatat di workflow\_history table dengan actor, timestamp, dari/ke state, comment.                                                                        |
| FR-WF-009 | Bulk Action                     | Approver dapat bulk-approve multiple items dengan single action (mis. approve 50 mapping changes); each item tetap individually traceable.                                          |
| FR-WF-010 | In-flight Modification          | Setelah submitted, Maker tidak boleh edit kecuali transition ke DRAFT (via Reviewer reject).                                                                                        |

## 3.5 Notification Service

| **Channel**        | **Use Case**                                                                               | **Spec**                                                                           |
| ------------------ | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| Email (SMTP)       | Approval pending; deadline reminder; closing milestone; SICR/Default trigger; system alert | Template-based (HTML); branding Tugure; from no-reply@tugu-re.com; bounce handling |
| In-App             | Real-time notification badge di header; notification center                                | WebSocket atau Server-Sent Events; unread counter; mark-as-read; archive           |
| Webhook (Optional) | Integrasi ke chat (Slack/Teams) untuk specific event critical                              | Configurable per event type                                                        |
| SMS (Optional)     | Critical alert untuk CFO (mis. periode hard-close fail)                                    | Via SMS gateway provider Indonesia                                                 |

**Notification settings:**

  - User-configurable preferences: per event type bisa pilih channel & frekuensi (real-time / digest harian / off).

  - Default settings per role (mis. CFO default email + in-app + SMS untuk critical).

  - Quiet hours: notification non-critical paused di luar jam kerja kecuali user enable.

  - Audit log untuk delivery: sent timestamp, delivered/bounced/failed, retry count.

## 3.6 Internationalization (i18n)

  - Primary language: Bahasa Indonesia. Secondary: English (untuk reporting eksternal & auditor).

  - Language toggle di top navigation; saved per user profile.

  - Translation keys di JSON file per language; key naming: {module}.{section}.{key} (mis. master.instrumen.title).

  - Date format: id-ID locale (DD/MM/YYYY) atau ISO (YYYY-MM-DD); user configurable.

  - Number format: id-ID (1.000.000,50) atau International (1,000,000.50); user configurable.

  - Currency display: Rp (Indonesian) atau IDR (international).

# 4\. Database Design Overview

## 4.1 Schema Strategy

Database schema BLIPS mengikuti pattern OLTP (Online Transaction Processing) untuk operasional, dengan optional read-replica untuk OLAP/Reporting. Untuk reporting yang kompleks (mis. ECL Detail, Roll-Forward), dipakai materialized views atau dedicated reporting schema.

| **Schema**          | **Tujuan**               | **Tabel Utama**                                                                                                                                                                                                                     |
| ------------------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| mst (Master)        | Master/reference data    | mst\_instrumen, mst\_counterparty, mst\_rating\_pefindo\_pd, mst\_lgd\_basel, mst\_periode\_buku, mst\_mata\_uang, mst\_kurs, mst\_chart\_of\_accounts, mst\_mapping\_jurnal\_header, mst\_mapping\_jurnal\_detail, mst\_portofolio |
| trx (Transaksi)     | Transactional data       | trx\_penempatan, trx\_mtm, trx\_renewal, trx\_penjualan, trx\_jatuh\_tempo, trx\_pendapatan\_akrual, trx\_amortisasi                                                                                                                |
| ecl (ECL/EIR)       | Compliance computation   | ecl\_calc\_header, ecl\_calc\_detail\_skenario, ecl\_stage\_history, eir\_amortization\_schedule, eir\_reestimation\_log, ecl\_lookthrough\_underlying                                                                              |
| sppi (SPPI/BM Test) | Klasifikasi              | sppi\_test, bm\_test, klasifikasi\_psak71\_history, reklasifikasi\_log                                                                                                                                                              |
| doc (Document)      | Document & file metadata | doc\_upload, doc\_link, doc\_access\_log                                                                                                                                                                                            |
| jrnl (Jurnal)       | Posted journal entries   | jrnl\_header, jrnl\_detail, jrnl\_gl\_status (delivery to GL host)                                                                                                                                                                  |
| aud (Audit)         | Audit trail (immutable)  | audit\_log, workflow\_history, login\_history                                                                                                                                                                                       |
| sec (Security)      | Security/RBAC            | sec\_user, sec\_role, sec\_permission, sec\_user\_role, sec\_session                                                                                                                                                                |
| sys (System)        | Configuration & lookup   | sys\_config, sys\_lookup, sys\_notification\_template, sys\_job\_run\_history                                                                                                                                                       |

## 4.2 Naming Conventions

| **Object**        | **Convention**                                        | **Contoh**                                                        |
| ----------------- | ----------------------------------------------------- | ----------------------------------------------------------------- |
| Schema            | Lowercase, max 4 karakter (mst, trx, ecl, dst)        | mst, trx, ecl, sppi                                               |
| Table             | {schema}.{entity}\_singular ATAU plural per kebijakan | mst.instrumen, trx.penempatan, ecl.calc\_header                   |
| Column            | snake\_case, lowercase                                | kode\_instrumen, tanggal\_penempatan                              |
| PK                | id (auto-generate via UUID atau bigserial)            | id                                                                |
| FK                | {referenced\_table}\_id                               | instrumen\_id, counterparty\_id, periode\_id                      |
| Index             | ix\_{table}\_{column(s)}                              | ix\_penempatan\_kode\_instrumen, ix\_audit\_log\_actor\_timestamp |
| Unique constraint | uq\_{table}\_{column(s)}                              | uq\_kurs\_kode\_tanggal                                           |
| Check constraint  | ck\_{table}\_{rule}                                   | ck\_penempatan\_jt\_after\_penempatan                             |
| Sequence          | seq\_{table}\_{column}                                | seq\_instrumen\_kode (untuk auto-generate kode)                   |
| Stored procedure  | sp\_{module}\_{action}                                | sp\_ecl\_calculate\_monthly, sp\_eir\_compute                     |
| Function          | fn\_{purpose}                                         | fn\_calculate\_idr\_equivalent, fn\_lookup\_pd                    |
| View              | vw\_{purpose}                                         | vw\_portofolio\_position, vw\_ecl\_summary                        |
| Materialized view | mvw\_{purpose}                                        | mvw\_ckpn\_rollforward, mvw\_stage\_distribution                  |
| Trigger           | tg\_{table}\_{event}                                  | tg\_audit\_log\_no\_delete                                        |

## 4.3 Common Columns (Audit Fields)

Setiap tabel transaksi/master WAJIB memiliki kolom audit standard berikut:

| **Column**     | **Type**                      | **Wajib**          | **Keterangan**                                                                             |
| -------------- | ----------------------------- | ------------------ | ------------------------------------------------------------------------------------------ |
| id             | UUID atau BIGSERIAL           | Ya                 | Primary key. UUID v7 (time-ordered) untuk performance.                                     |
| created\_by    | UUID FK ke sec\_user          | Ya                 | User yang create record.                                                                   |
| created\_at    | TIMESTAMPTZ                   | Ya                 | Timestamp UTC create.                                                                      |
| updated\_by    | UUID FK ke sec\_user          | Tidak              | User yang last update; null bila belum pernah update.                                      |
| updated\_at    | TIMESTAMPTZ                   | Tidak              | Timestamp last update.                                                                     |
| approved\_by   | UUID FK ke sec\_user          | Tidak              | User approver (untuk transaksi yang require approval).                                     |
| approved\_at   | TIMESTAMPTZ                   | Tidak              | Timestamp approve.                                                                         |
| status         | VARCHAR(30)                   | Ya                 | Status workflow (DRAFT, PENDING\_\*, APPROVED, REJECTED, EXPIRED).                         |
| version        | INT                           | Ya                 | Optimistic locking version. Default 1.                                                     |
| periode\_id    | UUID FK ke mst\_periode\_buku | Conditional        | Untuk transaksi yang ter-stamp ke periode buku tertentu.                                   |
| is\_deleted    | BOOLEAN                       | Ya (default FALSE) | Soft delete flag. Hard delete tidak diperbolehkan untuk audit; gunakan is\_deleted = TRUE. |
| deleted\_by    | UUID FK ke sec\_user          | Tidak              | User yang soft-delete.                                                                     |
| deleted\_at    | TIMESTAMPTZ                   | Tidak              | Timestamp soft-delete.                                                                     |
| delete\_reason | TEXT                          | Tidak              | Alasan soft-delete (wajib bila is\_deleted = TRUE).                                        |

**Catatan penting:**

  - Hard delete TIDAK DIPERBOLEHKAN untuk semua tabel transaksi & master kecuali sys\_lookup yang bersifat ephemeral.

  - Untuk audit trail (audit\_log, workflow\_history): tidak ada is\_deleted; record permanent.

  - Soft delete WAJIB justification (delete\_reason); untuk master dengan dependency, sistem block soft-delete jika ada FK aktif.

## 4.4 Indexing Strategy

| **Index Type**       | **Use Case**                            | **Contoh**                                                                                            |
| -------------------- | --------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Primary Key (B-tree) | Default untuk id column                 | id pada semua tabel                                                                                   |
| Unique Index         | Business unique constraint              | (kode\_mata\_uang, tanggal) pada mst\_kurs                                                            |
| Foreign Key Index    | Setiap FK column wajib punya index      | instrumen\_id, counterparty\_id, periode\_id                                                          |
| Composite Index      | Multi-column query pattern              | (periode\_id, status) pada trx\_penempatan; (counterparty\_id, tanggal\_berlaku) pada rating\_history |
| Partial Index        | Filter on subset (mis. status aktif)    | WHERE is\_deleted = FALSE; WHERE status = 'APPROVED'                                                  |
| GIN Index            | JSONB columns (audit\_log before/after) | audit\_log.before\_value, after\_value                                                                |
| Timestamp/Date Index | Range query untuk reporting             | tanggal\_transaksi, created\_at                                                                       |
| Covering Index       | Frequent SELECT dengan beberapa kolom   | INCLUDE clause untuk avoid table lookup                                                               |

**Performance baseline:**

  - Query untuk dashboard real-time: ≤ 200ms (P95) — pakai cache + read-replica + index.

  - Reporting query (5-tahun history): ≤ 30 detik (P95) — pakai materialized view + partitioning.

  - Audit log search: ≤ 5 detik (P95) untuk filter berdasarkan entity\_id.

## 4.5 Partitioning Strategy

| **Tabel**                   | **Partitioning Key**                            | **Granularity** | **Retention**                  |
| --------------------------- | ----------------------------------------------- | --------------- | ------------------------------ |
| audit\_log                  | RANGE (timestamp)                               | Bulanan         | 10 tahun online + cold storage |
| workflow\_history           | RANGE (timestamp)                               | Triwulanan      | 10 tahun                       |
| trx\_pendapatan\_akrual     | RANGE (tanggal\_akrual)                         | Tahunan         | 10 tahun                       |
| trx\_amortisasi             | RANGE (tanggal\_posting)                        | Tahunan         | 10 tahun                       |
| trx\_mtm                    | RANGE (tanggal\_valuasi)                        | Tahunan         | 10 tahun                       |
| doc\_access\_log            | RANGE (timestamp)                               | Bulanan         | 10 tahun                       |
| jrnl\_header                | RANGE (tanggal\_posting)                        | Tahunan         | 10 tahun                       |
| ecl\_calc\_detail\_skenario | LIST (periode\_id) atau RANGE (periode bulanan) | Bulanan         | 10 tahun                       |

*Rationale:*

  - Tabel transaksi berukuran besar di-partition untuk performance query historical & maintenance window.

  - Old partitions (\>5 tahun) dapat di-archive ke cold storage (S3 Glacier atau equivalent) untuk cost optimization.

  - Partitioning juga memudahkan retention policy (drop old partitions setelah expired).

## 4.6 Data Retention & Archival

| **Tipe Data**         | **Retention Online**               | **Retention Cold Storage** | **Total Retention**     |
| --------------------- | ---------------------------------- | -------------------------- | ----------------------- |
| Master Data Aktif     | Permanent (selama instrumen aktif) | —                          | Permanent               |
| Master Data Inactive  | 5 tahun setelah inactive           | 5 tahun                    | 10 tahun                |
| Transaksi             | 5 tahun                            | 5 tahun                    | 10 tahun                |
| Audit Trail           | 10 tahun                           | 10 tahun                   | 20 tahun                |
| Documents (S3)        | 10 tahun                           | —                          | 10 tahun (sesuai pajak) |
| Login/Session History | 1 tahun                            | 9 tahun                    | 10 tahun                |
| System Logs           | 90 hari                            | 9 bulan                    | 1 tahun                 |
| Notification Logs     | 1 tahun                            | —                          | 1 tahun                 |

# 5\. Integration Architecture

## 5.1 Integration Overview

BLIPS mengintegrasikan multiple sistem eksternal & internal. Setiap integrasi WAJIB:

  - Authenticated dengan credential yang aman (API key, OAuth token, mTLS).

  - Logged dengan request/response (excluded sensitive data).

  - Resilient — retry mechanism dengan exponential backoff; circuit breaker untuk dependency yang sering down.

  - Idempotent — repeat calls aman, tidak menghasilkan duplicate transactions.

  - Monitored — APM tracking latency, error rate, throughput per integrasi.

| **Integration**       | **Direction**       | **Mode**                                    | **Frekuensi**             | **Failure Mode**                                            |
| --------------------- | ------------------- | ------------------------------------------- | ------------------------- | ----------------------------------------------------------- |
| GL Host               | BLIPS → GL          | REST API atau file batch                    | Real-time atau end-of-day | Queue + retry; alert IT bila \> 3x retry                    |
| Pefindo Rating        | Pefindo → BLIPS     | Manual upload XLSX/CSV                      | Triwulanan + ad-hoc       | Rejected upload dengan validation report                    |
| IBPA Bond Pricing     | IBPA → BLIPS        | File feed (SFTP/HTTPS download)             | Harian H+0/H+1            | Use STALE\_PRICE flag bila feed gagal                       |
| KSEI / MI NAB         | KSEI/MI → BLIPS     | Manual upload XLSX (atau API jika tersedia) | Harian                    | Use NAB hari sebelumnya bila tidak ada upload               |
| BEI Closing Price     | BEI → BLIPS         | File feed                                   | Harian post-market        | Use STALE\_PRICE flag                                       |
| BI JISDOR             | BI → BLIPS          | Web scraping atau API publikasi             | Hari kerja jam 10:30      | Use kurs hari kerja sebelumnya + REPEAT\_RATE flag          |
| Fund Fact Sheet       | MI → BLIPS          | Manual upload PDF + XLSX                    | Bulanan                   | Notification ke Risk untuk follow up                        |
| LDAP / SSO            | BLIPS → AD          | SAML 2.0 / LDAPS                            | Real-time auth            | Local fallback (cached credentials disabled untuk security) |
| SMTP                  | BLIPS → Mail Server | SMTP TLS                                    | Event-driven              | Queue + retry; dead letter queue                            |
| Antivirus Scanner     | BLIPS → AV API      | REST API on-upload                          | Per upload                | Block upload bila scan unavailable                          |
| S3 (Document Storage) | BLIPS → S3          | S3 SDK                                      | Real-time                 | Retry; alert                                                |
| KMS (Encryption)      | BLIPS → KMS         | KMS SDK                                     | Real-time                 | Cache wrapped DEKs locally to avoid every request           |

## 5.2 GL Host Integration (Critical) \[PHASE 2 DEFERRED — Phase 1 berjalan standalone, lihat Decision Log DEC-005\]

Integrasi BLIPS → GL Host adalah critical path. Default mode: REST API; fallback: file batch.

### 5.2.1 REST API Mode

Endpoint contract:

> POST {GL\_HOST\_BASE\_URL}/journal-entries  
> Content-Type: application/json  
> Authorization: Bearer {OAUTH\_ACCESS\_TOKEN}  
> X-Source-System: BLIPS  
> X-Idempotency-Key: {UUID\_per\_event}  
>   
> Request Body:  
> {  
> "transaction\_id": "PNP-2026-00001",  
> "transaction\_type": "PENEMPATAN",  
> "posting\_date": "2026-01-15",  
> "periode\_id": "PRD-2026-01",  
> "currency": "IDR",  
> "lines": \[  
> {  
> "line\_number": 1,  
> "account\_code": "1.1.3.001",  
> "debit\_amount": 5080000000.00,  
> "credit\_amount": 0.00,  
> "description": "Penempatan Obligasi PT XYZ"  
> },  
> {  
> "line\_number": 2,  
> "account\_code": "1.1.1.001",  
> "debit\_amount": 0.00,  
> "credit\_amount": 5080000000.00,  
> "description": "Penempatan Obligasi PT XYZ"  
> }  
> \],  
> "reference\_id": "OBL-2026-00001",  
> "checksum": "sha256\_hash\_of\_lines"  
> }  
>   
> Response 201 Created:  
> {  
> "gl\_journal\_id": "GL-2026-00012345",  
> "status": "POSTED",  
> "posted\_at": "2026-01-15T10:23:45Z",  
> "balance\_check": "PASSED"  
> }  
>   
> Response 400 Bad Request (validation error):  
> {  
> "error\_code": "GL\_VALIDATION\_FAILED",  
> "error\_message": "Total debit ≠ total credit",  
> "details": \[...\]  
> }  
>   
> Response 409 Conflict (duplicate via Idempotency-Key):  
> {  
> "error\_code": "DUPLICATE\_TRANSACTION",  
> "existing\_gl\_journal\_id": "GL-2026-00012345"  
> }

### 5.2.2 Resilience Pattern

| **Failure**                          | **Behavior**                                                                                                                 |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| Network timeout (5xx, network error) | Retry dengan exponential backoff: 5s, 15s, 45s, 2m, 5m. Setelah 5 retries → move to dead letter queue + alert.               |
| Validation error (4xx)               | Tidak retry; mark BLIPS journal sebagai POSTING\_FAILED dengan error message; alert ke Akuntansi untuk manual investigation. |
| GL Host down extended                | Sistem queue semua jurnal di outbound buffer; alert IT setiap 30 menit; auto-resume saat GL up.                              |
| Idempotency mismatch                 | Bila BLIPS retry tetapi GL sudah post (idempotency key match) → BLIPS pakai existing gl\_journal\_id, mark sebagai POSTED.   |
| Reconciliation mismatch              | Daily reconciliation job: compare BLIPS jurnal status vs GL Host. Mismatch → alert untuk investigasi.                        |

### 5.2.3 File Batch Mode (Fallback)

Bila REST API tidak tersedia, BLIPS export jurnal harian sebagai CSV/XML batch file ke shared SFTP folder; GL Host scheduled job consume.

File naming: BLIPS\_JOURNAL\_{YYYYMMDD}\_{batch\_seq}.csv. Header row + data rows. Manifest file dengan checksum SHA-256.

## 5.3 Pefindo Rating Upload Pipeline

Workflow upload Pefindo:

1.  Risk Officer download Pefindo Default Study (XLSX/CSV) dari portal Pefindo.

2.  Risk Officer upload file ke BLIPS via UI Master Data → Pefindo Rating Upload.

3.  Sistem validasi schema: kolom expected (Issuer Code, Rating, PD 1Y, PD 3Y, PD 5Y, PD 7Y, PD 10Y, Effective Date, Action Type).

4.  Sistem parse rows; cross-reference ke Counterparty existing.

5.  Untuk match counterparty: trigger Rating History update (tanggal\_berlaku, action\_type, notch\_change auto-calc).

6.  Untuk no-match: flag sebagai NEW\_COUNTERPARTY untuk Risk Officer follow-up.

7.  Sistem auto-evaluate SICR & Default trigger berdasarkan rating baru vs origination.

8.  Risk Officer review staged updates → approve → commit ke production.

9.  Audit trail: filename, hash, uploader, rows processed, rows rejected.

## 5.4 IBPA Bond Pricing Feed

IBPA menyediakan harga referensi obligasi harian. Mode: SFTP atau HTTPS download.

| **Aspek**        | **Spec**                                                                                                                          |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Endpoint         | sftp://ibpa-feed.example.com/daily/ atau https://ibpa.co.id/api/pricing (TBD)                                                     |
| Schedule         | Job batch jalankan setiap hari kerja jam 18:00 WIB (post-market settle)                                                           |
| File Format      | CSV: ISIN, Tanggal, Harga (% nominal), Yield, Volume, Rating IBPA                                                                 |
| Validation       | Cross-check ISIN dengan Master Instrumen; price reasonability check (deviasi \> 5% dari hari sebelumnya → flag REVIEW\_NEEDED)    |
| Failure Handling | Bila feed gagal: retry 3x dengan interval 30 menit; bila masih gagal → use STALE\_PRICE dari hari kerja terakhir; alert Akuntansi |
| Audit            | Setiap feed run tercatat di sys\_job\_run\_history dengan rows processed, errors                                                  |

## 5.5 BI JISDOR Scheduled Job

BI JISDOR & Kurs Tengah BI di-update otomatis hari kerja jam 10:30 WIB.

| **Aspek**        | **Spec**                                                                                                                                               |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Source           | BI website publication (https://www.bi.go.id/.../jisdor) atau API resmi bila tersedia                                                                  |
| Schedule         | Cron: 30 10 \* \* MON-FRI (10:30 hari kerja)                                                                                                           |
| Data             | USD/IDR Tengah, Bid, Ask; mata uang lain dari kurs Tengah BI publication harian                                                                        |
| Validation       | Reasonability: kurs hari ini deviasi \> 3% dari hari sebelumnya → flag untuk Akuntansi review (bukan auto-reject, hanya alert)                         |
| Failure Handling | Bila scrape gagal: retry; bila masih gagal jam 11:30 → alert Akuntansi untuk manual upload; auto-flag REPEAT\_RATE bila perlu use kurs hari sebelumnya |
| Manual Override  | Akuntansi dapat manual upload kurs untuk koreksi (workflow approval)                                                                                   |

## 5.6 Document Storage (S3 + KMS)

Object storage untuk document upload — encrypted at rest dengan KMS-managed keys.

| **Aspek**          | **Spec**                                                                                                        |
| ------------------ | --------------------------------------------------------------------------------------------------------------- |
| Storage            | S3-compatible (AWS S3, Azure Blob, MinIO on-prem). Bucket per environment (prod, uat, dev).                     |
| Naming             | documents/{year}/{month}/{entity\_type}/{entity\_id}/{uuid}.{ext}                                               |
| Encryption         | SSE-KMS dengan customer-managed key (CMK) di KMS. Per-object DEK wrapped by CMK.                                |
| Access             | BLIPS service authenticated via IAM role / S3 access key (rotated quarterly).                                   |
| Pre-signed URL     | Untuk download: BLIPS generate pre-signed URL (validity 5 menit) — user browser fetch directly dari S3.         |
| Lifecycle Policy   | Hot tier (frequent access): 90 hari → Cool tier: 1-3 tahun → Archive: 3-10 tahun                                |
| Versioning         | Enabled — setiap upload menjadi version baru; previous version retained 90 hari.                                |
| Replication        | Cross-region replication ke DR site (async).                                                                    |
| Integrity Check    | SHA-256 hash di-compute saat upload dan stored; verification on download.                                       |
| Virus Scan         | Pre-storage virus scan via API (mis. ClamAV daemon, atau cloud AV); reject bila detected.                       |
| Max File Size      | 50 MB per file; multipart upload untuk file \> 5 MB.                                                            |
| Allowed MIME Types | application/pdf, image/png, image/jpeg, application/vnd.openxmlformats-officedocument.\* (XLSX, DOCX), text/csv |

# 6\. API Standards

## 6.1 REST Conventions

Semua API BLIPS WAJIB mengikuti REST + JSON conventions:

| **Aspek**       | **Spec**                                                                                                                                                                                                  |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base URL        | https://api.blips.tugu-re.com/api/v1/ (production); /api/v2/ untuk versi mayor berikut                                                                                                                    |
| Resource Naming | Plural lowercase: /instrumen, /counterparty, /transaksi-penempatan; nested: /instrumen/{id}/sppi-test                                                                                                     |
| HTTP Methods    | GET (read), POST (create), PUT (full update), PATCH (partial update), DELETE (soft-delete only)                                                                                                           |
| Status Codes    | 200 OK, 201 Created, 204 No Content, 400 Bad Request, 401 Unauthorized, 403 Forbidden, 404 Not Found, 409 Conflict, 422 Unprocessable Entity, 429 Rate Limited, 500 Server Error, 503 Service Unavailable |
| Content-Type    | application/json untuk request/response. multipart/form-data untuk upload.                                                                                                                                |
| Charset         | UTF-8                                                                                                                                                                                                     |
| Date Format     | ISO 8601 — date: YYYY-MM-DD, datetime: YYYY-MM-DDTHH:MM:SSZ (UTC)                                                                                                                                         |
| Number Format   | JSON number; untuk currency gunakan string untuk avoid precision loss: "5080000000.00"                                                                                                                    |
| Boolean         | true/false (lowercase)                                                                                                                                                                                    |
| Null            | null (lowercase) untuk explicit absence                                                                                                                                                                   |

## 6.2 Authentication & Authorization

API authentication via OAuth 2.0 Bearer Token (JWT):

> Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...  
>   
> JWT Payload:  
> {  
> "sub": "user\_id\_uuid",  
> "username": "fairuz.zabady@tugu-re.com",  
> "roles": \["ROLE-AKUN", "ROLE-AKUN-CTL"\],  
> "permissions": \["instrumen.read", "jurnal.create", ...\],  
> "iss": "https://idp.tugu-re.com",  
> "aud": "blips-api",  
> "exp": 1735659000,  
> "iat": 1735658100,  
> "jti": "unique\_token\_id"  
> }

**Permission check di API Gateway dan service layer:**

  - API Gateway: validate token signature, expiry, audience, issuer.

  - Service: extract permissions dari token; check terhadap required permission per endpoint.

  - 403 Forbidden bila permission insufficient; 401 Unauthorized bila token invalid/expired.

## 6.3 Standard Request/Response Format

### 6.3.1 List/Pagination Response

> GET /api/v1/instrumen?page=1\&size=20\&sort=tanggal\_penempatan,desc  
>   
> Response 200:  
> {  
> "data": \[  
> { "id": "...", "kode\_instrumen": "OBL-2026-00001", ... },  
> { "id": "...", "kode\_instrumen": "OBL-2026-00002", ... }  
> \],  
> "pagination": {  
> "page": 1,  
> "size": 20,  
> "total\_elements": 145,  
> "total\_pages": 8,  
> "first": true,  
> "last": false,  
> "has\_next": true,  
> "has\_previous": false  
> },  
> "\_links": {  
> "self": "/api/v1/instrumen?page=1\&size=20",  
> "next": "/api/v1/instrumen?page=2\&size=20",  
> "last": "/api/v1/instrumen?page=8\&size=20"  
> }  
> }

### 6.3.2 Filtering & Sorting

Query parameter convention:

  - Filter: ?status=APPROVED\&klasifikasi=AC,FVOCI (comma-separated untuk OR; multi-key untuk AND)

  - Range filter: ?tanggal\_penempatan\_from=2026-01-01\&tanggal\_penempatan\_to=2026-12-31

  - Sort: ?sort=tanggal\_penempatan,desc atau ?sort=tipe\_instrumen,asc\&sort=tanggal\_penempatan,desc

  - Search: ?q=mandiri (full-text search; backend implementation define).

### 6.3.3 Single Resource Response

> GET /api/v1/instrumen/{id}  
>   
> Response 200:  
> {  
> "data": {  
> "id": "550e8400-e29b-41d4-a716-446655440000",  
> "kode\_instrumen": "OBL-2026-00001",  
> "tipe\_instrumen": "OBLIGASI",  
> "sub\_tipe": "OBLIGASI\_KORPORASI",  
> "nama": "Obligasi PT XYZ Tbk Seri A 2026",  
> "isin": "ID1000123456",  
> "counterparty": {  
> "id": "...",  
> "kode": "CP-0042",  
> "nama": "PT XYZ Tbk",  
> "rating\_pefindo": "idA"  
> },  
> "klasifikasi\_psak71": "FVOCI",  
> "nominal": "5000000000.00",  
> "harga\_beli\_persen": "101.5000",  
> "eir": "0.04826688",  
> "tanggal\_penempatan": "2026-01-15",  
> "tanggal\_jatuh\_tempo": "2030-12-31",  
> "status": "AKTIF",  
> "audit": {  
> "created\_by": { "id": "...", "username": "treasury.maker" },  
> "created\_at": "2026-01-15T08:23:11Z",  
> "approved\_by": { "id": "...", "username": "treasury.manager" },  
> "approved\_at": "2026-01-15T09:45:33Z",  
> "version": 1  
> }  
> }  
> }

### 6.3.4 Create / Update Request

> POST /api/v1/instrumen  
> Content-Type: application/json  
>   
> Request Body:  
> {  
> "tipe\_instrumen": "OBLIGASI",  
> "sub\_tipe": "OBLIGASI\_KORPORASI",  
> "nama": "Obligasi PT XYZ Tbk Seri A 2026",  
> "isin": "ID1000123456",  
> "counterparty\_id": "550e8400-...",  
> "nominal": "5000000000.00",  
> "harga\_beli\_persen": "101.5000",  
> "tanggal\_penempatan": "2026-01-15",  
> "tanggal\_jatuh\_tempo": "2030-12-31",  
> "kupon": "5.0000",  
> "frekuensi\_bunga": "SEMESTERAN",  
> "mata\_uang": "IDR",  
> "biaya\_transaksi": "5000000.00"  
> }  
>   
> Response 201 Created:  
> {  
> "data": {  
> "id": "660e8400-e29b-41d4-a716-446655440111",  
> "kode\_instrumen": "OBL-2026-00001",  
> ...  
> },  
> "\_links": {  
> "self": "/api/v1/instrumen/660e8400-..."  
> }  
> }

### 6.3.5 Standard Error Response

> Response 400 Bad Request:  
> {  
> "error": {  
> "code": "ERR-VAL-0042",  
> "message": "Tanggal jatuh tempo harus setelah tanggal penempatan",  
> "details": \[  
> {  
> "field": "tanggal\_jatuh\_tempo",  
> "constraint": "AFTER\_TANGGAL\_PENEMPATAN",  
> "actual\_value": "2025-12-31",  
> "expected": "\> 2026-01-15"  
> }  
> \],  
> "trace\_id": "abc123-def456",  
> "timestamp": "2026-01-15T08:23:11Z"  
> }  
> }  
>   
> Response 422 Unprocessable Entity (multi-field validation):  
> {  
> "error": {  
> "code": "ERR-VAL-0001",  
> "message": "Validation failed",  
> "details": \[  
> { "field": "kupon", "message": "Kupon must be ≥ 0", "actual": "-1.5" },  
> { "field": "frekuensi\_bunga", "message": "Required field missing" }  
> \],  
> "trace\_id": "...",  
> "timestamp": "..."  
> }  
> }

## 6.4 Pagination Standards

| **Strategy**    | **Use Case**                                           | **Spec**                                                       |
| --------------- | ------------------------------------------------------ | -------------------------------------------------------------- |
| Offset-based    | Default; small-to-medium dataset                       | ?page=N\&size=M (page 1-indexed; size default 20, max 100)     |
| Cursor-based    | Large dataset (audit log, transaction history)         | ?cursor=ENCODED\_CURSOR\&size=M; response include next\_cursor |
| Streaming (SSE) | Real-time monitoring (notification, dashboard refresh) | GET /api/v1/notifications/stream → text/event-stream           |

## 6.5 Idempotency

Untuk operasi POST yang membuat resource (terutama jurnal posting & financial transactions): client WAJIB include header X-Idempotency-Key dengan UUID unik per logical operation.

Server cache idempotency\_key → response selama 24 jam. Repeat request dengan key yang sama akan return cached response (tanpa side effect duplicat).

Endpoint yang require idempotency: POST /api/v1/transaksi-penempatan, POST /api/v1/jurnal/post, POST /api/v1/ecl/calculate.

## 6.6 Versioning

API versioning via URL path: /api/v1/, /api/v2/, dst.

**Backward compatibility commitment:**

  - v1 dipertahankan minimal 2 tahun setelah v2 release.

  - Deprecation notice 6 bulan sebelum sunset.

  - Breaking changes (renamed field, changed type, removed endpoint) → MUST trigger versi mayor baru.

  - Non-breaking changes (new field, new endpoint, optional parameter) → tidak perlu versi baru.

## 6.7 Rate Limiting

| **Scope**                    | **Limit**                 | **Window**                         |
| ---------------------------- | ------------------------- | ---------------------------------- |
| Per User Token               | 1.000 requests/minute     | 1 menit rolling                    |
| Per IP Address               | 10.000 requests/minute    | 1 menit rolling                    |
| Per Endpoint (login)         | 10 attempts/minute per IP | 1 menit rolling                    |
| Per Endpoint (export/report) | 10 requests/hour per user | 1 jam rolling                      |
| Per Endpoint (ECL calculate) | 5 requests/hour per user  | 1 jam rolling (resource-intensive) |

Response 429 Too Many Requests dengan header Retry-After: {seconds}.

# 7\. UI/UX Standards

## 7.1 Layout & Grid

| **Aspek**          | **Spec**                                                                                                  |
| ------------------ | --------------------------------------------------------------------------------------------------------- |
| Resolution Target  | Desktop primary: 1366×768 (minimum), 1920×1080 (recommended). Tablet 1024×768 secondary (read-only mode). |
| Grid System        | 12-column responsive grid; gutters 16px                                                                   |
| Container Width    | Max 1280px untuk content; full-width untuk dashboard                                                      |
| Sidebar Navigation | Left sidebar collapsible 240px expanded / 64px collapsed; persistent across modules                       |
| Top Bar            | Fixed top: logo, search, notification bell, user avatar/menu, language toggle. Height 56px.               |
| Content Area       | Main content with padding 24px; cards/sections separated by 16px                                          |
| Footer             | Minimal footer: version, copyright, link to user manual                                                   |

## 7.2 Color Palette

| **Color Token** | **Hex**  | **Usage**                               |
| --------------- | -------- | --------------------------------------- |
| primary-900     | \#1F4E79 | Header utama, primary button (default)  |
| primary-700     | \#2E75B6 | Header sekunder, link hover             |
| primary-100     | \#D9E2F3 | Background highlight, info banner       |
| accent-orange   | \#ED7D31 | Warning, pending state, indicator       |
| accent-green    | \#70AD47 | Success, approved, positive value       |
| accent-red      | \#C00000 | Error, danger, rejected, negative value |
| accent-yellow   | \#FFC000 | Caution, expired, alert                 |
| neutral-900     | \#202124 | Text primary                            |
| neutral-700     | \#5F6368 | Text secondary                          |
| neutral-500     | \#9AA0A6 | Disabled, placeholder                   |
| neutral-300     | \#DADCE0 | Border, divider                         |
| neutral-100     | \#F8F9FA | Background subtle                       |
| neutral-0       | \#FFFFFF | Background primary                      |

## 7.3 Typography

| **Style**    | **Font**               | **Size** | **Weight** | **Usage**                                             |
| ------------ | ---------------------- | -------- | ---------- | ----------------------------------------------------- |
| Heading 1    | Inter / Arial          | 28px     | 600        | Page title                                            |
| Heading 2    | Inter / Arial          | 22px     | 600        | Section title                                         |
| Heading 3    | Inter / Arial          | 18px     | 600        | Subsection                                            |
| Body Default | Inter / Arial          | 14px     | 400        | Body text                                             |
| Body Small   | Inter / Arial          | 12px     | 400        | Caption, helper text                                  |
| Label/Form   | Inter / Arial          | 13px     | 500        | Form labels                                           |
| Code/Numeric | Roboto Mono / Consolas | 13px     | 400        | Numbers, codes (mis. EIR, ECL value, instrument code) |

## 7.4 Form Patterns

Form design conventions:

  - Labels above field (top-aligned), bold-weight 500, 4px space ke field.

  - Required field marked dengan red asterisk (\*) di label.

  - Helper text below field, abu-abu 12px.

  - Error message below field, red 12px, dengan icon ⚠ di awal.

  - Inline validation: real-time saat user blur dari field; submit-time untuk full form check.

  - Long form: section dengan progress indicator (mis. SPPI Test 10 questions: 4/10 completed).

  - Auto-save draft setiap 30 detik untuk form panjang; session recovery.

  - Submit button: primary (blue), disabled state saat validation fail.

  - Cancel button: secondary (outlined); confirmation dialog bila form dirty.

  - Numeric input: format mata uang (Rp 1.000.000,50) saat blur; raw number saat focus untuk edit.

## 7.5 Table Patterns

Data table conventions:

  - Header row: bold, neutral-700 text, neutral-100 background, sticky saat scroll.

  - Row alternating: neutral-0 dan neutral-50 untuk readability.

  - Hover row: neutral-100 background.

  - Selected row: primary-100 background dengan left-border primary-700.

  - Action column (Edit/View/Delete): icon-based, right-aligned.

  - Sort indicator: arrow up/down di header column yang sortable.

  - Filter: filter bar di atas table (chip-style untuk active filters).

  - Pagination: bottom-right; page size selector; jump-to-page input.

  - Bulk action: checkbox per row + select-all; floating action bar saat ada selection.

  - Empty state: icon + message + CTA ("Belum ada instrumen — Buat instrumen baru").

  - Loading state: skeleton screen (placeholder); jangan spinner-only.

  - Export: button "Export" → modal pilih format (Excel/PDF/CSV) dan kolom.

## 7.6 Workflow UI Pattern (Maker-Reviewer-Approver)

Pattern UI untuk transaksi yang require workflow:

  - Status badge (visual chip): DRAFT (abu-abu), PENDING REVIEW (kuning), PENDING APPROVAL (oranye), APPROVED (hijau), REJECTED (merah).

  - Workflow stepper di top of detail page menampilkan stages dengan checkmark untuk completed steps.

  - Action buttons context-aware: Maker view DRAFT lihat "Submit"; Reviewer view PENDING\_REVIEW lihat "Approve / Reject"; Approver view PENDING\_APPROVAL lihat "Approve / Reject".

  - Reject WAJIB modal dengan textarea "Reason for rejection" mandatory.

  - Approve dapat optional comment.

  - Workflow history panel: collapsible side panel showing all state transitions dengan actor, timestamp, comment.

## 7.7 Notification & Alert Patterns

| **Type**          | **Visual**                       | **Use Case**                                                | **Behavior**                  |
| ----------------- | -------------------------------- | ----------------------------------------------------------- | ----------------------------- |
| Toast (Snackbar)  | Bottom-right, slide-in 5 seconds | Success/info action confirmation                            | Auto-dismiss; dismissible     |
| Banner            | Top of page, full-width          | System-wide alert (mis. "Periode Buku Januari 2026 closed") | Persistent until acknowledged |
| Inline Alert      | In-context dengan icon           | Validation error, warning per field                         | Persists with form state      |
| Modal Dialog      | Center overlay                   | Confirmation ("Yakin reject transaksi ini?")                | Modal — blocking until action |
| Notification Bell | Top-right with badge count       | In-app notification center                                  | Click to expand list          |

## 7.8 Accessibility (WCAG 2.1 Level AA)

  - Color contrast: ratio ≥ 4.5:1 untuk normal text; ≥ 3:1 untuk large text.

  - Keyboard navigation: tab order logical; focus indicator visible.

  - Screen reader: semua interactive element memiliki ARIA label; form field linked dengan label via for/id.

  - Alt text untuk images & icons.

  - Error message tidak hanya by color (juga ada icon dan text).

  - Skip-to-content link untuk keyboard users.

  - Compliance check via tools: axe-core, Lighthouse, NVDA screen reader testing.

# 8\. Error Handling & Logging Standards

## 8.1 Error Categories

| **Category**   | **Code Prefix**    | **Severity** | **Example**                                        | **User Action**                          |
| -------------- | ------------------ | ------------ | -------------------------------------------------- | ---------------------------------------- |
| Validation     | ERR-VAL-\#\#\#\#   | INFO         | Field required missing, format invalid             | Correct input dan resubmit               |
| Authorization  | ERR-AUTHZ-\#\#\#\# | WARN         | Permission denied, role mismatch                   | Contact admin atau use different account |
| Authentication | ERR-AUTH-\#\#\#\#  | WARN         | Token expired, MFA required                        | Re-login                                 |
| Business Rule  | ERR-BIZ-\#\#\#\#   | WARN         | Periode buku CLOSED, segregation of duty violation | Tindakan tidak valid; refer to SOP       |
| Integration    | ERR-INT-\#\#\#\#   | ERROR        | GL host timeout, Pefindo upload failed             | Retry; bila persist contact IT           |
| Data           | ERR-DATA-\#\#\#\#  | ERROR        | FK constraint violation, data corruption           | Contact support                          |
| Calculation    | ERR-CALC-\#\#\#\#  | ERROR        | EIR not converged, ECL parameter missing           | Risk/Akuntansi review                    |
| System         | ERR-SYS-\#\#\#\#   | CRITICAL     | DB connection lost, service unavailable            | Auto-retry; notify IT                    |
| Workflow       | ERR-WF-\#\#\#\#    | WARN         | State transition invalid, deadline expired         | Review workflow status                   |
| Security       | ERR-SEC-\#\#\#\#   | CRITICAL     | Suspicious activity, brute force detected          | Account locked; admin investigates       |

## 8.2 Error Code Naming Convention

Format: ERR-{CATEGORY}-{4-digit-number} (mis. ERR-VAL-0042, ERR-EIR-0007)

Contoh error code per modul:

| **Code**       | **Module**     | **Description**                                      | **HTTP Status** |
| -------------- | -------------- | ---------------------------------------------------- | --------------- |
| ERR-VAL-0001   | Common         | Generic validation failed (multi-field)              | 422             |
| ERR-VAL-0042   | Common         | Tanggal jatuh tempo harus setelah tanggal penempatan | 400             |
| ERR-VAL-0050   | Common         | Saldo rekening sumber tidak mencukupi                | 400             |
| ERR-AUTHZ-0001 | Common         | Permission denied for this action                    | 403             |
| ERR-AUTH-0001  | Common         | Token expired, please re-login                       | 401             |
| ERR-AUTH-0010  | Common         | MFA verification required for this action            | 401             |
| ERR-BIZ-0010   | Periode Buku   | Cannot post transaction in CLOSED period             | 409             |
| ERR-BIZ-0020   | Workflow       | Maker cannot also be Approver (segregation of duty)  | 409             |
| ERR-INT-0001   | GL Integration | GL Host timeout — transaction queued                 | 503             |
| ERR-INT-0010   | GL Integration | GL Host validation failed                            | 400             |
| ERR-DATA-0001  | Common         | Foreign key constraint violation                     | 409             |
| ERR-CALC-0001  | EIR            | Newton-Raphson failed to converge in 50 iterations   | 500             |
| ERR-CALC-0010  | ECL            | Required parameter missing (PD, LGD, atau MEV)       | 400             |
| ERR-CALC-0020  | ECL            | LPS aggregator: counterparty not classified as BANK  | 400             |
| ERR-SYS-0001   | Common         | Database connection error                            | 500             |
| ERR-WF-0001    | Workflow       | Invalid state transition (mis. APPROVED → DRAFT)     | 409             |
| ERR-WF-0010    | Workflow       | Workflow deadline expired                            | 410 Gone        |
| ERR-SEC-0001   | Security       | Suspicious activity detected; account locked         | 403             |
| ERR-SEC-0010   | Security       | Brute force detected; rate limited                   | 429             |

## 8.3 Logging Standards

### 8.3.1 Log Levels

| **Level** | **Use Case**                                         | **Sample**                                                       |
| --------- | ---------------------------------------------------- | ---------------------------------------------------------------- |
| DEBUG     | Development troubleshooting (disabled di production) | "Cache hit for instrumen\_id=..."                                |
| INFO      | Normal operation events                              | "User authenticated: user\_id=...", "Job started: AKRUAL\_BUNGA" |
| WARN      | Anomaly tetapi tidak block operation                 | "Slow query detected: 3.2s", "Validation failed for field=..."   |
| ERROR     | Operation failed; recoverable                        | "GL post failed; queued for retry"                               |
| CRITICAL  | System-level failure; immediate attention            | "Database connection lost", "Audit log write failed"             |

### 8.3.2 Log Format (Structured JSON)

> {  
> "timestamp": "2026-05-02T10:23:45.123Z",  
> "level": "INFO",  
> "service": "blips-transaction-service",  
> "trace\_id": "abc123-def456",  
> "span\_id": "789xyz",  
> "user\_id": "550e8400-...",  
> "session\_id": "session-uuid",  
> "request\_id": "req-uuid",  
> "method": "POST",  
> "endpoint": "/api/v1/transaksi-penempatan",  
> "status\_code": 201,  
> "duration\_ms": 245,  
> "message": "Transaksi penempatan created",  
> "context": {  
> "kode\_instrumen": "OBL-2026-00001",  
> "amount\_idr": "5080000000.00"  
> }  
> }

### 8.3.3 Sensitive Data Masking di Log

  - Password, token, API key: NEVER logged.

  - PII (NPWP, no rekening): masked di log (mis. xxxxx1234).

  - Full request/response body: hanya di DEBUG level untuk specific endpoint; di production INFO/WARN: hanya metadata (size, status).

  - Audit log (untuk compliance): different retention policy dari application log.

## 8.4 Monitoring & Alerting Hooks

Setiap service WAJIB expose health & metric endpoint untuk monitoring:

| **Endpoint** | **Tujuan**                                                                             |
| ------------ | -------------------------------------------------------------------------------------- |
| GET /healthz | Liveness probe — returns 200 OK if service alive                                       |
| GET /readyz  | Readiness probe — returns 200 OK if ready to serve (DB connected, etc.)                |
| GET /metrics | Prometheus-format metrics: request\_count, request\_duration, error\_rate per endpoint |
| GET /info    | Build info: version, commit hash, build date                                           |

**Alert thresholds (configurable di monitoring system):**

  - Error rate \> 1% selama 5 menit → alert P2.

  - Error rate \> 5% selama 5 menit → alert P1 (critical).

  - Response time P95 \> 5s selama 10 menit → alert P3.

  - Job failure (MTM, ECL, Akrual) → alert P1 immediate.

  - DB connection pool exhausted \> 80% selama 5 menit → alert P2.

  - Disk usage \> 85% → alert P3.

  - Failed login attempts \> 100 per jam → alert P2 (security).

# 9\. Common Reference Data (Lookup Tables)

Berikut lookup data yang dipakai lintas modul. Stored di sys\_lookup table dengan grouping.

## 9.1 Master Klasifikasi & Kategori

| **Lookup Group**      | **Values**                                                                                                                                                                                                                                                                                                                                                                                        |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| TIPE\_INSTRUMEN       | CASH, DEPOSITO, OBLIGASI, SAHAM, RDN\_PASAR\_UANG, RDN\_PENDAPATAN\_TETAP, RDN\_SAHAM, RDN\_CAMPURAN                                                                                                                                                                                                                                                                                              |
| KLASIFIKASI\_PSAK71   | AC, FVOCI, FVOCI\_ELECTION, FVTPL                                                                                                                                                                                                                                                                                                                                                                 |
| BUSINESS\_MODEL       | HTC, HTCS, OTHER                                                                                                                                                                                                                                                                                                                                                                                  |
| SPPI\_RESULT          | PASS, FAIL                                                                                                                                                                                                                                                                                                                                                                                        |
| STAGE\_ECL            | STAGE\_1, STAGE\_2, STAGE\_3                                                                                                                                                                                                                                                                                                                                                                      |
| PERIODE\_STATUS       | OPEN, SOFT\_CLOSED, CLOSED                                                                                                                                                                                                                                                                                                                                                                        |
| TRANSAKSI\_STATUS     | DRAFT, PENDING\_REVIEW, PENDING\_APPROVAL, APPROVED, REJECTED, EXPIRED, REVOKED, REVERSED                                                                                                                                                                                                                                                                                                         |
| INSTRUMEN\_STATUS     | AKTIF, DICAIRKAN, JATUH\_TEMPO, DIJUAL, REKLASIFIKASI                                                                                                                                                                                                                                                                                                                                             |
| SKENARIO\_PD          | GOOD, NORMAL, BAD                                                                                                                                                                                                                                                                                                                                                                                 |
| TIPE\_EKSPOSUR\_BASEL | SOVEREIGN, SENIOR\_SECURED, SENIOR\_UNSECURED, SUBORDINATED                                                                                                                                                                                                                                                                                                                                       |
| RATING\_PEFINDO       | idAAA, idAA+, idAA, idAA-, idA+, idA, idA-, idBBB+, idBBB, idBBB-, idBB+, idBB, idBB-, idB+, idB, idB-, idCCC, idD                                                                                                                                                                                                                                                                                |
| RATING\_OUTLOOK       | POSITIVE, STABLE, NEGATIVE, DEVELOPING                                                                                                                                                                                                                                                                                                                                                            |
| MATA\_UANG            | IDR, USD, SGD, EUR, JPY, AUD, CNY, MYR, GBP (extensible)                                                                                                                                                                                                                                                                                                                                          |
| FREKUENSI\_BUNGA      | BULANAN, TRIWULANAN, SEMESTERAN, TAHUNAN, DI\_MUKA, JATUH\_TEMPO                                                                                                                                                                                                                                                                                                                                  |
| EVENT\_JURNAL         | PENEMPATAN, AKRUAL\_BUNGA, AMORTISASI\_PREMI\_DISKONTO, MTM\_FVOCI, MTM\_FVTPL, PEMBAYARAN\_BUNGA, PEMBAYARAN\_KUPON, PENERIMAAN\_DIVIDEN, DISTRIBUSI\_REKSADANA, ECL\_PEMBENTUKAN, ECL\_REVERSAL, PENJUALAN\_PENCAIRAN, JATUH\_TEMPO, REKLAS\_OCI\_PL, FX\_UNREALIZED, FX\_REALIZED, STAGE\_MIGRATION, EIR\_REESTIMATION, MODIFIKASI\_MATERIAL, PERIODE\_ADJUSTMENT, CORRECTION\_PERIODE\_CLOSED |
| STAGE\_TRIGGER\_TYPE  | RATING\_DOWNGRADE, DPD\_30\_90, DPD\_GT\_90, DEFAULT\_RATING\_D, PKPU\_PAILIT, RESTRUKTURISASI, CURING, MANUAL\_OVERRIDE                                                                                                                                                                                                                                                                          |

## 9.2 Master Bobot & Konstanta Default

| **Parameter**                | **Nilai Default**                     | **Locked / Adjustable**                               |
| ---------------------------- | ------------------------------------- | ----------------------------------------------------- |
| LPS Coverage                 | Rp 2.000.000.000 per nasabah per bank | Adjustable (oleh CFO bila regulasi LPS berubah)       |
| Bobot Skenario Good          | 0,2500                                | Adjustable oleh ALCO                                  |
| Bobot Skenario Normal        | 0,5000                                | Adjustable oleh ALCO                                  |
| Bobot Skenario Bad           | 0,2500                                | Adjustable oleh ALCO                                  |
| Day Count Convention         | ACT/365                               | Adjustable per instrumen (override) — ACT/365, 30/360 |
| EIR Tolerance                | 0,00000001                            | Locked (technical)                                    |
| EIR Max Iterations           | 50                                    | Locked (technical)                                    |
| SICR Threshold (notch)       | 2 notch dari origination              | Adjustable oleh Komite Risiko                         |
| DPD Stage 2 Threshold        | 30 hari                               | Locked (PSAK 71 default)                              |
| DPD Stage 3 Threshold        | 90 hari                               | Adjustable (rebuttable presumption per PSAK 71)       |
| Probationary Period (Curing) | 3-6 bulan                             | Adjustable oleh Komite Risiko                         |
| LGD Sovereign                | 0,4500                                | Adjustable bila Basel update                          |
| LGD Senior Secured           | 0,2500                                | Adjustable bila Basel update                          |
| LGD Senior Unsecured         | 0,4500                                | Adjustable bila Basel update                          |
| LGD Subordinated             | 0,7500                                | Adjustable bila Basel update                          |
| PPh Bunga Deposito           | 20% (Final 4(2))                      | Adjustable bila tarif berubah                         |
| PPh Kupon Korporasi          | 10% Final                             | Adjustable                                            |
| PPh Dividen WP OP            | 10% Final                             | Adjustable                                            |

# 10\. Appendix Module Index

Setiap modul fungsional di-detailkan di FSD Appendix terpisah. Index berikut memetakan modul SoW v1.1 ke FSD Appendix yang sesuai:

| **Modul SoW v1.1**                           | **FSD Appendix** | **Section** | **BR-IDs (sample)** |
| -------------------------------------------- | ---------------- | ----------- | ------------------- |
| §5.1 Master Data (10 sub-modul)              | Appendix A       | Bab 1       | BR-MAS-001 to 015   |
| §4 SPPI Test & BM Test                       | Appendix A       | Bab 2       | BR-SPP-001 to 012   |
| §5.2 Modul Penempatan                        | Appendix B       | Bab 1       | BR-PNP-001 to 012   |
| §5.3 Modul MTM                               | Appendix B       | Bab 2       | BR-MTM-001 to 009   |
| §5.4 Modul Renewal                           | Appendix B       | Bab 3       | BR-RNW-001 to 006   |
| §5.5 Modul Penjualan/Pencairan               | Appendix B       | Bab 4       | BR-SLE-001 to 008   |
| §5.6 Modul Jatuh Tempo                       | Appendix B       | Bab 5       | BR-MAT-001 to 006   |
| §5.7 Modul Pendapatan Investasi              | Appendix B       | Bab 6       | BR-PND-001 to 008   |
| §5.8 Modul Media Upload                      | Appendix B       | Bab 7       | BR-UPL-001 to 009   |
| §7-8 ECL Engine                              | Appendix C       | Bab 1       | BR-ECL-001 to 024   |
| §5.12 EIR & Amortisasi (NEW v1.1)            | Appendix C       | Bab 2       | BR-EIR-001 to 020   |
| §5.9 Periode Buku                            | Appendix D       | Bab 1       | BR-PRD-001 to 014   |
| §5.1.8, §5.10 FX Rate Mgmt                   | Appendix D       | Bab 2       | BR-FX-001 to 013    |
| §5.1.10, §5.11 Mapping Jurnal & GL Interface | Appendix D       | Bab 3       | BR-JNL-001 to 011   |
| §10.3 Reporting & Dashboard                  | Appendix E       | Bab 1       | BR-RPT-001 to 025   |

*Note: total 201 unique BR-IDs di BRD; setiap BR akan memiliki minimal 1 FR (Functional Requirement) di FSD Appendix yang sesuai.*

# Sign-Off Page

FSD Master ini menetapkan standard teknis lintas-modul untuk sistem BLIPS IFRS 9. Dengan menandatangani halaman ini, pihak-pihak menyatakan bahwa standard ini akan dipatuhi oleh seluruh FSD Appendix dan implementasi.

**Disusun oleh:**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| IT Architect / Lead Solution Designer                            | Lead Developer                                                   |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

**Direview oleh:**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Project Manager BLIPS / PMO                                      | Vendor Implementor Lead                                          |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Kepala IT Security                                               | Kepala Operations                                                |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

**Disetujui oleh:**

|                                                                  |                                                                  |
| ---------------------------------------------------------------- | ---------------------------------------------------------------- |
|                                                                  |                                                                  |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ |
| Direktur Teknologi Informasi                                     | Direktur Keuangan (CFO)                                          |
| Technical Approver                                               | Sponsor                                                          |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                    |

*--- AKHIR DOKUMEN FSD MASTER ---*
