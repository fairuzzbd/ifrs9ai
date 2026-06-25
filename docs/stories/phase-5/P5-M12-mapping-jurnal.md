# P5-M12 — APP-D Mapping Jurnal CRUD + 6-Eyes Workflow + RPT-19/20/21: User Stories

**Story Set ID**: P5-M12
**Modul**: APP-D — Periode Buku + FX + Mapping Jurnal
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `security-engineer` (BLOCKING); `ifrs9-compliance-reviewer` (BLOCKING — menyentuh integritas jurnal ECL/EIR/MTM/REKLAS)
**Author**: business-analyst
**Tanggal**: 2026-06-22
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §5 (APP-D Mapping Jurnal), FSD-APP-D (Jurnal Engine + GL Mapping)
**Linked BRD**: BRD §4.4 (APP-D jurnal mapping), §3 RACI: ROLE-AKUN (R), ROLE-AKUN-CTL (A-review), ROLE-RISK (A-approve regulated), ROLE-AUDIT (I), ROLE-IT-ADMIN (I config)
**Linked Decision Log**:
- `DEC-017` (LOCKED) — 6-eyes untuk klasifikasi PSAK 71 + parameter master; SoD `maker ≠ reviewer ≠ approver`
- `DEC-018` (LOCKED) — audit trail append-only, 10+10 tahun; soft-delete only; versi immutable
- `DEC-021` (LOCKED) — Idempotency-Key wajib di semua mutating endpoints
- `DEC-027` (LOCKED) — step-up MFA: ROLE-RISK approve mapping regulated (approver_2)

**Dependensi**:
- **P5-M2** — `mst.mapping_jurnal_header` + `mst.mapping_jurnal_detail` exist dengan 27 DRAFT seeds (EVT-001..027), workflow kolom (maker_id, reviewer_id, approver_id, approver_2_id, workflow_path, signature hashes), SoD CHECK constraints, state `APPROVED_ACTIVE`
- **P5-M3..M11** — event codes PENEMPATAN, MTM_*, JATUH_TEMPO, PENJUALAN_PENCAIRAN, RENEWAL_DEPOSITO, POCI_DELTA_ECL sudah seeded sebagai DRAFT; M12 = UI + workflow untuk mengisi akun_debit/kredit + aktivasi
- **`mst.chart_of_accounts`** — COA tabel referensi untuk validasi akun_debit/kredit setiap mapping detail

**Compliance path**: P5-M12 menyentuh **integritas jurnal yang dihasilkan ECL/EIR/MTM/REKLAS**. Mapping yang salah → jurnal salah → laporan keuangan salah. `ifrs9-compliance-reviewer` **BLOCKING** untuk merge. `security-engineer` **BLOCKING** untuk: audit in-tx, SoD enforce, step-up MFA, immutability versioning.

---

## Konteks & Arsitektur P5-M12

### State Machine `mst.mapping_jurnal_header.workflow_status`

```
[DRAFT seed dari P5-M2]
    ↓ POST /{id}/submit (ROLE-AKUN, maker)
PENDING_REVIEW
    ↓ POST /{id}/review (ROLE-AKUN-CTL, reviewer ≠ maker)
PENDING_APPROVAL          ← 4-eyes path (non-regulated events)
    ↓ POST /{id}/approve (ROLE-AKUN-CTL, approver ≠ reviewer ≠ maker)
APPROVED_ACTIVE           ← aktif_flag = TRUE; resolver dapat pakai

PENDING_REVIEW
    ↓ POST /{id}/review (ROLE-AKUN-CTL) [regulated event: 6-eyes path]
PENDING_APPROVAL_2
    ↓ POST /{id}/approve-2 (ROLE-RISK, approver_2 ≠ approver ≠ reviewer ≠ maker; step-up MFA)
APPROVED_ACTIVE           ← aktif_flag = TRUE

Reject di step manapun → kembali ke DRAFT (reject_reason wajib ≥ 30 char)
Edit APPROVED_ACTIVE → INSERT new version row (effective_from/to) → status DRAFT untuk version baru
                     → version lama: deleted_at = now(), aktif_flag tetap TRUE sampai new version APPROVED_ACTIVE
```

### Regulated vs Non-Regulated Events

| Kategori | Event Codes | Workflow |
|---|---|---|
| Regulated (6-eyes) | ECL_PEMBENTUKAN, ECL_REVERSAL, POCI_DELTA_ECL, MTM_FVTPL, MTM_FVOCI, MTM_FVOCI_ELECTION, REKLAS_OCI_PL, REKLASIFIKASI_AC_FVOCI, REKLASIFIKASI_FVOCI_AC, MODIFIKASI_MATERIAL, EIR_CATCH_UP_ADJUSTMENT, STAGE_MIGRATION, FX_UNREALIZED | 6-eyes (ROLE-RISK approver_2, MFA) |
| Operasional (4-eyes) | PENEMPATAN, AKRUAL_BUNGA, JATUH_TEMPO, PENJUALAN_PENCAIRAN, RENEWAL_DEPOSITO, PEMBAYARAN_BUNGA, PEMBAYARAN_POKOK, PENERIMAAN_DIVIDEN, DISTRIBUSI_REKSADANA, FX_REALIZED, AMORTISASI_PREMI_DISKONTO, PENGHAPUSAN, PERIODE_ADJUSTMENT, CORRECTION_PERIODE_CLOSED | 4-eyes (ROLE-AKUN-CTL approver) |

