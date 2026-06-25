# P5-M16 — APP-B Frontend Transaction Screens: UI/UX Design Specification

**Story Set**: P5-M16
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Desainer**: uiux-designer
**Tanggal**: 2026-06-25
**Status**: READY FOR HANDOFF
**Linked Stories**: `docs/stories/phase-5/P5-M16-fe-app-b.md`
**Decisions applied**:
- DEC-002 — Next.js 14+ App Router, TypeScript strict, shadcn/ui, React Hook Form + Zod, Zustand, TanStack Query
- DEC-017 — 4-eyes workflow; SoD maker ≠ reviewer ≠ approver enforced
- DEC-018 — audit trail; `{ENTITY}.EXPORT` on every export
- DEC-021 — Idempotency-Key wajib di setiap mutation endpoint
- DEC-022 — cursor-based pagination only
- DEC-025 — JWT permission check; `{entity}.{action}` pattern, not role string

**Gate**: `security-engineer` BLOCKING — absent-from-DOM per role; 308 redirect tidak bocorkan data; server component permission check; Idempotency-Key auto-inject di setiap form submit.

---

## 1. Information Architecture

### 1.1 Shared `/transaksi` Layout with Tab Navigation

The `/transaksi` segment hosts a shared layout rendered as a Next.js Server Component. All six APP-B sub-modules live under this single namespace. The tab strip is the primary navigation surface within this section.

**Tab order** (left to right, matching workflow sequence):

```
Penempatan | MTM | Renewal | Penjualan | Jatuh Tempo | Akrual
```

Rationale for this order: Penempatan originates instruments; MTM prices them daily; Renewal extends them; Penjualan disposes them; Jatuh Tempo monitors settlement; Akrual records interest income. Left-to-right reflects lifecycle progression, reducing cognitive load for operators who work across multiple stages.

**Tab badges**: Each tab shows a count badge when the current persona has pending items requiring action. Badges are read from a lightweight summary endpoint called once on layout mount, not per-tab. Absent-from-DOM if persona lacks read permission for that sub-route.

| Tab | Badge source | Badge meaning |
|---|---|---|
| Penempatan | `pending_review_count` from summary | Awaiting this user's review/approve action |
| MTM | `stale_price_count` | Instruments with stale price alert |
| Renewal | `pending_approval_count` | Awaiting approval |
| Penjualan | `bm_alert_count` | Active BM alerts |
| Jatuh Tempo | `upcoming_7d_count` | Maturing within 7 days |
| Akrual | — | No badge (batch-triggered, not user-action-pending) |

### 1.2 Redirect Strategy

URL namespace migration: existing screens at `/trx/penempatan/*` and `/mtm/*` are relocated to `/transaksi/*`. Permanent 308 redirects in `next.config.js` ensure all bookmarks, integrations, and cached links continue to function.

**308 rules (10 total)**:

```
/trx/penempatan              → /transaksi/penempatan
/trx/penempatan/new          → /transaksi/penempatan/new
/trx/penempatan/:id          → /transaksi/penempatan/:id
/trx/penempatan/:id/edit     → /transaksi/penempatan/:id/edit
/mtm                         → /transaksi/mtm
/mtm/upload                  → /transaksi/mtm/upload
/mtm/upload/batch/:batch_id  → /transaksi/mtm/upload/batch/:batch_id
/mtm/cron                    → /transaksi/mtm/cron
/mtm/:id                     → /transaksi/mtm/:id
/mtm/alerts/stale-price      → /transaksi/mtm/alerts/stale-price
```

Note: `/mtm/:id` and `/mtm/alerts/stale-price` ordering in `next.config.js` must place the specific path `/mtm/alerts/stale-price` before the wildcard `/mtm/:id` to prevent the wildcard consuming the alerts route. Similarly `/mtm/upload/batch/:batch_id` before `/mtm/upload` before `/mtm`.

**Security note for security-engineer**: Redirects must fire at the Next.js config layer (server), not client-side. The 308 response body must be empty — no partial page render before redirect. Verify in production that `Cache-Control: no-store` is not overridden by Traefik caching for redirect responses.

---

## 2. Screen Inventory

```
frontend/src/app/transaksi/
  layout.tsx                              NEW — shared layout: tab nav + breadcrumb + CTA
  penempatan/
    page.tsx                              LIST — git mv from /trx/penempatan/page.tsx
    new/page.tsx                          FORM — git mv from /trx/penempatan/new/page.tsx
    [id]/page.tsx                         DETAIL — git mv from /trx/penempatan/[id]/page.tsx
    [id]/edit/page.tsx                    FORM — git mv from /trx/penempatan/[id]/edit/page.tsx
  mtm/
    page.tsx                              LIST — git mv from /mtm/page.tsx
    upload/page.tsx                       BATCH UPLOAD — git mv from /mtm/upload/page.tsx
    upload/batch/[batch_id]/page.tsx      BATCH DETAIL — git mv from /mtm/upload/batch/[batch_id]/page.tsx
    cron/page.tsx                         CRON STATUS — git mv from /mtm/cron/page.tsx
    [id]/page.tsx                         DETAIL — git mv from /mtm/[id]/page.tsx
    alerts/stale-price/page.tsx           STALE ALERTS — git mv from /mtm/alerts/stale-price/page.tsx
  renewal/
    page.tsx                              LIST — existing; audit UX §1/§2 gap fix
    new/page.tsx                          FORM — existing; audit UX §2 gap fix
    [id]/page.tsx                         DETAIL+WORKFLOW — existing; audit UX §2 gap fix
    [id]/preview/page.tsx                 CASHFLOW PREVIEW — existing; no change needed
  penjualan/
    page.tsx                              LIST — existing; audit UX §1/§2 gap fix
    new/page.tsx                          FORM — existing; audit UX §2 gap fix
    [id]/page.tsx                         DETAIL+WORKFLOW — existing
    bm-alerts/page.tsx                    BM ALERTS LIST — existing; audit UX §1 gap fix
  jatuh-tempo/
    page.tsx                              LIST (read-only) — existing; audit UX §1 gap fix
  akrual/
    page.tsx                              LIST + BATCH TRIGGER — existing; add JobProgressPanel
    dashboard/page.tsx                    KPI DASHBOARD — existing; audit UX §1 on embedded table
    [id]/page.tsx                         DETAIL — existing

frontend/src/components/blips/transaksi/
  TransaksiTabNav.tsx                     NEW — permission-aware tab navigator (client island)
  index.ts                                NEW — barrel export
```

**Total routes**: 22 pages (10 moved, 12 existing audited/fixed).

---

## 3. Layout Shell: `app/transaksi/layout.tsx`

### 3.1 Architecture Decision

`layout.tsx` is a **Server Component**. It reads the JWT `permissions` array server-side (via the Next.js `headers()` helper or the session cookie decoded by middleware). This ensures:

- Tabs absent from server-rendered HTML if permission is absent — no DOM node, no comment, no hidden element.
- No client-side JavaScript needed to decide tab visibility (no flash of unauthorized tabs).
- The `usePathname()` active-state logic is isolated to `TransaksiTabNav.tsx`, a small `"use client"` island imported inside the server layout.

```
app/transaksi/
  layout.tsx   (Server Component)
               reads permissions from JWT
               renders: skip-link + breadcrumb + tab nav (TransaksiTabNav) + CTA
               passes permittedTabs[] + activeCTA to TransaksiTabNav
```

### 3.2 Wireframe: Shared Layout Shell

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│ [skip-to-main-content — visually hidden, first focusable element]                │
├──────────────────────────────────────────────────────────────────────────────────┤
│ BREADCRUMB                                                                        │
│ Beranda / Transaksi / {SubRoute} / {PageTitle}                                   │
│ (each segment a link except the last; last has aria-current="page")              │
├──────────────────────────────────────────────────────────────────────────────────┤
│ PAGE HEADER                                                                       │
│ [Icon] Transaksi                                          [CTA Button — if any]  │
│ Manajemen transaksi investasi — APP-B                                             │
├──────────────────────────────────────────────────────────────────────────────────┤
│ TAB NAV (shadcn/ui Tabs primitive, role="tablist")                                │
│ ┌──────────────┐ ┌──────┐ ┌─────────┐ ┌─────────────┐ ┌────────────┐ ┌───────┐│
│ │Penempatan [3]│ │MTM[!]│ │Renewal  │ │Penjualan[2] │ │Jatuh Tempo │ │Akrual ││
│ │ (active)     │ │      │ │         │ │             │ │            │ │       ││
│ └──────────────┘ └──────┘ └─────────┘ └─────────────┘ └────────────┘ └───────┘│
│   border-b accent on active; tab absent from DOM if no permission                │
├──────────────────────────────────────────────────────────────────────────────────┤
│ MAIN CONTENT (role="tabpanel")                                                    │
│ {children}                                                                        │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 Tab Definitions

| Tab label | `href` | `permission` | Badge |
|---|---|---|---|
| Penempatan | `/transaksi/penempatan` | `penempatan.read` | `pending_review_count` |
| MTM | `/transaksi/mtm` | `transaksi.mtm.read` | `stale_price_count` (amber `!` icon) |
| Renewal | `/transaksi/renewal` | `renewal.read` | `pending_approval_count` |
| Penjualan | `/transaksi/penjualan` | `penjualan.read` | `bm_alert_count` |
| Jatuh Tempo | `/transaksi/jatuh-tempo` | `transaksi.jatuh-tempo.read` | `upcoming_7d_count` |
| Akrual | `/transaksi/akrual` | `transaksi.akrual.read` | — |

**Badge rendering rules**:
- Count `> 0`: filled pill badge, right-aligned inside tab label (amber for warnings, blue for pending review).
- Count `= 0` or unavailable: badge absent from DOM.
- Stale price badge uses `!` icon + count (not just count) because the operator needs immediate visual weight.

### 3.4 Contextual CTA Button

The CTA button in the page header changes based on the active sub-route and the current user's create permission. It is rendered by the server component by inspecting `pathname` prefix and JWT permissions.

