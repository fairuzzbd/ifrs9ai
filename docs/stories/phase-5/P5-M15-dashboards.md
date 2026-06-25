# P5-M15 — APP-E Dashboards per Role: Next.js Role-Gated Dashboards + Job History Page: User Stories

**Story Set ID**: P5-M15
**Modul**: APP-E — Reporting & Dashboard
**Status**: DRAFT — menunggu handoff ke `system-analyst` (SSE aggregator spec); `uiux-designer` (widget layout + WCAG AA); `frontend-engineer-nextjs` (implementasi)
**Author**: business-analyst
**Tanggal**: 2026-06-25
**Linked FSD**: FSD-BLIPS-MASTER-v1.1.docx §6 (APP-E Reporting), §6.2 (Dashboard per Role)
**Linked BRD**: BRD §4.5 (APP-E laporan), §3 RACI: ROLE-AKUN-CTL (A), ROLE-RISK (R), ROLE-CFO (A), ROLE-AUDIT (I)
**Linked Decision Log**:
- `DEC-002` (LOCKED) — Next.js 14+ App Router, TypeScript strict, shadcn/ui, Recharts
- `DEC-007` (LOCKED) — Asynq job queue; SSE job events reuse dari M14
- `DEC-018` (LOCKED) — audit trail append-only; read-only dashboard tidak wajib audit per-view kecuali export
- `DEC-020` (LOCKED) — REST `/api/v1/`; semua dashboard widget consume endpoint yang sudah ada
- `DEC-025` (LOCKED) — JWT 15 min access; role-gated routing enforced server-side middleware
- `DEC-026` (LOCKED) — MFA mandatory: CFO, CEO, KOMITE, ALCO

**Dependensi**:
- **P5-M14** (WAJIB selesai — commit `9e82fd1`) — 25 report endpoints `GET /api/v1/reports/{slug}`; semua widget consume endpoint ini
- **P5-M12** — `jrnl.*` tables (Akuntansi dashboard jurnal widgets)
- **P5-M4** — `mst.periode_buku` (periode buku timeline widget)
- **P5-M13** — `sys.job` table + SSE job stream per-job; `GET /api/v1/jobs/{jobId}/stream` (JobProgressPanel reuse)
- **P5-M3** — `jrnl.gl_delivery` (GL delivery success rate widget)

**Gate**: `security-engineer` **BLOCKING** — role-gated routing absent-from-DOM; JWT permission check di server component; SSE auth. Tidak ada `ifrs9-compliance-reviewer` gate (dashboard read-only, tidak compute ECL/EIR/klasifikasi baru).

---

## Konteks & Scope P5-M15

P5-M15 mengimplementasi **5 role-specific dashboards** dan **1 shared job history page** di Next.js App Router.

**Prinsip utama M15:**
- **Frontend-only modul** — tidak ada migration baru, tidak ada mutating backend endpoint baru.
- Semua data berasal dari **M14 report endpoints** (`GET /api/v1/reports/{slug}`) + **job status endpoint** (`GET /api/v1/jobs/{jobId}`) yang sudah ada.
- **SSE push notifications** via `GET /api/v1/jobs/{jobId}/stream` (per-job SSE M13 pattern) — tidak ada agregator SSE baru di backend; frontend subscribe ke job stream yang relevan saat widget menampilkan active job.
- **5-minute polling fallback** untuk semua widget (jika SSE tidak applicable untuk metric-level data).
- **Role-gated routing** — `/dashboard/treasury`, `/dashboard/risk`, `/dashboard/akuntansi`, `/dashboard/cfo`, `/dashboard/audit` — masing-masing reject 403 + redirect ke `/dashboard` (role default) jika user tidak punya permission.
- **Absent-from-DOM** (bukan sekadar hidden via CSS) untuk widget yang user tidak punya permission.
- **Recharts** untuk semua chart (line, bar, donut, area). Tidak ada 3rd-party chart library lain.
- **DataTable pattern** (UX §1 — sort + paging + filter + export) untuk `/jobs` page.

**Tidak di-scope M15 (eksplisit):**
- Tidak ada mutating endpoint baru
- Tidak ada migration database baru
- Dashboard customization / widget reordering → Phase 6
- Widget drag-and-drop → Phase 6
- Push notification via email untuk dashboard events → M13 SSE sudah handle job notifications
- Dashboard print/export → gunakan laporan M14 langsung
- Mobile-responsive layout → best effort (target desktop-first, ≥ 1280px)

---

## Persona Table

| Role | Dashboard URL | Widget set | MFA |
|---|---|---|---|
| ROLE-MAKER-TR | `/dashboard/treasury` | Eksposur portfolio, jatuh tempo, pending workflow, transaksi recent | Tidak wajib |
| ROLE-APPR-TR | `/dashboard/treasury` | Sama dengan MAKER-TR; widget "Pending Approval Queue" lebih prominent | Tidak wajib (kecuali Treasury Manager senior) |
| ROLE-RISK | `/dashboard/risk` | ECL stage donut, SICR counter, stage movement trend, top-10 ECL, calc-run status | Tidak wajib |
| ROLE-AKUN | `/dashboard/akuntansi` | Jurnal pending, GL delivery rate, FX freshness, periode timeline | Tidak wajib |
| ROLE-AKUN-CTL | `/dashboard/akuntansi` | Sama + kolom "Approve" di jurnal pending widget | WAJIB |
| ROLE-CFO | `/dashboard/cfo` | Total NAV, ECL coverage ratio, sensitivity summary, stage 3 ratio, P&L MTD/YTD, hard-close status | WAJIB |
| ROLE-CEO | `/dashboard/cfo` | Sama dengan CFO (subset — executive summary) | WAJIB |
| ROLE-KOMITE | `/dashboard/cfo` | Sama dengan CFO | WAJIB |
| ROLE-ALCO | `/dashboard/cfo` | Sama dengan CFO | WAJIB |
| ROLE-AUDIT | `/dashboard/audit` | Audit log volume, hash-chain status, SoD violation alerts | Tidak wajib |
| Semua role | `/jobs` | Job history DataTable (filter milik sendiri; IT-ADMIN lihat semua) | Tidak wajib |

---

## Deliverables M15

| # | Artefak | Tipe |
|---|---|---|
| 1 | `web/app/(protected)/dashboard/treasury/page.tsx` | Next.js page |
| 2 | `web/app/(protected)/dashboard/risk/page.tsx` | Next.js page |
| 3 | `web/app/(protected)/dashboard/akuntansi/page.tsx` | Next.js page |
| 4 | `web/app/(protected)/dashboard/cfo/page.tsx` | Next.js page |
| 5 | `web/app/(protected)/dashboard/audit/page.tsx` | Next.js page |
| 6 | `web/app/(protected)/jobs/page.tsx` | Next.js page |
| 7 | `web/components/blips/dashboard/` | Widget components (per story) |
| 8 | `web/hooks/useDashboardPolling.ts` | 5-min polling hook |
| 9 | `web/hooks/useJobSSE.ts` | SSE subscription hook (reuse M13 JobProgressPanel pattern) |
| 10 | `web/middleware.ts` (update) | Role-gated route protection |
| 11 | `web/app/(protected)/dashboard/page.tsx` (update) | Role-based redirect ke dashboard spesifik |

