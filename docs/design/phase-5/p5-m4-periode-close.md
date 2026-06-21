# P5-M4 — Periode Buku Close Workflow UI Design Specification

**Story Set**: P5-M4
**Modul**: APP-D — Periode Buku (Phase 5, Module 4)
**Desainer**: uiux-designer
**Tanggal**: 2026-06-17
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M4-periode-close.md`
**Linked API**: `api/openapi/app-d-periode-close.yaml`
**Linked State Machine**: `docs/state-machines/p5-m4-periode-close.md`
**Decisions applied**:
- DEC-017 (4-eyes SoD; CFO sole approver untuk hard-close; SoD `approver_id ≠ requester_id`)
- DEC-018 (Audit trail append-only, 10+10 tahun retensi)
- DEC-021 (Idempotency-Key wajib di setiap mutating endpoint)
- DEC-022 (Cursor-only pagination)
- DEC-026 (MFA mandatory: ROLE-CFO)
- DEC-027 (Step-up MFA wajib: hard-close approve + reopen CLOSED→SOFT_CLOSED)
- DEC-036 RESOLVED (Cure evaluation diizinkan saat SOFT_CLOSED via CORRECTION_PERIODE_CLOSED)
- OQ-M4-1c RESOLVED (Checklist stale threshold = 24 jam)
- OQ-M4-3a RESOLVED (CFO bisa reject hard-close request tanpa step-up MFA)
- OQ-M4-3b RESOLVED (Grace window default 48 jam, global via sys.config)

**Dependensi**:
- P5-M2 (`/jrnl/journal-entries`) — JURNAL_BALANCED checklist item dari `jrnl.header`
- P5-M3 (`/jrnl/gl-delivery-dlq`) — GL_DELIVERED checklist item dari `jrnl.gl_status`
- P5-M3 (RECON_PASS) — `sys.gl_reconciliation_report` terakhir per periode
- P5-M13 (upcoming) — MV refresh triggered by hard-close approve

---

## 1. Screen Inventory

### 1.1 Sitemap P5-M4

```
Periode Buku (side nav group — baru)
├── /periode-buku                          — List semua periode (S5)
├── /periode-buku/[id]                     — Detail + closing workflow (S1–S4)
│   └── (modal) ChecklistSnapshotDetail   — Snapshot detail dialog
│   └── (modal) MFAStepUpDialog           — Step-up MFA challenge
│   └── (modal) SoftCloseRequestDialog    — Form request soft-close
│   └── (modal) SoftCloseApproveDialog    — Form approve soft-close
│   └── (modal) HardCloseRequestDialog    — Form request hard-close
│   └── (modal) HardCloseApproveDialog    — Form approve hard-close (MFA gated)
│   └── (modal) HardCloseRejectDialog     — Form reject hard-close (CFO)
│   └── (modal) ReopenRequestDialog       — Form reopen request (CFO)
│   └── (modal) ReopenApproveDialog       — Form reopen approve (CFO, MFA conditional)
└── Laporan (sub-group)
    └── /reports/status-periode            — Status periode DataTable report (S5)

Cross-cutting (global)
└── <PeriodeLockBanner>                    — Banner di transaksi/jurnal saat SOFT_CLOSED atau CLOSED
```

### 1.2 Navigasi Side Nav (tambahan ke APP-D)

```
Periode Buku
  /periode-buku             [badge oranye pulse jika ada HARD_CLOSE_PENDING]
  ─
Laporan
  Status Periode  → /reports/status-periode   (ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-RISK)
```

**HARD_CLOSE_PENDING Badge**: badge oranye berkedip di side nav dan top bar jika ada periode dalam status `HARD_CLOSE_PENDING`. Menampilkan angka periode yang menunggu approval CFO. Polling 60 detik atau SSE subscribe ke channel `periode-status-change`.

### 1.3 AC Mapping

| Screen | Route | Persona Utama | Story Ref | AC Yang Tercakup |
|---|---|---|---|---|
| Periode Buku List | `/periode-buku` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT | S5 | S5-AC2 (list view), S5-AC3 (export) |
| Periode Detail + Workflow | `/periode-buku/[id]` | ROLE-AKUN-CTL (S1,S2,S3 request), ROLE-CFO (S3 approve, S4) | S1,S2,S3,S4,S5 | S1-AC1..4, S2-AC1..4, S3-AC1..4, S4-AC1..4, S5-AC1,AC4 |
| ChecklistSnapshotDetail | dialog dalam `/periode-buku/[id]` | Semua dengan `periode.read` | S5 | S5-AC1, S5-AC4 |
| MFAStepUpDialog | modal dalam `/periode-buku/[id]` | ROLE-CFO | S3, S4 | S3-AC2, S3-AC3, S4-AC2 |
| Status Periode Report | `/reports/status-periode` | ROLE-AKUN-CTL, ROLE-CFO, ROLE-AUDIT, ROLE-RISK | S5 | S5-AC2, S5-AC3 |
| PeriodeLockBanner | global cross-cutting | Semua | S2, S3 | S2-AC1 (post-approve), S3-AC4 |

---

## 2. Status Badge Design — `status_periode`

Semua badge menggunakan warna + ikon + teks label. Warna bukan satu-satunya sinyal (WCAG 2.1 AA).

### 2.1 PeriodeStatusBadge — Color Matrix

| `status_periode` | Warna | Ikon | Label ID | Label EN (export) | Keterangan |
|---|---|---|---|---|---|
| `OPEN` | Abu-abu (slate-400) | circle | Terbuka | Open | Periode aktif, mutasi diizinkan |
| `SOFT_CLOSED` | Amber (amber-500) | lock | Soft-Closed | Soft Closed | Mutasi diblokir; menunggu hard-close CFO |
| `HARD_CLOSE_PENDING` | Oranye (orange-500) + animate-pulse | clock-alert | Menunggu CFO | Pending CFO Approval | CFO belum approve hard-close |
| `CLOSED` | Hijau gelap (green-700) | lock-keyhole | Ditutup Final | Closed | Hard-closed — tidak bisa mutasi |
| `CLOSED` (dalam grace) | Hijau gelap + badge sekunder countdown | lock-keyhole | Ditutup — Grace {N}j | Closed (Grace {N}h) | Bisa reopen dalam sisa waktu |

**Grace countdown sub-badge** (hanya tampil saat CLOSED dan `now() < hard_close_grace_expires_at`):

```
[lock-keyhole] Ditutup Final  [countdown] Reopen tersedia: 23j 45m
```

Countdown diperbarui setiap menit via `setInterval`. Saat grace expires: sub-badge berubah menjadi "Tidak bisa di-reopen" (slate, ikon ban).

### 2.2 Checklist Item Status

| Kondisi | Warna | Ikon | Label ID |
|---|---|---|---|
| `passed = true` | Hijau (green-600) | check-circle-2 | Lolos |
| `passed = false` | Merah (red-600) | x-circle | Gagal |
| Loading | Abu-abu (slate-300) | loader-2 (spin) | Memeriksa... |

### 2.3 Checklist Snapshot Transition Badges

| `transition` | Warna | Label ID |
|---|---|---|
| `SOFT_CLOSE_REQUEST` | Biru (blue-500) | Soft-Close Request |
| `SOFT_CLOSE_APPROVE` | Hijau (green-600) | Soft-Close Approved |
| `HARD_CLOSE_REQUEST` | Oranye (orange-500) | Hard-Close Request |
| `HARD_CLOSE_APPROVE` | Hijau gelap (green-700) | Hard-Close Approved |
| `REOPEN_REQUEST` | Amber (amber-500) | Reopen Request |
| `REOPEN_APPROVE` | Amber (amber-600) | Reopen Approved |

**`transition_status` badges** (secondary):

| `transition_status` | Warna | Label |
|---|---|---|
| `APPROVED` | Hijau | Disetujui |
| `REJECTED` | Merah | Ditolak |

---

## 3. Wireframes — 6 Screens

### SCREEN-P5-M4-01: Periode Buku List

**Route**: `/periode-buku`

**AC**: S5-AC2 (list + filter), S5-AC3 (export)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Periode Buku                                                      [Export ▾ CSV/XLSX]   │
│  Daftar periode buku dan status penutupan                                                │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari kode periode...]   [Status ▾]   [Tahun ▾]   [Bulan ▾]   [Tipe ▾]   [Clear]   │
│ Filter chips: [Status: OPEN ×]  [Tahun: 2026 ×]                                         │
│ Default filter: Tahun = tahun berjalan, sort = tanggal_akhir DESC                        │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]  "Diperbarui: 17 Jun 2026, 09:00"       │
├──────────────┬────────┬────────┬───────────────┬────────────────────────────┬───────────┤
│ Kode ↕       │ Tahun ↕│ Bulan ↕│ Tipe          │ Status                     │ Aksi      │
├──────────────┼────────┼────────┼───────────────┼────────────────────────────┼───────────┤
│ PRD-2026-07  │ 2026   │  7     │ BULANAN       │ [abu] Terbuka              │ [Detail →]│
│              │        │        │               │                            │           │
│ PRD-2026-06  │ 2026   │  6     │ BULANAN       │ [oranye●] Menunggu CFO    │ [Detail →]│
│              │        │        │               │ Sejak: 2 Jul 2026, 09:28  │           │
│              │        │        │               │                            │           │
│ PRD-2026-05  │ 2026   │  5     │ BULANAN       │ [hijau] Ditutup Final      │ [Detail →]│
│              │        │        │               │ 2 Jun 2026, 10:00          │           │
│ PRD-2026-04  │ 2026   │  4     │ BULANAN       │ [hijau] Ditutup Final      │ [Detail →]│
│ PRD-2026-03  │ 2026   │  3     │ BULANAN       │ [hijau] Ditutup Final      │ [Detail →]│
│ PRD-2026-02  │ 2026   │  2     │ BULANAN       │ [hijau] Ditutup Final      │ [Detail →]│
│ PRD-2026-01  │ 2026   │  1     │ BULANAN       │ [hijau] Ditutup Final      │ [Detail →]│
├──────────────┴────────┴────────┴───────────────┴────────────────────────────┴───────────┤
│ Footer: [← Prev]  Hal. 1 dari ~1  [Next →]  Baris: [50 ▾]  Total estimasi: 7           │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter | Format |
|---|---|---|---|---|
| `periode_kode` | Kode | Ya | Text search | `PRD-YYYY-MM` |
| `tahun_buku` | Tahun | Ya | Select: 2024–2029 | Integer |
| `bulan` | Bulan | Ya | Select: 1–12 | Integer → nama bulan tooltip |
| `tipe_periode` | Tipe | Tidak | Select: BULANAN/KUARTALAN/TAHUNAN | — |
| `status_periode` | Status | Ya | Select multi: OPEN/SOFT_CLOSED/HARD_CLOSE_PENDING/CLOSED | `<PeriodeStatusBadge>` |
| `tanggal_akhir` | Tgl Akhir | Ya | Date range | `DD MMM YYYY` |
| `tanggal_hard_close` | Tgl Close | Ya | Date range | `DD MMM YYYY HH:mm` atau `—` |
| actions | Aksi | Tidak | — | [Detail →] |

**Inline action buttons per baris** — gated by `status_periode` + persona (visible hanya jika user punya permission dan status cocok):

| Status | ROLE-AKUN-CTL (Maker) | ROLE-AKUN-CTL (Approver, SoD) | ROLE-CFO |
|---|---|---|---|
| OPEN | [Ajukan Soft Close] | — | — |
| OPEN (pending request, user lain) | — | [Approve Soft Close] | — |
| SOFT_CLOSED | [Ajukan Hard Close] | — | [Reopen] |
| HARD_CLOSE_PENDING | — | — | [Approve Hard Close ●MFA] [Tolak] |
| CLOSED (grace) | — | — | [Reopen ●MFA] |
| CLOSED (setelah grace) | — | — | — |

Tombol tidak di DOM (bukan hanya disabled) jika persona tidak memenuhi syarat.

**Status filter chips default**: tampilkan `OPEN` + `SOFT_CLOSED` + `HARD_CLOSE_PENDING` saat halaman pertama dibuka — exclude `CLOSED` agar daftar ringkas (toggle "Tampilkan CLOSED ☐" di bawah tabel).

**Empty state**: jika tidak ada periode — "Belum ada periode buku. Hubungi IT Admin untuk membuat periode buku awal." (tidak ada tombol create — periode dibuat via migration atau admin panel).

---

### SCREEN-P5-M4-02: Periode Detail + Closing Workflow

**Route**: `/periode-buku/[id]`

**AC**: S1-AC1..4, S2-AC1..4, S3-AC1..4, S4-AC1..4, S5-AC1, S5-AC4

Layout: halaman penuh, satu kolom utama dengan beberapa section vertikal. Panel workflow di sisi kanan (sticky sidebar 40% lebar pada layar ≥ 1280px, stack vertikal pada layar lebih kecil).

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Periode Buku > PRD-2026-06                                                   │
│                                                                                          │
│ STICKY STATUS HEADER                                                                     │
│  PRD-2026-06 · Juni 2026 · BULANAN                                                      │
│  [oranye●] Menunggu CFO  [badge sekunder: hard-close request 2 Jul 09:28 oleh Budi S.] │
│  ─────────────────────────────────────────────────────────────────────────────────────── │
│  Tgl Mulai: 1 Jun 2026  ·  Tgl Akhir: 30 Jun 2026  ·  row_version: 8                  │
├────────────────────────────────────────────────┬────────────────────────────────────────┤
│ MAIN CONTENT (60%)                             │ RIGHT PANEL — WorkflowPanel (40%)      │
│                                                │                                        │
│ SECTION — Closing Checklist                    │ CARD — Aksi yang Tersedia              │
│ ┌──────────────────────────────────────────┐   │ (gated by state + persona)             │
│ │ Closing Checklist           [↻ Refresh]  │   │ ┌──────────────────────────────────┐   │
│ │ Terakhir dievaluasi: 17 Jun, 09:28       │   │ │ STATUS: Menunggu CFO             │   │
│ │ ─────────────────────────────────────    │   │ │ [oranye●]                        │   │
│ │                                          │   │ │                                  │   │
│ │ [✓ check-circle-2] Lolos                 │   │ │ Hard-close request diajukan oleh │   │
│ │   0 transaksi/jurnal masih PENDING       │   │ │ Budi Santoso (AKUN-CTL)          │   │
│ │   Total PENDING_APPROVAL: 0              │   │ │ 2 Jul 2026 pukul 09:28           │   │
│ │                                          │   │ │                                  │   │
│ │ [✓ check-circle-2] Lolos                 │   │ │ ─────────────────────────────── │   │
│ │   Semua jurnal seimbang                  │   │ │                                  │   │
│ │   252 jurnal. Max delta: IDR 0.0000      │   │ │ [Approve Hard Close ●MFA]        │   │
│ │                                          │   │ │ (ROLE-CFO — MFA step-up wajib)   │   │
│ │ [✓ check-circle-2] Lolos                 │   │ │                                  │   │
│ │   Tidak ada GL delivery FAILED           │   │ │ [Tolak Hard Close]               │   │
│ │   DEAD_LETTER: 0 (dikecualikan)          │   │ │ (ROLE-CFO — tanpa MFA)           │   │
│ │                                          │   │ │                                  │   │
│ │ [✓ check-circle-2] Lolos                 │   │ │ ─────────────────────────────── │   │
│ │   GL rekonsiliasi terakhir COMPLETED     │   │ │                                  │   │
│ │   1 Jul 2026 — COMPLETED. Δ IDR 0.0000  │   │ │ [↻ Evaluasi Ulang Checklist]     │   │
│ │                                          │   │ └──────────────────────────────────┘   │
│ │ ─────────────────────────────────────    │   │                                        │
│ │ ● Semua checklist lolos                  │   │ CARD — Riwayat Workflow                │
│ │                                          │   │ (MakerReviewerApproverPanel)           │
│ │ [Lihat Snapshot Terakhir →]              │   │ ┌──────────────────────────────────┐   │
│ └──────────────────────────────────────────┘   │ │ ● Ajukan Soft Close (DONE)       │   │
│                                                │ │   Budi S. · 30 Jun · "Semua..."  │   │
│ SECTION — Informasi Periode                    │ │   [Lihat snapshot →]             │   │
│ ┌──────────────────────────────────────────┐   │ │                                  │   │
│ │ Tgl Mulai:  1 Jun 2026                   │   │ │ ● Approve Soft Close (DONE)      │   │
│ │ Tgl Akhir: 30 Jun 2026                   │   │ │   Sari W. · 30 Jun · "Approved." │   │
│ │ Jml Jurnal: 252 posting                  │   │ │   [Lihat snapshot →]             │   │
│ │ FX Locked:  Tidak (belum hard-close)     │   │ │                                  │   │
│ │ Reopen:     Tidak pernah                 │   │ │ ● Ajukan Hard Close (DONE)       │   │
│ └──────────────────────────────────────────┘   │ │   Budi S. · 2 Jul 09:28          │   │
│                                                │ │   "Semua koreksi selesai."       │   │
│ SECTION — MV Refresh Status                    │ │   [Lihat snapshot →]             │   │
│ (hanya tampil setelah CLOSED)                  │ │                                  │   │
│ [tersembunyi saat bukan CLOSED]                │ │ ▸ Approve Hard Close (PENDING)   │   │
│                                                │ │   Menunggu CFO                   │   │
│                                                │ └──────────────────────────────────┘   │
└────────────────────────────────────────────────┴────────────────────────────────────────┘
```

