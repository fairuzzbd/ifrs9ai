/**
 * /transaksi/penjualan — Server Component permission gate.
 * Requires: transaksi.penjualan.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import PenjualanListClient from "./_page-client";

export const metadata = { title: "Penjualan Instrumen — BLIPS IFRS9" };

export default async function PenjualanListPage() {
  await requirePermission("transaksi.penjualan.read");
  return <PenjualanListClient />;
}
