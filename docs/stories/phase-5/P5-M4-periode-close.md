# P5-M4 — APP-D Periode Buku Close Workflow: User Stories

**Story Set ID**: P5-M4
**Modul**: APP-D — Periode Buku (Phase 5, Sprint 2)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `security-engineer` (BLOCKING gate) + `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-17
**Linked FSD**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx §1 (Periode Buku Close Workflow)
**Linked BRD**: BRD §6.3 (APP-D Periode Buku), RACI: ROLE-CFO (A), ROLE-AKUN-CTL (R), ROLE-AKUN (C), ROLE-RISK (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-017` (LOCKED) — 4-eyes SoD wajib; 6-eyes untuk parameter master. Soft-close adalah 4-eyes: ROLE-AKUN-CTL sebagai maker + approver berbeda (SoD). Hard-close: ROLE-AKUN-CTL request + ROLE-CFO approve.
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun
- `DEC-021` (LOCKED) — Idempotency-Key wajib di setiap mutating endpoint
- `DEC-022` (LOCKED) — cursor-based pagination
- `DEC-026` (LOCKED) — MFA mandatory untuk ROLE-CFO
- `DEC-027` (LOCKED) — step-up MFA wajib untuk hard-close periode buku (X-Step-Up-Token header)
- `DEC-036` (RESOLVED) — cure evaluation berjalan di SOFT_CLOSED jika di-trigger manual ROLE-RISK; jurnal masuk sebagai CORRECTION_PERIODE_CLOSED

**Dependensi**:
- **P5-M2** (jurnal engine) — menyediakan `jrnl.header` rows; jurnal balance check (`total_debit == total_kredit`) wajib pass sebelum close
- **P5-M3** (GL delivery) — menyediakan `jrnl.gl_status`; pre-condition: tidak boleh ada `gl_host_status = 'FAILED'` yang belum resolved untuk periode yang ditutup
- **P5-M13** (reporting MV) — P5-M4 hard-close men-trigger refresh 8 materialized views `rpt.mv_*` via Asynq job

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment: 7 endpoints (soft-close-request, soft-close-approve, hard-close-request, hard-close-approve, reopen-request, reopen-approve, closing-checklist) + `GET /reports/status-periode`; state machine `mst.periode_buku.status_periode` (OPEN → SOFT_CLOSED → HARD_CLOSE_PENDING → CLOSED); error codes baru (lihat §Error Codes Proposed)
- `data-modeler` → migration 000038 (atau next sequence): (a) ADD COLUMNS ke `mst.periode_buku` untuk close workflow tracking; (b) CREATE TABLE `sys.closing_checklist_snapshot` untuk audit-replay snapshot setiap transisi; (c) ADD column `status_periode` CHECK constraint upgrade untuk HARD_CLOSE_PENDING state
- `security-engineer` → BLOCKING gate: step-up MFA enforcement di `/hard-close-approve`; `PERIODE.HARDCLOSE` audit in-transaction; soft-delete guard pada `mst.periode_buku` → verify audit append-only; step-up MFA token validation kontrol di middleware
- `ifrs9-compliance-reviewer` → BLOCKING gate: pre-condition checklist completeness (0 PENDING_APPROVAL, jurnal balanced, recon pass, GL delivered); CFO hard-close irreversibility setelah grace window; cure evaluation allowance saat SOFT_CLOSED

**Compliance path**: P5-M4 adalah **regulated path** karena menyentuh jurnal integrity (balance check sebagai pre-condition) dan step-up MFA CFO (DEC-027). `ifrs9-compliance-reviewer` BLOCKING untuk hard-close gate. `security-engineer` BLOCKING untuk MFA dan audit trail.

---

## Konteks & Arsitektur P5-M4

### State Machine Periode Buku

```
OPEN
  │
  ├─ [soft-close-request] ROLE-AKUN-CTL Maker
  │    Pre-conditions: 0 PENDING_APPROVAL, jurnal balanced, recon pass, GL delivered
  │    → status_periode = OPEN (belum berubah)
  │    → soft_close_request_id di-INSERT di sys.closing_checklist_snapshot
  │
  ├─ [soft-close-approve] ROLE-AKUN-CTL Approver (SoD: approver_id ≠ requester_id)
  │    → status_periode = SOFT_CLOSED
  │    → tanggal_soft_close = now()
  │    → snapshot persisted
  │    → mutation BLIPS untuk instrumen/transaksi/jurnal: BLOCKED (kecuali CORRECTION_PERIODE_CLOSED)
  │
SOFT_CLOSED
  │
  ├─ [hard-close-request] ROLE-AKUN-CTL (re-run checklist)
  │    → status_periode = HARD_CLOSE_PENDING
  │
  ├─ [hard-close-approve] ROLE-CFO (step-up MFA mandatory, DEC-027)
  │    → status_periode = CLOSED
  │    → tanggal_hard_close = now()
  │    → snapshot persisted
  │    → Asynq job: trigger MV refresh (P5-M13)
  │    → FX rate `locked_flag = TRUE` untuk periode
  │    → Reopen hanya dalam grace window (default 48 jam — konfigurabel)
  │
  ├─ [reopen-request + reopen-approve] ROLE-CFO (MFA mandatory)
  │    Hanya dari SOFT_CLOSED → OPEN (atau dari CLOSED → SOFT_CLOSED dalam grace window)
  │    Setelah grace window: PERIODE_GRACE_EXPIRED (423), tidak bisa reopen ke OPEN
  │
HARD_CLOSE_PENDING
  │
  └─ [hard-close-approve] → CLOSED
       atau
     [revert/reject oleh CFO] → SOFT_CLOSED (batalkan permintaan)

CLOSED   [terminal — no further mutation di transaksi/jurnal/instrumen]
```

### Cross-Cutting Enforcement: PERIODE_CLOSED

Setelah `status_periode = CLOSED`, semua endpoint mutating yang terkait periode ini wajib mengembalikan:
```
HTTP 423 Locked
{ "error": { "code": "PERIODE_CLOSED", "message": "Periode {kode} sudah hard-closed pada {tanggal}. Mutasi tidak diizinkan." } }
```
Enforcement dilakukan di middleware layer (`PeriodeLockMiddleware`) yang dicek sebelum setiap handler mutating di `trx.*`, `jrnl.*`, `ecl.*` yang memiliki `periode_id`.

### Schema Referensi P5-M4

#### `mst.periode_buku` (existing — migration 000001 + 000009)
| Kolom | Tipe | Status |
|---|---|---|
| `id` | UUID PK | existing |
| `periode_id_kode` | VARCHAR(20) UNIQUE | existing |
| `status_periode` | VARCHAR(20) `OPEN|SOFT_CLOSED|CLOSED` | existing — CHECK perlu di-upgrade untuk HARD_CLOSE_PENDING |
| `tanggal_soft_close` | TIMESTAMPTZ | existing |
| `tanggal_hard_close` | TIMESTAMPTZ | existing |
| `user_closer_id` | UUID FK | existing |
| `user_approver_close_id` | UUID FK | existing |
| `reopened_flag` | BOOLEAN | existing |
| `reopened_reason` | TEXT | existing |
| `reopened_at` | TIMESTAMPTZ | existing |
| `reopened_by` | UUID FK | existing |
| `reopened_approved_by` | UUID FK | existing |
| `row_version` | BIGINT | existing (migration 000009) |

**Kolom tambahan yang dibutuhkan P5-M4** (migration 000038 — data-modeler):
| Kolom | Tipe | Keterangan |
|---|---|---|
| `soft_close_requested_by` | UUID FK | User yang submit soft-close request |
| `soft_close_requested_at` | TIMESTAMPTZ | Timestamp request |
| `soft_close_approved_by` | UUID FK | User yang approve (SoD: ≠ requested_by) |
| `soft_close_approved_at` | TIMESTAMPTZ | |
| `hard_close_requested_by` | UUID FK | User yang submit hard-close request |
| `hard_close_requested_at` | TIMESTAMPTZ | |
| `hard_close_approved_by` | UUID FK | CFO yang approve (step-up MFA) |
| `hard_close_approved_at` | TIMESTAMPTZ | |
| `hard_close_grace_expires_at` | TIMESTAMPTZ | Grace window expiry untuk reopen |
| `reopen_reason` | TEXT | Alasan reopen (wajib ≥ 30 char) |
| `step_up_token_ref` | VARCHAR(100) | Referensi step-up MFA token untuk hard-close (audit trace) |

#### `sys.closing_checklist_snapshot` (baru — migration 000038)
Menyimpan snapshot hasil closing-checklist pada setiap transisi state untuk keperluan audit replay.

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK DEFAULT gen_random_uuid() | |
| `periode_id` | UUID NOT NULL FK `mst.periode_buku(id)` | |
| `transition` | VARCHAR(30) NOT NULL | `SOFT_CLOSE_REQUEST`, `SOFT_CLOSE_APPROVE`, `HARD_CLOSE_REQUEST`, `HARD_CLOSE_APPROVE`, `REOPEN_REQUEST`, `REOPEN_APPROVE` |
| `actor_user_id` | UUID NOT NULL | |
| `actor_role` | TEXT NOT NULL | |
| `checklist_jsonb` | JSONB NOT NULL | Snapshot 4-item checklist: hasil pass/fail tiap item pada saat transisi |
| `all_passed` | BOOLEAN NOT NULL | `TRUE` jika semua item pass |
| `transition_status` | VARCHAR(20) NOT NULL | `APPROVED` atau `REJECTED` (rejection sebab checklist gagal) |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `tenant_id` | TEXT NOT NULL DEFAULT 'TUGURE' | |