**States berbeda pada WorkflowPanel (Action Card) berdasarkan `status_periode`:**

**STATE: OPEN, belum ada pending request**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Terbuka  [abu]                                    │
│                                                           │
│ Periode masih aktif. Transaksi dan jurnal dapat diinput. │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Ajukan Soft Close]    ← disabled jika checklist !pass   │
│ (ROLE-AKUN-CTL)        Tooltip: "N item checklist belum  │
│                         lolos — lihat panel checklist"   │
│                                                           │
│ [↻ Evaluasi Ulang Checklist]                             │
└──────────────────────────────────────────────────────────┘
```

**STATE: OPEN, ada pending request (SoD — approver berbeda)**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Terbuka — Menunggu Approval  [abu]               │
│                                                           │
│ Soft-close request diajukan oleh                         │
│ Budi Santoso (AKUN-CTL) pada 30 Jun 2026, 17:00         │
│ Catatan: "Soft close request periode Juni 2026..."       │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Approve Soft Close]   ← visible hanya jika actor ≠ req │
│ (ROLE-AKUN-CTL Approver)                                  │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│ ⚠ Anda adalah pengaju request ini.                        │
│   Anda tidak dapat meng-approve request sendiri.         │
│   (pesan ini tampil HANYA jika actor = requester)        │
│   [penjelasan SoD inline — bukan tooltip]                │
└──────────────────────────────────────────────────────────┘
```

**STATE: SOFT_CLOSED**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Soft-Closed  [amber]                             │
│                                                           │
│ Soft-closed pada 30 Jun 2026 pukul 17:05                │
│ Oleh: Sari Wulandari (AKUN-CTL)                         │
│                                                           │
│ Mutasi transaksi/jurnal diblokir.                        │
│ GL delivery retry masih diperbolehkan.                   │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Ajukan Hard Close]    ← ROLE-AKUN-CTL                  │
│ (Re-run checklist akan dilakukan)                         │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Reopen ke OPEN ↩]     ← ROLE-CFO saja                  │
│ (Tidak perlu step-up MFA)                                │
└──────────────────────────────────────────────────────────┘
```

**STATE: HARD_CLOSE_PENDING**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Menunggu CFO  [oranye pulse]                     │
│                                                           │
│ Hard-close request diajukan oleh                         │
│ Budi Santoso (AKUN-CTL) pada 2 Jul 2026, 09:28          │
│                                                           │
│ CFO step-up MFA diperlukan untuk approve.                │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Approve Hard Close ●MFA]  ← ROLE-CFO saja              │
│ Membuka MFA challenge → konfirmasi → final               │
│                                                           │
│ [Tolak Hard Close]          ← ROLE-CFO saja             │
│ Mengembalikan ke SOFT_CLOSED (tanpa MFA)                 │
└──────────────────────────────────────────────────────────┘
```

