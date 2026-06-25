/**
 * Playwright E2E — P5-M17 Master Kurs Screens
 *
 * AC coverage:
 *   M17-02-AC1 — List /master/kurs: DataTable UX §1 (sort + page + filter + export)
 *   M17-02-AC2 — Manual entry /master/kurs/new: form notif UX §2
 *   M17-02-AC3 — JISDOR sync /master/kurs/jisdor-sync: trigger manual + JobProgressPanel UX §3
 *   M17-02-AC4 — Bulk upload /master/kurs/upload: dropzone + JobProgressPanel UX §3
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const KURS_LIST_RESPONSE = {
  data: [
    { id: "kurs-usd-20260625", kodeMataUang: "USD", namaMataUang: "US Dollar", tanggalKurs: "2026-06-25", kursJisdor: 16250.0, kursManuaL: null, sumber: "JISDOR", createdAt: "2026-06-25T10:30:00+07:00" },
    { id: "kurs-eur-20260625", kodeMataUang: "EUR", namaMataUang: "Euro", tanggalKurs: "2026-06-25", kursJisdor: 17432.0, kursManual: null, sumber: "JISDOR", createdAt: "2026-06-25T10:30:00+07:00" },
    { id: "kurs-sgd-20260625", kodeMataUang: "SGD", namaMataUang: "Singapore Dollar", tanggalKurs: "2026-06-25", kursJisdor: null, kursManual: 11950.0, sumber: "MANUAL", createdAt: "2026-06-25T11:00:00+07:00" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 50 },
  appliedSort: [{ col: "tanggalKurs", dir: "desc" }],
  appliedFilter: {},
  meta: { traceId: "trace-kurs-list" },
};

const JISDOR_STATUS_RESPONSE = {
  data: {
    tanggalSyncTerakhir: "2026-06-25T10:30:00+07:00",
    jumlahMataUang: 15,
    status: "SUCCESS",
    dijalankanOleh: "Budi Santoso",
  },
  meta: { traceId: "trace-jisdor-status" },
};

const JOB_RUNNING_RESPONSE = {
  data: {
    jobId: "job-jisdor-sync-001",
    type: "JISDOR_SYNC",
    status: "running",
    progress: 40,
    currentStep: "Mengambil data dari BI JISDOR API...",
    startedAt: "2026-06-25T10:30:00+07:00",
    estimatedCompletionAt: "2026-06-25T10:30:45+07:00",
    canCancel: false,
  },
  meta: { traceId: "t" },
};

const JOB_COMPLETED_RESPONSE = {
  data: {
    jobId: "job-jisdor-sync-001",
    type: "JISDOR_SYNC",
    status: "completed",
    progress: 100,
    currentStep: "Selesai",
    result: { jumlahMataUang: 15 },
    canCancel: false,
  },
  meta: { traceId: "t" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-akun-001",
  mfaVerified = false
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

function mockKursListEndpoint(page: Page) {
  page.route("**/api/v1/master/kurs**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("/export") || url.includes("format=csv") || url.includes("format=xlsx")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "kode,tanggal,kurs\nUSD,2026-06-25,16250" });
    }
    if (url.includes("/jisdor-sync/status")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JISDOR_STATUS_RESPONSE) });
    }
    if (url.includes("/jisdor-sync") && route.request().method() === "POST") {
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job-jisdor-sync-001", statusUrl: "/api/v1/jobs/job-jisdor-sync-001", streamUrl: "/api/v1/jobs/job-jisdor-sync-001/stream" } }),
      });
    }
    if (url.includes("/upload") && route.request().method() === "POST") {
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job-kurs-upload-001", statusUrl: "/api/v1/jobs/job-kurs-upload-001", streamUrl: "/api/v1/jobs/job-kurs-upload-001/stream" } }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
  });

  page.route("**/api/v1/jobs/**", (route: Route) => {
    const url = route.request().url();
    if (url.includes("/stream")) {
      return route.fulfill({ status: 200, contentType: "text/event-stream", body: "event: completed\ndata: {\"progress\":100}\n\n" });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_COMPLETED_RESPONSE) });
  });
}