---

## Story P5-M15-01 — Treasury Dashboard (ROLE-MAKER-TR / ROLE-APPR-TR)

**Actor**: ROLE-MAKER-TR (primary), ROLE-APPR-TR (primary — shared dashboard)
**Trigger**: User navigasi ke `/dashboard/treasury` — dashboard load otomatis, 5-menit polling refresh, manual refresh button.
**Goal**: Treasury team mendapat visibility real-time atas eksposur portfolio, jatuh tempo mendatang, pending workflow queue, dan transaksi terkini — tanpa harus membuka laporan M14 secara manual.

**Source RPTs dikonsumsi**:
- **RPT-01** (`rpt-01`) — Daftar Instrumen (widget: Eksposur by counterparty/bank/jenis)
- **RPT-06** (`rpt-06`) — Penempatan Log (widget: Recent Transactions)
- **RPT-10** (`rpt-10`) — Jatuh Tempo Log (widget: Upcoming Maturities next 30/60/90 days)
- **RPT-26** (`rpt-26`) — Workflow Pending Approval (widget: Pending Workflow Queue)

**Widget list**:
| Widget ID | Nama | Chart | Refresh |
|---|---|---|---|
| W-TR-01 | Eksposur Portfolio | Recharts BarChart (by jenis_instrumen) | 5-min polling |
| W-TR-02 | Eksposur by Bank/Counterparty | Recharts BarChart horizontal | 5-min polling |
| W-TR-03 | Upcoming Maturities (30/60/90 hari) | Recharts AreaChart timeline | 5-min polling |
| W-TR-04 | Pending Workflow Queue | DataTable (no paging, max 20 rows) | 5-min polling |
| W-TR-05 | Recent Transactions (20 terbaru) | DataTable | 5-min polling |

### Pre-conditions
1. M14 deployed; `GET /api/v1/reports/rpt-01`, `rpt-06`, `rpt-10`, `rpt-26` semua return 200
2. User ter-autentikasi dengan ROLE-MAKER-TR atau ROLE-APPR-TR; JWT `mfa_verified` sesuai role
3. Next.js middleware M15 deployed; route `/dashboard/treasury` dilindungi permission `dashboard.treasury.read`
4. Data: minimal 1 instrumen aktif di `mst.instrumen`; minimal 1 workflow pending di `rpt-26`

### Acceptance Criteria

```gherkin
Feature: Treasury Dashboard — eksposur, jatuh tempo, workflow queue, recent transactions

  Background:
    Given ROLE-MAKER-TR USR-MAKER-001 ter-autentikasi
    And permission 'dashboard.treasury.read' granted via ROLE-MAKER-TR
    And M14 report endpoints semua return 200

  Scenario: M15-01-AC1 — Widget load: data benar dari RPT-01 + RPT-10; Recharts render
    When USR-MAKER-001 navigasi ke /dashboard/treasury
    Then halaman load dalam < 3 detik (semua widget initial render)
    And W-TR-01 Eksposur Portfolio menampilkan BarChart:
      | data source   | GET /api/v1/reports/rpt-01?filter[status]=AKTIF&sort=ead_idr:desc&limit=200 |
      | X-axis        | jenis_instrumen (DEPOSITO, OBLIGASI, SAHAM, REKSADANA, etc.)               |
      | Y-axis        | total EAD IDR (sum per jenis) — Recharts BarChart                          |
      | format        | currency IDR dengan abbreviation (mis. "Rp 2,5 M")                        |
    And W-TR-03 Upcoming Maturities menampilkan AreaChart:
      | data source   | GET /api/v1/reports/rpt-10?filter[tanggal_jatuh_tempo]=gte:{today}&filter[tanggal_jatuh_tempo]=lte:{today+90d}&sort=tanggal_jatuh_tempo:asc&limit=200 |
      | X-axis        | tanggal_jatuh_tempo (bucketed: 30/60/90 hari)                              |
      | Y-axis        | nominal_idr sum per bucket                                                  |
      | tooltip       | kode_instrumen, counterparty, nominal_idr, tanggal_jatuh_tempo              |
    And setiap widget: loading state = skeleton row (bukan blank screen) selama fetch
    And setiap widget: error state = pesan error + retry button jika fetch gagal

  Scenario: M15-01-AC2 — Role gate: ROLE-RISK tidak bisa akses /dashboard/treasury
    Given ROLE-RISK USR-RISK-001 ter-autentikasi tanpa permission 'dashboard.treasury.read'
    When USR-RISK-001 navigasi ke /dashboard/treasury (direct URL)
    Then Next.js middleware: HTTP 302 redirect ke /dashboard (role default = /dashboard/risk)
    And W-TR-04 Pending Workflow Queue ABSENT dari DOM (tidak di-render, bukan hanya hidden via CSS)
    And server component permission check: permission 'dashboard.treasury.read' tidak ada → redirect
    And tidak ada data endpoint yang di-call untuk USR-RISK-001 (request gagal di middleware, sebelum page render)

  Scenario: M15-01-AC3 — 5-menit polling refresh; manual refresh button
    When USR-MAKER-001 membuka /dashboard/treasury dan menunggu 5 menit
    Then semua widget otomatis re-fetch data dari endpoint masing-masing (polling interval 5 menit = 300_000 ms)
    And indikator "Last updated: {timestamp}" diperbarui di setiap widget header setelah re-fetch sukses
    And tombol "Refresh" manual tersedia di header dashboard — klik trigger re-fetch semua widget seketika
    And polling tidak menggunakan offset pagination — setiap poll ambil dataset fresh (limit terbatas untuk performa)
    And polling continue saat tab browser dalam background (visibilitychange API: pause saat hidden, resume saat visible)

  Scenario: M15-01-AC4 — Aksesibilitas WCAG AA + ARIA labels Bahasa Indonesia
    When USR-MAKER-001 menggunakan screen reader di /dashboard/treasury
    Then setiap widget container punya aria-label="[Nama Widget] — BLIPS Treasury Dashboard"
    And Recharts BarChart W-TR-01: setiap bar punya aria-label="[jenis_instrumen]: Rp [nilai] EAD total"
    And DataTable W-TR-04 Pending Workflow Queue: column headers punya scope="col"; setiap row action link punya aria-label="Lihat detail workflow [kode_instrumen]"
    And warna chart tidak hanya bergantung pada warna (pattern fill atau label langsung) — WCAG contrast ratio ≥ 4.5:1
    And keyboard navigable: Tab key dapat fokus ke setiap widget, tombol Refresh, dan link di DataTable
```

---

## Story P5-M15-02 — Risk Dashboard (ROLE-RISK)

