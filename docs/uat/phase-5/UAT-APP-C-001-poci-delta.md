# UAT-APP-C-001 — POCI Delta ECL

**Modul:** APP-C ECL Engine
**Fase:** Phase 5 — Milestone 10
**Versi dokumen:** 1.0
**Tanggal:** 2026-06-21
**Author:** qa-engineer (P5-M10)
**Referensi:** P5-M10-poci-delta.md, FSD-APP-C-ECL-EIR-v1.0 §5, SoW_v1.4 §4, PSAK 71 §5.5.13-14, DEC-010/016/017/018/021/022

---

## Lingkup

Menguji 20 Acceptance Criteria dari 5 User Story P5-M10:

- S1: Penangkapan Baseline POCI — WORM write, penolakan overwrite
- S2: Komputasi Delta ECL — direction enum, idempotency, missing baseline handling
- S3: Pencatatan Jurnal P&L — INCREASE/DECREASE/ZERO routing, periode lock
- S4: Penghapusan Warning Lama — no stale warning on M10 engine, legacy runs immutable
- S5: Monitoring & Dashboard — history, summary MTD/YTD, large delta alert, export

---

## Prasyarat

1. Lingkungan UAT hidup (`docker compose up`, seed data P5-M10 tersedia).
2. User UAT tersedia per persona:
   - `uat.maker` (ROLE-MAKER-TR)
   - `uat.reviewer` (ROLE-APPR-TR)
   - `uat.risk` (ROLE-RISK)
   - `uat.akun` (ROLE-AKUN)
   - `uat.akun.ctl` (ROLE-AKUN-CTL)
   - `uat.it.admin` (ROLE-IT-ADMIN)
   - `uat.audit` (ROLE-AUDIT)
3. Seed data:
   - **POCI-DEP-0001** (Deposito POCI, is_poci=TRUE, baseline IDR 1.250.000.000,0000, credit_adjusted_eir 0.04500000, approved)
   - **POCI-OBL-0002** (Obligasi POCI, is_poci=TRUE, baseline IDR 800.000.000,0000, EIR 0.06000000)
   - **POCI-OBL-NOBSL** (Obligasi POCI, is_poci=TRUE, **tanpa** baseline — skenario missing)
   - **DEP-0099** (Deposito non-POCI, is_poci=FALSE)
   - **CALC-RUN-M10-001** (Calc run ECL Juni 2026, status=RUNNING, tipe includes POCI)
   - `sys.parameter`: `POCI_LARGE_DELTA_THRESHOLD` = 500000000
   - Periode Juni 2026 dalam status OPEN
   - Periode Mei 2026 dalam status CLOSED
4. Seed jurnal mapping: event code `POCI_ECL_DELTA_INCREASE` dan `POCI_ECL_DELTA_DECREASE` terdaftar di `jrnl.mapping_jurnal`.
5. DB trigger `trg_poci_baseline_worm` aktif (reject UPDATE dan DELETE pada `ecl.poci_baseline`).

---

## Skenario UAT

### TC-C001-01 — Baseline POCI Ditangkap Saat Approve Penempatan (S1-AC1)

**Referensi:** S1-AC1
**Persona:** ROLE-MAKER-TR (maker), ROLE-APPR-TR (approver), ROLE-AUDIT (verifikasi)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Login sebagai `uat.maker`.
2. Buat penempatan baru untuk instrumen POCI-DEP-NEW (is_poci=TRUE) melalui `/transaksi/penempatan/baru`.
3. Isi semua field wajib. Submit → status menjadi MENUNGGU_REVIEW.
4. Login sebagai `uat.reviewer`. Navigasi ke antrian review. Klik Review → Setuju.
5. Login sebagai `uat.it.admin` atau user dengan permission `poci.baseline.read`.
6. Navigasi ke `/poci/baseline`. Cari instrumen baru.

**Hasil yang diharapkan:**