**STATE: CLOSED (dalam grace window)**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Ditutup Final  [hijau gelap]                     │
│                                                           │
│ Hard-closed pada 2 Jul 2026 pukul 09:30                  │
│ Oleh: CFO — Hendra Gunawan                               │
│ FX Rate: Terkunci                                        │
│                                                           │
│ Grace Window Reopen:                                     │
│ [countdown] 23 jam 45 menit tersisa                      │
│ Berakhir: 4 Jul 2026 pukul 09:30                        │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ [Reopen ke SOFT_CLOSED ↩ ●MFA]  ← ROLE-CFO saja       │
│ (Step-up MFA wajib — DEC-027)                           │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│ MV Refresh: [status dari job] ← lihat SECTION MV bawah │
└──────────────────────────────────────────────────────────┘
```

**STATE: CLOSED (setelah grace window)**

```
┌──────────────────────────────────────────────────────────┐
│ STATUS: Ditutup Final  [hijau gelap]                     │
│                                                           │
│ Hard-closed pada 2 Jul 2026 pukul 09:30                  │
│ Grace window berakhir: 4 Jul 2026 pukul 09:30 ✓        │
│                                                           │
│ Mutasi tidak bisa dilakukan.                             │
│ Reopen via API tidak tersedia.                           │
│                                                           │
│ ─────────────────────────────────────────────────────     │
│                                                           │
│ ⚠ Untuk reopen setelah grace window berakhir,           │
│   ajukan RFC ke Direksi sesuai RACI BRD §3.             │
└──────────────────────────────────────────────────────────┘
```

**SECTION — MV Refresh Status** (hanya muncul saat `status_periode = 'CLOSED'`):

```
┌──────────────────────────────────────────────────────────────────────────┐
│ MATERIALIZED VIEW REFRESH                                                │
│ ─────────────────────────────────────────────────────────────────────── │
│ Status     : [hijau ✓] Selesai                                           │
│ Selesai    : 2 Jul 2026 pukul 09:35                                      │
│ Job ID     : job_MV_REFRESH_01HXYZ   [Lihat detail →]                   │
│                                                                          │
│ (Jika status = running / queued):                                        │
│ Status     : [biru ↺] Berjalan... (atau Antri)                           │
│ [lihat panel progress]  ← membuka JobProgressPanel (compact mode)        │
│                                                                          │
│ (Jika status = failed):                                                  │
│ Status     : [merah ✗] Gagal                                             │
│ ⚠ Materialized view laporan mungkin belum ter-update.                   │
│    Hubungi ROLE-IT-ADMIN atau coba refresh manual.  [Retry →]           │
└──────────────────────────────────────────────────────────────────────────┘
```

**MakerReviewerApproverPanel (Riwayat Workflow)**:

Panel vertikal di sisi kanan (di bawah action card). Menampilkan setiap transisi yang telah dilakukan, dari atas (terlama) ke bawah (terbaru). Step aktif (menunggu) ditandai dengan border kiri amber.

```
┌──────────────────────────────────────────────────────────┐
│ RIWAYAT TRANSISI PERIODE                                 │
│                                                           │
│ ─ Soft-Close Request ────────────────── [DONE ✓]         │
│   Budi Santoso (AKUN-CTL)                                 │
│   30 Jun 2026, 17:00                                     │
│   "Soft close request periode Juni 2026..."              │
│   Snapshot: [Lihat →]  checklist: 4/4 lolos              │
│                                                           │
│ ─ Soft-Close Approve ───────────────── [DONE ✓]         │
│   Sari Wulandari (AKUN-CTL)                              │
│   30 Jun 2026, 17:05                                     │
│   "Approved. Semua posisi diverifikasi."                  │
│   Snapshot: [Lihat →]  checklist: 4/4 lolos              │
│                                                           │
│ ─ Hard-Close Request ───────────────── [DONE ✓]         │
│   Budi Santoso (AKUN-CTL)                                 │
│   2 Jul 2026, 09:28                                      │
│   "Semua koreksi selesai. Siap hard-close."              │
│   Snapshot: [Lihat →]  checklist: 4/4 lolos              │
│                                                           │
│ ▸ Hard-Close Approve ─────────────── [PENDING ○]        │
│   Menunggu approval CFO                                   │
│   (step-up MFA diperlukan)                               │
└──────────────────────────────────────────────────────────┘
```

Setiap step "Lihat →" membuka `<ChecklistSnapshotDetailDialog>`.

**Data yang ditampilkan di Periode Detail:**

| Field | Sumber | Visible untuk |
|---|---|---|
| `status_periode`, semua tanggal | `mst.periode_buku` | `periode.read` |
| Checklist real-time | `GET /periode-buku/{id}/closing-checklist` | `periode.read` |
| Snapshot riwayat | `sys.closing_checklist_snapshot` | `periode.read` |
| MV refresh status | `sys.job` (job type `reporting:mv_refresh`) | `periode.read` |
| `row_version` | `mst.periode_buku` | Internal (dipakai di request body) |

**Polling checklist**: GET `/api/v1/periode-buku/{id}/closing-checklist` setiap 30 detik jika `status_periode IN ['OPEN','SOFT_CLOSED','HARD_CLOSE_PENDING']`. Berhenti saat `CLOSED`. Tombol "↻ Refresh" untuk refresh manual.

---

### SCREEN-P5-M4-03: MFA Step-Up Dialog

**Route**: modal dalam `/periode-buku/[id]` (tidak route tersendiri)

**AC**: S3-AC2, S3-AC3, S4-AC2

Komponen `<MFAStepUpDialog>` digunakan untuk dua aksi:
- Hard-close approve (`scope='hard_close'`)
- Reopen approve dari CLOSED ke SOFT_CLOSED (`scope='reopen_closed'`)

```
┌────────────────────────────────────────────────────────────────┐
│  Verifikasi MFA Tambahan                                     [×]│
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Aksi yang memerlukan verifikasi ini:                          │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ 🔒 Approve Hard-Close Periode PRD-2026-06                  │  │
│  │ Aksi ini bersifat permanen setelah grace window           │  │
│  │ berakhir (48 jam). Tidak bisa di-reverse via API.         │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  Masukkan kode TOTP dari aplikasi autentikator Anda:           │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │   [ _ _ _ ] - [ _ _ _ ]                                   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ⏱ Token berlaku 5 menit setelah verifikasi.                  │
│                                                                 │
│  (Error state — jika kode salah):                             │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ ✗ Kode salah atau sudah dipakai. Tunggu kode baru atau   │  │
│  │   gunakan kode cadangan.                                  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  (Error state — token expired di langkah berikutnya):         │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ ⏰ Token MFA sudah expired (> 5 menit). Harap ulangi     │  │
│  │   verifikasi dari awal.                       [Ulangi]   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│           [Batal]                 [Verifikasi & Lanjutkan]     │
│                                   (disabled sampai 6 digit)    │
└────────────────────────────────────────────────────────────────┘
```

**Alur internal MFA step-up (di balik komponen):**

```
1. User klik tombol yang memerlukan MFA (e.g., "Approve Hard Close ●MFA")
2. <DestructiveActionDialog> terbuka → user centang atestasi → klik "Lanjut ke MFA"
3. <MFAStepUpDialog> terbuka (bukan modal baru di atas modal — DestructiveActionDialog tertutup dahulu)
4. User input TOTP → POST /auth/step-up { scope: "hard_close" }
5. 200 → { stepUpToken: "..." } (TTL 5 menit)
6. Dialog tertutup, token disimpan di component state (bukan localStorage)
7. Langsung trigger action utama dengan X-Step-Up-Token header
8. Jika action gagal MFA_STEP_UP_EXPIRED → dialog buka kembali dengan pesan expired
```

**Props:**

```tsx
interface MFAStepUpDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: 'hard_close' | 'reopen_closed';
  actionDescription: string;   // kalimat pendek yang menjelaskan aksi
  onTokenReceived: (token: string) => void;
}
```

**Catatan anti-pattern**: dialog ini tidak membuka modal baru di atas `<DestructiveActionDialog>`. Urutan: destructive confirm → tutup → MFA dialog buka. Tidak ada stack modal.

---

### SCREEN-P5-M4-04: Closing Checklist Snapshot Detail Dialog

**Route**: dialog dalam `/periode-buku/[id]` (dipanggil dari link "Lihat →" di MakerReviewerApproverPanel)

**AC**: S5-AC1, S5-AC4

```
┌───────────────────────────────────────────────────────────────────────┐
│  Snapshot Checklist — Soft-Close Request                          [×]  │
│  PRD-2026-06 · 30 Jun 2026, 17:00                                      │
├───────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  Snapshot ID  : b2c3d4e5-f6a7-...                                      │
│  Transisi     : [biru] Soft-Close Request                             │
│  Status       : [hijau] Disetujui                                     │
│  Evaluator    : Budi Santoso (ROLE-AKUN-CTL)                          │
│  Dievaluasi   : 30 Jun 2026 pukul 16:58                              │
│                                                                        │
│  ─────────────────────────────────────────────────────────────────     │
│                                                                        │
│  HASIL CHECKLIST (snapshot saat transisi)                             │
│                                                                        │
│  [✓] 0 transaksi/jurnal masih PENDING_APPROVAL                       │
│      Total PENDING_APPROVAL: 0                                        │
│                                                                        │
│  [✓] Semua jurnal seimbang (delta ≤ IDR 0.01)                        │
│      Jurnal checked: 248. Max delta: IDR 0.0000                       │
│                                                                        │
│  [✓] Tidak ada GL delivery FAILED                                     │
│      Tidak ada FAILED delivery. DEAD_LETTER: 0 (dikecualikan).        │
│                                                                        │
│  [✓] GL rekonsiliasi harian terakhir COMPLETED                        │
│      Last recon: 2026-06-29 — COMPLETED. Delta IDR 0.0000             │
│                                                                        │
│  ─────────────────────────────────────────────────────────────────     │
│                                                                        │
│  (Jika ada item GAGAL):                                               │
│  [✗] Tidak ada GL delivery FAILED                                     │
│      3 jurnal masih FAILED. Header IDs: [uuid1, uuid2, uuid3]         │
│      [→ Lihat DLQ GL Delivery]  (action_url jika tersedia)           │
│                                                                        │
│  ─────────────────────────────────────────────────────────────────     │
│                                                                        │
│  (Indikator stale — jika snapshot ini adalah re-run stale approve):  │
│  ⚠ Snapshot ini adalah re-evaluasi karena checklist sebelumnya       │
│    sudah > 24 jam sejak request diajukan.                             │
│                                                                        │
│                                                     [Tutup]           │
└───────────────────────────────────────────────────────────────────────┘
```

**Data yang ditampilkan:**

| Field | Sumber |
|---|---|
| `id`, `transition`, `transition_status`, `created_at` | `sys.closing_checklist_snapshot` |
| `actor_user_id` + nama | `sec.user` JOIN |
| `checklist_jsonb.items[]` | JSONB — render tiap item via `<JSONBTreeView>` |
| `all_passed` | `sys.closing_checklist_snapshot.all_passed` |
| Stale indicator | hitung dari selisih waktu snapshot vs request snapshot sebelumnya |

---

### SCREEN-P5-M4-05: Destructive Action Confirmation Dialogs

Tiga dialog destructive — semuanya menggunakan `<DestructiveActionDialog>` base component.

#### 5a. Hard-Close Approve Confirm

```
┌────────────────────────────────────────────────────────────────────┐
│  Hard-close Periode PRD-2026-06?                                 [×]│
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ⛔ Tindakan ini tidak bisa di-reverse setelah 48 jam.             │
│                                                                     │
│  Yang akan terjadi:                                                │
│  · Status periode berubah ke CLOSED                               │
│  · Semua mutasi transaksi/jurnal untuk periode ini DIBLOKIR       │
│  · FX rate periode Juni 2026 akan DIKUNCI                         │
│  · Materialized view laporan akan di-refresh (background job)     │
│  · Grace window reopen: 48 jam (berakhir: 4 Jul 2026 09:30)      │
│                                                                     │
│  ─────────────────────────────────────────────────────────────────  │
│                                                                     │
│  ☐ Saya memahami aksi ini tidak bisa di-reverse setelah grace     │
│    window 48 jam berakhir.               ← wajib di-centang       │
│                                                                     │
│  Komentar Approval *  (wajib)                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                                                               │  │
│  └───────────────────────────────────────────────────────────────┘  │
│  Contoh: "Hard close approved. Periode Juni 2026 final."            │
│                                                                     │
│         [Batal]           [Lanjut ke Verifikasi MFA ●]            │
│                           (disabled sampai checkbox + komentar)    │
└────────────────────────────────────────────────────────────────────┘
```

Setelah klik "Lanjut ke Verifikasi MFA" → dialog ini tertutup → `<MFAStepUpDialog>` terbuka (scope='hard_close'). Tidak stack modal.

#### 5b. Reopen Approve Confirm (CLOSED → SOFT_CLOSED, butuh step-up MFA)

```
┌────────────────────────────────────────────────────────────────────┐
│  Reopen Periode PRD-2026-06 ke SOFT_CLOSED?                     [×]│
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ⚠ Tindakan Exceptional — Alasan wajib minimal 30 karakter.       │
│                                                                     │
│  Grace window aktif:  23 jam 45 menit tersisa                      │
│  Berakhir:  4 Jul 2026 pukul 09:30                                │
│                                                                     │
│  Yang akan terjadi:                                                │
│  · Status kembali ke SOFT_CLOSED (bukan OPEN)                     │
│  · FX rate periode diUNLOCK                                       │
│  · Mutasi melalui CORRECTION_PERIODE_CLOSED diizinkan             │
│  · FX rate dikunci kembali saat hard-close berikutnya             │
│                                                                     │
│  Alasan Reopen *  (minimal 30 karakter)                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                                                               │  │
│  │                                                               │  │
│  └───────────────────────────────────────────────────────────────┘  │
│  [sisa: 30 karakter lagi]                                           │
│                                                                     │
│  ☐ Saya memahami reopen ini akan dicatat permanen di audit trail. │
│                                                                     │
│         [Batal]           [Lanjut ke Verifikasi MFA ●]            │
│                           (disabled sampai reason ≥ 30 + checkbox) │
└────────────────────────────────────────────────────────────────────┘
```

#### 5c. Hard-Close Reject Confirm (CFO, tanpa MFA)

```
┌────────────────────────────────────────────────────────────────────┐
│  Tolak Hard-Close Request PRD-2026-06?                          [×]│
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Periode akan kembali ke status SOFT_CLOSED.                       │
│  ROLE-AKUN-CTL akan mendapat notifikasi.                          │
│                                                                     │
│  Alasan Penolakan *  (minimal 30 karakter)                        │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                                                               │  │
│  └───────────────────────────────────────────────────────────────┘  │
│  [sisa: 30 karakter lagi]                                           │
│                                                                     │
│         [Batal]                   [Tolak Hard-Close]               │
│                                   (disabled < 30 char, merah)      │
└────────────────────────────────────────────────────────────────────┘
```

---

### SCREEN-P5-M4-06: Status Periode Report

**Route**: `/reports/status-periode`

**AC**: S5-AC2, S5-AC3

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ PAGE HEADER                                                                              │
│  Laporan Status Periode Buku                                   [Export ▾ CSV / XLSX]    │
│  Ringkasan status seluruh periode buku dengan history penutupan                         │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                               │
│ [🔍 Cari kode periode...]  [Status ▾]  [Tahun ▾]  [Bulan ▾]  [Tipe ▾]   [Clear]       │
│ Filter chips: [Tahun: 2026 ×]                                                            │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR: [Export ▾ CSV / XLSX]  [Refresh ↺]  "Diperbarui: 17 Jun 2026, 09:00"       │
├──────────────┬────────┬──────────┬──────────────┬────────────────┬─────────────────────┤
│ Kode ↕       │ Thn ↕  │ Status   │ Soft Close ↕ │ Hard Close ↕   │ MV Refresh          │
├──────────────┼────────┼──────────┼──────────────┼────────────────┼─────────────────────┤
│ PRD-2026-07  │ 2026   │[abu]OPEN │ —            │ —              │ —                   │
│ PRD-2026-06  │ 2026   │[orn●]CFO │ 30 Jun 17:05 │ (pending)      │ —                   │
│ PRD-2026-05  │ 2026   │[hjg]DONE │ 31 Mei 16:00 │ 2 Jun 09:45    │ ✓ 2 Jun 10:00       │
│ PRD-2026-04  │ 2026   │[hjg]DONE │ 30 Apr 15:30 │ 2 Mei 09:00    │ ✓ 2 Mei 09:20       │
│ ...          │        │          │              │                │                     │
├──────────────┴────────┴──────────┴──────────────┴────────────────┴─────────────────────┤
│ (klik baris → navigasi ke /periode-buku/{id})                                           │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ Footer: [← Prev]  Hal. 1 dari ~1  [Next →]  Baris: [50 ▾]  Total estimasi: 7           │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Kolom DataTable:**

| ID Kolom | Header | Sort | Filter | Format |
|---|---|---|---|---|
| `periode_kode` | Kode | Ya | Text search | — |
| `tahun_buku` | Thn | Ya | Select: 2024–2029 | Integer |
| `bulan` | Bln | Ya | Select: 1–12 | Integer |
| `tipe_periode` | Tipe | Tidak | Select enum | — |
| `status_periode` | Status | Ya | Select multi | `<PeriodeStatusBadge>` |
| `tanggal_soft_close` | Soft Close | Ya | Date range | `DD Mmm HH:mm` atau `—` |
| `soft_close_by` | Oleh | Tidak | — | nama user singkat |
| `tanggal_hard_close` | Hard Close | Ya | Date range | `DD Mmm HH:mm` atau `—` |
| `hard_close_by` | Oleh | Tidak | — | nama user singkat |
| `hard_close_grace_expires_at` | Grace Exp | Ya | Date range | `DD Mmm HH:mm` atau `—` |
| `reopened_flag` | Pernah Reopen | Ya | Boolean toggle | Ya/Tidak |
| `mv_refresh_status` | MV Refresh | Tidak | Select: completed/failed/running | ✓/✗/↺ |
| `checklist_last_snapshot.transition` | Checklist Terakhir | Tidak | — | badge transisi |

**Row action**: klik baris (atau [→] di setiap row) → navigasi ke `/periode-buku/{id}`.

**Export**: CSV/XLSX. Dataset diperkirakan ≤ 120 rows per 10 tahun → selalu inline (tidak async). Jika operator memilih export tanpa filter dan jumlah > 10k (tidak realistis untuk modul ini tapi defensive) → trigger async pattern §1.4 UX. Audit: `PERIODE.EXPORT` per export.

**Permission guard**: ROLE-MAKER-TR tanpa `periode.read` → redirect ke `/unauthorized`. Tampil untuk: ROLE-AKUN-CTL, ROLE-CFO, ROLE-RISK, ROLE-AKUN, ROLE-AUDIT.

---

## 4. Cross-Cutting Component: PeriodeLockBanner

### 4.1 Deskripsi

`<PeriodeLockBanner>` adalah komponen global yang muncul di bagian atas halaman transaksi, jurnal, dan instrumen yang memiliki `periode_id` aktif. Muncul otomatis jika periode tersebut dalam status `SOFT_CLOSED`, `HARD_CLOSE_PENDING`, atau `CLOSED`.

### 4.2 Wireframe

**STATE: SOFT_CLOSED**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ [⚠ amber] PERIODE SOFT-CLOSED — PRD-2026-06                                             │
│ Periode Juni 2026 sudah soft-closed pada 30 Jun 2026 17:05. Mutasi tidak diizinkan.    │
│ GL delivery retry masih diperbolehkan.                [→ Lihat detail periode]          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**STATE: HARD_CLOSE_PENDING**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ [● oranye pulse] PERIODE MENUNGGU APPROVAL CFO — PRD-2026-06                           │
│ Hard-close request diajukan oleh Budi S. Menunggu approval CFO dengan step-up MFA.     │
│ Mutasi tidak diizinkan.                               [→ Lihat detail periode]          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**STATE: CLOSED**

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ [🔒 hijau gelap] PERIODE DITUTUP FINAL — PRD-2026-06                                   │
│ Periode Juni 2026 hard-closed pada 2 Jul 2026 09:30. Mutasi tidak bisa dilakukan.      │
│ Hubungi CFO untuk reopen (grace window tersedia).    [→ Lihat detail periode]          │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

### 4.3 Props

```tsx
interface PeriodeLockBannerProps {
  periodeId: string;
  periodeKode: string;
  statusPeriode: 'SOFT_CLOSED' | 'HARD_CLOSE_PENDING' | 'CLOSED';
  tanggal: string;            // tanggal soft/hard close
  graceExpiresAt?: string;    // hanya jika CLOSED + dalam grace
}
```

**Perilaku:**
- Banner tidak muncul saat status `OPEN`
- Bukan dismissible — selalu tampil selama periode terkunci
- Link "[→ Lihat detail periode]" navigasi ke `/periode-buku/{periodeId}`
- Data periode di-fetch secara lazy saat halaman muat (tidak menghambat render halaman utama)
- Jika fetch gagal → banner tidak muncul (fail silent, agar tidak block kerja user)

**Implementasi injection**: di setiap page yang periode-aware (`/jrnl/journal-entries/[id]`, `/transaksi/penempatan/[id]`, dll.), tambahkan:

```tsx
// Di server component atau layout level
<PeriodeLockBanner periodeId={params.periodeId} />
```

---

## 5. Component Specifications

### 5.1 Komponen Baru (perlu dibuat di P5-M4)

#### `<PeriodeStatusBadge>` — `/components/blips/periode/PeriodeStatusBadge.tsx`

```tsx
interface PeriodeStatusBadgeProps {
  status: 'OPEN' | 'SOFT_CLOSED' | 'HARD_CLOSE_PENDING' | 'CLOSED';
  graceExpiresAt?: string;   // ISO datetime — untuk show countdown sub-badge
  size?: 'sm' | 'md';
}
```

Warna + ikon + teks per §2.1. `HARD_CLOSE_PENDING`: `animate-pulse` pada dot indicator. Untuk CLOSED dalam grace: render countdown sub-badge sebagai `<span>` sibling (bukan tooltip — harus selalu visible).

#### `<ClosingChecklistPanel>` — `/components/blips/periode/ClosingChecklistPanel.tsx`

```tsx
interface ClosingChecklistPanelProps {
  periodeId: string;
  statusPeriode: string;
  onAllPassed?: (allPassed: boolean) => void;  // callback untuk enable/disable action buttons
}
```

**Perilaku:**
- Load via `GET /api/v1/periode-buku/{id}/closing-checklist`
- Auto-refresh setiap 30 detik jika status `IN ['OPEN','SOFT_CLOSED','HARD_CLOSE_PENDING']`
- Berhenti refresh saat `CLOSED`
- Setiap item render: `<ChecklistItem key={item.key} passed={item.passed} detail={item.detail} actionUrl={item.actionUrl} />`
- Item yang gagal: tampilkan tombol "[Tindak Lanjut →]" jika `actionUrl` ada (bukan null) — opens in same tab (bukan new tab)
- Tombol refresh manual: "[↻ Evaluasi Ulang Checklist]" → force re-fetch + disable tombol 2 detik
- Loading state: 4 skeleton baris dengan shimmer
- Error state: "Gagal memuat checklist. [Coba lagi]" (merah outline card)
- Untuk periode `CLOSED`: tampilkan banner "Data historis dari snapshot terakhir" + info evaluasi real-time tidak dilakukan

#### `<MakerReviewerApproverPanel>` — `/components/blips/MakerReviewerApproverPanel.tsx`

(Pattern yang sudah ada — extend untuk periode close workflow)

```tsx
interface WorkflowStep {
  id: string;
  label: string;                    // "Soft-Close Request", "Hard-Close Approve", dll.
  status: 'done' | 'pending' | 'skipped';
  actor?: {
    name: string;
    role: string;
    userId: string;
  };
  timestamp?: string;
  comment?: string;
  snapshotId?: string;              // untuk link "Lihat snapshot →"
  checklistSummary?: {              // summary inline
    allPassed: boolean;
    total: number;
    passed: number;
  };
}

