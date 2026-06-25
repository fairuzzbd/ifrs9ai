# UAT-APP-B-007 — Shared /transaksi Layout: Tab Nav + Breadcrumb + Role-Gated Visibility

**UAT ID**: UAT-APP-B-007
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-05
**AC yang dicakup**: M16-05-AC1, M16-05-AC2, M16-05-AC3, M16-05-AC4
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — absent-from-DOM server component check; tab visibility tidak menggunakan CSS hide/disabled

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. P5-M16 deployed — `frontend/src/app/transaksi/layout.tsx` dan `TransaksiTabNav.tsx` aktif
3. Semua sub-route `/transaksi/*` sudah dapat diakses (hasil M16-01 s/d M16-04)
4. JWT permission seeded di Keycloak per tabel persona di bawah:

| User ID | Role | Permission transaksi | MFA |
|---|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | penempatan.read + .create, transaksi.mtm.read + .upload, renewal.read + .create, penjualan.read + .create, transaksi.jatuh-tempo.read | Tidak |
| USR-APPR-001 | ROLE-APPR-TR | penempatan.read + .review + .approve, renewal.read + .review, penjualan.read + .review, transaksi.jatuh-tempo.read | Tidak |
| USR-AKUN-001 | ROLE-AKUN | transaksi.mtm.read + .upload, transaksi.akrual.read + .create, transaksi.jatuh-tempo.read | Tidak |
| USR-CTL-001 | ROLE-AKUN-CTL | transaksi.akrual.read + .approve, transaksi.jatuh-tempo.read | Ya (MFA) |
| USR-AUDIT-001 | ROLE-AUDIT | penempatan.read, transaksi.mtm.read, renewal.read, penjualan.read, transaksi.jatuh-tempo.read, transaksi.akrual.read | Tidak |
| USR-RISK-001 | ROLE-RISK | penempatan.read, transaksi.mtm.read, renewal.read, penjualan.read, transaksi.jatuh-tempo.read, transaksi.akrual.read | Tidak |

---

## Data Test

Tidak diperlukan data numerik spesifik untuk layout test — fokus pada visibilitas komponen UI dan navigasi.

---

## Skenario UAT

### TC-001 — M16-05-AC1: Tab absent-from-DOM untuk ROLE-AKUN

**Actor**: USR-AKUN-001 (ROLE-AKUN)
**Pre-kondisi**: USR-AKUN-001 ter-autentikasi

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Navigasi ke `/transaksi/akrual`
3. Inspeksi tab navigation menggunakan browser DevTools (F12 → Elements)
4. Perhatikan tab mana saja yang muncul

**Hasil yang Diharapkan**:
- Tab "MTM" **tampil** di tab nav (permission `transaksi.mtm.read` ada)
- Tab "Akrual" **tampil** di tab nav (permission `transaksi.akrual.read` ada)
- Tab "Jatuh Tempo" **tampil** di tab nav
- Tab "Penempatan" **TIDAK ADA** di HTML DOM (bukan disabled — tidak ada elemen sama sekali)
- Tab "Renewal" **TIDAK ADA** di DOM
- Tab "Penjualan" **TIDAK ADA** di DOM
- Inspeksi source HTML: tidak ada HTML comment atau elemen tersembunyi yang mengandung label "Penempatan", "Renewal", atau "Penjualan"

**Verifikasi Keamanan**:
- Buka DevTools → Elements → Cari `penempatan` di DOM → Tidak boleh ada elemen yang contain text ini di dalam tab nav
- Jalankan di konsol: `document.querySelectorAll('[role="tab"]')` → Hanya 3 tab yang kembali (MTM, Jatuh Tempo, Akrual)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-05-AC1: ROLE-AUDIT melihat semua 6 tab

**Actor**: USR-AUDIT-001 (ROLE-AUDIT)
**Pre-kondisi**: USR-AUDIT-001 ter-autentikasi

**Langkah**:
1. Login sebagai USR-AUDIT-001
2. Navigasi ke `/transaksi/penempatan`
3. Perhatikan tab navigation

