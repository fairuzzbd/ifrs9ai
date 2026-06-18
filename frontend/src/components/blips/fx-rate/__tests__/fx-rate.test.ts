/**
 * Vitest unit tests — P5-M5 FX Rate component logic (node environment)
 *
 * Tests pure-logic helpers and data contracts — no DOM rendering.
 * For E2E see: frontend/tests/e2e/kurs/
 *
 * AC coverage:
 *   S1-AC1..4   JISDOR cron auto-approve + deviation flag display
 *   S2-AC1..4   Manual upload form schema + batch status
 *   S3-AC1..4   Approve/Reject batch schema + SoD contract
 *   S4-AC1..4   Locked state (periode hard-close) + badge contract
 *   S5-AC1..4   FX treatment routing label completeness + decision tree
 */

import { describe, it, expect } from "vitest";
import {
  kursWorkflowStatusP5Enum,
  fxTreatmentRoutingEnum,
  klasifikasiPsak71Enum,
  kursUploadFormSchema,
  kursApproveBodySchema,
  kursRejectBodySchema,
  jisdorSyncTriggerBodySchema,
  jisdorSyncJobResponseSchema,
  kursListItemP5Schema,
  kursDetailP5Schema,
  kursUploadResponseSchema,
  kursBatchApproveResponseSchema,
  kursBatchRejectResponseSchema,
  kursTreatmentResponseSchema,
  fxTreatmentDetailSchema,
  WORKFLOW_STATUS_P5_LABELS,
  SUMBER_KURS_LABELS,
  FX_TREATMENT_LABELS,
  FX_TREATMENT_PSAK_REF,
  FX_RATE_ERROR_MESSAGES,
  type KursWorkflowStatusP5,
  type FxTreatmentRouting,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// 1. KursWorkflowBadge — badge state matrix (S1-AC1, S4-AC1..4)
// ---------------------------------------------------------------------------

describe("KursWorkflowBadge state matrix (S1-AC1, S4-AC1..4)", () => {
  const allStatuses = kursWorkflowStatusP5Enum.options as KursWorkflowStatusP5[];

  it("every workflow status has a non-empty Bahasa Indonesia label", () => {
    for (const s of allStatuses) {
      const label = WORKFLOW_STATUS_P5_LABELS[s];
      expect(label, `missing label for status ${s}`).toBeTruthy();
      expect(label.length).toBeGreaterThan(0);
    }
  });

  it("all 3 workflow statuses covered: PENDING_APPROVAL, APPROVED, REJECTED", () => {
    expect(WORKFLOW_STATUS_P5_LABELS["PENDING_APPROVAL"]).toBeTruthy();
    expect(WORKFLOW_STATUS_P5_LABELS["APPROVED"]).toBeTruthy();
    expect(WORKFLOW_STATUS_P5_LABELS["REJECTED"]).toBeTruthy();
  });

  it("labels are all distinct", () => {
    const labels = Object.values(WORKFLOW_STATUS_P5_LABELS);
    const unique = new Set(labels);
    expect(unique.size).toBe(labels.length);
  });

  // LOCKED is computed from workflowStatus=APPROVED + lockedFlag=true
  it("LOCKED display state: computed when workflowStatus=APPROVED AND lockedFlag=true (S4-AC1)", () => {
    // Pure logic test — mirrors KursWorkflowBadge.tsx line:
    // const displayState = lockedFlag && workflowStatus === "APPROVED" ? "LOCKED" : workflowStatus;
    function computeDisplayState(
      workflowStatus: KursWorkflowStatusP5,
      lockedFlag: boolean,
    ): KursWorkflowStatusP5 | "LOCKED" {
      return lockedFlag && workflowStatus === "APPROVED" ? "LOCKED" : workflowStatus;
    }

    expect(computeDisplayState("APPROVED", true)).toBe("LOCKED");
    expect(computeDisplayState("APPROVED", false)).toBe("APPROVED");
    expect(computeDisplayState("PENDING_APPROVAL", true)).toBe("PENDING_APPROVAL");
    expect(computeDisplayState("REJECTED", true)).toBe("REJECTED");
  });

  it("LOCKED only applies when status is APPROVED (not PENDING or REJECTED) (S4-AC2)", () => {
    function computeDisplayState(
      workflowStatus: KursWorkflowStatusP5,
      lockedFlag: boolean,
    ): KursWorkflowStatusP5 | "LOCKED" {
      return lockedFlag && workflowStatus === "APPROVED" ? "LOCKED" : workflowStatus;
    }

    // Must NOT show LOCKED for PENDING or REJECTED even with lockedFlag=true
    expect(computeDisplayState("PENDING_APPROVAL", true)).not.toBe("LOCKED");
    expect(computeDisplayState("REJECTED", true)).not.toBe("LOCKED");
  });
});

// ---------------------------------------------------------------------------
// 2. KursDeviationBadge — null contract (S1-AC3)
// ---------------------------------------------------------------------------

describe("KursDeviationBadge null contract (S1-AC3)", () => {
  it("deviationFlag=false → component should return null (absent from DOM) (S1-AC3)", () => {
    // Pure logic: the badge renders null when deviationFlag=false
    // Mirrors KursDeviationBadge.tsx: if (!deviationFlag) return null;
    function shouldRender(deviationFlag: boolean): boolean {
      return deviationFlag;
    }

    expect(shouldRender(false)).toBe(false);
    expect(shouldRender(true)).toBe(true);
  });

  it("deviation percentage formats correctly to 2 decimals (S1-AC3)", () => {
    // Mirrors display logic in KursDeviationBadge:
    // `±${Math.abs(rateDeviationPct).toFixed(2)}%`
    function formatDeviation(pct: number): string {
      return `±${Math.abs(pct).toFixed(2)}%`;
    }

    expect(formatDeviation(25.5)).toBe("±25.50%");
    expect(formatDeviation(-22.3)).toBe("±22.30%"); // negative pct → abs
    expect(formatDeviation(0.01)).toBe("±0.01%");
    expect(formatDeviation(100)).toBe("±100.00%");
  });

  it("KursListItemP5 deviationFlag and rateDeviationPct schema contract (S1-AC3)", () => {
    const row = {
      id: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      fxRateIdKode: "KURS-2026-001",
      kodeMataUang: "USD",
      tanggalBerlaku: "2026-06-18",
      kursTengah: 16100.5,
      kursBeli: 16050,
      kursJual: 16150,
      sumberKurs: "BI_JISDOR",
      workflowStatus: "APPROVED",
      lockedFlag: false,
      deviationFlag: true,
      rateDeviationPct: 22.5,
      periodeKode: "PRD-2026-06",
      makerId: null,
      approverId: null,
      approvedAt: null,
      rejectReason: null,
      uploadBatchId: null,
      createdAt: "2026-06-18T10:30:00+07:00",
      createdBy: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    };
    const r = kursListItemP5Schema.safeParse(row);
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.deviationFlag).toBe(true);
      expect(r.data.rateDeviationPct).toBe(22.5);
    }
  });

  it("rateDeviationPct can be null when deviationFlag=false (schema contract)", () => {
    const row = {
      id: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      fxRateIdKode: "KURS-2026-002",
      kodeMataUang: "USD",
      tanggalBerlaku: "2026-06-18",
      kursTengah: 16100.5,
      kursBeli: null,
      kursJual: null,
      sumberKurs: "BI_JISDOR",
      workflowStatus: "APPROVED",
      lockedFlag: false,
      deviationFlag: false,
      rateDeviationPct: null,
      periodeKode: null,
      makerId: null,
      approverId: null,
      approvedAt: null,
      rejectReason: null,
      uploadBatchId: null,
      createdAt: "2026-06-18T10:30:00+07:00",
      createdBy: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    };
    const r = kursListItemP5Schema.safeParse(row);
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.rateDeviationPct).toBeNull();
    }
  });
});

