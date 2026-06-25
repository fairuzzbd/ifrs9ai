# UAT-APP-B-011 — Transaksi Penjualan: DataTable UX §1 + BM Alerts + SoD

**UAT ID**: UAT-APP-B-011
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-03
**AC yang dicakup**: M16-03-AC3 (penjualan list + BM alerts widget), M16-03-AC4 (SoD penjualan)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — SoD enforcement; export filter fix; breadcrumb gap fix

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M8 deployed — penjualan endpoints aktif
3. P5-M16 deployed — gap fixes aktif (breadcrumb, sort useQueryState, BM alert shortcut, export filter)
4. Data seed:
   - 3+ penjualan records status bervariasi
   - 1 penjualan `PJL-TEST-001` status SUBMITTED, makerId = USR-MAKER-002
   - 1 instrumen aktif dengan `has_bm_alert=true`: SHM-0099
   - 1 BM alert aktif di `penjualan.bm-alerts` untuk portofolio PORT-SAHAM
5. User test:

| User ID | Role | Permission |
|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | penjualan.read, penjualan.create, penjualan.submit |
| USR-MAKER-002 | ROLE-MAKER-TR | penjualan.read, penjualan.create (makerId PJL-TEST-001) |
| USR-APPR-001 | ROLE-APPR-TR | penjualan.read, penjualan.review, penjualan.approve |
| USR-RISK-001 | ROLE-RISK | penjualan.read |

---

## Data Test Numerik

BM Alert test:
- Instrumen: SHM-0099 (saham, portofolio PORT-SAHAM)
- BM Status: POTENTIAL_RECLASSIFICATION
- Trigger Event: HIGH_FREQUENCY_SALE
- Tanggal Trigger: 2026-06-20
- Rekomendasi: "Tinjau BM portofolio PORT-SAHAM"

---

## Skenario UAT

### TC-001 — M16-03-AC3: Penjualan list DataTable — sort URL state + BM alert filter + breadcrumb

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penjualan`
2. Verifikasi breadcrumb ada di halaman
3. Klik header "Tanggal Eksekusi" → cek URL
4. Refresh halaman (F5) → cek sort masih aktif
5. Klik tombol "BM Alert" quick-filter
6. Cek filter chip dan URL
7. Klik "Ekspor" → CSV dengan filter `jenis_disposal=SELL` aktif
8. Verifikasi file CSV (filter direspect)

**Hasil yang Diharapkan**:
- Langkah 2: Breadcrumb `Beranda / Transaksi / Penjualan` tampil di atas halaman (M16 gap fix — sebelumnya MISSING)
- Langkah 3: URL update ke `?sort=tanggal_eksekusi:asc`
- Langkah 4: Setelah refresh, sort masih aktif (useQueryState deep-link, M16 gap fix — sebelumnya BROKEN dengan useState+useEffect)
- Langkah 5-6: Filter chip "BM Alert: Ya ×" muncul; URL mengandung `filter[bm_alert]=true`; tabel hanya menampilkan instrumen dengan BM alert aktif
- Langkah 7-8: File CSV mengandung kolom `jenis_disposal` dan hanya baris dengan `jenis_disposal=SELL` (M16 gap fix — sebelumnya filter tidak dikirim ke export URL)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-03-AC3: BM Alerts page /bm-alerts — DataTable dengan link Review ke APP-A

**Actor**: USR-MAKER-001, USR-RISK-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penjualan`
2. Klik link "BM Alerts" di header halaman
3. Verifikasi redirect ke `/transaksi/penjualan/bm-alerts`
4. Perhatikan DataTable BM alerts
5. Logout → Login sebagai USR-RISK-001, navigasi ke `/transaksi/penjualan/bm-alerts`
6. Perhatikan kolom "Aksi" dan link per baris

**Hasil yang Diharapkan**:
- Langkah 3: URL `/transaksi/penjualan/bm-alerts`; halaman tampil
- Langkah 4: Kolom: Kode Instrumen, Portofolio, BM Status, Trigger Event, Tanggal Trigger, Rekomendasi
  - Data BM alert PORT-SAHAM tampil dengan status POTENTIAL_RECLASSIFICATION
  - Tombol filter dan export tersedia
- Langkah 6 (USR-RISK-001): Link "Review BM Assessment" tampil per baris → href: `/master/portofolio/{id}/bm-assessment`
  - ROLE-RISK bisa navigasi ke APP-A territory untuk review BM

**Verifikasi Audit** (untuk export):
```sql
SELECT * FROM aud.audit_log WHERE action = 'PENJUALAN_BM_ALERT.EXPORT' ORDER BY event_time DESC LIMIT 1;
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-03-AC3: Form penjualan baru — BM warning banner (informational, tidak blok)

**Actor**: USR-MAKER-001

**Langkah**:
1. Login sebagai USR-MAKER-001, navigasi ke `/transaksi/penjualan/new`
2. Pilih instrumen SHM-0099 (instrumen dengan BM alert aktif)
3. Perhatikan halaman setelah memilih instrumen
4. Coba klik "Simpan" / submit form

**Hasil yang Diharapkan**:
- Langkah 3: Banner/card informational tampil: "Perhatian: penjualan instrumen ini mungkin berdampak pada Business Model portfolio. Konsultasikan dengan Risk Officer."
- Banner berwarna amber/kuning (warning, bukan merah/error)
- Langkah 4: Tombol submit **tetap enabled** — warning tidak memblok create; form dapat di-submit normal
- Toast sukses muncul jika validasi lulus

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-03-AC4: SoD enforcement penjualan — maker tidak bisa approve

**Actor**: USR-MAKER-002 (maker PJL-TEST-001), USR-APPR-001

**Langkah**:
1. Login sebagai USR-MAKER-002, navigasi ke `/transaksi/penjualan/PJL-TEST-001`
2. Inspect DOM untuk tombol approve
3. Logout → Login sebagai USR-APPR-001
4. Navigasi ke `/transaksi/penjualan/PJL-TEST-001`
5. Klik "Approve" → isi komentar → konfirmasi
6. Sebagai USR-MAKER-002: POST langsung ke API `/api/v1/transaksi/penjualan/PJL-TEST-001/approve`

**Hasil yang Diharapkan**:
- Langkah 2: Tombol "Approve" **TIDAK ADA** di DOM untuk USR-MAKER-002
- Langkah 5: USR-APPR-001 berhasil approve → toast: "Penjualan PJL-TEST-001 berhasil di-approve."
- Langkah 6: Response `HTTP 403 SOD_VIOLATION`

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