- Baris baseline muncul dengan kolom: Kode Instrumen, Tgl Baseline, Lifetime ECL Origination (IDR), Credit-Adjusted EIR (%), badge WORM (immutable).
- `lifetime_ecl_at_origination` presisi 4 desimal.
- `credit_adjusted_eir` tampil sebagai persentase dengan 4 desimal (misal: 4.5000%).
- Audit log: aksi `POCI.BASELINE_CAPTURED`, entity_type = `ecl.poci_baseline`.

---

### TC-C001-02 — Baseline POCI Tidak Bisa Di-Overwrite (S1-AC2)

**Referensi:** S1-AC2
**Persona:** ROLE-IT-ADMIN (attempt via API), ROLE-AUDIT (verifikasi)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Via API (Swagger/Postman): `POST /api/v1/poci/baseline` dengan `instrumen_id = POCI-DEP-0001` (sudah ada baseline) + Idempotency-Key baru.
2. Kirim request.

**Hasil yang diharapkan:**

- HTTP 422, body:
  ```json
  { "error": { "code": "POCI_BASELINE_IMMUTABLE_VIOLATION", ... } }
  ```
- Row baseline POCI-DEP-0001 tidak berubah (lifetime_ecl tetap IDR 1.250.000.000,0000).
- Audit log: `POCI.BASELINE_VIOLATION_ATTEMPT` tercatat (bukan baseline baru).

---

### TC-C001-03 — Non-POCI Instrumen Tidak Menghasilkan Baseline (S1-AC3)

**Referensi:** S1-AC3
**Persona:** ROLE-AUDIT (verifikasi)
**Prioritas:** P1

**Langkah pengujian:**

1. Cek bahwa DEP-0099 (is_poci=FALSE) tidak memiliki baseline.
2. Navigasi ke `/poci/baseline`. Filter atau cari kode "DEP-0099".

**Hasil yang diharapkan:**

- Tidak ada baris baseline untuk DEP-0099.
- `POST /api/v1/poci/baseline` dengan instrumen_id = DEP-0099 → HTTP 422 `POCI_INSTRUMEN_NOT_POCI`.

---

### TC-C001-04 — ROLE-AUDIT Baca Baseline; ROLE-AKUN Tidak Bisa Update (S1-AC4)

**Referensi:** S1-AC4
**Persona:** ROLE-AUDIT, ROLE-AKUN
**Prioritas:** P1

**Langkah pengujian:**

1. Login sebagai `uat.audit`. Navigasi ke `/poci/baseline`. Klik baris POCI-DEP-0001.
2. Amati: tidak ada tombol Edit/Delete — hanya tombol "Lihat Detail".
3. Via API: `PATCH /api/v1/poci/baseline/{id}` dengan token `uat.audit`.
4. Login sebagai `uat.akun`. Navigasi ke `/poci/baseline`.
5. Via API: `GET /api/v1/poci/baseline` dengan token `uat.akun`.

**Hasil yang diharapkan:**

- `uat.audit` mendapat HTTP 200 untuk GET; HTTP 403 `FORBIDDEN` untuk PATCH.
- `uat.akun` mendapat HTTP 403 `FORBIDDEN` untuk GET (tidak punya `poci.baseline.read`).

---

### TC-C001-05 — Komputasi Delta POCI INCREASE (S2-AC1)

**Referensi:** S2-AC1
**Persona:** ROLE-RISK (trigger), ROLE-AUDIT (verifikasi)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Update current ECL POCI-DEP-0001 (dalam scope CALC-RUN-M10-001) ke IDR 1.450.000.000.
2. Login sebagai `uat.risk`. Navigasi ke `/poci/delta-log`.
3. Klik tombol "Trigger Komputasi Delta POCI" (hanya tampil untuk ROLE-RISK/ROLE-IT-ADMIN).
4. Konfirmasi dialog. Amati JobProgressPanel — SSE progress muncul.
5. Tunggu status job "completed".
6. Refresh `/poci/delta-log`. Cari POCI-DEP-0001.

**Hasil yang diharapkan:**