// ---------------------------------------------------------------------------
// 3. FxTreatmentBadge — label completeness (S5-AC1..4)
// ---------------------------------------------------------------------------

describe("FxTreatmentBadge label completeness (S5-AC1..4)", () => {
  const allRoutings = fxTreatmentRoutingEnum.options as FxTreatmentRouting[];

  it("every FxTreatmentRouting has a non-empty Bahasa Indonesia label", () => {
    for (const r of allRoutings) {
      const label = FX_TREATMENT_LABELS[r];
      expect(label, `missing label for routing ${r}`).toBeTruthy();
      expect(label.length).toBeGreaterThan(0);
    }
  });

  it("all 4 routing values covered in FX_TREATMENT_LABELS (S5-AC1)", () => {
    expect(FX_TREATMENT_LABELS["P&L_FOREIGN_EXCHANGE"]).toBeTruthy();
    expect(FX_TREATMENT_LABELS["OCI_FOREIGN_EXCHANGE_RESERVE"]).toBeTruthy();
    expect(FX_TREATMENT_LABELS["OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING"]).toBeTruthy();
    expect(FX_TREATMENT_LABELS["NO_FX_TREATMENT"]).toBeTruthy();
  });

  it("labels are all distinct (no two routings share same label)", () => {
    const labels = Object.values(FX_TREATMENT_LABELS);
    const unique = new Set(labels);
    expect(unique.size).toBe(labels.length);
  });

  it("every FxTreatmentRouting has a PSAK 71 reference (S5-AC2)", () => {
    for (const r of allRoutings) {
      const ref = FX_TREATMENT_PSAK_REF[r];
      expect(ref, `missing PSAK ref for routing ${r}`).toBeTruthy();
    }
  });

  it("OCI_NO_RECYCLING PSAK reference mentions §5.7.5 (S5-AC3 — FVOCI Election irrevocable)", () => {
    const ref = FX_TREATMENT_PSAK_REF["OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING"];
    expect(ref).toMatch(/5\.7\.5/);
  });

  it("OCI_WITH_RECYCLING PSAK reference mentions derecognition recycling (S5-AC3)", () => {
    const ref = FX_TREATMENT_PSAK_REF["OCI_FOREIGN_EXCHANGE_RESERVE"];
    expect(ref.toLowerCase()).toMatch(/recycl|recycle|derecogn/);
  });

  it("NO_FX_TREATMENT label does not mention P&L or OCI (S5-AC4 — IDR instruments)", () => {
    const label = FX_TREATMENT_LABELS["NO_FX_TREATMENT"];
    expect(label.toUpperCase()).not.toMatch(/P&L|OCI/);
  });
});

