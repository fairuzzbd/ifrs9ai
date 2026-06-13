/**
 * Vitest unit tests for ecl-core.schema.ts (P4-M10)
 */

import { describe, it, expect } from "vitest";
import {
  routingPathEnum,
  scenarioLineSchema,
  eclResultLineSchema,
  portfolioSummarySchema,
  rollForwardComponentSchema,
  reconcileStatusEnum,
  rollForwardReportSchema,
} from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// RoutingPath enum
// ---------------------------------------------------------------------------

describe("routingPathEnum", () => {
  it("accepts all 5 routing paths", () => {
    const paths = ["STANDARD", "LPS", "LOOKTHROUGH", "POCI_DEFERRED", "FVTPL_SKIPPED"] as const;
    for (const p of paths) {
      expect(routingPathEnum.safeParse(p).success).toBe(true);
    }
  });

  it("rejects invalid path", () => {
    expect(routingPathEnum.safeParse("UNKNOWN").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// ScenarioLine
// ---------------------------------------------------------------------------

describe("scenarioLineSchema", () => {
  const validLine = {
    scenario: "GOOD",
    weight: "0.25000000",
    pdUsed: "0.02350000",
    lgdUsed: "0.45000000",
    eadIdr: "1000000000.0000",
    eclScenarioIdr: "10575000.0000",
    flMultiplier: "1.10000000",
    eclFlIdr: "11632500.0000",
  };

  it("parses valid scenario line", () => {
    const result = scenarioLineSchema.safeParse(validLine);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.scenario).toBe("GOOD");
      expect(result.data.weight).toBe("0.25000000");
    }
  });

  it("accepts null flMultiplier (Stage 3)", () => {
    const stage3Line = { ...validLine, flMultiplier: null };
    const result = scenarioLineSchema.safeParse(stage3Line);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.flMultiplier).toBeNull();
    }
  });

  it("rejects unknown scenario", () => {
    const bad = { ...validLine, scenario: "PESSIMISTIC" };
    expect(scenarioLineSchema.safeParse(bad).success).toBe(false);
  });

  it("requires string money fields (not number)", () => {
    // eadIdr must be string, not number (DEC-016)
    const bad = { ...validLine, eadIdr: 1000000 };
    expect(scenarioLineSchema.safeParse(bad).success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// EclResultLine
// ---------------------------------------------------------------------------

describe("eclResultLineSchema", () => {
  const minimal = {
    id: "ecl-result-001",
    calcRunId: "CR-2026-06-001",
    instrumenId: "INST-001",
    routingPath: "STANDARD",
    eadIdr: "500000000.0000",
    eclWeightedIdr: "5000000.0000",
    warnings: [],
  };

  it("parses minimal valid result line", () => {
    const result = eclResultLineSchema.safeParse(minimal);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.routingPath).toBe("STANDARD");
      expect(result.data.warnings).toEqual([]);
    }
  });

  it("parses Stage 3 with netCarrying fields", () => {
    const stage3 = {
      ...minimal,
      stage: 3 as const,
      grossCarryingIdr: "500000000.0000",
      eclAllowancePriorIdr: "50000000.0000",
      netCarryingIdr: "450000000.0000",
    };
    const result = eclResultLineSchema.safeParse(stage3);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.stage).toBe(3);
      expect(result.data.netCarryingIdr).toBe("450000000.0000");
    }
  });

  it("parses FVTPL_SKIPPED result (no scenarioBreakdown)", () => {
    const fvtpl = {
      ...minimal,
      routingPath: "FVTPL_SKIPPED",
      stage: null,
    };
    const result = eclResultLineSchema.safeParse(fvtpl);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.routingPath).toBe("FVTPL_SKIPPED");
      expect(result.data.stage).toBeNull();
    }
  });

  it("defaults warnings to empty array", () => {
    const noWarnings = { ...minimal };
    delete (noWarnings as Record<string, unknown>).warnings;
    const result = eclResultLineSchema.safeParse(noWarnings);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.warnings).toEqual([]);
    }
  });

  it("rejects invalid stage value", () => {
    const bad = { ...minimal, stage: 4 };
    expect(eclResultLineSchema.safeParse(bad).success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// PortfolioSummary
// ---------------------------------------------------------------------------

describe("portfolioSummarySchema", () => {
  const valid = {
    calcRunId: "CR-2026-06-001",
    portofolioId: "PORT-001",
    totalInstrumen: 50,
    processedCount: 50,
    totalEclWeightedIdr: "250000000.0000",
  };

  it("parses minimal valid portfolio summary", () => {
    const result = portfolioSummarySchema.safeParse(valid);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.errorCount).toBe(0);
      expect(result.data.stage1Count).toBe(0);
      expect(result.data.stage1EclIdr).toBe("0.0000");
    }
  });

  it("parses full summary with prior run comparison", () => {
    const full = {
      ...valid,
      stage1Count: 40,
      stage2Count: 8,
      stage3Count: 2,
      stage1EclIdr: "200000000.0000",
      stage2EclIdr: "40000000.0000",
      stage3EclIdr: "10000000.0000",
      priorCalcRunId: "CR-2026-05-001",
      priorTotalEclWeightedIdr: "230000000.0000",
    };
    const result = portfolioSummarySchema.safeParse(full);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.stage3Count).toBe(2);
      expect(result.data.priorCalcRunId).toBe("CR-2026-05-001");
    }
  });
});

