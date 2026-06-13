# P4-M9 — EIR + Staging UI Design Specification

**Story Set**: P4-M9  
**Modul**: APP-C — ECL Engine  
**Desainer**: uiux-designer  
**Tanggal**: 2026-06-13  
**Status**: READY FOR HANDOFF  
**Linked Stories**: `docs/stories/phase-4/M9-eir-staging-ui.md`

---

## 1. Information Architecture

### Sitemap M9

```
ECL (side nav group, collapsible)
├── Staging
│   ├── [tidak ada index — instrumen navigated dari master/instrumen/[id]]
│   ├── /ecl/staging/instrumen/[id]        — Detail staging per instrumen (M9-001)
│   ├── /ecl/staging/override              — Antrian override pending (M9-002)
│   ├── /ecl/staging/override/new          — Form proposal override (M9-002)
│   └── /ecl/staging/override/[id]         — Detail + aksi override (M9-002)
│   └── /ecl/staging/dpd                   — List + form entry DPD (M9-003)
└── EIR
    ├── [tidak ada index — instrumen navigated dari master/instrumen/[id]]
    ├── /ecl/eir/instrumen/[id]             — EIR + amortisasi schedule (M9-004)
    ├── /ecl/eir/amendments/queue           — Antrian proposal amandemen (M9-005)
    ├── /ecl/eir/amendments/new             — Form propose amandemen (M9-005)
    ├── /ecl/eir/amendments/[id]            — Detail + aksi amandemen (M9-005)
    ├── /ecl/eir/drift-reports              — List drift report (M9-006)
    └── /ecl/eir/drift-reports/[id]         — Detail drift report (M9-006)
```

### Navigasi side nav

Side nav item "ECL" collapsible dengan dua sub-group:

```
ECL
  ▾ Staging
      Antrian Override
      Record DPD
  ▾ EIR
      Antrian Amandemen
      Drift Report
```

Halaman per-instrumen (`/ecl/staging/instrumen/[id]` dan `/ecl/eir/instrumen/[id]`) diakses melalui link dari halaman detail instrumen di Master Data — tidak muncul di nav karena bersifat kontekstual.

---

## 2. Wireframes — 6 Screens

### SCREEN-M9-01: Staging Dashboard per Instrumen (`/ecl/staging/instrumen/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Staging › OBL-2026-00001                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│ INSTRUMEN HEADER CARD                                                        │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ OBL-2026-00001   "Obligasi Negara RI 2026"          [AC] [FVOCI debt]   │ │
│ │ Counterparty: PT XYZ  │  Jatuh Tempo: 2031-06-01   │  EIR: 4.25000000% │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ STAGE BANNER — color-coded full-width                                       │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ [CIRCLE-ICON]  Stage 1 — Berkinerja Baik                    [hijau]     │ │
│ │                Evaluasi terakhir: 2026-10-01                             │ │
│ │                Cure penuh dikonfirmasi 3 periode berturut-turut          │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ [Stage 3 only] ALERT CARD                                                   │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ [WARN] Instrumen ini dalam status default (Stage 3). PD = 1.0 digunakan │ │
│ │ untuk ECL. Bunga dihitung dari Net Carrying Amount (Gross − ECL).        │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ SICR EVIDENCE CARD (tampil hanya jika ada evidence pada stage aktif)        │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Pemicu SICR Terakhir                                                     │ │
│ │ Tipe: [RATING_DOWNGRADE pill]  Downgrade 2 notch: idBBB → idBB          │ │
│ │ Tanggal: 2026-03-15            Rating Sebelum: idBBB  Rating Baru: idBB │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ ACTION BAR                                                                   │
│ [Evaluasi Staging] (ROLE-RISK only)    [Request Override] (ROLE-RISK only)  │
│   spinner inline saat job berjalan       → navigasi ke /ecl/staging/override/new │
│                                                                              │
│ RIWAYAT TRANSISI STAGING                                                    │
│ Filter bar: [🔍 Cari...]  [Trigger ▾]  [Stage ▾]  [Tanggal Dari—Sampai]    │
│ Filter chips: [RATING_DOWNGRADE ×]  [Clear semua]                            │
│                                                                              │
│ ┌─────────┬───────────┬───────────┬────────────────┬──────────┬───────────┐ │
│ │ Tanggal ↕│ Sblm ↕   │ Sesudah ↕ │ Trigger        │ Evidence │ Disetujui│ │
│ ├─────────┼───────────┼───────────┼────────────────┼──────────┼───────────┤ │
│ │2026-10-01│ [St2 amb]│ [St1 grn] │ CURE_FULL      │ —        │ ALCO-01  │ │
│ │2026-07-01│ [St3 red]│ [St2 amb] │ CURE_PARTIAL   │ —        │ ALCO-01  │ │
│ │2026-04-01│ [St2 amb]│ [St3 red] │ DPD_THRESHOLD  │ DPD: 91  │ ALCO-02  │ │
│ │2026-03-15│ [St1 grn]│ [St2 amb] │ RATING_DOWNGRADE│[expand ▾]│ ALCO-01 │ │
│ │2026-01-01│ —        │ [St1 grn] │ ORIGINATION    │ —        │ —        │ │
│ └─────────┴───────────┴───────────┴────────────────┴──────────┴───────────┘ │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]  Limit: [50 ▾]                   │
│                                                                              │
│ Action bar tabel: [Export CSV] [Export XLSX] [Refresh ↺]  "Diperbarui: ..." │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Stage banner: `<StageBadge>` baru, full-width, warna sesuai stage
- Alert Stage 3: shadcn `<Alert variant="destructive">` dengan icon ShieldAlert
- SICR Evidence: `<SicrEvidenceCard>` baru
- DataTable: `<DataTable>` existing — sort, cursor paging, filter chip, export
- Evidence cell: collapsible expand dengan `<JSONBTreeView>` baru
- Stage badge di sel tabel: `<StageBadge>` size="sm"

---

### SCREEN-M9-02: Override Request Form + Queue

