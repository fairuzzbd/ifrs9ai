/**
 * Zod schemas — P5-M7 Renewal Deposito
 * Derived from api/openapi/app-b-renewal-deposito.yaml
 * DEC-016: NUMERIC(20,4) IDR, NUMERIC(10,8) EIR — stored as string from API
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const renewalSkemaEnum = z.enum(["POKOK_SAJA", "POKOK_PLUS_BUNGA"]);
export type RenewalSkema = z.infer<typeof renewalSkemaEnum>;

export const renewalStatusEnum = z.enum([
  "PENDING_APPROVAL",
  "APPROVED",
  "POSTED",
  "REJECTED",
]);
export type RenewalStatus = z.infer<typeof renewalStatusEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const RENEWAL_STATUS_LABELS: Record<RenewalStatus, string> = {
  PENDING_APPROVAL: "Menunggu Approval",
  APPROVED: "Disetujui",
  POSTED: "Diposting",
  REJECTED: "Ditolak",
};

export const RENEWAL_SKEMA_LABELS: Record<RenewalSkema, string> = {
  POKOK_SAJA: "Pokok Saja",
  POKOK_PLUS_BUNGA: "Pokok + Bunga",
};

// ---------------------------------------------------------------------------
// Preview (server-computed)
// ---------------------------------------------------------------------------

export const renewalPreviewSchema = z.object({
  pokokLama: z.string(),
  bungaKotor: z.string(),
  pph20pct: z.string(),
  bungaBersih: z.string(),
  pokokBaru: z.string(),
  eirBaru: z.string(),
  tanggalJatuhTempoBaru: z.string(),
  scheduleBaruPreview: z
    .array(
      z.object({
        bulan: z.number(),
        bungaKotorBulan: z.string(),
        pphBulan: z.string(),
        bungaBersihBulan: z.string(),
        saldoPokokAkhir: z.string(),
      }),
    )
    .optional(),
});
export type RenewalPreview = z.infer<typeof renewalPreviewSchema>;

// ---------------------------------------------------------------------------
// Create form input (S1)
// Validates tenor 1–60, rate 0–30 per state machine validation rules
// ---------------------------------------------------------------------------

export const createRenewalSchema = z.object({
  instrumenId: z.string().min(1, { message: "Pilih instrumen deposito yang valid." }),
  skema: renewalSkemaEnum,
  tenorBaruBulan: z
    .number()
    .int("Tenor harus bilangan bulat.")
    .min(1, "Tenor minimal 1 bulan.")
    .max(60, "Tenor maksimal 60 bulan."),
  rateBaruPersen: z
    .number()
    .min(0, "Rate minimal 0%.")
    .max(30, "Rate maksimal 30%."),
  tanggalEfektifBaru: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, { message: "Format tanggal YYYY-MM-DD." }),
});
export type CreateRenewalInput = z.infer<typeof createRenewalSchema>;

// ---------------------------------------------------------------------------
// Approve form (S2)
// ---------------------------------------------------------------------------

export const approveRenewalSchema = z.object({
  comment: z.string().min(1, { message: "Komentar wajib diisi." }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type ApproveRenewalInput = z.infer<typeof approveRenewalSchema>;

// ---------------------------------------------------------------------------
// Reject form (S2 — comment ≥ 30 char)
// ---------------------------------------------------------------------------

export const rejectRenewalSchema = z.object({
  comment: z
    .string()
    .min(30, { message: "Alasan penolakan minimal 30 karakter." }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});
export type RejectRenewalInput = z.infer<typeof rejectRenewalSchema>;

// ---------------------------------------------------------------------------
// API response shapes
// ---------------------------------------------------------------------------

export const createRenewalResponseSchema = z.object({
  renewalId: z.string().uuid(),
  status: z.literal("PENDING_APPROVAL"),
  preview: renewalPreviewSchema,
  nextStep: z.string().optional(),
});
export type CreateRenewalResponse = z.infer<typeof createRenewalResponseSchema>;

export const approveRenewalResponseSchema = z.object({
  renewalId: z.string().uuid(),
  status: z.literal("POSTED"),
  instrumenBaruId: z.string().uuid().optional(),
  jurnalEntryId: z.string().uuid().optional(),
  approvedBy: z.string().uuid(),
  approvedAt: z.string(),
  message: z.string().optional(),
});
export type ApproveRenewalResponse = z.infer<typeof approveRenewalResponseSchema>;

export const rejectRenewalResponseSchema = z.object({
  renewalId: z.string().uuid(),
  status: z.literal("REJECTED"),
  rejectedBy: z.string().uuid(),
  rejectedAt: z.string(),
  comment: z.string(),
});
export type RejectRenewalResponse = z.infer<typeof rejectRenewalResponseSchema>;

export const renewalListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenLamaId: z.string().uuid(),
  instrumenLamaKode: z.string(),
  instrumenBaruId: z.string().uuid().nullable().optional(),
  skema: renewalSkemaEnum,
  tenorBaruBulan: z.number(),
  rateBaruPersen: z.string(),
  tanggalEfektifBaru: z.string(),
  pokokLama: z.string(),
  pokokBaru: z.string(),
  bungaBersih: z.string(),
  status: renewalStatusEnum,
  makerId: z.string().uuid(),
  approverId: z.string().uuid().nullable().optional(),
  jurnalEntryId: z.string().uuid().nullable().optional(),
  createdAt: z.string(),
});
export type RenewalListItem = z.infer<typeof renewalListItemSchema>;

export const renewalDetailSchema = renewalListItemSchema.extend({
  bungaKotor: z.string().optional(),
  pph20pct: z.string().optional(),
  eirBaru: z.string().optional(),
  tanggalJatuhTempoBaru: z.string().optional(),
  approveReason: z.string().nullable().optional(),
  rejectReason: z.string().nullable().optional(),
  signatureMethod: z.string().nullable().optional(),
  periodeBulananId: z.string().uuid().nullable().optional(),
  updatedAt: z.string().optional(),
  rowVersion: z.number().optional(),
  preview: renewalPreviewSchema.optional(),
});
export type RenewalDetail = z.infer<typeof renewalDetailSchema>;

// ---------------------------------------------------------------------------
// Error codes — 6 new codes from OpenAPI
// ---------------------------------------------------------------------------

export const renewalErrorCodes = [
  "RENEWAL_INSTRUMEN_NOT_ELIGIBLE",
  "RENEWAL_SKEMA_INVALID",
  "RENEWAL_TENOR_OUT_OF_RANGE",
  "RENEWAL_RATE_OUT_OF_RANGE",
  "RENEWAL_BUNGA_BERSIH_TOO_SMALL",
  "RENEWAL_PPH_CALC_MISMATCH",
] as const;
export type RenewalErrorCode = (typeof renewalErrorCodes)[number];
