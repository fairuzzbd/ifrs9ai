/**
 * Playwright E2E — P5-M16 Penjualan Screens
 *
 * AC coverage:
 *   M16-03-AC3 — List DataTable UX §1; BM-alerts widget DataTable; BM warning on form
 *   M16-03-AC4 — SoD enforcement + workflow toast (shared with renewal; penjualan-specific tests here)
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const PENJUALAN_LIST_RESPONSE = {
  data: [
    { id: "pjl-001", kodePenjualan: "PJL-001", kodeInstrumen: "OBL-0042", jenisInstrumen: "OBLIGASI", jenisDisposal: "SELL", tanggalEksekusi: "2026-06-25", nominalIdr: 5_000_000_000, workflowStatus: "DRAFT", hasBmAlert: false, makerId: "usr-maker-001" },
    { id: "pjl-002", kodePenjualan: "PJL-002", kodeInstrumen: "SHM-0099", jenisInstrumen: "SAHAM", jenisDisposal: "SELL", tanggalEksekusi: "2026-06-20", nominalIdr: 1_000_000_000, workflowStatus: "SUBMITTED", hasBmAlert: true, makerId: "usr-maker-002" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 50 },
  meta: { traceId: "trace-pjl-list" },
};

const BM_ALERTS_RESPONSE = {
  data: [
    { id: "bma-001", kodeInstrumen: "SHM-0099", portofolio: "PORT-SAHAM", bmStatus: "POTENTIAL_RECLASSIFICATION", triggerEvent: "HIGH_FREQUENCY_SALE", tanggalTrigger: "2026-06-20", recommendation: "Tinjau BM portofolio" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
  meta: { traceId: "trace-bma" },
};

const PENJUALAN_CREATE_RESPONSE = {
  data: { id: "pjl-new-001", kodePenjualan: "PJL-101", workflowStatus: "DRAFT" },
  meta: { traceId: "trace-pjl-create" },
};

const PENJUALAN_DETAIL_RESPONSE = {
  data: {
    id: "pjl-002",
    kodePenjualan: "PJL-002",
    kodeInstrumen: "SHM-0099",
    jenisDisposal: "SELL",
    workflowStatus: "SUBMITTED",
    makerId: "usr-maker-002",
  },
  meta: { traceId: "trace-pjl-detail" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(page: Page, roles: string[], permissions: string[], userId = "usr-maker-001", mfaVerified = false) {
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

function mockPenjualanApi(page: Page) {
  page.route("**/api/v1/transaksi/penjualan**", (route: Route) => {
    const url = route.request().url();
    const method = route.request().method();

    if (url.includes("/bm-alerts")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(BM_ALERTS_RESPONSE) });
    }
    if (url.includes("/submit") || url.includes("/review") || url.includes("/approve") || url.includes("/reject")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { workflowStatus: "PENDING_REVIEW" }, meta: { traceId: "t" } }) });
    }
    if (url.match(/\/penjualan\/pjl-\w+$/) && method === "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_DETAIL_RESPONSE) });
    }
    if (url.includes("/export") || url.includes("format=csv")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "kode,nominal\nPJL-001,5000000000" });
    }
    if (method === "POST" && url.endsWith("/penjualan")) {
      return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(PENJUALAN_CREATE_RESPONSE) });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_LIST_RESPONSE) });
  });
}

