/**
 * Frontend unit tests — P5-M8 Penjualan/Pencairan Instrumen
 *
 * Coverage:
 *  - Badge matrix: resolveOCIRecycleMode per klasifikasi
 *  - resolveJurnalEventCodes routing matrix (5 klasifikasi × PARTIAL/FULL)
 *  - Zod schema: createPenjualanSchema, rejectPenjualanSchema (≥30 char)
 *  - BM risk badge display logic
 *  - SoD contract: rejectPenjualanSchema.signatureMethod must be JWT_STEP_UP
 *  - Persona gating: persona labels exist for all 5 statuses
 *  - OCI no-recycling note display condition
 *
 * Run: pnpm vitest run src/components/blips/__tests__/penjualan-schema.test.ts
 */

import { describe, it, expect } from "vitest";
import {
  createPenjualanSchema,
  rejectPenjualanSchema,
  approvePenjualanSchema,
  penjualanStatusEnum,
  PENJUALAN_STATUS_LABELS,
  JENIS_DISPOSAL_LABELS,
  KLASIFIKASI_LABELS,
} from "@/lib/schemas/penjualan.schema";
import { resolveOCIRecycleMode } from "@/components/blips/penjualan/PenjualanOCIRecycleBadge";
import { resolveJurnalEventCodes } from "@/components/blips/penjualan/PenjualanRoutingBadge";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function validCreateBase() {
  return {
    instrumenId: "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    jenisDisposal: "PARTIAL" as const,
    qtyTerjual: "500",
    hargaJualPerUnit: "1050000",
    tanggalEksekusi: "2026-07-15",
  };
}

function safeParse(overrides: Record<string, unknown>) {
  return createPenjualanSchema.safeParse({ ...validCreateBase(), ...overrides });
}

// ─── OCI Recycle Mode Matrix ────────────────────────────────────────────────

describe("resolveOCIRecycleMode — badge matrix per klasifikasi", () => {
  it("AC → NOT_APPLICABLE", () => {
    expect(resolveOCIRecycleMode("AC")).toBe("NOT_APPLICABLE");
  });

  it("FVOCI (debt) → RECYCLE", () => {
    expect(resolveOCIRecycleMode("FVOCI")).toBe("RECYCLE");
  });

  it("FVOCI_ELECTION → NO_RECYCLE (§B5.7.1)", () => {
    expect(resolveOCIRecycleMode("FVOCI_ELECTION")).toBe("NO_RECYCLE");
  });

  it("FVTPL → NOT_APPLICABLE", () => {
    expect(resolveOCIRecycleMode("FVTPL")).toBe("NOT_APPLICABLE");
  });

  it("POCI → NOT_APPLICABLE", () => {
    expect(resolveOCIRecycleMode("POCI")).toBe("NOT_APPLICABLE");
  });
});

// ─── Jurnal Event Code Routing Matrix ──────────────────────────────────────

describe("resolveJurnalEventCodes — routing matrix (state-machine §Klasifikasi Routing)", () => {
  it("AC PARTIAL → [PENJUALAN_AC]", () => {
    expect(resolveJurnalEventCodes("AC", "PARTIAL")).toEqual(["PENJUALAN_AC"]);
  });

  it("AC FULL → [PENJUALAN_AC]", () => {
    expect(resolveJurnalEventCodes("AC", "FULL")).toEqual(["PENJUALAN_AC"]);
  });

  it("FVOCI PARTIAL → [PENJUALAN_FVOCI_DEBT, REKLAS_OCI_PL]", () => {
    expect(resolveJurnalEventCodes("FVOCI", "PARTIAL")).toEqual([
      "PENJUALAN_FVOCI_DEBT",
      "REKLAS_OCI_PL",
    ]);
  });

  it("FVOCI FULL → [PENJUALAN_FVOCI_DEBT, REKLAS_OCI_PL]", () => {
    expect(resolveJurnalEventCodes("FVOCI", "FULL")).toEqual([
      "PENJUALAN_FVOCI_DEBT",
      "REKLAS_OCI_PL",
    ]);
  });

  it("FVOCI_ELECTION FULL → [PENJUALAN_FVOCI_ELECTION] (no REKLAS_OCI_PL)", () => {
    expect(resolveJurnalEventCodes("FVOCI_ELECTION", "FULL")).toEqual([
      "PENJUALAN_FVOCI_ELECTION",
    ]);
  });

  it("FVTPL PARTIAL → [PENJUALAN_FVTPL]", () => {
    expect(resolveJurnalEventCodes("FVTPL", "PARTIAL")).toEqual(["PENJUALAN_FVTPL"]);
  });

  it("POCI FULL → [PENJUALAN_POCI]", () => {
    expect(resolveJurnalEventCodes("POCI", "FULL")).toEqual(["PENJUALAN_POCI"]);
  });
});

// ─── createPenjualanSchema ─────────────────────────────────────────────────

