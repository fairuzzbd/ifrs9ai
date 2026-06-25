# P5-M15 — APP-E Dashboards per Role: UI/UX Design Specification

**Story Set**: P5-M15
**Modul**: APP-E — Reporting & Dashboard
**Desainer**: uiux-designer
**Tanggal**: 2026-06-25
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M15-dashboards.md`
**Linked API**: `api/openapi/app-e-dashboards.yaml` (to be created by system-analyst)
**Decisions applied**:
- DEC-002 (LOCKED) — Next.js 14+ App Router, TypeScript strict, shadcn/ui, Recharts
- DEC-007 (LOCKED) — Asynq job queue; SSE job events reuse dari M13
- DEC-017 (LOCKED) — SoD; absent-from-DOM for permission-gated widgets
- DEC-018 (LOCKED) — audit trail; dashboard read-only tidak wajib audit per-view
- DEC-020 (LOCKED) — REST /api/v1/; widget consume M14 report endpoints
- DEC-022 (LOCKED) — Cursor-only pagination untuk /jobs DataTable
- DEC-025 (LOCKED) — JWT 15 min; role-gated routing server-side middleware
- DEC-026 (LOCKED) — MFA mandatory: CFO, CEO, KOMITE, ALCO

**Dependensi**:
- P5-M13 (WAJIB) — `sys.job` table + SSE stream + `JobProgressPanel` component
- P5-M14 (WAJIB) — 25 report endpoints `GET /api/v1/reports/{slug}`
- P5-M12 — `jrnl.*` tables (Akuntansi dashboard)
- P5-M4 — `mst.periode_buku` (Periode Buku Timeline widget)
- P5-M3 — `jrnl.gl_delivery` (GL Delivery Status widget)

**Gate**: `security-engineer` BLOCKING — role-gated routing absent-from-DOM; JWT `mfa_verified` check server component `/dashboard/cfo`; SSE auth. Tidak ada `ifrs9-compliance-reviewer` gate (M15 read-only).

---

## 1. Persona Table

| Role | Dashboard URL | Dashboard Permission | MFA Required |
|---|---|---|---|
| ROLE-MAKER-TR | `/dashboard/treasury` | `dashboard.treasury.read` | Tidak |
| ROLE-APPR-TR | `/dashboard/treasury` | `dashboard.treasury.read` | Tidak (Treasury Manager: ya) |
| ROLE-RISK | `/dashboard/risk` | `dashboard.risk.read` | Tidak |
| ROLE-AKUN | `/dashboard/akuntansi` | `dashboard.akuntansi.read` | Tidak |
| ROLE-AKUN-CTL | `/dashboard/akuntansi` | `dashboard.akuntansi.read` | WAJIB |
| ROLE-CFO | `/dashboard/cfo` | `dashboard.cfo.read` | WAJIB + `mfa_verified=true` check server component |
| ROLE-CEO | `/dashboard/cfo` | `dashboard.cfo.read` | WAJIB |
| ROLE-KOMITE | `/dashboard/cfo` | `dashboard.cfo.read` | WAJIB |
| ROLE-ALCO | `/dashboard/cfo` | `dashboard.cfo.read` | WAJIB |
| ROLE-AUDIT | `/dashboard/audit` | `dashboard.audit.read` | Tidak |
| Semua role | `/jobs` | `jobs.read` (milik sendiri) | Tidak |
| ROLE-IT-ADMIN | `/jobs` | `jobs.read_all` (semua) | WAJIB |

**Role default redirect mapping** (diperlukan di `web/middleware.ts`):

| Role | Default dashboard |
|---|---|
| ROLE-MAKER-TR, ROLE-APPR-TR | `/dashboard/treasury` |
| ROLE-RISK | `/dashboard/risk` |
| ROLE-AKUN, ROLE-AKUN-CTL | `/dashboard/akuntansi` |
| ROLE-CFO, ROLE-CEO, ROLE-KOMITE, ROLE-ALCO | `/dashboard/cfo` |
| ROLE-AUDIT | `/dashboard/audit` |
| ROLE-IT-ADMIN | `/jobs` |
| (fallback) | `/dashboard/treasury` |

---

## 2. Screen Inventory

### 2.1 Sitemap P5-M15

```
Dashboard (side nav group)
├── /dashboard                          — Role router: redirect ke dashboard spesifik
├── /dashboard/treasury                 — Treasury Dashboard (ROLE-MAKER-TR, ROLE-APPR-TR)
├── /dashboard/risk                     — Risk Dashboard (ROLE-RISK)
├── /dashboard/akuntansi                — Akuntansi Dashboard (ROLE-AKUN, ROLE-AKUN-CTL)
├── /dashboard/cfo                      — CFO+Direksi Dashboard (ROLE-CFO, ROLE-CEO, ROLE-KOMITE, ROLE-ALCO)
└── /dashboard/audit                    — Auditor Dashboard (ROLE-AUDIT)

Jobs (shared)
└── /jobs                               — Job History DataTable (semua authenticated role)
    └── (drawer) /jobs/[jobId]          — Job detail drawer (JobProgressPanel reuse + raw payload)
```

### 2.2 AC Trace — Screen ke Story

| Screen | Route | Persona | Story / AC |
|---|---|---|---|
| Dashboard Router | `/dashboard` | Semua | M15-01-AC2, M15-02-AC3, M15-03-AC3, M15-04-AC3, M15-05-AC4 |
| Treasury Dashboard | `/dashboard/treasury` | MAKER-TR, APPR-TR | M15-01-AC1..4 |
| Risk Dashboard | `/dashboard/risk` | RISK | M15-02-AC1..4 |
| Akuntansi Dashboard | `/dashboard/akuntansi` | AKUN, AKUN-CTL | M15-03-AC1..4 |
| CFO Dashboard | `/dashboard/cfo` | CFO, CEO, KOMITE, ALCO | M15-04-AC1..4 |
| Auditor Dashboard | `/dashboard/audit` | AUDIT | M15-05-AC1, AC4 |
| Job History | `/jobs` | Semua | M15-05-AC2..4 |

---

## 3. Role-Gating Spec

### 3.1 Server Component Permission Check Pattern

Setiap dashboard route adalah Next.js Server Component. Permission check dilakukan **sebelum** rendering widget apa pun. Widget tidak di-render, tidak di-hide — **absent from DOM**.

```
/dashboard/treasury → check JWT.permissions.includes('dashboard.treasury.read')
  → false: redirect('/dashboard') → middleware → role-default redirect
  → true: render page

/dashboard/cfo → check JWT.permissions.includes('dashboard.cfo.read')
  → false: redirect('/dashboard')
  → true: check JWT.mfa_verified === true
    → false: redirect('/auth/mfa?returnUrl=/dashboard/cfo')
    → true: render page
```

**Middleware routing logic** (`web/middleware.ts`):

```
GET /dashboard
  → read JWT from cookie/header
  → look up roleDefaultDashboard(roles[0])
  → 302 redirect ke dashboard default

GET /dashboard/:role
  → check requiredPermission(role) in JWT.permissions
  → if absent: DASHBOARD_PERMISSION_DENIED (403) → redirect /dashboard
  → if /dashboard/cfo && !mfa_verified: redirect /auth/mfa?returnUrl=...
  → proceed
```

### 3.2 Permission Strings (baru, perlu ditambah ke JWT seeding)

Permission strings berikut harus di-grant ke role yang sesuai via Keycloak role seeding. Ini **bukan** permission yang sudah ada di M13/M14.

| Permission | Granted to |
|---|---|
| `dashboard.treasury.read` | ROLE-MAKER-TR, ROLE-APPR-TR |
| `dashboard.risk.read` | ROLE-RISK |
| `dashboard.akuntansi.read` | ROLE-AKUN, ROLE-AKUN-CTL |
| `dashboard.cfo.read` | ROLE-CFO, ROLE-CEO, ROLE-KOMITE, ROLE-ALCO |
| `dashboard.audit.read` | ROLE-AUDIT |
| `jobs.read` | Semua authenticated role (filter by owner server-side) |
| `jobs.read_all` | ROLE-IT-ADMIN only |

**Security-engineer catatan**: permission strings ini harus ditambahkan ke Keycloak role-permission seeding dan diverifikasi masuk ke JWT `permissions[]` claim sebelum route protection bisa berfungsi. Gate BLOCKING sebelum deploy.

---

## 4. Widget Catalog

Widget didesain sekali di sini dan direferensikan oleh beberapa dashboard. Semua widget berada di `web/components/blips/dashboard/`.

### 4.1 `KPICard`

Metrik tunggal dengan delta dan sub-label.

```
Props:
  title: string                   — label metric (Bahasa Indonesia)
  value: string                   — formatted string (e.g. "Rp 500 M" atau "2.50%")
  valueAriaLabel: string          — nilai penuh untuk screen reader (e.g. "Lima ratus miliar rupiah")
  delta?: {
    value: string                 — e.g. "+0.05%"
    direction: 'up' | 'down' | 'neutral'
    label: string                 — e.g. "vs periode sebelumnya"
  }
  subLabel?: string               — teks kecil di bawah value (e.g. "Berdasarkan 2.600 instrumen aktif")
  status?: 'default' | 'success' | 'warning' | 'danger'
  icon?: LucideIcon
  loading?: boolean               — show skeleton
  error?: string                  — show error inline
  ariaLive?: 'polite' | 'off'    — default 'polite' untuk auto-refresh
```

Visual saat loading: skeleton kotak 80px tinggi. Error: border merah tipis + pesan kecil.

### 4.2 `StageDistributionDonut`

Recharts PieChart donut untuk distribusi Stage ECL.

```
Props:
  data: { stage: 1|2|3; count: number; eclTotal: number }[]
  totalCount: number
  loading?: boolean
  error?: string
  ariaLabel?: string    — default "Distribusi Stage ECL"
```

Visual:
- `innerRadius=60`, `outerRadius=100`
- Warna: Stage 1 = `hsl(var(--chart-1))` (hijau), Stage 2 = `hsl(var(--chart-2))` (amber), Stage 3 = `hsl(var(--chart-3))` (merah)
- Center label: "Total: {totalCount}" — rendered via `<text>` SVG di tengah
- Tooltip: "Stage {N}: {count} instrumen — ECL total: Rp {eclTotal}"
- Legend bawah: teks "Stage 1 (Performing)" / "Stage 2 (SICR)" / "Stage 3 (Default)"
- WCAG: warna bukan satu-satunya signal — label teks eksplisit + pattern fill opsional
- Screen reader: `<title>` SVG + visually-hidden summary table (lihat §10)

### 4.3 `StageMovementBar`

Recharts BarChart perpindahan stage per periode.

```
Props:
  data: { periode: string; s1Count: number; s2Count: number; s3Count: number }[]
  loading?: boolean
  error?: string
```

Visual:
- Grouped BarChart, 3 bar per periode
- X-axis: label periode (e.g. "Jan 2026")
- Y-axis: jumlah instrumen
- Warna sama dengan StageDistributionDonut
- Legend: "Stage 1", "Stage 2", "Stage 3"

### 4.4 `MaturityBucketBar`

Recharts BarChart eksposur per bucket jatuh tempo.

```
Props:
  data: { bucket: '≤30d'|'31-90d'|'91-180d'|'>180d'; nominalIdr: number }[]
  loading?: boolean
  error?: string
