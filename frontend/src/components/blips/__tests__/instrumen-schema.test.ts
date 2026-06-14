/**
 * Unit tests for instrumen Zod schema conditional validation.
 *
 * Covers:
 * - manajerInvestasiId required when tipeInstrumen = REKSADANA
 * - bankKustodianId required when tipeInstrumen = SAHAM | REKSADANA
 * - tanggalJatuhTempo > tanggalPenempatan
 * - fvociElection only valid for SAHAM
 * - autoRenewalFlag only valid for DEPOSITO
 * - kupon decimal range 0-100
 * - eirAwal range 0-1
 * - kodeInstrumen alphanumeric regex
 */

import { describe, it, expect } from "vitest";
import { instrumenCreateSchema } from "@/lib/schemas/instrumen.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function validBase() {
  return {
    kodeInstrumen: "INST-001",
    tipeInstrumen: "OBLIGASI" as const,
    subTipe: "Fixed Rate",
    nama: "Obligasi Pemerintah FR0080",
    isin: "",
    counterpartyId: "00000000-0000-0000-0000-000000000001",
    manajerInvestasiId: "",
    bankKustodianId: "",
    mataUang: "IDR",
    portofolioId: "00000000-0000-0000-0000-000000000002",
    nominal: "1000000000",
    jumlahLot: "",
    tanggalPenempatan: "2026-01-01",
    tanggalJatuhTempo: "2031-01-01",
    kupon: "6.50",
    frekuensiBunga: "SEMESTERAN" as const,
    autoRenewalFlag: false,
    fvociElection: false,
    bmCategory: "HTC" as const,
    eirAwal: "",
    premiumDiskonto: "0",
    biayaTransaksi: "0",
    status: "AKTIF" as const,
  };
}

function safeParse(overrides: Record<string, unknown>) {
  return instrumenCreateSchema.safeParse({ ...validBase(), ...overrides });
}

// ---------------------------------------------------------------------------
// kodeInstrumen
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — kodeInstrumen", () => {
  it("accepts valid alphanumeric code", () => {
    expect(safeParse({ kodeInstrumen: "OBLIG-001" }).success).toBe(true);
  });

  it("accepts uppercase code", () => {
    expect(safeParse({ kodeInstrumen: "FR0080" }).success).toBe(true);
  });

  it("rejects too short (1 char)", () => {
    const r = safeParse({ kodeInstrumen: "A" });
    expect(r.success).toBe(false);
  });

  it("rejects empty string", () => {
    const r = safeParse({ kodeInstrumen: "" });
    expect(r.success).toBe(false);
  });

  it("rejects code with space", () => {
    const r = safeParse({ kodeInstrumen: "OBLIG 001" });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toContain("huruf");
    }
  });
});

