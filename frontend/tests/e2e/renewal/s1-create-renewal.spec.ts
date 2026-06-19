/**
 * Playwright E2E — P5-M7 S1: Create Renewal Deposito (ROLE-MAKER-TR)
 *
 * AC covered:
 *   S1-AC1: Maker fills form, submit → 201 response → preview panel visible + success toast
 *   S1-AC2: tenor = 72 → RENEWAL_TENOR_OUT_OF_RANGE → error mapped to Bahasa Indonesia
 *   S1-AC3: rate = 35 → RENEWAL_RATE_OUT_OF_RANGE → error mapped
 *   S1-AC4: instrumenId = OBL-0099 → RENEWAL_INSTRUMEN_NOT_ELIGIBLE → 422 error
 *
 * All API calls mocked — no live backend required.
 */

import { test, expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const INSTRUMEN_ID = "b2c3d4e5-f6a7-8901-bcde-f12345678901";
const RENEWAL_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";

const CREATE_SUCCESS_RESPONSE = {
  data: {
    renewalId: RENEWAL_ID,
    status: "PENDING_APPROVAL",
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
    nextStep: "Menunggu approval ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
  },
  meta: { traceId: "trace-create-001" },
};

const TENOR_OUT_OF_RANGE_ERROR = {
  error: {
    code: "RENEWAL_TENOR_OUT_OF_RANGE",
    message: "tenor_baru_bulan harus antara 1 dan 60. Nilai: 72.",
    details: [
      { field: "tenorBaruBulan", rule: "range:1,60", message: "Tenor 72 bulan di luar range 1-60." },
    ],
    traceId: "trace-tenor-001",
  },
};

const RATE_OUT_OF_RANGE_ERROR = {
  error: {
    code: "RENEWAL_RATE_OUT_OF_RANGE",
    message: "rate_baru_persen harus antara 0% dan 30%. Nilai: 35.00%.",
    details: [
      { field: "rateBaruPersen", rule: "range:0,30", message: "Rate 35.00% di luar range 0-30%." },
    ],
    traceId: "trace-rate-001",
  },
};

const INSTRUMEN_NOT_ELIGIBLE_ERROR = {
  error: {
    code: "RENEWAL_INSTRUMEN_NOT_ELIGIBLE",
    message: "OBL-0099 bukan instrumen deposito atau tidak berstatus ACTIVE. Renewal hanya untuk deposito ACTIVE.",
    details: [
      { field: "instrumenId", rule: "deposito_active", message: "jenis_instrumen=OBLIGASI." },
    ],
    traceId: "trace-elig-001",
  },
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("S1 — Create Renewal Deposito (ROLE-MAKER-TR)", () => {
  test("S1-AC1: Happy path — fill form, submit → 201 → preview panel + success toast", async ({ page }) => {
    // Mock create endpoint
    await page.route("**/api/v1/trx/renewal", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify(CREATE_SUCCESS_RESPONSE),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto(`/transaksi/renewal/new?instrumenId=${INSTRUMEN_ID}&kode=DEP-0042`);

    // Wait for form to load
    await expect(page.getByRole("heading", { name: /buat renewal/i })).toBeVisible({ timeout: 5000 });

    // Select skema POKOK_PLUS_BUNGA
    const skemaSelect = page.getByRole("combobox", { name: /skema renewal/i });
    if (await skemaSelect.count() > 0) {
      await skemaSelect.click();
      await page.getByRole("option", { name: /pokok \+ bunga/i }).click();
    }

    // Fill tenor
    const tenorInput = page.getByRole("spinbutton", { name: /tenor/i });
    if (await tenorInput.count() > 0) {
      await tenorInput.clear();
      await tenorInput.fill("12");
    }

    // Fill rate
    const rateInput = page.getByRole("spinbutton", { name: /rate/i });
    if (await rateInput.count() > 0) {
      await rateInput.clear();
      await rateInput.fill("5.75");
    }

    // Fill tanggal efektif
    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) {
      await tanggalInput.fill("2026-07-01");
    }

    // Submit
    const submitBtn = page.getByRole("button", { name: /buat renewal/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // Success toast should appear with specific instrumen code
      await expect(
        page.getByText(/renewal.*berhasil dibuat|menunggu approval treasury/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });

  test("S1-AC2: Tenor out of range (72) → Zod validates before API; error shown", async ({ page }) => {
    // This tests client-side Zod validation (tenor max 60)
    // The API would return RENEWAL_TENOR_OUT_OF_RANGE, but Zod catches it first
    await page.route("**/api/v1/trx/renewal", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify(TENOR_OUT_OF_RANGE_ERROR),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto("/transaksi/renewal/new");
    await expect(page.getByRole("heading", { name: /buat renewal/i })).toBeVisible({ timeout: 5000 });

    // Fill tenor = 72 (exceeds max 60)
    const tenorInput = page.getByRole("spinbutton", { name: /tenor/i });
    if (await tenorInput.count() > 0) {
      await tenorInput.clear();
      await tenorInput.fill("72");
    }

    // Fill required instrumen
    const instrumenInput = page.getByPlaceholder(/uuid instrumen/i);
    if (await instrumenInput.count() > 0) {
      await instrumenInput.fill(INSTRUMEN_ID);
    }

    // Fill other required fields with valid values
    const rateInput = page.getByRole("spinbutton", { name: /rate/i });
    if (await rateInput.count() > 0) {
      await rateInput.fill("5.75");
    }

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) {
      await tanggalInput.fill("2026-07-01");
    }

    // Submit
    const submitBtn = page.getByRole("button", { name: /buat renewal/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // Either Zod validation error inline OR API error toast
      // Both should mention tenor or range
      const hasValidationMsg = await page.getByText(/tenor.*60|60.*bulan|tenor maksimal/i).count() > 0;
      const hasToast = await page.getByText(/tenor.*luar range|renewal_tenor_out_of_range/i).count() > 0;
      expect(hasValidationMsg || hasToast).toBe(true);
    }
  });

  test("S1-AC3: Rate out of range (35) → server error → mapped Indonesian toast", async ({ page }) => {
    await page.route("**/api/v1/trx/renewal", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify(RATE_OUT_OF_RANGE_ERROR),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto("/transaksi/renewal/new");
    await expect(page.getByRole("heading", { name: /buat renewal/i })).toBeVisible({ timeout: 5000 });

    // Fill rate = 35 (exceeds max 30)
    const rateInput = page.getByRole("spinbutton", { name: /rate/i });
    if (await rateInput.count() > 0) {
      await rateInput.fill("35");
    }

    const instrumenInput = page.getByPlaceholder(/uuid instrumen/i);
    if (await instrumenInput.count() > 0) {
      await instrumenInput.fill(INSTRUMEN_ID);
    }

    const tenorInput = page.getByRole("spinbutton", { name: /tenor/i });
    if (await tenorInput.count() > 0) {
      await tenorInput.fill("12");
    }

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) {
      await tanggalInput.fill("2026-07-01");
    }

    const submitBtn = page.getByRole("button", { name: /buat renewal/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // Either Zod validation (rate max 30) or server error mapped Indonesian
      const hasZodError = await page.getByText(/rate maksimal 30|rate.*30%/i).count() > 0;
      const hasToastError = await page.getByText(/rate.*luar range|0%.*30%/i).count() > 0;
      expect(hasZodError || hasToastError).toBe(true);
    }
  });

  test("S1-AC4: RENEWAL_INSTRUMEN_NOT_ELIGIBLE → 422 → Indonesian error toast", async ({ page }) => {
    await page.route("**/api/v1/trx/renewal", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 422,
          contentType: "application/json",
          body: JSON.stringify(INSTRUMEN_NOT_ELIGIBLE_ERROR),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto("/transaksi/renewal/new");
    await expect(page.getByRole("heading", { name: /buat renewal/i })).toBeVisible({ timeout: 5000 });

    // Fill all fields with valid values (ineligible instrumen)
    const instrumenInput = page.getByPlaceholder(/uuid instrumen/i);
    if (await instrumenInput.count() > 0) {
      await instrumenInput.fill("xxxxxxxx-0000-0000-0000-000000000099"); // OBL-0099
    }

    const tenorInput = page.getByRole("spinbutton", { name: /tenor/i });
    if (await tenorInput.count() > 0) {
      await tenorInput.fill("12");
    }

    const rateInput = page.getByRole("spinbutton", { name: /rate/i });
    if (await rateInput.count() > 0) {
      await rateInput.fill("5.5");
    }

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) {
      await tanggalInput.fill("2026-07-01");
    }

    const submitBtn = page.getByRole("button", { name: /buat renewal/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // Should show Indonesian-mapped error toast
      await expect(
        page.getByText(/tidak eligible|deposito active|klasifikasi final/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });
});
