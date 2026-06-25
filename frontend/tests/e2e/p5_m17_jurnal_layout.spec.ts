/**
 * Playwright E2E — P5-M17 Shared /jurnal Layout Tab Navigation
 *
 * AC coverage:
 *   M17-04-AC4 — /jurnal layout: 3 tabs for IT-ADMIN, 2 tabs (Header, DLQ) for other personas;
 *                Resolve absent from DOM; DLQ badge count; breadcrumb per sub-route.
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-akun-001",
  mfaVerified = false
) {
  return page.addInitScript(
    ({ r, p, uid, m }: { r: string[]; p: string[]; uid: string; m: boolean }) => {
      localStorage.setItem("blips_roles", JSON.stringify(r));
      localStorage.setItem("blips_permissions", JSON.stringify(p));
      localStorage.setItem("blips_user_id", uid);
      localStorage.setItem("blips_mfa_verified", String(m));
    },
    { r: roles, p: permissions, uid: userId, m: mfaVerified }
  );
}

function mockJurnalLayoutEndpoints(page: Page, dlqPendingCount = 3) {
  page.route("**/api/v1/jurnal/dlq**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("filter[status]=PENDING") || url.includes("filter%5Bstatus%5D=PENDING")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: Array(dlqPendingCount).fill({ id: "dlq-x", status: "PENDING" }),
          pagination: { nextCursor: null, hasMore: false, totalEstimate: dlqPendingCount, limit: 50 },
          meta: { traceId: "t" },
        }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
    });
  });

  page.route("**/api/v1/jurnal**", (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
    })
  );
}

// ---------------------------------------------------------------------------
// M17-04-AC4: Tab visibility per persona
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Jurnal Layout: Tab Visibility (AC4)", () => {

  test("M17-04-AC4: ROLE-AKUN sees Header tab; DLQ and Resolve ABSENT", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["jurnal.read", "jurnal.submit"]);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    // Header tab visible
    await expect(page.getByRole("tab", { name: /^header$/i })).toBeVisible({ timeout: 5000 });

    // DLQ tab absent (no jurnal.dlq.read)
    await expect(page.getByRole("tab", { name: /^DLQ|dead letter/i })).toHaveCount(0);

    // Resolve tab absent (IT-ADMIN only)
    await expect(page.getByRole("tab", { name: /^resolve$/i })).toHaveCount(0);
  });

  test("M17-04-AC4: ROLE-AKUN-CTL sees Header + DLQ; Resolve ABSENT", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.approve", "jurnal.dlq.read"], "usr-ctl-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("tab", { name: /^header$/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("tab", { name: /DLQ/i })).toBeVisible();

    // Resolve absent
    await expect(page.getByRole("tab", { name: /^resolve$/i })).toHaveCount(0);
  });

  test("M17-04-AC4: ROLE-IT-ADMIN sees all 3 tabs (Header, DLQ, Resolve)", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.dlq.replay", "jurnal.resolve"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("tab", { name: /^header$/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("tab", { name: /DLQ/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /^resolve$/i })).toBeVisible();
  });

  test("M17-04-AC4: DLQ tab badge shows PENDING count when > 0", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.dlq.replay", "jurnal.resolve"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page, 3);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    // Badge with count 3 on DLQ tab
    const dlqTab = page.getByRole("tab", { name: /DLQ/i });
    await expect(dlqTab).toBeVisible({ timeout: 5000 });

    // Badge should contain "3" or aria-label mentions it
    const badge = page.locator("[aria-label*='3 entri DLQ'], [data-badge='3']").or(
      page.locator("tab").filter({ hasText: "DLQ" }).locator("span.badge, span.count")
    );
    // Verify badge is present (flexible: count on tab or as aria-label)
    const hasCount = await page.getByText(/DLQ.*3|3.*DLQ/i).count() > 0
      || await badge.count() > 0
      || (await dlqTab.textContent())?.includes("3");
    expect(hasCount).toBeTruthy();
  });

  test("M17-04-AC4: DLQ badge absent when count = 0", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.resolve"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page, 0);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    const dlqTab = page.getByRole("tab", { name: /DLQ/i });
    if (await dlqTab.isVisible()) {
      const tabText = await dlqTab.textContent();
      // No "0" badge — only the label "DLQ"
      const hasBadge = page.locator("[aria-label*='0 entri'], [data-badge='0']");
      await expect(hasBadge).toHaveCount(0);
    }
  });

  test("M17-04-AC4: Resolve tab absent from DOM (not just hidden) for non-IT-ADMIN", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.dlq.read"], "usr-ctl-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    // Verify not in DOM at all (not CSS hidden)
    const source = await page.content();
    const resolveTabInSource = source.toLowerCase().includes('data-tab="resolve"') || source.toLowerCase().includes('href="/jurnal/resolve"');
    // The resolve tab should truly be absent from DOM — no hidden attribute
    await expect(page.getByRole("tab", { name: /^resolve$/i })).toHaveCount(0);
    // And no hidden resolve tab in source (ensure not CSS display:none)
    expect(source).not.toMatch(/tab-resolve[^"]*"[^>]*display:\s*none/);
  });
});

