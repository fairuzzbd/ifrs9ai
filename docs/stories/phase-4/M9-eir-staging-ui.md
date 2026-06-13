# P4-M9 — EIR + Staging UI Screens: User Stories

**Story Set ID**: P4-M9
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 4)
**Status**: DRAFT — compliance review: advisory (bukan BLOCKING gate). Perlu dipastikan UI menampilkan schedule version history lengkap dan SICR evidence per plan §5 (P4-M9 compliance criteria).
**Author**: business-analyst
**Tanggal**: 2026-06-13
**Branch target**: `feature/app-c-eir-staging-ui`

**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (staging timeline), §4 (EIR schedule view), §4.4–4.5 (amendment lifecycle), §6.1 (drift report)
**Linked BRD**: BRD §8.3–8.4, RACI: ROLE-RISK (R), ROLE-AKUN (R/propose), ROLE-ALCO (A), ROLE-AUDIT (I)

**Linked Decision Log**:
- DEC-013 — EIR Newton-Raphson; presisi 8 desimal.
- DEC-016 — NUMERIC(20,4) IDR, NUMERIC(10,8) PD/LGD/EIR. No float64.
- DEC-017 — 4-eyes EIR amendment (AKUN propose → RISK review → ALCO approve). 6-eyes untuk staging override (RISK propose → ALCO approver-1 → ALCO/KOMITE approver-2).
- DEC-018 — Audit trail append-only. `ecl.*` no hard delete.
- DEC-026/DEC-027 — MFA mandatory + step-up MFA untuk ALCO approve.
- DEC-002 — Frontend: Next.js 14+ App Router, TypeScript strict, shadcn/ui, Zustand, TanStack Query.
- UX §1 — DataTable: sort + cursor paging + filter + export wajib di setiap tabel.
- UX §2 — Form notification: sukses/gagal explicit.
- UX §3 — Long-running: JobProgressPanel + SSE.

