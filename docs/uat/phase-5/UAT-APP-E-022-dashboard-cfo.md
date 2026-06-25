# UAT-APP-E-022 — CFO+Direksi Dashboard /dashboard/cfo

**UAT ID**: UAT-APP-E-022
**Modul**: APP-E — Reporting & Dashboard
**Story Set**: P5-M15 / Story P5-M15-04
**AC yang dicakup**: M15-04-AC1, M15-04-AC2, M15-04-AC3, M15-04-AC4
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: security-engineer BLOCKING — MFA gate server component; JWT `mfa_verified` check; absent-from-DOM

---

## Pre-Kondisi

1. Environment UAT berjalan (`docker compose -f deploy/docker-compose.uat.yml up -d`)
2. M14 deployed — `GET /api/v1/reports/rpt-01`, `rpt-13`, `rpt-18`, `rpt-23`, `rpt-27` return 200
3. Data seed:
   - `ecl.ecl_calc_result_line`: total EAD IDR = Rp 500.000.000.000; ECL weighted = Rp 12.500.000.000
   - Stage 3 EAD = Rp 7.500.000.000 (1.50% dari total)
   - Bobot skenario aktif: Good 0.25, Normal 0.50, Bad 0.25 (ALCO-approved)
   - `rpt-27`: ECL Good = Rp 10.200.000.000; ECL Normal = Rp 12.500.000.000; ECL Bad = Rp 15.800.000.000
   - `mst.periode_buku`: PRD-2026-06 status = OPEN
   - `ecl.ecl_roll_forward`: kumulatif MTD = Rp 2.100.000.000; YTD = Rp 12.500.000.000
4. User test:

| User ID | Role | `dashboard.cfo.read` | `mfa_verified` | MFA |
|---|---|---|---|---|
| USR-CFO-001 | ROLE-CFO | Ya | true | Ya |
| USR-CFO-002 | ROLE-CFO | Ya | **false** | Tidak (belum MFA) |
| USR-CEO-001 | ROLE-CEO | Ya | true | Ya |
| USR-ALCO-001 | ROLE-ALCO | Ya | true | Ya |
| USR-AKUN-001 | ROLE-AKUN | TIDAK | false | Tidak |

---

## Data Test Numerik

- Total Portfolio NAV: Rp 500.000.000.000 (Rp 500 M) — 2.600 instrumen aktif
- ECL Coverage Ratio: 2.50% = ECL Rp 12,5 M / EAD Rp 500 M
- Stage 3 Ratio: 1.50% = EAD Rp 7,5 M / Total EAD Rp 500 M → status hijau (< 2%)
- Skenario: Good Rp 10,2 M; Normal Rp 12,5 M; Bad Rp 15,8 M — bobot G25%/N50%/B25%
- P&L MTD: +Rp 2,1 M impairment; YTD: +Rp 12,5 M impairment
- Periode aktif: PRD-2026-06 — STATUS: OPEN

---

## Skenario UAT

### TC-001 — M15-04-AC1: W-CF-01 Total Portfolio NAV data benar

**Actor**: USR-CFO-001 (ROLE-CFO, `mfa_verified=true`)

**Langkah**:
1. Login sebagai USR-CFO-001 (sudah MFA)
2. Navigasi ke `/dashboard/cfo`
3. Perhatikan KPI card W-CF-01 Total Portfolio NAV

**Hasil yang Diharapkan**:
- KPI card: "Total NAV Portfolio: Rp 500.000.000.000 (Rp 500 M)"
- Sub-label: "Berdasarkan 2.600 instrumen aktif — per 25 Jun 2026" (tanggal calc run terakhir)
- Data diambil dari `GET /api/v1/reports/rpt-01?filter[status]=AKTIF&limit=200` — sum EAD dilakukan client-side

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M15-04-AC1: W-CF-02 ECL Coverage Ratio + W-CF-04 Stage 3 Ratio

**Actor**: USR-CFO-001

**Langkah**:
1. Navigasi ke `/dashboard/cfo`
2. Perhatikan KPI card W-CF-02 ECL Coverage Ratio
3. Hover pada BarChart 3 bars (Stage 1/2/3)
4. Perhatikan W-CF-04 Stage 3 Ratio

