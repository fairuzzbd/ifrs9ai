# P5-M2 — Jurnal Engine UI Design Specification

**Story Set**: P5-M2
**Modul**: APP-D — Periode Buku, FX, Mapping Jurnal & GL Interface
**Desainer**: uiux-designer
**Tanggal**: 2026-06-15
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M2-jurnal-engine.md`
**Linked API**: `api/openapi/app-d-jurnal-engine.yaml`
**Linked State Machine**: `docs/state-machines/p5-m2-jurnal-engine.md`
**Decisions applied**:
- DEC-P5-M1-002 (27 master event codes)
- DEC-P5-M1-003 (6-eyes regulated vs 4-eyes operational)
- DEC-017 (SoD enforcement: maker ≠ reviewer ≠ approver ≠ approver_2)
- DEC-018 (audit trail append-only, 10+10 tahun retensi)
- DEC-021 (Idempotency-Key wajib)
- DEC-027 (MFA step-up pada approve regulated + approve-2)

---

## 1. Information Architecture

### Sitemap P5-M2

```
Jurnal (side nav group, collapsible)
├── Mapping Jurnal
│   ├── /jrnl/mapping                         — List dashboard semua mapping (S1)
│   ├── /jrnl/mapping/new                     — Form create mapping baru (S1)
│   ├── /jrnl/mapping/[id]                    — Detail + workflow + action panel (S1)
│   └── /jrnl/mapping/[id]/edit               — Edit DRAFT only (S1)
├── Resolver
│   └── /jrnl/resolve                         — Playground preview (S2)
├── Posting Manual
│   └── /jrnl/post                            — Form posting manual ad-hoc (S4)
├── Journal Entries
│   ├── /jrnl/journal-entries                 — List jurnal posted (S5)
│   └── /jrnl/journal-entries/[id]            — Detail + baris D/K + source event (S5)
└── Dead Letter Queue
    ├── /jrnl/dlq                             — DLQ list + monitoring (S6)
    └── /jrnl/dlq/[id]                        — DLQ detail + replay/discard (S6)
```

### Navigasi Side Nav

```
Jurnal
  ▾ Mapping Jurnal
      Semua Mapping       → /jrnl/mapping
      Buat Baru           → /jrnl/mapping/new       (ROLE-AKUN only)
  ─
  Resolver                → /jrnl/resolve            (ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK)
  Posting Manual          → /jrnl/post               (ROLE-AKUN only)
  ─
  Journal Entries         → /jrnl/journal-entries    (semua role kecuali IT-ADMIN)
  ─
  Dead Letter Queue       → /jrnl/dlq                (ROLE-IT-ADMIN, ROLE-AKUN-CTL only)
                                                      [badge merah jika ada FAILED entries]
```

**DLQ Badge**: Global notification badge di top nav bar juga menyala (merah, angka) jika ada
`sys.dlq_jurnal_post` dengan status `FAILED`. Visible untuk ROLE-IT-ADMIN dan ROLE-AKUN-CTL.

---

## 2. Wireframes — 7 Screens

### SCREEN-P5-M2-01: Mapping List Dashboard (`/jrnl/mapping`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Mapping Jurnal                 [+ Buat Mapping] (ROLE-AKUN only)                       │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari event_code / nama_event...]  [Kategori ▾]  [Workflow ▾]  [Status ▾]           │
│ Filter chips: [Kategori: ECL ×]  [Workflow: 6-Mata ×]  [Status: APPROVED_ACTIVE ×]     │
│                                                                             [Clear semua]│
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]   "Diperbarui: 15 Jun 2026, 09:12"      │
├──────────────────┬───────────────┬────────────────┬──────────────────────┬──────────────┤
│ Kode Event ↕     │ Kategori ↕    │ Workflow        │ Status               │ Berlaku Untuk│
├──────────────────┼───────────────┼────────────────┼──────────────────────┼──────────────┤
│ ECL_PEMBENTUKAN  │ ECL           │[◆◆◆◆◆◆] 6-Mata │[hijau] APPROVED      │ AC FVOCI POCI│
│ PENEMPATAN       │ PENEMPATAN    │[◆◆◆◆]   4-Mata │[hijau] APPROVED      │ SEMUA        │
│ MTM_FVOCI        │ MUTASI_MTM    │[◆◆◆◆◆◆] 6-Mata │[abu]   DRAFT         │ FVOCI        │
│ AKRUAL_BUNGA     │ AKRUAL        │[◆◆◆◆]   4-Mata │[amber] Menunggu Rev. │ AC FVOCI POCI│
│ STAGE_MIGRATION  │ STAGE_MIGRAT..│[◆◆◆◆◆◆] 6-Mata │[amber] Menunggu Appr.│ AC FVOCI POCI│
│ JATUH_TEMPO      │ CLOSURE       │[◆◆◆◆]   4-Mata │[hijau] APPROVED      │ SEMUA        │
│ CORRECTION_PE... │ KOREKSI       │[◆◆◆◆]   4-Mata │[hijau] APPROVED      │ SEMUA        │
├──────────────────┴───────────────┴────────────────┴──────────────────────┴──────────────┤
│ Footer: [← Prev]  Hal. 1 dari ~3  [Next →]  Baris: [50 ▾]  Total estimasi: 27          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter |
|---|---|---|---|
| `event_code` | Kode Event | Ya | Text search |
| `nama_event` | Nama Event | Ya | Text search |
| `kategori_event` | Kategori | Ya | Select multi: PENEMPATAN/AKRUAL/ECL/MUTASI_MTM/STAGE_MIGRATION/CLOSURE/REKLASIFIKASI/FX/KOREKSI |
| `workflow_path` | Workflow | Tidak | Select: 4-Mata / 6-Mata |
| `workflow_status` | Status | Ya | Select multi: DRAFT/PENDING_REVIEW/PENDING_APPROVAL/PENDING_APPROVAL_2/APPROVED_ACTIVE/WITHDRAWN |
| `klasifikasi_berlaku` | Berlaku Untuk | Tidak | Select multi: AC/FVOCI/FVTPL/FVOCI_ELECTION/POCI |
| `aktif_flag` | Aktif | Ya | Toggle: Aktif / Non-aktif |
| `updated_at` | Diperbarui | Ya | Date range |

**Status badges (warna + teks + ikon — tidak hanya warna):**

| Status | Warna | Ikon | Label ID |
|---|---|---|---|
| DRAFT | Abu | pencil | Konsep |
| PENDING_REVIEW | Amber | clock | Menunggu Review |
| PENDING_APPROVAL | Amber | check-circle | Menunggu Approval |
| PENDING_APPROVAL_2 | Amber + outline | check-circle-2 | Menunggu Approval Kedua |
| APPROVED_ACTIVE + aktif=true | Hijau solid | shield-check | Aktif |
| APPROVED_ACTIVE + aktif=false | Abu | shield-off | Nonaktif |
| WITHDRAWN | Merah | x-circle | Ditarik |

**WorkflowPathBadge** (komponen baru — §3):
- 4-Mata: 4 chevron (diamond) icons, label "4-Mata" — tooltip: "Workflow 4 Mata: Maker → Reviewer → Approver"
- 6-Mata: 6 chevron icons, label "6-Mata" — tooltip: "Workflow 6 Mata: Maker → Reviewer → Approver → Approver Kedua (ROLE-RISK)"

Warna berbeda tidak menjadi satu-satunya sinyal: 4-Mata = biru, 6-Mata = ungu — keduanya juga punya
ikon berbeda (jumlah diamond) untuk aksesibilitas.

**Row action dropdown (per baris):**
- "Lihat Detail" (semua role dengan `jurnal_mapping.read`)
- "Edit" (hanya ROLE-AKUN, status DRAFT, current_user = maker)
- "Nonaktifkan" (hanya ROLE-AKUN-CTL, status APPROVED_ACTIVE, aktif=true)

---

### SCREEN-P5-M2-02: Mapping Create/Edit Form (`/jrnl/mapping/new` dan `/jrnl/mapping/[id]/edit`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Jurnal > Mapping Jurnal > Buat Mapping Baru                                  │
│ PAGE HEADER: Buat Mapping Jurnal                                    [Simpan Draft] [Batal]│
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  SECTION 1 — Identitas Event                                                             │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ Kode Event *                          Nama Event *                                │   │
│  │ [ECL_PEMBENTUKAN_BARU          ▾]     [___________________________]              │   │
│  │  (EventCodePicker — 27 codes +        max 120 karakter                           │   │
│  │   "Buat kode baru..." option)                                                     │   │
│  │                                                                                   │   │
│  │ Setelah memilih kode:                                                             │   │
│  │  [◆◆◆◆◆◆] Workflow 6 Mata — Event ini adalah REGULATED                          │   │
│  │  (badge auto-appear berdasarkan event_code pilihan)                              │   │
│  │                                                                                   │   │
│  │ Kategori Event *            Trigger Sumber *         Deskripsi                   │   │
│  │ [ECL             ▾]         [SYSTEM_JOB    ▾]        [____________________]      │   │
│  │                                                       max 500 karakter           │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  SECTION 2 — Klasifikasi Yang Berlaku                                                    │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ KlasifikasiCompatibilityChips                                                     │   │
│  │                                                                                   │   │
│  │  [■ AC]  [■ FVOCI]  [□ FVTPL (nonaktif)]  [■ FVOCI_ELECTION]  [■ POCI]         │   │
│  │                                                                                   │   │
│  │  ⚠ FVTPL dinonaktifkan: ECL_PEMBENTUKAN tidak berlaku untuk FVTPL               │   │
│  │    (PSAK 71 §5.5.15 — FVTPL tidak punya ECL)                                    │   │
│  │                                                                                   │   │
│  │  Pilihan: NULL (berlaku untuk SEMUA) — centang hanya jika perlu pembatasan       │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  SECTION 3 — Baris Detail (Template Debit/Kredit)                                       │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ [+ Tambah Baris]                                                                  │   │
│  │                                                                                   │   │
│  │ No  Posisi    Kode Akun                   Sumber Amount       Multiplier  Hapus  │   │
│  │ ─── ───────── ─────────────────────────── ─────────────────── ─────────── ──── │   │
│  │ 1   [D▾] DEBIT [1110-DEP Deposito...    ▾] [nominal_idr     ▾] [1.0000  ] [✕]  │   │
│  │ 2   [K▾] KREDIT[1001-KAS Kas Bank...    ▾] [nominal_idr     ▾] [1.0000  ] [✕]  │   │
│  │ 3   [D▾] DEBIT [____ Cari akun...       ▾] [ecl_amount      ▾] [1.0000  ] [✕]  │   │
│  │     (drag handle ⠿ untuk reorder urutan)                                         │   │
│  │                                                                                   │   │
│  │  Catatan per baris: [__________________________] (opsional)                      │   │
│  │  Klasifikasi Override: [AC ▾] (opsional — kosong = berlaku untuk semua)          │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  SECTION 4 — Balance Preview                                                             │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │  BalancePreviewCard (live update saat baris berubah)                              │   │
│  │                                                                                   │   │
│  │  Total DEBIT  : 2 baris  [Akan dihitung saat posting]                            │   │
│  │  Total KREDIT : 1 baris  [Akan dihitung saat posting]                            │   │
│  │                                                                                   │   │
│  │  ⚠ [BELUM SEIMBANG] — Jumlah baris DEBIT (2) ≠ KREDIT (1)                      │   │
│  │    Pastikan setiap sumber amount DEBIT memiliki pasangan KREDIT yang setara.     │   │
│  │                                                                                   │   │
│  │  (Saat balance terpenuhi: [✓ SUDAH SEIMBANG] badge hijau)                        │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  ACTION ROW: [Simpan Draft]  [Batal]                                                     │
│  (Tidak ada auto-save. Explicit save only.)                                              │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**EventCodePicker** (komponen baru — §3):
- Dropdown searchable (input field di atas list), 27 codes dikelompokkan per kategori:

```
PENEMPATAN
  PENEMPATAN
  JATUH_TEMPO
  PENJUALAN_PENCAIRAN
  RENEWAL_DEPOSITO
  ...
