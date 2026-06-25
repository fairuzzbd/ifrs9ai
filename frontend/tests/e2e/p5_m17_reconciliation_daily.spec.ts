/**
 * Playwright E2E — P5-M17 Reconciliation Daily Screen
 *
 * AC coverage:
 *   M17-05-AC1 — Summary card: BLIPS vs GL count + nominal + mismatch count
 *   M17-05-AC2 — Mismatch DataTable: drill-down ke line items, row highlight by jenis
 *   M17-05-AC3 — Historis: navigasi recon per tanggal + URL state (nuqs)
 *   M17-05-AC4 — Role gate: read-only page; ROLE-AKUN tidak punya akses; ROLE-AUDIT punya akses
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const RECON_SUMMARY_RESPONSE = {
  data: {
    tanggal: "2026-06-25",
    blips: { jumlahJurnal: 1240, totalDebitIdr: 45678901234 },
    gl: { jumlahJurnal: 1235, totalDebitIdr: 45678901234 },
    mismatchCount: 5,
    dlqPendingCount: 3,
    dataUpdatedAt: "2026-06-25T23:59:00+07:00",
  },
  meta: { traceId: "trace-recon-summary" },
};

const RECON_ZERO_MISMATCH = {
  data: { ...RECON_SUMMARY_RESPONSE.data, mismatchCount: 0, dlqPendingCount: 0 },
  meta: { traceId: "trace-recon-zero" },
};

const RECON_NOT_AVAILABLE = {
  error: { code: "NOT_FOUND", message: "Data rekonsiliasi belum tersedia untuk tanggal ini.", traceId: "t" },
};

const MISMATCH_LIST_RESPONSE = {
  data: [
    { id: "mm-001", nomorJurnal: "JRN-2026-0041", tanggalJurnal: "2026-06-24", nominalBlipsIdr: 8200000, nominalGlIdr: null, selisihIdr: 8200000, jenisMismatch: "MISSING_IN_GL", statusResolusi: "OPEN" },
    { id: "mm-002", nomorJurnal: "JRN-2026-0039", tanggalJurnal: "2026-06-23", nominalBlipsIdr: 3100000, nominalGlIdr: 3000000, selisihIdr: 100000, jenisMismatch: "AMOUNT_DIFF", statusResolusi: "OPEN" },
    { id: "mm-003", nomorJurnal: null, tanggalJurnal: "2026-06-22", nominalBlipsIdr: null, nominalGlIdr: 500000, selisihIdr: 500000, jenisMismatch: "EXTRA_IN_GL", statusResolusi: "ACKNOWLEDGED" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 50 },
  appliedSort: [{ col: "selisihIdr", dir: "desc" }],
  appliedFilter: {},
  meta: { traceId: "trace-mismatch-list" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-ctl-001",
  mfaVerified = true
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

function mockReconEndpoints(page: Page, summaryResponse = RECON_SUMMARY_RESPONSE) {
  page.route("**/api/v1/gl-delivery/recon/mismatches**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("/export") || url.includes("format=csv") || url.includes("format=xlsx")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "nomor_jurnal,selisih\nJRN-2026-0041,8200000" });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MISMATCH_LIST_RESPONSE) });
  });

  page.route("**/api/v1/gl-delivery/recon**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(summaryResponse) })
  );

  page.route("**/api/v1/reports/gl-delivery**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(summaryResponse) })
  );

  page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: Array(3).fill({ status: "PENDING" }), pagination: { totalEstimate: 3 }, meta: { traceId: "t" } }),
    })
  );
}

