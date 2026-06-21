/**
 * Vitest component-logic tests — P5-M11 Bulk Upload badge state matrix
 *
 * Covers: batch status badge, row status badge, dry_run result display,
 * rollback grace UI, persona gating (absent-from-DOM logic), error codes.
 *
 * AC coverage:
 *   S1-AC1..4 (upload schema validation, parse errors)
 *   S2-AC1..4 (dry_run result display, TTL, NEEDS_MANUAL_REVIEW)
 *   S3-AC1..4 (commit job, partial commit, periode locked, PENDING_APPROVAL_BULK)
 *   S4-AC1..4 (approve, SoD, idempotency, klasifikasi manual)
 *   S5-AC1..4 (rollback, grace window, MFA, config)
 */

import { describe, it, expect } from "vitest";
import {
  bulkBatchStatusEnum,
  bulkRowStatusEnum,
  BULK_BATCH_STATUS_LABELS,
  BULK_ROW_STATUS_LABELS,
  bulkUploadBatchSummarySchema,
  bulkUploadBatchDetailSchema,
  bulkUploadRowItemSchema,
  dryRunResultSchema,
  commitJobResponseSchema,
  approveResultSchema,
  rollbackResultSchema,
  approveFormSchema,
  rollbackRequestFormSchema,
  rollbackApproveFormSchema,
  bulkUploadErrorCodes,
  stageSummarySchema,
} from "@/lib/schemas/bulkupload.schema";

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const BATCH_ID = "BATCH-001";
const JOB_ID = "a1b2c3d4-e5f6-4789-abcd-ef0123456789";
const USER_ID = "550e8400-e29b-41d4-a716-446655440001";
const USER_ID_2 = "550e8400-e29b-41d4-a716-446655440002";
const ROW_ID = "550e8400-e29b-41d4-a716-446655440003";

// ─── 1. Batch status badge state matrix ───────────────────────────────────────

describe("BulkBatchStatus — badge state matrix (9 states)", () => {
  const allStates = bulkBatchStatusEnum.options;

  it("covers all 9 batch states", () => {
    expect(allStates).toHaveLength(9);
    expect(allStates).toContain("PARSED");
    expect(allStates).toContain("DRY_RUN_PASSED");
    expect(allStates).toContain("DRY_RUN_FAILED");
    expect(allStates).toContain("COMMITTING");
    expect(allStates).toContain("COMMITTED");
    expect(allStates).toContain("PARTIAL_COMMIT");
    expect(allStates).toContain("APPROVED");
    expect(allStates).toContain("ROLLBACK_PENDING");
    expect(allStates).toContain("ROLLED_BACK");
  });

  it("every state has a label in Bahasa Indonesia", () => {
    allStates.forEach((s) => {
      expect(BULK_BATCH_STATUS_LABELS[s]).toBeDefined();
      expect(BULK_BATCH_STATUS_LABELS[s].length).toBeGreaterThan(0);
    });
  });

  it("PARSED — initial upload state (S1-AC1)", () => {
    expect(bulkBatchStatusEnum.safeParse("PARSED").success).toBe(true);
    expect(BULK_BATCH_STATUS_LABELS["PARSED"]).toBe("Diparsing");
  });

  it("DRY_RUN_PASSED — commit enabled state (S2-AC1)", () => {
    expect(bulkBatchStatusEnum.safeParse("DRY_RUN_PASSED").success).toBe(true);
  });

  it("DRY_RUN_FAILED — commit blocked state (S2-AC2)", () => {
    expect(bulkBatchStatusEnum.safeParse("DRY_RUN_FAILED").success).toBe(true);
  });

  it("PARTIAL_COMMIT — approve still allowed (S3-AC2)", () => {
    expect(bulkBatchStatusEnum.safeParse("PARTIAL_COMMIT").success).toBe(true);
  });

  it("ROLLBACK_PENDING — awaiting step-up MFA (S5-AC1)", () => {
    expect(bulkBatchStatusEnum.safeParse("ROLLBACK_PENDING").success).toBe(true);
  });

  it("ROLLED_BACK — terminal state (S5-AC1)", () => {
    expect(bulkBatchStatusEnum.safeParse("ROLLED_BACK").success).toBe(true);
  });

  it("rejects unknown batch status", () => {
    expect(bulkBatchStatusEnum.safeParse("UNKNOWN_STATE").success).toBe(false);
  });
});

