*\[ LOGO TUGURE \]*

**DECISION LOG**

**BLIPS IFRS 9 — DECISION LOG**

*Critical Project Decisions • Addendum to BRD/SoW/FSD/ERD*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**       | **Keterangan**                                                                                                                                                                      |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Judul Dokumen     | Decision Log — Sistem BLIPS IFRS 9                                                                                                                                                  |
| Kode Dokumen      | DEC-BLIPS-IFRS9-2026-001                                                                                                                                                            |
| Versi             | 1.0                                                                                                                                                                                 |
| Status            | ACTIVE — Reference for ongoing implementation                                                                                                                                       |
| Tanggal Terbit    | 29 Mei 2026                                                                                                                                                                         |
| Bahasa            | Bahasa Indonesia                                                                                                                                                                    |
| Pemilik Dokumen   | Working Group BLIPS — IT Internal Team                                                                                                                                              |
| Penyusun          | Project Manager + IT Architect                                                                                                                                                      |
| Reviewer          | Working Group, Direksi                                                                                                                                                              |
| Approver          | CFO selaku Sponsor                                                                                                                                                                  |
| Fungsi Dokumen    | Mencatat seluruh keputusan critical yang menjadi addendum terhadap BRD/SoW/FSD/ERD. Dipakai sebagai single source of truth untuk decision history yang membentuk arah implementasi. |
| Audiens Utama     | Internal IT team (developers, DBA, DevOps), agentic AI agents yang implementasi, business stakeholder untuk konteks                                                                 |
| Reference Dokumen | SoW v1.3, BRD v1.1, FSD Master v1.0, FSD Appendix A-E, ERD v1.2                                                                                                                     |

# 1\. Pendahuluan

## 1.1 Tujuan Dokumen

Decision Log ini merekam keputusan-keputusan strategic dan teknis yang diambil oleh Working Group BLIPS dan stakeholder kunci selama Discovery & Planning Phase. Decision Log bersifat addendum — tidak mengganti BRD/SoW/FSD/ERD yang sudah ada, melainkan mencatat keputusan konkret untuk item-item yang sebelumnya disajikan sebagai recommendation, alternatif, atau placeholder di dokumen utama.

## 1.2 Konteks Strategis

PT Tugu Reasuransi Indonesia memutuskan untuk mengimplementasi BLIPS IFRS 9 dengan tim internal IT (tanpa vendor implementor eksternal) menggunakan pendekatan agentic AI sebagai team builder. Pendekatan ini memerlukan input deterministic yang lebih tegas dibanding delivery konvensional dengan vendor — semua ambiguitas di dokumen baseline harus di-resolve sebelum agent mulai produktif.

## 1.3 Format Keputusan