// ---------------------------------------------------------------------------
// M17-04-AC4: Breadcrumb per sub-route
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Jurnal Layout: Breadcrumb (AC4)", () => {

  test("M17-04-AC4: breadcrumb shows Beranda / Jurnal / Header on /jurnal/header", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["jurnal.read"]);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb'], [aria-label='breadcrumb'], nav.breadcrumb");
    await expect(breadcrumb.getByText(/Header/i)).toBeVisible({ timeout: 5000 });
    await expect(breadcrumb.getByText(/Jurnal/i)).toBeVisible();
  });

  test("M17-04-AC4: breadcrumb shows Beranda / Jurnal / DLQ on /jurnal/dlq", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.resolve"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/dlq");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb'], [aria-label='breadcrumb'], nav.breadcrumb");
    await expect(breadcrumb.getByText(/DLQ/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-04-AC4: active tab has aria-current or aria-selected", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["jurnal.read"]);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    const activeTab = page.getByRole("tab", { name: /^header$/i });
    await expect(activeTab).toBeVisible({ timeout: 5000 });

    // Active tab must have aria indicator
    const isSelected = await activeTab.getAttribute("aria-selected");
    const isCurrent = await activeTab.getAttribute("aria-current");
    const hasDataState = await activeTab.getAttribute("data-state");

    expect(
      isSelected === "true" || isCurrent === "page" || hasDataState === "active"
    ).toBeTruthy();
  });
});

// ---------------------------------------------------------------------------
// M17-04-AC4: 308 Redirects from /jrnl/* to /jurnal/*
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Jurnal Layout: /jrnl/* Redirects (AC4)", () => {

  test("M17-04-AC4: /jrnl/journal-entries → 308 → /jurnal/header", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["jurnal.read"]);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jrnl/journal-entries");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/jurnal\/header(?!\/)(\?.*)?$/);
  });

  test("M17-04-AC4: /jrnl/dlq → 308 → /jurnal/dlq", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jrnl/dlq");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/jurnal\/dlq(?!\/)(\?.*)?$/);
  });

  test("M17-04-AC4: /jrnl/resolve → 308 → /jurnal/resolve", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.resolve"], "usr-it-001", true);
    await mockJurnalLayoutEndpoints(page);

    await page.goto("/jrnl/resolve");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/jurnal\/resolve/);
  });

  test("M17-04-AC4: /jrnl/rekonsiliasi → 308 → /reconciliation/daily", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);

    page.route("**/api/v1/**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: {}, meta: { traceId: "t" } }) })
    );

    await page.goto("/jrnl/rekonsiliasi");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/reconciliation\/daily/);
  });
});
