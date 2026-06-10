/**
 * Vitest unit tests for portofolio Zod schema validation.
 *
 * Covers:
 * - kodePortofolio: regex ^[A-Z0-9_]{1,20}$
 * - bmCategoryDefault: enum HTC | HTCS | OTHER
 * - tujuanPengelolaan: min 10, max 2000
 * - nama: min 3, max 200
 * - optional fields: benchmark, kompensasiManagerBasis, periodeReviewTerakhir
 * - aktifFlag: boolean
 *
 * Happy path and one validation-fail path per critical field.
 */

import { describe, it, expect } from "vitest";
import { portofolioCreateSchema } from "@/lib/schemas/portofolio.schema";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function validBase() {
  return {
    kodePortofolio: "BOND_HTM_IDR",
    nama: "Obligasi Hold-to-Maturity IDR",
    tujuanPengelolaan:
      "Portofolio ini dikelola dengan tujuan memegang obligasi hingga jatuh tempo.",
    bmCategoryDefault: "HTC" as const,
    benchmark: null,
    kompensasiManagerBasis: null,
    periodeReviewTerakhir: null,
    aktifFlag: true,
  };
}

function safeParse(overrides: Record<string, unknown>) {
  return portofolioCreateSchema.safeParse({ ...validBase(), ...overrides });
}

// ─── kodePortofolio ───────────────────────────────────────────────────────────

describe("portofolioCreateSchema — kodePortofolio", () => {
  it("accepts valid uppercase+underscore code", () => {
    expect(safeParse({ kodePortofolio: "BOND_HTM_IDR" }).success).toBe(true);
  });

  it("accepts short single-word code", () => {
    expect(safeParse({ kodePortofolio: "EQUITY" }).success).toBe(true);
  });

  it("accepts code with digits", () => {
    expect(safeParse({ kodePortofolio: "BOND2026" }).success).toBe(true);
  });

  it("accepts exactly 20 characters", () => {
    expect(safeParse({ kodePortofolio: "A".repeat(20) }).success).toBe(true);
  });

  it("rejects lowercase letters", () => {
    const result = safeParse({ kodePortofolio: "bond_htm" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("kapital");
    }
  });

  it("rejects 21-character code (above max)", () => {
    const result = safeParse({ kodePortofolio: "A".repeat(21) });
    expect(result.success).toBe(false);
  });

  it("rejects empty code", () => {
    const result = safeParse({ kodePortofolio: "" });
    expect(result.success).toBe(false);
  });

  it("rejects code with spaces", () => {
    const result = safeParse({ kodePortofolio: "BOND HTM" });
    expect(result.success).toBe(false);
  });

  it("rejects code with hyphen", () => {
    const result = safeParse({ kodePortofolio: "BOND-HTM" });
    expect(result.success).toBe(false);
  });
});

// ─── bmCategoryDefault ────────────────────────────────────────────────────────

describe("portofolioCreateSchema — bmCategoryDefault", () => {
  it("accepts HTC", () => {
    expect(safeParse({ bmCategoryDefault: "HTC" }).success).toBe(true);
  });

  it("accepts HTCS", () => {
    expect(safeParse({ bmCategoryDefault: "HTCS" }).success).toBe(true);
  });

  it("accepts OTHER", () => {
    expect(safeParse({ bmCategoryDefault: "OTHER" }).success).toBe(true);
  });

  it("rejects lowercase htc", () => {
    const result = safeParse({ bmCategoryDefault: "htc" });
    expect(result.success).toBe(false);
  });

  it("rejects invalid FVTPL (not an allowed BM enum)", () => {
    const result = safeParse({ bmCategoryDefault: "FVTPL" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("Business Model");
    }
  });

  it("rejects empty string", () => {
    const result = safeParse({ bmCategoryDefault: "" });
    expect(result.success).toBe(false);
  });
});

// ─── tujuanPengelolaan ────────────────────────────────────────────────────────

describe("portofolioCreateSchema — tujuanPengelolaan", () => {
  it("accepts 10-character tujuan", () => {
    expect(safeParse({ tujuanPengelolaan: "A".repeat(10) }).success).toBe(true);
  });

  it("accepts 2000-character tujuan", () => {
    expect(
      safeParse({ tujuanPengelolaan: "A".repeat(2000) }).success,
    ).toBe(true);
  });

  it("rejects 9-character tujuan (below min)", () => {
    const result = safeParse({ tujuanPengelolaan: "A".repeat(9) });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("10");
    }
  });

  it("rejects 2001-character tujuan (above max)", () => {
    const result = safeParse({ tujuanPengelolaan: "A".repeat(2001) });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("2000");
    }
  });

  it("rejects empty string", () => {
    const result = safeParse({ tujuanPengelolaan: "" });
    expect(result.success).toBe(false);
  });
});