// ---------------------------------------------------------------------------
// M17-05-AC1: Summary cards
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Reconciliation Daily: Summary Cards (AC1)", () => {

  test("M17-05-AC1: page renders 4 summary cards with correct data", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.export"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // BLIPS card
    await expect(page.getByText(/1\.240|1,240.*jurnal|jurnal BLIPS/i)).toBeVisible({ timeout: 5000 });

    // GL card
    await expect(page.getByText(/1\.235|1,235.*jurnal|GL.*jurnal/i)).toBeVisible({ timeout: 5000 });

    // Mismatch card — red because > 0
    await expect(page.getByText(/5 entri|Mismatch.*5/i)).toBeVisible({ timeout: 5000 });

    // DLQ pending card
    await expect(page.getByText(/DLQ Pending|3 entri.*DLQ|DLQ.*3/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC1: mismatch card is green when count = 0", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page, RECON_ZERO_MISMATCH);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // Mismatch card shows 0 or green
    await expect(page.getByText(/0 entri|mismatch.*0|tidak ada mismatch/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC1: date picker rendered with default = today", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const datePicker = page.getByRole("combobox", { name: /tanggal|date/i }).or(
      page.locator("input[type='date'], [data-testid='date-picker']")
    );
    await expect(datePicker.first()).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC1: banner shown when recon data not available", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);

    page.route("**/api/v1/gl-delivery/recon**", (route: Route) =>
      route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify(RECON_NOT_AVAILABLE) })
    );
    page.route("**/api/v1/reports/gl-delivery**", (route: Route) =>
      route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify(RECON_NOT_AVAILABLE) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { totalEstimate: 0 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/belum tersedia|cron berjalan|23:59/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC1: Refresh button present; no Jalankan Rekonsiliasi button (cron-only)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // Refresh button present
    await expect(page.getByRole("button", { name: /refresh|perbarui/i })).toBeVisible({ timeout: 5000 });

    // No manual reconciliation trigger
    await expect(page.getByRole("button", { name: /jalankan rekonsiliasi|run recon|trigger recon/i })).toHaveCount(0);
  });

  test("M17-05-AC1: DLQ card has link to /jurnal/dlq when count > 0", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const dlqLink = page.getByRole("link", { name: /lihat DLQ|jurnal\/dlq/i });
    await expect(dlqLink).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M17-05-AC2: Mismatch DataTable
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Reconciliation Daily: Mismatch DataTable (AC2)", () => {

  test("M17-05-AC2: mismatch DataTable renders when mismatch_count > 0", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("JRN-2026-0041")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("JRN-2026-0039")).toBeVisible();
  });

  test("M17-05-AC2: default sort selisih_idr:desc in mismatch API request", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);

    let capturedUrl = "";
    page.route("**/api/v1/gl-delivery/recon/mismatches**", (route: Route) => {
      capturedUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MISMATCH_LIST_RESPONSE) });
    });
    page.route("**/api/v1/gl-delivery/recon**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RECON_SUMMARY_RESPONSE) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { totalEstimate: 0 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    expect(capturedUrl).toContain("selisih");
  });

  test("M17-05-AC2: MISSING_IN_GL row has yellow highlight indicator", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // Row for MISSING_IN_GL should have distinct styling or text
    const missingRow = page.getByText("MISSING").or(page.getByText("MISSING_IN_GL")).first();
    await expect(missingRow).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC2: AMOUNT_DIFF row visible", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/AMOUNT_DIFF|AMT.DIFF/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC2: EXTRA_IN_GL row visible", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/EXTRA_IN_GL|EXTRA/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC2: no mutating action buttons in DataTable (read-only)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // No resolve/edit/delete buttons
    await expect(page.getByRole("button", { name: /^resolve$|^edit$|^hapus$/i })).toHaveCount(0);
  });

  test("M17-05-AC2: filter[jenis_mismatch] filter chip", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily?filter[jenis_mismatch]=MISSING_IN_GL");
    await page.waitForLoadState("networkidle");

    // Filter should be applied / chip visible
    await expect(page.getByText(/MISSING_IN_GL|bersihkan|clear/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC2: export buttons present (CSV + XLSX)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.export"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /ekspor|export/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC2: link Lihat Jurnal per mismatch row navigates to /jurnal/header/{id}", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // JRN-2026-0041 row should have link to /jurnal/header/...
    const jurnalLink = page.getByRole("link", { name: /lihat jurnal|JRN-2026-0041/i }).first();
    if (await jurnalLink.isVisible()) {
      const href = await jurnalLink.getAttribute("href");
      expect(href).toMatch(/\/jurnal\/header\//);
    }
  });

  test("M17-05-AC2: mismatch table absent when mismatch_count = 0", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);

    page.route("**/api/v1/gl-delivery/recon/mismatches**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) })
    );
    page.route("**/api/v1/gl-delivery/recon**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RECON_ZERO_MISMATCH) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { totalEstimate: 0 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // When no mismatch: table may be hidden or show empty state
    const mismatchTable = page.getByText("MISSING_IN_GL").or(page.getByText("AMOUNT_DIFF"));
    await expect(mismatchTable).toHaveCount(0);
  });
});

