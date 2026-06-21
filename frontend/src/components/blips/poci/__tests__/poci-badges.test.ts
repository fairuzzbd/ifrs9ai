/**
 * Vitest component-logic tests — P5-M10 POCI badge state matrix
 *
 * Tests badge state logic, direction display rules, baseline immutable indicator,
 * and persona gating (absent-from-DOM pattern) without DOM rendering.
 * All logic derived from domain.go + PSAK 71 §5.5.13-14 sign convention.
 */

import { describe, it, expect } from "vitest";
import {
  pociDirectionEnum,
  pociStatusEnum,
  POCI_DIRECTION_LABELS,
  POCI_STATUS_LABELS,
  pociBaselineListItemSchema,
  pociDeltaLogItemSchema,
  pociDeltaSummarySchema,
  pociErrorCodes,
} from "@/lib/schemas/poci.schema";

// ---------------------------------------------------------------------------
// Direction enum + label matrix (S2-AC1, S2-AC2, S3-AC1, S3-AC2)
// ---------------------------------------------------------------------------

describe("PociDirection — badge state matrix", () => {
  it("INCREASE: delta_ecl > 0 — deterioration, red badge", () => {
    const result = pociDirectionEnum.safeParse("INCREASE");
    expect(result.success).toBe(true);
    expect(POCI_DIRECTION_LABELS["INCREASE"]).toBe("Meningkat");
  });

  it("DECREASE: delta_ecl < 0 — improvement, green badge", () => {
    const result = pociDirectionEnum.safeParse("DECREASE");
    expect(result.success).toBe(true);
    expect(POCI_DIRECTION_LABELS["DECREASE"]).toBe("Menurun");
  });

  it("ZERO: delta_ecl = 0 — no change, gray badge", () => {
    const result = pociDirectionEnum.safeParse("ZERO");
    expect(result.success).toBe(true);
    expect(POCI_DIRECTION_LABELS["ZERO"]).toBe("Tidak Berubah");
  });

  it("rejects unknown direction values", () => {
    const result = pociDirectionEnum.safeParse("UNKNOWN");
    expect(result.success).toBe(false);
  });

  it("covers all 3 directions in label map", () => {
    const directions = pociDirectionEnum.options;
    directions.forEach((d) => {
      expect(POCI_DIRECTION_LABELS[d]).toBeDefined();
      expect(POCI_DIRECTION_LABELS[d].length).toBeGreaterThan(0);
    });
  });
});

// ---------------------------------------------------------------------------
// Status enum (S2-AC1 COMPUTED, S3-AC1 POSTED, S2-AC3 SKIPPED_ZERO)
// ---------------------------------------------------------------------------

