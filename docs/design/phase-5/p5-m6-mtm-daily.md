# P5-M6 — MTM Daily UI Design Specification

**Story Set**: P5-M6
**Modul**: APP-D / APP-B — Mark-to-Market Harian + Manual Upload
**Desainer**: uiux-designer
**Tanggal**: 2026-06-18
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M6-mtm-daily.md`
**Linked API**: `api/openapi/app-b-mtm.yaml`
**Linked State Machine**: `docs/state-machines/p5-m6-mtm-daily.md`
**Decisions applied**:
- DEC-016 (NUMERIC(20,8) harga; NUMERIC(20,4) IDR; shopspring/decimal — never float64)
- DEC-017 (4-eyes SoD: uploader ≠ override-approver; enforced server-side)
- DEC-018 (Audit trail append-only, 10+10 tahun retensi)
- DEC-021 (Idempotency-Key wajib di setiap mutating endpoint)
- DEC-022 (Cursor-only pagination)
- DEC-026 (MFA mandatory: ROLE-AKUN-CTL, ROLE-IT-ADMIN)

---

## 1. Screen Inventory

### 1.1 Sitemap P5-M6

```
Mark-to-Market (side nav group — baru di bawah APP-D)
├── /mtm                                    — MTM List DataTable (S1, S2, S3)
├── /mtm/[id]                               — MTM Detail + Override Action Panel (S4)
├── /mtm/upload                             — Manual Upload Form + parse preview (S2)
│   └── /mtm/upload/batch/[batch_id]        — Batch Detail row breakdown (S2)
└── /mtm/alerts/stale-price                 — STALE_PRICE Monitoring Dashboard (S3)

Admin (existing group)
└── /mtm/cron                               — CronTriggerPanel — ROLE-IT-ADMIN only (S1)
    (alternatif: embedded panel di /mtm dengan DOM gating jika ROLE-IT-ADMIN)
```

### 1.2 Navigasi Side Nav

```
Mark-to-Market
  MTM Harian      → /mtm             [badge merah jika PENDING_REVIEW > 0]
  Stale Price     → /mtm/alerts/stale-price   [badge merah jika STALE_PRICE > 0]
  Upload Manual   → /mtm/upload
  ─
Admin (visible ROLE-IT-ADMIN only)
  MTM Cron        → /mtm/cron
