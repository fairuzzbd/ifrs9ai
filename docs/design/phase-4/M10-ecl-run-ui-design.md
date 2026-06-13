# P4-M10 — ECL Calc Run UI Design Specification

**Story Set**: P4-M10
**Modul**: APP-C — ECL Engine
**Desainer**: uiux-designer
**Tanggal**: 2026-06-13
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-4/M10-ecl-run-ui.md`

**OQ Resolutions applied**:
- OQ-M10-007-C: Seal = 4-eyes (RISK request → ALCO approve). Jika compliance reviewer memutuskan 6-eyes, section S7 perlu revisi layout.
- OQ-M10-003-B: `<JSONBTreeView>` reuse dari M9 (PR #82).
- OQ-M10-004-A: Drill-down endpoint `GET /api/v1/ecl/results/{calcRunId}/instrumen/{instrumenId}` dianggap tersedia (M7).
- OQ-M10-006-A: Roll-forward endpoint `GET /api/v1/ecl/results/{calcRunId}/roll-forward` tersedia (M7); M11 akan extend.

---

## 1. Information Architecture

### Sitemap M10

```
ECL (side nav group, collapsible)
├── Calc Runs
│   ├── /ecl/calc-runs                                       — List + create (S1)
│   ├── /ecl/calc-runs/new                                   — Create modal (inline di list, S1)
│   ├── /ecl/calc-runs/[id]                                  — Detail + actions (S2, S3, S7)
│   ├── /ecl/calc-runs/[id]/instrumen/[instrumenId]          — Drill-down per instrumen (S4)
│   ├── /ecl/calc-runs/[id]/portofolio/[portofolioId]/summary— Portfolio summary (S5)
│   └── /ecl/calc-runs/[id]/roll-forward                    — Roll-forward report (S6)
└── Jobs
    └── /ecl/jobs/[jobId]                                    — Long-running job status (reuse M9)
```

### Navigasi side nav

```
ECL
  ▾ Calc Runs
      Semua Calc Run         → /ecl/calc-runs
  ▾ Staging
      Antrian Override
      Record DPD
  ▾ EIR
      Antrian Amandemen
      Drift Report
```

"Semua Calc Run" adalah satu-satunya entry point nav — subhalaman (detail, drill-down, roll-forward) diakses via breadcrumb dan tabel, bukan via nav.

---

## 2. Wireframes — 6 Screens

### SCREEN-M10-01: Calc Run List + Create (`/ecl/calc-runs`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                  │
│  Perhitungan ECL — Calc Runs          [Buat Calc Run Baru] (ROLE-RISK only) │
├─────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                   │
│ [🔍 Cari ID / periode...]   [Periode ▾]   [Status ▾]   [Dibuat Oleh ▾]     │
│ Filter chips aktif: [Periode: JUNI-2026 ×]  [Status: COMPLETED ×]           │
│                                                     [Clear semua]           │
├─────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]   "Diperbarui: 10:35 WIB"   │
├──────────────┬───────────┬──────────────────────────┬───────┬───────┬──────┤
│ ID ↕         │ Periode ↕ │ Status                   │Instr ↕│Error ↕│Aksi  │
├──────────────┼───────────┼──────────────────────────┼───────┼───────┼──────┤
│CR-2026-07-001│ JULI-2026 │[abu]     DRAFT           │  —    │  —    │Detail│
│CR-2026-06-001│ JUNI-2026 │[hijau]   SELESAI         │  995  │  0    │Detail│
│CR-2026-06-003│ JUNI-2026 │[amber]   SELESAI+ERROR   │  990  │  3    │Detail│
│CR-2026-06-002│ JUNI-2026 │[merah-m] DIBATALKAN      │  —    │  —    │Detail│
│CR-2026-05-001│ MEI-2026  │[ungu 🔒] TERSEGEL        │ 1002  │  0    │Detail│
├──────────────┴───────────┴──────────────────────────┴───────┴───────┴──────┤
│ Footer: [← Prev]  Page 1 of ~2  [Next →]  Limit: [50 ▾]  Total estimasi: 60│
└─────────────────────────────────────────────────────────────────────────────┘
```

**Status badge mapping** (warna + ikon + teks — TIDAK hanya warna):

| Status backend        | Label UI             | Badge style            |
|-----------------------|----------------------|------------------------|
| `DRAFT`               | DRAFT                | abu-abu, ikon lingkaran|
| `IN_PROGRESS`         | Sedang Berjalan      | biru + spinner ikon    |
| `COMPLETED`           | SELESAI              | hijau, ikon centang    |
| `COMPLETED_WITH_ERRORS` | SELESAI+ERROR      | amber, ikon seru       |
| `SEAL_REQUESTED`      | Menunggu Seal        | kuning, ikon jam       |
| `SEALED`              | TERSEGEL             | ungu + ikon gembok 🔒  |
| `CANCELLED`           | DIBATALKAN           | merah-muted, ikon x    |

**Create modal** (muncul in-page, bukan navigasi baru):

```
┌─────────────────────────────────────────────┐
│ Buat Calc Run Baru                      [×] │
├─────────────────────────────────────────────┤
│ Periode Buku *                              │
│ [Pilih periode ▾]                           │
│  • JULI-2026 (OPEN)                         │
│  • JUNI-2026 (OPEN)                         │
│  • MEI-2026 (SOFT_CLOSED)                   │
│  — APRIL-2026 tidak tersedia (HARD_CLOSED)  │
│                                             │
│ Tanggal Evaluasi *                          │
│ [2026-06-13      📅]                        │
│                                             │
│ ─────────────────────────────────────────── │
│                    [Batal]  [Buat  ⏳→Buat] │
└─────────────────────────────────────────────┘
```

**Annotasi komponen:**
- DataTable: `<DataTable>` — sort semua kolom, cursor pagination, filter chips, export CSV/XLSX
- Status badge: `<CalcRunStatusBadge>` baru (extend pola `<StageBadge>` M9) — warna + ikon + teks
- Create modal: shadcn `<Dialog>`, periode picker `<Select>` (opsi di-fetch exclude `HARD_CLOSED`), eval date `<DatePicker>` default hari ini
- Button state: `submitting` → spinner inline, disabled; double-submit blocked via Idempotency-Key
- Toast sukses/error: `notify` dari `lib/notify.ts`

---

### SCREEN-M10-02: Calc Run Detail + Progress (`/ecl/calc-runs/[id]`)

