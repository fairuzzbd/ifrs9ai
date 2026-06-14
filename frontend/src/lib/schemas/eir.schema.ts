/**
 * Zod schemas for APP-C EIR Newton-Raphson Solver + Amendment (P4-M9).
 *
 * Mirrors OpenAPI app-c-eir.yaml schemas.
 * Money: string-based per DEC-016 — no parseFloat/float.
 * EIR: NUMERIC(10,8) stored as string.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Solver Metadata
// ---------------------------------------------------------------------------

export const solverMetadataSchema = z.object({
  iterations: z.number().int(),
  maxIterations: z.number().int().default(100),
  finalResidual: z.string(), // high-precision decimal as string
  converged: z.boolean(),
  precision: z.string().default("HALF_EVEN, 8 desimal"),
  convergencePath: z
    .array(z.number()) // float ok for display chart only
    .optional(),
  algorithm: z.string().optional(),
  initialGuess: z.string().optional(),
  toleranceUsed: z.string().optional(),
  wasPOCICreditAdjusted: z.boolean().optional(),
});
export type SolverMetadata = z.infer<typeof solverMetadataSchema>;

// ---------------------------------------------------------------------------
// EIR Compute Response
// ---------------------------------------------------------------------------

export const eirComputeResponseSchema = z.object({
  instrumenId: z.string().uuid(),
  eirPerPeriod: z.string(), // NUMERIC(10,8) as string
  eirAnnualEquivalent: z.string().optional(),
  iterationsUsed: z.number().int().optional(),
  convergenceResidual: z.string().optional(),
  eirType: z.enum(["STANDARD", "CREDIT_ADJUSTED"]).default("STANDARD"),
  persisted: z.boolean(),
  computedAt: z.string(),
  solverMetadata: solverMetadataSchema.optional(),
});
export type EIRComputeResponse = z.infer<typeof eirComputeResponseSchema>;

// ---------------------------------------------------------------------------
// EIR Schedule Row
// ---------------------------------------------------------------------------

export const eirScheduleRowSchema = z.object({
  id: z.string().uuid(),
  scheduleIdKode: z.string().optional(),
  instrumenId: z.string().uuid(),
  periodeSeq: z.number().int(),
  tanggalPosting: z.string(),
  openingCarryingIdr: z.string(), // NUMERIC(20,4) as string
  cashInflowIdr: z.string(),
  pendapatanBungaEirIdr: z.string(),
  amortisasiPdIdr: z.string(),
  pelunasanPokokIdr: z.string(),
  closingCarryingIdr: z.string(),
  eirPeriode: z.string(), // NUMERIC(10,8) as string
  stageSaatPosting: z.string().nullable().optional(),
  statusPosting: z
    .enum(["PROYEKSI", "POSTED", "REVERSED", "RECOMPUTED"])
    .default("PROYEKSI"),
  recomputedFromSeq: z.number().int().nullable().optional(),
  createdAt: z.string().optional(),
});
export type EIRScheduleRow = z.infer<typeof eirScheduleRowSchema>;

// ---------------------------------------------------------------------------
// Schedule Version (for version selector)
// ---------------------------------------------------------------------------

export const scheduleVersionSchema = z.object({
  versionLabel: z.string(),
  scheduleVersion: z.number().int(),
  isActive: z.boolean(),
  effectiveFrom: z.string(),
  effectiveTo: z.string().nullable().optional(),
});
export type ScheduleVersion = z.infer<typeof scheduleVersionSchema>;

// ---------------------------------------------------------------------------
// EIR Amendment Proposal
// ---------------------------------------------------------------------------

export const eirAmendmentProposalSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string().optional(),
  namaInstrumen: z.string().optional(),
  klasifikasiPsak71: z.string().optional(),
  amendmentDate: z.string().optional(),
  workflowStatus: z.enum([
    "DRAFT",
    "PENDING_REVIEW",
    "PENDING_APPROVAL",
    "APPROVED",
    "REJECTED",
  ]),
  triggerSource: z
    .enum(["MANUAL", "CRON_DRIFT", "AD_HOC_BULK", "DOCUMENT_UPLOAD"])
    .default("MANUAL"),
  eirSebelum: z.string().nullable().optional(), // NUMERIC(10,8) as string
  eirSesudah: z.string().nullable().optional(),
  driftDelta: z.string().nullable().optional(),
  alasan: z.string().optional(),
  dokumenPendukungId: z.string().uuid().nullable().optional(),
  makerId: z.string().uuid().optional(),
  reviewerId: z.string().uuid().nullable().optional(),
  approverId: z.string().uuid().nullable().optional(),
  reviewedAt: z.string().nullable().optional(),
  approvedAt: z.string().nullable().optional(),
  catchUpAdjustment: z
    .object({
      deltaAmount: z.string(), // IDR NUMERIC(20,4) as string
      formulaVersion: z.string(),
      approvalRecordUrl: z.string().optional(),
    })
    .nullable()
    .optional(),
  driftReportId: z.string().nullable().optional(),
  revisedCashflows: z
    .array(
      z.object({
        date: z.string(),
        amountIdr: z.number(), // number OK for display; stored server-side as NUMERIC
      }),
    )
    .optional(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export type EIRAmendmentProposal = z.infer<typeof eirAmendmentProposalSchema>;

// ---------------------------------------------------------------------------
// Amendment Propose Form
// ---------------------------------------------------------------------------

export const amendmentProposeFormSchema = z.object({
  instrumenId: z.string().uuid({ message: "Pilih instrumen yang valid." }),
  amendmentDate: z.string().min(1, { message: "Tanggal amandemen wajib diisi." }),
  alasan: z
    .string()
    .min(20, { message: "Alasan harus minimal 20 karakter." })
    .max(2000),
  dokumenPendukungId: z.string().uuid({ message: "Dokumen pendukung wajib dilampirkan." }),
  revisedCashflows: z
    .array(
      z.object({
        date: z.string(),
        amountIdr: z.number(),
      }),
    )
    .min(2, { message: "Minimal 2 cashflow (CF_0 negatif + 1 inflow)." }),
});
export type AmendmentProposeForm = z.infer<typeof amendmentProposeFormSchema>;

// ---------------------------------------------------------------------------
// Drift Report
// ---------------------------------------------------------------------------

export const driftReportEntrySchema = z.object({
  instrumenId: z.string().uuid(),
  kodeInstrumen: z.string().optional(),
  namaInstrumen: z.string().optional(),
  eirStored: z.string(), // NUMERIC(10,8) as string
  eirComputed: z.string(),
  delta: z.string(),
  deltaBp: z.number().optional(), // basis points for display
  severity: z.enum(["HIGH", "MEDIUM", "LOW", "MISSING"]),
  proposalId: z.string().uuid().nullable().optional(),
  proposalStatus: z.string().nullable().optional(),
});
export type DriftReportEntry = z.infer<typeof driftReportEntrySchema>;

export const driftReportSchema = z.object({
  id: z.string(),
  triggerSource: z.enum(["CRON_DAILY", "AD_HOC", "PRE_ECL_CALC"]),
  scanStartedAt: z.string(),
  scanCompletedAt: z.string().nullable().optional(),
  totalScanned: z.number().int(),
  driftCount: z.number().int(),
  proposalsAutoCreated: z.number().int(),
  scheduleMissingCount: z.number().int().optional(),
  status: z.string().optional(),
  entries: z.array(driftReportEntrySchema).optional(),
});
export type DriftReport = z.infer<typeof driftReportSchema>;

// ---------------------------------------------------------------------------
// Workflow reject form (reusable)
// ---------------------------------------------------------------------------

export const amendmentRejectFormSchema = z.object({
  comment: z
    .string()
    .min(20, { message: "Alasan penolakan minimal 20 karakter." })
    .max(1000),
  signatureMethod: z.string(),
});
export type AmendmentRejectForm = z.infer<typeof amendmentRejectFormSchema>;

export const amendmentCancelFormSchema = z.object({
  comment: z
    .string()
    .min(20, { message: "Alasan pembatalan minimal 20 karakter." })
    .max(1000),
});
export type AmendmentCancelForm = z.infer<typeof amendmentCancelFormSchema>;
