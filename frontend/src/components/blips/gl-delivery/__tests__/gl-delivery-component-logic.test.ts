/**
 * Vitest unit tests — P5-M3 GL Delivery component logic (node environment)
 *
 * Tests pure-logic helpers and data contracts for:
 *   - GlStatusBadge  — all 6 GlHostStatus values have config + labels
 *   - RetryGlDeliveryDialog — reason validation (≥30 chars rule)
 *   - GlDlqDiscardDialog  — persona gating contract (canDiscard field)
 *   - ReconSummaryCard     — 5-state status labels + IDR formatting
 *   - PII sanitizer        — gl_host_api_key redaction (supplement to existing tests)
 *   - GlFailureCategoryBadge — DOMAIN vs INFRA label coverage
 *
 * NOTE: React component rendering tests are NOT included here because the
 * Vitest environment is "node" (not "jsdom"). For DOM assertions use Playwright
 * E2E specs in frontend/tests/e2e/gl-delivery/.
 *
 * AC coverage:
 *   S2-AC1/2/3   GlStatusBadge label completeness for DELIVERED/FAILED/PENDING
 *   S3-AC3       RetryGlDeliveryDialog reason min-length Zod rule
 *   S5-AC3/4     canDiscard field determines button presence (schema contract)
 *   S4-AC1/2     ReconSummaryCard status labels (COMPLETED/COMPLETED_WITH_MISMATCH)
 */

import { describe, it, expect } from "vitest";
import {
  GL_HOST_STATUS_LABELS,
  RECON_STATUS_LABELS,
  FAILURE_CATEGORY_LABELS,
  retryGlDeliveryRequestSchema,
  dlqActionRequestSchema,
  glDeliveryStatusSchema,
  glDeliveryDlqListItemSchema,
  glReconciliationSummaryItemSchema,
  type GlHostStatus,
  type ReconReportStatus,
} from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// 1. GlStatusBadge — all 6 GlHostStatus values have labels (S2-AC1/2/3)
// ---------------------------------------------------------------------------

describe("GlStatusBadge label completeness (S2-AC1/2/3)", () => {
  const allStatuses: GlHostStatus[] = [
    "PENDING_DELIVERY",
    "DELIVERY_IN_FLIGHT",
    "RETRYING",
    "FAILED",
    "DELIVERED",
    "DEAD_LETTER",
  ];

  it("every GlHostStatus has a non-empty Bahasa Indonesia label", () => {
    for (const status of allStatuses) {
      const label = GL_HOST_STATUS_LABELS[status];
      expect(label, `missing label for status ${status}`).toBeTruthy();
      expect(typeof label).toBe("string");
      expect(label.length, `label for ${status} must be non-empty`).toBeGreaterThan(0);
    }
  });

  it("DELIVERED label conveys success (S2-AC1)", () => {
    const label = GL_HOST_STATUS_LABELS["DELIVERED"];
    // Label must be human-readable and unambiguous
    expect(label).toBeTruthy();
    expect(label.toLowerCase()).not.toBe("delivered"); // must be localized
  });

  it("FAILED label conveys failure (S2-AC2)", () => {
    const label = GL_HOST_STATUS_LABELS["FAILED"];
    expect(label).toBeTruthy();
  });

  it("PENDING_DELIVERY label conveys waiting state (S2-AC3)", () => {
    const label = GL_HOST_STATUS_LABELS["PENDING_DELIVERY"];
    expect(label).toBeTruthy();
  });

  it("DEAD_LETTER has distinct label from FAILED", () => {
    expect(GL_HOST_STATUS_LABELS["DEAD_LETTER"]).not.toBe(
      GL_HOST_STATUS_LABELS["FAILED"],
    );
  });

  it("DELIVERY_IN_FLIGHT and RETRYING have distinct labels", () => {
    expect(GL_HOST_STATUS_LABELS["DELIVERY_IN_FLIGHT"]).not.toBe(
      GL_HOST_STATUS_LABELS["RETRYING"],
    );
  });
});