```

Badge PENDING_REVIEW dan STALE_PRICE polling setiap 60 detik. Badge menampilkan jumlah baris, bukan dot.

### 1.3 AC Trace — Screen ke Story

| Screen | Route | Persona | Story / AC Tercakup |
|---|---|---|---|
| MTM List | `/mtm` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT | S1-AC1, S1-AC2, S1-AC3, S2-AC1, S3-AC1 |
| MTM Detail | `/mtm/[id]` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT | S1-AC1..2, S4-AC1..4, S5-AC1..4 |
| MTM Upload | `/mtm/upload` | ROLE-AKUN (Maker) | S2-AC1..4 |
| Batch Detail | `/mtm/upload/batch/[batch_id]` | ROLE-AKUN, ROLE-AKUN-CTL, ROLE-AUDIT | S2-AC1 |
| Stale Price Alerts | `/mtm/alerts/stale-price` | ROLE-AKUN-CTL, ROLE-RISK | S3-AC1, S3-AC2, S3-AC3 |
| Cron Trigger | `/mtm/cron` | ROLE-IT-ADMIN | S1-AC1, S1-AC4, S3-AC4 |
| PeriodeLockBanner | global, cross-cutting | Semua | S2-AC3, S4 locked |

---

## 2. Status Badge Design — `MtmStatusBadge`

5 status dengan warna, icon, dan label teks. Warna bukan satu-satunya signal (WCAG AA).

| Status | Variant | Icon (lucide) | Label ID | Label EN (export) |
|---|---|---|---|---|
| `AUTO_POSTED` | `default` (slate) | `CheckCircle2` | Auto Diposting | Auto Posted |
| `PENDING_REVIEW` | `warning` (amber) | `Clock` | Menunggu Review | Pending Review |
| `APPROVED` | `success` (green) | `CheckCircle` | Disetujui | Approved |
| `REJECTED` | `destructive` (red) | `XCircle` | Ditolak | Rejected |
| `STALE_PRICE` | `destructive` outline | `AlertTriangle` | Harga Kedaluwarsa | Stale Price |

Badge STALE_PRICE menggunakan outline destructive (border merah, background putih) untuk membedakan dari REJECTED (filled merah). Keduanya tetap memiliki teks yang berbeda.

Aksesibilitas: `aria-label="Status MTM: {label}"` pada setiap badge instance.

---

## 3. Screen 1 — MTM List `/mtm`

### 3.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ Mark-to-Market Harian                                          [Upload Manual]   │
│                                                                                  │
│ [Filter Aktif: Status: PENDING_REVIEW ×]  [Clear All]    [Refresh]  [Export ▾] │
│ Cari instrumen...  [Tanggal ▾] [Status ▾] [Klasifikasi ▾] [Sumber ▾] [+ Filter]│
├──────────────────────────────────────────────────────────────────────────────────┤
│ [PeriodeLockBanner — muncul jika periode CLOSED]                                 │
├──────────────────────────────────────────────────────────────────────────────────┤
│ Tanggal ↓  Instrumen ↑   Klasifikasi    Sumber   Harga Pasar    Delta     Status │
│──────────────────────────────────────────────────────────────────────────────────│
│ 2026-06-18 OBL-0042      FVOCI DEBT     IBPA     16.006.250     +1.05%   ●Auto   │
│            Obligasi FR0089 2028          IDR      ─ ─ ─          ─ ─     [Jurnal→]│
│──────────────────────────────────────────────────────────────────────────────────│
│ 2026-06-18 OBL-0088      FVTPL          IBPA      9.000.000     -9.09%  ●Review │
│            Obligasi BUMN 2030            IDR      [DEVIATION -9.09%]    [Override]│
│──────────────────────────────────────────────────────────────────────────────────│
│ 2026-06-17 SHM-0099      FVOCI ELEC     BEI                    —       ●Stale   │
│            Saham Telkom TLKM             IDR      [STALE 4 hari]        [Override]│
│──────────────────────────────────────────────────────────────────────────────────│
│                       Halaman 1 dari ~17  [Prev]  [Next]  Tampilkan: 50 ▾        │
│                       ~847 baris total                                            │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Kolom DataTable

| Kolom | Sortable | Filterable | Format | Lebar |
|---|---|---|---|---|
| Tanggal MTM | ya | date range | `DD MMM YYYY` | 100px |
| Instrumen | ya | text search | `kode (nama, 2 baris)` | 200px |
| Klasifikasi | ya | enum select | `MtmRoutingBadge` (pill kecil) | 110px |
| Sumber | ya | enum multi | `MtmSourceBadge` | 80px |
| Harga Pasar (IDR) | ya | — | `Rp 16.006.250` right-aligned | 130px |
| Delta | ya | — | `+1.05%` / `-9.09%` merah/hijau + `MtmDeviationBadge` jika flag | 130px |
| Umur Harga | ya | — | `0 hari` / `4 hari` + `MtmStaleBadge` jika flag | 90px |
| Status | ya | enum multi | `MtmStatusBadge` | 120px |
| Jurnal | — | — | icon link ke jurnal entry jika tersedia | 50px |
| Aksi | — | — | Kontekstual (lihat §3.3) | 100px |

Default sort: `tanggal_mtm DESC, created_at DESC`.

### 3.3 Kolom Aksi — Kontekstual per Status dan Persona

| Status | ROLE-AKUN | ROLE-AKUN-CTL | ROLE-RISK / AUDIT |
|---|---|---|---|
| `AUTO_POSTED` | — | — | — (klik baris → detail) |
| `PENDING_REVIEW` | — | [Override Setuju] [Tolak] | — |
| `STALE_PRICE` | — | [Override Setuju]* [Tolak] | — |
| `APPROVED` | — | — | — |
| `REJECTED` | — | — | — |

*Override approve untuk STALE_PRICE hanya aktif jika baris sudah punya harga baru via re-upload (status transisi ke PENDING_REVIEW dulu). Tombol [Override Setuju] pada baris STALE_PRICE yang belum di-re-upload ditampilkan dalam state `disabled` dengan tooltip: "Upload harga terbaru terlebih dahulu sebelum menyetujui baris ini."

Tombol di kolom Aksi membuka dialog, tidak navigasi halaman. Klik pada baris di luar kolom Aksi membuka `/mtm/[id]`.

### 3.4 Filter Bar

Filter chip yang aktif muncul di bawah search bar sebagai pills yang dapat dihapus. State filter disimpan di URL query params (`?filter[status]=PENDING_REVIEW&sort=tanggal_mtm:desc`) sehingga deep-link dan bookmark bekerja.

Filter yang tersedia:
- `q` — text search (instrumen kode + nama)
- `filter[tanggal_mtm]` — date picker range
- `filter[status][]` — multi-select checkbox dropdown
- `filter[klasifikasi_psak71]` — single select
- `filter[harga_sumber][]` — multi-select
- `filter[deviation_flag]` — toggle
- `filter[stale_price_flag]` — toggle
- `filter[periode_bulanan_id]` — dropdown periode aktif

### 3.5 Export

Tombol "Export" buka dropdown: CSV / XLSX. Export mengikuti filter + sort aktif.
- Dataset <= 10k baris: inline stream.
- Dataset > 10k baris: 202 Accepted + `<JobProgressPanel>` (UX rule §3). Audit `MTM.EXPORT` per call.

### 3.6 State Khusus

**Empty state (no data, no filter)**: teks "Belum ada data MTM hari ini. Cron berjalan pukul 18:00 WIB atau upload manual." + button [Upload Manual].

**Empty state (filter aktif, no result)**: teks "Tidak ada MTM yang cocok dengan filter ini." + button [Hapus Filter].

**Loading state**: skeleton rows 10 baris, bukan blank.

**PeriodeLockBanner**: reuse komponen dari P5-M4. Muncul di atas tabel jika periode CLOSED. Teks: "Periode [NAMA] sudah hard-closed. Semua baris MTM dalam periode ini dikunci."

---

## 4. Screen 2 — MTM Detail `/mtm/[id]`

### 4.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ ← MTM Harian   OBL-0042 — 18 Juni 2026          ●PENDING_REVIEW [DEVIATION]     │
├─────────────────────────────────┬────────────────────────────────────────────────┤
│ INFORMASI MTM                   │ AKSI OVERRIDE                                  │
│                                 │ (hanya ROLE-AKUN-CTL, status PENDING_REVIEW)   │
│ Instrumen    OBL-0042           │                                                │
│              Obligasi BUMN 2030 │ [DeviationWarningBanner]                       │
│                                 │ Delta 9.09% melebihi threshold 5%.             │
│ Tanggal MTM  2026-06-18         │ Verifikasi harga sebelum menyetujui.           │
│ Periode      Juni 2026          │                                                │
│ Klasifikasi  FVTPL              │ [MtmOverrideApproveDialog trigger]             │
│ Routing      MTM_FVTPL          │ [Override Setuju]                              │
│                                 │                                                │
│ HARGA                           │ [MtmOverrideRejectDialog trigger]              │
│ Sumber       IBPA               │ [Tolak]                                        │
│ Tgl Harga    2026-06-18 (0 hr)  │                                                │
│ Harga Pasar  Rp 9.000.000       │ Diupload oleh                                  │
│   (USD 90.00 × Rp 16.250,00)   │ Siti Akuntansi (ROLE-AKUN)                    │
│ Harga Buku   Rp 9.900.000       │ 2026-06-18 09:15:07                           │
│ Delta IDR    -Rp 900.000        │ Batch: 18c9d0e1-... [Lihat Batch →]           │
│ Delta %      -9.09%             │                                                │
│ Deviation    [DEVIATION -9.09%] │ SoD Note: Anda tidak dapat menyetujui         │
│                                 │ instrumen yang Anda upload sendiri.            │
│ JURNAL (belum ada)              │ (ditampilkan jika caller = uploader)           │
│ Status: Menunggu persetujuan    │                                                │
│                                 │ [PeriodeLockBanner jika locked_flag=TRUE]      │
│ RIWAYAT HARGA (30 hari)         │                                                │
│ [MtmPriceHistoryChart]          │                                                │
│                                 │                                                │
└─────────────────────────────────┴────────────────────────────────────────────────┘
```

### 4.2 Layout Sections

**Kiri (2/3 lebar)**: informasi MTM row lengkap. Grid 2 kolom untuk label-nilai.