Daftar regulated event dikonfigurasi di `sys.config` key `REGULATED_EVENT_CODES` oleh ROLE-IT-ADMIN; backend whitelist baca dari config saat submit.

---

## Story P5-M12-S1 — Mapping CRUD UI

**Actor**: ROLE-AKUN (Maker — list, create, edit), ROLE-AUDIT (read-only)
**Trigger**: User mengakses `/mapping-jurnal` — DataTable semua event codes; klik event → detail view dengan `mst.mapping_jurnal_detail` rows (akun_debit, akun_kredit, debit_kredit, jumlah_calc formula). Create / edit → form baru mengisi `akun_debit`/`akun_kredit` per detail row; submit membuat version DRAFT. Edit APPROVED_ACTIVE → INSERT new version row (bukan UPDATE); versi lama disimpan untuk history immutable (DEC-018 mirror pattern DEC-013).
**Goal**: ROLE-AKUN dapat mengisi dan mengedit mapping per event code tanpa menyentuh jurnal live. DataTable per §1 (sort/filter/export). Per-event deep-link URL.

### Pre-conditions
1. User ter-autentikasi ROLE-AKUN dengan permission `jurnal.mapping.create`
2. `mst.mapping_jurnal_header` memiliki ≥ 1 row (seeded P5-M2)
3. `mst.chart_of_accounts` ter-populate (COA referensi validasi akun)
4. `mst.periode_buku.status_periode = 'OPEN'` atau `'SOFT_CLOSED'` (mapping changes cannot land saat HARD_CLOSED)

### Acceptance Criteria

```gherkin
Feature: Mapping Jurnal CRUD UI untuk ROLE-AKUN

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi
    And mst.mapping_jurnal_header HEADER-ECL-001: event_code='ECL_PEMBENTUKAN', workflow_status='DRAFT', workflow_path='6-eyes'
    And mst.mapping_jurnal_header HEADER-PNM-001: event_code='PENEMPATAN', workflow_status='APPROVED_ACTIVE', aktif_flag=TRUE

  Scenario: S1-AC1 — DataTable list semua event codes: sort + filter + export berjalan
    When USR-AKUN-001 GET /api/v1/mapping-jurnal?sort=event_code:asc&filter[workflow_status]=DRAFT
    Then HTTP 200:
      | data[].event_code          | ada (EVT-001..027 ditampilkan per filter) |
      | data[].workflow_status     | DRAFT (filter aktif)                     |
      | data[].workflow_path       | 4-eyes atau 6-eyes per event             |
      | data[].aktif_flag          | false untuk DRAFT rows                   |
      | pagination.totalEstimate   | ≥ 1                                      |
    And header kolom event_code, nama_event, workflow_status, workflow_path, aktif_flag ter-sort asc
    And export CSV: GET /api/v1/mapping-jurnal/export?format=csv → file dengan filter aktif, MIME text/csv
    And aud.audit_log.action = MAPPING.EXPORT jika export dijalankan — in-transaction
    And URL state deep-link: /mapping-jurnal?sort=event_code:asc&filter[workflow_status]=DRAFT (bookmarkable)

  Scenario: S1-AC2 — Per-event detail view: daftar mapping_jurnal_detail (akun, debit_kredit, formula)
    When USR-AKUN-001 GET /api/v1/mapping-jurnal/HEADER-PNM-001
    Then HTTP 200:
      | data.event_code             | PENEMPATAN                               |
      | data.workflow_status        | APPROVED_ACTIVE                          |
      | data.aktif_flag             | true                                     |
      | data.detail[].akun_debit    | kode akun COA non-null                   |
      | data.detail[].akun_kredit   | kode akun COA non-null                   |
      | data.detail[].debit_kredit  | 'D' atau 'K'                             |
      | data.detail[].jumlah_calc   | formula string (mis. 'EAD * PD_LIFETIME') |
      | data.version_history        | list semua versi (effective_from/to)     |
    And USR-AKUN-001 dapat klik "Edit" → form edit terbuka dengan data lama pre-filled
    And badge "APPROVED_ACTIVE" ditampilkan dengan WCAG AA contrast ratio ≥ 4.5:1

  Scenario: S1-AC3 — Create mapping detail baru untuk DRAFT event: INSERT version row
    Given HEADER-ECL-001: workflow_status='DRAFT', 0 detail rows
    When USR-AKUN-001 POST /api/v1/mapping-jurnal/HEADER-ECL-001/detail
      With Idempotency-Key: IK-DETAIL-001
      With body: { akun_debit: "110201", akun_kredit: "440101", debit_kredit: "D", jumlah_calc: "ECL_weighted", urutan: 1 }
    Then HTTP 201:
      | data.id              | DETAIL-ECL-001              |
      | data.header_id       | HEADER-ECL-001              |
      | data.akun_debit      | 110201                      |
      | data.akun_kredit     | 440101                      |
    And mst.mapping_jurnal_detail INSERT: akun_debit, akun_kredit validated terhadap mst.chart_of_accounts
    And aud.audit_log.action = MAPPING.DETAIL_CREATED — in-transaction
      With after_jsonb: { header_id, akun_debit, akun_kredit, jumlah_calc, actor: USR-AKUN-001 }
    And toast: "Detail mapping ECL_PEMBENTUKAN (baris 1) berhasil ditambahkan. Status: DRAFT."

  Scenario: S1-AC4 — Edit APPROVED_ACTIVE mapping: INSERT new version row, versi lama preserved
    Given HEADER-PNM-001: workflow_status='APPROVED_ACTIVE', version=1, effective_to=NULL
    When USR-AKUN-001 POST /api/v1/mapping-jurnal/HEADER-PNM-001/new-version
      With Idempotency-Key: IK-VERSION-001
      With body: { reason: "Perubahan kode akun kas sesuai COA baru per Juli 2026", detail: [...updated rows...] }
    Then HTTP 201:
      | data.id              | HEADER-PNM-002 (new version)      |
      | data.parent_id       | HEADER-PNM-001                    |
      | data.workflow_status | DRAFT                             |
      | data.effective_from  | tanggal pengajuan                 |
      | data.effective_to    | NULL (akan diisi saat APPROVED)   |
    And HEADER-PNM-001: effective_to = tanggal effective_from new version; aktif_flag tetap TRUE (resolver pakai sampai new version APPROVED_ACTIVE)
    And HEADER-PNM-001 tidak di-UPDATE (bukan UPDATE) — immutable history (DEC-018)
    And aud.audit_log.action = MAPPING.VERSION_CREATED — in-transaction
      With after_jsonb: { parent_id: HEADER-PNM-001, new_version_id: HEADER-PNM-002, reason }
    And toast: "Versi baru mapping PENEMPATAN berhasil dibuat (DRAFT). Versi lama aktif sampai versi baru disetujui."
```

