# UAT-APP-B-008 — Transaksi Penempatan: DataTable + Form Notif + Workflow + SoD

**UAT ID**: UAT-APP-B-008
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-01
**AC yang dicakup**: M16-01-AC1 (redirect), M16-01-AC2 (list DataTable), M16-01-AC3 (form notif), M16-01-AC4 (workflow SoD)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — SoD enforcement, absent-from-DOM, Idempotency-Key

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M1 deployed — `POST /api/v1/transaksi/penempatan` + workflow endpoints aktif
3. P5-M16 deployed — screens relocated ke `/transaksi/penempatan/`; 308 redirects aktif di `next.config.js`
4. Data seed:
   - Minimal 5 penempatan aktif dengan jenis instrumen campuran (DEPOSITO, OBLIGASI)
   - 1 penempatan status DRAFT dibuat oleh USR-MAKER-001 (id: `PNP-TEST-001`)
   - 1 penempatan status PENDING_REVIEW dibuat oleh USR-MAKER-001 (id: `PNP-TEST-002`)
5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | penempatan.create, penempatan.read, penempatan.submit |
| USR-APPR-001 | ROLE-APPR-TR | penempatan.read, penempatan.review, penempatan.approve |
| USR-RISK-001 | ROLE-RISK | penempatan.read |

---

## Data Test Numerik

Seed data minimal untuk TC-003 (form create):
- Counterparty: Bank BCA (id: `cprt-bca-001`)
- Nominal: Rp 2.500.000.000
- Tenor: 92 hari
- Suku Bunga: 5.25% p.a.
- Metode: AKTUAL/365
- Tanggal Penempatan: tanggal UAT berlangsung

Hasil yang diharapkan dari API:
- `kode_penempatan`: `PNP-00XXXX` (auto-generated)
- `workflow_status`: `DRAFT`

---

## Skenario UAT

### TC-001 — M16-01-AC1: 308 redirect dari URL lama ke URL baru

**Actor**: USR-MAKER-001
**Pre-kondisi**: `next.config.js` sudah mengandung 4 redirect rules untuk penempatan

**Langkah**:
1. Buka browser baru (clear cache)
2. Akses langsung: `https://uat.blips.tugu-re.com/trx/penempatan`
3. Perhatikan URL di address bar setelah halaman load
4. Akses: `https://uat.blips.tugu-re.com/trx/penempatan/new`
5. Akses: `https://uat.blips.tugu-re.com/trx/penempatan/PNP-TEST-001`
6. Akses: `https://uat.blips.tugu-re.com/trx/penempatan/PNP-TEST-001/edit`
7. Verifikasi via curl (dari server terminal): `curl -I https://uat.blips.tugu-re.com/trx/penempatan`

**Hasil yang Diharapkan**:
- Langkah 2: Browser redirect → URL menjadi `/transaksi/penempatan` (tidak ada 404)
- Langkah 4: URL menjadi `/transaksi/penempatan/new`
- Langkah 5: URL menjadi `/transaksi/penempatan/PNP-TEST-001`
- Langkah 6: URL menjadi `/transaksi/penempatan/PNP-TEST-001/edit`
- Langkah 7 (curl): Response header menampilkan `HTTP/2 308` atau `Location: /transaksi/penempatan`
- Tidak ada 404 dari keempat path di atas
- Query string tidak hilang: akses `/trx/penempatan?filter[stage]=1` → redirect ke `/transaksi/penempatan?filter[stage]=1`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-01-AC2: List DataTable UX §1 — sort, filter, paging, export

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penempatan`
2. Tunggu DataTable load — verifikasi tidak ada blank screen (skeleton tampil)
3. Klik header kolom "Kode Penempatan" → perhatikan sort indicator
4. Klik lagi → arah sort berubah
5. Ketik "BCA" di search bar global
6. Buka filter dropdown "Status" → pilih "DRAFT"
7. Verifikasi filter chip muncul: "Status: DRAFT ×"
8. Klik "Bersihkan semua filter"
9. Klik tombol "Ekspor" → pilih CSV
10. Verifikasi URL di address bar saat filter aktif dapat di-bookmark/share

**Hasil yang Diharapkan**:
- Langkah 2: Skeleton row muncul sebentar → data tampil; tidak ada blank screen
- Langkah 3: Icon panah ↑ muncul di header; data di-sort ascending
- Langkah 4: Icon berubah ke ↓; data di-sort descending
- Langkah 5: Tabel filter ke penempatan yang mengandung "BCA"
- Langkah 6: Tabel menampilkan hanya status DRAFT; chip "Status: DRAFT" muncul di filter bar
- Langkah 8: Filter bersih; tabel kembali ke seluruh data
- Langkah 9: File CSV terunduh; nama file mengandung tanggal hari ini; audit `PENEMPATAN.EXPORT` tercatat di `aud.audit_log`
- Langkah 10: URL mengandung `sort=`, `filter[workflow_status]=DRAFT` — dapat di-bookmark dan di-restore
- Kolom yang ada: Kode, Jenis Instrumen, Counterparty, Nominal IDR, Tgl Penempatan, Tgl Jatuh Tempo, Stage, Status

**Verifikasi Audit**:
```sql
SELECT * FROM aud.audit_log
WHERE action = 'PENEMPATAN.EXPORT'
ORDER BY event_time DESC LIMIT 1;
```
Hasil: ada 1 row dengan `actor_user_id` = id USR-MAKER-001

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-01-AC3: Form create penempatan — sukses, gagal, pending

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penempatan/new`
2. Isi semua field wajib dengan data test numerik di atas
3. Klik "Simpan sebagai Draft"
4. Perhatikan tombol setelah diklik
5. Tunggu response server
6. Ulangi: buat form baru, **biarkan `counterparty_id` kosong**, klik "Simpan sebagai Draft"
7. Perhatikan response error
8. Ulangi: buka Network tab DevTools sebelum submit → perhatikan header `Idempotency-Key`

