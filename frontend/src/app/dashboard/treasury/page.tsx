/**
 * P5-M15 — /dashboard/treasury — Treasury Dashboard (ROLE-MAKER-TR / ROLE-APPR-TR)
 * Server Component: permission gate then render client shell.
 */

import { requirePermission } from "@/lib/auth/permissions";
import { TreasuryDashboardClient } from "./_client";

export const metadata = { title: "Treasury Dashboard — BLIPS IFRS9" };

export default async function TreasuryDashboardPage() {
  const jwtPayload = await requirePermission("dashboard.treasury.read");

  return (
    <TreasuryDashboardClient
      username={jwtPayload.preferred_username ?? ""}
      permissions={jwtPayload.permissions ?? []}
      userId={jwtPayload.sub ?? ""}
    />
  );
}