#### Sub-screen A: Antrian Override (`/ecl/staging/override`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Staging › Antrian Override                                 │
│                                      [+ Request Override] (ROLE-RISK only)  │
├─────────────────────────────────────────────────────────────────────────────┤
│ Filter bar: [🔍 Cari instrumen...]  [Status ▾]  [Target Stage ▾]  [Tgl ▾]  │
│ Filter chips: [Menunggu ×]  [Clear semua]                                    │
│                                                                              │
│ ┌──────────┬───────────┬────────────┬────────────┬──────────┬──────┬──────┐ │
│ │ ID Override│ Instrumen │Stage Saat  │Target Stage│ Proposer │Status│ Aksi │ │
│ ├──────────┼───────────┼────────────┼────────────┼──────────┼──────┼──────┤ │
│ │OVR-001   │DEP-2026-01│[St1 grn]   │[St2 amb]   │RISK-01   │[PRv] │[⋮]  │ │
│ │OVR-002   │OBL-2026-05│[St2 amb]   │[St3 red]   │RISK-02   │[PAp] │[⋮]  │ │
│ │OVR-003   │DEP-2026-10│[St1 grn]   │[St2 amb]   │RISK-01   │[PRv] │[⋮]  │ │
│ └──────────┴───────────┴────────────┴────────────┴──────────┴──────┴──────┘ │
│                                                                              │
│ [PRv]=PENDING_REVIEW badge kuning  [PAp]=PENDING_APPROVAL badge oranye      │
│                                                                              │
│ Dropdown aksi [⋮]: "Lihat Detail" | "Approve" (jika berhak) | "Tolak"      │
│   "Approve" di-disable dengan tooltip SoD jika user = proposer              │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]   [Export CSV] [Refresh ↺]        │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Sub-screen B: Form Proposal Override Baru (`/ecl/staging/override/new`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Staging › Antrian Override › Request Override Baru        │
├──────────────────────────────────┬──────────────────────────────────────────┤
│  FORM (kiri, 60%)                │  PREVIEW (kanan, 40%, sticky)           │
│                                  │                                          │
│  Instrumen *                     │  Ringkasan Proposal                      │
│  [ComboBox search instrumen]     │  ┌────────────────────────────────────┐  │
│  OBL-2026-00001 — Obligasi ...   │  │ Instrumen: OBL-2026-00001          │  │
│                                  │  │ Stage Saat Ini: Stage 1 (Hijau)    │  │
│  Stage Saat Ini (read-only)      │  │ Target Stage:  Stage 2 (Amber)     │  │
│  [Stage 1 — Berkinerja Baik]     │  │ Proposer: RISK-01                  │  │
│                                  │  │ Expiry:   DESEMBER 2026            │  │
│  Target Stage *                  │  │                                    │  │
│  [Select: Stage 2 / Stage 3]     │  │ Workflow: 6-eyes                   │  │
│                                  │  │ Step 1: RISK (Anda) — Proposer     │  │
│  Alasan Justifikasi *            │  │ Step 2: ALCO — Approver 1          │  │
│  [Textarea min 20 karakter]      │  │ Step 3: ALCO/KOMITE — Approver 2   │  │
│  "14/20 karakter" [merah jika<20]│  └────────────────────────────────────┘  │
│  error inline: aria-describedby  │                                          │
│                                  │  SoD Notice                              │
│  Expiry Periode *                │  [INFO] Anda tidak dapat menjadi        │
│  [MonthPicker — hanya masa depan]│  approver untuk proposal ini (SoD).     │
│  error: "MEI-2026 sudah lewat"   │                                          │
│                                  │                                          │
│  ─────────────────────────────── │                                          │
│  [Batal]          [Submit Proposal]│                                        │
│              spinner inline saat submit                                      │
└──────────────────────────────────┴──────────────────────────────────────────┘
```

#### Sub-screen C: Detail + Aksi Override (`/ecl/staging/override/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Staging › Antrian Override › OVR-001                      │
├───────────────────────────────────┬─────────────────────────────────────────┤
│  HERO CARD (kiri, 65%)            │  WORKFLOW PANEL (kanan, 35%)            │
│  ┌─────────────────────────────┐  │                                         │
│  │ Override OVR-001             │  │  <MakerReviewerApproverPanel>           │
│  │ [PENDING_REVIEW badge]       │  │  6-eyes mode (is6Eyes=true)             │
│  │                              │  │                                         │
│  │ Instrumen: DEP-2026-00001   │  │  Step 1: Proposer — done                │
│  │ Stage: [St1] → [St2]         │  │   RISK-01, 2026-06-10 14:22             │
│  │ Expiry: DESEMBER 2026        │  │   "Counterparty menunjukkan..."         │
│  │                              │  │                                         │
│  │ Alasan:                      │  │  Step 2: ALCO Approver 1 — ACTIVE      │
│  │ "Counterparty menunjukkan    │  │   [ApprovalWithSignature]               │
│  │ tanda-tanda kesulitan        │  │   requireStepUpMfa=true                 │
│  │ keuangan meski rating        │  │   sodBlocked=[if RISK-01 login]         │
│  │ belum turun resmi."          │  │                                         │
│  │                              │  │  Step 3: ALCO/KOMITE Approver 2 —      │
│  │ Diajukan oleh: RISK-01       │  │  Menunggu step 2 selesai                │
│  │ Tanggal: 2026-06-10          │  │                                         │
│  └─────────────────────────────┘  │  [Tolak Proposal] (merah, secondary)    │
│                                   │  → <DestructiveActionDialog>             │
└───────────────────────────────────┴─────────────────────────────────────────┘
```

**Annotasi komponen:**
- Form: React Hook Form + Zod, instrumen ComboBox dengan autocomplete
- Preview panel: sticky, updated real-time saat field berubah (controlled)
- `<MakerReviewerApproverPanel>` existing — mode 6-eyes dengan `onApprove2`
- `<ApprovalWithSignature>` existing — `requireStepUpMfa=true` untuk ALCO steps
- `<DestructiveActionDialog>` existing — textarea reason wajib min 20 char

---

### SCREEN-M9-03: DPD Record Entry (`/ecl/staging/dpd`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › Staging › Record Hari Tunggakan (DPD)                     │
│                                            [+ Tambah Record DPD] (ROLE-AKUN)│
├─────────────────────────────────────────────────────────────────────────────┤
│ Filter bar: [🔍 Cari instrumen...]  [Periode ▾]  [Stage ▾]  [Tgl Entry ▾]  │
│                                                                              │
│ ┌───────────┬────────┬─────────┬────────────┬────────────┬────────┬───────┐ │
│ │ Instrumen │ Kode   │ Periode │ DPD (hari) │ Stage Saat │ Oleh   │ Aksi  │ │
│ ├───────────┼────────┼─────────┼────────────┼────────────┼────────┼───────┤ │
│ │Obligasi.. │OBL-001 │JUN-2026 │         35 │ [St2 amb]  │AKUN-01 │ [✏ 🗑]│ │
│ │Deposito.. │DEP-010 │JUN-2026 │         15 │ [St1 grn]  │AKUN-02 │ [✏ 🗑]│ │
│ └───────────┴────────┴─────────┴────────────┴────────────┴────────┴───────┘ │
│                                                                              │
│ [🗑] → <DestructiveActionDialog> "Hapus record DPD ini? Re-staging otomatis."│
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]   [Export CSV] [Refresh ↺]        │
├─────────────────────────────────────────────────────────────────────────────┤
│ DRAWER (buka saat klik [+ Tambah] atau [✏ Edit]) — shadcn Sheet kanan       │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ Tambah / Edit Record DPD                                       [×]     │  │
│ │                                                                        │  │
│ │ Instrumen *  [ComboBox — search by kode/nama]                          │  │
│ │ Periode *    [Select — hanya periode OPEN]                             │  │
│ │ DPD (Hari) * [Input number, min=0]                                     │  │
│ │              error: "Nilai DPD tidak boleh negatif."                   │  │
│ │ Keterangan   [Textarea opsional, max 200 char]                         │  │
│ │                                                                        │  │
│ │ [Batal]                    [Simpan]  ← spinner saat submit             │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│ JOB PROGRESS (muncul setelah simpan berhasil, dalam halaman)                │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ [spinning icon]  Mengevaluasi staging OBL-2026-00005...   [Background] │  │
│ │ ████░░░░░░░░░░░  30%                                                   │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Sheet (Drawer): shadcn `<Sheet side="right">` — tidak menumpuk modal
- `<JobProgressPanel>` existing — embedded setelah submit sukses, bukan modal
- Toast saat job selesai: "Stage OBL-2026-00005 berubah: Stage 1 → Stage 2 (DPD 35 hari)"
- Edit: load existing data ke drawer form, submit ke `PUT /dpd-records/{id}`

---

### SCREEN-M9-04: EIR + Amortisasi Schedule (`/ecl/eir/instrumen/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › EIR › OBL-2026-00001                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ INSTRUMEN HEADER CARD (sama dengan SCREEN-M9-01)                             │
├──────────────────────────────────────────┬──────────────────────────────────┤
│ SECTION A: EIR SUMMARY (kiri, 65%)       │ SECTION B: SOLVER PANEL (kanan) │
│                                          │                                  │
│ ┌──────────────────────────────────────┐ │ <NewtonRaphsonSolverPanel>       │
│ │ EIR Aktif: 4.25000000%  [ver badge] │ │ ┌──────────────────────────────┐ │
│ │ Dihitung: 2026-01-15    Day: ACT/365│ │ │ Status: [Konvergen] ✓        │ │
│ │ Klasifikasi: AC         Kupon: 4.00%│ │ │ Iterasi: 23 dari maks 100    │ │
│ │ Nominal: Rp 10.000.000.000,0000     │ │ │ Residual: 3.45e-12           │ │
│ └──────────────────────────────────────┘ │ │ Presisi: HALF_EVEN, 8 des.  │ │
│                                          │ │                              │ │
│ [FVTPL only] INFO BANNER                 │ │ Convergence Chart:           │ │
│ "EIR tidak berlaku untuk instrumen FVTPL"│ │ [Recharts LineChart          │ │
│                                          │ │  x=iterasi y=residual        │ │
│ [eir_awal IS NULL] ALERT BANNER          │ │  log-scale y-axis]           │ │
│ "EIR belum dihitung. Klik Hitung EIR."   │ │                              │ │
│                                          │ └──────────────────────────────┘ │
│ ACTION BAR                               │                                  │
│ [Hitung EIR] (ROLE-RISK, non-FVTPL)     │ <CatchUpAdjustmentCard>          │
│   spinner inline saat compute            │ (tampil jika ada amandemen aktif)│
│ [Propose Amandemen] (ROLE-AKUN)          │ ┌──────────────────────────────┐ │
│ [Export XLSX]                            │ │ Catch-Up Adjustment          │ │
├──────────────────────────────────────────┤ │ Delta: Rp 1.234.567,0000     │ │
│ SECTION C: SCHEDULE DATATABLE            │ │ Formula: v1.3 [link]         │ │
│                                          │ └──────────────────────────────┘ │
│ Version Selector: [Versi Aktif (2026-01-15) ▾]                              │
│   Dropdown options:                                                          │
│   ● Versi Aktif (mulai 2026-04-01)  [default]                               │
│   ○ Versi v1 (2026-01-15 — 2026-04-01) [superseded badge abu]               │
│                                                                              │
│ [superseded banner] "Anda melihat versi schedule yang sudah digantikan."    │
│                                                                              │
│ ┌───┬────────┬────────────┬────────────┬────────────┬────────────┬────────┐ │
│ │Seq│Tgl Mlt │Opening Crr │Pend Bunga  │ Amortisasi │Closing Crr │ EIR % │ │
│ ├───┼────────┼────────────┼────────────┼────────────┼────────────┼────────┤ │
│ │  1│2026-01 │10.000.000..│   35.416...│    (5.416.)│ 9.999.994..│4.25000 │ │
│ │  2│2026-02 │ 9.999.994..│   35.416...│    (5.416.)│ 9.999.988..│4.25000 │ │
│ │...│   ...  │    ...     │    ...     │    ...     │    ...     │  ...   │ │
│ └───┴────────┴────────────┴────────────┴────────────┴────────────┴────────┘ │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~3 (120 total)  [Next →]  Limit: [50 ▾]       │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Version selector: shadcn `<Select>` — opsi dengan badge "superseded" abu untuk versi lama
- `<AmortizationScheduleTable>` baru — wraps `<DataTable>` dengan version state
- `<NewtonRaphsonSolverPanel>` baru — Recharts LineChart convergence + stats
- `<CatchUpAdjustmentCard>` baru — tampil hanya jika `catchUpAdjustment` ada di response
- IDR columns: `NUMERIC(20,4)` 4 desimal, format `#.###.###,0000`
- EIR columns: `NUMERIC(10,8)` 8 desimal
- Non-convergence: solver panel tampil dengan status "Tidak Konvergen" merah + toast error persistent

---

### SCREEN-M9-05: Antrian Amandemen EIR + Detail (`/ecl/eir/amendments/`)

#### Sub-screen A: Queue (`/ecl/eir/amendments/queue`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › EIR › Antrian Amandemen                                   │
│                                            [+ Propose Amandemen] (ROLE-AKUN)│
├─────────────────────────────────────────────────────────────────────────────┤
│ TABS: [Semua] [Menunggu Review Saya (3)] [Menunggu Approval Saya (2)] [Selesai (5)] │
│                                                                              │
│ Filter bar: [🔍 Cari instrumen...]  [Status ▾]  [Trigger ▾]  [Tgl ▾]      │
│                                                                              │
│ ┌──────────┬──────────┬──────────┬─────────────────┬────────┬──────┬──────┐ │
│ │ ID       │ Instrumen│  Status  │ Trigger         │EIR Lama│ Δ(bp)│ Aksi │ │
│ ├──────────┼──────────┼──────────┼─────────────────┼────────┼──────┼──────┤ │
│ │AMEND-001 │OBL-001   │[PRv orng]│[AUTO Drift]badge│4.2500% │  +20 │ [⋮] │ │
│ │AMEND-002 │DEP-005   │[PRv orng]│Manual — AKUN-01 │5.1200% │  —   │ [⋮] │ │
│ │AMEND-003 │OBL-010   │[PRv orng]│[AUTO Drift]badge│3.8700% │  +15 │ [⋮] │ │
│ └──────────┴──────────┴──────────┴─────────────────┴────────┴──────┴──────┘ │
│                                                                              │
│ [AUTO Drift] badge: bg-amber-100 text-amber-800, icon AutoIcon              │
│ Δ(bp) merah jika > threshold, abu jika tidak signifikan                     │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]   [Export CSV] [Refresh ↺]       │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Sub-screen B: Detail Amandemen (`/ecl/eir/amendments/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › EIR › Antrian Amandemen › AMEND-001                      │
├────────────────────────────────────┬────────────────────────────────────────┤
│ HERO CARD (kiri, 62%)              │ WORKFLOW PANEL (kanan, 38%)            │
│                                    │                                        │
│ ┌──────────────────────────────┐   │ <MakerReviewerApproverPanel>           │
│ │ Amandemen AMEND-001          │   │ 4-eyes mode (onApprove, no onApprove2) │
│ │ [CRON_DRIFT badge auto]      │   │                                        │
│ │ [PENDING_REVIEW badge]       │   │ Step 1: Maker — done                   │
│ │                              │   │  [CRON] Auto-generated, 2026-06-12     │
│ │ Instrumen: OBL-2026-00001   │   │                                        │
│ │ Kode: OBL-2026-00001         │   │ Step 2: Reviewer (RISK) — ACTIVE       │
│ │ Klasifikasi: AC              │   │  [ApprovalWithSignature]               │
│ │                              │   │  requireStepUpMfa=false                │
│ │ EIR Sebelum: 4.2500%         │   │  sodBlocked=[if AKUN login]            │
│ │ EIR Baru:    4.4500%         │   │  SoD tooltip: "Tidak bisa review       │
│ │ Delta:       +20 bp          │   │  proposal yang Anda buat sendiri"      │
│ │ [Lihat Drift Report DR-001 →]│   │                                        │
│ │  (jika trigger_source=CRON)  │   │ Step 3: Approver (ALCO) —              │
│ │                              │   │  Menunggu step 2 selesai               │
│ │ Catch-Up Adjustment:         │   │  requireStepUpMfa=true                 │
│ │ Rp 1.234.567,0000            │   │                                        │
│ │ [detail formula ▾]           │   │ [Batalkan Proposal] (ROLE-AKUN maker)  │
│ └──────────────────────────────┘   │ → <DestructiveActionDialog>            │
│                                    │   reason min 20 char                   │
│ CASHFLOW REVISI (inline table)     │                                        │
│ ┌──────────┬──────────┬──────────┐ │                                        │
│ │ Periode  │ CF Lama  │ CF Baru  │ │                                        │
│ ├──────────┼──────────┼──────────┤ │                                        │
│ │ 2026-07  │35.416... │38.750... │ │                                        │
│ │ 2026-08  │35.416... │38.750... │ │                                        │
│ │ ... (max 10 row preview)       │ │                                        │
│ └──────────┴──────────┴──────────┘ │                                        │
│ [Tampilkan Semua ▾] collapsible    │                                        │
└────────────────────────────────────┴────────────────────────────────────────┘
```

**Annotasi komponen:**
- `<RoutingPathBadge>` baru — tipe trigger: MANUAL, CRON_DRIFT, AD_HOC_BULK, DOCUMENT_UPLOAD
- Drift Report link: shadcn `<Button variant="link">` dengan ExternalLink icon
- Cashflow table: 10 row inline + Accordion "Tampilkan Semua" (shadcn Accordion)
- `<CatchUpAdjustmentCard>` — reuse dari SCREEN-M9-04

---

### SCREEN-M9-06: Drift Report List + Detail (`/ecl/eir/drift-reports`)

#### Sub-screen A: List (`/ecl/eir/drift-reports`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › EIR › Drift Report                                        │
│                            [Generate Drift Report] (ROLE-RISK, eir.amend.detect) │
├─────────────────────────────────────────────────────────────────────────────┤
│ Filter bar: [Trigger Type ▾]  [Tgl Run dari—sampai]  [🔴 Hanya Drift > 0]  │
│ Filter chips: [CRON_DAILY ×]  [Drift > 0 ×]  [Clear semua]                 │
│                                                                              │
│ ┌────────┬─────────────┬──────────────┬───────┬───────────┬──────────┬────┐ │
│ │ ID     │ Trigger     │ Waktu Scan   │ Total │ Drift     │ Proposal │Sts │ │
│ ├────────┼─────────────┼──────────────┼───────┼───────────┼──────────┼────┤ │
│ │ DR-001 │ CRON_DAILY  │2026-06-12 02:│  300  │ [🔴 3]   │    2     │✓   │ │
│ │ DR-002 │ AD_HOC      │2026-06-11 10:│  300  │ [🟡 1]   │    0     │✓   │ │
│ │ DR-003 │ CRON_DAILY  │2026-06-11 02:│  300  │ [🟢 0]   │    0     │✓   │ │
│ │ DR-004 │ AD_HOC      │2026-06-10 15:│   50  │ [🔴 5]   │    5     │✓   │ │
│ │ DR-005 │ PRE_ECL_CALC│2026-06-09 ..│  300  │ [🟢 0]   │    0     │✓   │ │
│ └────────┴─────────────┴──────────────┴───────┴───────────┴──────────┴────┘ │
│                                                                              │
│ Drift count badge: 🔴 merah jika > 0, 🟢 hijau jika 0 (+ angka + ikon)     │
│ Baris clickable → detail report                                              │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]   [Export CSV] [Refresh ↺]       │
│                                                                              │
│ [JobProgressPanel] — global, muncul saat drift detection berjalan:          │
│ ┌────────────────────────────────────────────────────────────────────────┐  │
│ │ [↻]  Drift Detection — EIR Scan                         [Background]  │  │
│ │       Mengevaluasi instrumen 150 dari 300                [Batalkan]    │  │
│ │       ████████████░░░░░░░░░░  50%   ETA: 3 menit lagi               │  │
│ └────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Sub-screen B: Detail Report (`/ecl/eir/drift-reports/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: ECL › EIR › Drift Report › DR-001                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ SUMMARY CARDS — 4 cards horizontal                                           │
│ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐│
│ │ Total Scan   │ │ Drift Count  │ │ Proposal     │ │ Missing Schedule     ││
│ │     300      │ │   3 (merah) │ │ Auto-Dibuat  │ │       0              ││
│ │ instrumen    │ │             │ │      2        │ │                      ││
│ └──────────────┘ └──────────────┘ └──────────────┘ └──────────────────────┘│
│                                                                              │
│ Trigger: CRON_DAILY — 2026-06-12 02:00 WIB                                  │
│                                                                              │
│ TABEL PER-INSTRUMEN DRIFT ENTRIES                                            │
│ ┌───────────┬──────────┬──────────┬────────┬──────────┬───────────────────┐ │
│ │ Instrumen │EIR Simpan│EIR Recomp│  Δ(bp) │ Severity │ Status Proposal   │ │
│ ├───────────┼──────────┼──────────┼────────┼──────────┼───────────────────┤ │
│ │OBL-001    │4.2500%   │4.4500%   │   +20  │[HIGH merah]│[Proposal Dibuat]│ │
│ │DEP-005    │5.1200%   │5.1350%   │   +15  │[MED orng]│[Proposal Dibuat]  │ │
│ │OBL-010    │3.8700%   │3.8705%   │    +1  │[LOW abu] │—                  │ │
│ └───────────┴──────────┴──────────┴────────┴──────────┴───────────────────┘ │
│                                                                              │
│ [Proposal Dibuat] = link badge → /ecl/eir/amendments/{amend_id}             │
│ Severity badge: HIGH=merah, MEDIUM=oranye, LOW=abu, MISSING=merah           │
│                                                                              │
│ Footer: [← Prev]  Page 1 of ~1  [Next →]   [Export CSV] [Refresh ↺]       │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Annotasi komponen:**
- Summary cards: shadcn `<Card>` — 4 per baris, angka bold besar
- Per-instrumen table: `<DataTable>` — data dari JSONB client-side (MVP ≤ 300 instrumen)
- "Generate Drift Report" → `<DestructiveActionDialog>` (confirmatory, bukan destructive)
- `<JobProgressPanel>` existing — global drawer/inline per ux-patterns.md §3
- Tombol "Generate Drift Report" di-disable saat ada job aktif, tooltip "Job sedang berjalan"

---

## 3. Component Specification

### Komponen Baru yang Perlu Dibuat

#### 3.1 `<StageBadge>`

**Path**: `frontend/src/components/blips/StageBadge.tsx`

**Props:**
```typescript
interface StageBadgeProps {
  stage: 1 | 2 | 3 | null;
  size?: "sm" | "default" | "lg";
  // lg = full-width banner (SCREEN-M9-01 stage banner)
  // default = inline pill (tabel, header)
  // sm = tiny chip (tabel cell)
  showIcon?: boolean;  // default true
  className?: string;
}
```

**Token mapping:**
| Stage | Kelas bg | Kelas text | Icon lucide | Label Bahasa |
|---|---|---|---|---|
| 1 | `bg-green-100` | `text-green-800` | `ShieldCheck` | "Stage 1 — Berkinerja Baik" |
| 2 | `bg-amber-100` | `text-amber-800` | `AlertTriangle` | "Stage 2 — Risiko Meningkat" |
| 3 | `bg-red-100` | `text-red-800` | `ShieldX` | "Stage 3 — Default" |
| null | `bg-gray-100` | `text-gray-600` | `Circle` | "Belum Dievaluasi" |

**Aksesibilitas**: `role="status"` pada size lg, `aria-label` selalu ada. Color-blind safe: ikon berbeda per stage, bukan hanya warna.

---

#### 3.2 `<MFAStepUpModal>`

**Path**: `frontend/src/components/blips/MFAStepUpModal.tsx`

**Status**: Belum ada di codebase (Phase 3 referenced di stories tapi belum dibuat). Buat baru.

**Props:**
```typescript
interface MFAStepUpModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  onVerified: (stepUpToken: string) => void;
  onCancel?: () => void;
}
```

**Layout (shadcn Dialog):**
```
┌─────────────────────────────────────────────────────┐
│ Verifikasi MFA Step-Up                          [×] │
├─────────────────────────────────────────────────────┤
│ {title}                                             │
│ {description}                                       │
│                                                     │
│ Metode verifikasi: TOTP / WebAuthn (dari JWT claim) │
│                                                     │
│ Kode TOTP (6 digit):                               │
│ [  ] [  ] [  ] [  ] [  ] [  ]   ← OTP input       │
│ aria-label="Kode OTP digit 1" dst per digit         │
│                                                     │
│ atau                                                │
│ [Gunakan WebAuthn / Touch ID]                       │
│                                                     │
│ [Batal]               [Verifikasi →] spinner       │
└─────────────────────────────────────────────────────┘
```

**Perilaku:**
- Auto-focus digit pertama saat modal terbuka
- Auto-submit saat digit ke-6 terisi
- Pada error: "Kode salah. Sisa percobaan: 2." (merah, `role="alert"`)
- Keyboard: Tab antar digit, Backspace hapus + focus sebelumnya
- Setelah verifikasi sukses: `onVerified(stepUpToken)` dipanggil, modal tutup
- Focus management: return focus ke trigger button saat modal tutup

**Integrasi**: Dipanggil dari SCREEN-M9-02 (override approve) dan SCREEN-M9-05 (amandemen approve). Direncanakan reuse di M10 juga.

---

#### 3.3 `<SicrEvidenceCard>`

**Path**: `frontend/src/components/blips/SicrEvidenceCard.tsx`

**Props:**
```typescript
interface SicrEvidenceCardProps {
  triggerType: "RATING_DOWNGRADE" | "IG_TO_NON_IG" | "DPD_THRESHOLD" | 
               "CURE_FULL" | "CURE_PARTIAL" | "MANUAL_OVERRIDE" | 
               "ORIGINATION" | "DPD_CORRECTION";
  evidence: {
    notchChange?: number;    // mis. -2
    ratingLama?: string;     // mis. "idBBB"
    ratingBaru?: string;     // mis. "idBB"
    dpd?: number;            // mis. 35
    curiePeriode?: number;   // mis. 3
  } | null;
  date?: string;
}
```

**Trigger type label map (Bahasa Indonesia):**
| triggerType | Label |
|---|---|
| RATING_DOWNGRADE | "Penurunan Peringkat" |
| IG_TO_NON_IG | "Perubahan ke Non-Investment Grade" |
| DPD_THRESHOLD | "Melebihi Ambang Tunggakan" |
| CURE_FULL | "Pemulihan Penuh" |
| CURE_PARTIAL | "Pemulihan Sebagian" |
| MANUAL_OVERRIDE | "Override Manual" |
| ORIGINATION | "Originasi Instrumen" |
| DPD_CORRECTION | "Koreksi Data DPD" |

---

#### 3.4 `<NewtonRaphsonSolverPanel>`

**Path**: `frontend/src/components/blips/NewtonRaphsonSolverPanel.tsx`

**Props:**
```typescript
interface SolverPanelProps {
  solverMetadata: {
    iterations: number;
    maxIterations: number;       // always 100 per DEC-013
    finalResidual: string;       // string karena decimal presisi tinggi
    converged: boolean;
    precision: string;           // "HALF_EVEN, 8 desimal"
    convergencePath?: Array<{    // optional — untuk chart
      iteration: number;
      residual: number;          // float ok untuk display chart saja
    }>;
  };
}
```

**Layout**: Vertikal card.
- Baris stats: Status badge (Konvergen hijau / Tidak Konvergen merah), Iterasi, Residual
- Recharts `<LineChart>`: x=iterasi (1..N), y=residual log-scale. Jika `convergencePath` kosong, tampilkan teks saja (tanpa chart).
- Non-convergence: entire panel border merah, status "Tidak Konvergen" dengan ikon XCircle

---

#### 3.5 `<CatchUpAdjustmentCard>`

**Path**: `frontend/src/components/blips/CatchUpAdjustmentCard.tsx`

**Props:**
```typescript
interface CatchUpAdjustmentCardProps {
  adjustment: {
    deltaAmount: string;      // IDR NUMERIC(20,4) sebagai string
    formulaVersion: string;   // mis. "v1.3"
    approvalRecordUrl?: string;
  } | null;
}
```

Tampil hanya jika `adjustment !== null`. Card compact dengan label "Penyesuaian Catch-Up", nilai IDR format `Rp #.###.###,0000`, link ke approval record.

