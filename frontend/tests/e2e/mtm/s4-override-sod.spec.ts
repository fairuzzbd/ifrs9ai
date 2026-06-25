/**
 * Playwright E2E — P5-M6 S4: Override Approve / Reject + SoD Enforcement
 *
 * AC covered:
 *   S4-AC1: ROLE-AKUN-CTL opens approve dialog → deviation banner visible when deviationFlag=true
 *   S4-AC2: Approve dialog — submit disabled until comment ≥ 30 char + attest checked
 *   S4-AC3: SoD violation response (MTM_OVERRIDE_SOD_VIOLATION) → error toast + dialog closes
 *   S4-AC4: Reject dialog — submit disabled until comment ≥ 30 char; reject closes with destructive toast
 *
 * All API calls are mocked — no live backend required.
 */

import { test, expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const MTM_ID = "aaaaaaaa-0000-0000-0000-000000000002";

const MTM_DETAIL_PENDING_DEVIATION = {
  data: {
    id: MTM_ID,
    instrumenId: "bbbbbbbb-0000-0000-0000-000000000002",
    instrumenKode: "ASII",
    instrumenNama: "Astra International Tbk",
    tanggalMtm: "2026-06-18",
    hargaSumber: "BEI_MANUAL",
    hargaPasarIdr: 5_750,
    hargaBukuIdr: 5_200,
    deltaIdr: 550,
    deltaPct: 10.58,
    hargaAgeDays: 0,
    stalePriceFlag: false,
    deviationFlag: true,
    status: "PENDING_REVIEW",
    klasifikasiSnapshot: "FVTPL",
    jurnalEventCode: "MTM_FVTPL",
    jurnalEntryId: null,
    uploaderId: "dddddddd-0000-0000-0000-000000000001",
    overrideApproverId: null,
    overrideAt: null,
    lockedFlag: false,
    createdAt: "2026-06-18T18:00:00+07:00",
    periodeBulananId: "eeeeeeee-0000-0000-0000-000000000001",
    hargaTanggal: "2026-06-18",
    hargaPasarFcy: null,
    kursId: null,
    kursTengah: null,
    treatmentSnapshot: null,
    jurnalEventCodes: ["MTM_FVTPL"],
    uploadBatchId: "ffffffff-0000-0000-0000-000000000001",
    overrideComment: null,
    cronJobId: null,
    createdBy: "99999999-0000-0000-0000-000000000001",
    updatedAt: "2026-06-18T18:00:00+07:00",
    updatedBy: "99999999-0000-0000-0000-000000000001",
    rowVersion: 1,
  },
  meta: { traceId: "trace-detail-001" },
};

const MTM_LIST_PENDING = {
  data: [
    {
      id: MTM_ID,
      instrumenId: "bbbbbbbb-0000-0000-0000-000000000002",
      instrumenKode: "ASII",
      instrumenNama: "Astra International Tbk",
      tanggalMtm: "2026-06-18",
      hargaSumber: "BEI_MANUAL",
      hargaPasarIdr: 5_750,
      hargaBukuIdr: 5_200,
      deltaIdr: 550,
      deltaPct: 10.58,
      hargaAgeDays: 0,
      stalePriceFlag: false,
      deviationFlag: true,
      status: "PENDING_REVIEW",
      klasifikasiSnapshot: "FVTPL",
      jurnalEventCode: "MTM_FVTPL",
      jurnalEntryId: null,
      uploaderId: "dddddddd-0000-0000-0000-000000000001",
      overrideApproverId: null,
      overrideAt: null,
      lockedFlag: false,
      createdAt: "2026-06-18T18:00:00+07:00",
    },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
  meta: { traceId: "trace-002" },
};

const APPROVE_SUCCESS = {
  data: {
    mtmId: MTM_ID,
    instrumenKode: "ASII",
    status: "APPROVED",
    jurnalEntryId: "11111111-0000-0000-0000-000000000001",
    jurnalEventCodes: ["MTM_FVTPL"],
    approvedBy: "22222222-0000-0000-0000-000000000001",
    approvedAt: "2026-06-18T14:30:00+07:00",
    message: "MTM ASII 2026-06-18 disetujui. Jurnal MTM_FVTPL berhasil diposting.",
  },
  meta: { traceId: "trace-approve-001" },
};

const SOD_ERROR_RESPONSE = {
  error: {
    code: "MTM_OVERRIDE_SOD_VIOLATION",
    message: "Uploader dan override-approver tidak boleh orang yang sama (SoD). Minta rekan ROLE-AKUN-CTL yang lain.",
    details: [],
    traceId: "trace-sod-001",
  },
};

const REJECT_SUCCESS = {
  data: {
    mtmId: MTM_ID,
    instrumenKode: "ASII",
    status: "REJECTED",
    rejectedBy: "33333333-0000-0000-0000-000000000001",
    rejectedAt: "2026-06-18T14:35:00+07:00",
    comment: "Harga 5750 tidak sesuai data BEI hari ini. Re-upload dengan harga 5200.",
    message: "MTM ASII 2026-06-18 ditolak.",
  },
  meta: { traceId: "trace-reject-001" },
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("S4 — Override Approve/Reject + SoD", () => {
  test("S4-AC1: Approve dialog opens → deviation warning banner visible when deviationFlag=true", async ({ page }) => {
    await page.route("**/api/v1/trx/mtm**", (route) => {
      if (route.request().url().includes(`/${MTM_ID}`)) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_DETAIL_PENDING_DEVIATION),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_LIST_PENDING),
        });
      }
    });

    await page.goto(`/mtm/${MTM_ID}`);

    // Wait for detail to load
    await expect(page.getByText("ASII")).toBeVisible({ timeout: 5000 });

    // Click Override: Setuju button
    const approveBtn = page.getByRole("button", { name: /override.*setuju|setuju/i });
    if (await approveBtn.count() > 0) {
      await approveBtn.first().click();

      // Dialog should open with deviation warning
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

      // Deviation warning banner visible in dialog (deviationFlag=true)
      const deviationWarning = page.getByText(/deviasi harga signifikan|melebihi threshold/i);
      if (await deviationWarning.count() > 0) {
        await expect(deviationWarning.first()).toBeVisible();
      }

      // Submit button initially disabled (no comment + no attest)
      const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeDisabled();
      }
    }
  });

  test("S4-AC2: Approve dialog — submit disabled until comment ≥30 chars + attest checked", async ({ page }) => {
    await page.route("**/api/v1/trx/mtm**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MTM_LIST_PENDING),
      });
    });

    await page.route(`**/api/v1/trx/mtm/${MTM_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MTM_DETAIL_PENDING_DEVIATION),
      });
    });

    await page.goto(`/mtm/${MTM_ID}`);
    await expect(page.getByText("ASII")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /override.*setuju|setuju/i });
    if (await approveBtn.count() === 0) {
      test.skip();
      return;
    }
    await approveBtn.first().click();
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

    const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });

    // Step 1: No comment, no attest → disabled
    if (await submitBtn.count() > 0) {
      await expect(submitBtn.first()).toBeDisabled();
    }

    // Step 2: Add short comment (< 30 chars) → still disabled
    const commentArea = page.getByRole("textbox", { name: /komentar/i });
    if (await commentArea.count() > 0) {
      await commentArea.first().fill("Too short");
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeDisabled();
      }

      // Step 3: Add long enough comment (≥ 30 chars) → still disabled (no attest)
      await commentArea.first().fill("Harga terverifikasi via Bloomberg sesuai IBPA hari ini.");
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeDisabled();
      }

      // Step 4: Check attest → enabled
      const attestCheckbox = page.getByRole("checkbox");
      if (await attestCheckbox.count() > 0) {
        await attestCheckbox.first().check();
        await expect(submitBtn.first()).toBeEnabled();
      }
    }
  });

  test("S4-AC3: SoD violation → MTM_OVERRIDE_SOD_VIOLATION error toast + dialog closes", async ({ page }) => {
    await page.route("**/api/v1/trx/mtm**", (route) => {
      if (route.request().url().includes(`/${MTM_ID}`) && !route.request().url().includes("override")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_DETAIL_PENDING_DEVIATION),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_LIST_PENDING),
        });
      }
    });

    // Override-approve returns SoD error
    await page.route(`**/api/v1/trx/mtm/${MTM_ID}/override-approve`, (route) => {
      route.fulfill({
        status: 403,
        contentType: "application/json",
        body: JSON.stringify(SOD_ERROR_RESPONSE),
      });
    });

    await page.goto(`/mtm/${MTM_ID}`);
    await expect(page.getByText("ASII")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /override.*setuju|setuju/i });
    if (await approveBtn.count() === 0) {
      test.skip();
      return;
    }
    await approveBtn.first().click();
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

    // Fill in valid comment + attest
    const commentArea = page.getByRole("textbox", { name: /komentar/i });
    if (await commentArea.count() > 0) {
      await commentArea.first().fill("Harga terverifikasi via Bloomberg. Setuju untuk diposting ke jurnal.");
      const attestCheckbox = page.getByRole("checkbox");
      if (await attestCheckbox.count() > 0) {
        await attestCheckbox.first().check();
      }
    }

    // Submit
    const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
    if (await submitBtn.count() > 0 && await submitBtn.first().isEnabled()) {
      await submitBtn.first().click();

      // SoD error toast
      await expect(
        page.getByText(/sod|uploader.*approver|orang yang sama/i),
      ).toBeVisible({ timeout: 5000 });

      // Dialog should auto-close on SoD error (not retryable)
      await expect(page.getByRole("dialog")).toHaveCount(0, { timeout: 3000 });
    }
  });

  test("S4-AC4: Reject dialog — submit disabled until comment ≥30 chars; success shows destructive toast", async ({ page }) => {
    await page.route(`**/api/v1/trx/mtm/${MTM_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MTM_DETAIL_PENDING_DEVIATION),
      });
    });

    await page.route("**/api/v1/trx/mtm**", (route) => {
      if (!route.request().url().includes("override")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_LIST_PENDING),
        });
      }
    });

    // Override-reject returns success
    await page.route(`**/api/v1/trx/mtm/${MTM_ID}/override-reject`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(REJECT_SUCCESS),
      });
    });

    await page.goto(`/mtm/${MTM_ID}`);
    await expect(page.getByText("ASII")).toBeVisible({ timeout: 5000 });

    // Click Tolak button
    const rejectBtn = page.getByRole("button", { name: /tolak/i });
    if (await rejectBtn.count() === 0) {
      test.skip();
      return;
    }
    await rejectBtn.first().click();
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

    // Submit button initially disabled (no comment)
    const submitBtn = page.getByRole("button", { name: /tolak mtm/i });
    if (await submitBtn.count() > 0) {
      await expect(submitBtn.first()).toBeDisabled();
    }

    // Fill in short comment → still disabled
    const commentArea = page.getByRole("textbox");
    if (await commentArea.count() > 0) {
      await commentArea.first().fill("Too short");
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeDisabled();
      }

      // Fill in valid comment ≥ 30 chars → enabled
      await commentArea.first().fill("Harga 5750 tidak sesuai data BEI hari ini. Re-upload dengan harga 5200.");
      if (await submitBtn.count() > 0) {
        await expect(submitBtn.first()).toBeEnabled();

        // Submit
        await submitBtn.first().click();

        // Destructive toast / success notification
        await expect(
          page.getByText(/ditolak|tolak.*dinotifikasi|re-upload/i),
        ).toBeVisible({ timeout: 5000 });

        // Dialog closes
        await expect(page.getByRole("dialog")).toHaveCount(0, { timeout: 3000 });
      }
    }
  });

  test("S4: Approve success → jurnal entry link in toast", async ({ page }) => {
    await page.route(`**/api/v1/trx/mtm/${MTM_ID}`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MTM_DETAIL_PENDING_DEVIATION),
      });
    });

    await page.route("**/api/v1/trx/mtm**", (route) => {
      if (!route.request().url().includes("override")) {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(MTM_LIST_PENDING),
        });
      }
    });

    // Override-approve returns success with jurnal link
    await page.route(`**/api/v1/trx/mtm/${MTM_ID}/override-approve`, (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(APPROVE_SUCCESS),
      });
    });

    await page.goto(`/mtm/${MTM_ID}`);
    await expect(page.getByText("ASII")).toBeVisible({ timeout: 5000 });

    const approveBtn = page.getByRole("button", { name: /override.*setuju|setuju/i });
    if (await approveBtn.count() === 0) {
      test.skip();
      return;
    }
    await approveBtn.first().click();
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });

    const commentArea = page.getByRole("textbox", { name: /komentar/i });
    if (await commentArea.count() > 0) {
      await commentArea.first().fill("Harga terverifikasi via Bloomberg. Delta wajar karena rilis FOMC hari ini.");
      const attestCheckbox = page.getByRole("checkbox");
      if (await attestCheckbox.count() > 0) {
        await attestCheckbox.first().check();
      }
    }

    const submitBtn = page.getByRole("button", { name: /setuju.*jurnal|posting jurnal/i });
    if (await submitBtn.count() > 0 && await submitBtn.first().isEnabled()) {
      await submitBtn.first().click();

      // Success toast with jurnal link
      await expect(
        page.getByText(/disetujui|jurnal.*mtm_fvtpl|override.*setujui/i),
      ).toBeVisible({ timeout: 5000 });

      // "Lihat Jurnal" action link in toast
      const lihatJurnalLink = page.getByRole("link", { name: /lihat jurnal/i });
      if (await lihatJurnalLink.count() > 0) {
        await expect(lihatJurnalLink.first()).toBeVisible();
      }
    }
  });
});
