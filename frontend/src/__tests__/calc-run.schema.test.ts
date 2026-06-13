/**
 * Vitest unit tests for calc-run.schema.ts (P4-M10)
 */

import { describe, it, expect } from "vitest";
import {
  calcRunSchema,
  calcRunStatusEnum,
  createCalcRunSchema,
  requestSealSchema,
  approveSealSchema,
  rejectSealSchema,
  cancelCalcRunSchema,
} from "@/lib/schemas/calc-run.schema";

// ---------------------------------------------------------------------------
// CalcRunStatus enum
// ---------------------------------------------------------------------------

describe("calcRunStatusEnum", () => {
  it("accepts all 8 valid status values", () => {
    const statuses = [
      "DRAFT",
      "IN_PROGRESS",
      "COMPLETED",
      "COMPLETED_WITH_ERRORS",
      "SEAL_REQUESTED",
      "SEALED",
      "SEAL_REJECTED",
      "CANCELLED",
    ] as const;

    for (const s of statuses) {
      expect(calcRunStatusEnum.safeParse(s).success).toBe(true);
    }
  });

  it("rejects unknown status", () => {
    expect(calcRunStatusEnum.safeParse("UNKNOWN").success).toBe(false);
    expect(calcRunStatusEnum.safeParse("").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// CalcRun schema
// ---------------------------------------------------------------------------

describe("calcRunSchema", () => {
  it("parses a minimal valid DRAFT calc run", () => {
    const input = {
      id: "CR-2026-06-001",
      periodeId: "JUNI-2026",
      evaluationDate: "2026-06-30",
      status: "DRAFT",
      errorCount: 0,
      skippedFvtplCount: 0,
      createdBy: "user-uuid-001",
      createdAt: "2026-06-13T10:00:00+07:00",
    };
    const result = calcRunSchema.safeParse(input);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("DRAFT");
      expect(result.data.errorCount).toBe(0);
      expect(result.data.totalInstrumen).toBeUndefined();
    }
  });

  it("parses a SEALED calc run with seal info", () => {
    const input = {
      id: "CR-2026-05-001",
      periodeId: "MEI-2026",
      evaluationDate: "2026-05-31",
      status: "SEALED",
      errorCount: 0,
      skippedFvtplCount: 5,
      createdBy: "user-uuid-002",
      createdAt: "2026-05-31T10:00:00+07:00",
      sealInfo: {
        sealedAt: "2026-06-01T09:00:00+07:00",
        sealApprovedBy: "alco-uuid-001",
        sealSignature1: "abc123def456789012345678",
      },
    };
    const result = calcRunSchema.safeParse(input);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("SEALED");
      expect(result.data.sealInfo?.sealSignature1).toBe("abc123def456789012345678");
    }
  });

  it("defaults errorCount to 0 when omitted", () => {
    const input = {
      id: "CR-X",
      periodeId: "JUNI-2026",
      evaluationDate: "2026-06-30",
      status: "COMPLETED",
      createdBy: "u",
      createdAt: "2026-06-13T00:00:00Z",
    };
    const result = calcRunSchema.safeParse(input);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.errorCount).toBe(0);
    }
  });
});

// ---------------------------------------------------------------------------
// Create form schema
// ---------------------------------------------------------------------------

describe("createCalcRunSchema", () => {
  it("accepts valid periode + evaluation date", () => {
    const result = createCalcRunSchema.safeParse({
      periodeId: "JUNI-2026",
      evaluationDate: "2026-06-30",
    });
    expect(result.success).toBe(true);
  });

  it("rejects empty periodeId", () => {
    const result = createCalcRunSchema.safeParse({
      periodeId: "",
      evaluationDate: "2026-06-30",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].path).toContain("periodeId");
    }
  });

  it("rejects invalid date format", () => {
    const result = createCalcRunSchema.safeParse({
      periodeId: "JUNI-2026",
      evaluationDate: "30-06-2026", // wrong format
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].path).toContain("evaluationDate");
    }
  });
});

// ---------------------------------------------------------------------------
// Seal forms
// ---------------------------------------------------------------------------

describe("requestSealSchema", () => {
  it("accepts comment >= 20 chars", () => {
    const result = requestSealSchema.safeParse({
      comment: "ECL sudah siap di-seal, sudah direvisi.",
    });
    expect(result.success).toBe(true);
  });

  it("rejects comment < 20 chars", () => {
    const result = requestSealSchema.safeParse({ comment: "Siap seal" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toMatch(/20/);
    }
  });
});

describe("approveSealSchema", () => {
  it("accepts comment >= 20 chars", () => {
    expect(
      approveSealSchema.safeParse({
        comment: "Parameter dan hasil ECL konsisten.",
      }).success,
    ).toBe(true);
  });

  it("rejects comment < 20 chars", () => {
    expect(approveSealSchema.safeParse({ comment: "OK" }).success).toBe(false);
  });
});

describe("rejectSealSchema", () => {
  it("accepts reject_reason >= 30 chars", () => {
    const result = rejectSealSchema.safeParse({
      rejectReason: "Parameter LGD pool perlu diverifikasi ulang sebelum seal.",
    });
    expect(result.success).toBe(true);
  });

  it("rejects reason < 30 chars", () => {
    const result = rejectSealSchema.safeParse({ rejectReason: "LGD salah" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toMatch(/30/);
    }
  });
});

describe("cancelCalcRunSchema", () => {
  it("accepts reason >= 30 chars", () => {
    const result = cancelCalcRunSchema.safeParse({
      cancelReason: "Data master PD belum final, perlu diperbarui ALCO terlebih dahulu.",
    });
    expect(result.success).toBe(true);
  });

  it("rejects reason < 30 chars", () => {
    const result = cancelCalcRunSchema.safeParse({ cancelReason: "Batal" });
    expect(result.success).toBe(false);
  });
});
