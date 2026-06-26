/**
 * P5-M15 — /dashboard landing: redirects to role-specific dashboard.
 * Server Component. Never renders — always redirects.
 */

import { redirectToDashboardByRole } from "@/lib/auth/permissions";

export default async function DashboardPage() {
  // This call always throws (redirect) — never returns
  await redirectToDashboardByRole();
}
