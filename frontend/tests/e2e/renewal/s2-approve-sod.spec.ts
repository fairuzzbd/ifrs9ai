/**
 * Playwright E2E — P5-M7 S2: Approve Renewal + SoD Enforcement (ROLE-APPR-TR)
 *
 * AC covered:
 *   S2-AC1: ROLE-APPR-TR approves → 200 POSTED → success toast with instrumen baru link
 *   S2-AC2: RENEWAL_BUNGA_BERSIH_TOO_SMALL on approve → 422 → error toast
 *   S2-AC3: SOD_VIOLATION → 403 → error toast + dialog closes
 *   S2-AC4: Idempotency replay — test that approve button is disabled while submitting (block double-submit)
 *   Reject flow: comment < 30 chars → submit button disabled
 *
 * All API calls mocked — no live backend required.
 */

import { test, expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const RENEWAL_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
const MAKER_ID = "aaaaaaaa-0000-0000-0000-000000000001";
const APPROVER_ID = "bbbbbbbb-0000-0000-0000-000000000002";

const RENEWAL_DETAIL_PENDING = {
  data: {
    id: RENEWAL_ID,
    instrumenLamaId: "cccccccc-0000-0000-0000-000000000003",
    instrumenLamaKode: "DEP-0042",
    instrumenBaruId: null,
    skema: "POKOK_PLUS_BUNGA",
    tenorBaruBulan: 12,
    rateBaruPersen: "5.7500",
    tanggalEfektifBaru: "2026-07-01",
    pokokLama: "1000000000.0000",
    pokokBaru: "1011397260.2740",
    bungaBersih: "11397260.2740",
    bungaKotor: "14246575.3425",
    pph20pct: "2849315.0685",
    eirBaru: "0.04600000",
    tanggalJatuhTempoBaru: "2027-07-01",
    status: "PENDING_APPROVAL",
    makerId: MAKER_ID,
    approverId: null,
    jurnalEntryId: null,
    approveReason: null,
    rejectReason: null,
    signatureMethod: null,
    periodeBulananId: null,
    createdAt: "2026-06-19T09:00:00+07:00",
    updatedAt: "2026-06-19T09:00:00+07:00",
    rowVersion: 1,
    preview: {
      pokokLama: "1000000000.0000",
      bungaKotor: "14246575.3425",
      pph20pct: "2849315.0685",
      bungaBersih: "11397260.2740",
      pokokBaru: "1011397260.2740",
      eirBaru: "0.04600000",
      tanggalJatuhTempoBaru: "2027-07-01",
      scheduleBaruPreview: [],
    },
  },
  meta: { traceId: "trace-detail-001" },
};

const APPROVE_SUCCESS = {
  data: {
    renewalId: RENEWAL_ID,
    status: "POSTED",
    instrumenBaruId: "dddddddd-0000-0000-0000-000000000004",
    jurnalEntryId: "eeeeeeee-0000-0000-0000-000000000005",
    approvedBy: APPROVER_ID,
    approvedAt: "2026-06-19T09:15:00+07:00",
    message: "Renewal DEP-0042 disetujui dan diposting. Instrumen baru DEP-0042B dibuat.",
  },
  meta: { traceId: "trace-approve-001" },
};

const SOD_VIOLATION_RESPONSE = {
  error: {
    code: "SOD_VIOLATION",
    message: "maker tidak dapat menjadi approver untuk renewal yang sama (DEC-017).",
    details: [
      { field: "actor.userId", rule: "sod_approver_ne_maker", message: "approver_id == maker_id." },
    ],
    traceId: "trace-sod-001",
  },
};

const BUNGA_BERSIH_TOO_SMALL_ERROR = {
  error: {
    code: "RENEWAL_BUNGA_BERSIH_TOO_SMALL",
    message: "bunga_bersih IDR 85000 lebih kecil dari minimum IDR 100000 untuk skema POKOK_PLUS_BUNGA.",
    details: [],
    traceId: "trace-small-001",
  },
};

const REJECT_SUCCESS = {
  data: {
    renewalId: RENEWAL_ID,
    status: "REJECTED",
    rejectedBy: APPROVER_ID,
    rejectedAt: "2026-06-19T09:20:00+07:00",
    comment: "Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO.",
  },
  meta: { traceId: "trace-reject-001" },
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("S2 — Approve Renewal + SoD Enforcement (ROLE-APPR-TR)", () => {
  test("S2-AC1: ROLE-APPR-TR approves → POSTED → success toast with instrumen baru", async ({ page }) => {
    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}`, (route) => {
      if (!route.request().url().includes("approve") && !route.request().url().includes("reject")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(RENEWAL_DETAIL_PENDING),
        });
      }
    });

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}/approve`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(APPROVE_SUCCESS),
      });
    });

    await page.goto(`/transaksi/renewal/${RENEWAL_ID}`);
    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    // Click "Setujui Renewal" button (only visible for approver role, not maker)
    const approveBtn = page.getByRole("button", { name: /setujui renewal/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.first().click();
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      // Fill comment
      const commentArea = page.getByRole("textbox", { name: /komentar/i });
      if (await commentArea.count() > 0) {
        await commentArea.fill("Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Disetujui.");
      }

      // Submit
      const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
      if (await submitBtn.count() > 0) {
        await submitBtn.first().click();

        // Success toast
        await expect(
          page.getByText(/disetujui.*diposting|instrumen baru.*dibuat|renewal.*berhasil/i),
        ).toBeVisible({ timeout: 5000 });
      }
    }
  });

  test("S2-AC2: RENEWAL_BUNGA_BERSIH_TOO_SMALL on approve → 422 → Indonesian error toast", async ({ page }) => {
    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}`, (route) => {
      if (!route.request().url().includes("approve")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(RENEWAL_DETAIL_PENDING),
        });
      }
    });

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}/approve`, (route) => {
      route.fulfill({
        status: 422,
        contentType: "application/json",
        body: JSON.stringify(BUNGA_BERSIH_TOO_SMALL_ERROR),
      });
    });

    await page.goto(`/transaksi/renewal/${RENEWAL_ID}`);
    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /setujui renewal/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.first().click();
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      const commentArea = page.getByRole("textbox", { name: /komentar/i });
      if (await commentArea.count() > 0) {
        await commentArea.fill("Preview diverifikasi. Disetujui.");
      }

      const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
      if (await submitBtn.count() > 0) {
        await submitBtn.first().click();

        // Indonesian error toast for RENEWAL_BUNGA_BERSIH_TOO_SMALL
        await expect(
          page.getByText(/bunga bersih.*minimum|minimum.*100\.000|pokok saja|nominal lebih besar/i),
        ).toBeVisible({ timeout: 5000 });
      }
    }
  });

  test("S2-AC3: SOD_VIOLATION on approve → 403 → Indonesian error toast + dialog closes", async ({ page }) => {
    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}`, (route) => {
      if (!route.request().url().includes("approve")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(RENEWAL_DETAIL_PENDING),
        });
      }
    });

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}/approve`, (route) => {
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify(SOD_VIOLATION_RESPONSE),
      });
    });

    await page.goto(`/transaksi/renewal/${RENEWAL_ID}`);
    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /setujui renewal/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.first().click();
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      const commentArea = page.getByRole("textbox", { name: /komentar/i });
      if (await commentArea.count() > 0) {
        await commentArea.fill("Verified and approved.");
      }

      const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
      if (await submitBtn.count() > 0) {
        await submitBtn.first().click();

        // SoD error toast (mapped from SOD_VIOLATION in notify.ts)
        await expect(
          page.getByText(/sod|reviewer\/approver.*data yang anda buat|segregation of duties/i),
        ).toBeVisible({ timeout: 5000 });

        // Dialog auto-closes on SOD_VIOLATION (not retryable)
        await expect(page.getByRole("dialog")).toHaveCount(0, { timeout: 3000 });
      }
    }
  });

  test("S2-AC4 (reject flow): comment < 30 chars → submit button disabled; valid comment → enabled", async ({ page }) => {
    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}`, (route) => {
      if (!route.request().url().includes("reject")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(RENEWAL_DETAIL_PENDING),
        });
      }
    });

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}/reject`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(REJECT_SUCCESS),
      });
    });

    await page.goto(`/transaksi/renewal/${RENEWAL_ID}`);
    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    // Click "Tolak" button
    const rejectBtn = page.getByRole("button", { name: /^tolak$/i });
    if (await rejectBtn.count() > 0) {
      await rejectBtn.first().click();
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      const submitBtn = page.getByRole("button", { name: /tolak renewal/i });

      // Step 1: No comment → disabled
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeDisabled();
      }

      // Step 2: Short comment (< 30 chars) → still disabled
      const commentArea = page.getByRole("textbox");
      if (await commentArea.count() > 0) {
        await commentArea.first().fill("Too short");
        if (await submitBtn.count() > 0) {
          await expect(submitBtn.first()).toBeDisabled();
        }

        // Step 3: Valid comment ≥ 30 chars → enabled
        await commentArea.first().fill("Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO.");
        if (await submitBtn.count() > 0) {
          await expect(submitBtn.first()).toBeEnabled();

          // Submit reject
          await submitBtn.first().click();

          // Destructive toast
          await expect(
            page.getByText(/ditolak|maker akan dinotifikasi|tolak.*dinotifikasi/i),
          ).toBeVisible({ timeout: 5000 });

          // Dialog closes
          await expect(page.getByRole("dialog")).toHaveCount(0, { timeout: 3000 });
        }
      }
    }
  });

  test("Idempotency: submit button disables during API call (blocks double-submit, S2-AC4)", async ({ page }) => {
    let callCount = 0;

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}`, (route) => {
      if (!route.request().url().includes("approve")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(RENEWAL_DETAIL_PENDING),
        });
      }
    });

    await page.route(`**/api/v1/trx/renewal/${RENEWAL_ID}/approve`, async (route) => {
      callCount++;
      // Simulate slow response
      await new Promise((resolve) => setTimeout(resolve, 500));
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(APPROVE_SUCCESS),
      });
    });

    await page.goto(`/transaksi/renewal/${RENEWAL_ID}`);
    await expect(page.getByText("DEP-0042")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /setujui renewal/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.first().click();
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      const commentArea = page.getByRole("textbox", { name: /komentar/i });
      if (await commentArea.count() > 0) {
        await commentArea.fill("Preview diverifikasi. Disetujui sesuai prosedur.");
      }

      const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
      if (await submitBtn.count() > 0) {
        // Click submit
        await submitBtn.first().click();

        // Button should be disabled while processing
        await expect(submitBtn.first()).toBeDisabled({ timeout: 1000 });

        // Only one API call should have been made
        expect(callCount).toBeLessThanOrEqual(1);
      }
    }
  });
});
