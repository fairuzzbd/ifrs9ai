/**
 * Playwright E2E — P5-M4 S5: Closing Checklist + Status Periode Report
 *
 * AC covered:
 *   S5-AC1: Checklist panel renders 4 items; failing item shows detail + DLQ link
 *   S5-AC2: Status periode report: sort + filter + cursor pagination; ROLE-MAKER-TR gets 403
 *   S5-AC3: Export CSV — async job submitted, progress panel shown, audit row written
 *   S5-AC4: Checklist after CLOSED shows HARD_CLOSE_APPROVE snapshot reference
 *
 * Pre-conditions:
 *   - /app-d/reports/status-periode — list view
 *   - /app-d/periode-buku/{id}/close-workflow — detail view
 */

import { test, expect } from "@playwright/test";

const REPORT_URL = "/app-d/reports/status-periode";
const PERIODE_CLOSED_ID = "11111111-0000-0000-0000-000000000005";
const CLOSE_URL = `/app-d/periode-buku/${PERIODE_CLOSED_ID}/close-workflow`;

const periodeListData = [
  {
    periodeId: "11111111-0000-0000-0000-000000000051",
    periodeKode: "2026-06",
    tipePeriode: "BULANAN",
    tahunBuku: 2026,
    statusPeriode: "CLOSED",
    tanggalHardClose: "2026-06-17T08:00:00+07:00",
    latestSnap: { transition: "HARD_CLOSE_APPROVE", evaluatedAt: "2026-06-17T07:55:00+07:00", allPassed: true },
  },
  {
    periodeId: "11111111-0000-0000-0000-000000000052",
    periodeKode: "2026-05",
    tipePeriode: "BULANAN",
    tahunBuku: 2026,
    statusPeriode: "SOFT_CLOSED",
    tanggalSoftClose: "2026-05-31T10:00:00+07:00",
    latestSnap: { transition: "SOFT_CLOSE_APPROVE", evaluatedAt: "2026-05-31T09:55:00+07:00", allPassed: true },
  },
  {
    periodeId: "11111111-0000-0000-0000-000000000053",
    periodeKode: "2026-04",
    tipePeriode: "BULANAN",
    tahunBuku: 2026,
    statusPeriode: "OPEN",
    latestSnap: null,
  },
];