Tabel ini **append-only** — tidak ada UPDATE atau DELETE. Diproteksi via trigger `BEFORE DELETE → RAISE EXCEPTION`.

**Format `checklist_jsonb`:**
```json
{
  "evaluated_at": "2026-06-30T17:00:00+07:00",
  "items": [
    {
      "key":     "PENDING_APPROVAL_ZERO",
      "label":   "0 transaksi/jurnal masih PENDING_APPROVAL",
      "passed":  true,
      "detail":  "Total PENDING_APPROVAL: 0"
    },
    {
      "key":     "JURNAL_BALANCED",
      "label":   "Semua jurnal seimbang (total_debit == total_kredit, delta ≤ IDR 0.01)",
      "passed":  true,
      "detail":  "Jurnal checked: 248. Max delta: IDR 0.0000"
    },
    {
      "key":     "GL_DELIVERED",
      "label":   "Tidak ada gl_host_status = FAILED yang belum diselesaikan",
      "passed":  false,
      "detail":  "3 jurnal masih FAILED. Header IDs: [uuid1, uuid2, uuid3]"
    },
    {
      "key":     "RECON_PASS",
      "label":   "GL rekonsiliasi harian terakhir COMPLETED (bukan COMPLETED_WITH_MISMATCH atau FAILED)",
      "passed":  true,
      "detail":  "Last recon: 2026-06-29 — COMPLETED. Delta IDR 0.0000"
    }
  ]
}
```

---

## Story P5-M4-S1 — Soft-Close Request dengan Pre-Condition Snapshot

**Actor**: ROLE-AKUN-CTL (Maker — user yang mengajukan request)
**Trigger**: Akhir periode buku, ROLE-AKUN-CTL membuka halaman `/periode-buku/{id}` dan mengklik "Ajukan Soft Close"
**Goal**: ROLE-AKUN-CTL dapat mengajukan permintaan soft-close untuk periode yang `status_periode = OPEN`. Sistem menjalankan 4-item closing checklist secara real-time, menolak request jika ada item yang gagal, dan mempersist snapshot checklist ke `sys.closing_checklist_snapshot`. Jika checklist lolos, status internal berubah ke `SOFT_CLOSE_PENDING` (internal state menunggu approval — `status_periode` masih OPEN hingga approve).

### Pre-conditions
1. User ter-autentikasi dengan permission `periode.softclose.request`
2. `mst.periode_buku.status_periode = 'OPEN'`
3. Request mengandung `Idempotency-Key` header (UUID v4)
4. Tidak ada soft-close request yang masih PENDING untuk periode yang sama (optimistic lock via `row_version`)

### 4-Item Closing Checklist
| Key | Pengecekan | Sumber data |
|---|---|---|
| `PENDING_APPROVAL_ZERO` | COUNT(*) dari semua entity (`trx.*`, `jrnl.header`) dengan `workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')` DAN `periode_id = {id}` = 0 | `trx.*`, `jrnl.header` |
| `JURNAL_BALANCED` | Semua `jrnl.header` dalam periode: `ABS(total_debit - total_kredit) ≤ 0.01` untuk setiap header | `jrnl.header` |
| `GL_DELIVERED` | Tidak ada `jrnl.gl_status.gl_host_status IN ('FAILED')` untuk jurnal dalam periode (DEAD_LETTER dikecualikan karena sudah di-discard secara explicit) | `jrnl.gl_status JOIN jrnl.header` |
| `RECON_PASS` | `sys.gl_reconciliation_report` terakhir untuk periode ini: `status IN ('COMPLETED')` — COMPLETED_WITH_MISMATCH tidak cukup | `sys.gl_reconciliation_report` |

### Endpoint
```
POST /api/v1/periode-buku/{id}/soft-close-request
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "catatan": "Soft close request periode Juni 2026. Semua transaksi telah diverifikasi."
}

→ 202 Accepted (checklist lolos)
{
  "data": {
    "periode_id": "<uuid>",
    "periode_kode": "PRD-2026-06",
    "transition": "SOFT_CLOSE_REQUEST",
    "checklist_snapshot_id": "<uuid>",
    "checklist": { ...4 items... },
    "all_passed": true,
    "next_step": "Menunggu approval ROLE-AKUN-CTL lain via /soft-close-approve"
  }
}

→ 422 Unprocessable Entity (checklist gagal)
{
  "error": {
    "code": "CLOSING_CHECKLIST_FAILED",
    "message": "Soft-close request ditolak: 2 item checklist tidak lolos.",
    "details": [ { "field": "GL_DELIVERED", "rule": "3 jurnal masih FAILED di GL delivery" } ]
  }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.periode_buku` | READ + UPDATE `row_version` | Optimistic lock check |
| `trx.*` (semua entity) | READ | PENDING_APPROVAL_ZERO check |
| `jrnl.header` | READ | JURNAL_BALANCED check + PENDING_APPROVAL check |
| `jrnl.gl_status` | READ | GL_DELIVERED check |
| `sys.gl_reconciliation_report` | READ | RECON_PASS check |
| `sys.closing_checklist_snapshot` | INSERT | Snapshot persisted in-transaction dengan response |
| `aud.audit_log` | INSERT | `PERIODE.SOFT_CLOSE_REQUESTED` — in-transaction |

### Permissions
| Permission | Role | MFA | Catatan |
|---|---|---|---|
| `periode.softclose.request` | ROLE-AKUN-CTL | Tidak | |

### Audit Events
| Action | Trigger |
|---|---|
| `PERIODE.SOFT_CLOSE_REQUESTED` | Request diterima, checklist lolos, snapshot persisted — in-transaction |
| `PERIODE.SOFT_CLOSE_REQUEST_REJECTED` | Checklist gagal — sebelum reject response (advisory log) |

### Acceptance Criteria

```gherkin
Feature: Soft-close request periode buku oleh ROLE-AKUN-CTL

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'OPEN', tanggal_akhir = 2026-06-30
    And user ROLE-AKUN-CTL (USR-011) ter-autentikasi dengan permission periode.softclose.request
    And 248 jrnl.header rows POSTED untuk PRD-2026-06
    And row_version PRD-2026-06 = 5

  # ─── HAPPY PATH: Semua checklist lolos — request diterima ───────────────────

  Scenario: S1-AC1 — ROLE-AKUN-CTL berhasil mengajukan soft-close request
    Given PENDING_APPROVAL_ZERO: 0 entity PENDING untuk PRD-2026-06
    And JURNAL_BALANCED: semua 248 jrnl.header balance (max delta IDR 0.0000)
    And GL_DELIVERED: tidak ada jrnl.gl_status FAILED untuk PRD-2026-06
    And RECON_PASS: sys.gl_reconciliation_report terakhir status = 'COMPLETED' untuk 2026-06-29
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-request
      With Idempotency-Key: IK-SC-REQ-001
      With body: { "catatan": "Soft close request periode Juni 2026. Semua transaksi telah diverifikasi." }
    Then HTTP 202
    And dalam satu transaksi DB:
      | sys.closing_checklist_snapshot.transition          | SOFT_CLOSE_REQUEST       |
      | sys.closing_checklist_snapshot.all_passed           | true                     |
      | sys.closing_checklist_snapshot.actor_user_id        | USR-011 UUID             |
      | sys.closing_checklist_snapshot.checklist_jsonb       | 4 items semua passed=true|
      | aud.audit_log.action                                | PERIODE.SOFT_CLOSE_REQUESTED |
      | mst.periode_buku.soft_close_requested_by            | USR-011 UUID             |
      | mst.periode_buku.soft_close_requested_at            | timestamp now            |
      | mst.periode_buku.row_version                        | 6                        |
    And response.data.all_passed = true
    And response.data.next_step = "Menunggu approval ROLE-AKUN-CTL lain via /soft-close-approve"
    And toast ke USR-011: "Soft-close request PRD-2026-06 berhasil diajukan. Menunggu approval dari Finance Controller lain."

  # ─── ERROR: Checklist gagal — GL_DELIVERED tidak lolos ──────────────────────

  Scenario: S1-AC2 — Request ditolak karena ada jurnal FAILED di GL delivery
    Given 3 jrnl.gl_status rows: gl_host_status = 'FAILED' untuk header dalam PRD-2026-06
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-request
      With Idempotency-Key: IK-SC-REQ-002
    Then HTTP 422:
      | error.code              | CLOSING_CHECKLIST_FAILED          |
      | error.details[0].field  | GL_DELIVERED                      |
      | error.details[0].rule   | "3 jurnal masih FAILED di GL delivery. Selesaikan via DLQ GL Delivery terlebih dahulu." |
    And sys.closing_checklist_snapshot ter-INSERT dengan:
      | all_passed         | false                              |
      | transition_status  | REJECTED                           |
      | checklist_jsonb[GL_DELIVERED].passed | false              |
    And aud.audit_log.action = PERIODE.SOFT_CLOSE_REQUEST_REJECTED (advisory)
    And mst.periode_buku.status_periode tetap 'OPEN'

  # ─── ERROR: Periode tidak OPEN ───────────────────────────────────────────────

  Scenario: S1-AC3 — Request ditolak karena periode sudah SOFT_CLOSED
    Given mst.periode_buku PRD-2026-06: status_periode = 'SOFT_CLOSED'
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-request
      With Idempotency-Key: IK-SC-REQ-003
    Then HTTP 422:
      | error.code    | WORKFLOW_INVALID_TRANSITION         |
      | error.message | "Periode PRD-2026-06 sudah dalam status SOFT_CLOSED. Soft-close request tidak dapat diajukan." |
    And tidak ada INSERT ke sys.closing_checklist_snapshot
    And tidak ada perubahan state

  # ─── ERROR: Optimistic lock — concurrent request ─────────────────────────────

  Scenario: S1-AC4 — Request kedua ditolak karena row_version mismatch (concurrent close attempt)
    Given USR-011 membaca mst.periode_buku row_version = 5
    And USR-012 (ROLE-AKUN-CTL lain) sudah submit soft-close-request lebih dahulu, row_version naik jadi 6
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-request
      With body yang mengandung row_version = 5 (stale)
    Then HTTP 409:
      | error.code    | CONFLICT                            |
      | error.message | "Data periode PRD-2026-06 sudah diubah oleh pengguna lain. Refresh dan coba lagi." |
    And tidak ada INSERT ke sys.closing_checklist_snapshot
    And tidak ada perubahan state
```

