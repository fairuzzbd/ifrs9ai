# APP-A-MSTR-001 — Pola Generik Master Data CRUD (Reusable Template)

**Story ID**: APP-A-MSTR-001
**Modul**: APP-A — Master Data Management
**Tipe**: Template Story (direplikasi ke 16 modul `mst.*`)
**Status**: DRAFT — menunggu review system-analyst
**Author**: business-analyst
**Tanggal**: 2026-06-03
**Linked FSD**: FSD-APP-A-MasterData-SPPI-BM-v1.1.md §1 (semua sub-modul master data)
**Linked BRD**: BR-MAS-001 sampai BR-MAS-015; BRD §8.1 (Master Data Requirements)
**Linked Decision Log**: DEC-017, DEC-021, DEC-022, DEC-018

---

## Konteks & Tujuan

Pola generik ini mendefinisikan kontrak behaviour yang HARUS diimplementasi secara konsisten di seluruh 16 modul master data `mst.*`. Setiap modul konkret (mis. `mst.mata_uang`, `mst.portofolio`) WAJIB mengimplementasi semua fitur dalam pola ini kecuali ada VARIAN yang secara eksplisit di-flag (lihat bagian Varian di akhir dokumen ini).

Tujuan: satu pola → 16 implementasi yang konsisten → UX predictable, audit trail uniform, keamanan merata.

---

## Actors

| Role | Aksi yang Diizinkan | Catatan |
|---|---|---|
| ROLE-MAKER-TR | Create, Update (DRAFT), Read, Submit, Soft-delete (sebelum active) | Per modul, maker adalah role domain yang relevan |
| ROLE-APPR-TR | Read, Review (sign/reject), Approve | Per modul, reviewer & approver bisa berbeda role |
| ROLE-RISK | Read; Review untuk modul terkait risk (pd, lgd, staging) | Role domain tergantung modul |
| ROLE-AKUN | Read; Maker untuk modul akuntansi (CoA, mapping jurnal) | |
| ROLE-AKUN-CTL | Review/Approve untuk modul akuntansi | MFA mandatory |
| ROLE-ALCO | Approve untuk ECL-param masters (bobot, LGD, PD, impact_mev_pd, impact_pd) | MFA mandatory + step-up |
| ROLE-AUDIT | Read-only semua modul; export dengan filter aktif | Tidak bisa mutasi apapun |
| ROLE-IT-ADMIN | Read; user management (tidak bisa domain workflow) | SoD — IT tidak approve domain entity |
| ROLE-KOMITE | Approve klasifikasi PSAK 71; Read semua | MFA mandatory |
| ROLE-CFO | Read semua; Final approve periode buku | MFA mandatory |

**Catatan SoD per pola generik**: `maker_id ≠ reviewer_id ≠ approver_id` ditegakkan server-side di service layer, bukan hanya UI.

---

## Trigger

- Pengguna dengan permission yang sesuai mengakses halaman list master data dan memilih aksi (Create / Edit / Delete / Export / Workflow transition).
- Atau, sistem eksternal (feed integration) memicu create/update via API (wajib tetap validasi permission & idempotency).

---

## Pre-conditions

1. User ter-autentikasi via Keycloak (JWT valid, `mfa_verified` sesuai role requirement).
2. User memiliki permission `{entity}.{action}` yang relevan di JWT claims.
3. Periode buku tidak dalam status `HARD_CLOSED` untuk operasi yang mengubah data historis (kecuali operasi master yang bukan terkait posting periode).
4. Idempotency-Key (UUID v4) dikirim di header untuk setiap mutation request.

---

## Acceptance Criteria — Pola Generik

### Feature: Create Master Record (Maker)

