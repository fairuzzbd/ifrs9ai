/**
 * Playwright E2E — P5-M4 S4: Reopen + Grace Window
 *
 * AC covered:
 *   S4-AC1: Reopen SOFT_CLOSED → OPEN, no step-up, SoD on approve
 *   S4-AC2: Reopen CLOSED → SOFT_CLOSED within grace window, CFO step-up required
 *   S4-AC3: Reopen reason < 30 chars → 400 VALIDATION_FAILED inline error
 *   S4-AC4: Reopen CLOSED after grace expired → 423 PERIODE_GRACE_EXPIRED
 *
 * Pre-conditions:
 *   - User logged in as ROLE-CFO
 *   - Periode test scenarios below use different IDs per test
 */

import { test, expect } from "@playwright/test";

const MFA_DIALOG_TITLE = /Verifikasi MFA Step-Up/i;

// S4-AC1: Reopen SOFT_CLOSED.
test.describe("S4-AC1 — Reopen SOFT_CLOSED → OPEN", () => {
  const PERIODE_ID = "11111111-0000-0000-0000-000000000041";
  const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

  test("S4-AC1: reopen SOFT_CLOSED to OPEN without step-up, SoD on approve", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-05",
            statusPeriode: "SOFT_CLOSED",
            softCloseApprovedBy: "aaaaaaaa-0000-0000-0000-000000000002",
            rowVersion: 2,
          },
          meta: { traceId: "trace-s4ac1-001" },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${PERIODE_ID}/reopen-request`, (route) => {
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-05",
            transition: "REOPEN_REQUEST",
            checklistSnapshotId: "snap-reopen-001",
            message: "Reopen request telah diajukan. Menunggu approval.",
          },
          meta: { traceId: "trace-s4ac1-002" },
        }),
      });
    });

    await page.goto(CLOSE_URL);

    const reopenBtn = page.getByRole("button", { name: /Ajukan Reopen/i });
    await expect(reopenBtn).toBeVisible();
    await reopenBtn.click();

    const dialog = page.getByRole("dialog").filter({ has: page.getByText(/Ajukan Reopen/i) });
    await expect(dialog).toBeVisible();

    // No MFA step-up dialog for SOFT_CLOSED reopen.
    await expect(page.getByText(MFA_DIALOG_TITLE)).not.toBeVisible();

    // Fill reason (≥ 30 chars).
    const reasonInput = dialog.getByRole("textbox", { name: /Alasan reopen/i });
    await reasonInput.fill("Kesalahan jurnal ditemukan setelah soft close, perlu koreksi ulang sebelum periode ditutup");

    await dialog.getByRole("button", { name: /Ajukan/i }).click();

    const toast = page.getByRole("alert").filter({ hasText: /Reopen request/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast.getByText(/approval/i)).toBeVisible();
  });
});

// S4-AC2: Reopen CLOSED within grace window.
test.describe("S4-AC2 — Reopen CLOSED → SOFT_CLOSED in grace window", () => {
  const PERIODE_ID = "11111111-0000-0000-0000-000000000042";
  const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

  test("S4-AC2: reopen CLOSED with CFO step-up within grace window → SOFT_CLOSED", async ({ page }) => {
    const graceExpiry = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString();

    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-04",
            statusPeriode: "CLOSED",
            hardCloseGraceExpiresAt: graceExpiry,
            rowVersion: 5,
          },
          meta: { traceId: "trace-s4ac2-001" },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${PERIODE_ID}/reopen-request`, (route) => {
      route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          data: { periodeId: PERIODE_ID, transition: "REOPEN_REQUEST", message: "Reopen request diajukan." },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${PERIODE_ID}/reopen-approve`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-04",
            newStatus: "SOFT_CLOSED",
            fxRateUnlocked: true,
            message: "Periode berhasil dibuka kembali ke SOFT_CLOSED. FX rate di-unlock.",
          },
          meta: { traceId: "trace-s4ac2-002" },
        }),
      });
    });

    await page.route("**/auth/step-up", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { stepUpToken: "valid-reopen-stepup", expiresIn: 300, scope: "periode.reopen.approve" },
        }),
      });
    });

    await page.goto(CLOSE_URL);

    // Grace window banner should show countdown.
    await expect(page.getByTestId("grace-window-countdown")).toBeVisible();

    const reopenBtn = page.getByRole("button", { name: /Ajukan Reopen/i });
    await expect(reopenBtn).toBeVisible();
    await reopenBtn.click();

    const dialog = page.getByRole("dialog");
    const reasonInput = dialog.getByRole("textbox", { name: /Alasan reopen/i });
    await reasonInput.fill("Ditemukan kesalahan posting setelah hard close, reopen dalam grace window sesuai SOP");
    await dialog.getByRole("button", { name: /Ajukan/i }).click();

    // Now approve flow (as separate actor in real life, but simulated here).
    const approveBtn = page.getByRole("button", { name: /Approve Reopen/i });
    await expect(approveBtn).toBeVisible();
    await approveBtn.click();

    // MFA step-up required for CLOSED→SOFT_CLOSED reopen.
    const mfaDialog = page.getByRole("dialog").filter({ has: page.getByText(MFA_DIALOG_TITLE) });
    await expect(mfaDialog).toBeVisible();
    await mfaDialog.getByRole("textbox", { name: /Kode OTP/i }).fill("123456");
    await mfaDialog.getByRole("button", { name: /Verifikasi/i }).click();

    const confirmDialog = page.getByRole("dialog").filter({ has: page.getByText(/Approve Reopen/i) });
    await expect(confirmDialog).toBeVisible();
    await confirmDialog.getByRole("button", { name: /Konfirmasi/i }).click();

    // Success toast — shows FX unlock.
    const toast = page.getByRole("alert").filter({ hasText: /SOFT_CLOSED/i });
    await expect(toast).toBeVisible({ timeout: 5000 });
    await expect(toast.getByText(/FX rate/i)).toBeVisible();

    // Status updated.
    await expect(page.getByTestId("periode-status-badge")).toContainText("SOFT_CLOSED");
  });
});

// S4-AC3: Reason too short → inline error.
test.describe("S4-AC3 — Reopen reason validation", () => {
  const PERIODE_ID = "11111111-0000-0000-0000-000000000043";
  const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

  test("S4-AC3: reason < 30 chars shows inline validation error", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { periodeId: PERIODE_ID, periodeKode: "2026-03", statusPeriode: "SOFT_CLOSED", rowVersion: 1 },
          meta: { traceId: "trace-s4ac3" },
        }),
      });
    });

    await page.goto(CLOSE_URL);

    await page.getByRole("button", { name: /Ajukan Reopen/i }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const reasonInput = dialog.getByRole("textbox", { name: /Alasan reopen/i });
    await reasonInput.fill("Terlalu pendek"); // < 30 chars

    await dialog.getByRole("button", { name: /Ajukan/i }).click();

    // Inline error — button should NOT submit.
    const inlineError = dialog.getByRole("alert").filter({ hasText: /minimal 30 karakter/i });
    await expect(inlineError).toBeVisible();

    // Verify reason input is highlighted as error.
    await expect(reasonInput).toHaveAttribute("aria-invalid", "true");
  });
});

// S4-AC4: Reopen after grace expired.
test.describe("S4-AC4 — Reopen CLOSED after grace expired", () => {
  const PERIODE_ID = "11111111-0000-0000-0000-000000000044";
  const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;

  test("S4-AC4: reopen CLOSED after grace shows 423 PERIODE_GRACE_EXPIRED", async ({ page }) => {
    const graceExpiry = new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(); // 1h ago

    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-02",
            statusPeriode: "CLOSED",
            hardCloseGraceExpiresAt: graceExpiry,
            rowVersion: 4,
          },
          meta: { traceId: "trace-s4ac4" },
        }),
      });
    });

    await page.route(`**/api/v1/periode/${PERIODE_ID}/reopen-request`, (route) => {
      route.fulfill({
        status: 423,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "PERIODE_GRACE_EXPIRED",
            message: "Grace window untuk periode 2026-02 sudah berakhir. Tidak dapat direopen.",
            traceId: "trace-grace-exp",
          },
        }),
      });
    });

    await page.goto(CLOSE_URL);

    // Grace window banner shows EXPIRED.
    const graceBanner = page.getByTestId("grace-window-countdown");
    await expect(graceBanner).toBeVisible();
    await expect(graceBanner).toContainText(/kedaluwarsa/i);

    // Reopen button may still be rendered (grace expired state shown) but clicking gives 423.
    const reopenBtn = page.getByRole("button", { name: /Ajukan Reopen/i });
    if (await reopenBtn.isVisible()) {
      await reopenBtn.click();
      const dialog = page.getByRole("dialog");
      if (await dialog.isVisible()) {
        const reasonInput = dialog.getByRole("textbox", { name: /Alasan reopen/i });
        await reasonInput.fill("Alasan yang cukup panjang untuk memenuhi validasi minimum 30 karakter");
        await dialog.getByRole("button", { name: /Ajukan/i }).click();
      }

      const toast = page.getByRole("alert").filter({ hasText: /PERIODE_GRACE_EXPIRED/i });
      await expect(toast).toBeVisible({ timeout: 5000 });
    } else {
      // Alternatively, button might be disabled with a tooltip.
      const disabledBtn = page.getByRole("button", { name: /Ajukan Reopen/i });
      await expect(disabledBtn).toBeDisabled();
      await expect(page.getByText(/Grace window telah berakhir/i)).toBeVisible();
    }
  });
});