```

Visual:
- Single BarChart horizontal
- X-axis: nominal IDR (diformat: "Rp 2,5 M")
- Y-axis: label bucket
- Tooltip: "Rp {nilai} — {count} instrumen" (jika count tersedia)

### 4.5 `ECLRollForwardLine`

Recharts LineChart kumulatif ECL movement.

```
Props:
  data: { tanggal: string; mtdCumulative: number; ytdCumulative: number }[]
  mode: 'mtd' | 'ytd' | 'both'
  loading?: boolean
  error?: string
```

Visual:
- LineChart, 1-2 series sesuai `mode`
- X-axis: tanggal (daily untuk MTD, monthly untuk YTD)
- Y-axis: nominal IDR (positif = tambah impairment, negatif = reversal)
- Reference line pada Y=0

### 4.6 `ScenarioSensitivityBar`

Recharts BarChart 3 skenario ECL.

```
Props:
  data: { scenario: 'Good'|'Normal'|'Bad'; eclTotal: number; weight: number }[]
  weightedEcl: number
  loading?: boolean
  error?: string
```

Visual:
- 3 bar: "Optimis (Good) 25%", "Base (Normal) 50%", "Pesimis (Bad) 25%"
- Warna: Good = hijau, Normal = biru, Bad = merah (dengan pattern berbeda untuk a11y)
- Label nilai di atas setiap bar: "Rp {N} M"
- Tooltip: nilai + delta vs weighted: "+Rp {X} M vs weighted"
- Sub-label di bawah chart: "Bobot skenario aktif: Good {w}% / Normal {w}% / Bad {w}% (ALCO-approved)"

### 4.7 `WorkflowQueueList`

DataTable ringan (tanpa paging) untuk antrian workflow pending.

```
Props:
  data: WorkflowPendingItem[]
  maxRows?: number           — default 20
  showApproveButton?: boolean  — hanya jika caller punya jurnal.approve
  loading?: boolean
  error?: string
  emptyMessage?: string
```

Kolom: ID entitas, tipe entitas, status, submitted_by, submitted_at, aksi (link ke detail).
Tombol "Approve" di-render hanya jika `showApproveButton && JWT.permissions.includes('jurnal.approve')`.

### 4.8 `RecentTransactionsList`

DataTable ringan 10-20 baris terbaru.

```
Props:
  endpoint: string        — report endpoint URL dengan params
  columns: ColumnDef[]
  maxRows?: number        — default 20
  linkToFull?: string     — link "Lihat semua →" ke laporan M14
  loading?: boolean
  error?: string
```

Tidak ada paging — ini widget dashboard bukan full DataTable.

### 4.9 `GLDeliveryStatusGauge`

Donut + KPI untuk success rate GL delivery.

```
Props:
  delivered: number
  failed: number
  pending: number
  loading?: boolean
  error?: string
  alertThresholdPct?: number   — default 5% (tampilkan warning banner jika failed > threshold)
```

Visual:
- PieChart donut kecil (outerRadius=60): DELIVERED hijau, FAILED merah, PENDING amber
- KPI card di sebelah kanan donut: "Success Rate: {pct}% — {delivered} dari {total}"
- Warning banner jika `(failed/total)*100 > alertThresholdPct`

### 4.10 `PeriodeBukuTimeline`

Recharts BarChart horizontal timeline 12 periode.

```
Props:
  data: { kode: string; status: 'OPEN'|'SOFT_CLOSED'|'HARD_CLOSED'; tanggalClose?: string }[]
  currentPeriodeKode?: string
  loading?: boolean
  error?: string
```

Visual:
- BarChart horizontal, 1 bar per periode
- Warna: OPEN = hijau, SOFT_CLOSED = amber, HARD_CLOSED = abu-abu
- Label: "{kode} — {status}" + tanggal close jika tersedia
- Badge "CURRENT" pada periode aktif (rendered di-atas bar via CustomLabel)

### 4.11 `AuditLogVolumeArea`

Recharts AreaChart volume event audit 30 hari.

```
Props:
  data: { tanggal: string; eventCount: number }[]
  totalEvents: number
  loading?: boolean
  error?: string
```

Visual:
- AreaChart, fill dengan opacity 0.2
- X-axis: tanggal (DD/MM)
- Y-axis: jumlah event
- KPI di atas chart: "Total 30 hari: {totalEvents} events"
- Tooltip: "Tanggal {date}: {count} events"

### 4.12 `HashChainStatusBadge`

KPI card status verifikasi hash-chain audit log.

```
Props:
  status: 'VERIFIED' | 'MISMATCH' | 'VERIFYING' | 'UNKNOWN'
  lastRunAt?: string
  jobId?: string
  loading?: boolean
```

Visual:
- VERIFIED: badge hijau `<CheckCircle>` "Hash-chain VERIFIED — last run: {ts}"
- MISMATCH: badge merah `<AlertTriangle>` "PERINGATAN: Hash-chain MISMATCH terdeteksi!" + link detail
- VERIFYING: badge biru spinner "Verifikasi sedang berjalan..."
- UNKNOWN: badge abu-abu "Status tidak diketahui — belum ada run"

### 4.13 `JobStatusList`

Daftar job aktif milik user saat ini dengan mini progress bars. SSE-subscribed per job.

```
Props:
  jobs: ActiveJob[]    — dari polling GET /api/v1/jobs?status=running,queued
  onViewAll?: () => void   — link ke /jobs
  loading?: boolean
  error?: string
