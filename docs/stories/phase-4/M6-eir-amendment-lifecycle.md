# P4-M6 — EIR Amendment Re-estimation Lifecycle: User Stories

**Story Set ID**: P4-M6
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 2)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-12
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.4–4.5 (amendment re-estimation, bulk operations)
**Linked BRD**: BRD §8.4 (EIR Requirements), RACI: ROLE-AKUN (R/propose), ROLE-RISK (R/review + trigger), ROLE-ALCO (A/approve, MFA), ROLE-AKUN-CTL (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-013 — EIR: Newton-Raphson tolerance `1e-10`, max 100 iter, presisi 8 desimal
- DEC-016 — NUMERIC precision. No float64.
- DEC-017 — 4-eyes workflow. SoD `maker_id ≠ reviewer_id ≠ approver_id`.
- DEC-018 — Audit trail append-only. `ecl.*` no hard delete. Schedule rows immutable setelah insert.
- DEC-027 — Step-up MFA untuk approve amendment EIR.

**Depends on**:
- P4-M5 MERGED (PR #60) — `backend/internal/ecl/eir/amendment_service.go` (Submit/Review/Approve/Reject endpoint sudah ada), `ecl.eir_amortization_schedule`, `ecl.eir_reestimation_log`, `BulkService`, drift threshold `0.0001` (1 bp, dikonfirmasi dari `bulk_service.go:75`)
- `doc.document` — tabel dokumen sudah ada (Phase 3)
- `sys.job` — tabel job sudah ada (P4-M8 atau Phase 2 infra)

**M6 scope** (yang belum ada di M5):
1. Auto-detect kontrak amandemen via document upload event
2. Scheduled periodic drift detection (daily cron)
3. Ad-hoc bulk re-estimation dengan drift report + amendment proposal auto-creation
4. Review queue UI untuk pending amendments
5. Cancel / withdraw amendment proposal pre-approval

**Dependency pada M7**: Catch-up adjustment (`catch_up_adjustment` di `ecl.eir_reestimation_log`) dihitung saat Approve (NPV difference) — sudah ada di M5 `Approve()`. **M7 scope** yang didefer adalah booking catch-up ke ECL engine (jurnal entry), bukan kalkulasinya. Stories M6 mencakup perhitungan dan penyimpanan nilai `catch_up_adjustment`; booking ke ledger = M7/Phase 5.

**Handoff berikutnya**:
- `system-analyst` — OpenAPI fragment: endpoint baru (document-trigger, drift-report, cancel, amendment queue)
- `data-modeler` — migration `000027`: tabel `sys.drift_report` baru + kolom `cancelled_at`, `cancel_reason` di `ecl.eir_reestimation_log` + notifikasi hook
- `ecl-eir-engineer` — detection worker + drift report service + proposal auto-creation logic
- `ifrs9-compliance-reviewer` — BLOCKING gate sebelum merge

---

## Konteks M5 yang Menjadi Titik Awal M6

P4-M5 sudah mengimplementasi:
- `AmendmentService.Propose()` — ROLE-AKUN manual submit proposal dengan cashflow revisi
- `AmendmentService.Review()` — ROLE-RISK sign-off (PENDING_REVIEW → PENDING_APPROVAL)
- `AmendmentService.Approve()` — ROLE-ALCO step-up MFA, execute Newton-Raphson + insert schedule version baru (atomic)
- `AmendmentService.Reject()` — reviewer atau approver reject
- `BulkService.Recompute()` — report-only, tidak auto-create amendment, drift threshold = `0.0001` (1 bp)

**Yang belum ada di M5** (M6 mengisi gap ini):
- Trigger otomatis dari upload dokumen kontrak amandemen
- Cron harian untuk scan drift dan auto-create proposal bila drift > threshold tinggi
- `sys.drift_report` tabel untuk menyimpan hasil scan harian
- Cancel/withdraw proposal oleh maker sebelum reviewer sign
- Notification dispatch ke ROLE-RISK saat proposal baru terbentuk
- UI review queue DataTable untuk pending amendments

---

## Schema Baru yang Dibutuhkan M6

### `sys.drift_report` (tabel baru, migration 000027)

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | |
| `run_at` | TIMESTAMPTZ | Waktu scan berjalan |
| `trigger_type` | TEXT | `CRON_DAILY` / `AD_HOC` / `PRE_ECL_CALC` |
| `total_scanned` | INT | Jumlah instrumen di-scan |
| `drift_count` | INT | Jumlah instrumen > drift threshold |
| `missing_count` | INT | Instrumen tanpa schedule aktif |
| `proposal_auto_created` | INT | Jumlah proposal yang di-auto-create |
| `result_jsonb` | JSONB | Detail per instrumen (drift entries, missing) |
| `job_id` | TEXT | FK ke `sys.job.id` (jika async) |
| + audit cols standar | | `created_at`, `created_by`, `updated_at`, dst |

### Kolom tambahan di `ecl.eir_reestimation_log` (migration 000027)

| Kolom | Tipe | Keterangan |
|---|---|---|
| `cancelled_at` | TIMESTAMPTZ | Di-set saat maker cancel |
| `cancel_reason` | TEXT | Wajib ≥ 20 karakter |
| `trigger_source` | TEXT | `MANUAL` / `DOCUMENT_UPLOAD` / `CRON_DRIFT` / `AD_HOC_BULK` |
| `drift_report_id` | UUID | FK → `sys.drift_report.id` (nullable, untuk auto-created proposals) |
| `document_id` | UUID | FK → `doc.document.id` (nullable, link ke kontrak amandemen) |

---

## Permissions Baru (M6)

| Permission | Holders | Deskripsi |
|---|---|---|
| `eir.amendment.detect` | System (cron/worker), ROLE-RISK (manual trigger) | Trigger detection scan + auto-create proposal |
| `eir.amendment.cancel` | ROLE-AKUN (maker only) | Cancel proposal DRAFT/PENDING_REVIEW milik sendiri |
| `eir.amendment_review.read` | ROLE-RISK, ROLE-ALCO, ROLE-AKUN (milik sendiri), ROLE-AUDIT | Baca queue pending amendments |
| `eir.drift_report.read` | ROLE-RISK, ROLE-AUDIT | Baca laporan drift harian |

---

## Story APP-C-M6-001 — Detect Kontrak Amandemen via Document Upload

**Actor**: ROLE-AKUN (upload dokumen kontrak amandemen)
**Trigger**: `doc.document` row baru di-insert dengan `document_type IN ('AMENDMENT_KONTRAK_DEPOSITO', 'AMENDMENT_KONTRAK_OBLIGASI')` dan `entity_type = 'mst.instrumen'` dan `status = 'APPROVED'`
**Goal**: System secara otomatis membuat draft `ecl.eir_reestimation_log` (proposal amandemen) yang ter-linked ke dokumen tersebut dan mengirim notifikasi ke ROLE-RISK reviewer.

**Pre-conditions**:
- Instrumen target berstatus `AKTIF` dan `eir_method_flag = TRUE` dan `klasifikasi_psak71 IN ('AC', 'FVOCI')`.
- `mst.instrumen.eir_awal` tidak NULL (EIR sudah pernah dihitung).
- Tidak ada proposal amandemen non-terminal yang sudah aktif untuk instrumen yang sama (`ecl.eir_reestimation_log` tanpa status `APPROVED`/`REJECTED`/`CANCELLED`).

**Post-conditions**:
- Baris baru di `ecl.eir_reestimation_log` dengan `workflow_status = 'PENDING_REVIEW'` dan `trigger_source = 'DOCUMENT_UPLOAD'` dan `document_id = {doc.document.id}`.
- Notifikasi dikirim ke semua pengguna dengan role ROLE-RISK (in-app + email, sesuai resolusi OQ-M6-1).
- Audit event `EIR.AMEND_AUTO_CREATED` ditulis ke `aud.audit_log` in-transaction.

**Audit Events**: `EIR.AMEND_AUTO_CREATED`
**Permission**: Event dipicu oleh system worker (tidak membutuhkan permission user); user uploader butuh `doc.document.create`.

### Acceptance Criteria

```gherkin
Feature: Auto-create EIR amendment proposal from document upload

  Background:
    Given instrumen "OBL-2026-00001" dengan:
      | klasifikasi_psak71 | AC       |
      | eir_method_flag    | true     |
      | eir_awal           | 0.04200000 |
      | status             | AKTIF    |
    And user AKUN-01 memiliki role ROLE-AKUN dan permission doc.document.create
    And tidak ada proposal amandemen aktif untuk "OBL-2026-00001"

  Scenario: Happy path — dokumen amandemen uploaded, proposal terbentuk otomatis
    Given AKUN-01 upload dokumen "amandemen-obl-001.pdf" dengan:
      | document_type | AMENDMENT_KONTRAK_OBLIGASI |
      | entity_type   | mst.instrumen              |
      | entity_id     | OBL-2026-00001             |
    When dokumen di-approve ke status "APPROVED"
    Then sistem membuat baris baru di ecl.eir_reestimation_log dengan:
      | instrumen_id    | OBL-2026-00001           |
      | workflow_status | PENDING_REVIEW           |
      | trigger_source  | DOCUMENT_UPLOAD          |
      | document_id     | {id dokumen yang di-upload}|
      | eir_lama        | 0.04200000               |
    And notifikasi dikirim ke semua user ROLE-RISK:
      "Proposal amandemen EIR otomatis dibuat untuk OBL-2026-00001. Harap review sebelum [deadline]."
    And aud.audit_log berisi event "EIR.AMEND_AUTO_CREATED" dengan:
      | entity_type  | ecl.eir_reestimation_log |
      | entity_id    | {proposal.id}            |
      | actor_role   | SYSTEM                   |
      | after_jsonb.trigger_source | DOCUMENT_UPLOAD |

  Scenario: Instrumen FVTPL — dokumen amandemen uploaded, tidak ada proposal dibuat
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When AKUN-01 upload dokumen AMENDMENT_KONTRAK_OBLIGASI untuk instrumen tersebut
    And dokumen di-approve
    Then tidak ada baris baru di ecl.eir_reestimation_log
    And log informasi "EIR amendment tidak applicable untuk instrumen FVTPL, skip" ditulis ke application log (bukan audit)

  Scenario: Sudah ada proposal amandemen aktif — tidak ada proposal duplikat
    Given instrumen "OBL-2026-00001" sudah punya proposal dengan workflow_status = "PENDING_REVIEW"
    When AKUN-01 upload dokumen amandemen kedua dan di-approve
    Then tidak ada baris proposal baru di ecl.eir_reestimation_log
    And notifikasi dikirim ke ROLE-RISK:
      "Dokumen amandemen baru diterima untuk OBL-2026-00001, namun sudah ada proposal aktif (ID: ...). Harap selesaikan proposal yang ada."
    And aud.audit_log berisi event "EIR.AMEND_AUTO_SKIPPED" dengan alasan "active_proposal_exists"

  Scenario: Dokumen gagal validasi schema (format tidak dikenali)
    When AKUN-01 upload dokumen dengan document_type = "INVOICE" (tipe tidak relevan)
    Then sistem tidak membuat proposal amandemen
    And response HTTP 422 dengan error code "VALIDATION_FAILED"
    And message "document_type INVOICE tidak memicu auto-amendment. Gunakan AMENDMENT_KONTRAK_DEPOSITO atau AMENDMENT_KONTRAK_OBLIGASI."

  Scenario: Instrumen eir_awal NULL — dokumen diupload tapi proposal tidak bisa dibuat
    Given instrumen "DEP-2026-00099" dengan eir_awal IS NULL
    When dokumen amandemen di-upload dan di-approve untuk instrumen tersebut
    Then sistem gagal membuat proposal
    And notifikasi warning dikirim ke AKUN-01:
      "Proposal amandemen tidak bisa dibuat untuk DEP-2026-00099: EIR belum pernah dihitung. Compute EIR terlebih dahulu."
    And aud.audit_log berisi event "EIR.AMEND_AUTO_FAILED" dengan alasan "eir_not_computed"
    And tidak ada baris baru di ecl.eir_reestimation_log

  Scenario: Proposal auto-created hanya berisi metadata — cashflow revisi belum tersedia
    Given proposal baru terbentuk via DOCUMENT_UPLOAD
    Then revised_cashflow_json di ecl.eir_reestimation_log BOLEH NULL pada saat pembuatan
    And ROLE-RISK yang mereview wajib melengkapi cashflow revisi sebelum bisa forward ke PENDING_APPROVAL
    And tampilan antarmuka review menunjukkan warning "Cashflow revisi belum diisi oleh proposer"
```

### Open Questions terkait Story 1
- **OQ-M6-1** [PERLU JAWABAN sebelum implementasi]: Notifikasi amandemen baru — apakah via **in-app notification** saja, **email** saja, atau keduanya? Default assume: in-app + email. Konfirmasi ke ROLE-RISK/ROLE-AKUN-CTL.
- **OQ-M6-2**: Siapa yang mengisi `revised_cashflow_json` jika proposal di-auto-create dari document upload (cashflow revisi belum diketahui saat itu)? Apakah ROLE-AKUN mengisi cashflow via API terpisah `PATCH /api/v1/eir/amendments/{id}/cashflows`, atau ROLE-RISK mengisi saat review? Perlu satu langkah UI yang jelas. Default assume: ROLE-AKUN wajib mengisi cashflow revisi sebelum reviewer bisa forward ke PENDING_APPROVAL.

---

## Story APP-C-M6-002 — Scheduled Periodic EIR Drift Detection (Daily Cron)

**Actor**: Asynq cron job (dijalankan harian oleh system, tidak memerlukan interaksi user)
**Trigger**: Cron schedule harian jam 02:00 WIB (UTC+7), atau manual trigger oleh ROLE-RISK via API
**Goal**: Scan semua instrumen aktif yang ber-EIR; bandingkan `eir_awal` tersimpan dengan re-compute Newton-Raphson dari schedule; flag drift; simpan ke `sys.drift_report`; auto-create proposal amandemen untuk instrumen dengan drift melampaui threshold tinggi (`drift_high_threshold` = `0.001`, yakni 10 bp — perlu konfirmasi ALCO, lihat OQ-M6-3).

**Pre-conditions**:
- Tidak ada cron job drift detection lain yang sedang berjalan untuk tenant yang sama.
- Parameter `drift_low_threshold = 0.0001` (1 bp — flag only) dan `drift_high_threshold` (perlu konfirmasi ALCO) tersedia di `sys.parameter` atau hardcoded dengan OQ terbuka.
- `sys.job` tabel tersedia untuk persist job state.

**Post-conditions**:
- Baris baru di `sys.drift_report` dengan semua instrumen yang di-scan, drift count, missing count.
- Untuk setiap instrumen dengan drift > `drift_high_threshold` dan belum punya proposal aktif: satu baris baru di `ecl.eir_reestimation_log` dengan `trigger_source = 'CRON_DRIFT'` dan `drift_report_id = {id report}`.
- Audit events `EIR.DRIFT_DETECTED` (summary per run) dan `EIR.AMEND_AUTO_CREATED` (per proposal yang dibuat).

**Long-running**: UX §3 mandatory jika di-trigger manual oleh ROLE-RISK (202 + jobId + SSE progress).
**Permission**: System (cron, tidak butuh user permission). Manual trigger: `eir.amendment.detect` (ROLE-RISK).

### Acceptance Criteria

```gherkin
Feature: Daily EIR drift detection cron job

  Background:
    Given 300 instrumen aktif dengan klasifikasi_psak71 IN ('AC', 'FVOCI') dan eir_method_flag = true
    And tidak ada cron drift job lain yang sedang berjalan
    And sys.parameter berisi:
      | drift_low_threshold  | 0.0001 |
      | drift_high_threshold | 0.001  |  # OQ-M6-3: nilai ini perlu konfirmasi ALCO

  Scenario: Happy path — cron berjalan, semua instrumen valid, tidak ada drift
    Given semua instrumen punya eir_awal yang tepat (delta re-compute = 0)
    When cron job berjalan jam 02:00 WIB
    Then sys.drift_report berisi baris baru dengan:
      | trigger_type     | CRON_DAILY |
      | total_scanned    | 300        |
      | drift_count      | 0          |
      | missing_count    | 0          |
      | proposal_auto_created | 0     |
    And aud.audit_log berisi event "EIR.DRIFT_DETECTED" dengan:
      | after_jsonb.trigger_type | CRON_DAILY |
      | after_jsonb.drift_count  | 0          |
    And tidak ada proposal baru di ecl.eir_reestimation_log

  Scenario: Drift rendah terdeteksi (> low threshold, <= high threshold) — flag only
    Given instrumen "OBL-2026-00002" dengan eir_awal = 0.04000000
    And re-compute menghasilkan EIR = 0.04005000 (delta = 0.00005000, < drift_high_threshold)
    When cron job berjalan
    Then sys.drift_report.result_jsonb berisi entry untuk "OBL-2026-00002" dengan:
      | eir_stored    | 0.04000000 |
      | eir_computed  | 0.04005000 |
      | abs_diff      | 0.00005000 |
      | severity      | LOW        |
    And tidak ada proposal amandemen auto-created untuk instrumen tersebut
    And instrumen di-flag "REVIEW_RECOMMENDED" di drift report

  Scenario: Drift tinggi terdeteksi (> high threshold) — auto-create proposal
    Given instrumen "OBL-2026-00003" dengan eir_awal = 0.04000000
    And re-compute menghasilkan EIR = 0.04200000 (delta = 0.00200000, > drift_high_threshold 0.001)
    And belum ada proposal aktif untuk "OBL-2026-00003"
    When cron job berjalan
    Then ecl.eir_reestimation_log berisi baris baru dengan:
      | instrumen_id    | OBL-2026-00003 |
      | workflow_status | PENDING_REVIEW |
      | trigger_source  | CRON_DRIFT     |
      | drift_report_id | {id drift report} |
    And sys.drift_report.proposal_auto_created bertambah 1
    And aud.audit_log berisi event "EIR.AMEND_AUTO_CREATED" dengan:
      | after_jsonb.trigger_source | CRON_DRIFT |
      | after_jsonb.delta_eir      | 0.00200000 |
    And notifikasi dikirim ke ROLE-RISK: "Drift EIR signifikan terdeteksi pada OBL-2026-00003 (Δ = 0.0020, 20 bp). Proposal amandemen otomatis dibuat."

  Scenario: Instrumen sudah punya proposal aktif — tidak duplikasi
    Given instrumen "OBL-2026-00003" sudah punya proposal dengan workflow_status = "PENDING_REVIEW"
    When cron job berjalan dan drift > high threshold untuk instrumen tersebut
    Then tidak ada proposal baru dibuat
    And drift report mencatat "skip: active_proposal_exists" untuk instrumen tersebut

  Scenario: Schedule missing — instrumen punya eir_awal tapi tanpa schedule rows
    Given instrumen "DEP-2026-00050" dengan eir_awal = 0.05000000 tapi tidak ada rows di ecl.eir_amortization_schedule
    When cron job berjalan
    Then sys.drift_report.missing_count bertambah 1
    And drift report berisi entry "OBL-2026-00050" dengan severity = "MISSING_SCHEDULE"
    Dan notifikasi ROLE-RISK: "1 instrumen tanpa schedule EIR ditemukan. Tindakan diperlukan."

  Scenario: Concurrent cron prevention — job kedua ditolak
    Given cron drift detection sudah berjalan (status "running")
    When cron mencoba trigger job kedua untuk tenant yang sama
    Then job kedua tidak di-submit ke Asynq
    And log warning "EIR drift detection job sudah berjalan, skip" ditulis ke application log

  Scenario: Manual trigger oleh ROLE-RISK via API
    Given ROLE-RISK memiliki permission eir.amendment.detect
    When ROLE-RISK POST /api/v1/eir/drift-detection/trigger
    Then response HTTP 202 dengan { jobId, statusUrl, streamUrl }
    And SSE stream mengirimkan progress setiap ~50 instrumen
    And sys.drift_report.trigger_type = "AD_HOC"
    And response 409 dengan CONFLICT jika job lain sedang berjalan

  Scenario: Instrumen FVTPL atau FVOCI_ELECTION — di-skip dari scan
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When cron job berjalan
    Then instrumen tersebut tidak masuk dalam total_scanned
    And tidak ada drift entry atau proposal dibuat untuknya
```

### Open Questions terkait Story 2
- **OQ-M6-3** [NEEDS ALCO SIGN-OFF]: `drift_high_threshold` untuk auto-create proposal — apakah 10 bp (0.001) atau nilai lain? **Flag: ALCO harus konfirmasi sebelum M6 merge.** Default sementara = `0.001`.
- **OQ-M6-4**: Apakah frekuensi scan harian (daily 02:00 WIB) sudah cukup, atau perlu opsi mingguan untuk instrumen dengan volatilitas rendah? Default assume daily. Konfirmasi ke ROLE-RISK / ROLE-ALCO.
- **OQ-M6-5**: Apakah `drift_low_threshold` dan `drift_high_threshold` harus bisa di-override per `periode_id` (ALCO per periode), atau cukup global di `sys.parameter`? Default assume global.

---

## Story APP-C-M6-003 — Ad-Hoc Bulk Re-estimation dengan Drift Report + Auto-Proposal

**Actor**: ROLE-RISK (trigger manual) atau hook pre-ECL calc run gate
**Trigger**: ROLE-RISK klik tombol "Jalankan Re-estimasi Bulk" di UI, atau ECL calc run submission secara otomatis menjalankan drift check sebagai pre-gate (opsional, lihat OQ-M6-6)
**Goal**: Jalankan bulk re-compute (existing `BulkService` dari M5) terhadap subset atau semua instrumen; generate drift report ke `sys.drift_report`; auto-create amendment proposals untuk instrumen dengan drift > `drift_high_threshold`; return hasil via SSE progress + notifikasi selesai.

**Catatan penting**: `BulkService.Recompute()` dari M5 sudah melakukan komputasi dan menghasilkan `BulkRecomputeResult`. M6 menambahkan: (a) persistensi hasil ke `sys.drift_report`, dan (b) auto-create amendment proposals berdasarkan result drift entries.

**Pre-conditions**:
- Caller memiliki permission `eir.amendment.detect` atau `eir.bulk_recompute`.
- Tidak ada bulk job lain yang sedang berjalan untuk tenant yang sama.

**Post-conditions**:
- `sys.drift_report` baris baru dengan `trigger_type = 'AD_HOC'` atau `'PRE_ECL_CALC'`.
- Proposal amandemen baru di `ecl.eir_reestimation_log` untuk setiap instrumen dengan drift > `drift_high_threshold` dan tanpa proposal aktif.
- Notifikasi selesai ke actor dengan link ke drift report.
- Audit events: `EIR.BULK_RECOMPUTE_STARTED`, `EIR.BULK_RECOMPUTE_COMPLETED`.

**Long-running**: UX §3 mandatory — Asynq job + SSE + `<JobProgressPanel>`. SLA ≤ 5 detik untuk 1000 instrumen (sesuai M5 SLA).
**Permission**: `eir.amendment.detect` atau `eir.bulk_recompute` (ROLE-RISK + System).

### Acceptance Criteria

```gherkin
Feature: Ad-hoc bulk re-estimation with drift report and auto-proposal creation

  Background:
    Given 1000 instrumen aktif dengan eir_method_flag = true dan klasifikasi_psak71 IN ('AC','FVOCI')
    And sys.parameter berisi drift_high_threshold = 0.001
    And ROLE-RISK memiliki permission eir.amendment.detect

  Scenario: Happy path — bulk selesai, drift report tersimpan, proposals auto-created
    Given 5 instrumen memiliki drift > 0.001 dan tidak ada proposal aktif untuk mereka
    When ROLE-RISK POST /api/v1/eir/bulk-recompute dengan body { scope: "ALL_ACTIVE" }
    Then response HTTP 202 dengan { jobId, statusUrl, streamUrl }
    And SSE stream mengirim event progress setiap ~100 instrumen dengan format:
      { "progress": 47, "currentStep": "Re-estimasi instrumen 470 dari 1000" }
    And job selesai dalam ≤ 5 detik untuk 1000 instrumen
    And sys.drift_report berisi baris baru dengan:
      | trigger_type           | AD_HOC |
      | total_scanned          | 1000   |
      | drift_count            | 5      |
      | proposal_auto_created  | 5      |
    And ecl.eir_reestimation_log berisi 5 baris baru dengan trigger_source = "AD_HOC_BULK"
    And aud.audit_log berisi "EIR.BULK_RECOMPUTE_STARTED" dan "EIR.BULK_RECOMPUTE_COMPLETED"
    And toast sukses muncul: "Re-estimasi bulk EIR selesai. 5 instrumen memerlukan amandemen. Lihat laporan."
      dengan action link ke /ecl/eir/drift-reports/{drift_report_id}

  Scenario: Bulk dengan scope SUBSET — hanya instrumen tertentu
    When ROLE-RISK POST /api/v1/eir/bulk-recompute dengan body:
      { "scope": "SUBSET", "instrumenIds": ["OBL-UUID-1", "OBL-UUID-2", "DEP-UUID-3"] }
    Then hanya 3 instrumen tersebut yang di-scan
    And sys.drift_report.total_scanned = 3
    And response dan audit trail mencatat instrumen IDs yang di-scope

  Scenario: Pre-ECL calc run gate (opsional hook)
    Given ECL calc run baru di-submit via POST /api/v1/ecl/calc-runs
    And system dikonfigurasi "run_eir_drift_check_before_ecl = true" di sys.parameter
    When ECL calc run submitted
    Then sistem otomatis menjalankan bulk drift check terlebih dahulu
    And calc run berstatus "PENDING_EIR_CHECK" sampai drift check selesai
    And sys.drift_report.trigger_type = "PRE_ECL_CALC"
    And jika drift_count > 0: alert warning ditampilkan di calc run UI "X instrumen memiliki EIR drift. Tinjau sebelum melanjutkan." (user dapat proceed atau cancel)
    And jika drift_count = 0: calc run lanjut otomatis

  Scenario: Concurrent prevention
    Given satu bulk job sedang berjalan
    When ROLE-RISK submit bulk job kedua
    Then response HTTP 409 dengan error code "CONFLICT"
    And message "Bulk EIR re-compute sedang berjalan (jobId: ...). Tunggu hingga selesai."

  Scenario: Partial failure — satu instrumen error tidak menghentikan batch
    Given instrumen "DEP-2026-00099" punya cashflow corrupt di schedule-nya
    When bulk berjalan
    Then job tetap memproses instrumen lain
    And sys.drift_report.result_jsonb berisi entry error untuk "DEP-2026-00099"
    And job selesai dengan status "completed_with_errors"
    And proposal tidak dibuat untuk instrumen yang gagal diproses

  Scenario: Cancel job yang sedang berjalan
    Given bulk job sedang berjalan dengan jobId "job_01HXYZ"
    When ROLE-RISK POST /api/v1/jobs/job_01HXYZ/cancel
    Then Asynq job dihentikan
    And sys.drift_report.result_jsonb berisi instrumen yang sudah diproses sebelum cancel
    And job.status = "cancelled"
    And aud.audit_log berisi "EIR.BULK_RECOMPUTE_CANCELLED" dengan jumlah instrumen yang sudah diproses

  Scenario: SLA — 1000 instrumen dalam ≤ 5 detik
    Given 1000 instrumen aktif
    When bulk re-compute selesai
    Then sys.job.completed_at - sys.job.started_at ≤ 5000ms
    And memory footprint per instrumen ≤ 10 KB (streaming, tidak semua load ke RAM sekaligus)
```

### Open Questions terkait Story 3
- **OQ-M6-6**: Apakah "pre-ECL calc run drift gate" bersifat mandatory (blok calc run jika ada drift) atau advisory (warning saja, user bisa lanjut)? Default assume advisory dengan warning. Konfirmasi ke ROLE-RISK + ROLE-ALCO.
- **OQ-M6-7**: Scope `SUBSET` — apakah instrumen IDs diinput manual di UI, atau bisa di-filter dari instrumen list (mis. "semua instrumen di portofolio X")? Default assume manual UUID list atau filter portofolio. Perlu desain UI dari `uiux-designer`.

---

## Story APP-C-M6-004 — Review Queue UI untuk Pending Amendments

**Actor**: ROLE-RISK (reviewer utama), ROLE-ALCO (approver), ROLE-AKUN (lihat status proposal milik sendiri)
**Trigger**: ROLE-RISK membuka halaman `/ecl/eir/amendments` di UI
**Goal**: Tampilkan daftar semua proposal amandemen EIR yang pending review atau approval dalam DataTable (sesuai UX §1: sort + filter + cursor paging + export), dengan filter cepat berdasarkan deadline, instrumen, severity drift, dan status. Memungkinkan ROLE-RISK langsung membuka detail proposal dan melakukan review dari antarmuka yang sama.

**Pre-conditions**:
- Caller ter-autentikasi dengan role ROLE-RISK, ROLE-ALCO, atau ROLE-AKUN.
- Token JWT valid dan berisi claim `permissions` yang mencakup `eir.amendment_review.read`.

**Permission**: `eir.amendment_review.read`
**Audit Event**: baca (read) tidak di-audit. Export di-audit dengan `EIR.AMENDMENT_EXPORT`.

### Acceptance Criteria

```gherkin
Feature: Amendment review queue DataTable

  Background:
    Given 20 proposal amandemen di ecl.eir_reestimation_log dengan berbagai status:
      | 8 PENDING_REVIEW | 5 PENDING_APPROVAL | 4 APPROVED | 2 REJECTED | 1 CANCELLED |
    And RISK-01 memiliki role ROLE-RISK dan permission eir.amendment_review.read

  Scenario: Happy path — ROLE-RISK membuka halaman dan melihat queue
    When RISK-01 mengakses GET /api/v1/eir/amendments?filter[workflow_status]=PENDING_REVIEW&sort=created_at:asc
    Then response HTTP 200 dengan:
      | data | array 8 proposal PENDING_REVIEW |
      | pagination.hasMore | false |
      | appliedFilter.workflow_status | PENDING_REVIEW |
      | appliedSort | [{col:"created_at", dir:"asc"}] |
    And setiap proposal berisi:
      | instrumen_id, kode_instrumen, workflow_status, trigger_source |
      | eir_lama, tanggal_amandemen, created_at, maker_id |
      | drift_delta (jika trigger_source=CRON_DRIFT atau AD_HOC_BULK) |

  Scenario: Filter multi-kolom — kombinasi status + trigger_source
    When GET /api/v1/eir/amendments?filter[workflow_status]=PENDING_REVIEW&filter[trigger_source]=DOCUMENT_UPLOAD
    Then hanya proposal PENDING_REVIEW dari trigger DOCUMENT_UPLOAD yang dikembalikan
    And appliedFilter berisi keduanya

  Scenario: Sort berdasarkan drift severity
    When GET /api/v1/eir/amendments?sort=abs_diff:desc
    Then proposal dengan drift terbesar tampil paling atas
    And sort indicator panah ↓ tampil di kolom "Drift (Δ EIR)" di UI tabel

  Scenario: Cursor pagination
    When GET /api/v1/eir/amendments?limit=5
    Then response berisi 5 proposal pertama dan pagination.nextCursor terisi
    When GET /api/v1/eir/amendments?cursor={nextCursor}&limit=5
    Then response berisi 5 proposal berikutnya

  Scenario: Filter cepat "Menunggu Aksi Saya" (kontekstual per role)
    Given RISK-01 login sebagai ROLE-RISK
    When RISK-01 klik tab "Menunggu Review Saya"
    Then hanya proposal dengan workflow_status = "PENDING_REVIEW" yang ditampilkan
    And proposal yang sudah di-review oleh RISK-01 tidak tampil di tab ini

    Given ALCO-01 login sebagai ROLE-ALCO
    When ALCO-01 klik tab "Menunggu Approval Saya"
    Then hanya proposal dengan workflow_status = "PENDING_APPROVAL" yang ditampilkan

  Scenario: ROLE-AKUN hanya melihat proposal milik sendiri
    Given AKUN-01 (ROLE-AKUN) punya 3 proposal sebagai maker
    And ada 5 proposal dari maker lain
    When AKUN-01 GET /api/v1/eir/amendments
    Then response hanya berisi 3 proposal milik AKUN-01
    Dan proposal dari maker lain tidak muncul di response

  Scenario: Export CSV sesuai UX §1
    When RISK-01 klik Export CSV dengan filter aktif filter[workflow_status]=PENDING_REVIEW
    Then file CSV ter-download dengan header Bahasa Indonesia
    And CSV hanya berisi 8 row (sesuai filter aktif, bukan semua 20 proposal)
    And aud.audit_log berisi event "EIR.AMENDMENT_EXPORT" dengan:
      | after_jsonb.format      | csv           |
      | after_jsonb.row_count   | 8             |
      | after_jsonb.filters     | { "workflow_status": "PENDING_REVIEW" } |

  Scenario: Halaman kosong — tidak ada proposal pending
    Given tidak ada proposal dengan status PENDING_REVIEW
    When RISK-01 mengakses queue dengan filter default
    Then response HTTP 200 dengan data = []
    And UI menampilkan empty state: "Tidak ada proposal amandemen yang menunggu review" + tombol "Refresh"

  Scenario: ROLE-AUDIT membuka queue — read-only, tidak ada tombol aksi
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 mengakses halaman amendment queue
    Then semua proposal tampil (semua status, semua maker)
    And tidak ada tombol "Review", "Approve", atau "Cancel" yang aktif di baris manapun
    And tombol Export CSV tersedia
```

---

## Story APP-C-M6-005 — Cancel / Withdraw Amendment Proposal (Pre-Approval)

**Actor**: ROLE-AKUN yang merupakan maker dari proposal (atau System yang membuat proposal via DOCUMENT_UPLOAD / CRON_DRIFT)
**Trigger**: Maker memutuskan membatalkan proposal sebelum reviewer menandatangani (misalnya data cashflow salah, kontrak amandemen dibatalkan)
**Goal**: Ubah status proposal dari `DRAFT` atau `PENDING_REVIEW` menjadi `CANCELLED`, catat `cancel_reason` yang informatif, tulis audit trail. Proposal yang sudah masuk ke `PENDING_APPROVAL` (reviewer sudah sign) tidak dapat di-cancel oleh maker — harus melalui Reject oleh reviewer/approver.

**Pre-conditions**:
- Proposal berstatus `DRAFT` atau `PENDING_REVIEW` (belum ada tanda tangan reviewer).
- Actor adalah user yang sama dengan `maker_id` di proposal, ATAU System (untuk proposal yang di-auto-create).
- `cancel_reason` wajib diisi, minimum 20 karakter.

**Post-conditions**:
- `ecl.eir_reestimation_log.workflow_status = 'CANCELLED'`.
- `ecl.eir_reestimation_log.cancelled_at` di-set ke timestamp saat ini.
- `ecl.eir_reestimation_log.cancel_reason` terisi.
- Instrumen kembali bisa menerima proposal amandemen baru (tidak ada proposal aktif).
- Audit event `EIR.AMEND_CANCELLED` ditulis ke `aud.audit_log` in-transaction.
- Notifikasi ke ROLE-RISK bahwa proposal dibatalkan.

**Workflow state setelah cancel**:
```
DRAFT          → CANCELLED  (oleh maker)
PENDING_REVIEW → CANCELLED  (oleh maker, sebelum reviewer sign)
PENDING_APPROVAL → TIDAK BISA di-cancel oleh maker; harus Reject via reviewer/approver
```

**Permission**: `eir.amendment.cancel`
**Audit Event**: `EIR.AMEND_CANCELLED`

### Acceptance Criteria

```gherkin
Feature: Cancel EIR amendment proposal pre-approval

  Background:
    Given instrumen "OBL-2026-00001" dengan eir_awal = 0.04200000
    And user AKUN-01 (ROLE-AKUN), RISK-01 (ROLE-RISK)
    And proposal "AMEND-001" dengan:
      | instrumen_id    | OBL-2026-00001 |
      | workflow_status | PENDING_REVIEW |
      | maker_id        | AKUN-01        |
    And AKUN-01 memiliki permission eir.amendment.cancel

  Scenario: Happy path — maker cancel proposal dalam PENDING_REVIEW
    When AKUN-01 POST /api/v1/eir/amendments/AMEND-001/cancel dengan body:
      { "cancel_reason": "Kontrak amandemen dibatalkan oleh counterparty, cashflow tetap original." }
    Then response HTTP 200 dengan proposal yang di-update:
      | workflow_status | CANCELLED  |
      | cancelled_at    | {timestamp now} |
      | cancel_reason   | "Kontrak amandemen dibatalkan oleh counterparty, cashflow tetap original." |
    And ecl.eir_reestimation_log tidak ada perubahan pada amounts finansial (eir_lama, eir_baru tetap sama)
    And ecl.eir_amortization_schedule tidak berubah (schedule aktif tetap sama)
    And aud.audit_log berisi event "EIR.AMEND_CANCELLED" dengan:
      | entity_type     | ecl.eir_reestimation_log |
      | entity_id       | AMEND-001                |
      | actor_user_id   | AKUN-01                  |
      | before_jsonb.workflow_status | PENDING_REVIEW |
      | after_jsonb.workflow_status  | CANCELLED      |
      | after_jsonb.cancel_reason    | "Kontrak amandemen dibatalkan..." |
    And notifikasi dikirim ke ROLE-RISK: "Proposal amandemen AMEND-001 untuk OBL-2026-00001 dibatalkan oleh maker."
    And instrumen "OBL-2026-00001" kini bisa menerima proposal baru

  Scenario: Cancel proposal dalam status DRAFT
    Given proposal "AMEND-002" dengan workflow_status = "DRAFT" dan maker_id = AKUN-01
    When AKUN-01 POST cancel dengan cancel_reason valid (≥ 20 karakter)
    Then response HTTP 200 dengan workflow_status = "CANCELLED"
    And aud.audit_log berisi "EIR.AMEND_CANCELLED"

  Scenario: Cancel setelah reviewer sign (PENDING_APPROVAL) — ditolak
    Given proposal "AMEND-003" dengan workflow_status = "PENDING_APPROVAL"
    And reviewer_id sudah terisi (RISK-01 sudah sign)
    When AKUN-01 POST cancel
    Then response HTTP 422 dengan error code "EIR_AMEND_INVALID_TRANSITION"
    And message "Proposal dalam status PENDING_APPROVAL tidak bisa di-cancel oleh maker. Minta reviewer atau approver untuk Reject."
    And proposal TIDAK berubah

  Scenario: Cancel proposal APPROVED — tidak bisa di-cancel
    Given proposal "AMEND-004" dengan workflow_status = "APPROVED"
    When AKUN-01 POST cancel
    Then response HTTP 422 dengan error code "EIR_AMEND_INVALID_TRANSITION"
    And message "Proposal yang sudah APPROVED tidak bisa dibatalkan."

  Scenario: SoD — user lain (bukan maker) mencoba cancel
    Given proposal "AMEND-001" dengan maker_id = AKUN-01
    And user AKUN-02 (ROLE-AKUN, berbeda dari maker) login
    When AKUN-02 POST cancel untuk AMEND-001
    Then response HTTP 403 dengan error code "FORBIDDEN"
    And message "Hanya maker proposal yang boleh membatalkan proposal ini."

  Scenario: cancel_reason terlalu pendek (< 20 karakter)
    When AKUN-01 POST cancel dengan body { "cancel_reason": "Salah input" }
    Then response HTTP 422 dengan error code "VALIDATION_FAILED"
    And detail: { field: "cancel_reason", rule: "min_length_20", message: "cancel_reason harus minimal 20 karakter. Saat ini: 10 karakter." }
    And proposal TIDAK berubah

  Scenario: cancel_reason tidak diisi (null atau empty string)
    When AKUN-01 POST cancel dengan body { "cancel_reason": "" }
    Then response HTTP 422 dengan error code "VALIDATION_FAILED"
    And detail: { field: "cancel_reason", rule: "required", message: "cancel_reason wajib diisi." }

  Scenario: Idempotent cancel — cancel proposal yang sudah CANCELLED
    Given proposal "AMEND-001" sudah berstatus CANCELLED
    When AKUN-01 POST cancel dengan Idempotency-Key yang sama
    Then response HTTP 200 dengan IDEMPOTENCY_REPLAY (return original response)
    When AKUN-01 POST cancel dengan Idempotency-Key baru
    Then response HTTP 422 dengan error code "EIR_AMEND_INVALID_TRANSITION"
    And message "Proposal sudah dalam status terminal CANCELLED."

  Scenario: Konfirmasi destruktif di UI
    When AKUN-01 klik tombol "Batalkan Proposal" di detail halaman amandemen
    Then dialog konfirmasi muncul:
      Judul: "Batalkan proposal amandemen AMEND-001?"
      Deskripsi: "Setelah dibatalkan, Anda perlu membuat proposal baru jika masih ingin melakukan amandemen EIR."
      Input wajib: textarea cancel_reason (placeholder: "Jelaskan alasan pembatalan (min 20 karakter)")
      Tombol: "Batalkan Proposal" (destructive/merah) | "Kembali"
    When AKUN-01 isi cancel_reason dan konfirmasi
    Then toast sukses: "Proposal amandemen AMEND-001 berhasil dibatalkan."
    And halaman kembali ke amendment queue
```

---

## Ringkasan Open Questions

| ID | Pertanyaan | Default sementara | Butuh konfirmasi dari | Blocking? |
|---|---|---|---|---|
| **OQ-M6-1** | Notifikasi: in-app saja, email saja, atau keduanya? | In-app + email | ROLE-RISK / ROLE-AKUN-CTL | Ya, sebelum implementasi notif |
| **OQ-M6-2** | Siapa mengisi `revised_cashflow_json` untuk proposal auto-created? | ROLE-AKUN via PATCH endpoint sebelum review | UX review dengan ROLE-AKUN | Ya, perlu flow UI |
| **OQ-M6-3** | `drift_high_threshold` untuk auto-create proposal — nilai berapa? | `0.001` (10 bp) sementara | **ALCO wajib sign-off** | **BLOCKING** |
| **OQ-M6-4** | Frekuensi cron drift: daily atau opsi weekly? | Daily 02:00 WIB | ROLE-RISK / ROLE-ALCO | Tidak blocking, bisa konfig |
| **OQ-M6-5** | Threshold global atau dapat override per periode? | Global di `sys.parameter` | ROLE-ALCO | Tidak blocking untuk M6 |
| **OQ-M6-6** | Pre-ECL drift gate: mandatory atau advisory? | Advisory (warning, user bisa lanjut) | ROLE-RISK + ROLE-ALCO | Ya, sebelum P4-M7 merge |
| **OQ-M6-7** | Scope SUBSET di bulk: manual UUID atau filter portofolio? | Manual UUID list | `uiux-designer` | Tidak blocking backend |

---

## Matriks Ringkasan Permissions

| Story | Permission | Actor |
|---|---|---|
| M6-001 (detect via upload) | `doc.document.create` (uploader), `eir.amendment.detect` (system worker) | ROLE-AKUN, System |
| M6-002 (cron drift) | `eir.amendment.detect` (manual trigger), System (cron) | System, ROLE-RISK |
| M6-003 (ad-hoc bulk) | `eir.amendment.detect` atau `eir.bulk_recompute` | ROLE-RISK, System |
| M6-004 (review queue) | `eir.amendment_review.read` | ROLE-RISK, ROLE-ALCO, ROLE-AKUN (proposal sendiri), ROLE-AUDIT |
| M6-005 (cancel) | `eir.amendment.cancel` | ROLE-AKUN (maker), System (untuk auto-created) |

---

## Handoff Checklist

```
P4-M6 stories selesai →

  system-analyst:
    - OpenAPI fragment tambahan: eir-amendment-lifecycle.yaml
      Endpoint baru:
        - GET  /api/v1/eir/amendments                           (queue, UX §1)
        - POST /api/v1/eir/amendments/{id}/cancel               (M6-005)
        - POST /api/v1/eir/drift-detection/trigger              (M6-002 manual trigger)
        - GET  /api/v1/eir/drift-reports                        (M6-004 view reports)
        - GET  /api/v1/eir/drift-reports/{id}                   (detail)
      State machine tambahan: PENDING_REVIEW → CANCELLED (via cancel)
      Error codes baru: EIR_CANCEL_FORBIDDEN, EIR_CANCEL_REASON_TOO_SHORT

  data-modeler (PARALEL dengan system-analyst):
    - migration 000027:
      - CREATE TABLE sys.drift_report (sesuai spec di atas)
      - ALTER TABLE ecl.eir_reestimation_log ADD COLUMN cancelled_at, cancel_reason,
        trigger_source, drift_report_id, document_id
      - CHECK constraint cancel_reason: length >= 20 (atau enforce di service)
      - INDEX idx_drift_report_run_at pada sys.drift_report
      - INDEX idx_eir_reestimation_instrumen_status pada (instrumen_id, workflow_status)
        WHERE workflow_status NOT IN ('APPROVED', 'REJECTED', 'CANCELLED')

  ecl-eir-engineer:
    - Implementasi DetectionService (trigger dari doc.document event)
    - Implementasi DriftCronHandler (Asynq cron job + scheduler config)
    - Update BulkService: persist ke sys.drift_report + auto-create proposals
    - Implementasi Cancel logic (service + repo)
    - Implementasi notification dispatch (in-app + email hook)

  uiux-designer (PARALEL):
    - Desain halaman /ecl/eir/amendments (DataTable UX §1)
    - Desain cancel confirmation dialog (UX §2 destructive)
    - Desain drift report view
    - Desain notifikasi banner untuk ROLE-RISK

  ifrs9-compliance-reviewer:
    BLOCKING gate sebelum merge PR P4-M6:
    - Verify bahwa auto-created proposals tidak langsung mengubah eir_awal atau schedule
      (harus tetap melewati 4-eyes full approval)
    - Verify cancel flow: amounts finansial di schedule TIDAK berubah saat cancel
    - Verify drift_high_threshold logic konsisten dengan DEC-013
    - Verify OQ-M6-3 (threshold value) sudah dikonfirmasi ALCO sebelum merge
```
