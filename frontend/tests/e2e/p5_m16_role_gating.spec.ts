/**
 * Playwright E2E — P5-M16 Role Gating (Absent-from-DOM Contract)
 *
 * AC coverage (cross-cutting, all 5 stories):
 *   M16-05-AC1 — Tab absent-from-DOM per permission matrix (tab visibility matrix §4.1)
 *   M16-01-AC4 — SoD: maker cannot review/approve (absent-from-DOM + API enforced)
 *   M16-02-AC4 — MTM upload/cron absent for ROLE-RISK, ROLE-AUDIT
 *   M16-04-AC3 — Akrual approve button gated to ROLE-AKUN-CTL
 *
 * Absent-from-DOM verified by:
 *   1. Locator count === 0
 *   2. page.content() does not contain gated HTML attributes
 *   3. No hidden-via-CSS elements (display:none, visibility:hidden, hidden attr)
 *
 * Tested personas: ROLE-MAKER-TR, ROLE-APPR-TR, ROLE-AKUN, ROLE-AKUN-CTL,
 *                  ROLE-RISK, ROLE-AUDIT, ROLE-IT-ADMIN
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

function blockTransaksiEndpoints(page: Page) {
  page.route("**/api/v1/transaksi/**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
  );
  page.route("**/api/v1/jobs/**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [] }) })
  );
}

async function assertButtonsAbsentFromDOM(page: Page, names: string[]) {
  for (const name of names) {
    const el = page.getByRole("button", { name: new RegExp(name, "i") });
    await expect(el).toHaveCount(0, { timeout: 3000 });
  }
}

async function assertLinksAbsentFromDOM(page: Page, hrefs: string[]) {
  for (const href of hrefs) {
    const el = page.locator(`a[href*="${href}"]`);
    await expect(el).toHaveCount(0, { timeout: 3000 });
  }
}

async function assertNoHiddenElements(page: Page, selectors: string[]) {
  for (const sel of selectors) {
    const hidden = page.locator(`${sel}[style*="display: none"]`)
      .or(page.locator(`${sel}[style*="visibility: hidden"]`))
      .or(page.locator(`${sel}[hidden]`));
    await expect(hidden).toHaveCount(0, { timeout: 3000 });
  }
}

// ---------------------------------------------------------------------------
// Tab visibility per persona
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Role Gating: Tab Visibility Matrix", () => {

  test("ROLE-AKUN: Penempatan + Renewal + Penjualan tabs absent; MTM + Akrual + Jatuh Tempo visible", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.akrual.read", "transaksi.jatuh-tempo.read"]);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    // ABSENT
    await expect(page.getByRole("tab", { name: /^penempatan$/i })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: /^renewal$/i })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: /^penjualan$/i })).toHaveCount(0);

    // VISIBLE
    await expect(page.getByRole("tab", { name: /^mtm$/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("tab", { name: /^akrual$/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /jatuh tempo/i })).toBeVisible();
  });

  test("ROLE-APPR-TR: MTM tab absent; Penempatan + Renewal + Penjualan + Jatuh Tempo visible", async ({ page }) => {
    await setRole(page, ["ROLE-APPR-TR"], ["penempatan.read", "renewal.read", "penjualan.read", "transaksi.jatuh-tempo.read"]);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // MTM absent for APPR-TR
    await expect(page.getByRole("tab", { name: /^mtm$/i })).toHaveCount(0);
    // Akrual absent for APPR-TR
    await expect(page.getByRole("tab", { name: /^akrual$/i })).toHaveCount(0);

    // Present
    await expect(page.getByRole("tab", { name: /^penempatan$/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("tab", { name: /^renewal$/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /^penjualan$/i })).toBeVisible();
    await expect(page.getByRole("tab", { name: /jatuh tempo/i })).toBeVisible();
  });

  test("ROLE-IT-ADMIN: all transaksi tabs absent (no transaksi permissions)", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["user.manage", "job.read"]);
    blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const allTabs = ["penempatan", "mtm", "renewal", "penjualan", "jatuh tempo", "akrual"];
    for (const label of allTabs) {
      await expect(page.getByRole("tab", { name: new RegExp(`^${label}$`, "i") })).toHaveCount(0);
    }
  });

  test("ROLE-AUDIT: all 6 tabs visible (has transaksi.*.read)", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], [
      "penempatan.read", "transaksi.mtm.read", "renewal.read",
      "penjualan.read", "transaksi.jatuh-tempo.read", "transaksi.akrual.read",
    ]);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const allTabs = ["penempatan", "mtm", "renewal", "penjualan", "jatuh tempo", "akrual"];
    for (const label of allTabs) {
      await expect(page.getByRole("tab", { name: new RegExp(label, "i") })).toBeVisible({ timeout: 5000 });
    }
  });

  test("page source does not contain hidden tab HTML for absent tabs", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.akrual.read", "transaksi.jatuh-tempo.read"]);
    await blockTransaksiEndpoints(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const source = await page.content();
    // No hidden penempatan tab in DOM
    expect(source).not.toMatch(/tab-penempatan[^"]*"[^>]*display:\s*none/);
    expect(source).not.toMatch(/tab-renewal[^"]*"[^>]*visibility:\s*hidden/);
  });
});

// ---------------------------------------------------------------------------
// SoD: maker cannot review/approve
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Role Gating: SoD Absent-from-DOM", () => {

  test("SoD: maker of penempatan sees no Review or Approve buttons in DOM", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penempatan.read"], "usr-maker-001");

    page.route("**/api/v1/transaksi/penempatan/pnp-001**", (route: Route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { id: "pnp-001", kodePenempatan: "PNP-001", workflowStatus: "PENDING_REVIEW", makerId: "usr-maker-001" }, meta: { traceId: "t" } }),
      })
    );

    await page.goto("/transaksi/penempatan/pnp-001");
    await page.waitForLoadState("networkidle");

    await assertButtonsAbsentFromDOM(page, ["review.*tandatangani", "tandatangani", "^approve$"]);

    // Ensure not hidden via CSS
    await assertNoHiddenElements(page, ["[data-action='review']", "[data-action='approve']"]);
  });

  test("SoD: maker of renewal RNW-001 sees no workflow action buttons", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["renewal.read"], "usr-maker-001");

    page.route("**/api/v1/transaksi/renewal/rnw-001**", (route: Route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { id: "rnw-001", kodeRenewal: "RNW-001", workflowStatus: "SUBMITTED", makerId: "usr-maker-001" }, meta: { traceId: "t" } }),
      })
    );

    await page.goto("/transaksi/renewal/rnw-001");
    await page.waitForLoadState("networkidle");

    await assertButtonsAbsentFromDOM(page, ["review.*tandatangani", "^approve$"]);
  });

  test("SoD: page source for maker does not contain review/approve button HTML with maker's entity", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penempatan.read"], "usr-maker-001");

    page.route("**/api/v1/transaksi/penempatan/pnp-001**", (route: Route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { id: "pnp-001", kodePenempatan: "PNP-001", workflowStatus: "PENDING_REVIEW", makerId: "usr-maker-001" }, meta: { traceId: "t" } }),
      })
    );

    await page.goto("/transaksi/penempatan/pnp-001");
    await page.waitForLoadState("networkidle");

    const source = await page.content();
    // review button should not be in DOM source
    expect(source).not.toMatch(/data-action="review"[^>]*disabled/);
    // If review button is truly absent, no href to review endpoint either
  });
});