**Depends on (harus MERGED sebelum frontend coding dimulai)**:
- P4-M1 merged (PR #39) — endpoint `GET /ecl/staging/current/{instrumen_id}`, `GET /ecl/staging/history/{instrumen_id}`, `POST /ecl/staging/evaluate/{instrumen_id}`, override workflow (submit/review/approve/reject), `POST /ecl/staging/dpd-records` CRUD
- P4-M5 merged (PR #60) — endpoint `POST /ecl/eir/compute`, `POST /ecl/eir/generate-schedule`, `GET /ecl/eir/schedule/{instrumen_id}`, `GET /ecl/eir/history/{instrumen_id}`, amendment workflow (propose/review/approve/reject)
- P4-M6 merged (PR #66) — `GET /ecl/eir/amendments`, `POST /ecl/eir/amendments/{id}/cancel`, `GET /ecl/eir/drift-reports`, `GET /ecl/eir/drift-reports/{id}`, `POST /ecl/eir/drift-detection/trigger`
- Phase 3 frontend merged — reuse `<DataTable>`, `<JobProgressPanel>`, `<MFAStepUpModal>`, `lib/notify.ts`, `<DestructiveActionDialog>` dari `web/components/blips/`

**Handoff berikutnya**:
- `system-analyst` — konfirmasi OpenAPI sudah include field `sicr_evidence_jsonb` di staging history response, field `solver_metadata` (iterations, residual) di EIR compute response, field `drift_delta` di amendment list
- `ifrs9-compliance-reviewer` — advisory review: pastikan UI tidak menyembunyikan superseded schedule versions, staging timeline tampilkan semua SICR trigger lengkap beserta evidence
- `qa-engineer` — UAT scripts per story + aksesibilitas (WCAG 2.1 AA minimum)

---

## Komponen Shared (reuse dari Phase 3)

| Komponen | Lokasi | Dipakai oleh |
|---|---|---|
| `<DataTable>` | `web/components/blips/DataTable.tsx` | Semua story |
| `<JobProgressPanel>` | `web/components/blips/JobProgressPanel.tsx` | Story 3 (DPD job), Story 4 (bulk recompute), Story 6 (drift cron) |
| `<MFAStepUpModal>` | `web/components/blips/MFAStepUpModal.tsx` | Story 2 (staging override approve), Story 5 (amendment approve) |
| `<DestructiveActionDialog>` | `web/components/blips/DestructiveActionDialog.tsx` | Story 2 (reject/cancel), Story 5 (cancel), Story 6 (manual drift trigger) |
| `notify` (lib) | `web/lib/notify.ts` | Semua story |
| `useJobProgress` | `web/hooks/useJobProgress.ts` | Story 3, 4, 6 |

---

## Permissions Summary

| Permission | Actors | Stories |
|---|---|---|
| `staging.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN-CTL, ROLE-CFO | S1, S2 (read) |
| `staging.override.propose` | ROLE-RISK | S2 (propose) |
| `staging.override.approve` | ROLE-ALCO, ROLE-KOMITE | S2 (approve, step-up MFA) |
| `dpd_record.create`, `dpd_record.update` | ROLE-AKUN | S3 |
| `dpd_record.read` | ROLE-RISK, ROLE-AUDIT, ROLE-AKUN | S3 (read) |
| `eir.preview` | ROLE-RISK, ROLE-AKUN, ROLE-AUDIT | S4 (read schedule) |
| `eir.compute` | ROLE-RISK | S4 (trigger compute) |
| `eir.amend.propose` | ROLE-AKUN | S5 (propose) |
| `eir.amend.review` | ROLE-RISK | S5 (review) |
| `eir.amend.approve` | ROLE-ALCO | S5 (approve, step-up MFA) |
| `eir.amendment_review.read` | ROLE-RISK, ROLE-ALCO, ROLE-AKUN (milik sendiri), ROLE-AUDIT | S5 (queue) |
| `eir.amendment.cancel` | ROLE-AKUN (maker) | S5 (cancel) |
| `eir.drift_report.read` | ROLE-RISK, ROLE-ALCO, ROLE-AUDIT | S6 (read) |
| `eir.amendment.detect` | ROLE-RISK | S6 (manual trigger) |

---

## Story APP-C-M9-001 — Staging Dashboard per Instrumen

**Actor**: ROLE-RISK (primary), ROLE-AUDIT (read-only)
**Trigger**: User navigasi ke `/ecl/staging/instrumen/[id]`
**Goal**: Melihat stage aktif instrumen beserta alasan SICR/Cure, dan menelusuri seluruh riwayat transisi stage dalam DataTable yang dapat difilter dan diekspor.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3.1–3.3
**Linked API**: `GET /api/v1/ecl/staging/current/{instrumen_id}`, `GET /api/v1/ecl/staging/history/{instrumen_id}`

### Pre-conditions
- Instrumen exists di `mst.instrumen` dengan `deleted_at IS NULL`.
- User ter-autentikasi dan JWT mengandung permission `staging.read`.
- `ecl.stage_history` minimal memiliki satu row untuk instrumen tersebut.

### Post-conditions
- Halaman menampilkan stage aktif (dari response `GET /staging/current/{id}`) dan DataTable history.
- Tidak ada mutasi data.

### Komponen
- Stage banner: colored badge (Stage 1 = green `bg-green-100 text-green-800`, Stage 2 = amber `bg-amber-100 text-amber-800`, Stage 3 = red `bg-red-100 text-red-800`)
- SICR trigger summary card di bawah banner
- `<DataTable>` untuk history (`ecl.stage_history`)
- Export CSV/XLSX

### Acceptance Criteria — APP-C-M9-001

```gherkin
Feature: Staging dashboard per instrumen

  Background:
    Given instrumen "OBL-2026-00001" dengan 5 riwayat stage di ecl.stage_history:
      | tanggal_migrasi | stage_sebelum | stage_sesudah | trigger_type        | sicr_evidence_jsonb         |
      | 2026-01-01      | NULL          | 1             | ORIGINATION         | {}                          |
      | 2026-03-15      | 1             | 2             | RATING_DOWNGRADE    | {notch_change: -2, rating_lama: "idBBB", rating_baru: "idBB"} |
      | 2026-04-01      | 2             | 3             | DPD_THRESHOLD       | {dpd: 91}                   |
      | 2026-07-01      | 3             | 2             | CURE_PARTIAL        | {}                          |
      | 2026-10-01      | 2             | 1             | CURE_FULL           | {}                          |
    And RISK-01 memiliki role ROLE-RISK dan permission staging.read

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: halaman tampil, banner Stage 1
  # ---------------------------------------------------------------
  Scenario: Halaman staging tampil dengan banner stage dan history DataTable
    When RISK-01 navigasi ke /ecl/staging/instrumen/OBL-2026-00001
    Then halaman menampilkan:
      | stage_banner    | "Stage 1 — Performing" dengan latar hijau                               |
      | trigger_summary | "Cure penuh dikonfirmasi 3 periode berturut-turut pada 2026-10-01"       |
      | last_evaluated  | "2026-10-01"                                                             |
    And DataTable history menampilkan 5 row dalam urutan tanggal_migrasi DESC
    And setiap row memiliki kolom: Tanggal, Stage Sebelum, Stage Sesudah, Trigger, Evidence, Approved By
    And row "2026-03-15" menampilkan SICR evidence: "Downgrade 2 notch: idBBB → idBB"

  # ---------------------------------------------------------------
  # Skenario 2 — Filter by date range
  # ---------------------------------------------------------------
  Scenario: Filter riwayat berdasarkan rentang tanggal
    When RISK-01 set filter tanggal_migrasi: 2026-03-01 to 2026-06-30
    Then DataTable hanya menampilkan 2 row (2026-03-15 dan 2026-04-01)
    And URL terupdate dengan query params: ?filter[tanggal_migrasi_from]=2026-03-01&filter[tanggal_migrasi_to]=2026-06-30
    And filter chip tampil di atas tabel dengan tombol "Clear" aktif

  # ---------------------------------------------------------------
  # Skenario 3 — Filter by trigger source
  # ---------------------------------------------------------------
  Scenario: Filter riwayat berdasarkan trigger_type
    When RISK-01 set filter trigger_type = "RATING_DOWNGRADE"
    Then DataTable hanya menampilkan 1 row (2026-03-15 RATING_DOWNGRADE)

  # ---------------------------------------------------------------
  # Skenario 4 — Export CSV history
  # ---------------------------------------------------------------
  Scenario: Export CSV riwayat staging sesuai UX §1
    When RISK-01 klik Export CSV dengan filter tanggal aktif (2026-03-01 to 2026-06-30)
    Then file CSV ter-download dengan nama "staging-history-OBL-2026-00001-20260613.csv"
    And CSV hanya berisi 2 row sesuai filter aktif (bukan semua 5 row)
    And aud.audit_log berisi event "STAGING_HISTORY.EXPORT" dengan:
      | after_jsonb.instrumen_id | OBL-2026-00001 |
      | after_jsonb.row_count    | 2              |
      | after_jsonb.format       | csv            |

  # ---------------------------------------------------------------
  # Skenario 5 — ROLE-AUDIT: read-only, tidak ada tombol aksi
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT membuka halaman — read-only mode
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 navigasi ke /ecl/staging/instrumen/OBL-2026-00001
    Then halaman tampil identik dengan ROLE-RISK
    And tombol "Request Override" tidak tampil (atau disabled dengan tooltip "Aksi tidak tersedia untuk ROLE-AUDIT")

  # ---------------------------------------------------------------
  # Skenario 6 — Stage 3 active: banner merah, peringatan
  # ---------------------------------------------------------------
  Scenario: Instrumen dalam Stage 3 — banner merah dengan alert
    Given instrumen "OBL-2026-00099" dengan stage aktif = 3
    When RISK-01 navigasi ke halaman staging instrumen tersebut
    Then stage banner berwarna merah dengan teks "Stage 3 — Credit-Impaired"
    And alert card tampil: "Instrumen ini dalam status default (Stage 3). PD = 1.0 digunakan untuk ECL. Bunga dihitung dari Net Carrying Amount."
    And tombol "Request Override" tersedia (stage 3 → 2 override)

  # ---------------------------------------------------------------
  # Skenario 7 — Instrumen tidak memiliki riwayat staging
  # ---------------------------------------------------------------
  Scenario: Instrumen baru tanpa stage history
    Given instrumen "DEP-2026-NEW" baru di-approve, belum ada row di ecl.stage_history
    When RISK-01 navigasi ke halaman staging instrumen tersebut
    Then banner menampilkan "Stage belum dievaluasi"
    And DataTable menampilkan empty state: "Belum ada riwayat staging. Jalankan evaluasi pertama."
    And tombol "Evaluasi Staging" tersedia (trigger manual `POST /ecl/staging/evaluate/{id}`)

  # ---------------------------------------------------------------
  # Skenario 8 — Cursor pagination DataTable
  # ---------------------------------------------------------------
  Scenario: Paging riwayat staging — instrumen dengan 60+ history rows
    Given instrumen "PORTFOLIO-IDX-001" memiliki 60 row riwayat
    When RISK-01 membuka halaman (default limit 50)
    Then DataTable menampilkan 50 row pertama
    And footer menampilkan "Page 1 of ~2" dan tombol "Next" aktif
    When RISK-01 klik "Next"
    Then 10 row berikutnya dimuat via GET /ecl/staging/history/PORTFOLIO-IDX-001?cursor=...

  # ---------------------------------------------------------------
  # Skenario 9 — Permission denied: ROLE-MAKER-TR tidak punya staging.read
  # ---------------------------------------------------------------
  Scenario: ROLE-MAKER-TR tidak bisa akses halaman staging
    Given MAKER-01 dengan role ROLE-MAKER-TR
    When MAKER-01 navigasi ke /ecl/staging/instrumen/OBL-2026-00001
    Then redirect ke halaman 403 dengan pesan: "Anda tidak memiliki akses ke halaman ini. Hubungi administrator."
```

### Open Questions — M9-001
- **OQ-M9-001-A**: Field `sicr_evidence_jsonb` di response `GET /staging/history/{id}` — apakah sudah ada di OpenAPI P4-M1 contract atau perlu system-analyst tambahkan? **Flag untuk system-analyst.**
- **OQ-M9-001-B**: Apakah staging dashboard mendukung tombol "Trigger Evaluasi Manual" (memanggil `POST /ecl/staging/evaluate/{id}`) dari halaman ini? Default assume: ya, tersedia untuk ROLE-RISK. Ini akan trigger background job kecil (< 2 detik, tidak perlu JobProgressPanel). Konfirmasi ke orchestrator.

---

## Story APP-C-M9-002 — Staging Override Request (6-Eyes Workflow)

**Actor**: ROLE-RISK (propose), ROLE-ALCO + ROLE-KOMITE (approve, 6-eyes)
**Trigger**: ROLE-RISK klik "Request Override" di halaman staging instrumen → navigasi ke `/ecl/staging/override/new`, atau dari queue `/ecl/staging/override`
**Goal**: ROLE-RISK membuat permintaan override stage manual (mis. Stage 1 → Stage 2 berdasarkan pertimbangan kualitatif). Request melewati 6-eyes workflow (RISK propose → ALCO approver-1 → ALCO/KOMITE approver-2). Setiap approve wajib step-up MFA. Approver tidak boleh sama dengan proposer (SoD).
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3.4 (manual override), DEC-017 (6-eyes)
**Linked API**: `POST /ecl/staging/override`, `GET /ecl/staging/override`, `POST /ecl/staging/override/{id}/review`, `POST /ecl/staging/override/{id}/approve`, `POST /ecl/staging/override/{id}/reject`

### Pre-conditions
- Instrumen exists dan berstatus AKTIF.
- Tidak ada override dalam status `PENDING_REVIEW` atau `PENDING_APPROVAL` untuk instrumen yang sama.
- User memiliki permission `staging.override.propose`.
- Instrumen bukan FVTPL atau FVOCI equity (tidak ber-ECL).

### Post-conditions (setelah full approval)
- Baris baru di `ecl.stage_history` dengan `trigger_type = 'MANUAL_OVERRIDE'` dan `status_approval = 'APPROVED'`.
- Notifikasi ke ROLE-RISK bahwa override disetujui.
- Audit event `STAGING.OVERRIDE_APPROVED` ditulis in-transaction.

### Komponen
- Form override baru: instrumen picker, target stage `<Select>`, reason textarea (min 20 char), expiry periode picker
- `<DataTable>` untuk queue override pending
- `<MFAStepUpModal>` untuk setiap step approve
- `<DestructiveActionDialog>` untuk reject

### Acceptance Criteria — APP-C-M9-002

```gherkin
Feature: Staging override request — 6-eyes workflow

  Background:
    Given instrumen "DEP-2026-00001" dengan stage aktif = Stage 1
    And RISK-01 memiliki role ROLE-RISK dan permission staging.override.propose
    And ALCO-01 dan ALCO-02 memiliki permission staging.override.approve (ROLE-ALCO)
    And tidak ada override aktif untuk "DEP-2026-00001"

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: form submit proposal berhasil
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK submit proposal override Stage 1 → Stage 2
    When RISK-01 akses /ecl/staging/override/new
    And RISK-01 mengisi form:
      | instrumen_id     | DEP-2026-00001                                              |
      | target_stage     | 2                                                           |
      | reason           | "Counterparty menunjukkan tanda-tanda kesulitan keuangan meski rating belum turun resmi." |
      | expiry_periode   | DESEMBER-2026                                               |
    And RISK-01 klik "Submit Proposal"
    Then response dari POST /api/v1/ecl/staging/override dengan 201
    And toast sukses: "Proposal override stage DEP-2026-00001 berhasil disubmit. Menunggu review ALCO."
    And form di-reset dan user di-redirect ke /ecl/staging/override
    And ALCO-01 dan ALCO-02 menerima notifikasi: "Proposal override staging baru menunggu persetujuan Anda."

  # ---------------------------------------------------------------
  # Skenario 2 — Queue pending: DataTable dengan filter dan sort
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK membuka queue override — DataTable UX §1
    Given 3 proposal override dalam status PENDING_REVIEW dan 2 dalam PENDING_APPROVAL
    When RISK-01 akses /ecl/staging/override
    Then DataTable menampilkan 5 row dengan kolom:
      | ID Override, Instrumen, Stage Saat Ini, Target Stage, Reason (truncated), Proposer, Status, Tanggal, Aksi |
    And default filter: semua status pending (PENDING_REVIEW + PENDING_APPROVAL)
    And default sort: created_at DESC
    And filter chip tersedia: Status, Target Stage, Instrumen, Tanggal
    And tombol Export CSV tersedia di action bar

  # ---------------------------------------------------------------
  # Skenario 3 — Approval step 1 (ALCO-01) dengan step-up MFA
  # ---------------------------------------------------------------
  Scenario: ALCO-01 approve step-1 — MFA step-up modal wajib tampil
    Given proposal "OVR-001" dalam status PENDING_REVIEW
    And ALCO-01 login dengan mfa_verified = true tapi 10 menit lalu (lebih dari 5 menit threshold DEC-027)
    When ALCO-01 klik "Approve" pada proposal OVR-001 di queue
    Then modal konfirmasi muncul terlebih dahulu:
      | judul   | "Approve Override Stage — DEP-2026-00001"                  |
      | detail  | "Proposal: Stage 1 → Stage 2. Alasan: [reason]. Periksa justifikasi sebelum melanjutkan." |
      | tombol  | "Lanjutkan ke MFA" | "Batal"                              |
    When ALCO-01 klik "Lanjutkan ke MFA"
    Then <MFAStepUpModal> tampil dengan instruksi verifikasi TOTP/WebAuthn
    When ALCO-01 submit kode MFA valid
    Then POST /api/v1/ecl/staging/override/OVR-001/review dikirim dengan X-Step-Up-Token
    And response 200 dengan status "PENDING_APPROVAL"
    And toast sukses (ALCO-01): "Approval step 1 berhasil. Menunggu approval step 2."
    And aud.audit_log berisi "STAGING.OVERRIDE_REVIEW_APPROVED_STEP_1"

  # ---------------------------------------------------------------
  # Skenario 4 — Approval step 2 (ALCO-02) — override final
  # ---------------------------------------------------------------
  Scenario: ALCO-02 approve step-2 — override dikonfirmasi, stage diubah
    Given proposal "OVR-001" dalam status PENDING_APPROVAL (step-1 sudah approve)
    When ALCO-02 approve dengan MFA step-up valid
    Then POST /api/v1/ecl/staging/override/OVR-001/approve sukses
    And ecl.stage_history berisi row baru:
      | instrumen_id  | DEP-2026-00001     |
      | stage_sebelum | 1                  |
      | stage_sesudah | 2                  |
      | trigger_type  | MANUAL_OVERRIDE    |
      | status_approval | APPROVED         |
    And aud.audit_log berisi "STAGING.OVERRIDE_APPROVED" dengan full sign-off chain
    And toast sukses (ALCO-02): "Override stage DEP-2026-00001 disetujui. Stage diubah dari 1 ke 2."
    And halaman staging instrumen DEP-2026-00001 otomatis terupdate: banner amber "Stage 2"

  # ---------------------------------------------------------------
  # Skenario 5 — SoD violation: RISK-01 tidak boleh approve proposal sendiri
  # ---------------------------------------------------------------
  Scenario: SoD violation — proposer tidak boleh menjadi approver
    Given proposal "OVR-002" dengan proposer = RISK-01
    And RISK-01 juga memiliki permission staging.override.approve (hipotesis)
    When RISK-01 mencoba approve OVR-002
    Then response 403 dengan error.code = "SOD_VIOLATION"
    And error.message: "Proposer tidak boleh menjadi approver untuk override yang sama."
    And tombol "Approve" di-disable di UI untuk row yang proposer-nya adalah user yang login

  # ---------------------------------------------------------------
  # Skenario 6 — Reject dengan komentar (dari approver)
  # ---------------------------------------------------------------
  Scenario: ALCO-01 reject proposal override
    When ALCO-01 klik "Tolak" pada proposal OVR-001
    Then <DestructiveActionDialog> muncul:
      | judul   | "Tolak Proposal Override?"                                         |
      | input   | Textarea "Alasan penolakan (wajib, min 20 karakter)"               |
      | tombol  | "Tolak Proposal" (merah) | "Batal"                               |
    When ALCO-01 mengisi alasan dan konfirmasi
    Then POST /api/v1/ecl/staging/override/OVR-001/reject dikirim
    And status proposal berubah ke "REJECTED"
    And notifikasi ke RISK-01: "Proposal override OVR-001 ditolak. Alasan: [komentar ALCO-01]"
    And toast sukses (ALCO-01): "Proposal OVR-001 berhasil ditolak."

  # ---------------------------------------------------------------
  # Skenario 7 — Validasi form: reason terlalu pendek
  # ---------------------------------------------------------------
  Scenario: Submit proposal — reason kurang dari 20 karakter
    When RISK-01 mengisi reason = "Perlu override"
    And klik Submit
    Then form tidak di-submit
    And field reason di-highlight merah dengan pesan inline: "Alasan harus minimal 20 karakter (saat ini: 14 karakter)."
    And submit button tetap aktif setelah user memperbaiki field

  # ---------------------------------------------------------------
  # Skenario 8 — Override duplikat: instrumen sudah punya proposal aktif
  # ---------------------------------------------------------------
  Scenario: Gagal submit — instrumen sudah punya proposal aktif
    Given instrumen "DEP-2026-00001" sudah punya proposal dengan status PENDING_REVIEW
    When RISK-01 submit proposal baru untuk instrumen yang sama
    Then response 422 dengan error.code = "STAGING_OVERRIDE_ACTIVE_EXISTS"
    And toast error persistent: "Instrumen DEP-2026-00001 sudah punya proposal override aktif. Selesaikan atau batalkan yang ada terlebih dahulu."

  # ---------------------------------------------------------------
  # Skenario 9 — Expiry periode: field tersedia dan di-validate
  # ---------------------------------------------------------------
  Scenario: Expiry periode wajib dipilih dan harus di masa depan
    When RISK-01 pilih expiry_periode = "MEI-2026" (periode yang sudah lewat dari tanggal hari ini 2026-06-13)
    And klik Submit
    Then form menampilkan error inline pada field expiry_periode:
      "Expiry periode harus di masa mendatang (MEI-2026 sudah lewat)."
```

### Open Questions — M9-002
- **OQ-M9-002-A**: 6-eyes untuk staging override: siapa approver-2 — harus ROLE-ALCO + ROLE-KOMITE, atau boleh dua ROLE-ALCO berbeda? Dari M1 story plan disebutkan "ROLE-ALCO + ROLE-KOMITE". **Perlu konfirmasi ke BRD §8.3 dan FSD-APP-C §3.4 sebelum system-analyst menulis state machine.** Default assume: approver-1 = ROLE-ALCO, approver-2 = ROLE-ALCO atau ROLE-KOMITE (keduanya valid).
- **OQ-M9-002-B**: Apakah override yang sudah disetujui bisa di-expire otomatis setelah `expiry_periode` lewat (kembali ke stage sebelumnya), atau expire hanya sebagai informasi saja? Default assume: expire = informasi saja, tidak auto-revert. Konfirmasi ke ROLE-RISK + ROLE-ALCO.

---

## Story APP-C-M9-003 — DPD Record Entry

**Actor**: ROLE-AKUN
**Trigger**: ROLE-AKUN navigasi ke `/ecl/staging/dpd` untuk melihat daftar DPD per instrumen, atau membuka form entry DPD untuk periode tertentu
**Goal**: ROLE-AKUN memasukkan dan mengedit nilai DPD per instrumen per periode (sebagai workaround manual sampai APP-B live, sesuai GAP-DPD di M1). Setiap penyimpanan DPD baru memicu background re-staging evaluation untuk instrumen tersebut, dengan notifikasi saat selesai.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3.1 (DPD trigger), GAP-DPD dari M1 plan
**Linked API**: `GET /ecl/staging/dpd-records`, `POST /ecl/staging/dpd-records`, `PUT /ecl/staging/dpd-records/{id}`, `DELETE /ecl/staging/dpd-records/{id}` (soft-delete)

### Pre-conditions
- Instrumen target exists dan `deleted_at IS NULL`.
- User memiliki permission `dpd_record.create` (ROLE-AKUN).
- Tidak ada duplikat `(instrumen_id, periode_id)` di `trx.dpd_record`.

### Post-conditions
- Baris baru di `trx.dpd_record` tersimpan.
- Background job re-staging evaluation di-dispatch untuk instrumen tersebut.
- Notifikasi ke ROLE-AKUN saat re-staging selesai (success/failure).
- Audit event `DPD_RECORD.CREATE` atau `DPD_RECORD.UPDATE` ditulis in-transaction.

### Komponen
- `<DataTable>` untuk list DPD records (instrumen + DPD terkini per periode)
- Form inline atau drawer: instrumen picker, periode picker, nilai DPD (INT, min 0), keterangan
- `<JobProgressPanel>` untuk re-staging background job (UX §3 — expected < 2 detik, tapi tetap tampilkan spinner + complete toast)
- `<DestructiveActionDialog>` untuk soft-delete

### Acceptance Criteria — APP-C-M9-003

```gherkin
Feature: DPD record entry dan re-staging trigger

  Background:
    Given AKUN-01 memiliki role ROLE-AKUN, permission dpd_record.create + dpd_record.update
    And instrumen "OBL-2026-00005" berstatus AKTIF, klasifikasi_psak71 = "AC", Stage 1
    And mst.periode_buku "JUNI-2026" berstatus OPEN

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: entry DPD baru, re-staging triggered
  # ---------------------------------------------------------------
  Scenario: AKUN-01 input DPD 35 hari untuk instrumen OBL-2026-00005
    When AKUN-01 akses /ecl/staging/dpd
    And klik "Tambah Record DPD"
    And mengisi form:
      | instrumen_id | OBL-2026-00005 |
      | periode_id   | JUNI-2026      |
      | dpd          | 35             |
      | keterangan   | "Pembayaran bunga terlambat, konfirmasi Bank X"     |
    And klik "Simpan"
    Then POST /api/v1/ecl/staging/dpd-records dikirim dengan Idempotency-Key
    And response 201 dengan data record DPD baru
    And toast sukses: "Record DPD OBL-2026-00005 periode JUNI-2026 berhasil disimpan. Mengevaluasi ulang staging..."
    And submit button di-disable selama proses (UX §2 pending state)
    And background job re-staging di-dispatch untuk OBL-2026-00005
    And JobProgressPanel mini muncul: "Mengevaluasi staging OBL-2026-00005..."
    And aud.audit_log berisi "DPD_RECORD.CREATE"

  # ---------------------------------------------------------------
  # Skenario 2 — Re-staging menghasilkan SICR: Stage 1 → Stage 2
  # ---------------------------------------------------------------
  Scenario: DPD 35 ≥ 30 hari SICR threshold — auto Stage 1 → Stage 2 setelah re-eval
    Given DPD record tersimpan dengan dpd = 35 untuk OBL-2026-00005
    When background job re-staging selesai
    Then ecl.stage_history berisi row baru:
      | instrumen_id  | OBL-2026-00005  |
      | stage_sebelum | 1               |
      | stage_sesudah | 2               |
      | trigger_type  | DPD_THRESHOLD   |
      | dpd           | 35              |
    And toast sukses di UI AKUN-01: "Stage OBL-2026-00005 berubah: Stage 1 → Stage 2 (DPD 35 hari ≥ 30 hari). Lihat detail."
    And link "Lihat detail" ke /ecl/staging/instrumen/OBL-2026-00005

  # ---------------------------------------------------------------
  # Skenario 3 — DPD di bawah threshold: tidak ada perubahan stage
  # ---------------------------------------------------------------
  Scenario: DPD 15 hari — tidak memicu SICR, stage tetap
    When AKUN-01 input DPD = 15 untuk instrumen lain "DEP-2026-00010"
    And background job re-staging selesai
    Then tidak ada row baru di ecl.stage_history untuk DEP-2026-00010
    And toast info: "Staging DEP-2026-00010 tidak berubah (DPD 15 hari < threshold 30 hari). Stage tetap: 1."

  # ---------------------------------------------------------------
  # Skenario 4 — List DPD: DataTable dengan filter dan sort
  # ---------------------------------------------------------------
  Scenario: AKUN-01 melihat list DPD records — DataTable UX §1
    Given 50 DPD records tersimpan untuk berbagai instrumen dan periode
    When AKUN-01 akses /ecl/staging/dpd
    Then DataTable menampilkan 50 records dengan kolom:
      | Instrumen, Kode, Periode, DPD (hari), Keterangan, Stage Saat Ini, Dibuat Oleh, Tanggal |
    And kolom DPD dapat di-sort (klik header)
    And filter tersedia: Instrumen (text search), Periode (dropdown), Stage (select 1/2/3)
    And sort default: tanggal DESC
    And Export CSV tersedia

  # ---------------------------------------------------------------
  # Skenario 5 — Edit DPD: nilai diubah, re-staging di-trigger ulang
  # ---------------------------------------------------------------
  Scenario: AKUN-01 edit DPD dari 35 → 20 (koreksi data), stage harus di-reevaluasi
    Given DPD record "DPD-001" dengan dpd = 35 untuk OBL-2026-00005, stage saat ini = 2
    When AKUN-01 edit dpd = 20
    And klik "Simpan Perubahan"
    Then PUT /api/v1/ecl/staging/dpd-records/DPD-001 dikirim
    And background job re-staging di-dispatch
    And toast: "Record DPD diperbarui. Mengevaluasi ulang staging..."
    And setelah re-staging:
      ecl.stage_history row baru dengan:
      | stage_sebelum | 2             |
      | stage_sesudah | 1             |
      | trigger_type  | DPD_CORRECTION |
    And toast: "Stage OBL-2026-00005 kembali ke Stage 1 (DPD dikoreksi menjadi 20 hari < threshold)."
    And aud.audit_log berisi "DPD_RECORD.UPDATE" dengan before_jsonb.dpd = 35, after_jsonb.dpd = 20

  # ---------------------------------------------------------------
  # Skenario 6 — Validasi: DPD negatif tidak diizinkan
  # ---------------------------------------------------------------
  Scenario: Validasi — nilai DPD negatif ditolak
    When AKUN-01 mengisi dpd = -5
    And klik "Simpan"
    Then form tidak di-submit
    And inline error pada field dpd: "Nilai DPD tidak boleh negatif."

  # ---------------------------------------------------------------
  # Skenario 7 — Duplikat record (instrumen + periode yang sama)
  # ---------------------------------------------------------------
  Scenario: Gagal input — sudah ada record DPD untuk (instrumen, periode) yang sama
    Given DPD record untuk (OBL-2026-00005, JUNI-2026) sudah ada
    When AKUN-01 submit DPD record baru untuk kombinasi yang sama
    Then response 422 dengan error.code = "DPD_RECORD_DUPLICATE"
    And toast error: "Record DPD untuk OBL-2026-00005 periode JUNI-2026 sudah ada. Gunakan fungsi Edit."

  # ---------------------------------------------------------------
  # Skenario 8 — Soft-delete: record dihapus, re-staging di-trigger
  # ---------------------------------------------------------------
  Scenario: AKUN-01 hapus record DPD — soft delete + re-staging
    When AKUN-01 klik ikon hapus pada DPD-001
    Then <DestructiveActionDialog> muncul: "Hapus record DPD ini? Re-staging akan dijalankan otomatis."
    When AKUN-01 konfirmasi
    Then record di-soft-delete (deleted_at di-set)
    And background re-staging di-dispatch
    And aud.audit_log berisi "DPD_RECORD.DELETE"
```

### Open Questions — M9-003
- **OQ-M9-003-A**: DPD trigger re-staging: apakah re-staging background job untuk DPD entry berjalan < 2 detik (single instrument)? Jika iya, cukup spinner inline + toast selesai. Jika ada overhead (karena juga trigger EAD recalc), perlu full JobProgressPanel. Konfirmasi ke `ecl-eir-engineer`.
- **OQ-M9-003-B**: Pada Skenario 5 (edit DPD lalu re-staging kembali ke Stage 1), apakah stage-down dari Stage 2 → 1 via DPD correction diperlakukan sebagai cure atau sebagai correction? Default assume: row baru di `ecl.stage_history` dengan `trigger_type = 'DPD_CORRECTION'` (bukan CURE, karena belum memenuhi 3 periode berturut-turut). Konfirmasi ke `ifrs9-compliance-reviewer`.

---

## Story APP-C-M9-004 — EIR Compute + Amortisasi Schedule View

**Actor**: ROLE-RISK (primary; juga trigger compute), ROLE-AKUN (read schedule), ROLE-AUDIT (read-only)
**Trigger**: User navigasi ke `/ecl/eir/instrumen/[id]`
**Goal**: Melihat EIR aktif instrumen, jadwal amortisasi per periode dalam DataTable (dengan version selector dropdown), metadata konvergensi solver, dan tombol trigger compute/recompute.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.1–4.3
**Linked API**: `GET /api/v1/ecl/eir/schedule/{instrumen_id}`, `GET /api/v1/ecl/eir/history/{instrumen_id}`, `POST /api/v1/ecl/eir/compute`

### Pre-conditions
- Instrumen exists, `klasifikasi_psak71 IN ('AC', 'FVOCI')`, `eir_method_flag = TRUE`.
- User ter-autentikasi dengan permission `eir.preview`.

### Post-conditions (setelah compute trigger)
- `mst.instrumen.eir_awal` ter-update jika belum ada EIR.
- Rows baru di `ecl.eir_amortization_schedule` dengan `recomputed_from_seq IS NULL`.
- Halaman di-refresh dengan schedule version terbaru.

### Komponen
- EIR summary card (EIR aktif, tanggal compute, day-count convention)
- Solver metadata side panel (iterations, final residual, convergence status)
- Version selector dropdown (active vs superseded schedules berdasarkan `recomputed_from_seq`)
- `<DataTable>` schedule per periode
- Tombol "Compute EIR" (trigger `POST /ecl/eir/compute`) dengan spinner + toast

### Acceptance Criteria — APP-C-M9-004

```gherkin
Feature: EIR compute dan amortisasi schedule view

  Background:
    Given instrumen "OBL-2026-00001" dengan:
      | klasifikasi_psak71      | AC               |
      | eir_method_flag         | true             |
      | eir_awal                | 0.04250000       |
      | day_count_convention    | ACT/365          |
      | kupon                   | 0.04000000       |
      | nominal                 | 10000000000.0000 |
      | tanggal_jatuh_tempo     | 2031-06-01       |
    And 60 rows di ecl.eir_amortization_schedule (schedule asli, recomputed_from_seq IS NULL)
    And RISK-01 memiliki role ROLE-RISK dan permission eir.preview + eir.compute

  # ---------------------------------------------------------------
  # Skenario 1 — Happy path: schedule tampil, version selector ada
  # ---------------------------------------------------------------
  Scenario: Halaman EIR tampil dengan schedule aktif dan version selector
    When RISK-01 navigasi ke /ecl/eir/instrumen/OBL-2026-00001
    Then EIR summary card menampilkan:
      | EIR Aktif      | "4.25000000%" (8 desimal — DEC-016)                     |
      | Dihitung pada  | {tanggal compute terakhir}                              |
      | Day-count      | ACT/365                                                 |
      | Klasifikasi    | AC                                                      |
    And version selector dropdown menampilkan: "Versi Aktif (2026-01-15)" sebagai default
    And DataTable schedule menampilkan 60 row dengan kolom:
      | Periode Seq, Tanggal Mulai, Opening Carrying, Pendapatan Bunga EIR, Amortisasi P/D, Pelunasan Pokok, Closing Carrying, EIR Periode, Status Posting |
    And semua amount IDR diformat dengan 4 desimal (NUMERIC(20,4) — DEC-016)
    And kolom "EIR Periode" diformat dengan 8 desimal (NUMERIC(10,8))

  # ---------------------------------------------------------------
  # Skenario 2 — Version selector: tampilkan semua versi (termasuk superseded)
  # ---------------------------------------------------------------
  Scenario: Version selector menampilkan semua versi schedule termasuk superseded
    Given amandemen telah di-approve pada 2026-04-01, menghasilkan schedule baru
    And rows lama di-mark recomputed_from_seq = 24 (mulai periode 24 diganti)
    When RISK-01 buka version selector dropdown
    Then dropdown menampilkan:
      | Opsi 1 | "Versi Aktif (mulai 2026-04-01)" — default dipilih                  |
      | Opsi 2 | "Versi v1 (2026-01-15 — 2026-04-01, superseded)" — dengan badge abu  |
    When RISK-01 pilih "Versi v1 (superseded)"
    Then DataTable terupdate menampilkan schedule lama (rows dengan recomputed_from_seq IS NOT NULL)
    And warning banner tampil: "Anda melihat schedule yang sudah digantikan (superseded). Versi aktif tersedia di dropdown."

  # ---------------------------------------------------------------
  # Skenario 3 — Trigger compute EIR manual
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK trigger compute EIR untuk instrumen baru (eir_awal IS NULL)
    Given instrumen "DEP-2026-00050" dengan eir_awal IS NULL dan eir_method_flag = true
    When RISK-01 akses halaman EIR instrumen tersebut
    Then halaman menampilkan alert: "EIR belum dihitung untuk instrumen ini." dan tombol "Hitung EIR"
    When RISK-01 klik "Hitung EIR"
    Then tombol berubah menjadi spinner "Menghitung..." (UX §2 pending state)
    And POST /api/v1/ecl/eir/compute dikirim dengan body { instrumen_id: "DEP-2026-00050" }
    And setelah response sukses:
      mst.instrumen.eir_awal terisi
      ecl.eir_amortization_schedule rows baru dibuat
      DataTable schedule ter-refresh
      toast sukses: "EIR berhasil dihitung: 4.32000000%. 60 baris schedule di-generate."
    And solver metadata side panel tampil dengan:
      | Iterasi      | {N} iterasi                |
      | Residual     | {nilai < 1e-10}            |
      | Status       | "Konvergen"                |
      | Presisi      | "HALF_EVEN, 8 desimal"     |

  # ---------------------------------------------------------------
  # Skenario 4 — EIR non-convergence: error eksplisit (DEC-013)
  # ---------------------------------------------------------------
  Scenario: EIR solver tidak konvergen — error eksplisit (DEC-013 fail-safe)
    Given instrumen "OBL-ZERO-COUPON-001" dengan cashflow edge case (zero coupon)
    When RISK-01 trigger compute EIR
    Then response 422 dengan error.code = "EIR_SOLVER_NON_CONVERGENT"
    And toast error persistent: "EIR solver tidak konvergen setelah 100 iterasi untuk OBL-ZERO-COUPON-001. Periksa input cashflow. Trace: [traceId]"
    And solver metadata panel tampil dengan:
      | Status   | "Tidak Konvergen"           |
      | Iterasi  | 100 (max reached)           |
      | Residual | {nilai > 1e-10}             |
    And mst.instrumen.eir_awal TIDAK diubah (tetap NULL)
    And ecl.eir_amortization_schedule TIDAK ada rows baru

  # ---------------------------------------------------------------
  # Skenario 5 — FVTPL instrumen: tombol compute tidak tersedia
  # ---------------------------------------------------------------
  Scenario: FVTPL instrumen — halaman EIR menampilkan pesan tidak applicable
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When RISK-01 akses /ecl/eir/instrumen/SHM-2026-00001
    Then halaman menampilkan info banner: "EIR tidak berlaku untuk instrumen FVTPL."
    And tombol "Hitung EIR" tidak tersedia
    And DataTable schedule kosong

  # ---------------------------------------------------------------
  # Skenario 6 — Cursor pagination: schedule > 50 periode
  # ---------------------------------------------------------------
  Scenario: Paging schedule DataTable — instrumen jangka panjang 120 periode
    Given instrumen "OBL-2026-LONGTERM" dengan 120 rows schedule
    When RISK-01 membuka halaman (default limit 50)
    Then DataTable menampilkan 50 row pertama
    And footer: "Page 1 of ~3, 120 total" dengan Prev/Next
    When RISK-01 klik Next
    Then 50 row berikutnya dimuat

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: read-only, tidak ada tombol compute
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT tidak dapat trigger compute
    Given AUDIT-01 memiliki role ROLE-AUDIT
    When AUDIT-01 akses halaman EIR instrumen OBL-2026-00001
    Then halaman tampil lengkap (schedule, version selector, solver metadata)
    And tombol "Hitung EIR" tidak tampil / disabled

  # ---------------------------------------------------------------
  # Skenario 8 — Export schedule
  # ---------------------------------------------------------------
  Scenario: Export XLSX schedule aktif
    When RISK-01 klik Export XLSX
    Then file "eir-schedule-OBL-2026-00001-versi-aktif-20260613.xlsx" ter-download
    And XLSX berisi: header bold + freeze baris pertama
    And kolom IDR diformat #,##0.0000
    And kolom EIR diformat 8 desimal
    And footer sheet: "Diekspor pada 2026-06-13 oleh RISK-01 | Filter: Versi Aktif"
    And aud.audit_log berisi "EIR_SCHEDULE.EXPORT"
```

### Open Questions — M9-004
- **OQ-M9-004-A**: Solver metadata (`iterations`, `residual`, `convergence_status`) — apakah sudah ada di response `POST /ecl/eir/compute` di OpenAPI P4-M5? Jika belum, perlu system-analyst tambahkan field `solver_metadata` ke response contract.
- **OQ-M9-004-B**: Bulk EIR recompute (untuk semua instrumen sekaligus) — apakah tombol ini ada di halaman ini atau di halaman terpisah? Default assume: ada di halaman terpisah `/ecl/eir/bulk-recompute` (M6 scope). Tidak ada di halaman per-instrumen ini.

---

## Story APP-C-M9-005 — EIR Amendment Proposal Workflow UI

**Actor**: ROLE-AKUN (propose + cancel), ROLE-RISK (review), ROLE-ALCO (approve, step-up MFA)
**Trigger**: ROLE-AKUN akses `/ecl/eir/amendments/queue` atau `/ecl/eir/amendments/[id]`
**Goal**: Tampilkan antrian proposal amandemen EIR dalam DataTable; buka detail proposal; eksekusi tindakan (review/approve/reject/cancel) dari halaman detail; badge khusus untuk proposal yang di-auto-create dari drift detection.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.4–4.5, M6 lifecycle
**Linked API**: `GET /api/v1/ecl/eir/amendments`, `GET /api/v1/ecl/eir/amendments/{id}`, `POST /api/v1/ecl/eir/amendments/{id}/review`, `POST /api/v1/ecl/eir/amendments/{id}/approve`, `POST /api/v1/ecl/eir/amendments/{id}/reject`, `POST /api/v1/ecl/eir/amendments/{id}/cancel`

### Pre-conditions
- User ter-autentikasi dengan permission `eir.amendment_review.read` (atau `eir.amendment.cancel` untuk aksi cancel).

### Komponen
- `<DataTable>` queue dengan tab cepat: "Semua", "Menunggu Review Saya", "Menunggu Approval Saya", "Sudah Selesai"
- Badge `AUTO (Drift)` (kuning) untuk proposal dengan `trigger_source IN ('CRON_DRIFT', 'AD_HOC_BULK', 'DOCUMENT_UPLOAD')`
- Halaman detail: info card instrumen, EIR lama vs baru, cashflow revisi (read-only table), sign-off timeline, action panel
- `<MFAStepUpModal>` untuk ROLE-ALCO approve
- `<DestructiveActionDialog>` untuk reject dan cancel

### Acceptance Criteria — APP-C-M9-005

```gherkin
Feature: EIR amendment proposal workflow UI

  Background:
    Given 10 proposal amandemen di ecl.eir_reestimation_log:
      | 3 PENDING_REVIEW  (2 MANUAL, 1 CRON_DRIFT auto)  |
      | 2 PENDING_APPROVAL                               |
      | 3 APPROVED                                       |
      | 2 REJECTED                                       |
    And AKUN-01 (ROLE-AKUN), RISK-01 (ROLE-RISK), ALCO-01 (ROLE-ALCO)
    And semua memiliki permission eir.amendment_review.read

  # ---------------------------------------------------------------
  # Skenario 1 — Queue DataTable: tab + badge drift
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK membuka queue — tab aktif dan badge AUTO (Drift)
    When RISK-01 akses /ecl/eir/amendments/queue
    Then DataTable menampilkan default tab "Menunggu Review Saya" dengan 3 PENDING_REVIEW
    And row dengan trigger_source = "CRON_DRIFT" menampilkan badge "AUTO (Drift)" berwarna kuning
    And kolom yang tersedia: ID, Instrumen, Status, Trigger, EIR Lama, Drift Δ, Dibuat, Deadline, Aksi
    And tab "Menunggu Approval Saya" menampilkan 2 PENDING_APPROVAL
    And tab "Sudah Selesai" menampilkan 5 (3 APPROVED + 2 REJECTED) dengan default sort Terbaru

  # ---------------------------------------------------------------
  # Skenario 2 — Detail proposal: info lengkap
  # ---------------------------------------------------------------
  Scenario: Membuka detail proposal amandemen
    When RISK-01 klik baris proposal "AMEND-001" (CRON_DRIFT, PENDING_REVIEW)
    Then halaman /ecl/eir/amendments/AMEND-001 menampilkan:
      | Info Instrumen    | kode, nama, klasifikasi, EIR lama                                   |
      | Trigger           | "Auto (Drift CRON) — Δ = 0.0020 (20 bp) pada 2026-06-12"           |
      | EIR Baru Preview  | {hasil solver re-compute, 8 desimal}                                |
      | Cashflow Revisi   | tabel read-only periods + cashflows terbaru                         |
      | Sign-off Timeline | Maker: [system/CRON], Review: Belum, Approve: Belum                 |
      | Drift Report Link | "Lihat Drift Report #DR-001" (badge link jika proposal auto-created)|
    And tombol "Review & Approve" tersedia untuk RISK-01 (sesuai tab Menunggu Review Saya)
    And tombol "Tolak" tersedia

  # ---------------------------------------------------------------
  # Skenario 3 — ROLE-RISK review proposal (sign-off step 1)
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK review proposal PENDING_REVIEW → PENDING_APPROVAL
    When RISK-01 klik "Setujui Review" pada halaman detail AMEND-001
    Then POST /api/v1/ecl/eir/amendments/AMEND-001/review dengan:
      | body | { "comment": "Drift 20 bp signifikan, amandemen valid.", "signature_method": "JWT_STEP_UP" } |
    And response 200 dengan workflow_status = "PENDING_APPROVAL"
    And toast sukses (RISK-01): "Proposal AMEND-001 disetujui review. Menunggu approval ALCO."
    And sign-off timeline di-update: "Review: RISK-01 pada 2026-06-13 14:30"
    And ALCO-01 menerima notifikasi: "Proposal amandemen EIR AMEND-001 menunggu approval Anda."

  # ---------------------------------------------------------------
  # Skenario 4 — ROLE-ALCO approve dengan step-up MFA
  # ---------------------------------------------------------------
  Scenario: ROLE-ALCO approve proposal — step-up MFA wajib (DEC-027)
    Given proposal "AMEND-001" dalam status PENDING_APPROVAL
    When ALCO-01 klik "Approve" di halaman detail AMEND-001
    Then modal konfirmasi muncul:
      | Judul   | "Approve Amandemen EIR — OBL-2026-00001"                                    |
      | Detail  | "EIR akan berubah dari 4.2500% → 4.4500%. Catchup adjustment: Rp. 1,234,567." |
      | Tombol  | "Lanjutkan ke MFA" | "Batal"                                                |
    When ALCO-01 klik "Lanjutkan ke MFA"
    Then <MFAStepUpModal> tampil
    When ALCO-01 submit kode MFA valid
    Then POST /api/v1/ecl/eir/amendments/AMEND-001/approve dikirim
    And response 200 dengan workflow_status = "APPROVED"
    And ecl.eir_amortization_schedule rows baru di-insert (schedule version baru)
    And ecl.eir_reestimation_log.eir_sesudah terisi
    And toast sukses (ALCO-01): "Amandemen EIR AMEND-001 disetujui. Schedule EIR OBL-2026-00001 diperbarui."
    And aud.audit_log berisi "EIR.AMENDMENT_APPROVED" dengan signed_at + signature_method

  # ---------------------------------------------------------------
  # Skenario 5 — SoD: AKUN-01 (maker) tidak boleh jadi reviewer
  # ---------------------------------------------------------------
  Scenario: AKUN-01 tidak bisa review proposal yang dia buat sendiri
    Given proposal "AMEND-002" dengan maker_id = AKUN-01
    And AKUN-01 juga memiliki permission eir.amend.review (hipotesis)
    When AKUN-01 mencoba klik "Review" pada AMEND-002
    Then button "Review" di-disable di UI dengan tooltip: "Anda tidak dapat mereview proposal yang Anda buat sendiri (SoD)."
    And jika akses langsung via API POST /review: response 403 error.code = "SOD_VIOLATION"

  # ---------------------------------------------------------------
  # Skenario 6 — Cancel proposal oleh maker (ROLE-AKUN)
  # ---------------------------------------------------------------
  Scenario: AKUN-01 cancel proposal PENDING_REVIEW
    Given proposal "AMEND-003" dengan maker_id = AKUN-01, status PENDING_REVIEW
    When AKUN-01 klik "Batalkan Proposal" di detail AMEND-003
    Then <DestructiveActionDialog> muncul:
      | Judul   | "Batalkan proposal amandemen AMEND-003?"                                 |
      | Input   | Textarea cancel_reason (min 20 karakter)                                  |
      | Tombol  | "Batalkan Proposal" (merah) | "Kembali"                                  |
    When AKUN-01 isi reason valid dan konfirmasi
    Then POST /api/v1/ecl/eir/amendments/AMEND-003/cancel dikirim
    And response 200 dengan workflow_status = "CANCELLED"
    And toast sukses: "Proposal amandemen AMEND-003 berhasil dibatalkan."
    And halaman kembali ke queue

  # ---------------------------------------------------------------
  # Skenario 7 — Drift report link: auto-created proposal punya badge linkage
  # ---------------------------------------------------------------
  Scenario: Proposal CRON_DRIFT menampilkan link ke drift report
    Given proposal "AMEND-001" dengan trigger_source = "CRON_DRIFT" dan drift_report_id = "DR-001"
    When user membuka halaman detail AMEND-001
    Then tombol "Lihat Drift Report DR-001" muncul di halaman detail
    And klik tombol → navigasi ke /ecl/eir/drift-reports/DR-001

  # ---------------------------------------------------------------
  # Skenario 8 — ROLE-AKUN hanya melihat proposal milik sendiri
  # ---------------------------------------------------------------
  Scenario: ROLE-AKUN melihat queue — hanya proposal milik sendiri
    Given AKUN-01 punya 2 proposal (1 PENDING_REVIEW, 1 APPROVED)
    And 8 proposal lain dari AKUN-02, AKUN-03
    When AKUN-01 akses /ecl/eir/amendments/queue
    Then DataTable menampilkan hanya 2 proposal milik AKUN-01
    And proposal dari maker lain tidak tampil

  # ---------------------------------------------------------------
  # Skenario 9 — Export queue
  # ---------------------------------------------------------------
  Scenario: Export CSV queue dengan filter aktif
    When RISK-01 klik Export CSV dengan filter tab "Menunggu Review Saya"
    Then file CSV ter-download hanya berisi 3 proposal PENDING_REVIEW
    And aud.audit_log berisi "EIR.AMENDMENT_EXPORT"
```

### Open Questions — M9-005
- **OQ-M9-005-A**: Halaman detail amandemen — apakah cashflow revisi (`revised_cashflow_json`) ditampilkan sebagai tabel inline, atau dalam collapsible/modal? Default assume: tabel inline dengan max 10 row preview + "Lihat Semua" collapsible.
- **OQ-M9-005-B**: Approval modal harus menampilkan `catch_up_adjustment` dalam IDR (Rp) — apakah nilai ini sudah tersedia di response `GET /amendments/{id}` atau dihitung on-the-fly saat modal dibuka? Konfirmasi ke `system-analyst` dan `ecl-eir-engineer`.

---

## Story APP-C-M9-006 — Drift Report Dashboard

**Actor**: ROLE-RISK (primary), ROLE-ALCO, ROLE-AUDIT (read-only)
**Trigger**: User navigasi ke `/ecl/eir/drift-reports` (list) atau `/ecl/eir/drift-reports/[id]` (detail)
**Goal**: Melihat daftar semua drift report (cron + ad-hoc), memeriksa detail per-instrumen, mengikuti link ke proposal auto-generated, dan memicu manual drift scan dengan konfirmasi dialog + progress notification.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.5 (drift detection output), M6 `sys.drift_report`
**Linked API**: `GET /api/v1/ecl/eir/drift-reports`, `GET /api/v1/ecl/eir/drift-reports/{id}`, `POST /api/v1/ecl/eir/drift-detection/trigger`

### Pre-conditions
- User ter-autentikasi dengan permission `eir.drift_report.read`.
- Minimal satu baris di `sys.drift_report` (atau empty state).

### Komponen
- `<DataTable>` list drift reports (sort, filter, cursor paging, export)
- Tombol "Generate Drift Report" (manual trigger) dengan `<DestructiveActionDialog>` untuk konfirmasi
- `<JobProgressPanel>` untuk job drift detection (UX §3 — operasi > 2 detik untuk ratusan instrumen)
- Per-instrumen detail table dalam halaman detail report
- Link badge "Proposal Dibuat" untuk row yang auto-generated proposal

### Acceptance Criteria — APP-C-M9-006

```gherkin
Feature: Drift report dashboard

  Background:
    Given 5 sys.drift_report tersimpan:
      | ID    | trigger_type | run_at           | total_scanned | drift_count | proposal_auto_created | status    |
      | DR-001 | CRON_DAILY  | 2026-06-12 02:00 | 300           | 3           | 2                     | COMPLETED |
      | DR-002 | AD_HOC      | 2026-06-11 10:15 | 300           | 1           | 0                     | COMPLETED |
      | DR-003 | CRON_DAILY  | 2026-06-11 02:00 | 300           | 0           | 0                     | COMPLETED |
      | DR-004 | AD_HOC      | 2026-06-10 15:30 | 50            | 5           | 5                     | COMPLETED |
      | DR-005 | PRE_ECL_CALC | 2026-06-09      | 300           | 0           | 0                     | COMPLETED |
    And RISK-01 memiliki role ROLE-RISK dan permission eir.drift_report.read + eir.amendment.detect

  # ---------------------------------------------------------------
  # Skenario 1 — List drift reports: DataTable UX §1
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK membuka halaman drift reports — list DataTable
    When RISK-01 akses /ecl/eir/drift-reports
    Then DataTable menampilkan 5 rows dengan kolom:
      | ID, Trigger, Waktu Scan, Total Instrumen, Drift Count, Proposal Dibuat, Status, Aksi |
    And default sort: run_at DESC
    And row DR-001 menampilkan drift_count = 3, proposal_auto_created = 2 dengan badge berwarna merah untuk drift_count > 0
    And row DR-003 menampilkan drift_count = 0, badge hijau
    And filter tersedia: Trigger Type (dropdown: CRON_DAILY, AD_HOC, PRE_ECL_CALC), Tanggal Run (date range), Drift Count (> 0 toggle)
    And Export CSV tersedia

  # ---------------------------------------------------------------
  # Skenario 2 — Detail report: per-instrumen table
  # ---------------------------------------------------------------
  Scenario: Membuka detail drift report DR-001
    When RISK-01 klik baris DR-001
    Then halaman /ecl/eir/drift-reports/DR-001 menampilkan:
      | Summary card | total_scanned=300, drift_count=3, missing_count=0, proposal_auto_created=2 |
      | Trigger      | "CRON_DAILY — 2026-06-12 02:00 WIB"                                        |
    And tabel per-instrumen drift entries dengan kolom:
      | Instrumen, EIR Tersimpan, EIR Re-compute, Δ (bp), Severity, Status Proposal        |
    And row dengan proposal_auto_created: badge "Proposal Dibuat" dengan link ke /ecl/eir/amendments/{id}
    And row dengan severity = "LOW": badge abu-abu, tanpa proposal link
    And row dengan severity = "MISSING_SCHEDULE": badge merah, keterangan "Schedule tidak ada"

  # ---------------------------------------------------------------
  # Skenario 3 — Manual trigger drift detection — konfirmasi dialog
  # ---------------------------------------------------------------
  Scenario: ROLE-RISK trigger drift detection manual — konfirmasi + progress
    When RISK-01 klik tombol "Generate Drift Report"
    Then <DestructiveActionDialog> muncul (bukan destructive tapi confirmatory):
      | Judul    | "Generate Drift Report Sekarang?"                                          |
      | Deskripsi | "Akan menjalankan re-estimasi EIR untuk semua instrumen aktif (estimasi ~300 instrumen, ≤ 5 detik). Proposal amandemen akan dibuat otomatis untuk drift > threshold." |
      | Tombol   | "Jalankan" | "Batal"                                                         |
    When RISK-01 klik "Jalankan"
    Then POST /api/v1/ecl/eir/drift-detection/trigger dikirim
    And response 202 dengan { jobId, statusUrl, streamUrl }
    And <JobProgressPanel> tampil dengan:
      | Judul         | "Drift Detection — EIR Scan"                             |
      | Progress bar  | 0% → update setiap ~50 instrumen                         |
      | Current step  | "Mengevaluasi instrumen 50 dari 300"                     |
      | Tombol        | "Background" + "Batalkan"                                |
    When job selesai (progress 100%)
    Then SSE event "completed" diterima
    And toast sukses: "Drift report berhasil dibuat. 300 instrumen dievaluasi, 3 drift terdeteksi, 2 proposal dibuat."
    And link "Lihat Laporan" ke /ecl/eir/drift-reports/{id_baru}
    And list DataTable di-refresh untuk tampilkan report terbaru

  # ---------------------------------------------------------------
  # Skenario 4 — Concurrent prevention: tidak bisa trigger manual jika cron sedang jalan
  # ---------------------------------------------------------------
  Scenario: Gagal trigger — job drift detection lain sedang berjalan
    Given job drift detection sedang berjalan (status "running")
    When RISK-01 klik "Generate Drift Report" dan konfirmasi
    Then response 409 dengan error.code = "CONFLICT"
    And toast error: "Drift detection sedang berjalan (Job: [jobId]). Tunggu hingga selesai sebelum memulai yang baru."
    And tombol "Generate Drift Report" di-disable selama ada job aktif

  # ---------------------------------------------------------------
  # Skenario 5 — Background mode: user lanjut kerja sambil job berjalan
  # ---------------------------------------------------------------
  Scenario: User klik "Background" pada JobProgressPanel — pindah ke halaman lain
    Given job drift detection sedang berjalan
    When RISK-01 klik "Background" di JobProgressPanel
    Then panel tertutup
    And global notification badge di top bar menyala (angka 1 badge merah)
    And RISK-01 bisa navigasi ke halaman lain
    When job selesai
    Then toast muncul secara otomatis di posisi manapun user berada: "Drift Report selesai. 3 instrumen terdeteksi drift."
    And badge notif at top bar terupdate

  # ---------------------------------------------------------------
  # Skenario 6 — Filter dan sort list
  # ---------------------------------------------------------------
  Scenario: Filter drift reports berdasarkan trigger type dan drift count > 0
    When RISK-01 set filter trigger_type = "CRON_DAILY" dan toggle "Hanya Drift > 0"
    Then DataTable hanya menampilkan DR-001 (CRON_DAILY, drift_count=3)
    And filter chips tampil di atas tabel
    And URL: ?filter[trigger_type]=CRON_DAILY&filter[drift_count]=gt:0

  # ---------------------------------------------------------------
  # Skenario 7 — ROLE-AUDIT: tidak bisa trigger manual
  # ---------------------------------------------------------------
  Scenario: ROLE-AUDIT membuka halaman — tombol trigger tidak tersedia
    Given AUDIT-01 memiliki permission eir.drift_report.read tapi TIDAK eir.amendment.detect
    When AUDIT-01 akses /ecl/eir/drift-reports
    Then DataTable tampil lengkap dengan semua reports
    And tombol "Generate Drift Report" tidak tampil / disabled dengan tooltip: "Memerlukan permission eir.amendment.detect"
    And Export CSV tersedia

  # ---------------------------------------------------------------
  # Skenario 8 — Empty state: belum ada drift report sama sekali
  # ---------------------------------------------------------------
  Scenario: Halaman pertama kali — belum ada drift report
    Given sys.drift_report kosong
    When RISK-01 akses /ecl/eir/drift-reports
    Then DataTable empty state: "Belum ada drift report. Klik 'Generate Drift Report' untuk menjalankan evaluasi pertama."
    And tombol "Generate Drift Report" aktif

  # ---------------------------------------------------------------
  # Skenario 9 — Export list drift reports
  # ---------------------------------------------------------------
  Scenario: Export CSV list drift reports dengan filter aktif
    When RISK-01 klik Export CSV dengan filter trigger_type = "CRON_DAILY"
    Then file CSV ter-download dengan hanya CRON_DAILY reports
    And aud.audit_log berisi "DRIFT_REPORT.EXPORT" dengan after_jsonb.filters
```

### Open Questions — M9-006
- **OQ-M9-006-A**: Halaman detail drift report — per-instrumen drift entries disimpan di `sys.drift_report.result_jsonb` (JSONB) bukan sebagai tabel terpisah. Apakah JSONB cukup untuk pagination (instrumen dalam drift report bisa ratusan)? Default assume: untuk MVP, load semua entries dari JSONB ke DataTable client-side (max ~300 instrumen = acceptable). Jika > 500 instrumen per report, perlu normalisasi ke tabel terpisah. Flag ke `data-modeler`.
- **OQ-M9-006-B**: JobProgressPanel di halaman drift reports: apakah panel muncul inline di halaman ini saja, atau juga dari global job progress drawer yang sudah ada di Phase 3? Default assume: gunakan komponen `<JobProgressPanel>` Phase 3 yang sudah ada (global), dengan redirect link ke `/ecl/eir/drift-reports` saat job selesai.

---

## Matriks Ringkasan Stories

| Story | Screen(s) | Actor Utama | Key AC | Komponen Utama |
|---|---|---|---|---|
| M9-001 | `/ecl/staging/instrumen/[id]` | ROLE-RISK, ROLE-AUDIT | Stage banner warna, history filter/export, SICR evidence | DataTable, stage badge |
| M9-002 | `/ecl/staging/override/new`, `/ecl/staging/override` | ROLE-RISK (propose), ROLE-ALCO (approve) | 6-eyes flow, step-up MFA, SoD, expiry picker | MFAStepUpModal, DataTable, DestructiveActionDialog |
| M9-003 | `/ecl/staging/dpd` | ROLE-AKUN | DPD entry → re-staging trigger, JobProgress, duplicate check | DataTable, form, JobProgressPanel |
| M9-004 | `/ecl/eir/instrumen/[id]` | ROLE-RISK | Schedule versioned view, solver metadata, non-convergence error | DataTable, version selector, solver panel |
| M9-005 | `/ecl/eir/amendments/queue`, `/ecl/eir/amendments/[id]` | ROLE-AKUN (propose), ROLE-RISK (review), ROLE-ALCO (approve) | 4-eyes flow, drift badge, cancel, MFA modal | DataTable, MFAStepUpModal, DestructiveActionDialog |
| M9-006 | `/ecl/eir/drift-reports`, `/ecl/eir/drift-reports/[id]` | ROLE-RISK, ROLE-ALCO | Manual trigger + JobProgress, per-instrumen entries, proposal link | DataTable, JobProgressPanel, DestructiveActionDialog |

---

## Ringkasan Open Questions

| ID | Pertanyaan | Default | Butuh dari | Blocking? |
|---|---|---|---|---|
| OQ-M9-001-A | `sicr_evidence_jsonb` sudah di OpenAPI M1? | Belum — perlu system-analyst tambah | system-analyst | Ya, sebelum frontend coding M9-001 |
| OQ-M9-001-B | Tombol "Evaluasi Manual" di halaman staging instrumen? | Ya, tersedia untuk ROLE-RISK | orchestrator | Tidak |
| OQ-M9-002-A | Staging override approver-2: hanya ROLE-KOMITE atau ROLE-ALCO juga boleh? | Keduanya boleh | BRD §8.3 + FSD §3.4 | Ya, sebelum state machine dibuat |
| OQ-M9-002-B | Override `expiry_periode` — auto-revert saat expire atau info saja? | Info saja, tidak auto-revert | ROLE-RISK + ROLE-ALCO | Tidak |
| OQ-M9-003-A | Re-staging per DPD entry — apakah < 2 detik (inline) atau butuh JobProgressPanel? | Tergantung overhead; default pakai spinner + toast | ecl-eir-engineer | Ya, sebelum UX frontend dibangun |
| OQ-M9-003-B | DPD correction yang membalik stage — trigger_type = "DPD_CORRECTION" atau "CURE"? | DPD_CORRECTION (berbeda dari CURE yang butuh 3 periode) | ifrs9-compliance-reviewer | Ya |
| OQ-M9-004-A | `solver_metadata` (iterations, residual) sudah di response M5 compute endpoint? | Belum — perlu system-analyst tambah | system-analyst | Ya |
| OQ-M9-004-B | Bulk EIR recompute button di halaman per-instrumen atau halaman terpisah? | Halaman terpisah /ecl/eir/bulk-recompute (M6 scope) | orchestrator | Tidak |
| OQ-M9-005-A | Cashflow revisi di detail amandemen: tabel inline atau collapsible? | Tabel inline, max 10 row + "Lihat Semua" | uiux-designer | Tidak |
| OQ-M9-005-B | `catch_up_adjustment` IDR di approve modal — dari response GET atau computed on-the-fly? | Dari response GET /amendments/{id} | system-analyst + ecl-eir-engineer | Ya |
| OQ-M9-006-A | Per-instrumen entries di drift report — JSONB cukup atau perlu normalisasi tabel? | JSONB untuk MVP (≤ 300 instrumen) | data-modeler | Tidak untuk MVP |
| OQ-M9-006-B | JobProgressPanel drift — inline halaman ini atau global drawer Phase 3? | Gunakan global dari Phase 3 | frontend-engineer-nextjs | Tidak |

---

## Handoff Checklist

```
P4-M9 stories selesai →

  system-analyst (PRIORITAS SEBELUM CODING FRONTEND):
    - Konfirmasi field sicr_evidence_jsonb di GET /staging/history/{id} response (OQ-M9-001-A)
    - Konfirmasi/tambah field solver_metadata di POST /ecl/eir/compute response (OQ-M9-004-A)
    - Konfirmasi field catch_up_adjustment tersedia di GET /amendments/{id} response (OQ-M9-005-B)
    - Konfirmasi state machine staging override: approver-2 role options (OQ-M9-002-A)

  uiux-designer (PARALEL dengan system-analyst):
    - Desain halaman /ecl/staging/instrumen/[id] — stage banner, timeline DataTable
    - Desain form override baru (/ecl/staging/override/new) + approval queue
    - Desain form DPD entry (/ecl/staging/dpd)
    - Desain halaman EIR schedule (/ecl/eir/instrumen/[id]) — version selector + solver panel
    - Desain amendment queue + detail (/ecl/eir/amendments/queue + [id])
    - Desain drift report list + detail (/ecl/eir/drift-reports + [id])

  frontend-engineer-nextjs (SETELAH system-analyst konfirmasi OpenAPI):
    - Implementasi 6 screen sesuai stories ini
    - Reuse komponen Phase 3: DataTable, JobProgressPanel, MFAStepUpModal, notify, DestructiveActionDialog
    - Implement version selector dropdown untuk EIR schedule view
    - Implement badge AUTO (Drift) di amendment queue
    - State URL (nuqs/searchParams) untuk semua DataTable filter/sort (deep-link friendly)

  ifrs9-compliance-reviewer (ADVISORY):
    - Verifikasi UI menampilkan semua versi schedule (termasuk superseded) — tidak hide old versions
    - Verifikasi staging timeline menampilkan semua SICR triggers lengkap beserta evidence
    - Konfirmasi OQ-M9-003-B (DPD correction trigger_type)

  qa-engineer:
    - UAT scripts sesuai happy path + edge case per story di atas
    - Aksesibilitas: WCAG 2.1 AA untuk semua form dan DataTable
    - Test SoD enforcement di UI (tombol Approve di-disable untuk proposer yang sama)
    - Test empty state + loading skeleton + error state per screen
    - Test URL deep-link: bookmark filter URL → restore state saat reload
```