```gherkin
Feature: Create master data record

  Background:
    Given user ter-autentikasi sebagai ROLE-MAKER-TR
    And user memiliki permission "{entity}.create"
    And request header mengandung "Idempotency-Key: <uuid-baru>"

  Scenario: Happy path — create record baru berhasil
    When user mengisi form dengan data valid lengkap
    And user klik "Simpan"
    Then sistem membuat record baru dengan status "DRAFT"
    And sistem menulis row ke "aud.audit_log" dengan action "{ENTITY}.CREATE" di transaksi yang sama
    And response 201 mengandung "{ data: { id, kode, status: 'DRAFT' }, meta: { traceId } }"
    And toast sukses muncul: "'{kode}' berhasil dibuat. Menunggu review."
    And toast mengandung action link "Lihat detail →"

  Scenario: Validation error — field wajib kosong
    When user submit form dengan field wajib yang kosong
    Then sistem mengembalikan HTTP 400 VALIDATION_FAILED
    And response mengandung "details" dengan daftar field bermasalah
    And field yang bermasalah di-highlight di form dengan inline error message
    And toast error merah persistent: "N field bermasalah — lihat form di bawah"
    And tidak ada record yang dibuat
    And tidak ada row di "aud.audit_log"

  Scenario: Idempotency — request duplikat dengan key & payload sama
    Given request pertama sudah berhasil (HTTP 201)
    When user kirim ulang request dengan Idempotency-Key yang sama dan payload identik
    Then sistem mengembalikan HTTP 200 dengan response identik ke request pertama (replay)
    And response error code "IDEMPOTENCY_REPLAY"
    And tidak ada row audit log tambahan

  Scenario: Idempotency — key sama tapi payload beda
    Given request pertama sudah berhasil
    When user kirim request dengan Idempotency-Key sama tapi payload berbeda
    Then sistem mengembalikan HTTP 422 "IDEMPOTENCY_MISMATCH"
    And tidak ada record baru dibuat
```

---

### Feature: Read & List (semua role yang punya {entity}.read)

```gherkin
Feature: List master data dengan sort, paging, filter, export

  Background:
    Given user ter-autentikasi dengan permission "{entity}.read"

  Scenario: List default — tampil dengan sort & paging default
    When user mengakses halaman list "{entity}"
    Then tabel menampilkan data dengan sort default "created_at DESC"
    And paging default 50 record per halaman
    And ada indikator sort di header kolom
    And footer menampilkan "Page 1 of ~N" dengan tombol Prev/Next
    And URL state mencerminkan sort & paging aktif (deep-link)

  Scenario: Sort multi-kolom
    When user klik header kolom pertama (mis. "Kode")
    Then data di-sort ascending berdasarkan kolom tersebut
    And ikon sort ↑ muncul di header kolom
    When user shift+click header kolom kedua
    Then data di-sort berdasarkan 2 kolom (multi-sort)
    And query URL berubah ke "?sort=kode:asc,created_at:desc"

  Scenario: Filter per kolom + text search global
    When user mengetik teks di search box
    Then tabel menampilkan hanya record yang cocok
    And filter chip muncul di filter bar
    And URL state di-update: "?q=teks_cari"
    When user tambahkan filter per kolom (mis. "status=AKTIF")
    Then tabel memfilter kombinasi keduanya
    And chip "status: AKTIF" muncul di filter bar
    And URL state: "?q=teks&filter[status]=AKTIF"
    When user klik "Clear all"
    Then semua filter dihapus dan tabel kembali ke default

  Scenario: Export CSV — dataset kecil (< 10k row)
    When user klik "Export" → pilih "CSV"
    Then sistem mengirim file CSV langsung ke browser
    And header row: nama kolom human-readable (Bahasa Indonesia)
    And filter & sort aktif di-respect (bukan dump semua data)
    And audit log mencatat "{ENTITY}.EXPORT" dengan filter, jumlah row, format

  Scenario: Export XLSX — dataset besar (>= 10k row)
    Given total record dengan filter aktif >= 10.000
    When user klik "Export" → pilih "XLSX"
    Then sistem mengembalikan 202 Accepted dengan jobId
    And komponen <JobProgressPanel> muncul dengan progress bar
    And user bisa menutup panel dan lanjut bekerja (background mode)
    And saat selesai, badge notifikasi muncul di top bar
    And toast sukses muncul dengan link download (signed URL, TTL 24 jam)
    And audit log mencatat "{ENTITY}.EXPORT" async

  Scenario: Empty state — tidak ada data
    Given tidak ada record yang cocok dengan filter aktif
    Then tabel menampilkan ilustrasi empty state
    And pesan "Tidak ada data yang cocok"
    And tombol "Clear filter" tampil jika filter aktif

  Scenario: ROLE-AUDIT read-only
    Given user ter-autentikasi sebagai ROLE-AUDIT
    Then tombol "New", "Edit", "Delete" tidak tampil
    And semua aksi workflow (Submit, Review, Approve, Reject) tidak tampil
    And user tetap bisa read & export
```

---

### Feature: Update Master Record (Maker, status DRAFT atau RETURNED)

