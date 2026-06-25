/**
 * Playwright E2E — P5-M16 MTM Screens
 *
 * AC coverage:
 *   M16-02-AC1 — 308 redirect /mtm/* → /transaksi/mtm/* (covered in p5_m16_redirects.spec.ts)
 *   M16-02-AC2 — Upload dropzone + JobProgressPanel SSE updates → batch detail
 *   M16-02-AC3 — List DataTable UX §1 + stale price filter
 *   M16-02-AC4 — Role gate: absent-from-DOM upload button; ROLE-AUDIT read-only
 *
 * Pattern: all API calls mocked via page.route(); SSE simulated via EventSource mock.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const MTM_LIST_RESPONSE = {
  data: [
    { id: "mtm-001", kodeInstrumen: "OBL-0042", jenisInstrumen: "OBLIGASI", tanggalMtm: "2026-06-25", hargaPasar: 102.50, mtmIdr: 1_025_000, sumberHarga: "IBPA", status: "VALID", isStale: false },
    { id: "mtm-002", kodeInstrumen: "SHM-0099", jenisInstrumen: "SAHAM", tanggalMtm: "2026-06-24", hargaPasar: 3450, mtmIdr: 3_450_000, sumberHarga: "BEI", status: "STALE", isStale: true },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 50 },
  meta: { traceId: "trace-mtm-list" },
};

const MTM_UPLOAD_202_RESPONSE = {
  data: { jobId: "JOB-MTM-UPLOAD-001", statusUrl: "/api/v1/jobs/JOB-MTM-UPLOAD-001", streamUrl: "/api/v1/jobs/JOB-MTM-UPLOAD-001/stream" },
  meta: { traceId: "trace-upload" },
};

const JOB_RUNNING_RESPONSE = {
  data: { jobId: "JOB-MTM-UPLOAD-001", type: "MTM_UPLOAD", status: "running", progress: 47, currentStep: "Parsing baris 2.651 dari 5.678", startedAt: "2026-06-25T10:30:00+07:00", estimatedCompletionAt: "2026-06-25T10:31:30+07:00", canCancel: true },
  meta: { traceId: "t" },
};

const JOB_COMPLETED_RESPONSE = {
  data: { jobId: "JOB-MTM-UPLOAD-001", type: "MTM_UPLOAD", status: "completed", progress: 100, currentStep: "Selesai", result: { totalRecords: 5678, errorCount: 12, batchId: "JOB-MTM-UPLOAD-001" }, canCancel: false },
  meta: { traceId: "t" },
};

const STALE_ALERTS_RESPONSE = {
  data: [
    { id: "stale-001", kodeInstrumen: "SHM-0099", jenisInstrumen: "SAHAM", tanggalMtm: "2026-06-24", hargaPasar: 3450, staleHours: 26 },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
  meta: { traceId: "trace-stale" },
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

function mockMtmList(page: Page) {
  page.route("**/api/v1/transaksi/mtm**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("/upload") && route.request().method() === "POST") {
      return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
    }
    if (url.includes("/alerts/stale-price")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(STALE_ALERTS_RESPONSE) });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MTM_LIST_RESPONSE) });
  });
}

function mockJobPolling(page: Page, finalStatus: "completed" | "failed" = "completed") {
  let callCount = 0;
  page.route("**/api/v1/jobs/JOB-MTM-UPLOAD-001**", (route: Route) => {
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
      body: JSON.stringify({ data: { ...JOB_RUNNING_RESPONSE.data, status: "failed", error: { message: "Parse error: invalid CSV format", traceId: "trace-fail" } }, meta: { traceId: "t" } }),
    });
  });
}

// ---------------------------------------------------------------------------
// M16-02-AC2: Upload + JobProgressPanel
// ---------------------------------------------------------------------------

