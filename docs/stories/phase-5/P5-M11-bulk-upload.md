# P5-M11 — APP-A Bulk Upload Master Instrumen: User Stories

**Story Set ID**: P5-M11
**Modul**: APP-A — Master Data (Bulk Import Instrumen, Phase 5)
**Status**: DRAFT — menunggu handoff ke `system-analyst` + `security-engineer` (BLOCKING); advisory gate (non-blocking, tidak menyentuh ECL/EIR)
**Author**: business-analyst
**Tanggal**: 2026-06-21
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §3 (master instrumen); FSD-APP-A (SPPI + BM klasifikasi workflow)
**Linked BRD**: BRD §4.1 (APP-A master data input), §4.2 (workflow 4-eyes), RACI: ROLE-MAKER-TR (R), ROLE-APPR-TR (A), ROLE-CFO (A rollback), ROLE-RISK (R flagged klasifikasi), ROLE-AUDIT (I), ROLE-IT-ADMIN (I config)
**Linked Decision Log**:
- `DEC-017` (LOCKED) — 4-eyes workflow; SoD `maker ≠ reviewer ≠ approver`
- `DEC-018` (LOCKED) — audit trail append-only, retensi 10+10 tahun; soft-delete only
- `DEC-021` (LOCKED) — Idempotency-Key wajib di semua mutating endpoints
- `DEC-023` (LOCKED) — single tenant Phase 1; `tenant_id = 'TUGURE'` di setiap row
- `DEC-007` (LOCKED) — Asynq job queue; `sys.job` table untuk progress tracking

**Dependensi**:
- **P5-M1** — `mst.instrumen` target tabel + `klasifikasi_psak71` enum sudah ada
- **Phase 3 SPPI + BM auto-eval** — batch evaluasi SPPI + BM per row; hasilkan `klasifikasi_psak71` (AC/FVOCI/FVTPL); jika ambiguous → row di-flag `NEEDS_MANUAL_REVIEW`

**Compliance path**: P5-M11 tidak menyentuh ECL/EIR/SPPI/BM computation logic — hanya consume hasil Phase 3 auto-eval sebagai read. `ifrs9-compliance-reviewer` **tidak di jalur kritis (advisory, non-blocking)**. `security-engineer` **BLOCKING** untuk: audit in-transaction, idempotency, SoD enforce, soft-delete rollback, MFA step-up CFO.

---

## Konteks & Arsitektur P5-M11

### Bulk Upload Flow

```
ROLE-MAKER-TR upload XLSX 5-sheet (max 50MB, MIME XLSX zip)
    ↓
S1: Parse + validate file → sys.upload_batch (status=PARSED) + sys.upload_batch_row per row
    Audit: BULK.UPLOADED in-transaction

ROLE-MAKER-TR trigger DRY_RUN
    ↓
S2: 4-stage validation pipeline (tidak INSERT ke mst.instrumen)
    Stage 1: file format (cell type, mandatory cols)
    Stage 2: business rules (range, enum)
    Stage 3: cross-sheet refs (counterparty, bank, mata_uang)
    Stage 4: SPPI+BM auto-eval (Phase 3 service call per row)
    → preview { total_rows, valid_rows, invalid_rows, errors_per_row[], stage_summary }
    Cache DRY_RUN result 1 jam (TTL sys.upload_batch.dry_run_expires_at)
    Audit: BULK.VALIDATED_DRY_RUN in-transaction

ROLE-MAKER-TR trigger COMMIT (setelah DRY_RUN PASS, dalam TTL)
    ↓
S3: Asynq enqueue bulkupload:commit_instrumen
    sys.job row progress 0→100%
    Per row INSERT mst.instrumen (partial OK: failed rows skip + log)
    Audit: BULK.COMMITTED (full batch) atau BULK.PARTIAL_COMMIT (ada failed rows)

ROLE-APPR-TR approve batch (4-eyes SoD)
    ↓
S4: batch status BATCH→APPROVED; instrumen rows PENDING_APPROVAL_BULK→ACTIVE
    Audit: BULK.APPROVED in-transaction

[opsional] ROLE-CFO rollback dalam 7-hari grace window (step-up MFA)
    ↓
S5: Soft-delete semua instrumen dari batch; jurnal kompensasi jika ada
    Audit: BULK.ROLLBACK_REQUESTED + BULK.ROLLBACK_APPROVED in-transaction
```

### Sheet mapping XLSX 5-sheet

| Sheet | Jenis Instrumen | Kolom mandatory (contoh) |
|---|---|---|
| Deposito | deposito | kode, counterparty_id, bank_id, mata_uang, saldo, tanggal_penempatan, jatuh_tempo, bunga |
| Obligasi | obligasi | kode, issuer_id, mata_uang, nilai_nominal, kupon, tanggal_penerbitan, jatuh_tempo |
| Saham | saham | kode, emiten_id, mata_uang, jumlah_lembar, harga_beli |
| Reksadana | reksadana | kode, manajer_id, mata_uang, nilai_investasi, tanggal_investasi |
| Tabungan_Cash | tabungan/cash | kode, bank_id, mata_uang, saldo, tanggal_penempatan |

### Row status lifecycle

```
PENDING → COMMITTED (berhasil INSERT mst.instrumen)
        → FAILED    (validasi atau INSERT error; partial commit)
        → ROLLED_BACK (CFO rollback dalam grace window)
```

---

## Story P5-M11-S1 — Upload XLSX 5-Sheet + Parse