ECL
  ECL_PEMBENTUKAN          [◆◆◆◆◆◆ Regulated]
  ECL_REVERSAL             [◆◆◆◆◆◆ Regulated]
  POCI_DELTA_ECL           [◆◆◆◆◆◆ Regulated]
MUTASI MTM
  MTM_FVTPL                [◆◆◆◆◆◆ Regulated]
  MTM_FVOCI                [◆◆◆◆◆◆ Regulated]
  ...
KOREKSI
  PERIODE_ADJUSTMENT       [◆◆◆◆ Operasional]
  CORRECTION_PERIODE_CLOSED[◆◆◆◆ Operasional]
...
+ Buat kode baru (nama custom, kategori + trigger wajib diisi)
```

Memilih kode yang sudah APPROVED_ACTIVE menampilkan warning:
"Kode ini sudah ter-approve. Membuat mapping baru akan membuat versi baru dan mematikan versi lama setelah approval."

**MappingDetailRowsBuilder annotations:**
- Posisi toggle: "D" (DEBIT) / "K" (KREDIT) — color + text label
- Kode akun: searchable select dari `mst.chart_of_accounts` (filter `aktif_flag=true`)
- Sumber amount: enum select — `nominal_idr`, `ecl_amount`, `mtm_change`, `accrued_interest`, `net_carrying_idr`, `fx_gain_loss`, `premium_discount_amortization`
- Multiplier: numeric input 0–1, default 1.0000
- Drag reorder via drag handle (⠿ icon), Tab/Shift+Tab antar field dalam satu baris
- Minimum 2 baris (1 DEBIT + 1 KREDIT) — submit blocked jika tidak terpenuhi

**BalancePreviewCard logic:**
- Counting rows (bukan nilai karena nilai bergantung runtime amount)
- Check: min 1 DEBIT row + min 1 KREDIT row = "Sudah Seimbang" (template-level check)
- Imbalance ditampilkan sebagai warning amber (blocking submit)
- Aktual balance (Sum DEBIT = Sum KREDIT) divalidasi server-side saat resolver dipanggil

---

### SCREEN-P5-M2-03: Mapping Detail + Workflow (`/jrnl/mapping/[id]`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Jurnal > Mapping Jurnal > ECL_PEMBENTUKAN                                    │
│ STICKY HEADER                                                                            │
│ ┌─────────────────────────────────────────────────────────────────────────────────────┐  │
│ │ ECL_PEMBENTUKAN — Pembentukan Cadangan ECL                                          │  │
│ │ Kategori: ECL  ·  [◆◆◆◆◆◆] Workflow 6 Mata  ·  [amber] Menunggu Approval Kedua    │  │
│ │ Trigger: SYSTEM_JOB  ·  EV-003  ·  Aktif: Ya                                      │  │
│ └─────────────────────────────────────────────────────────────────────────────────────┘  │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ LAYOUT: 2 kolom (65% kiri / 35% kanan)                                                  │
│                                                                                          │
│  KIRI — Konten Detail                  KANAN — Workflow Panel                            │
│  ─────────────────────────────────     ─────────────────────────────────────────         │
│                                                                                          │
│  SECTION: Klasifikasi Yang Berlaku     SixEyesWorkflowPanel                              │
│  ┌───────────────────────────────┐     ┌─────────────────────────────────────────┐       │
│  │ [■ AC] [■ FVOCI] [■ POCI]    │     │                                         │       │
│  │ [□ FVTPL ⚠ tidak berlaku]    │     │  1 ● MAKER (Selesai)                   │       │
│  │ [□ FVOCI_ELECTION n/a]        │     │    ─────────────────────────────        │       │
│  └───────────────────────────────┘     │    Dewi Rahayu (ROLE-AKUN)              │       │
│                                        │    Dibuat: 14 Jun 2026, 08:30           │       │
│  SECTION: Baris Detail Template        │    "Template ECL sesuai PSAK 71 §5.5.8" │       │
│  ┌───────────────────────────────┐     │                                         │       │
│  │ JurnalLinesTable              │     │  2 ● REVIEWER (Selesai)                │       │
│  │                               │     │    ─────────────────────────────        │       │
│  │ No  Posisi  Akun       Src    │     │    Eko Susanto (ROLE-AKUN-CTL)          │       │
│  │ 1   DEBIT   3210-ECL   ecl.. │     │    Review: 14 Jun 2026, 10:15           │       │
│  │ 2   KREDIT  2110-CKPN  ecl.. │     │    "Sudah dicek sesuai COA"             │       │
│  │                               │     │                                         │       │
│  │ Total baris: 2                │     │  3 ● APPROVER 1 (Selesai)              │       │
│  └───────────────────────────────┘     │    ─────────────────────────────        │       │
│                                        │    Eko Susanto (ROLE-AKUN-CTL)          │       │
│  SECTION: Sumber Event                 │    Approve-1: 14 Jun 2026, 14:00        │       │
│  ┌───────────────────────────────┐     │    "Disetujui, lanjut ke ROLE-RISK"     │       │
│  │ Event ID Kode: EV-003         │     │                                         │       │
│  │ Trigger: SYSTEM_JOB           │     │  4 ○ APPROVER 2 (Menunggu)  ←CURRENT  │       │
│  │ Deskripsi: Jurnal ECL...      │     │    ─────────────────────────────        │       │
│  │                               │     │    Diperlukan: ROLE-RISK                │       │
│  │ Versi: 1.0  [Lihat history ▾] │     │    Menunggu Approval Kedua              │       │
│  └───────────────────────────────┘     │                                         │       │
│                                        │  ─────────────────────────────────────  │       │
│  TABS: [Detail] [Riwayat Audit]        │  ACTION PANEL (state-aware)             │       │
│                                        │                                         │       │
│  Tab Riwayat Audit:                    │  Anda: Fajar Hidayat (ROLE-RISK)        │       │
│  WorkflowTimeline (collapse prior)     │                                         │       │
│                                        │  Komentar *                             │       │
│                                        │  [_________________________________]    │       │
│                                        │  [_________________________________]    │       │
│                                        │  (wajib diisi, min 10 karakter)         │       │
│                                        │                                         │       │
│                                        │  [■] Saya telah membaca dan memahami    │       │
│                                        │      template ini (atestasi)            │       │
│                                        │                                         │       │
│                                        │  [Approve Kedua (MFA)]  [Tolak]         │       │
│                                        │  (MFA step-up wajib per DEC-027)        │       │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Action panel state machine (right panel):**

| Status | Actor | Aksi Tersedia |
|---|---|---|
| DRAFT | maker_id = current_user | [Edit] [Submit] [Tarik] |
| DRAFT | bukan maker | View-only, tidak ada action |
| PENDING_REVIEW | ROLE-AKUN-CTL (bukan maker) | [Review & Lanjutkan] [Tolak] |
| PENDING_REVIEW | maker = current_user | SodBlockBanner "Anda tidak bisa mereview submission Anda sendiri." |
| PENDING_APPROVAL (4-eyes) | ROLE-AKUN-CTL (bukan maker, bukan reviewer) | [Approve] [Tolak] — NO MFA (operational) |
| PENDING_APPROVAL (6-eyes) | ROLE-AKUN-CTL (bukan maker, bukan reviewer) | [Approve (MFA)] [Tolak] — MFA wajib |
| PENDING_APPROVAL (any) | bukan ROLE-AKUN-CTL | View-only |
| PENDING_APPROVAL_2 | ROLE-RISK (bukan maker/reviewer/approver) | [Approve Kedua (MFA)] [Tolak] — MFA wajib |
| PENDING_APPROVAL_2 | approver_1 = current_user | SodBlockBanner "Anda tidak bisa menjadi approver kedua setelah approve pertama." |
| APPROVED_ACTIVE | ROLE-AKUN-CTL | [Nonaktifkan] (confirm dialog) |
| APPROVED_ACTIVE | lainnya | View-only |

**SodBlockBanner** (reuse dari P5-M1): Banner kuning di dalam action panel dengan pesan dan ikon kunci. Tombol disabled + tooltip menjelaskan aturan SoD.

**ApprovalWithSignature** (reuse): Komentar mandatory + checkbox atestasi + tombol approve. Untuk approve-2 dan approve regulated, tombol memicu MFAStepUpModal sebelum submit API.

**"Menunggu Approval Kedua" state**: Panel kanan menampilkan langkah 4 dengan outline (tidak solid) dan label "Menunggu". Tombol Approve Kedua muncul hanya untuk ROLE-RISK yang memenuhi SoD. Siapa pun lain yang membuka halaman ini melihat pesan "Menunggu approval dari ROLE-RISK."

**Tolak (Reject)**: Menghasilkan slide-down textarea (min 30 karakter, counter visible). Destructive button merah. Dialog konfirmasi sebelum submit. Status kembali ke DRAFT.

---

### SCREEN-P5-M2-04: Resolver Playground (`/jrnl/resolve`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Playground Resolver                                                                     │
│  Simulasikan output debit/kredit dari mapping yang aktif. Tidak ada posting.            │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ LAYOUT: 2 kolom (45% kiri / 55% kanan)                                                  │
│                                                                                          │
│  KIRI — Input Parameter               KANAN — Output Preview                            │
│  ─────────────────────────────────    ────────────────────────────────────────────       │
│                                                                                          │
│  Kode Event *                         [Preview / output area — kosong saat awal]         │
│  [ECL_PEMBENTUKAN            ▾]                                                         │
│  EventCodePicker (27 codes)           Saat tombol Preview diklik:                        │
│                                       ┌──────────────────────────────────────────┐      │
│  Klasifikasi PSAK 71 *                │ HASIL PREVIEW                            │      │
│  [AC                         ▾]       │ Event: ECL_PEMBENTUKAN  ·  FVOCI         │      │
│  (AC/FVOCI/FVTPL/FVOCI_EL./POCI)     │ Mapping: v3 (APPROVED_ACTIVE)            │      │
│                                       │                                          │      │
│  Instrumen (opsional)                 │ ┌─────────────────────────────────────┐  │      │
│  [─── Cari instrumen... ▾]            │ │JurnalLinesTable                     │  │      │
│                                       │ │                                     │  │      │
│  Periode *                            │ │ No  Pos.   Akun            Amount   │  │      │
│  [Juni 2026              ▾]           │ │ 1   DEBIT  3210-ECL-PEM    500.000  │  │      │
│                                       │ │ 2   KREDIT 2110-CKPN-STG2  500.000  │  │      │
│  Nominal (IDR) *                      │ │                                     │  │      │
│  [500.000,00                ]         │ │ Total DEBIT : 500.000,00            │  │      │
│                                       │ │ Total KREDIT: 500.000,00            │  │      │
│  Mata Uang     FX Rate                │ │ [✓ SUDAH SEIMBANG]                  │  │      │
│  [IDR ▾]       [1,00000000 ]          │ └─────────────────────────────────────┘  │      │
│                                       │                                          │      │
│  Metadata JSON (opsional)             │ Mapping digunakan: ECL_PEMBENTUKAN v3    │      │
│  [{ "ecl_stage": 2,        ]          │ Approved by: Eko Susanto, 14 Jun 2026   │      │
│  [  "delta_ecl": 500000 }  ]          │ [→ Lihat mapping]                        │      │
│                                       └──────────────────────────────────────────┘      │
│  [Preview]                                                                              │
│  (POST /api/v1/jurnal/resolve)         Jika error:                                      │
│  (tidak di-audit per SM §8)            ┌──────────────────────────────────────────┐     │
│                                        │ [✗] JURNAL_KLASIFIKASI_NOT_ELIGIBLE      │     │
│                                        │ ECL_PEMBENTUKAN tidak berlaku untuk FVTPL│     │
│                                        │ Klasifikasi yang berlaku: AC, FVOCI, POCI│     │
│                                        └──────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Interaksi resolver:**
- Klik "Preview" men-disable tombol + spinner inline sampai response kembali
- Response ≤ 100 ms per SLA — tidak ada JobProgressPanel (sync)
- Jika `JURNAL_EVENT_NOT_MAPPED`: show error card dengan instruksi "Buat mapping untuk kode ini di /jrnl/mapping/new"
- Jika `JURNAL_BALANCE_INVARIANT`: show error + "Perbaiki template mapping sebelum menggunakan event ini"
- Preview tidak membuat jurnal, tidak ada Idempotency-Key wajib di endpoint ini
- State URL: `?event_code=ECL_PEMBENTUKAN&klasifikasi=FVOCI` (deep-link friendly)

---

### SCREEN-P5-M2-05: Posting Manual (`/jrnl/post`)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Posting Jurnal Manual                                                                   │
│  Hanya untuk: PERIODE_ADJUSTMENT dan CORRECTION_PERIODE_CLOSED                          │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  SECTION 1 — Detail Jurnal                                                               │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ Kode Event *                    Periode *                                         │   │
│  │ [PERIODE_ADJUSTMENT      ▾]     [Juni 2026 (SOFT_CLOSED)    ▾]                   │   │
│  │ (hanya 2 pilihan tersedia)                                                        │   │
│  │                                                                                   │   │
│  │ Instrumen (opsional — global jika kosong)    Nominal (IDR) *                     │   │
│  │ [─── Cari instrumen... ▾]                    [500.000,00         ]               │   │
│  │                                                                                   │   │
│  │ Narasi *                                                                          │   │
│  │ [____________________________________________________________]                   │   │
│  │  max 500 karakter  (sisa: 487)                                                   │   │
│  │                                                                                   │   │
│  │ Dokumen Pendukung *  (wajib sebelum submit)                                      │   │
│  │ [Upload Dokumen]  atau  drag & drop PDF/XLSX/JPEG (max 10 MB)                   │   │
│  │ ✓ surat_disposisi_FinCon_Jun2026.pdf  [×]                                        │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  SECTION 2 — Preview Baris Debit/Kredit (auto-populate setelah isi nominal)             │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ Preview berdasarkan mapping PERIODE_ADJUSTMENT (APPROVED_ACTIVE):                 │   │
│  │                                                                                   │   │
│  │  No  Posisi  Akun                     Amount (IDR)                               │   │
│  │  1   DEBIT   9020-PENYESUAIAN         500.000,00                                 │   │
│  │  2   KREDIT  9099-CLEARING-ADJ        500.000,00                                 │   │
│  │                                                                                   │   │
│  │  [✓ SUDAH SEIMBANG]  Total: 500.000,00                                           │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  WORKFLOW: 4 Mata — ROLE-AKUN → ROLE-AKUN-CTL                                           │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ Setelah simpan draft, submit ke ROLE-AKUN-CTL untuk approval.                    │   │
│  │ Posting hanya terjadi setelah approval.                                          │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  ACTION ROW: [Simpan Draft]  [Batal]                                                     │
│  (Submit ke review tersedia di halaman detail setelah simpan)                            │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Interaksi posting manual:**
- Preview baris D/K: auto-call POST /resolve (debounced 500ms) setiap kali nominal berubah
- Jika periode HARD_CLOSED: field periode menampilkan error inline "Periode ini sudah hard-closed, posting tidak bisa dilakukan" + tombol simpan disabled
- Submit ke review membutuhkan dokumen — jika tidak ada, tombol Submit disabled dengan tooltip "Upload dokumen pendukung dulu"
- Setelah Simpan Draft: redirect ke halaman detail jurnal (status PENDING_APPROVAL) + toast "Draft jurnal disimpan. Submit ke reviewer untuk melanjutkan."
- Approve oleh ROLE-AKUN-CTL memicu re-resolve server-side (re-validate) sebelum INSERT final

---

### SCREEN-P5-M2-06: Journal Entries List + Detail (`/jrnl/journal-entries` dan `/jrnl/journal-entries/[id]`)

**List (`/jrnl/journal-entries`):**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Journal Entries                                         [Export ▾ CSV / XLSX]          │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari no_jurnal / narasi...]  [Periode ▾]  [Kode Event ▾]  [Status ▾]  [Tgl ─ Tgl] │
│ Filter chips: [Periode: Juni 2026 ×]  [Event: PENEMPATAN ×]      [Clear semua]          │
├──────────────┬──────────────┬─────────────────────┬─────────────┬────────────┬──────────┤
│ No. Jurnal ↕ │ Tgl Posting ↕│ Kode Event ↕        │ Instrumen   │ Total ↕    │ Status   │
├──────────────┼──────────────┼─────────────────────┼─────────────┼────────────┼──────────┤
│JRN-2026-000001│ 01 Jun 2026 │ PENEMPATAN          │ DEP-BCA-001 │ 5.000.000.000│[hijau]  │
│               │             │                     │             │            │ POSTED   │
│JRN-2026-000042│ 10 Jun 2026 │ PERIODE_ADJUSTMENT  │ (global)    │    500.000 │[hijau]   │
│               │             │                     │             │            │ POSTED   │
│JRN-2026-000099│ 14 Jun 2026 │ ECL_PEMBENTUKAN     │ INST-001    │  2.350.000 │[kuning]  │
│               │             │                     │             │            │ PENDING  │
├──────────────┴──────────────┴─────────────────────┴─────────────┴────────────┴──────────┤
│ Footer: [← Prev]  Hal. 1 dari ~57  [Next →]  Baris: [50 ▾]  Total estimasi: 2.847      │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable Journal Entries:**

| ID Kolom | Header | Sort | Filter |
|---|---|---|---|
| `no_jurnal` | No. Jurnal | Ya | Text search |
| `tanggal_posting` | Tgl Posting | Ya | Date range |
| `event_code` | Kode Event | Ya | Select multi (27 options) |
| `instrumen_nama` | Instrumen | Tidak | Search UUID / nama |
| `total_debit` | Total (IDR) | Ya | gte/lte range |
| `status_internal` | Status | Ya | Select: POSTED/REVERSED/PENDING_APPROVAL |
| `periode_label` | Periode | Ya | Select per periode buku |

**Detail (`/jrnl/journal-entries/[id]`):**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Jurnal > Journal Entries > JRN-2026-000001                                   │
│ STICKY HEADER                                                                            │
│  JRN-2026-000001  ·  PENEMPATAN  ·  01 Jun 2026  ·  [hijau] POSTED                     │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                          │
│  CARD HEADER                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ No. Jurnal  : JRN-2026-000001                                                     │   │
│  │ Tgl Posting : 01 Jun 2026                                                         │   │
│  │ Periode     : Juni 2026 (SOFT_CLOSED)                                             │   │
│  │ Kode Event  : PENEMPATAN                                                          │   │
│  │ Mapping     : PENEMPATAN v1  [→ Lihat mapping]                                   │   │
│  │ Instrumen   : DEP-BCA-001 — Deposito BCA  [→ Lihat instrumen]                    │   │
│  │ Narasi      : Penempatan deposito BCA 5 miliar, tenor 3 bulan                    │   │
│  │ Dibuat oleh : SYSTEM_WORKER  ·  01 Jun 2026, 08:35:22                            │   │
│  │ Sumber      : penempatan_deposito → PNP-2026-00001  [→ Lihat penempatan]         │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  BARIS DEBIT / KREDIT                                                                    │
│  ┌───────────────────────────────────────────────────────────────────────────────────┐   │
│  │ JurnalLinesTable                                                                  │   │
│  │                                                                                   │   │
│  │ No  Posisi   Kode Akun  Nama Akun                Debit (IDR)    Kredit (IDR)     │   │
│  │ 1   DEBIT    1110-DEP   Deposito                5.000.000.000                    │   │
│  │ 2   KREDIT   1001-KAS   Kas / Rekening Bank                     5.000.000.000    │   │
│  │ ─────────────────────────────────────────────────────────────────────────────── │   │
│  │           SUBTOTAL                              5.000.000.000   5.000.000.000    │   │
│  │           [✓ SUDAH SEIMBANG]                                                     │   │
│  └───────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                          │
│  TABS: [Detail]  [Riwayat Audit]                                                         │
│                                                                                          │
│  Tab Riwayat Audit:                                                                      │
│  WorkflowTimeline — aksi, aktor, timestamp (satu entri untuk auto-posting)              │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

### SCREEN-P5-M2-07: DLQ Console (`/jrnl/dlq` dan `/jrnl/dlq/[id]`)

**DLQ List (`/jrnl/dlq`):**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Dead Letter Queue                                                                       │
│  ⚠ 3 entri FAILED membutuhkan perhatian                [Refresh ↺]                     │
│  (Hanya ROLE-IT-ADMIN dan ROLE-AKUN-CTL)                                                │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari source_event_id / event_code...]  [Status ▾]  [Kategori Error ▾]  [Tgl ─ Tgl]│
│ Default filter: Status = FAILED, REPLAYING                                               │
│ Filter chips: [Status: FAILED ×]                                  [Clear semua]          │
├─────────────────┬──────────────────┬───────────────────┬───────────────┬────────────────┤
│ Event Source ↕  │ Kode Event       │ Kode Error        │ Percobaan ↕   │ Status         │
├─────────────────┼──────────────────┼───────────────────┼───────────────┼────────────────┤
│penempatan:appr..│ PENEMPATAN       │ JURNAL_EVENT_NOT..│ 1             │[merah] FAILED  │
│DLQ-001          │                  │                   │               │ Lihat ▸ Replay │
├─────────────────┼──────────────────┼───────────────────┼───────────────┼────────────────┤
│mtm:computed     │ MTM_FVOCI        │ JURNAL_PERIODE_C..│ 2             │[merah] FAILED  │
│DLQ-002          │                  │                   │               │ Lihat ▸ Replay │
├─────────────────┼──────────────────┼───────────────────┼───────────────┼────────────────┤
│ecl:charged      │ ECL_PEMBENTUKAN  │ JURNAL_KLASIFIKA..│ 1             │[amber] REPLAY..│
│DLQ-005          │                  │                   │               │ Lihat         │
├─────────────────┴──────────────────┴───────────────────┴───────────────┴────────────────┤
│ Footer: [← Prev]  Hal. 1 dari 1  [Next →]  Baris: [50 ▾]  Total estimasi: 3            │
│                                                                                          │
│  [Tampilkan REPLAYED_OK]  [Tampilkan ABANDONED]  (toggle — default off)                 │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**DLQ Detail (`/jrnl/dlq/[id]`):**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Jurnal > Dead Letter Queue > DLQ-001                                         │
│ STICKY HEADER: DLQ-001  ·  PENEMPATAN  ·  [merah] FAILED  ·  Percobaan: 1              │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ LAYOUT: 2 kolom (60% kiri / 40% kanan)                                                  │
│                                                                                          │
│  KIRI                                         KANAN — DLQActionPanel                    │
│  ─────────────────────────────────            ─────────────────────────────────          │
│                                                                                          │
│  CARD ERROR                                   STATUS: FAILED                             │
│  ┌─────────────────────────────────────┐      ┌─────────────────────────────────────┐   │
│  │ Kode Error: JURNAL_EVENT_NOT_MAPPED  │      │ Percobaan: 1                        │   │
│  │ Pesan:                              │      │ Terakhir: 14 Jun 2026, 08:32        │   │
│  │ "Tidak ada mapping jurnal APPROVED   │      │ Sumber: penempatan:approved         │   │
│  │  untuk event code 'PENEMPATAN'.      │      │                                     │   │
│  │  Pastikan template sudah dibuat      │      │ Solusi yang disarankan:             │   │
│  │  dan di-approve."                    │      │ Buat dan approve mapping untuk      │   │
│  │                                     │      │ PENEMPATAN di /jrnl/mapping/new     │   │
│  │ Sumber Event: penempatan:approved   │      │ [→ Buka halaman mapping]            │   │
│  │ Event Code: PENEMPATAN              │      │                                     │   │
│  │ Instrumen: DEP-BCA-001              │      │ ─────────────────────────────────── │   │
│  │ Periode: Juni 2026                  │      │                                     │   │
│  └─────────────────────────────────────┘      │ [Replay]  (ROLE-IT-ADMIN + CTL)    │   │
│                                               │ [Buang]   (ROLE-IT-ADMIN only)      │   │
│  PAYLOAD JSON                                 │                                     │   │
│  ┌─────────────────────────────────────┐      └─────────────────────────────────────┘   │
│  │ JSONBTreeView (collapsible tree)    │                                                 │
│  │ ▾ payload                           │                                                 │
│  │   eventCode: "PENEMPATAN"           │                                                 │
│  │   sourceEventId: "PNP-2026-00001.."│                                                 │
│  │   amountIdr: 5000000000.0000       │                                                 │
│  │   klasifikasiPsak: "AC"            │                                                 │
│  │   periodeId: "..."                  │                                                 │
│  │   ...                               │                                                 │
│  └─────────────────────────────────────┘                                                 │
│                                                                                          │
│  RIWAYAT PERCOBAAN                                                                       │
│  ┌─────────────────────────────────────┐                                                 │
│  │ 14 Jun 2026, 08:30 — Percobaan 1   │                                                 │
│  │   JURNAL_EVENT_NOT_MAPPED           │                                                 │
│  └─────────────────────────────────────┘                                                 │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**DLQ Replay confirm dialog:**

```
┌──────────────────────────────────────────────────────────────────┐
│  Replay DLQ-001?                                               [×]│
├──────────────────────────────────────────────────────────────────┤
│  Event akan dijalankan ulang menggunakan payload asli:           │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │  eventCode: PENEMPATAN                                   │    │
│  │  amountIdr: 5.000.000.000                                │    │
│  │  periode: Juni 2026                                      │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                   │
│  ⚠ Pastikan kondisi yang menyebabkan kegagalan sudah diperbaiki  │
│    sebelum replay (mis. mapping sudah di-approve, periode open). │
│                                                                   │
│  Jurnal yang sama tidak akan diposting dua kali (idempotency     │
│  dijaga via unique index di jrnl.header).                        │
│                                                                   │
│           [Batal]                [Replay Sekarang]               │
└──────────────────────────────────────────────────────────────────┘
```

**DLQ Discard confirm dialog:**

```
┌──────────────────────────────────────────────────────────────────┐
│  Buang DLQ-003?                                                [×]│
├──────────────────────────────────────────────────────────────────┤
│  ⚠ Discard tidak bisa di-undo.                                   │
│  Entry akan bertatus ABANDONED dan tidak bisa di-replay.        │
│  Record tetap ada untuk keperluan audit trail.                  │
│                                                                   │
│  Alasan Pembuangan *  (min 30 karakter)                          │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │                                                          │    │
│  │                                                          │    │
│  └──────────────────────────────────────────────────────────┘    │
│  [sisa: 30 karakter lagi]                                        │
│                                                                   │
│  [Batal]                              [Buang (Irreversible)]     │
│                                        (tombol merah — disabled  │
│                                         sampai 30 karakter)      │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Component Specifications

