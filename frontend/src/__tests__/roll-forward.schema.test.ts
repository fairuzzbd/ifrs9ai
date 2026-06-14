/**
 * Vitest unit tests for P4-M11 Roll-Forward CKPN schemas.
 *
 * No DOM render required — pure Zod schema + logic tests.
 * Covers:
 * - TransferBucket validation
 * - RollForwardM11Report happy path
 * - RollForwardM11Report validation failures (money string format)
 * - CKPNTrendPoint validation
 * - ComputeRollForwardForm validation (currentCalcRunId required)
 * - DetectionMethod enum exhaustiveness
 * - ReconcileStatus enum exhaustiveness
 */

import { describe, it, expect } from "vitest";

import {
  transferBucketSchema,
  transferBucketsSchema,
  rollForwardM11ReportSchema,
  ckpnTrendPointSchema,
  ckpnTrendResponseSchema,
  computeRollForwardFormSchema,
  detectionMethodEnum,
  rollForwardReconcileStatusEnum,
  portfolioRollForwardSchema,
  rollForwardInstrumentLineSchema,
  dataQualityWarningSchema,
} from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function validTransferBucket() {
  return {
    count: 45,
    eclMovementIdr: "2000000000.0000",
    countOverride: 0,
  };
}

function validTransferBuckets() {
  return {
    stage1To2: { count: 45, eclMovementIdr: "2000000000.0000", countOverride: 0 },
    stage2To1: { count: 12, eclMovementIdr: "-500000000.0000", countOverride: 0 },
    stage2To3: { count: 8, eclMovementIdr: "800000000.0000", countOverride: 0 },
    stage1To3: { count: 2, eclMovementIdr: "100000000.0000", countOverride: 0 },
    stage3To2: { count: 3, eclMovementIdr: "-300000000.0000", countOverride: 3 },
    stage3To1: { count: 0, eclMovementIdr: "0.0000", countOverride: 0 },
  };
}

function validReport() {
  return {
    reportId: "rf-test-001",
    currentCalcRunId: "00000000-0000-0000-0000-000000000001",
    priorCalcRunId: "00000000-0000-0000-0000-000000000002",
    currentPeriodeId: "JUNI-2026",
    priorPeriodeId: "MEI-2026",
    openingEclIdr: "13500000000.0000",
    transfers: validTransferBuckets(),
    newOriginations: { count: 15, eclIdr: "1200000000.0000" },
    derecognitions: { count: 7, priorEclIdr: "800000000.0000" },
    remeasurementsIdr: "500000000.0000",
    closingEclIdr: "16500000000.0000",
    reconcileStatus: "RECONCILED" as const,
    reconcileDeltaIdr: "0.0000",
    reconcileTolerance: "1.0000",
    detectionMethod: "BASIC_STATUS_DIFF" as const,
    phase5LimitationNote: "Deteksi menggunakan BASIC_STATUS_DIFF.",
    computedAt: "2026-06-13T09:00:00+07:00",
    warnings: [],
  };
}

// ---------------------------------------------------------------------------
// TransferBucket
// ---------------------------------------------------------------------------

