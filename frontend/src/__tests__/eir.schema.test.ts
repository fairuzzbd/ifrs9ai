/**
 * Vitest tests for APP-C EIR Zod schemas (P4-M9).
 */
import { describe, it, expect } from "vitest";
import {
  amendmentProposeFormSchema,
  amendmentRejectFormSchema,
  solverMetadataSchema,
  eirComputeResponseSchema,
  driftReportEntrySchema,
  driftReportSchema,
} from "@/lib/schemas/eir.schema";

// ---------------------------------------------------------------------------
// amendmentProposeFormSchema
// ---------------------------------------------------------------------------

describe("amendmentProposeFormSchema", () => {
  const validBase = {
    instrumenId: "550e8400-e29b-41d4-a716-446655440000",
    amendmentDate: "2026-06-01",
    alasan: "Kontrak SBN diubah — kupon variable mulai Juli 2026. Cashflow baru telah diverifikasi.",
    dokumenPendukungId: "550e8400-e29b-41d4-a716-446655440001",
    revisedCashflows: [
      { date: "2026-06-01", amountIdr: -10000000000 },
      { date: "2026-12-01", amountIdr: 350000000 },
      { date: "2027-06-01", amountIdr: 10350000000 },
    ],
  };

  it("accepts valid amendment", () => {
    const result = amendmentProposeFormSchema.safeParse(validBase);
    expect(result.success).toBe(true);
  });

  it("rejects alasan shorter than 20 chars", () => {
    const result = amendmentProposeFormSchema.safeParse({
      ...validBase,
      alasan: "terlalu pendek",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const issue = result.error.issues.find((i) => i.path[0] === "alasan");
      expect(issue?.message).toContain("20");
    }
  });

  it("rejects cashflows with fewer than 2 entries", () => {
    const result = amendmentProposeFormSchema.safeParse({
      ...validBase,
      revisedCashflows: [{ date: "2026-06-01", amountIdr: -1000000 }],
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const issue = result.error.issues.find(
        (i) => i.path[0] === "revisedCashflows",
      );
      expect(issue).toBeDefined();
    }
  });

  it("rejects invalid instrumenId UUID", () => {
    const result = amendmentProposeFormSchema.safeParse({
      ...validBase,
      instrumenId: "not-uuid",
    });
    expect(result.success).toBe(false);
  });

  it("accepts exactly 2 cashflows (CF_0 + 1 inflow)", () => {
    const result = amendmentProposeFormSchema.safeParse({
      ...validBase,
      revisedCashflows: [
        { date: "2026-06-01", amountIdr: -10000000000 },
        { date: "2027-06-01", amountIdr: 10500000000 },
      ],
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// amendmentRejectFormSchema
// ---------------------------------------------------------------------------

describe("amendmentRejectFormSchema", () => {
  it("accepts valid reject comment", () => {
    const result = amendmentRejectFormSchema.safeParse({
      comment: "Cashflow revision tidak sesuai dengan dokumen kontrak yang dilampirkan.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(true);
  });

  it("rejects comment shorter than 20 chars", () => {
    const result = amendmentRejectFormSchema.safeParse({ comment: "terlalu pendek" });
    expect(result.success).toBe(false);
    if (!result.success) {
      const issue = result.error.issues.find((i) => i.path[0] === "comment");
      expect(issue?.message).toContain("20");
    }
  });

  it("accepts signatureMethod as JWT_STANDARD", () => {
    const result = amendmentRejectFormSchema.safeParse({
      comment: "Dokumen tidak lengkap dan cashflow tidak terverifikasi dengan benar.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.signatureMethod).toBe("JWT_STANDARD");
    }
  });
});

// ---------------------------------------------------------------------------
// solverMetadataSchema
// ---------------------------------------------------------------------------

describe("solverMetadataSchema", () => {
  it("accepts converged solver result", () => {
    const result = solverMetadataSchema.safeParse({
      iterations: 7,
      maxIterations: 100,
      finalResidual: "0.00000000012345",
      converged: true,
      precision: "HALF_EVEN, 8 desimal",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.converged).toBe(true);
      expect(result.data.iterations).toBe(7);
    }
  });

  it("accepts non-converged solver result", () => {
    const result = solverMetadataSchema.safeParse({
      iterations: 100,
      maxIterations: 100,
      finalResidual: "0.001234567890",
      converged: false,
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.converged).toBe(false);
    }
  });

  it("accepts convergence path array for chart", () => {
    const result = solverMetadataSchema.safeParse({
      iterations: 5,
      maxIterations: 100,
      finalResidual: "0.0000000001",
      converged: true,
      convergencePath: [0.1, 0.05, 0.001, 0.00001, 0.0000000001],
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.convergencePath).toHaveLength(5);
    }
  });

  it("rejects non-integer iterations", () => {
    const result = solverMetadataSchema.safeParse({
      iterations: 7.5,
      maxIterations: 100,
      finalResidual: "0.00000000012",
      converged: true,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// eirComputeResponseSchema — EIR fields are strings (DEC-016)
// ---------------------------------------------------------------------------

describe("eirComputeResponseSchema", () => {
  it("accepts EIR fields as strings, not numbers", () => {
    const result = eirComputeResponseSchema.safeParse({
      instrumenId: "550e8400-e29b-41d4-a716-446655440000",
      eirPerPeriod: "0.07250000",
      eirType: "STANDARD",
      persisted: true,
      computedAt: "2026-06-01T10:00:00Z",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      // Ensure eirPerPeriod stays as string (no float conversion)
      expect(typeof result.data.eirPerPeriod).toBe("string");
      expect(result.data.eirPerPeriod).toBe("0.07250000");
    }
  });

  it("accepts CREDIT_ADJUSTED eirType (POCI)", () => {
    const result = eirComputeResponseSchema.safeParse({
      instrumenId: "550e8400-e29b-41d4-a716-446655440000",
      eirPerPeriod: "0.08500000",
      eirType: "CREDIT_ADJUSTED",
      persisted: false,
      computedAt: "2026-06-01T10:00:00Z",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.eirType).toBe("CREDIT_ADJUSTED");
    }
  });

  it("rejects non-string eirPerPeriod", () => {
    const result = eirComputeResponseSchema.safeParse({
      instrumenId: "550e8400-e29b-41d4-a716-446655440000",
      eirPerPeriod: 0.0725, // should be string per DEC-016
      eirType: "STANDARD",
      persisted: true,
      computedAt: "2026-06-01T10:00:00Z",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// driftReportEntrySchema — severity enum
// ---------------------------------------------------------------------------

describe("driftReportEntrySchema", () => {
  const validEntry = {
    instrumenId: "550e8400-e29b-41d4-a716-446655440000",
    eirStored: "0.07250000",
    eirComputed: "0.07400000",
    delta: "0.00150000",
    severity: "MEDIUM" as const,
  };

  it("accepts valid drift entry", () => {
    const result = driftReportEntrySchema.safeParse(validEntry);
    expect(result.success).toBe(true);
  });

  it("accepts all severity levels", () => {
    for (const sev of ["HIGH", "MEDIUM", "LOW", "MISSING"] as const) {
      const result = driftReportEntrySchema.safeParse({ ...validEntry, severity: sev });
      expect(result.success).toBe(true);
    }
  });

  it("rejects invalid severity", () => {
    const result = driftReportEntrySchema.safeParse({ ...validEntry, severity: "CRITICAL" });
    expect(result.success).toBe(false);
  });

  it("stores EIR values as strings not numbers", () => {
    const result = driftReportEntrySchema.safeParse(validEntry);
    if (result.success) {
      expect(typeof result.data.eirStored).toBe("string");
      expect(typeof result.data.eirComputed).toBe("string");
      expect(typeof result.data.delta).toBe("string");
    }
  });
});

// ---------------------------------------------------------------------------
// driftReportSchema
// ---------------------------------------------------------------------------

describe("driftReportSchema", () => {
  it("accepts valid drift report", () => {
    const result = driftReportSchema.safeParse({
      id: "drift-report-001",
      triggerSource: "CRON_DAILY",
      scanStartedAt: "2026-06-01T00:00:00Z",
      totalScanned: 542,
      driftCount: 12,
      proposalsAutoCreated: 8,
    });
    expect(result.success).toBe(true);
  });

  it("accepts all triggerSource values", () => {
    for (const src of ["CRON_DAILY", "AD_HOC", "PRE_ECL_CALC"] as const) {
      const result = driftReportSchema.safeParse({
        id: "r1",
        triggerSource: src,
        scanStartedAt: "2026-06-01T00:00:00Z",
        totalScanned: 100,
        driftCount: 0,
        proposalsAutoCreated: 0,
      });
      expect(result.success).toBe(true);
    }
  });
});