**Actor**: ROLE-MAKER-TR (upload), ROLE-AUDIT (read)
**Trigger**: `POST /api/v1/master/instrumen/bulk-upload` multipart/form-data dengan file XLSX. Server-side: (a) validasi file size ≤ 50MB, (b) validasi MIME = `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` (XLSX zip signature), (c) parse 5 sheet — collect parse errors per row per sheet, (d) INSERT `sys.upload_batch` (batch_type='INSTRUMEN_BULK', status=PARSED) dan `sys.upload_batch_row` per row dalam satu transaksi. Audit `BULK.UPLOADED` in-transaction. Return `batch_id` untuk tracking.
**Goal**: Setiap upload menghasilkan satu `sys.upload_batch` row dengan rows ter-capture; parse errors dilaporkan tanpa menyentuh `mst.instrumen`.

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-MAKER-TR dengan permission `instrumen.create`
2. File XLSX 5-sheet tersedia
3. `mst.periode_buku.status_periode = 'OPEN'` — jika CLOSED → `BULK_PERIODE_LOCKED` (tolak upload awal)
4. Tidak ada batch `status=RUNNING` milik user yang sama (prevent double submit)

### Acceptance Criteria

```gherkin
Feature: Upload XLSX 5-sheet master instrumen dan parse ke sys.upload_batch

  Background:
    Given ROLE-MAKER-TR (USR-MAKER-001) ter-autentikasi
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'

  Scenario: S1-AC1 — Upload XLSX valid: INSERT sys.upload_batch + sys.upload_batch_row
    Given file instrumen_bulk_jun2026.xlsx, ukuran 12MB, 5 sheet, 350 rows total
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload
      With Idempotency-Key: IK-UPLOAD-001
      With multipart file: instrumen_bulk_jun2026.xlsx
    Then HTTP 202:
      | data.batch_id   | BATCH-001                          |
      | data.status     | PARSED                              |
      | data.total_rows | 350                                 |
      | data.sheets     | [Deposito:80, Obligasi:120, Saham:60, Reksadana:50, Tabungan_Cash:40] |
      | data.parse_errors | []                               |
    And sys.upload_batch INSERT:
      | batch_id    | BATCH-001          |
      | batch_type  | INSTRUMEN_BULK     |
      | status      | PARSED             |
      | created_by  | USR-MAKER-001      |
      | tenant_id   | TUGURE             |
    And sys.upload_batch_row: 350 rows INSERT (row_status = 'PENDING')
    And aud.audit_log.action = BULK.UPLOADED — in-transaction
      With after_jsonb: { batch_id, total_rows: 350, file_name: "instrumen_bulk_jun2026.xlsx", sheets: {...} }

  Scenario: S1-AC2 — File terlalu besar (> 50MB): BULK_FILE_TOO_LARGE sebelum parse
    Given file instrumen_large.xlsx, ukuran 62MB
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload
      With Idempotency-Key: IK-UPLOAD-002
      With multipart file: instrumen_large.xlsx
    Then HTTP 413:
      | error.code    | BULK_FILE_TOO_LARGE                                     |
      | error.message | "Ukuran file 62MB melebihi batas 50MB. Upload dibatalkan." |
    And sys.upload_batch: tidak ada INSERT
    And pengecekan ukuran dilakukan server-side sebelum parsing (tidak mengandalkan client)

  Scenario: S1-AC3 — MIME bukan XLSX: BULK_MIME_INVALID sebelum parse
    Given file instrumen_data.csv (MIME text/csv) di-upload sebagai XLSX
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload
      With Idempotency-Key: IK-UPLOAD-003
      With multipart file: instrumen_data.csv
    Then HTTP 415:
      | error.code    | BULK_MIME_INVALID                                                                       |
      | error.message | "Tipe file tidak valid. Hanya XLSX (application/vnd.openxmlformats-officedocument.spreadsheetml.sheet) yang diterima." |
    And sys.upload_batch: tidak ada INSERT

  Scenario: S1-AC4 — Parse errors dikumpulkan; batch tetap PARSED untuk DRY_RUN (bukan gagal total)
    Given file instrumen_partial_err.xlsx: sheet Obligasi baris 45 — kolom kupon berisi teks "N/A" bukan numeric
    When USR-MAKER-001 meng-upload file tersebut
    Then HTTP 202:
      | data.status         | PARSED                              |
      | data.parse_errors   | [{ sheet: "Obligasi", row: 45, col: "kupon", error: "Expected NUMERIC, got TEXT 'N/A'" }] |
      | data.total_rows     | 349 (valid parse) + 1 error         |
    And sys.upload_batch_row untuk Obligasi baris 45: row_status = 'FAILED', error_detail = "cell type mismatch: kupon"
    And batch dapat dilanjutkan ke DRY_RUN — invalid rows akan tampil di stage_summary sebagai Stage 1 failures
    And aud.audit_log.action = BULK.UPLOADED with after_jsonb.parse_error_count = 1
```

---

## Story P5-M11-S2 — 4-Stage Validation Pipeline (DRY_RUN)