describe("transferBucketSchema", () => {
  it("accepts valid transfer bucket (positive movement)", () => {
    const r = transferBucketSchema.safeParse(validTransferBucket());
    expect(r.success).toBe(true);
  });

  it("accepts negative eclMovementIdr (cure)", () => {
    const r = transferBucketSchema.safeParse({
      count: 12,
      eclMovementIdr: "-500000000.0000",
      countOverride: 0,
    });
    expect(r.success).toBe(true);
  });

  it("accepts zero eclMovementIdr", () => {
    const r = transferBucketSchema.safeParse({
      count: 0,
      eclMovementIdr: "0.0000",
      countOverride: 0,
    });
    expect(r.success).toBe(true);
  });

  it("accepts countOverride > 0 (management override)", () => {
    const r = transferBucketSchema.safeParse({
      count: 3,
      eclMovementIdr: "-300000000.0000",
      countOverride: 3,
    });
    expect(r.success).toBe(true);
  });

  it("rejects negative count", () => {
    const r = transferBucketSchema.safeParse({
      count: -1,
      eclMovementIdr: "100.0000",
      countOverride: 0,
    });
    expect(r.success).toBe(false);
  });

  it("rejects negative countOverride", () => {
    const r = transferBucketSchema.safeParse({
      count: 5,
      eclMovementIdr: "100.0000",
      countOverride: -1,
    });
    expect(r.success).toBe(false);
  });

  it("rejects missing eclMovementIdr", () => {
    const r = transferBucketSchema.safeParse({
      count: 5,
      countOverride: 0,
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// TransferBuckets (all 6)
// ---------------------------------------------------------------------------

describe("transferBucketsSchema", () => {
  it("accepts complete 6-bucket object", () => {
    const r = transferBucketsSchema.safeParse(validTransferBuckets());
    expect(r.success).toBe(true);
  });

  it("rejects missing stage1To2", () => {
    const { stage1To2, ...rest } = validTransferBuckets();
    void stage1To2;
    const r = transferBucketsSchema.safeParse(rest);
    expect(r.success).toBe(false);
  });

  it("rejects missing stage3To1", () => {
    const { stage3To1, ...rest } = validTransferBuckets();
    void stage3To1;
    const r = transferBucketsSchema.safeParse(rest);
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// RollForwardM11Report
// ---------------------------------------------------------------------------

describe("rollForwardM11ReportSchema", () => {
  it("accepts full valid report (RECONCILED)", () => {
    const r = rollForwardM11ReportSchema.safeParse(validReport());
    expect(r.success).toBe(true);
  });

  it("accepts report with priorCalcRunId = null (first period)", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      priorCalcRunId: null,
      priorPeriodeId: null,
      warnings: ["ROLL_FORWARD_FIRST_PERIOD_OPENING_ZERO"],
    });
    expect(r.success).toBe(true);
  });

  it("accepts MISMATCH report", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      reconcileStatus: "MISMATCH",
      reconcileDeltaIdr: "5000.0000",
      warnings: ["ROLL_FORWARD_MISMATCH_DETECTED"],
    });
    expect(r.success).toBe(true);
  });

  it("accepts report with data quality warnings", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      warnings: ["ROLL_FORWARD_HAS_DATA_QUALITY_WARNINGS"],
      dataQualityWarnings: [
        {
          instrumenId: "00000000-0000-0000-0000-000000000099",
          instrumenKode: "INST-001",
          warningCode: "STAGE_HISTORY_MISSING_FALLBACK_CALC_HEADER",
          message: "Stage transition dari calc_header karena tidak ada stage_history entry.",
        },
      ],
    });
    expect(r.success).toBe(true);
  });

  it("rejects missing reportId", () => {
    const { reportId, ...rest } = validReport();
    void reportId;
    const r = rollForwardM11ReportSchema.safeParse(rest);
    expect(r.success).toBe(false);
  });

  it("rejects invalid reconcileStatus", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      reconcileStatus: "PARTIAL_PHASE_5_DEFER", // not valid in M11
    });
    expect(r.success).toBe(false);
  });

  it("rejects invalid detectionMethod", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      detectionMethod: "UNKNOWN_METHOD",
    });
    expect(r.success).toBe(false);
  });

  it("accepts negative remeasurementsIdr (net release)", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      remeasurementsIdr: "-12345678.0000",
    });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// CKPNTrendPoint
// ---------------------------------------------------------------------------

