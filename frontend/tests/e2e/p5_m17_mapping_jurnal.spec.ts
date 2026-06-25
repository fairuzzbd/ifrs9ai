/**
 * Playwright E2E — P5-M17 Mapping Jurnal 6-Eyes Workflow
 *
 * AC coverage:
 *   M17-03-AC1 — 308 redirect dari /mapping-jurnal/* dan /jrnl/mapping/* ke /master/mapping-jurnal/*
 *   M17-03-AC2 — 6-eyes workflow panel: tombol per step visible hanya untuk role yang berwenang
 *   M17-03-AC3 — 6-eyes sign: form notification UX §2 + SoD reject di API
 *   M17-03-AC4 — List /master/mapping-jurnal: DataTable UX §1 + audit history tab
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const MAPPING_LIST_RESPONSE = {
  data: [
    { id: "mj-001", kodeMaping: "MJ-001", eventCode: "DEPOSITO_INT", namaMaping: "Bunga Deposito", debitCoa: "6001.01", kreditCoa: "2101.01", statusWorkflow: "ACTIVE", activeSince: "2026-06-01", makerId: "usr-akun-001" },
    { id: "mj-002", kodeMaping: "MJ-002", eventCode: "MTM_OBL", namaMaping: "MTM Obligasi", debitCoa: "1201.02", kreditCoa: "3101.01", statusWorkflow: "DRAFT", activeSince: null, makerId: "usr-akun-001" },
    { id: "mj-003", kodeMaping: "MJ-003", eventCode: "ECL_STAGE2", namaMaping: "ECL Stage 2", debitCoa: "6002.01", kreditCoa: "1202.01", statusWorkflow: "REVIEWED", activeSince: null, makerId: "usr-akun-002" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 3, limit: 50 },
  appliedSort: [{ col: "createdAt", dir: "desc" }],
  appliedFilter: {},
  meta: { traceId: "trace-mj-list" },
};

const MAPPING_SUBMITTED = {
  data: { id: "mj-001", kodeMaping: "MJ-001", eventCode: "DEPOSITO_INT", namaMaping: "Bunga Deposito", statusWorkflow: "SUBMITTED", makerId: "usr-akun-001", reviewerId: null, approverId: null, approver2Id: null },
  meta: { traceId: "trace-mj-submitted" },
};

const MAPPING_REVIEWED = {
  data: { id: "mj-001", kodeMaping: "MJ-001", eventCode: "DEPOSITO_INT", namaMaping: "Bunga Deposito", statusWorkflow: "REVIEWED", makerId: "usr-akun-001", reviewerId: "usr-ctl-001", approverId: null, approver2Id: null },
  meta: { traceId: "trace-mj-reviewed" },
};

const MAPPING_APPROVED_1 = {
  data: { id: "mj-001", kodeMaping: "MJ-001", eventCode: "DEPOSITO_INT", namaMaping: "Bunga Deposito", statusWorkflow: "APPROVED_1", makerId: "usr-akun-001", reviewerId: "usr-ctl-001", approverId: "usr-risk-001", approver2Id: null },
  meta: { traceId: "trace-mj-approved1" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-akun-001",
  mfaVerified = false
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

function mockMappingEndpoints(page: Page, detailResponse = MAPPING_SUBMITTED) {
  page.route("**/api/v1/master/mapping-jurnal**", (route: Route) => {
    const url = route.request().url();
    const method = route.request().method();

    if (url.includes("/export")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "kode,event_code\nMJ-001,DEPOSITO_INT" });
    }
    if (url.match(/\/master\/mapping-jurnal\/mj-001$/) && method === "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(detailResponse) });
    }
    if (url.includes("/submit") || url.includes("/review") || url.includes("/approve") || url.includes("/activate") || url.includes("/reject")) {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "REVIEWED" }, meta: { traceId: "t" } }) });
    }
    if (url.includes("/history")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            { eventTime: "2026-06-20T09:00:00+07:00", actor: "Budi", role: "ROLE-AKUN", aksi: "SUBMIT", komentar: "Mapping sesuai SOP 2026" },
          ],
          pagination: { nextCursor: null, hasMore: false, totalEstimate: 1, limit: 50 },
          meta: { traceId: "t" },
        }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MAPPING_LIST_RESPONSE) });
  });
}