### Open Questions — Story 1
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M4-1a | Apakah `row_version` dikirim dari client di request body, atau server cukup check berdasarkan single-actor constraint (hanya 1 soft-close-request PENDING per periode)? | **Keduanya**: client wajib kirim `row_version` di body; server juga enforce single-pending-request constraint. Double guard terbaik untuk concurrent safety. system-analyst konfirmasi. |
| OQ-M4-1b | Apakah `COMPLETED_WITH_MISMATCH` diizinkan untuk item RECON_PASS, atau harus strict `COMPLETED`? | **Strict `COMPLETED`** sesuai instruksi roadmap. COMPLETED_WITH_MISMATCH membutuhkan eskalasi ROLE-AKUN-CTL + ROLE-CFO sebelum close. Konfirmasi ke Kepala Akuntansi Tugure. |
| OQ-M4-1c | Apa grace period yang diizinkan antara soft-close-request dan soft-close-approve sebelum checklist dianggap stale? | **24 jam** — jika approve melebihi 24 jam dari request, sistem re-run checklist saat approve. Konfigurabel via `sys.config_param` `SOFT_CLOSE_CHECKLIST_STALE_HOURS`. |

---

## Story P5-M4-S2 — Soft-Close Approve (4-Eyes, SoD, OPEN → SOFT_CLOSED)

**Actor**: ROLE-AKUN-CTL (Approver — user berbeda dari requester, SoD enforced)
**Trigger**: ROLE-AKUN-CTL Approver menerima notifikasi in-app "Ada soft-close request untuk PRD-2026-06 menunggu approval Anda" dan mengklik "Approve" di halaman `/periode-buku/{id}`
**Goal**: Approver (ROLE-AKUN-CTL berbeda dari requester) men-approve soft-close request. Sistem re-verifikasi checklist (karena kondisi bisa berubah sejak request), meng-enforce SoD (`approver_id ≠ requester_id`), mengubah `status_periode` menjadi `SOFT_CLOSED`, memblok semua mutasi non-koreksi untuk periode, dan mempersist snapshot approval ke `sys.closing_checklist_snapshot`. Notifikasi dikirim ke ROLE-CFO bahwa periode siap untuk hard-close.

### Pre-conditions
1. User ter-autentikasi dengan permission `periode.softclose.approve`
2. `mst.periode_buku.soft_close_requested_by` IS NOT NULL (ada pending request)
3. `approver_id ≠ soft_close_requested_by` (SoD — enforced server-side, bukan hanya UI)
4. `mst.periode_buku.status_periode = 'OPEN'` (belum SOFT_CLOSED atau CLOSED)
5. Request mengandung `Idempotency-Key` header
6. Jika waktu sejak request > `SOFT_CLOSE_CHECKLIST_STALE_HOURS` (default 24 jam) → re-run checklist; jika gagal → reject dengan `CLOSING_CHECKLIST_STALE`

### Endpoint
```
POST /api/v1/periode-buku/{id}/soft-close-approve
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "comment": "Approved. Semua posisi telah diverifikasi oleh Finance Controller.",
  "signature_method": "JWT_STEP_UP"
}

→ 200 OK
{
  "data": {
    "periode_id": "<uuid>",
    "periode_kode": "PRD-2026-06",
    "status_periode": "SOFT_CLOSED",
    "tanggal_soft_close": "2026-06-30T17:05:22+07:00",
    "approved_by": "USR-012",
    "checklist_snapshot_id": "<uuid>",
    "message": "Periode PRD-2026-06 berhasil di-soft-close. Mutasi transaksi/jurnal diblokir. Siap untuk hard-close oleh CFO."
  }
}
```

### Mutation Lock Post-Approval

Setelah `status_periode = 'SOFT_CLOSED'`, middleware `PeriodeLockMiddleware` memblok semua endpoint mutating yang memiliki `periode_id = {id}` KECUALI:
- `POST /api/v1/jurnal/header/{id}/retry-gl-delivery` (GL delivery masih diperbolehkan)
- Jurnal event code `CORRECTION_PERIODE_CLOSED` (via dedicated endpoint dengan ROLE-AKUN-CTL + ROLE-CFO approval khusus, DEC-036)

Response untuk mutasi yang diblok saat SOFT_CLOSED:
```json
{ "error": { "code": "PERIODE_SOFT_CLOSED", "message": "Periode PRD-2026-06 sudah soft-closed. Mutasi tidak diizinkan. Hubungi Finance Controller untuk koreksi darurat." } }
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.periode_buku` | READ + UPDATE | Set `status_periode = 'SOFT_CLOSED'`, `tanggal_soft_close`, `soft_close_approved_by` |
| `trx.*`, `jrnl.header`, `jrnl.gl_status`, `sys.gl_reconciliation_report` | READ | Re-run checklist jika stale |
| `sys.closing_checklist_snapshot` | INSERT | Snapshot SOFT_CLOSE_APPROVE — in-transaction |
| `aud.audit_log` | INSERT | `PERIODE.SOFT_CLOSED` — in-transaction |

### Permissions & SoD
| Permission | Role | MFA | SoD Rule |
|---|---|---|---|
| `periode.softclose.approve` | ROLE-AKUN-CTL | Tidak | `approver_id ≠ soft_close_requested_by` (server-side, DEC-017) |

### Audit Events
| Action | Trigger |
|---|---|
| `PERIODE.SOFT_CLOSED` | `status_periode` berhasil di-set `SOFT_CLOSED` — in-transaction dengan UPDATE `mst.periode_buku` dan INSERT `sys.closing_checklist_snapshot` |

### Acceptance Criteria