| Active sub-route prefix | CTA label | Route | Required permission |
|---|---|---|---|
| `/transaksi/penempatan` | `+ Penempatan Baru` | `/transaksi/penempatan/new` | `penempatan.create` |
| `/transaksi/renewal` | `+ Renewal Baru` | `/transaksi/renewal/new` | `renewal.create` |
| `/transaksi/penjualan` | `+ Penjualan Baru` | `/transaksi/penjualan/new` | `penjualan.create` |
| `/transaksi/mtm` | `+ Upload MTM` | `/transaksi/mtm/upload` | `transaksi.mtm.upload` |
| `/transaksi/jatuh-tempo` | — (no CTA) | — | — |
| `/transaksi/akrual` | CTA rendered on page, not layout | — | `transaksi.akrual.create` |

CTA button: `<Button asChild size="sm"><Link ...>`. `aria-label="Tambah {SubRoute} Baru"` not just the visible label.

**If user lacks create permission**: the CTA is absent from DOM (not disabled). Do not use `disabled` state for permission-denied CTAs — absence is clearer and prevents focus trap.

### 3.5 Breadcrumb Structure

```
<nav aria-label="Breadcrumb">
  <ol>
    <li><a href="/">Beranda</a></li>
    <li aria-hidden="true"> / </li>
    <li><a href="/transaksi/penempatan">Transaksi</a></li>
    <li aria-hidden="true"> / </li>
    <li><a href="/transaksi/penempatan">Penempatan</a></li>   ← only on child pages
    <li aria-hidden="true"> / </li>
    <li aria-current="page">Penempatan Baru</li>             ← last: not a link
  </ol>
</nav>
```

On the list page `/transaksi/penempatan`, the breadcrumb is: `Beranda / Transaksi / Penempatan` with "Penempatan" being `aria-current="page"` (no link to self).

---

## 4. Role-Gating Specification

### 4.1 Tab Visibility Matrix

| Tab | MAKER-TR | APPR-TR | RISK | AKUN | AKUN-CTL | CFO | AUDIT | IT-ADMIN |
|---|---|---|---|---|---|---|---|---|
| Penempatan | visible | visible | visible | — | visible | visible | visible | — |
| MTM | visible | — | visible | visible | — | visible | visible | — |
| Renewal | visible | visible | visible | — | visible | — | visible | — |
| Penjualan | visible | visible | visible | — | visible | — | visible | — |
| Jatuh Tempo | visible | visible | visible | visible | visible | visible | visible | — |
| Akrual | — | — | visible | visible | visible | — | visible | — |

"—" means absent from DOM (server component renders nothing for that tab). IT-ADMIN has no transaksi read permissions by default and sees no tabs; they are redirected to `/jobs` as their default route.

### 4.2 Route-Level Server Check

Each sub-route page performs its own permission check as a Server Component guard before rendering any content. Pattern (same as M15 dashboards):

```
/transaksi/penempatan
  → JWT.permissions includes 'penempatan.read'?
    No  → notFound() (renders 404, not redirect — avoids leaking existence of the route)
    Yes → render page

/transaksi/mtm/upload
  → JWT.permissions includes 'transaksi.mtm.upload'?
    No  → redirect('/transaksi/mtm') if user has 'transaksi.mtm.read', else notFound()
    Yes → render upload page
```

### 4.3 Mutation Button Absent-from-DOM Rules

Per-page mutation buttons are server-rendered where possible. For client-side pages (most list pages use `"use client"` for TanStack Query), the permission check is performed via `usePermissions()` from `@/lib/stores/auth.store`, which hydrates from the JWT stored in a Zustand store initialized by the server.

The critical rule: **any action the user cannot perform must be absent from the DOM** — not disabled, not hidden with CSS, not conditionally styled. This applies to:

- "Upload File" button on `/transaksi/mtm` list: absent if no `transaksi.mtm.upload`
- "Trigger Cron" button on `/transaksi/mtm/cron`: absent if no `transaksi.mtm.upload`
- "Review & Tandatangani" button on workflow detail pages: absent if current user = maker (SoD)
- "Approve Batch Akrual" on `/transaksi/akrual`: absent if no `transaksi.akrual.approve`
- "Jalankan Batch Akrual Harian" on `/transaksi/akrual`: absent if no `transaksi.akrual.create`

ROLE-AUDIT is a special case: all mutation buttons absent, but DataTable export remains available (AUDIT has `export` permission on all entities).

---

## 5. Screen Wireframes

### 5.1 `/transaksi/penempatan` — List

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / Penempatan                                 │
│ [FileText icon] Transaksi                   [+ Penempatan Baru] (if perm)   │
│ [TAB NAV: Penempatan* | MTM | Renewal | Penjualan | Jatuh Tempo | Akrual]   │
├──────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                    │
│ [Cari kode / counterparty...] [Jenis Instrumen ▾] [Status ▾] [Stage ▾]      │
│ Filter chips: [Status: Aktif ×] [Stage: 2 ×]          [Bersihkan semua]     │
├──────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                                  [↺ Refresh] [Ekspor ▾ CSV XLSX]│
├─────────────────┬──────────┬───────────┬────────────┬──────────┬────────────┤
│ Kode [↕]        │ Jenis[↕] │Counterpar.│Nominal IDR │Tgl Penem.│Status      │
├─────────────────┼──────────┼───────────┼────────────┼──────────┼────────────┤
│ PNP-001234      │DEPOSITO  │Bank BCA   │ Rp 2,5 M  │25 Jun 26 │●Aktif      │
│ PNP-001233      │DEPOSITO  │Bank Mandiri│ Rp 1,0 M │24 Jun 26 │●Review     │
│ PNP-001232      │OBLIGASI  │PT FI      │ Rp 5,0 M  │20 Jun 26 │●Draft      │
├─────────────────┴──────────┴───────────┴────────────┴──────────┴────────────┤
│ [← Prev]  Halaman 1 dari ~48   [Next →]   Tampilkan: [50 ▾]   Est. 2.400  │
└──────────────────────────────────────────────────────────────────────────────┘
```

**DataTable columns**:

| Kolom | Sortable | Filter | Notes |
|---|---|---|---|
| Kode Penempatan | yes | text search `?q=` | Font mono; link to detail |
| Jenis Instrumen | yes | `filter[jenis_instrumen]` select | |
| Counterparty/Bank | yes | `filter[counterparty_id]` typeahead | |
| Nominal (IDR) | yes | — | Right-aligned, IDR format |
| Tgl Penempatan | yes | date range | |
| Tgl Jatuh Tempo | yes | date range | |
| Stage | yes | `filter[stage]` select (1/2/3) | Stage badge with text |
| Status Workflow | yes | `filter[workflow_status]` multi-select | `WorkflowStatusBadge` |
| Aksi | no | — | Dropdown: Lihat Detail, Edit (draft+maker), Submit, Batalkan |

**Default sort**: `tanggal_penempatan:desc`.

**Row click**: navigates to `/transaksi/penempatan/{id}`.

**Export**: `< 10k` rows — streaming download. `≥ 10k` — async job + `JobProgressPanel` in export area. Both respect active filter + sort.

**SoD in actions column**: "Review & Tandatangani" rendered absent if `item.makerId === currentUserId`. Implementation in `cell()` function returns `null` — same pattern as existing renewal and penjualan pages.

**Empty state**: "Tidak ada penempatan yang cocok dengan filter." + "Bersihkan filter" CTA (if filters active). If no filters: "Belum ada data penempatan. Klik '+ Penempatan Baru' untuk memulai."

**Loading state**: skeleton rows (8 rows, consistent width distribution matching columns).

**Error state**: alert banner with error code + traceId + "Coba lagi" button. Not inline in table area — banner above table.

### 5.2 `/transaksi/penempatan/new` — Form

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / Penempatan / Penempatan Baru               │
│ [FileText icon] Penempatan Baru                                               │
│ [TAB NAV: Penempatan* | ...]                                                  │
├──────────────────────────────────────────────────────────────────────────────┤
│ FORM (max-w-2xl, centered)                                                    │
│                                                                               │
│ SECTION 1: Instrumen & Counterparty                                          │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ Jenis Instrumen *   [DEPOSITO ▾]                                        │ │
│ │ Counterparty *      [Cari bank... (typeahead)]                          │ │
│ │ Nomor Referensi     [_____________]                                     │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│ SECTION 2: Nominal & Tenor                                                   │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ Nominal (IDR) *     [_____________]          Rp                         │ │
│ │ Mata Uang           [IDR ▾]                                             │ │
│ │ Tgl Penempatan *    [DD/MM/YYYY]                                        │ │
│ │ Tenor (hari) *      [____]   →  Tgl Jatuh Tempo: {kalkulasi otomatis}  │ │
│ │ Suku Bunga (% p.a.)*[____.__]                                          │ │
│ │ Metode Bunga        [AKTUAL/365 ▾]                                     │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│ SECTION 3: Dokumen Pendukung (optional)                                      │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ [Unggah Dokumen] (dokumen konfirmasi bank, bilyet)                      │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│ [Batal]                              [Simpan sebagai Draft  [spinner inline]]│
└──────────────────────────────────────────────────────────────────────────────┘
```

**Idempotency-Key**: Generated as UUID v4 on component mount (`useRef` so it does not regenerate on re-render). Injected into the `POST` header automatically by the shared `useFormSubmit` hook. NOT visible to operator.

**Submit flow**:
1. Operator clicks "Simpan sebagai Draft"
2. Button disables + inline spinner appears (button label changes to "Menyimpan...")
3. Idempotency-Key from `useRef` injected into header
4. On success (201): toast green 4s: "Penempatan {kode} berhasil dibuat sebagai draft. Menunggu submit ke reviewer." + link "Lihat detail →"
5. Form resets only after success toast confirmed (toast fires, then reset)
6. On validation failure (400 VALIDATION_FAILED): toast red persistent + each errored field highlighted with red border + inline message below field (linked via `aria-describedby`)
7. On 409 CONFLICT (edit scenario): toast red persistent: "Data sudah diubah oleh pengguna lain. Muat ulang halaman untuk melihat versi terbaru." Form NOT reset.
8. Button re-enables after any error

**Auto-save is explicitly prohibited** per anti-patterns. Operator saves when ready.