### 3.1 Komponen Baru (perlu dibuat)

#### `<EventCodePicker>` — `/components/blips/jurnal/EventCodePicker.tsx`

**Props:**
```tsx
interface EventCodePickerProps {
  value: string | null;
  onChange: (code: string, meta: EventCodeMeta) => void;
  allowCustom?: boolean;         // apakah opsi "Buat kode baru..." tersedia
  disabled?: boolean;
  excludeApproved?: boolean;     // hide kode yang sudah APPROVED_ACTIVE
}

interface EventCodeMeta {
  eventCode: string;
  namaEvent: string;
  kategoriEvent: string;
  triggerSource: string;
  workflowPath: '4-eyes' | '6-eyes';
  klasifikasiAllowed: string[];  // dari matrix routing
  isRegulated: boolean;
}
```

**Perilaku:**
- Input search (searchable dropdown, min 1 karakter untuk trigger search)
- Kode dikelompokkan per `kategoriEvent`
- Setiap item memiliki: kode (bold), nama_event, WorkflowPathBadge
- Kode regulated ditandai `[◆◆◆◆◆◆ 6-Mata]` — warna ungu
- Kode operational: `[◆◆◆◆ 4-Mata]` — warna biru
- Pilih kode yang sudah APPROVED_ACTIVE: tampilkan warning inline
- Aksesibilitas: role="combobox", aria-label="Pilih kode event jurnal", keyboard navigasi list via panah atas/bawah, Enter untuk pilih
- 27 kode tersedia + opsional "Buat kode baru..."