// ---------------------------------------------------------------------------
// RollForwardComponent
// ---------------------------------------------------------------------------

describe("rollForwardComponentSchema", () => {
  it("parses normal component", () => {
    const result = rollForwardComponentSchema.safeParse({
      komponen: "Opening CKPN",
      sign: "=",
      jumlahIdr: "500000000.0000",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.isPhase5Deferred).toBe(false);
      expect(result.data.isClosing).toBe(false);
    }
  });

  it("accepts null jumlahIdr (phase 5 deferred)", () => {
    const result = rollForwardComponentSchema.safeParse({
      komponen: "Transfer Stage 1→2",
      sign: "+",
      jumlahIdr: null,
      isPhase5Deferred: true,
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.jumlahIdr).toBeNull();
      expect(result.data.isPhase5Deferred).toBe(true);
    }
  });

  it("rejects invalid sign", () => {
    const result = rollForwardComponentSchema.safeParse({
      komponen: "X",
      sign: "*",
      jumlahIdr: "100.0000",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// ReconcileStatus enum
// ---------------------------------------------------------------------------

describe("reconcileStatusEnum", () => {
  it("accepts all 3 valid statuses", () => {
    for (const s of ["RECONCILED", "PARTIAL_PHASE_5_DEFER", "MISMATCH"] as const) {
      expect(reconcileStatusEnum.safeParse(s).success).toBe(true);
    }
  });

  it("rejects invalid status", () => {
    expect(reconcileStatusEnum.safeParse("OK").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// RollForwardReport
// ---------------------------------------------------------------------------

describe("rollForwardReportSchema", () => {
  const validReport = {
    calcRunId: "CR-2026-06-001",
    periodeId: "JUNI-2026",
    components: [
      {
        komponen: "Opening CKPN",
        sign: "=" as const,
        jumlahIdr: "500000000.0000",
      },
      {
        komponen: "Closing CKPN",
        sign: "=" as const,
        jumlahIdr: "520000000.0000",
        isClosing: true,
      },
    ],
    closingIdr: "520000000.0000",
    eclTotalCalcRunIdr: "520000000.0000",
    selisihIdr: "0.0000",
    reconcileStatus: "RECONCILED" as const,
  };

  it("parses valid RECONCILED report", () => {
    const result = rollForwardReportSchema.safeParse(validReport);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.reconcileStatus).toBe("RECONCILED");
      expect(result.data.components).toHaveLength(2);
    }
  });

  it("parses PARTIAL_PHASE_5_DEFER report with null closing", () => {
    const partial = {
      ...validReport,
      closingIdr: null,
      eclTotalCalcRunIdr: "520000000.0000",
      selisihIdr: null,
      reconcileStatus: "PARTIAL_PHASE_5_DEFER" as const,
    };
    const result = rollForwardReportSchema.safeParse(partial);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.reconcileStatus).toBe("PARTIAL_PHASE_5_DEFER");
      expect(result.data.closingIdr).toBeNull();
    }
  });

  it("parses MISMATCH report with non-zero selisih", () => {
    const mismatch = {
      ...validReport,
      closingIdr: "520000000.0000",
      eclTotalCalcRunIdr: "525000000.0000",
      selisihIdr: "5000000.0000",
      reconcileStatus: "MISMATCH" as const,
    };
    const result = rollForwardReportSchema.safeParse(mismatch);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.reconcileStatus).toBe("MISMATCH");
      expect(result.data.selisihIdr).toBe("5000000.0000");
    }
  });
});
