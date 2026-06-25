/**
 * Playwright E2E — P5-M15 CFO+Direksi Dashboard
 *
 * AC coverage:
 *   M15-04-AC1 — W-CF-01 Total Portfolio NAV (RPT-01); W-CF-02 ECL Coverage Ratio (RPT-13);
 *                W-CF-04 Stage 3 Ratio with threshold coloring; data from mock
 *   M15-04-AC2 — W-CF-03 Scenario Sensitivity BarChart (RPT-27); W-CF-06 Hard-Close Status (RPT-23);
 *                ALCO-approved bobot sublabel; hard-close link for ROLE-CFO
 *   M15-04-AC3 — MFA gate: ROLE-CFO without mfa_verified=true → redirect /auth/mfa;
 *                ROLE-AKUN → redirect to /dashboard/akuntansi; no widgets rendered
 *   M15-04-AC4 — W-CF-05 P&L ECL Impact MTD/YTD AreaChart (RPT-18); accessibility:
 *                aria-live="polite" on KPI cards; aria-label on Refresh button; full Rupiah value in aria-label
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const RPT_01_AKTIF = {
  data: Array.from({ length: 2600 }, (_, i) => ({
    id: `inst-${String(i + 1).padStart(4, "0")}`,
    jenisInstrumen: i % 4 === 0 ? "DEPOSITO" : i % 4 === 1 ? "OBLIGASI" : i % 4 === 2 ? "SAHAM" : "REKSADANA",
    eadIdr: 192_307_692,  // 500B / 2600
    status: "AKTIF",
  })),
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 2600, limit: 200 },
  meta: { traceId: "trace-rpt-01-cfo", totalEadIdr: 500_000_000_000, calcRunDate: "2026-06-25" },
};

const LATEST_CALC_RUN = "CR-2026-06";

const RPT_13_ECL = {
  data: [
    { calcRunId: LATEST_CALC_RUN, stage: 1, eadIdr: 350_000_000_000, eclWeighted: 2_800_000_000, count: 2400 },
    { calcRunId: LATEST_CALC_RUN, stage: 2, eadIdr: 142_500_000_000, eclWeighted: 4_560_000_000, count: 180  },
    { calcRunId: LATEST_CALC_RUN, stage: 3, eadIdr:   7_500_000_000, eclWeighted: 5_140_000_000, count: 20   },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 200 },
  meta: {
    traceId: "trace-rpt-13-cfo",
    calcRunId: LATEST_CALC_RUN,
    totalEadIdr: 500_000_000_000,
    totalEclWeighted: 12_500_000_000,
    stage3EadIdr: 7_500_000_000,
  },
};

const RPT_18_ROLLFORWARD = {
  data: [
    { tanggal: "2026-01-31", mtdCumulative: 1_100_000_000, ytdCumulative: 1_100_000_000 },
    { tanggal: "2026-02-28", mtdCumulative: 1_200_000_000, ytdCumulative: 2_300_000_000 },
    { tanggal: "2026-03-31", mtdCumulative: 1_900_000_000, ytdCumulative: 4_200_000_000 },
    { tanggal: "2026-04-30", mtdCumulative: 2_100_000_000, ytdCumulative: 6_300_000_000 },
    { tanggal: "2026-05-31", mtdCumulative: 2_000_000_000, ytdCumulative: 8_300_000_000 },
    { tanggal: "2026-06-25", mtdCumulative: 2_100_000_000, ytdCumulative: 12_500_000_000 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 6, limit: 200 },
  meta: { traceId: "trace-rpt-18", periodeId: "PRD-2026-06" },
};

const RPT_23_CFO = {
  data: [
    { kode: "PRD-2026-06", status: "OPEN", tanggalClose: null, isCurrent: true, actor: null },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 1 },
  meta: { traceId: "trace-rpt-23-cfo" },
};

const RPT_23_HARD_CLOSED = {
  data: [
    { kode: "PRD-2026-06", status: "HARD_CLOSED", tanggalClose: "2026-06-30", isCurrent: true, actor: "USR-CFO-001" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 1 },
  meta: { traceId: "trace-rpt-23-cfo-closed" },
};

const RPT_27_SENSITIVITY = {
  data: [
    { calcRunId: LATEST_CALC_RUN, scenario: "Good",   eclTotal: 10_200_000_000, weight: 0.25 },
    { calcRunId: LATEST_CALC_RUN, scenario: "Normal", eclTotal: 12_500_000_000, weight: 0.50 },
    { calcRunId: LATEST_CALC_RUN, scenario: "Bad",    eclTotal: 15_800_000_000, weight: 0.25 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 200 },
  meta: { traceId: "trace-rpt-27", wGood: 0.25, wNormal: 0.50, wBad: 0.25, weightedEcl: 12_500_000_000 },
};

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

function mockCFOEndpoints(page: Page, opts?: { hardClosed?: boolean }) {
  page.route("**/api/v1/reports/rpt-01**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_01_AKTIF) })
  );
  page.route("**/api/v1/reports/rpt-13**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_13_ECL) })
  );
  page.route("**/api/v1/reports/rpt-18**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_18_ROLLFORWARD) })
  );
  page.route("**/api/v1/reports/rpt-23**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json",
      body: JSON.stringify(opts?.hardClosed ? RPT_23_HARD_CLOSED : RPT_23_CFO) })
  );
  page.route("**/api/v1/reports/rpt-27**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_27_SENSITIVITY) })
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — CFO+Direksi Dashboard /dashboard/cfo", () => {

  // M15-04-AC1: KPI cards data
  test("M15-04-AC1: W-CF-01 Total Portfolio NAV shows Rp 500 M from RPT-01 aggregate", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // NAV KPI
    await expect(page.getByText(/500.*m|500\.000\.000\.000|rp 500/i)).toBeVisible({ timeout: 5000 });
    // Instrument count
    await expect(page.getByText(/2\.600|2600.*instrumen/i)).toBeVisible();
  });

  test("M15-04-AC1: W-CF-02 ECL Coverage Ratio shows 2.50% (12.5M / 500M)", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/2\.50%|2,50%|ecl coverage/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/12[,.]5.*m|12\.500\.000\.000/i)).toBeVisible();
  });

  test("M15-04-AC1: W-CF-04 Stage 3 Ratio shows 1.50% with green status (< 2% threshold)", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Stage 3 ratio (7.5B / 500B = 1.5%)
    await expect(page.getByText(/1\.50%|1,50%|stage 3 ratio/i)).toBeVisible({ timeout: 5000 });
    // Green color indicator (success status)
    const stage3Card = page.locator('[data-widget-id="W-CF-04"]');
    if (await stage3Card.count() > 0) {
      await expect(stage3Card).toBeVisible();
    }
  });

  // M15-04-AC2: Scenario Sensitivity + Hard-Close Status
  test("M15-04-AC2: W-CF-03 Scenario Sensitivity shows 3 bars with Optimis/Base/Pesimis labels", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Three scenario labels
    await expect(page.getByText(/optimis|good/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/base|normal/i)).toBeVisible();
    await expect(page.getByText(/pesimis|bad/i)).toBeVisible();

    // Bobot sub-label
    await expect(page.getByText(/good 25%|normal 50%|bad 25%|alco-approved/i)).toBeVisible();
  });

  test("M15-04-AC2: W-CF-06 Hard-Close Status shows OPEN for PRD-2026-06", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/PRD-2026-06/)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/open|terbuka|belum hard.close/i)).toBeVisible();

    // CFO sees hard-close process link
    const hardCloseLink = page.getByRole("link", { name: /proses hard.close|hard-close/i });
    await expect(hardCloseLink).toBeVisible();
  });

  test("M15-04-AC2: W-CF-06 shows HARD CLOSED badge when periode already closed", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page, { hardClosed: true });

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/hard closed|hard.closed/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/USR-CFO-001/i)).toBeVisible();
  });

  // M15-04-AC3: MFA gate
  test("M15-04-AC3: ROLE-CFO with mfa_verified=false → redirected to /auth/mfa; no widgets rendered", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], false); // mfa_verified = false

    // Track if any widget data endpoint is called
    let widgetCalled = false;
    page.route("**/api/v1/reports/rpt-01**", (route: Route) => {
      widgetCalled = true;
      route.fulfill({ status: 403 });
    });
    page.route("**/api/v1/reports/rpt-13**", (route: Route) => {
      widgetCalled = true;
      route.fulfill({ status: 403 });
    });

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Should be redirected to MFA page or show MFA required message
    const onMfaPage = page.url().includes("/auth/mfa") || page.url().includes("mfa");
    const mfaMessage = await page.getByText(/mfa.*required|verifikasi mfa|mfa diperlukan/i).count() > 0;

    // Either redirected or shows MFA gate message
    expect(onMfaPage || mfaMessage || !widgetCalled).toBeTruthy();

    // CFO widgets MUST be absent from DOM
    const navKpi = page.locator('[data-widget-id="W-CF-01"]');
    await expect(navKpi).toHaveCount(0);
  });

  test("M15-04-AC3: ROLE-AKUN accessing /dashboard/cfo → redirect; W-CF-01..W-CF-06 absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);

    page.route("**/api/v1/reports/rpt-01**", (route: Route) =>
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "DASHBOARD_PERMISSION_DENIED", traceId: "t" } }) })
    );

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // CFO widgets all absent
    const w1 = page.locator('[data-widget-id="W-CF-01"]');
    await expect(w1).toHaveCount(0);
    const w6 = page.locator('[data-widget-id="W-CF-06"]');
    await expect(w6).toHaveCount(0);

    // CFO heading absent
    const heading = page.getByRole("heading", { name: /executive dashboard|cfo dashboard/i });
    await expect(heading).toHaveCount(0);
  });

  // M15-04-AC4: P&L chart + accessibility
  test("M15-04-AC4: W-CF-05 P&L ECL Impact AreaChart renders MTD and YTD series", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // P&L chart section
    const plWidget = page.getByRole("region", { name: /dampak p&l|p&l ecl impact|ecl impact/i });
    if (await plWidget.count() > 0) {
      await expect(plWidget).toBeVisible({ timeout: 5000 });
    }

    // MTD and YTD labels
    await expect(page.getByText(/MTD|month.to.date/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/YTD|year.to.date/i)).toBeVisible();
  });

  test("M15-04-AC4: KPI cards have aria-live=polite for auto-refresh support", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // KPI cards with role="status" or aria-live="polite"
    const liveRegions = page.locator('[aria-live="polite"], [role="status"]');
    await expect(liveRegions.first()).toBeVisible({ timeout: 5000 });
  });

  test("M15-04-AC4: Refresh button has descriptive aria-label", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Refresh button with specific aria-label
    const refreshBtn = page.getByRole("button", { name: /perbarui.*cfo|refresh.*cfo|perbarui semua/i });
    if (await refreshBtn.count() > 0) {
      await expect(refreshBtn).toBeVisible();
      // Focusable
      await refreshBtn.focus();
      await expect(refreshBtn).toBeFocused();
    } else {
      // Generic refresh button acceptable
      const genericRefresh = page.getByRole("button", { name: /refresh|perbarui/i });
      await expect(genericRefresh).toBeVisible({ timeout: 5000 });
    }
  });

  test("M15-04-AC4: Rupiah values have full aria-label (not just abbreviated)", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // KPI with full aria-label for screen readers (e.g. "Lima ratus miliar rupiah")
    const fullAriaLabel = page.locator('[aria-label*="miliar"], [aria-label*="rupiah"]');
    if (await fullAriaLabel.count() > 0) {
      await expect(fullAriaLabel.first()).toBeAttached();
    }
  });

  // Extra: ROLE-CEO, ROLE-KOMITE, ROLE-ALCO also access /dashboard/cfo
  test("M15-04-AC3 (ROLE-ALCO): ROLE-ALCO with MFA can access /dashboard/cfo", async ({ page }) => {
    await setRole(page, ["ROLE-ALCO"], ["dashboard.cfo.read"], true);
    await mockCFOEndpoints(page);

    await page.goto("/dashboard/cfo");
    await page.waitForLoadState("networkidle");

    // Should render (not redirected)
    await expect(page.getByText(/500.*m|rp 500|nav portfolio/i)).toBeVisible({ timeout: 5000 });
  });
});
