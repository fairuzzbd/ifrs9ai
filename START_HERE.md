# START HERE — BLIPS IFRS 9 Implementation Guide

Welcome. Dokumen ini adalah **single entry point** untuk memulai implementasi BLIPS IFRS 9 menggunakan agentic AI sebagai team builder dengan tim IT internal Tugure. Baca dokumen ini terlebih dahulu sebelum mulai coding.

**Target Audience:**
- Internal IT team Tugure yang akan jadi developer/reviewer
- Agentic AI tools (Claude Code, Cursor, Aider, dll) yang akan jadi team builder
- Project Manager BLIPS untuk orchestration

---

## 0. Quick Status Check

Sebelum mulai, pastikan kondisi berikut sudah terpenuhi:

| Item | Status | Cara Verifikasi |
|------|--------|-----------------|
| ✅ Dokumen lengkap di `msword/` | DONE | `ls msword/*.docx` |
| ✅ Dokumen markdown di `docs/` | DONE | `ls docs/*.md` |
| ✅ SQL DDL siap eksekusi | DONE | `cat BLIPS_init_schema.sql | head` |
| ✅ Pefindo PD data aktual | DONE | Section 12.8 di SQL |
| ✅ Sample seed data lengkap | DONE | Section 12.10 di SQL |
| ✅ Decision Log 6 keputusan critical | DONE | `docs/BLIPS_Decision_Log_v1.0.md` |
| ⏳ Tech stack tersinstal | TODO | Lihat Bab 2 |
| ⏳ PostgreSQL 18 running | TODO | Lihat Bab 3 |
| ⏳ Git repository initialized | TODO | Lihat Bab 4 |
| ⏳ AI tool dipilih | TODO | Lihat Bab 5 |

---

## 1. Pre-Requisites Software

Install software berikut sebelum mulai (gunakan versi spesifik yang sudah lock di Decision Log DEC-001):

### Backend
- **Go 1.22+** — https://go.dev/dl/ (download installer untuk Windows/Linux/macOS)
- **Air** (hot-reload Go) — `go install github.com/cosmtrek/air@latest`
- **golangci-lint** — https://golangci-lint.run/usage/install/

### Frontend
- **Node.js 20 LTS** — https://nodejs.org
- **pnpm** (recommended package manager) — `npm install -g pnpm`

### Database
- **PostgreSQL 18** — https://www.postgresql.org/download/
- **pgAdmin 4** atau **DBeaver** untuk GUI database

### Infrastructure
- **Docker Desktop** atau **Docker Engine** (untuk container dev environment)
- **Docker Compose** (bundled dengan Docker Desktop)
- **Git** — https://git-scm.com/

### Code Editor (pilih salah satu)
- **VS Code** + Go extension + ESLint extension + Prettier
- **Cursor** (recommended untuk AI-assisted dev) — https://cursor.sh
- **JetBrains GoLand** + WebStorm (untuk yang prefer JetBrains)

### Verifikasi Installation

```bash
go version           # harus go1.22.x
node --version       # harus v20.x
pnpm --version       # harus 9.x atau lebih baru
psql --version       # harus PostgreSQL 18.x
docker --version     # harus Docker Engine 24.x atau lebih baru
git --version        # version any modern
```

---

## 2. Bootstrap PostgreSQL Database

```bash
# 1. Start PostgreSQL 18 service
# (Windows: via Services; Linux: sudo systemctl start postgresql)

# 2. Create database
createdb -U postgres blips_db

# 3. Create application user
psql -U postgres -d blips_db <<EOF
CREATE USER blips_admin WITH PASSWORD 'change_me_in_production';
GRANT ALL PRIVILEGES ON DATABASE blips_db TO blips_admin;
EOF

# 4. Execute initial schema (creates 9 schemas + ~50 tables + indexes + triggers + sample data)
psql -U blips_admin -d blips_db -f BLIPS_init_schema.sql

# 5. Verify
psql -U blips_admin -d blips_db -c "\dn"   # list schemas (should show: aud, doc, ecl, jrnl, mst, public, sec, sppi, sys, trx)
psql -U blips_admin -d blips_db -c "SELECT COUNT(*) FROM mst.instrumen;"  # should return 4 (sample data)
psql -U blips_admin -d blips_db -c "SELECT COUNT(*) FROM mst.pd_pefindo;"  # should return 8 (Pefindo seed)
psql -U blips_admin -d blips_db -c "SELECT COUNT(*) FROM mst.chart_of_accounts;"  # should return 50+
```