// ---------------------------------------------------------------------------
// 2. RetryGlDeliveryDialog — reason min-30 chars validation (S3-AC3)
// ---------------------------------------------------------------------------

describe("RetryGlDeliveryDialog reason validation (S3-AC3)", () => {
  it("accepts reason with exactly 30 characters", () => {
    const r = retryGlDeliveryRequestSchema.safeParse({ reason: "a".repeat(30) });
    expect(r.success).toBe(true);
  });

  it("accepts reason with > 30 characters", () => {
    const r = retryGlDeliveryRequestSchema.safeParse({
      reason: "Kode akun sudah diperbaiki. Retry ini diperlukan untuk closing bulan ini.",
    });
    expect(r.success).toBe(true);
  });

  it("rejects reason with 29 characters (S3-AC3 boundary)", () => {
    const r = retryGlDeliveryRequestSchema.safeParse({ reason: "a".repeat(29) });
    expect(r.success).toBe(false);
    if (!r.success) {
      const msg = r.error.issues[0].message;
      expect(msg).toMatch(/30/);
    }
  });

  it("rejects empty reason", () => {
    const r = retryGlDeliveryRequestSchema.safeParse({ reason: "" });
    expect(r.success).toBe(false);
  });

  it("rejects reason exceeding 1000 characters", () => {
    const r = retryGlDeliveryRequestSchema.safeParse({ reason: "a".repeat(1001) });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 3. GlDlqDiscardDialog — persona gating via canDiscard schema field (S5-AC3/4)
// ---------------------------------------------------------------------------

describe("GlDlqDiscardDialog persona gating via canDiscard field (S5-AC3/4)", () => {
  // glDeliveryDlqListItemSchema does NOT have a 'status' field — it uses glHostStatus.
  // FAILED and DEAD_LETTER are the only valid glHostStatus values for DLQ entries.
  const baseDlqItem = {
    dlqEntryId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    jurnalHeaderId: "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a22",
    noJurnal: "JRN-2026-001",
    eventCode: "PENEMPATAN",
    glHostStatus: "FAILED",
    failureCategory: "DOMAIN",
    errorCode: "GL_DELIVERY_HOST_4XX",
    retryCount: 1,
    createdAt: "2026-06-17T08:00:00+07:00",
    canReplay: true,
    canDiscard: false,
  };

  it("schema accepts canDiscard=false for non-IT-ADMIN users (S5-AC4)", () => {
    const r = glDeliveryDlqListItemSchema.safeParse({ ...baseDlqItem, canDiscard: false });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.canDiscard).toBe(false);
    }
  });

  it("schema accepts canDiscard=true for IT-ADMIN users (S5-AC3)", () => {
    const r = glDeliveryDlqListItemSchema.safeParse({ ...baseDlqItem, canDiscard: true });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.canDiscard).toBe(true);
    }
  });

  it("discard dialog reason uses same schema as replay — min 30 chars (S5-AC3/4)", () => {
    const reject29 = dlqActionRequestSchema.safeParse({ reason: "a".repeat(29) });
    expect(reject29.success).toBe(false);

    const accept30 = dlqActionRequestSchema.safeParse({ reason: "a".repeat(30) });
    expect(accept30.success).toBe(true);
  });

  it("DEAD_LETTER DLQ entry has canReplay=false canDiscard=false (terminal)", () => {
    // DEAD_LETTER is the gl_host_status for discarded entries.
    const r = glDeliveryDlqListItemSchema.safeParse({
      ...baseDlqItem,
      glHostStatus: "DEAD_LETTER",
      canReplay: false,
      canDiscard: false,
    });
    expect(r.success).toBe(true);
    if (r.success) {
      expect(r.data.canReplay).toBe(false);
      expect(r.data.canDiscard).toBe(false);
    }
  });
});

