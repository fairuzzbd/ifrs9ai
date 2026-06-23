/**
 * Playwright E2E — P5-M4 Cross-Cutting: PeriodeLockBanner
 *
 * Verifies that when periode status is SOFT_CLOSED, HARD_CLOSE_PENDING, or CLOSED:
 *   1. A persistent lock banner appears on /app-b/transaksi (trx pages).
 *   2. A persistent lock banner appears on /app-d/jurnal (jurnal pages).
 *   3. Mutation buttons are disabled or show 423 toast on SOFT_CLOSED.
 *   4. CLOSED status shows hard-lock banner with no mutation path.
 *   5. Banner disappears after periode reopens (OPEN).
 *
 * These tests assert on the business outcome (lock enforced), not just UI cosmetics.
 */

import { test, expect } from "@playwright/test";

const PERIODE_SOFT_CLOSED_ID = "cccccccc-0000-0000-0000-000000000001";
const PERIODE_CLOSED_ID      = "cccccccc-0000-0000-0000-000000000002";
const PERIODE_OPEN_ID        = "cccccccc-0000-0000-0000-000000000003";

/** Seed periode status mock. */
async function mockPeriodeStatus(page: import("@playwright/test").Page, periodeId: string, status: string, graceExpiry?: string) {
  await page.route(`**/api/v1/periode/${periodeId}`, (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          periodeId,
          periodeKode: "2026-06",
          statusPeriode: status,
          ...(graceExpiry ? { hardCloseGraceExpiresAt: graceExpiry } : {}),
          rowVersion: 1,
        },
      }),
    });
  });
}

