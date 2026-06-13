/**
 * Zod schemas for APP-C Roll-Forward CKPN (P4-M11).
 *
 * Mirrors OpenAPI app-c-roll-forward.yaml.
 * Money: string-based per DEC-016 — no parseFloat/float.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export const detectionMethodEnum = z.enum([
  "BASIC_STATUS_DIFF",
  "FULL_LIFECYCLE_PHASE_5",
]);
export type DetectionMethod = z.infer<typeof detectionMethodEnum>;

export const rollForwardReconcileStatusEnum = z.enum([
  "RECONCILED",
  "MISMATCH",
]);
export type RollForwardReconcileStatus = z.infer<typeof rollForwardReconcileStatusEnum>;

export const transferBucketKeyEnum = z.enum([
  "stage_1_to_2",
  "stage_2_to_1",
  "stage_2_to_3",
  "stage_1_to_3",
  "stage_3_to_2",
  "stage_3_to_1",
  "new_origination",
  "derecognition",
  "stage_same",
]);
export type TransferBucketKey = z.infer<typeof transferBucketKeyEnum>;

// ---------------------------------------------------------------------------
// TransferBucket
// ---------------------------------------------------------------------------

export const transferBucketSchema = z.object({
  count: z.number().int().min(0),
  eclMovementIdr: z.string(), // NUMERIC(20,4) as string, signed
  countOverride: z.number().int().min(0),
});
export type TransferBucket = z.infer<typeof transferBucketSchema>;

// ---------------------------------------------------------------------------
// TransferBuckets (all 6 directional)
// ---------------------------------------------------------------------------

export const transferBucketsSchema = z.object({
  stage1To2: transferBucketSchema,
  stage2To1: transferBucketSchema,
  stage2To3: transferBucketSchema,
  stage1To3: transferBucketSchema,
  stage3To2: transferBucketSchema,
  stage3To1: transferBucketSchema,
});
export type TransferBuckets = z.infer<typeof transferBucketsSchema>;

// ---------------------------------------------------------------------------
// Origination & Derecognition
// ---------------------------------------------------------------------------

export const originationSummarySchema = z.object({
  count: z.number().int().min(0),
  eclIdr: z.string(), // NUMERIC(20,4) positive
});
export type OriginationSummary = z.infer<typeof originationSummarySchema>;

export const derecognitionSummarySchema = z.object({
  count: z.number().int().min(0),
  priorEclIdr: z.string(), // NUMERIC(20,4) positive
});
export type DerecognitionSummary = z.infer<typeof derecognitionSummarySchema>;

// ---------------------------------------------------------------------------
// Data Quality Warning
// ---------------------------------------------------------------------------

export const dataQualityWarningSchema = z.object({
  instrumenId: z.string(),
  instrumenKode: z.string().nullable().optional(),
  warningCode: z.enum([
    "STAGE_HISTORY_MISSING_FALLBACK_CALC_HEADER",
    "INSTRUMEN_AKTIF_NOT_IN_CURRENT_RUN",
    "DERECOGNITION_REASON_UNKNOWN",
  ]),
  message: z.string(),
});
export type DataQualityWarning = z.infer<typeof dataQualityWarningSchema>;

// ---------------------------------------------------------------------------
// Full Roll-Forward Report (M11)
// ---------------------------------------------------------------------------

export const rollForwardM11ReportSchema = z.object({
  reportId: z.string(),
  currentCalcRunId: z.string(),
  priorCalcRunId: z.string().nullable().optional(),
  currentPeriodeId: z.string(),
  priorPeriodeId: z.string().nullable().optional(),
  openingEclIdr: z.string(), // NUMERIC(20,4)
  transfers: transferBucketsSchema,
  newOriginations: originationSummarySchema,
  derecognitions: derecognitionSummarySchema,
  remeasurementsIdr: z.string(), // signed NUMERIC(20,4)
  closingEclIdr: z.string(), // NUMERIC(20,4)
  reconcileStatus: rollForwardReconcileStatusEnum,
  reconcileDeltaIdr: z.string(), // signed NUMERIC(20,4)
  reconcileTolerance: z.string(), // "1.0000"
  detectionMethod: detectionMethodEnum,
  phase5LimitationNote: z.string(),
  computedAt: z.string(), // ISO 8601
  warnings: z.array(
    z.enum([
      "ROLL_FORWARD_FIRST_PERIOD_OPENING_ZERO",
      "ROLL_FORWARD_MISMATCH_DETECTED",
      "ROLL_FORWARD_PRIOR_NOT_SEALED_PREVIEW",
      "ROLL_FORWARD_HAS_DATA_QUALITY_WARNINGS",
    ]),
  ),
  dataQualityWarnings: z.array(dataQualityWarningSchema).nullable().optional(),
});
export type RollForwardM11Report = z.infer<typeof rollForwardM11ReportSchema>;

// ---------------------------------------------------------------------------
// Async Job Response
// ---------------------------------------------------------------------------

export const rollForwardJobAcceptedSchema = z.object({
  jobId: z.string(),
  type: z.enum(["ROLL_FORWARD_COMPUTE", "ROLL_FORWARD_EXPORT"]),
  statusUrl: z.string(),
  streamUrl: z.string(),
  currentCalcRunId: z.string(),
  priorCalcRunId: z.string().nullable().optional(),
});
export type RollForwardJobAccepted = z.infer<typeof rollForwardJobAcceptedSchema>;

// ---------------------------------------------------------------------------
// Per-portfolio Roll-Forward
// ---------------------------------------------------------------------------

export const portfolioRollForwardSchema = z.object({
  portofolioId: z.string(),
  portofolioNama: z.string().nullable().optional(),
  currentCalcRunId: z.string(),
  priorCalcRunId: z.string().nullable().optional(),
  instrumentCount: z.number().int(),
  openingEclIdr: z.string(),
  transfers: transferBucketsSchema,
  newOriginations: originationSummarySchema,
  derecognitions: derecognitionSummarySchema,
  remeasurementsIdr: z.string(),
  closingEclIdr: z.string(),
  detectionMethod: detectionMethodEnum,
  dataQualityWarnings: z.array(dataQualityWarningSchema).optional(),
});
export type PortfolioRollForward = z.infer<typeof portfolioRollForwardSchema>;

// ---------------------------------------------------------------------------
// Instrument drill-down line
// ---------------------------------------------------------------------------

export const rollForwardInstrumentLineSchema = z.object({
  instrumenId: z.string(),
  instrumenKode: z.string().optional(),
  instrumenNama: z.string().optional(),
  portofolioId: z.string().optional(),
  stagePrior: z.enum(["STAGE_1", "STAGE_2", "STAGE_3"]).nullable().optional(),
  stageCurrent: z.enum(["STAGE_1", "STAGE_2", "STAGE_3"]).nullable().optional(),
  eclPriorIdr: z.string().nullable().optional(),
  eclCurrentIdr: z.string().nullable().optional(),
  eclMovementIdr: z.string().nullable().optional(),
  overrideFlag: z.boolean().optional(),
  bucket: transferBucketKeyEnum.optional(),
});
export type RollForwardInstrumentLine = z.infer<typeof rollForwardInstrumentLineSchema>;

// ---------------------------------------------------------------------------
// CKPN Trend
// ---------------------------------------------------------------------------

export const ckpnTrendPointSchema = z.object({
  periodeId: z.string(),
  calcRunId: z.string(),
  priorCalcRunId: z.string().nullable().optional(),
  eclTotalIdr: z.string(), // NUMERIC(20,4)
  eclByStage: z.object({
    stage1: z.string(),
    stage2: z.string(),
    stage3: z.string(),
  }),
  deltaVsPriorIdr: z.string().nullable().optional(),
  deltaPct: z.string().nullable().optional(),
});
export type CKPNTrendPoint = z.infer<typeof ckpnTrendPointSchema>;

export const ckpnTrendResponseSchema = z.object({
  periods: z.array(ckpnTrendPointSchema),
  totalPeriodsAvailable: z.number().int(),
  periodsRequested: z.number().int(),
  note: z.string().optional(),
});
export type CKPNTrendResponse = z.infer<typeof ckpnTrendResponseSchema>;

// ---------------------------------------------------------------------------
// Form schemas
// ---------------------------------------------------------------------------

export const computeRollForwardFormSchema = z.object({
  currentCalcRunId: z.string().min(1, "Pilih calc run saat ini"),
  priorCalcRunId: z.string().nullable().optional(),
  detectionMethod: z.literal("BASIC_STATUS_DIFF"),
});
export type ComputeRollForwardForm = z.infer<typeof computeRollForwardFormSchema>;