---

#### 3.6 `<AmortizationScheduleTable>`

**Path**: `frontend/src/components/blips/AmortizationScheduleTable.tsx`

Wrapper `<DataTable>` yang mengelola state version selector.

**Props:**
```typescript
interface AmortizationScheduleTableProps {
  instrumenId: string;
  versions: Array<{
    versionLabel: string;    // mis. "Versi Aktif (2026-04-01)"
    scheduleVersion: number;
    isActive: boolean;
    effectiveFrom: string;
    effectiveTo: string | null;  // null = active
  }>;
}
```

Kolom DataTable:
- `periode_seq` (seq number, right-align)
- `tanggal_mulai` (date)
- `opening_carrying_amount` (IDR 4 des)
- `pendapatan_bunga_eir` (IDR 4 des)
- `amortisasi_premium_diskon` (IDR 4 des, bisa negatif)
- `pelunasan_pokok` (IDR 4 des)
- `closing_carrying_amount` (IDR 4 des)
- `eir_periode` (persen 8 des)
- `status_posting` (badge: "Diposting" hijau / "Belum" abu)

---

#### 3.7 `<JSONBTreeView>`

**Path**: `frontend/src/components/blips/JSONBTreeView.tsx`

Display collapsed JSONB dengan tree expand/collapse. Dipakai di Evidence cell di SCREEN-M9-01 dan parameter snapshot di M10.