```gherkin
Feature: Update master data record

  Background:
    Given user ROLE-MAKER-TR dengan permission "{entity}.update"
    And record dalam status "DRAFT" atau "RETURNED" (dikembalikan approver)

  Scenario: Happy path — update field berhasil
    When user mengubah satu atau lebih field dan klik "Simpan"
    Then sistem update record, increment row_version
    And audit log mencatat "{ENTITY}.UPDATE" dengan before_jsonb & after_jsonb
    And toast sukses: "'{kode}' berhasil diperbarui."

  Scenario: Optimistic lock conflict — row_version mismatch
    Given user A dan user B membuka form edit record yang sama secara bersamaan
    When user A simpan lebih dulu
    And user B kemudian mencoba simpan
    Then sistem mengembalikan HTTP 409 CONFLICT
    And pesan: "Record telah diubah oleh user lain. Muat ulang halaman."
    And data user B tidak ditimpa

  Scenario: Tidak bisa update record yang sudah APPROVED
    Given record dalam status "APPROVED" atau "AKTIF"
    When user mencoba akses form edit
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Record yang sudah disetujui tidak bisa diubah langsung. Ajukan amandemen."
```

---

### Feature: Workflow Approval 4-Eyes (Maker → Reviewer → Approver)

```gherkin
Feature: Workflow approval 4-eyes untuk master data

  Background:
    Given record ada dalam status "DRAFT"

  Scenario: Maker submit untuk review
    Given user adalah maker record ini (ROLE-MAKER-TR)
    When user klik "Kirim untuk Review"
    Then status berubah menjadi "PENDING_REVIEW"
    And sistem mencatat maker_id + submitted_at
    And notifikasi dikirim ke user dengan role Reviewer yang relevan
    And audit log: "{ENTITY}.SUBMIT"

  Scenario: Reviewer sign-off (approve ke approver)
    Given record dalam status "PENDING_REVIEW"
    And user adalah REVIEWER yang berbeda dari maker (SoD check)
    When user klik "Setujui untuk Approval"
    And mengisi komentar (opsional)
    Then status berubah menjadi "PENDING_APPROVAL"
    And sistem mencatat reviewer_id + reviewed_at + signature_hash
    And audit log: "{ENTITY}.REVIEW"

  Scenario: SoD violation — reviewer sama dengan maker
    Given record dalam status "PENDING_REVIEW"
    And user yang login adalah maker record ini
    When user mencoba klik "Setujui untuk Approval"
    Then sistem mengembalikan HTTP 403 SOD_VIOLATION
    And pesan: "Anda tidak bisa menjadi reviewer untuk record yang Anda buat sendiri."
    And aksi tidak dilakukan

  Scenario: Approver final approve
    Given record dalam status "PENDING_APPROVAL"
    And user adalah APPROVER yang berbeda dari maker dan reviewer (SoD check)
    When user klik "Approve"
    And mengisi komentar (opsional)
    Then status berubah menjadi "APPROVED" (atau "AKTIF" sesuai logika modul)
    And sistem mencatat approver_id + approved_at + signature_hash
    And audit log: "{ENTITY}.APPROVE"
    And toast sukses: "'{kode}' berhasil disetujui dan sekarang aktif."

  Scenario: SoD violation — approver adalah maker atau reviewer
    Given record dalam status "PENDING_APPROVAL"
    And user yang login adalah maker ATAU reviewer record ini
    When user mencoba approve
    Then sistem mengembalikan HTTP 403 SOD_VIOLATION
    And pesan spesifik sesuai peran yang dilanggar

  Scenario: Reviewer reject — kembalikan ke maker
    Given record dalam status "PENDING_REVIEW" atau "PENDING_APPROVAL"
    When reviewer/approver klik "Tolak"
    And mengisi komentar alasan penolakan (WAJIB)
    Then status kembali ke "RETURNED"
    And komentar penolakan tersimpan
    And audit log: "{ENTITY}.REJECT" dengan komentar
    And notifikasi ke maker: "Record '{kode}' dikembalikan: {komentar}"
    And maker bisa edit dan re-submit

  Scenario: Confirm dialog untuk aksi destructive
    When user klik "Tolak" atau "Hapus (soft-delete)"
    Then dialog konfirmasi muncul sebelum aksi dieksekusi
    And dialog menampilkan: judul, deskripsi konsekuensi, tombol "Lanjut" (destructive) dan "Batal"
    And jika MFA diperlukan: step-up MFA prompt sebelum konfirmasi
```

---

### Feature: Soft-Delete

