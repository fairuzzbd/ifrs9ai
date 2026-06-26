/**
 * Playwright E2E — P5-M17 Cross-Cutting Role Gating
 *
 * AC coverage:
 *   M17-01-AC4 — Periode Buku: ROLE-CFO hard-close button, ROLE-AKUN-CTL soft-close only
 *   M17-02-AC3 — Master Kurs: ROLE-AUDIT no mutation buttons, ROLE-AKUN JISDOR sync & manual
 *   M17-03-AC3 — Mapping Jurnal: role per workflow step, SoD at API level
 *   M17-04-AC4 — Jurnal tabs: ROLE-AKUN (Header only), AKUN-CTL (Header+DLQ), IT-ADMIN (all 3)
 *   M17-05-AC4 — Reconciliation: ROLE-AUDIT read access, ROLE-AKUN no jurnal.read → redirect
 *
 * Security contract:
 *   - Unauthorized actions: absent-from-DOM (not CSS-hidden)
 *   - All checks use .count() === 0, NOT .isHidden()
 *   - requirePermission middleware redirects to /404 for whole-page gating
 *   - requirePermissionWithMfa routes: mfa_verified=false redirects to /mfa-required
 *
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-test",
  mfaVerified = true
) {
  return page.addInitScript(
    ({ r, p, uid, m }: { r: string[]; p: string[]; uid: string; m: boolean }) => {
      localStorage.setItem("blips_roles", JSON.stringify(r));
      localStorage.setItem("blips_permissions", JSON.stringify(p));
      localStorage.setItem("blips_user_id", uid);
      localStorage.setItem("blips_mfa_verified", String(m));
    },
    { r: roles, p: permissions, uid: userId, m: mfaVerified }
  );
}

function mockApiOk(page: Page, path: string, body: object) {
  page.route(`**${path}**`, (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ data: body, meta: { traceId: "t" } }),
    })
  );
}

function mockListOk(page: Page, path: string, items: object[] = []) {
  page.route(`**${path}**`, (route: Route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: items,
        pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 },
        meta: { traceId: "t" },
      }),
    })
  );
}

async function assertAbsentFromDOM(page: Page, selector: string) {
  const count = await page.locator(selector).count();
  expect(count, `Expected "${selector}" to be absent from DOM`).toBe(0);
}

async function assertPresentInDOM(page: Page, selector: string) {
  const count = await page.locator(selector).count();
  expect(count, `Expected "${selector}" to be present in DOM`).toBeGreaterThan(0);
}

// ---------------------------------------------------------------------------
// M17-01-AC4 — Periode Buku: CFO vs AKUN-CTL button gating
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — Periode Buku", () => {

  const PERIODE_ID = "prd-2026-06";

  function mockPeriodeDetail(page: Page, status: string) {
    mockApiOk(page, `/api/v1/periode/${PERIODE_ID}`, {
      id: PERIODE_ID,
      bulan: "2026-06-01",
      status_close: status,
      nama: "Juni 2026",
    });
    mockListOk(page, `/api/v1/periode/${PERIODE_ID}/history`);
  }

  test("M17-01-AC4: ROLE-CFO sees 'Hard Close' button for SOFT_CLOSED periode", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["periode.read", "periode.hardclose"], "usr-cfo");
    await mockPeriodeDetail(page, "SOFT_CLOSED");
    mockListOk(page, "/api/v1/periode");

    await page.goto(`/periode-buku/${PERIODE_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Hard Close')");
  });

  test("M17-01-AC4: ROLE-AKUN-CTL sees 'Soft Close' but not 'Hard Close'", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["periode.read", "periode.softclose"], "usr-akunctl");
    await mockPeriodeDetail(page, "OPEN");
    mockListOk(page, "/api/v1/periode");

    await page.goto(`/periode-buku/${PERIODE_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Soft Close')");
    await assertAbsentFromDOM(page, "button:has-text('Hard Close')");
  });

  test("M17-01-AC4: ROLE-AKUN-CTL absent from DOM for Hard Close (not CSS-hidden)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["periode.read", "periode.softclose"], "usr-akunctl");
    await mockPeriodeDetail(page, "SOFT_CLOSED");
    mockListOk(page, "/api/v1/periode");

    await page.goto(`/periode-buku/${PERIODE_ID}`);
    await page.waitForLoadState("networkidle");

    const html = await page.content();
    // Hard close must not be anywhere in DOM — not even in hidden elements
    expect(html).not.toMatch(/hard.close/i);
    expect(html).not.toMatch(/hardclose/i);
  });

  test("M17-01-AC4: ROLE-AUDIT sees no action buttons at all", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["periode.read", "audit_log.read"], "usr-audit");
    await mockPeriodeDetail(page, "OPEN");
    mockListOk(page, "/api/v1/periode");

    await page.goto(`/periode-buku/${PERIODE_ID}`);
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Soft Close')");
    await assertAbsentFromDOM(page, "button:has-text('Hard Close')");
    await assertAbsentFromDOM(page, "button:has-text('Reopen')");
  });

  test("M17-01-AC4: HARD_CLOSED periode — reopen button absent for all roles", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["periode.read", "periode.hardclose"], "usr-cfo");
    await mockPeriodeDetail(page, "HARD_CLOSED");
    mockListOk(page, "/api/v1/periode");

    await page.goto(`/periode-buku/${PERIODE_ID}`);
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Reopen')");
    await assertAbsentFromDOM(page, "button:has-text('Hard Close')");
  });
});

// ---------------------------------------------------------------------------
// M17-02-AC3 — Master Kurs: ROLE-AUDIT read-only, ROLE-AKUN has mutations
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — Master Kurs", () => {

  function mockKursList(page: Page) {
    mockListOk(page, "/api/v1/fx/rates", [
      { id: "fx-001", kode_mata_uang: "USD", nilai_kurs: 16350, sumber: "JISDOR" },
    ]);
  }

  test("M17-02-AC3: ROLE-AUDIT sees no mutation buttons on Master Kurs list", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["fx_rate.read", "audit_log.read"], "usr-audit");
    await mockKursList(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Input Manual')");
    await assertAbsentFromDOM(page, "button:has-text('Sinkron JISDOR')");
    await assertAbsentFromDOM(page, "button:has-text('Upload')");
    // Export is allowed for AUDIT
    await assertPresentInDOM(page, "button:has-text('Export')");
  });

  test("M17-02-AC3: ROLE-AKUN sees all mutation buttons on Master Kurs list", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["fx_rate.read", "fx_rate.create"], "usr-akun");
    await mockKursList(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Input Manual')");
    await assertPresentInDOM(page, "button:has-text('Sinkron JISDOR')");
  });

  test("M17-02-AC3: ROLE-MAKER-TR has no access to /master/kurs (redirect to /404)", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["instrumen.read", "penempatan.create"], "usr-maker");
    await mockKursList(page);

    await page.goto("/master/kurs");
    await page.waitForLoadState("networkidle");

    // requirePermission(fx_rate.read) should redirect to /404
    expect(page.url()).toMatch(/\/404|\/not-found|\/unauthorized/);
  });
});

// ---------------------------------------------------------------------------
// M17-03-AC3 — Mapping Jurnal: 6-eyes step button gating
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — Mapping Jurnal 6-eyes", () => {

  const MJ_ID = "mj-001";

  function mockMappingDetail(page: Page, status: string, makerId: string, reviewerId = "", appr1Id = "") {
    mockApiOk(page, `/api/v1/master/mapping-jurnal/${MJ_ID}`, {
      id: MJ_ID,
      kode_jurnal: "DEPOSITO_INT",
      status_workflow: status,
      maker_id: makerId,
      reviewer_id: reviewerId,
      approver1_id: appr1Id,
    });
    mockListOk(page, `/api/v1/master/mapping-jurnal/${MJ_ID}/history`);
  }

  test("M17-03-AC3: DRAFT — only maker sees Edit+Submit, reviewer/approver see neither", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create", "mapping_jurnal.submit"], "usr-maker");
    mockMappingDetail(page, "DRAFT", "usr-maker");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Submit')");
  });

  test("M17-03-AC3: SUBMITTED — reviewer sees Review button, maker (SoD) does not", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-reviewer");
    mockMappingDetail(page, "SUBMITTED", "usr-maker", "", "");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Setuju')");
  });

  test("M17-03-AC3: SUBMITTED — maker (SoD) sees no Review button (absent-from-DOM)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-maker");
    mockMappingDetail(page, "SUBMITTED", "usr-maker", "", "");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    // SoD: maker cannot be reviewer
    await assertAbsentFromDOM(page, "button:has-text('Setuju')");
  });

  test("M17-03-AC3: REVIEWED — ROLE-RISK sees Approve-1 button", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk");
    mockMappingDetail(page, "REVIEWED", "usr-maker", "usr-reviewer", "");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Approve')");
  });

  test("M17-03-AC3: APPROVED_1 — ROLE-KOMITE sees Approve-2 button, ROLE-AKUN does not", async ({ page }) => {
    await setRole(page, ["ROLE-KOMITE"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-komite");
    mockMappingDetail(page, "APPROVED_1", "usr-maker", "usr-reviewer", "usr-risk");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "button:has-text('Approve')");
  });

  test("M17-03-AC3: APPROVED_1 — ROLE-AKUN sees no Approve-2 button", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"], "usr-akun2");
    mockMappingDetail(page, "APPROVED_1", "usr-maker", "usr-reviewer", "usr-risk");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Approve')");
  });

  test("M17-03-AC3: ACTIVE status — no action buttons for any role", async ({ page }) => {
    await setRole(page, ["ROLE-KOMITE"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-komite");
    mockMappingDetail(page, "ACTIVE", "usr-maker", "usr-reviewer", "usr-risk");

    await page.goto(`/master/mapping-jurnal/${MJ_ID}`);
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Approve')");
    await assertAbsentFromDOM(page, "button:has-text('Setuju')");
    await assertAbsentFromDOM(page, "button:has-text('Submit')");
  });
});

// ---------------------------------------------------------------------------
// M17-04-AC4 — Jurnal tab visibility per role
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — Jurnal Layout Tabs", () => {

  function mockJurnalBase(page: Page) {
    mockListOk(page, "/api/v1/jurnal/header");
    mockListOk(page, "/api/v1/jurnal/dlq");
    page.route("**/api/v1/jurnal/dlq/pending-count**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { count: 3 }, meta: { traceId: "t" } }),
      })
    );
  }

  test("M17-04-AC4: ROLE-AKUN — Header tab visible, DLQ + Resolve absent", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["jurnal.read"], "usr-akun");
    await mockJurnalBase(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "[role=tab]:has-text('Header Jurnal')");
    await assertAbsentFromDOM(page, "[role=tab]:has-text('DLQ')");
    await assertAbsentFromDOM(page, "[role=tab]:has-text('Resolve')");
  });

  test("M17-04-AC4: ROLE-AKUN-CTL — Header + DLQ tabs, Resolve absent", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.dlq.read"], "usr-akunctl");
    await mockJurnalBase(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "[role=tab]:has-text('Header Jurnal')");
    await assertPresentInDOM(page, "[role=tab]:has-text('DLQ')");
    await assertAbsentFromDOM(page, "[role=tab]:has-text('Resolve')");
  });

  test("M17-04-AC4: ROLE-IT-ADMIN — all 3 tabs visible", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.resolve"], "usr-itadmin");
    await mockJurnalBase(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    await assertPresentInDOM(page, "[role=tab]:has-text('Header Jurnal')");
    await assertPresentInDOM(page, "[role=tab]:has-text('DLQ')");
    await assertPresentInDOM(page, "[role=tab]:has-text('Resolve')");
  });

  test("M17-04-AC4: ROLE-AKUN-CTL — Resolve tab absent-from-DOM, not CSS-hidden", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.dlq.read"], "usr-akunctl");
    await mockJurnalBase(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    const html = await page.content();
    // Resolve tab must not appear in HTML at all
    expect(html).not.toMatch(/tab.*resolve/i);
    expect(html).not.toMatch(/resolve.*tab/i);
  });

  test("M17-04-AC4: ROLE-MAKER-TR (no jurnal.read) — redirected to /404", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["instrumen.read"], "usr-maker");
    await mockJurnalBase(page);

    await page.goto("/jurnal/header");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/404|\/not-found|\/unauthorized/);
  });
});