```gherkin
Feature: Soft-close approve — transisi OPEN ke SOFT_CLOSED (4-eyes)

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And soft_close_requested_by = USR-011 (ROLE-AKUN-CTL Maker)
    And sys.closing_checklist_snapshot: SOFT_CLOSE_REQUEST dari USR-011, all_passed = true, created_at = 1 jam lalu
    And user ROLE-AKUN-CTL (USR-012) ter-autentikasi — bukan USR-011

  # ─── HAPPY PATH: Approve sukses — status menjadi SOFT_CLOSED ─────────────────

  Scenario: S2-AC1 — ROLE-AKUN-CTL Approver berhasil approve soft-close
    Given checklist masih fresh (< 24 jam dari request)
    And semua 4 checklist item masih passed (kondisi tidak berubah)
    When USR-012 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-approve
      With Idempotency-Key: IK-SC-APR-001
      With body: { "comment": "Approved. Semua posisi diverifikasi.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.periode_buku.status_periode        | SOFT_CLOSED              |
      | mst.periode_buku.tanggal_soft_close    | timestamp now            |
      | mst.periode_buku.soft_close_approved_by | USR-012 UUID            |
      | mst.periode_buku.soft_close_approved_at | timestamp now           |
      | sys.closing_checklist_snapshot.transition | SOFT_CLOSE_APPROVE    |
      | sys.closing_checklist_snapshot.actor_user_id | USR-012 UUID       |
      | sys.closing_checklist_snapshot.all_passed | true                 |
      | aud.audit_log.action                   | PERIODE.SOFT_CLOSED      |
      | aud.audit_log.after_jsonb.status_periode | SOFT_CLOSED            |
    And notifikasi ke ROLE-CFO: "Periode PRD-2026-06 telah di-soft-close oleh USR-012. Siap untuk hard-close sesuai jadwal."
    And notifikasi ke ROLE-RISK: "Periode PRD-2026-06 SOFT_CLOSED — cure evaluation dapat dijalankan manual jika diperlukan (DEC-036)."
    And mutasi `POST /transaksi/penempatan` untuk PRD-2026-06 → HTTP 423 PERIODE_SOFT_CLOSED

  # ─── ERROR: SoD violation — Maker mencoba approve sendiri ───────────────────

  Scenario: S2-AC2 — USR-011 mencoba approve request yang dia sendiri buat — SoD violation
    Given USR-011 adalah soft_close_requested_by untuk PRD-2026-06
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-approve
      With Idempotency-Key: IK-SC-APR-SOD
    Then HTTP 403:
      | error.code    | SOD_VIOLATION                       |
      | error.message | "Anda tidak dapat meng-approve request soft-close yang Anda ajukan sendiri. Segregation of Duties wajib: approver_id ≠ requester_id (DEC-017)." |
    And tidak ada perubahan state
    And aud.audit_log.action = PERIODE.SOFT_CLOSE_APPROVE_REJECTED (advisory, actor = USR-011)

  # ─── ERROR: Checklist stale — kondisi berubah sejak request ─────────────────

  Scenario: S2-AC3 — Approve ditolak karena checklist menjadi stale (transaksi baru PENDING muncul)
    Given 2 transaksi baru masuk dalam PENDING_APPROVAL setelah request diajukan
    And sys.closing_checklist_snapshot SOFT_CLOSE_REQUEST: created_at 25 jam lalu (> 24 jam threshold)
    When USR-012 mengirim POST /api/v1/periode-buku/PRD-2026-06/soft-close-approve
      With Idempotency-Key: IK-SC-APR-003
    Then sistem re-run checklist
    And PENDING_APPROVAL_ZERO item: passed = false (2 transaksi PENDING)
    Then HTTP 422:
      | error.code    | CLOSING_CHECKLIST_FAILED            |
      | error.message | "Checklist dievaluasi ulang karena sudah > 24 jam sejak request. 1 item gagal: PENDING_APPROVAL_ZERO — 2 transaksi masih PENDING." |
    And sys.closing_checklist_snapshot ter-INSERT (re-run, SOFT_CLOSE_APPROVE, REJECTED)
    And mst.periode_buku.status_periode tetap 'OPEN'
    And notifikasi ke USR-011 (requester): "Soft-close approval ditolak: ada 2 transaksi baru PENDING. Request perlu diajukan ulang setelah diselesaikan."

  # ─── ERROR: Tidak ada pending request — approve prematur ─────────────────────

  Scenario: S2-AC4 — Approve dipanggil tanpa ada soft-close request sebelumnya
    Given mst.periode_buku PRD-2026-07: status_periode = 'OPEN', soft_close_requested_by = NULL
    When USR-012 mengirim POST /api/v1/periode-buku/PRD-2026-07/soft-close-approve
      With Idempotency-Key: IK-SC-APR-004
    Then HTTP 422:
      | error.code    | WORKFLOW_INVALID_TRANSITION         |
      | error.message | "Tidak ada soft-close request yang menunggu approval untuk periode PRD-2026-07. Ajukan soft-close request terlebih dahulu." |
    And tidak ada perubahan state
```

---

## Story P5-M4-S3 — Hard-Close Request + Approve (CFO Step-Up MFA, SOFT_CLOSED → CLOSED)

**Actor**: ROLE-AKUN-CTL (request) + ROLE-CFO (approve, step-up MFA mandatory, DEC-027)
**Trigger**: ROLE-AKUN-CTL memulai hard-close setelah periode SOFT_CLOSED dan semua koreksi selesai. ROLE-CFO menerima notifikasi dan mengotorisasi hard-close dengan step-up MFA.
**Goal**: Hard-close adalah operasi irreversible (setelah grace window). ROLE-AKUN-CTL submit `hard-close-request` yang kembali menjalankan checklist. ROLE-CFO approve dengan step-up MFA — mengubah `status_periode` menjadi `CLOSED`, me-lock FX rate untuk periode, dan men-trigger refresh materialized views. Setelah grace window berakhir (default 48 jam), periode tidak bisa di-reopen ke OPEN (hanya ke SOFT_CLOSED dalam window).

### Pre-conditions Hard-Close Request (ROLE-AKUN-CTL)
1. User ter-autentikasi dengan permission `periode.hardclose.request`
2. `mst.periode_buku.status_periode = 'SOFT_CLOSED'`
3. Request mengandung `Idempotency-Key`
4. Checklist 4-item di-run ulang (kondisi mungkin berubah sejak soft-close)

### Pre-conditions Hard-Close Approve (ROLE-CFO)
1. User ter-autentikasi dengan permission `periode.hardclose.approve`
2. `mst.periode_buku.status_periode = 'HARD_CLOSE_PENDING'`
3. **Step-up MFA mandatory** — request HARUS mengandung `X-Step-Up-Token` header yang valid (fresh, < 5 menit dari step-up challenge) (DEC-027)
4. MFA missing atau expired → `HTTP 401 MFA_STEP_UP_REQUIRED`
5. Request mengandung `Idempotency-Key`

### Post-Hard-Close Actions (in-transaction atau via Asynq)
1. `mst.periode_buku.status_periode` = `'CLOSED'`
2. `mst.periode_buku.tanggal_hard_close` = `now()`
3. `mst.periode_buku.hard_close_grace_expires_at` = `now() + INTERVAL '48 hours'` (konfigurabel)
4. `mst.kurs.locked_flag = TRUE` untuk semua row dengan `periode_id = {id}` (P5-M5 dependency)
5. `sys.closing_checklist_snapshot` INSERT (HARD_CLOSE_APPROVE)
6. `aud.audit_log` INSERT `PERIODE.HARDCLOSED` in-transaction
7. Asynq job: `reporting:mv_refresh` — trigger refresh 8 materialized views (dependency P5-M13)
8. Middleware `PeriodeLockMiddleware`: upgrade lock dari SOFT_CLOSED ke CLOSED (423 PERIODE_CLOSED untuk semua mutasi)

### Endpoints
```
POST /api/v1/periode-buku/{id}/hard-close-request
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Body: { "catatan": "Hard close request PRD-2026-06." }
→ 202 Accepted (checklist lolos)

POST /api/v1/periode-buku/{id}/hard-close-approve
Authorization: Bearer <jwt>
X-Step-Up-Token: <mfa-step-up-token>   ← WAJIB (DEC-027)
Idempotency-Key: <uuid-v4>
Body: { "comment": "Hard close approved. Periode final.", "signature_method": "JWT_STEP_UP" }
→ 200 OK
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.periode_buku` | READ + UPDATE | Dua langkah: SOFT_CLOSED → HARD_CLOSE_PENDING → CLOSED |
| `mst.kurs` | UPDATE | `locked_flag = TRUE` setelah hard-close approve |
| `sys.closing_checklist_snapshot` | INSERT | Snapshot HARD_CLOSE_REQUEST + HARD_CLOSE_APPROVE |
| `aud.audit_log` | INSERT | `PERIODE.HARD_CLOSE_REQUESTED`, `PERIODE.HARDCLOSED` — in-transaction |
| `sys.job` | INSERT | Asynq job MV refresh (reporting:mv_refresh) |

### Permissions
| Permission | Role | MFA | Step-Up MFA | Catatan |
|---|---|---|---|---|
| `periode.hardclose.request` | ROLE-AKUN-CTL | Tidak | Tidak | |
| `periode.hardclose.approve` | ROLE-CFO | Ya (mandatory) | Ya (step-up, DEC-027) | `X-Step-Up-Token` header required |

### Audit Events
| Action | Trigger |
|---|---|
| `PERIODE.HARD_CLOSE_REQUESTED` | Request diterima + checklist lolos — in-transaction |
| `PERIODE.HARDCLOSED` | CFO approve, `status_periode = 'CLOSED'` — in-transaction. `after_jsonb` berisi: `{status_periode, tanggal_hard_close, grace_expires_at, step_up_token_ref, mfa_method}` |

### Acceptance Criteria

