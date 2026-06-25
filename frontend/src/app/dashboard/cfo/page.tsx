/**
 * P5-M15 — /dashboard/cfo — CFO Dashboard (ROLE-CFO / ROLE-CEO / ROLE-KOMITE / ROLE-ALCO)
 * MFA is mandatory for CFO dashboard access.
 */

import { requirePermissionWithMfa } from "@/lib/auth/permissions";
import { CfoDashboardClient } from "./_client";

export const metadata = { title: "CFO Dashboard — BLIPS IFRS9" };

export default async function CfoDashboardPage() {
  const jwtPayload = await requirePermissionWithMfa(
    "dashboard.cfo.read",
    "/dashboard/cfo",
  );

  const permissions = jwtPayload.permissions ?? [];
  const canHardClose = permissions.includes("periode.hardclose");
  const canSealRun = permissions.includes("ecl_run.seal");
  const canApproveParam = permissions.includes("ecl_parameter.approve");

  return (
    <CfoDashboardClient
      username={jwtPayload.preferred_username ?? ""}
      permissions={permissions}
      userId={jwtPayload.sub ?? ""}
      canHardClose={canHardClose}
      canSealRun={canSealRun}
      canApproveParam={canApproveParam}
    />
  );
}
