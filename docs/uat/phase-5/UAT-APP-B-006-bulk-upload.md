# UAT APP-B-006 — Bulk Upload Master Instrumen
**Module**: APP-B Transaction Lifecycle — P5-M11
**Version**: 1.0 | **Tanggal**: 2026-06-21 | **Penulis**: qa-engineer
**Story refs**: S1–S5 (P5-M11-bulk-upload.md) | **AC count**: 20

---

## Ringkasan

Skenario UAT ini memvalidasi fitur bulk upload master instrumen (deposito, obligasi, saham, reksadana, tabungan/cash) menggunakan XLSX multi-sheet, termasuk:

- Upload + parse XLSX (S1)
- 4-stage DRY_RUN validation pipeline (S2)
- Async commit via Asynq job + SSE progress (S3)
- 4-eyes approve dengan SoD (S4)
- CFO rollback dalam grace window + step-up MFA (S5)

---

## Prasyarat

| Item | Nilai |
|---|---|
| Environment | UAT |
| User Maker | `umaker@tugu-re.com` (ROLE-MAKER-TR) |
| User Approver | `uappr@tugu-re.com` (ROLE-APPR-TR) |
| User CFO | `ucfo@tugu-re.com` (ROLE-CFO, MFA aktif) |
| User Risk | `urisk@tugu-re.com` (ROLE-RISK) |
| User IT Admin | `uitadmin@tugu-re.com` (ROLE-IT-ADMIN) |
| User Audit | `uaudit@tugu-re.com` (ROLE-AUDIT, read-only) |
| File template | `template_instrumen_bulk_v1.xlsx` (tersedia di MinIO `uploads/templates/`) |
| File test 350 baris | `instrumen_bulk_350rows_uat.xlsx` (test artifact) |
| File test > 50MB | `instrumen_bulk_oversized.xlsx` (55MB, test artifact) |
| File test CSV | `instrumen_bulk.csv` (MIME invalid test) |
| Periode buku | Juni 2026 (status OPEN) |
| Grace window default | 7 hari (`sys.config_param.BULK_ROLLBACK_GRACE_DAYS = 7`) |

---

## TC-B006-01 — Upload file valid 350 baris, format XLSX

**Story/AC**: S1-AC1
**Executor**: Maker
**Pre-kondisi**: Login sebagai `umaker`, periode Juni 2026 OPEN

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Buka `/master/instrumen/bulk-upload` | — | Halaman upload muncul dengan dropzone |
| 2 | Drag-and-drop `instrumen_bulk_350rows_uat.xlsx` ke dropzone | File 12MB | Nama file dan ukuran tampil di dropzone |
| 3 | Klik tombol **Upload** | — | Spinner tampil, tombol disabled |
| 4 | Tunggu respons | — | Toast hijau: "Batch BULK-XXXX berhasil diupload. 350 baris terdeteksi." |
| 5 | Cek tabel riwayat upload | — | Baris baru muncul dengan status badge **Terurai** (PARSED) dan totalRows = 350 |
| 6 | Klik link batch ID | — | Navigasi ke halaman detail batch |
| 7 | Cek breakdown sheet | — | Sheet breakdown: Deposito/Obligasi/Saham/Reksadana/Tabungan_Cash semua tampil |

**Pass criteria**: Status PARSED, totalRows = 350, audit `BULK.UPLOADED` tercatat, `tenant_id = 'TUGURE'`.

---

## TC-B006-02 — Upload file > 50MB ditolak

**Story/AC**: S1-AC2
**Executor**: Maker
**Pre-kondisi**: Login sebagai `umaker`

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Buka halaman upload | — | Dropzone tampil; ada catatan "Maks. 50 MB" |
| 2 | Pilih `instrumen_bulk_oversized.xlsx` (55MB) | File 55MB | Client hint: ukuran file melebihi limit |
| 3 | Klik Upload | — | Toast merah persistent: "BULK_FILE_TOO_LARGE: file 55MB melebihi batas 50MB" |
| 4 | Cek tabel riwayat | — | Tidak ada batch baru (tidak ada INSERT) |