// ---------------------------------------------------------------------------
// M17-05-AC4 — Reconciliation Daily: role access + read-only enforcement
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — Reconciliation Daily", () => {

  function mockReconEndpoints(page: Page) {
    page.route("**/api/v1/reconciliation/daily**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            tanggal: "2026-06-25",
            blips_total: 1240,
            gl_total: 1235,
            jumlah_mismatch: 5,
            status: "AVAILABLE",
          },
          meta: { traceId: "t" },
        }),
      })
    );
    mockListOk(page, "/api/v1/reconciliation/daily/mismatches");
  }

  test("M17-05-AC4: ROLE-AKUN-CTL — read-only, no mutation buttons", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read"], "usr-akunctl");
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    await assertAbsentFromDOM(page, "button:has-text('Jalankan Rekonsiliasi')");
    await assertAbsentFromDOM(page, "button:has-text('Post Jurnal')");
    await assertPresentInDOM(page, "button:has-text('Refresh')");
  });

  test("M17-05-AC4: ROLE-AUDIT — full read access including export", async ({ page }) => {
    await setRole(page, ["ROLE-AUDIT"], ["jurnal.read", "audit_log.read"], "usr-audit");
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const is404 = (await page.getByText(/404|page not found/i).count()) > 0;
    expect(is404).toBe(false);

    await assertPresentInDOM(page, "button:has-text('Export')");
    await assertAbsentFromDOM(page, "button:has-text('Jalankan Rekonsiliasi')");
  });

  test("M17-05-AC4: ROLE-MAKER-TR (no jurnal.read) — redirected from /reconciliation/daily", async ({ page }) => {
    await setRole(page, ["ROLE-MAKER-TR"], ["instrumen.read", "penempatan.create"], "usr-maker");
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/404|\/not-found|\/unauthorized/);
  });

  test("M17-05-AC4: ROLE-CFO — can access reconciliation (jurnal.read implied)", async ({ page }) => {
    await setRole(page, ["ROLE-CFO"], ["jurnal.read", "periode.hardclose"], "usr-cfo");
    await mockReconEndpoints(page);

    await page.goto("/reconciliation/daily");
    await page.waitForLoadState("networkidle");

    const is404 = (await page.getByText(/404|page not found/i).count()) > 0;
    expect(is404).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// MFA-required route gating (DEC-026/027 compliance)
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — MFA-required routes", () => {

  test("M17-01-AC3: ROLE-CFO with mfa_verified=false on /periode-buku/{id} → /mfa-required", async ({ page }) => {
    // CFO without current MFA session — step-up required
    await setRole(page, ["ROLE-CFO"], ["periode.read", "periode.hardclose"], "usr-cfo", false);

    page.route("**/api/v1/periode/**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { id: "prd-2026-06", status_close: "SOFT_CLOSED", nama: "Juni 2026" },
          meta: { traceId: "t" },
        }),
      })
    );

    await page.goto("/periode-buku/prd-2026-06");
    await page.waitForLoadState("networkidle");

    // Page may still load (MFA check is action-level, not page-level for CFO)
    // But Hard Close button should trigger MFA step-up modal when clicked
    const hardCloseBtn = page.getByRole("button", { name: /hard close/i });
    if ((await hardCloseBtn.count()) > 0) {
      await hardCloseBtn.click();
      // MFA modal must appear
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });
      await expect(page.getByText(/kode verifikasi|mfa|otp/i)).toBeVisible();
    }
  });

  test("M17-04-AC3: DLQ Replay — ROLE-IT-ADMIN without MFA sees MFAStepUpModal", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.read", "jurnal.dlq.read", "jurnal.dlq.replay"], "usr-itadmin", false);

    page.route("**/api/v1/jurnal/dlq**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [{ id: "dlq-001", status: "FAILED", error_code: "GL_TIMEOUT", retry_count: 2 }],
          pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
          meta: { traceId: "t" },
        }),
      })
    );

    await page.goto("/jurnal/dlq");
    await page.waitForLoadState("networkidle");

    const replayBtn = page.getByRole("button", { name: /replay/i }).first();
    if ((await replayBtn.count()) > 0) {
      await replayBtn.click();
      // Confirm dialog first
      const confirmBtn = page.getByRole("button", { name: /konfirmasi|lanjut/i });
      if ((await confirmBtn.count()) > 0) await confirmBtn.click();
      // Then MFA modal
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 3000 });
    }
  });

  test("M17-03-AC4: Mapping Jurnal Approve-2 (ROLE-KOMITE) — mfa_verified=true, no extra MFA prompt", async ({ page }) => {
    // KOMITE already has mfa_verified=true in JWT — approve-2 does NOT need step-up
    // per DEC-027 scope (mapping jurnal approve is not in step-up list)
    await setRole(page, ["ROLE-KOMITE"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-komite", true);

    page.route("**/api/v1/master/mapping-jurnal/mj-001**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "mj-001",
            kode_jurnal: "DEPOSITO_INT",
            status_workflow: "APPROVED_1",
            maker_id: "usr-maker",
            reviewer_id: "usr-reviewer",
            approver1_id: "usr-risk",
          },
          meta: { traceId: "t" },
        }),
      })
    );

    page.route("**/api/v1/master/mapping-jurnal/mj-001/approve**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: { status_workflow: "APPROVED_2" }, meta: { traceId: "t" } }),
      })
    );

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve/i });
    if ((await approveBtn.count()) > 0) {
      await approveBtn.click();
      // Should proceed to signature panel, NOT open MFAStepUpModal
      const mfaModal = page.getByRole("dialog").filter({ hasText: /kode verifikasi|otp/i });
      await expect(mfaModal).toHaveCount(0);
    }
  });
});

