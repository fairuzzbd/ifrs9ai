/**
 * Playwright E2E — P5-M3 S5: DLQ Action Panel (Replay + Discard)
 * Stories: S5-AC1 (replay dialog happy path), S5-AC2 (replay validation fail),
 *          S5-AC3 (discard IT-ADMIN only — not in DOM for others),
 *          S5-AC4 (PII redacted for non-IT-ADMIN)
 *
 * Pre-conditions:
 *   - Two users: ROLE-AKUN (canReplay, !canDiscard) and ROLE-IT-ADMIN (canDiscard)
 *   - Seed: DLQ entry aaaaaaaa-0000-0000-0000-000000000001, status FAILED
 */

import { test, expect } from "@playwright/test";

const DLQ_DETAIL_URL = "/jrnl/gl-delivery-dlq/aaaaaaaa-0000-0000-0000-000000000001";
const DLQ_ENTRY_ID = "aaaaaaaa-0000-0000-0000-000000000001";
const JRNL_HEADER_ID = "bbbbbbbb-0000-0000-0000-000000000001";

function mockDlqDetail(
  page: Parameters<typeof test>[1]["page"],
  status: string,
  extras: Record<string, unknown> = {},
) {
  return page.route(`**/api/v1/jurnal/gl-delivery-dlq/${DLQ_ENTRY_ID}`, (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          dlqEntryId: DLQ_ENTRY_ID,
          jurnalHeaderId: JRNL_HEADER_ID,
          noJurnal: "JRN-2026-005",
          glHostStatus: status,
          failureCategory: "INFRA",
          errorCode: "GL_DELIVERY_HOST_UNREACHABLE",
          errorMessage: "Connection timeout after 30s",
          retryCount: 3,
          createdAt: "2026-06-17T08:00:00+07:00",
          canReplay: status === "FAILED",
          canDiscard: status === "FAILED",
          payloadSnapshotJsonb: {
            journalDate: "2026-06-17",
            customer_name: "Test Customer",
            account_no: "1234567890",
            entries: [{ amount: 1000000 }],
          },
          ...extras,
        },
        meta: { traceId: "test-s5-001" },
      }),
    });
  });
}