#### `<WorkflowPathBadge>` — `/components/blips/jurnal/WorkflowPathBadge.tsx`

**Props:**
```tsx
interface WorkflowPathBadgeProps {
  path: '4-eyes' | '6-eyes';
  size?: 'sm' | 'md';
  showTooltip?: boolean;
}
```

**Visual:**
- 4-eyes: 4 ikon diamond biru (◆◆◆◆), label "4-Mata", tooltip "Workflow 4 Mata: Maker → Reviewer → Approver"
- 6-eyes: 6 ikon diamond ungu (◆◆◆◆◆◆), label "6-Mata", tooltip "Workflow 6 Mata: Maker → Reviewer → Approver → Approver Kedua (ROLE-RISK)"
- Ikon + teks (warna TIDAK menjadi satu-satunya sinyal — count ikon berbeda)
- aria-label: "Workflow path: [4-Mata / 6-Mata]"

#### `<KlasifikasiCompatibilityChips>` — `/components/blips/jurnal/KlasifikasiCompatibilityChips.tsx`

**Props:**
```tsx
interface KlasifikasiCompatibilityChipsProps {
  selectedEventCode: string | null;
  value: string[];                    // klasifikasi yang dipilih
  onChange: (value: string[]) => void;
  allowNull?: boolean;                // NULL = berlaku untuk semua
}
```