- Baris muncul dengan `delta_ecl` = +200.000.000,0000 (IDR, presisi 4).
- Badge direction: "Meningkat" (merah, icon TrendingUp).
- `stage_marker` pada result line = "POCI" (bukan 1/2/3).
- `prior_delta_cumulative` tampil jika ada riwayat sebelumnya.
- Audit: `POCI.DELTA_COMPUTED`.

---

### TC-C001-06 — Komputasi Delta POCI DECREASE (S2-AC2)

**Referensi:** S2-AC2
**Persona:** ROLE-RISK
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Update current ECL POCI-OBL-0002 ke IDR 650.000.000 (lebih kecil dari baseline IDR 800.000.000).
2. Trigger komputasi delta.
3. Refresh `/poci/delta-log`. Cari POCI-OBL-0002.

**Hasil yang diharapkan:**

- `delta_ecl` = −150.000.000,0000.
- Badge direction: "Menurun" (hijau, icon TrendingDown).
- Audit: `POCI.DELTA_COMPUTED`.

---

### TC-C001-07 — Baseline Tidak Ada → Error Log, Run Lanjut (S2-AC3)

**Referensi:** S2-AC3
**Persona:** ROLE-RISK, ROLE-IT-ADMIN
**Prioritas:** P1

**Langkah pengujian:**

1. POCI-OBL-NOBSL (tanpa baseline) di-include dalam CALC-RUN-M10-001.
2. Trigger komputasi delta.
3. Setelah job selesai, navigasi ke detail calc run → tab "Error Log".

**Hasil yang diharapkan:**

- Error log berisi entri: error_code = `POCI_BASELINE_MISSING`, instrumen = POCI-OBL-NOBSL.
- Instrumen lain (POCI-DEP-0001, POCI-OBL-0002) berhasil dihitung (bukan gagal semua).
- Job status = "completed" (bukan "failed") — run lanjut skip instrumen bermasalah.

---

### TC-C001-08 — Idempotency: Hitung Ulang Sama Calc Run Tidak Membuat Baris Duplikat (S2-AC4)

**Referensi:** S2-AC4
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Trigger komputasi delta CALC-RUN-M10-001 (sudah selesai skenario TC-05).
2. Trigger lagi dengan Idempotency-Key berbeda (atau API langsung).
3. Cek `/poci/delta-log` filter calc_run_id = CALC-RUN-M10-001.

**Hasil yang diharapkan:**

- Hanya 1 baris per (calc_run_id, instrumen_id) — tidak ada duplikat.
- Error log berisi entri `POCI_DELTA_DUPLICATE` untuk instrumen yang sudah dihitung.

---

### TC-C001-09 — Jurnal P&L INCREASE: Debet Beban / Kredit Cadangan (S3-AC1)

**Referensi:** S3-AC1
**Persona:** ROLE-RISK (verify), ROLE-AKUN-CTL (verify jurnal)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Setelah TC-05 (POCI-DEP-0001 INCREASE +200jt), baris status harus "POSTED".
2. Klik link "Lihat Jurnal" pada baris POCI-DEP-0001.

**Hasil yang diharapkan:**

- Jurnal header ada dengan event_code = `POCI_ECL_DELTA_INCREASE`.
- Baris jurnal:
  - Debet: Beban Penurunan Nilai ECL POCI, jumlah IDR 200.000.000,0000.
  - Kredit: Cadangan ECL POCI, jumlah IDR 200.000.000,0000.
- `debit = kredit` (balanced).
- Audit: `POCI.DELTA_POSTED`.

---

### TC-C001-10 — Jurnal P&L DECREASE: Debet Cadangan / Kredit Pendapatan (S3-AC2)

**Referensi:** S3-AC2
**Persona:** ROLE-RISK (verify), ROLE-AKUN-CTL (verify jurnal)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Setelah TC-06 (POCI-OBL-0002 DECREASE −150jt), klik "Lihat Jurnal" pada baris tersebut.

**Hasil yang diharapkan:**