**Pass criteria**: HTTP 413, pesan spesifik, tidak ada batch tercatat.

---

## TC-B006-03 — Upload file CSV (bukan XLSX) ditolak

**Story/AC**: S1-AC3
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Drag-and-drop `instrumen_bulk.csv` | File CSV | Dropzone menampilkan peringatan "Hanya file .xlsx yang diterima" |
| 2 | Klik Upload (jika masih bisa) | — | Toast merah: "BULK_MIME_INVALID: file bukan XLSX ZIP archive" |
| 3 | Cek server log | — | Server membaca 4 magic bytes, bukan `PK\x03\x04` → reject |

**Pass criteria**: HTTP 415, magic bytes check terbukti (cek server log traceId).

---

## TC-B006-04 — Parse error: beberapa baris invalid format, batch tetap PARSED

**Story/AC**: S1-AC4
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Upload file dengan 2 baris format salah (kupon kosong, saldo negatif) | File 348 valid + 2 invalid | Toast: "350 baris terdeteksi, 2 memiliki error format" |
| 2 | Cek status batch | — | Status = **Terurai** (PARSED), bukan FAILED |
| 3 | Buka detail batch → tab Baris | — | 2 baris dengan status FAILED + keterangan error field |
| 4 | Cek audit | — | `BULK.UPLOADED` tercatat, parse_error_count = 2 |

**Pass criteria**: Batch PARSED (bukan terminal error), 2 baris RowStatus = FAILED, pesan error per-baris tersedia.

---

## TC-B006-05 — DRY_RUN: semua stage lulus, 3 baris FLAGGED

**Story/AC**: S2-AC1, S2-AC3
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Dari detail batch (status PARSED), klik **Jalankan DRY_RUN** | — | Request POST ke `/dry-run` |
| 2 | Tunggu hasil | — | Panel DRY_RUN muncul dengan 4 stage summary |
| 3 | Cek stage 1–4 | — | Stage 1: LULUS, Stage 2: LULUS, Stage 3: LULUS, Stage 4: LULUS (3 flagged) |
| 4 | Cek banner flagged | — | Banner kuning: "3 baris perlu review klasifikasi PSAK 71 manual" |
| 5 | Cek status batch | — | Status badge: **DRY_RUN Lulus** |
| 6 | Cek tombol Lanjut ke Commit | — | Tombol **Lanjut ke Commit** tampil (enabled) |
| 7 | Cek TTL | — | "DRY_RUN berlaku hingga [waktu + 1 jam]" tampil |

**Pass criteria**: Status DRY_RUN_PASSED, 3 baris FLAGGED_MANUAL_REVIEW, TTL tampil, tombol commit ada.

---

## TC-B006-06 — DRY_RUN: Stage 3 gagal, batch DRY_RUN_FAILED

**Story/AC**: S2-AC2
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Upload file dengan counterparty tidak ada di master | File dengan CP-999 tidak ada | Upload sukses (PARSED) |
| 2 | Klik Jalankan DRY_RUN | — | DRY_RUN diproses |
| 3 | Cek hasil | — | Panel: Stage 3 GAGAL, error "Counterparty CP-999 tidak ditemukan" |
| 4 | Cek status badge | — | **Validasi Gagal** (DRY_RUN_FAILED) |
| 5 | Cek tabel error per baris | — | Tabel tampilkan: Sheet, No. Baris, Stage, Kolom, Pesan Error |
| 6 | Cek tombol Lanjut ke Commit | — | Tidak tampil (absent from DOM) |

**Pass criteria**: Status DRY_RUN_FAILED, error table ada, tombol commit absent.

---

## TC-B006-07 — DRY_RUN: Stage 4 SPPI ambiguous → FLAGGED, bukan FAILED

