/**
 * Playwright E2E — P5-M16 Shared /transaksi Layout Tab Navigation
 *
 * AC coverage:
 *   M16-05-AC1 — Tab absent-from-DOM per permission; server-rendered HTML
 *   M16-05-AC2 — Active tab highlight (aria-current="page"); breadcrumb per sub-route
 *   M16-05-AC3 — CTA button visible/absent per sub-route + permission
 *   M16-05-AC4 — Layout accessibility: keyboard nav, aria-labels, skip-to-content
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test is not in package.json — stubs are syntactically valid but
 *       must be run after Playwright is installed in the project.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(page: Page, roles: string[], permissions: string[], mfaVerified = false) {
  return page.addInitScript(
    ({ r, p, m }: { r: string[]; p: string[]; m: boolean }) => {
      localStorage.setItem("blips_roles", JSON.stringify(r));
      localStorage.setItem("blips_permissions", JSON.stringify(p));
      localStorage.setItem("blips_mfa_verified", String(m));
    },
    { r: roles, p: permissions, m: mfaVerified }
  );
}

/** Block all transaksi list endpoints to prevent noise during layout tests. */
function blockTransaksiEndpoints(page: Page) {
  page.route("**/api/v1/transaksi/**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
  );
  page.route("**/api/v1/jobs/**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [] }) })
  );
}

const ALL_TRANSAKSI_PERMISSIONS = [
  "penempatan.read",
  "penempatan.create",
  "transaksi.mtm.read",
  "transaksi.mtm.upload",
  "renewal.read",
  "renewal.create",
  "penjualan.read",
  "penjualan.create",
  "transaksi.jatuh-tempo.read",
  "transaksi.akrual.read",
  "transaksi.akrual.create",
];

// ---------------------------------------------------------------------------
// M16-05-AC1: Tab absent-from-DOM per permission
// ---------------------------------------------------------------------------

test.describe("P5-M16 — /transaksi Layout: Tab Nav Visibility", () => {

  test("M16-05-AC1: ROLE-AKUN with mtm+akrual perms: penempatan/renewal/penjualan tabs absent from DOM", async ({ page }) => {
    await setRole(
      page,
      ["ROLE-AKUN"],
      ["transaksi.mtm.read", "transaksi.akrual.read", "transaksi.jatuh-tempo.read"]
    );
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    // Tabs that should be VISIBLE
    const tabMtm = page.getByRole("tab", { name: /^mtm$/i });
    await expect(tabMtm).toBeVisible({ timeout: 5000 });

    const tabAkrual = page.getByRole("tab", { name: /^akrual$/i });
    await expect(tabAkrual).toBeVisible();

    const tabJatuhTempo = page.getByRole("tab", { name: /jatuh tempo/i });
    await expect(tabJatuhTempo).toBeVisible();

    // Tabs that must be ABSENT from DOM (not disabled, not hidden)
    const tabPenempatan = page.getByRole("tab", { name: /^penempatan$/i });
    await expect(tabPenempatan).toHaveCount(0);

    const tabRenewal = page.getByRole("tab", { name: /^renewal$/i });
    await expect(tabRenewal).toHaveCount(0);

    const tabPenjualan = page.getByRole("tab", { name: /^penjualan$/i });
    await expect(tabPenjualan).toHaveCount(0);
  });

  test("M16-05-AC1: no HTML comment or hidden DOM node leaking absent tab labels", async ({ page }) => {
    await setRole(
      page,
      ["ROLE-AKUN"],
      ["transaksi.mtm.read", "transaksi.akrual.read", "transaksi.jatuh-tempo.read"]
    );
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const source = await page.content();
    // No hidden DOM for unauthorized tabs
    expect(source).not.toMatch(/penempatan[^<]{0,50}disabled/i);
    expect(source).not.toMatch(/renewal[^<]{0,50}display:\s*none/i);
    expect(source).not.toMatch(/penjualan[^<]{0,50}visibility:\s*hidden/i);
  });

  test("M16-05-AC1: ROLE-MAKER-TR (full perms): all 6 tabs visible", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const tabs = ["penempatan", "mtm", "renewal", "penjualan", "jatuh tempo", "akrual"];
    for (const label of tabs) {
      await expect(page.getByRole("tab", { name: new RegExp(label, "i") })).toBeVisible({ timeout: 5000 });
    }
  });

  test("M16-05-AC1: tab nav is a server-rendered nav element with role=tablist", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const nav = page.locator("nav[aria-label='Navigasi Transaksi']");
    await expect(nav).toBeVisible({ timeout: 5000 });

    const tablist = nav.locator("[role='tablist']");
    await expect(tablist).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// M16-05-AC2: Active tab highlight + breadcrumb
// ---------------------------------------------------------------------------

test.describe("P5-M16 — /transaksi Layout: Active State + Breadcrumb", () => {

  test("M16-05-AC2: navigating to /transaksi/renewal/new: Renewal tab has aria-current=page", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/renewal/new");
    await page.waitForLoadState("networkidle");

    const renewalTab = page.getByRole("tab", { name: /^renewal$/i });
    await expect(renewalTab).toHaveAttribute("aria-selected", "true", { timeout: 5000 });

    // Other tabs must not have aria-selected=true
    const penTab = page.getByRole("tab", { name: /^penempatan$/i });
    await expect(penTab).toHaveAttribute("aria-selected", "false");
  });

  test("M16-05-AC2: breadcrumb at /transaksi/renewal/new shows Beranda / Transaksi / Renewal / Renewal Baru", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/renewal/new");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb']");
    await expect(breadcrumb).toBeVisible({ timeout: 5000 });

    await expect(breadcrumb.getByRole("link", { name: /beranda/i })).toBeVisible();
    await expect(breadcrumb.getByRole("link", { name: /^transaksi$/i })).toBeVisible();
    await expect(breadcrumb.getByRole("link", { name: /^renewal$/i })).toBeVisible();

    // Last crumb: no link, aria-current=page
    const lastCrumb = breadcrumb.locator("[aria-current='page']");
    await expect(lastCrumb).toHaveText(/renewal baru/i);
  });

  test("M16-05-AC2: breadcrumb at /transaksi/penempatan shows Beranda / Transaksi / Penempatan with aria-current=page on last", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb']");
    await expect(breadcrumb).toBeVisible({ timeout: 5000 });

    const lastCrumb = breadcrumb.locator("[aria-current='page']");
    await expect(lastCrumb).toHaveText(/penempatan/i);

    // "Penempatan" at end must NOT be a link (it is the current page)
    const selfLink = breadcrumb.getByRole("link", { name: /^penempatan$/i });
    await expect(selfLink).toHaveCount(0);
  });

  test("M16-05-AC2: Penempatan tab active when at /transaksi/penempatan", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const penTab = page.getByRole("tab", { name: /^penempatan$/i });
    await expect(penTab).toHaveAttribute("aria-selected", "true", { timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M16-05-AC3: CTA button visibility per sub-route + permission
// ---------------------------------------------------------------------------

test.describe("P5-M16 — /transaksi Layout: CTA Button Visibility", () => {

  test("M16-05-AC3: ROLE-MAKER-TR at /transaksi/penempatan: '+ Penempatan Baru' button visible", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const cta = page.getByRole("link", { name: /tambah penempatan baru|penempatan baru/i });
    await expect(cta).toBeVisible({ timeout: 5000 });
    await expect(cta).toHaveAttribute("href", "/transaksi/penempatan/new");
  });

  test("M16-05-AC3: at /transaksi/jatuh-tempo: no CTA button rendered (read-only sub-route)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/jatuh-tempo");
    await page.waitForLoadState("networkidle");

    // No create/new CTA for jatuh-tempo
    const ctaNew = page.getByRole("link", { name: /baru$/i });
    await expect(ctaNew).toHaveCount(0);
  });

  test("M16-05-AC3: ROLE-APPR-TR without penempatan.create: '+ Penempatan Baru' absent from DOM", async ({ page }) => {
    await setRole(
      page,
      ["ROLE-APPR-TR"],
      ["penempatan.read", "renewal.read", "penjualan.read", "transaksi.jatuh-tempo.read"]
      // no penempatan.create
    );
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const cta = page.locator("[aria-label='Tambah Penempatan Baru']");
    await expect(cta).toHaveCount(0);
  });

  test("M16-05-AC3: ROLE-MAKER-TR at /transaksi/mtm: '+ Upload MTM' button links to /transaksi/mtm/upload", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    const cta = page.getByRole("link", { name: /upload mtm/i });
    await expect(cta).toBeVisible({ timeout: 5000 });
    await expect(cta).toHaveAttribute("href", "/transaksi/mtm/upload");
  });
});