---

## Story P5-M12-S2 — 6-Eyes Approval Workflow

**Actor**: ROLE-AKUN (Maker — submit), ROLE-AKUN-CTL (Reviewer + 4-eyes Approver), ROLE-RISK (6-eyes Approver-2, MFA step-up)
**Trigger**: Maker POST `/{id}/submit` → status PENDING_REVIEW. ROLE-AKUN-CTL POST `/{id}/review` → PENDING_APPROVAL (4-eyes) atau PENDING_APPROVAL_2 (6-eyes per workflow_path). ROLE-AKUN-CTL POST `/{id}/approve` (4-eyes path) → APPROVED_ACTIVE. ROLE-RISK POST `/{id}/approve-2` (6-eyes path, step-up MFA DEC-027) → APPROVED_ACTIVE; aktif_flag = TRUE. Reject di step manapun → DRAFT, reject_reason ≥ 30 char. Periode lock di step approve/approve-2 (jika periode HARD_CLOSED → block).
**Goal**: Regulated events mendapat 4-tier sign-off (M ≠ R ≠ A ≠ A2). Non-regulated 3-tier. SoD enforced server-side. MFA step-up ROLE-RISK wajib.

### Pre-conditions
1. `mst.mapping_jurnal_header.workflow_status = 'DRAFT'` dan semua detail rows tidak ada akun null
2. SoD: maker_id ≠ reviewer_id ≠ approver_id ≠ approver_2_id
3. `mst.periode_buku.status_periode IN ('OPEN', 'SOFT_CLOSED')` — approve/approve-2 tolak jika HARD_CLOSED
4. Idempotency-Key wajib di setiap step

### Acceptance Criteria