**Props:**
```typescript
interface JSONBTreeViewProps {
  data: Record<string, unknown>;
  maxDepth?: number;          // default 3
  initiallyExpanded?: boolean; // default false
}
```

Render key-value pairs dengan indentasi. Non-recursive past `maxDepth` — tampilkan `[Object]` placeholder. Keyboard navigable: Enter/Space toggle expand.

---

#### 3.8 `<RoutingPathBadge>`

**Path**: `frontend/src/components/blips/RoutingPathBadge.tsx`

Badge visual untuk `trigger_source` amandemen EIR.

**Props:**
```typescript
interface RoutingPathBadgeProps {
  triggerSource: "MANUAL" | "CRON_DRIFT" | "AD_HOC_BULK" | "DOCUMENT_UPLOAD";
}
```

| triggerSource | Label | Warna | Icon |
|---|---|---|---|
| MANUAL | "Manual" | abu/gray | UserIcon |
| CRON_DRIFT | "AUTO (Drift)" | amber | RefreshCwIcon |
| AD_HOC_BULK | "Bulk Ad-Hoc" | blue | LayersIcon |
| DOCUMENT_UPLOAD | "Upload Dokumen" | purple | UploadIcon |

---

#### 3.9 `<JobProgressPanel>` — konfirmasi ketersediaan