### 5.3 `/transaksi/penempatan/[id]` — Detail + Workflow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / Penempatan / PNP-001234                    │
│ [TAB NAV: Penempatan* | ...]                                                  │
├───────────────────────────────────────────┬──────────────────────────────────┤
│ DETAIL PANEL (col-8)                      │ WORKFLOW PANEL (col-4)           │
│                                           │                                  │
│ [PenempatanStatusBadge: DRAFT]            │ MakerReviewerApproverPanel       │
│ PNP-001234 — Deposito BCA                 │                                  │
│                                           │ [●] Maker                        │
│ Nominal    : Rp 2.500.000.000             │     Budi Santoso                 │
│ Counterpty : PT Bank BCA Tbk              │     25 Jun 2026 09:15            │
│ Tgl Placem.: 25 Jun 2026                  │     "Deposito bilyet #234"       │
│ Tgl Jatuh T: 25 Sep 2026 (92 hari)        │                                  │
│ Suku Bunga : 5.25% p.a.                   │ [○] Reviewer      ← awaiting    │
│ Metode     : AKTUAL/365                   │     (belum ada)                  │
│ EIR Preview: [EIRPreviewSidePanel →]      │                                  │
│                                           │ [○] Approver      ← locked       │
│ DOKUMEN                                   │     (belum ada)                  │
│ [dok-konfirmasi-bca.pdf] [download]       │                                  │
│                                           │ ─────────────────────────────    │
│ AUDIT HISTORY                             │ AKSI                             │
│ [AuditHistoryTable — last 5 events]       │                                  │
│                                           │ [Submit ke Reviewer]             │
│                                           │  (hanya jika maker + DRAFT)      │
│                                           │                                  │
│                                           │ [Review & Tandatangani]          │
│                                           │  (absent jika maker=current_user)│
│                                           │  (absent jika status != PENDING) │
│                                           │                                  │
│                                           │ [Tolak]                          │
│                                           │  (absent jika tidak ada perm)    │
└───────────────────────────────────────────┴──────────────────────────────────┘
```

**SoD enforcement**: The "Review & Tandatangani" button is rendered absent (returns `null` from server component check or client `usePermissions()`) when `penempatan.makerId === JWT.sub`. This mirrors the pattern already correctly implemented in the renewal and penjualan list pages (reading `isMaker` from item and returning `null` in the cell renderer).

**Workflow action dialogs**: Use existing `SubmitDialog`, `ReviewDialog`, `ApproveDialog`, `RejectDialog` from `components/blips/penempatan/dialogs/`. All require a mandatory text comment. Checkbox attest for approve. No new dialog components needed.

**Toast copy for workflow actions**:
- Submit: "PNP-001234 berhasil di-submit ke reviewer. Menunggu tanda tangan reviewer."
- Review sign: "PNP-001234 berhasil di-review. Menunggu persetujuan approver."
- Approve: "PNP-001234 berhasil disetujui dan aktif."
- Reject: "PNP-001234 ditolak. Penempatan dikembalikan ke maker dengan komentar."
- 403 SOD_VIOLATION (direct API): "Anda tidak bisa menjadi reviewer untuk transaksi yang Anda buat sendiri."

---

### 5.4 `/transaksi/mtm/upload` — Dropzone + JobProgressPanel

This is the primary batch-job screen in APP-B. The MTM upload currently lacks a `JobProgressPanel` — the existing implementation at `/mtm/upload/page.tsx` returns a static `batchResult` after synchronous processing. M16 must add the SSE-backed progress panel.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / MTM / Upload Harga MTM                     │
│ [TAB NAV: Penempatan | MTM* | Renewal | Penjualan | Jatuh Tempo | Akrual]    │
├──────────────────────────────────────────────────────────────────────────────┤
│ Upload Harga MTM Manual                                                       │
│ Format diterima: CSV, XLSX (IBPA format atau BEI closing price format)        │
│ Ukuran maks: 50 MB                                                            │
│                                                                               │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ MtmUploadDropzone                                                        │ │
│ │                                                                          │ │
│ │    [Upload icon]                                                         │ │
│ │    Taruh file di sini atau klik untuk browse                             │ │
│ │    CSV, XLSX • Maks. 50 MB                                               │ │
│ │                                                                          │ │
│ │    [Jika file sudah dipilih:]                                            │ │
│ │    mtm-ibpa-2026-06-25.csv (1.2 MB)   [×]                              │ │
│ │                                                                          │ │
│ │    [Upload  [spinner]]   ← disabled saat proses berlangsung              │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│ [Jika job sedang berjalan — JobProgressPanel muncul di sini:]                │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │  Memproses Upload MTM — JOB-MTM-UPLOAD-001                              │ │
│ │                                                                          │ │
│ │  ████████████████░░░░░░░░░░░░░░░  47%                                   │ │
│ │                                                                          │ │
│ │  Parsing baris 2.651 dari 5.678                                         │ │
│ │                                                                          │ │
│ │  Mulai: 10:30:00  •  ETA: 10:31:30 (90 detik lagi)                     │ │
│ │                                                                          │ │
│ │  [Batalkan Upload]   [Lanjutkan di Background]                          │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                               │
│ [Jika completed — pesan inline sebelum toast:]                               │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │  Upload selesai. 5.678 record MTM diproses, 12 error ditemukan.         │ │
│ │  [Lihat Hasil Batch →]                                                   │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Upload flow (M16 revised)**:
1. Operator selects file. Client-side validates format (CSV/XLSX only) and size (≤ 50 MB) immediately — toast error if invalid, no POST fired.
2. Operator clicks "Upload". Button disables + spinner. Idempotency-Key UUID v4 generated on click (not on mount — this allows re-upload of same file after error).
3. `POST /api/v1/transaksi/mtm/upload` with `multipart/form-data` + `Idempotency-Key` header. Server returns `202 { jobId, statusUrl, streamUrl }`.
4. `<JobProgressPanel jobId={jobId}>` mounts below the dropzone. Subscribes to SSE `GET /api/v1/jobs/{jobId}/stream`. Fallback polling 2s on SSE error.
5. On SSE `completed`: panel shows "Upload selesai" summary. Toast green: "Batch upload MTM JOB-MTM-UPLOAD-001 selesai. 5.678 record diproses." + action "Lihat hasil batch →" → `/transaksi/mtm/upload/batch/{jobId}`.
6. On SSE `failed`: toast red persistent: "Upload batch MTM gagal: {error.message}. Trace: {traceId}." + "Coba Upload Ulang" button (resets panel, re-enables dropzone).

**Current gap in existing `/mtm/upload/page.tsx`**: The existing page uses a synchronous `handleUploadSuccess` callback directly from `MtmUploadDropzone` with no SSE. The dropzone component at `MtmUploadDropzone.tsx` needs to be checked — if it already calls the upload endpoint and returns a result synchronously, the M16 fix is to change the `onSuccess` callback to receive `{ jobId }` instead of a full `MtmUploadBatchResponse`, then mount `JobProgressPanel`. The static result preview table becomes the batch detail page.

**"Lanjutkan di Background" button**: closes the `JobProgressPanel` panel (sets `showPanel = false`). The job continues server-side. Global notification badge in the top bar increments. On job completion, notification fires. Clicking the badge → `/jobs` list page.

### 5.5 `/transaksi/mtm` — List

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / MTM                                        │
│ [TrendingUp icon] Transaksi                    [+ Upload MTM] (if perm)      │
│ [TAB NAV: Penempatan | MTM* | ...]                                            │
├──────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                    │
│ [Cari kode instrumen...]  [Jenis ▾] [Sumber Harga ▾] [Status ▾]            │
│ [Harga Stale]  ← quick-filter button; when active becomes chip               │
│ Filter chips: [Stale: ya ×]                         [Bersihkan semua]        │
│ Link: "Lihat Stale Price Alerts →" (→ /transaksi/mtm/alerts/stale-price)     │
├──────────────────────────────────────────────────────────────────────────────┤
│ ACTION BAR                                [↺ Refresh] [Ekspor ▾ CSV XLSX]   │
│ [Upload File] ← absent if no upload perm                                     │
├────────────────┬──────────┬────────────┬──────────┬───────────┬─────────────┤
│ Kode [↕]       │Jenis [↕] │Tgl MTM [↕]│Harga (IDR│MTM IDR[↕]│Status       │
├────────────────┼──────────┼────────────┼──────────┼───────────┼─────────────┤
│ OBL-0042       │OBLIGASI  │25 Jun 26   │102.50    │Rp 1.025K  │●Valid       │
│ SHM-0099       │SAHAM     │25 Jun 26   │3.450     │Rp 3.450K  │[!]Stale    │
├────────────────┴──────────┴────────────┴──────────┴───────────┴─────────────┤
│ [← Prev]  Halaman 1 dari ~30   [Next →]   Tampilkan: [50 ▾]   Est. 1.500  │
└──────────────────────────────────────────────────────────────────────────────┘
```

**DataTable columns**:

| Kolom | Sortable | Filter |
|---|---|---|
| Kode Instrumen | yes | text search |
| Jenis Instrumen | yes | `filter[jenis_instrumen]` select |
| Tanggal MTM | yes | `filter[tanggal_mtm]` date range |
| Harga Pasar | yes | — |
| MTM IDR | yes | — |
| Sumber Harga | yes | `filter[sumber_harga]` select (IBPA/BEI/MANUAL) |
| Status | yes | `filter[status]` + `filter[is_stale]=true` |

**Default sort**: `tanggal_mtm:desc`.

**Stale quick-filter button**: A `<Button variant="outline">` above the filter bar labeled "[!] Harga Stale". Clicking it adds `filter[is_stale]=true` to URL state and shows as a filter chip. Not a separate filter dropdown.

### 5.6 `/transaksi/renewal` — List (Existing: Audit Findings + Required Fixes)

After reading the actual implementation at `frontend/src/app/transaksi/renewal/page.tsx`:

**What is already correct**:
- DataTable with `DataTable` component used correctly
- Cursor-based pagination implemented (cursor history pattern)
- Sort via URL state (`?sort=col:dir`)
- Filter via URL state (`?filter[status]=`, `?filter[skema]=`)
- Filter chips via `activeFilters` array
- Export via `onExport` with `renewalListApi.exportUrl()`
- `lastUpdated` timestamp + "Refresh" button
- Empty state message
- Loading/error states handled by DataTable component
- SoD: `isMaker` check returns `null` from cell renderer (correct absent-from-DOM pattern)
- `aria-label` on action buttons