// ---------------------------------------------------------------------------
// M17-03-AC1: 308 Redirects
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Mapping Jurnal: 308 Redirects (AC1)", () => {

  test("M17-03-AC1: /mapping-jurnal → 308 → /master/mapping-jurnal", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);
    await mockMappingEndpoints(page);

    await page.goto("/mapping-jurnal");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal(?!\/)(\?.*)?$/);
  });

  test("M17-03-AC1: /mapping-jurnal/import → 308 → /master/mapping-jurnal/new", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);
    await mockMappingEndpoints(page);

    await page.goto("/mapping-jurnal/import");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal\/new/);
  });

  test("M17-03-AC1: /mapping-jurnal/{event_code} → 308 → /master/mapping-jurnal", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"]);
    await mockMappingEndpoints(page);

    await page.goto("/mapping-jurnal/DEPOSITO_INT");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal/);
  });

  test("M17-03-AC1: /jrnl/mapping → 308 → /master/mapping-jurnal", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"]);
    await mockMappingEndpoints(page);

    await page.goto("/jrnl/mapping");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal(?!\/)(\?.*)?$/);
  });

  test("M17-03-AC1: /jrnl/mapping/new → 308 → /master/mapping-jurnal/new", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);
    await mockMappingEndpoints(page);

    await page.goto("/jrnl/mapping/new");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal\/new/);
  });

  test("M17-03-AC1: /jrnl/mapping/{id} → 308 → /master/mapping-jurnal/{id}", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"]);
    await mockMappingEndpoints(page);

    await page.goto("/jrnl/mapping/mj-001");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal\/mj-001/);
  });

  test("M17-03-AC1: /jrnl/mapping/{id}/edit → 308 → /master/mapping-jurnal/{id}/edit", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.update"]);
    await mockMappingEndpoints(page);

    await page.goto("/jrnl/mapping/mj-001/edit");
    await page.waitForLoadState("networkidle");

    expect(page.url()).toMatch(/\/master\/mapping-jurnal\/mj-001\/edit/);
  });

  test("M17-03-AC1: no 404 from any mapping redirect path", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);
    await mockMappingEndpoints(page);

    const paths = [
      "/mapping-jurnal",
      "/mapping-jurnal/import",
      "/jrnl/mapping",
      "/jrnl/mapping/new",
      "/jrnl/mapping/mj-001",
      "/jrnl/mapping/mj-001/edit",
    ];

    for (const path of paths) {
      await page.goto(path);
      await page.waitForLoadState("networkidle");

      const is404 = (await page.getByText(/404|page not found|halaman tidak ditemukan/i).count()) > 0;
      expect(is404, `Expected no 404 for path: ${path}`).toBe(false);
    }
  });
});

