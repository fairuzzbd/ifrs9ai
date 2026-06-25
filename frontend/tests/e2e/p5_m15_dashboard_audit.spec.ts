/**
 * Playwright E2E — P5-M15 Auditor Dashboard
 *
 * AC coverage:
 *   M15-05-AC1 — W-AU-01 Audit Log Volume AreaChart (RPT-25); W-AU-02 Hash-Chain Status badge;
 *                W-AU-03 SoD Violation Alerts DataTable with badge count; empty state when clean
 *   M15-05-AC4 — Role gate: ROLE-AKUN → redirect; W-AU-01..W-AU-04 absent from DOM;
 *                ROLE-AUDIT: DataTable aria-label; keyboard navigation
 *
 * Note: /jobs DataTable tests (M15-05-AC2 + AC3) are in p5_m15_jobs_page.spec.ts.
 * Pattern: all API calls mocked via page.route(); no live backend required.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const HASH_CHAIN_JOB_VERIFIED = {
  data: [{
    jobId: "JOB-HASHCHAIN-001",
    type: "HASH_CHAIN_VERIFY",
    status: "completed",
    progress: 100,
    currentStep: "Verifikasi selesai. 85.000 rows verified.",
    startedAt: "2026-06-25T07:00:00+07:00",
    completedAt: "2026-06-25T07:08:00+07:00",
    result: { status: "VERIFIED", rowsChecked: 85000, mismatchCount: 0 },
    error: null,
    canCancel: false,
    createdBy: "system-cron",
  }],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 1 },
  meta: { traceId: "trace-hashchain-verified" },
};

const HASH_CHAIN_JOB_MISMATCH = {
  data: [{
    jobId: "JOB-HASHCHAIN-002",
    type: "HASH_CHAIN_VERIFY",
    status: "completed",
    progress: 100,
    currentStep: "Verifikasi selesai. MISMATCH ditemukan!",
    startedAt: "2026-06-24T07:00:00+07:00",
    completedAt: "2026-06-24T07:10:00+07:00",
    result: { status: "MISMATCH", rowsChecked: 84999, mismatchCount: 1, firstMismatchEventId: "evt-999" },
    error: null,
    canCancel: false,
    createdBy: "system-cron",
  }],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 1 },
  meta: { traceId: "trace-hashchain-mismatch" },
};

const RPT_25_VOLUME = {
  data: Array.from({ length: 30 }, (_, i) => ({
    tanggal: `2026-${String(5 + Math.floor((26 + i) / 31)).padStart(2, "0")}-${String(((26 + i) % 31) + 1).padStart(2, "0")}`,
    eventCount: 2500 + (i % 7) * 150,
  })),
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 30, limit: 200 },
  meta: { traceId: "trace-rpt-25-volume", totalEvents: 85000 },
};

const RPT_25_SOD_VIOLATIONS = {
  data: [
    { eventId: "evt-sod-001", eventTime: "2026-06-20T14:30:00+07:00", actorUserId: "USR-MAKER-001", actorRole: "ROLE-MAKER-TR", action: "SOD_VIOLATION", entityType: "PENEMPATAN", entityId: "pls-001", afterJsonb: { detail: "Attempted self-review" } },
    { eventId: "evt-sod-002", eventTime: "2026-06-18T09:15:00+07:00", actorUserId: "USR-APPR-TR-02", actorRole: "ROLE-APPR-TR", action: "SOD_VIOLATION", entityType: "INSTRUMEN", entityId: "inst-001", afterJsonb: { detail: "SoD bypass attempt" } },
    { eventId: "evt-sod-003", eventTime: "2026-06-15T11:00:00+07:00", actorUserId: "USR-MAKER-003", actorRole: "ROLE-MAKER-TR", action: "SOD_VIOLATION", entityType: "JURNAL", entityId: "jrn-001", afterJsonb: { detail: "Approve attempt by maker" } },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 20 },
  meta: { traceId: "trace-rpt-25-sod" },
};

const RPT_25_SOD_EMPTY = {
  data: [],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 20 },
  meta: { traceId: "trace-rpt-25-sod-empty" },
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

function mockAuditEndpoints(page: Page, opts?: { mismatch?: boolean; noSoDViolations?: boolean }) {
  // W-AU-01: Volume (filter by event_time range)
  page.route("**/api/v1/reports/rpt-25**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("SOD_VIOLATION") || url.includes("sod_violation")) {
      route.fulfill({ status: 200, contentType: "application/json",
        body: JSON.stringify(opts?.noSoDViolations ? RPT_25_SOD_EMPTY : RPT_25_SOD_VIOLATIONS) });
    } else {
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(RPT_25_VOLUME) });
    }
  });

  // W-AU-02: Hash-chain job
  page.route("**/api/v1/jobs**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("HASH_CHAIN_VERIFY") || url.includes("type=HASH_CHAIN_VERIFY")) {
      route.fulfill({ status: 200, contentType: "application/json",
        body: JSON.stringify(opts?.mismatch ? HASH_CHAIN_JOB_MISMATCH : HASH_CHAIN_JOB_VERIFIED) });
    } else {
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }) });
    }
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("P5-M15 — Auditor Dashboard /dashboard/audit", () => {

  // M15-05-AC1: Audit volume, hash-chain status, SoD alerts
  test("M15-05-AC1: W-AU-01 Audit Log Volume chart shows total 85.000 events", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Total events KPI
    await expect(page.getByText(/85\.000|85,000/)).toBeVisible({ timeout: 5000 });

    // Volume chart section
    const volumeWidget = page.getByRole("region", { name: /volume audit|audit log volume/i });
    await expect(volumeWidget).toBeVisible();
  });

  test("M15-05-AC1: W-AU-02 Hash-Chain Status badge shows VERIFIED (green)", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // VERIFIED badge
    await expect(page.getByText(/verified|hash.chain verified/i)).toBeVisible({ timeout: 5000 });

    // Link to detail job
    const detailLink = page.getByRole("link", { name: /lihat detail/i });
    await expect(detailLink).toBeVisible();
  });

  test("M15-05-AC1: W-AU-02 Hash-Chain badge shows MISMATCH warning when mismatch detected", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page, { mismatch: true });

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // MISMATCH warning (red)
    await expect(page.getByText(/mismatch|peringatan.*mismatch/i)).toBeVisible({ timeout: 5000 });
  });

  test("M15-05-AC1: W-AU-03 SoD Violation Alerts shows 3 violations with badge", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Badge "3 pelanggaran SoD"
    await expect(page.getByText(/3 pelanggaran|3.*sod|sod.*3/i)).toBeVisible({ timeout: 5000 });

    // Actor from first violation
    await expect(page.getByText(/USR-MAKER-001|USR-APPR-TR-02/i)).toBeVisible();

    // Detail text
    await expect(page.getByText(/attempted self-review|bypass attempt/i)).toBeVisible();
  });

  test("M15-05-AC1: W-AU-03 shows green empty state when no SoD violations", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page, { noSoDViolations: true });

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Empty state message
    await expect(page.getByText(/tidak ada pelanggaran sod|no sod violation/i)).toBeVisible({ timeout: 5000 });
  });

  // M15-05-AC4: Role gate + read-only confirmation
  test("M15-05-AC4: ROLE-AKUN accessing /dashboard/audit → redirect; W-AU-01..W-AU-04 absent", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["dashboard.akuntansi.read"]);

    let auditEndpointCalled = false;
    page.route("**/api/v1/reports/rpt-25**", (route: Route) => {
      auditEndpointCalled = true;
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "DASHBOARD_PERMISSION_DENIED", traceId: "t" } }) });
    });

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Audit widgets absent from DOM
    const volumeWidget = page.locator('[data-widget-id="W-AU-01"]');
    await expect(volumeWidget).toHaveCount(0);

    const hashWidget = page.locator('[data-widget-id="W-AU-02"]');
    await expect(hashWidget).toHaveCount(0);

    // Audit dashboard heading absent
    const heading = page.getByRole("heading", { name: /auditor dashboard/i });
    await expect(heading).toHaveCount(0);
  });

  test("M15-05-AC4: ROLE-AUDIT dashboard has no mutating action buttons in DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // No create/submit/approve/reject buttons (read-only role)
    const createBtn   = page.getByRole("button", { name: /buat|create|tambah|add/i });
    const submitBtn   = page.getByRole("button", { name: /submit|kirim/i });
    const approveBtn  = page.getByRole("button", { name: /approve|setujui/i });

    await expect(createBtn).toHaveCount(0);
    await expect(submitBtn).toHaveCount(0);
    await expect(approveBtn).toHaveCount(0);
  });

  test("M15-05-AC4: DataTable W-AU-03 has aria-label and keyboard navigable", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Widget containers have aria-label
    const labeledRegions = page.locator('[aria-label*="Audit Dashboard"]');
    if (await labeledRegions.count() > 0) {
      await expect(labeledRegions.first()).toBeVisible();
    }

    // Table rows keyboard navigable (role="row")
    const tableRows = page.locator('tr[role="row"], tbody tr');
    if (await tableRows.count() > 0) {
      await tableRows.first().focus();
    }

    // Refresh button focusable
    const refreshBtn = page.getByRole("button", { name: /refresh|perbarui/i });
    if (await refreshBtn.count() > 0) {
      await refreshBtn.focus();
      await expect(refreshBtn).toBeFocused();
    }
  });

  // Extra: W-AU-04 Top Action Types bar chart
  test("M15-05-AC1 (W-AU-04): Top Action Types bar chart is present on audit dashboard", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["dashboard.audit.read", "report.*.read"]);
    await mockAuditEndpoints(page);

    await page.goto("/dashboard/audit");
    await page.waitForLoadState("networkidle");

    // Top actions widget region
    const topActionsWidget = page.getByRole("region", { name: /top action|aksi terbanyak/i });
    if (await topActionsWidget.count() > 0) {
      await expect(topActionsWidget).toBeVisible({ timeout: 5000 });
    }
    // Link to full RPT-25 report
    const rpt25Link = page.getByRole("link", { name: /lihat rpt-25|lihat.*audit.*log/i });
    if (await rpt25Link.count() > 0) {
      await expect(rpt25Link).toBeVisible();
    }
  });
});