**Kanan (1/3 lebar)**: Override Action Panel — hanya render di DOM jika `status IN ('PENDING_REVIEW', 'STALE_PRICE')` DAN caller memiliki permission `mtm.override`. Jika tidak memenuhi syarat, panel kanan tidak ada (bukan disabled — absen dari DOM, kecuali SoD note yang tetap visible jika caller = uploader).

### 4.3 Informasi yang Ditampilkan

**Informasi Instrumen**:
- kode + nama instrumen (link ke `/master/instrumen/[id]`)
- tanggal_mtm, periode_bulanan
- klasifikasi_psak71 snapshot
- `MtmRoutingBadge` — event code(s) yang akan/sudah diposting

**Informasi Harga**:
- harga_sumber (`MtmSourceBadge`)
- harga_tanggal + harga_age_days (`MtmStaleBadge` jika stale)
- harga_pasar_fcy (jika FCY) + kurs_tengah snapshot
- harga_pasar_idr, harga_buku_idr, delta_idr, delta_pct
- `MtmDeviationBadge` jika deviation_flag

**Informasi Jurnal** (collapsed jika null):
- jurnal_entry_id: link ke jurnal detail (P5-M2)
- jurnal_event_code(s)
- Jika null: "Menunggu persetujuan" atau "Ditolak — tidak diposting"

**Riwayat Harga**: `MtmPriceHistoryChart` — Recharts LineChart, harga_pasar_idr 30 hari terakhir untuk instrumen ini. Titik hari ini diberi marker khusus. Sumbu X: tanggal; Sumbu Y: IDR. Tooltip: tanggal + harga + sumber.

**Override History** (jika status APPROVED atau REJECTED):
- override_approver display name
- override_comment (full text)
- override_at timestamp

**Upload Info** (jika manual upload):
- uploader display name + timestamp
- upload_batch_id + link ke batch detail

### 4.4 DeviationWarningBanner

Muncul di Override Action Panel jika `deviation_flag = TRUE`:

```
┌───────────────────────────────────────────────────────┐
│ ! Deviasi Harga Signifikan                            │
│ Delta -9.09% melebihi threshold 5.00%.               │
│ Pastikan harga telah diverifikasi dari sumber primer  │
│ (IBPA / Bloomberg) sebelum menyetujui.               │
└───────────────────────────────────────────────────────┘
```

Background amber-50, border amber-200, teks amber-800. Icon `AlertTriangle`.

---

## 5. Screen 3 — MTM Upload `/mtm/upload`

### 5.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ Upload Harga MTM Manual                                                          │
│ ROLE-AKUN (Maker) · Semua baris masuk status PENDING_REVIEW                     │
├──────────────────────────────────────────────────────────────────────────────────┤
│ [PeriodeLockBanner — jika periode aktif CLOSED]                                  │
│                                                                                  │
│ File Harga MTM *                                                                 │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │                      [Upload icon]                                           │ │
│ │              Drag & drop file XLSX atau CSV di sini                          │ │
│ │                    atau klik untuk memilih file                              │ │
│ │         XLSX atau CSV · Maks 10 MB · Maks 500 baris per batch               │ │
│ │    Kolom: kode_instrumen, tanggal_mtm, harga_pasar, [harga_sumber], [catatan]│ │
│ │    [Unduh Template XLSX]   [Unduh Template CSV]                              │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│ Catatan Upload                                                                   │
│ [Textarea — mis: Upload harga OBL-0088 dari Bloomberg 2026-06-18]               │
│                                                                                  │
│ Override Tanggal MTM (opsional)                                                  │
│ [Date input — default kosong, pakai tanggal dari kolom di file]                 │
│                                                                                  │
│ ! Catatan: Instrumen AC (Amortised Cost) tidak dapat di-MTM per PSAK 71.        │
│   Instrumen FCY memerlukan kurs BI JISDOR hari ini (status APPROVED).           │
│                                                                                  │
│                                              [Batal]  [Upload & Preview]        │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Setelah Upload Sukses — Parse Preview

Setelah `POST /trx/mtm/upload/batch` return 202, tampilkan inline summary (tanpa navigasi halaman):

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ Upload Berhasil — Batch 18c9d0e1                                    [×]          │
│                                                                                  │
│ 3 baris diparse  ·  3 valid  ·  0 error                                         │
│ Status: Semua menunggu review Finance Controller                                 │
│                                                                                  │
│ Peringatan Deviasi:                                                              │
│ ! OBL-0088: delta -7.30% melebihi threshold 5.00%                               │
│   Finance Controller wajib verifikasi sebelum jurnal diposting.                  │
│                                                                                  │
│ ┌──────────────┬────────────┬──────────────┬──────────┬─────────────────────┐   │
│ │ Instrumen    │ Tgl MTM    │ Harga Pasar  │ Delta    │ Status              │   │
│ ├──────────────┼────────────┼──────────────┼──────────┼─────────────────────┤   │
│ │ OBL-0088     │ 2026-06-18 │ Rp 1.584.375 │ -1.52%  │ ●PENDING_REVIEW     │   │
│ │ OBL-0042     │ 2026-06-18 │ Rp 1.462.500 │ -7.30%  │ ●PENDING_REVIEW [!] │   │
│ │ SHM-0015     │ 2026-06-18 │ Rp   185.000 │ +2.78%  │ ●PENDING_REVIEW     │   │
│ └──────────────┴────────────┴──────────────┴──────────┴─────────────────────┘   │
│                                                                                  │
│ [Lihat Detail Batch →]     [Upload Baru]                                         │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Toast sukses: "3 MTM berhasil di-upload untuk 2026-06-18. Status: Menunggu approval Finance Controller."

### 5.3 Skenario Error Upload

**Error per-row (422)**: highlight setiap baris bermasalah dalam preview table dengan ikon merah + pesan inline. Toast error persisten: "Validasi gagal: 1 baris tidak valid — lihat detail di bawah."

**AC instrumen (MTM_INSTRUMEN_AC_SKIP)**: row ditandai merah dengan pesan "DEP-0001 berklasifikasi AC — tidak ada MTM untuk AC per PSAK 71. Hapus baris ini dari file."

**Periode CLOSED (MTM_PERIODE_LOCKED)**: toast merah persisten + PeriodeLockBanner muncul di atas form. Tidak ada perubahan ke input form.

**Kurs FCY tidak tersedia (MTM_PRICE_STALE)**: pesan spesifik per baris: "OBL-0088 (USD) — Kurs USD 2026-06-18 belum tersedia. Upload kurs manual via halaman Kurs terlebih dahulu."

