# P5-M3 — GL Host REST Delivery: User Stories

**Story Set ID**: P5-M3
**Modul**: APP-D — GL Interface (Phase 5, Sprint 2)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `security-engineer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-15
**Linked FSD**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx §4 (GL Host REST Interface)
**Linked BRD**: BRD §6.4 (APP-D GL Integration), RACI: ROLE-AKUN-CTL (A), ROLE-AKUN (R), ROLE-IT-ADMIN (R), ROLE-CFO (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-005` (LOCKED) — GL Integration Phase 1 = file batch (deferred); **Phase 2 REST real-time = sekarang aktif di P5-M3**
- `DEC-007` — Job queue: Asynq (Go-native, Redis-based)
- `DEC-018` — audit trail append-only, retensi 10+10 tahun
- `DEC-021` — Idempotency-Key wajib setiap mutating endpoint
- `DEC-027` — Step-up MFA tidak diperlukan untuk GL delivery routine; berlaku untuk periode hard-close (P5-M4)
- `DEC-030` — Async REST via Asynq (resolved) — GL delivery mode = Asynq queue per jurnal_header
- `DEC-031` — GL Host vendor: **TBD** — integration-engineer lakukan discovery; P5-M3 menggunakan adapter interface sehingga vendor bisa di-swap sebelum UAT

