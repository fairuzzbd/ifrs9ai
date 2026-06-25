# UAT-APP-B-010 — Transaksi Renewal: DataTable UX §1 + Form Notif + SoD

**UAT ID**: UAT-APP-B-010
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-03
**AC yang dicakup**: M16-03-AC1 (renewal list DataTable), M16-03-AC2 (renewal form notif), M16-03-AC4 (SoD renewal)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — SoD enforcement; Idempotency-Key auto-inject

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M7 deployed — renewal endpoints aktif
3. P5-M16 deployed — gap fixes pada `/transaksi/renewal/` aktif (default sort, date range filter, export threshold)
4. Data seed:
   - 5+ renewal records dengan status bervariasi (DRAFT, SUBMITTED, APPROVED)
   - 1 renewal `RNW-TEST-001` status SUBMITTED, makerId = USR-MAKER-001
   - 1 penempatan aktif `DEP-0042` (sebagai instrumen asal renewal baru)
   - 1 penempatan yang sudah EXPIRED `DEP-EXPIRED` (untuk negative test)
   - Periode buku PRD-2026-06 status OPEN
5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | renewal.read, renewal.create, renewal.submit |
| USR-APPR-001 | ROLE-APPR-TR | renewal.read, renewal.review, renewal.approve |

---

## Data Test Numerik

Renewal baru:
- Instrumen asal: DEP-0042 (deposito BCA, Rp 2.000.000.000, jatuh tempo 2026-09-25)
- Pokok baru: Rp 2.000.000.000 (full rollover)
- Tenor baru: 91 hari
- Suku bunga baru: 5.50% p.a.
- Tanggal efektif baru: 2026-09-25 (pada atau setelah jatuh tempo asal)

Preview cashflow yang diharapkan:
- Bunga 91 hari: Rp 2.000.000.000 × 5.50% × 91/365 = Rp 27.397.260
- Total jatuh tempo baru: Rp 2.027.397.260

---

## Skenario UAT

### TC-001 — M16-03-AC1: List renewal DataTable UX §1 lengkap

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/renewal`
2. Verifikasi kolom dan default sort
3. Klik filter "Status" → pilih "SUBMITTED"
4. Cek URL
5. Buka filter tanggal renewal → set range: 01 Jun 2026 — 30 Jun 2026
6. Klik X pada filter chip "Status: SUBMITTED" → hanya date range aktif
7. Klik "Ekspor" → CSV
8. Verifikasi file CSV
9. Klik "Selanjutnya" untuk paginasi

**Hasil yang Diharapkan**:
- Langkah 2: Default sort `tanggal_renewal:desc` (renewal terbaru di atas); kolom: Kode Renewal, Instrumen Asal, Nominal IDR, Suku Bunga Baru, Tgl Renewal, Status
- Langkah 3-4: URL mengandung `filter[workflow_status]=SUBMITTED`; filter chip tampil
- Langkah 5: Date range picker tersedia untuk kolom `tanggal_renewal` (M16 gap fix: sebelumnya MISSING)
- Langkah 6: Filter chip Date Range tetap; filter Status hilang; data di-filter hanya by date
- Langkah 7-8: File CSV mengandung hanya data yang di-filter oleh date range aktif; header row dalam Bahasa Indonesia; nilai nominal IDR dengan 4 desimal
- Langkah 9: Halaman berikutnya load dengan cursor baru; pagination info update ("Halaman 2 dari ~X")

**Verifikasi Audit** (untuk export):
```sql
SELECT * FROM aud.audit_log WHERE action = 'RENEWAL.EXPORT' ORDER BY event_time DESC LIMIT 1;
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-03-AC2: Form renewal baru — preview + sukses + gagal

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/renewal/new`
2. Pilih instrumen asal: DEP-0042
3. Isi suku bunga baru: 5.50%, tenor baru: 91 hari, tanggal efektif: 2026-09-25, pokok: Rp 2.000.000.000
4. Klik "Hitung Preview"
5. Cek preview cashflow yang tampil
6. Klik "Simpan Draft"
7. Perhatikan pending state dan response
8. Ulangi dengan instrumen DEP-EXPIRED → klik Simpan → lihat error

**Hasil yang Diharapkan**:
- Langkah 4-5: Halaman menampilkan tabel preview cashflow secara inline (tanpa toast); nilai bunga ≈ Rp 27.397.260; total ≈ Rp 2.027.397.260
- Langkah 6-7 (sukses): Tombol disable + "Menyimpan..." spinner; 201 response → toast hijau 4 detik: "Renewal RNW-XXXXX berhasil dibuat. Menunggu submit ke reviewer." + link "Lihat detail →"; form reset setelah toast
- Langkah 8 (WORKFLOW_INVALID_TRANSITION): Toast merah persistent: "Renewal tidak dapat dibuat: instrumen DEP-EXPIRED sudah melewati tanggal jatuh tempo." + error code + traceId
- Idempotency-Key: UUID v4 terlihat di Network tab DevTools sebagai header `Idempotency-Key`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-03-AC4: SoD enforcement — maker tidak bisa review renewal sendiri

**Actor**: USR-MAKER-001 (maker), USR-APPR-001 (approver)
**Pre-kondisi**: RNW-TEST-001 status SUBMITTED, makerId = USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/renewal/RNW-TEST-001`
2. Inspect DOM untuk tombol "Review & Tandatangani"
3. Logout → Login sebagai USR-APPR-001
4. Navigasi ke `/transaksi/renewal/RNW-TEST-001`
5. Klik "Approve" → isi komentar "Suku bunga sesuai mandate" → konfirmasi
6. Sebagai USR-MAKER-001: POST langsung ke API `/api/v1/transaksi/renewal/RNW-TEST-001/approve`

**Hasil yang Diharapkan**:
- Langkah 2 (USR-MAKER-001): Tombol "Review & Tandatangani" dan "Approve" **TIDAK ADA** di DOM; DevTools → `document.querySelectorAll('[data-action="approve"]').length` → 0
- Langkah 4-5 (USR-APPR-001): Tombol "Approve" tampil; dialog konfirmasi muncul; POST `/approve` → 200; toast: "Renewal RNW-TEST-001 berhasil di-approve. Jurnal otomatis akan di-buat."
- Langkah 6: Response `HTTP 403` dengan `{ "error": { "code": "SOD_VIOLATION" } }`

**Verifikasi Audit**:
```sql
SELECT action, actor_user_id, entity_id FROM aud.audit_log
WHERE action LIKE 'RENEWAL.%' AND entity_id = '{id RNW-TEST-001}'
ORDER BY event_time DESC;
```
Harus terlihat: `RENEWAL.SUBMIT` by USR-MAKER-001, `RENEWAL.APPROVE` by USR-APPR-001

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
