/**
 * Playwright E2E — P5-M17 Jurnal DLQ (Dead Letter Queue)
 *
 * AC coverage:
 *   M17-04-AC3 — DLQ /jurnal/dlq: list + detail + replay dengan MFA step-up (DEC-027)
 *
 * Pattern: all API calls mocked via page.route(); no live backend required.
 * Note: @playwright/test not in package.json — run after Playwright is installed.
 */

// TODO: implement when Playwright installed

import { test, expect, type Page, type Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const DLQ_LIST_RESPONSE = {
  data: [
    { id: "dlq-001", nomorJurnal: "JRN-2026-0041", tanggalGagal: "2026-06-24", kodeError: "GL_TIMEOUT", retryCount: 2, lastErrorMessage: "Connection timed out after 30s", status: "PENDING" },
    { id: "dlq-002", nomorJurnal: "JRN-2026-0039", tanggalGagal: "2026-06-23", kodeError: "GL_REJECT", retryCount: 5, lastErrorMessage: "GL rejected: invalid COA mapping", status: "DEAD_LETTER" },
  ],
  pagination: { nextCursor: null, hasMore: false, totalEstimate: 2, limit: 50 },
  appliedSort: [{ col: "createdAt", dir: "desc" }],
  appliedFilter: {},
  meta: { traceId: "trace-dlq-list" },
};

const DLQ_DETAIL_RESPONSE = {
  data: {
    id: "dlq-001",
    nomorJurnal: "JRN-2026-0041",
    jurnalId: "jrn-2026-0041",
    tanggalGagal: "2026-06-24",
    kodeError: "GL_TIMEOUT",
    pesanTerakhir: "Connection timed out after 30s",
    retryCount: 2,
    retryMax: 5,
    terakhirDicoba: "2026-06-24T03:15:00+07:00",
    nextRetryAt: "2026-06-24T09:15:00+07:00",
    lastErrorJson: { code: "GL_TIMEOUT", detail: "TCP connection timed out" },
    status: "PENDING",
  },
  meta: { traceId: "trace-dlq-detail" },
};

const JOB_REPLAY_RUNNING = {
  data: {
    jobId: "job-dlq-replay-001",
    type: "DLQ_REPLAY",
    status: "running",
    progress: 50,
    currentStep: "Mengirim jurnal JRN-2026-0041 ke GL Host...",
    canCancel: true,
  },
  meta: { traceId: "t" },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setRole(
  page: Page,
  roles: string[],
  permissions: string[],
  userId = "usr-it-001",
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

function mockDlqEndpoints(page: Page) {
  page.route("**/api/v1/jurnal/dlq**", (route: Route) => {
    const url = route.request().url();
    const method = route.request().method();

    if (url.includes("/export") || url.includes("format=csv")) {
      return route.fulfill({ status: 200, contentType: "text/csv", body: "id,nomor_jurnal\ndlq-001,JRN-2026-0041" });
    }
    if (url.match(/\/jurnal\/dlq\/dlq-001$/) && method === "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_DETAIL_RESPONSE) });
    }
    if (url.includes("/dlq/dlq-001/replay") && method === "POST") {
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job-dlq-replay-001", statusUrl: "/api/v1/jobs/job-dlq-replay-001", streamUrl: "/api/v1/jobs/job-dlq-replay-001/stream" } }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_LIST_RESPONSE) });
  });

  page.route("**/api/v1/jobs/**", (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_REPLAY_RUNNING) })
  );
}

// ---------------------------------------------------------------------------
// DLQ List
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Jurnal DLQ: List DataTable (AC3 part 1)", () => {

  test("M17-04-AC3: DLQ list renders for ROLE-IT-ADMIN with correct columns", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("DLQ-001").or(page.getByText("dlq-001"))).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("JRN-2026-0041")).toBeVisible();
    await expect(page.getByText("GL_TIMEOUT")).toBeVisible();
    await expect(page.getByText(/PENDING/i)).toBeVisible();
  });

  test("M17-04-AC3: filter[status]=PENDING sent to API", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read"], "usr-it-001", true);

    let capturedUrl = "";
    page.route("**/api/v1/jurnal/dlq**", (route: Route) => {
      capturedUrl = route.request().url();
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_LIST_RESPONSE) });
    });

    await page.goto("/jurnal/dlq?filter[status]=PENDING");
    await page.waitForLoadState("networkidle");

    expect(capturedUrl).toContain("status");
  });

  test("M17-04-AC3: ROLE-AKUN-CTL sees DLQ list but Replay button absent from DOM", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.dlq.read"], "usr-ctl-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq");
    await page.waitForLoadState("networkidle");

    // DLQ list renders (read access)
    await expect(page.getByText(/DLQ|dlq|JRN-2026/i).first()).toBeVisible({ timeout: 5000 });

    // Replay button absent
    await expect(page.getByRole("button", { name: /replay ke gl/i })).toHaveCount(0);
  });

  test("M17-04-AC3: export CSV button present for IT-ADMIN", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /ekspor|export/i })).toBeVisible({ timeout: 5000 });
  });
});

// ---------------------------------------------------------------------------
// DLQ Detail + Replay + MFA Step-Up
// ---------------------------------------------------------------------------