**Gaps requiring M16 fix**:

1. **Missing filter: `tanggal_renewal` date range** — AC requires `filter[tanggal_renewal]` date range. Currently only `filter[status]` and `filter[skema]`. Add date range picker for tanggal_renewal.

2. **Missing columns**: AC requires `kode_renewal` and `nominal_idr` in minimal columns. Current implementation shows `instrumenLamaKode` (rename needed to `kode_renewal` or add `nominalIdr` column).

3. **Sort default mismatch**: AC requires `sort=tanggal_renewal:desc`. Current default is `created_at:desc`. Fix: change `parseAsString.withDefault("created_at:desc")` to `parseAsString.withDefault("tanggal_renewal:desc")` in `useRenewalFilters`.

4. **Export row count check**: Current `onExport` opens URL directly without checking row count for async-vs-sync threshold. Fix: check `data?.pagination?.totalEstimate` — if `> 10000`, POST job and show `JobProgressPanel` inline in export area instead of `window.open`.

5. **Form notification UX §2**: Not yet auditable from list page — requires reading `new/page.tsx`. Flag for frontend-engineer to verify.

**No structural DataTable redesign needed** — all four pillars (sort/page/filter/export) are present; only specific values require adjustment.

**Wireframe delta** (addition to existing page):

```
FILTER BAR (enhanced):
[Cari kode instrumen...]  [Status ▾]  [Skema ▾]  [Tgl Renewal: dari — s.d. ▾]
Filter chips: [Status: PENDING ×] [Tgl: Jun 2026 ×]        [Bersihkan semua]
```

### 5.7 `/transaksi/penjualan` — List (Existing: Audit Findings + Required Fixes)

After reading `frontend/src/app/transaksi/penjualan/page.tsx`:

**What is already correct**:
- DataTable used correctly with sort/filter/export/pagination
- BM Alerts link in header → `/transaksi/penjualan/bm-alerts`
- SoD absent-from-DOM for approve actions
- Export with filter params respected (partially — `filter[jenis_disposal]` not passed to export URL)

**Gaps requiring M16 fix**:

1. **Export missing filter params**: `handleExport` does not pass `filter[jenis_disposal]`. Fix: add to `penjualanListApi.exportUrl()` call.

2. **Breadcrumb absent**: The penjualan list page has no breadcrumb nav. Add `<nav aria-label="Breadcrumb">` above the h1 (same pattern as renewal page which already has it).

3. **URL state for sort**: The current implementation uses local `useState` for sort and syncs to URL via `useEffect`. This causes a flash on page load. Fix: use `useQueryState` for sort (same pattern as renewal page). This is needed for deep-link/bookmark correctness (AC M16-03-AC1 filter chip + URL state requirement).

4. **Missing `filter[bm_alert]=true` shortcut**: AC M16-03-AC3 requires a quick-filter button on the list for instruments with BM alert. Add `[BM Alert ▾]` button that sets `filter[bm_alert]=true` in URL state.

5. **BM warning inline on form**: When creating a penjualan for an instrument with active BM alert, show `ReturnedBanner` or `<Alert variant="warning">` on the form page. This is on `/transaksi/penjualan/new` not the list — note for frontend-engineer to verify new/page.tsx.

6. **Missing filter: `filter[jenis_disposal]` in URL activeFilters**: The filter chips array currently builds correctly, but uses `onRemove` lambda directly rather than `key` + `handleRemoveFilter` pattern. This means the "Bersihkan semua" button may not clear jenis filter if built differently. Verify and align with renewal pattern.

### 5.8 `/transaksi/jatuh-tempo` — List (Existing: Audit Findings + Required Fixes)

After reading `frontend/src/app/transaksi/jatuh-tempo/page.tsx`:

**What is already correct**:
- DataTable with sort, filter, export, pagination — all four present
- `JatuhTempoStatusBadge` in status column
- `aria-label` on filter selects
- Export with filter params

**Gaps requiring M16 fix**:

1. **Default sort wrong**: AC requires `sort=tanggal_jatuh_tempo:asc` (upcoming first). Current default is `tanggal_jatuh_tempo:desc`. Fix: change `parseAsString.withDefault("tanggal_jatuh_tempo:desc")` to `asc`.

2. **Missing quick-filter shortcuts**: AC M16-04-AC1 requires buttons "Dalam 7 hari", "Dalam 30 hari", "Sudah Jatuh Tempo" as shortcut filters above the DataTable. Currently only status and jenis selects. Add three `<Button variant="outline" size="sm">` buttons that set `filter[tanggal_jatuh_tempo]` and `filter[status]` combinations in URL state.

3. **Missing "Buat Renewal" CTA per row**: AC requires that ROLE-MAKER-TR and ROLE-APPR-TR see a "Buat Renewal" link per UPCOMING row → `/transaksi/renewal/new?instrumen_id={id}`. Currently the actions column is absent entirely. Add a thin "Buat Renewal" `<Link>` in an `actions` column, rendered only when `item.status === 'UPCOMING'` and `usePermissions().can('renewal.create')`.

4. **`AkrualCronTriggerButton` in jatuh-tempo header**: The current page shows a "Trigger Maturity Cron" button. This is a mutation action that should be absent for ROLE-AUDIT. Wrap in `{perms.can('transaksi.akrual.create') && <AkrualCronTriggerButton />}`.

5. **Missing `hari_tersisa` column**: AC requires a "Hari Tersisa" computed column. This should be derived client-side: `Math.ceil((new Date(item.tanggalJatuhTempo).getTime() - Date.now()) / 86_400_000)`. Negative values → styled red ("−N hari"). Add this computed column between `tanggalJatuhTempo` and `status`.

**Wireframe delta** (additions to existing page):

```
QUICK FILTERS (above main filter bar):
[Dalam 7 hari]  [Dalam 30 hari]  [Sudah Jatuh Tempo]
Active button gets filled variant; clears other shortcut when clicked

REVISED COLUMN ORDER:
Kode Instrumen | Jenis | Counterparty | Nominal IDR | Tgl Jatuh Tempo | Hari Tersisa | Status | Aksi
```

### 5.9 `/transaksi/akrual` — List + Batch Trigger + Dashboard KPI

This is the most complex existing screen requiring M16 changes. It currently uses `AkrualCronTriggerButton` which fires an Asynq job but does not show `JobProgressPanel` inline.

**Current state** (from reading `akrual/page.tsx`): The page uses `AkrualCronTriggerButton` component for triggering. The component presumably shows some UI (need to check implementation), but there is no `JobProgressPanel` integration.

**M16 required changes**:

1. Add `<JobProgressPanel>` inline above the DataTable, shown when a batch akrual job is running.
2. The "Jalankan Batch Akrual Harian" trigger now goes through a confirmation dialog before posting.
3. On job `completed`, DataTable auto-refreshes (`queryClient.invalidateQueries`).
4. KPI cards from `/api/v1/transaksi/akrual/dashboard` shown above DataTable.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BREADCRUMB: Beranda / Transaksi / Akrual                                     │
│ [TAB NAV: Penempatan | MTM | Renewal | Penjualan | Jatuh Tempo | Akrual*]    │
├──────────────────────────────────────────────────────────────────────────────┤
│ PAGE HEADER                                                                   │
│ Pendapatan Akrual Harian                   [Approve Batch] (if AKUN-CTL)    │
│                           [Jalankan Batch Akrual Harian] (if akrual.create)  │
├──────────────────────────────────────────────────────────────────────────────┤
│ KPI CARDS ROW (3 × col-4)                                                    │
│ ┌──────────────────────┐ ┌──────────────────────┐ ┌──────────────────────┐  │
│ │ Total Akrual Hari Ini │ │ Instrumen Diproses   │ │ Status Batch Terakhir│  │
│ │ Rp 1.234.567.890     │ │ 1.100 instrumen      │ │ COMPLETED            │  │
│ │                       │ │ (batch terakhir)     │ │ 25 Jun 26 06:00      │  │
│ └──────────────────────┘ └──────────────────────┘ └──────────────────────┘  │
│ [↺ Refresh KPI]  Terakhir diperbarui: 10:30:00   (auto-refresh 5 menit)     │
├──────────────────────────────────────────────────────────────────────────────┤
│ [JobProgressPanel — muncul di sini saat batch berjalan]                      │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │  Batch Akrual Harian — JOB-AKRUAL-2026-06-25                           │ │
│ │  ████████████████████████████░░░░  75%                                  │ │
│ │  Menghitung akrual instrumen 825 dari 1.100                             │ │
│ │  ETA: 10:31:15  •  [Batalkan]  [Lanjutkan di Background]               │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
├──────────────────────────────────────────────────────────────────────────────┤
│ FILTER BAR                                                                    │
│ [Cari kode instrumen...] [Status ▾] [Jenis ▾] [Stage ▾] [Stale ▾]          │
├──────────────────────────────────────────────────────────────────────────────┤
│ [DataTable — existing columns, already UX §1 compliant]                      │
│ (sort/page/filter/export all present — see audit above)                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Batch trigger dialog** (before POST):

```
┌────────────────────────────────────────────────────────┐
│ Jalankan Batch Akrual Harian?                          │
│                                                        │
│ Periode: PRD-2026-06 (Juni 2026)                       │
│ Semua instrumen aktif akan diproses.                   │
│                                                        │
│ Operasi ini tidak dapat dibatalkan setelah dimulai     │
│ (kecuali job canCancel=true).                          │
│                                                        │
│             [Batal]   [Jalankan Sekarang]              │
└────────────────────────────────────────────────────────┘
```

**Toast copy for akrual batch**:
- Confirm dialog triggers: "Mengirim permintaan batch..."
- POST 202 received: no toast yet — panel mounts
- SSE completed: "Batch akrual harian JOB-AKRUAL-2026-06-25 selesai. 1.100 instrumen diproses. Total akrual: Rp {total}."
- SSE failed: "Batch akrual gagal: {error.message}. Trace: {traceId}." + "Coba Lagi" button
- ROLE-AKUN-CTL approve: "Batch akrual JOB-AKRUAL-2026-06-25 berhasil di-approve. Jurnal akrual akan di-post."