**Bila ada error pada DDL execution:**
1. Periksa PostgreSQL version (`SELECT version();`) — minimal 15, recommended 18
2. Periksa extensions tersedia: `pgcrypto`, `btree_gin`, `btree_gist`
3. Bila `uuidv7()` function gagal, edit DDL untuk gunakan `gen_random_uuid()` fallback

---

## 3. Setup Repository Structure

```bash
cd "D:\00 tugure\ifrs_src\blips-ifrs9-ai"

# Initialize Git
git init
git checkout -b main

# Create recommended folder structure
mkdir -p backend/cmd/api backend/internal/{config,domain,handler,middleware,repository,service,worker,migration,util}
mkdir -p backend/pkg
mkdir -p frontend/src/{app,components,lib,hooks,types,api}
mkdir -p frontend/public
mkdir -p deploy/{docker,k8s,terraform,ansible}
mkdir -p scripts
mkdir -p .github/workflows
```

### Recommended Final Structure

```
blips-ifrs9-ai/
├── START_HERE.md              ← THIS FILE
├── README.md                  ← Project README (high-level)
├── CLAUDE.md                  ← Existing — context untuk Claude Code
├── BLIPS_init_schema.sql      ← Database DDL
├── docs/                      ← Markdown specs
├── msword/                    ← Source Word documents
├── backend/                   ← Go backend (Gin/Fiber)
│   ├── cmd/
│   │   └── api/main.go        ← Entry point
│   ├── internal/
│   │   ├── config/
│   │   ├── domain/            ← Entities (mirror dari ERD)
│   │   ├── handler/           ← HTTP handlers (per modul)
│   │   ├── middleware/        ← Auth, logging, audit
│   │   ├── repository/        ← Database access layer
│   │   ├── service/           ← Business logic (per modul)
│   │   ├── worker/            ← Async jobs (MTM, ECL, Akrual)
│   │   ├── migration/         ← Migration scripts (Flyway-equivalent)
│   │   └── util/
│   ├── pkg/                   ← Public reusable packages
│   ├── go.mod
│   ├── go.sum
│   └── Dockerfile
├── frontend/                  ← Next.js 14 (App Router, TypeScript)
│   ├── src/
│   │   ├── app/               ← Pages (App Router)
│   │   ├── components/        ← React components
│   │   ├── lib/               ← Utils
│   │   ├── hooks/             ← Custom React hooks
│   │   ├── types/             ← TypeScript types
│   │   └── api/               ← API client (fetch wrapper)
│   ├── public/
│   ├── package.json
│   ├── tsconfig.json
│   ├── next.config.js
│   └── Dockerfile
├── deploy/
│   ├── docker/
│   │   └── docker-compose.dev.yml  ← Dev environment
│   ├── k8s/                   ← Kubernetes manifests (production)
│   ├── terraform/             ← Infrastructure as Code
│   └── ansible/               ← Configuration management
├── scripts/
│   ├── seed-dev.sh            ← Seed database for dev
│   ├── regen-docs.sh          ← Regenerate markdown from docx
│   └── ...
├── .github/workflows/
│   └── ci.yml                 ← GitLab CI (atau GitHub Actions)
├── .gitignore
├── .editorconfig
└── LICENSE
```

### Initial .gitignore