**Hasil yang Diharapkan**:
- Langkah 3: Tombol langsung berubah menjadi "Menyimpan..." + spinner inline; tombol disabled (tidak bisa diklik dua kali)
- Langkah 5 (sukses):
  - Toast hijau muncul di kanan atas, 4 detik: "Penempatan PNP-00XXXX berhasil dibuat sebagai draft. Menunggu submit ke reviewer."
  - Toast mengandung link "Lihat detail →" ke `/transaksi/penempatan/{id}`
  - Form di-reset ke state kosong (setelah toast tampil, bukan sebelum)
- Langkah 6-7 (gagal VALIDATION_FAILED):
  - Toast merah persistent (tidak auto-dismiss) dengan jumlah field bermasalah
  - Error code `VALIDATION_FAILED` tampil di toast
  - traceId (8 char) tampil di footer toast
  - Field `counterparty_id` highlight merah border + message inline di bawah field
  - Tombol kembali enabled; data form dipertahankan (user tidak kehilangan input)
- Langkah 8: Header `Idempotency-Key` berisi UUID v4 format `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`; bukan visible ke user

**Verifikasi Audit** (untuk submit sukses):
```sql
SELECT * FROM aud.audit_log
WHERE action = 'PENEMPATAN.CREATE'
  AND actor_user_id = '{id USR-MAKER-001}'
ORDER BY event_time DESC LIMIT 1;
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-01-AC4: Workflow Maker → Reviewer → Approver + SoD enforcement

**Actor**: USR-MAKER-001 (maker), USR-APPR-001 (reviewer + approver)
**Pre-kondisi**: PNP-TEST-001 status DRAFT, makerId = USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penempatan/PNP-TEST-001`
2. Klik "Submit ke Reviewer" → dialog konfirmasi muncul → konfirmasi
3. Perhatikan response dan toast
4. Coba klik tombol "Review & Tandatangani" dari halaman yang sama (masih login sebagai USR-MAKER-001)
5. Logout → Login sebagai USR-APPR-001
6. Navigasi ke `/transaksi/penempatan/PNP-TEST-001`
7. Klik "Review & Tandatangani" → isi komentar → konfirmasi
8. Login sebagai USR-MAKER-001 → coba langsung POST ke API review: `POST /api/v1/transaksi/penempatan/PNP-TEST-001/review`

**Hasil yang Diharapkan**:
- Langkah 2-3: POST `/submit` berhasil → 200; toast: "PNP-TEST-001 berhasil di-submit ke reviewer. Menunggu tanda tangan reviewer."
- Langkah 4 (SoD check): Tombol "Review & Tandatangani" **TIDAK ADA** di DOM untuk USR-MAKER-001 — server component tidak render tombol jika `maker_id == current_user_id`
- Langkah 7: USR-APPR-001 (bukan maker) dapat review — tombol ada; POST `/review` → 200; toast: "PNP-TEST-001 berhasil di-review. Menunggu persetujuan approver."
- Langkah 8 (direct API): Response `HTTP 403` dengan body `{ "error": { "code": "SOD_VIOLATION", ... } }`

**Verifikasi**:
- DevTools Elements: Saat login USR-MAKER-001, jalankan `document.querySelectorAll('[data-action="review"]').length` → 0
- curl: `curl -X POST -H "Authorization: Bearer {JWT-maker}" .../review` → 403 SOD_VIOLATION

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — ROLE-RISK: Read-only access, tidak ada tombol create/submit/approve

**Actor**: USR-RISK-001 (ROLE-RISK)

**Langkah**:
1. Login sebagai USR-RISK-001
2. Navigasi ke `/transaksi/penempatan`
3. Klik salah satu row → detail page
4. Coba akses `/transaksi/penempatan/new`

**Hasil yang Diharapkan**:
- Langkah 2: DataTable tampil read-only; tidak ada tombol "+ Penempatan Baru" di header
- Langkah 3: Detail page tampil; tidak ada tombol "Submit", "Review", "Approve" di MakerReviewerApproverPanel
- Langkah 4: Redirect ke `/transaksi/penempatan` atau render `notFound()` — tidak bisa akses form create

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