describe("ckpnTrendPointSchema", () => {
  it("accepts full trend point", () => {
    const r = ckpnTrendPointSchema.safeParse({
      periodeId: "JUNI-2026",
      calcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: "00000000-0000-0000-0000-000000000002",
      eclTotalIdr: "15000000000.0000",
      eclByStage: {
        stage1: "10500000000.0000",
        stage2: "3600000000.0000",
        stage3: "900000000.0000",
      },
      deltaVsPriorIdr: "800000000.0000",
      deltaPct: "5.634",
    });
    expect(r.success).toBe(true);
  });

  it("accepts first-period point (delta = null)", () => {
    const r = ckpnTrendPointSchema.safeParse({
      periodeId: "JANUARI-2026",
      calcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: null,
      eclTotalIdr: "10500000000.0000",
      eclByStage: {
        stage1: "8000000000.0000",
        stage2: "2000000000.0000",
        stage3: "500000000.0000",
      },
      deltaVsPriorIdr: null,
      deltaPct: null,
    });
    expect(r.success).toBe(true);
  });

  it("rejects missing eclByStage", () => {
    const r = ckpnTrendPointSchema.safeParse({
      periodeId: "JUNI-2026",
      calcRunId: "00000000-0000-0000-0000-000000000001",
      eclTotalIdr: "15000000000.0000",
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// CKPNTrendResponse
// ---------------------------------------------------------------------------

describe("ckpnTrendResponseSchema", () => {
  it("accepts valid multi-period trend response", () => {
    const r = ckpnTrendResponseSchema.safeParse({
      periods: [
        {
          periodeId: "JANUARI-2026",
          calcRunId: "00000000-0000-0000-0000-000000000001",
          priorCalcRunId: null,
          eclTotalIdr: "10000000000.0000",
          eclByStage: {
            stage1: "8000000000.0000",
            stage2: "1500000000.0000",
            stage3: "500000000.0000",
          },
          deltaVsPriorIdr: null,
          deltaPct: null,
        },
        {
          periodeId: "FEBRUARI-2026",
          calcRunId: "00000000-0000-0000-0000-000000000002",
          priorCalcRunId: "00000000-0000-0000-0000-000000000001",
          eclTotalIdr: "11000000000.0000",
          eclByStage: {
            stage1: "8500000000.0000",
            stage2: "1900000000.0000",
            stage3: "600000000.0000",
          },
          deltaVsPriorIdr: "1000000000.0000",
          deltaPct: "10.00",
        },
      ],
      totalPeriodsAvailable: 2,
      periodsRequested: 12,
    });
    expect(r.success).toBe(true);
  });

  it("rejects empty periods array as structurally valid (API semantics)", () => {
    // Empty periods array is structurally valid Zod-wise; API returns 422 for < 2 sealed
    const r = ckpnTrendResponseSchema.safeParse({
      periods: [],
      totalPeriodsAvailable: 0,
      periodsRequested: 12,
    });
    expect(r.success).toBe(true); // schema allows empty; server enforces min 2 via 422
  });
});

// ---------------------------------------------------------------------------
// ComputeRollForwardForm
// ---------------------------------------------------------------------------

describe("computeRollForwardFormSchema", () => {
  it("accepts valid form with prior run", () => {
    const r = computeRollForwardFormSchema.safeParse({
      currentCalcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: "00000000-0000-0000-0000-000000000002",
      detectionMethod: "BASIC_STATUS_DIFF",
    });
    expect(r.success).toBe(true);
  });

  it("accepts first-period form (no prior)", () => {
    const r = computeRollForwardFormSchema.safeParse({
      currentCalcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: null,
      detectionMethod: "BASIC_STATUS_DIFF",
    });
    expect(r.success).toBe(true);
  });

  it("rejects empty currentCalcRunId", () => {
    const r = computeRollForwardFormSchema.safeParse({
      currentCalcRunId: "",
      priorCalcRunId: null,
      detectionMethod: "BASIC_STATUS_DIFF",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("currentCalcRunId"),
      );
      expect(issue).toBeDefined();
    }
  });

  it("rejects invalid detectionMethod", () => {
    const r = computeRollForwardFormSchema.safeParse({
      currentCalcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: null,
      detectionMethod: "FULL_LIFECYCLE_PHASE_5", // not allowed in Phase 4 form
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Enum exhaustiveness
// ---------------------------------------------------------------------------

describe("detectionMethodEnum", () => {
  it("defines exactly 2 values", () => {
    expect(detectionMethodEnum.options).toHaveLength(2);
    expect(detectionMethodEnum.options).toContain("BASIC_STATUS_DIFF");
    expect(detectionMethodEnum.options).toContain("FULL_LIFECYCLE_PHASE_5");
  });
});

describe("rollForwardReconcileStatusEnum", () => {
  it("defines exactly 2 values (RECONCILED and MISMATCH)", () => {
    expect(rollForwardReconcileStatusEnum.options).toHaveLength(2);
    expect(rollForwardReconcileStatusEnum.options).toContain("RECONCILED");
    expect(rollForwardReconcileStatusEnum.options).toContain("MISMATCH");
    // M11 does NOT include PARTIAL_PHASE_5_DEFER (that was M10)
    expect(rollForwardReconcileStatusEnum.options).not.toContain(
      "PARTIAL_PHASE_5_DEFER",
    );
  });
});

// ---------------------------------------------------------------------------
// PortfolioRollForward
// ---------------------------------------------------------------------------

describe("portfolioRollForwardSchema", () => {
  it("accepts valid portfolio roll-forward", () => {
    const r = portfolioRollForwardSchema.safeParse({
      portofolioId: "00000000-0000-0000-0000-000000000010",
      portofolioNama: "Portofolio Obligasi",
      currentCalcRunId: "00000000-0000-0000-0000-000000000001",
      priorCalcRunId: "00000000-0000-0000-0000-000000000002",
      instrumentCount: 120,
      openingEclIdr: "5000000000.0000",
      transfers: validTransferBuckets(),
      newOriginations: { count: 5, eclIdr: "400000000.0000" },
      derecognitions: { count: 2, priorEclIdr: "200000000.0000" },
      remeasurementsIdr: "150000000.0000",
      closingEclIdr: "6200000000.0000",
      detectionMethod: "BASIC_STATUS_DIFF",
      dataQualityWarnings: [],
    });
    expect(r.success).toBe(true);
  });

  it("rejects missing instrumentCount", () => {
    const base = {
      portofolioId: "00000000-0000-0000-0000-000000000010",
      currentCalcRunId: "00000000-0000-0000-0000-000000000001",
      openingEclIdr: "5000000000.0000",
      transfers: validTransferBuckets(),
      newOriginations: { count: 5, eclIdr: "400000000.0000" },
      derecognitions: { count: 2, priorEclIdr: "200000000.0000" },
      remeasurementsIdr: "150000000.0000",
      closingEclIdr: "6200000000.0000",
      detectionMethod: "BASIC_STATUS_DIFF",
    };
    const r = portfolioRollForwardSchema.safeParse(base);
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// RollForwardInstrumentLine
// ---------------------------------------------------------------------------

describe("rollForwardInstrumentLineSchema", () => {
  it("accepts origination line (no stagePrior)", () => {
    const r = rollForwardInstrumentLineSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000020",
      instrumenKode: "OBLIG-001",
      instrumenNama: "Obligasi Pemerintah FR0080",
      stagePrior: null,
      stageCurrent: "STAGE_1",
      eclPriorIdr: null,
      eclCurrentIdr: "5000000.0000",
      eclMovementIdr: "5000000.0000",
      overrideFlag: false,
      bucket: "new_origination",
    });
    expect(r.success).toBe(true);
  });

  it("accepts Stage 3 → 2 override transfer", () => {
    const r = rollForwardInstrumentLineSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000021",
      instrumenKode: "DEPO-010",
      stagePrior: "STAGE_3",
      stageCurrent: "STAGE_2",
      eclPriorIdr: "50000000.0000",
      eclCurrentIdr: "20000000.0000",
      eclMovementIdr: "-30000000.0000",
      overrideFlag: true,
      bucket: "stage_3_to_2",
    });
    expect(r.success).toBe(true);
  });

  it("rejects invalid bucket value", () => {
    const r = rollForwardInstrumentLineSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000022",
      bucket: "invalid_bucket",
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// DataQualityWarning
// ---------------------------------------------------------------------------

describe("dataQualityWarningSchema", () => {
  it("accepts STAGE_HISTORY_MISSING_FALLBACK_CALC_HEADER warning", () => {
    const r = dataQualityWarningSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000030",
      instrumenKode: "INST-009",
      warningCode: "STAGE_HISTORY_MISSING_FALLBACK_CALC_HEADER",
      message: "Stage transition dari calc_header.",
    });
    expect(r.success).toBe(true);
  });

  it("accepts INSTRUMEN_AKTIF_NOT_IN_CURRENT_RUN warning", () => {
    const r = dataQualityWarningSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000031",
      warningCode: "INSTRUMEN_AKTIF_NOT_IN_CURRENT_RUN",
      message: "Instrumen AKTIF tidak ada di current run.",
    });
    expect(r.success).toBe(true);
  });

  it("rejects unknown warningCode", () => {
    const r = dataQualityWarningSchema.safeParse({
      instrumenId: "00000000-0000-0000-0000-000000000032",
      warningCode: "UNKNOWN_WARNING",
      message: "Unknown warning.",
    });
    expect(r.success).toBe(false);
  });

  it("rejects missing instrumenId", () => {
    const r = dataQualityWarningSchema.safeParse({
      warningCode: "DERECOGNITION_REASON_UNKNOWN",
      message: "Cannot determine reason.",
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Money: no parseFloat contract (string-based)
// ---------------------------------------------------------------------------

describe("Money: string-based contract (no parseFloat)", () => {
  it("openingEclIdr is parsed as string, not number", () => {
    const r = rollForwardM11ReportSchema.safeParse(validReport());
    if (!r.success) throw new Error("Should succeed");
    expect(typeof r.data.openingEclIdr).toBe("string");
    expect(typeof r.data.closingEclIdr).toBe("string");
    expect(typeof r.data.remeasurementsIdr).toBe("string");
    expect(typeof r.data.transfers.stage1To2.eclMovementIdr).toBe("string");
  });

  it("rejects numeric openingEclIdr (must be string)", () => {
    const r = rollForwardM11ReportSchema.safeParse({
      ...validReport(),
      openingEclIdr: 13500000000, // number instead of string
    });
    expect(r.success).toBe(false);
  });
});
