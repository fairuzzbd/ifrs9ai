# UAT-APP-B-005 — Jatuh Tempo + Pendapatan Akrual Harian

**Modul:** APP-B Transaction Lifecycle
**Fase:** Phase 5 — Milestone 9
**Versi dokumen:** 1.0
**Tanggal:** 2026-06-20
**Author:** qa-engineer (P5-M9)
**Referensi:** P5-M9-jatuh-tempo-akrual.md, FSD-APP-B v1.1 §9-10, SoW_v1.4 §4

---

## Lingkup

Menguji 20 Acceptance Criteria dari 5 User Story P5-M9:
- S1: Jatuh Tempo — Settlement maturity event
- S2: Akrual Bunga Harian — Stage-aware accrual (PSAK 71 §5.4.1(b))
- S3: Dividen + Distribusi Reksadana — 4-eyes workflow
- S4: Amortisasi Premium/Diskon — EIR-based amortization
- S5: Monitoring Akrual — List, Dashboard, Stale staging

---

## Prasyarat

1. Environmen UAT hidup (`docker compose up`, seed data P5-M9 tersedia).
2. User UAT tersedia untuk setiap persona: `uat.maker`, `uat.reviewer`, `uat.akun`, `uat.akun.ctl`, `uat.it.admin`.
3. Seed data: DEP-0055 (Deposito AC, jatuh tempo 2026-06-20, gross IDR 5B, EIR 6.39%), OBL-0101 (Bond AC, Stage 1, gross IDR 10B, EIR 7.5%), OBL-0202 (Bond AC, Stage 3, gross IDR 8B, ECL sealed IDR 2.4B, EIR 9%), BOND-USD-003 (Bond USD, Stage 1, FCY USD 5.000.000, EIR 5%, FX 16.200), SAHAM-FVTPL-001 (Saham FVTPL), RF-001 (Reksadana FVTPL).
4. Kalender libur: 2026-06-17 ditandai sebagai hari libur nasional.
5. sys.parameter: AKRUAL_STAGING_STALE_DAYS = 30.

---

## Skenario UAT

### TC-B005-01 — Deposito Jatuh Tempo: Settlement dengan PPh 20%

**Referensi:** S1-AC1
**Persona:** ROLE-IT-ADMIN (trigger cron), ROLE-AKUN (verifikasi)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Login sebagai `uat.it.admin`.
2. Navigasi ke `/transaksi/jatuh-tempo`.
3. Klik tombol "Trigger Maturity Cron" (hanya tampil untuk ROLE-IT-ADMIN).
4. Konfirmasi dialog. Amati JobProgressPanel muncul dengan SSE progress.
5. Tunggu job selesai (status "completed").
6. Refresh halaman `/transaksi/jatuh-tempo`.
7. Cari DEP-0055 (tanggal 2026-06-20).

**Hasil yang diharapkan:**

- Baris DEP-0055 muncul dengan status badge "DISELESAIKAN" (hijau).
- Kolom "Pokok IDR" = Rp 5.000.000.000,0000.
- Kolom "PPh IDR" ≈ Rp 17.534,2466 (20% dari bunga last).
- Kolom "Net Kas IDR" = Pokok + Bunga Last − PPh (tampilkan 4 desimal).
- Link jurnal tersedia ("Lihat Jurnal") → navigasi ke halaman jurnal terkait.
- Audit log: `MATURITY.DERECOGNIZED` dengan `after_jsonb` berisi `net_kas_idr`.

---

### TC-B005-02 — Bond Jatuh Tempo At Par: Realized G/L = 0

**Referensi:** S1-AC2
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Pastikan OBL-0099 seed: Bond AC, pokok = IDR 10B, gross carrying = IDR 10B (at par).
2. Trigger maturity cron untuk tanggal OBL-0099 jatuh tempo.
3. Navigasi ke detail jatuh tempo OBL-0099.

**Hasil yang diharapkan:**

- Status = DISELESAIKAN.
- Kolom "Realized G/L" = 0,0000.
- PPh = 0 (bond tidak kena PPh final atas pokok).
- Net Kas = IDR 10B + bunga last.

