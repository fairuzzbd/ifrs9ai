/**
 * Zod schemas — P5-M8 Penjualan/Pencairan Instrumen
 * Derived from api/openapi/app-b-penjualan.yaml
 * DEC-016: NUMERIC(20,4) IDR, NUMERIC(10,8) EIR — stored as string from API
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const penjualanStatusEnum = z.enum([
  "PENDING_APPROVAL",
  "APPROVED",
  "POSTED",
  "REJECTED",
  "PENDING_BM_REVIEW",
]);
export type PenjualanStatus = z.infer<typeof penjualanStatusEnum>;

export const jenisDisposalEnum = z.enum(["PARTIAL", "FULL"]);
export type JenisDisposal = z.infer<typeof jenisDisposalEnum>;

export const klasifikasiPsak71Enum = z.enum([
  "AC",
  "FVOCI",
  "FVOCI_ELECTION",
  "FVTPL",
  "POCI",
]);
export type KlasifikasiPsak71 = z.infer<typeof klasifikasiPsak71Enum>;

export const bmViolationFlagEnum = z.enum([
  "BM_VIOLATION_RISK",
  "BM_VIOLATION_BLOCK",
]);
export type BMViolationFlag = z.infer<typeof bmViolationFlagEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const PENJUALAN_STATUS_LABELS: Record<PenjualanStatus, string> = {
  PENDING_APPROVAL: "Menunggu Approval",
  APPROVED: "Disetujui",
  POSTED: "Diposting",
  REJECTED: "Ditolak",
  PENDING_BM_REVIEW: "Menunggu Review BM",
};

export const JENIS_DISPOSAL_LABELS: Record<JenisDisposal, string> = {
  PARTIAL: "Sebagian",
  FULL: "Penuh",
};

export const KLASIFIKASI_LABELS: Record<KlasifikasiPsak71, string> = {
  AC: "Biaya Perolehan Diamortisasi (AC)",
  FVOCI: "Nilai Wajar melalui OCI (FVOCI Debt)",
  FVOCI_ELECTION: "Nilai Wajar melalui OCI — Opsi Ekuitas",
  FVTPL: "Nilai Wajar melalui L/R (FVTPL)",
  POCI: "Dibeli/Diterbitkan Kredit Impaired (POCI)",
};

// ---------------------------------------------------------------------------
// Preview (server-computed)
// ---------------------------------------------------------------------------

export const penjualanPreviewSchema = z.object({
  klasifikasiPsak71: klasifikasiPsak71Enum,
  proceedIdr: z.string(),
  costBasis: z.string(),
  realizedGl: z.string(),
  ociRecycled: z.string().nullable().optional(),
  noRecyclingNote: z.string().nullable().optional(),
  bmFreqImpactPct: z.string().nullable().optional(),
  bmFreqWarning: z.string().nullable().optional(),
});
export type PenjualanPreview = z.infer<typeof penjualanPreviewSchema>;

// ---------------------------------------------------------------------------
// Create form input (S1)
// ---------------------------------------------------------------------------

export const createPenjualanSchema = z.object({
  instrumenId: z.string().uuid({ message: "Pilih instrumen yang valid (UUID)." }),
  jenisDisposal: jenisDisposalEnum,
  qtyTerjual: z
    .string()
    .regex(/^\d+(\.\d+)?$/, { message: "Qty harus angka positif." })
    .refine((v) => parseFloat(v) > 0, { message: "Qty terjual harus lebih dari 0." }),
  hargaJualPerUnit: z
    .string()
    .regex(/^\d+(\.\d+)?$/, { message: "Harga jual harus angka positif." })
    .refine((v) => parseFloat(v) > 0, { message: "Harga jual per unit harus lebih dari 0." }),
  tanggalEksekusi: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, { message: "Format tanggal YYYY-MM-DD." }),
});
export type CreatePenjualanInput = z.infer<typeof createPenjualanSchema>;

// ---------------------------------------------------------------------------
// Approve form (S2)
// ---------------------------------------------------------------------------

export const approvePenjualanSchema = z.object({
  comment: z.string().min(1, { message: "Komentar wajib diisi." }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type ApprovePenjualanInput = z.infer<typeof approvePenjualanSchema>;

// ---------------------------------------------------------------------------
// Reject form (S2 — reason ≥ 30 char)
// ---------------------------------------------------------------------------

export const rejectPenjualanSchema = z.object({
  reason: z
    .string()
    .min(30, { message: "Alasan penolakan minimal 30 karakter." }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type RejectPenjualanInput = z.infer<typeof rejectPenjualanSchema>;

// ---------------------------------------------------------------------------
// API response shapes
// ---------------------------------------------------------------------------

export const createPenjualanResponseSchema = z.object({
  penjualanId: z.string().uuid(),
  status: z.literal("PENDING_APPROVAL"),
  preview: penjualanPreviewSchema,
  nextStep: z.string().optional(),
});
export type CreatePenjualanResponse = z.infer<typeof createPenjualanResponseSchema>;

export const approvePenjualanResponseSchema = z.object({
  penjualanId: z.string().uuid(),
  status: z.enum(["POSTED", "PENDING_BM_REVIEW"]),
  jurnalEntryId: z.string().uuid().nullable().optional(),
  instrumenStatusAfter: z.string().nullable().optional(),
  approvedBy: z.string().uuid(),
  approvedAt: z.string(),
  ociRecycled: z.string().nullable().optional(),
  noRecyclingNote: z.string().nullable().optional(),
  bmViolationRisk: z.boolean().optional(),
  warnings: z.array(z.string()).optional(),
});
export type ApprovePenjualanResponse = z.infer<typeof approvePenjualanResponseSchema>;

export const rejectPenjualanResponseSchema = z.object({
  penjualanId: z.string().uuid(),
  status: z.literal("REJECTED"),
  rejectedBy: z.string().uuid(),
  rejectedAt: z.string(),
  reason: z.string(),
});
export type RejectPenjualanResponse = z.infer<typeof rejectPenjualanResponseSchema>;

export const penjualanListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  jenisDisposal: jenisDisposalEnum,
  qtyTerjual: z.string(),
  qtyHoldingPre: z.string(),
  qtyHoldingPost: z.string().nullable().optional(),
  proceedIdr: z.string(),
  realizedGl: z.string().nullable().optional(),
  klasifikasiSnapshot: z.string(),
  status: penjualanStatusEnum,
  tanggalEksekusi: z.string(),
  makerId: z.string().uuid(),
  approverId: z.string().uuid().nullable().optional(),
  jurnalHeaderId: z.string().uuid().nullable().optional(),
  bmViolationRisk: z.boolean(),
  createdAt: z.string(),
});
export type PenjualanListItem = z.infer<typeof penjualanListItemSchema>;

export const penjualanDetailSchema = penjualanListItemSchema.extend({
  costBasis: z.string().optional(),
  ociRecycled: z.string().nullable().optional(),
  ociCumulativeTotal: z.string().nullable().optional(),
  noRecyclingNote: z.string().nullable().optional(),
  jurnalEventCode: z.string().nullable().optional(),
  approveComment: z.string().nullable().optional(),
  rejectReason: z.string().nullable().optional(),
  signatureMethod: z.string().nullable().optional(),
  periodeBulananId: z.string().uuid().nullable().optional(),
  bmViolationPct: z.string().nullable().optional(),
  updatedAt: z.string().optional(),
  rowVersion: z.number().optional(),
  preview: penjualanPreviewSchema.optional(),
});
export type PenjualanDetail = z.infer<typeof penjualanDetailSchema>;

export const bmAlertItemSchema = z.object({
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  portofolioId: z.string().uuid(),
  portofolioNama: z.string(),
  cumulativeSold12mPct: z.string(),
  warnThresholdPct: z.string(),
  blockThresholdPct: z.string(),
  flagStatus: bmViolationFlagEnum,
  lastUpdated: z.string(),
});
export type BMAlertItem = z.infer<typeof bmAlertItemSchema>;

// ---------------------------------------------------------------------------
// Error codes — 7 new codes from OpenAPI (P5-M8)
// ---------------------------------------------------------------------------

export const penjualanErrorCodes = [
  "PENJUALAN_INSTRUMEN_NOT_ACTIVE",
  "PENJUALAN_QTY_EXCEEDS_HOLDING",
  "PENJUALAN_KLASIFIKASI_NOT_LOCKED",
  "PENJUALAN_HARGA_INVALID",
  "PENJUALAN_PERIODE_LOCKED",
  "PENJUALAN_BM_VIOLATION_BLOCK",
  "PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN",
] as const;
export type PenjualanErrorCode = (typeof penjualanErrorCodes)[number];
