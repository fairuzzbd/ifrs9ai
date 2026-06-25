/**
 * /transaksi/penempatan — Server Component permission gate.
 * Requires: transaksi.penempatan.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import PenempatanListClient from "./_page-client";

export const metadata = { title: "Penempatan Deposito — BLIPS IFRS9" };

export default async function PenempatanListPage() {
  await requirePermission("transaksi.penempatan.read");
  return <PenempatanListClient />;
}
