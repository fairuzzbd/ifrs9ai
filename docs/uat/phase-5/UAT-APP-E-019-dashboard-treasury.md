# UAT-APP-E-019 — Treasury Dashboard /dashboard/treasury

**UAT ID**: UAT-APP-E-019
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-01
**AC yang dicakup**: M15-01-AC1, M15-01-AC2, M15-01-AC3, M15-01-AC4
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — route guard absent-from-DOM; JWT permission check server component

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M14 deployed — `GET /api/v1/reports/rpt-01`, `rpt-06`, `rpt-10`, `rpt-26` semua return 200
3. Next.js middleware M15 deployed; route `/dashboard/treasury` dilindungi permission `dashboard.treasury.read`
4. Data seed:
   - `mst.instrumen`: minimal 10 instrumen aktif (campuran DEPOSITO, OBLIGASI, SAHAM, REKSADANA)
   - `mst.instrumen` terdistribusi ke minimal 3 bank/counterparty berbeda
   - `trx.penempatan`: minimal 5 record dengan `tanggal_jatuh_tempo` dalam 90 hari ke depan
   - Workflow pending: minimal 1 record di `rpt-26` dengan status PENDING
5. User test:

| User ID | Role | Permission `dashboard.treasury.read` | MFA |
|---|---|---|---|
| USR-MAKER-001 | ROLE-MAKER-TR | Ya | Tidak |
| USR-APPR-001 | ROLE-APPR-TR | Ya | Tidak |
| USR-RISK-001 | ROLE-RISK | TIDAK | Tidak |
| USR-AKUN-001 | ROLE-AKUN | TIDAK | Tidak |

---

## Data Test Numerik (SoW Example)

Berdasarkan seed data:
- Total portofolio EAD IDR: Rp 500.000.000.000 (Rp 500 M) — 2.600 instrumen
- Eksposur by jenis: DEPOSITO Rp 200 M, OBLIGASI Rp 150 M, SAHAM Rp 100 M, REKSADANA Rp 50 M
- Jatuh tempo 30 hari: Rp 12 M (3 instrumen); 60 hari: Rp 8 M; 90 hari: Rp 3 M
- Pending workflow: 3 dokumen

---

## Skenario UAT

### TC-001 — M15-01-AC1: Widget load benar dari RPT-01 + RPT-10

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR)
**Pre-kondisi**: USR-MAKER-001 ter-autentikasi; data seed aktif

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Navigasi ke `/dashboard/treasury`
3. Tunggu halaman selesai loading (≤ 3 detik)
4. Perhatikan widget W-TR-01 Eksposur Portfolio (BarChart)
5. Periksa W-TR-03 Upcoming Maturities (AreaChart)
6. Pastikan tooltip muncul saat hover pada bar/area chart

**Hasil yang Diharapkan**:
- Halaman load dalam ≤ 3 detik (semua widget initial render selesai)
- W-TR-01 menampilkan BarChart dengan sumbu X = jenis instrumen; Y = total EAD IDR
  - DEPOSITO: Rp 200 M; OBLIGASI: Rp 150 M; SAHAM: Rp 100 M; REKSADANA: Rp 50 M
  - Nilai diformat dengan abbreviation "Rp X M"
- W-TR-03 menampilkan AreaChart dengan 3 bucket: ≤30 hari, 31-60 hari, 61-90 hari
  - Tooltip menampilkan: kode_instrumen, counterparty, nominal_idr, tanggal_jatuh_tempo
- Saat data sedang di-fetch (initial): skeleton row tampil (bukan blank screen)
- Setelah data loaded: skeleton hilang, data tampil

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-01-AC1: Widget error state + retry

**Actor**: USR-MAKER-001
**Pre-kondisi**: Matikan sementara endpoint `rpt-01` (simulasi server error) ATAU putus koneksi API

**Langkah**:
1. Navigasi ke `/dashboard/treasury` saat endpoint `rpt-01` return 500
2. Perhatikan widget W-TR-01

