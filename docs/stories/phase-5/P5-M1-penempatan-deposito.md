# P5-M1 — trx.penempatan_deposito CRUD + 4-Eyes Workflow: User Stories

**Story Set ID**: P5-M1
**Modul**: APP-B — Transaction Lifecycle (Phase 5, Sprint 1)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate) + `security-engineer` (BLOCKING gate)
**Business OQs**: OQ-M1-1a RESOLVED (DEC-P5-M1-004, stakeholder sign-off PENDING) · OQ-M1-5a RESOLVED (DEC-P5-M1-005, stakeholder sign-off PENDING) — lihat `docs/decisions/P5-M1-business-decisions.md`
**Author**: business-analyst
**Tanggal**: 2026-06-14
**Linked FSD**: FSD-APP-B-TransactionLifecycle-v1.1.docx §1 (Modul Penempatan)
**Linked BRD**: BRD §6.2 (APP-B Transaction Lifecycle), RACI: ROLE-MAKER-TR (R), ROLE-APPR-TR (A), ROLE-RISK (C), ROLE-AUDIT (I)
**Linked Decision Log**: DEC-013 (EIR Newton-Raphson), DEC-016 (decimal precision), DEC-017 (4-eyes SoD), DEC-018 (audit trail), DEC-021 (Idempotency-Key), DEC-022 (cursor pagination)

**Handoff berikutnya**: `system-analyst` (OpenAPI + state machine + error codes ERR-VAL-2001..2005), lalu `data-modeler` (migration 000028 `trx.penempatan_deposito`), lalu `backend-engineer-go` + `ecl-eir-engineer` (EIR trigger post-approve)

**Compliance path**: P5-M1 adalah BLOCKING gate — EIR auto-trigger post-approve + event PENEMPATAN untuk jurnal engine (P5-M2) wajib diverifikasi `ifrs9-compliance-reviewer` sebelum merge ke `develop`.

---

## Konteks & Dependensi Phase 3 + Phase 4

Phase 3 (master data) telah mendeliver:
- `mst.instrumen` — CRUD + klasifikasi PSAK71 locked (`AC`, `FVOCI`, `FVTPL`, `FVOCI_ELECTION`, `POCI`), field `status = 'AKTIF'`, `tipe_instrumen`, `counterparty_id`, `portofolio_id`
- `mst.counterparty` — counterparty bank aktif, `status = 'AKTIF'`, `rating_pefindo` terkini
- `mst.kurs` — FX rate BI JISDOR (dikelola P5-M5, tapi schema sudah ada dari Phase 3)
- `mst.periode_buku` — state `OPEN`, `SOFT_CLOSED`, `HARD_CLOSED`; penempatan hanya boleh saat `OPEN`
- `doc.*` — document upload service (Phase 2), kontrak deposito wajib dilampirkan

Phase 4 (ECL Engine + EIR) telah mendeliver:
- `ecl.eir_service` — Newton-Raphson IRR solver, presisi 8 desimal, tolerance 1e-10 (DEC-013)
- `ecl.amortisasi_schedule` — immutable versioned schedule per instrumen
- `ecl.stage_history` — initial staging (Stage 1 default untuk instrumen baru non-POCI)
- `ecl.calc_header` / `ecl.calc_result_line` — ECL calc run engine operational

