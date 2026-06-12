# P4-M8 — ECL Calc Run Orchestration (Asynq + Seal): User Stories

**Story Set ID**: P4-M8
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 3)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate) + `security-engineer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-12
**Branch**: `feature/phase-4-m7-ecl-core-engine` (current) → akan dipecah ke `feature/app-c-calcrun-seal` sebelum PR

**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §5 (calc run orchestration), §6 (seal workflow), SoW_v1.4.docx §4
**Linked BRD**: BRD §8.2 (ECL Computation Requirements), §9.3 (Seal + Immutability), RACI: ROLE-RISK (R/trigger), ROLE-ALCO (A/seal approve), ROLE-CFO (I/final), ROLE-AUDIT (I)

**Linked Decision Log**:
- DEC-010 — ECL formula: 3-stage × 3-skenario × dual FL multiplier. Default bobot Good/Normal/Bad = 0.25/0.50/0.25.
- DEC-017 — Workflow: 4-eyes rutin, **6-eyes** untuk parameter master + klasifikasi PSAK 71. SoD `maker_id ≠ reviewer_id ≠ approver_id`.
- DEC-018 — Audit trail append-only, hash-chain. `ecl.*` no hard delete. `ecl.calc_header` immutable setelah `sealed_at` set.
- DEC-021 — Idempotency-Key wajib di setiap endpoint mutating.
- DEC-026 — MFA mandatory: ROLE-ALCO (seal approve).
- DEC-027 — Step-up MFA: **seal calc run** (re-prompt meski `mfa_verified=true` jika > 5 menit lalu).

**Depends on (harus MERGED)**:
- P4-M7 — ECL core engine: `ECLEngine.ComputeSingle()`, `ECLEngine.BulkCompute()`, `ecl.calc_result_line` (mig 000029+000030), `fn_ecl_calc_no_modify_when_sealed`, sealed-run guard di ComputeSingle/BulkCompute
- Phase 2 Asynq + job infra: `sys.job` (mig 000004), `GET /api/v1/jobs/{jobId}`, SSE stream endpoint, `JobProgressPanel` frontend component

**Handoff berikutnya**:
- `system-analyst` — OpenAPI fragment: `POST /ecl/calc-runs`, `POST /ecl/calc-runs/{id}/start`, `GET /ecl/calc-runs/{id}`, `POST /ecl/calc-runs/{id}/seal`, `POST /ecl/calc-runs/{id}/cancel`; state machine calc_run lifecycle; Go interface `CalcRunService`
- `data-modeler` — migration 000031: tabel `ecl.calc_run` (header baru terpisah dari `ecl.calc_header` per-instrumen), kolom `parameter_snapshot` (frozen params), `seal_workflow` tracking (maker_id, reviewer_id, approver_id, sign-off chain)
- `security-engineer` — **BLOCKING** review: seal endpoint step-up MFA, audit in-transaction, SoD 6-eyes enforcement
- `ifrs9-compliance-reviewer` — **BLOCKING** gate: seal = immutable sesuai DEC-018, parameter freeze, 6-eyes workflow

**Open Questions (OQ) flagged dari modul ini**:

| ID | Pertanyaan | Default assumsi | Keputusan butuh |
|---|---|---|---|
| **OQ-M8-1** | Seal workflow: 4-eyes atau 6-eyes? DEC-017 menyebut 6-eyes untuk "klasifikasi PSAK 71 dan parameter master". Calc run seal bukan klasifikasi, namun menyentuh ECL result yang menjadi dasar laporan keuangan. | **Asumsi default: 6-eyes** (RISK maker → ALCO approver-1 → ALCO approver-2 atau ROLE-CFO). Ceklist perlu dikonfirmasi ke ALCO + FSD-APP-C. | ifrs9-compliance-reviewer + ALCO + FSD-APP-C §6 |
| **OQ-M8-2** | Setelah periode pertama di-SEAL, apakah create calc_run baru untuk periode yang sama di-block atau hanya dibolehkan dengan ALCO override? Story 6 mendokumentasikan "blocked after seal + defer override feature". | Block create new calc_run jika sudah ada sealed run untuk periode yang sama (default). Override feature defer ke backlog. | BRD §9.3 + ALCO policy |
| **OQ-M8-3** | `ecl.calc_run` sebagai tabel baru vs. reuse field di `ecl.calc_header`? Mig 000029 menambahkan `status`, `sealed_at`, `calc_run_job_id` ke `ecl.calc_header` per-instrumen, tapi tidak ada "run header" level yang mewakili seluruh calc run. M8 butuh entitas `calc_run` terpisah. | Tabel baru `ecl.calc_run` (mig 000031): satu row per calc run, berisi `periode_id`, `status`, `total_instrumen`, `processed_count`, `error_count`, `seal_workflow`. `ecl.calc_result_line.calc_run_id` FK ke sini. | data-modeler + system-analyst — konfirmasi sebelum mig 000031 ditulis |
| **OQ-M8-4** | FK type unification: mig 000029 meninggalkan `ecl.calc_result_line.calc_run_id` tanpa FK (OQ-M7-2: UUID vs TEXT mismatch `sys.job`). M8 memiliki kesempatan resolve ini saat membuat `ecl.calc_run` (tabel baru, PK bisa UUID). | `ecl.calc_run.id UUID PK`. `ecl.calc_result_line.calc_run_id UUID FK → ecl.calc_run(id)` (resolve OQ-M7-2 sekaligus). `sys.job.id TEXT` tetap, tidak di-ubah PK-nya. | data-modeler + system-analyst |
| **OQ-M8-5** | `parameter_snapshot`: bagaimana scope freeze? Freeze seluruh parameter aktif saat `start` dipanggil (PD curves, LGD pools, bobot skenario, FL multiplier, LPS coverage, kurs BI JISDOR) atau hanya parameter yang berubah sejak run terakhir? | Freeze **semua** parameter aktif saat `start` (full snapshot). Disimpan di `ecl.calc_run.parameter_snapshot_jsonb`. Tidak ada tabel terpisah untuk fase pertama. | ifrs9-compliance-reviewer (audit trail completeness) |

---

## Schema Target (M8 — `ecl.calc_run` baru, mig 000031)

Tabel baru `ecl.calc_run` diperlukan untuk M8 karena `ecl.calc_header` adalah per-instrumen (bukan per-run). M8 perlu entitas "run header" yang mengkoordinasikan seluruh lifecycle.

```sql
-- ecl.calc_run (diusulkan, dikonfirmasi oleh data-modeler + system-analyst)
CREATE TABLE ecl.calc_run (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    periode_id              TEXT        NOT NULL,  -- FK → mst.periode_buku(id)
    evaluation_date         DATE        NOT NULL,
    scope                   TEXT        NOT NULL DEFAULT 'ALL_ACTIVE',
    status                  TEXT        NOT NULL DEFAULT 'DRAFT',
    -- status: DRAFT | IN_PROGRESS | COMPLETED | COMPLETED_WITH_ERRORS | SEALED | CANCELLED

    -- Asynq job linkage
    job_id                  TEXT REFERENCES sys.job(id) ON DELETE RESTRICT,

    -- Progress counters (updated by worker)
    total_instrumen         INTEGER,
    processed_count         INTEGER     NOT NULL DEFAULT 0,
    error_count             INTEGER     NOT NULL DEFAULT 0,

    -- Timing
    started_at              TIMESTAMPTZ,
    completed_at            TIMESTAMPTZ,

    -- Parameter snapshot frozen at start (full snapshot for audit trail)
    parameter_snapshot_jsonb JSONB,

    -- Seal workflow (6-eyes: RISK maker → ALCO approver-1 → ALCO/CFO approver-2)
    seal_requested_by       UUID,
    seal_requested_at       TIMESTAMPTZ,
    seal_approved_by_1      UUID,
    seal_approved_at_1      TIMESTAMPTZ,
    seal_signature_1        TEXT,       -- SHA-256 of approval payload
    seal_approved_by_2      UUID,
    seal_approved_at_2      TIMESTAMPTZ,
    seal_signature_2        TEXT,
    sealed_at               TIMESTAMPTZ,
    seal_chain_jsonb        JSONB,      -- full sign-off audit chain

    -- Cancel tracking
    cancelled_by            UUID,
    cancelled_at            TIMESTAMPTZ,
    cancel_reason           TEXT,       -- ≥ 30 chars

    -- Superseded tracking
    superseded_by_run_id    UUID REFERENCES ecl.calc_run(id),

    -- Standard audit cols per db-conventions.md
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by              UUID        NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by              UUID        NOT NULL,
    deleted_at              TIMESTAMPTZ,
    deleted_by              UUID,
    row_version             BIGINT      NOT NULL DEFAULT 1,
    tenant_id               TEXT        NOT NULL DEFAULT 'TUGURE',

    CONSTRAINT chk_ecl_calc_run_status CHECK (
        status IN ('DRAFT','IN_PROGRESS','COMPLETED','COMPLETED_WITH_ERRORS','SEALED','CANCELLED')
    ),
    CONSTRAINT chk_ecl_calc_run_cancel_reason
        CHECK (cancelled_at IS NULL OR length(cancel_reason) >= 30),
    CONSTRAINT chk_ecl_calc_run_sod_seal
        CHECK (seal_requested_by IS DISTINCT FROM seal_approved_by_1
               AND seal_requested_by IS DISTINCT FROM seal_approved_by_2
               AND seal_approved_by_1 IS DISTINCT FROM seal_approved_by_2)
);
```

**M8 juga akan**:
- Tambah FK constraint `ecl.calc_result_line.calc_run_id → ecl.calc_run(id)` (resolve OQ-M7-2, OQ-M8-4)
- Backfill `ecl.calc_header.calc_run_job_id` dari legacy `calc_run_id` sesuai rencana mig 000029
- Tambah trigger `fn_ecl_calc_run_no_modify_when_sealed` pada `ecl.calc_run`

---

## Permissions

| Permission | Actor | Deskripsi |
|---|---|---|
| `calc_run.create` | ROLE-RISK | Buat calc_run baru (DRAFT) |
| `calc_run.start` | ROLE-RISK | Trigger bulk compute (DRAFT → IN_PROGRESS) |
| `calc_run.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO | Baca status dan hasil calc_run |
| `calc_run.seal_request` | ROLE-RISK | Request seal (submit ke approver) |
| `calc_run.seal_approve` | ROLE-ALCO, ROLE-CFO | Approve seal step (step-up MFA wajib) |
| `calc_run.cancel` | ROLE-RISK (maker) | Cancel calc_run IN_PROGRESS atau DRAFT |
| `calc_run.export` | ROLE-RISK, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Export hasil |

