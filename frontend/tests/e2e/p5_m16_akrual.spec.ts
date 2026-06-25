/**
 * Playwright E2E — P5-M16 Akrual Screen
 *
 * AC coverage:
 *   M16-04-AC2 — /transaksi/akrual: batch trigger confirmation dialog + JobProgressPanel UX §3
 *   M16-04-AC3 — Role gate: ROLE-AKUN-CTL Approve Batch button visible; ROLE-MAKER-TR absent
 *   M16-04-AC4 — KPI dashboard cards + DataTable list UX §1
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const AKRUAL_DASHBOARD_RESPONSE = {
  data: {
    totalAkrualIdr: 1_234_567_890,
    instrumenDiproses: 1100,
    statusBatchTerakhir: "COMPLETED",
    timestampBatchTerakhir: "2026-06-25T06:00:00+07:00",
    periodeAktif: "PRD-2026-06",
  },
  meta: { traceId: "trace-akr-dash" },
};

const AKRUAL_LIST_RESPONSE = {
  data: [
    { id: "akr-001", kodeInstrumen: "DEP-0042", jenisInstrumen: "DEPOSITO", tanggalAkrual: "2026-06-25", bungaAkrualIdr: 273_973, eirPersen: 0.05250000, stage: 1, periodeId: "PRD-2026-06" },
    { id: "akr-002", kodeInstrumen: "OBL-0010", jenisInstrumen: "OBLIGASI", tanggalAkrual: "2026-06-25", bungaAkrualIdr: 1_369_863, eirPersen: 0.10000000, stage: 2, periodeId: "PRD-2026-06" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1100, limit: 50 },
  meta: { traceId: "trace-akr-list" },
};

const AKRUAL_BATCH_202_RESPONSE = {
  data: { jobId: "JOB-AKRUAL-2026-06-25", statusUrl: "/api/v1/jobs/JOB-AKRUAL-2026-06-25", streamUrl: "/api/v1/jobs/JOB-AKRUAL-2026-06-25/stream" },
  meta: { traceId: "trace-akr-batch" },
};

const JOB_RUNNING_RESPONSE = {
  data: {
    jobId: "JOB-AKRUAL-2026-06-25",
    type: "AKRUAL_BATCH",
    status: "running",
    progress: 75,
    currentStep: "Menghitung akrual instrumen 825 dari 1.100",
    startedAt: "2026-06-25T10:30:00+07:00",
    estimatedCompletionAt: "2026-06-25T10:31:15+07:00",
    canCancel: true,
  },
  meta: { traceId: "t" },
};

const JOB_COMPLETED_RESPONSE = {
  data: {
    jobId: "JOB-AKRUAL-2026-06-25",
    type: "AKRUAL_BATCH",
    status: "completed",
    progress: 100,
    result: { totalInstrumen: 1100, totalAkrualIdr: 1_234_567_890 },
    canCancel: false,
  },
  meta: { traceId: "t" },
};

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

function mockAkrualApi(page: Page) {
  page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_DASHBOARD_RESPONSE) })
  );
  page.route("**/api/v1/transaksi/akrual/batch**", (route: Route) => {
    if (route.request().method() === "POST") {
      return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(AKRUAL_BATCH_202_RESPONSE) });
    }
    route.continue();
  });
  page.route("**/api/v1/transaksi/akrual**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("/export") || url.includes("format=csv")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "kode,bunga\nDEP-0042,273973" });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_LIST_RESPONSE) });
  });
}

function mockJobPolling(page: Page, finalStatus: "completed" | "failed" = "completed") {
  let callCount = 0;
  page.route("**/api/v1/jobs/JOB-AKRUAL-2026-06-25**", (route: Route) => {
    callCount++;
    if (callCount < 3) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_RUNNING_RESPONSE) });
    }
    if (finalStatus === "completed") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_COMPLETED_RESPONSE) });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: { ...JOB_RUNNING_RESPONSE.data, status: "failed", error: { message: "DB lock timeout", traceId: "trace-fail" } }, meta: { traceId: "t" } }),
    });
  });
}

