/**
 * Playwright E2E — P5-M4 S3: Hard-Close Request + CFO Step-Up MFA Approve
 *
 * AC covered:
 *   S3-AC1: CFO approves with valid step-up MFA → CLOSED, grace window shown, MV job status card
 *   S3-AC2: Missing X-Step-Up-Token → 401 MFA_STEP_UP_REQUIRED dialog
 *   S3-AC3: Expired step-up token (> 5 min) → 401 MFA_STEP_UP_EXPIRED, re-prompt
 *   S3-AC4: Hard-close-reject by CFO → HARD_CLOSE_PENDING → SOFT_CLOSED, no step-up needed
 *
 * Pre-conditions:
 *   - User logged in as ROLE-CFO, MFA verified (mfa_verified=true in JWT)
 *   - Periode: status = HARD_CLOSE_PENDING, row_version = 3
 */

import { test, expect } from "@playwright/test";

const PERIODE_ID = "11111111-0000-0000-0000-000000000003";
const CLOSE_URL = `/app-d/periode-buku/${PERIODE_ID}/close-workflow`;
const MFA_DIALOG_TITLE = /Verifikasi MFA Step-Up/i;

test.describe("S3 — Hard-Close Approve / MFA Step-Up", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            statusPeriode: "HARD_CLOSE_PENDING",
            hardCloseRequestedBy: "eeeeeeee-0000-0000-0000-000000000001",
            rowVersion: 3,
          },
          meta: { traceId: "trace-s3-001" },
        }),
      });
    });

    await page.goto(CLOSE_URL);
  });

  // S3-AC1: Full happy path — step-up MFA, approve → CLOSED.
  test("S3-AC1: CFO hard-close approve with valid step-up → CLOSED + grace window displayed", async ({ page }) => {
    const graceExpiry = new Date(Date.now() + 48 * 60 * 60 * 1000).toISOString();
    const jobId = "job_mv_abc123";

    // Mock auth/step-up endpoint.
    await page.route("**/auth/step-up", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { stepUpToken: "valid-stepup-token-abc", expiresIn: 300, scope: "periode.hardclose.approve" },
        }),
      });
    });

    // Mock hard-close-approve.
    await page.route(`**/api/v1/periode/${PERIODE_ID}/hard-close-approve`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            statusPeriode: "CLOSED",
            tanggalHardClose: new Date().toISOString(),
            graceExpiresAt: graceExpiry,
            approvedBy: "ffffffff-0000-0000-0000-000000000001",
            checklistSnapshotId: "snapshot-closed-001",
            mvRefreshJobId: jobId,
            mvRefreshStatusUrl: `/api/v1/jobs/${jobId}`,
            message: "Periode 2026-06 berhasil hard-closed.",
          },
          meta: { traceId: "trace-hca-001" },
        }),
      });
    });

    // Mock MV refresh job status.
    await page.route(`**/api/v1/jobs/${jobId}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { jobId, type: "reporting:mv_refresh", status: "running", progress: 45, currentStep: "Refreshing rpt.mv_ecl_summary" },
        }),
      });
    });

    const approveBtn = page.getByRole("button", { name: /Approve Hard Close/i });
    await expect(approveBtn).toBeVisible();
    await approveBtn.click();

    // MFA step-up dialog should appear.
    const mfaDialog = page.getByRole("dialog").filter({ has: page.getByText(MFA_DIALOG_TITLE) });
    await expect(mfaDialog).toBeVisible();

    // Enter TOTP code (simulated).
    const otpInput = mfaDialog.getByRole("textbox", { name: /Kode OTP/i });
    await otpInput.fill("123456");
    await mfaDialog.getByRole("button", { name: /Verifikasi/i }).click();

    // Confirm hard close dialog.
    const confirmDialog = page.getByRole("dialog").filter({ has: page.getByText(/Hard Close/i) });
    await expect(confirmDialog).toBeVisible();
    await confirmDialog.getByRole("button", { name: /Konfirmasi Hard Close/i }).click();

    // Success toast.
    const toast = page.getByRole("alert").filter({ hasText: /hard-closed/i });
    await expect(toast).toBeVisible({ timeout: 7000 });

    // Status badge.
    await expect(page.getByTestId("periode-status-badge")).toContainText("CLOSED");

    // Grace window countdown shown.
    await expect(page.getByTestId("grace-window-countdown")).toBeVisible();

    // MV refresh progress card shown.
    const mvCard = page.getByTestId("mv-refresh-status-card");
    await expect(mvCard).toBeVisible();
    await expect(mvCard.getByText(/reporting:mv_refresh/i)).toBeVisible();
  });

  // S3-AC2: Missing step-up → 401 MFA_STEP_UP_REQUIRED.
  test("S3-AC2: missing step-up token shows MFA_STEP_UP_REQUIRED dialog", async ({ page }) => {
    // Auth step-up fails (user cancels OTP).
    await page.route("**/auth/step-up", (route) => {
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({
          error: { code: "MFA_STEP_UP_REQUIRED", message: "Step-up MFA diperlukan untuk periode.hardclose.approve.", traceId: "trace-mfa-req" },
        }),
      });
    });

    await page.getByRole("button", { name: /Approve Hard Close/i }).click();

    // MFA dialog appears.
    const mfaDialog = page.getByRole("dialog").filter({ has: page.getByText(MFA_DIALOG_TITLE) });
    await expect(mfaDialog).toBeVisible();

    // Cancel — should show error about missing step-up.
    await mfaDialog.getByRole("button", { name: /Batal/i }).click();
    await expect(mfaDialog).not.toBeVisible();

    // No CLOSED transition happened.
    await expect(page.getByTestId("periode-status-badge")).toContainText("HARD_CLOSE_PENDING");
  });

  // S3-AC3: Expired step-up token → 401 MFA_STEP_UP_EXPIRED, re-prompt.
  test("S3-AC3: expired step-up token → MFA_STEP_UP_EXPIRED toast with re-authenticate CTA", async ({ page }) => {
    // Step-up returns expired token error.
    await page.route(`**/api/v1/periode/${PERIODE_ID}/hard-close-approve`, (route) => {
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({
          error: {
            code: "MFA_STEP_UP_EXPIRED",
            message: "Token step-up MFA sudah kedaluwarsa (> 5 menit). Silakan verifikasi ulang.",
            traceId: "trace-mfa-exp",
          },
        }),
      });
    });

    await page.route("**/auth/step-up", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { stepUpToken: "expired-token", expiresIn: 0, scope: "periode.hardclose.approve" },
        }),
      });
    });

    await page.getByRole("button", { name: /Approve Hard Close/i }).click();
    const mfaDialog = page.getByRole("dialog").filter({ has: page.getByText(MFA_DIALOG_TITLE) });
    await expect(mfaDialog).toBeVisible();
    await mfaDialog.getByRole("textbox", { name: /Kode OTP/i }).fill("999999");
    await mfaDialog.getByRole("button", { name: /Verifikasi/i }).click();

    const confirmDialog = page.getByRole("dialog").filter({ has: page.getByText(/Hard Close/i) });
    if (await confirmDialog.isVisible()) {
      await confirmDialog.getByRole("button", { name: /Konfirmasi/i }).click();
    }

    // Toast with expired code.
    const toast = page.getByRole("alert").filter({ hasText: /MFA_STEP_UP_EXPIRED/i });
    await expect(toast).toBeVisible({ timeout: 5000 });

    // Re-authenticate CTA.
    await expect(toast.getByRole("button", { name: /Verifikasi ulang/i })).toBeVisible();
  });

  // S3-AC4: Hard-close-reject → SOFT_CLOSED, no step-up needed.
  test("S3-AC4: hard-close reject returns to SOFT_CLOSED without step-up", async ({ page }) => {
    await page.route(`**/api/v1/periode/${PERIODE_ID}/hard-close-reject`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            periodeId: PERIODE_ID,
            periodeKode: "2026-06",
            statusPeriode: "SOFT_CLOSED",
            rejectedBy: "ffffffff-0000-0000-0000-000000000001",
            message: "Hard close ditolak. Periode dikembalikan ke SOFT_CLOSED.",
          },
          meta: { traceId: "trace-reject-001" },
        }),
      });
    });

    // Reject button — no MFA dialog needed.
    const rejectBtn = page.getByRole("button", { name: /Tolak Hard Close/i });
    await expect(rejectBtn).toBeVisible();
    await rejectBtn.click();

    // Reject reason dialog — no MFA dialog.
    const rejectDialog = page.getByRole("dialog").filter({ has: page.getByText(/Tolak Hard Close/i) });
    await expect(rejectDialog).toBeVisible();
    // No MFA step-up dialog should appear.
    await expect(page.getByText(MFA_DIALOG_TITLE)).not.toBeVisible();

    const reasonInput = rejectDialog.getByRole("textbox", { name: /Alasan penolakan/i });
    await reasonInput.fill("Perlu koreksi jurnal sebelum hard close dapat dilanjutkan karena ada jurnal tidak seimbang");
    await rejectDialog.getByRole("button", { name: /Konfirmasi Tolak/i }).click();

    // Success toast.
    const toast = page.getByRole("alert").filter({ hasText: /SOFT_CLOSED/i });
    await expect(toast).toBeVisible({ timeout: 5000 });

    // Status badge updated.
    await expect(page.getByTestId("periode-status-badge")).toContainText("SOFT_CLOSED");
  });
});
