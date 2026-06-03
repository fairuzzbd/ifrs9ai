# Handoff Checklist — frontend-engineer-nextjs
## Master Data Phase 3 (APP-A) UX Implementation

**Handoff dari**: uiux-designer  
**Ditujukan ke**: frontend-engineer-nextjs  
**Via**: tech-lead-orchestrator dispatch  
**Tanggal**: 2026-06-03  
**Story refs**: APP-A-MSTR-001, APP-A-MSTR-002  
**Wireframes**: docs/ux/master-data/wireframe-MSTR-001-list.md, MSTR-002-form.md, MSTR-003-detail-workflow.md, MSTR-004-variants.md  
**Interactions**: docs/ux/master-data/interaction-FLOW-001-master-crud.md, FLOW-002-mata-uang-konkret.md  
**Tokens**: docs/ux/master-data/tokens.md

---

## Pre-requisite

- [ ] PR #10 (Phase 2 Foundation) sudah merge ke `develop`. JANGAN mulai implementasi sebelum ini.
- [ ] Migration 000008 (data-modeler) sudah jalan: kolom `decimal_places`, `is_system_currency`, `workflow_status`, `row_version`, dll sudah ada di `mst.mata_uang`.
- [ ] Backend endpoint `/api/v1/master/mata-uang` + workflow endpoints sudah tersedia (atau mock via MSW).
- [ ] OpenAPI contract `api/openapi/mata-uang.yaml` sudah final dari system-analyst.

---

## Komponen yang Sudah Ada (Phase 2 — reuse saja)

| Komponen | Path | Keterangan |
|---|---|---|
| `DataTable` | `components/blips/DataTable.tsx` | Sort + paging + filter + export (UX §1) |
| `JobProgressPanel` | `components/blips/JobProgressPanel.tsx` | SSE + polling progress (UX §3) |
| `notify` | `lib/notify.ts` | Toast sukses/gagal spesifik (UX §2) |

---

## Komponen Baru yang Harus Dibuat (urutan prioritas)

### PRIORITAS 1 — Diperlukan untuk pilot mata_uang

| Komponen | File | Spec |
|---|---|---|
| `WorkflowStatusBadge` | `components/blips/WorkflowStatusBadge.tsx` | Badge status workflow dengan ikon + text + warna. Props: `status: MasterWorkflowState`, `size?: "sm" \| "default"`. Semua 6 state (DRAFT/PENDING_REVIEW/PENDING_APPROVAL/PENDING_APPROVAL_2/APPROVED/RETURNED). Warna dari tokens.md. Tidak hanya warna — ikon wajib. |
| `MakerReviewerApproverPanel` | `components/blips/MakerReviewerApproverPanel.tsx` | Stepper vertikal workflow 4-eyes. Props: `workflowData: WorkflowStatus`, `currentUserId: string`, `onSubmit?: () => void`, `onReview?: (data) => void`, `onApprove?: (data) => void`, `onReject?: (data) => void`. Step selesai: collapsed + summary. Step aktif: expanded + action area. Deteksi SoD otomatis dari currentUserId vs maker/reviewer ID. |
| `ApprovalWithSignature` | `components/blips/ApprovalWithSignature.tsx` | Panel aksi di dalam stepper step aktif. Props: `actionType: "review" \| "approve"`, `onApprove: (comment, attestChecked) => void`, `onReject: () => void`, `sodBlocked: boolean`, `sodMessage?: string`, `requireMfa?: boolean`, `mfaVerified?: boolean`. Komentar textarea + attest checkbox + tombol. Reject sub-panel inline (bukan modal). |
| `ReturnedBanner` | `components/blips/ReturnedBanner.tsx` | Banner kuning saat status RETURNED. Props: `rejectedBy: string`, `rejectedAt: string`, `comment: string`. Tampil di atas form edit. |
| `SodBlockBanner` | `components/blips/SodBlockBanner.tsx` | Banner info saat user adalah maker. Props: `message: string`. Tampil di dalam step aktif menggantikan action area. |
| `FilterBar` | `components/blips/FilterBar.tsx` | Komponen filter bar dengan search + filter chips + clear all. Sync state ke URL (pakai `nuqs` atau Next.js searchParams). Props: `filters: FilterConfig[]`, `onFilterChange: (filters) => void`. |
| `ExportButton` | `components/blips/ExportButton.tsx` | Dropdown Export (CSV/XLSX). Handle sync vs async logic. Sync: trigger download langsung. Async: tampilkan JobProgressPanel. Props: `endpoint: string`, `filename: (filters) => string`, `formats: ("csv" \| "xlsx")[]`, `currentFilters: Record<string, any>`. |

### PRIORITAS 2 — Diperlukan untuk varian