**Perilaku:**
- Multi-select chips: AC, FVOCI, FVTPL, FVOCI_ELECTION, POCI
- Disabled items ditentukan dari matrix routing (§5 state machine doc) berdasarkan `selectedEventCode`
- Contoh: ECL_PEMBENTUKAN → FVTPL disabled (PSAK 71 §5.5.15), tooltip menjelaskan alasan
- Disabled item ditampilkan dengan opacity reduced + ikon kunci + tooltip alasan
- Warning chip muncul di bawah jika ada kombinasi tidak valid
- Jika NULL dibolehkan: toggle "Berlaku untuk SEMUA klasifikasi" di atas chips

#### `<MappingDetailRowsBuilder>` — `/components/blips/jurnal/MappingDetailRowsBuilder.tsx`

**Props:**
```tsx
interface MappingDetailRowsBuilderProps {
  value: DetailRow[];
  onChange: (rows: DetailRow[]) => void;
  disabled?: boolean;
  klasifikasiBerlaku: string[];       // untuk validate klasifikasi_filter per baris
}

interface DetailRow {
  id: string;                         // local key untuk React
  urutan: number;
  dkIndicator: 'DEBIT' | 'KREDIT';
  kodeAkunId: string | null;
  sumberAmount: string;
  multiplier: string;                 // string untuk display "1.0000"
  klasifikasiFilter: string | null;   // optional per-row override
  catatan: string;
}
```

**Perilaku:**
- Repeatable rows dengan drag handle (⠿) untuk reorder — urutan ter-update otomatis
- "Tambah Baris" menambahkan row kosong di akhir
- "Hapus" (✕) per baris — konfirmasi tidak diperlukan (draft state)
- Tab order: drag handle (skip) → posisi → akun → sumber_amount → multiplier → klasifikasi_override → catatan → next row
- Minimum 2 baris (1 DEBIT + 1 KREDIT) — submit diblok jika tidak terpenuhi, error inline

#### `<BalancePreviewCard>` — `/components/blips/jurnal/BalancePreviewCard.tsx`

**Props:**
```tsx
interface BalancePreviewCardProps {
  rows: DetailRow[];
  amountIdr?: string;                 // jika known (resolver playground)
}
```

**Perilaku:**
- Template-level check (saat form baru): min 1 DEBIT row + min 1 KREDIT row
- Resolver playground: jika `amountIdr` diberikan, hitung totalDebit/Kredit actual
- Badge states:
  - `[✓ SUDAH SEIMBANG]` — hijau solid
  - `[⚠ BELUM SEIMBANG]` — amber, pesan "Jumlah DEBIT ≠ KREDIT"
  - `[? Akan divalidasi saat posting]` — abu (saat template, nilai runtime tidak diketahui)

#### `<JurnalLinesTable>` — `/components/blips/jurnal/JurnalLinesTable.tsx`

**Props:**
```tsx
interface JurnalLinesTableProps {
  lines: JurnalLine[];
  showSubtotal?: boolean;
  showBalanceBadge?: boolean;
}

interface JurnalLine {
  urutan: number;
  posisi: 'DEBIT' | 'KREDIT';
  kodeAkun: string;
  namaAkun: string;
  debitAmount: string | null;
  kreditAmount: string | null;
  narrativeLine?: string;
}
```