Layout dua-zona: header card (sticky saat scroll) + scrollable body.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Calc Runs › CR-JUNI-2026-001                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ HEADER CARD (sticky top)                                                     │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ CR-JUNI-2026-001                          [SELESAI ✓] (badge hijau)   │   │
│ │ Periode: JUNI-2026  │  Eval Date: 2026-06-30  │  Dibuat: RISK-01      │   │
│ │ Mulai: 13 Jun 10:30  │  Selesai: 13 Jun 10:35 (durasi 5 menit)       │   │
│ │ Diproses: 995 / 1000  │  Error: 0                                     │   │
│ │                                                                       │   │
│ │ ACTION ROW (kontekstual per status):                                  │   │
│ │ [COMPLETED]  [Request Seal]  [Lihat Audit Trail ↗]                   │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [SEAL_REQUESTED state — full-width info bar kuning]                          │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ ⏳ Menunggu persetujuan seal — diminta RISK-01 pada 13 Jun 11:00 WIB  │   │
│ │  Catatan: "ECL JUNI-2026 siap di-seal, sudah direview tim Risk."      │   │
│ │  [Approve Seal] (ALCO/CFO only, SoD enforced)  [Tolak Seal]          │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [SEALED state — full-width info bar ungu]                                    │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ 🔒 Tersegel — 14 Jun 2026 09:00 WIB  oleh ALCO-01                    │   │
│ │  Tanda tangan digital: abc12345def67890... [Salin 📋]                 │   │
│ │  "Calc run ini immutable. Tidak dapat dimodifikasi (DEC-018)."        │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [IN_PROGRESS state — JobProgressPanel menggantikan action row]               │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ 🔄 Sedang Berjalan — ECL Bulk Compute JUNI-2026                       │   │
│ │ ████████████░░░░░░░░░░  47%                                            │   │
│ │ Menghitung Stage 2 instruments (1234 dari 2600)                       │   │
│ │ Mulai: 10:30:00 · ETA: 10:35:00 (5 menit lagi)                       │   │
│ │                        [Batalkan]  [Background]                       │   │
│ │ — indikator kecil "Polling mode" jika SSE tidak tersedia —            │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [COMPLETED_WITH_ERRORS — rejection notice bar merah-muted]                  │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ ! Seal ditolak oleh ALCO-01: "Parameter LGD pool perlu diverifikasi." │   │
│ │  Ditolak pada: 14 Jun 10:00 WIB.  Request seal ulang jika sudah fix.  │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ SECTION: Parameter Snapshot                                                  │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ Parameter Snapshot  [🔒 Read-only — Frozen pada 13 Jun 10:30]  [v ▸]  │   │
│ │ (collapsed by default)                                                │   │
│ │ ────────────────────────── expanded state ────────────────────────── │   │
│ │ [Expand All]  [Collapse All]                                          │   │
│ │ ▾ bobot_skenario                                                      │   │
│ │     good:   "0.25000000"                                              │   │
│ │     normal: "0.50000000"                                              │   │
│ │     bad:    "0.25000000"                                              │   │
│ │ ▾ pd_pefindo                                                          │   │
│ │     idAAA: { pd_12m: "0.00010000", pd_lifetime: "..." }  ...         │   │
│ │ ▾ lgd_basel { ... }                                                   │   │
│ │ ▾ impact_mev_pd { good: {...}, normal: {...}, bad: {...} }            │   │
│ │ ▾ lps_coverage { cap_idr: "2000000000.0000", aktif: true }           │   │
│ │ ▾ kurs_jisdor { USD_IDR: "15432.12345678", ... }                     │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ SECTION: Hasil ECL                                                           │
│ [Semua (995)] [Stage 1 (700)] [Stage 2 (250)] [Stage 3 (45)]                │
│ [Error (3)]* [Di-skip FVTPL (5)]                           *jika error > 0  │
│                                                                              │
│ ACTION BAR TAB: [Export ▾ CSV/XLSX]  [Refresh ↺]                            │
│ ┌────────────────┬──────────────┬─────────┬────────────┬──────────┬────────┐│
│ │ Kode Instr ↕   │ Portofolio ↕ │ Stage ↕ │ EAD (IDR)↕ │ECL_FL ↕ │Routing ││
│ ├────────────────┼──────────────┼─────────┼────────────┼──────────┼────────┤│
│ │ OBL-2026-00001 │ PORT-OBLIGASI│[St2 amb]│ 10.000.000 │ 167.625  │STANDARD││
│ │ DEP-2026-00001 │ PORT-DEPOSITO│[St1 grn]│  5.000.000 │  12.500  │[LPS 🔵]││
│ │ RD-2026-00001  │ PORT-REKSA   │[St1 grn]│  8.000.000 │  18.000  │[LT 🟣] ││
│ │ ...            │              │         │            │          │        ││
│ └────────────────┴──────────────┴─────────┴────────────┴──────────┴────────┘│
│ Footer: [← Prev]  Page 1 of ~20  [Next →]  Limit: [50 ▾]                   │
│                                                                              │
│ [Klik baris → navigasi ke /ecl/calc-runs/[id]/instrumen/[instrumenId]]      │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Action buttons per status** (kompilasi dari S2, S3, S7):

| Status               | Action buttons yang tampil                                   |
|----------------------|--------------------------------------------------------------|
| `DRAFT`              | "Start Bulk Compute" (biru) + "Batalkan Draft" (destructive) |
| `IN_PROGRESS`        | `<JobProgressPanel>` embed (gantikan action row)            |
| `COMPLETED`          | "Request Seal" (biru) + "Lihat Audit Trail" (ghost)         |
| `COMPLETED_WITH_ERRORS` | Tab Error aktif — "Request Seal" TIDAK ada                |
| `SEAL_REQUESTED`     | info bar kuning; ALCO/CFO: "Approve Seal" + "Tolak Seal"   |
| `SEALED`             | Semua action hidden/disabled; seal info bar ungu permanen   |
| `CANCELLED`          | "Buat Calc Run Baru →" (link ke create modal)               |

