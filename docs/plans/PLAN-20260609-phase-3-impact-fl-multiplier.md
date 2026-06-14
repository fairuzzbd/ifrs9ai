# PLAN-20260609 — Phase 3 Modul 6/12: `mst.impact_mev_pd` + `mst.impact_pd` (Forward-Looking Multiplier)

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-09
**Branch base**: `develop` (setelah PR #16 pd_pefindo merge)
**Working branch**: `feature/phase-3-fl-multiplier`
**Sumber kebenaran**:
- `docs/handoff/RESUME-phase-3.md` §3 row 6
- `BLIPS_Decision_Log_v1.0.docx` DEC-010 (dual FL multiplier, default bobot), DEC-016 (presisi NUMERIC(10,8)), DEC-027 (step-up MFA approve)
- `.claude/memory/formulas.md` §ECL formula (FL_multiplier applied per skenario)
- `db/migrations/000001_init_schema.up.sql` §4.13–4.14 (existing `mst.impact_mev_pd` + `mst.impact_pd`)
- `db/migrations/000008_mata_uang_schema_fix.up.sql` (WORKFLOW_CONFIG_IMPACT_MEV_PD + WORKFLOW_CONFIG_IMPACT_PD sudah di-seed)
- `backend/internal/master/bobotskenario/` — pola terdekat (ECL param dengan skenario enum, period-aware)
- `backend/internal/master/lpscoverage/` — pola 6-eyes + BeforeCommit hook
- `backend/internal/master/pdpefindo/` — pola terbaru (termasuk upload.go)
- `docs/plans/PLAN-20260608-phase-3-pd-pefindo.md` — template plan layout
**Klasifikasi**: feature — **path regulated ECL parameter** → ifrs9-compliance-reviewer BLOCKING gate sebelum merge
**Kompleksitas modul**: MEDIUM-HIGH (dual-table: MEV→PD mapping + per-skenario multiplier; saling terkait secara semantik; 6-eyes dual step-up MFA; visualisasi multiplier chart)
**Target**: ~1.5 minggu setelah PR pd_pefindo merge

---

## 1. Goal

Implementasi end-to-end modul Forward-Looking (FL) Multiplier untuk ECL:

1. **`mst.impact_mev_pd`** — tabel MEV-to-PD impact: Macro Economic Variable (GDP growth, BI rate, inflation, dll.) dipetakan ke PD adjustment multiplier per skenario (GOOD/BAD). Ini adalah "MEV component" dari dual FL multiplier (DEC-010).
2. **`mst.impact_pd`** — tabel FL PD multiplier per periode buku: satu baris per `(periode_id, skenario)` berisi `impact_multiplier` yang di-apply ke `ECL_skenario = EAD × PD × LGD × impact_multiplier`. Ini adalah "final FL applied" value yang dikonsumsi ECL engine.
3. **6-eyes ECL parameter workflow** pada kedua tabel (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED) dengan dua step-up MFA (DEC-027).
4. **Single-active invariant per periode** — dua APPROVED rows tidak boleh overlap pada `periode_id` yang sama.
5. **Konsumsi ECL engine** — `GET /master/impact-pd/active?periode_id=...` adalah data source untuk ECL calc run Phase 4 (FL multiplier lookup per skenario).

**Exit criteria**: migration 0015 up/down idempotent; TC-001..TC-014 hijau di CI; UAT signed ROLE-RISK + ROLE-ALCO; compliance VERDICT = PASS atau CONDITIONAL-PASS; CI hijau.

---

## 2. Analisis Schema Existing (migration 0001)

### `mst.impact_mev_pd` (0001 §4.13)

Kolom existing:
- `id UUID PK`, `periode_id UUID → mst.periode_buku(id)`, `skenario VARCHAR(20)`, `impact_multiplier NUMERIC(8,4)`, `mev_components_json JSONB`, `catatan TEXT`, `dokumen_pendukung_id UUID`, `maker_id UUID → sec.user(id)`, `approver_id UUID → sec.user(id)`, `created_at TIMESTAMPTZ`, `approved_at TIMESTAMPTZ`
- UNIQUE: `(periode_id, skenario)`, CHECK: `skenario IN ('GOOD','BAD')` — hanya GOOD dan BAD (NORMAL tidak ada MEV impact? — ini harus diklarifikasi, lihat OQ-1)

**Missing** (sama dengan pola sebelumnya): `created_by`, `updated_at/by`, `deleted_at/by`, `row_version`, `tenant_id`, `workflow_status`, `workflow_instance_id`
**Precision salah**: `NUMERIC(8,4)` → harus `NUMERIC(10,8)` per DEC-016

### `mst.impact_pd` (0001 §4.14)

Kolom existing:
- `id UUID PK`, `periode_id UUID → mst.periode_buku(id)`, `impact_multiplier NUMERIC(8,4) DEFAULT 1.0000`, `catatan TEXT`, `dokumen_pendukung_id UUID`, `maker_id UUID → sec.user(id)`, `approver_id UUID → sec.user(id)`, `created_at TIMESTAMPTZ`, `approved_at TIMESTAMPTZ`
- UNIQUE: `(periode_id)`, CHECK: `impact_multiplier BETWEEN 0.5 AND 2.0`
- **Catatan**: tabel ini hanya satu row per periode (tidak per-skenario). Ini flat multiplier. Hubungannya dengan `impact_mev_pd` harus diklarifikasi (lihat OQ-1 dan OQ-2).

**Missing** (sama): audit cols, `workflow_status`, `workflow_instance_id`
**Precision salah**: `NUMERIC(8,4)` → harus `NUMERIC(10,8)` per DEC-016

### WORKFLOW_CONFIG sudah di-seed di migration 0008

`WORKFLOW_CONFIG_IMPACT_MEV_PD` dan `WORKFLOW_CONFIG_IMPACT_PD` sudah ada di `sys.config` dari migration 0008. Config menggunakan permission `ecl_parameter.*` (bukan entity-specific). Migration 0015 **tidak perlu** seed ulang config ini — cukup backfill schema.

### Keputusan desain (data-modeler confirm)

**Tidak perlu tabel baru** — berbeda dari pd_pefindo yang memerlukan redesign total, kedua tabel ini cukup diretrofit audit cols + precision + workflow_status. **Migration 0015 adalah schema-fix migration** (pola 0010/0011/0012).

---

## 3. Decision Log Check

| DEC | Implikasi modul ini |
|---|---|
| DEC-010 | ECL formula: `ECL_FL_skenario = EAD × PD × LGD × Impact_PD_multiplier_skenario`. FL multiplier adalah `impact_pd.impact_multiplier` per skenario. ALCO dapat override per periode. Default weights 0.25/0.50/0.25 di `bobot_skenario` — modul ini adalah FL adjustment diatasnya. |
| DEC-016 | `impact_multiplier NUMERIC(10,8)`; no float64; shopspring/decimal di Go. Existing schema `NUMERIC(8,4)` harus di-fix. |
| DEC-021 | Idempotency-Key wajib di semua mutating endpoints. |
| DEC-022 | Cursor pagination (no offset). |
| DEC-027 | Step-up MFA untuk approve (Approver1) dan approve2 (Approver2). Kedua sudah configured di WORKFLOW_CONFIG_IMPACT_MEV_PD + WORKFLOW_CONFIG_IMPACT_PD. |
| DEC-017 | 6-eyes workflow, SoD: maker ≠ reviewer ≠ approver ≠ approver2. |

**Locked decisions** yang TIDAK boleh diubah:
- Presisi `NUMERIC(10,8)` — refuse downgrade.
- Step-up MFA di kedua approve step (DEC-027).
- WORKFLOW_CONFIG sudah di-seed di 0008 — **jangan** seed ulang atau modifikasi via migration baru.

---

## 4. Story Breakdown

### APP-A-MSTR-008-01 — Input MEV Impact (Maker) — `impact_mev_pd`

**Actor**: ROLE-RISK (Maker)
**Story**: Sebagai Risk Officer, saya ingin menginput mapping MEV-to-PD impact untuk skenario GOOD dan BAD per periode buku, agar sistem dapat menghitung FL multiplier ECL sesuai kondisi makroekonomi.
**AC (singkat)**:
- GIVEN: Risk Officer login, GET `/master/impact-mev-pd/new` → form input
- WHEN: submit `{ periode_id, skenario: GOOD|BAD, impact_multiplier, mev_components_json (optional), catatan (optional) }` dengan Idempotency-Key
- THEN: `201 Created` + entity DRAFT
- AND: `audit event IMPACT_MEV_PD.CREATE` ditulis ke `aud.audit_log` di transaksi yang sama
- AND: SoD constraint pre-checked saat submit/review/approve
- AND: `(periode_id, skenario)` unique enforced — duplicate → `CONFLICT` 409

**Validasi**:
- `skenario` WAJIB enum `GOOD` atau `BAD` (per schema 0001 CHECK — lihat OQ-1)
- `impact_multiplier` NUMERIC(10,8), > 0 (tidak ada range cap yang jelas di 0001 untuk MEV; lihat OQ-3)
- `periode_id` harus ada di `mst.periode_buku`

---

### APP-A-MSTR-008-02 — Input FL PD Multiplier (Maker) — `impact_pd`

**Actor**: ROLE-RISK (Maker)
**Story**: Sebagai Risk Officer, saya ingin menginput FL PD multiplier per periode buku, yang akan diapply ke kalkulasi ECL sebagai `impact_multiplier` dalam `ECL_FL_skenario = ECL_skenario × impact_multiplier`.
**AC (singkat)**:
- GIVEN: Risk Officer login, GET `/master/impact-pd/new` → form input
- WHEN: submit `{ periode_id, impact_multiplier, catatan (optional) }` dengan Idempotency-Key
- THEN: `201 Created` + entity DRAFT
- AND: `audit event IMPACT_PD.CREATE` ditulis
- AND: `(periode_id)` unique enforced — duplicate → `CONFLICT` 409

**Validasi**:
- `impact_multiplier` NUMERIC(10,8), CHECK `BETWEEN 0.5 AND 2.0` (per schema 0001) — enforce di service layer juga
- `periode_id` harus ada di `mst.periode_buku`

---

### APP-A-MSTR-008-03 — Review (Reviewer)

**Actor**: ROLE-AKUN-CTL (Reviewer)
**Story (berlaku untuk kedua entitas)**: Sebagai Finance Controller, saya ingin mereview input MEV impact dan FL PD multiplier sebelum dikirim ke approval, untuk memastikan nilai konsisten dengan kondisi ekonomi dan kebijakan ALCO.
**AC (singkat)**:
- Reviewer GET `/master/impact-mev-pd/{id}` atau `/master/impact-pd/{id}` → tampil detail + nilai multiplier
- POST `/{id}/review` → `workflow_status` = `PENDING_APPROVAL`
- SoD: reviewer ≠ maker → `SOD_VIOLATION` 403
- Reject dengan komentar min 10 karakter → `workflow_status` = `REJECTED`/`RETURNED`

---

### APP-A-MSTR-008-04 — Approve1 + step-up MFA (Approver1 = ROLE-RISK)

**Actor**: ROLE-RISK (Approver1 — beda user dari Maker)
**Story (berlaku untuk kedua entitas)**:
**AC (singkat)**:
- ROLE-RISK POST `/{id}/approve` dengan `signatureMethod = JWT_STEP_UP`
- Backend verify `stepup_token` valid (fresh, < 5 menit)
- SoD: Approver1 ≠ Maker ≠ Reviewer
- On success: `workflow_status` = `PENDING_APPROVAL_2`
- `sys.workflow_signature` row ditulis

---

### APP-A-MSTR-008-05 — Approve2 + step-up MFA (Approver2 = ROLE-ALCO)

**Actor**: ROLE-ALCO (Approver2)
**Story (berlaku untuk kedua entitas)**:
**AC (singkat)**:
- ROLE-ALCO POST `/{id}/approve2` dengan `signatureMethod = JWT_STEP_UP`
- Backend verify step-up MFA fresh
- SoD: Approver2 ≠ Maker ≠ Reviewer ≠ Approver1 (`approver2NotAnyPrevious = true`)
- On success: `workflow_status` = `APPROVED`
- Single-active invariant per `periode_id` enforced sebelum commit
- Notifikasi in-app ke Maker

---

### APP-A-MSTR-008-06 — Lookup Aktif (ECL Engine consumer)

**Actor**: System (ECL calc run Phase 4) + ROLE-RISK, ROLE-AUDIT
**Story**:
**AC (singkat)**:
- `GET /master/impact-pd/active?periode_id=...` → return APPROVED row untuk periode tsb
- `GET /master/impact-mev-pd/active?periode_id=...` → return APPROVED rows per skenario
- Permission: `ecl_parameter.read`
- Response contract: compatible dengan ECL engine Phase 4 (placeholder wiring)

---

### APP-A-MSTR-008-07 — List + History + Export

**Actor**: ROLE-RISK, ROLE-AKUN-CTL, ROLE-AUDIT
**AC (singkat)**:
- `GET /master/impact-mev-pd` → DataTable (UX §1): sort + cursor paging + filter + export CSV/XLSX
- `GET /master/impact-pd` → DataTable (UX §1): sort + cursor paging + filter + export CSV/XLSX
- Filter: `workflow_status`, `periode_id`, `skenario` (untuk impact_mev_pd)
- Export: CSV/XLSX respects active filter. Audit `IMPACT_MEV_PD.EXPORT` / `IMPACT_PD.EXPORT`

---

## 5. DB Schema (delegate ke data-modeler)

### Migration: `000015_impact_fl_multiplier_schema_fix.up.sql` + `.down.sql`

**Requires**: 0001, 0007, 0008, 0014

**Tag migration**:
```sql
-- migration: 0015 impact_fl_multiplier_schema_fix
-- author: data-modeler
-- requires: 0001, 0007, 0008, 0014
-- description: Retrofit mst.impact_mev_pd + mst.impact_pd with:
--   (a) missing audit cols (created_by, updated_at/by, deleted_at/by, row_version, tenant_id)
--   (b) workflow_status + CHECK constraint + workflow_instance_id FK
--   (c) precision fix: impact_multiplier NUMERIC(8,4) → NUMERIC(10,8) per DEC-016
--   (d) indexes: tenant+created, workflow status hot-path, active lookup per periode
--   WORKFLOW_CONFIG_IMPACT_MEV_PD + WORKFLOW_CONFIG_IMPACT_PD already seeded in 0008.
--   No new seed required.
```

### Part A — `mst.impact_mev_pd`

```sql
-- A1. Precision fix
ALTER TABLE mst.impact_mev_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8);

-- A2. Add missing audit cols
ALTER TABLE mst.impact_mev_pd
    ADD COLUMN IF NOT EXISTS created_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version        BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id          TEXT   NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status    VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID REFERENCES sys.workflow_instance(id);

-- A3. workflow_status CHECK constraint
ALTER TABLE mst.impact_mev_pd
    DROP CONSTRAINT IF EXISTS chk_impact_mev_pd_workflow_status;
ALTER TABLE mst.impact_mev_pd
    ADD CONSTRAINT chk_impact_mev_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- A4. Backfill
UPDATE mst.impact_mev_pd SET workflow_status = 'APPROVED'
    WHERE approver_id IS NOT NULL AND workflow_status = 'DRAFT';
UPDATE mst.impact_mev_pd SET created_by = maker_id WHERE created_by IS NULL;

-- A5. Indexes
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_tenant_created
    ON mst.impact_mev_pd(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_workflow_status
    ON mst.impact_mev_pd(workflow_status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_impact_mev_pd_active
    ON mst.impact_mev_pd(periode_id, skenario)
    WHERE deleted_at IS NULL AND workflow_status = 'APPROVED';
```

### Part B — `mst.impact_pd`

```sql
-- B1. Precision fix
ALTER TABLE mst.impact_pd
    ALTER COLUMN impact_multiplier TYPE NUMERIC(10,8);

-- B2. Add missing audit cols
ALTER TABLE mst.impact_pd
    ADD COLUMN IF NOT EXISTS created_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS updated_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS deleted_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_by         UUID REFERENCES sec.user(id),
    ADD COLUMN IF NOT EXISTS row_version        BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS tenant_id          TEXT   NOT NULL DEFAULT 'TUGURE',
    ADD COLUMN IF NOT EXISTS workflow_status    VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    ADD COLUMN IF NOT EXISTS workflow_instance_id UUID REFERENCES sys.workflow_instance(id);

-- B3. workflow_status CHECK constraint
ALTER TABLE mst.impact_pd
    DROP CONSTRAINT IF EXISTS chk_impact_pd_workflow_status;
ALTER TABLE mst.impact_pd
    ADD CONSTRAINT chk_impact_pd_workflow_status
        CHECK (workflow_status IN (
            'DRAFT','PENDING_REVIEW','PENDING_APPROVAL',
            'PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'
        ));

-- B4. Backfill
UPDATE mst.impact_pd SET workflow_status = 'APPROVED'
    WHERE approver_id IS NOT NULL AND workflow_status = 'DRAFT';
UPDATE mst.impact_pd SET created_by = maker_id WHERE created_by IS NULL;

-- B5. Indexes
CREATE INDEX IF NOT EXISTS idx_impact_pd_tenant_created
    ON mst.impact_pd(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_impact_pd_workflow_status
    ON mst.impact_pd(workflow_status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_impact_pd_active
    ON mst.impact_pd(periode_id)
    WHERE deleted_at IS NULL AND workflow_status = 'APPROVED';
```

### down.sql

Reversible:
- Undo `workflow_instance_id`, `workflow_status`, audit cols (`DROP COLUMN IF EXISTS` per kolom yang ditambah)
- Revert precision: `ALTER COLUMN impact_multiplier TYPE NUMERIC(8,4)` di kedua tabel
- DROP indexes yang ditambah
- DROP CHECK constraints yang ditambah
- **Tidak** DELETE data yang di-backfill (tidak merusak data, hanya schema reversal)

---

## 6. Backend Module Design

### Package layout

Dua tabel terpisah semantically tapi pola implementasinya identik → **dua sub-package terpisah** (tidak digabung ke `flmultiplier/`) untuk konsistensi dengan pola modul sebelumnya dan agar `cmd/api/main.go` wiring tetap granular.

```
backend/internal/master/impactmevpd/
├── domain.go           — entity, request/response, allowed cols, error codes
├── repo.go             — Repository interface + sqlx impl (List, GetByID, Create, Update,
│                         SoftDelete, UpdateWorkflowStatusTx, GetActive, GetByPeriodeAndSkenario)
├── service.go          — Business logic: Create, Update, Delete, List, GetByID, GetActive,
│                         SyncWorkflowStatus, ValidatePeriodeUnique
├── handler.go          — HTTP handler: List, GetByID, Create, Update, Delete, Export,
│                         GetActive, Submit, Review, Approve, Approve2, Reject, GetWorkflow
├── routes.go           — RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
└── workflow_hook.go    — WorkflowHook implements workflow.EntityHook (BeforeCommit)
```

```
backend/internal/master/impactpd/
├── domain.go           — entity, request/response, allowed cols, error codes
├── repo.go             — Repository interface + sqlx impl (List, GetByID, Create, Update,
│                         SoftDelete, UpdateWorkflowStatusTx, GetActive)
├── service.go          — Business logic: Create, Update, Delete, List, GetByID, GetActive,
│                         SyncWorkflowStatus, ValidatePeriodeUnique
├── handler.go          — HTTP handler: List, GetByID, Create, Update, Delete, Export,
│                         GetActive, Submit, Review, Approve, Approve2, Reject, GetWorkflow
├── routes.go           — RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
└── workflow_hook.go    — WorkflowHook implements workflow.EntityHook (BeforeCommit)
```

Test files per package:
```
*_test.go: handler_test.go, workflow_hook_test.go, testutil_test.go
```

### API Endpoints

#### `mst.impact_mev_pd`

```
GET    /master/impact-mev-pd                    → List (ecl_parameter.read)
POST   /master/impact-mev-pd                    → Create (ecl_parameter.create)
GET    /master/impact-mev-pd/export             → Export CSV/XLSX (ecl_parameter.read) — BEFORE /:id
GET    /master/impact-mev-pd/active             → GetActive ?periode_id=UUID (ecl_parameter.read)
GET    /master/impact-mev-pd/:id                → GetByID (ecl_parameter.read)
PUT    /master/impact-mev-pd/:id                → Update (ecl_parameter.update)
DELETE /master/impact-mev-pd/:id                → SoftDelete DRAFT/REJECTED only (ecl_parameter.delete)
GET    /master/impact-mev-pd/:id/history        → Audit history (ecl_parameter.read)
GET    /master/impact-mev-pd/:id/workflow       → Workflow status (ecl_parameter.read)
POST   /master/impact-mev-pd/:id/submit         → Submit (ecl_parameter.submit)
POST   /master/impact-mev-pd/:id/review         → Review (ecl_parameter.review)
POST   /master/impact-mev-pd/:id/approve        → Approve step-up MFA (ecl_parameter.approve)
POST   /master/impact-mev-pd/:id/approve2       → Approve2 step-up MFA (ecl_parameter.approve)
POST   /master/impact-mev-pd/:id/reject         → Reject (ecl_parameter.reject)
```

#### `mst.impact_pd`

```
GET    /master/impact-pd                        → List (ecl_parameter.read)
POST   /master/impact-pd                        → Create (ecl_parameter.create)
GET    /master/impact-pd/export                 → Export CSV/XLSX (ecl_parameter.read) — BEFORE /:id
GET    /master/impact-pd/active                 → GetActive ?periode_id=UUID (ecl_parameter.read)
GET    /master/impact-pd/:id                    → GetByID (ecl_parameter.read)
PUT    /master/impact-pd/:id                    → Update (ecl_parameter.update)
DELETE /master/impact-pd/:id                    → SoftDelete DRAFT/REJECTED only (ecl_parameter.delete)
GET    /master/impact-pd/:id/history            → Audit history (ecl_parameter.read)
GET    /master/impact-pd/:id/workflow           → Workflow status (ecl_parameter.read)
POST   /master/impact-pd/:id/submit             → Submit (ecl_parameter.submit)
POST   /master/impact-pd/:id/review             → Review (ecl_parameter.review)
POST   /master/impact-pd/:id/approve            → Approve step-up MFA (ecl_parameter.approve)
POST   /master/impact-pd/:id/approve2           → Approve2 step-up MFA (ecl_parameter.approve)
POST   /master/impact-pd/:id/reject             → Reject (ecl_parameter.reject)
```

**Catatan permissions**: kedua entitas menggunakan `ecl_parameter.*` — sesuai WORKFLOW_CONFIG yang sudah di-seed di 0008.

### 6-eyes config (sudah ada di 0008)

`WORKFLOW_CONFIG_IMPACT_MEV_PD` dan `WORKFLOW_CONFIG_IMPACT_PD`:
- `eyes: 6`
- `stepUpRequired: { approve: true, approve2: true }`
- `sodRules: { reviewerNotMaker: true, approverNotMakerOrReviewer: true, approver2NotAnyPrevious: true }`
- Semua permissions: `ecl_parameter.*`

Approver1 = ROLE-RISK (validasi teknis nilai MEV/FL), Approver2 = ROLE-ALCO (policy override per DEC-010).

### DI wiring di `cmd/api/main.go`

Ikuti pola bobotskenario/lpscoverage:
```go
// Impact MEV PD
impactMevPdRepo    := impactmevpd.NewDBRepository(db)
impactMevPdSvc     := impactmevpd.NewService(impactMevPdRepo, auditWriter, logger)
impactMevPdHook    := impactmevpd.NewWorkflowHook(impactMevPdSvc, impactMevPdRepo)
impactMevPdHandler := impactmevpd.NewHandler(impactMevPdSvc, wfHandler)
wfEngine.RegisterHook("IMPACT_MEV_PD", impactMevPdHook)
impactmevpd.RegisterRoutes(v1, impactMevPdHandler)

// Impact PD
impactPdRepo    := impactpd.NewDBRepository(db)
impactPdSvc     := impactpd.NewService(impactPdRepo, auditWriter, logger)
impactPdHook    := impactpd.NewWorkflowHook(impactPdSvc, impactPdRepo)
impactPdHandler := impactpd.NewHandler(impactPdSvc, wfHandler)
wfEngine.RegisterHook("IMPACT_PD", impactPdHook)
impactpd.RegisterRoutes(v1, impactPdHandler)
```

### Error codes spesifik

- `FL_PERIODE_DUPLICATE` (422): `(periode_id)` atau `(periode_id, skenario)` sudah ada dengan workflow_status ≠ REJECTED/RETURNED
- `FL_APPROVED_NO_EDIT` (403): record sudah APPROVED, edit tidak diizinkan — alias `CodeMasterApprovedNoEdit`
- `FL_MULTIPLIER_OUT_OF_RANGE` (422): untuk impact_pd, nilai di luar `[0.5, 2.0]`

### Permission set (kedua entitas)

```
ecl_parameter.create   — ROLE-RISK (maker input)
ecl_parameter.read     — ROLE-RISK, ROLE-AKUN-CTL, ROLE-AUDIT, ROLE-ALCO, ROLE-AKUN
ecl_parameter.update   — ROLE-RISK (DRAFT/RETURNED only)
ecl_parameter.delete   — ROLE-RISK (soft-delete DRAFT/REJECTED only)
ecl_parameter.submit   — ROLE-RISK
ecl_parameter.review   — ROLE-AKUN-CTL
ecl_parameter.approve  — ROLE-RISK (Approver1) + ROLE-ALCO (Approver2)
ecl_parameter.reject   — ROLE-AKUN-CTL, ROLE-RISK, ROLE-ALCO (sesuai step aktif)
ecl_parameter.export   — ROLE-RISK, ROLE-AUDIT, ROLE-AKUN-CTL
```

---

## 7. Frontend Screens (delegate ke frontend-engineer-nextjs)

**Page structure**:
```
frontend/src/app/master/
├── impact-mev-pd/
│   ├── page.tsx              — List releases (DataTable §1)
│   ├── new/
│   │   └── page.tsx          — Create form
│   └── [id]/
│       ├── page.tsx          — Detail + multiplier display
│       ├── edit/
│       │   └── page.tsx      — Edit form (DRAFT/RETURNED only)
│       └── history/
│           └── page.tsx      — Workflow trail + audit history
└── impact-pd/
    ├── page.tsx              — List (DataTable §1)
    ├── new/
    │   └── page.tsx          — Create form
    └── [id]/
        ├── page.tsx          — Detail + multiplier chart
        ├── edit/
        │   └── page.tsx      — Edit form (DRAFT/RETURNED only)
        └── history/
            └── page.tsx      — Workflow trail + audit history
```

**Komponen kunci (reuse dari sebelumnya)**:
- `components/blips/DataTable` — list dengan sort/filter/cursor/export
- `components/blips/WorkflowStatusBadge` — reuse
- `components/blips/MakerReviewerApproverPanel` — 6-eyes panel (reuse + approve2)

**Komponen baru**:
- `components/blips/MultiplierChart` — visualisasi nilai multiplier per skenario (bar chart Recharts): sumbu x = skenario (GOOD/BAD), sumbu y = impact_multiplier. Color-coded: GOOD = hijau, BAD = merah. Untuk `impact_pd` tampilkan single value + range [0.5, 2.0] sebagai reference line.
- `components/blips/FLMultiplierSummaryCard` — card summary: menampilkan MEV component GOOD/BAD dan final FL PD multiplier untuk periode yang dipilih. Dipakai di halaman detail dan dashboard ECL Phase 4.

**Schemas**: `frontend/src/lib/schemas/`
- `impact-mev-pd.schema.ts` — Zod schema untuk create/update form
- `impact-pd.schema.ts` — Zod schema untuk create/update form

**API client**: `frontend/src/lib/api/`
- `impact-mev-pd.api.ts` — CRUD + workflow + active lookup
- `impact-pd.api.ts` — CRUD + workflow + active lookup

**UX rules wajib**:
- List = sort + cursor paging + filter + export (UX §1)
- Form = toast sukses/gagal spesifik (UX §2): mis. "FL PD Multiplier untuk Periode Jun-2026 berhasil dibuat. Menunggu review."
- Step-up MFA dialogs untuk approve + approve2 (reuse pola pd_pefindo)
- Multiplier display: nilai numerik 8 decimal places + chart visual

---

## 8. QA (delegate ke qa-engineer)

**UAT file**: `docs/uat/master-data/APP-A-MSTR-008-impact-fl-multiplier-uat-001.md`

**Integration tests**:
- `backend/internal/test/integration/impactmevpd_test.go`
- `backend/internal/test/integration/impactpd_test.go`

### Test Cases

| TC | Entitas | Deskripsi | Expected |
|---|---|---|---|
| TC-001 | impact_mev_pd | Create GOOD + BAD untuk periode baru → DRAFT | 201, `workflow_status=DRAFT`, presisi 8dp di response |
| TC-002 | impact_mev_pd | Create duplicate `(periode_id, skenario)` yang sudah APPROVED | 422 `FL_PERIODE_DUPLICATE` |
| TC-003 | impact_mev_pd | 6-eyes cycle lengkap: Maker→Reviewer→Approver1(RISK+MFA)→Approver2(ALCO+MFA) | `workflow_status=APPROVED`, 4 distinct users, 2 step-up MFA sigs |
| TC-004 | impact_mev_pd | SoD violation — Maker mencoba jadi Reviewer via API | `SOD_VIOLATION` 403 |
| TC-005 | impact_mev_pd | SoD violation — Approver1 = Maker atau Reviewer | `SOD_VIOLATION` 403 |
| TC-006 | impact_mev_pd | SoD violation — Approver2 = siapapun sebelumnya | `SOD_VIOLATION` 403 |
| TC-007 | impact_mev_pd | Approve tanpa step-up MFA | `MFA_STEP_UP_REQUIRED` 401/403 |
| TC-008 | impact_pd | Create untuk periode baru → DRAFT, multiplier presisi 8dp | 201, `workflow_status=DRAFT` |
| TC-009 | impact_pd | Create dengan multiplier di luar [0.5, 2.0] | 422 `FL_MULTIPLIER_OUT_OF_RANGE` |
| TC-010 | impact_pd | 6-eyes cycle lengkap + single-active invariant check | `workflow_status=APPROVED`, dua APPROVED rows beda periode tidak conflict |
| TC-011 | impact_pd | Single-active conflict: dua APPROVED rows dengan `periode_id` sama → reject pada approve2 | 422 `FL_PERIODE_DUPLICATE` |
| TC-012 | kedua | Idempotency: POST dengan Idempotency-Key sama dua kali | Kedua return 201, hanya satu INSERT |
| TC-013 | kedua | Export CSV list dengan filter aktif | File CSV respects filter, audit `*.EXPORT` ditulis |
| TC-014 | kedua | `GET /active?periode_id=...` → return APPROVED row + compatible dengan ECL engine Phase 4 placeholder | Response shape sesuai kontrak |

**4 User test wajib (SoD)**:
```
risk.officer.1   → ROLE-RISK      (Maker + Approver1 — dua user berbeda)
risk.officer.2   → ROLE-RISK      (Approver1 — berbeda dengan maker)
akun.ctl.1       → ROLE-AKUN-CTL  (Reviewer)
alco.1           → ROLE-ALCO      (Approver2)
```

---

## 9. Compliance Gate (BLOCKING — ifrs9-compliance-reviewer)

Wajib selesai sebelum merge ke `develop`.

**Checklist compliance reviewer**:

1. **6-eyes workflow konfigurasi benar**: `WORKFLOW_CONFIG_IMPACT_MEV_PD` + `WORKFLOW_CONFIG_IMPACT_PD` dari 0008 memiliki `eyes=6`, `stepUpRequired.approve=true`, `stepUpRequired.approve2=true`, `approver2NotAnyPrevious=true`. Verifikasi config masih di-load dengan benar setelah migration 0015 (tidak ada perubahan di 0015 yang bisa merusak ON CONFLICT DO NOTHING behavior di 0008).

2. **FL Multiplier precision `NUMERIC(10,8)`**: verifikasi di migration DDL dan di Go layer (`shopspring/decimal` — bukan float64). Test TC-001 dan TC-008 memeriksa presisi 8dp setelah round-trip DB.

3. **Formula ECL FL sesuai DEC-010**: verifikasi bahwa `impact_pd.impact_multiplier` diapply sebagai `ECL_FL_skenario = ECL_skenario × impact_multiplier` (bukan sebagai additive offset). Getter `GetActive` harus mengembalikan nilai yang ECL engine Phase 4 dapat langsung multiply. Cek TC-014 response shape.

4. **Dual FL semantic**: verifikasi bahwa `impact_mev_pd` (MEV component GOOD/BAD) dan `impact_pd` (final FL PD multiplier per periode) memiliki semantik yang tidak tumpang tindih — sesuai SoW §4 formula. Jika tumpang tindih ditemukan, flag `[NEEDS-HUMAN]` untuk ALCO clarification sebelum ECL engine Phase 4.

5. **Single-active invariant**: verifikasi service `ValidatePeriodeUnique` + test TC-011 untuk `impact_pd` dan TC-002 untuk `impact_mev_pd`. Dua APPROVED rows dengan `periode_id` + `skenario` sama = invalid.

6. **No hard delete** di kedua tabel — soft delete only. Verifikasi service `SoftDelete` hanya set `deleted_at`.

7. **Range check `impact_pd.impact_multiplier [0.5, 2.0]`**: verifikasi CHECK di migration DDL AND di service layer validation (TC-009). Range berdasarkan 0001 schema — reviewer konfirmasi ini sesuai regulasi ALCO.

8. **Backfill di migration 0015**: verifikasi backfill `workflow_status = 'APPROVED'` untuk rows yang sudah punya `approver_id IS NOT NULL` tidak mengakibatkan regresi data yang sebelumnya DRAFT tanpa approver → dicek aman karena UPDATE hanya untuk `approver_id IS NOT NULL`.

**VERDICT format**: `PASS` / `CONDITIONAL-PASS (issue: <keterangan>)` / `BLOCK (<alasan>)`.

---

## 10. Open Questions — RESOLVED 2026-06-09

### OQ-1 — Skenario enum `impact_mev_pd`: GOOD + BAD saja? ✅ RESOLVED

**Decision**: GOOD + BAD saja. NORMAL = baseline implicit 1.0. Tidak ada row di `mst.impact_mev_pd` untuk skenario NORMAL.

**Implication**: CHECK constraint `('GOOD','BAD')` di 0001 **tidak diubah**. Migration 0015 tidak perlu update CHECK. Service layer reject `skenario = 'NORMAL'` dengan `VALIDATION_FAILED` 400.

**Action**: Lanjutkan implementation sesuai schema 0001 — dua rows per periode untuk impact_mev_pd (GOOD dan BAD). ECL engine Phase 4 lookup: GOOD = `impact_mev_pd[skenario=GOOD].impact_multiplier`, NORMAL = 1.0 (hardcode), BAD = `impact_mev_pd[skenario=BAD].impact_multiplier`.

---

### OQ-2 — Relasi `impact_mev_pd` dan `impact_pd`: Independent atau Derived? ✅ RESOLVED

**Decision**: **Option A — Independent**. Keduanya adalah input manual oleh ALCO. `impact_pd` adalah final FL PD multiplier yang di-set ALCO secara independen tanpa compute logic dari `impact_mev_pd`. `impact_mev_pd` adalah audit trail analisis MEV, `impact_pd` adalah keputusan final per periode.

**Implication**: Tidak ada backend compute logic untuk derive `impact_pd` dari `impact_mev_pd`. Dua workflow 6-eyes terpisah. Dua set endpoint terpisah. ECL engine Phase 4 hanya consume `impact_pd` (final FL multiplier) untuk formula `ECL_FL_skenario = ECL_skenario × impact_pd.impact_multiplier`.

**Action**: Implementasi keduanya sebagai modul CRUD + workflow independen. Tidak ada FK atau dependency antar tabel di service layer.

---

### OQ-3 — Range constraint `impact_mev_pd.impact_multiplier` ✅ RESOLVED

**Decision**: **Hanya `> 0`** — tidak ada upper bound. Service layer enforce `impact_multiplier > 0` saja. `impact_pd.impact_multiplier` tetap `BETWEEN 0.5 AND 2.0` (existing CHECK di 0001 tidak diubah).

**Implication**: Migration 0015 menambahkan CHECK `impact_multiplier > 0` di `mst.impact_mev_pd`. Error code `FL_MULTIPLIER_OUT_OF_RANGE` (422) hanya berlaku untuk `impact_pd` (out of [0.5, 2.0]). Untuk `impact_mev_pd`, violation = value ≤ 0 → `VALIDATION_FAILED` 400.

**Action**: DDL di 0015 tambahkan `ADD CONSTRAINT chk_impact_mev_pd_multiplier_positive CHECK (impact_multiplier > 0)`. Service layer `impactmevpd` validate `> 0`. Service layer `impactpd` validate `BETWEEN 0.5 AND 2.0`.

---

### OQ-4 — Versioning: revise via soft-delete + create baru ✅ RESOLVED

**Decision**: Revisi FL multiplier yang sudah APPROVED dilakukan via **soft-delete record lama + create record baru** (pola identik dengan `bobot_skenario`, `lps_coverage`). Tidak ada supersede version column atau history chain FK.

**Implication**: `SoftDelete` service hanya memperbolehkan soft-delete jika `workflow_status IN ('DRAFT','REJECTED','RETURNED')`. Record APPROVED tidak bisa di-soft-delete langsung — harus lewat workflow (reject → RETURNED → soft-delete atau create new yang akan menjadi APPROVED baru via single-active invariant enforcement).

**Action**: Implementasi `SoftDelete` dengan guard `workflow_status` check. Error code `FL_APPROVED_NO_EDIT` (403) saat attempt delete/update pada record APPROVED.

---

### OQ-5 — ECL engine Phase 4 contract: dua endpoint terpisah ✅ RESOLVED

**Decision**: **Dua endpoint terpisah** — `GET /master/impact-mev-pd/active?periode_id=UUID` dan `GET /master/impact-pd/active?periode_id=UUID`. Tidak ada endpoint gabungan Phase 3.

**Implication**: ECL engine Phase 4 akan call keduanya saat init per calc run (dua HTTP request). Response shape didokumentasikan di plan §6 dan di-test via TC-014. Phase 4 engineer dapat request endpoint gabungan di Phase 4 jika N+1 menjadi masalah performa — noted sebagai Phase 4 backlog.

**Action**: Implementasi dua `GetActive` method di repo masing-masing. Document response shape di handler comments + OpenAPI spec.

---

## 11. Security Gate (advisory)

Modul ini tidak menyentuh PII kolom terenkripsi atau `aud.*` secara langsung. Security gate **advisory**.

Checklist:
1. Tenant isolation: `WHERE tenant_id = $1` di semua query.
2. Idempotency-Key check di semua POST endpoints.
3. Input validation: `impact_multiplier` range enforced service-side + DB constraint.
4. Audit trail: `IMPACT_MEV_PD.CREATE/UPDATE/DELETE`, `IMPACT_PD.CREATE/UPDATE/DELETE`, `*.EXPORT` semua di `aud.audit_log` di-transaksi yang sama.
5. Step-up MFA token tidak di-log.

---

## 12. Handoff Order (DAG)

```
[Prasyarat: feature/phase-3-pd-pefindo merge ke develop]
            |
            v
     data-modeler
     migration 0015 (impact_mev_pd + impact_pd schema-fix)
            |
            +---------------------------+
            v                           v
  backend-engineer-go          uiux-designer
  (impactmevpd + impactpd       (screen desain: list,
  CRUD + workflow hook +         create form, detail chart,
  GetActive endpoint)            workflow panel)
            |                           |
            +---------------------------+
                        |
                        v
              frontend-engineer-nextjs
              (pages + MultiplierChart + FLMultiplierSummaryCard)
                        |
                        v
                   qa-engineer
                   (UAT + integration tests TC-001..TC-014)
                        |
                        v
              ifrs9-compliance-reviewer
              (BLOCKING gate)
                        |
                        v
              security-engineer
              (advisory review)
                        |
                        v
                  merge → develop
                  PR: feature/phase-3-fl-multiplier → develop
```

**Paralel yang valid**:
- `data-modeler` tidak perlu menunggu siapapun.
- `backend-engineer-go` + `uiux-designer` paralel setelah `data-modeler` selesai.
- `impactmevpd` dan `impactpd` dapat dikerjakan oleh backend-engineer-go secara sekuensial dalam satu PR (kedua package dalam branch yang sama).
- `frontend-engineer-nextjs` mulai setelah uiux-designer selesai desain.
- `qa-engineer` dapat mulai menulis UAT doc (§8) paralel; integration test menunggu backend.
- `ifrs9-compliance-reviewer` dan `security-engineer` paralel setelah QA hijau.

**Tidak ada integration-engineer** di modul ini (tidak ada XLSX parser, tidak ada external feed). Semua input via UI form standard.

---

## 13. Open Questions (original — superseded by §10 resolutions)

### OQ-1 — Skenario enum `impact_mev_pd`: GOOD + BAD saja, atau GOOD + NORMAL + BAD?

**Konteks**: `mst.impact_mev_pd` di schema 0001 memiliki `CHECK (skenario IN ('GOOD','BAD'))` — hanya dua skenario. Tidak ada NORMAL. Ini bermakna MEV impact hanya ada untuk extreme cases; NORMAL baseline dianggap multiplier = 1.0 (no impact).

**Formulas.md**: "ECL_FL_skenario = ECL_skenario × Impact_PD_multiplier_skenario" — semua tiga skenario. Namun jika NORMAL = 1.0 by definition (no FL adjustment), maka tabel tidak perlu baris untuk NORMAL.

**Impact**: jika NORMAL perlu `impact_mev_pd` row → CHECK constraint di 0001 harus diupdate di 0015 untuk include `'NORMAL'`. Jika tidak perlu → status quo tetap (cukup GOOD + BAD).

**Default assumsi (jika tidak ada input user)**: GOOD + BAD only, NORMAL assumed 1.0 (berdasarkan schema existing 0001). Migration 0015 tidak perlu ubah CHECK constraint.

**Butuh keputusan**: apakah NORMAL memiliki MEV impact multiplier yang bukan 1.0? Ini kebijakan ALCO.

---

### OQ-2 — Relasi semantik `impact_mev_pd` dan `impact_pd`: apakah keduanya INDEPENDENT atau `impact_pd` = f(impact_mev_pd)?

**Konteks**: Dua tabel, dua entitas, dua workflow terpisah. Ada dua kemungkinan:
- **Option A (Independent)**: `impact_mev_pd` adalah audit trail analisis MEV oleh RISK; `impact_pd` adalah final FL multiplier yang di-set ALCO secara independen (tidak harus derived dari MEV computation). ALCO bisa override berdasarkan judgment.
- **Option B (Derived)**: `impact_pd` dihitung dari `impact_mev_pd` + `bobot_skenario` via formula. User input `impact_mev_pd`; sistem compute `impact_pd` secara otomatis. `impact_pd` workflow adalah approval atas nilai computed.

**Impact desain**: Option B butuh backend compute logic (formula aggregation); Option A adalah pure manual input. Formula di formulas.md menggunakan `Impact_PD_multiplier_skenario` langsung — tidak menyebut agregasi dari MEV.

**Default assumsi**: **Option A (Independent)** — keduanya manual input, tidak ada derive. Ini sesuai pola tabel lain (ALCO override explicit).

**Butuh keputusan user/ALCO**.

---

### OQ-3 — Range constraint `impact_mev_pd.impact_multiplier`: berapa batas valid?

**Konteks**: Schema 0001 tidak memiliki CHECK constraint untuk range `impact_mev_pd.impact_multiplier` (berbeda dengan `impact_pd` yang punya `BETWEEN 0.5 AND 2.0`).

**Pilihan**:
- A. Sama dengan impact_pd: `BETWEEN 0.5 AND 2.0`
- B. Lebih lebar: `BETWEEN 0.1 AND 5.0` (MEV component bisa lebih extreme)
- C. Hanya positif: `> 0` (biarkan ALCO yang decide batas)

**Default assumsi**: `> 0` (hanya positif) untuk fleksibilitas. Service layer validasi ini.

**Butuh keputusan compliance reviewer / ALCO** untuk konfirmasi range yang sesuai kebijakan.

---

### OQ-4 — Versioning: apakah satu `(periode_id, skenario)` bisa punya multiple versions (APPROVED lama + APPROVED baru)?

**Konteks**: Pola saat ini di bobot_skenario dan lps_coverage: single-active invariant (tidak bisa ada dua APPROVED dengan periode overlap). Untuk `impact_mev_pd`, UNIQUE constraint di 0001 adalah `(periode_id, skenario)` — artinya **paling banyak satu row per (periode, skenario)**.

**Masalah**: jika ALCO ingin revise FL multiplier untuk periode yang sudah APPROVED (bukan belum selesai), apakah harus soft-delete dulu baru create baru, ataukah ada supersede pattern?

**Default assumsi**: sama dengan bobot_skenario — soft-delete record APPROVED lama dulu, buat baru. Tidak ada supersede version column. Service layer enforce ini.

**Butuh keputusan** jika audit trail versioning lebih ketat diperlukan.

---

### OQ-5 — ECL engine Phase 4 contract: format `GET /active` response untuk dual FL

**Konteks**: ECL engine Phase 4 akan consume kedua endpoint:
- `GET /master/impact-mev-pd/active?periode_id=X` → return rows untuk GOOD + BAD
- `GET /master/impact-pd/active?periode_id=X` → return single row

**Pertanyaan**: apakah Phase 4 butuh **satu endpoint gabungan** (e.g. `GET /master/fl-multiplier/active?periode_id=X` yang return keduanya sekaligus) untuk menghindari N+1 HTTP call di ECL engine?

**Default assumsi**: dua endpoint terpisah (sesuai desain per-tabel). ECL engine Phase 4 call keduanya saat init per calc run. Tidak perlu endpoint gabungan Phase 3.

**Bisa berubah** jika Phase 4 engineer butuh single-call contract. Catat sebagai Phase 4 concern.

---

## 14. Risk Matrix

| Risk | Severity | Mitigasi |
|---|---|---|
| OQ-1 unresolved → skenario NORMAL butuh MEV row di production → data gap | MEDIUM | Default GOOD+BAD; jika NORMAL butuh row, migration 0015 tambah CHECK update |
| OQ-2 unresolved → ALCO set `impact_pd` tidak konsisten dengan `impact_mev_pd` GOOD/BAD → ECL over/under-stated | HIGH | Default Option A; compliance reviewer flag `[NEEDS-HUMAN]` jika formula derived diperlukan |
| Precision regression: jika down migration NUMERIC(10,8) → NUMERIC(8,4) di production menyebabkan truncation | MEDIUM | Down migration dokumentasikan risk; tidak di-run di production tanpa DBA review |
| WORKFLOW_CONFIG sudah di-seed 0008 tapi engine tidak load karena key tidak matched | LOW | Unit test `wfEngine.RegisterHook("IMPACT_MEV_PD", ...)` + integration test TC-003 |
| Backfill di 0015 meng-APPROVE rows yang seharusnya tetap DRAFT | LOW | Backfill kondisional `WHERE approver_id IS NOT NULL`; reviewer verifikasi pre/post row counts |
| Phase 4 ECL engine tidak dapat consume `GetActive` response (shape mismatch) | MEDIUM | TC-014 explicit contract test; document response shape di plan |

---

## 15. Acceptance Criteria Sebelum Merge

Semua item berikut HARUS terpenuhi sebelum PR `feature/phase-3-fl-multiplier` → `develop`:

- [ ] CI hijau: `backend-lint` (golangci-lint v1.64.5), `backend-test` (unit), `frontend-build`, `frontend-lint`
- [ ] Integration tests TC-001 sampai TC-014 hijau
- [ ] UAT `APP-A-MSTR-008-impact-fl-multiplier-uat-001.md` di-sign-off oleh ROLE-RISK dan ROLE-ALCO
- [ ] `ifrs9-compliance-reviewer` VERDICT = `PASS` atau `CONDITIONAL-PASS` dengan follow-up ticket
- [ ] `security-engineer` review selesai (advisory); temuan HIGH = none
- [ ] Migration `000015_impact_fl_multiplier_schema_fix.up.sql` + `.down.sql` ditest idempotent (up twice, down, up again)
- [ ] `GET /master/impact-pd/active` dan `GET /master/impact-mev-pd/active` contract documented + TC-014 hijau
- [ ] Tidak ada `float64` untuk impact_multiplier di Go code (shopspring/decimal enforced — golangci-lint + review)
- [x] OQ-1..OQ-5 dijawab dan dicatat di plan doc §10 (RESOLVED 2026-06-09)
- [ ] `docs/uat/master-data/APP-A-MSTR-008-impact-fl-multiplier-uat-001.md` exists dan complete

---

## Lampiran — Commit Convention

Branch: `feature/phase-3-fl-multiplier`
Commits expected:
```
feat(db): add migration 0015 impact_mev_pd + impact_pd schema-fix (audit cols + precision + workflow_status)
feat(app-a): implement impactmevpd CRUD + 6-eyes workflow
feat(app-a): implement impactpd CRUD + 6-eyes workflow
feat(web): add impact-mev-pd list/create/detail screens with MultiplierChart
feat(web): add impact-pd list/create/detail screens with FLMultiplierSummaryCard
test(app-a): add impactmevpd + impactpd integration tests TC-001..TC-014
docs(app-a): add UAT script APP-A-MSTR-008-impact-fl-multiplier
```

PR target: `develop` (squash and merge per git-conventions.md).