- event_code = `POCI_ECL_DELTA_DECREASE`.
- Baris jurnal:
  - Debet: Cadangan ECL POCI, jumlah IDR 150.000.000,0000 (nilai absolut).
  - Kredit: Pendapatan Pemulihan ECL POCI, jumlah IDR 150.000.000,0000.
- Jumlah jurnal = |delta_ecl|, bukan delta_ecl negatif.

---

### TC-C001-11 — Delta Nol: Tidak Ada Jurnal, Status SKIPPED_ZERO (S3-AC3 ZERO)

**Referensi:** S3-AC3
**Persona:** ROLE-RISK
**Prioritas:** P1

**Langkah pengujian:**

1. Pastikan current ECL POCI-OBL-0002 sama persis dengan baseline (delta = 0).
2. Trigger komputasi. Cari baris di `/poci/delta-log`.

**Hasil yang diharapkan:**

- `delta_ecl` = 0,0000.
- Badge direction: "Tidak Berubah" (abu-abu, icon Minus).
- Badge status: "Dilewati (Nol)" (abu-abu).
- Tidak ada jurnal header terkait.
- Tidak ada entri `POCI.DELTA_POSTED` di audit untuk instrumen ini.

---

### TC-C001-12 — Periode CLOSED: Posting Ditolak (S3-AC3 Locked)

**Referensi:** S3-AC3
**Persona:** ROLE-RISK, ROLE-IT-ADMIN
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Ubah `periode_id` target compute delta ke Periode Mei 2026 (status CLOSED).
2. Trigger komputasi delta via API: `POST /api/v1/poci/compute-delta-batch` dengan `periode_id` = Mei 2026.

**Hasil yang diharapkan:**

- HTTP 423, body `{ "error": { "code": "POCI_PERIODE_LOCKED", ... } }`.
- Tidak ada INSERT ke `jrnl.jurnal_header`.
- Toast error di UI: "Periode buku sudah CLOSED. Delta POCI tidak dapat diposting..."

---

### TC-C001-13 — Direction Mismatch Terdeteksi Sebelum Posting (S3-AC4)

**Referensi:** S3-AC4
**Persona:** ROLE-IT-ADMIN (API test)
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Via API (simulasi data corrupt): inject baris `ecl.poci_delta_log` dengan `delta_ecl` = +200.000.000 tetapi `direction` = "DECREASE".
2. Trigger posting jurnal untuk baris tersebut (POST /api/v1/poci/delta-log/{id}/post).

**Hasil yang diharapkan:**

- HTTP 422, body `{ "error": { "code": "POCI_JURNAL_DIRECTION_MISMATCH", ... } }`.
- Tidak ada jurnal INSERT.
- Audit: `POCI.DIRECTION_MISMATCH_DETECTED` tercatat.

---

### TC-C001-14 — Result Line M10 Tidak Ada Warning Lama (S4-AC1)

**Referensi:** S4-AC1
**Persona:** ROLE-RISK, ROLE-AKUN-CTL
**Prioritas:** P1

**Langkah pengujian:**

1. Navigasi ke detail CALC-RUN-M10-001 → tab "Result Lines".
2. Filter tipe = POCI. Klik baris POCI-DEP-0001.

**Hasil yang diharapkan:**

- Field `deltaEcl` tersedia (bukan diisi `null`).
- Field `warnings` = `[]` (array kosong) — tidak ada `POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA`.
- `stageMarker` = "POCI".
- `baselineEcl` dan `currentEcl` tampil.

---

### TC-C001-15 — Calc Run Lama Tetap Punya Warning Lama (S4-AC3)

**Referensi:** S4-AC3
**Persona:** ROLE-AUDIT
**Prioritas:** P1

**Langkah pengujian:**

1. Navigasi ke calc run yang dibuat sebelum M10 (misal CALC-RUN-PRE-M10-001).
2. Lihat result line untuk instrumen POCI.

**Hasil yang diharapkan:**

