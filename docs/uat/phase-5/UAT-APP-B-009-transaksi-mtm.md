# UAT-APP-B-009 — Transaksi MTM: Upload Dropzone + JobProgressPanel + Role Gate

**UAT ID**: UAT-APP-B-009
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-02
**AC yang dicakup**: M16-02-AC1 (redirect), M16-02-AC2 (upload JobProgressPanel), M16-02-AC3 (list DataTable), M16-02-AC4 (role gate)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — upload role gate; JobProgressPanel SSE; absent-from-DOM

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M6 deployed — MTM endpoints aktif; M13 job SSE tersedia
3. P5-M16 deployed — MTM screens relocated ke `/transaksi/mtm/`; 308 redirects aktif
4. File test tersedia:
   - `mtm-ibpa-2026-06-25.csv` — format IBPA valid, ~5.000 baris, ukuran ~1.2 MB
   - `mtm-invalid-format.pdf` — untuk negative test
   - `mtm-too-large.csv` — >50 MB untuk test batas ukuran
5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-AKUN-001 | ROLE-AKUN | transaksi.mtm.read, transaksi.mtm.upload |
| USR-RISK-001 | ROLE-RISK | transaksi.mtm.read |
| USR-AUDIT-001 | ROLE-AUDIT | transaksi.mtm.read |

---

## Data Test Numerik

File upload `mtm-ibpa-2026-06-25.csv`:
- Total record: 5.678 baris
- Expected: 5.666 record berhasil diproses; 12 error (instrumen tidak dikenal)
- Total MTM IDR expected: ~Rp 1.025.000.000 (contoh)
- Batch ID yang di-generate: `JOB-MTM-UPLOAD-{tanggal UAT}`

---

## Skenario UAT

### TC-001 — M16-02-AC1: 308 redirect dari /mtm/* ke /transaksi/mtm/*

**Actor**: USR-AKUN-001

**Langkah**:
1. Clear browser cache
2. Akses 6 path lama secara bergantian:
   - `/mtm`
   - `/mtm/upload`
   - `/mtm/upload/batch/BATCH-TEST-001`
   - `/mtm/cron`
   - `/mtm/{id MTM record valid}`
   - `/mtm/alerts/stale-price`
3. Verifikasi via curl: `curl -I https://uat.blips.tugu-re.com/mtm/alerts/stale-price`

**Hasil yang Diharapkan**:
- Setiap path redirect ke `/transaksi/mtm/...` equivalentnya
- Tidak ada 404 dari keenam path
- `/mtm/alerts/stale-price` → `/transaksi/mtm/alerts/stale-price` (bukan `/transaksi/mtm/alerts` — wildcard tidak menangkap path ini)
- curl: Response header `HTTP/2 308` dengan `Location` yang benar
- Query string dipertahankan: `/mtm?filter[status]=VALID` → `/transaksi/mtm?filter[status]=VALID`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-02-AC2: Upload dropzone validasi client-side

**Actor**: USR-AKUN-001

**Langkah**:
1. Login sebagai USR-AKUN-001, navigasi ke `/transaksi/mtm/upload`
2. Drag-and-drop file `mtm-invalid-format.pdf` ke area dropzone
3. Cek toast/pesan error
4. Cek Network tab DevTools — verifikasi tidak ada POST yang terkirim
5. Drag-and-drop file `mtm-too-large.csv` ke area dropzone
6. Cek pesan error ukuran

**Hasil yang Diharapkan**:
- Langkah 2-4: Toast error instant (sebelum POST): "Format file tidak didukung. Gunakan CSV atau XLSX." — tidak ada request POST ke server
- Langkah 5-6: Toast error instant: "Ukuran file melebihi batas 50 MB."
- Dropzone area ter-highlight saat file di-drag (visual feedback drag-and-drop)
- Label "Taruh file di sini atau klik untuk browse" tampil dengan benar
- Subtitle "CSV, XLSX • Maks. 50 MB" tampil

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-02-AC2: Upload file valid → JobProgressPanel SSE → batch detail

**Actor**: USR-AKUN-001