// ─── nama ─────────────────────────────────────────────────────────────────────

describe("portofolioCreateSchema — nama", () => {
  it("accepts 3-character nama", () => {
    expect(safeParse({ nama: "ABC" }).success).toBe(true);
  });

  it("accepts 200-character nama", () => {
    expect(safeParse({ nama: "A".repeat(200) }).success).toBe(true);
  });

  it("rejects 2-character nama (below min)", () => {
    const result = safeParse({ nama: "AB" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("3");
    }
  });

  it("rejects 201-character nama (above max)", () => {
    const result = safeParse({ nama: "A".repeat(201) });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("200");
    }
  });
});

// ─── optional fields ──────────────────────────────────────────────────────────

describe("portofolioCreateSchema — optional fields", () => {
  it("accepts null benchmark", () => {
    expect(safeParse({ benchmark: null }).success).toBe(true);
  });

  it("accepts benchmark string", () => {
    expect(safeParse({ benchmark: "IndoBeX Bond Index" }).success).toBe(true);
  });

  it("rejects benchmark over 500 chars", () => {
    const result = safeParse({ benchmark: "A".repeat(501) });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("500");
    }
  });

  it("accepts null kompensasiManagerBasis", () => {
    expect(safeParse({ kompensasiManagerBasis: null }).success).toBe(true);
  });

  it("accepts null periodeReviewTerakhir", () => {
    expect(safeParse({ periodeReviewTerakhir: null }).success).toBe(true);
  });

  it("accepts valid periodeReviewTerakhir date", () => {
    expect(
      safeParse({ periodeReviewTerakhir: "2026-03-31" }).success,
    ).toBe(true);
  });

  it("rejects periodeReviewTerakhir with invalid format DD/MM/YYYY", () => {
    const result = safeParse({ periodeReviewTerakhir: "31/03/2026" });
    expect(result.success).toBe(false);
  });
});

// ─── aktifFlag ────────────────────────────────────────────────────────────────

describe("portofolioCreateSchema — aktifFlag", () => {
  it("accepts true", () => {
    expect(safeParse({ aktifFlag: true }).success).toBe(true);
  });

  it("accepts false", () => {
    expect(safeParse({ aktifFlag: false }).success).toBe(true);
  });

  it("rejects string 'true'", () => {
    // Zod strict boolean should reject string
    const result = portofolioCreateSchema.safeParse({
      ...validBase(),
      aktifFlag: "true",
    });
    expect(result.success).toBe(false);
  });
});

// ─── happy path ───────────────────────────────────────────────────────────────

describe("portofolioCreateSchema — happy path", () => {
  it("passes with all required fields and no optional fields", () => {
    const result = safeParse({});
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.kodePortofolio).toBe("BOND_HTM_IDR");
      expect(result.data.bmCategoryDefault).toBe("HTC");
      expect(result.data.aktifFlag).toBe(true);
    }
  });

  it("passes with all fields populated", () => {
    const result = safeParse({
      kodePortofolio: "EQUITY_FVTPL",
      nama: "Portofolio Saham FVTPL",
      tujuanPengelolaan:
        "Portofolio saham yang diperdagangkan aktif dengan tujuan mengambil keuntungan jangka pendek.",
      bmCategoryDefault: "OTHER",
      benchmark: "IDX Composite",
      kompensasiManagerBasis: "Performance fee 20% above hurdle",
      periodeReviewTerakhir: "2026-03-31",
      aktifFlag: true,
    });
    expect(result.success).toBe(true);
  });
});
