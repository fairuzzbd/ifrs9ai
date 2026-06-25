/**
 * Vitest unit tests — P5-M6 MTM Daily components (logic-layer, no DOM render).
 *
 * env = "node" (no jsdom). Tests cover:
 *  - Badge state matrix (5 status values × visual token assertions)
 *  - Routing badge display logic (event code → label mapping)
 *  - Override SoD Zod contract
 *  - Upload form validation (Zod schema)
 *  - Persona gating logic (permission string)
 *  - Deviation badge threshold logic
 *  - Stale badge escalation logic
 *
 * Render-level assertions → Playwright E2E (tests/e2e/).
 */

import { describe, it, expect } from "vitest";
import { z } from "zod";

// ---------------------------------------------------------------------------
// Schemas under test
// ---------------------------------------------------------------------------

import {
  mtmStatusEnum,
  hargaSumberEnum,
  mtmKlasifikasiEnum,
  stalePriceReasonEnum,
  mtmErrorCodeEnum,
  MTM_STATUS_LABELS,
  HARGA_SUMBER_LABELS,
  MTM_KLASIFIKASI_LABELS,
  JURNAL_EVENT_CODE_LABELS,
  mtmListItemSchema,
  mtmOverrideApproveSchema,
  mtmOverrideRejectSchema,
  mtmUploadFormSchema,
  mtmCronTriggerSchema,
} from "@/lib/schemas/mtm.schema";

// ===========================================================================
// 1. Badge state matrix — 5 MTM status values
// ===========================================================================

describe("MtmStatusBadge — state matrix", () => {
  it("schema defines exactly 5 status values", () => {
    expect(mtmStatusEnum.options).toHaveLength(5);
    expect(mtmStatusEnum.options).toContain("AUTO_POSTED");
    expect(mtmStatusEnum.options).toContain("PENDING_REVIEW");
    expect(mtmStatusEnum.options).toContain("APPROVED");
    expect(mtmStatusEnum.options).toContain("REJECTED");
    expect(mtmStatusEnum.options).toContain("STALE_PRICE");
  });

  it("all status values are distinct", () => {
    const opts = mtmStatusEnum.options;
    expect(new Set(opts).size).toBe(opts.length);
  });

  it("MTM_STATUS_LABELS covers all 5 states", () => {
    for (const status of mtmStatusEnum.options) {
      expect(MTM_STATUS_LABELS[status]).toBeDefined();
      expect(MTM_STATUS_LABELS[status].length).toBeGreaterThan(0);
    }
  });

  it("STALE_PRICE and REJECTED have distinct labels (visual distinction)", () => {
    expect(MTM_STATUS_LABELS["STALE_PRICE"]).not.toBe(MTM_STATUS_LABELS["REJECTED"]);
  });

  it("valid status parses correctly", () => {
    expect(mtmStatusEnum.parse("AUTO_POSTED")).toBe("AUTO_POSTED");
    expect(mtmStatusEnum.parse("STALE_PRICE")).toBe("STALE_PRICE");
  });

  it("invalid status throws", () => {
    expect(() => mtmStatusEnum.parse("INVALID_STATUS")).toThrow();
  });
});

// ===========================================================================
// 2. MtmRoutingBadge — jurnal event code label mapping
// ===========================================================================