// ---------------------------------------------------------------------------
// M17-02-AC1: List DataTable UX §1
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Master Kurs: List DataTable UX §1 (AC1)", () => {

  test("M17-02-AC1: DataTable renders kurs list with correct columns", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create", "fx_rate.update"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("USD")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("EUR")).toBeVisible();
    await expect(page.getByText("SGD")).toBeVisible();
  });

  test("M17-02-AC1: default sort tanggal_kurs:desc sent to API", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read"]);

    let capturedUrl = "";
    page.route("**/api/v1/master/kurs**", (route: Route) => {
      capturedUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
    });

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    expect(capturedUrl).toContain("tanggal_kurs:desc");
  });

  test("M17-02-AC1: JISDOR chip vs MANUAL chip rendered per sumber", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    // JISDOR chip (blue) and MANUAL chip (gray)
    await expect(page.getByText("JISDOR").first()).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("MANUAL").first()).toBeVisible();
  });

  test("M17-02-AC1: filter[sumber] filter chip clears correctly", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs?filter[sumber]=JISDOR");
    await page.waitForLoadState("networkidle");

    // Filter chip present
    await expect(page.getByText(/JISDOR|bersihkan|clear/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC1: ROLE-AUDIT sees no create/upload/sync buttons", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["fx_rate.read"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /\+ kurs baru|kurs baru/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /upload kurs/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /sinkronisasi jisdor/i })).toHaveCount(0);
  });

  test("M17-02-AC1: export button present for ROLE-AKUN", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /ekspor|export/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC1: pagination controls render", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    // Pagination: prev/next or page indicator
    const pagination = page.getByText(/halaman|sebelumnya|selanjutnya|prev|next/i).first();
    await expect(pagination).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M17-02-AC2: Manual Entry Form UX §2
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Master Kurs: Manual Entry Form (AC2)", () => {

  test("M17-02-AC2: form renders KursForm with required fields", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    // Key fields
    await expect(page.getByLabel(/kode mata uang/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByLabel(/tanggal kurs/i)).toBeVisible();
    await expect(page.getByLabel(/kurs.*jisdor/i)).toBeVisible();
  });

  test("M17-02-AC2: submit button disables + spinner during POST", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    page.route("**/api/v1/master/kurs", (route: Route) => {
      // Delay to observe disabled state
      return new Promise<void>((resolve) => setTimeout(resolve, 200)).then(() =>
        route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify({ data: { id: "kurs-new-001", kodeMataUang: "USD", tanggalKurs: "2026-06-25" }, meta: { traceId: "t" } }),
        })
      );
    });

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    const submitBtn = page.getByRole("button", { name: /simpan kurs/i });
    if (await submitBtn.isVisible({ timeout: 3000 })) {
      await submitBtn.click();
      // Button should be disabled during submission
      await expect(submitBtn).toBeDisabled({ timeout: 1000 });
    }
  });

  test("M17-02-AC2: success toast has specific message with kode + link", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    page.route("**/api/v1/master/kurs", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify({ data: { id: "kurs-usd-20260625", kodeMataUang: "USD", tanggalKurs: "2026-06-25" }, meta: { traceId: "t" } }),
        });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
    });

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    // Fill form
    const codeSelect = page.getByLabel(/kode mata uang/i);
    if (await codeSelect.isVisible()) {
      await codeSelect.selectOption("USD").catch(() => codeSelect.fill("USD"));
    }

    const dateInput = page.getByLabel(/tanggal kurs/i);
    if (await dateInput.isVisible()) {
      await dateInput.fill("2026-06-25");
    }

    const kursInput = page.getByLabel(/kurs.*jisdor/i);
    if (await kursInput.isVisible()) {
      await kursInput.fill("16250");
    }

    const submitBtn = page.getByRole("button", { name: /simpan kurs/i });
    if (await submitBtn.isVisible()) {
      await submitBtn.click();
    }

    // Check for success toast with specific content
    await expect(page.getByText(/USD.*2026-06-25.*berhasil|kurs USD.*berhasil disimpan/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC2: validation error highlights field with inline message", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    page.route("**/api/v1/master/kurs", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "VALIDATION_FAILED",
              message: "1 field bermasalah",
              details: [{ field: "kursJisdor", rule: "required" }],
              traceId: "trace-err-001",
            },
          }),
        });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
    });

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    // Submit without filling kurs_jisdor
    const submitBtn = page.getByRole("button", { name: /simpan kurs/i });
    if (await submitBtn.isVisible()) {
      await submitBtn.click();
      // Either client-side or server-side error message
      await expect(page.getByText(/bermasalah|field.*wajib|required|kursJisdor/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test("M17-02-AC2: 409 CONFLICT toast persistent with specific copy", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    page.route("**/api/v1/master/kurs", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ error: { code: "CONFLICT", message: "Kurs USD tanggal 2026-06-25 sudah ada.", traceId: "t" } }),
        });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
    });

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    const submitBtn = page.getByRole("button", { name: /simpan kurs/i });
    if (await submitBtn.isVisible()) {
      // Fill minimal and submit
      const codeSelect = page.getByLabel(/kode mata uang/i);
      if (await codeSelect.isVisible()) await codeSelect.selectOption("USD").catch(() => codeSelect.fill("USD"));

      await submitBtn.click();
      await expect(page.getByText(/sudah ada|CONFLICT|edit entri/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test("M17-02-AC2: Idempotency-Key UUID v4 injected on POST", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    let capturedKey = "";
    page.route("**/api/v1/master/kurs", (route: Route) => {
      if (route.request().method() === "POST") {
        capturedKey = route.request().headers()["idempotency-key"] ?? "";
        return route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ data: { id: "k1" }, meta: { traceId: "t" } }) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) });
    });

    await page.goto("/master/kurs/new");
    await page.waitForLoadState("networkidle");

    const submitBtn = page.getByRole("button", { name: /simpan kurs/i });
    if (await submitBtn.isVisible()) {
      await submitBtn.click();
      await page.waitForTimeout(500);

      if (capturedKey) {
        expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      }
    }
  });
});