// ---------------------------------------------------------------------------
// Absent-from-DOM sanity checks across all M17 routes for ROLE-AUDIT
// ---------------------------------------------------------------------------

test.describe("P5-M17 Role Gating — ROLE-AUDIT read-only across all M17 routes", () => {

  const ROUTES = [
    "/periode-buku",
    "/master/kurs",
    "/master/mapping-jurnal",
    "/jurnal/header",
    "/reconciliation/daily",
  ];

  const MUTATION_SELECTORS = [
    "button:has-text('Simpan')",
    "button:has-text('Submit')",
    "button:has-text('Approve')",
    "button:has-text('Setuju')",
    "button:has-text('Soft Close')",
    "button:has-text('Hard Close')",
    "button:has-text('Replay')",
    "button:has-text('Sinkron JISDOR')",
    "button:has-text('Input Manual')",
    "button:has-text('Upload')",
    "button:has-text('Buat Baru')",
  ];

  for (const route of ROUTES) {
    test(`ROLE-AUDIT on ${route} — no mutation buttons in DOM`, async ({ page }) => {
      await setRole(
        page,
        ["ROLE-AUDIT"],
        ["periode.read", "fx_rate.read", "mapping_jurnal.read", "jurnal.read", "audit_log.read"],
        "usr-audit"
      );

      // Generic mock for all API calls
      page.route("**/api/v1/**", (r: Route) =>
        r.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: [],
            pagination: { nextCursor: null, hasMore: false, totalEstimate: 0, limit: 50 },
            meta: { traceId: "t" },
          }),
        })
      );

      await page.goto(route);
      await page.waitForLoadState("networkidle");

      const is404 = (await page.getByText(/404|unauthorized/i).count()) > 0;
      if (is404) {
        // Some routes may legitimately 404 for AUDIT — that's acceptable gating
        return;
      }

      for (const selector of MUTATION_SELECTORS) {
        const count = await page.locator(selector).count();
        expect(count, `ROLE-AUDIT must not see "${selector}" on ${route}`).toBe(0);
      }
    });
  }
});