```gherkin
Feature: 6-eyes workflow approval mapping jurnal

  Background:
    Given HEADER-ECL-001: event_code='ECL_PEMBENTUKAN', workflow_path='6-eyes', workflow_status='DRAFT'
    And maker=USR-AKUN-001, reviewer kandidat=USR-AKUN-CTL-001, approver=USR-AKUN-CTL-002, approver_2=USR-RISK-001
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'

  Scenario: S2-AC1 — 6-eyes full flow: DRAFT→PENDING_REVIEW→PENDING_APPROVAL_2→APPROVED_ACTIVE dengan MFA
    When USR-AKUN-001 POST /api/v1/mapping-jurnal/HEADER-ECL-001/submit
      With Idempotency-Key: IK-SUBMIT-001
      With body: { comment: "Akun debit/kredit ECL_PEMBENTUKAN sudah diisi sesuai COA" }
    Then HTTP 200: workflow_status = 'PENDING_REVIEW', submit_at = now()
    And aud.audit_log.action = MAPPING.SUBMITTED — in-transaction

    When USR-AKUN-CTL-001 POST /api/v1/mapping-jurnal/HEADER-ECL-001/review
      With Idempotency-Key: IK-REVIEW-001
      With body: { comment: "Akun diverifikasi ke COA — lanjut ke RISK approval", signature_method: "JWT_STEP_UP" }
    Then HTTP 200: workflow_status = 'PENDING_APPROVAL_2' (6-eyes karena ECL_PEMBENTUKAN regulated)
    And reviewer_id = USR-AKUN-CTL-001, reviewer_signed_at = now(), reviewer_signature_hash = SHA-256(...)
    And aud.audit_log.action = MAPPING.REVIEWED — in-transaction

    When USR-RISK-001 POST /api/v1/mapping-jurnal/HEADER-ECL-001/approve-2
      With Idempotency-Key: IK-APPR2-001
      With X-Step-Up-Token: <valid MFA step-up token>
      With body: { comment: "Akun ECL mapping sesuai PSAK 71 §5.5 — disetujui", signature_method: "JWT_STEP_UP" }
    Then HTTP 200:
      | data.workflow_status     | APPROVED_ACTIVE                       |
      | data.aktif_flag          | true                                  |
      | data.approver_2_id       | USR-RISK-001                          |
      | data.approver_2_signed_at | now()                                |
    And approver_2_signature_hash = SHA-256(USR-RISK-001 || 'APPROVE_2' || HEADER-ECL-001 || approver_2_signed_at || comment)
    And aud.audit_log.action = MAPPING.APPROVED_ACTIVE — in-transaction
      With after_jsonb: { header_id, event_code, approver_2_id, mfa_method: "TOTP" }
    And toast ke USR-RISK-001: "Mapping ECL_PEMBENTUKAN disetujui dan aktif. Resolver dapat menggunakan template ini."

  Scenario: S2-AC2 — 4-eyes flow untuk event non-regulated: PENDING_REVIEW→PENDING_APPROVAL→APPROVED_ACTIVE
    Given HEADER-PNM-002: event_code='PENEMPATAN', workflow_path='4-eyes', workflow_status='PENDING_REVIEW'
    And reviewer = USR-AKUN-CTL-001 (sudah review), approver kandidat = USR-AKUN-CTL-002
    When USR-AKUN-CTL-002 POST /api/v1/mapping-jurnal/HEADER-PNM-002/approve
      With Idempotency-Key: IK-APPR-001
      With body: { comment: "Verified — akun penempatan sesuai buku besar", signature_method: "JWT_STEP_UP" }
    Then HTTP 200: workflow_status = 'APPROVED_ACTIVE', aktif_flag = TRUE
    And approver_id = USR-AKUN-CTL-002 (≠ reviewer USR-AKUN-CTL-001 ≠ maker USR-AKUN-001 — SoD OK)
    And aud.audit_log.action = MAPPING.APPROVED_ACTIVE — in-transaction

  Scenario: S2-AC3 — SoD violation: reviewer mencoba menjadi approver — MAPPING_SOD_VIOLATION
    Given HEADER-PNM-002: workflow_status='PENDING_APPROVAL', reviewer_id=USR-AKUN-CTL-001
    When USR-AKUN-CTL-001 (sama dengan reviewer) POST /approve
      With Idempotency-Key: IK-APPR-SOD-001
    Then HTTP 403:
      | error.code    | MAPPING_SOD_VIOLATION                                            |
      | error.message | "SoD: reviewer tidak dapat menjadi approver untuk mapping yang sama (DEC-017)." |
    And mst.mapping_jurnal_header: workflow_status tidak berubah
    And aud.audit_log.action = MAPPING.SOD_VIOLATION_ATTEMPT — in-transaction

  Scenario: S2-AC4 — Approve-2 tanpa step-up MFA: 403; periode HARD_CLOSED saat approve-2: MAPPING_PERIODE_LOCKED
    Given USR-RISK-001 mencoba approve-2 HEADER-ECL-001 tanpa X-Step-Up-Token
    When POST /api/v1/mapping-jurnal/HEADER-ECL-001/approve-2 tanpa step-up header
    Then HTTP 403:
      | error.code    | FORBIDDEN                                                        |
      | error.message | "Approve-2 mapping regulated memerlukan step-up MFA (DEC-027). Re-autentikasi MFA." |
    And periode HARD_CLOSED guard: jika mst.periode_buku.status_periode = 'HARD_CLOSED'
      Then HTTP 423:
        | error.code    | MAPPING_PERIODE_LOCKED                                                             |
        | error.message | "Periode buku HARD_CLOSED. Perubahan mapping tidak dapat diaktifkan di periode ini." |
```

---

## Story P5-M12-S3 — Import/Export XLSX Bulk Mapping

**Actor**: ROLE-AKUN (export current + upload XLSX), ROLE-AKUN-CTL (review per batch), ROLE-RISK (approve-2 regulated rows per batch)
**Trigger**: Export — `GET /api/v1/mapping-jurnal/export?format=xlsx` → stream XLSX berisi semua ACTIVE mapping + detail rows. Import — `POST /api/v1/mapping-jurnal/bulk-import` dengan XLSX; re-uses `sys.upload_batch` pattern (batch_type='MAPPING_BULK') dari P5-M11; 4-stage validation; auto-creates DRAFT version per row; ROLE-AKUN-CTL + ROLE-RISK 6-eyes per batch aggregate (regulated rows).
**Goal**: Operator dapat migrasi mapping COA serentak via XLSX tanpa re-enter satu-per-satu. Semua row yang hasil import tetap harus melalui workflow approval.

### Pre-conditions
1. ROLE-AKUN dengan permission `jurnal.mapping.create` + `jurnal.mapping.export`
2. `mst.chart_of_accounts` ter-populate (validasi akun saat import)
3. Import: file XLSX ≤ 20MB, format: kolom event_code, akun_debit, akun_kredit, debit_kredit, jumlah_calc, urutan
4. Tidak ada batch MAPPING_BULK `status=RUNNING` milik user yang sama

### Acceptance Criteria