**Actor**: ROLE-MAKER-TR (trigger DRY_RUN), ROLE-RISK (review flagged klasifikasi dari Stage 4)
**Trigger**: `POST /api/v1/master/instrumen/bulk-upload/{batch_id}/dry-run`. Jalankan 4 tahap validasi tanpa INSERT ke `mst.instrumen`. Stage 4 memanggil Phase 3 SPPI+BM auto-eval service per row; jika ambiguous → `row_status = 'FLAGGED_MANUAL_REVIEW'`, dilaporkan di `errors_per_row`. Cache result DRY_RUN di `sys.upload_batch.dry_run_result_jsonb` dengan TTL 1 jam (`dry_run_expires_at = now() + 1h`). Jika batch sudah dalam status `DRY_RUN_PASSED` tapi TTL expired → `BULK_DRY_RUN_EXPIRED` saat COMMIT dicoba. Jika ada rows Stage 1–3 FAILED → status = `DRY_RUN_FAILED`; tidak ada COMMIT yang bisa dijalankan. Audit `BULK.VALIDATED_DRY_RUN` in-transaction.
**Goal**: Maker tahu persis baris mana yang bermasalah sebelum commit. SPPI+BM result preview tersedia untuk ROLE-RISK review. Tidak ada mutasi `mst.instrumen`.

### Pre-conditions
1. `sys.upload_batch.status = 'PARSED'` untuk batch_id ini
2. User ROLE-MAKER-TR adalah `created_by` batch (SoD — orang lain tidak bisa DRY_RUN batch orang lain)
3. `mst.periode_buku.status_periode = 'OPEN'`
4. Phase 3 SPPI+BM auto-eval service tersedia (jika unavailable → DRY_RUN Stage 4 partial skip, flag `SPPI_SERVICE_UNAVAILABLE` di stage_summary)

### Acceptance Criteria

```gherkin
Feature: DRY_RUN 4-stage validation pipeline bulk instrumen

  Background:
    Given sys.upload_batch BATCH-001: status = 'PARSED', 350 rows
    And ROLE-MAKER-TR USR-MAKER-001 adalah created_by BATCH-001

  Scenario: S2-AC1 — DRY_RUN sukses: preview semua stage, klasifikasi SPPI+BM tampil
    Given semua 350 rows lulus Stage 1–3; Stage 4 SPPI+BM auto-eval hasilkan klasifikasi
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/dry-run
      With Idempotency-Key: IK-DRY-001
    Then HTTP 200:
      | data.status          | DRY_RUN_PASSED                      |
      | data.total_rows      | 350                                 |
      | data.valid_rows      | 347                                 |
      | data.invalid_rows    | 0                                   |
      | data.flagged_rows    | 3 (SPPI+BM ambiguous → NEEDS_MANUAL_REVIEW) |
      | data.stage_summary   | { stage1: PASS, stage2: PASS, stage3: PASS, stage4: { evaluated: 350, classified: 347, flagged: 3 } } |
      | data.errors_per_row  | [3 rows dengan klasifikasi_psak71: null, reason: "SPPI Q7 ambiguous"] |
      | data.dry_run_expires_at | 2026-06-21T11:30:00+07:00 (now + 1h) |
    And sys.upload_batch: status = 'DRY_RUN_PASSED', dry_run_result_jsonb ter-update
    And sys.upload_batch_row: 3 rows dengan row_status = 'FLAGGED_MANUAL_REVIEW'
    And aud.audit_log.action = BULK.VALIDATED_DRY_RUN — in-transaction
      With after_jsonb: { batch_id, valid_rows: 347, invalid_rows: 0, flagged_rows: 3, stage_summary }

  Scenario: S2-AC2 — DRY_RUN FAILED: Stage 3 cross-ref error (counterparty tidak ada)
    Given sys.upload_batch_row Obligasi baris 10: counterparty_id = 'CP-999' tidak ada di mst.counterparty
    When USR-MAKER-001 trigger DRY_RUN untuk BATCH-001
    Then HTTP 200 (DRY_RUN selalu return 200 — gagal adalah isi data):
      | data.status          | DRY_RUN_FAILED                      |
      | data.invalid_rows    | 1                                   |
      | data.errors_per_row  | [{ sheet: "Obligasi", row: 10, stage: 3, col: "counterparty_id", error: "Counterparty CP-999 tidak ditemukan di master data." }] |
      | data.stage_summary   | { stage3: FAIL: { error_count: 1 } } |
    And sys.upload_batch: status = 'DRY_RUN_FAILED'
    And COMMIT tidak bisa dijalankan saat status = 'DRY_RUN_FAILED' (S3 guard: BULK_DRY_RUN_FAILED)
    And aud.audit_log.action = BULK.VALIDATED_DRY_RUN with after_jsonb.status = 'DRY_RUN_FAILED'

  Scenario: S2-AC3 — Stage 4 SPPI+BM ambiguous row: NEEDS_MANUAL_REVIEW flag, commit tetap lanjut row lain
    Given Deposito baris 22: SPPI Q7 (prepayment clause) = 'Y' dengan nilai tidak wajar — Phase 3 return ambiguous
    When Stage 4 memproses baris 22
    Then sys.upload_batch_row baris 22: row_status = 'FLAGGED_MANUAL_REVIEW'
      | klassifikasi_psak71    | null (belum ditentukan)              |
      | flag_reason            | "SPPI Q7 ambiguous — perlu review manual ROLE-RISK sebelum ACTIVE" |
    And DRY_RUN status = 'DRY_RUN_PASSED' (flagged bukan invalid — COMMIT tetap diizinkan)
    And notifikasi ke ROLE-RISK: "BATCH-001 DRY_RUN: 1 row memerlukan review klasifikasi PSAK 71 manual."
    And COMMIT untuk baris 22 akan INSERT mst.instrumen dengan status = 'PENDING_CLASSIFICATION'

  Scenario: S2-AC4 — DRY_RUN TTL 1 jam: re-run jika expired; BULK_DRY_RUN_EXPIRED saat coba COMMIT
    Given sys.upload_batch BATCH-001: status = 'DRY_RUN_PASSED', dry_run_expires_at = 2026-06-21T09:00:00+07:00
    And waktu sekarang = 2026-06-21T10:05:00+07:00 (TTL expired)
    When USR-MAKER-001 mencoba POST /api/v1/master/instrumen/bulk-upload/BATCH-001/commit
      With Idempotency-Key: IK-COMMIT-001
    Then HTTP 422:
      | error.code    | BULK_DRY_RUN_EXPIRED                                              |
      | error.message | "DRY_RUN BATCH-001 expired pukul 09:00. Jalankan ulang DRY_RUN sebelum COMMIT." |
    And tidak ada Asynq job yang di-enqueue
    And toast ke USR-MAKER-001: "DRY_RUN expired — silakan jalankan ulang DRY_RUN untuk BATCH-001."
```

