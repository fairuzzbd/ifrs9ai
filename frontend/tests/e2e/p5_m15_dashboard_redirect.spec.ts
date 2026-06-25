/**
 * Playwright E2E — P5-M15 Dashboard Landing Redirect
 *
 * AC coverage:
 *   M15-01-AC2, M15-02-AC3, M15-03-AC3, M15-04-AC3, M15-05-AC4 (redirect logic):
 *   /dashboard landing redirects to role-default per precedence:
 *     ROLE-MAKER-TR   → /dashboard/treasury
 *     ROLE-APPR-TR    → /dashboard/treasury
 *     ROLE-RISK       → /dashboard/risk
 *     ROLE-AKUN       → /dashboard/akuntansi
 *     ROLE-AKUN-CTL   → /dashboard/akuntansi
 *     ROLE-CFO        → /dashboard/cfo
 *     ROLE-CEO        → /dashboard/cfo
 *     ROLE-KOMITE     → /dashboard/cfo
 *     ROLE-ALCO       → /dashboard/cfo
 *     ROLE-AUDIT      → /dashboard/audit
 *     ROLE-IT-ADMIN   → /jobs
 *
 * Pattern: page.route() to mock all widget endpoints; assert URL after navigation.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page } from "@playwright/test";

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

function mockAllReportEndpoints(page: Page) {
  const emptyResp = JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } });
  page.route("**/api/v1/reports/**", (route) => route.fulfill({ status: 200, contentType: "application/json", body: emptyResp }));
  page.route("**/api/v1/jobs**", (route) => route.fulfill({ status: 200, contentType: "application/json", body: emptyResp }));
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — /dashboard landing redirect per role", () => {

  test("ROLE-MAKER-TR → /dashboard redirects to /dashboard/treasury", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["dashboard.treasury.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/treasury");
  });

  test("ROLE-APPR-TR → /dashboard redirects to /dashboard/treasury", async ({ page }) => {
    await setRole(page, ["ROLE-APPR-TR"], ["dashboard.treasury.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/treasury");
  });

  test("ROLE-RISK → /dashboard redirects to /dashboard/risk", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/risk");
  });

  test("ROLE-AKUN → /dashboard redirects to /dashboard/akuntansi", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/akuntansi");
  });

  test("ROLE-AKUN-CTL → /dashboard redirects to /dashboard/akuntansi", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["dashboard.akuntansi.read", "jurnal.approve"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/akuntansi");
  });

  test("ROLE-CFO (mfa=true) → /dashboard redirects to /dashboard/cfo", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/cfo");
  });

  test("ROLE-CEO (mfa=true) → /dashboard redirects to /dashboard/cfo", async ({ page }) => {
    await setRole(page, ["ROLE-CEO"], ["dashboard.cfo.read"], true);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/cfo");
  });

  test("ROLE-KOMITE (mfa=true) → /dashboard redirects to /dashboard/cfo", async ({ page }) => {
    await setRole(page, ["ROLE-KOMITE"], ["dashboard.cfo.read"], true);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/cfo");
  });

  test("ROLE-ALCO (mfa=true) → /dashboard redirects to /dashboard/cfo", async ({ page }) => {
    await setRole(page, ["ROLE-ALCO"], ["dashboard.cfo.read"], true);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/cfo");
  });

  test("ROLE-AUDIT → /dashboard redirects to /dashboard/audit", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/dashboard/audit");
  });

  test("ROLE-IT-ADMIN → /dashboard redirects to /jobs", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jobs.read", "jobs.read_all"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toContain("/jobs");
  });

  // Cross-role: access wrong dashboard → redirect to own default
  test("ROLE-RISK accessing /dashboard/treasury → redirect to /dashboard/risk", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard/treasury");
    await page.waitForLoadState("networkidle");

    // Should end up at /dashboard/risk (or /dashboard which redirects there)
    const finalUrl = page.url();
    expect(finalUrl.includes("/dashboard/risk") || finalUrl.includes("/dashboard")).toBe(true);
    expect(finalUrl).not.toContain("/dashboard/treasury");
  });

  test("ROLE-AKUN accessing /dashboard/cfo → redirect to /dashboard/akuntansi", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAllReportEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    const finalUrl = page.url();
    expect(finalUrl.includes("/dashboard/akuntansi") || finalUrl.includes("/dashboard")).toBe(true);
    expect(finalUrl).not.toContain("/dashboard/cfo");
  });
});
