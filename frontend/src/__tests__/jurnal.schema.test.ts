/**
 * Vitest tests for P5-M2 Jurnal Engine Zod schemas.
 * Happy paths + validation-fail paths per story AC.
 */
import { describe, it, expect } from "vitest";
import {
  mappingHeaderCreateSchema,
  resolverRequestSchema,
  manualPostSchema,
  rejectMappingSchema,
  isRegulatedCode,
  REGULATED_EVENT_CODES,
  KLASIFIKASI_COMPATIBILITY,
  EVENT_CODE_METADATA,
  EVENT_CODE_GROUPS,
  EVENT_CODE_LIST,
} from "@/lib/schemas/jurnal.schema";

// ---------------------------------------------------------------------------
// isRegulatedCode
// ---------------------------------------------------------------------------

describe("isRegulatedCode", () => {
  it("returns true for ECL_PEMBENTUKAN (regulated)", () => {
    expect(isRegulatedCode("ECL_PEMBENTUKAN")).toBe(true);
  });

  it("returns true for MTM_FVTPL (regulated)", () => {
    expect(isRegulatedCode("MTM_FVTPL")).toBe(true);
  });

  it("returns false for PENEMPATAN (operational)", () => {
    expect(isRegulatedCode("PENEMPATAN")).toBe(false);
  });

  it("returns false for AKRUAL_BUNGA (operational)", () => {
    expect(isRegulatedCode("AKRUAL_BUNGA")).toBe(false);
  });

  it("REGULATED_EVENT_CODES contains exactly 13 entries", () => {
    expect(REGULATED_EVENT_CODES.size).toBe(13);
  });

  it("EVENT_CODE_LIST has 27 codes", () => {
    expect(EVENT_CODE_LIST.length).toBe(27);
  });
});

// ---------------------------------------------------------------------------
// KLASIFIKASI_COMPATIBILITY matrix
// ---------------------------------------------------------------------------

describe("KLASIFIKASI_COMPATIBILITY", () => {
  it("MTM_FVTPL only allows FVTPL", () => {
    expect(KLASIFIKASI_COMPATIBILITY["MTM_FVTPL"]).toEqual(["FVTPL"]);
  });

  it("MTM_FVOCI only allows FVOCI", () => {
    expect(KLASIFIKASI_COMPATIBILITY["MTM_FVOCI"]).toEqual(["FVOCI"]);
  });

  it("ECL_PEMBENTUKAN allows AC, FVOCI, POCI but not FVTPL", () => {
    const allowed = KLASIFIKASI_COMPATIBILITY["ECL_PEMBENTUKAN"];
    expect(allowed).toContain("AC");
    expect(allowed).toContain("FVOCI");
    expect(allowed).toContain("POCI");
    expect(allowed).not.toContain("FVTPL");
    expect(allowed).not.toContain("FVOCI_ELECTION");
  });

  it("POCI_DELTA_ECL only allows POCI", () => {
    expect(KLASIFIKASI_COMPATIBILITY["POCI_DELTA_ECL"]).toEqual(["POCI"]);
  });

  it("PENEMPATAN allows all 5 klasifikasi", () => {
    const allowed = KLASIFIKASI_COMPATIBILITY["PENEMPATAN"];
    expect(allowed.length).toBe(5);
  });

  it("all 27 event codes have a compatibility entry", () => {
    for (const code of EVENT_CODE_LIST) {
      expect(KLASIFIKASI_COMPATIBILITY[code]).toBeDefined();
    }
  });
});

// ---------------------------------------------------------------------------
// EVENT_CODE_METADATA
// ---------------------------------------------------------------------------