```gherkin
Feature: Hard-close periode buku — ROLE-AKUN-CTL request + ROLE-CFO step-up MFA approve

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'SOFT_CLOSED'
    And semua 4 checklist item passed (kondisi clean setelah soft-close)
    And user ROLE-AKUN-CTL (USR-011) dan ROLE-CFO (USR-CFO-001)

  # ─── HAPPY PATH: Hard-close end-to-end — request + MFA approve ───────────────

  Scenario: S3-AC1 — Hard-close sukses: AKUN-CTL request + CFO approve dengan step-up MFA
    When USR-011 mengirim POST /api/v1/periode-buku/PRD-2026-06/hard-close-request
      With Idempotency-Key: IK-HC-REQ-001
      With body: { "catatan": "Hard close request PRD-2026-06. Semua koreksi selesai." }
    Then HTTP 202
    And mst.periode_buku.status_periode = 'HARD_CLOSE_PENDING'
    And sys.closing_checklist_snapshot INSERT (HARD_CLOSE_REQUEST, all_passed = true)
    And aud.audit_log action = PERIODE.HARD_CLOSE_REQUESTED
    And notifikasi ke ROLE-CFO: "Hard-close request PRD-2026-06 dari USR-011 menunggu approval CFO. Step-up MFA diperlukan."

    When USR-CFO-001 menyelesaikan step-up MFA challenge (TOTP valid, segar < 5 menit)
    And USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/hard-close-approve
      With X-Step-Up-Token: <valid-stepup-token>
      With Idempotency-Key: IK-HC-APR-001
      With body: { "comment": "Approved. Periode Juni 2026 final.", "signature_method": "JWT_STEP_UP" }
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.periode_buku.status_periode          | CLOSED                   |
      | mst.periode_buku.tanggal_hard_close      | timestamp now            |
      | mst.periode_buku.hard_close_approved_by  | USR-CFO-001 UUID         |
      | mst.periode_buku.hard_close_grace_expires_at | now + 48 jam         |
      | mst.periode_buku.step_up_token_ref       | token reference          |
      | mst.kurs.locked_flag (PRD-2026-06)       | TRUE (semua row)         |
      | sys.closing_checklist_snapshot.transition | HARD_CLOSE_APPROVE      |
      | aud.audit_log.action                     | PERIODE.HARDCLOSED       |
      | aud.audit_log.after_jsonb.mfa_method     | TOTP                     |
    And Asynq job "reporting:mv_refresh" di-enqueue (dependency P5-M13)
    And mutasi apapun untuk PRD-2026-06 → HTTP 423 PERIODE_CLOSED
    And notifikasi ke ROLE-AKUN-CTL + ROLE-RISK + ROLE-AUDIT: "Periode PRD-2026-06 berhasil HARD CLOSED oleh CFO pada [timestamp]."

  # ─── ERROR: Step-up MFA missing — CFO tidak bisa approve tanpa token ─────────

  Scenario: S3-AC2 — Hard-close approve ditolak karena X-Step-Up-Token tidak ada (DEC-027)
    Given mst.periode_buku PRD-2026-06: status_periode = 'HARD_CLOSE_PENDING'
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/hard-close-approve
      Without X-Step-Up-Token header
      With Idempotency-Key: IK-HC-APR-NOMFA
    Then HTTP 401:
      | error.code    | MFA_STEP_UP_REQUIRED                |
      | error.message | "Hard-close periode buku wajib step-up MFA (DEC-027). Lakukan challenge via POST /auth/step-up lalu sertakan X-Step-Up-Token di request." |
    And tidak ada perubahan state
    And mst.periode_buku.status_periode tetap 'HARD_CLOSE_PENDING'
    And aud.audit_log.action = PERIODE.HARD_CLOSE_APPROVE_REJECTED_NO_MFA (advisory)

  # ─── ERROR: Step-up MFA expired — token > 5 menit ────────────────────────────

  Scenario: S3-AC3 — Hard-close approve ditolak karena step-up token sudah expired (> 5 menit)
    Given USR-CFO-001 menyelesaikan step-up MFA 8 menit lalu
    And X-Step-Up-Token: <expired-token>
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/hard-close-approve
      With X-Step-Up-Token: <expired-token>
      With Idempotency-Key: IK-HC-APR-EXPIRY
    Then HTTP 401:
      | error.code    | MFA_STEP_UP_EXPIRED                 |
      | error.message | "Step-up MFA token sudah expired (maksimal 5 menit). Ulangi step-up challenge." |
    And tidak ada perubahan state

  # ─── ERROR: PERIODE_CLOSED — mutasi setelah hard-close diblok ───────────────

  Scenario: S3-AC4 — Mutasi instrumen ditolak 423 karena periode sudah CLOSED
    Given mst.periode_buku PRD-2026-06: status_periode = 'CLOSED'
    When ROLE-MAKER-TR mencoba POST /api/v1/transaksi/penempatan
      With body mengandung periode_id = PRD-2026-06
    Then HTTP 423:
      | error.code    | PERIODE_CLOSED                      |
      | error.message | "Periode PRD-2026-06 sudah hard-closed pada [tanggal]. Mutasi tidak diizinkan. Hubungi CFO untuk reopen (hanya dalam grace window 48 jam)." |
    And tidak ada INSERT ke trx.penempatan
    And tidak ada entry audit
```

### Open Questions — Story 3
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M4-3a | Apakah ROLE-CFO dapat me-reject hard-close request (mengembalikan ke SOFT_CLOSED) tanpa MFA? | **Ya** — reject tidak butuh step-up MFA karena bukan action destructive. `HARD_CLOSE_PENDING → SOFT_CLOSED` via `POST /hard-close-reject`. system-analyst tambahkan endpoint ini. |
| OQ-M4-3b | Grace window default 48 jam — apakah konfigurabel per periode atau global? | **Global via `sys.config_param` `HARD_CLOSE_GRACE_WINDOW_HOURS`**, default 48. Override per-periode tidak diizinkan (untuk konsistensi audit). |
| OQ-M4-3c | Apakah Asynq job MV refresh (P5-M13) blocking hard-close response atau async? | **Async** — hard-close response dikembalikan setelah transaksi DB commit. MV refresh adalah best-effort background job. Failure MV refresh tidak reverse hard-close. |

---

## Story P5-M4-S4 — Reopen Periode (ROLE-CFO, MFA, Audit Reason Mandatory)

**Actor**: ROLE-CFO (request + approve — karena ini operasi sensitif, CFO sendiri yang memvalidasi)
**Trigger**: Situasi exceptional: ditemukan koreksi material setelah soft-close atau closed. ROLE-CFO membuka `/periode-buku/{id}` dan memilih "Ajukan Reopen".
**Goal**: Reopen adalah operasi exceptional dengan audit reason wajib (≥ 30 karakter). Dua skenario yang diizinkan: (1) `SOFT_CLOSED → OPEN` — tidak ada batasan waktu, tidak perlu step-up MFA. (2) `CLOSED → SOFT_CLOSED` — hanya dalam grace window (`hard_close_grace_expires_at`), wajib step-up MFA. Setelah grace window: `PERIODE_GRACE_EXPIRED` (423) — tidak bisa reopen ke OPEN, hanya ke SOFT_CLOSED via mekanisme manual yang membutuhkan eskalasi ke DEC-RFC. SoD: untuk konsistensi, reopen CLOSED→SOFT_CLOSED harus diajukan dan disetujui oleh CFO (actor yang sama diizinkan karena tidak ada opsi lain — single CFO role), namun kedua action (`reopen-request` dan `reopen-approve`) di-log secara terpisah.

### State Transitions yang Diizinkan
| Dari | Ke | Syarat | MFA Step-Up |
|---|---|---|---|
| `SOFT_CLOSED` | `OPEN` | Tidak ada batasan waktu; CFO MFA (login) cukup | Tidak |
| `CLOSED` | `SOFT_CLOSED` | Dalam grace window (`now() < hard_close_grace_expires_at`) | **Wajib** (DEC-027) |
| `CLOSED` | `OPEN` | **Tidak diizinkan** dalam P5-M4 | — |
| `CLOSED` setelah grace | apapun | `PERIODE_GRACE_EXPIRED` — eskalasi via RFC | — |

### Endpoints
```
POST /api/v1/periode-buku/{id}/reopen-request
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>
Body: {
  "target_status": "OPEN",          // atau "SOFT_CLOSED" (CLOSED → SOFT_CLOSED)
  "reason": "Ditemukan jurnal FVOCI yang salah booking OCI periode Juni 2026. Koreksi material. Persetujuan ALCO sudah diperoleh via email referensi ALCO/2026/VI/001.",
  "row_version": 9
}

POST /api/v1/periode-buku/{id}/reopen-approve
Authorization: Bearer <jwt>
X-Step-Up-Token: <mfa-step-up-token>   ← Wajib hanya jika CLOSED → SOFT_CLOSED
Idempotency-Key: <uuid-v4>
Body: { "comment": "Reopen disetujui.", "signature_method": "JWT_STEP_UP" }
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.periode_buku` | READ + UPDATE | Reset `status_periode`, `reopened_flag = TRUE`, `reopened_reason`, `reopened_at`, `reopened_by` |
| `mst.kurs` | UPDATE | `locked_flag = FALSE` jika reopen dari CLOSED |
| `sys.closing_checklist_snapshot` | INSERT | Snapshot REOPEN_REQUEST + REOPEN_APPROVE |
| `aud.audit_log` | INSERT | `PERIODE.REOPEN_REQUESTED`, `PERIODE.REOPENED` — in-transaction |

### Permissions
| Permission | Role | MFA | Step-Up MFA | Catatan |
|---|---|---|---|---|
| `periode.reopen.request` | ROLE-CFO | Ya (login) | Tidak (request) | |
| `periode.reopen.approve` | ROLE-CFO | Ya (login) | Ya jika CLOSED→SOFT_CLOSED (DEC-027) | |

### Audit Events
| Action | Trigger | Catatan |
|---|---|---|
| `PERIODE.REOPEN_REQUESTED` | Request diterima | `after_jsonb.reason` wajib ada |
| `PERIODE.REOPENED` | Status berhasil dikembalikan | `after_jsonb.previous_status`, `after_jsonb.new_status`, `after_jsonb.mfa_method` (jika ada) |

### Acceptance Criteria

