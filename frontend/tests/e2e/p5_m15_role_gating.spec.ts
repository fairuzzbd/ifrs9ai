/**
 * Playwright E2E — P5-M15 Role Gating (Absent-from-DOM Contract)
 *
 * AC coverage (cross-cutting — all 5 stories, AC2/3 from each):
 *   Verifies that unauthorized users get a Next.js 404/403 page AND that no widget HTML
 *   appears in the DOM source — not merely hidden via CSS (visibility:hidden / display:none).
 *
 *   Tested scenarios:
 *   - ROLE-RISK → /dashboard/treasury: absent-from-DOM (M15-01-AC2)
 *   - ROLE-AKUN → /dashboard/risk:    absent-from-DOM (M15-02-AC3)
 *   - ROLE-RISK → /dashboard/akuntansi: absent-from-DOM (M15-03-AC3)
 *   - ROLE-AKUN → /dashboard/cfo:     absent-from-DOM (M15-04-AC3)
 *   - ROLE-AKUN → /dashboard/audit:   absent-from-DOM (M15-05-AC4)
 *   - ROLE-CFO (mfa=false) → /dashboard/cfo: absent-from-DOM (M15-04-AC3)
 *
 * Absent-from-DOM is verified by asserting:
 *   1. Widget region locators return count === 0
 *   2. Data-widget-id attributes from the forbidden dashboard are NOT present in page source
 *   3. Hidden-via-CSS check: element must not exist at all (not just invisible)
 *
 * Pattern: page.route() to block report endpoints; setRole() via addInitScript.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(page: Page, roles: string[], permissions: string[], mfaVerified = true) {
  return page.addInitScript(
    ({ r, p, m }: { r: string[]; p: string[]; m: boolean }) => {
      localStorage.setItem("blips_roles", JSON.stringify(r));
      localStorage.setItem("blips_permissions", JSON.stringify(p));
      localStorage.setItem("blips_mfa_verified", String(m));
    },
    { r: roles, p: permissions, m: mfaVerified }
  );
}

function blockAllReportEndpoints(page: Page) {
  page.route("**/api/v1/reports/**", (route: Route) =>
    route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "DASHBOARD_PERMISSION_DENIED", traceId: "t" } }) })
  );
  page.route("**/api/v1/jobs**", (route: Route) =>
    route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "FORBIDDEN", traceId: "t" } }) })
  );
}

/** Assert that a set of data-widget-id attributes are absent from DOM (not just hidden). */
async function assertWidgetsAbsentFromDOM(page: Page, widgetIds: string[]) {
  for (const wid of widgetIds) {
    const el = page.locator(`[data-widget-id="${wid}"]`);
    await expect(el).toHaveCount(0, { timeout: 3000 });
  }
}