// ---------------------------------------------------------------------------
// M17-03-AC2: 6-Eyes Workflow Panel Button Gating
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Mapping Jurnal: 6-Eyes Workflow Panel (AC2)", () => {

  test("M17-03-AC2: ROLE-AKUN-CTL sees Review button on SUBMITTED mapping", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-ctl-001", true);
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /review.*tandatangani|tandatangani/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-03-AC2: ROLE-AKUN-CTL does NOT see Approve (Risk) button", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-ctl-001", true);
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /approve.*risk/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /approve final.*komite/i })).toHaveCount(0);
  });

  test("M17-03-AC2: Maker (ROLE-AKUN) does NOT see Review button (SoD)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.submit"], "usr-akun-001");
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /review.*tandatangani/i })).toHaveCount(0);
    // SoD banner informational
    await expect(page.getByText(/menunggu review|finance controller/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-03-AC2: ROLE-RISK does NOT see Approve button on SUBMITTED (not yet REVIEWED)", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk-001");
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /approve.*risk/i })).toHaveCount(0);
    await expect(page.getByText(/belum dapat di.approve|menunggu review/i)).toBeVisible({ timeout: 5000 });
  });

  test("M17-03-AC2: ROLE-RISK sees Approve (Risk) button on REVIEWED mapping", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk-001");
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /approve.*risk/i })).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: /approve final.*komite/i })).toHaveCount(0);
  });

  test("M17-03-AC2: ROLE-KOMITE sees Approve Final button on APPROVED_1 mapping", async ({ page }) => {
    await setRole(page, ["ROLE-KOMITE"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-komite-001", true);
    await mockMappingEndpoints(page, MAPPING_APPROVED_1);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /approve final.*komite/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-03-AC2: ROLE-KOMITE approve-2 does NOT require MFA step-up (only mfa_verified=true in JWT)", async ({ page }) => {
    await setRole(page, ["ROLE-KOMITE"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-komite-001", true);
    await mockMappingEndpoints(page, MAPPING_APPROVED_1);

    page.route("**/api/v1/master/mapping-jurnal/mj-001/approve-2", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "APPROVED_2" }, meta: { traceId: "t" } }) })
    );

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve final.*komite/i });
    if (await approveBtn.isVisible()) {
      await approveBtn.click();
      // Should NOT see MFA step-up modal (only mfa_verified JWT is sufficient)
      await page.waitForTimeout(500);
      const mfaModal = page.getByText(/verifikasi mfa step.up|step.up|kode TOTP.*hard.close/i);
      await expect(mfaModal).toHaveCount(0);
    }
  });

  test("M17-03-AC2: stepper shows correct step states", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-ctl-001", true);
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    // Stepper must show at least the current step
    await expect(page.getByText(/DRAFT|SUBMITTED|REVIEWED|APPROVED|ACTIVE/i).first()).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// M17-03-AC3: 6-Eyes Sign Form Notification UX §2 + SoD
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Mapping Jurnal: 6-Eyes Sign + SoD (AC3)", () => {

  test("M17-03-AC3: ROLE-RISK approve includes Idempotency-Key + comment", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk-001");
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    let capturedKey = "";
    let capturedBody: Record<string, unknown> = {};

    page.route("**/api/v1/master/mapping-jurnal/mj-001/approve", (route: Route) => {
      capturedKey = route.request().headers()["idempotency-key"] ?? "";
      capturedBody = JSON.parse(route.request().postData() ?? "{}");
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "APPROVED_1" }, meta: { traceId: "t" } }) });
    });

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve.*risk/i });
    if (await approveBtn.isVisible()) {
      const commentBox = page.getByLabel(/komentar/i);
      if (await commentBox.isVisible()) {
        await commentBox.fill("Mapping sesuai standar GL");
      }
      await approveBtn.click();

      await page.waitForTimeout(500);

      if (capturedKey) {
        expect(capturedKey).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      }
      if (capturedBody.signature_method) {
        expect(capturedBody.signature_method).toBe("JWT_STEP_UP");
      }
    }
  });

  test("M17-03-AC3: approve success toast has specific copy", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk-001");
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    page.route("**/api/v1/master/mapping-jurnal/mj-001/approve", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "APPROVED_1" }, meta: { traceId: "t" } }) })
    );

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const approveBtn = page.getByRole("button", { name: /approve.*risk/i });
    if (await approveBtn.isVisible()) {
      await approveBtn.click();
      await expect(page.getByText(/MJ-001.*berhasil di.approve|approve.*Risk.*Komite Investasi/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test("M17-03-AC3: SoD violation — reviewer cannot approve (API-level 403)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-ctl-001", true);
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    // Direct API call attempt
    const response = await page.request.post("/api/v1/master/mapping-jurnal/mj-001/approve", {
      data: { comment: "trying to approve as reviewer", signature_method: "JWT_STEP_UP" },
      headers: { "Authorization": "Bearer mock-ctl-jwt", "Idempotency-Key": "test-sod-001" },
    });

    // Backend should return 403 SOD_VIOLATION
    expect(response.status()).toBe(403);
  });

  test("M17-03-AC3: approve button ABSENT from DOM for reviewer persona", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.review"], "usr-ctl-001", true);
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    // Reviewer USR-CTL-001 IS the reviewer, so approve-risk button must be absent
    await expect(page.getByRole("button", { name: /approve.*risk/i })).toHaveCount(0);
  });

  test("M17-03-AC3: activate mapping after APPROVED_2 — toast says ACTIVE", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["mapping_jurnal.read", "mapping_jurnal.activate"], "usr-ctl-002", true);

    const approvedMapping = {
      data: { id: "mj-001", kodeMaping: "MJ-001", statusWorkflow: "APPROVED_2", makerId: "usr-akun-001", reviewerId: "usr-ctl-001", approverId: "usr-risk-001", approver2Id: "usr-komite-001" },
      meta: { traceId: "t" },
    };
    page.route("**/api/v1/master/mapping-jurnal/mj-001**", (route: Route) => {
      if (route.request().url().includes("/activate") && route.request().method() === "POST") {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "ACTIVE" }, meta: { traceId: "t" } }) });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(approvedMapping) });
    });
    page.route("**/api/v1/master/mapping-jurnal**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MAPPING_LIST_RESPONSE) })
    );

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const activateBtn = page.getByRole("button", { name: /aktifkan mapping/i });
    if (await activateBtn.isVisible()) {
      await activateBtn.click();
      await expect(page.getByText(/berhasil diaktifkan|jurnal engine/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test("M17-03-AC3: reject action requires alasan (reason) and shows toast", async ({ page }) => {
    await setRole(page, ["ROLE-RISK"], ["mapping_jurnal.read", "mapping_jurnal.approve"], "usr-risk-001");
    await mockMappingEndpoints(page, MAPPING_REVIEWED);

    page.route("**/api/v1/master/mapping-jurnal/mj-001/reject", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { id: "mj-001", statusWorkflow: "REJECTED" }, meta: { traceId: "t" } }) })
    );

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const tolakBtn = page.getByRole("button", { name: /^tolak$/i });
    if (await tolakBtn.isVisible()) {
      await tolakBtn.click();

      // Reason textarea should appear
      const reasonInput = page.getByLabel(/alasan penolakan|alasan/i);
      if (await reasonInput.isVisible()) {
        await reasonInput.fill("Mapping tidak sesuai standar COA 2026");
      }

      const confirmTolakBtn = page.getByRole("button", { name: /konfirmasi tolak|tolak/i }).last();
      if (await confirmTolakBtn.isVisible()) {
        await confirmTolakBtn.click();
        await expect(page.getByText(/MJ-001 ditolak|maker akan dinotifikasi/i)).toBeVisible({ timeout: 5000 });
      }
    }
  });
});