```gherkin
Feature: Soft-delete master record

  Background:
    Given user ROLE-MAKER-TR dengan permission "{entity}.delete"
    And record tidak memiliki referensi aktif di tabel lain (FK constraint)

  Scenario: Happy path — soft-delete record DRAFT
    Given record dalam status "DRAFT"
    When user klik "Hapus" dan konfirmasi di dialog
    Then sistem men-set "deleted_at" dan "deleted_by" pada record
    And record tidak muncul di list default
    And record tetap ada di DB (tidak hard-delete)
    And audit log: "{ENTITY}.DELETE" dengan before_jsonb

  Scenario: Tidak bisa soft-delete record dengan referensi aktif
    Given record sudah direferensikan oleh entitas lain (mis. instrumen dengan counterparty_id ini)
    When user mencoba menghapus record
    Then sistem mengembalikan HTTP 409 CONFLICT
    And pesan: "Record tidak bisa dihapus karena masih digunakan oleh N entitas lain."
    And tidak ada perubahan data

  Scenario: ROLE-AUDIT membaca record yang di-soft-delete
    Given ROLE-AUDIT mengakses list dengan parameter "?include_deleted=true"
    Then record yang sudah di-delete juga muncul (dengan indikator visual)
    And tombol restore/delete tetap tidak tampil untuk ROLE-AUDIT
```

---

### Feature: Audit Trail per Mutasi

```gherkin
Feature: Audit trail immutable untuk setiap mutasi

  Scenario: Setiap mutasi menghasilkan audit log entry
    Given user melakukan aksi mutasi apapun (create/update/submit/review/approve/reject/delete)
    Then sistem menulis row ke "aud.audit_log" DI TRANSAKSI DATABASE YANG SAMA
    And row berisi: event_id, event_time, actor_user_id, actor_role, action, entity_type, entity_id
    And row berisi: before_jsonb (null jika create), after_jsonb (null jika delete), ip, user_agent, trace_id, idempotency_key
    And row berisi: current_hash = sha256(previous_hash || canonical_json(row))

  Scenario: Audit log tidak bisa dimodifikasi via API
    Given user apapun mencoba DELETE atau UPDATE ke "aud.audit_log" via API
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Audit log bersifat immutable."
```

---

## Non-Functional Requirements (per pola generik)

| NFR | Requirement | Referensi |
|---|---|---|
| Idempotency | Setiap mutation endpoint wajib periksa `Idempotency-Key` (DEC-021) | api-conventions.md |
| Pagination | Cursor-based only, max 200 per request (DEC-022) | api-conventions.md |
| Decimal | Semua angka money/rate pakai `shopspring/decimal`, storage NUMERIC (DEC-016) | db-conventions.md |
| Audit retention | `aud.audit_log` 10+10 tahun, append-only (DEC-018) | security-baseline.md |
| Soft-delete | Tidak ada hard-delete di `aud`, `jrnl`, `ecl` (CLAUDE.md) | db-conventions.md |
| SoD enforcement | Server-side di service layer (DEC-017) | security-baseline.md |
| MFA | Wajib untuk role CFO/KOMITE/ALCO/AKUN-CTL (DEC-026) | security-baseline.md |
| Step-up MFA | Untuk approve ECL param dan hard-close (DEC-027) | security-baseline.md |

---

## Varian Pola (Flag, bukan detail penuh)

### VARIAN-A: ECL-Param Masters (6-Eyes + ALCO + Compliance Gate)
**Modul**: `pd_pefindo`, `lgd_basel`, `bobot_skenario`, `lps_coverage`, `impact_mev_pd`, `impact_pd`
**Delta dari pola generik**:
- Workflow: **6-eyes** (Maker → Reviewer 1 (RISK) → Reviewer 2 (AKUN-CTL) → Approver (ALCO))
  - DEC-017: "6-eyes untuk klasifikasi PSAK 71 & parameter master"
- Approval ALCO wajib **MFA step-up** (DEC-027)
- Gate blocking: `ifrs9-compliance-reviewer` harus sign-off sebelum PR merge
- ECL param yang sudah digunakan dalam `calc-run SEALED` tidak bisa diubah retroaktif (HTTP 423 `ECL_PARAM_FROZEN`)
- Perubahan parameter memicu re-run kalkulator PD untuk instrumen aktif (long-running job, UX rule §3)

**Flag untuk dispatch**: `ifrs9-compliance-reviewer` BLOCKING gate + story terpisah APP-A-MSTR-ECL-xxx

---