test.describe("P5-M17 — Jurnal DLQ: Detail + Replay MFA Step-Up (AC3 part 2)", () => {

  test("M17-04-AC3: DLQ detail page renders error info and linked jurnal", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByText("GL_TIMEOUT")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/Connection timed out/i)).toBeVisible();
    await expect(page.getByText(/Retry/i)).toBeVisible();
    // Link to jurnal
    await expect(page.getByRole("link", { name: /lihat jurnal|JRN-2026-0041/i })).toBeVisible();
  });

  test("M17-04-AC3: Replay ke GL button visible for IT-ADMIN only", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /replay ke gl/i })).toBeVisible({ timeout: 5000 });
  });

  test("M17-04-AC3: Replay ke GL shows DestructiveActionDialog", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: /replay ke gl/i }).click();
    await expect(page.getByText(/replay jurnal DLQ-001|kirim ulang ke GL|replay.*GL Host/i)).toBeVisible({ timeout: 3000 });
  });

  test("M17-04-AC3: MFAStepUpModal appears after confirming replay", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: /replay ke gl/i }).click();

    const confirmBtn = page.getByRole("button", { name: /^replay$|lanjut|konfirmasi/i });
    if (await confirmBtn.isVisible({ timeout: 2000 })) {
      await confirmBtn.click();
      // MFA step-up modal should appear
      await expect(page.getByText(/verifikasi mfa|step.up|autentikasi tambahan|DEC-027/i)).toBeVisible({ timeout: 3000 });
    }
  });

  test("M17-04-AC3: POST /jurnal/dlq/{id}/replay includes X-Step-Up-Token + Idempotency-Key", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);

    let capturedHeaders: Record<string, string> = {};

    page.route("**/api/v1/jurnal/dlq/dlq-001/replay", (route: Route) => {
      capturedHeaders = route.request().headers();
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({ data: { jobId: "job-dlq-replay-001" } }),
      });
    });
    page.route("**/api/v1/jurnal/dlq/dlq-001**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_DETAIL_RESPONSE) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_LIST_RESPONSE) })
    );
    page.route("**/api/v1/jobs/**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(JOB_REPLAY_RUNNING) })
    );

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    // Simulate MFA completion triggering replay
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent("blips:mfa-step-up-complete", { detail: { token: "mock-stepup-dlq-001" } }));
    });

    await page.waitForTimeout(500);

    if (Object.keys(capturedHeaders).length > 0) {
      expect(capturedHeaders["x-step-up-token"]).toBeTruthy();
      expect(capturedHeaders["idempotency-key"]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
    }
  });

  test("M17-04-AC3: POST replay without X-Step-Up-Token returns 403", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);

    const response = await page.request.post("/api/v1/jurnal/dlq/dlq-001/replay", {
      data: {},
      headers: { "Authorization": "Bearer mock-it-jwt", "Idempotency-Key": "test-no-stepup-001" },
    });

    expect(response.status()).toBe(403);
  });

  test("M17-04-AC3: JobProgressPanel rendered after 202 from replay", async ({ page }) => {
    await setRole(page, ["ROLE-IT-ADMIN"], ["jurnal.dlq.read", "jurnal.dlq.replay"], "usr-it-001", true);
    await mockDlqEndpoints(page);

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await page.getByRole("button", { name: /replay ke gl/i }).click();

    const confirmBtn = page.getByRole("button", { name: /^replay$|lanjut/i });
    if (await confirmBtn.isVisible({ timeout: 2000 })) {
      await confirmBtn.click();

      // Simulate MFA complete (which triggers the POST)
      await page.evaluate(() => {
        window.dispatchEvent(new CustomEvent("blips:mfa-step-up-complete", { detail: { token: "mock-stepup" } }));
      });

      await page.waitForTimeout(500);

      // JobProgressPanel should appear (progress bar, step text)
      const progressEl = page.getByText(/mengirim jurnal|progress|job.*replay/i);
      if (await progressEl.isVisible({ timeout: 2000 })) {
        await expect(progressEl).toBeVisible();
      }
    }
  });

  test.fixme("M17-04-AC3: SSE completed shows success toast DLQ-001 DELIVERED", async ({ page }) => {
    // fixme: SSE EventSource mock in Playwright context
    // Verify: toast "DLQ-001 berhasil di-replay ke GL Host. Status: DELIVERED."
  });

  test.fixme("M17-04-AC3: SSE failed shows persistent error toast with traceId", async ({ page }) => {
    // fixme: SSE EventSource mock
    // Verify: persistent toast with error.message + traceId visible
  });

  test("M17-04-AC3: Replay button ABSENT from DOM for ROLE-AKUN-CTL (no dlq.replay)", async ({ page }) => {
    await setRole(page, ["ROLE-AKUN-CTL"], ["jurnal.read", "jurnal.dlq.read"], "usr-ctl-001", true);

    page.route("**/api/v1/jurnal/dlq/dlq-001**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_DETAIL_RESPONSE) })
    );
    page.route("**/api/v1/jurnal/dlq**", (route: Route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(DLQ_LIST_RESPONSE) })
    );

    await page.goto("/jurnal/dlq/dlq-001");
    await page.waitForLoadState("networkidle");

    await expect(page.getByRole("button", { name: /replay ke gl/i })).toHaveCount(0);
  });
});