**Annotasi komponen:**
- Header card: `<Card>` shadcn, `<CalcRunStatusBadge>`
- JobProgressPanel: `<JobProgressPanel>` reuse M9, embed inline di header card area
- Parameter snapshot: `<JSONBTreeView>` reuse M9 (PR #82), wrapped dalam `<Accordion>` shadcn
- Tabs hasil: shadcn `<Tabs>` — badge count per tab, tab Error muncul kondisional
- DataTable hasil: `<DataTable>` — kolom ECL, sort, cursor paging, filter, export per-tab
- Routing badge dalam tabel: `<RoutingPathBadge>` reuse M9
- Seal info bar: `<SealInfoBanner>` baru (komponen presentasional)
- Rejection notice bar: shadcn `<Alert variant="destructive">`

---

### SCREEN-M10-03: ECL Drill-Down per Instrumen (`/ecl/calc-runs/[id]/instrumen/[instrumenId]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Calc Runs › CR-JUNI-2026-001 › OBL-2026-00001             │
├─────────────────────────────────────────────────────────────────────────────┤
│ INFO CARD                                                                    │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ OBL-2026-00001  "Obligasi Negara RI 2026"         [STANDARD]  [St2] │   │
│ │ Klasifikasi: AC  │  Portofolio: PORT-OBLIGASI  │  LGD: 45.00000000%   │   │
│ │ EAD: Rp 10.000.000.000,0000  │  ECL Weighted: Rp 167.625.000,0000    │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [Stage 3 only — info bar biru]                                               │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ ℹ Bunga dihitung dari Net Carrying Amount = Rp 9.500.000.000,0000     │   │
│ │   (Gross Rp 10M − ECL Allowance Rp 500K) — PSAK 71 §5.4.1(b)        │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [FVTPL_SKIPPED — info banner abu-abu]                                        │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ ℹ Instrumen ini berstatus FVTPL. ECL tidak dihitung sesuai PSAK 71.   │   │
│ │   Tidak ada breakdown skenario.                                       │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [WARNING CARD — amber, hanya jika warnings[] tidak kosong]                  │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ ⚠ 1 Warning                                                           │   │
│ │  POCI_FLAG_MISMATCH: instrumen diduga POCI berdasarkan rating pada     │   │
│ │  origination, perlu verifikasi manual.                                │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ TABEL BREAKDOWN SKENARIO                                                     │
│ (tidak tampil jika FVTPL_SKIPPED)                                           │
│                                                                              │
│ ┌──────────────────┬──────────────────┬──────────────────┬─────────────────┐│
│ │ Metrik           │ Good (Bobot 25%) │Normal (Bobot 50%)│ Bad  (Bobot 25%)││
│ ├──────────────────┼──────────────────┼──────────────────┼─────────────────┤│
│ │ PD               │ 2.00000000%      │ 3.00000000%      │ 6.00000000%     ││
│ │ LGD              │ 45.00000000%     │ 45.00000000%     │ 45.00000000%    ││
│ │ EAD (IDR)        │ Rp 10.000.000... │ Rp 10.000.000... │ Rp 10.000.000...││
│ │ ECL Skenario     │ Rp 90.000.000... │ Rp 135.000.000.. │ Rp 270.000.000..││
│ │ FL Multiplier    │ 0.85000000       │ 1.00000000       │ 1.20000000      ││
│ │                  │ [Stage 3: "N/A"  │ tooltip: "FL not │ applied, PD=1"] ││
│ │ ECL FL           │ Rp 76.500.000... │ Rp 135.000.000.. │ Rp 324.000.000..││
│ ├──────────────────┼──────────────────┼──────────────────┼─────────────────┤│
│ │ ECL Weighted (IDR) ── colspan 3 ──────────────────── Rp 167.625.000,0000 ││
│ │                  (bold, large, formula: 76.5M×0.25 + 135M×0.50 + 324M×0.25)││
│ └──────────────────┴──────────────────┴──────────────────┴─────────────────┘│
│                                                                              │
│ Formula tooltip (pada baris ECL Weighted): ikon ℹ hover →                   │
│  "ECL_weighted = ECL_FL_Good × 0.25 + ECL_FL_Normal × 0.50                 │
│                + ECL_FL_Bad × 0.25 = Rp 167.625.000,0000"                  │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ [LOOKTHROUGH section — hanya jika routing = LOOKTHROUGH]                     │
│ Look-through Underlying                                                      │
│ ┌──────────────────┬───────────┬──────────────────┬──────────────────┐      │
│ │ Asset Class      │ % NAB     │ EAD (IDR)         │ ECL per Class    │      │
│ ├──────────────────┼───────────┼──────────────────┼──────────────────┤      │
│ │ Obligasi         │ 60%       │ Rp 6.000.000.000 │ Rp xxx.xxx.xxx   │      │
│ │ Deposito         │ 30%       │ Rp 3.000.000.000 │ Rp xxx.xxx.xxx   │      │
│ │ Cash             │ 10%       │ Rp 1.000.000.000 │ Rp xxx.xxx.xxx   │      │
│ ├──────────────────┴───────────┴──────────────────┴──────────────────┤      │
│ │ Total ECL (sama dengan ECL Weighted di atas)       Rp xxx.xxx.xxx   │      │
│ └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│ [LPS section — hanya jika routing = LPS]                                     │
│ LPS Aggregasi                                                                │
│ ┌────────────────────────────────────┬────────────────────────────────┐      │
│ │ Total Eksposur (nasabah + bank)    │ Rp 3.000.000.000,0000          │      │
│ │ Dijamin LPS (cap IDR 2 miliar)     │ Rp 2.000.000.000,0000          │      │
│ │ Excess (ECL basis)                 │ Rp 1.000.000.000,0000          │      │
│ └────────────────────────────────────┴────────────────────────────────┘      │
│ Catatan: "ECL hanya dihitung untuk excess di atas cap LPS IDR 2 miliar       │
│  (DEC-014)."                                                                 │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Info card: `<Card>` shadcn + `<RoutingPathBadge>` (M9) + `<StageBadge>` (M9)
- Tabel breakdown skenario: `<EclResultDrillDownTable>` baru — fixed 3-kolom skenario + baris metrik, summary row colspan
- Formula tooltip: shadcn `<Tooltip>` pada baris ECL Weighted
- FL Multiplier Stage 3: sel "N/A" + `<Tooltip>` penjelasan
- Warning card: shadcn `<Alert variant="warning">` (amber)
- Look-through/LPS section: shadcn `<Card>` presentasional, muncul kondisional per `routing_path`
- Stage 3 info bar: shadcn `<Alert variant="default">` (biru/info)
- FVTPL info banner: shadcn `<Alert>` (abu-abu/muted)

---

### SCREEN-M10-04: Portfolio Summary (`/ecl/calc-runs/[id]/portofolio/[portofolioId]/summary`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Calc Runs › CR-JUNI-2026-001 › PORT-OBLIGASI              │
├─────────────────────────────────────────────────────────────────────────────┤
│ PAGE HEADER                                                                  │
│  Ringkasan Portofolio: PORT-OBLIGASI                                         │
│  Calc Run: CR-JUNI-2026-001 (JUNI-2026)    [Export ▾ CSV/XLSX]              │
├─────────────────────────────────────────────────────────────────────────────┤
│ KPI CARDS (1 row, 5 cards)                                                   │
│ ┌──────────────┐ ┌──────────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐ │
│ │Total ECL     │ │Total         │ │Stage 1   │ │Stage 2   │ │Stage 3     │ │
│ │Weighted      │ │Instrumen     │ │Count     │ │Count     │ │Count       │ │
│ │              │ │              │ │          │ │          │ │            │ │
│ │Rp 1,5 Miliar │ │    100       │ │    70    │ │    25    │ │     5      │ │
│ └──────────────┘ └──────────────┘ └──────────┘ └──────────┘ └────────────┘ │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ CHART ROW (dua chart side-by-side)                                           │
│                                                                              │
│ ┌──────────────────────────────────┐  ┌───────────────────────────────────┐ │
│ │ ECL per Stage (BarChart Recharts)│  │ Distribusi Routing Path (PieChart)│ │
│ │                                  │  │                                   │ │
│ │  800M ┤     ████                 │  │   ┌────────────────────────────┐  │ │
│ │  500M ┤████                      │  │   │  STANDARD   ████  55%      │  │ │
│ │  200M ┤          ████            │  │   │  LOOKTHROUGH ██   10%      │  │ │
│ │       └──────────────────        │  │   │  LPS         █     5%      │  │ │
│ │         S1     S2     S3         │  │   │  POCI_DEFERRED█    2%      │  │ │
│ │ Hover: tooltip nilai IDR penuh   │  │   └────────────────────────────┘  │ │
│ └──────────────────────────────────┘  └───────────────────────────────────┘ │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ PERBANDINGAN DENGAN PRIOR RUN                                                │
│ Prior Run: [CR-MEI-2026-001 (MEI-2026, SEALED) ▾]                           │
│ (dropdown hanya tampilkan COMPLETED/SEALED dari periode ≤ current)          │
│                                                                              │
│ ┌───────────────────┬─────────────────────┬─────────────────────┬─────┬────┐│
│ │ Metrik            │ CR-MEI-2026-001      │ CR-JUNI-2026-001    │Delta│ %  ││
│ ├───────────────────┼─────────────────────┼─────────────────────┼─────┼────┤│
│ │ Total ECL Weighted│ Rp 1.400.000.000     │ Rp 1.500.000.000    │+100M│[merah+7.14%]││
│ │ Stage 1 ECL       │ Rp 450.000.000       │ Rp 500.000.000      │ +50M│[merah+11.1%]││
│ │ Stage 2 ECL       │ Rp 750.000.000       │ Rp 800.000.000      │ +50M│[merah+6.67%]││
│ │ Stage 3 ECL       │ Rp 200.000.000       │ Rp 200.000.000      │    0│[hijau 0.00%]││
│ └───────────────────┴─────────────────────┴─────────────────────┴─────┴────┘│
│                                                                              │
│ Catatan warna: delta positif (ECL naik) = merah; delta negatif = hijau      │
│ (ECL naik = risiko kredit meningkat — bukan "baik")                         │
│                                                                              │
│ [Tidak ada prior run] → info: "Tidak ada calc run sebelumnya untuk           │
│  perbandingan." + dropdown disabled                                          │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ DRILL-DOWN INSTRUMEN                                                         │
│ [Lihat Instrumen ▾] (toggle expand/collapse)                                │
│                                                                              │
│ Filter: [Stage ▾]  [Routing ▾]  [🔍 Cari kode...]                           │
│ ┌────────────────┬──────────┬─────────┬────────────┬──────────┐             │
│ │ Kode Instrumen │ Nama     │ Stage   │ EAD (IDR)  │ ECL (IDR)│             │
│ ├────────────────┼──────────┼─────────┼────────────┼──────────┤             │
│ │ OBL-2026-00001 │ Obl Neg..│[St2 amb]│ 10.000.000 │ 167.625  │             │
│ │ ...            │ ...      │         │            │          │             │
│ └────────────────┴──────────┴─────────┴────────────┴──────────┘             │
│ [Klik baris → /ecl/calc-runs/[id]/instrumen/[instrumenId]]                  │
│ [Export ▾ CSV/XLSX]                                                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- KPI cards: `<PortfolioSummaryKPI>` baru — 5-card row, responsive (3 kolom di mobile)
- BarChart: Recharts `BarChart` via `<EclTrendChart>` baru (reusable untuk S5 + S6 future)
- PieChart: Recharts `PieChart`, legend di kanan
- Comparison table: `<DataTable>` atau `<Table>` shadcn static — delta coloring via `cn()` conditional
- Prior run picker: `<Select>` shadcn, opsi di-fetch dari endpoint dengan filter
- Drill-down DataTable: `<DataTable>` dengan cursor pagination, filter, export

---

### SCREEN-M10-05: Roll-Forward Report (`/ecl/calc-runs/[id]/roll-forward`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Calc Runs › CR-JUNI-2026-001 › Roll Forward               │
├─────────────────────────────────────────────────────────────────────────────┤
│ PAGE HEADER                                                                  │
│  Roll Forward CKPN: MEI-2026 → JUNI-2026                                    │
│  Prior Run: [CR-MEI-2026-001 (MEI-2026, SEALED) ▾]                          │
│  [Export ▾ CSV / XLSX]    [RECONCILED ✓] ← ReconcileBadge                  │
├─────────────────────────────────────────────────────────────────────────────┤
│ WATERFALL TABLE                                                              │
│                                                                              │
│ ┌──────────────────────────────────────┬──────────────────────┬────────────┐│
│ │ Komponen                             │ Jumlah (IDR)         │ Ket        ││
│ ├──────────────────────────────────────┼──────────────────────┼────────────┤│
│ │ Opening — MEI-2026                   │  Rp 1.400.000.000,0  │ [prior run]││
│ │ + Originations                       │+   Rp 200.000.000,0  │            ││
│ │ − Derecognitions                     │−    Rp 50.000.000,0  │            ││
│ │ + Transfer Stage 1→2                 │+    Rp 80.000.000,0  │            ││
│ │ − Transfer Stage 2→1                 │−    Rp 30.000.000,0  │            ││
│ │ + Transfer Stage 2→3                 │+   Rp 100.000.000,0  │            ││
│ │ ± Remeasurements                     │±          Rp 0,0000  │            ││
│ │ [PARTIAL: komponen null]             │                  "—" │ tooltip ℹ  ││
│ ├──────────────────────────────────────┼──────────────────────┼────────────┤│
│ │ CLOSING — JUNI-2026                  │  Rp 1.700.000.000,0  │ (bold)     ││
│ └──────────────────────────────────────┴──────────────────────┴────────────┘│
│                                                                              │
│ RECONCILE BADGE (di bawah tabel, prominent)                                  │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ [RECONCILED ✓] (hijau)                                                │   │
│ │ "Closing matches ECL total calc run JUNI-2026. Selisih: Rp 0."        │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [PARTIAL_PHASE_5_DEFER — amber badge + info bar]                             │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ [PARTIAL — Fase 5 Defer] (amber)                                      │   │
│ │ "Transfer antar stage dan remeasurements akan tersedia setelah Phase 5 │   │
│ │  (GL/jurnal engine) selesai. Laporan ini bersifat partial."            │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [MISMATCH — merah badge + alert]                                             │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ [! MISMATCH] (merah)                                                  │   │
│ │ "Closing roll-forward (Rp 1.700.000.000) ≠ ECL total calc run         │   │
│ │  (Rp 1.700.001.000). Selisih: Rp 1.000. Investigasi diperlukan        │   │
│ │  sebelum seal."                                                       │   │
│ │ [Lihat Audit Trail →]                                                 │   │
│ │ Catatan: Tombol "Request Seal" di halaman detail di-disable            │   │
│ │  selama status MISMATCH.                                              │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│ [Tidak ada prior run]                                                        │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ "Roll-forward tidak tersedia: tidak ada calc run dari periode          │   │
│ │  sebelumnya yang COMPLETED atau SEALED."                               │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ BREAKDOWN PER PORTOFOLIO (accordion, collapsed by default)                   │
│ ▾ PORT-OBLIGASI                                                              │
│     Opening: Rp 900M  →  Closing: Rp 1.050M   [Lihat Detail ↗]             │
│ ▾ PORT-DEPOSITO                                                              │
│     Opening: Rp 300M  →  Closing: Rp 420M     [Lihat Detail ↗]             │
│ ▾ PORT-REKSADANA  (...)                                                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Waterfall table: `<RollForwardWaterfall>` baru — tabel statis + baris "null as dash" + tooltip Phase 5
- Reconcile badge: `<ReconcileBadge>` baru — 3 variant: `RECONCILED` (hijau), `PARTIAL` (amber), `MISMATCH` (merah). Setiap variant: warna + ikon + teks (bukan warna saja, untuk color-blind)
- MISMATCH alert: shadcn `<Alert variant="destructive">`
- Breakdown per portofolio: shadcn `<Accordion>` collapsible
- Prior run picker: shadcn `<Select>` — filter COMPLETED/SEALED, periode ≤ current
- Export: respects filter + reconcile state; MISMATCH → confirm dialog sebelum export

---

### SCREEN-M10-06: Seal Workflow Modals (Inline di `/ecl/calc-runs/[id]`)

Seal workflow tidak menggunakan halaman baru — semua action adalah modal di atas halaman detail.

**Modal 1 — Request Seal** (ROLE-RISK, status = COMPLETED):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Request Seal — CR-JUNI-2026-001                                         [×] │
├─────────────────────────────────────────────────────────────────────────────┤
│ Seal akan mengunci hasil ECL ini secara permanen.                           │
│ Setelah di-seal, tidak ada modifikasi yang diizinkan (DEC-018).             │
│                                                                             │
│ Catatan Request *                                                            │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Tulis catatan permintaan seal di sini (minimal 20 karakter)...          │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│  Sisa karakter minimum: 20 / 20   (counter live — merah jika < 20)         │
│  [inline error jika submit < 20: "Catatan harus minimal 20 karakter        │
│   (saat ini: 9 karakter)."]                                                 │
│                                                                             │
│                                             [Batal]  [Kirim Request →]     │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Modal 2a — Approve Seal: Konfirmasi Pra-MFA** (ROLE-ALCO/CFO, status = SEAL_REQUESTED):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Approve Seal — CR-JUNI-2026-001                                         [×] │
├─────────────────────────────────────────────────────────────────────────────┤
│ Anda akan menyetujui final seal hasil ECL JUNI-2026.                        │
│ Tindakan ini tidak dapat dibalik.                                           │
│                                                                             │
│ Catatan Approval *                                                           │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Tulis catatan persetujuan seal (minimal 20 karakter)...                 │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│  Sisa karakter minimum: 20 / 20                                             │
│                                                                             │
│ [SoD enforced — tombol di-disable jika actor = created_by ATAU              │
│  seal_requested_by, dengan tooltip:                                         │
│  "Anda adalah pembuat calc run ini. SoD tidak memperbolehkan                │
│   self-approval (DEC-017)."]                                                │
│                                                                             │
│                      [Batal]  [Lanjutkan ke Verifikasi MFA →]              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Modal 2b — MFA Step-Up** (setelah konfirmasi pra-MFA, reuse `<MFAStepUpModal>` M9):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Verifikasi Identitas — MFA Step-Up                                      [×] │
├─────────────────────────────────────────────────────────────────────────────┤
│ Step-up MFA diperlukan untuk tindakan sensitif ini (DEC-027).               │
│ Verifikasi identitas Anda untuk melanjutkan.                                │
│                                                                             │
│ [TOTP input / WebAuthn challenge — sesuai method MFA user]                  │
│                                                                             │
│ [inline error jika token invalid: "Kode MFA tidak valid atau sudah         │
│  kadaluarsa. Coba lagi."]                                                   │
│                                                                             │
│                                             [Batal]  [Verifikasi →]        │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Modal 3 — Tolak Seal** (ROLE-ALCO/CFO, status = SEAL_REQUESTED):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Tolak Seal Calc Run?                                                    [×] │
├─────────────────────────────────────────────────────────────────────────────┤
│ Penolakan akan mengembalikan calc run ke status SELESAI.                    │
│ ROLE-RISK dapat mengajukan ulang setelah perbaikan.                         │
│                                                                             │
│ Alasan Penolakan *                                                           │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Tulis alasan penolakan (minimal 30 karakter)...                         │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│  Sisa karakter minimum: 30 / 30                                             │
│                                                                             │
│                                             [Batal]  [Tolak Seal ✗] (merah)│
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Modal 1 + 3: shadcn `<Dialog>` (bukan `<AlertDialog>`) — tidak stack modal
- Modal 2a: shadcn `<Dialog>` dengan tombol yang memicu modal 2b via `open` state flag
- Modal 2b: `<MFAStepUpModal>` reuse M9 — dipanggil dari dalam 2a, bukan stacking dialog baru
- Implementasi anti-stacking: modal 2a tutup diri sebelum modal 2b buka (state swap, bukan push)
- Character counter: live Zod validation via React Hook Form `watch()`
- SoD UI check: banding `created_by` + `seal_requested_by` dengan `currentUser.id` di client sebelum render tombol approve; tetap enforce server-side
- `<SealWorkflowPanel>`: wrapper komponen baru yang mengorkestrasi state machine 3 modal ini

---

## 3. Komponen Baru yang Perlu Dibuat

| Komponen | Lokasi | Deskripsi |
|---|---|---|
| `<CalcRunStatusBadge>` | `web/components/blips/CalcRunStatusBadge.tsx` | Status badge dengan warna + ikon + teks. Extend pola `<StageBadge>`. |
| `<EclResultDrillDownTable>` | `web/components/blips/EclResultDrillDownTable.tsx` | Tabel 3-skenario × 6-metrik matrix. Summary row ECL Weighted colspan. FL N/A untuk Stage 3. |
| `<PortfolioSummaryKPI>` | `web/components/blips/PortfolioSummaryKPI.tsx` | 5-card KPI cluster untuk total ECL + stage breakdown. |
| `<EclTrendChart>` | `web/components/blips/EclTrendChart.tsx` | Recharts `BarChart` ECL per stage. Reusable untuk portfolio summary + future perbandingan. |
| `<RollForwardWaterfall>` | `web/components/blips/RollForwardWaterfall.tsx` | Waterfall table dengan null-as-dash + tooltip Phase 5 defer. |
| `<ReconcileBadge>` | `web/components/blips/ReconcileBadge.tsx` | 3 variant: RECONCILED/PARTIAL/MISMATCH. Warna + ikon + teks (bukan warna saja). |
| `<SealWorkflowPanel>` | `web/components/blips/SealWorkflowPanel.tsx` | State machine orchestrator untuk 3 seal modals (request/approve/reject). |
| `<SignatureHashBadge>` | `web/components/blips/SignatureHashBadge.tsx` | Display 16-char truncated hash + tombol salin. |
| `<SealInfoBanner>` | `web/components/blips/SealInfoBanner.tsx` | Full-width ungu banner untuk status SEALED di header. |

**Komponen M9 yang direuse (tidak perlu dibuat ulang):**

| Komponen | Sumber | Dipakai di screen |
|---|---|---|
| `<StageBadge>` | M9 PR #82 | S2 tabel hasil, S3 info card |
| `<RoutingPathBadge>` | M9 PR #82 | S2 tabel hasil, S3 info card |
| `<MFAStepUpModal>` | M9 PR #82 | S6 modal approve seal |
| `<JobProgressPanel>` | Phase 3 | S2 status IN_PROGRESS |
| `<JSONBTreeView>` | M9 PR #82 | S2 parameter snapshot section |
| `<DataTable>` | Phase 3 | Semua screen dengan list |
| `notify` (lib) | Phase 3 | Semua form submit |

---

## 4. Interaction Spec per Story

### S1 — Create Calc Run

**Happy path:**
1. User klik "Buat Calc Run Baru" → `<Dialog>` terbuka.
2. Periode picker di-fetch `GET /api/v1/master/periode?filter[status][]=OPEN&filter[status][]=SOFT_CLOSED` (exclude `HARD_CLOSED`).
3. User pilih periode + evaluation date (default hari ini) → klik "Buat".
4. Tombol spinner, `Idempotency-Key` UUID baru di-generate.
5. `POST /api/v1/ecl/calc-runs` → 201 → toast sukses: `"Calc run untuk periode {periode} berhasil dibuat ({id}). Status: DRAFT."` + link "Lihat detail →".
6. Modal tutup → redirect `/ecl/calc-runs/{id}`.

**Failure modes:**
- Periode SEALED sudah ada → API 422 `CALC_RUN_PERIODE_ALREADY_SEALED` → toast error persistent; modal tetap terbuka; user dapat pilih periode lain.
- Network error → toast error + traceId; tombol kembali aktif; tidak terjadi double-submit karena Idempotency-Key.
- Validation (periode tidak dipilih) → inline error `aria-describedby` pada field.

**Empty state list:** ilustrasi + teks "Belum ada calc run. Buat yang pertama untuk memulai." + button "Buat Calc Run Baru".

**Loading state list:** skeleton rows (5 baris placeholder).

**ROLE-AUDIT:** tombol "Buat Calc Run Baru" tidak di-render (permission guard `calc_run.create`).

---

### S2 — Start + Monitor Progress

**Happy path:**
1. `<DestructiveActionDialog>` confirm (judul, deskripsi, tombol "Mulai" biru).
2. `POST /api/v1/ecl/calc-runs/{id}/start` → 202 `{jobId, statusUrl, streamUrl}`.
3. Status badge berubah ke "Sedang Berjalan" (spinner).
4. `<JobProgressPanel>` embed di area action row — subscribe SSE `streamUrl`.
5. Progress update real-time via SSE events `progress` dan `completed`.
6. Completion: status badge → "SELESAI", progress panel → ringkasan, toast sukses + link.
7. `COMPLETED_WITH_ERRORS`: toast warning + link tab Error.

**SSE fallback:** `EventSource.onerror` → polling `GET /jobs/{jobId}` tiap 2 detik; indikator kecil "Polling mode" (tidak blocking UX).

**Cancel flow:**
- User klik "Batalkan" dalam `<JobProgressPanel>`.
- `<DestructiveActionDialog>` dengan textarea `cancel_reason` (min 30 char, Zod validation).
- Submit → `POST /api/v1/ecl/calc-runs/{id}/cancel`.
- Toast sukses + status badge "DIBATALKAN".

**Failure:** Start gagal karena parameter belum APPROVED → toast error `ECL_PARAM_NOT_FOUND`; status tetap DRAFT.

---

### S3 — Calc Run Detail + Parameter Snapshot

**Parameter snapshot:**
- Default collapsed (accordion closed).
- "Expand All" klik → semua node JSONBTreeView terbuka.
- Read-only — tidak ada input, tidak ada tombol edit.
- Badge "Read-only — Frozen" di header section mencegah ambiguitas.

**Tabs hasil:**
- Tab "Error" hanya di-render jika `error_count > 0` (kondisional).
- Badge count per tab terupdate setelah status berubah ke COMPLETED.
- Export per tab: file name mencantumkan stage, contoh `ecl-results-stage2-CR-JUNI-2026-001-20260613.csv`.

**SEALED state:**
- Semua tombol action tidak di-render (bukan hanya disabled) untuk SEALED.
- `<SealInfoBanner>` permanent di atas parameter snapshot section.
- DataTable hasil tetap dapat dilihat dan diekspor.

---

### S4 — Drill-Down per Instrumen

**Navigation:** klik baris DataTable hasil → navigasi push state (bukan modal) ke `/ecl/calc-runs/[id]/instrumen/[instrumenId]`. Breadcrumb memungkinkan kembali.

**Tabel skenario:**
- Baris "FL Multiplier" untuk Stage 3: tampilkan teks "N/A" di setiap sel skenario + tooltip shadcn `<Tooltip>`: "FL multiplier tidak diaplikasikan untuk Stage 3 (PD sudah fixed = 1.0)".
- Baris "ECL Weighted": ikon ℹ di header baris → tooltip formula lengkap.
- Semua nilai IDR: format `Rp X.XXX.XXX,XXXX` (titik ribuan, koma desimal, 4 desimal).
- Semua nilai rate: 8 desimal dengan `%` suffix.

**FVTPL_SKIPPED:** tabel skenario tidak di-render; info banner muncul.

**Look-through/LPS:** section muncul kondisional berdasarkan `routing_path` dari response API.

---

### S5 — Portfolio Summary

**Chart interaktivitas:**
- BarChart: hover tooltip nilai IDR penuh; klik bar belum ada aksi (future drill-down).
- PieChart: hover tooltip nilai + persen; klik slice belum ada aksi.
- Chart color: gunakan token warna stage (Stage 1 = hijau, Stage 2 = amber, Stage 3 = merah) yang konsisten dengan badge.

**Comparison delta coloring:**
- Delta positif (ECL naik) = teks merah — ECL naik berarti risiko kredit meningkat.
- Delta negatif (ECL turun) = teks hijau.
- Delta nol = teks muted, tidak ada warna.
- Color tidak satu-satunya sinyal: delta value + tanda + % selalu tampil.

**Empty comparison state:** dropdown prior run disable + info teks "Tidak ada calc run sebelumnya."

---

### S6 — Roll-Forward Report

**Waterfall table behavior:**
- Baris null (PARTIAL_PHASE_5_DEFER): tampilkan "—" pada kolom Jumlah + tooltip "Data belum tersedia (Phase 5)".
- Baris Closing: bold, row di-highlight ringan (background muted).
- MISMATCH: tombol "Request Seal" di halaman detail (S3) di-disable → `disabled` attribute + tooltip "Roll-forward mismatch. Selesaikan investigasi sebelum seal."

**Export MISMATCH:** dialog konfirmasi tambahan sebelum export XLSX jika status MISMATCH: "Laporan ini memiliki mismatch. Ekspor tetap tersedia untuk investigasi. Lanjutkan?"

---

### S7 — Seal Workflow

**State machine modal (anti-stacking):**
1. "Request Seal" → Modal 1 buka.
2. "Approve Seal" → Modal 2a buka → user isi comment → "Lanjutkan ke MFA" → Modal 2a **tutup** → Modal 2b (`<MFAStepUpModal>`) **buka**. Tidak ada dua modal bersamaan.
3. "Tolak Seal" → Modal 3 (destructive) buka.

**SoD client-side check:**
- Jika `currentUser.id === calcRun.created_by` ATAU `currentUser.id === calcRun.seal_requested_by`, tombol "Approve Seal" diganti dengan disabled `<Button>` + `<Tooltip>` "Anda adalah pembuat calc run ini. SoD tidak memperbolehkan self-approval (DEC-017)."
- Client check adalah UX safeguard — server-side SoD enforcement tetap mandatory.

**MFA failure:** inline error dalam `<MFAStepUpModal>` tanpa menutup modal — user coba ulang tanpa kehilangan context.

**Post-seal:** halaman di-refresh otomatis (atau data di-revalidate via TanStack Query `invalidateQueries`) → `<SealInfoBanner>` muncul, semua action buttons tidak di-render.

---

## 5. Accessibility Checklist

| Requirement | Implementasi |
|---|---|
| WCAG 2.1 AA contrast semua teks dan badge | Verifikasi token warna dengan contrast ratio ≥ 4.5:1 (normal text) / 3:1 (large text) |
| Color bukan satu-satunya sinyal | `<CalcRunStatusBadge>`: warna + ikon + teks. `<ReconcileBadge>`: warna + ikon + teks. Delta comparison: warna + tanda (+/−) + nilai |
| Screen reader progress update | `<JobProgressPanel>` gunakan `role="status"` + `aria-live="polite"` pada progress text dan current step |
| Keyboard navigation waterfall table | Seluruh baris `<RollForwardWaterfall>` focusable; tooltip via keyboard (`Tab` + `Enter`) |
| Focus management modal chain | Setelah Modal 2a tutup dan 2b buka: focus pindah ke elemen pertama Modal 2b. Setelah modal tutup (semua scenario): focus kembali ke tombol yang membuka modal |
| Form error association | Semua `<Textarea>` dengan inline error: `aria-describedby={fieldId + "-error"}`, `aria-invalid="true"` |
| Tab order halaman detail | Breadcrumb → Header card → Action row → Parameter snapshot accordion → Tabs → DataTable headers → DataTable rows |
| Stage badge non-visual | StageBadge: `aria-label="Stage 2 — SICR"` tidak hanya `aria-label="Stage 2"` |
| Color-blind safe routing badge | RoutingPathBadge (M9): sudah ada teks + ikon — verifikasi ulang untuk 4 varian warna M10 |

---

## 6. Bahasa Indonesia Copy Guide

### Terminologi layar

| Term teknis | Label UI Bahasa Indonesia | Catatan |
|---|---|---|
| Calc Run | Calc Run | Pertahankan sebagai istilah teknis; subtitle "Perhitungan ECL" di page header |
| Periode Buku | Periode Buku | Tidak diterjemahkan |
| Roll-forward | Roll Forward | Pertahankan; subtitle "Rekap Perubahan ECL" opsional |
| Seal / Sealed | Segel / Tersegel | "Tersegel" di badge; "Segel" di tombol aksi |
| Seal Request | Request Seal | Pertahankan hibrid; judul modal "Request Seal" |
| Signature hash | Tanda Tangan Digital | Label pada seal info section |
| Reconcile | Rekonsiliasi | `<ReconcileBadge>`: "REKONSILIASI OK" / "PARTIAL — Fase 5 Defer" / "MISMATCH" |
| Stage | Stage | Tidak diterjemahkan (PSAK 71 menggunakan Stage) |
| Routing path | Jalur Perhitungan | Label di kolom tabel; badge tetap gunakan kode (STANDARD, LPS, dll.) |
| Instrumen | Instrumen | Tidak diterjemahkan |

### Status labels

| Status backend | Label UI |
|---|---|
| `DRAFT` | DRAFT |
| `IN_PROGRESS` | Sedang Berjalan |
| `COMPLETED` | SELESAI |
| `COMPLETED_WITH_ERRORS` | SELESAI dengan Error |
| `SEAL_REQUESTED` | Menunggu Segel |
| `SEALED` | TERSEGEL |
| `CANCELLED` | DIBATALKAN |

### Format angka IDR

- Pemisah ribuan: titik (`.`)
- Pemisah desimal: koma (`,`)
- 4 desimal untuk IDR: `Rp 1.234.567.890,0000`
- 8 desimal untuk rate/PD/LGD: `2,00000000%`
- Negatif: `−Rp 50.000.000,0000` (gunakan minus en-dash, bukan hyphen)

### Pesan toast (contoh)

| Skenario | Pesan toast |
|---|---|
| Create sukses | "Calc run untuk periode JULI-2026 berhasil dibuat (CR-2026-07-002). Status: DRAFT." + "Lihat detail →" |
| Start sukses | "ECL Calc Run JUNI-2026 selesai. 995 instrumen dihitung. Siap untuk di-segel." |
| COMPLETED_WITH_ERRORS | "Calc run selesai dengan 3 error. Perbaiki data instrumen sebelum segel." |
| Cancel sukses | "Calc run CR-JUNI-2026-001 berhasil dibatalkan. 400 instrumen partial tetap tersimpan." |
| Request seal sukses | "Request segel CR-JUNI-2026-001 dikirim. Menunggu persetujuan ALCO." |
| Approve seal sukses | "Calc run CR-JUNI-2026-001 berhasil di-segel. Hasil ECL JUNI-2026 final dan immutable." |
| Reject seal sukses | "Segel CR-JUNI-2026-001 berhasil ditolak." |
| Create gagal SEALED | "Periode MEI-2026 sudah memiliki calc run yang di-segel (CR-2026-05-001). Override memerlukan persetujuan ALCO — fitur belum tersedia." |
| Start gagal param | "Start gagal: Parameter ECL (bobot skenario) untuk JUNI-2026 belum disetujui ALCO. Hubungi ROLE-ALCO." |

---

## 7. Hand-off untuk Frontend Engineer Next.js

### File structure proposal

```
web/src/app/ecl/
├── calc-runs/
│   ├── page.tsx                          — S1 list + create modal
│   ├── [id]/
│   │   ├── page.tsx                      — S2+S3+S7 detail (composed)
│   │   ├── instrumen/
│   │   │   └── [instrumenId]/
│   │   │       └── page.tsx             — S4 drill-down
│   │   ├── portofolio/
│   │   │   └── [portofolioId]/
│   │   │       └── summary/
│   │   │           └── page.tsx         — S5 portfolio summary
│   │   └── roll-forward/
│   │       └── page.tsx                 — S6 roll-forward
│   └── _components/                     — private components (route-scoped)
│       ├── CreateCalcRunModal.tsx
│       ├── CalcRunDetailHeader.tsx
│       ├── ParameterSnapshotSection.tsx
│       ├── ResultsTabSection.tsx
│       └── SealWorkflowPanel.tsx        — orchestrates seal modals
├── jobs/
│   └── [jobId]/
│       └── page.tsx                     — reuse M9 pattern
```

### Zustand store

```
web/src/stores/calcRun.store.ts
  state: {
    activeCalcRun: CalcRunDetail | null
    listFilters: CalcRunListFilters
    activeJobId: string | null
    sealModalState: 'closed' | 'request' | 'approve-confirm' | 'approve-mfa' | 'reject'
  }
```

### API client wrappers

```
web/src/lib/api/eclCore.api.ts      — calc run CRUD + start/cancel + seal
web/src/lib/api/calcRun.api.ts      — results, drill-down, portfolio summary, roll-forward
```

### Shadcn/ui yang dibutuhkan

- `Card`, `CardHeader`, `CardContent`
- `Dialog` (bukan `AlertDialog` untuk seal modals — perlu custom content)
- `AlertDialog` (`DestructiveActionDialog` cancel + reject seal)
- `Sheet` (opsional untuk filter di mobile)
- `Tabs`, `TabsContent`
- `Accordion`, `AccordionItem`
- `Alert`, `AlertTitle`, `AlertDescription`
- `Tooltip`, `TooltipContent`, `TooltipTrigger`
- `Select`, `SelectContent`, `SelectItem`
- `Textarea`
- `Badge`

### Recharts

- `BarChart` + `Bar` + `XAxis` + `YAxis` + `Tooltip` + `ResponsiveContainer` — portfolio ECL per stage
- `PieChart` + `Pie` + `Cell` + `Legend` + `Tooltip` — distribusi routing path

### Validation rules (Zod)

```typescript
// Create calc run
const createCalcRunSchema = z.object({
  periode_id: z.string().min(1, "Periode wajib dipilih"),
  evaluation_date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal tidak valid"),
})

// Request seal
const requestSealSchema = z.object({
  comment: z.string().min(20, "Catatan harus minimal 20 karakter"),
})

// Approve seal (pre-MFA step)
const approveSealSchema = z.object({
  comment: z.string().min(20, "Catatan harus minimal 20 karakter"),
})

// Reject seal
const rejectSealSchema = z.object({
  reject_reason: z.string().min(30, "Alasan penolakan harus minimal 30 karakter"),
})

// Cancel calc run
const cancelCalcRunSchema = z.object({
  cancel_reason: z.string().min(30, "Alasan pembatalan harus minimal 30 karakter"),
})
```

### Checklist handoff

- [ ] Semua komponen baru di tabel Section 3 perlu dibuat
- [ ] `<CalcRunStatusBadge>`: extend dari `<StageBadge>` M9; 7 status variant
- [ ] `<SealWorkflowPanel>`: implementasikan state machine anti-stacking modal (modal 2a close → 2b open)
- [ ] `<JobProgressPanel>`: embed inline di header card, bukan sebagai overlay/sheet
- [ ] SoD UI check di `<SealWorkflowPanel>`: banding `created_by` + `seal_requested_by` dengan `useSession().user.id`
- [ ] `<ReconcileBadge>`: 3 variant wajib warna + ikon + teks (WCAG color-blind)
- [ ] `<EclResultDrillDownTable>`: FL Multiplier Stage 3 = "N/A" + tooltip; summary row colspan 3
- [ ] Roll-forward waterfall: null components tampil "—" bukan "Rp 0" — backend field akan null, bukan 0
- [ ] Export MISMATCH: confirm dialog tambahan sebelum download
- [ ] SEALED state: action buttons tidak di-render (bukan `disabled`) — gunakan kondisional render
- [ ] Deep-link URL state: filter + sort list di URL params (nuqs atau Next.js searchParams)
- [ ] `aria-live="polite"` pada `<JobProgressPanel>` progress text
- [ ] Focus management: test manual modal chain (S7 approve flow: 2a → 2b → konfirmasi → kembali ke halaman)
- [ ] IDR formatter: buat `lib/formatters/idr.ts` dengan fungsi `formatIDR(value, decimals)` — titik ribuan, koma desimal

---

*Dokumen ini adalah satu-satunya source of truth UX untuk P4-M10. Perubahan pada AC atau OQ resolution yang mempengaruhi layout/flow harus di-update di sini sebelum koding dimulai.*