**KPI auto-refresh**: 5-minute polling using `useInterval` (same pattern as M15 dashboards). `document.visibilityState` check to pause when tab not visible.

---

## 6. Batch Job Spec: MTM Upload + Akrual Batch

### 6.1 JobProgressPanel Placement

Per UX §3, `<JobProgressPanel>` is placed **inline** on the page that triggered the job — not in a modal, not in the global notification center only. The operator sees progress in context.

| Screen | Placement | Dismissible? |
|---|---|---|
| `/transaksi/mtm/upload` | Below the `MtmUploadDropzone`, above "Post-upload summary" | Via "Lanjutkan di Background" button |
| `/transaksi/akrual` | Above the DataTable, below KPI cards | Via "Lanjutkan di Background" button |

### 6.2 SSE Subscription + Fallback

```
useJobProgress(jobId):
  1. Open EventSource at /api/v1/jobs/{jobId}/stream
  2. On 'progress' event: update state {progress, currentStep, eta}
  3. On 'completed' event: update state; fire onComplete(result) callback; close ES
  4. On 'failed' event: update state; fire onFail(error) callback; close ES
  5. On EventSource.onerror:
     - Close EventSource
     - Start polling: GET /api/v1/jobs/{jobId} every 2000ms
     - Same state updates from polling response
     - On completed/failed: clearInterval
  6. On unmount: close EventSource / clearInterval (cleanup)
```

### 6.3 Cancel + Background Mode

- **Cancel**: `POST /api/v1/jobs/{jobId}/cancel`. Visible only if `job.canCancel === true`. After cancel confirm: toast info: "Job {jobId} dibatalkan."
- **Background**: closes the inline panel. Global notification badge in top nav shows active job count. On completion, badge count updates + toast fires from a global subscription.

### 6.4 JobProgressPanel Visual States

```
State: QUEUED
  [Clock icon] Menunggu di antrian... Job ID: {jobId}
  [Batalkan]   [Lanjutkan di Background]

State: RUNNING  
  [Spinner icon] {JobTitle}
  [████████░░░░░░░] {N}%
  {currentStep}
  Mulai: {startedAt} • ETA: {eta} ({N} detik lagi)
  [Batalkan]   [Lanjutkan di Background]

State: COMPLETED
  [CheckCircle icon — green] Selesai
  {successMessage from result}
  [Lihat Hasil →]

State: FAILED
  [XCircle icon — red] Gagal
  {error.message}
  Trace: {traceId}
  [Coba Lagi]   [Tutup]

State: CANCELLED
  [Ban icon — gray] Dibatalkan
  [Tutup]
```

---

## 7. `TransaksiTabNav.tsx` Component Design

### 7.1 Component Contract

```
// TransaksiTabNav.tsx — "use client" island
Props:
  permittedTabs: Array<{
    key: 'penempatan'|'mtm'|'renewal'|'penjualan'|'jatuh-tempo'|'akrual'
    label: string           // Bahasa Indonesia display label
    href: string            // absolute path
    badge?: number          // count; omit or 0 = no badge
    badgeVariant?: 'default' | 'warning'
  }>
  activeCTA?: {
    label: string
    ariaLabel: string
    href: string
  } | null
```

The parent Server Component (`layout.tsx`) computes `permittedTabs` from JWT permissions and passes it as a prop. The client island uses `usePathname()` to determine which tab is active.

### 7.2 Tab Keyboard Navigation

Per WCAG 2.1 success criterion 2.1.1 and the ARIA `tablist` pattern:

- Tab key: moves focus to the tab nav (the first tab, or the last focused tab if returning)
- Arrow Right / Arrow Left: moves between tabs in the list (wraps at ends)
- Enter / Space: activates the focused tab (navigates to href)
- Tab key from within tab: moves focus to main content (skipping remaining tabs)

The shadcn/ui `Tabs` primitive handles this keyboard pattern natively when configured correctly with `activationMode="manual"` to separate focus from navigation (allowing Arrow keys to move focus without immediately triggering navigation, which is disorienting in SSR tab nav).

### 7.3 ARIA Structure

```html
<nav aria-label="Navigasi Transaksi">
  <ul role="tablist">
    <li role="presentation">
      <a
        href="/transaksi/penempatan"
        role="tab"
        aria-selected="true"              ← active tab
        aria-controls="tabpanel-transaksi"
        id="tab-penempatan"
      >
        Penempatan
        <span aria-label="3 menunggu review" class="badge">3</span>
      </a>
    </li>
    <li role="presentation">
      <a
        href="/transaksi/mtm"
        role="tab"
        aria-selected="false"
        tabindex="-1"                     ← non-active tabs: tabindex=-1
      >
        MTM
        <span aria-label="Harga stale terdeteksi" class="badge-warning">!</span>
      </a>
    </li>
    <!-- only permitted tabs rendered -->
  </ul>
</nav>

<main id="tabpanel-transaksi" role="tabpanel" aria-labelledby="tab-penempatan">
  {children}
</main>
```

Note: because the tab nav drives full-page navigation (not in-page panel switching), `aria-controls` points to the `<main>` element rather than separate panel divs. This is consistent with the ARIA tab pattern for multi-page tab-style navigation.

---

## 8. Redirect Implementation Spec for `next.config.js`

### 8.1 Redirect Block Structure

```js
// next.config.js — redirects() addition
async redirects() {
  return [
    // MTM — specific paths BEFORE wildcard
    {
      source: '/mtm/alerts/stale-price',
      destination: '/transaksi/mtm/alerts/stale-price',
      permanent: true,  // 308
    },
    {
      source: '/mtm/upload/batch/:batch_id',
      destination: '/transaksi/mtm/upload/batch/:batch_id',
      permanent: true,
    },
    {
      source: '/mtm/upload',
      destination: '/transaksi/mtm/upload',
      permanent: true,
    },
    {
      source: '/mtm/cron',
      destination: '/transaksi/mtm/cron',
      permanent: true,
    },
    {
      source: '/mtm/:id',
      destination: '/transaksi/mtm/:id',
      permanent: true,
    },
    {
      source: '/mtm',
      destination: '/transaksi/mtm',
      permanent: true,
    },
    // Penempatan — specific paths BEFORE wildcard
    {
      source: '/trx/penempatan/new',
      destination: '/transaksi/penempatan/new',
      permanent: true,
    },
    {
      source: '/trx/penempatan/:id/edit',
      destination: '/transaksi/penempatan/:id/edit',
      permanent: true,
    },
    {
      source: '/trx/penempatan/:id',
      destination: '/transaksi/penempatan/:id',
      permanent: true,
    },
    {
      source: '/trx/penempatan',
      destination: '/transaksi/penempatan',
      permanent: true,
    },
  ]
}
```

**Order matters**: Next.js processes redirects in array order. More specific paths must precede wildcards. The above ordering is correct.

**Security-engineer verification items**:
- Confirm 308 status code (not 301) — Next.js `permanent: true` maps to 308 by default for App Router.
- Confirm no query string stripping (Next.js preserves query strings in redirects by default).
- Confirm no partial render before redirect fires (must happen at config layer, not middleware).
- Test with curl: `curl -I http://localhost:3000/mtm/alerts/stale-price` must return 308, not 200.

---

## 9. Audit Checklist per Existing Screen

### 9.1 Renewal (`/transaksi/renewal/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort | Sort | PASS | Default sort: fix to `tanggal_renewal:desc` |
| §1 Paging | Pagination | PASS | No change |
| §1 Filter | Filter | PARTIAL | Add `filter[tanggal_renewal]` date range |
| §1 Export | Export | PARTIAL | Add async threshold check (> 10k → job) |
| §2 Sukses notif | Form (new/page.tsx) | UNKNOWN | Frontend-engineer to verify |
| §2 Gagal notif | Form (new/page.tsx) | UNKNOWN | Frontend-engineer to verify |
| §2 Pending state | Form (new/page.tsx) | UNKNOWN | Frontend-engineer to verify |
| §3 Job progress | N/A (no batch op) | N/A | — |
| SoD absent-from-DOM | List + detail | PASS | Correct implementation already present |
| URL state deep-link | Sort + filter | PASS | Sort default value fix needed |
| Empty/loading/error | All states | PASS | DataTable handles these |
| Idempotency-Key | Form submit | UNKNOWN | Frontend-engineer to verify auto-inject |

### 9.2 Penjualan (`/transaksi/penjualan/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort | Sort | PARTIAL | Fix: use `useQueryState` instead of `useState + useEffect` for sort |
| §1 Paging | Pagination | PASS | No change |
| §1 Filter | Filter | PARTIAL | Add `filter[bm_alert]=true` shortcut button |
| §1 Export | Export | PARTIAL | Fix: pass `filter[jenis_disposal]` to export URL |
| §2 Sukses notif | Form (new/page.tsx) | UNKNOWN | Frontend-engineer to verify |
| §2 Gagal notif | Form (new/page.tsx) | UNKNOWN | Frontend-engineer to verify |
| §3 Job progress | N/A (no batch op) | N/A | — |
| Breadcrumb | List | MISSING | Add `<nav aria-label="Breadcrumb">` |
| BM warning on form | Penjualan new | NOT VERIFIED | Frontend-engineer to check form page |
| SoD absent-from-DOM | List actions | PASS | `canApprove` gate correct |
| URL state deep-link | Sort | BROKEN | Fix sort to use `useQueryState` |

### 9.3 Jatuh Tempo (`/transaksi/jatuh-tempo/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort | Sort | PARTIAL | Default sort: fix to `tanggal_jatuh_tempo:asc` |
| §1 Paging | Pagination | PASS | No change |
| §1 Filter | Filter | PARTIAL | Add date range quick-filter buttons |
| §1 Export | Export | PASS | Export with filter params already correct |
| §2 Notif | N/A (read-only) | N/A | Read-only screen; no form submission |
| §3 Job progress | N/A | N/A | — |
| `hari_tersisa` column | List | MISSING | Add computed column |
| "Buat Renewal" CTA | Row action | MISSING | Add per-row link for UPCOMING rows |
| Mutation buttons gating | Cron trigger | PARTIAL | Wrap `AkrualCronTriggerButton` in permission check |