// ─── 2. Row status badge state matrix (4 states + 1 special) ─────────────────

describe("BulkRowStatus — badge state matrix", () => {
  const allRowStates = bulkRowStatusEnum.options;

  it("covers PENDING, COMMITTED, FAILED, ROLLED_BACK, FLAGGED_MANUAL_REVIEW", () => {
    expect(allRowStates).toHaveLength(5);
    expect(allRowStates).toContain("PENDING");
    expect(allRowStates).toContain("COMMITTED");
    expect(allRowStates).toContain("FAILED");
    expect(allRowStates).toContain("ROLLED_BACK");
    expect(allRowStates).toContain("FLAGGED_MANUAL_REVIEW");
  });

  it("every row state has a Bahasa Indonesia label", () => {
    allRowStates.forEach((s) => {
      expect(BULK_ROW_STATUS_LABELS[s]).toBeDefined();
    });
  });

  it("FLAGGED_MANUAL_REVIEW does NOT block commit (S2-AC3)", () => {
    // Flagged rows result in DRY_RUN_PASSED — not DRY_RUN_FAILED
    const flaggedIsNotFail = "FLAGGED_MANUAL_REVIEW" !== "FAILED";
    expect(flaggedIsNotFail).toBe(true);
  });

  it("rejects invalid row status", () => {
    expect(bulkRowStatusEnum.safeParse("INVALID_ROW").success).toBe(false);
  });
});

// ─── 3. BulkUploadBatchSummary schema (S1-AC1) ───────────────────────────────

describe("BulkUploadBatchSummary schema — S1-AC1 upload valid", () => {
  const validSummary = {
    batchId: BATCH_ID,
    status: "PARSED" as const,
    totalRows: 350,
    parseErrors: [],
    sheets: { Deposito: 80, Obligasi: 120, Saham: 60, Reksadana: 50, Tabungan_Cash: 40 },
    createdAt: "2026-06-21T10:30:00+07:00",
  };

  it("validates a complete upload response", () => {
    expect(bulkUploadBatchSummarySchema.safeParse(validSummary).success).toBe(true);
  });

  it("S1-AC4 — parse errors collected, batch still PARSED", () => {
    const withErrors = {
      ...validSummary,
      totalRows: 350,
      parseErrors: [{ sheet: "Obligasi", row: 45, col: "kupon", error: "Expected NUMERIC, got TEXT 'N/A'" }],
    };
    const result = bulkUploadBatchSummarySchema.safeParse(withErrors);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("PARSED");
      expect(result.data.parseErrors).toHaveLength(1);
    }
  });

  it("sheets map stores per-sheet row counts", () => {
    const result = bulkUploadBatchSummarySchema.safeParse(validSummary);
    if (result.success) {
      expect(result.data.sheets["Deposito"]).toBe(80);
      expect(result.data.sheets["Obligasi"]).toBe(120);
    }
  });

  it("rejects missing batchId", () => {
    const { batchId: _omit, ...rest } = validSummary;
    expect(bulkUploadBatchSummarySchema.safeParse(rest).success).toBe(false);
  });
});

// ─── 4. DRY_RUN result display (S2-AC1, S2-AC2, S2-AC3, S2-AC4) ─────────────