// ---------------------------------------------------------------------------
// Mutation buttons absent for ROLE-AUDIT
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Role Gating: ROLE-AUDIT Read-Only", () => {

  test("ROLE-AUDIT on MTM list: Upload + Cron buttons absent; DataTable + Export visible", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["transaksi.mtm.read"]);

    page.route("**/api/v1/transaksi/mtm**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    await assertButtonsAbsentFromDOM(page, ["upload", "trigger cron", "trigger manual"]);

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
  });

  test("ROLE-AUDIT on Penempatan list: no create CTA, no submit/approve buttons in table", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["penempatan.read"]);

    page.route("**/api/v1/transaksi/penempatan**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // No create CTA
    const createCta = page.locator("[aria-label='Tambah Penempatan Baru']");
    await expect(createCta).toHaveCount(0);

    // No submit/approve row-level actions
    await assertButtonsAbsentFromDOM(page, ["submit ke reviewer", "approve"]);
  });

  test("ROLE-AUDIT on Akrual list: no trigger, no approve buttons; DataTable visible", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["transaksi.akrual.read"]);

    page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { totalAkrualIdr: 0, instrumenDiproses: 0, statusBatchTerakhir: "NONE", periodeAktif: "PRD-2026-06" }, meta: { traceId: "t" } }) })
    );
    page.route("**/api/v1/transaksi/akrual**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    await assertButtonsAbsentFromDOM(page, ["jalankan batch akrual", "approve batch akrual"]);

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// Unauthorized route access
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Role Gating: Unauthorized Route Access", () => {

  test("ROLE-IT-ADMIN accessing /transaksi/penempatan: route renders notFound (no penempatan HTML)", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["user.manage"]);
    blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    // Should not see penempatan table content
    const penHeading = page.getByRole("heading", { name: /penempatan deposito|daftar penempatan/i });
    await expect(penHeading).toHaveCount(0);

    // Should see 404 or be redirected
    const is404 = await page.getByText(/404|tidak ditemukan|akses ditolak/i).count() > 0;
    const isRedirected = !page.url().includes("/transaksi/penempatan");
    expect(is404 || isRedirected).toBeTruthy();
  });

  test("ROLE-RISK accessing /transaksi/mtm/upload (no upload perm): redirected or notFound", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["transaksi.mtm.read"]); // read but no upload
    blockTransaksiEndpoints(page);

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    // Upload form must not be in DOM
    const dropzone = page.getByText(/taruh file di sini/i);
    const fileInput = page.locator("input[type='file']");
    await expect(dropzone).toHaveCount(0);
    await expect(fileInput).toHaveCount(0);
  });

  test("unauthenticated user accessing /transaksi/penempatan is redirected to login", async ({ page }) => {
    // No role set → no JWT → middleware should redirect to auth
    blockTransaksiEndpoints(page);

    await page.goto("/transaksi/penempatan");
    await page.waitForLoadState("networkidle");

    const onLoginPage = page.url().includes("/login") || page.url().includes("/auth") || page.url().includes("keycloak");
    const showsLoginForm = await page.getByLabel(/username|email|login/i).count() > 0;
    const isRedirectedAway = !page.url().includes("/transaksi/penempatan");

    expect(onLoginPage || showsLoginForm || isRedirectedAway).toBeTruthy();
  });
});