**Actor**: ROLE-RISK (exclusive)
**Trigger**: User navigasi ke `/dashboard/risk` — dashboard load, 5-menit polling, SSE subscribe untuk active calc-run job jika ada.
**Goal**: Risk Officer mendapat visibility real-time atas distribusi ECL stage, SICR triggers terkini, trend stage movement, top-10 instrumen berdasarkan ECL, dan status calc-run terakhir — sebagai pusat kontrol monitoring risiko kredit.

**Source RPTs dikonsumsi**:
- **RPT-13** (`rpt-13`) — ECL Calc Run Detail (widget: top-10 instrumen by ECL weighted; calc-run status)
- **RPT-14** (`rpt-14`) — Stage Movement (widget: stage movement trend line chart)
- **RPT-15** (`rpt-15`) — SICR Trigger Log (widget: SICR triggers count + recent list)
- **RPT-26** (`rpt-26`) — Workflow Pending Approval (widget: pending klasifikasi / ECL approval)
- `GET /api/v1/jobs/{jobId}` — job status endpoint (widget: active calc-run progress)

**Widget list**:
| Widget ID | Nama | Chart | Refresh |
|---|---|---|---|
| W-RK-01 | ECL Stage Distribution | Recharts PieChart/Donut | 5-min polling |
| W-RK-02 | SICR Triggers Counter | KPI card (count per trigger type) | 5-min polling |
| W-RK-03 | Stage Movement Trend | Recharts LineChart (S1/S2/S3 count per periode) | 5-min polling |
| W-RK-04 | Top-10 Instrumen by ECL Weighted | DataTable (sorted desc by ecl_weighted) | 5-min polling |
| W-RK-05 | Calc-Run Status | JobProgressPanel (M13 pattern) jika ada active job; else "Last Run: {ts} — COMPLETED" | SSE push (active job); 5-min polling (status last run) |

### Pre-conditions
1. M14 deployed; `GET /api/v1/reports/rpt-13`, `rpt-14`, `rpt-15`, `rpt-26` return 200
2. User ter-autentikasi dengan ROLE-RISK; JWT permission `dashboard.risk.read`
3. `ecl.ecl_calc_result_line` ter-populate untuk minimal 1 calc_run
4. Minimal 1 SICR trigger event di `ecl.sicr_trigger_log`
5. `sys.job` memiliki record calc-run terkini (selesai atau running)

### Acceptance Criteria

```gherkin
Feature: Risk Dashboard — ECL stage donut, SICR, stage movement, top-10 ECL, calc-run status

  Background:
    Given ROLE-RISK USR-RISK-001 ter-autentikasi dengan permission 'dashboard.risk.read'
    And ecl.ecl_calc_result_line ter-populate: 2.600 instrumen, Stage 1: 2.400, Stage 2: 180, Stage 3: 20
    And ecl.sicr_trigger_log: 12 events bulan ini (5 rating_downgrade, 4 ig_to_nonig, 3 dpd_30)

  Scenario: M15-02-AC1 — W-RK-01 ECL Stage Distribution donut + W-RK-04 Top-10 data benar
    When USR-RISK-001 navigasi ke /dashboard/risk
    Then W-RK-01 ECL Stage Distribution Donut:
      | data source   | GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest_run_id}&sort=stage:asc&limit=200 |
      | Donut slices  | Stage 1: 92.3%, Stage 2: 6.9%, Stage 3: 0.8% (count-based)                            |
      | Center label  | "Total: 2.600 instrumen"                                                               |
      | Recharts      | PieChart dengan innerRadius=60 (donut style)                                           |
      | Tooltip       | "Stage [N]: [count] instrumen — ECL total: Rp [sum_ecl_weighted]"                      |
    And W-RK-04 Top-10 Instrumen by ECL Weighted:
      | data source   | GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest_run_id}&sort=ecl_weighted:desc&limit=10 |
      | kolom         | kode_instrumen, nama, stage, ead_idr, ecl_weighted, fl_multiplier (worst scenario)             |
      | link          | setiap baris → /reports/rpt-13?filter[instrumen_id]={id} (laporan M14 detail)                  |
    And W-RK-02 SICR Triggers Counter:
      | data source   | GET /api/v1/reports/rpt-15?filter[periode_id]={current_periode}&sort=tanggal_trigger:desc&limit=50 |
      | KPI cards     | "Rating Downgrade ≥ 2 notch: 5" — "IG → Non-IG: 4" — "DPD ≥ 30: 3"                               |

  Scenario: M15-02-AC2 — W-RK-05 Calc-Run Status: SSE live progress jika ada active job
    Given ada active ECL calc-run job JOB-ECL-2026-06 dengan status=running, progress=47
    When USR-RISK-001 navigasi ke /dashboard/risk
    Then W-RK-05 Calc-Run Status menampilkan <JobProgressPanel>:
      | SSE subscribe | GET /api/v1/jobs/JOB-ECL-2026-06/stream                                                  |
      | progress bar  | 47% — "Menghitung Stage 2 instruments (1234 dari 2600)"                                   |
      | ETA display   | "Estimasi selesai: 10:35:00 (5 menit lagi)"                                               |
      | cancel button | absent (ROLE-RISK tidak punya permission job.cancel — hanya owner atau ROLE-IT-ADMIN)     |
    When job selesai (SSE event: completed)
    Then W-RK-05 update ke: "Last Run: CR-2026-06 — COMPLETED {timestamp} — 2.600 instrumen diproses"
    And toast success: "ECL Calc Run CR-2026-06 selesai. Total ECL weighted: Rp {nilai}." + link "Lihat detail →"
    When tidak ada active job (status last run = COMPLETED atau FAILED)
    Then W-RK-05 menampilkan KPI card: "Last Run: {calc_run_id} — {status} — {completed_at}" (5-min polling via GET /api/v1/jobs/{latest_jobId})

  Scenario: M15-02-AC3 — Role gate: ROLE-AKUN tidak bisa akses /dashboard/risk
    Given ROLE-AKUN USR-AKUN-001 tanpa permission 'dashboard.risk.read'
    When USR-AKUN-001 navigasi ke /dashboard/risk
    Then Next.js middleware: HTTP 302 redirect ke /dashboard/akuntansi
    And tidak ada widget W-RK-01..W-RK-05 di DOM (absent, bukan hidden)
    And tidak ada API call ke /api/v1/reports/rpt-13, rpt-14, rpt-15 dari session USR-AKUN-001

  Scenario: M15-02-AC4 — W-RK-03 Stage Movement Trend: linechart per periode; aksesibilitas
    When USR-RISK-001 membuka /dashboard/risk
    Then W-RK-03 Stage Movement Trend:
      | data source   | GET /api/v1/reports/rpt-14?sort=tanggal_transisi:asc&limit=200&filter[periode_id]={last_6_periodes} |
      | Recharts      | LineChart, 3 series (Stage 1/2/3 count), X-axis = periode label, Y-axis = count                    |
      | legend        | "Stage 1 (Performing)", "Stage 2 (SICR)", "Stage 3 (Default)"                                      |
    And chart aria-label="Tren Perpindahan Stage ECL — 6 periode terakhir"
    And setiap data point punya aria-label="[Periode]: Stage [N] = [count] instrumen"
    And color palette: Stage 1 = hijau (#16a34a), Stage 2 = kuning (#ca8a04), Stage 3 = merah (#dc2626) — WCAG contrast ≥ 4.5:1 terhadap background putih
```

