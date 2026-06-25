# P5-M17 — APP-D Frontend: Periode Buku, Kurs, Mapping Jurnal, Jurnal Header, Reconciliation: UI/UX Design Specification

**Story Set**: P5-M17
**Modul**: APP-D — Periode Buku + FX + Mapping Jurnal + Jurnal Engine + GL Delivery (Frontend Consolidation)
**Desainer**: uiux-designer
**Tanggal**: 2026-06-25
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M17-fe-app-d.md`
**Decisions applied**:
- DEC-002 — Next.js 14+ App Router, TypeScript strict, shadcn/ui, React Hook Form + Zod, Zustand, TanStack Query
- DEC-005 — GL Integration Phase 1 file batch; reconciliation UI = read-only; no manual recon trigger
- DEC-017 — 4-eyes workflow rutin; 6-eyes untuk mapping jurnal; SoD maker ≠ reviewer ≠ approver
- DEC-018 — audit trail; `{ENTITY}.EXPORT` on every export
- DEC-021 — Idempotency-Key wajib di setiap mutation endpoint
- DEC-022 — cursor-based pagination only
- DEC-026 — MFA mandatory: ROLE-AKUN-CTL, ROLE-CFO, ROLE-KOMITE, ROLE-ALCO
- DEC-027 — step-up MFA: hard-close periode, DLQ replay, mapping-jurnal approve-2 (final 6-eyes)

**Gate**: `security-engineer` BLOCKING — absent-from-DOM per role pada semua tombol mutasi dan aksi sensitif; MFA step-up hard-close + DLQ replay + mapping approve-2: token dari Keycloak, diverifikasi server-side sebelum execute; SoD 6-eyes enforced di server (bukan hanya UI hide); 308 redirect tidak partial-render sebelum redirect; Idempotency-Key auto-inject via shared form hook.

---

## 1. Information Architecture

### 1.1 Layout Hierarchy — Empat Shared Layouts

M17 memperkenalkan atau merevisi empat shared layout yang bekerja secara independen. Tidak ada layout yang bersarang ke dalam layout lain — masing-masing memiliki segmen root sendiri.

```
/periode-buku/layout.tsx        ← NEW: vertical timeline left + content right; no tab nav
/master/layout.tsx              ← EXISTING: tambah tab Kurs + Mapping Jurnal; hapus tab Periode Buku
/jurnal/layout.tsx              ← NEW: tab nav 3 slot (Header | DLQ | Resolve)
/reconciliation/layout.tsx      ← NEW (minimal): single-page, no tab; breadcrumb only
```

#### `/periode-buku/layout.tsx` — Vertical Timeline Layout

Bukan tab nav. Layout ini menggunakan struktur dua kolom:
- **Kolom kiri (col-3)**: daftar periode sebagai timeline visual vertikal, urutan kronologis desc. Setiap item menampilkan kode periode, nama, tahun-bulan, dan `PeriodeStatusBadge`. Item yang sedang aktif (match `[id]`) di-highlight.
- **Kolom kanan (col-9)**: konten halaman (`{children}` — list view atau detail+workflow).

Pada route `/periode-buku` (list), kolom kanan menampilkan DataTable full-width. Pada route `/periode-buku/[id]`, kolom kanan menampilkan detail + workflow panel.

Timeline sidebar hanya dirender bila user memiliki `periode.read`. Bila tidak: kolom kiri absent dari DOM, konten full-width.

#### `/master/layout.tsx` — Tab Nav (Revised)

Tab "Periode Buku" dihapus. Tab setelah M17:

| Tab | Route | Permission | Keterangan |
|---|---|---|---|
| Instrumen | `/master/instrumen` | `instrumen.read` | Existing (M11) |
| Counterparty | `/master/counterparty` | `counterparty.read` | Existing |
| Portofolio | `/master/portofolio` | `portofolio.read` | Existing |
| Bank / COA | `/master/coa` | `coa.read` | Existing |
| Kurs | `/master/kurs` | `fx_rate.read` | Existing (M5) — M17 audit |
| Mapping Jurnal | `/master/mapping-jurnal` | `mapping_jurnal.read` | Existing (M12) — M17 audit 6-eyes UI |
| Rating | `/master/pd-pefindo` | `pd.read` | Existing |
| LGD / LPS | `/master/lgd-basel` | `lgd.read` | Existing |

Tab absent dari DOM bila user tidak memiliki permission. Server Component reads JWT server-side — tidak ada flash.

#### `/jurnal/layout.tsx` — Tab Nav 3 Slot

```
Header | DLQ [badge] | Resolve (IT-ADMIN only)
```

| Tab | Route | Permission | Badge |
|---|---|---|---|
| Header | `/jurnal/header` | `jurnal.read` | — |
| DLQ | `/jurnal/dlq` | `jurnal.dlq.read` | count PENDING DLQ entries (dari summary endpoint) |
| Resolve | `/jurnal/resolve` | `jurnal.resolve` (IT-ADMIN only) | — |

DLQ badge: count dari `GET /api/v1/jurnal/dlq?filter[status]=PENDING` summary. Fetch sekali saat layout mount. Badge absent bila count = 0 atau permission tidak ada.

#### `/reconciliation/layout.tsx` — Minimal Layout

Tidak ada tab. Breadcrumb + page header + `{children}`. Single route: `/reconciliation/daily`.

---

### 1.2 Screen Inventory

```
frontend/src/app/
  periode-buku/
    layout.tsx                      NEW — vertical timeline sidebar + content col
    page.tsx                        UPGRADE (M4) — DataTable list dengan timeline view + action buttons
    new/page.tsx                    git mv dari /master/periode-buku/new/
    [id]/
      page.tsx                      UPGRADE (M4) — detail + closing workflow panel + MFA step-up wiring
      edit/page.tsx                 git mv dari /master/periode-buku/[id]/edit/
      history/page.tsx              git mv dari /master/periode-buku/[id]/history/

  master/
    layout.tsx                      EXISTING — remove "Periode Buku" tab
    kurs/
      page.tsx                      AUDIT + FIX — DataTable UX §1 gaps
      new/page.tsx                  AUDIT + FIX — form notif UX §2 gaps
      [id]/page.tsx                 EXISTING — no change needed
      [id]/edit/page.tsx            AUDIT + FIX — form notif UX §2 gaps
      [id]/history/page.tsx         EXISTING — AuditHistoryTable already present
      upload/page.tsx               AUDIT + FIX — add KursUploadDropzone + JobProgressPanel §3
      jisdor-sync/page.tsx          AUDIT + FIX — add JisdorSyncTriggerButton + JobProgressPanel §3
    mapping-jurnal/
      page.tsx                      AUDIT + FIX — DataTable UX §1; WorkflowStatusBadge per step
      new/page.tsx                  AUDIT + FIX — MappingJurnalForm; form notif UX §2
      [id]/page.tsx                 AUDIT + FIX — SixEyesWorkflowPanel; per-step button gating
      [id]/edit/page.tsx            AUDIT + FIX — form notif UX §2
      [id]/history/page.tsx         EXISTING — AuditHistoryTable; audit history tab
    periode-buku/                   308 redirect ke /periode-buku/* (all sub-paths)

  jurnal/
    layout.tsx                      NEW — tab nav 3 slot + JurnalTabNav component
    header/
      page.tsx                      NEW — list jurnal header (DataTable UX §1)
      [id]/page.tsx                 NEW — detail + line items table + 4-eyes approval panel
    dlq/
      page.tsx                      git mv dari /jrnl/dlq/page.tsx
      [id]/page.tsx                 git mv dari /jrnl/dlq/[id]/page.tsx
    resolve/
      page.tsx                      git mv dari /jrnl/resolve/page.tsx (IT-ADMIN only)

  reconciliation/
    layout.tsx                      NEW (minimal)
    daily/
      page.tsx                      NEW — BLIPS vs GL recon view (read-only DataTable)

  mapping-jurnal/                   308 redirect ke /master/mapping-jurnal/*
  jrnl/
    mapping/                        308 redirect ke /master/mapping-jurnal/*
    journal-entries/                308 redirect ke /jurnal/header/*
    dlq/                            308 redirect ke /jurnal/dlq/*
    resolve/                        308 redirect ke /jurnal/resolve
    rekonsiliasi/                   308 redirect ke /reconciliation/daily
    gl-delivery-dlq/                308 redirect ke /jurnal/dlq/*
    post/                           308 redirect ke /jurnal/header

frontend/src/components/blips/
  jurnal/
    JurnalTabNav.tsx                NEW — "use client" island; 3-tab nav dengan badge
    index.ts                        UPDATE — add JurnalTabNav to barrel
  (SixEyesWorkflowPanel.tsx — EXISTING, sudah benar; tidak perlu perubahan)
  (MFAStepUpModal.tsx — EXISTING M4; tidak dimodifikasi)

next.config.js                      UPDATE — tambah 22 redirect rules (append ke 10 dari M16)
```

**Total route baru/upgrade**: 18 deliverable per stories (lihat M17 story table).

---

## 2. Redirect Strategy — `next.config.js`

### 2.1 Semua 22 Redirect Rules M17

Append ke array redirects M16 yang sudah ada (10 rules). Urutan penting: specific path sebelum wildcard.

```js
// M17 redirects — append ke M16 list; total 32 rules setelah merge

// Periode Buku — dari /master/periode-buku/* ke /periode-buku/*
{ source: '/master/periode-buku/new',                  destination: '/periode-buku/new',                   permanent: true },
{ source: '/master/periode-buku/:id/edit',             destination: '/periode-buku/:id/edit',              permanent: true },
{ source: '/master/periode-buku/:id/history',          destination: '/periode-buku/:id/history',           permanent: true },
{ source: '/master/periode-buku/:id',                  destination: '/periode-buku/:id',                   permanent: true },
{ source: '/master/periode-buku',                      destination: '/periode-buku',                       permanent: true },

// Mapping Jurnal namespace 1 — dari /mapping-jurnal/* ke /master/mapping-jurnal/*
{ source: '/mapping-jurnal/import',                    destination: '/master/mapping-jurnal/new',          permanent: true },
{ source: '/mapping-jurnal/:event_code',               destination: '/master/mapping-jurnal',              permanent: true },
{ source: '/mapping-jurnal',                           destination: '/master/mapping-jurnal',              permanent: true },

// Mapping Jurnal namespace 2 — dari /jrnl/mapping/* ke /master/mapping-jurnal/*
{ source: '/jrnl/mapping/new',                         destination: '/master/mapping-jurnal/new',          permanent: true },
{ source: '/jrnl/mapping/:id/edit',                    destination: '/master/mapping-jurnal/:id/edit',     permanent: true },
{ source: '/jrnl/mapping/:id',                         destination: '/master/mapping-jurnal/:id',          permanent: true },
{ source: '/jrnl/mapping',                             destination: '/master/mapping-jurnal',              permanent: true },

// Jurnal namespaces — /jrnl/* ke /jurnal/* dan /reconciliation/daily
{ source: '/jrnl/journal-entries/:id',                 destination: '/jurnal/header/:id',                  permanent: true },
{ source: '/jrnl/journal-entries',                     destination: '/jurnal/header',                      permanent: true },
{ source: '/jrnl/gl-delivery-dlq/:id',                 destination: '/jurnal/dlq/:id',                     permanent: true },
{ source: '/jrnl/gl-delivery-dlq',                     destination: '/jurnal/dlq',                         permanent: true },
{ source: '/jrnl/dlq/:id',                             destination: '/jurnal/dlq/:id',                     permanent: true },
{ source: '/jrnl/dlq',                                 destination: '/jurnal/dlq',                         permanent: true },
{ source: '/jrnl/resolve',                             destination: '/jurnal/resolve',                     permanent: true },
{ source: '/jrnl/post',                                destination: '/jurnal/header',                      permanent: true },
{ source: '/jrnl/rekonsiliasi/riwayat',                destination: '/reconciliation/daily',               permanent: true },
{ source: '/jrnl/rekonsiliasi',                        destination: '/reconciliation/daily',               permanent: true },
```

**Catatan urutan**: `/mapping-jurnal/import` dan `/jrnl/mapping/new` harus sebelum wildcard `:event_code` / `:id`. `/jrnl/gl-delivery-dlq/:id` sebelum `/jrnl/dlq/:id` karena path berbeda.

**Security note**: 308 (permanent: true) dieksekusi di Next.js config layer, bukan middleware. Response body kosong sebelum redirect. `Cache-Control` tidak di-override Traefik untuk redirect response.

---

## 3. Periode Buku Layout + Screens (Story P5-M17-01)

### 3.1 Wireframe: `/periode-buku/layout.tsx`

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ [skip-to-main-content — visually hidden, first focusable]                        │
├──────────────────────────────────────────────────────────────────────────────────┤
│ BREADCRUMB: Beranda / Periode Buku / {PageTitle}   (aria-label="Breadcrumb")    │
│ [Calendar icon] Periode Buku                                                     │
├────────────────────┬─────────────────────────────────────────────────────────────┤
│ TIMELINE SIDEBAR   │ MAIN CONTENT (role="main")                                  │
│ (col-3)            │ (col-9)                                                     │
│                    │                                                             │
│ ┌──────────────┐   │ {children}                                                  │
│ │ PRD-2026-07  │   │                                                             │
│ │ Juli 2026    │   │                                                             │
│ │ [●OPEN]      │   │                                                             │
│ └──────────────┘   │                                                             │
│       │            │                                                             │
│ ┌──────────────┐   │                                                             │
│ │ PRD-2026-06  │◄──┤ ← highlighted (active route)                               │
│ │ Juni 2026    │   │                                                             │
│ │ [●SOFT_CLOSED│   │                                                             │
│ └──────────────┘   │                                                             │
│       │            │                                                             │
│ ┌──────────────┐   │                                                             │
│ │ PRD-2026-05  │   │                                                             │
│ │ Mei 2026     │   │                                                             │
│ │ [●HARD_CLOSED│   │                                                             │
│ └──────────────┘   │                                                             │
│       │            │                                                             │
│  [Lihat lebih →]   │                                                             │
│  (link ke /periode-│                                                             │
│   buku?all=true)   │                                                             │
│                    │                                                             │
│ [+ Periode Baru]   │                                                             │
│ (absent jika tidak │                                                             │
│  punya perm)       │                                                             │
├────────────────────┴─────────────────────────────────────────────────────────────┤
```

**Timeline sidebar behaviour**:
- Fetch `GET /api/v1/periode?sort=tanggal_mulai:desc&limit=12` untuk 12 periode terakhir.
- Setiap item: link ke `/periode-buku/[id]`. Active item: `border-l-2 border-primary bg-primary/5`.
- `PeriodeStatusBadge` per item: OPEN (hijau), SOFT_CLOSED (kuning), HARD_CLOSED (merah), REOPENED (biru).
- "Lihat lebih" link muncul bila total > 12 — navigasi ke `/periode-buku` list dengan tanpa filter.
- "+ Periode Baru" button: absent bila tidak punya `periode.create`.
- Timeline sidebar: `aria-label="Navigasi Periode Buku"`, setiap item `aria-current="page"` bila aktif.

### 3.2 Wireframe: `/periode-buku` — List (DataTable)

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Periode Buku                                               │
│ [Calendar icon] Daftar Periode Buku                    [+ Periode Baru] (if perm)│
├──────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                       │
│ [Cari kode/nama periode...]  [Status ▾]  [Tahun ▾]                              │
│ Filter chips: [Status: OPEN ×] [Tahun: 2026 ×]        [Bersihkan semua filter]  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                               [↺ Refresh]  [Ekspor ▾ CSV  XLSX]      │
├──────────────────┬──────────────┬────────────────┬────────────────┬─────────────┤
│ Kode [↕]         │ Nama [↕]     │ Tanggal Mulai  │ Tanggal Selesai│ Status      │
│                  │              │ [↕]            │ [↕]            │             │
├──────────────────┼──────────────┼────────────────┼────────────────┼─────────────┤
│ PRD-2026-07      │ Juli 2026    │ 01 Jul 2026    │ 31 Jul 2026    │ ●OPEN       │
│ PRD-2026-06      │ Juni 2026    │ 01 Jun 2026    │ 30 Jun 2026    │ ●SOFT_CLOSED│
│ PRD-2026-05      │ Mei 2026     │ 01 Mei 2026    │ 31 Mei 2026    │ ●HARD_CLOSED│
├──────────────────┴──────────────┴────────────────┴────────────────┴─────────────┤
│ [← Sebelumnya]  Halaman 1 dari ~24   [Selanjutnya →]   Tampilkan: [50 ▾]       │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**DataTable spec**:

| Kolom | Sortable | Filter | Notes |
|---|---|---|---|
| Kode Periode | ya | `?q=` text search | Font mono; link ke `/periode-buku/[id]` |
| Nama Periode | ya | `?q=` text search | |
| Tanggal Mulai | ya | — | Format: DD MMM YYYY |
| Tanggal Selesai | ya | — | Format: DD MMM YYYY |
| Tanggal Hard-Close | ya | — | Blank bila belum hard-closed |
| Status | ya | `filter[status_close]` multi-select: OPEN, SOFT_CLOSED, HARD_CLOSED, REOPENED | `WorkflowStatusBadge` + icon |
| Dibuat Oleh | ya | — | Display name |
| Aksi | tidak | — | Lihat Detail (selalu); Edit (bila OPEN + `periode.update`); absent untuk HARD_CLOSED |

**Default sort**: `tanggal_mulai:desc`.

**Status badge mapping**:
- OPEN → `variant="success"` hijau + CheckCircle icon
- SOFT_CLOSED → `variant="warning"` kuning + Clock icon
- HARD_CLOSED → `variant="destructive"` merah + Lock icon
- REOPENED → `variant="info"` biru + RotateCcw icon

Color bukan sole signal: setiap badge memiliki teks label + icon.

**Export**: `< 10k` rows → streaming download langsung. `≥ 10k` → async job + `JobProgressPanel` di export area. Audit: `PERIODE.EXPORT`.

**Empty state**: "Tidak ada periode yang cocok dengan filter." + "Bersihkan filter" CTA bila filter aktif. Bila tidak ada filter: "Belum ada periode buku. Klik '+ Periode Baru' untuk memulai."

### 3.3 Wireframe: `/periode-buku/[id]` — Detail + Closing Workflow

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Periode Buku / PRD-2026-06                                 │
├───────────────────────────────────────────┬──────────────────────────────────────┤
│ DETAIL PANEL (col-7)                      │ CLOSING WORKFLOW PANEL (col-5)        │
│                                           │                                      │
│ [PeriodeStatusBadge: SOFT_CLOSED]         │ CLOSING WORKFLOW                     │
│ PRD-2026-06 — Juni 2026                  │                                      │
│                                           │ ─── Riwayat Aksi ─────────────      │
│ Tanggal Mulai  : 01 Jun 2026             │                                      │
│ Tanggal Selesai: 30 Jun 2026             │ [●] Soft-close                       │
│ Status         : SOFT_CLOSED             │     Siti Rahayu (ROLE-AKUN-CTL)      │
│ Tgl Soft-close : 25 Jun 2026 14:30       │     25 Jun 2026 14:30                │
│ Hard-close oleh: —                       │     "Checklist selesai, siap close"  │
│ Dibuat         : Budi (ROLE-AKUN)        │                                      │
│ Dibuat pada    : 28 Mei 2026             │ [○] Hard-close    ← AWAITING CFO     │
│                                           │     (belum ada)                      │
│ ─── Checklist Jurnal ──────────────────  │                                      │
│ [ClosingChecklistPanel — read-only view] │ ─── AKSI ─────────────────────────  │
│ 12 / 12 jurnal verified                   │                                      │
│                                           │ [Hard-close Periode]                 │
│ ─── Riwayat Perubahan ─────────────────  │  VISIBLE: USR-CFO-001 (periode.      │
│ [AuditHistoryTable — 5 events terakhir] │  hardclose)                          │
│ [Lihat semua riwayat →]                  │  ABSENT: semua role lain             │
│ (/periode-buku/PRD-2026-06/history)      │                                      │
│                                           │ [Reopen Periode]                     │
│                                           │  ABSENT (status=SOFT_CLOSED;        │
│                                           │  reopen hanya untuk SOFT_CLOSED;    │
│                                           │  ini benar — tampil hanya setelah   │
│                                           │  hard-close bukan mungkin. Reopen   │
│                                           │  dari SOFT → OPEN tersedia)         │
│                                           │                                      │
│                                           │  [Reopen Periode]                   │
│                                           │   VISIBLE untuk AKUN-CTL jika        │
│                                           │   status = SOFT_CLOSED               │
└───────────────────────────────────────────┴──────────────────────────────────────┘
```

**Closing Workflow Panel — state machine**:

```
OPEN ──[soft-close: AKUN-CTL]──► SOFT_CLOSED ──[hard-close: CFO + MFA]──► HARD_CLOSED
                                      │
                                      └──[reopen: AKUN-CTL]──► OPEN
```

**Action buttons per state per persona**:

| Status Periode | Tombol | Role yang melihat | Role yang tidak melihat |
|---|---|---|---|
| OPEN | [Soft-close Periode] | ROLE-AKUN-CTL | semua lain |
| OPEN | [Edit Periode] | ROLE-AKUN (bila punya `periode.update`) | semua lain |
| SOFT_CLOSED | [Hard-close Periode] | ROLE-CFO | semua lain |
| SOFT_CLOSED | [Reopen Periode] | ROLE-AKUN-CTL | semua lain |
| HARD_CLOSED | (tidak ada tombol aksi) | — | — |

Semua tombol yang tidak berwenang: **absent dari DOM** (server component check dari JWT, bukan CSS hide).

### 3.4 Periode Buku — Hard-Close + MFA Step-Up Flow

```
USR-CFO-001 klik [Hard-close Periode]
        │
        ▼
DestructiveActionDialog:
┌─────────────────────────────────────────────────────┐
│ Hard-close periode Juni 2026?                       │
│                                                     │
│ Setelah hard-close, periode tidak bisa di-reopen.  │
│ Semua jurnal akan bersifat final. Lanjut?           │
│                                                     │
│              [Batal]   [Lanjutkan]                  │
└─────────────────────────────────────────────────────┘
        │ klik "Lanjutkan"
        ▼
MFAStepUpModal (dari M4 — tidak dimodifikasi):
┌─────────────────────────────────────────────────────┐
│ Verifikasi MFA Step-Up                              │
│ Konfirmasi hard-close dengan autentikasi tambahan   │
│                                                     │
│ Kode TOTP (6 digit)                                │
│ [_][_][_][_][_][_]                                 │
│                                                     │
│ [error inline bila salah — role="alert", bukan toast│
│  "Kode salah. Sisa percobaan: 2."]                 │
│                                                     │
│         [Batal]   [Verifikasi]                      │
└─────────────────────────────────────────────────────┘
        │ MFA valid → stepUpToken diterima
        ▼
POST /api/v1/periode/PRD-2026-06/hard-close
Headers:
  Authorization: Bearer {jwt}
  X-Step-Up-Token: {stepUpToken}   ← dari MFAStepUpModal.onVerified callback
  Idempotency-Key: {uuid-v4}       ← auto-generated saat tombol diklik
        │
        ▼ server 200
Toast success (4 detik, hijau):
  "Periode Juni 2026 berhasil di-hard-close. Semua jurnal bersifat final."
WorkflowStatusBadge → HARD_CLOSED (merah + Lock icon) — re-fetch GET /periode/[id], no full reload
Semua action buttons absent dari DOM (HARD_CLOSED state)
```

**MFA error handling (di dalam modal, bukan toast)**:
- Kode salah: inline `role="alert"` di dalam modal. POST hard-close tidak di-trigger.
- Setelah 3 kali salah: "Anda telah melebihi batas percobaan. Silakan minta kode baru." Modal bisa ditutup.
- MFA expired (> 5 menit dari verifikasi): getStepUpToken() returns null → modal re-triggered.

### 3.5 Periode Buku — Soft-Close + Reopen Flow

**Soft-close (ROLE-AKUN-CTL)**:
1. Klik [Soft-close Periode] → ConfirmationDialog (bukan DestructiveActionDialog karena reversible).
2. Dialog: "Soft-close periode Juli 2026? Periode masih bisa di-reopen untuk koreksi. Lanjut?"
3. Konfirmasi → tombol disable + spinner. Idempotency-Key UUID v4 di-inject.
4. POST `/api/v1/periode/PRD-2026-07/soft-close` dengan JWT `mfa_verified=true` (cukup, bukan step-up).
5. Success: toast hijau 4 detik: "Periode Juli 2026 berhasil di-soft-close. Bisa dibuka kembali untuk koreksi."
6. Badge → SOFT_CLOSED. Tombol soft-close absent; tombol reopen muncul.

**Reopen (ROLE-AKUN-CTL)**:
1. Klik [Reopen Periode] → ConfirmationDialog: "Reopen periode Juli 2026? Status akan kembali ke OPEN."
2. POST `/api/v1/periode/PRD-2026-07/reopen`.
3. Toast: "Periode Juli 2026 berhasil di-reopen. Status kembali ke OPEN."

**Error handling**:
- 422 WORKFLOW_INVALID_TRANSITION: toast merah persistent: "Periode ini sudah hard-closed dan tidak bisa di-soft-close kembali. Kode: WORKFLOW_INVALID_TRANSITION · trace: {traceId:8}"
- 403 FORBIDDEN: toast merah persistent: "Anda tidak memiliki permission untuk aksi ini."

---

## 4. Kurs Screens (Story P5-M17-02)

### 4.1 Audit Findings — Gap vs UX §1/§2/§3

Berdasarkan file tree yang ada (`frontend/src/app/master/kurs/`), semua route sudah ada. Audit gap yang perlu diperbaiki:

| Screen | UX Rule | Gap | M17 Action |
|---|---|---|---|
| `/master/kurs` (list) | §1 Filter | Perlu verifikasi: `filter[sumber]` (JISDOR/MANUAL) dan `filter[tanggal_kurs]` date range | Add bila missing |
| `/master/kurs` (list) | §1 Export | Perlu verifikasi async threshold > 10k | Add `JobProgressPanel` untuk export besar |
| `/master/kurs/new` | §2 Toast | Perlu verifikasi toast spesifik + field error highlight | Fix bila generic |
| `/master/kurs/upload` | §3 JobProgressPanel | `KursUploadDropzone` sudah ada; perlu verifikasi `JisdorJobProgressPanel` integration | Add bila missing |
| `/master/kurs/jisdor-sync` | §3 JobProgressPanel | `JisdorSyncTriggerButton` + `JisdorJobProgressPanel` sudah ada di components | Wire ke halaman bila belum |

### 4.2 Wireframe: `/master/kurs` — List

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Master / Kurs                                              │
│ [TAB NAV: Instrumen | ... | Kurs* | Mapping Jurnal | ...]                        │
│ [DollarSign icon] Kurs Mata Uang             [+ Kurs Baru] (if fx_rate.create)  │
│                                              [Upload Kurs] (if fx_rate.create)  │
│                                              [Sinkronisasi JISDOR]              │
├──────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                       │
│ [Cari kode mata uang...]  [Sumber ▾: JISDOR | MANUAL | SEMUA]  [Tgl Kurs ▾]   │
│ Filter chips: [Sumber: JISDOR ×] [Tgl: Jun 2026 ×]   [Bersihkan semua filter]  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                              [↺ Refresh]  [Ekspor ▾ CSV  XLSX]       │
├───────────────┬──────────────┬───────────────┬──────────────┬────────────────────┤
│ Kode [↕]      │ Nama [↕]     │ Tgl Kurs [↕] │ Kurs JISDOR │ Sumber             │
│               │              │               │ (IDR) [↕]   │                    │
├───────────────┼──────────────┼───────────────┼──────────────┼────────────────────┤
│ USD           │ US Dollar    │ 25 Jun 2026   │ 16.250,0000 │ [JISDOR]           │
│ EUR           │ Euro         │ 25 Jun 2026   │ 17.432,0000 │ [JISDOR]           │
│ SGD           │ Sing. Dollar │ 25 Jun 2026   │ 11.950,0000 │ [MANUAL]           │
├───────────────┴──────────────┴───────────────┴──────────────┴────────────────────┤
│ [← Sebelumnya]  Halaman 1 dari ~30   [Selanjutnya →]   Tampilkan: [50 ▾]       │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**DataTable spec**:

| Kolom | Sortable | Filter | Notes |
|---|---|---|---|
| Kode Mata Uang | ya | `filter[kode_mata_uang][]` multi-value | Font mono |
| Nama Mata Uang | ya | `?q=` | |
| Tanggal Kurs | ya | `filter[tanggal_kurs]` date range | Default sort desc |
| Kurs JISDOR (IDR) | ya | — | Right-align; NUMERIC(20,8) format |
| Kurs Manual (IDR) | ya | — | Right-align; blank bila tidak ada |
| Sumber | ya | `filter[sumber]` select: JISDOR / MANUAL | Chip biru "JISDOR" vs chip abu "MANUAL" |
| Dibuat pada | ya | — | |
| Aksi | tidak | — | Lihat Detail; Edit (bila punya `fx_rate.update`) |

**Default sort**: `tanggal_kurs:desc`.

**ROLE-AUDIT**: tombol "+ Kurs Baru", "Upload Kurs", "Sinkronisasi JISDOR" absent dari DOM.

### 4.3 Wireframe: `/master/kurs/jisdor-sync` — Trigger + JobProgressPanel

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Master / Kurs / Sinkronisasi JISDOR                       │
│ [TAB NAV: ... | Kurs* | ...]                                                     │
│ [RefreshCw icon] Sinkronisasi Kurs BI JISDOR                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│ STATUS SINKRONISASI TERAKHIR                                                     │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │  GET /api/v1/master/kurs/jisdor-sync/status                                 │ │
│ │                                                                              │ │
│ │  Tanggal Sync Terakhir : 25 Jun 2026 10:30:00                               │ │
│ │  Jumlah Mata Uang      : 15 mata uang                                       │ │
│ │  Status                : [SUCCESS ✓] / [FAILED ✗] / [RUNNING ⟳]            │ │
│ │  Dijalankan oleh       : Budi Santoso (ROLE-AKUN)                           │ │
│ │                                                                              │ │
│ │  [↺ Refresh Status]  Polling otomatis setiap 60 detik                       │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│ TRIGGER MANUAL                                                                   │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │  [Sinkronisasi Sekarang]  ← disabled saat job running                       │ │
│ │  (permission: fx_rate.create; absent dari DOM bila tidak punya perm)        │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│ [JobProgressPanel — muncul di sini setelah POST → 202]                          │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │  Sinkronisasi JISDOR — JOB-JISDOR-SYNC-001                                 │ │
│ │                                                                              │ │
│ │  ████████████░░░░░░░░░░░░░░░░  40%                                          │ │
│ │                                                                              │ │
│ │  Mengambil data dari BI JISDOR API...                                       │ │
│ │                                                                              │ │
│ │  Mulai: 10:30:00  •  ETA: 10:30:45                                          │ │
│ │                                                                              │ │
│ │  (tidak ada tombol Batalkan — canCancel=false)                              │ │
│ │  [Lanjutkan di Background]                                                  │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Confirmation dialog sebelum trigger**:
```
"Sinkronisasi kurs BI JISDOR sekarang?
 Kurs hari ini akan di-overwrite dengan data terbaru dari BI JISDOR.
 [Batal]  [Sinkronisasi]"
```

**Post-completion toast**: "Sinkronisasi JISDOR selesai. 15 mata uang diperbarui." + link "Lihat daftar kurs →"

**Failure toast (persistent)**: "Sinkronisasi JISDOR gagal: {error.message}. Kurs hari ini belum tersedia — gunakan entry manual." + link "Entry Manual →" → `/master/kurs/new`

### 4.4 Wireframe: `/master/kurs/upload` — Dropzone + JobProgressPanel

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Master / Kurs / Upload Kurs                               │
│ [Upload icon] Upload Kurs Batch                                                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ DROPZONE (KursUploadDropzone — existing component)                               │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │                                                                              │ │
│ │    [Upload icon]                                                             │ │
│ │    Taruh file di sini atau klik untuk browse                                │ │
│ │    CSV, XLSX (format BI JISDOR atau format internal)  •  Maks. 10 MB        │ │
│ │                                                                              │ │
│ │    [Unduh template CSV]  ← download template kosong                         │ │
│ │                                                                              │ │
│ │    [Jika file dipilih:]                                                      │ │
│ │    kurs-2026-06-25.csv (450 KB)   [×]                                       │ │
│ │    [Upload  [spinner]]   ← disable saat proses berlangsung                  │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│ [JobProgressPanel — muncul setelah 202 Accepted]                                │
│ ┌──────────────────────────────────────────────────────────────────────────────┐ │
│ │  Upload Kurs Batch — JOB-KURS-UPLOAD-001                                   │ │
│ │  ██████████████████░░░░░░░░░░  60%                                          │ │
│ │  Menyimpan entri kurs 90 dari 150...                                        │ │
│ │  ETA: 10:31:20  •  [Batalkan]  [Lanjutkan di Background]                   │ │
│ └──────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Client-side validasi sebelum POST**:
- Format: hanya CSV atau XLSX. Error instant bila lain: "Format file tidak valid. Gunakan template yang tersedia." (toast merah — tidak POST ke server).
- Ukuran: max 10 MB. Error: "Ukuran file melebihi batas 10 MB."

**Idempotency-Key**: generated saat tombol Upload diklik (bukan pada mount), supaya re-upload setelah error menghasilkan key baru.

---

## 5. Mapping Jurnal 6-Eyes Screens (Story P5-M17-03)

### 5.1 State Machine

```
DRAFT ──[submit: AKUN]──► SUBMITTED ──[review: AKUN-CTL]──► REVIEWED
                               │                                  │
                               ▼                                  ▼
                           REJECTED ◄──[reject: any]──► APPROVED_1
                                                              │
                                                    [approve-2: KOMITE]
                                                              │
                                                              ▼
                                                         APPROVED_2
                                                              │
                                                    [activate: AKUN-CTL]
                                                              │
                                                              ▼
                                                           ACTIVE
```

SoD constraint: `maker_id ≠ reviewer_id ≠ approver1_id ≠ approver2_id` (4 user berbeda).

### 5.2 Wireframe: `/master/mapping-jurnal` — List

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Master / Mapping Jurnal                                    │
│ [TAB NAV: ... | Kurs | Mapping Jurnal* | ...]                                   │
│ [GitMerge icon] Mapping Jurnal       [+ Mapping Baru] (if mapping_jurnal.create)│
├──────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                       │
│ [Cari kode/nama mapping...]  [Status ▾]  [Event Code ▾]                        │
│ Filter chips: [Status: DRAFT ×]                       [Bersihkan semua filter]  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                              [↺ Refresh]  [Ekspor ▾ CSV  XLSX]       │
├──────────────┬──────────────┬──────────────┬──────────────┬──────────────────────┤
│ Kode [↕]     │ Event Code   │ Nama [↕]     │ Status [↕]   │ Aktif Sejak [↕]    │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────────────┤
│ MJ-001       │ DEPOSITO_INT │ Bunga Dep.   │ [ACTIVE ✓]   │ 01 Jun 2026        │
│ MJ-002       │ MTM_OBL      │ MTM Obligasi │ [DRAFT]      │ —                  │
│ MJ-003       │ ECL_STAGE2   │ ECL Stage 2  │ [REVIEWED]   │ —                  │
├──────────────┴──────────────┴──────────────┴──────────────┴──────────────────────┤
│ [← Sebelumnya]  Halaman 1 dari ~40   [Selanjutnya →]   Tampilkan: [50 ▾]       │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**WorkflowStatusBadge mapping**:
- DRAFT → abu (Pencil icon)
- SUBMITTED → biru (ArrowRight icon)
- REVIEWED → cyan (Eye icon)
- APPROVED_1 → indigo (CheckCircle icon)
- APPROVED_2 → violet (ShieldCheck icon)
- ACTIVE → hijau (Check icon)
- REJECTED → merah (XCircle icon)

**Tombol "+ Mapping Baru"**: absent dari DOM untuk ROLE-APPR-TR, ROLE-RISK, ROLE-AUDIT, ROLE-KOMITE (tidak punya `mapping_jurnal.create`).

### 5.3 Wireframe: `/master/mapping-jurnal/[id]` — Detail + 6-Eyes Workflow Panel

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Master / Mapping Jurnal / MJ-001                          │
├───────────────────────────────────────────┬──────────────────────────────────────┤
│ DETAIL PANEL (col-7)                      │ 6-EYES WORKFLOW PANEL (col-5)         │
│                                           │ (SixEyesWorkflowPanel.tsx — existing)│
│ [WorkflowStatusBadge: REVIEWED]           │                                      │
│ MJ-001 — Bunga Deposito                  │ STATUS WORKFLOW                      │
│                                           │                                      │
│ Event Code  : DEPOSITO_INT               │ ①[✓] Maker          DONE             │
│ Debit COA   : 6001.01 Beban Bunga        │     Budi (ROLE-AKUN)                 │
│ Kredit COA  : 2101.01 Hutang Bunga       │     20 Jun 2026 09:00                │
│ Keterangan  : Pencatatan bunga deposito  │     "Mapping sesuai SOP 2026"        │
│ Berlaku Dari: 01 Jun 2026                │        │                             │
│                                           │ ②[✓] Reviewer       DONE             │
│ ─── Tab: Detail | History ─────────────  │     Siti (ROLE-AKUN-CTL)             │
│                                           │     22 Jun 2026 10:15                │
│ TAB Detail:                              │     "Mapping terverifikasi"           │
│   [MappingDetailRowsBuilder — debit/     │        │                             │
│    kredit lines + conditions]            │ ③[▶] Approver 1     CURRENT          │
│                                           │     (menunggu ROLE-RISK)             │
│ TAB History:                             │                                      │
│   [AuditHistoryTable — semua events]    │ ④[○] Approver 2     PENDING          │
│   Kolom: tgl, actor, role, aksi,        │     (menunggu ROLE-KOMITE)           │
│   komentar                               │                                      │
│                                           │ ─────────────────────────────────   │
│                                           │ AKSI (gated per role + state):      │
│                                           │                                      │
│                                           │ [untuk ROLE-RISK jika status=       │
│                                           │  REVIEWED dan bukan maker/reviewer:]│
│                                           │ ┌──────────────────────────────┐   │
│                                           │ │ Komentar (opsional)          │   │
│                                           │ │ [textarea]                   │   │
│                                           │ │                              │   │
│                                           │ │ [☐] Saya menyatakan data ini │   │
│                                           │ │     telah diperiksa dan      │   │
│                                           │ │     sesuai standar berlaku.  │   │
│                                           │ │                              │   │
│                                           │ │ [Tolak]  [Approve (Risk)]    │   │
│                                           │ └──────────────────────────────┘   │
│                                           │                                      │
│                                           │ [SodBlockBanner — bila user adalah  │
│                                           │  maker/reviewer dan mencoba aksi    │
│                                           │  pada step yang bukan giliran mereka]│
└───────────────────────────────────────────┴──────────────────────────────────────┘
```

### 5.4 Mapping Jurnal — Button Gating Matrix per Step

| Status | Tombol Visible | Untuk Role | Kondisi Tambahan |
|---|---|---|---|
| DRAFT | [Submit ke Review] | ROLE-AKUN | makerId = currentUser |
| SUBMITTED | [Review & Tandatangani] | ROLE-AKUN-CTL | bukan makerId; punya `mapping_jurnal.review` |
| SUBMITTED | [Tolak] | ROLE-AKUN-CTL | sama dengan Review |
| REVIEWED | [Approve (Risk)] | ROLE-RISK | bukan makerId, reviewerId; punya `mapping_jurnal.approve` |
| REVIEWED | [Tolak] | ROLE-RISK | sama dengan Approve |
| APPROVED_1 | [Approve Final (Komite)] | ROLE-KOMITE | bukan makerId, reviewerId, approverId; punya `mapping_jurnal.approve` |
| APPROVED_1 | [Tolak] | ROLE-KOMITE | sama |
| APPROVED_2 | [Aktifkan Mapping] | ROLE-AKUN-CTL | punya `mapping_jurnal.activate` |
| ACTIVE | (tidak ada tombol) | — | — |
| REJECTED | [Edit Draft] | ROLE-AKUN (maker original) | makerId = currentUser |

**Tidak ada step-up MFA** untuk approve-2 mapping jurnal (per DEC-027 — step-up hanya untuk hard-close periode, ECL param approve, calc-run seal, klasifikasi approve). ROLE-KOMITE cukup `mfa_verified=true` di JWT (mandatory per DEC-026).

**SoD enforcement**: `SixEyesWorkflowPanel` sudah handle via `reviewSodBlock`, `approveSodBlock`, `approve2SodBlock` + `SodBlockBanner` (lihat existing component — tidak perlu modifikasi).

### 5.5 Mapping Jurnal — Toast Copy per Aksi

| Aksi | Toast (Bahasa Indonesia) | Durasi |
|---|---|---|
| Submit berhasil | "Mapping jurnal MJ-001 berhasil di-submit. Menunggu review oleh Finance Controller." | 4 detik |
| Review berhasil | "Mapping jurnal MJ-001 berhasil di-review. Menunggu approval dari Risk Officer." | 4 detik |
| Approve-1 berhasil | "Mapping jurnal MJ-001 berhasil di-approve (Risk). Menunggu approval final dari Komite Investasi." | 4 detik |
| Approve-2 berhasil | "Mapping jurnal MJ-001 berhasil di-approve (Komite). Siap untuk diaktifkan." | 4 detik |
| Aktifkan berhasil | "Mapping jurnal MJ-001 berhasil diaktifkan. Jurnal engine akan menggunakan mapping ini mulai sekarang." | 4 detik |
| Tolak berhasil | "Mapping jurnal MJ-001 ditolak. Maker akan dinotifikasi untuk perbaikan." | 4 detik |
| SOD_VIOLATION (API) | "Anda tidak bisa menjadi approver untuk mapping yang Anda buat atau review sendiri." | Persistent |

---

## 6. Jurnal Header + DLQ Screens (Story P5-M17-04)

### 6.1 Wireframe: `/jurnal/layout.tsx` — Tab Nav 3 Slot

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ [skip-to-main-content]                                                           │
├──────────────────────────────────────────────────────────────────────────────────┤
│ BREADCRUMB: Beranda / Jurnal / {SubRoute}                                        │
│ [FileText icon] Jurnal                                                           │
├──────────────────────────────────────────────────────────────────────────────────┤
│ TAB NAV (role="tablist", aria-label="Navigasi Jurnal")                          │
│ ┌──────────────┐ ┌──────────────────┐ ┌─────────────────────────────────────┐   │
│ │   Header     │ │  DLQ [badge: 3]  │ │  Resolve  (IT-ADMIN only)          │   │
│ │  (active)    │ │                  │ │                                     │   │
│ └──────────────┘ └──────────────────┘ └─────────────────────────────────────┘   │
│   tab absent dari DOM bila tidak punya permission                                │
├──────────────────────────────────────────────────────────────────────────────────┤
│ MAIN CONTENT (role="tabpanel")                                                   │
│ {children}                                                                       │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**DLQ badge**: fetched dari summary endpoint sekali saat layout mount. Hanya tampil bila count > 0 dan user punya `jurnal.dlq.read`. Badge: `<span aria-label="{N} entri DLQ menunggu" class="badge-destructive">`.

### 6.2 Wireframe: `/jurnal/header` — List

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Jurnal / Header                                            │
│ [TAB NAV: Header* | DLQ [3] | Resolve]                                          │
│ [FileText icon] Jurnal Header                                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                       │
│ [Cari nomor/keterangan...]  [Status ▾]  [Periode ▾]  [Tgl Jurnal: dari—s.d. ▾]│
│ Filter chips: [Status: DRAFT ×]                       [Bersihkan semua filter]  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                              [↺ Refresh]  [Ekspor ▾ CSV  XLSX]       │
├───────────────┬──────────────┬──────────┬──────────┬──────────┬─────────────────┤
│ Nomor [↕]     │ Tgl Jurnal   │ Keterangan│ Debit IDR│ Kredit   │ Status          │
│               │ [↕]          │           │ [↕]      │ IDR [↕]  │                 │
├───────────────┼──────────────┼──────────┼──────────┼──────────┼─────────────────┤
│ JRN-2026-0042 │ 25 Jun 2026  │ Bunga Dep│ 12.500K  │ 12.500K  │ [DRAFT]         │
│ JRN-2026-0041 │ 24 Jun 2026  │ MTM OBL  │ 8.200K   │ 8.200K   │ [APPROVED]      │
│ JRN-2026-0040 │ 24 Jun 2026  │ ECL Stage│ 45.000K  │ 45.000K  │ [POSTED_TO_GL ●]│
├───────────────┴──────────────┴──────────┴──────────┴──────────┴─────────────────┤
│ [← Sebelumnya]  Halaman 1 dari ~200   [Selanjutnya →]   Tampilkan: [50 ▾]      │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**DataTable spec**:

| Kolom | Sortable | Filter | Notes |
|---|---|---|---|
| Nomor Jurnal | ya | `?q=` | Font mono; link ke detail |
| Tanggal Jurnal | ya | `filter[tanggal_jurnal]` date range | |
| Keterangan | ya | `?q=` | truncate 50 char |
| Total Debit IDR | ya | — | Right-align |
| Total Kredit IDR | ya | — | Right-align |
| Periode | ya | `filter[periode_id]` select | |
| Status | ya | `filter[status_workflow]` multi-select | WorkflowStatusBadge |
| Dibuat Oleh | ya | — | |
| Aksi | tidak | — | Lihat Detail; [Submit] hanya bila DRAFT + currentUser = maker |

**Status badge**: DRAFT (abu), SUBMITTED (biru), APPROVED (hijau), REJECTED (merah), POSTED_TO_GL (ungu + Database icon).

**Baris POSTED_TO_GL**: text-muted + tidak ada tombol aksi workflow (read-only).

**Default sort**: `tanggal_jurnal:desc`.

### 6.3 Wireframe: `/jurnal/header/[id]` — Detail + 4-Eyes Panel

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Jurnal / Header / JRN-2026-0042                           │
│ [TAB NAV: Header* | DLQ | Resolve]                                               │
├───────────────────────────────────────────┬──────────────────────────────────────┤
│ DETAIL PANEL (col-8)                      │ WORKFLOW PANEL (col-4)               │
│                                           │ (MakerReviewerApproverPanel pattern) │
│ [WorkflowStatusBadge: SUBMITTED]          │                                      │
│ JRN-2026-0042 — Bunga Deposito           │ ①[✓] Maker                          │
│                                           │     Budi (ROLE-AKUN)                │
│ Tgl Jurnal : 25 Jun 2026                 │     25 Jun 2026 09:00                │
│ Keterangan : Pencatatan bunga deposito   │     "Submit dari sistem"             │
│ Periode    : PRD-2026-06                 │        │                             │
│ Dibuat oleh: Budi (ROLE-AKUN)            │ ②[▶] Approver      AWAITING          │
│                                           │     (menunggu ROLE-AKUN-CTL)        │
│ LINE ITEMS                               │                                      │
│ ┌──────────────────────────────────────┐ │ ─────────────────────────────────   │
│ │ JurnalLinesTable (existing component)│ │                                      │
│ │ No | COA | Keterangan | Debit | Kredit│ │ AKSI:                               │
│ │ 1  |6001 | Bunga Dep.| 12.5K |      │ │ [Submit ke Approver]                 │
│ │ 2  |2101 | Hutang B. |      | 12.5K │ │  (VISIBLE: ROLE-AKUN + status=DRAFT  │
│ │ ───────────────────────────────────  │ │   + currentUser = makerId)           │
│ │ TOTAL               12.5K   12.5K   │ │  (ABSENT: semua lain)               │
│ └──────────────────────────────────────┘ │                                      │
│ [Balance: SEIMBANG ✓]                    │ [Approve Jurnal]                     │
│                                           │  (VISIBLE: ROLE-AKUN-CTL             │
│ AUDIT HISTORY                            │   + status=SUBMITTED                 │
│ [AuditHistoryTable — 5 events terakhir] │   + currentUser ≠ makerId)           │
│                                           │  (ABSENT: semua lain)               │
│                                           │                                      │
│                                           │ [Tolak]                             │
│                                           │  (VISIBLE: ROLE-AKUN-CTL             │
│                                           │   + status=SUBMITTED)               │
└───────────────────────────────────────────┴──────────────────────────────────────┘
```

**4-eyes stepper**:
- [DRAFT] → [SUBMITTED] → [APPROVED] → [POSTED_TO_GL]
- Current step highlighted (amber). Done steps: green + checkmark. Future steps: gray.

**Approve action**:
- Klik [Approve Jurnal] → `ApprovalWithSignature` pattern: textarea komentar (opsional) + checkbox attest ("Saya menyatakan jurnal ini sesuai standar.") + tombol confirm.
- POST `/api/v1/jurnal/{id}/approve` dengan `{ comment, signature_method: "JWT_STEP_UP" }` + Idempotency-Key.
- Toast: "Jurnal JRN-2026-0042 disetujui. Akan di-post ke GL pada jadwal berikutnya."

**Tolak action**:
- Klik [Tolak] → inline textarea wajib alasan (min 30 karakter).
- POST `/api/v1/jurnal/{id}/reject`.
- Toast: "Jurnal JRN-2026-0042 ditolak. Maker akan dinotifikasi."

**SOD_VIOLATION**: bila maker mencoba approve via API langsung → server 403 → toast persistent: "Anda tidak bisa menyetujui jurnal yang Anda buat sendiri."

### 6.4 Wireframe: `/jurnal/dlq` — DLQ List

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Jurnal / DLQ                                              │
│ [TAB NAV: Header | DLQ [3]* | Resolve]                                          │
│ [AlertCircle icon] Dead Letter Queue — Jurnal Gagal GL                         │
├──────────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                       │
│ [Cari nomor jurnal/error...]  [Status ▾: PENDING | RETRYING | DEAD_LETTER]     │
│ [Kode Error ▾]                                                                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                              [↺ Refresh]  [Ekspor ▾ CSV]             │
├────────────┬──────────────┬──────────────┬────────────┬──────────┬──────────────┤
│ ID DLQ [↕] │ Nomor Jurnal │ Tgl Gagal[↕] │ Kode Error │ Retry[↕] │ Status      │
├────────────┼──────────────┼──────────────┼────────────┼──────────┼──────────────┤
│ DLQ-001    │ JRN-2026-041 │ 24 Jun 2026  │ GL_TIMEOUT │ 2        │ [PENDING]   │
│ DLQ-002    │ JRN-2026-039 │ 23 Jun 2026  │ GL_REJECT  │ 5        │ [DEAD_LETTER│
├────────────┴──────────────┴──────────────┴────────────┴──────────┴──────────────┤
│ [← Sebelumnya]  Halaman 1 dari ~10   [Selanjutnya →]   Tampilkan: [50 ▾]      │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Akses**: hanya ROLE-IT-ADMIN (punya `jurnal.dlq.read`). Untuk ROLE-AKUN-CTL dan lain: read-only tanpa tombol Replay. Tombol "Replay ke GL" absent dari DOM bila tidak punya `jurnal.dlq.replay`.

**Default sort**: `created_at:desc`.

### 6.5 Wireframe: `/jurnal/dlq/[id]` — DLQ Detail + Replay + MFA Step-Up

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Jurnal / DLQ / DLQ-001                                    │
│ [TAB NAV: Header | DLQ [3]* | Resolve]                                          │
├──────────────────────────────────────────────────────────────────────────────────┤
│ DLQ-001 — JRN-2026-0041                    [GlFailureCategoryBadge: GL_TIMEOUT] │
│                                                                                  │
│ ─── Error Detail ──────────────────────────────────────────────────────────────  │
│ Kode Error     : GL_TIMEOUT                                                     │
│ Pesan Terakhir : Connection timed out after 30s                                 │
│ Retry Count    : 2 dari 5 max                                                   │
│ Terakhir Coba  : 24 Jun 2026 03:15:00                                           │
│ Next Retry At  : 24 Jun 2026 09:15:00                                           │
│                                                                                  │
│ ─── Error Payload ─────────────────────────────────────────────────────────────  │
│ [JSONBTreeView — last_error JSON, collapsible]                                  │
│                                                                                  │
│ ─── Retry History ─────────────────────────────────────────────────────────────  │
│ [GlDeliveryHistoryTimeline — existing component]                                │
│                                                                                  │
│ ─── Jurnal Terkait ─────────────────────────────────────────────────────────────│
│ [Lihat Jurnal JRN-2026-0041 →]  (/jurnal/header/JRN-2026-0041)                 │
│                                                                                  │
│ ─── AKSI ───────────────────────────────────────────────────────────────────────│
│ [Replay ke GL]  ← VISIBLE hanya IT-ADMIN (jurnal.dlq.replay); absent lain      │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**DLQ Replay Flow**:

```
USR-IT-001 klik [Replay ke GL]
        │
        ▼
DestructiveActionDialog:
  "Replay jurnal DLQ-001 ke GL Host?
   Ini akan mencoba kirim ulang ke sistem GL.
   [Batal]  [Replay]"
        │ konfirmasi
        ▼
MFAStepUpModal (reuse dari M4):
  title="Verifikasi MFA untuk replay DLQ ke GL"
  description="Replay DLQ adalah aksi sensitif (DEC-027). Verifikasi MFA step-up diperlukan."
        │ MFA valid → stepUpToken
        ▼
POST /api/v1/jurnal/dlq/DLQ-001/replay
Headers: X-Step-Up-Token + Idempotency-Key
        │ 202 Accepted { jobId }
        ▼
JobProgressPanel inline di halaman:
  "Replay DLQ-001 ke GL Host..."
  [progress bar SSE]
        │ SSE completed
        ▼
toast success: "DLQ-001 berhasil di-replay ke GL Host. Status: DELIVERED."
DLQ entry status badge → DELIVERED (hijau)
        │ SSE failed
        ▼
toast error persistent: "Replay DLQ-001 gagal: {error.message}. Entry dikembalikan ke antrian. Trace: {traceId}"
```

---

## 7. Reconciliation Screen (Story P5-M17-05)

### 7.1 Wireframe: `/reconciliation/daily` — Summary Cards + Mismatch DataTable

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Rekonsiliasi / Harian                                     │
│                                                                                  │
│ Rekonsiliasi Harian — {selected_date formatted}                                 │
│                                                                                  │
│ DATE PICKER: [25 Jun 2026 ▾]   [↺ Refresh]                                     │
│ (date picker → update URL ?tanggal=YYYY-MM-DD; tombol Refresh re-fetch)         │
│                                                                                  │
│ DATA TERAKHIR: Data per 25 Jun 2026 23:59 (diperbarui oleh cron setiap hari)   │
│                                                                                  │
│ ─── SUMMARY CARDS (2×2 grid) ──────────────────────────────────────────────────│
│ ┌───────────────────────┐  ┌───────────────────────┐                            │
│ │ BLIPS                 │  │ GL Host               │                            │
│ │ 1.240 jurnal          │  │ 1.235 jurnal          │                            │
│ │ Total Debit:          │  │ Total Debit:          │                            │
│ │ Rp 45.678.901.234     │  │ Rp 45.678.901.234     │                            │
│ └───────────────────────┘  └───────────────────────┘                            │
│ ┌───────────────────────┐  ┌───────────────────────┐                            │
│ │ Mismatch              │  │ DLQ Pending           │                            │
│ │ [5 entri]             │  │ [3 entri]             │                            │
│ │ (merah jika > 0)      │  │ [Lihat DLQ →]         │                            │
│ │ (hijau jika = 0)      │  │ (/jurnal/dlq)         │                            │
│ └───────────────────────┘  └───────────────────────┘                            │
│                                                                                  │
│ [Banner jika data belum tersedia:]                                              │
│ ┌──────────────────────────────────────────────────────────────────────────────┐│
│ │ [Info icon] Data rekonsiliasi untuk tanggal ini belum tersedia.              ││
│ │ Cron berjalan setiap hari pukul 23:59.                                      ││
│ └──────────────────────────────────────────────────────────────────────────────┘│
│                                                                                  │
│ ─── MISMATCH DETAIL (DataTable — hanya muncul bila mismatch_count > 0) ────────│
│ Judul: "5 Mismatch Ditemukan — 25 Jun 2026"                                    │
│                                                                                  │
│ [Lihat Detail Mismatch ▾] ← accordion atau scroll ke DataTable                 │
│                                                                                  │
│ FILTER BAR (mismatch table):                                                    │
│ [Jenis ▾: MISSING_IN_GL | AMOUNT_DIFF | EXTRA_IN_GL]  [Status ▾: OPEN | ...]  │
│                                                                                  │
│ ACTION BAR:  [Ekspor ▾ CSV  XLSX]                                              │
│                                                                                  │
│ ┌────────────┬──────────────┬──────────────┬────────────┬────────────┬─────────┤
│ │ Nomor Jrnl │ Tgl Jurnal   │ Nominal BLIPS│ Nominal GL │ Selisih[↕] │ Jenis   │
│ ├────────────┼──────────────┼──────────────┼────────────┼────────────┼─────────┤
│ │ JRN-0041   │ 24 Jun 2026  │ Rp 8.200K    │ —          │ Rp 8.200K  │ MISSING │
│ │ JRN-0039   │ 23 Jun 2026  │ Rp 3.100K    │ Rp 3.000K  │ Rp 100K    │ AMT_DIFF│
│ └────────────┴──────────────┴──────────────┴────────────┴────────────┴─────────┤
│ [← Sebelumnya]  Halaman 1 dari ~5   [Selanjutnya →]                            │
└──────────────────────────────────────────────────────────────────────────────────┘
```

**Row highlighting** (background color + tooltip — color bukan sole signal):
- MISSING_IN_GL: `bg-yellow-50` + icon "!" + label "MISSING"
- AMOUNT_DIFF: `bg-orange-50` + icon "≠" + label "AMT DIFF"
- EXTRA_IN_GL: `bg-red-50` + icon "+" + label "EXTRA"

**DataTable spec — mismatch**:

| Kolom | Sortable | Filter | Notes |
|---|---|---|---|
| Nomor Jurnal | ya | `?q=` | Link "Lihat Jurnal →" bila tersedia |
| Tanggal Jurnal | ya | — | |
| Nominal BLIPS IDR | ya | — | Right-align |
| Nominal GL IDR | ya | — | Right-align; blank bila MISSING_IN_GL |
| Selisih IDR | ya | — | Right-align; **default sort desc** (mismatch terbesar di atas) |
| Jenis Mismatch | ya | `filter[jenis_mismatch]` | `ReconMismatchTypeBadge` (existing component) |
| Status Resolusi | ya | `filter[status_resolusi]` | OPEN / RESOLVED / ACKNOWLEDGED |
| Link DLQ | tidak | — | "Lihat DLQ →" bila entry ada di DLQ |

**Tidak ada tombol mutasi** di halaman ini — full read-only untuk semua role. Resolusi via DLQ replay di `/jurnal/dlq`.

**URL state**: `?tanggal=YYYY-MM-DD` di URL. Date picker → update searchParam → re-fetch. Browser Back → tanggal sebelumnya. Shareable URL.

**Role gate**: `jurnal.read` wajib. ROLE-AKUN tanpa `jurnal.read` scope recon → HTTP 403 dari server component → redirect ke `/dashboard`. ROLE-AUDIT: read-only + export tersedia.

**Export**: audit event `REKON_HARIAN.EXPORT`. Async bila > 10k rows (jarang terjadi — recon harian biasanya ratusan baris).

---

## 8. JobProgressPanel Spec — Semua Long-Running Operations

### 8.1 Placement per Screen

| Screen | Trigger | Panel Placement | canCancel |
|---|---|---|---|
| `/master/kurs/upload` | Tombol Upload | Di bawah dropzone | ya |
| `/master/kurs/jisdor-sync` | Tombol Sinkronisasi | Di bawah trigger button | tidak |
| `/jurnal/dlq/[id]` | Tombol Replay ke GL | Di bawah tombol Replay | ya |
| Export > 10k rows (semua list) | Tombol Ekspor | Di area action bar | ya |

### 8.2 JobProgressPanel Visual States

Gunakan `JobProgressPanel.tsx` yang sudah ada (tidak dimodifikasi). States:

```
QUEUED:
  [Clock icon]  Menunggu di antrian...  Job ID: {jobId}
  [Lanjutkan di Background]

RUNNING:
  [Spinner]  {JobTitle}
  [████░░░░░░░░]  47%
  {currentStep}
  Mulai: {startedAt}  •  ETA: {eta}
  [Batalkan]  (absent bila canCancel=false)
  [Lanjutkan di Background]

COMPLETED:
  [CheckCircle — hijau]  Selesai
  {successSummary}
  [Lihat Hasil →]

FAILED:
  [XCircle — merah]  Gagal
  {error.message}
  Trace: {traceId}
  [Coba Lagi]  [Tutup]

CANCELLED:
  [Ban — abu]  Dibatalkan
  [Tutup]
```

### 8.3 Step Text per Job Type

| Job Type | Step Text | Completed Toast |
|---|---|---|
| KURS_UPLOAD | "Menyimpan entri kurs {current} dari {total}..." | "Upload kurs selesai. {N} entri kurs berhasil diimpor." |
| JISDOR_SYNC | "Mengambil data dari BI JISDOR API..." / "Menyimpan {N} kurs mata uang..." | "Sinkronisasi JISDOR selesai. {N} mata uang diperbarui." |
| DLQ_REPLAY | "Mengirim jurnal {nomor} ke GL Host..." | "DLQ-{id} berhasil di-replay ke GL Host. Status: DELIVERED." |
| EXPORT | "Menyiapkan export {N} baris..." | "Export {entity} selesai. Download dimulai." |

---

## 9. MFA Step-Up Reuse Spec

### 9.1 Kapan Invoke `MFAStepUpModal` (DEC-027)

| Aksi | Trigger | Header ke API |
|---|---|---|
| Hard-close periode buku | USR-CFO klik [Hard-close Periode] + konfirmasi dialog | `X-Step-Up-Token` ke `POST /api/v1/periode/{id}/hard-close` |
| DLQ replay ke GL | USR-IT-ADMIN klik [Replay ke GL] + konfirmasi dialog | `X-Step-Up-Token` ke `POST /api/v1/jurnal/dlq/{id}/replay` |

**Tidak ada step-up MFA** untuk: mapping jurnal approve-2 (cukup `mfa_verified=true` di JWT per DEC-026 KOMITE), soft-close (cukup AKUN-CTL JWT), reopen, submit/review/approve jurnal biasa.

### 9.2 Token Flow (Reuse Existing Pattern)

```tsx
// Pattern — sama untuk hard-close dan DLQ replay
const [mfaOpen, setMfaOpen] = useState(false);

const handleActionClick = () => {
  // 1. Cek token freshness (TTL 5 menit per isMFAFresh())
  const existing = getStepUpToken();
  if (existing) {
    executeAction(existing);  // token masih valid
  } else {
    setMfaOpen(true);         // prompt MFA
  }
};

// MFAStepUpModal props:
<MFAStepUpModal
  open={mfaOpen}
  onOpenChange={setMfaOpen}
  title="Konfirmasi {actionLabel} dengan autentikasi tambahan"
  description="Step-up MFA diperlukan (DEC-027)"
  onVerified={(token) => {
    setStepUpToken(token);    // simpan dengan TTL 5 menit
    executeAction(token);
  }}
/>

// executeAction:
const executeAction = async (stepUpToken: string) => {
  await apiPost(`/api/v1/{endpoint}`, body, {
    headers: {
      "X-Step-Up-Token": stepUpToken,
      "Idempotency-Key": uuidv4(),
    }
  });
};
```

**Penting**: `MFAStepUpModal` dari M4 tidak dimodifikasi. Props `title` dan `description` yang berbeda per use case sudah disupport.

---

## 10. DataTable Spec — Ringkasan Semua List

| Screen | Endpoint | Default Sort | Kolom Wajib | Filter Kunci | Export |
|---|---|---|---|---|---|
| `/periode-buku` | `GET /api/v1/periode` | `tanggal_mulai:desc` | kode, nama, tgl_mulai, tgl_selesai, status | `filter[status_close]`, `filter[tahun]` | CSV + XLSX; async > 10k |
| `/master/kurs` | `GET /api/v1/master/kurs` | `tanggal_kurs:desc` | kode, nama, tgl, kurs_jisdor, kurs_manual, sumber | `filter[kode_mata_uang][]`, `filter[sumber]`, date range | CSV + XLSX; async > 10k |
| `/master/mapping-jurnal` | `GET /api/v1/master/mapping-jurnal` | `created_at:desc` | kode, event_code, nama, status_workflow, active_since | `filter[status_workflow]`, `filter[event_code][]` | CSV + XLSX; async > 10k |
| `/jurnal/header` | `GET /api/v1/jurnal` | `tanggal_jurnal:desc` | nomor, tgl, keterangan, debit_idr, kredit_idr, periode, status | `filter[status_workflow]`, `filter[periode_id]`, date range | CSV + XLSX; async > 10k |
| `/jurnal/dlq` | `GET /api/v1/jurnal/dlq` | `created_at:desc` | id_dlq, nomor_jurnal, tgl_gagal, kode_error, retry_count, status | `filter[status]`, `filter[kode_error]` | CSV; async > 10k |
| `/reconciliation/daily` mismatch | `GET /api/v1/gl-delivery/recon/mismatches` | `selisih_idr:desc` | nomor_jurnal, tgl, nominal_blips, nominal_gl, selisih, jenis, status_resolusi | `filter[jenis_mismatch]`, `filter[status_resolusi]` | CSV + XLSX; async > 10k |

---

## 11. Form Spec per New/Edit

### 11.1 Periode Buku — Form New/Edit

**Endpoint**: `POST /api/v1/periode` (new) · `PATCH /api/v1/periode/{id}` (edit)

**Fields**:
- `kode_periode` *: text, format PRD-YYYY-MM; auto-generated suggestion atau manual
- `nama_periode` *: text, max 100 char
- `tanggal_mulai` *: date picker
- `tanggal_selesai` *: date picker; harus setelah `tanggal_mulai`
- `keterangan`: textarea opsional

**Idempotency-Key**: UUID v4, generated pada mount via `useRef`.

**Toast sukses (new)**: "Periode {kode_periode} berhasil dibuat." + link "Lihat detail →"

**Toast sukses (edit)**: "Periode {kode_periode} berhasil diperbarui."

**Toast error (409 CONFLICT)**: "Data sudah diubah oleh pengguna lain. Muat ulang halaman untuk melihat versi terbaru."

### 11.2 Kurs — Form New/Edit

**Endpoint**: `POST /api/v1/master/kurs` (new) · `PATCH /api/v1/master/kurs/{id}` (edit)

**Fields**:
- `kode_mata_uang` *: select dari daftar mata uang ISO 4217
- `tanggal_kurs` *: date picker
- `kurs_jisdor` *: numeric; NUMERIC(20,8); label "Kurs BI JISDOR (IDR)"
- `kurs_manual`: numeric opsional; NUMERIC(20,8); label "Kurs Manual (IDR)"
- `catatan`: textarea opsional

**Toast sukses**: "Kurs {kode_mata_uang} tanggal {tanggal_kurs} berhasil disimpan." + link "Lihat detail →"

**Toast error (409 CONFLICT)**: "Kurs {kode_mata_uang} tanggal {tanggal_kurs} sudah ada. Edit entri yang ada atau gunakan tanggal berbeda."

### 11.3 Mapping Jurnal — Form New/Edit

**Endpoint**: `POST /api/v1/master/mapping-jurnal` · `PATCH /api/v1/master/mapping-jurnal/{id}`

**Fields**:
- `event_code` *: select dari daftar event code yang tersedia
- `nama_mapping` *: text
- `debit_coa` *: typeahead dari COA master
- `kredit_coa` *: typeahead dari COA master
- `keterangan`: textarea
- `berlaku_dari` *: date picker

**Toast sukses (new)**: "Mapping jurnal {kode} berhasil dibuat sebagai draft. Submit untuk memulai review."

**Catatan**: form hanya bisa diisi oleh ROLE-AKUN (maker). Setelah submit ke review, form tidak bisa diedit (absent-from-DOM untuk non-maker setelah status ≠ DRAFT).

---

## 12. Role Gating Spec per Route

### 12.1 Route Permission Matrix

| Route | Required Permission | Absent-from-DOM Mutations | Redirect jika gagal |
|---|---|---|---|
| `/periode-buku` (list) | `periode.read` | "+ Periode Baru" jika tidak punya `periode.create` | `notFound()` |
| `/periode-buku/[id]` | `periode.read` | [Hard-close], [Soft-close], [Reopen] per state + role | `notFound()` |
| `/periode-buku/new` | `periode.create` | — | `redirect('/periode-buku')` |
| `/master/kurs` | `fx_rate.read` | "+ Kurs Baru", "Upload", "Sinkronisasi JISDOR" | `notFound()` |
| `/master/kurs/new` | `fx_rate.create` | — | `redirect('/master/kurs')` |
| `/master/kurs/upload` | `fx_rate.create` | — | `redirect('/master/kurs')` |
| `/master/kurs/jisdor-sync` | `fx_rate.create` | "Sinkronisasi Sekarang" jika tidak punya | `redirect('/master/kurs')` |
| `/master/mapping-jurnal` | `mapping_jurnal.read` | "+ Mapping Baru" per role | `notFound()` |
| `/master/mapping-jurnal/new` | `mapping_jurnal.create` | — | `redirect('/master/mapping-jurnal')` |
| `/master/mapping-jurnal/[id]` | `mapping_jurnal.read` | Action buttons per step + role | `notFound()` |
| `/jurnal/header` | `jurnal.read` | Submit button gated per item | `notFound()` |
| `/jurnal/header/[id]` | `jurnal.read` | [Submit], [Approve], [Tolak] per state + role | `notFound()` |
| `/jurnal/dlq` | `jurnal.dlq.read` | [Replay] absent bila tidak punya `jurnal.dlq.replay` | `notFound()` |
| `/jurnal/dlq/[id]` | `jurnal.dlq.read` | [Replay] absent | `notFound()` |
| `/jurnal/resolve` | `jurnal.resolve` | — (IT-ADMIN only; halaman ini sudah restricted) | `notFound()` |
| `/reconciliation/daily` | `jurnal.read` | Tidak ada tombol mutasi | redirect `/dashboard` |

### 12.2 Tab Visibility Matrix

| Tab (Jurnal layout) | AKUN | AKUN-CTL | RISK | KOMITE | AUDIT | IT-ADMIN |
|---|---|---|---|---|---|---|
| Header | visible | visible | visible | visible | visible | visible |
| DLQ | — | — | — | — | — | visible |
| Resolve | — | — | — | — | — | visible |

| Tab (Master layout, bagian kurs+mapping) | AKUN | AKUN-CTL | RISK | KOMITE | AUDIT | IT-ADMIN |
|---|---|---|---|---|---|---|
| Kurs | visible | visible | visible | — | visible | — |
| Mapping Jurnal | visible | visible | visible | visible | visible | — |

---

## 13. Bahasa Indonesia Copy Table

### 13.1 Periode Buku Labels

| Key | ID | EN (export) |
|---|---|---|
| periode.title | Periode Buku | Accounting Periods |
| periode.new | + Periode Baru | + New Period |
| periode.status.open | OPEN | Open |
| periode.status.soft_closed | SOFT CLOSED | Soft Closed |
| periode.status.hard_closed | HARD CLOSED | Hard Closed |
| periode.status.reopened | REOPENED | Reopened |
| periode.action.soft_close | Soft-close Periode | Soft-close Period |
| periode.action.hard_close | Hard-close Periode | Hard-close Period |
| periode.action.reopen | Reopen Periode | Reopen Period |
| periode.confirm.soft_close | Soft-close periode {kode}? Periode masih bisa di-reopen untuk koreksi. Lanjut? | Soft-close period {kode}? The period can still be reopened for corrections. Proceed? |
| periode.confirm.hard_close | Hard-close periode {kode}? Setelah hard-close, periode tidak bisa di-reopen. Semua jurnal akan bersifat final. Lanjut? | Hard-close period {kode}? After hard-close, the period cannot be reopened. All journals will be final. Proceed? |
| periode.confirm.reopen | Reopen periode {kode}? Status akan kembali ke OPEN. | Reopen period {kode}? Status will return to OPEN. |
| periode.toast.soft_closed | Periode {kode} berhasil di-soft-close. Bisa dibuka kembali untuk koreksi. | Period {kode} soft-closed. Can be reopened for corrections. |
| periode.toast.hard_closed | Periode {kode} berhasil di-hard-close. Semua jurnal bersifat final. | Period {kode} hard-closed. All journals are final. |
| periode.toast.reopened | Periode {kode} berhasil di-reopen. Status kembali ke OPEN. | Period {kode} reopened. Status returned to OPEN. |
| periode.error.workflow_invalid | Periode ini sudah {status} dan tidak bisa melakukan aksi ini. | This period is already {status} and cannot perform this action. |
| periode.mfa.title | Konfirmasi hard-close dengan autentikasi tambahan | Confirm hard-close with additional authentication |
| periode.export.success | Export periode buku selesai. {N} baris diunduh. | Accounting period export complete. {N} rows downloaded. |

### 13.2 Kurs Labels

| Key | ID | EN |
|---|---|---|
| kurs.title | Kurs Mata Uang | Exchange Rates |
| kurs.new | + Kurs Baru | + New Rate |
| kurs.upload.btn | Upload Kurs | Upload Rates |
| kurs.jisdor.btn | Sinkronisasi JISDOR | Sync JISDOR |
| kurs.jisdor.confirm | Sinkronisasi kurs BI JISDOR sekarang? Kurs hari ini akan di-overwrite dengan data terbaru. | Sync BI JISDOR rates now? Today's rates will be overwritten with latest data. |
| kurs.jisdor.running | Mengambil data dari BI JISDOR API... | Fetching data from BI JISDOR API... |
| kurs.jisdor.toast.success | Sinkronisasi JISDOR selesai. {N} mata uang diperbarui. | JISDOR sync complete. {N} currencies updated. |
| kurs.jisdor.toast.fail | Sinkronisasi JISDOR gagal: {message}. Kurs hari ini belum tersedia — gunakan entry manual. | JISDOR sync failed: {message}. Today's rates unavailable — use manual entry. |
| kurs.upload.toast.success | Upload kurs selesai. {N} entri kurs berhasil diimpor. | Rate upload complete. {N} rate entries imported. |
| kurs.upload.error.format | Format file tidak valid. Gunakan template yang tersedia. | Invalid file format. Use the provided template. |
| kurs.upload.error.size | Ukuran file melebihi batas 10 MB. | File size exceeds 10 MB limit. |
| kurs.form.toast.created | Kurs {kode} tanggal {tgl} berhasil disimpan. | Rate {kode} for {tgl} saved successfully. |
| kurs.form.error.conflict | Kurs {kode} tanggal {tgl} sudah ada. Edit entri yang ada atau gunakan tanggal berbeda. | Rate {kode} for {tgl} already exists. Edit the existing entry or use a different date. |
| kurs.sumber.jisdor | JISDOR | JISDOR |
| kurs.sumber.manual | MANUAL | MANUAL |

### 13.3 Mapping Jurnal Labels

| Key | ID | EN |
|---|---|---|
| mapping_jurnal.title | Mapping Jurnal | Journal Mappings |
| mapping_jurnal.new | + Mapping Baru | + New Mapping |
| mapping_jurnal.action.submit | Submit ke Review | Submit for Review |
| mapping_jurnal.action.review | Review & Tandatangani | Review & Sign |
| mapping_jurnal.action.approve1 | Approve (Risk) | Approve (Risk) |
| mapping_jurnal.action.approve2 | Approve Final (Komite) | Final Approval (Committee) |
| mapping_jurnal.action.activate | Aktifkan Mapping | Activate Mapping |
| mapping_jurnal.action.reject | Tolak | Reject |
| mapping_jurnal.toast.submitted | Mapping jurnal {kode} berhasil di-submit. Menunggu review oleh Finance Controller. | Journal mapping {kode} submitted. Awaiting Finance Controller review. |
| mapping_jurnal.toast.reviewed | Mapping jurnal {kode} berhasil di-review. Menunggu approval dari Risk Officer. | Journal mapping {kode} reviewed. Awaiting Risk Officer approval. |
| mapping_jurnal.toast.approved1 | Mapping jurnal {kode} berhasil di-approve (Risk). Menunggu approval final dari Komite Investasi. | Journal mapping {kode} approved (Risk). Awaiting Investment Committee final approval. |
| mapping_jurnal.toast.approved2 | Mapping jurnal {kode} berhasil di-approve (Komite). Siap untuk diaktifkan. | Journal mapping {kode} approved (Committee). Ready to activate. |
| mapping_jurnal.toast.activated | Mapping jurnal {kode} berhasil diaktifkan. Jurnal engine akan menggunakan mapping ini mulai sekarang. | Journal mapping {kode} activated. The journal engine will use this mapping from now on. |
| mapping_jurnal.toast.rejected | Mapping jurnal {kode} ditolak. Maker akan dinotifikasi untuk perbaikan. | Journal mapping {kode} rejected. Maker will be notified for corrections. |
| mapping_jurnal.error.sod | Anda tidak bisa menjadi approver untuk mapping yang Anda buat atau review sendiri. | SoD: you cannot approve a mapping you created or reviewed yourself. |
| mapping_jurnal.banner.awaiting_review | Menunggu review oleh Finance Controller. | Awaiting Finance Controller review. |
| mapping_jurnal.banner.awaiting_approve1 | Menunggu approval dari Risk Officer. | Awaiting Risk Officer approval. |
| mapping_jurnal.banner.awaiting_approve2 | Menunggu approval final dari Komite Investasi. | Awaiting Investment Committee final approval. |
| mapping_jurnal.banner.not_yet_ready | Belum dapat di-approve. Menunggu {role} terlebih dahulu. | Cannot approve yet. Awaiting {role} first. |

### 13.4 Jurnal Header + DLQ Labels

| Key | ID | EN |
|---|---|---|
| jurnal.title | Jurnal Header | Journal Headers |
| jurnal.tab.header | Header | Header |
| jurnal.tab.dlq | DLQ | DLQ |
| jurnal.tab.resolve | Resolve | Resolve |
| jurnal.action.submit | Submit ke Approver | Submit to Approver |
| jurnal.action.approve | Approve Jurnal | Approve Journal |
| jurnal.action.reject | Tolak | Reject |
| jurnal.toast.submitted | Jurnal {nomor} berhasil di-submit. Menunggu approval Finance Controller. | Journal {nomor} submitted. Awaiting Finance Controller approval. |
| jurnal.toast.approved | Jurnal {nomor} disetujui. Akan di-post ke GL pada jadwal berikutnya. | Journal {nomor} approved. Will be posted to GL on next schedule. |
| jurnal.toast.rejected | Jurnal {nomor} ditolak. Maker akan dinotifikasi. | Journal {nomor} rejected. Maker will be notified. |
| jurnal.error.sod | Anda tidak bisa menyetujui jurnal yang Anda buat sendiri. | You cannot approve a journal you created yourself. |
| jurnal.error.periode_closed | Periode buku {periode} sudah hard-closed. Tidak bisa memutasi jurnal. | Accounting period {periode} is hard-closed. Cannot mutate journals. |
| jurnal.dlq.title | Dead Letter Queue — Jurnal Gagal GL | Dead Letter Queue — Failed GL Journals |
| jurnal.dlq.action.replay | Replay ke GL | Replay to GL |
| jurnal.dlq.confirm.replay | Replay jurnal {id} ke GL Host? Ini akan mencoba kirim ulang ke sistem GL. | Replay journal {id} to GL Host? This will attempt to resend to the GL system. |
| jurnal.dlq.mfa.title | Verifikasi MFA untuk replay DLQ ke GL | MFA verification for DLQ replay to GL |
| jurnal.dlq.toast.success | {id} berhasil di-replay ke GL Host. Status: DELIVERED. | {id} successfully replayed to GL Host. Status: DELIVERED. |
| jurnal.dlq.toast.fail | Replay {id} gagal: {message}. Entry dikembalikan ke antrian. | Replay {id} failed: {message}. Entry returned to queue. |

### 13.5 Reconciliation Labels

| Key | ID | EN |
|---|---|---|
| recon.title | Rekonsiliasi Harian | Daily Reconciliation |
| recon.subtitle | BLIPS vs GL Host — {tanggal} | BLIPS vs GL Host — {date} |
| recon.card.blips | Jurnal BLIPS | BLIPS Journals |
| recon.card.gl | Jurnal Diterima GL | Journals Received by GL |
| recon.card.mismatch | Mismatch | Mismatch |
| recon.card.dlq | DLQ Pending | DLQ Pending |
| recon.data_per | Data per: {timestamp} (diperbarui oleh cron setiap hari pukul 23:59) | Data as of: {timestamp} (updated by cron daily at 23:59) |
| recon.no_data | Data rekonsiliasi untuk tanggal ini belum tersedia. Cron berjalan setiap hari pukul 23:59. | Reconciliation data for this date is not yet available. Cron runs daily at 23:59. |
| recon.mismatch.detail | {N} Mismatch Ditemukan — {tanggal} | {N} Mismatches Found — {date} |
| recon.mismatch.none | Tidak ada mismatch. BLIPS dan GL Host seimbang untuk tanggal ini. | No mismatches. BLIPS and GL Host are balanced for this date. |
| recon.jenis.missing_in_gl | MISSING (tidak ada di GL) | MISSING (not in GL) |
| recon.jenis.amount_diff | AMT DIFF (nominal berbeda) | AMT DIFF (amount differs) |
| recon.jenis.extra_in_gl | EXTRA (tidak ada di BLIPS) | EXTRA (not in BLIPS) |
| recon.export.success | Export rekonsiliasi {tanggal} selesai. {N} baris mismatch diunduh. | Reconciliation export for {date} complete. {N} mismatch rows downloaded. |

### 13.6 Shared Labels (tambahan dari M16)

| Key | ID | EN |
|---|---|---|
| shared.workflow.stepper.done | Selesai | Done |
| shared.workflow.stepper.current | Sekarang | Now |
| shared.workflow.stepper.pending | Menunggu | Pending |
| shared.mfa.prompt | Verifikasi MFA Step-Up | MFA Step-Up Verification |
| shared.confirm.lanjutkan | Lanjutkan | Proceed |
| shared.confirm.batal | Batal | Cancel |
| shared.action.lihat_detail | Lihat detail → | View detail → |
| shared.action.lihat_hasil | Lihat hasil → | View result → |
| shared.action.entry_manual | Entry Manual → | Manual Entry → |

---

## 14. Accessibility Checklist

### 14.1 Layout Shell — Periode Buku (Vertical Timeline)

- [x] "Lewati ke konten utama" (`#main-content`) sebagai elemen pertama yang bisa difokuskan, visually hidden, muncul saat fokus
- [x] Timeline sidebar: `<nav aria-label="Navigasi Periode Buku">` wraps daftar link
- [x] Setiap timeline item: `<a>` dengan `aria-current="page"` bila aktif
- [x] `PeriodeStatusBadge`: teks label + icon (bukan hanya warna)
- [x] Timeline item tidak aktif: `tabindex="0"` natural; aktif: `aria-current="page"`

### 14.2 Layout Shell — Jurnal Tab Nav

- [x] `<nav aria-label="Navigasi Jurnal">` wraps tab list
- [x] `<ul role="tablist">` dengan `<li role="presentation">` children
- [x] Tab aktif: `role="tab"` + `aria-selected="true"` + `tabindex="0"`
- [x] Tab non-aktif: `role="tab"` + `aria-selected="false"` + `tabindex="-1"`
- [x] Main content: `role="tabpanel"` + `aria-labelledby="{active-tab-id}"`
- [x] DLQ badge: `<span aria-label="{N} entri DLQ menunggu">` + numeric count
- [x] Arrow keys: Right/Left antar tab; wraps di ujung; Enter/Space aktivasi

### 14.3 Status Badges — ARIA Semantics

Semua `WorkflowStatusBadge` dan `PeriodeStatusBadge` harus:
- Teks label visible (bukan hanya warna atau icon)
- Icon dengan `aria-hidden="true"` (decorative)
- Tidak mengandalkan warna sebagai sole signal (WCAG 1.4.1)

Status badge accessibility pattern:
```html
<span class="badge badge-{variant}" role="status">
  <Icon aria-hidden="true" />
  SOFT CLOSED   <!-- visible text label -->
</span>
```

### 14.4 Timeline ARIA Semantics (Jurnal Header + Mapping Jurnal 4/6-eyes stepper)

`SixEyesWorkflowPanel` dan `MakerReviewerApproverPanel` sudah memiliki struktur yang benar. Verifikasi:
- Stepper container: tidak perlu `role="list"` karena ini visual + aksi, bukan navigasi. `aria-label` pada section heading cukup.
- Step yang "current": warna + teks "(SEKARANG)" sudah ada di existing component. Tambah `aria-describedby` ke action button yang menjelaskan langkah mana yang sedang aktif.
- `SodBlockBanner`: `role="alert"` atau `aria-live="assertive"` untuk announce SoD violation saat muncul.

### 14.5 MFAStepUpModal Accessibility

Existing component sudah memenuhi:
- [x] 6 input dengan `aria-label="Kode OTP digit {N}"`
- [x] Error: `role="alert"` + `aria-live="polite"` + `aria-describedby` pada inputs
- [x] `aria-invalid` pada inputs saat error
- [x] Auto-focus digit pertama saat modal buka
- [x] Paste handler (6 digit sekaligus)
- [x] Backspace: kembali ke digit sebelumnya

**Tidak perlu modifikasi** — sudah WCAG 2.1 AA compliant.

### 14.6 Reconciliation Page — Color Coding

Row highlighting di mismatch DataTable menggunakan warna latar belakang. Color bukan sole signal:
- MISSING_IN_GL: `bg-yellow-50` + icon "!" + label teks "MISSING"
- AMOUNT_DIFF: `bg-orange-50` + icon "≠" + label "AMT DIFF"
- EXTRA_IN_GL: `bg-red-50` + icon "+" + label "EXTRA"

`ReconMismatchTypeBadge` (existing component) sudah menyediakan icon + label — gunakan ini di kolom Jenis Mismatch.

### 14.7 Summary Cards — Reconciliation

Summary cards menggunakan warna untuk mismatch count:
- Mismatch = 0: `text-success` hijau + CheckCircle icon + teks "0 entri (seimbang)"
- Mismatch > 0: `text-destructive` merah + AlertTriangle icon + teks "{N} entri"
- DLQ pending: amber + Clock icon + teks "{N} entri"

Semua: color + icon + teks. `role="status"` pada setiap card container untuk screen reader.

### 14.8 DataTable Accessibility (konsisten dengan M16)

- [x] `<table>` dengan `<thead>` / `<tbody>` / `<th scope="col">`
- [x] Sort headers: `aria-sort="ascending|descending|none"` pada `<th>` saat sorted
- [x] `aria-busy="true"` pada table container saat loading
- [x] Empty state: `role="status"` pada pesan container
- [x] Filter inputs: `<label>` associated via `htmlFor` atau `aria-label`
- [x] Error field: `aria-describedby="{field-id}-error"` + `id="{field-id}-error"` pada pesan error

### 14.9 Contrast Requirements (WCAG 2.1 AA)

- Normal text: ≥ 4.5:1 terhadap latar belakang
- Large text (≥ 18pt): ≥ 3:1
- Interactive component borders (input, focus ring): ≥ 3:1
- `text-muted-foreground` shadcn default: ~4.7:1 pada white — PASS untuk normal text
- Mismatch row highlight (`bg-yellow-50`, `bg-orange-50`, `bg-red-50`): pastikan text di atasnya tetap ≥ 4.5:1 — `text-gray-900` pada `bg-yellow-50` ≈ 16:1 — PASS

---

## 15. Component Handoff Checklist untuk `frontend-engineer-nextjs`

### 15.1 File Baru yang Harus Dibuat

| File | Tipe | Reuses |
|---|---|---|
| `frontend/src/app/periode-buku/layout.tsx` | Next.js Server Component | `PeriodeStatusBadge` dari `components/blips/periode-close/` |
| `frontend/src/app/jurnal/layout.tsx` | Next.js Server Component | `JurnalTabNav` (baru) |
| `frontend/src/app/reconciliation/layout.tsx` | Next.js Server Component (minimal) | — |
| `frontend/src/app/jurnal/header/page.tsx` | Next.js page | `DataTable`, `WorkflowStatusBadge` |
| `frontend/src/app/jurnal/header/[id]/page.tsx` | Next.js page | `JurnalLinesTable`, `MakerReviewerApproverPanel`, `AuditHistoryTable`, `ApprovalWithSignature` |
| `frontend/src/app/reconciliation/daily/page.tsx` | Next.js page | `DataTable`, `ReconSummaryCard`, `ReconMismatchTypeBadge` |
| `frontend/src/components/blips/jurnal/JurnalTabNav.tsx` | React Client Component ("use client") | shadcn Tabs primitive |

### 15.2 File yang Di-move (`git mv`)

| From | To |
|---|---|
| `frontend/src/app/master/periode-buku/new/page.tsx` | `frontend/src/app/periode-buku/new/page.tsx` |
| `frontend/src/app/master/periode-buku/[id]/edit/page.tsx` | `frontend/src/app/periode-buku/[id]/edit/page.tsx` |
| `frontend/src/app/master/periode-buku/[id]/history/page.tsx` | `frontend/src/app/periode-buku/[id]/history/page.tsx` |
| `frontend/src/app/jrnl/dlq/page.tsx` | `frontend/src/app/jurnal/dlq/page.tsx` |
| `frontend/src/app/jrnl/dlq/[id]/page.tsx` | `frontend/src/app/jurnal/dlq/[id]/page.tsx` |
| `frontend/src/app/jrnl/resolve/page.tsx` | `frontend/src/app/jurnal/resolve/page.tsx` |

Setelah `git mv`: update semua internal link dari `/jrnl/dlq` ke `/jurnal/dlq`, dst.

### 15.3 File yang Di-upgrade (Existing M4)

| File | Perubahan |
|---|---|
| `frontend/src/app/periode-buku/page.tsx` | Upgrade: DataTable full UX §1 (sort/page/filter/export), status badges, CTA per permission |
| `frontend/src/app/periode-buku/[id]/page.tsx` | Upgrade: closing workflow panel, MFA step-up wiring, action buttons per state per role |

### 15.4 File yang Di-audit + Fix (Existing)

| File | Gap | Aksi |
|---|---|---|
| `frontend/src/app/master/kurs/page.tsx` | Verifikasi UX §1 filter `sumber` + date range; export async threshold | Fix bila missing |
| `frontend/src/app/master/kurs/new/page.tsx` | Verifikasi form notif UX §2 spesifik + Idempotency-Key auto-inject | Fix bila missing |
| `frontend/src/app/master/kurs/upload/page.tsx` | Verifikasi `KursUploadDropzone` + `JisdorJobProgressPanel` sudah wired | Wire bila missing |
| `frontend/src/app/master/kurs/jisdor-sync/page.tsx` | Verifikasi `JisdorSyncTriggerButton` + `JobProgressPanel` wired | Wire bila missing |
| `frontend/src/app/master/mapping-jurnal/page.tsx` | Verifikasi DataTable UX §1 lengkap; WorkflowStatusBadge semua 7 states | Fix bila missing |
| `frontend/src/app/master/mapping-jurnal/[id]/page.tsx` | Verifikasi `SixEyesWorkflowPanel` terpasang + action buttons per step gated | Fix bila missing |
| `frontend/src/app/master/layout.tsx` | Hapus tab "Periode Buku" dari nav | Remove |

### 15.5 Validation Rules per Form

**Periode Buku**:
- `kode_periode`: required; format regex `PRD-\d{4}-\d{2}`
- `nama_periode`: required; max 100 char
- `tanggal_mulai`: required; date; tidak lebih dari 1 tahun ke depan
- `tanggal_selesai`: required; date; harus > `tanggal_mulai`; biasanya `tanggal_mulai + EOM`

**Kurs**:
- `kode_mata_uang`: required; enum dari ISO 4217 list
- `tanggal_kurs`: required; date; tidak lebih dari 1 hari ke depan
- `kurs_jisdor`: required; numeric; > 0; max 8 desimal
- `kurs_manual`: optional; numeric; > 0 bila diisi; max 8 desimal

**Mapping Jurnal**:
- `event_code`: required; select dari enum yang valid
- `nama_mapping`: required; max 200 char
- `debit_coa`: required; UUID dari COA master
- `kredit_coa`: required; UUID dari COA master; tidak boleh sama dengan `debit_coa`
- `berlaku_dari`: required; date

### 15.6 Idempotency-Key Pattern (Sama dengan M16)

```tsx
// Untuk form dengan mount yang stabil (new/edit form)
const idempotencyKey = useRef(uuidv4());

// Untuk form yang bisa di-resubmit setelah error (upload, trigger)
const [idempotencyKey] = useState(() => uuidv4());
// atau: regenerate on button click untuk upload (allow re-upload)

// Inject via shared form hook
headers: {
  "Idempotency-Key": idempotencyKey.current,
}
```

### 15.7 `JurnalTabNav.tsx` Component Contract

```tsx
// JurnalTabNav.tsx — "use client" island
interface JurnalTabNavProps {
  permittedTabs: Array<{
    key: 'header' | 'dlq' | 'resolve';
    label: string;
    href: string;
    badge?: number;  // DLQ count; absent atau 0 = no badge
    badgeVariant?: 'default' | 'destructive';
  }>;
}
```

Parent Server Component (`layout.tsx`) compute `permittedTabs` dari JWT permissions. Client island gunakan `usePathname()` untuk active state.

**Tab keyboard nav**: Arrow Right/Left antar tab; wraps. Enter/Space aktivasi. Tab key → ke main content (bukan ke tab berikutnya).

---

## 16. Anti-Pattern Enforcement

Anti-pattern berikut **dilarang** di semua screen M17 (konsisten dengan M16):

- **Modal stacking**: `MFAStepUpModal` tidak boleh dipanggil dari dalam modal lain. `DestructiveActionDialog` ditutup sebelum `MFAStepUpModal` terbuka.
- **Workflow state di balik tab**: Closing workflow panel, 6-eyes stepper, dan 4-eyes approval panel WAJIB visible di main content — tidak disembunyikan di balik tab sekunder.
- **Auto-save**: Semua form (periode, kurs, mapping) tidak ada auto-save. Operator saves explicitly.
- **Toast sebagai satu-satunya konfirmasi untuk irreversible action**: Hard-close dan DLQ replay WAJIB melalui `DestructiveActionDialog` + `MFAStepUpModal` sebelum toast konfirmasi.
- **CSS hide untuk permission-denied**: Semua tombol yang tidak berwenang harus `absent from DOM`, bukan `display:none` atau `visibility:hidden`.
- **Offset pagination**: Gunakan cursor-based only (DEC-022).
- **`float64` untuk amount**: Tampilkan dari server sebagai string/decimal; format di UI dengan `Intl.NumberFormat('id-ID', { style: 'currency' })`.
