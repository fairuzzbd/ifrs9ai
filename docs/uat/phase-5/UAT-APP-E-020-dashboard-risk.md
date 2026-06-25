# UAT-APP-E-020 — Risk Dashboard /dashboard/risk

**UAT ID**: UAT-APP-E-020
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-02
**AC yang dicakup**: M15-02-AC1, M15-02-AC2, M15-02-AC3, M15-02-AC4
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — role-gated routing absent-from-DOM; SSE auth; JWT permission check

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M14 deployed — `GET /api/v1/reports/rpt-13`, `rpt-14`, `rpt-15`, `rpt-26` return 200
3. M13 deployed — `GET /api/v1/jobs/{jobId}` + SSE stream aktif
4. Data seed:
   - `ecl.ecl_calc_result_line`: 2.600 instrumen; Stage 1: 2.400; Stage 2: 180; Stage 3: 20
   - Total ECL weighted = Rp 12.500.000.000; calc_run_id = `CR-2026-06`
   - `ecl.sicr_trigger_log`: 12 events bulan ini — 5 RATING_DOWNGRADE, 4 IG_TO_NONIG, 3 DPD_30
   - `sys.job`: ECL calc run terakhir `JOB-ECL-2026-06` — satu dalam status `running` (progress 47%), satu dalam status `completed`
5. User test:

| User ID | Role | Permission `dashboard.risk.read` | MFA |
|---|---|---|---|
| USR-RISK-001 | ROLE-RISK | Ya | Tidak |
| USR-AKUN-001 | ROLE-AKUN | TIDAK | Tidak |
| USR-MAKER-001 | ROLE-MAKER-TR | TIDAK | Tidak |

---

## Data Test Numerik

- ECL Stage Distribution: Stage 1 = 92.3% (2.400), Stage 2 = 6.9% (180), Stage 3 = 0.8% (20)
- Total instrumen: 2.600 (center label donut)
- SICR triggers bulan ini: Rating Downgrade ≥ 2 notch = 5; IG → Non-IG = 4; DPD ≥ 30 = 3
- Calc-run active: JOB-ECL-2026-06, progress 47%, step: "Menghitung Stage 2 instruments (1234 dari 2600)"
- ETA: 10:35:00 (5 menit dari start 10:30:00)

---

## Skenario UAT

### TC-001 — M15-02-AC1: W-RK-01 ECL Stage Distribution Donut data benar

**Actor**: USR-RISK-001 (ROLE-RISK)

**Langkah**:
1. Login sebagai USR-RISK-001
2. Navigasi ke `/dashboard/risk`
3. Perhatikan widget W-RK-01 ECL Stage Distribution (PieChart/Donut)
4. Hover pada setiap slice donut

**Hasil yang Diharapkan**:
- PieChart donut dengan `innerRadius=60` tampil
- Center label: "Total: 2.600 instrumen"
- Slice: Stage 1 = 92.3%, Stage 2 = 6.9%, Stage 3 = 0.8%
- Tooltip per slice: "Stage [N]: [count] instrumen — ECL total: Rp [sum_ecl_weighted]"
- Legend bawah: "Stage 1 (Performing)", "Stage 2 (SICR)", "Stage 3 (Default)" dengan warna hijau/kuning/merah

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-02-AC1: W-RK-04 Top-10 Instrumen by ECL Weighted

**Actor**: USR-RISK-001

**Langkah**:
1. Navigasi ke `/dashboard/risk`
2. Perhatikan widget W-RK-04 Top-10 Instrumen by ECL Weighted (DataTable)
3. Klik link instrumen pada baris pertama (ECL tertinggi)

**Hasil yang Diharapkan**:
- DataTable 10 baris diurut descending by `ecl_weighted`
- Kolom: kode_instrumen, nama, stage, ead_idr, ecl_weighted, fl_multiplier (worst scenario)
- Nilai `ecl_weighted` tertinggi ada di baris pertama
- Klik link instrumen → navigasi ke `/reports/rpt-13?filter[instrumen_id]={id}`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-02-AC1: W-RK-02 SICR Triggers Counter

**Actor**: USR-RISK-001

**Langkah**:
1. Navigasi ke `/dashboard/risk`
2. Perhatikan widget W-RK-02 SICR Triggers Counter (KPI cards per trigger type)
3. Klik link "Lihat RPT-15 →"

**Hasil yang Diharapkan**:
- 3 KPI cards: "Rating Downgrade ≥ 2 notch: 5", "IG → Non-IG: 4", "DPD ≥ 30: 3"
- Link "Lihat RPT-15 →" mengarah ke laporan SICR M14
- Data diambil dari `GET /api/v1/reports/rpt-15?filter[periode_id]={current_periode}&sort=tanggal_trigger:desc&limit=50`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-02-AC2: W-RK-05 Calc-Run Status dengan active job (SSE live progress)

**Actor**: USR-RISK-001
**Pre-kondisi**: `JOB-ECL-2026-06` status=`running`, progress=47

**Langkah**:
1. Login USR-RISK-001; navigasi ke `/dashboard/risk`
2. Perhatikan widget W-RK-05 Calc-Run Status
3. Tunggu progress bar berubah (SSE push atau polling fallback 2 detik)
4. Tunggu hingga job selesai (status=`completed`)