---

## Story APP-C-M8-001 — Create Calc Run (DRAFT)

**Actor**: ROLE-RISK
**Trigger**: Manual via UI tombol "Buat Calc Run Baru" di halaman `/ecl/runs`; atau Asynq cron otomatis sebelum periode close (cron mode: `created_by` = service account ROLE-RISK)
**Goal**: Membuat entri `ecl.calc_run` baru dengan status `DRAFT`, terikat ke `periode_buku`, dengan scope default semua instrumen aktif di periode tersebut. Return `calc_run_id` yang dapat digunakan untuk trigger `start`.

**Pre-conditions**:
- `mst.periode_buku` dengan `periode_id` yang dimaksud exists, `status != 'HARD_CLOSED'`
- Tidak ada `ecl.calc_run` dengan status `IN_PROGRESS` untuk `periode_id` yang sama
- Tidak ada `ecl.calc_run` dengan status `SEALED` untuk `periode_id` yang sama (jika ada → error `CALC_RUN_PERIODE_ALREADY_SEALED`, kecuali override OQ-M8-2)
- User memiliki permission `calc_run.create`
- Idempotency-Key disertakan di header

**Post-conditions**:
- Baris baru di `ecl.calc_run` dengan `status = 'DRAFT'`, `processed_count = 0`, `error_count = 0`
- `created_by = RISK-01`, `created_at = now()`, `tenant_id = 'TUGURE'`
- Audit event `CALC_RUN.CREATED` ditulis ke `aud.audit_log` in-transaction
- Response `201 Created` dengan `{ data: { id, periode_id, status, evaluation_date } }`

**Permissions**: `calc_run.create`
**Audit Events**: `CALC_RUN.CREATED`

### Acceptance Criteria — APP-C-M8-001

```gherkin
Feature: Create ECL calc run (DRAFT)

  Background:
    Given mst.periode_buku "JUNI-2026" exists dengan status "OPEN"
    And RISK-01 memiliki role ROLE-RISK dan permission calc_run.create
    And tidak ada calc_run IN_PROGRESS atau SEALED untuk "JUNI-2026"

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: create DRAFT berhasil
  # ---------------------------------------------------------------
  Scenario: Create calc_run DRAFT sukses
    When RISK-01 POST /api/v1/ecl/calc-runs dengan:
      | body        | { "periode_id": "JUNI-2026", "evaluation_date": "2026-06-30", "scope": "ALL_ACTIVE" } |
      | headers     | Idempotency-Key: uuid-aaa-111                                                         |
    Then response 201 dengan:
      | data.status          | "DRAFT"                         |
      | data.periode_id      | "JUNI-2026"                     |
      | data.evaluation_date | "2026-06-30"                    |
      | data.processed_count | 0                               |
      | data.error_count     | 0                               |
    And ecl.calc_run row baru dengan status = 'DRAFT' dan created_by = RISK-01
    And aud.audit_log berisi event "CALC_RUN.CREATED" dengan:
      | entity_type   | ecl.calc_run         |
      | actor_user_id | RISK-01              |
      | after_jsonb   | {periode_id, status} |
    And toast sukses di UI: "Calc run untuk periode JUNI-2026 berhasil dibuat. Status: DRAFT."

  # ---------------------------------------------------------------
  # Skenario 2 — Idempotency: request ulang dengan key sama → replay
  # ---------------------------------------------------------------
  Scenario: Idempotent create — Idempotency-Key sama, payload sama → replay
    Given calc_run "CR-001" sudah dibuat dengan Idempotency-Key: uuid-aaa-111
    When RISK-01 POST /api/v1/ecl/calc-runs lagi dengan Idempotency-Key: uuid-aaa-111 dan payload identik
    Then response 200 dengan body identik dengan response pertama (IDEMPOTENCY_REPLAY)
    And tidak ada row baru di ecl.calc_run
    And aud.audit_log tidak menambah row baru untuk event ini

  # ---------------------------------------------------------------
  # Skenario 3 — Error: periode hard-closed
  # ---------------------------------------------------------------
  Scenario: Gagal create — periode buku hard-closed
    Given mst.periode_buku "MEI-2026" memiliki status "HARD_CLOSED"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "MEI-2026"
    Then response 423 dengan:
      | error.code    | "PERIODE_CLOSED"                                     |
      | error.message | "Periode MEI-2026 sudah hard-closed, tidak bisa membuat calc run baru." |
    And tidak ada row baru di ecl.calc_run

  # ---------------------------------------------------------------
  # Skenario 4 — Error: already IN_PROGRESS untuk periode yang sama
  # ---------------------------------------------------------------
  Scenario: Gagal create — sudah ada calc_run IN_PROGRESS untuk periode
    Given ecl.calc_run "CR-002" dengan status "IN_PROGRESS" untuk periode_id = "JUNI-2026"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026"
    Then response 422 dengan:
      | error.code    | "CALC_RUN_IN_PROGRESS_EXISTS"                                              |
      | error.message | "Sudah ada calc run CR-002 sedang berjalan untuk periode JUNI-2026. Tunggu hingga selesai atau cancel terlebih dahulu." |
    And tidak ada row baru di ecl.calc_run

  # ---------------------------------------------------------------
  # Skenario 5 — Error: sudah ada SEALED run untuk periode
  # ---------------------------------------------------------------
  Scenario: Gagal create — periode sudah memiliki calc_run SEALED
    Given ecl.calc_run "CR-SEALED-001" dengan status "SEALED" untuk periode_id = "JUNI-2026"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026"
    Then response 422 dengan:
      | error.code    | "CALC_RUN_PERIODE_ALREADY_SEALED"                                      |
      | error.message | "Periode JUNI-2026 sudah memiliki calc run yang di-seal (CR-SEALED-001). Override memerlukan persetujuan ALCO — fitur ini belum tersedia (backlog)." |
    And tidak ada row baru di ecl.calc_run

  # ---------------------------------------------------------------
  # Skenario 6 — Permission denied: ROLE-AKUN tidak boleh create
  # ---------------------------------------------------------------
  Scenario: Permission denied — ROLE-AKUN tidak punya calc_run.create
    Given AKUN-01 dengan role ROLE-AKUN (tidak memiliki calc_run.create)
    When AKUN-01 POST /api/v1/ecl/calc-runs
    Then response 403 dengan error.code = "FORBIDDEN"
    And error.message berisi "Permission calc_run.create tidak terpenuhi."

  # ---------------------------------------------------------------
  # Skenario 7 — Cron auto-create: created_by = service account, audit tetap ditulis
  # ---------------------------------------------------------------
  Scenario: Cron auto-create — created_by = service account ROLE-RISK
    When Asynq cron job "ECL_AUTO_CALC_RUN" di-trigger sebelum soft-close JULI-2026
    Then ecl.calc_run DRAFT dibuat dengan created_by = SERVICE_ACCOUNT_RISK_ID
    And aud.audit_log berisi event "CALC_RUN.CREATED" dengan actor_role = "ROLE-RISK-SERVICE"
    And tidak ada notif UI (cron mode — notif dikirim via email/webhook ke ROLE-RISK)
```

