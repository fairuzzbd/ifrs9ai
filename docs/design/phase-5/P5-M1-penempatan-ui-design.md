# P5-M1 — Penempatan Deposito UI Design Specification

**Story Set**: P5-M1
**Modul**: APP-B — Transaction Lifecycle
**Desainer**: uiux-designer
**Tanggal**: 2026-06-14
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M1-penempatan-deposito.md`
**Linked API**: `api/openapi/app-b-penempatan-deposito.yaml`
**Linked State Machine**: `docs/state-machines/p5-m1-penempatan.md`
**Decisions applied**:
- DEC-P5-M1-001 (FVTPL staging skip — informational banner only)
- DEC-P5-M1-004 (settlement balance informational, non-blocking)
- DEC-P5-M1-005 (terminate = 4-eyes full workflow)
- DEC-017 (SoD enforcement)
- DEC-027 (MFA step-up on approve + terminate-approve)

---

## 1. Information Architecture

### Sitemap P5-M1

```
Transaksi (side nav group, collapsible)
├── Penempatan Deposito
│   ├── /trx/penempatan                        — List dashboard (S4)
│   ├── /trx/penempatan/new                    — Form create (S1)
│   ├── /trx/penempatan/[id]                   — Detail + workflow panel + EIR panel (S2, S3, S4b)
│   └── /trx/penempatan/[id]/edit              — Edit DRAFT only (S4b)
```

### Navigasi side nav

```
Transaksi
  ▾ Penempatan Deposito
      Semua Penempatan     → /trx/penempatan
      Buat Baru            → /trx/penempatan/new   (hanya ROLE-MAKER-TR)
```

Subhalaman detail dan edit diakses via tabel row klik atau action button — tidak muncul di nav.
Audit timeline (S6) ditampilkan sebagai tab di dalam halaman detail `/trx/penempatan/[id]`, bukan halaman terpisah.

---

## 2. Wireframes — 5 Screens

### SCREEN-P5-01: List Dashboard (`/trx/penempatan`)

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                           │
│  Penempatan Deposito                                 [+ Buat Penempatan] (MAKER only) │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                            │
│ [🔍 Cari kode / ref bank...]  [Status ▾]  [Bank ▾]  [Periode ▾]  [Klasifikasi ▾]    │
│ Filter chips: [Status: PENDING_REVIEW ×]  [Bank: BCA ×]          [Clear semua]       │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]   "Diperbarui: 14 Jun 2026, 10:35"   │
├──────────────┬────────────────┬──────────────────────────┬────────────┬──────────────┤
│ Kode ↕       │ Bank ↕         │ Status                   │ Nominal ↕  │ Aksi         │
├──────────────┼────────────────┼──────────────────────────┼────────────┼──────────────┤
│PNP-2026-00001│ BCA            │[abu]    Konsep           │ 5 Miliar   │ ▾ Aksi       │
│PNP-2026-00002│ BNI            │[amber]  Menunggu Review  │ 10 Miliar  │ ▾ Aksi       │
│PNP-2026-00003│ Mandiri        │[amber]  Menunggu Approval│ 3 Miliar   │ ▾ Aksi       │
│PNP-2026-00004│ CITI           │[hijau]  Aktif            │ 15,4 Miliar│ ▾ Aksi       │
│PNP-2026-00005│ BRI            │[merah]  Ditolak          │ 2 Miliar   │ ▾ Aksi       │
│PNP-2026-00006│ BCA            │[biru]   Jatuh Tempo      │ 8 Miliar   │ ▾ Aksi       │
│PNP-2026-00007│ BNI            │[ungu]   Menunggu Review  │ 5 Miliar   │ ▾ Aksi       │
│              │                │        Terminasi         │            │              │
├──────────────┴────────────────┴──────────────────────────┴────────────┴──────────────┤
│ Footer: [← Prev]  Hal. 1 dari ~13  [Next →]  Baris: [50 ▾]  Total estimasi: 127    │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter |
|---|---|---|---|
| `kode_transaksi` | Kode | ya | text search |
| `counterparty_bank` | Bank Counterparty | ya | select multi |
| `workflow_status` | Status | ya | select multi (11 opsi) |
| `nominal_idr` | Nominal (IDR) | ya | gte/lte |
| `tanggal_penempatan` | Tgl Penempatan | ya | between date |
| `tanggal_jatuh_tempo` | Jatuh Tempo | ya | — |
| `klasifikasi_psak71` | Klasifikasi | ya | select multi |
| `kupon_persen` | Kupon (%) | ya | — |
| — | Aksi | — | — |

**Action menu per row** (kontekstual per status + role):

| Status | Menu item (role yang melihat) |
|---|---|
| DRAFT (maker = user saat ini) | Lihat Detail, Edit, Submit, Batalkan |
| DRAFT (maker bukan user ini) | Lihat Detail |
| PENDING_REVIEW (user bukan maker) | Lihat Detail, Review, Tolak |
| PENDING_REVIEW (user = maker) | Lihat Detail [SoD — aksi review tidak muncul] |
| PENDING_APPROVAL (user bukan maker/reviewer) | Lihat Detail, Approve (MFA), Tolak |
| PENDING_APPROVAL (user = maker/reviewer) | Lihat Detail [SoD — aksi approve tidak muncul] |
| APPROVED_ACTIVE (user = maker) | Lihat Detail, Ajukan Terminasi |
| APPROVED_ACTIVE (user bukan maker) | Lihat Detail |
| TERMINATION_PENDING_REVIEW | Lihat Detail, [Review Terminasi jika SoD ok] |
| TERMINATION_PENDING_APPROVAL | Lihat Detail, [Approve Terminasi (MFA) jika SoD ok] |
| MATURED / TERMINATED / CANCELLED | Lihat Detail |

**Catatan**: menu item yang melanggar SoD tidak muncul sama sekali (bukan disabled — pengguna tidak perlu melihat aksi yang tidak bisa mereka lakukan). Tooltip penjelasan SoD hanya muncul jika user secara eksplisit bertanya (hover ikon info).

**Kolom Nominal**: format IDR, ribu dipisah titik, desimal koma, disingkat miliar untuk display (e.g., "5.000.000.000,0000" → "5 Miliar" di kolom list, angka penuh di detail).

**Export**: CSV dan XLSX. Menghormati filter aktif. Dataset > 10.000 baris → async export via `<JobProgressPanel>`. Dataset ≤ 10.000 baris → inline download. Audit `PENEMPATAN.EXPORT` setiap export.

---

### SCREEN-P5-02: Form Create (`/trx/penempatan/new`)

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Transaksi › Penempatan Deposito › Buat Baru                              │
├───────────────────────────────────────────────────────┬──────────────────────────────┤
│ AREA FORM (kiri, ~65%)                                │ SIDEBAR KANAN (~35%)         │
│                                                       │                              │
│ ┌── SEKSI 1: Instrumen & Counterparty ──────────────┐ │ ┌── SETTLEMENT BALANCE ────┐  │
│ │                                                   │ │ │ Rekening Settlement      │  │
│ │ Instrumen *                                       │ │ │                          │  │
│ │ [Cari dan pilih instrumen...]      [COMBOBOX]     │ │ │ 1234567890               │  │
│ │ ↳ Autocomplete by kode / nama                    │ │ │                          │  │
│ │   Badge PSAK: [AC] [FVOCI] [FVTPL] dll.          │ │ │ Saldo terakhir diketahui:│  │
│ │   Badge status instrumen (harus AKTIF + APPROVED) │ │ │ Rp 3.000.000.000,0000   │  │
│ │                                                   │ │ │ per 2026-06-13           │  │
│ │ Bank Counterparty *                               │ │ │                          │  │
│ │ [Cari dan pilih bank...]           [COMBOBOX]     │ │ │ [!] Perhatian: Nominal   │  │
│ │ ↳ Autocomplete, status AKTIF only                 │ │ │ yang diisi (5 Miliar)    │  │
│ │                                                   │ │ │ melebihi saldo terakhir  │  │
│ │ Periode Buku *                                    │ │ │ yang diketahui (3 Miliar │  │
│ │ [JUNI 2026 (OPEN) ▾]              [SELECT]        │ │ │ per 2026-06-13). Pastikan│  │
│ └───────────────────────────────────────────────────┘ │ │ saldo tersedia sebelum   │  │
│                                                       │ │ submit.                  │  │
│ ┌── SEKSI 2: Nominal & Mata Uang ───────────────────┐ │ │                          │  │
│ │                                                   │ │ │ [Saldo tidak tersedia —  │  │
│ │ Mata Uang *     [IDR ▾]           [SELECT]        │ │ │  pastikan saldo cukup    │  │
│ │                                                   │ │ │  sebelum submit]         │  │
│ │ [IDR selected]  Nominal IDR *                     │ │ │   (tampil jika null)     │  │
│ │ [         5.000.000.000,0000]     [INPUT IDR]     │ │ └──────────────────────────┘  │
│ │                                                   │ │                              │
│ │ [FCY selected]  Nominal FCY *                     │ │ ┌── EIR PREVIEW ───────────┐  │
│ │ [         1.000.000,0000]         [INPUT NUM]     │ │ │ [Hitung EIR Preview]     │  │
│ │ Kurs JISDOR: 15.432,50000000      (auto-load)     │ │ │                          │  │
│ │ Nominal IDR (hasil):              (auto-compute)   │ │ │ (tampil setelah form     │  │
│ │ Rp 15.432.500.000,0000                            │ │ │  lengkap & record ada)   │  │
│ │                                                   │ │ │                          │  │
│ │ Biaya Transaksi (IDR)                             │ │ │ EIR Approx: 5,25000000% │  │
│ │ [              0,0000]            [INPUT IDR]     │ │ │ [!] Estimasi, bukan final│  │
│ │                                                   │ │ │                          │  │
│ └───────────────────────────────────────────────────┘ │ │ Lihat jadwal amortisasi  │  │
│                                                       │ │ [▸ Expand 10 periode]    │  │
│ ┌── SEKSI 3: Tenor & Kupon ─────────────────────────┐ │ └──────────────────────────┘  │
│ │                                                   │ │                              │
│ │ Tanggal Penempatan *                              │ │                              │
│ │ [14 Jun 2026]                     [DATEPICKER]    │ │                              │
│ │                                                   │ │                              │
│ │ Tenor (Bulan) *                                   │ │                              │
│ │ [12]                              [INPUT INT]     │ │                              │
│ │ Jatuh Tempo: 14 Jun 2027          (auto-compute)  │ │                              │
│ │                                                   │ │                              │
│ │ Kupon (%) *                                       │ │                              │
│ │ [5,25000000]                      [INPUT NUM 8dp] │ │                              │
│ │                                                   │ │                              │
│ │ Nomor Referensi Bank                              │ │                              │
│ │ [BCA/DEP/2026/001]               [INPUT TEXT]     │ │                              │
│ └───────────────────────────────────────────────────┘ │                              │
│                                                       │                              │
│ ┌── SEKSI 4: Settlement & Dokumen ──────────────────┐ │                              │
│ │                                                   │ │                              │
│ │ Rekening Settlement                               │ │                              │
│ │ [1234567890]                      [INPUT TEXT]    │ │                              │
│ │                                                   │ │                              │
│ │ Dokumen Kontrak                                   │ │                              │
│ │ [Upload PDF / lampirkan dari library]   [UPLOAD]  │ │                              │
│ │ kontrak_deposito_BCA_Jun2026.pdf [× hapus]        │ │                              │
│ │ (minimal 1 sebelum submit, opsional saat create)  │ │                              │
│ │                                                   │ │                              │
│ │ Catatan                                           │ │                              │
│ │ [textarea max 2000 chars]                         │ │                              │
│ └───────────────────────────────────────────────────┘ │                              │
│                                                       │                              │
│ [FVTPL BANNER — muncul hanya jika instrumen FVTPL]    │                              │
│ ┌───────────────────────────────────────────────────┐ │                              │
│ │  [i] FVTPL: Instrumen ini tidak memerlukan ECL    │ │                              │
│ │  staging atau EIR computation (PSAK 71 §5.5.15).  │ │                              │
│ │  Fair value akan diproses oleh MTM engine (P5-M6).│ │                              │
│ └───────────────────────────────────────────────────┘ │                              │
│                                                       │                              │
│ FORM FOOTER                                           │                              │
│ [Simpan sebagai Konsep]        [Batal]                │                              │
│ ↳ Bukan auto-save. Submit eksplisit.                 │                              │
└───────────────────────────────────────────────────────┴──────────────────────────────┘
```