---

## Story P5-M15-03 — Akuntansi Dashboard (ROLE-AKUN / ROLE-AKUN-CTL)

**Actor**: ROLE-AKUN (primary), ROLE-AKUN-CTL (shared — dengan aksi "Approve" di jurnal pending widget)
**Trigger**: User navigasi ke `/dashboard/akuntansi` — dashboard load, 5-menit polling.
**Goal**: Tim Akuntansi mendapat visibility atas jurnal yang menunggu posting/approval, success rate GL delivery, freshness FX rate (jika JISDOR feed terlambat), timeline periode buku, dan log jurnal terkini.

**Source RPTs dikonsumsi**:
- **RPT-22** (`rpt-22`) — Jurnal Posting Log (widget: recent jurnal log)
- **RPT-22b** (`rpt-22b`) — GL Delivery Status (widget: GL delivery success rate)
- **RPT-05** (`rpt-05`) — FX Rate History (widget: FX rate freshness check)
- **RPT-26** (`rpt-26`) — Workflow Pending Approval (widget: jurnal pending posting/approval)
- **RPT-23** (`rpt-23`) — Periode Close Audit (widget: periode buku timeline)

**Widget list**:
| Widget ID | Nama | Chart/Type | Refresh |
|---|---|---|---|
| W-AK-01 | Jurnal Pending Posting | DataTable (ROLE-AKUN view) | 5-min polling |
| W-AK-02 | GL Delivery Success Rate | Recharts PieChart donut + KPI card | 5-min polling |
| W-AK-03 | FX Rate Freshness | KPI card dengan status indicator | 5-min polling |
| W-AK-04 | Periode Buku Timeline | Horizontal Recharts BarChart/Gantt-style | 5-min polling |
| W-AK-05 | Recent Jurnal Log (20 terbaru) | DataTable | 5-min polling |

### Pre-conditions
1. M14 deployed; `GET /api/v1/reports/rpt-22`, `rpt-22b`, `rpt-05`, `rpt-26`, `rpt-23` return 200
2. User ROLE-AKUN dengan permission `dashboard.akuntansi.read`; ROLE-AKUN-CTL dengan permission tambahan `jurnal.approve` (untuk action button W-AK-01)
3. `jrnl.jurnal_header` ter-populate; `jrnl.gl_delivery` ter-populate (M3)
4. `sys.fx_rate` ter-populate dengan entry terakhir (digunakan untuk freshness check)

### Acceptance Criteria

```gherkin
Feature: Akuntansi Dashboard — jurnal pending, GL delivery rate, FX freshness, periode timeline

  Background:
    Given ROLE-AKUN USR-AKUN-001 ter-autentikasi dengan permission 'dashboard.akuntansi.read'
    And jurnl.jurnal_header: 15 jurnal status=PENDING_APPROVAL untuk periode PRD-2026-06
    And rpt.mv_gl_delivery_status: 980 DELIVERED, 15 FAILED, 5 PENDING dari total 1.000 delivery

  Scenario: M15-03-AC1 — W-AK-01 Jurnal Pending + W-AK-02 GL Delivery Rate data benar
    When USR-AKUN-001 navigasi ke /dashboard/akuntansi
    Then W-AK-01 Jurnal Pending Posting:
      | data source   | GET /api/v1/reports/rpt-26?filter[entity_type]=JURNAL&filter[status]=PENDING_APPROVAL&sort=created_at:desc&limit=20 |
      | kolom         | jurnal_id, event_code, instrumen_id, nominal_idr, submitted_by, submitted_at, status                               |
      | action button | ROLE-AKUN: button "Approve" ABSENT dari DOM (permission 'jurnal.approve' tidak ada)                                 |
      |               | ROLE-AKUN-CTL: button "Approve" VISIBLE — klik redirect ke /mapping-jurnal/approve/{jurnal_id}                     |
      | badge         | "15 jurnal menunggu approval" — badge merah di widget header                                                        |
    And W-AK-02 GL Delivery Success Rate:
      | data source   | GET /api/v1/reports/rpt-22b?filter[periode_id]={current_periode}&limit=200                         |
      | Donut         | Recharts PieChart: DELIVERED 98.0%, FAILED 1.5%, PENDING 0.5%                                      |
      | KPI card      | "Success Rate: 98.0% — 980 dari 1.000 jurnal berhasil dikirim ke GL"                               |
      | alert         | jika FAILED > 5%: banner warning kuning: "Peringatan: Tingkat kegagalan GL delivery melebihi 5%"   |

  Scenario: M15-03-AC2 — W-AK-03 FX Rate Freshness: alert jika data > 1 hari kerja
    When USR-AKUN-001 membuka /dashboard/akuntansi
    And sys.fx_rate: last_entry.tanggal = {yesterday} (sebelum cutoff JISDOR 10:30 hari ini)
    Then W-AK-03 FX Rate Freshness:
      | data source   | GET /api/v1/reports/rpt-05?sort=tanggal:desc&limit=5 (5 rate terakhir)                   |
      | KPI card      | "FX Rate terakhir: {tanggal} — {kurs IDR/USD}" dengan sumber=JISDOR atau MANUAL           |
      | status        | FRESH (hijau) jika tanggal = hari ini; STALE (merah) jika tanggal < hari ini pada hari kerja |
      | alert STALE   | banner merah: "FX Rate belum diperbarui hari ini. Upload manual via Pengaturan > FX Rate." + link |
    And W-AK-04 Periode Buku Timeline:
      | data source   | GET /api/v1/reports/rpt-23?sort=tanggal_close:desc&limit=12 (12 periode terakhir)         |
      | visual        | Recharts BarChart horizontal: setiap periode = 1 bar, warna: OPEN=hijau, SOFT_CLOSED=kuning, HARD_CLOSED=abu-abu |
      | label         | setiap bar = nama periode (mis. "Jun 2026 — OPEN") + tanggal close jika sudah closed       |
      | current badge | badge "CURRENT" pada periode aktif                                                         |

  Scenario: M15-03-AC3 — Role gate: ROLE-RISK tidak bisa akses /dashboard/akuntansi; SoD button gate
    Given ROLE-RISK USR-RISK-001 tanpa permission 'dashboard.akuntansi.read'
    When USR-RISK-001 navigasi ke /dashboard/akuntansi
    Then Next.js middleware: HTTP 302 redirect ke /dashboard/risk
    And W-AK-01..W-AK-05 ABSENT dari DOM untuk USR-RISK-001
    Given ROLE-AKUN USR-AKUN-001 (bukan CTL) dengan permission 'dashboard.akuntansi.read'
    When USR-AKUN-001 membuka /dashboard/akuntansi
    Then W-AK-01 Jurnal Pending: button "Approve" ABSENT dari DOM (permission 'jurnal.approve' tidak ada di token)
    And server component check: permission 'jurnal.approve' dari JWT — tidak render button jika absent

  Scenario: M15-03-AC4 — W-AK-05 Recent Jurnal Log; aksesibilitas; empty state
    When USR-AKUN-001 membuka /dashboard/akuntansi
    Then W-AK-05 Recent Jurnal Log:
      | data source   | GET /api/v1/reports/rpt-22?sort=posted_at:desc&limit=20                                   |
      | kolom         | jurnal_id, event_code, instrumen_id (link), nominal_idr, posted_at, status_posting        |
      | link instrumen| klik instrumen_id → /master/instrumen/{instrumen_id}                                      |
    When tidak ada jurnal dalam periode (empty state)
    Then W-AK-05 tampilkan: ilustrasi + "Tidak ada jurnal yang tersedia untuk periode ini." + link "Lihat semua jurnal →" ke /reports/rpt-22
    And setiap widget punya aria-label="[Nama Widget] — BLIPS Akuntansi Dashboard"
    And DataTable W-AK-01 kolom "Nominal IDR" punya text-align: right; aria-label per cell: "Nominal: Rp [nilai]"
```

