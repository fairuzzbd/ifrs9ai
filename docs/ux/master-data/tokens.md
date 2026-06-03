# Design Tokens — Master Data Module (APP-A)

**Scope**: Delta dari shadcn/ui defaults yang spesifik untuk BLIPS Master Data screens.  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

Prinsip: token delta minimal — hanya override yang benar-benar dibutuhkan untuk finansial context dan WCAG AA compliance. Mayoritas menggunakan shadcn/ui defaults.

---

## Color Tokens (delta dari shadcn defaults)

### Workflow Status Colors

```css
/* Dipakai di WorkflowStatusBadge component */
--blips-status-draft-bg: hsl(215 20% 96%);         /* slate-100 */
--blips-status-draft-text: hsl(215 19% 35%);        /* slate-700 */
--blips-status-draft-border: hsl(215 19% 80%);      /* slate-300 */

--blips-status-pending-review-bg: hsl(45 93% 94%);  /* amber-100 */
--blips-status-pending-review-text: hsl(26 83% 31%);/* amber-800 */
--blips-status-pending-review-border: hsl(37 90% 70%);/* amber-300 */

--blips-status-pending-approval-bg: hsl(214 100% 94%); /* blue-100 */
--blips-status-pending-approval-text: hsl(221 83% 26%); /* blue-800 */
--blips-status-pending-approval-border: hsl(213 97% 75%); /* blue-300 */

--blips-status-pending-approval-2-bg: hsl(270 76% 95%); /* purple-100 */
--blips-status-pending-approval-2-text: hsl(268 57% 32%); /* purple-800 */
--blips-status-pending-approval-2-border: hsl(271 81% 75%); /* purple-300 */

--blips-status-approved-bg: hsl(142 76% 93%);       /* green-100 */
--blips-status-approved-text: hsl(141 63% 23%);     /* green-800 */
--blips-status-approved-border: hsl(141 83% 69%);   /* green-300 */

--blips-status-returned-bg: hsl(33 100% 93%);       /* orange-100 */
--blips-status-returned-text: hsl(21 88% 28%);      /* orange-800 */
--blips-status-returned-border: hsl(30 97% 73%);    /* orange-300 */
```

Semua pasangan memenuhi WCAG AA (contrast ratio ≥ 4.5:1 untuk teks normal).

### Banner Colors

```css
/* ECL Param Banner */
--blips-ecl-banner-bg: hsl(48 96% 89%);             /* amber-200 */
--blips-ecl-banner-border: hsl(43 96% 58%);         /* amber-400 */
--blips-ecl-banner-text: hsl(26 90% 20%);           /* amber-900 */

/* RETURNED Banner */
--blips-returned-banner-bg: hsl(48 96% 89%);        /* amber-100 */
--blips-returned-banner-border: hsl(37 90% 60%);    /* amber-400 */
--blips-returned-banner-text: hsl(26 90% 30%);      /* amber-800 */

/* ECL_PARAM_FROZEN */
--blips-frozen-bg: hsl(215 14% 95%);                /* slate-100 */
--blips-frozen-border: hsl(215 14% 70%);            /* slate-400 */
--blips-frozen-text: hsl(215 14% 42%);              /* slate-600 */

/* SICR Trigger */
--blips-sicr-bg: hsl(0 86% 97%);                    /* red-50 */
--blips-sicr-border: hsl(0 72% 51%);                /* red-500 */
--blips-sicr-text: hsl(0 63% 31%);                  /* red-800 */
```

### PII Field Colors

```css
/* Field termasking */
--blips-pii-masked-text: hsl(215 19% 55%);          /* slate-500 — intentionally muted */
--blips-pii-masked-font: var(--font-mono);           /* monospace agar * alignment rapi */
```

### Import Diff Colors

```css
/* Delta naik (PD lebih buruk — merah) */
--blips-diff-up-bg: hsl(0 86% 97%);                 /* red-50 */
--blips-diff-up-text: hsl(0 72% 45%);               /* red-600 */

/* Delta turun (PD lebih baik — hijau) */
--blips-diff-down-bg: hsl(142 52% 96%);             /* green-50 */
--blips-diff-down-text: hsl(141 63% 32%);           /* green-700 */

/* Baris baru */
--blips-diff-new-bg: hsl(214 100% 97%);             /* blue-50 */
--blips-diff-new-text: hsl(221 83% 36%);            /* blue-700 */
```