**Hasil yang Diharapkan**:
- W-CF-02 KPI card: "ECL Coverage Ratio: 2.50% (ECL Rp 12,5 M / EAD Rp 500 M)"
- BarChart: 3 bars Stage 1/2/3; Y = ECL/EAD ratio per stage
- Tooltip: "Stage [N]: ECL Rp [x] / EAD Rp [y] = [z]%"
- W-CF-04 KPI card: "Stage 3 Ratio: 1.50% (EAD Rp 7,5 M / Total EAD Rp 500 M)"
- Status: hijau (< 2% threshold) — badge atau warna hijau di card
- Trend arrow: perbandingan dengan periode sebelumnya (↑ / ↓ / →)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — M15-04-AC2: W-CF-03 Scenario Sensitivity Summary (RPT-27)

**Actor**: USR-CFO-001

**Langkah**:
1. Navigasi ke `/dashboard/cfo`
2. Perhatikan widget W-CF-03 Scenario Sensitivity Summary (BarChart 3 bars)
3. Hover pada setiap bar
4. Perhatikan sub-label bobot skenario

**Hasil yang Diharapkan**:
- 3 bars: "Optimis (Good)", "Base (Normal)", "Pesimis (Bad)"
- Nilai: Good = Rp 10,2 M; Normal = Rp 12,5 M; Bad = Rp 15,8 M
- Label di atas setiap bar
- Tooltip: nilai + delta vs weighted: mis. "+Rp 3,3 M vs base scenario" (Bad vs Normal)
- Sub-label: "Bobot skenario aktif: Good 25% / Normal 50% / Bad 25% (ALCO-approved)"
- Link "Lihat RPT-27 →" ke laporan sensitivity M14

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-004 — M15-04-AC2: W-CF-06 Hard-Close Status Periode

**Actor**: USR-CFO-001

**Langkah**:
1. Navigasi ke `/dashboard/cfo`
2. Perhatikan KPI card W-CF-06 Hard-Close Status Periode
3. Klik tombol "Proses Hard-Close →"

**Hasil yang Diharapkan**:
- Status card: "Periode PRD-2026-06 — STATUS: OPEN (belum hard-close)"
- Tombol "Lihat Detail" → `/reports/rpt-23`
- Link "Proses Hard-Close →" → `/periode-buku/{id}/hardclose` (navigasi berhasil, meskipun aksi hard-close di luar scope M15)
- Badge: tidak ada badge hijau "HARD CLOSED" (periode masih OPEN)

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-005 — M15-04-AC3: MFA gate — ROLE-CFO tanpa mfa_verified tidak bisa akses

**Actor**: USR-CFO-002 (ROLE-CFO, `mfa_verified=false`)

**Langkah**:
1. Login sebagai USR-CFO-002 (autentikasi OK tapi BELUM MFA)
2. Navigasi langsung ke `/dashboard/cfo`
3. Perhatikan hasilnya

**Hasil yang Diharapkan**:
- Server component mendeteksi `mfa_verified=false` dalam JWT
- HTTP 302 redirect ke `/auth/mfa?returnUrl=/dashboard/cfo`
- Halaman MFA tampil — user diminta verifikasi MFA
- Widget W-CF-01..W-CF-06 TIDAK ADA di DOM (halaman tidak pernah mencapai widget render)
- Network tab: tidak ada request ke `rpt-01`, `rpt-13`, `rpt-18`, `rpt-23`, `rpt-27` dari sesi ini

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-006 — M15-04-AC3: Role gate — ROLE-AKUN tanpa permission tidak bisa akses

**Actor**: USR-AKUN-001 (ROLE-AKUN — tanpa `dashboard.cfo.read`)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Ketik langsung: `[URL_UAT]/dashboard/cfo`
3. Perhatikan hasilnya

**Hasil yang Diharapkan**:
- Redirect ke `/dashboard/akuntansi` (role default ROLE-AKUN)
- Widget W-CF-01..W-CF-06 TIDAK ADA di DOM
- Network tab: tidak ada request ke endpoint rpt-01, rpt-13, rpt-18, rpt-23, rpt-27

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-007 — M15-04-AC3: ROLE-CEO, ROLE-KOMITE, ROLE-ALCO dapat akses /dashboard/cfo