---

## Story P5-M15-04 — CFO+Direksi Dashboard (ROLE-CFO / ROLE-CEO / ROLE-KOMITE / ROLE-ALCO)

**Actor**: ROLE-CFO (primary), ROLE-CEO, ROLE-KOMITE, ROLE-ALCO (shared — semua ke `/dashboard/cfo`)
**Trigger**: User navigasi ke `/dashboard/cfo` — setelah JWT MFA check; dashboard load, 5-menit polling.
**Goal**: CFO dan Direksi mendapat executive view atas total portfolio NAV, ECL coverage ratio, scenario sensitivity summary (uses RPT-27), stage 3 ratio, P&L impact MTD/YTD, dan status hard-close periode buku aktif.

**Source RPTs dikonsumsi**:
- **RPT-01** (`rpt-01`) — Daftar Instrumen (widget: Total NAV portfolio)
- **RPT-13** (`rpt-13`) — ECL Calc Run Detail (widget: ECL coverage ratio, stage 3 ratio)
- **RPT-18** (`rpt-18`) — ECL Roll-Forward (widget: P&L ECL impact MTD/YTD)
- **RPT-23** (`rpt-23`) — Periode Close Audit (widget: hard-close status periode aktif)
- **RPT-27** (`rpt-27`) — ECL Sensitivity Analysis (widget: scenario sensitivity summary)

**Widget list**:
| Widget ID | Nama | Chart/Type | Refresh |
|---|---|---|---|
| W-CF-01 | Total Portfolio NAV | KPI card (sum EAD IDR) | 5-min polling |
| W-CF-02 | ECL Coverage Ratio | KPI card + Recharts BarChart (ECL/EAD per stage) | 5-min polling |
| W-CF-03 | Scenario Sensitivity Summary | Recharts BarChart (3-bar: Good/Normal/Bad ECL) | 5-min polling |
| W-CF-04 | Stage 3 Ratio | KPI card (Stage 3 EAD / Total EAD) + trend | 5-min polling |
| W-CF-05 | P&L ECL Impact MTD/YTD | Recharts AreaChart (kumulatif ECL movement) | 5-min polling |
| W-CF-06 | Hard-Close Status Periode | Status card + timeline | 5-min polling |

### Pre-conditions
1. M14 deployed; `GET /api/v1/reports/rpt-01`, `rpt-13`, `rpt-18`, `rpt-23`, `rpt-27` return 200
2. User ter-autentikasi dengan ROLE-CFO/CEO/KOMITE/ALCO; `mfa_verified=true` di JWT (DEC-026)
3. Minimal 1 completed ECL calc-run di `ecl.ecl_calc_result_line`
4. Server component check: jika `mfa_verified=false` → redirect ke `/auth/mfa` sebelum render dashboard

### Acceptance Criteria

```gherkin
Feature: CFO+Direksi Dashboard — NAV, ECL ratio, sensitivity, stage 3, P&L, hard-close status

  Background:
    Given ROLE-CFO USR-CFO-001 ter-autentikasi dengan permission 'dashboard.cfo.read' dan mfa_verified=true
    And ecl.ecl_calc_result_line: total EAD IDR = 500 miliar; ECL weighted total = 12,5 miliar; Stage 3 EAD = 7,5 miliar

  Scenario: M15-04-AC1 — W-CF-01 Total NAV + W-CF-02 ECL Coverage Ratio + W-CF-04 Stage 3 Ratio data benar
    When USR-CFO-001 navigasi ke /dashboard/cfo
    Then W-CF-01 Total Portfolio NAV:
      | data source   | GET /api/v1/reports/rpt-01?filter[status]=AKTIF&limit=200 — aggregate client-side sum(ead_idr) |
      | KPI card      | "Total NAV Portfolio: Rp 500.000.000.000 (Rp 500 M)"                                          |
      | sub-label     | "Berdasarkan {count} instrumen aktif — per {last_calc_run_date}"                               |
    And W-CF-02 ECL Coverage Ratio:
      | data source   | GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest}&limit=200                              |
      | KPI card      | "ECL Coverage Ratio: 2.50% (ECL Rp 12,5 M / EAD Rp 500 M)"                                   |
      | BarChart      | Recharts BarChart 3 bars (Stage 1/2/3): X=stage, Y=ECL/EAD ratio per stage                    |
      | tooltip       | "Stage [N]: ECL Rp [x] / EAD Rp [y] = [z]%"                                                  |
    And W-CF-04 Stage 3 Ratio:
      | KPI card      | "Stage 3 Ratio: 1.50% (EAD Rp 7,5 M / Total EAD Rp 500 M)"                                   |
      | status        | hijau jika < 2%; kuning jika 2-5%; merah jika > 5% (threshold hardcoded di widget)             |
      | trend arrow   | bandingkan dengan periode sebelumnya: ↑ naik (merah) / ↓ turun (hijau) / → stabil             |

  Scenario: M15-04-AC2 — W-CF-03 Scenario Sensitivity (RPT-27); W-CF-06 Hard-Close Status
    When USR-CFO-001 membuka /dashboard/cfo
    Then W-CF-03 Scenario Sensitivity Summary:
      | data source   | GET /api/v1/reports/rpt-27?filter[calc_run_id]={latest}&w_good=0.25&w_normal=0.50&w_bad=0.25 (bobot default DEC-010) |
      | BarChart      | Recharts BarChart 3 bars: "Optimis (Good)", "Base (Normal)", "Pesimis (Bad)"                                         |
      | Y-axis        | total ECL per scenario (sum ecl_fl_good / ecl_fl_normal / ecl_fl_bad dari rpt-27)                                   |
      | label         | nilai Rp di atas bar; delta vs weighted di tooltip: "+Rp X M vs base scenario"                                       |
      | bobot display | sub-label: "Bobot skenario aktif: Good 25% / Normal 50% / Bad 25% (ALCO-approved)"                                  |
    And W-CF-06 Hard-Close Status Periode:
      | data source   | GET /api/v1/reports/rpt-23?sort=tanggal_close:desc&limit=1 (periode terkini)                                         |
      | status card   | "Periode PRD-2026-06 — STATUS: OPEN (belum hard-close)" dengan tombol "Lihat Detail" → /reports/rpt-23               |
      |               | jika HARD_CLOSED: "PRD-2026-06 — HARD CLOSED — {tanggal} oleh {actor_user}" dengan badge hijau                       |
      | aksi          | ROLE-CFO: link "Proses Hard-Close →" → /periode-buku/{id}/hardclose (di luar scope M15)                             |

  Scenario: M15-04-AC3 — MFA gate: ROLE-CFO tanpa mfa_verified tidak bisa akses dashboard
    Given ROLE-CFO USR-CFO-002 ter-autentikasi tapi mfa_verified=false
    When USR-CFO-002 navigasi ke /dashboard/cfo
    Then Next.js server component: check JWT claim mfa_verified=false
    And HTTP 302 redirect ke /auth/mfa?returnUrl=/dashboard/cfo
    And tidak ada widget W-CF-01..W-CF-06 yang di-render (halaman tidak pernah mencapai widget render)
    And tidak ada API call ke report endpoints dari session USR-CFO-002
    Given ROLE-AKUN USR-AKUN-001 (bukan CFO/CEO/KOMITE/ALCO) tanpa permission 'dashboard.cfo.read'
    When USR-AKUN-001 navigasi ke /dashboard/cfo
    Then Next.js middleware: HTTP 302 redirect ke /dashboard/akuntansi (role default)
    And semua widget W-CF-01..W-CF-06 ABSENT dari DOM

  Scenario: M15-04-AC4 — W-CF-05 P&L ECL Impact MTD/YTD; aksesibilitas CFO level
    When USR-CFO-001 membuka /dashboard/cfo
    Then W-CF-05 P&L ECL Impact MTD/YTD:
      | data source   | GET /api/v1/reports/rpt-18?filter[periode_id]={current_periode} (ECL Roll-Forward) |
      | AreaChart     | Recharts AreaChart: 2 series — MTD cumulative ECL movement, YTD cumulative           |
      | X-axis        | tanggal (daily bucket untuk MTD; monthly bucket untuk YTD)                           |
      | Y-axis        | nominal IDR ECL movement (+ = penambahan impairment loss, - = reversal)              |
      | label         | "MTD: +Rp [x] M impairment" dan "YTD: +Rp [y] M impairment"                         |
    And seluruh dashboard /dashboard/cfo: keyboard accessible; aria-label per widget dalam Bahasa Indonesia
    And KPI cards: role="status" aria-live="polite" untuk update otomatis tiap 5 menit
    And tombol "Refresh" dashboard: aria-label="Perbarui semua data dashboard CFO"
    And semua angka Rupiah: aria-label menyebut nilai penuh (mis. "Lima ratus miliar rupiah") tidak hanya "Rp 500 M"
```

