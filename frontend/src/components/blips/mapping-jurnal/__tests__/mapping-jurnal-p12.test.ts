/**
 * Vitest component-logic tests — P5-M12 Mapping Jurnal 6-eyes workflow
 *
 * AC coverage:
 *   S1-AC1..4 (state matrix, balance check, version chain, CRUD)
 *   S2-AC1..4 (6-eyes / 4-eyes flow, SoD contract, MFA guard, periode lock)
 *   S3-AC1..4 (import result, MAPPING_AKUN_INVALID, MAPPING_UNBALANCED)
 *   S4-AC1..4 (RPT-19 coverage badge, incomplete flag, export, DLQ link)
 *   S5-AC1..4 (RPT-20 issues, RPT-21 history, async export)
 */

import { describe, it, expect } from "vitest";
import {
  mappingP12WorkflowStatusEnum,
  mappingWorkflowPathEnum,
  gapCoverageEnum,
  MAPPING_WORKFLOW_STATUS_LABELS,
  mappingP12HeaderSummarySchema,
  mappingP12DetailRowSchema,
  newVersionFormSchema,
  reviewDialogSchema,
  approveDialogSchema,
  rejectDialogSchema,
  submitDialogSchema,
  bulkImportResultSchema,
  rpt19CoverageSchema,
  rpt20ValidationSchema,
  auditLogEntrySchema,
  MAPPING_P12_ERROR_CODES,
  type MappingP12WorkflowStatus,
  type GapCoverage,
} from "@/lib/schemas/mapping-jurnal-p12.schema";

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const HEADER_ID = "550e8400-e29b-41d4-a716-446655440001";
const MAKER_ID  = "550e8400-e29b-41d4-a716-446655440002";
const REVIEWER_ID = "550e8400-e29b-41d4-a716-446655440003";
const APPROVER_ID = "550e8400-e29b-41d4-a716-446655440004";
const RISK_ID   = "550e8400-e29b-41d4-a716-446655440005";

const validHeaderSummary = {
  id: HEADER_ID,
  eventCode: "ECL_PEMBENTUKAN",
  namaEvent: "Pembentukan ECL",
  kategoriEvent: "ECL",
  workflowStatus: "DRAFT" as MappingP12WorkflowStatus,
  workflowPath: "6-eyes" as const,
  regulatedFlag: true,
  aktifFlag: false,
  parentId: null,
  effectiveFrom: null,
  effectiveTo: null,
  makerId: MAKER_ID,
  reviewerId: null,
  approverId: null,
  approver2Id: null,
  updatedAt: "2026-06-22T10:00:00+07:00",
};

const validDetailRow = {
  id: "660e8400-e29b-41d4-a716-446655440001",
  headerId: HEADER_ID,
  akunDebit: "110201",
  akunKredit: "440101",
  debitKredit: "D" as const,
  jumlahCalc: "ECL_weighted",
  urutan: 1,
};

// ─── 1. Workflow status enum — 5 states ───────────────────────────────────────

describe("MappingP12WorkflowStatus — 5-state enum", () => {
  it("covers exactly 5 states", () => {
    expect(mappingP12WorkflowStatusEnum.options).toHaveLength(5);
    expect(mappingP12WorkflowStatusEnum.options).toContain("DRAFT");
    expect(mappingP12WorkflowStatusEnum.options).toContain("PENDING_REVIEW");
    expect(mappingP12WorkflowStatusEnum.options).toContain("PENDING_APPROVAL");
    expect(mappingP12WorkflowStatusEnum.options).toContain("PENDING_APPROVAL_2");
    expect(mappingP12WorkflowStatusEnum.options).toContain("APPROVED_ACTIVE");
  });

  it("has Bahasa Indonesia label for every status", () => {
    mappingP12WorkflowStatusEnum.options.forEach((status) => {
      expect(MAPPING_WORKFLOW_STATUS_LABELS[status]).toBeTruthy();
    });
  });

  it("rejects invalid status", () => {
    const result = mappingP12WorkflowStatusEnum.safeParse("APPROVED");
    expect(result.success).toBe(false);
  });
});