**Path**: `frontend/src/components/blips/JobProgressPanel.tsx`

**Status**: Belum ada di filesystem (tidak terdeteksi di Glob). Harus dibuat sesuai ux-patterns.md §3.

Komponen ini dipakai di M9-003 (DPD re-staging), M9-006 (drift detection), dan M9-004 (EIR compute inline).

**Props minimal:**
```typescript
interface JobProgressPanelProps {
  jobId: string;
  title: string;
  onComplete?: (result: unknown) => void;
  onFail?: (error: unknown) => void;
  showCancel?: boolean;
  showBackground?: boolean;  // default true
  variant?: "inline" | "panel";
  // inline = embedded dalam halaman (DPD re-staging kecil)
  // panel = floating card (drift detection)
}
```

---

## 4. Interaction Specification

### 4.1 SoD UI Enforcement

**Aturan**: Tombol aksi di-disable (bukan disembunyikan) dengan tooltip yang menjelaskan alasan jika SoD berlaku.

| Skenario | Tombol | State | Tooltip |
|---|---|---|---|
| Override: user = proposer | "Approve" | `disabled` | "Anda tidak dapat menyetujui proposal yang Anda buat sendiri (SoD)." |
| Amandemen: AKUN = maker | "Review" | `disabled` | "Anda tidak dapat mereview proposal yang Anda buat sendiri (SoD)." |
| Override: ALCO reviewer = approver-1 | "Approve Step 2" | `disabled` | "Anda sudah menjadi Approver 1. Approver 2 harus berbeda." |