```gitignore
# Go
backend/bin/
backend/tmp/
*.exe

# Node
frontend/node_modules/
frontend/.next/
frontend/out/
.env.local
.env.production

# IDE
.vscode/
.idea/
*.swp

# OS
.DS_Store
Thumbs.db

# Microsoft Word lock files
~$*

# Logs
*.log

# Database
*.dump
*.sql.gz

# Sensitive
.env
secrets/
```

---

## 4. AI Tool Selection

Berdasarkan workflow agentic AI yang Anda rencanakan, berikut rekomendasi tool stack:

### Primary AI Coding Assistant

**Pilihan A: Claude Code (Recommended)**
- Installation: https://docs.claude.com/claude-code
- Strengths: Long context (200K tokens), excellent untuk multi-file refactor, native MCP support, terminal-native workflow
- Use case: Backend Go service implementation, schema migrations, ECL/EIR business logic
- Setup: `npm install -g @anthropic-ai/claude-code` lalu `claude` di terminal di folder project

**Pilihan B: Cursor**
- Installation: https://cursor.sh
- Strengths: IDE-native, good untuk Next.js frontend, inline AI suggestions
- Use case: Frontend Next.js development, UI components, form handling

**Pilihan C: Aider (open-source alternative)**
- Installation: `pip install aider-chat`
- Strengths: Git-integrated, terminal-based, support multiple models
- Use case: Bulk refactor across files

### Recommended Tool Mix untuk BLIPS

| Phase | Tool | Reason |
|-------|------|--------|
| Database design, migrations, DDL | Claude Code | Excellent SQL + multi-file refactor |
| Backend services Go (Master Data) | Claude Code atau Cursor | Long context untuk modul besar |
| Backend ECL/EIR compliance core | Claude Code (dengan DSAK reviewer manual) | Long context untuk formula correctness |
| Frontend Next.js screens | Cursor | IDE-native UI dev |
| Test case generation | Claude Code | Read BRD acceptance criteria → generate Jest/Go tests |
| Code review automation | Claude Code | Multi-file context untuk consistency check |

### Context Setup untuk AI Agent

Saat agent dijalankan di project folder, pastikan tersedia:

1. **`CLAUDE.md`** di root (sudah ada) — high-level project context
2. **`docs/` folder** dengan semua markdown — agent baca on-demand per modul
3. **`BLIPS_init_schema.sql`** — schema reference
4. **`START_HERE.md`** (file ini) — workflow guide

Pada start session AI, beri context awal:

```
Saya membangun sistem BLIPS IFRS 9 untuk PT Tugu Reasuransi Indonesia.

Tech stack: Golang 1.22 (Gin) + Next.js 14 + PostgreSQL 18. On-premise deployment.

Baca dulu dokumen referensi berikut sebelum mulai task:
1. docs/BLIPS_Decision_Log_v1.0.md (constraints kunci)
2. docs/SoW_v1.4.md (scope keseluruhan)
3. docs/FSD-BLIPS-MASTER-v1.1.md (tech standards & arsitektur)
4. docs/ERD-BLIPS-IFRS9-v1.2.md (database schema)

Spesifik untuk task ini, baca juga:
- [docs/FSD-APP-X-...md] yang relevan dengan modul yang sedang dikerjakan

Lalu mulai kerjakan: [TASK DESCRIPTION]
```

---

## 5. Order of Implementation

Mengikuti SoW v1.4 §11.2 Milestone, ditambah modifikasi karena GL integration deferred (DEC-005):

### Phase 0 — Bootstrap (1 minggu) — DO THIS FIRST
- [ ] Setup PostgreSQL 18 + execute DDL
- [ ] Create Git repo dengan struktur folder
- [ ] Initialize Go module: `cd backend && go mod init blips-ifrs9.tugu-re.com`
- [ ] Initialize Next.js app: `cd frontend && pnpm create next-app . --typescript --tailwind --app --eslint`
- [ ] Setup `docker-compose.dev.yml` (PostgreSQL + Redis + MinIO)
- [ ] Setup CI/CD pipeline skeleton
- [ ] Setup linting (golangci-lint, ESLint)
- [ ] First "Hello World" Go API endpoint `/healthz`
- [ ] First Next.js page yang call ke `/healthz`
- [ ] Commit baseline