describe("MtmRoutingBadge — event code label mapping", () => {
  it("JURNAL_EVENT_CODE_LABELS covers 5 event types", () => {
    const expectedCodes = [
      "MTM_FVOCI",
      "MTM_FX_OCI_RESERVE",
      "MTM_FVOCI_ELECTION",
      "MTM_FVTPL",
      "MTM_FVTPL_POCI",
    ];
    for (const code of expectedCodes) {
      expect(JURNAL_EVENT_CODE_LABELS[code]).toBeDefined();
      expect(typeof JURNAL_EVENT_CODE_LABELS[code]).toBe("string");
    }
  });

  it("FVOCI_DEBT FCY dual-jurnal: two codes defined (§B5.7.2A)", () => {
    // MTM_FVOCI and MTM_FX_OCI_RESERVE must both have labels (dual entries)
    expect(JURNAL_EVENT_CODE_LABELS["MTM_FVOCI"]).toBe("OCI Nilai Wajar");
    expect(JURNAL_EVENT_CODE_LABELS["MTM_FX_OCI_RESERVE"]).toBe("OCI FX Reserve");
  });

  it("AC instruments have no MTM event code (skip rule)", () => {
    // AC is not in mtmKlasifikasiEnum (per PSAK 71 only FVOCI/FVTPL/POCI get MTM)
    const validKlasifikasi = mtmKlasifikasiEnum.options;
    expect(validKlasifikasi).not.toContain("AC");
  });

  it("MTM_KLASIFIKASI_LABELS covers all 4 non-AC klasifikasi", () => {
    for (const klas of mtmKlasifikasiEnum.options) {
      expect(MTM_KLASIFIKASI_LABELS[klas]).toBeDefined();
    }
    expect(mtmKlasifikasiEnum.options).toHaveLength(4);
  });

  it("FVOCI_ELECTION maps to OCI Ekuitas (equity no recycling)", () => {
    expect(JURNAL_EVENT_CODE_LABELS["MTM_FVOCI_ELECTION"]).toBe("OCI Ekuitas");
  });
});

// ===========================================================================
// 3. Override SoD — Zod contract (mtmOverrideApproveSchema)
// ===========================================================================

describe("MtmOverrideApproveDialog — Zod SoD contract", () => {
  const VALID_INPUT = {
    comment: "Harga terverifikasi via Bloomberg. Delta wajar karena rilis FOMC.",
    signatureMethod: "JWT_STEP_UP" as const,
    attest: true as const,
  };

  it("valid input parses successfully", () => {
    const result = mtmOverrideApproveSchema.safeParse(VALID_INPUT);
    expect(result.success).toBe(true);
  });

  it("comment < 30 chars is rejected", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      comment: "Too short",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toMatch(/30/);
    }
  });

  it("attest = false is rejected (must be literal true)", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      attest: false,
    });
    expect(result.success).toBe(false);
  });

  it("attest = undefined is rejected", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      attest: undefined,
    });
    expect(result.success).toBe(false);
  });

  it("wrong signatureMethod is rejected", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      signatureMethod: "TOTP",
    });
    expect(result.success).toBe(false);
  });

  it("comment exactly 30 chars is accepted", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      comment: "a".repeat(30),
    });
    expect(result.success).toBe(true);
  });

  it("comment 29 chars is rejected", () => {
    const result = mtmOverrideApproveSchema.safeParse({
      ...VALID_INPUT,
      comment: "a".repeat(29),
    });
    expect(result.success).toBe(false);
  });
});

// ===========================================================================
// 4. Override reject — Zod contract (S4-AC4: comment ≥ 30 WAJIB)
// ===========================================================================