Setiap keputusan mencakup elemen berikut:

  - Decision ID — kode unik (DEC-\#\#\#)

  - Decision Title — judul ringkas

  - Decision — keputusan aktual yang dibuat

  - Rationale — alasan dipilih option ini

  - Impact — dampak ke dokumen lain & implementasi

  - Downstream Changes — perubahan yang harus dilakukan ke dokumen turunan

  - Status — ACTIVE / SUPERSEDED

  - Decided By — siapa yang mengambil keputusan

  - Decided On — tanggal

# 2\. Critical Decisions (29 Mei 2026)

## 2.1 DEC-001 — Technology Stack Final

| **Aspek**  | **Detail**                                                                                         |
| ---------- | -------------------------------------------------------------------------------------------------- |
| Decision   | Technology stack final: Backend Golang, Frontend Next.js (versi 14+), Database PostgreSQL 18       |
| Status     | ACTIVE                                                                                             |
| Decided By | Working Group BLIPS + IT Architect                                                                 |
| Decided On | 29 Mei 2026                                                                                        |
| Supersedes | FSD Master v1.0 §2.2 (yang sebelumnya menyajikan recommendation matrix dengan multiple alternatif) |

**Detail technology stack final:**

| **Layer**                 | **Pilihan Final**                                                                               | **Rasional**                                                                                                                                                                                                 |
| ------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Backend Language          | Golang (Go) 1.22+                                                                               | Performance tinggi untuk batch processing (MTM harian, ECL akhir bulan); concurrent processing native via goroutines; binary deployment sederhana untuk on-premise; ecosystem mature untuk financial systems |
| Backend Framework         | Gin atau Fiber (HTTP router) + GORM (ORM) ATAU sqlx untuk performance-critical queries          | Lightweight, fast, well-documented. GORM untuk CRUD standard; sqlx untuk reporting query kompleks                                                                                                            |
| Frontend Framework        | Next.js 14+ (React 18, App Router, TypeScript)                                                  | Modern React framework, SSR support untuk reporting page yang berat, App Router untuk modular structure, TypeScript untuk type safety di financial calculations                                              |
| Frontend State Management | Zustand atau Redux Toolkit                                                                      | Zustand simpler untuk module independence; Redux Toolkit bila perlu time-travel debugging                                                                                                                    |
| Frontend UI Library       | shadcn/ui (Radix UI primitives + Tailwind CSS) atau Ant Design                                  | shadcn/ui untuk modern customizable UI; Ant Design bila prefer mature enterprise components                                                                                                                  |
| Database                  | PostgreSQL 18                                                                                   | Latest stable; advanced features (improved partitioning, JSONB performance, vector support untuk future AI features); proven untuk financial workloads                                                       |
| ORM                       | GORM v2 untuk standard CRUD; sqlx untuk complex reporting                                       | GORM auto-migration untuk schema management; sqlx untuk fine-grained control                                                                                                                                 |
| API Style                 | REST + JSON (sesuai FSD Master Bab 6)                                                           | Mengikuti spec API yang sudah didefinisikan; future-proof untuk gRPC internal services                                                                                                                       |
| Authentication            | JWT (golang-jwt/jwt) + Argon2id password hashing                                                | Standard JWT untuk stateless auth; Argon2 untuk strength password hashing                                                                                                                                    |
| IDP / SSO                 | Keycloak (on-premise) atau Active Directory + custom SAML adapter                               | Keycloak open-source, on-premise, integrate dengan Tugure AD via LDAP federation                                                                                                                             |
| Job Scheduler / Queue     | Asynq atau River (Go-native) atau Temporal                                                      | Asynq simple untuk job queue; Temporal untuk complex workflow orchestration                                                                                                                                  |
| Caching                   | Redis 7+ (single-node atau cluster bila scale up)                                               | Standard untuk Go ecosystem, well-supported                                                                                                                                                                  |
| Object Storage            | MinIO (S3-compatible, on-premise)                                                               | On-premise S3 alternative; sesuai requirement on-premise deployment                                                                                                                                          |
| Monitoring & Logging      | Prometheus + Grafana + Loki (logs) + Jaeger (tracing)                                           | Open-source observability stack; on-premise deployment friendly                                                                                                                                              |
| CI/CD                     | GitLab CI (self-hosted) atau Jenkins                                                            | Self-hosted untuk on-premise; GitLab integration dengan Git repo internal                                                                                                                                    |
| Containerization          | Docker + Docker Compose untuk development; Kubernetes (on-premise) untuk production             | Standard containerization; K8s untuk orchestration scale                                                                                                                                                     |
| Infrastructure-as-Code    | Terraform untuk infrastructure; Ansible untuk configuration management                          | Industry standard untuk IaC                                                                                                                                                                                  |
| Testing — Backend         | Go built-in testing + testify (assertions) + gomock (mocking) + testcontainers-go (integration) | Go ecosystem standard                                                                                                                                                                                        |
| Testing — Frontend        | Jest + React Testing Library + Playwright (E2E)                                                 | Modern frontend testing stack                                                                                                                                                                                |
| Code Quality              | golangci-lint untuk Go; ESLint + Prettier untuk TypeScript                                      | Standard linters                                                                                                                                                                                             |
| Documentation             | Swagger / OpenAPI 3.0 untuk API docs (swaggo/swag); Storybook untuk frontend components         | Auto-generated docs                                                                                                                                                                                          |

**Rasional dipilih Golang + Next.js + PostgreSQL 18:**

  - Performance untuk job batch — Go goroutines memungkinkan parallel processing ECL/MTM untuk 1.500+ instrumen dalam SLA.

  - Single binary deployment di on-premise — Go compile ke binary tunggal, mudah deploy & rollback tanpa runtime dependency seperti JVM atau Node.js runtime.

  - PostgreSQL 18 — pgcrypto, JSONB, GIN, partitioning native (sudah dipakai di ERD), parallel query.

  - Next.js untuk frontend — modern, productive, TypeScript-friendly untuk complex financial UI; SSR untuk reporting page yang akses banyak data.

  - Talent availability — Go developer pool di Indonesia growing; Next.js sangat populer untuk modern web dev.

  - Open-source friendly — sesuai pendekatan internal IT team tanpa vendor lock-in.

**Impact ke FSD Master:**

  - FSD Master §2.2 Recommended Tech Stack table di-update: hapus alternatif, set Go/Next.js/PostgreSQL 18 sebagai LOCKED choice.

  - FSD Master §2.3 Deployment Topology di-revise: on-premise only (lihat DEC-002).

  - FSD Appendix B-E §API endpoints di-implement dengan Gin/Fiber spec.

  - ERD §SQL DDL sudah ditulis untuk PostgreSQL — confirm compatible dengan v18 (sebagian besar syntax sama dengan v15; perlu test edge cases).

  - BLIPS\_init\_schema.sql perlu cek: extension pgcrypto + uuidv7 function compatibility dengan PostgreSQL 18.

## 2.2 DEC-002 — Deployment Topology: On-Premise

| **Aspek**  | **Detail**                                                                                |
| ---------- | ----------------------------------------------------------------------------------------- |
| Decision   | Deployment full on-premise di data center Tugure. Tidak menggunakan cloud public/private. |
| Status     | ACTIVE                                                                                    |
| Decided By | Direktur IT + CFO                                                                         |
| Decided On | 29 Mei 2026                                                                               |
| Supersedes | FSD Master v1.0 §2.3 (yang sebelumnya menyajikan opsi cloud Indonesia)                    |

**Detail deployment topology:**

| **Component**          | **Spec On-Premise**                                                                                                                           |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Production Data Center | Data center primary Tugure (Jakarta)                                                                                                          |
| DR Data Center         | Data center secondary (Surabaya/Cikarang) — opsional Phase 2                                                                                  |
| Compute                | Bare-metal servers atau virtualization (VMware / Proxmox / KVM)                                                                               |
| Network                | Internal corporate network; integrate dengan existing Tugure infrastructure                                                                   |
| Storage                | SAN/NAS on-premise; MinIO untuk object storage (document upload)                                                                              |
| Database               | PostgreSQL 18 — primary di production rack; read-replica di same DC untuk reporting                                                           |
| Backup                 | Local backup server + offsite tape/storage; retention 10 tahun                                                                                |
| Network Perimeter      | Existing Tugure firewall + WAF; tidak ada exposure ke internet kecuali integrasi terbatas (Pefindo manual upload, BI JISDOR via egress proxy) |
| Identity Provider      | Integrate dengan Tugure Active Directory (LDAP federation atau Keycloak)                                                                      |
| Monitoring             | Prometheus + Grafana on-premise                                                                                                               |
| Backup & Recovery      | PostgreSQL pg\_basebackup + WAL archival; restore test quarterly                                                                              |
| High Availability      | Phase 1: single-DC dengan internal redundancy (DB primary+replica, app tier 2-node). Phase 2: cross-DC failover bila scale up                 |

**Rasional on-premise:**

  - Data sovereignty — financial data Tugure tetap di dalam fisik infrastructure perusahaan, simplify compliance UU PDP.

  - Existing infrastructure — Tugure sudah punya data center matang dengan tim ops yang familiar.

  - No vendor cloud lock-in — sesuai pendekatan internal IT team.

  - Cost predictability — fixed infrastructure cost vs variable cloud cost untuk workload steady-state.

  - Latency dengan GL host & sistem existing Tugure (saat integrate Phase 2) lebih rendah karena sama-sama on-premise.

**Impact:**

  - FSD Master §2.3 Deployment Topology: hapus opsi cloud, fokus on-premise.

  - FSD Master §3.1 Security Architecture: update untuk on-premise network model (no AWS IAM, gunakan internal RBAC + Tugure AD).

  - Document Storage: MinIO on-premise sebagai S3-compatible (FSD Master §5.6).

  - Backup retention 10 tahun harus include offsite component (tape, atau replicate ke DR site).

  - DR Site planning untuk Phase 2 — Phase 1 single-DC dulu untuk simplify go-live.

## 2.3 DEC-003 — Delivery Model: Internal IT Team (No Vendor Implementor)

| **Aspek**  | **Detail**                                                                                                                            |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Decision   | Implementasi 100% oleh tim IT internal Tugure dengan bantuan agentic AI sebagai team builder. Tidak ada vendor implementor eksternal. |
| Status     | ACTIVE                                                                                                                                |
| Decided By | Direksi (Direktur IT + CFO)                                                                                                           |
| Decided On | 29 Mei 2026                                                                                                                           |
| Supersedes | BRD v1.1 §12.3 D-02 (Vendor selection sebagai critical path) — sekarang TIDAK BERLAKU                                                 |

**Struktur tim internal IT:**

| **Role**                           | **Tanggung Jawab**                                                                              | **Catatan**                                                                    |
| ---------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| IT Architect (Lead)                | Solution architecture, technical decisions, code review escalation                              | Senior engineer Tugure                                                         |
| Backend Developers                 | Go service development per modul (master data, transaction, ECL/EIR, reporting)                 | 2-3 mid/senior engineers atau setara via agentic AI                            |
| Frontend Developers                | Next.js UI development per modul                                                                | 1-2 engineers atau setara via agentic AI                                       |
| Database Architect / DBA           | PostgreSQL design, migration, performance tuning, backup/recovery                               | Senior DBA Tugure                                                              |
| DevOps Engineer                    | Infrastructure provisioning, CI/CD, monitoring, deployment                                      | 1-2 engineers                                                                  |
| QA Engineer                        | Test case design, automation, UAT facilitation                                                  | 1 engineer + agent assistance                                                  |
| Security Engineer                  | Security review, pentest coordination, audit trail validation                                   | Existing Tugure Security team                                                  |
| Project Manager                    | Project tracking, risk management, stakeholder communication                                    | Senior PM dengan financial systems experience                                  |
| Business Analyst                   | Bridge antara business requirement & technical implementation; review compliance interpretation | BA dengan PSAK 71 knowledge (atau external consultant pada milestone tertentu) |
| DSAK-Certified Accountant Reviewer | Review compliance implementation (EIR, ECL, klasifikasi); sign-off Phase 6                      | Internal Akuntansi senior atau external consultant per milestone               |

**Rasional:**

  - Kontrol penuh atas codebase dan IP — tidak ada vendor lock-in.

  - Knowledge retention — tim internal yang membangun akan maintain jangka panjang.

  - Cost efficiency — eliminasi vendor margin (typical 30-50% markup).

  - Iteration speed — tidak ada friction kontrak/PO untuk small changes.

  - Existing skills — Tugure IT team punya base technical capability yang bisa di-leverage dengan agentic AI sebagai force multiplier.

**Risiko & Mitigasi:**

  - RISK: Lack of vendor accountability untuk delivery. MITIGATION: Internal SLA dengan tim IT; CFO sebagai accountable executive; phase gate reviews bulanan dengan Steering.

  - RISK: Compliance interpretation gap (DSAK-level expertise). MITIGATION: Engage DSAK-certified external consultant untuk milestone reviews (Phase 1 design, Phase 6 ECL/EIR, Phase 9 UAT, Phase 10 go-live).

  - RISK: Production incident response 24x7. MITIGATION: On-call rotation tim internal + runbook + escalation matrix.

  - RISK: Auditor eksternal Tugure skeptical terhadap AI-implemented code. MITIGATION: Engage auditor early (briefing di Phase 1), document AI development methodology + human review checkpoints, demonstrate audit trail integrity.

**Impact:**

  - BRD §12.3 D-02 Vendor selection: TIDAK BERLAKU. Update dependency status menjadi N/A.

  - BRD §13 Risk Register R-05 (Vendor underdeliver): TIDAK BERLAKU. Tambah risk baru R-16 (Internal team capacity gap).

  - BRD §15 Implementation Approach: update — no vendor management overhead.

  - Working Group BLIPS struktur: tidak include vendor lead; full internal.

## 2.4 DEC-004 — Master CoA: Gunakan Existing Tugure GL CoA

| **Aspek**  | **Detail**                                                                                            |
| ---------- | ----------------------------------------------------------------------------------------------------- |
| Decision   | Gunakan CoA existing Tugure dari sistem GL existing sebagai master CoA BLIPS. Tidak membuat CoA baru. |
| Status     | ACTIVE                                                                                                |
| Decided By | Akuntansi + CFO                                                                                       |
| Decided On | 29 Mei 2026                                                                                           |
| Supersedes | SoW v1.3 §5.1.9 (yang memberikan opsi internal/import\_erp/import\_excel)                             |

**Approach:**

1.  Akuntansi Tugure export full CoA dari sistem GL existing (Excel format).

2.  Sample 12 baris CoA yang sudah ada di SQL DDL Section 12.9 hanya sebagai placeholder — replace dengan actual Tugure CoA.

3.  Sumber\_coa di mst.chart\_of\_accounts = 'IMPORT\_EXCEL\_TUGURE\_GL'.

4.  Mapping kategori\_investasi (AC/FVOCI/FVTPL/OCI\_FVOCI/CKPN) per akun dilakukan secara manual oleh Akuntansi saat import — bukan auto-detect.

5.  Master Mapping Jurnal Bab 5.1.10 di-config menggunakan kode\_akun dari CoA existing.

**Catatan untuk Implementasi:**

  - Akuntansi Tugure perlu sediakan: Excel file CoA dengan kolom (Kode Akun, Nama Akun, Tipe, Sub-Tipe, Posisi Normal, Kategori Investasi mapping per akun).

  - Akuntansi memutuskan: akun mana yang dipakai untuk Investasi AC, FVOCI, FVTPL, OCI, CKPN, Pendapatan Bunga, Pendapatan Dividen, Realized Gain/Loss, FX Unrealized/Realized, PPh Beban.

  - Bila CoA existing tidak punya akun spesifik untuk FVOCI / OCI / CKPN (mis. GL legacy tidak granular per klasifikasi PSAK 71), Akuntansi perlu setup akun baru di GL existing DULU sebelum import ke BLIPS.

**Impact:**

  - SQL DDL Section 12.9 (sample CoA) — replace dengan actual Tugure CoA saat seed initial.

  - Master Mapping Jurnal Bab 5.1.10 — config akun mapping menggunakan kode\_akun aktual Tugure (bukan 1.1.2.001 dll yang generik).

  - UI Master CoA Bab 5.1.9: prioritas tinggi pada import Excel dari GL existing.

## 2.5 DEC-005 — GL Host Integration: DEFERRED ke Phase 2

| **Aspek**  | **Detail**                                                                                                                                                                                                                |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Decision   | Phase 1 (initial go-live) BLIPS berjalan STANDALONE — tidak ada integrasi otomatis ke GL Host. Jurnal posted di BLIPS sebagai internal record; export manual / batch ke GL Host. Integrasi otomatis dilakukan di Phase 2. |
| Status     | ACTIVE                                                                                                                                                                                                                    |
| Decided By | CFO + Direktur IT                                                                                                                                                                                                         |
| Decided On | 29 Mei 2026                                                                                                                                                                                                               |
| Supersedes | FSD Master v1.0 §5.2 (REST API spec ke GL Host); BRD v1.1 §6.4 (GL Host interface as required)                                                                                                                            |

**Mode Phase 1 (Standalone):**

6.  BLIPS post jurnal lengkap di internal table jrnl.header + jrnl.detail.

7.  Generate file export harian (CSV/XLSX format yang compatible dengan GL Host Tugure).

8.  Akuntansi download file export → upload manual ke GL Host (atau via SFTP file batch).

9.  Reconciliation: Akuntansi cross-check total D/K BLIPS vs GL Host monthly.

10. BLIPS jrnl.gl\_status: status 'EXPORTED\_MANUAL' untuk Phase 1.

**Mode Phase 2 (Future Integration):**

  - REST API integration ke GL Host (sesuai FSD Master §5.2 spec).

  - Real-time posting per transaksi.

  - Automated reconciliation daily.

  - Migrasi Phase 1 export-manual → Phase 2 API integration tanpa data loss.

**Rasional defer ke Phase 2:**

  - Reduce scope Phase 1 — fokus pada core compliance (klasifikasi, ECL, EIR) terlebih dahulu.

  - Negosiasi/setup API dengan GL host vendor membutuhkan koordinasi yang dapat memakan waktu — tidak boleh menjadi critical path Phase 1.

  - Standalone mode memungkinkan parallel run dengan sistem legacy: BLIPS hitung correct, Akuntansi tetap entry manual ke GL legacy untuk safety.

  - Phase 2 integration setelah BLIPS prove correctness dan trust terbentuk.

**Impact:**

  - FSD Master §5.2 GL Host Integration: tandai sebagai 'Phase 2 — Deferred'. Spec tetap valid untuk Phase 2 planning.

  - BRD §6.4 System Boundaries — GL Host interface: update jadi 'Manual export Phase 1 / API Phase 2'.

  - FSD Appendix D Bab 3 (Master Mapping Jurnal + GL Interface): tambah section 'Standalone Mode (Phase 1)' yang menjelaskan export file approach.

  - BRD §15.2 Milestone — Phase 5 (Development Modul Jual + JT + Jurnal & GL Interface): Adjust scope ke 'Jurnal Internal + Export File'.

  - Risk register: GL integration deferred — risk reduced untuk Phase 1.

## 2.6 DEC-006 — Pefindo PD Source: Annual Default Study 2007-2025 PDF

| **Aspek**  | **Detail**                                                                                                                                                                                              |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Decision   | Initial Pefindo PD values di-seed dari Pefindo Annual Default Study 2007-2025 (file PDF di root folder workspace), Appendix 2 (Survival Pool Cumulative Average Default Rate based on Debt Instrument). |
| Status     | ACTIVE                                                                                                                                                                                                  |
| Decided By | Risk Officer + Akuntansi                                                                                                                                                                                |
| Decided On | 29 Mei 2026                                                                                                                                                                                             |
| Supersedes | SoW v1.3 §5.1.3 catatan ('angka di atas bersifat ilustratif'); SQL DDL Section 12.8 sample values                                                                                                       |

**Source Data:**

  - File: Pefindo\_Annual\_Default\_Study\_2007-2025\_EN.pdf

  - Author: PT Pemeringkat Efek Indonesia (PEFINDO)

  - Publication date: April 2026

  - Section referenced: Appendix 2 — Survival Pool Cumulative Average Default Rate (Based on Debt Instrument), pages 19-25

**Initial PD Seed Values (Debt Instrument basis — ACTUAL Pefindo data):**

| **Rating** | **Y1 (12-Month)** | **Y3** | **Y5** | **Y7** | **Y10** | **Catatan**                                                              |
| ---------- | ----------------- | ------ | ------ | ------ | ------- | ------------------------------------------------------------------------ |
| idAAA      | 0,0000            | 0,0000 | 0,0000 | 0,0000 | 0,0000  | No default observed 2007-2025                                            |
| idAA       | 0,0000            | 0,0000 | 0,0020 | 0,0020 | 0,0020  | 1 default (2012, SHIP) di Y5; remain flat sampai Y18                     |
| idA        | 0,0031            | 0,0290 | 0,0549 | 0,0549 | 0,0549  | Multiple defaults — 0,31% Y1, naik ke 5,49% Y5+                          |
| idBBB      | 0,0567            | 0,1734 | 0,1866 | 0,1934 | 0,1934  | Highest default rate among investment grade; flat di 19,34% setelah Y6   |
| idBB       | 0,5008            | 0,5683 | 0,5683 | 0,5683 | 0,5683  | Very high Y1 PD; non-investment grade                                    |
| idB        | 0,0000            | 0,0000 | 0,0000 | 0,0000 | 0,0000  | Limited monitoring population di Pefindo (perlu internal model override) |
| idCCC      | 0,0939            | 0,6633 | 0,6633 | 0,6633 | 0,6633  | Y2 jump ke 66,33% (1 issuer SHIP default)                                |
| idD        | 1,0000            | 1,0000 | 1,0000 | 1,0000 | 1,0000  | Default actual                                                           |

**Catatan Penting:**

  - PD untuk idB = 0,00% karena Pefindo memiliki limited monitoring population (hanya 2 issuer dalam 3 tahun terakhir). Tugure Risk Management WAJIB override dengan internal model atau gunakan mapping ke rating international (mis. S\&P B → 4-5%).

  - PD untuk idBB sangat tinggi (50,08% Y1) — reflect actual historical default Pefindo termasuk SHIP sector. Konservatif untuk portfolio Tugure tetapi accurate untuk eksposur high-yield Indonesia.

  - Tabel di atas adalah Debt Instrument basis. Bila perlu Issuing Company basis untuk Counterparty (bank/issuer level), values berbeda — lihat Appendix 3 PDF.

  - Lifetime PD Y3/Y5/Y7/Y10 tidak compound dari Y1 — ini ACTUAL observed cumulative dari Pefindo Survival Pool methodology, bukan derivasi mathematical.

  - Update berkala: Pefindo publish Annual Default Study setiap kuartal — Risk Officer wajib upload version terbaru setiap publikasi.

**Impact:**

  - SQL DDL Section 12.8 PD Pefindo seed: REPLACED dengan actual Pefindo values (sudah dilakukan dalam revisi ini).

  - SoW v1.3 §5.1.3 catatan 'ilustratif': perlu di-update jadi 'actual Pefindo Annual Default Study 2007-2025'.

  - SoW v1.3 §5.1.3.a Lifetime PD: catatan 'derivation' perlu di-update — Lifetime PD adalah actual cumulative dari Pefindo Survival Pool, bukan derivation mathematical.

  - Risk Management policy: dokumentasikan keputusan internal override untuk idB (limited population) — perlu memo ALCO.

  - FSD Appendix C §2 ECL Algorithm: PD values dipakai sesuai tabel actual di atas; tidak ada derivasi cumulative dari 12-month.

# 3\. Summary of Downstream Document Changes

Sebagai akibat 6 keputusan di atas, dokumen berikut perlu di-update:

| **Dokumen**                    | **Section**                      | **Perubahan**                                          | **Priority**             |
| ------------------------------ | -------------------------------- | ------------------------------------------------------ | ------------------------ |
| FSD Master v1.0 → v1.1         | §2.2 Technology Stack            | Lock pilihan: Go/Next.js/PostgreSQL 18                 | HIGH                     |
| FSD Master v1.0 → v1.1         | §2.3 Deployment Topology         | On-premise only; hapus cloud options                   | HIGH                     |
| FSD Master v1.0 → v1.1         | §5.2 GL Host Integration         | Tandai DEFERRED ke Phase 2                             | HIGH                     |
| SoW v1.3 → v1.4                | §5.1.3 Master PD Pefindo         | Update sumber: actual Pefindo PDF, bukan ilustratif    | HIGH                     |
| SoW v1.3 → v1.4                | §5.1.10/5.11 Mapping Jurnal & GL | Tambah section 'Standalone Mode Phase 1' (export file) | HIGH                     |
| SoW v1.3 → v1.4                | §5.1.9 Master CoA                | Update default sumber: IMPORT\_EXCEL\_TUGURE\_GL       | MEDIUM                   |
| BRD v1.1 → v1.2 (optional)     | §12.3 Dependencies               | Hapus vendor selection D-02 (no vendor)                | MEDIUM                   |
| BRD v1.1 → v1.2 (optional)     | §13 Risk Register                | Hapus R-05; tambah R-16 (Internal capacity)            | MEDIUM                   |
| BLIPS\_init\_schema.sql v1.2   | Section 12.8 Pefindo PD          | REPLACED dengan actual Pefindo values (DONE)           | DONE                     |
| BLIPS\_init\_schema.sql v1.2   | Section 12.9 CoA Sample          | Replace dengan actual Tugure CoA saat seed             | PENDING Akuntansi export |
| FSD Appendix D v1.1 (optional) | §3 Mapping Jurnal & GL Interface | Tambah section Phase 1 Standalone mode                 | MEDIUM                   |
| BRD v1.1 → v1.2 (optional)     | §6.4 System Boundaries           | GL Host marked 'Manual export Phase 1 / API Phase 2'   | MEDIUM                   |

*Saya rekomendasikan untuk DEC-001 dan DEC-002 update langsung di FSD Master (release v1.1). Untuk DEC-005 dan DEC-006 update di SoW v1.4. BRD/FSD Appendix updates bisa di-batch nanti atau dilakukan parallel saat agent sudah mulai implementasi.*

# 4\. Open Items yang Masih Perlu Diresolve

Berdasarkan analisis dokumen, masih ada beberapa item dengan priority HIGH yang perlu diklarifikasi sebelum Phase 1 development mulai:

| **Open Item**                                                                                                              | **Owner**        | **Priority** | **Target Resolve**                |
| -------------------------------------------------------------------------------------------------------------------------- | ---------------- | ------------ | --------------------------------- |
| Master CoA Tugure existing — Excel export dari GL legacy untuk seed initial                                                | Akuntansi        | CRITICAL     | Sebelum Phase 1 kick-off          |
| Counterparty list initial — bank, issuer korporasi, MI, kustodian existing Tugure                                          | Treasury + Risk  | CRITICAL     | Sebelum Phase 3 (Master Data dev) |
| Initial portfolio + instrumen actual — untuk migration (jika ada)                                                          | Treasury         | HIGH         | Sebelum Phase 9 (UAT)             |
| User list dengan role assignment untuk sec.user + sec.user\_role                                                           | IT Admin + HR    | HIGH         | Sebelum Phase 3                   |
| Tugure Active Directory integration spec (LDAP credentials, group mapping)                                                 | IT Security      | HIGH         | Sebelum Phase 2 (Desain)          |
| LGD per tipe eksposur final — confirm Tugure pakai Basel III default (0,4500/0,2500/0,4500/0,7500) atau ada internal model | Risk Management  | HIGH         | Sebelum Phase 6 (ECL/EIR)         |
| Bobot skenario PD — confirm Tugure pakai 0,25/0,50/0,25 default atau adjust                                                | ALCO             | HIGH         | Sebelum Phase 6                   |
| Impact MEV to PD initial values per skenario                                                                               | ALCO + Risk      | HIGH         | Sebelum Phase 6                   |
| Impact PD multiplier initial value (default 1,1500)                                                                        | ALCO + Risk      | HIGH         | Sebelum Phase 6                   |
| SICR threshold confirmation (2 notch downgrade default)                                                                    | Komite Risiko    | MEDIUM       | Sebelum Phase 6                   |
| Probationary period curing (3 vs 6 bulan)                                                                                  | Komite Risiko    | MEDIUM       | Sebelum Phase 6                   |
| GL Host export file format — confirm format yang accepted oleh GL existing Tugure                                          | Akuntansi + IT   | MEDIUM       | Sebelum Phase 5                   |
| DSAK-certified consultant engagement — siapa, kapan, berapa milestone reviews                                              | CFO              | MEDIUM       | Sebelum Phase 6                   |
| Auditor eksternal briefing schedule                                                                                        | CFO + Compliance | LOW          | Phase 8/9                         |
| Initial Pefindo PD upload procedure (manual atau API)                                                                      | Risk Officer     | LOW          | Phase 6                           |

# 5\. Approval & Sign-Off

Dengan menandatangani halaman ini, pihak-pihak menyatakan bahwa keputusan-keputusan di Bab 2 menjadi authoritative reference untuk implementasi BLIPS IFRS 9. Perubahan dari keputusan ini ke depan WAJIB melalui formal Change Request.

**Disusun oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Project Manager BLIPS                                                | IT Architect Lead                                                    |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Direview oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Kepala Akuntansi & Pelaporan                                         | Kepala Risk Management                                               |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |
|                                                                      |                                                                      |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_     | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_     |
| Kepala Treasury                                                      | Kepala IT Security                                                   |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Disetujui oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Direktur Teknologi Informasi                                         | Direktur Keuangan (CFO)                                              |
| Technical Approver                                                   | Sponsor Proyek                                                       |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR DECISION LOG v1.0 ---*