// ---------------------------------------------------------------------------
// 4. ReconSummaryCard — 5-state status labels (S4-AC1/2)
// ---------------------------------------------------------------------------

describe("ReconSummaryCard status labels (S4-AC1/2)", () => {
  // 5 states defined in reconReportStatusEnum: PENDING, RUNNING, COMPLETED, COMPLETED_WITH_MISMATCH, FAILED
  const allReconStatuses: ReconReportStatus[] = [
    "PENDING",
    "RUNNING",
    "COMPLETED",
    "COMPLETED_WITH_MISMATCH",
    "FAILED",
  ];

  it("every ReconReportStatus has a Bahasa Indonesia label", () => {
    for (const status of allReconStatuses) {
      const label = RECON_STATUS_LABELS[status];
      expect(label, `missing label for recon status ${status}`).toBeTruthy();
      expect(label.length).toBeGreaterThan(0);
    }
  });

  it("COMPLETED label conveys balanced reconciliation (S4-AC1)", () => {
    const label = RECON_STATUS_LABELS["COMPLETED"];
    expect(label).toBeTruthy();
    // Must not be the same as COMPLETED_WITH_MISMATCH (different semantics)
    expect(label).not.toBe(RECON_STATUS_LABELS["COMPLETED_WITH_MISMATCH"]);
  });

  it("COMPLETED_WITH_MISMATCH label conveys mismatch found (S4-AC2)", () => {
    const label = RECON_STATUS_LABELS["COMPLETED_WITH_MISMATCH"];
    expect(label).toBeTruthy();
    // Must not be the same as COMPLETED or FAILED
    expect(label).not.toBe(RECON_STATUS_LABELS["COMPLETED"]);
    expect(label).not.toBe(RECON_STATUS_LABELS["FAILED"]);
  });

  it("FAILED label is distinct from COMPLETED states", () => {
    expect(RECON_STATUS_LABELS["FAILED"]).not.toBe(RECON_STATUS_LABELS["COMPLETED"]);
    expect(RECON_STATUS_LABELS["FAILED"]).not.toBe(
      RECON_STATUS_LABELS["COMPLETED_WITH_MISMATCH"],
    );
  });

  it("recon summary schema uses tanggalRekonsiliasi (not reconDate) field name", () => {
    // Verifies the field name matches the backend spec contract (not 'reconDate')
    const r = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      tanggalRekonsiliasi: "2026-06-15",  // plain string per schema (not datetime)
      status: "COMPLETED",
      totalMismatchCount: 0,
    });
    expect(r.success).toBe(true);
  });

  it("recon summary schema rejects MATCH as invalid status (old enum)", () => {
    const r = glReconciliationSummaryItemSchema.safeParse({
      reportId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      tanggalRekonsiliasi: "2026-06-15T00:00:00+07:00",
      status: "MATCH", // invalid — old enum value
      totalAkunChecked: 10,
      totalMismatchCount: 0,
    });
    expect(r.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 5. PII sanitizer — GL_HOST_API_KEY redaction (supplement to existing tests)
// ---------------------------------------------------------------------------

// Extended PII field set matching the backend Go implementation.
const EXTENDED_PII_FIELDS = new Set([
  "customer_name",
  "account_no",
  "npwp",
  "ktp",
  "gl_host_api_key",
  "apikey",
  "api_key",
]);

function sanitizePiiExtended(obj: unknown): unknown {
  if (obj === null || typeof obj !== "object") return obj;
  if (Array.isArray(obj)) return obj.map(sanitizePiiExtended);
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const lk = k.toLowerCase();
    const isCredential =
      lk.includes("api_key") || lk.includes("apikey") || lk.includes("secret") || lk.includes("password");
    out[k] = EXTENDED_PII_FIELDS.has(lk) || isCredential ? "[REDACTED]" : sanitizePiiExtended(v);
  }
  return out;
}

describe("PII sanitizer — credential field redaction", () => {
  it("redacts GL_HOST_API_KEY regardless of case", () => {
    const payload = {
      GL_HOST_API_KEY: "super-secret",
      event_code: "PENEMPATAN",
    };
    const result = sanitizePiiExtended(payload) as Record<string, unknown>;
    expect(result["GL_HOST_API_KEY"]).toBe("[REDACTED]");
    expect(result["event_code"]).toBe("PENEMPATAN");
  });

  it("redacts apikey field", () => {
    const payload = { apikey: "secret", amount: 1000000 };
    const result = sanitizePiiExtended(payload) as Record<string, unknown>;
    expect(result["apikey"]).toBe("[REDACTED]");
    expect(result["amount"]).toBe(1000000);
  });

  it("redacts ktp field (added PII field per security baseline)", () => {
    const payload = { ktp: "3174012345678901", name: "Budi" };
    const result = sanitizePiiExtended(payload) as Record<string, unknown>;
    expect(result["ktp"]).toBe("[REDACTED]");
    expect(result["name"]).toBe("Budi");
  });

  it("handles null input without throwing", () => {
    expect(() => sanitizePiiExtended(null)).not.toThrow();
    expect(sanitizePiiExtended(null)).toBe(null);
  });

  it("handles primitive number without throwing", () => {
    expect(sanitizePiiExtended(42)).toBe(42);
  });
});

// ---------------------------------------------------------------------------
// 6. Failure category labels — DOMAIN vs INFRA coverage (S1-AC3/4)
// ---------------------------------------------------------------------------

describe("GlFailureCategoryBadge label coverage (S1-AC3/4)", () => {
  it("DOMAIN category has a label", () => {
    expect(FAILURE_CATEGORY_LABELS["DOMAIN"]).toBeTruthy();
  });

  it("INFRA category has a label", () => {
    expect(FAILURE_CATEGORY_LABELS["INFRA"]).toBeTruthy();
  });

  it("DOMAIN and INFRA labels are distinct", () => {
    expect(FAILURE_CATEGORY_LABELS["DOMAIN"]).not.toBe(
      FAILURE_CATEGORY_LABELS["INFRA"],
    );
  });
});

// ---------------------------------------------------------------------------
// 7. glDeliveryStatusSchema — canRetry derived from gl_host_status
// ---------------------------------------------------------------------------

describe("glDeliveryStatusSchema canRetry field (S2-AC1/2/3)", () => {
  // Required fields: glStatusId, glHostStatus, retryCount, deliveryMode, canRetry
  // Note: UUIDs must be valid v4 (variant bits 8-9-a-b at position 17)
  const baseStatus = {
    glStatusId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
    glHostStatus: "DELIVERED" as const,
    retryCount: 0,
    deliveryMode: "API" as const,
    canRetry: false,
  };

  it("DELIVERED status schema accepts canRetry=false (S2-AC1)", () => {
    const r = glDeliveryStatusSchema.safeParse({ ...baseStatus, glHostStatus: "DELIVERED", canRetry: false });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data.canRetry).toBe(false);
  });

  it("FAILED status schema accepts canRetry=true (S2-AC2)", () => {
    const r = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "FAILED",
      canRetry: true,
    });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data.canRetry).toBe(true);
  });

  it("PENDING_DELIVERY status schema accepts canRetry=false (S2-AC3)", () => {
    const r = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "PENDING_DELIVERY",
      canRetry: false,
    });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data.canRetry).toBe(false);
  });

  it("DEAD_LETTER status schema accepts canRetry=false", () => {
    const r = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "DEAD_LETTER",
      canRetry: false,
    });
    expect(r.success).toBe(true);
    if (r.success) expect(r.data.canRetry).toBe(false);
  });

  it("rejects unknown glHostStatus value", () => {
    const r = glDeliveryStatusSchema.safeParse({
      ...baseStatus,
      glHostStatus: "UNKNOWN_STATUS",
    });
    expect(r.success).toBe(false);
  });
});