### Phase 1 — Discovery Refinement (1 minggu, parallel ke Phase 0)
- [ ] Agent: read all docs/ markdown files
- [ ] Agent: identify questions / ambiguities (sudah 90% resolved via Decision Log)
- [ ] Agent: generate detailed task breakdown per modul → Jira/Linear/GitHub Issues

### Phase 2 — Foundation Layer (2 minggu)
Building blocks yang dipakai semua modul:

- [ ] **Sec (Auth & RBAC)** — JWT auth, middleware, 10 standard roles
- [ ] **Audit Trail** — append-only trigger, audit log middleware
- [ ] **Workflow Engine** — generic Maker-Reviewer-Approver framework
- [ ] **Notification Service** — email + in-app notification
- [ ] **Document Upload Service** — MinIO integration, virus scan, SHA-256 hash
- [ ] **Common middleware** — logging, request ID, error handling
- [ ] Unit tests untuk semua foundation modules

### Phase 3 — Master Data Module (3 minggu)
Reference: `docs/FSD-APP-A-MasterData-SPPI-BM-v1.1.md`

Order rekomendasi:
1. mst.mata_uang (paling simple)
2. mst.periode_buku (sudah ada seed)
3. mst.lgd_basel, mst.bobot_skenario, mst.lps_coverage (static reference)
4. mst.pd_pefindo (dengan upload workflow)
5. mst.counterparty + rating_history
6. mst.chart_of_accounts (dengan import Excel)
7. mst.mapping_jurnal_header + detail
8. mst.portofolio
9. mst.instrumen (paling kompleks — FK ke banyak master)
10. mst.kurs + BI JISDOR scheduled job

Setiap modul: CRUD endpoint + workflow approval + UI screen + audit trail + test.

### Phase 4 — SPPI/BM Test + Klasifikasi Engine (2 minggu)
Reference: `docs/FSD-APP-A-MasterData-SPPI-BM-v1.1.md` (Bab 2-6)
- SPPI Test 10-question engine
- BM Test indicator-based engine
- Klasifikasi matriks auto-derivation
- Pre-trade clearance workflow
- Reklasifikasi prospektif (6 kombinasi)

### Phase 5 — Transaction Lifecycle (3 minggu)
Reference: `docs/FSD-APP-B-TransactionLifecycle-v1.1.md`
- Penempatan (dengan EIR computation trigger)
- MTM (dengan upload batch — Bab 8 FSD-B)
- Bulk Upload Master Instrumen (Bab 9 FSD-B)
- Renewal, Penjualan, Jatuh Tempo
- Pendapatan Akrual (daily job dengan EIR)
- Media Upload integration

### Phase 6 — ECL Engine + EIR & Amortisasi (3 minggu) — COMPLIANCE CRITICAL
Reference: `docs/FSD-APP-C-ECL-EIR-v1.0.md`

**⚠️ Wajib DSAK-certified reviewer untuk phase ini.** Setiap PR harus dapat sign-off dari Akuntansi senior.

- Newton-Raphson IRR solver untuk EIR
- Amortization Schedule generation
- EIR Re-estimation flow
- ECL 3-stage model dengan SICR/Default trigger
- 3-skenario PD computation
- Dual forward-looking layer (Impact MEV + Impact PD)
- LPS Aggregator
- Look-through Reksadana
- Monthly ECL batch job
- Stage migration jurnal posting

### Phase 7 — Periode Buku + FX + Jurnal Internal (2 minggu)
Reference: `docs/FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.md`
- Periode Buku state machine
- FX Rate Management + BI JISDOR scheduled sync
- Master Mapping Jurnal resolusi runtime
- Jurnal internal posting (TANPA GL Host integration — DEC-005)
- Export file CSV/XLSX untuk Akuntansi manual ke GL legacy