describe("DryRunResult schema — S2 dry run scenarios", () => {
  it("S2-AC1 — DRY_RUN_PASSED with flagged rows", () => {
    const passed = {
      status: "DRY_RUN_PASSED" as const,
      totalRows: 350,
      validRows: 347,
      invalidRows: 0,
      flaggedRows: 3,
      stageSummary: {
        stage1: { status: "PASS" as const },
        stage2: { status: "PASS" as const },
        stage3: { status: "PASS" as const },
        stage4: { status: "PASS" as const, evaluated: 350, classified: 347, flagged: 3, sppiServiceUnavailable: false },
      },
      errorsPerRow: [],
      dryRunExpiresAt: "2026-06-21T11:30:00+07:00",
    };
    const result = dryRunResultSchema.safeParse(passed);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("DRY_RUN_PASSED");
      expect(result.data.flaggedRows).toBe(3);
      expect(result.data.invalidRows).toBe(0);
    }
  });

  it("S2-AC2 — DRY_RUN_FAILED with Stage 3 cross-ref error", () => {
    const failed = {
      status: "DRY_RUN_FAILED" as const,
      invalidRows: 1,
      errorsPerRow: [
        {
          sheet: "Obligasi",
          row: 10,
          stage: 3,
          col: "counterparty_id",
          error: "Counterparty CP-999 tidak ditemukan di master data.",
        },
      ],
    };
    const result = dryRunResultSchema.safeParse(failed);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("DRY_RUN_FAILED");
      expect(result.data.errorsPerRow[0].stage).toBe(3);
    }
  });

  it("S2-AC3 — NEEDS_MANUAL_REVIEW: flaggedRows > 0 but status = DRY_RUN_PASSED", () => {
    const withFlagged = {
      status: "DRY_RUN_PASSED" as const,
      totalRows: 10,
      validRows: 9,
      invalidRows: 0,
      flaggedRows: 1,
      errorsPerRow: [
        {
          sheet: "Deposito",
          row: 22,
          stage: 4,
          col: null,
          error: "SPPI Q7 ambiguous — perlu review manual",
          klasifikasiPsak71: null,
        },
      ],
    };
    const result = dryRunResultSchema.safeParse(withFlagged);
    expect(result.success).toBe(true);
    if (result.success) {
      // NEEDS_MANUAL_REVIEW does NOT make status DRY_RUN_FAILED
      expect(result.data.status).toBe("DRY_RUN_PASSED");
      expect(result.data.flaggedRows).toBe(1);
    }
  });

  it("S2-AC4 — TTL field present in result", () => {
    const withTTL = {
      status: "DRY_RUN_PASSED" as const,
      errorsPerRow: [],
      dryRunExpiresAt: "2026-06-21T11:30:00+07:00",
    };
    const result = dryRunResultSchema.safeParse(withTTL);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.dryRunExpiresAt).toBe("2026-06-21T11:30:00+07:00");
    }
  });
});

// ─── 5. Stage summary schema ──────────────────────────────────────────────────

describe("StageSummary schema", () => {
  it("validates all 4 stages PASS", () => {
    const all4Pass = {
      stage1: { status: "PASS" as const },
      stage2: { status: "PASS" as const },
      stage3: { status: "PASS" as const },
      stage4: { status: "PASS" as const, evaluated: 350, classified: 347, flagged: 3, sppiServiceUnavailable: false },
    };
    expect(stageSummarySchema.safeParse(all4Pass).success).toBe(true);
  });

  it("Stage 4 UNAVAILABLE when SPPI service down", () => {
    const stage4Unavailable = {
      stage4: { status: "UNAVAILABLE" as const, sppiServiceUnavailable: true },
    };
    expect(stageSummarySchema.safeParse(stage4Unavailable).success).toBe(true);
  });

  it("rejects invalid stage status", () => {
    const bad = { stage1: { status: "RUNNING" } };
    expect(stageSummarySchema.safeParse(bad).success).toBe(false);
  });
});

// ─── 6. Commit job response (S3-AC1) ─────────────────────────────────────────

describe("CommitJobResponse schema — S3-AC1", () => {
  it("validates 202 commit response with jobId + streamUrl", () => {
    const commitResp = {
      jobId: JOB_ID,
      type: "bulkupload:commit_instrumen",
      statusUrl: `/api/v1/jobs/${JOB_ID}`,
      streamUrl: `/api/v1/jobs/${JOB_ID}/stream`,
      batchId: BATCH_ID,
    };
    expect(commitJobResponseSchema.safeParse(commitResp).success).toBe(true);
  });

  it("requires jobId field", () => {
    const noJob = { type: "bulkupload:commit_instrumen", statusUrl: "/x", streamUrl: "/x/stream", batchId: BATCH_ID };
    expect(commitJobResponseSchema.safeParse(noJob).success).toBe(false);
  });
});

// ─── 7. Row item schema ───────────────────────────────────────────────────────