**Story/AC**: S2-AC3
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Upload file dengan 1 baris obligasi kupon 13% (ambiguous SPPI Q7) | — | Upload sukses (PARSED) |
| 2 | Jalankan DRY_RUN | — | Stage 1-3 LULUS, Stage 4 memiliki 1 flagged |
| 3 | Cek status | — | **DRY_RUN_PASSED** (bukan FAILED) |
| 4 | Cek baris flagged | — | RowStatus = FLAGGED_MANUAL_REVIEW, flag_reason = "SPPI Q7 ambiguous" |

**Pass criteria**: DRY_RUN_PASSED (bukan FAILED), flagged ≠ invalid.

---

## TC-B006-08 — DRY_RUN TTL habis, commit ditolak

**Story/AC**: S2-AC4
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Buka batch dengan DRY_RUN_PASSED dari >1 jam lalu | — | Detail batch status DRY_RUN_PASSED (sudah expired di server) |
| 2 | Klik Enqueue Commit Job | — | Toast merah: "BULK_DRY_RUN_EXPIRED: DRY_RUN kadaluarsa. Jalankan ulang DRY_RUN." |
| 3 | Cek status batch | — | Status tidak berubah (tetap DRY_RUN_PASSED) |
| 4 | Klik Ulangi DRY_RUN | — | DRY_RUN baru berjalan, TTL di-reset |

**Pass criteria**: HTTP 422 BULK_DRY_RUN_EXPIRED, status tidak berubah, re-run DRY_RUN berhasil.

---

## TC-B006-09 — Commit: 350 baris berhasil, job progress SSE

**Story/AC**: S3-AC1
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Dari batch DRY_RUN_PASSED, buka halaman `/commit` | — | Halaman commit tampil stats batch |
| 2 | Klik **Enqueue Commit Job** | — | Toast biru: "Commit job enqueued — ID: job_XYZ" |
| 3 | Progress panel muncul | — | Progress bar 0%, status "Memulai commit..." |
| 4 | Pantau progress SSE | — | Progress naik bertahap, step "Memasukkan instrumen 50 dari 350..." |
| 5 | Job selesai | — | Toast hijau: "Commit selesai: 350 berhasil, 0 gagal." + link "Lihat detail" |
| 6 | Klik "Lihat detail" | — | Navigasi ke halaman detail batch, status = **COMMITTED** |
| 7 | Cek instrumen di grid | — | 350 instrumen status PENDING_APPROVAL_BULK |

**Pass criteria**: Status COMMITTED, 350 instrumen PENDING_APPROVAL_BULK, audit BULK.COMMITTED tercatat.

---

## TC-B006-10 — Commit parsial: 2 baris duplikat gagal, 348 berhasil

**Story/AC**: S3-AC2
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Upload file dengan 2 baris kode instrumen duplikat dari batch sebelumnya | — | Upload PARSED |
| 2 | DRY_RUN (stage 2 flagged, bukan failed untuk business logic) | — | DRY_RUN_PASSED |
| 3 | Commit | — | Job berjalan |
| 4 | Job selesai | — | Toast: "Commit selesai: 348 berhasil, 2 gagal." |
| 5 | Cek status batch | — | Status = **PARTIAL_COMMIT** |
| 6 | Cek baris gagal | — | 2 baris RowStatus = FAILED, error "duplikat kode instrumen" |

**Pass criteria**: Status PARTIAL_COMMIT, 348 COMMITTED + 2 FAILED, audit BULK.PARTIAL_COMMIT.

---

## TC-B006-11 — Commit diblok saat periode buku CLOSED

**Story/AC**: S3-AC3
**Executor**: Maker (IT-Admin tutup periode lebih dulu)

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | IT-Admin hard-close periode Juni 2026 | — | Periode CLOSED |
| 2 | Maker coba commit batch DRY_RUN_PASSED | — | Toast merah: "BULK_PERIODE_LOCKED: periode buku CLOSED, commit tidak dapat diproses" |
| 3 | Cek status batch | — | Status tidak berubah (tetap DRY_RUN_PASSED) |

**Pass criteria**: HTTP 423 BULK_PERIODE_LOCKED, batch tidak berubah.

---

## TC-B006-12 — Instrumen berstatus PENDING_APPROVAL_BULK sebelum approve