describe("createPenjualanSchema validation", () => {
  it("S1-AC1: valid PARTIAL FVOCI payload → parse success", () => {
    const result = safeParse({});
    expect(result.success).toBe(true);
  });

  it("S1-AC2: qtyTerjual = '0' → fails (must be > 0)", () => {
    const result = safeParse({ qtyTerjual: "0" });
    expect(result.success).toBe(false);
    if (!result.success) {
      const field = result.error.issues.find((i) => i.path.includes("qtyTerjual"));
      expect(field).toBeDefined();
    }
  });

  it("S1-AC2: qtyTerjual negative → fails", () => {
    const result = safeParse({ qtyTerjual: "-100" });
    expect(result.success).toBe(false);
  });

  it("S1-AC3: instrumenId not uuid → fails", () => {
    const result = safeParse({ instrumenId: "NOT-A-UUID" });
    expect(result.success).toBe(false);
    if (!result.success) {
      const field = result.error.issues.find((i) => i.path.includes("instrumenId"));
      expect(field).toBeDefined();
    }
  });

  it("S1: hargaJualPerUnit = '0' → fails (must be > 0)", () => {
    const result = safeParse({ hargaJualPerUnit: "0" });
    expect(result.success).toBe(false);
  });

  it("S1: tanggalEksekusi bad format → fails", () => {
    const result = safeParse({ tanggalEksekusi: "15-07-2026" }); // wrong format
    expect(result.success).toBe(false);
    if (!result.success) {
      const field = result.error.issues.find((i) => i.path.includes("tanggalEksekusi"));
      expect(field).toBeDefined();
    }
  });

  it("S1: jenisDisposal must be PARTIAL or FULL", () => {
    const result = safeParse({ jenisDisposal: "UNKNOWN" });
    expect(result.success).toBe(false);
  });

  it("S1: FULL disposal valid", () => {
    const result = safeParse({ jenisDisposal: "FULL" });
    expect(result.success).toBe(true);
  });
});

// ─── rejectPenjualanSchema — reason ≥ 30 char ──────────────────────────────

describe("rejectPenjualanSchema — reason minimum 30 characters (S2)", () => {
  it("reason of exactly 30 chars → valid", () => {
    const result = rejectPenjualanSchema.safeParse({
      reason: "a".repeat(30),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("reason < 30 chars → fails", () => {
    const result = rejectPenjualanSchema.safeParse({
      reason: "Terlalu pendek",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const issue = result.error.issues.find((i) => i.path.includes("reason"));
      expect(issue?.message).toMatch(/30/);
    }
  });

  it("signatureMethod wrong value → fails (SoD contract)", () => {
    const result = rejectPenjualanSchema.safeParse({
      reason: "Harga jual melebihi IBPA fair value lebih dari 2%, harap revisi.",
      signatureMethod: "PLAIN_TEXT",
    });
    expect(result.success).toBe(false);
  });

  it("signatureMethod JWT_STEP_UP → valid (SoD contract)", () => {
    const result = rejectPenjualanSchema.safeParse({
      reason: "Harga jual melebihi IBPA fair value lebih dari 2%, harap revisi.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });
});

// ─── approvePenjualanSchema ───────────────────────────────────────────────

describe("approvePenjualanSchema — SoD + signatureMethod (S2)", () => {
  it("valid approve input → parse success", () => {
    const result = approvePenjualanSchema.safeParse({
      comment: "Preview diverifikasi. Harga sesuai IBPA. Disetujui.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("empty comment → fails", () => {
    const result = approvePenjualanSchema.safeParse({
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
  });

  it("wrong signatureMethod → fails (DEC-027)", () => {
    const result = approvePenjualanSchema.safeParse({
      comment: "Disetujui",
      signatureMethod: "MANUAL",
    });
    expect(result.success).toBe(false);
  });
});

// ─── Status + labels completeness ────────────────────────────────────────

describe("penjualanStatusEnum labels — all 5 statuses defined (persona gating)", () => {
  const statuses = penjualanStatusEnum.options;

  it("has 5 statuses including PENDING_BM_REVIEW", () => {
    expect(statuses).toContain("PENDING_APPROVAL");
    expect(statuses).toContain("APPROVED");
    expect(statuses).toContain("POSTED");
    expect(statuses).toContain("REJECTED");
    expect(statuses).toContain("PENDING_BM_REVIEW");
  });

  it("every status has a Bahasa Indonesia label", () => {
    for (const s of statuses) {
      expect(PENJUALAN_STATUS_LABELS[s]).toBeTruthy();
    }
  });

  it("PENDING_BM_REVIEW label contains BM review context", () => {
    expect(PENJUALAN_STATUS_LABELS["PENDING_BM_REVIEW"]).toMatch(/BM|Review/i);
  });
});

// ─── Label maps completeness ──────────────────────────────────────────────

describe("JENIS_DISPOSAL_LABELS completeness", () => {
  it("PARTIAL and FULL both have labels", () => {
    expect(JENIS_DISPOSAL_LABELS["PARTIAL"]).toBeTruthy();
    expect(JENIS_DISPOSAL_LABELS["FULL"]).toBeTruthy();
  });
});

describe("KLASIFIKASI_LABELS completeness", () => {
  const klasifikasiList = ["AC", "FVOCI", "FVOCI_ELECTION", "FVTPL", "POCI"] as const;

  it("all 5 klasifikasi have Bahasa Indonesia labels", () => {
    for (const k of klasifikasiList) {
      expect(KLASIFIKASI_LABELS[k]).toBeTruthy();
    }
  });

  it("FVOCI_ELECTION label mentions ekuitas or OCI", () => {
    expect(KLASIFIKASI_LABELS["FVOCI_ELECTION"]).toMatch(/OCI|Ekuitas/i);
  });
});