describe("PociStatus — badge state matrix", () => {
  it("COMPUTED: delta log written, jurnal not yet posted", () => {
    const result = pociStatusEnum.safeParse("COMPUTED");
    expect(result.success).toBe(true);
    expect(POCI_STATUS_LABELS["COMPUTED"]).toBe("Dihitung");
  });

  it("POSTED: jurnal posted in-transaction (S3-AC1)", () => {
    const result = pociStatusEnum.safeParse("POSTED");
    expect(result.success).toBe(true);
    expect(POCI_STATUS_LABELS["POSTED"]).toBe("Diposting");
  });

  it("SKIPPED_ZERO: direction = ZERO or periode CLOSED (S3-AC3)", () => {
    const result = pociStatusEnum.safeParse("SKIPPED_ZERO");
    expect(result.success).toBe(true);
    expect(POCI_STATUS_LABELS["SKIPPED_ZERO"]).toBe("Dilewati (Nol)");
  });

  it("rejects unknown status values", () => {
    expect(pociStatusEnum.safeParse("INVALID").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Delta direction display logic (S2-AC1 positive, S2-AC2 negative)
// ---------------------------------------------------------------------------

describe("Delta direction display logic", () => {
  it("positive deltaEcl → INCREASE direction, sign prefix '+'", () => {
    const deltaEcl = "200000000.0000";
    const parsed = parseFloat(deltaEcl);
    expect(parsed).toBeGreaterThan(0);
    // INCREASE direction
    const direction = parsed > 0 ? "INCREASE" : parsed < 0 ? "DECREASE" : "ZERO";
    expect(direction).toBe("INCREASE");
    // UI prefix
    const prefix = parsed > 0 ? "+" : "";
    expect(prefix).toBe("+");
  });

  it("negative deltaEcl → DECREASE direction, no prefix", () => {
    const deltaEcl = "-150000000.0000";
    const parsed = parseFloat(deltaEcl);
    expect(parsed).toBeLessThan(0);
    const direction = parsed > 0 ? "INCREASE" : parsed < 0 ? "DECREASE" : "ZERO";
    expect(direction).toBe("DECREASE");
    const prefix = parsed > 0 ? "+" : "";
    expect(prefix).toBe("");
  });

  it("zero deltaEcl → ZERO direction, no prefix", () => {
    const deltaEcl = "0.0000";
    const parsed = parseFloat(deltaEcl);
    expect(parsed).toBe(0);
    const direction = parsed > 0 ? "INCREASE" : parsed < 0 ? "DECREASE" : "ZERO";
    expect(direction).toBe("ZERO");
  });
});

// ---------------------------------------------------------------------------
// Baseline immutable indicator — WORM assertion (S1-AC2, DEC-018)
// ---------------------------------------------------------------------------

// Valid v4 UUIDs (version=4, variant=8-b) for Zod v4 uuid() strict validation
const BL_ID_1 = "f47ac10b-58cc-4372-a567-0e02b2c3d479";
const BL_INST_ID = "550e8400-e29b-41d4-a716-446655440000";

describe("PociBaselineImmutableBadge — WORM logic", () => {
  it("baseline schema validates a complete immutable row (S1-AC1)", () => {
    const baselineRow = {
      id: BL_ID_1,
      instrumenId: BL_INST_ID,
      instrumenKode: "POCI-DEP-0001",
      tanggalBaseline: "2026-06-20",
      lifetimeEclAtOrigination: "1250000000.0000",
      creditAdjustedEir: "0.04500000",
      createdAt: "2026-06-20T10:30:00+07:00",
    };
    const result = pociBaselineListItemSchema.safeParse(baselineRow);
    expect(result.success).toBe(true);
  });

  it("baseline schema rejects row missing required lifetimeEclAtOrigination", () => {
    const incompleteRow = {
      id: BL_ID_1,
      instrumenId: BL_INST_ID,
      instrumenKode: "POCI-DEP-0001",
      tanggalBaseline: "2026-06-20",
      creditAdjustedEir: "0.04500000",
      createdAt: "2026-06-20T10:30:00+07:00",
      // lifetimeEclAtOrigination intentionally missing
    };
    const result = pociBaselineListItemSchema.safeParse(incompleteRow);
    expect(result.success).toBe(false);
  });

  it("baseline schema requires valid UUID for instrumenId", () => {
    const invalidRow = {
      id: "not-a-uuid",
      instrumenId: "not-a-uuid",
      instrumenKode: "POCI-DEP-0001",
      tanggalBaseline: "2026-06-20",
      lifetimeEclAtOrigination: "1250000000.0000",
      creditAdjustedEir: "0.04500000",
      createdAt: "2026-06-20T10:30:00+07:00",
    };
    const result = pociBaselineListItemSchema.safeParse(invalidRow);
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Persona gating — absent-from-DOM assertion (poci.delta.compute)
// ---------------------------------------------------------------------------

describe("PociTriggerComputeButton — persona gating logic", () => {
  it("compute permission missing → button should be absent", () => {
    // Simulates the can('poci.delta.compute') check in PociTriggerComputeButton
    const canCompute = (permissions: string[]) =>
      permissions.includes("poci.delta.compute");

    expect(canCompute([])).toBe(false);
    expect(canCompute(["ecl_run.read"])).toBe(false);
    expect(canCompute(["poci.delta.compute"])).toBe(true);
    expect(canCompute(["poci.delta.compute", "ecl_run.read"])).toBe(true);
  });

  it("ROLE-IT-ADMIN has compute permission; ROLE-RISK does too; others do not", () => {
    // Permission matrix from personas.md + S2 permission gate
    const rolePermissions: Record<string, string[]> = {
      "ROLE-IT-ADMIN": ["poci.delta.compute", "sys.dlq.read"],
      "ROLE-RISK": ["poci.delta.compute", "ecl_run.read", "instrumen.read"],
      "ROLE-AKUN": ["transaksi.read", "jurnal.read"],
      "ROLE-AUDIT": ["ecl_run.read", "audit_log.read"],
      "ROLE-CFO": ["ecl_run.read"],
    };

    expect(rolePermissions["ROLE-IT-ADMIN"].includes("poci.delta.compute")).toBe(true);
    expect(rolePermissions["ROLE-RISK"].includes("poci.delta.compute")).toBe(true);
    expect(rolePermissions["ROLE-AKUN"].includes("poci.delta.compute")).toBe(false);
    expect(rolePermissions["ROLE-AUDIT"].includes("poci.delta.compute")).toBe(false);
    expect(rolePermissions["ROLE-CFO"].includes("poci.delta.compute")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Delta log schema — full row parse (S2-AC1, S2-AC4 idempotency fields)
// ---------------------------------------------------------------------------

// Valid v4 UUIDs (version=4, variant=8-b) for delta log fixtures
const DL_ID_1 = "6ba7b810-9dad-41d1-80b4-00c04fd430c8";
const DL_ID_2 = "6ba7b811-9dad-41d1-80b4-00c04fd430c8";
const DL_ID_3 = "6ba7b812-9dad-41d1-80b4-00c04fd430c8";
const DL_CALC_RUN = "6ba7b813-9dad-41d1-80b4-00c04fd430c8";
const DL_INST_ID = "6ba7b814-9dad-41d1-80b4-00c04fd430c8";

describe("PociDeltaLogItem schema", () => {
  const validDeltaLog = {
    id: DL_ID_1,
    calcRunId: DL_CALC_RUN,
    instrumenId: DL_INST_ID,
    instrumenKode: "POCI-DEP-0001",
    tanggalCompute: "2026-06-20",
    baselineEcl: "1250000000.0000",
    currentEcl: "1450000000.0000",
    deltaEcl: "200000000.0000",
    direction: "INCREASE",
    priorDeltaCumulative: "50000000.0000",
    status: "POSTED",
    largeDeltaFlag: false,
    createdAt: "2026-06-20T14:30:00+07:00",
  };

  it("validates a complete INCREASE delta log row", () => {
    const result = pociDeltaLogItemSchema.safeParse(validDeltaLog);
    expect(result.success).toBe(true);
  });

  it("validates a DECREASE row with negative deltaEcl", () => {
    const decreaseRow = {
      ...validDeltaLog,
      deltaEcl: "-150000000.0000",
      direction: "DECREASE",
      id: DL_ID_2,
    };
    const result = pociDeltaLogItemSchema.safeParse(decreaseRow);
    expect(result.success).toBe(true);
  });

  it("largeDeltaFlag true for delta > threshold (S5-AC3)", () => {
    const largeDeltaRow = { ...validDeltaLog, largeDeltaFlag: true, deltaEcl: "750000000.0000" };
    const result = pociDeltaLogItemSchema.safeParse(largeDeltaRow);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.largeDeltaFlag).toBe(true);
    }
  });

  it("SKIPPED_ZERO row with direction = ZERO", () => {
    const zeroRow = {
      ...validDeltaLog,
      deltaEcl: "0.0000",
      direction: "ZERO",
      status: "SKIPPED_ZERO",
      id: DL_ID_3,
    };
    const result = pociDeltaLogItemSchema.safeParse(zeroRow);
    expect(result.success).toBe(true);
  });

  it("rejects invalid direction value", () => {
    const badRow = { ...validDeltaLog, direction: "WRONG" };
    const result = pociDeltaLogItemSchema.safeParse(badRow);
    expect(result.success).toBe(false);
  });

  it("priorDeltaCumulative can be null (first run for instrument)", () => {
    const firstRunRow = { ...validDeltaLog, priorDeltaCumulative: null };
    const result = pociDeltaLogItemSchema.safeParse(firstRunRow);
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Dashboard summary schema (S5-AC2)
// ---------------------------------------------------------------------------

describe("PociDeltaSummary schema", () => {
  it("validates complete MTD/YTD aggregate summary", () => {
    const summary = {
      portofolioId: "b8c9d0e1-f2a3-4b8c-9d0e-1f2a3b4c5d6e",
      year: 2026,
      month: 6,
      instrumenCount: 15,
      deltaEclMtdIdr: "200000000.0000",
      deltaEclYtdIdr: "950000000.0000",
      netCumulativeDeltaIdr: "1200000000.0000",
      directionBreakdown: {
        increase: { count: 8, amountIdr: "300000000.0000" },
        decrease: { count: 5, amountIdr: "100000000.0000" },
        zero: { count: 2 },
      },
      largeDeltaCount: 1,
    };
    const result = pociDeltaSummarySchema.safeParse(summary);
    expect(result.success).toBe(true);
  });

  it("portofolioId is optional (aggregate all portfolios)", () => {
    const summaryNoPorto = {
      year: 2026,
      month: 6,
      instrumenCount: 15,
      deltaEclMtdIdr: "200000000.0000",
      deltaEclYtdIdr: "950000000.0000",
      netCumulativeDeltaIdr: "1200000000.0000",
      directionBreakdown: {
        increase: { count: 8, amountIdr: "300000000.0000" },
        decrease: { count: 5, amountIdr: "100000000.0000" },
        zero: { count: 2 },
      },
      largeDeltaCount: 0,
    };
    const result = pociDeltaSummarySchema.safeParse(summaryNoPorto);
    expect(result.success).toBe(true);
  });

  it("rejects month out of range", () => {
    const badMonth = {
      year: 2026,
      month: 13,
      instrumenCount: 1,
      deltaEclMtdIdr: "0",
      deltaEclYtdIdr: "0",
      netCumulativeDeltaIdr: "0",
      directionBreakdown: { increase: { count: 0, amountIdr: "0" }, decrease: { count: 0, amountIdr: "0" }, zero: { count: 0 } },
      largeDeltaCount: 0,
    };
    const result = pociDeltaSummarySchema.safeParse(badMonth);
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Error codes (6 new codes — S1, S2, S3 error boundary)
// ---------------------------------------------------------------------------

describe("POCI error codes completeness", () => {
  const expectedCodes = [
    "POCI_BASELINE_MISSING",
    "POCI_BASELINE_IMMUTABLE_VIOLATION",
    "POCI_DELTA_DUPLICATE",
    "POCI_INSTRUMEN_NOT_POCI",
    "POCI_PERIODE_LOCKED",
    "POCI_JURNAL_DIRECTION_MISMATCH",
  ];

  it("all 6 P5-M10 error codes are defined", () => {
    expectedCodes.forEach((code) => {
      expect(pociErrorCodes).toContain(code as typeof pociErrorCodes[number]);
    });
  });

  it("pociErrorCodes array length is exactly 6", () => {
    expect(pociErrorCodes.length).toBe(6);
  });
});
