/**
 * Vitest tests for APP-B P5-M1 Penempatan Deposito Zod schemas.
 * Happy paths + validation-fail paths.
 */
import { describe, it, expect } from "vitest";
import {
  PenempatanCreateSchema,
  PenempatanUpdateSchema,
  RejectCommentSchema,
  TerminateRequestSchema,
  penempatanWorkflowStatusEnum,
  klasifikasiPsak71Enum,
  isFvtpl,
  PenempatanDepositoSchema,
  EirPreviewResultSchema,
  AuditTimelineEventSchema,
} from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// PenempatanCreateSchema
// ---------------------------------------------------------------------------

describe("PenempatanCreateSchema", () => {
  const validBase = {
    instrumenId: "550e8400-e29b-41d4-a716-446655440000",
    counterpartyBankId: "550e8400-e29b-41d4-a716-446655440001",
    periodeId: "550e8400-e29b-41d4-a716-446655440002",
    tanggalPenempatan: "2026-06-02",
    mataUangId: "550e8400-e29b-41d4-a716-446655440003",
    tenorBulan: 6,
    kuponPersen: "0.05250000",
    biayaTransaksiIdr: "0.0000",
  };

  it("accepts a valid create input", () => {
    const result = PenempatanCreateSchema.safeParse(validBase);
    expect(result.success).toBe(true);
  });

  it("accepts optional fields omitted", () => {
    const result = PenempatanCreateSchema.safeParse(validBase);
    expect(result.success).toBe(true);
  });

  it("rejects invalid instrumenId (not a UUID)", () => {
    const result = PenempatanCreateSchema.safeParse({
      ...validBase,
      instrumenId: "not-a-uuid",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("instrumenId");
    }
  });

  it("rejects tanggalPenempatan with wrong format", () => {
    const result = PenempatanCreateSchema.safeParse({
      ...validBase,
      tanggalPenempatan: "02/06/2026",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("tanggalPenempatan");
    }
  });

  it("rejects tenorBulan = 0", () => {
    const result = PenempatanCreateSchema.safeParse({
      ...validBase,
      tenorBulan: 0,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("tenorBulan");
    }
  });

  it("rejects negative kuponPersen", () => {
    const result = PenempatanCreateSchema.safeParse({
      ...validBase,
      kuponPersen: "-0.01",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("kuponPersen");
    }
  });

  it("allows zero kuponPersen (some deposits have 0%)", () => {
    const result = PenempatanCreateSchema.safeParse({
      ...validBase,
      kuponPersen: "0.00000000",
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// PenempatanUpdateSchema (DRAFT edit, optimistic locking)
// ---------------------------------------------------------------------------

describe("PenempatanUpdateSchema", () => {
  it("accepts partial update with rowVersion", () => {
    const result = PenempatanUpdateSchema.safeParse({
      rowVersion: 3,
      tenorBulan: 12,
    });
    expect(result.success).toBe(true);
  });

  it("rejects if rowVersion missing", () => {
    const result = PenempatanUpdateSchema.safeParse({
      tenorBulan: 12,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("rowVersion");
    }
  });

  it("rejects tenorBulan < 1 on update", () => {
    const result = PenempatanUpdateSchema.safeParse({
      rowVersion: 1,
      tenorBulan: 0,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("tenorBulan");
    }
  });
});

// ---------------------------------------------------------------------------
// RejectCommentSchema
// ---------------------------------------------------------------------------

describe("RejectCommentSchema", () => {
  const validReject = {
    comment: "Instrumen ini tidak memenuhi kriteria SPPI karena ada klausul opsi yang tidak wajar.",
    signatureMethod: "JWT_STANDARD" as const,
    attestChecked: true,
  };

  it("accepts valid reject comment", () => {
    const result = RejectCommentSchema.safeParse(validReject);
    expect(result.success).toBe(true);
  });

  it("rejects comment shorter than 30 chars", () => {
    const result = RejectCommentSchema.safeParse({
      ...validReject,
      comment: "Terlalu pendek",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("comment");
    }
  });

  it("rejects when attestChecked = false", () => {
    const result = RejectCommentSchema.safeParse({
      ...validReject,
      attestChecked: false,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("attestChecked");
    }
  });

  it("requires exactly 30 chars minimum", () => {
    const exactly30 = "a".repeat(30);
    const result = RejectCommentSchema.safeParse({
      ...validReject,
      comment: exactly30,
    });
    expect(result.success).toBe(true);
  });

  it("rejects 29 chars", () => {
    const twentyNine = "a".repeat(29);
    const result = RejectCommentSchema.safeParse({
      ...validReject,
      comment: twentyNine,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// TerminateRequestSchema
// ---------------------------------------------------------------------------

describe("TerminateRequestSchema", () => {
  const validTerminate = {
    terminateReason:
      "Kebutuhan likuiditas mendesak untuk membayar klaim CAT bond Oktober 2026. Dewan Komisaris menyetujui.",
    signatureMethod: "JWT_STANDARD" as const,
    attestChecked: true,
  };

  it("accepts valid terminate request", () => {
    const result = TerminateRequestSchema.safeParse(validTerminate);
    expect(result.success).toBe(true);
  });

  it("rejects reason shorter than 30 chars", () => {
    const result = TerminateRequestSchema.safeParse({
      ...validTerminate,
      terminateReason: "Pendek",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const fields = result.error.issues.map((i) => i.path.join("."));
      expect(fields).toContain("terminateReason");
    }
  });

  it("allows optional dokumenTerminasiId", () => {
    const result = TerminateRequestSchema.safeParse({
      ...validTerminate,
      dokumenTerminasiId: "550e8400-e29b-41d4-a716-446655440099",
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// penempatanWorkflowStatusEnum
// ---------------------------------------------------------------------------

describe("penempatanWorkflowStatusEnum", () => {
  const validStatuses = [
    "DRAFT",
    "PENDING_REVIEW",
    "PENDING_APPROVAL",
    "APPROVED_ACTIVE",
    "TERMINATION_PENDING_REVIEW",
    "TERMINATION_PENDING_APPROVAL",
    "TERMINATED",
    "MATURED",
    "CANCELLED",
  ];

  validStatuses.forEach((status) => {
    it(`accepts status: ${status}`, () => {
      expect(penempatanWorkflowStatusEnum.safeParse(status).success).toBe(true);
    });
  });

  it("rejects unknown status", () => {
    expect(penempatanWorkflowStatusEnum.safeParse("INVALID_STATUS").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// klasifikasiPsak71Enum
// ---------------------------------------------------------------------------

describe("klasifikasiPsak71Enum", () => {
  ["AC", "FVOCI", "FVTPL", "FVOCI_ELECTION", "POCI"].forEach((k) => {
    it(`accepts klasifikasi: ${k}`, () => {
      expect(klasifikasiPsak71Enum.safeParse(k).success).toBe(true);
    });
  });

  it("rejects invalid klasifikasi", () => {
    expect(klasifikasiPsak71Enum.safeParse("AMORTISED_COST").success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// isFvtpl helper
// ---------------------------------------------------------------------------

describe("isFvtpl", () => {
  it("returns true for FVTPL", () => {
    expect(isFvtpl("FVTPL")).toBe(true);
  });

  it("returns true for FVOCI_ELECTION (irrevocable equity option)", () => {
    expect(isFvtpl("FVOCI_ELECTION")).toBe(true);
  });

  it("returns false for AC", () => {
    expect(isFvtpl("AC")).toBe(false);
  });

  it("returns false for FVOCI debt", () => {
    expect(isFvtpl("FVOCI")).toBe(false);
  });

  it("returns false for POCI", () => {
    expect(isFvtpl("POCI")).toBe(false);
  });

  it("returns false for null", () => {
    expect(isFvtpl(null)).toBe(false);
  });

  it("returns false for undefined", () => {
    expect(isFvtpl(undefined)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// PenempatanDepositoSchema (API response)
// ---------------------------------------------------------------------------

describe("PenempatanDepositoSchema", () => {
  const validDeposito = {
    id: "550e8400-e29b-41d4-a716-446655440000",
    kodeTransaksi: "DP-2026-0001",
    workflowStatus: "APPROVED_ACTIVE",
    instrumenId: "550e8400-e29b-41d4-a716-446655440001",
    counterpartyBankId: "550e8400-e29b-41d4-a716-446655440002",
    periodeId: "550e8400-e29b-41d4-a716-446655440003",
    tanggalPenempatan: "2026-06-02",
    tanggalJatuhTempo: "2026-12-02",
    nominalIdr: 5000000000.0,
    nominalFcy: null,
    mataUangId: "550e8400-e29b-41d4-a716-446655440004",
    tenorBulan: 6,
    kuponPersen: 0.0525,
    biayaTransaksiIdr: 0.0,
    nomorReferensiBankIn: null,
    settlementAccount: null,
    catatan: null,
    klasifikasiPsak71: "AC",
    eirAwal: 0.0527,
    carryingAmountAwal: 5000000000.0,
    makerId: "550e8400-e29b-41d4-a716-446655440010",
    reviewerId: "550e8400-e29b-41d4-a716-446655440011",
    approverId: "550e8400-e29b-41d4-a716-446655440012",
    reviewerSignedAt: "2026-06-02T10:00:00+07:00",
    approverSignedAt: "2026-06-02T11:00:00+07:00",
    reviewerSignatureHash: "abc123def456",
    approverSignatureHash: "def456ghi789",
    rejectReason: null,
    terminateReason: null,
    terminateReviewerId: null,
    terminateApproverId: null,
    terminatedAt: null,
    maturedAt: null,
    settlementBalanceHint: null,
    kursPenempatan: null,
    rowVersion: 3,
    createdAt: "2026-06-01T09:00:00+07:00",
    updatedAt: "2026-06-02T11:00:00+07:00",
  };

  it("accepts a fully populated response", () => {
    const result = PenempatanDepositoSchema.safeParse(validDeposito);
    expect(result.success).toBe(true);
  });

  it("rejects if id is not UUID", () => {
    const result = PenempatanDepositoSchema.safeParse({
      ...validDeposito,
      id: "not-a-uuid",
    });
    expect(result.success).toBe(false);
  });

  it("rejects unknown workflowStatus", () => {
    const result = PenempatanDepositoSchema.safeParse({
      ...validDeposito,
      workflowStatus: "PENDING",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// EirPreviewResultSchema
// ---------------------------------------------------------------------------

describe("EirPreviewResultSchema", () => {
  const validPreview = {
    eirAwalApprox: 0.0528,
    isApproximate: true,
    carryingAmountAwal: 5000000000.0,
    periodePreview: 6,
    info: "Estimasi berdasarkan cashflow kontraktual. EIR final dihitung setelah approve.",
    amortizationSchedule: [
      {
        periode: 1,
        tanggalAngsuran: "2026-07-02",
        angsuranBunga: 22000000.0,
        angsuranPokok: 0.0,
        carryingAmount: 5000000000.0,
      },
    ],
  };

  it("accepts valid EIR preview", () => {
    const result = EirPreviewResultSchema.safeParse(validPreview);
    expect(result.success).toBe(true);
  });

  it("accepts empty amortization schedule", () => {
    const result = EirPreviewResultSchema.safeParse({
      ...validPreview,
      amortizationSchedule: [],
    });
    expect(result.success).toBe(true);
  });

  it("accepts null eirAwalApprox (not yet computed)", () => {
    const result = EirPreviewResultSchema.safeParse({
      ...validPreview,
      eirAwalApprox: null,
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// AuditTimelineEventSchema
// ---------------------------------------------------------------------------

describe("AuditTimelineEventSchema", () => {
  const validEvent = {
    eventId: "550e8400-e29b-41d4-a716-446655440099",
    eventTime: "2026-06-02T10:00:00+07:00",
    actorUserId: "550e8400-e29b-41d4-a716-446655440010",
    actorUsername: "treasury.maker1",
    actorRole: "ROLE-MAKER-TR",
    action: "PENEMPATAN.SUBMIT",
    comment: "Siap untuk direview",
    signatureHash: null,
    beforeJsonb: null,
    afterJsonb: { workflowStatus: "PENDING_REVIEW" },
    ip: "10.0.0.1",
    traceId: "trace-abc123",
  };

  it("accepts valid audit event", () => {
    const result = AuditTimelineEventSchema.safeParse(validEvent);
    expect(result.success).toBe(true);
  });

  it("accepts null optional fields", () => {
    const result = AuditTimelineEventSchema.safeParse({
      ...validEvent,
      comment: null,
      ip: null,
      traceId: null,
      afterJsonb: null,
    });
    expect(result.success).toBe(true);
  });

  it("rejects if eventId is not UUID", () => {
    const result = AuditTimelineEventSchema.safeParse({
      ...validEvent,
      eventId: "not-a-uuid",
    });
    expect(result.success).toBe(false);
  });
});
