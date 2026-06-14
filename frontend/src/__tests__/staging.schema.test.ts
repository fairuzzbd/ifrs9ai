/**
 * Vitest tests for APP-C Staging Zod schemas (P4-M9).
 */
import { describe, it, expect } from "vitest";
import {
  overrideSubmitFormSchema,
  dpdRecordFormSchema,
  stageHistoryRowSchema,
  stagingCurrentSchema,
  stagingOverrideProposalSchema,
} from "@/lib/schemas/staging.schema";

// ---------------------------------------------------------------------------
// overrideSubmitFormSchema
// ---------------------------------------------------------------------------

describe("overrideSubmitFormSchema", () => {
  const validBase = {
    instrumenId: "550e8400-e29b-41d4-a716-446655440000",
    stageTarget: "STAGE_2" as const,
    alasan: "Rating turun 3 notch dari AAA ke A, melewati threshold SICR (DEC-011).",
    periodeId: "550e8400-e29b-41d4-a716-446655440001",
  };

  it("accepts valid input", () => {
    const result = overrideSubmitFormSchema.safeParse(validBase);
    expect(result.success).toBe(true);
  });

  it("rejects invalid instrumenId UUID", () => {
    const result = overrideSubmitFormSchema.safeParse({
      ...validBase,
      instrumenId: "not-a-uuid",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path[0]);
      expect(fields).toContain("instrumenId");
    }
  });

  it("rejects alasan shorter than 20 chars", () => {
    const result = overrideSubmitFormSchema.safeParse({
      ...validBase,
      alasan: "terlalu pendek",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const alasanIssue = result.error.issues.find((i) => i.path[0] === "alasan");
      expect(alasanIssue).toBeDefined();
      expect(alasanIssue?.message).toContain("20");
    }
  });

  it("rejects invalid stageTarget value", () => {
    const result = overrideSubmitFormSchema.safeParse({
      ...validBase,
      stageTarget: "STAGE_4",
    });
    expect(result.success).toBe(false);
  });

  it("accepts optional dokumenPendukungId as UUID", () => {
    const result = overrideSubmitFormSchema.safeParse({
      ...validBase,
      dokumenPendukungId: "550e8400-e29b-41d4-a716-446655440002",
    });
    expect(result.success).toBe(true);
  });

  it("rejects dokumenPendukungId as invalid UUID", () => {
    const result = overrideSubmitFormSchema.safeParse({
      ...validBase,
      dokumenPendukungId: "not-a-uuid",
    });
    expect(result.success).toBe(false);
  });

  it("allows all three stage targets", () => {
    for (const stage of ["STAGE_1", "STAGE_2", "STAGE_3"] as const) {
      const result = overrideSubmitFormSchema.safeParse({ ...validBase, stageTarget: stage });
      expect(result.success).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------
// dpdRecordFormSchema
// ---------------------------------------------------------------------------

describe("dpdRecordFormSchema", () => {
  const validBase = {
    instrumenId: "550e8400-e29b-41d4-a716-446655440000",
    periode: "2026-06-01",
    dpdValue: 45,
  };

  it("accepts valid DPD record", () => {
    const result = dpdRecordFormSchema.safeParse(validBase);
    expect(result.success).toBe(true);
  });

  it("rejects negative DPD", () => {
    const result = dpdRecordFormSchema.safeParse({ ...validBase, dpdValue: -1 });
    expect(result.success).toBe(false);
    if (!result.success) {
      const issue = result.error.issues.find((i) => i.path[0] === "dpdValue");
      expect(issue).toBeDefined();
      expect(issue?.message).toMatch(/negatif/i);
    }
  });

  it("rejects non-integer DPD", () => {
    const result = dpdRecordFormSchema.safeParse({ ...validBase, dpdValue: 1.5 });
    expect(result.success).toBe(false);
  });

  it("accepts zero DPD (no days past due)", () => {
    const result = dpdRecordFormSchema.safeParse({ ...validBase, dpdValue: 0 });
    expect(result.success).toBe(true);
  });

  it("rejects missing periode", () => {
    const result = dpdRecordFormSchema.safeParse({ ...validBase, periode: "" });
    expect(result.success).toBe(false);
  });

  it("accepts catatan within 200 chars", () => {
    const result = dpdRecordFormSchema.safeParse({
      ...validBase,
      catatan: "Koreksi berdasarkan data sistem core banking per 2026-06-01.",
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// stageHistoryRowSchema
// ---------------------------------------------------------------------------

describe("stageHistoryRowSchema", () => {
  const validRow = {
    id: "550e8400-e29b-41d4-a716-446655440000",
    stageHistoryIdKode: "SH-001",
    instrumenId: "550e8400-e29b-41d4-a716-446655440001",
    tanggalMigrasi: "2026-06-01",
    stageSesudah: "STAGE_2" as const,
    triggerType: "RATING_DOWNGRADE" as const,
    statusApproval: "AUTO" as const,
    createdAt: "2026-06-01T10:00:00Z",
  };

  it("accepts valid stage history row", () => {
    const result = stageHistoryRowSchema.safeParse(validRow);
    expect(result.success).toBe(true);
  });

  it("accepts sicrEvidence with nested fields", () => {
    const result = stageHistoryRowSchema.safeParse({
      ...validRow,
      sicrEvidence: {
        triggerType: "RATING_DOWNGRADE_NOTCH",
        ratingBaseline: "AA",
        ratingCurrent: "BB",
        notchDelta: -5,
      },
    });
    expect(result.success).toBe(true);
  });

  it("rejects invalid triggerType", () => {
    const result = stageHistoryRowSchema.safeParse({
      ...validRow,
      triggerType: "UNKNOWN_TRIGGER",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// StagingOverrideProposal — field alignment checks
// ---------------------------------------------------------------------------

describe("stagingOverrideProposalSchema", () => {
  const validProposal = {
    id: "550e8400-e29b-41d4-a716-446655440000",
    instrumenId: "550e8400-e29b-41d4-a716-446655440001",
    stageFrom: "STAGE_1" as const,
    stageTo: "STAGE_2" as const,
    alasan: "Override manual approved by ALCO June 2026.",
    status: "PENDING_REVIEW" as const,
    makerId: "550e8400-e29b-41d4-a716-446655440002",
    periodeId: "550e8400-e29b-41d4-a716-446655440003",
    periodeAkhir: "2026-06-30",
    createdAt: "2026-06-01T10:00:00Z",
  };

  it("uses stageFrom/stageTo/alasan (not stageTarget/alasanOverride)", () => {
    const result = stagingOverrideProposalSchema.safeParse(validProposal);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.stageFrom).toBe("STAGE_1");
      expect(result.data.stageTo).toBe("STAGE_2");
      expect(result.data.alasan).toBe(validProposal.alasan);
    }
  });
});