### Phase 8 — Reporting & Dashboard (2 minggu)
Reference: `docs/FSD-APP-E-Reporting-v1.0.md`
- Materialized views untuk reports
- 28 reports implementation (priority H first)
- Dashboard per role
- Excel/PDF export
- Scheduled email reports

### Phase 9 — SIT (2 minggu)
- End-to-end integration testing
- Performance testing (1.500 instrumen, 100 concurrent users)
- Security testing
- Defect remediation

### Phase 10 — UAT (3 minggu)
- Business user testing (Treasury, Risk, Akuntansi)
- Acceptance criteria verification
- DSAK-certified review untuk compliance modules
- Auditor walkthrough simulation

### Phase 11 — Production Deployment + Hypercare (4 minggu)
- Data migration dari sistem legacy (replace sample data dengan actual Tugure data)
- Production go-live
- 4 minggu hypercare dengan dedicated standby

**Total estimasi: 25 minggu (6 bulan)** — lebih singkat dari SoW v1.4 estimate karena GL integration deferred.

---

## 6. Prompt Templates untuk AI Agent

### Template 1: New Module Implementation

```
Task: Implementasi Modul [NAMA_MODUL] backend Go service.

Reference Documents (baca dulu):
- docs/FSD-APP-[X].md (full module spec)
- docs/ERD-BLIPS-IFRS9-v1.2.md (skim section database schema untuk modul ini)
- docs/BLIPS_Decision_Log_v1.0.md (tech stack constraints)
- backend/internal/domain/ (existing entities — follow pattern)

Acceptance Criteria (dari BRD):
- BR-XXX-001: [paste content]
- BR-XXX-002: [paste content]
- BR-XXX-003: [paste content]

Implementation Order:
1. Domain entity di backend/internal/domain/[module].go (mirror dari ERD)
2. Repository interface + PostgreSQL implementation di internal/repository/
3. Service layer dengan business rules di internal/service/
4. HTTP handlers di internal/handler/ — REST endpoints sesuai FSD §X.X API spec
5. Unit tests untuk service layer (testify + gomock)
6. Integration test dengan testcontainers-go
7. Validation: pastikan semua acceptance criteria pass

Constraints:
- Follow FSD Master §6 API Standards (REST + JSON conventions)
- Follow FSD Master §8 Error Handling (ERR-XXX-#### convention)
- Audit trail wajib untuk semua mutation (CREATE/UPDATE/DELETE)
- Workflow approval untuk transaction material — gunakan Workflow Engine yang sudah ada

Output:
- Code files dengan tests passing
- Brief summary perubahan yang dilakukan
- TODO list untuk frontend integration (akan dikerjakan task terpisah)
```

### Template 2: Frontend Page Implementation

```
Task: Build Next.js page untuk [MODUL_SCREEN_NAME].

Reference:
- docs/FSD-APP-[X].md §[X.Y] Screen Flow & UI Mockup (ASCII art mockup)
- docs/FSD-BLIPS-MASTER-v1.1.md §7 UI/UX Standards (color palette, typography, form patterns)
- frontend/src/components/ (existing components — reuse)

Acceptance Criteria:
- Form validation per FSD field specs
- Error handling per FSD Master §8 error code
- Workflow status display sesuai pattern §7.6
- Table dengan pagination, filter, sort sesuai pattern §7.5
- Accessibility WCAG 2.1 Level AA

Implementation:
1. Create page di src/app/[modul]/[screen]/page.tsx
2. Server component untuk data fetching
3. Client component untuk interactive form (react-hook-form + zod)
4. API client di src/api/[modul].ts (fetch wrapper)
5. TypeScript types di src/types/[modul].ts
6. Test dengan React Testing Library

Constraints:
- TypeScript strict mode
- Tailwind CSS untuk styling
- shadcn/ui untuk komponen baseline
- Server Component default, Client Component hanya untuk interactivity

Output:
- Page yang ter-render dengan correct data
- Brief PR description
```