/** Assert there are no hidden (display:none / visibility:hidden) widgets either. */
async function assertNoHiddenWidgets(page: Page, widgetIds: string[]) {
  for (const wid of widgetIds) {
    const hiddenEl = page.locator(`[data-widget-id="${wid}"][style*="display: none"]`)
      .or(page.locator(`[data-widget-id="${wid}"][style*="visibility: hidden"]`))
      .or(page.locator(`[data-widget-id="${wid}"][hidden]`));
    await expect(hiddenEl).toHaveCount(0, { timeout: 3000 });
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — Role Gating Absent-from-DOM Contract", () => {

  // M15-01-AC2: ROLE-RISK → /dashboard/treasury
  test("M15-01-AC2: ROLE-RISK → /dashboard/treasury: Treasury widgets absent from DOM (not hidden)", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/treasury");
    await page.waitForLoadState("networkidle");

    // Treasury dashboard heading absent
    const heading = page.getByRole("heading", { name: /treasury dashboard/i });
    await expect(heading).toHaveCount(0);

    // Widget data-widget-ids absent
    await assertWidgetsAbsentFromDOM(page, ["W-TR-01", "W-TR-02", "W-TR-03", "W-TR-04", "W-TR-05"]);
    await assertNoHiddenWidgets(page, ["W-TR-01", "W-TR-02", "W-TR-03", "W-TR-04", "W-TR-05"]);

    // Page shows 404 or redirect content
    const is404 = await page.getByText(/404|tidak ditemukan|not found|akses ditolak|dilarang/i).count() > 0;
    const isRedirected = !page.url().includes("/dashboard/treasury");
    expect(is404 || isRedirected).toBeTruthy();
  });

  // M15-02-AC3: ROLE-AKUN → /dashboard/risk
  test("M15-02-AC3: ROLE-AKUN → /dashboard/risk: Risk widgets absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    const heading = page.getByRole("heading", { name: /risk dashboard/i });
    await expect(heading).toHaveCount(0);

    await assertWidgetsAbsentFromDOM(page, ["W-RK-01", "W-RK-02", "W-RK-03", "W-RK-04", "W-RK-05"]);
    await assertNoHiddenWidgets(page, ["W-RK-01", "W-RK-02", "W-RK-03", "W-RK-04", "W-RK-05"]);

    const isRedirected = !page.url().includes("/dashboard/risk");
    expect(isRedirected).toBeTruthy();
  });

  // M15-03-AC3: ROLE-RISK → /dashboard/akuntansi
  test("M15-03-AC3: ROLE-RISK → /dashboard/akuntansi: Akuntansi widgets absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    const heading = page.getByRole("heading", { name: /akuntansi dashboard/i });
    await expect(heading).toHaveCount(0);

    await assertWidgetsAbsentFromDOM(page, ["W-AK-01", "W-AK-02", "W-AK-03", "W-AK-04", "W-AK-05"]);
    await assertNoHiddenWidgets(page, ["W-AK-01", "W-AK-02", "W-AK-03", "W-AK-04", "W-AK-05"]);

    const isRedirected = !page.url().includes("/dashboard/akuntansi");
    expect(isRedirected).toBeTruthy();
  });

  // M15-04-AC3: ROLE-AKUN → /dashboard/cfo (wrong role)
  test("M15-04-AC3: ROLE-AKUN → /dashboard/cfo: CFO widgets absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    const heading = page.getByRole("heading", { name: /executive dashboard|cfo/i });
    await expect(heading).toHaveCount(0);

    await assertWidgetsAbsentFromDOM(page, ["W-CF-01", "W-CF-02", "W-CF-03", "W-CF-04", "W-CF-05", "W-CF-06"]);
    await assertNoHiddenWidgets(page, ["W-CF-01", "W-CF-02", "W-CF-03", "W-CF-04", "W-CF-05", "W-CF-06"]);

    const isRedirected = !page.url().includes("/dashboard/cfo");
    expect(isRedirected).toBeTruthy();
  });

  // M15-04-AC3: ROLE-CFO with mfa_verified=false → /dashboard/cfo MFA gate
  test("M15-04-AC3: ROLE-CFO mfa_verified=false → CFO dashboard widgets absent; redirect to /auth/mfa", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], false); // mfa NOT verified
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // CFO widgets must not render
    await assertWidgetsAbsentFromDOM(page, ["W-CF-01", "W-CF-02", "W-CF-03", "W-CF-04", "W-CF-05", "W-CF-06"]);
    await assertNoHiddenWidgets(page, ["W-CF-01", "W-CF-02", "W-CF-03", "W-CF-04", "W-CF-05", "W-CF-06"]);

    // Either redirected to MFA page or shows MFA prompt
    const onMfaRoute = page.url().includes("/auth/mfa") || page.url().includes("mfa");
    const showsMfaText = await page.getByText(/mfa.*required|verifikasi.*mfa|mfa diperlukan/i).count() > 0;
    expect(onMfaRoute || showsMfaText).toBeTruthy();
  });

  // M15-05-AC4: ROLE-AKUN → /dashboard/audit
  test("M15-05-AC4: ROLE-AKUN → /dashboard/audit: Audit widgets absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    const heading = page.getByRole("heading", { name: /auditor dashboard/i });
    await expect(heading).toHaveCount(0);

    await assertWidgetsAbsentFromDOM(page, ["W-AU-01", "W-AU-02", "W-AU-03", "W-AU-04"]);
    await assertNoHiddenWidgets(page, ["W-AU-01", "W-AU-02", "W-AU-03", "W-AU-04"]);

    const isRedirected = !page.url().includes("/dashboard/audit");
    expect(isRedirected).toBeTruthy();
  });

  // Cross-check: no hidden widget data attributes that could leak info
  test("no forbidden dashboard widget HTML in source when wrong role", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Check page HTML source does not contain CFO widget IDs
    const source = await page.content();
    expect(source).not.toContain('data-widget-id="W-CF-01"');
    expect(source).not.toContain('data-widget-id="W-CF-02"');
    expect(source).not.toContain('data-widget-id="W-CF-06"');
  });

  test("no forbidden dashboard widget HTML in source when MFA not verified", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], false);
    await blockAllReportEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    const source = await page.content();
    expect(source).not.toContain('data-widget-id="W-CF-01"');
    expect(source).not.toContain("Total Portfolio NAV");
  });
});
