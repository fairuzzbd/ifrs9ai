# Wireframe MSTR-001-LIST — Halaman Daftar Master Data (Pola Generik)

**Screen ID**: MSTR-001-LIST  
**Berlaku untuk**: Semua 16 modul `mst.*` — pilot adalah `mata_uang`  
**Story**: APP-A-MSTR-001, APP-A-MSTR-002  
**Pattern utama**: DataTable (UX §1) — sort + paging + filter + export  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Layout Keseluruhan

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  BLIPS IFRS9   [nav: Dashboard / Master Data / ...] [user badge] [notif 3]    │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  ▸ Master Data  /  Mata Uang                         [breadcrumb, 14px muted] │
│                                                                                │
│  Daftar Mata Uang                                                              │
│  ──────────────────────────────────────────────────────────────                │
│                                                                                │
│  [ACTION BAR]                                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │ [+ Tambah Mata Uang]  (ROLE-AKUN saja)        [Refresh ↺] [Export ▾]   │ │
│  │                                               Terakhir: 10:31 WIB       │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│  [FILTER BAR]                                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │ 🔍 Cari kode, nama, simbol...     [Filter ▾]  [Status ▾] [Aktif ▾]     │ │
│  │                                                                          │ │
│  │ Filter aktif: [Status: APPROVED ×] [Aktif: Ya ×]   [Hapus semua]       │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│  [DATA TABLE]                                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │ Kode ↑↓  │ Nama          │ Simbol │ Des. │ Sumber Kurs │ Status     │ Aksi│ │
│  │──────────┼───────────────┼────────┼──────┼─────────────┼────────────┼─────│ │
│  │ IDR  ↑   │ Rupiah Indon… │ Rp     │  0   │ BI_JISDOR   │ ●APPROVED  │ ••• │ │
│  │ USD      │ Dolar Amerika │ $      │  2   │ BI_JISDOR   │ ●APPROVED  │ ••• │ │
│  │ EUR      │ Euro          │ €      │  2   │ BI_KURS_T…  │ ●APPROVED  │ ••• │ │
│  │ GBP      │ Pound Sterli… │ £      │  2   │ BI_KURS_T…  │ ○DRAFT     │ ••• │ │
│  │ JPY      │ Yen Jepang    │ ¥      │  0   │ BI_KURS_T…  │ ●APPROVED  │ ••• │ │
│  │ SGD      │ Dolar Singap… │ S$     │  2   │ BI_KURS_T…  │ ◑PENDING…  │ ••• │ │
│  │ CHF      │ Franc Swiss   │ Fr     │  2   │ INTERNAL    │ ●APPROVED  │ ••• │ │
│  │          │               │        │      │             │            │     │ │
│  │ ── 7 dari ~8 ──────────────────────────────── Prev 1 / ~1 Next ────  │ │
│  └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## Komponen & Anotasi

### A. Action Bar

```
[+ Tambah Mata Uang]   (shadcn Button variant="default", size="sm")
  → Tampil HANYA jika user punya permission {entity}.create
  → href: /master/mata-uang/new
  → ROLE-AUDIT: tombol ini tidak ada

[Refresh ↺]   (shadcn Button variant="ghost")
  → Reload data tanpa reset filter/sort
  → Tooltip: "Muat ulang data"

[Export ▾]   (shadcn DropdownMenu)
  → Item: "CSV" | "XLSX"
  → CSV: request sync, download langsung
  → XLSX > 10k row: async, muncul JobProgressPanel
  → Selalu respect filter + sort yang aktif
```

**Label copy (ID/EN):**
| ID | EN |
|---|---|
| + Tambah Mata Uang | + Add Currency |
| Refresh | Refresh |
| Export | Export |
| CSV | CSV |
| Excel (XLSX) | Excel (XLSX) |

---

### B. Filter Bar

**Search box**: placeholder "Cari kode, nama, simbol..."
- Text search global, ketik min 2 karakter → debounce 300ms → trigger query
- URL state: `?q=dollar`
- Bersihkan dengan tombol × di dalam input

**Filter ▾ dropdown** (shadcn Popover):
- Filter per kolom lanjut: Sumber Kurs (checkbox: BI_JISDOR / BI_KURS_TENGAH / INTERNAL), Frekuensi (checkbox: HARIAN / INTRA_DAY / BULANAN), Tanggal Mulai Aktif (date range)
- Tombol "Terapkan" + "Reset"

**Status ▾** (shadcn Select, inline cepat):
- Semua Status | DRAFT | PENDING_REVIEW | PENDING_APPROVAL | APPROVED | RETURNED

**Aktif ▾** (shadcn Select, inline cepat):
- Semua | Aktif | Tidak Aktif

**Filter chips**: muncul di bawah filter bar, satu chip per filter aktif. Format: "[Label: Nilai ×]". Klik × hapus satu filter. Tombol "Hapus semua" hapus semuanya. URL deep-link diupdate setiap perubahan.

---

### C. Data Table — Kolom mata_uang

| Kolom header | Field DB | Sortable | Filterable | Format |
|---|---|---|---|---|
| Kode | kode_mata_uang | Ya (default asc) | eq / in | CHAR(3), monospace bold |
| Nama | nama_mata_uang | Ya | - (via search) | truncate 30ch, title tooltip |
| Simbol | simbol | Tidak | Tidak | center, font-bold |
| Des. | decimal_places | Tidak | Tidak | center, angka |
| Sumber Kurs | sumber_kurs_default | Tidak | select (via Filter ▾) | badge outline kecil |
| Status WF | workflow_status | Ya | select (via Status ▾) | badge berwarna (lihat bawah) |
| Aksi | - | Tidak | Tidak | DropdownMenu ••• |

