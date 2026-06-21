/**
 * Playwright E2E — P5-M3 S4: GL Reconciliation
 * Stories: S4-AC1 (today's report card), S4-AC2 (run recon → job progress),
 *          S4-AC3 (mismatch table), S4-AC4 (history list)
 *
 * Pre-conditions:
 *   - User logged in as ROLE-AKUN-CTL (has jurnal.gl_reconciliation.run)
 *   - Today = 2026-06-17 (fixed in test)
 */

import { test, expect } from "@playwright/test";

const RECON_URL = "/jrnl/rekonsiliasi";
const HISTORY_URL = "/jrnl/rekonsiliasi/riwayat";

test.describe("S4 — GL Reconciliation", () => {
  // S4-AC1: happy path — report card shows COMPLETED state
  test("S4-AC1: shows COMPLETED report card for today", async ({ page }) => {
    await page.route("**/api/v1/jurnal/reconciliation/daily*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            reportId: "dddddddd-0000-0000-0000-000000000001",
            tanggalRekonsiliasi: "2026-06-17",
            status: "COMPLETED",
            totalAkunChecked: 120,
            totalMismatchCount: 0,
            blipsTotalIdr: 50000000000,
            glHostTotalIdr: 50000000000,
            deltaIdr: 0,
            toleranceIdr: 0,
            generatedAt: "2026-06-17T22:00:00+07:00",
          },
          meta: { traceId: "test-s4-001" },
        }),
      });
    });

    await page.goto(RECON_URL);

    await expect(page.getByText("Rekonsiliasi GL")).toBeVisible();
    // Should display COMPLETED badge/label
    await expect(page.getByText(/Sesuai|COMPLETED/i)).toBeVisible();
    // Should display total accounts checked
    await expect(page.getByText("120")).toBeVisible();
  });

  // S4-AC3: COMPLETED_WITH_MISMATCH shows mismatch table
  test("S4-AC3: COMPLETED_WITH_MISMATCH shows mismatch table", async ({ page }) => {
    await page.route("**/api/v1/jurnal/reconciliation/daily*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            reportId: "dddddddd-0000-0000-0000-000000000002",
            tanggalRekonsiliasi: "2026-06-17",
            status: "COMPLETED_WITH_MISMATCH",
            totalAkunChecked: 120,
            totalMismatchCount: 2,
            blipsTotalIdr: 50000000000,
            glHostTotalIdr: 49999000000,
            deltaIdr: 1000000,
            toleranceIdr: 0,
            generatedAt: "2026-06-17T22:00:00+07:00",
            mismatchLines: [
              {
                kodeAkun: "1-1101",
                namaAkun: "Kas dan Setara Kas",
                blipsAmountIdr: 10000000000,
                glHostAmountIdr: 9999000000,
                deltaIdr: 1000000,
                mismatchType: "AMOUNT_DIFF",
                jurnalHeaderIds: ["bbbbbbbb-0000-0000-0000-000000000001"],
              },
            ],
          },
          meta: { traceId: "test-s4-002" },
        }),
      });
    });

    await page.goto(RECON_URL);

    // Mismatch count shown
    await expect(page.getByText(/2 akun.*selisih/i)).toBeVisible();
    // Mismatch table header
    await expect(page.getByText("Mismatch Detail")).toBeVisible();
    // Mismatch type badge
    await expect(page.getByText(/Selisih Jumlah/i)).toBeVisible();
    // Account code
    await expect(page.getByText("1-1101")).toBeVisible();
  });

  // S4-AC2: run recon trigger shows job progress panel
  test("S4-AC2: run reconciliation shows job progress panel (§3)", async ({ page }) => {
    // No existing report
    await page.route("**/api/v1/jurnal/reconciliation/daily*", (route) => {
      route.fulfill({ status: 404, body: JSON.stringify({ error: { code: "NOT_FOUND" } }) });
    });

    // Mock run endpoint
    await page.route("**/api/v1/jurnal/reconciliation/run", (route) => {
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            jobId: "job-recon-001",
            statusUrl: "/api/v1/jobs/job-recon-001",
            streamUrl: "/api/v1/jobs/job-recon-001/stream",
            tanggalRekonsiliasi: "2026-06-17",
          },
          meta: { traceId: "t-run" },
        }),
      });
    });

    // Mock job status (running)
    await page.route("**/api/v1/jobs/job-recon-001", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            jobId: "job-recon-001",
            status: "running",
            progress: 45,
            currentStep: "Membandingkan 54 dari 120 akun...",
            canCancel: false,
          },
        }),
      });
    });

    await page.goto(RECON_URL);

    // Click run button
    const runBtn = page.getByRole("button", { name: /Jalankan Rekonsiliasi/i });
    await expect(runBtn).toBeVisible();
    await runBtn.click();

    // Job progress panel should appear (§3 UX pattern)
    await expect(page.getByRole("region", { name: /Kemajuan/i })).toBeVisible({
      timeout: 5000,
    });

    // Progress bar present
    await expect(page.locator("progress")).toBeVisible();
  });

  // S4-AC1: empty state when no report
  test("S4-AC1: shows empty state when no report exists", async ({ page }) => {
    await page.route("**/api/v1/jurnal/reconciliation/daily*", (route) => {
      route.fulfill({ status: 404, body: "{}" });
    });

    await page.goto(RECON_URL);

    await expect(page.getByText(/Tidak ada laporan rekonsiliasi/i)).toBeVisible();
  });

  // S4-AC4: history list renders
  test("S4-AC4: history list shows reconciliation reports", async ({ page }) => {
    await page.route("**/api/v1/jurnal/reconciliation/history*", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              reportId: "dddddddd-0000-0000-0000-000000000010",
              tanggalRekonsiliasi: "2026-06-16",
              status: "COMPLETED",
              totalMismatchCount: 0,
              generatedAt: "2026-06-16T23:00:00+07:00",
            },
            {
              reportId: "dddddddd-0000-0000-0000-000000000011",
              tanggalRekonsiliasi: "2026-06-15",
              status: "COMPLETED_WITH_MISMATCH",
              totalMismatchCount: 1,
              deltaIdr: -250000,
              generatedAt: "2026-06-15T23:05:00+07:00",
            },
          ],
          pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 50 },
          meta: { traceId: "t-hist" },
        }),
      });
    });

    await page.goto(HISTORY_URL);

    await expect(page.getByRole("heading", { name: /Riwayat Rekonsiliasi/i })).toBeVisible();
    await expect(page.getByText("2026-06-16")).toBeVisible();
    await expect(page.getByText("2026-06-15")).toBeVisible();
    // Status badges
    await expect(page.getByText(/Sesuai/i)).toBeVisible();
    await expect(page.getByText(/Ada Mismatch/i)).toBeVisible();
  });
});
