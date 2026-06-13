/**
 * Zod schemas for APP-C ECL Core results (P4-M10).
 *
 * Mirrors OpenAPI app-c-ecl-core.yaml.
 * Money: string-based per DEC-016 — no parseFloat/float.
 */

import { z } from "zod";

// ---------------------------------------------------------------------------
// Routing Path
// ---------------------------------------------------------------------------

export const routingPathEnum = z.enum([
  "STANDARD",
  "LPS",
  "LOOKTHROUGH",
  "POCI_DEFERRED",
  "FVTPL_SKIPPED",
]);
export type RoutingPath = z.infer<typeof routingPathEnum>;

// ---------------------------------------------------------------------------
// Scenario Breakdown (per instrument)
// ---------------------------------------------------------------------------

export const scenarioLineSchema = z.object({
  scenario: z.enum(["GOOD", "NORMAL", "BAD"]),
  weight: z.string(), // NUMERIC(10,8) as string e.g. "0.25000000"
  pdUsed: z.string(), // NUMERIC(10,8)
  lgdUsed: z.string(), // NUMERIC(10,8)
  eadIdr: z.string(), // NUMERIC(20,4)
  eclScenarioIdr: z.string(), // NUMERIC(20,4)
  flMultiplier: z.string().nullable().optional(), // null for Stage 3
  eclFlIdr: z.string(), // NUMERIC(20,4)
});
export type ScenarioLine = z.infer<typeof scenarioLineSchema>;

// ---------------------------------------------------------------------------
// ECL Result Line (per instrument)
// ---------------------------------------------------------------------------

export const eclResultLineSchema = z.object({
  id: z.string(),
  calcRunId: z.string(),
  instrumenId: z.string(),
  kodeInstrumen: z.string().optional(),
  namaInstrumen: z.string().optional(),
  tipeInstrumen: z.string().optional(),
  klasifikasiPsak71: z.string().optional(),
  portofolioId: z.string().optional(),
  portofolioNama: z.string().optional(),
  routingPath: routingPathEnum,
  stage: z.union([z.literal(1), z.literal(2), z.literal(3)]).nullable().optional(),
  eadIdr: z.string(), // NUMERIC(20,4) as string
  lgdUsed: z.string().optional(), // NUMERIC(10,8)
  eclWeightedIdr: z.string(), // NUMERIC(20,4)
  eclFlIdr: z.string().optional(), // NUMERIC(20,4)
  // Stage 3 specific
  grossCarryingIdr: z.string().nullable().optional(),
  eclAllowancePriorIdr: z.string().nullable().optional(),
  netCarryingIdr: z.string().nullable().optional(),
  // Scenario breakdown
  scenarioBreakdown: z.array(scenarioLineSchema).optional(),
  // Look-through underlying
  lookthroughUnderlying: z
    .array(
      z.object({
        assetClass: z.string(),
        percentNab: z.string(), // e.g. "60.00"
        eadIdr: z.string(),
        eclIdr: z.string(),
      }),
    )
    .optional(),
  // LPS aggregation
  lpsAggregation: z
    .object({
      totalEksposurIdr: z.string(),
      coveredIdr: z.string(),
      excessIdr: z.string(),
    })
    .nullable()
    .optional(),
  warnings: z.array(z.string()).default([]),
  errorMessage: z.string().nullable().optional(),
  computedAt: z.string().optional(),
  createdAt: z.string().optional(),
});
export type EclResultLine = z.infer<typeof eclResultLineSchema>;

// ---------------------------------------------------------------------------
// Portfolio Summary
// ---------------------------------------------------------------------------

export const portfolioSummarySchema = z.object({
  calcRunId: z.string(),
  portofolioId: z.string(),
  portofolioNama: z.string().optional(),
  totalInstrumen: z.number().int(),
  processedCount: z.number().int(),
  errorCount: z.number().int().default(0),
  stage1Count: z.number().int().default(0),
  stage2Count: z.number().int().default(0),
  stage3Count: z.number().int().default(0),
  totalEclWeightedIdr: z.string(), // NUMERIC(20,4)
  stage1EclIdr: z.string().default("0.0000"),
  stage2EclIdr: z.string().default("0.0000"),
  stage3EclIdr: z.string().default("0.0000"),
  routingDistribution: z
    .array(
      z.object({
        routingPath: routingPathEnum,
        count: z.number().int(),
        eclIdr: z.string(),
      }),
    )
    .optional(),
  // Comparison with prior run
  priorCalcRunId: z.string().nullable().optional(),
  priorTotalEclWeightedIdr: z.string().nullable().optional(),
  priorStage1EclIdr: z.string().nullable().optional(),
  priorStage2EclIdr: z.string().nullable().optional(),
  priorStage3EclIdr: z.string().nullable().optional(),
});
export type PortfolioSummary = z.infer<typeof portfolioSummarySchema>;

// ---------------------------------------------------------------------------
// Roll-Forward Component
// ---------------------------------------------------------------------------

export const rollForwardComponentSchema = z.object({
  komponen: z.string(),
  sign: z.enum(["+", "-", "=", "±"]),
  jumlahIdr: z.string().nullable(), // null = PARTIAL_PHASE_5_DEFER
  isPhase5Deferred: z.boolean().default(false),
  isClosing: z.boolean().default(false),
});
export type RollForwardComponent = z.infer<typeof rollForwardComponentSchema>;

export const reconcileStatusEnum = z.enum([
  "RECONCILED",
  "PARTIAL_PHASE_5_DEFER",
  "MISMATCH",
]);
export type ReconcileStatus = z.infer<typeof reconcileStatusEnum>;

export const rollForwardReportSchema = z.object({
  calcRunId: z.string(),
  periodeId: z.string(),
  periodeLabel: z.string().optional(),
  priorCalcRunId: z.string().nullable().optional(),
  priorPeriodeLabel: z.string().nullable().optional(),
  components: z.array(rollForwardComponentSchema),
  closingIdr: z.string().nullable(), // NUMERIC(20,4)
  eclTotalCalcRunIdr: z.string().nullable(),
  selisihIdr: z.string().nullable(),
  reconcileStatus: reconcileStatusEnum,
  // Per-portfolio breakdown
  portofolioBreakdown: z
    .array(
      z.object({
        portofolioId: z.string(),
        portofolioNama: z.string().optional(),
        openingIdr: z.string().nullable(),
        closingIdr: z.string().nullable(),
      }),
    )
    .optional(),
  generatedAt: z.string().optional(),
});
export type RollForwardReport = z.infer<typeof rollForwardReportSchema>;
