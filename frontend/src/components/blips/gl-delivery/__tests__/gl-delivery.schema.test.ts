/**
 * Vitest unit tests — P5-M3 GL Delivery schemas
 * Coverage: Zod validation (happy path + fail path), label map completeness
 */

import { describe, it, expect } from "vitest";
import {
  retryGlDeliveryRequestSchema,
  dlqActionRequestSchema,
  runReconciliationRequestSchema,
  glDeliveryStatusSchema,
  glDeliveryDlqListItemSchema,
  glDeliveryDlqDetailSchema,
  glReconciliationSummaryItemSchema,
  GL_HOST_STATUS_LABELS,
  RECON_STATUS_LABELS,
  MISMATCH_TYPE_LABELS,
  FAILURE_CATEGORY_LABELS,
  type GlHostStatus,
  type ReconReportStatus,
  type MismatchType,
  type GlFailureCategory,
} from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// retryGlDeliveryRequestSchema
// ---------------------------------------------------------------------------

describe("retryGlDeliveryRequestSchema", () => {
  it("accepts reason of exactly 30 chars", () => {
    const result = retryGlDeliveryRequestSchema.safeParse({
      reason: "a".repeat(30),
    });
    expect(result.success).toBe(true);
  });

  it("accepts reason within bounds", () => {
    const result = retryGlDeliveryRequestSchema.safeParse({
      reason: "Retry diperlukan karena GL Host mengalami downtime sementara.",
    });
    expect(result.success).toBe(true);
  });

  it("rejects reason shorter than 30 chars", () => {
    const result = retryGlDeliveryRequestSchema.safeParse({ reason: "Terlalu pendek" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toMatch(/30/);
    }
  });

  it("rejects empty reason", () => {
    const result = retryGlDeliveryRequestSchema.safeParse({ reason: "" });
    expect(result.success).toBe(false);
  });

  it("rejects reason > 1000 chars", () => {
    const result = retryGlDeliveryRequestSchema.safeParse({
      reason: "a".repeat(1001),
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// dlqActionRequestSchema (replay + discard)
// ---------------------------------------------------------------------------

describe("dlqActionRequestSchema", () => {
  it("accepts valid reason", () => {
    const result = dlqActionRequestSchema.safeParse({
      reason: "Discard karena GL Host menolak secara permanen (HTTP 422 UNPROCESSABLE).",
    });
    expect(result.success).toBe(true);
  });

  it("rejects reason < 30 chars", () => {
    const result = dlqActionRequestSchema.safeParse({ reason: "Terlalu singkat." });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toMatch(/30/);
    }
  });
});

// ---------------------------------------------------------------------------
// runReconciliationRequestSchema
// ---------------------------------------------------------------------------

describe("runReconciliationRequestSchema", () => {
  it("accepts valid date string", () => {
    const result = runReconciliationRequestSchema.safeParse({ date: "2026-06-17" });
    expect(result.success).toBe(true);
  });

  it("accepts date with optional reason", () => {
    const result = runReconciliationRequestSchema.safeParse({
      date: "2026-06-17",
      reason: "Manual trigger untuk audit bulan Juni.",
    });
    expect(result.success).toBe(true);
  });

  it("rejects invalid date format", () => {
    const result = runReconciliationRequestSchema.safeParse({ date: "17-06-2026" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toMatch(/YYYY-MM-DD/);
    }
  });

  it("rejects non-date string", () => {
    const result = runReconciliationRequestSchema.safeParse({ date: "today" });
    expect(result.success).toBe(false);
  });

  it("rejects missing date", () => {
    const result = runReconciliationRequestSchema.safeParse({});
    expect(result.success).toBe(false);
  });

  it("uses field name 'date' (not reconDate)", () => {
    // Field must be 'date' per OpenAPI spec
    const parsed = runReconciliationRequestSchema.safeParse({ date: "2026-01-15" });
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      expect(Object.keys(parsed.data)).toContain("date");
      expect(Object.keys(parsed.data)).not.toContain("reconDate");
    }
  });
});

// ---------------------------------------------------------------------------
// glDeliveryStatusSchema (S2)
// ---------------------------------------------------------------------------

describe("glDeliveryStatusSchema", () => {
  // Valid UUID v4 values for tests
  const UUID1 = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";

  const baseStatus = {
    glStatusId: UUID1,
    glHostStatus: "PENDING_DELIVERY",
    retryCount: 0,
    deliveryMode: "API",
    canRetry: false,
  };

  it("parses minimal valid status", () => {
    const result = glDeliveryStatusSchema.safeParse(baseStatus);
    expect(result.success).toBe(true);
  });

  it("parses DELIVERED status with GL journal ID", () => {
    const result = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "DELIVERED",
      glHostJournalId: "GL-JRN-2026-001234",
      deliveredAt: "2026-06-17T10:30:00+07:00",
      canRetry: false,
    });
    expect(result.success).toBe(true);
  });

  it("parses FAILED status with error info", () => {
    const result = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "FAILED",
      failureCategory: "DOMAIN",
      lastError: "GL Host rejected: duplicate journal",
      retryCount: 3,
      canRetry: true,
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.canRetry).toBe(true);
      expect(result.data.failureCategory).toBe("DOMAIN");
    }
  });

  it("rejects invalid glHostStatus", () => {
    const result = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "REPLAYING", // old schema value — does NOT exist in OpenAPI
    });
    expect(result.success).toBe(false);
  });

  it("rejects negative retryCount", () => {
    const result = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      retryCount: -1,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// glDeliveryDlqListItemSchema (S5)
// ---------------------------------------------------------------------------

describe("glDeliveryDlqListItemSchema", () => {
  const baseDlqItem = {
    dlqEntryId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10",
    jurnalHeaderId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    noJurnal: "JRN-2026-001",
    glHostStatus: "FAILED",
    failureCategory: "INFRA",
    errorCode: "GL_DELIVERY_HOST_UNREACHABLE",
    retryCount: 2,
    createdAt: "2026-06-17T09:00:00+07:00",
    canReplay: true,
    canDiscard: false,
  };

  it("parses valid DLQ list item", () => {
    const result = glDeliveryDlqListItemSchema.safeParse(baseDlqItem);
    expect(result.success).toBe(true);
  });

  it("uses glHostStatus field (not dlqStatus)", () => {
    const parsed = glDeliveryDlqListItemSchema.safeParse(baseDlqItem);
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      expect(parsed.data).toHaveProperty("glHostStatus");
      expect(parsed.data).not.toHaveProperty("dlqStatus");
    }
  });

  it("uses dlqEntryId field (not id)", () => {
    const parsed = glDeliveryDlqListItemSchema.safeParse(baseDlqItem);
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      expect(parsed.data).toHaveProperty("dlqEntryId");
    }
  });

  it("uses noJurnal field (not jurnalNumber)", () => {
    const parsed = glDeliveryDlqListItemSchema.safeParse(baseDlqItem);
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      expect(parsed.data).toHaveProperty("noJurnal");
      expect(parsed.data).not.toHaveProperty("jurnalNumber");
    }
  });

  it("rejects REPLAYING as glHostStatus (removed in P5-M3)", () => {
    const result = glDeliveryDlqListItemSchema.safeParse({
      ...baseDlqItem,
      glHostStatus: "REPLAYING",
    });
    expect(result.success).toBe(false);
  });

  it("accepts DEAD_LETTER status", () => {
    const result = glDeliveryDlqListItemSchema.safeParse({
      ...baseDlqItem,
      glHostStatus: "DEAD_LETTER",
      canReplay: false,
      canDiscard: false,
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// glDeliveryDlqDetailSchema (S5 detail)
// ---------------------------------------------------------------------------

describe("glDeliveryDlqDetailSchema", () => {
  const baseDlqDetail = {
    dlqEntryId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a20",
    jurnalHeaderId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a21",
    noJurnal: "JRN-2026-002",
    glHostStatus: "FAILED",
    failureCategory: "DOMAIN",
    errorCode: "GL_DELIVERY_HOST_4XX",
    errorMessage: "HTTP 422: account code not found",
    retryCount: 1,
    createdAt: "2026-06-17T10:00:00+07:00",
    canReplay: true,
    canDiscard: false,
  };

  it("parses detail without optional fields", () => {
    const result = glDeliveryDlqDetailSchema.safeParse(baseDlqDetail);
    expect(result.success).toBe(true);
  });

  it("parses detail with payloadSnapshotJsonb", () => {
    const result = glDeliveryDlqDetailSchema.safeParse({
      ...baseDlqDetail,
      payloadSnapshotJsonb: { journalDate: "2026-06-17", entries: [] },
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toHaveProperty("payloadSnapshotJsonb");
      expect(result.data).not.toHaveProperty("glRequestPayloadJsonb"); // old name
    }
  });

  it("parses detail with discardInfo", () => {
    const result = glDeliveryDlqDetailSchema.safeParse({
      ...baseDlqDetail,
      glHostStatus: "DEAD_LETTER",
      discardInfo: {
        discardedBy: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99",
        discardedAt: "2026-06-17T11:00:00+07:00",
        discardReason: "GL Host telah dikonfirmasi menolak secara permanen karena kode akun tidak valid.",
      },
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.discardInfo?.discardReason).toBeDefined();
      // Field name is discardReason (not discardedReason)
      expect(result.data.discardInfo).toHaveProperty("discardReason");
    }
  });
});

// ---------------------------------------------------------------------------
// glReconciliationSummaryItemSchema (S4 history list)
// ---------------------------------------------------------------------------

describe("glReconciliationSummaryItemSchema", () => {
  it("parses summary item with minimal fields", () => {
    const result = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a30",
      tanggalRekonsiliasi: "2026-06-17",
      status: "COMPLETED",
      totalMismatchCount: 0,
    });
    expect(result.success).toBe(true);
  });

  it("parses COMPLETED_WITH_MISMATCH summary", () => {
    const result = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a31",
      tanggalRekonsiliasi: "2026-06-16",
      status: "COMPLETED_WITH_MISMATCH",
      totalMismatchCount: 3,
      deltaIdr: -500000,
      generatedAt: "2026-06-16T23:59:00+07:00",
    });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.status).toBe("COMPLETED_WITH_MISMATCH");
    }
  });

  it("uses tanggalRekonsiliasi field (not reconDate)", () => {
    const parsed = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a32",
      tanggalRekonsiliasi: "2026-06-15",
      status: "COMPLETED",
      totalMismatchCount: 0,
    });
    expect(parsed.success).toBe(true);
    if (parsed.success) {
      expect(parsed.data).toHaveProperty("tanggalRekonsiliasi");
      expect(parsed.data).not.toHaveProperty("reconDate");
    }
  });

  it("rejects MATCH as status (old enum — not in OpenAPI)", () => {
    const result = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a33",
      tanggalRekonsiliasi: "2026-06-14",
      status: "MATCH", // does not exist
      totalMismatchCount: 0,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Label map completeness — every enum value must have a label
// ---------------------------------------------------------------------------

describe("label maps completeness", () => {
  const GL_HOST_STATUSES: GlHostStatus[] = [
    "PENDING_DELIVERY",
    "DELIVERY_IN_FLIGHT",
    "DELIVERED",
    "RETRYING",
    "FAILED",
    "DEAD_LETTER",
  ];

  const RECON_STATUSES: ReconReportStatus[] = [
    "PENDING",
    "RUNNING",
    "COMPLETED",
    "COMPLETED_WITH_MISMATCH",
    "FAILED",
  ];

  const MISMATCH_TYPES: MismatchType[] = ["BLIPS_ONLY", "GL_ONLY", "AMOUNT_DIFF"];
  const FAILURE_CATEGORIES: GlFailureCategory[] = ["DOMAIN", "INFRA"];

  it("GL_HOST_STATUS_LABELS covers all 6 enum values", () => {
    for (const s of GL_HOST_STATUSES) {
      expect(GL_HOST_STATUS_LABELS[s]).toBeTruthy();
    }
  });

  it("RECON_STATUS_LABELS covers all 5 enum values", () => {
    for (const s of RECON_STATUSES) {
      expect(RECON_STATUS_LABELS[s]).toBeTruthy();
    }
  });

  it("MISMATCH_TYPE_LABELS covers all 3 enum values", () => {
    for (const t of MISMATCH_TYPES) {
      expect(MISMATCH_TYPE_LABELS[t]).toBeTruthy();
    }
  });

  it("FAILURE_CATEGORY_LABELS covers all 2 enum values", () => {
    for (const c of FAILURE_CATEGORIES) {
      expect(FAILURE_CATEGORY_LABELS[c]).toBeTruthy();
    }
  });

  it("GL_HOST_STATUS_LABELS does not contain old enum REPLAYING", () => {
    expect(GL_HOST_STATUS_LABELS).not.toHaveProperty("REPLAYING");
    expect(GL_HOST_STATUS_LABELS).not.toHaveProperty("REPLAYED_OK");
    expect(GL_HOST_STATUS_LABELS).not.toHaveProperty("ABANDONED");
  });

  it("RECON_STATUS_LABELS does not contain old MATCH/MISMATCH enum values", () => {
    expect(RECON_STATUS_LABELS).not.toHaveProperty("MATCH");
    expect(RECON_STATUS_LABELS).not.toHaveProperty("MISMATCH");
  });
});

// ---------------------------------------------------------------------------
// PII sanitizer logic (extracted for unit test)
// ---------------------------------------------------------------------------

const PII_FIELDS = new Set(["customer_name", "account_no", "npwp"]);

function sanitizePii(obj: unknown): unknown {
  if (obj === null || typeof obj !== "object") return obj;
  if (Array.isArray(obj)) return obj.map(sanitizePii);
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    out[k] = PII_FIELDS.has(k) ? "[REDACTED]" : sanitizePii(v);
  }
  return out;
}

describe("sanitizePii (DLQ detail PII redaction)", () => {
  it("redacts top-level PII fields", () => {
    const payload = {
      customer_name: "John Doe",
      account_no: "1234567890",
      npwp: "01.234.567.8-901.000",
      journalDate: "2026-06-17",
    };
    const result = sanitizePii(payload) as Record<string, unknown>;
    expect(result.customer_name).toBe("[REDACTED]");
    expect(result.account_no).toBe("[REDACTED]");
    expect(result.npwp).toBe("[REDACTED]");
    expect(result.journalDate).toBe("2026-06-17"); // not PII — preserved
  });

  it("redacts nested PII fields recursively", () => {
    const payload = {
      header: {
        customer_name: "Jane Doe",
        period: "2026-06",
      },
      lines: [
        { account_no: "9876543210", amount: 1000000 },
      ],
    };
    const result = sanitizePii(payload) as Record<string, unknown>;
    const header = result.header as Record<string, unknown>;
    expect(header.customer_name).toBe("[REDACTED]");
    expect(header.period).toBe("2026-06"); // preserved

    const lines = result.lines as Record<string, unknown>[];
    expect(lines[0].account_no).toBe("[REDACTED]");
    expect(lines[0].amount).toBe(1000000); // preserved
  });

  it("passes non-PII keys unchanged", () => {
    const payload = { journalDate: "2026-06-17", totalLines: 5, entries: [] };
    const result = sanitizePii(payload) as Record<string, unknown>;
    expect(result).toEqual(payload);
  });

  it("handles null/primitive inputs gracefully", () => {
    expect(sanitizePii(null)).toBeNull();
    expect(sanitizePii("string")).toBe("string");
    expect(sanitizePii(42)).toBe(42);
    expect(sanitizePii(true)).toBe(true);
  });

  it("handles empty object", () => {
    expect(sanitizePii({})).toEqual({});
  });

  it("handles nested arrays", () => {
    const payload = { entries: [{ npwp: "secret", amount: 999 }] };
    const result = sanitizePii(payload) as { entries: Array<Record<string, unknown>> };
    expect(result.entries[0].npwp).toBe("[REDACTED]");
    expect(result.entries[0].amount).toBe(999);
  });
});