**Idempotency replay (200 IDEMPOTENCY_REPLAY)**: toast info "Request ini sudah diproses sebelumnya. Menampilkan hasil upload sebelumnya." + tampilkan batch preview dari response original.

---

## 6. Screen 4 — Batch Detail `/mtm/upload/batch/[batch_id]`

### 6.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ ← Upload Manual   Detail Batch 18c9d0e1-...                                     │
│                                                                                  │
│ Batch ID   18c9d0e1-f2a3-4567-1234-678901234567                                 │
│ Uploader   Siti Akuntansi (ROLE-AKUN)   2026-06-18 09:15:07                    │
│ Catatan    Upload harga manual OBL-0088 dari Bloomberg 2026-06-18               │
│ Baris      3 diparse · 3 valid · 0 error                                        │
│ Status     Batch PENDING_REVIEW                                                  │
│                                                                                  │
│ ┌──────┬─────────────┬────────────┬──────────────┬────────────┬────────────────┐ │
│ │ Baris│ Instrumen   │ Tgl MTM    │ Harga Pasar  │ Delta      │ Status         │ │
│ ├──────┼─────────────┼────────────┼──────────────┼────────────┼────────────────┤ │
│ │ 1    │ OBL-0088    │ 2026-06-18 │ Rp 1.584.375 │ -1.52%    │ ●PENDING_REVIEW│ │
│ │ 2    │ OBL-0042    │ 2026-06-18 │ Rp 1.462.500 │ -7.30% [!]│ ●PENDING_REVIEW│ │
│ │ 3    │ SHM-0015    │ 2026-06-18 │ Rp   185.000 │ +2.78%    │ ●PENDING_REVIEW│ │
│ └──────┴─────────────┴────────────┴──────────────┴────────────┴────────────────┘ │
│                                                                                  │
│ Klik baris → /mtm/[id] untuk detail lengkap dan aksi override                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Kolom "Baris" = line_number dari file asli. Berguna untuk tracing error kembali ke spreadsheet. Klik baris navigasi ke `/mtm/[id]`.

Untuk ROLE-AKUN: hanya bisa lihat batch milik sendiri. ROLE-AKUN-CTL dan ROLE-AUDIT bisa lihat semua batch.

---

## 7. Screen 5 — STALE_PRICE Alerts `/mtm/alerts/stale-price`

### 7.1 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ Peringatan Harga Kedaluwarsa (Stale Price)                     [Refresh]        │
│ Harga yang tidak diperbarui selama lebih dari 5 hari.                           │
│                                                                                  │
│ ! 2 instrumen dengan harga > 7 hari — eskalasi ke Risk Officer sudah dikirim.  │
│                                                                                  │
│ Sort: Umur Harga ↓   [Filter Klasifikasi ▾] [Filter Sumber ▾]                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ Instrumen       Klasifikasi   Tgl MTM     Tgl Harga   Umur     Alasan   Status  │
│──────────────────────────────────────────────────────────────────────────────────│
│ SHM-0099        FVOCI ELEC    2026-06-17  2026-06-11  6 hari  [ESKALASI]        │
│ Saham Telkom              IDR                         [STALE 6 hr] HARGA-NULL  │
│                                                                          ●Stale  │
│ [Override Setuju]  [Tolak]  [Lihat Detail →]                                    │
│──────────────────────────────────────────────────────────────────────────────────│
│ OBL-0077        FVTPL         2026-06-18  2026-06-14  4 hari                    │
│ Obligasi XYZ               USD           (kurs OK)   [STALE 4 hr] KURS-N/A  │
│                                                                          ●Stale  │
│ [Override Setuju disabled: "Upload harga baru dulu"]  [Lihat Detail →]          │
│──────────────────────────────────────────────────────────────────────────────────│
│                       Halaman 1 dari ~1   2 baris total                          │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Kolom

| Kolom | Keterangan |
|---|---|
| Instrumen | kode + nama (2 baris) |
| Klasifikasi | `MtmRoutingBadge` |
| Tgl MTM | tanggal assessment |
| Tgl Harga | harga_tanggal (terakhir tersedia) |
| Umur Harga | `MtmStaleBadge` — amber jika 5-7 hari, merah jika > 7 hari (eskalasi) |
| Alasan | `HARGA_TIDAK_TERSEDIA` / `KURS_FCY_TIDAK_TERSEDIA` (pill kecil) |
| Status | `MtmStatusBadge` |
| Aksi | kontekstual per persona dan kondisi |

### 7.3 Eskalasi Banner

Jika ada baris dengan `harga_age_days > MTM_STALE_ESCALATION_DAYS` (default 7):

```
! 2 instrumen telah dieskalasi ke Risk Officer (harga > 7 hari).
  Risk Officer telah menerima notifikasi. Tindakan segera diperlukan.
```

Background red-50, border red-200. Icon `AlertTriangle`.

### 7.4 Aksi per Baris

- **Override Setuju**: aktif hanya jika baris dalam status `PENDING_REVIEW` (sudah di-re-upload dengan harga baru). Jika masih `STALE_PRICE`: tombol disabled dengan tooltip "Upload harga terbaru terlebih dahulu via Upload Manual."
- **Tolak**: aktif jika `PENDING_REVIEW` atau `STALE_PRICE` + persona ROLE-AKUN-CTL.
- **Lihat Detail**: navigasi ke `/mtm/[id]`.

---

## 8. Screen 6 — Cron Trigger Panel `/mtm/cron`

### 8.1 Persona Gate

Seluruh halaman `/mtm/cron` hanya dirender jika `permissions.includes('mtm.trigger')`. Middleware Next.js redirect ke 403 jika tidak punya permission. Komponen `MtmCronTriggerButton` sendiri juga tidak dirender di DOM untuk persona lain (absen dari DOM, bukan hanya disabled).