```

Visual per job item:
- Nama job + status chip
- Progress bar kecil (h-2)
- ETA teks kecil
- Link "Lihat →" ke /jobs/{jobId}

Data: `GET /api/v1/jobs?status=running,queued&limit=5` polling 10 detik.

---

## 5. Refresh Patterns

### 5.1 Cadence per Widget

| Widget / Dashboard | Mode | Interval | Notes |
|---|---|---|---|
| Semua widget default | 5-min polling | 300.000 ms | Hook `useDashboardPolling.ts`; pause saat `document.visibilityState === 'hidden'` |
| W-RK-05 Calc-Run Status (active job) | SSE push | real-time | Hook `useJobSSE.ts` subscribe ke `/api/v1/jobs/{jobId}/stream`; fallback polling 2s jika SSE error via `SSE_STREAM_UNAVAILABLE` |
| W-AU-02 Hash-Chain Status | 5-min polling | 300.000 ms | Query `GET /api/v1/jobs?type=HASH_CHAIN_VERIFY&sort=created_at:desc&limit=1` |
| `JobStatusList` widget (all dashboards) | 10-detik polling | 10.000 ms | Hanya query jobs milik user; stop saat count=0 |
| `/jobs` DataTable | 5-min polling + manual | 300.000 ms | Refresh badge di header; manual "Refresh" button |
| Manual refresh button | On-demand | — | Trigger re-fetch semua widget seketika via shared query key invalidation |

### 5.2 Global Last-Updated Indicator

Di setiap dashboard page header:

```
[Refresh ↺]   Terakhir diperbarui: 25 Jun 2026, 10:30:45
```

- Timestamp diperbarui setiap kali **salah satu widget** berhasil re-fetch
- Ikon `↺` berputar (animate-spin) selama ada widget yang sedang re-fetching
- Tombol "Refresh" klik: invalidate semua TanStack Query keys untuk dashboard ini → re-fetch serentak

---

## 6. Dashboard Wireframes

Semua dashboard menggunakan **12-kolom grid** pada lebar ≥ 1280px (CSS grid `grid-cols-12`). Notasi di wireframe: `[col-X]` = lebar X kolom dari 12.

### 6.1 Layout Umum (shared shell)

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ TOP BAR                                                                           │
│ [Logo BLIPS] [Nav: Dashboard / Laporan / ...] [Refresh ↺] [Terakhir: HH:MM:SS] │
│                                                              [Notif Badge] [User] │
├──────────────────────────────────────────────────────────────────────────────────┤
│ SIDE NAV (collapsible, 240px saat expanded)                                       │
│ ◉ Dashboard       ← active group                                                  │
│   ├ Treasury      ← active (contoh)                                               │
│   ├ Risk                                                                          │
│   ├ Akuntansi                                                                     │
│   ├ CFO                                                                           │
│   └ Audit                                                                         │
│ ○ Laporan                                                                         │
│ ○ Jobs            [badge: N active jobs]                                          │
│ ...                                                                               │
├──────────────────────────────────────────────────────────────────────────────────┤
│ PAGE CONTENT (flex-1, 12-col grid)                                                │
│ [Page Header: Judul dashboard + Refresh + Last Updated + (active job status)]    │
│ [Widget Grid — lihat wireframe per dashboard]                                    │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Page Header pattern** (dipakai semua dashboard):

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ [TrendingUp icon] Treasury Dashboard                                              │
│ ROLE-MAKER-TR · Budi Santoso                        [↺ Refresh] [Terakhir 10:30] │
│ Periode Aktif: PRD-2026-06 · Juni 2026 — OPEN                                    │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

### 6.2 Treasury Dashboard `/dashboard/treasury`

Grid: 12 kolom. Layout pada ≥ 1280px.

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [TrendingUp] Treasury Dashboard                      [↺ Refresh] [Terakhir 10:30:15]│
│ ROLE-MAKER-TR · {username}     Periode Aktif: PRD-2026-06 — OPEN                    │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                       │
│ ROW 1: KPI Cards (4 × [col-3])                                                       │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ │
│ │ KPICard           │ │ KPICard           │ │ KPICard           │ │ KPICard           │ │
│ │ Total Portfolio   │ │ Jatuh Tempo ≤30h  │ │ Pending Review    │ │ Transaksi Bulan  │ │
│ │ Rp 500 M          │ │ Rp 12 M           │ │ 3 dokumen         │ │ 8 transaksi      │ │
│ │ 2.600 instrumen  │ │ 12 instrumen      │ │ ↓ dari 5 kemarin  │ │ +2 hari ini      │ │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘ └──────────────────┘ │
│                                                                                       │
│ ROW 2: Charts (col-7 + col-5)                                                        │
│ ┌────────────────────────────────────────────┐ ┌──────────────────────────────────┐  │
│ │ W-TR-01 Eksposur Portfolio [col-7]          │ │ W-TR-03 Jatuh Tempo [col-5]     │  │
│ │ BarChart by jenis_instrumen                 │ │ AreaChart timeline               │  │
│ │                                             │ │ bucket 30/60/90 hari            │  │
│ │ DEPOSITO  ████████████████  Rp 200 M        │ │                                 │  │
│ │ OBLIGASI  ████████████      Rp 150 M        │ │   Rp 12M ────                   │  │
│ │ SAHAM     ████████          Rp 100 M        │ │   Rp  8M      ────              │  │
│ │ REKSADANA ████              Rp  50 M        │ │   Rp  3M           ────         │  │
│ │                             [Lihat RPT-01] │ │         30d  60d  90d           │  │
│ └────────────────────────────────────────────┘ └──────────────────────────────────┘  │
│                                                                                       │
│ ROW 3: Charts (col-5 + col-7)                                                        │
│ ┌──────────────────────────────────┐ ┌────────────────────────────────────────────┐  │
│ │ W-TR-02 Eksposur by Bank [col-5] │ │ W-TR-04 Pending Workflow Queue [col-7]    │  │
│ │ BarChart horizontal              │ │ DataTable (max 20 rows, no paging)        │  │
│ │                                  │ │ [badge: 3 dokumen menunggu approval]      │  │
│ │ BCA    ████████████  Rp 120 M   │ │                                           │  │
│ │ BNI    █████████     Rp 90 M    │ │ Kode       Tipe       Submitted   Aksi   │  │
│ │ MANDIRI ████████     Rp 80 M    │ │ INST-0042  Instrumen  5 mnt lalu  [→]   │  │
│ │ BRI    ███████       Rp 70 M    │ │ INST-0089  Instrumen  2 jam lalu  [→]   │  │
│ │        [Lihat RPT-01]           │ │ PLSM-0012  Penempatan 1 hari lalu [→]   │  │
│ └──────────────────────────────────┘ │ [Lihat semua workflow →ent /reports/rpt-26] │  │
│                                      └────────────────────────────────────────────┘  │
│                                                                                       │
│ ROW 4: Recent Transactions (col-12)                                                  │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-TR-05 Transaksi Terbaru (20 terbaru)                  [Lihat semua →]       │    │
│ │ DataTable: kode, jenis, counterparty, nominal, tgl, status                    │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                       │
│ ROW 5: Active Jobs (col-12, collapsed jika tidak ada active job)                     │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ JobStatusList (job milik user ini) — tampil hanya jika ada active/queued job)  │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Widget mapping Treasury:**

| Widget ID | Komponen | Data Source | Refresh |
|---|---|---|---|
| W-TR-01 | Recharts BarChart (inline, bukan widget catalog) | `GET /api/v1/reports/rpt-01?filter[status]=AKTIF&sort=ead_idr:desc&limit=200` | 5-min |
| W-TR-02 | Recharts BarChart horizontal | `GET /api/v1/reports/rpt-01` (group by bank_id client-side) | 5-min |
| W-TR-03 | Recharts AreaChart (MaturityBucketBar variant) | `GET /api/v1/reports/rpt-10?filter[tanggal_jatuh_tempo]=gte:{today}&filter[tanggal_jatuh_tempo]=lte:{today+90d}&sort=tanggal_jatuh_tempo:asc&limit=200` | 5-min |
| W-TR-04 | WorkflowQueueList | `GET /api/v1/reports/rpt-26?filter[entity_type]=INSTRUMEN&filter[status]=PENDING&sort=created_at:desc&limit=20` | 5-min |
| W-TR-05 | RecentTransactionsList | `GET /api/v1/reports/rpt-06?sort=tanggal_penempatan:desc&limit=20` | 5-min |
| KPI Row | KPICard × 4 | Agregat dari rpt-01 + rpt-10 + rpt-26 + rpt-06 | 5-min |

---

### 6.3 Risk Dashboard `/dashboard/risk`

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [Shield] Risk Dashboard                              [↺ Refresh] [Terakhir 10:30:15]│
│ ROLE-RISK · {username}                                                                │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                       │
│ ROW 1: KPI Cards (4 × [col-3])                                                       │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ │
│ │ Total ECL Weighted│ │ Stage 2 Count     │ │ Stage 3 Count     │ │ SICR Triggers    │ │
│ │ KPICard           │ │ KPICard           │ │ KPICard           │ │ KPICard          │ │
│ │ Rp 12,5 M         │ │ 180 instrumen     │ │ 20 instrumen      │ │ 12 bulan ini     │ │
│ │ vs Rp 11,8 M lalu │ │ ↑ 12 dari lalu   │ │ → stabil          │ │ ↑ 3 dari lalu    │ │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘ └──────────────────┘ │
│                                                                                       │
│ ROW 2: Distribusi + Stage Movement (col-5 + col-7)                                   │
│ ┌───────────────────────────────────┐ ┌──────────────────────────────────────────┐   │
│ │ W-RK-01 ECL Stage Distribution   │ │ W-RK-03 Stage Movement Trend [col-7]    │   │
│ │ StageDistributionDonut [col-5]    │ │ LineChart S1/S2/S3 per 6 periode        │   │
│ │                                   │ │                                          │   │
│ │    [Stage 1: 92.3%]               │ │  2600 ─────────────────────────────     │   │
│ │        ●●●●●                      │ │   180                      ── S2        │   │
│ │     ●         ●    Total:         │ │    20                    ── S3          │   │
│ │   ●    2600    ●   2.600          │ │        Jan Feb Mar Apr Mai Jun           │   │
│ │   ●  instrumen ●                  │ │ Legend: — Stage1  ── Stage2  ·· Stage3  │   │
│ │     ●         ●                   │ │                                          │   │
│ │        ●●●                        │ │ [Lihat RPT-14 →]                        │   │
│ │ Stage 1 ■ 92.3%  (Performing)    │ └──────────────────────────────────────────┘   │
│ │ Stage 2 ■  6.9%  (SICR)          │                                                │
│ │ Stage 3 ■  0.8%  (Default)       │                                                │
│ └───────────────────────────────────┘                                                │
│                                                                                       │
│ ROW 3: SICR Triggers + Calc-Run Status (col-4 + col-8)                              │
│ ┌─────────────────────────────────┐ ┌─────────────────────────────────────────────┐  │
│ │ W-RK-02 SICR Triggers [col-4]  │ │ W-RK-05 Calc-Run Status [col-8]            │  │
│ │ KPI cards per trigger type      │ │                                             │  │
│ │                                 │ │ [Jika ada active job → JobProgressPanel]   │  │
│ │ Rating Downgrade ≥2 notch       │ │ ████████████░░░░░░░  47%                  │  │
│ │ [KPICard: 5]                    │ │ Menghitung Stage 2 instruments (1234/2600) │  │
│ │                                 │ │ Mulai: 10:30:00 · ETA: 10:35:00           │  │
│ │ IG → Non-IG                     │ │ [Background]   (cancel: absent, no perm)   │  │
│ │ [KPICard: 4]                    │ │                                             │  │
│ │                                 │ │ [Jika tidak ada active job:]               │  │
│ │ DPD ≥ 30 hari                   │ │ KPICard: "Last Run: CR-2026-06"           │  │
│ │ [KPICard: 3]                    │ │ "COMPLETED — 25 Jun 2026 10:45"           │  │
│ │                                 │ │ "2.600 instrumen diproses"                 │  │
│ │ [Lihat RPT-15 →]                │ │ [Lihat detail →] → /reports/rpt-13        │  │
│ └─────────────────────────────────┘ └─────────────────────────────────────────────┘  │
│                                                                                       │
│ ROW 4: Top-10 Instrumen by ECL (col-12)                                             │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-RK-04 Top-10 Instrumen by ECL Weighted                [Lihat semua →]       │    │
│ │ DataTable (10 rows, no paging)                                                │    │
│ │ Kode Instrumen | Nama | Stage | EAD IDR | ECL Weighted | FL Multiplier       │    │
│ │ OBL-0042  | Obligasi FR... | 3 | Rp 5M | Rp 1.2M | 1.45 | [→ Detail]       │    │
│ │ SHM-0099  | Saham Telkom   | 2 | Rp 3M | Rp 0.8M | 1.32 | [→ Detail]       │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Widget mapping Risk:**

| Widget ID | Komponen | Data Source | Refresh |
|---|---|---|---|
| W-RK-01 | StageDistributionDonut | `GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest}&sort=stage:asc&limit=200` | 5-min |
| W-RK-02 | KPICard × 3 (per trigger type) | `GET /api/v1/reports/rpt-15?filter[periode_id]={current}&sort=tanggal_trigger:desc&limit=50` | 5-min |
| W-RK-03 | StageMovementBar (LineChart variant) | `GET /api/v1/reports/rpt-14?sort=tanggal_transisi:asc&limit=200&filter[periode_id]={last_6}` | 5-min |
| W-RK-04 | RecentTransactionsList (columns ECL-specific) | `GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest}&sort=ecl_weighted:desc&limit=10` | 5-min |
| W-RK-05 | JobProgressPanel (M13) atau KPICard | `GET /api/v1/jobs/{latest_calc_jobId}` + SSE stream | SSE / 5-min |

---

### 6.4 Akuntansi Dashboard `/dashboard/akuntansi`

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [BookOpen] Akuntansi Dashboard                       [↺ Refresh] [Terakhir 10:30:15]│
│ ROLE-AKUN · {username}                                                                │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                       │
│ ROW 1: KPI Cards (4 × [col-3])                                                       │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ │
│ │ Jurnal Pending    │ │ GL Success Rate   │ │ FX Rate Status    │ │ Periode Status   │ │
│ │ KPICard           │ │ KPICard           │ │ KPICard           │ │ KPICard          │ │
│ │ 15 jurnal         │ │ 98.0%             │ │ FRESH ✓           │ │ PRD-2026-06      │ │
│ │ menunggu approval │ │ 980 dari 1.000    │ │ USD 16.250        │ │ OPEN             │ │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘ └──────────────────┘ │
│                                                                                       │
│ ROW 2: Jurnal Pending Queue (col-12)                                                  │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-AK-01 Jurnal Menunggu Posting             [badge merah: 15 menunggu]         │    │
│ │                                                                                │    │
│ │ Jurnal ID   | Kode Event | Instrumen | Nominal IDR  | Submitted By | Aksi     │    │
│ │ JRN-001234  | MTM_FVOCI  | OBL-0042  | Rp 900.000   | Siti A.     | [→]      │    │
│ │ JRN-001235  | EIR_AC     | DEP-0089  | Rp 12.500    | Siti A.     | [→]      │    │
│ │ ...                                                                            │    │
│ │ [ROLE-AKUN: tombol Approve ABSENT dari DOM]                                   │    │
│ │ [ROLE-AKUN-CTL: tombol Approve VISIBLE → /mapping-jurnal/approve/{id}]       │    │
│ │                                             [Lihat semua → /reports/rpt-26]   │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                       │
│ ROW 3: GL Delivery + FX Freshness (col-5 + col-7)                                   │
│ ┌───────────────────────────────────┐ ┌──────────────────────────────────────────┐   │
│ │ W-AK-02 GL Delivery Rate [col-5] │ │ W-AK-03 FX Rate Freshness [col-7]       │   │
│ │ GLDeliveryStatusGauge             │ │                                          │   │
│ │                                   │ │ KPICard: "FX Rate terakhir:"             │   │
│ │  [Donut] 98.0%                    │ │ "25 Jun 2026 — USD 16.250"              │   │
│ │  DELIVERED ■ 98.0% (980)          │ │ "Sumber: JISDOR"                         │   │
│ │  FAILED    ■  1.5%  (15)          │ │ Status: [hijau] FRESH                    │   │
│ │  PENDING   ■  0.5%   (5)          │ │                                          │   │
│ │                                   │ │ [Jika STALE:]                            │   │
│ │  KPI: "Success Rate: 98.0%"       │ │ [merah] STALE — "FX Rate belum diperbarui│   │
│ │  "980 dari 1.000 berhasil"        │ │ hari ini. Upload manual via Pengaturan > │   │
│ │                                   │ │ FX Rate." [→ Upload FX Rate]            │   │
│ │  [Jika FAILED > 5%:]              │ │                                          │   │
│ │  [amber banner] ⚠ Tingkat        │ │ Riwayat 5 hari:                          │   │
│ │  kegagalan GL melebihi 5%        │ │ 25 Jun | USD 16.250 | JISDOR            │   │
│ │                                   │ │ 24 Jun | USD 16.225 | JISDOR            │   │
│ └───────────────────────────────────┘ └──────────────────────────────────────────┘   │
│                                                                                       │
│ ROW 4: Periode Buku Timeline (col-12)                                               │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-AK-04 Periode Buku Timeline (12 periode terakhir)              [→ Detail]   │    │
│ │ PeriodeBukuTimeline                                                           │    │
│ │                                                                               │    │
│ │  [CURRENT] Jun 2026 — OPEN ████████████████████████████░░░░░░░░░░░░░░░░     │    │
│ │  Mei 2026 — HARD_CLOSED      ████████████████████████████████████████     │    │
│ │  Apr 2026 — HARD_CLOSED      ████████████████████████████████████████     │    │
│ │  ...                                                                          │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                       │
│ ROW 5: Recent Jurnal Log (col-12)                                                   │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-AK-05 Log Jurnal Terbaru (20 terbaru)                [Lihat semua →]        │    │
│ │ DataTable: jurnal_id, event_code, instrumen (link), nominal_idr, posted_at    │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Widget mapping Akuntansi:**

| Widget ID | Komponen | Data Source | Refresh | SoD Gate |
|---|---|---|---|---|
| W-AK-01 | WorkflowQueueList | `GET /api/v1/reports/rpt-26?filter[entity_type]=JURNAL&filter[status]=PENDING_APPROVAL&sort=created_at:desc&limit=20` | 5-min | `showApproveButton` = JWT.permissions.includes('jurnal.approve') |
| W-AK-02 | GLDeliveryStatusGauge | `GET /api/v1/reports/rpt-22b?filter[periode_id]={current}&limit=200` | 5-min | — |
| W-AK-03 | KPICard (FX freshness) | `GET /api/v1/reports/rpt-05?sort=tanggal:desc&limit=5` | 5-min | — |
| W-AK-04 | PeriodeBukuTimeline | `GET /api/v1/reports/rpt-23?sort=tanggal_close:desc&limit=12` | 5-min | — |
| W-AK-05 | RecentTransactionsList (columns jurnal) | `GET /api/v1/reports/rpt-22?sort=posted_at:desc&limit=20` | 5-min | — |

---

### 6.5 CFO+Direksi Dashboard `/dashboard/cfo`

**Gate tambahan**: server component wajib check `JWT.mfa_verified === true`. Jika false → redirect `/auth/mfa?returnUrl=/dashboard/cfo` sebelum render apapun.

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [BarChart3] Executive Dashboard                      [↺ Refresh] [Terakhir 10:30:15]│
│ ROLE-CFO · {username}       ● MFA Terverifikasi                                      │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                       │
│ ROW 1: KPI Cards Utama (4 × [col-3])                                                │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐ │
│ │ W-CF-01           │ │ W-CF-02           │ │ W-CF-04           │ │ W-CF-06 Status  │ │
│ │ Total Portfolio   │ │ ECL Coverage      │ │ Stage 3 Ratio     │ │ Periode         │ │
│ │ NAV               │ │ Ratio             │ │                   │ │ PRD-2026-06     │ │
│ │ Rp 500 M          │ │ 2.50%             │ │ 1.50%             │ │ OPEN            │ │
│ │ 2.600 instrumen  │ │ ECL Rp 12,5 M     │ │ EAD Rp 7,5 M     │ │ [→ Hard-Close]  │ │
│ │ per {last run}    │ │ / EAD Rp 500 M    │ │ [hijau < 2%]      │ │                 │ │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘ └──────────────────┘ │
│                                                                                       │
│ ROW 2: ECL Coverage by Stage + Scenario Sensitivity (col-7 + col-5)                 │
│ ┌──────────────────────────────────────────────┐ ┌────────────────────────────────┐  │
│ │ W-CF-02 ECL Coverage by Stage [col-7]        │ │ W-CF-03 Scenario Sensitivity  │  │
│ │ BarChart: Stage 1/2/3, Y=ECL/EAD ratio       │ │ ScenarioSensitivityBar [col-5]│  │
│ │                                               │ │                               │  │
│ │ Stage 1 ██ 0.8%                               │ │ Good   ██ Rp 10,2 M          │  │
│ │ Stage 2 ████████ 3.2%                         │ │ Normal ████ Rp 12,5 M        │  │
│ │ Stage 3 ████████████████ 16%                  │ │ Bad    ██████ Rp 15,8 M      │  │
│ │                                               │ │                               │  │
│ │ Tooltip: "Stage N: ECL Rp X / EAD Rp Y = Z%" │ │ Bobot aktif: G25%/N50%/B25%  │  │
│ │ [Lihat RPT-13]                                │ │ (ALCO-approved)               │  │
│ └──────────────────────────────────────────────┘ │ [Lihat RPT-27]                │  │
│                                                  └────────────────────────────────┘  │
│                                                                                       │
│ ROW 3: P&L ECL Impact MTD/YTD (col-12)                                             │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-CF-05 Dampak P&L ECL — MTD & YTD                   [Lihat RPT-18 →]        │    │
│ │ ECLRollForwardLine (AreaChart)                                                │    │
│ │                                                                               │    │
│ │  Rp 12M ──────────────────── YTD kumulatif                                   │    │
│ │  Rp  2M ───── MTD kumulatif                                                   │    │
│ │  Rp  0 ─────────────────────────────────────────────────────────────         │    │
│ │         Jan  Feb  Mar  Apr  Mai  Jun                                          │    │
│ │                                                                               │    │
│ │ MTD: +Rp 2,1 M impairment · YTD: +Rp 12,5 M impairment                     │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Widget mapping CFO:**

| Widget ID | Komponen | Data Source | Refresh |
|---|---|---|---|
| W-CF-01 | KPICard | `GET /api/v1/reports/rpt-01?filter[status]=AKTIF&limit=200` (sum EAD client-side) | 5-min |
| W-CF-02 | KPICard + BarChart | `GET /api/v1/reports/rpt-13?filter[calc_run_id]={latest}&limit=200` | 5-min |
| W-CF-03 | ScenarioSensitivityBar | `GET /api/v1/reports/rpt-27?filter[calc_run_id]={latest}&w_good=0.25&w_normal=0.50&w_bad=0.25` | 5-min |
| W-CF-04 | KPICard (dengan status color + trend arrow) | `GET /api/v1/reports/rpt-13` (Stage 3 EAD / total EAD) | 5-min |
| W-CF-05 | ECLRollForwardLine | `GET /api/v1/reports/rpt-18?filter[periode_id]={current}` | 5-min |
| W-CF-06 | KPICard (Status Periode) | `GET /api/v1/reports/rpt-23?sort=tanggal_close:desc&limit=1` | 5-min |

**Stage 3 Ratio threshold coloring:**
- `< 2%` → `status='success'` (hijau)
- `2% – 5%` → `status='warning'` (amber)
- `> 5%` → `status='danger'` (merah)

**Trend arrow delta:**
- Bandingkan dengan run periode sebelumnya dari rpt-13 — jika tidak tersedia, sembunyikan delta

---

### 6.6 Auditor Dashboard `/dashboard/audit`

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [ShieldCheck] Auditor Dashboard                      [↺ Refresh] [Terakhir 10:30:15]│
│ ROLE-AUDIT · {username}     Read-Only — Semua aksi mutasi tidak tersedia             │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                       │
│ ROW 1: KPI Cards (3 × [col-4])                                                      │
│ ┌────────────────────────┐ ┌────────────────────────┐ ┌────────────────────────┐     │
│ │ Total Audit Events     │ │ Hash-Chain Status       │ │ SoD Violations         │     │
│ │ KPICard                │ │ HashChainStatusBadge    │ │ KPICard                │     │
│ │ 85.000 events          │ │ VERIFIED ✓              │ │ 3 bulan ini            │     │
│ │ 30 hari terakhir       │ │ Last run: 25 Jun 10:00  │ │ [badge merah] ⚠        │     │
│ │                        │ │ [Lihat detail →]        │ │ [Lihat detail →]       │     │
│ └────────────────────────┘ └────────────────────────┘ └────────────────────────┘     │
│                                                                                       │
│ ROW 2: Audit Log Volume (col-8) + Top Actions (col-4)                               │
│ ┌────────────────────────────────────────────────┐ ┌──────────────────────────────┐  │
│ │ W-AU-01 Volume Audit Log 30 Hari [col-8]       │ │ W-AU-04 Top Action Types    │  │
│ │ AuditLogVolumeArea                             │ │ BarChart horizontal [col-4] │  │
│ │                                                │ │                             │  │
│ │ KPI: Total 30 hari: 85.000 events              │ │ INSTRUMEN.CREATE  ████ 1234 │  │
│ │                                                │ │ JURNAL.POST       ████ 987  │  │
│ │  3000 ─                        ─               │ │ MTM.APPROVE       ███  756  │  │
│ │  2000 ─  ─   ─                ─ ─ ─ ─          │ │ KLASIFIKASI.READ  ██   543  │  │
│ │  1000 ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─                 │ │ ...                         │  │
│ │       1  5  10  15  20  25  30 Jun              │ │ [Lihat RPT-25 →]            │  │
│ └────────────────────────────────────────────────┘ └──────────────────────────────┘  │
│                                                                                       │
│ ROW 3: SoD Violation Alerts (col-12)                                                │
│ ┌────────────────────────────────────────────────────────────────────────────────┐    │
│ │ W-AU-03 Peringatan Pelanggaran SoD               [badge merah: 3 bulan ini]   │    │
│ │                                                                               │    │
│ │ [Jika ada pelanggaran:]                                                       │    │
│ │ Waktu            | Aktor          | Entitas      | Detail                    │    │
│ │ 20 Jun 14:30:00  | USR-MAKER-001  | PENEMPATAN   | Attempted self-review     │    │
│ │ 18 Jun 09:15:00  | USR-APPR-TR-02 | INSTRUMEN    | SoD bypass attempt        │    │
│ │ ...                                                                           │    │
│ │                                                                               │    │
│ │ [Jika tidak ada pelanggaran:]                                                 │    │
│ │ [hijau ✓] Tidak ada pelanggaran SoD yang terdeteksi dalam 30 hari terakhir. │    │
│ └────────────────────────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Widget mapping Audit:**

| Widget ID | Komponen | Data Source | Refresh |
|---|---|---|---|
| W-AU-01 | AuditLogVolumeArea | `GET /api/v1/reports/rpt-25?filter[event_time]=between:{today-30d},{today}&sort=event_time:asc&limit=200` | 5-min |
| W-AU-02 | HashChainStatusBadge | `GET /api/v1/jobs?type=HASH_CHAIN_VERIFY&sort=created_at:desc&limit=1` | 5-min |
| W-AU-03 | WorkflowQueueList (columns: event_time, actor, entity, detail) | `GET /api/v1/reports/rpt-25?filter[action]=SOD_VIOLATION&sort=event_time:desc&limit=20` | 5-min |
| W-AU-04 | Recharts BarChart horizontal (inline) | `GET /api/v1/reports/rpt-25?sort=count:desc&limit=10&group_by=action` | 5-min |

---

## 7. `/jobs` Page Design

### 7.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│ [Clock] Riwayat Job                                  [↺ Refresh] [Terakhir 10:30:15]│
│ Pantau dan kelola background jobs BLIPS                                              │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                            │
│ [Cari job ID atau tipe...]   [Status ▾]  [Tipe ▾]  [Rentang Tanggal ▾]  [Clear]    │
│ Filter chips: [Status: running ×]  [Status: queued ×]                                │
│ (ROLE-IT-ADMIN only): [Filter by User ▾ (typeahead)]                                 │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Batalkan Terpilih] (disabled jika tidak ada selection)   [Export ▾]    │
├────────────┬─────────────────────┬─────────────┬─────────┬────────┬──────┬─────────┤
│ ☐ Job ID   │ Tipe                │ Status      │Progress │Dimulai │Durasi│ Aksi    │
├────────────┼─────────────────────┼─────────────┼─────────┼────────┼──────┼─────────┤
│ ☐JOB-00001 │ ECL Calc Run        │●Berjalan    │ 47%     │10:30   │ 5m   │[→][✗]  │
│ ☐JOB-00002 │ Export MTM Daily    │●Selesai     │100%     │09:15   │ 2m   │[→][↓]  │
│ ☐JOB-00003 │ Export Instrumen    │●Gagal       │ 30%     │08:00   │ 1m   │[→][↺]  │
│ ☐JOB-00004 │ Hash-Chain Verify   │●Selesai     │100%     │07:30   │ 8m   │[→]     │
│ ☐JOB-00005 │ MV Refresh          │●Dibatalkan  │ 15%     │06:00   │ 0m   │[→]     │
├────────────┴─────────────────────┴─────────────┴─────────┴────────┴──────┴─────────┤
│ Footer: [← Prev]  Hal. 1 dari ~5  [Next →]  Tampilkan: [50 ▾]  Total estimasi: 240 │
└──────────────────────────────────────────────────────────────────────────────────────┘

Legenda:
[→] = Lihat detail (drawer)
[✗] = Batalkan (hanya owner + ROLE-IT-ADMIN, hanya status running/queued)
[↓] = Unduh result (hanya jika result_url tersedia)
[↺] = Retry (hanya jika implementasi backend support — opsional M15, defer ke Phase 6)

(ROLE-IT-ADMIN: kolom tambahan "Dibuat Oleh" muncul di antara "Tipe" dan "Status")
```