---

## Story APP-C-M8-002 — Start Bulk Compute (DRAFT → IN_PROGRESS)

**Actor**: ROLE-RISK (manual trigger post-create) atau auto-trigger setelah DRAFT create
**Trigger**: `POST /api/v1/ecl/calc-runs/{id}/start`
**Goal**: Transisi `ecl.calc_run.status` dari `DRAFT` ke `IN_PROGRESS`, freeze parameter snapshot, dispatch Asynq job memanggil M7 `ECLEngine.BulkCompute()`, link `job_id → sys.job`, return `{ jobId, statusUrl, streamUrl }` untuk progress monitoring (UX §3).

**Pre-conditions**:
- `ecl.calc_run` dengan `id` exists dan `status = 'DRAFT'`
- `mst.periode_buku` tidak `HARD_CLOSED`
- Parameter ECL aktif tersedia untuk `periode_id`: `mst.bobot_skenario`, `mst.pd_pefindo`, `mst.lgd_basel`, `mst.impact_mev_pd`, `mst.lps_coverage` — semua dengan `workflow_status = 'APPROVED'` oleh ALCO
- `mst.kurs` (BI JISDOR) tersedia untuk `evaluation_date`
- User memiliki permission `calc_run.start`
- Idempotency-Key disertakan

**Post-conditions**:
- `ecl.calc_run.status = 'IN_PROGRESS'`
- `ecl.calc_run.job_id` terisi dengan ULID dari `sys.job` yang baru dibuat
- `ecl.calc_run.parameter_snapshot_jsonb` di-freeze (snapshot semua parameter aktif)
- `ecl.calc_run.started_at = now()`
- `sys.job` baris baru dengan `type = 'ECL_CALC_RUN'`, `status = 'queued'`, `can_cancel = true`
- Audit event `CALC_RUN.STARTED` ditulis in-transaction
- Asynq task `ECL_CALC_RUN` di-dispatch ke worker queue
- Response `202 Accepted` dengan `{ jobId, statusUrl, streamUrl }`

**Permissions**: `calc_run.start`
**Audit Events**: `CALC_RUN.STARTED`

### Acceptance Criteria — APP-C-M8-002

```gherkin
Feature: Start bulk ECL compute — DRAFT → IN_PROGRESS

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" dengan status = "DRAFT", periode_id = "JUNI-2026"
    And parameter ECL APPROVED tersedia untuk JUNI-2026 (bobot, PD, LGD, FL, LPS, kurs)
    And RISK-01 memiliki permission calc_run.start

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: start berhasil, job dispatched
  # ---------------------------------------------------------------
  Scenario: Start calc_run sukses — transition DRAFT → IN_PROGRESS, Asynq job dispatched
    When RISK-01 POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/start dengan:
      | headers | Idempotency-Key: uuid-bbb-222 |
    Then response 202 dengan:
      | data.jobId     | {ULID dari sys.job baru}                              |
      | data.statusUrl | "/api/v1/jobs/{jobId}"                                |
      | data.streamUrl | "/api/v1/jobs/{jobId}/stream"                         |
    And ecl.calc_run "CR-JUNI-2026-001":
      | status                      | "IN_PROGRESS"                   |
      | job_id                      | {ULID dari sys.job baru}        |
      | started_at                  | IS NOT NULL                     |
      | parameter_snapshot_jsonb    | snapshot semua parameter aktif  |
    And sys.job baru dengan:
      | type       | "ECL_CALC_RUN"                   |
      | status     | "queued"                         |
      | can_cancel | true                             |
    And aud.audit_log berisi event "CALC_RUN.STARTED"
    And Asynq queue berisi task "ECL_CALC_RUN" dengan payload.calc_run_id = "CR-JUNI-2026-001"
    And UI: JobProgressPanel muncul dengan jobId yang baru

  # ---------------------------------------------------------------
  # Skenario 2 — Idempotency: double-click start → replay
  # ---------------------------------------------------------------
  Scenario: Idempotent start — Idempotency-Key sama dikirim dua kali
    Given RISK-01 sudah berhasil start CR-JUNI-2026-001 dengan Idempotency-Key uuid-bbb-222
    When RISK-01 POST start lagi dengan Idempotency-Key uuid-bbb-222 dan payload identik
    Then response 200 dengan body identik (IDEMPOTENCY_REPLAY)
    And tidak ada sys.job duplikat
    And tidak ada Asynq task duplikat
    And calc_run status tetap "IN_PROGRESS" (tidak di-restart)

  # ---------------------------------------------------------------
  # Skenario 3 — Error: status bukan DRAFT
  # ---------------------------------------------------------------
  Scenario: Gagal start — calc_run sudah IN_PROGRESS
    Given ecl.calc_run "CR-ALT-001" dengan status = "IN_PROGRESS"
    When RISK-01 POST /api/v1/ecl/calc-runs/CR-ALT-001/start
    Then response 422 dengan:
      | error.code    | "CALC_RUN_INVALID_TRANSITION"                                        |
      | error.message | "Calc run CR-ALT-001 tidak bisa di-start: status saat ini IN_PROGRESS (hanya DRAFT yang bisa di-start)." |

  # ---------------------------------------------------------------
  # Skenario 4 — Error: parameter ECL belum diapprove ALCO
  # ---------------------------------------------------------------
  Scenario: Gagal start — parameter ECL tidak tersedia / belum APPROVED
    Given mst.bobot_skenario untuk JUNI-2026 dalam status "PENDING_APPROVAL" (belum ALCO)
    When RISK-01 POST start untuk CR-JUNI-2026-001
    Then response 422 dengan:
      | error.code    | "ECL_PARAM_NOT_FOUND"                                                                 |
      | error.message | "Parameter ECL (bobot skenario) untuk periode JUNI-2026 belum disetujui ALCO. Hubungi ROLE-ALCO." |
    And ecl.calc_run status tetap "DRAFT"
    And tidak ada sys.job dibuat
    And tidak ada Asynq task di-dispatch

  # ---------------------------------------------------------------
  # Skenario 5 — Error: kurs BI JISDOR belum tersedia
  # ---------------------------------------------------------------
  Scenario: Gagal start — kurs BI JISDOR evaluation_date belum tersedia
    Given mst.kurs untuk tanggal "2026-06-30" belum diupload
    When RISK-01 POST start untuk CR-JUNI-2026-001 (evaluation_date = 2026-06-30)
    Then response 422 dengan:
      | error.code    | "FX_RATE_NOT_FOUND"                                                                |
      | error.message | "Kurs BI JISDOR untuk tanggal 2026-06-30 tidak tersedia. Upload kurs terlebih dahulu." |
    And tidak ada sys.job dibuat

  # ---------------------------------------------------------------
  # Skenario 6 — Error: calc_run SEALED tidak bisa di-start lagi
  # ---------------------------------------------------------------
  Scenario: Gagal start — calc_run sudah SEALED
    Given ecl.calc_run "CR-SEALED-001" dengan status = "SEALED"
    When RISK-01 POST /api/v1/ecl/calc-runs/CR-SEALED-001/start
    Then response 422 dengan error.code = "CALC_RUN_INVALID_TRANSITION"
    And error.message menyebutkan "status SEALED tidak bisa di-start kembali"
```

