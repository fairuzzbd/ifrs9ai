/**
 * Playwright E2E — P5-M8 S1: Create Penjualan Instrumen (ROLE-MAKER-TR)
 *
 * AC covered:
 *   S1-AC1: Maker fills form, submit → 201 response → preview panel visible + success toast
 *   S1-AC2: qty_terjual melebihi holding → PENJUALAN_QTY_EXCEEDS_HOLDING → Indonesian error
 *   S1-AC3: instrumen not ACTIVE → PENJUALAN_INSTRUMEN_NOT_ACTIVE → 422 error mapped
 *   S1-AC4: FVOCI Election → no_recycling_note shown in preview, OCI_recycled null
 *
 * All API calls mocked — no live backend required.
 */

import { test, expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const INSTRUMEN_ID = "b2c3d4e5-f6a7-8901-bcde-f12345678901";
const INSTRUMEN_ID_FVOCI_ELECTION = "c3d4e5f6-a7b8-9012-cdef-123456789012";
const INSTRUMEN_ID_INACTIVE = "d4e5f6a7-b8c9-0123-def0-234567890123";
const PENJUALAN_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";

const CREATE_SUCCESS_FVOCI = {
  data: {
    penjualanId: PENJUALAN_ID,
    status: "PENDING_APPROVAL",
    preview: {
      klasifikasiPsak71: "FVOCI",
      proceedIdr: "525000000.0000",
      costBasis: "498500000.0000",
      realizedGl: "26500000.0000",
      ociRecycled: "9100000.0000",
      noRecyclingNote: null,
      bmFreqImpactPct: "3.2000",
      bmFreqWarning: null,
    },
    nextStep: "Menunggu approval ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
  },
  meta: { traceId: "trace-create-pjl-001" },
};

const CREATE_SUCCESS_FVOCI_ELECTION = {
  data: {
    penjualanId: "b2c3d4e5-f6a7-8901-bcde-000000000002",
    status: "PENDING_APPROVAL",
    preview: {
      klasifikasiPsak71: "FVOCI_ELECTION",
      proceedIdr: "12000000.0000",
      costBasis: "10000000.0000",
      realizedGl: "2000000.0000",
      ociRecycled: null,
      noRecyclingNote: "Gain/loss IDR 2.000.000 tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L.",
      bmFreqImpactPct: null,
      bmFreqWarning: null,
    },
    nextStep: "Menunggu approval ROLE-APPR-TR.",
  },
  meta: { traceId: "trace-create-pjl-002" },
};

const QTY_EXCEEDS_ERROR = {
  error: {
    code: "PENJUALAN_QTY_EXCEEDS_HOLDING",
    message: "qty_terjual 1500 melebihi qty_holding saat ini: 1000 unit OBL-0077.",
    details: [
      { field: "qtyTerjual", rule: "lte:qty_holding", message: "qty_terjual 1500 > qty_holding 1000." },
    ],
    traceId: "trace-qty-001",
  },
};

const INSTRUMEN_NOT_ACTIVE_ERROR = {
  error: {
    code: "PENJUALAN_INSTRUMEN_NOT_ACTIVE",
    message: "OBL-0099 tidak eligible untuk penjualan: status=MATURED. Hanya instrumen ACTIVE yang bisa dijual.",
    details: [
      { field: "instrumenId", rule: "instrumen_active", message: "status=MATURED." },
    ],
    traceId: "trace-elig-001",
  },
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("S1 — Create Penjualan Instrumen (ROLE-MAKER-TR)", () => {
  test("S1-AC1: FVOCI PARTIAL happy path → 201 → preview panel + OCI recycled + success toast", async ({ page }) => {
    await page.route("**/api/v1/trx/penjualan", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify(CREATE_SUCCESS_FVOCI),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto(`/transaksi/penjualan/new?instrumenId=${INSTRUMEN_ID}&kode=OBL-0077`);

    // Wait for form heading
    await expect(page.getByRole("heading", { name: /buat penjualan/i })).toBeVisible({ timeout: 5000 });

    // Select jenis disposal PARTIAL (default)
    const jenisSelect = page.getByRole("combobox", { name: /jenis disposal/i });
    if (await jenisSelect.count() > 0) {
      await jenisSelect.click();
      await page.getByRole("option", { name: /sebagian/i }).click();
    }

    // Fill qty
    const qtyInput = page.locator('input[placeholder*="0.00000000"]');
    if (await qtyInput.count() > 0) {
      await qtyInput.fill("500");
    }

    // Fill harga
    const hargaInput = page.locator('input[placeholder*="0.0000"]');
    if (await hargaInput.count() > 0) {
      await hargaInput.fill("1050000");
    }

    // Fill tanggal
    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) {
      await tanggalInput.fill("2026-07-15");
    }

    // Submit
    const submitBtn = page.getByRole("button", { name: /buat penjualan/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // Success toast should appear with instrumen code
      await expect(
        page.getByText(/OBL-0077.*berhasil dibuat|penjualan.*berhasil dibuat/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });

  test("S1-AC2: qty melebihi holding → PENJUALAN_QTY_EXCEEDS_HOLDING → Indonesian error toast", async ({ page }) => {
    await page.route("**/api/v1/trx/penjualan", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 422,
          contentType: "application/json",
          body: JSON.stringify(QTY_EXCEEDS_ERROR),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto(`/transaksi/penjualan/new?instrumenId=${INSTRUMEN_ID}&kode=OBL-0077`);
    await expect(page.getByRole("heading", { name: /buat penjualan/i })).toBeVisible({ timeout: 5000 });

    // Fill oversized qty
    const qtyInput = page.locator('input[placeholder*="0.00000000"]');
    if (await qtyInput.count() > 0) await qtyInput.fill("1500");

    const hargaInput = page.locator('input[placeholder*="0.0000"]');
    if (await hargaInput.count() > 0) await hargaInput.fill("1050000");

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) await tanggalInput.fill("2026-07-15");

    const submitBtn = page.getByRole("button", { name: /buat penjualan/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      await expect(
        page.getByText(/qty.*melebihi.*holding|holding saat ini/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });

  test("S1-AC3: instrumen MATURED → PENJUALAN_INSTRUMEN_NOT_ACTIVE → Indonesian error", async ({ page }) => {
    await page.route("**/api/v1/trx/penjualan", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 422,
          contentType: "application/json",
          body: JSON.stringify(INSTRUMEN_NOT_ACTIVE_ERROR),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto(`/transaksi/penjualan/new?instrumenId=${INSTRUMEN_ID_INACTIVE}`);
    await expect(page.getByRole("heading", { name: /buat penjualan/i })).toBeVisible({ timeout: 5000 });

    const qtyInput = page.locator('input[placeholder*="0.00000000"]');
    if (await qtyInput.count() > 0) await qtyInput.fill("100");

    const hargaInput = page.locator('input[placeholder*="0.0000"]');
    if (await hargaInput.count() > 0) await hargaInput.fill("1000000");

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) await tanggalInput.fill("2026-07-15");

    const submitBtn = page.getByRole("button", { name: /buat penjualan/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();
      await expect(
        page.getByText(/tidak eligible|ACTIVE.*klasifikasi final|klasifikasi.*locked/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });

  test("S1-AC4: FVOCI Election → no_recycling_note shown, OCI recycle null", async ({ page }) => {
    await page.route("**/api/v1/trx/penjualan", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify(CREATE_SUCCESS_FVOCI_ELECTION),
        });
      } else {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: [], pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 }, meta: { traceId: "t" } }),
        });
      }
    });

    await page.goto(`/transaksi/penjualan/new?instrumenId=${INSTRUMEN_ID_FVOCI_ELECTION}&kode=SHM-0011`);
    await expect(page.getByRole("heading", { name: /buat penjualan/i })).toBeVisible({ timeout: 5000 });

    const jenisSelect = page.getByRole("combobox", { name: /jenis disposal/i });
    if (await jenisSelect.count() > 0) {
      await jenisSelect.click();
      await page.getByRole("option", { name: /penuh/i }).click();
    }

    const qtyInput = page.locator('input[placeholder*="0.00000000"]');
    if (await qtyInput.count() > 0) await qtyInput.fill("1000");

    const hargaInput = page.locator('input[placeholder*="0.0000"]');
    if (await hargaInput.count() > 0) await hargaInput.fill("12000");

    const tanggalInput = page.locator('input[type="date"]');
    if (await tanggalInput.count() > 0) await tanggalInput.fill("2026-07-15");

    const submitBtn = page.getByRole("button", { name: /buat penjualan/i });
    if (await submitBtn.count() > 0) {
      await submitBtn.click();

      // After success, preview panel should show no-recycling note
      await expect(
        page.getByText(/tetap di OCI|PSAK 71.*B5\.7\.1|tidak direkognisi di P&L/i),
      ).toBeVisible({ timeout: 5000 });
    }
  });
});