### 8.2 Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ Admin — MTM Cron Manual Trigger         [ROLE-IT-ADMIN]                         │
│                                                                                  │
│ Jalankan MTM daily run secara manual (jika cron 18:00 WIB terlewat atau gagal). │
│                                                                                  │
│ Tanggal Target MTM                                                              │
│ [Date input — default: hari ini]   (tidak boleh tanggal masa depan)            │
│                                                                                  │
│ Force Re-run                                                                     │
│ [Checkbox] Re-run instrumen yang sudah AUTO_POSTED atau APPROVED               │
│ Hati-hati: force re-run dapat membuat duplikat jika tidak diperlukan.          │
│                                                                                  │
│ [MtmCronTriggerButton: "Jalankan MTM Cron"]   (MFA required — ROLE-IT-ADMIN)  │
│                                                                                  │
│ ─────────────────────────────────────────────────────────────────────────────── │
│ JOB PROGRESS (muncul setelah trigger)                                           │
│                                                                                  │
│ [JobProgressPanel — reuse dari P5-M5]                                          │
│ ████████░░░░░░░░░░░  47%                                                        │
│ Menghitung MTM instrumen 470 dari 1000: OBL-0042                               │
│ Mulai: 14:30:00 · ETA: 14:35:00                                                 │
│ [Batalkan]  [Background]                                                        │
│                                                                                  │
│ ─────────────────────────────────────────────────────────────────────────────── │
│ RIWAYAT CRON (terakhir 10 run)                                                  │
│ ┌────────────┬────────────┬───────────┬──────────────┬──────────┬────────────┐  │
│ │ Tanggal    │ Dipicu     │ Status    │ Auto Posted  │ Pending  │ Stale      │  │
│ ├────────────┼────────────┼───────────┼──────────────┼──────────┼────────────┤  │
│ │ 2026-06-18 │ Otomatis   │ Completed │ 850          │ 3        │ 2          │  │
│ │ 2026-06-17 │ Manual     │ Completed │ 845          │ 5        │ 0          │  │
│ │ 2026-06-16 │ Otomatis   │ Failed    │ —            │ —        │ —          │  │
│ └────────────┴────────────┴───────────┴──────────────┴──────────┴────────────┘  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Riwayat cron bersumber dari `GET /api/v1/jobs?filter[type]=MTM_DAILY_RUN&sort=started_at:desc&limit=10`. Jika cron terakhir berstatus `failed`: baris merah + badge "GAGAL" + link "Lihat DLQ".

---

## 9. Komponen Baru — `frontend/src/components/blips/mtm/`

### 9.1 `MtmStatusBadge`

```
Props:
  status: 'AUTO_POSTED' | 'PENDING_REVIEW' | 'APPROVED' | 'REJECTED' | 'STALE_PRICE'
  size?: 'sm' | 'default'

Aksesibilitas: aria-label="Status MTM: {label}"
WCAG AA: setiap status punya teks + icon (warna bukan satu-satunya signal)
```

### 9.2 `MtmDeviationBadge`

```
Props:
  deltaPct: number
  thresholdPct: number

Tampilan: "DEVIATION {deltaPct}%" amber-700 background amber-100
Tooltip: "Melebihi threshold {thresholdPct}%. Verifikasi diperlukan."
Hanya dirender jika deviation_flag = TRUE (deltaPct dikirim dari API bukan dari perhitungan client)
Aksesibilitas: aria-label="Peringatan deviasi harga: {deltaPct}%"
```

### 9.3 `MtmStaleBadge`

```
Props:
  hargaAgeDays: number
  stalePriceReason: 'HARGA_TIDAK_TERSEDIA' | 'KURS_FCY_TIDAK_TERSEDIA'
  escalated: boolean   (TRUE jika hargaAgeDays > MTM_STALE_ESCALATION_DAYS)

Tampilan normal: "STALE {N} hari" amber-700 background amber-100
Tampilan eskalasi: "ESKALASI {N} hari" red-700 background red-100
Tooltip: full reason string + "Eskalasi ke Risk Officer sudah dikirim" jika escalated
Aksesibilitas: aria-label="Harga kedaluwarsa: {N} hari{, dieskalasi}" 
```

### 9.4 `MtmSourceBadge`

```
Props:
  source: 'IBPA' | 'BEI' | 'KSEI' | 'MANUAL' | 'IBPA_MANUAL' | 'BEI_MANUAL'

Warna:
  IBPA / IBPA_MANUAL: blue
  BEI / BEI_MANUAL: green
  KSEI: purple
  MANUAL: slate (grey)

MANUAL dan *_MANUAL mendapat icon pencil kecil untuk membedakan feed vs manual entry.
Aksesibilitas: aria-label="Sumber harga: {source}"
```

### 9.5 `MtmRoutingBadge`

```
Props:
  eventCodes: string[]   — mis. ['MTM_FVOCI', 'MTM_FX_OCI_RESERVE']
  klasifikasi: string    — mis. 'FVOCI_DEBT'

Tampilan: pill kecil per event code. Jika lebih dari 1 (FVOCI_DEBT FCY): stack vertikal.
Tooltip per pill: deskripsi singkat akun target.
Aksesibilitas: aria-label="Routing jurnal: {eventCodes.join(', ')}"
```

Mapping event code ke label:
- `MTM_FVOCI` → "OCI Nilai Wajar"
- `MTM_FX_OCI_RESERVE` → "OCI FX Reserve"
- `MTM_FVOCI_ELECTION` → "OCI Ekuitas"
- `MTM_FVTPL` → "P&L Fair Value"
- `MTM_FVTPL_POCI` → "P&L POCI"

### 9.6 `MtmUploadDropzone`

Clone dari `KursUploadDropzone` dengan perbedaan:
- Kolom template: `kode_instrumen, tanggal_mtm, harga_pasar, [harga_sumber], [catatan]`
- Max size: 10 MB (vs 5 MB Kurs)
- Max rows: 500 per batch
- Link download template XLSX + CSV
- Info notice instrumen AC + FCY kurs

```
Props:
  onSuccess?: (response: MtmUploadBatchResponse) => void
  onCancel?: () => void
  className?: string

Pola identik dengan KursUploadDropzone:
  - react-hook-form + zod validation
  - idempotencyKey via useRef(uuidv4()), refresh per file change
  - drag & drop + click to browse
  - inline file preview (nama + ukuran) + tombol hapus
  - submit button disabled + spinner saat submitting (block double-submit)
```

### 9.7 `MtmOverrideApproveDialog`