---

## Story APP-C-M8-003 — Monitor Progress + Mark Completion

**Actor**: Siapapun dengan permission `calc_run.read` (progress polling); ECL engine worker (internal completion update)
**Trigger**: Frontend polling `GET /api/v1/ecl/calc-runs/{id}` atau `JobProgressPanel` subscribe ke SSE stream `GET /api/v1/jobs/{jobId}/stream`. Worker internal: callback ke M8 handler setelah setiap batch + setelah seluruh instrumen selesai.
**Goal**: Memberikan visibility real-time progress bulk compute kepada user (UX §3), dan memastikan `ecl.calc_run.status` di-update ke `COMPLETED` atau `COMPLETED_WITH_ERRORS` ketika semua instrumen selesai diproses, dengan summary tersimpan di `sys.job.result_jsonb` dan `ecl.calc_run`.

**Pre-conditions**:
- `ecl.calc_run` exists dengan status `IN_PROGRESS`
- `sys.job` dengan `job_id` terkait exists dengan status `running` atau `completed`

**Post-conditions** (saat semua instrumen selesai):
- `ecl.calc_run.status = 'COMPLETED'` (jika `error_count = 0`) atau `'COMPLETED_WITH_ERRORS'` (jika `error_count > 0`)
- `ecl.calc_run.completed_at = now()`
- `ecl.calc_run.processed_count` dan `error_count` ter-update final
- `sys.job.status = 'completed'`, `sys.job.result_jsonb` berisi summary
- Audit event `CALC_RUN.COMPLETED` atau `CALC_RUN.COMPLETED_WITH_ERRORS` ditulis in-transaction
- Notifikasi sukses/gagal dikirim ke ROLE-RISK (via SSE completed event + global notif badge)

**Permissions**: `calc_run.read`
**Audit Events**: `CALC_RUN.COMPLETED`, `CALC_RUN.COMPLETED_WITH_ERRORS`

### Acceptance Criteria — APP-C-M8-003

```gherkin
Feature: Monitor progress bulk compute + auto-complete

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" status = "IN_PROGRESS"
    And sys.job "JOB-ULID-001" terkait dengan status = "running"
    And total instrumen scope = 1.000

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: progress real-time via SSE + completion
  # ---------------------------------------------------------------
  Scenario: Monitor progress via SSE + calc_run auto-COMPLETED
    When RISK-01 subscribe ke GET /api/v1/jobs/JOB-ULID-001/stream
    Then SSE events diterima saat worker update:
      | event: progress | data: { progress: 10, currentStep: "Menghitung instrumen 100 dari 1.000" } |
      | event: progress | data: { progress: 50, currentStep: "Menghitung instrumen 500 dari 1.000" } |
      | event: progress | data: { progress: 100, currentStep: "Selesai. 1.000 instrumen diproses." } |
    And setelah semua instrumen selesai:
      sys.job.status = "completed"
      sys.job.result_jsonb berisi:
        | total_scanned          | 1000  |
        | total_computed         | 995   |
        | total_skipped_fvtpl    | 5     |
        | total_poci_deferred    | 0     |
        | error_count            | 0     |
        | ecl_weighted_idr_total | {IDR} |
    And ecl.calc_run "CR-JUNI-2026-001":
      | status          | "COMPLETED" |
      | completed_at    | IS NOT NULL |
      | processed_count | 1000        |
      | error_count     | 0           |
    And aud.audit_log berisi "CALC_RUN.COMPLETED" dengan after_jsonb.processed_count = 1000
    And SSE event "completed" dikirim ke frontend
    And toast sukses di UI: "ECL Calc Run JUNI-2026 selesai. 995 instrumen dihitung, 5 FVTPL di-skip. Siap untuk di-seal."

  # ---------------------------------------------------------------
  # Skenario 2 — Polling fallback saat SSE tidak tersedia
  # ---------------------------------------------------------------
  Scenario: Progress polling fallback — SSE error, frontend polling setiap 2 detik
    Given SSE stream mengembalikan error (koneksi timeout)
    When frontend fallback polling GET /api/v1/jobs/JOB-ULID-001 setiap 2 detik
    Then setiap response 200 berisi:
      | data.status       | "running"                  |
      | data.progress     | {0-100}                    |
      | data.currentStep  | "..."                      |
      | data.canCancel    | true                       |
    And setelah completed: response berisi status="completed" dan result_jsonb
    And ecl.calc_run UI di-update dengan nilai terbaru

  # ---------------------------------------------------------------
  # Skenario 3 — COMPLETED_WITH_ERRORS: ada instrumen gagal
  # ---------------------------------------------------------------
  Scenario: Selesai dengan error — status COMPLETED_WITH_ERRORS
    Given 3 instrumen gagal diproses (EAD_MISSING_OUTSTANDING pada "OBL-CORRUPT-001..003")
    And 997 instrumen berhasil
    When bulk compute selesai
    Then ecl.calc_run "CR-JUNI-2026-001":
      | status      | "COMPLETED_WITH_ERRORS" |
      | error_count | 3                       |
    And sys.job.result_jsonb.errors berisi detail 3 instrumen gagal:
      [ { instrumen_id: "OBL-CORRUPT-001", error_code: "EAD_MISSING_OUTSTANDING" }, ... ]
    And aud.audit_log berisi "CALC_RUN.COMPLETED_WITH_ERRORS"
    And toast warning di UI: "Calc run selesai dengan 3 error. Instrumen dengan error perlu diperbaiki sebelum seal. Lihat detail."
    And link "Lihat detail error" mengarah ke /ecl/runs/CR-JUNI-2026-001?tab=errors

  # ---------------------------------------------------------------
  # Skenario 4 — GET calc_run status endpoint: data lengkap dikembalikan
  # ---------------------------------------------------------------
  Scenario: GET /api/v1/ecl/calc-runs/{id} mengembalikan status + progress terkini
    When RISK-01 GET /api/v1/ecl/calc-runs/CR-JUNI-2026-001
    Then response 200 dengan:
      | data.id               | "CR-JUNI-2026-001"   |
      | data.status           | {current_status}     |
      | data.processed_count  | {current_count}      |
      | data.error_count      | {current_count}      |
      | data.job.progress     | {0-100}              |
      | data.job.currentStep  | {latest_step}        |
      | data.job.statusUrl    | "/api/v1/jobs/..."   |

  # ---------------------------------------------------------------
  # Skenario 5 — Permission: ROLE-MAKER-TR tidak punya calc_run.read
  # ---------------------------------------------------------------
  Scenario: Permission denied — ROLE-MAKER-TR tidak bisa lihat calc_run status
    Given MAKER-01 dengan role ROLE-MAKER-TR
    When MAKER-01 GET /api/v1/ecl/calc-runs/CR-JUNI-2026-001
    Then response 403 dengan error.code = "FORBIDDEN"
```

