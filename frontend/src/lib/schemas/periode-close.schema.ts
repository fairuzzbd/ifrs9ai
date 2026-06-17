import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums — derived from OpenAPI app-d-periode-close.yaml
// ---------------------------------------------------------------------------

export const statusPeriodeEnum = z.enum([
  "OPEN",
  "SOFT_CLOSED",
  "HARD_CLOSE_PENDING",
  "CLOSED",
]);
export type StatusPeriode = z.infer<typeof statusPeriodeEnum>;

export const tipePeriodeEnum = z.enum(["BULANAN", "KUARTALAN", "TAHUNAN"]);
export type TipePeriode = z.infer<typeof tipePeriodeEnum>;

export const checklistTransitionEnum = z.enum([
  "SOFT_CLOSE_REQUEST",
  "SOFT_CLOSE_APPROVE",
  "HARD_CLOSE_REQUEST",
  "HARD_CLOSE_APPROVE",
  "REOPEN_REQUEST",
  "REOPEN_APPROVE",
]);
export type ChecklistTransition = z.infer<typeof checklistTransitionEnum>;

export const checklistTransitionStatusEnum = z.enum(["APPROVED", "REJECTED"]);
export type ChecklistTransitionStatus = z.infer<typeof checklistTransitionStatusEnum>;

export const checklistItemKeyEnum = z.enum([
  "PENDING_APPROVAL_ZERO",
  "JURNAL_BALANCED",
  "GL_DELIVERED",
  "RECON_PASS",
]);
export type ChecklistItemKey = z.infer<typeof checklistItemKeyEnum>;

export const mvRefreshStatusEnum = z.enum([
  "queued",
  "running",
  "completed",
  "failed",
  "cancelled",
]);
export type MvRefreshStatus = z.infer<typeof mvRefreshStatusEnum>;

// ---------------------------------------------------------------------------
// Error codes — P5-M4 (7 new codes)
// ---------------------------------------------------------------------------

export const periodeCloseErrorCodeEnum = z.enum([
  "CLOSING_CHECKLIST_FAILED",
  "CLOSING_CHECKLIST_STALE",
  "PERIODE_SOFT_CLOSED",
  "MFA_STEP_UP_REQUIRED",
  "MFA_STEP_UP_EXPIRED",
  "PERIODE_GRACE_EXPIRED",
  "SOFT_CLOSE_PENDING_EXISTS",
  // existing codes also used
  "WORKFLOW_INVALID_TRANSITION",
  "CONFLICT",
  "SOD_VIOLATION",
  "VALIDATION_FAILED",
  "FORBIDDEN",
  "NOT_FOUND",
  "PERIODE_CLOSED",
]);
export type PeriodeCloseErrorCode = z.infer<typeof periodeCloseErrorCodeEnum>;

// ---------------------------------------------------------------------------
// Checklist item — shared sub-schema
// ---------------------------------------------------------------------------

export const checklistItemSchema = z.object({
  key: checklistItemKeyEnum,
  label: z.string(),
  passed: z.boolean(),
  detail: z.string().nullable().optional(),
  actionUrl: z.string().nullable().optional(),
});
export type ChecklistItem = z.infer<typeof checklistItemSchema>;

export const checklistResultSchema = z.object({
  evaluatedAt: z.string().datetime({ offset: true }),
  allPassed: z.boolean(),
  items: z.array(checklistItemSchema),
});
export type ChecklistResult = z.infer<typeof checklistResultSchema>;

// ---------------------------------------------------------------------------
// Snapshot sub-schemas
// ---------------------------------------------------------------------------

export const checklistSnapshotSummarySchema = z.object({
  snapshotId: z.string().uuid(),
  transition: checklistTransitionEnum,
  evaluatedAt: z.string().datetime({ offset: true }),
  allPassed: z.boolean(),
});
export type ChecklistSnapshotSummary = z.infer<typeof checklistSnapshotSummarySchema>;

