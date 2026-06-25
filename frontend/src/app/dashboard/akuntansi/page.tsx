/**
 * P5-M15 — /dashboard/akuntansi — Akuntansi Dashboard (ROLE-AKUN / ROLE-AKUN-CTL)
 */

import { requirePermission } from "@/lib/auth/permissions";
import { AkuntansiDashboardClient } from "./_client";

export const metadata = { title: "Akuntansi Dashboard — BLIPS IFRS9" };

export default async function AkuntansiDashboardPage() {
  const jwtPayload = await requirePermission("dashboard.akuntansi.read");
  const permissions = jwtPayload.permissions ?? [];

  return (
    <AkuntansiDashboardClient
      username={jwtPayload.preferred_username ?? ""}
      permissions={permissions}
      userId={jwtPayload.sub ?? ""}
      canApproveJurnal={permissions.includes("jurnal.approve")}
    />
  );
}
