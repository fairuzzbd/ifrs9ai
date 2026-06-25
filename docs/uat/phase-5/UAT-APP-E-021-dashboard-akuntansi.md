# UAT-APP-E-021 — Akuntansi Dashboard /dashboard/akuntansi

**UAT ID**: UAT-APP-E-021
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-03
**AC yang dicakup**: M15-03-AC1, M15-03-AC2, M15-03-AC3, M15-03-AC4
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — SoD button gate server-side; absent-from-DOM; JWT permission check

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M14 deployed — `GET /api/v1/reports/rpt-22`, `rpt-22b`, `rpt-05`, `rpt-26`, `rpt-23` return 200
3. M12 deployed — `jrnl.jurnal_header` ter-populate
4. M3 deployed — `jrnl.gl_delivery` ter-populate
5. Data seed:
   - `jrnl.jurnal_header`: 15 jurnal status=`PENDING_APPROVAL` untuk periode PRD-2026-06
   - `jrnl.gl_delivery`: 980 DELIVERED, 15 FAILED, 5 PENDING (total 1.000)
   - `sys.fx_rate`: entry terakhir pada hari ini (FRESH) — USD 16.250 sumber JISDOR
   - `mst.periode_buku`: PRD-2026-06 OPEN (current); PRD-2026-05 HARD_CLOSED; PRD-2026-04 HARD_CLOSED; PRD-2026-03 SOFT_CLOSED
6. User test:

| User ID | Role | `dashboard.akuntansi.read` | `jurnal.approve` | MFA |
|---|---|---|---|---|
| USR-AKUN-001 | ROLE-AKUN | Ya | TIDAK | Tidak |
| USR-CTL-001 | ROLE-AKUN-CTL | Ya | Ya | Ya |
| USR-RISK-001 | ROLE-RISK | TIDAK | TIDAK | Tidak |

---

## Data Test Numerik

- Jurnal pending: 15 jurnal (JRN-001234..JRN-001248) status PENDING_APPROVAL
- GL delivery success rate: 98.0% (980 dari 1.000)
- FX Rate fresh: tanggal hari ini = 2026-06-25; USD 16.250; sumber JISDOR
- FX Rate stale scenario: entry terakhir tanggal 2026-06-24 (kemarin)
- GL failure threshold alert: > 5% (scenario test: FAILED = 90 dari 1.000 = 9%)

---

## Skenario UAT

### TC-001 — M15-03-AC1: ROLE-AKUN melihat W-AK-01 Jurnal Pending — Approve button ABSENT

**Actor**: USR-AKUN-001 (ROLE-AKUN — tanpa `jurnal.approve`)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Navigasi ke `/dashboard/akuntansi`
3. Perhatikan widget W-AK-01 Jurnal Menunggu Posting
4. Inspect element: cari button dengan teks "Approve" atau "Setujui"

**Hasil yang Diharapkan**:
- W-AK-01 tampil dengan 15 baris jurnal pending
- Badge merah di header: "15 jurnal menunggu approval"
- Kolom: Jurnal ID, Kode Event, Instrumen ID, Nominal IDR, Submitted By, Submitted At, Status, Aksi
- Tombol "Approve" / "Setujui" TIDAK ADA di DOM (inspect element: tidak ada `<button>` dengan teks approve)
- Link Aksi (→) mengarah ke detail jurnal (view only)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-03-AC1: ROLE-AKUN-CTL melihat W-AK-01 — Approve button VISIBLE

**Actor**: USR-CTL-001 (ROLE-AKUN-CTL — dengan `jurnal.approve`)

**Langkah**:
1. Login sebagai USR-CTL-001
2. Navigasi ke `/dashboard/akuntansi`
3. Perhatikan widget W-AK-01
4. Klik tombol "Approve" pada baris jurnal JRN-001234

**Hasil yang Diharapkan**:
- Tombol "Approve" VISIBLE untuk setiap baris jurnal pending
- Klik "Approve" → redirect ke `/mapping-jurnal/approve/{jurnal_id}` (di luar scope M15 — hanya redirect)
- Server component check: `jurnal.approve` ada di JWT → button di-render
- Jika `jurnal.approve` TIDAK ada di JWT → button tidak di-render (absent from DOM)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-03-AC1: W-AK-02 GL Delivery Success Rate donut + alert

**Actor**: USR-AKUN-001

**Langkah** (normal — FAILED ≤ 5%):
1. Navigasi ke `/dashboard/akuntansi`
2. Perhatikan widget W-AK-02 GL Delivery Success Rate

**Langkah** (alert scenario — FAILED > 5%):
3. Ubah sementara data seed: FAILED = 90 dari 1.000 (9%)
4. Refresh `/dashboard/akuntansi`

**Hasil yang Diharapkan** (normal):
- PieChart donut: DELIVERED 98.0% (hijau), FAILED 1.5% (merah), PENDING 0.5% (amber)
- KPI card: "Success Rate: 98.0% — 980 dari 1.000 jurnal berhasil dikirim ke GL"
- Tidak ada warning banner

**Hasil yang Diharapkan** (alert scenario):
- KPI card: "Success Rate: 90.0%"
- Banner warning amber: "Peringatan: Tingkat kegagalan GL delivery melebihi 5%"

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-03-AC2: W-AK-03 FX Rate Freshness — status FRESH

**Actor**: USR-AKUN-001
**Pre-kondisi**: `sys.fx_rate` entry terakhir = hari ini (2026-06-25); sumber = JISDOR

**Langkah**:
1. Navigasi ke `/dashboard/akuntansi`
2. Perhatikan widget W-AK-03 FX Rate Freshness (KPI card)