```
Props:
  mtmId: string
  instrumenKode: string
  tanggalMtm: string
  deviationFlag: boolean
  onSuccess?: (response: MtmOverrideApproveResponse) => void

Pola: DestructiveActionDialog dari P5-M4, tapi bukan destructive variant (ini approve).
Konten dialog:
  - Judul: "Override Approve MTM {instrumenKode} {tanggalMtm}"
  - DeviationWarningBanner jika deviationFlag = TRUE
  - Textarea "Komentar Persetujuan" (min 30 karakter, aria-describedby ke hint)
  - Checkbox attest: "Saya menyatakan bahwa harga ini telah diverifikasi dari sumber primer."
  - Tombol "Setuju & Posting Jurnal" — disabled sampai checkbox dicentang + komentar ≥ 30 char
  - SoD note (info) di footer dialog: "Anda bertindak sebagai Finance Controller (override-approver)."
  - Idempotency-Key dibuat fresh saat dialog dibuka (useRef uuidv4())

Error handling:
  - MTM_OVERRIDE_SOD_VIOLATION: toast merah persisten + tutup dialog
  - WORKFLOW_INVALID_TRANSITION: toast merah + tutup dialog
  - MTM_PERIODE_LOCKED: toast merah + tutup dialog + trigger refetch list
  - VALIDATION_FAILED: highlight textarea + inline message
```

### 9.8 `MtmOverrideRejectDialog`

```
Props:
  mtmId: string
  instrumenKode: string
  tanggalMtm: string
  onSuccess?: (response: MtmOverrideRejectResponse) => void

Pola: DestructiveActionDialog (variant destructive — reject adalah tindakan yang perlu konfirmasi).
Konten dialog:
  - Judul: "Tolak MTM {instrumenKode} {tanggalMtm}"
  - Warning: "Jurnal tidak akan diposting. ROLE-AKUN akan dinotifikasi untuk re-upload."
  - Textarea "Alasan Penolakan" (min 30 karakter WAJIB — AC4, aria-describedby ke hint)
  - Hint: "Wajib minimum 30 karakter. Jelaskan alasan penolakan agar ROLE-AKUN dapat re-upload."
  - Tombol "Tolak MTM" variant destructive — disabled jika komentar < 30 char
  - Idempotency-Key fresh saat dialog dibuka

Error S4-AC4: komentar < 30 char → highlight textarea + "Komentar wajib minimal 30 karakter untuk override-reject MTM."
```

### 9.9 `MtmCronTriggerButton`

```
Props:
  tanggalTarget: string   — dari date input di halaman /mtm/cron
  forceRerun: boolean
  onJobStarted?: (jobId: string) => void

Komponen ini TIDAK dirender di DOM jika caller tidak memiliki permission mtm.trigger.
Pemeriksaan permission dilakukan di level halaman (middleware + server component) BUKAN hanya di komponen.

Setelah klik:
  1. POST /trx/mtm/cron/trigger dengan Idempotency-Key + body
  2. Return 202 + jobId
  3. onJobStarted(jobId) dipanggil → parent render <JobProgressPanel jobId={jobId}>
  4. Toast info: "MTM cron dijadwalkan untuk {tanggalTarget}. Pantau progres di bawah."

Rate limit display: jika 429 → toast merah persisten "Terlalu banyak request MTM cron trigger. Maksimal 10 kali per jam."
```

### 9.10 `MtmPriceHistoryChart`

```
Props:
  instrumenId: string
  tanggalMtm: string   — titik hari ini yang diberi marker khusus

Data source: GET /trx/mtm?filter[instrumen_id]={id}&filter[tanggal_mtm]=between:{30_hari_lalu},{tanggal_mtm}&sort=tanggal_mtm:asc&limit=30

Chart:
  - Library: Recharts LineChart (sesuai stack, DEC-002)
  - Sumbu X: tanggal (format DD MMM)
  - Sumbu Y: harga_pasar_idr (format IDR singkat: "16,0 Jt")
  - Line: warna primary
  - Titik hari ini: marker diamond merah
  - Tooltip: tanggal + harga_pasar_idr + harga_sumber + delta_pct
  - Ukuran: 100% lebar kolom × 200px tinggi
  - Loading state: skeleton 200px
  - Empty state: "Tidak ada riwayat harga untuk 30 hari terakhir."
  - Aksesibilitas: aria-label="Grafik riwayat harga {instrumenKode} 30 hari terakhir"
                   role="img" — data tabel aksesibilitas via visually-hidden summary table
```

---

## 10. Notifikasi — Bahasa Indonesia (UX §2)

### 10.1 Sukses

| Aksi | Copy (ID) |
|---|---|
| Upload berhasil | "{{N}} MTM berhasil di-upload untuk {{tanggal}}. Status: Menunggu approval Finance Controller." + action "Lihat Batch →" |
| Override approve | "Override MTM {{kode}} {{tanggal}} disetujui. Jurnal {{eventCodes}} berhasil diposting." + action "Lihat Jurnal →" |
| Override reject | "MTM {{kode}} {{tanggal}} ditolak. ROLE-AKUN telah dinotifikasi untuk re-upload." |
| Cron trigger | "MTM cron dijadwalkan untuk {{tanggal}}. Pantau progres di panel." |
| Cron selesai | "MTM run {{tanggal}} selesai: {{autoPosted}} auto-posted, {{pending}} pending review, {{stale}} stale price, {{acSkip}} AC skipped." + action "Lihat MTM →" |

### 10.2 Gagal (persistent toast)

| Kode Error | Copy (ID) |
|---|---|
| `MTM_INSTRUMEN_AC_SKIP` | "{{kode}} berklasifikasi AC — tidak ada MTM untuk AC per PSAK 71. Hapus baris dari file upload." |
| `MTM_PERIODE_LOCKED` | "Periode {{kodePeriode}} sudah hard-closed. Tidak bisa menambah atau mengubah MTM untuk periode ini." |
| `MTM_PRICE_STALE` | "Kurs {{currency}} untuk {{tanggal}} belum tersedia (APPROVED). Upload kurs manual via halaman Kurs terlebih dahulu." |
| `MTM_OVERRIDE_SOD_VIOLATION` | "Anda tidak dapat menyetujui MTM yang Anda upload sendiri. SoD: override-approver ≠ uploader (DEC-017)." |
| `MTM_BATCH_NOT_FOUND` | "Batch upload tidak ditemukan. Pastikan batch_id benar dan Anda memiliki akses." |
| `VALIDATION_FAILED` (comment) | "Komentar wajib minimal 30 karakter untuk tindakan ini. Saat ini: {{N}} karakter." |
| `SOD_VIOLATION` (generic) | "Anda tidak bisa menjadi reviewer/approver untuk transaksi yang Anda buat sendiri." |
| `WORKFLOW_INVALID_TRANSITION` | "Tindakan ini tidak valid untuk status MTM saat ini ({{currentStatus}}). Muat ulang halaman untuk melihat status terkini." |
| `RATE_LIMITED` (cron) | "Terlalu banyak request MTM cron trigger. Maksimal 10 kali per jam per user." |