export const checklistSnapshotDetailSchema = z.object({
  id: z.string().uuid(),
  periodeId: z.string().uuid(),
  transition: checklistTransitionEnum,
  transitionStatus: checklistTransitionStatusEnum,
  actorUserId: z.string().uuid(),
  actorRole: z.string(),
  actorName: z.string().optional(),
  checklist: checklistResultSchema,
  allPassed: z.boolean(),
  createdAt: z.string().datetime({ offset: true }),
  isStaleRerun: z.boolean().optional(),
});
export type ChecklistSnapshotDetail = z.infer<typeof checklistSnapshotDetailSchema>;

// ---------------------------------------------------------------------------
// MV Refresh job status sub-schema
// ---------------------------------------------------------------------------

export const mvRefreshInfoSchema = z.object({
  jobId: z.string(),
  status: mvRefreshStatusEnum,
  completedAt: z.string().datetime({ offset: true }).nullable().optional(),
  statusUrl: z.string().optional(),
});
export type MvRefreshInfo = z.infer<typeof mvRefreshInfoSchema>;

// ---------------------------------------------------------------------------
// S1 — Soft-close request
// ---------------------------------------------------------------------------

export const softCloseRequestBodySchema = z.object({
  catatan: z
    .string()
    .max(1000, "Catatan maksimal 1000 karakter")
    .optional(),
  rowVersion: z
    .number()
    .int()
    .min(1, "rowVersion harus ≥ 1"),
});
export type SoftCloseRequestBody = z.infer<typeof softCloseRequestBodySchema>;

export const softCloseRequestResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  transition: z.literal("SOFT_CLOSE_REQUEST"),
  checklistSnapshotId: z.string().uuid(),
  checklist: checklistResultSchema,
  allPassed: z.boolean(),
  nextStep: z.string(),
});
export type SoftCloseRequestResponse = z.infer<typeof softCloseRequestResponseSchema>;

// ---------------------------------------------------------------------------
// S2 — Soft-close approve
// ---------------------------------------------------------------------------

export const workflowApproveBodySchema = z.object({
  comment: z
    .string()
    .min(1, "Komentar wajib diisi")
    .max(2000, "Komentar maksimal 2000 karakter"),
  signatureMethod: z.enum(["JWT_STEP_UP", "JWT_STANDARD"]),
});
export type WorkflowApproveBody = z.infer<typeof workflowApproveBodySchema>;

export const softCloseApproveResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  statusPeriode: z.literal("SOFT_CLOSED"),
  tanggalSoftClose: z.string().datetime({ offset: true }),
  approvedBy: z.string().uuid(),
  checklistSnapshotId: z.string().uuid(),
  message: z.string(),
});
export type SoftCloseApproveResponse = z.infer<typeof softCloseApproveResponseSchema>;

// ---------------------------------------------------------------------------
// S3 — Hard-close request
// ---------------------------------------------------------------------------

export const hardCloseRequestBodySchema = z.object({
  catatan: z
    .string()
    .max(1000, "Catatan maksimal 1000 karakter")
    .optional(),
  rowVersion: z
    .number()
    .int()
    .min(1, "rowVersion harus ≥ 1"),
});
export type HardCloseRequestBody = z.infer<typeof hardCloseRequestBodySchema>;

export const hardCloseRequestResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  transition: z.literal("HARD_CLOSE_REQUEST"),
  statusPeriode: z.literal("HARD_CLOSE_PENDING"),
  checklistSnapshotId: z.string().uuid(),
  checklist: checklistResultSchema,
  nextStep: z.string(),
});
export type HardCloseRequestResponse = z.infer<typeof hardCloseRequestResponseSchema>;

// ---------------------------------------------------------------------------
// S3 — Hard-close approve
// ---------------------------------------------------------------------------

export const hardCloseApproveResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  statusPeriode: z.literal("CLOSED"),
  tanggalHardClose: z.string().datetime({ offset: true }),
  graceExpiresAt: z.string().datetime({ offset: true }),
  approvedBy: z.string().uuid(),
  checklistSnapshotId: z.string().uuid(),
  mvRefreshJobId: z.string().optional(),
  mvRefreshStatusUrl: z.string().optional(),
  message: z.string(),
});
export type HardCloseApproveResponse = z.infer<typeof hardCloseApproveResponseSchema>;