---

## Story P5-M11-S3 — Async Commit Job + Partial Commit

**Actor**: ROLE-MAKER-TR (trigger COMMIT), ROLE-IT-ADMIN (monitor sys.job)
**Trigger**: `POST /api/v1/master/instrumen/bulk-upload/{batch_id}/commit`. Hanya boleh jika `batch.status = 'DRY_RUN_PASSED'` dan TTL belum expired dan `mst.periode_buku.status_periode = 'OPEN'`. Enqueue Asynq job `bulkupload:commit_instrumen`. Return `202 { jobId, statusUrl, streamUrl }`. Worker proses per row: INSERT `mst.instrumen` (klasifikasi dari SPPI+BM result S2); partial commit OK — failed rows dilog ke `sys.upload_batch_row.row_status = 'FAILED'`, committed rows = 'COMMITTED'. Setelah job selesai: batch status = 'COMMITTED' (full) atau 'PARTIAL_COMMIT'. `sys.job` tracking progress 0–100% via Redis + DB. Audit `BULK.COMMITTED` atau `BULK.PARTIAL_COMMIT` in-transaction setelah job done.
**Goal**: ROLE-MAKER-TR tidak block menunggu. Per-row partial success. Failed rows tidak menggagalkan seluruh batch. Progress visible via SSE.

### Pre-conditions
1. `sys.upload_batch.status = 'DRY_RUN_PASSED'` dan TTL belum expired
2. ROLE-MAKER-TR adalah `created_by` batch (SoD)
3. `mst.periode_buku.status_periode = 'OPEN'`
4. Idempotency-Key belum pernah dipakai untuk COMMIT batch ini

### Acceptance Criteria

```gherkin
Feature: Async commit bulk instrumen via Asynq job dengan partial commit

  Background:
    Given sys.upload_batch BATCH-001: status = 'DRY_RUN_PASSED', 350 rows, TTL valid
    And mst.periode_buku PRD-2026-06: status_periode = 'OPEN'
    And ROLE-MAKER-TR USR-MAKER-001 adalah created_by BATCH-001

  Scenario: S3-AC1 — Commit enqueue 202; JobProgressPanel track progress 0→100%
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/commit
      With Idempotency-Key: IK-COMMIT-001
    Then HTTP 202:
      | data.jobId     | JOB-BULK-001                                         |
      | data.statusUrl | /api/v1/jobs/JOB-BULK-001                            |
      | data.streamUrl | /api/v1/jobs/JOB-BULK-001/stream                     |
    And sys.job INSERT:
      | id         | JOB-BULK-001              |
      | type       | bulkupload:commit_instrumen |
      | status     | queued                    |
      | can_cancel | false                     |
      | created_by | USR-MAKER-001             |
    And JobProgressPanel frontend subscribe SSE stream
    And saat worker proses baris 175 dari 350: GET /api/v1/jobs/JOB-BULK-001 return:
      | status    | running                                               |
      | progress  | 50                                                    |
      | currentStep | "Memproses instrumen 175 dari 350 (Obligasi sheet)" |
    And saat selesai: toast "BATCH-001 commit selesai. 348 instrumen berhasil, 2 gagal. Lihat detail."

  Scenario: S3-AC2 — Partial commit: rows gagal di-skip, rows berhasil committed
    Given sys.upload_batch_row BATCH-001: 348 rows valid, 2 rows dengan duplikat kode (CONFLICT DB constraint)
    When worker memproses semua 350 rows
    Then 348 rows: mst.instrumen INSERT sukses, row_status = 'COMMITTED'
    And 2 rows duplikat: tidak di-INSERT, row_status = 'FAILED'
      | error_detail | "Duplikat kode instrumen 'INST-DEP-0042' — sudah ada di mst.instrumen" |
    And sys.upload_batch: status = 'PARTIAL_COMMIT'
      | committed_rows | 348 |
      | failed_rows    | 2   |
    And sys.job: status = 'completed', progress = 100
    And aud.audit_log.action = BULK.PARTIAL_COMMIT — in-transaction
      With after_jsonb: { batch_id, committed_rows: 348, failed_rows: 2, failed_row_ids: [...] }

  Scenario: S3-AC3 — BULK_PERIODE_LOCKED: periode CLOSED saat commit dicoba
    Given mst.periode_buku PRD-2026-06: status_periode = 'CLOSED' (di-close setelah DRY_RUN tapi sebelum COMMIT)
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/commit
      With Idempotency-Key: IK-COMMIT-002
    Then HTTP 423:
      | error.code    | BULK_PERIODE_LOCKED                                       |
      | error.message | "Periode PRD-2026-06 sudah CLOSED. Bulk commit tidak dapat diproses." |
    And tidak ada Asynq job yang di-enqueue
    And sys.upload_batch: status tetap 'DRY_RUN_PASSED' (tidak berubah)

  Scenario: S3-AC4 — Instrumen dari batch commit tersimpan dengan status PENDING_APPROVAL_BULK sampai S4 approve
    Given COMMIT BATCH-001 selesai: 348 instrumen berhasil INSERT
    Then setiap mst.instrumen dari batch ini: status = 'PENDING_APPROVAL_BULK'
      | batch_id       | BATCH-001         |
      | created_by     | USR-MAKER-001     |
      | klasifikasi_psak71 | dari SPPI+BM auto-eval S2 (atau null jika FLAGGED_MANUAL_REVIEW) |
    And instrumen belum ACTIVE — tidak tersedia di laporan sampai ROLE-APPR-TR approve (S4)
    And aud.audit_log.action = BULK.COMMITTED (jika full) atau BULK.PARTIAL_COMMIT — per batch (bukan per row)
```

