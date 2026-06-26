import { requirePermission } from "@/lib/auth/permissions";
import { JurnalHeaderListPageClient } from "./_page-client";

export default async function JurnalHeaderPage() {
  await requirePermission("jurnal.read");
  return <JurnalHeaderListPageClient />;
}