test.describe("S5 — DLQ Action Panel", () => {
  // S5-AC1 happy path: replay dialog works end-to-end
  test("S5-AC1: replay dialog submits successfully", async ({ page }) => {
    await mockDlqDetail(page, "FAILED");

    await page.route(
      `**/api/v1/jurnal/gl-delivery-dlq/${DLQ_ENTRY_ID}/replay`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              jobId: "job-replay-001",
              statusUrl: "/api/v1/jobs/job-replay-001",
              dlqEntryId: DLQ_ENTRY_ID,
              jurnalHeaderId: JRNL_HEADER_ID,
              noJurnal: "JRN-2026-005",
              previousStatus: "FAILED",
              newStatus: "PENDING_DELIVERY",
            },
            meta: { traceId: "t-replay" },
          }),
        });
      },
    );

    await page.goto(DLQ_DETAIL_URL);

    // Replay button visible
    const replayBtn = page.getByRole("button", { name: /Replay/i });
    await expect(replayBtn).toBeVisible();
    await replayBtn.click();

    // Dialog opens
    await expect(page.getByRole("dialog")).toBeVisible();

    // Fill reason (≥ 30 chars)
    const reasonField = page.getByLabel(/Alasan/i);
    await reasonField.fill(
      "GL Host sudah pulih dari downtime. Replay diperlukan untuk jurnal closing bulan Juni.",
    );

    await page.getByRole("button", { name: /Konfirmasi Replay/i }).click();

    // Success toast appears (§2 UX pattern)
    await expect(page.getByText(/Replay berhasil/i)).toBeVisible({ timeout: 5000 });

    // Dialog closes after success
    await expect(page.getByRole("dialog")).not.toBeVisible({ timeout: 3000 });
  });

  // S5-AC2: validation fail — reason < 30 chars
  test("S5-AC2: replay validation fail shows inline error", async ({ page }) => {
    await mockDlqDetail(page, "FAILED");
    await page.goto(DLQ_DETAIL_URL);

    await page.getByRole("button", { name: /Replay/i }).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    const reasonField = page.getByLabel(/Alasan/i);
    await reasonField.fill("Terlalu pendek."); // < 30 chars

    await page.getByRole("button", { name: /Konfirmasi Replay/i }).click();

    // Inline error message visible
    await expect(page.getByText(/30/)).toBeVisible();

    // Dialog stays open
    await expect(page.getByRole("dialog")).toBeVisible();
  });

  // S5-AC3: discard button NOT in DOM for non-IT-ADMIN
  test("S5-AC3: discard button not present for non-ROLE-IT-ADMIN", async ({ page }) => {
    // This test simulates a ROLE-AKUN user: server returns canDiscard=false
    await page.route(`**/api/v1/jurnal/gl-delivery-dlq/${DLQ_ENTRY_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            dlqEntryId: DLQ_ENTRY_ID,
            jurnalHeaderId: JRNL_HEADER_ID,
            noJurnal: "JRN-2026-005",
            glHostStatus: "FAILED",
            failureCategory: "INFRA",
            errorCode: "GL_DELIVERY_HOST_UNREACHABLE",
            retryCount: 2,
            createdAt: "2026-06-17T08:00:00+07:00",
            canReplay: true,
            canDiscard: false, // non-IT-ADMIN
          },
          meta: { traceId: "test-s5-003" },
        }),
      });
    });

    // Also mock permissions: no ROLE-IT-ADMIN
    await page.goto(DLQ_DETAIL_URL);

    // Discard button must NOT be in the DOM at all (not just hidden/disabled)
    // Per S5-AC3 requirement: "absent from DOM for non-ROLE-IT-ADMIN"
    const discardBtn = page.getByRole("button", { name: /Discard/i });
    await expect(discardBtn).toHaveCount(0);

    // Replay button IS present
    await expect(page.getByRole("button", { name: /Replay/i })).toBeVisible();
  });

  // S5-AC3: discard dialog works for ROLE-IT-ADMIN
  test("S5-AC3: discard dialog available for ROLE-IT-ADMIN", async ({ page }) => {
    await mockDlqDetail(page, "FAILED");

    await page.route(
      `**/api/v1/jurnal/gl-delivery-dlq/${DLQ_ENTRY_ID}/discard`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              dlqEntryId: DLQ_ENTRY_ID,
              jurnalHeaderId: JRNL_HEADER_ID,
              noJurnal: "JRN-2026-005",
              previousStatus: "FAILED",
              newStatus: "DEAD_LETTER",
              discardedAt: "2026-06-17T12:00:00+07:00",
              discardedBy: "eeeeeeee-0000-0000-0000-000000000001",
            },
            meta: { traceId: "t-discard" },
          }),
        });
      },
    );

    await page.goto(DLQ_DETAIL_URL);

    // For IT-ADMIN, discard button is present (canDiscard=true from mock)
    const discardBtn = page.getByRole("button", { name: /Discard/i });
    await expect(discardBtn).toBeVisible();
    await discardBtn.click();

    // Warning dialog opens
    await expect(page.getByRole("alertdialog")).toBeVisible();
    await expect(page.getByText(/permanen|tidak dapat dibatalkan/i)).toBeVisible();

    // Fill reason
    const reasonField = page.getByLabel(/Alasan discard/i);
    await reasonField.fill(
      "GL Host telah mengkonfirmasi kode akun tidak valid dan tidak bisa diperbaiki untuk jurnal ini.",
    );

    await page.getByRole("button", { name: /Konfirmasi Discard/i }).click();

    // Success toast
    await expect(page.getByText(/berhasil di-discard/i)).toBeVisible({ timeout: 5000 });
  });

  // S5-AC4: PII redacted for non-IT-ADMIN
  test("S5-AC4: PII fields redacted in payload for non-IT-ADMIN", async ({ page }) => {
    await mockDlqDetail(page, "FAILED");
    // Assume non-IT-ADMIN role via auth mock
    await page.goto(DLQ_DETAIL_URL);

    // Page shows payload section
    // PII fields should show [REDACTED]
    await expect(page.getByText("[REDACTED]")).toBeVisible();

    // Non-PII field should be visible
    await expect(page.getByText("1000000")).toBeVisible();
  });

  // S5: DEAD_LETTER entry shows terminal state — no replay/discard
  test("S5: DEAD_LETTER entry shows terminal state, no actions", async ({ page }) => {
    await mockDlqDetail(page, "DEAD_LETTER", {
      canReplay: false,
      canDiscard: false,
      discardInfo: {
        discardedBy: "eeeeeeee-0000-0000-0000-000000000001",
        discardedAt: "2026-06-17T11:00:00+07:00",
        discardReason: "Permanent rejection confirmed.",
      },
    });

    await page.goto(DLQ_DETAIL_URL);

    // Status badge shows DEAD_LETTER
    await expect(
      page.locator('[role="status"][aria-label*="Dihentikan"]'),
    ).toBeVisible();

    // No replay or discard buttons
    await expect(page.getByRole("button", { name: /Replay/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /Discard/i })).toHaveCount(0);

    // Discard reason shown
    await expect(page.getByText("Permanent rejection confirmed.")).toBeVisible();
  });
});