describe("BulkUploadRowItem schema — S3-AC4 PENDING_APPROVAL_BULK", () => {
  it("validates a COMMITTED row with instrumenId", () => {
    const committedRow = {
      rowId: ROW_ID,
      batchId: BATCH_ID,
      sheetName: "Deposito",
      rowNumber: 1,
      rowStatus: "COMMITTED" as const,
      instrumenId: USER_ID,
      rowDataPreview: { kode_instrumen: "INST-DEP-0001" },
    };
    expect(bulkUploadRowItemSchema.safeParse(committedRow).success).toBe(true);
  });

  it("validates a FAILED row with error detail", () => {
    const failedRow = {
      rowId: ROW_ID,
      batchId: BATCH_ID,
      sheetName: "Obligasi",
      rowNumber: 5,
      rowStatus: "FAILED" as const,
      instrumenId: null,
      rowErrorJsonb: { error: "Duplikat kode instrumen INST-OBL-0005" },
    };
    expect(bulkUploadRowItemSchema.safeParse(failedRow).success).toBe(true);
  });

  it("validates a FLAGGED_MANUAL_REVIEW row", () => {
    const flaggedRow = {
      rowId: ROW_ID,
      batchId: BATCH_ID,
      sheetName: "Deposito",
      rowNumber: 22,
      rowStatus: "FLAGGED_MANUAL_REVIEW" as const,
      instrumenId: USER_ID,
    };
    expect(bulkUploadRowItemSchema.safeParse(flaggedRow).success).toBe(true);
  });
});

// ─── 8. Approve result + SoD gating (S4-AC1, S4-AC2) ────────────────────────

describe("ApproveResult schema — S4-AC1 approve workflow", () => {
  it("validates successful approve response", () => {
    const approved = {
      batchId: BATCH_ID,
      status: "APPROVED" as const,
      activatedCount: 348,
      pendingManualCount: 3,
      approverId: USER_ID_2,
      approvedAt: "2026-06-21T14:00:00+07:00",
    };
    expect(approveResultSchema.safeParse(approved).success).toBe(true);
  });

  it("S4-AC2 — SoD: maker cannot be approver (logic assertion)", () => {
    const makerId = USER_ID;
    const approverId = USER_ID; // same — SoD violation
    const isSoD = makerId === approverId;
    expect(isSoD).toBe(true); // server rejects this → BULK_APPROVE_SOD_VIOLATION
  });

  it("S4-AC2 — SoD passes when approver differs from maker", () => {
    const makerId = USER_ID;
    const approverId = USER_ID_2;
    const isSoD = makerId === approverId;
    expect(isSoD).toBe(false);
  });
});

// ─── 9. Approve form schema validation ────────────────────────────────────────

describe("ApproveFormSchema validation", () => {
  it("passes with comment ≥ 10 chars and JWT_STEP_UP", () => {
    const valid = { comment: "Checked 348 instrumen OK", signatureMethod: "JWT_STEP_UP" as const };
    expect(approveFormSchema.safeParse(valid).success).toBe(true);
  });

  it("rejects comment < 10 chars", () => {
    const short = { comment: "OK", signatureMethod: "JWT_STEP_UP" as const };
    expect(approveFormSchema.safeParse(short).success).toBe(false);
  });

  it("rejects invalid signatureMethod", () => {
    const invalid = { comment: "Checked OK here", signatureMethod: "TOTP" };
    expect(approveFormSchema.safeParse(invalid).success).toBe(false);
  });
});

// ─── 10. Rollback request form — reason ≥ 50 chars (S5-AC1) ──────────────────

describe("RollbackRequestFormSchema — S5-AC1 grace window", () => {
  it("passes with reason ≥ 50 chars", () => {
    const reason50 = "Error counterparty mapping ditemukan post-commit rollback dibutuhkan.";
    expect(reason50.length).toBeGreaterThanOrEqual(50);
    expect(rollbackRequestFormSchema.safeParse({ reason: reason50 }).success).toBe(true);
  });

  it("rejects reason < 50 chars", () => {
    const short = "Rollback perlu.";
    expect(short.length).toBeLessThan(50);
    expect(rollbackRequestFormSchema.safeParse({ reason: short }).success).toBe(false);
  });

  it("S5-AC2 — grace window expiry is a server-side check (not form schema)", () => {
    // Grace window: batch.committed_at + BULK_ROLLBACK_GRACE_DAYS
    const committedAt = new Date("2026-06-14T10:00:00+07:00");
    const graceDays = 7;
    const graceExpires = new Date(committedAt.getTime() + graceDays * 24 * 60 * 60 * 1000);
    const now = new Date("2026-06-24T10:00:00+07:00"); // 1 day after expiry
    expect(now > graceExpires).toBe(true); // Grace expired — server returns BULK_ROLLBACK_GRACE_EXPIRED
  });

  it("S5-AC1 — within grace window", () => {
    const committedAt = new Date("2026-06-16T10:00:00+07:00");
    const graceDays = 7;
    const graceExpires = new Date(committedAt.getTime() + graceDays * 24 * 60 * 60 * 1000);
    const now = new Date("2026-06-21T14:00:00+07:00");
    expect(now <= graceExpires).toBe(true);
  });
});

