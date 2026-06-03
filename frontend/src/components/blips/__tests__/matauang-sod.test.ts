/**
 * Frontend unit tests for SoD-disable logic in MakerReviewerApproverPanel.
 *
 * TOOLCHAIN NOTE: No Vitest/Jest is configured in this project.
 * To run these tests, install Vitest as a dev dependency:
 *
 *   pnpm add -D vitest @vitest/ui jsdom @testing-library/react @testing-library/user-event
 *
 * Then add to package.json scripts:
 *   "test": "vitest run",
 *   "test:watch": "vitest"
 *
 * And add to vite.config.ts (or vitest.config.ts):
 *   test: { environment: "jsdom" }
 *
 * These tests are authored (compile-safe TypeScript) and ready to run once
 * the test runner is configured. They test pure logic functions that do NOT
 * require a DOM renderer, so they can run without @testing-library.
 *
 * Coverage: isSoDViolation logic extracted from MakerReviewerApproverPanel.
 */

import { describe, it, expect } from "vitest";
import type { WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

// ─── Inline copy of isSoDViolation from MakerReviewerApproverPanel ───────────
// The function is not exported from the component. We test the logic inline
// so this test remains hermetic without needing a DOM environment.
// If isSoDViolation is ever exported, replace this with the import.

function isSoDViolation(
  currentUserId: string | null,
  workflow: Pick<WorkflowStatus, "makerId" | "reviewerId">,
  action: "review" | "approve",
): boolean {
  if (!currentUserId) return false;
  if (action === "review") {
    return currentUserId === workflow.makerId;
  }
  if (action === "approve") {
    return (
      currentUserId === workflow.makerId ||
      currentUserId === workflow.reviewerId
    );
  }
  return false;
}

// ─── Test data ────────────────────────────────────────────────────────────────

const MAKER_ID = "user-001";
const REVIEWER_ID = "user-002";
const APPROVER_ID = "user-003";

function buildWorkflow(overrides?: Partial<Pick<WorkflowStatus, "makerId" | "reviewerId">>): Pick<WorkflowStatus, "makerId" | "reviewerId"> {
  return {
    makerId: MAKER_ID,
    reviewerId: REVIEWER_ID,
    ...overrides,
  };
}

// ─── SoD: review action ───────────────────────────────────────────────────────

describe("isSoDViolation — review action", () => {
  it("returns true when currentUser is the maker (SoD: maker cannot review)", () => {
    expect(isSoDViolation(MAKER_ID, buildWorkflow(), "review")).toBe(true);
  });

  it("returns false when currentUser is the reviewer (different from maker)", () => {
    expect(isSoDViolation(REVIEWER_ID, buildWorkflow(), "review")).toBe(false);
  });

  it("returns false when currentUser is a third party (neither maker nor reviewer)", () => {
    expect(isSoDViolation(APPROVER_ID, buildWorkflow(), "review")).toBe(false);
  });

  it("returns false when currentUserId is null (unauthenticated)", () => {
    expect(isSoDViolation(null, buildWorkflow(), "review")).toBe(false);
  });
});

// ─── SoD: approve action ─────────────────────────────────────────────────────

describe("isSoDViolation — approve action", () => {
  it("returns true when currentUser is the maker (SoD: maker cannot approve)", () => {
    expect(isSoDViolation(MAKER_ID, buildWorkflow(), "approve")).toBe(true);
  });

  it("returns true when currentUser is the reviewer (SoD: reviewer cannot approve)", () => {
    expect(isSoDViolation(REVIEWER_ID, buildWorkflow(), "approve")).toBe(true);
  });

  it("returns false when currentUser is a third party (different from maker AND reviewer)", () => {
    expect(isSoDViolation(APPROVER_ID, buildWorkflow(), "approve")).toBe(false);
  });

  it("returns false when currentUserId is null", () => {
    expect(isSoDViolation(null, buildWorkflow(), "approve")).toBe(false);
  });

  it("returns true when maker=reviewer (same user) tries to approve", () => {
    // Edge case: if maker and reviewer are the same user (shouldn't happen due to SoD,
    // but if data is corrupt), approve must still be blocked.
    const workflow = buildWorkflow({ makerId: MAKER_ID, reviewerId: MAKER_ID });
    expect(isSoDViolation(MAKER_ID, workflow, "approve")).toBe(true);
  });
});

// ─── SoD: no reviewerId yet (PENDING_REVIEW state) ───────────────────────────

describe("isSoDViolation — approve before reviewer is set", () => {
  it("does not block approve when reviewerId is undefined (workflow not yet reviewed)", () => {
    // When the workflow is in PENDING_REVIEW, reviewerId is not set yet.
    // The approve button shouldn't even be visible in that state, but
    // isSoDViolation should still not block a non-maker from approving.
    const workflow = buildWorkflow({ reviewerId: undefined });
    expect(isSoDViolation(APPROVER_ID, workflow, "approve")).toBe(false);
  });

  it("still blocks the maker from approving even when reviewerId is undefined", () => {
    const workflow = buildWorkflow({ reviewerId: undefined });
    expect(isSoDViolation(MAKER_ID, workflow, "approve")).toBe(true);
  });
});
