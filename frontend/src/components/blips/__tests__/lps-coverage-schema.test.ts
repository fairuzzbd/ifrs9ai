/**
 * Unit tests for lps-coverage Zod schema helpers (fix #25).
 *
 * Tests verify:
 * - decimalPositiveString refinement uses no parseFloat (string-based check)
 * - formatIDR is pure string manipulation — no parseFloat
 * - Edge cases: zero, negative, very large IDR, 4-decimal precision
 */

import { describe, it, expect } from "vitest";
import {
  lpsCoverageCreateSchema,
  formatIDR,
} from "@/lib/schemas/lps-coverage.schema";

// ─── decimalPositiveString (fix #25 — no parseFloat) ─────────────────────────

describe("decimalPositiveString (coverageAmount)", () => {
  function validate(val: string) {
    return lpsCoverageCreateSchema.safeParse({
      coverageAmount: val,
      periodeBerlakuDari: "2026-01-01",
    });
  }

  it("accepts the default 2-billion IDR value", () => {
    expect(validate("2000000000.0000").success).toBe(true);
  });

  it("accepts whole number without decimal", () => {
    expect(validate("1").success).toBe(true);
    expect(validate("100000000").success).toBe(true);
  });

  it("accepts value with 1-4 decimal places", () => {
    expect(validate("1500000000.5").success).toBe(true);
    expect(validate("1500000000.50").success).toBe(true);
    expect(validate("1500000000.500").success).toBe(true);
    expect(validate("1500000000.5000").success).toBe(true);
  });

  it("rejects zero as string '0'", () => {
    expect(validate("0").success).toBe(false);
  });

  it("rejects zero with decimals '0.0000'", () => {
    expect(validate("0.0000").success).toBe(false);
  });

  it("rejects zero with decimals '0.00'", () => {
    expect(validate("0.00").success).toBe(false);
  });

  it("rejects negative values (fail regex)", () => {
    expect(validate("-1").success).toBe(false);
    expect(validate("-1000000").success).toBe(false);
  });

  it("rejects non-numeric strings", () => {
    expect(validate("abc").success).toBe(false);
    expect(validate("1e9").success).toBe(false);  // scientific notation rejected
  });

  it("rejects empty string", () => {
    expect(validate("").success).toBe(false);
  });

  it("rejects > 4 decimal places", () => {
    expect(validate("1000.00001").success).toBe(false);
  });
});

// ─── formatIDR (fix #25 — pure string manipulation) ──────────────────────────

describe("formatIDR", () => {
  it("formats 2 billion IDR correctly", () => {
    expect(formatIDR("2000000000.0000")).toBe("Rp 2.000.000.000,00");
  });

  it("formats a round value without decimal input", () => {
    expect(formatIDR("1000000")).toBe("Rp 1.000.000,00");
  });

  it("formats a value with exactly 2 decimal places", () => {
    expect(formatIDR("1500000000.50")).toBe("Rp 1.500.000.000,50");
  });

  it("truncates fractional display to 2 decimals (id-ID convention)", () => {
    // 4-decimal stored, displayed as 2
    expect(formatIDR("999999999.1234")).toBe("Rp 999.999.999,12");
  });

  it("pads fractional to 2 decimal places when only 1 given", () => {
    expect(formatIDR("100.5")).toBe("Rp 100,50");
  });

  it("returns original string for invalid input", () => {
    expect(formatIDR("not-a-number")).toBe("not-a-number");
    expect(formatIDR("")).toBe("");
  });

  it("handles single-digit values", () => {
    expect(formatIDR("1")).toBe("Rp 1,00");
  });

  it("does not use parseFloat (no float64 precision loss for large values)", () => {
    // 9_007_199_254_740_993 is beyond Number.MAX_SAFE_INTEGER — parseFloat would
    // truncate it. String manipulation preserves it exactly.
    const bigVal = "9007199254740993.0000";
    const result = formatIDR(bigVal);
    // Should contain the full integer part untouched
    expect(result).toContain("9.007.199.254.740.993");
  });
});