**Catatan kolom generik** (template untuk 15 modul lain):
- Selalu ada: [Kode/ID field], [Nama/Deskripsi], [Status WF], [Dibuat Oleh], [Tgl Dibuat], [Aksi]
- Tambahan per modul: sesuai field bisnis relevan

**Sort indicator**: ikon ↑ (asc) / ↓ (desc) muncul di header yang aktif. Klik header: tidak ada sort → asc → desc → tidak ada. Shift+klik untuk multi-sort (max 3 kolom). Ikon ↑↓ abu-abu di header yang bisa di-sort tapi tidak aktif.

---

### D. Status Badge Workflow

```
Komponen: components/blips/WorkflowStatusBadge.tsx
Props: status: MasterWorkflowState, size?: "sm" | "default"
```

| Status | Label (ID) | Warna background | Warna text | Ikon |
|---|---|---|---|---|
| DRAFT | Draf | slate-100 | slate-700 | ○ lingkaran kosong |
| PENDING_REVIEW | Menunggu Review | amber-100 | amber-800 | ◑ setengah penuh |
| PENDING_APPROVAL | Menunggu Approval | blue-100 | blue-800 | ◑ setengah penuh (biru) |
| PENDING_APPROVAL_2 | Menunggu Approval 2 | purple-100 | purple-800 | ◑ setengah penuh (ungu) |
| APPROVED | Disetujui | green-100 | green-800 | ● bulat penuh |
| RETURNED | Dikembalikan | orange-100 | orange-800 | ↩ ikon return |

Warna **tidak pernah satu-satunya sinyal** — selalu ada teks label + ikon (WCAG 1.4.1).

---

### E. Kolom Aksi (DropdownMenu •••)

Isi menu tergantung permission user + status record:

**ROLE-AKUN (Maker):**
- [Lihat Detail] — selalu ada
- [Edit] — hanya jika status DRAFT atau RETURNED
- [Kirim untuk Review] — hanya jika status DRAFT (langsung submit)
- [Kirim Ulang untuk Review] — hanya jika status RETURNED
- [Hapus] — hanya jika status DRAFT, dengan confirm dialog
- [Riwayat] — selalu ada (link ke /history)

**ROLE-AKUN-CTL (Reviewer/Approver):**
- [Lihat Detail] — selalu ada
- [Review & Setujui] — hanya jika status PENDING_REVIEW dan bukan maker
- [Approve Final] — hanya jika status PENDING_APPROVAL dan bukan maker/reviewer
- [Tolak] — jika status PENDING_REVIEW atau PENDING_APPROVAL, bukan maker
- [Riwayat] — selalu ada

**ROLE-AUDIT:**
- [Lihat Detail]
- [Riwayat]
- (tidak ada aksi mutasi apapun)

Jika menu item disable karena SoD, tampil dengan tooltip penjelasan (tidak disembunyikan, tapi disabled dengan penjelasan).

---

### F. Pagination Footer

```
┌─────────────────────────────────────────────────────────────┐
│  [25 ▾] per halaman    Prev  Page 1 / ~3   Next             │
│                        ←                   →                 │
└─────────────────────────────────────────────────────────────┘
```

- Dropdown limit: 25 / 50 / 100 / 200 (default 50)
- "Page X / ~Y" — tilde (~) menandakan estimasi total (dari EXPLAIN, bukan COUNT)
- Prev disabled di halaman pertama, Next disabled di halaman terakhir
- Cursor-based, tidak ada "lompat ke halaman N"

---

### G. Empty State

```
┌────────────────────────────────────────────────┐
│                                                │
│        [ilustrasi: dokumen kosong]             │
│                                                │
│     Tidak ada mata uang yang cocok             │
│     dengan pencarian "XXXXX"                   │
│                                                │
│          [Hapus pencarian]                     │
│                                                │
└────────────────────────────────────────────────┘
```

Jika filter aktif: tombol "Hapus semua filter". Jika tidak ada filter: "Belum ada data. Klik '+ Tambah Mata Uang' untuk mulai."

---

### H. Loading State

Skeleton rows: 7 baris dengan kolom skeleton (shimmer animation). Tidak ada blank screen. Filter bar dan action bar tetap interaktif saat loading row data.

---

### I. Error State

```
┌────────────────────────────────────────────────┐
│  Gagal memuat data                             │
│  Terjadi kesalahan saat menghubungi server.    │
│  Trace: a1b2c3...                              │
│                                                │
│          [Coba Lagi]                           │
└────────────────────────────────────────────────┘
```

---

## Komponen yang Dipakai (referensi ke components/blips/*)

| Komponen | Sumber | Catatan |
|---|---|---|
| `DataTable` | `components/blips/DataTable.tsx` | Phase 2, sudah ada |
| `WorkflowStatusBadge` | `components/blips/WorkflowStatusBadge.tsx` | **BARU — perlu dibuat** |
| `FilterBar` | `components/blips/FilterBar.tsx` | **BARU — perlu dibuat** |
| `ExportButton` | `components/blips/ExportButton.tsx` | **BARU — perlu dibuat** (wrap async/sync logic) |
| `JobProgressPanel` | `components/blips/JobProgressPanel.tsx` | Phase 2, sudah ada |
| `notify` | `lib/notify.ts` | Phase 2, sudah ada |

---

## URL & Route

| Aksi | URL |
|---|---|
| Halaman list | `/master/mata-uang` |
| Dengan filter aktif | `/master/mata-uang?sort=kode_mata_uang:asc&filter[aktif_flag]=true&q=dollar` |
| Tambah baru | `/master/mata-uang/new` |
| Detail | `/master/mata-uang/[kode]` |
| Riwayat audit | `/master/mata-uang/[kode]/history` |

Pola URL generik untuk modul lain: `/master/[resource]` — kebab-case sesuai path API.