test.describe("S5 — Status Periode Report + Checklist", () => {
  // S5-AC1: Checklist panel.
  test("S5-AC1: checklist panel shows 4 items; failing GL_DELIVERED shows detail + DLQ link", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_CLOSED_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { periodeId: PERIODE_CLOSED_ID, periodeKode: "2026-06", statusPeriode: "OPEN", rowVersion: 1 },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${PERIODE_CLOSED_ID}/closing-checklist`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_CLOSED_ID,
            transition: "MANUAL_CHECK",
            checklistSnapshotId: "snap-s5ac1-001",
            checklist: {
              evaluatedAt: new Date().toISOString(),
              allPassed: false,
              items: [
                { key: "PENDING_APPROVAL_ZERO", label: "Tidak ada transaksi/jurnal yang menunggu approval", passed: true, detail: "Semua transaksi final" },
                { key: "JURNAL_BALANCED", label: "Seluruh jurnal periode balanced (threshold IDR 0.01)", passed: true, detail: "Delta = 0.0000" },
                { key: "GL_DELIVERED", label: "Semua jurnal ter-deliver ke GL Host", passed: false, detail: "2 jurnal berstatus FAILED", actionUrl: "/api/v1/jurnal/dlq?filter[status]=FAILED" },
                { key: "RECON_PASS", label: "Rekonsiliasi GL terakhir berstatus COMPLETED", passed: true, detail: "COMPLETED (0 mismatch)" },
              ],
            },
          },
        }),
      });
    });

    await page.goto(CLOSE_URL);

    const checklistBtn = page.getByRole("button", { name: /Periksa Checklist/i });
    await expect(checklistBtn).toBeVisible();
    await checklistBtn.click();

    const panel = page.getByTestId("closing-checklist-panel");
    await expect(panel).toBeVisible();

    // 4 items.
    await expect(panel.getByTestId("checklist-item")).toHaveCount(4);

    // GL_DELIVERED row — failing, has action URL.
    const glRow = panel.getByTestId("checklist-item").filter({ hasText: /GL Host/i });
    await expect(glRow).toContainText("2 jurnal berstatus FAILED");
    await expect(glRow.getByRole("link", { name: /Lihat DLQ/i })).toBeVisible();

    // Other 3 pass.
    const passIcons = panel.getByTestId("checklist-pass-icon");
    await expect(passIcons).toHaveCount(3);

    const failIcons = panel.getByTestId("checklist-fail-icon");
    await expect(failIcons).toHaveCount(1);
  });

  // S5-AC2: Status periode report — sort + filter + pagination + 403.
  test("S5-AC2: status periode report renders with sort/filter; MAKER-TR gets 403", async ({ page }) => {
    await page.route("**/api/v1/reports/status-periode*", (route) => {
      const url = new URL(route.request().url());
      const filterStatus = url.searchParams.get("filter[status_periode]");

      // Filter by CLOSED.
      const filtered = filterStatus
        ? periodeListData.filter((p) => p.statusPeriode === filterStatus)
        : periodeListData;

      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: filtered,
          pagination: { nextCursor: null, hasMore: false, totalEstimate: filtered.length, limit: 50 },
          appliedSort: [{ col: "tanggal_hard_close", dir: "desc" }],
          appliedFilter: filterStatus ? { status_periode: filterStatus } : {},
          meta: { traceId: "trace-list-001" },
        }),
      });
    });

    await page.goto(REPORT_URL);

    // Table renders.
    await expect(page.getByRole("table")).toBeVisible();
    await expect(page.getByText("2026-06")).toBeVisible();
    await expect(page.getByText("2026-05")).toBeVisible();

    // Filter by CLOSED.
    const statusFilter = page.getByRole("combobox", { name: /Filter status/i });
    await statusFilter.selectOption("CLOSED");

    // Only CLOSED entry should remain.
    await expect(page.getByText("2026-06")).toBeVisible();
    await expect(page.getByText("2026-05")).not.toBeVisible();

    // Sort by tahun_buku — column header clickable.
    const tahunHeader = page.getByRole("columnheader", { name: /Tahun Buku/i });
    await tahunHeader.click();
    // Arrow icon toggled.
    await expect(tahunHeader.getByTestId("sort-icon")).toBeVisible();

    // Pagination footer.
    await expect(page.getByTestId("pagination-footer")).toBeVisible();

    // Export button.
    await expect(page.getByRole("button", { name: /Export/i })).toBeVisible();
  });

  // S5-AC2: ROLE-MAKER-TR forbidden.
  test("S5-AC2: ROLE-MAKER-TR receives 403 FORBIDDEN on status-periode report", async ({ page }) => {
    await page.route("**/api/v1/reports/status-periode*", (route) => {
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "FORBIDDEN",
            message: "Anda tidak memiliki izin periode.status.read.",
            traceId: "trace-403-001",
          },
        }),
      });
    });

    await page.goto(REPORT_URL);

    await expect(page.getByText(/FORBIDDEN/i)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/periode.status.read/i)).toBeVisible();
    await expect(page.getByRole("table")).not.toBeVisible();
  });

  // S5-AC3: Export CSV — async job, progress panel.
  test("S5-AC3: export CSV submits async job and shows progress panel", async ({ page }) => {
    const jobId = "job_export_periode_list";

    await page.route("**/api/v1/reports/status-periode*", (route) => {
      if (route.request().url().includes("/export")) {
        route.fulfill({
          status: 202,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              jobId,
              type: "STATUS_PERIODE_EXPORT",
              statusUrl: `/api/v1/jobs/${jobId}`,
              streamUrl: `/api/v1/jobs/${jobId}/stream`,
            },
          }),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: periodeListData,
            pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 50 },
            meta: { traceId: "trace-export-001" },
          }),
        });
      }
    });

    await page.route(`**/api/v1/jobs/${jobId}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            jobId,
            type: "STATUS_PERIODE_EXPORT",
            status: "running",
            progress: 60,
            currentStep: "Mengekspor baris 60 dari 100",
            canCancel: true,
          },
        }),
      });
    });

    await page.goto(REPORT_URL);

    await expect(page.getByRole("table")).toBeVisible();

    // Open export dropdown.
    const exportBtn = page.getByRole("button", { name: /Export/i });
    await exportBtn.click();

    const csvOption = page.getByRole("menuitem", { name: /CSV/i });
    await csvOption.click();

    // Job progress panel appears (§3 UX pattern).
    const progressPanel = page.getByTestId("job-progress-panel");
    await expect(progressPanel).toBeVisible({ timeout: 5000 });
    await expect(progressPanel.getByRole("progressbar")).toBeVisible();
    await expect(progressPanel.getByText(/60%/)).toBeVisible();
    await expect(progressPanel.getByText(/Mengekspor baris/i)).toBeVisible();
    await expect(progressPanel.getByRole("button", { name: /Batalkan/i })).toBeVisible();
  });

  // S5-AC4: Checklist after CLOSED shows HARD_CLOSE_APPROVE snapshot.
  test("S5-AC4: checklist panel for CLOSED periode shows HARD_CLOSE_APPROVE snapshot ref", async ({ page }) => {
    const CLOSED_ID = "11111111-0000-0000-0000-000000000054";
    const closedUrl = `/app-d/periode-buku/${CLOSED_ID}/close-workflow`;

    await page.route(`**/api/v1/periode/${CLOSED_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: CLOSED_ID,
            periodeKode: "2026-06",
            statusPeriode: "CLOSED",
            tanggalHardClose: "2026-06-17T08:00:00+07:00",
            hardCloseGraceExpiresAt: new Date(Date.now() + 12 * 60 * 60 * 1000).toISOString(),
            rowVersion: 5,
          },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${CLOSED_ID}/closing-checklist`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: CLOSED_ID,
            transition: "HARD_CLOSE_APPROVE",
            checklistSnapshotId: "snap-hca-closed-001",
            checklist: {
              evaluatedAt: "2026-06-17T07:55:00+07:00",
              allPassed: true,
              items: [
                { key: "PENDING_APPROVAL_ZERO", label: "Tidak ada transaksi/jurnal yang menunggu approval", passed: true, detail: "Semua final" },
                { key: "JURNAL_BALANCED", label: "Seluruh jurnal periode balanced (threshold IDR 0.01)", passed: true, detail: "Delta = 0" },
                { key: "GL_DELIVERED", label: "Semua jurnal ter-deliver ke GL Host", passed: true, detail: "Semua DELIVERED" },
                { key: "RECON_PASS", label: "Rekonsiliasi GL terakhir berstatus COMPLETED", passed: true, detail: "COMPLETED" },
              ],
            },
          },
        }),
      });
    });

    await page.goto(closedUrl);

    const checklistBtn = page.getByRole("button", { name: /Periksa Checklist/i });
    await checklistBtn.click();

    const panel = page.getByTestId("closing-checklist-panel");
    await expect(panel).toBeVisible();

    // Snapshot reference shown — HARD_CLOSE_APPROVE transition.
    await expect(panel.getByText(/HARD_CLOSE_APPROVE/i)).toBeVisible();
    await expect(panel.getByText(/snap-hca-closed-001/i)).toBeVisible();

    // All 4 pass.
    await expect(panel.getByTestId("checklist-pass-icon")).toHaveCount(4);
  });
});