interface MakerReviewerApproverPanelProps {
  steps: WorkflowStep[];
  onSnapshotClick?: (snapshotId: string) => void;
}
```

**Perilaku:**
- Step `done`: baris hijau, collapsed dengan timestamp + actor + comment (truncated 80 char, tooltip full)
- Step `pending`: highlighted amber border kiri, expanded dengan label "Menunggu..."
- "Lihat snapshot →" membuka `<ChecklistSnapshotDetailDialog>`

#### `<ChecklistSnapshotDetailDialog>` — `/components/blips/periode/ChecklistSnapshotDetailDialog.tsx`

```tsx
interface ChecklistSnapshotDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  snapshotId: string;
}
```

**Perilaku:**
- Load via `GET /api/v1/periode-buku/{periodeId}/closing-checklist-snapshot/{snapshotId}` (atau include dalam periode detail response)
- Render `checklist_jsonb.items[]` — tiap item: icon pass/fail + label + detail teks + action_url jika ada
- Stale indicator: bandingkan `created_at` dengan request snapshot sebelumnya (jika ada)
- Loading: skeleton 4 item
- Error: "Snapshot tidak ditemukan" card

#### `<MFAStepUpDialog>` — `/components/blips/MFAStepUpDialog.tsx`

(Pattern sudah ada di P5-M2/M3 — konfirmasi ada atau buat baru)

Lihat wireframe §3 (SCREEN-P5-M4-03). Mendukung `scope: 'hard_close' | 'reopen_closed'`. Mengembalikan token via `onTokenReceived` callback.

#### `<PeriodeLockBanner>` — `/components/blips/periode/PeriodeLockBanner.tsx`

Lihat §4.

#### `<ClosingWorkflowActionBar>` — `/components/blips/periode/ClosingWorkflowActionBar.tsx`

Wrapper yang membungkus semua action buttons di WorkflowPanel (kanan), mengatur visibility per state + persona:

```tsx
interface ClosingWorkflowActionBarProps {
  periodeId: string;
  statusPeriode: string;
  pendingRequestBy?: string;      // UUID user yang submit soft-close request
  currentUserId: string;
  currentUserPermissions: string[];
  checklistAllPassed: boolean;
  rowVersion: number;
  graceExpiresAt?: string;
  onActionSuccess: (newStatus: string) => void;
}
```

**Visibility matrix** (client-side, bukan hanya disabled):

| Tombol | Tampil jika | Hidden jika |
|---|---|---|
| "Ajukan Soft Close" | status=OPEN + tidak ada pending + permission `periode.softclose.request` | Status ≠ OPEN atau ada pending atau tidak ada perm |
| "Approve Soft Close" | status=OPEN + ada pending + actor ≠ pendingRequestBy + permission `periode.softclose.approve` | SoD violation atau status ≠ OPEN |
| "Ajukan Hard Close" | status=SOFT_CLOSED + permission `periode.hardclose.request` | Status ≠ SOFT_CLOSED |
| "Approve Hard Close ●MFA" | status=HARD_CLOSE_PENDING + permission `periode.hardclose.approve` (ROLE-CFO) | Status ≠ HCP atau bukan CFO |
| "Tolak Hard Close" | status=HARD_CLOSE_PENDING + permission `periode.hardclose.approve` (ROLE-CFO) | Status ≠ HCP atau bukan CFO |
| "Reopen ke OPEN ↩" | status=SOFT_CLOSED + permission `periode.reopen.request` (ROLE-CFO) | Status ≠ SOFT_CLOSED atau bukan CFO |
| "Reopen ke SOFT_CLOSED ↩ ●MFA" | status=CLOSED + dalam grace window + permission `periode.reopen.request` (ROLE-CFO) | Status ≠ CLOSED, grace expired, atau bukan CFO |

Tombol **tidak di-render di DOM** jika kondisi tidak terpenuhi (bukan hanya `disabled` atau `visibility:hidden`).

### 5.2 Komponen Reuse dari Prior Milestones

| Komponen | Asal | Penggunaan di P5-M4 |
|---|---|---|
| `<DataTable>` | `components/blips/DataTable.tsx` | Periode list, Status periode report |
| `<DestructiveActionDialog>` | `components/blips/DestructiveActionDialog.tsx` | Hard-close approve, reopen approve, hard-close reject |
| `<JobProgressPanel>` | `components/blips/JobProgressPanel.tsx` | MV refresh job tracking setelah hard-close (compact mode) |
| `<JSONBTreeView>` | `components/blips/JSONBTreeView.tsx` | Snapshot checklist_jsonb display di ChecklistSnapshotDetailDialog |
| `<MFAStepUpModal>` | `components/blips/MFAStepUpModal.tsx` | Dikonfirmasi sebelum handoff — jika sudah ada dari P5-M2/M3 auth flow, reuse; jika belum, buat baru |
| `<MakerReviewerApproverPanel>` | `components/blips/MakerReviewerApproverPanel.tsx` | Extend untuk periode close workflow steps |

---

## 6. Interaction Patterns

### 6.1 Soft-Close Request Flow (S1)

```
1. ROLE-AKUN-CTL membuka /periode-buku/{id}
2. ClosingChecklistPanel menampilkan evaluasi real-time
   - Jika !allPassed → tombol "Ajukan Soft Close" disabled + tooltip "N item belum lolos"
   - Jika allPassed → tombol enabled