---

## Typography

Tidak ada override dari shadcn defaults. Catatan penggunaan:

```
Kode mata uang (CHAR 3 PK):    font-mono, font-bold, text-sm
Signature hash (sha256:...):   font-mono, text-xs, text-muted-foreground, truncate
PII masked field:              font-mono, text-muted-foreground
Amount/decimal:                font-mono, tabular-nums, text-right
Tanggal/waktu dalam tabel:     text-sm text-muted-foreground
Label field di form:           text-sm font-medium
Helper text di form:           text-xs text-muted-foreground
Komentar workflow (komentar):  text-sm italic text-muted-foreground dalam tanda kutip
```

---

## Spacing

Tidak ada override. Gunakan shadcn gap/padding defaults:

```
Form seksi: space-y-6 antar seksi
Form field: space-y-2 (label + input + helper)
Form field row (2 kolom): grid grid-cols-2 gap-4
Workflow stepper item: space-y-4
Panel workflow (kanan): p-6, border rounded-lg
Banner (RETURNED, ECL): p-4, rounded-md, border
```

---

## Component Sizing

```
DataTable row height: 48px (h-12) — lebih tinggi dari shadcn default untuk tabel finansial yang dense
WorkflowStatusBadge: px-2 py-0.5, text-xs, rounded-full
Action bar: h-10, sticky top di mobile
Filter chip: px-2 py-1, text-xs, rounded-md, cursor-pointer, hover:bg-muted
```

---

## Motion

Minimal animation, sesuai financial context (tidak perlu delight animation):

```
Skeleton shimmer: 1.5s linear infinite (CSS animation)
Toast masuk: slide-in-from-top-right, 200ms ease-out
Toast keluar: fade-out, 150ms ease-in
Workflow step expand: collapse/expand 200ms ease-in-out
Filter chip appear: fade-in 100ms
```

Gunakan `prefers-reduced-motion` media query — matikan semua animasi jika user setting reduce motion.

---

## Icon Usage

Sumber: Lucide React (sudah bundled dengan shadcn/ui).

| Konteks | Ikon Lucide | Catatan |
|---|---|---|
| Sort asc | `ArrowUp` | size-4 |
| Sort desc | `ArrowDown` | size-4 |
| Sort inactive | `ArrowUpDown` | size-4, text-muted |
| Multi-sort indicator | `ArrowUpDown` + badge angka | |
| Filter aktif | `Filter` dengan dot merah | |
| Export | `Download` | |
| Refresh | `RefreshCw` | |
| Action menu | `MoreHorizontal` | ••• |
| Edit | `Pencil` | |
| Delete | `Trash2` | |
| Submit | `Send` | |
| Review/Approve | `CheckCircle` | |
| Reject/Return | `CornerUpLeft` | |
| SICR trigger | `Zap` | |
| ECL frozen | `Lock` | |
| PII masked | `EyeOff` | |
| PII reveal | `Eye` | |
| System currency | `Shield` | badge IDR |
| Workflow step done | `Check` dalam circle | |
| Workflow step active | `Circle` (filled) | |
| Workflow step pending | `Circle` (empty) | |
| Returned | `CornerDownLeft` | ↩ |

---

## Accessibility Rules (WCAG 2.1 AA)

Minimum yang harus dipenuhi:

1. **Contrast 4.5:1** untuk semua teks normal — semua token di atas sudah di-verify
2. **Focus ring**: menggunakan shadcn default `ring-2 ring-offset-2` — tidak di-remove
3. **Status badge**: selalu ada ikon + teks, tidak hanya warna
4. **Form error**: merah + ikon ⚠ + teks, tidak hanya border merah
5. **Interactive element min size**: 44x44px untuk mobile, 32px minimum untuk desktop
6. **Disabled button**: `opacity-50 cursor-not-allowed` + tooltip penjelasan kenapa disabled
7. **Loading state**: skeleton + `aria-busy="true"` pada container
8. **Toast**: `role="status"` (sukses/info) atau `role="alert"` (error) untuk screen reader