---

## Story P5-M11-S4 — Approval Workflow 4-Eyes

**Actor**: ROLE-APPR-TR (approve), ROLE-MAKER-TR (maker, SoD subject), ROLE-RISK (review flagged), ROLE-AUDIT (read)
**Trigger**: `POST /api/v1/master/instrumen/bulk-upload/{batch_id}/approve`. Hanya boleh jika `batch.status IN ('COMMITTED', 'PARTIAL_COMMIT')`. SoD enforce: `approver_id ≠ maker_id` — jika sama → `BULK_APPROVE_SOD_VIOLATION`. Idempotency-Key wajib. Setelah approve: semua instrumen dari batch dengan `row_status = 'COMMITTED'` → `mst.instrumen.status = 'ACTIVE'`. Instrumen `FLAGGED_MANUAL_REVIEW` tetap `PENDING_CLASSIFICATION` sampai ROLE-RISK resolve. Instrumen `FAILED` tetap soft-skip. Audit `BULK.APPROVED` in-transaction.
**Goal**: 4-eyes terpenuhi sebelum instrumen menjadi ACTIVE. SoD tidak bisa di-bypass via API langsung. Flagged rows tidak auto-ACTIVE.

### Pre-conditions
1. `sys.upload_batch.status IN ('COMMITTED', 'PARTIAL_COMMIT')`
2. Approver adalah ROLE-APPR-TR dengan permission `instrumen.approve`
3. `approver_id ≠ batch.created_by` (maker) — SoD
4. Idempotency-Key belum pernah dipakai untuk approve batch ini

### Acceptance Criteria

```gherkin
Feature: Approval 4-eyes bulk batch instrumen oleh ROLE-APPR-TR

  Background:
    Given sys.upload_batch BATCH-001: status = 'COMMITTED', maker = USR-MAKER-001
    And 348 mst.instrumen rows dari BATCH-001: status = 'PENDING_APPROVAL_BULK'
    And 3 rows: status = 'PENDING_CLASSIFICATION' (FLAGGED_MANUAL_REVIEW dari S2)

  Scenario: S4-AC1 — Approve sukses: instrumen COMMITTED → ACTIVE; flagged tetap PENDING_CLASSIFICATION
    Given ROLE-APPR-TR USR-APPR-001 (berbeda dari USR-MAKER-001)
    When USR-APPR-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/approve
      With Idempotency-Key: IK-APPR-001
      With body: { comment: "Checked — 348 instrumen sesuai daftar penempatan Juni 2026", signature_method: "JWT_STEP_UP" }
    Then HTTP 200:
      | data.batch_id         | BATCH-001                  |
      | data.status           | APPROVED                   |
      | data.activated_count  | 348                        |
      | data.pending_manual   | 3                          |
    And 348 mst.instrumen: status = 'ACTIVE'
    And 3 mst.instrumen PENDING_CLASSIFICATION: status tidak berubah — menunggu ROLE-RISK
    And sys.upload_batch: status = 'APPROVED', approver_id = USR-APPR-001, approved_at = now()
    And aud.audit_log.action = BULK.APPROVED — in-transaction
      With after_jsonb: { batch_id, activated_count: 348, approver_id: USR-APPR-001, comment }

  Scenario: S4-AC2 — SoD violation: maker mencoba approve batch sendiri
    Given USR-MAKER-001 mencoba approve BATCH-001 yang dia buat sendiri
    When USR-MAKER-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/approve
      With Idempotency-Key: IK-APPR-SOD-001
    Then HTTP 403:
      | error.code    | BULK_APPROVE_SOD_VIOLATION                                              |
      | error.message | "SoD: Maker tidak dapat menjadi approver untuk batch yang sama (DEC-017)." |
    And mst.instrumen: status tetap 'PENDING_APPROVAL_BULK' (tidak ada perubahan)
    And aud.audit_log.action = BULK.SOD_VIOLATION_ATTEMPT — in-transaction dengan detail approver_id = maker_id

  Scenario: S4-AC3 — Idempotency replay: approve dengan key yang sama → return response original
    Given USR-APPR-001 sudah berhasil approve BATCH-001 dengan IK-APPR-001
    When USR-APPR-001 POST approve kembali dengan Idempotency-Key: IK-APPR-001
    Then HTTP 200 (idempotency replay):
      | error.code (dalam meta) | IDEMPOTENCY_REPLAY (bukan error — per api-conventions.md) |
      | data                    | response original approve pertama                         |
    And tidak ada perubahan tambahan ke mst.instrumen atau aud.audit_log

  Scenario: S4-AC4 — ROLE-RISK review flagged klasifikasi setelah approve; resolve → ACTIVE
    Given 3 instrumen status = 'PENDING_CLASSIFICATION' setelah S4 approve
    When ROLE-RISK (USR-RISK-001) mengirim PATCH /api/v1/master/instrumen/{id}/klasifikasi-manual
      With body: { klasifikasi_psak71: "FVTPL", sppi_result: "FAIL", bm_result: "HTC", reason: "Prepayment clause material" }
    Then mst.instrumen {id}: klasifikasi_psak71 = 'FVTPL', status = 'ACTIVE'
    And aud.audit_log.action = INSTRUMEN.KLASIFIKASI_MANUAL_RESOLVED — in-transaction
    And notifikasi ke ROLE-APPR-TR: "Instrumen {id} klasifikasi manual diselesaikan ROLE-RISK → FVTPL. Aktif."
```