// ---------------------------------------------------------------------------
// S3 — Hard-close reject (OQ-M4-3a)
// ---------------------------------------------------------------------------

export const rejectBodySchema = z.object({
  reason: z
    .string()
    .min(30, "Alasan penolakan wajib minimal 30 karakter")
    .max(1000, "Alasan maksimal 1000 karakter"),
});
export type RejectBody = z.infer<typeof rejectBodySchema>;

export const periodeStateTransitionResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  previousStatus: statusPeriodeEnum,
  newStatus: statusPeriodeEnum,
  transition: z.string(),
  reason: z.string().optional(),
  actorId: z.string().uuid(),
  transitionedAt: z.string().datetime({ offset: true }),
});
export type PeriodeStateTransitionResponse = z.infer<typeof periodeStateTransitionResponseSchema>;

// ---------------------------------------------------------------------------
// S4 — Reopen request
// ---------------------------------------------------------------------------

export const reopenRequestBodySchema = z.object({
  targetStatus: z.enum(["OPEN", "SOFT_CLOSED"]),
  reason: z
    .string()
    .min(30, "Alasan reopen wajib minimal 30 karakter untuk audit compliance")
    .max(2000, "Alasan maksimal 2000 karakter"),
  rowVersion: z
    .number()
    .int()
    .min(1, "rowVersion harus ≥ 1"),
});
export type ReopenRequestBody = z.infer<typeof reopenRequestBodySchema>;

export const reopenRequestResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  currentStatus: statusPeriodeEnum,
  targetStatus: z.enum(["OPEN", "SOFT_CLOSED"]),
  checklistSnapshotId: z.string().uuid(),
  stepUpMfaRequired: z.boolean(),
  nextStep: z.string(),
});
export type ReopenRequestResponse = z.infer<typeof reopenRequestResponseSchema>;

export const reopenApproveResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  previousStatus: statusPeriodeEnum,
  newStatus: statusPeriodeEnum,
  reopenedAt: z.string().datetime({ offset: true }),
  reopenedBy: z.string().uuid(),
  fxRateUnlocked: z.boolean(),
  message: z.string(),
});
export type ReopenApproveResponse = z.infer<typeof reopenApproveResponseSchema>;

// ---------------------------------------------------------------------------
// S5 — Closing checklist response
// ---------------------------------------------------------------------------

export const closingChecklistResponseSchema = z.object({
  periodeId: z.string().uuid(),
  periodeKode: z.string(),
  statusPeriode: statusPeriodeEnum,
  evaluatedAt: z.string().datetime({ offset: true }),
  allPassed: z.boolean(),
  isRealTimeEval: z.boolean(),
  items: z.array(checklistItemSchema),
  lastSnapshot: checklistSnapshotSummarySchema.nullable().optional(),
  mvRefresh: mvRefreshInfoSchema.nullable().optional(),
});
export type ClosingChecklistResponse = z.infer<typeof closingChecklistResponseSchema>;

// ---------------------------------------------------------------------------
// S5 — Periode buku list item (for DataTable)
// ---------------------------------------------------------------------------

export const periodeBukuListItemSchema = z.object({
  id: z.string().uuid(),
  periodeKode: z.string(),
  tahunBuku: z.number().int(),
  bulan: z.number().int().min(1).max(12),
  tipePeriode: tipePeriodeEnum,
  statusPeriode: statusPeriodeEnum,
  tanggalMulai: z.string(),
  tanggalAkhir: z.string(),
  tanggalSoftClose: z.string().datetime({ offset: true }).nullable().optional(),
  softCloseBy: z.string().nullable().optional(),
  softCloseRequestedBy: z.string().uuid().nullable().optional(),
  tanggalHardClose: z.string().datetime({ offset: true }).nullable().optional(),
  hardCloseBy: z.string().nullable().optional(),
  hardCloseGraceExpiresAt: z.string().datetime({ offset: true }).nullable().optional(),
  reopenedFlag: z.boolean().optional(),
  rowVersion: z.number().int().min(1),
});
export type PeriodeBukuListItem = z.infer<typeof periodeBukuListItemSchema>;

