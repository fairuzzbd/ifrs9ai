/**
 * Playwright E2E — P5-M5 S5: FX Treatment Routing Display
 *
 * AC covered:
 *   S5-AC1: AC + FCY instrument → treatment badge shows "P&L" routing
 *   S5-AC2: FVOCI_DEBT + FCY instrument → treatment badge shows "OCI (dengan recycling)"
 *   S5-AC3: FVOCI_ELECTION + FCY instrument → treatment badge shows "OCI (tanpa recycling)"
 *   S5-AC4: IDR instrument (any klasifikasi) → treatment badge shows "Tidak ada FX"
 *          + error state: klasifikasi not locked → KLASIFIKASI_NOT_LOCKED message shown
 *
 * All API calls are mocked — no live backend required.
 */

import { test, expect } from "@playwright/test";

const KURS_TREATMENT_URL_BASE = "/master/kurs/treatment";

// ---------------------------------------------------------------------------
// Mock response factories
// ---------------------------------------------------------------------------

function makeTreatmentResponse(overrides: Record<string, unknown>) {
  return {
    data: {
      instrumenId: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
      kodeInstrumen: "INST-001234",
      klasifikasiPsak71: "AC",
      matauang: "USD",
      klasifikasiLocked: true,
      klasifikasiLockedAt: "2026-05-01T09:00:00+07:00",
      fxTreatment: {
        routing: "P&L_FOREIGN_EXCHANGE",
        accountType: "P&L",
        ociRecycling: null,
        jurnalEventCode: "FX_PL_MTM",
        psak71Reference: "PSAK 71 — FX ke P&L",
        notes: null,
      },
      ...overrides,
    },
    meta: { traceId: "trace-s5-001" },
  };
}

const INSTRUMEN_AC_FCY = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";
const INSTRUMEN_FVOCI_DEBT = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";
const INSTRUMEN_FVOCI_ELECTION = "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";
const INSTRUMEN_IDR = "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";
const INSTRUMEN_UNLOCKED = "e0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11";

// We assume a treatment detail page exists at /master/kurs/treatment/{instrumenId}
// If the actual route is a dialog/modal on the kurs list, adjust accordingly.