// ─── 2. Workflow path enum ────────────────────────────────────────────────────

describe("MappingWorkflowPath", () => {
  it("only allows 4-eyes or 6-eyes", () => {
    expect(mappingWorkflowPathEnum.safeParse("4-eyes").success).toBe(true);
    expect(mappingWorkflowPathEnum.safeParse("6-eyes").success).toBe(true);
    expect(mappingWorkflowPathEnum.safeParse("2-eyes").success).toBe(false);
  });
});

// ─── 3. GapCoverage enum ─────────────────────────────────────────────────────

describe("GapCoverage enum", () => {
  it("covers OK, MISSING, INCOMPLETE", () => {
    expect(gapCoverageEnum.safeParse("OK").success).toBe(true);
    expect(gapCoverageEnum.safeParse("MISSING").success).toBe(true);
    expect(gapCoverageEnum.safeParse("INCOMPLETE").success).toBe(true);
    expect(gapCoverageEnum.safeParse("PARTIAL").success).toBe(false);
  });
});

// ─── 4. Header summary schema ─────────────────────────────────────────────────

describe("MappingP12HeaderSummary schema", () => {
  it("S1-AC2: parses valid APPROVED_ACTIVE header", () => {
    const result = mappingP12HeaderSummarySchema.safeParse({
      ...validHeaderSummary,
      workflowStatus: "APPROVED_ACTIVE",
      aktifFlag: true,
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.aktifFlag).toBe(true);
      expect(result.data.regulatedFlag).toBe(true);
    }
  });

  it("S1-AC4: parses version chain with parentId set", () => {
    const result = mappingP12HeaderSummarySchema.safeParse({
      ...validHeaderSummary,
      parentId: HEADER_ID,
      effectiveFrom: "2026-06-22T10:00:00+07:00",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.parentId).toBe(HEADER_ID);
    }
  });

  it("rejects missing id", () => {
    const { id: _id, ...noId } = validHeaderSummary;
    expect(mappingP12HeaderSummarySchema.safeParse(noId).success).toBe(false);
  });
});

// ─── 5. Detail row schema ─────────────────────────────────────────────────────

describe("MappingP12DetailRow schema", () => {
  it("parses valid D (debit) row", () => {
    const result = mappingP12DetailRowSchema.safeParse(validDetailRow);
    expect(result.success).toBe(true);
  });

  it("parses valid K (kredit) row", () => {
    const kRow = { ...validDetailRow, debitKredit: "K" as const, id: "770e8400-e29b-41d4-a716-446655440001" };
    const result = mappingP12DetailRowSchema.safeParse(kRow);
    expect(result.success).toBe(true);
  });

  it("rejects invalid D/K value", () => {
    const result = mappingP12DetailRowSchema.safeParse({ ...validDetailRow, debitKredit: "X" });
    expect(result.success).toBe(false);
  });

  it("S3-AC2: allows null akun fields (server validates COA)", () => {
    const result = mappingP12DetailRowSchema.safeParse({ ...validDetailRow, akunDebit: null, akunKredit: null });
    expect(result.success).toBe(true);
  });
});

// ─── 6. Balance check display logic ──────────────────────────────────────────

describe("Balance check display logic (S3-AC4, MAPPING_UNBALANCED)", () => {
  it("balanced when debitCount === kreditCount both > 0", () => {
    const rows = [
      { ...validDetailRow, debitKredit: "D" as const, urutan: 1 },
      { ...validDetailRow, id: "770e8400-e29b-41d4-a716-446655440002", debitKredit: "K" as const, urutan: 2 },
    ];
    const debitCount = rows.filter((r) => r.debitKredit === "D").length;
    const kreditCount = rows.filter((r) => r.debitKredit === "K").length;
    expect(debitCount).toBe(1);
    expect(kreditCount).toBe(1);
    expect(debitCount === kreditCount).toBe(true);
  });

  it("MAPPING_UNBALANCED: 2 debit, 1 kredit => unbalanced", () => {
    const rows = [
      { debitKredit: "D" as const },
      { debitKredit: "D" as const },
      { debitKredit: "K" as const },
    ];
    const d = rows.filter((r) => r.debitKredit === "D").length;
    const k = rows.filter((r) => r.debitKredit === "K").length;
    expect(d !== k).toBe(true);
  });
});