// ---------------------------------------------------------------------------
// S5 — Periode detail (full)
// ---------------------------------------------------------------------------

export const periodeBukuDetailSchema = periodeBukuListItemSchema.extend({
  softCloseRequestedAt: z.string().datetime({ offset: true }).nullable().optional(),
  softCloseApprovedBy: z.string().uuid().nullable().optional(),
  softCloseApprovedAt: z.string().datetime({ offset: true }).nullable().optional(),
  hardCloseRequestedBy: z.string().uuid().nullable().optional(),
  hardCloseRequestedAt: z.string().datetime({ offset: true }).nullable().optional(),
  hardCloseApprovedBy: z.string().uuid().nullable().optional(),
  hardCloseApprovedAt: z.string().datetime({ offset: true }).nullable().optional(),
  reopenReason: z.string().nullable().optional(),
  reopenedAt: z.string().datetime({ offset: true }).nullable().optional(),
  reopenedBy: z.string().uuid().nullable().optional(),
  jumlahJurnal: z.number().int().min(0).optional(),
  fxLocked: z.boolean().optional(),
});
export type PeriodeBukuDetail = z.infer<typeof periodeBukuDetailSchema>;

// ---------------------------------------------------------------------------
// S5 — Status periode report item
// ---------------------------------------------------------------------------

export const statusPeriodeListItemSchema = z.object({
  id: z.string().uuid(),
  periodeKode: z.string(),
  tahunBuku: z.number().int(),
  bulan: z.number().int().min(1).max(12),
  tipePeriode: tipePeriodeEnum,
  statusPeriode: statusPeriodeEnum,
  tanggalSoftClose: z.string().datetime({ offset: true }).nullable().optional(),
  softCloseBy: z.string().nullable().optional(),
  tanggalHardClose: z.string().datetime({ offset: true }).nullable().optional(),
  hardCloseBy: z.string().nullable().optional(),
  hardCloseGraceExpiresAt: z.string().datetime({ offset: true }).nullable().optional(),
  reopenedFlag: z.boolean().optional(),
  mvRefreshStatus: mvRefreshStatusEnum.nullable().optional(),
  checklistLastSnapshot: z.object({
    transition: checklistTransitionEnum,
    allPassed: z.boolean(),
  }).nullable().optional(),
});
export type StatusPeriodeListItem = z.infer<typeof statusPeriodeListItemSchema>;

// ---------------------------------------------------------------------------
// Step-up MFA
// ---------------------------------------------------------------------------

export const mfaStepUpScopeEnum = z.enum(["hard_close", "reopen_closed"]);
export type MfaStepUpScope = z.infer<typeof mfaStepUpScopeEnum>;

export const mfaStepUpResponseSchema = z.object({
  stepUpToken: z.string(),
  expiresAt: z.string().datetime({ offset: true }),
  scope: mfaStepUpScopeEnum,
});
export type MfaStepUpResponse = z.infer<typeof mfaStepUpResponseSchema>;

export const mfaStepUpRequestSchema = z.object({
  totpCode: z
    .string()
    .length(6, "Kode TOTP harus 6 digit")
    .regex(/^\d{6}$/, "Kode TOTP harus berupa angka"),
  scope: mfaStepUpScopeEnum,
});
export type MfaStepUpRequest = z.infer<typeof mfaStepUpRequestSchema>;

// ---------------------------------------------------------------------------
// Workflow step (for MakerReviewerApproverPanel)
// ---------------------------------------------------------------------------

export const workflowStepStatusEnum = z.enum(["done", "pending", "skipped"]);
export type WorkflowStepStatus = z.infer<typeof workflowStepStatusEnum>;