describe("EVENT_CODE_METADATA", () => {
  it("has 27 entries", () => {
    expect(EVENT_CODE_METADATA.length).toBe(27);
  });

  it("regulated codes have workflowPath = 6-eyes", () => {
    const regulatedMeta = EVENT_CODE_METADATA.filter((m) => m.isRegulated);
    expect(regulatedMeta.length).toBe(13);
    for (const m of regulatedMeta) {
      expect(m.workflowPath).toBe("6-eyes");
    }
  });

  it("operational codes have workflowPath = 4-eyes", () => {
    const operationalMeta = EVENT_CODE_METADATA.filter((m) => !m.isRegulated);
    expect(operationalMeta.length).toBe(14);
    for (const m of operationalMeta) {
      expect(m.workflowPath).toBe("4-eyes");
    }
  });

  it("EVENT_CODE_GROUPS has 8 groups", () => {
    expect(EVENT_CODE_GROUPS.length).toBe(8);
  });
});

// ---------------------------------------------------------------------------
// mappingHeaderCreateSchema
// ---------------------------------------------------------------------------

describe("mappingHeaderCreateSchema", () => {
  const validDetailRows = [
    {
      urutan: 1,
      dkIndicator: "DEBIT",
      kodeAkunId: "akun-001",
      sumberAmount: "nominal_idr",
      multiplier: "1.0000",
    },
    {
      urutan: 2,
      dkIndicator: "KREDIT",
      kodeAkunId: "akun-002",
      sumberAmount: "nominal_idr",
      multiplier: "1.0000",
    },
  ];

  const validInput = {
    eventCode: "PENEMPATAN",
    namaEvent: "Penempatan Instrumen",
    kategoriEvent: "PENEMPATAN",
    triggerSource: "SYSTEM_JOB",
    detailRows: validDetailRows,
  };

  it("accepts a valid mapping header", () => {
    const result = mappingHeaderCreateSchema.safeParse(validInput);
    expect(result.success).toBe(true);
  });

  it("accepts null klasifikasiBerlaku (all klasifikasi)", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      klasifikasiBerlaku: null,
    });
    expect(result.success).toBe(true);
  });

  it("accepts array of klasifikasi", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      klasifikasiBerlaku: ["AC", "FVOCI"],
    });
    expect(result.success).toBe(true);
  });

  it("rejects empty eventCode", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      eventCode: "",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const paths = result.error.issues.map((i) => i.path.join("."));
      expect(paths).toContain("eventCode");
    }
  });

  it("rejects eventCode with lowercase", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      eventCode: "penempatan",
    });
    expect(result.success).toBe(false);
  });

  it("rejects when only DEBIT rows (no KREDIT)", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      detailRows: [
        { urutan: 1, dkIndicator: "DEBIT", kodeAkunId: "a", sumberAmount: "nominal_idr", multiplier: "1.0000" },
        { urutan: 2, dkIndicator: "DEBIT", kodeAkunId: "b", sumberAmount: "nominal_idr", multiplier: "1.0000" },
      ],
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const paths = result.error.issues.map((i) => i.path.join("."));
      expect(paths).toContain("detailRows");
    }
  });

  it("rejects when fewer than 2 detail rows", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      detailRows: [
        { urutan: 1, dkIndicator: "DEBIT", kodeAkunId: "a", sumberAmount: "nominal_idr", multiplier: "1.0000" },
      ],
    });
    expect(result.success).toBe(false);
  });

  it("rejects multiplier with 5+ decimal places", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      detailRows: [
        { urutan: 1, dkIndicator: "DEBIT", kodeAkunId: "a", sumberAmount: "nominal_idr", multiplier: "1.00000" },
        { urutan: 2, dkIndicator: "KREDIT", kodeAkunId: "b", sumberAmount: "nominal_idr", multiplier: "1.0000" },
      ],
    });
    expect(result.success).toBe(false);
  });

  it("rejects missing kodeAkunId", () => {
    const result = mappingHeaderCreateSchema.safeParse({
      ...validInput,
      detailRows: [
        { urutan: 1, dkIndicator: "DEBIT", kodeAkunId: "", sumberAmount: "nominal_idr", multiplier: "1.0000" },
        { urutan: 2, dkIndicator: "KREDIT", kodeAkunId: "b", sumberAmount: "nominal_idr", multiplier: "1.0000" },
      ],
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// resolverRequestSchema
// ---------------------------------------------------------------------------

describe("resolverRequestSchema", () => {
  const validRequest = {
    eventCode: "ECL_PEMBENTUKAN",
    klasifikasiPsak71: "AC",
    periodeId: "2026-06-01",
    amountIdr: "1000000.0000",
    currency: "IDR",
    fxRate: "1.00000000",
  };

  it("accepts a valid resolver request", () => {
    const result = resolverRequestSchema.safeParse(validRequest);
    expect(result.success).toBe(true);
  });

  it("rejects zero amount", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      amountIdr: "0.0000",
    });
    expect(result.success).toBe(false);
  });

  it("rejects negative amount via superRefine", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      amountIdr: "-500.0000",
    });
    expect(result.success).toBe(false);
  });

  it("rejects invalid amount format (text)", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      amountIdr: "abc",
    });
    expect(result.success).toBe(false);
  });

  it("rejects empty eventCode", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      eventCode: "",
    });
    expect(result.success).toBe(false);
  });

  it("rejects invalid klasifikasiPsak71", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      klasifikasiPsak71: "UNKNOWN_KLASIFIKASI",
    });
    expect(result.success).toBe(false);
  });

  it("accepts optional instrumenId as null", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      instrumenId: null,
    });
    expect(result.success).toBe(true);
  });

  it("rejects instrumenId that is not a UUID", () => {
    const result = resolverRequestSchema.safeParse({
      ...validRequest,
      instrumenId: "not-a-uuid",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// manualPostSchema
// ---------------------------------------------------------------------------

describe("manualPostSchema", () => {
  const validManualPost = {
    eventCode: "PERIODE_ADJUSTMENT",
    periodeId: "2026-06-01",
    amountIdr: "500000.0000",
    narasi: "Koreksi penyesuaian akhir periode",
  };

  it("accepts a valid manual post request", () => {
    const result = manualPostSchema.safeParse(validManualPost);
    expect(result.success).toBe(true);
  });

  it("accepts CORRECTION_PERIODE_CLOSED event code", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      eventCode: "CORRECTION_PERIODE_CLOSED",
    });
    expect(result.success).toBe(true);
  });

  it("rejects unsupported event code for manual post", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      eventCode: "ECL_PEMBENTUKAN",
    });
    expect(result.success).toBe(false);
  });

  it("rejects empty narasi", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      narasi: "",
    });
    expect(result.success).toBe(false);
  });

  it("rejects narasi over 500 chars", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      narasi: "a".repeat(501),
    });
    expect(result.success).toBe(false);
  });

  it("rejects zero amountIdr", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      amountIdr: "0.0000",
    });
    expect(result.success).toBe(false);
  });

  it("accepts optional dokumenDocId as null", () => {
    const result = manualPostSchema.safeParse({
      ...validManualPost,
      dokumenDocId: null,
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// rejectMappingSchema
// ---------------------------------------------------------------------------

describe("rejectMappingSchema", () => {
  it("accepts a valid reject reason", () => {
    const result = rejectMappingSchema.safeParse({
      rejectReason: "Template ini tidak memiliki pasangan DEBIT/KREDIT yang benar.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(true);
  });

  it("rejects reason shorter than 30 chars", () => {
    const result = rejectMappingSchema.safeParse({
      rejectReason: "Salah",
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const paths = result.error.issues.map((i) => i.path.join("."));
      expect(paths).toContain("rejectReason");
    }
  });

  it("rejects exactly 29 chars", () => {
    const result = rejectMappingSchema.safeParse({
      rejectReason: "a".repeat(29),
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(false);
  });

  it("accepts exactly 30 chars", () => {
    const result = rejectMappingSchema.safeParse({
      rejectReason: "a".repeat(30),
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(true);
  });
});