**Visual:** Tabel dua kolom amount (Debit IDR / Kredit IDR), baris kosong di kolom tidak berlaku,
subtotal row di bawah, BalanceBadge di footer.

#### `<DLQActionPanel>` — `/components/blips/jurnal/DLQActionPanel.tsx`

**Props:**
```tsx
interface DLQActionPanelProps {
  dlqEntry: DLQEntry;
  onReplay: () => Promise<void>;
  onDiscard: (reason: string) => Promise<void>;
  canReplay: boolean;                 // ROLE-IT-ADMIN || ROLE-AKUN-CTL
  canDiscard: boolean;                // ROLE-IT-ADMIN only
}
```

**Perilaku:**
- Replay button: disabled jika status = REPLAYED_OK atau ABANDONED. Click → ReplayConfirmDialog.
- Discard button: disabled jika status = REPLAYED_OK. Click → DiscardConfirmDialog (textarea 30 char min).
- Setelah replay: tombol disabled + status REPLAYING label + spinner
- Setelah replay OK: status badge berubah, tombol Replay disabled, toast sukses

#### `<SixEyesWorkflowPanel>` — `/components/blips/jurnal/SixEyesWorkflowPanel.tsx`

Extension dari `PenempatanWorkflowPanel` (P5-M1). Menerima 4 slots aktor: Maker, Reviewer, Approver-1, Approver-2. Slot yang belum terisi ditampilkan sebagai lingkaran kosong (○) dengan label peran yang dibutuhkan.

**Props:**
```tsx
interface SixEyesWorkflowPanelProps {
  workflowPath: '4-eyes' | '6-eyes';
  currentStatus: MappingWorkflowStatus;
  maker?: WorkflowActor;
  reviewer?: WorkflowActor;
  approver?: WorkflowActor;
  approver2?: WorkflowActor;           // hanya 6-eyes
  currentUserId: string;
  currentUserPermissions: string[];
  onReview?: (comment: string) => Promise<void>;
  onApprove?: (comment: string, mfaToken?: string) => Promise<void>;
  onApprove2?: (comment: string, mfaToken: string) => Promise<void>;
  onReject?: (reason: string) => Promise<void>;
}
```

### 3.2 Komponen Reuse

| Komponen | Asal | Penggunaan di P5-M2 |
|---|---|---|
| `<DataTable>` | components/blips/DataTable.tsx | Mapping list, Journal Entries list, DLQ list |
| `<MFAStepUpModal>` | components/blips/MFAStepUpModal.tsx | Approve regulated + Approve-2 |
| `<SodBlockBanner>` | components/blips/SodBlockBanner.tsx | Workflow panel saat actor sama |
| `<ApprovalWithSignature>` | components/blips/ApprovalWithSignature.tsx | Review, Approve-1, Approve-2 panels |
| `<JSONBTreeView>` | components/blips/JSONBTreeView.tsx | DLQ payload preview |
| `<JobProgressPanel>` | components/blips/JobProgressPanel.tsx | Export async > 10k rows |
| `PenempatanWorkflowPanel` | P5-M1 | Basis SixEyesWorkflowPanel |

---

## 4. Interaction Patterns

### 4.1 SoD Enforcement

- Approve buttons (semua jenis) disembunyikan (display:none) jika user = maker
- Approve-2 button disembunyikan jika user = maker OR reviewer OR approver_1
- SodBlockBanner tampil di action panel sebagai ganti tombol (tidak hanya hidden)
- SodBlockBanner copy: "Anda tidak bisa [review/approve/approve_2] mapping yang Anda [buat/review/approve] sendiri. (DEC-017 Segregasi Tugas)"
- Server-side SoD juga divalidasi — client hanya untuk UX, bukan sole guard

### 4.2 MFA Step-Up Flow

Diperlukan untuk:
- Approve mapping regulated (6-eyes path, step approve-1) — ROLE-AKUN-CTL
- Approve-2 mapping regulated — ROLE-RISK

Flow:
1. User klik tombol [Approve (MFA)] / [Approve Kedua (MFA)]
2. Frontend cek freshness token step-up: jika valid ≤ 5 menit → langsung submit
3. Jika stale: MFAStepUpModal terbuka (TOTP / push / WebAuthn)
4. Setelah MFA berhasil: submit POST /{id}/approve atau /{id}/approve-2 dengan `X-Step-Up-Token` header
5. Jika server return `JURNAL_STEP_UP_REQUIRED`: modal dibuka kembali dengan pesan "Token MFA sudah kedaluwarsa. Verifikasi ulang."

### 4.3 6-Eyes State "Menunggu Approval Kedua"

Setelah ROLE-AKUN-CTL approve-1 untuk mapping regulated:
- Status berubah ke PENDING_APPROVAL_2
- SixEyesWorkflowPanel step 4 berubah dari ○ ke ◔ (partially filled — waiting)
- In-app notification dikirim ke ROLE-RISK: "Template ECL_PEMBENTUKAN menunggu second approval Anda"
- Toast untuk approver-1: "Mapping ECL_PEMBENTUKAN berhasil approve tahap pertama. Menunggu approval kedua dari ROLE-RISK."
- Halaman detail tampilkan banner info biru di atas workflow panel: "Menunggu Approval Kedua — Diteruskan ke ROLE-RISK untuk persetujuan akhir."

### 4.4 DLQ Replay Interaction

1. User klik Replay → ReplayConfirmDialog muncul (tidak modals stacking — dialog hanya satu)
2. Confirm → POST /jurnal/dlq/{id}/replay → 202 Accepted {jobId}
3. Status DLQ berubah ke REPLAYING (instant UI update via optimistic update)
4. Toast info: "DLQ-001 sedang di-replay. Pantau hasilnya di sini."
5. Jika worker sukses: status berubah REPLAYED_OK, toast hijau: "Replay DLQ-001 berhasil. Jurnal JRN-2026-000099 diposting."
6. Jika worker gagal: status kembali FAILED, attempt_count++, toast merah persisten: "Replay DLQ-001 gagal: [pesan error]. Periksa kondisi sebelum retry."

Replay tidak menggunakan JobProgressPanel (operasi ≤ 500ms, cukup status poll atau SSE event tunggal).

### 4.5 DLQ Discard Interaction

1. User klik Buang → DiscardConfirmDialog muncul
2. Textarea reason: disabled tombol "Buang" sampai ≥ 30 karakter (counter real-time "sisa: X karakter")
3. Confirm → POST /jurnal/dlq/{id}/abandon → 200 OK
4. Status berubah ABANDONED, row hilang dari default list (filter FAILED/REPLAYING)
5. Toast: "DLQ-003 berhasil dibuang. Record tetap tersedia di riwayat audit (filter: Dibuang)."

### 4.6 Mapping Deactivate Interaction

1. ROLE-AKUN-CTL klik "Nonaktifkan" dari detail page atau row action
2. Confirm dialog (tidak destructive-red — status tidak dihapus, hanya flag berubah):
   "Nonaktifkan PENEMPATAN_LEGACY? Resolver tidak akan menggunakan template ini hingga diaktifkan kembali."
3. PATCH /{id}/deactivate → 200 OK
4. Toast: "Mapping PENEMPATAN_LEGACY berhasil dinonaktifkan. Resolver tidak akan menggunakan template ini."
5. Badge di header berubah dari "Aktif" (hijau solid) ke "Nonaktif" (abu)

### 4.7 Balance Preview Live Update

- Saat `MappingDetailRowsBuilder` berubah: BalancePreviewCard re-renders setiap kali rows berubah (tidak async)
- Check hanya template-level (apakah ada ≥ 1 DEBIT + ≥ 1 KREDIT row)
- Resolver Playground: BalancePreviewCard menerima actual amounts dari API response
- Actual balance dijaga server-side via CHECK CONSTRAINT `ck_jrnl_balance` (DB-level guarantee)

### 4.8 Klasifikasi Compatibility Live Validation

- Setiap kali EventCodePicker berubah: KlasifikasiCompatibilityChips mendisable opsi yang tidak valid
- Disable dengan tooltip: "FVTPL tidak berlaku untuk ECL_PEMBENTUKAN (PSAK 71 §5.5.15)"
- Jika user sudah memilih klasifikasi yang kemudian tidak valid: chip di-reset otomatis + warning toast
- Matrix enforcement juga di server-side (resolver cek klasifikasi_berlaku array)

### 4.9 Toast Copy (Bahasa Indonesia, spesifik)