export interface WorkflowStep {
  id: string;
  label: string;
  status: WorkflowStepStatus;
  actor?: {
    name: string;
    role: string;
    userId: string;
  };
  timestamp?: string;
  comment?: string;
  snapshotId?: string;
  checklistSummary?: {
    allPassed: boolean;
    total: number;
    passed: number;
  };
}

// ---------------------------------------------------------------------------
// Human-readable labels (Bahasa Indonesia)
// ---------------------------------------------------------------------------

export const STATUS_PERIODE_LABELS: Record<StatusPeriode, string> = {
  OPEN: "Terbuka",
  SOFT_CLOSED: "Soft-Closed",
  HARD_CLOSE_PENDING: "Menunggu CFO",
  CLOSED: "Ditutup Final",
};

export const CHECKLIST_TRANSITION_LABELS: Record<ChecklistTransition, string> = {
  SOFT_CLOSE_REQUEST: "Soft-Close Request",
  SOFT_CLOSE_APPROVE: "Soft-Close Approved",
  HARD_CLOSE_REQUEST: "Hard-Close Request",
  HARD_CLOSE_APPROVE: "Hard-Close Approved",
  REOPEN_REQUEST: "Reopen Request",
  REOPEN_APPROVE: "Reopen Approved",
};

export const TIPE_PERIODE_LABELS: Record<TipePeriode, string> = {
  BULANAN: "Bulanan",
  KUARTALAN: "Kuartalan",
  TAHUNAN: "Tahunan",
};

export const CHECKLIST_ITEM_LABELS: Record<ChecklistItemKey, string> = {
  PENDING_APPROVAL_ZERO: "0 transaksi/jurnal masih PENDING_APPROVAL",
  JURNAL_BALANCED: "Semua jurnal seimbang (delta ≤ IDR 0.01)",
  GL_DELIVERED: "Tidak ada GL delivery FAILED",
  RECON_PASS: "GL rekonsiliasi terakhir COMPLETED",
};

export const BULAN_LABELS: Record<number, string> = {
  1: "Januari",
  2: "Februari",
  3: "Maret",
  4: "April",
  5: "Mei",
  6: "Juni",
  7: "Juli",
  8: "Agustus",
  9: "September",
  10: "Oktober",
  11: "November",
  12: "Desember",
};

// ---------------------------------------------------------------------------
// Notify error messages — P5-M4 (merged into lib/notify.ts ERROR_MESSAGE_MAP)
// ---------------------------------------------------------------------------

export const PERIODE_CLOSE_ERROR_MESSAGES: Record<string, string> = {
  CLOSING_CHECKLIST_FAILED:
    "Permintaan ditolak: satu atau lebih item closing checklist tidak lolos. Selesaikan item yang gagal terlebih dahulu.",
  CLOSING_CHECKLIST_STALE:
    "Checklist sudah melebihi batas waktu (> 24 jam sejak request). Kondisi mungkin berubah. Periksa checklist terbaru.",
  PERIODE_SOFT_CLOSED:
    "Periode buku sudah soft-closed. Mutasi tidak diizinkan. Hubungi Finance Controller untuk koreksi darurat (CORRECTION_PERIODE_CLOSED).",
  MFA_STEP_UP_REQUIRED:
    "Aksi ini memerlukan step-up MFA. Lakukan verifikasi TOTP terlebih dahulu.",
  MFA_STEP_UP_EXPIRED:
    "Token MFA step-up sudah expired (> 5 menit). Ulangi verifikasi MFA dari awal.",
  PERIODE_GRACE_EXPIRED:
    "Grace window untuk reopen periode sudah berakhir. Reopen tidak bisa dilakukan via API. Ajukan RFC ke Direksi sesuai RACI BRD §3.",
  SOFT_CLOSE_PENDING_EXISTS:
    "Sudah ada soft-close request yang menunggu approval untuk periode ini. Batalkan request tersebut atau tunggu approval.",
};