```gherkin
Feature: Import/Export XLSX bulk mapping jurnal

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi
    And mst.chart_of_accounts: kode akun 110201, 440101, 220301 ter-populate

  Scenario: S3-AC1 — Export XLSX: semua ACTIVE mapping + detail, respect filter
    When USR-AKUN-001 GET /api/v1/mapping-jurnal/export?format=xlsx&filter[workflow_status]=APPROVED_ACTIVE
    Then HTTP 200 streaming XLSX:
      | Content-Disposition | attachment; filename="mapping-jurnal-20260622.xlsx"         |
      | Sheet "Header"      | event_code, nama_event, workflow_path, aktif_flag per row   |
      | Sheet "Detail"      | event_code FK, akun_debit, akun_kredit, debit_kredit, formula |
    And header baris: Bahasa Indonesia (mis. "Kode Event", "Akun Debit", "Akun Kredit")
    And dataset > 0 rows karena filter APPROVED_ACTIVE (hanya ACTIVE rows di-export)
    And aud.audit_log.action = MAPPING.EXPORT — in-transaction
      With after_jsonb: { format: "xlsx", row_count: N, filter: { workflow_status: "APPROVED_ACTIVE" } }

  Scenario: S3-AC2 — Import XLSX: parse + 4-stage validation; valid rows → DRAFT versions
    Given file mapping_bulk_update.xlsx: 15 rows (12 valid, 3 akun tidak ada di COA)
    When USR-AKUN-001 POST /api/v1/mapping-jurnal/bulk-import
      With Idempotency-Key: IK-IMPORT-001
      With multipart file: mapping_bulk_update.xlsx
    Then HTTP 202:
      | data.batch_id    | BATCH-MAP-001                     |
      | data.batch_type  | MAPPING_BULK                      |
      | data.total_rows  | 15                                |
      | data.valid_rows  | 12                                |
      | data.invalid_rows | 3                                |
      | data.errors      | [{ row: 5, col: "akun_debit", error: "Akun 999999 tidak ditemukan di COA." }, ...] |
    And sys.upload_batch INSERT: batch_type='MAPPING_BULK', status='PARSED'
    And 12 valid rows: auto-create new DRAFT version di mst.mapping_jurnal_header (INSERT, tidak UPDATE existing ACTIVE)
    And 3 invalid rows: sys.upload_batch_row.row_status = 'FAILED', tidak di-INSERT ke mapping_jurnal_header
    And aud.audit_log.action = MAPPING.BULK_IMPORTED — in-transaction
      With after_jsonb: { batch_id, valid_rows: 12, invalid_rows: 3 }
    And toast: "Import mapping berhasil diparsing. 12 baris valid (DRAFT dibuat), 3 baris gagal. Lihat detail batch BATCH-MAP-001."

  Scenario: S3-AC3 — Akun tidak ada di COA: MAPPING_AKUN_INVALID per row; batch tetap parseable
    Given file import: row 8 akun_debit = "999888" tidak ada di mst.chart_of_accounts
    When 4-stage validation stage 3 (cross-ref COA) memproses row 8
    Then sys.upload_batch_row row 8: row_status = 'FAILED'
      | error_detail | "MAPPING_AKUN_INVALID: akun_debit '999888' tidak ditemukan di Chart of Accounts." |
    And rows lain tidak terpengaruh (partial valid batch dilanjut)
    And response errors[]: [{ row: 8, error_code: "MAPPING_AKUN_INVALID", col: "akun_debit", value: "999888" }]

  Scenario: S3-AC4 — Debit/kredit tidak balance per event: MAPPING_UNBALANCED tolak row
    Given file import: row 11 event MTM_FVOCI — total debit lines ≠ total kredit lines (unbalanced)
    When validasi stage 4 (balance check) memproses row 11
    Then sys.upload_batch_row row 11: row_status = 'FAILED'
      | error_detail | "MAPPING_UNBALANCED: total debit 2 lines ≠ total kredit 1 line untuk event MTM_FVOCI. Jurnal harus balanced." |
    And row 11 tidak di-INSERT ke mst.mapping_jurnal_header
    And regulated events di batch (MTM_FVOCI = 6-eyes): DRAFT versions yang berhasil dibuat di-route ke 6-eyes workflow (ROLE-AKUN-CTL review + ROLE-RISK approve-2)
```

---

## Story P5-M12-S4 — RPT-19 Mapping Coverage Dashboard

**Actor**: ROLE-AKUN (view + export), ROLE-AKUN-CTL (view), ROLE-RISK (view), ROLE-AUDIT (read)
**Trigger**: `GET /api/v1/reports/rpt-19-mapping-coverage` — per event code: count ACTIVE mapping + detail completeness. Badge `GAP_COVERAGE` untuk event tanpa mapping APPROVED_ACTIVE atau dengan akun_debit/kredit null. Triggered oleh jurnal calls dari M2/M6/M8/M9 yang gagal karena `JURNAL_EVENT_NOT_MAPPED` → highlight event tersebut di dashboard.
**Goal**: Operator melihat sekaligus event mana yang belum mapped; gap alert agar tidak ada jurnal gagal karena mapping kosong.

### Pre-conditions
1. User ter-autentikasi dengan permission `jurnal.mapping.read`
2. `mst.mapping_jurnal_header` sudah ada (P5-M2 seeds)
3. Asynq DLQ (`sys.dlq_jurnal_post`) dipakai sebagai feed data event yang gagal karena NOT_MAPPED

### Acceptance Criteria

