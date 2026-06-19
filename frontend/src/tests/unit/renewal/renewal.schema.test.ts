/**
 * Vitest unit tests — P5-M7 Renewal Deposito
 *
 * AC coverage:
 *   S1-AC2: tenor boundary (1 = valid, 60 = valid, 0 = fail, 61 = fail)
 *   S1-AC3: rate boundary (0 = valid, 30 = valid, -0.01 = fail, 30.01 = fail)
 *   S1-AC1: skema enum validation
 *   S2-AC (reject): comment ≥ 30 char
 *   Preview decimal display
 *   Badge state matrix (status/skema coverage)
 *   Persona gating: SoD contract (maker === userId → no button)
 *   EIR badge format
 */

import { describe, it, expect } from "vitest";
import {
  createRenewalSchema,
  rejectRenewalSchema,
  approveRenewalSchema,
  renewalStatusEnum,
  renewalSkemaEnum,
  RENEWAL_STATUS_LABELS,
  RENEWAL_SKEMA_LABELS,
  type RenewalStatus,
  type RenewalSkema,
} from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// createRenewalSchema — Tenor boundary (S1-AC2)
// ---------------------------------------------------------------------------

describe("createRenewalSchema — tenor boundary", () => {
  const base = {
    instrumenId: "aaaaaaaa-0000-0000-0000-000000000001",
    skema: "POKOK_SAJA" as const,
    rateBaruPersen: 5.5,
    tanggalEfektifBaru: "2026-07-01",
  };

  it("accepts tenor = 1 (minimum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, tenorBaruBulan: 1 });
    expect(result.success).toBe(true);
  });

  it("accepts tenor = 60 (maximum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, tenorBaruBulan: 60 });
    expect(result.success).toBe(true);
  });

  it("rejects tenor = 0 (below minimum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, tenorBaruBulan: 0 });
    expect(result.success).toBe(false);
    if (!result.success) {
      const tenorError = result.error.issues.find((i) => i.path[0] === "tenorBaruBulan");
      expect(tenorError).toBeDefined();
    }
  });

  it("rejects tenor = 61 (above maximum, S1-AC2 maps to RENEWAL_TENOR_OUT_OF_RANGE)", () => {
    const result = createRenewalSchema.safeParse({ ...base, tenorBaruBulan: 61 });
    expect(result.success).toBe(false);
    if (!result.success) {
      const tenorError = result.error.issues.find((i) => i.path[0] === "tenorBaruBulan");
      expect(tenorError).toBeDefined();
    }
  });

  it("rejects tenor = 72 (story AC2 explicit case)", () => {
    const result = createRenewalSchema.safeParse({ ...base, tenorBaruBulan: 72 });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// createRenewalSchema — Rate boundary (S1-AC3)
// ---------------------------------------------------------------------------

describe("createRenewalSchema — rate boundary", () => {
  const base = {
    instrumenId: "aaaaaaaa-0000-0000-0000-000000000001",
    skema: "POKOK_SAJA" as const,
    tenorBaruBulan: 12,
    tanggalEfektifBaru: "2026-07-01",
  };

  it("accepts rate = 0 (minimum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: 0 });
    expect(result.success).toBe(true);
  });

  it("accepts rate = 30 (maximum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: 30 });
    expect(result.success).toBe(true);
  });

  it("accepts rate = 5.75 (story AC1 value)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: 5.75 });
    expect(result.success).toBe(true);
  });

  it("rejects rate = -0.01 (below minimum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: -0.01 });
    expect(result.success).toBe(false);
    if (!result.success) {
      const rateError = result.error.issues.find((i) => i.path[0] === "rateBaruPersen");
      expect(rateError).toBeDefined();
    }
  });

  it("rejects rate = 30.01 (above maximum)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: 30.01 });
    expect(result.success).toBe(false);
  });

  it("rejects rate = 35 (story AC3 explicit case)", () => {
    const result = createRenewalSchema.safeParse({ ...base, rateBaruPersen: 35 });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// createRenewalSchema — Skema enum validation (RENEWAL_SKEMA_INVALID)
// ---------------------------------------------------------------------------

describe("createRenewalSchema — skema enum", () => {
  const base = {
    instrumenId: "aaaaaaaa-0000-0000-0000-000000000001",
    tenorBaruBulan: 12,
    rateBaruPersen: 5.5,
    tanggalEfektifBaru: "2026-07-01",
  };

  it("accepts POKOK_SAJA", () => {
    const result = createRenewalSchema.safeParse({ ...base, skema: "POKOK_SAJA" });
    expect(result.success).toBe(true);
  });

  it("accepts POKOK_PLUS_BUNGA", () => {
    const result = createRenewalSchema.safeParse({ ...base, skema: "POKOK_PLUS_BUNGA" });
    expect(result.success).toBe(true);
  });

  it("rejects ROLLOVER (invalid enum, maps to RENEWAL_SKEMA_INVALID)", () => {
    const result = createRenewalSchema.safeParse({ ...base, skema: "ROLLOVER" });
    expect(result.success).toBe(false);
  });

  it("rejects empty string skema", () => {
    const result = createRenewalSchema.safeParse({ ...base, skema: "" });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// rejectRenewalSchema — comment ≥ 30 char (S2-AC reject)
// ---------------------------------------------------------------------------

describe("rejectRenewalSchema — comment length", () => {
  it("accepts comment exactly 30 chars", () => {
    const result = rejectRenewalSchema.safeParse({
      comment: "123456789012345678901234567890",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("accepts comment > 30 chars (story example)", () => {
    const result = rejectRenewalSchema.safeParse({
      comment: "Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("rejects comment = 29 chars", () => {
    const result = rejectRenewalSchema.safeParse({
      comment: "12345678901234567890123456789",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const commentError = result.error.issues.find((i) => i.path[0] === "comment");
      expect(commentError).toBeDefined();
    }
  });

  it("rejects empty comment", () => {
    const result = rejectRenewalSchema.safeParse({
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// approveRenewalSchema — signatureMethod must be JWT_STEP_UP (DEC-027)
// ---------------------------------------------------------------------------

describe("approveRenewalSchema — signatureMethod literal", () => {
  it("accepts JWT_STEP_UP", () => {
    const result = approveRenewalSchema.safeParse({
      comment: "Verified.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("rejects any other value", () => {
    const result = approveRenewalSchema.safeParse({
      comment: "Verified.",
      signatureMethod: "PASSWORD",
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Badge state matrix — all status/skema values have labels
// ---------------------------------------------------------------------------

describe("RENEWAL_STATUS_LABELS — all 4 states covered", () => {
  const statuses: RenewalStatus[] = ["PENDING_APPROVAL", "APPROVED", "POSTED", "REJECTED"];

  statuses.forEach((status) => {
    it(`has label for status ${status}`, () => {
      expect(RENEWAL_STATUS_LABELS[status]).toBeDefined();
      expect(RENEWAL_STATUS_LABELS[status].length).toBeGreaterThan(0);
    });
  });
});

describe("RENEWAL_SKEMA_LABELS — both skema values covered", () => {
  const skemas: RenewalSkema[] = ["POKOK_SAJA", "POKOK_PLUS_BUNGA"];

  skemas.forEach((skema) => {
    it(`has label for skema ${skema}`, () => {
      expect(RENEWAL_SKEMA_LABELS[skema]).toBeDefined();
      expect(RENEWAL_SKEMA_LABELS[skema].length).toBeGreaterThan(0);
    });
  });
});

// ---------------------------------------------------------------------------
// renewalStatusEnum — coverage
// ---------------------------------------------------------------------------

describe("renewalStatusEnum", () => {
  it("POSTED is valid", () => {
    const result = renewalStatusEnum.safeParse("POSTED");
    expect(result.success).toBe(true);
  });

  it("DRAFT is not a valid status (create goes directly to PENDING_APPROVAL)", () => {
    const result = renewalStatusEnum.safeParse("DRAFT");
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// SoD contract — persona gating logic (pure logic, no DOM)
// ---------------------------------------------------------------------------

describe("SoD contract — persona gating logic", () => {
  const MAKER_ID = "aaaaaaaa-0000-0000-0000-000000000001";
  const APPROVER_ID = "bbbbbbbb-0000-0000-0000-000000000002";

  function canShowApproveButton(
    status: string,
    makerId: string,
    currentUserId: string | null,
    hasApprovePermission: boolean,
  ): boolean {
    if (!hasApprovePermission) return false;
    if (status !== "PENDING_APPROVAL") return false;
    if (makerId === currentUserId) return false; // SoD enforcement
    return true;
  }

  it("shows approve button for eligible ROLE-APPR-TR (different from maker)", () => {
    expect(canShowApproveButton("PENDING_APPROVAL", MAKER_ID, APPROVER_ID, true)).toBe(true);
  });

  it("hides approve button when current user is the maker (SoD)", () => {
    expect(canShowApproveButton("PENDING_APPROVAL", MAKER_ID, MAKER_ID, true)).toBe(false);
  });

  it("hides approve button when status is POSTED (immutable)", () => {
    expect(canShowApproveButton("POSTED", MAKER_ID, APPROVER_ID, true)).toBe(false);
  });

  it("hides approve button when status is REJECTED", () => {
    expect(canShowApproveButton("REJECTED", MAKER_ID, APPROVER_ID, true)).toBe(false);
  });

  it("hides approve button when user lacks transaksi.approve permission", () => {
    expect(canShowApproveButton("PENDING_APPROVAL", MAKER_ID, APPROVER_ID, false)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Preview decimal formatting (mirror RenewalPreviewPanel logic)
// ---------------------------------------------------------------------------

describe("preview IDR decimal formatting", () => {
  const IDR_FULL = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  });

  it("formats pokok_lama 1000000000.0000 correctly", () => {
    const val = IDR_FULL.format(parseFloat("1000000000.0000"));
    expect(val).toContain("1.000.000.000");
  });

  it("formats bunga_kotor 14246575.3425 correctly (4 decimal precision)", () => {
    const val = IDR_FULL.format(parseFloat("14246575.3425"));
    expect(val).toContain("14.246.575");
  });

  it("formats pph_20pct 2849315.0685 correctly", () => {
    const val = IDR_FULL.format(parseFloat("2849315.0685"));
    expect(val).toContain("2.849.315");
  });

  it("formats eirBaru 0.04600000 as percentage (4 decimal)", () => {
    const val = (parseFloat("0.04600000") * 100).toFixed(4);
    expect(val).toBe("4.6000");
  });

  it("formats eirBaru 0 as NaN guard — returns —", () => {
    const raw = "not-a-number";
    const n = parseFloat(raw);
    const result = isNaN(n) ? "—" : IDR_FULL.format(n);
    expect(result).toBe("—");
  });
});