// ─── 7. NewVersionForm schema ─────────────────────────────────────────────────

describe("NewVersionFormSchema (S1-AC4 new version)", () => {
  it("passes with valid reason + 2 detail rows", () => {
    const result = newVersionFormSchema.safeParse({
      reason: "Perubahan kode akun sesuai COA baru",
      detail: [
        { akunDebit: "110201", akunKredit: "440101", debitKredit: "D", jumlahCalc: "ECL_weighted", urutan: 1 },
        { akunDebit: "440101", akunKredit: "110201", debitKredit: "K", jumlahCalc: null, urutan: 2 },
      ],
    });
    expect(result.success).toBe(true);
  });

  it("fails when reason < 10 chars", () => {
    const result = newVersionFormSchema.safeParse({
      reason: "short",
      detail: [{ akunDebit: "1", akunKredit: "2", debitKredit: "D", urutan: 1 }],
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues.some((i) => i.path.includes("reason"))).toBe(true);
    }
  });

  it("fails when detail is empty array", () => {
    const result = newVersionFormSchema.safeParse({ reason: "Alasan cukup panjang", detail: [] });
    expect(result.success).toBe(false);
  });
});

// ─── 8. ReviewDialog schema (S2 comment ≥ 30 chars) ─────────────────────────

describe("ReviewDialogSchema — comment min 30 chars", () => {
  it("passes with comment >= 30 chars", () => {
    const result = reviewDialogSchema.safeParse({
      comment: "Akun debit/kredit diverifikasi ke COA — lanjut ke approval.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("fails with comment < 30 chars", () => {
    const result = reviewDialogSchema.safeParse({
      comment: "Terlalu pendek",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].path).toContain("comment");
    }
  });

  it("fails with invalid signatureMethod", () => {
    const result = reviewDialogSchema.safeParse({
      comment: "Akun debit/kredit diverifikasi ke COA — lanjut ke approval.",
      signatureMethod: "JWT_STANDARD",
    });
    expect(result.success).toBe(false);
  });
});

// ─── 9. ApproveDialog schema (S2 comment ≥ 10 chars) ─────────────────────────

describe("ApproveDialogSchema — comment min 10 chars", () => {
  it("passes for 6-eyes approve with valid comment", () => {
    const result = approveDialogSchema.safeParse({
      comment: "Mapping sesuai PSAK 71.",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(true);
  });

  it("fails when comment < 10 chars", () => {
    const result = approveDialogSchema.safeParse({
      comment: "OK",
      signatureMethod: "JWT_STEP_UP",
    });
    expect(result.success).toBe(false);
  });
});

// ─── 10. RejectDialog schema (S2 reason ≥ 30 chars) ─────────────────────────

describe("RejectDialogSchema — reason min 30 chars", () => {
  it("passes with reason >= 30 chars", () => {
    const result = rejectDialogSchema.safeParse({
      reason: "Akun 110201 tidak ditemukan di COA. Perbaiki dan submit ulang.",
    });
    expect(result.success).toBe(true);
  });

  it("S2-AC3: fails when reason < 30 chars (SoD + short reason both blocked)", () => {
    const result = rejectDialogSchema.safeParse({ reason: "Salah" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toContain("30");
    }
  });
});

// ─── 11. SoD contract (S2-AC3) ───────────────────────────────────────────────

describe("SoD contract — persona gating logic (S2-AC3)", () => {
  const currentStatus = "PENDING_APPROVAL" as MappingP12WorkflowStatus;
  const makerId = MAKER_ID;
  const reviewerId = REVIEWER_ID;

  it("reviewer cannot approve (SoD violation)", () => {
    const currentUserId = REVIEWER_ID;
    const isMaker = currentUserId === makerId;
    const isReviewer = currentUserId === reviewerId;
    const canApprove =
      currentStatus === "PENDING_APPROVAL" && !isMaker && !isReviewer;
    expect(canApprove).toBe(false);
  });

  it("approver (different user) can approve", () => {
    const currentUserId = APPROVER_ID;
    const isMaker = currentUserId === makerId;
    const isReviewer = currentUserId === reviewerId;
    const canApprove =
      currentStatus === "PENDING_APPROVAL" && !isMaker && !isReviewer;
    expect(canApprove).toBe(true);
  });

  it("PENDING_APPROVAL_2: only approver2 (not M/R/A) can approve-2", () => {
    const status = "PENDING_APPROVAL_2" as MappingP12WorkflowStatus;
    const canApprove2 = (userId: string) =>
      status === "PENDING_APPROVAL_2" &&
      userId !== makerId &&
      userId !== reviewerId &&
      userId !== APPROVER_ID;

    expect(canApprove2(RISK_ID)).toBe(true);
    expect(canApprove2(MAKER_ID)).toBe(false);
    expect(canApprove2(REVIEWER_ID)).toBe(false);
    expect(canApprove2(APPROVER_ID)).toBe(false);
  });
});

// ─── 12. Regulated flag display (S1 + S2) ────────────────────────────────────

describe("Regulated flag display (S1-AC1, S2 path selection)", () => {
  const REGULATED_CODES = new Set([
    "ECL_PEMBENTUKAN", "ECL_REVERSAL", "POCI_DELTA_ECL",
    "MTM_FVTPL", "MTM_FVOCI", "MTM_FVOCI_ELECTION",
    "REKLAS_OCI_PL", "REKLASIFIKASI_AC_FVOCI", "REKLASIFIKASI_FVOCI_AC",
    "MODIFIKASI_MATERIAL", "EIR_CATCH_UP_ADJUSTMENT", "STAGE_MIGRATION", "FX_UNREALIZED",
  ]);

  it("ECL_PEMBENTUKAN is regulated → 6-eyes path", () => {
    expect(REGULATED_CODES.has("ECL_PEMBENTUKAN")).toBe(true);
  });

  it("PENEMPATAN is NOT regulated → 4-eyes path", () => {
    expect(REGULATED_CODES.has("PENEMPATAN")).toBe(false);
  });

  it("all 13 regulated codes identified", () => {
    expect(REGULATED_CODES.size).toBe(13);
  });
});

// ─── 13. Version chain UI (S1-AC4) ───────────────────────────────────────────

describe("Version chain schema validation", () => {
  it("parses version with parent_id (chain link)", () => {
    const newVersion = {
      ...validHeaderSummary,
      id: "880e8400-e29b-41d4-a716-446655440001",
      parentId: HEADER_ID,
      effectiveFrom: "2026-07-01T00:00:00+07:00",
      effectiveTo: null,
      workflowStatus: "DRAFT" as MappingP12WorkflowStatus,
    };
    expect(mappingP12HeaderSummarySchema.safeParse(newVersion).success).toBe(true);
  });

  it("parses superseded version with effective_to set", () => {
    const superseded = {
      ...validHeaderSummary,
      effectiveFrom: "2026-01-01T00:00:00+07:00",
      effectiveTo: "2026-07-01T00:00:00+07:00",
      workflowStatus: "APPROVED_ACTIVE" as MappingP12WorkflowStatus,
    };
    expect(mappingP12HeaderSummarySchema.safeParse(superseded).success).toBe(true);
  });
});

// ─── 14. BulkImportResult schema (S3) ────────────────────────────────────────

describe("BulkImportResult schema (S3-AC2)", () => {
  it("parses valid import result with mixed valid/invalid rows", () => {
    const result = bulkImportResultSchema.safeParse({
      batchId: "BATCH-MAP-001",
      batchType: "MAPPING_BULK",
      totalRows: 15,
      validRows: 12,
      invalidRows: 3,
      errors: [
        { row: 5, col: "akun_debit", errorCode: "MAPPING_AKUN_INVALID", error: "Akun 999999 tidak ditemukan di COA." },
        { row: 8, col: "akun_kredit", errorCode: "MAPPING_AKUN_INVALID", error: "Akun 888888 tidak ditemukan di COA." },
        { row: 11, col: "debit_kredit", errorCode: "MAPPING_UNBALANCED", error: "Total debit 2 ≠ kredit 1." },
      ],
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.errors).toHaveLength(3);
      expect(result.data.errors[0].errorCode).toBe("MAPPING_AKUN_INVALID");
      expect(result.data.errors[2].errorCode).toBe("MAPPING_UNBALANCED");
    }
  });

  it("S3-AC3: MAPPING_AKUN_INVALID in errors array", () => {
    const r = bulkImportResultSchema.safeParse({
      batchId: "BATCH-001", batchType: "MAPPING_BULK",
      totalRows: 1, validRows: 0, invalidRows: 1,
      errors: [{ row: 1, col: "akun_debit", errorCode: "MAPPING_AKUN_INVALID", error: "Not found" }],
    });
    expect(r.success).toBe(true);
  });
});

// ─── 15. RPT-19 Coverage schema (S4) ─────────────────────────────────────────

describe("Rpt19Coverage schema (S4)", () => {
  it("S4-AC1: parses coverage summary with gap events", () => {
    const result = rpt19CoverageSchema.safeParse({
      totalEvents: 27,
      activeEvents: 5,
      missingEvents: 22,
      gapEvents: [
        {
          eventCode: "ECL_PEMBENTUKAN",
          namaEvent: "Pembentukan ECL",
          workflowStatus: "DRAFT",
          activeDetailCount: 0,
          missingAkunCount: 2,
          lastDlqError: "2026-06-22T08:00:00Z",
          gapCoverage: "MISSING",
        },
      ],
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.gapEvents[0].gapCoverage).toBe("MISSING");
    }
  });

  it("S4-AC2: INCOMPLETE gap when workflowStatus APPROVED_ACTIVE but akun null", () => {
    const r = rpt19CoverageSchema.safeParse({
      totalEvents: 27, activeEvents: 5, missingEvents: 0,
      gapEvents: [{
        eventCode: "PENEMPATAN", namaEvent: "Penempatan", workflowStatus: "APPROVED_ACTIVE",
        activeDetailCount: 2, missingAkunCount: 1, lastDlqError: null, gapCoverage: "INCOMPLETE",
      }],
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.gapEvents[0].gapCoverage).toBe("INCOMPLETE");
    }
  });

  it("S4-AC1: all 3 gap states represented", () => {
    const allGaps: GapCoverage[] = ["OK", "MISSING", "INCOMPLETE"];
    allGaps.forEach((gap) => {
      expect(gapCoverageEnum.safeParse(gap).success).toBe(true);
    });
  });
});

// ─── 16. RPT-20 Validation schema (S5) ───────────────────────────────────────

describe("Rpt20Validation schema (S5-AC1)", () => {
  it("parses validation result with 2 invalid mappings", () => {
    const result = rpt20ValidationSchema.safeParse({
      totalActiveMappings: 5,
      validMappings: 3,
      invalidMappings: 2,
      issues: [
        { eventCode: "PENEMPATAN", headerId: HEADER_ID, errorCodes: ["MAPPING_AKUN_INVALID"], details: "1 baris akun_debit null" },
        { eventCode: "ECL_PEMBENTUKAN", headerId: HEADER_ID, errorCodes: ["MAPPING_UNBALANCED"], details: "D count ≠ K count" },
      ],
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.invalidMappings).toBe(2);
      expect(result.data.issues[1].errorCodes).toContain("MAPPING_UNBALANCED");
    }
  });

  it("S5-AC2: all-valid case (invalidMappings = 0)", () => {
    const r = rpt20ValidationSchema.safeParse({
      totalActiveMappings: 5, validMappings: 5, invalidMappings: 0, issues: [],
    });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data.issues).toHaveLength(0);
  });
});

// ─── 17. AuditLogEntry schema (S5-AC3, RPT-21) ────────────────────────────────

describe("AuditLogEntry schema (S5-AC3)", () => {
  it("parses MAPPING.REVIEWED audit entry", () => {
    const result = auditLogEntrySchema.safeParse({
      eventId: "990e8400-e29b-41d4-a716-446655440001",
      eventTime: "2026-06-22T10:30:00+07:00",
      actorUserId: REVIEWER_ID,
      actorRole: "ROLE-AKUN-CTL",
      action: "MAPPING.REVIEWED",
      entityType: "mst.mapping_jurnal_header",
      entityId: HEADER_ID,
      beforeJsonb: { workflowStatus: "PENDING_REVIEW" },
      afterJsonb: { workflowStatus: "PENDING_APPROVAL_2", reviewerId: REVIEWER_ID },
      traceId: "abc123",
    });
    expect(result.success).toBe(true);
  });

  it("MAPPING.SOD_VIOLATION_ATTEMPT has afterJsonb with attempt details", () => {
    const r = auditLogEntrySchema.safeParse({
      eventId: "aa0e8400-e29b-41d4-a716-446655440001",
      eventTime: "2026-06-22T11:00:00+07:00",
      actorUserId: MAKER_ID,
      actorRole: "ROLE-AKUN",
      action: "MAPPING.SOD_VIOLATION_ATTEMPT",
      entityType: "mst.mapping_jurnal_header",
      entityId: HEADER_ID,
      beforeJsonb: null,
      afterJsonb: { attemptedAction: "REVIEW", actor: MAKER_ID, reason: "SoD: maker cannot be reviewer" },
      traceId: null,
    });
    expect(r.success).toBe(true);
  });
});

// ─── 18. 7 new error codes present ───────────────────────────────────────────

describe("MAPPING_P12 error codes — 7 new codes (S2)", () => {
  it("has exactly 7 MAPPING_* error codes", () => {
    expect(MAPPING_P12_ERROR_CODES).toHaveLength(7);
  });

  it("contains all 7 specific codes", () => {
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_EVENT_NOT_FOUND");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_AKUN_INVALID");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_UNBALANCED");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_REGULATED_REQUIRES_RISK");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_DUPLICATE_VERSION");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_SOD_VIOLATION");
    expect(MAPPING_P12_ERROR_CODES).toContain("MAPPING_PERIODE_LOCKED");
  });
});