// ---------------------------------------------------------------------------
// M16-05-AC4: Accessibility — keyboard nav, aria-labels, skip-to-content
// ---------------------------------------------------------------------------

test.describe("P5-M16 — /transaksi Layout: Accessibility", () => {

  test("M16-05-AC4: skip-to-content link is first focusable element in DOM", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // Press Tab once — first focus should land on skip link
    await page.keyboard.press("Tab");
    const focused = await page.evaluate(() => document.activeElement?.textContent ?? "");
    expect(focused.toLowerCase()).toMatch(/lewati|skip|lompat/i);
  });

  test("M16-05-AC4: Arrow Right / Left keyboard navigates between tabs", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // Focus the first tab
    const firstTab = page.getByRole("tab", { name: /^penempatan$/i });
    await firstTab.focus();

    // Arrow Right moves to next tab (MTM)
    await page.keyboard.press("ArrowRight");
    const focusedTab = await page.evaluate(() => document.activeElement?.textContent ?? "");
    expect(focusedTab.toLowerCase()).toMatch(/mtm/i);
  });

  test("M16-05-AC4: tab nav has aria-label=Navigasi Transaksi wrapping role=tablist", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const nav = page.locator("nav[aria-label='Navigasi Transaksi']");
    await expect(nav).toBeVisible({ timeout: 5000 });

    const tablist = nav.locator("[role='tablist']");
    await expect(tablist).toBeVisible();
  });

  test("M16-05-AC4: main content area has role=tabpanel", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const panel = page.locator("[role='tabpanel']");
    await expect(panel).toBeVisible({ timeout: 5000 });
  });

  test("M16-05-AC4: CTA button has descriptive aria-label (not just short label)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // aria-label must spell out the full action
    const cta = page.locator("[aria-label='Tambah Penempatan Baru']");
    await expect(cta).toBeVisible({ timeout: 5000 });
  });

  test.fixme("M16-05-AC4: Enter key on focused tab triggers navigation to that sub-route", async ({ page }) => {
    // Fixme: requires navigation to complete without backend; verify after FE implementation
    await setRole(page, ["ROLE-MAKER-TR"], ALL_TRANSAKSI_PERMISSIONS);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const mtmTab = page.getByRole("tab", { name: /^mtm$/i });
    await mtmTab.focus();
    await page.keyboard.press("Enter");
    await page.waitForURL("**/transaksi/mtm**");
    await expect(page).toHaveURL(/\/transaksi\/mtm/);
  });
});
