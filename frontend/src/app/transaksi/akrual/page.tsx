/**
 * /transaksi/akrual — Server Component permission gate.
 * Requires: transaksi.akrual.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import AkrualListClient from "./_page-client";

export const metadata = { title: "Akrual Harian — BLIPS IFRS9" };

export default async function AkrualListPage() {
  await requirePermission("transaksi.akrual.read");
  return <AkrualListClient />;
}