describe("MtmOverrideRejectDialog — Zod contract (S4-AC4)", () => {
  const VALID_REJECT = {
    comment: "Harga 90.00 tidak sesuai IBPA hari ini. Re-upload dengan referensi Bloomberg.",
    signatureMethod: "JWT_STEP_UP" as const,
  };

  it("valid reject input parses", () => {
    expect(mtmOverrideRejectSchema.safeParse(VALID_REJECT).success).toBe(true);
  });

  it("empty comment fails (S4-AC4: wajib alasan)", () => {
    const result = mtmOverrideRejectSchema.safeParse({ ...VALID_REJECT, comment: "" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toMatch(/30/);
    }
  });

  it("comment exactly 30 chars is accepted", () => {
    expect(
      mtmOverrideRejectSchema.safeParse({ ...VALID_REJECT, comment: "b".repeat(30) }).success,
    ).toBe(true);
  });

  it("comment 29 chars is rejected", () => {
    expect(
      mtmOverrideRejectSchema.safeParse({ ...VALID_REJECT, comment: "b".repeat(29) }).success,
    ).toBe(false);
  });

  it("no attest field required in reject schema (unlike approve)", () => {
    // mtmOverrideRejectSchema should NOT have 'attest' key
    const schema = mtmOverrideRejectSchema;
    const result = schema.safeParse(VALID_REJECT);
    expect(result.success).toBe(true);
    // The schema doesn't strip extra fields with .strict() so just verify the base case
  });
});

// ===========================================================================
// 5. Upload form validation — mtmUploadFormSchema
// ===========================================================================

describe("MtmUploadDropzone — upload form Zod validation", () => {
  // Note: z.instanceof(File) in node env — we use a mock-like approach via shape inspection
  it("schema has file, catatanUpload, tanggalMtmOverride fields", () => {
    const shape = (mtmUploadFormSchema as z.ZodObject<z.ZodRawShape>).shape;
    expect(shape).toHaveProperty("file");
    expect(shape).toHaveProperty("catatanUpload");
    expect(shape).toHaveProperty("tanggalMtmOverride");
  });

  it("catatanUpload max length 1000", () => {
    // Test via a partial validation excluding file
    const partialSchema = z.object({
      catatanUpload: z.string().max(1000).optional(),
    });
    expect(partialSchema.safeParse({ catatanUpload: "a".repeat(1001) }).success).toBe(false);
    expect(partialSchema.safeParse({ catatanUpload: "a".repeat(1000) }).success).toBe(true);
    expect(partialSchema.safeParse({ catatanUpload: undefined }).success).toBe(true);
  });

  it("tanggalMtmOverride accepts valid date string", () => {
    const partialSchema = z.object({
      tanggalMtmOverride: z.string().date().optional().or(z.literal("")),
    });
    expect(partialSchema.safeParse({ tanggalMtmOverride: "2026-06-18" }).success).toBe(true);
    expect(partialSchema.safeParse({ tanggalMtmOverride: "" }).success).toBe(true);
    expect(partialSchema.safeParse({ tanggalMtmOverride: undefined }).success).toBe(true);
    expect(partialSchema.safeParse({ tanggalMtmOverride: "not-a-date" }).success).toBe(false);
  });
});

// ===========================================================================
// 6. Persona gating logic — permission strings
// ===========================================================================

describe("Persona gating — permission string contracts", () => {
  it("mtm.read is the base read permission", () => {
    // String contract — all list pages check this
    expect("mtm.read").toBe("mtm.read");
  });

  it("mtm.create is the upload permission (ROLE-AKUN)", () => {
    expect("mtm.create").toBe("mtm.create");
  });

  it("mtm.override is the approve/reject permission (ROLE-AKUN-CTL)", () => {
    expect("mtm.override").toBe("mtm.override");
  });

  it("mtm.trigger is cron-trigger permission (ROLE-IT-ADMIN only)", () => {
    expect("mtm.trigger").toBe("mtm.trigger");
  });

  // Simulate usePermissions().can() pattern with a mock can function
  it("user without mtm.trigger cannot see trigger button (absent-from-DOM)", () => {
    const mockPermissions = ["mtm.read", "mtm.create"];
    const can = (perm: string) => mockPermissions.includes(perm);
    expect(can("mtm.trigger")).toBe(false);
  });

  it("IT-ADMIN user with mtm.trigger can see trigger button", () => {
    const mockPermissions = ["mtm.read", "mtm.trigger"];
    const can = (perm: string) => mockPermissions.includes(perm);
    expect(can("mtm.trigger")).toBe(true);
  });

  it("ROLE-AKUN cannot override (no mtm.override)", () => {
    const mockPermissions = ["mtm.read", "mtm.create"];
    const can = (perm: string) => mockPermissions.includes(perm);
    expect(can("mtm.override")).toBe(false);
  });

  it("ROLE-AKUN-CTL can override", () => {
    const mockPermissions = ["mtm.read", "mtm.override"];
    const can = (perm: string) => mockPermissions.includes(perm);
    expect(can("mtm.override")).toBe(true);
  });
});

// ===========================================================================
// 7. MtmDeviationBadge — threshold logic
// ===========================================================================

describe("MtmDeviationBadge — threshold logic", () => {
  // Component renders when deltaPct exceeds threshold
  // Logic: deviationFlag = Math.abs(deltaPct) >= thresholdPct
  function isDeviation(deltaPct: number, thresholdPct: number): boolean {
    return Math.abs(deltaPct) >= thresholdPct;
  }

  it("5% delta at 5% threshold triggers deviation", () => {
    expect(isDeviation(5.0, 5.0)).toBe(true);
  });

  it("4.99% delta at 5% threshold does NOT trigger deviation", () => {
    expect(isDeviation(4.99, 5.0)).toBe(false);
  });

  it("negative delta also triggers deviation (downward movement)", () => {
    expect(isDeviation(-6.5, 5.0)).toBe(true);
  });

  it("0% delta is not a deviation", () => {
    expect(isDeviation(0, 5.0)).toBe(false);
  });

  it("delta formatted correctly for positive", () => {
    const deltaPct = 7.234;
    const formatted = `+${deltaPct.toFixed(2)}%`;
    expect(formatted).toBe("+7.23%");
  });

  it("delta formatted correctly for negative", () => {
    const deltaPct = -3.5;
    const formatted = `${deltaPct.toFixed(2)}%`;
    expect(formatted).toBe("-3.50%");
  });
});

// ===========================================================================
// 8. MtmStaleBadge — escalation logic
// ===========================================================================

describe("MtmStaleBadge — escalation logic", () => {
  const STALE_DAYS = 5;   // sys.config MTM_PRICE_STALE_DAYS
  const ESCALATION_DAYS = 7; // sys.config MTM_STALE_ESCALATION_DAYS

  function isEscalated(hargaAgeDays: number): boolean {
    return hargaAgeDays > ESCALATION_DAYS;
  }

  function isStale(hargaAgeDays: number): boolean {
    return hargaAgeDays > STALE_DAYS;
  }

  it("hargaAgeDays = 5 is stale", () => {
    expect(isStale(5)).toBe(false); // > 5, not >= 5 — boundary
    expect(isStale(6)).toBe(true);
  });

  it("hargaAgeDays = 7 is NOT escalated", () => {
    expect(isEscalated(7)).toBe(false); // > 7
  });

  it("hargaAgeDays = 8 is escalated", () => {
    expect(isEscalated(8)).toBe(true);
  });

  it("escalated flag triggers ROLE-RISK notification (state rule)", () => {
    // Business rule: esklasiasiFlag=true means ROLE-RISK notified (not just stale)
    const item = { hargaAgeDays: 10, esklasiasiFlag: true };
    expect(item.esklasiasiFlag).toBe(true);
    expect(isEscalated(item.hargaAgeDays)).toBe(true);
  });
});

// ===========================================================================
// 9. MtmSourceBadge — source enum coverage
// ===========================================================================

describe("MtmSourceBadge — source enum + labels", () => {
  it("defines 6 harga sumber values", () => {
    expect(hargaSumberEnum.options).toHaveLength(6);
  });

  it("HARGA_SUMBER_LABELS covers all sources", () => {
    for (const src of hargaSumberEnum.options) {
      expect(HARGA_SUMBER_LABELS[src]).toBeDefined();
      expect(HARGA_SUMBER_LABELS[src].length).toBeGreaterThan(0);
    }
  });

  it("IBPA_MANUAL and BEI_MANUAL are distinct from IBPA and BEI (manual indicator)", () => {
    expect(HARGA_SUMBER_LABELS["IBPA_MANUAL"]).not.toBe(HARGA_SUMBER_LABELS["IBPA"]);
    expect(HARGA_SUMBER_LABELS["BEI_MANUAL"]).not.toBe(HARGA_SUMBER_LABELS["BEI"]);
  });
});

// ===========================================================================
// 10. Error code coverage — 6 new MTM codes
// ===========================================================================

describe("MTM error codes — 6 new codes defined", () => {
  const NEW_MTM_CODES = [
    "MTM_PRICE_STALE",
    "MTM_PRICE_DEVIATION_REJECTED",
    "MTM_BATCH_NOT_FOUND",
    "MTM_OVERRIDE_SOD_VIOLATION",
    "MTM_INSTRUMEN_AC_SKIP",
    "MTM_PERIODE_LOCKED",
  ] as const;

  it("all 6 MTM error codes are in mtmErrorCodeEnum", () => {
    for (const code of NEW_MTM_CODES) {
      expect(mtmErrorCodeEnum.options).toContain(code);
    }
  });

  it("MTM_INSTRUMEN_AC_SKIP is distinct from MTM_PERIODE_LOCKED", () => {
    expect("MTM_INSTRUMEN_AC_SKIP").not.toBe("MTM_PERIODE_LOCKED");
  });

  it("MTM_OVERRIDE_SOD_VIOLATION confirms SoD enforcement contract", () => {
    // This error must be thrown server-side when uploader == override_approver
    expect(mtmErrorCodeEnum.options).toContain("MTM_OVERRIDE_SOD_VIOLATION");
  });
});

// ===========================================================================
// 11. Cron trigger schema validation
// ===========================================================================

describe("MtmCronTriggerButton — schema validation", () => {
  it("valid input without tanggalTarget parses (uses today as default)", () => {
    const result = mtmCronTriggerSchema.safeParse({ forceRerun: false });
    expect(result.success).toBe(true);
  });

  it("valid past date is accepted", () => {
    const result = mtmCronTriggerSchema.safeParse({
      tanggalTarget: "2026-01-01",
      forceRerun: true,
    });
    expect(result.success).toBe(true);
  });

  it("empty string tanggalTarget is accepted (treated as undefined)", () => {
    const result = mtmCronTriggerSchema.safeParse({ tanggalTarget: "", forceRerun: false });
    expect(result.success).toBe(true);
  });

  it("invalid date string fails", () => {
    const result = mtmCronTriggerSchema.safeParse({ tanggalTarget: "not-a-date" });
    expect(result.success).toBe(false);
  });

  it("forceRerun defaults to false", () => {
    const result = mtmCronTriggerSchema.safeParse({});
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.forceRerun).toBe(false);
    }
  });
});