### 7.2 DataTable Columns

| Kolom | Sortable | Filterable | Format | Lebar |
|---|---|---|---|---|
| Checkbox (select) | — | — | — | 40px |
| Job ID | ya | text search | `JOB-XXXXX` (disingkat) | 120px |
| Tipe | ya | select enum (dropdown) | Label Bahasa Indonesia | 180px |
| Status | ya | multi-select chip | `<JobStatusBadge>` | 120px |
| Progress | ya | — | `{N}%` + progress bar mini (h-2) | 100px |
| Dimulai | ya | date range | `DD MMM HH:mm` | 120px |
| Selesai/ETA | ya | — | `DD MMM HH:mm` atau "ETA: HH:mm" jika running | 120px |
| Durasi | ya | — | `{N}m {N}s` | 80px |
| Dibuat Oleh | ya | typeahead (ROLE-IT-ADMIN only) | display name | 150px (ROLE-IT-ADMIN only) |
| Aksi | — | — | kontekstual (lihat §7.3) | 100px |

**Default sort**: `created_at DESC`.

### 7.3 Aksi per Baris

| Status | Aksi tersedia | Kondisi tambahan |
|---|---|---|
| queued, running | [→ Detail] [✗ Batalkan] | Batalkan: hanya owner ATAU ROLE-IT-ADMIN |
| completed | [→ Detail] [↓ Unduh] | Unduh: hanya jika `result_url` tersedia |
| failed | [→ Detail] | — |
| cancelled | [→ Detail] | — |