### 10.3 Warning (8 detik auto-dismiss)

| Kondisi | Copy (ID) |
|---|---|
| Deviasi dalam upload | "Peringatan: {{kode}} memiliki deviasi {{deltaPct}}% melebihi threshold {{thresholdPct}}%. Finance Controller wajib verifikasi sebelum jurnal diposting." |

---

## 11. Interaksi Override — Happy Path dan Failure Modes

### 11.1 Happy Path Override Approve (S4-AC1)

```
1. ROLE-AKUN-CTL membuka /mtm atau /mtm/[id]
2. Klik [Override Setuju] → MtmOverrideApproveDialog terbuka
3. Jika deviation_flag: DeviationWarningBanner muncul dalam dialog
4. User isi komentar ≥ 30 char + centang checkbox attest
5. Tombol "Setuju & Posting Jurnal" aktif → klik
6. Loading spinner inline di tombol, tombol disabled (block double-submit)
7. POST /trx/mtm/{id}/override-approve dengan Idempotency-Key
8. HTTP 200 → dialog tutup → toast sukses → baris di DataTable diupdate status APPROVED
   (via invalidate TanStack Query key ['mtm', id] + ['mtm-list'])
9. Jurnal entry ID tersedia → jurnal icon di baris list link aktif
```

### 11.2 Failure — SoD Violation (S4-AC3)

```
1. User yang sama dengan uploader mencoba klik [Override Setuju]
2. Tombol [Override Setuju] TIDAK ditampilkan di DOM (server-driven via uploader_id comparison)
   → ATAU jika user multi-role melewati: dialog terbuka → klik → 403 MTM_OVERRIDE_SOD_VIOLATION
3. Toast merah persisten: "Anda tidak dapat menyetujui MTM yang Anda upload sendiri. SoD: DEC-017."
4. SoD Note tetap visible di Detail screen jika caller.userId === row.uploaderId
```

### 11.3 Failure — Comment Terlalu Pendek (S4-AC4)

```
1. User isi komentar < 30 char di RejectDialog → tombol "Tolak MTM" tetap disabled
   (client-side: button disabled jika comment.length < 30)
2. Jika bypass client-side: 400 VALIDATION_FAILED → highlight textarea red
   + inline message "Komentar wajib minimal 30 karakter. Saat ini: {N} karakter."
   + toast merah persisten "Validasi gagal: komentar terlalu pendek."
```

### 11.4 Failure — Periode Locked (S2-AC3)

```
1. User mencoba upload untuk tanggal dalam periode CLOSED
2. Server return 423 MTM_PERIODE_LOCKED
3. Toast merah persisten dengan pesan spesifik
4. PeriodeLockBanner muncul di atas form
5. Form tidak di-reset — user bisa ubah tanggal ke periode OPEN
```

### 11.5 Loading dan Empty States

| Screen | Loading | Empty (no filter) | Empty (filter aktif) | Error |
|---|---|---|---|---|
| MTM List | Skeleton 10 baris | "Belum ada MTM. Cron 18:00 WIB atau Upload Manual." + button | "Tidak ada hasil. Hapus filter." + button | Pesan error + Retry |
| Stale Alerts | Skeleton 5 baris | "Tidak ada harga kedaluwarsa saat ini." (icon hijau sukses) | "Tidak ada hasil." + button hapus filter | Pesan + Retry |
| Batch Detail | Skeleton tabel | — | — | Pesan MTM_BATCH_NOT_FOUND |
| Price History | Skeleton 200px | "Tidak ada riwayat 30 hari terakhir." | — | Fallback: hide chart silently |

---

## 12. Komponen Reuse dari M2–M5

| Komponen | Sumber | Cara dipakai di M6 |
|---|---|---|
| `DataTable` | `components/blips/DataTable.tsx` | Semua screen list (MTM List, Stale Alerts, Batch Detail, Cron History) |
| `DestructiveActionDialog` | `components/blips/DestructiveActionDialog.tsx` (P5-M4) | Basis pola untuk `MtmOverrideRejectDialog` |
| `JobProgressPanel` | `components/blips/JobProgressPanel.tsx` (P5-M5) | Cron Trigger progress di `/mtm/cron` |
| `PeriodeLockBanner` | `components/blips/PeriodeLockBanner.tsx` (P5-M4) | Semua screen yang bisa berinteraksi saat periode CLOSED |
| `KursUploadDropzone` | `components/blips/fx-rate/KursUploadDropzone.tsx` (P5-M5) | Pola langsung diklone untuk `MtmUploadDropzone` dengan kolom berbeda |
| `KursWorkflowBadge` / `KursDeviationBadge` | P5-M5 | Pola badge diadaptasi untuk `MtmStatusBadge`, `MtmDeviationBadge`, `MtmStaleBadge`, `MtmSourceBadge`, `MtmRoutingBadge` |
| `JisdorSyncTriggerButton` | P5-M5 | Pola diadaptasi untuk `MtmCronTriggerButton` |
| `notify` (lib/notify.ts) | semua modul | Error + sukses toast (ID string) |

---

## 13. Handoff ke Frontend Engineer

### 13.1 Checklist Komponen

Buat di `frontend/src/components/blips/mtm/`:

- [ ] `MtmStatusBadge.tsx` — 5 state, WCAG AA, aria-label
- [ ] `MtmDeviationBadge.tsx` — amber, tooltip threshold
- [ ] `MtmStaleBadge.tsx` — amber/merah per eskalasi, tooltip alasan
- [ ] `MtmSourceBadge.tsx` — 6 source, icon pencil untuk MANUAL
- [ ] `MtmRoutingBadge.tsx` — stack pills per event code, tooltip akun
- [ ] `MtmUploadDropzone.tsx` — clone KursUploadDropzone, kolom MTM
- [ ] `MtmOverrideApproveDialog.tsx` — comment ≥ 30 char, checkbox attest, idempotency
- [ ] `MtmOverrideRejectDialog.tsx` — destructive, comment ≥ 30 char WAJIB, idempotency
- [ ] `MtmCronTriggerButton.tsx` — DOM gating permission mtm.trigger, JobProgressPanel wrapper
- [ ] `MtmPriceHistoryChart.tsx` — Recharts LineChart, 30 hari, marker hari ini