Implementasi: di DataTable row, kolom Aksi render `<TooltipProvider><Tooltip><TooltipTrigger asChild><Button disabled>...</Button></TooltipTrigger><TooltipContent>{sodMessage}</TooltipContent></Tooltip></TooltipProvider>`.

Komponen `<MakerReviewerApproverPanel>` existing sudah handle SoD via `isSoDViolation()` — extend untuk 6-eyes override.

---

### 4.2 Step-Up MFA Flow

```
1. User klik "Approve" (qualifying action)
   ↓
2. Cek JWT claim mfa_verified_at timestamp
   - Jika < 5 menit lalu: lanjut langsung ke step 4
   - Jika ≥ 5 menit atau tidak ada: lanjut ke step 3
   ↓
3. Tampilkan confirmation dialog (shadcn AlertDialog):
   "Approve [entity] — [detail]. Periksa sebelum melanjutkan."
   [Batal] [Lanjutkan ke MFA]
   ↓
4. <MFAStepUpModal> terbuka
   User input TOTP atau WebAuthn
   ↓
5. Frontend POST ke /auth/step-up dengan kode
   - Sukses: dapat stepUpToken (exp: 5 menit)
   - Gagal: error inline di modal, sisa percobaan countdown
   ↓
6. Retry original action dengan header X-Step-Up-Token: {stepUpToken}
   ↓
7. Response sukses: modal tutup, toast sukses, panel workflow ter-refresh
   Response error (SOD_VIOLATION, dll): toast error persistent
```

**Client-side state management**: Zustand store `useStepUpStore` — simpan `{ stepUpToken, expiresAt }`. Cek freshness sebelum setiap MFA-required action.

---

### 4.3 Long-Running Operations (UX §3)

#### DPD Entry Re-staging (M9-003)
Estimasi < 2 detik untuk single instrument. Flow:
1. POST /dpd-records → 201 (sync, bukan async)
2. Response berisi `restagin_job_id` jika dispatched
3. Frontend: tampilkan JobProgressPanel `variant="inline"` di bawah form dengan `jobId`
4. SSE/polling 2s → saat complete: toast + auto-refresh list
5. Drawer tetap terbuka sampai job selesai, lalu tutup otomatis dengan toast

#### Drift Detection (M9-006)
Estimasi ≤ 5 detik untuk 300 instrumen. Flow:
1. Konfirmasi dialog → POST /drift-detection/trigger → 202 `{ jobId }`
2. JobProgressPanel `variant="panel"` muncul di atas list tabel
3. "Background" button: panel tutup, badge global notification +1
4. SSE complete: toast "Drift report selesai. 3 drift terdeteksi." + link "Lihat Laporan"
5. List DataTable di-refresh (TanStack Query invalidateQueries)

#### EIR Compute (M9-004)
Estimasi < 3 detik untuk single instrument.
1. POST /eir/compute → synchronous response (bukan job)
2. Tombol berubah spinner "Menghitung..." selama pending
3. Sukses: toast + solver panel ter-update + schedule DataTable di-refresh
4. Non-convergence (422): toast error persistent + solver panel tampil dengan status "Tidak Konvergen"

---

### 4.4 Empty States, Loading, Error

**Empty states per screen:**

| Screen | Empty State Copy | CTA |
|---|---|---|
| Staging history (M9-001) | "Belum ada riwayat staging. Jalankan evaluasi pertama." | [Evaluasi Staging] |
| Override queue (M9-002) | "Tidak ada proposal override aktif." | [+ Request Override] |
| DPD list (M9-003) | "Belum ada record DPD. Tambah record untuk instrumen." | [+ Tambah Record DPD] |
| EIR schedule (M9-004) eir_awal null | "EIR belum dihitung untuk instrumen ini." | [Hitung EIR] |
| Amendment queue (M9-005) | "Tidak ada proposal amandemen. Proposal baru atau tunggu deteksi drift." | [+ Propose Amandemen] |
| Drift reports (M9-006) | "Belum ada drift report. Klik 'Generate Drift Report' untuk evaluasi pertama." | [Generate Drift Report] |

**Loading states**: Skeleton rows via shadcn `<Skeleton>` — bukan blank screen. DataTable: 5 skeleton baris.

**Error states**: Toast error persistent (UX §2) + inline error di DataTable dengan retry button.

---

### 4.5 Form Validation Summary (Zod schemas)

| Form | Field | Aturan Validasi | Pesan Error (ID) |
|---|---|---|---|
| Override New | `reason` | min 20 char, max 1000 | "Alasan harus minimal 20 karakter (saat ini: {n} karakter)." |
| Override New | `expiry_periode` | harus masa depan dari today | "Expiry periode harus di masa mendatang ({periode} sudah lewat)." |
| Override New | `target_stage` | required, berbeda dari stage saat ini | "Target stage harus berbeda dari stage saat ini." |
| DPD Entry | `dpd` | integer, min 0 | "Nilai DPD tidak boleh negatif." |
| DPD Entry | `instrumen_id` + `periode_id` | kombinasi unik — check via API | "Record DPD sudah ada. Gunakan fungsi Edit." |
| Cancel Amendment | `cancel_reason` | min 20 char | "Alasan pembatalan minimal 20 karakter." |
| Reject Override | `reason` | min 20 char | "Alasan penolakan minimal 20 karakter." |

---

## 5. Accessibility Checklist

### DataTable
- `<table>` dengan `role="grid"`, kolom sortable header dengan `aria-sort="ascending|descending|none"`
- Tombol sort di header: `aria-label="Urutkan berdasarkan {kolom}, saat ini {asc/desc/none}"`
- Filter chip badge: `<span aria-label="Filter aktif: {kolom} = {nilai}">` + tombol clear `aria-label="Hapus filter {kolom}"`
- Pagination: `<nav aria-label="Navigasi halaman">`, tombol Prev/Next dengan `aria-label`

### Stage Badge
- WCAG 2.1 AA contrast: tested via token map di bawah
- Bukan hanya warna: icon + teks label selalu hadir
- `aria-label="Stage: {teks label}"` pada badge

**Contrast check (Stage badge):**
| Stage | bg | text | Approx contrast ratio |
|---|---|---|---|
| 1 (green-100 / green-800) | `#dcfce7` | `#166534` | 7.1:1 — AA pass |
| 2 (amber-100 / amber-800) | `#fef3c7` | `#92400e` | 5.3:1 — AA pass |
| 3 (red-100 / red-800) | `#fee2e2` | `#991b1b` | 5.8:1 — AA pass |