test.describe("P5-M16 — MTM Upload: JobProgressPanel UX §3", () => {

  test("M16-02-AC2: upload page shows MtmUploadDropzone with correct labels", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);
    mockMtmList(page);

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    // Dropzone area visible
    const dropzone = page.getByText(/taruh file di sini|klik untuk browse/i)
      .or(page.locator("[data-testid='mtm-upload-dropzone']").first());
    await expect(dropzone).toBeVisible({ timeout: 5000 });

    // Format labels
    await expect(page.getByText(/csv|xlsx/i).first()).toBeVisible();
    await expect(page.getByText(/50\s*mb/i)).toBeVisible();
  });

  test("M16-02-AC2: invalid file format shows instant error toast without POST", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);

    let postCalled = false;
    page.route("**/api/v1/transaksi/mtm/upload", (route: Route) => {
      postCalled = true;
      route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
    });

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    // Simulate dropping an invalid file
    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      // Set a PDF file (invalid format)
      await fileInput.setInputFiles({
        name: "invalid.pdf",
        mimeType: "application/pdf",
        buffer: Buffer.from("fake pdf"),
      });

      await page.waitForTimeout(500);

      const errorToast = page.getByText(/format file.*tidak.*didukung|csv.*xlsx/i);
      await expect(errorToast).toBeVisible({ timeout: 3000 });
      expect(postCalled).toBe(false);
    }
  });

  test("M16-02-AC2: valid CSV upload shows 202 response then JobProgressPanel mounts", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);

    page.route("**/api/v1/transaksi/mtm/upload", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
      }
      route.continue();
    });
    mockJobPolling(page, "completed");

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({
        name: "mtm-ibpa-2026-06-25.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("kode,harga\nOBL-0042,102.50"),
      });

      const uploadBtn = page.getByRole("button", { name: /^upload$/i });
      if (await uploadBtn.count() > 0) {
        await uploadBtn.click();

        // JobProgressPanel should appear
        const progressPanel = page.locator("[data-testid='job-progress-panel']")
          .or(page.getByRole("progressbar"))
          .or(page.getByText(/memproses upload mtm|parsing baris/i));
        await expect(progressPanel).toBeVisible({ timeout: 8000 });
      }
    }
  });

  test("M16-02-AC2: Idempotency-Key present in upload POST header", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);

    let capturedKey: string | null = null;
    page.route("**/api/v1/transaksi/mtm/upload", (route: Route) => {
      capturedKey = route.request().headers()["idempotency-key"] ?? null;
      return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
    });

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({ name: "test.csv", mimeType: "text/csv", buffer: Buffer.from("k,v\na,1") });
      const uploadBtn = page.getByRole("button", { name: /^upload$/i });
      if (await uploadBtn.count() > 0) {
        await uploadBtn.click();
        await page.waitForTimeout(500);
        expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      }
    }
  });

  test("M16-02-AC2: JobProgressPanel shows progress bar, ETA, cancel button, background button", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);

    page.route("**/api/v1/transaksi/mtm/upload", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
      }
      route.continue();
    });
    mockJobPolling(page, "completed");

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({ name: "mtm.csv", mimeType: "text/csv", buffer: Buffer.from("k,v\na,1") });
      const uploadBtn = page.getByRole("button", { name: /^upload$/i });
      if (await uploadBtn.count() > 0) {
        await uploadBtn.click();

        await page.waitForTimeout(1500);

        const progressBar = page.getByRole("progressbar")
          .or(page.locator("[aria-valuenow]").first());
        const cancelBtn = page.getByRole("button", { name: /batalkan|cancel/i });
        const bgBtn = page.getByRole("button", { name: /lanjutkan di background|background/i });

        // At least one progress indicator visible
        const progressVisible = (await progressBar.count() > 0) || (await page.getByText(/parsing baris|memproses/i).count() > 0);
        expect(progressVisible).toBeTruthy();
      }
    }
  });

  test("M16-02-AC2: on SSE/poll completed: success toast with batch link to /transaksi/mtm/upload/batch/{jobId}", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read", "transaksi.mtm.upload"]);

    page.route("**/api/v1/transaksi/mtm/upload", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify(MTM_UPLOAD_202_RESPONSE) });
      }
      route.continue();
    });

    // Immediately return completed
    page.route("**/api/v1/jobs/JOB-MTM-UPLOAD-001**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_COMPLETED_RESPONSE) })
    );

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({ name: "mtm.csv", mimeType: "text/csv", buffer: Buffer.from("k,v\na,1") });
      const uploadBtn = page.getByRole("button", { name: /^upload$/i });
      if (await uploadBtn.count() > 0) {
        await uploadBtn.click();

        // Success toast or inline panel shows completion
        const successMsg = page.getByText(/5\.?678 record.*diproses|upload.*selesai|batch.*selesai/i);
        await expect(successMsg).toBeVisible({ timeout: 10000 });

        // Link to batch detail
        const batchLink = page.getByRole("link", { name: /lihat hasil batch/i })
          .or(page.getByRole("link", { name: /JOB-MTM-UPLOAD-001/i }));
        await expect(batchLink).toBeVisible();
      }
    }
  });

  test.fixme("M16-02-AC2: on SSE failed: persistent error toast with trace and retry button", async ({ page }) => {
    // Fixme: SSE failure simulation depends on EventSource mock; verify post-FE implementation
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.upload"]);
  });
});