**Story/AC**: S3-AC4
**Executor**: Maker, kemudian cek sebagai Approver

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Setelah commit sukses, buka salah satu instrumen dari batch | — | Detail instrumen tampil |
| 2 | Cek field status | — | Status = "Menunggu Persetujuan Bulk" (PENDING_APPROVAL_BULK) |
| 3 | Coba edit instrumen | — | UI menampilkan ReadOnly; tombol edit disabled |
| 4 | Login sebagai Approver, cek instrumen yang sama | — | Status sama: PENDING_APPROVAL_BULK |

**Pass criteria**: Semua instrumen PENDING_APPROVAL_BULK, tidak bisa diedit sampai approve.

---

## TC-B006-13 — Approve batch: instrumen COMMITTED → ACTIVE; FLAGGED → PENDING_CLASSIFICATION

**Story/AC**: S4-AC1
**Executor**: Approver (berbeda dari Maker — SoD)

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `uappr`, buka detail batch (status COMMITTED) | — | Tombol **Setujui Batch** tampil |
| 2 | Klik Setujui Batch | — | Dialog approve muncul, tampilkan username Maker untuk konfirmasi SoD |
| 3 | Isi komentar ≥ 10 karakter | "Batch terverifikasi, semua data valid" | Komentar diterima |
| 4 | Klik Konfirmasi Persetujuan | — | Toast hijau: "Batch BULK-XXXX disetujui. 347 instrumen ACTIVE, 3 perlu klasifikasi manual." |
| 5 | Cek instrumen PENDING_APPROVAL_BULK | — | Status berubah → **ACTIVE** |
| 6 | Cek instrumen FLAGGED | — | Status = **PENDING_CLASSIFICATION** |
| 7 | Cek audit | — | `BULK.APPROVED` tercatat dengan approver_id + activated_count |

**Pass criteria**: 347 ACTIVE, 3 PENDING_CLASSIFICATION, audit BULK.APPROVED.

---

## TC-B006-14 — SoD: Maker tidak bisa approve batch sendiri

**Story/AC**: S4-AC2
**Executor**: Maker mencoba approve batch yang dibuat sendiri

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `umaker` | — | — |
| 2 | Buka detail batch yang dibuat sendiri (status COMMITTED) | — | Tombol **Setujui Batch** TIDAK tampil (absent from DOM) |
| 3 | Coba hit endpoint POST `/approve` langsung via API dengan JWT Maker | — | HTTP 403 BULK_APPROVE_SOD_VIOLATION |
| 4 | Cek audit | — | `BULK.SOD_VIOLATION_ATTEMPT` tercatat dengan actor = Maker |
| 5 | Cek status batch | — | Status tidak berubah |

**Pass criteria**: Tombol absent, API return 403, audit event SOD_VIOLATION_ATTEMPT tercatat.

---

## TC-B006-15 — Idempotency: approve dua kali dengan key yang sama

**Story/AC**: S4-AC3
**Executor**: Approver

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `uappr`, approve batch (Idempotency-Key: KEY-001) | KEY-001 | Approve sukses, HTTP 200 |
| 2 | Kirim request approve ulang dengan key sama dan body sama | KEY-001 | HTTP 200, response sama (IDEMPOTENCY_REPLAY) |
| 3 | Cek database | — | Hanya 1 event approve, tidak ada duplikat instrumen ACTIVE |
| 4 | Kirim request approve dengan key sama tapi body berbeda | KEY-001, body beda | HTTP 422 IDEMPOTENCY_MISMATCH |

**Pass criteria**: Replay return 200 dengan cached response, body beda return 422.

---

## TC-B006-16 — ROLE-RISK resolusi klasifikasi manual → ACTIVE

**Story/AC**: S4-AC4
**Executor**: Risk Officer

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `urisk`, buka instrumen berstatus PENDING_CLASSIFICATION | — | Detail instrumen tampil |
| 2 | Lihat flag_reason | — | "SPPI Q7 ambiguous — perlu review manual" |
| 3 | Pilih klasifikasi PSAK 71 = FVTPL | — | Form klasifikasi tampil |
| 4 | Submit klasifikasi | — | Toast hijau: "Klasifikasi FVTPL berhasil disimpan. Instrumen sekarang ACTIVE." |
| 5 | Cek status instrumen | — | Status = ACTIVE, klasifikasi_psak71 = FVTPL |

