/**
 * /transaksi/mtm — Server Component permission gate.
 * Requires: transaksi.mtm.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import MtmListClient from "./_page-client";

export const metadata = { title: "MTM — Mark-to-Market — BLIPS IFRS9" };

export default async function MtmListPage() {
  await requirePermission("transaksi.mtm.read");
  return <MtmListClient />;
}