```gherkin
Feature: RPT-19 Mapping Coverage Dashboard

  Background:
    Given 27 event codes seeded P5-M2; 5 APPROVED_ACTIVE, 22 masih DRAFT
    And sys.dlq_jurnal_post: 3 entries dengan error_code='JURNAL_EVENT_NOT_MAPPED' untuk ECL_PEMBENTUKAN, MTM_FVOCI, JATUH_TEMPO

  Scenario: S4-AC1 — Coverage summary: ACTIVE count + missing events ditampilkan
    When ROLE-AKUN GET /api/v1/reports/rpt-19-mapping-coverage
    Then HTTP 200:
      | data.total_events            | 27                                                |
      | data.active_events           | 5                                                 |
      | data.missing_events          | 22 (workflow_status ≠ APPROVED_ACTIVE)            |
      | data.gap_events[].event_code | [ECL_PEMBENTUKAN, MTM_FVOCI, JATUH_TEMPO, ...]   |
      | data.gap_events[].last_dlq_error | timestamp DLQ terakhir (jika ada)            |
    And badge GAP_COVERAGE: merah untuk event tanpa APPROVED_ACTIVE mapping; hijau jika APPROVED_ACTIVE
    And badge WCAG AA (contrast ≥ 4.5:1)
    And link tiap event → deep-link ke /mapping-jurnal?filter[event_code]=ECL_PEMBENTUKAN

  Scenario: S4-AC2 — Event dengan detail akun null flagged sebagai incomplete meski APPROVED_ACTIVE
    Given HEADER-PNMV2: workflow_status='APPROVED_ACTIVE' tapi mst.mapping_jurnal_detail.akun_debit IS NULL untuk 1 row
    When RPT-19 di-render untuk event PENEMPATAN
    Then data.gap_events[] includes PENEMPATAN:
      | reason | "MAPPING_AKUN_INVALID: 1 detail row dengan akun_debit null di version APPROVED_ACTIVE" |
    And badge GAP_COVERAGE untuk PENEMPATAN: kuning ("Incomplete — ada akun null")

  Scenario: S4-AC3 — Export RPT-19: CSV + XLSX per §1; dataset < 10k → inline
    When ROLE-AKUN GET /api/v1/reports/rpt-19-mapping-coverage/export?format=xlsx
    Then HTTP 200 streaming XLSX:
      | Sheet "Coverage" | event_code, status, active_count, missing_detail_count, last_dlq_at |
    And aud.audit_log.action = MAPPING.RPT19_EXPORTED — in-transaction

  Scenario: S4-AC4 — DLQ-linked events: klik event di dashboard → filter DLQ untuk event tersebut
    Given sys.dlq_jurnal_post: 3 entries error_code='JURNAL_EVENT_NOT_MAPPED', event_code='MTM_FVOCI'
    When ROLE-AKUN klik badge GAP_COVERAGE untuk MTM_FVOCI
    Then navigasi ke /jurnal/dlq?filter[event_code]=MTM_FVOCI&filter[error_code]=JURNAL_EVENT_NOT_MAPPED
    And DLQ browser tampil 3 entries untuk MTM_FVOCI — link replay ke S2 workflow approval
```

---

## Story P5-M12-S5 — RPT-20 Mapping Validation Report + RPT-21 Change History

**Actor**: ROLE-AKUN (RPT-20 view), ROLE-AUDIT (RPT-21 view + export), ROLE-RISK (RPT-20 review sebelum approve-2)
**Trigger**: RPT-20 `GET /api/v1/reports/rpt-20-mapping-validation` — verify setiap ACTIVE mapping: akun non-null, debit/kredit balanced, akun valid di COA. RPT-21 `GET /api/v1/reports/rpt-21-mapping-history` — audit log filter `action LIKE 'MAPPING.%'` digroup per event_code, urut waktu. Export CSV+XLSX per §1. ROLE-RISK menggunakan RPT-20 sebagai checklist sebelum approve-2.
**Goal**: RPT-20 = pre-flight validation formal. RPT-21 = audit trail semua perubahan mapping untuk auditor eksternal + compliance.

### Pre-conditions
1. Permission: RPT-20 → `jurnal.mapping.read`; RPT-21 → `audit_log.read` (ROLE-AUDIT)
2. `aud.audit_log` ter-populate dengan `MAPPING.*` actions dari S1-S4
3. `mst.chart_of_accounts` tersedia untuk COA cross-check

### Acceptance Criteria