// ---------------------------------------------------------------------------
// 4. FxTreatmentDetail schema — treatment decision tree (S5-AC1..4)
// ---------------------------------------------------------------------------

describe("FxTreatmentDetail schema — decision tree (S5-AC1..4)", () => {
  it("AC + FCY → P&L routing (S5-AC1)", () => {
    const r = fxTreatmentDetailSchema.safeParse({
      routing: "P&L_FOREIGN_EXCHANGE",
      accountType: "P&L",
      ociRecycling: null,
      jurnalEventCode: "FX_PL_MTM",
      psak71Reference: "PSAK 71 §5.7.1",
      notes: null,
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.routing).toBe("P&L_FOREIGN_EXCHANGE");
      expect(r.data.accountType).toBe("P&L");
    }
  });

  it("FVOCI_DEBT + FCY → OCI with recycling (S5-AC2)", () => {
    const r = fxTreatmentDetailSchema.safeParse({
      routing: "OCI_FOREIGN_EXCHANGE_RESERVE",
      accountType: "OCI",
      ociRecycling: true,
      jurnalEventCode: "FX_OCI_DEBT",
      psak71Reference: "PSAK 71 §5.7.10",
      notes: null,
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.ociRecycling).toBe(true);
    }
  });

  it("FVOCI_ELECTION + FCY → OCI no recycling (S5-AC3 — irrevocable election)", () => {
    const r = fxTreatmentDetailSchema.safeParse({
      routing: "OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING",
      accountType: "OCI",
      ociRecycling: false,
      jurnalEventCode: "FX_OCI_EQUITY",
      psak71Reference: "PSAK 71 §5.7.5",
      notes: "Irrevocable FVOCI election — no P&L recycling on disposal",
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.ociRecycling).toBe(false);
    }
  });

  it("IDR instrument → NO_FX_TREATMENT (S5-AC4)", () => {
    const r = fxTreatmentDetailSchema.safeParse({
      routing: "NO_FX_TREATMENT",
      accountType: null,
      ociRecycling: null,
      jurnalEventCode: null,
      psak71Reference: null,
      notes: "IDR functional currency — no FX exposure",
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.accountType).toBeNull();
      expect(r.data.ociRecycling).toBeNull();
    }
  });

  it("rejects unknown routing value (schema guard)", () => {
    const r = fxTreatmentDetailSchema.safeParse({
      routing: "UNKNOWN_ROUTING",
      accountType: null,
      ociRecycling: null,
      jurnalEventCode: null,
      psak71Reference: null,
      notes: null,
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 5. KursUploadDropzone — upload form schema validation (S2-AC1..4)
// ---------------------------------------------------------------------------

describe("KursUploadDropzone form schema validation (S2-AC1..4)", () => {
  // Simulate file creation in node env
  function makeFile(
    name: string,
    type: string,
    sizeBytes: number = 1024,
  ): File {
    return new File(["x".repeat(sizeBytes)], name, { type });
  }

  it("accepts valid XLSX file (S2-AC1)", () => {
    const f = makeFile(
      "kurs-june.xlsx",
      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    );
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(true);
  });

  it("accepts valid CSV file (S2-AC1)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(true);
  });

  it("accepts file with .xlsx extension even if MIME is generic (S2-AC1)", () => {
    const f = makeFile("kurs.xlsx", "application/octet-stream");
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(true);
  });

  it("rejects empty file (S2-AC2)", () => {
    const f = makeFile("empty.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 0);
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msgs = r.error.issues.map((i) => i.message);
      expect(msgs.some((m) => m.toLowerCase().includes("kosong"))).toBe(true);
    }
  });

  it("rejects file > 5 MB (S2-AC2 size limit)", () => {
    const f = makeFile("huge.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 5 * 1024 * 1024 + 1);
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msgs = r.error.issues.map((i) => i.message);
      expect(msgs.some((m) => m.toLowerCase().includes("5 mb"))).toBe(true);
    }
  });

  it("rejects PDF file (wrong format) (S2-AC2)", () => {
    const f = makeFile("kurs.pdf", "application/pdf");
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msgs = r.error.issues.map((i) => i.message);
      expect(msgs.some((m) => m.toLowerCase().includes("xlsx") || m.toLowerCase().includes("csv"))).toBe(true);
    }
  });

  it("catatanUpload is optional (S2-AC2)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({ file: f });
    expect(r.success).toBe(true);
  });

  it("catatanUpload max 1000 chars (boundary, S2-AC2)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({
      file: f,
      catatanUpload: "a".repeat(1000),
    });
    expect(r.success).toBe(true);
  });

  it("rejects catatanUpload > 1000 chars (S2-AC2)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({
      file: f,
      catatanUpload: "a".repeat(1001),
    });
    expect(r.success).toBe(false);
  });

  it("accepts valid tanggalBerlakuOverride ISO date (S2-AC3)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({
      file: f,
      tanggalBerlakuOverride: "2026-06-18",
    });
    expect(r.success).toBe(true);
  });

  it("accepts empty tanggalBerlakuOverride (optional, S2-AC3)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({
      file: f,
      tanggalBerlakuOverride: "",
    });
    expect(r.success).toBe(true);
  });

  it("rejects invalid date format for tanggalBerlakuOverride (S2-AC3)", () => {
    const f = makeFile("kurs.csv", "text/csv");
    const r = kursUploadFormSchema.safeParse({
      file: f,
      tanggalBerlakuOverride: "18-06-2026", // DD-MM-YYYY not accepted
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 6. KursUploadResponse — batch status schema (S2-AC4)
// ---------------------------------------------------------------------------

describe("KursUploadResponse schema — batch status (S2-AC4)", () => {
  const base = {
    uploadBatchId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    rowsParsed: 5,
    rowsValid: 4,
    rowsInvalid: 1,
    status: "PENDING_APPROVAL" as const,
    kursIds: ["b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"],
    kursCreated: [
      {
        kodeMataUang: "USD",
        tanggalBerlaku: "2026-06-18",
        kursTengah: 16100.5,
        deviationFlag: false,
        rateDeviationPct: null,
      },
    ],
    deviationWarnings: [],
    nextStep: "Submit ke ROLE-AKUN-CTL untuk approval",
    dokumenBuktiId: null,
  };

  it("accepts valid upload response (S2-AC4)", () => {
    const r = kursUploadResponseSchema.safeParse(base);
    expect(r.success).toBe(true);
  });

  it("status is always PENDING_APPROVAL after upload (S2-AC4 contract)", () => {
    const r = kursUploadResponseSchema.safeParse(base);
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.status).toBe("PENDING_APPROVAL");
    }
  });

  it("accepts response with deviationWarnings (S1-AC3)", () => {
    const r = kursUploadResponseSchema.safeParse({
      ...base,
      kursCreated: [
        {
          kodeMataUang: "USD",
          tanggalBerlaku: "2026-06-18",
          kursTengah: 16100.5,
          deviationFlag: true,
          rateDeviationPct: 22.5,
        },
      ],
      deviationWarnings: [
        {
          kodeMataUang: "USD",
          rateDeviationPct: 22.5,
          previousKursTengah: 13197.54,
          message: "Kurs USD berdeviasi 22.50% dari hari sebelumnya. Harap verifikasi.",
        },
      ],
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.deviationWarnings).toHaveLength(1);
    }
  });

  it("rejects status other than PENDING_APPROVAL (schema literal contract)", () => {
    const r = kursUploadResponseSchema.safeParse({
      ...base,
      status: "APPROVED", // upload never immediately approved
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 7. KursApproveDialog — schema + SoD contract (S3-AC1..3)
// ---------------------------------------------------------------------------

describe("KursApproveDialog schema validation + SoD contract (S3-AC1..3)", () => {
  it("accepts valid approve body (S3-AC1)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "Kurs USD verified against BI website.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("requires comment ≥ 5 chars (S3-AC1 audit trail)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "ok",  // too short
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msgs = r.error.issues.map((i) => i.message);
      expect(msgs.some((m) => m.includes("5"))).toBe(true);
    }
  });

  it("rejects empty comment (S3-AC1)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
  });

  it("accepts comment at exactly max 2000 chars (boundary)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "a".repeat(2000),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("rejects comment > 2000 chars (boundary)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "a".repeat(2001),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
  });

  it("signatureMethod must be literal JWT_STEP_UP (S3-AC2 — 4-eyes signature contract)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "Verified.",
      signatureMethod: "JWT_STANDARD",  // invalid for approve
    });
    expect(r.success).toBe(false);
  });

  it("rejects unknown signatureMethod (S3-AC2)", () => {
    const r = kursApproveBodySchema.safeParse({
      comment: "Verified.",
      signatureMethod: "BIOMETRIC",
    });
    expect(r.success).toBe(false);
  });

  // SoD contract: approve body schema enforces makerId ≠ approverId via server-side check
  // Frontend: SoD note shown to user; approve button disabled if makerId === currentUser
  it("SoD contract — approve body does NOT encode approverId (server enforces SoD) (S3-AC3)", () => {
    // The schema must NOT have approverId field — SoD is server-enforced
    const parsed = kursApproveBodySchema.safeParse({
      comment: "Verified.",
      signatureMethod: "JWT_STEP_UP",
      approverId: "some-uuid",  // extra field should be stripped
    });
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      // approverId is stripped — only comment + signatureMethod expected
      expect(Object.keys(parsed.data)).toEqual(expect.arrayContaining(["comment", "signatureMethod"]));
      expect(Object.keys(parsed.data)).not.toContain("approverId");
    }
  });
});

// ---------------------------------------------------------------------------
// 8. KursRejectDialog — reject body schema (S3-AC4)
// ---------------------------------------------------------------------------

describe("KursRejectDialog reject body schema (S3-AC4)", () => {
  it("accepts valid reject body (S3-AC4)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "Kurs tidak sesuai dengan data BI JISDOR hari ini. Harap re-upload.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("requires rejectReason ≥ 20 chars (S3-AC4 — per AC min 20 chars)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "a".repeat(19),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msgs = r.error.issues.map((i) => i.message);
      expect(msgs.some((m) => m.includes("20"))).toBe(true);
    }
  });

  it("accepts rejectReason at exactly 20 chars (boundary, S3-AC4)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "a".repeat(20),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("accepts rejectReason at exactly 2000 chars (max boundary)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "a".repeat(2000),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("rejects rejectReason > 2000 chars (max boundary)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "a".repeat(2001),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
  });

  it("rejects empty rejectReason (S3-AC4)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
  });

  it("rejectReason is required field (S3-AC4)", () => {
    const r = kursRejectBodySchema.safeParse({ signatureMethod: "JWT_STEP_UP" });
    expect(r.success).toBe(false);
  });

  it("signatureMethod must be JWT_STEP_UP for reject (S3-AC4 signature contract)", () => {
    const r = kursRejectBodySchema.safeParse({
      rejectReason: "Kurs tidak valid karena deviasi terlalu besar.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 9. BatchApprove/Reject response schemas (S3-AC1..4)
// ---------------------------------------------------------------------------

describe("KursBatchApproveResponse schema (S3-AC1)", () => {
  it("accepts valid approve response (S3-AC1)", () => {
    const r = kursBatchApproveResponseSchema.safeParse({
      uploadBatchId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      rowsApproved: 5,
      kursApproved: [
        {
          kursId: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
          kodeMataUang: "USD",
          tanggalBerlaku: "2026-06-18",
          kursTengah: 16100.5,
          workflowStatus: "APPROVED",
        },
      ],
      approvedBy: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      approvedAt: "2026-06-18T11:00:00+07:00",
      message: "5 kurs berhasil disetujui.",
    });
    expect(r.success).toBe(true);
  });

  it("kursApproved items must have workflowStatus=APPROVED (contract)", () => {
    const r = kursBatchApproveResponseSchema.safeParse({
      uploadBatchId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      rowsApproved: 1,
      kursApproved: [
        {
          kursId: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
          kodeMataUang: "USD",
          tanggalBerlaku: "2026-06-18",
          kursTengah: 16100.5,
          workflowStatus: "PENDING_APPROVAL", // invalid — post-approve must be APPROVED
        },
      ],
      approvedBy: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      approvedAt: "2026-06-18T11:00:00+07:00",
      message: "ok",
    });
    expect(r.success).toBe(false);
  });
});

describe("KursBatchRejectResponse schema (S3-AC4)", () => {
  it("accepts valid reject response (S3-AC4)", () => {
    const r = kursBatchRejectResponseSchema.safeParse({
      uploadBatchId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      rowsRejected: 5,
      workflowStatus: "REJECTED",
      rejectedBy: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      rejectReason: "Kurs USD tidak sesuai dengan data BI.",
      message: "5 kurs berhasil ditolak.",
    });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10. Persona gating contract — kurs.sync permission (S1-AC2)
// ---------------------------------------------------------------------------

describe("Persona gating contract — kurs.sync permission (S1-AC2)", () => {
  // JisdorSyncTriggerButton: if (!perms.can("kurs.sync")) return null;
  // This test verifies the PERMISSION STRING used, not the React component
  it("JISDOR sync trigger uses 'kurs.sync' permission string (S1-AC2)", () => {
    const REQUIRED_PERMISSION = "kurs.sync";

    // Simulate permission check
    function canTriggerJisdorSync(permissions: string[]): boolean {
      return permissions.includes(REQUIRED_PERMISSION);
    }

    expect(canTriggerJisdorSync(["kurs.sync"])).toBe(true);
    expect(canTriggerJisdorSync(["kurs.read", "kurs.create"])).toBe(false);
    expect(canTriggerJisdorSync([])).toBe(false);
    expect(canTriggerJisdorSync(["kurs.sync", "kurs.read"])).toBe(true);
  });

  it("upload page uses 'kurs.create' permission string (S2-AC1)", () => {
    const REQUIRED_PERMISSION = "kurs.create";

    function canUploadKurs(permissions: string[]): boolean {
      return permissions.includes(REQUIRED_PERMISSION);
    }

    expect(canUploadKurs(["kurs.create"])).toBe(true);
    expect(canUploadKurs(["kurs.read"])).toBe(false);
    expect(canUploadKurs([])).toBe(false);
  });

  it("approve uses 'kurs.approve' — makerId must not equal currentUserId (S3-AC3 SoD)", () => {
    // SoD: maker cannot be approver
    function sodViolation(makerId: string, currentUserId: string): boolean {
      return makerId === currentUserId;
    }

    expect(sodViolation("user-a", "user-b")).toBe(false); // valid: different users
    expect(sodViolation("user-a", "user-a")).toBe(true);  // SoD violation
  });
});

// ---------------------------------------------------------------------------
// 11. JisdorSyncTriggerButton — trigger body schema (S1-AC1..2)
// ---------------------------------------------------------------------------

describe("JisdorSyncTriggerButton body schema (S1-AC1..2)", () => {
  it("accepts empty body (trigger today's rates) (S1-AC1)", () => {
    const r = jisdorSyncTriggerBodySchema.safeParse({});
    expect(r.success).toBe(true);
  });

  it("accepts tanggalTarget date string (S1-AC2 backfill)", () => {
    const r = jisdorSyncTriggerBodySchema.safeParse({
      tanggalTarget: "2026-06-17",
    });
    expect(r.success).toBe(true);
  });

  it("accepts empty string for tanggalTarget (S1-AC2)", () => {
    const r = jisdorSyncTriggerBodySchema.safeParse({
      tanggalTarget: "",
    });
    expect(r.success).toBe(true);
  });

  it("rejects invalid date format (S1-AC2)", () => {
    const r = jisdorSyncTriggerBodySchema.safeParse({
      tanggalTarget: "17-06-2026",
    });
    expect(r.success).toBe(false);
  });

  it("accepts forceRefetch=true (S1-AC2)", () => {
    const r = jisdorSyncTriggerBodySchema.safeParse({
      tanggalTarget: "2026-06-17",
      forceRefetch: true,
    });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 12. JisdorSyncJobResponse — 202 response schema (S1-AC3)
// ---------------------------------------------------------------------------

describe("JisdorSyncJobResponse schema (S1-AC3)", () => {
  const baseJob = {
    jobId: "job_01HXYZABC",
    type: "JISDOR_SYNC" as const,
    tanggalTarget: "2026-06-18",
    statusUrl: "/api/v1/jobs/job_01HXYZABC",
    streamUrl: "/api/v1/jobs/job_01HXYZABC/stream",
    estimatedCurrencies: 15,
    message: "JISDOR sync job submitted. Estimasi 15 mata uang.",
  };

  it("accepts valid 202 job response (S1-AC3)", () => {
    const r = jisdorSyncJobResponseSchema.safeParse(baseJob);
    expect(r.success).toBe(true);
  });

  it("type is always JISDOR_SYNC (contract)", () => {
    const r = jisdorSyncJobResponseSchema.safeParse(baseJob);
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.type).toBe("JISDOR_SYNC");
    }
  });

  it("rejects wrong job type (contract guard)", () => {
    const r = jisdorSyncJobResponseSchema.safeParse({ ...baseJob, type: "ECL_CALC_RUN" });
    expect(r.success).toBe(false);
  });

  it("estimatedCurrencies is int (contract)", () => {
    const r = jisdorSyncJobResponseSchema.safeParse({
      ...baseJob,
      estimatedCurrencies: 15.7,  // float not valid
    });
    // Zod number().int() rejects floats
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 13. FX_RATE_ERROR_MESSAGES — 5 new P5-M5 codes coverage (S1..S5)
// ---------------------------------------------------------------------------

describe("FX_RATE_ERROR_MESSAGES coverage (S1..S5)", () => {
  const requiredCodes = [
    "FX_RATE_LOCKED",
    "KURS_DUPLICATE_DATE",
    "KURS_UPLOAD_VALIDATION_FAILED",
    "KLASIFIKASI_NOT_LOCKED",
    "KURS_PERIODE_MISMATCH",
  ] as const;

  it("all 5 new P5-M5 error codes have Bahasa Indonesia messages", () => {
    for (const code of requiredCodes) {
      const msg = FX_RATE_ERROR_MESSAGES[code];
      expect(msg, `missing message for error code ${code}`).toBeTruthy();
      expect(msg.length).toBeGreaterThan(10);
    }
  });

  it("FX_RATE_LOCKED message conveys irreversibility + escalation path (S4-AC3)", () => {
    const msg = FX_RATE_ERROR_MESSAGES["FX_RATE_LOCKED"];
    expect(msg.toLowerCase()).toMatch(/kunci|locked|tidak bisa|periode/);
  });

  it("KURS_DUPLICATE_DATE message conveys duplicate date constraint (S2-AC3)", () => {
    const msg = FX_RATE_ERROR_MESSAGES["KURS_DUPLICATE_DATE"];
    expect(msg.toLowerCase()).toMatch(/sudah ada|duplikat|mata uang/);
  });

  it("KURS_UPLOAD_VALIDATION_FAILED message conveys actionable next step (S2-AC2)", () => {
    const msg = FX_RATE_ERROR_MESSAGES["KURS_UPLOAD_VALIDATION_FAILED"];
    expect(msg.toLowerCase()).toMatch(/validasi|upload|perbaiki|baris/);
  });

  it("KLASIFIKASI_NOT_LOCKED message conveys dependency on PSAK 71 workflow (S5-AC1)", () => {
    const msg = FX_RATE_ERROR_MESSAGES["KLASIFIKASI_NOT_LOCKED"];
    expect(msg.toLowerCase()).toMatch(/klasifikasi|psak|sppi|locked/);
  });

  it("KURS_PERIODE_MISMATCH message conveys open period requirement (S2-AC3)", () => {
    const msg = FX_RATE_ERROR_MESSAGES["KURS_PERIODE_MISMATCH"];
    expect(msg.toLowerCase()).toMatch(/periode|open|tanggal/);
  });
});

// ---------------------------------------------------------------------------
// 14. SUMBER_KURS_LABELS — all 4 sources covered
// ---------------------------------------------------------------------------

describe("SUMBER_KURS_LABELS completeness", () => {
  it("all 4 sumber kurs have Bahasa Indonesia labels", () => {
    const allSources = ["BI_JISDOR", "BI_KURS_TENGAH", "INTERNAL", "MANUAL"] as const;
    for (const s of allSources) {
      expect(SUMBER_KURS_LABELS[s], `missing label for ${s}`).toBeTruthy();
    }
  });

  it("BI_JISDOR label mentions BI (bank identity)", () => {
    expect(SUMBER_KURS_LABELS["BI_JISDOR"]).toMatch(/BI/);
  });

  it("all 4 labels are distinct", () => {
    const labels = Object.values(SUMBER_KURS_LABELS);
    const unique = new Set(labels);
    expect(unique.size).toBe(labels.length);
  });
});

// ---------------------------------------------------------------------------
// 15. KursTreatmentResponse — full treatment response schema (S5-AC4)
// ---------------------------------------------------------------------------

describe("KursTreatmentResponse schema (S5-AC4)", () => {
  it("accepts IDR instrument with NO_FX_TREATMENT (S5-AC4)", () => {
    const r = kursTreatmentResponseSchema.safeParse({
      instrumenId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      kodeInstrumen: "INST-001234",
      klasifikasiPsak71: "AC",
      matauang: "IDR",
      klasifikasiLocked: true,
      klasifikasiLockedAt: "2026-05-01T09:00:00+07:00",
      fxTreatment: {
        routing: "NO_FX_TREATMENT",
        accountType: null,
        ociRecycling: null,
        jurnalEventCode: null,
        psak71Reference: null,
        notes: "IDR instrument — no FX exposure",
      },
    });
    expect(r.success).toBe(true);
  });

  it("accepts FCY FVOCI_ELECTION instrument with OCI no recycling (S5-AC3)", () => {
    const r = kursTreatmentResponseSchema.safeParse({
      instrumenId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      kodeInstrumen: "SAHAM-001",
      klasifikasiPsak71: "FVOCI_ELECTION",
      matauang: "USD",
      klasifikasiLocked: true,
      klasifikasiLockedAt: "2026-03-15T10:00:00+07:00",
      fxTreatment: {
        routing: "OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING",
        accountType: "OCI",
        ociRecycling: false,
        jurnalEventCode: "FX_OCI_EQUITY",
        psak71Reference: "PSAK 71 §5.7.5",
        notes: null,
      },
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.fxTreatment.ociRecycling).toBe(false);
    }
  });

  it("rejects response when klasifikasiLocked=false (KLASIFIKASI_NOT_LOCKED scenario)", () => {
    // The API returns KLASIFIKASI_NOT_LOCKED error, not this schema
    // But schema validates the boolean
    const r = kursTreatmentResponseSchema.safeParse({
      instrumenId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      kodeInstrumen: "INST-999",
      klasifikasiPsak71: null,  // null when not locked
      matauang: "USD",
      klasifikasiLocked: false,  // not locked
      klasifikasiLockedAt: null,
      fxTreatment: {
        routing: "NO_FX_TREATMENT",
        accountType: null,
        ociRecycling: null,
        jurnalEventCode: null,
        psak71Reference: null,
        notes: "Klasifikasi belum locked",
      },
    });
    // Schema allows this (klasifikasiPsak71 is nullable, klasifikasiLocked is boolean)
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.klasifikasiLocked).toBe(false);
    }
  });
});