**Batalkan**: tombol di-render hanya jika `(job.createdBy === currentUser.id) || currentUser.permissions.includes('jobs.read_all')`. Server-side juga enforce: `JOB_NOT_OWNED_BY_USER` (403).

**Bulk Cancel**: tombol "Batalkan Terpilih" di action bar — aktif hanya jika ada baris yang di-check dengan status `queued` atau `running` DAN user adalah owner atau IT-Admin. Klik → confirmation dialog:
```
"Batalkan {N} job terpilih? Job yang sedang berjalan akan dihentikan. Proses yang sudah selesai tidak bisa dikembalikan."
```

### 7.4 Detail Drawer

Klik [→ Detail] membuka drawer kanan (width 520px, tidak navigasi halaman — anti-pattern: tidak modal baru):

```
┌────────────────────────────────────────────────────────┐
│ Job Detail: JOB-00001                            [×]   │
│ ECL Calc Run                                           │
├────────────────────────────────────────────────────────┤
│                                                        │
│ Status    : ● Berjalan                                 │
│ Dimulai   : 25 Jun 2026 10:30:00                      │
│ ETA       : 25 Jun 2026 10:35:00 (5 menit lagi)       │
│ Dibuat oleh: Budi Santoso (ROLE-RISK)                 │
│ Job ID    : JOB-00001-ECL-2026-06                     │
│                                                        │
│ ─────────────────────────────────────────────────────  │
│ PROGRESS (JobProgressPanel reuse dari M13)             │
│ ████████████░░░░░░░░░░  47%                           │
│ Menghitung Stage 2 instruments (1234 dari 2600)       │
│ [Background]      [Batalkan]  (jika punya permission) │
│                                                        │
│ ─────────────────────────────────────────────────────  │
│ PAYLOAD (collapsed JSON viewer)                        │
│ ▸ { "periodeId": "PRD-2026-06", ... }  [Expand]      │
│                                                        │
│ ─────────────────────────────────────────────────────  │
│ RESULT (tampil saat completed)                         │
│ { "calcRunId": "CR-2026-06", "totalECL": "..." }      │
│ [↓ Unduh File Result]  (jika result_url ada)          │
│                                                        │
│ ─────────────────────────────────────────────────────  │
│ LOGS / STEPS (opsional, jika backend expose)           │
│ [10:30:01] Membaca daftar instrumen aktif...          │
│ [10:30:05] Loaded 2.600 instrumen                     │
│ [10:32:30] Stage 2: 1234/2600 processed               │
└────────────────────────────────────────────────────────┘
```

### 7.5 Job Type Label Map (Bahasa Indonesia)

| Backend type string | Label ID |
|---|---|
| `ECL_CALC_RUN` | ECL Calc Run |
| `EIR_RE_ESTIMATION` | Re-estimasi EIR |
| `PEFINDO_IMPORT` | Import Rating Pefindo |
| `IBPA_IMPORT` | Import Harga IBPA |
| `KSEI_NAB_IMPORT` | Import NAB KSEI |
| `BEI_PRICE_IMPORT` | Import Harga BEI |
| `EXPORT_CSV` | Export CSV |
| `EXPORT_XLSX` | Export XLSX |
| `MV_REFRESH` | Refresh Materialized View |
| `HASH_CHAIN_VERIFY` | Verifikasi Hash-Chain |
| `MTM_DAILY_RUN` | MTM Harian |
| `GL_RECON` | Rekonsiliasi GL |
| `JURNAL_BATCH_POST` | Posting Jurnal Batch |
| (lainnya) | {type} (raw) |

### 7.6 Export /jobs

Export mengikuti filter + sort aktif. Hanya job milik user tersebut (atau semua jika ROLE-IT-ADMIN).

- Dataset ≤ 10k: inline stream
- Dataset > 10k: async job pattern (M13 §3)
- Audit: `EXPORT.GENERATED` dengan entity_type=`sys.job`

### 7.7 Filter Detail

| Filter | Type | Options |
|---|---|---|
| q (search) | text | Job ID, type string |
| status | multi-select chip | queued / running / completed / failed / cancelled |
| type | select | dropdown enum per §7.5 |
| date range | date picker range | started_at |
| owner (ROLE-IT-ADMIN only) | typeahead text | username search |

**URL state**: filter disimpan di URL query params untuk deep-link:
```
/jobs?filter[status][]=running&filter[status][]=queued&sort=created_at:desc
```

---

## 8. Interaction Flows

### 8.1 Dashboard Load — Happy Path

```
1. User navigasi ke /dashboard (atau langsung ke /dashboard/{role})
2. Next.js middleware:
   a. Baca JWT dari cookie
   b. Cek permission('dashboard.{role}.read') → ada → lanjut
   c. Jika /dashboard/cfo: cek mfa_verified → true → lanjut
3. Server component render:
   a. Render Page Header (nama dashboard + user info + tombol Refresh)
   b. Render setiap widget dengan loading skeleton
4. TanStack Query parallel fetch semua widget endpoints
5. Widget berhasil fetch → ganti skeleton dengan data
6. "Last updated" timestamp diperbarui
7. 5-menit polling hook mulai countdown
8. Jika tab background (visibilitychange: hidden) → polling di-pause
9. Saat tab kembali aktif (visible) → trigger immediate re-fetch
```

### 8.2 Permission Denied — Failure Mode

```
1. ROLE-RISK navigasi ke /dashboard/treasury
2. Middleware: check permission('dashboard.treasury.read') → ABSENT
3. Response: DASHBOARD_PERMISSION_DENIED (403)
4. Redirect: 302 → /dashboard (role router)
5. Role router: check JWT.roles → ROLE-RISK → redirect /dashboard/risk
6. Widget /dashboard/treasury TIDAK di-render (absent from DOM)
7. Tidak ada API call ke report endpoints untuk /dashboard/treasury
```

### 8.3 MFA Gate — CFO Dashboard

```
1. ROLE-CFO navigasi ke /dashboard/cfo
2. Middleware: check permission('dashboard.cfo.read') → ada
3. Server component: check JWT.mfa_verified → false
4. Redirect: 302 → /auth/mfa?returnUrl=/dashboard/cfo
5. User selesaikan MFA challenge
6. JWT re-issued dengan mfa_verified=true
7. Redirect ke /dashboard/cfo → semua widget di-render
```

### 8.4 SSE Active Job — Risk Dashboard W-RK-05

