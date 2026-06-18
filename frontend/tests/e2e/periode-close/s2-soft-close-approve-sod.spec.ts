/**
 * Playwright E2E — P5-M4 S2: Soft-Close Approve + SoD
 *
 * AC covered:
 *   S2-AC1: SoD violation — maker tries to approve own request → 403 SOD_VIOLATION toast
 *   S2-AC2: Stale checklist (> 24h) at approve time → 422 CLOSING_CHECKLIST_STALE
 *   S2-AC3: Happy path — OPEN → SOFT_CLOSED, status badge updates, audit shown
 *   S2-AC4: Row-version conflict → 409 CONFLICT toast
 *
 * Pre-conditions:
 *   - User logged in as ROLE-AKUN-CTL (approver, different from maker)
 *   - Periode: status = OPEN, soft_close_requested_by ≠ current user, row_version = 2
 */

import { test, expect } from "@playwright/test";

const PERIODE_ID = "11111111-0000-0000-0000-000000000002";
const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

test.describe("S2 — Soft-Close Approve / SoD", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            statusPeriode: "OPEN",
            softCloseRequestedBy: "aaaaaaaa-ffff-0000-0000-000000000001",
            softCloseRequestedAt: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(),
            rowVersion: 2,
          },
          meta: { traceId: "trace-s2-001" },
        }),
      });
    });

    await page.goto(CLOSE_URL);
  });

  // S2-AC1: SoD — maker = approver → 403.
  test("S2-AC1: SoD violation shows 403 SOD_VIOLATION toast and blocks approve", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-approve`, (route) => {
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "SOD_VIOLATION",
            message: "Anda tidak bisa menjadi approver untuk transaksi yang Anda buat sendiri.",
            traceId: "trace-sod-001",
          },
        }),
      });
    });

    const approveBtn = page.getByRole("button", { name: /Approve Soft Close/i });
    await expect(approveBtn).toBeVisible();
    await approveBtn.click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Error toast — persistent.
    const toast = page.getByRole("alert").filter({ hasText: /SOD_VIOLATION/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast).not.toHaveAttribute("data-dismiss-timeout"); // persistent
    await expect(toast.getByText(/trace-sod-001/i)).toBeVisible();

    // Status badge still shows OPEN.
    await expect(page.getByTestId("periode-status-badge")).toContainText("OPEN");
  });

  // S2-AC2: Stale checklist → 422 CLOSING_CHECKLIST_STALE.
  test("S2-AC2: stale checklist at approve time shows 422 toast with re-eval context", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-approve`, (route) => {
      route.fulfill({
        status: 422,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "CLOSING_CHECKLIST_STALE",
            message: "Checklist lebih dari 24 jam yang lalu. Re-evaluasi gagal: GL_DELIVERED tidak pass.",
            traceId: "trace-stale-001",
          },
        }),
      });
    });

    await page.getByRole("button", { name: /Approve Soft Close/i }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    const toast = page.getByRole("alert").filter({ hasText: /CLOSING_CHECKLIST_STALE/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast.getByText(/Re-evaluasi/i)).toBeVisible();
  });

  // S2-AC3: Happy path → SOFT_CLOSED.
  test("S2-AC3: approve succeeds — status badge changes to SOFT_CLOSED", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-approve`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            statusPeriode: "SOFT_CLOSED",
            tanggalSoftClose: new Date().toISOString(),
            approvedBy: "cccccccc-0000-0000-0000-000000000001",
            checklistSnapshotId: "dddddddd-0000-0000-0000-000000000001",
            message: "Periode 2026-06 berhasil soft-closed.",
          },
          meta: { traceId: "trace-ok-001" },
        }),
      });
    });

    await page.getByRole("button", { name: /Approve Soft Close/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText(/approve/i)).toBeVisible();
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Success toast — auto-dismiss 4s.
    const toast = page.getByRole("alert").filter({ hasText: /SOFT_CLOSED/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast.getByRole("link", { name: /Lihat status/i })).toBeVisible();

    // Status badge updated.
    await expect(page.getByTestId("periode-status-badge")).toContainText("SOFT_CLOSED");

    // Workflow action bar shows next action: "Ajukan Hard Close".
    await expect(page.getByRole("button", { name: /Ajukan Hard Close/i })).toBeVisible();
  });

  // S2-AC4: Row-version conflict → 409.
  test("S2-AC4: row-version conflict shows 409 CONFLICT toast", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-approve`, (route) => {
      route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "CONFLICT",
            message: "Periode 2026-06 dimodifikasi oleh pengguna lain. Muat ulang halaman.",
            traceId: "trace-conflict-001",
          },
        }),
      });
    });

    await page.getByRole("button", { name: /Approve Soft Close/i }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    const toast = page.getByRole("alert").filter({ hasText: /CONFLICT/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    // Refresh CTA.
    await expect(toast.getByRole("button", { name: /Muat ulang/i })).toBeVisible();
  });
});