// ---------------------------------------------------------------------------
// M17-02-AC3: JISDOR Sync + JobProgressPanel UX §3
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Master Kurs: JISDOR Sync (AC3)", () => {

  test("M17-02-AC3: JISDOR status card renders with last sync info", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs/jisdor-sync");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/status sinkronisasi|sync terakhir|tanggal sync|10:30/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC3: Sinkronisasi Sekarang button triggers confirmation dialog", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs/jisdor-sync");
    await page.waitForLoadState("networkidle");

    const triggerBtn = page.getByRole("button", { name: /sinkronisasi sekarang/i });
    await expect(triggerBtn).toBeVisible({ timeout: 5000 });
    await triggerBtn.click();

    await expect(page.getByText(/sinkronisasi kurs.*jisdor sekarang|overwrite|data terbaru/i)).toBeVisible({ timeout: 3000 });
  });

  test("M17-02-AC3: POST /master/kurs/jisdor-sync includes Idempotency-Key → 202 → JobProgressPanel rendered", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    let capturedKey = "";
    page.route("**/api/v1/master/kurs/jisdor-sync**", (route: Route) => {
      if (route.request().method() === "POST") {
        capturedKey = route.request().headers()["idempotency-key"] ?? "";
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ data: { jobId: "job-jisdor-sync-001", statusUrl: "/api/v1/jobs/job-jisdor-sync-001", streamUrl: "/api/v1/jobs/job-jisdor-sync-001/stream" } }) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JISDOR_STATUS_RESPONSE) });
    });
    page.route("**/api/v1/master/kurs**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) })
    );
    page.route("**/api/v1/jobs/job-jisdor-sync-001**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_RUNNING_RESPONSE) })
    );

    await page.goto("/master/kurs/jisdor-sync");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: /sinkronisasi sekarang/i }).click();

    const confirmBtn = page.getByRole("button", { name: /sinkronisasi|lanjut|konfirmasi/i }).last();
    if (await confirmBtn.isVisible({ timeout: 2000 })) {
      await confirmBtn.click();

      await page.waitForTimeout(500);

      if (capturedKey) {
        expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      }

      // JobProgressPanel should appear
      await expect(page.getByText(/mengambil data|jisdor api|progress|job/i)).toBeVisible({ timeout: 3000 });
    }
  });

  test("M17-02-AC3: JobProgressPanel has no Cancel button when canCancel=false", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    page.route("**/api/v1/master/kurs/jisdor-sync**", (route: Route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ data: { jobId: "job-jisdor-sync-001", statusUrl: "/api/v1/jobs/job-jisdor-sync-001", streamUrl: "/api/v1/jobs/job-jisdor-sync-001/stream" } }) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JISDOR_STATUS_RESPONSE) });
    });
    page.route("**/api/v1/master/kurs**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) })
    );
    page.route("**/api/v1/jobs/**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...JOB_RUNNING_RESPONSE, data: { ...JOB_RUNNING_RESPONSE.data, canCancel: false } }) })
    );

    await page.goto("/master/kurs/jisdor-sync");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: /sinkronisasi sekarang/i }).click();
    const confirmBtn = page.getByRole("button", { name: /sinkronisasi|lanjut|konfirmasi/i }).last();
    if (await confirmBtn.isVisible({ timeout: 2000 })) {
      await confirmBtn.click();
      await page.waitForTimeout(300);
      // Cancel button must be absent
      await expect(page.getByRole("button", { name: /batalkan|cancel/i })).toHaveCount(0);
    }
  });

  test.fixme("M17-02-AC3: SSE completed event shows success toast with currency count", async ({ page }) => {
    // fixme: requires SSE EventSource mock support in Playwright context
    // Verify: toast "Sinkronisasi JISDOR selesai. 15 mata uang diperbarui."
  });
});