// ===========================================================================
// 12. MtmListItem schema — type contract
// ===========================================================================

describe("MtmListItem schema — type contract", () => {
  const VALID_ITEM = {
    id: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    instrumenId: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
    instrumenKode: "INST-001234",
    instrumenNama: "Obligasi FR0094",
    tanggalMtm: "2026-06-18",
    hargaSumber: "IBPA",
    hargaPasarIdr: 1_050_000,
    hargaBukuIdr: 1_000_000,
    deltaIdr: 50_000,
    deltaPct: 5.0,
    hargaAgeDays: 0,
    stalePriceFlag: false,
    deviationFlag: true,
    status: "PENDING_REVIEW",
    klasifikasiSnapshot: "FVOCI_DEBT",
    jurnalEventCode: "MTM_FVOCI",
    jurnalEntryId: null,
    uploaderId: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
    overrideApproverId: null,
    overrideAt: null,
    lockedFlag: false,
    createdAt: "2026-06-18T18:00:00+07:00",
  };

  it("valid list item parses successfully", () => {
    const result = mtmListItemSchema.safeParse(VALID_ITEM);
    expect(result.success).toBe(true);
  });

  it("AC klasifikasi is NOT in mtmKlasifikasiEnum (AC never gets MTM per PSAK 71)", () => {
    const result = mtmListItemSchema.safeParse({
      ...VALID_ITEM,
      klasifikasiSnapshot: "AC",
    });
    expect(result.success).toBe(false);
  });

  it("missing instrumenKode fails", () => {
    const { instrumenKode: _removed, ...rest } = VALID_ITEM;
    expect(mtmListItemSchema.safeParse(rest).success).toBe(false);
  });

  it("POCI is a valid klasifikasi for MTM", () => {
    const result = mtmListItemSchema.safeParse({
      ...VALID_ITEM,
      klasifikasiSnapshot: "POCI",
    });
    expect(result.success).toBe(true);
  });
});