```
1. Risk Dashboard load
2. Fetch GET /api/v1/jobs?type=ECL_CALC_RUN&status=running,queued&limit=1
3. Job ditemukan: jobId=JOB-ECL-2026-06, status=running
4. Render <JobProgressPanel jobId="JOB-ECL-2026-06">
5. useJobSSE hook: buka EventSource ke /api/v1/jobs/JOB-ECL-2026-06/stream
6. SSE event 'progress' → update progress bar + step text
7. SSE event 'completed' → update panel ke "COMPLETED" + trigger toast success
8. Toast: "ECL Calc Run CR-2026-06 selesai. Total ECL weighted: Rp 12,5 M." + [Lihat detail →]
9. Panel ganti dari JobProgressPanel ke KPICard "Last Run: COMPLETED {ts}"
10. Jika SSE error → useJobSSE fallback ke polling /api/v1/jobs/{jobId} setiap 2 detik
```

### 8.5 Cancel Job — /jobs

```
1. User klik [✗ Batalkan] pada baris JOB-EXPORT-001
2. DestructiveActionDialog muncul:
   "Batalkan job JOB-EXPORT-001 (Export MTM Daily)? Proses yang sudah selesai tidak bisa dikembalikan."
3. User klik "Batalkan Job" (confirm)
4. POST /api/v1/jobs/JOB-EXPORT-001/cancel + Idempotency-Key
5. Response 200 { status: "cancelled" }
6. toast success: "Job JOB-EXPORT-001 berhasil dibatalkan."
7. Invalidate TanStack Query ['jobs-list'] → baris update ke status CANCELLED
8. Jika job bukan milik user: server return 403 JOB_NOT_OWNED_BY_USER
   → toast error: "Anda tidak memiliki izin untuk membatalkan job ini."
9. Jika job sudah terminal: 409 JOB_ALREADY_TERMINAL
   → toast error: "Job sudah selesai/gagal/dibatalkan — tidak bisa dibatalkan."
```

### 8.6 Download Result — /jobs

```
1. User klik [↓ Unduh] pada baris JOB-EXPORT-002 (status=COMPLETED)
2. Frontend: GET /api/v1/jobs/JOB-EXPORT-002 → ambil result_url (MinIO signed URL)
3. Browser trigger download via window.open(result_url) atau anchor href
4. toast info: "Download dimulai — file {filename} sedang diunduh."
5. Audit: JOB.RESULT_DOWNLOADED di-log oleh MinIO gateway callback
```

---

## 9. Error / Empty / Loading / No-Permission States

### 9.1 Widget States (berlaku semua widget)

| State | Visual | Copy (Bahasa Indonesia) |
|---|---|---|
| Loading | Skeleton rows / skeleton chart area dengan shimmer animation | — |
| Error (fetch gagal) | Border merah tipis + ikon `AlertCircle` merah | "Gagal memuat data. [Coba lagi]" + traceId kecil di bawah |
| Error (timeout > 10s) | Sama dengan error | "Permintaan data melebihi batas waktu. [Coba lagi]" |
| Empty (no data) | Ikon abu-abu + pesan | Lihat §9.2 |
| No permission | Widget ABSENT dari DOM | — (tidak ada pesan; halaman tidak di-render) |

### 9.2 Empty State per Widget

| Widget | Empty Copy | CTA |
|---|---|---|
| W-TR-04 Workflow Queue | "Tidak ada dokumen yang menunggu approval." | [Lihat semua laporan →] |
| W-TR-05 Recent Transactions | "Belum ada transaksi dalam periode ini." | [Lihat laporan penempatan →] |
| W-RK-01 Stage Distribution | "Belum ada data ECL calc run." | [Jalankan ECL Calc Run →] |
| W-RK-04 Top-10 ECL | "Belum ada hasil ECL yang tersedia." | — |
| W-AK-01 Jurnal Pending | "Tidak ada jurnal yang menunggu." | [Lihat semua jurnal →] |
| W-AK-05 Recent Jurnal | "Tidak ada jurnal dalam periode ini." | [Lihat semua jurnal →] |
| W-AU-03 SoD Violations | "Tidak ada pelanggaran SoD yang terdeteksi dalam 30 hari terakhir." (badge hijau) | — |
| /jobs DataTable (no filter) | "Belum ada job yang dijalankan." | — |
| /jobs DataTable (filter aktif) | "Tidak ada job yang cocok dengan filter ini." | [Hapus filter] |

### 9.3 Spesifik FX Freshness (W-AK-03)

| Kondisi | Status | Banner |
|---|---|---|
| tanggal entry = hari ini | FRESH (hijau `✓`) | — |
| tanggal entry < hari ini (hari kerja) | STALE (merah `!`) | "FX Rate belum diperbarui hari ini. Upload manual via Pengaturan > FX Rate." + link |
| tanggal entry = kemarin tapi hari ini libur/weekend | FRESH (abu-abu `○`) | "FX Rate: {tanggal kemarin}. Hari ini libur/weekend — tidak ada JISDOR." |

### 9.4 W-CF-04 Stage 3 Ratio Color Rules

| Ratio | Status | Color |
|---|---|---|
| < 2% | success | text-green-700, border-green-200 |
| 2% – 5% | warning | text-amber-700, border-amber-200 |
| > 5% | danger | text-red-700, border-red-200 |

**Trend arrow**: render `↑` (merah, `ArrowUp` ikon) jika ratio naik vs periode lalu, `↓` (hijau, `ArrowDown`) jika turun, `→` (abu-abu, `Minus`) jika stabil (delta < 0.1%).

### 9.5 W-AU-02 Hash-Chain Mismatch

Jika status = `MISMATCH`:

```
┌────────────────────────────────────────────────────────────┐
│ [!] PERINGATAN: Hash-chain MISMATCH Terdeteksi!            │
│                                                            │
│ Verifikasi terakhir: 25 Jun 2026 10:00                    │
│ {mismatch_count} entry dengan hash tidak valid.           │
│                                                            │
│ Tindakan segera diperlukan. Hubungi IT Admin.             │
│ [Lihat detail job →]  [Kontak IT Admin →]                 │
└────────────────────────────────────────────────────────────┘
```

Background red-50, border red-300. `alert` role, `aria-live="assertive"`.

---

## 10. Accessibility Checklist

### 10.1 Global (berlaku semua screen)

- [ ] Contrast ratio ≥ 4.5:1 untuk semua teks normal; ≥ 3:1 untuk teks besar (≥ 18pt atau 14pt bold) — verified against shadcn HSL vars
- [ ] Semua interactive element reachable via Tab; tab order logical (top-left ke bottom-right)
- [ ] Focus visible: `ring-2 ring-ring ring-offset-2` pada semua focusable element
- [ ] Errors associated via `aria-describedby` ke field yang relevan
- [ ] Color bukan satu-satunya signal: stage badges menggunakan label teks + icon + warna

### 10.2 Recharts Charts — ARIA Pattern

Setiap chart Recharts wajib:

```tsx
<ResponsiveContainer ...>
  <BarChart data={data} role="img" aria-labelledby="chart-title-id" aria-describedby="chart-desc-id">
    <title id="chart-title-id">Eksposur Portfolio by Jenis Instrumen</title>
    <desc id="chart-desc-id">Bar chart menampilkan total EAD IDR per jenis instrumen. Deposito terbesar dengan Rp 200 M, diikuti Obligasi Rp 150 M.</desc>
    {/* ... */}
  </BarChart>
</ResponsiveContainer>

{/* Visually hidden summary table — screen reader only */}
<table className="sr-only" aria-label="Data tabel: Eksposur Portfolio">
  <caption>Data numerik grafik eksposur portfolio per jenis instrumen</caption>
  <thead><tr><th scope="col">Jenis Instrumen</th><th scope="col">EAD IDR</th></tr></thead>
  <tbody>
    {data.map(d => <tr key={d.jenis}><td>{d.jenis}</td><td>{formatIDR(d.ead)}</td></tr>)}
  </tbody>
</table>
```

### 10.3 ARIA Labels per Widget/Screen

| Element | aria-label pattern |
|---|---|
| Setiap widget container | `aria-label="{Nama Widget} — BLIPS {Dashboard} Dashboard"` |
| KPI Card (auto-refresh) | `role="status" aria-live="polite"` |
| Hash-chain MISMATCH banner | `role="alert" aria-live="assertive"` |
| DataTable W-TR-04 | `aria-label="Antrian Workflow Pending — BLIPS Treasury Dashboard"` |
| DataTable /jobs | `aria-label="Riwayat Job BLIPS"` |
| Job row | `aria-label="Job {id} — {type} — {status} — dibuat {created_at}"` |
| Tombol Batalkan job | `aria-label="Batalkan job {id}"` |
| Tombol Unduh result | `aria-label="Unduh hasil job {id}"` |
| Filter Status dropdown | `aria-label="Filter status job"` |
| Filter Tipe dropdown | `aria-label="Filter tipe job"` |
| Tombol Refresh dashboard | `aria-label="Perbarui semua data dashboard {nama}"` |
| Angka Rupiah (KPI card) | `aria-label="{nilai penuh dalam kata}" (e.g. "Lima ratus miliar rupiah")` |
| Chart bar per jenis | `aria-label="{jenis}: Rp {nilai} EAD total"` |
| Chart data point stage | `aria-label="{Periode}: Stage {N} = {count} instrumen"` |

### 10.4 Color Palette — WCAG Safe

Stage colors diambil dari CSS vars shadcn (HSL) untuk konsistensi dengan tema:

| Stage / Status | CSS var / HEX | Ratio vs white bg | WCAG |
|---|---|---|---|
| Stage 1 (Performing) | `hsl(142 76% 36%)` — hijau medium | 4.6:1 | AA ✓ |
| Stage 2 (SICR) | `hsl(38 92% 50%)` — amber (teks: amber-800 `hsl(32 95% 24%)`) | 7.1:1 | AAA ✓ |
| Stage 3 (Default) | `hsl(0 72% 51%)` — merah (text: red-800 jika perlu) | 4.5:1 | AA ✓ |
| FRESH (hijau) | `hsl(142 76% 36%)` | 4.6:1 | AA ✓ |
| STALE (merah) | `hsl(0 72% 51%)` | 4.5:1 | AA ✓ |
| VERIFIED (hijau) | `hsl(142 76% 36%)` | 4.6:1 | AA ✓ |
| MISMATCH (merah) | Teks red-800 `hsl(0 75% 20%)` | 10.7:1 | AAA ✓ |

**Color-blind safe**: setiap penggunaan warna dipadukan dengan icon Lucide (shape) dan label teks sehingga tidak hanya mengandalkan persepsi warna. Pattern fill opsional untuk chart prints.

### 10.5 Keyboard Navigation — /jobs

Tab order: Filter bar → Action bar ("Batalkan Terpilih", "Export") → DataTable header (sortable columns) → DataTable rows (checkbox → cells → action buttons) → Pagination controls.

DataTable sortable header: `aria-sort="ascending|descending|none"`. Enter/Space untuk toggle sort.

---

## 11. Bahasa Indonesia Copy Table

### 11.1 Page Headers & Titles