| Trigger | Toast | Tipe |
|---|---|---|
| Create mapping sukses | "Mapping ECL_PEMBENTUKAN berhasil dibuat. Status: Konsep. Klik Submit untuk memulai review." | success |
| Submit mapping | "Mapping ECL_PEMBENTUKAN dikirim ke reviewer. Menunggu review dari ROLE-AKUN-CTL." | success |
| Review sukses | "Mapping ECL_PEMBENTUKAN sudah direview. Lanjut ke tahap approval." | success |
| Approve-1 (4-eyes) | "Mapping PENEMPATAN berhasil disetujui dan aktif. Resolver siap menggunakan template ini." | success |
| Approve-1 (6-eyes) | "Mapping ECL_PEMBENTUKAN berhasil approve tahap pertama. Menunggu approval kedua dari ROLE-RISK." | success |
| Approve-2 sukses | "Mapping ECL_PEMBENTUKAN disetujui penuh (6-Mata) dan aktif. Resolver siap menggunakan template ini." | success |
| Reject | "Mapping ECL_PEMBENTUKAN ditolak. Maker akan mendapat notifikasi untuk perbaikan." | warning |
| Nonaktifkan mapping | "Mapping PENEMPATAN_LEGACY berhasil dinonaktifkan. Resolver tidak akan menggunakan template ini." | warning |
| Create jurnal manual | "Draft jurnal disimpan. Submit ke reviewer untuk melanjutkan posting." | success |
| Jurnal manual posted | "Jurnal JRN-2026-000042 berhasil diposting. PERIODE_ADJUSTMENT Juni 2026." | success |
| Replay DLQ sukses | "Replay DLQ-001 berhasil. Jurnal JRN-2026-000099 diposting." | success |
| Replay DLQ gagal | "Replay DLQ-002 gagal: Periode Maret-2026 masih HARD_CLOSED. Selesaikan periode reopen terlebih dahulu." | error (persisten) |
| Discard DLQ | "DLQ-003 berhasil dibuang. Record tetap tersedia di riwayat audit." | warning |
| SOD violation API | "Anda tidak bisa menjadi approver untuk mapping yang Anda buat sendiri. (DEC-017 Segregasi Tugas)" | error (persisten) |
| Export async started | "Export dimulai. Anda akan menerima notifikasi saat file siap diunduh." | info |
| Export done | "Export selesai. [Unduh file] (tersedia 24 jam)" | success |

### 4.10 Empty / Loading / Error States

| Layar | Empty State | Loading State | Error State |
|---|---|---|---|
| Mapping List | Ilustrasi + "Belum ada mapping jurnal. [Buat Mapping Pertama]" | Skeleton rows (5 rows) | Pesan + [Coba lagi] |
| Journal Entries List | Ilustrasi + "Belum ada jurnal diposting untuk periode ini." | Skeleton rows | Pesan + [Coba lagi] |
| DLQ List | Ilustrasi + "Tidak ada entri DLQ." (hijau — sehat) | Skeleton rows | Pesan + [Coba lagi] |
| Resolver output | "(Isi form dan klik Preview untuk melihat hasil)" | Spinner kecil inline | Error card per jenis error |
| Detail halaman | — | Skeleton card (satu blok) | 404 card dengan back button |

---

## 5. Accessibility

### 5.1 WCAG 2.1 AA

- Semua warna status memenuhi contrast ratio ≥ 4.5:1 (teks pada latar)
- Color bukan satu-satunya sinyal: badge status punya teks label + ikon (shield-check, clock, x-circle)
- WorkflowPathBadge: jumlah diamond + label teks + aria-label (tidak hanya warna biru vs ungu)

### 5.2 Keyboard Navigation

- EventCodePicker: role="combobox", panah atas/bawah untuk navigasi list, Enter untuk pilih, Escape tutup
- MappingDetailRowsBuilder: Tab antar field dalam satu row, Tab ke row berikutnya, Shift+Tab mundur
- Drag reorder rows: keyboard alternative via tombol "Pindah Naik" / "Pindah Turun" di setiap row (tampil saat keyboard focus)
- Modal dialogs: focus trap, Escape tutup, focus kembali ke trigger element
- Action buttons: semua reachable via Tab, logical tab order (left-to-right, top-to-bottom)

### 5.3 ARIA

- Field errors: `aria-describedby` pointing ke error message element
- Disabled chips dengan alasan: `aria-disabled="true"`, `title` dan tooltip accessible
- WorkflowPanel steps: `role="list"`, `role="listitem"`, `aria-current="step"` untuk langkah aktif
- Tabel DataTable: `<th scope="col">`, sort buttons dengan `aria-sort="ascending/descending/none"`
- Toast notifications: `role="alert"` untuk error, `role="status"` untuk success
- JSONBTreeView: `role="tree"`, `role="treeitem"`, `aria-expanded`

### 5.4 Screen Reader Copy

- WorkflowPathBadge: "Workflow path: 4 Mata — tiga aktor berbeda diperlukan" / "Workflow path: 6 Mata — empat aktor berbeda diperlukan"
- Status badge: "Status mapping: Menunggu Approval Kedua"
- BalancePreviewCard: "Status balance template: Sudah Seimbang" / "Status balance template: Belum Seimbang — tambahkan baris KREDIT"

---

## 6. Bahasa Indonesia Copy Reference

| Konsep | Label ID | Label EN (export/report) |
|---|---|---|
| Mapping Jurnal Header | Mapping Jurnal | Journal Mapping |
| Detail Mapping | Baris Detail | Mapping Detail Rows |
| Posisi Debit | Posisi Debit | Debit Position |
| Posisi Kredit | Posisi Kredit | Credit Position |
| Resolver | Playground Resolver | Resolver Playground |
| Dead Letter Queue | Dead Letter Queue | Dead Letter Queue |
| Replay | Replay | Replay |
| Abandon/Discard | Buang | Discard |
| Workflow 4-eyes | Workflow 4 Mata | 4-Eyes Workflow |
| Workflow 6-eyes | Workflow 6 Mata | 6-Eyes Workflow |
| Klasifikasi Yang Berlaku | Klasifikasi Yang Berlaku | Applicable Classifications |
| isBalanced=true | Sudah Seimbang | Balanced |
| isBalanced=false | Belum Seimbang | Unbalanced |
| PENDING_APPROVAL_2 | Menunggu Approval Kedua | Awaiting Second Approval |
| APPROVED_ACTIVE | Aktif | Active |
| WITHDRAWN | Ditarik | Withdrawn |
| Regulated | Regulated | Regulated |
| Operational | Operasional | Operational |
| aktif_flag=false | Nonaktif | Inactive |
| Reject reason | Alasan Penolakan | Rejection Reason |
| Discard reason | Alasan Pembuangan | Discard Reason |
| sumber_amount | Sumber Jumlah | Amount Source |
| dk_indicator | Posisi D/K | Debit/Credit Indicator |
| attempt_count | Jumlah Percobaan | Attempt Count |
| no_jurnal | No. Jurnal | Journal Number |
| tanggal_posting | Tgl. Posting | Posting Date |
| narrative | Narasi | Narrative |
| dokumen_pendukung | Dokumen Pendukung | Supporting Document |

---

## 7. Hand-off untuk Frontend Engineer Next.js

### 7.1 File Structure

```
frontend/src/app/jrnl/
├── mapping/
│   ├── page.tsx                    — SCREEN-P5-M2-01 (list)
│   ├── new/
│   │   └── page.tsx               — SCREEN-P5-M2-02 (create form)
│   └── [id]/
│       ├── page.tsx               — SCREEN-P5-M2-03 (detail + workflow)
│       └── edit/
│           └── page.tsx           — Edit form (reuse SCREEN-P5-M2-02 dengan data prefill)
├── resolve/
│   └── page.tsx                   — SCREEN-P5-M2-04 (resolver playground)
├── post/
│   └── page.tsx                   — SCREEN-P5-M2-05 (manual posting form)
├── journal-entries/
│   ├── page.tsx                   — SCREEN-P5-M2-06 list
│   └── [id]/
│       └── page.tsx               — SCREEN-P5-M2-06 detail
└── dlq/
    ├── page.tsx                   — SCREEN-P5-M2-07 DLQ list
    └── [id]/
        └── page.tsx               — SCREEN-P5-M2-07 DLQ detail

frontend/src/components/blips/jurnal/
├── EventCodePicker.tsx
├── WorkflowPathBadge.tsx
├── KlasifikasiCompatibilityChips.tsx
├── MappingDetailRowsBuilder.tsx
├── BalancePreviewCard.tsx
├── JurnalLinesTable.tsx
├── DLQActionPanel.tsx
└── SixEyesWorkflowPanel.tsx

frontend/src/lib/
├── jurnal.api.ts                  — API client (TanStack Query hooks)
├── jurnal.store.ts                — Zustand store (form state, filter state)
└── jurnal.schema.ts               — Zod schemas
```

### 7.2 shadcn/ui Components yang Digunakan

