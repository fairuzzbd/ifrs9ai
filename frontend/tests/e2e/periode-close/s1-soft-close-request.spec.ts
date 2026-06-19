/**
 * Playwright E2E — P5-M4 S1: Soft-Close Request
 *
 * AC covered:
 *   S1-AC1: Maker ROLE-AKUN-CTL can submit soft-close request when checklist all-pass → 202, dialog shows snapshot
 *   S1-AC2: Duplicate pending request → 409 toast with SOFT_CLOSE_PENDING_EXISTS
 *   S1-AC3: Checklist fails (GL_DELIVERED) → 422 toast with failed item visible, CTA to DLQ
 *   S1-AC4: MANUAL_CHECK via checklist modal — 4 items + MANUAL_CHECK transition
 *
 * Pre-conditions:
 *   - Page: /app-d/periode-buku/{periodeId}/close-workflow
 *   - User: ROLE-AKUN-CTL, MFA verified
 *   - Periode: status = OPEN, row_version = 1
 */

import { test, expect } from "@playwright/test";
import { v4 as uuidv4 } from "uuid";

const PERIODE_ID = "11111111-0000-0000-0000-000000000001";
const SNAPSHOT_ID = "22222222-0000-0000-0000-000000000001";
const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

const checklistAllPass = [
  { key: "PENDING_APPROVAL_ZERO", label: "Tidak ada transaksi/jurnal yang menunggu approval", passed: true, detail: "Semua transaksi final" },
  { key: "JURNAL_BALANCED", label: "Seluruh jurnal periode balanced (threshold IDR 0.01)", passed: true, detail: "Delta = 0.0000" },
  { key: "GL_DELIVERED", label: "Semua jurnal ter-deliver ke GL Host", passed: true, detail: "Semua DELIVERED" },
  { key: "RECON_PASS", label: "Rekonsiliasi GL terakhir berstatus COMPLETED", passed: true, detail: "COMPLETED (0 mismatch)" },
];

const checklistGLFailed = [
  { key: "PENDING_APPROVAL_ZERO", label: "Tidak ada transaksi/jurnal yang menunggu approval", passed: true, detail: "Semua transaksi final" },
  { key: "JURNAL_BALANCED", label: "Seluruh jurnal periode balanced (threshold IDR 0.01)", passed: true, detail: "Delta = 0.0000" },
  { key: "GL_DELIVERED", label: "Semua jurnal ter-deliver ke GL Host", passed: false, detail: "2 jurnal berstatus FAILED", actionUrl: "/api/v1/jurnal/dlq?filter[status]=FAILED" },
  { key: "RECON_PASS", label: "Rekonsiliasi GL terakhir berstatus COMPLETED", passed: true, detail: "COMPLETED (0 mismatch)" },
];

test.describe("S1 — Soft-Close Request", () => {
  test.beforeEach(async ({ page }) => {
    // Mock GET periode status.
    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            tahunBuku: 2026,
            statusPeriode: "OPEN",
            rowVersion: 1,
          },
          meta: { traceId: "trace-001" },
        }),
      });
    });

    await page.goto(CLOSE_URL);
  });

  // S1-AC1: Happy path.
  test("S1-AC1: all-pass checklist → soft-close request succeeds with 202", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-request`, (route) => {
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            transition: "SOFT_CLOSE_REQUEST",
            checklistSnapshotId: SNAPSHOT_ID,
            checklist: { evaluatedAt: new Date().toISOString(), allPassed: true, items: checklistAllPass },
            allPassed: true,
            nextStep: "Menunggu approval dari ROLE-AKUN-CTL lain.",
          },
          meta: { traceId: "trace-001" },
        }),
      });
    });

    const softCloseBtn = page.getByRole("button", { name: /Ajukan Soft Close/i });
    await expect(softCloseBtn).toBeVisible();
    await softCloseBtn.click();

    // Confirmation dialog appears.
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/Ajukan Soft Close Periode/i)).toBeVisible();

    // Checklist panel shows 4 items.
    await expect(dialog.getByTestId("checklist-item")).toHaveCount(4);

    // Confirm submit.
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Success toast.
    await expect(page.getByRole("alert").filter({ hasText: /berhasil/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/SOFT_CLOSE_REQUEST/)).toBeVisible();
    await expect(page.getByText(/snapshot/i)).toBeVisible();
  });

  // S1-AC2: Duplicate pending → 409.
  test("S1-AC2: duplicate pending request shows 409 toast SOFT_CLOSE_PENDING_EXISTS", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-request`, (route) => {
      route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "SOFT_CLOSE_PENDING_EXISTS",
            message: "Soft-close request sudah ada dan belum di-approve untuk periode 2026-06.",
            traceId: "trace-dup-001",
          },
        }),
      });
    });

    await page.getByRole("button", { name: /Ajukan Soft Close/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Error toast — persistent, contains error code.
    const toast = page.getByRole("alert").filter({ hasText: /SOFT_CLOSE_PENDING_EXISTS/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast.getByText(/trace-dup-001/i)).toBeVisible();
  });

  // S1-AC3: Checklist fails → 422 with GL_DELIVERED details + DLQ link.
  test("S1-AC3: GL_DELIVERED failing item shows 422 with DLQ action URL", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/soft-close-request`, (route) => {
      route.fulfill({
        status: 422,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "CLOSING_CHECKLIST_FAILED",
            message: "Checklist closing tidak lulus: 1 item gagal.",
            details: checklistGLFailed.filter((i) => !i.passed).map((i) => ({
              key: i.key,
              label: i.label,
              passed: false,
              detail: i.detail,
              actionUrl: i.actionUrl,
            })),
            traceId: "trace-chk-001",
          },
        }),
      });
    });

    await page.getByRole("button", { name: /Ajukan Soft Close/i }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Error toast.
    const toast = page.getByRole("alert").filter({ hasText: /CLOSING_CHECKLIST_FAILED/i });
    await expect(toast).toBeVisible({ timeout: 5000 });

    // Item detail visible.
    await expect(page.getByText(/2 jurnal berstatus FAILED/i)).toBeVisible();

    // CTA link to DLQ.
    const dlqLink = page.getByRole("link", { name: /Lihat DLQ/i });
    await expect(dlqLink).toBeVisible();
  });

  // S1-AC4: MANUAL_CHECK via checklist modal.
  test("S1-AC4: MANUAL_CHECK via opening checklist panel returns 4 items", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/closing-checklist`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            transition: "MANUAL_CHECK",
            checklistSnapshotId: SNAPSHOT_ID,
            checklist: { evaluatedAt: new Date().toISOString(), allPassed: true, items: checklistAllPass },
          },
          meta: { traceId: "trace-manual-001" },
        }),
      });
    });

    const checklistBtn = page.getByRole("button", { name: /Periksa Checklist/i });
    await expect(checklistBtn).toBeVisible();
    await checklistBtn.click();

    const panel = page.getByTestId("closing-checklist-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("checklist-item")).toHaveCount(4);
    await expect(panel.getByText("MANUAL_CHECK")).toBeVisible();

    // All items show pass icon.
    const passIcons = panel.getByTestId("checklist-pass-icon");
    await expect(passIcons).toHaveCount(4);
  });
});
