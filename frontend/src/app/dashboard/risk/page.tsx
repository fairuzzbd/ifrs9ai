/**
 * P5-M15 — /dashboard/risk — Risk Dashboard (ROLE-RISK)
 */

import { requirePermission } from "@/lib/auth/permissions";
import { RiskDashboardClient } from "./_client";

export const metadata = { title: "Risk Dashboard — BLIPS IFRS9" };

export default async function RiskDashboardPage() {
  const jwtPayload = await requirePermission("dashboard.risk.read");

  return (
    <RiskDashboardClient
      username={jwtPayload.preferred_username ?? ""}
      permissions={jwtPayload.permissions ?? []}
      userId={jwtPayload.sub ?? ""}
    />
  );
}
