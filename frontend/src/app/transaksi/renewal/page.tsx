/**
 * /transaksi/renewal — Server Component permission gate.
 * Requires: transaksi.renewal.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import RenewalListClient from "./_page-client";

export const metadata = { title: "Renewal Deposito — BLIPS IFRS9" };

export default async function RenewalListPage() {
  await requirePermission("transaksi.renewal.read");
  return <RenewalListClient />;
}