---

## Story APP-C-M8-004 — Seal Calc Run (Immutable Freeze)

**Actor**: ROLE-RISK (`seal_request`), ROLE-ALCO (approver-1 + approver-2) atau ROLE-CFO (approver-2)
**Trigger**: `POST /api/v1/ecl/calc-runs/{id}/seal` — dengan 3-step signing flow (6-eyes per OQ-M8-1 default assumption)
**Goal**: Membekukan `ecl.calc_run` dan seluruh `ecl.calc_result_line` terkait secara permanen dan irreversible setelah mendapat tanda tangan 6-eyes (RISK maker → ALCO approver-1 → ALCO/CFO approver-2). Setelah seal: `sealed_at` di-set, DB trigger `fn_ecl_calc_no_modify_when_sealed` aktif mencegah modifikasi, semua approver wajib step-up MFA.

**Pre-conditions**:
- `ecl.calc_run.status IN ('COMPLETED', 'COMPLETED_WITH_ERRORS')`
- User yang request seal adalah ROLE-RISK dengan permission `calc_run.seal_request`
- Approver-1 dan Approver-2 memiliki permission `calc_run.seal_approve` (ROLE-ALCO atau ROLE-CFO)
- SoD: `seal_requested_by ≠ seal_approved_by_1 ≠ seal_approved_by_2`
- Step-up MFA selesai untuk setiap approver (DEC-027)
- Tidak ada `ecl.calc_run` SEALED lain untuk `periode_id` yang sama

**Post-conditions**:
- `ecl.calc_run.status = 'SEALED'`
- `ecl.calc_run.sealed_at`, `seal_approved_by_1`, `seal_approved_by_2`, `seal_chain_jsonb` terisi
- `fn_ecl_calc_no_modify_when_sealed` aktif: setiap UPDATE pada `ecl.calc_header` + `ecl.calc_result_line` dengan `calc_run_id` ini akan di-reject oleh DB dengan HTTP 423
- Audit event `CALC_RUN.SEALED` ditulis in-transaction dengan full sign-off chain di `aud.audit_log.after_jsonb`
- Parameter tidak bisa diubah pasca seal (`ECL_PARAM_FROZEN` 423 untuk semua operasi compute terkait)

**Permissions**: `calc_run.seal_request` (ROLE-RISK), `calc_run.seal_approve` (ROLE-ALCO, ROLE-CFO)
**Audit Events**: `CALC_RUN.SEAL_REQUESTED`, `CALC_RUN.SEAL_APPROVED_STEP_1`, `CALC_RUN.SEAL_APPROVED_STEP_2`, `CALC_RUN.SEALED`

### Acceptance Criteria — APP-C-M8-004