| Komponen | File | Spec |
|---|---|---|
| `AuditHistoryTable` | `components/blips/AuditHistoryTable.tsx` | Tabel riwayat audit trail. Props: `entityType: string`, `entityId: string`. Kolom: Waktu, Aksi, Dilakukan Oleh, Komentar. before/after jsonb hanya untuk ROLE-AUDIT. Pakai DataTable base. |
| `MfaStepUpPrompt` | `components/blips/MfaStepUpPrompt.tsx` | Prompt OTP inline. Muncul di dalam ApprovalWithSignature. Props: `onVerified: (stepUpToken: string) => void`, `onCancel: () => void`. |
| `EclParamBanner` | `components/blips/EclParamBanner.tsx` | Banner peringatan parameter ECL. Tidak bisa di-dismiss. Fixed text. |
| `EclParamFrozenBanner` | `components/blips/EclParamFrozenBanner.tsx` | Banner ECL_PARAM_FROZEN. Props: `calcRunId: string`, `periode: string`. |
| `SixEyesWorkflowPanel` | Extend `MakerReviewerApproverPanel` | 4 step: Maker, Reviewer, Approver 1 (ALCO), Approver 2 (ALCO). Step 3 dan 4 memerlukan MFA step-up. |
| `PiiMaskedField` | `components/blips/PiiMaskedField.tsx` | Display field dengan masking. Props: `value: string`, `maskPattern: "npwp" \| "rekening" \| "ktp"`, `revealed: boolean`. |
| `PiiRevealDialog` | `components/blips/PiiRevealDialog.tsx` | Dialog konfirmasi sebelum reveal PII. Tulis audit log setelah confirm. |
| `SicrTriggerBanner` | `components/blips/SicrTriggerBanner.tsx` | Banner SICR triggered. Props: `ratingBefore: string`, `ratingAfter: string`, `triggerDate: string`, `instrumentCount: number`. |
| `FileUploadDropzone` | `components/blips/FileUploadDropzone.tsx` | Drag & drop file upload. Props: `accept: string[]`, `maxSizeMB: number`, `onFileSelect: (file: File) => void`. |
| `ImportDiffTable` | `components/blips/ImportDiffTable.tsx` | Tabel diff import (lama vs baru). Highlight delta besar (>10%). Filter: semua/baru/diperbarui/signifikan. |
| `FeedStatusPanel` | `components/blips/FeedStatusPanel.tsx` | Status scheduled feed job. Props: `lastRunAt: string`, `lastRunStatus: "success" \| "failed"`, `nextRunAt: string`. |
| `ComplianceGateBadge` | `components/blips/ComplianceGateBadge.tsx` | Badge compliance gate status. Props: `status: "pending" \| "verified" \| "rejected"`. |

---

## Pages yang Harus Dibuat (mata_uang pilot)

| Route | File | Keterangan |
|---|---|---|
| `/master/mata-uang` | `app/master/mata-uang/page.tsx` | List dengan DataTable |
| `/master/mata-uang/new` | `app/master/mata-uang/new/page.tsx` | Form create |
| `/master/mata-uang/[kode]` | `app/master/mata-uang/[kode]/page.tsx` | Detail + workflow panel |
| `/master/mata-uang/[kode]/edit` | `app/master/mata-uang/[kode]/edit/page.tsx` | Form edit (guard: DRAFT/RETURNED saja) |
| `/master/mata-uang/[kode]/history` | `app/master/mata-uang/[kode]/history/page.tsx` | Audit history table |

---

## Validasi Rules (Zod Schema — mata_uang)

```typescript
const mataUangCreateSchema = z.object({
  kodeMataUang: z
    .string()
    .regex(/^[A-Z]{3}$/, "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)"),
  namaMataUang: z
    .string()
    .min(3, "Nama mata uang minimal 3 karakter")
    .max(60, "Nama mata uang maksimal 60 karakter"),
  simbol: z
    .string()
    .min(1, "Simbol wajib diisi")
    .max(5, "Simbol maksimal 5 karakter"),
  decimalPlaces: z
    .number()
    .int()
    .min(0, "Decimal places minimal 0")
    .max(4, "Decimal places maksimal 4"),
  sumberKursDefault: z.enum(["BI_JISDOR", "BI_KURS_TENGAH", "INTERNAL"], {
    errorMap: () => ({ message: "Pilih sumber kurs yang valid" }),
  }),
  frekuensiUpdate: z.enum(["HARIAN", "INTRA_DAY", "BULANAN"], {
    errorMap: () => ({ message: "Pilih frekuensi update yang valid" }),
  }),
  tanggalMulaiAktif: z
    .string()
    .date()
    .refine((val) => new Date(val) <= new Date(), {
      message: "Tanggal mulai aktif tidak boleh di masa depan",
    }),
  aktifFlag: z.boolean().default(true),
});

const mataUangUpdateSchema = mataUangCreateSchema
  .omit({ kodeMataUang: true })  // kode immutable
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

const workflowApproveSchema = z.object({
  comment: z.string().max(1000).optional(),
  signatureMethod: z.enum(["JWT_STANDARD", "JWT_STEP_UP"]).default("JWT_STANDARD"),
  rowVersion: z.number().int().positive(),
});

const workflowRejectSchema = z.object({
  comment: z.string().min(10, "Alasan penolakan minimal 10 karakter").max(1000),
  signatureMethod: z.enum(["JWT_STANDARD", "JWT_STEP_UP"]).default("JWT_STANDARD"),
  rowVersion: z.number().int().positive(),
});
```