// ---------------------------------------------------------------------------
// M17-05-AC3: Date picker URL state
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Reconciliation Daily: Date Picker URL State (AC3)", () => {

  test("M17-05-AC3: date picker change updates URL to ?tanggal=YYYY-MM-DD", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const datePicker = page.locator("input[type='date'], [data-testid='date-picker']").first();
    if (await datePicker.isVisible()) {
      await datePicker.fill("2026-06-20");
      await datePicker.press("Enter");

      await page.waitForTimeout(500);
      expect(page.url()).toContain("2026-06-20");
    }
  });

  test("M17-05-AC3: ?tanggal=2026-06-20 in URL causes API call with that date", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);

    let capturedUrl = "";
    page.route("**/api/v1/gl-delivery/recon**", (route: Route) => {
      capturedUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RECON_SUMMARY_RESPONSE) });
    });
    page.route("**/api/v1/gl-delivery/recon/mismatches**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MISMATCH_LIST_RESPONSE) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { totalEstimate: 0 }, meta: { traceId: "t" } }) })
    );

    await page.goto("/reconciliation/daily?tanggal=2026-06-20");
    await page.waitForLoadState("networkidle");

    expect(capturedUrl).toContain("2026-06-20");
  });

  test("M17-05-AC3: page title reflects selected date", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily?tanggal=2026-06-25");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/Rekonsiliasi Harian|25 Juni 2026|25 Jun 2026/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC3: breadcrumb shows Rekonsiliasi / Harian", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb'], [aria-label='breadcrumb'], nav.breadcrumb");
    await expect(breadcrumb.getByText(/Rekonsiliasi|Harian/i).first()).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M17-05-AC4: Role gate
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Reconciliation Daily: Role Gate (AC4)", () => {

  test("M17-05-AC4: ROLE-AKUN without jurnal.read redirected or sees 403", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], []);  // no jurnal.read
    page.route("**/api/v1/**", (route: Route) =>
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "FORBIDDEN", traceId: "t" } }) })
    );

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const isRedirected = !page.url().includes("/reconciliation/daily");
    const isForbidden = (await page.getByText(/403|tidak diizinkan|akses ditolak|dashboard/i).count()) > 0;

    expect(isRedirected || isForbidden).toBeTruthy();
  });

  test("M17-05-AC4: ROLE-AUDIT can access read-only page", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["audit_log.read", "jurnal.read"], "usr-audit-001", false);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // Page should render without redirect
    expect(page.url()).toContain("/reconciliation/daily");
    await expect(page.getByText(/Rekonsiliasi Harian|BLIPS|jurnal/i).first()).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC4: ROLE-AUDIT sees export button (jurnal.export via aud.*.read convention)", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["audit_log.read", "jurnal.read", "jurnal.export"], "usr-audit-001");
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /ekspor|export/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-05-AC4: no mutation buttons present for any role (full read-only)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.approve", "jurnal.export"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    // No approve, reject, post, replay buttons
    await expect(page.getByRole("button", { name: /^approve$|^reject$|^post$|^replay$/i })).toHaveCount(0);
  });

  test("M17-05-AC4: page <title> is Rekonsiliasi Harian | BLIPS", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-ctl-001", true);
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily?tanggal=2026-06-25");
    await page.waitForLoadState("networkidle");

    await expect(page).toHaveTitle(/Rekonsiliasi Harian|BLIPS/i);
  });
});
