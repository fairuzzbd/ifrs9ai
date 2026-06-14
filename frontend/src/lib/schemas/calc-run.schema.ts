/**
 * Zod schemas for APP-C ECL Calc Run (P4-M10).
 *
 * Mirrors OpenAPI app-c-calc-run.yaml.
 * Money: string-based per DEC-016 — no parseFloat/float.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const calcRunStatusEnum = z.enum([
  "DRAFT",
  "IN_PROGRESS",
  "COMPLETED",
  "COMPLETED_WITH_ERRORS",
  "SEAL_REQUESTED",
  "SEALED",
  "SEAL_REJECTED",
  "CANCELLED",
]);
export type CalcRunStatus = z.infer<typeof calcRunStatusEnum>;

// ---------------------------------------------------------------------------
// Parameter Snapshot
// ---------------------------------------------------------------------------

export const parameterSnapshotSchema = z.object({
  bobotSkenario: z
    .object({
      good: z.string(),
      normal: z.string(),
      bad: z.string(),
    })
    .optional(),
  pdPefindo: z.record(z.string(), z.unknown()).optional(),
  lgdBasel: z.record(z.string(), z.unknown()).optional(),
  impactMevPd: z.record(z.string(), z.unknown()).optional(),
  lpsCoverage: z
    .object({
      capIdr: z.string(),
      aktif: z.boolean(),
    })
    .optional(),
  kursJisdor: z.record(z.string(), z.string()).optional(),
  frozenAt: z.string().optional(),
});
export type ParameterSnapshot = z.infer<typeof parameterSnapshotSchema>;

// ---------------------------------------------------------------------------
// Seal Info
// ---------------------------------------------------------------------------

export const sealInfoSchema = z.object({
  sealRequestedBy: z.string().nullable().optional(),
  sealRequestedAt: z.string().nullable().optional(),
  sealRequestedComment: z.string().nullable().optional(),
  sealApprovedBy: z.string().nullable().optional(),
  sealApprovedAt: z.string().nullable().optional(),
  sealApprovedComment: z.string().nullable().optional(),
  sealRejectedBy: z.string().nullable().optional(),
  sealRejectedAt: z.string().nullable().optional(),
  sealRejectedReason: z.string().nullable().optional(),
  sealSignature1: z.string().nullable().optional(),
  sealSignatureMethod: z.string().nullable().optional(),
  sealedAt: z.string().nullable().optional(),
  sealChain: z.record(z.string(), z.unknown()).nullable().optional(),
});
export type SealInfo = z.infer<typeof sealInfoSchema>;

// ---------------------------------------------------------------------------
// Calc Run
// ---------------------------------------------------------------------------

export const calcRunSchema = z.object({
  id: z.string(),
  periodeId: z.string(),
  periodeLabel: z.string().optional(),
  evaluationDate: z.string(),
  status: calcRunStatusEnum,
  totalInstrumen: z.number().int().nullable().optional(),
  processedCount: z.number().int().nullable().optional(),
  errorCount: z.number().int().default(0),
  skippedFvtplCount: z.number().int().default(0),
  createdBy: z.string(),
  createdByUsername: z.string().optional(),
  startedAt: z.string().nullable().optional(),
  completedAt: z.string().nullable().optional(),
  cancelledAt: z.string().nullable().optional(),
  cancelReason: z.string().nullable().optional(),
  jobId: z.string().nullable().optional(),
  parameterSnapshotJsonb: z.record(z.string(), z.unknown()).nullable().optional(),
  sealInfo: sealInfoSchema.optional(),
  rowVersion: z.number().int().optional(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export type CalcRun = z.infer<typeof calcRunSchema>;

// ---------------------------------------------------------------------------
// Forms
// ---------------------------------------------------------------------------

export const createCalcRunSchema = z.object({
  periodeId: z.string().min(1, "Periode wajib dipilih"),
  evaluationDate: z
    .string()
    .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal tidak valid (YYYY-MM-DD)"),
});
export type CreateCalcRunForm = z.infer<typeof createCalcRunSchema>;

export const requestSealSchema = z.object({
  comment: z
    .string()
    .min(20, "Catatan harus minimal 20 karakter")
    .max(2000),
});
export type RequestSealForm = z.infer<typeof requestSealSchema>;

export const approveSealSchema = z.object({
  comment: z
    .string()
    .min(20, "Catatan harus minimal 20 karakter")
    .max(2000),
});
export type ApproveSealForm = z.infer<typeof approveSealSchema>;

export const rejectSealSchema = z.object({
  rejectReason: z
    .string()
    .min(30, "Alasan penolakan harus minimal 30 karakter")
    .max(2000),
});
export type RejectSealForm = z.infer<typeof rejectSealSchema>;

export const cancelCalcRunSchema = z.object({
  cancelReason: z
    .string()
    .min(30, "Alasan pembatalan harus minimal 30 karakter")
    .max(2000),
});
export type CancelCalcRunForm = z.infer<typeof cancelCalcRunSchema>;