```gherkin
Feature: Reopen periode buku oleh ROLE-CFO — exceptional operation

  Background:
    Given user ROLE-CFO (USR-CFO-001) ter-autentikasi dengan permission periode.reopen.request + periode.reopen.approve

  # ─── HAPPY PATH: Reopen dari SOFT_CLOSED ke OPEN (tanpa step-up MFA) ─────────

  Scenario: S4-AC1 — CFO berhasil reopen periode dari SOFT_CLOSED ke OPEN
    Given mst.periode_buku PRD-2026-06: status_periode = 'SOFT_CLOSED'
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-request
      With Idempotency-Key: IK-RO-REQ-001
      With body: {
        "target_status": "OPEN",
        "reason": "Ditemukan salah posting FVOCI OCI periode Juni 2026. Koreksi material. Referensi: ALCO/2026/VI/001.",
        "row_version": 7
      }
    Then HTTP 202
    And aud.audit_log.action = PERIODE.REOPEN_REQUESTED
    And mst.periode_buku.reopened_reason = "Ditemukan salah posting..."
    And sys.closing_checklist_snapshot INSERT (REOPEN_REQUEST)

    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-approve
      Without X-Step-Up-Token (tidak wajib untuk SOFT_CLOSED → OPEN)
      With Idempotency-Key: IK-RO-APR-001
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.periode_buku.status_periode    | OPEN                         |
      | mst.periode_buku.reopened_flag     | TRUE                         |
      | mst.periode_buku.reopened_at       | timestamp now                |
      | mst.periode_buku.reopened_by       | USR-CFO-001 UUID             |
      | mst.periode_buku.reopened_approved_by | USR-CFO-001 UUID          |
      | sys.closing_checklist_snapshot.transition | REOPEN_APPROVE          |
      | aud.audit_log.action               | PERIODE.REOPENED             |
      | aud.audit_log.after_jsonb.previous_status | SOFT_CLOSED           |
      | aud.audit_log.after_jsonb.new_status      | OPEN                  |
      | aud.audit_log.after_jsonb.reason           | "Ditemukan salah posting..." |
    And notifikasi ke ROLE-AKUN-CTL + ROLE-RISK + ROLE-AKUN: "PERHATIAN: Periode PRD-2026-06 telah di-REOPEN oleh CFO. Alasan: [reason excerpt]. Transaksi dapat diinput kembali."

  # ─── HAPPY PATH: Reopen dari CLOSED ke SOFT_CLOSED (dalam grace window, step-up MFA) ──

  Scenario: S4-AC2 — CFO reopen dari CLOSED ke SOFT_CLOSED dalam grace window dengan step-up MFA
    Given mst.periode_buku PRD-2026-06: status_periode = 'CLOSED'
    And hard_close_grace_expires_at = sekarang + 20 jam (masih dalam 48 jam grace window)
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-request
      With body: { "target_status": "SOFT_CLOSED", "reason": "Koreksi deposito BCA INST-DEP-9988 salah periode. Persetujuan Direksi tersedia.", "row_version": 9 }
    And USR-CFO-001 menyelesaikan step-up MFA (TOTP valid)
    And USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-approve
      With X-Step-Up-Token: <valid-stepup-token>
    Then HTTP 200
    And dalam satu transaksi DB:
      | mst.periode_buku.status_periode    | SOFT_CLOSED                  |
      | mst.kurs.locked_flag (PRD-2026-06) | FALSE                        |
      | aud.audit_log.after_jsonb.mfa_method | TOTP                       |
    And mutasi kembali diizinkan (CORRECTION_PERIODE_CLOSED atau full OPEN jika SOFT_CLOSED → koreksi saja)

  # ─── ERROR: Alasan reopen terlalu pendek (< 30 karakter) ────────────────────

  Scenario: S4-AC3 — Reopen ditolak karena reason kurang dari 30 karakter
    Given mst.periode_buku PRD-2026-06: status_periode = 'SOFT_CLOSED'
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-request
      With body: { "target_status": "OPEN", "reason": "Salah posting.", "row_version": 7 }
    Then HTTP 400:
      | error.code              | VALIDATION_FAILED               |
      | error.details[0].field  | reason                          |
      | error.details[0].rule   | "min_length:30 — alasan reopen wajib minimal 30 karakter untuk audit compliance" |
    And tidak ada perubahan state
    And tidak ada INSERT ke sys.closing_checklist_snapshot

  # ─── ERROR: Reopen setelah grace window expired ──────────────────────────────

  Scenario: S4-AC4 — Reopen CLOSED ditolak karena grace window sudah lewat
    Given mst.periode_buku PRD-2026-06: status_periode = 'CLOSED'
    And hard_close_grace_expires_at = 3 hari lalu (sudah expired)
    When USR-CFO-001 mengirim POST /api/v1/periode-buku/PRD-2026-06/reopen-request
      With body: { "target_status": "SOFT_CLOSED", "reason": "Koreksi diperlukan setelah audit eksternal menemukan salah saji pada obligasi pemerintah.", "row_version": 9 }
    Then HTTP 423:
      | error.code    | PERIODE_GRACE_EXPIRED               |
      | error.message | "Grace window untuk reopen periode PRD-2026-06 telah berakhir pada [tanggal]. Reopen tidak dapat dilakukan secara otomatis. Ajukan RFC ke Direksi sesuai RACI BRD §3." |
    And tidak ada perubahan state
    And aud.audit_log.action = PERIODE.REOPEN_REJECTED_GRACE_EXPIRED (advisory)
```

### Open Questions — Story 4
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M4-4a | Apakah perlu 2-orang approval untuk reopen CLOSED (CFO + satu lagi)? | **Tidak di P5-M4** — CFO memiliki otoritas solo untuk reopen. Jiika ALCO atau Komite terlibat, itu di luar sistem (via email/persetujuan manual) dan alasannya di-log di field `reason`. Phase 6 bisa tambah multi-approver reopen jika compliance audit minta. |
| OQ-M4-4b | Apakah `reopened_flag` bisa menjadi TRUE lebih dari sekali (periode di-reopen berulang)? | **Ya** — `reopened_flag` tidak di-reset. Setiap reopen menambah row baru di `sys.closing_checklist_snapshot`. Counter reopen bisa ditambahkan di Phase 6. Konfirmasi ke data-modeler untuk `reopen_count INT DEFAULT 0`. |

---

## Story P5-M4-S5 — Closing Checklist Endpoint + Status Periode Report

**Actor**: ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT (read); ROLE-AKUN (read)
**Trigger 1 (real-time checklist)**: ROLE-AKUN-CTL membuka halaman `/periode-buku/{id}` dan mengklik "Cek Closing Checklist" untuk melihat status 4-item pre-condition sebelum mengajukan close
**Trigger 2 (status report)**: ROLE-CFO, ROLE-AKUN-CTL, atau ROLE-RISK membuka `/reports/status-periode` untuk melihat ringkasan status semua periode buku aktif
**Goal**: (1) `GET /api/v1/periode-buku/{id}/closing-checklist` mengembalikan evaluasi real-time 4-item checklist (tanpa mempersist snapshot — snapshot hanya di-persist pada actual transition). (2) `GET /api/v1/reports/status-periode` mengembalikan tabel summary seluruh periode buku dengan status, posisi closing checklist terakhir, dan history transisi. Keduanya mengikuti pattern list+filter+export (UX §1).

### Endpoint 1: Closing Checklist (Real-Time)
```
GET /api/v1/periode-buku/{id}/closing-checklist

→ 200 OK
{
  "data": {
    "periode_id": "<uuid>",
    "periode_kode": "PRD-2026-06",
    "status_periode": "OPEN",
    "evaluated_at": "2026-06-30T16:45:00+07:00",
    "all_passed": false,
    "items": [
      {
        "key":    "PENDING_APPROVAL_ZERO",
        "label":  "0 transaksi/jurnal masih PENDING_APPROVAL",
        "passed": true,
        "detail": "Total PENDING_APPROVAL: 0"
      },
      {
        "key":    "JURNAL_BALANCED",
        "label":  "Semua jurnal seimbang",
        "passed": true,
        "detail": "248 jurnal checked. Max delta: IDR 0.0000"
      },
      {
        "key":    "GL_DELIVERED",
        "label":  "Tidak ada GL delivery FAILED",
        "passed": false,
        "detail": "3 jurnal masih FAILED. Header IDs: [uuid1, uuid2, uuid3]",
        "action_url": "/jurnal/gl-delivery-dlq?filter[periode_id]=PRD-2026-06"
      },
      {
        "key":    "RECON_PASS",
        "label":  "GL rekonsiliasi terakhir COMPLETED",
        "passed": true,
        "detail": "Last recon: 2026-06-29 — COMPLETED. Delta IDR 0.0000"
      }
    ],
    "last_snapshot": {
      "snapshot_id": "<uuid>",
      "transition": "SOFT_CLOSE_REQUEST",
      "evaluated_at": "2026-06-29T14:00:00+07:00",
      "all_passed": true
    }
  }
}
```

### Endpoint 2: Status Periode Report
```
GET /api/v1/reports/status-periode
  ?sort=tanggal_akhir:desc
  &filter[status_periode]=OPEN
  &filter[tahun_buku]=2026
  &cursor=...&limit=50

→ 200 OK
{
  "data": [
    {
      "periode_id": "<uuid>",
      "periode_kode": "PRD-2026-06",
      "tipe_periode": "BULANAN",
      "tahun_buku": 2026,
      "bulan": 6,
      "tanggal_mulai": "2026-06-01",
      "tanggal_akhir": "2026-06-30",
      "status_periode": "CLOSED",
      "tanggal_soft_close": "2026-06-30T17:05:22+07:00",
      "tanggal_hard_close": "2026-07-02T09:30:00+07:00",
      "soft_close_by": "USR-012",
      "hard_close_by": "USR-CFO-001",
      "reopened_flag": false,
      "checklist_last_snapshot": {
        "transition": "HARD_CLOSE_APPROVE",
        "all_passed": true,
        "evaluated_at": "2026-07-02T09:29:00+07:00"
      },
      "mv_refresh_status": "COMPLETED",
      "mv_refresh_at": "2026-07-02T09:35:00+07:00"
    },
    ...
  ],
  "pagination": { "nextCursor": "...", "hasMore": false, "totalEstimate": 12 },
  "meta": { "traceId": "..." }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.periode_buku` | READ | Data utama |
