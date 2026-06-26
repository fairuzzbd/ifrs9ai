import { requirePermission } from "@/lib/auth/permissions";
import { ReconciliationDailyPageClient } from "./_page-client";

export default async function ReconciliationDailyPage() {
  await requirePermission("jurnal.read");
  return <ReconciliationDailyPageClient />;
}
