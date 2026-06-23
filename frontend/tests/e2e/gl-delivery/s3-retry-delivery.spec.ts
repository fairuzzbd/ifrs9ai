/**
 * Playwright E2E — P5-M3 S3: Manual Retry GL Delivery
 * Stories: S3-AC1 (retry from journal detail panel), S3-AC2 (validation fail < 30 chars),
 *          S3-AC3 (DEAD_LETTER hides retry button), S3-AC4 (success toast + refetch)
 *
 * Pre-conditions:
 *   - User logged in as ROLE-AKUN (has jurnal.gl_delivery.retry permission)
 *   - Seed: journal JRN-2026-010 with GL status FAILED + canRetry=true
 */

import { test, expect } from "@playwright/test";

const JURNAL_DETAIL_URL = "/jrnl/journal-entries/jrnl-hdr-test-001";
const JURNAL_ID = "jrnl-hdr-test-001";

// Mock GL delivery status responses
function mockGlStatus(page: Parameters<typeof test>[1]["page"], status: string, canRetry: boolean) {
  return page.route(
    `**/api/v1/jurnal/header/${JURNAL_ID}/gl-delivery-status`,
    (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            glStatusId: "cccccccc-0000-0000-0000-000000000001",
            glHostStatus: status,
            retryCount: 2,
            deliveryMode: "API",
            canRetry,
            failureCategory: status === "FAILED" ? "INFRA" : null,
            lastError: status === "FAILED" ? "GL Host timeout" : null,
          },
          meta: { traceId: "test-s3-001" },
        }),
      });
    },
  );
}

test.describe("S3 — Manual Retry GL Delivery", () => {
  // S3-AC1: retry button visible for FAILED + canRetry
  test("S3-AC1: retry button shown for FAILED + canRetry=true", async ({ page }) => {
    await mockGlStatus(page, "FAILED", true);
    await page.route(`**/api/v1/jurnal/header/${JURNAL_ID}/**`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { noJurnal: "JRN-2026-010" }, meta: { traceId: "t" } }),
      });
    });

    await page.goto(JURNAL_DETAIL_URL);

    // GlDeliveryStatusPanel should show retry button
    const retryBtn = page.getByRole("button", { name: /Retry/i });
    await expect(retryBtn).toBeVisible();
  });

  // S3-AC3: retry button NOT shown for DEAD_LETTER
  test("S3-AC3: retry button hidden for DEAD_LETTER", async ({ page }) => {
    await mockGlStatus(page, "DEAD_LETTER", false);
    await page.goto(JURNAL_DETAIL_URL);

    const retryBtn = page.getByRole("button", { name: /Retry/i });
    await expect(retryBtn).not.toBeVisible();
  });

  // S3-AC2: validation fail — reason < 30 chars shows inline error
  test("S3-AC2: retry dialog shows error for reason < 30 chars", async ({ page }) => {
    await mockGlStatus(page, "FAILED", true);
    await page.goto(JURNAL_DETAIL_URL);

    // Open retry dialog
    await page.getByRole("button", { name: /Retry/i }).click();

    // Fill short reason
    const reasonField = page.getByLabel(/Alasan/i);
    await reasonField.fill("Terlalu pendek.");

    // Submit
    await page.getByRole("button", { name: /Konfirmasi Retry/i }).click();

    // Should see validation error inline
    await expect(page.getByText(/30/)).toBeVisible();

    // Dialog should still be open (not closed on validation fail)
    await expect(page.getByRole("dialog")).toBeVisible();
  });

  // S3-AC4: successful retry shows toast and closes dialog
  test("S3-AC4: successful retry shows success toast", async ({ page }) => {
    await mockGlStatus(page, "FAILED", true);

    // Mock retry endpoint
    await page.route(
      `**/api/v1/jurnal/header/${JURNAL_ID}/retry-gl-delivery`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              jobId: "job-retry-001",
              statusUrl: "/api/v1/jobs/job-retry-001",
              glStatusId: "cccccccc-0000-0000-0000-000000000001",
              previousStatus: "FAILED",
              newStatus: "RETRYING",
              retryAttemptNumber: 3,
            },
            meta: { traceId: "t-retry" },
          }),
        });
      },
    );

    await page.goto(JURNAL_DETAIL_URL);
    await page.getByRole("button", { name: /Retry/i }).click();

    const reasonField = page.getByLabel(/Alasan/i);
    await reasonField.fill(
      "Retry manual karena GL Host sudah pulih dari downtime terjadwal kemarin sore.",
    );
    await page.getByRole("button", { name: /Konfirmasi Retry/i }).click();

    // Toast sukses harus muncul (§2 UX pattern)
    await expect(page.getByText(/Retry GL delivery berhasil/i)).toBeVisible({ timeout: 5000 });
  });
});