### 13.2 Checklist Routes (Next.js App Router)

- [ ] `app/(dashboard)/mtm/page.tsx` — MTM List
- [ ] `app/(dashboard)/mtm/[id]/page.tsx` — MTM Detail
- [ ] `app/(dashboard)/mtm/upload/page.tsx` — Upload Form
- [ ] `app/(dashboard)/mtm/upload/batch/[batch_id]/page.tsx` — Batch Detail
- [ ] `app/(dashboard)/mtm/alerts/stale-price/page.tsx` — Stale Alerts
- [ ] `app/(dashboard)/mtm/cron/page.tsx` — Cron Trigger (protected: mtm.trigger)

### 13.3 Validasi Client-Side (Zod Schema)

```
MtmUploadFormSchema:
  file: File, required, MIME xlsx|csv, max 10MB
  catatanUpload: string, optional, max 1000 char
  tanggalMtmOverride: string, optional, DATE format YYYY-MM-DD

MtmOverrideApproveSchema:
  comment: string, min 30 char (wajib untuk deviation_flag, best practice semua)
  signatureMethod: literal('JWT_STEP_UP')
  attest: boolean, must be true (checkbox)

MtmOverrideRejectSchema:
  comment: string, min 30 char WAJIB (S4-AC4)
  signatureMethod: literal('JWT_STEP_UP')

MtmCronTriggerSchema:
  tanggalTarget: string, DATE, tidak future date
  forceRerun: boolean, default false
```

### 13.4 API Integration Keys (TanStack Query)

```
['mtm-list', filters, sort, cursor]     → GET /trx/mtm
['mtm-detail', id]                      → GET /trx/mtm/{id}
['mtm-upload-batch', batch_id]          → GET /trx/mtm/upload/batch/{batch_id}
['mtm-alerts-stale', filters, cursor]   → GET /trx/mtm/alerts/stale-price
['mtm-price-history', instrumen_id]     → GET /trx/mtm?filter[instrumen_id]=...
['job-status', jobId]                   → GET /api/v1/jobs/{jobId} (polling fallback)
```

Invalidate `['mtm-list']` dan `['mtm-detail', id]` setelah override-approve, override-reject, dan upload sukses.

### 13.5 SoD DOM Gating Logic

```typescript
// Di MTM Detail page (server component atau client dengan user context)
const isUploader = mtmRow.uploaderId === currentUser.id
const canOverride = currentUser.permissions.includes('mtm.override')
const statusAllowsOverride = ['PENDING_REVIEW', 'STALE_PRICE'].includes(mtmRow.status)
const periodeLocked = mtmRow.lockedFlag

// Override Action Panel hanya render jika:
const showOverridePanel = canOverride && statusAllowsOverride && !periodeLocked

// SoD note di dalam panel jika uploader = caller
const showSoDNote = isUploader && canOverride
```

### 13.6 Copy Teks — Label Formulir (ID)

| Field | Label ID |
|---|---|
| File harga MTM | "File Harga MTM *" |
| Catatan upload | "Catatan Upload" |
| Override tanggal MTM | "Override Tanggal MTM (opsional)" |
| Komentar persetujuan | "Komentar Persetujuan *" |
| Komentar penolakan | "Alasan Penolakan *" |
| Override tanggal target | "Tanggal Target MTM" |
| Force rerun | "Jalankan ulang instrumen yang sudah diposting" |
| Attest checkbox | "Saya menyatakan bahwa harga ini telah diverifikasi dari sumber primer." |

Hint teks komentar approve: "Minimal 30 karakter. Jelaskan dasar verifikasi harga (Bloomberg, telepon counterparty, dll)."
Hint teks komentar reject: "Wajib minimal 30 karakter. Jelaskan alasan penolakan agar ROLE-AKUN dapat melakukan re-upload dengan benar."

---

## 14. Deviasi dari P5-M5 Pattern

| Aspek | P5-M5 (FX Rate) | P5-M6 (MTM) | Alasan |
|---|---|---|---|
| Workflow approve | Single approve button + comment | Override approve + WAJIB attest checkbox | Deviasi harga berisiko tinggi, perlu attestasi eksplisit per DEC-017 |
| Status badge | 5 status linear | 5 status dengan cabang STALE_PRICE dan AUTO_POSTED | MTM memiliki dua terminal state awal (auto vs pending) dari sumber berbeda |
| Price history | Tidak ada | MtmPriceHistoryChart 30 hari | Konteks tren harga penting untuk keputusan override |
| Cron trigger | JisdorSyncTriggerButton embedded di list page | Halaman terpisah /mtm/cron + riwayat run | MTM cron lebih kompleks, memerlukan forceRerun option + riwayat |
| Export threshold | 10k inline | 10k inline (sama) | Sama |
| Override reject | Approve/Reject sama kompleksitasnya | Reject WAJIB 30 char (DB constraint chk_mtm_override_comment) | SoD + audit requirement |
| Upload file limit | 5 MB | 10 MB, maks 500 baris | MTM bisa lebih banyak instrumen per batch |
| SoD note | Ditampilkan di form | Ditampilkan di Detail page + disabled button tooltip | Visibilitas SoD lebih eksplisit karena uploader juga bisa punya multi-role |

---

## 15. Aksesibilitas Ringkasan

- Semua badge: teks + icon (warna bukan satu-satunya signal) + aria-label
- Tombol override: `aria-disabled` + tooltip penjelasan ketika disabled (SoD, locked, status salah)
- Form fields: `aria-describedby` ke hint + error message
- DataTable header sortable: `aria-sort="ascending/descending/none"` + keyboard trigger
- Dialog: focus trap saat terbuka, focus kembali ke trigger saat ditutup
- Price history chart: `role="img"` + `aria-label` + visually-hidden data summary table
- Tab order: filter bar → table header → table rows → pagination (logical)
- Keyboard: semua tombol dan link reachable via Tab, Enter/Space untuk click
- Contrast: semua badge dan text memenuhi WCAG 2.1 AA (4.5:1 untuk teks normal)

---

_Design spec ini siap dihandoff ke `frontend-engineer-nextjs`. Backend (`backend-engineer-go`) harus menyelesaikan compliance gate S5 (`ifrs9-compliance-reviewer` BLOCKING) dan security gate S4 (`security-engineer` BLOCKING) sebelum routing.go dan override endpoint di-merge ke develop._
