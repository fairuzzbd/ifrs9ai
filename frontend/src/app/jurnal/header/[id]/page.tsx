import { requirePermission } from "@/lib/auth/permissions";
import { JurnalHeaderDetailPageClient } from "./_page-client";

export default async function JurnalHeaderDetailPage() {
  await requirePermission("jurnal.read");
  return <JurnalHeaderDetailPageClient />;
}