**Hasil yang Diharapkan**:
- W-TR-01 menampilkan error state: pesan error + tombol "Coba Lagi" (Retry)
- Widget lain yang endpoint-nya masih OK tetap tampil data normal
- Tombol Retry dapat diklik → widget re-fetch → jika endpoint sudah normal, data tampil

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-01-AC1: W-TR-04 Pending Workflow Queue + W-TR-05 Recent Transactions

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/dashboard/treasury`
2. Perhatikan widget W-TR-04 Pending Workflow Queue (DataTable, max 20 rows)
3. Klik salah satu link di kolom Aksi (→)
4. Perhatikan widget W-TR-05 Recent Transactions (20 terbaru)

**Hasil yang Diharapkan**:
- W-TR-04 menampilkan workflow pending dari `rpt-26` dengan kolom: Kode, Tipe, Submitted By, Submitted At, Aksi
  - Badge merah di header: "3 dokumen menunggu approval" (sesuai seed data)
  - Link "Lihat semua workflow →" di footer mengarah ke `/reports/rpt-26`
- Klik link Aksi → navigasi ke halaman detail entity yang sesuai
- W-TR-05 menampilkan 20 transaksi terbaru dengan kolom: kode, jenis, counterparty, nominal, tgl, status

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-01-AC2: Role gate — ROLE-RISK tidak bisa akses /dashboard/treasury

**Actor**: USR-RISK-001 (ROLE-RISK — tanpa permission `dashboard.treasury.read`)

**Langkah**:
1. Login sebagai USR-RISK-001
2. Ketik langsung di address bar: `[URL_UAT]/dashboard/treasury`
3. Tekan Enter
4. Perhatikan hasilnya

**Hasil yang Diharapkan**:
- Browser TIDAK menampilkan halaman Treasury Dashboard
- Terjadi redirect HTTP 302 ke `/dashboard` (yang lalu redirect ke `/dashboard/risk` sebagai role default)
- Widget W-TR-01..W-TR-05 TIDAK ADA di DOM — bukan sekadar hidden (inspect element: tidak ada `data-widget-id="W-TR-*"`)
- Tidak ada request ke `/api/v1/reports/rpt-01`, `rpt-10`, `rpt-26`, `rpt-06` dari session USR-RISK-001 (cek network tab browser)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-01-AC3: 5-menit polling refresh + manual Refresh button

**Actor**: USR-MAKER-001
**Pre-kondisi**: Dashboard terbuka; data sudah loaded

**Langkah** (auto-polling):
1. Buka `/dashboard/treasury`; catat timestamp "Terakhir diperbarui" di header
2. Buka browser Network tab, filter ke `/api/v1/reports/`
3. Tunggu 5 menit (300 detik)
4. Perhatikan Network tab: apakah ada request baru ke endpoint rpt-01, rpt-10, dll.?
5. Catat timestamp "Terakhir diperbarui" setelah polling

**Langkah** (manual refresh):
6. Klik tombol "Refresh" (ikon ↺) di header dashboard
7. Perhatikan: a) spinner berputar saat re-fetch; b) timestamp diperbarui

**Hasil yang Diharapkan**:
- Setelah 5 menit: semua widget melakukan re-fetch otomatis ke endpoint masing-masing
- Timestamp "Terakhir diperbarui" berubah ke waktu re-fetch terkini
- Klik Refresh manual → spinner aktif → re-fetch seketika → timestamp baru
- Tab browser di-minimize (background) → polling berhenti (tidak ada request di Network tab)
- Tab di-buka kembali → polling resume dalam ≤ 5 menit

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-01-AC4: Aksesibilitas WCAG AA + ARIA labels

**Actor**: USR-MAKER-001 menggunakan screen reader NVDA/VoiceOver atau Chrome Accessibility Inspector
**Tools**: Browser DevTools → Accessibility Inspector; keyboard only (tanpa mouse)

**Langkah** (ARIA):
1. Buka `/dashboard/treasury`
2. Buka DevTools → Accessibility Inspector
3. Inspeksi setiap widget container
4. Hover ke bar chart W-TR-01 — catat aria-label per bar
5. Lihat table header di W-TR-04

**Langkah** (keyboard):
6. Refresh halaman, gunakan keyboard only
7. Tekan Tab berulang kali — navigasikan ke tombol Refresh
8. Tekan Tab ke link aksi di tabel W-TR-04

**Hasil yang Diharapkan**:
- Setiap widget container punya `aria-label="[Nama Widget] — BLIPS Treasury Dashboard"`
- Recharts BarChart W-TR-01: setiap bar punya `aria-label="[jenis_instrumen]: Rp [nilai] EAD total"`
- DataTable W-TR-04: column headers punya `scope="col""`
- Setiap link aksi punya `aria-label="Lihat detail workflow [kode_instrumen]"`
- Chart tidak hanya bergantung warna — label teks tampil langsung di bar atau legend
- WCAG: contrast ratio ≥ 4.5:1 (cek via axe DevTools atau Chrome contrast checker)
- Keyboard: Tab dapat fokus ke tombol Refresh dan link di DataTable

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-01-AC4: W-TR-02 Eksposur by Bank/Counterparty

**Actor**: USR-MAKER-001

**Langkah**:
1. Navigasi ke `/dashboard/treasury`
2. Perhatikan widget W-TR-02 (BarChart horizontal)

**Hasil yang Diharapkan**:
- BarChart horizontal menampilkan eksposur per bank/counterparty
- Minimal 3 bank dari seed data tampil: BCA Rp 120 M, BNI Rp 90 M, BRI Rp 80 M (nilai sesuai seed)
- Link "Lihat RPT-01 →" ada di footer widget

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

Untuk setiap skenario yang melibatkan navigasi ke dashboard:
- Dashboard view read-only: **tidak ada** `aud.audit_log` row untuk view (`GET`) per DEC-018
- Exception: `/dashboard/treasury` view — tidak di-audit (hanya info log Loki)
- Export dari widget (jika ada) → `aud.audit_log` `action='EXPORT.GENERATED'` harus ada

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-01-AC1 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-01-AC1 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-01-AC1 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-01-AC2 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-01-AC3 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-006 | M15-01-AC4 | USR-MAKER-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-01-AC1 | USR-MAKER-001 | ☐ Pass ☐ Fail |

**Total: 7 TC covering all 4 AC (M15-01-AC1..AC4)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security) | | | |
| Approver (IT-Admin/PM) | | | |