### 9.4 Akrual (`/transaksi/akrual/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort | Sort | PARTIAL | Fix to use `useQueryState` for sort (same issue as penjualan) |
| §1 Paging | Pagination | PASS | No change |
| §1 Filter | Filter | PASS | All filters present |
| §1 Export | Export | PARTIAL | Add async threshold check (> 10k → job) |
| §2 Sukses notif | Batch trigger | PARTIAL | Add toast after job completes (not just after POST) |
| §2 Gagal notif | Batch trigger | MISSING | No error toast on SSE failed currently |
| §3 Job progress | Batch trigger | MISSING | No JobProgressPanel — PRIMARY M16 ADDITION |
| KPI dashboard cards | List page | MISSING | Add KPI section above DataTable |
| "Approve Batch" button | List | MISSING | Add for ROLE-AKUN-CTL |
| Confirmation dialog | Batch trigger | MISSING | Add pre-trigger dialog |
| DataTable auto-refresh | After job | MISSING | Add `invalidateQueries` on SSE completed |

### 9.5 Penempatan (`/trx/penempatan/` → `/transaksi/penempatan/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort | Sort | PASS | Works; local state approach acceptable |
| §1 Paging | Pagination | PASS | Cursor history pattern correct |
| §1 Filter | Filter | PARTIAL | Missing `filter[jenis_instrumen]`, `filter[stage]`, `filter[counterparty_id]`. Currently only `filter[workflow_status]`. |
| §1 Export | Export | PARTIAL | Current export uses `window.open` with `notify.info` — add async threshold check and proper audit |
| §2 Sukses notif | Form (new) | PARTIAL | `notify.info` used for export error — should be `notify.error`. Verify new form uses `notify.success` with specific message. |
| §2 Gagal notif | Form | UNKNOWN | Frontend-engineer to verify specific error messages |
| §3 Job progress | Export | MISSING | Large export (> 10k) needs JobProgressPanel |
| URL state | Sort + filter | PARTIAL | Status filter in local state (not URL). Fix: move to `useQueryState` for deep-link |
| Breadcrumb | List | MISSING | No breadcrumb in current implementation |
| Native `<select>` for filter | Filter | PARTIAL | Using native `<select>` instead of shadcn `Select` — fix for visual consistency |

### 9.6 MTM (`/mtm/` → `/transaksi/mtm/`)

| UX Rule | Pillar | Status | M16 Action Required |
|---|---|---|---|
| §1 Sort/Page/Filter/Export | List | UNKNOWN | Need to read `/mtm/page.tsx` — assume gaps based on early `/trx/` patterns |
| §3 Job progress | Upload | MISSING | Primary M16 addition — replace sync result with SSE JobProgressPanel |
| Upload breadcrumb | Upload page | PARTIAL | Breadcrumb links to `/mtm` not `/transaksi/mtm` — fix after git mv |
| Batch detail links | Upload page | BROKEN | All links in `/mtm/upload/page.tsx` point to `/mtm/...` — fix after git mv |

---

## 10. Bahasa Indonesia Copy Table

### 10.1 Tab Labels

| Key | ID | EN (export/report) |
|---|---|---|
| tab.penempatan | Penempatan | Placements |
| tab.mtm | MTM | Mark-to-Market |
| tab.renewal | Renewal | Renewals |
| tab.penjualan | Penjualan | Sales/Disposals |
| tab.jatuh_tempo | Jatuh Tempo | Maturities |
| tab.akrual | Akrual | Accruals |

### 10.2 Penempatan Labels

| Key | ID | EN |
|---|---|---|
| penempatan.title | Penempatan Deposito | Deposit Placements |
| penempatan.new | + Penempatan Baru | + New Placement |
| penempatan.new.ariaLabel | Tambah Penempatan Baru | Add New Placement |
| penempatan.form.jenis | Jenis Instrumen | Instrument Type |
| penempatan.form.counterparty | Counterparty / Bank | Counterparty / Bank |
| penempatan.form.nominal | Nominal (IDR) | Nominal (IDR) |
| penempatan.form.tanggal | Tanggal Penempatan | Placement Date |
| penempatan.form.tenor | Tenor (hari) | Tenor (days) |
| penempatan.form.rate | Suku Bunga (% p.a.) | Interest Rate (% p.a.) |
| penempatan.form.metode | Metode Bunga | Interest Method |
| penempatan.form.save | Simpan sebagai Draft | Save as Draft |
| penempatan.form.saving | Menyimpan... | Saving... |
| penempatan.toast.created | Penempatan {kode} berhasil dibuat sebagai draft. Menunggu submit ke reviewer. | Placement {kode} saved as draft. Awaiting reviewer submission. |
| penempatan.toast.submitted | {kode} berhasil di-submit ke reviewer. Menunggu tanda tangan reviewer. | {kode} submitted to reviewer. Awaiting signature. |
| penempatan.toast.reviewed | {kode} berhasil di-review. Menunggu persetujuan approver. | {kode} reviewed. Awaiting approver. |
| penempatan.toast.approved | {kode} berhasil disetujui dan aktif. | {kode} approved and active. |
| penempatan.toast.rejected | {kode} ditolak. Dikembalikan ke maker dengan komentar. | {kode} rejected. Returned to maker. |
| penempatan.error.sod | Anda tidak bisa menjadi reviewer untuk transaksi yang Anda buat sendiri. | SoD violation: you cannot review your own transaction. |
| penempatan.error.conflict | Data sudah diubah oleh pengguna lain. Muat ulang halaman untuk melihat versi terbaru. | Conflict: data changed by another user. Reload to see latest. |
| penempatan.filter.status | Status | Status |
| penempatan.filter.jenis | Jenis Instrumen | Instrument Type |
| penempatan.filter.stage | Stage | Stage |
| penempatan.empty.filter | Tidak ada penempatan yang cocok dengan filter. | No placements match the active filters. |
| penempatan.empty.no_data | Belum ada data penempatan. | No placement data yet. |
| penempatan.export.success | Export penempatan berhasil. {N} baris diunduh. | Placement export complete. {N} rows downloaded. |

### 10.3 MTM Labels

| Key | ID | EN |
|---|---|---|
| mtm.title | MTM Harian | Daily MTM |
| mtm.upload.title | Upload Harga MTM Manual | Upload MTM Prices |
| mtm.upload.dropzone | Taruh file di sini atau klik untuk browse | Drop file here or click to browse |
| mtm.upload.formats | CSV, XLSX (format IBPA atau BEI) | CSV, XLSX (IBPA or BEI format) |
| mtm.upload.maxsize | Maks. 50 MB | Max 50 MB |
| mtm.upload.btn | Upload | Upload |
| mtm.upload.uploading | Mengunggah... | Uploading... |
| mtm.upload.toast.complete | Batch upload MTM {jobId} selesai. {N} record diproses. | MTM batch upload {jobId} complete. {N} records processed. |
| mtm.upload.toast.failed | Upload batch MTM gagal: {message}. Trace: {traceId}. | MTM batch upload failed: {message}. Trace: {traceId}. |
| mtm.filter.stale | Harga Stale | Stale Price |
| mtm.alert.stale.link | Lihat Stale Price Alerts → | View Stale Price Alerts → |
| mtm.job.parsing | Parsing baris {current} dari {total} | Parsing row {current} of {total} |
| mtm.error.invalid_format | Format file tidak didukung. Gunakan CSV atau XLSX. | Unsupported file format. Use CSV or XLSX. |
| mtm.error.too_large | Ukuran file melebihi batas 50 MB. | File size exceeds 50 MB limit. |

### 10.4 Renewal Labels

| Key | ID | EN |
|---|---|---|
| renewal.title | Renewal Deposito | Deposit Renewals |
| renewal.new | + Renewal Baru | + New Renewal |
| renewal.form.save | Simpan Draft | Save Draft |
| renewal.toast.created | Renewal {kode} berhasil dibuat. Menunggu submit ke reviewer. | Renewal {kode} created. Awaiting reviewer submission. |
| renewal.toast.approved | Renewal {kode} berhasil di-approve. Jurnal otomatis akan di-buat. | Renewal {kode} approved. Journal will be auto-created. |
| renewal.error.past_due | Renewal tidak dapat dibuat: instrumen {kode} sudah melewati tanggal jatuh tempo. | Cannot create renewal: instrument {kode} is past maturity. |
| renewal.error.periode_closed | Periode buku {periode} sudah closed. Tidak bisa membuat transaksi baru. | Accounting period {periode} is closed. Cannot create new transactions. |

### 10.5 Penjualan Labels

| Key | ID | EN |
|---|---|---|
| penjualan.title | Penjualan / Pencairan Instrumen | Instrument Sales / Disposals |
| penjualan.new | + Penjualan Baru | + New Sale |
| penjualan.bm_alert.warning | Perhatian: penjualan instrumen ini mungkin berdampak pada Business Model portfolio. Konsultasikan dengan Risk Officer. | Warning: selling this instrument may impact the portfolio Business Model. Consult Risk Officer. |
| penjualan.bm_alerts.title | BM Frequency Alerts | BM Frequency Alerts |
| penjualan.filter.bm_alert | BM Alert | BM Alert |
| penjualan.toast.approved | Penjualan {kode} berhasil di-approve. | Sale {kode} approved. |

### 10.6 Jatuh Tempo Labels

| Key | ID | EN |
|---|---|---|
| jatuh_tempo.title | Jatuh Tempo — Maturity Events | Maturity Events |
| jatuh_tempo.filter.7d | Dalam 7 hari | Within 7 days |
| jatuh_tempo.filter.30d | Dalam 30 hari | Within 30 days |
| jatuh_tempo.filter.past_due | Sudah Jatuh Tempo | Past Due |
| jatuh_tempo.col.hari_tersisa | Hari Tersisa | Days Remaining |
| jatuh_tempo.col.hari_tersisa.past | −{N} hari (lewat jatuh tempo) | −{N} days (past maturity) |
| jatuh_tempo.cta.renewal | Buat Renewal | Create Renewal |
| jatuh_tempo.empty | Tidak ada maturity event yang cocok dengan filter. | No maturity events match the filter. |

### 10.7 Akrual Labels