**Dependensi**:
- **P5-M2** (merged PR #113/#118) — menghasilkan `jrnl.header` rows dengan `status_internal = 'POSTED'`
- `jrnl.gl_status` sudah ada dari migration 000001 (init_schema) + trigger hardening dari migration 000005
- `sys.job` dipakai untuk tracking progress job rekonsiliasi (pattern sesuai §3 UX)

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment: `POST /jurnal/header/{id}/retry-gl-delivery`, `GET /jurnal/gl-delivery-dlq`, `GET /jurnal/reconciliation/daily`; state machine `jrnl.gl_status.gl_host_status`; error codes `GL_DELIVERY_FAILED`, `GL_HOST_UNREACHABLE`, `GL_HOST_REJECTED`, `GL_RECON_MISMATCH`, `GL_DLQ_ABANDONED`
- `data-modeler` → migration 000036: (a) ADD COLUMN ke `jrnl.gl_status`: `failure_category`, `retry_reason`, `manual_retry_by`, `manual_retry_at`, `manual_retry_reason`, `discarded_by`, `discarded_at`, `discard_reason`, `gl_response_payload_jsonb`; (b) CREATE TABLE `sys.gl_reconciliation_report` + `sys.gl_recon_mismatch`; (c) CREATE TABLE `sys.dlq_gl_delivery` (alias/view dari `jrnl.gl_status WHERE gl_host_status IN ('FAILED','DEAD_LETTER')` atau tabel terpisah — konfirmasi ke data-modeler)
- `integration-engineer` → GL Host REST adapter interface (`internal/jrnl/gldelivery/adapter.go`), circuit breaker config, X-Idempotency-Key contract format per vendor
- `security-engineer` → BLOCKING gate: `GL_DELIVERY.SUCCESS/FAILED` audit in-transaction; `sys.gl_reconciliation_report` tidak di-hard-delete; `jrnl.gl_status` retain 10 tahun; retry endpoint permission enforcement

**Compliance path**: P5-M3 **bukan** ECL/EIR/klasifikasi regulated path → advisory gate saja. Namun security-engineer BLOCKING berlaku karena menyentuh audit trail delivery + PII potential di GL payload.

---

## Konteks & Arsitektur P5-M3

### Flow Keseluruhan
```
P5-M2 jurnal resolver
    → INSERT jrnl.header (status_internal = 'POSTED')
    → Asynq event: "jurnal:posted" diterbitkan (payload: header_id)
                                    ↓
                    P5-M3 Asynq worker "gl_delivery:deliver"
                    ↓
                    Read jrnl.header + jrnl.detail rows
                    ↓
                    Build GL Host payload (REST contract)
                    ↓
                    POST ke GL Host REST endpoint
                    (X-Idempotency-Key = jrnl.header.idempotency_key atau header_id)
                    ↓
              ┌─── 200/201 OK ──────────────────────────────────┐
              │    → jrnl.gl_status.gl_host_status = DELIVERED  │
              │    → gl_host_journal_id = response.journalId    │
              │    → audit GL_DELIVERY.SUCCESS                  │
              └──────────────────────────────────────────────────┘
              ┌─── 4xx (domain error) ──────────────────────────┐
              │    → jrnl.gl_status.gl_host_status = FAILED     │
              │    → failure_category = DOMAIN                  │
              │    → sys.dlq_gl_delivery INSERT                 │
              │    → audit GL_DELIVERY.FAILED                   │
              └──────────────────────────────────────────────────┘
              ┌─── 5xx / timeout (infra error) ─────────────────┐
              │    → retry 3× exponential backoff               │
              │      (1 min → 5 min → 15 min)                  │
              │    → setelah 3× gagal:                          │
              │      gl_host_status = FAILED / DEAD_LETTER      │
              │    → audit GL_DELIVERY.FAILED                   │
              └──────────────────────────────────────────────────┘
```

### Schema Referensi P5-M3

#### `jrnl.gl_status` (existing — migration 000001, hardened 000005)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `header_id` | UUID FK UNIQUE | → `jrnl.header.id` |
| `gl_host_status` | VARCHAR(20) | `PENDING_DELIVERY`, `DELIVERED`, `FAILED`, `RETRYING`, `DEAD_LETTER` |
| `gl_host_journal_id` | VARCHAR(50) | Journal ID dari GL Host (response body) |
| `delivered_at` | TIMESTAMPTZ | Waktu GL Host konfirmasi delivery |
| `retry_count` | INT | Jumlah percobaan infra-retry |
| `last_retry_at` | TIMESTAMPTZ | Timestamp retry terakhir |
| `last_error` | TEXT | Error message terakhir dari GL Host atau infra |
| `delivery_mode` | VARCHAR(20) | `API` (P5-M3), `BATCH_FILE` (legacy Phase 1) |
| `batch_file_id` | VARCHAR(100) | Null untuk Phase 2 REST mode |
| `created_at` | TIMESTAMPTZ | |
| `updated_at` | TIMESTAMPTZ | |

**Kolom tambahan yang dibutuhkan P5-M3** (migration 000036 — data-modeler):
| Kolom | Tipe | Keterangan |
|---|---|---|
| `failure_category` | VARCHAR(20) | `DOMAIN` (4xx) atau `INFRA` (5xx/timeout) |
| `gl_response_payload_jsonb` | JSONB | Raw response dari GL Host (sanitized — no PII) |
| `manual_retry_by` | UUID FK | User yang trigger retry manual |
| `manual_retry_at` | TIMESTAMPTZ | Waktu retry manual |
| `manual_retry_reason` | TEXT | Alasan retry manual (≥ 30 chars) |
| `discarded_by` | UUID FK | User yang discard entry DLQ |
| `discarded_at` | TIMESTAMPTZ | |
| `discard_reason` | TEXT | Alasan discard (≥ 30 chars) |

**Lifecycle `gl_host_status`**:
```
PENDING_DELIVERY → (worker berhasil)  → DELIVERED         (terminal sukses)
PENDING_DELIVERY → (infra error)      → RETRYING          (up to 3×)
RETRYING         → (infra, 3× habis) → FAILED             (DLQ masuk)
PENDING_DELIVERY → (domain 4xx)      → FAILED             (DLQ masuk, no retry)
FAILED           → (manual retry)    → PENDING_DELIVERY   (reset untuk re-attempt)
FAILED           → (discard)         → DEAD_LETTER        (terminal gagal)
DEAD_LETTER      → (tidak bisa berubah — immutable terminal)
```

#### `sys.dlq_gl_delivery` (baru — migration 000036)
View atau tabel terpisah untuk DLQ GL delivery. Lihat Open Question OQ-M3-5a.

Jika tabel terpisah:
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `gl_status_id` | UUID FK UNIQUE | → `jrnl.gl_status.id` |
| `header_id` | UUID FK | → `jrnl.header.id` (denormalisasi untuk query efisien) |
| `failure_category` | VARCHAR(20) | `DOMAIN` atau `INFRA` |
| `error_code` | TEXT | `GL_HOST_REJECTED`, `GL_HOST_UNREACHABLE`, dst |
| `error_message` | TEXT | |
| `payload_snapshot_jsonb` | JSONB | Snapshot payload yang dikirim ke GL Host (tanpa PII) |
| `attempt_count` | INT | |
| `status` | VARCHAR(20) | `FAILED`, `REPLAYING`, `REPLAYED_OK`, `ABANDONED` |
| `replayed_jurnal_delivery_at` | TIMESTAMPTZ | |
| ...audit cols... | — | `created_at/by`, `updated_at/by`, `row_version`, `tenant_id` |

#### `sys.gl_reconciliation_report` (baru — migration 000036)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `tanggal_rekonsiliasi` | DATE NOT NULL UNIQUE | Tanggal laporan (satu per hari) |
| `job_id` | TEXT | → `sys.job.id` (Asynq job yang menghasilkan laporan ini) |
| `status` | VARCHAR(20) | `COMPLETED`, `COMPLETED_WITH_MISMATCH`, `FAILED` |
| `total_akun_checked` | INT | |
| `total_mismatch_count` | INT | |
| `total_mismatch_amount_idr` | NUMERIC(20,4) | |
| `blips_total_idr` | NUMERIC(20,4) | Total BLIPS jrnl per hari tersebut |
| `gl_host_total_idr` | NUMERIC(20,4) | Total GL Host ledger per hari tersebut |
| `delta_idr` | NUMERIC(20,4) | `blips_total - gl_host_total` |
| `tolerance_idr` | NUMERIC(20,4) | Threshold mismatch (default 1.0000 IDR) |
| `summary_jsonb` | JSONB | Per-akun breakdown |
| `gl_host_snapshot_jsonb` | JSONB | Raw GL Host summary response (sanitized) |
| `generated_at` | TIMESTAMPTZ | |
| ...audit cols... | — | |

#### `sys.gl_recon_mismatch` (baru — migration 000036)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `report_id` | UUID FK | → `sys.gl_reconciliation_report.id` |
| `kode_akun` | TEXT | |
| `nama_akun` | TEXT | |
| `blips_amount_idr` | NUMERIC(20,4) | |
| `gl_host_amount_idr` | NUMERIC(20,4) | |
| `delta_idr` | NUMERIC(20,4) | |
| `mismatch_type` | VARCHAR(30) | `BLIPS_ONLY` (ada di BLIPS, tidak di GL), `GL_ONLY`, `AMOUNT_DIFF` |
| `jurnal_header_ids` | UUID[] | Header IDs di BLIPS untuk akun ini pada hari tersebut |
| ...audit cols... | — | |

---

## Story P5-M3-S1 — Auto-Deliver Jurnal ke GL Host

**Actor**: Asynq worker `gl_delivery:deliver` (event-driven, subscribes ke `jurnal:posted`)
**Trigger**: `jrnl.header` baru ter-INSERT dengan `status_internal = 'POSTED'` → P5-M2 menerbitkan event `jurnal:posted` ke Asynq queue
**Goal**: Worker membaca jurnal header + detail, membangun GL Host payload, mengirim via REST POST, dan mengupdate `jrnl.gl_status` sesuai response; idempoten terhadap replay; domain error langsung ke DLQ, infra error di-retry hingga 3×

### Pre-conditions
1. `jrnl.header` memiliki row dengan `status_internal = 'POSTED'` dan `header_id` sesuai event payload
2. `jrnl.detail` berisi minimal 1 DEBIT dan 1 KREDIT row untuk `header_id` tersebut
3. `jrnl.gl_status` row sudah ada dengan `gl_host_status = 'PENDING_DELIVERY'` (auto-INSERT oleh P5-M2 posting service, in-transaction)
4. GL Host endpoint URL dikonfigurasi via `sys.config_param` (`GL_HOST_BASE_URL`, `GL_HOST_API_KEY`) — bukan hardcoded
5. Circuit breaker Asynq worker dalam state CLOSED (normal) atau HALF_OPEN (recovery)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | READ | Header data + narrative |
| `jrnl.detail` | READ | Baris debit/kredit untuk payload |
| `jrnl.gl_status` | UPDATE | Update gl_host_status + response fields |
| `sys.dlq_gl_delivery` | INSERT | Hanya saat domain error (4xx) |
| `sys.config_param` | READ | GL Host URL + auth config |
| `aud.audit_log` | INSERT | `GL_DELIVERY.SUCCESS` atau `GL_DELIVERY.FAILED` dalam transaksi yang sama dengan UPDATE `jrnl.gl_status` |

### GL Host Payload Contract (provisional — pending DEC-031 vendor discovery)
```json
{
  "idempotency_key": "<jrnl.header.idempotency_key>",
  "journal_date": "2026-06-15",
  "reference": "JRN-2026-000001",
  "event_code": "PENEMPATAN",
  "narrative": "Penempatan deposito INST-DEP-001 — ACO 2026",
  "lines": [
    { "account_code": "1110-DEP", "debit": 5000000000.0000, "credit": 0.0000, "currency": "IDR" },
    { "account_code": "1001-KAS", "debit": 0.0000, "credit": 5000000000.0000, "currency": "IDR" }
  ],
  "metadata": {
    "source_system": "BLIPS",
    "instrumen_id": "<uuid>",
    "periode": "2026-06"
  }
}
```

### Permissions
Worker berjalan dengan service account. Tidak ada RBAC check di layer worker. Audit log ditulis dengan `actor_role = 'SYSTEM_WORKER'`.

### Audit Events
| Action | Trigger | Catatan |
|---|---|---|
| `GL_DELIVERY.SUCCESS` | GL Host mengembalikan 200/201, `jrnl.gl_status.gl_host_status = 'DELIVERED'` | In-transaction dengan UPDATE gl_status |
| `GL_DELIVERY.FAILED` | Setelah domain 4xx atau setelah 3× infra retry habis, status `FAILED` | In-transaction dengan UPDATE gl_status |
| `GL_DELIVERY.RETRY` | Setiap percobaan infra retry (status `RETRYING`) | Lightweight, advisory |

### Acceptance Criteria

```gherkin
Feature: Auto-delivery jurnal ke GL Host via Asynq worker

  Background:
    Given jrnl.header JRN-2026-000001 (status_internal = 'POSTED', total_debit = 5000000000.00)
    And jrnl.detail 2 rows: DEBIT 1110-DEP + KREDIT 1001-KAS, masing-masing 5000000000.00
    And jrnl.gl_status untuk JRN-2026-000001: gl_host_status = 'PENDING_DELIVERY'
    And GL Host endpoint: https://glhost.tugu-re.com/api/journals
    And sys.config_param: GL_HOST_BASE_URL, GL_HOST_API_KEY tersedia

  # ─── HAPPY PATH 1: Delivery sukses (GL Host 201) ────────────────────────────

  Scenario: Worker berhasil mengirim jurnal ke GL Host — 201 Created
    When Asynq worker "gl_delivery:deliver" menerima event "jurnal:posted"
      With payload: { "header_id": "<JRN-2026-000001-uuid>" }
    Then worker membaca jrnl.header + jrnl.detail
    And worker membangun GL Host payload dengan X-Idempotency-Key = jrnl.header.idempotency_key
    And worker mengirim POST ke GL Host REST endpoint
    And GL Host mengembalikan HTTP 201:
      { "journalId": "GLHOST-JRN-20260615-00001", "status": "ACCEPTED" }
    Then dalam satu transaksi DB:
      | jrnl.gl_status.gl_host_status      | DELIVERED                      |
      | jrnl.gl_status.gl_host_journal_id  | GLHOST-JRN-20260615-00001      |
      | jrnl.gl_status.delivered_at         | timestamp now                  |
      | jrnl.gl_status.gl_response_payload_jsonb | sanitized response jsonb  |
      | aud.audit_log action                | GL_DELIVERY.SUCCESS            |
      | aud.audit_log entity_id             | jrnl.gl_status.id              |
    And Asynq task di-acknowledge (selesai, tidak di-retry)

  # ─── HAPPY PATH 2: Idempotency — X-Idempotency-Key sudah dikenal GL Host ────

  Scenario: Worker replay event yang sama — GL Host mengembalikan idempotency replay 200
    Given jrnl.gl_status sudah DELIVERED (delivery pertama sukses)
    When Asynq worker menerima replay event "jurnal:posted" untuk header_id yang sama
    Then worker mendeteksi gl_host_status = 'DELIVERED' sebelum memanggil GL Host
    And worker skip pengiriman ulang (early return idempotent)
    And tidak ada UPDATE ke jrnl.gl_status
    And tidak ada entry audit baru
    And Asynq task di-acknowledge

  # ─── ERROR CASE: Domain error (4xx) — langsung DLQ, tidak retry ─────────────

  Scenario: GL Host mengembalikan 422 Unprocessable Entity (invalid payload)
    Given jurnl.gl_status gl_host_status = 'PENDING_DELIVERY'
    When worker mengirim POST ke GL Host
    And GL Host mengembalikan HTTP 422:
      { "error": "INVALID_ACCOUNT_CODE", "message": "Account 1110-DEP not found in GL chart" }
    Then dalam satu transaksi DB:
      | jrnl.gl_status.gl_host_status    | FAILED                         |
      | jrnl.gl_status.failure_category  | DOMAIN                         |
      | jrnl.gl_status.last_error        | "GL_HOST_REJECTED: INVALID_ACCOUNT_CODE — Account 1110-DEP not found in GL chart" |
      | jrnl.gl_status.gl_response_payload_jsonb | sanitized 422 response   |
      | aud.audit_log action              | GL_DELIVERY.FAILED             |
    And INSERT ke sys.dlq_gl_delivery:
      | error_code       | GL_HOST_REJECTED                 |
      | failure_category | DOMAIN                          |
      | payload_snapshot | snapshot payload (tanpa PII)     |
      | status           | FAILED                           |
    And Asynq task di-acknowledge (domain error, tidak di-retry oleh Asynq)
    And notifikasi in-app ke ROLE-AKUN-CTL + ROLE-IT-ADMIN: "GL delivery gagal untuk JRN-2026-000001: invalid account code. Cek DLQ GL Delivery."

  # ─── ERROR CASE: Infra error (5xx/timeout) — 3× exponential backoff → DLQ ───

  Scenario: GL Host 503 Service Unavailable — retry 3× lalu FAILED
    Given jrnl.gl_status gl_host_status = 'PENDING_DELIVERY'
    When worker mencoba POST ke GL Host
    And GL Host mengembalikan HTTP 503 pada attempt ke-1
    Then jrnl.gl_status.gl_host_status = 'RETRYING', retry_count = 1
    And Asynq re-enqueue task dengan delay 1 menit (exponential backoff attempt 1)
    And audit log: GL_DELIVERY.RETRY, attempt = 1

    When retry attempt ke-2 juga mendapat 503
    Then retry_count = 2, delay 5 menit
    And audit log: GL_DELIVERY.RETRY, attempt = 2

    When retry attempt ke-3 juga mendapat 503
    Then dalam satu transaksi DB:
      | jrnl.gl_status.gl_host_status    | FAILED                         |
      | jrnl.gl_status.failure_category  | INFRA                          |
      | jrnl.gl_status.retry_count       | 3                              |
      | aud.audit_log action              | GL_DELIVERY.FAILED             |
    And INSERT ke sys.dlq_gl_delivery:
      | error_code       | GL_HOST_UNREACHABLE              |
      | failure_category | INFRA                           |
      | attempt_count    | 3                               |
    And notifikasi in-app ke ROLE-AKUN-CTL + ROLE-IT-ADMIN

  # ─── EDGE CASE: Periode buku hard-closed antara posting dan delivery ──────────

  Scenario: jrnl.header valid tapi periode buku sudah HARD_CLOSED saat delivery attempt
    Given periode buku Juni-2026 menjadi HARD_CLOSED setelah jrnl.header diposting
    When worker mencoba delivery JRN-2026-000001
    Then worker **tidak** memblok delivery — GL delivery tidak bergantung pada status periode
    And worker mengirimkan payload ke GL Host seperti biasa
    And jika GL Host terima → DELIVERED normal
    And catatan: periode close adalah domain P5-M4, bukan gate delivery GL
```

### Open Questions — Story 1
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M3-1a | GL Host vendor (DEC-031 TBD) — apakah butuh OAuth2/Bearer token, API key header, atau mTLS? | **API key di header `X-API-Key`** sebagai default. OAuth2 client credentials jika GL Host SAP/Oracle. integration-engineer lakukan discovery Sprint 1. |
| OQ-M3-1b | Format X-Idempotency-Key yang diterima GL Host — apakah UUID, atau format tertentu? | **`BLIPS-{jrnl.header.idempotency_key}`** (prefix untuk namespace isolation). Dikonfigurasi via template di `sys.config_param`. Konfirmasi integration-engineer. |
| OQ-M3-1c | Apakah retry backoff intervals (1m/5m/15m) konfigurabel atau hard-coded? | **Konfigurabel via `sys.config_param`**: `GL_DELIVERY_RETRY_DELAYS_SECONDS = "60,300,900"`. Default values sesuai contoh. |
| OQ-M3-1d | Apakah circuit breaker menggunakan library `github.com/sony/gobreaker` atau Hystrix-Go? | **`gobreaker`** sesuai existing pattern. Threshold: 5 failures dalam 60 detik → OPEN. integration-engineer konfirmasi. |

---

## Story P5-M3-S2 — Track Status Delivery per Jurnal Header

**Actor**: ROLE-AKUN, ROLE-AKUN-CTL, ROLE-AUDIT
**Trigger**: User membuka detail jurnal di screen `/jurnal/header/{id}` (P5-M2 S5 screen) dan ingin melihat status pengiriman ke GL Host
**Goal**: Detail jurnal header diperkaya dengan sub-object `delivery_status` yang menampilkan `gl_host_status`, waktu delivery, retry count, response dari GL Host, dan error details jika gagal

### Pre-conditions
1. User ter-autentikasi dengan permission `jurnal.read`
2. `jrnl.gl_status` memiliki row untuk `header_id` yang diminta (auto-created saat posting P5-M2)
3. Jika belum pernah ada delivery attempt: `gl_host_status = 'PENDING_DELIVERY'`

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | READ | Data header utama |
| `jrnl.gl_status` | READ | LEFT JOIN by header_id |

### Enriched GET Response — sub-object `delivery_status`
```json
{
  "data": {
    "id": "<uuid>",
    "no_jurnal": "JRN-2026-000001",
    "event_code": "PENEMPATAN",
    "total_debit": 5000000000.0000,
    "total_kredit": 5000000000.0000,
    "status_internal": "POSTED",
    "detail": [...],
    "delivery_status": {
      "gl_status_id": "<uuid>",
      "gl_host_status": "DELIVERED",
      "gl_host_journal_id": "GLHOST-JRN-20260615-00001",
      "delivered_at": "2026-06-15T10:32:44+07:00",
      "retry_count": 0,
      "last_retry_at": null,
      "last_error": null,
      "failure_category": null,
      "delivery_mode": "API",
      "can_retry": false
    }
  }
}
```

`can_retry` = `true` hanya jika `gl_host_status IN ('FAILED')` DAN actor memiliki permission `jurnal.gl_delivery.retry`.

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal.read` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-RISK | Termasuk delivery_status sub-object |
| `jurnal.gl_delivery.retry` | ROLE-AKUN-CTL, ROLE-IT-ADMIN | Menentukan nilai `can_retry` di response |

### Audit Events
Tidak ada audit event baru untuk read-only endpoint ini.

### Acceptance Criteria

```gherkin
Feature: Status delivery GL Host tampil di detail jurnal

  Background:
    Given user ROLE-AKUN-CTL ter-autentikasi
    And jrnl.header JRN-2026-000001 (status_internal = 'POSTED')

  # ─── HAPPY PATH 1: Jurnal sudah DELIVERED — status tampil lengkap ─────────────

  Scenario: User melihat detail jurnal yang sudah terkirim ke GL Host
    Given jrnl.gl_status untuk JRN-2026-000001:
      | gl_host_status     | DELIVERED                          |
      | gl_host_journal_id | GLHOST-JRN-20260615-00001          |
      | delivered_at       | 2026-06-15T10:32:44+07:00          |
      | retry_count        | 0                                  |
    When user mengakses GET /api/v1/jurnal/header/JRN-2026-000001
    Then response mengandung delivery_status:
      | gl_host_status     | DELIVERED                          |
      | gl_host_journal_id | GLHOST-JRN-20260615-00001          |
      | delivered_at       | 2026-06-15T10:32:44+07:00          |
      | can_retry          | false                              |
    And UI menampilkan badge hijau "Terkirim ke GL" dengan tanggal + GL journal ID

  # ─── HAPPY PATH 2: Jurnal FAILED — delivery_status tampil dengan error + can_retry ──

  Scenario: ROLE-AKUN-CTL melihat jurnal yang gagal terkirim
    Given jrnl.gl_status untuk JRN-2026-000077:
      | gl_host_status     | FAILED                             |
      | failure_category   | DOMAIN                             |
      | last_error         | "GL_HOST_REJECTED: INVALID_ACCOUNT_CODE" |
      | retry_count        | 0                                  |
    When ROLE-AKUN-CTL mengakses GET /api/v1/jurnal/header/JRN-2026-000077
    Then delivery_status mengandung:
      | gl_host_status     | FAILED                             |
      | failure_category   | DOMAIN                             |
      | last_error         | "GL_HOST_REJECTED: INVALID_ACCOUNT_CODE" |
      | can_retry          | true                               |
    And UI menampilkan badge merah "Gagal — Delivery" + tombol "Retry Pengiriman" (jika `can_retry = true`)

  # ─── HAPPY PATH 3: Jurnal PENDING_DELIVERY — masih antri ─────────────────────

  Scenario: Jurnal baru diposting, belum ada delivery attempt
    Given jrnl.gl_status untuk JRN-2026-000099: gl_host_status = 'PENDING_DELIVERY'
    When user mengakses GET /api/v1/jurnal/header/JRN-2026-000099
    Then delivery_status mengandung:
      | gl_host_status | PENDING_DELIVERY                   |
      | can_retry      | false                              |
    And UI menampilkan badge abu-abu "Menunggu Pengiriman"
    And tidak ada tombol retry (can_retry = false)
```

### Open Questions — Story 2
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M3-2a | Apakah `gl_response_payload_jsonb` ditampilkan ke ROLE-AKUN atau hanya ROLE-IT-ADMIN? | **ROLE-IT-ADMIN only** untuk raw response. ROLE-AKUN hanya melihat gl_host_status + gl_host_journal_id + last_error summary. Response API memfilter berdasarkan permission. |
| OQ-M3-2b | Apakah ROLE-AUDIT bisa melihat history delivery attempts (bukan hanya status terakhir)? | **Ya** — ROLE-AUDIT mendapat history via `aud.audit_log` query (action IN ('GL_DELIVERY.SUCCESS', 'GL_DELIVERY.FAILED', 'GL_DELIVERY.RETRY')). Tidak ada tabel history terpisah. |

---

## Story P5-M3-S3 — Manual Retry GL Delivery

**Actor**: ROLE-AKUN-CTL, ROLE-IT-ADMIN
**Trigger**: User membuka detail jurnal yang `gl_host_status = 'FAILED'` dan mengklik tombol "Retry Pengiriman" (atau memanggil endpoint langsung)
**Goal**: Actor dapat me-retry pengiriman jurnal yang gagal ke GL Host, dengan alasan yang jelas, audit trail, dan idempotency; retry me-reset status ke `PENDING_DELIVERY` dan enqueue Asynq task baru

### Pre-conditions
1. User ter-autentikasi dengan permission `jurnal.gl_delivery.retry`
2. `jrnl.gl_status.gl_host_status` HARUS `IN ('FAILED')` — status `DEAD_LETTER` tidak bisa di-retry (discard terminal)
3. `jrnl.header.status_internal = 'POSTED'` (jurnal tetap valid)
4. Request mengandung `Idempotency-Key` header (UUID v4)
5. Field `reason` wajib, minimal 30 karakter

### Endpoint
```
POST /api/v1/jurnal/header/{id}/retry-gl-delivery
Authorization: Bearer <jwt>
Idempotency-Key: <uuid-v4>

Body:
{
  "reason": "Kode akun 1110-DEP sudah diperbaiki di GL Host Chart of Accounts. Retry delivery."
}

→ 202 Accepted
{
  "data": {
    "jobId": "job_GL_RETRY_01HXYZ...",
    "statusUrl": "/api/v1/jobs/job_GL_RETRY_01HXYZ...",
    "gl_status_id": "<uuid>",
    "previous_status": "FAILED",
    "new_status": "PENDING_DELIVERY"
  }
}
```

### State Transition
```
FAILED → (manual retry accepted) → PENDING_DELIVERY (reset)
      → Asynq task "gl_delivery:deliver" di-enqueue dengan header_id
      → Asynq worker mengeksekusi → DELIVERED atau FAILED lagi
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.gl_status` | UPDATE | Reset gl_host_status = 'PENDING_DELIVERY', retry_count tidak di-reset (incremental) |
| `jrnl.header` | READ | Validasi status_internal = 'POSTED' |
| `sys.job` | INSERT | Job tracking untuk Asynq task retry |
| `aud.audit_log` | INSERT | `GL_DELIVERY.MANUAL_RETRY_INITIATED` sebelum enqueue |

### Permissions
| Permission | Role | MFA | Catatan |
|---|---|---|---|
| `jurnal.gl_delivery.retry` | ROLE-AKUN-CTL | Tidak | |
| `jurnal.gl_delivery.retry` | ROLE-IT-ADMIN | Tidak | |

### Audit Events
| Action | Trigger |
|---|---|
| `GL_DELIVERY.MANUAL_RETRY_INITIATED` | Saat request diterima dan validated — sebelum Asynq task di-enqueue |
| `GL_DELIVERY.SUCCESS` | Saat Asynq worker berhasil deliver (dari Story 1) |
| `GL_DELIVERY.FAILED` | Saat Asynq worker gagal lagi (dari Story 1) |

### Acceptance Criteria

```gherkin
Feature: Manual retry GL delivery oleh ROLE-AKUN-CTL / ROLE-IT-ADMIN

  Background:
    Given jrnl.header JRN-2026-000077 (status_internal = 'POSTED')
    And jrnl.gl_status: gl_host_status = 'FAILED', failure_category = 'DOMAIN'
    And user ROLE-AKUN-CTL (USR-011) ter-autentikasi

  # ─── HAPPY PATH 1: Retry diterima, status reset ke PENDING_DELIVERY ───────────

  Scenario: ROLE-AKUN-CTL berhasil memicu retry delivery
    When USR-011 mengirim POST /api/v1/jurnal/header/JRN-2026-000077/retry-gl-delivery
      With Idempotency-Key: IK-RETRY-001
      With body: { "reason": "Kode akun 1110-DEP sudah diperbaiki di GL Host Chart of Accounts. Retry delivery." }
    Then sistem mengembalikan HTTP 202:
      | jobId           | job_GL_RETRY_01HXYZ...             |
      | previous_status | FAILED                             |
      | new_status      | PENDING_DELIVERY                   |
    And dalam satu transaksi DB:
      | jrnl.gl_status.gl_host_status    | PENDING_DELIVERY              |
      | jrnl.gl_status.manual_retry_by   | USR-011 UUID                  |
      | jrnl.gl_status.manual_retry_at   | timestamp now                 |
      | jrnl.gl_status.manual_retry_reason | "Kode akun 1110-DEP sudah diperbaiki..." |
      | aud.audit_log action              | GL_DELIVERY.MANUAL_RETRY_INITIATED |
      | aud.audit_log actor_user_id       | USR-011 UUID                  |
    And sys.job ter-INSERT untuk tracking
    And Asynq task "gl_delivery:deliver" di-enqueue dengan header_id
    And toast ke USR-011: "Retry delivery JRN-2026-000077 berhasil dijadwalkan. Pantau status di panel jurnal."

  # ─── ERROR CASE: Retry ditolak karena status DEAD_LETTER ─────────────────────

  Scenario: User mencoba retry jurnal yang sudah di-abandon (DEAD_LETTER)
    Given jrnl.gl_status untuk JRN-2026-000055: gl_host_status = 'DEAD_LETTER'
    When USR-011 mengirim POST /api/v1/jurnal/header/JRN-2026-000055/retry-gl-delivery
      With body: { "reason": "Coba retry ulang yang sudah diabaikan sebelumnya." }
    Then sistem mengembalikan HTTP 422:
      | error.code    | WORKFLOW_INVALID_TRANSITION                                |
      | error.message | "Jurnal JRN-2026-000055 memiliki status DEAD_LETTER dan tidak dapat di-retry. Hubungi ROLE-IT-ADMIN jika pengiriman ini masih diperlukan." |
    And tidak ada perubahan pada jrnl.gl_status
    And tidak ada Asynq task yang di-enqueue

  # ─── ERROR CASE: Alasan retry terlalu pendek ─────────────────────────────────

  Scenario: Request retry ditolak karena reason kurang dari 30 karakter
    Given jrnl.gl_status: gl_host_status = 'FAILED'
    When USR-011 mengirim POST /api/v1/jurnal/header/{id}/retry-gl-delivery
      With body: { "reason": "Sudah diperbaiki." }
    Then sistem mengembalikan HTTP 400:
      | error.code            | VALIDATION_FAILED                  |
      | error.details[0].field| reason                             |
      | error.details[0].rule | "min_length:30 — alasan retry wajib minimal 30 karakter" |
    And tidak ada perubahan state

  # ─── ERROR CASE: Permission denied — ROLE-AKUN tidak bisa retry ──────────────

  Scenario: ROLE-AKUN mencoba trigger manual retry — permission denied
    Given user ROLE-AKUN (USR-010) ter-autentikasi
    When USR-010 mengirim POST /api/v1/jurnal/header/{id}/retry-gl-delivery
      With body: { "reason": "Mencoba retry tapi tidak punya permission jurnal.gl_delivery.retry." }
    Then sistem mengembalikan HTTP 403:
      | error.code    | FORBIDDEN                                                 |
      | error.message | "Tidak memiliki permission jurnal.gl_delivery.retry."     |
```

### Open Questions — Story 3
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M3-3a | Apakah ada batas maksimum total manual retry per jurnal header? | **Max 5 retry total** (auto + manual). Setelah 5, harus DEAD_LETTER. Konfigurabel via `sys.config_param` `GL_DELIVERY_MAX_TOTAL_ATTEMPTS`. |
| OQ-M3-3b | Apakah ROLE-IT-ADMIN perlu MFA step-up untuk retry? | **Tidak** — manual retry bukan operasi destructive. MFA step-up hanya berlaku untuk hard-close (DEC-027). Konfirmasi ke security-engineer. |

---

## Story P5-M3-S4 — Daily Reconciliation Report

**Actor**: Asynq cron job `gl_recon:daily` (system) + Read actors: ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT
**Trigger 1 (auto)**: Asynq cron `gl_recon:daily` berjalan setiap hari pukul 08:00 WIB, memproses rekonsiliasi untuk hari kerja sebelumnya (D-1)
**Trigger 2 (manual)**: ROLE-AKUN-CTL meminta rerun rekonsiliasi via `POST /api/v1/jurnal/reconciliation/daily/run?date=YYYY-MM-DD`
**Goal**: Sistem membandingkan total per-akun di BLIPS `jrnl.header/detail` dengan ledger summary yang di-fetch dari GL Host, mempersist laporan ke `sys.gl_reconciliation_report`, dan memflag setiap selisih > 1 IDR dengan detail per akun; ROLE-AKUN-CTL, ROLE-CFO, dan ROLE-AUDIT dapat melihat laporan via REST endpoint

### Pre-conditions
1. GL Host menyediakan endpoint summary harian: `GET /api/gl/daily-summary?date=YYYY-MM-DD` (konfirmasi per DEC-031)
2. `jrnl.header` rows untuk tanggal tersebut sudah POSTED (dan delivery status ada — tidak semua harus DELIVERED untuk rekon berjalan)
3. Tanggal rekonsiliasi adalah hari kerja (bukan hari libur nasional — menggunakan `sys.calendar_holiday`)
4. `sys.gl_reconciliation_report` belum ada row untuk tanggal tersebut (idempotent: jika sudah ada, overwrite atau return existing — konfirmasi OQ-M3-4b)
5. FX rate JISDOR untuk tanggal tersebut sudah tersedia di `mst.kurs` (dipakai P5-M5) untuk konversi FCY ke IDR

### Reconciliation Logic
```
Untuk setiap tanggal D:

1. BLIPS side:
   SELECT kode_akun, SUM(debit_amount - kredit_amount) AS blips_net_idr
   FROM jrnl.detail d
   JOIN jrnl.header h ON d.header_id = h.id
   WHERE h.tanggal_posting = D
     AND h.status_internal = 'POSTED'
   GROUP BY kode_akun

2. GL Host side:
   GET {GL_HOST_BASE_URL}/api/gl/daily-summary?date=D
   → { "accounts": [{ "account_code": "1110-DEP", "net_amount": 5000000000.00 }, ...] }

3. Comparison:
   For each account in UNION(BLIPS accounts, GL Host accounts):
     delta = blips_net_idr − gl_host_net_idr
     if ABS(delta) > tolerance (default 1.0000 IDR):
       → mismatch_type = BLIPS_ONLY | GL_ONLY | AMOUNT_DIFF
       → INSERT sys.gl_recon_mismatch

4. Summary:
   INSERT sys.gl_reconciliation_report dengan status:
     COMPLETED             jika total_mismatch_count = 0
     COMPLETED_WITH_MISMATCH jika ada mismatch
     FAILED                jika GL Host tidak bisa di-reach
```

### Endpoint Read
```
GET /api/v1/jurnal/reconciliation/daily?date=2026-06-14

→ 200 OK
{
  "data": {
    "tanggal_rekonsiliasi": "2026-06-14",
    "status": "COMPLETED_WITH_MISMATCH",
    "total_akun_checked": 28,
    "total_mismatch_count": 2,
    "total_mismatch_amount_idr": 15000.0000,
    "blips_total_idr": 12500000000.0000,
    "gl_host_total_idr": 12499985000.0000,
    "delta_idr": 15000.0000,
    "tolerance_idr": 1.0000,
    "mismatches": [
      {
        "kode_akun": "3010-OCI-AST",
        "nama_akun": "Aset OCI",
        "blips_amount_idr": 1000000.0000,
        "gl_host_amount_idr": 0.0000,
        "delta_idr": 1000000.0000,
        "mismatch_type": "BLIPS_ONLY",
        "jurnal_header_ids": ["<uuid1>", "<uuid2>"]
      },
      ...
    ],
    "generated_at": "2026-06-15T08:05:12+07:00",
    "job_id": "job_RECON_01HXYZ..."
  }
}
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | READ | Filter by tanggal_posting + status_internal = 'POSTED' |
| `jrnl.detail` | READ | Aggregate per kode_akun |
| `sys.gl_reconciliation_report` | INSERT / READ | One row per tanggal |
| `sys.gl_recon_mismatch` | INSERT / READ | Detail mismatch per akun |
| `sys.job` | INSERT / UPDATE | Progress tracking (§3 UX) |
| `sys.calendar_holiday` | READ | Skip rekon hari libur |
| `mst.chart_of_accounts` | READ | Nama akun untuk mismatch detail |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal.reconciliation.read` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Lihat laporan |
| `jurnal.reconciliation.run` | ROLE-AKUN-CTL | Manual trigger rerun |

### Audit Events
| Action | Trigger |
|---|---|
| `GL_RECONCILIATION.STARTED` | Cron atau manual trigger dimulai |
| `GL_RECONCILIATION.COMPLETED` | Laporan selesai — in-transaction dengan INSERT `sys.gl_reconciliation_report` |
| `GL_RECONCILIATION.FAILED` | Gagal fetch GL Host atau query error |

### Acceptance Criteria

```gherkin
Feature: Daily reconciliation jurnal BLIPS vs GL Host

  Background:
    Given tanggal rekonsiliasi target: 2026-06-14 (hari kerja)
    And jrnl.header 50 rows POSTED pada tanggal 2026-06-14, total debit = IDR 12.500.000.000
    And GL Host endpoint tersedia: GET /api/gl/daily-summary?date=2026-06-14

  # ─── HAPPY PATH 1: Rekonsiliasi berhasil — tidak ada mismatch ────────────────

  Scenario: Cron berjalan, BLIPS dan GL Host sesuai — COMPLETED
    Given GL Host mengembalikan ledger summary yang identik dengan BLIPS aggregate
    When Asynq cron "gl_recon:daily" dieksekusi pukul 08:00 WIB untuk D-1 = 2026-06-14
    Then sys.job ter-INSERT dengan status = 'running'
    And sistem query jrnl.detail untuk tanggal 2026-06-14 → aggregate 28 akun
    And sistem GET GL Host daily-summary?date=2026-06-14 → 28 akun, amount identik
    And comparison: delta = 0 untuk semua akun (< tolerance 1.0000 IDR)
    And dalam satu transaksi:
      | sys.gl_reconciliation_report.status               | COMPLETED         |
      | sys.gl_reconciliation_report.total_mismatch_count | 0                 |
      | sys.gl_reconciliation_report.delta_idr             | 0.0000            |
      | aud.audit_log action                               | GL_RECONCILIATION.COMPLETED |
    And sys.job status = 'completed'
    And toast notifikasi ke ROLE-AKUN-CTL: "Rekonsiliasi GL 2026-06-14 selesai — tidak ada mismatch."

  # ─── HAPPY PATH 2: Rekonsiliasi menemukan mismatch — COMPLETED_WITH_MISMATCH ──

  Scenario: Rekonsiliasi menemukan 2 akun mismatch
    Given GL Host daily-summary?date=2026-06-14 mengembalikan total IDR 12.499.985.000
    And akun "3010-OCI-AST" ada di BLIPS tapi tidak ada di GL Host (BLIPS_ONLY)
    And akun "1210-OBLIGASI" memiliki selisih IDR 14.000 (AMOUNT_DIFF)
    When cron "gl_recon:daily" berjalan
    Then sys.gl_reconciliation_report:
      | status                    | COMPLETED_WITH_MISMATCH           |
      | total_mismatch_count      | 2                                 |
      | total_mismatch_amount_idr | 15000.0000                        |
    And sys.gl_recon_mismatch ter-INSERT 2 rows:
      | Row 1: kode_akun = "3010-OCI-AST", mismatch_type = "BLIPS_ONLY", delta_idr = 1000000.0000 |
      | Row 2: kode_akun = "1210-OBLIGASI", mismatch_type = "AMOUNT_DIFF", delta_idr = 14000.0000 |
    And audit log: GL_RECONCILIATION.COMPLETED, after_jsonb: { mismatch_count: 2, total_mismatch_idr: 15000 }
    And notifikasi ALERT ke ROLE-AKUN-CTL + ROLE-CFO: "MISMATCH GL Rekonsiliasi 2026-06-14: 2 akun, total selisih IDR 15.000. Tindak lanjut diperlukan."

  # ─── HAPPY PATH 3: ROLE-AKUN-CTL membaca laporan rekonsiliasi ────────────────

  Scenario: ROLE-AKUN-CTL membuka laporan rekonsiliasi
    Given sys.gl_reconciliation_report tersedia untuk 2026-06-14
    When ROLE-AKUN-CTL mengakses GET /api/v1/jurnal/reconciliation/daily?date=2026-06-14
    Then HTTP 200 dengan:
      | status                    | COMPLETED_WITH_MISMATCH           |
      | total_mismatch_count      | 2                                 |
      | mismatches[0].kode_akun   | 3010-OCI-AST                      |
      | mismatches[0].mismatch_type | BLIPS_ONLY                      |
    And ROLE-AUDIT bisa akses endpoint yang sama (permission `jurnal.reconciliation.read`)
    And ROLE-AKUN tanpa permission → HTTP 403 FORBIDDEN

  # ─── ERROR CASE: GL Host tidak bisa di-reach saat rekon ─────────────────────

  Scenario: GL Host 503 saat cron berjalan — rekon FAILED, tidak ada INSERT mismatch
    Given GL Host mengembalikan HTTP 503 saat GET daily-summary
    When cron "gl_recon:daily" berjalan untuk 2026-06-14
    Then sys.gl_reconciliation_report:
      | status    | FAILED                             |
      | summary_jsonb | { "error": "GL Host unreachable: 503" } |
    And tidak ada rows di sys.gl_recon_mismatch untuk tanggal ini
    And audit log: GL_RECONCILIATION.FAILED
    And sys.job status = 'failed'
    And alert ke ROLE-AKUN-CTL + ROLE-IT-ADMIN: "Rekonsiliasi GL 2026-06-14 GAGAL — GL Host tidak tersedia. Rerun manual diperlukan."
```

### Open Questions — Story 4
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M3-4a | GL Host daily summary API contract — apakah GL Host punya endpoint summary per hari? Format JSON atau XML? | **TBD per DEC-031**. Asumsi JSON REST `GET /api/gl/daily-summary?date=YYYY-MM-DD`. Jika GL Host tidak punya → alternatif: BLIPS download full transaction list dan aggregate sendiri. integration-engineer konfirmasi. |
| OQ-M3-4b | Jika cron dijalankan dua kali untuk hari yang sama (manual rerun) — apakah laporan di-overwrite atau duplikat? | **Overwrite (UPSERT)** — unique constraint `tanggal_rekonsiliasi`. Row `sys.gl_recon_mismatch` lama di-soft-delete, laporan baru di-INSERT. Konfirmasi ke data-modeler. |
| OQ-M3-4c | Tolerance IDR 1.0000 — apakah konfigurabel per akun atau global? | **Global, konfigurabel via `sys.config_param` `GL_RECON_TOLERANCE_IDR`**, default = 1.0000. Per-akun tolerance adalah Phase 6 feature request. |
| OQ-M3-4d | Apakah rekonsiliasi berjalan juga untuk hari libur (mis. jurnal dikerjakan hari kerja, rekon diakumulasi)? | **Skip hari libur** (check `sys.calendar_holiday`). Hari Senin: rekon mencakup Jumat, Sabtu, Minggu sekaligus. Rekonsiliasi per-tanggal (3 laporan terpisah). Konfirmasi ke Kepala Akuntansi. |

---

## Story P5-M3-S5 — GL Delivery DLQ Console

**Actor**: ROLE-AKUN-CTL, ROLE-IT-ADMIN
**Trigger**: ROLE-AKUN-CTL atau ROLE-IT-ADMIN membuka halaman `/jurnal/gl-delivery-dlq` untuk melihat jurnal yang gagal dikirim ke GL Host (status `FAILED`)
**Goal**: Actor dapat melihat daftar delivery failure (terpisah dari DLQ jurnal posting P5-M2), memahami penyebab kegagalan, memilih untuk re-attempt pengiriman ke GL Host (replay) atau mengabaikan (discard/DEAD_LETTER), dengan audit trail lengkap

### Perbedaan dengan P5-M2 DLQ
| Aspek | P5-M2 `sys.dlq_jurnal_post` | P5-M3 GL Delivery DLQ |
|---|---|---|
| Kegagalan yang dicatat | Gagal **memposting** jurnal ke `jrnl.header` | Gagal **mengirim** jurnal yang sudah POSTED ke GL Host |
| Tabel sumber | `sys.dlq_jurnal_post` | `jrnl.gl_status WHERE gl_host_status = 'FAILED'` |
| Aksi replay | Enqueue Asynq task posting jurnal ulang | Enqueue Asynq task re-POST ke GL Host |
| Endpoint | `GET /api/v1/jurnal/dlq` | `GET /api/v1/jurnal/gl-delivery-dlq` |

### Endpoints
```
GET  /api/v1/jurnal/gl-delivery-dlq
     ?filter[gl_host_status]=FAILED
     &filter[failure_category]=DOMAIN
     &cursor=...&limit=50&sort=last_retry_at:desc

POST /api/v1/jurnal/gl-delivery-dlq/{gl_status_id}/replay
     Body: { "reason": "Reason min 30 chars..." }
     Idempotency-Key: <uuid>

POST /api/v1/jurnal/gl-delivery-dlq/{gl_status_id}/discard
     Body: { "reason": "Reason min 30 chars..." }
     Idempotency-Key: <uuid>
```

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.gl_status` | READ (list) / UPDATE (replay/discard) | Source data untuk DLQ console |
| `jrnl.header` | READ | JOIN untuk no_jurnal, event_code, tanggal_posting |
| `sys.dlq_gl_delivery` | READ / UPDATE | Jika terpisah dari jrnl.gl_status (lihat OQ-M3-5a) |
| `sys.job` | INSERT | Tracking progress replay Asynq task |
| `aud.audit_log` | INSERT | Setiap replay/discard action |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal.gl_delivery.read` | ROLE-AKUN-CTL, ROLE-IT-ADMIN, ROLE-AUDIT | Read-only view DLQ |
| `jurnal.gl_delivery.replay` | ROLE-AKUN-CTL, ROLE-IT-ADMIN | Trigger replay ke GL Host |
| `jurnal.gl_delivery.discard` | ROLE-IT-ADMIN | Discard (DEAD_LETTER) — hanya IT Admin |

### Audit Events
| Action | Trigger |
|---|---|
| `GL_DELIVERY.DLQ_REPLAY_INITIATED` | User trigger replay dari DLQ console |
| `GL_DELIVERY.DLQ_DISCARDED` | User discard entry (status → DEAD_LETTER) |
| `GL_DELIVERY.SUCCESS` | Jika replay berhasil (dari Asynq worker — Story 1) |
| `GL_DELIVERY.FAILED` | Jika replay gagal lagi (dari Asynq worker — Story 1) |

### Acceptance Criteria

```gherkin
Feature: GL Delivery DLQ console — inspect, replay, discard

  Background:
    Given 8 entries di jrnl.gl_status dengan gl_host_status = 'FAILED':
      | header_id     | no_jurnal         | failure_category | error_code            | retry_count |
      | GL-STS-001    | JRN-2026-000077   | DOMAIN           | GL_HOST_REJECTED      | 0           |
      | GL-STS-002    | JRN-2026-000088   | INFRA            | GL_HOST_UNREACHABLE   | 3           |
      | GL-STS-003..8 | ...               | ...              | ...                   | ...         |
    And user ter-autentikasi sebagai ROLE-IT-ADMIN (USR-030)

  # ─── HAPPY PATH 1: Lihat DLQ list dengan filter + sort ────────────────────────

  Scenario: ROLE-IT-ADMIN membuka DLQ GL delivery list
    When user mengakses GET /api/v1/jurnal/gl-delivery-dlq?filter[gl_host_status]=FAILED&sort=last_retry_at:desc&limit=50
    Then HTTP 200 dengan 8 entries
    And setiap entry mengandung:
      | no_jurnal, event_code, tanggal_posting, gl_host_status, failure_category, error_code, retry_count, last_retry_at |
    And UI menampilkan:
      | Badge DOMAIN merah untuk GL_HOST_REJECTED                                            |
      | Badge INFRA amber untuk GL_HOST_UNREACHABLE                                          |
      | Kolom retry_count untuk visibility cepat                                              |
      | Tombol "Replay" per row (jika permission `jurnal.gl_delivery.replay`)                 |
      | Tombol "Discard" per row (jika permission `jurnal.gl_delivery.discard`)               |
    And filter chip: "Status: FAILED" aktif di UI
    And cursor pagination: Prev/Next + "8 of 8"
    And export CSV/XLSX tersedia (audit `GL_DELIVERY.DLQ_EXPORT`)

  # ─── HAPPY PATH 2: Replay dari DLQ — re-attempt POST ke GL Host ───────────────

  Scenario: ROLE-IT-ADMIN replay GL-STS-002 setelah GL Host pulih
    Given GL Host sudah kembali online
    When USR-030 mengirim POST /api/v1/jurnal/gl-delivery-dlq/GL-STS-002/replay
      With Idempotency-Key: IK-DLQ-REPLAY-002
      With body: { "reason": "GL Host sudah pulih setelah maintenance window 2026-06-14 22:00-02:00 WIB." }
    Then HTTP 202:
      | jobId       | job_GL_RETRY_DLQ_002                |
      | previous_status | FAILED                          |
      | new_status  | PENDING_DELIVERY                    |
    And dalam satu transaksi:
      | jrnl.gl_status.gl_host_status       | PENDING_DELIVERY              |
      | jrnl.gl_status.manual_retry_by      | USR-030 UUID                  |
      | jrnl.gl_status.manual_retry_reason  | "GL Host sudah pulih..."      |
      | aud.audit_log action                 | GL_DELIVERY.DLQ_REPLAY_INITIATED |
    And Asynq task "gl_delivery:deliver" di-enqueue (fresh idempotency_key = UUID baru, bukan yang lama)
    And ketika worker berhasil:
      | jrnl.gl_status.gl_host_status | DELIVERED                      |
    And toast ke USR-030: "Replay GL-STS-002 (JRN-2026-000088) berhasil. GL Host journal ID: GLHOST-JRN-20260615-00088."

  # ─── HAPPY PATH 3: Discard entry DLQ — status DEAD_LETTER ────────────────────

  Scenario: ROLE-IT-ADMIN mendiscard GL-STS-001 (jurnal sudah diinput manual di GL Host)
    When USR-030 mengirim POST /api/v1/jurnal/gl-delivery-dlq/GL-STS-001/discard
      With Idempotency-Key: IK-DLQ-DISCARD-001
      With body: { "reason": "Jurnal JRN-2026-000077 sudah diinput manual di GL Host pada 2026-06-14 oleh tim Akuntansi. Delivery otomatis tidak diperlukan." }
    Then HTTP 200:
      | gl_status_id | GL-STS-001                        |
      | new_status   | DEAD_LETTER                       |
    And dalam satu transaksi:
      | jrnl.gl_status.gl_host_status | DEAD_LETTER                   |
      | jrnl.gl_status.discarded_by   | USR-030 UUID                  |
      | jrnl.gl_status.discarded_at   | timestamp now                 |
      | jrnl.gl_status.discard_reason | "Jurnal JRN-2026-000077 sudah diinput manual..." |
      | aud.audit_log action           | GL_DELIVERY.DLQ_DISCARDED     |
    And entry TIDAK muncul di default DLQ list (filter `FAILED` tidak include DEAD_LETTER)
    And ROLE-AUDIT masih bisa melihat via `?filter[gl_host_status]=DEAD_LETTER`
    And toast ke USR-030: "JRN-2026-000077 berhasil di-discard. Status: DEAD_LETTER. Entry tersimpan di audit trail."

  # ─── ERROR CASE: ROLE-AKUN-CTL mencoba discard — permission denied ────────────

  Scenario: ROLE-AKUN-CTL mencoba discard entry DLQ — hanya ROLE-IT-ADMIN yang bisa
    Given user ROLE-AKUN-CTL (USR-011) ter-autentikasi
    When USR-011 mengirim POST /api/v1/jurnal/gl-delivery-dlq/GL-STS-003/discard
      With body: { "reason": "Ingin mendiscard jurnal ini karena sudah tidak relevan lagi diposting." }
    Then HTTP 403:
      | error.code    | FORBIDDEN                                                 |
      | error.message | "Tidak memiliki permission jurnal.gl_delivery.discard. Hanya ROLE-IT-ADMIN yang dapat mendiscard entry GL Delivery DLQ." |
    And tidak ada perubahan pada jrnl.gl_status
```

### Open Questions — Story 5
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M3-5a | Apakah DLQ GL delivery adalah view dari `jrnl.gl_status WHERE gl_host_status IN ('FAILED')` atau tabel terpisah `sys.dlq_gl_delivery`? | **Rekomendasi**: view/query dari `jrnl.gl_status` + `jrnl.header` JOIN (simpler, single source of truth). `sys.dlq_gl_delivery` tabel terpisah hanya jika butuh additional columns yang tidak cocok di `jrnl.gl_status`. Keputusan akhir ke data-modeler. |
| OQ-M3-5b | Replay dari DLQ console vs Story 3 `retry-gl-delivery` endpoint — apakah sama atau berbeda code path? | **Code path yang sama** — Story 3 retry endpoint adalah shortcut single-header, DLQ console Story 5 adalah bulk interface. Keduanya memanggil `GLDeliveryService.Retry(headerID, reason, actorID)` yang sama. |
| OQ-M3-5c | Apakah ada "bulk replay" untuk semua FAILED entries sekaligus? | **Tidak di P5-M3** — bulk replay berisiko jika GL Host masih bermasalah. Manual per-entry atau filtered batch di Phase 6. |

---

## Ringkasan P5-M3 Story Set

| Story | Judul | Actor Utama | AC Count | Workflow | Gate |
|---|---|---|---|---|---|
| P5-M3-S1 | Auto-deliver jurnal ke GL Host | Asynq worker (system) | 5 | Event-driven, retry, DLQ | security BLOCKING (audit in-tx) |
| P5-M3-S2 | Track delivery status per jurnal | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-AUDIT | 3 | Read-only | advisory |
| P5-M3-S3 | Manual retry GL delivery | ROLE-AKUN-CTL, ROLE-IT-ADMIN | 4 | Reset FAILED → PENDING_DELIVERY | security BLOCKING |
| P5-M3-S4 | Daily reconciliation report | Asynq cron + ROLE-AKUN-CTL/CFO/AUDIT | 4 | Cron job, sys.job progress | advisory |
| P5-M3-S5 | GL Delivery DLQ console | ROLE-AKUN-CTL, ROLE-IT-ADMIN | 4 | Replay / Discard | security BLOCKING (audit DEAD_LETTER) |
| **Total** | | | **20** | | |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `jrnl.header` status POSTED | P5-M2 → P5-M3 | P5-M3 mengkonsumsi event `jurnal:posted` dari P5-M2 |
| `jrnl.gl_status` row per header | P5-M2 (in-transaction) → P5-M3 | P5-M2 posting service INSERT `jrnl.gl_status` dengan `PENDING_DELIVERY` di transaksi yang sama dengan INSERT `jrnl.header` |
| `sys.calendar_holiday` | P5-M5 atau existing → P5-M3 | Rekonsiliasi skip hari libur |
| `mst.kurs` (FX rate) | P5-M5 → P5-M3 | Konversi FCY jika jurnal multi-currency saat rekonsiliasi |
| GL delivery status | P5-M3 → P5-M4 | P5-M4 periode close pre-condition: tidak boleh ada `gl_host_status = 'FAILED'` yang belum resolved untuk periode yang akan ditutup |
| Reporting | P5-M3 → P5-M13/M14 | RPT: GL delivery status summary, mismatch history |

---

## Compliance & Security Handoff Checklist

### Untuk security-engineer (BLOCKING gate)
- [ ] `GL_DELIVERY.SUCCESS` dan `GL_DELIVERY.FAILED` ditulis ke `aud.audit_log` **in-transaction** dengan UPDATE `jrnl.gl_status` (atomik — jangan async)
- [ ] `sys.gl_reconciliation_report` dan `sys.gl_recon_mismatch` tidak boleh di-hard-delete (append-only via trigger, seperti `jrnl.*`)
- [ ] `jrnl.gl_status` tidak boleh di-hard-delete; update hanya di-allow untuk kolom status/delivery, bukan data jurnal
- [ ] `gl_response_payload_jsonb` dan `payload_snapshot_jsonb` wajib di-sanitize sebelum INSERT: strip field yang mengandung PII (nomor_rekening, NPWP, nama_nasabah dari metadata instrumen)
- [ ] Endpoint `retry-gl-delivery` dan DLQ `replay`/`discard` cek permission server-side — bukan hanya UI disable
- [ ] `sys.config_param` `GL_HOST_API_KEY` tidak boleh di-log, tidak boleh di-return via API, tidak boleh masuk `gl_response_payload_jsonb`
- [ ] Audit `GL_DELIVERY.MANUAL_RETRY_INITIATED` ditulis **sebelum** Asynq task di-enqueue (bukan setelah) — jika enqueue gagal, audit tetap ada sebagai evidence
- [ ] Audit `GL_DELIVERY.DLQ_DISCARDED` mencatat `discard_reason` di `after_jsonb` (minimal evidence untuk DEAD_LETTER decision)
- [ ] Retry endpoint: `reason` field di-sanitize — tidak boleh mengandung credential/secret

### Untuk integration-engineer
- [ ] GL Host adapter interface `GLHostAdapter` di `internal/jrnl/gldelivery/adapter.go` — implementasi mock untuk testing + real adapter saat vendor confirmed (DEC-031)
- [ ] Circuit breaker config: threshold, timeout, HALF_OPEN recovery window
- [ ] X-Idempotency-Key format final (OQ-M3-1b) — konfirmasi dengan GL Host vendor
- [ ] GL Host daily summary endpoint (OQ-M3-4a) — konfirmasi contract dan format

### Untuk data-modeler
- [ ] Migration 000036: ADD COLUMNS ke `jrnl.gl_status` (P5-M3 delivery tracking columns)
- [ ] Migration 000036: CREATE TABLE `sys.gl_reconciliation_report` + `sys.gl_recon_mismatch`
- [ ] Migration 000036: Putuskan `sys.dlq_gl_delivery` = view atau tabel terpisah (OQ-M3-5a)
- [ ] `sys.gl_reconciliation_report` UNIQUE constraint pada `tanggal_rekonsiliasi`
- [ ] `jrnl.gl_status` — pastikan BEFORE DELETE trigger sudah ada (dari migration 000005) atau tambahkan BEFORE UPDATE yang hanya allow kolom-kolom delivery status
- [ ] Index: `jrnl.gl_status (gl_host_status, updated_at DESC)` untuk DLQ query performance
- [ ] `sys.gl_recon_mismatch` tidak boleh di-hard-delete; append-only trigger

### Confirmed decisions P5-M3
- **DEC-030 RESOLVED**: GL delivery mode = **Async REST via Asynq** (telah dikonfirmasi di phase-5-roadmap.md)
- **DEC-031 PENDING**: GL Host vendor — integration-engineer lakukan discovery Sprint 1. P5-M3 menggunakan adapter interface pattern untuk isolasi
- Retry policy: **domain 4xx → DLQ immediate (no retry)** / **infra 5xx/timeout → 3× exponential backoff (1m/5m/15m)** → DLQ
- DEAD_LETTER: hanya via explicit `discard` action oleh ROLE-IT-ADMIN (bukan auto dari max retries)

---

_Story set ini siap dihandoff ke `system-analyst` untuk OpenAPI contract + state machine, dan ke `data-modeler` untuk migration 000036. Tunggu jawaban OQ-M3-1a (GL Host vendor auth) sebelum finalizing adapter contract. DEC-031 harus resolved paling lambat awal Sprint 2._