**Anotasi komponen:**

- Instrumen picker: `<ComboboxAsync>` — panggil `GET /api/v1/master/instrumen?q=...&filter[status]=AKTIF&filter[workflow_status]=APPROVED`. Tampilkan kode + nama + badge PSAK71.
- Nominal IDR: string-based input dengan format ribu-titik + desimal-koma (Bahasa Indonesia format). Dikonversi ke `NUMERIC(20,4)` sebelum dikirim ke API.
- FCY path: jika mata uang bukan IDR, tampilkan field `nominalFcy` + fetch kurs JISDOR otomatis + hitung `nominalIdr` = FCY × kurs (read-only preview).
- Settlement sidebar: sticky saat scroll, fetch `settlementBalanceHint` dari response create/GET detail. Amber warning jika `lastKnownIdr` < `nominalIdr` atau `isStale = true`. Abu-abu jika null.
- EIR Preview sidebar: muncul setelah record DRAFT ada (setelah simpan pertama). Tombol "Hitung EIR Preview" memanggil `GET /eir-preview`. Spinner inline saat loading. FVTPL: panel tidak tampil (bukan error — form menampilkan FVTPL banner yang menjelaskan EIR tidak relevan).
- Jatuh tempo auto-compute: client-side saat `tanggalPenempatan` atau `tenorBulan` berubah, ditampilkan sebagai preview read-only di bawah field tenor.

**Catatan UX:**
- Tidak ada auto-save. Semua field tersimpan hanya saat klik "Simpan sebagai Konsep".
- Toast sukses setelah simpan: "Penempatan PNP-2026-00001 berhasil dibuat. Status: Konsep. Lampirkan dokumen jika belum, lalu submit untuk review." + link "Lihat detail".
- Setelah simpan DRAFT berhasil, redirect ke `/trx/penempatan/[id]` (detail view).

---

