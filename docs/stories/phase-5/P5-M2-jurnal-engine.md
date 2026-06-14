# P5-M2 — Jurnal Event Resolver & Posting Engine: User Stories

**Story Set ID**: P5-M2
**Modul**: APP-D — Periode Buku, FX, Mapping Jurnal & GL Interface (Phase 5, Sprint 1)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate) + `security-engineer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-14
**Linked FSD**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx §3 (Jurnal Automation Spec)
**Linked BRD**: BRD §6.4 (APP-D Jurnal & GL), RACI: ROLE-AKUN (R), ROLE-AKUN-CTL (A), ROLE-RISK (C/A-2), ROLE-AUDIT (I), ROLE-IT-ADMIN (S — DLQ)
**Linked Decision Log**:
- `DEC-P5-M1-002` (LOCKED) — 27 master event codes, seed `mst.mapping_jurnal_header` di migration 000035
- `DEC-P5-M1-003` (LOCKED) — 6-eyes untuk regulated codes (ECL/EIR/klasifikasi), 4-eyes baseline lainnya
- `DEC-017` — 4-eyes/6-eyes SoD enforcement; `maker_id ≠ reviewer_id ≠ approver_id`
- `DEC-018` — audit trail append-only, 10+10 tahun retensi
- `DEC-019` — modular monolith; `jrnl.*` schema shared dengan semua modul
- `DEC-021` — Idempotency-Key wajib setiap mutating endpoint

**Handoff berikutnya**:
- `system-analyst` → OpenAPI fragment `/mapping-jurnal/*`, `/jurnal/*`, `/jobs/{id}/stream`, state machine mapping workflow; error codes `JURNAL_EVENT_NOT_MAPPED`, `JURNAL_BALANCE_FAILED`, `JURNAL_DUP_POST`, `JURNAL_PERIODE_CLOSED`, `JURNAL_KLASIFIKASI_MISMATCH`, `JURNAL_MAPPING_WORKFLOW_GATE`
- `data-modeler` → migration 000035: seed 27 event codes ke `mst.mapping_jurnal_header`; tabel baru `sys.dlq_jurnal_post`; `jrnl.header` + `jrnl.detail` audit column hardening (append-only trigger + DDL CHECK)
- `ifrs9-compliance-reviewer` → BLOCKING gate sebelum merge: verifikasi balance invariant runtime, klasifikasi routing matrix per DEC-P5-M1-002, regulated code 6-eyes enforcement
- `security-engineer` → BLOCKING gate: `jrnl.*` append-only enforcement, `JURNAL.POST` in-transaction, PERIODE_CLOSED bypass guard

**Compliance path**: P5-M2 adalah BLOCKING gate — jurnal posting balance (`total_debit == total_kredit`) + klasifikasi routing + DLQ fallback wajib diverifikasi `ifrs9-compliance-reviewer` sebelum merge ke `develop`. `jrnl.header` + `jrnl.detail` bersifat append-only (wajib diverifikasi `security-engineer`).

---

## Konteks & Dependensi

### Phase 3 (Master Data) telah mendeliver
- `mst.mapping_jurnal_header` — struktur ada (migration 000001 + 000017), `workflow_status` 7-state (`DRAFT/PENDING_REVIEW/PENDING_APPROVAL/PENDING_APPROVAL_2/APPROVED/REJECTED/RETURNED`), kolom: `event_code`, `event_id_kode`, `nama_event`, `kategori_event`, `trigger_source`, `klasifikasi_berlaku VARCHAR(20)[]`, `aktif_flag`, audit cols lengkap
- `mst.mapping_jurnal_detail` — struktur ada (000001 + 000017), kolom: `event_header_id`, `urutan`, `kode_akun_id`, `dk_indicator`, `sumber_amount`, `klasifikasi_filter`, `multiplier`, `aktif_flag`, audit cols
- `mst.chart_of_accounts` — Chart of Accounts lengkap per Phase 3
- `jrnl.header` + `jrnl.detail` + `jrnl.gl_status` — tabel ada di 000001; `CHECK (total_debit = total_kredit)` sudah ada di `jrnl.header`; `CHECK ck_dk_exclusive` sudah ada di `jrnl.detail`

### P5-M1 (PR #104 + #105) telah mendeliver
- `trx.penempatan_deposito` — lifecycle `DRAFT → APPROVED_ACTIVE → MATURED/TERMINATED`
- 3 Asynq events: `penempatan:approved`, `penempatan:matured`, `penempatan:terminated`
- `sys.settlement_account_balance` (migration 000033)

### Yang dibutuhkan di P5-M2 (migration 000035)
- **Seed 27 event codes** ke `mst.mapping_jurnal_header` per DEC-P5-M1-002 — Kepala Akuntansi sign-off required
- **Seed mapping detail rows** per klasifikasi untuk setiap event code — ifrs9-compliance-reviewer review required
- **Tabel baru** `sys.dlq_jurnal_post` — dead-letter queue jurnal posting yang gagal
- **Hardening** `jrnl.header` + `jrnl.detail`: tambah `updated_at`, `updated_by` (audit columns per db-conventions); DDL trigger BEFORE UPDATE yang menolak perubahan (append-only enforcement)

### Event-to-Story mapping
| Event (Asynq) | Story | Modul yang akan produce event lainnya |
|---|---|---|
| `penempatan:approved` | Story 3 | P5-M1 |
| `penempatan:matured` | Story 3 | P5-M1 |
| `penempatan:terminated` | Story 3 | P5-M1 |
| `mtm:computed`, `akrual:computed`, `ecl:charged`, dst | Story 3 (future) | P5-M6, M7, M9, M10 |

---

## Schema Referensi P5-M2

### `mst.mapping_jurnal_header` (existing + seed)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `event_code` | VARCHAR(40) UNIQUE | Contoh: `PENEMPATAN`, `ECL_PEMBENTUKAN` |
| `event_id_kode` | VARCHAR(40) UNIQUE | Format `EV-{NNN}` |
| `nama_event` | VARCHAR(120) | Label human-readable |
| `kategori_event` | VARCHAR(30) | `PENEMPATAN`, `AKRUAL`, `ECL`, `MUTASI_MTM`, `CLOSURE`, dst |
| `trigger_source` | VARCHAR(20) | `USER_INPUT` atau `SYSTEM_JOB` |
| `klasifikasi_berlaku` | VARCHAR(20)[] | Array klasifikasi PSAK 71 yang berlaku. NULL = ALL |
| `aktif_flag` | BOOLEAN | Hanya APPROVED + aktif_flag=true yang bisa dipake resolver |
| `workflow_status` | VARCHAR(30) | `APPROVED` = siap production; 7-state dari migration 000017 |
| `maker_id` | UUID FK | Dari migration 000017 workflow support |
| `reviewer_id` | UUID FK | |
| `approver_id` | UUID FK | Approver pertama (ROLE-AKUN-CTL) |
| `approver_2_id` | UUID FK | Approver kedua (ROLE-RISK) — hanya regulated codes |
| `approver_2_signed_at` | TIMESTAMPTZ | Nullable — hanya 6-eyes |
| `approver_2_signature_hash` | TEXT | |
| `deskripsi` | TEXT | Penjelasan template |
| ...audit cols... | — | `created_at/by`, `updated_at/by`, `deleted_at/by`, `row_version`, `tenant_id` |

### `mst.mapping_jurnal_detail` (existing)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `event_header_id` | UUID FK | → `mst.mapping_jurnal_header.id` |
| `urutan` | INT | Urutan baris posting (1, 2, 3...) |
| `kode_akun_id` | UUID FK | → `mst.chart_of_accounts.id` |
| `dk_indicator` | VARCHAR(10) | `DEBIT` atau `KREDIT` |
| `sumber_amount` | VARCHAR(50) | Field resolver maps: `nominal_idr`, `ecl_amount`, `mtm_change`, dst |
| `klasifikasi_filter` | VARCHAR(20) | Override untuk klasifikasi tertentu (NULL = apply to all klasifikasi_berlaku) |
| `multiplier` | NUMERIC(8,4) | Default 1.0000 — untuk proportion / PPh computation |
| `catatan` | TEXT | |
| ...audit cols... | — | |

### `jrnl.header` (existing — append-only)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `no_jurnal` | VARCHAR(20) UNIQUE | Format `JRN-{YYYY}-{######}`, server-generated |
| `tanggal_posting` | DATE NOT NULL | |
| `periode_id` | UUID FK | → `mst.periode_buku.id` — OPEN atau SOFT_CLOSED only |
| `event_code` | VARCHAR(40) | Dari DEC-P5-M1-002 |
| `mapping_header_id` | UUID FK | → `mst.mapping_jurnal_header.id` |
| `instrumen_id` | UUID FK | Optional (NULL untuk PERIODE_ADJUSTMENT global) |
| `reference_event_type` | VARCHAR(50) | `penempatan_deposito`, `calc_run`, dst |
| `reference_event_id` | UUID | ID dari source event (dedup key bersama event_code) |
| `currency` | CHAR(3) | Default `IDR` |
| `total_debit` | NUMERIC(20,2) | **CHECK: total_debit = total_kredit** |
| `total_kredit` | NUMERIC(20,2) | |
| `narrative` | VARCHAR(500) | |
| `status_internal` | VARCHAR(20) | `POSTED`, `REVERSED`, `PENDING_APPROVAL` (manual entry) |
| `idempotency_key` | VARCHAR(100) | UNIQUE INDEX — dedup Asynq event replay |
| `created_at` | TIMESTAMPTZ | **Immutable setelah INSERT** |
| `created_by` | UUID FK | |

### `jrnl.detail` (existing — append-only)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `header_id` | UUID FK | → `jrnl.header.id` |
| `urutan` | INT | |
| `kode_akun_id` | UUID FK | → `mst.chart_of_accounts.id` |
| `debit_amount` | NUMERIC(20,2) | 0 jika KREDIT — CHECK ck_dk_exclusive |
| `kredit_amount` | NUMERIC(20,2) | 0 jika DEBIT — CHECK ck_dk_exclusive |
| `mata_uang` | CHAR(3) | |
| `narrative_line` | VARCHAR(500) | |
| `created_at` | TIMESTAMPTZ | |

### `sys.dlq_jurnal_post` (baru — migration 000035)
| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `source_event_id` | UUID NOT NULL | ID event sumber (reference_event_id) |
| `source_event_type` | VARCHAR(50) NOT NULL | `penempatan:approved`, `mtm:computed`, dst |
| `event_code` | VARCHAR(40) NOT NULL | Dari DEC-P5-M1-002 |
| `instrumen_id` | UUID | Optional |
| `periode_id` | UUID FK | Periode target posting |
| `payload_jsonb` | JSONB NOT NULL | Full resolver input snapshot (idempotent replay) |
| `error_code` | VARCHAR(50) NOT NULL | `JURNAL_EVENT_NOT_MAPPED`, `JURNAL_BALANCE_FAILED`, `JURNAL_PERIODE_CLOSED`, dst |
| `error_message` | TEXT NOT NULL | |
| `attempt_count` | SMALLINT NOT NULL DEFAULT 1 | |
| `last_attempt_at` | TIMESTAMPTZ NOT NULL | |
| `status` | VARCHAR(20) NOT NULL DEFAULT 'FAILED' | `FAILED`, `REPLAYING`, `REPLAYED_OK`, `ABANDONED` |
| `replayed_jurnal_id` | UUID | → `jrnl.header.id` (terisi setelah replay sukses) |
| `replayed_by` | UUID FK | User yang trigger replay |
| `replayed_at` | TIMESTAMPTZ | |
| ...audit cols... | — | `created_at/by`, `updated_at/by`, `row_version`, `tenant_id` |

---

## Klasifikasi Regulated vs Operational (DEC-P5-M1-003)

| Kategori | Event Codes (DEC-P5-M1-002 nomor) | Workflow |
|---|---|---|
| **Regulated** (6-eyes) | 2, 3, 4, 5, 6, 7, 12, 13, 14, 15, 17, 20, 26, 27 | ROLE-AKUN → ROLE-AKUN-CTL → ROLE-RISK |
| **Operational** (4-eyes) | 1, 8, 9, 10, 11, 18, 19, 21, 22, 23, 24, 25 | ROLE-AKUN → ROLE-AKUN-CTL |

Regulated codes menyentuh: ECL, EIR catch-up, MTM valuation, staging migration, POCI delta, OCI recycling, modifikasi material, reklasifikasi instrumen.

---

## Story P5-M2-S1 — Maintain Mapping Jurnal Header CRUD

**Actor**: ROLE-AKUN (Maker), ROLE-AKUN-CTL (Reviewer/Approver pertama), ROLE-RISK (Second Approver — regulated only)
**Trigger**: Kepala Akuntansi atau Risk Officer meminta penambahan, perubahan, atau non-aktifan template mapping jurnal
**Goal**: Pengelola mapping jurnal dapat membuat dan memelihara template mapping event dengan 6-eyes (regulated) atau 4-eyes (operational), dengan SoD enforcement; seed 27 event codes dari DEC-P5-M1-002 tersedia setelah migration 000035

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-AKUN dengan permission `jurnal_mapping.create`
2. `mst.chart_of_accounts` memiliki akun yang valid (aktif_flag = true) untuk dipilih sebagai kode_akun di detail rows
3. Untuk create event_code baru: event_code belum ada di `mst.mapping_jurnal_header` (UNIQUE constraint)
4. Untuk 6-eyes (regulated): ROLE-RISK user tersedia dan ter-assign sebagai second approver
5. Seed 27 codes dari DEC-P5-M1-002 tersedia (migration 000035 sudah dijalankan)
6. Kepala Akuntansi sign-off dan ALCO sign-off (untuk ECL codes) terdokumentasi sebelum migration 000035 deploy

### Pre-conditions untuk 6-eyes (codes regulated per DEC-P5-M1-003)
- event_code harus masuk kategori regulated sebelum 6-eyes workflow diaktifkan
- Backend mengklasifikasikan berdasarkan `kategori_event IN ('ECL','AKRUAL','MUTASI_MTM','STAGE_MIGRATION','REKLASIFIKASI')` OR explicit list event_code per DEC-P5-M1-003

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.mapping_jurnal_header` | CREATE / UPDATE (workflow-gated) / READ | Soft-delete, never hard-delete |
| `mst.mapping_jurnal_detail` | CREATE / UPDATE / DELETE (cascade dari header) | Audit cols wajib; inherit workflow dari header |
| `mst.chart_of_accounts` | READ | Picker akun untuk detail rows |
| `sec.user` | READ | Validasi SoD: maker ≠ reviewer ≠ approver ≠ approver_2 |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal_mapping.create` | ROLE-AKUN | Hanya create, bukan approve |
| `jurnal_mapping.review` | ROLE-AKUN-CTL | Review + approver pertama |
| `jurnal_mapping.approve` | ROLE-AKUN-CTL | Approval step 1 |
| `jurnal_mapping.approve_2` | ROLE-RISK | 6-eyes: approval step 2; hanya regulated |
| `jurnal_mapping.read` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT, ROLE-CFO | |
| `jurnal_mapping.export` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | Export XLSX |

### Audit Events
| Action | Trigger |
|---|---|
| `JURNAL_MAPPING.CREATE` | Header + detail rows dibuat (DRAFT) |
| `JURNAL_MAPPING.SUBMIT` | Maker submit ke review |
| `JURNAL_MAPPING.REVIEW` | ROLE-AKUN-CTL sign-off |
| `JURNAL_MAPPING.APPROVE` | ROLE-AKUN-CTL final approve (operational) atau approver_1 (regulated) |
| `JURNAL_MAPPING.APPROVE_2` | ROLE-RISK second approver (regulated 6-eyes) |
| `JURNAL_MAPPING.REJECT` | Di step mana pun, dengan reject_reason |
| `JURNAL_MAPPING.DEACTIVATE` | aktif_flag di-set false oleh ROLE-AKUN-CTL |
| `JURNAL_MAPPING.EXPORT` | Export XLSX mapping coverage |

### Acceptance Criteria

```gherkin
Feature: Mapping jurnal header CRUD dengan 6-eyes (regulated) / 4-eyes (operational)

  Background:
    Given user maker: USR-010 (Dewi Rahayu, ROLE-AKUN)
    And user reviewer/approver_1: USR-011 (Eko Susanto, ROLE-AKUN-CTL)
    And user approver_2: USR-012 (Fajar Hidayat, ROLE-RISK)
    And USR-010 ≠ USR-011 ≠ USR-012
    And migration 000035 selesai — 27 event codes sudah ter-seed sebagai APPROVED

  # ─── HAPPY PATH 1: Create + 4-eyes approve mapping OPERATIONAL ───────────────

  Scenario: Maker berhasil membuat mapping PENEMPATAN baru (operational, 4-eyes)
    Given event_code "PENEMPATAN_BARU_INSTRUMENT" belum ada di mst.mapping_jurnal_header
    And kategori_event = "PENEMPATAN" (operational per DEC-P5-M1-003)
    When USR-010 mengirim POST /api/v1/mapping-jurnal
      With body:
        | event_code          | PENEMPATAN_BARU_INSTRUMENT       |
        | nama_event          | Penempatan Instrumen Baru         |
        | kategori_event      | PENEMPATAN                        |
        | trigger_source      | USER_INPUT                        |
        | klasifikasi_berlaku | ["AC", "FVOCI", "FVTPL"]          |
        | deskripsi           | Template untuk penempatan instrumen baru |
        | detail_rows         | [{urutan:1, kode_akun_id:..., dk_indicator:DEBIT, sumber_amount:nominal_idr}, {urutan:2, kode_akun_id:..., dk_indicator:KREDIT, sumber_amount:nominal_idr}] |
      With Idempotency-Key: IK-MAP-001
    Then sistem mengembalikan HTTP 201 dengan:
      | workflow_status | DRAFT              |
      | event_code      | PENEMPATAN_BARU_INSTRUMENT |
      | detail_rows.count | 2                |
    And audit log: JURNAL_MAPPING.CREATE, actor = USR-010

    When USR-010 mengirim POST /api/v1/mapping-jurnal/{id}/submit
    Then workflow_status = PENDING_REVIEW
    And audit log: JURNAL_MAPPING.SUBMIT

    When USR-011 mengirim POST /api/v1/mapping-jurnal/{id}/review
      With body: { "comment": "Template sudah sesuai chart of accounts", "signature_method": "JWT_STEP_UP" }
    Then workflow_status = PENDING_APPROVAL
    And audit log: JURNAL_MAPPING.REVIEW, actor = USR-011

    When USR-011 mengirim POST /api/v1/mapping-jurnal/{id}/approve
      With body: { "comment": "Disetujui sesuai FSD-APP-D", "signature_method": "JWT_STEP_UP" }
    Then workflow_status = APPROVED
    And aktif_flag = true
    And audit log: JURNAL_MAPPING.APPROVE, actor = USR-011
    And template dapat segera dipakai oleh resolver engine

  # ─── HAPPY PATH 2: Create + 6-eyes approve mapping REGULATED (ECL) ──────────

  Scenario: Maker membuat mapping ECL_PEMBENTUKAN baru (regulated, 6-eyes)
    Given event_code "ECL_PEMBENTUKAN" sudah ter-seed sebagai APPROVED
    And ROLE-AKUN ingin mengubah detail rows ECL_PEMBENTUKAN → perlu versi baru
    When USR-010 mengirim PUT /api/v1/mapping-jurnal/{id} dengan perubahan detail rows
    Then sistem mendeteksi event_code masuk kategori regulated
    And workflow_status kembali ke DRAFT (versi baru — row lama immutable via audit trail)
    And backend route workflow ke 6-eyes (PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2)

    When USR-010 submit → USR-011 review → USR-011 approve (approver_1)
    Then workflow_status = PENDING_APPROVAL_2
    And notifikasi ke ROLE-RISK: "Template ECL_PEMBENTUKAN menunggu second approval Anda"

    When USR-012 mengirim POST /api/v1/mapping-jurnal/{id}/approve-2
      With body: { "comment": "Template ECL sudah sesuai PSAK 71 §5.5.8", "signature_method": "JWT_STEP_UP" }
      With MFA required: ya (ROLE-RISK step-up per DEC-027)
    Then workflow_status = APPROVED
    And approver_2_id = USR-012
    And approver_2_signed_at terisi timestamp
    And approver_2_signature_hash terisi SHA-256 dari payload approve_2
    And audit log: JURNAL_MAPPING.APPROVE_2, actor = USR-012

  # ─── ERROR CASE: SoD violation — maker coba jadi approver ───────────────────

  Scenario: SoD violation — ROLE-AKUN maker mencoba approve mapping miliknya sendiri
    Given mapping JM-001 dalam PENDING_APPROVAL, maker_id = USR-010
    When USR-010 mencoba POST /api/v1/mapping-jurnal/JM-001/approve
    Then sistem mengembalikan HTTP 403:
      | error.code    | SOD_VIOLATION                                               |
      | error.message | "Anda tidak bisa menjadi approver untuk mapping yang Anda buat sendiri (DEC-017)." |
    And workflow_status tetap PENDING_APPROVAL
    And audit log mencatat upaya SOD_VIOLATION

  # ─── ERROR CASE: Regulated code tanpa ROLE-RISK second approver ─────────────

  Scenario: Regulated mapping di-approve tanpa ROLE-RISK — blocked
    Given mapping JM-ECL-001 (event_code = STAGE_MIGRATION, regulated) dalam PENDING_APPROVAL
    And workflow_status = PENDING_APPROVAL (bukan PENDING_APPROVAL_2)
    When USR-011 approve tapi sistem tidak ada path ke PENDING_APPROVAL_2 karena misconfigured
    Then sistem mengembalikan HTTP 422:
      | error.code    | JURNAL_MAPPING_WORKFLOW_GATE                                |
      | error.message | "Event code STAGE_MIGRATION adalah regulated — 6-eyes wajib. Approve_2 oleh ROLE-RISK diperlukan setelah approver_1." |

  # ─── ERROR CASE: Balance validation detail rows — debit ≠ kredit count ───────

  Scenario: Create mapping gagal karena balance detail rows tidak simetris
    When USR-010 membuat mapping dengan 1 baris DEBIT dan 0 baris KREDIT
    Then sistem mengembalikan HTTP 422:
      | error.code    | VALIDATION_FAILED                                           |
      | error.message | "Template harus memiliki minimal 1 baris DEBIT dan 1 baris KREDIT untuk menjamin balance jurnal." |
    And tidak ada record yang dibuat

  # ─── HAPPY PATH 3: Non-aktifkan mapping yang sudah diapprove ────────────────

  Scenario: ROLE-AKUN-CTL men-deactivate mapping yang tidak lagi relevan
    Given mapping JM-OLD-001 (event_code = PENEMPATAN_LEGACY, APPROVED, aktif_flag = true)
    When USR-011 mengirim PATCH /api/v1/mapping-jurnal/JM-OLD-001/deactivate
    Then mapping.aktif_flag = false
    And mapping.workflow_status tetap APPROVED (tidak berubah — hanya flag)
    And audit log: JURNAL_MAPPING.DEACTIVATE, actor = USR-011
    And resolver engine tidak akan menemukan template ini saat event PENEMPATAN_LEGACY dipicu
    And toast: "Mapping PENEMPATAN_LEGACY berhasil di-nonaktifkan. Resolver tidak akan menggunakan template ini."

  # ─── READ: List mapping jurnal dengan filter dan export ─────────────────────

  Scenario: ROLE-AUDIT melihat seluruh mapping history + export XLSX
    Given 27 mapping seed sudah APPROVED + beberapa DRAFT/PENDING
    When ROLE-AUDIT mengakses GET /api/v1/mapping-jurnal?filter[workflow_status]=APPROVED&sort=event_code:asc
    Then response berisi semua APPROVED mapping, sorted by event_code
    And setiap item mengandung: event_code, kategori_event, workflow_status, aktif_flag, detail_count
    When ROLE-AUDIT klik Export XLSX
    Then file download: `mapping-jurnal-20260614.xlsx` dengan semua 27 APPROVED event codes
    And audit log: JURNAL_MAPPING.EXPORT
```

### Open Questions — Story 1
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-1a | Apakah update mapping yang sudah APPROVED otomatis membuat versi baru (new UUID) atau menimpa dengan workflow reset? | **New UUID + APPROVED baru = immutable history**. Row lama: `aktif_flag = false`, `deleted_at` terisi. Data-modeler konfirmasi. |
| OQ-M2-1b | Export XLSX mapping coverage — apakah per event_code satu sheet, atau semua dalam satu sheet? | **Satu sheet**, kolom: event_code, kategori, trigger_source, klasifikasi_berlaku, DEBIT_COA, KREDIT_COA, aktif_flag, approved_by, approved_at. Flag ke uiux-designer. |
| OQ-M2-1c | Apakah ROLE-RISK step-up MFA diperlukan saat approve_2? | DEC-027 mewajibkan step-up untuk ECL parameter approve. Mapping ECL masuk? **Asumsi: Ya, step-up MFA untuk approve_2 regulated**. Konfirmasi ke security-engineer. |

---

## Story P5-M2-S2 — Jurnal Resolver — Single Event

**Actor**: Asynq subscriber (event-driven), atau API caller internal (service-to-service)
**Trigger**: Event jurnal masuk (dari Asynq queue) atau direct call dari service layer saat transaksi terjadi
**Goal**: Resolver service menghasilkan array `JurnalLine` (debit/kredit) yang balance dari input event metadata, siap untuk diposting ke `jrnl.header` + `jrnl.detail`

### Input Schema (Resolver Contract)
```go
type ResolverInput struct {
    EventCode        string           // Dari DEC-P5-M1-002
    KlasifikasiPSAK  string           // AC | FVOCI | FVTPL | FVOCI_ELECTION | POCI
    InstrumenID      *uuid.UUID       // Optional (nil untuk PERIODE_ADJUSTMENT global)
    PeriodeID        uuid.UUID
    AmountIDR        decimal.Decimal  // Nominal posting (DEC-016: shopspring/decimal)
    Currency         string           // Default "IDR"
    FxRate           decimal.Decimal  // JISDOR rate; 1.0000 untuk IDR
    MetadataJSON     json.RawMessage  // Event-specific (mis. {ecl_stage: 2, eir: 0.065})
    SourceEventID    uuid.UUID        // Dedup key
    SourceEventType  string           // "penempatan_deposito", "calc_run", dst
}

type JurnalLine struct {
    Urutan           int
    Posisi           string           // DEBIT atau KREDIT
    AkunID           uuid.UUID
    AmountIDR        decimal.Decimal
    Narasi           string
    KlasifikasiEligible string
}
```

### Output
Array `[]JurnalLine` yang:
1. Sum DEBIT == Sum KREDIT (invariant checked sebelum return)
2. Setiap line ter-trace ke `mapping_jurnal_detail.id` (untuk audit)

### Pre-conditions
1. `mst.mapping_jurnal_header` memiliki row `event_code = input.EventCode` dengan `workflow_status = 'APPROVED'` dan `aktif_flag = true`
2. `klasifikasi_berlaku` pada header mengandung `input.KlasifikasiPSAK` (atau NULL = apply all)
3. Semua `kode_akun_id` di detail rows memiliki akun aktif di `mst.chart_of_accounts`
4. `AmountIDR` > 0

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `mst.mapping_jurnal_header` | READ | Lookup by event_code + klasifikasi |
| `mst.mapping_jurnal_detail` | READ | Fetch ordered by urutan, WHERE aktif_flag = true |
| `mst.chart_of_accounts` | READ | Validasi akun masih aktif |

### Permissions
Resolver adalah internal service call — tidak ada RBAC check di layer ini. Pemanggil (Asynq worker / service handler) sudah ter-autentikasi dan terotoritasi sebelum memanggil resolver.

### Audit Events
Resolver tidak menulis audit log — posting service-lah yang menulis `JURNAL.POST` ke `aud.audit_log` pada transaksi yang sama dengan INSERT ke `jrnl.header`.

### Acceptance Criteria

```gherkin
Feature: Jurnal resolver — mapping event ke JurnalLine debit/kredit

  Background:
    Given mst.mapping_jurnal_header row APPROVED untuk event_code "PENEMPATAN":
      | klasifikasi_berlaku | ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"] |
    And detail rows:
      | urutan | dk_indicator | kode_akun   | sumber_amount   |
      | 1      | DEBIT        | 1110-DEP    | nominal_idr     |
      | 2      | KREDIT       | 1001-KAS    | nominal_idr     |

  # ─── HAPPY PATH 1: Resolver menghasilkan JurnalLine yang balance ─────────────

  Scenario: Resolver menyelesaikan event PENEMPATAN untuk instrumen AC
    Given ResolverInput:
      | EventCode       | PENEMPATAN         |
      | KlasifikasiPSAK | AC                 |
      | AmountIDR       | 5000000000.0000    |
      | Currency        | IDR                |
      | FxRate          | 1.00000000         |
    When resolver.Resolve(input) dipanggil
    Then output adalah array [JurnalLine]:
      | urutan | posisi  | akun_id  | amount_idr      |
      | 1      | DEBIT   | 1110-DEP | 5000000000.0000 |
      | 2      | KREDIT  | 1001-KAS | 5000000000.0000 |
    And Sum DEBIT = Sum KREDIT = 5000000000.0000 (balance check PASS)
    And semua AkunID valid (chart_of_accounts.aktif_flag = true)

  # ─── HAPPY PATH 2: Resolver dengan klasifikasi_filter override per detail row ─

  Scenario: Resolver untuk event MTM_FVOCI — hanya klasifikasi FVOCI yang di-serve, bukan FVTPL
    Given mapping_jurnal_header "MTM_FVOCI" dengan klasifikasi_berlaku = ["FVOCI"]
    And detail rows untuk FVOCI:
      | urutan | dk_indicator | kode_akun     | klasifikasi_filter |
      | 1      | DEBIT        | 3010-OCI-AST  | FVOCI              |
      | 2      | KREDIT       | 1210-OBLIGASI | FVOCI              |
    When resolver dipanggil dengan KlasifikasiPSAK = "FVOCI" dan EventCode = "MTM_FVOCI"
    Then output mengandung 2 baris FVOCI-specific
    And balance: Sum DEBIT == Sum KREDIT

  # ─── ERROR CASE: Event code tidak ditemukan di mapping ──────────────────────

  Scenario: Resolver gagal karena event_code tidak ada di mst.mapping_jurnal_header
    Given tidak ada APPROVED mapping untuk event_code "UNKNOWN_EVENT"
    When resolver dipanggil dengan EventCode = "UNKNOWN_EVENT"
    Then resolver mengembalikan error:
      | error.code    | JURNAL_EVENT_NOT_MAPPED                                     |
      | error.message | "Tidak ada mapping jurnal APPROVED untuk event code 'UNKNOWN_EVENT'. Pastikan template sudah dibuat dan di-approve." |
    And resolver tidak menghasilkan JurnalLine

  # ─── ERROR CASE: Klasifikasi tidak match ke mapping yang tersedia ────────────

  Scenario: Resolver gagal karena klasifikasi instrumen tidak ada di klasifikasi_berlaku mapping
    Given mapping "AKRUAL_BUNGA" memiliki klasifikasi_berlaku = ["AC", "FVOCI"]
    When resolver dipanggil dengan KlasifikasiPSAK = "FVTPL" dan EventCode = "AKRUAL_BUNGA"
    Then resolver mengembalikan error:
      | error.code    | JURNAL_KLASIFIKASI_MISMATCH                                 |
      | error.message | "Event code 'AKRUAL_BUNGA' tidak berlaku untuk klasifikasi PSAK 71 'FVTPL'. Mapping hanya untuk: AC, FVOCI." |

  # ─── ERROR CASE: Balance check gagal pre-post (bug di template data) ─────────

  Scenario: Resolver mendeteksi imbalance sebelum return (data mapping rusak)
    Given mapping "BROKEN_TEMPLATE" memiliki detail rows:
      | urutan | dk_indicator | amount |
      | 1      | DEBIT        | 1000   |
      | 2      | KREDIT       | 900    |    ← tidak balance
    When resolver dipanggil untuk "BROKEN_TEMPLATE"
    Then resolver mengembalikan error:
      | error.code    | JURNAL_BALANCE_FAILED                                       |
      | error.message | "Resolver menghasilkan imbalance: DEBIT 1000.0000 ≠ KREDIT 900.0000. Template mapping harus diperbaiki oleh ROLE-AKUN." |
    And resolver tidak menghasilkan JurnalLine (gagal fast sebelum posting)
```

### Open Questions — Story 2
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-2a | Untuk event FCY (FX_UNREALIZED, PENEMPATAN USD), apakah resolver mengkonversi ke IDR, atau menghasilkan JurnalLine dengan `mata_uang = 'USD'` dan `amount` dalam FCY? | **Resolver output selalu IDR** (amount_idr = amount_fcy × fx_rate). Detail mata_uang_posting = 'IDR' di jrnl.detail. Konfirmasi ke ifrs9-compliance-reviewer. |
| OQ-M2-2b | Apakah multiplier di `mapping_jurnal_detail` berlaku untuk PPh 20% (mis. `kredit_amount = nominal × 0.20`)? | Ya. Detail row PPh: `sumber_amount = nominal_idr`, `multiplier = 0.2000`. Resolver menghitung: `amount = input.AmountIDR × multiplier`. |
| OQ-M2-2c | Untuk ECL_PEMBENTUKAN: apakah `AmountIDR` di-pass sebagai nilai ECL kotor, atau net movement (ECL_baru − ECL_lama)? | **Net movement** (delta ECL). Pemanggil (ECL calc service) bertanggung jawab menghitung delta. Resolver hanya menggunakan nilai yang di-pass. Konfirmasi ke ecl-eir-engineer. |

---

## Story P5-M2-S3 — Jurnal Posting via Asynq Subscribers

**Actor**: Asynq worker (system) subscribing ke events dari P5-M1 dan modul berikutnya
**Trigger**: Asynq event diterima: `penempatan:approved`, `penempatan:matured`, `penempatan:terminated`; (future P5-M6..M10: `mtm:computed`, `akrual:computed`, `ecl:charged`, dst)
**Goal**: Worker memanggil resolver, menghasilkan JurnalLine, INSERT ke `jrnl.header` + `jrnl.detail` dalam satu transaksi database, menjamin balance, menulis audit, dan idempotent terhadap event replay

### Pre-conditions
1. Event berisi payload lengkap: `source_event_id`, `event_code`, `instrumen_id`, `periode_id`, `amount_idr`, `klasifikasi_psak71`, `fx_rate`
2. Mapping resolver menghasilkan JurnalLine yang balance (Sum DEBIT == Sum KREDIT)
3. Periode target: `status_periode IN ('OPEN', 'SOFT_CLOSED')` — HARD_CLOSED ditolak
4. Idempotency: `(reference_event_id, event_code)` belum ada di `jrnl.header` (UNIQUE enforced via index)

### Idempotency Key
`jrnl.header.idempotency_key = SHA256(source_event_id || "::" || event_code)`
Unique index `uq_jrnl_idempotency` sudah ada di migration 000001. Replay Asynq event yang sama tidak menghasilkan posting duplikat.

### On Failure → DLQ
Apapun error yang tidak bisa di-recover (event_code tidak mapped, balance gagal, periode CLOSED) → worker menulis baris ke `sys.dlq_jurnal_post` + menulis `aud.audit_log` action `JURNAL.POST_FAILED`, kemudian task dianggap selesai (tidak di-retry oleh Asynq untuk domain error — hanya infra error yang di-retry).

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | INSERT (append-only) | Tidak ada UPDATE setelah INSERT |
| `jrnl.detail` | INSERT (append-only) | Tidak ada UPDATE setelah INSERT |
| `mst.periode_buku` | READ | Check status sebelum posting |
| `mst.mapping_jurnal_header` | READ | Via resolver |
| `sys.dlq_jurnal_post` | INSERT | Hanya saat posting gagal |

### Permissions
Asynq worker berjalan dengan service account (tidak ada JWT RBAC check di level worker). Audit log ditulis dengan `actor_role = 'SYSTEM_WORKER'`.

### Audit Events
| Action | Trigger |
|---|---|
| `JURNAL.POST` | INSERT `jrnl.header` berhasil (in-transaction, same tx) |
| `JURNAL.POST_FAILED` | Gagal posting, masuk DLQ |

### Acceptance Criteria

```gherkin
Feature: Jurnal posting otomatis via Asynq event subscriber

  Background:
    Given mapping APPROVED untuk event_code "PENEMPATAN" (klasifikasi_berlaku = ALL)
    And penempatan PNP-2026-00001 (APPROVED_ACTIVE, instrumen AC, nominal_idr = 5000000000.0000)
    And periode buku Juni-2026 status = 'OPEN'
    And sys.settlement_account_balance tersedia

  # ─── HAPPY PATH 1: penempatan:approved → jurnal PENEMPATAN diposting ────────

  Scenario: Worker menerima event penempatan:approved dan memposting jurnal
    When Asynq worker menerima event penempatan:approved:
      | source_event_id  | PNP-2026-00001-uuid |
      | event_code       | PENEMPATAN          |
      | instrumen_id     | INST-DEP-001        |
      | periode_id       | PERIODE-2026-06     |
      | amount_idr       | 5000000000.0000     |
      | klasifikasi_psak | AC                  |
      | fx_rate          | 1.00000000          |
    Then worker memanggil resolver → menghasilkan 2 JurnalLine (DEBIT + KREDIT 5000000000.0000)
    And worker melakukan INSERT atomik dalam satu transaksi DB:
      | jrnl.header: no_jurnal        | JRN-2026-000001                          |
      | jrnl.header: event_code       | PENEMPATAN                               |
      | jrnl.header: reference_event_type | penempatan_deposito                  |
      | jrnl.header: reference_event_id   | PNP-2026-00001-uuid                  |
      | jrnl.header: total_debit      | 5000000000.00                            |
      | jrnl.header: total_kredit     | 5000000000.00                            |
      | jrnl.header: status_internal  | POSTED                                   |
      | jrnl.header: idempotency_key  | SHA256("PNP-2026-00001-uuid::PENEMPATAN") |
      | jrnl.detail: 2 rows           | DEBIT 5000000000.00 + KREDIT 5000000000.00 |
    And CHECK CONSTRAINT `ck_jrnl_balance` PASS (total_debit = total_kredit)
    And audit log: JURNAL.POST, action = "PENEMPATAN", entity_id = jrnl.header.id (in same transaction)
    And Asynq task selesai (acknowledge)

  # ─── HAPPY PATH 2: Idempotency — replay event yang sama tidak double-post ───

  Scenario: Asynq worker menerima event penempatan:approved yang sama dua kali (retry/replay)
    Given jrnl.header sudah ada dengan idempotency_key = SHA256("PNP-2026-00001-uuid::PENEMPATAN")
    When Asynq worker menerima event penempatan:approved kedua dengan source_event_id yang sama
    Then INSERT `jrnl.header` ditolak oleh unique index `uq_jrnl_idempotency`
    And worker mendeteksi error = duplicate key + mengembalikan IDEMPOTENCY_REPLAY (task acknowledge, bukan retry)
    And tidak ada baris baru di `jrnl.header` atau `jrnl.detail`
    And audit log: tidak ada entry baru (idempotent)

  # ─── ERROR CASE: Periode HARD_CLOSED — posting ditolak, masuk DLQ ──────────

  Scenario: Worker mencoba posting ke periode yang sudah HARD_CLOSED
    Given periode buku Mei-2026 status = 'HARD_CLOSED'
    And event masuk dengan periode_id = PERIODE-2026-05
    When worker mencoba memposting jurnal
    Then posting ditolak sebelum INSERT
    And worker menulis `sys.dlq_jurnal_post`:
      | error_code    | JURNAL_PERIODE_CLOSED                |
      | error_message | "Periode Mei-2026 sudah HARD_CLOSED. Posting tidak dapat dilakukan." |
      | payload_jsonb | full event payload (untuk replay)    |
      | status        | FAILED                               |
    And audit log: JURNAL.POST_FAILED
    And Asynq task di-acknowledge (bukan retry — domain error)

  # ─── ERROR CASE: Mapping tidak tersedia → DLQ ───────────────────────────────

  Scenario: Worker menerima event dengan event_code yang belum ter-mapping
    Given event_code "DISTRIBUSI_REKSADANA" belum punya APPROVED mapping
    When worker menerima event dengan event_code = "DISTRIBUSI_REKSADANA"
    Then resolver mengembalikan JURNAL_EVENT_NOT_MAPPED
    And worker menulis `sys.dlq_jurnal_post`:
      | error_code    | JURNAL_EVENT_NOT_MAPPED              |
      | status        | FAILED                               |
    And audit log: JURNAL.POST_FAILED
    And notifikasi alert ke ROLE-AKUN-CTL: "Jurnal event DISTRIBUSI_REKSADANA tidak ter-mapping. Cek DLQ."
```

### Open Questions — Story 3
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-3a | Apakah Asynq task di-retry untuk infra error (DB down) vs domain error (mapping not found, periode closed)? | **Infra error = retry** dengan exponential backoff (max 3×). Domain error = acknowledge + DLQ (tidak retry). Konfirmasi ke backend-engineer-go. |
| OQ-M2-3b | `no_jurnal` format `JRN-{YYYY}-{######}` — apakah sequence per tahun atau global? | **Per tahun** (reset tiap 1 Januari), max 999.999 per tahun. Flag ke data-modeler: sequence di `sys.config_param` atau PostgreSQL sequence. |
| OQ-M2-3c | Apakah event dari modul lain (P5-M6 MTM, P5-M9 akrual, P5-M10 ECL) menggunakan Asynq yang sama atau channel berbeda? | **Asynq queue yang sama** (`jurnal:post`), payload dibedakan via `source_event_type`. Konfirmasi ke backend-engineer-go. |

---

## Story P5-M2-S4 — Jurnal Posting Manual (Ad-hoc Entry)

**Actor**: ROLE-AKUN
**Trigger**: Kepala Akuntansi membutuhkan penyesuaian periode (PERIODE_ADJUSTMENT) atau koreksi atas jurnal sebelumnya (CORRECTION_PERIODE_CLOSED) yang tidak bisa di-trigger otomatis oleh system job
**Goal**: ROLE-AKUN dapat membuat jurnal manual melalui form UI dengan 4-eyes workflow (karena PERIODE_ADJUSTMENT dan CORRECTION_PERIODE_CLOSED adalah operational codes per DEC-P5-M1-003), dengan dokumen pendukung wajib

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-AKUN dengan permission `jurnal.post`
2. event_code yang dipilih ada di list: `PERIODE_ADJUSTMENT` atau `CORRECTION_PERIODE_CLOSED`
3. Untuk `CORRECTION_PERIODE_CLOSED`: periode target boleh `SOFT_CLOSED` (sesuai FSD-APP-D §1.4); instrumen optional
4. Minimal 1 dokumen pendukung di-upload sebelum submit ke workflow
5. Periode target: `OPEN` atau `SOFT_CLOSED` (tidak bisa `HARD_CLOSED`)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | INSERT (PENDING_APPROVAL dulu, bukan langsung POSTED) | status_internal = 'PENDING_APPROVAL' sampai approved |
| `jrnl.detail` | INSERT | Setelah approved, baru di-commit jadi POSTED |
| `doc.upload` | READ | Referensi dokumen pendukung wajib |
| `mst.periode_buku` | READ | Validasi status periode |

### Workflow manual entry
```
ROLE-AKUN create → status_internal = PENDING_APPROVAL
       ↓
  ROLE-AKUN submit
       ↓
  ROLE-AKUN-CTL review + approve
       ↓
  status_internal = POSTED
  INSERT ke jrnl.detail permanent
  Audit JURNAL.POST ditulis
```

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal.post` | ROLE-AKUN | Create manual entry draft |
| `jurnal.approve` | ROLE-AKUN-CTL | Approve setelah review — 4-eyes |
| `jurnal.read` | Semua role kecuali ROLE-IT-ADMIN | |

### Audit Events
| Action | Trigger |
|---|---|
| `JURNAL.MANUAL_DRAFT` | ROLE-AKUN membuat draft manual |
| `JURNAL.MANUAL_SUBMIT` | Maker submit ke review |
| `JURNAL.MANUAL_APPROVE` | ROLE-AKUN-CTL approve + jurnal POSTED |
| `JURNAL.MANUAL_REJECT` | ROLE-AKUN-CTL reject dengan reason |
| `JURNAL.POST` | status_internal di-set POSTED (in-transaction dengan approve) |

### Acceptance Criteria

```gherkin
Feature: Jurnal posting manual oleh ROLE-AKUN (PERIODE_ADJUSTMENT + CORRECTION)

  Background:
    Given user: USR-010 (ROLE-AKUN, Dewi Rahayu)
    And user approver: USR-011 (ROLE-AKUN-CTL, Eko Susanto)
    And periode buku Juni-2026 status = 'SOFT_CLOSED'
    And mapping PERIODE_ADJUSTMENT APPROVED dengan template debit/kredit

  # ─── HAPPY PATH 1: PERIODE_ADJUSTMENT berhasil diposting ────────────────────

  Scenario: ROLE-AKUN membuat dan mengapprove jurnal PERIODE_ADJUSTMENT
    Given USR-010 mengisi form manual entry:
      | event_code      | PERIODE_ADJUSTMENT               |
      | periode_id      | PERIODE-2026-06                  |
      | instrumen_id    | null (penyesuaian global)        |
      | amount_idr      | 500000.0000                      |
      | narasi          | "Koreksi akrual bunga Juni 2026 karena perubahan rounding" |
      | dokumen         | "surat_disposisi_FinCon_Jun2026.pdf" |
    When USR-010 submit POST /api/v1/jurnal/manual-entry
    Then jrnl.header ter-INSERT dengan status_internal = 'PENDING_APPROVAL'
    And audit log: JURNAL.MANUAL_DRAFT

    When USR-011 review dan approve POST /api/v1/jurnal/{id}/manual-approve
      With body: { "comment": "Sudah dicek, koreksi valid per memo CFO", "signature_method": "JWT_STEP_UP" }
    Then jrnl.header status_internal = 'POSTED'
    And jrnl.detail ter-INSERT (2 baris: DEBIT + KREDIT 500000.0000)
    And CHECK CONSTRAINT `ck_jrnl_balance` PASS
    And audit log: JURNAL.POST (in-transaction dengan approve)
    And toast ke USR-011: "Jurnal JRN-2026-000042 berhasil diposting. PERIODE_ADJUSTMENT Juni 2026."

  # ─── ERROR CASE: Posting manual ke HARD_CLOSED periode ditolak ──────────────

  Scenario: ROLE-AKUN mencoba posting PERIODE_ADJUSTMENT ke periode HARD_CLOSED
    Given periode buku Maret-2026 status = 'HARD_CLOSED'
    When USR-010 mengirim POST /api/v1/jurnal/manual-entry dengan periode_id = PERIODE-2026-03
    Then sistem mengembalikan HTTP 423:
      | error.code    | JURNAL_PERIODE_CLOSED                                       |
      | error.message | "Periode Maret-2026 sudah HARD_CLOSED. Posting manual tidak dapat dilakukan." |
    And tidak ada record yang dibuat

  # ─── ERROR CASE: Submit tanpa dokumen pendukung ──────────────────────────────

  Scenario: Submit jurnal manual gagal karena tidak ada dokumen pendukung
    Given draft manual entry sudah dibuat tanpa dokumen dilampirkan
    When USR-010 mencoba submit ke review
    Then sistem mengembalikan HTTP 422:
      | error.code    | VALIDATION_FAILED                                           |
      | error.details[0].field | dokumen_pendukung_id                         |
      | error.details[0].rule  | "required — lampirkan dokumen pendukung sebelum submit" |
    And workflow_status tetap DRAFT (tidak berpindah ke PENDING_APPROVAL)
```

### Open Questions — Story 4
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-4a | Apakah ad-hoc manual entry juga bisa digunakan untuk event codes selain PERIODE_ADJUSTMENT dan CORRECTION_PERIODE_CLOSED? | **Tidak** — manual entry di-restrict hanya untuk 2 event operational ini. Event lain harus dari sistem (Asynq). Konfirmasi ke ifrs9-compliance-reviewer. |
| OQ-M2-4b | CORRECTION_PERIODE_CLOSED: apakah periode yang boleh ditarget hanya SOFT_CLOSED, atau bisa OPEN juga? | **OPEN dan SOFT_CLOSED** keduanya valid. HARD_CLOSED ditolak. Sesuai FSD-APP-D §1.4. |

---

## Story P5-M2-S5 — Read Jurnal — List + Drill-down

**Actor**: ROLE-AKUN, ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-RISK (read-only)
**Trigger**: User membuka halaman `/jurnal/header` untuk melihat jurnal yang sudah diposting, atau membuka detail jurnal dari halaman instrumen/calc-run
**Goal**: User dapat melihat daftar jurnal dengan filter komprehensif (periode, event_code, instrumen, status), sort multi-kolom, cursor pagination, export CSV/XLSX, dan drill-down ke detail baris debit/kredit

### Pre-conditions
1. User ter-autentikasi dengan permission `jurnal.read`
2. `jrnl.header` tidak memiliki baris soft-deleted (append-only — no deletion)
3. Untuk export besar (> 10k rows): Asynq async export → MinIO → notif download link

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `jrnl.header` | READ | Base table |
| `jrnl.detail` | READ | JOIN untuk drill-down |
| `mst.mapping_jurnal_header` | READ | JOIN untuk nama_event |
| `mst.instrumen` | READ | JOIN untuk nama instrumen |
| `mst.periode_buku` | READ | JOIN untuk label periode |
| `mst.chart_of_accounts` | READ | JOIN di drill-down (nama akun) |

### Permissions
| Role | Permission | Catatan |
|---|---|---|
| ROLE-AKUN | `jurnal.read` | Semua POSTED jurnal |
| ROLE-AKUN-CTL | `jurnal.read` | + PENDING_APPROVAL entries |
| ROLE-CFO | `jurnal.read` | Summary + export |
| ROLE-AUDIT | `jurnal.read`, `audit_log.read` | Full access termasuk sebelum periode ini |
| ROLE-RISK | `jurnal.read` | Read-only, filter per instrumen ECL-related |

### Filter yang tersedia
| Filter | Type | Contoh |
|---|---|---|
| `?filter[periode_id]=` | UUID | Filter per periode buku |
| `?filter[event_code]=` | Enum multi | `PENEMPATAN,ECL_PEMBENTUKAN` |
| `?filter[instrumen_id]=` | UUID | Filter per instrumen |
| `?filter[status_internal]=` | Enum multi | `POSTED,REVERSED` |
| `?filter[tanggal_posting]=` | `between:2026-06-01,2026-06-30` | Rentang tanggal |
| `?filter[total_debit]=` | `gte:1000000` | Nominal minimum |
| `?q=` | Text search | Cari `no_jurnal`, `narrative` |

### Sort yang tersedia
`no_jurnal`, `tanggal_posting`, `event_code`, `total_debit`, `created_at`

### Audit Events
| Action | Trigger |
|---|---|
| `JURNAL.EXPORT` | Setiap export dijalankan (filter + row_count dicatat) |

### Acceptance Criteria

```gherkin
Feature: Read jurnal — list, filter, sort, paging, export, drill-down

  Background:
    Given database memiliki 2.847 jrnl.header records dengan berbagai event_code dan periode
    And user ter-autentikasi sebagai ROLE-AKUN-CTL

  # ─── HAPPY PATH 1: List jurnal dengan filter dan cursor pagination ────────────

  Scenario: List jurnal per periode + event_code dengan cursor pagination
    When user mengakses GET /api/v1/jurnal/header?filter[periode_id]=PERIODE-2026-06&filter[event_code]=PENEMPATAN&limit=50&sort=tanggal_posting:desc
    Then sistem mengembalikan HTTP 200 dengan:
      | data.length        | ≤ 50 (sesuai limit)                         |
      | pagination.hasMore | true jika total PENEMPATAN Juni-2026 > 50   |
      | appliedFilter      | {"periode_id":"...","event_code":"PENEMPATAN"} |
      | appliedSort        | [{"col":"tanggal_posting","dir":"desc"}]     |
    And setiap item mengandung: no_jurnal, tanggal_posting, event_code, instrumen_nama, total_debit, status_internal
    And deep-link URL: `/jurnal/header?filter[periode_id]=...&filter[event_code]=PENEMPATAN&sort=tanggal_posting:desc`

  # ─── HAPPY PATH 2: Drill-down ke detail jurnal ────────────────────────────────

  Scenario: User melihat detail jurnal header + baris debit/kredit
    When user mengakses GET /api/v1/jurnal/header/JRN-2026-000001
    Then sistem mengembalikan HTTP 200:
      | jrnl_header.no_jurnal    | JRN-2026-000001   |
      | jrnl_header.event_code   | PENEMPATAN        |
      | jrnl_header.total_debit  | 5000000000.00     |
      | jrnl_header.total_kredit | 5000000000.00     |
      | detail[0].posisi         | DEBIT             |
      | detail[0].kode_akun      | 1110-DEP          |
      | detail[0].nama_akun      | Deposito (dari chart_of_accounts) |
      | detail[0].debit_amount   | 5000000000.00     |
      | detail[1].posisi         | KREDIT            |
      | detail[1].kode_akun      | 1001-KAS          |
      | detail[1].kredit_amount  | 5000000000.00     |
      | source.reference_event_type | penempatan_deposito |
      | source.reference_event_id   | PNP-2026-00001-uuid |
    And link ke penempatan source: "/trx/penempatan/PNP-2026-00001"

  # ─── HAPPY PATH 3: Export > 10k rows → async MinIO ──────────────────────────

  Scenario: ROLE-AUDIT export semua jurnal 2026 — dataset besar (async)
    Given total jurnal 2026 = 35.000 records (> 10k threshold)
    When ROLE-AUDIT klik Export XLSX (tanpa filter aktif)
    Then sistem mengembalikan HTTP 202:
      | jobId     | job_01HXYZexport              |
      | statusUrl | /api/v1/jobs/job_01HXYZexport |
    And JobProgressPanel tampil dengan progress bar
    And setelah selesai: toast sukses + signed download URL (TTL 24 jam)
    And audit log: JURNAL.EXPORT, row_count = 35000

  # ─── PERMISSION: ROLE-RISK hanya bisa baca, tidak bisa export ────────────────

  Scenario: ROLE-RISK mencoba export jurnal — permission denied
    Given user ter-autentikasi sebagai ROLE-RISK (tidak memiliki `jurnal.export`)
    When user klik tombol Export
    Then tombol Export disabled di UI (permission check client-side)
    And jika API dipanggil langsung: HTTP 403 FORBIDDEN
      | error.code | FORBIDDEN |
      | error.message | "Tidak memiliki permission jurnal.export." |
```

### Open Questions — Story 5
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-5a | Apakah `jrnl.header` hasil reverse (status_internal = 'REVERSED') tampil di list default atau di-hide? | **Tampil** di list default dengan badge "REVERSED". Filter `?filter[status_internal]=POSTED` untuk exclude. |
| OQ-M2-5b | Apakah ROLE-AKUN bisa melihat jurnal dari periode lain (mis. 2025)? | **Ya** — tidak ada pembatasan periode untuk read. Filter optional untuk memudahkan navigasi. |

---

## Story P5-M2-S6 — DLQ Inspection + Replay

**Actor**: ROLE-IT-ADMIN, ROLE-AKUN-CTL
**Trigger**: Alert masuk (via audit log notification atau monitoring) bahwa ada jurnal posting yang gagal masuk ke `sys.dlq_jurnal_post`; atau ROLE-IT-ADMIN membuka halaman DLQ untuk monitoring rutin
**Goal**: Actor dapat melihat daftar entri DLQ, memahami penyebab kegagalan, memperbaiki kondisi (mis. approve mapping baru), dan me-replay posting jurnal yang gagal tanpa duplikasi

### Pre-conditions
1. `sys.dlq_jurnal_post` memiliki entri dengan `status = 'FAILED'`
2. Untuk replay: kondisi yang menyebabkan kegagalan sudah diperbaiki (mis. mapping sudah APPROVED, periode sudah di-reopen)
3. Replay membuat Asynq task baru menggunakan `payload_jsonb` dari DLQ entry (idempotency dijaga via `source_event_id + event_code` di `jrnl.header`)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `sys.dlq_jurnal_post` | READ / UPDATE (status, replayed_by, replayed_at) | |
| `jrnl.header` | READ | Cek apakah replay sudah sukses (replayed_jurnal_id) |
| `sys.job` | INSERT | Replay menghasilkan new Asynq job + job tracking |

### Permissions
| Permission | Role | Catatan |
|---|---|---|
| `jurnal.dlq.read` | ROLE-IT-ADMIN, ROLE-AKUN-CTL | List DLQ |
| `jurnal.dlq.replay` | ROLE-IT-ADMIN, ROLE-AKUN-CTL | Trigger replay |
| `jurnal.dlq.abandon` | ROLE-IT-ADMIN | Tandai ABANDONED (tidak bisa di-replay lagi) |

### Audit Events
| Action | Trigger |
|---|---|
| `JURNAL.DLQ_REPLAY` | Replay di-trigger oleh user |
| `JURNAL.DLQ_ABANDON` | Entry di-tandai ABANDONED |
| `JURNAL.POST` | Jika replay berhasil (in-transaction di Asynq worker) |

### Acceptance Criteria

```gherkin
Feature: DLQ inspection dan replay jurnal posting yang gagal

  Background:
    Given sys.dlq_jurnal_post memiliki 5 entries FAILED:
      | id      | source_event_type    | event_code            | error_code                | status |
      | DLQ-001 | penempatan:approved  | PENEMPATAN            | JURNAL_EVENT_NOT_MAPPED   | FAILED |
      | DLQ-002 | mtm:computed         | MTM_FVOCI             | JURNAL_PERIODE_CLOSED     | FAILED |
      | DLQ-003 | akrual:computed      | AKRUAL_BUNGA          | JURNAL_BALANCE_FAILED     | FAILED |
      | DLQ-004 | penempatan:matured   | JATUH_TEMPO           | JURNAL_EVENT_NOT_MAPPED   | FAILED |
      | DLQ-005 | ecl:charged          | ECL_PEMBENTUKAN       | JURNAL_KLASIFIKASI_MISMATCH| FAILED|
    And user ter-autentikasi sebagai ROLE-IT-ADMIN

  # ─── HAPPY PATH 1: Lihat DLQ list + filter ──────────────────────────────────

  Scenario: ROLE-IT-ADMIN membuka DLQ list
    When user mengakses GET /api/v1/jurnal/dlq?filter[status]=FAILED&sort=last_attempt_at:desc
    Then sistem mengembalikan HTTP 200 dengan 5 entries FAILED
    And setiap entry mengandung: id, source_event_type, event_code, error_code, error_message, attempt_count, last_attempt_at, status
    And UI menampilkan: badge warna merah untuk FAILED, amber untuk REPLAYING

  # ─── HAPPY PATH 2: Replay sukses setelah mapping diperbaiki ─────────────────

  Scenario: DLQ-001 di-replay setelah mapping PENEMPATAN berhasil di-approve
    Given mapping PENEMPATAN sudah ter-approve (problem DLQ-001 sudah terselesaikan)
    When ROLE-IT-ADMIN mengklik Replay pada DLQ-001
      (POST /api/v1/jurnal/dlq/DLQ-001/replay)
    Then `sys.dlq_jurnal_post` DLQ-001 status berubah ke 'REPLAYING'
    And Asynq task baru di-enqueue dengan payload dari DLQ-001.payload_jsonb
    And sys.job ter-INSERT untuk tracking progress replay
    And audit log: JURNAL.DLQ_REPLAY, actor = ROLE-IT-ADMIN, entity_id = DLQ-001

    When Asynq worker menjalankan replay task
    And posting berhasil (mapping tersedia, periode OPEN)
    Then jrnl.header baru ter-INSERT dengan idempotency_key unik
    And `sys.dlq_jurnal_post` DLQ-001:
      | status            | REPLAYED_OK           |
      | replayed_jurnal_id| new jrnl.header UUID  |
      | replayed_by       | ROLE-IT-ADMIN UUID    |
      | replayed_at       | timestamp now         |
    And audit log: JURNAL.POST (in-transaction dengan worker)
    And toast ke ROLE-IT-ADMIN: "Replay DLQ-001 berhasil. Jurnal JRN-2026-000099 diposting."

  # ─── ERROR CASE: Replay gagal lagi (kondisi belum diperbaiki) ────────────────

  Scenario: DLQ-002 di-replay tapi periode masih HARD_CLOSED
    Given periode Maret-2026 masih HARD_CLOSED
    When ROLE-IT-ADMIN me-replay DLQ-002
    Then Asynq worker gagal lagi dengan JURNAL_PERIODE_CLOSED
    And `sys.dlq_jurnal_post` DLQ-002:
      | status        | FAILED (kembali)         |
      | attempt_count | 2 (incremented)          |
      | last_attempt_at | timestamp               |
    And audit log: JURNAL.POST_FAILED (attempt 2)
    And toast error: "Replay DLQ-002 gagal: Periode Maret-2026 masih HARD_CLOSED. Selesaikan periode reopen terlebih dahulu."

  # ─── HAPPY PATH 3: Abandon DLQ entry (no longer needed) ──────────────────────

  Scenario: ROLE-IT-ADMIN mengabaikan DLQ-003 (JURNAL_BALANCE_FAILED — bug di mapping sudah diperbaiki tapi entry sudah tidak relevan)
    When ROLE-IT-ADMIN mengirim POST /api/v1/jurnal/dlq/DLQ-003/abandon
      With body: { "reason": "Entry duplikat dengan DLQ-003-REV. Manual review shows balance sudah benar di versi baru." }
    Then `sys.dlq_jurnal_post` DLQ-003 status = 'ABANDONED'
    And entry tidak muncul di default DLQ list (filter FAILED/REPLAYING)
    And audit log: JURNAL.DLQ_ABANDON, actor = ROLE-IT-ADMIN, reason tersimpan
    And entry masih bisa diakses via `?filter[status]=ABANDONED` untuk audit trail
```

### Open Questions — Story 6
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M2-6a | Apakah ROLE-AKUN-CTL bisa replay DLQ atau hanya ROLE-IT-ADMIN? | **Keduanya** per permissions tabel di atas. ROLE-AKUN-CTL lebih tahu konteks domain. Konfirmasi ke BRD RACI §6.4. |
| OQ-M2-6b | Apakah ada batas maksimum attempt_count sebelum DLQ entry otomatis di-ABANDONED? | **Max 5 attempts**. Setelah attempt 5 gagal, status = ABANDONED + alert ke ROLE-IT-ADMIN + ROLE-AKUN-CTL. Konfigurabel via `sys.config_param`. |
| OQ-M2-6c | Notifikasi DLQ — push in-app atau email? | **In-app badge** (notification center) + email ke ROLE-AKUN-CTL dan ROLE-IT-ADMIN setelah 1 jam pertama failure. Konfirmasi ke integration-engineer (email SMTP P5-M13). |

---

## Ringkasan P5-M2 Story Set

| Story | Judul | Actor Utama | AC Count | Workflow | Gate |
|---|---|---|---|---|---|
| P5-M2-S1 | Mapping Jurnal CRUD (6-eyes/4-eyes) | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK | 6 | 6-eyes regulated / 4-eyes operational | compliance BLOCKING + security BLOCKING |
| P5-M2-S2 | Jurnal Resolver single event | Asynq worker / service internal | 4 | — (service call) | compliance BLOCKING |
| P5-M2-S3 | Jurnal Posting via Asynq subscribers | Asynq worker (system) | 4 | Event-driven | compliance BLOCKING + security BLOCKING |
| P5-M2-S4 | Jurnal Posting Manual (ad-hoc) | ROLE-AKUN + ROLE-AKUN-CTL | 3 | 4-eyes | compliance BLOCKING |
| P5-M2-S5 | Read Jurnal list + drill-down | ROLE-AKUN/CTL/CFO/AUDIT/RISK | 4 | — (read-only) | advisory |
| P5-M2-S6 | DLQ Inspection + Replay | ROLE-IT-ADMIN + ROLE-AKUN-CTL | 4 | replay Asynq | security BLOCKING (audit trail) |
| **Total** | | | **25** | | |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| P5-M1 `penempatan:approved/matured/terminated` | P5-M1 → P5-M2 | P5-M2 Story 3 mengkonsumsi events ini |
| P5-M6 `mtm:computed` | P5-M6 → P5-M2 | MTM jurnal posting |
| P5-M7 `renewal:approved` | P5-M7 → P5-M2 | RENEWAL_DEPOSITO jurnal |
| P5-M9 `akrual:computed` | P5-M9 → P5-M2 | AKRUAL_BUNGA jurnal harian |
| P5-M10 `ecl:charged/reversed` | P5-M10 → P5-M2 | ECL_PEMBENTUKAN + ECL_REVERSAL + POCI_DELTA_ECL |
| P5-M3 GL delivery | P5-M2 → P5-M3 | P5-M3 mengkonsumsi `jrnl.header` rows dari P5-M2 |
| P5-M4 Periode close checklist | P5-M2 → P5-M4 | P5-M4 butuh jurnal balanced sebelum close |
| P5-M12 Frontend CRUD | P5-M2 → P5-M12 | P5-M12 UI untuk Story 1 + Story 5 |

---

## Compliance & Security Handoff Checklist

### Untuk ifrs9-compliance-reviewer
- [ ] Balance invariant enforcement (`total_debit == total_kredit` via CHECK constraint) — runtime guarantee
- [ ] Klasifikasi routing matrix: semua 27 event codes + semua klasifikasi PSAK 71 (AC/FVOCI/FVTPL/FVOCI_ELECTION/POCI) ter-covered di seed templates
- [ ] Regulated code list (DEC-P5-M1-003) diimplementasikan sebagai explicit whitelist server-side (bukan client-configurable)
- [ ] FVTPL tidak mendapat jurnal ECL_PEMBENTUKAN / ECL_REVERSAL (PSAK 71 §5.5.15)
- [ ] FVOCI_ELECTION tidak ada recycling (MTM_FVOCI_ELECTION vs MTM_FVOCI debit/kredit berbeda arah OCI)
- [ ] POCI_DELTA_ECL hanya untuk `klasifikasi_psak71 = 'POCI'` (DEC-POCI-002)
- [ ] Stage 3 net carrying: jika event AKRUAL_BUNGA untuk Stage 3, `sumber_amount = net_carrying_idr` bukan `nominal_idr`

### Untuk security-engineer
- [ ] `jrnl.header` + `jrnl.detail` TIDAK memiliki UPDATE/DELETE privilege di service layer — append-only enforced via DDL trigger BEFORE UPDATE (return error)
- [ ] `JURNAL.POST` ditulis ke `aud.audit_log` **di transaksi yang sama** dengan INSERT `jrnl.header`
- [ ] PERIODE_CLOSED bypass tidak bisa di-skip (server-side check, bukan hanya UI)
- [ ] `sys.dlq_jurnal_post.payload_jsonb` tidak mengandung PII (nomor rekening nasabah, NPWP) — validator sebelum INSERT
- [ ] DLQ replay tidak bisa duplikat jurnal — unique index `uq_jrnl_idempotency` adalah satu-satunya guard
- [ ] Audit `JURNAL.DLQ_REPLAY` ditulis dengan `actor_user_id`, `actor_role`, timestamp sebelum Asynq task di-enqueue (bukan setelah)