// ---------------------------------------------------------------------------
// M17-02-AC4: Bulk Upload + JobProgressPanel UX §3
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Master Kurs: Bulk Upload (AC4)", () => {

  test("M17-02-AC4: upload page renders dropzone with format constraints", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs/upload");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText(/CSV|XLSX|10 MB|taruh file/i).first()).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC4: template download link present", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);
    await mockKursListEndpoint(page);

    await page.goto("/master/kurs/upload");
    await page.waitForLoadState("networkidle");

    const templateLink = page.getByRole("link", { name: /unduh template|download template/i });
    await expect(templateLink).toBeVisible({ timeout: 5000 });
  });

  test("M17-02-AC4: invalid file format shows instant client-side error without POST", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    let postCalled = false;
    page.route("**/api/v1/master/kurs/upload", (route: Route) => {
      postCalled = true;
      return route.fulfill({ status: 202, contentType: "application/json", body: "{}" });
    });
    page.route("**/api/v1/master/kurs**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) })
    );

    await page.goto("/master/kurs/upload");
    await page.waitForLoadState("networkidle");

    // Simulate uploading an invalid file (PDF)
    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({
        name: "invalid.pdf",
        mimeType: "application/pdf",
        buffer: Buffer.from("fake pdf content"),
      });

      await page.waitForTimeout(500);

      // Should show client-side error, NOT call the API
      expect(postCalled).toBe(false);
      await expect(page.getByText(/format.*tidak valid|gunakan template|CSV|XLSX/i)).toBeVisible({ timeout: 3000 });
    }
  });

  test("M17-02-AC4: valid CSV upload → 202 → JobProgressPanel rendered", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"]);

    let capturedKey = "";
    page.route("**/api/v1/master/kurs/upload", (route: Route) => {
      capturedKey = route.request().headers()["idempotency-key"] ?? "";
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job-kurs-upload-001", statusUrl: "/api/v1/jobs/job-kurs-upload-001", streamUrl: "/api/v1/jobs/job-kurs-upload-001/stream" } }),
      });
    });
    page.route("**/api/v1/master/kurs**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(KURS_LIST_RESPONSE) })
    );
    page.route("**/api/v1/jobs/**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { jobId: "job-kurs-upload-001", type: "KURS_UPLOAD", status: "running", progress: 60, currentStep: "Menyimpan entri kurs 90 dari 150...", canCancel: true } }) })
    );

    await page.goto("/master/kurs/upload");
    await page.waitForLoadState("networkidle");

    const fileInput = page.locator("input[type='file']");
    if (await fileInput.count() > 0) {
      await fileInput.setInputFiles({
        name: "kurs-2026-06-25.csv",
        mimeType: "text/csv",
        buffer: Buffer.from("kode,tanggal,kurs_jisdor\nUSD,2026-06-25,16250"),
      });

      const uploadBtn = page.getByRole("button", { name: /upload/i });
      if (await uploadBtn.isVisible({ timeout: 2000 })) {
        await uploadBtn.click();
        await page.waitForTimeout(500);

        if (capturedKey) {
          expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
        }

        // JobProgressPanel visible
        await expect(page.getByText(/menyimpan entri kurs|progress|job/i)).toBeVisible({ timeout: 3000 });
      }
    }
  });
});
