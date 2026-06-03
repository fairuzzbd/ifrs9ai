/**
 * Frontend unit tests for mata-uang Zod schema validation.
 *
 * TOOLCHAIN NOTE: Requires Vitest. See matauang-sod.test.ts for setup instructions.
 *
 * These tests are pure TypeScript (no DOM), runnable with just Vitest + Node.
 * They validate the Zod schema rules that mirror backend validation:
 * - kodeMataUang: exactly 3 uppercase ASCII letters (ISO 4217)
 * - decimalPlaces: 0..4 range
 * - tanggalMulaiAktif: YYYY-MM-DD, not in the future
 * - namaMataUang: 3..60 chars
 *
 * If a backend validation rule changes, the corresponding Zod schema and this
 * test MUST be updated in the same PR.
 */

import { describe, it, expect } from "vitest";
import { mataUangCreateSchema } from "@/lib/schemas/mata-uang.schema";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function validBase() {
  return {
    kodeMataUang: "GBP",
    namaMataUang: "Pound Sterling",
    simbol: "£",
    decimalPlaces: 2,
    sumberKursDefault: "BI_KURS_TENGAH" as const,
    frekuensiUpdate: "HARIAN" as const,
    tanggalMulaiAktif: "2020-01-01", // well in the past
    aktifFlag: true,
  };
}

function safeParse(overrides: Record<string, unknown>) {
  return mataUangCreateSchema.safeParse({ ...validBase(), ...overrides });
}

// ─── kodeMataUang ─────────────────────────────────────────────────────────────

describe("mataUangCreateSchema — kodeMataUang", () => {
  it("accepts valid 3-uppercase-letter code (GBP)", () => {
    expect(safeParse({}).success).toBe(true);
  });

  it("rejects lowercase code (gbp)", () => {
    const result = safeParse({ kodeMataUang: "gbp" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("ISO 4217");
    }
  });

  it("rejects 4-letter code (USDD)", () => {
    const result = safeParse({ kodeMataUang: "USDD" });
    expect(result.success).toBe(false);
  });

  it("rejects 2-letter code (US)", () => {
    const result = safeParse({ kodeMataUang: "US" });
    expect(result.success).toBe(false);
  });

  it("rejects empty code", () => {
    const result = safeParse({ kodeMataUang: "" });
    expect(result.success).toBe(false);
  });

  it("rejects numeric-start code (1DR)", () => {
    const result = safeParse({ kodeMataUang: "1DR" });
    expect(result.success).toBe(false);
  });

  it("accepts IDR", () => {
    expect(safeParse({ kodeMataUang: "IDR" }).success).toBe(true);
  });
});

// ─── decimalPlaces ────────────────────────────────────────────────────────────

describe("mataUangCreateSchema — decimalPlaces", () => {
  it("accepts 0", () => {
    expect(safeParse({ decimalPlaces: 0 }).success).toBe(true);
  });

  it("accepts 2", () => {
    expect(safeParse({ decimalPlaces: 2 }).success).toBe(true);
  });

  it("accepts 4", () => {
    expect(safeParse({ decimalPlaces: 4 }).success).toBe(true);
  });

  it("rejects -1 (below min)", () => {
    const result = safeParse({ decimalPlaces: -1 });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("0");
    }
  });

  it("rejects 5 (above max)", () => {
    const result = safeParse({ decimalPlaces: 5 });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("4");
    }
  });
});

// ─── tanggalMulaiAktif ────────────────────────────────────────────────────────

describe("mataUangCreateSchema — tanggalMulaiAktif", () => {
  it("accepts a past date", () => {
    expect(safeParse({ tanggalMulaiAktif: "2020-01-01" }).success).toBe(true);
  });

  it("accepts today's date", () => {
    const today = new Date().toISOString().split("T")[0];
    expect(safeParse({ tanggalMulaiAktif: today }).success).toBe(true);
  });

  it("rejects a future date", () => {
    const future = new Date(Date.now() + 86400 * 1000).toISOString().split("T")[0];
    const result = safeParse({ tanggalMulaiAktif: future });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("masa depan");
    }
  });

  it("rejects wrong format DD-MM-YYYY", () => {
    const result = safeParse({ tanggalMulaiAktif: "03-06-2026" });
    expect(result.success).toBe(false);
  });

  it("rejects wrong format YYYY/MM/DD", () => {
    const result = safeParse({ tanggalMulaiAktif: "2026/06/03" });
    expect(result.success).toBe(false);
  });
});

// ─── namaMataUang ─────────────────────────────────────────────────────────────

describe("mataUangCreateSchema — namaMataUang", () => {
  it("accepts 3-char name", () => {
    expect(safeParse({ namaMataUang: "ABC" }).success).toBe(true);
  });

  it("accepts 60-char name", () => {
    expect(safeParse({ namaMataUang: "A".repeat(60) }).success).toBe(true);
  });

  it("rejects 2-char name (below min)", () => {
    const result = safeParse({ namaMataUang: "AB" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("3");
    }
  });

  it("rejects 61-char name (above max)", () => {
    const result = safeParse({ namaMataUang: "A".repeat(61) });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("60");
    }
  });
});

// ─── sumberKursDefault enum ───────────────────────────────────────────────────

describe("mataUangCreateSchema — sumberKursDefault", () => {
  it("accepts BI_JISDOR", () => {
    expect(safeParse({ sumberKursDefault: "BI_JISDOR" }).success).toBe(true);
  });

  it("accepts BI_KURS_TENGAH", () => {
    expect(safeParse({ sumberKursDefault: "BI_KURS_TENGAH" }).success).toBe(true);
  });

  it("accepts INTERNAL", () => {
    expect(safeParse({ sumberKursDefault: "INTERNAL" }).success).toBe(true);
  });

  it("rejects invalid value", () => {
    const result = safeParse({ sumberKursDefault: "UNKNOWN" });
    expect(result.success).toBe(false);
  });
});
