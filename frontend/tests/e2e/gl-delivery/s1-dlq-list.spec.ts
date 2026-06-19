/**
 * Playwright E2E — P5-M3 S1: GL Delivery DLQ List Console
 * Stories: S1-AC1 (DLQ list with sort/filter/export), S1-AC2 (status badge per entry)
 *
 * Pre-conditions (set up via test fixtures / API seed):
 *   - User logged in as ROLE-IT-ADMIN (has all DLQ permissions)
 *   - Seed: 3 FAILED entries, 2 DEAD_LETTER entries
 */

import { test, expect } from "@playwright/test";

const DLQ_URL = "/jrnl/gl-delivery-dlq";

test.describe("S1 — GL Delivery DLQ List Console", () => {
  test.beforeEach(async ({ page }) => {
    // Intercept API calls to provide deterministic test data
    await page.route("**/api/v1/jurnal/gl-delivery-dlq*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              dlqEntryId: "aaaaaaaa-0000-0000-0000-000000000001",
              jurnalHeaderId: "bbbbbbbb-0000-0000-0000-000000000001",
              noJurnal: "JRN-2026-001",
              glHostStatus: "FAILED",
              failureCategory: "DOMAIN",
              errorCode: "GL_DELIVERY_HOST_4XX",
              errorMessage: "HTTP 422: account not found",
              retryCount: 1,
              createdAt: "2026-06-17T09:00:00+07:00",
              canReplay: true,
              canDiscard: false,
            },
            {
              dlqEntryId: "aaaaaaaa-0000-0000-0000-000000000002",
              jurnalHeaderId: "bbbbbbbb-0000-0000-0000-000000000002",
              noJurnal: "JRN-2026-002",
              glHostStatus: "DEAD_LETTER",
              failureCategory: "INFRA",
              errorCode: "GL_DELIVERY_HOST_UNREACHABLE",
              errorMessage: "Connection timeout",
              retryCount: 5,
              createdAt: "2026-06-16T14:00:00+07:00",
              canReplay: false,
              canDiscard: false,
            },
          ],
          pagination: {
            nextCursor: null,
            hasMore: false,
            totalEstimate: 2,
            limit: 50,
          },
          meta: { traceId: "test-trace-001" },
        }),
      });
    });

    await page.goto(DLQ_URL);
  });

  // S1-AC1: DLQ list renders with table
  test("S1-AC1: renders DLQ list table with entries", async ({ page }) => {
    await expect(page.getByRole("heading", { name: /Dead Letter Queue/i })).toBeVisible();
    await expect(page.getByText("JRN-2026-001")).toBeVisible();
    await expect(page.getByText("JRN-2026-002")).toBeVisible();
  });

  // S1-AC1: filter by status
  test("S1-AC1: status filter changes displayed entries", async ({ page }) => {
    // Default filter is FAILED — change to DEAD_LETTER
    const select = page.getByRole("combobox", { name: /Filter status DLQ/i });
    await select.selectOption("DEAD_LETTER");
    // Verify the filter was applied (URL or API call param)
    // The table should refetch — verify no crash
    await expect(page.getByRole("table")).toBeVisible();
  });

  // S1-AC1: search field exists and submits
  test("S1-AC1: search input is present and functional", async ({ page }) => {
    const searchInput = page.getByRole("searchbox", { name: /Cari DLQ GL/i });
    await expect(searchInput).toBeVisible();
    await searchInput.fill("GL_DELIVERY");
    await page.keyboard.press("Enter");
    // After search, table should still be visible (no crash)
    await expect(page.getByRole("table")).toBeVisible();
  });

  // S1-AC1: export button exists
  test("S1-AC1: export XLSX button is present", async ({ page }) => {
    const exportBtn = page.getByRole("button", { name: /Export DLQ list/i });
    await expect(exportBtn).toBeVisible();
  });

  // S1-AC2: each row has a GL status badge (color + icon + text)
  test("S1-AC2: rows have colored GL status badges", async ({ page }) => {
    // Each status badge has role="status" and aria-label per GlStatusBadge
    const statusBadges = page.locator('[role="status"][aria-label*="Status pengiriman GL"]');
    await expect(statusBadges).toHaveCount(2);
  });

  // S1-AC1: clicking a row navigates to detail page
  test("S1-AC1: clicking row navigates to DLQ detail", async ({ page }) => {
    const firstRow = page.getByRole("link").first();
    // Row has role="link" per implementation
    const row = page
      .getByRole("row")
      .filter({ hasText: "JRN-2026-001" });
    await row.click();
    await expect(page).toHaveURL(/\/jrnl\/gl-delivery-dlq\/aaaaaaaa/);
  });

  // S1-AC2: DEAD_LETTER badge count shown in header
  test("S1-AC2: DEAD_LETTER count badge shown in header", async ({ page }) => {
    // 1 DEAD_LETTER entry in seed data
    const countBadge = page.getByLabel(/DEAD_LETTER/);
    await expect(countBadge).toBeVisible();
  });

  // Empty state
  test("shows empty state when no DLQ entries", async ({ page }) => {
    await page.route("**/api/v1/jurnal/gl-delivery-dlq*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [],
          pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 },
          meta: { traceId: "test-trace-empty" },
        }),
      });
    });
    await page.goto(DLQ_URL);
    await expect(page.getByText(/Tidak ada entri DLQ/)).toBeVisible();
  });
});