### Template 3: Business Logic / Algorithm Implementation

```
Task: Implementasi [ALGORITHM_NAME] (mis. Newton-Raphson EIR solver).

Reference:
- docs/FSD-APP-C-ECL-EIR-v1.0.md §[X.Y] (full algorithm pseudocode)
- docs/SoW_v1.4.md §[X.Y] (business context dan numerical example)

Spec:
- Input: [paste from FSD]
- Output: [paste from FSD]
- Edge cases: [list]
- Sample data: [paste numerical example dari SoW]

Implementation:
1. Pure function di backend/internal/util/ atau internal/service/[modul]_algorithm.go
2. Comprehensive unit tests termasuk:
   - Happy path dengan sample data dari FSD/SoW (assert output match)
   - Edge cases (overflow, convergence failure, negative input)
   - Performance benchmark (Newton-Raphson harus konvergen ≤50 iterasi)
3. Fallback strategy bila primary algorithm fail (mis. bisection bila Newton-Raphson tidak konvergen)
4. Documentation: GoDoc comment dengan link ke FSD section

Constraints:
- COMPLIANCE-CRITICAL: numerical precision 8 desimal internal, 4 desimal display
- Tidak ada pembulatan intermediate
- Banker's rounding (round-half-to-even) untuk pembulatan final

Output:
- Code dengan tests passing
- Computational verification: output dengan sample input dari FSD harus match exactly
- Notice ke human reviewer untuk DSAK-certified accountant review sebelum merge
```

### Template 4: Database Migration

```
Task: Buat migration script untuk [PERUBAHAN_SCHEMA].

Reference:
- docs/ERD-BLIPS-IFRS9-v1.2.md (existing schema)
- BLIPS_init_schema.sql (current baseline)

Output:
- File: backend/internal/migration/V{NEXT_NUMBER}__{description}.sql
- Up migration: ALTER/CREATE statements
- Down migration: REVERSE statements (untuk rollback)
- Test migration di local PostgreSQL: dry-run + verify schema state

Constraints:
- Idempotent: bisa di-run multiple times tanpa error (gunakan IF NOT EXISTS / IF EXISTS)
- Backward compatible: existing data tidak corrupt
- Update ERD docx + markdown SETELAH migration approved
```

---

## 7. Quality Gates per Phase

Sebelum lanjut ke phase berikutnya, semua kriteria ini WAJIB pass:

### Phase 0 → 1
- [ ] PostgreSQL DDL execute clean (no errors)
- [ ] Backend `/healthz` return 200
- [ ] Frontend bisa fetch `/healthz` cross-origin
- [ ] CI pipeline green
- [ ] First commit di Git

### Phase 2 → 3
- [ ] Auth flow working (login dengan sample user)
- [ ] Audit log auto-populate saat ada mutation
- [ ] Workflow engine bisa transition state DRAFT → PENDING → APPROVED
- [ ] Document upload + SHA-256 hash verified

### Phase 3 → 4
- [ ] Semua master data CRUD passing
- [ ] Master CoA + Mapping Jurnal terbentuk dari sample data
- [ ] Periode buku 2026 generated

### Phase 4 → 5
- [ ] SPPI Test 10-question engine return correct PASS/FAIL untuk 10+ test cases
- [ ] BM Test auto-suggest correctly
- [ ] Klasifikasi matriks output match expected untuk semua kombinasi

### Phase 5 → 6
- [ ] Penempatan + EIR computation working
- [ ] MTM daily job complete dalam <30 menit untuk 1.500 instrumen
- [ ] Bulk upload Master Instrumen working end-to-end