**Langkah**:
1. Login sebagai USR-AKUN-001, navigasi ke `/transaksi/mtm/upload`
2. Upload file `mtm-ibpa-2026-06-25.csv`
3. Klik tombol "Upload"
4. Perhatikan respons dan perubahan UI segera setelah klik
5. Perhatikan JobProgressPanel yang muncul
6. Baca step text dan progress percentage
7. Tunggu job selesai (atau klik "Lanjutkan di Background")
8. Setelah selesai: klik link "Lihat hasil batch →"

**Hasil yang Diharapkan**:
- Langkah 3-4: Tombol "Upload" disable + spinner inline; header `Idempotency-Key` terkirim (verify via Network DevTools)
- Langkah 5: `JobProgressPanel` muncul di bawah dropzone area dengan:
  - Progress bar (0% naik bertahap)
  - Step text: "Parsing baris X dari 5.678"
  - ETA: "Estimasi selesai: {timestamp} ({N} detik lagi)"
  - Tombol "Batalkan Upload" (jika `canCancel=true`)
  - Tombol "Lanjutkan di Background"
- Langkah 7 (setelah completed): Panel menampilkan "Upload selesai. 5.678 record MTM diproses, 12 error ditemukan."
- Toast hijau: "Batch upload MTM JOB-MTM-UPLOAD-{date} selesai. 5.678 record diproses." + link "Lihat hasil batch →"
- Langkah 8: Navigasi ke `/transaksi/mtm/upload/batch/{jobId}` dengan tabel hasil parsing

**Verifikasi Teknis**:
- Network DevTools: Request ke `GET /api/v1/jobs/{jobId}/stream` dengan header `Accept: text/event-stream`
- SSE events terlihat di Network tab sebagai Server-Sent Events stream

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-02-AC3: List DataTable UX §1 + stale price filter

**Actor**: USR-AKUN-001

**Langkah**:
1. Login sebagai USR-AKUN-001, navigasi ke `/transaksi/mtm`
2. Verifikasi kolom DataTable
3. Klik header "Tanggal MTM" → data di-sort
4. Klik tombol "[!] Harga Stale" di atas tabel
5. Perhatikan filter chip yang muncul
6. Klik link "Lihat Stale Price Alerts →"

**Hasil yang Diharapkan**:
- Langkah 2: Kolom minimal: Kode Instrumen, Jenis, Tanggal MTM, Harga Pasar, MTM IDR, Sumber Harga, Status
- Langkah 3: Data di-sort; icon indicator berubah
- Langkah 4: Filter chip "Stale: ya ×" muncul; tabel hanya menampilkan record `is_stale=true`; URL mengandung `filter[is_stale]=true`
- Langkah 6: Navigasi ke `/transaksi/mtm/alerts/stale-price`; DataTable alerts tampil dengan instrumen stale

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M16-02-AC4: Role gate — upload button absent untuk ROLE-RISK; ROLE-AUDIT read-only

**Actor**: USR-RISK-001 (ROLE-RISK), USR-AUDIT-001 (ROLE-AUDIT)

**Langkah**:
1. Login sebagai USR-RISK-001, navigasi ke `/transaksi/mtm`
2. Inspect DOM untuk tombol upload
3. Coba akses langsung `/transaksi/mtm/upload`
4. Logout → Login sebagai USR-AUDIT-001, navigasi ke `/transaksi/mtm`
5. Inspect DOM untuk semua tombol mutasi

**Hasil yang Diharapkan**:
- Langkah 2: Tombol "Upload File" atau "+ Upload MTM" **TIDAK ADA** di DOM; bukan disabled — absent
- Langkah 3: Halaman upload tidak di-render; redirect ke `/transaksi/mtm` (read) atau notFound (403/404)
- Langkah 5 (ROLE-AUDIT):
  - DataTable tampil dengan data MTM (read-only access berfungsi)
  - Tombol "Upload", "Trigger Cron" **TIDAK ADA** di DOM
  - Tombol "Ekspor" **TAMPIL** (AUDIT punya export permission)
- DevTools Console: `document.querySelectorAll('[aria-label*="Upload"]').length` → 0 untuk kedua role

**Verifikasi Keamanan**:
- Inspect page HTML source: tidak mengandung `data-action="upload"` atau form upload untuk ROLE-RISK/AUDIT

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