**Pass criteria**: Instrumen PENDING_CLASSIFICATION → ACTIVE setelah Risk submit klasifikasi.

---

## TC-B006-17 — CFO rollback dalam grace window (7 hari)

**Story/AC**: S5-AC1
**Executor**: CFO

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `ucfo`, buka detail batch (status APPROVED, committed 1 hari lalu) | — | Tombol **Ajukan Rollback** tampil |
| 2 | Klik Ajukan Rollback | — | Dialog rollback request muncul |
| 3 | Cek info grace window | — | "Grace window berakhir: [tanggal + 7 hari dari commit]" tampil |
| 4 | Isi alasan < 50 karakter | "Salah data" | Tombol disabled, error "minimal 50 karakter" |
| 5 | Isi alasan ≥ 50 karakter | "Error counterparty mapping ditemukan post-commit. Rollback untuk koreksi." | Counter: "74/50 karakter minimum" (hijau) |
| 6 | Klik Ajukan Rollback (confirm) | — | Toast info: "Rollback diminta. Menunggu konfirmasi MFA CFO." Status → ROLLBACK_PENDING |
| 7 | Klik **Konfirmasi Rollback (MFA)** | — | Dialog step-up MFA muncul |
| 8 | Isi step-up token (dari authenticator) | Token valid | Token diisi, tombol konfirmasi enabled |
| 9 | Klik Konfirmasi Rollback | — | Toast hijau: "Rollback berhasil. 350 instrumen dihapus (soft-delete)." |
| 10 | Cek instrumen | — | Semua instrumen dari batch: deleted_at terisi, tidak muncul di daftar aktif |
| 11 | Cek audit | — | `BULK.ROLLBACK_REQUESTED` + `BULK.ROLLBACK_APPROVED` keduanya tercatat (in-tx) |

**Pass criteria**: Status ROLLED_BACK, soft-delete (bukan hard-delete), 2 audit events in-tx.

---

## TC-B006-18 — Rollback ditolak karena grace window habis

**Story/AC**: S5-AC2
**Executor**: CFO

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Buka batch APPROVED yang committed 8 hari lalu (grace = 7 hari) | — | Detail batch tampil |
| 2 | Cek tombol Ajukan Rollback | — | Tombol TIDAK tampil ATAU disabled dengan keterangan "Grace window telah berakhir" |
| 3 | Coba hit endpoint POST `/rollback-request` langsung via API | — | HTTP 422 BULK_ROLLBACK_GRACE_EXPIRED |

**Pass criteria**: Grace window check di server-side, HTTP 422, pesan jelas.

---

## TC-B006-19 — Rollback approve tanpa step-up MFA ditolak

**Story/AC**: S5-AC3
**Executor**: CFO

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `ucfo`, batch status ROLLBACK_PENDING | — | Tombol Konfirmasi Rollback (MFA) tampil |
| 2 | Klik Konfirmasi Rollback (MFA) | — | Dialog muncul, field `#step-up-token` visible |
| 3 | Biarkan step-up token kosong | — | Tombol Konfirmasi Rollback disabled |
| 4 | Coba POST `/rollback-approve` tanpa header `X-Step-Up-Token` via API | — | HTTP 403 FORBIDDEN: "Rollback memerlukan step-up MFA (scope=bulk_rollback)" |
| 5 | Coba dengan step-up token kedaluwarsa (> 5 menit) | — | HTTP 403 FORBIDDEN: "step-up token expired" |

**Pass criteria**: Tombol disabled tanpa token, API 403 tanpa header, 403 dengan stale token.

---

## TC-B006-20 — IT-Admin update grace window config (non-retroaktif)