// ---------------------------------------------------------------------------
// Conditional: manajerInvestasiId required for REKSADANA
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — manajerInvestasiId (REKSADANA)", () => {
  it("accepts REKSADANA with manajerInvestasiId provided", () => {
    const r = safeParse({
      tipeInstrumen: "REKSADANA",
      manajerInvestasiId: "00000000-0000-0000-0000-000000000010",
      bankKustodianId: "00000000-0000-0000-0000-000000000011",
      kupon: "", // REKSADANA has no kupon
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(true);
  });

  it("rejects REKSADANA without manajerInvestasiId", () => {
    const r = safeParse({
      tipeInstrumen: "REKSADANA",
      manajerInvestasiId: "",
      bankKustodianId: "00000000-0000-0000-0000-000000000011",
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("manajerInvestasiId"),
      );
      expect(issue).toBeDefined();
      expect(issue?.message).toContain("REKSADANA");
    }
  });

  it("does NOT require manajerInvestasiId for OBLIGASI", () => {
    const r = safeParse({
      tipeInstrumen: "OBLIGASI",
      manajerInvestasiId: "",
    });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Conditional: bankKustodianId required for SAHAM, REKSADANA
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — bankKustodianId (SAHAM/REKSADANA)", () => {
  it("rejects SAHAM without bankKustodianId", () => {
    const r = safeParse({
      tipeInstrumen: "SAHAM",
      bankKustodianId: "",
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("bankKustodianId"),
      );
      expect(issue).toBeDefined();
    }
  });

  it("accepts SAHAM with bankKustodianId", () => {
    const r = safeParse({
      tipeInstrumen: "SAHAM",
      bankKustodianId: "00000000-0000-0000-0000-000000000011",
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(true);
  });

  it("does NOT require bankKustodianId for DEPOSITO", () => {
    const r = safeParse({
      tipeInstrumen: "DEPOSITO",
      bankKustodianId: "",
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// tanggalJatuhTempo > tanggalPenempatan
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — tanggalJatuhTempo", () => {
  it("accepts jatuh tempo after penempatan", () => {
    const r = safeParse({
      tanggalPenempatan: "2026-01-01",
      tanggalJatuhTempo: "2031-01-01",
    });
    expect(r.success).toBe(true);
  });

  it("rejects jatuh tempo same day as penempatan", () => {
    const r = safeParse({
      tanggalPenempatan: "2026-01-01",
      tanggalJatuhTempo: "2026-01-01",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("tanggalJatuhTempo"),
      );
      expect(issue?.message).toContain("setelah");
    }
  });

  it("rejects jatuh tempo before penempatan", () => {
    const r = safeParse({
      tanggalPenempatan: "2026-06-01",
      tanggalJatuhTempo: "2026-01-01",
    });
    expect(r.success).toBe(false);
  });

  it("accepts empty jatuh tempo (optional)", () => {
    const r = safeParse({ tanggalJatuhTempo: "" });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// fvociElection — only for SAHAM
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — fvociElection", () => {
  it("accepts fvociElection=true for SAHAM", () => {
    const r = safeParse({
      tipeInstrumen: "SAHAM",
      fvociElection: true,
      bankKustodianId: "00000000-0000-0000-0000-000000000011",
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(true);
  });

  it("rejects fvociElection=true for DEPOSITO", () => {
    const r = safeParse({
      tipeInstrumen: "DEPOSITO",
      fvociElection: true,
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("fvociElection"),
      );
      expect(issue).toBeDefined();
      expect(issue?.message).toContain("SAHAM");
    }
  });
});

// ---------------------------------------------------------------------------
// autoRenewalFlag — only for DEPOSITO
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — autoRenewalFlag", () => {
  it("accepts autoRenewalFlag=true for DEPOSITO", () => {
    const r = safeParse({
      tipeInstrumen: "DEPOSITO",
      autoRenewalFlag: true,
      kupon: "",
      frekuensiBunga: undefined,
    });
    expect(r.success).toBe(true);
  });

  it("rejects autoRenewalFlag=true for OBLIGASI", () => {
    const r = safeParse({
      tipeInstrumen: "OBLIGASI",
      autoRenewalFlag: true,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) =>
        i.path.includes("autoRenewalFlag"),
      );
      expect(issue).toBeDefined();
      expect(issue?.message).toContain("DEPOSITO");
    }
  });
});

// ---------------------------------------------------------------------------
// kupon range
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — kupon", () => {
  it("accepts kupon 6.50", () => {
    expect(safeParse({ kupon: "6.50" }).success).toBe(true);
  });

  it("accepts kupon 0", () => {
    expect(safeParse({ kupon: "0" }).success).toBe(true);
  });

  it("accepts kupon 100", () => {
    expect(safeParse({ kupon: "100" }).success).toBe(true);
  });

  it("rejects kupon 101 (above max)", () => {
    const r = safeParse({ kupon: "101" });
    expect(r.success).toBe(false);
  });

  it("rejects kupon -1 (below min)", () => {
    const r = safeParse({ kupon: "-1" });
    expect(r.success).toBe(false);
  });

  it("accepts empty kupon (optional)", () => {
    expect(safeParse({ kupon: "" }).success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// eirAwal range 0-1
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — eirAwal", () => {
  it("accepts valid eirAwal 0.0625", () => {
    expect(safeParse({ eirAwal: "0.0625" }).success).toBe(true);
  });

  it("accepts eirAwal 0", () => {
    expect(safeParse({ eirAwal: "0" }).success).toBe(true);
  });

  it("accepts eirAwal 1", () => {
    expect(safeParse({ eirAwal: "1" }).success).toBe(true);
  });

  it("rejects eirAwal 1.5 (above 1)", () => {
    const r = safeParse({ eirAwal: "1.5" });
    expect(r.success).toBe(false);
    if (!r.success) {
      const issue = r.error.issues.find((i) => i.path.includes("eirAwal"));
      expect(issue?.message).toContain("0 dan 1");
    }
  });

  it("rejects eirAwal -0.1 (below 0)", () => {
    const r = safeParse({ eirAwal: "-0.1" });
    expect(r.success).toBe(false);
  });

  it("accepts empty eirAwal (optional)", () => {
    expect(safeParse({ eirAwal: "" }).success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Happy path: full valid record
// ---------------------------------------------------------------------------

describe("instrumenCreateSchema — happy path", () => {
  it("accepts a complete valid OBLIGASI record", () => {
    const r = instrumenCreateSchema.safeParse(validBase());
    expect(r.success).toBe(true);
  });

  it("accepts a DEPOSITO with autoRenewal", () => {
    const r = safeParse({
      tipeInstrumen: "DEPOSITO",
      kupon: "",
      frekuensiBunga: undefined,
      autoRenewalFlag: true,
      tanggalJatuhTempo: "2027-01-01",
    });
    expect(r.success).toBe(true);
  });

  it("accepts a SAHAM with FVOCI election and bank kustodian", () => {
    const r = safeParse({
      tipeInstrumen: "SAHAM",
      fvociElection: true,
      bankKustodianId: "00000000-0000-0000-0000-000000000011",
      kupon: "",
      frekuensiBunga: undefined,
      tanggalJatuhTempo: "",
    });
    expect(r.success).toBe(true);
  });
});
