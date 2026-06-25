/**
 * /transaksi/penempatan/new — Server Component permission gate.
 * Requires: transaksi.penempatan.create
 */
import { requirePermission } from "@/lib/auth/permissions";
import PenempatanNewClient from "./_page-client";

export const metadata = { title: "Buat Penempatan Deposito — BLIPS IFRS9" };

export default async function PenempatanNewPage() {
  await requirePermission("transaksi.penempatan.create");
  return <PenempatanNewClient />;
}