---

## Story P5-M15-05 — Auditor Dashboard + Job History `/jobs` (ROLE-AUDIT + Semua Role)

**Actor**: ROLE-AUDIT (Auditor Dashboard exclusive); semua role (Job History `/jobs` page)
**Trigger**: ROLE-AUDIT navigasi ke `/dashboard/audit`; semua user navigasi ke `/jobs`.
**Goal**: Auditor mendapat visibility atas volume audit log, status hash-chain verification, dan SoD violation alerts. Semua user mendapat halaman `/jobs` sebagai DataTable job history dengan filter/sort/export, cancel active job (owner atau IT-Admin), dan download result.

**Source RPTs / endpoints dikonsumsi** (Auditor Dashboard):
- **RPT-25** (`rpt-25`) — Audit Log Browser (widget: audit log volume + recent events)
- **RPT-26** (`rpt-26`) — Workflow Pending Approval (widget: SoD violation alerts — filter SOD_VIOLATION error di audit)

**Endpoints dikonsumsi** (`/jobs` page):
- `GET /api/v1/jobs` — job list (query params: `status`, `type`, `cursor`, `limit`, `sort`, `q`) — **catatan untuk system-analyst**: endpoint ini dibutuhkan sebagai endpoint baru thin (list all jobs) yang belum ada di M13/M14; M13 hanya punya `GET /api/v1/jobs/{jobId}` (single job) dan per-job SSE. Rekomendasikan: tambah `GET /api/v1/jobs` ke OpenAPI di system-analyst handoff.
- `POST /api/v1/jobs/{jobId}/cancel` — cancel running job (existing, M13 pattern)
- MinIO signed URL — download result file (existing, M13 pattern)

**Widget list (Auditor Dashboard)**:
| Widget ID | Nama | Chart/Type | Refresh |
|---|---|---|---|
| W-AU-01 | Audit Log Volume (30 hari terakhir) | Recharts BarChart (event count per day) | 5-min polling |
| W-AU-02 | Hash-Chain Verification Status | KPI card (VERIFIED / MISMATCH count) | 5-min polling |
| W-AU-03 | SoD Violation Alerts | DataTable (filtered audit log entries action=SOD_VIOLATION) | 5-min polling |
| W-AU-04 | Top Action Types | Recharts BarChart horizontal (top 10 action per count) | 5-min polling |

### Pre-conditions
1. M14 deployed; `GET /api/v1/reports/rpt-25`, `rpt-26` return 200 untuk ROLE-AUDIT
2. `aud.audit_log` ter-populate dengan > 0 entries
3. Job hash-chain verify job berjalan harian (Asynq cron — M13/M14 pattern); hasil disimpan di `sys.job` type=HASH_CHAIN_VERIFY
4. `sys.job` ter-populate dengan history job dari semua tipe
5. Permission `dashboard.audit.read` granted ke ROLE-AUDIT; permission `jobs.read` granted ke semua authenticated role (filtered by owner); `jobs.read_all` hanya ROLE-IT-ADMIN

### Acceptance Criteria