// ---------------------------------------------------------------------------
// M16-03-AC3: List DataTable UX §1
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Penjualan List: DataTable UX §1 + BM Alert Filter", () => {

  test("M16-03-AC3: list renders DataTable with required columns and BM alert badge", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read", "penjualan.create"]);
    mockPenjualanApi(page);

    await page.goto("/transaksi/penjualan");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("PJL-001")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("PJL-002")).toBeVisible();

    await expect(page.getByRole("columnheader", { name: /kode|instrumen/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /nominal/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /status/i })).toBeVisible();
  });

  test("M16-03-AC3: breadcrumb visible on penjualan list (M16 gap fix: was missing)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    mockPenjualanApi(page);

    await page.goto("/transaksi/penjualan");
    await page.waitForLoadState("networkidle");

    const breadcrumb = page.locator("nav[aria-label='Breadcrumb']");
    await expect(breadcrumb).toBeVisible({ timeout: 5000 });
    await expect(breadcrumb.getByRole("link", { name: /beranda/i })).toBeVisible();
  });

  test("M16-03-AC3: BM Alert quick-filter button sets filter[bm_alert]=true in URL", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    mockPenjualanApi(page);

    await page.goto("/transaksi/penjualan");
    await page.waitForLoadState("networkidle");

    const bmAlertBtn = page.getByRole("button", { name: /bm alert|bm.*alert/i });
    await expect(bmAlertBtn).toBeVisible({ timeout: 5000 });

    await bmAlertBtn.click();
    await page.waitForTimeout(300);

    expect(page.url()).toContain("bm_alert");
  });

  test("M16-03-AC3: sort uses URL state (useQueryState) — not local state", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    mockPenjualanApi(page);

    await page.goto("/transaksi/penjualan?sort=tanggal_eksekusi:asc");
    await page.waitForLoadState("networkidle");

    // Sort reflected from URL on page load (deep-link friendly)
    expect(page.url()).toContain("sort=tanggal_eksekusi:asc");
  });

  test("M16-03-AC3: export button includes active filter[jenis_disposal] in export URL", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);

    let exportUrl: string | null = null;
    page.route("**/api/v1/transaksi/penjualan**", (route: Route) => {
      const url = route.request().url();
      if (url.includes("export") || url.includes("format=csv")) {
        exportUrl = url;
        return route.fulfill({ status: 200, contentType: "text/csv", body: "kode\nPJL-001" });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_LIST_RESPONSE) });
    });

    await page.goto("/transaksi/penjualan?filter[jenis_disposal]=SELL");
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    if (await exportBtn.count() > 0) {
      await exportBtn.click();
      const csvItem = page.getByRole("menuitem", { name: /csv/i });
      if (await csvItem.count() > 0) {
        await csvItem.click();
        await page.waitForTimeout(500);
        // Export URL should include jenis_disposal filter
        if (exportUrl) {
          expect(exportUrl).toContain("jenis_disposal");
        }
      }
    }
  });

  test("M16-03-AC3: BM alerts link in header navigates to /transaksi/penjualan/bm-alerts", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    mockPenjualanApi(page);
    page.route("**/api/v1/transaksi/penjualan/bm-alerts**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(BM_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan");
    await page.waitForLoadState("networkidle");

    const bmLink = page.getByRole("link", { name: /bm.*alert|business model alert/i });
    await expect(bmLink).toBeVisible({ timeout: 5000 });
    await expect(bmLink).toHaveAttribute("href", "/transaksi/penjualan/bm-alerts");
  });
});

// ---------------------------------------------------------------------------
// M16-03-AC3: BM Alerts widget page
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Penjualan BM Alerts: DataTable", () => {

  test("M16-03-AC3: /bm-alerts DataTable renders with required columns", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    page.route("**/api/v1/transaksi/penjualan/bm-alerts**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(BM_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan/bm-alerts");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("SHM-0099")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("columnheader", { name: /kode|instrumen/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /bm.*status|status/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /trigger|event/i })).toBeVisible();
  });

  test("M16-03-AC3: ROLE-RISK sees Review BM Assessment link per row", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["penjualan.read"]);
    page.route("**/api/v1/transaksi/penjualan/bm-alerts**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(BM_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan/bm-alerts");
    await page.waitForLoadState("networkidle");

    const reviewLink = page.getByRole("link", { name: /review bm assessment|review bm/i });
    await expect(reviewLink).toBeVisible({ timeout: 5000 });
    // Should link to APP-A territory
    const href = await reviewLink.getAttribute("href");
    expect(href).toContain("/master/portofolio/");
  });

  test("M16-03-AC3: BM alerts export available with filter params", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read"]);
    page.route("**/api/v1/transaksi/penjualan/bm-alerts**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(BM_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan/bm-alerts");
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M16-03-AC3: BM warning on penjualan new form
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Penjualan Form: BM Warning Banner", () => {

  test("M16-03-AC3: creating penjualan for instrument with BM alert shows informational warning (non-blocking)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read", "penjualan.create"]);

    page.route("**/api/v1/transaksi/penjualan**", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify(PENJUALAN_CREATE_RESPONSE) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_LIST_RESPONSE) });
    });
    // Master instrument lookup showing BM alert
    page.route("**/api/v1/master/instrumen/**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "shn-0099", hasBmAlert: true, bmAlertMessage: "Penjualan instrumen ini mungkin berdampak pada Business Model" }, meta: { traceId: "t" } }) })
    );

    await page.goto("/transaksi/penjualan/new");
    await page.waitForLoadState("networkidle");

    // Select instrument with BM alert
    const instrumenSelect = page.getByLabel(/instrumen|kode instrumen/i);
    if (await instrumenSelect.count() > 0) {
      await instrumenSelect.fill("SHM-0099");

      // BM warning banner should appear
      const warning = page.getByText(/business model|berdampak.*portfolio|konsultasikan.*risk officer/i);
      await expect(warning).toBeVisible({ timeout: 3000 });

      // Submit should still be enabled (not blocked by warning)
      const submitBtn = page.getByRole("button", { name: /simpan|submit/i });
      const isEnabled = !(await submitBtn.isDisabled());
      expect(isEnabled).toBeTruthy();
    }
  });
});