**Catatan POCI**: Instrumen `klasifikasi_psak71 = 'POCI'` membutuhkan credit-adjusted EIR sejak inisiasi. EIR trigger post-approve untuk POCI memanggil CA-EIR solver (Phase 4.5, PR #89). Delta ECL POCI akan dikerjakan di P5-M10.

---

## Skema `trx.penempatan_deposito` (Baru — Migration 000028)

Kolom utama yang direferensi di AC berikut (bukan exhaustive — data-modeler tentukan schema penuh):

| Kolom | Tipe | Keterangan |
|---|---|---|
| `id` | UUID PK | gen_random_uuid() |
| `kode_transaksi` | TEXT UNIQUE NOT NULL | Format `PNP-{YYYY}-{#####}`, auto-generate server-side |
| `instrumen_id` | UUID FK `mst.instrumen.id` | |
| `counterparty_bank_id` | UUID FK `mst.counterparty.id` | Harus bank, bukan issuer obligasi |
| `periode_id` | UUID FK `mst.periode_buku.id` | Periode `OPEN` saat create |
| `tanggal_penempatan` | DATE NOT NULL | ≤ today |
| `tanggal_jatuh_tempo` | DATE NOT NULL | Auto-compute: `tanggal_penempatan + tenor_bulan months` |
| `nominal_idr` | NUMERIC(20,4) NOT NULL | IDR amount (konversi FCY via kurs JISDOR) |
| `nominal_fcy` | NUMERIC(20,4) | Null jika IDR |
| `mata_uang_id` | UUID FK `mst.mata_uang.id` | |
| `kurs_penempatan` | NUMERIC(20,8) | BI JISDOR rate tanggal penempatan |
| `tenor_bulan` | SMALLINT NOT NULL | > 0 |
| `kupon_persen` | NUMERIC(10,8) NOT NULL | ≥ 0 |
| `nomor_referensi_bank` | TEXT | Nomor konfirmasi dari bank counterparty |
| `eir_awal` | NUMERIC(10,8) | Di-set post-approve oleh EIR solver |
| `carrying_amount_awal` | NUMERIC(20,4) | = nominal_idr (sebelum amortisasi) |
| `workflow_status` | TEXT NOT NULL | DRAFT, PENDING_REVIEW, PENDING_APPROVAL, APPROVED_ACTIVE, MATURED, TERMINATED |
| `maker_id` | UUID FK `sec.user.id` | |
| `reviewer_id` | UUID FK `sec.user.id` | Null saat DRAFT |
| `approver_id` | UUID FK `sec.user.id` | Null sampai APPROVED |
| `reject_reason` | TEXT | ≥ 30 chars jika REJECTED |
| `reviewer_signed_at` | TIMESTAMPTZ | |
| `approver_signed_at` | TIMESTAMPTZ | |
| `reviewer_signature_hash` | TEXT | SHA-256 dari payload review |
| `approver_signature_hash` | TEXT | SHA-256 dari payload approve |
| `terminate_reason` | TEXT | ≥ 30 chars jika TERMINATED |
| `terminated_at` | DATE | |
| `matured_at` | DATE | Di-set saat jatuh tempo |
| `settlement_account` | TEXT | Nomor rekening settlement Tugure |
| `biaya_transaksi_idr` | NUMERIC(20,4) | Biaya transaksi untuk EIR calculation |
| `catatan` | TEXT | |
| ...audit cols... | — | `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `row_version`, `tenant_id` |

---

## Status Lifecycle (State Machine Ringkas)

```
DRAFT
  ├─ submit by maker ──────────────────────────► PENDING_REVIEW
  │                                                    │
  │                              review by reviewer ───┤
  │                                                    ▼
  │                                           PENDING_APPROVAL
  │                                                    │
  │                         approve by approver ───────┼──► APPROVED_ACTIVE
  │                                                    │        │
  │                         reject at any step ────────►        │ Asynq maturity job
  │                                ▼                            ├──────────────────► MATURED (auto)
  └─ withdraw by maker ──► CANCELLED                            │
                                                                │ manual terminate (4-eyes)
                                                                │  DEC-P5-M1-005
                                                                ▼
                                                   TERMINATION_PENDING_REVIEW
                                                                │
                                                  reviewer sign-off ─────────► TERMINATION_PENDING_APPROVAL
                                                                                        │
                                                              approver (TM) approve ────┼──► TERMINATED
                                                                                        │
                                                              reject at any step ────────►  APPROVED_ACTIVE
                                                                                              (proposal dropped)
```

Transition valid:
- `DRAFT → PENDING_REVIEW`: maker submit
- `PENDING_REVIEW → PENDING_APPROVAL`: reviewer approve
- `PENDING_REVIEW → DRAFT`: reviewer reject (maker dapat edit + resubmit)
- `PENDING_APPROVAL → APPROVED_ACTIVE`: approver approve + trigger EIR + staging
- `PENDING_APPROVAL → DRAFT`: approver reject (maker dapat edit + resubmit)
- `DRAFT → CANCELLED`: maker withdraw (soft-delete, tidak bisa di-resubmit)
- `APPROVED_ACTIVE → MATURED`: Asynq job pada `tanggal_jatuh_tempo`
- `APPROVED_ACTIVE → TERMINATED`: manual 4-eyes (Maker submit terminate → APPR-TR approve)

---

## Story P5-M1-S1 — Create Penempatan Deposito (Maker)

**Actor**: ROLE-MAKER-TR
**Trigger**: Transaksi penempatan obligasi atau deposito baru dilakukan (bond/deposito di bank counterparty)
**Goal**: Maker membuat record penempatan dalam status DRAFT, dengan `kode_transaksi` di-generate otomatis, siap untuk di-submit ke workflow 4-eyes

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-MAKER-TR dengan permission `transaksi.create`
2. `mst.instrumen` target: `status = 'AKTIF'`, `klasifikasi_psak71 IN ('AC', 'FVOCI', 'FVTPL', 'FVOCI_ELECTION', 'POCI')`, `workflow_status = 'APPROVED'` (klasifikasi sudah locked)
3. `mst.counterparty` target: `status = 'AKTIF'`, tipe sesuai (bank untuk deposito)
4. `mst.periode_buku` aktif: `status_periode = 'OPEN'`
5. FX rate tersedia di `mst.kurs` untuk tanggal penempatan (jika FCY)
6. Minimal 1 dokumen kontrak sudah di-upload via `doc.*` service (bisa dilampirkan saat create atau sebelum submit)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | CREATE | Record baru, `workflow_status = 'DRAFT'` |
| `mst.instrumen` | READ | Validasi status, klasifikasi, counterparty |
| `mst.counterparty` | READ | Validasi status bank |
| `mst.periode_buku` | READ | Pastikan OPEN |
| `mst.kurs` | READ | FX rate untuk konversi FCY→IDR |
| `doc.upload` | READ | Referensi dokumen kontrak |

### Permissions
| Permission | Catatan |
|---|---|
| `transaksi.create` | Wajib |
| `instrumen.read` | Untuk picker instrumen |
| `counterparty.read` | Untuk picker bank |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.CREATE` | Record DRAFT berhasil dibuat |
| `PENEMPATAN.DOCUMENT_ATTACHED` | Dokumen dilampirkan ke penempatan |

### Acceptance Criteria

```gherkin
Feature: Create penempatan deposito oleh Treasury Maker

  Background:
    Given user ter-autentikasi sebagai ROLE-MAKER-TR (user: "Ahmad Fauzi", user_id: USR-001)
    And instrumen INST-DEP-001 (deposito BCA, klasifikasi_psak71 = 'AC', status = 'AKTIF')
    And counterparty CP-BCA-001 (BCA, status = 'AKTIF')
    And periode buku Juni-2026 status = 'OPEN'
    And mst.kurs USD/IDR pada 2026-06-14 = 15432.50 tersedia

  # ─── HAPPY PATH 1: Penempatan deposito IDR berhasil dibuat ───────────────

  Scenario: Maker berhasil membuat penempatan deposito IDR dalam status DRAFT
    Given Maker mengisi form penempatan:
      | instrumen_id          | INST-DEP-001     |
      | counterparty_bank_id  | CP-BCA-001       |
      | tanggal_penempatan    | 2026-06-14       |
      | nominal_idr           | 5000000000.0000  |
      | mata_uang_id          | IDR              |
      | tenor_bulan           | 12               |
      | kupon_persen          | 0.05250000       |
      | nomor_referensi_bank  | BCA/DEP/2026/001 |
      | settlement_account    | 1234567890       |
      | biaya_transaksi_idr   | 0.0000           |
    And Maker melampirkan dokumen kontrak: "kontrak_deposito_BCA_Jun2026.pdf" (doc_id: DOC-001)
    When Maker mengirim POST /api/v1/transaksi/penempatan dengan Idempotency-Key: IK-001
    Then sistem mengembalikan HTTP 201 Created dengan body:
      | kode_transaksi     | PNP-2026-00001    |
      | workflow_status    | DRAFT             |
      | tanggal_jatuh_tempo| 2026-06-14 + 12 months = 2027-06-14 |
      | instrumen_id       | INST-DEP-001      |
      | nominal_idr        | 5000000000.0000   |
    And record `trx.penempatan_deposito` ter-insert dengan:
      | maker_id           | USR-001           |
      | reviewer_id        | NULL              |
      | approver_id        | NULL              |
      | eir_awal           | NULL (belum computed) |
    And audit log: PENEMPATAN.CREATE, actor = USR-001, entity_id = penempatan.id
    And audit log: PENEMPATAN.DOCUMENT_ATTACHED, doc_id = DOC-001
    And toast sukses: "Penempatan PNP-2026-00001 berhasil dibuat. Status: DRAFT. Lampirkan dokumen jika belum, lalu submit untuk review."

  # ─── HAPPY PATH 2: Penempatan FCY (USD) dengan konversi kurs ────────────

  Scenario: Maker berhasil membuat penempatan deposito USD, nominal_idr di-compute dari kurs
    Given Maker mengisi form penempatan:
      | instrumen_id          | INST-DEP-USD-001  |
      | counterparty_bank_id  | CP-CITI-001       |
      | tanggal_penempatan    | 2026-06-14        |
      | nominal_fcy           | 1000000.0000      |
      | mata_uang_id          | USD               |
      | tenor_bulan           | 6                 |
      | kupon_persen          | 0.04500000        |
    And kurs USD/IDR pada 2026-06-14 = 15432.50 tersedia di mst.kurs
    When Maker mengirim POST /api/v1/transaksi/penempatan
    Then sistem menghitung nominal_idr = 1000000 × 15432.50 = 15432500000.0000
    And record ter-insert dengan:
      | nominal_fcy         | 1000000.0000      |
      | kurs_penempatan     | 15432.50000000    |
      | nominal_idr         | 15432500000.0000  |
    And audit log: PENEMPATAN.CREATE dengan mata_uang = 'USD'

  # ─── ERROR CASE: Instrumen belum memiliki klasifikasi locked ─────────────

  Scenario: Create gagal karena klasifikasi instrumen belum di-approve (ERR-VAL-2001)
    Given instrumen INST-NEW-001 baru dibuat, workflow_status = 'PENDING_REVIEW' (klasifikasi belum locked)
    When Maker mencoba create penempatan dengan instrumen_id = INST-NEW-001
    Then sistem mengembalikan HTTP 422 Unprocessable Entity:
      | error.code    | ERR-VAL-2001                                              |
      | error.message | "Instrumen INST-NEW-001 belum memiliki klasifikasi PSAK 71 yang di-approve. Selesaikan proses klasifikasi terlebih dahulu." |
    And tidak ada record `trx.penempatan_deposito` yang dibuat

  # ─── INFORMATIONAL: Settlement balance hint (DEC-P5-M1-004) ─────────────
  # NOTE: ERR-VAL-2002 (hard block) adalah Phase 6 deferred (post P5-M14 GL Host live).
  # Phase 5: balance ditampilkan sebagai informational hint saja — tidak memblok create.

  Scenario: Create menampilkan informational balance hint jika saldo tersedia di sys.settlement_account_balance
    Given sys.settlement_account_balance untuk settlement_account "1234567890":
      | last_known_balance_idr | 3000000000.0000 |
      | as_of_date             | 2026-06-13      |
    And Maker memasukkan nominal_idr = 5000000000.0000 (melebihi saldo yang diketahui)
    When Maker mengirim POST /api/v1/transaksi/penempatan
    Then sistem mengembalikan HTTP 201 Created (penempatan berhasil dibuat, tidak diblok)
    And response body mengandung field:
      | settlement_balance_hint.last_known_idr | 3000000000.0000 |
      | settlement_balance_hint.as_of_date     | 2026-06-13      |
      | settlement_balance_hint.is_sufficient  | null            |
    And UI menampilkan warning informatif (amber, non-blocking):
      "Perhatian: Nominal (IDR 5.000.000.000) melebihi saldo terakhir yang diketahui (IDR 3.000.000.000 per 2026-06-13). Pastikan saldo tersedia sebelum submit."
    And record trx.penempatan_deposito ter-insert dengan workflow_status = DRAFT
    And audit log: PENEMPATAN.CREATE (tidak ada catatan ERR-VAL-2002)

  Scenario: Create tanpa saldo tersedia di sys.settlement_account_balance — hint tidak muncul
    Given sys.settlement_account_balance tidak memiliki record untuk settlement_account "9999999999"
    When Maker mengirim POST /api/v1/transaksi/penempatan dengan settlement_account = "9999999999"
    Then sistem mengembalikan HTTP 201 Created
    And response body mengandung:
      | settlement_balance_hint | null (absent) |
    And UI menampilkan label: "Saldo tidak tersedia — pastikan saldo mencukupi sebelum submit"
    And tidak ada block atau warning merah — hanya info abu-abu

  # ─── ERROR CASE: Periode buku bukan OPEN ─────────────────────────────────

  Scenario: Create gagal karena periode buku sudah di-close
    Given periode buku Juni-2026 status = 'SOFT_CLOSED'
    When Maker mencoba create penempatan dengan tanggal_penempatan = 2026-06-14
    Then sistem mengembalikan HTTP 423:
      | error.code    | PERIODE_CLOSED                                             |
      | error.message | "Periode buku Juni-2026 sudah di-close. Penempatan tidak dapat dibuat." |

  # ─── ERROR CASE: Tenor tidak valid ───────────────────────────────────────

  Scenario: Validasi gagal — tenor_bulan = 0
    When Maker mengirim form dengan tenor_bulan = 0
    Then sistem mengembalikan HTTP 400:
      | error.code    | VALIDATION_FAILED                   |
      | error.details[0].field | tenor_bulan          |
      | error.details[0].rule  | "gt:0 (tenor harus > 0 bulan)"  |

  # ─── EDGE CASE: Idempotency — double submit dengan key yang sama ──────────

  Scenario: Maker mengirim request yang sama dua kali dengan Idempotency-Key yang sama
    Given request pertama dengan Idempotency-Key: IK-001 berhasil (HTTP 201, PNP-2026-00001)
    When Maker mengirim request kedua identik dengan Idempotency-Key: IK-001
    Then sistem mengembalikan HTTP 200 dengan response identik (IDEMPOTENCY_REPLAY)
    And tidak ada record duplikat di trx.penempatan_deposito
    And kode_transaksi tetap PNP-2026-00001

```

### Open Questions — Story 1
| ID | Pertanyaan | Status | Resolusi |
|---|---|---|---|
| OQ-M1-1a | Apakah saldo rekening settlement di-validasi real-time atau hanya informational warning? | **RESOLVED** — ref DEC-P5-M1-004 | **Option B Informational only** (Phase 5). ERR-VAL-2002 hard block deferred ke Phase 6 setelah P5-M14 GL Host live. Balance hint ditampilkan dari `sys.settlement_account_balance`, non-blocking. Stakeholder formal sign-off: PENDING (lihat `docs/decisions/P5-M1-business-decisions.md`). |
| OQ-M1-1b | `kode_transaksi` format — apakah `PNP-{YYYY}-{#####}` sudah sesuai FSD-APP-B §1.3, atau ada format berbeda untuk deposito vs obligasi? | OPEN | Asumsi format generik `PNP`. Flag ke system-analyst untuk konfirmasi. |
| OQ-M1-1c | Apakah FCY deposit wajib ada kurs dari `mst.kurs`, atau Maker boleh input kurs manual (override)? | OPEN | Wajib dari `mst.kurs` (BI JISDOR, DEC per FX — dikonfirmasi P5-M5). Kurs manual hanya jika tanggal tidak memiliki JISDOR (weekend/holiday) dan FinCon approve. |

---

## Story P5-M1-S2 — Submit + Review + Approve Penempatan (4-Eyes Workflow)

**Actor**: ROLE-MAKER-TR (submit), ROLE-APPR-TR (review), ROLE-APPR-TR senior / Treasury Manager (approve)
**Trigger**: Maker menyelesaikan pengisian form DRAFT dan siap mengirim ke workflow; Reviewer memeriksa kelengkapan; Approver memberikan persetujuan akhir
**Goal**: Penempatan di-approve melalui 4-eyes dengan SoD enforcement; on approve: EIR initial computation di-trigger dan ECL initial staging (Stage 1) di-record

### Pre-conditions
1. Penempatan dalam status yang valid untuk masing-masing step:
   - Submit: `DRAFT`, actor = maker_id (pemilik)
   - Review: `PENDING_REVIEW`, actor ≠ maker_id
   - Approve: `PENDING_APPROVAL`, actor ≠ maker_id AND actor ≠ reviewer_id
   - Reject: `PENDING_REVIEW` atau `PENDING_APPROVAL`, dengan `reject_reason` ≥ 30 chars
2. Untuk Approve: periode buku masih `OPEN` (re-check saat approve)
3. Minimal 1 dokumen kontrak terlampir pada record penempatan

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | UPDATE (`workflow_status`, `reviewer_id`, `approver_id`, `*_signed_at`, `*_signature_hash`) | |
| `ecl.stage_history` | INSERT (append-only) | Initial staging Stage 1, triggered post-approve |
| `ecl.amortisasi_schedule` | INSERT | EIR amortization schedule (via ecl-eir-engineer) |
| `mst.instrumen` | READ | EIR computation input |
| `mst.periode_buku` | READ | Re-check OPEN saat approve |

### Permissions
| Step | Permission | MFA | Catatan |
|---|---|---|---|
| Submit | `transaksi.submit` | Tidak | Hanya maker_id sendiri |
| Review | `transaksi.review` | Tidak | Reviewer ≠ maker |
| Approve | `transaksi.approve` | Wajib jika Treasury Manager (DEC-026) | Approver ≠ maker AND ≠ reviewer |
| Reject | `transaksi.reject` | Tidak | Reviewer atau approver |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.SUBMIT` | Maker submit ke review |
| `PENEMPATAN.REVIEW` | Reviewer sign-off |
| `PENEMPATAN.APPROVE` | Approver final approve + signature_hash |
| `PENEMPATAN.REJECT` | Reviewer atau approver menolak + reason |
| `PENEMPATAN.EIR_COMPUTED` | EIR solver selesai (post-approve, async) |
| `PENEMPATAN.STAGING_INITIAL` | ECL Stage 1 di-record (post-approve) |

### Acceptance Criteria

```gherkin
Feature: 4-eyes workflow penempatan — submit, review, approve, reject

  Background:
    Given penempatan PNP-2026-00001 dalam status DRAFT
    And maker: USR-001 (Ahmad Fauzi, ROLE-MAKER-TR)
    And reviewer: USR-002 (Budi Santoso, ROLE-APPR-TR)
    And approver: USR-003 (Citra Dewi, ROLE-APPR-TR, Treasury Manager, MFA aktif)
    And USR-001 ≠ USR-002 ≠ USR-003

  # ─── HAPPY PATH: Full 4-eyes workflow selesai → APPROVED_ACTIVE ──────────

  Scenario: Penempatan berhasil melalui 4-eyes lengkap dan EIR di-trigger
    When USR-001 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/submit
      With body: { "comment": "Penempatan deposito BCA sesuai limit portofolio", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-SUBMIT-001
    Then workflow_status berubah ke PENDING_REVIEW
    And audit log: PENEMPATAN.SUBMIT, actor = USR-001
    And notifikasi ke ROLE-APPR-TR: "Penempatan PNP-2026-00001 menunggu review Anda"

    When USR-002 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/review
      With body: { "comment": "Dokumen lengkap, nominal dan tenor sesuai limit", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-REVIEW-001
    Then workflow_status berubah ke PENDING_APPROVAL
    And reviewer_id = USR-002
    And reviewer_signed_at terisi timestamp sekarang
    And reviewer_signature_hash terisi SHA-256 dari payload review
    And audit log: PENEMPATAN.REVIEW, actor = USR-002

    When USR-003 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/approve
      With body: { "comment": "Disetujui sesuai RKAP 2026", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-APPROVE-001
    Then workflow_status berubah ke APPROVED_ACTIVE
    And approver_id = USR-003
    And approver_signed_at terisi timestamp sekarang
    And approver_signature_hash terisi SHA-256 dari payload approve
    And sistem menerbitkan event PenempatanApprovedEvent (consumed by P5-M2: jurnal engine)
    And Asynq job EIR_COMPUTE di-enqueue untuk PNP-2026-00001 (async, DEC-013)
    And ecl.stage_history INSERT atomik:
      | instrumen_id    | INST-DEP-001  |
      | stage_sesudah   | STAGE_1       |
      | trigger_type    | INITIAL_PLACEMENT |
      | penempatan_id   | PNP-2026-00001|
      | status_approval | AUTO          |
    And audit log: PENEMPATAN.APPROVE, PENEMPATAN.STAGING_INITIAL
    And toast sukses ke USR-003: "Penempatan PNP-2026-00001 disetujui. EIR sedang dihitung (lihat progress di /jobs)."
    And after Asynq EIR_COMPUTE selesai: trx.penempatan_deposito.eir_awal terisi, PENEMPATAN.EIR_COMPUTED di-audit

  # ─── HAPPY PATH: Reject oleh reviewer → status kembali DRAFT ─────────────

  Scenario: Reviewer menolak penempatan — status kembali DRAFT, maker dapat edit ulang
    Given PNP-2026-00001 dalam status PENDING_REVIEW
    When USR-002 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/reject
      With body: { "comment": "Nomor referensi bank tidak sesuai konfirmasi tertulis. Mohon perbaiki.", "signature_method": "JWT_STEP_UP" }
    Then workflow_status berubah ke DRAFT
    And reject_reason = "Nomor referensi bank tidak sesuai konfirmasi tertulis. Mohon perbaiki."
    And reviewer_id = NULL (di-reset untuk submit ulang)
    And audit log: PENEMPATAN.REJECT, actor = USR-002, reject_reason tersimpan
    And notifikasi ke USR-001: "Penempatan PNP-2026-00001 ditolak oleh reviewer: Nomor referensi bank tidak sesuai..."
    And USR-001 dapat edit record dan submit ulang

  # ─── ERROR CASE: SoD Violation — reviewer sama dengan maker ──────────────

  Scenario: SoD violation — maker mencoba me-review transaksi sendiri (ERR-SOD-VIOLATION)
    Given PNP-2026-00001 dalam status PENDING_REVIEW, maker_id = USR-001
    When USR-001 mencoba POST /api/v1/transaksi/penempatan/PNP-2026-00001/review
    Then sistem mengembalikan HTTP 403:
      | error.code    | SOD_VIOLATION                                              |
      | error.message | "Anda tidak bisa menjadi reviewer untuk penempatan yang Anda buat sendiri (DEC-017)." |
    And workflow_status tetap PENDING_REVIEW
    And audit log mencatat upaya SOD_VIOLATION

  # ─── ERROR CASE: SoD Violation — approver sama dengan reviewer ───────────

  Scenario: SoD violation — reviewer mencoba menjadi approver
    Given PNP-2026-00001 dalam PENDING_APPROVAL, reviewer_id = USR-002
    When USR-002 mencoba POST /api/v1/transaksi/penempatan/PNP-2026-00001/approve
    Then sistem mengembalikan HTTP 403:
      | error.code    | SOD_VIOLATION                                              |
      | error.message | "Reviewer tidak bisa menjadi approver pada transaksi yang sama (DEC-017)." |

  # ─── ERROR CASE: Periode sudah SOFT_CLOSED saat approve ──────────────────

  Scenario: Periode buku berubah menjadi SOFT_CLOSED sebelum approver menyetujui
    Given PNP-2026-00001 dalam PENDING_APPROVAL
    And periode buku Juni-2026 status berubah ke 'SOFT_CLOSED' setelah review selesai
    When USR-003 mencoba POST /api/v1/transaksi/penempatan/PNP-2026-00001/approve
    Then sistem mengembalikan HTTP 423:
      | error.code    | PERIODE_CLOSED                                              |
      | error.message | "Periode buku Juni-2026 sudah di-close. Penempatan tidak dapat di-approve. Hubungi Finance Controller untuk re-open jika diperlukan." |
    And workflow_status tetap PENDING_APPROVAL

  # ─── ERROR CASE: Reject tanpa komentar cukup panjang ────────────────────

  Scenario: Reject gagal karena alasan penolakan terlalu singkat (< 30 chars)
    Given PNP-2026-00001 dalam PENDING_REVIEW
    When USR-002 mencoba reject dengan comment: "Salah nominal"
    Then sistem mengembalikan HTTP 400:
      | error.code    | VALIDATION_FAILED                         |
      | error.details[0].field | comment                      |
      | error.details[0].rule  | "minLength:30 chars"         |
    And workflow_status tetap PENDING_REVIEW

  # ─── EDGE CASE: Double-submit approve dengan Idempotency-Key sama ─────────

  Scenario: Approver mengirim approve dua kali dengan Idempotency-Key yang sama
    Given approve pertama dengan IK-APPROVE-001 berhasil (APPROVED_ACTIVE)
    When USR-003 mengirim request approve kedua dengan IK-APPROVE-001
    Then sistem mengembalikan HTTP 200 IDEMPOTENCY_REPLAY (response approve asli)
    And EIR compute tidak di-trigger dua kali
    And tidak ada duplikasi di ecl.stage_history

```

### Open Questions — Story 2
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-M1-2a | Apakah notifikasi reviewer setelah submit bersifat email, push notification, atau hanya badge in-app? | Default: **badge in-app** (notification center) + email opsional Phase 2. Konfirmasi ke system-analyst. |
| OQ-M1-2b | Treasury Manager (senior APPR-TR) — apakah ini role terpisah atau ditandai dengan atribut di `sec.user`? | Atribut `is_treasury_manager: boolean` di `sec.user` (DEC-026 mewajibkan MFA untuk Treasury Manager). Flag ke data-modeler. |
| OQ-M1-2c | EIR compute gagal setelah approve (misalnya Newton-Raphson tidak konvergen) — apakah penempatan tetap `APPROVED_ACTIVE` atau di-rollback? | Penempatan tetap `APPROVED_ACTIVE`. EIR compute failure di-flag sebagai job error, ROLE-RISK di-notifikasi, bisa retry manual. Penempatan tidak di-suspend (compliance path diverifikasi `ifrs9-compliance-reviewer`). |
| OQ-M1-2d | Untuk instrumen `FVTPL` — apakah EIR computation di-skip post-approve? | Ya, EIR tidak diperlukan untuk FVTPL. Staging assignment (Stage 1) tetap berjalan untuk FVTPL? Konfirmasi ke `ifrs9-compliance-reviewer` — kemungkinan FVTPL juga tidak butuh ECL staging. |

---

## Story P5-M1-S3 — EIR Preview Sebelum Submit

**Actor**: ROLE-MAKER-TR
**Trigger**: Maker ingin melihat estimasi EIR dan amortization schedule sebelum mengirim ke workflow
**Goal**: Maker mendapatkan preview kalkulasi EIR (efektif interest rate) dan 10 periode amortization schedule berdasarkan data form yang sudah diisi, sebagai alat verifikasi sebelum submit

### Pre-conditions
1. Penempatan dalam status `DRAFT` (record sudah tersimpan)
2. Field wajib untuk kalkulasi terisi: `nominal_idr`, `kupon_persen`, `tenor_bulan`, `biaya_transaksi_idr`, `tanggal_penempatan`
3. User = maker_id (owner of the draft)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | READ | Baca data draft |
| `ecl.eir_service` | COMPUTE (tidak persist) | Preview saja, tidak masuk ke tabel |

### Permissions
| Permission | Catatan |
|---|---|
| `transaksi.read` | Baca record sendiri |
| `eir.preview` | Akses endpoint preview |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.EIR_PREVIEW` | Setiap kali endpoint dipanggil (untuk tracking usage, non-blocking) |

### Acceptance Criteria

```gherkin
Feature: EIR preview sebelum submit penempatan

  Background:
    Given penempatan PNP-2026-00001 (DRAFT) instrumen AC
    And nominal_idr = 5000000000.0000, kupon_persen = 0.05250000
    And tenor_bulan = 12, biaya_transaksi_idr = 0.0000
    And tanggal_penempatan = 2026-06-14

  # ─── HAPPY PATH 1: EIR preview tanpa biaya transaksi ─────────────────────

  Scenario: EIR preview berhasil — deposito tanpa biaya transaksi
    When Maker memanggil GET /api/v1/transaksi/penempatan/PNP-2026-00001/eir-preview
    Then sistem mengembalikan HTTP 200:
      | eir_awal              | 0.05250000 (= kupon rate karena biaya = 0) |
      | carrying_amount_awal  | 5000000000.0000                            |
      | periode_preview       | 10                                         |
      | amortization_schedule | Array 10 row: periode, tanggal_angsuran, angsuran_pokok, angsuran_bunga, carrying_amount |
    And kalkulasi menggunakan Newton-Raphson (DEC-013), tolerance 1e-10
    And hasil diformat dengan 8 desimal untuk rate, 4 desimal untuk IDR amounts
    And audit log: PENEMPATAN.EIR_PREVIEW (non-blocking, fire-and-forget ok)

  # ─── HAPPY PATH 2: EIR preview dengan biaya transaksi ────────────────────

  Scenario: EIR preview dengan biaya transaksi — EIR lebih tinggi dari kupon
    Given penempatan PNP-2026-00002 (DRAFT) dengan:
      | nominal_idr         | 10000000000.0000  |
      | kupon_persen        | 0.06000000        |
      | tenor_bulan         | 24                |
      | biaya_transaksi_idr | 50000000.0000     |
    When Maker memanggil GET /api/v1/transaksi/penempatan/PNP-2026-00002/eir-preview
    Then sistem mengembalikan EIR yang lebih tinggi dari kupon (karena biaya transaksi menaikkan effective cost)
    And EIR computed menggunakan cashflow: CF_0 = -(nominal + biaya), CF_t = kupon payments + principal at maturity
    And amortization_schedule konsisten dengan EIR (sum of interest revenue = nominal × EIR × n)

  # ─── HAPPY PATH 3: Instrumen FVTPL — EIR preview mengembalikan info bahwa EIR tidak relevan

  Scenario: Instrumen FVTPL — EIR preview mengembalikan pesan informatif
    Given penempatan PNP-2026-00003 instrumen INST-OBL-FVTPL-001 (klasifikasi = FVTPL)
    When Maker memanggil GET /api/v1/transaksi/penempatan/PNP-2026-00003/eir-preview
    Then sistem mengembalikan HTTP 200 dengan:
      | eir_awal             | null                                    |
      | info                 | "EIR tidak dihitung untuk instrumen FVTPL. Fair value remeasurement akan di-proses oleh MTM engine (P5-M6)." |
      | amortization_schedule| [] (kosong)                             |

  # ─── ERROR CASE: Field wajib belum terisi — preview tidak bisa dihitung ──

  Scenario: EIR preview gagal karena nominal_idr belum diisi
    Given penempatan PNP-2026-DRAFT dengan nominal_idr = NULL (form belum lengkap)
    When Maker memanggil GET /api/v1/transaksi/penempatan/PNP-2026-DRAFT/eir-preview
    Then sistem mengembalikan HTTP 422:
      | error.code    | ERR-CALC-2010                                                |
      | error.message | "EIR tidak dapat dihitung: nominal_idr wajib diisi terlebih dahulu." |

  # ─── ERROR CASE: User bukan owner draft ──────────────────────────────────

  Scenario: User lain mencoba akses EIR preview milik Maker berbeda
    Given PNP-2026-00001 maker_id = USR-001
    And user saat ini = USR-099 (Maker berbeda, ROLE-MAKER-TR)
    When USR-099 memanggil GET /api/v1/transaksi/penempatan/PNP-2026-00001/eir-preview
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Anda tidak memiliki akses ke penempatan ini."

```

---

## Story P5-M1-S4 — List + Search Penempatan

**Actor**: ROLE-MAKER-TR, ROLE-APPR-TR, ROLE-RISK, ROLE-AUDIT
**Trigger**: User membuka halaman `/trx/penempatan` untuk melihat daftar penempatan yang ada
**Goal**: User dapat melihat daftar penempatan dengan fitur sort multi-kolom, filter komprehensif, cursor pagination, dan export CSV/XLSX sesuai UX rule §1

### Pre-conditions
1. User ter-autentikasi dengan permission `transaksi.read`
2. Data list terfilt berdasarkan `deleted_at IS NULL` (kecuali ROLE-AUDIT dengan `?include_deleted=true`)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | READ | Base table |
| `mst.instrumen` | READ | JOIN untuk nama, klasifikasi, tipe |
| `mst.counterparty` | READ | JOIN untuk nama bank |
| `mst.periode_buku` | READ | JOIN untuk label periode |

### Permissions
| Role | Permission | Catatan |
|---|---|---|
| ROLE-MAKER-TR | `transaksi.read` | Hanya melihat milik unit kerja |
| ROLE-APPR-TR | `transaksi.read` | Semua, termasuk yang pending review/approval |
| ROLE-RISK | `transaksi.read` | Semua, read-only |
| ROLE-AUDIT | `transaksi.read`, `audit_log.read` | + include_deleted, export semua |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.EXPORT` | Setiap kali export dijalankan, dengan filter aktif dan row_count |

### Filter yang tersedia
| Filter | Type | Contoh |
|---|---|---|
| `?q=` | Text search global | Cari kode_transaksi, nomor_referensi_bank |
| `?filter[workflow_status]=` | Enum multi | `DRAFT,PENDING_REVIEW` |
| `?filter[counterparty_bank_id]=` | UUID | Filter bank tertentu |
| `?filter[tipe_instrumen]=` | Enum multi | `DEPOSITO,OBLIGASI` |
| `?filter[klasifikasi_psak71]=` | Enum multi | `AC,FVOCI` |
| `?filter[tanggal_penempatan]=` | `between:2026-01-01,2026-06-30` | Rentang tanggal |
| `?filter[periode_id]=` | UUID | Filter per periode buku |
| `?filter[nominal_idr]=` | `gte:1000000000` | Nominal minimum |

### Sort yang tersedia (multi-column, max 3)
`kode_transaksi`, `tanggal_penempatan`, `nominal_idr`, `workflow_status`, `kupon_persen`, `tanggal_jatuh_tempo`, `created_at`

### Acceptance Criteria

```gherkin
Feature: List dan search penempatan deposito dengan sort, filter, paging, export

  Background:
    Given database memiliki 127 record penempatan dengan berbagai status dan counterparty
    And user ter-autentikasi sebagai ROLE-APPR-TR

  # ─── HAPPY PATH 1: Default list dengan cursor pagination ─────────────────

  Scenario: List penempatan — default sort terbaru, halaman pertama
    When user mengakses GET /api/v1/transaksi/penempatan?limit=50
    Then sistem mengembalikan HTTP 200 dengan:
      | data.length      | 50                             |
      | pagination.hasMore | true                         |
      | pagination.totalEstimate | ~127                 |
      | appliedSort      | [{"col":"created_at","dir":"desc"}] |
    And setiap item mengandung: kode_transaksi, workflow_status, nominal_idr, tanggal_penempatan, nama_counterparty, klasifikasi_psak71

  # ─── HAPPY PATH 2: Filter multi-status + sort custom ─────────────────────

  Scenario: Filter penempatan PENDING_REVIEW dan PENDING_APPROVAL, sort nominal IDR terbesar
    When user mengakses GET /api/v1/transaksi/penempatan?filter[workflow_status]=PENDING_REVIEW,PENDING_APPROVAL&sort=nominal_idr:desc
    Then response hanya berisi record dengan workflow_status IN ('PENDING_REVIEW', 'PENDING_APPROVAL')
    And diurutkan dari nominal_idr terbesar ke terkecil
    And filter chip tampil: "Status: PENDING_REVIEW, PENDING_APPROVAL" dengan tombol clear
    And URL state: `?filter[workflow_status]=PENDING_REVIEW,PENDING_APPROVAL&sort=nominal_idr:desc`

  # ─── HAPPY PATH 3: Export CSV menghormati filter aktif ───────────────────

  Scenario: Export penempatan ke CSV dengan filter periode buku Juni-2026
    Given filter aktif: filter[periode_id] = PERIODE-2026-06
    And total record yang cocok = 45 (< 10k → inline export)
    When user klik Export → CSV
    Then browser mengunduh file langsung: `penempatan-20260614.csv`
    And CSV hanya mengandung 45 record yang cocok filter (bukan semua 127)
    And header row: "Kode Transaksi,Tanggal Penempatan,Nama Bank,Instrumen,Nominal IDR,Kupon (%),Status,Jatuh Tempo"
    And audit log: PENEMPATAN.EXPORT dengan filter aktif + row_count = 45

  # ─── HAPPY PATH 4: ROLE-AUDIT mengakses penempatan yang sudah soft-deleted

  Scenario: ROLE-AUDIT melihat semua termasuk soft-deleted
    Given 3 record penempatan memiliki deleted_at IS NOT NULL (CANCELLED/ditarik)
    And user ter-autentikasi sebagai ROLE-AUDIT
    When ROLE-AUDIT mengakses GET /api/v1/transaksi/penempatan?include_deleted=true
    Then response mengandung ketiga record soft-deleted bersama yang aktif
    And setiap record soft-deleted memiliki badge "Dibatalkan" di UI

  # ─── ERROR CASE: ROLE-MAKER-TR mencoba export dengan include_deleted ──────

  Scenario: ROLE-MAKER-TR tidak bisa akses record soft-deleted
    Given user ter-autentikasi sebagai ROLE-MAKER-TR
    When user mengakses GET /api/v1/transaksi/penempatan?include_deleted=true
    Then parameter include_deleted di-ignore (default false)
    And response tidak mengandung record soft-deleted
    And tidak ada error (silent enforcement)

  # ─── EDGE CASE: Sort by kolom yang tidak diizinkan ───────────────────────

  Scenario: Sort by kolom tidak ada di allowlist — error informatif
    When user mengakses GET /api/v1/transaksi/penempatan?sort=maker_password_hash:asc
    Then sistem mengembalikan HTTP 400:
      | error.code    | VALIDATION_FAILED                             |
      | error.message | "Kolom 'maker_password_hash' tidak diizinkan untuk sort." |

```

---

## Story P5-M1-S4b — Detail View + Edit Draft Penempatan

**Actor**: ROLE-MAKER-TR (owner draft), ROLE-APPR-TR (read-only selama review), ROLE-RISK (read-only), ROLE-AUDIT (read-only)
**Trigger**: User membuka halaman detail penempatan (`/trx/penempatan/{id}`)
**Goal**: Maker dapat melihat detail lengkap dan mengedit record dalam status DRAFT; setelah PENDING_REVIEW, edit tidak diizinkan; Maker dapat menarik (withdraw/cancel) DRAFT

### Pre-conditions
1. Record `trx.penempatan_deposito` ada
2. Edit hanya boleh jika `workflow_status = 'DRAFT'` DAN `maker_id = current_user.id`
3. Withdraw hanya oleh maker_id sendiri pada status DRAFT

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | READ (all) / UPDATE (maker, DRAFT only) | row_version untuk optimistic lock |
| `doc.upload` | READ | Tampilkan daftar dokumen terlampir |

### Permissions
| Action | Permission | Catatan |
|---|---|---|
| View detail | `transaksi.read` | Semua role |
| Edit DRAFT | `transaksi.update` | Hanya maker_id pada DRAFT |
| Withdraw | `transaksi.delete` | Hanya maker_id pada DRAFT |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.UPDATE` | Field diubah pada DRAFT |
| `PENEMPATAN.WITHDRAW` | Maker membatalkan DRAFT |

### Acceptance Criteria

```gherkin
Feature: Detail view dan edit penempatan deposito

  Background:
    Given penempatan PNP-2026-00001 (DRAFT), maker_id = USR-001
    And user ter-autentikasi sebagai USR-001 (ROLE-MAKER-TR)

  # ─── HAPPY PATH 1: Edit field pada DRAFT ─────────────────────────────────

  Scenario: Maker berhasil mengedit nominal dan kupon pada DRAFT
    Given PNP-2026-00001 workflow_status = DRAFT, row_version = 1
    When USR-001 mengirim PATCH /api/v1/transaksi/penempatan/PNP-2026-00001
      With body: { "nominal_idr": 6000000000.0000, "kupon_persen": 0.05500000, "row_version": 1 }
      With Idempotency-Key: IK-EDIT-001
    Then sistem mengembalikan HTTP 200 dengan record yang diupdate
    And row_version bertambah menjadi 2
    And audit log: PENEMPATAN.UPDATE dengan before_jsonb + after_jsonb
    And EIR preview tidak otomatis recompute (user trigger manual via /eir-preview)

  # ─── HAPPY PATH 2: Withdraw DRAFT — soft delete ──────────────────────────

  Scenario: Maker menarik penempatan DRAFT — status menjadi CANCELLED (soft delete)
    Given PNP-2026-00001 workflow_status = DRAFT
    When USR-001 mengirim DELETE /api/v1/transaksi/penempatan/PNP-2026-00001
      With confirmation dialog: "Apakah Anda yakin ingin membatalkan penempatan ini? Tindakan ini tidak dapat dibalik."
    Then sistem set deleted_at = now(), deleted_by = USR-001, workflow_status = CANCELLED
    And record TIDAK dihapus fisik dari database
    And audit log: PENEMPATAN.WITHDRAW, actor = USR-001
    And toast: "Penempatan PNP-2026-00001 berhasil dibatalkan."
    And record tidak muncul di list default (deleted_at IS NOT NULL)

  # ─── ERROR CASE: Edit setelah disubmit — tidak diizinkan ─────────────────

  Scenario: Maker mencoba edit setelah submit ke review — ditolak
    Given PNP-2026-00001 workflow_status = PENDING_REVIEW
    When USR-001 mencoba PATCH /api/v1/transaksi/penempatan/PNP-2026-00001
      With body: { "nominal_idr": 7000000000.0000 }
    Then sistem mengembalikan HTTP 422:
      | error.code    | WORKFLOW_INVALID_TRANSITION                                 |
      | error.message | "Penempatan PNP-2026-00001 tidak dapat diedit karena sudah dalam status PENDING_REVIEW. Minta reviewer untuk reject terlebih dahulu jika perlu perubahan." |

  # ─── ERROR CASE: Optimistic lock conflict — row_version mismatch ──────────

  Scenario: Dua Maker mengedit DRAFT bersamaan — conflict terdeteksi
    Given PNP-2026-00001 row_version = 3 di database
    When Maker mengirim PATCH dengan row_version = 2 (stale)
    Then sistem mengembalikan HTTP 409 CONFLICT:
      | error.code    | CONFLICT                                                    |
      | error.message | "Data sudah diubah oleh user lain. Muat ulang halaman dan coba lagi." |

  # ─── ERROR CASE: User lain mencoba edit DRAFT milik Maker lain ───────────

  Scenario: USR-099 mencoba edit DRAFT yang di-buat USR-001
    Given PNP-2026-00001 maker_id = USR-001
    When USR-099 (ROLE-MAKER-TR, user berbeda) mengirim PATCH
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Anda hanya dapat mengedit penempatan yang Anda buat sendiri."

```

---

## Story P5-M1-S5 — Mature (Auto) / Terminate (Manual)

**Actor**: Asynq daily job (auto-mature) ATAU ROLE-MAKER-TR + ROLE-APPR-TR (manual terminate, **4-eyes penuh — DEC-P5-M1-005**)
**Trigger**: Tanggal hari ini = `tanggal_jatuh_tempo` (auto-mature via Asynq cron job 09:00 WIB) ATAU Maker mengajukan terminasi lebih awal dengan alasan
**Goal**: Penempatan yang sudah jatuh tempo otomatis di-close ke status `MATURED`; terminasi manual membutuhkan **4-eyes workflow** (Maker → Reviewer → Approver, konsisten dengan create) dan alasan jelas; kedua path memancarkan event untuk derecognition di P5-M9

> **DEC-P5-M1-005 (PROPOSED, pending sign-off)**: Terminate workflow = 4-eyes (Option B). Rationale: termination triggers EIR catch-up + ECL derecognition + realized G/L — material financial impact identical in risk tier to origination. 2-eyes (Option A) rejected untuk alasan asimetri kontrol. 6-eyes (Option C) rejected sebagai disproportionate untuk individual transaction. Lihat `docs/decisions/P5-M1-business-decisions.md`.

### Pre-conditions
- **Auto-mature**: `workflow_status = 'APPROVED_ACTIVE'` DAN `tanggal_jatuh_tempo ≤ today`
- **Manual terminate (4-eyes per DEC-P5-M1-005)**:
  - Propose: `workflow_status = 'APPROVED_ACTIVE'`, actor = ROLE-MAKER-TR, `terminate_reason ≥ 30 chars`, dokumen terminasi terlampir
  - Review: `workflow_status = 'TERMINATION_PENDING_REVIEW'`, actor = ROLE-APPR-TR ≠ maker
  - Approve: `workflow_status = 'TERMINATION_PENDING_APPROVAL'`, actor = ROLE-APPR-TR (Treasury Manager) ≠ maker AND ≠ terminate_reviewer
  - Reject: dapat dilakukan oleh reviewer atau approver pada step masing-masing; status kembali ke `APPROVED_ACTIVE`

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `trx.penempatan_deposito` | UPDATE (`workflow_status`, `matured_at` / `terminated_at`, `terminate_reason`) | |
| `ecl.stage_history` | READ | Untuk audit — stage instrumen saat maturity |
| `sys.job` | INSERT | Asynq job progress (rule §3) |

### Permissions
| Action | Permission | Actor | MFA |
|---|---|---|---|
| Auto-mature (Asynq) | system (no human role) | Asynq worker service account | — |
| Propose terminate | `transaksi.terminate` | ROLE-MAKER-TR | Tidak |
| Review terminate | `transaksi.review` (terminate context) | ROLE-APPR-TR (≠ maker) | Tidak wajib |
| Approve terminate | `transaksi.approve` (terminate context) | ROLE-APPR-TR, Treasury Manager (≠ maker AND ≠ terminate_reviewer) | Wajib jika Treasury Manager (DEC-026) |
| Reject terminate | `transaksi.reject` (terminate context) | ROLE-APPR-TR pada step masing-masing | Tidak wajib |

### Audit Events
| Action | Trigger |
|---|---|
| `PENEMPATAN.MATURED` | Auto-mature job sukses |
| `PENEMPATAN.TERMINATE_PROPOSED` | Maker propose terminasi |
| `PENEMPATAN.TERMINATE_REVIEWED` | Reviewer sign-off terminasi (NEW — DEC-P5-M1-005) |
| `PENEMPATAN.TERMINATE_APPROVED` | Approver final approve terminasi |
| `PENEMPATAN.TERMINATE_REJECTED` | Reviewer atau approver tolak terminasi |
| `PENEMPATAN.DERECOGNITION_QUEUED` | Event ke P5-M9 untuk derecognition |

### Acceptance Criteria

```gherkin
Feature: Mature otomatis dan terminasi manual penempatan deposito

  Background:
    Given penempatan PNP-2026-00001 workflow_status = APPROVED_ACTIVE
    And tanggal_jatuh_tempo = 2027-06-14
    And instrumen INST-DEP-001 dalam ecl.stage_history terbaru: Stage 1

  # ─── HAPPY PATH 1: Auto-mature via Asynq job ─────────────────────────────

  Scenario: Asynq maturity job berjalan pada tanggal jatuh tempo — status berubah MATURED
    Given hari ini = 2027-06-14 (= tanggal_jatuh_tempo PNP-2026-00001)
    And Asynq cron job "maturity-checker" berjalan pukul 09:00 WIB
    When job menemukan PNP-2026-00001 memiliki tanggal_jatuh_tempo = today dan status APPROVED_ACTIVE
    Then sistem mengupdate trx.penempatan_deposito:
      | workflow_status  | MATURED        |
      | matured_at       | 2027-06-14     |
    And menerbitkan event PenempatanMaturedEvent (consumed by P5-M9 untuk derecognition)
    And audit log: PENEMPATAN.MATURED, actor = system-worker
    And audit log: PENEMPATAN.DERECOGNITION_QUEUED
    And notifikasi ke ROLE-MAKER-TR + ROLE-RISK: "Penempatan PNP-2026-00001 telah jatuh tempo pada 2027-06-14. Proses derecognition di-queue."
    And sys.job.status = COMPLETED untuk job maturity-checker
    And instrumen INST-DEP-001 workflow_status tidak berubah (instrumen masih aktif, derecognition di P5-M9)

  # ─── HAPPY PATH 2: Manual terminate dengan 4-eyes penuh (DEC-P5-M1-005) ──
  # NOTE: Terminate adalah 4-eyes (Maker → Reviewer → Approver), konsisten dengan create.
  # 2-eyes (Option A) ditolak karena termination = material financial impact (EIR + ECL + G/L).

  Scenario: Maker mengajukan terminasi lebih awal — 4-eyes lengkap berhasil
    Given hari ini = 2026-12-01 (sebelum tanggal_jatuh_tempo)
    And terminate reviewer: USR-002 (ROLE-APPR-TR, ≠ USR-001)
    And terminate approver: USR-003 (ROLE-APPR-TR, Treasury Manager, ≠ USR-001 AND ≠ USR-002)

    When ROLE-MAKER-TR USR-001 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/terminate-request
      With body:
        | terminate_reason     | "Bank counterparty meminta pengembalian dana lebih awal karena restrukturisasi internal. Surat tertanggal 2026-11-30 terlampir." |
        | dokumen_terminasi_id | DOC-TERM-001 |
      With Idempotency-Key: IK-TERM-REQ-001
    Then workflow_status berubah ke TERMINATION_PENDING_REVIEW
    And audit log: PENEMPATAN.TERMINATE_PROPOSED, actor = USR-001
    And notifikasi ke ROLE-APPR-TR: "Proposal terminasi PNP-2026-00001 menunggu review."

    When ROLE-APPR-TR USR-002 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/terminate-review
      With body: { "comment": "Dokumen surat dari bank terlampir dan valid. Alasan termination sesuai prosedur.", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-TERM-REV-001
    Then workflow_status berubah ke TERMINATION_PENDING_APPROVAL
    And terminate_reviewer_id = USR-002
    And terminate_reviewer_signed_at terisi timestamp sekarang
    And terminate_reviewer_signature_hash terisi SHA-256 dari payload review
    And audit log: PENEMPATAN.TERMINATE_REVIEWED, actor = USR-002
    And notifikasi ke ROLE-APPR-TR (Treasury Manager): "Proposal terminasi PNP-2026-00001 menunggu persetujuan akhir."

    When ROLE-APPR-TR USR-003 (Treasury Manager) mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/terminate-approve
      With body: { "comment": "Disetujui sesuai memo Direktur Keuangan No. 123/2026", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-TERM-APPR-001
    Then workflow_status berubah ke TERMINATED
    And terminated_at = 2026-12-01
    And terminate_approver_id = USR-003
    And terminate_approver_signed_at terisi timestamp sekarang
    And terminate_approver_signature_hash terisi SHA-256 dari payload approve
    And SoD diverifikasi server-side: USR-003 ≠ USR-001 (maker) AND USR-003 ≠ USR-002 (reviewer)
    And menerbitkan event PenempatanTerminatedEvent (consumed by P5-M9 untuk derecognition)
    And audit log: PENEMPATAN.TERMINATE_APPROVED, PENEMPATAN.DERECOGNITION_QUEUED
    And toast ke USR-003: "Terminasi PNP-2026-00001 disetujui. Proses derecognition di-queue (P5-M9). EIR catch-up adjustment akan dihitung otomatis."

  # ─── HAPPY PATH 3: Terminate rejected oleh reviewer — kembali ke APPROVED_ACTIVE ──

  Scenario: Reviewer menolak proposal terminasi — instrumen tetap APPROVED_ACTIVE
    Given PNP-2026-00001 dalam TERMINATION_PENDING_REVIEW
    When USR-002 mengirim POST /api/v1/transaksi/penempatan/PNP-2026-00001/terminate-reject
      With body: { "comment": "Dokumen surat dari bank tidak lengkap — cap resmi tidak ada. Mohon lengkapi dokumen pendukung.", "signature_method": "JWT_STEP_UP" }
      With Idempotency-Key: IK-TERM-REJ-001
    Then workflow_status kembali ke APPROVED_ACTIVE
    And terminate_reviewer_id = NULL (di-reset)
    And terminate_reason tetap tersimpan (audit only)
    And audit log: PENEMPATAN.TERMINATE_REJECTED, actor = USR-002, reject_reason tersimpan
    And notifikasi ke USR-001: "Proposal terminasi PNP-2026-00001 ditolak oleh reviewer: Dokumen surat dari bank tidak lengkap..."
    And instrumen dapat di-propose terminate ulang oleh Maker dengan dokumen diperbaiki

  # ─── ERROR CASE: Terminate reason terlalu singkat ────────────────────────

  Scenario: Terminate gagal karena reason < 30 karakter
    When USR-001 mengirim terminate-request dengan terminate_reason = "Pencairan dipercepat"
    Then sistem mengembalikan HTTP 400:
      | error.code    | VALIDATION_FAILED                           |
      | error.details[0].field | terminate_reason             |
      | error.details[0].rule  | "minLength:30 chars"         |

  # ─── ERROR CASE: Terminate pada penempatan yang sudah MATURED ────────────

  Scenario: Terminate penempatan yang sudah matured — tidak valid
    Given PNP-2026-00001 workflow_status = MATURED
    When USR-001 mengirim terminate-request
    Then sistem mengembalikan HTTP 422:
      | error.code    | WORKFLOW_INVALID_TRANSITION                  |
      | error.message | "Penempatan PNP-2026-00001 sudah MATURED dan tidak dapat di-terminate." |

  # ─── EDGE CASE: Asynq job menemukan beberapa penempatan jatuh tempo ──────

  Scenario: Maturity job batch — beberapa penempatan jatuh tempo pada hari yang sama
    Given 5 penempatan memiliki tanggal_jatuh_tempo = 2027-06-14
    And semua dalam status APPROVED_ACTIVE
    When Asynq maturity-checker job berjalan 09:00 WIB 2027-06-14
    Then kelima penempatan di-update ke MATURED secara atomik (per-penempatan transaction, bukan satu big transaction)
    And 5 event PenempatanMaturedEvent diterbitkan
    And sys.job progress: "5 of 5 penempatan diproses" (UX rule §3: JobProgressPanel)
    And jika 1 dari 5 gagal (error akses instrumen), job mencatat partial failure dan lanjutkan 4 sisanya
    And ROLE-RISK di-notifikasi: "Maturity job selesai: 4 berhasil, 1 gagal — lihat detail di /jobs"

```

### Open Questions — Story 5
| ID | Pertanyaan | Status | Resolusi |
|---|---|---|---|
| OQ-M1-5a | Apakah terminasi manual memerlukan 4-eyes penuh (Maker → Reviewer → Approver) atau cukup 2-eyes (Maker → Approver saja)? | **RESOLVED** — ref DEC-P5-M1-005 | **Option B 4-eyes penuh**, konsisten dengan create workflow. Termination memiliki material financial impact (EIR catch-up + ECL derecognition + realized G/L). 2-eyes (Option A) ditolak karena asimetri kontrol. 6-eyes (Option C) ditolak sebagai disproportionate untuk individual transaction. State machine diupdate: tambah state `TERMINATION_PENDING_REVIEW` dan endpoint `terminate-review`. Stakeholder formal sign-off: PENDING (lihat `docs/decisions/P5-M1-business-decisions.md`). |
| OQ-M1-5b | Apakah penempatan yang MATURED bisa di-reopen oleh admin jika ada sengketa settlement? | OPEN | Tidak di-scope P5-M1. Jika diperlukan, butuh RFC dan `REOPEN` workflow. |
| OQ-M1-5c | Maturity job pukul 09:00 WIB — bagaimana jika hari itu adalah hari libur nasional? | OPEN | Job tetap jalan (Asynq tidak tahu libur). Penempatan jatuh tempo pada tanggal kalender tersebut di-close. Tanggal settlement urusan treasury operasional (di luar BLIPS scope). |

---

## Story P5-M1-S6 — Audit Trail View per Penempatan

**Actor**: ROLE-AUDIT (full access), ROLE-RISK (read monitoring), ROLE-AKUN-CTL (read untuk reconciliation)
**Trigger**: Auditor atau Risk Officer membuka timeline audit untuk satu penempatan tertentu
**Goal**: User melihat timeline immutable semua kejadian pada penempatan, termasuk CREATE, SUBMIT, REVIEW, APPROVE, REJECT, MATURE, TERMINATE, EIR_COMPUTED, STAGING_INITIAL, DOCUMENT_ATTACHED, EXPORT — dengan detail `before_jsonb` / `after_jsonb` dan signature hash untuk setiap approval step

### Pre-conditions
1. User ter-autentikasi dengan permission `audit_log.read`
2. `trx.penempatan_deposito` exists (atau ROLE-AUDIT + `?include_deleted=true`)
3. `aud.audit_log` tersedia, append-only, hash-chain valid (DEC-018)

### Data References
| Tabel | Akses | Catatan |
|---|---|---|
| `aud.audit_log` | READ | Filter: `entity_type = 'trx.penempatan_deposito' AND entity_id = ?` |
| `trx.penempatan_deposito` | READ | Context header |
| `sec.user` | READ | JOIN untuk nama actor |

### Permissions
| Role | Permission | Catatan |
|---|---|---|
| ROLE-AUDIT | `audit_log.read`, `transaksi.read` | Termasuk deleted, before/after jsonb |
| ROLE-RISK | `transaksi.read` (partial audit) | Tanpa before/after jsonb PII yang sensitif |
| ROLE-AKUN-CTL | `transaksi.read` | View-only workflow timeline |

### Audit Events
*(Tidak ada audit event untuk read-only action ini, sesuai konvensi)*

### Acceptance Criteria

```gherkin
Feature: Audit trail view untuk penempatan deposito

  Background:
    Given penempatan PNP-2026-00001 sudah melalui lifecycle CREATE → SUBMIT → REVIEW → APPROVE
    And audit_log memiliki 6 event untuk entity_id = penempatan.id:
      | PENEMPATAN.CREATE        | USR-001 | 2026-06-14T09:00:00+07:00 |
      | PENEMPATAN.DOCUMENT_ATTACHED | USR-001 | 2026-06-14T09:01:00+07:00 |
      | PENEMPATAN.SUBMIT        | USR-001 | 2026-06-14T10:00:00+07:00 |
      | PENEMPATAN.REVIEW        | USR-002 | 2026-06-14T14:00:00+07:00 |
      | PENEMPATAN.APPROVE       | USR-003 | 2026-06-14T16:00:00+07:00 |
      | PENEMPATAN.EIR_COMPUTED  | system  | 2026-06-14T16:05:00+07:00 |
    And user ter-autentikasi sebagai ROLE-AUDIT

  # ─── HAPPY PATH 1: Tampilkan full audit timeline ─────────────────────────

  Scenario: ROLE-AUDIT melihat timeline lengkap penempatan
    When ROLE-AUDIT mengakses GET /api/v1/transaksi/penempatan/PNP-2026-00001/audit-trail
    Then sistem mengembalikan HTTP 200 dengan array 6 event terurut tanggal ASC
    And setiap event mengandung:
      | event_id         | UUID                                    |
      | event_time       | ISO 8601 timestamp dengan TZ            |
      | action           | e.g. "PENEMPATAN.APPROVE"               |
      | actor_user_id    | UUID user                               |
      | actor_name       | "Citra Dewi" (JOIN sec.user)            |
      | actor_role       | "ROLE-APPR-TR"                          |
      | before_jsonb     | State sebelum (null untuk CREATE)       |
      | after_jsonb      | State sesudah                           |
      | ip               | IP address actor                        |
      | trace_id         | Trace correlation ID                    |
      | current_hash     | SHA-256 hash chain value                |
    And event PENEMPATAN.APPROVE mengandung signature_hash di after_jsonb
    And UI menampilkan timeline visual (bukan tabel datar): step, ikon, timestamp, actor, badge status

  # ─── HAPPY PATH 2: ROLE-RISK melihat timeline (tanpa before/after jsonb sensitif) ─

  Scenario: ROLE-RISK melihat audit timeline dengan data tereduksi
    Given user ter-autentikasi sebagai ROLE-RISK
    When ROLE-RISK mengakses GET /api/v1/transaksi/penempatan/PNP-2026-00001/audit-trail
    Then sistem mengembalikan HTTP 200 dengan 6 event
    And setiap event TIDAK mengandung `before_jsonb` dan `after_jsonb` (redacted)
    And setiap event mengandung: event_time, action, actor_name, actor_role
    And tidak ada field IP address (privacy — hanya ROLE-AUDIT yang bisa lihat)

  # ─── ERROR CASE: User tanpa permission — forbidden ────────────────────────

  Scenario: ROLE-MAKER-TR mencoba akses audit trail — forbidden
    Given user ter-autentikasi sebagai ROLE-MAKER-TR
    When user mengakses GET /api/v1/transaksi/penempatan/PNP-2026-00001/audit-trail
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Permission audit_log.read dibutuhkan untuk mengakses audit trail."

  # ─── EDGE CASE: Hash chain verify muncul sebagai info flag ───────────────

  Scenario: Audit trail menampilkan status hash chain validity
    Given hash chain valid (semua current_hash cocok dengan sha256(previous_hash || canonical_json))
    When ROLE-AUDIT mengakses audit trail
    Then response mengandung meta field: "hash_chain_valid": true
    And UI menampilkan badge hijau "Hash Chain: Valid"
    And jika hash_chain_valid = false → badge merah "Hash Chain: BROKEN — Hubungi IT Security" + alert ke ROLE-IT-ADMIN

```

---

## Non-Functional Requirements (Semua Story P5-M1)

| NFR | Requirement | Referensi |
|---|---|---|
| Compliance gate | `ifrs9-compliance-reviewer` BLOCKING sebelum merge: EIR trigger + event PENEMPATAN + FVTPL handling | FSD-APP-B §1, DEC-013 |
| Security gate | `security-engineer` BLOCKING: SoD enforcement server-side, audit in-transaction, Idempotency-Key | DEC-017, DEC-018, DEC-021 |
| Decimal precision | Semua nominal via `shopspring/decimal`. No float64. | DEC-016 |
| Idempotency | Wajib di semua mutating endpoint: POST create, submit, review, approve, reject, terminate | DEC-021 |
| Cursor pagination | GET list menggunakan cursor, bukan offset | DEC-022 |
| Soft-delete | `deleted_at` saja. Hard delete dilarang, `aud.audit_log` write in-transaction | DEC-018 |
| EIR async | EIR compute di-queue via Asynq (long-running), progress via JobProgressPanel + SSE (UX §3) | DEC-007, DEC-013 |
| Sort + filter + export | DataTable: multi-col sort, filter, cursor paging, CSV/XLSX export | CLAUDE.md UX §1 |
| Form notifications | Setiap form action: toast sukses spesifik atau error persistent dengan traceId | CLAUDE.md UX §2 |
| Maturity job | Asynq cron 09:00 WIB, batch partial failure allowed, progress di sys.job | CLAUDE.md UX §3 |
| Audit trail | Append-only hash-chain, WORM semantics, retention 10+10 tahun | DEC-018 |
| SoD server-side | maker_id ≠ reviewer_id ≠ approver_id di-enforce service layer, bukan UI saja | DEC-017 |
| POCI handling | klasifikasi = POCI → credit-adjusted EIR (CA-EIR), trigger CA-EIR solver Phase 4.5 | glossary, DEC-POCI-002 |
| FVTPL handling | FVTPL tidak butuh EIR; ECL staging relevance dikonfirmasi oleh `ifrs9-compliance-reviewer` | glossary |

---

## Downstream Module Dependencies dari P5-M1

| Event/Output | Consumer | Modul | Catatan |
|---|---|---|---|
| `PenempatanApprovedEvent` | Jurnal resolver | **P5-M2** | BLOCKING dependency — M2 harus consume event ini untuk generate jurnal PENEMPATAN |
| `eir_awal` computed | EIR amortization schedule | **P5-M2** (jurnal AKRUAL_BUNGA), **P5-M9** (akrual harian) | |
| `ecl.stage_history` initial Stage 1 | ECL calc run | **Phase 4** (ongoing) | Stage assignment ke calc engine |
| `PenempatanMaturedEvent` | Derecognition + jurnal JATUH_TEMPO | **P5-M9** | BLOCKING dependency |
| `PenempatanTerminatedEvent` | Derecognition + jurnal PENJUALAN/terminasi | **P5-M9** | |
| `trx.penempatan_deposito` record | MTM daily job | **P5-M6** | List instrumen APPROVED_ACTIVE untuk daily price update |
| `trx.penempatan_deposito` record | Renewal | **P5-M7** | Penempatan yang bisa di-renew |
| `trx.penempatan_deposito` record | Penjualan/pencairan | **P5-M8** | Penempatan sebagai underlying asset |
| `carrying_amount_awal` | Roll-forward ECL report | **P5-M13**, **P5-M14** (RPT-07, RPT-25) | |

---

## Handoff Checklist

Setelah story set ini di-sign-off:

- [ ] `ifrs9-compliance-reviewer` review BLOCKING: EIR trigger post-approve, FVTPL/POCI handling, initial Stage 1 logic, event contract
- [ ] `security-engineer` review BLOCKING: SoD enforcement, audit in-transaction, Idempotency-Key, MFA scoping
- [ ] `system-analyst` → OpenAPI fragment: semua 9 endpoint (create, submit, review, approve, reject, terminate-request, terminate-approve, eir-preview, audit-trail) + state machine + error codes ERR-VAL-2001..2005, ERR-CALC-2010
- [ ] `system-analyst` → Go interface: `PenempatanService` + `PenempatanApprovedEvent` contract untuk P5-M2
- [ ] `data-modeler` → Migration 000028: `trx.penempatan_deposito` schema (audit cols, NUMERIC precision, indexes, FK, constraints, down.sql wajib)
- [ ] `uiux-designer` → Screen `/trx/penempatan/new` (form), `/trx/penempatan` (DataTable), `/trx/penempatan/{id}` (detail + audit trail) — paralel dengan backend
- [ ] `backend-engineer-go` → HTTP handlers + Gin routing + Asynq maturity cron worker + middleware
- [ ] `ecl-eir-engineer` → EIR compute trigger post-approve (async Asynq job, Newton-Raphson via existing Phase 4 service), POCI CA-EIR path
- [x] OQ-M1-1a RESOLVED — DEC-P5-M1-004 (Option B Informational). ERR-VAL-2002 deferred ke Phase 6. Stakeholder sign-off PENDING (Kepala Treasury, Treasury Manager) — lihat `docs/decisions/P5-M1-business-decisions.md`
- [ ] Konfirmasi OQ-M1-2d (FVTPL ECL staging) ke `ifrs9-compliance-reviewer` sebelum staging post-approve implementasi
- [x] OQ-M1-5a RESOLVED — DEC-P5-M1-005 (Option B 4-eyes). State machine diupdate. Stakeholder sign-off PENDING (Kepala Treasury, ROLE-RISK, ROLE-AKUN-CTL) — lihat `docs/decisions/P5-M1-business-decisions.md`

---

_Dokumen ini adalah source of truth untuk P5-M1. Setiap perubahan scope harus diajukan sebagai delta proposal (bukan edit langsung) dan di-review oleh `ifrs9-compliance-reviewer` jika menyentuh EIR/ECL/klasifikasi, atau `security-engineer` jika menyentuh audit/auth/SoD._