```gherkin
Feature: Seal ECL calc run — 6-eyes workflow + step-up MFA

  Background:
    Given ecl.calc_run "CR-JUNI-2026-001" dengan status = "COMPLETED", error_count = 0
    And RISK-01 memiliki permission calc_run.seal_request
    And ALCO-01 dan ALCO-02 memiliki permission calc_run.seal_approve (ROLE-ALCO)
    And tidak ada SEALED calc_run untuk periode_id = "JUNI-2026"

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: 6-eyes seal flow lengkap
  # ---------------------------------------------------------------
  Scenario: Seal sukses — 6-eyes flow: RISK request → ALCO-01 approve → ALCO-02 approve
    When RISK-01 POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan:
      | body    | { "action": "REQUEST", "comment": "ECL JUNI-2026 siap di-seal setelah review Risk." } |
      | headers | Idempotency-Key: uuid-ccc-333                                                          |
    Then response 200 dengan:
      | data.status              | "PENDING_SEAL_APPROVAL_1" |
      | data.seal_requested_by   | RISK-01                   |
    And aud.audit_log berisi "CALC_RUN.SEAL_REQUESTED"
    And ALCO-01 menerima notifikasi: "Calc run CR-JUNI-2026-001 menunggu approval seal Anda."

    When ALCO-01 POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan:
      | body      | { "action": "APPROVE_STEP_1", "comment": "Reviewed — parameters consistent." } |
      | headers   | Idempotency-Key: uuid-ddd-444, X-Step-Up-Token: {valid_mfa_token_ALCO-01}       |
    Then MFA step-up ALCO-01 di-verify (re-prompt wajib meski mfa_verified=true)
    And response 200 dengan:
      | data.status              | "PENDING_SEAL_APPROVAL_2" |
      | data.seal_approved_by_1  | ALCO-01                   |
    And aud.audit_log berisi "CALC_RUN.SEAL_APPROVED_STEP_1" dengan signature_hash

    When ALCO-02 POST /api/v1/ecl/calc-runs/CR-JUNI-2026-001/seal dengan:
      | body      | { "action": "APPROVE_STEP_2", "comment": "Second approval confirmed." } |
      | headers   | Idempotency-Key: uuid-eee-555, X-Step-Up-Token: {valid_mfa_token_ALCO-02} |
    Then MFA step-up ALCO-02 di-verify
    And response 200 dengan:
      | data.status    | "SEALED"                |
      | data.sealed_at | IS NOT NULL             |
    And ecl.calc_run "CR-JUNI-2026-001":
      | status              | "SEALED"    |
      | sealed_at           | IS NOT NULL |
      | seal_approved_by_1  | ALCO-01     |
      | seal_approved_by_2  | ALCO-02     |
      | seal_chain_jsonb    | {full chain dengan signature hashes} |
    And aud.audit_log berisi "CALC_RUN.SEALED" dengan:
      | after_jsonb.seal_chain_jsonb | {sign-off chain} |
      | after_jsonb.sealed_at        | IS NOT NULL      |
    And DB trigger fn_ecl_calc_no_modify_when_sealed aktif (setiap UPDATE pada calc_result_line ditolak)
    And toast sukses di UI (ALCO-02): "Calc run CR-JUNI-2026-001 berhasil di-seal. Hasil ECL JUNI-2026 final dan immutable."

  # ---------------------------------------------------------------
  # Skenario 2 — SoD violation: maker tidak boleh jadi approver
  # ---------------------------------------------------------------
  Scenario: SoD violation — RISK-01 tidak boleh approve seal yang dia sendiri request
    Given RISK-01 sudah request seal untuk CR-JUNI-2026-001
    And RISK-01 memiliki permission calc_run.seal_approve (hipotesis)
    When RISK-01 POST seal dengan action = "APPROVE_STEP_1"
    Then response 403 dengan:
      | error.code    | "SOD_VIOLATION"                                                     |
      | error.message | "Maker (RISK-01) tidak boleh menjadi approver untuk seal yang sama." |
    And status calc_run tetap "PENDING_SEAL_APPROVAL_1"
    And aud.audit_log berisi event "CALC_RUN.SOD_VIOLATION_ATTEMPT"

  # ---------------------------------------------------------------
  # Skenario 3 — Step-up MFA gagal: approve ditolak
  # ---------------------------------------------------------------
  Scenario: Seal approve ditolak — step-up MFA token tidak valid
    Given ALCO-01 mengirim approve step-1 dengan X-Step-Up-Token expired/invalid
    When ALCO-01 POST seal APPROVE_STEP_1
    Then response 401 dengan:
      | error.code    | "MFA_STEP_UP_REQUIRED"                                         |
      | error.message | "Step-up MFA diperlukan untuk tindakan ini. Token tidak valid." |
    And status calc_run tetap "PENDING_SEAL_APPROVAL_1"
    And tidak ada perubahan di aud.audit_log (request ditolak sebelum business logic)

  # ---------------------------------------------------------------
  # Skenario 4 — Seal tidak bisa dilakukan pada COMPLETED_WITH_ERRORS
  # ---------------------------------------------------------------
  Scenario: Gagal seal — calc_run COMPLETED_WITH_ERRORS (ada instrumen error)
    Given ecl.calc_run "CR-JUNI-2026-ERR" dengan status = "COMPLETED_WITH_ERRORS", error_count = 3
    When RISK-01 POST seal request untuk CR-JUNI-2026-ERR
    Then response 422 dengan:
      | error.code    | "CALC_RUN_HAS_ERRORS"                                                               |
      | error.message | "Calc run CR-JUNI-2026-ERR memiliki 3 error instrumen. Perbaiki data dan re-run sebelum seal." |
    And status calc_run tetap "COMPLETED_WITH_ERRORS"

  # ---------------------------------------------------------------
  # Skenario 5 — Modifikasi setelah seal ditolak DB trigger
  # ---------------------------------------------------------------
  Scenario: Modifikasi pasca seal di-reject oleh DB trigger
    Given ecl.calc_run "CR-SEALED-001" dengan status = "SEALED"
    When internal process mencoba UPDATE ecl.calc_result_line dimana calc_run_id = "CR-SEALED-001"
    Then DB trigger fn_ecl_calc_no_modify_when_sealed raise exception "ECL_CALC_RUN_SEALED"
    And HTTP response untuk API call yang memicu UPDATE: 423 dengan error.code = "ECL_PARAM_FROZEN"

  # ---------------------------------------------------------------
  # Skenario 6 — CFO sebagai approver-2 (alternatif ALCO)
  # ---------------------------------------------------------------
  Scenario: CFO sebagai seal approver-2 — valid
    Given ALCO-01 sudah approve step-1
    And CFO-01 memiliki permission calc_run.seal_approve (ROLE-CFO)
    When CFO-01 POST seal APPROVE_STEP_2 dengan valid MFA step-up token
    Then response 200 dengan data.status = "SEALED"
    And ecl.calc_run.seal_approved_by_2 = CFO-01

  # ---------------------------------------------------------------
  # Skenario 7 — Idempotency: double approve step tidak create double seal
  # ---------------------------------------------------------------
  Scenario: Idempotent approve — Idempotency-Key sama dikirim dua kali untuk APPROVE_STEP_2
    Given ALCO-02 sudah berhasil approve step-2 dengan Idempotency-Key uuid-eee-555
    When ALCO-02 POST seal APPROVE_STEP_2 lagi dengan Idempotency-Key uuid-eee-555
    Then response 200 dengan body identik (IDEMPOTENCY_REPLAY)
    And calc_run sudah SEALED, tidak ada perubahan
    And tidak ada row duplikat di aud.audit_log untuk event ini

  # ---------------------------------------------------------------
  # OQ-M8-1 flag: 4-eyes vs 6-eyes — documented as open question
  # ---------------------------------------------------------------
  # NOTE: Story ini mengimplementasikan asumsi DEFAULT = 6-eyes.
  # Jika ifrs9-compliance-reviewer atau ALCO menentukan 4-eyes cukup,
  # APPROVE_STEP_2 di-skip dan ALCO-01 approval langsung trigger SEALED.
  # Perubahan ini perlu RFC (tidak boleh langsung override DEC-017).
  # OQ-M8-1 harus dijawab sebelum system-analyst menulis state machine.
```

---

## Story APP-C-M8-005 — Cancel Calc Run

**Actor**: ROLE-RISK (hanya maker = `created_by` dari calc_run tersebut)
**Trigger**: `POST /api/v1/ecl/calc-runs/{id}/cancel` — saat calc_run `DRAFT` atau `IN_PROGRESS`
**Goal**: Membatalkan calc_run. Jika `IN_PROGRESS`: kirim sinyal cancel ke Asynq job, worker berhenti setelah batch yang sedang berjalan selesai, rows `ecl.calc_result_line` yang sudah di-commit TIDAK di-rollback (partial result tetap ada untuk audit). Jika `DRAFT`: langsung cancel tanpa ada Asynq job.

**Pre-conditions**:
- `ecl.calc_run.status IN ('DRAFT', 'IN_PROGRESS')`
- User adalah `created_by` dari calc_run tersebut (maker-only rule)
- `cancel_reason` ≥ 30 karakter
- User memiliki permission `calc_run.cancel`

**Post-conditions**:
- `ecl.calc_run.status = 'CANCELLED'`
- `ecl.calc_run.cancelled_by`, `cancelled_at`, `cancel_reason` terisi
- Jika `IN_PROGRESS`: `sys.job.status = 'cancelled'` setelah worker stop
- Rows `ecl.calc_result_line` yang sudah di-commit tetap ada (partial, tidak di-delete)
- Audit event `CALC_RUN.CANCELLED` ditulis in-transaction

**Permissions**: `calc_run.cancel`
**Audit Events**: `CALC_RUN.CANCELLED`

### Acceptance Criteria — APP-C-M8-005