---

### TC-B005-03 — Instrumen TIDAK AKTIF: DLQ dan Batch Lanjut

**Referensi:** S1-AC3
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Pastikan DEP-0060 dalam status "DISPOSED" (bukan ACTIVE).
2. Set tanggal jatuh tempo DEP-0060 = hari ini.
3. Trigger maturity cron.
4. Setelah selesai, cek halaman DLQ (`/jobs` → DLQ tab) atau endpoint `GET /api/v1/sys/dlq`.

**Hasil yang diharapkan:**

- DEP-0060 tidak muncul sebagai SETTLED di list jatuh tempo.
- DLQ berisi entry dengan `error_code = MATURITY_INSTRUMEN_NOT_ACTIVE`, `instrumen_id = DEP-0060`.
- Instrumen lain yang jatuh tempo hari ini tetap diproses (batch tidak berhenti).
- Toast error muncul di UI untuk DEP-0060 dengan kode `MATURITY_INSTRUMEN_NOT_ACTIVE`.

---

### TC-B005-04 — Hari Libur: MATURITY.HOLIDAY_SKIP

**Referensi:** S1-AC4
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Pastikan ada instrumen dengan tanggal jatuh tempo = 2026-06-17 (hari libur).
2. Set tanggal sistem UAT ke 2026-06-17 (atau jalankan cron dengan parameter tanggal).
3. Trigger maturity cron.
4. Cek list jatuh tempo dan audit log.

**Hasil yang diharapkan:**

- Tidak ada baris baru dengan status SETTLED untuk 2026-06-17.
- Audit log berisi `MATURITY.HOLIDAY_SKIP` dengan `tanggal = 2026-06-17`.
- Instrumen tetap ACTIVE (belum di-settle).
- Tidak ada DLQ error (skip bukan error).

---

### TC-B005-05 — Akrual Harian Stage 1: Gross × EIR / 365

**Referensi:** S2-AC1
**Persona:** ROLE-IT-ADMIN (trigger), ROLE-AKUN (verifikasi)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Trigger akrual cron harian untuk OBL-0101.
2. Navigasi ke `/transaksi/akrual`.
3. Filter: Instrumen = OBL-0101, Tanggal = hari ini.

**Hasil yang diharapkan:**

- Baris OBL-0101 muncul dengan status AUTO_POSTED.
- Kolom "Basis" = GROSS.
- Kolom "Stage" = 1.
- Kolom "Akrual IDR" ≈ Rp 2.054.794,5205 (IDR 10B × 7.5% / 365).
- Kolom "Carrying IDR" = IDR 10.000.000.000,0000.
- Tidak ada `stale_staging_flag`.

---

### TC-B005-06 — Akrual Harian Stage 3: Net Carrying (Gross − ECL)

**Referensi:** S2-AC2
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P0 - Kritis (PSAK 71 §5.4.1(b))

**Langkah pengujian:**

1. Pastikan OBL-0202: gross = IDR 8B, ECL sealed = IDR 2.4B, stage = 3, EIR = 9%.
2. Trigger akrual cron harian.
3. Buka detail akrual OBL-0202 hari ini.

**Hasil yang diharapkan:**

- Kolom "Basis" = NET_CARRYING (ditampilkan merah di UI).
- Kolom "Carrying IDR" = IDR 5.600.000.000,0000 (8B − 2.4B).
- Kolom "Akrual IDR" ≈ IDR 1.380.821,9178 (5.6B × 9% / 365).
- Tooltip menampilkan: Gross = 8B, ECL = 2.4B, Net = 5.6B.
- Status = AUTO_POSTED.
- `ecl_run_id_used` diisi (referensi ke sealed run).
- Badge "Stage 3" merah, basis merah NET_CARRYING.

---

### TC-B005-07 — Stage 3 Tanpa ECL Sealed: PENDING_STALE_REVIEW

**Referensi:** S2-AC2 (edge case)
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN-CTL
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Hapus referensi ECL sealed untuk OBL-0202 (atau gunakan instrumen baru tanpa sealed run).
2. Trigger akrual cron.
3. Cek list akrual.

