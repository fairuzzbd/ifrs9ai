/**
 * P5-M15 — Permission helpers for dashboard role-gating.
 *
 * Server-side: `requirePermission` reads JWT from `blips_token` cookie,
 * decodes (no verify — verification done by backend/middleware), checks
 * the permissions[] claim, and redirects via Next.js `redirect()` on miss.
 *
 * Client-side: `hasPermission` / `hasMfaVerified` read from Zustand auth store.
 *
 * Note: This project stores JWT in localStorage (baseFetch reads it).
 * For server components we rely on the `blips_token` cookie that the login
 * flow ALSO sets (so SSR can read it). If only localStorage is set, the
 * server component falls back to `notFound()` — forcing the login flow to
 * set the cookie as well. This matches the security-baseline: server-side
 * gate is the primary protection, client-side is UX-only.
 */

import { cookies } from "next/headers";
import { redirect, notFound } from "next/navigation";

// ---------------------------------------------------------------------------
// JWT decode (no crypto — trust the API gateway to validate)
// ---------------------------------------------------------------------------

interface JwtPayload {
  sub?: string;
  preferred_username?: string;
  roles?: string[];
  permissions?: string[];
  tenant_id?: string;
  mfa_verified?: boolean;
  mfa_method?: string;
  exp?: number;
}

function decodeJwt(token: string): JwtPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    // parts[1] is the base64url-encoded payload
    const raw = parts[1];
    // pad to multiple of 4
    const padded = raw.replace(/-/g, "+").replace(/_/g, "/").padEnd(
      raw.length + ((4 - (raw.length % 4)) % 4),
      "=",
    );
    const json = Buffer.from(padded, "base64").toString("utf-8");
    return JSON.parse(json) as JwtPayload;
  } catch {
    return null;
  }
}

function getJwtFromCookie(): JwtPayload | null {
  const cookieStore = cookies();
  const token = cookieStore.get("blips_token")?.value;
  if (!token) return null;
  return decodeJwt(token);
}

// ---------------------------------------------------------------------------
// Role → default dashboard mapping
// ---------------------------------------------------------------------------

export type DashboardRole =
  | "treasury"
  | "risk"
  | "akuntansi"
  | "cfo"
  | "audit"
  | "jobs";

const ROLE_DASHBOARD_MAP: Record<string, DashboardRole> = {
  "ROLE-MAKER-TR": "treasury",
  "ROLE-APPR-TR": "treasury",
  "ROLE-RISK": "risk",
  "ROLE-AKUN": "akuntansi",
  "ROLE-AKUN-CTL": "akuntansi",
  "ROLE-CFO": "cfo",
  "ROLE-CEO": "cfo",
  "ROLE-KOMITE": "cfo",
  "ROLE-ALCO": "cfo",
  "ROLE-AUDIT": "audit",
  "ROLE-IT-ADMIN": "jobs",
};

export const DASHBOARD_PERMISSION: Record<DashboardRole, string> = {
  treasury: "dashboard.treasury.read",
  risk: "dashboard.risk.read",
  akuntansi: "dashboard.akuntansi.read",
  cfo: "dashboard.cfo.read",
  audit: "dashboard.audit.read",
  jobs: "jobs.read",
};

/**
 * Determine the default dashboard for a user given their roles.
 * Precedence: treasury > risk > akuntansi > cfo > audit > jobs.
 */
export function getDefaultDashboard(roles: string[]): string {
  const precedence: string[] = [
    "ROLE-MAKER-TR",
    "ROLE-APPR-TR",
    "ROLE-RISK",
    "ROLE-AKUN",
    "ROLE-AKUN-CTL",
    "ROLE-CFO",
    "ROLE-CEO",
    "ROLE-KOMITE",
    "ROLE-ALCO",
    "ROLE-AUDIT",
    "ROLE-IT-ADMIN",
  ];
  for (const role of precedence) {
    if (roles.includes(role)) {
      const dash = ROLE_DASHBOARD_MAP[role];
      if (dash === "jobs") return "/jobs";
      return `/dashboard/${dash}`;
    }
  }
  return "/dashboard/treasury";
}

// ---------------------------------------------------------------------------
// Server-side guard (use in Server Components)
// ---------------------------------------------------------------------------

/**
 * Check that the current user (from cookie JWT) has `permission`.
 * If not: redirect to /dashboard (which will re-route based on role).
 * Returns the decoded JWT payload so the page can use it.
 */
export async function requirePermission(permission: string): Promise<JwtPayload> {
  const payload = getJwtFromCookie();
  if (!payload) {
    redirect("/");
  }
  const perms: string[] = payload.permissions ?? [];
  if (!perms.includes(permission)) {
    redirect("/dashboard");
  }
  return payload;
}

/**
 * Same as requirePermission but also checks mfa_verified.
 * Used for /dashboard/cfo.
 */
export async function requirePermissionWithMfa(
  permission: string,
  returnUrl: string,
): Promise<JwtPayload> {
  const payload = await requirePermission(permission);
  if (!payload.mfa_verified) {
    redirect(`/auth/mfa?returnUrl=${encodeURIComponent(returnUrl)}`);
  }
  return payload;
}

/**
 * For /dashboard page: redirect based on role.
 * Never renders; always redirects.
 */
export async function redirectToDashboardByRole(): Promise<never> {
  const payload = getJwtFromCookie();
  if (!payload) {
    redirect("/");
  }
  const roles = payload.roles ?? [];
  const target = getDefaultDashboard(roles);
  redirect(target);
}

// ---------------------------------------------------------------------------
// Client-side helpers (used in Client Components with Zustand)
// ---------------------------------------------------------------------------

/**
 * Client-side permission check.
 * Reads from Zustand auth store user.permissions[].
 * Import UserProfile from auth.store if needed.
 */
export function hasPermission(
  permissions: string[],
  permission: string,
): boolean {
  return permissions.includes(permission);
}

export function hasMfaVerified(mfaVerified: boolean): boolean {
  return mfaVerified === true;
}