```gherkin
Feature: Auditor Dashboard + Job History /jobs — audit volume, hash-chain, SoD alerts, job DataTable

  Background:
    Given ROLE-AUDIT USR-AUDIT-001 ter-autentikasi dengan permission 'dashboard.audit.read', 'report.*.read'
    And aud.audit_log: 85.000 entries total; 3 entries action='SOD_VIOLATION' bulan ini
    And sys.job: 240 job records; 2 job status=running, 235 COMPLETED, 3 FAILED

  Scenario: M15-05-AC1 — Auditor Dashboard: W-AU-01 volume + W-AU-02 hash-chain + W-AU-03 SoD alerts
    When USR-AUDIT-001 navigasi ke /dashboard/audit
    Then W-AU-01 Audit Log Volume:
      | data source   | GET /api/v1/reports/rpt-25?filter[event_time]=between:{today-30d},{today}&sort=event_time:asc&limit=200 |
      | BarChart      | Recharts BarChart: X=tanggal (30 days), Y=event count per hari                                          |
      | tooltip       | "Tanggal {date}: {count} events"                                                                        |
      | KPI total     | "Total 30 hari: 85.000 events" di atas chart                                                            |
    And W-AU-02 Hash-Chain Verification Status:
      | data source   | GET /api/v1/jobs?type=HASH_CHAIN_VERIFY&sort=created_at:desc&limit=1 (hasil verify job terakhir)        |
      | KPI card      | jika result.status=VERIFIED: "Hash-chain VERIFIED — last run: {timestamp}" badge hijau                  |
      |               | jika result.status=MISMATCH: "PERINGATAN: Hash-chain MISMATCH terdeteksi!" badge merah + detail         |
      | action        | link "Lihat detail →" → /jobs/{jobId} (detail job hash-chain)                                           |
    And W-AU-03 SoD Violation Alerts:
      | data source   | GET /api/v1/reports/rpt-25?filter[action]=SOD_VIOLATION&sort=event_time:desc&limit=20                   |
      | DataTable     | kolom: event_time, actor_user_id, entity_type, entity_id, detail (from after_jsonb)                     |
      | badge         | "3 pelanggaran SoD bulan ini" badge merah di widget header                                              |
      | empty state   | "Tidak ada pelanggaran SoD yang terdeteksi dalam 30 hari terakhir." badge hijau                         |

  Scenario: M15-05-AC2 — /jobs page: DataTable semua job milik user; ROLE-IT-ADMIN lihat semua
    Given USR-MAKER-001 ROLE-MAKER-TR ter-autentikasi (permission 'jobs.read')
    When USR-MAKER-001 navigasi ke /jobs
    Then DataTable `/jobs` menampilkan UX §1 (sort + paging + filter + export):
      | data source   | GET /api/v1/jobs?cursor=...&limit=50&sort=created_at:desc (filter owner = USR-MAKER-001 di backend) |
      | kolom         | Job ID, Type (label Indonesia), Status, Progress, Started, Completed/ETA, Duration, Actions          |
      | filter        | status (dropdown: queued/running/completed/failed/cancelled), type (dropdown per job type), date range |
      | sort          | semua kolom sortable; default: created_at DESC                                                        |
      | pagination    | cursor-based; "Page X of ~Y"; Prev/Next; limit selector 25/50/100                                    |
      | export        | CSV + XLSX — hanya job milik user tersebut; audit EXPORT.GENERATED                                   |
    When USR-IT-ADMIN ROLE-IT-ADMIN dengan permission 'jobs.read_all' navigasi ke /jobs
    Then DataTable menampilkan SEMUA job (semua owner) + kolom tambahan "Created By"
    And filter tambahan: "Filter by user" (text search username)

  Scenario: M15-05-AC3 — /jobs: cancel running job; download result; lihat detail
    Given USR-MAKER-001 memiliki job JOB-EXPORT-001 status=running (owned job)
    When USR-MAKER-001 klik "Batalkan" pada row JOB-EXPORT-001
    Then muncul konfirmasi dialog: "Batalkan job JOB-EXPORT-001 (Export MTM Daily)? Proses yang sudah selesai tidak bisa dikembalikan."
    When USR-MAKER-001 konfirmasi
    Then POST /api/v1/jobs/JOB-EXPORT-001/cancel → HTTP 200 { status: "cancelled" }
    And toast success: "Job JOB-EXPORT-001 berhasil dibatalkan."
    And row JOB-EXPORT-001 status update ke CANCELLED (polling 2 detik atau SSE event)
    Given USR-MAKER-001 memiliki job JOB-EXPORT-002 status=COMPLETED dengan result_url tersedia
    When USR-MAKER-001 klik "Unduh" pada row JOB-EXPORT-002
    Then MinIO signed download URL di-fetch; browser trigger download file (Content-Disposition: attachment)
    And toast success: "Download dimulai — file {filename} sedang diunduh."
    Given USR-RISK-001 ROLE-RISK mencoba klik "Batalkan" pada job JOB-ECL-RUN-001 milik USR-MAKER-001
    Then button "Batalkan" ABSENT dari DOM (hanya owner + ROLE-IT-ADMIN yang boleh cancel — permission check server-side)
    And jika USR-RISK-001 langsung POST /api/v1/jobs/JOB-ECL-RUN-001/cancel → HTTP 403 JOB_NOT_OWNED_BY_USER

  Scenario: M15-05-AC4 — Auditor Dashboard role gate; /jobs aksesibilitas
    Given ROLE-AKUN USR-AKUN-001 tanpa permission 'dashboard.audit.read'
    When USR-AKUN-001 navigasi ke /dashboard/audit
    Then Next.js middleware: HTTP 302 redirect ke /dashboard/akuntansi
    And W-AU-01..W-AU-04 ABSENT dari DOM untuk USR-AKUN-001
    When USR-AUDIT-001 navigasi ke /jobs
    Then DataTable /jobs: aria-label="Riwayat Job BLIPS"
    And setiap row: aria-label="Job {id} — {type} — {status} — dibuat {created_at}"
    And tombol "Batalkan": aria-label="Batalkan job {id}"
    And tombol "Unduh": aria-label="Unduh hasil job {id}"
    And filter dropdown "Status": aria-label="Filter status job"; filter "Tipe": aria-label="Filter tipe job"
    And keyboard navigation: Tab cycle melalui header filter, rows, action buttons, paging controls
```

---

## Ringkasan P5-M15 Story Set

| Story | Judul | Actor Utama | RPTs dikonsumsi | AC Count | Gate |
|---|---|---|---|---|---|
| P5-M15-01 | Treasury Dashboard | ROLE-MAKER-TR, ROLE-APPR-TR | RPT-01, RPT-06, RPT-10, RPT-26 | 4 | security BLOCKING (route guard) |
| P5-M15-02 | Risk Dashboard | ROLE-RISK | RPT-13, RPT-14, RPT-15, RPT-26 + job SSE | 4 | security BLOCKING |
| P5-M15-03 | Akuntansi Dashboard | ROLE-AKUN, ROLE-AKUN-CTL | RPT-05, RPT-22, RPT-22b, RPT-23, RPT-26 | 4 | security BLOCKING |
| P5-M15-04 | CFO+Direksi Dashboard | ROLE-CFO, ROLE-CEO, ROLE-KOMITE, ROLE-ALCO | RPT-01, RPT-13, RPT-18, RPT-23, RPT-27 | 4 | security BLOCKING + MFA gate |
| P5-M15-05 | Auditor Dashboard + `/jobs` | ROLE-AUDIT + semua role | RPT-25, RPT-26 + `GET /api/v1/jobs` (list) | 4 | security BLOCKING |
| **Total** | | | | **20** | |

---

## Error Codes Baru yang Diusulkan (untuk system-analyst → `api/openapi/_common.yaml`)

