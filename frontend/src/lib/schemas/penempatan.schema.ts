/**
 * Zod schemas for APP-B P5-M1 Penempatan Deposito.
 *
 * Money / rate fields are string-based to preserve NUMERIC(20,4) / NUMERIC(10,8) precision.
 * Conversion to numeric happens in the API client, not here.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const penempatanWorkflowStatusEnum = z.enum([
  "DRAFT",
  "PENDING_REVIEW",
  "PENDING_APPROVAL",
  "APPROVED_ACTIVE",
  "TERMINATION_PENDING_REVIEW",
  "TERMINATION_PENDING_APPROVAL",
  "TERMINATED",
  "MATURED",
  "CANCELLED",
]);
export type PenempatanWorkflowStatus = z.infer<typeof penempatanWorkflowStatusEnum>;

export const klasifikasiPsak71Enum = z.enum([
  "AC",
  "FVOCI",
  "FVTPL",
  "FVOCI_ELECTION",
  "POCI",
]);
export type KlasifikasiPsak71 = z.infer<typeof klasifikasiPsak71Enum>;

export const penempatanErrorCodeEnum = z.enum([
  "VALIDATION_FAILED",
  "UNAUTHORIZED",
  "FORBIDDEN",
  "SOD_VIOLATION",
  "NOT_FOUND",
  "CONFLICT",
  "IDEMPOTENCY_REPLAY",
  "IDEMPOTENCY_MISMATCH",
  "WORKFLOW_INVALID_TRANSITION",
  "PERIODE_CLOSED",
  "RATE_LIMITED",
  "INTERNAL",
  "INVALID_SORT_COL",
  "PENEMPATAN_INSTRUMEN_NOT_FOUND",
  "PENEMPATAN_INSTRUMEN_INVALID_KLASIFIKASI",
  "PENEMPATAN_TANGGAL_PENEMPATAN_INVALID",
  "PENEMPATAN_TENOR_INVALID",
  "PENEMPATAN_KUPON_INVALID",
  "PENEMPATAN_INVALID_TRANSITION",
  "PENEMPATAN_SOD_VIOLATION",
  "PENEMPATAN_STEP_UP_REQUIRED",
  "PENEMPATAN_REASON_TOO_SHORT",
  "PENEMPATAN_EDIT_LOCKED",
  "PENEMPATAN_PERIODE_HARD_CLOSED",
  "PENEMPATAN_TERMINATE_FORBIDDEN_NOT_ACTIVE",
  "ERR_CALC_2010",
]);
export type PenempatanErrorCode = z.infer<typeof penempatanErrorCodeEnum>;

// ---------------------------------------------------------------------------
// Settlement Balance Hint
// ---------------------------------------------------------------------------

export const SettlementBalanceHintSchema = z.object({
  lastKnownIdr: z.number().nullable(),
  asOfDate: z.string().nullable(),
  isStale: z.boolean(),
  isSufficient: z.null(),
});
export type SettlementBalanceHint = z.infer<typeof SettlementBalanceHintSchema>;

// ---------------------------------------------------------------------------
// Amortization schedule item (EIR preview)
// ---------------------------------------------------------------------------

export const AmortizationScheduleItemSchema = z.object({
  periode: z.number().int(),
  tanggalAngsuran: z.string(),
  angsuranBunga: z.number(),
  angsuranPokok: z.number(),
  carryingAmount: z.number(),
});
export type AmortizationScheduleItem = z.infer<typeof AmortizationScheduleItemSchema>;

// ---------------------------------------------------------------------------
// EIR Preview result
// ---------------------------------------------------------------------------

export const EirPreviewResultSchema = z.object({
  eirAwalApprox: z.number().nullable(),
  isApproximate: z.boolean(),
  carryingAmountAwal: z.number().nullable(),
  periodePreview: z.number().int(),
  info: z.string().nullable(),
  amortizationSchedule: z.array(AmortizationScheduleItemSchema),
});
export type EirPreviewResult = z.infer<typeof EirPreviewResultSchema>;

// ---------------------------------------------------------------------------
// Core PenempatanDeposito shape (API response)
// ---------------------------------------------------------------------------

export const PenempatanDepositoSchema = z.object({
  id: z.string().uuid(),
  kodeTransaksi: z.string(),
  workflowStatus: penempatanWorkflowStatusEnum,
  instrumenId: z.string().uuid(),
  instrumenNama: z.string().optional().nullable(),
  instrumenKode: z.string().optional().nullable(),
  klasifikasiPsak71: klasifikasiPsak71Enum.optional().nullable(),
  counterpartyBankId: z.string().uuid(),
  counterpartyBankNama: z.string().optional().nullable(),
  periodeId: z.string().uuid(),
  periodeLabel: z.string().optional().nullable(),
  tanggalPenempatan: z.string(),
  tanggalJatuhTempo: z.string(),
  nominalIdr: z.number(),
  nominalFcy: z.number().nullable(),
  mataUangId: z.string().uuid(),
  mataUangKode: z.string().optional().nullable(),
  kursPenempatan: z.number().nullable(),
  tenorBulan: z.number().int(),
  kuponPersen: z.number(),
  biayaTransaksiIdr: z.number(),
  nomorReferensiBankIn: z.string().nullable(),
  settlementAccount: z.string().nullable(),
  catatan: z.string().nullable(),
  eirAwal: z.number().nullable(),
  carryingAmountAwal: z.number().nullable(),
  makerId: z.string().uuid().nullable(),
  makerNama: z.string().optional().nullable(),
  reviewerId: z.string().uuid().nullable(),
  reviewerNama: z.string().optional().nullable(),
  approverId: z.string().uuid().nullable(),
  approverNama: z.string().optional().nullable(),
  reviewerSignedAt: z.string().nullable(),
  approverSignedAt: z.string().nullable(),
  reviewerSignatureHash: z.string().nullable(),
  approverSignatureHash: z.string().nullable(),
  rejectReason: z.string().nullable(),
  terminateReason: z.string().nullable(),
  terminateReviewerId: z.string().uuid().nullable(),
  terminateApproverId: z.string().uuid().nullable(),
  terminatedAt: z.string().nullable(),
  maturedAt: z.string().nullable(),
  settlementBalanceHint: SettlementBalanceHintSchema.nullable(),
  rowVersion: z.number().int(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type PenempatanDeposito = z.infer<typeof PenempatanDepositoSchema>;

// ---------------------------------------------------------------------------
// List item (lighter shape for DataTable)
// ---------------------------------------------------------------------------

export const PenempatanListItemSchema = z.object({
  id: z.string().uuid(),
  kodeTransaksi: z.string(),
  workflowStatus: penempatanWorkflowStatusEnum,
  counterpartyBankNama: z.string(),
  instrumenNama: z.string().nullable(),
  instrumenKode: z.string().nullable(),
  klasifikasiPsak71: klasifikasiPsak71Enum.nullable(),
  nominalIdr: z.number(),
  tanggalPenempatan: z.string(),
  tanggalJatuhTempo: z.string(),
  kuponPersen: z.number(),
  makerId: z.string().uuid().nullable(),
  createdAt: z.string(),
});
export type PenempatanListItem = z.infer<typeof PenempatanListItemSchema>;

// ---------------------------------------------------------------------------
// Create request
// ---------------------------------------------------------------------------

export const PenempatanCreateSchema = z.object({
  instrumenId: z.string().uuid("Pilih instrumen yang valid"),
  counterpartyBankId: z.string().uuid("Pilih bank counterparty yang valid"),
  periodeId: z.string().uuid("Pilih periode buku"),
  tanggalPenempatan: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal: YYYY-MM-DD"),
  nominalIdr: z.string().optional(),
  nominalFcy: z.string().optional(),
  mataUangId: z.string().uuid("Pilih mata uang"),
  tenorBulan: z
    .number({ message: "Tenor harus bilangan bulat" })
    .int("Tenor harus bilangan bulat")
    .min(1, "Tenor harus lebih dari 0 bulan"),
  kuponPersen: z
    .string()
    .refine((v) => parseFloat(v) >= 0, { message: "Kupon tidak boleh negatif" }),
  biayaTransaksiIdr: z.string().optional(),
  nomorReferensiBankIn: z.string().max(100).optional(),
  settlementAccount: z.string().max(50).optional(),
  catatan: z.string().max(2000).optional(),
  kontrakDocId: z.string().uuid().optional(),
});
export type PenempatanCreateInput = z.infer<typeof PenempatanCreateSchema>;

// ---------------------------------------------------------------------------
// Update request (DRAFT edit)
// ---------------------------------------------------------------------------

export const PenempatanUpdateSchema = z.object({
  rowVersion: z.number().int(),
  tanggalPenempatan: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/)
    .optional(),
  nominalIdr: z.string().optional(),
  nominalFcy: z.string().optional(),
  tenorBulan: z
    .number()
    .int()
    .min(1, "Tenor harus lebih dari 0 bulan")
    .optional(),
  kuponPersen: z
    .string()
    .refine((v) => !v || parseFloat(v) >= 0, { message: "Kupon tidak boleh negatif" })
    .optional(),
  biayaTransaksiIdr: z.string().optional(),
  nomorReferensiBankIn: z.string().max(100).optional(),
  settlementAccount: z.string().max(50).optional(),
  catatan: z.string().max(2000).optional(),
});
export type PenempatanUpdateInput = z.infer<typeof PenempatanUpdateSchema>;

// ---------------------------------------------------------------------------
// Workflow action schemas
// ---------------------------------------------------------------------------

export const WorkflowCommentSchema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi").max(1000),
  signatureMethod: z.literal("JWT_STANDARD"),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini",
  }),
});
export type WorkflowCommentInput = z.infer<typeof WorkflowCommentSchema>;

export const RejectCommentSchema = z.object({
  comment: z
    .string()
    .min(30, "Alasan penolakan minimal 30 karakter")
    .max(1000),
  signatureMethod: z.literal("JWT_STANDARD"),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini",
  }),
});
export type RejectCommentInput = z.infer<typeof RejectCommentSchema>;

export const TerminateRequestSchema = z.object({
  terminateReason: z
    .string()
    .min(30, "Alasan terminasi minimal 30 karakter")
    .max(2000),
  dokumenTerminasiId: z.string().uuid().optional(),
  signatureMethod: z.literal("JWT_STANDARD"),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini",
  }),
});
export type TerminateRequestInput = z.infer<typeof TerminateRequestSchema>;

// ---------------------------------------------------------------------------
// Audit timeline event
// ---------------------------------------------------------------------------

export const AuditTimelineEventSchema = z.object({
  eventId: z.string().uuid(),
  eventTime: z.string(),
  actorUserId: z.string().uuid(),
  actorUsername: z.string(),
  actorRole: z.string(),
  action: z.string(),
  comment: z.string().nullable(),
  signatureHash: z.string().nullable(),
  beforeJsonb: z.record(z.string(), z.unknown()).nullable(),
  afterJsonb: z.record(z.string(), z.unknown()).nullable(),
  ip: z.string().nullable(),
  traceId: z.string().nullable(),
});
export type AuditTimelineEvent = z.infer<typeof AuditTimelineEventSchema>;

// ---------------------------------------------------------------------------
// Workflow transition response
// ---------------------------------------------------------------------------

export const WorkflowTransitionResponseSchema = z.object({
  data: PenempatanDepositoSchema,
  meta: z.object({ traceId: z.string() }),
});
export type WorkflowTransitionResponse = z.infer<typeof WorkflowTransitionResponseSchema>;

// ---------------------------------------------------------------------------
// Approve response (includes optional EIR job id)
// ---------------------------------------------------------------------------

export const ApproveResponseDataSchema = PenempatanDepositoSchema.extend({
  eirComputeJobId: z.string().nullable(),
  stagingAction: z.enum(["STAGE_1_ASSIGNED", "SKIPPED_FVTPL"]).nullable(),
});
export type ApproveResponseData = z.infer<typeof ApproveResponseDataSchema>;

// ---------------------------------------------------------------------------
// List filters
// ---------------------------------------------------------------------------

export interface PenempatanListFilters {
  q?: string;
  "filter[workflow_status]"?: string;
  "filter[counterparty_bank_id]"?: string;
  "filter[tipe_instrumen]"?: string;
  "filter[klasifikasi_psak71]"?: string;
  "filter[tanggal_penempatan]"?: string;
  "filter[periode_id]"?: string;
  "filter[nominal_idr]"?: string;
  sort?: string;
  cursor?: string | null;
  limit?: number;
  include_deleted?: boolean;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

export const FVTPL_TYPES: KlasifikasiPsak71[] = ["FVTPL", "FVOCI_ELECTION"];

export function isFvtpl(klasifikasi: KlasifikasiPsak71 | null | undefined): boolean {
  return !!klasifikasi && FVTPL_TYPES.includes(klasifikasi);
}