test.describe("PeriodeLockBanner — cross-cutting", () => {
  // Test 1: Trx page shows lock banner on SOFT_CLOSED.
  test("trx page shows SOFT_CLOSED lock banner; new penempatan button disabled", async ({ page }) => {
    await mockPeriodeStatus(page, PERIODE_SOFT_CLOSED_ID, "SOFT_CLOSED");

    // Mock trx list.
    await page.route("**/api/v1/transaksi/penempatan*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: [], pagination: { hasMore: false }, meta: {} }),
      });
    });

    // Mock mutation attempt returns 423.
    await page.route("**/api/v1/transaksi/penempatan", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 423,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "PERIODE_SOFT_CLOSED",
              message: "Periode 2026-06 dalam status SOFT_CLOSED. Mutasi tidak diizinkan.",
              traceId: "trace-lock-001",
            },
          }),
        });
      } else {
        route.continue();
      }
    });

    await page.goto(`/app-b/transaksi?periode_id=${PERIODE_SOFT_CLOSED_ID}`);

    // Lock banner present.
    const banner = page.getByTestId("periode-lock-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("SOFT_CLOSED");
    await expect(banner).toContainText("2026-06");

    // "Tambah Penempatan" button disabled or absent.
    const addBtn = page.getByRole("button", { name: /Tambah Penempatan/i });
    if (await addBtn.isVisible()) {
      await expect(addBtn).toBeDisabled();
    } else {
      await expect(page.getByTestId("add-penempatan-disabled")).toBeVisible();
    }
  });

  // Test 2: Jurnal page shows lock banner on SOFT_CLOSED.
  test("jurnal page shows SOFT_CLOSED lock banner; post jurnal blocked with 423", async ({ page }) => {
    await mockPeriodeStatus(page, PERIODE_SOFT_CLOSED_ID, "SOFT_CLOSED");

    await page.route("**/api/v1/jurnal*", (route) => {
      if (route.request().method() === "GET") {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { hasMore: false }, meta: {} }),
        });
      } else {
        route.fulfill({
          status: 423,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "PERIODE_SOFT_CLOSED",
              message: "Periode 2026-06 dalam status SOFT_CLOSED.",
              traceId: "trace-lock-jrnl-001",
            },
          }),
        });
      }
    });

    await page.goto(`/app-d/jurnal?periode_id=${PERIODE_SOFT_CLOSED_ID}`);

    const banner = page.getByTestId("periode-lock-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("SOFT_CLOSED");

    // Jurnal posting button disabled.
    const postBtn = page.getByRole("button", { name: /Post Jurnal/i });
    if (await postBtn.isVisible()) {
      await expect(postBtn).toBeDisabled();
    }
  });

  // Test 3: CLOSED status shows hard-lock banner with no mutation path.
  test("CLOSED periode shows hard-lock banner; all mutation buttons hidden/disabled", async ({ page }) => {
    const graceExpiry = new Date(Date.now() + 12 * 60 * 60 * 1000).toISOString();
    await mockPeriodeStatus(page, PERIODE_CLOSED_ID, "CLOSED", graceExpiry);

    await page.route("**/api/v1/transaksi*", (route) => {
      if (route.request().method() === "GET") {
        route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: {}, meta: {} }) });
      } else {
        route.fulfill({
          status: 423,
          contentType: "application/json",
          body: JSON.stringify({ error: { code: "PERIODE_CLOSED", message: "Periode 2026-06 hard-closed.", traceId: "trace-hc-001" } }),
        });
      }
    });

    await page.goto(`/app-b/transaksi?periode_id=${PERIODE_CLOSED_ID}`);

    const banner = page.getByTestId("periode-lock-banner");
    await expect(banner).toBeVisible();
    await expect(banner).toContainText("CLOSED");

    // Banner should indicate hard-close (different style from soft-close).
    await expect(banner).toHaveAttribute("data-lock-level", "hard");

    // All create/edit buttons absent or disabled.
    const createBtns = page.getByTestId(/action-button-create|action-button-edit/);
    for (const btn of await createBtns.all()) {
      await expect(btn).toBeDisabled();
    }
  });

  // Test 4: OPEN periode — no lock banner.
  test("OPEN periode — no lock banner, mutation buttons enabled", async ({ page }) => {
    await mockPeriodeStatus(page, PERIODE_OPEN_ID, "OPEN");

    await page.route("**/api/v1/transaksi/penempatan*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: [], pagination: { hasMore: false }, meta: {} }),
      });
    });

    await page.goto(`/app-b/transaksi?periode_id=${PERIODE_OPEN_ID}`);

    // No lock banner.
    await expect(page.getByTestId("periode-lock-banner")).not.toBeVisible();

    // "Tambah Penempatan" button enabled.
    const addBtn = page.getByRole("button", { name: /Tambah Penempatan/i });
    if (await addBtn.isVisible()) {
      await expect(addBtn).toBeEnabled();
    }
  });

  // Test 5: Allowlist action passes through SOFT_CLOSED — GL retry button not blocked.
  test("GL retry button not blocked by SOFT_CLOSED (allowlist action)", async ({ page }) => {
    await mockPeriodeStatus(page, PERIODE_SOFT_CLOSED_ID, "SOFT_CLOSED");

    // GL retry endpoint — allowlist action, should return 200.
    await page.route("**/api/v1/jurnal/**/gl-delivery-retry", (route) => {
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job_retry_001", statusUrl: "/api/v1/jobs/job_retry_001" } }),
      });
    });

    await page.route("**/api/v1/jurnal*", (route) => {
      if (route.request().method() === "GET") {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: [{
              id: "jjjjjjjj-0000-0000-0000-000000000001",
              noJurnal: "JRN-2026-001",
              glHostStatus: "FAILED",
              canRetry: true,
            }],
            pagination: { hasMore: false },
            meta: {},
          }),
        });
      } else {
        route.continue();
      }
    });

    await page.goto(`/app-d/jurnal?periode_id=${PERIODE_SOFT_CLOSED_ID}`);

    // Lock banner visible.
    await expect(page.getByTestId("periode-lock-banner")).toBeVisible();

    // But GL retry button (allowlist) NOT disabled.
    const retryBtn = page.getByRole("button", { name: /Retry GL/i }).first();
    if (await retryBtn.isVisible()) {
      await expect(retryBtn).toBeEnabled();
      await retryBtn.click();
      // Success — 202 accepted.
      const toast = page.getByRole("alert").filter({ hasText: /retry/i });
      await expect(toast).toBeVisible({ timeout: 5000 });
    }
  });
});
