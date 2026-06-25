# UAT-APP-B-013 — Transaksi Akrual: KPI Dashboard + Batch Trigger + Role Gate

**UAT ID**: UAT-APP-B-013
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-05
**AC yang dicakup**: M16-05-AC1 (KPI cards), M16-05-AC2 (batch trigger JobProgressPanel), M16-05-AC3 (role gate AKUN-CTL vs MAKER-TR), M16-05-AC4 (DataTable auto-refresh)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — batch trigger gated; AKUN-CTL approve vs MAKER-TR trigger SoD; Idempotency-Key

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M10 deployed — akrual endpoints aktif (`/api/v1/transaksi/akrual/**`)
3. P5-M16 deployed — akrual screen di `/transaksi/akrual/`; KPI cards; batch trigger; role gating
4. Data seed:
   - 1.100 instrumen aktif dengan akrual pending untuk periode PRD-2026-06
   - Total akrual IDR seed: Rp 1.234.567.890 (4 desimal)
   - Status batch terakhir: IDLE (belum ada batch berjalan)
   - Periode buku PRD-2026-06 status OPEN
5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | transaksi.akrual.read, transaksi.akrual.trigger |
| USR-AKUN-CTL-001 | ROLE-AKUN-CTL | transaksi.akrual.read, transaksi.akrual.trigger, transaksi.akrual.approve |
| USR-AUDIT-001 | ROLE-AUDIT | transaksi.akrual.read |

---

## Data Test Numerik

KPI dashboard seed (endpoint `GET /api/v1/transaksi/akrual/dashboard`):
- `total_akrual_idr`: 1234567890.0000 → tampil sebagai "Rp 1.234.567.890"
- `instrumen_diproses`: 1100
- `instrumen_pending`: 0 (sebelum batch pertama akan > 0)
- `last_batch_status`: null (IDLE)
- `last_batch_at`: null

Batch trigger expected output:
- jobId: `JOB-AKRUAL-{tanggal-UAT}-001`
- Instrumen yang akan diproses: 1.100
- ETA: ~3 menit (estimasi)

---

## Skenario UAT

### TC-001 — M16-05-AC1: KPI Dashboard cards tampil dengan data valid

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/akrual`
2. Perhatikan bagian KPI cards di atas halaman
3. Cek nilai total akrual IDR
4. Cek jumlah instrumen diproses
5. Cek status batch terakhir
6. Refresh halaman (F5) → KPI masih tampil

**Hasil yang Diharapkan**:
- Langkah 2: 3-4 KPI card tampil di atas DataTable dengan layout grid horizontal
- Langkah 3: Card "Total Akrual" menampilkan "Rp 1.234.567.890" (format IDR dengan titik ribuan, tanpa desimal untuk display)
- Langkah 4: Card "Instrumen Diproses" menampilkan "1.100" (format dengan titik ribuan)
- Langkah 5: Card "Batch Terakhir" menampilkan "Belum ada batch" atau status null yang informatif
- Langkah 6: KPI cards tetap tampil setelah refresh (data di-fetch dari API, bukan hanya client state)

**Verifikasi Teknis**:
- Network DevTools: request ke `GET /api/v1/transaksi/akrual/dashboard` → 200 dengan body mengandung `total_akrual_idr` dan `instrumen_diproses`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-05-AC2: Batch trigger → konfirmasi dialog → JobProgressPanel → selesai

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/akrual`
2. Klik tombol "Jalankan Batch Akrual"
3. Perhatikan dialog yang muncul
4. Baca konten dialog
5. Klik "Batalkan" → verifikasi tidak ada POST terkirim
6. Klik "Jalankan Batch Akrual" lagi → kali ini klik "Konfirmasi"
7. Perhatikan perubahan UI setelah konfirmasi
8. Monitor JobProgressPanel sampai selesai
9. Setelah selesai: perhatikan toast dan perubahan DataTable

**Hasil yang Diharapkan**:
- Langkah 3: Dialog konfirmasi modal muncul (bukan langsung POST)
- Langkah 4: Dialog berisi: judul "Konfirmasi Batch Akrual", deskripsi "Proses batch akrual untuk 1.100 instrumen akan dijalankan. Ini tidak dapat dibatalkan setelah dimulai.", tombol "Batalkan" + "Konfirmasi"
- Langkah 5: POST tidak terkirim; dialog tutup
- Langkah 6: POST terkirim ke `POST /api/v1/transaksi/akrual/batch-trigger` dengan header `Idempotency-Key`; response `202 Accepted` dengan `{ jobId, statusUrl, streamUrl }`
- Langkah 7: JobProgressPanel muncul dengan progress bar 0%, step text "Menginisialisasi batch akrual..."
- Langkah 8: Progress naik bertahap; step text update: "Menghitung akrual instrumen X dari 1.100"; ETA ditampilkan
- Langkah 9 (selesai): Toast hijau "Batch akrual selesai. 1.100 instrumen diproses. Total akrual: Rp 1.234.567.890." + link "Lihat detail"; DataTable refresh otomatis dengan data baru