| shadcn component | Digunakan untuk |
|---|---|
| `Card` | Detail sections, preview cards |
| `Dialog` | Confirm dialogs (Replay, Discard, Deactivate) |
| `Sheet` | Filter panel (mobile breakpoint fallback) |
| `Tabs` | Detail + Riwayat Audit |
| `Form` | Create/edit forms (React Hook Form integration) |
| `Select` | Single select (kategori, trigger, status) |
| `Command` | EventCodePicker (combobox searchable) |
| `Popover` | Tooltip-based chip explanations |
| `Badge` | Status badges, workflow path badges |
| `Separator` | Section dividers |
| `Skeleton` | Loading states |
| `ScrollArea` | JSONBTreeView, long form scrollable sections |
| `Alert` | Warning banners (SoD notice, PENDING_APPROVAL_2 banner) |
| `Progress` | BalancePreviewCard visual indicator |
| `Textarea` | Reject reason, Discard reason, Narasi |

### 7.3 Zod Schemas (`jurnal.schema.ts`)

Catatan: Semua nominal uang menggunakan string (bukan number) untuk menjaga presisi sesuai DEC-016.

```ts
// Sketsa schema — implementasi detail di jurnal.schema.ts
const DetailRowSchema = z.object({
  urutan: z.number().int().min(1),
  dkIndicator: z.enum(['DEBIT', 'KREDIT']),
  kodeAkunId: z.string().uuid("Akun tidak valid"),
  sumberAmount: z.enum(['nominal_idr', 'ecl_amount', 'mtm_change',
    'accrued_interest', 'net_carrying_idr', 'fx_gain_loss',
    'premium_discount_amortization']),
  multiplier: z.string().regex(/^\d+\.\d{4}$/).default("1.0000"),
  klasifikasiFilter: z.enum(['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI']).nullable(),
  catatan: z.string().max(500).optional(),
});

const CreateMappingSchema = z.object({
  eventCode: z.string().min(1).max(40).regex(/^[A-Z_]+$/, "Hanya UPPERCASE dan underscore"),
  namaEvent: z.string().min(1).max(120),
  kategoriEvent: z.enum(['PENEMPATAN','AKRUAL','ECL','MUTASI_MTM',
    'STAGE_MIGRATION','CLOSURE','REKLASIFIKASI','FX','KOREKSI']),
  triggerSource: z.enum(['USER_INPUT','SYSTEM_JOB']),
  klasifikasiBerlaku: z.array(z.enum(['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI'])).nullable(),
  deskripsi: z.string().max(500).optional(),
  detailRows: z.array(DetailRowSchema).min(2, "Minimal 2 baris detail"),
}).refine(
  (data) => {
    const hasDebit = data.detailRows.some(r => r.dkIndicator === 'DEBIT');
    const hasKredit = data.detailRows.some(r => r.dkIndicator === 'KREDIT');
    return hasDebit && hasKredit;
  },
  { message: "Template harus memiliki minimal 1 baris DEBIT dan 1 baris KREDIT", path: ["detailRows"] }
);

const ResolverInputSchema = z.object({
  eventCode: z.string().min(1),
  klasifikasiPsak71: z.enum(['AC','FVOCI','FVTPL','FVOCI_ELECTION','POCI']),
  instrumenId: z.string().uuid().optional(),
  periodeId: z.string().uuid(),
  amountIdr: z.string().regex(/^\d+(\.\d{1,4})?$/, "Nominal tidak valid"),
  currency: z.string().length(3).default("IDR"),
  fxRate: z.string().regex(/^\d+\.\d{8}$/).default("1.00000000"),
  metadataJson: z.string().optional(),
});

const ManualPostingSchema = z.object({
  eventCode: z.enum(['PERIODE_ADJUSTMENT', 'CORRECTION_PERIODE_CLOSED']),
  periodeId: z.string().uuid(),
  instrumenId: z.string().uuid().optional(),
  amountIdr: z.string().regex(/^\d+(\.\d{1,4})?$/),
  narasi: z.string().min(1).max(500),
  dokumenDocId: z.string().uuid("Dokumen pendukung wajib sebelum submit").optional(),
});

const RejectSchema = z.object({
  rejectReason: z.string().min(30, "Alasan penolakan minimal 30 karakter"),
});

const DiscardSchema = z.object({
  reason: z.string().min(30, "Alasan pembuangan minimal 30 karakter"),
});
```

### 7.4 API Client Hooks (`jurnal.api.ts`)

Key TanStack Query hooks yang diperlukan:

```ts
// Sketsa hooks — implementasi di jurnal.api.ts
useMappingList(params: MappingListParams)         // GET /jurnal/mapping-headers
useMappingDetail(id: string)                      // GET /jurnal/mapping-headers/{id}
useCreateMapping()                                // POST /jurnal/mapping-headers
useUpdateMapping(id: string)                      // PATCH /jurnal/mapping-headers/{id}
useMappingWorkflow(id: string)                    // POST /{id}/submit|review|approve|approve-2|reject
useMappingDeactivate(id: string)                  // PATCH /{id}/deactivate

useResolvePreview()                               // POST /jurnal/resolve (no cache, always fresh)

useJurnalList(params: JurnalListParams)           // GET /jurnal/header
useJurnalDetail(id: string)                       // GET /jurnal/header/{id}

useManualPost()                                   // POST /jurnal/post
useManualPostWorkflow(id: string)                 // POST /{id}/submit|approve|reject

useDLQList(params: DLQListParams)                 // GET /jurnal/dlq
useDLQDetail(id: string)                          // GET /jurnal/dlq/{id}
useDLQReplay(id: string)                          // POST /jurnal/dlq/{id}/replay
useDLQDiscard(id: string)                         // POST /jurnal/dlq/{id}/abandon
```

### 7.5 Zustand Store (`jurnal.store.ts`)

```ts
// Sketsa store slices
interface JurnalStore {
  // Mapping form state (tidak auto-saved — explicit save)
  mappingDraft: Partial<CreateMappingInput>;
  setMappingDraft: (draft: Partial<CreateMappingInput>) => void;
  clearMappingDraft: () => void;

  // Resolver playground state (persisted di URL search params)
  resolverInput: Partial<ResolverInput>;
  setResolverInput: (input: Partial<ResolverInput>) => void;

  // DLQ badge count (untuk global top bar)
  dlqFailedCount: number;
  setDlqFailedCount: (count: number) => void;

  // Active filters per page (URL-synced via nuqs)
  // Filter state di-handle via nuqs/searchParams, bukan Zustand
}
```

### 7.6 Validation Rules (Checklist untuk Engineer)

Frontend validasi (Zod + React Hook Form):
- [ ] `event_code`: UNIQUE check async via API (debounce 500ms setelah input berhenti)
- [ ] `detailRows`: min 2 rows, cross-field check (≥1 DEBIT + ≥1 KREDIT)
- [ ] `multiplier`: range [0, 1], 4 decimal places
- [ ] `klasifikasi_berlaku`: disabled options per event_code matrix
- [ ] `amountIdr` (manual post): string > "0", format numeric
- [ ] `narasi` (manual post): required, max 500 chars, counter visible
- [ ] `dokumenDocId` (submit): required saat submit (tidak saat simpan draft)
- [ ] `rejectReason` / `discardReason`: min 30 chars, counter visible
- [ ] Periode HARD_CLOSED: disabled di dropdown manual posting + inline error
- [ ] SoD check: disable/hide approve buttons jika user = maker/reviewer/approver (client-side)

Server-side validasi tetap diperlukan sebagai sole guarantee — client validation untuk UX saja.

### 7.7 Permission Checks (Client-side)

| Element | Permission diperlukan | Jika tidak ada |
|---|---|---|
| Tombol "+ Buat Mapping" | `jurnal_mapping.create` | Hidden |
| Tombol Review | `jurnal_mapping.review` AND ≠ maker | Hidden + SodBlockBanner |
| Tombol Approve | `jurnal_mapping.approve` AND ≠ maker AND ≠ reviewer | Hidden |
| Tombol Approve-2 | `jurnal_mapping.approve_2` AND ≠ all previous | Hidden |
| Tombol Nonaktifkan | `jurnal_mapping.approve` | Hidden |
| Menu Posting Manual | `jurnal.post` | Hidden dari side nav |
| Tombol Approve manual | `jurnal.approve` AND ≠ created_by | Hidden |
| DLQ page access | `jurnal.dlq.read` | 403 redirect ke /unauthorized |
| Tombol Replay | `jurnal.dlq.replay` | Disabled |
| Tombol Buang | `jurnal.dlq.abandon` | Hidden |
| Export button | `jurnal_mapping.export` atau `jurnal.export` | Disabled + tooltip |

---

## 8. Anti-pattern Notes

Anti-pattern yang dihindari di design ini:
- **Modals stacking**: Confirm dialogs tidak membuka modal baru dari dalam modal. DLQ detail page + action panel, bukan modal-in-modal.
- **Auto-save**: Tidak ada auto-save di form mapping atau manual posting. User harus eksplisit klik "Simpan Draft".
- **Hiding workflow state**: Status workflow selalu visible di sticky header dan workflow panel — tidak disembunyikan di tab.
- **Toast only confirmation**: Semua aksi irreversible (reject, discard, deactivate) punya confirm dialog SEBELUM action, bukan hanya toast setelah.
- **Color sole signal**: Semua status badges menggunakan warna + teks + ikon. WorkflowPathBadge menggunakan jumlah ikon berbeda.