**Hasil yang Diharapkan**:
- Semua 6 tab tampil: Penempatan, MTM, Renewal, Penjualan, Jatuh Tempo, Akrual
- Tidak ada tombol mutasi (create, upload, trigger, approve) di tab nav atau header layout

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M16-05-AC2: Active tab highlight dan breadcrumb di sub-route

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR)
**Pre-kondisi**: USR-MAKER-001 ter-autentikasi

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/transaksi/renewal/new`
3. Perhatikan tab "Renewal" di tab nav
4. Perhatikan breadcrumb di atas halaman
5. Navigasi ke `/transaksi/penempatan`
6. Perhatikan tab "Penempatan"

**Hasil yang Diharapkan**:
- Di `/transaksi/renewal/new`:
  - Tab "Renewal" memiliki visual active state (border-bottom accent atau background berbeda)
  - Tab "Renewal" memiliki `aria-selected="true"` (verify di DevTools)
  - Tab lain: `aria-selected="false"`
  - Breadcrumb menampilkan: **Beranda / Transaksi / Renewal / Renewal Baru**
  - "Beranda", "Transaksi", "Renewal" adalah link yang bisa diklik
  - "Renewal Baru" bukan link; memiliki `aria-current="page"`
- Di `/transaksi/penempatan`:
  - Tab "Penempatan" active
  - Breadcrumb: **Beranda / Transaksi / Penempatan**
  - "Penempatan" terakhir: `aria-current="page"`, bukan link

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M16-05-AC3: CTA button visible sesuai sub-route dan permission

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR), USR-APPR-001 (ROLE-APPR-TR)

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/transaksi/penempatan`
3. Cek tombol CTA di header layout
4. Navigasi ke `/transaksi/jatuh-tempo`
5. Cek tombol CTA
6. Logout → Login sebagai USR-APPR-001
7. Navigasi ke `/transaksi/penempatan`
8. Cek tombol CTA

**Hasil yang Diharapkan**:
- USR-MAKER-001 di `/transaksi/penempatan`:
  - Tombol "+ Penempatan Baru" tampil
  - `aria-label="Tambah Penempatan Baru"` pada tombol tersebut
  - Klik tombol → navigasi ke `/transaksi/penempatan/new`
- USR-MAKER-001 di `/transaksi/jatuh-tempo`:
  - Tidak ada tombol CTA "Baru" di header layout (jatuh-tempo read-only)
- USR-APPR-001 di `/transaksi/penempatan`:
  - Tombol "+ Penempatan Baru" **TIDAK ADA** di DOM (USR-APPR-001 tidak punya `penempatan.create`)
  - Bukan disabled — absent dari HTML DOM

**Verifikasi Keamanan**:
- DevTools → Elements → Cari `Tambah Penempatan Baru` → Tidak boleh ada untuk USR-APPR-001

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M16-05-AC4: Aksesibilitas keyboard dan ARIA

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR)

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/transaksi/penempatan`
3. Tekan Tab sekali dari awal halaman
4. Periksa elemen pertama yang mendapat fokus
5. Navigasikan ke tab nav menggunakan Tab
6. Tekan Arrow Right
7. Tekan Arrow Left
8. Periksa ARIA attributes di DevTools

**Hasil yang Diharapkan**:
- Tekan Tab pertama kali: fokus jatuh ke "Skip to main content" / "Lewati ke konten utama" link (visually hidden tapi accessible)
- Tab nav: `<nav aria-label="Navigasi Transaksi">` dengan `<ul role="tablist">` di dalamnya
- Setiap tab: `role="tab"` + `aria-selected="true/false"` + `tabindex="0"` (active) atau `tabindex="-1"` (inactive)
- Arrow Right: fokus berpindah ke tab berikutnya di kanan
- Arrow Left: fokus berpindah ke tab sebelumnya
- Tab wrap: Arrow Right dari tab terakhir kembali ke tab pertama
- Main content area: `role="tabpanel"` dan `aria-labelledby` menunjuk ke active tab id
- Breadcrumb nav: `aria-label="Breadcrumb"` + `<ol>` + `aria-current="page"` pada item terakhir
- Screen reader mode: navigasi menggunakan NVDA/VoiceOver — tab labels diumumkan dengan benar

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