### Phase 6 → 7 (COMPLIANCE GATE)
- [ ] EIR sample case match SoW §5.12.11 (Carrying Rp 5.080M → EIR 4,8267% → Closing Carrying ≈ 0)
- [ ] ECL sample case match SoW §8.2.2 (Obligasi PT XYZ → ECL FL Rp 6.565.781,25)
- [ ] LPS Aggregator sample match SoW §8.1.3 (Bank Mandiri → ECL FL Rp 107.812,50 + Rp 215.625,00)
- [ ] Look-through Reksadana match SoW §8.3.2
- [ ] Stage migration trigger working (rating downgrade test)
- [ ] **DSAK-certified accountant sign-off** ⚠️

### Phase 7 → 8
- [ ] Periode buku state machine working (OPEN → SOFT_CLOSED → CLOSED)
- [ ] FX rate sync dari BI JISDOR working
- [ ] Mapping Jurnal resolusi runtime balance (Σ D = Σ K)
- [ ] Export jurnal CSV/XLSX siap

### Phase 8 → 9 (SIT)
- [ ] 28 reports rendering
- [ ] Dashboard refresh < 5 detik
- [ ] Excel export working untuk 10k rows
- [ ] Materialized view refresh < 30 menit

### Phase 9 → 10 (UAT)
- [ ] SIT defect 0 critical, ≤ 5 high open
- [ ] Performance test PASS
- [ ] Security pen-test 0 critical, 0 high
- [ ] All UAT scripts ready

### Phase 10 → 11 (Production)
- [ ] UAT sign-off oleh CFO
- [ ] Production env provisioned (on-premise)
- [ ] Backup + DR drill done
- [ ] Rollback plan documented
- [ ] CEO/Steering approval

---

## 8. First Task — Recommended Starting Point

Untuk first AI agent run, mulai dengan **Phase 0 bootstrap** yang paling concrete dan low-risk:

```
Saya membangun sistem BLIPS IFRS 9 untuk PT Tugu Reasuransi Indonesia.

Context:
- Tech stack: Golang 1.22 (Gin) + Next.js 14 + PostgreSQL 18, on-premise.
- Repository di D:\00 tugure\ifrs_src\blips-ifrs9-ai
- Decision Log + dokumen lengkap di folder docs/

TASK PERTAMA: Bootstrap Phase 0.

Buat:

1. backend/cmd/api/main.go — Gin HTTP server dengan single endpoint GET /healthz return {"status":"ok","timestamp":"<now>"}
2. backend/internal/config/config.go — load config dari env (DB_URL, SERVER_PORT, etc.)
3. backend/go.mod dengan dependencies: gin-gonic/gin, jmoiron/sqlx, joho/godotenv, golang-jwt/jwt
4. backend/Dockerfile (multi-stage build)
5. backend/.air.toml untuk hot reload

6. frontend/src/app/page.tsx — Next.js page yang fetch /healthz dan display status
7. frontend/src/lib/api.ts — fetch wrapper dengan base URL dari env
8. frontend/.env.local dengan NEXT_PUBLIC_API_URL
9. frontend/Dockerfile

10. deploy/docker/docker-compose.dev.yml dengan services:
    - postgres (port 5432, image postgres:18, mount BLIPS_init_schema.sql ke /docker-entrypoint-initdb.d/)
    - redis (port 6379, image redis:7-alpine)
    - minio (port 9000, image minio/minio:latest, basic config)
    - backend (build dari backend/)
    - frontend (build dari frontend/)

11. .gitignore yang sesuai (Go + Node + lock files)

12. README.md di root dengan instruksi:
    - cd ke project, jalankan `docker-compose -f deploy/docker/docker-compose.dev.yml up -d`
    - Verify: curl http://localhost:8080/healthz
    - Verify: open http://localhost:3000

Goal: Setelah commit, anggota tim lain bisa clone repo, jalankan docker-compose, dan langsung melihat aplikasi running.
```

Setelah Phase 0 berhasil, lanjut ke Phase 2 (foundation layer) sesuai order di Bab 5.

---