**Hasil yang Diharapkan**:
- KPI card: "FX Rate terakhir: 25 Jun 2026 — USD 16.250"
- Status: [hijau] FRESH
- Sumber: JISDOR
- Tidak ada banner alert
- Riwayat 5 hari terakhir tampil di bawah KPI card

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-03-AC2: W-AK-03 FX Rate Freshness — status STALE + alert + link

**Actor**: USR-AKUN-001
**Pre-kondisi**: Ubah seed data — entry terakhir `sys.fx_rate` = 2026-06-24 (kemarin, sebelum cutoff 10:30)

**Langkah**:
1. Navigasi ke `/dashboard/akuntansi`
2. Perhatikan widget W-AK-03
3. Klik link upload di banner alert

**Hasil yang Diharapkan**:
- KPI card: "FX Rate terakhir: 24 Jun 2026 — USD 16.225"
- Status: [merah] STALE
- Banner merah: "FX Rate belum diperbarui hari ini. Upload manual via Pengaturan > FX Rate."
- Link di banner → navigasi ke halaman upload FX rate manual
- Data source: `GET /api/v1/reports/rpt-05?sort=tanggal:desc&limit=5`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-03-AC2: W-AK-04 Periode Buku Timeline

**Actor**: USR-AKUN-001

**Langkah**:
1. Navigasi ke `/dashboard/akuntansi`
2. Perhatikan widget W-AK-04 Periode Buku Timeline (BarChart horizontal)

**Hasil yang Diharapkan**:
- Recharts BarChart horizontal: setiap periode = 1 bar
  - PRD-2026-06: warna hijau (OPEN), badge "CURRENT"
  - PRD-2026-05: warna abu-abu (HARD_CLOSED), label tanggal close: "2 Jun 2026"
  - PRD-2026-03: warna amber (SOFT_CLOSED)
- Label setiap bar: "PRD-2026-XX — STATUS + tanggal close"
- Badge "CURRENT" pada periode aktif (rendered di atas bar)
- Link "→ Detail" navigasi ke laporan periode buku M14

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-03-AC3: Role gate — ROLE-RISK tidak bisa akses /dashboard/akuntansi

**Actor**: USR-RISK-001 (ROLE-RISK — tanpa `dashboard.akuntansi.read`)

**Langkah**:
1. Login sebagai USR-RISK-001
2. Ketik langsung: `[URL_UAT]/dashboard/akuntansi`
3. Perhatikan hasilnya
4. Buka Network tab — cek request ke `rpt-26`, `rpt-22`, dll.

**Hasil yang Diharapkan**:
- Redirect ke `/dashboard/risk` (role default ROLE-RISK)
- Widget W-AK-01..W-AK-05 TIDAK ADA di DOM
- Network tab: tidak ada request ke rpt-26, rpt-22, rpt-22b, rpt-05, rpt-23 dari sesi ini

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-008 — M15-03-AC4: W-AK-05 Recent Jurnal Log + empty state

**Actor**: USR-AKUN-001

**Langkah** (normal):
1. Navigasi ke `/dashboard/akuntansi`
2. Perhatikan widget W-AK-05 Recent Jurnal Log (DataTable 20 terbaru)
3. Klik link instrumen_id pada salah satu baris

**Langkah** (empty state):
4. Ubah filter periode ke periode yang tidak memiliki jurnal
5. Perhatikan W-AK-05

**Hasil yang Diharapkan** (normal):
- DataTable 20 baris terbaru, diurut by `posted_at DESC`
- Kolom: jurnal_id, event_code, instrumen_id (link), nominal_idr (right-aligned), posted_at, status_posting
- Klik instrumen_id → `/master/instrumen/{instrumen_id}`

**Hasil yang Diharapkan** (empty state):
- Ilustrasi + "Tidak ada jurnal yang tersedia untuk periode ini."
- Link "Lihat semua jurnal →" → `/reports/rpt-22`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-009 — M15-03-AC4: Aksesibilitas — aria-labels + DataTable kolom IDR

**Actor**: USR-AKUN-001
**Tools**: DevTools → Accessibility Inspector; keyboard only

**Langkah**:
1. Navigasi ke `/dashboard/akuntansi`
2. Buka Accessibility Inspector; inspeksi widget containers
3. Inspeksi kolom "Nominal IDR" di W-AK-01 (atau W-AK-05) — lihat aria-label per cell
4. Verifikasi text-align kolom nominal IDR

**Hasil yang Diharapkan**:
- Setiap widget container: `aria-label="[Nama Widget] — BLIPS Akuntansi Dashboard"`
- Kolom "Nominal IDR": text-align: right
- Cell nominal IDR punya `aria-label="Nominal: Rp [nilai]"` — mis. "Nominal: Rp 900.000"
- Keyboard Tab dapat mencapai tombol "Coba Lagi" (retry) jika ada, link aksi DataTable, tombol Refresh

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

- View dashboard read-only: tidak ada `aud.audit_log` row per DEC-018
- Jika ROLE-AKUN-CTL klik Approve dari widget → navigasi ke halaman approve: audit ada di endpoint approve tersebut (scope M12)
- Export dari widget: `aud.audit_log` `action='EXPORT.GENERATED'`

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-03-AC1 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-03-AC1 | USR-CTL-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-03-AC1 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-03-AC2 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-03-AC2 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-006 | M15-03-AC2 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-03-AC3 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-008 | M15-03-AC4 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-009 | M15-03-AC4 | USR-AKUN-001 | ☐ Pass ☐ Fail |

**Total: 9 TC covering all 4 AC (M15-03-AC1..AC4)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security) | | | |
| Approver (IT-Admin/PM) | | | |