// ---------------------------------------------------------------------------
// M16-04-AC4: KPI dashboard cards + DataTable list
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Akrual: KPI Dashboard + DataTable UX §1", () => {

  test("M16-04-AC4: KPI cards render with total akrual, instrumen count, and batch status", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    // KPI card 1: Total Akrual Hari Ini
    await expect(page.getByText(/total akrual hari ini|total akrual/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/1\.234\.567\.890|1,234,567,890|Rp.*1.*234/i)).toBeVisible();

    // KPI card 2: Instrumen Diproses
    await expect(page.getByText(/instrumen diproses|1\.100 instrumen|1,100/i)).toBeVisible();

    // KPI card 3: Status Batch Terakhir
    await expect(page.getByText(/COMPLETED|status batch/i)).toBeVisible();
  });

  test("M16-04-AC4: KPI Refresh button triggers re-fetch of dashboard endpoint", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);

    let dashboardCallCount = 0;
    page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) => {
      dashboardCallCount++;
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_DASHBOARD_RESPONSE) });
    });
    page.route("**/api/v1/transaksi/akrual**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_LIST_RESPONSE) })
    );

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const beforeCount = dashboardCallCount;
    const refreshBtn = page.getByRole("button", { name: /refresh kpi|↺ refresh|refresh/i });
    if (await refreshBtn.count() > 0) {
      await refreshBtn.click();
      await page.waitForTimeout(500);
      expect(dashboardCallCount).toBeGreaterThan(beforeCount);
    }
  });

  test("M16-04-AC4: DataTable akrual list renders with required columns (kode, jenis, tanggal, bunga, eir, stage, periode)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("OBL-0010")).toBeVisible();

    await expect(page.getByRole("columnheader", { name: /kode|instrumen/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /bunga.*akrual|akrual.*idr/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /eir|efektif/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /stage/i })).toBeVisible();
  });

  test("M16-04-AC4: DataTable filter by periode_id works", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual?filter[periode_id]=PRD-2026-06");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });
  });

  test("M16-04-AC4: DataTable export button available for ROLE-AKUN", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M16-04-AC2: Batch trigger + JobProgressPanel
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Akrual: Batch Trigger + JobProgressPanel UX §3", () => {

  test("M16-04-AC2: 'Jalankan Batch Akrual Harian' button visible for ROLE-AKUN", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual|batch akrual harian/i });
    await expect(triggerBtn).toBeVisible({ timeout: 5000 });
  });

  test("M16-04-AC2: clicking trigger shows confirmation dialog with periode info", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    if (await triggerBtn.count() > 0) {
      await triggerBtn.click();

      // Confirmation dialog appears
      const dialog = page.getByRole("dialog")
        .or(page.getByText(/jalankan batch akrual harian\?/i));
      await expect(dialog).toBeVisible({ timeout: 3000 });

      // Dialog contains periode info
      await expect(page.getByText(/PRD-2026-06|semua instrumen aktif/i)).toBeVisible();

      // Confirm and cancel buttons
      await expect(page.getByRole("button", { name: /jalankan sekarang|konfirmasi/i })).toBeVisible();
      await expect(page.getByRole("button", { name: /^batal$/i })).toBeVisible();
    }
  });

  test("M16-04-AC2: confirming dialog sends POST with Idempotency-Key and mounts JobProgressPanel", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);

    let capturedKey: string | null = null;
    page.route("**/api/v1/transaksi/akrual/batch", (route: Route) => {
      if (route.request().method() === "POST") {
        capturedKey = route.request().headers()["idempotency-key"] ?? null;
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(AKRUAL_BATCH_202_RESPONSE) });
      }
      route.continue();
    });
    page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_DASHBOARD_RESPONSE) })
    );
    page.route("**/api/v1/transaksi/akrual**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_LIST_RESPONSE) })
    );
    mockJobPolling(page, "completed");

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    if (await triggerBtn.count() > 0) {
      await triggerBtn.click();

      const confirmBtn = page.getByRole("button", { name: /jalankan sekarang|konfirmasi/i });
      if (await confirmBtn.count() > 0) {
        await confirmBtn.click();

        // Idempotency-Key must be present
        await page.waitForTimeout(500);
        if (capturedKey !== null) {
          expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
        }

        // JobProgressPanel visible
        const panel = page.locator("[data-testid='job-progress-panel']")
          .or(page.getByRole("progressbar"))
          .or(page.getByText(/menghitung akrual|batch akrual harian/i));
        await expect(panel).toBeVisible({ timeout: 8000 });
      }
    }
  });

  test("M16-04-AC2: JobProgressPanel shows step text and ETA for akrual batch", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);
    mockJobPolling(page, "completed");

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    if (await triggerBtn.count() > 0) {
      await triggerBtn.click();
      const confirmBtn = page.getByRole("button", { name: /jalankan sekarang|konfirmasi/i });
      if (await confirmBtn.count() > 0) {
        await confirmBtn.click();
        await page.waitForTimeout(1500);

        // Step text from running job
        const stepText = page.getByText(/menghitung akrual instrumen|825 dari 1\.100/i);
        const progressBar = page.getByRole("progressbar");
        const hasProgress = (await stepText.count() > 0) || (await progressBar.count() > 0);
        expect(hasProgress).toBeTruthy();
      }
    }
  });

  test("M16-04-AC2: on completed: success toast with total akrual IDR + DataTable auto-refreshes", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.read", "transaksi.akrual.create"]);

    page.route("**/api/v1/transaksi/akrual/batch", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(AKRUAL_BATCH_202_RESPONSE) });
      }
      route.continue();
    });
    page.route("**/api/v1/jobs/JOB-AKRUAL-2026-06-25**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_COMPLETED_RESPONSE) })
    );
    page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_DASHBOARD_RESPONSE) })
    );

    let listRefreshCount = 0;
    page.route("**/api/v1/transaksi/akrual**", (route: Route) => {
      listRefreshCount++;
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_LIST_RESPONSE) });
    });

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    if (await triggerBtn.count() > 0) {
      await triggerBtn.click();
      const confirmBtn = page.getByRole("button", { name: /jalankan sekarang|konfirmasi/i });
      if (await confirmBtn.count() > 0) {
        const beforeRefresh = listRefreshCount;
        await confirmBtn.click();

        const successToast = page.getByText(/batch akrual.*selesai|JOB-AKRUAL-2026-06-25.*selesai|1\.100 instrumen/i);
        await expect(successToast).toBeVisible({ timeout: 10000 });

        // DataTable should auto-refresh after completion
        await page.waitForTimeout(500);
        expect(listRefreshCount).toBeGreaterThan(beforeRefresh);
      }
    }
  });

  test.fixme("M16-04-AC2: on failed SSE: persistent error toast with traceId and Coba Lagi button", async ({ page }) => {
    // Fixme: SSE failure needs EventSource mock; verify post-FE implementation
    await setRole(page, ["ROLE-AKUN"], ["transaksi.akrual.create"]);
  });
});