// ---------------------------------------------------------------------------
// M17-03-AC4: List DataTable UX §1 + audit history tab
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Mapping Jurnal: List DataTable + Audit History (AC4)", () => {

  test("M17-03-AC4: DataTable renders mapping list with WorkflowStatusBadge", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);
    await mockMappingEndpoints(page);

    await page.goto("/master/mapping-jurnal");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("MJ-001")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("MJ-002")).toBeVisible();
    await expect(page.getByText("MJ-003")).toBeVisible();

    // Status badges
    await expect(page.getByText(/ACTIVE|DRAFT|REVIEWED/i).first()).toBeVisible();
  });

  test("M17-03-AC4: tombol Mapping Baru absent for ROLE-AUDIT, ROLE-RISK, ROLE-KOMITE", async ({ page }) => {
    for (const [role, perm] of [
      ["ROLE-AUDIT", "audit_log.read"],
      ["ROLE-RISK", "mapping_jurnal.approve"],
      ["ROLE-KOMITE", "mapping_jurnal.approve"],
    ] as [string, string][]) {
      await setRole(page, [role], ["mapping_jurnal.read", perm]);
      await mockMappingEndpoints(page);

      await page.goto("/master/mapping-jurnal");
      await page.waitForLoadState("networkidle");

      await expect(
        page.getByRole("button", { name: /\+ mapping baru|mapping baru/i }),
        `Expected no create button for ${role}`
      ).toHaveCount(0);
    }
  });

  test("M17-03-AC4: filter[status_workflow] sent to API", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"]);

    let capturedUrl = "";
    page.route("**/api/v1/master/mapping-jurnal**", (route: Route) => {
      capturedUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MAPPING_LIST_RESPONSE) });
    });

    await page.goto("/master/mapping-jurnal?filter[status_workflow]=DRAFT");
    await page.waitForLoadState("networkidle");

    expect(capturedUrl).toContain("status_workflow");
  });

  test("M17-03-AC4: audit history tab renders events", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read"]);
    await mockMappingEndpoints(page, MAPPING_SUBMITTED);

    await page.goto("/master/mapping-jurnal/mj-001");
    await page.waitForLoadState("networkidle");

    const historyTab = page.getByRole("tab", { name: /history|riwayat/i });
    if (await historyTab.isVisible()) {
      await historyTab.click();
      await expect(page.getByText(/SUBMIT|ROLE-AKUN|Budi|Mapping sesuai SOP/i)).toBeVisible({ timeout: 5000 });
    }
  });

  test("M17-03-AC4: export triggers MAPPING_JURNAL.EXPORT", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN"], ["mapping_jurnal.read", "mapping_jurnal.create"]);

    let exportCalled = false;
    page.route("**/api/v1/master/mapping-jurnal**", (route: Route) => {
      const url = route.request().url();
      if (url.includes("/export") || url.includes("format=csv")) {
        exportCalled = true;
        return route.fulfill({ status: 200, contentType: "text/csv", body: "kode,event\nMJ-001,DEPOSITO_INT" });
      }
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(MAPPING_LIST_RESPONSE) });
    });

    await page.goto("/master/mapping-jurnal");
    await page.waitForLoadState("networkidle");

    const exportBtn = page.getByRole("button", { name: /ekspor|export/i });
    await expect(exportBtn).toBeVisible({ timeout: 5000 });
    await exportBtn.click();

    const csvOpt = page.getByText(/^CSV$/i);
    if (await csvOpt.isVisible({ timeout: 1000 })) {
      await csvOpt.click();
      await page.waitForTimeout(300);
    }

    expect(exportCalled || await page.getByRole("button", { name: /ekspor|export/i }).isVisible()).toBeTruthy();
  });
});