3. Klik "Ajukan Soft Close" → SoftCloseRequestDialog terbuka
4. User mengisi catatan (opsional, max 1000 char)
5. Submit → disabled button + spinner
6. Client: generate Idempotency-Key (uuidv4), POST /periode-buku/{id}/soft-close-request
   Body: { catatan, rowVersion }
7. 202 → dialog tutup, toast sukses, halaman refresh (GET periode detail)
8. WorkflowPanel: step "Ajukan Soft Close" → done + waktu + catatan
   Action card: berubah ke STATE "OPEN dengan pending request"
9. Notifikasi ke ROLE-AKUN-CTL lain (via sistem in-app notification — tidak di-scope UI ini)
```

**Error handling:**
- 422 `CLOSING_CHECKLIST_FAILED` → dialog tetap terbuka, tampilkan panel merah dengan item yang gagal + link action_url per item. Toast merah persisten: "Checklist gagal: {N} item belum terpenuhi — lihat detail."
- 409 `CONFLICT` (row_version) → toast merah: "Data periode sudah diubah oleh pengguna lain. Halaman akan di-refresh otomatis." + auto-refresh 2 detik
- 409 `SOFT_CLOSE_PENDING_EXISTS` → toast amber: "Sudah ada soft-close request yang menunggu approval. Refresh halaman untuk melihat status terbaru."
- 422 `WORKFLOW_INVALID_TRANSITION` → toast merah: "Periode bukan OPEN. Refresh halaman untuk melihat status terkini."

### 6.2 Soft-Close Approve Flow (S2)

```
1. ROLE-AKUN-CTL (approver, berbeda dari requester) membuka /periode-buku/{id}
2. Action card menampilkan STATE "OPEN dengan pending request"
   - Jika actor = requester → tampilkan pesan SoD, tombol "Approve" tidak di-render
   - Jika actor ≠ requester → tampilkan tombol "Approve Soft Close"
3. Klik "Approve Soft Close" → SoftCloseApproveDialog terbuka
   Dialog menampilkan: siapa yang mengajukan + kapan + catatan request + form komentar
4. User mengisi komentar + signatureMethod (hidden field: JWT_STEP_UP)
5. Submit → POST /periode-buku/{id}/soft-close-approve
6. 200 → dialog tutup, toast sukses, halaman refresh
7. Status berubah → SOFT_CLOSED, WorkflowPanel: step "Approve" → done
8. PeriodeLockBanner muncul di halaman transaksi/jurnal terkait (cross-cutting)
```

**Error handling:**
- 403 `SOD_VIOLATION` → toast merah persisten: "Anda tidak bisa meng-approve request soft-close yang Anda ajukan sendiri. SoD violation (DEC-017)." Tombol "Approve" langsung dihide (client state update)
- 422 `CLOSING_CHECKLIST_FAILED` (stale re-run) → toast merah: "Checklist dievaluasi ulang (> 24 jam sejak request). {N} item gagal. Lihat detail." + panel checklist refresh otomatis
- 422 `WORKFLOW_INVALID_TRANSITION` → toast merah: "Tidak ada soft-close request yang menunggu." + refresh halaman

### 6.3 Hard-Close Request Flow (S3a — ROLE-AKUN-CTL)

```
1. ROLE-AKUN-CTL membuka /periode-buku/{id} (status: SOFT_CLOSED)
2. Klik "Ajukan Hard Close" → HardCloseRequestDialog terbuka
   Dialog: warning bahwa checklist akan di-run ulang, form catatan
3. Submit → POST /periode-buku/{id}/hard-close-request
   Body: { catatan, rowVersion }
4. 202 → status berubah ke HARD_CLOSE_PENDING → dialog tutup, toast sukses
   Toast: "Hard-close request PRD-2026-06 berhasil diajukan. Menunggu approval CFO."
5. Action card berubah ke STATE "HARD_CLOSE_PENDING" (CFO buttons muncul)
6. Badge di side nav/top bar berkedip oranye
```

**Error handling:**
- 422 `CLOSING_CHECKLIST_FAILED` → dialog tetap terbuka, tampilkan item yang gagal. Toast: "Hard-close request ditolak: checklist tidak lolos. Lihat detail." (checklist panel refresh)

### 6.4 Hard-Close Approve Flow (S3b — ROLE-CFO + step-up MFA)

```
1. ROLE-CFO membuka /periode-buku/{id} (status: HARD_CLOSE_PENDING)
2. Action card: tombol "Approve Hard Close ●MFA" + "Tolak Hard Close"
3. Klik "Approve Hard Close ●MFA" → HardCloseApproveConfirmDialog terbuka
   (lihat wireframe §5a)
4. User centang atestasi + mengisi komentar
5. Klik "Lanjut ke Verifikasi MFA" → dialog tertutup → MFAStepUpDialog terbuka
6. User input TOTP → POST /auth/step-up { scope: "hard_close" }
7. 200 → { stepUpToken } (simpan di state)
8. MFAStepUpDialog: "Verifikasi berhasil, melanjutkan hard-close..." → auto-submit
9. POST /periode-buku/{id}/hard-close-approve
   Headers: X-Step-Up-Token, Idempotency-Key
   Body: { comment, signatureMethod: "JWT_STEP_UP" }
10. 200 → dialog tutup, toast sukses (amber, persisten 8 detik):
    "Periode PRD-2026-06 berhasil HARD CLOSED pada [timestamp]. FX rate terkunci.
     Grace window reopen: 48 jam. MV refresh dijadwalkan."
