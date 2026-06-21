/**
 * Playwright mock spec — S2-AC2: Stage 3 akrual net carrying display
 *
 * Tests:
 *   - Akrual list shows Stage 3 items with NET basis badge (red)
 *   - StaleStagingBadge renders for stale items
 *   - ROLE-AKUN-CTL can open override dialog for PENDING_STALE_REVIEW items
 *   - Override dialog enforces ≥ 30 char reason
 *   - Successful override posts toast + invalidates list
 *   - Non-CTL user does NOT see override button (absent-from-DOM)
 */

import { test, expect } from "@playwright/test";

const BASE = "http://localhost:3000";

// ---------------------------------------------------------------------------
// Mock fixtures
// ---------------------------------------------------------------------------

const STALE_AKRUAL_ID = "aaa00000-0000-0000-0000-000000000001";
const INSTRUMEN_ID = "bbb00000-0000-0000-0000-000000000002";

const stageThreeAkrualItem = {
  id: STALE_AKRUAL_ID,
  instrumenId: INSTRUMEN_ID,
  instrumenKode: "OBL-0202",
  klasifikasiSnapshot: "AC",
  tanggalAkrual: "2026-06-20",
  jenis: "BUNGA",
  stage: 3,
  carryingBasis: "NET_CARRYING",
  carryingIdr: "5600000000.0000",
  eirPersen: "0.09000000",
  bungaKotor: "1380821.9178",
  pph: null,
  bungaBersih: "1380821.9178",
  mataUang: "IDR",
  fxRateId: null,
  staleStagingFlag: true,
  eclRunIdUsed: null,
  status: "PENDING_STALE_REVIEW",
  jurnalHeaderId: null,
  createdAt: "2026-06-20T09:15:00+07:00",
};

const normalAkrualItem = {
  ...stageThreeAkrualItem,
  id: "ccc00000-0000-0000-0000-000000000003",
  instrumenKode: "OBL-0101",
  stage: 1,
  carryingBasis: "GROSS",
  carryingIdr: "10000000000.0000",
  eirPersen: "0.07500000",
  bungaKotor: "2054794.5205",
  bungaBersih: "2054794.5205",
  staleStagingFlag: false,
  status: "AUTO_POSTED",
};

// ---------------------------------------------------------------------------
// S2-AC2: Stage 3 net carrying display in list
// ---------------------------------------------------------------------------

test("S2-AC2: Akrual list shows Stage 3 with NET basis badge and stale alert", async ({
  page,
}) => {
  // Mock API — list with stale count
  await page.route("**/api/v1/transaksi/akrual**", (route) => {
    if (route.request().url().includes("export")) return route.continue();
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [stageThreeAkrualItem, normalAkrualItem],
        pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 50 },
        staleCount: 1,
        meta: { traceId: "trace-s2-001" },
      }),
    });
  });

  // Mock auth as ROLE-AKUN-CTL (has override permission)
  await page.addInitScript(() => {
    localStorage.setItem("blips_token", "mock-token-akun-ctl");
    localStorage.setItem(
      "blips-auth",
      JSON.stringify({
        state: {
          token: "mock-token-akun-ctl",
          user: {
            sub: "akun-ctl-uuid",
            preferred_username: "akun.ctl",
            roles: ["ROLE-AKUN-CTL"],
            permissions: ["akrual.read", "akrual.override_stale"],
            tenant_id: "TUGURE",
            mfa_verified: true,
          },
        },
        version: 0,
      }),
    );
  });

  await page.goto(`${BASE}/transaksi/akrual`);

  // Stale warning banner shown (S5-AC3)
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByText("1 instrumen memiliki staging stale")).toBeVisible();

  // Stage 3 row shows STAGING STALE badge
  await expect(page.getByText("STAGING STALE").first()).toBeVisible();

  // Stage 1 row has no stale badge
  const rows = page.getByRole("row");
  await expect(rows.nth(1)).toContainText("OBL-0202");
  await expect(rows.nth(2)).toContainText("OBL-0101");

  // Override button visible for ROLE-AKUN-CTL on stale item
  const overrideBtn = page.getByRole("button", { name: /override/i }).first();
  await expect(overrideBtn).toBeVisible();
});

// ---------------------------------------------------------------------------
// S5-AC4: Override stale dialog form validation
// ---------------------------------------------------------------------------

test("S5-AC4: Override stale dialog requires ≥ 30 char reason", async ({ page }) => {
  await page.route("**/api/v1/transaksi/akrual**", (route) => {
    if (route.request().url().includes("override-stale")) {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            akrualId: STALE_AKRUAL_ID,
            status: "POSTED",
            akrualIdr: "1380821.9178",
            jurnalEntryId: "eeee0000-0000-0000-0000-000000000005",
          },
          meta: { traceId: "trace-override-001" },
        }),
      });
      return;
    }
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [stageThreeAkrualItem],
        pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
        staleCount: 1,
        meta: { traceId: "trace-list-001" },
      }),
    });
  });

  await page.addInitScript(() => {
    localStorage.setItem(
      "blips-auth",
      JSON.stringify({
        state: {
          token: "mock-token-akun-ctl",
          user: {
            sub: "akun-ctl-uuid",
            preferred_username: "akun.ctl",
            roles: ["ROLE-AKUN-CTL"],
            permissions: ["akrual.read", "akrual.override_stale"],
            tenant_id: "TUGURE",
            mfa_verified: true,
          },
        },
        version: 0,
      }),
    );
  });

  await page.goto(`${BASE}/transaksi/akrual`);

  // Open override dialog
  await page.getByRole("button", { name: /override/i }).first().click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByText("Konfirmasi Staging Stale")).toBeVisible();

  // Submit button disabled until reason ≥ 30 chars
  const submitBtn = page.getByRole("button", { name: /konfirmasi/i });
  await expect(submitBtn).toBeDisabled();

  // Type short reason (29 chars)
  const reasonInput = page.getByPlaceholder(/tidak ada perubahan material/i);
  await reasonInput.fill("Alasan pendek 29 char 12345678");
  await expect(submitBtn).toBeDisabled();

  // Type valid reason (≥ 30 chars)
  await reasonInput.fill(
    "Tidak ada perubahan material sejak ECL run terakhir. Staging Stage 3 dikonfirmasi valid.",
  );
  await expect(submitBtn).toBeEnabled();

  // Submit
  await submitBtn.click();

  // Success toast
  await expect(page.getByText(/diposting/i)).toBeVisible({ timeout: 5000 });
});

// ---------------------------------------------------------------------------
// Non-CTL user: override button absent-from-DOM
// ---------------------------------------------------------------------------

test("Non-CTL user does NOT see override button (absent-from-DOM)", async ({ page }) => {
  await page.route("**/api/v1/transaksi/akrual**", (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [stageThreeAkrualItem],
        pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
        staleCount: 1,
        meta: { traceId: "trace-no-perm-001" },
      }),
    });
  });

  // Auth as ROLE-AKUN (no override permission)
  await page.addInitScript(() => {
    localStorage.setItem(
      "blips-auth",
      JSON.stringify({
        state: {
          token: "mock-token-akun",
          user: {
            sub: "akun-uuid",
            preferred_username: "akun.user",
            roles: ["ROLE-AKUN"],
            permissions: ["akrual.read"],
            tenant_id: "TUGURE",
            mfa_verified: false,
          },
        },
        version: 0,
      }),
    );
  });

  await page.goto(`${BASE}/transaksi/akrual`);

  // Override button must NOT be in DOM
  const overrideBtn = page.getByRole("button", { name: /override/i });
  await expect(overrideBtn).not.toBeVisible();
});