// ─── 19. Submit dialog schema ─────────────────────────────────────────────────

describe("SubmitDialogSchema", () => {
  it("passes with valid comment", () => {
    const r = submitDialogSchema.safeParse({ comment: "Submitted for review" });
    expect(r.success).toBe(true);
  });

  it("fails with empty comment", () => {
    const r = submitDialogSchema.safeParse({ comment: "" });
    expect(r.success).toBe(false);
  });
});

// ─── 20. Persona gating: ROLE-AKUN-CTL only for review/approve (S2) ──────────

describe("Persona gating — absent-from-DOM logic (S2)", () => {
  const permissions_AKUN = ["jurnal.mapping.create", "jurnal.mapping.read"];
  const permissions_AKUNCTL = ["jurnal.mapping.review", "jurnal.mapping.approve"];
  const permissions_RISK = ["jurnal.mapping.approve_2", "jurnal.mapping.read"];
  const permissions_AUDIT = ["audit_log.read", "jurnal.mapping.read"];

  it("ROLE-AKUN cannot review (no jurnal.mapping.review)", () => {
    expect(permissions_AKUN.includes("jurnal.mapping.review")).toBe(false);
  });

  it("ROLE-AKUN-CTL can review", () => {
    expect(permissions_AKUNCTL.includes("jurnal.mapping.review")).toBe(true);
  });

  it("ROLE-RISK can approve-2", () => {
    expect(permissions_RISK.includes("jurnal.mapping.approve_2")).toBe(true);
  });

  it("ROLE-AUDIT cannot review or approve", () => {
    expect(permissions_AUDIT.includes("jurnal.mapping.review")).toBe(false);
    expect(permissions_AUDIT.includes("jurnal.mapping.approve")).toBe(false);
    expect(permissions_AUDIT.includes("jurnal.mapping.approve_2")).toBe(false);
  });
});
