/**
 * Playwright mock spec — P5-M11-S2: DRY_RUN 4-stage validation pipeline
 *
 * Tests:
 *   - /master/instrumen/bulk-upload/BATCH-001/dry-run renders DRY_RUN preview panel
 *   - S2-AC1: DRY_RUN_PASSED — stage summary all PASS, flagged count shown
 *   - S2-AC2: DRY_RUN_FAILED — stage 3 error shown in errors_per_row table
 *   - S2-AC3: NEEDS_MANUAL_REVIEW: flaggedRows banner visible, status still DRY_RUN_PASSED
 *   - S2-AC4: dry_run TTL shown in panel
 *   - "Lanjut ke Commit" button present when DRY_RUN_PASSED
 *   - "Lanjut ke Commit" button absent when DRY_RUN_FAILED
 *   - "Ulangi DRY_RUN" button triggers POST /bulk-upload/BATCH-001/dry-run
 */

import { test, expect } from "@playwright/test";

const BASE = "http://localhost:3000";
const BATCH_ID = "BATCH-001";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const batchDetailPassed = {
  data: {
    batchId: BATCH_ID,
    status: "DRY_RUN_PASSED",
    totalRows: 350,
    parseErrors: [],
    sheets: { Deposito: 80, Obligasi: 120, Saham: 60, Reksadana: 50, Tabungan_Cash: 40 },
    createdAt: "2026-06-21T10:30:00+07:00",
    committedRows: 0,
    failedRows: 0,
    flaggedRows: 3,
    dryRunExpiresAt: "2026-06-21T11:30:00+07:00",
    rollbackStatus: null,
    rollbackGraceExpiresAt: null,
    approverId: null,
    approvedAt: null,
  },
  rows: [],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 },
  meta: { traceId: "trace-001" },
};

const batchDetailFailed = {
  ...batchDetailPassed,
  data: {
    ...batchDetailPassed.data,
    status: "DRY_RUN_FAILED",
    failedRows: 1,
    flaggedRows: 0,
    dryRunExpiresAt: null,
  },
};

const dryRunPassedResponse = {
  data: {
    status: "DRY_RUN_PASSED",
    totalRows: 350,
    validRows: 347,
    invalidRows: 0,
    flaggedRows: 3,
    stageSummary: {
      stage1: { status: "PASS" },
      stage2: { status: "PASS" },
      stage3: { status: "PASS" },
      stage4: { status: "PASS", evaluated: 350, classified: 347, flagged: 3, sppiServiceUnavailable: false },
    },
    errorsPerRow: [],
    dryRunExpiresAt: "2026-06-21T11:30:00+07:00",
  },
  meta: { traceId: "trace-dry-001" },
};

const dryRunFailedResponse = {
  data: {
    status: "DRY_RUN_FAILED",
    totalRows: 350,
    validRows: 349,
    invalidRows: 1,
    flaggedRows: 0,
    stageSummary: {
      stage1: { status: "PASS" },
      stage2: { status: "PASS" },
      stage3: { status: "FAIL", errorCount: 1 },
      stage4: { status: "PASS", evaluated: 0, classified: 0, flagged: 0, sppiServiceUnavailable: false },
    },
    errorsPerRow: [
      {
        sheet: "Obligasi",
        row: 10,
        stage: 3,
        col: "counterparty_id",
        error: "Counterparty CP-999 tidak ditemukan di master data.",
      },
    ],
    dryRunExpiresAt: null,
  },
  meta: { traceId: "trace-dry-002" },
};

// ---------------------------------------------------------------------------
// S2-AC1 — DRY_RUN_PASSED
// ---------------------------------------------------------------------------

test.describe("S2: DRY_RUN preview — PASSED", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(`**/api/v1/master/instrumen/bulk-upload/${BATCH_ID}**`, async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(batchDetailPassed) });
      } else {
        await route.continue();
      }
    });

    await page.route(`**/api/v1/master/instrumen/bulk-upload/${BATCH_ID}/dry-run`, async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(dryRunPassedResponse) });
    });

    await page.goto(`${BASE}/master/instrumen/bulk-upload/${BATCH_ID}/dry-run`);
    await page.waitForLoadState("networkidle");
  });

  test("S2-AC1: Preview panel shows DRY_RUN_PASSED heading", async ({ page }) => {
    await expect(page.locator("text=Lulus").first()).toBeVisible();
  });

  test("S2-AC1: Stage summary shows all 4 stages", async ({ page }) => {
    await expect(page.locator("text=Tahap 1").first()).toBeVisible();
    await expect(page.locator("text=Tahap 2").first()).toBeVisible();
    await expect(page.locator("text=Tahap 3").first()).toBeVisible();
    await expect(page.locator("text=Tahap 4").first()).toBeVisible();
  });

  test("S2-AC3: NEEDS_MANUAL_REVIEW banner visible for flagged rows", async ({ page }) => {
    // The 3 flagged rows banner from BulkDryRunResultPanel
    await expect(page.locator("text=review klasifikasi PSAK 71 manual").first()).toBeVisible();
  });

  test("S2-AC4: DRY_RUN expiry timestamp shown", async ({ page }) => {
    // Expiry shown somewhere on page
    await expect(page.locator("text=DRY_RUN berlaku").first()).toBeVisible();
  });

  test("S2-AC1: Lanjut ke Commit button present when DRY_RUN_PASSED", async ({ page }) => {
    const commitBtn = page.locator("text=Lanjut ke Commit");
    await expect(commitBtn).toBeVisible();
  });

  test("Ulangi DRY_RUN button triggers POST to dry-run endpoint", async ({ page }) => {
    const [request] = await Promise.all([
      page.waitForRequest((req) =>
        req.url().includes(`/dry-run`) && req.method() === "POST",
      ),
      page.locator("text=Ulangi DRY_RUN").click(),
    ]);
    expect(request.url()).toContain(`/${BATCH_ID}/dry-run`);
    expect(request.method()).toBe("POST");
  });
});

// ---------------------------------------------------------------------------
// S2-AC2 — DRY_RUN_FAILED
// ---------------------------------------------------------------------------

test.describe("S2: DRY_RUN preview — FAILED", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(`**/api/v1/master/instrumen/bulk-upload/${BATCH_ID}**`, async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(batchDetailFailed) });
      } else {
        await route.continue();
      }
    });

    await page.route(`**/api/v1/master/instrumen/bulk-upload/${BATCH_ID}/dry-run`, async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(dryRunFailedResponse) });
    });

    await page.goto(`${BASE}/master/instrumen/bulk-upload/${BATCH_ID}/dry-run`);
    await page.waitForLoadState("networkidle");
  });

  test("S2-AC2: DRY_RUN_FAILED status badge visible", async ({ page }) => {
    await expect(page.locator('[aria-label="Status batch: Validasi Gagal"]').first()).toBeVisible();
  });

  test("S2-AC2: Lanjut ke Commit button absent when DRY_RUN_FAILED", async ({ page }) => {
    const commitBtn = page.locator("text=Lanjut ke Commit");
    await expect(commitBtn).toHaveCount(0);
  });

  test("S2-AC2: Re-run DRY_RUN and see errors_per_row after result", async ({ page }) => {
    await page.locator("text=Ulangi DRY_RUN").click();
    await page.waitForTimeout(500);
    // After dry run, errors table should appear (detail toggled open)
    // The "Tampilkan detail error" toggle should be present with 1 error
    await expect(page.locator("text=detail error (1 baris)")).toBeVisible();
  });
});