```gherkin
Feature: Cancel ECL calc run (DRAFT atau IN_PROGRESS)

  Background:
    Given RISK-01 adalah creator (created_by) dari ecl.calc_run "CR-JUNI-2026-001"
    And RISK-01 memiliki permission calc_run.cancel

  # ---------------------------------------------------------------
  # Skenario 1 — Cancel DRAFT: langsung cancelled
  # ---------------------------------------------------------------
  Scenario: Cancel calc_run DRAFT — langsung transisi ke CANCELLED
    Given ecl.calc_run "CR-DRAFT-001" dengan status = "DRAFT" dan created_by = RISK-01
    When RISK-01 POST /api/v1/ecl/calc-runs/CR-DRAFT-001/cancel dengan:
      | body    | { "cancel_reason": "Data master belum lengkap, akan dibuat ulang setelah koreksi." } |
      | headers | Idempotency-Key: uuid-fff-666                                                        |
    Then response 200 dengan:
      | data.status        | "CANCELLED"                                                          |
      | data.cancelled_by  | RISK-01                                                              |
      | data.cancel_reason | "Data master belum lengkap, akan dibuat ulang setelah koreksi."      |
    And ecl.calc_run.status = "CANCELLED", cancelled_at IS NOT NULL
    And aud.audit_log berisi "CALC_RUN.CANCELLED" dengan after_jsonb.cancel_reason
    And toast sukses di UI: "Calc run CR-DRAFT-001 berhasil dibatalkan."

  # ---------------------------------------------------------------
  # Skenario 2 — Cancel IN_PROGRESS: sinyal ke Asynq, partial rows tetap ada
  # ---------------------------------------------------------------
  Scenario: Cancel IN_PROGRESS — Asynq job dihentikan, partial result tetap ada
    Given ecl.calc_run "CR-JUNI-2026-001" status = "IN_PROGRESS", sudah memproses 400 instrumen
    And sys.job "JOB-ULID-001" status = "running"
    When RISK-01 POST cancel untuk CR-JUNI-2026-001 dengan cancel_reason yang valid (≥30 chars)
    Then Asynq job "JOB-ULID-001" menerima sinyal cancel (ctx.Done() fired)
    And worker berhenti setelah batch aktif selesai (≤ 100 instrumen tambahan)
    And ecl.calc_run.status = "CANCELLED"
    And sys.job.status = "cancelled"
    And 400 rows ecl.calc_result_line yang sudah di-commit TIDAK di-delete (partial result tetap)
    And sys.job.result_jsonb.total_computed = 400 (partial)
    And aud.audit_log berisi "CALC_RUN.CANCELLED" dengan after_jsonb.partial_count = 400
    And toast info: "Calc run dibatalkan. 400 instrumen sudah dihitung (partial, tidak digunakan untuk pelaporan resmi)."

  # ---------------------------------------------------------------
  # Skenario 3 — Error: cancel_reason terlalu pendek (< 30 chars)
  # ---------------------------------------------------------------
  Scenario: Gagal cancel — cancel_reason kurang dari 30 karakter
    When RISK-01 POST cancel dengan body { "cancel_reason": "Salah input" }
    Then response 400 dengan:
      | error.code            | "VALIDATION_FAILED"                                                       |
      | error.details[0].field | "cancel_reason"                                                          |
      | error.details[0].rule  | "min_length:30"                                                          |
    And calc_run status tidak berubah

  # ---------------------------------------------------------------
  # Skenario 4 — Error: bukan maker (SoD-style restriction)
  # ---------------------------------------------------------------
  Scenario: Gagal cancel — bukan maker dari calc_run ini
    Given RISK-02 (bukan creator dari CR-JUNI-2026-001)
    When RISK-02 POST cancel untuk CR-JUNI-2026-001
    Then response 403 dengan:
      | error.code    | "FORBIDDEN"                                                              |
      | error.message | "Hanya creator calc run (RISK-01) yang dapat membatalkannya."            |
    And calc_run status tidak berubah

  # ---------------------------------------------------------------
  # Skenario 5 — Error: tidak bisa cancel COMPLETED
  # ---------------------------------------------------------------
  Scenario: Gagal cancel — calc_run sudah COMPLETED
    Given ecl.calc_run "CR-JUNI-2026-001" dengan status = "COMPLETED"
    When RISK-01 POST cancel
    Then response 422 dengan:
      | error.code    | "CALC_RUN_INVALID_TRANSITION"                                                |
      | error.message | "Calc run CR-JUNI-2026-001 tidak bisa dibatalkan: status COMPLETED. Hanya DRAFT dan IN_PROGRESS yang bisa di-cancel." |

  # ---------------------------------------------------------------
  # Skenario 6 — Error: tidak bisa cancel SEALED
  # ---------------------------------------------------------------
  Scenario: Gagal cancel — calc_run sudah SEALED
    Given ecl.calc_run "CR-SEALED-001" dengan status = "SEALED"
    When RISK-01 POST cancel
    Then response 422 dengan error.code = "CALC_RUN_INVALID_TRANSITION"
    And error.message menyebutkan "SEALED calc run tidak bisa dibatalkan"

  # ---------------------------------------------------------------
  # Skenario 7 — Idempotency: double cancel
  # ---------------------------------------------------------------
  Scenario: Idempotent cancel — Idempotency-Key sama dikirim dua kali
    Given RISK-01 sudah berhasil cancel CR-DRAFT-001 dengan Idempotency-Key uuid-fff-666
    When RISK-01 POST cancel lagi dengan Idempotency-Key uuid-fff-666
    Then response 200 dengan body identik (IDEMPOTENCY_REPLAY)
    And tidak ada perubahan state tambahan
```

---

## Story APP-C-M8-006 — Re-run: Multiple Unsealed Runs per Periode + Block After Seal

**Actor**: ROLE-RISK
**Trigger**: ROLE-RISK membuat calc_run baru untuk `periode_id` yang sudah memiliki satu atau lebih calc_run sebelumnya (CANCELLED, COMPLETED, DRAFT)
**Goal**: Membolehkan pembuatan beberapa calc_run untuk periode yang sama selama belum ada yang SEALED (misal: setelah koreksi data master, parameter update, atau cancel mid-run). Setelah satu calc_run SEALED untuk periode tersebut, pembuatan calc_run baru di-block (tidak otomatis — perlu ALCO override yang di-defer ke backlog, per OQ-M8-2). Calc_run yang lama tidak otomatis di-supersede — multiple COMPLETED/DRAFT runs boleh exist secara bersamaan, tapi hanya satu yang dapat di-seal per periode.

**Pre-conditions** (untuk "multiple unsealed OK"):
- Tidak ada `ecl.calc_run` dengan `status = 'SEALED'` untuk `periode_id` yang sama
- Boleh ada multiple `DRAFT`, `IN_PROGRESS`, `COMPLETED`, `CANCELLED`, `COMPLETED_WITH_ERRORS` untuk periode yang sama
- Hanya satu `IN_PROGRESS` yang diizinkan pada satu waktu (constraint dari Story 1)

**Pre-conditions** (untuk "block after seal"):
- Ada `ecl.calc_run` dengan `status = 'SEALED'` untuk `periode_id` yang sama
- Override belum tersedia (defer ke backlog)

**Post-conditions** (multiple unsealed):
- Baris baru `ecl.calc_run` DRAFT dibuat
- Run lama tidak otomatis di-supersede atau soft-deleted

**Post-conditions** (blocked after seal):
- Error `CALC_RUN_PERIODE_ALREADY_SEALED` dikembalikan, tidak ada row baru

**Permissions**: `calc_run.create`
**Audit Events**: `CALC_RUN.CREATED` (normal), tidak ada event khusus untuk "multiple run" scenario

### Acceptance Criteria — APP-C-M8-006