| Element | Bahasa Indonesia | English (export/technical) |
|---|---|---|
| Treasury Dashboard title | "Treasury Dashboard" | Treasury Dashboard |
| Risk Dashboard title | "Risk Dashboard" | Risk Dashboard |
| Akuntansi Dashboard title | "Akuntansi Dashboard" | Accounting Dashboard |
| CFO Dashboard title | "Executive Dashboard" | Executive Dashboard |
| Audit Dashboard title | "Auditor Dashboard" | Auditor Dashboard |
| /jobs page title | "Riwayat Job" | Job History |
| Refresh button | "Perbarui" (ikon ↺) | Refresh |
| Last updated | "Terakhir diperbarui: {ts}" | Last updated: {ts} |
| Periode aktif | "Periode Aktif: {kode} — {status}" | Active Period: {kode} — {status} |

### 11.2 Widget Labels

| Widget | Label ID | Tooltip ID (jika ada) |
|---|---|---|
| Total Portfolio NAV | "Total Portfolio NAV" | "Total nilai eksposur (EAD IDR) seluruh instrumen aktif" |
| ECL Coverage Ratio | "ECL Coverage Ratio" | "Rasio total ECL weighted terhadap total EAD IDR" |
| Stage 3 Ratio | "Stage 3 Ratio" | "Persentase EAD instrumen Stage 3 (kredit bermasalah) terhadap total EAD" |
| ECL Stage Distribution | "Distribusi Stage ECL" | "Distribusi jumlah instrumen per stage ECL pada calc run terkini" |
| Stage Movement Trend | "Tren Perpindahan Stage ECL" | "Jumlah instrumen per stage dalam 6 periode terakhir" |
| SICR Triggers Counter | "Pemicu SICR Periode Ini" | "Jumlah kejadian SICR trigger sejak awal periode berjalan" |
| Pending Workflow Queue | "Antrian Menunggu Approval" | "Dokumen yang menunggu review atau approval" |
| Recent Transactions | "Transaksi Terbaru" | "Daftar transaksi terbaru dalam sistem" |
| GL Delivery Success Rate | "Success Rate GL Delivery" | "Persentase jurnal yang berhasil dikirim ke sistem GL" |
| FX Rate Freshness | "Kurs FX Terkini" | "Status kurs BI JISDOR — FRESH = sudah diperbarui hari ini" |
| Periode Buku Timeline | "Timeline Periode Buku" | "Status penutupan 12 periode buku terakhir" |
| Audit Log Volume | "Volume Audit Log (30 Hari)" | "Jumlah event audit log yang tercatat per hari selama 30 hari terakhir" |
| Hash-Chain Status | "Status Verifikasi Hash-Chain" | "Hasil verifikasi integritas rantai hash audit log" |
| SoD Violation Alerts | "Peringatan Pelanggaran SoD" | "Pelanggaran Segregation of Duties yang terdeteksi di audit log" |
| Top Action Types | "Tipe Aksi Terbanyak" | "10 jenis event audit dengan frekuensi tertinggi" |
| Scenario Sensitivity | "Sensitivitas Skenario ECL" | "Perbandingan total ECL per skenario: Optimis, Base, Pesimis" |
| P&L ECL Impact MTD/YTD | "Dampak P&L ECL — MTD & YTD" | "Pergerakan kumulatif ECL terhadap P&L bulan ini dan tahun ini" |
| Hard-Close Status | "Status Penutupan Periode" | "Status hard-close periode buku aktif" |
| Calc-Run Status | "Status Calc Run Terakhir" | "Progress atau hasil ECL calculation run" |
| Eksposur Portfolio | "Eksposur Portfolio by Jenis" | "Total EAD per jenis instrumen" |
| Eksposur by Bank | "Eksposur by Bank/Counterparty" | "Total EAD per bank atau counterparty" |
| Upcoming Maturities | "Jatuh Tempo Mendatang" | "Nominal instrumen yang jatuh tempo dalam 30/60/90 hari" |
| Recent Jurnal Log | "Log Jurnal Terbaru" | "Jurnal posting terbaru" |

### 11.3 Status Labels

| Status / Kondisi | Label ID | Label EN (export) |
|---|---|---|
| Job: queued | Antri | Queued |
| Job: running | Berjalan | Running |
| Job: completed | Selesai | Completed |
| Job: failed | Gagal | Failed |
| Job: cancelled | Dibatalkan | Cancelled |
| FX: FRESH | Terkini | Fresh |
| FX: STALE | Kedaluwarsa | Stale |
| Hash-chain: VERIFIED | Terverifikasi | Verified |
| Hash-chain: MISMATCH | Tidak Cocok | Mismatch |
| Hash-chain: VERIFYING | Sedang Diverifikasi | Verifying |
| GL: DELIVERED | Terkirim | Delivered |
| GL: FAILED | Gagal Terkirim | Failed |
| GL: PENDING | Menunggu | Pending |

### 11.4 Tombol & Aksi

| Aksi | Label ID |
|---|---|
| Lihat detail | "Lihat detail" |
| Batalkan job | "Batalkan" |
| Unduh hasil | "Unduh" |
| Batalkan terpilih | "Batalkan Terpilih" |
| Export dropdown | "Export ▾" |
| Hapus filter | "Hapus Filter" |
| Lihat semua | "Lihat semua →" |
| Coba lagi | "Coba lagi" |
| Background (job) | "Background" |
| Approve jurnal | "Setujui" |

### 11.5 Toast & Notification Copy

| Kondisi | Toast (ID) | Level |
|---|---|---|
| Calc run selesai (SSE) | "ECL Calc Run {id} selesai. Total ECL weighted: Rp {nilai}." + [Lihat detail →] | success (4s) |
| Job cancel berhasil | "Job {id} berhasil dibatalkan." | success (4s) |
| Download dimulai | "Download dimulai — file {filename} sedang diunduh." | info (4s) |
| Widget fetch gagal | "Gagal memuat {nama widget}. [Coba lagi]" + traceId | error (persistent) |
| Permission denied (redirect) | — (tidak ada toast; redirect saja) | — |
| MFA required (redirect) | — (redirect ke /auth/mfa) | — |
| JOB_NOT_OWNED_BY_USER | "Anda tidak memiliki izin untuk membatalkan job ini." | error (persistent) |
| JOB_ALREADY_TERMINAL | "Job sudah selesai/gagal/dibatalkan — tidak bisa dibatalkan lagi." | error (persistent) |
| SSE_STREAM_UNAVAILABLE | "Live update tidak tersedia. Beralih ke polling otomatis." | warning (8s) |
| DASHBOARD_WIDGET_TIMEOUT | "Permintaan data {nama widget} melebihi batas waktu (10 detik). [Coba lagi]" | error (persistent) |

### 11.6 Empty State Copy

| Widget / Screen | Empty message (ID) |
|---|---|
| W-TR-04 Workflow Queue | "Tidak ada dokumen yang menunggu approval saat ini." |
| W-TR-05 Recent Transactions | "Belum ada transaksi dalam periode ini." |
| W-RK-01 Stage Distribution | "Belum ada data ECL calc run untuk periode ini." |
| W-AK-01 Jurnal Pending | "Tidak ada jurnal yang menunggu approval saat ini." |
| W-AK-05 Recent Jurnal | "Tidak ada jurnal yang tersedia untuk periode ini." |
| W-AU-03 SoD Alerts | "Tidak ada pelanggaran SoD yang terdeteksi dalam 30 hari terakhir." |
| /jobs (tanpa filter) | "Belum ada job yang dijalankan." |
| /jobs (dengan filter aktif) | "Tidak ada job yang cocok dengan filter ini." |

---

## 12. Component Catalog — Baru di M15

Semua komponen baru di `web/components/blips/dashboard/`.

### 12.1 Komponen Baru

| Komponen | File | Reuse dari |
|---|---|---|
| `KPICard` | `dashboard/KPICard.tsx` | Baru |
| `StageDistributionDonut` | `dashboard/StageDistributionDonut.tsx` | Baru (Recharts PieChart) |
| `StageMovementBar` | `dashboard/StageMovementBar.tsx` | Baru (Recharts LineChart variant) |
| `MaturityBucketBar` | `dashboard/MaturityBucketBar.tsx` | Baru (Recharts BarChart) |
| `ECLRollForwardLine` | `dashboard/ECLRollForwardLine.tsx` | Baru (Recharts AreaChart) |
| `ScenarioSensitivityBar` | `dashboard/ScenarioSensitivityBar.tsx` | Baru (Recharts BarChart) |
| `WorkflowQueueList` | `dashboard/WorkflowQueueList.tsx` | Reuse `DataTable` |
| `RecentTransactionsList` | `dashboard/RecentTransactionsList.tsx` | Reuse `DataTable` |
| `GLDeliveryStatusGauge` | `dashboard/GLDeliveryStatusGauge.tsx` | Baru (Recharts PieChart) |
| `PeriodeBukuTimeline` | `dashboard/PeriodeBukuTimeline.tsx` | Baru (Recharts BarChart horizontal) |
| `AuditLogVolumeArea` | `dashboard/AuditLogVolumeArea.tsx` | Baru (Recharts AreaChart) |
| `HashChainStatusBadge` | `dashboard/HashChainStatusBadge.tsx` | Baru |
| `JobStatusList` | `dashboard/JobStatusList.tsx` | Reuse `JobProgressPanel` pattern |
| `DashboardPageHeader` | `dashboard/DashboardPageHeader.tsx` | Baru (shared header pattern) |
| `WidgetErrorBoundary` | `dashboard/WidgetErrorBoundary.tsx` | Baru (React Error Boundary per widget) |
| `WidgetSkeleton` | `dashboard/WidgetSkeleton.tsx` | Baru (skeleton variants) |
| `JobStatusBadge` | `dashboard/JobStatusBadge.tsx` | Baru (untuk /jobs DataTable) |

### 12.2 Komponen Reuse dari M13

| Komponen | Sumber | Cara dipakai di M15 |
|---|---|---|
| `JobProgressPanel` | `components/blips/JobProgressPanel.tsx` (M13) | W-RK-05 Calc-Run Status + /jobs detail drawer |
| `DataTable` | `components/blips/DataTable.tsx` | W-TR-04, W-TR-05, W-AK-01, W-AK-05, W-AU-03, /jobs |
| `notify` | `lib/notify.ts` | Toast notifications semua dashboard |

### 12.3 Hooks Baru

| Hook | File | Fungsi |
|---|---|---|
| `useDashboardPolling` | `hooks/useDashboardPolling.ts` | 5-min polling dengan visibilitychange pause |
| `useJobSSE` | `hooks/useJobSSE.ts` | SSE subscription per jobId dengan fallback polling 2s |

**`useDashboardPolling.ts` interface:**

```typescript
function useDashboardPolling(
  queryKeys: QueryKey[],           // TanStack Query keys untuk di-invalidate
  intervalMs: number = 300_000,    // 5 menit default
  options?: {
    pauseOnHidden?: boolean;       // default true (visibilitychange)
    enabled?: boolean;             // default true
  }
): { lastUpdated: Date | null; isRefetching: boolean; manualRefresh: () => void }
```