11. Status berubah → CLOSED. JobProgressPanel (compact) muncul di SECTION MV Refresh
12. Badge side nav berubah: HARD_CLOSE_PENDING badge hilang
```

**Error handling:**
- 401 `MFA_STEP_UP_REQUIRED` (token hilang) → MFAStepUpDialog buka otomatis. Toast amber: "Aksi memerlukan verifikasi MFA tambahan. Klik untuk memulai challenge." + auto-open
- 401 `MFA_STEP_UP_EXPIRED` → MFAStepUpDialog buka kembali dengan pesan "Token expired. Ulangi verifikasi MFA."
- 403 `FORBIDDEN` → tombol di-hide (client permission refresh diperlukan)

### 6.5 Hard-Close Reject Flow (S3c — ROLE-CFO, tanpa MFA)

```
1. ROLE-CFO klik "Tolak Hard Close"
2. HardCloseRejectDialog terbuka (wireframe §5c)
3. User isi alasan (≥ 30 char)
4. Submit → POST /periode-buku/{id}/hard-close-reject
5. 200 → status kembali ke SOFT_CLOSED
6. Toast: "Hard-close request PRD-2026-06 ditolak. Periode kembali ke SOFT_CLOSED."
7. Notifikasi ke ROLE-AKUN-CTL (sistem)
```

### 6.6 Reopen Flow (S4 — ROLE-CFO)

Dua sub-flow berdasarkan status sumber:

**Sub-flow A: SOFT_CLOSED → OPEN (tanpa step-up MFA)**

```
1. ROLE-CFO klik "Reopen ke OPEN ↩"
2. ReopenRequestDialog terbuka:
   - target_status: OPEN (read-only field)
   - Form alasan (≥ 30 char, ≤ 2000 char)
   - Tidak perlu checkbox atestasi MFA (karena step-up tidak diperlukan)
3. Submit → POST /periode-buku/{id}/reopen-request { targetStatus: "OPEN", reason, rowVersion }
4. 202 → intermediate state saved
5. Langsung lanjut: POST /periode-buku/{id}/reopen-approve { comment, signatureMethod: "JWT_STEP_UP" }
   (tanpa X-Step-Up-Token)
6. 200 → status kembali OPEN
7. Toast: "Periode PRD-2026-06 berhasil di-reopen ke OPEN. Alasan: [excerpt]. Transaksi dapat diinput kembali."
8. PeriodeLockBanner hilang dari halaman terkait
```

**Sub-flow B: CLOSED → SOFT_CLOSED (dengan step-up MFA)**

```
1. ROLE-CFO klik "Reopen ke SOFT_CLOSED ↩ ●MFA"
2. ReopenApproveConfirmDialog terbuka (wireframe §5b)
3. User isi alasan (≥ 30 char) + centang atestasi
4. Klik "Lanjut ke Verifikasi MFA" → MFAStepUpDialog terbuka (scope='reopen_closed')
5. User input TOTP → token diterima
6. POST /periode-buku/{id}/reopen-request { targetStatus: "SOFT_CLOSED", reason, rowVersion }
7. 202 → lanjut POST /periode-buku/{id}/reopen-approve
   Headers: X-Step-Up-Token
8. 200 → status berubah ke SOFT_CLOSED, FX rate di-unlock
9. Toast amber (8 detik): "PERHATIAN: Periode PRD-2026-06 di-reopen ke SOFT_CLOSED oleh CFO.
    FX rate di-unlock. Alasan: [excerpt]."
10. PeriodeLockBanner: berubah dari CLOSED → SOFT_CLOSED state
```

**Error handling reopen:**
- 423 `PERIODE_GRACE_EXPIRED` → ReopenDialog menutup, tampilkan card merah di WorkflowPanel: "Grace window telah berakhir pada [tanggal]. Reopen tidak dapat dilakukan via API. Ajukan RFC ke Direksi sesuai RACI BRD §3."
- 400 `VALIDATION_FAILED` (reason < 30 char) → inline error di textarea + counter merah
- 409 `CONFLICT` → toast merah: "row_version sudah berubah. Refresh dan coba lagi."

### 6.7 Checklist Polling dan Stale Warning

```
Di ClosingChecklistPanel:

- Saat status OPEN/SOFT_CLOSED/HARD_CLOSE_PENDING → polling setiap 30 detik
- Jika checklist sebelumnya all_passed=true tapi sekarang ada item yang gagal
  (kondisi berubah selama polling) → tampilkan banner kuning di atas panel:
  "⚠ Status checklist berubah — ada kondisi baru yang perlu diperhatikan."
  + highlight item yang berubah dengan animasi brief flash

- Soft-close approve stale check (24 jam):
  Server yang menentukan (backend re-run checklist jika stale).
  Frontend: tampilkan timestamp "Terakhir dievaluasi: {waktu}" di bawah panel header.
  Jika timestamp > 20 jam → tampilkan advisory: "Checklist akan dievaluasi ulang saat approval karena mendekati batas 24 jam."
