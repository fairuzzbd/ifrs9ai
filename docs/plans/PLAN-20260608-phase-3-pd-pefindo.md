# PLAN-20260608 — Phase 3 Modul 5/12: `mst.pd_pefindo` (PD Curve Pefindo)

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-08
**Branch base**: `develop` (setelah PR #15 lps_coverage merge)
**Working branch**: `feature/phase-3-pd-pefindo`
**Sumber kebenaran**:
- `docs/handoff/RESUME-phase-3.md` §3 row 5
- `BLIPS_Decision_Log_v1.0.docx` DEC-010, DEC-011, DEC-016, DEC-027
- `.claude/memory/formulas.md` §PD
- `Pefindo_Annual_Default_Study_2007-2025_EN.pdf`
- `SoW_v1.4.docx` §4 (PD parameter struktur)
- `db/migrations/000001_init_schema.up.sql` §4.4 (existing `mst.pd_pefindo`)
- `db/migrations/000012_lps_coverage_schema_fix.up.sql` (pola migration terbaru)
- `backend/internal/master/lpscoverage/` (pola modul terbaru)
**Klasifikasi**: feature — **path regulated ECL parameter** → ifrs9-compliance-reviewer BLOCKING
**Kompleksitas modul**: HIGH (upload XLSX feed + async parse job + PD matrix rating×tenor + 6-eyes dual step-up MFA)
**Target**: ~2 minggu setelah PR #15 merge

---

## 1. Goal

Implementasi end-to-end modul PD (Probability of Default) dari Pefindo Annual Default Study:

1. **Upload + parse** file XLSX Pefindo triwulanan → async job (Asynq) → extract PD per rating bucket × tenor.
2. **6-eyes ECL parameter workflow** (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED) dengan dua step-up MFA di kedua APPROVE step (DEC-027).
3. **PD versioning** — setiap upload Pefindo = satu `release_id` baru; rilis lama tetap valid sampai cutover eksplisit; single-active invariant per `tanggal_efektif`.
4. **Rating bucket × tenor matrix** — PD tersimpan sebagai baris per `(rating_code, tenor_bucket)`, presisi `NUMERIC(10,8)` per DEC-016.
5. **Kalibrasi check** — reconcile output terhadap published default rate Pefindo per bucket (tolerance `NUMERIC(10,8)`); mismatch > toleransi → flag di `sys.job.result_jsonb`.
6. **Konsumsi ECL engine** — modul ini adalah data source untuk ECL calc run Phase 4 (PD lookup per rating × tenor × stage). Phase 4 placeholder wiring direncanakan di §6.

**Exit criteria**: migration 0013 up/down idempotent, TC-001..TC-008 hijau di CI, UAT signed ROLE-RISK + ROLE-ALCO, compliance VERDICT = PASS atau CONDITIONAL-PASS dengan follow-up ticket, CI hijau (backend-lint + test + frontend-build + lint).

---

## 2. Decision Log check (sebelum eksekusi)

| DEC | Implikasi modul ini |
|---|---|
| DEC-010 | ECL param 3-stage × 3-skenario × dual FL — PD adalah input utama; presisi dan kalibrasi kritikal |
| DEC-011 | SICR trigger: rating turun ≥ 2 notch OR IG→non-IG. PD notch mapping harus konsisten dengan `mst.rating_history_counterparty` |
| DEC-016 | PD/LGD/EIR storage `NUMERIC(10,8)`; no float64; shopspring/decimal di Go |
| DEC-021 | Idempotency-Key wajib di semua mutating endpoints |
| DEC-022 | Cursor pagination (no offset) |
| DEC-027 | Step-up MFA untuk approve (ALCO Approver1) dan approve2 (ALCO Approver2). Kedua step = distinct user |
| DEC-017 | 6-eyes workflow, SoD: maker ≠ reviewer ≠ approver ≠ approver2 |
| UX §3 | Parse XLSX > 2 detik → long-running job pattern (202+jobId+SSE) |

**Locked decisions** yang TIDAK boleh diubah di modul ini:
- Toleransi ECL precision `NUMERIC(10,8)` — refuse downgrade ke `NUMERIC(8,4)` (ini sudah typo di `mst.pd_pefindo` 0001, harus di-fix di 0013).
- Step-up MFA di kedua approve step — DEC-027 berlaku.
- `WORKFLOW_CONFIG_PD_PEFINDO` harus diselesaikan di migration 0013 (bukan di 0008 karena entitas baru berbeda dari yang sudah di-seed).

---

## 3. Prasyarat

- **PR #15 (`feature/phase-3-lps-coverage`) sudah merge ke `develop`** sebelum branch `feature/phase-3-pd-pefindo` dibuat.
- Workflow engine (`sys.workflow_instance`, `sys.workflow_signature`, `sys.config WORKFLOW_CONFIG_*`) sudah ada dari migrations 0007 + 0008.
- Asynq worker infrastructure sudah ada dari Phase 2 (`internal/worker/`, `sys.job` tabel dari migration 0004).
- MinIO tersedia (Phase 2). Bucket `feeds/pefindo/` belum ada — integration-engineer buat.
- `mst.pd_pefindo` sudah ada di 0001 tapi **schema tidak memadai** (missing audit cols, precision salah, tidak ada PD matrix per tenor bucket, tidak ada release versioning). Data-modeler harus **refactor via migration 0013** — bukan hanya backfill.

---

## 4. Analisis schema existing (`mst.pd_pefindo` dari 0001)

Schema 0001 memiliki kolom:
- `id`, `rating`, `pd_12month NUMERIC(8,4)`, `pd_lifetime_{3,5,7,10}y NUMERIC(8,4)` (wide format — satu baris per rating)
- `sumber`, `tanggal_publikasi`, `periode_berlaku_{dari,sampai}`, `dokumen_pendukung_id`
- `uploaded_by`, `uploaded_at`, `approved_by`, `approved_at`
- Missing: `created_by`, `updated_at/by`, `deleted_at/by`, `row_version`, `tenant_id`, `workflow_status`, `workflow_instance_id`
- Precision salah: `NUMERIC(8,4)` — harus `NUMERIC(10,8)` per DEC-016

**Masalah desain wide format**: tenor disimpan sebagai kolom (pd_12month, pd_lifetime_3y, ...) — tidak extensible, tenor baru butuh DDL change. Migration 0013 harus memperkenalkan skema baru yang proper.

**Keputusan desain untuk data-modeler**: Perkenalkan dua tabel baru:
1. `mst.pd_pefindo_release` — metadata per rilis (upload, workflow, file MinIO ref, tanggal_efektif)
2. `mst.pd_pefindo_curve` — baris per `(release_id, rating_code, tenor_bucket)` dengan `pd_value NUMERIC(10,8)`

Tabel `mst.pd_pefindo` lama di-deprecate (tidak di-drop, data lama dipertahankan) dan tidak dipakai oleh service layer baru. Data-modeler dokumentasikan deprecation di migration comment.

---

## §1. Story Breakdown

### APP-A-MSTR-007-01 — Upload File Pefindo XLSX (Maker)

**Actor**: ROLE-RISK (Maker)
**Story**: Sebagai Risk Officer, saya ingin mengupload file XLSX Pefindo Annual Default Study agar sistem dapat mem-parse PD curve terbaru untuk kalibrasi ECL.
**AC (singkat)**:
- GIVEN: Risk Officer login, GET `/master/pd-pefindo/new` → form upload
- WHEN: upload file XLSX valid (≤ 50MB), submit dengan Idempotency-Key
- THEN: `202 Accepted` + `{ jobId, statusUrl, streamUrl }` — job async dimulai
- AND: `JobProgressPanel` muncul dengan SSE progress update
- AND: setelah job selesai, `sys.job` status = `completed`, `mst.pd_pefindo_release` dibuat dengan `workflow_status = DRAFT`
- AND: file tersimpan di MinIO `feeds/pefindo/{yyyy}/{mm}/{file_original_name}`
- AND: audit event `PEFINDO.UPLOAD` ditulis ke `aud.audit_log` di transaksi yang sama dengan insert release

**Validasi upload**:
- MIME: hanya `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` (xlsx)
- Size: ≤ 50MB
- File TIDAK dieksekusi/preview formula (antivirus placeholder di Phase 3, aktifkan ClamAV Phase 5)
- Nama file di-sanitize (no path traversal)
- Duplikasi (Idempotency-Key) → `IDEMPOTENCY_REPLAY` 200

---

### APP-A-MSTR-007-02 — Parse + Kalibrasi (Background Job)

**Actor**: System (Asynq worker)
**Story**: Sebagai sistem, setelah file XLSX diupload, saya harus mem-parse sheet Pefindo, extract PD per rating bucket × tenor, dan reconcile terhadap published default rate.
**AC (singkat)**:
- Worker receives task `pefindo.parse_release` dengan `releaseId` dan `fileKey` (MinIO path)
- Worker download file dari MinIO, buka dengan `excelize`
- Header validation: cek kolom wajib (rating_code, default_rate_1y, default_rate_3y, dst.)
- Untuk setiap baris: parse `rating_code` → strip prefix `id` jika ada (OQ-1 RESOLVED) → validate against internal enum, parse PD values
- Insert rows ke `mst.pd_pefindo_curve` dengan `NUMERIC(10,8)` precision
- Kalibrasi check: bandingkan PD per bucket dengan reference Pefindo published rate (dari PDF §4 table). Jika delta > 1e-8 → flag `calibration_warning` di `sys.job.result_jsonb` (tidak block, tapi harus dilaporkan ke reviewer)
- Update `sys.job` status bertahap: `progress 0→100`, `currentStep` deskriptif
- SSE stream: emit `event: progress` setiap 1% atau setiap 10 rating rows
- On success: `sys.job.status = completed`, `result_jsonb` berisi `{ releaseId, rowCount, ratingBuckets, tenorBuckets, calibrationWarnings: [] }`
- On failure: `sys.job.status = failed`, `error_jsonb` berisi detail. Retry max 3 via Asynq RetryMax. Setelah max retry → DLQ.

---

### APP-A-MSTR-007-03 — Review Hasil Parsing (Reviewer)

**Actor**: ROLE-AKUN-CTL (Reviewer)
**Story**: Sebagai Finance Controller, saya ingin mereview hasil parsing Pefindo sebelum dikirim ke RISK untuk approval, agar memastikan PD matrix lengkap dan tidak ada anomali.
**AC (singkat)**:
- Reviewer GET `/master/pd-pefindo/{id}` → tampil matrix heatmap rating × tenor dengan PD values
- Jika ada `calibration_warnings` dari job → tampil banner WARNING kuning di detail page
- Reviewer POST `/{id}/review` → `workflow_status` = `PENDING_APPROVAL`
- SoD: reviewer ≠ maker. Jika sama → `SOD_VIOLATION` 403
- Reject dengan komentar min 10 karakter → `workflow_status` = `REJECTED`/`RETURNED`

---

### APP-A-MSTR-007-04 — Approve PD Calibration — RISK (Approver1)

**Actor**: ROLE-RISK (Approver1)
**Story**: Sebagai Risk Officer, saya ingin men-approve kalibrasi PD sebagai approval pertama (6-eyes step ke-3) dengan step-up MFA, untuk memvalidasi bahwa PD curve sesuai PSAK 71 / Pefindo study.
**AC (singkat)**:
- ROLE-RISK POST `/{id}/approve` dengan `signatureMethod = JWT_STEP_UP`
- Backend verify `mfa_verified = true` dan `stepup_token` valid (fresh, < 5 menit)
- SoD: Approver1 ≠ Maker ≠ Reviewer
- On success: `workflow_status` = `PENDING_APPROVAL_2`
- `sys.workflow_signature` row ditulis: `signed_at`, `signature_hash`, `signature_method = JWT_STEP_UP`

---

### APP-A-MSTR-007-05 — Approve Final Activation — ALCO (Approver2)

**Actor**: ROLE-ALCO (Approver2)
**Story**: Sebagai ALCO member, saya ingin memberikan approval final (6-eyes step ke-4) untuk mengaktifkan PD curve baru sebagai parameter ECL aktif, dengan step-up MFA.
**AC (singkat)**:
- ROLE-ALCO POST `/{id}/approve2` dengan `signatureMethod = JWT_STEP_UP`
- Backend verify step-up MFA fresh
- SoD: Approver2 ≠ Maker ≠ Reviewer ≠ Approver1 (`approver2NotAnyPrevious = true`)
- On success: `workflow_status` = `APPROVED`
- Single-active invariant check: jika tanggal_efektif overlap dengan release APPROVED lain → `422 PD_PERIOD_OVERLAP`
- Audit event `PD_PEFINDO.APPROVE_FINAL` ditulis
- Notifikasi in-app ke Maker: "PD Curve release {id} telah diaktifkan oleh ALCO."

---

### APP-A-MSTR-007-06 — Baca PD Curve Aktif (Any Authorized)

**Actor**: ROLE-RISK, ROLE-AKUN, ROLE-AUDIT, dst.
**Story**: Sebagai pengguna authorized, saya ingin membaca PD curve aktif per tanggal tertentu, untuk referensi dan verifikasi ECL calculation.
**AC (singkat)**:
- GET `/master/pd-pefindo/active?tanggal=2026-06-01` → return release APPROVED yang berlaku pada tanggal tersebut + full matrix rating × tenor
- Permission: `pd_pefindo.read`
- Response include `calibration_warnings` dari `sys.job.result_jsonb` (advisory, bukan blocker)
- ROLE-AUDIT mendapat `include_deleted=true` support

---

### APP-A-MSTR-007-07 — List PD Release History

**Actor**: ROLE-RISK, ROLE-AKUN-CTL, ROLE-AUDIT
**Story**: Sebagai pengguna, saya ingin melihat seluruh riwayat rilis PD Pefindo (termasuk yang sudah expired/superseded), untuk audit trail.
**AC (singkat)**:
- GET `/master/pd-pefindo` → DataTable (UX §1): sort + cursor paging + filter + export CSV/XLSX
- Filter: `workflow_status`, `tanggal_efektif`, `sumber_file`
- Export: CSV/XLSX (max 10k row inline; > 10k → async export ke MinIO)
- Audit event `PD_PEFINDO.EXPORT` per export

---

## §2. DB Schema (delegate ke data-modeler)

### Migration: `000013_pd_pefindo_schema.up.sql` + `.down.sql`
**Requires**: 0001, 0007, 0008, 0012

**Tag migration**:
```
-- migration: 0013 pd_pefindo_schema
-- author: data-modeler
-- requires: 0001, 0007, 0008, 0012
-- description: Introduce mst.pd_pefindo_release + mst.pd_pefindo_curve
--              to replace the flat-column mst.pd_pefindo design.
--              Fix precision NUMERIC(8,4) → NUMERIC(10,8) per DEC-016.
--              Deprecate (not drop) mst.pd_pefindo — legacy rows kept for
--              audit continuity; service layer baru tidak menulis ke tabel lama.
--              Seed WORKFLOW_CONFIG_PD_PEFINDO in sys.config.
```

### Tabel 1: `mst.pd_pefindo_release`

Metadata per rilis (satu baris per upload Pefindo).

Kolom wajib:
- `id UUID PK DEFAULT uuidv7()`
- `kode_rilis TEXT NOT NULL` — e.g. `PEFINDO-2025-Q4`, unique per tenant
- `tanggal_publikasi DATE NOT NULL` — tanggal Pefindo mempublikasikan study
- `tanggal_efektif DATE NOT NULL` — mulai berlaku di BLIPS (set by maker)
- `tanggal_berakhir DATE` — nullable; set saat superseded oleh release baru
- `sumber_file TEXT NOT NULL` — original filename (sanitized)
- `file_minio_key TEXT NOT NULL` — MinIO object key `feeds/pefindo/{yyyy}/{mm}/{filename}`
- `file_size_bytes BIGINT NOT NULL`
- `job_id TEXT` — FK ke `sys.job.id` (nullable sebelum job dibuat)
- `calibration_status TEXT NOT NULL DEFAULT 'PENDING'` — `PENDING|PASSED|WARNING|FAILED`
- `calibration_warnings_count SMALLINT NOT NULL DEFAULT 0`
- `catatan TEXT` — optional maker notes
- `workflow_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT'`
- `workflow_instance_id UUID REFERENCES sys.workflow_instance(id)`
- Semua audit cols standard: `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `row_version`, `tenant_id`

Constraints:
- `chk_pd_release_workflow_status CHECK (workflow_status IN ('DRAFT','PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2','APPROVED','REJECTED','RETURNED'))`
- `chk_pd_release_calibration_status CHECK (calibration_status IN ('PENDING','PASSED','WARNING','FAILED'))`
- `uq_pd_release_kode UNIQUE (kode_rilis, tenant_id)`

Indexes:
- `idx_pd_release_tenant_created ON (tenant_id, created_at DESC)`
- `idx_pd_release_workflow ON (workflow_status) WHERE deleted_at IS NULL`
- `idx_pd_release_active ON (tanggal_efektif DESC) WHERE deleted_at IS NULL AND tanggal_berakhir IS NULL AND workflow_status = 'APPROVED'`
- `idx_pd_release_efektif ON (tanggal_efektif, tanggal_berakhir)` — untuk range lookup ECL engine

### Tabel 2: `mst.pd_pefindo_curve`

Matrix PD per baris `(release_id, rating_code, tenor_bucket)`.

Kolom wajib:
- `id UUID PK DEFAULT uuidv7()`
- `release_id UUID NOT NULL REFERENCES mst.pd_pefindo_release(id) ON DELETE RESTRICT`
- `rating_code VARCHAR(10) NOT NULL` — internal codes (post-strip `id` prefix): `AAA`, `AA+`, `AA`, `AA-`, `A+`, `A`, `A-`, `BBB+`, `BBB`, `BBB-`, `BB+`, `BB`, `BB-`, `B+`, `B`, `B-`, `CCC`, `D`. CHECK enum. (OQ-1 RESOLVED §10)
- `tenor_bucket VARCHAR(10) NOT NULL` — enum: `1Y`, `2Y`, `3Y`, `5Y`, `7Y`, `10Y`. Tenor > 10y di ECL Phase 4 extrapolate ke `10Y`. (OQ-2 RESOLVED §10)
- `pd_value NUMERIC(10,8) NOT NULL` — presisi DEC-016; constraint 0 ≤ pd_value ≤ 1
- `published_reference NUMERIC(10,8)` — nilai dari PDF tabel Pefindo (untuk kalibrasi check; nullable jika tenor tidak dipublikasikan eksplisit)
- `calibration_delta NUMERIC(10,8)` — `|pd_value - published_reference|`; null jika `published_reference` null
- Audit cols standard (minimal: `created_at`, `created_by`, `tenant_id`)

Constraints:
- `uq_pd_curve UNIQUE (release_id, rating_code, tenor_bucket)`
- `chk_pd_value_range CHECK (pd_value BETWEEN 0 AND 1)`
- `chk_pd_reference_range CHECK (published_reference IS NULL OR published_reference BETWEEN 0 AND 1)`

Indexes:
- `idx_pd_curve_release ON (release_id, rating_code, tenor_bucket)` — primary lookup
- `idx_pd_curve_rating_tenor ON (rating_code, tenor_bucket)` — cross-release comparison

### Deprecation `mst.pd_pefindo` (tabel lama)

Tambahkan comment + kolom `deprecated_at TIMESTAMPTZ DEFAULT now()` (nullable):
```sql
COMMENT ON TABLE mst.pd_pefindo IS
    'DEPRECATED as of migration 0013. Replaced by mst.pd_pefindo_release + mst.pd_pefindo_curve.
     Existing rows preserved for audit continuity. Service layer writes to new tables only.
     Scheduled for DROP in Phase 5 after data migration verified.';

ALTER TABLE mst.pd_pefindo
    ADD COLUMN IF NOT EXISTS deprecated_note TEXT DEFAULT 'Superseded by mst.pd_pefindo_release/curve per migration 0013';
```

### WORKFLOW_CONFIG seed di migration 0013

```json
{
  "entityType": "PD_PEFINDO",
  "eyes": 6,
  "retractable": false,
  "requiredPermissions": {
    "submit":   "pd_pefindo.submit",
    "review":   "pd_pefindo.review",
    "approve":  "pd_pefindo.approve",
    "approve2": "pd_pefindo.approve2",
    "reject":   "pd_pefindo.reject"
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
}
```

Seed ke `sys.config` dengan `ON CONFLICT DO NOTHING`.

### down.sql

Harus reversible:
- DROP TABLE `mst.pd_pefindo_curve`
- DROP TABLE `mst.pd_pefindo_release`
- Undo deprecation comment + kolom `deprecated_note` dari `mst.pd_pefindo`
- DELETE `WORKFLOW_CONFIG_PD_PEFINDO` dari `sys.config`
- DROP indexes

---

## §3. Backend (delegate ke backend-engineer-go)

**Module path**: `backend/internal/master/pdpefindo/`

**File structure** (ikuti pola `lpscoverage/`):
```
backend/internal/master/pdpefindo/
├── domain.go          — entity, request/response types, error codes, allowed cols
├── repo.go            — Repository interface + sqlx impl (List, GetByID, Create, Update, SoftDelete,
│                        UpdateWorkflowStatus, ListCurve, GetActiveCurve)
├── service.go         — Business logic: Create, Update, Delete, List, GetByID, GetActive, ListCurve,
│                        SyncWorkflowStatus, ValidatePeriodOverlap
├── handler.go         — HTTP handler: List, GetByID, Create, Update, Delete, Export, History,
│                        WorkflowStatus, Submit, Review, Approve, Approve2, Reject, GetActive
├── routes.go          — RegisterRoutes(v1 *gin.RouterGroup, h *Handler)
└── workflow_hook.go   — WorkflowHook implements workflow.EntityHook (BeforeCommit)
```

**Test files** (unit handler + workflow hook):
```
backend/internal/master/pdpefindo/
├── handler_test.go
├── workflow_hook_test.go
└── testutil_test.go
```

### API Endpoints

```
GET    /master/pd-pefindo                     → List releases (pd_pefindo.read)
POST   /master/pd-pefindo                     → Create release (upload initiation) (pd_pefindo.create)
GET    /master/pd-pefindo/export              → Export releases (pd_pefindo.read) — BEFORE /:id
GET    /master/pd-pefindo/active              → Get active curve ?tanggal=YYYY-MM-DD (pd_pefindo.read)
GET    /master/pd-pefindo/:id                 → GetByID (pd_pefindo.read)
PUT    /master/pd-pefindo/:id                 → Update metadata (catatan only; PD values immutable post-parse) (pd_pefindo.update)
DELETE /master/pd-pefindo/:id                 → SoftDelete DRAFT/REJECTED only (pd_pefindo.delete)
GET    /master/pd-pefindo/:id/curve           → List PD curve matrix for release (pd_pefindo.read)
GET    /master/pd-pefindo/:id/history         → Audit history (pd_pefindo.read)
GET    /master/pd-pefindo/:id/workflow        → Workflow status (pd_pefindo.read)
POST   /master/pd-pefindo/:id/submit          → Submit (pd_pefindo.submit)
POST   /master/pd-pefindo/:id/review          → Review (pd_pefindo.review)
POST   /master/pd-pefindo/:id/approve         → Approve step-up MFA (pd_pefindo.approve)
POST   /master/pd-pefindo/:id/approve2        → Approve2 step-up MFA (pd_pefindo.approve2)
POST   /master/pd-pefindo/:id/reject          → Reject (pd_pefindo.reject)
```

**Catatan penting**:
- `POST /master/pd-pefindo` menerima `multipart/form-data` (bukan JSON biasa) karena ada file upload.
  Fields: `file` (XLSX), `kode_rilis`, `tanggal_publikasi`, `tanggal_efektif`, `catatan` (optional).
  Handler validasi MIME + size → simpan ke MinIO via `internal/storage` → enqueue Asynq job → return 202.
- Upload endpoint WAJIB idempotent via `Idempotency-Key` header.
- PD curve values (`mst.pd_pefindo_curve`) **tidak bisa diedit** setelah parse job selesai — service layer enforce immutability (hanya `catatan` di release yang bisa di-update selama DRAFT/RETURNED).
- `GET /master/pd-pefindo/active` adalah endpoint yang akan di-consume ECL engine Phase 4 — desain harus efisien (indexed lookup `tanggal_efektif <= $date AND (tanggal_berakhir IS NULL OR tanggal_berakhir > $date) AND workflow_status = 'APPROVED'`).

### 6-eyes config (`WORKFLOW_CONFIG_PD_PEFINDO`)

Sama persis dengan `WORKFLOW_CONFIG_LGD_BASEL` dan `WORKFLOW_CONFIG_LPS_COVERAGE`:
- `eyes: 6`
- `stepUpRequired: { approve: true, approve2: true }`
- `sodRules: { reviewerNotMaker: true, approverNotMakerOrReviewer: true, approver2NotAnyPrevious: true }`
- Approver1 = ROLE-RISK, Approver2 = ROLE-ALCO

**Penting**: `approve` = ROLE-RISK (bukan ALCO seperti di lps_coverage). Approver1 adalah RISK Officer yang validasi teknis kalibrasi PD; Approver2 adalah ALCO yang beri sign-off kebijakan. Ini **beda** dari lps_coverage (kedua approver ALCO). Perlu konfirmasi di §10 OQ open-question — default sementara: Approver1=ROLE-RISK, Approver2=ROLE-ALCO.

### DI wiring di `cmd/api/main.go`

Ikuti pola lpscoverage:
```go
// PD Pefindo
pdPefindoRepo    := pdpefindo.NewRepo(db)
pdPefindoSvc     := pdpefindo.NewService(pdPefindoRepo, auditSvc, wfEngine, storageSvc, jobSvc)
pdPefindoHook    := pdpefindo.NewWorkflowHook(pdPefindoSvc, pdPefindoRepo)
pdPefindoHandler := pdpefindo.NewHandler(pdPefindoSvc, wfHandler)
wfEngine.RegisterHook("PD_PEFINDO", pdPefindoHook)
pdpefindo.RegisterRoutes(v1, pdPefindoHandler)
```

### Permission set

```
pd_pefindo.create   — ROLE-RISK (maker upload)
pd_pefindo.read     — ROLE-RISK, ROLE-AKUN-CTL, ROLE-AUDIT, ROLE-ALCO, ROLE-AKUN
pd_pefindo.update   — ROLE-RISK (catatan only, DRAFT/RETURNED)
pd_pefindo.delete   — ROLE-RISK (soft-delete DRAFT/REJECTED only)
pd_pefindo.submit   — ROLE-RISK
pd_pefindo.review   — ROLE-AKUN-CTL
pd_pefindo.approve  — ROLE-RISK (Approver1 — berbeda dengan maker)
pd_pefindo.approve2 — ROLE-ALCO
pd_pefindo.reject   — ROLE-AKUN-CTL, ROLE-RISK, ROLE-ALCO (sesuai step aktif)
pd_pefindo.export   — ROLE-RISK, ROLE-AUDIT, ROLE-AKUN-CTL
```

---

## §4. Integration Adapter (delegate ke integration-engineer)

### Pefindo XLSX Parser

**Worker package**: `backend/internal/worker/pefindo/`

**Task type**: `pefindo.parse_release`

**Payload**:
```go
type ParseReleasePayload struct {
    TenantID  string    `json:"tenantId"`
    ReleaseID uuid.UUID `json:"releaseId"`
    FileKey   string    `json:"fileKey"`   // MinIO object key
    ActorID   uuid.UUID `json:"actorId"`
}
```

**Sheet structure yang di-expect dari Pefindo XLSX**:
- Sheet name: `Default Rate` atau `PD Matrix` (integration-engineer harus validasi per PDF §4)
- Header row: `Rating`, `1Y`, `2Y`, `3Y`, `5Y`, `7Y`, `10Y` (dan potentially `Lifetime`)
- Data rows: satu per rating code
- Parser wajib: header presence validation, unknown header = warning (tidak fail), empty cell = skip row dengan log

**Rating code mapping**:
- Pefindo codes (`idAAA`, `idAA+`, dll.) → internal normalized codes (`AAA`, `AA+`, dll.)
- Mapping table di `internal/worker/pefindo/rating_mapping.go` (const map, tidak dari DB — Phase 3)
- Unknown rating code → `calibration_warnings` + skip; jangan fail seluruh job

**Tenor bucket normalization**:
- Header `1Y` → bucket `1Y`, `12M` → `1Y`, `3Y` → `3Y`, dst.
- Header tidak dikenal → warning, skip

**Kalibrasi check**:
- Reference values dari `mst.pd_pefindo_curve.published_reference` (diisi parser dari PDF hardcoded table atau future DB config)
- `calibration_delta = |parsed_pd - published_reference|`
- Threshold: jika `calibration_delta > 0.00000001` (1e-8) → `calibration_warning` entry
- Worker update `mst.pd_pefindo_release.calibration_status`:
  - `PASSED`: tidak ada warning
  - `WARNING`: ada warning tapi tidak fatal
  - `FAILED`: parse error yang mencegah insert (return error, trigger retry)

**Asynq job config**:
```go
asynq.NewTask("pefindo.parse_release", payload,
    asynq.MaxRetry(3),
    asynq.Timeout(5*time.Minute),
    asynq.Queue("feeds"),
)
```

**Progress reporting** (ikuti pola `internal/worker/ecl_calc.go` future reference):
- Redis `HSET job:{jobId} progress {pct} currentStep "..." updated_at {ts}`
- Redis `PUBLISH job-events:{jobId}` setiap update
- DB `sys.job` di-update setiap 10 row atau 5 detik (whichever first) — jangan hanya Redis

**MinIO**:
- Bucket: `feeds` (sudah ada dari Phase 2)
- Object path: `feeds/pefindo/{yyyy}/{mm}/{original_filename_sanitized}`
- Presigned GET URL TTL: 24 jam (untuk download review oleh reviewer)
- Upload via `internal/storage` package (multipart streaming, bukan baca full ke memory)

**DLQ handling**:
- Setelah 3 retry gagal → Asynq DLQ queue `feeds:dlq`
- `sys.job.status = failed`, `error_jsonb` berisi last error + stack trace ringkas
- Manual replay: `POST /api/v1/jobs/{jobId}/retry` (standard job endpoint Phase 2)
- Alert: Grafana alert rule `pefindo_parse_dlq_count > 0` → notify IT Admin

**Audit**:
- `PEFINDO.UPLOAD` → ditulis di handler (sebelum enqueue job, di transaksi insert release)
- `PEFINDO.PARSE_COMPLETE` → ditulis worker saat job selesai (dengan `result_jsonb` summary)
- `PEFINDO.PARSE_FAILED` → ditulis worker saat max retry habis

---

## §5. Frontend (delegate ke frontend-engineer-nextjs — setelah uiux-designer selesai desain)

**Page structure**: `frontend/src/app/master/pd-pefindo/`

```
app/master/pd-pefindo/
├── page.tsx                   — List releases (DataTable §1)
├── new/
│   └── page.tsx               — Upload form + JobProgressPanel (UX §3)
├── [id]/
│   ├── page.tsx               — Detail release + PD matrix heatmap (rating × tenor)
│   ├── edit/
│   │   └── page.tsx           — Edit catatan only (limited fields)
│   └── history/
│       └── page.tsx           — Workflow trail + audit history
```

**Komponen kunci**:
- `components/blips/DataTable` — reuse untuk list releases (sort, cursor paging, filter, export)
- `components/blips/JobProgressPanel` — subscribe SSE `GET /api/v1/jobs/{jobId}/stream`, fallback polling 2s
- `components/blips/WorkflowStatusBadge` — reuse dari lpscoverage
- `components/blips/MakerReviewerApproverPanel` — 6-eyes panel (reuse + extend untuk approve2)
- **Baru**: `components/blips/PDCurveHeatmap` — matrix visualization rating × tenor, color-coded by PD intensity (Recharts atau CSS grid)
- **Baru**: `components/blips/CalibrationWarningBanner` — banner WARNING kuning jika `calibration_warnings_count > 0`

**Upload form** (`new/page.tsx`):
- `react-dropzone` atau native `<input type="file">` dengan XLSX MIME restriction client-side
- Fields: `kode_rilis`, `tanggal_publikasi` (date), `tanggal_efektif` (date), `catatan` (textarea optional)
- Submit → `POST /api/v1/master/pd-pefindo` (multipart) → 202 → `JobProgressPanel` muncul
- Form submit pattern: disable button + spinner (UX §2), Idempotency-Key di-generate client-side (`uuidv4()`)
- Error: highlight field + inline message, toast merah persistent
- Sukses toast: "Release {kode_rilis} berhasil diupload. Job parsing sedang berjalan."

**Detail page** (`[id]/page.tsx`):
- Header: release metadata + `WorkflowStatusBadge` + `CalibrationWarningBanner`
- Tab 1: PD Curve Matrix — `PDCurveHeatmap` rating × tenor dengan nilai PD
- Tab 2: Calibration Check — list warning (rating_code, tenor_bucket, parsed, reference, delta)
- Tab 3: Workflow — `MakerReviewerApproverPanel` (submit/review/approve/approve2/reject)
- Action buttons sesuai `workflow_status` dan permission user

**Schemas**: `frontend/src/lib/schemas/pd-pefindo.schema.ts` (Zod)
- `pdPefindoUploadSchema` — untuk form upload
- `pdPefindoUpdateSchema` — untuk edit catatan
- `workflowActionSchema` — reuse dari lpscoverage

**API client**: `frontend/src/lib/api/pd-pefindo.api.ts`
- `list(params)`, `getById(id)`, `create(formData)`, `update(id, data)`, `softDelete(id)`, `export(params)`
- `getActive(tanggal)`, `getCurve(releaseId)`
- `submit(id, body)`, `review(id, body)`, `approve(id, body)`, `approve2(id, body)`, `reject(id, body)`
- `getAuditHistory(id)`, `getWorkflowStatus(id)`

**UX rules wajib**:
- List = sort + cursor paging + filter + export (UX §1)
- Upload + workflow = toast sukses/gagal spesifik (UX §2)
- Parse job = `JobProgressPanel` SSE (UX §3)
- Step-up MFA dialogs untuk approve + approve2
- Empty state matrix jika job belum selesai: "Parsing sedang berlangsung..." skeleton

---

## §6. QA (delegate ke qa-engineer)

**UAT file**: `docs/uat/master-data/APP-A-MSTR-007-pd-pefindo-uat-001.md`

**Integration test**: `backend/internal/test/integration/pdpefindo_test.go`

### Test Cases

| TC | Deskripsi | Expected |
|---|---|---|
| TC-001 | Upload XLSX valid → parse job selesai → release DRAFT | `sys.job.status=completed`, `mst.pd_pefindo_curve` rows, `calibration_status=PASSED/WARNING` |
| TC-002 | Upload XLSX invalid (broken file / wrong MIME) → 422 | Error `VALIDATION_FAILED`, job NOT enqueued, file NOT stored |
| TC-003 | Upload XLSX dengan header kolom tidak dikenal | Job complete dengan `calibration_warnings`, bukan fail |
| TC-004 | 6-eyes cycle lengkap: Maker→Reviewer→Approver1(RISK+MFA)→Approver2(ALCO+MFA) | `workflow_status=APPROVED`, 4 distinct users, 2 step-up MFA signatures |
| TC-005 | SoD violation — Maker coba menjadi Reviewer via API langsung | `SOD_VIOLATION` 403 |
| TC-006 | SoD violation — Approver1 sama dengan Maker atau Reviewer | `SOD_VIOLATION` 403 |
| TC-007 | SoD violation — Approver2 sama dengan siapapun sebelumnya | `SOD_VIOLATION` 403 |
| TC-008 | Single-active period overlap: approve release dengan tanggal_efektif yang sudah ada APPROVED release aktif | `422 PD_PERIOD_OVERLAP` |
| TC-009 | Kalibrasi mismatch > 1e-8 → `calibration_warnings` populated, job tetap `completed` (tidak gagal) | `calibration_status=WARNING`, `calibration_warnings_count > 0` |
| TC-010 | Workflow hook atomicity: `BeforeCommit` gagal → rollback workflow + release status tidak berubah | Atomik; tidak ada partial state |
| TC-011 | Approve tanpa step-up MFA (expired atau missing stepup_token) | `401/403 MFA_STEP_UP_REQUIRED` |
| TC-012 | Idempotency: upload dengan Idempotency-Key sama dua kali | Kedua return 202, hanya satu job di-enqueue |
| TC-013 | Export CSV list dengan filter aktif | File CSV respects filter, audit `PD_PEFINDO.EXPORT` ditulis |
| TC-014 | GET `/active?tanggal=...` dengan tanggal di luar semua periode release | 404 `NOT_FOUND` + meaningful message |
| TC-015 | Phase 4 placeholder: `GET /active` response dapat di-consume oleh ECL engine mock (happy path) | Response shape sesuai kontrak |

**Catatan TC yang diganti/diperluas dari instruksi awal**:
- TC-006 dari instruksi (`calibration mismatch`) diimplementasi sebagai TC-009 (non-blocking warning)
- TC-007 dari instruksi (`workflow hook atomicity`) menjadi TC-010
- TC-008 dari instruksi (`LPS aggregator preview`) diubah menjadi TC-015 (Phase 4 placeholder contract check) karena LPS aggregator sudah di modul 4

**4 User test wajib (SoD)**:
```
risk.officer.1  → ROLE-RISK      (Maker upload + Approver1)
akun.ctl.1      → ROLE-AKUN-CTL  (Reviewer)
risk.officer.2  → ROLE-RISK      (Approver1 — beda dengan maker)
alco.1          → ROLE-ALCO      (Approver2)
```

Catatan: karena Approver1 adalah ROLE-RISK, dibutuhkan 2 user ROLE-RISK berbeda (risk.officer.1 sebagai maker, risk.officer.2 sebagai approver1). Tambah `risk.officer.2` ke seed data.

---

## §7. Compliance Gate (BLOCKING — ifrs9-compliance-reviewer)

Wajib selesai sebelum merge ke `develop`.

**Checklist compliance reviewer**:

1. **6-eyes workflow konfigurasi benar**: `eyes=6`, `stepUpRequired.approve=true`, `stepUpRequired.approve2=true`, `approver2NotAnyPrevious=true` — verifikasi di `WORKFLOW_CONFIG_PD_PEFINDO` dan di integration test TC-004.

2. **PD precision `NUMERIC(10,8)`**: verifikasi di migration DDL `mst.pd_pefindo_curve.pd_value` dan di Go layer (`shopspring/decimal` — bukan float64). Verifikasi test TC-001 memeriksa presisi setelah round-trip DB.

3. **Kalibrasi reconcile ke Pefindo study**: verifikasi parser mengisi `published_reference` dan `calibration_delta` — dan bahwa warning di-expose ke reviewer (bukan disembunyikan). Test TC-009. Compliance reviewer wajib spot-check 3 rating bucket dari PDF `Pefindo_Annual_Default_Study_2007-2025_EN.pdf` terhadap nilai dalam `mst.pd_pefindo_curve`.

4. **Rating notch mapping untuk SICR** (DEC-011): verifikasi `rating_mapping.go` menghasilkan urutan notch yang benar (AAA=1, AA+=2, dst.) konsisten dengan `mst.rating_history_counterparty` dan SICR trigger logic Phase 4. Ini termasuk dalam scope review karena PD salah urutan notch = ECL salah.

5. **Single-active invariant**: verifikasi service `ValidatePeriodOverlap` + test TC-008. Dua release APPROVED tidak boleh overlap `tanggal_efektif`.

6. **Stage-aware PD usage**: verifikasi bahwa getter `GetActive` mengembalikan `pd_value` yang dapat di-dispatch ke Stage 1 (12M) vs Stage 2 (Lifetime). Tenor bucket `1Y` = Stage 1 PD_12M, `LIFETIME` = Stage 2 PD_Lifetime.

7. **No hard delete** di `mst.pd_pefindo_release` dan `mst.pd_pefindo_curve` — soft delete only. Verifikasi service `SoftDelete` hanya set `deleted_at`.

**VERDICT format**: `PASS` / `CONDITIONAL-PASS (issue: <keterangan> — follow-up ticket)` / `BLOCK (<alasan>)`.

---

## §8. Security Gate (advisory — kecuali touch auth/aud/PII)

Modul ini TIDAK menyentuh auth, PII kolom terenkripsi, atau `aud.*` schema secara langsung (audit ditulis via `internal/audit` package standard). Security gate adalah **advisory** untuk modul ini.

**Security checklist** (untuk security-engineer review):

1. **File upload security**:
   - MIME validation di handler layer (content-type header + `http.DetectContentType` untuk magic bytes)
   - Size limit 50MB enforced sebelum read (tidak buffer full file ke memory)
   - Filename sanitization (strip path separators, normalize, no `..`)
   - Antivirus: Phase 3 = placeholder log warning; ClamAV integration = Phase 5 backlog. Catat sebagai MEDIUM finding.

2. **MinIO presigned URL**: TTL 24 jam, hanya untuk user dengan `pd_pefindo.read` permission. URL generation TIDAK di-log (signed URL mengandung credential).

3. **Idempotency-Key check** di upload endpoint (multipart form).

4. **Tenant isolation**: semua query wajib `WHERE tenant_id = $1`.

5. **Input validation**: rating_code, tenor_bucket dari XLSX di-parse ke whitelist internal — tidak di-pass langsung ke SQL.

6. **Rate limiting**: upload endpoint 10 req/min per user (file upload lebih sensitif dari read).

7. **Audit trail**: `PEFINDO.UPLOAD`, `PEFINDO.PARSE_COMPLETE`, `PEFINDO.PARSE_FAILED`, `PD_PEFINDO.EXPORT` semua di `aud.audit_log` di-transaksi yang sama.

**Temuan yang diprediksi HIGH** (wajib ditangani sebelum merge):
- Tidak ada — modul ini tidak touch PII/auth/audit schema directly.

**Temuan yang diprediksi MEDIUM** (boleh merge dengan follow-up ticket):
- Antivirus ClamAV belum terimplementasi (placeholder di Phase 3) → ticket Phase 5.

---

## §9. Handoff Order (DAG)

```
[Prasyarat: PR #15 merge ke develop]
            |
            v
     data-modeler
     migration 0013 (mst.pd_pefindo_release + curve + deprecate lama + WORKFLOW_CONFIG seed)
            |
            +---------------------------+
            v                           v
  backend-engineer-go          uiux-designer
  (pdpefindo CRUD +            (screen desain: list,
  workflow hook +               upload form, detail heatmap,
  upload multipart handler)     workflow panel, progress)
            |                           |
            v                           |
  integration-engineer                  |
  (pefindo XLSX parser +               |
  Asynq job + MinIO +                  |
  SSE progress +                       |
  DLQ + audit)                         |
            |                           v
            +---------------------------+
                        |
                        v
              frontend-engineer-nextjs
              (pages + components +
               JobProgressPanel + PDCurveHeatmap)
                        |
                        v
                   qa-engineer
                   (UAT + integration tests TC-001..TC-015)
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
                  PR: feature/phase-3-pd-pefindo → develop
```

**Paralel yang valid**:
- `data-modeler` tidak perlu menunggu siapapun.
- `backend-engineer-go` + `uiux-designer` paralel setelah `data-modeler` selesai.
- `integration-engineer` bisa mulai parser logic segera setelah schema terkonfirmasi (tidak perlu backend-engineer-go selesai).
- `frontend-engineer-nextjs` mulai setelah `uiux-designer` selesai desain DAN OpenAPI contract dari `system-analyst` terkonfirmasi.
- `qa-engineer` mulai UAT script (doc) paralel; integration test menunggu backend + integration selesai.
- `ifrs9-compliance-reviewer` dan `security-engineer` paralel setelah QA test hijau.

**Blocking dependencies**:
- `system-analyst` harus produksi OpenAPI fragment untuk `pd-pefindo` SEBELUM `frontend-engineer-nextjs` dan `backend-engineer-go` mulai implementation. Ini termasuk multipart upload contract + job response shape.
- `integration-engineer` (Asynq worker) harus selesai sebelum `qa-engineer` bisa jalankan TC-001..TC-003.

---

## §10. Open Questions + Risk

### Open Questions — RESOLVED 2026-06-08

**OQ-1 — Rating code mapping** ✅ RESOLVED
Format storage di `pd_pefindo_curve.rating_code` = **internal** (`AAA`, `AA+`, `AA`, ...). Pefindo native (`idAAA`, dsb.) di-strip prefix oleh parser di integration-engineer layer sebelum INSERT. Konsisten dengan `mst.rating_mapping` (Phase 4 instrumen).

**Action**: parser worker hold deterministic map `idAAA`→`AAA`, dst. Unit test wajib cover semua 18 kode + 4 sub-tier `+/-`.

**OQ-2 — Tenor bucket granularity** ✅ RESOLVED
Tenor max = **10 tahun** (`10Y`). Sesuai Pefindo Annual Default Study publish 1Y-10Y cumulative default rate. Tenor instrumen > 10y di Phase 4 ECL engine: **extrapolate ke nilai `10Y`** (flat, bukan asymptotic). Bucket `LIFETIME` = alias `10Y` di lookup, bukan kolom terpisah.

**Action**: drop `LIFETIME` dari enum tenor_bucket di schema. Whitelist: `1Y, 2Y, 3Y, 5Y, 7Y, 10Y`.

**OQ-3 — Multi-currency PD** ✅ RESOLVED — DEFER Phase 4
Phase 3 IDR-only. FCY exposure (USD/SGD/JPY) pakai PD IDR sebagai proxy sementara di ECL engine Phase 4. Adjustment policy (sovereign spread, dsb.) didefinisikan di Phase 4 saat counterparty + FX rate terintegrasi.

**Action**: tidak ada perubahan schema; documented as Phase 4 dependency di SoW.

**OQ-4 — Backfill historis 2007-2025** ✅ RESOLVED — DEFER Phase 5
Phase 3 cukup ingest rilis terbaru (2025-Q4) sebagai release pertama via workflow normal. Backfill 2007-2024 = one-time migration job di Phase 5 saat butuh historical ECL recompute. Tidak via 6-eyes workflow (impractical untuk 18 tahun); direct SQL insert dengan audit trail manual + ALCO sign-off batch.

**Action**: tidak ada perubahan Phase 3. Ditulis di RESUME-phase-3 sebagai Phase 5 backlog.

**OQ-5 — Approver1 Role (RISK vs AKUN-CTL)** ✅ RESOLVED
Approver1 = **ROLE-RISK** (Risk Officer). Validasi teknis kalibrasi PD curve vs Pefindo published rate adalah domain Risk. Approver2 = ROLE-ALCO (kebijakan + cutover authorization).

**Action**: `WORKFLOW_CONFIG_PD_PEFINDO.approve.role = "ROLE-RISK"`, `approve.requireStepUp = true` (DEC-027 step-up mandatory untuk ECL param). Permission `ecl_parameter.approve` already covers ROLE-RISK per personas.md.

### Risk Matrix

| Risk | Severity | Mitigasi |
|---|---|---|
| Rating code mismatch Pefindo vs internal → PD salah di-lookup | HIGH | Parser unit test dengan sample file dari PDF; kalibrasi check; compliance review mandatory |
| Tenor bucket inconsistency → Stage 1 vs Stage 2 PD confusion | HIGH | Enum whitelist di DB + parser; compliance reviewer spot-check mapping |
| XLSX dengan formula macro (formula injection) | MEDIUM | MIME check + ClamAV placeholder; tidak evaluate formulas (excelize default read-only) |
| Kalibrasi mismatch tidak terdeteksi karena reference hardcode stale | MEDIUM | Reviewer wajib lihat calibration_warnings di UI; compliance reviewer spot-check PDF |
| Backfill 2007-2025 tidak dilakukan → ECL engine Phase 4 tidak punya historical PD | LOW | Noted di OQ-4; Phase 5 task |
| Single-active overlap jika cutover tidak di-set saat approve rilis baru | MEDIUM | Service `ValidatePeriodOverlap` + TC-008; UI warn jika ada active release saat membuat baru |
| MinIO storage full (XLSX files akumulasi) | LOW | Lifecycle policy: files > 2 tahun → archive tier. Phase 5. |
| Worker timeout untuk file besar (> 50MB edge case) | LOW | Asynq timeout 5 menit; size limit 50MB di handler |

### Rollback plan
- Jika migration 0013 harus di-rollback: `down.sql` drop tables baru, restore `mst.pd_pefindo` (no data loss karena tabel lama tidak di-drop).
- Jika bug post-merge di phase-3-pd-pefindo: hotfix branch dari `develop` (bukan `main` karena belum di-release).

---

## §11. Acceptance Criteria Sebelum Merge

Semua item berikut HARUS terpenuhi sebelum PR `feature/phase-3-pd-pefindo` → `develop` di-merge:

- [ ] CI hijau: `backend-lint` (golangci-lint v1.59.1), `backend-test` (unit), `frontend-build`, `frontend-lint`
- [ ] Integration tests TC-001 sampai TC-015 hijau (dijalankan di CI environment dengan Docker)
- [ ] UAT `APP-A-MSTR-007-pd-pefindo-uat-001.md` di-sign-off oleh ROLE-RISK dan ROLE-ALCO (manual sign di dokumen UAT)
- [ ] `ifrs9-compliance-reviewer` VERDICT = `PASS` atau `CONDITIONAL-PASS` dengan follow-up ticket terdokumentasi
- [ ] `security-engineer` review selesai (advisory); temuan HIGH = none; MEDIUM = ticket dibuat
- [ ] Migration `000013_pd_pefindo_schema.up.sql` + `.down.sql` ditest idempotent di UAT environment (up twice, down, up again)
- [ ] `GET /master/pd-pefindo/active` contract compatible dengan ECL engine Phase 4 placeholder (TC-015)
- [ ] Tidak ada `float64` untuk PD values di Go code (shopspring/decimal enforced — verifikasi golangci-lint + code review)
- [ ] `docs/uat/master-data/APP-A-MSTR-007-pd-pefindo-uat-001.md` exists dan complete
- [ ] Open questions OQ-1, OQ-2, OQ-5 dijawab dan decision dicatat di bagian ini (update plan doc)

---

## Lampiran — Commit convention

Branch: `feature/phase-3-pd-pefindo`
Commits expected:
```
feat(db): add migration 0013 mst.pd_pefindo_release and curve schema
feat(app-a): implement pdpefindo CRUD + 6-eyes workflow
feat(integ): add pefindo XLSX parser Asynq worker + MinIO upload
feat(web): add pd-pefindo list/upload/detail screens with PD heatmap
test(app-a): add pdpefindo integration tests TC-001..TC-015
docs(app-a): add UAT script APP-A-MSTR-007-pd-pefindo
```

PR target: `develop` (squash and merge per git-conventions.md).