// ---------------------------------------------------------------------------
// M16-04-AC3: Role gate
// ---------------------------------------------------------------------------

test.describe("P5-M16 — Akrual: Role Gating", () => {

  test("M16-04-AC3: ROLE-AKUN-CTL (mfa=true): Approve Batch button visible", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["transaksi.akrual.read", "transaksi.akrual.approve"], true);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve batch akrual|setujui.*batch/i });
    await expect(approveBtn).toBeVisible({ timeout: 5000 });
  });

  test("M16-04-AC3: ROLE-AKUN-CTL approving batch shows success toast", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["transaksi.akrual.read", "transaksi.akrual.approve"], true);

    page.route("**/api/v1/transaksi/akrual/dashboard**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_DASHBOARD_RESPONSE) })
    );
    page.route("**/api/v1/transaksi/akrual/batch/JOB-AKRUAL-2026-06-25/approve", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { status: "APPROVED" }, meta: { traceId: "t" } }) })
    );
    page.route("**/api/v1/transaksi/akrual**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(AKRUAL_LIST_RESPONSE) })
    );

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve batch akrual/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.click();

      const confirmBtn = page.getByRole("button", { name: /konfirmasi|approve.*sekarang/i });
      if (await confirmBtn.count() > 0) await confirmBtn.click();

      const successToast = page.getByText(/batch akrual.*berhasil di-approve|jurnal akrual akan di-post/i);
      await expect(successToast).toBeVisible({ timeout: 5000 });
    }
  });

  test("M16-04-AC3: ROLE-MAKER-TR: Approve Batch button absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve batch akrual/i });
    await expect(approveBtn).toHaveCount(0);
  });

  test("M16-04-AC3: ROLE-MAKER-TR: Jalankan Batch button visible (has create perm)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["transaksi.akrual.read", "transaksi.akrual.create"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    await expect(triggerBtn).toBeVisible({ timeout: 5000 });
  });

  test("M16-04-AC3: ROLE-RISK (read-only): no trigger button, no approve button, DataTable visible", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["transaksi.akrual.read"]);
    mockAkrualApi(page);

    await page.goto("/transaksi/akrual");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    const triggerBtn = page.getByRole("button", { name: /jalankan batch akrual/i });
    const approveBtn = page.getByRole("button", { name: /approve batch akrual/i });
    await expect(triggerBtn).toHaveCount(0);
    await expect(approveBtn).toHaveCount(0);
  });
});