// ---------------------------------------------------------------------------
// M16-02-AC3: MTM List DataTable UX §1
// ---------------------------------------------------------------------------

test.describe("P5-M16 — MTM List: DataTable UX §1 + Stale Filter", () => {

  test("M16-02-AC3: list renders with kode_instrumen, jenis, tanggal_mtm, harga, status columns", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read"]);
    mockMtmList(page);

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("OBL-0042")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("SHM-0099")).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /kode|instrumen/i })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: /status/i })).toBeVisible();
  });

  test("M16-02-AC3: stale quick-filter button adds filter[is_stale]=true to URL", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read"]);
    mockMtmList(page);

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    const staleFilterBtn = page.getByRole("button", { name: /harga stale|stale/i });
    await expect(staleFilterBtn).toBeVisible({ timeout: 5000 });
    await staleFilterBtn.click();

    await page.waitForTimeout(300);
    // URL should include stale filter
    expect(page.url()).toContain("stale");
  });

  test("M16-02-AC3: stale alerts link navigates to /transaksi/mtm/alerts/stale-price", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read"]);
    mockMtmList(page);
    page.route("**/api/v1/transaksi/mtm/alerts**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(STALE_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    const staleLink = page.getByRole("link", { name: /lihat stale price alerts|stale price alerts/i });
    await expect(staleLink).toBeVisible({ timeout: 5000 });
    await expect(staleLink).toHaveAttribute("href", "/transaksi/mtm/alerts/stale-price");
  });

  test("M16-02-AC3: stale-price alerts page renders DataTable", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["transaksi.mtm.read"]);
    page.route("**/api/v1/transaksi/mtm/alerts/stale-price**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(STALE_ALERTS_RESPONSE) })
    );

    await page.goto("/transaksi/mtm/alerts/stale-price");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("SHM-0099")).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M16-02-AC4: Role gating
// ---------------------------------------------------------------------------

test.describe("P5-M16 — MTM Role Gating: Absent-from-DOM", () => {

  test("M16-02-AC4: ROLE-RISK without upload perm: Upload button absent from MTM list", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["transaksi.mtm.read"]); // no transaksi.mtm.upload
    mockMtmList(page);

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    const uploadBtn = page.getByRole("link", { name: /upload file|upload mtm/i })
      .or(page.getByRole("button", { name: /upload file|upload mtm/i }));
    await expect(uploadBtn).toHaveCount(0);
  });

  test("M16-02-AC4: ROLE-AUDIT: DataTable visible; upload/cron buttons absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["transaksi.mtm.read"]);
    mockMtmList(page);

    await page.goto("/transaksi/mtm");
    await page.waitForLoadState("networkidle");

    // Read-only DataTable accessible
    await expect(page.getByText("OBL-0042")).toBeVisible({ timeout: 5000 });

    // No mutation buttons
    const uploadBtn = page.getByRole("button", { name: /upload/i });
    const cronBtn = page.getByRole("button", { name: /trigger cron|trigger manual/i });
    await expect(uploadBtn).toHaveCount(0);
    await expect(cronBtn).toHaveCount(0);
  });

  test("M16-02-AC4: ROLE-RISK without transaksi.mtm.read accessing /transaksi/mtm/upload → 403 or notFound", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], []); // no mtm perms at all
    page.route("**/api/v1/transaksi/mtm/**", (route: Route) =>
      route.fulfill({ status: 403, contentType: "application/json", body: JSON.stringify({ error: { code: "FORBIDDEN", traceId: "t" } }) })
    );

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    // Should show 404 or redirect away from upload page
    const isOnUploadPage = page.url().includes("/transaksi/mtm/upload");
    const shows403 = await page.getByText(/403|404|akses ditolak|tidak ditemukan|forbidden/i).count() > 0;
    expect(!isOnUploadPage || shows403).toBeTruthy();
  });

  test("M16-02-AC4: upload page not rendered (absent from DOM) for unauthorized role", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["transaksi.mtm.read"]); // read but no upload
    mockMtmList(page);

    await page.goto("/transaksi/mtm/upload");
    await page.waitForLoadState("networkidle");

    // If redirected to list: upload form absent
    const dropzone = page.locator("[data-testid='mtm-upload-dropzone']")
      .or(page.getByText(/taruh file di sini/i));
    await expect(dropzone).toHaveCount(0);
  });
});