| Key | ID | EN |
|---|---|---|
| akrual.title | Pendapatan Akrual Harian | Daily Accrual Income |
| akrual.trigger.btn | Jalankan Batch Akrual Harian | Run Daily Accrual Batch |
| akrual.trigger.confirm.title | Jalankan Batch Akrual Harian? | Run Daily Accrual Batch? |
| akrual.trigger.confirm.body | Periode: {kode}. Semua instrumen aktif akan diproses. | Period: {kode}. All active instruments will be processed. |
| akrual.trigger.confirm.cta | Jalankan Sekarang | Run Now |
| akrual.approve.btn | Approve Batch Akrual | Approve Accrual Batch |
| akrual.kpi.total | Total Akrual Hari Ini | Today's Total Accrual |
| akrual.kpi.instrumen | Instrumen Diproses | Instruments Processed |
| akrual.kpi.status | Status Batch Terakhir | Last Batch Status |
| akrual.job.step | Menghitung akrual instrumen {current} dari {total} | Calculating accrual for instrument {current} of {total} |
| akrual.toast.complete | Batch akrual harian {jobId} selesai. {N} instrumen diproses. Total akrual: Rp {total}. | Daily accrual batch {jobId} complete. {N} instruments processed. Total: Rp {total}. |
| akrual.toast.failed | Batch akrual gagal: {message}. Trace: {traceId}. | Accrual batch failed: {message}. Trace: {traceId}. |
| akrual.toast.approved | Batch akrual {jobId} berhasil di-approve. Jurnal akrual akan di-post. | Accrual batch {jobId} approved. Accrual journals will be posted. |

### 10.8 Shared UI Copy

| Key | ID | EN |
|---|---|---|
| shared.filter.clear_all | Bersihkan semua filter | Clear all filters |
| shared.filter.search_placeholder | Cari... | Search... |
| shared.export.btn | Ekspor | Export |
| shared.export.csv | CSV | CSV |
| shared.export.xlsx | Excel (XLSX) | Excel (XLSX) |
| shared.refresh | ↺ Refresh | ↺ Refresh |
| shared.last_updated | Terakhir diperbarui: {time} | Last updated: {time} |
| shared.pagination.prev | ← Sebelumnya | ← Previous |
| shared.pagination.next | Selanjutnya → | Next → |
| shared.pagination.summary | Halaman {page} dari ~{total} | Page {page} of ~{total} |
| shared.pagination.limit | Tampilkan | Show |
| shared.empty.filtered | Tidak ada data yang cocok dengan filter ini. | No data matches the current filters. |
| shared.loading | Memuat data... | Loading data... |
| shared.error.retry | Coba lagi | Retry |
| shared.job.cancel | Batalkan | Cancel |
| shared.job.background | Lanjutkan di Background | Continue in Background |
| shared.job.view_result | Lihat Hasil → | View Result → |
| shared.view_detail | Lihat detail → | View detail → |
| shared.sod.error | Anda tidak bisa menjadi reviewer/approver untuk transaksi yang Anda buat sendiri. | SoD: you cannot review or approve your own transaction. |

---

## 11. Accessibility Checklist

### 11.1 Layout Shell

- [x] "Lewati ke konten utama" (`#main-content`) as first focusable element, visually hidden, appears on focus
- [x] `<nav aria-label="Navigasi Transaksi">` wraps tab list
- [x] `<ul role="tablist">` with `<li role="presentation">` children
- [x] Each tab: `role="tab"` + `aria-selected="true|false"` + `id` for `aria-labelledby`
- [x] Active tab: `aria-selected="true"`, `tabindex="0"`; inactive tabs: `tabindex="-1"`
- [x] Main content: `role="tabpanel"` + `aria-labelledby="{active-tab-id}"`
- [x] Breadcrumb: `<nav aria-label="Breadcrumb"><ol>` with `aria-current="page"` on last item
- [x] CTA button: `aria-label="Tambah {SubRoute} Baru"` (not just icon + short label)
- [x] Tab badges: `<span aria-label="{N} {meaning}" class="badge">` not just the count

### 11.2 Tab Keyboard Navigation

- [x] Tab key enters tab nav at active tab (or first tab if none active)
- [x] Arrow Right moves focus to next permitted tab (wraps at end)
- [x] Arrow Left moves focus to previous permitted tab (wraps at start)
- [x] Home key moves to first permitted tab
- [x] End key moves to last permitted tab
- [x] Enter / Space activates focused tab (triggers navigation)
- [x] Shift+Tab from tab nav returns to previous focus target (not into tabs)
- [x] Tab from within tabs moves to main content

### 11.3 DataTable Accessibility

- [x] `<table>` with proper `<thead>` / `<tbody>` / `<th scope="col">` for each header
- [x] Sort headers: `aria-sort="ascending|descending|none"` on `<th>` when sorted
- [x] Sort indicator: icon visible, also communicated via `aria-label` on sort button ("Urutkan {kolom} naik/turun/batal")
- [x] Filter inputs: `<label>` associated via `htmlFor` or `aria-label`
- [x] Error fields: `aria-describedby="{field-id}-error"` on input; `id="{field-id}-error"` on error message
- [x] Loading state: `aria-busy="true"` on table container during load
- [x] Empty state: `role="status"` on empty message container

### 11.4 Color and Visual Signals

Color must never be the sole signal per WCAG 1.4.1 (Use of Color):

- Stage badges: colored background + text label + icon (e.g., "Stage 2" not just amber)
- Workflow status badges: color + text + icon (e.g., green checkmark + "Aktif")
- Stale price badge: amber + "!" icon + label "Stale" — not just amber color
- Error fields: red border + error icon + inline text message — not just red border
- Export async threshold: progress bar shows step text alongside visual fill
- Jatuh Tempo past-due: red color + "−{N} hari" text + icon — not just red
- MTM delta %: red/green color + "+" or "−" prefix + % symbol

### 11.5 Contrast Requirements (WCAG 2.1 AA)

All text: ≥ 4.5:1 contrast ratio against background.
Large text (≥ 18pt or ≥ 14pt bold): ≥ 3:1 ratio.
Interactive components (borders of inputs, focus rings): ≥ 3:1 against adjacent color.

Shadow/muted-foreground text (e.g., `text-muted-foreground`) must be checked: shadcn default `hsl(var(--muted-foreground))` on white background is typically ~4.7:1 — passes AA for normal text. Verify in production at smallest font size used.

Focus ring: `focus-visible:ring-2 focus-visible:ring-ring` from shadcn. The ring color must have ≥ 3:1 contrast against the element background. This is satisfied by shadcn defaults on white/light background.

### 11.6 JobProgressPanel Accessibility

- [x] Progress bar: `<progress>` element or `role="progressbar"` with `aria-valuenow` + `aria-valuemin="0"` + `aria-valuemax="100"` + `aria-label="{job type} progress"`
- [x] `aria-live="polite"` on `currentStep` text — updates announced without interrupting user
- [x] On completion: focus returns to the trigger button (or a logical element) — not trapped in panel
- [x] Cancel button: `aria-label="Batalkan job {jobId}"`
- [x] Error state: `role="alert"` for immediate announcement

### 11.7 Form Accessibility

- [x] All inputs have associated `<label>` elements (not placeholder-only)
- [x] Required fields: `aria-required="true"` + visual `*` indicator with legend explaining `*` = required
- [x] Validation errors: `aria-describedby="{input-id}-error"` on input; error message `role="alert"` for VALIDATION_FAILED toast
- [x] Auto-complete where appropriate: `autocomplete="off"` on financial amount fields (prevent browser interference with IDR formatting)
- [x] Spinner during submit: `aria-label="Menyimpan..."` on button; `aria-disabled="true"` (not `disabled` attribute) to keep it keyboard-navigable

---

## 12. Component Handoff Checklist for `frontend-engineer-nextjs`

### 12.1 New Files to Create

| File | Type | Reuses |
|---|---|---|
| `frontend/src/app/transaksi/layout.tsx` | Next.js Server Component | — |
| `frontend/src/components/blips/transaksi/TransaksiTabNav.tsx` | React Client Component | shadcn Tabs |
| `frontend/src/components/blips/transaksi/index.ts` | Barrel export | — |

### 12.2 Files to Move (git mv)

| From | To |
|---|---|
| `frontend/src/app/trx/penempatan/page.tsx` | `frontend/src/app/transaksi/penempatan/page.tsx` |
| `frontend/src/app/trx/penempatan/new/page.tsx` | `frontend/src/app/transaksi/penempatan/new/page.tsx` |
| `frontend/src/app/trx/penempatan/[id]/page.tsx` | `frontend/src/app/transaksi/penempatan/[id]/page.tsx` |
| `frontend/src/app/trx/penempatan/[id]/edit/page.tsx` | `frontend/src/app/transaksi/penempatan/[id]/edit/page.tsx` |
| `frontend/src/app/mtm/page.tsx` | `frontend/src/app/transaksi/mtm/page.tsx` |
| `frontend/src/app/mtm/upload/page.tsx` | `frontend/src/app/transaksi/mtm/upload/page.tsx` |
| `frontend/src/app/mtm/upload/batch/[batch_id]/page.tsx` | `frontend/src/app/transaksi/mtm/upload/batch/[batch_id]/page.tsx` |
| `frontend/src/app/mtm/cron/page.tsx` | `frontend/src/app/transaksi/mtm/cron/page.tsx` |
| `frontend/src/app/mtm/[id]/page.tsx` | `frontend/src/app/transaksi/mtm/[id]/page.tsx` |
| `frontend/src/app/mtm/alerts/stale-price/page.tsx` | `frontend/src/app/transaksi/mtm/alerts/stale-price/page.tsx` |

After `git mv`: update all internal links in moved files from `/trx/penempatan/` and `/mtm/` to `/transaksi/penempatan/` and `/transaksi/mtm/` respectively.

### 12.3 Files to Modify (Audit Gap Fixes)