```gherkin
Feature: RPT-20 Mapping Validation + RPT-21 Change History

  Background:
    Given 5 APPROVED_ACTIVE mapping headers dengan total 18 detail rows
    And 1 detail row: akun_kredit null (incomplete)
    And 1 mapping: total debit lines ≠ kredit lines (unbalanced)
    And aud.audit_log: 24 MAPPING.* actions dari USR-AKUN-001, USR-AKUN-CTL-001, USR-RISK-001

  Scenario: S5-AC1 — RPT-20: per ACTIVE mapping — validasi akun non-null + balanced + COA valid
    When ROLE-RISK GET /api/v1/reports/rpt-20-mapping-validation
    Then HTTP 200:
      | data.total_active_mappings    | 5                                     |
      | data.valid_mappings           | 3                                     |
      | data.invalid_mappings         | 2                                     |
      | data.issues[].event_code      | [PENEMPATAN, ECL_PEMBENTUKAN]         |
      | data.issues[0].error_codes    | [MAPPING_AKUN_INVALID]                |
      | data.issues[1].error_codes    | [MAPPING_UNBALANCED]                  |
    And tiap issue: link → /mapping-jurnal/{header_id} per-event detail view (S1-AC2)
    And ROLE-RISK dapat pakai RPT-20 sebagai pre-checklist sebelum POST /approve-2

  Scenario: S5-AC2 — RPT-20 export XLSX; hanya invalid rows di export "issues" sheet
    When ROLE-AKUN GET /api/v1/reports/rpt-20-mapping-validation/export?format=xlsx
    Then HTTP 200 streaming XLSX:
      | Sheet "Semua Mapping"  | event_code, status, detail_count, balanced, akun_valid |
      | Sheet "Issues"         | hanya mapping dengan error; event_code, error_code, detail_row, keterangan |
    And aud.audit_log.action = MAPPING.RPT20_EXPORTED — in-transaction

  Scenario: S5-AC3 — RPT-21: audit change history per event_code, urut waktu desc
    When ROLE-AUDIT GET /api/v1/reports/rpt-21-mapping-history?filter[event_code]=ECL_PEMBENTUKAN
    Then HTTP 200:
      | data[].action       | MAPPING.SUBMITTED, MAPPING.REVIEWED, MAPPING.APPROVED_ACTIVE, ... |
      | data[].actor_role   | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK                               |
      | data[].event_time   | urut desc                                                          |
      | data[].before_jsonb | state sebelum aksi (null untuk CREATED)                           |
      | data[].after_jsonb  | state sesudah aksi                                                 |
    And filter berjalan per §1: sort, cursor-paging, export CSV/XLSX
    And ROLE-AUDIT (read-only): tidak ada tombol mutasi di UI

  Scenario: S5-AC4 — RPT-21 export CSV: semua MAPPING.* actions; dataset > 10k → async job
    Given aud.audit_log.action LIKE 'MAPPING.%': 15.000 entries
    When ROLE-AUDIT GET /api/v1/reports/rpt-21-mapping-history/export?format=csv
    Then HTTP 202 (async karena > 10k rows per §1 export rule):
      | data.jobId     | JOB-RPT21-001                           |
      | data.statusUrl | /api/v1/jobs/JOB-RPT21-001              |
    And Asynq worker stream CSV ke MinIO bucket 'exports/' + notif download link setelah selesai
    And aud.audit_log.action = MAPPING.RPT21_EXPORTED — in-transaction (saat job complete)
    And toast saat selesai: "RPT-21 export selesai. 15.000 baris siap diunduh (TTL 24 jam)."
```

---

## Ringkasan P5-M12 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M12-S1 | Mapping CRUD UI | ROLE-AKUN, ROLE-AUDIT | 4 | **security-engineer BLOCKING** (audit in-tx, immutability versioning, idempotency) |
| P5-M12-S2 | 6-eyes approval workflow | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK | 4 | **security-engineer BLOCKING** (SoD 4-way, step-up MFA); **ifrs9-compliance-reviewer BLOCKING** |
| P5-M12-S3 | Import/Export XLSX bulk mapping | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK | 4 | **security-engineer BLOCKING** (idempotency, audit); **ifrs9-compliance-reviewer BLOCKING** |
| P5-M12-S4 | RPT-19 Mapping Coverage Dashboard | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT | 4 | advisory (non-blocking) |
| P5-M12-S5 | RPT-20 Validation + RPT-21 Change History | ROLE-AKUN, ROLE-AUDIT, ROLE-RISK | 4 | **ifrs9-compliance-reviewer BLOCKING** (RPT-20 = pre-flight formal validation) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `MAPPING_EVENT_NOT_FOUND` | 404 | event_code tidak ada di mst.mapping_jurnal_header | Sudah ada di jrnl engine sebagai `JURNAL_EVENT_NOT_MAPPED`; alias baru untuk mapping CRUD context |
| `MAPPING_AKUN_INVALID` | 422 | akun_debit atau akun_kredit tidak ada di mst.chart_of_accounts | Validasi saat detail create + import stage 3 |
| `MAPPING_UNBALANCED` | 422 | Total debit lines ≠ kredit lines untuk satu event mapping | Validasi saat submit + import stage 4 |
| `MAPPING_REGULATED_REQUIRES_RISK` | 422 | Event regulated di-submit ke 4-eyes path atau approve-2 dilakukan non-ROLE-RISK | Server cek whitelist `REGULATED_EVENT_CODES` dari sys.config |
| `MAPPING_DUPLICATE_VERSION` | 409 | New version untuk event_code sudah ada dalam status DRAFT atau PENDING_* (conflict) | Idempotency guard — satu event tidak boleh punya 2 in-flight versions |
| `MAPPING_SOD_VIOLATION` | 403 | maker=reviewer, reviewer=approver, atau approver=approver_2 | DEC-017; server-side enforce sebelum DB constraint |
| `MAPPING_PERIODE_LOCKED` | 423 | Approve/approve-2 dilakukan saat mst.periode_buku HARD_CLOSED | Mapping changes tidak landing di periode HARD_CLOSED |

Catatan: `FORBIDDEN` (403), `SOD_VIOLATION` (403), `IDEMPOTENCY_REPLAY` (200), `IDEMPOTENCY_MISMATCH` (422), `WORKFLOW_INVALID_TRANSITION` (422), `PERIODE_CLOSED` (423) sudah ada di api-conventions.md.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M12 | MFA Level |
|---|---|---|---|
| ROLE-AKUN | `jurnal.mapping.create`, `jurnal.mapping.read`, `jurnal.mapping.export` | CRUD UI (S1), Import (S3), Export (S3), Submit workflow (S2), RPT-19/20 view (S4/S5) | Tidak wajib |
| ROLE-AKUN-CTL | `jurnal.mapping.review`, `jurnal.mapping.approve` | Reviewer 6-eyes (S2), 4-eyes Approver (S2), Batch review import (S3) | WAJIB (DEC-026) |
| ROLE-RISK | `jurnal.mapping.approve_2`, `jurnal.mapping.read` | Approver-2 regulated 6-eyes + step-up MFA (S2), RPT-20 pre-checklist (S5) | WAJIB + step-up (DEC-027) |
| ROLE-AUDIT | `audit_log.read`, `jurnal.mapping.read` | Read-only semua mapping + RPT-21 export (S5) | Tidak wajib |
| ROLE-IT-ADMIN | `sys.config.update` | Update `REGULATED_EVENT_CODES` whitelist di sys.config | WAJIB (DEC-026) |

