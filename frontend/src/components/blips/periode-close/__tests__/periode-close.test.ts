/**
 * Vitest unit tests — P5-M4 Periode Close component logic (node environment)
 *
 * Tests pure-logic helpers and data contracts for all 14 components.
 * No DOM rendering — for E2E use Playwright in frontend/tests/e2e/periode-buku/.
 *
 * AC coverage:
 *   S1-AC1..4   SoftCloseRequestDialog schema + checklist gate logic
 *   S2-AC1..4   SoftCloseApproveDialog schema + SoD contract
 *   S3-AC1..4   HardCloseRequest/Approve/Reject schema + MFA scope contract
 *   S4-AC1..3   Reopen schema + grace window + target status options
 *   S5-AC1..4   PeriodeStatusBadge labels + badge config completeness
 *               + error code messages + checklist item labels
 */

import { describe, it, expect } from "vitest";
import {
  statusPeriodeEnum,
  softCloseRequestBodySchema,
  workflowApproveBodySchema,
  hardCloseRequestBodySchema,
  rejectBodySchema,
  reopenRequestBodySchema,
  mfaStepUpRequestSchema,
  periodeBukuListItemSchema,
  closingChecklistResponseSchema,
  STATUS_PERIODE_LABELS,
  CHECKLIST_ITEM_LABELS,
  TIPE_PERIODE_LABELS,
  BULAN_LABELS,
  CHECKLIST_TRANSITION_LABELS,
  PERIODE_CLOSE_ERROR_MESSAGES,
  type StatusPeriode,
  type ChecklistItemKey,
  type MfaStepUpScope,
} from "@/lib/schemas/periode-close.schema";

// ---------------------------------------------------------------------------
// 1. PeriodeStatusBadge — all 4 StatusPeriode values have labels (S5-AC2)
// ---------------------------------------------------------------------------

describe("PeriodeStatusBadge label completeness (S5-AC2)", () => {
  const allStatuses = statusPeriodeEnum.options as StatusPeriode[];

  it("every StatusPeriode has a non-empty Bahasa Indonesia label", () => {
    for (const s of allStatuses) {
      const label = STATUS_PERIODE_LABELS[s];
      expect(label, `missing label for status ${s}`).toBeTruthy();
      expect(label.length).toBeGreaterThan(0);
    }
  });

  it("all 4 statuses covered: OPEN, SOFT_CLOSED, HARD_CLOSE_PENDING, CLOSED", () => {
    expect(STATUS_PERIODE_LABELS["OPEN"]).toBeTruthy();
    expect(STATUS_PERIODE_LABELS["SOFT_CLOSED"]).toBeTruthy();
    expect(STATUS_PERIODE_LABELS["HARD_CLOSE_PENDING"]).toBeTruthy();
    expect(STATUS_PERIODE_LABELS["CLOSED"]).toBeTruthy();
  });

  it("labels are all distinct", () => {
    const labels = Object.values(STATUS_PERIODE_LABELS);
    const unique = new Set(labels);
    expect(unique.size).toBe(labels.length);
  });

  it("CLOSED label is distinct from SOFT_CLOSED (user must distinguish)", () => {
    expect(STATUS_PERIODE_LABELS["CLOSED"]).not.toBe(STATUS_PERIODE_LABELS["SOFT_CLOSED"]);
  });
});

// ---------------------------------------------------------------------------
// 2. Checklist item labels — all 4 keys covered (S5-AC2)
// ---------------------------------------------------------------------------

describe("ClosingChecklistPanel item label completeness (S5-AC2)", () => {
  const allKeys: ChecklistItemKey[] = [
    "PENDING_APPROVAL_ZERO",
    "JURNAL_BALANCED",
    "GL_DELIVERED",
    "RECON_PASS",
  ];

  it("every ChecklistItemKey has a human-readable Bahasa Indonesia label", () => {
    for (const key of allKeys) {
      const label = CHECKLIST_ITEM_LABELS[key];
      expect(label, `missing label for key ${key}`).toBeTruthy();
      expect(label.length).toBeGreaterThan(0);
    }
  });

  it("all 4 checklist labels are distinct", () => {
    const labels = allKeys.map((k) => CHECKLIST_ITEM_LABELS[k]);
    const unique = new Set(labels);
    expect(unique.size).toBe(4);
  });
});