- Field `warnings` masih berisi `["POCI_ECL_REPRESENTS_INITIAL_BASELINE_NOT_DELTA"]`.
- Baris tidak berubah — immutable per DEC-018.
- M10 engine tidak meng-update baris lama.

---

### TC-C001-16 — Riwayat Delta POCI Per Instrumen (S5-AC1 History)

**Referensi:** S5-AC1
**Persona:** ROLE-RISK, ROLE-AUDIT
**Prioritas:** P1

**Langkah pengujian:**

1. Dari `/poci/delta-log`, klik link "Riwayat" pada baris POCI-DEP-0001.
2. Navigasi ke `/poci/instrumen/{id}/history`.
3. Amati grafik dan tabel.
4. Klik header kolom "Delta (IDR)" untuk sort.
5. Klik filter direction → pilih "Meningkat (INCREASE)".

**Hasil yang diharapkan:**

- Grafik LineChart muncul dengan 2 garis: delta_ecl (merah) dan kumulatif (biru putus-putus).
- Tabel menampilkan kolom: Tgl Hitung, Baseline (IDR), ECL Saat Ini (IDR), Delta (IDR), Arah, Kumulatif Sebelum, Status.
- Sort by Delta berfungsi (URL diperbarui: `?sort=delta_ecl:asc`).
- Filter INCREASE memfilter tabel + URL: `?filter[direction]=INCREASE`.
- Cursor pagination: tombol Prev/Next + estimasi total.

---

### TC-C001-17 — Dashboard POCI MTD/YTD + Direction Breakdown (S5-AC2)

**Referensi:** S5-AC2
**Persona:** ROLE-RISK, ROLE-AKUN-CTL
**Prioritas:** P1

**Langkah pengujian:**

1. Navigasi ke `/poci/dashboard`.
2. Pilih periode: Juni 2026 (month=6, year=2026).
3. Amati card-card summary.

**Hasil yang diharapkan:**

- Card "Delta ECL MTD" menampilkan jumlah dalam IDR dengan format `Rp X.XXX.XXX.XXX`.
- Card "Delta ECL YTD" tersedia.
- Grid direction breakdown menampilkan:
  - INCREASE: jumlah instrumen + total IDR.
  - DECREASE: jumlah instrumen + total IDR.
  - ZERO: jumlah instrumen.
- Selektor periode mengubah URL: `?month=6&year=2026`.

---

### TC-C001-18 — Large Delta Alert (S5-AC3)

**Referensi:** S5-AC3
**Persona:** ROLE-RISK
**Prioritas:** P1

**Langkah pengujian:**

1. Setelah POCI-DEP-0001 menghasilkan delta +200jt (di bawah threshold 500jt), cek badge LARGE.
2. Update current ECL POCI-DEP-0001 ke IDR 2.000.000.000 (delta = +750jt > threshold 500jt).
3. Trigger komputasi delta.
4. Cek `/poci/delta-log` dan `/poci/dashboard`.

**Hasil yang diharapkan:**

- Baris POCI-DEP-0001 menampilkan badge "LARGE" (merah).
- Dashboard `/poci/dashboard` menampilkan banner merah: "X instrumen dengan large delta terdeteksi."
- Audit: `POCI.LARGE_DELTA_ALERT` ditulis tepat satu kali per (calc_run_id, instrumen_id).

---

### TC-C001-19 — Export Async Delta Log (S5-AC4 ROLE-AUDIT)

**Referensi:** S5-AC4
**Persona:** ROLE-AUDIT, ROLE-AKUN
**Prioritas:** P1

**Langkah pengujian:**

1. Login sebagai `uat.audit`. Navigasi ke `/poci/delta-log`.
2. Klik tombol "Export ▾" → pilih "CSV".
3. Amati: jika row count > 10.000, muncul JobProgressPanel (async, 202).
4. Setelah selesai, link download muncul.
5. Login sebagai `uat.akun`. Coba export tanpa filter aktif.

**Hasil yang diharapkan:**