---

## Story P5-M11-S5 — Rollback oleh CFO dalam Grace Window

**Actor**: ROLE-CFO (rollback), ROLE-IT-ADMIN (config grace window + size limit), ROLE-AUDIT (read)
**Trigger**: `POST /api/v1/master/instrumen/bulk-upload/{batch_id}/rollback`. Hanya boleh jika `batch.status = 'APPROVED'` DAN `now() ≤ batch.commit_at + grace_window`. Grace window default 7 hari; dikonfigurasi via `sys.config_param.BULK_ROLLBACK_GRACE_DAYS` oleh ROLE-IT-ADMIN. Jika expired → `BULK_ROLLBACK_GRACE_EXPIRED`. Step-up MFA wajib (DEC-027 — irreversible high-impact). Rollback: soft-delete (`deleted_at`, `deleted_by`) semua `mst.instrumen` dari batch (row_status = 'ROLLED_BACK'); jurnal kompensasi jika ada posting terkait. SoD: rollback tidak memerlukan SoD terhadap maker/approver karena ini CFO authority. Audit `BULK.ROLLBACK_REQUESTED` + `BULK.ROLLBACK_APPROVED` in-transaction.
**Goal**: CFO dapat batalkan batch erroneous post-commit dalam grace window. Soft-delete only (DEC-018). Tidak ada hard-delete. Audit immutable.

### Pre-conditions
1. `sys.upload_batch.status = 'APPROVED'` untuk batch_id
2. `now() ≤ batch.commit_at + BULK_ROLLBACK_GRACE_DAYS` (default 7 hari)
3. User ROLE-CFO dengan permission `instrumen.delete` + MFA step-up `mfa_verified = true` + step-up token valid
4. Idempotency-Key belum pernah dipakai untuk rollback batch ini

### Acceptance Criteria

```gherkin
Feature: CFO rollback bulk batch instrumen dalam grace window

  Background:
    Given sys.upload_batch BATCH-001: status = 'APPROVED', commit_at = 2026-06-16T10:00:00+07:00
    And sys.config_param.BULK_ROLLBACK_GRACE_DAYS = 7
    And grace window expires: 2026-06-23T10:00:00+07:00
    And waktu sekarang = 2026-06-21T14:00:00+07:00 (dalam grace window)
    And ROLE-CFO USR-CFO-001 sudah step-up MFA (mfa_verified = true, step_up_token valid)

  Scenario: S5-AC1 — Rollback dalam grace window: soft-delete semua instrumen batch + audit
    When USR-CFO-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/rollback
      With Idempotency-Key: IK-ROLLBACK-001
      With body: { reason: "Error counterparty mapping ditemukan post-commit", mfa_token: "...", signature_method: "JWT_STEP_UP" }
    Then HTTP 200:
      | data.batch_id           | BATCH-001                         |
      | data.status             | ROLLED_BACK                       |
      | data.rolled_back_count  | 348                               |
    And 348 mst.instrumen dari BATCH-001:
      | deleted_at  | now()              |
      | deleted_by  | USR-CFO-001        |
      | row_status (upload_batch_row) | ROLLED_BACK |
    And sys.upload_batch: status = 'ROLLED_BACK'
    And aud.audit_log INSERT dua event in-transaction:
      1. action = BULK.ROLLBACK_REQUESTED: { batch_id, reason, actor: USR-CFO-001, mfa_method }
      2. action = BULK.ROLLBACK_APPROVED: { batch_id, rolled_back_count: 348, commit_at: 2026-06-16T10:00, rollback_at: now() }
    And toast ke USR-CFO-001: "BATCH-001 berhasil di-rollback. 348 instrumen soft-deleted. Audit log dicatat."

  Scenario: S5-AC2 — Grace window expired: BULK_ROLLBACK_GRACE_EXPIRED
    Given waktu sekarang = 2026-06-24T10:00:00+07:00 (lebih dari 7 hari sejak commit_at)
    When USR-CFO-001 POST /api/v1/master/instrumen/bulk-upload/BATCH-001/rollback
      With Idempotency-Key: IK-ROLLBACK-002
    Then HTTP 422:
      | error.code    | BULK_ROLLBACK_GRACE_EXPIRED                                                                     |
      | error.message | "Grace window 7 hari telah berakhir (commit_at + 7d = 2026-06-23T10:00). Rollback tidak dapat dilakukan." |
    And mst.instrumen: tidak ada perubahan (deleted_at tetap null)
    And notifikasi ke USR-CFO-001: "Grace window expired. Eskalasi ke ROLE-IT-ADMIN untuk penanganan lanjutan."

  Scenario: S5-AC3 — Step-up MFA wajib; tanpa step-up token → 403
    Given USR-CFO-001 tidak melakukan step-up MFA (step_up_token absent atau expired > 5 menit)
    When USR-CFO-001 POST rollback BATCH-001 tanpa X-Step-Up-Token header
    Then HTTP 403:
      | error.code    | FORBIDDEN                                                              |
      | error.message | "Rollback memerlukan step-up MFA. Lakukan re-autentikasi MFA terlebih dahulu (DEC-027)." |
    And tidak ada perubahan ke mst.instrumen
    And tidak ada aud.audit_log ROLLBACK event

  Scenario: S5-AC4 — ROLE-IT-ADMIN update grace window via sys.config_param; efektif untuk batch berikutnya
    Given ROLE-IT-ADMIN USR-IT-001 mengubah grace window menjadi 14 hari
    When USR-IT-001 PATCH /api/v1/sys/config-param/BULK_ROLLBACK_GRACE_DAYS
      With Idempotency-Key: IK-CONFIG-001
      With body: { value: "14", reason: "Audit cycle butuh 2 minggu window" }
    Then HTTP 200: sys.config_param.BULK_ROLLBACK_GRACE_DAYS = '14'
    And aud.audit_log.action = SYS.CONFIG_PARAM_UPDATED — in-transaction
      With after_jsonb: { param: "BULK_ROLLBACK_GRACE_DAYS", old_value: "7", new_value: "14", actor: USR-IT-001 }
    And batch yang sudah APPROVED sebelum perubahan ini: grace window dihitung dari nilai lama (tidak retroaktif)
    And batch baru yang di-commit setelah perubahan ini: menggunakan nilai 14 hari
```

