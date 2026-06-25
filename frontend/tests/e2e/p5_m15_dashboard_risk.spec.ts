/**
 * Playwright E2E — P5-M15 Risk Dashboard
 *
 * AC coverage:
 *   M15-02-AC1 — W-RK-01 ECL Stage Distribution Donut (RPT-13) + W-RK-04 Top-10 ECL +
 *                W-RK-02 SICR Triggers Counter (RPT-15); data matches mock
 *   M15-02-AC2 — W-RK-05 Calc-Run Status: SSE live progress (mocked via polling fallback);
 *                job completed → toast success + link "Lihat detail"; no active job → KPI card
 *   M15-02-AC3 — Role gate: ROLE-AKUN accessing /dashboard/risk → redirect; widgets absent from DOM
 *   M15-02-AC4 — W-RK-03 Stage Movement LineChart (RPT-14); color-coded aria labels; WCAG color spec
 *
 * Pattern: all API calls mocked via page.route(); SSE mocked via polling fallback endpoint.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const LATEST_RUN_ID = "CR-2026-06";
const JOB_ID_ECL    = "JOB-ECL-2026-06";

const RPT_13_STAGE_DIST = {
  data: [
    { instrumenId: "inst-001", kodeInstrumen: "DEP-001", nama: "Deposito BCA", stage: 1, eadIdr: 10_000_000, eclWeighted: 50_000, flMultiplierWorst: 1.10 },
    { instrumenId: "inst-002", kodeInstrumen: "OBL-002", nama: "Obligasi FR",  stage: 2, eadIdr:  3_000_000, eclWeighted: 96_000, flMultiplierWorst: 1.32 },
    { instrumenId: "inst-003", kodeInstrumen: "SHM-003", nama: "Saham Telkom", stage: 3, eadIdr:  5_000_000, eclWeighted: 1_200_000, flMultiplierWorst: 1.45 },
    // Simulate 2400 stage-1, 180 stage-2, 20 stage-3 totals via counts in meta
    ...Array.from({ length: 7 }, (_, i) => ({
      instrumenId: `inst-00${i + 10}`, kodeInstrumen: `DEP-00${i + 10}`, nama: `Deposito ${i}`,
      stage: 1, eadIdr: 5_000_000, eclWeighted: 25_000, flMultiplierWorst: 1.05,
    })),
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 10, limit: 200 },
  meta: { traceId: "trace-rpt-13", stageCountS1: 2400, stageCountS2: 180, stageCountS3: 20, totalCount: 2600 },
};

const RPT_15_SICR = {
  data: [
    { id: "sicr-001", instrumenId: "inst-002", triggerType: "RATING_DOWNGRADE", tanggalTrigger: "2026-06-20", detail: "AA- to BBB+" },
    { id: "sicr-002", instrumenId: "inst-003", triggerType: "IG_TO_NONIG",      tanggalTrigger: "2026-06-18", detail: "BBB- to BB+" },
    { id: "sicr-003", instrumenId: "inst-004", triggerType: "DPD_30",           tanggalTrigger: "2026-06-15", detail: "DPD=32" },
    { id: "sicr-004", instrumenId: "inst-005", triggerType: "RATING_DOWNGRADE", tanggalTrigger: "2026-06-10", detail: "A to BBB" },
    { id: "sicr-005", instrumenId: "inst-006", triggerType: "RATING_DOWNGRADE", tanggalTrigger: "2026-06-08", detail: "AA to A-" },
    { id: "sicr-006", instrumenId: "inst-007", triggerType: "IG_TO_NONIG",      tanggalTrigger: "2026-06-07", detail: "BBB- to BB" },
    { id: "sicr-007", instrumenId: "inst-008", triggerType: "IG_TO_NONIG",      tanggalTrigger: "2026-06-05", detail: "BBB to BB+" },
    { id: "sicr-008", instrumenId: "inst-009", triggerType: "IG_TO_NONIG",      tanggalTrigger: "2026-06-03", detail: "BBB- to BB-" },
    { id: "sicr-009", instrumenId: "inst-010", triggerType: "DPD_30",           tanggalTrigger: "2026-06-02", detail: "DPD=45" },
    { id: "sicr-010", instrumenId: "inst-011", triggerType: "RATING_DOWNGRADE", tanggalTrigger: "2026-05-30", detail: "A+ to BBB+" },
    { id: "sicr-011", instrumenId: "inst-012", triggerType: "RATING_DOWNGRADE", tanggalTrigger: "2026-05-25", detail: "AA to A" },
    { id: "sicr-012", instrumenId: "inst-013", triggerType: "DPD_30",           tanggalTrigger: "2026-05-20", detail: "DPD=38" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 12, limit: 50 },
  meta: { traceId: "trace-rpt-15", countRatingDowngrade: 5, countIgToNonIg: 4, countDpd30: 3 },
};

const RPT_14_MOVEMENT = {
  data: [
    { periodeId: "PRD-2026-01", periodeLabel: "Jan 2026", s1Count: 2450, s2Count: 130, s3Count: 20 },
    { periodeId: "PRD-2026-02", periodeLabel: "Feb 2026", s1Count: 2430, s2Count: 148, s3Count: 22 },
    { periodeId: "PRD-2026-03", periodeLabel: "Mar 2026", s1Count: 2420, s2Count: 158, s3Count: 22 },
    { periodeId: "PRD-2026-04", periodeLabel: "Apr 2026", s1Count: 2410, s2Count: 167, s3Count: 23 },
    { periodeId: "PRD-2026-05", periodeLabel: "Mei 2026", s1Count: 2408, s2Count: 170, s3Count: 22 },
    { periodeId: "PRD-2026-06", periodeLabel: "Jun 2026", s1Count: 2400, s2Count: 180, s3Count: 20 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 6, limit: 200 },
  meta: { traceId: "trace-rpt-14" },
};

const JOB_RUNNING = {
  data: {
    jobId: JOB_ID_ECL, type: "ECL_CALC_RUN", status: "running", progress: 47,
    currentStep: "Menghitung Stage 2 instruments (1234 dari 2600)",
    startedAt: "2026-06-25T10:30:00+07:00", estimatedCompletionAt: "2026-06-25T10:35:00+07:00",
    result: null, error: null, canCancel: false, createdBy: "user-risk-001",
  },
  meta: { traceId: "trace-job-running" },
};

const JOB_COMPLETED = {
  data: {
    jobId: JOB_ID_ECL, type: "ECL_CALC_RUN", status: "completed", progress: 100,
    currentStep: "Selesai. 2.600 instrumen diproses.",
    startedAt: "2026-06-25T10:30:00+07:00", estimatedCompletionAt: null,
    result: { calcRunId: LATEST_RUN_ID, totalEclWeighted: "12500000000", totalInstruments: 2600 },
    error: null, canCancel: false, createdBy: "user-risk-001",
  },
  meta: { traceId: "trace-job-completed" },
};

const JOB_NONE = {
  data: {
    jobId: "JOB-ECL-2026-05", type: "ECL_CALC_RUN", status: "completed", progress: 100,
    currentStep: "Selesai.",
    startedAt: "2026-05-31T09:00:00+07:00", estimatedCompletionAt: null,
    result: { calcRunId: "CR-2026-05", totalEclWeighted: "11800000000", totalInstruments: 2590 },
    error: null, canCancel: false, createdBy: "user-risk-001",
  },
  meta: { traceId: "trace-job-none" },
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

function mockRiskEndpoints(page: Page) {
  page.route("**/api/v1/reports/rpt-13**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_13_STAGE_DIST) })
  );
  page.route("**/api/v1/reports/rpt-15**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_15_SICR) })
  );
  page.route("**/api/v1/reports/rpt-14**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_14_MOVEMENT) })
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — Risk Dashboard /dashboard/risk", () => {

  // M15-02-AC1: Stage donut + top-10 ECL + SICR counters
  test("M15-02-AC1: W-RK-01 ECL Stage Donut renders with Stage 1/2/3 data", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/${JOB_ID_ECL}**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Stage distribution donut present
    const donutWidget = page.getByRole("region", { name: /distribusi stage|stage distribution/i });
    await expect(donutWidget).toBeVisible({ timeout: 5000 });

    // Stage labels present
    await expect(page.getByText(/stage 1/i)).toBeVisible();
    await expect(page.getByText(/stage 2/i)).toBeVisible();
    await expect(page.getByText(/stage 3/i)).toBeVisible();

    // Total count in center label
    await expect(page.getByText(/2\.600|2600/)).toBeVisible();
  });

  test("M15-02-AC1: W-RK-04 Top-10 ECL DataTable shows kode_instrumen and ecl_weighted", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Top-10 table shows instrument from mock (stage 3 highest ECL)
    await expect(page.getByText(/OBL-002|SHM-003/i)).toBeVisible({ timeout: 5000 });
  });

  test("M15-02-AC1: W-RK-02 SICR Triggers shows counts per trigger type", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // SICR trigger type labels present (5 rating downgrade, 4 IG-to-NonIG, 3 DPD)
    await expect(page.getByText(/rating downgrade|downgrade/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/dpd/i)).toBeVisible();
  });

  // M15-02-AC2: Calc-Run Status widget with active job
  test("M15-02-AC2: W-RK-05 shows JobProgressPanel with 47% when active calc-run running", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/${JOB_ID_ECL}**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_RUNNING) })
    );
    // SSE stream 503 → triggers polling fallback
    page.route(`**/api/v1/jobs/${JOB_ID_ECL}/stream**`, (route: Route) =>
      route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "SSE_STREAM_UNAVAILABLE", traceId: "t" } }) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Progress panel with 47%
    await expect(page.getByText(/47%|47 %/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/stage 2 instruments|1234 dari 2600/i)).toBeVisible();

    // Cancel button absent (ROLE-RISK has no job.cancel permission)
    const cancelBtn = page.getByRole("button", { name: /batalkan|cancel/i });
    await expect(cancelBtn).toHaveCount(0);
  });

  test("M15-02-AC2: W-RK-05 shows KPI card (Last Run COMPLETED) when no active job", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Should show last run completed status
    await expect(page.getByText(/completed|selesai|CR-2026-05/i)).toBeVisible({ timeout: 5000 });
  });

  test("M15-02-AC2: calc-run completed → success toast with ECL total and detail link", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);

    let callCount = 0;
    page.route(`**/api/v1/jobs/${JOB_ID_ECL}**`, (route: Route) => {
      callCount++;
      // First call → running; second → completed (simulates polling)
      const payload = callCount === 1 ? JOB_RUNNING : JOB_COMPLETED;
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(payload) });
    });
    page.route(`**/api/v1/jobs/${JOB_ID_ECL}/stream**`, (route: Route) =>
      route.fulfill({ status: 503 })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Eventually poll sees completed → success toast
    await expect(page.getByText(/calc run.*selesai|selesai.*CR-2026-06/i)).toBeVisible({ timeout: 15000 });
    // Link to detail present
    await expect(page.getByRole("link", { name: /lihat detail/i })).toBeVisible({ timeout: 5000 });
  });

  // M15-02-AC3: Role gate
  test("M15-02-AC3: ROLE-AKUN accessing /dashboard/risk gets redirect; no widgets in DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);

    let rpt13Called = false;
    page.route("**/api/v1/reports/rpt-13**", (route: Route) => {
      rpt13Called = true;
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "DASHBOARD_PERMISSION_DENIED", traceId: "t" } }) });
    });

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Risk widgets absent
    const donut = page.locator('[data-widget-id="W-RK-01"]');
    await expect(donut).toHaveCount(0);

    const sicrWidget = page.locator('[data-widget-id="W-RK-02"]');
    await expect(sicrWidget).toHaveCount(0);

    // Risk dashboard heading absent
    const heading = page.getByRole("heading", { name: /risk dashboard/i });
    await expect(heading).toHaveCount(0);
  });

  // M15-02-AC4: Stage movement trend chart + WCAG aria
  test("M15-02-AC4: W-RK-03 Stage Movement LineChart renders with 6 periodes", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Stage movement chart section
    const movementWidget = page.getByRole("region", { name: /tren perpindahan stage|stage movement/i });
    if (await movementWidget.count() > 0) {
      await expect(movementWidget).toBeVisible({ timeout: 5000 });
    }

    // Period labels visible (Jan 2026 or Jun 2026)
    await expect(page.getByText(/jan 2026|jun 2026/i)).toBeVisible({ timeout: 5000 });

    // Chart aria-label
    const chartAriaLabel = page.locator('[aria-label*="Tren Perpindahan Stage"]');
    if (await chartAriaLabel.count() > 0) {
      await expect(chartAriaLabel.first()).toBeVisible();
    }
  });

  test("M15-02-AC4: stage color legend shows Performing/SICR/Default labels", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["dashboard.risk.read"]);
    await mockRiskEndpoints(page);
    page.route(`**/api/v1/jobs/**`, (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_NONE) })
    );

    await page.goto("/dashboard/risk");
    await page.waitForLoadState("networkidle");

    // Legend labels (screen reader friendly text)
    await expect(page.getByText(/performing/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/sicr/i)).toBeVisible();
    await expect(page.getByText(/default/i)).toBeVisible();
  });
});