### SCREEN-P5-03: Detail View + Workflow Panel (`/trx/penempatan/[id]`)

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Transaksi › Penempatan Deposito › PNP-2026-00001                         │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ STICKY HEADER CARD (selalu visible saat scroll)                                      │
│ ┌────────────────────────────────────────────────────────────────────────────────┐   │
│ │ PNP-2026-00001   BCA — Deposito   Rp 5.000.000.000,0000   [Menunggu Review]   │   │
│ │ Periode: Juni 2026 │ Penempatan: 14 Jun 2026 │ Jatuh Tempo: 14 Jun 2027       │   │
│ │ Kupon: 5,25%  │  EIR: — (belum dihitung)  │  Klasifikasi: [AC]               │   │
│ └────────────────────────────────────────────────────────────────────────────────┘   │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ TAB BAR: [Detail]  [Workflow]  [EIR & Amortisasi]  [Audit Trail]                     │
├────────────────────────────────────────────────────┬─────────────────────────────────┤
│ TAB: DETAIL (default)                              │ ACTION PANEL (kanan, sticky)    │
│                                                    │                                 │
│ ┌── DATA POKOK ─────────────────────────────────┐  │ ┌── AKSI TERSEDIA ────────────┐  │
│ │ Instrumen     │ INST-DEP-001 / Dep BCA        │  │ │                             │  │
│ │ Bank          │ PT Bank Central Asia Tbk       │  │ │ [STATUS STATE MACHINE]      │  │
│ │ Mata Uang     │ IDR                            │  │ │                             │  │
│ │ Nominal IDR   │ Rp 5.000.000.000,0000          │  │ │ ── DRAFT ──                │  │
│ │ Biaya Trx.    │ Rp 0,0000                      │  │ │ [Edit]  [Submit]  [Batalkan]│  │
│ │ Tenor         │ 12 bulan                       │  │ │                             │  │
│ │ Tgl Penempatan│ 14 Juni 2026                   │  │ │ ── PENDING_REVIEW ──        │  │
│ │ Tgl Jatuh Tmp │ 14 Juni 2027                   │  │ │ [Review]  [Tolak]           │  │
│ │ Kupon         │ 5,25000000 %                   │  │ │ (SoD: disembunyikan jika    │  │
│ │ No. Ref. Bank │ BCA/DEP/2026/001               │  │ │ user = maker)               │  │
│ │ Settlement    │ 1234567890                     │  │ │                             │  │
│ │ Catatan       │ Penempatan BCA sesuai RKAP     │  │ │ ── PENDING_APPROVAL ──      │  │
│ └────────────────────────────────────────────────┘  │ │ [Approve] (MFA) [Tolak]     │  │
│                                                    │ │ (SoD: disembunyikan jika    │  │
│ ┌── DOKUMEN TERLAMPIR ──────────────────────────┐  │ │ user = maker/reviewer)      │  │
│ │ [PDF] kontrak_deposito_BCA_Jun2026.pdf         │  │ │                             │  │
│ │       Dilampirkan 14 Jun 2026 09:01 oleh Ahmad │  │ │ ── APPROVED_ACTIVE ──       │  │
│ │ [+ Lampirkan dokumen tambahan]                 │  │ │ [Ajukan Terminasi]          │  │
│ └────────────────────────────────────────────────┘  │ │ (hanya maker; SoD disemb.   │  │
│                                                    │ │ jika bukan maker)           │  │
│                                                    │ │                             │  │
│                                                    │ │ ── TERMINATION_PENDING_ ──  │  │
│                                                    │ │    REVIEW                   │  │
│                                                    │ │ [Review Terminasi] [Tolak]  │  │
│                                                    │ │ (SoD: disemb. jika maker)   │  │
│                                                    │ │                             │  │
│                                                    │ │ ── TERMINATION_PENDING_ ──  │  │
│                                                    │ │    APPROVAL                 │  │
│                                                    │ │ [Approve Terminasi] (MFA)   │  │
│                                                    │ │ [Tolak]                     │  │
│                                                    │ │ (SoD: disemb.)              │  │
│                                                    │ │                             │  │
│                                                    │ │ ── MATURED / TERMINATED ─── │  │
│                                                    │ │ [locked badge + info]       │  │
│                                                    │ └─────────────────────────────┘  │
├────────────────────────────────────────────────────┴─────────────────────────────────┤
│ TAB: WORKFLOW                                                                         │
│                                                                                      │
│ ┌── PenempatanWorkflowPanel ──────────────────────────────────────────────────────┐  │
│ │                                                                                 │  │
│ │  CREATE WORKFLOW                                                                │  │
│ │  ● Maker (selesai)                                                              │  │
│ │    Ahmad Fauzi · 14 Jun 2026 09:00 · "Penempatan BCA sesuai limit portofolio" │  │
│ │    [signature: — ]                                                              │  │
│ │                                                                                 │  │
│ │  ▶ Reviewer (saat ini)                        [CURRENT — highlight amber]      │  │
│ │    Menunggu review dari ROLE-APPR-TR                                            │  │
│ │                                                                                 │  │
│ │  ○ Approver (menunggu)                        [future — abu-abu]               │  │
│ │    Belum ada reviewer yang sign-off                                             │  │
│ │                                                                                 │  │
│ │  ─────────────────────────────────────────────────────────────────────────     │  │
│ │  TERMINATE WORKFLOW (muncul hanya jika dalam state TERMINATION_*)              │  │
│ │                                                                                 │  │
│ │  ● Maker Terminasi (selesai)                                                   │  │
│ │    Ahmad Fauzi · 01 Des 2026 · "Bank counterparty meminta pengembalian dana..."│  │
│ │    Dokumen: [PDF] surat_terminasi_BCA.pdf                                      │  │
│ │                                                                                 │  │
│ │  ▶ Reviewer Terminasi (saat ini)              [CURRENT — highlight ungu]       │  │
│ │    Menunggu review dari ROLE-APPR-TR                                            │  │
│ │                                                                                 │  │
│ │  ○ Approver Terminasi (menunggu)              [future — abu-abu]               │  │
│ └─────────────────────────────────────────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ TAB: EIR & AMORTISASI                                                                │
│                                                                                      │
│ ┌── EIRPreviewSidePanel (di dalam tab) ──────────────────────────────────────────┐  │
│ │                                                                                 │  │
│ │  [DRAFT / PENDING] EIR (Estimasi)       [APPROVED_ACTIVE] EIR Final           │  │
│ │  EIR Approx: 5,25000000 %               EIR Awal: 5,25000000 %               │  │
│ │  [!] Ini estimasi. EIR final dihitung   Carrying Amount Awal:                 │  │
│ │  setelah approve.                        Rp 5.000.000.000,0000                │  │
│ │                                          Dihitung: 14 Jun 2026 16:05          │  │
│ │                                          [Job: EIR_COMPUTE selesai]            │  │
│ │                                                                                 │  │
│ │  [FVTPL] Pesan informatif                                                       │  │
│ │  EIR tidak dihitung untuk instrumen FVTPL. Fair value                          │  │
│ │  diproses oleh MTM engine (P5-M6).                                             │  │
│ │                                                                                 │  │
│ │  ▸ Jadwal Amortisasi — 10 Periode Pertama  [Expand / Collapse]                 │  │
│ │  ┌──────┬────────────┬─────────────┬──────────────┬─────────────────────────┐ │  │
│ │  │Per.  │Tgl Angsuran│Ang. Bunga   │Ang. Pokok    │Carrying Amount          │ │  │
│ │  ├──────┼────────────┼─────────────┼──────────────┼─────────────────────────┤ │  │
│ │  │  1   │14 Jul 2026 │21.875.000,00│0,0000        │5.000.000.000,0000       │ │  │
│ │  │  2   │14 Agu 2026 │21.875.000,00│0,0000        │5.000.000.000,0000       │ │  │
│ │  │ ...  │...         │...          │...           │...                      │ │  │
│ │  │ 10   │14 Mar 2027 │21.875.000,00│0,0000        │5.000.000.000,0000       │ │  │
│ │  └──────┴────────────┴─────────────┴──────────────┴─────────────────────────┘ │  │
│ │  [Lihat jadwal lengkap →] (link ke ecl.amortisasi_schedule setelah approved)   │  │
│ └─────────────────────────────────────────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────────────────────────────────────────┤
│ TAB: AUDIT TRAIL  (hanya ROLE-AUDIT, ROLE-RISK, ROLE-AKUN-CTL)                       │
│                                                                                      │
│  [Hash Chain: Valid ✓]    atau   [Hash Chain: BROKEN — Hubungi IT Security]          │
│                                                                                      │
│  Kronologi event (ASC):                                                              │
│  ┌──────────────────────────────────────────────────────────────────────────────┐   │
│  │ [CREATE] PENEMPATAN.CREATED   14 Jun 09:00   Ahmad Fauzi (ROLE-MAKER-TR)    │   │
│  │   [▸ Detail — only ROLE-AUDIT: before/after JSON]                            │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [ATTACH] PENEMPATAN.DOCUMENT_ATTACHED   14 Jun 09:01   Ahmad Fauzi          │   │
│  │   doc: kontrak_deposito_BCA_Jun2026.pdf                                     │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [SUBMIT] PENEMPATAN.SUBMITTED   14 Jun 10:00   Ahmad Fauzi                  │   │
│  │   Komentar: "Penempatan deposito BCA sesuai limit portofolio"               │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [REVIEW] PENEMPATAN.REVIEWED   14 Jun 14:00   Budi Santoso (ROLE-APPR-TR)   │   │
│  │   Komentar: "Dokumen lengkap, nominal dan tenor sesuai limit"               │   │
│  │   Sig hash: abc123...  (truncated, full di expand)                          │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [APPROVE] PENEMPATAN.APPROVED   14 Jun 16:00   Citra Dewi (ROLE-APPR-TR)    │   │
│  │   Komentar: "Disetujui sesuai RKAP 2026"                                   │   │
│  │   Sig hash: def456...                                                       │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [STAGING] PENEMPATAN.STAGING_INITIAL   14 Jun 16:00   [system]              │   │
│  │   Stage 1 ditetapkan (INITIAL_PLACEMENT, AC)                                │   │
│  ├──────────────────────────────────────────────────────────────────────────────┤   │
│  │ [EIR] PENEMPATAN.EIR_COMPUTED   14 Jun 16:05   [system]                     │   │
│  │   EIR awal: 5,25000000%   Jadwal amortisasi versi 1                         │   │
│  └──────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                      │
│  Footer paginasi: [← Prev]  Page 1 of ~1  [Next →]                                  │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