| Code | HTTP | Trigger |
|---|---|---|
| `DASHBOARD_WIDGET_TIMEOUT` | 422 | Widget fetch ke report endpoint melebihi 10 detik client-side; backend statement_timeout 30s sudah cover server-side, ini untuk gateway timeout dari frontend perspective |
| `DASHBOARD_PERMISSION_DENIED` | 403 | User mengakses `/dashboard/{role}` tanpa permission `dashboard.{role}.read`; distinct dari `FORBIDDEN` umum untuk dashboard-specific logging |
| `JOB_NOT_OWNED_BY_USER` | 403 | `POST /api/v1/jobs/{jobId}/cancel` oleh user yang bukan owner dan bukan ROLE-IT-ADMIN |
| `JOB_ALREADY_TERMINAL` | 409 | `POST /api/v1/jobs/{jobId}/cancel` pada job yang sudah status=COMPLETED/FAILED/CANCELLED |
| `JOB_NOT_VISIBLE_TO_USER` | 404 | `GET /api/v1/jobs/{jobId}` atau list; job ada tapi milik user lain dan requestor bukan ROLE-IT-ADMIN |
| `SSE_STREAM_UNAVAILABLE` | 503 | SSE endpoint tidak bisa subscribe (Redis down, circuit breaker open); frontend fallback ke 2-detik polling |

Catatan: `FORBIDDEN` (403) untuk MFA step-up missing di `/dashboard/cfo` tetap reuse dari common — redirect ke `/auth/mfa` (bukan error JSON).

---

## Audit Events — Dashboard (Minimal; read-only dashboard)

Dashboard adalah read-only — audit per-view tidak wajib per DEC-018 (menghindari audit log bloat). Event yang WAJIB di-audit:

| Event | Trigger | In-transaction |
|---|---|---|
| `JOB.CANCEL_REQUESTED` | `POST /api/v1/jobs/{jobId}/cancel` | Ya |
| `JOB.RESULT_DOWNLOADED` | Download signed URL di klik (MinIO callback atau gateway log) | Ya (jika via gateway) |
| `EXPORT.GENERATED` | Export DataTable `/jobs` (CSV/XLSX) | Ya |

Dashboard view (`GET /dashboard/*`) tidak di-audit per-view. Exception: `/dashboard/cfo` + `/dashboard/audit` — server-side log ke Loki (bukan `aud.audit_log`) untuk monitoring tanpa bloat.

---

## Backend Endpoint Baru yang Diperlukan (catatan untuk system-analyst)

M15 adalah FE-only kecuali **satu endpoint list** yang belum ada:

| Endpoint | Status | Keterangan |
|---|---|---|
| `GET /api/v1/jobs` | **BARU — perlu system-analyst** | List jobs dengan filter owner/status/type; cursor-based; ROLE-IT-ADMIN lihat semua, role lain filter by `created_by = current_user`. M13 hanya punya `GET /api/v1/jobs/{jobId}` (single). |
| `GET /api/v1/jobs/{jobId}` | Existing (M13) | Reuse |
| `GET /api/v1/jobs/{jobId}/stream` | Existing (M13) | Reuse untuk W-RK-05 Calc-Run Status SSE |
| `POST /api/v1/jobs/{jobId}/cancel` | Existing (M13) | Reuse |
| `GET /api/v1/reports/{slug}` | Existing (M14) | Semua widget consume ini |
| `GET /api/v1/dashboards/events` (SSE aggregator) | **Tidak direkomendasikan** | Reuse per-job SSE M13 di frontend; tidak perlu agregator backend baru |

---

## Refresh Cadence Summary

| Widget | Refresh Mode | Interval | Notes |
|---|---|---|---|
| Semua widget default | 5-min polling | 300.000 ms | `useDashboardPolling.ts` hook; pause saat tab hidden |
| W-RK-05 Calc-Run Status (active job) | SSE push | real-time | `useJobSSE.ts` subscribe ke `/api/v1/jobs/{jobId}/stream`; fallback polling 2s jika SSE error |
| Manual refresh button | On-demand | — | Trigger re-fetch semua widget seketika |
| `/jobs` DataTable | 5-min polling + manual | 300.000 ms | Refresh badge di DataTable header; user bisa klik "Refresh" |

---

## Out of Scope (Eksplisit M15)

1. Tidak ada mutating endpoint baru di backend (kecuali `GET /api/v1/jobs` yang missing dari M13).
2. Tidak ada migration database baru — M15 adalah FE-only modul.
3. Dashboard customization (widget reordering, hide/show widget, persistent user preference) → Phase 6.
4. Widget drag-and-drop → Phase 6.
5. Dashboard untuk ROLE-IT-ADMIN → gunakan `/jobs` page + Grafana operational dashboard (di-scope devops).
6. Dashboard print / PDF export → gunakan laporan M14 langsung.
7. Mobile-responsive layout < 1280px → best effort; target primary desktop (≥ 1280px).
8. Email/push notification dari dashboard event → M13 SSE job notification sudah cover; tidak tambah channel baru.
9. Real-time websocket dua arah → SSE read-only sudah cukup untuk dashboard; WS → Phase 6 jika dibutuhkan.
10. Widget untuk ROLE-ALCO ECL parameter approve — action ini di APP-C / APP-D modul; dashboard hanya read.

---

## Handoff Berikutnya

- `system-analyst` → OpenAPI: **satu endpoint baru** `GET /api/v1/jobs` (list, cursor, filter owner/status/type); 6 error codes baru (tabel di atas); permission `dashboard.{role}.read` per role (5 permissions + `jobs.read` + `jobs.read_all`); `dashboard.treasury.read` granted ke ROLE-MAKER-TR + ROLE-APPR-TR; update `web/middleware.ts` routing spec
- `uiux-designer` → wireframe 5 dashboard + `/jobs` page; widget layout per role; color palette untuk stage (hijau/kuning/merah); WCAG AA contrast checklist
- `frontend-engineer-nextjs` → implementasi setelah `system-analyst` + `uiux-designer` selesai; komponen: `useDashboardPolling.ts`, `useJobSSE.ts`, 5 page.tsx, `components/blips/dashboard/` widgets; update `web/middleware.ts`
- `security-engineer` → **BLOCKING**: role-gated routing absent-from-DOM; JWT `mfa_verified` check server component `/dashboard/cfo`; SSE auth (JWT passed di Authorization header atau `?token=` query — pilih satu, document di OpenAPI); `JOB_NOT_OWNED_BY_USER` permission enforcement server-side; `/dashboard/audit` data tidak bocor ke non-AUDIT role
- `devops-engineer` → pastikan Redis tersedia untuk SSE broker; SSE keepalive config di Traefik (timeout ≥ 5 menit); MinIO signed URL TTL 24 jam sesuai M13; Loki log shipping untuk `/dashboard/cfo` + `/dashboard/audit` view events

_Story set P5-M15 siap dihandoff ke `system-analyst` (1 endpoint baru + permission spec) dan `uiux-designer` (wireframe) secara paralel. `security-engineer` BLOCKING sebelum frontend deploy. Tidak ada `ifrs9-compliance-reviewer` gate (M15 read-only, tidak compute ECL/EIR/SPPI/BM)._