// ---------------------------------------------------------------------------
// 3. SoftCloseRequestDialog — schema validation (S1-AC1..4)
// ---------------------------------------------------------------------------

describe("SoftCloseRequestDialog schema validation (S1-AC1..4)", () => {
  it("accepts empty catatan (optional field, S1-AC2)", () => {
    const r = softCloseRequestBodySchema.safeParse({ catatan: "", rowVersion: 1 });
    expect(r.success).toBe(true);
  });

  it("accepts catatan of exactly 1000 chars (boundary S1-AC2)", () => {
    const r = softCloseRequestBodySchema.safeParse({
      catatan: "a".repeat(1000),
      rowVersion: 1,
    });
    expect(r.success).toBe(true);
  });

  it("rejects catatan exceeding 1000 chars (S1-AC2 boundary)", () => {
    const r = softCloseRequestBodySchema.safeParse({
      catatan: "a".repeat(1001),
      rowVersion: 1,
    });
    expect(r.success).toBe(false);
  });

  it("requires rowVersion ≥ 1 (optimistic lock, S1-AC3)", () => {
    const r = softCloseRequestBodySchema.safeParse({ rowVersion: 0 });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toMatch(/1/);
    }
  });

  it("rowVersion is required — rejects missing (S1-AC3)", () => {
    const r = softCloseRequestBodySchema.safeParse({ catatan: "test" });
    expect(r.success).toBe(false);
  });

  it("accepts valid rowVersion = 1 (S1-AC1)", () => {
    const r = softCloseRequestBodySchema.safeParse({ rowVersion: 1 });
    expect(r.success).toBe(true);
  });

  it("accepts catatan = undefined (optional, S1-AC2)", () => {
    const r = softCloseRequestBodySchema.safeParse({ rowVersion: 5 });
    expect(r.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 4. SoftCloseApproveDialog — schema validation + SoD (S2-AC1..4)
// ---------------------------------------------------------------------------

describe("SoftCloseApproveDialog schema validation (S2-AC1..4)", () => {
  it("requires comment (min 1 char, S2-AC2)", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].path).toContain("comment");
    }
  });

  it("accepts valid comment + JWT_STEP_UP (S2-AC1)", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "Approved. Semua posisi terverifikasi.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("accepts JWT_STANDARD signatureMethod (S2-AC1)", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "Approved.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(r.success).toBe(true);
  });

  it("rejects unknown signatureMethod (S2-AC4 — data integrity)", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "ok",
      signatureMethod: "BIOMETRIC",
    });
    expect(r.success).toBe(false);
  });

  it("comment max 2000 chars (boundary test)", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "a".repeat(2000),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(true);
  });

  it("rejects comment exceeding 2000 chars", () => {
    const r = workflowApproveBodySchema.safeParse({
      comment: "a".repeat(2001),
      signatureMethod: "JWT_STEP_UP",
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 5. HardCloseRequestDialog — schema validation (S3-AC1)
// ---------------------------------------------------------------------------

describe("HardCloseRequestDialog schema validation (S3-AC1)", () => {
  it("accepts valid hard-close request body (S3-AC1)", () => {
    const r = hardCloseRequestBodySchema.safeParse({
      catatan: "Hard close request periode Juni 2026",
      rowVersion: 3,
    });
    expect(r.success).toBe(true);
  });

  it("catatan is optional (S3-AC1)", () => {
    const r = hardCloseRequestBodySchema.safeParse({ rowVersion: 2 });
    expect(r.success).toBe(true);
  });

  it("rowVersion required ≥ 1 (S3-AC1 + optimistic lock)", () => {
    const r = hardCloseRequestBodySchema.safeParse({ catatan: "test", rowVersion: 0 });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 6. HardCloseRejectDialog — reject body schema (S3-AC4)
// ---------------------------------------------------------------------------

describe("HardCloseRejectDialog reject body schema (S3-AC4)", () => {
  it("requires reason ≥ 30 chars (S3-AC4 boundary)", () => {
    const exactlyThirty = rejectBodySchema.safeParse({ reason: "a".repeat(30) });
    expect(exactlyThirty.success).toBe(true);
  });

  it("rejects reason < 30 chars (S3-AC4 boundary)", () => {
    const r = rejectBodySchema.safeParse({ reason: "a".repeat(29) });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toMatch(/30/);
    }
  });

  it("rejects empty reason (S3-AC4)", () => {
    const r = rejectBodySchema.safeParse({ reason: "" });
    expect(r.success).toBe(false);
  });

  it("accepts reason exactly at max (1000 chars)", () => {
    const r = rejectBodySchema.safeParse({ reason: "a".repeat(1000) });
    expect(r.success).toBe(true);
  });

  it("rejects reason > 1000 chars", () => {
    const r = rejectBodySchema.safeParse({ reason: "a".repeat(1001) });
    expect(r.success).toBe(false);
  });

  it("reason is required field (S3-AC4)", () => {
    const r = rejectBodySchema.safeParse({});
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 7. ReopenRequestDialog — schema + target status (S4-AC1..3)
// ---------------------------------------------------------------------------

describe("ReopenRequestDialog schema validation (S4-AC1..3)", () => {
  it("accepts CLOSED→SOFT_CLOSED reopen with valid reason (S4-AC1)", () => {
    const r = reopenRequestBodySchema.safeParse({
      targetStatus: "SOFT_CLOSED",
      reason: "Koreksi akun 2030 diperlukan sesuai temuan audit internal.",
      rowVersion: 4,
    });
    expect(r.success).toBe(true);
  });

  it("accepts SOFT_CLOSED→OPEN reopen with valid reason (S4-AC3)", () => {
    const r = reopenRequestBodySchema.safeParse({
      targetStatus: "OPEN",
      reason: "Revisi jurnal manual diperlukan sebelum hard-close ulang.",
      rowVersion: 2,
    });
    expect(r.success).toBe(true);
  });

  it("requires reason ≥ 30 chars (S4-AC1 audit compliance)", () => {
    const r = reopenRequestBodySchema.safeParse({
      targetStatus: "SOFT_CLOSED",
      reason: "a".repeat(29),
      rowVersion: 1,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toMatch(/30/);
    }
  });

  it("rejects invalid targetStatus (must be OPEN or SOFT_CLOSED)", () => {
    const r = reopenRequestBodySchema.safeParse({
      targetStatus: "HARD_CLOSE_PENDING", // invalid
      reason: "a".repeat(30),
      rowVersion: 1,
    });
    expect(r.success).toBe(false);
  });

  it("reason is required (S4-AC2 audit trail)", () => {
    const r = reopenRequestBodySchema.safeParse({
      targetStatus: "OPEN",
      rowVersion: 1,
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 8. MFAStepUpDialog — TOTP schema (S3-AC2, S4-AC2)
// ---------------------------------------------------------------------------

describe("MFAStepUpDialog TOTP schema (S3-AC2, S4-AC2)", () => {
  const allScopes: MfaStepUpScope[] = ["hard_close", "reopen_closed"];

  it("each MfaStepUpScope value is valid", () => {
    for (const scope of allScopes) {
      const r = mfaStepUpRequestSchema.safeParse({ totpCode: "123456", scope });
      expect(r.success, `scope ${scope} should be valid`).toBe(true);
    }
  });

  it("accepts exactly 6 numeric digits (S3-AC2)", () => {
    const r = mfaStepUpRequestSchema.safeParse({
      totpCode: "654321",
      scope: "hard_close",
    });
    expect(r.success).toBe(true);
  });

  it("rejects TOTP code shorter than 6 digits (S3-AC2 boundary)", () => {
    const r = mfaStepUpRequestSchema.safeParse({
      totpCode: "12345",
      scope: "hard_close",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toMatch(/6/);
    }
  });

  it("rejects TOTP code longer than 6 digits (S3-AC2 boundary)", () => {
    const r = mfaStepUpRequestSchema.safeParse({
      totpCode: "1234567",
      scope: "hard_close",
    });
    expect(r.success).toBe(false);
  });

  it("rejects non-numeric TOTP code (S3-AC2)", () => {
    const r = mfaStepUpRequestSchema.safeParse({
      totpCode: "12345a",
      scope: "hard_close",
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.error.issues[0].message).toMatch(/angka/);
    }
  });

  it("rejects empty totpCode (S3-AC2)", () => {
    const r = mfaStepUpRequestSchema.safeParse({ totpCode: "", scope: "reopen_closed" });
    expect(r.success).toBe(false);
  });

  it("rejects unknown scope (S3-AC2 security gate)", () => {
    const r = mfaStepUpRequestSchema.safeParse({
      totpCode: "123456",
      scope: "delete_all", // invalid
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 9. ClosingChecklistPanel — response schema (S1-AC1, S3-AC1)
// ---------------------------------------------------------------------------

describe("ClosingChecklistPanel response schema (S1-AC1, S3-AC1)", () => {
  // closingChecklistResponseSchema uses FLAT fields (not nested currentChecklist)
  const makeChecklist = (overrides: Record<string, unknown> = {}) => ({
    periodeId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    periodeKode: "PRD-2026-06",
    statusPeriode: "OPEN",
    evaluatedAt: "2026-06-17T10:00:00+07:00",
    allPassed: true,
    isRealTimeEval: true,
    items: [
      { key: "PENDING_APPROVAL_ZERO", label: "0 transaksi PENDING_APPROVAL", passed: true },
      { key: "JURNAL_BALANCED", label: "Semua jurnal balanced", passed: true },
      { key: "GL_DELIVERED", label: "Tidak ada GL FAILED", passed: true },
      { key: "RECON_PASS", label: "GL recon COMPLETED", passed: true },
    ],
    ...overrides,
  });

  it("accepts valid all-passed checklist response (S1-AC1)", () => {
    const r = closingChecklistResponseSchema.safeParse(makeChecklist());
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.allPassed).toBe(true);
      expect(r.data.items).toHaveLength(4);
    }
  });

  it("accepts checklist with some items failed (S1-AC1 validation scenario)", () => {
    const r = closingChecklistResponseSchema.safeParse(
      makeChecklist({
        allPassed: false,
        items: [
          { key: "PENDING_APPROVAL_ZERO", label: "0 pending", passed: false, detail: "3 masih PENDING", actionUrl: "/transaksi/pending" },
          { key: "JURNAL_BALANCED", label: "Balanced", passed: true },
          { key: "GL_DELIVERED", label: "GL ok", passed: true },
          { key: "RECON_PASS", label: "Recon ok", passed: true },
        ],
      }),
    );
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.allPassed).toBe(false);
      expect(r.data.items[0].passed).toBe(false);
    }
  });

  it("does not have isStale at top level — it is not in schema (schema contract)", () => {
    // closingChecklistResponseSchema has no isStale field; passing it should be ignored
    const r = closingChecklistResponseSchema.safeParse(makeChecklist({ isStale: true }));
    // Extra fields are stripped but schema still accepts the input (passthrough behavior)
    // Zod strips unknown keys by default so it won't fail on extra fields
    expect(r.success).toBe(true);
  });

  it("requires periodeId to be UUID (S5-AC4 schema contract)", () => {
    const r = closingChecklistResponseSchema.safeParse(
      makeChecklist({ periodeId: "not-a-uuid" }),
    );
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 10. PeriodeBukuListItem — schema validation (S5-AC1)
// ---------------------------------------------------------------------------

describe("PeriodeBukuListItem schema (S5-AC1)", () => {
  const baseItem = {
    id: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    periodeKode: "PRD-2026-06",
    tahunBuku: 2026,
    bulan: 6,
    tipePeriode: "BULANAN",
    statusPeriode: "OPEN",
    tanggalMulai: "2026-06-01",
    tanggalAkhir: "2026-06-30",
    rowVersion: 1,
  };

  it("accepts minimal valid list item (S5-AC1)", () => {
    const r = periodeBukuListItemSchema.safeParse(baseItem);
    expect(r.success).toBe(true);
  });

  it("accepts CLOSED item with grace expiry (S5-AC1)", () => {
    const r = periodeBukuListItemSchema.safeParse({
      ...baseItem,
      statusPeriode: "CLOSED",
      tanggalSoftClose: "2026-06-28T16:00:00+07:00",
      tanggalHardClose: "2026-06-30T10:00:00+07:00",
      hardCloseGraceExpiresAt: "2026-07-02T10:00:00+07:00",
      rowVersion: 5,
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.statusPeriode).toBe("CLOSED");
      expect(r.data.hardCloseGraceExpiresAt).toBeTruthy();
    }
  });

  it("rejects invalid bulan (must be 1-12)", () => {
    const r = periodeBukuListItemSchema.safeParse({ ...baseItem, bulan: 13 });
    expect(r.success).toBe(false);
  });

  it("rejects invalid statusPeriode enum value", () => {
    const r = periodeBukuListItemSchema.safeParse({
      ...baseItem,
      statusPeriode: "PENDING",
    });
    expect(r.success).toBe(false);
  });

  it("rejects rowVersion < 1 (S1-AC3 optimistic lock)", () => {
    const r = periodeBukuListItemSchema.safeParse({ ...baseItem, rowVersion: 0 });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 11. Error code messages — 7 new P5-M4 codes covered (S1..S4)
// ---------------------------------------------------------------------------

describe("PERIODE_CLOSE_ERROR_MESSAGES coverage (S1..S4)", () => {
  const requiredCodes = [
    "CLOSING_CHECKLIST_FAILED",
    "CLOSING_CHECKLIST_STALE",
    "PERIODE_SOFT_CLOSED",
    "MFA_STEP_UP_REQUIRED",
    "MFA_STEP_UP_EXPIRED",
    "PERIODE_GRACE_EXPIRED",
    "SOFT_CLOSE_PENDING_EXISTS",
  ] as const;

  it("all 7 new error codes have Bahasa Indonesia messages", () => {
    for (const code of requiredCodes) {
      const msg = PERIODE_CLOSE_ERROR_MESSAGES[code];
      expect(msg, `missing message for error code ${code}`).toBeTruthy();
      expect(msg.length).toBeGreaterThan(10);
    }
  });

  it("PERIODE_GRACE_EXPIRED message conveys irreversibility (S4-AC1)", () => {
    const msg = PERIODE_CLOSE_ERROR_MESSAGES["PERIODE_GRACE_EXPIRED"];
    // Must convey that grace window has ended
    expect(msg.toLowerCase()).toMatch(/grace|tidak dapat|sudah berakhir|tidak bisa/);
  });

  it("MFA_STEP_UP_EXPIRED message conveys retry instruction (S3-AC2)", () => {
    const msg = PERIODE_CLOSE_ERROR_MESSAGES["MFA_STEP_UP_EXPIRED"];
    // Must mention the expiry
    expect(msg.toLowerCase()).toMatch(/expired|ulangi|expired|5 menit/);
  });

  it("CLOSING_CHECKLIST_FAILED message conveys actionable next step (S1-AC1)", () => {
    const msg = PERIODE_CLOSE_ERROR_MESSAGES["CLOSING_CHECKLIST_FAILED"];
    expect(msg.toLowerCase()).toMatch(/checklist|gagal|selesaikan/);
  });
});

// ---------------------------------------------------------------------------
// 12. Helper label maps — BULAN, TIPE_PERIODE, CHECKLIST_TRANSITION
// ---------------------------------------------------------------------------

describe("Label map completeness", () => {
  it("BULAN_LABELS covers all 12 months", () => {
    for (let m = 1; m <= 12; m++) {
      expect(BULAN_LABELS[m], `missing label for bulan ${m}`).toBeTruthy();
    }
  });

  it("TIPE_PERIODE_LABELS covers BULANAN, KUARTALAN, TAHUNAN", () => {
    expect(TIPE_PERIODE_LABELS["BULANAN"]).toBeTruthy();
    expect(TIPE_PERIODE_LABELS["KUARTALAN"]).toBeTruthy();
    expect(TIPE_PERIODE_LABELS["TAHUNAN"]).toBeTruthy();
  });

  it("CHECKLIST_TRANSITION_LABELS covers all 6 transitions", () => {
    const transitions = [
      "SOFT_CLOSE_REQUEST",
      "SOFT_CLOSE_APPROVE",
      "HARD_CLOSE_REQUEST",
      "HARD_CLOSE_APPROVE",
      "REOPEN_REQUEST",
      "REOPEN_APPROVE",
    ] as const;
    for (const t of transitions) {
      expect(CHECKLIST_TRANSITION_LABELS[t], `missing label for ${t}`).toBeTruthy();
    }
  });

  it("BULAN_LABELS month 1 = Januari and month 12 = Desember", () => {
    expect(BULAN_LABELS[1]).toMatch(/januari/i);
    expect(BULAN_LABELS[12]).toMatch(/desember/i);
  });
});