---

### SCREEN-P5-04: Edit DRAFT (`/trx/penempatan/[id]/edit`)

Layout identik dengan Create form (SCREEN-P5-02) kecuali:
- Judul: "Edit Penempatan — PNP-2026-00001"
- Breadcrumb: Transaksi › Penempatan Deposito › PNP-2026-00001 › Edit
- `kode_transaksi` ditampilkan read-only di header, tidak dapat diubah.
- `instrumen_id` dan `counterparty_bank_id` **tidak dapat diubah** setelah create (displayed read-only, tidak ada picker). Ini mencegah switching instrumen yang sudah linked ke dokumen.
- Field `row_version` disertakan tersembunyi di payload untuk optimistic locking.
- Banner merah + redirect ke detail jika status bukan DRAFT (guard client-side, server tetap enforce).
- Tombol footer: "Simpan Perubahan" dan "Batal" (kembali ke detail tanpa save).
- Toast sukses: "Penempatan PNP-2026-00001 berhasil diperbarui."
- Toast error 409: "Data sudah diubah oleh pengguna lain. Muat ulang halaman dan coba lagi."

---

### SCREEN-P5-05: Inline Action Dialogs (modal — bukan halaman terpisah)

Semua action modals mengikuti pola `ApprovalWithSignature` dari Phase 4.

#### Dialog: Submit untuk Review

```
┌── Submit Penempatan PNP-2026-00001 ─────────────────────────────┐
│                                                                   │
│ Anda akan mengirim penempatan ini ke antrian review.             │
│ Pastikan semua data sudah benar dan minimal 1 dokumen            │
│ kontrak sudah terlampir.                                          │
│                                                                   │
│ Komentar Submit *                                                  │
│ [textarea — wajib, tidak ada batas minimum chars]                │
│                                                                   │
│ [ ] Saya menyatakan data yang diisi akurat dan lengkap.          │
│     (checkbox attest — wajib dicentang)                          │
│                                                                   │
│                             [Batal]  [Submit ke Review]           │
└───────────────────────────────────────────────────────────────────┘
```

#### Dialog: Review (PENDING_REVIEW)

```
┌── Review Penempatan PNP-2026-00001 ─────────────────────────────┐
│                                                                   │
│ Anda akan memberikan persetujuan review untuk penempatan ini.    │
│                                                                   │
│ [SodBlockBanner — muncul jika user = maker, tombol disabled]     │
│ "Anda tidak bisa menjadi reviewer untuk penempatan yang          │
│  Anda buat sendiri (SoD — DEC-017)."                            │
│                                                                   │
│ Komentar Review *                                                  │
│ [textarea — wajib diisi]                                          │
│                                                                   │
│ [ ] Saya menyatakan telah memeriksa kelengkapan dan              │
│     kebenaran data penempatan ini.                               │
│                                                                   │
│                             [Batal]  [Setujui Review]             │
└───────────────────────────────────────────────────────────────────┘
```

#### Dialog: Approve (PENDING_APPROVAL) — MFA Step-Up Required

```
┌── Approve Penempatan PNP-2026-00001 ────────────────────────────┐
│                                                                   │
│ Persetujuan final untuk penempatan ini memerlukan verifikasi     │
│ MFA (DEC-027).                                                    │
│                                                                   │
│ [SodBlockBanner — muncul jika user = maker atau reviewer]        │
│                                                                   │
│ Komentar Approve *                                                 │
│ [textarea — wajib diisi]                                          │
│                                                                   │
│ [ ] Saya menyatakan penempatan ini sesuai dengan batas           │
│     investasi dan RKAP yang berlaku, dan menyetujui              │
│     pemrosesan EIR serta staging ECL otomatis.                  │
│                                                                   │
│                             [Batal]  [Lanjut ke Verifikasi MFA]  │
└───────────────────────────────────────────────────────────────────┘

→ Setelah klik "Lanjut ke Verifikasi MFA":
  MFAStepUpModal muncul (reuse dari Phase 4)
  → user input OTP/TOTP
  → on success: kirim POST /approve dengan X-Step-Up-Token
  → JobProgressPanel muncul untuk EIR_COMPUTE job (jika instrumen AC/FVOCI/POCI)
```

#### Dialog: Tolak (dari PENDING_REVIEW atau PENDING_APPROVAL)

```
┌── Tolak Penempatan PNP-2026-00001 ─────────────────────────────┐
│                                                                   │
│ [!] Penempatan akan dikembalikan ke status Konsep (DRAFT).       │
│     Maker dapat memperbaiki dan submit ulang.                    │
│                                                                   │
│ Alasan Penolakan * (minimal 30 karakter)                          │
│ [textarea — min 30 chars, counter: "12/30"]                      │
│                                                                   │
│ [ ] Saya menyatakan penolakan ini berdasarkan alasan             │
│     yang tertulis di atas.                                        │
│                                                                   │
│                             [Batal]  [Tolak Penempatan]           │
└───────────────────────────────────────────────────────────────────┘
```

#### Dialog: Batalkan DRAFT (Withdraw)

```
┌── Batalkan Penempatan PNP-2026-00001 ──────────────────────────┐
│                                                                   │
│ [!] Tindakan ini tidak dapat dibalik.                            │
│ Penempatan ini akan dibatalkan dan tidak dapat disubmit          │
│ kembali.                                                          │
│                                                                   │
│ Apakah Anda yakin ingin membatalkan penempatan ini?             │
│                                                                   │
│                        [Kembali]  [Ya, Batalkan Sekarang]         │
└───────────────────────────────────────────────────────────────────┘
```

#### Dialog: Ajukan Terminasi (APPROVED_ACTIVE → TERMINATION_PENDING_REVIEW)

```
┌── Ajukan Terminasi PNP-2026-00001 ─────────────────────────────┐
│                                                                   │
│ [!] Terminasi lebih awal dari jatuh tempo memiliki dampak        │
│ finansial material: EIR catch-up, ECL derecognition, dan         │
│ realized gain/loss (per DEC-P5-M1-005).                          │
│                                                                   │
│ Alasan Terminasi * (minimal 30 karakter)                          │
│ [textarea — min 30 chars, counter: "0/30"]                       │
│                                                                   │
│ Dokumen Pendukung (sangat dianjurkan)                            │
│ [Upload atau pilih dari library]                                  │
│                                                                   │
│ [ ] Saya menyatakan terminasi ini berdasarkan alasan             │
│     yang valid dan dokumen pendukung terlampir.                  │
│                                                                   │
│                             [Batal]  [Ajukan Terminasi]           │
└───────────────────────────────────────────────────────────────────┘
```