**Hasil yang diharapkan:**

- Baris OBL-0202 muncul dengan status PENDING_STALE_REVIEW.
- Badge "STAGING STALE" amber muncul di kolom.
- Banner peringatan di atas tabel: "X instrumen memiliki staging stale".
- DLQ entry: `AKRUAL_STAGING_STALE`.

---

### TC-B005-08 — Akrual FCY: Konversi × FX Rate APPROVED

**Referensi:** S2-AC3
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Pastikan FX Rate IDR/USD hari ini di-approve oleh ROLE-AKUN: 16.200.
2. Trigger akrual cron untuk BOND-USD-003.
3. Buka detail akrual BOND-USD-003.

**Hasil yang diharapkan:**

- Kolom "Mata Uang" = USD.
- Kolom "Akrual FCY" ≈ USD 684,9315 (5.000.000 × 5% / 365).
- Kolom "Akrual IDR" = FCY × 16.200 (4 desimal).
- `fx_rate_id` diisi.

---

### TC-B005-09 — FX Rate Belum Ada: DLQ AKRUAL_FX_RATE_MISSING

**Referensi:** S2-AC3 (edge case)
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Hapus/batalkan FX Rate IDR/USD untuk tanggal hari ini.
2. Trigger akrual cron.

**Hasil yang diharapkan:**

- BOND-USD-003 tidak menghasilkan akrual row.
- DLQ: `AKRUAL_FX_RATE_MISSING` untuk BOND-USD-003.
- Instrumen IDR lain tetap diproses normal.
- Toast error muncul untuk BOND-USD-003 saja.

---

### TC-B005-10 — Duplikat Akrual: Idempotency Guard

**Referensi:** S2-AC4
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Trigger akrual cron — berhasil (AUTO_POSTED untuk OBL-0101).
2. Trigger akrual cron lagi di hari yang sama tanpa clear data.

**Hasil yang diharapkan:**

- Tidak ada baris duplikat `(OBL-0101, hari ini, BUNGA)`.
- DLQ: `AKRUAL_DUPLICATE` untuk OBL-0101 pada trigger kedua.
- Total akrual tidak berlipat ganda.
- DB unique constraint `(instrumen_id, tanggal_akrual, jenis)` tidak di-violated.

---

### TC-B005-11 — Dividen FVTPL: Approve PPh 10%

**Referensi:** S3-AC1
**Persona:** ROLE-MAKER-TR (input), ROLE-APPR-TR (approve)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Login `uat.maker`. Navigasi ke "Input Dividen" (atau POST `/api/v1/transaksi/dividen`).
2. Input: Instrumen = SAHAM-FVTPL-001, Gross Dividen = IDR 50.000.000.
3. Submit. Catat ID dividen.
4. Login `uat.reviewer`. Navigasi ke queue pending approval.
5. Buka dividen, klik "Approve".

**Hasil yang diharapkan:**

- Setelah submit (step 3): status = PENDING_APPROVAL, PPh = IDR 5.000.000 (10%), Net = IDR 45.000.000.
- Toast sukses Maker: "Dividen SAHAM-FVTPL-001 berhasil disubmit. Menunggu approval."
- Setelah approve (step 5): status = POSTED.
- Audit: `DIVIDEN.CREATED` + `DIVIDEN.POSTED`.
- Jurnal posting ke P&L.

---

### TC-B005-12 — Distribusi Reksadana: is_reksadana = TRUE

**Referensi:** S3-AC2
**Persona:** ROLE-MAKER-TR, ROLE-APPR-TR
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Input distribusi untuk RF-001 (Reksadana): Gross = IDR 12.000.000.
2. Submit + Approve.

**Hasil yang diharapkan:**

- PPh = IDR 1.200.000 (10%), Net = IDR 10.800.000.
- Audit `after_jsonb` berisi `"is_reksadana": true`.
- Label di UI: "Distribusi Reksadana" (bukan "Dividen Saham").

---

### TC-B005-13 — SoD Dividen: Maker Tidak Bisa Approve Sendiri