```gherkin
Feature: Multiple calc_run per periode — unsealed OK, blocked after seal

  Background:
    Given RISK-01 memiliki permission calc_run.create
    And mst.periode_buku "JUNI-2026" dengan status "OPEN"

  # ---------------------------------------------------------------
  # Skenario 1 — Multiple COMPLETED runs untuk periode yang sama: OK
  # ---------------------------------------------------------------
  Scenario: Create calc_run kedua untuk periode yang sudah ada COMPLETED run (belum sealed)
    Given ecl.calc_run "CR-JUNI-2026-001" dengan status = "COMPLETED" untuk "JUNI-2026"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026" (evaluation_date berbeda: "2026-06-29")
    Then response 201 dengan data.status = "DRAFT" (calc_run baru "CR-JUNI-2026-002")
    And ecl.calc_run "CR-JUNI-2026-001" status tetap "COMPLETED" (tidak di-supersede otomatis)
    And ecl.calc_run "CR-JUNI-2026-002" DRAFT baru dibuat
    And GET /api/v1/ecl/calc-runs?periode_id=JUNI-2026 mengembalikan kedua run

  # ---------------------------------------------------------------
  # Skenario 2 — Multiple CANCELLED runs OK + re-run setelah cancel
  # ---------------------------------------------------------------
  Scenario: Re-run setelah cancel — create baru sukses
    Given ecl.calc_run "CR-JUNI-2026-001" status = "CANCELLED" (dibatalkan mid-run)
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026"
    Then response 201 dengan data.status = "DRAFT" (calc_run baru "CR-JUNI-2026-002")
    And "CR-JUNI-2026-001" status tetap "CANCELLED" (tidak di-hapus)
    And aud.audit_log "CR-JUNI-2026-001" masih ada (immutable history)
    And toast sukses: "Calc run baru untuk periode JUNI-2026 berhasil dibuat (menggantikan CR-JUNI-2026-001 yang dibatalkan)."

  # ---------------------------------------------------------------
  # Skenario 3 — Block setelah seal: tidak bisa create run baru
  # ---------------------------------------------------------------
  Scenario: Blocked create — sudah ada SEALED run untuk periode
    Given ecl.calc_run "CR-JUNI-2026-001" dengan status = "SEALED" untuk "JUNI-2026"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026"
    Then response 422 dengan:
      | error.code    | "CALC_RUN_PERIODE_ALREADY_SEALED"                                                                             |
      | error.message | "Periode JUNI-2026 sudah memiliki calc run yang di-seal (CR-JUNI-2026-001, sealed_at: ...). Override memerlukan persetujuan ALCO — fitur ini belum tersedia. Hubungi tech-lead-orchestrator untuk membuka RFC." |
    And tidak ada row baru di ecl.calc_run

  # ---------------------------------------------------------------
  # Skenario 4 — List all runs per periode: filter + sort + cursor (UX §1)
  # ---------------------------------------------------------------
  Scenario: List semua calc_run untuk periode — DataTable UX §1
    Given 3 calc_run untuk "JUNI-2026": CANCELLED, COMPLETED, SEALED
    When RISK-01 GET /api/v1/ecl/calc-runs?filter[periode_id]=JUNI-2026&sort=created_at:desc&limit=50
    Then response 200 dengan:
      | data       | array 3 calc_run                                             |
      | appliedSort | [{col: "created_at", dir: "desc"}]                          |
      | appliedFilter | {periode_id: "JUNI-2026"}                                 |
      | pagination | {nextCursor: null, hasMore: false, totalEstimate: 3}        |
    And UI DataTable menampilkan sort header, filter chips, status badge per row

  # ---------------------------------------------------------------
  # Skenario 5 — Hanya satu IN_PROGRESS per periode: re-create ditolak
  # ---------------------------------------------------------------
  Scenario: Gagal create — sudah ada IN_PROGRESS untuk periode yang sama
    Given ecl.calc_run "CR-JUNI-2026-002" dengan status = "IN_PROGRESS" untuk "JUNI-2026"
    When RISK-01 POST /api/v1/ecl/calc-runs dengan periode_id = "JUNI-2026"
    Then response 422 dengan error.code = "CALC_RUN_IN_PROGRESS_EXISTS"
    And message menyebutkan "CR-JUNI-2026-002 sedang berjalan — tunggu atau cancel dulu"

  # ---------------------------------------------------------------
  # Skenario 6 — Audit trail: semua run untuk periode visible ke ROLE-AUDIT
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT dapat melihat semua run history per periode termasuk CANCELLED
    Given AUDIT-01 dengan role ROLE-AUDIT
    When AUDIT-01 GET /api/v1/ecl/calc-runs?filter[periode_id]=JUNI-2026&include_cancelled=true
    Then response berisi semua calc_run termasuk CANCELLED (soft-deleted rows tidak termasuk — none expected)
    And aud.audit_log untuk setiap run dapat di-query via audit browser
    And tidak ada tombol aksi mutasi di UI (read-only)

  # ---------------------------------------------------------------
  # Nota: ALCO override untuk re-run setelah seal — DEFERRED ke backlog
  # ---------------------------------------------------------------
  # OQ-M8-2: Fitur "ALCO override untuk create calc_run baru pada periode yang sudah sealed"
  # TIDAK di-implementasi di M8. Behavior saat ini: HARD BLOCK (422 CALC_RUN_PERIODE_ALREADY_SEALED).
  # Jika bisnis butuh recompute, alur yang benar adalah:
  #   1. ROLE-RISK meminta RFC ke tech-lead-orchestrator
  #   2. ALCO mengeluarkan override decision dengan dokumentasi alasan
  #   3. Fitur override di-build di modul terpisah (post-Phase 4)
  # Dokumentasi ini ada untuk SOP tim, bukan implementasi saat ini.
```

---

## State Machine Ringkasan

```
                    ┌──────────────────────────────────────────────────────────┐
                    │                   ecl.calc_run states                    │
                    └──────────────────────────────────────────────────────────┘

  [create]          [start]            [worker done]         [seal flow]
  DRAFT ──────────► IN_PROGRESS ──────► COMPLETED ──────────► SEALED (terminal)
    │                   │                   │
    │                   │                   └──► COMPLETED_WITH_ERRORS ─► SEALED (setelah error fix + re-run)
    │                   │
    └──[cancel]──────── └──[cancel]──────► CANCELLED (terminal)
```

**Valid transitions**:
| From | To | Trigger | Actor |
|---|---|---|---|
| `DRAFT` | `IN_PROGRESS` | `/start` | ROLE-RISK |
| `DRAFT` | `CANCELLED` | `/cancel` | ROLE-RISK (maker only) |
| `IN_PROGRESS` | `COMPLETED` | Worker selesai, `error_count = 0` | Engine internal |
| `IN_PROGRESS` | `COMPLETED_WITH_ERRORS` | Worker selesai, `error_count > 0` | Engine internal |
| `IN_PROGRESS` | `CANCELLED` | `/cancel` | ROLE-RISK (maker only) |
| `COMPLETED` | `SEALED` | Seal flow 6-eyes selesai | RISK + ALCO + ALCO/CFO |
| `COMPLETED_WITH_ERRORS` | `SEALED` | **Dilarang** — harus re-run dengan data clean terlebih dahulu | — |
| `SEALED` | any | **Tidak ada** — terminal state | — |
| `CANCELLED` | any | **Tidak ada** — terminal state (buat run baru jika perlu) | — |

---

## Handoff Checklist

- [ ] **system-analyst** — OpenAPI fragment: 5 endpoints + state machine + Go interface `CalcRunService`
- [ ] **data-modeler** — migration 000031: `ecl.calc_run`, FK resolution OQ-M7-2/M8-4, seal workflow cols, trigger `fn_ecl_calc_run_no_modify_when_sealed`
- [ ] **ecl-eir-engineer** — interface `CalcRunOrchestrator` yang memanggil `ECLEngine.BulkCompute()` dengan progress callback
- [ ] **backend-engineer-go** — HTTP handler, Gin routing, Asynq worker plumbing, middleware: idempotency + MFA step-up check
- [ ] **security-engineer** — **BLOCKING** review: step-up MFA, audit in-transaction, SoD enforcement, `ECL_PARAM_FROZEN` guard
- [ ] **ifrs9-compliance-reviewer** — **BLOCKING** gate: seal = irreversible (DEC-018), parameter freeze completeness (OQ-M8-5), 6-eyes atau 4-eyes (OQ-M8-1)
- [ ] **Jawab OQ-M8-1..5** sebelum system-analyst menulis state machine

---

_Dibuat oleh `business-analyst` pada 2026-06-12. Menunggu OQ-M8-1 jawaban dari ifrs9-compliance-reviewer + ALCO sebelum state machine di-finalize._