#### Dialog: Approve Terminasi (TERMINATION_PENDING_APPROVAL) — MFA Required

```
┌── Approve Terminasi PNP-2026-00001 ─────────────────────────────┐
│                                                                   │
│ Persetujuan terminasi final memerlukan verifikasi MFA (DEC-027). │
│                                                                   │
│ [SodBlockBanner — muncul jika user = maker atau terminate_reviewer]│
│                                                                   │
│ Komentar Approve Terminasi *                                       │
│ [textarea — wajib diisi]                                          │
│                                                                   │
│ [ ] Saya menyetujui terminasi dini instrumen ini. Saya memahami   │
│     bahwa EIR catch-up, ECL reversal (jika Stage 2/3), dan       │
│     realized gain/loss akan diproses otomatis (P5-M9).           │
│                                                                   │
│                         [Batal]  [Lanjut ke Verifikasi MFA]       │
└───────────────────────────────────────────────────────────────────┘
```

---

## 3. Component Specifications

### 3.1 Komponen Baru — `<PenempatanStatusBadge>`

Badge teks + ikon untuk 9 status (CANCELLED jadi 9 total dengan DRAFT = Konsep):

| Status (API) | Label (ID) | Warna | Ikon |
|---|---|---|---|
| `DRAFT` | Konsep | Abu-abu (#6B7280) | circle-dashed |
| `PENDING_REVIEW` | Menunggu Review | Amber (#D97706) | clock |
| `PENDING_APPROVAL` | Menunggu Approval | Amber (#D97706) | clock-check |
| `APPROVED_ACTIVE` | Aktif | Hijau (#16A34A) | check-circle |
| `REJECTED` | Ditolak | Merah (#DC2626) | x-circle |
| `CANCELLED` | Dibatalkan | Abu-abu (#9CA3AF) | ban |
| `MATURED` | Jatuh Tempo | Biru (#2563EB) | calendar-check |
| `TERMINATION_PENDING_REVIEW` | Menunggu Review Terminasi | Ungu (#7C3AED) | clock |
| `TERMINATION_PENDING_APPROVAL` | Menunggu Approval Terminasi | Ungu (#7C3AED) | clock-check |
| `TERMINATED` | Diterminasi | Ungu (#6D28D9) | x-octagon |
| `TERMINATION_REJECTED` | Terminasi Ditolak | Merah (#DC2626) | x-circle |

Warna tidak pernah menjadi satu-satunya sinyal: setiap badge juga memiliki label teks dan ikon berbeda. Memenuhi WCAG 2.1 AA contrast minimum 4.5:1 untuk teks putih di atas warna badge.

Status `REJECTED` dan `TERMINATION_REJECTED` bukan status permanen di state machine — ditampilkan hanya saat reject_reason ada dan status adalah DRAFT (kembali dari reject). Badge berubah ke Konsep, tetapi panel workflow menampilkan riwayat penolakan.

### 3.2 Komponen Baru — `<SettlementBalanceHintCard>`

Komponen sidebar yang menampilkan saldo settlement. Tidak memblok submit.

State-state yang ditangani:
1. **Saldo tersedia, cukup**: abu-abu biasa. "Saldo terakhir: Rp X (per tanggal Y)."
2. **Saldo tersedia, kurang**: amber warning. "Perhatian: Nominal (Rp X) melebihi saldo terakhir yang diketahui (Rp Y per tanggal Z). Pastikan saldo tersedia sebelum submit."
3. **Saldo tersedia, stale (> 24 jam)**: amber warning tambahan. "Perhatian: Data saldo mungkin tidak terkini (terakhir diperbarui Z). Konfirmasi saldo sebelum submit."
4. **Saldo tidak tersedia (null)**: abu-abu, info. "Saldo tidak tersedia — pastikan saldo mencukupi sebelum submit."
5. **Settlement account belum diisi**: panel tidak tampil.

```tsx
// Komponen props
interface SettlementBalanceHintCardProps {
  hint: {
    lastKnownIdr: string | null;   // string decimal, bukan number
    asOfDate: string | null;
    isStale: boolean;
    isSufficient: null;             // selalu null Phase 5
  } | null;
  nominalIdr: string;              // string decimal dari form state
}
```

### 3.3 Komponen Baru — `<PenempatanWorkflowPanel>`

Vertical stepper yang menampilkan dua sub-workflow: create workflow dan terminate workflow (jika dalam state TERMINATION_*).

Aturan tampilan:
- Create workflow selalu tampil.
- Terminate workflow tampil hanya jika `workflowStatus` ada dalam `[TERMINATION_PENDING_REVIEW, TERMINATION_PENDING_APPROVAL, TERMINATED, TERMINATION_REJECTED]` atau jika `terminateReason != null` (artinya pernah ada proposal terminate).
- Step yang selesai: ikon check, teks abu-abu, collapsed (klik untuk expand komentar + timestamp + signature hash truncated).
- Step current: highlight, ikon spinner/clock, teks warna primer.
- Step future: ikon lingkaran kosong, teks abu-abu sangat terang.
- Signature hash: tampilkan 8 char pertama + "..." + tombol "Salin" untuk full hash.

### 3.4 Komponen Baru — `<TerminateActionDialog>`

Dialog destructive untuk "Ajukan Terminasi". Extend pola M8 cancel pattern:
- Warning banner merah di atas.
- Textarea alasan terminasi dengan character counter (min 30 char).
- Upload dokumen pendukung (opsional tapi sangat dianjurkan).
- Checkbox attest mandatory.
- Tombol "Ajukan Terminasi" disabled sampai checkbox dicentang dan alasan ≥ 30 char.

### 3.5 Komponen Baru — `<EIRPreviewSidePanel>`

Panel yang menampilkan EIR preview atau EIR final tergantung status penempatan:
- Status DRAFT/PENDING_*: tampilkan `eirAwalApprox` dengan amber badge "Estimasi". Tombol "Hitung ulang".
- Status APPROVED_ACTIVE dan `eirAwal` sudah ada: tampilkan `eirAwal` dengan badge "Final".
- Status APPROVED_ACTIVE dan `eirAwal` null (EIR compute masih berjalan): tampilkan `<JobProgressPanel>` untuk job `eirComputeJobId`.
- FVTPL: tampilkan pesan info, tidak ada EIR.
- Accordion "Jadwal Amortisasi 10 Periode" — collapsed by default.

### 3.6 Komponen yang Digunakan Ulang dari Phase 4

| Komponen | Sumber | Cara digunakan |
|---|---|---|
| `<DataTable>` | M9 PR #82 | List penempatan di SCREEN-P5-01 |
| `<MFAStepUpModal>` | M10 PR #84 | Approve + Terminate Approve flow |
| `<JobProgressPanel>` | M9 PR #82 | EIR compute post-approve + async export |
| `<JSONBTreeView>` | M9 PR #82 | Before/after jsonb di audit trail (ROLE-AUDIT) |
| `<SealWorkflowPanel>` | M10 PR #84 | Adapt jadi `<PenempatanWorkflowPanel>` — bukan identik |
| `<ApprovalWithSignature>` | M10 PR #84 | Base pattern untuk semua action dialog |
| `<SodBlockBanner>` | M10 PR #84 | Review/approve dialog jika SoD violation |
| `lib/notify.ts` | cross-module | Semua toast notifications |

---

## 4. Interaction Specifications

### 4.1 Create Flow (S1)

**Happy path:**
1. User navigasi ke `/trx/penempatan/new`.
2. User mengisi seksi 1–4.
3. Jika mata uang FCY: fetch kurs otomatis saat `tanggalPenempatan` atau `mataUangId` berubah. Tampilkan loading spinner di baris kurs. Error jika kurs tidak tersedia.
4. Jika `settlementAccount` diisi: sidebar settlement balance terupdate saat field blur.
5. User klik "Simpan sebagai Konsep".
6. Submit button disabled + spinner inline. Idempotency-Key di-generate client-side (UUID v4) sebelum request.
7. POST `/api/v1/trx/penempatan-deposito`.
8. Sukses (201): toast hijau spesifik + redirect ke `/trx/penempatan/[id]`.
9. Gagal validasi (400/422): field tertentu highlight merah + inline message (`aria-describedby`) + toast merah persistent dengan error code + traceId.

**Empty state / loading state:**
- Instrumen picker loading: skeleton row di dropdown.
- Kurs loading: "Memuat kurs JISDOR..." di bawah field.
- Form tidak auto-save.

**Error states:**
- `PENEMPATAN_INSTRUMEN_INVALID_KLASIFIKASI`: banner inline di bawah picker instrumen: "Instrumen ini belum memiliki klasifikasi PSAK 71 yang di-approve."
- `PERIODE_CLOSED`: banner di atas form: "Periode buku Juni 2026 sudah di-close. Hubungi Finance Controller."
- `CONFLICT` (409 pada PATCH): toast merah: "Data sudah diubah oleh pengguna lain. Halaman akan dimuat ulang." + auto-reload setelah 3 detik.

### 4.2 Submit Flow (S2, step 1)

1. Dari detail page, user klik "Submit" di action panel.
2. Dialog "Submit untuk Review" muncul.
3. User isi komentar + centang checkbox attest.
4. Klik "Submit ke Review" → POST `/submit`.
5. Sukses: dialog tutup, status badge update, workflow panel step maju. Toast: "Penempatan PNP-2026-00001 berhasil disubmit. Menunggu review dari Treasury Approver."
6. Error (tidak ada dokumen): toast error persisten: "Minimal 1 dokumen kontrak harus dilampirkan sebelum submit."

### 4.3 Review Flow (S2, step 2)

1. Reviewer membuka halaman detail.
2. Jika user = maker_id: action panel menampilkan `<SodBlockBanner>` di tempat tombol Review. Tombol tidak ada.
3. Jika user ≠ maker_id: tombol "Review" tersedia.
4. Klik "Review" → dialog review muncul.
5. User isi komentar + attest + klik "Setujui Review".
6. POST `/review`.
7. Sukses: toast: "Review PNP-2026-00001 berhasil. Menunggu persetujuan Treasury Manager."

### 4.4 Approve Flow (S2, step 3) — MFA Required

1. Approver membuka detail. Jika user = maker atau reviewer: SodBlockBanner muncul.
2. Klik "Approve" → ApprovalDialog muncul.
3. User isi komentar + attest.
4. Klik "Lanjut ke Verifikasi MFA".
5. `<MFAStepUpModal>` muncul.
6. Jika MFA sudah fresh (< 5 menit): skip langsung ke step 7.
7. User selesaikan MFA → POST `/auth/step-up` → terima `X-Step-Up-Token`.
8. POST `/approve` dengan token.
9. Response: `eirComputeJobId` ada (jika AC/FVOCI/POCI) atau null (FVTPL).
10. Sukses: toast: "Penempatan PNP-2026-00001 disetujui. EIR sedang dihitung (lihat progress di bawah)."
11. Jika `eirComputeJobId` ada: `<JobProgressPanel>` muncul di tab EIR. SSE subscribe ke `/api/v1/jobs/{jobId}/stream`.
12. EIR selesai: toast sukses tambahan: "EIR awal PNP-2026-00001 berhasil dihitung: 5,25000000%."
13. FVTPL path: tidak ada EIR progress. Toast: "Penempatan PNP-2026-00001 disetujui. Instrumen FVTPL — tidak ada EIR atau ECL staging yang ditetapkan (PSAK 71 §5.5.15)."

### 4.5 Reject Flow (S2)

1. Reviewer atau Approver klik "Tolak" → dialog tolak muncul.
2. User isi alasan (counter char real-time, min 30).
3. Tombol "Tolak Penempatan" disabled sampai ≥ 30 char + attest.
4. POST `/reject`.
5. Sukses: status kembali DRAFT. Toast: "Penempatan PNP-2026-00001 ditolak. Status kembali ke Konsep." + notifikasi in-app ke maker.

### 4.6 Terminate Flow (S5) — 4-Eyes

**Propose (Maker):**
1. Status APPROVED_ACTIVE, user = maker.
2. Klik "Ajukan Terminasi" → `<TerminateActionDialog>`.
3. User isi alasan (min 30 char) + upload dokumen (opsional) + attest.
4. POST `/terminate`.
5. Sukses: toast: "Proposal terminasi PNP-2026-00001 berhasil diajukan. Menunggu review dari Treasury Approver."

**Review Terminasi (Reviewer, ≠ Maker):**
1. Status TERMINATION_PENDING_REVIEW.
2. Jika user = maker: SodBlockBanner menggantikan tombol.
3. Klik "Review Terminasi" → dialog standard ApprovalWithSignature.
4. POST `/terminate-review`.
5. Sukses: toast: "Review terminasi PNP-2026-00001 berhasil. Menunggu persetujuan akhir."

**Approve Terminasi (Treasury Manager, ≠ Maker, ≠ Reviewer Terminasi) — MFA:**
1. Status TERMINATION_PENDING_APPROVAL.
2. SodBlockBanner jika user = maker atau terminate_reviewer.
3. Dialog approve terminasi → MFA step-up modal → POST `/terminate-approve`.
4. Sukses: toast: "Terminasi PNP-2026-00001 disetujui. Proses derecognition di-queue (P5-M9). EIR catch-up adjustment akan dihitung otomatis."

**Tolak Terminasi (any step):**
1. POST `/terminate-reject` dengan alasan ≥ 30 char.
2. Status kembali APPROVED_ACTIVE.
3. Toast: "Proposal terminasi PNP-2026-00001 ditolak. Instrumen tetap Aktif."

### 4.7 EIR Preview (S3)

1. Di form create (setelah simpan DRAFT pertama) atau di tab EIR detail view.
2. Tombol "Hitung EIR Preview" tersedia jika `nominalIdr`, `kuponPersen`, `tenorBulan` sudah terisi.
3. Klik → GET `/eir-preview`. Spinner inline di panel (bukan full page).
4. Sukses: tampilkan `eirAwalApprox` dengan amber badge "Estimasi" + accordion jadwal 10 periode.
5. FVTPL: pesan info tampil, tidak ada spinner (API return 200 dengan null eirAwal).
6. Error `ERR_CALC_2010`: toast merah: "EIR tidak dapat dihitung: [field] wajib diisi terlebih dahulu."

### 4.8 Audit Trail (S6)

1. Tab "Audit Trail" di detail view.
2. Hanya ROLE-AUDIT, ROLE-RISK, ROLE-AKUN-CTL yang dapat melihat tab ini. ROLE-MAKER-TR dan ROLE-APPR-TR tidak melihat tab (tersembunyi, bukan disabled).
3. Hash chain badge di atas timeline. Hijau = valid, merah = broken + alert.
4. Timeline: chronological ASC. Setiap event: action badge, timestamp, actor, komentar.
5. ROLE-AUDIT: expand untuk before/after JSON → `<JSONBTreeView>`.
6. ROLE-RISK / ROLE-AKUN-CTL: expand tanpa before/after JSON, tanpa IP.
7. Paginasi cursor jika event > 50.

### 4.9 Maturity Job Progress (S5 auto-mature)

Tidak ada aksi UI dari user untuk auto-mature. Namun:
- Job maturity-checker terlihat di `/jobs` page dengan `type = PENEMPATAN_MATURITY_CHECK`.
- Notifikasi in-app badge ke ROLE-MAKER-TR dan ROLE-RISK saat penempatan MATURED.
- Klik notifikasi → detail halaman penempatan yang baru MATURED.
- Status badge berubah ke "Jatuh Tempo" (biru).

---

## 5. Accessibility Checklist

| Requirement | Implementasi |
|---|---|
| WCAG 2.1 AA kontras (min 4.5:1) | Semua warna badge memiliki contrast ratio ≥ 4.5:1. Status selalu memiliki teks label + ikon + warna (tidak hanya warna). |
| Keyboard navigation | Tab order: Filter bar → DataTable → Paginasi → Action bar. Modal: focus trap dalam modal, ESC tutup modal. Form: tab order mengikuti urutan seksi. |
| Screen reader | `aria-live="polite"` untuk update status workflow. `aria-describedby` untuk field errors. Dialog modal: `role="dialog"`, `aria-labelledby`, `aria-modal="true"`. |
| SoD warning accessible | `<SodBlockBanner>` memiliki `role="alert"` + `aria-live="assertive"`. Bukan hanya visual. |
| MFA modal state | `aria-live="assertive"` untuk MFA prompt dan success/error. Focus langsung ke OTP input saat modal muncul. |
| Destructive confirm | Dialog "Batalkan Penempatan" dan "Ajukan Terminasi" memiliki `role="alertdialog"`. Tombol destructive adalah yang kedua (bukan pertama) dalam tab order. |
| Color blind | Badge status punya teks label + ikon berbeda. Settlement warning punya ikon [!] dan teks penjelasan, bukan hanya warna amber. |
| Form labels | Semua label terhubung ke input dengan `htmlFor`. Wajib `*` ditandai secara teks dan `aria-required="true"`. |

---

## 6. Bahasa Indonesia Copy Guide

### Label UI Utama

| Konsep | Label Indonesia | Catatan |
|---|---|---|
| Penempatan Deposito | Penempatan Deposito | Pertahankan nama modul |
| Create | Buat Penempatan | Bukan "Tambah" |
| DRAFT | Konsep | Status badge |
| PENDING_REVIEW | Menunggu Review | |
| PENDING_APPROVAL | Menunggu Approval | Bukan "Menunggu Persetujuan" (keep English "Approval" untuk konsistensi dengan workflow lain) |
| APPROVED_ACTIVE | Aktif | |
| REJECTED (implied dari back to DRAFT) | Ditolak | Hanya tampil di riwayat workflow, bukan status badge |
| CANCELLED | Dibatalkan | |
| MATURED | Jatuh Tempo | |
| TERMINATION_PENDING_REVIEW | Menunggu Review Terminasi | |
| TERMINATION_PENDING_APPROVAL | Menunggu Approval Terminasi | |
| TERMINATED | Diterminasi | Bukan "Dihentikan" atau "Diakhiri" |
| TERMINATION_REJECTED | Terminasi Ditolak | |
| Approve Terminasi | Approve Terminasi | Bukan "Setujui Terminasi" |
| Kupon | Kupon | Rate bunga deposito |
| Tenor | Tenor | Durasi dalam bulan |
| Settlement Account | Rekening Settlement | |
| Kontrak | Dokumen Kontrak | |
| Withdraw | Batalkan | Untuk aksi maker menarik DRAFT |
| Submit | Submit | Tetap Inggris, konsisten dengan module lain |
| Review | Review | Tetap Inggris |

### Format Angka IDR

```
Format tampilan: 1.000.000.000,5000
(ribu dipisah titik, desimal koma — standar ID)

Di export CSV/XLSX: 1000000000.5000
(format numerik standar agar dapat diproses Excel)

Di form input: user ketik dengan titik sebagai separator ribu
Input component melakukan stripping sebelum parse ke decimal
```

### Toast Messages (Template)

| Aksi | Toast |
|---|---|
| Create sukses | "Penempatan PNP-2026-00001 berhasil dibuat. Status: Konsep. Lampirkan dokumen jika belum, lalu submit untuk review." |
| Submit sukses | "Penempatan PNP-2026-00001 berhasil disubmit. Menunggu review dari Treasury Approver." |
| Review sukses | "Review PNP-2026-00001 berhasil. Menunggu persetujuan Treasury Manager." |
| Approve sukses (AC/FVOCI) | "Penempatan PNP-2026-00001 disetujui. EIR sedang dihitung (lihat progress di tab EIR)." |
| Approve sukses (FVTPL) | "Penempatan PNP-2026-00001 disetujui. Instrumen FVTPL — EIR dan ECL staging tidak diterapkan (PSAK 71 §5.5.15)." |
| EIR computed | "EIR awal PNP-2026-00001 berhasil dihitung: 5,25000000%." |
| Reject sukses | "Penempatan PNP-2026-00001 ditolak. Status kembali ke Konsep." |
| Withdraw sukses | "Penempatan PNP-2026-00001 berhasil dibatalkan." |
| Terminate propose sukses | "Proposal terminasi PNP-2026-00001 berhasil diajukan. Menunggu review." |
| Terminate review sukses | "Review terminasi PNP-2026-00001 berhasil. Menunggu persetujuan akhir." |
| Terminate approve sukses | "Terminasi PNP-2026-00001 disetujui. Proses derecognition di-queue (P5-M9)." |
| Terminate reject sukses | "Proposal terminasi PNP-2026-00001 ditolak. Instrumen tetap Aktif." |
| Export sukses (inline) | "Export berhasil: 45 penempatan (filter: Periode Juni 2026) diunduh." |
| Export async queued | "Export lebih dari 10.000 baris sedang diproses. Anda akan diberitahu saat selesai." |
| SOD_VIOLATION (API error) | "Anda tidak bisa menjadi reviewer/approver untuk penempatan yang Anda buat sendiri (SoD — DEC-017)." |
| PERIODE_CLOSED | "Periode buku Juni 2026 sudah di-close. Penempatan tidak dapat diproses. Hubungi Finance Controller untuk re-open." |
| CONFLICT (409) | "Data sudah diubah oleh pengguna lain. Muat ulang halaman dan coba lagi." |
| MFA step-up required | "Tindakan ini memerlukan verifikasi MFA. Silakan selesaikan verifikasi MFA terlebih dahulu." |

---

## 7. Hand-off untuk frontend-engineer-nextjs

### File Structure

```
frontend/src/
├── app/
│   └── trx/
│       └── penempatan/
│           ├── page.tsx                          — SCREEN-P5-01 List
│           ├── new/
│           │   └── page.tsx                      — SCREEN-P5-02 Create
│           └── [id]/
│               ├── page.tsx                      — SCREEN-P5-03 Detail (tabs)
│               └── edit/
│                   └── page.tsx                  — SCREEN-P5-04 Edit DRAFT
├── components/
│   └── blips/
│       └── penempatan/
│           ├── PenempatanStatusBadge.tsx         — komponen baru §3.1
│           ├── SettlementBalanceHintCard.tsx      — komponen baru §3.2
│           ├── PenempatanWorkflowPanel.tsx        — komponen baru §3.3
│           ├── TerminateActionDialog.tsx          — komponen baru §3.4
│           ├── EIRPreviewSidePanel.tsx            — komponen baru §3.5
│           ├── PenempatanForm.tsx                — form create + edit (shared)
│           └── dialogs/
│               ├── SubmitDialog.tsx
│               ├── ReviewDialog.tsx
│               ├── ApproveDialog.tsx
│               ├── RejectDialog.tsx
│               ├── WithdrawDialog.tsx
│               ├── TerminateRequestDialog.tsx
│               ├── TerminateReviewDialog.tsx
│               └── TerminateApproveDialog.tsx
├── lib/
│   ├── api/
│   │   └── penempatan.api.ts                     — semua API calls P5-M1
│   ├── schemas/
│   │   └── penempatan.schema.ts                  — Zod schemas
│   └── stores/
│       └── penempatan.store.ts                   — Zustand store
```

### Required shadcn/ui Components

```
Card, CardHeader, CardContent, CardFooter
Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription
Sheet (untuk side panel EIR di mobile)
Tabs, TabsList, TabsTrigger, TabsContent
Form, FormField, FormItem, FormLabel, FormControl, FormMessage
Select, SelectTrigger, SelectContent, SelectItem
DatePicker (Calendar + Popover)
Textarea
Badge
Separator
Accordion, AccordionItem, AccordionTrigger, AccordionContent
Alert, AlertDescription
Progress (untuk JobProgressPanel)
Tooltip (untuk SoD explanation)
```

### Zustand Store — `penempatan.store.ts`

```typescript
// Zustand store shape (interface saja — implementasi adalah tugas frontend-engineer)
interface PenempatanStore {
  // List state
  listFilters: PenempatanFilters;
  listSort: SortSpec[];
  listCursor: string | null;
  setListFilters: (filters: Partial<PenempatanFilters>) => void;
  clearListFilters: () => void;

  // Detail state (satu record at a time)
  currentPenempatan: PenempatanDeposito | null;
  setCurrentPenempatan: (p: PenempatanDeposito | null) => void;

  // EIR preview state
  eirPreview: EirPreviewResult | null;
  eirPreviewLoading: boolean;
  setEirPreview: (result: EirPreviewResult | null) => void;

  // Active job state (EIR compute, export)
  activeJobs: Record<string, JobState>;  // keyed by jobId
  updateJob: (jobId: string, state: Partial<JobState>) => void;
  clearJob: (jobId: string) => void;
}
```

### Zod Schemas — `penempatan.schema.ts`

Catatan penting: semua field uang dan rate menggunakan `z.string()` bukan `z.number()` untuk menghindari floating-point precision loss. Konversi ke `Decimal` dilakukan di API client sebelum serialisasi ke JSON.

```typescript
// Schema outline — implementasi adalah tugas frontend-engineer
const PenempatanCreateSchema = z.object({
  instrumenId: z.string().uuid("Pilih instrumen yang valid"),
  counterpartyBankId: z.string().uuid("Pilih bank counterparty yang valid"),
  periodeId: z.string().uuid("Pilih periode buku"),
  tanggalPenempatan: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal: YYYY-MM-DD"),
  nominalIdr: z.string().optional(),              // wajib jika IDR
  nominalFcy: z.string().optional(),              // wajib jika FCY
  mataUangId: z.string().uuid(),
  tenorBulan: z.coerce.number().int().min(1, "Tenor harus lebih dari 0 bulan"),
  kuponPersen: z.string().refine(
    (v) => parseFloat(v) >= 0,
    { message: "Kupon tidak boleh negatif" }
  ),
  biayaTransaksiIdr: z.string().default("0.0000"),
  nomorReferensiBankIn: z.string().max(100).optional(),
  settlementAccount: z.string().max(50).optional(),
  catatan: z.string().max(2000).optional(),
  kontrakDocId: z.string().uuid().optional(),
});

const WorkflowCommentSchema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi"),
  signatureMethod: z.literal("JWT_STANDARD"),
});

const RejectCommentSchema = z.object({
  comment: z.string().min(30, "Alasan penolakan minimal 30 karakter"),
  signatureMethod: z.literal("JWT_STANDARD"),
});

const TerminateRequestSchema = z.object({
  terminateReason: z.string().min(30, "Alasan terminasi minimal 30 karakter"),
  dokumenTerminasiId: z.string().uuid().optional(),
  signatureMethod: z.literal("JWT_STANDARD"),
});
```

### API Client — `penempatan.api.ts`

Endpoint mapping:

| Fungsi | Method | Path |
|---|---|---|
| `createPenempatan` | POST | `/api/v1/trx/penempatan-deposito` |
| `listPenempatan` | GET | `/api/v1/trx/penempatan-deposito` |
| `getPenempatan` | GET | `/api/v1/trx/penempatan-deposito/{id}` |
| `updatePenempatan` | PATCH | `/api/v1/trx/penempatan-deposito/{id}` |
| `withdrawPenempatan` | DELETE | `/api/v1/trx/penempatan-deposito/{id}` |
| `submitPenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/submit` |
| `reviewPenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/review` |
| `approvePenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/approve` |
| `rejectPenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/reject` |
| `terminatePenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/terminate` |
| `terminateReviewPenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/terminate-review` |
| `terminateApprovePenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/terminate-approve` |
| `terminateRejectPenempatan` | POST | `/api/v1/trx/penempatan-deposito/{id}/terminate-reject` |
| `eirPreviewPenempatan` | GET | `/api/v1/trx/penempatan-deposito/{id}/eir-preview` |
| `auditTimelinePenempatan` | GET | `/api/v1/trx/penempatan-deposito/{id}/audit-timeline` |

Semua mutating calls wajib menyertakan `Idempotency-Key: uuidv4()` di header. `/approve` dan `/terminate-approve` wajib menyertakan `X-Step-Up-Token`.

### Validation Rules Summary (Client-side, sebelum submit)

| Field | Rule | Message |
|---|---|---|
| instrumenId | required, UUID | "Pilih instrumen yang valid" |
| counterpartyBankId | required, UUID | "Pilih bank counterparty yang valid" |
| periodeId | required, UUID | "Pilih periode buku" |
| tanggalPenempatan | required, ≤ today | "Tanggal penempatan tidak boleh lebih dari hari ini" |
| nominalIdr | required jika IDR, > 0 | "Nominal IDR wajib diisi dan harus lebih dari 0" |
| nominalFcy | required jika FCY, > 0 | "Nominal FCY wajib diisi dan harus lebih dari 0" |
| tenorBulan | required, integer, ≥ 1 | "Tenor harus lebih dari 0 bulan" |
| kuponPersen | required, ≥ 0, 8 desimal | "Kupon tidak boleh negatif" |
| biayaTransaksiIdr | required (default 0), ≥ 0 | "Biaya transaksi tidak boleh negatif" |
| comment (reject) | min 30 char | "Alasan penolakan minimal 30 karakter" |
| terminateReason | min 30 char | "Alasan terminasi minimal 30 karakter" |
| rowVersion (PATCH) | required integer | — (hidden field, tidak perlu pesan) |

### URL State Management (Deep-link)

List page `/trx/penempatan` menyimpan filter + sort di URL dengan `nuqs`:

```
/trx/penempatan
  ?filter[workflow_status]=PENDING_REVIEW,PENDING_APPROVAL
  &filter[counterparty_bank_id]=<uuid>
  &sort=created_at:desc
  &q=BCA
  &limit=50
```

State di-restore dari URL saat halaman dibuka ulang atau di-share.

---

## 8. Anti-Patterns yang Dihindari

| Anti-pattern | Solusi di design ini |
|---|---|
| Auto-save di form workflow | Simpan hanya saat klik tombol eksplisit |
| Modal stacking modal | MFA modal menggantikan action dialog (bukan stack di atas) |
| Toast sebagai satu-satunya konfirmasi untuk aksi irreversible | Confirm dialog dulu, toast hanya sebagai notifikasi setelah selesai |
| Workflow state tersembunyi di belakang tab | Status badge di sticky header selalu visible, tab Workflow di posisi kedua |
| Warna sebagai satu-satunya sinyal status | Setiap badge status memiliki teks + ikon + warna |
| Loading spinner tanpa konteks | Semua spinner disertai label (contoh: "Menghitung EIR...") |
| SoD enforcement hanya di UI | UI menyembunyikan/menonaktifkan, server tetap enforce. SodBlockBanner memberikan penjelasan teks |

---

_Design ini adalah source of truth UX untuk P5-M1. Perubahan yang menyentuh FVTPL handling atau EIR flow harus dikonsultasikan dengan `ifrs9-compliance-reviewer` sebelum implementasi frontend._