**Verifikasi Audit**:
```sql
SELECT * FROM aud.audit_log WHERE action = 'AKRUAL.BATCH_TRIGGER' ORDER BY event_time DESC LIMIT 1;
```
Hasil: 1 row dengan `actor_user_id` = id USR-MAKER-001, `after_jsonb` mengandung `job_id`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-05-AC3: Role gate — ROLE-AKUN-CTL approve vs ROLE-MAKER-TR trigger

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR), USR-AKUN-CTL-001 (ROLE-AKUN-CTL)

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/akrual`
2. Inspect DOM: cek tombol "Jalankan Batch Akrual" dan "Approve Akrual"
3. Logout → Login sebagai USR-AKUN-CTL-001, navigasi ke `/transaksi/akrual`
4. Inspect DOM: cek kedua tombol
5. Logout → Login sebagai USR-AUDIT-001, navigasi ke `/transaksi/akrual`
6. Inspect DOM: cek semua tombol aksi

**Hasil yang Diharapkan**:
- Langkah 2 (USR-MAKER-001):
  - Tombol "Jalankan Batch Akrual" **TAMPIL** (role punya `transaksi.akrual.trigger`)
  - Tombol "Approve Akrual" / "Setujui Akrual" **TIDAK ADA** di DOM (bukan disabled)
- Langkah 4 (USR-AKUN-CTL-001):
  - Tombol "Jalankan Batch Akrual" **TAMPIL**
  - Tombol "Approve Akrual" **TAMPIL** (Finance Controller punya `transaksi.akrual.approve`)
- Langkah 6 (USR-AUDIT-001):
  - Tombol "Jalankan Batch Akrual" **TIDAK ADA** di DOM
  - Tombol "Approve Akrual" **TIDAK ADA** di DOM
  - Tombol "Ekspor" **TAMPIL**
  - DataTable **TAMPIL** (read-only)

**Verifikasi Keamanan**:
- Sebagai USR-MAKER-001: `POST /api/v1/transaksi/akrual/batch/{id}/approve` → `HTTP 403 FORBIDDEN`
- Sebagai USR-AUDIT-001: `POST /api/v1/transaksi/akrual/batch-trigger` → `HTTP 403 FORBIDDEN`
- DevTools Console: `document.querySelectorAll('[data-action="approve-akrual"]').length` → 0 untuk USR-MAKER-001

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-05-AC4: DataTable akrual — UX §1 + auto-refresh setelah batch selesai

**Actor**: USR-AKUN-CTL-001

**Langkah**:
1. Login sebagai USR-AKUN-CTL-001, navigasi ke `/transaksi/akrual`
2. Verifikasi kolom DataTable
3. Filter by status: pilih "PENDING"
4. Klik header "Tgl Akrual" → sort
5. Klik "Ekspor" → XLSX
6. Verifikasi file XLSX
7. Trigger batch akrual baru (via tombol "Jalankan Batch Akrual")
8. Monitor sampai selesai
9. Perhatikan DataTable setelah batch selesai

**Hasil yang Diharapkan**:
- Langkah 2: Kolom minimal: Kode Instrumen, Jenis, Counterparty, Tgl Akrual, Nominal Akrual IDR, EIR, Status
- Langkah 3: Filter chip "Status: PENDING" muncul; tabel hanya instrumen dengan akrual PENDING
- Langkah 4: Data di-sort ascending/descending; icon indicator berubah
- Langkah 5-6: File XLSX terunduh; header bold + freeze baris pertama; nilai IDR formatted `#,##0.0000`; hanya data yang sesuai filter PENDING
- Langkah 9: Setelah batch selesai, DataTable otomatis refresh (tanpa user klik Refresh manual); data status berubah dari PENDING ke PROCESSED

**Verifikasi Audit** (untuk export XLSX):
```sql
SELECT * FROM aud.audit_log WHERE action = 'AKRUAL.EXPORT' ORDER BY event_time DESC LIMIT 1;
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Tech Lead) | | | |
| Security Reviewer | | | |
| Approver (Business) | | | |
