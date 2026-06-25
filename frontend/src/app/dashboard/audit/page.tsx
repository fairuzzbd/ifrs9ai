/**
 * P5-M15 — /dashboard/audit — Audit Dashboard (ROLE-AUDIT)
 */

import { requirePermission } from "@/lib/auth/permissions";
import { AuditDashboardClient } from "./_client";

export const metadata = { title: "Audit Dashboard — BLIPS IFRS9" };

export default async function AuditDashboardPage() {
  const jwtPayload = await requirePermission("dashboard.audit.read");

  return (
    <AuditDashboardClient
      username={jwtPayload.preferred_username ?? ""}
      permissions={jwtPayload.permissions ?? []}
      userId={jwtPayload.sub ?? ""}
    />
  );
}