**Referensi:** S3-AC3
**Persona:** ROLE-MAKER-TR (sebagai approver yang salah)
**Prioritas:** P0 - Kritis (DEC-017)

**Langkah pengujian:**

1. `uat.maker` submit dividen (Gross = IDR 50.000.000).
2. `uat.maker` mencoba approve dividen yang sama via API: `POST /api/v1/transaksi/dividen/{id}/approve`.

**Hasil yang diharapkan:**

- HTTP 403.
- Response body: `{ "error": { "code": "SOD_VIOLATION", ... } }`.
- Status dividen tetap PENDING_APPROVAL.
- Audit log mencatat percobaan gagal.
- Toast merah: "Anda tidak bisa menjadi reviewer/approver untuk data yang Anda buat sendiri (Segregation of Duties)."

---

### TC-B005-14 — Gross Dividen ≤ 0: DIVIDEN_VALIDATION_FAILED

**Referensi:** S3-AC4
**Persona:** ROLE-MAKER-TR
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. POST `/api/v1/transaksi/dividen` dengan `gross_dividen_idr = 0`.

**Hasil yang diharapkan:**

- HTTP 422.
- Response: `{ "error": { "code": "DIVIDEN_VALIDATION_FAILED", ... } }`.
- Tidak ada row dividen di-insert.
- Form UI: field "Gross Dividen" di-highlight merah dengan pesan validasi inline.

---

### TC-B005-15 — Amortisasi Premium Bond AC: Carrying Turun

**Referensi:** S4-AC1
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Trigger `AMORTISASI_PD_JOB` untuk bond AC premium (dibeli di atas par).
2. Cek `/transaksi/akrual` dengan filter jenis = AMORTISASI_PREMIUM.

**Hasil yang diharapkan:**

- Baris muncul dengan jenis = AMORTISASI_PREMIUM, basis = GROSS.
- Amortisasi harian non-negatif.
- Gross carrying bond berkurang sesuai amortisasi.
- Jurnal: Dr Beban Premium / Cr Aset Obligasi.
- `amortisasi_schedule` row TIDAK diupdate — hanya dipakai (DEC-013).

---

### TC-B005-16 — Amortisasi Diskon Bond FVOCI: Carrying Naik

**Referensi:** S4-AC2
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Trigger amortisasi untuk bond FVOCI diskon (dibeli di bawah par).
2. Cek list akrual filter jenis = AMORTISASI_DISKON.

**Hasil yang diharapkan:**

- Jenis = AMORTISASI_DISKON.
- Gross carrying naik sesuai amortisasi.
- Jurnal: Dr Aset Obligasi / Cr Pendapatan Bunga.

---

### TC-B005-17 — POCI: Credit-Adjusted EIR

**Referensi:** S4-AC3
**Persona:** ROLE-IT-ADMIN, ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Siapkan instrumen POCI dengan `credit_adjusted_eir = 4.5%`, gross EIR = 6.5%.
2. Trigger amortisasi cron.
3. Verifikasi akrual yang dihitung.

**Hasil yang diharapkan:**

- Akrual menggunakan `credit_adjusted_eir = 0.04500000` (bukan gross EIR).
- Audit `after_jsonb` berisi `"is_poci": true`, `"eir_used": "0.04500000"`.

---

### TC-B005-18 — Schedule EIR Tidak Ada: DLQ AKRUAL_EIR_NOT_FOUND

**Referensi:** S4-AC4
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Hapus / expire semua `amortisasi_schedule` row untuk instrumen OBL-0505 (set `effective_to = now()`).
2. Trigger amortisasi cron.

**Hasil yang diharapkan:**

- OBL-0505 tidak menghasilkan akrual amortisasi.
- DLQ: `AKRUAL_EIR_NOT_FOUND` untuk OBL-0505.
- Instrumen lain yang memiliki schedule aktif tetap diproses.

---

### TC-B005-19 — List Akrual: Filter + Sort + Pagination

**Referensi:** S5-AC1
**Persona:** ROLE-AKUN
**Prioritas:** P1 - Tinggi

**Langkah pengujian:**