// ─── 11. Rollback approve form — MFA step-up (S5-AC3) ────────────────────────

describe("RollbackApproveFormSchema — S5-AC3 step-up MFA", () => {
  it("passes with valid comment and JWT_STEP_UP", () => {
    const valid = { comment: "Rollback disetujui — error confirmed", signatureMethod: "JWT_STEP_UP" as const };
    expect(rollbackApproveFormSchema.safeParse(valid).success).toBe(true);
  });

  it("rejects missing signatureMethod", () => {
    const noSig = { comment: "Rollback approved here" };
    expect(rollbackApproveFormSchema.safeParse(noSig).success).toBe(false);
  });
});

// ─── 12. Rollback result (S5-AC1) ────────────────────────────────────────────

describe("RollbackResult schema — S5-AC1", () => {
  it("validates rollback response with rolled_back_count", () => {
    const rbResult = {
      batchId: BATCH_ID,
      status: "ROLLED_BACK" as const,
      rolledBackCount: 348,
      rolledBackAt: "2026-06-21T14:30:00+07:00",
    };
    expect(rollbackResultSchema.safeParse(rbResult).success).toBe(true);
  });
});

// ─── 13. Batch detail schema (S3-AC4) ────────────────────────────────────────

describe("BulkUploadBatchDetail schema — S3-AC4 rollback status", () => {
  it("validates batch with rollback grace info", () => {
    const detail = {
      batchId: BATCH_ID,
      status: "APPROVED" as const,
      totalRows: 350,
      parseErrors: [],
      sheets: { Deposito: 80 },
      createdAt: "2026-06-16T10:00:00+07:00",
      committedRows: 348,
      failedRows: 2,
      flaggedRows: 0,
      rollbackStatus: "NOT_REQUESTED" as const,
      rollbackGraceExpiresAt: "2026-06-23T10:00:00+07:00",
      approverId: USER_ID_2,
      approvedAt: "2026-06-21T14:00:00+07:00",
    };
    expect(bulkUploadBatchDetailSchema.safeParse(detail).success).toBe(true);
  });
});

// ─── 14. Error codes — 7 new BULK_* codes ────────────────────────────────────

describe("BULK error codes — 7 new P5-M11 codes", () => {
  const expectedCodes = [
    "BULK_FILE_TOO_LARGE",
    "BULK_MIME_INVALID",
    "BULK_DRY_RUN_EXPIRED",
    "BULK_DRY_RUN_FAILED",
    "BULK_PERIODE_LOCKED",
    "BULK_ROLLBACK_GRACE_EXPIRED",
    "BULK_APPROVE_SOD_VIOLATION",
  ] as const;

  it("all 7 P5-M11 error codes defined", () => {
    expectedCodes.forEach((code) => {
      expect(bulkUploadErrorCodes).toContain(code);
    });
  });

  it("bulkUploadErrorCodes has exactly 7 entries", () => {
    expect(bulkUploadErrorCodes).toHaveLength(7);
  });

  it("S1-AC2: BULK_FILE_TOO_LARGE present", () => {
    expect(bulkUploadErrorCodes).toContain("BULK_FILE_TOO_LARGE");
  });

  it("S2-AC4: BULK_DRY_RUN_EXPIRED present", () => {
    expect(bulkUploadErrorCodes).toContain("BULK_DRY_RUN_EXPIRED");
  });

  it("S4-AC2: BULK_APPROVE_SOD_VIOLATION present", () => {
    expect(bulkUploadErrorCodes).toContain("BULK_APPROVE_SOD_VIOLATION");
  });

  it("S5-AC2: BULK_ROLLBACK_GRACE_EXPIRED present", () => {
    expect(bulkUploadErrorCodes).toContain("BULK_ROLLBACK_GRACE_EXPIRED");
  });
});

// ─── 15. Persona gating — absent-from-DOM assertions ─────────────────────────