| `trx.*`, `jrnl.header` | READ | PENDING_APPROVAL_ZERO real-time check |
| `jrnl.gl_status` | READ | GL_DELIVERED real-time check |
| `sys.gl_reconciliation_report` | READ | RECON_PASS real-time check |
| `sys.closing_checklist_snapshot` | READ | Last snapshot untuk context |
| `sys.job` | READ | MV refresh status (dari P5-M13) |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `periode.read` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-RISK, ROLE-AKUN, ROLE-AUDIT | Lihat checklist + status report |
| `periode.export` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Export status periode report |

### Audit Events
- `PERIODE.CHECKLIST.READ` — setiap call ke closing-checklist endpoint (untuk monitoring akses)
- `PERIODE.STATUS_REPORT.EXPORT` — saat export CSV/XLSX dilakukan (`PERIODE.EXPORT`)

### Acceptance Criteria

```gherkin
Feature: Closing checklist real-time + status periode report

  Background:
    Given mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And 248 jrnl.header rows POSTED untuk PRD-2026-06
    And user ROLE-AKUN-CTL (USR-011) ter-autentikasi

  # ─── HAPPY PATH: Checklist real-time — 1 item gagal, actionable detail ────────

  Scenario: S5-AC1 — ROLE-AKUN-CTL melihat checklist dengan 1 item gagal (GL_DELIVERED)
    Given 3 jrnl.gl_status rows: gl_host_status = 'FAILED' untuk header dalam PRD-2026-06
    And PENDING_APPROVAL_ZERO: passed, JURNAL_BALANCED: passed, RECON_PASS: passed
    When USR-011 mengakses GET /api/v1/periode-buku/PRD-2026-06/closing-checklist
    Then HTTP 200
    And response.data.all_passed = false
    And response.data.items[GL_DELIVERED].passed = false
    And response.data.items[GL_DELIVERED].detail = "3 jurnal masih FAILED. Header IDs: [uuid1, uuid2, uuid3]"
    And response.data.items[GL_DELIVERED].action_url = "/jurnal/gl-delivery-dlq?filter[periode_id]=PRD-2026-06"
    And response.data.items[PENDING_APPROVAL_ZERO].passed = true
    And response.data.items[JURNAL_BALANCED].passed = true
    And response.data.items[RECON_PASS].passed = true
    And UI menampilkan:
      | Badge hijau untuk item passed                                        |
      | Badge merah untuk item failed + tombol "Tindak Lanjut →" (action_url) |
      | Tombol "Ajukan Soft Close" di-disable karena all_passed = false       |
      | Tombol "Refresh Checklist" untuk re-evaluasi manual                   |
    And tidak ada INSERT ke sys.closing_checklist_snapshot (read-only endpoint)

  # ─── HAPPY PATH: Status periode report — list multi-periode dengan filter ─────

  Scenario: S5-AC2 — ROLE-CFO melihat status report semua periode 2026
    Given 12 periode buku tahun 2026 dalam berbagai status:
      | PRD-2026-01 | CLOSED        | hard-closed Februari 2026 |
      | PRD-2026-06 | SOFT_CLOSED   | soft-closed kemarin       |
      | PRD-2026-07 | OPEN          | periode berjalan          |
      | PRD-2026-08..12 | OPEN      | periode mendatang         |
    When ROLE-CFO mengakses GET /api/v1/reports/status-periode?filter[tahun_buku]=2026&sort=tanggal_akhir:desc
    Then HTTP 200 dengan 12 entries
    And setiap entry mengandung: periode_kode, status_periode, tanggal_soft_close, tanggal_hard_close, checklist_last_snapshot, mv_refresh_status
    And UI menampilkan DataTable dengan:
      | Badge CLOSED hijau gelap untuk PRD-2026-01..05  |
      | Badge SOFT_CLOSED amber untuk PRD-2026-06       |
      | Badge OPEN biru untuk PRD-2026-07..12           |
      | Filter chip "Tahun: 2026" aktif                 |
      | Sort by tanggal_akhir descending aktif          |
      | Export CSV/XLSX tersedia                        |
    And ROLE-AUDIT juga bisa akses (permission periode.read) dengan data yang sama
    And ROLE-MAKER-TR (tanpa permission periode.read) → HTTP 403 FORBIDDEN

  # ─── HAPPY PATH: Export status-periode — audit trail PERIODE.EXPORT ──────────

  Scenario: S5-AC3 — ROLE-AUDIT export status periode report ke CSV
    Given 12 periode buku tersedia
    When ROLE-AUDIT mengakses GET /api/v1/reports/status-periode/export?format=csv&filter[tahun_buku]=2026
    Then HTTP 200 dengan header Content-Disposition: attachment; filename="status-periode-2026-20260617.csv"
    And CSV berisi 12 rows + header row dalam Bahasa Indonesia
    And dalam satu transaksi:
      | aud.audit_log.action       | PERIODE.EXPORT                  |
      | aud.audit_log.after_jsonb  | { format: "csv", row_count: 12, filters: {tahun_buku: 2026} } |
    And file tidak mengandung data keuangan sensitif di luar scope user (RBAC respected)

  # ─── ERROR: Checklist untuk periode yang sudah CLOSED (informatif, tidak blocking) ──

  Scenario: S5-AC4 — Checklist untuk periode CLOSED menampilkan status historical + informasi MV refresh
    Given mst.periode_buku PRD-2026-05: status_periode = 'CLOSED'
    And sys.closing_checklist_snapshot terakhir: HARD_CLOSE_APPROVE, all_passed = true
    And Asynq job MV refresh: status = 'completed', completed_at = 2026-06-02T10:00:00+07:00
    When USR-011 mengakses GET /api/v1/periode-buku/PRD-2026-05/closing-checklist
    Then HTTP 200
    And response.data.status_periode = 'CLOSED'
    And response.data.all_passed = true (dari last snapshot, karena periode CLOSED tidak perlu re-evaluate)
    And response.data.last_snapshot.transition = 'HARD_CLOSE_APPROVE'
    And response.data.mv_refresh = { "status": "completed", "completed_at": "2026-06-02T10:00:00+07:00" }
    And UI menampilkan badge "CLOSED — Final" dengan timestamp hard-close + nama CFO
    And UI menampilkan "Materialized Views: Diperbarui 2026-06-02 10:00" dengan link ke /reports/status-periode
    And tidak ada tombol "Ajukan Soft Close" atau "Ajukan Hard Close" (periode sudah terminal)
```

### Open Questions — Story 5
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M4-5a | Apakah `/reports/status-periode` termasuk dalam `rpt.mv_*` atau query langsung? | **Query langsung** dari `mst.periode_buku` JOIN `sys.closing_checklist_snapshot` — tidak perlu MV karena jumlah periode relatif kecil (max ~120 per 10 tahun). MV overhead tidak worth it. |
| OQ-M4-5b | Apakah checklist endpoint untuk periode CLOSED melakukan real-time check atau hanya return last snapshot? | **Last snapshot** untuk periode CLOSED (tidak re-run — kondisi di tabel historis mungkin sudah tidak representatif). Periode OPEN/SOFT_CLOSED/HARD_CLOSE_PENDING: real-time. |
| OQ-M4-5c | Apakah ROLE-AKUN (bukan CTL) bisa melihat closing-checklist? | **Ya** — `periode.read` permission termasuk `ROLE-AKUN`. Mereka perlu tahu apakah periode bisa di-close untuk planning transaksi. Export dikontrol oleh `periode.export` permission yang lebih terbatas. |

---

## Ringkasan P5-M4 Story Set

| Story | Judul | Actor Utama | AC Count | State Transition | Gate |
|---|---|---|---|---|---|
| P5-M4-S1 | Soft-close request + checklist snapshot | ROLE-AKUN-CTL (Maker) | 4 | OPEN (pending request) | compliance BLOCKING + security (audit) |
| P5-M4-S2 | Soft-close approve (4-eyes, SoD) | ROLE-AKUN-CTL (Approver, SoD) | 4 | OPEN → SOFT_CLOSED | compliance BLOCKING + security (SoD) |
| P5-M4-S3 | Hard-close request + CFO approve (step-up MFA) | ROLE-AKUN-CTL + ROLE-CFO | 4 | SOFT_CLOSED → HARD_CLOSE_PENDING → CLOSED | compliance BLOCKING + security BLOCKING (DEC-027) |
| P5-M4-S4 | Reopen periode (CFO, audit reason, grace window) | ROLE-CFO | 4 | SOFT_CLOSED → OPEN \| CLOSED → SOFT_CLOSED | security BLOCKING (MFA CLOSED→SOFT_CLOSED) |
| P5-M4-S5 | Closing checklist + status periode report | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | 4 | Read-only (no state change) | advisory |
| **Total** | | | **20** | | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