| File | Changes |
|---|---|
| `frontend/src/app/transaksi/renewal/page.tsx` | Fix default sort; add date range filter; fix export async threshold |
| `frontend/src/app/transaksi/renewal/new/page.tsx` | Verify/add: Idempotency-Key auto-inject; specific toast messages; field-level error display |
| `frontend/src/app/transaksi/penjualan/page.tsx` | Add breadcrumb; fix sort to useQueryState; add BM alert quick-filter; fix export filter params |
| `frontend/src/app/transaksi/penjualan/new/page.tsx` | Add BM alert warning banner; verify Idempotency-Key; verify toast messages |
| `frontend/src/app/transaksi/jatuh-tempo/page.tsx` | Fix default sort to asc; add quick-filter buttons; add hari_tersisa column; add "Buat Renewal" CTA; gate cron trigger by permission |
| `frontend/src/app/transaksi/akrual/page.tsx` | Add KPI cards; add confirmation dialog for batch trigger; add JobProgressPanel; add "Approve Batch" for AKUN-CTL; fix sort to useQueryState; add DataTable auto-refresh on completion |
| `frontend/src/app/transaksi/mtm/upload/page.tsx` (after mv) | Replace synchronous result display with JobProgressPanel + SSE; update all internal links |
| `frontend/src/app/transaksi/penempatan/page.tsx` (after mv) | Add breadcrumb; move filter state to useQueryState; add jenis/stage/counterparty filters; fix native select to shadcn Select; update links from /trx/ to /transaksi/ |
| `next.config.js` | Add redirects() block (10 rules) |

### 12.4 Validation Rules per Form

**Penempatan new/edit**:
- `jenis_instrumen`: required; enum `['DEPOSITO','OBLIGASI','SAHAM','REKSADANA','KAS']`
- `counterparty_id`: required; UUID; typeahead from `GET /api/v1/master/counterparty`
- `nominal_idr`: required; `> 0`; numeric; no decimals for IDR
- `tanggal_penempatan`: required; date; not in the past (soft warning, not hard block)
- `tenor_hari`: required; `> 0`; integer
- `suku_bunga_persen`: required; `>= 0`; `<= 100`; 2 decimal places
- `metode_bunga`: required; enum `['AKTUAL/365','AKTUAL/360','30/360']`

**Renewal new**:
- `instrumen_id`: required; UUID; linked to APPROVED_ACTIVE penempatan
- `tenor_baru_bulan`: required; `> 0`; integer
- `rate_baru_persen`: required; `>= 0`; `<= 100`
- `pokok_baru`: required; `> 0`; `<= original nominal`
- `tanggal_efektif_baru`: required; date; must be on or after original maturity date

**Penjualan new**:
- `instrumen_id`: required; UUID
- `tanggal_eksekusi`: required; date
- `qty_terjual`: required; `> 0`
- `harga_per_unit`: required; `> 0`
- `jenis_disposal`: required; enum `['SELL','REDEEM','MATURE']`

### 12.5 Idempotency-Key Pattern

All form submit handlers must use this pattern:

```typescript
// useIdempotencyKey.ts — shared hook
import { useRef } from "react";
import { v4 as uuidv4 } from "uuid";

export function useIdempotencyKey() {
  const keyRef = useRef<string>(uuidv4());
  const rotate = () => { keyRef.current = uuidv4(); };
  return { getKey: () => keyRef.current, rotate };
}
```

Usage in form:
- `useRef` initializes key on mount (not on every render)
- Key injected into every mutating request header as `Idempotency-Key`
- After successful form submission: call `rotate()` to generate fresh key (prevents IDEMPOTENCY_REPLAY on subsequent new forms)
- After error: do NOT rotate — same key allows safe retry

### 12.6 Components to Reuse (Do Not Redesign)

All the following components are pre-existing and must be reused without modification to their internal logic or visual design:

- `frontend/src/components/blips/DataTable.tsx`
- `frontend/src/components/blips/JobProgressPanel.tsx`
- `frontend/src/components/blips/penempatan/PenempatanWorkflowPanel.tsx`
- `frontend/src/components/blips/penempatan/PenempatanStatusBadge.tsx`
- `frontend/src/components/blips/penempatan/EIRPreviewSidePanel.tsx`
- `frontend/src/components/blips/penempatan/dialogs/SubmitDialog.tsx`
- `frontend/src/components/blips/penempatan/dialogs/ReviewDialog.tsx`
- `frontend/src/components/blips/penempatan/dialogs/ApproveDialog.tsx`
- `frontend/src/components/blips/penempatan/dialogs/RejectDialog.tsx`
- `frontend/src/components/blips/penempatan/dialogs/WithdrawDialog.tsx`
- `frontend/src/components/blips/mtm/MtmUploadDropzone.tsx`
- `frontend/src/components/blips/mtm/MtmStatusBadge.tsx`
- `frontend/src/components/blips/mtm/MtmStaleBadge.tsx`
- `frontend/src/components/blips/mtm/MtmCronTriggerButton.tsx`

The only widget-level change permitted by M16 is the `onSuccess` callback signature of `MtmUploadDropzone` — if it currently returns a full `MtmUploadBatchResponse`, it should be extended to also support returning `{ jobId: string }` when the backend issues a 202 instead of 200. This is an interface extension, not a redesign.

### 12.7 Export Async Threshold Implementation

All DataTable export handlers must implement the 10k row threshold:

```typescript
const handleExport = async (format: "csv" | "xlsx") => {
  const totalEstimate = data?.pagination?.totalEstimate ?? 0;
  
  if (totalEstimate > 10_000) {
    // Async path: POST job
    const { data: job } = await api.export.createJob({
      entity: "penempatan",
      format,
      filters: activeFilters,
      sort: currentSort,
    });
    // Mount JobProgressPanel for export job
    setExportJobId(job.jobId);
  } else {
    // Sync path: direct download
    const url = buildExportUrl({ format, filters: activeFilters, sort: currentSort });
    window.open(url, "_blank");
  }
};
```

Export `JobProgressPanel` appears in a collapsible area below the action bar, not in a modal.

---

## 13. Security Engineer Verification Checklist

Items requiring **BLOCKING** security review before merge:

### 13.1 Absent-from-DOM Verification
- [ ] No permitted tab renders any HTML for unauthorized roles — inspect server-side rendered HTML in test
- [ ] "Upload File", "Trigger Cron", "Approve Batch" buttons absent from DOM (not hidden, not disabled) for roles lacking permission
- [ ] "Review & Tandatangani" and "Approve" buttons absent from DOM when `currentUser.id === item.makerId`
- [ ] No HTML comment like `<!-- Tab: Penempatan (permission denied) -->` — nothing that leaks menu structure

### 13.2 308 Redirect Security
- [ ] Redirect fires before any page content renders (no partial SSR before redirect)
- [ ] Old routes `/trx/penempatan/*` and `/mtm/*` return 308 with empty body — verify with `curl -I`
- [ ] Query strings preserved in redirects (test: `/mtm?filter[status]=VALID` → `/transaksi/mtm?filter[status]=VALID`)
- [ ] Redirects not cacheable in a way that bypasses auth (Traefik cache must not cache 3xx responses)

### 13.3 SoD Enforcement
- [ ] UI absent-from-DOM is supplementary only; API must also enforce SoD (already enforced server-side in M1/M7/M8 — verify unchanged)
- [ ] E2E test: `POST /api/v1/transaksi/penempatan/{id}/review` by same user as maker returns 403 `SOD_VIOLATION`
- [ ] E2E test: Cannot access "Review" button via DOM manipulation (server component renders null)

### 13.4 Permission Check Pattern
- [ ] Server Components use JWT decoded from cookie/header — not from client-side Zustand store (Zustand is for client components only)
- [ ] `requirePermission()` utility returns `notFound()` (not redirect) for unauthorized route access — avoids confirming route existence to unauthorized users
- [ ] Permission strings match those seeded in Keycloak (same as M15 security gate items)

### 13.5 Idempotency-Key
- [ ] All mutation form submits include `Idempotency-Key` header — verify via network tab in E2E tests
- [ ] Key is UUID v4 format — validate against regex in E2E test
- [ ] Key is NOT visible to user (not in form, not in URL)
- [ ] On IDEMPOTENCY_REPLAY (200): toast shows correct message, no duplicate form processing

### 13.6 Export RBAC
- [ ] Export endpoint respects same filters that the user's permission scope enforces — data returned is bounded by `tenant_id` and user's accessible entity subset
- [ ] ROLE-AUDIT export returns full data (intended) but no mutation buttons rendered
- [ ] Export audit event `{ENTITY}.EXPORT` written to `aud.audit_log` in same transaction

---

## 14. AC Trace Matrix

| AC | Screen | Design Section |
|---|---|---|
| M16-01-AC1 | 308 redirect penempatan | §8 Redirect Spec |
| M16-01-AC2 | /transaksi/penempatan list DataTable | §5.1, §9.5 |
| M16-01-AC3 | /transaksi/penempatan/new form notif | §5.2 |
| M16-01-AC4 | Workflow + SoD enforcement | §5.3, §4.3 |
| M16-02-AC1 | 308 redirect MTM | §8 Redirect Spec |
| M16-02-AC2 | /transaksi/mtm/upload JobProgressPanel | §5.4, §6 |
| M16-02-AC3 | /transaksi/mtm list DataTable + stale filter | §5.5 |
| M16-02-AC4 | MTM role gate absent-from-DOM | §4.1, §4.3 |
| M16-03-AC1 | /transaksi/renewal list DataTable | §5.6, §9.1 |
| M16-03-AC2 | /transaksi/renewal/new form notif | §5.6, §10.4 |
| M16-03-AC3 | /transaksi/penjualan list + BM-alerts widget | §5.7 |
| M16-03-AC4 | Renewal/Penjualan SoD + workflow toast | §4.3, §10.5 |
| M16-04-AC1 | /transaksi/jatuh-tempo DataTable monitoring | §5.8, §9.3 |
| M16-04-AC2 | /transaksi/akrual batch trigger + JobProgressPanel | §5.9, §6 |
| M16-04-AC3 | Akrual role gate AKUN-CTL vs MAKER-TR | §4.1, §5.9 |
| M16-04-AC4 | /transaksi/akrual/dashboard KPI + DataTable | §5.9, §10.7 |
| M16-05-AC1 | Tab nav absent-from-DOM per permission | §3.3, §4.1, §7 |
| M16-05-AC2 | Tab active state + breadcrumb | §3.5, §7.2, §7.3 |
| M16-05-AC3 | CTA button visibility per sub-route | §3.4 |
| M16-05-AC4 | Layout accessibility keyboard + ARIA | §11 |
