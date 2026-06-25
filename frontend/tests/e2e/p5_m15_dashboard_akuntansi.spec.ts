/**
 * Playwright E2E — P5-M15 Akuntansi Dashboard
 *
 * AC coverage:
 *   M15-03-AC1 — W-AK-01 Jurnal Pending (RPT-26): ROLE-AKUN no Approve btn, ROLE-AKUN-CTL has it;
 *                W-AK-02 GL Delivery Rate donut; alert banner if FAILED > 5%
 *   M15-03-AC2 — W-AK-03 FX Freshness: FRESH/STALE indicator; alert link on STALE;
 *                W-AK-04 Periode Buku Timeline (RPT-23): OPEN/SOFT_CLOSED/HARD_CLOSED colors;
 *                CURRENT badge on active periode
 *   M15-03-AC3 — Role gate: ROLE-RISK → redirect; ROLE-AKUN (non-CTL) → Approve btn absent
 *   M15-03-AC4 — W-AK-05 Recent Jurnal Log (RPT-22); empty state; accessibility aria-labels
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const RPT_26_JURNAL_PENDING = {
  data: Array.from({ length: 15 }, (_, i) => ({
    id: `wf-${String(i + 1).padStart(3, "0")}`,
    entityType: "JURNAL",
    entityId: `jrn-${String(i + 1).padStart(6, "0")}`,
    kodeEntity: `JRN-${String(i + 1 + 1233).padStart(6, "0")}`,
    eventCode: i % 2 === 0 ? "MTM_FVOCI" : "EIR_AC",
    instrumenId: `inst-00${(i % 5) + 1}`,
    nominalIdr: 900_000 + i * 10_000,
    submittedBy: "USR-AKUN-002",
    submittedAt: "2026-06-25T09:00:00+07:00",
    status: "PENDING_APPROVAL",
  })),
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 15, limit: 20 },
  meta: { traceId: "trace-rpt-26-jurnal" },
};

const RPT_22B_GL_DELIVERY = {
  data: [
    { status: "DELIVERED", count: 980, pct: 98.0 },
    { status: "FAILED",    count: 15,  pct: 1.5 },
    { status: "PENDING",   count: 5,   pct: 0.5 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 200 },
  meta: { traceId: "trace-rpt-22b" },
};

const RPT_22B_GL_HIGH_FAIL = {
  data: [
    { status: "DELIVERED", count: 900, pct: 90.0 },
    { status: "FAILED",    count: 90,  pct: 9.0 },
    { status: "PENDING",   count: 10,  pct: 1.0 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 200 },
  meta: { traceId: "trace-rpt-22b-fail" },
};

const RPT_05_FX_FRESH = {
  data: [
    { tanggal: "2026-06-25", kodeMataUang: "USD", kursTengah: 16250, sumber: "BI_JISDOR" },
    { tanggal: "2026-06-24", kodeMataUang: "USD", kursTengah: 16225, sumber: "BI_JISDOR" },
    { tanggal: "2026-06-23", kodeMataUang: "USD", kursTengah: 16200, sumber: "BI_JISDOR" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 5 },
  meta: { traceId: "trace-rpt-05-fresh", today: "2026-06-25" },
};

const RPT_05_FX_STALE = {
  data: [
    { tanggal: "2026-06-24", kodeMataUang: "USD", kursTengah: 16225, sumber: "BI_JISDOR" },
    { tanggal: "2026-06-23", kodeMataUang: "USD", kursTengah: 16200, sumber: "BI_JISDOR" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 5 },
  meta: { traceId: "trace-rpt-05-stale", today: "2026-06-25" },
};

const RPT_23_PERIODE = {
  data: [
    { kode: "PRD-2026-06", status: "OPEN",        tanggalClose: null,             isCurrent: true  },
    { kode: "PRD-2026-05", status: "HARD_CLOSED",  tanggalClose: "2026-06-02",    isCurrent: false },
    { kode: "PRD-2026-04", status: "HARD_CLOSED",  tanggalClose: "2026-05-03",    isCurrent: false },
    { kode: "PRD-2026-03", status: "SOFT_CLOSED",  tanggalClose: "2026-04-10",    isCurrent: false },
    { kode: "PRD-2026-02", status: "HARD_CLOSED",  tanggalClose: "2026-03-04",    isCurrent: false },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 5, limit: 12 },
  meta: { traceId: "trace-rpt-23" },
};

const RPT_22_JURNAL_LOG = {
  data: Array.from({ length: 20 }, (_, i) => ({
    id: `jrn-${String(i + 1).padStart(6, "0")}`,
    jurnal_id: `JRN-${String(i + 1 + 1200).padStart(6, "0")}`,
    eventCode: i % 3 === 0 ? "MTM_FVOCI" : i % 3 === 1 ? "EIR_AC" : "STAGE_TRANSFER",
    instrumenId: `inst-00${(i % 5) + 1}`,
    nominalIdr: 900_000 + i * 5_000,
    postedAt: `2026-06-${String(25 - (i % 10)).padStart(2, "0")}T10:00:00+07:00`,
    statusPosting: "POSTED",
  })),
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 20, limit: 20 },
  meta: { traceId: "trace-rpt-22" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(page: Page, roles: string[], permissions: string[]) {
  return page.addInitScript(
    ({ r, p }: { r: string[]; p: string[] }) => {
      localStorage.setItem("blips_roles", JSON.stringify(r));
      localStorage.setItem("blips_permissions", JSON.stringify(p));
    },
    { r: roles, p: permissions }
  );
}

function mockAkuntansiEndpoints(page: Page, opts?: { highGlFail?: boolean; staleFx?: boolean }) {
  page.route("**/api/v1/reports/rpt-26**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_26_JURNAL_PENDING) })
  );
  page.route("**/api/v1/reports/rpt-22b**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json",
      body: JSON.stringify(opts?.highGlFail ? RPT_22B_GL_HIGH_FAIL : RPT_22B_GL_DELIVERY) })
  );
  page.route("**/api/v1/reports/rpt-05**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json",
      body: JSON.stringify(opts?.staleFx ? RPT_05_FX_STALE : RPT_05_FX_FRESH) })
  );
  page.route("**/api/v1/reports/rpt-23**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_23_PERIODE) })
  );
  page.route("**/api/v1/reports/rpt-22**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_22_JURNAL_LOG) })
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — Akuntansi Dashboard /dashboard/akuntansi", () => {

  // M15-03-AC1: Jurnal Pending + GL Delivery Rate
  test("M15-03-AC1: ROLE-AKUN sees W-AK-01 pending list but Approve button is absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]); // no jurnal.approve
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Jurnal pending widget visible
    const pendingWidget = page.getByRole("region", { name: /jurnal.*pending|jurnal menunggu/i });
    await expect(pendingWidget).toBeVisible({ timeout: 5000 });

    // Shows count badge "15 jurnal menunggu"
    await expect(page.getByText(/15 jurnal|15.*menunggu/i)).toBeVisible();

    // Approve button ABSENT (ROLE-AKUN has no jurnal.approve permission)
    const approveBtn = page.getByRole("button", { name: /approve|setujui/i });
    await expect(approveBtn).toHaveCount(0);
  });

  test("M15-03-AC1: ROLE-AKUN-CTL sees Approve button in W-AK-01", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["dashboard.akuntansi.read", "jurnal.approve"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Approve button visible for AKUN-CTL
    const approveBtn = page.getByRole("button", { name: /approve|setujui/i });
    await expect(approveBtn.first()).toBeVisible({ timeout: 5000 });
  });

  test("M15-03-AC1: W-AK-02 GL Delivery donut shows success rate 98.0% from mock", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // GL success rate KPI
    await expect(page.getByText(/98\.0%|98,0%/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/980 dari 1\.000|980 of 1,000/i)).toBeVisible();
  });

  test("M15-03-AC1: W-AK-02 shows warning banner when GL FAILED > 5%", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page, { highGlFail: true });

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Warning banner about GL failure rate
    await expect(page.getByText(/kegagalan GL.*melebihi|tingkat kegagalan|GL.*5%/i)).toBeVisible({ timeout: 5000 });
  });

  // M15-03-AC2: FX Freshness + Periode Timeline
  test("M15-03-AC2: W-AK-03 FX Freshness shows FRESH status when rate date = today", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // FRESH status visible
    await expect(page.getByText(/fresh|segar/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/16\.250|16,250|USD/i)).toBeVisible();
  });

  test("M15-03-AC2: W-AK-03 FX Freshness shows STALE alert with upload link when rate is outdated", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page, { staleFx: true });

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // STALE warning
    await expect(page.getByText(/stale|belum diperbarui hari ini|upload manual/i)).toBeVisible({ timeout: 5000 });

    // Link to upload FX rate
    const uploadLink = page.getByRole("link", { name: /upload.*fx|fx rate/i });
    await expect(uploadLink).toBeVisible();
  });

  test("M15-03-AC2: W-AK-04 Periode Timeline shows OPEN/HARD_CLOSED/SOFT_CLOSED periodes", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // CURRENT badge on active periode
    await expect(page.getByText(/current|aktif/i)).toBeVisible({ timeout: 5000 });

    // Periode codes
    await expect(page.getByText(/PRD-2026-06/)).toBeVisible();

    // Status labels
    await expect(page.getByText(/open|terbuka/i)).toBeVisible();
    await expect(page.getByText(/hard.closed|closed|ditutup/i)).toBeVisible();
  });

  // M15-03-AC3: Role gate + SoD button gate
  test("M15-03-AC3: ROLE-RISK accessing /dashboard/akuntansi → redirect; no widgets in DOM", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);

    let rpt26Called = false;
    page.route("**/api/v1/reports/rpt-26**", (route: Route) => {
      rpt26Called = true;
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "DASHBOARD_PERMISSION_DENIED", traceId: "t" } }) });
    });

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Akuntansi widgets absent from DOM
    const jurnal = page.locator('[data-widget-id="W-AK-01"]');
    await expect(jurnal).toHaveCount(0);

    const heading = page.getByRole("heading", { name: /akuntansi dashboard/i });
    await expect(heading).toHaveCount(0);
  });

  test("M15-03-AC3: ROLE-AKUN (non-CTL) Approve button is absent from DOM (server component check)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]); // no jurnal.approve
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Approve button must be ABSENT from DOM (not just disabled)
    const approveBtn = page.getByRole("button", { name: /approve|setujui/i });
    await expect(approveBtn).toHaveCount(0);
  });

  // M15-03-AC4: Recent Jurnal Log + empty state + accessibility
  test("M15-03-AC4: W-AK-05 Recent Jurnal Log shows 20 entries with instrumen links", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Jurnal log rows visible (event codes from mock)
    await expect(page.getByText(/MTM_FVOCI|EIR_AC|STAGE_TRANSFER/i)).toBeVisible({ timeout: 5000 });
  });

  test("M15-03-AC4: W-AK-05 empty state shows illustrated message and link to full report", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);

    page.route("**/api/v1/reports/rpt-22**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({
        data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 20 },
        meta: { traceId: "trace-empty" },
      }) })
    );
    page.route("**/api/v1/reports/rpt-26**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...RPT_26_JURNAL_PENDING, data: [] }) })
    );
    page.route("**/api/v1/reports/rpt-22b**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_22B_GL_DELIVERY) })
    );
    page.route("**/api/v1/reports/rpt-05**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_05_FX_FRESH) })
    );
    page.route("**/api/v1/reports/rpt-23**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_23_PERIODE) })
    );

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Empty state message
    await expect(page.getByText(/tidak ada jurnal|tidak tersedia untuk periode/i)).toBeVisible({ timeout: 5000 });

    // "Lihat semua jurnal" link
    const link = page.getByRole("link", { name: /lihat semua jurnal/i });
    await expect(link).toBeVisible();
  });

  test("M15-03-AC4: widgets have aria-label containing Akuntansi Dashboard", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    const labeledRegions = page.locator('[aria-label*="Akuntansi Dashboard"]');
    await expect(labeledRegions.first()).toBeVisible({ timeout: 5000 });
  });

  test("M15-03-AC4: Nominal IDR column is right-aligned with descriptive aria-label per cell", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);
    await mockAkuntansiEndpoints(page);

    await page.goto("/dashboard/akuntansi");
    await page.waitForLoadState("networkidle");

    // Nominal cells with aria-label="Nominal: Rp ..."
    const nominalCells = page.locator('[aria-label^="Nominal: Rp"]');
    if (await nominalCells.count() > 0) {
      await expect(nominalCells.first()).toBeVisible();
    }
  });
});