---

## Audit Events Summary

| Event | Trigger | In-transaction |
|---|---|---|
| `MAPPING.DETAIL_CREATED` | POST /{id}/detail (S1-AC3) | Ya |
| `MAPPING.VERSION_CREATED` | POST /{id}/new-version (S1-AC4) | Ya |
| `MAPPING.EXPORT` | GET /export atau /export?format=xlsx (S3-AC1) | Ya |
| `MAPPING.SUBMITTED` | POST /{id}/submit (S2-AC1) | Ya |
| `MAPPING.REVIEWED` | POST /{id}/review (S2-AC1) | Ya |
| `MAPPING.APPROVED_ACTIVE` | POST /{id}/approve atau /approve-2 (S2-AC1, S2-AC2) | Ya |
| `MAPPING.REJECTED` | POST /{id}/reject di step manapun | Ya |
| `MAPPING.SOD_VIOLATION_ATTEMPT` | SoD violation attempt (S2-AC3) | Ya |
| `MAPPING.BULK_IMPORTED` | POST /bulk-import selesai parse (S3-AC2) | Ya |
| `MAPPING.RPT19_EXPORTED` | RPT-19 export (S4-AC3) | Ya |
| `MAPPING.RPT20_EXPORTED` | RPT-20 export (S5-AC2) | Ya |
| `MAPPING.RPT21_EXPORTED` | RPT-21 export selesai (S5-AC4) | Ya (saat job complete) |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.mapping_jurnal_header` + `_detail` | P5-M2 → P5-M12 | 27 DRAFT seeds dengan workflow kolom sudah ada; M12 = UI + workflow to fill akun + aktivasi |
| `sys.upload_batch` + `sys.upload_batch_row` | P5-M11 → P5-M12 | Pattern re-used untuk bulk import (batch_type='MAPPING_BULK'); schema harus sudah ada dari M11 |
| `sys.dlq_jurnal_post` | P5-M2 → P5-M12-S4 | RPT-19 reads DLQ untuk highlight JURNAL_EVENT_NOT_MAPPED events |
| `mst.chart_of_accounts` | APP-A/D → P5-M12-S1,S3 | Validasi akun_debit/kredit; COA harus ter-populate sebelum M12 |
| `aud.audit_log` | P5-M12 → P5-M12-S5 | RPT-21 query `action LIKE 'MAPPING.%'` dari audit log |
| `sys.config` (REGULATED_EVENT_CODES) | P5-M2 seeded config → P5-M12-S2 | Backend whitelist dibaca saat submit; ROLE-IT-ADMIN dapat update |
| P5-M2/M6/M8/M9 jurnal calls | jrnl engine → S4 | `JURNAL_EVENT_NOT_MAPPED` dari engine memicu badge GAP_COVERAGE di RPT-19 |

---

## Handoff Berikutnya

- `system-analyst` → OpenAPI: 9 endpoints (`GET /mapping-jurnal`, `GET /mapping-jurnal/{id}`, `POST /mapping-jurnal/{id}/detail`, `POST /mapping-jurnal/{id}/new-version`, `POST /mapping-jurnal/{id}/submit`, `POST /mapping-jurnal/{id}/review`, `POST /mapping-jurnal/{id}/approve`, `POST /mapping-jurnal/{id}/approve-2`, `POST /mapping-jurnal/bulk-import`); state machine DRAFT→PENDING_REVIEW→PENDING_APPROVAL[_2]→APPROVED_ACTIVE; 7 error codes baru; RPT-19/20/21 report endpoints
- `data-modeler` → migration 000048: tambah kolom `parent_id UUID REFERENCES mst.mapping_jurnal_header(id)` + `effective_from TIMESTAMPTZ` + `effective_to TIMESTAMPTZ` ke `mst.mapping_jurnal_header` (version chain); index `(event_code, workflow_status, deleted_at)` partial; `sys.config` seed `REGULATED_EVENT_CODES` text array
- `security-engineer` → **BLOCKING**: SoD 4-way server-side enforce (M≠R≠A≠A2) untuk setiap step; step-up MFA approve-2 (DEC-027); audit semua 12 events in-transaction; immutability — NEVER UPDATE existing `APPROVED_ACTIVE` row header/detail, INSERT new version only (DEC-018); idempotency semua mutating endpoints; `MAPPING_PERIODE_LOCKED` enforce saat HARD_CLOSED
- `ifrs9-compliance-reviewer` → **BLOCKING**: mapping yang mengatur ECL/EIR/MTM/REKLAS jurnal = regulated path; RPT-20 balance check harus pass sebelum approve-2 dianggap valid; verifikasi MAPPING_UNBALANCED rule cover semua D/K balance scenarios per PSAK 71

_Story set ini siap dihandoff ke `system-analyst` (OpenAPI + state machine) dan `security-engineer` (BLOCKING). `data-modeler` mulai migration 000048 paralel. `ifrs9-compliance-reviewer` BLOCKING gate untuk S2/S3/S5 yang menyentuh integritas jurnal regulated. `uiux-designer` dapat mulai desain DataTable + workflow stepper paralel setelah SA selesai contract._