**Hasil yang Diharapkan**:
- JobProgressPanel tampil: progress bar 47%; step: "Menghitung Stage 2 instruments (1234 dari 2600)"
- ETA: "Estimasi selesai: 10:35:00 (5 menit lagi)"
- Tombol "Batalkan" TIDAK ADA di DOM (ROLE-RISK tidak punya permission `job.cancel`)
- Saat job selesai (SSE event `completed` atau polling deteksi status=completed):
  - W-RK-05 update ke: "Last Run: CR-2026-06 — COMPLETED {timestamp} — 2.600 instrumen diproses"
  - Toast success muncul: "ECL Calc Run CR-2026-06 selesai. Total ECL weighted: Rp 12.500.000.000."
  - Link "Lihat detail →" ada di toast → klik → `/reports/rpt-13?filter[calc_run_id]=CR-2026-06`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-02-AC2: W-RK-05 KPI card saat tidak ada active job

**Actor**: USR-RISK-001
**Pre-kondisi**: Tidak ada job status=`running` atau `queued`; job terakhir status=`completed`

**Langkah**:
1. Navigasi ke `/dashboard/risk`
2. Perhatikan widget W-RK-05

**Hasil yang Diharapkan**:
- Bukan JobProgressPanel — menampilkan KPI card: "Last Run: CR-2026-05 — COMPLETED — {timestamp}"
- Data di-fetch via `GET /api/v1/jobs/{latest_jobId}` dengan 5-menit polling
- Link "Lihat detail →" ke `/reports/rpt-13`

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-02-AC3: Role gate — ROLE-AKUN tidak bisa akses /dashboard/risk

**Actor**: USR-AKUN-001 (ROLE-AKUN — tanpa `dashboard.risk.read`)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Ketik langsung: `[URL_UAT]/dashboard/risk`
3. Perhatikan hasilnya
4. Buka Network tab — cek apakah ada request ke `rpt-13`, `rpt-14`, `rpt-15`

**Hasil yang Diharapkan**:
- Redirect ke `/dashboard/akuntansi` (role default ROLE-AKUN)
- Widget W-RK-01..W-RK-05 TIDAK ADA di DOM (inspect element: tidak ada `data-widget-id="W-RK-*"`)
- Network tab: tidak ada request ke `rpt-13`, `rpt-14`, `rpt-15` dari sesi ini

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-02-AC4: W-RK-03 Stage Movement Trend + aksesibilitas

**Actor**: USR-RISK-001

**Langkah**:
1. Navigasi ke `/dashboard/risk`
2. Perhatikan widget W-RK-03 Stage Movement Trend (LineChart)
3. Buka DevTools → Accessibility Inspector → inspeksi chart SVG
4. Hover pada data point di chart

**Hasil yang Diharapkan**:
- LineChart tampil dengan 3 series (S1=hijau, S2=kuning, S3=merah)
- X-axis: label periode (Jan 2026..Jun 2026 — 6 periode terakhir)
- Y-axis: jumlah instrumen
- Legend: "Stage 1 (Performing)", "Stage 2 (SICR)", "Stage 3 (Default)"
- `aria-label="Tren Perpindahan Stage ECL — 6 periode terakhir"` ada di chart wrapper
- Data point tooltip/aria-label: "[Periode]: Stage [N] = [count] instrumen"
- Warna: Stage 1 = `#16a34a` (hijau), Stage 2 = `#ca8a04` (kuning), Stage 3 = `#dc2626` (merah)
- WCAG contrast: setiap warna ≥ 4.5:1 kontras terhadap background putih (verifikasi via contrast checker)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-008 — M15-02-AC4: SSE fallback ke polling jika SSE error

**Actor**: USR-RISK-001
**Pre-kondisi**: Simulasi SSE unavailable — matikan Redis atau block SSE endpoint di proxy

**Langkah**:
1. Matikan Redis sementara ATAU block `GET /api/v1/jobs/{jobId}/stream` di network level
2. Login USR-RISK-001; navigasi ke `/dashboard/risk`
3. Perhatikan W-RK-05

**Hasil yang Diharapkan**:
- Widget tidak blank/crash — fallback ke polling setiap 2 detik
- Widget masih update progress (dari REST endpoint `GET /api/v1/jobs/{jobId}`)
- Backend return `SSE_STREAM_UNAVAILABLE` 503 saat SSE tidak tersedia

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

- Dashboard view read-only: tidak ada `aud.audit_log` row per view (DEC-018)
- `/dashboard/risk` view — Loki log saja (monitoring), bukan `aud.audit_log`
- Jika ada export dari widget: `aud.audit_log` `action='EXPORT.GENERATED'` harus ada

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-02-AC1 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-02-AC1 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-02-AC1 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-02-AC2 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-02-AC2 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-006 | M15-02-AC3 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-02-AC4 | USR-RISK-001 | ☐ Pass ☐ Fail |
| TC-008 | M15-02-AC4 | USR-RISK-001 | ☐ Pass ☐ Fail |

**Total: 8 TC covering all 4 AC (M15-02-AC1..AC4)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security) | | | |
| Approver (IT-Admin/PM) | | | |