---

## Ringkasan P5-M11 Story Set

| Story | Judul | Actor Utama | AC Count | Gate |
|---|---|---|---|---|
| P5-M11-S1 | Upload XLSX 5-sheet + parse | ROLE-MAKER-TR | 4 | **security-engineer BLOCKING** (audit in-tx, MIME server-side validate) |
| P5-M11-S2 | 4-stage validation pipeline (DRY_RUN) | ROLE-MAKER-TR, ROLE-RISK | 4 | advisory (non-blocking; tidak menyentuh ECL/EIR/SPPI compute) |
| P5-M11-S3 | Async commit job + partial commit | ROLE-MAKER-TR, ROLE-IT-ADMIN | 4 | **security-engineer BLOCKING** (idempotency, audit in-tx, sys.job immutability) |
| P5-M11-S4 | Approval workflow 4-eyes | ROLE-APPR-TR, ROLE-RISK | 4 | **security-engineer BLOCKING** (SoD enforce server-side, idempotency replay) |
| P5-M11-S5 | Rollback CFO grace window | ROLE-CFO, ROLE-IT-ADMIN | 4 | **security-engineer BLOCKING** (step-up MFA, soft-delete only DEC-018, audit in-tx) |
| **Total** | | | **20** | |

---

## Error Codes Proposed (Baru — untuk system-analyst)

| Code | HTTP | Trigger | Catatan |
|---|---|---|---|
| `BULK_FILE_TOO_LARGE` | 413 | File size > 50MB saat upload | Server-side check sebelum parse; tidak bergantung Content-Length header client |
| `BULK_MIME_INVALID` | 415 | MIME bukan XLSX zip signature | Magic-byte check server-side, bukan hanya extension |
| `BULK_DRY_RUN_EXPIRED` | 422 | TTL 1 jam `dry_run_expires_at` expired saat COMMIT dicoba | User harus re-run DRY_RUN |
| `BULK_DRY_RUN_FAILED` | 422 | COMMIT dicoba saat status = 'DRY_RUN_FAILED' | Stage 1–3 harus semua PASS dulu |
| `BULK_PERIODE_LOCKED` | 423 | `mst.periode_buku.status_periode = 'CLOSED'` saat upload atau commit | Berlaku saat upload (S1) dan commit (S3) |
| `BULK_ROLLBACK_GRACE_EXPIRED` | 422 | `now() > commit_at + BULK_ROLLBACK_GRACE_DAYS` | Grace window; IT-ADMIN configurable |
| `BULK_APPROVE_SOD_VIOLATION` | 403 | `approver_id = batch.created_by` (maker) | DEC-017; server-side enforce, bukan hanya UI disable |

Catatan: `SOD_VIOLATION` (403), `FORBIDDEN` (403), `IDEMPOTENCY_REPLAY` (200), `NOT_FOUND` (404), `WORKFLOW_INVALID_TRANSITION` (422) sudah ada di api-conventions.md.

---

## Persona Summary Table

| Actor | Permission | Aksi di P5-M11 | MFA Level |
|---|---|---|---|
| ROLE-MAKER-TR | `instrumen.create` | Upload (S1), DRY_RUN (S2), COMMIT (S3) | Tidak wajib |
| ROLE-APPR-TR | `instrumen.approve` | Approve batch 4-eyes (S4); SoD ≠ maker | Wajib jika Treasury Manager (DEC-026) |
| ROLE-RISK | `instrumen.read`, `klasifikasi.update` | Review flagged klasifikasi (S2-AC3, S4-AC4); resolve NEEDS_MANUAL_REVIEW | Tidak wajib |
| ROLE-CFO | `instrumen.delete` | Rollback dalam grace window (S5); step-up MFA wajib (DEC-027) | WAJIB + step-up |
| ROLE-AUDIT | `instrumen.read`, `audit_log.read` | Read-only seluruh batch history + audit log | Tidak wajib |
| ROLE-IT-ADMIN | `sys.config.update` | Update grace window + file size limit di sys.config_param (S5-AC4) | WAJIB (DEC-026) |