**`useJobSSE.ts` interface:**

```typescript
function useJobSSE(
  jobId: string | null,
  options?: {
    onProgress?: (data: JobProgressEvent) => void;
    onCompleted?: (data: JobCompletedEvent) => void;
    onFailed?: (data: JobFailedEvent) => void;
    fallbackPollingMs?: number;    // default 2000ms
  }
): { state: JobState; isSSEConnected: boolean }
```

---

## 13. Design Tokens — Delta dari shadcn Defaults

Minimal delta — ikuti shadcn HSL CSS vars. Hanya tambahan berikut:

```css
/* _dashboard.css — extend dari globals.css */

/* Stage colors — consistent dengan glossary */
--stage-1: 142 76% 36%;     /* hijau */
--stage-2: 32 95% 42%;      /* amber-700 (text pada light bg) */
--stage-3: 0 72% 51%;       /* merah */

/* Chart palette (extend shadcn chart vars) */
--chart-1: 142 76% 36%;     /* hijau — Stage 1, Good scenario */
--chart-2: 38 92% 50%;      /* amber — Stage 2, Normal scenario */
--chart-3: 0 72% 51%;       /* merah — Stage 3, Bad scenario */
--chart-4: 217 91% 60%;     /* biru — misc chart series */
--chart-5: 262 83% 58%;     /* ungu — misc chart series */

/* Status colors */
--status-fresh: 142 76% 36%;
--status-stale: 0 72% 51%;
--status-warning: 38 92% 50%;

/* Widget spacing */
--widget-padding: 1.5rem;       /* p-6 */
--widget-gap: 1rem;             /* gap-4 */
--widget-border-radius: 0.5rem; /* rounded-lg (shadcn default) */

/* Dashboard grid */
--dashboard-cols: 12;
--dashboard-gap: 1rem;         /* gap-4 */
```

Typography: tidak ada delta dari shadcn defaults. Dashboard menggunakan `text-sm` untuk data, `text-xs` untuk sub-labels dan timestamps.

---

## 14. Handoff Checklist ke Frontend Engineer

### 14.1 Komponen yang Perlu Dibuat

Buat di `web/components/blips/dashboard/`:

- [ ] `KPICard.tsx` — props per §4.1; aria-live="polite"; loading skeleton; status coloring
- [ ] `StageDistributionDonut.tsx` — Recharts PieChart; innerRadius=60; sr-only table; `<title>` + `<desc>`
- [ ] `StageMovementBar.tsx` — Recharts LineChart; 3 series; legend; aria labels per data point
- [ ] `MaturityBucketBar.tsx` — Recharts BarChart; bucket labels; tooltips
- [ ] `ECLRollForwardLine.tsx` — Recharts AreaChart; 2 series (MTD, YTD); reference line Y=0
- [ ] `ScenarioSensitivityBar.tsx` — Recharts BarChart; 3 bars; bobot sub-label; delta tooltip
- [ ] `WorkflowQueueList.tsx` — `DataTable` wrapper; `showApproveButton` gate; badge counter
- [ ] `RecentTransactionsList.tsx` — `DataTable` wrapper; `linkToFull` prop; max rows
- [ ] `GLDeliveryStatusGauge.tsx` — Recharts PieChart donut; KPI text; alertThreshold banner
- [ ] `PeriodeBukuTimeline.tsx` — Recharts BarChart horizontal; CURRENT badge; status coloring
- [ ] `AuditLogVolumeArea.tsx` — Recharts AreaChart; KPI total; daily X-axis
- [ ] `HashChainStatusBadge.tsx` — 4 states; MISMATCH alert role + aria-live assertive
- [ ] `JobStatusList.tsx` — polling 10s; mini progress bars; SSE per active job
- [ ] `DashboardPageHeader.tsx` — title, user info, refresh button, last-updated indicator, periode aktif
- [ ] `WidgetErrorBoundary.tsx` — React Error Boundary; border merah; "Coba lagi" button
- [ ] `WidgetSkeleton.tsx` — KPI skeleton, chart skeleton, table skeleton variants
- [ ] `JobStatusBadge.tsx` — 5 states; icon + text + color (WCAG AA)

### 14.2 Hooks yang Perlu Dibuat

- [ ] `web/hooks/useDashboardPolling.ts` — interface per §12.3
- [ ] `web/hooks/useJobSSE.ts` — interface per §12.3; EventSource + fallback polling; cleanup on unmount

### 14.3 Routes (Next.js App Router)

- [ ] `web/app/(protected)/dashboard/page.tsx` — role-based redirect
- [ ] `web/app/(protected)/dashboard/treasury/page.tsx` — Treasury Dashboard; permission gate
- [ ] `web/app/(protected)/dashboard/risk/page.tsx` — Risk Dashboard; permission gate
- [ ] `web/app/(protected)/dashboard/akuntansi/page.tsx` — Akuntansi Dashboard; permission gate + SoD button gate
- [ ] `web/app/(protected)/dashboard/cfo/page.tsx` — CFO Dashboard; permission gate + mfa_verified check
- [ ] `web/app/(protected)/dashboard/audit/page.tsx` — Auditor Dashboard; permission gate
- [ ] `web/app/(protected)/jobs/page.tsx` — /jobs DataTable; permission gate jobs.read
- [ ] `web/middleware.ts` (update) — role-default redirect; DASHBOARD_PERMISSION_DENIED; mfa_verified check untuk /dashboard/cfo

### 14.4 TanStack Query Keys

```typescript
// Dashboard query keys
['dashboard-treasury', periodeId]
['dashboard-risk', latestCalcRunId]
['dashboard-akuntansi', periodeId]
['dashboard-cfo', latestCalcRunId]
['dashboard-audit']

// Widget-level keys (untuk selective invalidation)
['widget-rpt', slug, filters]          // generic per report
['widget-jobs-active']                 // JobStatusList
['jobs-list', filters, sort, cursor]   // /jobs DataTable
['job-detail', jobId]                  // drawer detail
['job-sse', jobId]                     // managed by useJobSSE (tidak di-cache TanStack)
```

### 14.5 Validasi & Rules Client-Side

| Rule | Widget | Perilaku |
|---|---|---|
| Stage 3 Ratio coloring | W-CF-04 | `< 2%` = success, `2-5%` = warning, `> 5%` = danger (hardcoded threshold, tidak dari API) |
| GL FAILED threshold | W-AK-02 | `> 5%` = tampilkan warning banner (ambil dari `alertThresholdPct` prop, default 5) |
| FX Freshness | W-AK-03 | Compare `entry.tanggal` vs `today` (hari kerja). Weekend/libur = status FRESH dengan catatan |
| Tombol Batalkan job | /jobs | Render hanya jika `(job.createdBy === currentUser.id) \|\| hasPermission('jobs.read_all')` |
| Tombol Approve jurnal | W-AK-01 | Render hanya jika `hasPermission('jurnal.approve')` |
| mfa_verified check | /dashboard/cfo | Server component check sebelum render — tidak client-side |
| Progress polling | useJobSSE | Stop SSE + stop polling saat `status IN ['completed','failed','cancelled']` |

### 14.6 Permission Constants

```typescript
// web/lib/permissions.ts
export const PERMISSIONS = {
  DASHBOARD_TREASURY: 'dashboard.treasury.read',
  DASHBOARD_RISK: 'dashboard.risk.read',
  DASHBOARD_AKUNTANSI: 'dashboard.akuntansi.read',
  DASHBOARD_CFO: 'dashboard.cfo.read',
  DASHBOARD_AUDIT: 'dashboard.audit.read',
  JOBS_READ: 'jobs.read',
  JOBS_READ_ALL: 'jobs.read_all',
  JURNAL_APPROVE: 'jurnal.approve',
} as const;

export const ROLE_DEFAULT_DASHBOARD: Record<string, string> = {
  'ROLE-MAKER-TR': '/dashboard/treasury',
  'ROLE-APPR-TR': '/dashboard/treasury',
  'ROLE-RISK': '/dashboard/risk',
  'ROLE-AKUN': '/dashboard/akuntansi',
  'ROLE-AKUN-CTL': '/dashboard/akuntansi',
  'ROLE-CFO': '/dashboard/cfo',
  'ROLE-CEO': '/dashboard/cfo',
  'ROLE-KOMITE': '/dashboard/cfo',
  'ROLE-ALCO': '/dashboard/cfo',
  'ROLE-AUDIT': '/dashboard/audit',
  'ROLE-IT-ADMIN': '/jobs',
};
```

---

## 15. Deviasi dari Pola M4/M6

| Aspek | M4/M6 | M15 | Alasan |
|---|---|---|---|
| Layout | 1-2 kolom content | 12-kolom grid dengan multiple widgets | Dashboard butuh high-density multi-widget layout |
| Data fetch | Single entity (form/list) | Parallel multi-endpoint per widget | Setiap widget memiliki data source berbeda |
| Polling | Tidak ada (mutating screens) | 5-min polling + SSE hybrid | Dashboard read-only butuh auto-refresh |
| Error boundary | Page-level | Per-widget (WidgetErrorBoundary) | Satu widget gagal tidak boleh break seluruh dashboard |
| Navigation | Side nav + page | Shared layout + per-role redirect | 5 distinct dashboards memerlukan routing strategy |
| JobProgressPanel | Diembed di specific action (cron trigger) | Diembed langsung di widget W-RK-05 + /jobs drawer | Dashboard harus show active job tanpa user trigger |
| Export | Per-page export | Widget-level "Lihat semua →" link ke M14 report | Dashboard bukan laporan — redirect ke M14 untuk export |

---

## 16. Anti-Patterns yang Dilarang (M15 specific)

- Tidak boleh menyembunyikan widget via CSS `display:none` untuk role yang tidak punya permission — harus **absent from DOM** (server component check).
- Tidak boleh auto-refresh saat `document.visibilityState === 'hidden'` — pause polling saat tab hidden.
- Tidak boleh SSE polling interval < 2 detik sebagai fallback.
- Tidak boleh render /dashboard/cfo tanpa `mfa_verified === true` check di server component.
- Tidak boleh tombol "Batalkan Job" di-render untuk user yang bukan owner dan bukan IT-Admin — server-side check juga wajib (defense in depth).
- Tidak boleh dashboard widget melakukan mutasi data — M15 adalah read-only sepenuhnya.
- Tidak boleh chart Recharts tanpa `<title>` SVG + visually-hidden summary table.
- Tidak boleh KPI card update otomatis tanpa `aria-live="polite"` (screen reader tidak tahu nilai berubah).
- Tidak boleh stack modal di /jobs drawer — drawer kanan bukan modal baru di atas modal.

---

_Design spec ini siap dihandoff ke `frontend-engineer-nextjs`. `security-engineer` **BLOCKING** sebelum deploy: verifikasi absent-from-DOM permission gates, JWT `mfa_verified` server component check `/dashboard/cfo`, SSE auth mechanism, dan `JOB_NOT_OWNED_BY_USER` server-side enforcement. Tidak ada `ifrs9-compliance-reviewer` gate (M15 read-only, tidak compute ECL/EIR/SPPI/BM)._