---

## Pola URL State (Filter/Sort)

Gunakan `nuqs` library untuk sync filter/sort ke URL:

```typescript
// app/master/mata-uang/page.tsx
const [q, setQ] = useQueryState("q");
const [sort, setSort] = useQueryState("sort", { defaultValue: "kode_mata_uang:asc" });
const [filterAktif, setFilterAktif] = useQueryState("filter[aktif_flag]");
const [filterStatus, setFilterStatus] = useQueryState("filter[workflow_status]");
const [filterSumber, setFilterSumber] = useQueryState("filter[sumber_kurs_default]");
const [cursor, setCursor] = useQueryState("cursor");
```

URL harus selalu mencerminkan state tabel. Deep-link harus bekerja (paste URL → state ter-restore).

---

## Permission Checks (Client-side)

```typescript
// hooks/usePermissions.ts — helper berdasarkan JWT claims
function useCanCreate(entity: string): boolean
function useCanUpdate(entity: string): boolean
function useCanReview(entity: string): boolean
function useCanApprove(entity: string): boolean
function useCanDelete(entity: string): boolean
function useIsAuditRole(): boolean
```

Client-side check hanya untuk UX (sembunyikan/tampilkan tombol). Server tetap enforce. Jangan trust client claim untuk aksi kritis.

---

## SoD Check (Client-side hint)

```typescript
// Detect apakah user adalah maker — untuk disable workflow action tombol
function isSoDViolation(
  currentUserId: string,
  workflowData: WorkflowStatus,
  action: "review" | "approve"
): boolean {
  if (action === "review") {
    return currentUserId === workflowData.makerId;
  }
  if (action === "approve") {
    return (
      currentUserId === workflowData.makerId ||
      currentUserId === workflowData.reviewerId
    );
  }
  return false;
}
```

---

## Anti-patterns yang Harus Dihindari

1. Modal stacking — reject sub-panel HARUS inline, bukan modal di atas modal
2. Auto-save — tidak ada. User harus klik "Simpan" secara eksplisit
3. Toast auto-dismiss untuk error — error toast HARUS persistent (manual close)
4. Optimistic update untuk workflow action — tunggu server response dulu
5. Toast sebagai satu-satunya konfirmasi untuk delete/reject — wajib confirm dialog dulu
6. Hiding workflow state di tab tersembunyi — workflow panel selalu visible di detail page (kolom kanan)
7. Export tanpa respect filter aktif — selalu kirim filter aktif ke export endpoint
8. `console.log` yang berisi JWT / token / data sensitif — dilarang

---

## Testing Expectation (untuk qa-engineer)

Frontend engineer wajib pastikan:
- [ ] SoD: Maker tidak bisa klik tombol review/approve (disabled + tooltip)
- [ ] MFA: AKUN-CTL tanpa mfa_verified tidak bisa approve (banner informasi)
- [ ] Double-submit: tombol disabled saat submitting, tidak bisa klik ganda
- [ ] Conflict: 409 response menampilkan pesan spesifik + lock form
- [ ] AUDIT: tombol Create/Edit/Delete tidak muncul untuk ROLE-AUDIT
- [ ] Empty state: benar tampil saat data kosong (bukan blank screen)
- [ ] Loading state: skeleton rows tampil, bukan blank screen
- [ ] Filter: URL state di-update setiap perubahan filter
- [ ] Export: file nama sesuai format `mata-uang-{YYYYMMDD}.{ext}`
- [ ] WCAG: semua komponen baru harus keyboard navigable (Tab order logical)
- [ ] WCAG: error messages punya `aria-describedby` linking ke field terkait

---

## Dispatch Note

Dokumen ini adalah handoff dari uiux-designer. Frontend-engineer-nextjs HARUS:
1. Membaca semua wireframe dan interaction docs di `docs/ux/master-data/`
2. Membaca OpenAPI contract `api/openapi/mata-uang.yaml` untuk response shapes
3. Mulai dari komponen Prioritas 1 (WorkflowStatusBadge, MakerReviewerApproverPanel, ApprovalWithSignature) sebelum pages
4. Koordinasi dengan backend-engineer-go untuk mock API (jika backend belum ready, gunakan MSW)
5. Laporan ke qa-engineer setelah komponen selesai untuk testing

**Orchestrator**: dispatch frontend-engineer-nextjs setelah backend engineer selesai pilot mata_uang endpoint. UX design ini sudah final untuk pilot; varian (ECL param, PII, upload) akan di-dispatch terpisah setelah pilot selesai dan pola generik terbukti.
