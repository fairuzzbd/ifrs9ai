/**
 * Zod schemas — P5-M10 POCI Delta ECL
 * Derived from api/openapi/app-c-poci-delta.yaml
 * DEC-016: NUMERIC(20,4) IDR, NUMERIC(10,8) EIR — numbers serialised as strings from API
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const pociDirectionEnum = z.enum(["INCREASE", "DECREASE", "ZERO"]);
export type PociDirection = z.infer<typeof pociDirectionEnum>;

export const pociStatusEnum = z.enum(["COMPUTED", "POSTED", "SKIPPED_ZERO"]);
export type PociStatus = z.infer<typeof pociStatusEnum>;

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

export const POCI_DIRECTION_LABELS: Record<PociDirection, string> = {
  INCREASE: "Meningkat",
  DECREASE: "Menurun",
  ZERO: "Tidak Berubah",
};

export const POCI_STATUS_LABELS: Record<PociStatus, string> = {
  COMPUTED: "Dihitung",
  POSTED: "Diposting",
  SKIPPED_ZERO: "Dilewati (Nol)",
};

// ---------------------------------------------------------------------------
// Baseline schemas
// ---------------------------------------------------------------------------

export const pociBaselineListItemSchema = z.object({
  id: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  tanggalBaseline: z.string(),
  lifetimeEclAtOrigination: z.string(),
  creditAdjustedEir: z.string(),
  createdAt: z.string(),
});
export type PociBaselineListItem = z.infer<typeof pociBaselineListItemSchema>;

export const pociBaselineDetailSchema = pociBaselineListItemSchema.extend({
  cashflowExpektasiJsonb: z.unknown().nullable().optional(),
  originationDate: z.string().optional(),
  createdBy: z.string().uuid().optional(),
});
export type PociBaselineDetail = z.infer<typeof pociBaselineDetailSchema>;

// ---------------------------------------------------------------------------
// Delta log schemas
// ---------------------------------------------------------------------------

export const pociDeltaLogItemSchema = z.object({
  id: z.string().uuid(),
  calcRunId: z.string().uuid(),
  instrumenId: z.string().uuid(),
  instrumenKode: z.string(),
  tanggalCompute: z.string(),
  baselineEcl: z.string(),
  currentEcl: z.string(),
  deltaEcl: z.string(),
  direction: pociDirectionEnum,
  priorDeltaCumulative: z.string().nullable().optional(),
  jurnalHeaderId: z.string().uuid().nullable().optional(),
  periodeBulananId: z.string().uuid().nullable().optional(),
  status: pociStatusEnum,
  largeDeltaFlag: z.boolean(),
  createdAt: z.string(),
});
export type PociDeltaLogItem = z.infer<typeof pociDeltaLogItemSchema>;

// Delta history is same shape as delta log item
export const pociDeltaHistoryItemSchema = pociDeltaLogItemSchema;
export type PociDeltaHistoryItem = z.infer<typeof pociDeltaHistoryItemSchema>;

// ---------------------------------------------------------------------------
// Dashboard summary schema
// ---------------------------------------------------------------------------

export const pociDirectionEntrySchema = z.object({
  count: z.number().int(),
  amountIdr: z.string(),
});

export const pociZeroEntrySchema = z.object({
  count: z.number().int(),
});

export const pociDeltaSummarySchema = z.object({
  portofolioId: z.string().uuid().nullable().optional(),
  year: z.number().int(),
  month: z.number().int().min(1).max(12),
  instrumenCount: z.number().int(),
  deltaEclMtdIdr: z.string(),
  deltaEclYtdIdr: z.string(),
  netCumulativeDeltaIdr: z.string(),
  directionBreakdown: z.object({
    increase: pociDirectionEntrySchema,
    decrease: pociDirectionEntrySchema,
    zero: pociZeroEntrySchema,
  }),
  largeDeltaCount: z.number().int(),
});
export type PociDeltaSummary = z.infer<typeof pociDeltaSummarySchema>;

// ---------------------------------------------------------------------------
// Error codes (6 baru dari OpenAPI P5-M10)
// ---------------------------------------------------------------------------

export const pociErrorCodes = [
  "POCI_BASELINE_MISSING",
  "POCI_BASELINE_IMMUTABLE_VIOLATION",
  "POCI_DELTA_DUPLICATE",
  "POCI_INSTRUMEN_NOT_POCI",
  "POCI_PERIODE_LOCKED",
  "POCI_JURNAL_DIRECTION_MISMATCH",
] as const;
export type PociErrorCode = (typeof pociErrorCodes)[number];