1. Navigasi `/transaksi/akrual`.
2. Aktifkan filter: Stage = 3.
3. Sort kolom "Akrual IDR" descending.
4. Klik Next page beberapa kali.
5. Klik Export → CSV.

**Hasil yang diharapkan:**

- Hanya baris Stage 3 tampil (dengan basis NET_CARRYING).
- Sort berjalan: baris pertama = Akrual IDR terbesar.
- Pagination: "Page X of ~Y", tombol Prev/Next berfungsi.
- URL berubah: `?filter[stage]=3&sort=akrual_idr:desc&cursor=...` (deep-link friendly).
- CSV ter-download, respek filter aktif, header Bahasa Indonesia, format NUMERIC(20,4).
- Audit log: `AKRUAL.EXPORT`.

---

### TC-B005-20 — Override Stale: ROLE-AKUN-CTL, Alasan ≥ 30 Karakter

**Referensi:** S5-AC4
**Persona:** ROLE-AKUN-CTL, ROLE-AKUN (negative)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Pastikan ada akrual dengan `stale_staging_flag = TRUE`, status = PENDING_STALE_REVIEW.
2. Login `uat.akun` (non-CTL). Buka akrual tersebut.
3. Verifikasi tombol "Override Stale" **tidak tampil** di DOM.
4. Login `uat.akun.ctl`. Buka akrual yang sama.
5. Klik "Override Stale".
6. Isi alasan kurang dari 30 karakter → tombol "Konfirmasi" tetap disabled.
7. Isi alasan ≥ 30 karakter.
8. Klik "Konfirmasi".

**Hasil yang diharapkan:**

- Step 3: tombol Override tidak ada di DOM (absent-from-DOM, bukan hanya disabled).
- Step 6: submit button disabled, error inline "Alasan minimal 30 karakter".
- Step 7: tombol Konfirmasi enabled.
- Step 8: HTTP 200 dengan `{ status: "POSTED" }`.
- Toast hijau: "Akrual [kode] berhasil diposting. Jurnal entry: [link]."
- Status akrual berubah ke POSTED di list.
- Audit log: `AKRUAL.POSTED_OVERRIDE` dengan `after_jsonb.reason` berisi alasan.
- Signature method = `JWT_STEP_UP` di audit.

---

## Kriteria Kelulusan

| # | Skenario | Status |
|---|---|---|
| 01 | Deposito jatuh tempo PPh 20% | - |
| 02 | Bond at par G/L = 0 | - |
| 03 | NOT ACTIVE DLQ + batch lanjut | - |
| 04 | Holiday SKIP audit | - |
| 05 | Stage 1 akrual GROSS × EIR/365 | - |
| 06 | Stage 3 NET_CARRYING (Gross − ECL) | - |
| 07 | Stage 3 tanpa ECL → PENDING_STALE_REVIEW | - |
| 08 | FCY akrual × FX rate | - |
| 09 | FX missing → DLQ | - |
| 10 | Duplikat akrual → idempotency guard | - |
| 11 | Dividen FVTPL PPh 10% 4-eyes | - |
| 12 | Distribusi Reksadana is_reksadana | - |
| 13 | SoD dividen → SOD_VIOLATION | - |
| 14 | Gross dividen ≤ 0 → VALIDATION_FAILED | - |
| 15 | Amortisasi premium carrying turun | - |
| 16 | Amortisasi diskon carrying naik | - |
| 17 | POCI credit-adjusted EIR | - |
| 18 | Missing schedule → DLQ | - |
| 19 | List filter + sort + pagination + export | - |
| 20 | Override stale AKUN-CTL ≥ 30 char | - |

**Kelulusan UAT:** Semua 20 TC P0 dan P1 harus hijau. Zero defect P0. Defect P1 maksimum 2 (dengan workaround terdokumentasi).

---

## Catatan Defect

| ID | TC | Deskripsi | Severity | Status |
|---|---|---|---|---|
| - | - | - | - | - |

---

## Sign-off

| Role | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| QA Engineer | | | |
| Finance Controller | | | |
| IFRS9 Compliance Reviewer | | | |