**Story/AC**: S5-AC4
**Executor**: IT-Admin

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `uitadmin`, buka halaman config params | — | `BULK_ROLLBACK_GRACE_DAYS` = 7 tampil |
| 2 | Update nilai ke 14 | 14 | Toast: "Config BULK_ROLLBACK_GRACE_DAYS diperbarui: 7 → 14" |
| 3 | Cek audit | — | `SYS.CONFIG_PARAM_UPDATED` tercatat dengan old_value=7, new_value=14 |
| 4 | Cek batch lama (committed 8 hari lalu, sebelum update) | — | Grace window tetap 7 hari (expired) — non-retroaktif |
| 5 | Upload + commit batch baru | — | Grace window = 14 hari (berlaku untuk batch baru) |
| 6 | Cek tombol Ajukan Rollback pada batch baru (10 hari post-commit) | — | Tombol masih aktif (belum expire, 10 < 14) |

**Pass criteria**: Config update non-retroaktif (batch lama pakai nilai lama), batch baru pakai nilai baru.

---

## TC-B006-X1 — ROLE-AUDIT: read-only, tidak ada tombol mutasi

**Story/AC**: Persona gating cross-cutting
**Executor**: Audit

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Login sebagai `uaudit` | — | — |
| 2 | Buka halaman upload | — | Dropzone tidak ada / disabled |
| 3 | Buka detail batch (COMMITTED) | — | Halaman tampil tapi semua tombol aksi absent from DOM (bukan disabled, tapi tidak ada) |
| 4 | Buka halaman DRY_RUN | — | Tombol "Ulangi DRY_RUN" absent from DOM |
| 5 | Cek audit log | — | ROLE-AUDIT bisa baca `aud.audit_log` |

**Pass criteria**: No mutation buttons rendered for ROLE-AUDIT (absent-from-DOM pattern).

---

## TC-B006-X2 — Export riwayat batch (sort + filter + export)

**Story/AC**: UX-§1 cross-cutting
**Executor**: Maker

| Langkah | Aksi | Data | Hasil yang Diharapkan |
|---|---|---|---|
| 1 | Buka `/master/instrumen/bulk-upload` | — | Tabel riwayat tampil |
| 2 | Klik header kolom "Status" | — | Sort aktif, icon panah muncul |
| 3 | Gunakan filter "Status = COMMITTED" | — | Hanya batch COMMITTED tampil, filter chip muncul |
| 4 | Klik Export → CSV | — | File CSV diunduh, hanya batch yang difilter |
| 5 | Cek audit | — | `BULK_BATCH.EXPORT` tercatat |

**Pass criteria**: Sort/filter/export bekerja, state di URL (deep-link), audit export tercatat.

---

## Checklist Sign-off

| Test Case | Executor | Tgl | Hasil | Catatan |
|---|---|---|---|---|
| TC-B006-01 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-02 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-03 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-04 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-05 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-06 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-07 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-08 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-09 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-10 | Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-11 | IT-Admin + Maker | | ☐ PASS ☐ FAIL | |
| TC-B006-12 | Maker + Approver | | ☐ PASS ☐ FAIL | |
| TC-B006-13 | Approver | | ☐ PASS ☐ FAIL | |
| TC-B006-14 | Maker (SoD test) | | ☐ PASS ☐ FAIL | |
| TC-B006-15 | Approver | | ☐ PASS ☐ FAIL | |
| TC-B006-16 | Risk Officer | | ☐ PASS ☐ FAIL | |
| TC-B006-17 | CFO | | ☐ PASS ☐ FAIL | |
| TC-B006-18 | CFO | | ☐ PASS ☐ FAIL | |
| TC-B006-19 | CFO | | ☐ PASS ☐ FAIL | |
| TC-B006-20 | IT-Admin | | ☐ PASS ☐ FAIL | |
| TC-B006-X1 | Audit | | ☐ PASS ☐ FAIL | |
| TC-B006-X2 | Maker | | ☐ PASS ☐ FAIL | |

**Persetujuan UAT**: __________________ Tanggal: __________
**QA Engineer**: __________________ Tanggal: __________