```

---

## 7. Form Notification Copy (Bahasa Indonesia, Spesifik per §2 UX)

### 7.1 Toast Sukses (hijau, 4 detik kecuali noted)

| Trigger | Toast |
|---|---|
| Soft-close request sukses (S1-AC1) | "Permintaan soft-close PRD-2026-06 berhasil diajukan. Menunggu approval Finance Controller lain." |
| Soft-close approve sukses (S2-AC1) | "Periode PRD-2026-06 berhasil di-soft-close oleh Sari W. Mutasi transaksi diblokir. Siap untuk hard-close CFO." |
| Hard-close request sukses (S3-AC1 step 1) | "Hard-close request PRD-2026-06 berhasil diajukan. CFO Hendra G. akan mendapat notifikasi." |
| Hard-close approve sukses (S3-AC1 step 2) | **amber 8 detik** "Periode PRD-2026-06 berhasil HARD CLOSED pada [timestamp]. Grace window: 48 jam (berakhir [timestamp]). MV refresh dijadwalkan." |
| Hard-close reject sukses | "Hard-close request PRD-2026-06 ditolak. Periode kembali ke SOFT_CLOSED. AKUN-CTL akan mendapat notifikasi." |
| Reopen SOFT→OPEN sukses (S4-AC1) | **amber 8 detik** "PERHATIAN: Periode PRD-2026-06 berhasil di-reopen ke OPEN. Transaksi dapat diinput kembali. Audit trail tersimpan." |
| Reopen CLOSED→SOFT_CLOSED sukses (S4-AC2) | **amber 8 detik** "PERHATIAN: Periode PRD-2026-06 berhasil di-reopen ke SOFT_CLOSED. FX rate di-unlock. Audit trail tersimpan." |
| Export sukses (inline) | "Export status periode berhasil. File sedang diunduh." |

### 7.2 Toast Gagal (merah, persisten)

| Trigger | Toast |
|---|---|
| `CLOSING_CHECKLIST_FAILED` (S1-AC2, S2-AC3) | "Checklist gagal: {N} item belum terpenuhi — lihat panel checklist untuk detail dan tindak lanjut." |
| `SOD_VIOLATION` (S2-AC2) | "Anda tidak bisa meng-approve request soft-close yang Anda ajukan sendiri. Segregation of Duties wajib (DEC-017)." |
| `MFA_STEP_UP_REQUIRED` (S3-AC2) | "Hard-close periode buku wajib step-up MFA. Verifikasi ulang diperlukan." (auto-open MFAStepUpDialog) |
| `MFA_STEP_UP_EXPIRED` (S3-AC3) | "Token MFA sudah expired (> 5 menit). Ulangi verifikasi." (auto-open MFAStepUpDialog) |
| `PERIODE_GRACE_EXPIRED` (S4-AC4) | "Grace window reopen PRD-2026-06 telah berakhir. Ajukan RFC ke Direksi sesuai RACI BRD §3." |
| `WORKFLOW_INVALID_TRANSITION` | "Transisi tidak valid: [message dari server]. Refresh halaman untuk melihat status terkini." |
| `CONFLICT` (row_version) | "Data periode sudah diubah oleh pengguna lain. Refresh dan coba lagi." (+ auto-refresh 2 detik) |
| `SOFT_CLOSE_PENDING_EXISTS` | "Sudah ada soft-close request yang menunggu approval untuk PRD-2026-06. Refresh halaman." |
| `VALIDATION_FAILED` (reason < 30) | "Alasan reopen wajib minimal 30 karakter untuk audit compliance." (inline di field + toast) |
| `PERIODE_CLOSED` (cross-cutting, S3-AC4) | "Periode PRD-2026-06 sudah hard-closed. Mutasi tidak diizinkan. Hubungi CFO untuk reopen (hanya dalam grace window)." |
| `PERIODE_SOFT_CLOSED` (cross-cutting) | "Periode PRD-2026-06 sudah soft-closed. Mutasi tidak diizinkan. Hubungi Finance Controller untuk koreksi darurat." |

### 7.3 Toast Informasi (biru/amber)

| Trigger | Toast |
|---|---|
| MFA step-up required (auto-detect, pre-toast) | **amber 8 detik** "Aksi ini memerlukan verifikasi MFA tambahan. Dialog verifikasi akan terbuka." |
| Checklist stale warning (advisory) | **amber 8 detik** "Checklist mendekati batas waktu 24 jam. Evaluate ulang sebelum soft-close approve." |
| MV refresh dimulai (background) | **info 4 detik** "Materialized view laporan sedang di-refresh di latar belakang. Laporan akan ter-update dalam beberapa menit." |
| MV refresh selesai (SSE) | **sukses 4 detik** "Laporan periode PRD-2026-06 berhasil di-update." |

---

## 8. Accessibility

### 8.1 WCAG 2.1 AA

- Semua `<PeriodeStatusBadge>` menggunakan warna + ikon + teks. `HARD_CLOSE_PENDING` pulsing dot: `aria-label="Status: Menunggu Approval CFO"`, tidak hanya warna berkedip
- Contrast ratio: badge hijau gelap (green-700) di atas putih = 8.2:1 (pass). Badge amber di atas putih = 4.8:1 (pass). Oranye (orange-500) di atas putih: verify di implementasi, gunakan orange-600 jika perlu
- Countdown timer: angka dan satuan teks — bukan hanya animasi visual
- `prefers-reduced-motion`: disable `animate-pulse` di HARD_CLOSE_PENDING badge, tampilkan ikon statis clock-alert

### 8.2 Keyboard Navigation

- ClosingChecklistPanel: setiap item "Tindak Lanjut →" reachable via Tab, trigger Enter/Space
- WorkflowPanel action buttons: Tab order logical (dari atas ke bawah: aksi positif → aksi negatif → aksi sekunder)
- Dialog: focus trap aktif, Escape menutup dialog (kecuali MFAStepUpDialog yang sedang submit), focus return ke trigger button
- MFAStepUpDialog: input TOTP auto-focused saat dialog buka. Tab ke tombol "Verifikasi", Enter submit
- DataTable: row klik via Enter/Space, sort header via Enter

### 8.3 ARIA

- `<PeriodeStatusBadge>`: `role="status"` jika di area yang auto-update, `aria-live="polite"` untuk perubahan status
- ClosingChecklistPanel: `aria-label="Closing checklist periode {kode}"`. Item badge: `aria-label="[LOLOS/GAGAL]: {label item}"`
- MakerReviewerApproverPanel: `role="list"` untuk langkah-langkah, setiap step `role="listitem"` dengan `aria-current="step"` untuk step aktif
- Dialog: `role="dialog"`, `aria-labelledby` → ID judul, `aria-modal="true"`
- Textarea alasan/catatan: `aria-describedby` → ID counter + error message
- Tombol disabled dengan penjelasan: `aria-disabled="true"` + `aria-describedby` menunjuk ke teks penjelasan (bukan hanya `title` tooltip)
- PeriodeLockBanner: `role="alert"` agar screen reader umumkan segera

### 8.4 Screen Reader Copy

- Badge OPEN: "Status periode: Terbuka. Mutasi transaksi diizinkan."
- Badge SOFT_CLOSED: "Status periode: Soft-Closed. Mutasi transaksi diblokir. Menunggu hard-close dari CFO."
- Badge HARD_CLOSE_PENDING: "Status periode: Menunggu Approval CFO. Hard-close request sudah diajukan."
- Badge CLOSED (grace): "Status periode: Ditutup Final. Grace window reopen tersisa: 23 jam 45 menit."
- Checklist item LOLOS: "Item checklist lolos: {label}. Detail: {detail}"
- Checklist item GAGAL: "Item checklist gagal: {label}. {detail}. Tombol tindak lanjut tersedia."

---

## 9. Bahasa Indonesia Copy Reference

| Konsep | Label ID | Label EN (export/report) |
|---|---|---|
| Periode buku | Periode Buku | Accounting Period |
| status_periode OPEN | Terbuka | Open |
| status_periode SOFT_CLOSED | Soft-Closed | Soft Closed |
| status_periode HARD_CLOSE_PENDING | Menunggu CFO | Pending CFO Approval |
| status_periode CLOSED | Ditutup Final | Closed |
| Soft-close request | Ajukan Soft Close | Submit Soft-Close Request |
| Soft-close approve | Setujui Soft Close | Approve Soft Close |
| Hard-close request | Ajukan Hard Close | Submit Hard-Close Request |
| Hard-close approve | Setujui Hard Close | Approve Hard Close |
| Hard-close reject | Tolak Hard Close | Reject Hard Close |
| Reopen | Buka Kembali | Reopen |
| Grace window | Grace Window Reopen | Reopen Grace Window |
| Checklist | Checklist Penutupan | Closing Checklist |
| Checklist PENDING_APPROVAL_ZERO | 0 transaksi/jurnal masih PENDING | Pending Approvals = 0 |
| Checklist JURNAL_BALANCED | Semua jurnal seimbang | Journals Balanced |
| Checklist GL_DELIVERED | Tidak ada GL delivery FAILED | No Failed GL Deliveries |
| Checklist RECON_PASS | Rekonsiliasi GL terakhir COMPLETED | GL Recon: Passed |
| Snapshot checklist | Snapshot Checklist | Checklist Snapshot |
| Transition SOFT_CLOSE_REQUEST | Soft-Close Request | Soft-Close Requested |
| Transition SOFT_CLOSE_APPROVE | Soft-Close Approved | Soft-Close Approved |
| Transition HARD_CLOSE_REQUEST | Hard-Close Request | Hard-Close Requested |
| Transition HARD_CLOSE_APPROVE | Hard-Close Approved | Hard-Close Approved |
| Transition REOPEN_REQUEST | Reopen Request | Reopen Requested |
| Transition REOPEN_APPROVE | Reopen Approved | Reopen Approved |
| Step-up MFA | Verifikasi MFA Tambahan | MFA Step-Up Verification |
| FX rate terkunci | FX Rate Terkunci | FX Rate Locked |
| MV refresh | Pembaruan Laporan | Report MV Refresh |
| Catatan | Catatan | Notes |
| Alasan | Alasan | Reason |
| Komentar approval | Komentar Approval | Approval Comment |
| tanggal_soft_close | Tgl Soft Close | Soft-Close Date |
| tanggal_hard_close | Tgl Hard Close | Hard-Close Date |
| hard_close_grace_expires_at | Berakhir (Grace) | Grace Expiry |
| soft_close_requested_by | Diminta Oleh | Requested By |
| soft_close_approved_by | Disetujui Oleh | Approved By |
| hard_close_requested_by | Hard Close Diminta Oleh | Hard-Close Requested By |
| hard_close_approved_by | Hard Close Disetujui CFO | Hard-Close Approved By (CFO) |
| reopened_flag | Pernah Dibuka Kembali | Previously Reopened |
| reopened_reason | Alasan Reopen | Reopen Reason |

---

## 10. Persona Gating — Explicit Rendering Rules

Aturan berikut berlaku di client-side rendering. Server tetap enforce — client gating hanya untuk UX.

### 10.1 Halaman /periode-buku (list)

| Elemen | Visible untuk | Hidden untuk |
|---|---|---|
| Inline "Ajukan Soft Close" button | ROLE-AKUN-CTL + status=OPEN | Semua lainnya |
| Inline "Approve Soft Close" button | ROLE-AKUN-CTL + status=OPEN + actor≠requester | Requester sendiri, non-CTL, status≠OPEN |
| Inline "Ajukan Hard Close" button | ROLE-AKUN-CTL + status=SOFT_CLOSED | Semua lainnya |
| Inline "Approve Hard Close" button | ROLE-CFO + status=HARD_CLOSE_PENDING | Non-CFO, status≠HCP |
| Inline "Tolak Hard Close" button | ROLE-CFO + status=HARD_CLOSE_PENDING | Non-CFO, status≠HCP |
| Inline "Reopen" button (SOFT_CLOSED) | ROLE-CFO + status=SOFT_CLOSED | Non-CFO, status≠SOFT_CLOSED |
| Inline "Reopen" button (CLOSED) | ROLE-CFO + status=CLOSED + dalam grace | Non-CFO, status≠CLOSED, grace expired |

### 10.2 Halaman /periode-buku/[id]

| Elemen | Visible untuk | Hidden untuk |
|---|---|---|
| "Ajukan Soft Close" | ROLE-AKUN-CTL + status=OPEN + tidak ada pending | Semua lainnya |
| "Approve Soft Close" | ROLE-AKUN-CTL + status=OPEN + ada pending + actor≠requester | SoD violation (requester=actor), non-CTL, status≠OPEN |
| SoD warning banner | ROLE-AKUN-CTL + actor=requester + ada pending | Semua lainnya |
| "Ajukan Hard Close" | ROLE-AKUN-CTL + status=SOFT_CLOSED | Semua lainnya |
| "Approve Hard Close ●MFA" | ROLE-CFO + status=HARD_CLOSE_PENDING | Non-CFO, status≠HCP |
| "Tolak Hard Close" | ROLE-CFO + status=HARD_CLOSE_PENDING | Non-CFO, status≠HCP |
| "Reopen ke OPEN ↩" | ROLE-CFO + status=SOFT_CLOSED | Non-CFO, status≠SOFT_CLOSED |
| "Reopen ke SOFT_CLOSED ↩ ●MFA" | ROLE-CFO + status=CLOSED + grace aktif | Non-CFO, status≠CLOSED, grace expired |
| Grace expired card (CFO only) | ROLE-CFO + status=CLOSED + grace expired | Non-CFO |
| MV Refresh section | Semua + `periode.read` + status=CLOSED | Status≠CLOSED |
| Export button (report) | `periode.export` | Non-export permission |

### 10.3 Cross-cutting PeriodeLockBanner

| Status periode | Tampil di halaman | Tidak tampil jika |
|---|---|---|
| SOFT_CLOSED / HARD_CLOSE_PENDING / CLOSED | `/jrnl/journal-entries/*`, `/transaksi/*`, `/ecl/*` dengan periode_id tersebut | Status OPEN, atau tidak ada periode_id terkait halaman |

---

## 11. Hand-off untuk Frontend Engineer Next.js

### 11.1 File Structure

```
frontend/src/app/periode-buku/
├── page.tsx                              — SCREEN-P5-M4-01 Periode list
└── [id]/
    └── page.tsx                          — SCREEN-P5-M4-02 Periode detail + workflow

frontend/src/app/reports/
└── status-periode/
    └── page.tsx                          — SCREEN-P5-M4-06 Status periode report

frontend/src/components/blips/periode/
├── PeriodeStatusBadge.tsx
├── ClosingChecklistPanel.tsx
├── ChecklistItem.tsx                     — sub-komponen
├── ChecklistSnapshotDetailDialog.tsx
├── ClosingWorkflowActionBar.tsx
├── PeriodeLockBanner.tsx
├── SoftCloseRequestDialog.tsx
├── SoftCloseApproveDialog.tsx
├── HardCloseRequestDialog.tsx
├── HardCloseApproveConfirmDialog.tsx     — destructive confirm (sebelum MFA)
├── HardCloseRejectDialog.tsx
├── ReopenRequestDialog.tsx
├── ReopenApproveConfirmDialog.tsx        — destructive confirm (sebelum MFA)
└── MvRefreshStatusCard.tsx

frontend/src/components/blips/
├── MFAStepUpDialog.tsx                   — baru (atau extend existing)
└── MakerReviewerApproverPanel.tsx        — extend untuk periode workflow

frontend/src/lib/
├── periode.api.ts                        — API client (TanStack Query hooks)
├── periode.store.ts                      — Zustand store
└── periode.schema.ts                     — Zod schemas
```

### 11.2 shadcn/ui Components yang Digunakan

| shadcn component | Digunakan untuk |
|---|---|
| `Card` | ClosingChecklistPanel, WorkflowPanel action card, MvRefreshStatusCard |
| `Dialog` | Semua modal dialog (request, approve, reject, reopen, MFA, snapshot detail) |
| `Badge` | PeriodeStatusBadge, ChecklistItem badge, transition badge |
| `Alert` | PeriodeLockBanner, warning panel di dialog destructive |
| `Collapsible` | MakerReviewerApproverPanel collapsed steps |
| `Skeleton` | Loading states semua panel |
| `Textarea` | Catatan/alasan/komentar di semua dialog |
| `Checkbox` | Atestasi di dialog destructive |
| `Progress` | Grace window countdown (tidak progress bar — teks countdown cukup) |
| `Separator` | Section dividers di dialog |
| `Tooltip` | Penjelasan tombol disabled, badge detail, SoD explanation |
| `ScrollArea` | ChecklistSnapshotDetailDialog jika checklist_jsonb panjang |
| `InputOTP` | Input kode TOTP di MFAStepUpDialog |

### 11.3 Zod Schemas (`periode.schema.ts`)

```ts
const SoftCloseRequestSchema = z.object({
  catatan: z.string().max(1000).optional(),
  rowVersion: z.number().int().min(1, "rowVersion wajib untuk optimistic lock"),
});

const SoftCloseApproveSchema = z.object({
  comment: z
    .string()
    .min(1, "Komentar approval wajib diisi")
    .max(1000),
  signatureMethod: z.enum(["JWT_STEP_UP", "JWT_STANDARD"]),
});

const HardCloseRequestSchema = z.object({
  catatan: z.string().max(1000).optional(),
  rowVersion: z.number().int().min(1),
});

const HardCloseApproveSchema = z.object({
  comment: z
    .string()
    .min(1, "Komentar approval wajib diisi")
    .max(1000),
  signatureMethod: z.enum(["JWT_STEP_UP"]),
  // stepUpToken: dihandle via X-Step-Up-Token header, bukan body
});

const HardCloseRejectSchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan penolakan wajib minimal 30 karakter")
    .max(1000),
});

const ReopenRequestSchema = z.object({
  targetStatus: z.enum(["OPEN", "SOFT_CLOSED"]),
  reason: z
    .string()
    .min(30, "Alasan reopen wajib minimal 30 karakter untuk audit compliance")
    .max(2000),
  rowVersion: z.number().int().min(1),
});

const ReopenApproveSchema = z.object({
  comment: z.string().min(1).max(1000),
  signatureMethod: z.enum(["JWT_STEP_UP", "JWT_STANDARD"]),
  // X-Step-Up-Token via header (hanya jika CLOSED→SOFT_CLOSED)
});

// Response shapes
const PeriodeStatusBadgeStatusSchema = z.enum([
  "OPEN", "SOFT_CLOSED", "HARD_CLOSE_PENDING", "CLOSED"
]);

const ChecklistItemSchema = z.object({
  key: z.enum([
    "PENDING_APPROVAL_ZERO",
    "JURNAL_BALANCED",
    "GL_DELIVERED",
    "RECON_PASS"
  ]),
  label: z.string(),
  passed: z.boolean(),
  detail: z.string(),
  actionUrl: z.string().url().nullable(),
});

const ClosingChecklistResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  statusPeriode: PeriodeStatusBadgeStatusSchema,
  evaluatedAt: z.string().datetime(),
  allPassed: z.boolean(),
  isRealTimeEval: z.boolean(),
  items: z.array(ChecklistItemSchema),
  lastSnapshot: z.object({
    snapshotId: z.string().uuid(),
    transition: z.string(),
    evaluatedAt: z.string().datetime(),
    allPassed: z.boolean(),
  }).nullable(),
  mvRefresh: z.object({
    jobId: z.string(),
    status: z.enum(["queued", "running", "completed", "failed"]),
    completedAt: z.string().datetime().nullable(),
  }).nullable(),
});

const PeriodeBukuDetailSchema = z.object({
  id: z.string().uuid(),
  periodeKode: z.string(),
  tipePeriode: z.enum(["BULANAN", "KUARTALAN", "TAHUNAN"]),
  tahunBuku: z.number().int(),
  bulan: z.number().int(),
  tanggalMulai: z.string(),
  tanggalAkhir: z.string(),
  statusPeriode: PeriodeStatusBadgeStatusSchema,
  tanggalSoftClose: z.string().datetime().nullable(),
  tanggalHardClose: z.string().datetime().nullable(),
  hardCloseGraceExpiresAt: z.string().datetime().nullable(),
  softCloseRequestedBy: z.string().uuid().nullable(),
  softCloseRequestedAt: z.string().datetime().nullable(),
  reopenedFlag: z.boolean(),
  reopenedReason: z.string().nullable(),
  rowVersion: z.number().int(),
});
```

### 11.4 API Client Hooks (`periode.api.ts`)

```ts
// Periode list
usePeriodeBukuList(params: PeriodeBukuListParams)
  // GET /api/v1/periode-buku (cursor paginated)

// Periode detail
usePeriodeBukuDetail(id: string)
  // GET /api/v1/periode-buku/{id}

// Closing checklist (polling 30s)
useClosingChecklist(id: string, enabled: boolean)
  // GET /api/v1/periode-buku/{id}/closing-checklist
  // refetchInterval: enabled ? 30_000 : false

// Mutations — semua wajib Idempotency-Key baru per submit
useSoftCloseRequest()
  // POST /api/v1/periode-buku/{id}/soft-close-request

useSoftCloseApprove()
  // POST /api/v1/periode-buku/{id}/soft-close-approve

useHardCloseRequest()
  // POST /api/v1/periode-buku/{id}/hard-close-request

useHardCloseApprove()
  // POST /api/v1/periode-buku/{id}/hard-close-approve
  // Extra header: X-Step-Up-Token

useHardCloseReject()
  // POST /api/v1/periode-buku/{id}/hard-close-reject

useReopenRequest()
  // POST /api/v1/periode-buku/{id}/reopen-request

useReopenApprove()
  // POST /api/v1/periode-buku/{id}/reopen-approve
  // Extra header: X-Step-Up-Token (conditional)

// MFA step-up
useMfaStepUp()
  // POST /auth/step-up { scope: "hard_close" | "reopen_closed" }
  // → { stepUpToken: string }

// Status report
useStatusPeriodeReport(params: StatusPeriodeReportParams)
  // GET /api/v1/reports/status-periode

// Job tracking (reuse dari P5-M3)
useJobStatus(jobId: string | null)
  // GET /api/v1/jobs/{jobId}
```

**TanStack Query key factory:**

```ts
export const periodeKeys = {
  all: ['periode-buku'] as const,
  lists: () => [...periodeKeys.all, 'list'] as const,
  list: (params: PeriodeBukuListParams) => [...periodeKeys.lists(), params] as const,
  detail: (id: string) => [...periodeKeys.all, 'detail', id] as const,
  checklist: (id: string) => [...periodeKeys.all, 'checklist', id] as const,
  statusReport: (params: StatusPeriodeReportParams) =>
    ['status-periode-report', params] as const,
};
```

### 11.5 Zustand Store (`periode.store.ts`)

```ts
interface PeriodeStore {
  // Badge count untuk HARD_CLOSE_PENDING (top nav + side nav)
  hardClosePendingCount: number;
  setHardClosePendingCount: (count: number) => void;

  // Step-up token (in-memory, tidak persist ke localStorage)
  // Disimpan hanya saat flow berlangsung, di-clear setelah dipakai atau timeout
  stepUpToken: string | null;
  stepUpTokenExpiresAt: number | null;  // unix timestamp
  setStepUpToken: (token: string, expiresAt: number) => void;
  clearStepUpToken: () => void;
  isStepUpTokenValid: () => boolean;    // now() < expiresAt
}
```

### 11.6 Route Map

| Route | Component | Guard (permission) |
|---|---|---|
| `/periode-buku` | Periode list | `periode.read` |
| `/periode-buku/[id]` | Periode detail + workflow | `periode.read` |
| `/reports/status-periode` | Status periode DataTable | `periode.read` |

Permission tidak terpenuhi → redirect ke `/unauthorized`.

### 11.7 Validation Rules (Checklist untuk Engineer)

Frontend validasi (Zod + React Hook Form):

- [ ] `rowVersion`: required di semua request yang menyertakannya, kirim nilai terbaru dari halaman (dari GET detail response). Jika CONFLICT → refresh ulang dan ambil rowVersion terbaru.
- [ ] `catatan`/`comment`/`reason`: counter karakter real-time di semua textarea
- [ ] `reason` (hard-close reject, reopen request): tombol submit disabled sampai ≥ 30 karakter
- [ ] Checkbox atestasi (hard-close approve, reopen CLOSED→SOFT_CLOSED): tombol "Lanjut" disabled sampai di-centang
- [ ] Tombol "Approve Soft Close": visible hanya jika `currentUserId ≠ softCloseRequestedBy` (client check, server juga enforce)
- [ ] MFA TOTP input: accept 6 digit, format `000-000` untuk readability, auto-submit saat digit ke-6 diisi
- [ ] `Idempotency-Key`: generate `uuidv4()` baru per setiap submit attempt (bukan cached)
- [ ] Grace window countdown: update via `setInterval(60_000)` — akurat sampai menit
- [ ] `stepUpToken` validity check sebelum submit hard-close-approve/reopen-approve: jika expired → re-open MFAStepUpDialog sebelum submit

### 11.8 Permission Checks (Client-side, tidak di DOM jika false)

| Elemen | Permission | Jika tidak ada |
|---|---|---|
| "Ajukan Soft Close" | `periode.softclose.request` | Tidak di-render |
| "Approve Soft Close" | `periode.softclose.approve` | Tidak di-render |
| "Ajukan Hard Close" | `periode.hardclose.request` | Tidak di-render |
| "Approve Hard Close ●MFA" | `periode.hardclose.approve` | Tidak di-render |
| "Tolak Hard Close" | `periode.hardclose.approve` | Tidak di-render |
| "Reopen" (SOFT_CLOSED→OPEN) | `periode.reopen.request` + `periode.reopen.approve` | Tidak di-render |
| "Reopen" (CLOSED→SOFT_CLOSED) | `periode.reopen.request` + `periode.reopen.approve` | Tidak di-render |
| Export status periode | `periode.export` | Disabled + tooltip |
| Akses /periode-buku | `periode.read` | 403 redirect |
| Akses /reports/status-periode | `periode.read` | 403 redirect |

---

## 12. Anti-pattern Notes

Anti-pattern yang dihindari di design ini:

- **Modals stacking**: `<DestructiveActionDialog>` (atestasi) dan `<MFAStepUpDialog>` tidak tampil bersamaan. Urutan: destructive confirm tertutup → MFA dialog terbuka. Satu dialog per waktu.
- **Auto-saving workflow form**: Semua action (request, approve, reject, reopen) menggunakan explicit submit. Tidak ada auto-save.
- **Hiding workflow state behind tab**: Status workflow, checklist, dan riwayat transisi selalu visible di main content + right panel, bukan tersembunyi di tab.
- **Toast as only confirmation for irreversible action**: Hard-close approve memiliki `<DestructiveActionDialog>` dengan checkbox atestasi + MFA step-up, sebelum aksi dilakukan. Toast hanya konfirmasi setelah selesai.
- **Color sole signal**: Semua badge status menggunakan warna + ikon + teks. HARD_CLOSE_PENDING menggunakan pulsing dot + teks "Menunggu CFO" + ikon clock-alert.
- **Discard/approve button disabled vs hidden**: Tombol aksi yang tidak boleh diakses persona tertentu (mis. "Approve Hard Close" untuk non-CFO) tidak di-render di DOM sama sekali — bukan hanya `disabled` atau `visibility:hidden`.
- **Workflow state behind navigation**: PeriodeLockBanner muncul di halaman transaksi/jurnal terkait tanpa perlu user mengunjungi halaman periode terlebih dahulu.
- **Checklist hanya satu kali**: Checklist di-poll setiap 30 detik dan ada tombol refresh manual — operator selalu melihat kondisi terbaru.
- **Toast saja untuk PERIODE_CLOSED**: PeriodeLockBanner tampil persisten di halaman yang diblokir — bukan hanya toast setelah operasi gagal.

---

_Dokumen ini siap dihandoff ke `frontend-engineer-nextjs`. Backend contracts ada di `api/openapi/app-d-periode-close.yaml` dan `docs/state-machines/p5-m4-periode-close.md`. Migration 000038 di-handoff ke `data-modeler`. Security-engineer BLOCKING gate sebelum implementasi hard-close approve. ifrs9-compliance-reviewer BLOCKING gate sebelum UAT._