---

## Audit Events Summary

| Event | Trigger | In-transaction |
|---|---|---|
| `BULK.UPLOADED` | POST upload selesai parse | Ya (S1) |
| `BULK.VALIDATED_DRY_RUN` | DRY_RUN selesai (PASS atau FAILED) | Ya (S2) |
| `BULK.COMMITTED` | Asynq job commit selesai, semua rows berhasil | Ya, di job commit transaction (S3) |
| `BULK.PARTIAL_COMMIT` | Asynq job commit selesai, ada failed rows | Ya, di job commit transaction (S3) |
| `BULK.APPROVED` | Approver approve batch | Ya (S4) |
| `BULK.SOD_VIOLATION_ATTEMPT` | Maker mencoba approve batch sendiri | Ya (S4) |
| `BULK.ROLLBACK_REQUESTED` | CFO trigger rollback | Ya, event pertama in-transaction (S5) |
| `BULK.ROLLBACK_APPROVED` | Rollback selesai soft-delete | Ya, event kedua in-transaction (S5) |
| `SYS.CONFIG_PARAM_UPDATED` | IT-ADMIN update grace window | Ya (S5-AC4) |

---

## Dependensi Lintas Modul

| Dependensi | Arah | Keterangan |
|---|---|---|
| `mst.instrumen` target tabel | P5-M1 → P5-M11 | M11 INSERT rows ke tabel yang sudah ada dari M1; schema harus sudah punya `batch_id`, `row_status`, `status` enum termasuk `PENDING_APPROVAL_BULK`, `PENDING_CLASSIFICATION` |
| Phase 3 SPPI+BM auto-eval service | Phase 3 → P5-M11-S2 | Stage 4 DRY_RUN memanggil Phase 3 service; jika unavailable → `SPPI_SERVICE_UNAVAILABLE` di stage_summary; batch tetap bisa DRY_RUN_PASSED (treated sebagai flagged) |
| `sys.config_param` | P5-M11-S5 ↔ ROLE-IT-ADMIN | `BULK_ROLLBACK_GRACE_DAYS` dan `BULK_FILE_SIZE_LIMIT_MB` harus ada di sys.config_param seed |
| `sys.job` tracking | DEC-007 → P5-M11-S3 | Asynq worker update sys.job.progress + Redis pub/sub; schema `sys.job` sudah ada dari M10 pattern |
| `mst.periode_buku` | APP-D → P5-M11-S1,S3 | Periode lock cek di upload (S1) dan commit (S3); `status_periode = 'CLOSED'` → `BULK_PERIODE_LOCKED` |

---

## Handoff Berikutnya

- `system-analyst` → OpenAPI: 7 endpoints (`POST /master/instrumen/bulk-upload`, `POST /batch/{id}/dry-run`, `POST /batch/{id}/commit`, `POST /batch/{id}/approve`, `POST /batch/{id}/rollback`, `GET /batch/{id}/status`, `PATCH /sys/config-param/{key}`); state machine `sys.upload_batch.status` (PARSED → DRY_RUN_PASSED/FAILED → COMMITTED/PARTIAL_COMMIT → APPROVED → ROLLED_BACK); 7 error codes baru; SSE stream `/jobs/{jobId}/stream`
- `data-modeler` → migration: `sys.upload_batch` (batch_id, batch_type, status, dry_run_result_jsonb, dry_run_expires_at, commit_at, approver_id, approved_at, created_by, tenant_id + audit cols); `sys.upload_batch_row` (batch_id FK, sheet, row_number, row_data_jsonb, row_status, klasifikasi_psak71, flag_reason, error_detail + audit cols); index `(batch_id, row_status)`; partial index `WHERE deleted_at IS NULL`; tambah kolom `batch_id`, `PENDING_APPROVAL_BULK`, `PENDING_CLASSIFICATION` ke `mst.instrumen.status` enum jika belum ada
- `security-engineer` → **BLOCKING**: SoD enforce server-side (S4 approver ≠ maker); step-up MFA rollback (S5); audit semua 9 events in-transaction; soft-delete only DEC-018; idempotency semua 5 mutating endpoints; MIME magic-byte validation (bukan hanya Content-Type header); file size server-side (tidak percaya Content-Length)
- `ifrs9-compliance-reviewer` → **advisory, non-blocking**: P5-M11 tidak menyentuh ECL/EIR/SPPI/BM computation — hanya consume Phase 3 auto-eval result. Tidak diperlukan BLOCKING gate. Jika reviewer ingin verifikasi bahwa klasifikasi auto-eval dari Phase 3 di-store dengan benar di `mst.instrumen.klasifikasi_psak71`, ini bisa dilakukan in-parallel tanpa menghentikan implementasi.

_Story set ini siap dihandoff ke `system-analyst` (OpenAPI + state machine) dan `security-engineer` (BLOCKING). `data-modeler` dapat mulai migration `sys.upload_batch` + `sys.upload_batch_row` paralel setelah system-analyst selesai schema contract. `ifrs9-compliance-reviewer` advisory (non-blocking) — dipanggil sesuai kebutuhan, tidak di jalur kritis P5-M11._