**Actor**: USR-CEO-001 atau USR-ALCO-001

**Langkah**:
1. Login sebagai USR-ALCO-001 (dengan MFA selesai)
2. Navigasi ke `/dashboard/cfo`
3. Verifikasi dashboard tampil

**Hasil yang Diharapkan**:
- Dashboard CFO tampil lengkap (semua widget)
- Badge "MFA Terverifikasi" tampil di page header
- Data sama seperti USR-CFO-001 melihat

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-008 — M15-04-AC4: W-CF-05 P&L ECL Impact MTD/YTD AreaChart

**Actor**: USR-CFO-001

**Langkah**:
1. Navigasi ke `/dashboard/cfo`
2. Perhatikan widget W-CF-05 P&L ECL Impact MTD & YTD (AreaChart)
3. Perhatikan label MTD dan YTD
4. Klik "Lihat RPT-18 →"

**Hasil yang Diharapkan**:
- AreaChart dengan 2 series: MTD cumulative, YTD cumulative
- X-axis: tanggal (daily untuk MTD; monthly untuk YTD — Jun 2026)
- Y-axis: nominal IDR ECL movement (+ = penambahan, - = reversal)
- Reference line pada Y=0
- Label: "MTD: +Rp 2,1 M impairment" dan "YTD: +Rp 12,5 M impairment"
- Link "Lihat RPT-18 →" → laporan roll-forward M14

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-009 — M15-04-AC4: Aksesibilitas CFO Dashboard

**Actor**: USR-CFO-001
**Tools**: DevTools Accessibility Inspector; keyboard only

**Langkah**:
1. Navigasi ke `/dashboard/cfo`
2. Inspeksi KPI cards via DevTools
3. Hover pada nilai Rupiah abbreviated — inspeksi aria-label
4. Navigasi keyboard — Tab ke tombol Refresh

**Hasil yang Diharapkan**:
- KPI cards: `role="status"` atau `aria-live="polite"` (untuk auto-refresh setiap 5 menit)
- Tombol Refresh: `aria-label="Perbarui semua data dashboard CFO"` (atau serupa deskriptif)
- Nilai abbreviated "Rp 500 M": aria-label menyebut nilai penuh — mis. "Lima ratus miliar rupiah" atau "500.000.000.000"
- Keyboard Tab dari header → tombol Refresh → widget pertama → link dalam widget

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Audit Checks

- Dashboard view read-only: tidak ada `aud.audit_log` per view
- `/dashboard/cfo` view → Loki log untuk monitoring (`ROLE-CFO accessed /dashboard/cfo at {timestamp}`)
- Jika ada export: `aud.audit_log` `action='EXPORT.GENERATED'`
- MFA step-up tidak dibutuhkan untuk VIEW dashboard — hanya untuk action (hard-close dll.) yang ada di modul lain

---

## Rollback / Cleanup

- Kembalikan `sys.fx_rate` ke state normal setelah TC-005 (STALE scenario)
- Tidak ada data yang diubah oleh dashboard view (read-only)

---

## Ringkasan TC

| TC | AC | Actor | Status |
|---|---|---|---|
| TC-001 | M15-04-AC1 | USR-CFO-001 | ☐ Pass ☐ Fail |
| TC-002 | M15-04-AC1 | USR-CFO-001 | ☐ Pass ☐ Fail |
| TC-003 | M15-04-AC2 | USR-CFO-001 | ☐ Pass ☐ Fail |
| TC-004 | M15-04-AC2 | USR-CFO-001 | ☐ Pass ☐ Fail |
| TC-005 | M15-04-AC3 | USR-CFO-002 | ☐ Pass ☐ Fail |
| TC-006 | M15-04-AC3 | USR-AKUN-001 | ☐ Pass ☐ Fail |
| TC-007 | M15-04-AC3 | USR-ALCO-001 | ☐ Pass ☐ Fail |
| TC-008 | M15-04-AC4 | USR-CFO-001 | ☐ Pass ☐ Fail |
| TC-009 | M15-04-AC4 | USR-CFO-001 | ☐ Pass ☐ Fail |

**Total: 9 TC covering all 4 AC (M15-04-AC1..AC4)**

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Security — BLOCKING) | | | |
| Approver (CFO/IT-Admin/PM) | | | |