### MFA Modal
- Focus trap saat modal terbuka (Dialog inherits dari shadcn `<DialogContent>`)
- Escape tutup modal, focus kembali ke trigger button
- OTP input: `autocomplete="one-time-code"`, `inputmode="numeric"`, `pattern="[0-9]*"`
- Error: `aria-describedby` pada input, `role="alert"` pada pesan error
- "Kode salah. Sisa percobaan: {n}." — pengumuman screen reader via `aria-live="polite"`

### Long-Running Progress
- `<progress value={percent} max="100" aria-label="Kemajuan: {percent}%">`
- Status step: `aria-live="polite"` — diumumkan ke screen reader saat berubah
- Toast notifikasi: `role="status"` untuk sukses, `role="alert"` untuk error

### Form Fields
- Semua field dengan error: `aria-describedby="{field-id}-error"` dan `aria-invalid="true"`
- Required fields: `aria-required="true"` + tanda `*` dengan `aria-hidden="true"`
- Textarea karakter counter: terhubung ke field via `aria-describedby`

### Keyboard Navigation
- Tab order logis: breadcrumb → header → action bar → filter bar → tabel → pagination → footer
- DataTable row: Enter membuka detail, tombol aksi row terjangkau dengan Tab
- Drawer (Sheet): focus masuk ke field pertama saat terbuka, Escape tutup
- Version selector dropdown: Arrow Up/Down navigasi opsi, Enter pilih, Escape tutup

---

## 6. Bahasa Indonesia Copy Guide

### Label Utama UI

| Konsep Teknis | Label Bahasa Indonesia | Keterangan |
|---|---|---|
| Stage 1 | "Stage 1 — Berkinerja Baik" (banner) / "Stage 1" (tabel) | |
| Stage 2 | "Stage 2 — Risiko Meningkat" (banner) / "Stage 2" (tabel) | |
| Stage 3 | "Stage 3 — Default" (banner) / "Stage 3" (tabel) | |
| SICR trigger | "Pemicu SICR" | Significant Increase in Credit Risk |
| DPD | "Hari Tunggakan" (label form) / "DPD" (header tabel — akronim dipertahankan) | |
| Override | "Override Manual" | Hindari "Override" saja |
| Cure | "Pemulihan" | Cure penuh = "Pemulihan Penuh", parsial = "Pemulihan Sebagian" |
| Amendment | "Amandemen Kontrak" (full) / "Amandemen" (abbreviated) | |
| Drift | "Drift Parameter EIR" | |
| Solver | "Solver EIR" | Tidak perlu di-translate |
| Non-convergent | "Tidak Konvergen" | |
| Residual | "Nilai Sisa" (di tooltip) / "Residual" (di panel teknis) | Panel teknis boleh pakai EN |
| Effective Interest Rate | "Suku Bunga Efektif (EIR)" (pertama kali) / "EIR" (selanjutnya) | |
| Schedule | "Jadwal Amortisasi" | |
| Superseded | "Sudah Digantikan" | |
| Catch-Up Adjustment | "Penyesuaian Catch-Up" | Tidak ada padanan baik |
| Version selector | "Pilih Versi Schedule" | |
| Drift report | "Laporan Drift EIR" | |

### Error Messages (Bahasa Indonesia)

| Error Code | Toast Message ID |
|---|---|
| `SOD_VIOLATION` | "Anda tidak dapat melakukan tindakan ini pada data yang Anda buat sendiri (Segregasi Tugas)." |
| `STAGING_OVERRIDE_ACTIVE_EXISTS` | "Instrumen {id} sudah punya proposal override aktif. Selesaikan atau batalkan yang ada." |
| `DPD_RECORD_DUPLICATE` | "Record DPD untuk {instrumen} periode {periode} sudah ada. Gunakan fungsi Edit." |
| `EIR_SOLVER_NON_CONVERGENT` | "EIR solver tidak konvergen setelah 100 iterasi untuk {instrumen}. Periksa input cashflow. Trace: {traceId}" |
| `CONFLICT` (drift concurrent) | "Drift detection sedang berjalan (Job: {jobId}). Tunggu hingga selesai." |
| `WORKFLOW_INVALID_TRANSITION` | "Tindakan tidak valid untuk status saat ini ({status}). Muat ulang halaman." |
| `FORBIDDEN` | "Anda tidak memiliki izin untuk tindakan ini. Hubungi administrator." |

### Toast Messages — Sukses

| Aksi | Toast Sukses ID |
|---|---|
| Override proposed | "Proposal override stage {instrumen} berhasil disubmit. Menunggu review ALCO." |
| Override approved (step 1) | "Approval step 1 berhasil. Menunggu approval step 2." |
| Override approved (final) | "Override stage {instrumen} disetujui. Stage diubah dari {dari} ke {ke}." |
| Override rejected | "Proposal {id} berhasil ditolak." |
| DPD saved | "Record DPD {instrumen} periode {periode} berhasil disimpan. Mengevaluasi ulang staging..." |
| DPD re-staging changed | "Stage {instrumen} berubah: Stage {dari} → Stage {ke} ({trigger}). Lihat detail." |
| DPD re-staging unchanged | "Staging {instrumen} tidak berubah ({alasan}). Stage tetap: {stage}." |
| EIR computed | "EIR berhasil dihitung: {eir}%. {n} baris schedule di-generate." |
| Amendment reviewed | "Proposal {id} disetujui review. Menunggu approval ALCO." |
| Amendment approved | "Amandemen EIR {id} disetujui. Schedule EIR {instrumen} diperbarui." |
| Amendment cancelled | "Proposal amandemen {id} berhasil dibatalkan." |
| Drift detection done | "Drift report berhasil dibuat. {total} instrumen dievaluasi, {drift} drift terdeteksi, {proposal} proposal dibuat." |

---

## 7. Hand-off untuk frontend-engineer-nextjs

### 7.1 File Structure Proposal

```
frontend/src/
├── app/
│   └── ecl/
│       ├── staging/
│       │   ├── instrumen/
│       │   │   └── [id]/
│       │   │       └── page.tsx            — M9-001
│       │   ├── override/
│       │   │   ├── page.tsx               — M9-002 queue
│       │   │   ├── new/
│       │   │   │   └── page.tsx           — M9-002 form
│       │   │   └── [id]/
│       │   │       └── page.tsx           — M9-002 detail
│       │   └── dpd/
│       │       └── page.tsx               — M9-003
│       └── eir/
│           ├── instrumen/
│           │   └── [id]/
│           │       └── page.tsx           — M9-004
│           ├── amendments/
│           │   ├── queue/
│           │   │   └── page.tsx           — M9-005 queue
│           │   ├── new/
│           │   │   └── page.tsx           — M9-005 form (propose)
│           │   └── [id]/
│           │       └── page.tsx           — M9-005 detail
│           └── drift-reports/
│               ├── page.tsx               — M9-006 list
│               └── [id]/
│                   └── page.tsx           — M9-006 detail
├── components/blips/
│   ├── StageBadge.tsx                     — BARU
│   ├── MFAStepUpModal.tsx                 — BARU
│   ├── SicrEvidenceCard.tsx               — BARU
│   ├── NewtonRaphsonSolverPanel.tsx        — BARU
│   ├── CatchUpAdjustmentCard.tsx          — BARU
│   ├── AmortizationScheduleTable.tsx      — BARU
│   ├── JSONBTreeView.tsx                  — BARU
│   ├── RoutingPathBadge.tsx               — BARU
│   └── JobProgressPanel.tsx              — BARU (referenced di stories tapi tidak ada)
└── hooks/
    └── useJobProgress.ts                  — BARU (SSE + polling fallback)
```