- `uat.audit`: export berhasil (202 + jobId → progress → download link).
- Audit: `POCI.EXPORT` ditulis dengan detail format + row_count.
- `uat.akun` tanpa filter: HTTP 403 `FORBIDDEN` ("Anda tidak punya izin export tanpa filter").
- `uat.akun` dengan filter aktif (mis. direction=INCREASE): export diizinkan.

---

### TC-C001-20 — Idempotency-Key Replay pada POST Baseline (Cross-cutting)

**Referensi:** DEC-021
**Persona:** ROLE-IT-ADMIN
**Prioritas:** P0 - Kritis

**Langkah pengujian:**

1. Via API: `POST /api/v1/poci/baseline` untuk instrumen baru (is_poci=TRUE) dengan Idempotency-Key: `test-idem-001`.
2. Kirim request yang sama lagi (sama payload, sama key).
3. Kirim request dengan key yang sama tapi payload berbeda.

**Hasil yang diharapkan:**

- Request 1: HTTP 201, baseline dibuat.
- Request 2 (same key, same payload): HTTP 200, respons identik dengan request 1 — `IDEMPOTENCY_REPLAY`. Tidak ada baris baseline baru.
- Request 3 (same key, different payload): HTTP 422 `IDEMPOTENCY_MISMATCH`.

---

## Ringkasan Coverage AC

| TC | Story | AC | Prioritas | Status |
|----|-------|----|-----------|--------|
| TC-C001-01 | S1 | AC1 Baseline captured in-tx | P0 | — |
| TC-C001-02 | S1 | AC2 Baseline immutable (WORM) | P0 | — |
| TC-C001-03 | S1 | AC3 Non-POCI no baseline | P1 | — |
| TC-C001-04 | S1 | AC4 Role-gated read/write | P1 | — |
| TC-C001-05 | S2 | AC1 Delta INCREASE direction | P0 | — |
| TC-C001-06 | S2 | AC2 Delta DECREASE direction | P0 | — |
| TC-C001-07 | S2 | AC3 Missing baseline error-log | P1 | — |
| TC-C001-08 | S2 | AC4 Duplicate idempotency | P0 | — |
| TC-C001-09 | S3 | AC1 Jurnal INCREASE D/K | P0 | — |
| TC-C001-10 | S3 | AC2 Jurnal DECREASE D/K | P0 | — |
| TC-C001-11 | S3 | AC3 ZERO skipped | P1 | — |
| TC-C001-12 | S3 | AC3 Periode CLOSED lock | P0 | — |
| TC-C001-13 | S3 | AC4 Direction mismatch guard | P0 | — |
| TC-C001-14 | S4 | AC1 No stale warning M10 | P1 | — |
| TC-C001-15 | S4 | AC3 Legacy runs retain warning | P1 | — |
| TC-C001-16 | S5 | AC1 History per-instrument | P1 | — |
| TC-C001-17 | S5 | AC2 Dashboard MTD/YTD | P1 | — |
| TC-C001-18 | S5 | AC3 Large delta flag + alert | P1 | — |
| TC-C001-19 | S5 | AC4 Export permission async | P1 | — |
| TC-C001-20 | Cross | DEC-021 Idempotency replay | P0 | — |

**20 of 20 AC covered.**

---

## Catatan Compliance

- **DEC-010**: POCI bypasses staging engine; delta = current − baseline. Ditest TC-05, TC-06, TC-11.
- **DEC-016**: `shopspring/decimal`, NUMERIC(20,4) IDR, NUMERIC(10,8) EIR. Ditest TC-01, TC-05, TC-09, TC-10.
- **DEC-018**: Audit append-only, baseline WORM. Ditest TC-02, TC-15.
- **DEC-021**: Idempotency-Key wajib. Ditest TC-20, TC-08.
- **DEC-022**: Cursor-based pagination. Ditest TC-16, TC-19.
- **UX §3**: Proses > 2s pakai JobProgressPanel. Ditest TC-05 (compute trigger), TC-19 (export async).