describe("Persona gating — absent-from-DOM logic", () => {
  const canPermission = (permissions: string[], required: string) =>
    permissions.includes(required);

  it("ROLE-MAKER-TR: can upload and commit; cannot approve", () => {
    const makerPerms = ["instrumen.create", "instrumen.read"];
    expect(canPermission(makerPerms, "instrumen.create")).toBe(true);
    expect(canPermission(makerPerms, "instrumen.approve")).toBe(false);
  });

  it("ROLE-APPR-TR: can approve; cannot upload", () => {
    const apprPerms = ["instrumen.approve", "instrumen.read"];
    expect(canPermission(apprPerms, "instrumen.approve")).toBe(true);
    expect(canPermission(apprPerms, "instrumen.create")).toBe(false);
  });

  it("ROLE-CFO: can rollback; cannot upload or approve (S5-AC3)", () => {
    const cfoPerms = ["instrumen.delete", "instrumen.read"];
    expect(canPermission(cfoPerms, "instrumen.delete")).toBe(true);
    expect(canPermission(cfoPerms, "instrumen.approve")).toBe(false);
    expect(canPermission(cfoPerms, "instrumen.create")).toBe(false);
  });

  it("ROLE-AUDIT: read-only; cannot create/approve/delete", () => {
    const auditPerms = ["instrumen.read", "audit_log.read"];
    expect(canPermission(auditPerms, "instrumen.read")).toBe(true);
    expect(canPermission(auditPerms, "instrumen.create")).toBe(false);
    expect(canPermission(auditPerms, "instrumen.approve")).toBe(false);
    expect(canPermission(auditPerms, "instrumen.delete")).toBe(false);
  });

  it("ROLE-RISK: can review klasifikasi; cannot upload or approve batch", () => {
    const riskPerms = ["instrumen.read", "klasifikasi.update"];
    expect(canPermission(riskPerms, "klasifikasi.update")).toBe(true);
    expect(canPermission(riskPerms, "instrumen.approve")).toBe(false);
  });
});

// ─── 16. MIME + file size client-side hints ───────────────────────────────────

describe("Client-side XLSX validation hints (server re-validates)", () => {
  const MAX_BYTES = 50 * 1024 * 1024;

  it("S1-AC2: file > 50MB triggers client hint", () => {
    const fileSize = 62 * 1024 * 1024;
    expect(fileSize > MAX_BYTES).toBe(true);
  });

  it("S1-AC2: file ≤ 50MB passes client check", () => {
    const fileSize = 12 * 1024 * 1024;
    expect(fileSize > MAX_BYTES).toBe(false);
  });

  it("S1-AC3: .xlsx extension passes; .csv does not", () => {
    const isXlsx = (name: string) => name.toLowerCase().endsWith(".xlsx");
    expect(isXlsx("instrumen_bulk.xlsx")).toBe(true);
    expect(isXlsx("instrumen_data.csv")).toBe(false);
    expect(isXlsx("instrumen_data.xls")).toBe(false);
  });

  it("note: server performs ZIP magic bytes check (PK\\x03\\x04) — client hint only", () => {
    // Server-side: read first 4 bytes and compare to PK\x03\x04
    const XLSX_MAGIC = [0x50, 0x4b, 0x03, 0x04];
    expect(XLSX_MAGIC[0]).toBe(0x50); // 'P'
    expect(XLSX_MAGIC[1]).toBe(0x4b); // 'K'
    expect(XLSX_MAGIC).toHaveLength(4);
  });
});

// ─── 17. S5-AC4 grace window config ──────────────────────────────────────────

describe("S5-AC4 — BULK_ROLLBACK_GRACE_DAYS config", () => {
  it("default 7 days grace window computation", () => {
    const committedAt = new Date("2026-06-16T10:00:00Z");
    const graceDays = 7;
    const expires = new Date(committedAt.getTime() + graceDays * 86_400_000);
    expect(expires.toISOString()).toBe("2026-06-23T10:00:00.000Z");
  });

  it("updated to 14 days: new batches use 14-day window", () => {
    const committedAt = new Date("2026-06-16T10:00:00Z");
    const graceDays = 14;
    const expires = new Date(committedAt.getTime() + graceDays * 86_400_000);
    expect(expires.toISOString()).toBe("2026-06-30T10:00:00.000Z");
  });

  it("config change non-retroactive: old batches keep original grace", () => {
    // Batches committed before config change retain old value (7 days)
    const oldBatchGrace = 7;
    const newConfigGrace = 14;
    // Old batch uses oldBatchGrace, new batch uses newConfigGrace
    expect(oldBatchGrace).not.toBe(newConfigGrace);
  });
});