test.describe("S5 — FX Treatment Routing Display", () => {
  // S5-AC1: AC + FCY → P&L
  test("S5-AC1: AC + FCY instrument → FxTreatmentBadge shows P&L routing", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_AC_FCY}`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            makeTreatmentResponse({
              instrumenId: INSTRUMEN_AC_FCY,
              klasifikasiPsak71: "AC",
              matauang: "USD",
              fxTreatment: {
                routing: "P&L_FOREIGN_EXCHANGE",
                accountType: "P&L",
                ociRecycling: null,
                jurnalEventCode: "FX_PL_MTM",
                psak71Reference: "PSAK 71 — FX ke P&L",
                notes: null,
              },
            }),
          ),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_AC_FCY}`);
    await page.waitForLoadState("domcontentloaded");

    // Treatment badge should show P&L label
    await expect(page.getByText(/P&L|P\&L/i)).toBeVisible({ timeout: 5000 });
    // Routing indicator
    await expect(page.getByText(/P&L_FOREIGN_EXCHANGE|P&L|P\&L/i)).toBeVisible();
  });

  // S5-AC2: FVOCI_DEBT + FCY → OCI with recycling
  test("S5-AC2: FVOCI_DEBT + FCY instrument → FxTreatmentBadge shows OCI dengan recycling", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_FVOCI_DEBT}`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            makeTreatmentResponse({
              instrumenId: INSTRUMEN_FVOCI_DEBT,
              klasifikasiPsak71: "FVOCI_DEBT",
              matauang: "USD",
              fxTreatment: {
                routing: "OCI_FOREIGN_EXCHANGE_RESERVE",
                accountType: "OCI",
                ociRecycling: true,
                jurnalEventCode: "FX_OCI_DEBT",
                psak71Reference: "PSAK 71 §5.7.10 — FX ke OCI, di-recycle ke P&L saat derecognition",
                notes: null,
              },
            }),
          ),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_FVOCI_DEBT}`);
    await page.waitForLoadState("domcontentloaded");

    // OCI with recycling badge/text should be visible
    await expect(
      page.getByText(/OCI.*recycling|OCI \(dengan recycling\)|OCI_FOREIGN_EXCHANGE_RESERVE[^_]/i),
    ).toBeVisible({ timeout: 5000 });
  });

  // S5-AC3: FVOCI_ELECTION + FCY → OCI no recycling
  test("S5-AC3: FVOCI_ELECTION + FCY instrument → FxTreatmentBadge shows OCI tanpa recycling", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_FVOCI_ELECTION}`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            makeTreatmentResponse({
              instrumenId: INSTRUMEN_FVOCI_ELECTION,
              klasifikasiPsak71: "FVOCI_ELECTION",
              matauang: "USD",
              fxTreatment: {
                routing: "OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING",
                accountType: "OCI",
                ociRecycling: false,
                jurnalEventCode: "FX_OCI_EQUITY",
                psak71Reference: "PSAK 71 §5.7.5 — FX ke OCI, irrevocable, tidak di-recycle",
                notes: "Irrevocable FVOCI election — no P&L recycling on disposal",
              },
            }),
          ),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_FVOCI_ELECTION}`);
    await page.waitForLoadState("domcontentloaded");

    // OCI no recycling — should NOT show "dengan recycling"
    await expect(
      page.getByText(/OCI.*tanpa recycling|OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING/i),
    ).toBeVisible({ timeout: 5000 });

    // PSAK 71 §5.7.5 reference visible (compliance-critical)
    await expect(page.getByText(/5\.7\.5/i)).toBeVisible({ timeout: 5000 });
  });

  // S5-AC4a: IDR instrument → NO_FX_TREATMENT
  test("S5-AC4: IDR instrument → FxTreatmentBadge shows Tidak ada FX", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_IDR}`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            makeTreatmentResponse({
              instrumenId: INSTRUMEN_IDR,
              klasifikasiPsak71: "AC",
              matauang: "IDR",
              fxTreatment: {
                routing: "NO_FX_TREATMENT",
                accountType: null,
                ociRecycling: null,
                jurnalEventCode: null,
                psak71Reference: null,
                notes: "IDR functional currency — no FX exposure",
              },
            }),
          ),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_IDR}`);
    await page.waitForLoadState("domcontentloaded");

    // "Tidak ada FX" or NO_FX label
    await expect(
      page.getByText(/Tidak ada FX|NO_FX_TREATMENT|Tidak ada FX Treatment/i),
    ).toBeVisible({ timeout: 5000 });
  });

  // S5-AC4b: Klasifikasi not locked → KLASIFIKASI_NOT_LOCKED error shown
  test("S5-AC4: klasifikasi not locked → KLASIFIKASI_NOT_LOCKED error message displayed", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_UNLOCKED}`,
      (route) => {
        route.fulfill({
          status: 422,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "KLASIFIKASI_NOT_LOCKED",
              message:
                "Instrumen belum memiliki klasifikasi PSAK 71 yang final (locked). FX treatment tidak dapat ditentukan.",
              details: [],
              traceId: "trace-s5-klasifikasi-err",
            },
          }),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_UNLOCKED}`);
    await page.waitForLoadState("domcontentloaded");

    // Error message about klasifikasi not locked
    await expect(
      page.getByText(/klasifikasi.*locked|KLASIFIKASI_NOT_LOCKED|SPPI|belum.*final/i),
    ).toBeVisible({ timeout: 5000 });
  });

  // FxTreatmentBadge tooltip — PSAK 71 reference should be accessible
  test("S5-AC2/3: FxTreatmentBadge tooltip shows PSAK 71 reference on hover", async ({ page }) => {
    await page.route(
      `**/api/v1/master/kurs/treatment/${INSTRUMEN_FVOCI_DEBT}`,
      (route) => {
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            makeTreatmentResponse({
              instrumenId: INSTRUMEN_FVOCI_DEBT,
              klasifikasiPsak71: "FVOCI_DEBT",
              matauang: "EUR",
              fxTreatment: {
                routing: "OCI_FOREIGN_EXCHANGE_RESERVE",
                accountType: "OCI",
                ociRecycling: true,
                jurnalEventCode: "FX_OCI_DEBT",
                psak71Reference: "PSAK 71 §5.7.10",
                notes: null,
              },
            }),
          ),
        });
      },
    );

    await page.goto(`${KURS_TREATMENT_URL_BASE}/${INSTRUMEN_FVOCI_DEBT}`);
    await page.waitForLoadState("domcontentloaded");

    // Hover over treatment badge to reveal tooltip
    const badge = page.getByText(/OCI.*recycling|OCI/i).first();
    await badge.hover();

    // Tooltip should show PSAK 71 reference (via Radix Tooltip)
    await expect(page.getByText(/PSAK 71|§5\.7/i)).toBeVisible({ timeout: 3000 });
  });
});