## 9. Best Practices untuk AI-Driven Development

### Do:
- ✅ **Selalu reference ke dokumen** — "Sesuai FSD §X.Y" lebih authoritative dibanding asumsi
- ✅ **Verify dengan sample data** — Setelah modul jadi, run terhadap sample seed di SQL DDL
- ✅ **Small commits** — Per modul / per feature, bukan per phase
- ✅ **Test coverage minimum 70%** — Sesuai NFR-MAIN-04
- ✅ **Code review** — Even AI-generated code butuh manusia review, terutama compliance
- ✅ **Sync markdown setelah Word update** — Run regen-docs.sh setelah edit dokumen Word

### Don't:
- ❌ **Skip Decision Log** — 6 keputusan critical jangan di-override tanpa formal CR
- ❌ **Generate compliance code tanpa DSAK review** — Phase 6 mandatory human review
- ❌ **Bypass workflow** — Maker-Reviewer-Approver wajib, jangan shortcut
- ❌ **Hardcode tenant data** — Sample seed boleh hardcode, production data via migration
- ❌ **Skip audit trail** — Setiap mutation HARUS ter-log
- ❌ **Edit DDL langsung di produksi** — Selalu via migration script versioned

---

## 10. Where to Get Help

| Topik | Reference |
|-------|-----------|
| Tech stack decisions | `docs/BLIPS_Decision_Log_v1.0.md` |
| Business requirements | `docs/BRD_BLIPS_IFRS9_v1.1.md` |
| Scope & flow | `docs/SoW_v1.4.md` |
| Tech architecture & standards | `docs/FSD-BLIPS-MASTER-v1.1.md` |
| Module-specific spec | `docs/FSD-APP-A/B/C/D/E-*.md` |
| Database schema | `docs/ERD-BLIPS-IFRS9-v1.2.md` + `BLIPS_init_schema.sql` |
| Pefindo PD source | `msword/Pefindo_Annual_Default_Study_2007-2025_EN.pdf` |

### Decision Authority Escalation

- **Technical decisions** → IT Architect Lead
- **Business interpretation** → Working Group BLIPS + Business Analyst
- **Compliance interpretation (PSAK 71)** → DSAK-certified Accountant + Akuntansi senior
- **Scope changes** → Steering Committee via formal Change Request
- **Production incidents** → On-call rotation + escalation matrix

---

## 11. Document Maintenance

Bila ada perubahan dokumen Word di `msword/`, regenerate markdown:

```bash
# Save sebagai scripts/regen-docs.sh
#!/bin/bash
cd "$(dirname "$0")/.."
for f in msword/*.docx; do
  base=$(basename "$f" .docx)
  pandoc "$f" -t gfm --wrap=preserve -o "docs/${base}.md"
  echo "✓ ${base}.md"
done
```

Jalankan: `bash scripts/regen-docs.sh`

---

## 12. Final Checklist Before Coding

- [ ] Software prerequisites installed (Go 1.22, Node 20, PostgreSQL 18, Docker)
- [ ] Database bootstrapped dengan `BLIPS_init_schema.sql`
- [ ] Sample data verified (`SELECT COUNT(*) FROM mst.instrumen;` returns 4)
- [ ] Git repository initialized di workspace
- [ ] AI tool dipilih dan installed (Claude Code recommended)
- [ ] Tim internal IT briefed: PM, IT Architect, Backend, Frontend, DBA, QA
- [ ] DSAK-certified consultant identified untuk Phase 6 review
- [ ] Schedule kick-off meeting dengan Working Group BLIPS

Setelah semua checked ✅, run **First Task** di Bab 8 dengan AI agent. Selamat mulai membangun BLIPS IFRS 9! 🚀

---

**Dokumen ini di-maintain oleh:** Working Group BLIPS
**Versi:** 1.0 — 02 Mei 2026
**Untuk pertanyaan:** Konsultasi dengan IT Architect Lead atau Project Manager BLIPS