### VARIAN-B: Counterparty + Rating History (PII Encrypted + Security Gate)
**Modul**: `mst.counterparty`, `mst.rating_history`
**Delta dari pola generik**:
- Kolom PII (`npwp`, `nomor_rekening`, `ktp`) wajib **column-level encryption** via `sec.encrypt(col)` (DEC-028)
- Response API untuk non-AUDIT role: PII di-mask (mis. `"npwp": "**.**.***.*.******"`)
- ROLE-AUDIT mendapat decrypted data hanya dengan permission `counterparty.read.pii` (terpisah dari read biasa)
- SICR trigger otomatis: saat rating diinput, sistem auto-evaluate perubahan notch dan set `sicr_triggered` flag
- Gate blocking: `security-engineer` BLOCKING review untuk setiap PR yang menyentuh counterparty endpoint

**Flag untuk dispatch**: `security-engineer` BLOCKING gate + story terpisah APP-A-MSTR-CP-xxx

---

### VARIAN-C: Upload/Import Masters (Long-Process, UX §3)
**Modul**: `pd_pefindo` (XLSX upload), `chart_of_accounts` (Excel import), `kurs` (BI JISDOR scheduled job)
**Delta dari pola generik**:
- Trigger: upload file (XLSX/CSV) atau scheduled job, bukan form UI
- Proses: async via Asynq job (UX rule §3 mandatory — operasi > 2 detik)
- Submit endpoint: `POST /api/v1/{resource}/import` → `202 Accepted { jobId, statusUrl, streamUrl }`
- Frontend: `<JobProgressPanel>` dengan progress %, current step, ETA
- Validasi file: schema check (kolom wajib ada), max size (configurable), format check, hash SHA-256 tersimpan
- Preview diff: sebelum commit, tampilkan diff vs data existing (mis. PD berubah dari 0.0031 → 0.0035)
- Rollback: jika import gagal di tengah jalan, seluruh batch di-rollback (transactional)
- Audit: setiap import mencatat filename, SHA-256, uploader, row count, row diff, job_id

**Flag untuk dispatch**: `integration-engineer` untuk feed adapter + story terpisah APP-A-MSTR-IMP-xxx

---

## Open Questions untuk System-Analyst & Stakeholder

1. **Workflow config granularity**: Apakah tiap modul `mst.*` punya konfigurasi workflow terpisah di `sys.config WORKFLOW_CONFIG_*`, atau satu config generik dengan override per modul? Ini menentukan struktur tabel `sys.workflow_config`.

2. **Reviewer role per modul**: Beberapa modul mst.* belum ada kejelasan siapa reviewer-nya selain maker & approver umum. Perlu matrix lengkap: modul → maker role → reviewer role → approver role. (Contoh: `mst.portofolio` — siapa reviewer-nya? RISK atau AKUN?)

3. **Notifikasi delivery channel**: Apakah notifikasi workflow (mis. "ada record menunggu review") dikirim via in-app only, atau juga email? Jika email, perlu SMTP config di `sys.config`.

4. **Return reason wajib atau opsional?**: Pola generik menetapkan komentar penolakan WAJIB. Konfirmasi apakah ada modul di mana return reason bisa opsional.

5. **Bulk approve**: Apakah ROLE-APPR-TR bisa approve beberapa record sekaligus (bulk)? Jika ya, SoD tetap berlaku per record — sistem cek setiap record secara individual.

6. **Amandemen setelah APPROVED**: Untuk modul yang datanya bisa berubah (mis. `mst.portofolio.bm_category_default` berubah saat BM review tahunan) — apakah workflow amandemen menggunakan record baru (versioning) atau update in-place dengan workflow baru? Perlu keputusan explisit sebelum system-analyst membuat state machine.

---

## Rekomendasi Next Step

1. **system-analyst**: Buat OpenAPI CRUD contract generik + state machine workflow. Prioritas: endpoint `/submit`, `/review`, `/approve`, `/reject` dengan SoD check. Lihat api-conventions.md untuk envelope format.

2. **system-analyst**: Konfirmasi pertanyaan terbuka nomor 1, 2, 3, 6 di atas bersama stakeholder.

3. **data-modeler**: Verifikasi tabel `sys.workflow_config` sudah ada di schema Phase 2; jika belum, buat migration untuk generic workflow config table.

4. **ifrs9-compliance-reviewer**: HARUS di-dispatch untuk story VARIAN-A (ECL param masters) sebelum implementasi backend.

5. **security-engineer**: HARUS di-dispatch untuk story VARIAN-B (counterparty PII) sebelum implementasi backend.

6. **Pilot `mst.mata_uang`**: Implementasi story konkret APP-A-MSTR-002 sebagai first implementation — validasi pola generik ini berfungsi sebelum replikasi ke 15 modul lain.