Kode baru yang dibutuhkan P5-M4 dan belum ada di `api-conventions.md`:

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `CLOSING_CHECKLIST_FAILED` | 422 | Pre-condition checklist gagal saat soft/hard-close request | `details[]` berisi item mana yang gagal + alasan |
| `CLOSING_CHECKLIST_STALE` | 422 | Checklist lebih dari `SOFT_CLOSE_CHECKLIST_STALE_HOURS` sejak request — kondisi berubah | Re-run checklist diperlukan |
| `PERIODE_SOFT_CLOSED` | 423 | Mutasi ditolak karena periode dalam status SOFT_CLOSED | Subset dari 423, lebih informatif |
| `MFA_STEP_UP_REQUIRED` | 401 | Hard-close approve atau reopen CLOSED tanpa `X-Step-Up-Token` | Instruksi endpoint step-up di message |
| `MFA_STEP_UP_EXPIRED` | 401 | `X-Step-Up-Token` sudah > 5 menit | User harus ulangi step-up challenge |
| `PERIODE_GRACE_EXPIRED` | 423 | Reopen CLOSED setelah grace window berakhir | Eskalasi manual via RFC |
| `SOFT_CLOSE_PENDING_EXISTS` | 409 | Soft-close request baru diajukan padahal sudah ada yang pending | Satu pending request per periode |

Catatan: `PERIODE_CLOSED` (HTTP 423) dan `SOD_VIOLATION` (HTTP 403) sudah ada di `api-conventions.md` — tidak perlu ditambahkan ulang.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M4 | MFA Level |
|---|---|---|---|
| ROLE-AKUN-CTL | `periode.softclose.request`, `periode.softclose.approve` (SoD), `periode.hardclose.request`, `periode.read`, `periode.export` | Submit soft-close request; approve soft-close (SoD); submit hard-close request; view checklist + report | Login MFA jika senior (tidak wajib per DEC-026 untuk level ini) |
| ROLE-CFO | `periode.hardclose.approve`, `periode.reopen.request`, `periode.reopen.approve`, `periode.read`, `periode.export` | Approve hard-close (step-up MFA mandatory); reopen (step-up MFA untuk CLOSED→SOFT_CLOSED) | MFA wajib (DEC-026) + step-up untuk approve/reopen CLOSED (DEC-027) |
| ROLE-AKUN | `periode.read` | View closing checklist + status report (read-only) | Tidak wajib |
| ROLE-RISK | `periode.read` | View status periode; trigger cure evaluation di SOFT_CLOSED (DEC-036) | Tidak wajib |
| ROLE-AUDIT | `periode.read`, `periode.export` | View + export semua periode data, checklist snapshots | Tidak wajib |
| ROLE-MAKER-TR, ROLE-APPR-TR | — | Mutasi ditolak 423 saat SOFT_CLOSED/CLOSED (cross-cutting) | — |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `jrnl.header` jurnal balanced | P5-M2 → P5-M4 | JURNAL_BALANCED checklist item query dari `jrnl.header.total_debit/kredit` |
| `jrnl.gl_status` FAILED check | P5-M3 → P5-M4 | GL_DELIVERED checklist item query dari `jrnl.gl_status WHERE gl_host_status = 'FAILED'` |
| `sys.gl_reconciliation_report` | P5-M3 → P5-M4 | RECON_PASS checklist item dari report terakhir per periode |
| `mst.kurs.locked_flag` | P5-M4 → P5-M5 | P5-M4 hard-close men-set `locked_flag = TRUE`; FX entry baru ditolak saat CLOSED |
| MV refresh trigger | P5-M4 → P5-M13 | Hard-close approve menerbitkan Asynq event `reporting:mv_refresh` yang dikonsumsi P5-M13 |
| Status periode filter | P5-M4 → P5-M9 | Akrual harian (P5-M9) skip posting jurnal ke jrnl.header jika periode CLOSED |
| Cure evaluation | P5-M4 ↔ DEC-036 | SOFT_CLOSED mengizinkan cure evaluation manual oleh ROLE-RISK dengan jurnal CORRECTION_PERIODE_CLOSED |
| Frontend screens | P5-M4 → P5-M17 | P5-M17 mengimplementasikan UI closing workflow berdasarkan OpenAPI contract P5-M4 |

---

## Compliance & Security Handoff Checklist

### Untuk security-engineer (BLOCKING gate)
- [ ] `X-Step-Up-Token` validation di middleware sebelum `/hard-close-approve` handler — jika missing atau expired → 401 (bukan 403). Token harus memiliki `issued_at < 5 menit` dan `scope = "hard_close"`.
- [ ] `PERIODE.HARDCLOSED` audit row ditulis **in-transaction** dengan UPDATE `mst.periode_buku` — tidak boleh async
- [ ] `sys.closing_checklist_snapshot` wajib di-protect dengan BEFORE DELETE trigger (append-only, seperti `aud.audit_log`) — tidak bisa di-hard-delete
- [ ] Endpoint `/reopen-approve` (CLOSED→SOFT_CLOSED) wajib step-up MFA — test skenario "POST tanpa X-Step-Up-Token → 401 bukan 200"
- [ ] SoD enforcement `/soft-close-approve`: service layer check `approver_id ≠ soft_close_requested_by` sebelum processing — bukan hanya UI disable
- [ ] `step_up_token_ref` di `mst.periode_buku` — simpan referensi token (bukan plaintext MFA code), cukup token ID atau hash
- [ ] `PERIODE.SOFT_CLOSE_APPROVE_REJECTED` dan `PERIODE.HARD_CLOSE_APPROVE_REJECTED_NO_MFA` ditulis ke `aud.audit_log` (advisory — outside main transaction karena main transaction di-abort)
- [ ] PeriodeLockMiddleware wajib check `status_periode` dari DB (bukan dari JWT/session cache) untuk hindari stale data bypass

### Untuk ifrs9-compliance-reviewer (BLOCKING gate)
- [ ] JURNAL_BALANCED threshold IDR 0.01 — apakah sesuai dengan FSD-APP-D toleransi? Konfirmasi: delta > 0.01 per jurnal header = FAIL
- [ ] GL_DELIVERED: DEAD_LETTER entries dikecualikan dari check (sudah di-discard eksplisit oleh ROLE-IT-ADMIN) — apakah ini sesuai dengan interpretasi PSAK 71 completeness?
- [ ] RECON_PASS strict `COMPLETED` (bukan `COMPLETED_WITH_MISMATCH`) — apakah ada threshold yang bisa dikonfigurasikan? Saat ini: ANY mismatch > IDR 1 = FAIL
- [ ] Cure evaluation di SOFT_CLOSED (DEC-036) — konfirmasi: jurnal CORRECTION_PERIODE_CLOSED adalah event code terpisah dari AKRUAL_BUNGA/ECL_CHARGE. Tidak boleh mengubah ECL calc run yang sudah di-seal.
- [ ] Hard-close post-actions: MV refresh (P5-M13) — apakah MV harus selesai sebelum periode dianggap CLOSED, atau async? Rekomendasi: async, tapi jika MV gagal → alert ke ROLE-AKUN-CTL, tidak reverse hard-close

### Untuk data-modeler
- [ ] Migration 000038: ADD COLUMNS ke `mst.periode_buku` (12 kolom baru untuk close workflow tracking — lihat tabel di atas)
- [ ] Migration 000038: UPGRADE `ck_periode_status` CHECK constraint untuk include `'HARD_CLOSE_PENDING'` state
- [ ] Migration 000038: CREATE TABLE `sys.closing_checklist_snapshot` (append-only, BEFORE DELETE trigger)
- [ ] Migration 000038: Indexes: `(periode_id, transition, created_at DESC)` di `sys.closing_checklist_snapshot`; `(status_periode, tahun_buku, bulan)` partial `WHERE deleted_at IS NULL` di `mst.periode_buku` (sudah ada di 000009 — verify)
- [ ] `mst.kurs.locked_flag BOOLEAN DEFAULT FALSE` — konfirmasi kolom ini sudah ada atau perlu ditambah di migration P5-M5

### Confirmed decisions P5-M4
- **DEC-017 ENFORCED**: 4-eyes dengan SoD `approver_id ≠ requester_id` untuk soft-close; CFO sole approver untuk hard-close (tidak ada 6-eyes untuk close workflow)
- **DEC-027 ENFORCED**: Step-up MFA mandatory di `/hard-close-approve` dan `/reopen-approve` (CLOSED→SOFT_CLOSED)
- **DEC-036 RESOLVED**: Cure evaluation boleh berjalan di SOFT_CLOSED — jurnal via event code `CORRECTION_PERIODE_CLOSED`
- **PERIODE_CLOSED cross-cutting**: Enforcement via middleware, bukan per-endpoint check — konsisten untuk semua `trx.*`, `jrnl.*`, `ecl.*` endpoint

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine, dan ke `data-modeler` untuk migration 000038. Security-engineer harus mereview sebelum implementasi hard-close approve. ifrs9-compliance-reviewer harus memvalidasi checklist thresholds sebelum UAT._