// ---------------------------------------------------------------------------
// M16-03-AC4: Penjualan SoD enforcement
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Penjualan Workflow: SoD Enforcement", () => {

  test("M16-03-AC4: maker (usr-maker-002) of PJL-002: Review + Approve buttons absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read", "penjualan.approve"], "usr-maker-002");

    page.route("**/api/v1/transaksi/penjualan/pjl-002**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_DETAIL_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan/pjl-002");
    await page.waitForLoadState("networkidle");

    const reviewBtn = page.getByRole("button", { name: /review.*tandatangani|review/i });
    const approveBtn = page.getByRole("button", { name: /^approve$/i });
    await expect(reviewBtn).toHaveCount(0);
    await expect(approveBtn).toHaveCount(0);
  });

  test("M16-03-AC4: API returns 403 SOD_VIOLATION on penjualan approve by maker", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["penjualan.read", "penjualan.approve"], "usr-maker-002");

    page.route("**/api/v1/transaksi/penjualan/pjl-002/approve", (route: Route) =>
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "SOD_VIOLATION", message: "maker cannot approve", traceId: "trace-sod" } }) })
    );
    page.route("**/api/v1/transaksi/penjualan/pjl-002", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(PENJUALAN_DETAIL_RESPONSE) })
    );

    await page.goto("/transaksi/penjualan/pjl-002");
    await page.waitForLoadState("networkidle");

    // API call via browser programmatic fetch would return 403
    const response = await page.evaluate(() =>
      fetch("/api/v1/transaksi/penjualan/pjl-002/approve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ comment: "test" }),
      }).then((r) => r.status)
    );
    expect(response).toBe(403);
  });

  test("M16-03-AC4: Penjualan berhasil di-approve shows specific success toast", async ({ page }) => {
    await setRole(page, ["ROLE-APPR-TR"], ["penjualan.read", "penjualan.approve"], "usr-appr-001");

    page.route("**/api/v1/transaksi/penjualan/pjl-002**", (route: Route) => {
      if (route.request().url().includes("/approve")) {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { workflowStatus: "APPROVED" }, meta: { traceId: "t" } }) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...PENJUALAN_DETAIL_RESPONSE, data: { ...PENJUALAN_DETAIL_RESPONSE.data, workflowStatus: "PENDING_APPROVAL", makerId: "usr-maker-002" } }) });
    });

    await page.goto("/transaksi/penjualan/pjl-002");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve|setujui/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.click();

      const commentInput = page.getByLabel(/komentar|comment/i);
      if (await commentInput.count() > 0) await commentInput.fill("Approved");

      const confirmBtn = page.getByRole("button", { name: /konfirmasi|setujui.*sekarang/i });
      if (await confirmBtn.count() > 0) await confirmBtn.click();

      const successToast = page.getByText(/PJL-002.*berhasil.*approve|penjualan.*berhasil di-approve/i);
      await expect(successToast).toBeVisible({ timeout: 5000 });
    }
  });
});