### 7.2 shadcn/ui Components yang Diperlukan

Pastikan komponen berikut sudah ada di `components/ui/` (run `npx shadcn-ui@latest add` jika belum):

| Komponen | Dipakai di |
|---|---|
| `card` | Semua header card, summary card |
| `dialog` | MFAStepUpModal, confirmation dialog |
| `sheet` | DPD entry drawer (M9-003) |
| `tabs` | Amendment queue tab (M9-005) |
| `accordion` | Cashflow revisi "Tampilkan Semua" (M9-005) |
| `select` | Version selector (M9-004), filter dropdowns |
| `tooltip` | SoD disabled button tooltip |
| `alert` | Stage 3 warning, EIR not computed, FVTPL info |
| `badge` | Trigger source, severity, status |
| `skeleton` | Loading states semua DataTable |
| `progress` | JobProgressPanel progress bar |

### 7.3 Recharts Components

| Chart | Screen | Komponen Recharts |
|---|---|---|
| Convergence path | NewtonRaphsonSolverPanel (M9-004) | `<LineChart>`, `<Line>`, `<XAxis>`, `<YAxis scale="log">`, `<Tooltip>` |

### 7.4 State Management (Zustand)

Buat dua Zustand stores baru:

**`useOverrideWorkflowStore`** (`lib/stores/overrideWorkflow.ts`):
```typescript
interface OverrideWorkflowState {
  stepUpToken: string | null;
  stepUpExpiresAt: number | null;
  isStepUpFresh: () => boolean;
  setStepUp: (token: string, expiresAt: number) => void;
  clearStepUp: () => void;
}
```

**`useAmendmentWorkflowStore`** (`lib/stores/amendmentWorkflow.ts`):
- Sama struktur dengan Override, shared state untuk step-up token

**Alternatif**: Gunakan satu shared `useStepUpStore` karena kedua flow memerlukan step-up MFA yang sama.

**`useActiveJobsStore`** (`lib/stores/activeJobs.ts`):
```typescript
interface ActiveJobsState {
  jobs: Map<string, { jobId: string; title: string; status: string; progress: number }>;
  badgeCount: number;
  addJob: (jobId: string, title: string) => void;
  updateJob: (jobId: string, updates: Partial<...>) => void;
  completeJob: (jobId: string) => void;
}
```

### 7.5 API Query Hooks (TanStack Query)

```typescript
// Staging
useQuery(['staging-current', instrumenId], () => api.staging.getCurrent(instrumenId))
useQuery(['staging-history', instrumenId, filters], () => api.staging.getHistory(instrumenId, filters))
useQuery(['staging-overrides', filters], () => api.staging.getOverrides(filters))
useMutation(api.staging.createOverride)
useMutation(api.staging.reviewOverride)
useMutation(api.staging.approveOverride)
useMutation(api.staging.rejectOverride)
useQuery(['dpd-records', filters], () => api.staging.getDpdRecords(filters))
useMutation(api.staging.createDpdRecord)
useMutation(api.staging.updateDpdRecord)

// EIR
useQuery(['eir-schedule', instrumenId, version], () => api.eir.getSchedule(instrumenId, version))
useQuery(['eir-history', instrumenId], () => api.eir.getHistory(instrumenId))
useMutation(api.eir.compute)
useQuery(['eir-amendments', filters, tab], () => api.eir.getAmendments(filters))
useMutation(api.eir.reviewAmendment)
useMutation(api.eir.approveAmendment)
useMutation(api.eir.rejectAmendment)
useMutation(api.eir.cancelAmendment)
useQuery(['drift-reports', filters], () => api.eir.getDriftReports(filters))
useQuery(['drift-report-detail', id], () => api.eir.getDriftReport(id))
useMutation(api.eir.triggerDriftDetection)

// Jobs
useQuery(['job-status', jobId], () => api.jobs.getStatus(jobId), { enabled: !!jobId })
```

### 7.6 URL State (nuqs / searchParams)

Semua DataTable filter/sort harus tercermin di URL untuk deep-link:

| Route | URL State keys |
|---|---|
| `/ecl/staging/instrumen/[id]` | `?filter[trigger_type]=&filter[tanggal_from]=&filter[tanggal_to]=&sort=&cursor=` |
| `/ecl/staging/override` | `?filter[status]=&filter[target_stage]=&sort=&cursor=` |
| `/ecl/staging/dpd` | `?filter[instrumen]=&filter[periode]=&filter[stage]=&sort=&cursor=` |
| `/ecl/eir/instrumen/[id]` | `?schedule_version=&sort=&cursor=` |
| `/ecl/eir/amendments/queue` | `?tab=&filter[status]=&filter[trigger]=&sort=&cursor=` |
| `/ecl/eir/drift-reports` | `?filter[trigger_type]=&filter[drift_gt0]=&filter[date_from]=&filter[date_to]=&sort=&cursor=` |

### 7.7 Validation Rules (Zod)

```typescript
// Override proposal schema
const overrideProposalSchema = z.object({
  instrumen_id: z.string().uuid("Pilih instrumen yang valid"),
  target_stage: z.union([z.literal(2), z.literal(3)]),
  reason: z.string().min(20, "Alasan harus minimal 20 karakter").max(1000),
  expiry_periode: z.string().refine(
    (val) => isPeriodeInFuture(val),
    { message: "Expiry periode harus di masa mendatang" }
  ),
});

// DPD entry schema  
const dpdEntrySchema = z.object({
  instrumen_id: z.string().uuid(),
  periode_id: z.string(),
  dpd: z.number().int().min(0, "Nilai DPD tidak boleh negatif"),
  keterangan: z.string().max(200).optional(),
});

// Cancel/reject reason schema (shared)
const reasonSchema = z.object({
  reason: z.string().min(20, "Alasan minimal 20 karakter").max(1000),
});
```

---

## 8. Frontend-Engineer Handoff Checklist

```
[ ] Buat folder structure frontend/src/app/ecl/ sesuai §7.1
[ ] Verifikasi shadcn components tersedia (§7.2) — install yang belum ada
[ ] Buat 9 komponen baru (§3) dengan props sesuai spec
    [ ] StageBadge — color map + contrast test
    [ ] MFAStepUpModal — OTP input + focus trap + keyboard nav
    [ ] SicrEvidenceCard — trigger type label map
    [ ] NewtonRaphsonSolverPanel — Recharts log-scale chart
    [ ] CatchUpAdjustmentCard — IDR format
    [ ] AmortizationScheduleTable — version selector + DataTable
    [ ] JSONBTreeView — collapsible tree
    [ ] RoutingPathBadge — trigger source map
    [ ] JobProgressPanel — SSE + polling fallback (useJobProgress hook)
[ ] Buat Zustand stores (§7.4)
[ ] Buat TanStack Query hooks (§7.5)
[ ] Set up URL state via nuqs untuk semua DataTable (§7.6)
[ ] Implementasi 6 screen (11 sub-screen total per sitemap §1)
[ ] SoD enforcement: disabled button + tooltip per §4.1
[ ] Step-up MFA flow per §4.2
[ ] Long-running progress per §4.3
[ ] Empty states / loading skeleton / error states per §4.4
[ ] Bahasa Indonesia copy sesuai §6
[ ] Aksesibilitas: semua checklist §5
[ ] Test URL deep-link: bookmark filter URL → restore state saat reload
[ ] Idempotency-Key header di semua POST/PUT/PATCH mutations
```

---

*Desainer: uiux-designer | Review aksesibilitas: qa-engineer | Advisory compliance: ifrs9-compliance-reviewer*
